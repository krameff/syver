package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/krameff/syver/util"
)

type ServiceSystemd struct {
	// ctx is the Validate context this service was constructed for. It is held
	// rather than passed per call because the Service interface's methods take
	// no arguments, and it is what bounds and cancels the systemctl subprocesses
	// below -- see runHelperCommand.
	ctx     context.Context
	service string
	legacy  bool
}

func NewServiceSystemd(ctx context.Context, service string, system *System, config util.Config) Service {
	return &ServiceSystemd{
		ctx:     ctx,
		service: service,
	}
}

func NewServiceSystemdLegacy(ctx context.Context, service string, system *System, config util.Config) Service {
	return &ServiceSystemd{
		ctx:     ctx,
		service: service,
		legacy:  true,
	}
}

func (s *ServiceSystemd) Service() string {
	return s.service
}

func (s *ServiceSystemd) Exists() (bool, error) {
	if invalidService(s.service) {
		return false, nil
	}
	cmd, err := runHelperCommand(s.ctx, "systemctl", "-q", "list-unit-files", "--type=service")
	if err != nil {
		return false, err
	}
	if strings.Contains(cmd.Stdout.String(), fmt.Sprintf("%s.service", s.service)) {
		return true, cmd.Err
	}
	if s.legacy {
		// Fallback on sysv
		sysv := &ServiceInit{ctx: s.ctx, service: s.service}
		if e, err := sysv.Exists(); e && err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *ServiceSystemd) Enabled() (bool, error) {
	if invalidService(s.service) {
		return false, nil
	}
	cmd, err := runHelperCommand(s.ctx, "systemctl", "-q", "is-enabled", s.service)
	if err != nil {
		return false, err
	}
	if cmd.Status == 0 {
		return true, cmd.Err
	}
	if s.legacy {
		// Fallback on sysv
		sysv := &ServiceInit{ctx: s.ctx, service: s.service}
		if en, err := sysv.Enabled(); en && err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *ServiceSystemd) Running() (bool, error) {
	if invalidService(s.service) {
		return false, nil
	}
	cmd, err := runHelperCommand(s.ctx, "systemctl", "-q", "is-active", s.service)
	if err != nil {
		return false, err
	}
	if cmd.Status == 0 {
		return true, cmd.Err
	}
	if s.legacy {
		// Fallback on sysv
		sysv := &ServiceInit{ctx: s.ctx, service: s.service}
		if r, err := sysv.Running(); r && err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *ServiceSystemd) RunLevels() ([]string, error) {
	return nil, nil
}
