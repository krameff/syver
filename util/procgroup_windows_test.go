//go:build windows
// +build windows

package util

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTimeoutKillsGrandchild is the FEAT-017 acceptance criterion, and it is
// the reason the Job Object exists rather than being a tidier way to spell the
// old no-op.
//
// The shape matters. `command:` runs through `cmd /c`, so the process syver
// starts is a shell; exec.CommandContext kills that shell and stops. Anything
// the shell launched with `start /b` is detached from it and, before this
// change, outlived the whole run. That is the FEAT-008 leak, which was closed
// on POSIX and left open here.
//
// Liveness is probed with a marker file rather than by enumerating processes.
// Reading another process's command line on Windows needs WMI or
// NtQueryInformationProcess, and a test that needs its own privileged lookup to
// decide whether it passed is a test that can be wrong in two places. A file
// that does or does not appear cannot be misread.
//
// REVERT-PROOF: with configureProcessGroup restored to its old empty body the
// grandchild survives the timeout, writes the marker, and this fails. Confirm
// that before trusting it.
func TestTimeoutKillsGrandchild(t *testing.T) {
	if testing.Short() {
		t.Skip("waits on a real timeout and a survival window; not a -short test")
	}

	dir := t.TempDir()
	started := filepath.Join(dir, "grandchild-started.txt")
	survived := filepath.Join(dir, "grandchild-survived.txt")

	// TWO markers, and the first one is what stops this test proving nothing.
	//
	// Asserting only that "survived" is absent passes just as happily when the
	// grandchild was killed as when it NEVER RAN -- a typo in the script, a
	// `start /b` that failed, a cmd.exe that is not where we thought. Both look
	// identical from the outside: no file. So the grandchild announces itself
	// BEFORE it waits, and the test requires that announcement. Absence of
	// "survived" only means anything once "started" proves there was something
	// to kill.
	//
	// The parent shell launches it detached and then blocks itself, so the run
	// reaches its timeout with both alive. ping is the wait: present on every
	// Windows host, and no quoting gymnastics through SysProcAttr.CmdLine.
	script := "start /b cmd /c \"echo started > " + started +
		" & ping -n 8 127.0.0.1 >nul & echo survived > " + survived + "\"" +
		" & ping -n 8 127.0.0.1 >nul"

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	start := time.Now()
	cmd := NewCommandForWindowsCmdContext(ctx, "cmd", "/c", script)
	_ = cmd.Run() // a killed command reports an error; that is not what is under test

	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("Run took %v, so the command was not interrupted by the context; "+
			"the test has not exercised the timeout path at all", elapsed)
	}

	// Outlive the grandchild's own wait. If it is alive, this is when it writes.
	time.Sleep(12 * time.Second)

	// Order matters: establish that there WAS a grandchild before concluding
	// anything from the absence of the second marker.
	if _, err := os.Stat(started); err != nil {
		t.Fatalf("the grandchild never announced itself (%v), so this test has "+
			"proved nothing about killing it -- fix the script rather than "+
			"reading the missing survival marker as success", err)
	}

	if _, err := os.Stat(survived); err == nil {
		t.Fatalf("grandchild wrote %s after the parent was killed, so it survived "+
			"the timeout: the process tree was not terminated", survived)
	} else if !os.IsNotExist(err) {
		t.Fatalf("cannot tell whether the grandchild survived: %v", err)
	}
}

// TestNormalExitDoesNotKillSurvivors pins the boundary the kill-on-close flag
// would otherwise cross, and it is the reason releaseProcessGroup takes a kill
// argument at all.
//
// A `command:` that deliberately starts something and exits zero is legitimate.
// Closing the Job Object handle on that path would terminate what it started,
// which is a NEW failure this spec never asked for and which no existing test
// would have caught. Clearing the limit flag before the close is what separates
// "the command timed out" from "the command finished".
//
// REVERT-PROOF: drop the `if !kill` branch in releaseProcessGroup and this
// fails while TestTimeoutKillsGrandchild still passes. The two together are
// what pin the behaviour; either alone permits a wrong implementation.
func TestNormalExitDoesNotKillSurvivors(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for a background process to outlive its parent")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "background-finished.txt")

	// The shell launches a background task and exits immediately, succeeding.
	script := "start /b cmd /c \"ping -n 6 127.0.0.1 >nul & echo finished > " + marker + "\""

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := NewCommandForWindowsCmdContext(ctx, "cmd", "/c", script)
	if err := cmd.Run(); err != nil {
		t.Fatalf("the command should have succeeded: %v", err)
	}

	time.Sleep(10 * time.Second)

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("a background process started by a SUCCESSFUL command was killed "+
			"when its handle was released (%v); only a timeout may terminate the tree", err)
	}
}
