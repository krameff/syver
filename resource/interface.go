package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Interface struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Name          string  `json:"name,omitempty" yaml:"name,omitempty"`
	Exists        matcher `json:"exists" yaml:"exists"`
	Addrs         matcher `json:"addrs,omitempty" yaml:"addrs,omitempty"`
	MTU           matcher `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	InterfaceResourceKey  = "interface"
	InterfaceResourceName = "Interface"
)

func init() {
	Register(Descriptor{
		Key:          InterfaceResourceKey,
		Name:         InterfaceResourceName,
		New:          func() Resource { return &Interface{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Interface{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
	})
}

func (i *Interface) ID() string {
	if i.Name != "" && i.Name != i.id {
		return fmt.Sprintf("%s: %s", i.id, i.Name)
	}
	return i.id
}
func (i *Interface) SetID(id string)  { i.id = id }
func (i *Interface) SetSkip()         { i.Skip = true }
func (i *Interface) TypeKey() string  { return InterfaceResourceKey }
func (i *Interface) TypeName() string { return InterfaceResourceName }

// FIXME: Can this be refactored?
func (i *Interface) GetTitle() string { return i.Title }
func (i *Interface) GetMeta() meta    { return i.Meta }
func (i *Interface) GetName() string {
	if i.Name != "" {
		return i.Name
	}
	return i.id
}

func (i *Interface) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, i.ID())
	skip := i.Skip
	sysInterface := sys.NewInterface(ctx, i.GetName(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(i, "exists", i.Exists, sysInterface.Exists, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(i.Addrs, fmt.Sprintf("%s: interface.addrs", i.ID()), i.Skip) {
		results = append(results, ValidateValue(i, "addrs", i.Addrs, sysInterface.Addrs, skip))
	}
	if isSet(i.MTU) {
		results = append(results, ValidateValue(i, "mtu", i.MTU, sysInterface.MTU, skip))
	}
	return results
}

func NewInterface(sysInterface system.Interface, config util.Config) (*Interface, error) {
	name := sysInterface.Name()
	// Propagate rather than discard -- see resource/registry.go's NewRegistry
	// for the shared rationale (BUG-004 / FEAT-010 SW-10). After FEAT-010
	// Task 5, system.DefInterface.Exists only returns a non-nil error when
	// the platform syscall itself failed; a genuinely absent interface name
	// still yields (false, nil), unchanged.
	exists, err := sysInterface.Exists()
	if err != nil {
		return nil, err
	}
	i := &Interface{
		id:     name,
		Exists: exists,
	}
	if !contains(config.IgnoreList, "addrs") {
		// Only record addrs when the interface actually has some. system's
		// Addrs() builds with `var ret []string` + append, so a down or
		// unaddressed interface returns a typed nil with a nil error -- which
		// the err == nil guard happily passes and which omitempty does NOT
		// drop, because matcher is an interface and yaml.v2's isZero for an
		// interface field is IsNil(), true only for a nil interface.
		if addrs, err := sysInterface.Addrs(); err == nil && len(addrs) > 0 {
			i.Addrs = addrs
		}
	}
	if !contains(config.IgnoreList, "mtu") {
		if mtu, err := sysInterface.MTU(); err == nil {
			i.MTU = mtu
		}
	}
	return i, nil
}

// fromSystem builds a fresh Interface from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewInterface" off
// *system.System from a type parameter alone.
func (i *Interface) fromSystem(sys *system.System, key string, config util.Config) (system.Interface, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewInterface(ctx, key, sys, config)
	n, err := NewInterface(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*i = *n
	return sysRes, nil
}
