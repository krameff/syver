package syver

import (
	"strings"
	"testing"
)

// The seed corpus below runs with every `go test`. `make fuzz` explores
// further from it; a failing input it finds is written to testdata/fuzz/ and
// from then on runs as a seed too.

// FuzzParseRequiredVersion: any accepted value has no pre-release left on it,
// and its canonical form parses back to the same version.
func FuzzParseRequiredVersion(f *testing.F) {
	for _, s := range []string{
		"0.20.0", "v0.20.0", "0.20", "1", "0.20.0-rc1", "", "~", ">=0.20.0",
		"~0.20", "0.20.0 - 0.21.0", "00.20", "0.020.0", "0.20.0.1", "latest",
		"true", "0x14", "v", "-1", "1.2.3+build", " 0.20.0 ", "\t", "1..2",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		v, err := parseRequiredVersion(raw)
		if err != nil {
			if !strings.Contains(err.Error(), syverVersionKey) {
				t.Fatalf("error for %q does not name the key: %v", raw, err)
			}
			return
		}
		if len(v.Pre) != 0 || len(v.Build) != 0 {
			t.Fatalf("%q kept a pre-release or build part: %v", raw, v)
		}
		again, err := parseRequiredVersion(v.String())
		if err != nil || !again.EQ(v) {
			t.Fatalf("%q -> %v does not round-trip: %v, %v", raw, v, again, err)
		}
	})
}

// FuzzClassifyRunningVersion: only a plain X.Y.Z stamp other than 0.0.0 is a
// release, and a development build never fails the minimum-version check.
func FuzzClassifyRunningVersion(f *testing.F) {
	for _, s := range []string{
		"0.20.0", "v0.20.0", "", "0.0.0", "v0.16.0-6-gabc1234", "0.16.1-next",
		"0.20.0-rc1", "0.20.0-SNAPSHOT-abc", "c103c3a", "1.2", "01.2.3",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, stamp string) {
		v, release := classifyRunningVersion(stamp)
		if release {
			if !releaseStampShape.MatchString(stamp) {
				t.Fatalf("%q is not X.Y.Z but counted as a release", stamp)
			}
			if v.Major == 0 && v.Minor == 0 && v.Patch == 0 {
				t.Fatalf("%q counted as a release", stamp)
			}
			return
		}
		required, _ := parseRequiredVersion("999.0.0")
		if err := checkMinimumVersion(required, stamp); err != nil {
			t.Fatalf("development build %q failed the version check: %v", stamp, err)
		}
	})
}

// FuzzScanTopLevelYAML: the scan never panics on any input, and it never
// reports a key that is legal or starts with x-.
func FuzzScanTopLevelYAML(f *testing.F) {
	for _, s := range []string{
		"syver-version: 0.20.0\ncommand: {}\n",
		"prot:\n  tcp:22:\n    listening: true\n",
		"x-a: &a\n  prot: 1\n<<: *a\n",
		"x-a: &a\n  <<: *a\n<<: *a\n",
		"<<: [*a]\n",
		"<<: {bogus: 1}\n",
		"a: &a [*a]\n<<: *a\n",
		"- not\n- a map\n",
		"syver-version:\n",
		"syver-version: [1]\n",
		"x-plain: 1\nx-map:\n  a: 1\n",
		"? [complex]\n: key\n",
		"",
	} {
		f.Add(s)
	}
	legal := legalTopLevelKeys()
	f.Fuzz(func(t *testing.T, doc string) {
		scan := scanTopLevelYAML([]byte(doc), "fuzz.yaml")
		for _, w := range scan.unknown {
			if legal[w.Key] || strings.HasPrefix(w.Key, "x-") {
				t.Fatalf("reported exempt key %q in %q", w.Key, doc)
			}
		}
	})
}
