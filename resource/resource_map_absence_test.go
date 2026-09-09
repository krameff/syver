package resource

import (
	"errors"
	"fmt"
	"testing"

	"github.com/krameff/syver/system"
)

// FEAT-013 Task 1 / FEAT-011 W2-8. AppendSysResourceIfExists discarded its
// Exists() error entirely, so an unreadable resource was indistinguishable
// from one that genuinely did not exist. It now skips and warns -- but only
// for errors that mean the lookup FAILED, not for the ones that mean it ran
// and found nothing.
//
// Getting that boundary wrong is the whole risk: warn on too much and autoadd
// emits noise for outcomes that are answers rather than failures; warn on too
// little and the original silent skip comes straight back.
//
// CORRECTION, 2026-09-08, after Vision checked the basis rather than the
// pattern. An earlier version of this comment justified the mount case as
// "every `syver autoadd` over an ordinary directory emits noise about paths
// that are not mounts". That scenario cannot occur. `mount:` has no
// `AutoAddSpec` -- only file, group, package, port, process, service and user
// do -- so `ErrMountpointNotFound` never reaches this function today, and
// `file:`'s Exists() is an os.Lstat that touches no mount lookup at all.
//
// SECOND CORRECTION, same day, same mistake one level down. The line above
// originally went on to claim `ErrServiceNotFound` IS reachable, because
// `service:` has an AutoAddSpec and returns that sentinel. Wrong again, and
// wrong the same way: it checked that the type is autoadd-capable and that the
// sentinel exists, without checking WHICH METHOD returns it.
// `AppendSysResourceIfExists` calls only `Exists()`, and
// `ServiceWindows.Exists()` returns `(false, nil)` for a missing service
// (system/service_windows.go). `ErrServiceNotFound` comes from `Enabled()` and
// `Running()`, which this path never calls.
//
// So BOTH listed sentinels are currently unreachable from the only call site,
// and this function is forward-looking insurance rather than live protection.
// It is kept deliberately: FEAT-018 makes the mount branch live, and the safe
// direction is to over-list, since an unlisted sentinel costs one noisy warning
// while a wrongly-listed one silently hides a broken lookup. But do not read
// the comment above as describing something that fires today.

func TestIsExpectedAbsenceAcceptsFoundNothing(t *testing.T) {
	for _, err := range []error{
		system.ErrMountpointNotFound,
		system.ErrServiceNotFound,
		fmt.Errorf("wrapped: %w", system.ErrMountpointNotFound),
		fmt.Errorf("wrapped: %w", system.ErrServiceNotFound),
	} {
		if !isExpectedAbsence(err) {
			t.Errorf("isExpectedAbsence(%v) = false; this is a lookup that ran and found nothing, and must not warn", err)
		}
	}
}

func TestIsExpectedAbsenceRejectsRealFailures(t *testing.T) {
	for _, err := range []error{
		system.ErrRegistryUnsupported,
		system.ErrMountUnsupported,
		system.ErrKernelParamUnsupported,
		system.ErrFileOwnershipUnsupported,
		errors.New("permission denied"),
		errors.New("getMount operation timed out"),
	} {
		if isExpectedAbsence(err) {
			t.Errorf("isExpectedAbsence(%v) = true; this is a lookup that could not run, and must be reported", err)
		}
	}
}

// The unsupported sentinels are the case that motivated this. On a platform
// where a resource type is not implemented, every Exists() returns one of
// them, and the old code turned all of them into a silent "does not exist".
func TestUnsupportedSentinelsAreNotTreatedAsAbsence(t *testing.T) {
	if isExpectedAbsence(system.ErrMountUnsupported) {
		t.Fatal("ErrMountUnsupported was classified as absence; a platform with no mount lookup would autoadd silently")
	}
	if isExpectedAbsence(system.ErrRegistryUnsupported) {
		t.Fatal("ErrRegistryUnsupported was classified as absence; a non-Windows autoadd would hide the reason")
	}
}
