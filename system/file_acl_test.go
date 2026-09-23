package system

import "testing"

// FEAT-016. These cases are MEASURED against `icacls` on Windows Server 2025,
// not derived from documentation. Each one that came from real output says which
// path produced it, so a future change can re-measure the same path rather than
// argue with the table.
//
// This file has no build tag on purpose: the mapping is the part of the ACL work
// most likely to be silently wrong, and it is worth catching from Linux in the
// ordinary gate. See file_acl.go's header.
func TestFormatAceRights(t *testing.T) {
	tests := []struct {
		name string
		mask uint32
		want string
	}{
		// Simple composites.
		{"full via FILE_ALL_ACCESS", aclFileAllAccess, "F"},
		{"modify", aclModify, "M"},
		{"read and execute", aclReadExecute, "RX"},
		{"read only", aclReadOnly, "R"},
		{"write only", aclWriteOnly, "W"},
		{"delete alone", aclDelete, "D"},
		{"empty mask", 0, "N"},

		// The two icacls asymmetries, both measured on C:\Windows. These are the
		// cases a from-first-principles implementation gets wrong.
		//
		// GENERIC_ALL alone prints F, not GA: the (OI)(CI)(IO) ACEs on C:\Windows
		// carry 0x10000000 and icacls shows "(F)". .NET renders the same ACE as
		// the bare integer 268435456.
		{"GENERIC_ALL prints F, not GA", aclGenericAll, "F"},
		// GENERIC_READ|GENERIC_EXECUTE does NOT collapse to RX. icacls prints
		// "(GR,GE)" for BUILTIN\Users on C:\Windows. .NET renders it -1610612736.
		{"generic read+execute stays GR,GE", aclGenericRead | aclGenericExecute, "GR,GE"},

		// Fallback vocabulary and its order.
		{"read control alone", aclReadControl, "RC"},
		{"delete plus read control", aclDelete | aclReadControl, "DE,RC"},
		{"generic write alone", aclGenericWrite, "GW"},
		{"specific bits keep icacls order", aclReadData | aclWriteData | aclExecute, "RD,WD,X"},

		// A bit with no name must be reported, never dropped: consist-of would
		// otherwise assert "these rights and no others" against a mask syver had
		// failed to render in full.
		{"unnamed bit is surfaced", aclReadData | 0x800, "RD,0x800"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatAceRights(tt.mask); got != tt.want {
				t.Errorf("formatAceRights(%#x) = %q, want %q", tt.mask, got, tt.want)
			}
		})
	}
}

// TestFormatAce covers the element spelling itself, including the ordering of
// the parenthesised groups. Every expected string here is a line that appeared
// in real icacls output, with the path noted.
func TestFormatAce(t *testing.T) {
	tests := []struct {
		name      string
		principal string
		aceType   uint8
		aceFlags  uint8
		mask      uint32
		want      string
	}{
		{
			// icacls C:\Windows\System32\drivers\etc\hosts
			name: "inherited full control", principal: `NT AUTHORITY\SYSTEM`,
			aceType: aceTypeAccessAllowed, aceFlags: aceInherited, mask: aclFileAllAccess,
			want: `NT AUTHORITY\SYSTEM:(I)(F)`,
		},
		{
			// icacls on a temp file after `icacls /deny "BUILTIN\Users:(W)"`
			name: "explicit deny", principal: `BUILTIN\Users`,
			aceType: aceTypeAccessDenied, aceFlags: 0, mask: aclWriteOnly,
			want: `BUILTIN\Users:(DENY)(W)`,
		},
		{
			// icacls C:\Windows
			name: "inherit-only generic", principal: `BUILTIN\Users`,
			aceType:  aceTypeAccessAllowed,
			aceFlags: aceObjectInherit | aceContainerInherit | aceInheritOnly,
			mask:     aclGenericRead | aclGenericExecute,
			want:     `BUILTIN\Users:(OI)(CI)(IO)(GR,GE)`,
		},
		{
			// icacls C:\Windows -- the same principal's other ACE, which is why
			// acl: is a list and not a map: this key would collide with the one
			// above.
			name: "modify on the object itself", principal: `BUILTIN\Administrators`,
			aceType: aceTypeAccessAllowed, aceFlags: 0, mask: aclModify,
			want: `BUILTIN\Administrators:(M)`,
		},
		{
			name: "inherited deny puts I before DENY", principal: `BUILTIN\Users`,
			aceType: aceTypeAccessDenied, aceFlags: aceInherited, mask: aclWriteOnly,
			want: `BUILTIN\Users:(I)(DENY)(W)`,
		},
		{
			name: "SID projection differs only in the principal", principal: "S-1-5-32-544",
			aceType: aceTypeAccessAllowed, aceFlags: aceInherited, mask: aclFileAllAccess,
			want: `S-1-5-32-544:(I)(F)`,
		},
		{
			name: "no-propagate flag", principal: `BUILTIN\Users`,
			aceType:  aceTypeAccessAllowed,
			aceFlags: aceContainerInherit | aceNoPropagateInherit, mask: aclReadExecute,
			want: `BUILTIN\Users:(CI)(NP)(RX)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAce(tt.principal, tt.aceType, tt.aceFlags, tt.mask)
			if got != tt.want {
				t.Errorf("formatAce() = %q, want %q", got, tt.want)
			}
		})
	}
}
