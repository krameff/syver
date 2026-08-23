package resource

import (
	"context"
	"fmt"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type Group struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Groupname     string  `json:"groupname,omitempty" yaml:"groupname,omitempty"`
	Exists        matcher `json:"exists" yaml:"exists"`
	GID           matcher `json:"gid,omitempty" yaml:"gid,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	GroupResourceKey  = "group"
	GroupResourceName = "Group"
)

func init() {
	Register(Descriptor{
		Key:          GroupResourceKey,
		Name:         GroupResourceName,
		New:          func() Resource { return &Group{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Group{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		AutoAdd: &AutoAddSpec{Order: 2},
	})
}

func (g *Group) ID() string {
	if g.Groupname != "" && g.Groupname != g.id {
		return fmt.Sprintf("%s: %s", g.id, g.Groupname)
	}
	return g.id
}
func (g *Group) SetID(id string)  { g.id = id }
func (g *Group) SetSkip()         { g.Skip = true }
func (g *Group) TypeKey() string  { return GroupResourceKey }
func (g *Group) TypeName() string { return GroupResourceName }
func (g *Group) GetTitle() string { return g.Title }
func (g *Group) GetMeta() meta    { return g.Meta }
func (g *Group) GetGroupname() string {
	if g.Groupname != "" {
		return g.Groupname
	}
	return g.id
}

func (g *Group) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, g.ID())
	skip := g.Skip
	sysgroup := sys.NewGroup(ctx, g.GetGroupname(), sys, util.Config{})

	var results []TestResult
	results = append(results, ValidateValue(g, "exists", g.Exists, sysgroup.Exists, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSet(g.GID) {
		gGID := deprecateAtoI(g.GID, fmt.Sprintf("%s: group.gid", g.ID()))
		results = append(results, ValidateValue(g, "gid", gGID, sysgroup.GID, skip))
	}
	return results
}

func NewGroup(sysGroup system.Group, config util.Config) (*Group, error) {
	groupname := sysGroup.Groupname()
	exists, _ := sysGroup.Exists()
	g := &Group{
		id:     groupname,
		Exists: exists,
	}
	if !contains(config.IgnoreList, "stderr") {
		if gid, err := sysGroup.GID(); err == nil {
			g.GID = gid
		}
	}
	return g, nil
}

// fromSystem builds a fresh Group from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewGroup" off
// *system.System from a type parameter alone.
func (g *Group) fromSystem(sys *system.System, key string, config util.Config) (system.Group, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewGroup(ctx, key, sys, config)
	n, err := NewGroup(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*g = *n
	return sysRes, nil
}
