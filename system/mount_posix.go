//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"fmt"
	"math"
	"syscall"
	"time"

	"github.com/moby/sys/mountinfo"
)

// mountAttributeSupported reports that every mount attribute has a meaning
// here. See ErrMountAttributeUnsupported in mount.go.
func mountAttributeSupported(string) error { return nil }

// getMount looks the mountpoint up in the mount table. It lived in mount.go,
// shared by every OS, until FEAT-018 gave Windows its own lookup; it moved here
// unchanged, so Linux and macOS behaviour is the same by construction.

func getUsage(mountpoint string) (int, error) {
	statfsOut := &syscall.Statfs_t{}
	err := syscall.Statfs(mountpoint, statfsOut)
	if err != nil {
		return -1, err
	}

	percentageFree := float64(statfsOut.Bfree) / float64(statfsOut.Blocks)
	usage := math.Round((1 - percentageFree) * 100)

	return int(usage), nil
}

func getMount(mountpoint string, timeout int) (*mountinfo.Info, error) {
	c1 := make(chan *mountinfo.Info, 1)
	e1 := make(chan error, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond

	go func() {
		entries, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter(mountpoint))
		if err != nil {
			e1 <- err
			return
		}
		if len(entries) == 0 {
			e1 <- ErrMountpointNotFound
			return
		}
		c1 <- entries[0]
	}()

	select {
	case result := <-c1:
		return result, nil
	case err := <-e1:
		return nil, err
	case <-time.After(timeoutD):
		return nil, fmt.Errorf("getMount operation timed out after %s milliseconds", timeoutD)
	}

}
