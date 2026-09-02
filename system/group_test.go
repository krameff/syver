package system

import (
	"errors"
	"fmt"
	"os/user"
	"testing"
)

// TestGroupLookupFoundNothing mirrors TestUserLookupFoundNothing for
// groupLookupFoundNothing.
func TestGroupLookupFoundNothing(t *testing.T) {
	t.Run("UnknownGroupError means the lookup ran and found nothing", func(t *testing.T) {
		if !groupLookupFoundNothing(user.UnknownGroupError("feat010-nonexistent")) {
			t.Error("expected UnknownGroupError to be classified as found-nothing")
		}
	})

	t.Run("a wrapped UnknownGroupError is still recognised via errors.As unwrapping", func(t *testing.T) {
		wrapped := fmt.Errorf("looking up group: %w", user.UnknownGroupError("feat010-nonexistent"))
		if !groupLookupFoundNothing(wrapped) {
			t.Error("expected a wrapped UnknownGroupError to still be classified as found-nothing")
		}
	})

	t.Run("any other error means the lookup could not run -- must not be swallowed", func(t *testing.T) {
		transportErr := errors.New("could not reach domain controller")
		if groupLookupFoundNothing(transportErr) {
			t.Error("a transport/lookup failure must NOT be classified as found-nothing")
		}
	})
}

// TestDefGroupExists_GenuinelyAbsent is FEAT-010 Task 5 / Trap 1's required
// regression: a genuinely absent group must still report (false, nil).
func TestDefGroupExists_GenuinelyAbsent(t *testing.T) {
	g := &DefGroup{groupname: "feat010-nonexistent-group-xyz"}
	exists, err := g.Exists()
	if exists {
		t.Error("expected a made-up group name to report as not existing")
	}
	if err != nil {
		t.Errorf("a genuinely absent group must report (false, nil), got (%v, %v)", exists, err)
	}
}

// TestDefGroupExists_RealGroupFound sanity-checks the happy path: the
// current user's primary group must still report (true, nil).
func TestDefGroupExists_RealGroupFound(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("could not determine current user in this environment: %v", err)
	}
	grp, err := user.LookupGroupId(cur.Gid)
	if err != nil {
		t.Skipf("could not look up the current user's primary group: %v", err)
	}
	g := &DefGroup{groupname: grp.Name}
	exists, err := g.Exists()
	if err != nil {
		t.Errorf("unexpected error looking up group %q: %v", grp.Name, err)
	}
	if !exists {
		t.Errorf("expected group %q to exist", grp.Name)
	}
}
