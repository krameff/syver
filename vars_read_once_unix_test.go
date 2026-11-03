//go:build !windows

package syver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
)

// varsPipe returns a /dev/fd/N path backed by a pipe that already holds data
// and whose write end is closed. This is what a shell hands syver for
// `--vars <(command)`: the path can be opened more than once, but only the
// first read sees the bytes; every later read gets an immediate EOF.
func varsPipe(t *testing.T, data string) string {
	t.Helper()
	if _, err := os.Stat("/dev/fd"); err != nil {
		t.Skipf("/dev/fd not available: %v", err)
	}
	resetVarsFileCache(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if _, err := w.WriteString(data); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	return fmt.Sprintf("/dev/fd/%d", r.Fd())
}

// resetVarsFileCache clears the per-process vars file cache so a /dev/fd/N
// number reused by an earlier test cannot serve that test's bytes.
func resetVarsFileCache(t *testing.T) {
	t.Helper()
	reset := func() {
		varsFileCacheMu.Lock()
		varsFileCache = map[string]varsFileEntry{}
		varsFileCacheMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func writeVarsTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestVarsFromFile_PipeReadTwiceReturnsSameVars: the second read of the same
// pipe path must return the vars the first read saw, not an empty map.
func TestVarsFromFile_PipeReadTwiceReturnsSameVars(t *testing.T) {
	path := varsPipe(t, `{"want": true}`)

	for i := 1; i <= 2; i++ {
		vars, err := varsFromFile(path)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if vars["want"] != true {
			t.Fatalf("read %d: want vars[want]=true, got %#v", i, vars)
		}
	}
}

// TestValidate_VarsFromPipe is the reported failure end to end: validate
// loads the spec twice (peek, then the real load), so a --vars pipe used to be
// empty by the second load and the template failed with "map has no entry".
func TestValidate_VarsFromPipe(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "s.yaml")
	writeVarsTestFile(t, spec, fmt.Sprintf("file:\n  %s:\n    exists: {{ .Vars.want }}\n", spec))

	var out bytes.Buffer
	cfg, err := util.NewConfig(
		util.WithSpecFile(spec),
		util.WithVarsFiles([]string{varsPipe(t, `{"want": true}`)}),
		util.WithOutputFormat("documentation"),
		util.WithResultWriter(&out),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(t.Context(), cfg)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if code != 0 {
		t.Fatalf("want exit 0, got %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Count: 1, Failed: 0") {
		t.Fatalf("want one passing test, got\n%s", out.String())
	}
}

// TestValidate_VarsFromPipeWithDiscover covers --discover, which reads the
// vars three times: the discover spec, the main spec's peek, the real load.
// Both specs use the vars.
func TestValidate_VarsFromPipeWithDiscover(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "main.yaml")
	discover := filepath.Join(dir, "discover.yaml")
	writeVarsTestFile(t, discover, fmt.Sprintf(
		"discovery:\n  file:\n    %s:\n      register: found\n      exists: {{ .Vars.want }}\n", discover))
	writeVarsTestFile(t, spec, fmt.Sprintf(
		"{{ if .Discovered.found }}\nfile:\n  %s:\n    exists: {{ .Vars.want }}\n{{ end }}\n", spec))

	var out bytes.Buffer
	cfg, err := util.NewConfig(
		util.WithSpecFile(spec),
		util.WithDiscoverFile(discover),
		util.WithVarsFiles([]string{varsPipe(t, `{"want": true}`)}),
		util.WithOutputFormat("documentation"),
		util.WithResultWriter(&out),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("new config: %v", err)
	}

	code, err := Validate(t.Context(), cfg)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if code != 0 {
		t.Fatalf("want exit 0, got %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Count: 1, Failed: 0") {
		t.Fatalf("want one passing test, got\n%s", out.String())
	}
}

// TestVarsFromFile_MissingFileErrorUnchanged: a missing vars file is still an
// error on the first read, with the same os error as before, and stays an
// error on a repeat read.
func TestVarsFromFile_MissingFileErrorUnchanged(t *testing.T) {
	resetVarsFileCache(t)
	path := filepath.Join(t.TempDir(), "nope.yaml")
	_, want := os.ReadFile(path)

	for i := 1; i <= 2; i++ {
		_, err := varsFromFile(path)
		if err == nil || err.Error() != want.Error() {
			t.Fatalf("read %d: want %v, got %v", i, want, err)
		}
	}
}
