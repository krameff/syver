package system

import (
	"errors"
	"reflect"
	"testing"
)

func TestIsGroupSIDType(t *testing.T) {
	cases := map[uint32]bool{
		sidTypeUser:           false,
		sidTypeGroup:          true,
		sidTypeDomain:         false,
		sidTypeAlias:          true,
		sidTypeWellKnownGroup: true,
		sidTypeDeletedAccount: false,
		sidTypeInvalid:        false,
		sidTypeUnknown:        false,
		sidTypeComputer:       false,
		sidTypeLabel:          false,
		sidTypeLogonSession:   false,
	}
	for sidType, want := range cases {
		if got := isGroupSIDType(sidType); got != want {
			t.Errorf("isGroupSIDType(%d) = %v, want %v", sidType, got, want)
		}
	}
}

func TestSplitAccountName(t *testing.T) {
	cases := []struct{ in, domain, name string }{
		{`HOST\vagrant`, "HOST", "vagrant"},
		{`vagrant`, "", "vagrant"},
		{`CORP\sub\odd`, `CORP\sub`, "odd"},
	}
	for _, c := range cases {
		d, n := splitAccountName(c.in)
		if d != c.domain || n != c.name {
			t.Errorf("splitAccountName(%q) = %q, %q; want %q, %q", c.in, d, n, c.domain, c.name)
		}
	}
}

// fakeSIDs maps the SIDs a test passes as a primary group.
var fakeSIDs = map[string]struct {
	name    string
	sidType uint32
}{
	"S-1-5-21-1-2-3-513":  {"None", sidTypeGroup},
	"S-1-5-21-1-2-3-1001": {"someone", sidTypeUser},
}

func fakeSIDLookup(sid string) (string, uint32, error) {
	a, ok := fakeSIDs[sid]
	if !ok {
		return "", 0, errors.New("no mapping")
	}
	return a.name, a.sidType, nil
}

func fakeLocal(groups []string, err error) (localGroupsLookup, *[]string) {
	var asked []string
	return func(qualified string) ([]string, error) {
		asked = append(asked, qualified)
		return groups, err
	}, &asked
}

func TestMembershipGroupsLocalPlusPrimary(t *testing.T) {
	local, asked := fakeLocal([]string{"Users", "Administrators"}, nil)
	got, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-1-2-3-513", local, fakeSIDLookup)
	if err != nil {
		t.Fatalf("membershipGroups: %v", err)
	}
	want := []string{"Administrators", "None", "Users"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("membershipGroups = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(*asked, []string{`HOST\vagrant`}) {
		t.Errorf("local groups asked for %q, want [HOST\\vagrant]", *asked)
	}
}

// The result must be a function of the account alone. Nothing the caller passes
// describes the logon, so two calls for the same account agree by construction;
// this pins that the output is deduplicated and order-independent too, which
// is what an exact `groups:` list (ConsistOf) relies on.
func TestMembershipGroupsDeterministic(t *testing.T) {
	a, _ := fakeLocal([]string{"Users", "Administrators", "Users"}, nil)
	b, _ := fakeLocal([]string{"Administrators", "Users"}, nil)
	ga, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-1-2-3-513", a, fakeSIDLookup)
	if err != nil {
		t.Fatal(err)
	}
	gb, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-1-2-3-513", b, fakeSIDLookup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ga, gb) {
		t.Errorf("membershipGroups differs by input order or duplicates: %q vs %q", ga, gb)
	}
}

func TestMembershipGroupsPrimaryNotAGroupIsSkipped(t *testing.T) {
	local, _ := fakeLocal([]string{"Users"}, nil)
	got, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-1-2-3-1001", local, fakeSIDLookup)
	if err != nil {
		t.Fatalf("membershipGroups: %v", err)
	}
	if want := []string{"Users"}; !reflect.DeepEqual(got, want) {
		t.Errorf("membershipGroups = %q, want %q", got, want)
	}
}

func TestMembershipGroupsNoPrimary(t *testing.T) {
	local, _ := fakeLocal(nil, nil)
	got, err := membershipGroups(`HOST\vagrant`, "", local, fakeSIDLookup)
	if err != nil || len(got) != 0 {
		t.Errorf("membershipGroups = %q, %v; want empty, nil", got, err)
	}
}

func TestMembershipGroupsErrorsFail(t *testing.T) {
	local, _ := fakeLocal(nil, errors.New("access denied"))
	if _, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-1-2-3-513", local, fakeSIDLookup); err == nil {
		t.Error("want an error when the local-group lookup fails, got nil")
	}
	ok, _ := fakeLocal([]string{"Users"}, nil)
	if _, err := membershipGroups(`HOST\vagrant`, "S-1-5-21-7-7-7-513", ok, fakeSIDLookup); err == nil {
		t.Error("want an error when the primary group SID cannot be resolved, got nil")
	}
}
