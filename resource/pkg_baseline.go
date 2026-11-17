package resource

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

// PkgBaseline asserts that a Linux host has exactly a recorded set of
// installed packages. Each entry is keyed by package manager, which must be
// the host's own.
type PkgBaseline struct {
	DiscoveryMeta `yaml:",inline" json:",inline"`
	Title         string              `json:"title,omitempty" yaml:"title,omitempty"`
	Meta          meta                `json:"meta,omitempty" yaml:"meta,omitempty"`
	id            string              `json:"-" yaml:"-"`
	Installed     matcher             `json:"installed" yaml:"installed"`
	Ignore        []string            `json:"ignore,omitempty" yaml:"ignore,omitempty"`
	Allow         []string            `json:"allow,omitempty" yaml:"allow,omitempty"`
	Versions      map[string][]string `json:"versions,omitempty" yaml:"versions,omitempty"`
	VersionMatch  string              `json:"version-match,omitempty" yaml:"version-match,omitempty"`
	Skip          bool                `json:"skip,omitempty" yaml:"skip,omitempty"`

	// added is set only on an entry built by `syver add`, for readd.
	added *pkgBaselineAdd
}

// pkgBaselineAdd is what `syver add pkg-baseline` learned and was asked for,
// kept so that re-adding over an existing entry can keep that entry's policy
// and still rebuild the package list from the host.
type pkgBaselineAdd struct {
	inventory map[string][]string
	// Which policy flags were given. A flag that was not given leaves the
	// existing entry's value in place.
	ignoreSet, allowSet, modeSet bool
}

const (
	PkgBaselineResourceKey  = "pkg-baseline"
	PkgBaselineResourceName = "PkgBaseline"

	versionMatchNameOnly = "name-only"
	versionMatchExact    = "exact"
	versionMatchNewer    = "newer"

	// gpgPubkey is rpm's entry for imported signing keys. Its versions are key
	// ids, so they are compared as a set whatever version-match says.
	gpgPubkey = "gpg-pubkey"
)

func init() {
	Register(Descriptor{
		Key:          PkgBaselineResourceKey,
		Name:         PkgBaselineResourceName,
		New:          func() Resource { return &PkgBaseline{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &PkgBaseline{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{
				&cli.StringFlag{Name: "versions", Usage: "record and check versions: name-only, newer or exact"},
				&cli.StringSliceFlag{Name: "ignore", Usage: "glob of package names to leave out entirely"},
				&cli.StringSliceFlag{Name: "allow", Usage: "glob of package names that may be installed without being recorded"},
			}
		},
		AddWithoutArgs: true,
		Since:          "0.20.0",
	})
}

func (b *PkgBaseline) ID() string       { return b.id }
func (b *PkgBaseline) SetID(id string)  { b.id = id }
func (b *PkgBaseline) SetSkip()         { b.Skip = true }
func (b *PkgBaseline) TypeKey() string  { return PkgBaselineResourceKey }
func (b *PkgBaseline) TypeName() string { return PkgBaselineResourceName }
func (b *PkgBaseline) GetTitle() string { return b.Title }
func (b *PkgBaseline) GetMeta() meta    { return b.Meta }

// mode is the version-match in effect: as written, or when absent, exact if
// versions are recorded and name-only if not.
func (b *PkgBaseline) mode() string {
	if b.VersionMatch != "" {
		return b.VersionMatch
	}
	if len(b.Versions) > 0 {
		return versionMatchExact
	}
	return versionMatchNameOnly
}

func (b *PkgBaseline) properties() []string {
	props := []string{"missing", "unexpected"}
	if checksVersions(b.mode()) {
		props = append(props, "versions")
	}
	return props
}

// consistOfNone is the matcher every result shows: the difference list it
// reports should be empty.
func consistOfNone() map[string]any { return map[string]any{"consist-of": []any{}} }

func (b *PkgBaseline) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = withID(ctx, b.ID())
	props := b.properties()
	if b.Skip {
		results := make([]TestResult, 0, len(props))
		for _, p := range props {
			results = append(results, ValidateValue(b, p, consistOfNone(), nil, true))
		}
		return results
	}

	diff, err := b.check(ctx, sys)
	actual := map[string][]string{
		"missing":    diff.Missing,
		"unexpected": diff.Unexpected,
		"versions":   diff.Versions,
	}
	results := make([]TestResult, 0, len(props))
	for _, p := range props {
		list := actual[p]
		if list == nil {
			list = []string{}
		}
		results = append(results, ValidateValue(b, p, consistOfNone(), func() ([]string, error) {
			if err != nil {
				return nil, err
			}
			return list, nil
		}, false))
	}
	return results
}

// check runs the rules, resolves the host's package manager, enumerates and
// compares. Any error fails every property.
func (b *PkgBaseline) check(ctx context.Context, sys *system.System) (inventoryDiff, error) {
	policy, err := b.policy()
	if err != nil {
		return inventoryDiff{}, err
	}
	inv, err := sys.PackageInventory(ctx)
	if err != nil {
		return inventoryDiff{}, err
	}
	if inv.Manager() != b.ID() {
		return inventoryDiff{}, fmt.Errorf("baseline recorded for %s; this host's package manager is %s", b.ID(), inv.Manager())
	}
	installed, err := inv.Packages()
	if err != nil {
		return inventoryDiff{}, err
	}
	var cmp versionComparer
	if policy.Mode != versionMatchNameOnly {
		if cmp, err = system.NewPackageVersionComparer(ctx, inv.Manager()); err != nil {
			return inventoryDiff{}, err
		}
	}
	return diffInventory(installed, policy, cmp)
}

// baselinePolicy is a validated baseline entry, in the shape diffInventory
// compares.
type baselinePolicy struct {
	Recorded []string
	Ignore   []string
	Allow    []string
	Versions map[string][]string
	Mode     string
}

// versionComparer orders two versions. system.PackageVersionComparer is one.
type versionComparer interface {
	Compare(a, b string) (int, error)
}

// policy applies the entry's rules and returns it in the shape diffInventory
// takes. Every rule that is broken is reported, each with its own message.
func (b *PkgBaseline) policy() (baselinePolicy, error) {
	var errs []error
	if !system.IsSupportedPackageManager(b.ID()) {
		errs = append(errs, fmt.Errorf("pkg-baseline key %q is not a package manager; use one of %s",
			b.ID(), strings.Join(system.SupportedPackageManagers(), ", ")))
	}
	recorded, namesErr := installedNames(b.Installed)
	if namesErr != nil {
		errs = append(errs, namesErr)
	}
	for _, f := range []struct {
		name  string
		globs []string
	}{{"ignore", b.Ignore}, {"allow", b.Allow}} {
		if err := checkGlobs(f.name, f.globs); err != nil {
			errs = append(errs, err)
		}
	}
	inRecorded := make(map[string]bool, len(recorded))
	for _, n := range recorded {
		inRecorded[n] = true
	}
	for _, name := range sortedKeys(b.Versions) {
		vs := b.Versions[name]
		if namesErr == nil && !inRecorded[name] {
			errs = append(errs, fmt.Errorf("versions: %q is not listed in installed", name))
		}
		if len(vs) == 0 {
			errs = append(errs, fmt.Errorf("versions: %q has no versions", name))
		}
		if slices.Contains(vs, "") {
			errs = append(errs, fmt.Errorf("versions: %q has an empty version", name))
		}
	}
	switch b.VersionMatch {
	case "", versionMatchNameOnly, versionMatchExact, versionMatchNewer:
	default:
		errs = append(errs, fmt.Errorf("version-match %q is not one of name-only, exact, newer", b.VersionMatch))
	}
	switch {
	case b.VersionMatch == versionMatchNameOnly && len(b.Versions) > 0:
		errs = append(errs, errors.New("version-match name-only cannot be used with versions; remove one"))
	case (b.VersionMatch == versionMatchExact || b.VersionMatch == versionMatchNewer) && len(b.Versions) == 0:
		errs = append(errs, fmt.Errorf("version-match %s needs versions; record them with syver add pkg-baseline --versions %s", b.VersionMatch, b.VersionMatch))
	}
	if len(errs) > 0 {
		return baselinePolicy{}, errors.Join(errs...)
	}
	return baselinePolicy{Recorded: recorded, Ignore: b.Ignore, Allow: b.Allow, Versions: b.Versions, Mode: b.mode()}, nil
}

var errInstalledShape = errors.New("installed must be consist-of with a list of package names")

// installedNames reads installed:, which takes exactly one shape:
// consist-of with a list of distinct, non-empty names.
func installedNames(m matcher) ([]string, error) {
	if m == nil {
		return nil, errors.New("installed is required")
	}
	var list any
	switch x := m.(type) {
	case map[string]any:
		if len(x) != 1 {
			return nil, errInstalledShape
		}
		v, ok := x["consist-of"]
		if !ok {
			return nil, errInstalledShape
		}
		list = v
	case map[any]any:
		if len(x) != 1 {
			return nil, errInstalledShape
		}
		v, ok := x["consist-of"]
		if !ok {
			return nil, errInstalledShape
		}
		list = v
	default:
		return nil, errInstalledShape
	}
	var items []any
	switch l := list.(type) {
	case []any:
		items = l
	case []string:
		for _, s := range l {
			items = append(items, s)
		}
	default:
		return nil, errInstalledShape
	}
	names := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, it := range items {
		s, ok := it.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("installed: entry %d is not a package name: %v", i+1, it)
		}
		if seen[s] {
			return nil, fmt.Errorf("installed lists %q more than once", s)
		}
		seen[s] = true
		names = append(names, s)
	}
	return names, nil
}

func checkGlobs(field string, globs []string) error {
	for _, g := range globs {
		if g == "" {
			return fmt.Errorf("%s: empty pattern", field)
		}
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("%s: %q is not a valid pattern: %w", field, g, err)
		}
	}
	return nil
}

func matchesAny(globs []string, name string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// inventoryDiff is what a baseline found: recorded names not installed,
// installed names not recorded or allowed, and one line per recorded name
// whose installed versions do not satisfy the version rule. Each is sorted.
type inventoryDiff struct {
	Missing, Unexpected, Versions []string
}

// diffInventory compares an installed inventory against a baseline policy. It
// does no I/O apart from cmp, which is called only for versions that differ,
// and it does not modify installed.
//
//  1. Names matching ignore are dropped from both sides and from versions.
//  2. Missing is recorded minus installed.
//  3. Unexpected is installed minus recorded minus names matching allow.
//  4. With versions on, each recorded, installed name that has versions is
//     compared: exact as sets, newer as "no installed version older than the
//     lowest recorded one". gpg-pubkey is always exact.
func diffInventory(installed map[string][]string, b baselinePolicy, cmp versionComparer) (inventoryDiff, error) {
	var d inventoryDiff
	recorded := make(map[string]bool, len(b.Recorded))
	for _, n := range b.Recorded {
		if !matchesAny(b.Ignore, n) {
			recorded[n] = true
		}
	}
	for n := range recorded {
		if _, ok := installed[n]; !ok {
			d.Missing = append(d.Missing, n)
		}
	}
	for n := range installed {
		if recorded[n] || matchesAny(b.Ignore, n) || matchesAny(b.Allow, n) {
			continue
		}
		d.Unexpected = append(d.Unexpected, n)
	}
	sort.Strings(d.Missing)
	sort.Strings(d.Unexpected)

	if !checksVersions(b.Mode) {
		return d, nil
	}
	if cmp == nil {
		return inventoryDiff{}, errors.New("no version comparison available")
	}
	for _, name := range sortedKeys(b.Versions) {
		have, ok := installed[name]
		if !recorded[name] || !ok {
			continue
		}
		want := dedupSorted(b.Versions[name])
		mode := b.Mode
		if name == gpgPubkey {
			mode = versionMatchExact
		}
		if mode == versionMatchExact {
			if got := dedupSorted(have); !slices.Equal(got, want) {
				d.Versions = append(d.Versions, fmt.Sprintf("%s: installed %s, recorded %s",
					name, strings.Join(got, ", "), strings.Join(want, ", ")))
			}
			continue
		}
		lowest := want[0]
		for _, v := range want[1:] {
			c, err := cmp.Compare(v, lowest)
			if err != nil {
				return inventoryDiff{}, fmt.Errorf("comparing %s versions: %w", name, err)
			}
			if c < 0 {
				lowest = v
			}
		}
		var older []string
		for _, v := range dedupSorted(have) {
			c, err := cmp.Compare(v, lowest)
			if err != nil {
				return inventoryDiff{}, fmt.Errorf("comparing %s versions: %w", name, err)
			}
			if c < 0 {
				older = append(older, v)
			}
		}
		if len(older) > 0 {
			d.Versions = append(d.Versions, fmt.Sprintf("%s: installed %s, older than recorded %s",
				name, strings.Join(older, ", "), lowest))
		}
	}
	return d, nil
}

func dedupSorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// fromSystem builds a baseline of every package installed on the host. An
// empty key becomes the host's package manager; any other key must be it.
// The flags are checked before anything is enumerated.
func (b *PkgBaseline) fromSystem(sys *system.System, key string, config util.Config) (system.PackageInventory, error) {
	switch config.PkgBaselineVersions {
	case "", versionMatchNameOnly, versionMatchExact, versionMatchNewer:
	default:
		return nil, fmt.Errorf("--versions %q is not one of name-only, newer, exact", config.PkgBaselineVersions)
	}
	if err := checkGlobs("--ignore", config.PkgBaselineIgnore); err != nil {
		return nil, err
	}
	if err := checkGlobs("--allow", config.PkgBaselineAllow); err != nil {
		return nil, err
	}
	if key != "" && !system.IsSupportedPackageManager(key) {
		return nil, fmt.Errorf("pkg-baseline %q is not a package manager; use one of %s",
			key, strings.Join(system.SupportedPackageManagers(), ", "))
	}

	ctx := context.WithValue(context.Background(), idKey{}, key)
	inv, err := sys.PackageInventory(ctx)
	if err != nil {
		return nil, err
	}
	if key != "" && key != inv.Manager() {
		return inv, fmt.Errorf("pkg-baseline %s does not match this host's package manager, %s", key, inv.Manager())
	}
	installed, err := inv.Packages()
	if err != nil {
		return inv, err
	}

	// An explicit name-only is written as no version-match, which means the
	// same thing.
	mode := config.PkgBaselineVersions
	if mode == versionMatchNameOnly {
		mode = ""
	}
	*b = PkgBaseline{
		id:           inv.Manager(),
		Ignore:       config.PkgBaselineIgnore,
		Allow:        config.PkgBaselineAllow,
		VersionMatch: mode,
		added: &pkgBaselineAdd{
			inventory: installed,
			ignoreSet: len(config.PkgBaselineIgnore) > 0,
			allowSet:  len(config.PkgBaselineAllow) > 0,
			modeSet:   config.PkgBaselineVersions != "",
		},
	}
	b.record(checksVersions(mode))
	return inv, nil
}

// record fills installed and versions from the enumerated inventory, leaving
// out ignored names, with versions only when withVersions is set.
func (b *PkgBaseline) record(withVersions bool) {
	var names []any
	b.Versions = nil
	for _, name := range sortedKeys(b.added.inventory) {
		if matchesAny(b.Ignore, name) {
			continue
		}
		names = append(names, name)
		if withVersions {
			if b.Versions == nil {
				b.Versions = map[string][]string{}
			}
			b.Versions[name] = dedupSorted(b.added.inventory[name])
		}
	}
	if names == nil {
		names = []any{}
	}
	b.Installed = map[string]any{"consist-of": names}
}

// checksVersions reports whether a version-match value records versions.
func checksVersions(mode string) bool {
	return mode == versionMatchExact || mode == versionMatchNewer
}

// readd keeps an existing entry's policy when `syver add pkg-baseline` runs
// over it: ignore, allow and version-match unless the matching flag was
// given, and skip, skip-reason, depends-on and register always. The package
// list is rebuilt from the host, and so are the versions if the kept mode
// checks them. Title and meta are kept by the caller, as for every type.
func (b *PkgBaseline) readd(old any) {
	prev, ok := old.(*PkgBaseline)
	if !ok || b.added == nil {
		return
	}
	if !b.added.ignoreSet {
		b.Ignore = prev.Ignore
	}
	if !b.added.allowSet {
		b.Allow = prev.Allow
	}
	withVersions := checksVersions(b.VersionMatch)
	if !b.added.modeSet {
		// prev.mode(), not prev.VersionMatch: an entry with versions and no
		// version-match is exact, and stays written that way.
		b.VersionMatch = prev.VersionMatch
		withVersions = checksVersions(prev.mode())
	}
	b.Skip = prev.Skip
	b.DiscoveryMeta = prev.DiscoveryMeta
	b.record(withVersions)
}
