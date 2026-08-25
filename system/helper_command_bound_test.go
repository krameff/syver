//go:build linux || darwin

package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// escapingShim puts an executable named `name` at the front of PATH which spawns
// a grandchild OUTSIDE its own process group and then blocks.
//
// The grandchild is the whole point. util.configureProcessGroup puts the direct
// child in its own group and Cancel signals that group, so a plain `&` child is
// reaped and nothing escapes. setsid detaches, and the detached process inherits
// the stdout pipe -- which is what keeps exec's copy goroutine readable, and so
// keeps Wait blocked, long after the process syver started is dead.
func escapingShim(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available (expected on macOS); the escaped-grandchild " +
			"path is covered on linux only")
	}
	dir := t.TempDir()
	ran := filepath.Join(dir, name+".ran")
	script := "#!/bin/sh\n: >" + ran + "\nsetsid sleep 60 &\nsleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	resolved, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s: shim written to %s but not resolvable on PATH: %v", name, dir, err)
	}
	if filepath.Dir(resolved) != filepath.Clean(dir) {
		t.Fatalf("%s resolved to %s, not the escaping shim in %s -- the real binary "+
			"would answer immediately, which is NOT a timing failure", name, resolved, dir)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if _, err := os.Stat(ran); err != nil {
			t.Errorf("%s: the shim at %s resolved but never executed (no %s) -- exec "+
				"failed, e.g. a noexec TMPDIR. NOT a timing failure", name, resolved, ran)
		}
	})
}

// The helper path must be BOUNDED, not merely cancellable.
//
// Cancelling the context kills the helper PROCESS. It does not bound Wait, which
// blocks on the copy goroutine for as long as anything holds the write end of
// the stdout pipe. An escaped grandchild holds it for its own lifetime, so
// without a WaitDelay this returns when the GRANDCHILD exits, not when the
// budget expires -- under `serve` that holds syverMu for the life of the
// process, reachable from an unauthenticated /healthz.
//
// The budget here is deliberately far below the grandchild's 60s lifetime: that
// gap IS the assertion. A fixture whose grandchild outlives the test binary
// cannot distinguish "bounded correctly" from "got lucky".
func TestHelperCommandIsBoundedNotJustCancelled(t *testing.T) {
	escapingShim(t, "systemctl")

	oldTimeout, oldGrace := helperCommandTimeout, helperIOGrace
	// Both, not just the deadline. They are additive, so leaving the real 5s grace
	// in place would make this test spend 6s proving a 1s point -- and would hide
	// the additive relationship it is meant to exercise.
	helperCommandTimeout, helperIOGrace = 1*time.Second, 1*time.Second
	t.Cleanup(func() { helperCommandTimeout, helperIOGrace = oldTimeout, oldGrace })

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		runHelperCommand(context.Background(), "systemctl", "status", "nope")
		done <- time.Since(start)
	}()

	select {
	case elapsed := <-done:
		if elapsed > 10*time.Second {
			t.Errorf("returned after %v, far beyond the 1s budget", elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("WEDGED: runHelperCommand did not return within 15s despite a 1s " +
			"budget -- the context killed the process but nothing bounded Wait")
	}
}

// The WaitDelay must be the I/O GRACE, not the deadline.
//
// TestHelperCommandIsBoundedNotJustCancelled cannot see this: any non-zero value
// under its ceiling passes that test, including one equal to helperCommandTimeout
// which doubles the worst-case hold on syverMu to ~60s. Assert the property, not
// just the symptom.
//
// Three things are checked, and each has a way of being wrong on its own:
//   - it tracks helperIOGrace, so it is not some unrelated constant that happens
//     to match today's value. Two distinct values, because one cannot tell
//     "tracks the var" from "is a constant equal to it".
//   - it is non-zero. exec reads zero as "no delay at all", so a zero here is the
//     original unbounded bug wearing an assignment.
//   - it is strictly below helperCommandTimeout. Tightening the worst case from
//     ~60s to ~35s is the entire reason this is a separate var, and deriving it
//     from the deadline again would silently undo that while every other
//     assertion here still passed.
func TestHelperWaitDelayIsTheGraceNotTheDeadline(t *testing.T) {
	oldGrace := helperIOGrace
	t.Cleanup(func() { helperIOGrace = oldGrace })

	for _, grace := range []time.Duration{2 * time.Second, 7 * time.Second} {
		helperIOGrace = grace
		// `true` is POSIX-mandated, exits 0 immediately, and exists on every host
		// this suite runs on. The command's outcome is irrelevant; only the field
		// set on it before Start is under test.
		cmd, _ := runHelperCommand(context.Background(), "true")
		if cmd == nil {
			t.Fatalf("grace %v: runHelperCommand returned no command", grace)
		}
		if cmd.Cmd.WaitDelay != grace {
			t.Errorf("grace %v: WaitDelay is %v -- it must track helperIOGrace",
				grace, cmd.Cmd.WaitDelay)
		}
	}

	helperIOGrace = oldGrace
	if helperIOGrace <= 0 {
		t.Errorf("helperIOGrace is %v -- exec treats a zero WaitDelay as no bound "+
			"at all, which is the unbounded Wait this whole file exists to close",
			helperIOGrace)
	}
	if helperIOGrace >= helperCommandTimeout {
		t.Errorf("helperIOGrace (%v) must stay below helperCommandTimeout (%v) -- "+
			"they are additive, so an equal value puts the worst-case hold on "+
			"syverMu back at ~2x the deadline, which is what splitting them fixed",
			helperIOGrace, helperCommandTimeout)
	}
}
