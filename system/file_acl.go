package system

import (
	"strconv"
	"strings"
)

// ACL rendering, deliberately in a file with NO build tag.
//
// FEAT-016. The mask-to-token mapping is pure arithmetic, and it is the part of
// this feature most likely to be wrong: see the D5 trap in the spec, where .NET
// renders a generic mask as a raw signed integer ("268435456") on precisely the
// ACEs a hardening control asserts. Keeping it platform-neutral means
// file_acl_test.go exercises every case on Linux, in the ordinary gate, with no
// Windows host -- the same reasoning as system/windows_unsupported_guard_test.go.
//
// Only the syscalls live in file_acl_windows.go.
//
// The tokens and their spelling are icacls's own, taken from `icacls /?` on
// Windows Server 2025 rather than from memory. An element renders as
//
//	PRINCIPAL:(I)(DENY)(OI)(CI)(IO)(NP)(RIGHTS)
//
// with each parenthesised group present only when it applies, in that order,
// which is the order icacls prints. Verified against real output:
//
//	NT AUTHORITY\SYSTEM:(I)(F)                  an inherited full-control ACE
//	BUILTIN\Users:(DENY)(W)                     an explicit deny
//	BUILTIN\Users:(OI)(CI)(IO)(GR,GE)           inherit-only, generic rights
//	BUILTIN\Administrators:(M)                  modify on the object itself
const (
	// Standard and generic rights. Values are winnt.h's; a Windows-only test
	// (file_acl_windows_test.go) pins each one against golang.org/x/sys/windows
	// so these copies cannot drift from the real constants.
	aclDelete               = 0x00010000
	aclReadControl          = 0x00020000
	aclWriteDac             = 0x00040000
	aclWriteOwner           = 0x00080000
	aclSynchronize          = 0x00100000
	aclAccessSystemSecurity = 0x01000000
	aclMaximumAllowed       = 0x02000000
	aclGenericAll           = 0x10000000
	aclGenericExecute       = 0x20000000
	aclGenericWrite         = 0x40000000
	aclGenericRead          = 0x80000000

	// File-specific rights.
	aclReadData        = 0x0001
	aclWriteData       = 0x0002
	aclAppendData      = 0x0004
	aclReadEA          = 0x0008
	aclWriteEA         = 0x0010
	aclExecute         = 0x0020
	aclDeleteChild     = 0x0040
	aclReadAttributes  = 0x0080
	aclWriteAttributes = 0x0100

	aclStandardRightsRequired = 0x000F0000

	// Composites. Expressed from the bits above rather than as hex literals, so
	// a reader can check the arithmetic and a typo cannot pass review unnoticed.
	aclFileGenericRead    = aclReadControl | aclReadData | aclReadAttributes | aclReadEA | aclSynchronize
	aclFileGenericWrite   = aclReadControl | aclWriteData | aclWriteAttributes | aclWriteEA | aclAppendData | aclSynchronize
	aclFileGenericExecute = aclReadControl | aclReadAttributes | aclExecute | aclSynchronize
	aclFileAllAccess      = aclStandardRightsRequired | aclSynchronize | 0x1FF

	aclModify      = aclFileGenericRead | aclFileGenericWrite | aclFileGenericExecute | aclDelete
	aclReadExecute = aclFileGenericRead | aclFileGenericExecute
	aclReadOnly    = aclFileGenericRead
	aclWriteOnly   = aclFileGenericWrite
)

// aclSpecificRights is the fallback vocabulary, in the order `icacls /?` lists
// it, which is also the order icacls prints multiple tokens in: the measured
// "(GR,GE)" has GR before GE. Order matters because a spec asserts the rendered
// string.
var aclSpecificRights = []struct {
	mask  uint32
	token string
}{
	{aclDelete, "DE"},
	{aclReadControl, "RC"},
	{aclWriteDac, "WDAC"},
	{aclWriteOwner, "WO"},
	{aclSynchronize, "S"},
	{aclAccessSystemSecurity, "AS"},
	{aclMaximumAllowed, "MA"},
	{aclGenericRead, "GR"},
	{aclGenericWrite, "GW"},
	{aclGenericExecute, "GE"},
	{aclGenericAll, "GA"},
	{aclReadData, "RD"},
	{aclWriteData, "WD"},
	{aclAppendData, "AD"},
	{aclReadEA, "REA"},
	{aclWriteEA, "WEA"},
	{aclExecute, "X"},
	{aclDeleteChild, "DC"},
	{aclReadAttributes, "RA"},
	{aclWriteAttributes, "WA"},
}

// formatAceRights renders an access mask the way icacls prints it: a single
// simple-rights letter when the mask is exactly one of the well-known
// composites, otherwise the comma-joined specific tokens.
//
// Two asymmetries here are icacls's, not ours, and both were MEASURED on
// Windows Server 2025 rather than reasoned about:
//
//   - GENERIC_ALL alone prints as "F", not "GA". The inherit-only ACEs on
//     C:\Windows carry mask 0x10000000 and icacls shows "(F)".
//   - GENERIC_READ|GENERIC_EXECUTE does NOT collapse to "RX". The matching ACE
//     on C:\Windows prints "(GR,GE)". So the collapse applies to the
//     file-specific composites only, and the generic bits fall through to
//     tokens.
//
// Reproducing icacls's quirks is the whole point: the spec promises an operator
// can diff a failure against `icacls <path>` with no translation step.
func formatAceRights(mask uint32) string {
	switch mask {
	case 0:
		return "N"
	case aclGenericAll, aclFileAllAccess:
		return "F"
	case aclModify:
		return "M"
	case aclReadExecute:
		return "RX"
	case aclReadOnly:
		return "R"
	case aclWriteOnly:
		return "W"
	case aclDelete:
		return "D"
	}

	var tokens []string
	remaining := mask
	for _, r := range aclSpecificRights {
		if mask&r.mask == r.mask {
			tokens = append(tokens, r.token)
			remaining &^= r.mask
		}
	}
	// A bit nobody has a name for is reported rather than dropped. Silently
	// discarding part of a mask would let `consist-of` assert "these rights and
	// no others" while syver had failed to render some of them.
	if remaining != 0 {
		tokens = append(tokens, "0x"+strings.ToUpper(strconv.FormatUint(uint64(remaining), 16)))
	}
	if len(tokens) == 0 {
		return "N"
	}
	return strings.Join(tokens, ",")
}

// aclInheritFlags are the ACE header flags, in icacls's printed order. The
// inherited marker (I) comes first and is handled separately because icacls
// prints it ahead of the deny marker.
var aclInheritFlags = []struct {
	flag  uint8
	token string
}{
	{aceObjectInherit, "OI"},
	{aceContainerInherit, "CI"},
	{aceInheritOnly, "IO"},
	{aceNoPropagateInherit, "NP"},
}

// ACE header flag and type values. Pinned against golang.org/x/sys/windows by
// the Windows-only test, same as the rights constants above.
const (
	aceObjectInherit      = 0x1
	aceContainerInherit   = 0x2
	aceNoPropagateInherit = 0x4
	aceInheritOnly        = 0x8
	aceInherited          = 0x10

	aceTypeAccessAllowed = 0x0
	aceTypeAccessDenied  = 0x1
)

// formatAce renders one ACE as a single gossfile list element. principal is
// either a DOMAIN\Name or a SID string; the two projections differ only there,
// which is what lets acl: and acl-sid: be converted into one another
// index-for-index.
func formatAce(principal string, aceType, aceFlags uint8, mask uint32) string {
	var b strings.Builder
	b.WriteString(principal)
	b.WriteString(":")
	if aceFlags&aceInherited != 0 {
		b.WriteString("(I)")
	}
	if aceType == aceTypeAccessDenied {
		b.WriteString("(DENY)")
	}
	for _, f := range aclInheritFlags {
		if aceFlags&f.flag != 0 {
			b.WriteString("(" + f.token + ")")
		}
	}
	b.WriteString("(" + formatAceRights(mask) + ")")
	return b.String()
}
