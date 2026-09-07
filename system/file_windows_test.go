//go:build windows
// +build windows

package system

import (
	"errors"
	"testing"
)

// TestDefFileAccessorsReturnSentinelOnWindows is FEAT-010 Task 2's
// Windows-only proof that Mode/Owner/Uid/Group/Gid all return
// ErrFileOwnershipUnsupported rather than the old fabricated "-1"/-1 with a
// nil error. It cannot run on this sandbox (no Windows host, no Actions
// minutes -- see FEAT-010 §5) but is written now so GOOS=windows go vet
// (Task 8) catches any signature drift, and so it exists to run the moment a
// Windows runner is available again.
func TestDefFileAccessorsReturnSentinelOnWindows(t *testing.T) {
	f := &DefFile{path: `C:\Windows\System32\drivers\etc\hosts`}

	if mode, err := f.Mode(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Mode() = (%q, %v), want (_, ErrFileOwnershipUnsupported)", mode, err)
	}
	if owner, err := f.Owner(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Owner() = (%q, %v), want (_, ErrFileOwnershipUnsupported)", owner, err)
	}
	if uid, err := f.Uid(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Uid() = (%d, %v), want (_, ErrFileOwnershipUnsupported)", uid, err)
	}
	if group, err := f.Group(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Group() = (%q, %v), want (_, ErrFileOwnershipUnsupported)", group, err)
	}
	if gid, err := f.Gid(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Gid() = (%d, %v), want (_, ErrFileOwnershipUnsupported)", gid, err)
	}
}
