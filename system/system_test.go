package system

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/krameff/syver/util"
)

// fakePath returns a hasCommand stand-in that reports only the named
// executables as present, so detection does not depend on what is installed
// on the machine running the tests.
func fakePath(cmds ...string) func(string) bool {
	return func(c string) bool {
		for _, have := range cmds {
			if c == have {
				return true
			}
		}
		return false
	}
}

func fixture(name string) string {
	return filepath.Join("testdata", "distros", name)
}

// TestDetectors pins the exact distro, package manager and service manager
// each fixture tree resolves to. Every row asserts all three, so gutting any
// one detector (or the distro detection the other two lean on) fails rows.
func TestDetectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tree    string
		goos    string
		path    []string
		distro  string
		pkg     string
		service string
	}{
		// Container-style hosts with no systemctl on PATH: the service
		// manager comes from the distro.
		{"ubuntu, no systemctl", "ubuntu-22.04", "linux", nil, "ubuntu", "dpkg", "upstart"},
		{"debian, no systemctl", "debian-12", "linux", nil, "debian", "dpkg", "init"},
		{"alma, no systemctl", "alma-9", "linux", nil, "redhat", "rpm", "init"},
		{"amazon via system-release only", "amazon-2", "linux", nil, "redhat", "rpm", "init"},
		{"alpine", "alpine-3.20", "linux", nil, "alpine", "apk", "alpineinit"},
		{"arch, no systemctl", "arch", "linux", nil, "arch", "pacman", "systemd"},

		// Full hosts with systemctl: systemd wins over the distro default,
		// and Debian before 9 gets the legacy service checks.
		{"ubuntu with systemctl", "ubuntu-22.04", "linux", []string{"systemctl", "dpkg"}, "ubuntu", "dpkg", "systemd"},
		{"debian 12 with systemctl", "debian-12", "linux", []string{"systemctl"}, "debian", "dpkg", "systemd"},
		{"debian 8 with systemctl", "debian-8", "linux", []string{"systemctl"}, "debian", "dpkg", "systemdlegacy"},
		{"alma with systemctl", "alma-9", "linux", []string{"systemctl", "rpm"}, "redhat", "rpm", "systemd"},

		// The distro decides the package manager even when another
		// manager's binary is also on PATH.
		{"alma with dpkg also installed", "alma-9", "linux", []string{"dpkg"}, "redhat", "rpm", "init"},

		// No recognisable release files: fall back to binaries on PATH, in
		// dpkg, rpm, apk, pacman order.
		{"unknown, nothing on PATH", "empty", "linux", nil, "", "", "init"},
		{"unknown, rpm on PATH", "empty", "linux", []string{"rpm"}, "", "rpm", "init"},
		{"unknown, apk and pacman on PATH", "empty", "linux", []string{"pacman", "apk"}, "", "apk", "init"},
		{"unknown, dpkg wins the fallback order", "empty", "linux", []string{"pacman", "rpm", "dpkg"}, "", "dpkg", "init"},
		{"lsb-release without Ubuntu is not ubuntu", "mint-lsb", "linux", []string{"dpkg"}, "", "dpkg", "init"},

		// Windows short-circuits the service manager before anything else.
		{"windows", "empty", "windows", []string{"systemctl"}, "", "", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := detector{root: fixture(tt.tree), goos: tt.goos, hasCommand: fakePath(tt.path...)}
			if got := d.distro(); got != tt.distro {
				t.Errorf("distro() = %q, want %q", got, tt.distro)
			}
			if got := d.packageManager(); got != tt.pkg {
				t.Errorf("packageManager() = %q, want %q", got, tt.pkg)
			}
			if got := d.service(); got != tt.service {
				t.Errorf("service() = %q, want %q", got, tt.service)
			}
		})
	}
}

// TestDetectorsUseHost checks the exported functions are the host-rooted
// detector and nothing else.
func TestDetectorsUseHost(t *testing.T) {
	t.Parallel()
	h := hostDetector()
	if h.root != "" || h.goos != runtime.GOOS {
		t.Fatalf("hostDetector() = {root: %q, goos: %q}, want {root: \"\", goos: %q}", h.root, h.goos, runtime.GOOS)
	}
	if got, want := DetectDistro(), h.distro(); got != want {
		t.Errorf("DetectDistro() = %q, host detector says %q", got, want)
	}
	if got, want := DetectPackageManager(), h.packageManager(); got != want {
		t.Errorf("DetectPackageManager() = %q, host detector says %q", got, want)
	}
	if got, want := DetectService(), h.service(); got != want {
		t.Errorf("DetectService() = %q, host detector says %q", got, want)
	}
}

func TestIsLegacyDebianVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{"7.11\n", true},
		{"8.11\n", true},
		{"8.0", true},
		{"9.0\n", false},
		{"9.13\n", false},
		{"10.13\n", false},
		{"12.5\n", false},
		{"0.1", true},
		{"bookworm/sid\n", false},
		{"trixie/sid", false},
		{"", false},
		{".5", false},
		{"x.5", false},
		{" 8.11", false},
	}
	for _, tt := range tests {
		if got := isLegacyDebianVersion([]byte(tt.in)); got != tt.want {
			t.Errorf("isLegacyDebianVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestHasCommand(t *testing.T) {
	t.Parallel()
	if !HasCommand("sh") {
		t.Error("System didn't have sh!")
	}
}

// funcName returns a human-readable identity for a Package constructor, for
// use in test failure messages -- the constructors themselves are not
// comparable with %v.
func funcName(f func(context.Context, string, *System, util.Config) Package) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// funcEqual compares two Package constructors by underlying function
// pointer, which is the standard idiom for asserting func equality in Go
// tests.
func funcEqual(a, b func(context.Context, string, *System, util.Config) Package) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

// TestResolveNewPackage_WindowsBranch is SW-1's regression test (FEAT-010
// Task 1). It exercises the Windows-detects-nothing branch entirely from
// Linux, by passing "windows" as goos directly rather than relying on
// runtime.GOOS -- see resolveNewPackage's own comment for why it takes goos
// as a parameter.
func TestResolveNewPackage_WindowsBranch(t *testing.T) {
	t.Parallel()

	noneDetected := func() string { return "" }
	detects := func(p string) func() string { return func() string { return p } }

	tests := []struct {
		name     string
		explicit string
		detect   func() string
		goos     string
		want     func(context.Context, string, *System, util.Config) Package
	}{
		{
			name:     "windows, nothing detected, no explicit flag -> NullPackage",
			explicit: "",
			detect:   noneDetected,
			goos:     "windows",
			want:     NewNullPackage,
		},
		{
			name:     "linux, nothing detected, no explicit flag -> RpmPackage (unchanged default)",
			explicit: "",
			detect:   noneDetected,
			goos:     "linux",
			want:     NewRpmPackage,
		},
		{
			name:     "darwin, nothing detected, no explicit flag -> RpmPackage (unchanged default)",
			explicit: "",
			detect:   noneDetected,
			goos:     "darwin",
			want:     NewRpmPackage,
		},
		{
			// AC-1 / D-3: an explicit --package rpm must keep working on
			// Windows -- the Windows branch applies only to the DETECTED
			// case, never overriding an explicit flag.
			name:     "windows, explicit --package rpm -> RpmPackage, not overridden",
			explicit: "rpm",
			detect:   func() string { t.Fatal("detect must not be called when explicit is valid"); return "" },
			goos:     "windows",
			want:     NewRpmPackage,
		},
		{
			name:     "windows, explicit --package dpkg -> DebPackage, not overridden",
			explicit: "dpkg",
			detect:   func() string { t.Fatal("detect must not be called when explicit is valid"); return "" },
			goos:     "windows",
			want:     NewDebPackage,
		},
		{
			name:     "windows, something IS detected -> use it, do not fall to NullPackage",
			explicit: "",
			detect:   detects("apk"),
			goos:     "windows",
			want:     NewAlpinePackage,
		},
		{
			name:     "unsupported explicit value falls through to detection, same as before this change",
			explicit: "not-a-real-manager",
			detect:   detects("pacman"),
			goos:     "linux",
			want:     NewPacmanPackage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNewPackage(tt.explicit, tt.detect, tt.goos)
			if !funcEqual(got, tt.want) {
				t.Errorf("resolveNewPackage(%q, detect, %q) = %s, want %s",
					tt.explicit, tt.goos, funcName(got), funcName(tt.want))
			}
		})
	}
}
