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
	cmd.Run()

	if err := ctx.Err(); err != nil {
		return cmd, err
	}
	return cmd, nil
}
