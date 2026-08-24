//go:build linux || darwin

package system

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// A timed-out command must say what the budget was. ctx.Err() alone renders as
// "context deadline exceeded", which names neither the timeout nor its value.
// Worse, it was non-deterministic: the context deadline and runCommand's own
// time.After share a duration and race, so the same fixture produced the
// readable message on Linux and the opaque one on a Windows CI runner.
//
// Cancellation must still report as cancellation: both arrive via ctx.Err() and
// only errors.Is separates them.
func TestTimeoutErrorNamesTheBudget(t *testing.T) {
	t.Run("timeout names the duration", func(t *testing.T) {
		c := NewDefCommand(context.Background(), "sleep 10", &System{},
			util.Config{Timeout: 150 * time.Millisecond})
		err := c.(*DefCommand).setup()
		if err == nil {
			t.Fatal("expected an error from a command that outlives its timeout")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error should say it timed out, got: %v", err)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("raw ctx.Err() leaked to the operator: %v", err)
		}
		if !strings.Contains(err.Error(), "150ms") {
			t.Errorf("error should name the configured budget, got: %v", err)
		}
	})

	t.Run("cancellation still reports as cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c := NewDefCommand(ctx, "sleep 10", &System{},
			util.Config{Timeout: 10 * time.Second})
		err := c.(*DefCommand).setup()
		if err == nil {
			t.Fatal("expected an error from a cancelled context")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("cancellation should still say so, got: %v", err)
		}
	})
}
