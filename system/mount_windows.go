//go:build windows
// +build windows

package system

// mountSupported reports that Windows has no mount lookup.
//
// Until FEAT-013 this package could not say so. system/mount.go's setup()
// called getMount first, and on Windows the vendored mountinfo returns an
// EMPTY TABLE rather than an error -- its own comment says "Do NOT return an
// error!" -- which getMount converted to ErrMountpointNotFound. Every Windows
// mount check therefore failed by blaming the operator's mountpoint for what
// is actually a missing implementation, and the honest error below was
// unreachable. Recorded as FEAT-010 SW-7, scheduled as FEAT-011 W2-12(a).
//
// setup() now consults this before getMount, so the answer is truthful. See
// ErrMountUnsupported in mount.go for why the fix is a capability check rather
// than a reordering of the shared code path.
//
// A real Windows backend is FEAT-011 W2-12(b), specced as FEAT-018:
// GetLogicalDriveStringsW, GetVolumeInformationW and GetDiskFreeSpaceExW.
// `opts` and `source` have no Windows meaning and stay unsupported rather than
// being faked.
func mountSupported() error { return ErrMountUnsupported }

// getUsage is unreachable while mountSupported returns an error, and is kept
// so the platform still satisfies the same shape as mount_posix.go. FEAT-018
// replaces its body.
func getUsage(mountpoint string) (int, error) {
	return 0, ErrMountUnsupported
}
