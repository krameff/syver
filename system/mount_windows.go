//go:build windows
// +build windows

package system

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/moby/sys/mountinfo"
	"golang.org/x/sys/windows"
)

// Windows has no mount table in the POSIX sense, and the vendored mountinfo
// returns an empty one there rather than an error, which is what once made
// every Windows mount check report "mountpoint not found". FEAT-018 gives it a
// backend over the Win32 volume calls instead, covering drive letters only:
//
//   - exists: the letter is in GetLogicalDriveStrings
//   - filesystem: GetVolumeInformation, reported as Windows names it (NTFS)
//   - usage: GetDiskFreeSpaceEx, meaning the same as Bfree/Blocks on POSIX
//   - opts, vfs-opts, source: no Windows meaning; each returns
//     ErrMountAttributeUnsupported rather than an empty value
//
// Mapped network drives belong to a logon session, so a service or a WinRM
// session does not see a user's mapped drives. What is reported is what the
// running session can see.

func mountAttributeSupported(name string) error {
	return fmt.Errorf("%w: %s has no meaning on Windows", ErrMountAttributeUnsupported, name)
}

func getMount(mountpoint string, timeout int) (*mountinfo.Info, error) {
	root, err := windowsDriveRoot(mountpoint)
	if err != nil {
		return nil, err
	}

	type result struct {
		info *mountinfo.Info
		err  error
	}
	c := make(chan result, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond

	// A volume query against a dead network mapping can block, so the calls
	// keep the same timeout as the POSIX lookup.
	go func() {
		info, err := lookupDrive(root)
		c <- result{info, err}
	}()

	select {
	case r := <-c:
		return r.info, r.err
	case <-time.After(timeoutD):
		return nil, fmt.Errorf("getMount operation timed out after %s milliseconds", timeoutD)
	}
}

func lookupDrive(root string) (*mountinfo.Info, error) {
	drives, err := logicalDrives()
	if err != nil {
		return nil, err
	}
	found := false
	for _, d := range drives {
		if strings.EqualFold(d, root) {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrMountpointNotFound
	}

	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return nil, err
	}
	fsName := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumeInformation(rootPtr, nil, 0, nil, nil, nil, &fsName[0], uint32(len(fsName))); err != nil {
		// Present but unreadable, for example a card reader with no card. An
		// error, not an empty filesystem name.
		return nil, fmt.Errorf("mount: %s is present but its volume cannot be read: %w", root, err)
	}
	return &mountinfo.Info{
		Mountpoint: root,
		FSType:     windows.UTF16ToString(fsName),
	}, nil
}

// logicalDrives returns the drive roots GetLogicalDriveStrings reports, such as
// `C:\`, for the calling session.
func logicalDrives() ([]string, error) {
	n, err := windows.GetLogicalDriveStrings(0, nil)
	if err != nil {
		return nil, fmt.Errorf("mount: listing drives: %w", err)
	}
	buf := make([]uint16, n)
	n, err = windows.GetLogicalDriveStrings(uint32(len(buf)), &buf[0])
	if err != nil {
		return nil, fmt.Errorf("mount: listing drives: %w", err)
	}
	var drives []string
	start := 0
	for i := 0; i < int(n); i++ {
		if buf[i] == 0 {
			if i > start {
				drives = append(drives, windows.UTF16ToString(buf[start:i]))
			}
			start = i + 1
		}
	}
	return drives, nil
}

func getUsage(mountpoint string) (int, error) {
	root, err := windowsDriveRoot(mountpoint)
	if err != nil {
		return -1, err
	}
	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return -1, err
	}
	var availableToCaller, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(rootPtr, &availableToCaller, &total, &totalFree); err != nil {
		return -1, fmt.Errorf("mount: reading usage of %s: %w", root, err)
	}
	if total == 0 {
		return -1, fmt.Errorf("mount: %s reports a total size of zero", root)
	}

	// Total free bytes, not the caller's quota-limited figure, so this matches
	// what Bfree/Blocks means in mount_posix.go.
	percentageFree := float64(totalFree) / float64(total)
	usage := math.Round((1 - percentageFree) * 100)

	return int(usage), nil
}
