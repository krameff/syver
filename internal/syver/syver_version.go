package syver

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/blang/semver/v4"
)

// syverVersionKey is the top-level key that names the oldest Syver release a
// spec was written for. Declaring it also makes unknown top-level keys in that
// file, and in every gossfile: it includes, an error rather than a warning.
const syverVersionKey = "syver-version"

// requiredVersionShape is the only shape parseRequiredVersion accepts: one to
// three dot-separated numbers with no leading zeros, an optional leading v and
// an optional pre-release suffix. semver.ParseTolerant alone also accepts
// "00.20" and "0.020.0", which nobody means as a version.
var requiredVersionShape = regexp.MustCompile(`^v?(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*)){0,2}(-[0-9A-Za-z.-]+)?$`)

// releaseStampShape matches a plain release stamp and nothing else. Every
// other stamp is a development build: the ci/version.sh describe form
// (v0.16.0-6-gabc1234), a goreleaser snapshot (0.16.1-next), an -rcN, a bare
// commit sha, or an empty stamp from a plain go build.
var releaseStampShape = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// parseRequiredVersion parses the value of syver-version:. It accepts a single
// bare version (0.20.0, v0.20.0, 0.20, or 1 meaning 1.0.0) and rejects an
// empty value, an operator or range, and anything else that is not a version.
// A pre-release suffix is accepted and dropped, so 0.20.0-rc1 requires 0.20.0.
// The returned error's text carries no location; the caller adds it.
func parseRequiredVersion(raw string) (semver.Version, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return semver.Version{}, fmt.Errorf("%s needs a version, e.g. %s: 0.20.0", syverVersionKey, syverVersionKey)
	}
	if strings.ContainsAny(s, "<>=~^!*|, ") {
		return semver.Version{}, fmt.Errorf("%s takes a single minimum version such as 0.20.0, not %q", syverVersionKey, s)
	}
	if !requiredVersionShape.MatchString(s) {
		return semver.Version{}, fmt.Errorf("%s %q is not a version", syverVersionKey, s)
	}
	v, err := semver.ParseTolerant(s)
	if err != nil {
		return semver.Version{}, fmt.Errorf("%s %q is not a version", syverVersionKey, s)
	}
	v.Pre = nil
	v.Build = nil
	return v, nil
}

// classifyRunningVersion reports the running binary's version and whether it
// is a plain release. Only a stamp of the form X.Y.Z or vX.Y.Z counts, and
// 0.0.0 (ci/release-build.sh's default) does not. Everything else is a
// development build.
func classifyRunningVersion(stamp string) (semver.Version, bool) {
	if !releaseStampShape.MatchString(stamp) {
		return semver.Version{}, false
	}
	v, err := semver.ParseTolerant(stamp)
	if err != nil {
		return semver.Version{}, false
	}
	if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
		return semver.Version{}, false
	}
	return v, true
}

// versionTooOldError is returned by checkMinimumVersion when a release build
// is older than a spec's syver-version:.
type versionTooOldError struct {
	required semver.Version
	running  string
}

func (e *versionTooOldError) Error() string {
	return fmt.Sprintf("requires syver %s or later; this is syver %s", e.required, e.running)
}

// devBuildSkipLogged makes the development-build DEBUG line print once per
// process however many marked files a run reads.
var devBuildSkipLogged atomic.Bool

// checkMinimumVersion compares required against the running stamp. A
// development build skips the comparison and logs one DEBUG line saying so.
// Only MAJOR.MINOR.PATCH is compared; a release stamp never has more.
func checkMinimumVersion(required semver.Version, stamp string) error {
	running, release := classifyRunningVersion(stamp)
	if !release {
		if devBuildSkipLogged.CompareAndSwap(false, true) {
			log.Printf("[DEBUG] %s check skipped: development build %q", syverVersionKey, stamp)
		}
		return nil
	}
	if running.LT(required) {
		return &versionTooOldError{required: required, running: stamp}
	}
	return nil
}

// higherSyverVersion returns whichever of a and b names the higher version,
// keeping the value as written. A value that does not parse loses to one that
// does; if neither parses, the result is "". On a tie a is kept.
func higherSyverVersion(a, b string) string {
	va, errA := parseRequiredVersion(a)
	vb, errB := parseRequiredVersion(b)
	switch {
	case errB != nil:
		if errA != nil {
			return ""
		}
		return a
	case errA != nil:
		return b
	case vb.GT(va):
		return b
	default:
		return a
	}
}
