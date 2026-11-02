//go:build windows
// +build windows

package util

import (
	"context"
	"strings"

	//"fmt"
	"os/exec"
	"syscall"
)

// NewCommandForWindowsCmdContext builds a cmd.exe command bound to ctx, so
// cancelling the context kills the child. See NewCommandContext in command.go
// for why the `command:` resource needs this.
func NewCommandForWindowsCmdContext(ctx context.Context, name string, arg ...string) *Command {
	command := new(Command)
	command.name = name

	// cmd.exe has a unique unquoting algorithm
	// provide the full command line in SysProcAttr.CmdLine, leaving Args empty.
	// more information: https://golang.org/pkg/os/exec/#Command
	command.Cmd = exec.CommandContext(ctx, name)
	command.Cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CmdLine:       strings.Join(arg, " "),
		CreationFlags: 0,
	}
	// AFTER SysProcAttr, which is assigned wholesale just above and would
	// otherwise discard anything set before it. This constructor is the
	// `command:` path on Windows and does not go through NewCommandContext, so
	// without this line the Job Object hook never runs on the one path whose
	// grandchildren leak.
	configureProcessGroup(command.Cmd)

	return command
}
