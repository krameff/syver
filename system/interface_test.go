package system

import (
	"errors"
	"net"
	"testing"
)

// TestDefInterfaceExists_GenuinelyAbsent is FEAT-010 Task 5 / Trap 1's
// required regression: a genuinely absent interface name must still report
// (false, nil).
func TestDefInterfaceExists_GenuinelyAbsent(t *testing.T) {
	i := &DefInterface{name: "feat010-nonexistent-iface-xyz"}
	exists, err := i.Exists()
	if exists {
		t.Error("expected a made-up interface name to report as not existing")
	}
	if err != nil {
		t.Errorf("a genuinely absent interface must report (false, nil), got (%v, %v)", exists, err)
	}
}

// TestDefInterfaceExists_RealInterfaceFound sanity-checks the happy path
// against whatever the first real interface on this host is (every host
// running these tests has at least loopback).
func TestDefInterfaceExists_RealInterfaceFound(t *testing.T) {
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Skipf("could not enumerate interfaces in this environment: %v", err)
	}
	i := &DefInterface{name: ifaces[0].Name}
	exists, err := i.Exists()
	if err != nil {
		t.Errorf("unexpected error looking up interface %q: %v", ifaces[0].Name, err)
	}
	if !exists {
		t.Errorf("expected interface %q to exist", ifaces[0].Name)
	}
}

// TestInterfaceLookupRanAndFoundNothing pins the exact string match, since
// it is the seam a future Go stdlib change could silently break.
func TestInterfaceLookupRanAndFoundNothing(t *testing.T) {
	_, err := net.InterfaceByName("feat010-nonexistent-iface-xyz")
	if err == nil {
		t.Fatal("expected an error looking up a made-up interface name")
	}
	if !interfaceLookupRanAndFoundNothing(err) {
		t.Errorf("interfaceLookupRanAndFoundNothing(%v) = false, want true", err)
	}
}

// TestInterfaceLookupRanAndFoundNothing_NegativeCase is SW-8's regression:
// a syscall-level failure (permission, driver, transport) must NOT be
// classified as "found nothing", or it would be silently swallowed into
// (false, nil) exactly like before this fix.
func TestInterfaceLookupRanAndFoundNothing_NegativeCase(t *testing.T) {
	syscallErr := errors.New("permission denied enumerating network adapters")
	if interfaceLookupRanAndFoundNothing(syscallErr) {
		t.Error("a syscall-level failure must not be classified as found-nothing")
	}
}
