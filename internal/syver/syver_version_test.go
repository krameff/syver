package syver

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krameff/syver/util"
)

// A matching: block validates the same on every platform and touches nothing
// on the host, so it is the resource these tests use to keep a spec non-empty.
const okMatching = "matching:\n  ok:\n    content: 1\n    matches: 1\n"

func writeSpecIn(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// captureLog points the standard logger at a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// withVersion sets the running binary's stamp for the rest of the test.
func withVersion(t *testing.T, stamp string) {
	t.Helper()
	orig := util.Version
	util.Version = stamp
	t.Cleanup(func() { util.Version = orig })
}

// resetDevBuildNotice lets a test see the once-per-process DEBUG line.
func resetDevBuildNotice(t *testing.T) {
	t.Helper()
	devBuildSkipLogged.Store(false)
	t.Cleanup(func() { devBuildSkipLogged.Store(false) })
}

// T-2: one row per line of the accepted-value table.
func TestParseRequiredVersion(t *testing.T) {
	ok := map[string]string{
		"0.20.0":     "0.20.0",
		"v0.20.0":    "0.20.0",
		"0.20":       "0.20.0",
		"1":          "1.0.0",
		"0.20.0-rc1": "0.20.0",
		" 0.20.0 ":   "0.20.0",
	}
	for in, want := range ok {
		t.Run("ok "+in, func(t *testing.T) {
			v, err := parseRequiredVersion(in)
			require.NoError(t, err)
			assert.Equal(t, want, v.String())
		})
	}

	bad := map[string]string{
		"":                "needs a version",
		">=0.20.0":        "takes a single minimum version",
		"~0.20":           "takes a single minimum version",
		"0.20.0 - 0.21.0": "takes a single minimum version",
		"00.20":           "is not a version",
		"0.020.0":         "is not a version",
		"0.20.0.1":        "is not a version",
		"latest":          "is not a version",
		"true":            "is not a version",
		"0x14":            "is not a version",
	}
	for in, want := range bad {
		t.Run("bad "+in, func(t *testing.T) {
			_, err := parseRequiredVersion(in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), want)
		})
	}
}

// T-3: only a plain release stamp is compared; every development stamp skips
// with exactly one DEBUG line however often it is asked.
func TestCheckMinimumVersion(t *testing.T) {
	req := semver.MustParse("0.20.0")
	t.Run("release stamps compare", func(t *testing.T) {
		for stamp, wantErr := range map[string]bool{
			"0.19.0":  true,
			"v0.19.9": true,
			"0.20.0":  false,
			"v0.20.0": false,
			"0.21.0":  false,
			"1.0.0":   false,
		} {
			err := checkMinimumVersion(req, stamp)
			if wantErr {
				assert.Error(t, err, stamp)
				var old *versionTooOldError
				assert.ErrorAs(t, err, &old, stamp)
			} else {
				assert.NoError(t, err, stamp)
			}
		}
	})

	far := semver.MustParse("99.0.0")
	for _, stamp := range []string{"", "0.0.0", "v0.0.0", "v0.16.0-6-gabc1234", "0.16.1-next", "0.20.0-rc1", "0.20.0-SNAPSHOT-abc", "c103c3a"} {
		t.Run("dev "+stamp, func(t *testing.T) {
			resetDevBuildNotice(t)
			buf := captureLog(t)
			_, release := classifyRunningVersion(stamp)
			assert.False(t, release)
			assert.NoError(t, checkMinimumVersion(far, stamp))
			assert.NoError(t, checkMinimumVersion(far, stamp))
			assert.Equal(t, 1, strings.Count(buf.String(), "[DEBUG] syver-version check skipped: development build"),
				"one DEBUG line per process, not per file: %q", buf.String())
			assert.Contains(t, buf.String(), fmt.Sprintf("%q", stamp))
		})
	}
}

// T-4: a strict file keeps the anchor-carrier and x- patterns.
func TestStrict_AnchorCarrierAndXKeyValidateClean(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpecIn(t, dir, "syver.yaml", "syver-version: 0.20.0\n"+
		"common-checks: &common\n  exit-status: 0\n"+
		"x-shared:\n  anything: true\n"+
		"command:\n  echo one:\n    <<: *common\n"+okMatching)
	buf := captureLog(t)

	cfg, err := getSyverConfig(nil, "", spec, nil)
	require.NoError(t, err)
	assert.Contains(t, cfg.Commands, "echo one")
	assert.NotContains(t, buf.String(), "[WARN]")
}

// T-4b: a top-level merge of an alias, or of a sequence of aliases, is legal
// in both modes and its merged keys still decode. An inline map under << is
// still reported.
func TestTopLevelMergeKey(t *testing.T) {
	base := "x-a: &a\n  matching:\n    from a:\n      content: 1\n      matches: 1\n" +
		"x-b: &b\n  command:\n    echo from b:\n      exit-status: 0\n"
	for name, merge := range map[string]string{
		"alias":    "<<: *a\n",
		"sequence": "<<: [*a, *b]\n",
	} {
		for _, marker := range []string{"", "syver-version: 0.20.0\n"} {
			t.Run(fmt.Sprintf("%s strict=%v", name, marker != ""), func(t *testing.T) {
				dir := t.TempDir()
				spec := writeSpecIn(t, dir, "syver.yaml", marker+base+merge)
				buf := captureLog(t)
				cfg, err := getSyverConfig(nil, "", spec, nil)
				require.NoError(t, err)
				assert.Contains(t, cfg.Matchings, "from a", "the merged keys must still decode")
				if name == "sequence" {
					assert.Contains(t, cfg.Commands, "echo from b")
				}
				assert.NotContains(t, buf.String(), "unknown top-level key")
			})
		}
	}

	// The keys a merge brings in land at the top level, so a typo among them
	// is as unknown as one written there. REVERT-PROOF: skip the
	// mergedUnknownKeys call and the typo is dropped silently in both modes.
	t.Run("merged keys are checked", func(t *testing.T) {
		typo := "x-a: &a\n  prot:\n    tcp:22:\n      listening: true\n" +
			"x-n: &n\n  <<: *a\n  bogus-nested: 1\n" +
			"<<: *n\n" + okMatching
		w := checkTopLevelKeys([]byte(typo), "syver.yaml")
		if assert.Len(t, w, 2) {
			assert.Equal(t, "prot", w[0].Key, "a merge nested inside the merged block is followed")
			assert.Equal(t, 2, w[0].Line, "the line of the key inside the anchored block")
			assert.Equal(t, "port", w[0].Suggestion)
			assert.Equal(t, "bogus-nested", w[1].Key)
			assert.Equal(t, 7, w[1].Line)
		}

		dir := t.TempDir()
		buf := captureLog(t)
		_, err := getSyverConfig(nil, "", writeSpecIn(t, dir, "warn.yaml", typo), nil)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(buf.String(), `unknown top-level key "prot"`), "unmarked: one WARN")

		_, err = getSyverConfig(nil, "", writeSpecIn(t, dir, "strict.yaml", "syver-version: 0.20.0\n"+typo), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `strict.yaml:3: "prot" (did you mean "port"?)`)
	})

	t.Run("inline map is unknown", func(t *testing.T) {
		w := checkTopLevelKeys([]byte("<<: {bogus: 1}\n"+okMatching), "syver.yaml")
		if assert.Len(t, w, 1) {
			assert.Equal(t, "<<", w[0].Key)
		}
	})
}

// T-4c: the two documented gaps, pinned so changing either is deliberate.
func TestStrict_DocumentedAnchorGaps(t *testing.T) {
	dir := t.TempDir()
	typoWithAnchor := writeSpecIn(t, dir, "a.yaml", "syver-version: 0.20.0\nservce: &s\n  sshd:\n    running: true\n"+okMatching)
	_, err := getSyverConfig(nil, "", typoWithAnchor, nil)
	assert.NoError(t, err, "a typo'd key carrying its own anchor is exempt even in strict mode")

	nested := writeSpecIn(t, dir, "b.yaml", "syver-version: 0.20.0\nbogus:\n  nested:\n    deep: &d\n      a: 1\n"+okMatching)
	_, err = getSyverConfig(nil, "", nested, nil)
	require.Error(t, err, "an anchor nested below the key does not exempt it")
	assert.Contains(t, err.Error(), `"bogus"`)
	assert.Contains(t, err.Error(), "x-")
}

// T-5: every unknown key in a strict file comes back in one error, with
// lines and the suggestion.
func TestStrict_OneErrorNamesEveryKey(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpecIn(t, dir, "syver.yaml", "syver-version: 0.20.0\nprot:\n  tcp:22:\n    listening: true\n"+okMatching+"totally-made-up-key: 1\n")
	buf := captureLog(t)

	_, err := getSyverConfig(nil, "", spec, nil)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, spec+": syver-version is set, so unknown top-level keys are errors (prefix a key with x- to keep it):")
	assert.Contains(t, msg, spec+`:2: "prot" (did you mean "port"?)`)
	assert.Contains(t, msg, spec+`:9: "totally-made-up-key"`)
	assert.Less(t, strings.Index(msg, `"prot"`), strings.Index(msg, `"totally-made-up-key"`), "document order")
	assert.NotContains(t, buf.String(), "[WARN]", "a strict file's keys are an error, not also WARNs")
}

// T-6: the peek pass neither errors nor logs; the real load errors once.
func TestStrict_PeekThenReal(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpecIn(t, dir, "syver.yaml", "syver-version: 0.20.0\nprot: {}\n"+okMatching)
	buf := captureLog(t)

	_, err := getSyverConfigPeek(nil, "", spec)
	assert.NoError(t, err, "the peek pass must not error")
	assert.Empty(t, buf.String(), "the peek pass must not log")

	_, err = getSyverConfig(nil, "", spec, nil)
	require.Error(t, err)
	assert.Equal(t, 1, strings.Count(err.Error(), `"prot"`))
	assert.NotContains(t, buf.String(), "[WARN]")
}

// T-7: an unmarked file is unchanged: one WARN, no error.
func TestUnmarked_UnknownKeyWarnsOnce(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpecIn(t, dir, "syver.yaml", "prot: {}\n"+okMatching)
	buf := captureLog(t)

	_, err := getSyverConfigPeek(nil, "", spec)
	require.NoError(t, err)
	_, err = getSyverConfig(nil, "", spec, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(buf.String(), `unknown top-level key "prot"`))
}

// T-7b: an empty or null value makes the file strict and is itself an error.
func TestStrict_EmptyOrNullMarker(t *testing.T) {
	for _, marker := range []string{"syver-version:\n", "syver-version: ~\n"} {
		t.Run(strings.TrimSpace(marker), func(t *testing.T) {
			dir := t.TempDir()
			clean := writeSpecIn(t, dir, "clean.yaml", marker+okMatching)
			_, err := getSyverConfig(nil, "", clean, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), clean+":1: syver-version needs a version, e.g. syver-version: 0.20.0")

			typo := writeSpecIn(t, dir, "typo.yaml", marker+"prot: {}\n"+okMatching)
			buf := captureLog(t)
			_, err = getSyverConfig(nil, "", typo, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "needs a version")
			assert.Contains(t, err.Error(), `"prot"`, "the file is strict, so its unknown key is part of the error")
			assert.NotContains(t, buf.String(), "[WARN]")
		})
	}
}

// T-8: strictness sees the rendered template.
func TestStrict_TemplateConditionalKey(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })
	origFilter := currentTemplateFilter
	t.Cleanup(func() { currentTemplateFilter = origFilter })

	data := []byte("syver-version: 0.20.0\n{{if .Vars.on}}\nbogus-block:\n  x: 1\n{{end}}\n" + okMatching)
	for on, wantErr := range map[string]bool{"true": true, "false": false} {
		t.Run("on="+on, func(t *testing.T) {
			var err error
			currentTemplateFilter, err = NewTemplateFilter(nil, `{"on": `+on+`}`, nil)
			require.NoError(t, err)
			_, err = ReadJSONData(data, false, "syver.yaml")
			if wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), `"bogus-block"`)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// T-9: a strict root makes its includes strict, all the way down, and the
// error names the include and the root.
func TestStrict_InheritedByIncludes(t *testing.T) {
	t.Run("child", func(t *testing.T) {
		dir := t.TempDir()
		child := writeSpecIn(t, dir, "child.yaml", "prot: {}\n"+okMatching)
		root := writeSpecIn(t, dir, "root.yaml", "syver-version: 0.20.0\ngossfile:\n  child.yaml: {}\n"+okMatching)
		buf := captureLog(t)
		_, err := getSyverConfig(nil, "", root, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), child+": unknown top-level keys are errors")
		assert.Contains(t, err.Error(), "(strict because it is included from "+root+", which sets syver-version)")
		assert.Contains(t, err.Error(), child+`:1: "prot"`)
		assert.NotContains(t, buf.String(), "[WARN]")
	})

	t.Run("grandchild", func(t *testing.T) {
		dir := t.TempDir()
		grand := writeSpecIn(t, dir, "grand.yaml", "prot: {}\n"+okMatching)
		writeSpecIn(t, dir, "mid.yaml", "gossfile:\n  grand.yaml: {}\n"+okMatching)
		root := writeSpecIn(t, dir, "root.yaml", "syver-version: 0.20.0\ngossfile:\n  mid.yaml: {}\n"+okMatching)
		_, err := getSyverConfig(nil, "", root, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), grand+": unknown top-level keys are errors")
		assert.Contains(t, err.Error(), "included from "+root+",")
	})
}

// T-10: a marked include is strict for itself; an unmarked sibling only warns.
func TestStrict_MarkedIncludeUnderUnmarkedRoot(t *testing.T) {
	dir := t.TempDir()
	// gossfile: entries are merged in sorted key order, so the unmarked
	// sibling is read (and warns) before the marked one errors.
	sibling := writeSpecIn(t, dir, "a-sibling.yaml", "sibling-typo: {}\n"+okMatching)
	marked := writeSpecIn(t, dir, "b-marked.yaml", "syver-version: 0.20.0\nmarked-typo: {}\n"+okMatching)
	root := writeSpecIn(t, dir, "root.yaml", "gossfile:\n  a-sibling.yaml: {}\n  b-marked.yaml: {}\n"+okMatching)
	buf := captureLog(t)

	_, err := getSyverConfig(nil, "", root, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), marked+": syver-version is set, so unknown top-level keys are errors")
	assert.Contains(t, err.Error(), `"marked-typo"`)
	assert.NotContains(t, err.Error(), "sibling-typo")
	assert.Contains(t, buf.String(), sibling+`:1: unknown top-level key "sibling-typo" -- ignored`)
}

// T-11: render carries the highest marker among the files it read.
func TestRender_CarriesHighestSyverVersion(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr); outStoreFormat = UNSET })

	dir := t.TempDir()
	writeSpecIn(t, dir, "child.yaml", "syver-version: 0.21.0\n"+okMatching)
	writeSpecIn(t, dir, "skipped.yaml", "syver-version: 9.0.0\n"+okMatching)
	root := writeSpecIn(t, dir, "root.yaml", "syver-version: 0.20.0\ngossfile:\n  child.yaml: {}\n  skipped.yaml:\n    skip: true\n"+okMatching)
	c, err := util.NewConfig(util.WithSpecFile(root))
	require.NoError(t, err)
	out, err := RenderJSON(c)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "syver-version: 0.21.0\n"), "got:\n%s", out)

	plainDir := t.TempDir()
	writeSpecIn(t, plainDir, "child.yaml", okMatching)
	plain := writeSpecIn(t, plainDir, "root.yaml", "gossfile:\n  child.yaml: {}\n"+okMatching)
	c, err = util.NewConfig(util.WithSpecFile(plain))
	require.NoError(t, err)
	out, err = RenderJSON(c)
	require.NoError(t, err)
	assert.NotContains(t, out, "syver-version")
}

// T-12: JSON specs get the strict check without line numbers; vars files are
// never checked.
func TestStrict_JSONAndVarsFiles(t *testing.T) {
	dir := t.TempDir()
	marked := writeSpecIn(t, dir, "marked.json", `{"syver-version": "0.20.0", "prot": {}, "matching": {"ok": {"content": 1, "matches": 1}}}`)
	buf := captureLog(t)
	_, err := getSyverConfig(nil, "", marked, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), marked+": syver-version is set")
	assert.Contains(t, err.Error(), marked+`: "prot" (did you mean "port"?)`)

	unmarked := writeSpecIn(t, dir, "unmarked.json", `{"prot": {}, "matching": {"ok": {"content": 1, "matches": 1}}}`)
	_, err = getSyverConfig(nil, "", unmarked, nil)
	assert.NoError(t, err)

	for name, content := range map[string]string{
		"vars.yaml": "syver-version: 0.20.0\nprot: 1\nanything: 2\n",
		"vars.json": `{"syver-version": "0.20.0", "prot": 1}`,
	} {
		vars, err := varsFromFile(writeSpecIn(t, dir, name, content))
		assert.NoError(t, err, name)
		assert.Contains(t, vars, "prot", name)
	}
	assert.NotContains(t, buf.String(), "[WARN]")
	assert.NotContains(t, buf.String(), "unknown top-level key")
}

// addCommand runs `syver add command` against spec in-process.
func addCommand(t *testing.T, spec string) error {
	t.Helper()
	t.Cleanup(func() { log.SetOutput(os.Stderr); outStoreFormat = UNSET })
	c, err := util.NewConfig()
	require.NoError(t, err)
	return AddResources(spec, "Command", []string{"echo added"}, c)
}

// T-13: add keeps the marker at the top; a short form comes back quoted.
func TestAdd_KeepsSyverVersionAtTop(t *testing.T) {
	for in, want := range map[string]string{
		"0.20.0": "syver-version: 0.20.0\n",
		"0.20":   "syver-version: \"0.20\"\n",
	} {
		t.Run(in, func(t *testing.T) {
			spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "syver-version: "+in+"\n"+okMatching)
			require.NoError(t, addCommand(t, spec))
			got, err := os.ReadFile(spec)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(string(got), want), "got:\n%s", got)
			assert.Contains(t, string(got), "echo added")
		})
	}
}

// T-14 and T-14b: add refuses to rewrite a strict file with a typo, or one
// marked for a newer release, and leaves it byte-identical.
func TestAdd_RefusesStrictErrors(t *testing.T) {
	t.Run("typo", func(t *testing.T) {
		content := "syver-version: 0.20.0\nprot: {}\n" + okMatching
		spec := writeSpecIn(t, t.TempDir(), "syver.yaml", content)
		err := addCommand(t, spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"prot"`)
		got, _ := os.ReadFile(spec)
		assert.Equal(t, content, string(got))
	})
	t.Run("too old", func(t *testing.T) {
		withVersion(t, "0.19.0")
		content := "syver-version: 0.20.0\n" + okMatching
		spec := writeSpecIn(t, t.TempDir(), "syver.yaml", content)
		err := addCommand(t, spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), spec+" requires syver 0.20.0 or later (syver-version: 0.20.0); this is syver 0.19.0")
		got, _ := os.ReadFile(spec)
		assert.Equal(t, content, string(got))
	})
}

// T-15: validate exits non-zero on a release older than the marker, and a
// describe-stamped development build passes the same spec.
func TestValidate_MinimumVersion(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "syver-version: 0.20.0\n"+okMatching)
	run := func() (int, error) {
		var out bytes.Buffer
		c, err := util.NewConfig(util.WithSpecFile(spec), util.WithResultWriter(&out), util.WithNoColor())
		require.NoError(t, err)
		return Validate(t.Context(), c)
	}

	withVersion(t, "0.19.0")
	code, err := run()
	require.Error(t, err)
	assert.Equal(t, 78, code)
	assert.Equal(t, spec+" requires syver 0.20.0 or later (syver-version: 0.20.0); this is syver 0.19.0", err.Error())

	for _, stamp := range []string{"v0.19.0-3-gabc1234", "0.20.0", "v0.21.0"} {
		util.Version = stamp
		code, err = run()
		assert.NoError(t, err, stamp)
		assert.Equal(t, 0, code, stamp)
	}
}

// T-16: serve stops at startup on a strict error and never binds its port.
func TestServe_StrictErrorStopsBeforeListening(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "syver-version: 0.20.0\nprot: {}\n"+okMatching)
	c, err := util.NewConfig(util.WithSpecFile(spec))
	require.NoError(t, err)
	port := freePort(t)
	c.ListenAddress = fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, c) }()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"prot"`)
	case <-time.After(5 * time.Second):
		conn, dialErr := net.DialTimeout("tcp", c.ListenAddress, time.Second)
		if dialErr == nil {
			_ = conn.Close()
		}
		cancel()
		<-done
		t.Fatalf("Serve did not return on a strict load error (listener bound: %v)", dialErr == nil)
	}

	conn, dialErr := net.DialTimeout("tcp", c.ListenAddress, time.Second)
	if dialErr == nil {
		_ = conn.Close()
	}
	assert.Error(t, dialErr, "nothing may be listening on the chosen port")
}
