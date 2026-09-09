package resource

import (
	"errors"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// FEAT-010 SW-10: NewRegistry, NewGroup, NewInterface, NewMount and NewUser
// all used to discard Exists()'s error (`exists, _ := sysX.Exists()`), the
// same shape BUG-004 fixed for NewPackage -- an unreadable/unsupported
// resource silently became `exists: false` in the generated gossfile, an
// assertion the user never made on a host syver had learned nothing about.
// These tests prove each site now propagates instead.

type sw10FakeRegistry struct{ existsErr error }

func (f sw10FakeRegistry) Key() string            { return "HKLM\\SW10\\Fake" }
func (f sw10FakeRegistry) Exists() (bool, error)  { return false, f.existsErr }
func (f sw10FakeRegistry) Value() (string, error) { return "", nil }
func (f sw10FakeRegistry) Type() (string, error)  { return "", nil }
func (f sw10FakeRegistry) SetView(string) error   { return nil }

func TestNewRegistry_PropagatesExistsError(t *testing.T) {
	_, err := NewRegistry(sw10FakeRegistry{existsErr: system.ErrRegistryUnsupported}, util.Config{})
	assert.ErrorIs(t, err, system.ErrRegistryUnsupported)
}

func TestNewRegistry_GenuinelyAbsentStillOmitsNoError(t *testing.T) {
	r, err := NewRegistry(sw10FakeRegistry{}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, r.Exists, matcher(false))
}

type sw10FakeGroup struct{ existsErr error }

func (f sw10FakeGroup) Groupname() string     { return "sw10-fake-group" }
func (f sw10FakeGroup) Exists() (bool, error) { return false, f.existsErr }
func (f sw10FakeGroup) GID() (int, error)     { return 0, errors.New("not called") }

func TestNewGroup_PropagatesExistsError(t *testing.T) {
	boom := errors.New("unreachable domain controller")
	_, err := NewGroup(sw10FakeGroup{existsErr: boom}, util.Config{})
	assert.ErrorIs(t, err, boom)
}

func TestNewGroup_GenuinelyAbsentStillOmitsNoError(t *testing.T) {
	g, err := NewGroup(sw10FakeGroup{}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, g.Exists, matcher(false))
}

type sw10FakeInterface struct{ existsErr error }

func (f sw10FakeInterface) Name() string             { return "sw10-fake-iface" }
func (f sw10FakeInterface) Exists() (bool, error)    { return false, f.existsErr }
func (f sw10FakeInterface) Addrs() ([]string, error) { return nil, nil }
func (f sw10FakeInterface) MTU() (int, error)        { return 0, nil }

func TestNewInterface_PropagatesExistsError(t *testing.T) {
	boom := errors.New("permission denied enumerating adapters")
	_, err := NewInterface(sw10FakeInterface{existsErr: boom}, util.Config{})
	assert.ErrorIs(t, err, boom)
}

func TestNewInterface_GenuinelyAbsentStillOmitsNoError(t *testing.T) {
	i, err := NewInterface(sw10FakeInterface{}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, i.Exists, matcher(false))
}

type sw10FakeUser struct{ existsErr error }

func (f sw10FakeUser) Username() string          { return "sw10-fake-user" }
func (f sw10FakeUser) Exists() (bool, error)     { return false, f.existsErr }
func (f sw10FakeUser) UID() (int, error)         { return 0, errors.New("not called") }
func (f sw10FakeUser) GID() (int, error)         { return 0, errors.New("not called") }
func (f sw10FakeUser) Groups() ([]string, error) { return nil, errors.New("not called") }
func (f sw10FakeUser) Home() (string, error)     { return "", errors.New("not called") }
func (f sw10FakeUser) Shell() (string, error)    { return "", errors.New("not called") }

func TestNewUser_PropagatesExistsError(t *testing.T) {
	boom := errors.New("unreachable domain controller")
	_, err := NewUser(sw10FakeUser{existsErr: boom}, util.Config{})
	assert.ErrorIs(t, err, boom)
}

func TestNewUser_GenuinelyAbsentStillOmitsNoError(t *testing.T) {
	u, err := NewUser(sw10FakeUser{}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, u.Exists, matcher(false))
}

type sw10FakeMount struct{ existsErr error }

func (f sw10FakeMount) MountPoint() string          { return "/sw10-fake-mount" }
func (f sw10FakeMount) Exists() (bool, error)       { return false, f.existsErr }
func (f sw10FakeMount) Opts() ([]string, error)     { return nil, nil }
func (f sw10FakeMount) VfsOpts() ([]string, error)  { return nil, nil }
func (f sw10FakeMount) Source() (string, error)     { return "", nil }
func (f sw10FakeMount) Filesystem() (string, error) { return "", nil }
func (f sw10FakeMount) Usage() (int, error)         { return 0, nil }

func TestNewMount_PropagatesGenuineFailure(t *testing.T) {
	boom := errors.New("getMount operation timed out")
	_, err := NewMount(sw10FakeMount{existsErr: boom}, util.Config{})
	assert.ErrorIs(t, err, boom)
}

// TestNewMount_ErrMountpointNotFoundDoesNotAbort is the trap this sweep had
// to avoid: ErrMountpointNotFound is Exists' "ran and found nothing"
// outcome for the everyday case of `syver add mount /some/ordinary/path`,
// not a lookup failure. Propagating it like every other error would turn
// the common case into a hard failure.
func TestNewMount_ErrMountpointNotFoundDoesNotAbort(t *testing.T) {
	m, err := NewMount(sw10FakeMount{existsErr: system.ErrMountpointNotFound}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, m.Exists, matcher(false))
}

func TestNewMount_GenuinelyAbsentStillOmitsNoError(t *testing.T) {
	m, err := NewMount(sw10FakeMount{}, util.Config{})
	assert.NilError(t, err)
	assert.Equal(t, m.Exists, matcher(false))
}
