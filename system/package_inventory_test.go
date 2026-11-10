package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// The fixtures under testdata/pkg-inventory were captured from the integration
// images (integration-tests/Dockerfile_*): the exact command each backend runs,
// or for apk the database file itself, after `apk add --virtual .syver-e2e`.
func readInventoryFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "pkg-inventory", name))
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout may convert these to CRLF; the tools never print it.
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func wantVersions(t *testing.T, got map[string][]string, name string, want ...string) {
	t.Helper()
	if strings.Join(got[name], ",") != strings.Join(want, ",") {
		t.Errorf("%s: got versions %q, want %q", name, got[name], want)
	}
}

// S-1: each backend parses real output from the integration images.
func TestInventoryParsesCapturedOutput(t *testing.T) {
	t.Run("rpm rockylinux9", func(t *testing.T) {
		got, err := parseRpmInventory(readInventoryFixture(t, "rpm-rockylinux9.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 169 {
			t.Errorf("got %d names, want 169", len(got))
		}
		wantVersions(t, got, "bash", "5.1.8-6.el9_1")
		wantVersions(t, got, "shadow-utils", "2:4.9-8.el9")
		// Two imported signing keys: one name, two versions.
		wantVersions(t, got, "gpg-pubkey", "3228467c-613798eb", "350d275d-6279464b")
	})
	t.Run("rpm almalinux10", func(t *testing.T) {
		got, err := parseRpmInventory(readInventoryFixture(t, "rpm-almalinux10.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 157 {
			t.Errorf("got %d names, want 157", len(got))
		}
		wantVersions(t, got, "bash", "5.2.26-6.el10")
		wantVersions(t, got, "gpg-pubkey", "c2a1e572-668fe8ef", "e37ed158-65785fa9")
	})
	t.Run("dpkg jammy", func(t *testing.T) {
		got, err := parseDpkgInventory(readInventoryFixture(t, "dpkg-jammy.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 165 {
			t.Errorf("got %d names, want 165", len(got))
		}
		wantVersions(t, got, "bash", "5.1-6ubuntu1.1")
	})
	t.Run("pacman arch", func(t *testing.T) {
		got, err := parsePacmanInventory(readInventoryFixture(t, "pacman-arch.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 142 {
			t.Errorf("got %d names, want 142", len(got))
		}
		wantVersions(t, got, "bash", "5.3.20-1")
	})
	t.Run("apk alpine3.20 db", func(t *testing.T) {
		got, err := parseApkInstalledDB(readInventoryFixture(t, "apk-alpine3.20-installed"))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 35 {
			t.Errorf("got %d names, want 35", len(got))
		}
		wantVersions(t, got, "busybox", "1.36.1-r31")
		wantVersions(t, got, ".syver-e2e", "20261110.152804")
	})
}

// S-2: a removed-but-not-purged package is dropped, and a multiarch package
// listed once per architecture is one name. Lines are real dpkg-query output
// from an Ubuntu 24.04 host.
//
// REVERT-PROOF: removing the status filter in parseDpkgInventory keeps
// javascript-common and fails this test.
func TestDpkgInventoryFiltersStatusAndFoldsMultiarch(t *testing.T) {
	out := "javascript-common\tdeinstall ok config-files\t11+nmu1\n" +
		"libc6\tinstall ok installed\t2.39-0ubuntu8.9\n" +
		"libc6\tinstall ok installed\t2.39-0ubuntu8.9\n" +
		"libcap2\tinstall ok installed\t1:2.66-5ubuntu2.4\n" +
		"libcap2\tinstall ok installed\t1:2.66-5ubuntu2.4\n" +
		"gcc-14-base\thold ok installed\t14.2.0-4ubuntu2~24.04.1\n" +
		"gcc-14-base\thold ok installed\t14.2.0-4ubuntu2~24.04.1\n" +
		"oldpkg\tunknown ok not-installed\t\n"
	got, err := parseDpkgInventory(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["javascript-common"]; ok {
		t.Error("deinstall ok config-files package was kept")
	}
	if _, ok := got["oldpkg"]; ok {
		t.Error("not-installed package was kept")
	}
	wantVersions(t, got, "libc6", "2.39-0ubuntu8.9")
	wantVersions(t, got, "libcap2", "1:2.66-5ubuntu2.4")
	wantVersions(t, got, "gcc-14-base", "14.2.0-4ubuntu2~24.04.1")
	if len(got) != 3 {
		t.Errorf("got %d names %v, want 3", len(got), got)
	}
}

// S-3: names and versions come from P: and V:, never from splitting a joined
// string on "-". A virtual package's version has no -rN suffix.
//
// REVERT-PROOF: replacing parseApkInstalledDB with a split of `apk info -v`
// output on the last two "-" separators turns ".build-deps-20261104.172653"
// into name ".build", version "deps-20261104.172653", and fails here.
func TestApkInstalledDB(t *testing.T) {
	db := "C:Q1abc=\nP:name-with-dashes\nV:1.2.3-r4\nA:x86_64\nF:usr\n\n" +
		"P:.build-deps\nV:20261104.172653\nA:noarch\nT:virtual meta package\n\n" +
		"P:name-with-dashes\nV:1.2.3-r5\n"
	got, err := parseApkInstalledDB(db)
	if err != nil {
		t.Fatal(err)
	}
	wantVersions(t, got, "name-with-dashes", "1.2.3-r4", "1.2.3-r5")
	wantVersions(t, got, ".build-deps", "20261104.172653")
	if len(got) != 2 {
		t.Errorf("got %d names %v, want 2", len(got), got)
	}

	for name, bad := range map[string]string{
		"P: without V:":   "P:lonely\nA:x86_64\n\nP:ok\nV:1-r0\n",
		"V: without P:":   "V:1-r0\n\nP:ok\nV:1-r0\n",
		"P: twice":        "P:a\nP:b\nV:1-r0\n",
		"V: twice":        "P:a\nV:1-r0\nV:2-r0\n",
		"P: last, no V:":  "P:ok\nV:1-r0\n\nP:tail\n",
		"empty P: value":  "P:\nV:1-r0\n",
		"no records":      "\n\n",
		"empty file":      "",
		"other keys only": "C:Q1abc=\nA:x86_64\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := parseApkInstalledDB(bad); err == nil {
				t.Errorf("parsed without error: %v", got)
			}
		})
	}
}

// S-4, parser half: an unparsable line or zero packages is an error, never an
// empty inventory.
func TestInventoryParsersRejectBadInput(t *testing.T) {
	for name, tc := range map[string]struct {
		parse func(string) (map[string][]string, error)
		in    string
	}{
		"rpm no tab":          {parseRpmInventory, "bash 5.1\n"},
		"rpm empty version":   {parseRpmInventory, "bash\t\n"},
		"rpm empty":           {parseRpmInventory, ""},
		"dpkg two fields":     {parseDpkgInventory, "bash\tinstall ok installed\n"},
		"dpkg no version":     {parseDpkgInventory, "bash\tinstall ok installed\t\n"},
		"dpkg none installed": {parseDpkgInventory, "x\tdeinstall ok config-files\t1\n"},
		"pacman one field":    {parsePacmanInventory, "bash\n"},
		"pacman three":        {parsePacmanInventory, "bash 5.3 extra\n"},
		"pacman empty":        {parsePacmanInventory, "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := tc.parse(tc.in); err == nil {
				t.Errorf("parsed without error: %v", got)
			}
		})
	}
	if _, err := parseRpmInventory(""); !errors.Is(err, ErrPackageInventoryEmpty) {
		t.Errorf("empty rpm output: got %v, want ErrPackageInventoryEmpty", err)
	}
}

func TestApkInventoryMissingDB(t *testing.T) {
	inv := &apkInventory{path: filepath.Join(t.TempDir(), "installed")}
	got, err := inv.Packages()
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, %v; want a not-exist error", got, err)
	}
}

// S-5: only Linux enumerates, whatever --package says; an explicit manager
// beats detection; nothing detected is ErrNullPackage, never an rpm default.
func TestResolvePackageInventory(t *testing.T) {
	detect := func(m string) func() string { return func() string { return m } }
	for _, tc := range []struct {
		name, explicit, detected, goos, want string
		err                                  error
	}{
		{"windows", "", "", "windows", "", ErrPackageInventoryUnsupported},
		{"windows with --package rpm", "rpm", "rpm", "windows", "", ErrPackageInventoryUnsupported},
		{"darwin", "", "", "darwin", "", ErrPackageInventoryUnsupported},
		{"darwin with --package dpkg", "dpkg", "", "darwin", "", ErrPackageInventoryUnsupported},
		{"freebsd", "", "rpm", "freebsd", "", ErrPackageInventoryUnsupported},
		{"explicit beats detect", "apk", "dpkg", "linux", "apk", nil},
		{"detected", "", "pacman", "linux", "pacman", nil},
		{"unsupported explicit falls back to detect", "yum", "rpm", "linux", "rpm", nil},
		{"nothing detected", "", "", "linux", "", ErrNullPackage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePackageInventory(tc.explicit, detect(tc.detected), tc.goos)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Errorf("got %q, %v; want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}
}

type countingInventory struct{ calls *atomic.Int32 }

func (countingInventory) Manager() string { return "rpm" }
func (c countingInventory) Packages() (map[string][]string, error) {
	c.calls.Add(1)
	return map[string][]string{"bash": {"1"}}, nil
}

// S-6: one enumeration per System however many times it is asked, and one
// per System: a run that builds two Systems enumerates twice.
//
// REVERT-PROOF: calling c.inner.Packages() directly in cachedInventory.Packages
// makes the first System count 3 and fails this test.
func TestPackageInventoryEnumeratesOncePerSystem(t *testing.T) {
	var calls atomic.Int32
	newSys := func() *System {
		return &System{NewPackageInventory: func(context.Context) (PackageInventory, error) {
			return countingInventory{calls: &calls}, nil
		}}
	}
	first := newSys()
	for range 3 {
		inv, err := first.PackageInventory(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := inv.Packages(); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("one System enumerated %d times, want 1", n)
	}
	inv, err := newSys().PackageInventory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Packages(); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("two Systems enumerated %d times, want 2", n)
	}
}

func TestPackageInventoryNotConfigured(t *testing.T) {
	if _, err := (&System{}).PackageInventory(t.Context()); err == nil {
		t.Fatal("a System with no NewPackageInventory returned an inventory")
	}
}

// New wires resolution through resolvePackageInventory: an explicit manager is
// honoured on Linux, and every other OS refuses.
func TestNewWiresPackageInventory(t *testing.T) {
	inv, err := New("pacman").PackageInventory(t.Context())
	if err != nil {
		if errors.Is(err, ErrPackageInventoryUnsupported) {
			return // not Linux; the table above covers this
		}
		t.Fatal(err)
	}
	if inv.Manager() != "pacman" {
		t.Errorf("manager %q, want pacman", inv.Manager())
	}
}
