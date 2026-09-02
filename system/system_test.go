package system

import (
	"context"
	"reflect"
	"runtime"
	"testing"

	"github.com/krameff/syver/util"
)

type noInputs func() string

// test that a function with no inputs returns one of the expected strings
func testOutputs(f noInputs, validOutputs []string, t *testing.T) {
	output := f()
	// use reflect to get the name of the function
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	failed := true
	for _, valid := range validOutputs {
		if output == valid {
			failed = false
		}
	}
	if failed {
		t.Errorf("Function %v returned %v, which is not one of %v", name, output, validOutputs)
	}
}

func TestPackageManager(t *testing.T) {
	t.Parallel()
	testOutputs(
		DetectPackageManager,
		[]string{"dpkg", "rpm", "apk", "pacman", ""},
		t,
	)
}

func TestDetectService(t *testing.T) {
	t.Parallel()
	testOutputs(
		DetectService,
		[]string{"systemd", "init", "alpineinit", "upstart", "windows", ""},
		t,
	)
}

func TestDetectDistro(t *testing.T) {
	t.Parallel()
	testOutputs(
		DetectDistro,
		[]string{"ubuntu", "redhat", "alpine", "arch", "debian", ""},
		t,
	)
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
// tests (see TestPackageManager/testOutputs above for the same pattern
// applied to a simpler func type).
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
