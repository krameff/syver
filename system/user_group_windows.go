package system

import (
	"fmt"
	"os/user"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Groups reports the account's local group memberships plus its primary group,
// the same way for every account. See membershipGroups for why the process
// token is not used, even for the account running syver.
func (u *DefUser) Groups() ([]string, error) {
	usr, err := user.Lookup(u.username)
	if err != nil {
		return nil, err
	}
	return membershipGroups(usr.Username, usr.Gid, netUserLocalGroups, lookupSIDAccount)
}

// NetUserGetLocalGroups is not wrapped by golang.org/x/sys/windows.
var procNetUserGetLocalGroups = windows.NewLazySystemDLL("netapi32.dll").NewProc("NetUserGetLocalGroups")

// localGroupUsersInfo0 is LOCALGROUP_USERS_INFO_0.
type localGroupUsersInfo0 struct {
	name *uint16
}

const (
	lgIncludeIndirect  = 1          // LG_INCLUDE_INDIRECT
	maxPreferredLength = 0xFFFFFFFF // MAX_PREFERRED_LENGTH
)

// netUserLocalGroups returns the local groups qualified (DOMAIN\user) belongs
// to, including those it belongs to through a global group, via
// NetUserGetLocalGroups on the local machine. The account name is qualified
// with its domain only when the host is joined to a domain, which is the same
// rule os/user applies when it lists another account's groups.
func netUserLocalGroups(qualified string) ([]string, error) {
	domain, name := splitAccountName(qualified)
	query := name
	if domain != "" && isDomainJoined() {
		query = qualified
	}
	q, err := windows.UTF16PtrFromString(query)
	if err != nil {
		return nil, err
	}

	var buf *byte
	var entriesRead, totalEntries uint32
	r0, _, _ := syscall.SyscallN(procNetUserGetLocalGroups.Addr(),
		0, // local machine
		uintptr(unsafe.Pointer(q)),
		0, // level 0: LOCALGROUP_USERS_INFO_0
		lgIncludeIndirect,
		uintptr(unsafe.Pointer(&buf)),
		maxPreferredLength,
		uintptr(unsafe.Pointer(&entriesRead)),
		uintptr(unsafe.Pointer(&totalEntries)),
	)
	if buf != nil {
		defer func() { _ = windows.NetApiBufferFree(buf) }()
	}
	if r0 != 0 {
		return nil, fmt.Errorf("NetUserGetLocalGroups(%q): %w", query, syscall.Errno(r0))
	}
	if entriesRead == 0 {
		return nil, nil
	}

	entries := unsafe.Slice((*localGroupUsersInfo0)(unsafe.Pointer(buf)), entriesRead)
	names := make([]string, 0, entriesRead)
	for _, e := range entries {
		if e.name == nil {
			continue
		}
		names = append(names, windows.UTF16PtrToString(e.name))
	}
	return names, nil
}

// isDomainJoined reports whether the host is joined to a domain. A failure to
// ask is treated as not joined, as os/user does.
func isDomainJoined() bool {
	var domain *uint16
	var status uint32
	if err := windows.NetGetJoinInformation(nil, &domain, &status); err != nil {
		return false
	}
	_ = windows.NetApiBufferFree((*byte)(unsafe.Pointer(domain)))
	return status == windows.NetSetupDomainName
}

// lookupSIDAccount resolves a SID string with LookupAccountSid on the local
// machine and returns the bare account name, matching the name os/user gives a
// group.
func lookupSIDAccount(s string) (string, uint32, error) {
	sid, err := windows.StringToSid(s)
	if err != nil {
		return "", 0, err
	}
	name, _, sidType, err := sid.LookupAccount("")
	if err != nil {
		return "", 0, err
	}
	return name, sidType, nil
}
