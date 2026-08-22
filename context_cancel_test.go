package syver

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
)

// The whole point of threading a context is that cancelling it stops work in
// flight. That chain is long -- CLI signal handler -> Validate -> the runner ->
// Resource.Validate -> system.DefCommand.setup -> exec -- and every link is a
// plain parameter, so any one of them reverting to context.Background() breaks
// cancellation silently, with every other test still green. system/command.go
// did exactly that before this change.
func TestCancellingContextStopsARunningCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep(1)")
	}

	c := &resource.Command{Exec: "sleep 30", ExitStatus: 0}
	c.SetID("sleep 30")

	ctx, cancel := context.WithCancel(t.Context())
	sys := system.New("")

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Validate(ctx, sys)
	}()

	// Let the child actually start before pulling the rug.
	time.Sleep(500 * time.Millisecond)
	cancel()
	cancelledAt := time.Now()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Validate never returned after cancellation")
	}

	// The bound has to be well under the command timeout, not merely finite.
	// With no timeout configured, runCommand falls back to a 10s default that
	// reaps the child anyway -- so a test that only asserted "returns
	// eventually" passed just as happily with the context ignored, it simply
	// took ten seconds to do it. What distinguishes a context that reaches
	// exec is that the return is immediate.
	if elapsed := time.Since(cancelledAt); elapsed > 3*time.Second {
		t.Fatalf("Validate took %s to return after cancellation; the context is not reaching exec "+
			"(it is being reaped by runCommand's default timeout instead)", elapsed.Round(time.Millisecond))
	}
}
