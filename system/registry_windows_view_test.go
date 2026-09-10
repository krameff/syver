//go:build windows

package system

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// TestNativeViewAddsNoAccessFlag is the safety property behind the whole
// `view:` attribute, and it is the one worth pinning hardest.
//
// Every gossfile written before this attribute existed has no `view:`, parses
// as native, and MUST produce byte-identical registry access to the code that
// had no concept of views. If native ever starts adding WOW64_64KEY "because
// syver is a 64-bit process anyway", that is a silent behaviour change to every
// existing spec on every Windows host, and nothing else in the suite would
// notice.
func TestNativeViewAddsNoAccessFlag(t *testing.T) {
	r := &defRegistryWindows{key: `HKLM\SOFTWARE\Syver\X`}

	if got := r.accessMask(); got != uint32(registry.QUERY_VALUE) {
		t.Fatalf("default accessMask = %#x, want exactly QUERY_VALUE (%#x): "+
			"an unset view must not change how the registry is opened",
			got, uint32(registry.QUERY_VALUE))
	}

	if err := r.SetView("native"); err != nil {
		t.Fatalf("SetView(native): %v", err)
	}
	if got := r.accessMask(); got != uint32(registry.QUERY_VALUE) {
		t.Fatalf("explicit native accessMask = %#x, want exactly QUERY_VALUE (%#x)",
			got, uint32(registry.QUERY_VALUE))
	}
}

func TestViewSelectsTheWow64Flag(t *testing.T) {
	for _, tc := range []struct {
		view string
		want uint32
	}{
		{"32", uint32(registry.QUERY_VALUE) | registry.WOW64_32KEY},
		{"64", uint32(registry.QUERY_VALUE) | registry.WOW64_64KEY},
	} {
		r := &defRegistryWindows{key: `HKLM\SOFTWARE\Syver\X`}
		if err := r.SetView(tc.view); err != nil {
			t.Fatalf("SetView(%q): %v", tc.view, err)
		}
		if got := r.accessMask(); got != tc.want {
			t.Errorf("view %q gave accessMask %#x, want %#x", tc.view, got, tc.want)
		}
	}
}

// An unusable view must fail the check that used it, not vanish. SetView
// returns nil by design -- the error is carried and surfaces from the
// accessors, so it lands on the assertion rather than in a constructor whose
// return value nothing inspects.
func TestInvalidViewFailsTheCheckRatherThanDisappearing(t *testing.T) {
	r := &defRegistryWindows{key: `HKLM\SOFTWARE\Syver\X`}
	if err := r.SetView("x86"); err != nil {
		t.Fatalf("SetView is not the place the error surfaces; it returned %v", err)
	}

	if _, err := r.Exists(); err == nil {
		t.Error("Exists() succeeded with an invalid view; the check would pass or fail " +
			"on a reading from a view the author never asked for")
	}
	if _, err := r.Value(); err == nil {
		t.Error("Value() succeeded with an invalid view")
	}
	if _, err := r.Type(); err == nil {
		t.Error("Type() succeeded with an invalid view")
	}
}

// FEAT-012 W2-6. Every REG_* type must have a name, because a `type:`
// assertion cannot be written against UNKNOWN(n). typeName is pure, so this is
// a cheap and complete check rather than a sampled one.
func TestTypeNameCoversEveryRegistryType(t *testing.T) {
	for raw, want := range map[uint32]string{
		registry.NONE:                       "REG_NONE",
		registry.SZ:                         "REG_SZ",
		registry.EXPAND_SZ:                  "REG_EXPAND_SZ",
		registry.BINARY:                     "REG_BINARY",
		registry.DWORD:                      "REG_DWORD",
		registry.DWORD_BIG_ENDIAN:           "REG_DWORD_BIG_ENDIAN",
		registry.LINK:                       "REG_LINK",
		registry.MULTI_SZ:                   "REG_MULTI_SZ",
		registry.RESOURCE_LIST:              "REG_RESOURCE_LIST",
		registry.FULL_RESOURCE_DESCRIPTOR:   "REG_FULL_RESOURCE_DESCRIPTOR",
		registry.RESOURCE_REQUIREMENTS_LIST: "REG_RESOURCE_REQUIREMENTS_LIST",
		registry.QWORD:                      "REG_QWORD",
	} {
		if got := typeName(raw); got != want {
			t.Errorf("typeName(%d) = %q, want %q", raw, got, want)
		}
	}

	// A type outside the documented set must still say so rather than be
	// silently renamed to something plausible.
	if got := typeName(9999); got == "" || got[:7] != "UNKNOWN" {
		t.Errorf("typeName(9999) = %q, want an UNKNOWN(...) form", got)
	}
}
