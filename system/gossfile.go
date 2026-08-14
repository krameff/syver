package system

import (
	"context"

	"github.com/krameff/syver/util"
)

type Syverfile interface {
	Path() string
	Exists() (bool, error)
}

type DefSyverfile struct {
	path string
}

func (g *DefSyverfile) Path() string {
	return g.path
}

// Stub out
func (g *DefSyverfile) Exists() (bool, error) {
	return false, nil
}

func NewDefSyverfile(_ context.Context, path string, system *System, config util.Config) Syverfile {
	return &DefSyverfile{path: path}
}
