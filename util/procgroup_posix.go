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
//
// KNOWN LIMITATION, verified rather than assumed: this reaches the group, and a
// process that has left the group is out of reach. setsid, nohup and most
// daemons start a new session precisely so that signals to the old group miss
// them. Measured on this tree: `setsid sleep 4242 &` behind a 500ms timeout was
// still alive, already reparented, two seconds after the command reported the
// timeout, and outlived the run.
//
// There is no portable fix. The escaped process is in a session syver never
// learns the id of, so there is nothing to signal; finding it again would mean
// cgroup or job-object tracking, which is per-platform and much larger than the
// bound this file provides. The cost is one process, one goroutine and two fds
// per timed-out command that daemonises -- bounded per event, unbounded over the
// lifetime of `serve`. Documented for users at docs/gossfile.md's command
// timeout note.
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
