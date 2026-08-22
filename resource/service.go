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
	ctx = context.WithValue(ctx, idKey{}, s.ID())
	skip := s.Skip
	sysservice := sys.NewService(ctx, s.GetName(), sys, util.Config{})

	var results []TestResult
	if isSet(s.Enabled) {
		results = append(results, ValidateValue(s, "enabled", s.Enabled, sysservice.Enabled, skip))
	}
	if isSet(s.Running) {
		results = append(results, ValidateValue(s, "running", s.Running, sysservice.Running, skip))
	}
	if isSetWarnEmpty(s.RunLevels, fmt.Sprintf("%s: service.runlevels", s.ID())) {
		results = append(results, ValidateValue(s, "runlevels", s.RunLevels, sysservice.RunLevels, skip))
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
	return &Service{
		id:      service,
		Enabled: enabled,
		Running: running,
	}, nil
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
