package system

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// These tests exist for FEAT-013 Task 4 / FEAT-011 W2-12(a). The change makes
// Windows report honestly that mount: is not implemented, instead of blaming
// the operator's mountpoint. Making Windows truthful is the easy half. The
// acceptance criterion is the other half: that Linux and macOS behaviour is
// unchanged. That is what these assert.

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

// TestMountSupportedMatchesPlatform pins the capability answer itself. If a
// future change makes mountSupported return an error on a platform where
// mount: is implemented, every mount assertion on that platform starts failing
// and this catches it in one line.
func TestMountSupportedMatchesPlatform(t *testing.T) {
	err := mountSupported()
	if runtime.GOOS == "windows" {
		if !errors.Is(err, ErrMountUnsupported) {
			t.Fatalf("mountSupported() = %v, want ErrMountUnsupported on windows", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("mountSupported() = %v, want nil on %s", err, runtime.GOOS)
	}
}

// TestMountLookupStillRunsOnSupportedPlatforms is the regression guard for the
// half of this change that carries risk.
//
// The capability check was deliberately placed BEFORE getMount rather than
// reordering getMount and getUsage, because those two are shared by every OS
// and their order determines which error a missing path produces. This asserts
// the shared path still reaches the real lookup: a bogus mountpoint must come
// back as ErrMountpointNotFound, the everyday "ran and found nothing" answer,
// and must NOT come back as ErrMountUnsupported.
//
// Getting those two confused is precisely the defect being fixed, in reverse.
func TestMountLookupStillRunsOnSupportedPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no mount lookup; covered by TestMountSupportedMatchesPlatform")
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
// consistently. Without it, a mountSupported() that wrongly returned an error
// would still pass the test above if the error text happened to match.
func TestMountResolvesARealMountpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no mount lookup")
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
