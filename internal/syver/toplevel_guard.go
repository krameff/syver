package syver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/krameff/syver/resource"
)

// topLevelWarning describes one top-level key found in a spec that
// checkTopLevelKeys does not recognise. It is a value, not a log line, so
// callers decide whether and how to surface it. By default an unknown key is
// only reported, as a WARN: a hard error would break the anchor pattern
// described on scanTopLevelYAML, and any spec written for a newer syver and
// run on an older one. A file that declares syver-version: opts in to having
// them returned as one error instead (see strictKeysError).
type topLevelWarning struct {
	// Path is the spec file this key was found in, or "" if the caller had
	// none to give (e.g. content read from a byte slice with no filename).
	// An included gossfile's warning carries that file's own path, not its
	// parent's.
	Path string
	// Line is the 1-indexed source line of the key itself, from yaml.v3's
	// Node.Line.
	Line int
	// Key is the unrecognised top-level key, exactly as written.
	Key string
	// Suggestion is the closest legal key within edit distance 2, or "" if
	// none qualifies.
	Suggestion string
}

// String renders the warning body (without the leading "[WARN] " prefix --
// callers that log it are expected to add that themselves, matching the
// existing convention at store.go's gossfile:/syverfile: collision WARN).
func (w topLevelWarning) String() string {
	loc := ""
	if w.Path != "" {
		loc = w.Path + ":"
	}
	msg := fmt.Sprintf("%s%d: unknown top-level key %q -- ignored", loc, w.Line, w.Key)
	if w.Suggestion != "" {
		msg += fmt.Sprintf(" (did you mean %q?)", w.Suggestion)
	}
	return msg
}

// legalTopLevelKeys returns the full set of top-level keys checkTopLevelKeys
// treats as known. Derived from resource.Descriptors() rather than
// hand-written, plus the three top-level keys that are not resource types:
// "discovery" (SyverConfig.Discovery), "syverfile" (the gossfile: input
// alias, resource.SyverfileMap's Alias entry -- Descriptors() itself omits
// alias keys, see its doc comment) and "syver-version"
// (SyverConfig.SyverVersion).
//
// TestLegalTopLevelKeys_MatchesSyverConfigYAMLTags pins this set against
// SyverConfig's own yaml tags by reflection: a resource type added without a
// matching Register call, or a SyverConfig field added without one, fails
// that test instead of producing a false warning (or a silent gap) here.
func legalTopLevelKeys() map[string]bool {
	descriptors := resource.Descriptors()
	keys := make(map[string]bool, len(descriptors)+3)
	for key := range descriptors {
		keys[key] = true
	}
	keys["discovery"] = true
	keys["syverfile"] = true
	keys[syverVersionKey] = true
	return keys
}

// checkTopLevelKeys reports unknown top-level keys in a YAML spec document.
// It never fails a run and never mutates anything -- it returns the
// warnings rather than logging them, so the caller decides (and so this is
// testable without capturing log output). It is scanTopLevelYAML's unknown
// keys, without the syver-version: details.
func checkTopLevelKeys(data []byte, path string) []topLevelWarning {
	return scanTopLevelYAML(data, path).unknown
}

// topLevelScan is what one walk of a spec's top-level mapping finds: the keys
// syver does not recognise, and whether and how the file declares
// syver-version:.
type topLevelScan struct {
	unknown []topLevelWarning
	// marker is true when the syver-version key is present at all, whatever
	// its value. An empty or null value still makes the file strict, which
	// is why this is not derived from the decoded SyverVersion field: both
	// `syver-version:` and `syver-version: ~` decode to "".
	marker bool
	// markerLine is the 1-indexed line of the syver-version key, or 0 where
	// the format carries no line numbers (JSON).
	markerLine int
	// markerValue is the value as written, or "" for a null value. A value
	// that is not a scalar is left "" here; the real decode rejects it.
	markerValue string
	// markerScalar is false when the value is a sequence or map, so the
	// caller can leave that to the decode error instead of also reporting a
	// missing version.
	markerScalar bool
}

// scanTopLevelYAML walks a YAML spec's top-level mapping once. data is parsed
// independently of the real decode in ReadJSONData. Spec files are small and
// this runs once per file, so the second parse costs nothing worth saving,
// and threading the node through to the real decode would touch the merge
// and `gossfile:` recursion paths for no gain.
//
// Four kinds of key are exempt:
//
//   - a key already in legalTopLevelKeys()
//
//   - a key beginning "x-" (the docker-compose-style extension convention)
//
//   - a key whose immediate value node itself carries an anchor. Detected
//     structurally via yaml.v3's Node.Anchor, not by name: an anchor carrier
//     can be called anything, so nothing about its name tells it apart from
//     a typo. Deliberately NOT a recursive subtree search -- see
//     valueHasAnchor's doc comment for why that distinction matters.
//
//   - a YAML merge key, `<<`, whose value is an alias or a sequence of
//     aliases. The decode applies the merge, so the key itself is never
//     dropped. Any other `<<` value is reported like any unknown key.
//
// The third exemption is the mechanism that keeps a shared-block-via-YAML-anchor
// pattern working today:
//
//	common-checks: &common
//	  exit-status: 0
//
// A malformed document, or one whose top level is not a mapping, produces an
// empty scan -- the real decode in ReadJSONData surfaces that failure on its
// own path; this guard is not the place to report it a second time.
func scanTopLevelYAML(data []byte, path string) topLevelScan {
	var scan topLevelScan
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return scan
	}
	if len(root.Content) == 0 {
		return scan
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return scan
	}

	legal := legalTopLevelKeys()

	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		key := keyNode.Value

		if key == syverVersionKey {
			scan.marker = true
			scan.markerLine = keyNode.Line
			if valNode.Kind == yaml.ScalarNode {
				scan.markerScalar = true
				if valNode.Tag != "!!null" {
					scan.markerValue = valNode.Value
				}
			}
		}
		if legal[key] {
			continue
		}
		if strings.HasPrefix(key, "x-") {
			continue
		}
		if valueHasAnchor(valNode) {
			continue
		}
		if key == "<<" && isMergeOfAliases(valNode) {
			// The merge itself is legal, but the keys it brings in land at
			// the top level and are dropped like any other unknown key.
			scan.unknown = append(scan.unknown, mergedUnknownKeys(valNode, path, legal, map[*yaml.Node]bool{})...)
			continue
		}

		scan.unknown = append(scan.unknown, topLevelWarning{
			Path:       path,
			Line:       keyNode.Line,
			Key:        key,
			Suggestion: suggestKey(key, legal),
		})
	}

	return scan
}

// isMergeOfAliases reports whether n, the value of a `<<` key, is an alias or
// a non-empty sequence made only of aliases: the two forms a YAML merge takes.
func isMergeOfAliases(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.AliasNode:
		return true
	case yaml.SequenceNode:
		if len(n.Content) == 0 {
			return false
		}
		for _, item := range n.Content {
			if item.Kind != yaml.AliasNode {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// mergedUnknownKeys returns the unknown keys a top-level `<<` merge brings in:
// the keys of each aliased mapping, with the same exemptions as a key written
// at the top level, and following merges nested inside those mappings. Lines
// point at the key inside the anchored block. seen stops an alias cycle.
func mergedUnknownKeys(n *yaml.Node, path string, legal map[string]bool, seen map[*yaml.Node]bool) []topLevelWarning {
	var targets []*yaml.Node
	switch n.Kind {
	case yaml.AliasNode:
		targets = []*yaml.Node{n.Alias}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if item.Kind == yaml.AliasNode {
				targets = append(targets, item.Alias)
			}
		}
	}
	var out []topLevelWarning
	for _, m := range targets {
		if m == nil || m.Kind != yaml.MappingNode || seen[m] {
			continue
		}
		seen[m] = true
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			if k.Kind != yaml.ScalarNode {
				continue
			}
			switch {
			case legal[k.Value], strings.HasPrefix(k.Value, "x-"), valueHasAnchor(v):
			case k.Value == "<<" && isMergeOfAliases(v):
				out = append(out, mergedUnknownKeys(v, path, legal, seen)...)
			default:
				out = append(out, topLevelWarning{
					Path:       path,
					Line:       k.Line,
					Key:        k.Value,
					Suggestion: suggestKey(k.Value, legal),
				})
			}
		}
	}
	return out
}

// scanTopLevelJSON is scanTopLevelYAML for a JSON spec: the same exemptions
// apart from the two that are YAML syntax (anchors and merge keys), and no
// line numbers. A document whose top level is not an object produces an empty
// scan and is left to the real decode.
func scanTopLevelJSON(data []byte, path string) topLevelScan {
	var scan topLevelScan
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return scan
	}

	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	// encoding/json does not keep key order, so report in a stable order.
	sort.Strings(keys)

	legal := legalTopLevelKeys()
	for _, key := range keys {
		if key == syverVersionKey {
			scan.marker = true
			raw := bytes.TrimSpace(doc[key])
			var s string
			switch {
			case bytes.Equal(raw, []byte("null")):
				scan.markerScalar = true
			case json.Unmarshal(raw, &s) == nil:
				scan.markerScalar = true
				scan.markerValue = s
			}
		}
		if legal[key] || strings.HasPrefix(key, "x-") {
			continue
		}
		scan.unknown = append(scan.unknown, topLevelWarning{
			Path:       path,
			Key:        key,
			Suggestion: suggestKey(key, legal),
		})
	}
	return scan
}

// strictKeysError is the single error a strict file's unknown top-level keys
// produce. strictOrigin is the file whose syver-version: made this one strict:
// path itself, or an ancestor that includes it.
func strictKeysError(path, strictOrigin string, unknown []topLevelWarning) error {
	var b strings.Builder
	if strictOrigin == specLabel(path) {
		fmt.Fprintf(&b, "%s: %s is set, so unknown top-level keys are errors (prefix a key with x- to keep it):",
			specLabel(path), syverVersionKey)
	} else {
		fmt.Fprintf(&b, "%s: unknown top-level keys are errors (prefix a key with x- to keep it) "+
			"(strict because it is included from %s, which sets %s):",
			specLabel(path), specLabel(strictOrigin), syverVersionKey)
	}
	for _, w := range unknown {
		b.WriteString("\n  ")
		b.WriteString(specLabel(w.Path))
		if w.Line > 0 {
			fmt.Fprintf(&b, ":%d", w.Line)
		}
		fmt.Fprintf(&b, ": %q", w.Key)
		if w.Suggestion != "" {
			fmt.Fprintf(&b, " (did you mean %q?)", w.Suggestion)
		}
	}
	return errors.New(b.String())
}

// specLabel names a spec in a message. Content read with no path, such as a
// byte slice in a test, is called "spec".
func specLabel(path string) string {
	if path == "" {
		return "spec"
	}
	return path
}

// markerLocation is the "path:line" (or just "path" for JSON) prefix of a
// message about a file's syver-version: value.
func markerLocation(path string, scan topLevelScan) string {
	if scan.markerLine > 0 {
		return fmt.Sprintf("%s:%d", specLabel(path), scan.markerLine)
	}
	return specLabel(path)
}

// checkDeclaredVersion validates a file's own syver-version: value and
// compares it with the running binary's stamp. It returns nil for a file that
// does not declare the key, and for a non-scalar value, which the real decode
// has already rejected.
func checkDeclaredVersion(path string, scan topLevelScan, stamp string) error {
	if !scan.marker || !scan.markerScalar {
		return nil
	}
	required, err := parseRequiredVersion(scan.markerValue)
	if err != nil {
		return fmt.Errorf("%s: %w", markerLocation(path, scan), err)
	}
	if err := checkMinimumVersion(required, stamp); err != nil {
		var old *versionTooOldError
		if errors.As(err, &old) {
			return fmt.Errorf("%s requires syver %s or later (%s: %s); this is syver %s",
				specLabel(path), old.required, syverVersionKey, strings.TrimSpace(scan.markerValue), old.running)
		}
		return err
	}
	return nil
}

// valueHasAnchor reports whether n *itself* carries a YAML anchor --
// NOT whether an anchor exists anywhere inside it.
//
// This is deliberately not recursive. The exemption exists to cover the
// specific pattern where a top-level key's value is the anchor definition
// itself:
//
//	common-checks: &common
//	  exit-status: 0
//
// Here the anchor is declared directly on the node checkTopLevelKeys is
// asking about (the value of common-checks:), so n.Anchor != "" catches it
// with no need to look further. It must NOT extend to a case like
//
//	bogus:
//	  nested:
//	    deep: &deepanchor
//	      a: 1
//
// where an anchor merely happens to exist three levels down inside an
// otherwise ordinary block. That is not evidence bogus: exists to carry
// anything -- it is an unrelated anchor nested inside a block that is
// still, independently, an unrecognised top-level key, and it must warn.
// Recursing into n.Content (as an earlier version of this function did)
// blurs that distinction and silently exempts any unknown key with an
// anchor buried anywhere below it, however deep or unrelated. If a future
// "simplify this" pass reintroduces a walk over n.Content here, it is
// reintroducing exactly that bug -- see
// TestCheckTopLevelKeys_DeepAnchorDoesNotExempt, which pins this.
//
// An alias reference (`<<: *common`) does not itself define an anchor --
// yaml.v3 only sets Node.Anchor on the node that originally declared it --
// so a block that merely *uses* an anchor defined elsewhere is unaffected
// by this function either way. See the anchor-reference negative test.
func valueHasAnchor(n *yaml.Node) bool {
	return n != nil && n.Anchor != ""
}

// suggestKey returns the legal key closest to key by Levenshtein distance,
// if that distance is 1 or 2, or "" if nothing qualifies. Ties are
// broken by sorting the candidate set first, so the result is deterministic.
func suggestKey(key string, legal map[string]bool) string {
	candidates := make([]string, 0, len(legal))
	for k := range legal {
		candidates = append(candidates, k)
	}
	sort.Strings(candidates)

	best := ""
	bestDist := 3 // anything >= 3 means "no suggestion"
	for _, k := range candidates {
		d := levenshtein(key, k)
		if d >= 1 && d <= 2 && d < bestDist {
			bestDist = d
			best = k
		}
	}
	return best
}

// levenshtein computes the classic edit distance between a and b (insert,
// delete, substitute, each cost 1). Rune-based so it never misbehaves on
// multi-byte keys, though every legal top-level key today is plain ASCII.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}
