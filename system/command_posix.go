//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import "github.com/krameff/syver/util"

const linuxShell string = "sh"

func commandWrapper(cmd string) *util.Command {
	return util.NewCommand(linuxShell, "-c", cmd)
}
