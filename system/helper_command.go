package system

import (
	"context"
	"time"

	"github.com/krameff/syver/util"
)

// helperCommandTimeout bounds every internal helper subprocess syver shells out
// to: systemctl, service, rpm, dpkg-query, apk, pacman, getent.
//
// It is a fixed constant rather than a user-facing knob, and that is deliberate.
// The `command:` resource has a per-resource `timeout:` because the thing being
// run is the user's, and only they can say how long it should take. These are
// syver's own argv -- the only user-supplied part is a service or package name
// in a fixed position -- and how long they take is a property of the host's
// tooling, not of the spec. On a healthy host every one of them returns in well
// under a second; the slowest legitimate case is `getent` against a remote
// nsswitch backend (LDAP/NIS), where glibc's own bind and search timeouts are
// themselves in the tens of seconds. 30s clears that with room to spare while
// still being a bound, which is the entire point: before this, there was none.
//
// A var, not a const, so tests can shorten it. Nothing outside this package
// writes it.
var helperCommandTimeout = 30 * time.Second

// helperIOGrace is how long Wait may spend draining output AFTER syver has given
// up on the helper -- a separate concern from helperCommandTimeout, which is how
// long the helper gets to answer at all.
//
// Deliberately NOT derived from helperCommandTimeout, and that is a departure
// worth explaining, because the `command:` path derives its WaitDelay and a
// fixed constant there was a real defect (system/command.go:205-216).
//
// The reason that rule does not carry over: on the `command:` path the number
// being overridden is the USER'S, set per-resource via `timeout:`, and a
// constant below it silently overrules what they asked for. Here there is no
// user-supplied budget -- helperCommandTimeout is syver's own fixed 30s for its
// own argv -- so there is nothing to overrule.
//
// And the two do different jobs. The deadline decides a host tool has stopped
// answering. The grace only drains pipes once that decision is made, which for
// any process that has actually exited takes microseconds; it runs long only
// when a grandchild is holding the pipe open, which is exactly the case syver
// wants to abandon quickly rather than wait out. Deriving it made the worst case
// 2 x 30s = 60s of held syverMu, on an endpoint whose whole job is to answer
// promptly: default Kubernetes probe settings (single-digit periodSeconds,
// failureThreshold 3) can restart a healthy container inside that window, which
// would be an outage caused by the health check itself. 5s brings the worst case
// to ~35s while leaving far more drain time than a live process ever needs.
//
// A var, not a const, so tests can vary it.
var helperIOGrace = 5 * time.Second

// runHelperCommand runs one of those helpers with the caller's context and a
// bounded lifetime, and reports whether the context ended the run.
//
// Two separate failure modes are being closed here, and both were live:
//
//   - No cancellation. `util.NewCommand` produces a context-free `exec.Cmd`, so
//     Run's `Wait` had nothing to interrupt it. Under `serve` that is not one
//     slow request: fillCache holds syverMu for the duration of the sweep, and
//     WriteTimeout is deliberately unset (see serve.go), so a single wedged
//     helper parks every subsequent probe behind the mutex forever. The daemon
//     then needs SIGKILL, and the helper outlives it.
//   - No bound. Even with cancellation wired up, nothing cancels on a host that
//     is merely stuck rather than shutting down -- a systemd that never answers,
//     an rpmdb lock nobody releases. The deadline is what makes such a check
//     fail as a check instead of taking the process with it.
//
// The deadline is derived from the caller's context rather than started fresh,
// so both apply: a shutdown or Ctrl-C kills the child immediately, and a wedge
// with no shutdown still ends at helperCommandTimeout. Deriving from the
// `add`/fromSystem path's context.Background() is safe -- Background is never
// cancelled, so the only thing that changes there is that `syver add` can no
// longer hang forever on a stuck helper either.
//
// The returned error is the context's, never the command's. Callers already
// have their own conventions for what a non-zero exit or a missing binary
// means (usually "absent", quietly), and rewriting those would change output on
// healthy hosts. This only tells them the run did not happen -- see the
// matching ctx.Err() block in command.go for why that has to be said out loud
// rather than inferred from the exit status: exec.CommandContext signals the
// child, so a killed helper reports status -1 and reads as a genuine crash.
func runHelperCommand(ctx context.Context, name string, arg ...string) (*util.Command, error) {
	// Callers reach here from constructors that take a context, but the
	// constructors are exported and a zero-value struct built outside the normal
	// path has a nil one. context.WithTimeout panics on a nil parent, and nothing
	// in non-test code recovers a panic in system.
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helperCommandTimeout)
	defer cancel()

	cmd := util.NewCommandContext(ctx, name, arg...)
	// The context bounds the PROCESS. It does not bound Wait.
	//
	// Cancelling kills the helper, but exec's copy goroutine stays blocked for as
	// long as ANYTHING holds the write end of the stdout pipe -- and a grandchild
	// that escaped the process group (setsid, nohup, a service that daemonises)
	// inherits it and holds it for its own lifetime. Wait then returns when the
	// GRANDCHILD exits, not when the budget expires. Measured before this line
	// existed: a 1s budget did not return within 15s.
	//
	// Under `serve` that is not one slow probe. fillCache holds syverMu for the
	// sweep and WriteTimeout is deliberately unset (see serve.go), so one wedged
	// helper parks every subsequent request behind the mutex for the life of the
	// process -- reachable from an unauthenticated /healthz against any spec whose
	// service or package check shells out. That is precisely the failure this file
	// was written to prevent; the context closed the cancellation half and left
	// the I/O half open.
	//
	// helperIOGrace, not helperCommandTimeout -- see that var's comment for why
	// this path deliberately does NOT follow the `command:` path's derive-it rule.
	//
	// WaitDelay is synchronising even when it expires: os/exec closes the parent
	// pipes and then blocks on the copy goroutines before returning ErrWaitDelay
	// (go1.26.6 os/exec/exec.go:867-872, for go.dev/issue/23019). So the buffers
	// this returns are never read while still being written.
	//
	// Worst case is helperCommandTimeout + helperIOGrace, not either alone.
	// WaitDelay's timer starts at whichever comes first, the context being done or
	// the process exiting, so a helper that runs its full 30s and then leaves a
	// grandchild holding the pipe takes the grace on top: ~35s of held syverMu,
	// reachable unauthenticated. Bounded where it used to be infinite, and stated
	// here rather than left to be rediscovered. docs/gossfile.md tells users the
	// same number.
	//
	// system/service_windows.go's runHelperPowershell is a near-duplicate of this
	// function and needs the same line. Change one, change both.
	cmd.Cmd.WaitDelay = helperIOGrace
	cmd.Run()

	if err := ctx.Err(); err != nil {
		return cmd, err
	}
	return cmd, nil
}
