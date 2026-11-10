package system

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// rpmInventoryFormat prints one "name<TAB>version" line per installed package,
// with the version in the same [E:]V-R form `package:` reads per name.
const rpmInventoryFormat = "%{NAME}\t%|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\n"

type rpmInventory struct{ ctx context.Context }

func (*rpmInventory) Manager() string { return "rpm" }

func (i *rpmInventory) Packages() (map[string][]string, error) {
	cmd, err := runInventoryCommand(i.ctx, "rpm", "-qa", "--nosignature", "--nohdrchk", "--nodigest", "--qf", rpmInventoryFormat)
	if err != nil {
		return nil, err
	}
	return parseRpmInventory(cmd.Stdout.String())
}

// parseRpmInventory reads rpmInventoryFormat output. gpg-pubkey entries, one
// per imported signing key, are kept like any other package.
func parseRpmInventory(out string) (map[string][]string, error) {
	b := inventoryBuilder{}
	for n, line := range splitLines(out) {
		name, version, ok := strings.Cut(line, "\t")
		if !ok || name == "" || version == "" || strings.Contains(version, "\t") {
			return nil, fmt.Errorf("rpm: unparsable line %d: %q", n+1, line)
		}
		b.add(name, version)
	}
	return b.finish("rpm")
}

type dpkgInventory struct{ ctx context.Context }

func (*dpkgInventory) Manager() string { return "dpkg" }

func (i *dpkgInventory) Packages() (map[string][]string, error) {
	cmd, err := runInventoryCommand(i.ctx, "dpkg-query", "-W", "-f", "${Package}\t${Status}\t${Version}\n")
	if err != nil {
		return nil, err
	}
	return parseDpkgInventory(cmd.Stdout.String())
}

// parseDpkgInventory reads "name<TAB>status<TAB>version" lines and keeps only
// packages that are installed, the same two statuses `package:` accepts. A
// package removed but not purged ("deinstall ok config-files") is listed by
// dpkg-query and dropped here. A multiarch package is listed once per
// architecture under the same name; the builder folds those together.
func parseDpkgInventory(out string) (map[string][]string, error) {
	b := inventoryBuilder{}
	for n, line := range splitLines(out) {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[0] == "" {
			return nil, fmt.Errorf("dpkg-query: unparsable line %d: %q", n+1, line)
		}
		if fields[1] != "install ok installed" && fields[1] != "hold ok installed" {
			continue
		}
		if fields[2] == "" {
			return nil, fmt.Errorf("dpkg-query: installed package %q has no version (line %d)", fields[0], n+1)
		}
		b.add(fields[0], fields[2])
	}
	return b.finish("dpkg")
}

// apkInstalledDB is apk's record of installed packages. It is read directly:
// `apk info -v` prints name and version joined by "-", which cannot be split
// reliably, and a virtual package (apk add --virtual) has a version with no
// -rN suffix at all.
const apkInstalledDB = "/lib/apk/db/installed"

type apkInventory struct{ path string }

func (*apkInventory) Manager() string { return "apk" }

func (i *apkInventory) Packages() (map[string][]string, error) {
	data, err := os.ReadFile(i.path)
	if err != nil {
		return nil, fmt.Errorf("apk: %w", err)
	}
	return parseApkInstalledDB(string(data))
}

// parseApkInstalledDB reads apk's installed database: blank-line separated
// records of "X:value" lines, where P: is the package name and V: its
// version. Every other field is ignored. A record with one of P: and V: but
// not the other, or with either twice, is an error.
func parseApkInstalledDB(data string) (map[string][]string, error) {
	b := inventoryBuilder{}
	var name, version string
	var hasName, hasVersion bool
	start := 0
	flush := func() error {
		defer func() { name, version, hasName, hasVersion = "", "", false, false }()
		switch {
		case !hasName && !hasVersion:
			return nil
		case !hasVersion:
			return fmt.Errorf("apk: record at line %d has P:%s but no V:", start, name)
		case !hasName:
			return fmt.Errorf("apk: record at line %d has V:%s but no P:", start, version)
		}
		b.add(name, version)
		return nil
	}
	for n, line := range strings.Split(data, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if !hasName && !hasVersion {
			start = n + 1
		}
		switch {
		case strings.HasPrefix(line, "P:"):
			if hasName {
				return nil, fmt.Errorf("apk: record at line %d has more than one P: line", start)
			}
			name, hasName = line[2:], true
			if name == "" {
				return nil, fmt.Errorf("apk: empty P: at line %d", n+1)
			}
		case strings.HasPrefix(line, "V:"):
			if hasVersion {
				return nil, fmt.Errorf("apk: record at line %d has more than one V: line", start)
			}
			version, hasVersion = line[2:], true
			if version == "" {
				return nil, fmt.Errorf("apk: empty V: at line %d", n+1)
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return b.finish("apk")
}

type pacmanInventory struct{ ctx context.Context }

func (*pacmanInventory) Manager() string { return "pacman" }

func (i *pacmanInventory) Packages() (map[string][]string, error) {
	cmd, err := runInventoryCommand(i.ctx, "pacman", "-Q", "--color", "never")
	if err != nil {
		return nil, err
	}
	return parsePacmanInventory(cmd.Stdout.String())
}

// parsePacmanInventory reads "name version" lines.
func parsePacmanInventory(out string) (map[string][]string, error) {
	b := inventoryBuilder{}
	for n, line := range splitLines(out) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("pacman: unparsable line %d: %q", n+1, line)
		}
		b.add(fields[0], fields[1])
	}
	return b.finish("pacman")
}
