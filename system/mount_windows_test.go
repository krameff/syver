//go:build windows
// +build windows

package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/krameff/syver/util"
	"golang.org/x/sys/windows"
)

// FEAT-018: mount: on Windows, driven through DefMount, the path a gossfile
// takes. These replace the FEAT-013 tests that asserted every Windows mount
// check reported "not supported"; that was the honest answer while there was
// no backend.

func windowsTestConfig(t *testing.T) util.Config {
	t.Helper()
	c, err := util.NewConfig()
	if err != nil {
		t.Fatalf("util.NewConfig(): %v", err)
	}
	// A zero Timeout makes getMount time out immediately, which would mask
	// which error is being returned.
	c.Timeout = 5 * time.Second
	return *c
}

// systemDrive is the drive Windows is installed on, which certainly exists and
// is readable, rather than assuming it is C:.
func systemDrive(t *testing.T) string {
	t.Helper()
	dir, err := windows.GetWindowsDirectory()
	if err != nil {
		t.Fatalf("GetWindowsDirectory: %v", err)
	}
	return dir[:2]
}

func TestMountSystemDriveExistsWithFilesystemAndUsage(t *testing.T) {
	drive := systemDrive(t)
	for _, spelling := range []string{drive, drive + `\`} {
		t.Run(spelling, func(t *testing.T) {
			m := NewDefMount(context.Background(), spelling, nil, windowsTestConfig(t))

			exists, err := m.Exists()
			if err != nil || !exists {
				t.Fatalf("Exists() = (%v, %v), want (true, nil)", exists, err)
			}
			fs, err := m.Filesystem()
			if err != nil || fs == "" {
				t.Fatalf("Filesystem() = (%q, %v), want a filesystem name", fs, err)
			}
			usage, err := m.Usage()
			if err != nil || usage < 0 || usage > 100 {
				t.Fatalf("Usage() = (%d, %v), want a percentage", usage, err)
			}
			t.Logf("%s: filesystem=%s usage=%d%%", spelling, fs, usage)
		})
	}
}

// The filesystem name is reported as Windows gives it, not lower-cased, so it
// matches what every Windows tool shows.
func TestMountFilesystemIsReportedAsWindowsNamesIt(t *testing.T) {
	drive := systemDrive(t)
	root, err := windows.UTF16PtrFromString(drive + `\`)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(root, nil, 0, nil, nil, nil, &buf[0], uint32(len(buf))); err != nil {
		t.Fatalf("GetVolumeInformation: %v", err)
	}
	want := windows.UTF16ToString(buf)

	got, err := NewDefMount(context.Background(), drive, nil, windowsTestConfig(t)).Filesystem()
	if err != nil {
		t.Fatalf("Filesystem() error = %v", err)
	}
	if got != want {
		t.Errorf("Filesystem() = %q, want %q exactly as Windows reports it", got, want)
	}
}

// An unused letter is a lookup that ran and found nothing.
func TestMountUnusedDriveLetterIsNotFound(t *testing.T) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		t.Fatalf("GetLogicalDrives: %v", err)
	}
	unused := ""
	for i := 25; i >= 0; i-- {
		if mask&(1<<uint(i)) == 0 {
			unused = string(rune('A'+i)) + ":"
			break
		}
	}
	if unused == "" {
		t.Skip("every drive letter is in use")
	}

	_, err = NewDefMount(context.Background(), unused, nil, windowsTestConfig(t)).Exists()
	if !errors.Is(err, ErrMountpointNotFound) {
		t.Fatalf("Exists() on unused %s = %v, want ErrMountpointNotFound", unused, err)
	}
}

// A folder path is outside what mount: covers on Windows, and must say so
// rather than read as a missing mountpoint.
func TestMountFolderPathIsUnsupportedNotNotFound(t *testing.T) {
	for _, path := range []string{systemDrive(t) + `\Windows`, `/`} {
		_, err := NewDefMount(context.Background(), path, nil, windowsTestConfig(t)).Exists()
		if errors.Is(err, ErrMountpointNotFound) {
			t.Errorf("Exists() on %q = %v; it blames the path for a scope limit", path, err)
		}
		if !errors.Is(err, ErrMountUnsupported) {
			t.Errorf("Exists() on %q = %v, want ErrMountUnsupported", path, err)
		}
	}
}

// opts, vfs-opts and source have no Windows meaning. An empty value with a nil
// error would let `opts: []` pass for the wrong reason.
func TestMountAttributesWithoutWindowsMeaningError(t *testing.T) {
	m := NewDefMount(context.Background(), systemDrive(t), nil, windowsTestConfig(t))

	if _, err := m.Opts(); !errors.Is(err, ErrMountAttributeUnsupported) {
		t.Errorf("Opts() = %v, want ErrMountAttributeUnsupported", err)
	}
	if _, err := m.VfsOpts(); !errors.Is(err, ErrMountAttributeUnsupported) {
		t.Errorf("VfsOpts() = %v, want ErrMountAttributeUnsupported", err)
	}
	if _, err := m.Source(); !errors.Is(err, ErrMountAttributeUnsupported) {
		t.Errorf("Source() = %v, want ErrMountAttributeUnsupported", err)
	}
}
