package resource

import (
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
	registerResource(SyverFileResourceKey, &Syverfile{})
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

func (g *Syverfile) Validate(sys *system.System) []TestResult {
	return []TestResult{}
}

func NewSyverfile(sysSyverfile system.Syverfile, config util.Config) (*Syverfile, error) {
	path := sysSyverfile.Path()
	return &Syverfile{
		Path: path,
	}, nil
}
