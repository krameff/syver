package system

import (
	"errors"
	"os"
	"path/filepath"
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

// TestDefKernelParamValue_ReadsDottedKeyAsPath pins the lookup that replaced
// go-sysctl's Get against a fixture tree, so it runs identically on hosts with
// and without /proc/sys: dots become directories, and the trailing newline
// procfs always writes is trimmed.
//
// REVERT-PROOF: dropping the TrimSpace fails the value comparison, and dropping
// the dot-to-separator replacement fails the lookup with os.ErrNotExist.
func TestDefKernelParamValue_ReadsDottedKeyAsPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "net", "ipv4")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ip_forward"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := procSysPath
	procSysPath = root
	defer func() { procSysPath = orig }()

	got, err := (&DefKernelParam{key: "net.ipv4.ip_forward"}).Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if got != "1" {
		t.Errorf("Value() = %q, want %q", got, "1")
	}

	_, err = (&DefKernelParam{key: "net.ipv4.not_a_param"}).Value()
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Value() for a missing key error = %v, want os.ErrNotExist", err)
	}
}
