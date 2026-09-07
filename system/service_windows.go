//go:build windows
// +build windows

package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/krameff/syver/util"
)

type ServiceWindows struct {
	// ctx bounds and cancels the Get-Service subprocesses below. See the
	// matching field on ServiceSystemd.
	ctx     context.Context
	service string
}

func NewServiceWindows(ctx context.Context, service string, system *System, config util.Config) Service {
	return &ServiceWindows{
		ctx:     ctx,
		service: service,
	}
}

// runHelperPowershell is runHelperCommand for the powershell wrapper, which
// builds its Cmd differently (the command line goes in SysProcAttr.CmdLine) and
// so cannot go through util.NewCommandContext. Same contract: the caller's
// context, a bounded lifetime, and an error only when the context ended the run.
//
// Note the process-group caveat in util/procgroup_windows.go -- on Windows the
// started powershell is killed, but a grandchild it spawned can survive.
func runHelperPowershell(ctx context.Context, name string, arg ...string) (*util.Command, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helperCommandTimeout)
	defer cancel()

	cmd := util.NewCommandForWindowsPowershellContext(ctx, name, arg...)
	// Must stay in step with runHelperCommand's WaitDelay. This function is a
	// deliberate near-duplicate of it, and that duplication has already cost
	// once: the bound was added to runHelperCommand and this copy was missed,
	// leaving the Windows service path with cancellation and no I/O bound.
	//
	// The consequence is worse here than on POSIX rather than merely equal.
	// util/procgroup_windows.go is a documented no-op, so there is no process
	// group to kill and ANY grandchild survives -- not just one that deliberately
	// detached. Every powershell that spawns something outliving it therefore
	// takes the wedge path, where on Linux it takes a setsid to get there.
	//
	// helperIOGrace, matching runHelperCommand. See that var's comment for why
	// this path deliberately does not derive the grace from the deadline.
	cmd.Cmd.WaitDelay = helperIOGrace
	cmd.Run()

	if err := ctx.Err(); err != nil {
		return cmd, err
	}
	return cmd, nil
}

func (s *ServiceWindows) Service() string {
	return s.service
}

// Exists asks PowerShell for a fixed, English literal ("True"/"False") that
// syver itself chose, rather than string-matching Windows' own (localised)
// "Cannot find any service with service name" error text in stderr. That
// match only worked on an English host -- see FEAT-010 D-7, SW-14.
// -ErrorAction SilentlyContinue turns the missing-service case into a $null
// value with no error text produced at all, so boolean-coercing it via
// `if (...)` is a structural check, not a text match, and works in every
// locale identically.
func (s *ServiceWindows) Exists() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, fmt.Sprintf(
		"if (Get-Service -Name %s -ErrorAction SilentlyContinue) { 'True' } else { 'False' }", psSingleQuote(s.service)))
	if err != nil {
		return false, err
	}
	exists, parseErr := parseServiceExistsProbe(cmd.Stdout.String(), cmd.Stderr.String())
	if parseErr != nil {
		return false, parseErr
	}
	if !exists {
		return false, nil
	}
	return true, cmd.Err
}

// Enabled previously reported a missing service as false (not enabled),
// identically to a service that genuinely exists but is set to Manual or
// Disabled -- resource/service.go's Validate never calls Exists(), so
// `service: TypoedName: {enabled: false}` passed. See FEAT-010 SW-4.
//
// The existence check here uses the same locale-independent structural
// signal as Exists ('EXISTS|<value>' vs the literal 'ABSENT', both chosen by
// syver, never rendered by Windows), combined into the same PowerShell
// invocation that already fetches StartType so this costs no extra
// subprocess. StartType itself is still matched via "Automatic": that
// substring is the .NET enum member name of ServiceStartMode, which
// PowerShell renders invariantly -- it is not localised message text. See
// FEAT-010 D-7 for why that distinction (enum names vs. Windows' own error
// prose) is the one that matters here.
func (s *ServiceWindows) Enabled() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, fmt.Sprintf(
		"$s = Get-Service -Name %s -ErrorAction SilentlyContinue; if ($s) { 'EXISTS|' + $s.StartType } else { 'ABSENT' }",
		psSingleQuote(s.service)))
	if err != nil {
		return false, err
	}
	value, exists := parseServiceAttributeProbe(cmd.Stdout.String())
	if !exists {
		return false, ErrServiceNotFound
	}
	return strings.Contains(value, "Automatic"), cmd.Err
}

// Running is Enabled's counterpart for the Status property. See Enabled's
// comment for the absence-detection mechanism and why "Running" is safe to
// substring-match.
func (s *ServiceWindows) Running() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, fmt.Sprintf(
		"$s = Get-Service -Name %s -ErrorAction SilentlyContinue; if ($s) { 'EXISTS|' + $s.Status } else { 'ABSENT' }",
		psSingleQuote(s.service)))
	if err != nil {
		return false, err
	}
	value, exists := parseServiceAttributeProbe(cmd.Stdout.String())
	if !exists {
		return false, ErrServiceNotFound
	}
	return strings.Contains(value, "Running"), cmd.Err
}

// RunLevels has no Windows equivalent -- there is no SysV/systemd-style
// runlevel concept here. Previously returned (nil, nil); a spec ported from
// Linux that sets `runlevels:` now gets ErrServiceRunLevelsUnsupported
// instead of a ContainElements failure against a nil slice that never
// mentioned Windows. See FEAT-010 S-5.
func (s *ServiceWindows) RunLevels() ([]string, error) {
	return nil, ErrServiceRunLevelsUnsupported
}
