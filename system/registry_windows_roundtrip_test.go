//go:build windows

package system

import (
	"errors"
	"math"
	"strconv"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The two tests in this file are the only ones in the tree that read a value
// syver itself put into a real registry. Everything else about the registry
// resource is either pure (parsing, view flags, type names) or asserts against
// keys Windows happened to ship, which can prove that a read RESOLVES but
// cannot prove it resolved to the right place.

const (
	viewTestSubKey  = `SOFTWARE\SyverViewRoundtripTest`
	viewTestValue   = "Marker"
	typeTestSubKey  = `Software\SyverTypeRoundtripTest`
	wow64PhysicalSK = `SOFTWARE\Wow6432Node\SyverViewRoundtripTest`
)

// createViewKey makes the test's own key in ONE WOW64 view and registers its
// removal. It reports whether the caller can proceed: writing under
// HKLM\SOFTWARE needs elevation, and a key that already exists belongs to
// somebody else and must not be written over or deleted.
func createViewKey(t *testing.T, viewFlag uint32, value string) bool {
	t.Helper()

	k, existed, err := registry.CreateKey(
		registry.LOCAL_MACHINE, viewTestSubKey,
		registry.SET_VALUE|registry.QUERY_VALUE|viewFlag)
	if errors.Is(err, registry.ErrNotExist) || err != nil {
		t.Skipf("cannot create HKLM\\%s in view %#x (%v); this test needs an "+
			"elevated Windows session and asserts nothing without one",
			viewTestSubKey, viewFlag, err)
		return false
	}
	if existed {
		k.Close()
		t.Skipf("HKLM\\%s already exists; refusing to overwrite and then delete "+
			"a key this test did not create", viewTestSubKey)
		return false
	}
	// Removal is registered HERE, before the write below, and not in the caller.
	// The key exists from the CreateKey above onwards; SetStringValue can fail;
	// and a caller-side t.Cleanup is not registered until this function has
	// already returned. That gap leaked an empty key on the one path nobody
	// exercises. Found by re-reading for the question "can this litter a real
	// machine", not by it happening.
	physical := viewTestSubKey
	if viewFlag == registry.WOW64_32KEY {
		physical = wow64PhysicalSK
	}
	t.Cleanup(func() {
		if err := registry.DeleteKey(registry.LOCAL_MACHINE, physical); err != nil {
			t.Errorf("leaked HKLM\\%s: %v", physical, err)
		}
	})

	if err := k.SetStringValue(viewTestValue, value); err != nil {
		k.Close()
		t.Fatalf("setting %s in view %#x: %v", viewTestValue, viewFlag, err)
	}
	k.Close()
	return true
}

// TestThe32And64ViewsReadDifferentValuesFromTheSamePath is the assertion the
// `view:` attribute was added for, and until this test existed nothing made it.
//
// TestViewSelectsTheWow64Flag proves the right WOW64 bit reaches the access
// mask. The Windows fixture proves both views RESOLVE. Neither proves the two
// views land on different data, and an implementation that accepted `view:` and
// quietly ignored it would pass both.
//
// The key is created twice, once through each view, with a DIFFERENT value each
// time, and then read back through the production accessor. On 64-bit Windows
// the 32-bit branch is physically HKLM\SOFTWARE\Wow6432Node\..., which is
// asserted here too: if the redirection ever stopped happening the two writes
// would collide on one key and the last one would win.
//
// REVERT-PROOF: make accessMask() return QUERY_VALUE unconditionally -- the
// change that turns `view:` into decoration -- and this fails on the 32-bit
// read, reporting "sixtyfour" where "thirtytwo" was written. Confirmed on
// win11-test, 2026-09-09.
func TestThe32And64ViewsReadDifferentValuesFromTheSamePath(t *testing.T) {
	const (
		want64 = "sixtyfour"
		want32 = "thirtytwo"
	)

	// Each createViewKey registers its own removal before it can fail, so there
	// is deliberately no cleanup wiring out here.
	if !createViewKey(t, registry.WOW64_64KEY, want64) {
		return
	}
	if !createViewKey(t, registry.WOW64_32KEY, want32) {
		return
	}

	full := `HKLM\` + viewTestSubKey + `\` + viewTestValue

	for _, tc := range []struct{ view, want string }{
		{"64", want64},
		{"32", want32},
		// No view at all, and the explicit spelling of no view at all. syver is
		// a 64-bit binary, so both must agree with the 64-bit branch. This is
		// the compatibility promise in RegistryViewNative's comment, checked
		// against data rather than against an access mask.
		{"", want64},
		{"native", want64},
	} {
		r := &defRegistryWindows{key: full}
		if err := r.SetView(tc.view); err != nil {
			t.Fatalf("SetView(%q): %v", tc.view, err)
		}

		if exists, err := r.Exists(); err != nil || !exists {
			t.Fatalf("view %q: Exists() = %v, %v; want true, nil", tc.view, exists, err)
		}
		got, err := r.Value()
		if err != nil {
			t.Fatalf("view %q: Value(): %v", tc.view, err)
		}
		if got != tc.want {
			t.Errorf("view %q read %q, want %q: the view did not select the branch "+
				"it names", tc.view, got, tc.want)
		}
	}

	// The redirection is what makes the two branches distinct in the first
	// place. Assert it directly, so that a failure above can be told apart from
	// the two writes having landed on one key.
	phys := &defRegistryWindows{key: `HKLM\` + wow64PhysicalSK + `\` + viewTestValue}
	got, err := phys.Value()
	if err != nil {
		t.Fatalf("reading the physical Wow6432Node path: %v", err)
	}
	if got != want32 {
		t.Errorf("HKLM\\%s holds %q, want %q: the 32-bit write was not redirected, "+
			"so the two views were never separate", wow64PhysicalSK, got, want32)
	}
}

// TestValueRenderingForTheTypesTheSpecNames closes FEAT-012's REG_MULTI_SZ and
// REG_QWORD acceptance criteria, both of which were claimed rather than
// asserted: typeName is a pure map and proves only that a NAME exists, and
// formatValue had no test against a value of either type at all.
//
// HKCU, not HKLM, and deliberately: registry redirection does not apply to
// HKCU\Software, so nothing here needs elevation and this runs on any Windows
// developer's machine.
//
// REVERT-PROOF, each independently confirmed on win11-test, 2026-09-09:
//   - MULTI_SZ joined with " " instead of "\n"     -> the multi-line case fails
//   - QWORD folded into the SZ branch              -> every QWORD case fails
//   - QWORD rendered with strconv.FormatInt        -> maxUint64 fails, and only
//     that case, which is why the boundary value is in the table
func TestValueRenderingForTheTypesTheSpecNames(t *testing.T) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, typeTestSubKey,
		registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("creating HKCU\\%s: %v", typeTestSubKey, err)
	}
	t.Cleanup(func() {
		k.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, typeTestSubKey); err != nil {
			t.Errorf("leaked HKCU\\%s: %v", typeTestSubKey, err)
		}
	})

	multi := map[string][]string{
		// The ordinary case, and the one docs/gossfile.md describes: the
		// elements come back joined with newlines, NOT as a YAML list. An
		// author writing `value:` for one of these has to know that.
		"MultiOrdinary": {"alpha", "beta", "gamma"},
		// One element must produce NO separator. A join implemented as
		// "append a newline after each" passes the case above and fails here.
		"MultiSingle": {"only"},
		// An empty element is not the same as no element. Windows stores this
		// faithfully and the rendering has to keep the blank line, or two
		// different registry contents render identically.
		"MultiWithBlank": {"alpha", "", "gamma"},
	}
	for name, val := range multi {
		if err := k.SetStringsValue(name, val); err != nil {
			t.Fatalf("SetStringsValue(%s): %v", name, err)
		}
	}

	qwords := map[string]uint64{
		"QwordOrdinary": 1234567890123,
		// Above 2^32, so a REG_QWORD that was silently read as a REG_DWORD
		// would truncate rather than merely be mistyped.
		"QwordAbove32Bits": 1 << 40,
		// The value that separates FormatUint from FormatInt. FormatInt renders
		// this as -1, which is not a number that appears in any registry.
		"QwordMax": math.MaxUint64,
	}
	for name, val := range qwords {
		if err := k.SetQWordValue(name, val); err != nil {
			t.Fatalf("SetQWordValue(%s): %v", name, err)
		}
	}

	// The control. REG_DWORD and REG_QWORD share formatValue's integer branch,
	// so a DWORD asserted alongside is what shows that the QWORD results are
	// not simply the DWORD path answering for everything.
	if err := k.SetDWordValue("DwordControl", 42); err != nil {
		t.Fatalf("SetDWordValue: %v", err)
	}

	cases := []struct{ name, wantType, wantValue string }{
		{"MultiOrdinary", "REG_MULTI_SZ", "alpha\nbeta\ngamma"},
		{"MultiSingle", "REG_MULTI_SZ", "only"},
		{"MultiWithBlank", "REG_MULTI_SZ", "alpha\n\ngamma"},
		{"QwordOrdinary", "REG_QWORD", "1234567890123"},
		{"QwordAbove32Bits", "REG_QWORD", strconv.FormatUint(1<<40, 10)},
		{"QwordMax", "REG_QWORD", strconv.FormatUint(math.MaxUint64, 10)},
		{"DwordControl", "REG_DWORD", "42"},
	}

	for _, tc := range cases {
		r := &defRegistryWindows{key: `HKCU\` + typeTestSubKey + `\` + tc.name}

		if exists, err := r.Exists(); err != nil || !exists {
			t.Errorf("%s: Exists() = %v, %v; want true, nil", tc.name, exists, err)
			continue
		}
		gotType, err := r.Type()
		if err != nil {
			t.Errorf("%s: Type(): %v", tc.name, err)
			continue
		}
		if gotType != tc.wantType {
			t.Errorf("%s: Type() = %q, want %q", tc.name, gotType, tc.wantType)
		}
		gotValue, err := r.Value()
		if err != nil {
			t.Errorf("%s: Value(): %v", tc.name, err)
			continue
		}
		if gotValue != tc.wantValue {
			t.Errorf("%s: Value() = %q, want %q", tc.name, gotValue, tc.wantValue)
		}
	}
}
