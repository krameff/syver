package main

import (
	"bytes"
	"context"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrintVersionLines: lines 1 and 2 are byte-for-byte what earlier
// releases printed, and line 3 is the FIPS state. Audit scripts parse line 1
// (`head -n 1`) or the first x.y.z anywhere in the output, so line 1 must stay
// first and nothing before it may carry a version-like string.
// Revert-proof: print the fips140 line first in printVersion; the line-1 and
// first-match assertions fail.
func TestPrintVersionLines(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf, "syver", "0.22.0")
	out := buf.String()

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	require.Len(t, lines, 3, "output: %q", out)
	assert.Equal(t, "syver version 0.22.0", lines[0])
	assert.Equal(t, "Krameff Solutions Ltd", lines[1])
	assert.Equal(t, "fips140: "+util.FIPS140().String(), lines[2])
	assert.True(t, strings.HasPrefix(lines[2], "fips140: mode="), "line 3: %q", lines[2])

	// The first-match parser the Windows audit uses, and `head -n 1`.
	m := regexp.MustCompile(`v?(\d+\.\d+\.\d+)`).FindStringSubmatch(out)
	require.NotNil(t, m)
	assert.Equal(t, "0.22.0", m[1])
	assert.Equal(t, lines[0], strings.SplitN(out, "\n", 2)[0])
}

// TestPrintVersionFirstMatchWithAModule: when a frozen module is linked, line 3
// carries its vX.Y.Z; the first match must still be syver's version. Drives the
// formatting with a synthetic state rather than a GOFIPS140 build.
// Revert-proof: as TestPrintVersionLines (fips140 line first): the first match
// becomes the module version.
func TestPrintVersionFirstMatchWithAModule(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf, "syver", "0.22.0")
	out := strings.Replace(buf.String(), util.FIPS140().String(), "mode=on module=v1.26.0 module-id=v1.26.0", 1)
	m := regexp.MustCompile(`v?(\d+\.\d+\.\d+)`).FindStringSubmatch(out)
	require.NotNil(t, m)
	assert.Equal(t, "0.22.0", m[1])
}

// TestVersionFrom: the ldflags stamp wins; otherwise a release module
// version from the build info, without its leading "v"; otherwise "(devel)",
// which covers every Go pseudo-version and a "+dirty" build from a checkout.
// Revert-proofs: return util.Version unconditionally (the unstamped rows get
// ""); trim nothing instead of "v" (the go install row gets "v0.22.0"); make
// the pseudo-version branch report the raw string (the three pseudo-version
// rows fail).
func TestVersionFrom(t *testing.T) {
	bi := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/krameff/syver", Version: v}}
	}
	for _, tc := range []struct {
		name    string
		stamped string
		bi      *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags stamp", "0.22.0", bi("(devel)"), true, "0.22.0"},
		{"ldflags stamp wins over build info", "0.22.0", bi("v0.21.0"), true, "0.22.0"},
		{"go install @vX.Y.Z", "", bi("v0.22.0"), true, "0.22.0"},
		{"go install @vX.Y.Z-rc.1", "", bi("v0.22.0-rc.1"), true, "0.22.0-rc.1"},
		{"pseudo-version, no tag", "", bi("v0.0.0-20261123150944-7aec6cf9742f"), true, "(devel)"},
		{"pseudo-version after a pre-release tag", "", bi("v0.22.0-rc.1.0.20261123150944-7aec6cf9742f"), true, "(devel)"},
		{"pseudo-version after a release tag", "", bi("v0.21.1-0.20261123150944-7aec6cf9742f"), true, "(devel)"},
		{"pseudo-version, dirty tree", "", bi("v0.21.1-0.20261123150944-7aec6cf9742f+dirty"), true, "(devel)"},
		{"tag at HEAD, dirty tree", "", bi("v0.22.0+dirty"), true, "(devel)"},
		{"no VCS information", "", bi("(devel)"), true, "(devel)"},
		{"empty main version", "", bi(""), true, "(devel)"},
		{"no build info", "", nil, false, "(devel)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, versionFrom(tc.stamped, tc.bi, tc.ok))
		})
	}
}

// TestVersionFlagOnUnstampedBuild: with util.Version empty, as in a plain
// `go build` or `go install`, --version is still registered and prints three
// lines. The test binary itself is unstamped, so its build info gives
// "(devel)" or a module version. Not parallel: it swaps util.Version.
// Revert-proof: set newApp's Version back to util.Version; urfave/cli then
// hides the flag and Run fails with "flag provided but not defined: -version".
func TestVersionFlagOnUnstampedBuild(t *testing.T) {
	orig := util.Version
	util.Version = ""
	t.Cleanup(func() { util.Version = orig })

	app := newApp()
	var buf bytes.Buffer
	app.Writer = &buf
	app.ErrWriter = &buf
	require.NoError(t, app.Run(context.Background(), []string{"syver", "--version"}), "output: %q", buf.String())

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, 3, "output: %q", buf.String())
	assert.True(t, strings.HasPrefix(lines[0], "syver version "), "line 1: %q", lines[0])
	assert.NotEqual(t, "syver version ", lines[0], "empty version on line 1")
	assert.True(t, strings.HasPrefix(lines[2], "fips140: mode="), "line 3: %q", lines[2])
}
