package system

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/krameff/syver/util"
)

type ServiceUpstart struct {
	// ctx bounds and cancels the `service ... status` subprocess in Running.
	// See the matching field on ServiceSystemd.
	ctx     context.Context
	service string
}

var upstartEnabled = regexp.MustCompile(`^\s*start on`)
var upstartDisabled = regexp.MustCompile(`^manual`)

func NewServiceUpstart(ctx context.Context, service string, system *System, config util.Config) Service {
	return &ServiceUpstart{ctx: ctx, service: service}
}

func (s *ServiceUpstart) Service() string {
	return s.service
}

func (s *ServiceUpstart) Exists() (bool, error) {
	// upstart
	if _, err := os.Stat(fmt.Sprintf("/etc/init/%s.conf", s.service)); err == nil {
		return true, nil
	}
	// Fallback on sysv
	sysv := &ServiceInit{ctx: s.ctx, service: s.service}
	if e, err := sysv.Exists(); e && err == nil {
		return true, nil
	}
	return false, nil
}

func (s *ServiceUpstart) Enabled() (bool, error) {
	if fh, err := os.Open(fmt.Sprintf("/etc/init/%s.override", s.service)); err == nil {
		// Deferred, not closed at the end of this block: the loop below can
		// return early, and `serve` mode re-runs this per request for the life
		// of the process, so a missed Close leaks a descriptor per request.
		defer fh.Close()
		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			line := scanner.Text()
			if upstartDisabled.MatchString(line) {
				return false, nil
			}
		}
	}

	// If no /etc/init/<service>.override with `manual` keyword in it has been found
	// Check the contents of the upstart manifest.
	if fh, err := os.Open(fmt.Sprintf("/etc/init/%s.conf", s.service)); err == nil {
		defer fh.Close()
		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			line := scanner.Text()
			if upstartEnabled.MatchString(line) {
				return true, nil
			}
		}
	}
	// Fallback on sysv
	sysv := &ServiceInit{ctx: s.ctx, service: s.service}
	if en, err := sysv.Enabled(); en && err == nil {
		return true, nil
	}
	return false, nil
}

func (s *ServiceUpstart) Running() (bool, error) {
	cmd, err := runHelperCommand(s.ctx, "service", s.service, "status")
	if err != nil {
		return false, err
	}
	out := cmd.Stdout.String()
	if cmd.Status == 0 && (strings.Contains(out, "running") || strings.Contains(out, "online")) {
		return true, cmd.Err
	}
	return false, nil
}
func (s *ServiceUpstart) RunLevels() ([]string, error) {
	sysv := &ServiceInit{ctx: s.ctx, service: s.service}
	return sysv.RunLevels()
}
