//go:build windows
// +build windows

package system

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// FEAT-013 Task 4 / FEAT-011 W2-12(a), asserted end to end rather than through
// the helper.
//
// mount_supported_test.go proves mountSupported() returns the right answer.
// That is not the same as proving an operator sees it: the whole defect was
// that the honest error existed and was unreachable, so a test of the honest
// error alone would have passed before this fix as well.
//
// These drive DefMount.Exists(), the path a gossfile actually takes.

func windowsTestConfig(t *testing.T) util.Config {
	t.Helper()
	c, err := util.NewConfig()
	if err != nil {
		t.Fatalf("util.NewConfig(): %v", err)
	}
	// A zero Timeout makes getMount time out immediately, which would mask
	// which error is being returned.
	c.Timeout = 5 * time.Second
	return *c
}

// TestMountReportsUnsupportedNotNotFound is the regression this fix exists for.
//
// Before it, every Windows mount check failed with "mountpoint not found",
// because the vendored mountinfo returns an empty table on Windows and
// getMount converts that to ErrMountpointNotFound. The message blamed the
// operator's path for a missing implementation, and for a hardening audit a
// confidently wrong reason is worse than no answer.
func TestMountReportsUnsupportedNotNotFound(t *testing.T) {
	// A drive that certainly exists. The point is that even a real, present
	// mount point reports unsupported, because there is no backend at all.
	for _, mountPoint := range []string{`c:`, `C:\`, `/`} {
		t.Run(mountPoint, func(t *testing.T) {
			m := NewDefMount(context.Background(), mountPoint, nil, windowsTestConfig(t))
			exists, err := m.Exists()
			if err == nil {
				t.Fatalf("Exists() on %q returned (%v, nil); Windows has no mount backend and must say so", mountPoint, exists)
			}
			if errors.Is(err, ErrMountpointNotFound) {
				t.Fatalf("Exists() on %q = %v; this is the regression -- it blames the mountpoint for a missing implementation", mountPoint, err)
			}
			if !errors.Is(err, ErrMountUnsupported) {
				t.Fatalf("Exists() on %q = %v, want ErrMountUnsupported", mountPoint, err)
			}
		})
	}
}

// TestMountAttributesAlsoReportUnsupported checks the other accessors take the
// same path. Each calls setup() independently, so one of them could report
// differently from Exists() without this noticing.
func TestMountAttributesAlsoReportUnsupported(t *testing.T) {
	m := NewDefMount(context.Background(), `c:`, nil, windowsTestConfig(t))

	if _, err := m.Filesystem(); !errors.Is(err, ErrMountUnsupported) {
		t.Errorf("Filesystem() = %v, want ErrMountUnsupported", err)
	}
	if _, err := m.Usage(); !errors.Is(err, ErrMountUnsupported) {
		t.Errorf("Usage() = %v, want ErrMountUnsupported", err)
	}
	if _, err := m.Opts(); !errors.Is(err, ErrMountUnsupported) {
		t.Errorf("Opts() = %v, want ErrMountUnsupported", err)
	}
	if _, err := m.Source(); !errors.Is(err, ErrMountUnsupported) {
		t.Errorf("Source() = %v, want ErrMountUnsupported", err)
	}
}

// TestMountErrorIsActionable guards the operator-facing half. The error is the
// only thing a user sees, and "not supported on this platform" is what tells
// them to stop debugging their path. A future refactor that preserved the
// sentinel identity but lost the wording would pass every test above.
func TestMountErrorIsActionable(t *testing.T) {
	m := NewDefMount(context.Background(), `c:`, nil, windowsTestConfig(t))
	_, err := m.Exists()
	if err == nil {
		t.Fatal("Exists() returned nil error")
	}
	msg := err.Error()
	for _, want := range []string{"mount", "not supported"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q; an operator cannot act on it", msg, want)
		}
	}
	if strings.Contains(msg, "not found") {
		t.Errorf("error %q still reads as a missing mountpoint", msg)
	}
}
