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
// Getting that boundary wrong is the whole risk. Warn on too much and every
// `syver autoadd` over an ordinary directory emits noise about paths that are
// not mounts; warn on too little and the original silent skip comes straight
// back.

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
