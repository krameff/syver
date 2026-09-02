package system

import (
	"errors"
	"fmt"
	"os/user"
	"testing"
)

// TestUserLookupFoundNothing is the direct, synthetic-error proof behind
// TestDefUserExists_GenuinelyAbsent: it isolates exactly the classification
// SW-9's fix depends on, including the "could not run" case that is hard to
// provoke from a real user.Lookup call without an actual unreachable domain
// controller.
func TestUserLookupFoundNothing(t *testing.T) {
	t.Run("UnknownUserError means the lookup ran and found nothing", func(t *testing.T) {
		if !userLookupFoundNothing(user.UnknownUserError("feat010-nonexistent")) {
			t.Error("expected UnknownUserError to be classified as found-nothing")
		}
	})

	t.Run("a wrapped UnknownUserError is still recognised via errors.As unwrapping", func(t *testing.T) {
		wrapped := fmt.Errorf("looking up user: %w", user.UnknownUserError("feat010-nonexistent"))
		if !userLookupFoundNothing(wrapped) {
			t.Error("expected a wrapped UnknownUserError to still be classified as found-nothing")
		}
	})

	t.Run("any other error means the lookup could not run -- must not be swallowed", func(t *testing.T) {
		transportErr := errors.New("could not reach domain controller")
		if userLookupFoundNothing(transportErr) {
			t.Error("a transport/lookup failure must NOT be classified as found-nothing -- " +
				"this is exactly SW-9: folding it in silently discards a real error")
		}
	})
}

// TestDefUserExists_GenuinelyAbsent is FEAT-010 Task 5 / Trap 1's required
// regression: a genuinely absent user must still report (false, nil),
// unchanged from before this fix, on every platform this test runs on.
func TestDefUserExists_GenuinelyAbsent(t *testing.T) {
	u := &DefUser{username: "feat010-nonexistent-user-xyz"}
	exists, err := u.Exists()
	if exists {
		t.Error("expected a made-up username to report as not existing")
	}
	if err != nil {
		t.Errorf("a genuinely absent user must report (false, nil), got (%v, %v) -- "+
			"user.Lookup ran and found nothing, that is not an error", exists, err)
	}
}

// TestDefUserExists_RealUserFound sanity-checks the happy path still works:
// a real, current user must still report (true, nil).
func TestDefUserExists_RealUserFound(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("could not determine current user in this environment: %v", err)
	}
	u := &DefUser{username: cur.Username}
	exists, err := u.Exists()
	if err != nil {
		t.Errorf("unexpected error looking up current user %q: %v", cur.Username, err)
	}
	if !exists {
		t.Errorf("expected current user %q to exist", cur.Username)
	}
}
