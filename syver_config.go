package syver

import (
	"log"
	"reflect"

	"github.com/krameff/syver/resource"
)

type SyverConfig struct {
	Discovery  DiscoveryConfig       `json:"discovery,omitempty" yaml:"discovery,omitempty"`
	Files      resource.FileMap      `json:"file,omitempty" yaml:"file,omitempty"`
	Packages   resource.PackageMap   `json:"package,omitempty" yaml:"package,omitempty"`
	Addrs      resource.AddrMap      `json:"addr,omitempty" yaml:"addr,omitempty"`
	Ports      resource.PortMap      `json:"port,omitempty" yaml:"port,omitempty"`
	Services   resource.ServiceMap   `json:"service,omitempty" yaml:"service,omitempty"`
	Users      resource.UserMap      `json:"user,omitempty" yaml:"user,omitempty"`
	Groups     resource.GroupMap     `json:"group,omitempty" yaml:"group,omitempty"`
	Commands   resource.CommandMap   `json:"command,omitempty" yaml:"command,omitempty"`
	DNS        resource.DNSMap       `json:"dns,omitempty" yaml:"dns,omitempty"`
	Processes  resource.ProcessMap   `json:"process,omitempty" yaml:"process,omitempty"`
	Syverfiles resource.SyverfileMap `json:"gossfile,omitempty" yaml:"gossfile,omitempty"`
	// SyverfileAlias is an input-only alias for Syverfiles: a gossfile
	// written with `syverfile:` entries decodes here, gets folded into
	// Syverfiles (see ReadJSONData in store.go), and is then nil'd out --
	// it is never itself written back out (omitempty drops it once nil).
	//
	// NOTE (FEAT-007): this field intentionally has no dispatch.go
	// accessor entry and is never touched by the configAccessors loops
	// below -- it isn't a registered resource type, it's pure store.go
	// decode plumbing (see store.go:255-268). Handled explicitly here,
	// same as before the refactor.
	SyverfileAlias resource.SyverfileMap   `json:"syverfile,omitempty" yaml:"syverfile,omitempty"`
	KernelParams   resource.KernelParamMap `json:"kernel-param,omitempty" yaml:"kernel-param,omitempty"`
	Mounts         resource.MountMap       `json:"mount,omitempty" yaml:"mount,omitempty"`
	Interfaces     resource.InterfaceMap   `json:"interface,omitempty" yaml:"interface,omitempty"`
	HTTPs          resource.HTTPMap        `json:"http,omitempty" yaml:"http,omitempty"`
	Matchings      resource.MatchingMap    `json:"matching,omitempty" yaml:"matching,omitempty"`
	Registries     resource.RegistryMap    `json:"registry,omitempty" yaml:"registry,omitempty"`
}

// NewSyverConfig builds an empty SyverConfig with every map field
// initialised (FEAT-007: replaces two 16-entry make() blocks with a loop
// over configAccessors/discoveryAccessors). gossfile has no accessor Make
// (see the accessor table's comment in dispatch.go) so Syverfiles and
// SyverfileAlias are still made explicitly, exactly as before.
func NewSyverConfig() *SyverConfig {
	c := &SyverConfig{
		Syverfiles:     make(resource.SyverfileMap),
		SyverfileAlias: make(resource.SyverfileMap),
	}
	for _, key := range fieldOrder {
		configAccessors[key].Make(c)
		discoveryAccessors[key].Make(&c.Discovery)
	}
	return c
}

// Merge consumes all the resources in g2 into c, duplicate resources
// will be overwritten with the ones in g2
func (c *SyverConfig) Merge(g2 SyverConfig) {
	c.Discovery.Merge(g2.Discovery)

	for _, key := range fieldOrder {
		if acc := configAccessors[key]; acc.Merge != nil {
			acc.Merge(c, &g2)
		}
	}
}

func mergeType[V any](m map[string]V, t, k string, v V) {
	if _, ok := m[k]; ok {
		log.Printf("[WARN] Duplicate key detected: '%s: %s'. The value from a later-loaded goss file has overwritten the previous value.", t, k)
	}
	m[k] = v
}

// Resources returns every validation-eligible resource (FEAT-007: replaces
// the 16-argument genericConcatMaps call with a loop over resourceOrder;
// see the accessor table's InValidation field, which is what "eligible"
// means here -- gossfile is the one registered type excluded).
func (c *SyverConfig) Resources() []resource.Resource {
	var tests []resource.Resource

	for _, key := range resourceOrder {
		m := configAccessors[key].Get(c)
		for _, t := range m {
			// This type assertion is now checked at boot by dispatch.go's
			// init() guard (every InValidation resource type must resolve
			// through here), which is the resolution of the old
			// "// FIXME: Can this be moved to a safer compile-time check?"
			tests = append(tests, t.(resource.Resource))
		}
	}

	return tests
}

func interfaceMap(slice any) map[string]any {
	m := reflect.ValueOf(slice)
	if m.Kind() != reflect.Map {
		panic("InterfaceSlice() given a non-slice type")
	}

	ret := make(map[string]any)

	for _, k := range m.MapKeys() {
		ret[k.Interface().(string)] = m.MapIndex(k).Interface()
	}

	return ret
}

func mergeSyver(g1, g2 SyverConfig) SyverConfig {
	g1.Syverfiles = nil

	g1.Merge(g2)

	return g1
}
