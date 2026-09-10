//go:build windows

package system

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/krameff/syver/util"
)

type defRegistryWindows struct {
	key string
	// view and viewErr are set by SetView. An unusable view is carried rather
	// than returned so that it surfaces from the accessor that would have used
	// it, and therefore fails the check, instead of being swallowed at
	// construction time. Same shape as system/mount.go's deferred error.
	view    RegistryView
	viewErr error
}

func NewDefRegistry(_ context.Context, key string, system *System, config util.Config) Registry {
	return &defRegistryWindows{key: key}
}

func (r *defRegistryWindows) Key() string { return r.key }

// SetView records the requested WOW64 view. See the Registry interface for why
// an invalid value is stored rather than returned.
func (r *defRegistryWindows) SetView(view string) error {
	v, err := ParseRegistryView(view)
	r.view, r.viewErr = v, err
	return nil
}

// accessMask is the access the registry opens are performed with.
//
// QUERY_VALUE is what every read needs. The WOW64 flags are additive and, for
// RegistryViewNative, nothing is added at all -- so a spec that does not
// mention `view:` produces byte-identical behaviour to the code that existed
// before views did. That property is the whole safety argument for adding the
// attribute, and a test pins it.
func (r *defRegistryWindows) accessMask() uint32 {
	mask := uint32(registry.QUERY_VALUE)
	switch r.view {
	case RegistryView32:
		mask |= registry.WOW64_32KEY
	case RegistryView64:
		mask |= registry.WOW64_64KEY
	}
	return mask
}

// Exists distinguishes "the key/value genuinely does not exist"
// (registry.ErrNotExist -> false, nil) from every other failure, most
// importantly ERROR_ACCESS_DENIED (-> false, err). The two used to be folded
// together into an unconditional (false, nil), which is the most misleading
// answer this code could give: the keys a hardening spec targets are
// precisely the locked-down ones under paths like
// HKLM\SYSTEM\CurrentControlSet\Control\Lsa, and "could not read it" is not
// "it is absent". See FEAT-010 SW-3.
//
// The classification itself lives in classifyRegistryError, a pure function,
// so the not-found-vs-access-denied distinction is unit-testable without an
// actual unreadable registry key -- see registry_windows_error_test.go.
func (r *defRegistryWindows) Exists() (bool, error) {
	if r.viewErr != nil {
		return false, r.viewErr
	}

	parts, err := parseRegistryKey(r.key)
	if err != nil {
		return false, err
	}

	hive, err := lookupHive(parts.Hive)
	if err != nil {
		return false, err
	}

	k, openErr := registry.OpenKey(hive, parts.SubKey, r.accessMask())
	if exists, err := classifyRegistryError(openErr, "opening registry key"); !exists || err != nil {
		return exists, err
	}
	defer k.Close()

	if parts.ValueName == "" {
		return true, nil
	}

	_, _, getErr := k.GetValue(parts.ValueName, nil)
	exists, err := classifyRegistryError(getErr, "reading registry value")
	if err == nil && !exists {
		r.warnIfKeyOfThatNameExists(k, parts.ValueName)
	}
	return exists, err
}

// warnIfKeyOfThatNameExists is the FEAT-012 W2-5 diagnostic.
//
// A value lookup that misses while a SUBKEY of that name sits under the same
// parent is the one case where a truthful answer reliably answers a different
// question from the one the author asked. `exists: false` then PASSES, which
// makes it the same class of false confidence FEAT-010 existed to remove --
// worse, because nothing fails and nothing is logged.
//
// THREE DECISIONS, all carried unanswered out of FEAT-011 1.4 and settled here:
//
//  1. Warning, not failure text. Attaching it to failure output only would stay
//     silent on the false PASS, which is the case that motivated it. A warning
//     fires on both.
//  2. The extra registry open is paid only on a miss that is genuinely
//     not-found, never on a hit and never on an access-denied error. A spec
//     full of absent-value assertions under `serve` pays one OpenKey per such
//     assertion per cycle. That is the accepted cost; the alternative is
//     staying quiet about a passing check that is not testing what it says.
//  3. It goes through log, so it is formatter-independent. That sidesteps the
//     question of which formatters should carry it: none of them do, and
//     --log-level governs it like every other warning.
//
// Deliberately silent when the probe itself fails: this is a hint, and a hint
// that reports its own errors is noise on top of noise.
func (r *defRegistryWindows) warnIfKeyOfThatNameExists(parent registry.Key, name string) {
	sub, err := registry.OpenKey(parent, name, r.accessMask())
	if err != nil {
		return
	}
	sub.Close()
	// log.Print, not log.Printf: the message is already formatted, and a
	// registry path or value name containing a % would otherwise be rendered as
	// a verb against no arguments and print as %!x(MISSING). The formatting
	// itself lives in registryValueVsKeyWarning, in the untagged file, so the
	// parts of it that can be wrong without a Windows host are tested without
	// one -- see registry_warning_test.go for what "wrong" means here.
	log.Print(registryValueVsKeyWarning(r.key, name))
}

// classifyRegistryError turns a raw OpenKey/GetValue error into the
// three-way answer Exists needs: nil -> the key/value exists; a genuine
// registry.ErrNotExist -> does not exist, and that is not an error;
// anything else (ERROR_ACCESS_DENIED foremost among them) -> an error, never
// silently folded into "does not exist".
func classifyRegistryError(err error, wrapMsg string) (exists bool, resultErr error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("%s: %w", wrapMsg, err)
}

// openForRead is the shared open path for Value and Type. Both previously
// repeated parse, hive lookup and open verbatim, and both would have needed the
// view threading through independently.
func (r *defRegistryWindows) openForRead() (registry.Key, registryPathParts, error) {
	if r.viewErr != nil {
		return 0, registryPathParts{}, r.viewErr
	}

	parts, err := parseRegistryKey(r.key)
	if err != nil {
		return 0, parts, err
	}

	hive, err := lookupHive(parts.Hive)
	if err != nil {
		return 0, parts, err
	}

	k, err := registry.OpenKey(hive, parts.SubKey, r.accessMask())
	if err != nil {
		return 0, parts, fmt.Errorf("opening registry key: %w", err)
	}
	return k, parts, nil
}

func (r *defRegistryWindows) Value() (string, error) {
	k, parts, err := r.openForRead()
	if err != nil {
		return "", err
	}
	defer k.Close()

	_, valType, err := k.GetValue(parts.ValueName, nil)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) && parts.ValueName != "" {
			r.warnIfKeyOfThatNameExists(k, parts.ValueName)
		}
		return "", fmt.Errorf("reading registry value: %w", err)
	}

	return formatValue(valType, k, parts.ValueName)
}

func (r *defRegistryWindows) Type() (string, error) {
	k, parts, err := r.openForRead()
	if err != nil {
		return "", err
	}
	defer k.Close()

	_, valType, err := k.GetValue(parts.ValueName, nil)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) && parts.ValueName != "" {
			r.warnIfKeyOfThatNameExists(k, parts.ValueName)
		}
		return "", fmt.Errorf("reading registry value: %w", err)
	}

	return typeName(valType), nil
}

// lookupHive maps a CANONICAL short hive name onto its root key.
//
// It no longer re-validates spellings: parseRegistryKey normalises every
// accepted form (long, short, PowerShell colon) to the canonical short name
// before this is reached, so the two switches that used to list the same five
// hives are now one list, in hiveAliases. The default branch stays as a guard
// against a caller that skipped the parser, not as a second grammar.
func lookupHive(name string) (registry.Key, error) {
	switch name {
	case "HKLM":
		return registry.LOCAL_MACHINE, nil
	case "HKCU":
		return registry.CURRENT_USER, nil
	case "HKCR":
		return registry.CLASSES_ROOT, nil
	case "HKU":
		return registry.USERS, nil
	case "HKCC":
		return registry.CURRENT_CONFIG, nil
	default:
		return 0, fmt.Errorf("unknown registry hive: %s", name)
	}
}

// formatValue renders a value as the string a matcher compares against.
//
// REG_EXPAND_SZ IS COMPARED UNEXPANDED. GetStringValue returns the stored form,
// so a value holding "%SystemRoot%\System32" is matched as that literal text
// and not as "C:\Windows\System32". That is the right default for compliance
// work -- the benchmark states the stored form, and expansion depends on the
// environment of whoever is asking -- but it is not what an operator
// necessarily expects, so it is stated here and in docs/gossfile.md.
func formatValue(valType uint32, k registry.Key, name string) (string, error) {
	switch valType {
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return "", err
		}
		return s, nil
	case registry.DWORD, registry.QWORD:
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return "", err
		}
		return strconv.FormatUint(v, 10), nil
	case registry.BINARY, registry.NONE, registry.DWORD_BIG_ENDIAN,
		registry.RESOURCE_LIST, registry.FULL_RESOURCE_DESCRIPTOR,
		registry.RESOURCE_REQUIREMENTS_LIST:
		// Everything with no more specific rendering is hex. The three
		// RESOURCE_* types are hardware descriptors that have no textual form
		// worth inventing, and DWORD_BIG_ENDIAN is deliberately NOT folded in
		// with DWORD: GetIntegerValue would read it in the wrong byte order and
		// return a confidently wrong number.
		b, _, err := k.GetBinaryValue(name)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(b), nil
	case registry.MULTI_SZ:
		ss, _, err := k.GetStringsValue(name)
		if err != nil {
			return "", err
		}
		return strings.Join(ss, "\n"), nil
	case registry.LINK:
		// A symbolic link's target is not readable through the normal value
		// API, and pretending otherwise would fabricate a value.
		return "", errors.New("REG_LINK values cannot be read as a value; assert on type instead")
	default:
		return "", fmt.Errorf("unsupported registry value type: %d", valType)
	}
}

// typeName covers the full REG_* set, so a `type:` assertion works against the
// exotic types rather than reporting UNKNOWN(n) for anything outside the common
// six. The default branch stays: a type this build does not know about should
// say so rather than be silently renamed.
func typeName(t uint32) string {
	switch t {
	case registry.NONE:
		return "REG_NONE"
	case registry.SZ:
		return "REG_SZ"
	case registry.EXPAND_SZ:
		return "REG_EXPAND_SZ"
	case registry.BINARY:
		return "REG_BINARY"
	case registry.DWORD:
		return "REG_DWORD"
	case registry.DWORD_BIG_ENDIAN:
		return "REG_DWORD_BIG_ENDIAN"
	case registry.LINK:
		return "REG_LINK"
	case registry.MULTI_SZ:
		return "REG_MULTI_SZ"
	case registry.RESOURCE_LIST:
		return "REG_RESOURCE_LIST"
	case registry.FULL_RESOURCE_DESCRIPTOR:
		return "REG_FULL_RESOURCE_DESCRIPTOR"
	case registry.RESOURCE_REQUIREMENTS_LIST:
		return "REG_RESOURCE_REQUIREMENTS_LIST"
	case registry.QWORD:
		return "REG_QWORD"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", t)
	}
}
