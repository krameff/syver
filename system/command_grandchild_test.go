//go:build linux || darwin

package system

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// The case that proved a hand-rolled bound cannot work here. A grandchild that
// escapes the process group (setsid, nohup, a daemonising service) inherits the
// stdout pipe, so Wait blocks on the copy goroutine.
//
// A bound that gives up and returns leaves that goroutine writing into buffers
// setup() is already reading: measured at 5 data races, with seconds of added
// latency bought for nothing. exec's WaitDelay force-closes the pipes and does
// not return until the I/O goroutines are done, so this must stay race-free.
// Run under -race or the race half asserts nothing.
//
// The 3s budget is load-bearing and must stay ABOVE any fixed delay anyone is
// tempted to introduce. An earlier version of this test used 300ms against a
// hardcoded 2s WaitDelay; the deadline won, the message came out right, and the
// test passed while a command at the DEFAULT 10s budget returned after 2s with
// the Go-internal "exec: WaitDelay expired before I/O complete". A fixture below
// the delay cannot reach the failing state.
func TestGrandchildEscapesTheBound(t *testing.T) {
	const budget = 3 * time.Second

	start := time.Now()
	c := NewDefCommand(context.Background(), "setsid sleep 30 & echo started", &System{},
		util.Config{Timeout: budget})
	err := c.(*DefCommand).setup()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error from a command whose grandchild outlives it")
	}
	// The operator must see their own budget, never stdlib wording.
	if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "3s") {
		t.Errorf("error should name the exceeded budget, got: %v", err)
	}
	// Returning EARLY is the defect: it means a fixed delay overrode the budget.
	if elapsed < budget {
		t.Errorf("returned after %v, before the %v budget -- a fixed WaitDelay is "+
			"overriding the configured timeout", elapsed.Round(time.Millisecond), budget)
	}
	if elapsed > budget*3 {
		t.Errorf("bound did not hold: took %v for a %v budget", elapsed, budget)
	}
}
