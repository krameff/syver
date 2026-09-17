package system

import (
	"context"
	"errors"
	"strings"

	"github.com/krameff/syver/util"
	"github.com/moby/sys/mountinfo"
	"github.com/samber/lo"
)

// ErrMountpointNotFound is returned by getMount (and therefore by Exists)
// when the platform lookup ran and genuinely found no such mountpoint --
// the common, everyday case for a path that simply isn't a separate mount.
// Exported (was errMountpointNotFound) so resource/mount.go's `add` path
// can distinguish this expected outcome from a genuine lookup failure
// (e.g. a timeout) rather than propagating both identically as a hard
// error. See FEAT-010 SW-10 / Trap 1's reasoning, applied here.
var ErrMountpointNotFound = errors.New("mountpoint not found")

// ErrMountUnsupported is returned when a mount lookup cannot be performed at
// all, as distinct from a lookup that ran and found nothing
// (ErrMountpointNotFound). Following the sentinel-error idiom already used by
// registry (ErrRegistryUnsupported), package (ErrNullPackage) and file
// (ErrFileOwnershipUnsupported).
//
// On Windows it is what a mountpoint that is not a drive letter unwraps to:
// mount: covers drive letters only there, and a folder mount point is a scope
// limit, not a missing path. Returning ErrMountpointNotFound for it would blame
// the operator's path, which is the defect FEAT-013 removed when every Windows
// mount check still failed that way.
var ErrMountUnsupported = errors.New("mount: not supported on this platform")

// ErrMountAttributeUnsupported is returned by an attribute that has no meaning
// on the platform: opts, vfs-opts and source on Windows. An empty value with a
// nil error would let `opts: []` pass there for the wrong reason.
var ErrMountAttributeUnsupported = errors.New("mount: attribute not supported on this platform")

type Mount interface {
	MountPoint() string
	Exists() (bool, error)
	Opts() ([]string, error)
	VfsOpts() ([]string, error)
	Source() (string, error)
	Filesystem() (string, error)
	Usage() (int, error)
}

type DefMount struct {
	mountPoint string
	loaded     bool
	exists     bool
	mountInfo  *mountinfo.Info
	usage      int
	Timeout    int
	err        error
}

func NewDefMount(_ context.Context, mountPoint string, system *System, config util.Config) Mount {
	return &DefMount{
		mountPoint: mountPoint,
		Timeout:    config.TimeOutMilliSeconds(),
	}
}

func (m *DefMount) setup() error {
	if m.loaded {
		return m.err
	}
	m.loaded = true

	mountInfo, err := getMount(m.mountPoint, m.Timeout)
	if err != nil {
		m.exists = false
		m.err = err
		return m.err
	}
	m.mountInfo = mountInfo
	m.exists = true

	usage, err := getUsage(m.mountPoint)
	if err != nil {
		m.err = err
		return m.err
	}
	m.usage = usage

	return nil
}

func (m *DefMount) ID() string {
	return m.mountPoint
}

func (m *DefMount) MountPoint() string {
	return m.mountPoint
}

func (m *DefMount) Exists() (bool, error) {
	if err := m.setup(); err != nil {
		return false, err
	}
	return m.exists, nil
}

func (m *DefMount) Opts() ([]string, error) {
	if err := m.setup(); err != nil {
		return nil, err
	}
	if err := mountAttributeSupported("opts"); err != nil {
		return nil, err
	}
	allOpts := splitMountInfo(m.mountInfo.Options)

	return lo.Uniq(allOpts), nil
}

func (m *DefMount) VfsOpts() ([]string, error) {
	if err := m.setup(); err != nil {
		return nil, err
	}
	if err := mountAttributeSupported("vfs-opts"); err != nil {
		return nil, err
	}
	opts := splitMountInfo(m.mountInfo.VFSOptions)
	return opts, nil
}

func (m *DefMount) Source() (string, error) {
	if err := m.setup(); err != nil {
		return "", err
	}
	if err := mountAttributeSupported("source"); err != nil {
		return "", err
	}

	return m.mountInfo.Source, nil
}

func (m *DefMount) Filesystem() (string, error) {
	if err := m.setup(); err != nil {
		return "", err
	}

	return m.mountInfo.FSType, nil
}

func (m *DefMount) Usage() (int, error) {
	if err := m.setup(); err != nil {
		return -1, err
	}

	return m.usage, nil
}

func splitMountInfo(s string) []string {
	quoted := false
	return strings.FieldsFunc(s, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == ','
	})
}
