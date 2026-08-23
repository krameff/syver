package resource

import (
	"context"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Syverfile struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta   `json:"meta,omitempty" yaml:"meta,omitempty"`
	Path          string `json:"-" yaml:"-"`
	Skip          bool   `json:"skip,omitempty" yaml:"skip,omitempty"`
	File          string `json:"file,omitempty" yaml:"file,omitempty"`
}

const (
	SyverFileResourceKey  = "gossfile"
	SyverFileResourceName = "Gossfile"
)

func init() {
	// FEAT-007 G3: the old registerResource calls registered this type
	// under TWO separate keys ("gossfile" and "syverfile"). Registering it
	// as two separate Descriptors would give Descriptors() two entries for
	// what's really one type, and dispatch.go's exhaustiveness guard would
	// then demand a "syverfile" accessor that doesn't naturally exist --
	// SyverfileAlias is folded and nil'd entirely inside store.go, outside
	// the registry. So: one Register call, "syverfile" as an Alias.
	//
	// Excluded from validation (InValidation: false) and discovery
	// (InDiscovery: false) -- SyverConfig.Resources() and DiscoveryConfig
	// both omit gossfile today; Validate() always returns an empty result
	// set anyway, so this has no observable effect either way, but the
	// exhaustiveness guard in dispatch.go needs the flags to be correct.
	Register(Descriptor{
		Key:   SyverFileResourceKey,
		Name:  SyverFileResourceName,
		New:   func() Resource { return &Syverfile{} },
		Alias: []string{"syverfile"},
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Syverfile{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		// FEAT-007 G4: the `syver add` subcommand for this type is named
		// neither Key ("gossfile") nor Name ("Gossfile") -- it's "syver",
		// aliased to "goss". See cmd/syver/syver.go's descriptor-driven
		// command generation (task 8).
		CLIName:    "syver",
		CLIAliases: []string{"goss"},
	})
}

func (g *Syverfile) ID() string       { return g.Path }
func (g *Syverfile) SetID(id string)  { g.Path = id }
func (g *Syverfile) SetSkip()         {}
func (g *Syverfile) TypeKey() string  { return SyverFileResourceKey }
func (g *Syverfile) TypeName() string { return SyverFileResourceName }

func (g *Syverfile) GetTitle() string { return g.Title }
func (g *Syverfile) GetMeta() meta    { return g.Meta }

func (g *Syverfile) GetSkip() bool { return g.Skip }

func (g *Syverfile) GetSyverfile() string {
	if g.File != "" {
		return g.File
	}
	return g.Path
}

func (g *Syverfile) Validate(ctx context.Context, sys *system.System) []TestResult {
	return []TestResult{}
}

func NewSyverfile(sysSyverfile system.Syverfile, config util.Config) (*Syverfile, error) {
	path := sysSyverfile.Path()
	return &Syverfile{
		Path: path,
	}, nil
}

// fromSystem builds a fresh Syverfile from live system state, populating
// the receiver in place. See the fromSystem comment on any other type
// (e.g. port.go) for why this can't be derived generically.
func (g *Syverfile) fromSystem(sys *system.System, key string, config util.Config) (system.Syverfile, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewSyverfile(ctx, key, sys, config)
	n, err := NewSyverfile(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*g = *n
	return sysRes, nil
}
