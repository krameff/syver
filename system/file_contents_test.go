package system

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
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
