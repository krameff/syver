package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysFile is a minimal system.File fake so resource-level tests do not need
// a real file or a working directory service.
type fakeSysFile struct {
	ownerErr, groupErr error
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
}
