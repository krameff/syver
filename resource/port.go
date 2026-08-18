package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Port struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Port          string  `json:"port,omitempty" yaml:"port,omitempty"`
	Listening     matcher `json:"listening" yaml:"listening"`
	IP            matcher `json:"ip,omitempty" yaml:"ip,omitempty"`
	PID           matcher `json:"pid,omitempty" yaml:"pid,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	PortResourceKey  = "port"
	PortResourceName = "Port"
)

func init() {
	registerResource(PortResourceKey, &Port{})
}

func (p *Port) ID() string {
	if p.Port != "" && p.Port != p.id {
		return fmt.Sprintf("%s: %s", p.id, p.Port)
	}
	return p.id
}
func (p *Port) SetID(id string)  { p.id = id }
func (p *Port) SetSkip()         { p.Skip = true }
func (p *Port) TypeKey() string  { return PortResourceKey }
func (p *Port) TypeName() string { return PortResourceName }
func (p *Port) GetTitle() string { return p.Title }
func (p *Port) GetMeta() meta    { return p.Meta }
func (p *Port) GetPort() string {
	if p.Port != "" {
		return p.Port
	}
	return p.id
}

func (p *Port) Validate(sys *system.System) []TestResult {
	ctx := context.WithValue(context.Background(), idKey{}, p.ID())
	skip := p.Skip
	sysPort := sys.NewPort(ctx, p.GetPort(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(p, "listening", p.Listening, sysPort.Listening, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSet(p.IP) {
		results = append(results, ValidateValue(p, "ip", p.IP, sysPort.IP, skip))
	}
	if isSet(p.PID) {
		results = append(results, ValidateValue(p, "pid", p.PID, sysPort.PID, skip))
	}
	return results
}

func NewPort(sysPort system.Port, config util.Config) (*Port, error) {
	port := sysPort.Port()
	listening, _ := sysPort.Listening()
	p := &Port{
		id:        port,
		Listening: listening,
	}
	if !contains(config.IgnoreList, "ip") {
		if ip, err := sysPort.IP(); err == nil {
			p.IP = ip
		}
	}
	// pid is intentionally not auto-populated by discovery/add: process PIDs are
	// reassigned on every restart, so a discovered gossfile would pin a value
	// that's already stale by the time it's used. The field still works if added
	// to a gossfile by hand -- Validate() checks it whenever it's set.
	return p, nil
}
