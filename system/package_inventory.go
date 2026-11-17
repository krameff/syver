package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/krameff/syver/util"
)

// PackageInventory lists every package installed on the host.
type PackageInventory interface {
	// Manager is the backend in use, one of SupportedPackageManagers().
	Manager() string
	// Packages maps each installed package name to its sorted, de-duplicated
	// versions. It never returns an empty map with a nil error: a host with
	// no packages, or one whose package database could not be read, is an
	// error. The map is shared by every caller on one System; do not modify it.
	Packages() (map[string][]string, error)
}

var (
	// ErrPackageInventoryUnsupported is returned on any OS but Linux, whatever
	// --package says. There is no package manager there to enumerate.
	ErrPackageInventoryUnsupported = errors.New("package inventory is supported on Linux only (rpm, dpkg, apk, pacman)")
	// ErrPackageInventoryEmpty is returned when a package manager reports no
	// installed packages at all, which on a real host means it was not asked
	// the right question rather than that nothing is installed.
	ErrPackageInventoryEmpty = errors.New("package manager reported no installed packages")
	// errPackageInventoryNotConfigured is returned by a System built without
	// a NewPackageInventory function, such as a struct literal in a test.
	errPackageInventoryNotConfigured = errors.New("package inventory is not configured on this system")
)

// resolvePackageInventory picks the package manager to enumerate. It is pure,
// like resolveNewPackage, so the non-Linux branch is table-testable anywhere.
//
// Unlike resolveNewPackage it has no rpm default: when nothing is detected the
// answer is ErrNullPackage, because enumerating the wrong backend would report
// every recorded package as missing. And it refuses every OS but Linux before
// looking at explicit, so --package rpm on macOS is still an error.
func resolvePackageInventory(explicit string, detect func() string, goos string) (string, error) {
	if goos != "linux" {
		return "", ErrPackageInventoryUnsupported
	}
	if IsSupportedPackageManager(explicit) {
		return explicit, nil
	}
	if m := detect(); IsSupportedPackageManager(m) {
		return m, nil
	}
	return "", ErrNullPackage
}

// newPackageInventory returns the enumerator for a resolved manager.
func newPackageInventory(ctx context.Context, manager string) PackageInventory {
	switch manager {
	case "rpm":
		return &rpmInventory{ctx: ctx}
	case "dpkg":
		return &dpkgInventory{ctx: ctx}
	case "apk":
		return &apkInventory{path: apkInstalledDB}
	case "pacman":
		return &pacmanInventory{ctx: ctx}
	}
	// Unreachable through resolvePackageInventory, which only returns the four
	// supported names.
	return nil
}

// PackageInventory resolves this host's package inventory. Resolution is cheap
// and runs on every call; enumeration, through the returned value's Packages,
// runs at most once per System however many callers ask.
//
// Each System enumerates separately: a run with a discovery: block builds one
// System for discovery and another for validation, and each enumerates once.
func (s *System) PackageInventory(ctx context.Context) (PackageInventory, error) {
	if s.NewPackageInventory == nil {
		return nil, errPackageInventoryNotConfigured
	}
	inv, err := s.NewPackageInventory(ctx)
	if err != nil {
		return nil, err
	}
	return &cachedInventory{sys: s, inner: inv}, nil
}

// cachedInventory shares one enumeration across every caller on a System.
type cachedInventory struct {
	sys   *System
	inner PackageInventory
}

func (c *cachedInventory) Manager() string { return c.inner.Manager() }

func (c *cachedInventory) Packages() (map[string][]string, error) {
	c.sys.inventoryOnce.Do(func() {
		c.sys.inventory, c.sys.inventoryErr = c.inner.Packages()
	})
	return c.sys.inventory, c.sys.inventoryErr
}

// runInventoryCommand runs a package-manager helper and turns every way it can
// fail into an error: a cancelled or expired context, a missing binary, and a
// non-zero exit, the last with its trimmed stderr. The per-name package
// backends fold a missing binary and a non-zero exit into "not installed" on
// purpose; an enumeration must not, because an empty answer would read as a
// host with nothing installed.
func runInventoryCommand(ctx context.Context, name string, arg ...string) (*util.Command, error) {
	cmd, ctxErr := runHelperCommand(ctx, name, arg...)
	if ctxErr != nil {
		return cmd, fmt.Errorf("%s: %w", name, ctxErr)
	}
	if cmd.Err != nil {
		var exitErr *exec.ExitError
		if errors.As(cmd.Err, &exitErr) {
			if msg := strings.TrimSpace(cmd.Stderr.String()); msg != "" {
				return cmd, fmt.Errorf("%s exited %d: %s", name, cmd.Status, msg)
			}
			return cmd, fmt.Errorf("%s exited %d", name, cmd.Status)
		}
		return cmd, fmt.Errorf("%s: %w", name, cmd.Err)
	}
	return cmd, nil
}

// inventoryBuilder collects name/version pairs into the sorted, de-duplicated
// shape Packages returns.
type inventoryBuilder map[string][]string

func (b inventoryBuilder) add(name, version string) {
	if !slices.Contains(b[name], version) {
		b[name] = append(b[name], version)
	}
}

func (b inventoryBuilder) finish(manager string) (map[string][]string, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%s: %w", manager, ErrPackageInventoryEmpty)
	}
	for _, versions := range b {
		slices.Sort(versions)
	}
	return b, nil
}

// splitLines splits command output into lines, dropping only the final empty
// line a trailing newline leaves.
func splitLines(out string) []string {
	out = strings.TrimSuffix(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
