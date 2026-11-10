package syver

import (
	"bytes"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/go-sprout/sprout/sprigin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krameff/syver/util"
)

// This file did not exist before FEAT-009 (the sprig -> sprout migration).
// The template layer had almost no direct coverage, so these tests are the
// whole safety net for that swap. Every case here was revert-proved by hand
// while writing it: either by temporarily reverting the assertion to sprig's
// old value/behaviour, by removing the entry under test, or by breaking the
// specific mechanism the test claims to guard, then restoring it once the
// test failed as expected. See FEAT-009-sprig-to-sprout.md "New tests".

// render runs tmplSrc through the exact same construction NewTemplateFilter
// uses (missingkey=error, no discovery, no vars files) and returns the
// rendered output, failing the test on any error.
func render(t *testing.T, varsInline, tmplSrc string) string {
	t.Helper()
	filter, err := NewTemplateFilter(nil, varsInline, nil)
	require.NoError(t, err, "NewTemplateFilter")
	out, err := filter([]byte(tmplSrc))
	require.NoError(t, err, "render %q", tmplSrc)
	return string(out)
}

// --- custom functions, called directly -------------------------------------

func TestMkSlice(t *testing.T) {
	got := mkSlice("a", "b", 3)
	assert.Equal(t, []any{"a", "b", 3}, got)
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token.txt"
	require.NoError(t, os.WriteFile(path, []byte("  secret-token\n"), 0o644))

	got, err := readFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "secret-token", got)
}

func TestReadFile_MissingFileErrors(t *testing.T) {
	_, err := readFile("/does/not/exist/token.txt")
	assert.Error(t, err)
}

func TestGetEnv(t *testing.T) {
	t.Setenv("SYVER_TEST_TEMPLATE_VAR", "present")
	assert.Equal(t, "present", getEnv("SYVER_TEST_TEMPLATE_VAR"))
}

func TestGetEnv_MissingUsesDefault(t *testing.T) {
	t.Setenv("SYVER_TEST_TEMPLATE_VAR_UNSET", "")
	assert.Equal(t, "fallback", getEnv("SYVER_TEST_TEMPLATE_VAR_UNSET", "fallback"))
}

func TestGetEnv_MissingNoDefault(t *testing.T) {
	assert.Equal(t, "", getEnv("SYVER_TEST_TEMPLATE_VAR_TRULY_UNSET"))
}

func TestRegexMatch(t *testing.T) {
	ok, err := regexMatch("^[a-z]+$", "hello")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = regexMatch("^[a-z]+$", "HELLO")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestRegexMatch_InvalidPatternErrors(t *testing.T) {
	_, err := regexMatch("(unterminated", "hello")
	assert.Error(t, err)
}

func TestFindStringSubmatch_NamedSubexpressions(t *testing.T) {
	pattern := `(?P<login>[a-z0-9]+):(?P<password>[a-z0-9]+)`
	got := findStringSubmatch(pattern, "bob:secret")
	assert.Equal(t, map[string]interface{}{"login": "bob", "password": "secret"}, got)
}

func TestFindStringSubmatch_NumberedSubexpressions(t *testing.T) {
	pattern := `([a-z0-9]+):([a-z0-9]+)`
	got := findStringSubmatch(pattern, "bob:secret")
	// index "0" is the whole match; "1" and "2" are the two groups. This is
	// the stringified-key behaviour the template.go:93 comment documents:
	// `get` needs string keys to look these up from a gossfile.
	assert.Equal(t, map[string]interface{}{
		"0": "bob:secret",
		"1": "bob",
		"2": "secret",
	}, got)
}

// --- the two library functions the fixtures actually use -------------------

func TestUpperAndRepeat_MatchTheShippedFixture(t *testing.T) {
	// The exact expression from docs/goss.yaml and
	// integration-tests/syver/goss-shared.yaml (sping_basic).
	got := render(t, "", `{{ "hello!" | upper | repeat 5 }}`)
	assert.Equal(t, "HELLO!HELLO!HELLO!HELLO!HELLO!", got)
}

// --- funcMap overrides sprout on collision ----------------------------------

// TestFuncMapWinsOnCollision proves the property template.go:41 depends on:
// syver's own funcMap, applied via .Funcs() AFTER sprigin.TxtFuncMap(), wins
// on a name collision. It does not use the package's real funcMap, because
// syver's real toUpper/toLower happen to be byte-identical to sprout's own
// (both are strings.ToUpper/ToLower) -- rendering through the real pipeline
// would pass whether the override worked or not, which is exactly the
// "generic overriding-works assertion" the spec warns would pass vacuously.
// Instead this reconstructs the identical two-call chain from template.go:41
// with a deliberately distinguishable stand-in for "toUpper", so the test can
// only pass if .Funcs(funcMap) actually wins the collision.
//
// Revert-proved by swapping the two .Funcs() calls below (base map applied
// last instead of the marker map): the test failed with the base sprout
// output instead of the marker, then was restored to the order below.
func TestFuncMapWinsOnCollision(t *testing.T) {
	marker := template.FuncMap{
		"toUpper": func(s string) string { return "OVERRIDDEN:" + s },
	}

	tmpl, err := template.New("t").Funcs(sprigin.TxtFuncMap()).Funcs(marker).Parse(`{{ "hello" | toUpper }}`)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, tmpl.Execute(&buf, nil))
	assert.Equal(t, "OVERRIDDEN:hello", buf.String())
}

// TestFuncMapDefinesToUpperAndToLower pins that syver's own funcMap still
// carries "toUpper" and "toLower" as strings.ToUpper/strings.ToLower. Sprout
// added its own toUpper/toLower/toupper/tolower (see FEAT-009 spec), which
// makes these two names the actual collisions template.go:41's ordering
// protects -- a generic "some function named toUpper exists somewhere" check
// would not catch funcMap losing these entries. Revert-proved by deleting
// the "toUpper" entry from funcMap: the test failed on the pointer
// comparison, then the entry was restored.
func TestFuncMapDefinesToUpperAndToLower(t *testing.T) {
	upperFn, ok := funcMap["toUpper"]
	require.True(t, ok, "funcMap must define toUpper")
	assert.Equal(t,
		reflect.ValueOf(strings.ToUpper).Pointer(),
		reflect.ValueOf(upperFn).Pointer(),
		"funcMap[\"toUpper\"] must still be strings.ToUpper",
	)

	lowerFn, ok := funcMap["toLower"]
	require.True(t, ok, "funcMap must define toLower")
	assert.Equal(t,
		reflect.ValueOf(strings.ToLower).Pointer(),
		reflect.ValueOf(lowerFn).Pointer(),
		"funcMap[\"toLower\"] must still be strings.ToLower",
	)
}

// TestFuncMapOrderingIsLoadBearing pins the ONE property template.go:41-51
// claims and that nothing else here actually protects: that the real pipeline
// applies syver's funcMap AFTER sprigin.TxtFuncMap(), so ours wins.
//
// TestFuncMapWinsOnCollision above cannot cover this. It rebuilds the two-call
// chain inside the test with its own marker map, so it passes no matter what
// order template.go uses -- verified by swapping the two .Funcs() calls in
// template.go and running the whole package: everything still passed. That is
// the gap this test closes.
//
// The distinguishing input is a NON-STRING argument, because for a string the
// two implementations agree byte for byte and any assertion is vacuous.
// Measured against sprout v1.1.1:
//
//	sprout-then-ours (correct):  {{ toUpper 42 }} -> exec error "expected string; found 42"
//	ours-then-sprout (reversed): {{ toUpper 42 }} -> renders "<no value>", no error
//
// So the reversed order turns a check that loudly refuses to run into one that
// silently asserts nothing, which for a compliance tool is the worse of the two
// by a wide margin. Asserting the ERROR is what pins the ordering; asserting
// the uppercased output never could.
//
// Revert-proved: with the .Funcs() calls in template.go swapped, this test
// fails on the require.Error; restored, it passes.
func TestFuncMapOrderingIsLoadBearing(t *testing.T) {
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)

	_, err = filter([]byte(`{{ toUpper 42 }}`))
	require.Error(t, err, "toUpper on a non-string must fail the run, not render empty")
	assert.Contains(t, err.Error(), "expected string",
		"the error must name the type problem, so a gossfile author can see why")

	// The string case still works, so the override is not merely breaking things.
	assert.Equal(t, "HI", render(t, "", `{{ toUpper "hi" }}`))
}

// --- missingkey behaviour: NewTemplateFilter (error) vs NewPeekTemplateFilter (zero) ---

func TestNewTemplateFilter_MissingKeyErrors(t *testing.T) {
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)

	_, err = filter([]byte(`{{ .Vars.doesNotExist }}`))
	assert.Error(t, err, "missingkey=error should fail on an absent Vars key")
}

func TestNewPeekTemplateFilter_MissingKeyIsZero(t *testing.T) {
	filter, err := NewPeekTemplateFilter(nil, "")
	require.NoError(t, err)

	out, err := filter([]byte(`{{ .Discovered.doesNotExist }}`))
	assert.NoError(t, err, "missingkey=zero must not fail on an absent Discovered key")
	assert.Equal(t, "<no value>", string(out))
}

// --- the five sprig-bug-fixes sprout corrects, pinned so a future sprout ---
// --- bump cannot quietly regress them ---------------------------------------

func TestSproutCorrectedExpressions(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want string
	}{
		// sprig gave "foo__bar" (double underscore); sprout gives "foo_bar".
		{"snakecase", `{{ "foo  bar" | snakecase }}`, "foo_bar"},
		// sprig gave "Foo Bar" (space kept, both capitalised); sprout gives "FooBar".
		{"camelcase", `{{ "foo  bar" | camelcase }}`, "FooBar"},
		// sprig gave "foo--bar" (double hyphen); sprout gives "foo-bar".
		{"kebabcase", `{{ "foo  bar" | kebabcase }}`, "foo-bar"},
		// sprig raised an exec error for a mismatched plural call; sprout
		// resolves it to the zero value instead of erroring.
		{"plural", `{{ plural 1 "a" "b" }}`, "<no value>"},
		// sprig raised an exec error for reverse on a string; sprout returns
		// an (unhelpful but non-erroring) empty slice representation.
		{"reverse", `{{ "hello" | reverse }}`, "[]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, render(t, "", tc.expr))
		})
	}
}

// --- the notice-silence measurement the whole release scope rests on -------

// TestUpperEmitsNoDeprecationNotice pins the measurement Deviation 1 is built
// on: sprout registers a deprecation notice for the sprig-compat alias
// "upper", but an ordering quirk in sprigin.SprigHandler.Build() (the
// rename-alias loop that appends "upper"'s notice runs AFTER
// sprout.AssignNotices wraps functions to actually emit) means that notice is
// never wired to fire at call time -- Notices() reports it, nothing consults
// it. This is why FEAT-009 shipped with NO logger suppression: syver's only
// deprecated usage (upper, in docs/goss.yaml and
// integration-tests/syver/goss-shared.yaml) is silent.
//
// This is also a KNOWN, NAMED UPSTREAM GAP, not an assumption the test takes
// on faith: sprout's own migration-from-sprig.md states these aliases "will
// log deprecation warnings", which is false for "upper" in the shipped
// v1.1.1 code. If a future sprout release "fixes" that ordering, this test
// starts failing -- that failure is a real signal to re-open the "syver
// lint" bundling decision Deviation 1 withdrew, not a bug to silence away.
//
// Rendered TWICE, mimicking the peek-then-load double decode
// (loadSyverConfig's quietDecode path), per the spec: two renders produced
// zero notices in measurement, so there is no double-fire risk to guard
// against here the way there is for the top-level-key guard's warnings.
//
// Revert-proved two ways while writing this test: (1) temporarily rendering
// `{{ "abc" | title }}` instead (a name that DOES warn via an unconditional
// inline call, unlike upper) confirmed the slog capture below actually
// detects output when it is present, ruling out a broken/no-op capture
// mechanism; (2) with the real "upper" expression restored, the buffer was
// confirmed empty on both renders.
func TestUpperEmitsNoDeprecationNotice(t *testing.T) {
	var buf bytes.Buffer
	origDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)

	for i := 1; i <= 2; i++ {
		buf.Reset()
		out, err := filter([]byte(`{{ "hello!" | upper | repeat 5 }}`))
		require.NoError(t, err)
		assert.Equal(t, "HELLO!HELLO!HELLO!HELLO!HELLO!", string(out))
		assert.Empty(t, buf.String(), "render #%d of a deprecated-but-silent \"upper\" call must emit nothing", i)
	}
}

// --- glob ------------------------------------------------------------------

// resetGlobWarned clears the process-level "already warned" set so each test
// starts from a fresh process's state, and again afterwards so a test cannot
// leak a recorded pattern into the next one.
func resetGlobWarned(t *testing.T) {
	t.Helper()
	clear := func() {
		globWarned.Lock()
		globWarned.seen = map[string]bool{}
		globWarned.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// cliDefaultLogLevel is the CLI's --log-level default. util.NewConfig defaults
// to ERROR, which would hide every WARN these tests look for.
const cliDefaultLogLevel = "INFO"

// globFixture creates dir/{a.conf,b.conf,c.txt} and returns dir.
func globFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"b.conf", "a.conf", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte(n), 0o644))
	}
	return dir
}

// q quotes a path as a template string literal, escaping the backslashes a
// Windows path contains.
func q(s string) string { return strconv.Quote(s) }

func TestGlob_MergesDedupesAndSorts(t *testing.T) {
	resetGlobWarned(t)
	dir := globFixture(t)
	// The second pattern repeats a.conf as a plain path and the third adds
	// c.txt: a.conf must appear once, and the result must be sorted across
	// patterns rather than concatenated in argument order.
	src := `{{ range glob ` + q(filepath.Join(dir, "*.txt")) + " " + q(filepath.Join(dir, "*.conf")) +
		" " + q(filepath.Join(dir, "a.conf")) + ` }}{{ . }}|{{ end }}`
	got := render(t, "", src)
	want := filepath.Join(dir, "a.conf") + "|" + filepath.Join(dir, "b.conf") + "|" + filepath.Join(dir, "c.txt") + "|"
	assert.Equal(t, want, got)
}

// TestGlob_DedupesAcrossSpellings: a literal path spelled with a doubled
// separator and the same file found through a wildcard are one path, and the
// result is the cleaned spelling.
func TestGlob_DedupesAcrossSpellings(t *testing.T) {
	resetGlobWarned(t)
	dir := globFixture(t)
	doubled := dir + string(filepath.Separator) + string(filepath.Separator) + "c.txt"
	src := `{{ range glob ` + q(doubled) + " " + q(filepath.Join(dir, "*.txt")) + ` }}{{ . }}|{{ end }}`
	assert.Equal(t, filepath.Join(dir, "c.txt")+"|", render(t, "", src))
}

func TestGlob_NoPatternsErrors(t *testing.T) {
	_, err := templateGlob()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one pattern")

	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)
	_, err = filter([]byte(`{{ range glob }}{{ . }}{{ end }}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one pattern")
}

func TestGlob_MalformedPatternFailsNamingIt(t *testing.T) {
	resetGlobWarned(t)
	dir := globFixture(t)
	bad := filepath.Join(dir, "[ab")
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)
	// A good pattern before the bad one must not rescue the render.
	_, err = filter([]byte(`{{ range glob ` + q(filepath.Join(dir, "*")) + " " + q(bad) + ` }}{{ . }}{{ end }}`))
	require.Error(t, err)
	assert.ErrorIs(t, err, filepath.ErrBadPattern)
	assert.Contains(t, err.Error(), `error in expanding glob pattern `+q(bad))
}

// TestGlob_NoMatchWarnsOncePerPatternAndRenders checks the WARN goes through
// the leveled logger in the RFC3339 format, names each pattern once even when
// the pattern is evaluated twice, and that the render still succeeds.
func TestGlob_NoMatchWarnsOncePerPatternAndRenders(t *testing.T) {
	resetGlobWarned(t)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	dir := globFixture(t)
	missingGlob := filepath.Join(dir, "*.nope")
	missingPlain := filepath.Join(dir, "absent")

	var buf bytes.Buffer
	cfg, err := util.NewConfig()
	require.NoError(t, err)
	cfg.LogLevel = cliDefaultLogLevel
	require.NoError(t, setLogLevelTo(cfg, &buf))

	src := `{{ range glob ` + q(missingGlob) + " " + q(missingPlain) + ` }}{{ . }}{{ end }}` +
		`{{ range glob ` + q(missingGlob) + " " + q(filepath.Join(dir, "a.conf")) + ` }}{{ . }}{{ end }}`
	got := render(t, "", src)
	assert.Equal(t, filepath.Join(dir, "a.conf"), got)

	out := buf.String()
	assert.Equal(t, 2, strings.Count(out, "[WARN]"), out)
	assert.Equal(t, 1, strings.Count(out, q(missingGlob)), out)
	assert.Equal(t, 1, strings.Count(out, q(missingPlain)), out)
	assert.Regexp(t, `(?m)^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ \[WARN\] glob pattern ".*" matched no files$`, out)
}

func TestGlob_NoMatchWarnRespectsLogLevel(t *testing.T) {
	resetGlobWarned(t)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	var buf bytes.Buffer
	cfg, err := util.NewConfig()
	require.NoError(t, err)
	cfg.LogLevel = "ERROR"
	require.NoError(t, setLogLevelTo(cfg, &buf))

	render(t, "", `{{ range glob `+q(filepath.Join(t.TempDir(), "*"))+` }}{{ end }}`)
	assert.Empty(t, buf.String())
}

// globSpec writes a spec whose only content is a glob of a missing pattern and
// returns the spec path and the pattern.
func globSpec(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	missing := filepath.Join(dir, "*.nope")
	spec := filepath.Join(dir, "syver.yaml")
	content := `{{ range glob ` + q(missing) + ` }}{{ end }}
file:
  ` + q(dir) + `:
    exists: true
`
	require.NoError(t, os.WriteFile(spec, []byte(content), 0o644))
	return spec, missing
}

// TestGlob_ValidateWarnsExactlyOnce goes through Validate, which renders the
// spec twice (peek, then the real load). The WARN must appear once.
func TestGlob_ValidateWarnsExactlyOnce(t *testing.T) {
	resetGlobWarned(t)
	spec, missing := globSpec(t)
	var results bytes.Buffer
	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor(), util.WithResultWriter(&results))
	require.NoError(t, err)
	cfg.LogLevel = cliDefaultLogLevel

	var code int
	_, stderr := captureStdoutStderr(t, func() { code, err = Validate(t.Context(), cfg) })
	require.NoError(t, err)
	require.Equal(t, 0, code, results.String())
	assert.Equal(t, 1, strings.Count(stderr, "[WARN] glob pattern "+q(missing)+" matched no files"), stderr)
}

// TestGlob_PeekOnlyGlobDoesNotWarn proves the peek render is silent rather than
// merely de-duplicated: a glob reached only in the peek pass (where
// .Discovered is empty) and not in the real load must not warn at all.
func TestGlob_PeekOnlyGlobDoesNotWarn(t *testing.T) {
	resetGlobWarned(t)
	dir := globFixture(t)
	peekOnly := filepath.Join(dir, "*.peek-only")
	spec := filepath.Join(dir, "syver.yaml")
	content := `discovery:
  file:
    ` + q(filepath.Join(dir, "a.conf")) + `:
      register: present
      exists: true
{{ if not .Discovered.present }}{{ range glob ` + q(peekOnly) + ` }}{{ end }}{{ end }}
file:
  ` + q(dir) + `:
    exists: true
`
	require.NoError(t, os.WriteFile(spec, []byte(content), 0o644))
	var results bytes.Buffer
	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor(), util.WithResultWriter(&results))
	require.NoError(t, err)
	cfg.LogLevel = cliDefaultLogLevel

	var code int
	_, stderr := captureStdoutStderr(t, func() { code, err = Validate(t.Context(), cfg) })
	require.NoError(t, err)
	require.Equal(t, 0, code, results.String())
	assert.NotContains(t, stderr, "glob pattern", stderr)
}

func TestGlob_RenderWarnsExactlyOnce(t *testing.T) {
	resetGlobWarned(t)
	spec, missing := globSpec(t)
	cfg, err := util.NewConfig(util.WithSpecFile(spec))
	require.NoError(t, err)
	cfg.LogLevel = cliDefaultLogLevel

	var renderErr error
	_, stderr := captureStdoutStderr(t, func() { _, renderErr = RenderJSON(cfg) })
	require.NoError(t, renderErr)
	assert.Equal(t, 1, strings.Count(stderr, "[WARN] glob pattern "+q(missing)+" matched no files"), stderr)
}

// TestGlob_ServeWarnsOnceAtStartup: serve renders the spec once, when the
// health handler is built, and not per request.
func TestGlob_ServeWarnsOnceAtStartup(t *testing.T) {
	resetGlobWarned(t)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	spec, missing := globSpec(t)
	var buf bytes.Buffer
	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithOutputFormat("json"))
	require.NoError(t, err)
	cfg.LogLevel = cliDefaultLogLevel
	require.NoError(t, setLogLevelTo(cfg, &buf))

	hh, err := newHealthHandler(t.Context(), cfg)
	require.NoError(t, err)
	for range 2 {
		rr := httptest.NewRecorder()
		http.HandlerFunc(hh.ServeHTTP).ServeHTTP(rr, makeRequest(t, cfg, nil))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	}
	assert.Equal(t, 1, strings.Count(buf.String(), "[WARN] glob pattern "+q(missing)+" matched no files"), buf.String())
}
