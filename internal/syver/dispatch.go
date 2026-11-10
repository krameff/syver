package syver

import (
	"fmt"

	"github.com/krameff/syver/resource"
)

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

// accessor is a resource type's handle on its SyverConfig field.
//
// Get and Field are deliberately different (FEAT-007 G1): Get returns a
// *copy* of the map, which is fine for iteration (Resources(), Merge()) but
// wrong for `syver add` -- add.go must mutate syverConfig.<Field> itself, or
// newly added resources silently stop being written to the file. Field
// returns a pointer to the live struct field instead (e.g. &c.Ports), for
// resource.UpsertLive to write through.
type accessor struct {
	// Make initialises this field's map on a fresh SyverConfig.
	Make func(c *SyverConfig)
	// Get returns a *copy* of this field as a generic map, for iteration
	// only. Never use this to mutate -- see Field.
	Get func(c *SyverConfig) map[string]any
	// Field returns a pointer to the live struct field itself.
	Field func(c *SyverConfig) any
	// Merge folds src's entries for this type into dst, warning on
	// duplicates (mergeType).
	Merge func(dst, src *SyverConfig)
}

// discoveryAccessor is the same handle on a DiscoveryConfig field.
// DiscoveryConfig has no `syver add` path, so it needs no Field.
type discoveryAccessor struct {
	Make  func(c *DiscoveryConfig)
	Get   func(c *DiscoveryConfig) map[string]any
	Merge func(dst, src *DiscoveryConfig)
}

// wiredType is one resource type's pair of accessors.
type wiredType struct {
	key  string
	cfg  accessor
	disc discoveryAccessor
}

// wire builds both accessors for a resource type from the two struct fields
// that hold it, one on SyverConfig and one on DiscoveryConfig.
func wire[V any, M ~map[string]V](key string, cfg func(*SyverConfig) *M, disc func(*DiscoveryConfig) *M) wiredType {
	return wiredType{
		key: key,
		cfg: accessor{
			Make:  func(c *SyverConfig) { *cfg(c) = make(M) },
			Get:   func(c *SyverConfig) map[string]any { return anyMap(*cfg(c)) },
			Field: func(c *SyverConfig) any { return cfg(c) },
			Merge: func(dst, src *SyverConfig) {
				for k, v := range *cfg(src) {
					mergeType(*cfg(dst), key, k, v)
				}
			},
		},
		disc: discoveryAccessor{
			Make: func(c *DiscoveryConfig) { *disc(c) = make(M) },
			Get:  func(c *DiscoveryConfig) map[string]any { return anyMap(*disc(c)) },
			Merge: func(dst, src *DiscoveryConfig) {
				for k, v := range *disc(src) {
					mergeType(*disc(dst), key, k, v)
				}
			},
		},
	}
}

// anyMap copies a typed resource map into a map[string]any.
func anyMap[V any, M ~map[string]V](m M) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// wiring is the one per-type table: a new resource type is one line here.
// Listed in SyverConfig/DiscoveryConfig's struct declaration order, which
// becomes fieldOrder.
//
// gossfile is not listed: it has no DiscoveryConfig field (there's no
// "discover other gossfiles" concept), it is excluded from
// SyverConfig.Resources() (InValidation: false), and its merge is special
// (mergeSyver nils Syverfiles before the generic Merge loop runs). It gets
// a Field-only configAccessors entry below, because `syver add syver` still
// needs to reach the live Syverfiles field.
var wiring = []wiredType{
	wire("file", func(c *SyverConfig) *resource.FileMap { return &c.Files }, func(c *DiscoveryConfig) *resource.FileMap { return &c.Files }),
	wire("package", func(c *SyverConfig) *resource.PackageMap { return &c.Packages }, func(c *DiscoveryConfig) *resource.PackageMap { return &c.Packages }),
	wire("addr", func(c *SyverConfig) *resource.AddrMap { return &c.Addrs }, func(c *DiscoveryConfig) *resource.AddrMap { return &c.Addrs }),
	wire("port", func(c *SyverConfig) *resource.PortMap { return &c.Ports }, func(c *DiscoveryConfig) *resource.PortMap { return &c.Ports }),
	wire("service", func(c *SyverConfig) *resource.ServiceMap { return &c.Services }, func(c *DiscoveryConfig) *resource.ServiceMap { return &c.Services }),
	wire("user", func(c *SyverConfig) *resource.UserMap { return &c.Users }, func(c *DiscoveryConfig) *resource.UserMap { return &c.Users }),
	wire("group", func(c *SyverConfig) *resource.GroupMap { return &c.Groups }, func(c *DiscoveryConfig) *resource.GroupMap { return &c.Groups }),
	wire("command", func(c *SyverConfig) *resource.CommandMap { return &c.Commands }, func(c *DiscoveryConfig) *resource.CommandMap { return &c.Commands }),
	wire("dns", func(c *SyverConfig) *resource.DNSMap { return &c.DNS }, func(c *DiscoveryConfig) *resource.DNSMap { return &c.DNS }),
	wire("process", func(c *SyverConfig) *resource.ProcessMap { return &c.Processes }, func(c *DiscoveryConfig) *resource.ProcessMap { return &c.Processes }),
	wire("kernel-param", func(c *SyverConfig) *resource.KernelParamMap { return &c.KernelParams }, func(c *DiscoveryConfig) *resource.KernelParamMap { return &c.KernelParams }),
	wire("mount", func(c *SyverConfig) *resource.MountMap { return &c.Mounts }, func(c *DiscoveryConfig) *resource.MountMap { return &c.Mounts }),
	wire("interface", func(c *SyverConfig) *resource.InterfaceMap { return &c.Interfaces }, func(c *DiscoveryConfig) *resource.InterfaceMap { return &c.Interfaces }),
	wire("http", func(c *SyverConfig) *resource.HTTPMap { return &c.HTTPs }, func(c *DiscoveryConfig) *resource.HTTPMap { return &c.HTTPs }),
	wire("matching", func(c *SyverConfig) *resource.MatchingMap { return &c.Matchings }, func(c *DiscoveryConfig) *resource.MatchingMap { return &c.Matchings }),
	wire("registry", func(c *SyverConfig) *resource.RegistryMap { return &c.Registries }, func(c *DiscoveryConfig) *resource.RegistryMap { return &c.Registries }),
}

// configAccessors and discoveryAccessors are wiring indexed by key, and
// fieldOrder is its key order. fieldOrder drives NewSyverConfig()'s Make()
// loop and the two Merge() methods; nothing observably depends on it beyond
// the sequence of "[WARN] Duplicate key" log lines.
var configAccessors, discoveryAccessors, fieldOrder = indexWiring(wiring)

func indexWiring(ws []wiredType) (map[string]accessor, map[string]discoveryAccessor, []string) {
	cfg := map[string]accessor{
		"gossfile": {Field: func(c *SyverConfig) any { return &c.Syverfiles }},
	}
	disc := make(map[string]discoveryAccessor, len(ws))
	order := make([]string, 0, len(ws))
	for _, w := range ws {
		cfg[w.key] = w.cfg
		disc[w.key] = w.disc
		order = append(order, w.key)
	}
	return cfg, disc, order
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
