package system

import (
	"fmt"
	"sort"
	"strings"
)

// SID_NAME_USE values, from winnt.h. They are part of the Windows ABI, so they
// are spelled out here rather than imported from golang.org/x/sys/windows: this
// file has no build tag, so the logic below can be unit tested on every
// platform. user_group_windows_test.go asserts they match x/sys/windows.
const (
	sidTypeUser           uint32 = 1
	sidTypeGroup          uint32 = 2
	sidTypeDomain         uint32 = 3
	sidTypeAlias          uint32 = 4
	sidTypeWellKnownGroup uint32 = 5
	sidTypeDeletedAccount uint32 = 6
	sidTypeInvalid        uint32 = 7
	sidTypeUnknown        uint32 = 8
	sidTypeComputer       uint32 = 9
	sidTypeLabel          uint32 = 10
	sidTypeLogonSession   uint32 = 11
)

// isGroupSIDType reports whether a SID_NAME_USE value denotes a group: a domain
// or local group, a local alias such as BUILTIN\Administrators, or a
// well-known group. It is the set os/user accepts for a group.
func isGroupSIDType(t uint32) bool {
	switch t {
	case sidTypeGroup, sidTypeAlias, sidTypeWellKnownGroup:
		return true
	}
	return false
}

// sidLookup resolves a SID string to its account name and SID_NAME_USE.
type sidLookup func(sid string) (name string, sidType uint32, err error)

// localGroupsLookup returns the names of the local groups an account belongs
// to, directly or through a nested group. qualified is the account as os/user
// names it, DOMAIN\user.
type localGroupsLookup func(qualified string) ([]string, error)

// splitAccountName splits DOMAIN\user into its two halves. A name with no
// backslash has an empty domain.
func splitAccountName(qualified string) (domain, name string) {
	if i := strings.LastIndex(qualified, `\`); i >= 0 {
		return qualified[:i], qualified[i+1:]
	}
	return "", qualified
}

// membershipGroups builds an account's group list on Windows from its
// membership: the local groups it belongs to, plus its primary group
// (primarySID, os/user's Gid). The result is sorted and has no duplicates.
//
// It deliberately does NOT read a process token, even for the account running
// syver. A token also lists groups that describe the logon rather than the
// account (NETWORK, BATCH, INTERACTIVE, CONSOLE LOGON, LOCAL, Authenticated
// Users, the NTLM authentication package, the integrity label), so a token-based
// list would change with how syver was started and would mean something
// different for the running account than for every other one.
//
// A primary group SID that does not resolve to a group is left out. Any lookup
// error fails the whole list rather than returning one that is silently short.
func membershipGroups(qualified, primarySID string, local localGroupsLookup, lookup sidLookup) ([]string, error) {
	names, err := local(qualified)
	if err != nil {
		return nil, fmt.Errorf("unable to find groups for user %v: %w", qualified, err)
	}
	if primarySID != "" {
		name, sidType, err := lookup(primarySID)
		if err != nil {
			return nil, fmt.Errorf("unable to find primary group %s for user %v: %w", primarySID, qualified, err)
		}
		if isGroupSIDType(sidType) {
			names = append(names, name)
		}
	}

	seen := make(map[string]bool, len(names))
	groups := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		groups = append(groups, n)
	}
	sort.Strings(groups)
	return groups, nil
}
