package syver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/krameff/syver/matchers"
	"github.com/krameff/syver/resource"
)

// patternWarning is one pattern that matchers.UnterminatedRegex flags: text
// that reads like a regex but is matched as plain text. A value, not a log
// line, like topLevelWarning.
type patternWarning struct {
	Path     string // spec file, "STDIN", or "" (printed as given, omitted when "")
	Type     string // resource key: "file", "command", ...
	ID       string // resource id as written
	Attr     string // resource.PatternRef.Attr
	Pattern  string // resource.PatternRef.Pattern
	Trailing string // from matchers.UnterminatedRegex
}

// String renders the warning body, without the "[WARN] " prefix the caller
// adds. Values are printed verbatim between backticks so the pattern can be
// searched for in the spec; see quoteValue.
func (w patternWarning) String() string {
	loc := ""
	if w.Path != "" {
		loc = w.Path + ": "
	}
	why := `has no closing "/"`
	if w.Trailing != "" {
		why = fmt.Sprintf(`what follows its last "/" (%s) is not i, m or s flags`, quoteValue(w.Trailing))
	}
	return fmt.Sprintf(`%s%s %s %s: %s is matched as plain text: it starts like a regex but %s. `+
		`To make it a regex, end it with "/" (optionally followed by i, m or s); `+
		`to keep it as text, write %s, which also silences this warning`,
		loc, w.Type, quoteValue(w.ID), w.Attr, quoteValue(w.Pattern), why,
		quoteValue(matchers.EscapeAsText(w.Pattern)))
}

// quoteValue prints s between backticks, exactly as written, which is what a
// user pastes back into a single-quoted or plain YAML scalar. A value holding
// a backtick, a newline, invalid UTF-8 or any other unprintable rune is
// printed with strconv.Quote instead, so nothing reaches the terminal raw.
func quoteValue(s string) string {
	if !utf8.ValidString(s) || strings.ContainsAny(s, "`\n") {
		return strconv.Quote(s)
	}
	for _, r := range s {
		if !strconv.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return "`" + s + "`"
}

// checkPatterns walks cfg (top level, then discovery:) and returns one
// warning per distinct (Path, Type, ID, Attr, Pattern), in a stable order:
// resource order from resourceOrder, then ID, then PatternStrings order. It
// only reads cfg.
func checkPatterns(cfg *SyverConfig, path string) []patternWarning {
	descs := resource.Descriptors()
	seen := map[patternWarning]bool{}
	var out []patternWarning

	add := func(typ string, resources map[string]any) {
		ids := make([]string, 0, len(resources))
		for id := range resources {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			r, ok := resources[id].(resource.Resource)
			if !ok {
				continue
			}
			for _, ref := range resource.PatternStrings(r, descs[typ].PatternAttrs) {
				trailing, ok := matchers.UnterminatedRegex(ref.Pattern)
				if !ok {
					continue
				}
				w := patternWarning{Path: path, Type: typ, ID: id, Attr: ref.Attr, Pattern: ref.Pattern, Trailing: trailing}
				if seen[w] {
					continue
				}
				seen[w] = true
				out = append(out, w)
			}
		}
	}

	for _, typ := range resourceOrder {
		if acc, ok := configAccessors[typ]; ok && acc.Get != nil {
			add(typ, acc.Get(cfg))
		}
	}
	for _, typ := range resourceOrder {
		if acc, ok := discoveryAccessors[typ]; ok && acc.Get != nil {
			add(typ, acc.Get(&cfg.Discovery))
		}
	}
	return out
}

// checkPatternsFn is what readJSONData calls. A variable so a test can stub
// the check out and show that it changes no result.
var checkPatternsFn = checkPatterns
