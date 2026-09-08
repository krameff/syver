package system

import (
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRealPathLeavesNonTildePathsAlone is the invariant that must not move:
// realPath is only supposed to do anything at all for a `~` prefix.
func TestRealPathLeavesNonTildePathsAlone(t *testing.T) {
	for _, p := range []string{
		"/etc/passwd",
		"relative/path",
		"",
		`C:\Windows\System32`,
		"file~with~tildes~inside",
	} {
		got, err := realPath(p)
		if err != nil {
			t.Fatalf("realPath(%q) returned error: %v", p, err)
		}
		if got != p {
			t.Errorf("realPath(%q) = %q, want it returned unchanged", p, got)
		}
	}
}

// TestRealPathExpandsTilde covers FEAT-013 / FEAT-011 W2-9 (SW-13).
//
// realPath used to Split and Join on "/" unconditionally. On Windows that left
// `~\Documents\x` as a single segment, so `\Documents\x` was handed to
// user.Lookup as an account name and expansion failed. Splitting at the
// platform's own path separator fixes Windows and is identical to the old
// behaviour on Unix, which is what the shared cases below pin down.
func TestRealPathExpandsTilde(t *testing.T) {
	usr, err := user.Current()
	if err != nil {
		t.Skipf("cannot determine current user: %v", err)
	}
	home := usr.HomeDir

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare tilde", "~", home},
		{"tilde with slash path", "~/Documents/x", filepath.Join(home, "Documents", "x")},
		{"tilde with single segment", "~/x", filepath.Join(home, "x")},
	}
	if runtime.GOOS == "windows" {
		// The case the old implementation could not handle at all.
		cases = append(cases,
			struct {
				name string
				in   string
				want string
			}{"tilde with backslash path", `~\Documents\x`, filepath.Join(home, "Documents", "x")},
		)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := realPath(tc.in)
			if err != nil {
				t.Fatalf("realPath(%q) returned error: %v", tc.in, err)
			}
			want, err := filepath.Abs(tc.want)
			if err != nil {
				t.Fatalf("filepath.Abs(%q): %v", tc.want, err)
			}
			if got != want {
				t.Errorf("realPath(%q) = %q, want %q", tc.in, got, want)
			}
		})
	}
}

// TestRealPathRejectsUnknownUser pins the error path. A `~someone` prefix for
// an account that does not exist must fail rather than silently returning
// something plausible, which is the same principle FEAT-010 applied throughout.
func TestRealPathRejectsUnknownUser(t *testing.T) {
	const absent = "~syver-no-such-account-ffffffff/x"
	got, err := realPath(absent)
	if err == nil {
		t.Fatalf("realPath(%q) = %q with nil error, want an error", absent, got)
	}
	if got != "" {
		t.Errorf("realPath(%q) returned %q alongside its error, want the empty string", absent, got)
	}
	if strings.Contains(got, "syver-no-such-account") {
		t.Errorf("realPath(%q) leaked the unresolved name into its result", absent)
	}
}
