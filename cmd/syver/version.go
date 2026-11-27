package main

import (
	"fmt"
	"io"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

// The printer is installed at package init, not in main, so the tests that
// run newApp() with --version exercise the same output as the binary.
func init() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		printVersion(cmd.Root().Writer, cmd.Name, cmd.Version)
	}
}

// develVersion is what an unstamped binary reports when the Go toolchain did
// not record a module version either; it is the string Go itself uses.
const develVersion = "(devel)"

// appVersion is the version `syver --version` reports. Release builds stamp
// util.Version with -ldflags; without a stamp, a release module version from
// the build info is used, so `go install github.com/krameff/syver/cmd/syver@vX.Y.Z`
// reports X.Y.Z. Anything else, including the pseudo-version Go records for a
// build from a git checkout, reports "(devel)". It is never empty, so
// --version is registered on every build: urfave/cli hides the flag when the
// version is empty.
func appVersion() string {
	bi, ok := debug.ReadBuildInfo()
	return versionFrom(util.Version, bi, ok)
}

// versionFrom is appVersion's logic, fed its inputs for the tests. The leading
// "v" of a module version is dropped to match release stamping, which writes
// the bare version.
func versionFrom(stamped string, bi *debug.BuildInfo, ok bool) string {
	if stamped != "" {
		return stamped
	}
	if ok && bi != nil {
		if v := bi.Main.Version; isReleaseModuleVersion(v) {
			return strings.TrimPrefix(v, "v")
		}
	}
	return develVersion
}

// releaseVersionRE matches a module version a tag produces: vX.Y.Z with an
// optional pre-release, and no build metadata.
var releaseVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// pseudoVersionRE matches the three Go pseudo-version forms
// (vX.0.0-yyyymmddhhmmss-abcdefabcdef, vX.Y.Z-pre.0.yyyymmddhhmmss-abcdef...,
// vX.Y.Z-0.yyyymmddhhmmss-abcdef...), as golang.org/x/mod/module defines them.
var pseudoVersionRE = regexp.MustCompile(`^v[0-9]+\.(0\.0-|[0-9]+\.[0-9]+-([^+]*\.)?0\.)[0-9]{14}-[A-Za-z0-9]+(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// isReleaseModuleVersion reports whether a build info module version names a
// release. "(devel)", an empty version, a pseudo-version and any version with
// build metadata (such as "+dirty") do not.
func isReleaseModuleVersion(v string) bool {
	if strings.Contains(v, "+") || pseudoVersionRE.MatchString(v) {
		return false
	}
	return releaseVersionRE.MatchString(v)
}

// printVersion writes `syver --version`. Lines 1 and 2 are unchanged from
// earlier releases and must stay first: audit scripts take the version from
// the first line (`head -n 1`) or from the first x.y.z in the output. Line 3
// reports the binary's FIPS 140-3 state (util.FIPS140State) for tooling that
// needs to know whether syver runs in FIPS 140-3 mode.
func printVersion(w io.Writer, name, version string) {
	fmt.Fprintf(w, "%v version %v\nKrameff Solutions Ltd\nfips140: %s\n", name, version, util.FIPS140())
}
