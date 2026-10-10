package syver

import (
	"runtime"
	"testing"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
)

// workerCount had three pasted copies, and the one on the main validate path
// had no floor: --max-concurrent 0 started no worker, so a failing spec
// reported "Count: 0, Failed: 0" and exited 0, and serve answered 200.
//
// Revert-proof: drop the max(1, ...) from workerCount and the 0 and -3 rows
// here fail, and so does TestZeroMaxConcurrentStillValidates.
func TestWorkerCountHasAFloorOfOne(t *testing.T) {
	ceiling := runtime.NumCPU() * 5
	for _, tc := range []struct{ in, want int }{
		{0, 1},
		{-3, 1},
		{1, 1},
		{2, min(2, ceiling)},
		{ceiling + 100, ceiling},
	} {
		if got := workerCount(tc.in); got != tc.want {
			t.Errorf("workerCount(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The behaviour the floor exists for, end to end through the main validate
// path: a library caller that sets MaxConcurrent to 0 directly, bypassing the
// CLI's validator, must still get every check run, not an empty pass.
func TestZeroMaxConcurrentStillValidates(t *testing.T) {
	json := `{"matching":{"test1":{"content":"actual","matches":"expected-but-does-not-match"}}}`
	cfg, err := ReadJSONData([]byte(json), true, "")
	checkErr(t, err, "reading config failed")

	out, err := runValidation(t.Context(), system.New(""), cfg, nil, 0)
	checkErr(t, err, "runValidation failed")

	var total, failed int
	for group := range out {
		for _, r := range group {
			total++
			if r.Result == resource.FAIL {
				failed++
			}
		}
	}
	if total == 0 || failed == 0 {
		t.Fatalf("got %d results, %d failed: with MaxConcurrent 0 the failing check must still run and fail", total, failed)
	}
}

func TestValidateMaxConcurrent(t *testing.T) {
	for _, n := range []int{0, -1} {
		if err := ValidateMaxConcurrent(n); err == nil {
			t.Errorf("ValidateMaxConcurrent(%d) = nil, want an error", n)
		}
	}
	// The control: a validator that rejected everything would pass the loop
	// above and break the flag.
	for _, n := range []int{1, 50} {
		if err := ValidateMaxConcurrent(n); err != nil {
			t.Errorf("ValidateMaxConcurrent(%d) = %v, want nil", n, err)
		}
	}
}
