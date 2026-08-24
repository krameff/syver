package system

import (
	"context"
	"testing"
	"time"
)

// WaitDelay must track the command's own budget, never a constant.
//
// This asserts the INVARIANT rather than a symptom, because no behavioural
// fixture can cover the defect. A fixed delay K is wrong exactly when
// K < timeout, and timeout is user-configurable and unbounded -- so a test that
// times a real command out only catches K below whatever budget the fixture
// happens to use. That trap has been walked into twice here: a 300ms fixture was
// blind to a 2s constant, and its 3s replacement was blind to a 5s one. Raising
// the number again just moves the blind spot.
//
// Checking the assignment directly costs milliseconds and cannot be outrun by a
// larger constant. Keep TestGrandchildEscapesTheBound alongside it: that one
// proves the race and the message end to end, which this cannot.
func TestWaitDelayTracksTheBudget(t *testing.T) {
	for _, ms := range []int{1234, 10000, 45000} {
		// `echo` deliberately: a builtin in both sh and cmd.exe, so this untagged
		// test carries no assumption about what binaries the platform ships. The
		// command's outcome is irrelevant anyway -- WaitDelay is set before Run,
		// and the error is discarded.
		cmd := commandWrapper(context.Background(), "echo waitdelay-probe")
		_ = runCommand(cmd, ms)
		want := time.Duration(ms) * time.Millisecond
		if cmd.Cmd.WaitDelay != want {
			t.Errorf("timeout=%dms: WaitDelay=%v, want %v -- a constant is "+
				"overriding the configured budget", ms, cmd.Cmd.WaitDelay, want)
		}
	}
}
