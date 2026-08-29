package resource

import (
	"context"
	"testing"

	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysPackage is a minimal system.Package fake so resource-level tests do
// not need a real package manager.
type fakeSysPackage struct {
	name       string
	installed  bool
	versions   []string
	err        error
	versionErr error
}

func (f *fakeSysPackage) Name() string                { return f.name }
func (f *fakeSysPackage) Exists() (bool, error)       { return f.installed, f.err }
func (f *fakeSysPackage) Installed() (bool, error)    { return f.installed, f.err }
func (f *fakeSysPackage) Versions() ([]string, error) { return f.versions, f.versionErr }

// A timed-out package manager must not be recorded as "not installed".
//
// `syver add` writes what it observed into a gossfile the user will then run
// against other hosts. When the helper is cancelled or exceeds its bound,
// NOTHING was observed: system/package_rpm.go and its three siblings return the
// context error precisely so this layer can tell "absent" from "unknown", and
// each carries a comment saying reporting it as not-installed is "a confident
// wrong answer rather than an unknown one".
//
// Discarding that error here threw the distinction away one layer above where
// it was made, and produced `installed: false` -- an assertion the user did not
// make and cannot see is wrong. NewService has always propagated; this brings
// NewPackage into line with it.
func TestNewPackageDoesNotRecordAbsentOnTimeout(t *testing.T) {
	t.Run("a context error is propagated, not swallowed", func(t *testing.T) {
		sysPkg := &fakeSysPackage{name: "nginx", err: context.DeadlineExceeded}
		p, err := NewPackage(sysPkg, util.Config{})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		// The caller must get nothing to write, not a plausible-looking zero
		// value. Returning both a resource and an error invites the resource to
		// be used anyway.
		assert.Assert(t, p == nil, "no package should be returned when nothing was learned")
	})

	t.Run("a genuinely absent package still records installed: false", func(t *testing.T) {
		// The system layer folds a non-zero exit or a missing binary into
		// (false, nil) deliberately. That is a real observation and must keep
		// working -- this fix must not turn "absent" into an error.
		sysPkg := &fakeSysPackage{name: "nginx", installed: false}
		p, err := NewPackage(sysPkg, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.Installed, false)
		assert.Equal(t, p.id, "nginx")
	})

	t.Run("an installed package still records its versions", func(t *testing.T) {
		sysPkg := &fakeSysPackage{name: "nginx", installed: true, versions: []string{"1.2.3"}}
		p, err := NewPackage(sysPkg, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.Installed, true)
		assert.DeepEqual(t, p.Versions, []string{"1.2.3"})
	})
}
