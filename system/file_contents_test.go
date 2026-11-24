package system

import (
	"context"
	"crypto/fips140"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// Contents used to return the open *os.File as an io.Reader, which nothing on
// the validation path closed: one descriptor leaked per check. Counted through
// /proc, with GC off so a finalizer cannot close them behind the test's back.
func TestFileContentsDoesNotLeakDescriptors(t *testing.T) {
	if _, err := os.Stat("/proc/self/fd"); err != nil {
		t.Skip("needs /proc/self/fd")
	}
	p := writeTempFile(t, "line one\nline two\n")
	defer debug.SetGCPercent(debug.SetGCPercent(-1))

	countFDs := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatalf("read /proc/self/fd: %v", err)
		}
		return len(entries)
	}

	before := countFDs()
	const checks = 50
	for range checks {
		r, err := NewDefFile(context.Background(), p, nil, util.Config{}).Contents()
		if err != nil {
			t.Fatalf("Contents: %v", err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(b) != "line one\nline two\n" {
			t.Fatalf("contents = %q", b)
		}
	}
	if leaked := countFDs() - before; leaked > 5 {
		t.Fatalf("%d descriptors still open after %d contents checks", leaked, checks)
	}
}

func TestFileHashes(t *testing.T) {
	// FIPS 180 / RFC 1321 test vectors for "abc".
	p := writeTempFile(t, "abc")
	tests := []struct {
		name string
		hash func(File) (string, error)
		want string
	}{
		{"md5", File.Md5, "900150983cd24fb0d6963f7d28e17f72"},
		{"sha256", File.Sha256, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"sha512", File.Sha512, "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.hash(NewDefFile(context.Background(), p, nil, util.Config{}))
			// Under GODEBUG=fips140=only, md5 is refused with syver's own
			// error (TestFileMd5FIPSOnly covers it); the sha rows are approved
			// algorithms and are checked as usual.
			if tt.name == "md5" && fips140.Enforced() {
				if err == nil || err.Error() != errMd5FIPSOnly.Error() {
					t.Fatalf("FIPS 140-only mode: got %q, %v; want the md5 FIPS error", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
		t.Run(tt.name+"/missing file", func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "absent")
			if _, err := tt.hash(NewDefFile(context.Background(), missing, nil, util.Config{})); err == nil {
				t.Error("want an error for a missing file")
			}
		})
	}
}

// forceFIPSEnforced makes the md5 guard see FIPS 140-only mode for one test.
// Not parallel: it swaps a package variable.
func forceFIPSEnforced(t *testing.T, on bool) {
	t.Helper()
	orig := fipsEnforced
	fipsEnforced = func() bool { return on }
	t.Cleanup(func() { fipsEnforced = orig })
}

// TestFileMd5FIPSOnly: in FIPS 140-only mode md5 returns syver's error, which
// names sha256 as the way out, for a non-empty AND an empty file. The empty
// file matters: without the guard io.Copy never calls Write, so hash.Sum runs
// and panics in that mode. The mode is forced, so this runs in every pass.
// Revert-proof: delete the fipsHashError call in hash(); both md5 subtests
// then get a digest and no error.
func TestFileMd5FIPSOnly(t *testing.T) {
	forceFIPSEnforced(t, true)
	for _, content := range []string{"abc", ""} {
		t.Run(fmt.Sprintf("content %q", content), func(t *testing.T) {
			got, err := NewDefFile(context.Background(), writeTempFile(t, content), nil, util.Config{}).Md5()
			if err == nil {
				t.Fatalf("got digest %q and no error; want the FIPS 140-only md5 error", got)
			}
			if err.Error() != errMd5FIPSOnly.Error() {
				t.Fatalf("error = %q, want %q", err, errMd5FIPSOnly)
			}
			if !strings.Contains(err.Error(), "sha256") {
				t.Errorf("error %q does not name sha256", err)
			}
		})
	}
}

// TestFileShaUnaffectedByFIPSOnly: sha256 and sha512 are approved, so the
// forced mode must not touch them. Revert-proof: make fipsHashError refuse
// every hash function (drop the md5Hash comparison); both subtests fail.
func TestFileShaUnaffectedByFIPSOnly(t *testing.T) {
	forceFIPSEnforced(t, true)
	if fips140.Enforced() {
		// The real mode refuses nothing here either, so the test still means
		// what it says; noted only for whoever reads a -v log.
		t.Log("running under real FIPS 140-only mode")
	}
	p := writeTempFile(t, "abc")
	f := NewDefFile(context.Background(), p, nil, util.Config{})
	for name, tc := range map[string]struct {
		hash func() (string, error)
		want string
	}{
		"sha256": {f.Sha256, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		"sha512": {f.Sha512, "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := tc.hash()
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestFileMd5NotForced: with the mode forced OFF, md5 returns the RFC 1321
// digest of "abc". Skips under real FIPS 140-only mode, where Go itself
// refuses md5 whatever the variable says. Revert-proof: invert the condition
// in fipsHashError (!fipsEnforced()); this test gets the FIPS error.
func TestFileMd5NotForced(t *testing.T) {
	if fips140.Enforced() {
		t.Skip("Go refuses md5 in FIPS 140-only mode; covered by the default-mode run")
	}
	forceFIPSEnforced(t, false)
	got, err := NewDefFile(context.Background(), writeTempFile(t, "abc"), nil, util.Config{}).Md5()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("got %s, want the RFC 1321 vector", got)
	}
}
