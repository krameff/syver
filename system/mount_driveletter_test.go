package system

import (
	"errors"
	"strings"
	"testing"
)

// The Windows mountpoint parser runs on every platform, so its rules are held
// by the Linux and macOS jobs too, not only by a Windows runner.

func TestWindowsDriveRootAcceptsDriveLetterSpellings(t *testing.T) {
	for in, want := range map[string]string{
		`c:`:  `C:\`,
		`C:`:  `C:\`,
		`c:\`: `C:\`,
		`C:\`: `C:\`,
		`d:/`: `D:\`,
		`Z:`:  `Z:\`,
	} {
		got, err := windowsDriveRoot(in)
		if err != nil {
			t.Errorf("windowsDriveRoot(%q) error = %v, want %q", in, err, want)
			continue
		}
		if got != want {
			t.Errorf("windowsDriveRoot(%q) = %q, want %q", in, got, want)
		}
	}
}

// A path that is not a drive letter is a scope limit, not a missing mount. It
// must never read as ErrMountpointNotFound, which would send an operator off to
// debug a path that was never going to be looked up.
func TestWindowsDriveRootRefusesOtherPaths(t *testing.T) {
	for _, in := range []string{
		``, `/`, `c`, `1:`, `cd:`, `c:\data`, `C:\mnt\vol`, `\\server\share`, `c:x`,
	} {
		_, err := windowsDriveRoot(in)
		if err == nil {
			t.Errorf("windowsDriveRoot(%q) accepted a non-drive-letter path", in)
			continue
		}
		if !errors.Is(err, ErrMountUnsupported) {
			t.Errorf("windowsDriveRoot(%q) error = %v, want it to unwrap to ErrMountUnsupported", in, err)
		}
		if errors.Is(err, ErrMountpointNotFound) {
			t.Errorf("windowsDriveRoot(%q) error = %v reads as a missing mountpoint", in, err)
		}
		if msg := err.Error(); !strings.Contains(msg, "drive letters") {
			t.Errorf("windowsDriveRoot(%q) error %q does not say drive letters are what is supported", in, msg)
		}
	}
}
