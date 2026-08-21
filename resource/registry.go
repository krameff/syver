package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Registry struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Name          string  `json:"name,omitempty" yaml:"name,omitempty"`
	Exists        matcher `json:"exists" yaml:"exists"`
	Value         matcher `json:"value,omitempty" yaml:"value,omitempty"`
	Type          matcher `json:"type,omitempty" yaml:"type,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	RegistryResourceKey  = "registry"
	RegistryResourceName = "Registry"
)

func init() {
	Register(Descriptor{
		Key:          RegistryResourceKey,
		Name:         RegistryResourceName,
		New:          func() Resource { return &Registry{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Registry{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
	})
}

func (r *Registry) ID() string {
	if r.Name != "" && r.Name != r.id {
		return fmt.Sprintf("%s: %s", r.id, r.Name)
	}
	return r.id
}

func (r *Registry) SetID(id string)  { r.id = id }
func (r *Registry) SetSkip()         { r.Skip = true }
func (r *Registry) TypeKey() string  { return RegistryResourceKey }
func (r *Registry) TypeName() string { return RegistryResourceName }
func (r *Registry) GetTitle() string { return r.Title }
func (r *Registry) GetMeta() meta    { return r.Meta }
func (r *Registry) GetName() string {
	if r.Name != "" {
		return r.Name
	}
	return r.id
}

func (r *Registry) Validate(sys *system.System) []TestResult {
	ctx := context.WithValue(context.Background(), idKey{}, r.ID())
	skip := r.Skip
	sysRegistry := sys.NewRegistry(ctx, r.GetName(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(r, "exists", r.Exists, sysRegistry.Exists, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSet(r.Value) {
		results = append(results, ValidateValue(r, "value", r.Value, sysRegistry.Value, skip))
	}
	if isSet(r.Type) {
		results = append(results, ValidateValue(r, "type", r.Type, sysRegistry.Type, skip))
	}
	return results
}

func NewRegistry(sysRegistry system.Registry, config util.Config) (*Registry, error) {
	key := sysRegistry.Key()
	exists, _ := sysRegistry.Exists()
	if !exists {
		return &Registry{
			id:     key,
			Exists: exists,
		}, nil
	}
	value, err := sysRegistry.Value()
	if err != nil {
		return nil, err
	}
	regType, err := sysRegistry.Type()
	if err != nil {
		return nil, err
	}
	return &Registry{
		id:     key,
		Exists: exists,
		Value:  value,
		Type:   regType,
	}, nil
}

// fromSystem builds a fresh Registry from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewRegistry" off
// *system.System from a type parameter alone.
func (r *Registry) fromSystem(sys *system.System, key string, config util.Config) (system.Registry, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewRegistry(ctx, key, sys, config)
	n, err := NewRegistry(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*r = *n
	return sysRes, nil
}
