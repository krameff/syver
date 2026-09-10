package system

import (
	"testing"
)

func TestParseRegistryKey(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantHive  string
		wantSub   string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "standard path",
			input:     `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Microsoft\Windows\CurrentVersion`,
			wantValue: "Run",
		},
		{
			name:      "path with spaces",
			input:     `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProductName`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
			wantValue: "ProductName",
		},
		{
			name:      "default value with trailing backslash",
			input:     `HKLM\SOFTWARE\Microsoft\Windows\`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Microsoft\Windows`,
			wantValue: "",
		},
		{
			name:      "HKCU hive",
			input:     `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced\Hidden`,
			wantHive:  "HKCU",
			wantSub:   `Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`,
			wantValue: "Hidden",
		},
		{
			name:      "HKCR hive",
			input:     `HKCR\.txt\Content Type`,
			wantHive:  "HKCR",
			wantSub:   ".txt",
			wantValue: "Content Type",
		},
		{
			name:      "HKU hive",
			input:     `HKU\.DEFAULT\Software\Test`,
			wantHive:  "HKU",
			wantSub:   `.DEFAULT\Software`,
			wantValue: "Test",
		},
		{
			name:      "HKCC hive",
			input:     `HKCC\System\CurrentControlSet\Setting`,
			wantHive:  "HKCC",
			wantSub:   `System\CurrentControlSet`,
			wantValue: "Setting",
		},
		{
			name:      "single segment value under hive",
			input:     `HKLM\ValueOnly`,
			wantHive:  "HKLM",
			wantSub:   "",
			wantValue: "ValueOnly",
		},
		{
			name:      "lowercase hive is normalized",
			input:     `hklm\SOFTWARE\Test`,
			wantHive:  "HKLM",
			wantSub:   "SOFTWARE",
			wantValue: "Test",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no backslash",
			input:   "HKLM",
			wantErr: true,
		},
		{
			name:    "invalid hive",
			input:   `INVALID\SOFTWARE\Test`,
			wantErr: true,
		},
		{
			name:    "empty subkey after hive",
			input:   `HKLM\`,
			wantErr: true,
		},
		{
			name:      "explicit separator with backslashes in value name",
			input:     `HKLM\SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths::\\*\NETLOGON`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths`,
			wantValue: `\\*\NETLOGON`,
		},
		{
			name:      "explicit separator with SYSVOL UNC path",
			input:     `HKLM\SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths::\\*\SYSVOL`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Policies\Microsoft\Windows\NetworkProvider\HardenedPaths`,
			wantValue: `\\*\SYSVOL`,
		},
		{
			name:      "explicit separator with simple value name",
			input:     `HKLM\SOFTWARE\Microsoft\Windows::ProductName`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Microsoft\Windows`,
			wantValue: "ProductName",
		},
		{
			name:      "explicit separator with empty value name (default value)",
			input:     `HKLM\SOFTWARE\Microsoft\Windows::`,
			wantHive:  "HKLM",
			wantSub:   `SOFTWARE\Microsoft\Windows`,
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRegistryKey(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Hive != tt.wantHive {
				t.Errorf("Hive = %q, want %q", got.Hive, tt.wantHive)
			}
			if got.SubKey != tt.wantSub {
				t.Errorf("SubKey = %q, want %q", got.SubKey, tt.wantSub)
			}
			if got.ValueName != tt.wantValue {
				t.Errorf("ValueName = %q, want %q", got.ValueName, tt.wantValue)
			}
		})
	}
}

// FEAT-012 W2-3. The hive grammar used to accept only the five short names, so
// a path copied out of regedit's address bar or out of Get-ItemProperty output
// had to be hand-edited before syver would take it. These are the spellings the
// tools an operator copies from actually produce.
//
// This runs on Linux on purpose: parseRegistryKey is untagged, and a grammar
// contract does not need a Windows host to be wrong.
func TestParseRegistryKeyAcceptsTheSpellingsWindowsToolsShow(t *testing.T) {
	for _, tc := range []struct {
		key      string
		wantHive string
	}{
		{`HKLM\Software\Syver\Value`, "HKLM"},
		{`HKEY_LOCAL_MACHINE\Software\Syver\Value`, "HKLM"},
		{`HKLM:\Software\Syver\Value`, "HKLM"},
		{`HKEY_LOCAL_MACHINE:\Software\Syver\Value`, "HKLM"},
		{`hkey_local_machine\Software\Syver\Value`, "HKLM"},
		{`HKEY_CURRENT_USER\Console\Value`, "HKCU"},
		{`HKCU:\Console\Value`, "HKCU"},
		{`HKEY_CLASSES_ROOT\.txt\Value`, "HKCR"},
		{`HKEY_USERS\.DEFAULT\Value`, "HKU"},
		{`HKEY_CURRENT_CONFIG\System\Value`, "HKCC"},
	} {
		got, err := parseRegistryKey(tc.key)
		if err != nil {
			t.Errorf("parseRegistryKey(%q) = %v; this is a spelling regedit or PowerShell produces", tc.key, err)
			continue
		}
		if got.Hive != tc.wantHive {
			t.Errorf("parseRegistryKey(%q).Hive = %q, want the canonical %q", tc.key, got.Hive, tc.wantHive)
		}
		if got.ValueName != "Value" {
			t.Errorf("parseRegistryKey(%q).ValueName = %q, want %q", tc.key, got.ValueName, "Value")
		}
	}
}

// Normalising must not turn everything into a hive. A name that merely looks
// hive-shaped is still an error, or a typo silently reads the wrong hive.
func TestParseRegistryKeyStillRejectsNonHives(t *testing.T) {
	for _, key := range []string{
		`HKEY_LOCAL_MACHIN\Software\X`,
		`HKLMM\Software\X`,
		`HK\Software\X`,
		`HKEY_LOCAL_MACHINE_EXTRA\Software\X`,
		`:\Software\X`,
		`Software\X`,
	} {
		if _, err := parseRegistryKey(key); err == nil {
			t.Errorf("parseRegistryKey(%q) succeeded; a hive that does not exist must not be accepted", key)
		}
	}
}

// The trailing backslash is the distinction FEAT-012 W2-5 refuses to guess at.
// These two strings differ by one character and ask different questions, and
// that is the documented contract rather than an accident.
func TestParseRegistryKeyTrailingBackslashAddressesTheKey(t *testing.T) {
	withValue, err := parseRegistryKey(`HKLM\Software\Syver\ProfileList`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withValue.SubKey != `Software\Syver` || withValue.ValueName != "ProfileList" {
		t.Errorf("no trailing backslash should name a VALUE: got subkey %q value %q",
			withValue.SubKey, withValue.ValueName)
	}

	withoutValue, err := parseRegistryKey(`HKLM\Software\Syver\ProfileList\`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withoutValue.SubKey != `Software\Syver\ProfileList` || withoutValue.ValueName != "" {
		t.Errorf("a trailing backslash should name a KEY: got subkey %q value %q",
			withoutValue.SubKey, withoutValue.ValueName)
	}
}

// FEAT-012 W2-4. The view grammar is user-facing spec syntax, so it is pinned
// on the platform that cannot honour it as well as the one that can.
func TestParseRegistryView(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want RegistryView
	}{
		{"", RegistryViewNative},
		{"native", RegistryViewNative},
		{"NATIVE", RegistryViewNative},
		{" native ", RegistryViewNative},
		{"32", RegistryView32},
		{"64", RegistryView64},
	} {
		got, err := ParseRegistryView(tc.in)
		if err != nil {
			t.Errorf("ParseRegistryView(%q) = %v, want %v", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseRegistryView(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"x86", "amd64", "wow64", "32bit", "0", "true"} {
		if _, err := ParseRegistryView(bad); err == nil {
			t.Errorf("ParseRegistryView(%q) succeeded; an unrecognised view must not "+
				"silently fall back to native, which would read the wrong registry branch", bad)
		}
	}
}

// The empty view must be native, and native must be the zero value. If either
// stops holding, every gossfile written before `view:` existed changes meaning
// without its author touching it.
func TestUnsetViewIsNativeAndNativeIsTheZeroValue(t *testing.T) {
	var zero RegistryView
	if zero != RegistryViewNative {
		t.Fatal("RegistryViewNative is not the zero value; an unset view would not be native")
	}
	got, err := ParseRegistryView("")
	if err != nil || got != RegistryViewNative {
		t.Fatalf(`ParseRegistryView("") = %v, %v; want native and no error`, got, err)
	}
}
