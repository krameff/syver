//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
)

// fakeSystemctl puts a `systemctl` on PATH that prints stdout, exits with
// status, and records its argv (one argument per line) to the returned file.
// The same pattern as TestHealthyHelperIsUnaffected.
func fakeSystemctl(t *testing.T, stdout string, status int) (argvFile string) {
	t.Helper()
	dir := t.TempDir()
	outFile := filepath.Join(dir, "stdout")
	argvFile = filepath.Join(dir, "argv")
	if err := os.WriteFile(outFile, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\"; done > '" + argvFile + "'\n" +
		"cat '" + outFile + "'\n" +
		"exit " + strconv.Itoa(status) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argvFile
}

// FEAT-019. The three outputs were captured from systemd 255; the fourth puts
// the keys in the opposite order to the one requested, which systemd does in
// practice (it prints in its own order, not the -p order).
func TestServiceSystemdMasked(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"masked unit", "LoadState=masked\nUnitFileState=masked\n", true},
		{"missing unit", "LoadState=not-found\nUnitFileState=\n", false},
		{"enabled template instance", "LoadState=loaded\nUnitFileState=enabled\n", false},
		{"keys in an unexpected order", "UnitFileState=masked\nLoadState=masked\n", true},
		{"runtime mask", "LoadState=loaded\nUnitFileState=masked-runtime\n", true},
		{"disabled unit", "LoadState=loaded\nUnitFileState=disabled\n", false},
	}
	for _, legacy := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("legacy=%v/%s", legacy, tc.name), func(t *testing.T) {
				argv := fakeSystemctl(t, tc.stdout, 0)
				ctor := NewServiceSystemd
				if legacy {
					ctor = NewServiceSystemdLegacy
				}
				got, err := ctor(context.Background(), "alsa-utils", nil, util.Config{}).Masked()
				if err != nil {
					t.Fatalf("Masked() error = %v, want nil", err)
				}
				if got != tc.want {
					t.Errorf("Masked() = %v, want %v for output %q", got, tc.want, tc.stdout)
				}
				// Repeated -p, not a comma list: the comma form's minimum systemd
				// version is unverified and the legacy backend serves old hosts.
				b, err := os.ReadFile(argv)
				if err != nil {
					t.Fatal(err)
				}
				wantArgv := "show\n-p\nLoadState\n-p\nUnitFileState\n--\nalsa-utils\n"
				if string(b) != wantArgv {
					t.Errorf("systemctl argv = %q, want %q", b, wantArgv)
				}
			})
		}
	}
}

// `systemctl show` exits 0 for a unit that does not exist, so a non-zero exit
// means the query itself failed. That must not read as `masked: false`.
func TestServiceSystemdMaskedQueryFailureIsAnError(t *testing.T) {
	t.Run("non-zero exit", func(t *testing.T) {
		// Plausible output on stdout, so only the exit status can catch it.
		fakeSystemctl(t, "LoadState=loaded\nUnitFileState=disabled\n", 1)
		_, err := NewServiceSystemd(context.Background(), "alsa-utils", nil, util.Config{}).Masked()
		if err == nil {
			t.Fatal("Masked() error = nil for a failed systemctl show, want an error")
		}
	})
	t.Run("no LoadState in the output", func(t *testing.T) {
		fakeSystemctl(t, "Something=else\n", 0)
		_, err := NewServiceSystemd(context.Background(), "alsa-utils", nil, util.Config{}).Masked()
		if err == nil {
			t.Fatal("Masked() error = nil for output with no LoadState, want an error")
		}
	})
}

// FEAT-019 / P5 deferred item: Exists matched `<name>.service` anywhere in the
// list-unit-files output, so `cron` matched `anacron.service`.
func TestServiceSystemdExistsMatchesTheUnitNameExactly(t *testing.T) {
	listing := "anacron.service                            enabled         enabled\n" +
		"getty@.service                             enabled         enabled\n" +
		"ssh.service                                enabled         enabled\n"
	cases := []struct {
		name string
		want bool
	}{
		{"cron", false},
		{"anacron", true},
		{"ssh", true},
		{"sh", false},
		{"getty@", true},
		{"getty", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeSystemctl(t, listing, 0)
			got, err := NewServiceSystemd(context.Background(), tc.name, nil, util.Config{}).Exists()
			if err != nil {
				t.Fatalf("Exists(%q) error = %v", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("Exists(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// Every backend with no mask concept must fail `masked:` loudly rather than
// answer false. Windows is covered in service_windows_test.go; darwin resolves
// to ServiceInit through detectService's default.
func TestNonSystemdBackendsRejectMasked(t *testing.T) {
	ctx := context.Background()
	backends := map[string]Service{
		"init":        NewServiceInit(ctx, "sshd", nil, util.Config{}),
		"alpine init": NewAlpineServiceInit(ctx, "sshd", nil, util.Config{}),
		"upstart":     NewServiceUpstart(ctx, "sshd", nil, util.Config{}),
	}
	for name, svc := range backends {
		t.Run(name, func(t *testing.T) {
			got, err := svc.Masked()
			if !errors.Is(err, ErrServiceMaskedUnsupported) {
				t.Fatalf("Masked() error = %v, want ErrServiceMaskedUnsupported", err)
			}
			if got {
				t.Error("Masked() = true alongside the sentinel")
			}
		})
	}
	// The shared sentinel is seen on sysv, Alpine, upstart and darwin, so it must
	// not carry the Windows hint; ServiceWindows.Masked adds that itself.
	if strings.Contains(ErrServiceMaskedUnsupported.Error(), "start-type") {
		t.Errorf("shared sentinel text %q carries the Windows-only hint", ErrServiceMaskedUnsupported)
	}
}

// A unit name beginning with "-" must reach systemctl as a name, not be parsed
// as an option, so every call that passes the name puts "--" before it.
func TestServiceSystemdPassesTheNameAfterDoubleDash(t *testing.T) {
	const name = "-H"
	calls := map[string]struct {
		run  func(Service) error
		argv string
	}{
		"is-enabled": {func(s Service) error { _, err := s.Enabled(); return err }, "-q\nis-enabled\n--\n-H\n"},
		"is-active":  {func(s Service) error { _, err := s.Running(); return err }, "-q\nis-active\n--\n-H\n"},
		"show":       {func(s Service) error { _, err := s.Masked(); return err }, "show\n-p\nLoadState\n-p\nUnitFileState\n--\n-H\n"},
	}
	for sub, c := range calls {
		t.Run(sub, func(t *testing.T) {
			argv := fakeSystemctl(t, "LoadState=not-found\n", 0)
			if err := c.run(NewServiceSystemd(context.Background(), name, nil, util.Config{})); err != nil {
				t.Fatalf("%s error = %v", sub, err)
			}
			b, err := os.ReadFile(argv)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != c.argv {
				t.Errorf("systemctl argv = %q, want %q", b, c.argv)
			}
		})
	}
}
