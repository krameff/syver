package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

type Mount struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	MountPoint    string  `json:"mountpoint,omitempty" yaml:"mountpoint,omitempty"`
	Exists        matcher `json:"exists" yaml:"exists"`
	Opts          matcher `json:"opts,omitempty" yaml:"opts,omitempty"`
	VfsOpts       matcher `json:"vfs-opts,omitempty" yaml:"vfs-opts,omitempty"`
	Source        matcher `json:"source,omitempty" yaml:"source,omitempty"`
	Filesystem    matcher `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Timeout       int     `json:"timeout" yaml:"timeout"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
	Usage         matcher `json:"usage,omitempty" yaml:"usage,omitempty"`
}

const (
	MountResourceKey  = "mount"
	MountResourceName = "Mount"
)

func init() {
	Register(Descriptor{
		Key:          MountResourceKey,
		Name:         MountResourceName,
		New:          func() Resource { return &Mount{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Mount{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{&cli.DurationFlag{Name: "timeout", Value: 1000 * time.Millisecond}}
		},
	})
}

func (m *Mount) ID() string {
	if m.MountPoint != "" && m.MountPoint != m.id {
		return fmt.Sprintf("%s: %s", m.id, m.MountPoint)
	}
	return m.id
}
func (m *Mount) SetID(id string)  { m.id = id }
func (m *Mount) SetSkip()         { m.Skip = true }
func (m *Mount) TypeKey() string  { return MountResourceKey }
func (m *Mount) TypeName() string { return MountResourceName }

// FIXME: Can this be refactored?
func (m *Mount) GetTitle() string { return m.Title }
func (m *Mount) GetMeta() meta    { return m.Meta }
func (m *Mount) GetMountPoint() string {
	if m.MountPoint != "" {
		return m.MountPoint
	}
	return m.id
}

func (m *Mount) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, m.ID())
	skip := m.Skip

	if m.Timeout == 0 {
		m.Timeout = 1000
	}

	sysMount := sys.NewMount(ctx, m.GetMountPoint(), sys, util.Config{Timeout: time.Duration(m.Timeout) * time.Millisecond})

	var results []TestResult
	results = append(results, ValidateValue(m, "exists", m.Exists, sysMount.Exists, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(m.Opts, fmt.Sprintf("%s: mount.opts", m.ID()), skip) {
		results = append(results, ValidateValue(m, "opts", m.Opts, sysMount.Opts, skip))
	}
	if isSetWarnEmpty(m.VfsOpts, fmt.Sprintf("%s: mount.vfs-opts", m.ID()), skip) {
		results = append(results, ValidateValue(m, "vfs-opts", m.VfsOpts, sysMount.VfsOpts, skip))
	}
	if isSet(m.Source) {
		results = append(results, ValidateValue(m, "source", m.Source, sysMount.Source, skip))
	}
	if isSet(m.Filesystem) {
		results = append(results, ValidateValue(m, "filesystem", m.Filesystem, sysMount.Filesystem, skip))
	}
	if isSet(m.Usage) {
		results = append(results, ValidateValue(m, "usage", m.Usage, sysMount.Usage, skip))
	}
	return results
}

func NewMount(sysMount system.Mount, config util.Config) (*Mount, error) {
	mountPoint := sysMount.MountPoint()
	exists, _ := sysMount.Exists()
	m := &Mount{
		id:      mountPoint,
		Exists:  exists,
		Timeout: config.TimeOutMilliSeconds(),
	}
	if !contains(config.IgnoreList, "opts") {
		if opts, err := sysMount.Opts(); err == nil {
			m.Opts = opts
		}
	}
	if !contains(config.IgnoreList, "vfs-opts") {
		if vfsOpts, err := sysMount.VfsOpts(); err == nil {
			m.VfsOpts = vfsOpts
		}
	}
	if !contains(config.IgnoreList, "source") {
		if source, err := sysMount.Source(); err == nil {
			m.Source = source
		}
	}
	if !contains(config.IgnoreList, "filesystem") {
		if filesystem, err := sysMount.Filesystem(); err == nil {
			m.Filesystem = filesystem
		}
	}
	return m, nil
}

// fromSystem builds a fresh Mount from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewMount" off
// *system.System from a type parameter alone.
func (m *Mount) fromSystem(sys *system.System, key string, config util.Config) (system.Mount, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewMount(ctx, key, sys, config)
	n, err := NewMount(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*m = *n
	return sysRes, nil
}
