package syver

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

type addInventory struct{ pkgs map[string][]string }

func (addInventory) Manager() string                          { return "rpm" }
func (a addInventory) Packages() (map[string][]string, error) { return a.pkgs, nil }

// fakeHost is a system.New stand-in whose package inventory is pkgs on rpm,
// and which counts how many times it was enumerated.
func fakeHost(pkgs map[string][]string, enumerated *int) func(string) *system.System {
	return func(string) *system.System {
		return &system.System{NewPackageInventory: func(context.Context) (system.PackageInventory, error) {
			*enumerated++
			return addInventory{pkgs: pkgs}, nil
		}}
	}
}

var hostPkgs = map[string][]string{"zsh": {"5.9-1"}, "bash": {"5.2-1"}, "gpg-pubkey": {"bbbb-2", "aaaa-1"}}

// addConfig is the CLI's default config plus the three pkg-baseline flags.
func addConfig(t *testing.T, flags util.Config) *util.Config {
	t.Helper()
	c, err := util.NewConfig()
	require.NoError(t, err)
	c.AnnounceToCLI = true
	c.PkgBaselineVersions = flags.PkgBaselineVersions
	c.PkgBaselineIgnore = flags.PkgBaselineIgnore
	c.PkgBaselineAllow = flags.PkgBaselineAllow
	return c
}

// addBaseline runs `syver add pkg-baseline` in-process against spec and
// returns what it printed.
func addBaseline(t *testing.T, spec string, args []string, flags util.Config) (string, error) {
	t.Helper()
	t.Cleanup(func() { outStoreFormat = UNSET })
	c := addConfig(t, flags)
	n := 0
	var err error
	stdout, _ := captureStdoutStderr(t, func() {
		err = addResources(spec, "PkgBaseline", args, c, fakeHost(hostPkgs, &n))
	})
	return stdout, err
}

func readSpec(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// A-1: with no arguments, one entry keyed by the host's package manager,
// names sorted, no versions, and syver-version: written first.
func TestAddPkgBaselineNoArgs(t *testing.T) {
	spec := t.TempDir() + "/syver.yaml"
	out, err := addBaseline(t, spec, nil, util.Config{})
	require.NoError(t, err)
	want := "syver-version: 0.20.0\n" +
		"pkg-baseline:\n" +
		"  rpm:\n" +
		"    installed:\n" +
		"      consist-of:\n" +
		"      - bash\n" +
		"      - gpg-pubkey\n" +
		"      - zsh\n"
	assert.Equal(t, want, readSpec(t, spec))
	assert.Contains(t, out, "Adding PkgBaseline to '"+spec+"': rpm")
	assert.Contains(t, out, "Setting syver-version: 0.20.0 in '"+spec+"'")
}

// A-2: an argument naming another package manager fails and writes nothing.
func TestAddPkgBaselineMismatchedArg(t *testing.T) {
	spec := t.TempDir() + "/syver.yaml"
	_, err := addBaseline(t, spec, []string{"dpkg"}, util.Config{})
	require.EqualError(t, err, "pkg-baseline dpkg does not match this host's package manager, rpm")
	_, statErr := os.Stat(spec)
	assert.True(t, os.IsNotExist(statErr), "a file was written")

	existing := writeSpecIn(t, t.TempDir(), "syver.yaml", okMatching)
	_, err = addBaseline(t, existing, []string{"dpkg"}, util.Config{})
	require.Error(t, err)
	assert.Equal(t, okMatching, readSpec(t, existing))
}

// A-3: --versions exact and newer write both keys; a bad --versions or glob
// fails before the host is enumerated.
func TestAddPkgBaselineVersionsFlag(t *testing.T) {
	for _, mode := range []string{"exact", "newer"} {
		t.Run(mode, func(t *testing.T) {
			spec := t.TempDir() + "/syver.yaml"
			_, err := addBaseline(t, spec, []string{"rpm"}, util.Config{PkgBaselineVersions: mode})
			require.NoError(t, err)
			got := readSpec(t, spec)
			assert.Contains(t, got, "    version-match: "+mode+"\n")
			assert.Contains(t, got, "    versions:\n      bash:\n      - 5.2-1\n")
			assert.Contains(t, got, "      gpg-pubkey:\n      - aaaa-1\n      - bbbb-2\n")
		})
	}
	for name, cfg := range map[string]util.Config{
		"versions": {PkgBaselineVersions: "all"},
		"ignore":   {PkgBaselineIgnore: []string{"[a"}},
		"allow":    {PkgBaselineAllow: []string{"[a"}},
	} {
		t.Run("bad "+name, func(t *testing.T) {
			spec := t.TempDir() + "/syver.yaml"
			n := 0
			err := addResources(spec, "PkgBaseline", nil, addConfig(t, cfg), fakeHost(hostPkgs, &n))
			t.Cleanup(func() { outStoreFormat = UNSET })
			require.Error(t, err)
			assert.Zero(t, n, "enumerated before rejecting the flag")
			_, statErr := os.Stat(spec)
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}

// A-4, end to end through the file: re-adding keeps the entry's settings.
func TestAddPkgBaselineReaddThroughFile(t *testing.T) {
	spec := writeSpecIn(t, t.TempDir(), "syver.yaml", `syver-version: 0.20.0
pkg-baseline:
  rpm:
    title: base
    meta:
      owner: ops
    register: pkgs
    depends-on: [matching:ok]
    skip: true
    skip-reason: not yet
    installed:
      consist-of: [bash]
    ignore: [gpg-*]
    allow: [z*]
    versions:
      bash: ["5.0-1"]
    version-match: newer
`+okMatching)
	_, err := addBaseline(t, spec, nil, util.Config{})
	require.NoError(t, err)
	got := readSpec(t, spec)
	for _, want := range []string{
		"    title: base\n", "      owner: ops\n", "    register: pkgs\n",
		"    depends-on:\n    - matching:ok\n", "    skip-reason: not yet\n", "    skip: true\n",
		"    ignore:\n    - gpg-*\n", "    allow:\n    - z*\n", "    version-match: newer\n",
		"    installed:\n      consist-of:\n      - bash\n      - zsh\n",
		"    versions:\n      bash:\n      - 5.2-1\n      zsh:\n      - 5.9-1\n",
	} {
		assert.Contains(t, got, want)
	}
	assert.NotContains(t, got, "gpg-pubkey")
}

// A-5: syver-version: is raised to 0.20.0 when it is lower, with one line
// saying so, and never lowered.
func TestAddPkgBaselineRaisesSyverVersion(t *testing.T) {
	t.Run("raised from 0.9.0", func(t *testing.T) {
		spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "syver-version: 0.9.0\n"+okMatching)
		out, err := addBaseline(t, spec, nil, util.Config{})
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(readSpec(t, spec), "syver-version: 0.20.0\n"), readSpec(t, spec))
		assert.Equal(t, 1, strings.Count(out, "Setting syver-version"), out)
	})
	t.Run("not lowered from 0.25.0", func(t *testing.T) {
		spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "syver-version: 0.25.0\n"+okMatching)
		out, err := addBaseline(t, spec, nil, util.Config{})
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(readSpec(t, spec), "syver-version: 0.25.0\n"), readSpec(t, spec))
		assert.NotContains(t, out, "Setting syver-version")
	})
	t.Run("other types leave it alone", func(t *testing.T) {
		spec := writeSpecIn(t, t.TempDir(), "syver.yaml", okMatching)
		require.NoError(t, addCommand(t, spec))
		assert.NotContains(t, readSpec(t, spec), "syver-version")
	})
}

// A-7: like every `syver add`, the rewrite drops what SyverConfig does not
// hold: an unknown top-level block and an x- block.
func TestAddPkgBaselineDropsUnknownBlocks(t *testing.T) {
	spec := writeSpecIn(t, t.TempDir(), "syver.yaml", "x-common: &c\n  content: 1\nnotatype:\n  a: 1\n"+okMatching)
	_, err := addBaseline(t, spec, nil, util.Config{})
	require.NoError(t, err)
	got := readSpec(t, spec)
	assert.NotContains(t, got, "x-common")
	assert.NotContains(t, got, "notatype")
	assert.Contains(t, got, "syver-version: 0.20.0\n")
	assert.Contains(t, got, "matching:\n")
}

// R-2, decode half: a wrongly typed attribute fails the whole spec when it is
// read, as any typed attribute does, before any result exists.
func TestPkgBaselineTypeErrorsFailAtDecode(t *testing.T) {
	t.Cleanup(func() { outStoreFormat = UNSET })
	outStoreFormat = YAML
	for name, body := range map[string]string{
		"scalar ignore":   "    ignore: kernel-*\n",
		"list versions":   "    versions: [a]\n",
		"scalar allow":    "    allow: python3-*\n",
		"unknown attr":    "    exclude: [a]\n",
		"versions string": "    versions:\n      bash: 5.2\n",
	} {
		t.Run(name, func(t *testing.T) {
			spec := "pkg-baseline:\n  rpm:\n    installed:\n      consist-of: [bash]\n" + body
			_, err := ReadJSONData([]byte(spec), false, "")
			require.Error(t, err)
		})
	}
	_, err := ReadJSONData([]byte("pkg-baseline:\n  rpm:\n    installed:\n      consist-of: [bash]\n    ignore: [kernel-*]\n"), false, "")
	require.NoError(t, err)
}
