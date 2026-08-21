package syver

import (
	"fmt"

	"github.com/krameff/syver/resource"
)

// DiscoveryConfig holds discovery-phase tests keyed by resource type.
type DiscoveryConfig struct {
	Files        resource.FileMap        `json:"file,omitempty" yaml:"file,omitempty"`
	Packages     resource.PackageMap     `json:"package,omitempty" yaml:"package,omitempty"`
	Addrs        resource.AddrMap        `json:"addr,omitempty" yaml:"addr,omitempty"`
	Ports        resource.PortMap        `json:"port,omitempty" yaml:"port,omitempty"`
	Services     resource.ServiceMap     `json:"service,omitempty" yaml:"service,omitempty"`
	Users        resource.UserMap        `json:"user,omitempty" yaml:"user,omitempty"`
	Groups       resource.GroupMap       `json:"group,omitempty" yaml:"group,omitempty"`
	Commands     resource.CommandMap     `json:"command,omitempty" yaml:"command,omitempty"`
	DNS          resource.DNSMap         `json:"dns,omitempty" yaml:"dns,omitempty"`
	Processes    resource.ProcessMap     `json:"process,omitempty" yaml:"process,omitempty"`
	KernelParams resource.KernelParamMap `json:"kernel-param,omitempty" yaml:"kernel-param,omitempty"`
	Mounts       resource.MountMap       `json:"mount,omitempty" yaml:"mount,omitempty"`
	Interfaces   resource.InterfaceMap   `json:"interface,omitempty" yaml:"interface,omitempty"`
	HTTPs        resource.HTTPMap        `json:"http,omitempty" yaml:"http,omitempty"`
	Matchings    resource.MatchingMap    `json:"matching,omitempty" yaml:"matching,omitempty"`
	Registries   resource.RegistryMap    `json:"registry,omitempty" yaml:"registry,omitempty"`
}

// DiscoveryEntry pairs a register name with a discovery resource.
type DiscoveryEntry struct {
	Register string
	Resource resource.Resource
}

func (c *DiscoveryConfig) Entries() ([]DiscoveryEntry, error) {
	var entries []DiscoveryEntry

	for _, key := range resourceOrder {
		acc, ok := discoveryAccessors[key]
		if !ok {
			continue // defensive; every resourceOrder key has a discoveryAccessors entry today
		}
		for _, t := range acc.Get(c) {
			res := t.(resource.Resource)
			register := res.GetRegister()
			if register == "" {
				id := res.TypeKey()
				if rr, ok := res.(resource.ResourceRead); ok {
					id = fmt.Sprintf("%s:%s", res.TypeKey(), rr.ID())
				}
				return nil, fmt.Errorf("discovery resource %s missing required register attribute", id)
			}
			for _, existing := range entries {
				if existing.Register == register {
					return nil, fmt.Errorf("duplicate discovery register %q", register)
				}
			}
			entries = append(entries, DiscoveryEntry{
				Register: register,
				Resource: res,
			})
		}
	}

	return entries, nil
}

func (c *DiscoveryConfig) IsEmpty() bool {
	entries, err := c.entriesWithoutValidation()
	if err != nil {
		return true
	}
	return len(entries) == 0
}

func (c *DiscoveryConfig) entriesWithoutValidation() ([]DiscoveryEntry, error) {
	var entries []DiscoveryEntry

	for _, key := range resourceOrder {
		acc, ok := discoveryAccessors[key]
		if !ok {
			continue // defensive; see Entries() above
		}
		for _, t := range acc.Get(c) {
			res := t.(resource.Resource)
			entries = append(entries, DiscoveryEntry{
				Register: res.GetRegister(),
				Resource: res,
			})
		}
	}

	return entries, nil
}

func (c *DiscoveryConfig) Merge(g2 DiscoveryConfig) {
	for _, key := range fieldOrder {
		if acc, ok := discoveryAccessors[key]; ok {
			acc.Merge(c, &g2)
		}
	}
}
