package system

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// These tests date from FEAT-013 Task 4, which made Windows report that mount:
// was not implemented instead of blaming the operator's mountpoint. FEAT-018
// then moved the POSIX lookup into mount_posix.go and gave Windows its own
// (mount_windows.go, tested in mount_windows_test.go). What these still pin is
// the half that carries risk for the platforms mount: has always served: Linux
// and macOS behaviour is unchanged.

// newTestConfig builds the minimal util.Config these tests need. NewConfig
// returns an error, and a test that ignored it could mask a config change with
// a nil-pointer panic further down.
func newTestConfig(t *testing.T) util.Config {
	t.Helper()
	c, err := util.NewConfig()
	if err != nil {
		t.Fatalf("util.NewConfig(): %v", err)
	}
	// The zero-value Timeout means getMount times out immediately, which would
	// mask the very distinction these tests exist to check.
	c.Timeout = 5 * time.Second
	return *c
}

// TestMountLookupStillRunsOnSupportedPlatforms is the regression guard for the
// half of this change that carries risk.
//
// The POSIX lookup must still run: a bogus mountpoint comes back as
// ErrMountpointNotFound, the everyday "ran and found nothing" answer, and must
// NOT come back as ErrMountUnsupported. Confusing those two is the defect
// FEAT-013 fixed on Windows, in reverse.
func TestMountLookupStillRunsOnSupportedPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mount table; windows is covered by mount_windows_test.go")
	}

	m := NewDefMount(context.Background(), "/syver-no-such-mountpoint-ffffffff", nil, newTestConfig(t))
	_, err := m.Exists()
	if err == nil {
		t.Fatal("Exists() on a bogus mountpoint returned nil error, want ErrMountpointNotFound")
	}
	if errors.Is(err, ErrMountUnsupported) {
		t.Fatalf("Exists() = %v; the capability check short-circuited a platform that DOES support mount", err)
	}
	if !errors.Is(err, ErrMountpointNotFound) {
		t.Fatalf("Exists() = %v, want ErrMountpointNotFound", err)
	}
}

// TestMountResolvesARealMountpoint proves the lookup does more than fail
// consistently: a real mountpoint resolves, with a filesystem and a usage.
func TestMountResolvesARealMountpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mount table; windows is covered by mount_windows_test.go")
	}

	m := NewDefMount(context.Background(), "/", nil, newTestConfig(t))
	exists, err := m.Exists()
	if err != nil {
		t.Fatalf("Exists() on \"/\" returned %v, want it to resolve", err)
	}
	if !exists {
		t.Fatal("Exists() on \"/\" = false, want true")
	}
	if _, err := m.Filesystem(); err != nil {
		t.Errorf("Filesystem() on \"/\" returned %v, want a filesystem type", err)
	}
	if _, err := m.Usage(); err != nil {
		t.Errorf("Usage() on \"/\" returned %v, want a usage percentage", err)
	}
}
