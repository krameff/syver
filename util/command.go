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

	if _, err := exec.LookPath(c.name); err != nil {
		c.Err = err
		return c.Err
	}

	if err := c.Cmd.Start(); err != nil {
		c.Err = err
		return c.Err
	}

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
