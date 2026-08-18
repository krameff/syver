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
	registerResource(ProcessResourceKey, &Process{})
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

func (p *Process) Validate(sys *system.System) []TestResult {
	ctx := context.WithValue(context.Background(), idKey{}, p.ID())
	skip := p.Skip
	sysProcess := sys.NewProcess(ctx, p.GetComm(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(p, "running", p.Running, sysProcess.Running, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSet(p.Status) {
		results = append(results, ValidateValue(p, "status", p.Status, sysProcess.Status, skip))
	}
	if isSet(p.User) {
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
	if !contains(config.IgnoreList, "status") {
		if status, err := sysProcess.Status(); err == nil {
			p.Status = status
		}
	}
	if !contains(config.IgnoreList, "user") {
		if user, err := sysProcess.User(); err == nil {
			p.User = user
		}
	}
	return p, nil
}
