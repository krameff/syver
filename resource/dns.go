package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

type DNS struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string  `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta    `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string  `json:"-" yaml:"-"`
	Resolve       string  `json:"resolve,omitempty" yaml:"resolve,omitempty"`
	Resolveable   matcher `json:"resolveable,omitempty" yaml:"resolveable,omitempty"`
	Resolvable    matcher `json:"resolvable" yaml:"resolvable"`
	Addrs         matcher `json:"addrs,omitempty" yaml:"addrs,omitempty"`
	Timeout       int     `json:"timeout" yaml:"timeout"`
	Server        string  `json:"server,omitempty" yaml:"server,omitempty"`
	Skip          bool    `json:"skip,omitempty" yaml:"skip,omitempty"`
}

const (
	DNSResourceKey  = "dns"
	DNSResourceName = "DNS"
)

func init() {
	Register(Descriptor{
		Key:          DNSResourceKey,
		Name:         DNSResourceName,
		New:          func() Resource { return &DNS{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &DNS{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{
				&cli.DurationFlag{Name: "timeout", Value: 500 * time.Millisecond},
				&cli.StringFlag{Name: "server", Usage: "The IP address of a DNS server to query"},
			}
		},
	})
}

func (d *DNS) ID() string {
	if d.Resolve != "" && d.Resolve != d.id {
		return fmt.Sprintf("%s: %s", d.id, d.Resolve)
	}
	return d.id
}
func (d *DNS) SetID(id string)  { d.id = id }
func (d *DNS) SetSkip()         { d.Skip = true }
func (d *DNS) TypeKey() string  { return DNSResourceKey }
func (d *DNS) TypeName() string { return DNSResourceName }
func (d *DNS) GetTitle() string { return d.Title }
func (d *DNS) GetMeta() meta    { return d.Meta }
func (d *DNS) GetResolve() string {
	if d.Resolve != "" {
		return d.Resolve
	}
	return d.id
}

func (d *DNS) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, d.ID())
	skip := d.Skip
	if d.Timeout == 0 {
		d.Timeout = 500
	}

	sysDNS := sys.NewDNS(ctx, d.GetResolve(), sys, util.Config{Timeout: time.Duration(d.Timeout) * time.Millisecond, Server: d.Server})

	var results []TestResult
	// Backwards compatibility hack for now
	if d.Resolvable == nil {
		d.Resolvable = d.Resolveable
	}
	results = append(results, ValidateValue(d, "resolvable", d.Resolvable, sysDNS.Resolvable, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(d.Addrs, fmt.Sprintf("%s: dns.addrs", d.ID()), skip) {
		results = append(results, ValidateValue(d, "addrs", d.Addrs, sysDNS.Addrs, skip))
	}
	return results
}

func NewDNS(sysDNS system.DNS, config util.Config) (*DNS, error) {
	var host string
	if sysDNS.Qtype() != "" {
		host = strings.Join([]string{sysDNS.Qtype(), sysDNS.Host()}, ":")
	} else {
		host = sysDNS.Host()
	}

	resolvable, err := sysDNS.Resolvable()
	server := sysDNS.Server()

	d := &DNS{
		id:         host,
		Resolvable: resolvable,
		Timeout:    config.TimeOutMilliSeconds(),
		Server:     server,
	}
	if !contains(config.IgnoreList, "addrs") {
		addrs, _ := sysDNS.Addrs()
		d.Addrs = addrs
	}
	return d, err
}

// fromSystem builds a fresh DNS from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewDNS" off
// *system.System from a type parameter alone.
func (d *DNS) fromSystem(sys *system.System, key string, config util.Config) (system.DNS, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewDNS(ctx, key, sys, config)
	n, err := NewDNS(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*d = *n
	return sysRes, nil
}
