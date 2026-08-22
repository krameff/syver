package system

import (
	"bytes"
	"context"
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

	// Bound the child's lifetime by the same timeout runCommand selects on, so
	// that a command which never returns is actually killed rather than left
	// running with its goroutine parked in Wait. Only applied when a timeout is
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
	if ctxErr := ctx.Err(); ctxErr != nil {
		c.err = ctxErr
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

func runCommand(cmd *util.Command, timeout int) error {
	c1 := make(chan bool, 1)
	e1 := make(chan error, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond
	go func() {
		err := cmd.Run()
		if err != nil {
			e1 <- err
		}
		c1 <- true
	}()
	select {
	case <-c1:
		return nil
	case err := <-e1:
		return err
	case <-time.After(timeoutD):
		return fmt.Errorf("Command execution timed out (%s)", timeoutD)
	}
}
