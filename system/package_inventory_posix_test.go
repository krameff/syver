//go:build !windows

package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shimOnly makes PATH a directory holding only the given scripts, so the real
// package manager on the test host can never answer instead.
func shimOnly(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// S-4: every way a helper can fail is an error, never an empty inventory.
//
// REVERT-PROOF: making runInventoryCommand return (cmd, nil) for an
// *exec.ExitError lets the exit-1 case parse empty output and fails it with
// the empty-inventory error instead of the stderr text it asserts.
func TestInventoryCommandFailuresAreErrors(t *testing.T) {
	backends := map[string]func(context.Context) PackageInventory{
		"rpm":        func(ctx context.Context) PackageInventory { return &rpmInventory{ctx: ctx} },
		"dpkg-query": func(ctx context.Context) PackageInventory { return &dpkgInventory{ctx: ctx} },
		"pacman":     func(ctx context.Context) PackageInventory { return &pacmanInventory{ctx: ctx} },
	}
	for tool, mk := range backends {
		t.Run(tool+" missing", func(t *testing.T) {
			shimOnly(t, nil)
			got, err := mk(t.Context()).Packages()
			if err == nil || !errors.Is(err, exec.ErrNotFound) {
				t.Fatalf("got %v, %v; want a not-found error", got, err)
			}
		})
		t.Run(tool+" exit 1", func(t *testing.T) {
			shimOnly(t, map[string]string{tool: "echo 'database is locked' >&2; exit 1"})
			got, err := mk(t.Context()).Packages()
			if err == nil || !strings.Contains(err.Error(), "database is locked") || !strings.Contains(err.Error(), "exited 1") {
				t.Fatalf("got %v, %v; want an exit-1 error carrying stderr", got, err)
			}
		})
		t.Run(tool+" zero packages", func(t *testing.T) {
			shimOnly(t, map[string]string{tool: "exit 0"})
			got, err := mk(t.Context()).Packages()
			if !errors.Is(err, ErrPackageInventoryEmpty) {
				t.Fatalf("got %v, %v; want ErrPackageInventoryEmpty", got, err)
			}
		})
		t.Run(tool+" unparsable", func(t *testing.T) {
			shimOnly(t, map[string]string{tool: "echo 'garbage'"})
			got, err := mk(t.Context()).Packages()
			if err == nil || !strings.Contains(err.Error(), "unparsable") {
				t.Fatalf("got %v, %v; want an unparsable-line error", got, err)
			}
		})
		t.Run(tool+" cancelled", func(t *testing.T) {
			shimOnly(t, map[string]string{tool: "printf 'bash\\t1\\n'"})
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			got, err := mk(ctx).Packages()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, %v; want context.Canceled", got, err)
			}
		})
	}
}

// S-7, the parts that need no real package manager: equality runs nothing,
// a missing tool is an error, a version that is not one never reaches a
// helper, and output the tool should never print is an error.
func TestVersionComparerWithoutTools(t *testing.T) {
	shimOnly(t, nil)
	for _, m := range SupportedPackageManagers() {
		c, err := NewPackageVersionComparer(t.Context(), m)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := c.Compare("1.0-1", "1.0-1"); n != 0 || err != nil {
			t.Errorf("%s: equal versions: got %d, %v; want 0 with no tool on PATH", m, n, err)
		}
		if _, err := c.Compare("1.0-1", "1.0-2"); err == nil {
			t.Errorf("%s: missing tool compared without error", m)
		}
		for _, bad := range []string{`1.0"); os.exit(1) --`, "-1.0", "1.0 2.0", "", "1%{_bindir}"} {
			if _, err := c.Compare(bad, "1.0"); err == nil || !strings.Contains(err.Error(), "is not a version") {
				t.Errorf("%s: %q: got %v, want a not-a-version error", m, bad, err)
			}
		}
	}
	if _, err := NewPackageVersionComparer(t.Context(), "yum"); err == nil {
		t.Error("an unsupported manager got a comparer")
	}

	for tool, out := range map[string]string{"rpm": "x", "apk": "?", "vercmp": ""} {
		shimOnly(t, map[string]string{tool: "echo '" + out + "'"})
		manager := map[string]string{"rpm": "rpm", "apk": "apk", "vercmp": "pacman"}[tool]
		c, _ := NewPackageVersionComparer(t.Context(), manager)
		if _, err := c.Compare("1", "2"); err == nil {
			t.Errorf("%s printing %q compared without error", tool, out)
		}
	}
	shimOnly(t, map[string]string{"dpkg": "echo 'bad syntax' >&2; exit 2"})
	c, _ := NewPackageVersionComparer(t.Context(), "dpkg")
	if _, err := c.Compare("1", "2"); err == nil || !strings.Contains(err.Error(), "bad syntax") {
		t.Errorf("dpkg exit 2: got %v, want an error carrying stderr", err)
	}
}

// S-7, against the real tools. Each vector is written for this test, not taken
// from a package manager's own suite. A tool that is not installed skips its
// rows; SYVER_REQUIRE_VERCMP names the manager that must be present, so the
// per-distro runs cannot pass by skipping.
func TestVersionComparerVectors(t *testing.T) {
	vectors := map[string][]struct {
		a, b string
		want int
	}{
		"rpm": {
			{"1.0-1", "1.0-2", -1},
			{"1:1.0-1", "2.0-1", 1},    // epoch wins
			{"1.0~rc1-1", "1.0-1", -1}, // tilde sorts before release
			{"5.1.8-6.el9_1", "5.1.8-6.el9", 1},
			{"1.10-1", "1.9-1", 1}, // numeric, not lexical
		},
		"dpkg": {
			{"1.0-1", "1.0-2", -1},
			{"1:1.0", "2.0", 1},
			{"1.0~rc1", "1.0", -1},
			{"5.1-6ubuntu1.1", "5.1-6ubuntu1", 1},
			{"1.10", "1.9", 1},
		},
		"apk": {
			{"1.2.3-r4", "1.2.3-r10", -1}, // -rN is numeric
			{"1.0_rc1-r0", "1.0-r0", -1},  // _rc sorts before release
			{"1.10-r0", "1.9-r0", 1},
			{"2.0-r0", "1.0-r5", 1},
		},
		"pacman": {
			{"1.0-1", "1.0-2", -1}, // pkgrel
			{"1:1.0-1", "2.0-1", 1},
			{"1.10-1", "1.9-1", 1},
			{"5.3.20-1", "5.3.3-1", 1},
		},
	}
	tools := map[string]string{"rpm": "rpm", "dpkg": "dpkg", "apk": "apk", "pacman": "vercmp"}
	required := os.Getenv("SYVER_REQUIRE_VERCMP")
	for manager, rows := range vectors {
		t.Run(manager, func(t *testing.T) {
			if _, err := exec.LookPath(tools[manager]); err != nil {
				if required == manager {
					t.Fatalf("SYVER_REQUIRE_VERCMP=%s but %s is not installed", manager, tools[manager])
				}
				t.Skipf("%s not installed", tools[manager])
			}
			c, err := NewPackageVersionComparer(t.Context(), manager)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				for _, dir := range []struct {
					a, b string
					want int
				}{{r.a, r.b, r.want}, {r.b, r.a, -r.want}} {
					got, err := c.Compare(dir.a, dir.b)
					if err != nil || got != dir.want {
						t.Errorf("Compare(%q, %q) = %d, %v; want %d", dir.a, dir.b, got, err, dir.want)
					}
				}
			}
		})
	}
}
