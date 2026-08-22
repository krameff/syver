package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Process struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Comm          string  `json:"comm,omitempty" yaml:"comm,omitempty"`
	Running       matcher `json:"running" yaml:"running"`
	Status        matcher `json:"status,omitempty" yaml:"status,omitempty"`
	User          matcher `json:"user,omitempty" yaml:"user,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	ProcessResourceKey  = "process"
	ProcessResourceName = "Process"
)

func init() {
	Register(Descriptor{
		Key:          ProcessResourceKey,
		Name:         ProcessResourceName,
		New:          func() Resource { return &Process{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Process{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 5},
	})
}

func (p *Process) ID() string {
	if p.Comm != "" && p.Comm != p.id {
		return fmt.Sprintf("%s: %s", p.id, p.Comm)
	}
	return p.id
}
func (p *Process) SetID(id string)  { p.id = id }
func (p *Process) SetSkip()         { p.Skip = true }
func (p *Process) TypeKey() string  { return ProcessResourceKey }
func (p *Process) TypeName() string { return ProcessResourceName }
func (p *Process) GetTitle() string { return p.Title }
func (p *Process) GetMeta() meta    { return p.Meta }
func (p *Process) GetComm() string {
	if p.Comm != "" {
		return p.Comm
	}
	return p.id
}

func (p *Process) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, p.ID())
	skip := p.Skip
	sysProcess := sys.NewProcess(ctx, p.GetComm(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(p, "running", p.Running, sysProcess.Running, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(p.Status, fmt.Sprintf("%s: process.status", p.ID()), skip) {
		results = append(results, ValidateValue(p, "status", p.Status, sysProcess.Status, skip))
	}
	if isSetWarnEmpty(p.User, fmt.Sprintf("%s: process.user", p.ID()), skip) {
		results = append(results, ValidateValue(p, "user", p.User, sysProcess.User, skip))
	}
	return results
}

func NewProcess(sysProcess system.Process, config util.Config) (*Process, error) {
	executable := sysProcess.Executable()
	running, err := sysProcess.Running()
	if err != nil {
		return nil, err
	}
	p := &Process{
		id:      executable,
		Running: running,
	}
	// Only record these when the process actually reports them; an empty list
	// asserts nothing and isSet skips it, so writing it out is noise.
	if !contains(config.IgnoreList, "status") {
		if status, err := sysProcess.Status(); err == nil && len(status) > 0 {
			p.Status = status
		}
	}
	if !contains(config.IgnoreList, "user") {
		if user, err := sysProcess.User(); err == nil && len(user) > 0 {
			p.User = user
		}
	}
	return p, nil
}

// fromSystem builds a fresh Process from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewProcess" off
// *system.System from a type parameter alone.
func (p *Process) fromSystem(sys *system.System, key string, config util.Config) (system.Process, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewProcess(ctx, key, sys, config)
	n, err := NewProcess(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*p = *n
	return sysRes, nil
}
