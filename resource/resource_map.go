package resource

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// ResourceMap is the generic replacement for the 16 near-identical
// genny-generated per-type maps (AddrMap, FileMap, PortMap, ...) that used
// to live in resource_list.go.
//
// T is the concrete resource struct (e.g. Port). ST is the concrete
// system-side value produced while building one (e.g. system.Port) --
// carried as its own type parameter, not collapsed to `any`, specifically
// so AppendSysResourceIfExists can keep its original per-type return
// signature (some callers, e.g. add.go's process auto-add fan-out, use it).
// PT is *T, constrained to satisfy Resource, ResourceRead, SetID and
// fromSystem, which is what lets AppendSysResource/AppendSysResourceIfExists
// be written once instead of once per type.
//
// fromSystem is unexported and type-specific (one ~8-line method per type,
// in that type's own file) -- it is the one piece genny used to
// text-substitute (sys.NewPort / resource.NewPort) that Go generics cannot
// derive generically, since it has no way to pick "NewPort" off *system.System
// from the type parameter T alone.
type ResourceMap[T any, ST any, PT interface {
	*T
	Resource
	ResourceRead
	SetID(string)
	fromSystem(sys *system.System, key string, config util.Config) (ST, error)
}] map[string]*T

// AppendSysResource builds a fresh T from live system state and stores it,
// preserving Title/Meta from any existing entry under the same ID -- same
// contract as the old generated method.
func (r ResourceMap[T, ST, PT]) AppendSysResource(sr string, sys *system.System, config util.Config) (PT, error) {
	var t T
	res := PT(&t)
	if _, err := res.fromSystem(sys, sr, config); err != nil {
		return nil, err
	}
	id := res.ID()
	if old, ok := r[id]; ok {
		copyTitleMeta(res, PT(old))
	}
	r[id] = res
	return res, nil
}

// AppendSysResourceIfExists is AppendSysResource, but only stores the result
// if the underlying system resource actually exists -- used by `syver
// autoadd`. The bool return reports whether it existed (and was therefore
// stored).
func (r ResourceMap[T, ST, PT]) AppendSysResourceIfExists(sr string, sys *system.System) (PT, ST, bool, error) {
	var t T
	res := PT(&t)
	sysRes, err := res.fromSystem(sys, sr, util.Config{})
	if err != nil {
		return nil, sysRes, false, err
	}
	// FEAT-010 SW-10 Trap 2 (deliberate, documented deferral -- not missed):
	// this is the one Exists()-error-discard site left unfixed by the
	// otherwise-identical sweep applied to resource/registry.go, group.go,
	// interface.go, mount.go and user.go. Unlike those per-type sites, this
	// generic fan-out backs `syver autoadd` for all seven auto-addable
	// types on every platform -- propagating here would mean a single
	// unreadable resource aborts the entire autoadd run instead of skipping
	// just that one entry, which is a different (and probably worse)
	// failure mode than the other five sites' fix. `add` already reports
	// partial results elsewhere, so the likely-correct shape is "keep
	// going, surface a warning" -- but that needs a warning channel this
	// generic path does not have today, and deciding it needs its own
	// review, not a byproduct of this sweep. Deferred to FEAT-011; see
	// .claude/architecture/windows-unsupported-errors.md.
	exists := false
	if er, ok := any(sysRes).(system.Resource); ok {
		exists, _ = er.Exists()
	}
	if !exists {
		return res, sysRes, false, nil
	}
	id := res.ID()
	if old, ok := r[id]; ok {
		copyTitleMeta(res, PT(old))
	}
	r[id] = res
	return res, sysRes, true, nil
}

func (ret *ResourceMap[T, ST, PT]) UnmarshalJSON(data []byte) error {
	unmarshal := func(i interface{}) error {
		return json.Unmarshal(data, i)
	}
	var zero T
	whitelist, err := util.WhitelistAttrs(zero, util.JSON)
	if err != nil {
		return err
	}
	return ret.decode(unmarshal, zero, whitelist)
}

func (ret *ResourceMap[T, ST, PT]) UnmarshalYAML(unmarshal func(v interface{}) error) error {
	var zero T
	whitelist, err := util.WhitelistAttrs(zero, util.YAML)
	if err != nil {
		return err
	}
	return ret.decode(unmarshal, zero, whitelist)
}

func (ret *ResourceMap[T, ST, PT]) decode(unmarshal func(v interface{}) error, zero T, whitelist map[string]bool) error {
	if err := util.ValidateSections(unmarshal, zero, whitelist); err != nil {
		return err
	}

	var tmp map[string]*T
	if err := unmarshal(&tmp); err != nil {
		return err
	}

	typ := reflect.TypeOf(zero)
	typs := strings.Split(typ.String(), ".")[1]
	for id, res := range tmp {
		if res == nil {
			return fmt.Errorf("could not parse resource %s:%s", typs, id)
		}
		PT(res).SetID(id)
	}

	*ret = tmp
	return nil
}

// copyTitleMeta preserves the Title/Meta of an existing resource onto its
// replacement when `syver add`/`autoadd` re-adds an already-present ID --
// same behaviour as the old generated `res.Title = old_res.Title; res.Meta =
// old_res.Meta`, expressed via reflection since Go generics give no way to
// name a struct field from a type parameter alone. Every one of the 17
// resource types declares exported `Title string` and `Meta meta` fields
// (verified directly, not assumed), so this is safe across the whole set.
func copyTitleMeta(dst, src any) {
	dv := reflect.ValueOf(dst)
	sv := reflect.ValueOf(src)
	if dv.Kind() == reflect.Pointer {
		dv = dv.Elem()
	}
	if sv.Kind() == reflect.Pointer {
		sv = sv.Elem()
	}
	if f := dv.FieldByName("Title"); f.IsValid() && f.CanSet() {
		if sf := sv.FieldByName("Title"); sf.IsValid() {
			f.Set(sf)
		}
	}
	if f := dv.FieldByName("Meta"); f.IsValid() && f.CanSet() {
		if sf := sv.FieldByName("Meta"); sf.IsValid() {
			f.Set(sf)
		}
	}
}

// UpsertLive inserts res into the live map pointed to by field -- field must
// be a pointer to one of the type aliases below (e.g. *PortMap), such as
// what an accessor.Field(c) call in the root package's dispatch.go returns.
// It preserves Title/Meta from any existing entry with the same ID, exactly
// like AppendSysResource does for its own map.
//
// This exists because add.go, in the root package, cannot call
// AppendSysResource directly (it doesn't know which SyverConfig field a
// dispatched-by-name resource belongs to, and resource cannot import the
// root package to find out -- see FEAT-007 G1). It gets a freshly-built
// Resource from Descriptor.AppendSys instead, and needs a generic way to
// store it in whichever live field the accessor table points at.
func UpsertLive(field any, res Resource) error {
	rr, ok := res.(ResourceRead)
	if !ok {
		return fmt.Errorf("resource %T does not implement ResourceRead", res)
	}
	fv := reflect.ValueOf(field)
	if fv.Kind() != reflect.Pointer || fv.Elem().Kind() != reflect.Map {
		return fmt.Errorf("UpsertLive: field must be a pointer to a resource map, got %T", field)
	}
	mv := fv.Elem()
	keyV := reflect.ValueOf(rr.ID())
	resV := reflect.ValueOf(res)
	if old := mv.MapIndex(keyV); old.IsValid() {
		copyTitleMeta(res, old.Interface())
	}
	mv.SetMapIndex(keyV, resV)
	return nil
}

// --- Public API preservation: type aliases -----------------------------
//
// These are aliases (`=`), not new defined types, on purpose: an alias
// inherits every method of its target automatically, which is what lets
// AppendSysResource/AppendSysResourceIfExists/UnmarshalJSON/UnmarshalYAML be
// written once above instead of once per type. A `type FileMap
// ResourceMap[...]` (no `=`) would not inherit these methods and would need
// a forwarding wrapper for each one, defeating the point.

type AddrMap = ResourceMap[Addr, system.Addr, *Addr]
type CommandMap = ResourceMap[Command, system.Command, *Command]
type DNSMap = ResourceMap[DNS, system.DNS, *DNS]
type FileMap = ResourceMap[File, system.File, *File]
type SyverfileMap = ResourceMap[Syverfile, system.Syverfile, *Syverfile]
type GroupMap = ResourceMap[Group, system.Group, *Group]
type PackageMap = ResourceMap[Package, system.Package, *Package]
type PortMap = ResourceMap[Port, system.Port, *Port]
type ProcessMap = ResourceMap[Process, system.Process, *Process]
type ServiceMap = ResourceMap[Service, system.Service, *Service]
type UserMap = ResourceMap[User, system.User, *User]
type KernelParamMap = ResourceMap[KernelParam, system.KernelParam, *KernelParam]
type MountMap = ResourceMap[Mount, system.Mount, *Mount]
type InterfaceMap = ResourceMap[Interface, system.Interface, *Interface]
type HTTPMap = ResourceMap[HTTP, system.HTTP, *HTTP]
type RegistryMap = ResourceMap[Registry, system.Registry, *Registry]

// MatchingMap has no system backing (matching validates a literal `content:`
// value, not anything read off the host), so its ST is `any` and its
// fromSystem stub (matching.go) always errors -- matching has no `syver
// add` support (Descriptor.AppendSys is nil for it), so that stub is never
// actually reached.
type MatchingMap = ResourceMap[Matching, any, *Matching]
