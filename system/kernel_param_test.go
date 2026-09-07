package system

import (
	"errors"
	"testing"
)

// TestDefKernelParamExists_GenuinelyAbsent is FEAT-010 Task 5 / Trap 1's
// required regression: on a host that genuinely has /proc/sys (this
// sandbox), a made-up key must still report (false, nil), unchanged.
func TestDefKernelParamExists_GenuinelyAbsent(t *testing.T) {
	if !procSysExists() {
		t.Skip("no /proc/sys on this host -- covered by the no-procfs subtest instead")
	}
	k := &DefKernelParam{key: "feat010.totally-made-up-key-xyz"}
	exists, err := k.Exists()
	if exists {
		t.Error("expected a made-up sysctl key to report as not existing")
	}
	if err != nil {
		t.Errorf("a genuinely absent key on a host WITH /proc/sys must report (false, nil), got (%v, %v)", exists, err)
	}
}

// TestDefKernelParamExists_RealKeyFound sanity-checks the happy path.
func TestDefKernelParamExists_RealKeyFound(t *testing.T) {
	if !procSysExists() {
		t.Skip("no /proc/sys on this host")
	}
	k := &DefKernelParam{key: "kernel.hostname"}
	exists, err := k.Exists()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected kernel.hostname to exist on a Linux host")
	}
}

// TestDefKernelParamExists_NoProcSys is the substitution half of Trap 1's
// fix for kernel-param: on a host with no /proc/sys at all (simulated here
// via the procSysExists seam, since this sandbox cannot actually remove
// /proc/sys), Exists must return the sentinel rather than a bare (false,
// nil) that claims to have checked something it never looked at.
func TestDefKernelParamExists_NoProcSys(t *testing.T) {
	orig := procSysExists
	procSysExists = func() bool { return false }
	defer func() { procSysExists = orig }()

	// Deliberately a key that fails Value() lookup, so Exists reaches the
	// procSysExists branch -- a key that genuinely exists (like
	// kernel.hostname) short-circuits before ever consulting it.
	k := &DefKernelParam{key: "feat010.totally-made-up-key-xyz"}
	exists, err := k.Exists()
	if exists {
		t.Error("Exists must not report true when /proc/sys does not exist")
	}
	if !errors.Is(err, ErrKernelParamUnsupported) {
		t.Errorf("Exists() with no /proc/sys = (%v, %v), want (false, ErrKernelParamUnsupported)", exists, err)
	}
}
