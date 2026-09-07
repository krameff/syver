package system

import (
	"errors"
	"fmt"
	"strings"
)

type Service interface {
	Service() string
	Exists() (bool, error)
	Enabled() (bool, error)
	Running() (bool, error)
	RunLevels() ([]string, error)
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

// psSingleQuote renders s as a PowerShell SINGLE-quoted string literal, which
// is the only correct way to put a gossfile-supplied service name into a
// PowerShell command line here.
//
// THIS EXISTS BECAUSE `%q` WAS WRONG AND LOOKED RIGHT. The three probes in
// service_windows.go used fmt.Sprintf("... -Name %q ...", s.service), and %q
// applies GO string-literal escaping, not PowerShell escaping. PowerShell
// DOUBLE-quoted strings interpolate $(...) subexpressions, executing what is
// inside while merely building the string, and %q does not escape `$` because
// `$` is an ordinary printable character to Go. So a service name of
// `$(calc.exe)` ran calc.exe with no quote-breaking needed at all. Separately,
// PowerShell does not treat `\` as an escape inside double quotes, so the `\"`
// that %q emits for an embedded quote does not escape it either.
//
// Single-quoted PowerShell strings interpolate NOTHING. The only metacharacter
// is `'` itself, escaped by doubling. That makes this the minimal complete fix.
// Do not "improve" it back to %q, and do not switch the call sites to double
// quotes.
//
// The command line reaches CreateProcess verbatim via SysProcAttr.CmdLine (see
// util.NewCommandForWindowsPowershellContext), so Go's own argv escaping never
// applies and cannot be relied on here.
func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func invalidService(s string) bool {
	return strings.ContainsRune(s, '/')
}

// parseServiceExistsProbe turns ServiceWindows.Exists's PowerShell probe
// output into (bool, error). It expects exactly the literal "True" or
// "False" that syver's own probe script emits -- never a Windows-generated
// message -- so kept as a pure function, and in this untagged file rather
// than service_windows.go, so it is unit-testable from Linux by feeding it a
// captured PowerShell result directly, without spawning PowerShell. See
// FEAT-010 Task 4 / D-7.
func parseServiceExistsProbe(stdout, stderr string) (bool, error) {
	switch strings.TrimSpace(stdout) {
	case "True":
		return true, nil
	case "False":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected Get-Service probe output: stdout=%q stderr=%q", stdout, stderr)
	}
}

// parseServiceAttributeProbe turns Enabled/Running's combined
// existence-plus-attribute probe output into the raw attribute value and an
// existence flag. The probe script emits either "EXISTS|<value>" or the bare
// literal "ABSENT" -- both chosen by syver, never rendered by Windows, so
// absence detection here does not depend on locale. Pure and untagged for
// the same testability reason as parseServiceExistsProbe.
func parseServiceAttributeProbe(stdout string) (value string, exists bool) {
	out := strings.TrimSpace(stdout)
	if out == "ABSENT" {
		return "", false
	}
	return out, true
}
