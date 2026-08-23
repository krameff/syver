package system

import (
	"context"
	"strings"

	"github.com/krameff/syver/util"
)

type PacmanPackage struct {
	// ctx bounds and cancels the package-manager subprocess in setup.
	// See runHelperCommand.
	ctx       context.Context
	name      string
	versions  []string
	loaded    bool
	installed bool
	// err holds a context error only -- see setup.
	err error
}

func NewPacmanPackage(ctx context.Context, name string, system *System, config util.Config) Package {
	return &PacmanPackage{ctx: ctx, name: name}
}

func (p *PacmanPackage) setup() {
	if p.loaded {
		return
	}
	p.loaded = true
	// TODO: extract versions
	cmd, ctxErr := runHelperCommand(p.ctx, "pacman", "-Q", "--color", "never", "--noconfirm", p.name)
	if ctxErr != nil {
		// Only the context error is retained. A non-zero exit or a missing
		// package manager binary still means "not installed" here, quietly, as
		// it always has; a cancelled or timed-out run means nothing was
		// learned, and reporting that as "not installed" is a confident wrong
		// answer rather than an unknown one.
		p.err = ctxErr
		return
	}
	if cmd.Err != nil {
		return
	}
	p.installed = true
	// the output format is "pkgname version\n", so if we split the string on
	// whitespace, the version is the second item.
	p.versions = []string{strings.Fields(cmd.Stdout.String())[1]}
}

func (p *PacmanPackage) Name() string {
	return p.name
}

func (p *PacmanPackage) Exists() (bool, error) { return p.Installed() }

func (p *PacmanPackage) Installed() (bool, error) {
	p.setup()

	return p.installed, p.err
}

func (p *PacmanPackage) Versions() ([]string, error) {
	p.setup()
	if p.err != nil {
		return p.versions, p.err
	}
	if len(p.versions) == 0 {
		return p.versions, ErrPackageVersionNotFound
	}
	return p.versions, nil
}
