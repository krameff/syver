package syver

import (
	"testing"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
)

// TestMatchingSetSkipDisablesValidation is the regression test for
// FEAT-007 G2/AC-7, the one deliberate behaviour change in this refactor.
//
// Before the fix, Matching.SetSkip() was a no-op. matching IS in
// SyverConfig.Resources() (unlike gossfile), so it does reach
// applyDisabledTypes (dependency_scheduler.go) via runValidation below --
// meaning util.WithDisabledResourceTypes("matching") silently failed to
// disable it: total=1 skipped=0 failed=1, reproduced here with a matching
// resource whose content deliberately doesn't match (so an un-skipped
// Validate() fails, not passes -- a false SKIP wouldn't be distinguishable
// from a false SUCCESS otherwise). After the fix: total=1 skipped=1
// failed=0.
func TestMatchingSetSkipDisablesValidation(t *testing.T) {
	json := `{"matching":{"test1":{"content":"actual","matches":"expected-but-does-not-match"}}}`
	cfg, err := ReadJSONData([]byte(json), true, "")
	checkErr(t, err, "reading config failed")

	sys := system.New("")

	out, err := runValidation(t.Context(), sys, cfg, []string{"matching"}, 1)
	checkErr(t, err, "runValidation failed")

	var total, skipped, failed int
	for results := range out {
		for _, r := range results {
			total++
			switch r.Result {
			case resource.SKIP:
				skipped++
			case resource.FAIL:
				failed++
			}
		}
	}

	if total != 1 || skipped != 1 || failed != 0 {
		t.Fatalf("disabling \"matching\" via DisabledResourceTypes: got total=%d skipped=%d failed=%d, want total=1 skipped=1 failed=0", total, skipped, failed)
	}
}
