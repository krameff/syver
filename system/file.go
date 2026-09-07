package system

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/krameff/syver/util"
)

// ErrFileOwnershipUnsupported is returned by Mode, Owner, Uid, Group and Gid
// on Windows (system/file_windows.go) -- there is no POSIX mode/owner/group
// concept to report there. Following the sentinel-error idiom already used by
// registry (ErrRegistryUnsupported) and package (ErrNullPackage): a named,
// comparable error rather than a fabricated zero value with a nil error.
var ErrFileOwnershipUnsupported = errors.New("file mode/owner/group is not supported on this platform")

type File interface {
	Path() string
	Exists() (bool, error)
	Contents() (io.Reader, error)
	Mode() (string, error)
	Size() (int, error)
	Filetype() (string, error)
	Owner() (string, error)
	Uid() (int, error)
	Group() (string, error)
	Gid() (int, error)
	LinkedTo() (string, error)
	Md5() (string, error)
	Sha256() (string, error)
	Sha512() (string, error)
}

type hashFuncType string

const (
	md5Hash    hashFuncType = "md5"
	sha256Hash hashFuncType = "sha256"
	sha512Hash hashFuncType = "sha512"
)

type DefFile struct {
	// ctx bounds and cancels the `getent` fallbacks in Owner and Group. Those
	// run only when the local passwd/group lookup misses, which on a host using
	// a remote nsswitch backend is exactly the case that can block.
	ctx      context.Context
	path     string
	realPath string
	loaded   bool
	err      error
}

func NewDefFile(ctx context.Context, path string, system *System, config util.Config) File {
	var err error
	if !strings.HasPrefix(path, "~") {
		path, err = filepath.Abs(path)
	}
	return &DefFile{ctx: ctx, path: path, err: err}
}

func (f *DefFile) setup() error {
	if f.loaded || f.err != nil {
		return f.err
	}
	f.loaded = true
	if f.realPath, f.err = realPath(f.path); f.err != nil {
		return f.err
	}

	return f.err
}

func (f *DefFile) Path() string {
	return f.path
}

func (f *DefFile) Exists() (bool, error) {
	if err := f.setup(); err != nil {
		return false, err
	}

	_, err := os.Lstat(f.realPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func (f *DefFile) Contents() (io.Reader, error) {
	if err := f.setup(); err != nil {
		return nil, err
	}

	fh, err := os.Open(f.realPath)
	if err != nil {
		return nil, err
	}
	return fh, nil
}

func (f *DefFile) Size() (int, error) {
	if err := f.setup(); err != nil {
		return 0, err
	}

	fi, err := os.Lstat(f.realPath)
	if err != nil {
		return 0, err
	}

	size := fi.Size()
	return int(size), nil
}

func (f *DefFile) Filetype() (string, error) {
	if err := f.setup(); err != nil {
		return "", err
	}

	fi, err := os.Lstat(f.realPath)
	if err != nil {
		return "", err
	}

	switch {
	case fi.Mode()&os.ModeSymlink == os.ModeSymlink:
		return "symlink", nil
	case fi.Mode()&os.ModeDevice == os.ModeDevice:
		if fi.Mode()&os.ModeCharDevice == os.ModeCharDevice {
			return "character-device", nil
		}
		return "block-device", nil
	case fi.Mode()&os.ModeNamedPipe == os.ModeNamedPipe:
		return "pipe", nil
	case fi.Mode()&os.ModeSocket == os.ModeSocket:
		return "socket", nil
	case fi.IsDir():
		return "directory", nil
	case fi.Mode().IsRegular():
		return "file", nil
	}
	// FIXME: file as a catchall?
	return "file", nil
}

func (f *DefFile) LinkedTo() (string, error) {
	if err := f.setup(); err != nil {
		return "", err
	}

	dst, err := os.Readlink(f.realPath)
	if err != nil {
		return "", err
	}
	return dst, nil
}

func realPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	pathS := strings.Split(path, "/")
	f := pathS[0]

	var usr *user.User
	var err error
	if f == "~" {
		usr, err = user.Current()
	} else {
		usr, err = user.Lookup(f[1:])
	}
	if err != nil {
		return "", err
	}
	pathS[0] = usr.HomeDir

	realPath := strings.Join(pathS, "/")
	realPath, err = filepath.Abs(realPath)

	return realPath, err
}

func (f *DefFile) hash(hashFunc hashFuncType) (string, error) {

	if err := f.setup(); err != nil {
		return "", err
	}

	fh, err := os.Open(f.realPath)
	if err != nil {
		return "", err
	}
	defer fh.Close()

	var hash hash.Hash

	switch hashFunc {
	case md5Hash:
		hash = md5.New()
	case sha256Hash:
		hash = sha256.New()
	case sha512Hash:
		hash = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash function %s", hashFunc)
	}

	if _, err := io.Copy(hash, fh); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (f *DefFile) Md5() (string, error) {
	return f.hash(md5Hash)
}

func (f *DefFile) Sha256() (string, error) {
	return f.hash(sha256Hash)
}

func (f *DefFile) Sha512() (string, error) {
	return f.hash(sha512Hash)
}

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
