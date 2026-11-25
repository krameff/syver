package matchers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/onsi/gomega/format"
)

const (
	maxScanTokenSize = 1024 * 1024
)

type HavePatternsMatcher struct {
	fakeOmegaMatcher

	Elements        interface{}
	missingElements []string
	foundElements   []string
}

func HavePatterns(elements interface{}) SyverMatcher {
	return &HavePatternsMatcher{
		Elements: elements,
	}
}

func (m *HavePatternsMatcher) Match(actual interface{}) (success bool, err error) {
	t, ok := m.Elements.([]interface{})
	if !ok {
		return false, fmt.Errorf("HavePatterns matcher expects an array of matchers.  Got:\n%s", format.Object(m.Elements, 1))
	}
	elements := make([]string, len(t))
	for i, v := range t {
		switch v := v.(type) {
		case string:
			elements[i] = v
		default:
			return false, fmt.Errorf("HavePatterns matcher expects patterns to be a string. got: \n%s", format.Object(v, 1))
		}
	}
	notfound, err := sliceToPatterns(elements)
	if err != nil {
		return false, err
	}
	// short circuit
	if len(notfound) == 0 {
		return true, nil
	}
	var fh io.Reader
	switch av := actual.(type) {
	case io.Reader:
		fh = av
	case string:
		fh = strings.NewReader(av)
	case []string:
		fh = strings.NewReader(strings.Join(av, "\n"))
	default:
		err = fmt.Errorf("incorrect type %T", actual)

	}
	if err != nil {
		return false, err
	}

	defer func() {
		if rc, ok := fh.(io.ReadCloser); ok {
			rc.Close()
		}
	}()

	scanner := bufio.NewScanner(fh)
	scanner.Buffer(nil, maxScanTokenSize)
	var found []patternMatcher
	for scanner.Scan() {
		line := scanner.Text()

		i := 0
		for _, pat := range notfound {
			if pat.Match(line) {
				// Found it, but wasn't supposed to, don't mark it as found, but remove it from search
				if !pat.Inverse() {
					found = append(found, pat)
				}
				continue
			}
			notfound[i] = pat
			i++
		}
		notfound = notfound[:i]
		if len(notfound) == 0 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}

	for _, pat := range notfound {
		// Didn't find it, but we didn't want to.. so we mark it as found
		// Empty pattern should match even if input to scanner is empty
		if pat.Inverse() || pat.Pattern() == "" {
			found = append(found, pat)
		}
	}

	foundSlice := patternsToSlice(found)
	m.foundElements = foundSlice
	if len(elements) != len(found) {
		m.missingElements = subtractSlice(elements, foundSlice)
		return false, nil
	}
	return true, nil
}

func (m *HavePatternsMatcher) FailureResult(actual interface{}) MatcherResult {
	var a interface{}
	switch actual.(type) {
	case string, []string:
		a = actual
	default:
		a = fmt.Sprintf("object: %T", actual)
	}
	return MatcherResult{
		Actual:          a,
		Message:         "to have patterns",
		Expected:        m.Elements,
		MissingElements: m.missingElements,
		FoundElements:   m.foundElements,
	}
}

func (m *HavePatternsMatcher) NegatedFailureResult(actual interface{}) MatcherResult {
	a, ok := actual.(string)
	if !ok {
		a = fmt.Sprintf("object: %T", actual)
	}
	return MatcherResult{
		Actual:   a,
		Message:  "not to have patterns",
		Expected: m.Elements,
	}
}

type patternMatcher interface {
	Match(string) bool
	Pattern() string
	Inverse() bool
}

type stringPattern struct {
	pattern      string
	cleanPattern string
	inverse      bool
}

func newStringPattern(str string) *stringPattern {
	var inverse bool
	cleanPattern := str
	if strings.HasPrefix(cleanPattern, "!") {
		inverse = true
		cleanPattern = cleanPattern[1:]
	}
	// One leading backslash escapes the next character: "\!foo" is the
	// literal "!foo" and "\/foo" the literal "/foo" (which is how `syver add`
	// keeps a line such as "/var/lib/mis" from parsing as a regex). Only that
	// one backslash goes; every other leading character is part of the text.
	// This used to be strings.TrimLeft(str, "\\/!"), which also ate a leading
	// "/" or "!" of the text itself, so "/tmp" matched "tmpfs".
	cleanPattern = strings.TrimPrefix(cleanPattern, "\\")
	return &stringPattern{
		pattern:      str,
		cleanPattern: cleanPattern,
		inverse:      inverse,
	}
}

func (s *stringPattern) Match(str string) bool {
	return strings.Contains(str, s.cleanPattern)
}

func (s *stringPattern) Pattern() string { return s.pattern }
func (s *stringPattern) Inverse() bool   { return s.inverse }

type regexPattern struct {
	pattern string
	re      *regexp.Regexp
	inverse bool
}

func newRegexPattern(str string) (*regexPattern, error) {
	var inverse bool
	cleanStr := str
	if strings.HasPrefix(str, "!") {
		inverse = true
		cleanStr = cleanStr[1:]
	}
	trimLeft := []rune{'\\', '/'}
	for _, r := range trimLeft {
		if rune(cleanStr[0]) == r {
			cleanStr = cleanStr[1:]
			break
		}
	}
	// Strip trailing closing delimiter and any flags that follow it.
	// Supported flags mirror Go's regexp inline flags: i, m, s.
	// E.g. /pattern/i → compile as (?i)pattern
	// /pattern/ms → compile as (?ms)pattern
	var flags string
	if idx := strings.LastIndex(cleanStr, "/"); idx >= 0 {
		flags = cleanStr[idx+1:]
		cleanStr = cleanStr[:idx]
	}
	if flags != "" {
		cleanStr = "(?" + flags + ")" + cleanStr
	}

	re, err := regexp.Compile(cleanStr)

	return &regexPattern{
		pattern: str,
		re:      re,
		inverse: inverse,
	}, err

}

func (re *regexPattern) Match(str string) bool {
	return re.re.MatchString(str)
}

func (re *regexPattern) Pattern() string { return re.pattern }
func (re *regexPattern) Inverse() bool   { return re.inverse }

// isRegexPattern reports whether s looks like a /regex/ or /regex/flags pattern
// (with an optional leading ! for negation).
func isRegexPattern(s string) bool {
	core := strings.TrimPrefix(s, "!")
	if !strings.HasPrefix(core, "/") {
		return false
	}
	// Find the closing '/' after the opening one.
	closing := strings.LastIndex(core[1:], "/")
	if closing < 0 {
		return false
	}
	// Everything after the closing '/' must be valid regexp flag letters.
	trailingFlags := core[1:][closing+1:]
	for _, r := range trailingFlags {
		switch r {
		case 'i', 'm', 's':
			// valid Go regexp inline flags
		default:
			return false
		}
	}
	return true
}

var (
	// regexEscapeRe is a backslash before a regex class letter or a
	// metacharacter: \s, \d, \b, \., \(, ...
	regexEscapeRe = regexp.MustCompile(`\\[sSdDwWbB.\[\]()*+?{}|^$/]`)
	// regexGroupRe is "(?", a group holding "|", or ")" followed by a
	// quantifier. Parenthesised text such as "(rw,nosuid)" or "$(uname -r)"
	// is none of these.
	regexGroupRe = regexp.MustCompile(`\(\?|\([^)]*\|[^)]*\)|\)[?*+{]`)
	// regexAltRe is a "|" with neither a space nor another "|" on either
	// side, so a shell pipe or "||" is not alternation.
	regexAltRe = regexp.MustCompile(`[^ |]\|[^ |]`)
	// regexQuantRe is a bounded quantifier: {n} or {n,} or {n,m}.
	regexQuantRe = regexp.MustCompile(`\{\d+(,\d*)?\}`)
)

// hasRegexSyntax reports whether body (a pattern without its leading "!" and
// "/") contains syntax that is common in regexes and rare in paths and plain
// text. A lone ".", "*", "+" or "?" does not count: paths and audit text use
// them all the time.
func hasRegexSyntax(body string) bool {
	if strings.HasPrefix(body, "^") || strings.HasSuffix(body, "$") {
		return true
	}
	if regexEscapeRe.MatchString(body) {
		return true
	}
	// ".*" or ".+" anywhere but at the very end: a trailing one is how a
	// glob-like path is often written ("/etc/cron.*", "/var/log/.*").
	rest := body
	if strings.HasSuffix(rest, ".*") || strings.HasSuffix(rest, ".+") {
		rest = rest[:len(rest)-2]
	}
	if strings.Contains(rest, ".*") || strings.Contains(rest, ".+") {
		return true
	}
	if i := strings.Index(body, "["); i >= 0 && strings.Contains(body[i+1:], "]") {
		return true
	}
	return regexGroupRe.MatchString(body) || regexAltRe.MatchString(body) || regexQuantRe.MatchString(body)
}

// UnterminatedRegex reports whether s will be matched as plain text
// although it reads as a regex whose closing "/" is missing or is followed
// by something other than i, m, s flags. ok is false for everything else,
// including every real regex and every plain path. trailing is the text
// after the last "/" when there is one and it is not empty ("" when the
// pattern has no later "/"), so the caller can say which mistake it is.
//
// The test is: s starts with "/" or "!/", isRegexPattern(s) is false (so
// this can never disagree with how the pattern is matched), and the rest
// contains regex syntax (see hasRegexSyntax). A slash-led pattern with no
// such syntax, such as "/faillog_t", cannot be told from a path and is not
// reported.
func UnterminatedRegex(s string) (trailing string, ok bool) {
	core := strings.TrimPrefix(s, "!")
	if !strings.HasPrefix(core, "/") || isRegexPattern(s) {
		return "", false
	}
	body := core[1:]
	if !hasRegexSyntax(body) {
		return "", false
	}
	if i := strings.LastIndex(body, "/"); i >= 0 {
		trailing = body[i+1:]
	}
	return trailing, true
}

// EscapeAsText returns s with a "\" before its leading "/" ("!/x" -> "!\/x",
// "/x" -> "\/x"): the same text pattern, written so it no longer warns. A
// pattern that does not start with "/" or "!/" is returned unchanged.
func EscapeAsText(s string) string {
	switch {
	case strings.HasPrefix(s, "!/"):
		return `!\` + s[1:]
	case strings.HasPrefix(s, "/"):
		return `\` + s
	default:
		return s
	}
}

func sliceToPatterns(slice []string) ([]patternMatcher, error) {
	var patterns []patternMatcher
	for _, s := range slice {
		if isRegexPattern(s) {
			pat, err := newRegexPattern(s)
			if err != nil {
				return nil, err
			}
			patterns = append(patterns, pat)
		} else {
			patterns = append(patterns, newStringPattern(s))
		}
	}
	return patterns, nil
}

func patternsToSlice(patterns []patternMatcher) []string {
	var slice []string
	for _, p := range patterns {
		slice = append(slice, p.Pattern())
	}
	return slice
}
func subtractSlice(x, y []string) []string {
	m := make(map[string]bool)

	for _, y := range y {
		m[y] = true
	}

	var ret []string
	for _, x := range x {
		if m[x] {
			continue
		}
		ret = append(ret, x)
	}

	return ret
}

func (matcher *HavePatternsMatcher) MarshalJSON() ([]byte, error) {
	j := make(map[string]interface{})
	j["have-patterns"] = matcher.Elements
	return json.Marshal(j)
}
