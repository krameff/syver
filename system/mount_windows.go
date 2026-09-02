//go:build windows
// +build windows

package system

import "errors"

// errNotImplemented is currently UNREACHABLE in practice (FEAT-010 SW-7,
// deliberate, documented deferral -- not missed). system/mount.go's
// setup() calls getMount() first; on Windows the vendored mountinfo
// implementation returns an empty table rather than an error (its own
// comment says "Do NOT return an error!"), which getMount converts to
// ErrMountpointNotFound before getUsage (this function) is ever reached.
// So every Windows mount check fails with "mountpoint not found" --
// misleading (it blames the mountpoint, not the missing implementation)
// but loud, which this spec's own priority order (a misleading-but-loud
// error is a far smaller problem than a silent pass) treats as acceptable
// to leave as-is here.
//
// Making this reachable means reordering getMount, which is cross-platform
// code shared by every OS -- not "obviously safe" to restructure inside
// this spec's Windows-focused, Linux-verified scope, so it is left as-is.
var errNotImplemented = errors.New("not implemented")

func getUsage(mountpoint string) (int, error) {
	return 0, errNotImplemented
}
