package syver

import (
	"fmt"

	"github.com/krameff/syver/resource"
)

// fieldOrder mirrors SyverConfig/DiscoveryConfig's struct declaration
// order. Used by NewSyverConfig()'s Make() loop and SyverConfig.Merge() /
// DiscoveryConfig.Merge() -- preserved even though nothing observably
// depends on it (Make has no side effects to order, and Merge's only
// observable ordering effect is the sequence of "[WARN] Duplicate key"
// log lines), simply because there's no reason to introduce a difference.
var fieldOrder = []string{
	"file", "package", "addr", "port", "service", "user", "group",
	"command", "dns", "process", "kernel-param", "mount", "interface",
	"http", "matching", "registry",
}

// resourceOrder mirrors the old genericConcatMaps() call order in
// SyverConfig.Resources() and DiscoveryConfig.Entries()/
// entriesWithoutValidation() -- a different order than fieldOrder. Kept
// exact for safety: Resources()'s result feeds the dependency scheduler,
// and while final output ordering downstream is normally driven by
// TestResult.SortKey() (see outputs.go's foSort), preserving the original
// type-group order removes any doubt.
var resourceOrder = []string{
	"command", "http", "addr", "dns", "package", "service", "file",
	"process", "user", "group", "port", "kernel-param", "mount",
	"interface", "matching", "registry",
}

// accessor is the one remaining per-type enumeration (FEAT-007 S4.3): every
// other switch/list/map literal that used to be duplicated once per
// resource type across syver_config.go, discovery_config.go and add.go now
// drives off configAccessors/discoveryAccessors instead.
//
// Field exists because of FEAT-007 G1: Get returns a *copy* (built by
// genericConcatMaps/interfaceMap, which allocates a new map by reflection),
// which is fine for iteration (Resources(), Merge()) but wrong for `syver
// add` -- add.go must mutate syverConfig.<Field> itself, or newly added
// resources silently stop being written to the file. Field returns a
// pointer to the live struct field instead (e.g. &c.Ports), for
// resource.UpsertLive to write through.
type accessor struct {
	// Make initialises this field's map on a fresh SyverConfig (replaces
	// syver_config.go's two 16-entry make() blocks).
	Make func(c *SyverConfig)
	// Get returns a *copy* of this field as a generic map, for iteration
	// only (SyverConfig.Resources(), DiscoveryConfig.Entries()). Never use
	// this to mutate -- see Field.
	Get func(c *SyverConfig) map[string]any
	// Field returns a pointer to the live struct field itself (e.g.
	// &c.Ports), for callers that need to insert into it directly (`syver
	// add`, via resource.UpsertLive). See FEAT-007 G1.
	Field func(c *SyverConfig) any
	// Merge folds src's entries for this type into dst, using the existing
	// mergeType warn-on-duplicate helper.
	Merge func(dst, src *SyverConfig)
}

// configAccessors covers every registered resource type (including
// "gossfile" and "matching") -- one entry per SyverConfig field. gossfile's
// Make/Get/Merge are deliberately nil: it's excluded from
// SyverConfig.Resources() (InValidation: false) and has its own special
// merge handling (mergeSyver nils Syverfiles before the generic Merge loop
// runs -- see §4.6), so there is nothing generic for those three to do for
// it. Field is set for every entry, including gossfile, because `syver add
// syver` still needs to reach the live Syverfiles field.
var configAccessors = map[string]accessor{
	"file": {
		Make:  func(c *SyverConfig) { c.Files = make(resource.FileMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Files) },
		Field: func(c *SyverConfig) any { return &c.Files },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Files {
				mergeType(dst.Files, "file", k, v)
			}
		},
	},
	"package": {
		Make:  func(c *SyverConfig) { c.Packages = make(resource.PackageMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Packages) },
		Field: func(c *SyverConfig) any { return &c.Packages },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Packages {
				mergeType(dst.Packages, "package", k, v)
			}
		},
	},
	"addr": {
		Make:  func(c *SyverConfig) { c.Addrs = make(resource.AddrMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Addrs) },
		Field: func(c *SyverConfig) any { return &c.Addrs },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Addrs {
				mergeType(dst.Addrs, "addr", k, v)
			}
		},
	},
	"port": {
		Make:  func(c *SyverConfig) { c.Ports = make(resource.PortMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Ports) },
		Field: func(c *SyverConfig) any { return &c.Ports },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Ports {
				mergeType(dst.Ports, "port", k, v)
			}
		},
	},
	"service": {
		Make:  func(c *SyverConfig) { c.Services = make(resource.ServiceMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Services) },
		Field: func(c *SyverConfig) any { return &c.Services },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Services {
				mergeType(dst.Services, "service", k, v)
			}
		},
	},
	"user": {
		Make:  func(c *SyverConfig) { c.Users = make(resource.UserMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Users) },
		Field: func(c *SyverConfig) any { return &c.Users },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Users {
				mergeType(dst.Users, "user", k, v)
			}
		},
	},
	"group": {
		Make:  func(c *SyverConfig) { c.Groups = make(resource.GroupMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Groups) },
		Field: func(c *SyverConfig) any { return &c.Groups },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Groups {
				mergeType(dst.Groups, "group", k, v)
			}
		},
	},
	"command": {
		Make:  func(c *SyverConfig) { c.Commands = make(resource.CommandMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Commands) },
		Field: func(c *SyverConfig) any { return &c.Commands },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Commands {
				mergeType(dst.Commands, "command", k, v)
			}
		},
	},
	"dns": {
		Make:  func(c *SyverConfig) { c.DNS = make(resource.DNSMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.DNS) },
		Field: func(c *SyverConfig) any { return &c.DNS },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.DNS {
				mergeType(dst.DNS, "dns", k, v)
			}
		},
	},
	"process": {
		Make:  func(c *SyverConfig) { c.Processes = make(resource.ProcessMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Processes) },
		Field: func(c *SyverConfig) any { return &c.Processes },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Processes {
				mergeType(dst.Processes, "process", k, v)
			}
		},
	},
	"gossfile": {
		Field: func(c *SyverConfig) any { return &c.Syverfiles },
	},
	"kernel-param": {
		Make:  func(c *SyverConfig) { c.KernelParams = make(resource.KernelParamMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.KernelParams) },
		Field: func(c *SyverConfig) any { return &c.KernelParams },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.KernelParams {
				mergeType(dst.KernelParams, "kernel-param", k, v)
			}
		},
	},
	"mount": {
		Make:  func(c *SyverConfig) { c.Mounts = make(resource.MountMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Mounts) },
		Field: func(c *SyverConfig) any { return &c.Mounts },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Mounts {
				mergeType(dst.Mounts, "mount", k, v)
			}
		},
	},
	"interface": {
		Make:  func(c *SyverConfig) { c.Interfaces = make(resource.InterfaceMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Interfaces) },
		Field: func(c *SyverConfig) any { return &c.Interfaces },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Interfaces {
				mergeType(dst.Interfaces, "interface", k, v)
			}
		},
	},
	"http": {
		Make:  func(c *SyverConfig) { c.HTTPs = make(resource.HTTPMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.HTTPs) },
		Field: func(c *SyverConfig) any { return &c.HTTPs },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.HTTPs {
				mergeType(dst.HTTPs, "http", k, v)
			}
		},
	},
	"matching": {
		Make:  func(c *SyverConfig) { c.Matchings = make(resource.MatchingMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Matchings) },
		Field: func(c *SyverConfig) any { return &c.Matchings },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Matchings {
				mergeType(dst.Matchings, "matching", k, v)
			}
		},
	},
	"registry": {
		Make:  func(c *SyverConfig) { c.Registries = make(resource.RegistryMap) },
		Get:   func(c *SyverConfig) map[string]any { return interfaceMap(c.Registries) },
		Field: func(c *SyverConfig) any { return &c.Registries },
		Merge: func(dst, src *SyverConfig) {
			for k, v := range src.Registries {
				mergeType(dst.Registries, "registry", k, v)
			}
		},
	},
}

// discoveryAccessor is discoveryAccessors' per-type record. Deliberately
// smaller than accessor: DiscoveryConfig has no live-field add path and no
// per-type Make() call site distinct from configAccessors' (NewSyverConfig
// builds both SyverConfig's and DiscoveryConfig's maps from the same
// resource.Descriptors() pass -- see syver_config.go), so only Get and
// Merge are needed here.
type discoveryAccessor struct {
	Make  func(c *DiscoveryConfig)
	Get   func(c *DiscoveryConfig) map[string]any
	Merge func(dst, src *DiscoveryConfig)
}

// discoveryAccessors covers exactly the registered types with InDiscovery
// true -- everything except gossfile, which has no DiscoveryConfig field at
// all (there's no "discover other gossfiles" concept).
var discoveryAccessors = map[string]discoveryAccessor{
	"file": {
		Make: func(c *DiscoveryConfig) { c.Files = make(resource.FileMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Files) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Files {
				mergeType(dst.Files, "file", k, v)
			}
		},
	},
	"package": {
		Make: func(c *DiscoveryConfig) { c.Packages = make(resource.PackageMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Packages) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Packages {
				mergeType(dst.Packages, "package", k, v)
			}
		},
	},
	"addr": {
		Make: func(c *DiscoveryConfig) { c.Addrs = make(resource.AddrMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Addrs) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Addrs {
				mergeType(dst.Addrs, "addr", k, v)
			}
		},
	},
	"port": {
		Make: func(c *DiscoveryConfig) { c.Ports = make(resource.PortMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Ports) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Ports {
				mergeType(dst.Ports, "port", k, v)
			}
		},
	},
	"service": {
		Make: func(c *DiscoveryConfig) { c.Services = make(resource.ServiceMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Services) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Services {
				mergeType(dst.Services, "service", k, v)
			}
		},
	},
	"user": {
		Make: func(c *DiscoveryConfig) { c.Users = make(resource.UserMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Users) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Users {
				mergeType(dst.Users, "user", k, v)
			}
		},
	},
	"group": {
		Make: func(c *DiscoveryConfig) { c.Groups = make(resource.GroupMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Groups) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Groups {
				mergeType(dst.Groups, "group", k, v)
			}
		},
	},
	"command": {
		Make: func(c *DiscoveryConfig) { c.Commands = make(resource.CommandMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Commands) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Commands {
				mergeType(dst.Commands, "command", k, v)
			}
		},
	},
	"dns": {
		Make: func(c *DiscoveryConfig) { c.DNS = make(resource.DNSMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.DNS) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.DNS {
				mergeType(dst.DNS, "dns", k, v)
			}
		},
	},
	"process": {
		Make: func(c *DiscoveryConfig) { c.Processes = make(resource.ProcessMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Processes) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Processes {
				mergeType(dst.Processes, "process", k, v)
			}
		},
	},
	"kernel-param": {
		Make: func(c *DiscoveryConfig) { c.KernelParams = make(resource.KernelParamMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.KernelParams) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.KernelParams {
				mergeType(dst.KernelParams, "kernel-param", k, v)
			}
		},
	},
	"mount": {
		Make: func(c *DiscoveryConfig) { c.Mounts = make(resource.MountMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Mounts) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Mounts {
				mergeType(dst.Mounts, "mount", k, v)
			}
		},
	},
	"interface": {
		Make: func(c *DiscoveryConfig) { c.Interfaces = make(resource.InterfaceMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Interfaces) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Interfaces {
				mergeType(dst.Interfaces, "interface", k, v)
			}
		},
	},
	"http": {
		Make: func(c *DiscoveryConfig) { c.HTTPs = make(resource.HTTPMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.HTTPs) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.HTTPs {
				mergeType(dst.HTTPs, "http", k, v)
			}
		},
	},
	"matching": {
		Make: func(c *DiscoveryConfig) { c.Matchings = make(resource.MatchingMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Matchings) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Matchings {
				mergeType(dst.Matchings, "matching", k, v)
			}
		},
	},
	"registry": {
		Make: func(c *DiscoveryConfig) { c.Registries = make(resource.RegistryMap) },
		Get:  func(c *DiscoveryConfig) map[string]any { return interfaceMap(c.Registries) },
		Merge: func(dst, src *DiscoveryConfig) {
			for k, v := range src.Registries {
				mergeType(dst.Registries, "registry", k, v)
			}
		},
	},
}

// unrecognizedResourceType is the single fall-through point for a lookup
// miss: every "this resource name isn't registered" case in `syver add`'s
// dispatch (add.go) funnels through here, returning the same
// "undefined resource name" error it always has. Keeping it in one named
// function means a future external-plugin lane can route an unrecognised
// key onward by replacing this body alone, without reopening add.go or the
// accessor tables above.
func unrecognizedResourceType(name string) error {
	return fmt.Errorf("undefined resource name: %s", name)
}

// checkAccessorExhaustiveness asserts configAccessors/discoveryAccessors
// and resource.Descriptors() cover exactly the same key sets, returning an
// error on the first mismatch found. This is the resolution of
// syver_config.go's old
// "// FIXME: Can this be moved to a safer compile-time check?" -- a
// resource type registered in the resource package but not wired here (or
// vice versa) is now a boot-time panic (via init() below) instead of a
// silent gap.
//
// Split out from init() as its own function (returning an error rather
// than panicking directly) so it's callable from a test after deliberately
// breaking one of the tables -- see TestAccessorExhaustivenessGuard
// (FEAT-007 AC-5/T-3).
func checkAccessorExhaustiveness() error {
	descs := resource.Descriptors()

	for key, d := range descs {
		acc, ok := configAccessors[key]
		if !ok {
			return fmt.Errorf("dispatch: resource type %q is registered but has no configAccessors entry", key)
		}
		if acc.Field == nil {
			return fmt.Errorf("dispatch: configAccessors[%q].Field is nil", key)
		}
		if d.InValidation {
			if acc.Make == nil || acc.Get == nil || acc.Merge == nil {
				return fmt.Errorf("dispatch: resource type %q has InValidation=true but configAccessors[%q] is missing Make/Get/Merge", key, key)
			}
		}
	}
	for key := range configAccessors {
		if _, ok := descs[key]; !ok {
			return fmt.Errorf("dispatch: configAccessors has entry %q with no matching registered resource type", key)
		}
	}

	for key, d := range descs {
		if !d.InDiscovery {
			continue
		}
		acc, ok := discoveryAccessors[key]
		if !ok {
			return fmt.Errorf("dispatch: resource type %q has InDiscovery=true but has no discoveryAccessors entry", key)
		}
		if acc.Make == nil || acc.Get == nil || acc.Merge == nil {
			return fmt.Errorf("dispatch: discoveryAccessors[%q] is missing Make/Get/Merge", key)
		}
	}
	for key := range discoveryAccessors {
		d, ok := descs[key]
		if !ok || !d.InDiscovery {
			return fmt.Errorf("dispatch: discoveryAccessors has entry %q with no matching InDiscovery resource type", key)
		}
	}
	return nil
}

func init() {
	if err := checkAccessorExhaustiveness(); err != nil {
		panic(err)
	}
}
