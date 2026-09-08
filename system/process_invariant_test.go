package system

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/krameff/syver/util"
)

// TestProcessNeverReportsNothingSuccessfully is the invariant FEAT-013 Task 3
// introduces, written so that it does not presume what the underlying library
// does on any given platform.
//
// The defect was that Status() returned an empty slice AND a nil error on
// Windows, because gopsutil fails for every process there and the loop skipped
// them all. The check ran, found the process, said nothing about it, and
// passed.
//
// Asserting "Status() errors on Windows" would encode a claim about gopsutil
// that has not been measured on a Windows host, and this project has been
// burnt by exactly that kind of assumption. So the assertion is the weaker and
// more durable one: for a process that demonstrably EXISTS, reporting nothing
// while also reporting success is not an available outcome. Either the
// attribute is known, or the reason it is not is.
//
// It runs everywhere on purpose. On Linux and macOS it passes because the
// lookup works, which keeps the invariant honest rather than making it a
// Windows-only special case.
func TestProcessNeverReportsNothingSuccessfully(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot determine own executable: %v", err)
	}
	name := filepath.Base(exe)

	cfg, err := util.NewConfig()
	if err != nil {
		t.Fatalf("util.NewConfig(): %v", err)
	}

	p := NewDefProcess(context.Background(), name, New(""), *cfg)

	exists, err := p.Exists()
	if err != nil {
		t.Skipf("process lookup itself failed on %s (%v); this test has nothing to assert about", runtime.GOOS, err)
	}
	if !exists {
		t.Skipf("could not find own process %q in the table on %s; environment-dependent, not a defect", name, runtime.GOOS)
	}

	t.Run("status", func(t *testing.T) {
		got, err := p.Status()
		if err != nil {
			t.Logf("Status() on %s reported: %v", runtime.GOOS, err)
			return
		}
		if len(got) == 0 {
			t.Fatalf("Status() returned an empty result AND a nil error for a process that exists; "+
				"that is the silent-nothing outcome FEAT-013 removed (GOOS=%s)", runtime.GOOS)
		}
	})

	t.Run("user", func(t *testing.T) {
		got, err := p.User()
		if err != nil {
			t.Logf("User() on %s reported: %v", runtime.GOOS, err)
			return
		}
		if len(got) == 0 {
			t.Fatalf("User() returned an empty result AND a nil error for a process that exists; "+
				"same silent-nothing outcome (GOOS=%s)", runtime.GOOS)
		}
	})
}
