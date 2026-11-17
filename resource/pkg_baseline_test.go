package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInventory struct {
	manager string
	pkgs    map[string][]string
	err     error
}

func (f fakeInventory) Manager() string                        { return f.manager }
func (f fakeInventory) Packages() (map[string][]string, error) { return f.pkgs, f.err }

// fakeInventorySystem is a System whose package inventory is inv, or whose
// resolution fails with resolveErr.
func fakeInventorySystem(inv fakeInventory, resolveErr error) *system.System {
	return &system.System{NewPackageInventory: func(context.Context) (system.PackageInventory, error) {
		if resolveErr != nil {
			return nil, resolveErr
		}
		return inv, nil
	}}
}

// orderComparer orders versions as integers, and records every call. With
// equalIsFree it returns 0 for equal strings without parsing them, as the
// system comparers do without running a tool.
type orderComparer struct {
	calls       []string
	equalIsFree bool
}

func (c *orderComparer) Compare(a, b string) (int, error) {
	c.calls = append(c.calls, a+" "+b)
	if c.equalIsFree && a == b {
		return 0, nil
	}
	var x, y int
	if _, err := fmt.Sscanf(a, "%d", &x); err != nil {
		return 0, err
	}
	if _, err := fmt.Sscanf(b, "%d", &y); err != nil {
		return 0, err
	}
	switch {
	case x < y:
		return -1, nil
	case x > y:
		return 1, nil
	}
	return 0, nil
}

// R-1: the comparison, on its own.
func TestDiffInventory(t *testing.T) {
	installed := map[string][]string{
		"bash":         {"5"},
		"kernel-core":  {"1", "2"},
		"kernel-tools": {"1"},
		"python3-foo":  {"3"},
		"vim":          {"9"},
		"gpg-pubkey":   {"aaaa1111-60000000", "bbbb2222-61000000", "cccc3333-62000000"},
	}
	base := func() baselinePolicy {
		return baselinePolicy{
			Recorded: []string{"bash", "kernel-core", "kernel-tools", "python3-foo", "vim", "gpg-pubkey"},
			Mode:     versionMatchNameOnly,
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*baselinePolicy)
		want inventoryDiff
	}{
		{"clean", func(*baselinePolicy) {}, inventoryDiff{}},
		{"missing", func(b *baselinePolicy) { b.Recorded = append(b.Recorded, "zsh", "absent") },
			inventoryDiff{Missing: []string{"absent", "zsh"}}},
		{"unexpected", func(b *baselinePolicy) { b.Recorded = b.Recorded[1:] },
			inventoryDiff{Unexpected: []string{"bash"}}},
		{"ignore drops a name from both sides", func(b *baselinePolicy) {
			b.Recorded = []string{"bash", "python3-foo", "vim", "gpg-pubkey", "kernel-old"}
			b.Ignore = []string{"kernel-*"}
		}, inventoryDiff{}},
		{"allow tolerates extras but a recorded allowed name stays required", func(b *baselinePolicy) {
			b.Recorded = []string{"bash", "kernel-core", "kernel-tools", "vim", "gpg-pubkey", "python3-bar"}
			b.Allow = []string{"python3-*"}
		}, inventoryDiff{Missing: []string{"python3-bar"}}},
		{"ignore beats allow", func(b *baselinePolicy) {
			b.Recorded = []string{"bash", "vim", "gpg-pubkey", "python3-foo", "kernel-gone"}
			b.Ignore = []string{"kernel-*"}
			b.Allow = []string{"kernel-*"}
		}, inventoryDiff{}},
		{"exact with multi-version names", func(b *baselinePolicy) {
			b.Mode = versionMatchExact
			b.Versions = map[string][]string{"kernel-core": {"2", "1"}, "bash": {"4"}, "vim": {"9", "9"}}
		}, inventoryDiff{Versions: []string{"bash: installed 5, recorded 4"}}},
		{"exact reports an extra version of a multi-version name", func(b *baselinePolicy) {
			b.Mode = versionMatchExact
			b.Versions = map[string][]string{"kernel-core": {"1"}}
		}, inventoryDiff{Versions: []string{"kernel-core: installed 1, 2, recorded 1"}}},
		{"newer accepts upgrades", func(b *baselinePolicy) {
			b.Mode = versionMatchNewer
			b.Versions = map[string][]string{"bash": {"4"}, "kernel-core": {"1", "2"}, "vim": {"9"}}
		}, inventoryDiff{}},
		{"newer reports a downgrade below the lowest recorded", func(b *baselinePolicy) {
			b.Mode = versionMatchNewer
			b.Versions = map[string][]string{"kernel-core": {"3", "2"}, "bash": {"6"}}
		}, inventoryDiff{Versions: []string{"bash: installed 5, older than recorded 6", "kernel-core: installed 1, older than recorded 2"}}},
		{"gpg-pubkey is exact under newer", func(b *baselinePolicy) {
			b.Mode = versionMatchNewer
			b.Versions = map[string][]string{"gpg-pubkey": {"aaaa1111-60000000", "bbbb2222-61000000"}}
		}, inventoryDiff{Versions: []string{
			"gpg-pubkey: installed aaaa1111-60000000, bbbb2222-61000000, cccc3333-62000000, recorded aaaa1111-60000000, bbbb2222-61000000",
		}}},
		{"versions of a missing name are not compared", func(b *baselinePolicy) {
			b.Recorded = append(b.Recorded, "zsh")
			b.Mode = versionMatchExact
			b.Versions = map[string][]string{"zsh": {"1"}}
		}, inventoryDiff{Missing: []string{"zsh"}}},
		{"versions of an ignored name are not compared", func(b *baselinePolicy) {
			b.Mode = versionMatchExact
			b.Ignore = []string{"bash"}
			b.Recorded = b.Recorded[1:]
			b.Versions = map[string][]string{"bash": {"1"}}
		}, inventoryDiff{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := base()
			tc.edit(&b)
			got, err := diffInventory(installed, b, &orderComparer{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("newer compares each installed version with the lowest recorded", func(t *testing.T) {
		b := base()
		b.Mode = versionMatchNewer
		b.Versions = map[string][]string{"bash": {"5"}, "vim": {"9"}}
		cmp := &orderComparer{}
		_, err := diffInventory(installed, b, cmp)
		require.NoError(t, err)
		// Equal strings reach the comparer; the system comparers return 0 for
		// them without running a tool (system TestVersionComparerWithoutTools).
		assert.Equal(t, []string{"5 5", "9 9"}, cmp.calls)
	})

	t.Run("a comparer error fails, never passes", func(t *testing.T) {
		b := base()
		b.Mode = versionMatchNewer
		b.Versions = map[string][]string{"bash": {"not-a-number"}}
		_, err := diffInventory(installed, b, &orderComparer{})
		require.Error(t, err)
	})

	t.Run("installed is not modified", func(t *testing.T) {
		b := base()
		b.Ignore = []string{"*"}
		_, err := diffInventory(installed, b, nil)
		require.NoError(t, err)
		assert.Len(t, installed, 6)
	})
}

func validBaseline() *PkgBaseline {
	b := &PkgBaseline{Installed: map[string]any{"consist-of": []any{"bash", "vim"}}}
	b.SetID("rpm")
	return b
}

func resultsByProperty(results []TestResult) map[string]TestResult {
	out := map[string]TestResult{}
	for _, r := range results {
		out[r.Property] = r
	}
	return out
}

// R-2: each rule has its own message, and a broken rule fails every property.
func TestPkgBaselineRules(t *testing.T) {
	sys := fakeInventorySystem(fakeInventory{manager: "rpm", pkgs: map[string][]string{"bash": {"5"}, "vim": {"9"}}}, nil)
	for _, tc := range []struct {
		name string
		edit func(*PkgBaseline)
		want string
	}{
		{"key", func(b *PkgBaseline) { b.SetID("yum") }, `pkg-baseline key "yum" is not a package manager; use one of apk, dpkg, pacman, rpm`},
		{"installed required", func(b *PkgBaseline) { b.Installed = nil }, "installed is required"},
		{"installed a list", func(b *PkgBaseline) { b.Installed = []any{"bash"} }, "installed must be consist-of with a list of package names"},
		{"installed another matcher", func(b *PkgBaseline) { b.Installed = map[string]any{"contain-elements": []any{"bash"}} }, "installed must be consist-of"},
		{"installed two keys", func(b *PkgBaseline) {
			b.Installed = map[string]any{"consist-of": []any{"bash"}, "contain-elements": []any{"vim"}}
		}, "installed must be consist-of"},
		{"installed not a list", func(b *PkgBaseline) { b.Installed = map[string]any{"consist-of": "bash"} }, "installed must be consist-of"},
		{"installed a matcher entry", func(b *PkgBaseline) {
			b.Installed = map[string]any{"consist-of": []any{map[string]any{"have-prefix": "b"}}}
		}, "installed: entry 1 is not a package name"},
		{"installed empty name", func(b *PkgBaseline) { b.Installed = map[string]any{"consist-of": []any{"bash", ""}} }, "installed: entry 2 is not a package name"},
		{"installed duplicate", func(b *PkgBaseline) { b.Installed = map[string]any{"consist-of": []any{"bash", "bash"}} }, `installed lists "bash" more than once`},
		{"ignore pattern", func(b *PkgBaseline) { b.Ignore = []string{"kernel-["} }, `ignore: "kernel-[" is not a valid pattern`},
		{"allow pattern", func(b *PkgBaseline) { b.Allow = []string{"["} }, `allow: "[" is not a valid pattern`},
		{"ignore empty", func(b *PkgBaseline) { b.Ignore = []string{""} }, "ignore: empty pattern"},
		{"versions unknown name", func(b *PkgBaseline) { b.Versions = map[string][]string{"zsh": {"1"}} }, `versions: "zsh" is not listed in installed`},
		{"versions empty list", func(b *PkgBaseline) { b.Versions = map[string][]string{"bash": {}} }, `versions: "bash" has no versions`},
		{"versions empty version", func(b *PkgBaseline) { b.Versions = map[string][]string{"bash": {""}} }, `versions: "bash" has an empty version`},
		{"version-match value", func(b *PkgBaseline) { b.VersionMatch = "loose" }, `version-match "loose" is not one of name-only, exact, newer`},
		{"name-only with versions", func(b *PkgBaseline) {
			b.VersionMatch = "name-only"
			b.Versions = map[string][]string{"bash": {"5"}}
		}, "version-match name-only cannot be used with versions"},
		{"exact without versions", func(b *PkgBaseline) { b.VersionMatch = "exact" }, "version-match exact needs versions"},
		{"newer without versions", func(b *PkgBaseline) { b.VersionMatch = "newer" }, "version-match newer needs versions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := validBaseline()
			tc.edit(b)
			results := b.Validate(t.Context(), sys)
			require.GreaterOrEqual(t, len(results), 2)
			for _, r := range results {
				assert.Equal(t, FAIL, r.Result, "%s", r.Property)
				require.NotNil(t, r.Err, "%s", r.Property)
				assert.Contains(t, string(*r.Err), tc.want, "%s", r.Property)
			}
		})
	}

	t.Run("a valid entry passes", func(t *testing.T) {
		for _, r := range validBaseline().Validate(t.Context(), sys) {
			assert.Equal(t, SUCCESS, r.Result, "%s: %v", r.Property, r.Err)
		}
	})
	t.Run("name-only is legal without versions", func(t *testing.T) {
		b := validBaseline()
		b.VersionMatch = "name-only"
		for _, r := range b.Validate(t.Context(), sys) {
			assert.Equal(t, SUCCESS, r.Result, "%s: %v", r.Property, r.Err)
		}
	})
}

// R-3: a baseline for another package manager fails every property, and
// does not enumerate.
func TestPkgBaselineKeyMismatch(t *testing.T) {
	sys := fakeInventorySystem(fakeInventory{manager: "rpm", err: errors.New("enumerated")}, nil)
	b := validBaseline()
	b.SetID("dpkg")
	results := b.Validate(t.Context(), sys)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, FAIL, r.Result)
		require.NotNil(t, r.Err)
		assert.Equal(t, "baseline recorded for dpkg; this host's package manager is rpm", string(*r.Err))
	}
}

// R-4: an inventory that cannot be read, or a host that cannot have one,
// fails every property. Never a skip, never a pass.
func TestPkgBaselineInventoryErrorsFail(t *testing.T) {
	for name, sys := range map[string]*system.System{
		"enumeration": fakeInventorySystem(fakeInventory{manager: "rpm", err: system.ErrPackageInventoryEmpty}, nil),
		"unsupported": fakeInventorySystem(fakeInventory{}, system.ErrPackageInventoryUnsupported),
		"undetected":  fakeInventorySystem(fakeInventory{}, system.ErrNullPackage),
		"no backend":  {},
	} {
		t.Run(name, func(t *testing.T) {
			b := validBaseline()
			b.Versions = map[string][]string{"bash": {"5"}}
			results := ValidateSafe(t.Context(), b, sys)
			require.Len(t, results, 3)
			for _, r := range results {
				assert.Equal(t, FAIL, r.Result, "%s", r.Property)
				assert.False(t, r.Skipped)
				require.NotNil(t, r.Err)
			}
		})
	}
	t.Run("unsupported names its error", func(t *testing.T) {
		results := validBaseline().Validate(t.Context(), fakeInventorySystem(fakeInventory{}, system.ErrPackageInventoryUnsupported))
		assert.Equal(t, system.ErrPackageInventoryUnsupported.Error(), string(*results[0].Err))
	})
}

// R-5: skip: true skips every property, with the author's reason, and runs
// nothing.
func TestPkgBaselineSkip(t *testing.T) {
	b := validBaseline()
	b.Versions = map[string][]string{"bash": {"5"}}
	b.Skip = true
	b.SkipReason = "rebuilt nightly"
	results := ValidateSafe(t.Context(), b, &system.System{})
	require.Len(t, results, 3)
	for _, r := range results {
		assert.Equal(t, SKIP, r.Result, "%s", r.Property)
		assert.Equal(t, "rebuilt nightly", r.SkipReason)
	}
}

// Results carry the difference list as actual, under consist-of: [].
func TestPkgBaselineResults(t *testing.T) {
	sys := fakeInventorySystem(fakeInventory{manager: "rpm", pkgs: map[string][]string{"bash": {"5"}, "zsh": {"1"}}}, nil)
	results := resultsByProperty(validBaseline().Validate(t.Context(), sys))
	require.Len(t, results, 2)
	assert.Equal(t, FAIL, results["missing"].Result)
	assert.Equal(t, []string{"vim"}, results["missing"].MatcherResult.Actual)
	assert.Equal(t, FAIL, results["unexpected"].Result)
	assert.Equal(t, []string{"zsh"}, results["unexpected"].MatcherResult.Actual)
	assert.Equal(t, "PkgBaseline", results["missing"].ResourceType)
	assert.Equal(t, "rpm", results["missing"].ResourceId)
}

// R-6: a 5000-name host against a 5000-name baseline with ten differences.
func BenchmarkDiffInventory(b *testing.B) {
	installed, policy := largeInventory()
	for b.Loop() {
		cmp := &orderComparer{equalIsFree: true}
		if _, err := diffInventory(installed, policy, cmp); err != nil {
			b.Fatal(err)
		}
	}
}

func largeInventory() (map[string][]string, baselinePolicy) {
	installed := make(map[string][]string, 5000)
	policy := baselinePolicy{Mode: versionMatchNewer, Versions: map[string][]string{},
		Ignore: []string{"kernel-*"}, Allow: []string{"python3-*"}}
	for i := range 5000 {
		name := fmt.Sprintf("pkg-%04d", i)
		installed[name] = []string{"10"}
		policy.Recorded = append(policy.Recorded, name)
		policy.Versions[name] = []string{"10"}
	}
	for i := range 5 {
		delete(installed, fmt.Sprintf("pkg-%04d", i))
		installed[fmt.Sprintf("extra-%d", i)] = []string{"1"}
	}
	return installed, policy
}

func TestDiffInventoryLargeIsFast(t *testing.T) {
	installed, policy := largeInventory()
	d, err := diffInventory(installed, policy, &orderComparer{})
	require.NoError(t, err)
	assert.Len(t, d.Missing, 5)
	assert.Len(t, d.Unexpected, 5)
	r := testing.Benchmark(BenchmarkDiffInventory)
	took := time.Duration(r.NsPerOp())
	t.Logf("diffInventory, 5000 names and 10 differences: %v per run", took)
	if raceEnabled {
		t.Skip("timing is not asserted under the race detector")
	}
	assert.Less(t, took, 50*time.Millisecond, "diffInventory took %v per run", took)
}

// The add path: fromSystem builds from the host, and readd keeps policy.
func TestPkgBaselineFromSystem(t *testing.T) {
	inv := fakeInventory{manager: "dpkg", pkgs: map[string][]string{"zsh": {"5.9"}, "bash": {"5.2"}, "linux-image": {"6.1", "6.2"}}}
	sys := fakeInventorySystem(inv, nil)

	t.Run("no key, no flags", func(t *testing.T) {
		b := &PkgBaseline{}
		_, err := b.fromSystem(sys, "", util.Config{})
		require.NoError(t, err)
		assert.Equal(t, "dpkg", b.ID())
		assert.Equal(t, map[string]any{"consist-of": []any{"bash", "linux-image", "zsh"}}, b.Installed)
		assert.Nil(t, b.Versions)
		assert.Empty(t, b.VersionMatch)
	})
	t.Run("matching key", func(t *testing.T) {
		_, err := (&PkgBaseline{}).fromSystem(sys, "dpkg", util.Config{})
		require.NoError(t, err)
	})
	t.Run("mismatched key", func(t *testing.T) {
		_, err := (&PkgBaseline{}).fromSystem(sys, "rpm", util.Config{})
		require.EqualError(t, err, "pkg-baseline rpm does not match this host's package manager, dpkg")
	})
	t.Run("versions exact, ignore", func(t *testing.T) {
		b := &PkgBaseline{}
		_, err := b.fromSystem(sys, "", util.Config{PkgBaselineVersions: "exact", PkgBaselineIgnore: []string{"linux-*"}})
		require.NoError(t, err)
		assert.Equal(t, "exact", b.VersionMatch)
		assert.Equal(t, map[string][]string{"bash": {"5.2"}, "zsh": {"5.9"}}, b.Versions)
		assert.Equal(t, map[string]any{"consist-of": []any{"bash", "zsh"}}, b.Installed)
	})

	// A-3, second half: a bad flag fails before anything is enumerated.
	enumerated := false
	counting := &system.System{NewPackageInventory: func(context.Context) (system.PackageInventory, error) {
		enumerated = true
		return inv, nil
	}}
	for name, cfg := range map[string]util.Config{
		"bad --versions": {PkgBaselineVersions: "loose"},
		"bad --ignore":   {PkgBaselineIgnore: []string{"["}},
		"bad --allow":    {PkgBaselineAllow: []string{"a["}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&PkgBaseline{}).fromSystem(counting, "", cfg)
			require.Error(t, err)
			assert.False(t, enumerated, "enumerated before rejecting the flag")
		})
	}
	t.Run("bad key", func(t *testing.T) {
		_, err := (&PkgBaseline{}).fromSystem(counting, "yum", util.Config{})
		require.ErrorContains(t, err, `"yum" is not a package manager`)
		assert.False(t, enumerated)
	})
}

// A-4: re-adding over an existing entry keeps its policy and rebuilds the
// package list, and the versions only if the kept mode checks them.
//
// REVERT-PROOF: removing the readdOver call from UpsertLive loses ignore,
// allow, version-match and the DiscoveryMeta fields, and fails this test.
func TestPkgBaselineReadd(t *testing.T) {
	inv := fakeInventory{manager: "rpm", pkgs: map[string][]string{
		"bash": {"5.3"}, "kernel-core": {"6.1"}, "python3-x": {"3"}, "zsh": {"5.9"},
	}}
	sys := fakeInventorySystem(inv, nil)
	add := func(t *testing.T, m PkgBaselineMap, cfg util.Config) *PkgBaseline {
		t.Helper()
		built := &PkgBaseline{}
		_, err := built.fromSystem(sys, "", cfg)
		require.NoError(t, err)
		require.NoError(t, UpsertLive(&m, built))
		return m["rpm"]
	}
	existing := func(mode string, versions map[string][]string) PkgBaselineMap {
		old := &PkgBaseline{
			DiscoveryMeta: DiscoveryMeta{Register: "pkgs", DependsOn: []string{"file:/etc/os-release"}, SkipReason: "later"},
			Title:         "web tier",
			Meta:          meta{"owner": "ops"},
			Installed:     map[string]any{"consist-of": []any{"bash"}},
			Ignore:        []string{"kernel-*"},
			Allow:         []string{"python3-*"},
			Versions:      versions,
			VersionMatch:  mode,
			Skip:          true,
		}
		old.SetID("rpm")
		return PkgBaselineMap{"rpm": old}
	}

	t.Run("keeps policy, regenerates exact versions", func(t *testing.T) {
		got := add(t, existing("exact", map[string][]string{"bash": {"5.2"}}), util.Config{})
		assert.Equal(t, []string{"kernel-*"}, got.Ignore)
		assert.Equal(t, []string{"python3-*"}, got.Allow)
		assert.Equal(t, "exact", got.VersionMatch)
		assert.True(t, got.Skip)
		assert.Equal(t, "later", got.SkipReason)
		assert.Equal(t, []string{"file:/etc/os-release"}, got.DependsOn)
		assert.Equal(t, "pkgs", got.Register)
		assert.Equal(t, "web tier", got.Title)
		assert.Equal(t, meta{"owner": "ops"}, got.Meta)
		assert.Equal(t, map[string]any{"consist-of": []any{"bash", "python3-x", "zsh"}}, got.Installed)
		assert.Equal(t, map[string][]string{"bash": {"5.3"}, "python3-x": {"3"}, "zsh": {"5.9"}}, got.Versions)
	})
	t.Run("an implicit exact entry stays implicit", func(t *testing.T) {
		got := add(t, existing("", map[string][]string{"bash": {"5.2"}}), util.Config{})
		assert.Empty(t, got.VersionMatch)
		assert.Equal(t, []string{"5.3"}, got.Versions["bash"])
	})
	t.Run("name-only gets no versions", func(t *testing.T) {
		got := add(t, existing("name-only", nil), util.Config{})
		assert.Equal(t, "name-only", got.VersionMatch)
		assert.Nil(t, got.Versions)
	})
	t.Run("no mode gets no versions", func(t *testing.T) {
		got := add(t, existing("", nil), util.Config{})
		assert.Nil(t, got.Versions)
	})
	t.Run("flags replace the kept values", func(t *testing.T) {
		got := add(t, existing("exact", map[string][]string{"bash": {"5.2"}}),
			util.Config{PkgBaselineVersions: "name-only", PkgBaselineIgnore: []string{"zsh"}, PkgBaselineAllow: []string{"x-*"}})
		assert.Equal(t, []string{"zsh"}, got.Ignore)
		assert.Equal(t, []string{"x-*"}, got.Allow)
		assert.Empty(t, got.VersionMatch)
		assert.Nil(t, got.Versions)
		assert.Equal(t, map[string]any{"consist-of": []any{"bash", "kernel-core", "python3-x"}}, got.Installed)
	})
	t.Run("AppendSysResource keeps policy too", func(t *testing.T) {
		m := existing("newer", map[string][]string{"bash": {"5.2"}})
		got, err := m.AppendSysResource("", sys, util.Config{})
		require.NoError(t, err)
		assert.Equal(t, []string{"kernel-*"}, got.Ignore)
		assert.Equal(t, "newer", got.VersionMatch)
		assert.NotContains(t, strings.Join(sortedKeys(got.Versions), ","), "kernel")
	})
}
