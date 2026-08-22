package syver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// A wide dependency wave: one root plus many dependents that all become runnable
// at the same moment, so every worker goroutine writes the scheduler's status
// and completed maps concurrently.
//
// Unsynchronised this was `fatal error: concurrent map writes` -- a runtime
// throw, not a panic, so recover() cannot catch it and no handler-level
// mitigation is possible. Under `serve` a single unauthenticated GET /healthz
// against a spec using depends-on killed the daemon: 30 of 30 runs crashed.
//
// It survived a -race gate over 431 tests because the detector only reports
// races that actually execute, and no existing fixture had two resources
// runnable in the same wave. The fan-out is the whole point of this test; a
// narrower one passes just as happily against the broken code.
func TestWideDependencyWaveDoesNotRaceOnSchedulerState(t *testing.T) {
	const dependents = 40

	var b strings.Builder
	b.WriteString("command:\n  root:\n    exec: \"true\"\n    exit-status: 0\n")
	for i := 0; i < dependents; i++ {
		fmt.Fprintf(&b, "  dep%d:\n    exec: \"true\"\n    exit-status: 0\n    depends-on: [\"command:root\"]\n", i)
	}
	spec := writeSpec(t, b.String())

	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithNoColor())
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	syverConfig, err := loadSyverConfigWithDiscover(t.Context(), cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	out, err := runValidation(t.Context(), system.New(""), *syverConfig,
		cfg.DisabledResourceTypes, cfg.MaxConcurrent)
	if err != nil {
		t.Fatalf("runValidation: %v", err)
	}

	var count, failed int
	for group := range out {
		for _, r := range group {
			count++
			if r.Result == resource.FAIL {
				failed++
			}
		}
	}

	if want := dependents + 1; count != want {
		t.Errorf("ran %d checks, want %d: the dependency wave did not schedule cleanly", count, want)
	}
	if failed != 0 {
		t.Errorf("%d checks failed; all should pass", failed)
	}
}

func writeSpec(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goss.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
