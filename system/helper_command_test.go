//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// hangingShim puts an executable named `name` at the front of PATH which never
// returns. It spawns a child rather than sleeping itself, so a fix that only
// signals the direct child would still leave something behind -- the shims
// syver invokes here are real programs that fork (systemctl talks to a bus,
// dpkg-query takes a lock), and killing only the process we started is the
// failure mode util.configureProcessGroup exists to prevent.
func hangingShim(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	// The shim touches a sentinel before sleeping. LookPath only checks the mode
	// bits, so a shim that resolves correctly and then FAILS TO EXECUTE -- a
	// noexec TMPDIR, a missing interpreter -- passes the resolution check below
	// and still yields "probe reported <nil>" for 6 of the 11 probes, whose
	// checks treat an exec failure as "absent". That is the same string as a
	// PATH failure, so without this the diagnostic closes only half the case.
	ran := filepath.Join(dir, name+".ran")
	script := "#!/bin/sh\n: >" + ran + "\nsleep 3600\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Confirm the shim is what actually resolves, rather than assuming the PATH
	// prepend took. systemctl and getent exist on most Linux hosts including CI
	// runners, so if the real binary wins it answers in milliseconds and the
	// probe returns nil -- which surfaces as the deeply unhelpful "probe reported
	// <nil>, want context.DeadlineExceeded" and looks like a timing flake rather
	// than a PATH problem. Say which it is.
	resolved, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s: shim written to %s but not resolvable on PATH: %v", name, dir, err)
	}
	// filepath.Clean(dir), not dir. LookPath builds its result with filepath.Join,
	// which cleans; t.TempDir() does not -- os.TempDir() strips only TRAILING
	// slashes, so a TMPDIR containing "//", "/./" or ".." yields a non-clean dir
	// and this compares a real path against lexical noise. Verified: TMPDIR with
	// a "/./" segment failed every subtest while the shim had in fact won.
	if filepath.Dir(resolved) != filepath.Clean(dir) {
		t.Fatalf("%s resolved to %s, not the hanging shim in %s -- the real binary "+
			"would answer immediately and the probe would report nil, which is NOT a "+
			"timing failure", name, resolved, dir)
	}

	// Registered AFTER the resolution check above, deliberately. If the shim did
	// not resolve, that check Fatalf's here and this cleanup is never registered
	// -- otherwise a PATH failure fired BOTH diagnostics and they contradicted
	// each other ("not the hanging shim" vs "resolved but never executed"), both
	// insisting they were not a timing failure. Registering after t.TempDir()
	// also puts this Stat before the directory is removed, LIFO. Both orderings
	// are load-bearing.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if _, err := os.Stat(ran); err != nil {
			t.Errorf("%s: the shim at %s resolved but never executed (no %s) -- exec "+
				"failed, e.g. a noexec TMPDIR or a bad interpreter. NOT a timing failure",
				name, resolved, ran)
		}
	})
}

// probe is one system-layer check that shells out to an internal helper. Each
// returns the error the check reported, which is the thing under test: a check
// that cannot run must say so, not answer "absent" with a straight face.
type probe struct {
	// shim is the helper binary the probe execs, and so the one to hang.
	shim string
	run  func(ctx context.Context) error
}

func helperProbes() []probe {
	return []probe{
		{"systemctl", func(ctx context.Context) error {
			_, err := NewServiceSystemd(ctx, "sshd", nil, util.Config{}).Running()
			return err
		}},
		{"systemctl", func(ctx context.Context) error {
			_, err := NewServiceSystemd(ctx, "sshd", nil, util.Config{}).Enabled()
			return err
		}},
		{"systemctl", func(ctx context.Context) error {
			_, err := NewServiceSystemd(ctx, "sshd", nil, util.Config{}).Exists()
			return err
		}},
		{"service", func(ctx context.Context) error {
			_, err := NewServiceInit(ctx, "sshd", nil, util.Config{}).Running()
			return err
		}},
		{"service", func(ctx context.Context) error {
			_, err := NewServiceUpstart(ctx, "sshd", nil, util.Config{}).Running()
			return err
		}},
		{"rpm", func(ctx context.Context) error {
			_, err := NewRpmPackage(ctx, "bash", nil, util.Config{}).Installed()
			return err
		}},
		{"dpkg-query", func(ctx context.Context) error {
			_, err := NewDebPackage(ctx, "bash", nil, util.Config{}).Installed()
			return err
		}},
		{"apk", func(ctx context.Context) error {
			_, err := NewAlpinePackage(ctx, "bash", nil, util.Config{}).Installed()
			return err
		}},
		{"pacman", func(ctx context.Context) error {
			_, err := NewPacmanPackage(ctx, "bash", nil, util.Config{}).Installed()
			return err
		}},
		// getent only runs when the local passwd/group lookup misses, so these
		// need ids nothing on the host can resolve. That miss-then-shell-out
		// shape is also why this is the slowest legitimate helper: on a host
		// with a remote nsswitch backend it is a network round trip.
		{"getent", func(ctx context.Context) error {
			_, err := getUserForUid(ctx, 2147483000)
			return err
		}},
		{"getent", func(ctx context.Context) error {
			_, err := getGroupForGid(ctx, 2147483000)
			return err
		}},
	}
}

// runProbe runs p off the calling goroutine so a probe that ignores its context
// fails this test rather than hanging it. A hang is technically a failure too --
// go test panics after its own timeout -- but ten minutes later and with a
// stack dump instead of a sentence, which is not a useful way to learn that a
// call site was missed.
func runProbe(t *testing.T, p probe, ctx context.Context, patience time.Duration) error {
	t.Helper()
	type result struct{ err error }
	done := make(chan result, 1)
	go func() { done <- result{p.run(ctx)} }()

	select {
	case r := <-done:
		return r.err
	case <-time.After(patience):
		t.Fatalf("%s probe did not return within %s: the helper is still running "+
			"context-free (util.NewCommand), so nothing can interrupt it. Under `serve` "+
			"this holds syverMu and wedges every later probe for the life of the process.",
			p.shim, patience)
		return nil
	}
}

// Cancelling the context must stop a helper that has already wedged.
//
// This is the shutdown half of the fix: SIGTERM to a `serve` daemon, or Ctrl-C
// during `validate`, has to reach the child. Threading the context through the
// eleven call sites is the whole mechanism, and each one is a separate plain
// parameter, so any one of them left on util.NewCommand breaks only itself and
// stays green in every other test.
func TestCancellingContextStopsAHangingHelper(t *testing.T) {
	for _, p := range helperProbes() {
		t.Run(p.shim, func(t *testing.T) {
			hangingShim(t, p.shim)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Give the child time to actually start before pulling the rug;
			// cancelling before Start would prove nothing about the wait.
			time.AfterFunc(300*time.Millisecond, cancel)

			// 5s is far below the 30s bound, so a pass here cannot come from
			// helperCommandTimeout reaping the child instead.
			err := runProbe(t, p, ctx, 5*time.Second)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("probe reported %v, want context.Canceled: a check that was "+
					"cancelled mid-flight must report that, not a confident answer it "+
					"never actually obtained", err)
			}
		})
	}
}

// And with nothing cancelling, the helper must still end.
//
// This is the half that matters most in production: a host whose systemd never
// answers or whose rpmdb lock is held is not shutting down, so no cancellation
// is ever coming. Without a deadline the daemon stays wedged until someone
// SIGKILLs it.
func TestHangingHelperIsBoundedWithNoCancellation(t *testing.T) {
	// Not t.Parallel(): this rewrites a package global. Go runs sequential
	// tests to completion before resuming parallel ones, so no other test in
	// this package can observe the shortened value.
	restore := helperCommandTimeout
	helperCommandTimeout = 500 * time.Millisecond
	t.Cleanup(func() { helperCommandTimeout = restore })

	for _, p := range helperProbes() {
		t.Run(p.shim, func(t *testing.T) {
			hangingShim(t, p.shim)

			err := runProbe(t, p, context.Background(), 5*time.Second)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("probe reported %v, want context.DeadlineExceeded", err)
			}
		})
	}
}

// A helper that answers normally must be untouched: same result, no error, and
// no deadline in the way. Without this, "returns promptly with a context error"
// is satisfiable by a change that breaks every check on every healthy host.
func TestHealthyHelperIsUnaffected(t *testing.T) {
	dir := t.TempDir()
	// `is-active` is the third argument; anything else (list-unit-files,
	// is-enabled) must not answer 0, or Exists/Enabled would report nonsense.
	script := "#!/bin/sh\n[ \"$2\" = is-active ] && exit 0\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	running, err := NewServiceSystemd(context.Background(), "sshd", nil, util.Config{}).Running()
	if err != nil {
		t.Fatalf("Running() on a responsive systemctl returned %v, want nil", err)
	}
	if !running {
		t.Error("Running() = false for a systemctl reporting is-active; the exit status is no longer being read")
	}
}
