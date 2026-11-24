package syver

import (
	"bytes"
	"crypto/fips140"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/go-sprout/sprout/sprigin"
)

// TemplateFilter is the type of the Goss Template Filter which include custom variables and functions.
type TemplateFilter func([]byte) ([]byte, error)

// NewTemplateFilter creates a new Template Filter based in the file and inline variables.
func NewTemplateFilter(varsFiles []string, varsInline string, discovered map[string]bool) (func([]byte) ([]byte, error), error) {
	return newTemplateFilter(varsFiles, varsInline, discovered, "error")
}

// NewPeekTemplateFilter renders templates before discovery results exist. Missing
// .Discovered keys evaluate as false instead of failing template execution.
func NewPeekTemplateFilter(varsFiles []string, varsInline string) (TemplateFilter, error) {
	return newTemplateFilter(varsFiles, varsInline, nil, "zero")
}

func newTemplateFilter(varsFiles []string, varsInline string, discovered map[string]bool, missingKey string) (TemplateFilter, error) {
	vars, err := loadVarsForTemplates(varsFiles, varsInline, discovered)
	if err != nil {
		return nil, fmt.Errorf("failed while loading vars files %q: %w", varsFiles, err)
	}

	tVars := &TmplVars{
		Vars:       vars,
		Discovered: discoveredFromVars(vars),
	}

	// Built once per filter, so a sprout upgrade that changes a guarded
	// function's shape panics here, in every test that renders a template.
	fipsGuards := fipsGuardTemplateFuncs(sprigin.TxtFuncMap())

	f := func(data []byte) ([]byte, error) {
		// funcMap is applied SECOND on purpose, and the order is load-bearing.
		// Three names collide with sprout: regexMatch, toUpper and toLower.
		// The last two collide only because of sprout -- sprig had no toUpper
		// or toLower at all, so before that swap nothing competed with ours.
		//
		// It is not a cosmetic win. sprout's toUpper is
		// func(...any) (any, error) and yields nothing for a non-string, so
		// `{{ toUpper 42 }}` would render an empty value and the check would
		// pass having asserted nothing. Ours is func(string) string, so the
		// same expression fails the run and names the line. Reversing these
		// two calls turns a loud failure into a silent one.
		//
		// fipsGuards sits between the two: it replaces sprout's own entries for
		// the guarded names and shares no name with funcMap.
		t := template.New("test").Funcs(sprigin.TxtFuncMap()).Funcs(fipsGuards).Funcs(funcMap)

		tmpl, err := t.Parse(string(data))
		if err != nil {
			return []byte{}, err
		}

		tmpl.Option("missingkey=" + missingKey)
		var doc bytes.Buffer

		err = tmpl.Execute(&doc, tVars)
		if err != nil {
			return []byte{}, err
		}

		return doc.Bytes(), nil
	}

	return f, nil
}

func mkSlice(args ...any) []any {
	return args
}

func readFile(f string) (string, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return "", err

	}
	return strings.TrimSpace(string(b)), nil
}

// globWarned records each pattern that has already produced a "matched no
// files" WARN, so a pattern warns once per process however many times it is
// evaluated: twice in one template, in several included gossfiles, or in both
// the --discover spec and the main one.
var globWarned = struct {
	sync.Mutex
	seen map[string]bool
}{seen: map[string]bool{}}

// warnGlobNoMatch logs the no-match WARN for pattern, at most once per
// process. During a peek pass (quietDecode, see store.go) it does nothing and
// records nothing: the peek render is discarded, and the real load that
// follows is the one that should warn.
func warnGlobNoMatch(pattern string) {
	if quietDecode {
		return
	}
	globWarned.Lock()
	defer globWarned.Unlock()
	if globWarned.seen[pattern] {
		return
	}
	globWarned.seen[pattern] = true
	log.Printf("[WARN] glob pattern %q matched no files", pattern)
}

// templateGlob is the "glob" template function. Each argument is one
// filepath.Glob pattern; the matches of every pattern are cleaned, merged,
// de-duplicated and returned sorted. A pattern that matches nothing (including
// a plain path that does not exist) WARNs and contributes no paths. A
// malformed pattern fails the render, naming it.
func templateGlob(patterns ...string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, errors.New("glob: at least one pattern is required")
	}

	seen := make(map[string]bool)
	matches := []string{}
	for _, p := range patterns {
		m, err := filepath.Glob(p)
		if err != nil {
			return nil, fmt.Errorf("error in expanding glob pattern %q: %w", p, err)
		}
		if len(m) == 0 {
			warnGlobNoMatch(p)
			continue
		}
		for _, f := range m {
			// A plain path comes back exactly as written; Clean it so two
			// spellings of one file ("/etc//hosts", "C:/x" vs "C:\x") dedupe.
			f = filepath.Clean(f)
			if !seen[f] {
				seen[f] = true
				matches = append(matches, f)
			}
		}
	}
	slices.Sort(matches)

	return matches, nil
}

func getEnv(key string, def ...string) string {
	val := os.Getenv(key)
	if val == "" && len(def) > 0 {
		return def[0]
	}

	return os.Getenv(key)
}

func regexMatch(re, s string) (bool, error) {
	compiled, err := regexp.Compile(re)
	if err != nil {
		return false, err
	}

	return compiled.MatchString(s), nil
}

// return named parenthesized subexpresions, if received, or stringfied (Sprout "get" needs strings) keys like array
func findStringSubmatch(pattern, input string) map[string]interface{} {
	re := regexp.MustCompile(pattern)
	els := re.FindStringSubmatch(input)

	elsMap := make(map[string]interface{})
	elsMapNamed := make(map[string]interface{})

	// create always elsMaps but returns elsMapNamed if exists named parenthesized subexps
	for i := 0; i < len(els); i++ {
		// convert i to string according returned (https://github.com/goss-org/goss/pull/895#issuecomment-2075716706)
		elsMap[strconv.Itoa(i)] = els[i]

		if re.SubexpNames()[i] != "" {
			elsMapNamed[re.SubexpNames()[i]] = els[i]
		}
	}

	// returns elsMapNamed if exists named parenthesized subexps
	if len(elsMapNamed) > 0 {
		return elsMapNamed
	}
	return elsMap
}

var funcMap = template.FuncMap{
	"mkSlice":            mkSlice,
	"readFile":           readFile,
	"glob":               templateGlob,
	"getEnv":             getEnv,
	"regexMatch":         regexMatch,
	"toUpper":            strings.ToUpper,
	"toLower":            strings.ToLower,
	"findStringSubmatch": findStringSubmatch,
}

// fipsGuardedTemplateFuncs names the sprout template functions that render an
// empty value, silently, when syver runs in Go's FIPS 140-only mode
// (GODEBUG=fips140=only), mapped to the hint their error gives. sprout's
// backward-compatible wrapper discards every error and recovered panic, so a
// refused algorithm becomes "<no value>" and the render succeeds. The guard
// turns that into a failed render naming the function.
//
// The authority for this set is TestTemplateFIPSProbe, which calls every
// sprout function with and without enforcement: a function that returns a
// value without it and an empty one with it must be listed here or in
// fipsSilentTemplateFuncsAccepted, and every name here must show that
// difference.
var fipsGuardedTemplateFuncs = map[string]string{
	"md5sum":        "for a checksum, use sha256Sum",
	"md5Sum":        "for a checksum, use sha256Sum",
	"sha1sum":       "for a checksum, use sha256Sum",
	"sha1Sum":       "for a checksum, use sha256Sum",
	"genPrivateKey": "for a key, use rsa, ecdsa or ed25519",
}

// fipsSilentTemplateFuncsAccepted lists functions that render empty in FIPS
// 140-only mode but are deliberately not guarded, with the reason. Empty:
// everything the probe finds is guarded.
var fipsSilentTemplateFuncsAccepted = map[string]string{}

// templateFIPSEnforced reports FIPS 140-only mode. A variable so the tests
// can force it.
var templateFIPSEnforced = fips140.Enforced

// sproutFunc is the shape sprout's backward-compatible map gives every
// function whose name does not start with "must".
type sproutFunc = func(...any) (any, error)

// fipsGuardTemplateFuncs wraps each guarded function of the sprout map. It
// panics if a guarded name is missing or has another shape: a sprout upgrade
// that changes either must fail loudly, not quietly drop the guard.
func fipsGuardTemplateFuncs(sprout template.FuncMap) template.FuncMap {
	guards := make(template.FuncMap, len(fipsGuardedTemplateFuncs))
	for name, hint := range fipsGuardedTemplateFuncs {
		v, ok := sprout[name]
		if !ok {
			panic(fmt.Sprintf("template function %s is FIPS-guarded but sprout no longer provides it", name))
		}
		fn, ok := v.(sproutFunc)
		if !ok {
			panic(fmt.Sprintf("template function %s is FIPS-guarded but sprout's value is now %T, not func(...any) (any, error)", name, v))
		}
		guards[name] = fipsGuardTemplateFunc(name, hint, fn)
	}
	return guards
}

// emptyTemplateValue reports whether a sprout result is what its wrapper
// leaves behind when the function failed: nil after a recovered panic (the
// md5 and sha1 checksums), or the zero value the function returned alongside
// its error (genPrivateKey "dsa" gives "").
func emptyTemplateValue(v any) bool {
	return v == nil || reflect.ValueOf(v).IsZero()
}

// fipsGuardTemplateFunc calls the sprout function unchanged, sprout's own
// deprecation warnings included. Only in FIPS 140-only mode, and only when
// the call produced an empty value, does it return an error instead: a value
// is passed through, so `genPrivateKey "rsa"` still works in that mode. None
// of the guarded functions returns an empty value on success.
func fipsGuardTemplateFunc(name, hint string, delegate sproutFunc) sproutFunc {
	return func(args ...any) (any, error) {
		out, err := delegate(args...)
		if err != nil || !emptyTemplateValue(out) || !templateFIPSEnforced() {
			return out, err
		}
		return nil, fmt.Errorf("template function %s failed in FIPS 140-only mode: it uses an algorithm that mode does not allow (%s)", name, hint)
	}
}
