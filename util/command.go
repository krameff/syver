package util

import (
	"bytes"
	"context"

	//"fmt"
	"os/exec"
	"syscall"
)

type Command struct {
	name           string
	Cmd            *exec.Cmd
	Stdout, Stderr bytes.Buffer
	Err            error
	Status         int
}

// NewCommand builds a context-free command: nothing can interrupt its Run.
//
// Nothing in syver calls it any more, and new code should not. Every internal
// caller was moved to NewCommandContext because a context-free child has no
// cancellation and no bound, which under `serve` wedges the health endpoint for
// the life of the process -- see system/helper_command.go. It stays exported
// because it is part of the package's published surface.
func NewCommand(name string, arg ...string) *Command {
	//fmt.Println(arg)
	command := new(Command)
	command.name = name
	command.Cmd = exec.Command(name, arg...)

	return command
}

// NewCommandContext is NewCommand with a context attached, so that cancelling
// the context kills the child process.
//
// Only the `command:` resource needs this, because it is the only caller that
// imposes a timeout. runCommand (system/command.go) used to return on timeout
// while leaving the child running and its goroutine parked in Wait: the process
// was never signalled, so a command that hangs leaked one process and one
// goroutine every time it ran. Under `serve` that is once per cache refresh for
// the life of the daemon.
//
// Killing via the context rather than calling Process.Kill from the timeout
// goroutine avoids racing on Cmd.Process, which Start writes and Wait consumes.
func NewCommandContext(ctx context.Context, name string, arg ...string) *Command {
	command := new(Command)
	command.name = name
	command.Cmd = exec.CommandContext(ctx, name, arg...)
	configureProcessGroup(command.Cmd)

	return command
}

func (c *Command) Run() error {
	c.Cmd.Stdout = &c.Stdout
	c.Cmd.Stderr = &c.Stderr

	// FIRST, and before the LookPath early return below: on Windows
	// configureProcessGroup has already created a Job Object handle, and every
	// path out of this function has to give it back. Releasing is idempotent and
	// a no-op when nothing was registered, so this is safe on the paths that
	// never start a process and on POSIX, where both hooks do nothing.
	//
	// kill=false because this is the ORDINARY exit. Terminating the tree is
	// cmd.Cancel's job and happens only on a timeout; doing it here would kill a
	// background process a successful command deliberately started.
	defer releaseProcessGroup(c.Cmd, false)

	if _, err := exec.LookPath(c.name); err != nil {
		c.Err = err
		return c.Err
	}

	if err := c.Cmd.Start(); err != nil {
		c.Err = err
		return c.Err
	}

	// A no-op unless configureProcessGroup registered something, so a
	// context-free NewCommand is unaffected. On Windows this is where the
	// process joins its Job Object -- the earliest point at which a process
	// exists to assign. An assignment failure is not a check failure: the
	// command ran, and cmd.Cancel falls back to killing the direct child.
	_ = attachProcessGroup(c.Cmd)

	if err := c.Cmd.Wait(); err != nil {
		c.Err = err
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				c.Status = status.ExitStatus()
			}
		}
	} else {
		c.Status = 0
	}
	return c.Err
}
