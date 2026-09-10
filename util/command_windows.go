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

func NewCommandForWindowsCmd(name string, arg ...string) *Command {
	//fmt.Println(arg)
	command := new(Command)
	command.name = name

	// cmd.exe has a unique unquoting algorithm
	// provide the full command line in SysProcAttr.CmdLine, leaving Args empty.
	// more information: https://golang.org/pkg/os/exec/#Command
	command.Cmd = exec.Command(name)
	command.Cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CmdLine:       strings.Join(arg, " "),
		CreationFlags: 0,
	}

	return command
}

// NewCommandForWindowsCmdContext is NewCommandForWindowsCmd with a context
// attached, so cancelling the context kills the child. See NewCommandContext in
// command.go for why the `command:` resource needs this.
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

// NewCommandForWindowsPowershellContext is NewCommandForWindowsPowershell with
// a context attached. See NewCommandContext in command.go, and
// runHelperCommand in system/helper_command.go for what uses it: the service
// checks shell out to Get-Service, and a Get-Service that never returns used to
// have nothing able to interrupt it.
func NewCommandForWindowsPowershellContext(ctx context.Context, name string, arg ...string) *Command {
	command := new(Command)
	command.name = "powershell"

	cmdLine := "-NoProfile -Command " + name
	if len(arg) > 0 {
		cmdLine += " " + strings.Join(arg, " ")
	}

	command.Cmd = exec.CommandContext(ctx, "powershell")
	command.Cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CmdLine:       cmdLine,
		CreationFlags: 0,
	}
	// Same reason as NewCommandForWindowsCmdContext. system/service_windows.go
	// carries a comment saying this path has no process-group protection at
	// all; this is the line that makes that comment obsolete, and that comment
	// is updated to match.
	configureProcessGroup(command.Cmd)

	return command
}

func NewCommandForWindowsPowershell(name string, arg ...string) *Command {
	command := new(Command)
	command.name = "powershell"

	cmdLine := "-NoProfile -Command " + name
	if len(arg) > 0 {
		cmdLine += " " + strings.Join(arg, " ")
	}

	command.Cmd = exec.Command("powershell")
	command.Cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CmdLine:       cmdLine,
		CreationFlags: 0,
	}

	return command
}
