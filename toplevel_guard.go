package syver

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/krameff/syver/resource"
)

// topLevelWarning describes one top-level key found in a spec that
// checkTopLevelKeys does not recognise. It is a value, not a log line, so
// callers decide whether and how to surface it -- see D1 in
// PLAN_toplevel_key_guard.md: this guard never fails a run, it only reports.
type topLevelWarning struct {
	// Path is the spec file this key was found in, or "" if the caller had
	// none to give (e.g. content read from a byte slice with no filename).
	// See D7: an included gossfile's warning carries that file's own path,
	// not its parent's.
	Path string
	// Line is the 1-indexed source line of the key itself, from yaml.v3's
	// Node.Line.
	Line int
	// Key is the unrecognised top-level key, exactly as written.
	Key string
	// Suggestion is the closest legal key within edit distance 2, or "" if
	// none qualifies. See D6.
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
// hand-written, plus the two top-level keys that are not resource types:
// "discovery" (SyverConfig.Discovery) and "syverfile" (the gossfile: input
// alias, resource.SyverfileMap's Alias entry -- Descriptors() itself omits
// alias keys, see its doc comment).
//
// TestLegalTopLevelKeys_MatchesSyverConfigYAMLTags pins this set against
// SyverConfig's own yaml tags by reflection: a resource type added without a
// matching Register call, or a SyverConfig field added without one, fails
// that test instead of producing a false warning (or a silent gap) here.
func legalTopLevelKeys() map[string]bool {
	descriptors := resource.Descriptors()
	keys := make(map[string]bool, len(descriptors)+2)
	for key := range descriptors {
		keys[key] = true
	}
	keys["discovery"] = true
	keys["syverfile"] = true
	return keys
}

// checkTopLevelKeys reports unknown top-level keys in a YAML spec document.
// It never fails a run (see D1) and never mutates anything -- it returns the
// warnings rather than logging them, so the caller decides (and so this is
// testable without capturing log output). data is parsed independently of
// the real decode in ReadJSONData; see the design note in
// PLAN_toplevel_key_guard.md on why the node is not threaded through to the
// real unmarshal.
//
// Three kinds of key are exempt, per D2:
//   - a key already in legalTopLevelKeys()
//   - a key beginning "x-" (the docker-compose-style extension convention)
//   - a key whose immediate value node itself carries an anchor -- the
//     mechanism that makes a shared-block-via-YAML-anchor pattern like
//
//       common-checks: &common
//         exit-status: 0
//
//     work today. Detected structurally via yaml.v3's Node.Anchor, not by
//     name -- see the trap section of PLAN_toplevel_key_guard.md for why a
//     naming heuristic would be wrong here. Deliberately NOT a recursive
//     subtree search -- see valueHasAnchor's doc comment for why that
//     distinction matters.
//
// A malformed document, or one whose top level is not a mapping, produces no
// warnings -- the real decode in ReadJSONData surfaces that failure on its
// own path; this guard is not the place to report it a second time.
func checkTopLevelKeys(data []byte, path string) []topLevelWarning {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil
	}
	if len(root.Content) == 0 {
		return nil
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}

	legal := legalTopLevelKeys()

	var warnings []topLevelWarning
	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		key := keyNode.Value

		if legal[key] {
			continue
		}
		if strings.HasPrefix(key, "x-") {
			continue
		}
		if valueHasAnchor(valNode) {
			continue
		}

		warnings = append(warnings, topLevelWarning{
			Path:       path,
			Line:       keyNode.Line,
			Key:        key,
			Suggestion: suggestKey(key, legal),
		})
	}

	return warnings
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
// if that distance is 1 or 2 (D6), or "" if nothing qualifies. Ties are
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
