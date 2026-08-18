//go:build windows
// +build windows

package util

import "os/exec"

// configureProcessGroup is a deliberate no-op on Windows.
//
// There is no process-group kill equivalent; terminating a whole tree needs a
// Job Object, which is a larger change than this fix warrants and cannot be
// tested from here. Windows therefore keeps exec.CommandContext's default
// behaviour: the started process is killed on timeout, but a grandchild it
// spawned can still survive. Windows support is alpha (see docs/platforms.md).
func configureProcessGroup(cmd *exec.Cmd) {}
