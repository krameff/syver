package system

import (
	"errors"
	"fmt"
	"strings"
)

type Registry interface {
	Key() string
	Exists() (bool, error)
	Value() (string, error)
	Type() (string, error)
	// SetView selects the WOW64 registry view this resource reads. It is part
	// of the interface rather than a constructor argument because the view is
	// per-resource spec syntax, while the constructor is shared with every
	// other resource type and takes only global config.
	//
	// An unusable view is recorded and surfaces from Exists, Value and Type
	// rather than being returned here, so that a bad `view:` fails the check
	// that used it instead of disappearing into a constructor nobody checks.
	SetView(view string) error
}

var ErrRegistryUnsupported = errors.New("registry resource is only supported on Windows")

// RegistryView is the WOW64 view a registry read is performed against.
//
// On 64-bit Windows some keys exist twice: 64-bit programs see one copy and
// 32-bit programs, redirected through WOW64, see another under Wow6432Node. A
// spec that does not say which it means gets whichever the running binary
// happens to be, which for syver is always 64-bit. Compliance work needs to be
// able to assert against either.
type RegistryView int

const (
	// RegistryViewNative is the default and MUST behave exactly as the code did
	// before views existed: no view flag is added to the access mask, so the
	// answer is whatever the OS gives a 64-bit process. Any change here is a
	// silent change to every existing gossfile.
	RegistryViewNative RegistryView = iota
	RegistryView32
	RegistryView64
)

// ParseRegistryView turns the spec's `view:` string into a RegistryView.
//
// It lives in this untagged file, not in registry_windows.go, so that its
// tests run on Linux. The grammar is a user-facing contract and does not need
// a Windows host to be wrong.
func ParseRegistryView(view string) (RegistryView, error) {
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "", "native":
		return RegistryViewNative, nil
	case "32":
		return RegistryView32, nil
	case "64":
		return RegistryView64, nil
	default:
		return RegistryViewNative, errors.New(
			`invalid registry view "` + view + `": want 32, 64 or native`)
	}
}

func (v RegistryView) String() string {
	switch v {
	case RegistryView32:
		return "32"
	case RegistryView64:
		return "64"
	default:
		return "native"
	}
}

// registryValueVsKeyWarning is the text of the FEAT-012 W2-5 diagnostic, kept
// in this untagged file so that the parts of it that can be wrong without a
// Windows host are checkable without one.
//
// Three things about this string are load-bearing and none of them are
// reachable from a build-tagged file on Linux:
//
//  1. The "[WARN]" prefix. logs.go filters on that literal through
//     logutils.LevelFilter. Drop it, lowercase it or move it after the
//     "registry:" label and the line stops being level-filtered at all -- it
//     then prints even at --log-level=ERROR, and nothing else in the suite
//     would notice.
//  2. %s, not %q, for the suggested path. %q escapes every backslash, so
//     HKLM\SOFTWARE\... prints as HKLM\\SOFTWARE\\..., which reads as a
//     different path from the one the operator has to type.
//  3. %q, not %s, for the value name. A name that is empty, or that has
//     leading or trailing spaces, is invisible unquoted -- and those are
//     exactly the names that get an author into this diagnostic.
//
// The FIRING CONDITION -- a value miss with a subkey of that name alongside it
// -- needs a real registry and is tested on Windows in
// registry_windows_diag_test.go.
func registryValueVsKeyWarning(key, valueName string) string {
	return fmt.Sprintf(
		"[WARN] registry: %s: no value named %q, but a key of that name exists here; "+
			"a trailing backslash asks about the key: %s",
		key, valueName, key+`\`)
}

// registryPathParts holds the parsed components of a registry key path.
type registryPathParts struct {
	Hive      string
	SubKey    string
	ValueName string
}

// hiveAliases maps every spelling of a hive that syver accepts onto the short
// canonical form. The long names and the PowerShell provider form are here
// because they are what an operator copies: regedit's address bar shows
// HKEY_LOCAL_MACHINE\..., and Get-ItemProperty output shows HKLM:\....
// Requiring the short form meant hand-editing every path pasted from the tools
// the spec is describing.
var hiveAliases = map[string]string{
	"HKLM":                "HKLM",
	"HKEY_LOCAL_MACHINE":  "HKLM",
	"HKCU":                "HKCU",
	"HKEY_CURRENT_USER":   "HKCU",
	"HKCR":                "HKCR",
	"HKEY_CLASSES_ROOT":   "HKCR",
	"HKU":                 "HKU",
	"HKEY_USERS":          "HKU",
	"HKCC":                "HKCC",
	"HKEY_CURRENT_CONFIG": "HKCC",
}

// normaliseHive maps any accepted spelling onto the canonical short form.
//
// The trailing colon of the PowerShell provider form (HKLM:\...) is stripped
// here rather than in the caller so that there is exactly one place that knows
// which spellings exist. lookupHive then only ever sees canonical names, which
// is what removes the second, duplicated switch that used to validate them
// again on the Windows side.
func normaliseHive(raw string) (string, bool) {
	name := strings.ToUpper(strings.TrimSuffix(raw, ":"))
	canonical, ok := hiveAliases[name]
	return canonical, ok
}

// parseRegistryKey splits a full registry path into hive, subkey, and value name.
//
// Accepted hive spellings: the short forms (HKLM), the long forms
// (HKEY_LOCAL_MACHINE), and either with the PowerShell provider colon
// (HKLM:\...). All normalise to the short form.
//
// Two path formats are supported:
//
// Standard format: HIVE\subkey\path\ValueName
// The last backslash-separated segment is the value name.
//
// Explicit format: HIVE\subkey\path::ValueName
// Use "::" to explicitly separate the subkey from the value name. This
// is required when the value name itself contains backslashes (e.g.
// HardenedPaths entries like "\\*\NETLOGON").
//
// # A TRAILING BACKSLASH ADDRESSES THE KEY, NOT A VALUE
//
// `HKLM\A\B\` parses to subkey `A\B` with an empty value name, and that is a
// deliberate and load-bearing distinction from `HKLM\A\B`, which parses to
// subkey `A` and value name `B`. The two ask different questions of the
// registry and get different answers:
//
//	HKLM\...\ProfileList     exists -> false   (is there a VALUE named ProfileList?)
//	HKLM\...\ProfileList\    exists -> true    (is there a KEY named ProfileList?)
//
// Both answers are truthful. Measured on Windows Server 2025, 2026-09-05.
//
// This is NOT made optional or auto-detected, and that decision is not up for
// revisiting on the strength of a confused spec: guessing which the author
// meant moves the ambiguity one level down, where it cannot be seen at all.
// What the Windows implementation does instead is say so when the two are
// confusable -- see the diagnostic in registry_windows.go.
//
// Empty value name therefore means "the key itself" to Exists, and "the key's
// default value" to Value and Type, which is what regedit shows as (Default).
// A key can exist while having no default value, so `exists: true` with a
// value lookup that reports not-found is a coherent pair, not a contradiction.
func parseRegistryKey(key string) (registryPathParts, error) {
	if key == "" {
		return registryPathParts{}, errors.New("empty registry key")
	}

	parts := strings.SplitN(key, `\`, 2)
	if len(parts) < 2 {
		return registryPathParts{}, errors.New("invalid registry key: missing subkey path")
	}

	hive, ok := normaliseHive(parts[0])
	if !ok {
		return registryPathParts{}, errors.New("invalid registry hive: " + parts[0])
	}

	rest := parts[1]
	if rest == "" {
		return registryPathParts{}, errors.New("invalid registry key: empty subkey path")
	}

	// Check for explicit "::" separator first. This handles value names
	// that contain backslashes (e.g. HardenedPaths UNC entries).
	if idx := strings.Index(rest, "::"); idx >= 0 {
		return registryPathParts{
			Hive:      hive,
			SubKey:    rest[:idx],
			ValueName: rest[idx+2:],
		}, nil
	}

	// Standard format: split at the last backslash.
	// Trailing backslash means the key itself (empty value name).
	lastSep := strings.LastIndex(rest, `\`)
	if lastSep < 0 {
		return registryPathParts{
			Hive:      hive,
			SubKey:    "",
			ValueName: rest,
		}, nil
	}

	return registryPathParts{
		Hive:      hive,
		SubKey:    rest[:lastSep],
		ValueName: rest[lastSep+1:],
	}, nil
}
