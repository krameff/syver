package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

type Addr struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Address       string  `json:"address,omitempty" yaml:"address,omitempty"`
	LocalAddress  string  `json:"local-address,omitempty" yaml:"local-address,omitempty"`
	Reachable     matcher `json:"reachable" yaml:"reachable"`
	Timeout       int     `json:"timeout" yaml:"timeout"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

type idKey struct{}

const (
	AddrResourceKey  = "addr"
	AddrResourceName = "Addr"
)

func init() {
	Register(Descriptor{
		Key:          AddrResourceKey,
		Name:         AddrResourceName,
		New:          func() Resource { return &Addr{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &Addr{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{&cli.DurationFlag{Name: "timeout", Value: 500 * time.Millisecond}}
		},
	})
}

func (a *Addr) ID() string {
	if a.Address != "" && a.Address != a.id {
		return fmt.Sprintf("%s: %s", a.id, a.Address)
	}
	return a.id
}
func (a *Addr) SetID(id string)  { a.id = id }
func (a *Addr) SetSkip()         { a.Skip = true }
func (a *Addr) TypeKey() string  { return AddrResourceKey }
func (a *Addr) TypeName() string { return AddrResourceName }

// FIXME: Can this be refactored?
func (a *Addr) GetTitle() string { return a.Title }
func (a *Addr) GetMeta() meta    { return a.Meta }
func (a *Addr) GetAddress() string {
	if a.Address != "" {
		return a.Address
	}
	return a.id
}

func (a *Addr) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, a.ID())
	skip := a.Skip

	if a.Timeout == 0 {
		a.Timeout = 500
	}

	sysAddr := sys.NewAddr(ctx, a.GetAddress(), sys, util.Config{Timeout: time.Duration(a.Timeout) * time.Millisecond, LocalAddress: a.LocalAddress})

	var results []TestResult
	results = append(results, ValidateValue(a, "reachable", a.Reachable, sysAddr.Reachable, skip))
	return results
}

func NewAddr(sysAddr system.Addr, config util.Config) (*Addr, error) {
	address := sysAddr.Address()
	reachable, err := sysAddr.Reachable()
	a := &Addr{
		id:           address,
		Reachable:    reachable,
		Timeout:      config.TimeOutMilliSeconds(),
		LocalAddress: config.LocalAddress,
	}
	return a, err
}

// fromSystem builds a fresh Addr from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewAddr" off
// *system.System from a type parameter alone.
func (a *Addr) fromSystem(sys *system.System, key string, config util.Config) (system.Addr, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewAddr(ctx, key, sys, config)
	n, err := NewAddr(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*a = *n
	return sysRes, nil
}
