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
	Register(Descriptor{
		Key:          PortResourceKey,
		Name:         PortResourceName,
		New:          func() Resource { return &Port{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Port{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 4},
	})
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

func (p *Port) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, p.ID())
	skip := p.Skip
	sysPort := sys.NewPort(ctx, p.GetPort(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(p, "listening", p.Listening, sysPort.Listening, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(p.IP, fmt.Sprintf("%s: port.ip", p.ID()), skip) {
		results = append(results, ValidateValue(p, "ip", p.IP, sysPort.IP, skip))
	}
	if isSetWarnEmpty(p.PID, fmt.Sprintf("%s: port.pid", p.ID()), skip) {
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
		// Only record ip when the port actually has addresses; an empty list
		// asserts nothing and isSet skips it, so writing it out is noise.
		if ip, err := sysPort.IP(); err == nil && len(ip) > 0 {
			p.IP = ip
		}
	}
	// pid is intentionally not auto-populated by discovery/add: process PIDs are
	// reassigned on every restart, so a discovered gossfile would pin a value
	// that's already stale by the time it's used. The field still works if added
	// to a gossfile by hand -- Validate() checks it whenever it's set.
	return p, nil
}

// fromSystem builds a fresh Port from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewPort" off
// *system.System from a type parameter alone.
func (p *Port) fromSystem(sys *system.System, key string, config util.Config) (system.Port, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewPort(ctx, key, sys, config)
	n, err := NewPort(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*p = *n
	return sysRes, nil
}
