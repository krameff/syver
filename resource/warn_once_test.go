package resource

import (
	"strings"
	"sync"
	"testing"
)

// Every spec-mistake warning must fire once per process per key.
//
// This asserts the INVARIANT rather than observing a caller, because the flood
// this prevents is invisible from a single validate run: each CLI invocation is
// a fresh process, so a warning that repeats per check only shows up under
// `serve`, where the checks re-run on every cache refresh. That is exactly how
// the empty-list warning shipped unguarded.
//
// It also guards a subtler trap. A caller can be accidentally self-limiting --
// Command.Validate rescues a negative timeout by MUTATING the resource, so the
// condition is false on every later sweep. Measured before this guard existed:
// a non-mutating Validate, an entirely reasonable refactor, produced one warning
// per sweep. The once-ness must live here, where it is stated, not there, where
// it is inherited.
func TestWarnSpecOnceFiresOncePerKey(t *testing.T) {
	var buf lockedBuilder
	restore := warnSpecOutput(&buf)
	defer restore()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			warnSpecOnce("test-key-a", "%s went wrong", "thing")
			warnSpecOnce("test-key-b", "%s went wrong", "other")
		}()
	}
	wg.Wait()

	if got := strings.Count(buf.String(), "went wrong"); got != 2 {
		t.Errorf("50 concurrent calls on 2 keys emitted %d warnings, want 2:\n%s", got, buf.String())
	}
}
