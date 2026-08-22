package resource

import (
	"sort"
	"sync"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

// Source distinguishes where a resource type's implementation lives.
// SourceExternal is reserved for Phase 2 (out-of-process plugin modules);
// nothing in Phase 1 sets it.
type Source int

const (
	SourceBuiltin Source = iota
	SourceExternal
)

// AppendFn constructs a fresh resource from live system state for `syver
// add`/`autoadd`. It closes over both the system-side constructor
// (sys.NewPort, ...) and the resource-side constructor (resource.NewPort,
// ...), which is what lets add.go dispatch without knowing which SyverConfig
// field a type lives in -- that part is handled separately, by the caller,
// via the live-field accessor (see dispatch.go, FEAT-007 G1). A nil AppendFn
// means the type has no `syver add` support (only `matching` today).
type AppendFn func(sys *system.System, key string, config util.Config) (Resource, error)

// AutoAddSpec marks a type as eligible for `syver autoadd` and fixes its
// position in the (order-sensitive -- it drives resourcePrint's stdout
// sequence, which is golden-tested) autoadd scan. Order values are sparse on
// purpose: File=1, Group=2, Package=3, Port=4, Process=5, Service=6, User=7,
// matching every other type out of the loop entirely (AutoAdd == nil).
type AutoAddSpec struct {
	Order int
}

// Descriptor is the single per-type registration record that replaces the
// old registerResource(key, &Type{}) call plus the ~19 scattered touch
// points a new resource type used to require (see FEAT-007 spec S1).
//
// Do not confuse this file with resource/registry.go -- that is the Windows
// Registry *resource*, an unrelated pre-existing file.
type Descriptor struct {
	Key  string          // gossfile key: "port", "kernel-param", "gossfile"
	Name string          // dispatch name: "Port", "Gossfile" -- NOT always the Go type name
	New  func() Resource // zero-value factory

	// Set membership. Today these two diverge implicitly (see the FEAT-007
	// spec's "canonical inventory" table); made explicit here.
	InValidation bool // in SyverConfig.Resources(): matching=true, gossfile=false
	InDiscovery  bool // has a DiscoveryConfig field: gossfile=false, others=true

	Alias     []string          // e.g. "syverfile" -> gossfile
	AppendSys AppendFn          // nil = no `syver add` (matching)
	AutoAdd   *AutoAddSpec      // nil = not auto-addable
	CLIFlags  func() []cli.Flag // per-type `add` flags (http, dns, addr, ...); nil = none

	// CLIName/CLIAliases drive the generated `cmd/syver add` subcommand.
	// Default to Key/nil when unset. Needed because at least one type
	// (gossfile) is named neither by its Key nor its Name on the CLI --
	// see FEAT-007 G4.
	CLIName    string
	CLIAliases []string

	Source Source
}

var (
	descriptorsMu sync.Mutex
	descriptors   = map[string]Descriptor{}
)

// Register adds a Descriptor to the registry. Mirrors the one working
// registry already in this tree, outputs.RegisterOutputer
// (outputs/outputs.go): mutex-guarded, panics on a nil New or on a
// duplicate key (including alias keys).
func Register(d Descriptor) {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()

	if d.New == nil {
		panic("resource: Register called with a nil New for " + d.Key)
	}
	register := func(key string) {
		if _, dup := descriptors[key]; dup {
			panic("resource: Register called twice for " + key)
		}
		descriptors[key] = d
	}
	register(d.Key)
	for _, alias := range d.Alias {
		register(alias)
	}
}

// Descriptors returns every registered descriptor, keyed by its primary Key
// (aliases are omitted from this view -- use DescriptorByName/lookup by Key
// on the map returned by descriptorsSnapshot for alias-inclusive access).
func Descriptors() map[string]Descriptor {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()

	out := make(map[string]Descriptor, len(descriptors))
	for key, d := range descriptors {
		if key != d.Key {
			continue // skip alias entries; Descriptors() is keyed by primary Key
		}
		out[key] = d
	}
	return out
}

// DescriptorByKey looks up a descriptor by its gossfile key, including alias
// keys (e.g. "syverfile" resolves to the same Descriptor as "gossfile").
func DescriptorByKey(key string) (Descriptor, bool) {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()
	d, ok := descriptors[key]
	return d, ok
}

// DescriptorByName looks up a descriptor by its dispatch Name (e.g. "Port",
// "Gossfile") -- the value add.go's switch used to key on.
func DescriptorByName(name string) (Descriptor, bool) {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()
	for _, d := range descriptors {
		if d.Name == name {
			return d, true
		}
	}
	return Descriptor{}, false
}

// AutoAddDescriptors returns every AutoAdd-eligible descriptor, sorted by
// AutoAddSpec.Order. The order is load-bearing: `syver autoadd` prints one
// line per discovered resource, in this sequence, and that sequence is
// golden-tested.
func AutoAddDescriptors() []Descriptor {
	descriptorsMu.Lock()
	defer descriptorsMu.Unlock()

	var out []Descriptor
	for key, d := range descriptors {
		if key != d.Key || d.AutoAdd == nil {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AutoAdd.Order < out[j].AutoAdd.Order })
	return out
}

// --- Deprecated shim ---------------------------------------------------
//
// Resources() is kept, exported, for any library embedder (util/config.go
// documents embedding as supported) that might call it directly. There is
// no unexported registerResource() shim alongside it: every internal
// resource/<type>.go call site was converted to Register(Descriptor{...})
// in the same change that added this file (FEAT-007 task 3), and
// registerResource was never exported, so no embedder could have called it
// either -- keeping it around would just be dead code.

// Resources returns the deprecated key->Resource map. Deprecated: use
// Descriptors() and Descriptor.New() instead.
func Resources() map[string]Resource {
	out := map[string]Resource{}
	for key, d := range Descriptors() {
		out[key] = d.New()
	}
	return out
}
