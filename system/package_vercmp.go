package system

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PackageVersionComparer orders two versions the way the host's package
// manager does.
type PackageVersionComparer interface {
	// Compare returns -1, 0 or 1 as a is older than, equal to or newer than b.
	Compare(a, b string) (int, error)
}

// versionArgShape is every character a version from rpm, dpkg, apk or pacman
// can contain. Anything else is refused before a helper runs, so a version
// can never be read as a flag or break out of the rpm Lua string below.
var versionArgShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+~^:_-]*$`)

// NewPackageVersionComparer returns the comparer for a package manager. Each
// one asks the host's own tool, so ordering matches the package manager
// exactly: dpkg --compare-versions, apk version -t, pacman's vercmp, and for
// rpm, rpm.vercmp through rpm's embedded Lua.
func NewPackageVersionComparer(ctx context.Context, manager string) (PackageVersionComparer, error) {
	if !IsSupportedPackageManager(manager) {
		return nil, fmt.Errorf("no version comparison for package manager %q", manager)
	}
	return &toolComparer{ctx: ctx, manager: manager}, nil
}

type toolComparer struct {
	ctx     context.Context
	manager string
}

func (c *toolComparer) Compare(a, b string) (int, error) {
	// Byte-equal versions are equal under every package manager, so the
	// common case of an unchanged package runs no subprocess.
	if a == b {
		return 0, nil
	}
	for _, v := range []string{a, b} {
		if !versionArgShape.MatchString(v) {
			return 0, fmt.Errorf("%s: %q is not a version", c.manager, v)
		}
	}
	switch c.manager {
	case "rpm":
		return c.rpm(a, b)
	case "dpkg":
		return c.dpkg(a, b)
	case "apk":
		return c.apk(a, b)
	default:
		return c.pacman(a, b)
	}
}

func (c *toolComparer) rpm(a, b string) (int, error) {
	cmd, err := runInventoryCommand(c.ctx, "rpm", "--eval", fmt.Sprintf(`%%{lua: print(rpm.vercmp("%s", "%s"))}`, a, b))
	if err != nil {
		return 0, err
	}
	return parseSignedResult("rpm", cmd.Stdout.String())
}

// dpkg answers a relation with its exit status: 0 true, 1 false, anything
// else an error. Equal is what is left when neither lt nor gt holds.
func (c *toolComparer) dpkg(a, b string) (int, error) {
	for _, rel := range []struct {
		op     string
		result int
	}{{"lt", -1}, {"gt", 1}} {
		cmd, err := runHelperCommand(c.ctx, "dpkg", "--compare-versions", a, rel.op, b)
		if err != nil {
			return 0, fmt.Errorf("dpkg: %w", err)
		}
		if cmd.Err == nil {
			return rel.result, nil
		}
		var exitErr *exec.ExitError
		if !errors.As(cmd.Err, &exitErr) {
			return 0, fmt.Errorf("dpkg: %w", cmd.Err)
		}
		if cmd.Status != 1 {
			return 0, fmt.Errorf("dpkg --compare-versions exited %d: %s", cmd.Status, strings.TrimSpace(cmd.Stderr.String()))
		}
	}
	return 0, nil
}

func (c *toolComparer) apk(a, b string) (int, error) {
	cmd, err := runInventoryCommand(c.ctx, "apk", "version", "-t", a, b)
	if err != nil {
		return 0, err
	}
	switch strings.TrimSpace(cmd.Stdout.String()) {
	case "<":
		return -1, nil
	case "=":
		return 0, nil
	case ">":
		return 1, nil
	}
	return 0, fmt.Errorf("apk version -t %s %s: unexpected output %q", a, b, strings.TrimSpace(cmd.Stdout.String()))
}

func (c *toolComparer) pacman(a, b string) (int, error) {
	cmd, err := runInventoryCommand(c.ctx, "vercmp", a, b)
	if err != nil {
		return 0, err
	}
	return parseSignedResult("vercmp", cmd.Stdout.String())
}

// parseSignedResult reads an integer and reduces it to its sign. vercmp is
// documented to print a negative, zero or positive number, not only -1/0/1.
func parseSignedResult(tool, out string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("%s: unexpected comparison output %q", tool, strings.TrimSpace(out))
	}
	switch {
	case n < 0:
		return -1, nil
	case n > 0:
		return 1, nil
	}
	return 0, nil
}
