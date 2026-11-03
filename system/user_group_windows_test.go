//go:build windows

package system

import (
	"os/user"
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

// The SID_NAME_USE values in user_group_sid.go are written out so the filter
// can be tested off Windows. Hold them to the real ones.
func TestSIDTypeConstantsMatchWindows(t *testing.T) {
	pairs := []struct {
		name      string
		ours, sys uint32
	}{
		{"User", sidTypeUser, windows.SidTypeUser},
		{"Group", sidTypeGroup, windows.SidTypeGroup},
		{"Domain", sidTypeDomain, windows.SidTypeDomain},
		{"Alias", sidTypeAlias, windows.SidTypeAlias},
		{"WellKnownGroup", sidTypeWellKnownGroup, windows.SidTypeWellKnownGroup},
		{"DeletedAccount", sidTypeDeletedAccount, windows.SidTypeDeletedAccount},
		{"Invalid", sidTypeInvalid, windows.SidTypeInvalid},
		{"Unknown", sidTypeUnknown, windows.SidTypeUnknown},
		{"Computer", sidTypeComputer, windows.SidTypeComputer},
		{"Label", sidTypeLabel, windows.SidTypeLabel},
		// x/sys/windows has no constant for SidTypeLogonSession; it follows
		// SidTypeLabel in winnt.h.
		{"LogonSession", sidTypeLogonSession, windows.SidTypeLabel + 1},
	}
	for _, p := range pairs {
		if p.ours != p.sys {
			t.Errorf("sidType%s = %d, x/sys/windows has %d", p.name, p.ours, p.sys)
		}
	}
}

// sessionSIDs are the well-known SIDs a process token carries because of HOW
// the account logged on, not what it is a member of. Groups() must report none
// of them, or a spec generated in one context (WinRM, a scheduled task, a
// console session, a service) fails in another.
var sessionSIDs = []string{
	"S-1-1-0",      // Everyone
	"S-1-2-0",      // LOCAL
	"S-1-2-1",      // CONSOLE LOGON
	"S-1-5-1",      // DIALUP
	"S-1-5-2",      // NETWORK
	"S-1-5-3",      // BATCH
	"S-1-5-4",      // INTERACTIVE
	"S-1-5-6",      // SERVICE
	"S-1-5-11",     // Authenticated Users
	"S-1-5-13",     // TERMINAL SERVER USER
	"S-1-5-14",     // REMOTE INTERACTIVE LOGON
	"S-1-5-15",     // This Organization
	"S-1-5-64-10",  // NTLM Authentication
	"S-1-5-64-14",  // SChannel Authentication
	"S-1-5-64-21",  // Digest Authentication
	"S-1-5-113",    // Local account
	"S-1-5-114",    // Local account and member of Administrators group
	"S-1-16-0",     // integrity levels: untrusted
	"S-1-16-4096",  // low
	"S-1-16-8192",  // medium
	"S-1-16-8448",  // medium plus
	"S-1-16-12288", // high
	"S-1-16-16384", // system
}

// The account running the test is the case that used to be read from the
// process token. Names are compared via their SIDs, because Windows localises
// them. Run it elevated and with a limited token: both must pass.
func TestGroupsCurrentUser(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	u := &DefUser{username: cur.Username}
	groups, err := u.Groups()
	if err != nil {
		t.Fatalf("Groups() for %s: %v", cur.Username, err)
	}
	t.Logf("Groups() for %s = %q", cur.Username, groups)

	// The primary group is always reported.
	primary, _, err := lookupSIDAccount(cur.Gid)
	if err != nil {
		t.Fatalf("lookup primary group %s: %v", cur.Gid, err)
	}
	if !slices.Contains(groups, primary) {
		t.Errorf("Groups() for %s = %q, want it to contain primary group %q (%s)", cur.Username, groups, primary, cur.Gid)
	}

	for _, sid := range sessionSIDs {
		name, _, err := lookupSIDAccount(sid)
		if err != nil {
			continue
		}
		if slices.Contains(groups, name) {
			t.Errorf("Groups() for %s = %q, contains logon-dependent group %q (%s)", cur.Username, groups, name, sid)
		}
	}
}
