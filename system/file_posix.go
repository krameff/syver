//go:build linux || darwin || !windows
// +build linux darwin !windows

package system

import (
	"fmt"
	"os"
	"strconv"
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
