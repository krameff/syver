//go:build windows

package system

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

const diagTestSubKey = `Software\SyverDiagRoundtripTest`

// withDiagKey builds the shape the FEAT-012 W2-5 diagnostic exists for: a
// parent holding a SUBKEY named "Ambiguous" and, separately, a VALUE named
// "Real". Asking for the value "Ambiguous" then misses while something of that
// name is plainly sitting there.
//
// It also creates two shapes the obvious version of this fixture misses. A
// subkey whose name contains a percent sign, because registry names are full of
// those -- everything under Session Manager\Environment -- and they are the
// input that tells log.Print from log.Printf. And "Twin", which exists as BOTH
// a value and a subkey: values and subkeys are separate namespaces, so that is
// legal, common, and the only shape in which the diagnostic could fire on a
// lookup that HIT.
func withDiagKey(t *testing.T) {
	t.Helper()

	k, _, err := registry.CreateKey(registry.CURRENT_USER, diagTestSubKey,
		registry.SET_VALUE|registry.QUERY_VALUE|registry.CREATE_SUB_KEY)
	if err != nil {
		t.Fatalf("creating HKCU\\%s: %v", diagTestSubKey, err)
	}
	for name, val := range map[string]string{
		"Real": "a genuine value",
		"Twin": "a value that also has a subkey of the same name",
	} {
		if err := k.SetStringValue(name, val); err != nil {
			t.Fatalf("SetStringValue(%s): %v", name, err)
		}
	}
	for _, sub := range []string{"Ambiguous", "%SystemRoot%", "Twin"} {
		sk, _, err := registry.CreateKey(k, sub, registry.QUERY_VALUE)
		if err != nil {
			t.Fatalf("creating subkey %q: %v", sub, err)
		}
		sk.Close()
	}

	t.Cleanup(func() {
		for _, sub := range []string{"Ambiguous", "%SystemRoot%", "Twin"} {
			if err := registry.DeleteKey(k, sub); err != nil {
				t.Errorf("leaked HKCU\\%s\\%s: %v", diagTestSubKey, sub, err)
			}
		}
		k.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, diagTestSubKey); err != nil {
			t.Errorf("leaked HKCU\\%s: %v", diagTestSubKey, err)
		}
	})
}

// captureLog redirects the standard logger for the duration of fn. The
// diagnostic goes through log rather than through a formatter precisely so that
// --log-level governs it, which also makes it observable here.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	fn()
	log.SetOutput(os.Stderr)
	return buf.String()
}

// TestValueMissBesideAKeyOfThatNameWarnsFromEveryAccessor is the firing
// condition, which no test reached before: the diagnostic was a log.Printf with
// nothing observing it, on the one code path whose whole purpose is to say
// something about a check that PASSES.
//
// All three accessors are exercised, because all three have their own call site
// and a fix applied to one of them is exactly the kind of change that leaves
// the other two silent.
//
// REVERT-PROOF: delete the warnIfKeyOfThatNameExists call from Exists (or from
// Value, or from Type) and the corresponding subtest fails while the other two
// still pass. Confirmed on win11-test, 2026-09-09.
func TestValueMissBesideAKeyOfThatNameWarnsFromEveryAccessor(t *testing.T) {
	withDiagKey(t)

	const key = `HKCU\` + diagTestSubKey + `\Ambiguous`

	t.Run("Exists", func(t *testing.T) {
		r := &defRegistryWindows{key: key}
		var exists bool
		var err error
		out := captureLog(t, func() { exists, err = r.Exists() })

		// The trap this warns about: the honest answer is false and the check
		// PASSES, so nothing in the output tells the author they asked about a
		// value when they meant a key.
		if err != nil || exists {
			t.Fatalf("Exists() = %v, %v; want false, nil -- a subkey is not a value", exists, err)
		}
		assertWarnsAbout(t, out, key)
	})

	t.Run("Value", func(t *testing.T) {
		r := &defRegistryWindows{key: key}
		out := captureLog(t, func() { _, _ = r.Value() })
		assertWarnsAbout(t, out, key)
	})

	t.Run("Type", func(t *testing.T) {
		r := &defRegistryWindows{key: key}
		out := captureLog(t, func() { _, _ = r.Type() })
		assertWarnsAbout(t, out, key)
	})
}

func assertWarnsAbout(t *testing.T, out, key string) {
	t.Helper()

	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("no warning was logged for %s; the false PASS this diagnostic "+
			"exists for is silent again. Got: %q", key, out)
	}
	if !strings.Contains(out, key+`\`) {
		t.Errorf("the warning does not hand back the corrected path %q, which is "+
			"the only actionable thing in it. Got: %q", key+`\`, out)
	}
}

// TestTheDiagnosticStaysQuietWhenThereIsNothingToSay pins the other half.
//
// A warning that fires on ordinary absent values would be worse than no warning
// at all: `exists: false` is a completely normal assertion in a hardening spec,
// and a spec full of them would bury the one case that matters under noise it
// caused itself. The extra OpenKey it costs is also only justified on a miss.
//
// TWO SEPARATE GUARDS keep it quiet, and an earlier version of this test
// covered only one of them -- the mutation meant to break it passed. They are
// listed here as the mutations that actually reach them, because "this test is
// about silence" is not enough to tell you what silence depends on:
//
// REVERT-PROOF 1, the guard inside warnIfKeyOfThatNameExists: drop its
// `if err != nil { return }` so it warns even when the probe open failed, and
// the two absent-name cases fail.
//
// REVERT-PROOF 2, the `err == nil && !exists` guard in Exists: replace it with
// `if true` so the probe runs on a HIT too, and the "Twin" case fails -- and
// ONLY the Twin case, because a name that is not also a subkey cannot produce a
// warning however unconditionally the probe is called. Twin is in the table for
// exactly that reason.
//
// Both confirmed on win11-test, 2026-09-09.
func TestTheDiagnosticStaysQuietWhenThereIsNothingToSay(t *testing.T) {
	withDiagKey(t)

	for _, tc := range []struct {
		name, key  string
		wantExists bool
	}{
		// Nothing of that name exists in any form. An ordinary `exists: false`.
		{"absent value with no key beside it", `HKCU\` + diagTestSubKey + `\NoSuchThingAtAll`, false},
		// A value that is really there. The miss branch is never entered, so
		// the second OpenKey is never paid.
		{"value that exists", `HKCU\` + diagTestSubKey + `\Real`, true},
		// The corrected form the warning itself suggests. Having asked the
		// right question, the author must not be told to ask it.
		{"the key, addressed with a trailing backslash", `HKCU\` + diagTestSubKey + `\Ambiguous\`, true},
		// A name that is a value AND a subkey. The lookup HITS, so there is
		// nothing ambiguous about the answer and nothing to warn about -- but a
		// subkey of that name really is sitting there, so the probe inside
		// warnIfKeyOfThatNameExists would succeed if it were ever reached. This
		// is the only row that depends on Exists guarding the call at all.
		{"a name that is both a value and a key", `HKCU\` + diagTestSubKey + `\Twin`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &defRegistryWindows{key: tc.key}
			var exists bool
			var err error
			out := captureLog(t, func() { exists, err = r.Exists() })

			if err != nil {
				t.Fatalf("Exists(): %v", err)
			}
			if exists != tc.wantExists {
				t.Fatalf("Exists() = %v, want %v", exists, tc.wantExists)
			}
			if strings.Contains(out, "[WARN]") {
				t.Errorf("warned about %s, which is not ambiguous: %q", tc.key, out)
			}
		})
	}
}

// TestTheDiagnosticSurvivesAPercentInTheName is why the call site uses
// log.Print and not log.Printf.
//
// The message arrives pre-formatted. Handed to Printf with no arguments, a
// registry name containing a percent is read as a format verb and the path
// comes out as %!S(MISSING) -- so the diagnostic would corrupt the one thing it
// exists to hand back, and only for the names most likely to appear under
// Session Manager\Environment.
//
// REVERT-PROOF: change the call site back to
// log.Printf(registryValueVsKeyWarning(...)) and this fails with
// %!S(MISSING) in the output. Confirmed on win11-test, 2026-09-09.
func TestTheDiagnosticSurvivesAPercentInTheName(t *testing.T) {
	withDiagKey(t)

	const key = `HKCU\` + diagTestSubKey + `\%SystemRoot%`

	r := &defRegistryWindows{key: key}
	out := captureLog(t, func() { _, _ = r.Exists() })

	if strings.Contains(out, "%!") {
		t.Fatalf("the pre-formatted warning was re-interpreted as a format string: %q", out)
	}
	assertWarnsAbout(t, out, key)
}
