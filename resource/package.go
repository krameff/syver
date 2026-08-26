package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Package struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Name          string  `json:"name,omitempty" yaml:"name,omitempty"`
	Installed     matcher `json:"installed" yaml:"installed"`
	Versions      matcher `json:"versions,omitempty" yaml:"versions,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	PackageResourceKey  = "package"
	PackageResourceName = "Package"
)

func init() {
	Register(Descriptor{
		Key:          PackageResourceKey,
		Name:         PackageResourceName,
		New:          func() Resource { return &Package{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Package{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 3},
	})
}

func (p *Package) ID() string {
	if p.Name != "" && p.Name != p.id {
		return fmt.Sprintf("%s: %s", p.id, p.Name)
	}
	return p.id
}
func (p *Package) SetID(id string)  { p.id = id }
func (p *Package) SetSkip()         { p.Skip = true }
func (p *Package) TypeKey() string  { return PackageResourceKey }
func (p *Package) TypeName() string { return PackageResourceName }
func (p *Package) GetTitle() string { return p.Title }
func (p *Package) GetMeta() meta    { return p.Meta }
func (p *Package) GetName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.id
}

func (p *Package) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, p.ID())
	skip := p.Skip
	sysPkg := sys.NewPackage(ctx, p.GetName(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(p, "installed", p.Installed, sysPkg.Installed, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(p.Versions, fmt.Sprintf("%s: package.versions", p.ID()), p.Skip) {
		results = append(results, ValidateValue(p, "version", p.Versions, sysPkg.Versions, skip))
	}
	return results
}

func NewPackage(sysPackage system.Package, config util.Config) (*Package, error) {
	name := sysPackage.Name()
	// Propagate, do not swallow. A non-nil error here means the run did not
	// happen: the system layer folds a non-zero exit and a missing binary into
	// (false, nil) on purpose, and returns an error only when the helper was
	// cancelled or exceeded its bound. package_rpm.go and its three siblings each
	// say so in a comment -- reporting that as "not installed" is "a confident
	// wrong answer rather than an unknown one".
	//
	// Discarding it here threw that distinction away one layer above where it was
	// made, and wrote `installed: false` into the generated gossfile: an
	// assertion the user never made, on a host syver had learned nothing about.
	// NewService (service.go) has always propagated; this matches it.
	installed, err := sysPackage.Installed()
	if err != nil {
		return nil, err
	}
	p := &Package{
		id:        name,
		Installed: installed,
	}
	if !contains(config.IgnoreList, "versions") {
		if versions, err := sysPackage.Versions(); err == nil && len(versions) > 0 {
			p.Versions = versions
		}
	}
	return p, nil
}

// fromSystem builds a fresh Package from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewPackage" off
// *system.System from a type parameter alone.
func (p *Package) fromSystem(sys *system.System, key string, config util.Config) (system.Package, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewPackage(ctx, key, sys, config)
	n, err := NewPackage(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*p = *n
	return sysRes, nil
}
