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

func (s *ServiceWindows) Exists() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, "Get-Service", "-Name", s.service)
	if err != nil {
		return false, err
	}
	if strings.Contains(cmd.Stderr.String(), "Cannot find any service with service name") {
		return false, nil
	}
	return true, cmd.Err
}

func (s *ServiceWindows) Enabled() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, fmt.Sprintf("$(Get-Service -Name %q).StartType", s.service))
	if err != nil {
		return false, err
	}
	if strings.Contains(cmd.Stdout.String(), "Automatic") {
		return true, cmd.Err
	}
	return false, cmd.Err
}

func (s *ServiceWindows) Running() (bool, error) {
	cmd, err := runHelperPowershell(s.ctx, fmt.Sprintf("$(Get-Service -Name %q).Status", s.service))
	if err != nil {
		return false, err
	}
	if strings.Contains(cmd.Stdout.String(), "Running") {
		return true, cmd.Err
	}
	return false, cmd.Err
}

func (s *ServiceWindows) RunLevels() ([]string, error) {
	return nil, nil
}
