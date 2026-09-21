package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Service struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Name          string  `json:"name,omitempty" yaml:"name,omitempty"`
	Enabled       matcher `json:"enabled" yaml:"enabled"`
	Running       matcher `json:"running" yaml:"running"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
	RunLevels     matcher `json:"runlevels,omitempty" yaml:"runlevels,omitempty"`
	// FEAT-014, Windows only. Every other platform reports
	// ErrServiceWindowsAttrUnsupported for these, so a spec that sets one on Linux
	// fails rather than comparing against a zero value.
	StartType    matcher `json:"start-type,omitempty" yaml:"start-type,omitempty"`
	DelayedStart matcher `json:"delayed-start,omitempty" yaml:"delayed-start,omitempty"`
	RunAs        matcher `json:"run-as,omitempty" yaml:"run-as,omitempty"`
	Dependencies matcher `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	DisplayName  matcher `json:"display-name,omitempty" yaml:"display-name,omitempty"`
	Pid          matcher `json:"pid,omitempty" yaml:"pid,omitempty"`
}

const (
	ServiceResourceKey  = "service"
	ServiceResourceName = "Service"
)

func init() {
	Register(Descriptor{
		Key:          ServiceResourceKey,
		Name:         ServiceResourceName,
		New:          func() Resource { return &Service{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Service{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 6},
	})
}

func (s *Service) ID() string {
	if s.Name != "" && s.Name != s.id {
		return fmt.Sprintf("%s: %s", s.id, s.Name)
	}
	return s.id
}
func (s *Service) SetID(id string)  { s.id = id }
func (s *Service) SetSkip()         { s.Skip = true }
func (s *Service) TypeKey() string  { return ServiceResourceKey }
func (s *Service) TypeName() string { return ServiceResourceName }
func (s *Service) GetTitle() string { return s.Title }
func (s *Service) GetMeta() meta    { return s.Meta }
func (s *Service) GetName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.id
}

func (s *Service) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, s.ID())
	skip := s.Skip
	sysservice := sys.NewService(ctx, s.GetName(), sys, util.Config{})

	var results []TestResult
	if isSet(s.Enabled) {
		results = append(results, ValidateValue(s, "enabled", s.Enabled, sysservice.Enabled, skip))
	}
	if isSet(s.Running) {
		results = append(results, ValidateValue(s, "running", s.Running, sysservice.Running, skip))
	}
	if isSetWarnEmpty(s.RunLevels, fmt.Sprintf("%s: service.runlevels", s.ID()), s.Skip) {
		results = append(results, ValidateValue(s, "runlevels", s.RunLevels, sysservice.RunLevels, skip))
	}
	if isSet(s.StartType) {
		results = append(results, ValidateValue(s, "start-type", s.StartType, sysservice.StartType, skip))
	}
	if isSet(s.DelayedStart) {
		results = append(results, ValidateValue(s, "delayed-start", s.DelayedStart, sysservice.DelayedStart, skip))
	}
	if isSet(s.RunAs) {
		results = append(results, ValidateValue(s, "run-as", s.RunAs, sysservice.RunAs, skip))
	}
	// A list, so isSetWarnEmpty: `dependencies: []` asserts nothing and passes.
	if isSetWarnEmpty(s.Dependencies, fmt.Sprintf("%s: service.dependencies", s.ID()), s.Skip) {
		results = append(results, ValidateValue(s, "dependencies", s.Dependencies, sysservice.Dependencies, skip))
	}
	if isSet(s.DisplayName) {
		results = append(results, ValidateValue(s, "display-name", s.DisplayName, sysservice.DisplayName, skip))
	}
	if isSet(s.Pid) {
		results = append(results, ValidateValue(s, "pid", s.Pid, sysservice.Pid, skip))
	}
	return results
}

func NewService(sysService system.Service, config util.Config) (*Service, error) {
	service := sysService.Service()
	enabled, err := sysService.Enabled()
	if err != nil {
		return nil, err
	}
	running, err := sysService.Running()
	if err != nil {
		return nil, err
	}
	svc := &Service{
		id:      service,
		Enabled: enabled,
		Running: running,
	}
	// The Windows-only attributes, emitted only where the platform can report
	// them: everywhere else these return ErrServiceWindowsAttrUnsupported and the
	// key is omitted, exactly as `syver add` already treats an attribute the host
	// cannot answer. An error here is never fatal to discovery.
	//
	// `pid` is DELIBERATELY NOT EMITTED, though validate supports it. A pid changes
	// at every restart, so writing one into a generated spec produces an assertion
	// guaranteed to fail the next time the service bounces -- a spec that rots on a
	// timer is worse than one key fewer.
	if startType, err := sysService.StartType(); err == nil {
		svc.StartType = startType
	}
	if delayed, err := sysService.DelayedStart(); err == nil {
		svc.DelayedStart = delayed
	}
	if runAs, err := sysService.RunAs(); err == nil {
		svc.RunAs = runAs
	}
	// len > 0 for the reason resource/file.go's acl: records: these fields are
	// `matcher`, an interface, so yaml.v2's omitempty does not omit an interface
	// holding an empty slice and `dependencies: []` would be emitted.
	// resource/generated_empty_test.go pins that for every generator.
	if deps, err := sysService.Dependencies(); err == nil && len(deps) > 0 {
		svc.Dependencies = deps
	}
	if displayName, err := sysService.DisplayName(); err == nil {
		svc.DisplayName = displayName
	}
	return svc, nil
}

// fromSystem builds a fresh Service from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewService" off
// *system.System from a type parameter alone.
func (s *Service) fromSystem(sys *system.System, key string, config util.Config) (system.Service, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewService(ctx, key, sys, config)
	n, err := NewService(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*s = *n
	return sysRes, nil
}
