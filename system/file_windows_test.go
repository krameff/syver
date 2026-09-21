//go:build windows
// +build windows

package system

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// TestDefFileAccessorsReturnSentinelOnWindows began as FEAT-010 Task 2's proof
// that Mode/Owner/Uid/Group/Gid all returned ErrFileOwnershipUnsupported rather
// than the old fabricated "-1"/-1 with a nil error.
//
// FEAT-016 made Owner and Group real, so those two assertions were removed rather
// than the file deleted: the three below are STILL TRUE and two of them are
// permanent. Uid and Gid are a category mismatch (Windows identifies accounts by
// SID, syver parses these as integers) and Mode stays unsupported by decision,
// because a POSIX mode derived from a DACL would be a confident wrong answer. See
// file_windows.go's header.
func TestDefFileAccessorsReturnSentinelOnWindows(t *testing.T) {
	f := &DefFile{path: `C:\Windows\System32\drivers\etc\hosts`}

	if mode, err := f.Mode(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Mode() = (%q, %v), want (_, ErrFileOwnershipUnsupported)", mode, err)
	}
	if uid, err := f.Uid(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Uid() = (%d, %v), want (_, ErrFileOwnershipUnsupported)", uid, err)
	}
	if gid, err := f.Gid(); !errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Errorf("Gid() = (%d, %v), want (_, ErrFileOwnershipUnsupported)", gid, err)
	}
}

// aceShape matches one rendered element: a principal, a colon, then one or more
// parenthesised groups. Deliberately loose about WHICH groups -- file_acl_test.go
// pins the exact spellings from Linux, and this test's job is the syscall path.
var aceShape = regexp.MustCompile(`^.+:(\([A-Za-z0-9,]+\))+$`)

// TestDefFileOwnerAndGroupAreReal is FEAT-016's proof that the sentinel is GONE
// rather than shadowed, which the spec calls out as the worse outcome than not
// starting. It asserts the shape rather than a literal owner: the owner of this
// path is an implementation detail of the Windows build, and pinning it would
// make the test a Windows-version detector.
func TestDefFileOwnerAndGroupAreReal(t *testing.T) {
	f := &DefFile{path: `C:\Windows\System32\drivers\etc\hosts`}

	owner, err := f.Owner()
	if err != nil {
		t.Fatalf("Owner() error = %v, want nil", err)
	}
	if owner == "" {
		t.Error("Owner() returned an empty string with a nil error, which is the fabricated-answer shape FEAT-010 removed")
	}
	if errors.Is(err, ErrFileOwnershipUnsupported) {
		t.Error("Owner() still returns the FEAT-010 sentinel")
	}

	group, err := f.Group()
	if err != nil {
		t.Fatalf("Group() error = %v, want nil", err)
	}
	if group == "" {
		t.Error("Group() returned an empty string with a nil error")
	}
}

// TestDefFileAcl exercises the DACL read against a path whose ACL is stable
// across Windows builds. Measured on Server 2025, every ACE on it is inherited,
// so each element carries (I) -- but that is asserted as "at least one", not
// "all", so a build that adds an explicit ACE does not fail the suite.
func TestDefFileAcl(t *testing.T) {
	f := &DefFile{path: `C:\Windows\System32\drivers\etc\hosts`}

	acl, err := f.Acl()
	if err != nil {
		t.Fatalf("Acl() error = %v, want nil", err)
	}
	if len(acl) == 0 {
		t.Fatal("Acl() returned no ACEs for a path that has them")
	}
	for _, ace := range acl {
		if !aceShape.MatchString(ace) {
			t.Errorf("Acl() element %q does not match PRINCIPAL:(GROUP)...", ace)
		}
	}
	var sawInherited bool
	for _, ace := range acl {
		if strings.Contains(ace, "(I)") {
			sawInherited = true
		}
	}
	if !sawInherited {
		t.Errorf("Acl() reported no inherited ACE for %s, which inherits all of its permissions: %v", f.path, acl)
	}
}

// TestDefFileAclSidMatchesAcl is the acceptance criterion for the two-attribute
// decision (spec D7): acl: and acl-sid: must be the same ACEs in the same order,
// so a spec can be converted between the readable and the locale-proof form
// index for index. If they can drift, the pair is worse than either alone.
func TestDefFileAclSidMatchesAcl(t *testing.T) {
	f := &DefFile{path: `C:\Windows\System32\drivers\etc\hosts`}

	acl, err := f.Acl()
	if err != nil {
		t.Fatalf("Acl() error = %v", err)
	}
	aclSid, err := f.AclSid()
	if err != nil {
		t.Fatalf("AclSid() error = %v", err)
	}
	if len(acl) != len(aclSid) {
		t.Fatalf("Acl() has %d elements and AclSid() has %d; the two projections must describe the same ACEs\nacl:     %v\nacl-sid: %v",
			len(acl), len(aclSid), acl, aclSid)
	}
	for i := range acl {
		// Split on the FIRST colon: a Windows account name cannot contain one,
		// and neither can a SID string, so everything after it is the rights and
		// flags -- which must be identical between the two projections.
		nameRights := strings.SplitN(acl[i], ":", 2)
		sidRights := strings.SplitN(aclSid[i], ":", 2)
		if len(nameRights) != 2 || len(sidRights) != 2 {
			t.Fatalf("unparseable element pair at %d: %q / %q", i, acl[i], aclSid[i])
		}
		if nameRights[1] != sidRights[1] {
			t.Errorf("index %d: acl rights %q != acl-sid rights %q (%q vs %q)",
				i, nameRights[1], sidRights[1], acl[i], aclSid[i])
		}
		if !strings.HasPrefix(sidRights[0], "S-1-") {
			t.Errorf("index %d: acl-sid principal %q is not a SID string", i, sidRights[0])
		}
	}
}
