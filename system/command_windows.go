//go:build windows
// +build windows

package system

import (
	"context"

	"github.com/krameff/syver/util"
)

const windowsShell string = "cmd"

func commandWrapper(ctx context.Context, cmd string) *util.Command {
	return util.NewCommandForWindowsCmdContext(ctx, windowsShell, "/c", cmd)
}
