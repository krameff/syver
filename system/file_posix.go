//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

func (f *DefFile) Mode() (string, error) {
	mode, err := f.getFileInfo(func(fi os.FileInfo) string {
		stat := fi.Sys().(*syscall.Stat_t)
		return fmt.Sprintf("%04o", (stat.Mode & 07777))
	})
	if err != nil {
		return "", err
	}

	return mode, nil
}

// Acl and AclSid report the Windows DACL and have no POSIX implementation. See
// ErrFileAclUnsupported in file.go for why this is an error and not an empty
// list. A POSIX ACL backend would be new scope, not a gap in FEAT-016.
func (f *DefFile) Acl() ([]string, error) {
	return nil, ErrFileAclUnsupported
}

func (f *DefFile) AclSid() ([]string, error) {
	return nil, ErrFileAclUnsupported
}

func (f *DefFile) Owner() (string, error) {
	uidS, err := f.getFileInfo(func(fi os.FileInfo) string {
		return fmt.Sprint(fi.Sys().(*syscall.Stat_t).Uid)
	})
	if err != nil {
		return "", err
	}

	uid, err := strconv.Atoi(uidS)
	if err != nil {
		return "", err
	}
	return getUserForUid(f.ctx, uid)
}

func (f *DefFile) Uid() (int, error) {
	uidS, err := f.getFileInfo(func(fi os.FileInfo) string {
		return fmt.Sprint(fi.Sys().(*syscall.Stat_t).Uid)
	})
	if err != nil {
		return -1, err
	}

	uid, err := strconv.Atoi(uidS)
	if err != nil {
		return -1, err
	}
	return uid, nil
}

func (f *DefFile) Group() (string, error) {
	gidS, err := f.getFileInfo(func(fi os.FileInfo) string {
		return fmt.Sprint(fi.Sys().(*syscall.Stat_t).Gid)
	})
	if err != nil {
		return "", err
	}

	gid, err := strconv.Atoi(gidS)
	if err != nil {
		return "", err
	}
	return getGroupForGid(f.ctx, gid)
}

func (f *DefFile) Gid() (int, error) {
	gidS, err := f.getFileInfo(func(fi os.FileInfo) string {
		return fmt.Sprint(fi.Sys().(*syscall.Stat_t).Gid)
	})
	if err != nil {
		return -1, err
	}

	gid, err := strconv.Atoi(gidS)
	if err != nil {
		return -1, err
	}
	return gid, nil
}

func (f *DefFile) getFileInfo(selectorFunc func(os.FileInfo) string) (string, error) {
	if err := f.setup(); err != nil {
		return "", err
	}

	fi, err := os.Lstat(f.realPath)
	if err != nil {
		return "", err
	}
	return selectorFunc(fi), nil
}

// getUserForUid and getGroupForGid MOVED HERE from file.go on 2026-09-21.
//
// They resolve a numeric uid/gid to a name and are called only from Owner and
// Group below, plus helper_command_test.go -- all POSIX-tagged. In the untagged
// file.go they compiled into the Windows build and were unreachable there, which
// `GOOS=windows golangci-lint run` reports as two `unused` findings. Nothing had
// ever run lint cross-platform, so nobody saw them; adding the `lint-cross` target
// to the gate made them a wall rather than a curiosity.
//
// Windows resolves ownership through the security descriptor (file_acl_windows.go)
// and has no use for either function: an account there is a SID, not an integer.
func getUserForUid(ctx context.Context, uid int) (string, error) {
	if user, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		return user.Username, nil
	}

	cmd, ctxErr := runHelperCommand(ctx, "getent", "passwd", strconv.Itoa(uid))
	if ctxErr != nil {
		return "", ctxErr
	}
	if cmd.Err != nil {
		return "", fmt.Errorf("no matching entries in passwd file. getent passwd: %w", cmd.Err)
	}
	userS := strings.Split(cmd.Stdout.String(), ":")[0]

	return userS, nil
}

func getGroupForGid(ctx context.Context, gid int) (string, error) {
	if group, err := user.LookupGroupId(strconv.Itoa(gid)); err == nil {
		return group.Name, nil
	}

	cmd, ctxErr := runHelperCommand(ctx, "getent", "group", strconv.Itoa(gid))
	if ctxErr != nil {
		return "", ctxErr
	}
	if cmd.Err != nil {
		return "", fmt.Errorf("no matching entries in group file. getent group: %w", cmd.Err)
	}
	groupS := strings.Split(cmd.Stdout.String(), ":")[0]

	return groupS, nil
}
