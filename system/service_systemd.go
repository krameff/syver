package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/krameff/syver/util"
)

type ServiceSystemd struct {
	windowsOnlyServiceAttrs // FEAT-014: the six Windows-only attributes, as errors
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

// Every systemctl call below that passes the unit name puts "--" before it, so a
// name beginning with "-" is read as a unit name rather than as an option.

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
	// Compare the first column exactly. This used to be a substring match on
	// "<name>.service" anywhere in the output, so `cron` matched
	// `anacron.service` and autoadd wrote an entry for a service that is not
	// installed. FEAT-019.
	unit := s.service + ".service"
	for _, line := range strings.Split(cmd.Stdout.String(), "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == unit {
			return true, cmd.Err
		}
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
	cmd, err := runHelperCommand(s.ctx, "systemctl", "-q", "is-enabled", "--", s.service)
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
	cmd, err := runHelperCommand(s.ctx, "systemctl", "-q", "is-active", "--", s.service)
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

// Masked reports whether the unit is masked, from one
// `systemctl show -p LoadState -p UnitFileState <name>` call. FEAT-019.
//
// `is-enabled` cannot answer this: a masked unit prints "masked" and exits 1,
// exactly as a disabled one does, which is why `enabled:` cannot tell them
// apart.
//
// Repeated -p flags rather than a comma list, because the legacy backend serves
// old systemd. systemd prints the properties in its own order, not the order
// requested, so the output is parsed into a map.
//
// `show` exits 0 for a unit that does not exist (LoadState=not-found), so a
// missing unit is masked: false, the same answer enabled and running give. A
// non-zero exit therefore means the query itself failed, and that is reported
// as an error rather than as "not masked". There is no sysv fallback: sysv has
// no mask concept.
//
// A bare template name such as `getty@` is rejected by `systemctl show` ("is
// neither a valid invocation ID nor unit name") and so reports an error; name an
// instance, such as `getty@tty1`, instead.
func (s *ServiceSystemd) Masked() (bool, error) {
	if invalidService(s.service) {
		return false, nil
	}
	cmd, err := runHelperCommand(s.ctx, "systemctl", "show", "-p", "LoadState", "-p", "UnitFileState", "--", s.service)
	if err != nil {
		return false, err
	}
	if cmd.Err != nil {
		return false, fmt.Errorf("systemctl show %s: %w: %s", s.service, cmd.Err, strings.TrimSpace(cmd.Stderr.String()))
	}
	props := map[string]string{}
	for _, line := range strings.Split(cmd.Stdout.String(), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	load, ok := props["LoadState"]
	if !ok {
		return false, fmt.Errorf("systemctl show %s: no LoadState in output", s.service)
	}
	switch {
	case load == "masked":
		return true, nil
	case props["UnitFileState"] == "masked", props["UnitFileState"] == "masked-runtime":
		return true, nil
	}
	return false, nil
}
