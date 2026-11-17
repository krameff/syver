package syver

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
)

func checkErr(t *testing.T, err error, format string, a ...any) {
	t.Helper()
	if err == nil {
		return
	}

	t.Fatalf(format+": "+err.Error(), a...)
}

func TestConfigMerge(t *testing.T) {
	var g1json = `file:
  /etc/passwd:
    exists: true
    mode: "0644"
    size: 1722
    owner: root
    group: root
    filetype: file
    contains: []`

	var g2json = `service:
  sshd:
    enabled: true
    running: true
`

	g1, err := ReadJSONData([]byte(g1json), true, "")
	checkErr(t, err, "reading g1 failed")
	_, ok := g1.Services["sshd"]
	if ok {
		t.Fatalf("did not expect sshd service")
	}

	g2, err := ReadJSONData([]byte(g2json), true, "")
	checkErr(t, err, "reading g1 failed")

	g1.Merge(g2)
	_, ok = g1.Files["/etc/passwd"]
	if !ok {
		t.Fatalf("expected passwd file, got none")
	}
	_, ok = g1.Services["sshd"]
	if !ok {
		t.Fatalf("expected sshd service, got none")
	}
}

func TestUseAsPackage(t *testing.T) {
	output := &bytes.Buffer{}

	// temp spec file
	fh, err := os.CreateTemp("", "*.yaml")
	checkErr(t, err, "temp file failed")
	fh.Close()

	// new config that doesnt spam output etc
	cfg, err := util.NewConfig(util.WithFormatOptions("pretty"), util.WithResultWriter(output), util.WithSpecFile(fh.Name()))
	checkErr(t, err, "new config failed")

	// adds the os tmp dir to the goss spec file
	err = AddResources(fh.Name(), "File", []string{os.TempDir()}, cfg)
	checkErr(t, err, "could not add resource %q", os.TempDir())

	// validate and sanity check, compare structured vs direct results etc
	results, err := ValidateResults(t.Context(), cfg)
	checkErr(t, err, "check failed")

	found := 0
	passed := 0
	for rg := range results {
		for _, r := range rg {
			found++

			if r.Result == resource.SUCCESS {
				passed++
			}
		}
	}

	code, err := Validate(t.Context(), cfg)
	checkErr(t, err, "check failed")
	if code != 0 {
		t.Fatalf("check failed, expected 0 got %d", code)
	}

	res := &outputs.StructuredOutput{}
	err = json.Unmarshal(output.Bytes(), res)
	checkErr(t, err, "unmarshal failed")

	if res.Summary.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", res.Summary.Failed)
	}

	if len(res.Results) != found {
		t.Fatalf("expected %d results for %d", found, len(res.Results))
	}

	okcount := 0
	for _, r := range res.Results {
		if r.Result == resource.SUCCESS {
			okcount++
		}
	}

	if okcount != passed {
		t.Fatalf("expected %d passed but got %d", passed, okcount)
	}
}

func TestSkipResourcesByType(t *testing.T) {
	output := &bytes.Buffer{}

	// temp spec file
	fh, err := os.CreateTemp("", "*.yaml")
	checkErr(t, err, "temp file failed")
	fh.Close()

	// new config that doesnt spam output etc
	cfg, err := util.NewConfig(util.WithFormatOptions("pretty"), util.WithResultWriter(output), util.WithSpecFile(fh.Name()), util.WithDisabledResourceTypes("file"))
	checkErr(t, err, "new config failed")

	// adds the os tmp dir to the goss spec file
	err = AddResources(fh.Name(), "File", []string{os.TempDir()}, cfg)
	checkErr(t, err, "could not add resource %q", os.TempDir())

	// validate and sanity check, compare structured vs direct results etc
	results, err := ValidateResults(t.Context(), cfg)
	checkErr(t, err, "check failed")

	total, skipped := 0, 0
	for rg := range results {
		for _, r := range rg {
			total++
			if r.Skipped {
				skipped++
			}
		}
	}

	// Derived, not a constant. This asserted `skipped != 5` until 2026-09-02,
	// which made it platform-dependent without saying so: FEAT-010 stops
	// `syver add file` writing mode/owner/group on Windows (they were
	// fabricated "-1" values), so the generated spec carries three fewer
	// attributes there and the count dropped 5 -> 2. The test failed on
	// Windows while passing on Linux, and the CODE was right -- the number
	// was wrong. What this test actually cares about is that disabling a
	// resource type skips every test generated for it, which is true on both
	// platforms whatever `add` chose to write.
	if total == 0 {
		t.Fatal("no results at all: the fixture generated nothing, so an " +
			"all-skipped assertion would pass vacuously")
	}
	if skipped != total {
		t.Fatalf("disabling the file resource type must skip every generated "+
			"test: %d of %d skipped", skipped, total)
	}
}
