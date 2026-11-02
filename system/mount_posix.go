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

	return usagePercent(uint64(statfsOut.Bfree), uint64(statfsOut.Blocks)), nil
}

// usagePercent is the percentage of blocks in use, rounded. Pseudo
// filesystems such as /proc and sysfs report zero blocks; dividing by that
// gives NaN, which converts to an arbitrary integer (math.MinInt64 on amd64)
// and lets any `lt:` check pass. They hold no data, so their usage is 0.
func usagePercent(free, total uint64) int {
	if total == 0 {
		return 0
	}
	percentageFree := float64(free) / float64(total)
	return int(math.Round((1 - percentageFree) * 100))
}

func getMount(mountpoint string, timeout int) (*mountinfo.Info, error) {
	c1 := make(chan *mountinfo.Info, 1)
	e1 := make(chan error, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond

	go func() {
		// Nothing above this goroutine recovers, so a panic here would end
		// the process rather than fail the one check. The channels are
		// buffered and nothing has been sent if we got here.
		defer func() {
			if rec := recover(); rec != nil {
				e1 <- fmt.Errorf("getMount %s failed: panic: %v", mountpoint, rec)
			}
		}()
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
		return nil, fmt.Errorf("getMount operation timed out after %s", timeoutD)
	}

}
