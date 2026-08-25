package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/krameff/syver/util"
)

type Command interface {
	Command() string
	Exists() (bool, error)
	ExitStatus() (int, error)
	Stdout() (io.Reader, error)
	Stderr() (io.Reader, error)
}

type DefCommand struct {
	Ctx        context.Context
	command    string
	exitStatus int
	stdout     io.Reader
	stderr     io.Reader
	loaded     bool
	Timeout    int
	err        error
}

func NewDefCommand(ctx context.Context, command string, system *System, config util.Config) Command {
	return &DefCommand{
		Ctx:     ctx,
		command: command,
		Timeout: config.TimeOutMilliSeconds(),
	}
}

func (c *DefCommand) setup() error {
	if c.loaded {
		return c.err
	}
	c.loaded = true

	// Bound the child's lifetime. This context IS the timeout: runCommand has no
	// timer of its own any more, so a command which never returns is killed here
	// or nowhere. Only applied when a timeout is
	// set: Timeout <= 0 reaches here from callers that pass an empty
	// util.Config, and attaching an already-expired context would change their
	// behaviour rather than fix a leak.
	// Inherit the caller's context rather than starting a fresh root: it is what
	// carries cancellation down from the CLI's signal handler and from the
	// server, so a Ctrl-C or a shutdown kills the child instead of leaving it to
	// run to completion. c.Ctx is nil only for a zero-value DefCommand built
	// outside the normal path.
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Millisecond)
		defer cancel()
	}

	cmd := commandWrapper(ctx, c.command)
	err := runCommand(cmd, c.Timeout)

	// We don't care about ExitError since it's covered by status
	if _, ok := err.(*exec.ExitError); !ok {
		c.err = err
	}
	// ...except when the context killed the child. exec.CommandContext signals
	// the process, so Run returns an ExitError and the branch above discards it,
	// leaving err nil and status -1 (util/command.go reads a signalled process's
	// WaitStatus as -1). The result is a cancelled run rendering as
	// "Expected -1 to be numerically eq 0" -- byte-identical to a binary that
	// genuinely died on a signal. During a graceful shutdown that reads to an
	// operator as the daemon crashing, which is the opposite of what happened.
	// Surfacing ctx.Err() makes every output format say "context canceled".
	//
	// ...but NOT verbatim for a timeout. This synthesis is deliberately unconditional for a deadline: it
	// wins over whatever Run returned, because when the deadline fired the command
	// did time out and the budget is the useful thing to say. Without that
	// precedence, exec's "WaitDelay expired before I/O complete" reached the
	// operator whenever a grandchild held the stdout pipe.
	//
	// History, since the shape here only makes sense with it: runCommand used to
	// carry a second timer on the same duration, so whichever won decided the
	// wording and the same spec reported "Command execution timed out (10s)" on
	// one host and "context deadline exceeded" on another. That timer is gone --
	// see runCommand -- and the message is now produced in exactly one place.
	// Either timer means "this command exceeded its budget", and BOTH must produce
	// the same sentence. WaitDelay's clock starts at process exit, not at the
	// deadline, so for a command whose own process exits early while a grandchild
	// holds the pipe the two expire at almost the same instant -- and whichever
	// wins would otherwise decide the wording. That is the same non-determinism
	// this function was rewritten to remove; it reappeared here as a load-
	// dependent test failure leaking "exec: WaitDelay expired before I/O
	// complete" to the operator.
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(c.err, exec.ErrWaitDelay)
	if ctxErr := ctx.Err(); ctxErr != nil || timedOut {
		if timedOut && c.Timeout > 0 {
			c.err = fmt.Errorf("Command execution timed out (%s)",
				time.Duration(c.Timeout)*time.Millisecond)
		} else if c.err == nil {
			c.err = ctxErr
		}
	}
	c.exitStatus = cmd.Status
	stdoutB := cmd.Stdout.Bytes()
	stderrB := cmd.Stderr.Bytes()
	// Read from the local ctx, not c.Ctx: the nil guard above puts the fallback
	// in ctx, so dereferencing c.Ctx here would panic on exactly the zero-value
	// DefCommand that guard exists for -- and a nil-interface method call is a
	// process kill, since nothing in non-test code recovers.
	//
	// The value has never actually been found: producers set it under
	// resource.idKey{} (an unexported struct type) while this looks up the
	// string "id". Different key types never match, so this has always logged
	// <nil>. idKey is unexported, so system structurally cannot read it; leaving
	// the lookup in place keeps the log line's shape until the key is made
	// reachable, rather than pretending it works.
	id := ctx.Value("id")
	logBytes(stdoutB, fmt.Sprintf("[Command][%s][stdout] ", id))
	logBytes(stderrB, fmt.Sprintf("[Command][%s][stderr] ", id))
	c.stdout = bytes.NewReader(stdoutB)
	c.stderr = bytes.NewReader(stderrB)

	return c.err
}

func (c *DefCommand) Command() string {
	return c.command
}

func (c *DefCommand) ExitStatus() (int, error) {
	err := c.setup()

	return c.exitStatus, err
}

func (c *DefCommand) Stdout() (io.Reader, error) {
	err := c.setup()

	return c.stdout, err
}

func (c *DefCommand) Stderr() (io.Reader, error) {
	err := c.setup()

	return c.stderr, err
}

// Stub out
func (c *DefCommand) Exists() (bool, error) {
	return false, nil
}

// runCommand runs cmd to completion. The caller's context carries the timeout
// and kills the child when it expires, so this needs no timer of its own.
//
// It used to have one: a goroutine running cmd.Run() plus a `select` on
// time.After with the SAME duration as the context. That second timer caused
// three separate defects.
//
//   - A data race. On timeout the select returned while the goroutine was still
//     writing into cmd.Stdout/cmd.Stderr, which setup() reads immediately. It
//     fired for every timed-out command, not only chatty ones, because exec's
//     copy goroutine calls Buffer.ReadFrom, which grows the buffer before any
//     output arrives.
//   - A non-deterministic error message. Two timers on one duration meant
//     whichever won decided the wording, so the same spec reported
//     "Command execution timed out (10s)" on one host and the far less useful
//     "context deadline exceeded" on another.
//   - A dropped error. The goroutine sent its error and its done-signal on two
//     buffered channels; if both were ready the select picked at random and
//     could discard the error. Because util.Command.Run returns early on
//     LookPath/Start failure without assigning Status, a discarded error meant a
//     command that never executed reported exit status 0 -- a spec asserting
//     `exit-status: 0` would have PASSED.
//
// All three are structural in that design, and all three vanish here rather than
// being patched: no goroutine, no second timer, no channels.
//
// WaitDelay is the piece that makes this safe where a hand-rolled bound was not.
// A grandchild that escapes the process group (setsid, nohup, a daemonising
// service) inherits the stdout pipe, so Wait blocks on the copy goroutine. A
// bound that gives up and returns leaves that goroutine writing into buffers the
// caller is already reading -- measured at 5 races. WaitDelay instead force-
// closes the pipes and guarantees the I/O goroutines are done before Wait
// returns. The cost is that the delay can add to the deadline: worst case ~2x
// the configured timeout, in the shape where the process exits partway through
// its budget while a grandchild still holds the pipe. Bounded, and the number is
// the user's own budget.
//
// The bound is the caller's context plus a WaitDelay derived from the same
// timeout -- see the body for why deriving it matters rather than picking a
// constant.
func runCommand(cmd *util.Command, timeout int) error {
	// WaitDelay is DERIVED from the command's own budget, not a constant. A fixed
	// value smaller than the budget silently overrides it: with the default
	// `timeout: 10000` (resource/command.go:73-74) and a 2s constant, a command
	// whose grandchild holds the stdout pipe returned at 2s with the Go-internal
	// string "exec: WaitDelay expired before I/O complete" -- ignoring the
	// configured budget and leaking stdlib wording to the operator. That is the
	// same class of defect this function was rewritten to remove.
	//
	// Worst case is ~2x timeout in one shape: the process exits partway through
	// its budget while a grandchild still holds the pipe, so the deadline and the
	// delay run back to back. Bounded, and no worse than the constant was for any
	// budget below it.
	cmd.Cmd.WaitDelay = time.Duration(timeout) * time.Millisecond
	return cmd.Run()
}
