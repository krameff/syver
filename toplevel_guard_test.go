package syver

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
)

// TestCheckTopLevelKeys_UnknownKeysWarn is the measured fixture from
// PLAN_toplevel_key_guard.md: a spec with two unknown top-level keys and one
// legal one must report exactly the two, each with its own line number, and
// no suggestion for either (neither is close to a legal key).
func TestCheckTopLevelKeys_UnknownKeysWarn(t *testing.T) {
	data := []byte("plugins:\n  some-vendor-thing:\n    binary: /nonexistent/plugin\ntotally-made-up-key:\n  whatever: true\ncommand:\n  echo ok:\n    exit-status: 0\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	require := assert.New(t)
	require.Len(warnings, 2, "plugins and totally-made-up-key are unknown; command is legal and must not warn")

	require.Equal("syver.yaml", warnings[0].Path)
	require.Equal(1, warnings[0].Line)
	require.Equal("plugins", warnings[0].Key)
	require.Equal("", warnings[0].Suggestion, "plugins is not within edit distance 2 of any legal key")

	require.Equal("syver.yaml", warnings[1].Path)
	require.Equal(4, warnings[1].Line)
	require.Equal("totally-made-up-key", warnings[1].Key)
	require.Equal("", warnings[1].Suggestion)
}

// TestCheckTopLevelKeys_AnchorCarrierExempt is the plan's anchor fixture:
// a top-level key that exists purely to carry a shared YAML anchor must
// produce zero warnings, or the anchor pattern documented in the plan's
// "trap" section stops working silently.
func TestCheckTopLevelKeys_AnchorCarrierExempt(t *testing.T) {
	data := []byte("common-checks: &common\n  exit-status: 0\ncommand:\n  echo one:\n    <<: *common\n  echo two:\n    <<: *common\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	assert.Empty(t, warnings, "an anchor-carrying top-level key must be exempt, not warned about")
}

// TestCheckTopLevelKeys_AnchorReferenceDoesNotSuppressUnrelatedWarning is
// the anchor exemption's required negative: an anchor *reference* nested
// under a known key must not suppress a warning for a genuinely unrelated
// unknown top-level key elsewhere in the same document.
func TestCheckTopLevelKeys_AnchorReferenceDoesNotSuppressUnrelatedWarning(t *testing.T) {
	data := []byte("common-checks: &common\n  exit-status: 0\ncommand:\n  echo one:\n    <<: *common\nprot:\n  8080:\n    listening: true\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	require := assert.New(t)
	require.Len(warnings, 1, "the anchor reference under command: must not suppress the unrelated unknown key prot:")
	require.Equal("prot", warnings[0].Key)
	require.Equal("port", warnings[0].Suggestion)
}

// TestCheckTopLevelKeys_DeepAnchorDoesNotExempt pins D2's narrowed anchor
// exemption: only a top-level key whose IMMEDIATE value node carries the
// anchor is exempt. An anchor buried further down inside an otherwise
// ordinary (and unrecognised) block is not evidence the key exists to carry
// anything, and must still warn -- unlike
// TestCheckTopLevelKeys_AnchorCarrierExempt above, where the anchor sits
// directly on common-checks:'s own value.
func TestCheckTopLevelKeys_DeepAnchorDoesNotExempt(t *testing.T) {
	data := []byte("bogus:\n  nested:\n    deep: &deepanchor\n      a: 1\ncommand:\n  echo ok:\n    exit-status: 0\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	if assert.Len(t, warnings, 1, "an anchor three levels below bogus: is not evidence bogus: exists to carry it -- it must still warn") {
		assert.Equal(t, "bogus", warnings[0].Key)
	}
}

// TestCheckTopLevelKeys_XPrefixExempt covers D2's second exemption: a
// top-level key beginning "x-" is always allowed, independent of whether it
// carries an anchor.
func TestCheckTopLevelKeys_XPrefixExempt(t *testing.T) {
	data := []byte("x-shared-vars:\n  timeout: 30\ncommand:\n  echo ok:\n    exit-status: 0\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	assert.Empty(t, warnings, "a top-level key prefixed x- must be exempt")
}

// TestCheckTopLevelKeys_SuggestionWithinEditDistance2 covers D6's positive
// case with a key distinct from the prot/port example already exercised
// above: a single missing character.
func TestCheckTopLevelKeys_SuggestionWithinEditDistance2(t *testing.T) {
	data := []byte("gossfil:\n  extra:\n    file: extra.yaml\ncommand:\n  echo ok:\n    exit-status: 0\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	require := assert.New(t)
	require.Len(warnings, 1)
	require.Equal("gossfil", warnings[0].Key)
	require.Equal("gossfile", warnings[0].Suggestion)
}

// TestCheckTopLevelKeys_NoSuggestionWhenFar covers D6's required negative:
// a key far from every legal one warns with no "did you mean".
func TestCheckTopLevelKeys_NoSuggestionWhenFar(t *testing.T) {
	data := []byte("totally-unrelated-thing:\n  whatever: true\ncommand:\n  echo ok:\n    exit-status: 0\n")

	warnings := checkTopLevelKeys(data, "syver.yaml")

	require := assert.New(t)
	require.Len(warnings, 1)
	require.Equal("totally-unrelated-thing", warnings[0].Key)
	require.Equal("", warnings[0].Suggestion, "a key far from every legal one must not get a suggestion")
}

// TestTopLevelWarning_String pins the exact wording against the plan's
// worked examples, including the D7 fallback (no path -- just the line).
func TestTopLevelWarning_String(t *testing.T) {
	withPath := topLevelWarning{Path: "syver.yaml", Line: 4, Key: "plugins"}
	assert.Equal(t, `syver.yaml:4: unknown top-level key "plugins" -- ignored`, withPath.String())

	withSuggestion := topLevelWarning{Path: "syver.yaml", Line: 7, Key: "prot", Suggestion: "port"}
	assert.Equal(t, `syver.yaml:7: unknown top-level key "prot" -- ignored (did you mean "port"?)`, withSuggestion.String())

	noPath := topLevelWarning{Line: 3, Key: "plugins"}
	assert.Equal(t, `3: unknown top-level key "plugins" -- ignored`, noPath.String(), "D7 fallback: no path available, just the line")
}

// TestLegalTopLevelKeys_MatchesSyverConfigYAMLTags is the reflection guard
// the plan requires: legalTopLevelKeys() must derive to exactly the same set
// as SyverConfig's own yaml tags. A field added to SyverConfig without a
// matching resource.Register (or, for a non-resource field, without adding
// its key to legalTopLevelKeys()) must fail this test, not silently start
// producing a false "unknown top-level key" warning for real specs.
func TestLegalTopLevelKeys_MatchesSyverConfigYAMLTags(t *testing.T) {
	typ := reflect.TypeOf(SyverConfig{})
	want := make(map[string]bool, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		want[name] = true
	}

	got := legalTopLevelKeys()

	assert.Equal(t, want, got, "legalTopLevelKeys() must match SyverConfig's yaml tags exactly -- if this fails after adding a field, register the resource type via resource.Register, or for a non-resource field add its key to legalTopLevelKeys()")
}

// TestReadJSONData_UnknownTopLevelKey_WarnsExactlyOnceDespiteDoubleDecode is
// D5's correctness requirement, modelled directly on
// Test_syverfileAlias_CollisionLogsWarnAndGossfileWins (the BUG-001 test
// this is required to match the shape of): every validate run decodes a
// spec twice, peek then real (see quietDecode's doc comment in store.go),
// and the guard must warn on the real load only -- an exactly-once
// assertion, not merely "the warning appears".
func TestReadJSONData_UnknownTopLevelKey_WarnsExactlyOnceDespiteDoubleDecode(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	data := []byte("plugins:\n  thing: true\ncommand:\n  echo ok:\n    exit-status: 0\n")

	// Simulate loadSyverConfig's peek-then-load sequence (validate.go):
	// quietDecode is set true for the peek pass and reset for the real one.
	quietDecode = true
	t.Cleanup(func() { quietDecode = false })
	_, err := ReadJSONData(data, false, "syver.yaml")
	assert.NoError(t, err)

	quietDecode = false
	_, err = ReadJSONData(data, false, "syver.yaml")
	assert.NoError(t, err)

	assert.Equal(t, 1, strings.Count(logOutput.String(), "unknown top-level key"),
		"the unknown-key warning must appear exactly once across the peek+real double decode, not twice")
	assert.Contains(t, logOutput.String(), `unknown top-level key "plugins"`)
}

// TestReadJSONData_ValidatesCleanDespiteUnknownTopLevelKeys is the plan's
// "does not change results" claim (D1), exercised end to end: two unknown
// top-level keys warn, but the spec still decodes and validates exactly as
// if they were not there.
func TestReadJSONData_ValidatesCleanDespiteUnknownTopLevelKeys(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	data := []byte("plugins:\n  some-vendor-thing:\n    binary: /nonexistent/plugin\ntotally-made-up-key:\n  whatever: true\nmatching:\n  ok:\n    content: actual\n    matches: actual\n")

	cfg, err := ReadJSONData(data, false, "guard.yaml")
	assert.NoError(t, err)

	assert.Equal(t, 2, strings.Count(logOutput.String(), "unknown top-level key"), "plugins and totally-made-up-key should each warn once")

	sys := system.New("")
	out, err := runValidation(t.Context(), sys, cfg, nil, 1)
	assert.NoError(t, err)

	var total, failed int
	for results := range out {
		for _, r := range results {
			total++
			if r.Result == resource.FAIL {
				failed++
			}
		}
	}

	assert.Equal(t, 1, total, "the guard must not change what gets validated -- only the matching: block should run")
	assert.Equal(t, 0, failed, "the run must still exit clean")
}

// TestReadJSONData_IncludedGossfile_WarnsWithIncludedFilePath covers D7:
// a typo inside an *included* gossfile must warn with that file's own path,
// not the parent's -- the least useful version of this diagnostic would be
// one that always names the root spec regardless of where the typo lives.
func TestReadJSONData_IncludedGossfile_WarnsWithIncludedFilePath(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	dir := t.TempDir()
	childPath := filepath.Join(dir, "child.yaml")
	parentPath := filepath.Join(dir, "parent.yaml")

	childContent := "bogus-key:\n  whatever: true\ncommand:\n  echo ok:\n    exit-status: 0\n"
	parentContent := "gossfile:\n  child:\n    file: child.yaml\ncommand:\n  echo hi:\n    exit-status: 0\n"

	assert.NoError(t, os.WriteFile(childPath, []byte(childContent), 0644))
	assert.NoError(t, os.WriteFile(parentPath, []byte(parentContent), 0644))

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cfg, err := ReadJSON(parentPath)
	assert.NoError(t, err)

	_, err = mergeJSONData(cfg, 0, dir)
	assert.NoError(t, err)

	assert.Contains(t, logOutput.String(), "child.yaml", "the warning for the included file's typo must name that file")
	assert.NotContains(t, logOutput.String(), "parent.yaml:", "the warning must not misattribute the included file's typo to the parent")
}

// TestVarsFile_UnknownKeysProduceNoTopLevelWarning is the regression test
// for the plan's "easiest mistake available" (§ Feasibility findings,
// "Vars files must not be checked"): --vars files are arbitrary
// user-supplied data with no fixed vocabulary, decoded through unmarshal
// directly rather than ReadJSONData, and must never trigger this guard.
func TestVarsFile_UnknownKeysProduceNoTopLevelWarning(t *testing.T) {
	dir := t.TempDir()
	varsPath := filepath.Join(dir, "vars.yaml")
	assert.NoError(t, os.WriteFile(varsPath, []byte("plugins: whatever\ntotally-made-up-key: 1\n"), 0644))

	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	vars, err := varsFromFile(varsPath)
	assert.NoError(t, err)
	assert.Contains(t, vars, "plugins")

	assert.NotContains(t, logOutput.String(), "unknown top-level key", "vars files are arbitrary data and must never be checked against the spec vocabulary")
}

// TestReadJSONData_TemplateConditionalTopLevelKey pins that the guard sees
// the spec AFTER template rendering, not before: an unknown top-level key
// that only exists once a template condition renders it in must warn, and
// the same spec with the condition rendered out must not -- there is no
// unknown key left in the byte stream for the guard to see. ReadJSONData
// applies currentTemplateFilter to data before checkTopLevelKeys ever runs
// (see the top of ReadJSONData in store.go), so this exercises that
// ordering directly rather than assuming it.
func TestReadJSONData_TemplateConditionalTopLevelKey(t *testing.T) {
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = UNSET })

	origFilter := currentTemplateFilter
	t.Cleanup(func() { currentTemplateFilter = origFilter })

	data := []byte("{{if .Vars.enableBogus}}\nbogus-block:\n  whatever: true\n{{end}}\ncommand:\n  echo ok:\n    exit-status: 0\n")

	t.Run("rendered in", func(t *testing.T) {
		var logOutput bytes.Buffer
		log.SetOutput(&logOutput)
		t.Cleanup(func() { log.SetOutput(os.Stderr) })

		var err error
		currentTemplateFilter, err = NewTemplateFilter(nil, `{"enableBogus": true}`, nil)
		assert.NoError(t, err)

		cfg, err := ReadJSONData(data, false, "syver.yaml")
		assert.NoError(t, err)
		assert.Contains(t, cfg.Commands, "echo ok", "the legal command: key must still decode regardless of the guard")

		assert.Contains(t, logOutput.String(), `unknown top-level key "bogus-block"`,
			"a top-level key that only exists after template rendering must still be caught once it's rendered in")
	})

	t.Run("rendered out", func(t *testing.T) {
		var logOutput bytes.Buffer
		log.SetOutput(&logOutput)
		t.Cleanup(func() { log.SetOutput(os.Stderr) })

		var err error
		currentTemplateFilter, err = NewTemplateFilter(nil, `{"enableBogus": false}`, nil)
		assert.NoError(t, err)

		cfg, err := ReadJSONData(data, false, "syver.yaml")
		assert.NoError(t, err)
		assert.Contains(t, cfg.Commands, "echo ok", "the legal command: key must still decode regardless of the guard")

		assert.NotContains(t, logOutput.String(), "unknown top-level key",
			"once the template condition renders bogus-block: out, there is no unknown key left in the byte stream to warn about")
	})
}
