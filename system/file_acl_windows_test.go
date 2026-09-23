//go:build windows
// +build windows

package system

import (
	"testing"

	"golang.org/x/sys/windows"
)

// file_acl.go carries its own copies of the access-mask and ACE-flag constants
// because it has NO build tag -- it cannot import golang.org/x/sys/windows, which
// builds on Windows only, and keeping the rendering platform-neutral is what lets
// file_acl_test.go run in the ordinary Linux gate.
//
// That duplication is the price, and this test is what stops it becoming a
// liability: every local copy is compared against the real constant. A drift here
// would make the Linux tests pass while Windows rendered the wrong token, which
// is the worst arrangement of the two.
func TestAclConstantsMatchXSys(t *testing.T) {
	for _, tt := range []struct {
		name  string
		local uint32
		real  uint32
	}{
		{"DELETE", aclDelete, windows.DELETE},
		{"READ_CONTROL", aclReadControl, windows.READ_CONTROL},
		{"WRITE_DAC", aclWriteDac, windows.WRITE_DAC},
		{"WRITE_OWNER", aclWriteOwner, windows.WRITE_OWNER},
		{"SYNCHRONIZE", aclSynchronize, windows.SYNCHRONIZE},
		{"ACCESS_SYSTEM_SECURITY", aclAccessSystemSecurity, windows.ACCESS_SYSTEM_SECURITY},
		{"MAXIMUM_ALLOWED", aclMaximumAllowed, windows.MAXIMUM_ALLOWED},
		{"GENERIC_ALL", aclGenericAll, windows.GENERIC_ALL},
		{"GENERIC_EXECUTE", aclGenericExecute, windows.GENERIC_EXECUTE},
		{"GENERIC_WRITE", aclGenericWrite, windows.GENERIC_WRITE},
		{"GENERIC_READ", aclGenericRead, windows.GENERIC_READ},
		{"FILE_READ_DATA", aclReadData, windows.FILE_READ_DATA},
		{"FILE_WRITE_DATA", aclWriteData, windows.FILE_WRITE_DATA},
		{"FILE_APPEND_DATA", aclAppendData, windows.FILE_APPEND_DATA},
		{"FILE_READ_EA", aclReadEA, windows.FILE_READ_EA},
		{"FILE_WRITE_EA", aclWriteEA, windows.FILE_WRITE_EA},
		{"FILE_EXECUTE", aclExecute, windows.FILE_EXECUTE},
		{"FILE_READ_ATTRIBUTES", aclReadAttributes, windows.FILE_READ_ATTRIBUTES},
		{"FILE_WRITE_ATTRIBUTES", aclWriteAttributes, windows.FILE_WRITE_ATTRIBUTES},
		{"STANDARD_RIGHTS_REQUIRED", aclStandardRightsRequired, windows.STANDARD_RIGHTS_REQUIRED},
		{"FILE_GENERIC_READ", aclFileGenericRead, windows.FILE_GENERIC_READ},
		{"FILE_GENERIC_WRITE", aclFileGenericWrite, windows.FILE_GENERIC_WRITE},
		{"FILE_GENERIC_EXECUTE", aclFileGenericExecute, windows.FILE_GENERIC_EXECUTE},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.local != tt.real {
				t.Errorf("file_acl.go has %s = %#x, golang.org/x/sys/windows has %#x", tt.name, tt.local, tt.real)
			}
		})
	}

	// The ACE header flags and types, which live in the same duplicated block.
	for _, tt := range []struct {
		name  string
		local uint8
		real  uint8
	}{
		{"OBJECT_INHERIT_ACE", aceObjectInherit, windows.OBJECT_INHERIT_ACE},
		{"CONTAINER_INHERIT_ACE", aceContainerInherit, windows.CONTAINER_INHERIT_ACE},
		{"NO_PROPAGATE_INHERIT_ACE", aceNoPropagateInherit, windows.NO_PROPAGATE_INHERIT_ACE},
		{"INHERIT_ONLY_ACE", aceInheritOnly, windows.INHERIT_ONLY_ACE},
		{"INHERITED_ACE", aceInherited, windows.INHERITED_ACE},
		{"ACCESS_ALLOWED_ACE_TYPE", aceTypeAccessAllowed, windows.ACCESS_ALLOWED_ACE_TYPE},
		{"ACCESS_DENIED_ACE_TYPE", aceTypeAccessDenied, windows.ACCESS_DENIED_ACE_TYPE},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.local != tt.real {
				t.Errorf("file_acl.go has %s = %#x, golang.org/x/sys/windows has %#x", tt.name, tt.local, tt.real)
			}
		})
	}

	// FILE_ALL_ACCESS is NOT in x/sys at v0.48.0 -- checked, which is why
	// file_acl.go composes it. Assert the composition rather than leave it
	// unpinned: STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1FF.
	if want := uint32(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1FF); aclFileAllAccess != want {
		t.Errorf("aclFileAllAccess = %#x, want %#x", aclFileAllAccess, want)
	}
}
