package system

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	allOpts := splitMountInfo(m.mountInfo.Options)

	return lo.Uniq(allOpts), nil
}

func (m *DefMount) VfsOpts() ([]string, error) {
	if err := m.setup(); err != nil {
		return nil, err
	}
	opts := splitMountInfo(m.mountInfo.VFSOptions)
	return opts, nil
}

func (m *DefMount) Source() (string, error) {
	if err := m.setup(); err != nil {
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

func getMount(mountpoint string, timeout int) (*mountinfo.Info, error) {
	c1 := make(chan *mountinfo.Info, 1)
	e1 := make(chan error, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond

	go func() {
		entries, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(mountpoint))
		if err != nil {
			e1 <- err
			return
		}
		if len(entries) == 0 {
			e1 <- ErrMountpointNotFound
			return
		}
		c1 <- entries[0]
	}()

	select {
	case result := <-c1:
		return result, nil
	case err := <-e1:
		return nil, err
	case <-time.After(timeoutD):
		return nil, fmt.Errorf("getMount operation timed out after %s milliseconds", timeoutD)
	}

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
