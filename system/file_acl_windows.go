//go:build windows

package system

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FEAT-016. The syscall half of the ACL work; the rendering lives in the
// platform-neutral file_acl.go so it can be tested from Linux.
//
// Privilege: reading the OWNER, GROUP and DACL needs READ_CONTROL on the object,
// which an ordinary user normally holds on a file it can read. Syver on Windows
// is run as an administrator (a recorded decision, see the spec), so no effort is
// made here to degrade gracefully for a caller that lacks it -- the syscall error
// is returned as-is, which is the honest answer and names the real cause.
//
// The SACL (audit entries) is deliberately NOT read. It requires
// SE_SECURITY_NAME, a materially higher bar, and no control in the audit corpus
// asked for it. See the spec's Out of scope.

// errFileNullDacl is returned for an object with a NULL DACL, which grants
// everyone full access. It is reported rather than rendered because both
// alternatives lie: an empty list reads as "no permissions granted", and a
// synthetic "Everyone:(F)" element invents an ACE the object does not contain.
// A NULL DACL is itself a finding, and the one thing a verification tool must not
// do is hide it.
var errFileNullDacl = errors.New("file has a NULL DACL (all access granted to everyone); it has no ACEs to report")

func (f *DefFile) securityDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	return windows.GetNamedSecurityInfo(
		f.path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
}

// sidName renders a SID as DOMAIN\Name, falling back to the SID string when it
// cannot be resolved. An unresolvable SID is the ordinary case for an account
// that has been deleted, and it is what the Windows UI shows too -- more useful
// than an error, and it keeps acl: and acl-sid: the same length.
func sidName(sid *windows.SID) string {
	account, domain, _, err := sid.LookupAccount("")
	if err != nil {
		return sid.String()
	}
	if domain == "" {
		return account
	}
	return domain + `\` + account
}

func (f *DefFile) Owner() (string, error) {
	sd, err := f.securityDescriptor()
	if err != nil {
		return "", err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return "", err
	}
	return sidName(owner), nil
}

func (f *DefFile) Group() (string, error) {
	sd, err := f.securityDescriptor()
	if err != nil {
		return "", err
	}
	group, _, err := sd.Group()
	if err != nil {
		return "", err
	}
	return sidName(group), nil
}

// Acl reports the DACL as one element per ACE, principals as DOMAIN\Name.
func (f *DefFile) Acl() ([]string, error) {
	return f.acl(sidName)
}

// AclSid is the same list with principals as SID strings, so a spec can be
// locale-proof: BUILTIN\Administrators is a localised name and S-1-5-32-544 is
// not. The two projections are the same ACEs in the same order, which a test
// pins, so a spec can be converted between them index for index.
func (f *DefFile) AclSid() ([]string, error) {
	return f.acl(func(sid *windows.SID) string { return sid.String() })
}

func (f *DefFile) acl(principal func(*windows.SID) string) ([]string, error) {
	sd, err := f.securityDescriptor()
	if err != nil {
		return nil, err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	if dacl == nil {
		return nil, errFileNullDacl
	}

	// A DACL with no ACEs is NOT the same as a NULL DACL: it grants nothing to
	// nobody, which is a legitimate (if unusual) state and renders as an empty
	// list. Note an empty list asserts nothing in a gossfile -- isSetWarnEmpty
	// warns about that -- so a spec asserting it must use consist-of: [].
	aces := make([]string, 0, dacl.AceCount)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var header *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &header); err != nil {
			return nil, fmt.Errorf("reading ACE %d of %d: %w", i, dacl.AceCount, err)
		}
		switch header.Header.AceType {
		case aceTypeAccessAllowed, aceTypeAccessDenied:
		default:
			// Anything else on a file DACL -- an object ACE, or a type added to
			// Windows after this was written -- has a different memory layout,
			// so the SID below would be read from the wrong offset. Erroring
			// keeps `consist-of` honest: silently skipping the ACE would let a
			// spec assert "these and no others" against a list syver knew was
			// incomplete.
			return nil, fmt.Errorf("unsupported ACE type %d at index %d of %d; syver reads allow and deny ACEs only",
				header.Header.AceType, i, dacl.AceCount)
		}
		sid := (*windows.SID)(unsafe.Pointer(&header.SidStart))
		aces = append(aces, formatAce(principal(sid), header.Header.AceType, header.Header.AceFlags, uint32(header.Mask)))
	}
	return aces, nil
}
