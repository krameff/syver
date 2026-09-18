package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/krameff/syver/util"
)

type KernelParam interface {
	Key() string
	Exists() (bool, error)
	Value() (string, error)
}

type DefKernelParam struct {
	key string
}

func NewDefKernelParam(_ context.Context, key string, system *System, config util.Config) KernelParam {
	return &DefKernelParam{
		key: key,
	}
}

func (k *DefKernelParam) ID() string {
	return k.key
}

func (k *DefKernelParam) Key() string {
	return k.key
}

// ErrKernelParamUnsupported is returned by Exists when the host has no
// /proc/sys filesystem to query at all -- most notably Windows, which has no
// procfs concept, but any other host missing procfs hits the same path.
//
// Value is a single os.ReadFile of the key's path under procSysPath, so a missing
// individual key and a missing /proc/sys tree both surface as the identical
// os.ErrNotExist -- there is no way to distinguish "this key does not exist
// on a real Linux host" from "there is no procfs at all here" from that one
// error alone (system/kernel_param.go's Trap 1 in FEAT-010). Instead of
// inspecting the leaf error, Exists checks whether the base directory
// itself is present: if it is not, nothing was actually learned about the
// specific key, and that must not be reported as false, nil. See SW-5.
var ErrKernelParamUnsupported = errors.New("kernel-param is not supported on this platform (no /proc/sys); on Windows use registry: instead")

// procSysPath is where kernel parameters are read from. A package-level var so
// tests can point it at a fixture tree.
var procSysPath = "/proc/sys/"

// procSysExists reports whether procSysPath exists at all.
// A package-level var (rather than a direct os.Stat call inside Exists) so
// tests can substitute it without needing a host that genuinely lacks
// /proc/sys.
var procSysExists = func() bool {
	_, err := os.Stat(procSysPath)
	return err == nil
}

func (k *DefKernelParam) Exists() (bool, error) {
	if _, err := k.Value(); err != nil {
		if !procSysExists() {
			return false, ErrKernelParamUnsupported
		}
		return false, nil
	}
	return true, nil
}

// Value reads the parameter the way sysctl(8) names it: every dot in the key
// becomes a path separator under procSysPath, and surrounding whitespace is
// trimmed. This is exactly what github.com/lorenzosaino/go-sysctl's Get did,
// which syver used for this one call until it was replaced by these few lines.
func (k *DefKernelParam) Value() (string, error) {
	data, err := os.ReadFile(filepath.Join(procSysPath, strings.ReplaceAll(k.key, ".", "/")))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
