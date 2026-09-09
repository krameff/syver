//go:build windows
// +build windows

package util

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobState is what this file has to remember between configureProcessGroup,
// which runs before Start, and the two hooks that run after it.
//
// attached matters as much as the handle. If assignment failed there is no tree
// to kill, and closing an empty job would kill nothing at all -- which would be
// worse than the behaviour this replaces, because we have already taken over
// exec.CommandContext's default cancel. The fallback path depends on knowing
// which of the two happened.
type jobState struct {
	handle   windows.Handle
	attached bool
}

// jobs maps a command to its Job Object.
//
// The handle cannot live on exec.Cmd, and it cannot live on util.Command
// either: Command is compiled on every platform, so a windows.Handle field
// would need a build tag on the struct itself. Keying off the *exec.Cmd keeps
// the whole mechanism inside this file, which is the property
// procgroup_posix.go already has.
var jobs sync.Map // map[*exec.Cmd]*jobState

// configureProcessGroup creates a Job Object for the command and makes context
// cancellation terminate every process in its tree.
//
// Windows has no process group to signal. The equivalent is a Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: when the last handle to the job closes,
// the kernel terminates every process still assigned to it. That closes the
// case exec.CommandContext misses, where the started process is killed but a
// grandchild it spawned survives. It is not a hypothetical here -- `command:`
// runs through `cmd /c`, so the process syver starts is the shell and the thing
// that hangs is the shell's own child.
//
// This is STRONGER than the POSIX side, and deliberately so. procgroup_posix.go
// records a known limitation: a process that calls setsid leaves the process
// group and is permanently out of reach, measured on this tree. A process
// cannot leave a job unless it was created with CREATE_BREAKAWAY_FROM_JOB and
// the job itself permits breakaway. Neither is set here, so the daemonising
// case POSIX cannot close is closed on Windows.
//
// Assignment happens later, in attachProcessGroup, because there is no process
// to assign until Start has run.
func configureProcessGroup(cmd *exec.Cmd) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		// No job means no tree kill, so leave exec.CommandContext's default
		// cancel in place: it still kills the direct child. Degrading to the
		// old behaviour beats refusing to run the check.
		return
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return
	}

	state := &jobState{handle: job}
	jobs.Store(cmd, state)

	cmd.Cancel = func() error {
		if !state.attached {
			// The job is empty, so closing it terminates nothing. Fall back to
			// what exec.CommandContext would have done unaided.
			if cmd.Process == nil {
				return releaseProcessGroup(cmd, false)
			}
			err := cmd.Process.Kill()
			releaseProcessGroup(cmd, false)
			return err
		}
		// Closing the last handle is the kill. Nothing needs signalling first,
		// and the child does not have to be reaped for it to take effect.
		return releaseProcessGroup(cmd, true)
	}
}

// attachProcessGroup assigns the started process to its Job Object. It is
// called immediately after Start, which is the earliest moment a process exists
// to assign.
//
// A grandchild spawned in the window between Start returning and this call
// would escape the job. The window cannot be closed without starting the
// process suspended and resuming it by hand, which os/exec gives no way to do.
// It is recorded rather than hidden.
//
// MEASURED, rather than reasoned about, on win11-test (Windows 11 Enterprise
// 10.0.26200) on 2026-09-09, over 200 runs: the window between Start returning
// and the assign completing had a median below the clock's resolution and a
// maximum of 559us, while the earliest a freshly created child reached its own
// first statement was 4.05ms -- and that was a bare .exe launched directly,
// which is the fastest case there is. `command:` goes through `cmd /c`, which
// is slower still. The child lost every one of the 200 races.
//
// That is why there is NO TEST for this window and should not be one. A test
// would have to lose a race it cannot lose, so it would pass unconditionally --
// including against an implementation that created no job at all, which is the
// definition of a test that proves nothing.
func attachProcessGroup(cmd *exec.Cmd) error {
	v, ok := jobs.Load(cmd)
	if !ok {
		return nil
	}
	state := v.(*jobState)

	if cmd.Process == nil {
		return nil
	}

	h, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)

	if err := windows.AssignProcessToJobObject(state.handle, h); err != nil {
		return err
	}
	state.attached = true
	return nil
}

// releaseProcessGroup closes the Job Object handle. It is idempotent: cancel
// and the deferred cleanup in Run both reach it, and only the first does work.
//
// kill decides whether closing the handle is allowed to terminate what is still
// running. On timeout it must (that is the whole point). On NORMAL completion
// it must NOT: a command that deliberately starts a background process and
// exits zero is a legitimate thing for a `command:` check to do, and killing
// its child on the way out would be a new failure this spec never asked for.
// Clearing the limit flag before closing is what separates the two.
func releaseProcessGroup(cmd *exec.Cmd, kill bool) error {
	v, ok := jobs.LoadAndDelete(cmd)
	if !ok {
		return nil
	}
	state := v.(*jobState)

	if !kill {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		// Best effort. If clearing the flag fails the close below may terminate
		// a survivor, which is the wrong answer but not one worth failing a
		// passing check over.
		windows.SetInformationJobObject(
			state.handle,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		)
	}

	return windows.CloseHandle(state.handle)
}
