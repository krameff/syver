//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"math"
	"syscall"
)

// mountSupported reports that this platform has a working mount lookup. It is
// a no-op by design: the whole point of the check is that it changes nothing
// where mount: is implemented. See ErrMountUnsupported in mount.go.
func mountSupported() error { return nil }

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
