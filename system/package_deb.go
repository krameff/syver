package system

import (
	"context"
	"strings"

	"github.com/krameff/syver/util"
)

type DebPackage struct {
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

func NewDebPackage(ctx context.Context, name string, system *System, config util.Config) Package {
	return &DebPackage{ctx: ctx, name: name}
}

func (p *DebPackage) setup() {
	if p.loaded {
		return
	}
	p.loaded = true
	cmd, ctxErr := runHelperCommand(p.ctx, "dpkg-query", "-f", "${Status} ${Version}\n", "-W", p.name)
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
	for _, l := range strings.Split(strings.TrimSpace(cmd.Stdout.String()), "\n") {
		if !strings.HasPrefix(l, "install ok installed") && !strings.HasPrefix(l, "hold ok installed") {
			continue
		}
		ver := strings.Fields(l)[3]
		p.versions = append(p.versions, ver)
	}

	if len(p.versions) > 0 {
		p.installed = true
	}
}

func (p *DebPackage) Name() string {
	return p.name
}

func (p *DebPackage) Exists() (bool, error) { return p.Installed() }

func (p *DebPackage) Installed() (bool, error) {
	p.setup()

	return p.installed, p.err
}

func (p *DebPackage) Versions() ([]string, error) {
	p.setup()
	if p.err != nil {
		return p.versions, p.err
	}
	if len(p.versions) == 0 {
		return p.versions, ErrPackageVersionNotFound
	}
	return p.versions, nil
}
