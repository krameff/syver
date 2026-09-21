package system

import (
	"errors"
	"strings"
)

type Service interface {
	Service() string
	Exists() (bool, error)
	Enabled() (bool, error)
	Running() (bool, error)
	RunLevels() ([]string, error)
	// The six below are FEAT-014 and describe a Windows service. Every non-Windows
	// backend gets them from windowsOnlyServiceAttrs, which returns
	// ErrServiceWindowsAttrUnsupported -- see that type.
	StartType() (string, error)
	DelayedStart() (bool, error)
	RunAs() (string, error)
	Dependencies() ([]string, error)
	DisplayName() (string, error)
	Pid() (int, error)
}

// ErrServiceNotFound is returned by Enabled and Running on Windows
// (system/service_windows.go) when the named service does not exist, instead
// of folding "does not exist" into "exists but is disabled/not running" --
// the two used to be indistinguishable, so `service: TypoedName: {enabled:
// false}` passed. See FEAT-010 SW-4.
var ErrServiceNotFound = errors.New("service not found")

// ErrServiceRunLevelsUnsupported is returned by RunLevels on Windows
// (system/service_windows.go), which has no SysV/systemd-style runlevel
// concept. RunLevels previously returned (nil, nil) there, so a spec ported
// from Linux that sets `runlevels:` got a ContainElements failure against a
// nil slice with no indication Windows was the reason. See FEAT-010 S-5.
var ErrServiceRunLevelsUnsupported = errors.New("service runlevels is not supported on this platform")

// ErrServiceWindowsAttrUnsupported is returned by the six Windows service
// attributes on every other platform. FEAT-014.
//
// A sentinel rather than a zero value, for the reason FEAT-010 established across
// this package: `start-type: automatic` on a Linux host must FAIL, loudly, rather
// than compare against "" and report something the system was never asked. A
// gossfile that sets a Windows-only attribute on Linux is a spec bug, and the
// tool's job is to say so.
var ErrServiceWindowsAttrUnsupported = errors.New("service start-type/delayed-start/run-as/dependencies/display-name/pid are Windows-only")

// windowsOnlyServiceAttrs supplies those six accessors for ServiceInit,
// ServiceUpstart and ServiceSystemd by embedding, rather than repeating eighteen
// near-identical stubs across three files. Adding a seventh Windows attribute
// means one method here, not four.
//
// Deliberately NOT given to ServiceWindows, which implements all six for real.
type windowsOnlyServiceAttrs struct{}

func (windowsOnlyServiceAttrs) StartType() (string, error) {
	return "", ErrServiceWindowsAttrUnsupported
}

func (windowsOnlyServiceAttrs) DelayedStart() (bool, error) {
	return false, ErrServiceWindowsAttrUnsupported
}

func (windowsOnlyServiceAttrs) RunAs() (string, error) {
	return "", ErrServiceWindowsAttrUnsupported
}

func (windowsOnlyServiceAttrs) Dependencies() ([]string, error) {
	return nil, ErrServiceWindowsAttrUnsupported
}

func (windowsOnlyServiceAttrs) DisplayName() (string, error) {
	return "", ErrServiceWindowsAttrUnsupported
}

func (windowsOnlyServiceAttrs) Pid() (int, error) {
	return 0, ErrServiceWindowsAttrUnsupported
}

func invalidService(s string) bool {
	return strings.ContainsRune(s, '/')
}
