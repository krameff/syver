package system

import "fmt"

// windowsDriveRoot turns a Windows mountpoint into the drive root the Win32
// volume calls take: `c:`, `C:`, `c:\` and `C:/` all become `C:\`.
//
// It carries no build tag, although only mount_windows.go calls it, so the
// parsing is tested on every platform rather than only where a Windows runner
// is available.
//
// Anything that is not a drive letter is refused with an error unwrapping to
// ErrMountUnsupported rather than ErrMountpointNotFound. Windows can also mount
// a volume at a folder path, but mount: covers drive letters only (the API that
// would enumerate folder mount points is FindFirstVolumeMountPoint), and
// reporting such a path as missing would blame the operator for a scope limit.
func windowsDriveRoot(mountpoint string) (string, error) {
	s := mountpoint
	if len(s) == 3 && (s[2] == '\\' || s[2] == '/') {
		s = s[:2]
	}
	if len(s) != 2 || s[1] != ':' || !isASCIILetter(s[0]) {
		return "", &mountpointFormError{mountpoint: mountpoint}
	}
	letter := s[0]
	if letter >= 'a' {
		letter -= 'a' - 'A'
	}
	return string(letter) + `:\`, nil
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// mountpointFormError reports a Windows mountpoint that is not a drive letter.
type mountpointFormError struct {
	mountpoint string
}

func (e *mountpointFormError) Error() string {
	return fmt.Sprintf("mount: %q is not supported on Windows, which covers drive letters such as C: only, not folder mount points", e.mountpoint)
}

func (e *mountpointFormError) Unwrap() error { return ErrMountUnsupported }
