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

	gm := genericConcatMaps(c.Commands,
		c.HTTPs,
		c.Addrs,
		c.DNS,
		c.Packages,
		c.Services,
		c.Files,
		c.Processes,
		c.Users,
		c.Groups,
		c.Ports,
		c.KernelParams,
		c.Mounts,
		c.Interfaces,
		c.Matchings,
		c.Registries,
	)

	for _, m := range gm {
		for _, t := range m {
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

	gm := genericConcatMaps(c.Commands,
		c.HTTPs,
		c.Addrs,
		c.DNS,
		c.Packages,
		c.Services,
		c.Files,
		c.Processes,
		c.Users,
		c.Groups,
		c.Ports,
		c.KernelParams,
		c.Mounts,
		c.Interfaces,
		c.Matchings,
		c.Registries,
	)

	for _, m := range gm {
		for _, t := range m {
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
	for k, v := range g2.Files {
		mergeType(c.Files, "file", k, v)
	}
	for k, v := range g2.Packages {
		mergeType(c.Packages, "package", k, v)
	}
	for k, v := range g2.Addrs {
		mergeType(c.Addrs, "addr", k, v)
	}
	for k, v := range g2.Ports {
		mergeType(c.Ports, "port", k, v)
	}
	for k, v := range g2.Services {
		mergeType(c.Services, "service", k, v)
	}
	for k, v := range g2.Users {
		mergeType(c.Users, "user", k, v)
	}
	for k, v := range g2.Groups {
		mergeType(c.Groups, "group", k, v)
	}
	for k, v := range g2.Commands {
		mergeType(c.Commands, "command", k, v)
	}
	for k, v := range g2.DNS {
		mergeType(c.DNS, "dns", k, v)
	}
	for k, v := range g2.Processes {
		mergeType(c.Processes, "process", k, v)
	}
	for k, v := range g2.KernelParams {
		mergeType(c.KernelParams, "kernel-param", k, v)
	}
	for k, v := range g2.Mounts {
		mergeType(c.Mounts, "mount", k, v)
	}
	for k, v := range g2.Interfaces {
		mergeType(c.Interfaces, "interface", k, v)
	}
	for k, v := range g2.HTTPs {
		mergeType(c.HTTPs, "http", k, v)
	}
	for k, v := range g2.Matchings {
		mergeType(c.Matchings, "matching", k, v)
	}
	for k, v := range g2.Registries {
		mergeType(c.Registries, "registry", k, v)
	}
}
