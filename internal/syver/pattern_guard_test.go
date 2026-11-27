package syver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/krameff/syver/matchers"
	"github.com/krameff/syver/util"
)

// captureStderr points os.Stderr at a temp file until the test ends. The
// commands under test call setLogLevel, which sends the log to whatever
// os.Stderr is at that moment, so a log.SetOutput made before the call would
// be replaced. The returned func reads what has been written so far.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = orig
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		f.Close()
	})
	return func() string {
		b, err := os.ReadFile(f.Name())
		require.NoError(t, err)
		return string(b)
	}
}

// warnConfig is a config whose log level lets a WARN through.
func warnConfig(t *testing.T, opts ...util.ConfigOption) *util.Config {
	t.Helper()
	c, err := util.NewConfig(append([]util.ConfigOption{util.WithOutputFormat("json")}, opts...)...)
	require.NoError(t, err)
	c.LogLevel = "WARN"
	c.OutputWriter = io.Discard
	return c
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

const (
	// sanityKey is an unknown top-level key: it must log exactly once per
	// command, which proves the log capture works before a count is trusted.
	sanityKey  = "bogus-sanity-key"
	sanityLine = `unknown top-level key "` + sanityKey + `"`

	mainBroken = `!/^broken-main`
	discBroken = `!/^broken-discover`
	inclBroken = `!/^broken-include`
)

// warnLine is the text that identifies the warning for pattern p.
func warnLine(p string) string { return "`" + p + "` is matched as plain text" }

// mainSpec is a spec holding one broken pattern, in a check that passes
// (the inverse text never appears in "foo bar").
func mainSpec(extra string) string {
	return sanityKey + ": 1\n" +
		"matching:\n  check:\n    content: foo bar\n    matches:\n      have-patterns: ['" + mainBroken + "']\n" +
		extra
}

const discoveryBlock = "discovery:\n  matching:\n    found:\n      register: found\n      content: x\n      matches: x\n"

// TestPatternGuard_ExactlyOncePerCommand is the once-per-command contract: a
// validate run decodes the spec twice (peek, then the real load), so the
// warning must come from the real load only.
func TestPatternGuard_ExactlyOncePerCommand(t *testing.T) {
	t.Run("validate with a discovery block", func(t *testing.T) {
		spec := filepath.Join(t.TempDir(), "syver.yaml")
		writeFile(t, spec, mainSpec(discoveryBlock))
		read := captureStderr(t)

		code, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec)))
		require.NoError(t, err)
		assert.Equal(t, 0, code)

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
		assert.Contains(t, out, spec+": matching `check` matches have-patterns: ")
	})

	t.Run("validate without a discovery block", func(t *testing.T) {
		spec := filepath.Join(t.TempDir(), "syver.yaml")
		writeFile(t, spec, mainSpec(""))
		read := captureStderr(t)

		code, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec)))
		require.NoError(t, err)
		assert.Equal(t, 0, code)

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
	})

	t.Run("validate with --discover", func(t *testing.T) {
		dir := t.TempDir()
		spec := filepath.Join(dir, "main.yaml")
		discover := filepath.Join(dir, "discover.yaml")
		writeFile(t, spec, mainSpec(""))
		writeFile(t, discover, "discovery:\n  matching:\n    found:\n      register: found\n      content: x\n"+
			"      matches:\n        not:\n          have-patterns: ['"+discBroken+"']\n")
		read := captureStderr(t)

		code, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec), util.WithDiscoverFile(discover)))
		require.NoError(t, err)
		assert.Equal(t, 0, code)

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(discBroken)), out)
		assert.Contains(t, out, spec+": matching `check` matches have-patterns: "+"`"+mainBroken+"`")
		assert.Contains(t, out, discover+": matching `found` matches have-patterns: "+"`"+discBroken+"`")
	})

	t.Run("validate with retries", func(t *testing.T) {
		spec := filepath.Join(t.TempDir(), "syver.yaml")
		writeFile(t, spec, mainSpec("  fails:\n    content: a\n    matches: b\n"))
		read := captureStderr(t)

		var results bytes.Buffer
		c := warnConfig(t, util.WithSpecFile(spec))
		c.OutputWriter = &results
		c.RetryTimeout = 400 * time.Millisecond
		c.Sleep = 50 * time.Millisecond
		code, err := Validate(t.Context(), c)
		require.Error(t, err, "the always-failing check must time out")
		assert.Equal(t, 3, code)
		require.GreaterOrEqual(t, strings.Count(results.String(), `"summary"`), 2, "it must have attempted more than once")

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
	})

	for _, noDiscover := range []bool{false, true} {
		t.Run(fmt.Sprintf("render no-discover=%v", noDiscover), func(t *testing.T) {
			spec := filepath.Join(t.TempDir(), "syver.yaml")
			writeFile(t, spec, mainSpec(discoveryBlock))
			read := captureStderr(t)

			c := warnConfig(t, util.WithSpecFile(spec))
			c.NoDiscover = noDiscover
			_, err := RenderJSON(t.Context(), c)
			require.NoError(t, err)

			out := read()
			require.Equal(t, 1, strings.Count(out, sanityLine), out)
			assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
		})
	}

	t.Run("add", func(t *testing.T) {
		dir := t.TempDir()
		spec := filepath.Join(dir, "syver.yaml")
		present := filepath.Join(dir, "present")
		writeFile(t, present, "x")
		writeFile(t, spec, mainSpec(""))
		read := captureStderr(t)

		require.NoError(t, AddResources(spec, "File", []string{present}, warnConfig(t)))

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
	})

	t.Run("autoadd", func(t *testing.T) {
		skipAutoAddOffLinux(t)
		dir := t.TempDir()
		spec := filepath.Join(dir, "syver.yaml")
		present := filepath.Join(dir, "present")
		writeFile(t, present, "x")
		writeFile(t, spec, mainSpec(""))
		read := captureStderr(t)

		require.NoError(t, AutoAddResources(spec, []string{present}, warnConfig(t)))

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
	})

	t.Run("included gossfile", func(t *testing.T) {
		dir := t.TempDir()
		spec := filepath.Join(dir, "syver.yaml")
		child := filepath.Join(dir, "child.yaml")
		writeFile(t, spec, mainSpec("gossfile:\n  child.yaml: {}\n"))
		writeFile(t, child, "command:\n  echo:\n    exit-status: 0\n    stdout: ['"+inclBroken+"']\n    skip: true\n")
		read := captureStderr(t)

		code, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec)))
		require.NoError(t, err)
		assert.Equal(t, 0, code)

		out := read()
		require.Equal(t, 1, strings.Count(out, sanityLine), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
		assert.Equal(t, 1, strings.Count(out, warnLine(inclBroken)), out)
		assert.Contains(t, out, child+": command `echo` stdout: `"+inclBroken+"`")
	})
}

// TestPatternGuard_ServeWarnsAtStartOnly: with the cache off every request
// runs the checks, so a warning raised while matching would appear per
// request. It must appear once, when the server loads the spec.
func TestPatternGuard_ServeWarnsAtStartOnly(t *testing.T) {
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	spec := filepath.Join(t.TempDir(), "syver.yaml")
	writeFile(t, spec, mainSpec(""))

	var logOutput bytes.Buffer
	c := warnConfig(t, util.WithSpecFile(spec), util.WithCache(0))
	require.NoError(t, setLogLevelTo(c, &logOutput))

	hh, err := newHealthHandler(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(logOutput.String(), sanityLine), logOutput.String())
	assert.Equal(t, 1, strings.Count(logOutput.String(), warnLine(mainBroken)), "at start")

	for i := range 3 {
		rr := httptest.NewRecorder()
		http.HandlerFunc(hh.ServeHTTP).ServeHTTP(rr, makeRequest(t, c, nil))
		require.Equal(t, http.StatusOK, rr.Code, "request %d", i)
	}
	assert.Equal(t, 1, strings.Count(logOutput.String(), warnLine(mainBroken)), "after three requests")
}

// TestPatternGuard_LogLevelFiltersIt: the warning goes through the log, so
// -L ERROR hides it.
func TestPatternGuard_LogLevelFiltersIt(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  int
	}{{"ERROR", 0}, {"WARN", 1}} {
		t.Run(tc.level, func(t *testing.T) {
			spec := filepath.Join(t.TempDir(), "syver.yaml")
			writeFile(t, spec, mainSpec(""))
			read := captureStderr(t)

			c := warnConfig(t, util.WithSpecFile(spec))
			c.LogLevel = tc.level
			_, err := Validate(t.Context(), c)
			require.NoError(t, err)

			out := read()
			require.Equal(t, tc.want, strings.Count(out, sanityLine), out)
			assert.Equal(t, tc.want, strings.Count(out, warnLine(mainBroken)), out)
		})
	}
}

// normaliseJSONResult drops what differs between two identical runs: times,
// durations and the summary line that prints the duration.
func normaliseJSONResult(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal(b, &v), string(b))
	durationRe := regexp.MustCompile(`Duration: [0-9.]+s`)
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range []string{"duration", "total-duration", "start-time", "end-time"} {
				delete(x, k)
			}
			for k, vv := range x {
				x[k] = walk(vv)
			}
		case []any:
			for i, vv := range x {
				x[i] = walk(vv)
			}
		case string:
			return durationRe.ReplaceAllString(x, "Duration: X")
		}
		return v
	}
	out, err := json.Marshal(walk(v))
	require.NoError(t, err)
	return string(out)
}

// TestPatternGuard_ChangesNoResult holds the spec constant and runs it with
// the guard and with the guard stubbed out. As text "/^foo" does not match
// "foo bar"; read as a regex it would, so any rewrite of the pattern by the
// guard flips the result.
func TestPatternGuard_ChangesNoResult(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	writeFile(t, target, "foo bar\n")
	spec := filepath.Join(dir, "syver.yaml")
	writeFile(t, spec, "file:\n  target:\n    path: '"+filepath.ToSlash(target)+"'\n    exists: true\n    contents: ['/^foo']\n")

	run := func() (int, string) {
		var results bytes.Buffer
		c := warnConfig(t, util.WithSpecFile(spec))
		c.OutputWriter = &results
		code, err := Validate(t.Context(), c)
		require.NoError(t, err)
		assert.NotContains(t, results.String(), "[WARN]", "the warning must not reach the results")
		return code, normaliseJSONResult(t, results.Bytes())
	}

	read := captureStderr(t)
	guardedCode, guarded := run()
	require.Equal(t, 1, strings.Count(read(), warnLine(`/^foo`)), "the guard must have run")

	orig := checkPatternsFn
	checkPatternsFn = func(*SyverConfig, string) []patternWarning { return nil }
	t.Cleanup(func() { checkPatternsFn = orig })
	stubbedCode, stubbed := run()

	assert.Equal(t, stubbedCode, guardedCode)
	assert.Equal(t, stubbed, guarded)
	assert.Equal(t, 1, guardedCode, "the text pattern must fail")
	assert.Contains(t, guarded, `"failed-count":1`)
}

// TestPatternGuard_JSONSpec: a JSON spec is checked the same way.
func TestPatternGuard_JSONSpec(t *testing.T) {
	spec := filepath.Join(t.TempDir(), "syver.json")
	writeFile(t, spec, `{"`+sanityKey+`": 1, "matching": {"check": {"content": "foo bar", "matches": {"have-patterns": ["`+mainBroken+`"]}}}}`)
	read := captureStderr(t)

	code, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec)))
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	out := read()
	assert.Equal(t, 1, strings.Count(out, warnLine(mainBroken)), out)
	assert.Contains(t, out, spec+": matching `check` matches have-patterns: `"+mainBroken+"`")
}

// warnLines returns the [WARN] lines of out about patterns, without the
// timestamp in front of each.
func patternWarnLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "[WARN] "); i >= 0 && strings.Contains(l, "is matched as plain text") {
			lines = append(lines, l[i:])
		}
	}
	return lines
}

// TestPatternGuard_CRLFSpec: a spec with CRLF line ends (a Windows checkout)
// gives the same warning, byte for byte.
func TestPatternGuard_CRLFSpec(t *testing.T) {
	dir := t.TempDir()
	lf := filepath.Join(dir, "syver.yaml")
	crlf := filepath.Join(dir, "crlf", "syver.yaml")
	require.NoError(t, os.Mkdir(filepath.Dir(crlf), 0o755))
	writeFile(t, lf, mainSpec(""))
	writeFile(t, crlf, strings.ReplaceAll(mainSpec(""), "\n", "\r\n"))

	runOn := func(spec string) []string {
		read := captureStderr(t)
		_, err := Validate(t.Context(), warnConfig(t, util.WithSpecFile(spec)))
		require.NoError(t, err)
		return patternWarnLines(read())
	}
	lfLines := runOn(lf)
	crlfLines := runOn(crlf)
	require.Len(t, lfLines, 1)
	require.Len(t, crlfLines, 1)
	assert.Equal(t, strings.Replace(lfLines[0], lf, crlf, 1), crlfLines[0])
}

// TestPatternWarning_String pins both wordings, the omitted path, and the
// fallback to a quoted string for content that is not safe to print.
func TestPatternWarning_String(t *testing.T) {
	w := patternWarning{Path: "syver.yaml", Type: "file", ID: "/etc/issue", Attr: "contents", Pattern: `!/[Ll]inux`}
	assert.Equal(t, "syver.yaml: file `/etc/issue` contents: `!/[Ll]inux` is matched as plain text: "+
		`it starts like a regex but has no closing "/". To make it a regex, end it with "/" `+
		"(optionally followed by i, m or s); to keep it as text, write `!\\/[Ll]inux`, which also silences this warning",
		w.String())

	p := `/^password\s+requisite\s+pam_pwhistory.so remember=5 retry=3/'`
	w = patternWarning{Path: "syver.yaml", Type: "file", ID: "/etc/pam.d/system-auth", Attr: "contents", Pattern: p, Trailing: "'"}
	assert.Equal(t, "syver.yaml: file `/etc/pam.d/system-auth` contents: `"+p+"` is matched as plain text: "+
		`it starts like a regex but what follows its last "/" (`+"`'`"+`) is not i, m or s flags. `+
		`To make it a regex, end it with "/" (optionally followed by i, m or s); to keep it as text, write `+
		"`\\"+p+"`, which also silences this warning",
		w.String())

	w = patternWarning{Type: "command", ID: "echo", Attr: "stdout", Pattern: "/^a\x1b[31m"}
	got := w.String()
	assert.True(t, strings.HasPrefix(got, "command `echo` stdout: \"/^a\\x1b[31m\" is matched"), got)
	assert.Contains(t, got, `write "\\/^a\x1b[31m"`)
	assert.NotContains(t, got, "\x1b", "a control character must not reach the terminal")

	w = patternWarning{Type: "command", ID: "has`tick", Attr: "stdout", Pattern: "/^x`y"}
	assert.True(t, strings.HasPrefix(w.String(), "command \"has`tick\" stdout: \"/^x`y\" is matched"), w.String())
}

// roundTripPatterns are flagged patterns with backslashes, quotes, braces
// and a trailing "/" plus junk: the shapes a copied escape must survive.
var roundTripPatterns = []string{
	`!/[Ll]inux`,
	`!/^\s*etm\b`,
	`/^password\s+requisite\s+pam_pwhistory.so remember=5 retry=3/'`,
	`/\s\/var\/log\/audit\s.*noexec`,
	`/^.*.conf\:\*\.\* {{ .Vars.x }}\[{{ .Vars.y }}\]`,
	`/^\s*password_pbkdf2.*\s{{ .Vars.hash }}\`,
	`!/^cipher@/(lib|open)ssh(-server|-client)?\s*=\s*([^#\n\r]+)?-CBC\b`,
	`/^x/ `,
	`/^Policy:.*:\s\b([1-9]|10)\b`,
}

var (
	printedPatternRe = regexp.MustCompile(`^file ` + "`id`" + ` contents: (.*) is matched as plain text: `)
	printedEscapeRe  = regexp.MustCompile(`; to keep it as text, write (.*), which also silences this warning$`)
)

// unBacktick strips the backticks a value is printed between; anything else
// is returned unchanged, so a quoted value fails the round trip.
func unBacktick(s string) string {
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1]
	}
	return s
}

// singleQuotedYAML decodes s as the value of a single-quoted YAML scalar.
func singleQuotedYAML(t *testing.T, s string) string {
	t.Helper()
	var v map[string]string
	require.NoError(t, yaml.Unmarshal([]byte("k: '"+strings.ReplaceAll(s, "'", "''")+"'\n"), &v), s)
	return v["k"]
}

// TestPatternWarning_AdviceSurvivesSingleQuotedYAML: what the message prints,
// pasted into a single-quoted YAML scalar, is the pattern (for the pattern)
// and the escape (for the advice).
func TestPatternWarning_AdviceSurvivesSingleQuotedYAML(t *testing.T) {
	for _, p := range roundTripPatterns {
		trailing, ok := matchers.UnterminatedRegex(p)
		require.True(t, ok, p)
		msg := patternWarning{Type: "file", ID: "id", Attr: "contents", Pattern: p, Trailing: trailing}.String()

		m := printedPatternRe.FindStringSubmatch(msg)
		require.Len(t, m, 2, msg)
		assert.Equal(t, p, singleQuotedYAML(t, unBacktick(m[1])), "printed pattern of %s", p)

		m = printedEscapeRe.FindStringSubmatch(msg)
		require.Len(t, m, 2, msg)
		assert.Equal(t, matchers.EscapeAsText(p), singleQuotedYAML(t, unBacktick(m[1])), "printed escape of %s", p)
	}
}
