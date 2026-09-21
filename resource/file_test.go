package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysFile is a minimal system.File fake so resource-level tests do not need
// a real file or a working directory service.
type fakeSysFile struct {
	ownerErr, groupErr error
	acl, aclSid        []string
	aclErr             error
}

func (f *fakeSysFile) Path() string                 { return "/srv/app/config.yaml" }
func (f *fakeSysFile) Exists() (bool, error)        { return true, nil }
func (f *fakeSysFile) Contents() (io.Reader, error) { return strings.NewReader(""), nil }
func (f *fakeSysFile) Mode() (string, error)        { return "0640", nil }
func (f *fakeSysFile) Size() (int, error)           { return 128, nil }
func (f *fakeSysFile) Filetype() (string, error)    { return "file", nil }
func (f *fakeSysFile) Owner() (string, error)       { return "svc-app", f.ownerErr }
func (f *fakeSysFile) Uid() (int, error)            { return 20001, nil }
func (f *fakeSysFile) Group() (string, error)       { return "svc-grp", f.groupErr }
func (f *fakeSysFile) Gid() (int, error)            { return 20001, nil }
func (f *fakeSysFile) LinkedTo() (string, error)    { return "", nil }
func (f *fakeSysFile) Md5() (string, error)         { return "", nil }
func (f *fakeSysFile) Sha256() (string, error)      { return "", nil }
func (f *fakeSysFile) Sha512() (string, error)      { return "", nil }
func (f *fakeSysFile) Acl() ([]string, error)       { return f.acl, f.aclErr }
func (f *fakeSysFile) AclSid() ([]string, error)    { return f.aclSid, f.aclErr }

// A wedged directory service must not produce a gossfile that quietly checks
// less than the user thinks.
//
// system/file.go resolves owner and group through `getent`, but only after
// user.LookupId misses -- so this is the LDAP/NIS/SSSD case, which is also
// exactly where a 30 second hang is plausible. The keys were previously guarded
// with `if x, err := ...; err == nil`, so a timeout dropped `owner:` and
// `group:` from the emitted spec, `syver add` exited 0, and nothing was printed.
// The committed file then asserted ownership forever without checking it.
//
// The distinction that makes this subtle: unlike the package path, where the
// system layer folds "absent" into (false, nil), getUserForUid returns a REAL
// error for a uid with no name anywhere. That is the lookup happening and
// finding nothing, which is not a failure and must still omit the key quietly.
// Only "the lookup did not happen" is fatal.
func TestNewFileDoesNotSilentlyDropOwnershipOnTimeout(t *testing.T) {
	t.Run("owner lookup timing out is fatal", func(t *testing.T) {
		f, err := NewFile(&fakeSysFile{ownerErr: context.DeadlineExceeded}, util.Config{})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Assert(t, f == nil, "no file should be returned when ownership is unknown")
	})

	t.Run("group lookup timing out is fatal", func(t *testing.T) {
		f, err := NewFile(&fakeSysFile{groupErr: context.DeadlineExceeded}, util.Config{})
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Assert(t, f == nil)
	})

	t.Run("a cancelled run is fatal too", func(t *testing.T) {
		_, err := NewFile(&fakeSysFile{ownerErr: context.Canceled}, util.Config{})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("a uid with no name is NOT fatal: key omitted, as before", func(t *testing.T) {
		// getUserForUid's own wording for a lookup that ran and found nothing.
		miss := fmt.Errorf("no matching entries in passwd file. getent passwd: %w",
			errors.New("exit status 2"))
		f, err := NewFile(&fakeSysFile{ownerErr: miss}, util.Config{})
		assert.NilError(t, err)
		// Owner is a `matcher`, an interface, so an omitted key is nil rather
		// than "". That is precisely what makes the omission invisible in the
		// emitted YAML: `omitempty` drops it and nothing marks its absence.
		assert.Assert(t, f.Owner == nil, "owner should be omitted, got %v", f.Owner)
		assert.Equal(t, f.Group, matcher("svc-grp"))
	})

	t.Run("a healthy host still records both", func(t *testing.T) {
		f, err := NewFile(&fakeSysFile{}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, f.Owner, matcher("svc-app"))
		assert.Equal(t, f.Group, matcher("svc-grp"))
	})

	// FEAT-010 Task 2 / D-4: system.ErrFileOwnershipUnsupported (what
	// system/file_windows.go's Owner/Group now return, replacing the old
	// fabricated "-1") must NOT match lookupDidNotRun. If it did, `syver add
	// file` on Windows would abort instead of merely omitting the key -- a
	// second, unannounced behaviour change the spec explicitly forbids.
	//
	// This is deliberately runnable on Linux: lookupDidNotRun lives in the
	// untagged resource/file.go, and system.ErrFileOwnershipUnsupported lives
	// in the untagged system/file.go, so neither side of the comparison
	// requires GOOS=windows to exercise.
	t.Run("ErrFileOwnershipUnsupported is NOT fatal: key omitted, add does not abort", func(t *testing.T) {
		f, err := NewFile(&fakeSysFile{ownerErr: system.ErrFileOwnershipUnsupported}, util.Config{})
		assert.NilError(t, err)
		assert.Assert(t, f.Owner == nil, "owner should be omitted on Windows, got %v", f.Owner)
		assert.Equal(t, f.Group, matcher("svc-grp"))
	})

	t.Run("ErrFileOwnershipUnsupported on group is also not fatal", func(t *testing.T) {
		f, err := NewFile(&fakeSysFile{groupErr: system.ErrFileOwnershipUnsupported}, util.Config{})
		assert.NilError(t, err)
		assert.Assert(t, f.Group == nil, "group should be omitted on Windows, got %v", f.Group)
		assert.Equal(t, f.Owner, matcher("svc-app"))
	})
}

// TestLookupDidNotRunDoesNotMatchErrFileOwnershipUnsupported is the direct,
// narrower assertion behind the two subtests above -- it isolates exactly
// the claim D-4 depends on, independent of NewFile's surrounding logic.
func TestLookupDidNotRunDoesNotMatchErrFileOwnershipUnsupported(t *testing.T) {
	if lookupDidNotRun(system.ErrFileOwnershipUnsupported) {
		t.Fatal("lookupDidNotRun must not match ErrFileOwnershipUnsupported: " +
			"doing so turns `syver add file` on Windows into a hard failure " +
			"instead of omitting mode/owner/group, per FEAT-010 D-4")
	}
}

// FEAT-016 D7. `syver add file` emits acl: and NOT acl-sid:, because add exists
// to bootstrap a readable spec and a wall of SIDs is not that. Making a spec
// locale-proof is a deliberate edit to acl-sid:, which docs/windows.md says.
//
// If this test is ever changed to expect both, read D7 first: emitting both
// doubles every discovered file's output for a need most specs do not have.
func TestNewFileEmitsAclNamesOnly(t *testing.T) {
	sys := &fakeSysFile{
		acl:    []string{`BUILTIN\Administrators:(I)(F)`, `BUILTIN\Users:(I)(RX)`},
		aclSid: []string{`S-1-5-32-544:(I)(F)`, `S-1-5-32-545:(I)(RX)`},
	}
	f, err := NewFile(sys, util.Config{})
	assert.NilError(t, err)
	assert.DeepEqual(t, f.Acl, matcher([]string{`BUILTIN\Administrators:(I)(F)`, `BUILTIN\Users:(I)(RX)`}))
	assert.Assert(t, f.AclSid == nil, "add must not emit acl-sid:, got %v", f.AclSid)
}

// A platform with no ACL support must omit the key quietly rather than fail the
// run: `syver add file` on Linux returns ErrFileAclUnsupported for every file, and
// aborting there would break discovery on the platform most people use.
//
// Contrast owner/group above, where a TIMEOUT is fatal. The distinction is the
// same one NewFile's comment draws: a lookup that ran and found nothing may be
// omitted silently; a lookup that never ran may not. An unsupported platform is
// the former -- syver knows the answer, and the answer is "not here".
func TestNewFileOmitsAclWhenUnsupported(t *testing.T) {
	f, err := NewFile(&fakeSysFile{aclErr: system.ErrFileAclUnsupported}, util.Config{})
	assert.NilError(t, err)
	assert.Assert(t, f.Acl == nil, "acl: should be omitted when the platform cannot report it, got %v", f.Acl)
}
