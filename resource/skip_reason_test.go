package resource

import (
	"path/filepath"
	"testing"

	"github.com/krameff/syver/matchers"
	"github.com/krameff/syver/system"
)

func skipReasonsByProperty(results []TestResult) map[string]string {
	out := make(map[string]string, len(results))
	for _, r := range results {
		out[r.Property] = r.SkipReason
	}
	return out
}

// A failing first check promotes every later attribute to skipped. The reason
// names the check that caused it, so the skip does not read as deliberate.
func TestSkipReasonPromotedByFailingFirstCheck(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: true, Mode: "0644"}
	f.SetID("probe")
	results := ValidateSafe(t.Context(), f, system.New(""))
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d: %+v", len(results), results)
	}
	if results[0].Result != FAIL || results[0].SkipReason != "" {
		t.Errorf("exists: want FAIL with no skip-reason, got result=%d reason=%q", results[0].Result, results[0].SkipReason)
	}
	if !results[1].Skipped || results[1].SkipReason != "exists did not pass" {
		t.Errorf("mode: want skipped with %q, got skipped=%v reason=%q", "exists did not pass", results[1].Skipped, results[1].SkipReason)
	}
}

// `exists: false` that passes still promotes the rest to skipped (there is no
// file to have a mode), so "did not pass" would be untrue there.
func TestSkipReasonPromotedByPassingFalse(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: false, Mode: "0644"}
	f.SetID("probe")
	got := skipReasonsByProperty(ValidateSafe(t.Context(), f, system.New("")))
	if got["exists"] != "" || got["mode"] != "exists is false" {
		t.Errorf("want exists=\"\" mode=%q, got %v", "exists is false", got)
	}
}

func TestSkipReasonAuthorWrittenOnSkipTrue(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: true, Mode: "0644", Skip: true,
		DiscoveryMeta: DiscoveryMeta{SkipReason: "not applicable to containers"}}
	f.SetID("probe")
	for _, r := range ValidateSafe(t.Context(), f, system.New("")) {
		if !r.Skipped || r.SkipReason != "not applicable to containers" {
			t.Errorf("%s: want skipped with the author's reason, got skipped=%v reason=%q", r.Property, r.Skipped, r.SkipReason)
		}
	}
}

func TestSkipReasonEmptyOnSkipTrueWithoutReason(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: true, Mode: "0644", Skip: true}
	f.SetID("probe")
	for _, r := range ValidateSafe(t.Context(), f, system.New("")) {
		if !r.Skipped || r.SkipReason != "" {
			t.Errorf("%s: want skipped with no reason, got skipped=%v reason=%q", r.Property, r.Skipped, r.SkipReason)
		}
	}
}

// An author's skip-reason only explains an author's skip. When skip: is false
// and the skip is a promotion, Syver's own reason is the accurate one.
func TestSkipReasonAuthorReasonIgnoredWhenNotSkipped(t *testing.T) {
	f := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: true, Mode: "0644",
		DiscoveryMeta: DiscoveryMeta{SkipReason: "not applicable to containers"}}
	f.SetID("probe")
	got := skipReasonsByProperty(ValidateSafe(t.Context(), f, system.New("")))
	if got["exists"] != "" || got["mode"] != "exists did not pass" {
		t.Errorf("want exists=\"\" mode=%q, got %v", "exists did not pass", got)
	}
}

func TestSkipReasonNotSetOnPassingResource(t *testing.T) {
	dir := t.TempDir()
	f := &File{Path: dir, Exists: true, Filetype: "directory",
		DiscoveryMeta: DiscoveryMeta{SkipReason: "unused"}}
	f.SetID("probe")
	for _, r := range ValidateSafe(t.Context(), f, system.New("")) {
		if r.Skipped || r.SkipReason != "" {
			t.Errorf("%s: want a non-skipped result with no reason, got skipped=%v reason=%q", r.Property, r.Skipped, r.SkipReason)
		}
	}
}

// promotedSkipReason must cover exactly the three conditions shouldSkip
// promotes on, or a promoted skip renders with no reason.
func TestPromotedSkipReasonCoversEveryShouldSkipCondition(t *testing.T) {
	ve := ValidateError("boom")
	cases := []struct {
		name  string
		first TestResult
		want  string
	}{
		{"error", TestResult{Property: "installed", Result: FAIL, Err: &ve}, "installed did not pass"},
		{"fail", TestResult{Property: "status", Result: FAIL}, "status did not pass"},
		{"passing false", TestResult{Property: "running", Result: SUCCESS, MatcherResult: matchers.MatcherResult{Actual: false}}, "running is false"},
		{"passing true", TestResult{Property: "running", Result: SUCCESS, MatcherResult: matchers.MatcherResult{Actual: true}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldSkip([]TestResult{c.first}); got != (c.want != "") {
				t.Fatalf("test case disagrees with shouldSkip: shouldSkip=%v", got)
			}
			if got := promotedSkipReason(c.first); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestSkipResourceResultsCarriesReasonInBothFields(t *testing.T) {
	f := &File{Path: "/x", Exists: true}
	f.SetID("dependent")
	results := SkipResourceResults(f, `dependency "file:prereq" failed`)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	r := results[0]
	if r.SkipReason != `dependency "file:prereq" failed` {
		t.Errorf("SkipReason = %q", r.SkipReason)
	}
	if r.Err == nil || string(*r.Err) != `dependency "file:prereq" failed` {
		t.Errorf("Err must still carry the reason, got %v", r.Err)
	}
}

func TestSkipReasonCollapsesWhitespaceAtBothSources(t *testing.T) {
	f := &File{Path: "/x", Exists: true}
	f.SetID("dependent")
	r := SkipResourceResults(f, "dependency\n  \"file:a\"\tfailed\n")[0]
	if r.SkipReason != `dependency "file:a" failed` || string(*r.Err) != r.SkipReason {
		t.Errorf("SkipResourceResults: reason %q err %q", r.SkipReason, string(*r.Err))
	}

	g := &File{Path: filepath.Join(t.TempDir(), "missing"), Exists: true, Skip: true,
		DiscoveryMeta: DiscoveryMeta{SkipReason: "line one\nok 99 - injected\n\nBail out!\n"}}
	g.SetID("probe")
	for _, r := range ValidateSafe(t.Context(), g, system.New("")) {
		if r.SkipReason != "line one ok 99 - injected Bail out!" {
			t.Errorf("%s: skip-reason %q", r.Property, r.SkipReason)
		}
	}
}
