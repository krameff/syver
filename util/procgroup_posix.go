//go:build !windows
// +build !windows

package util

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the child into its own process group and makes
// context cancellation kill that entire group rather than just the child.
//
// Killing only the direct child is not enough for the `command:` resource.
// Commands run through `sh -c`, so the process we start is the shell and the
// thing that actually hangs is the shell's own child. exec.CommandContext's
// default cancel kills the shell alone, which leaves that child running and
// reparented to init -- measurably so: a `sleep 300` behind a 1.5s timeout
// survived the run until this was added.
func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid addresses the whole process group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
