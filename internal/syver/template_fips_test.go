package syver

import (
	"bytes"
	"crypto/fips140"
	"encoding/base64"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/go-sprout/sprout/sprigin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forceTemplateFIPSEnforced makes the template guards see FIPS 140-only mode.
// Not parallel: it swaps a package variable.
func forceTemplateFIPSEnforced(t *testing.T, on bool) {
	t.Helper()
	orig := templateFIPSEnforced
	templateFIPSEnforced = func() bool { return on }
	t.Cleanup(func() { templateFIPSEnforced = orig })
}

// templateFIPSArg is an argument the real FIPS 140-only mode refuses for each
// guarded function.
func templateFIPSArg(name string) string {
	if name == "genPrivateKey" {
		return `"dsa"`
	}
	return `"abc"`
}

// TestTemplateFIPSGuardFailsTheRender (T3a): with the mode forced, a guarded
// function whose sprout value returns an empty value fails the render, naming the
// function and the mode. A normal run cannot make sprout's real functions
// return one, so each guarded name gets a stub delegate that does, and
// the stubbed map goes through the real fipsGuardTemplateFuncs.
// Revert-proof: make fipsGuardTemplateFunc return its delegate unchanged; every
// subtest then renders an empty value with no error.
func TestTemplateFIPSGuardFailsTheRender(t *testing.T) {
	forceTemplateFIPSEnforced(t, true)
	stubbed := sprigin.TxtFuncMap()
	for name := range fipsGuardedTemplateFuncs {
		// What the mode leaves, as measured: nil after sprout recovers the
		// checksums' panic, "" from genPrivateKey "dsa" (its error discarded).
		var empty any
		if name == "genPrivateKey" {
			empty = ""
		}
		stubbed[name] = sproutFunc(func(...any) (any, error) { return empty, nil })
	}
	guards := fipsGuardTemplateFuncs(stubbed)

	for name := range fipsGuardedTemplateFuncs {
		t.Run(name, func(t *testing.T) {
			src := "{{ " + name + " " + templateFIPSArg(name) + " }}"
			tmpl, err := template.New("t").Funcs(stubbed).Funcs(guards).Funcs(funcMap).Parse(src)
			require.NoError(t, err)
			var out bytes.Buffer
			err = tmpl.Execute(&out, nil)
			require.Error(t, err, "rendered %q", out.String())
			assert.Contains(t, err.Error(), name)
			assert.Contains(t, err.Error(), "FIPS 140-only")
			assert.NotContains(t, out.String(), "<no value>")
		})
	}
}

// TestTemplateFIPSGuardPassesValuesThrough (T3a, second half): in the forced
// mode a value from the delegate is returned as is, so a guarded function
// that the mode allows (genPrivateKey "rsa", say) keeps working.
// Revert-proof: return the error whenever templateFIPSEnforced() is true,
// whatever the delegate gave; this test fails.
func TestTemplateFIPSGuardPassesValuesThrough(t *testing.T) {
	forceTemplateFIPSEnforced(t, true)
	g := fipsGuardTemplateFunc("genPrivateKey", "hint", func(...any) (any, error) { return "KEY", nil })
	out, err := g("rsa")
	require.NoError(t, err)
	assert.Equal(t, "KEY", out)
}

// TestTemplateFIPSGuardOffPassesEmptyThrough: outside FIPS 140-only mode the
// guard changes nothing, even for an empty result. sprout renders an empty
// value for a failed call in the default mode, and that stays as it was.
// Revert-proof: delete `|| !templateFIPSEnforced()` from the guard's
// condition; the empty result becomes an error and this test fails.
func TestTemplateFIPSGuardOffPassesEmptyThrough(t *testing.T) {
	forceTemplateFIPSEnforced(t, false)
	g := fipsGuardTemplateFunc("genPrivateKey", "hint", func(...any) (any, error) { return "", nil })
	out, err := g("foo")
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

// TestTemplateFIPSGuardInTheRealFilter: the guard layer is part of the
// production filter, not only of fipsGuardTemplateFuncs. With the mode forced,
// `genPrivateKey "foo"` (an unknown key type, so sprout's real function
// returns "" in any mode) fails the render through NewTemplateFilter. Runs in
// the default pass.
// Revert-proof: remove `.Funcs(fipsGuards)` from newTemplateFilter (with
// `_ = fipsGuards` so it compiles); the render succeeds with an empty value.
func TestTemplateFIPSGuardInTheRealFilter(t *testing.T) {
	forceTemplateFIPSEnforced(t, true)
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)
	out, err := filter([]byte(`{{ genPrivateKey "foo" }}`))
	require.Error(t, err, "rendered %q", out)
	assert.Contains(t, err.Error(), "genPrivateKey")
	assert.Contains(t, err.Error(), "FIPS 140-only")
}

// TestTemplateFIPSGuardNotForced (T3b): outside FIPS 140-only mode the guarded
// checksums render the published "abc" vectors (RFC 1321 for md5, FIPS 180
// for sha1), an oracle independent of sprout. The guard must also leave
// sprout's own logging alone: the deprecated names still log sprout's
// deprecation WARN, and whatever sprout logs for the bare call the guarded
// render logs too. Skips under real
// FIPS 140-only mode, where Go refuses both algorithms.
// Revert-proofs: reimplement md5sum in the guard with crypto/md5 instead of
// delegating (the logged output differs from sprout's); swap the delegate for
// one returning "" (the vectors fail).
func TestTemplateFIPSGuardNotForced(t *testing.T) {
	if fips140.Enforced() {
		t.Skip("Go refuses md5 and sha1 in FIPS 140-only mode; covered by the default-mode run")
	}
	forceTemplateFIPSEnforced(t, false)

	captureSlog := func(f func()) string {
		var buf bytes.Buffer
		orig := slog.Default()
		// No time attribute, so two captures compare equal.
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		})))
		defer slog.SetDefault(orig)
		f()
		return buf.String()
	}

	for _, tc := range []struct {
		name, want string
		deprecated bool
	}{
		{"md5sum", "900150983cd24fb0d6963f7d28e17f72", true},
		{"md5Sum", "900150983cd24fb0d6963f7d28e17f72", false},
		{"sha1sum", "a9993e364706816aba3e25717850c26c9cd0d89d", true},
		{"sha1Sum", "a9993e364706816aba3e25717850c26c9cd0d89d", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			guardedLog := captureSlog(func() {
				got = render(t, "", `{{ `+tc.name+` "abc" }}`)
			})
			assert.Equal(t, tc.want, got)
			if tc.deprecated {
				// So the comparison below is not two empty captures.
				assert.Contains(t, guardedLog, "is deprecated", "sprout's deprecation WARN for %s is missing", tc.name)
			}

			bareLog := captureSlog(func() {
				fn, ok := sprigin.TxtFuncMap()[tc.name].(sproutFunc)
				require.True(t, ok)
				out, err := fn("abc")
				require.NoError(t, err)
				assert.Equal(t, tc.want, out)
			})
			assert.Equal(t, bareLog, guardedLog, "the guard changed what sprout logs for %s", tc.name)
		})
	}
}

// TestTemplateSha256SumIsNotGuarded (T3d): sha256Sum is approved, is not in
// the guarded set, and renders under the forced mode.
// Revert-proof: add sha256Sum to fipsGuardedTemplateFuncs; the membership
// assertion fails.
func TestTemplateSha256SumIsNotGuarded(t *testing.T) {
	forceTemplateFIPSEnforced(t, true)
	_, guarded := fipsGuardedTemplateFuncs["sha256Sum"]
	assert.False(t, guarded)
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		render(t, "", `{{ sha256Sum "abc" }}`))
}

// TestTemplateFIPSProbe (T3c) is a FIPS-only test: it skips unless the process
// runs in Go's FIPS 140-only mode. Run it with
//
//	GODEBUG=fips140=only go test -count=1 -v -run TestTemplateFIPSProbe ./internal/syver/
//
// and check it reports PASS, not SKIP. It is the authority for
// fipsGuardedTemplateFuncs. sprout's values are all the same opaque closure,
// so which ones the mode silences can only be found by calling them: every
// function in sprigin.TxtFuncMap() is called with each probe tuple, once under
// enforcement and once inside fips140.WithoutEnforcement. A function that
// returns a value without enforcement and an empty one (nil or the zero value,
// see emptyTemplateValue) with it, for any tuple, must
// be guarded or accepted; every guarded name must show that difference.
//
// Functions whose names start with "must" are not probed: sprout leaves them
// unwrapped, so their errors reach text/template and fail the render already.
//
// The generic tuples cannot reach functions that need typed or several
// arguments (derivePassword, the certificate functions, buildCustomCert,
// decryptAES), so those get their own valid argument sets, built once without
// enforcement. Each such set must give a value without enforcement, or the
// probe would be blind to that function; the test fails if one does not.
//
// Revert-proofs (run in the only-mode pass): remove genPrivateKey from
// fipsGuardedTemplateFuncs; the probe fails naming it. Wrap the map's
// genSignedCert so it returns "" under enforcement; the probe fails naming
// it, through its own argument set (with only the generic tuples it stayed
// green, which is why the sets exist).
func TestTemplateFIPSProbe(t *testing.T) {
	if !fips140.Enforced() {
		t.Skip("run with GODEBUG=fips140=only go test -count=1 -v -run TestTemplateFIPSProbe ./internal/syver/")
	}
	start := time.Now()

	tuples := [][]any{
		{},
		{"abc"},
		{"abc", "abc"},
		{"dsa"},
		{"rsa"},
		{"ecdsa"},
		{"ed25519"},
	}

	// The guarded values must still be sprout's variadic shape.
	sm := sprigin.TxtFuncMap()
	extra := cryptoProbeArgs(t, sm)
	for name := range fipsGuardedTemplateFuncs {
		_, ok := sm[name].(sproutFunc)
		assert.True(t, ok, "guarded %s is %T, not func(...any) (any, error)", name, sm[name])
	}

	names := make([]string, 0, len(sm))
	for name := range sm {
		names = append(names, name)
	}
	sort.Strings(names)

	silenced := map[string][]string{}
	for _, name := range names {
		fn, ok := sm[name].(sproutFunc)
		if !ok {
			assert.True(t, strings.HasPrefix(name, "must"),
				"%s is %T: neither sprout's wrapped shape nor a must* function; the probe cannot call it", name, sm[name])
			continue
		}
		for _, args := range append(slices.Clone(tuples), extra[name]...) {
			enforced, _ := fn(args...)
			var bypassed any
			fips140.WithoutEnforcement(func() { bypassed, _ = fn(args...) })
			if !emptyTemplateValue(bypassed) && emptyTemplateValue(enforced) {
				silenced[name] = append(silenced[name], formatTuple(args))
			}
		}
	}

	for name, tuples := range silenced {
		_, guarded := fipsGuardedTemplateFuncs[name]
		_, accepted := fipsSilentTemplateFuncsAccepted[name]
		assert.True(t, guarded || accepted,
			"%s renders empty in FIPS 140-only mode for %v: add it to fipsGuardedTemplateFuncs, or to fipsSilentTemplateFuncsAccepted with a reason", name, tuples)
	}
	for name := range fipsGuardedTemplateFuncs {
		assert.Contains(t, silenced, name, "%s is guarded but no probe tuple shows the mode silencing it", name)
	}

	found := make([]string, 0, len(silenced))
	for name := range silenced {
		found = append(found, name)
	}
	slices.Sort(found)
	t.Logf("probed %d functions in %s; silenced in FIPS 140-only mode: %v", len(names), time.Since(start).Round(time.Millisecond), found)
}

func formatTuple(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprintf("%#v", a)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// cryptoProbeArgs builds valid argument sets for sprout's crypto functions
// that take typed or several arguments (signatures read from sprout's
// registry/crypto), keyed by template name. The inputs (PEM keys, a CA, a
// certificate and a ciphertext) are produced by sprout itself, inside
// fips140.WithoutEnforcement so they exist in every mode. Each set is checked
// to give a non-empty value without enforcement.
func cryptoProbeArgs(t *testing.T, sm template.FuncMap) map[string][][]any {
	t.Helper()
	call := func(name string, args ...any) any {
		t.Helper()
		fn, ok := sm[name].(sproutFunc)
		require.True(t, ok, "%s is %T", name, sm[name])
		var out any
		fips140.WithoutEnforcement(func() { out, _ = fn(args...) })
		require.False(t, emptyTemplateValue(out), "probe input: %s%s gave an empty value", name, formatTuple(args))
		return out
	}

	rsaPEM := call("genPrivateKey", "rsa")
	ecdsaPEM := call("genPrivateKey", "ecdsa")
	ca := call("genCA", "probe-ca", 365)
	caCert := reflect.ValueOf(ca)
	b64 := func(field string) string {
		return base64.StdEncoding.EncodeToString([]byte(caCert.FieldByName(field).String()))
	}
	ips := []any{"127.0.0.1"}
	dns := []any{"probe.example"}
	cipher := call("encryptAES", "password", "plaintext")

	args := map[string][][]any{
		"derivePassword":           {{1, "long", "password", "user", "example.com"}},
		"genCA":                    {{"probe", 365}},
		"genCAWithKey":             {{"probe", 365, rsaPEM}, {"probe", 365, ecdsaPEM}},
		"genSelfSignedCert":        {{"probe", ips, dns, 365}},
		"genSelfSignedCertWithKey": {{"probe", ips, dns, 365, rsaPEM}, {"probe", ips, dns, 365, ecdsaPEM}},
		"genSignedCert":            {{"probe", ips, dns, 365, ca}},
		"genSignedCertWithKey":     {{"probe", ips, dns, 365, ca, rsaPEM}, {"probe", ips, dns, 365, ca, ecdsaPEM}},
		"buildCustomCert":          {{b64("Cert"), b64("Key")}},
		"decryptAES":               {{"password", cipher}},
	}
	for name, sets := range args {
		for _, a := range sets {
			call(name, a...)
		}
	}
	return args
}

// TestTemplateFIPSOnlyRender (T4) is a FIPS-only test: it skips unless the
// process runs in Go's FIPS 140-only mode. Run it with
//
//	GODEBUG=fips140=only go test -count=1 -v -run TestTemplateFIPSOnlyRender ./internal/syver/
//
// and check it reports PASS, not SKIP. Through the production filter, a
// refused checksum and a refused key type each fail the render, naming the
// function, instead of rendering "<no value>".
// Revert-proof (run in the only-mode pass): in newTemplateFilter, apply
// sprigin.TxtFuncMap() again after .Funcs(fipsGuards), which undoes the guard
// layer; both renders then succeed with an empty value.
func TestTemplateFIPSOnlyRender(t *testing.T) {
	if !fips140.Enforced() {
		t.Skip("run with GODEBUG=fips140=only go test -count=1 -v -run TestTemplateFIPSOnlyRender ./internal/syver/")
	}
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)
	for name, src := range map[string]string{
		"md5sum":        `{{ md5sum "x" }}`,
		"genPrivateKey": `{{ genPrivateKey "dsa" }}`,
	} {
		t.Run(name, func(t *testing.T) {
			out, err := filter([]byte(src))
			require.Error(t, err, "rendered %q", out)
			assert.Contains(t, err.Error(), name)
			assert.Contains(t, err.Error(), "FIPS 140-only")
			assert.NotContains(t, string(out), "<no value>")
		})
	}
}

// TestTemplateBcryptUnderFIPSOnly is a FIPS-only test: it skips unless the
// process runs in Go's FIPS 140-only mode. Run it with
//
//	GODEBUG=fips140=only go test -count=1 -v -run TestTemplateBcryptUnderFIPSOnly ./internal/syver/
//
// and check it reports PASS, not SKIP. bcrypt and htpasswd come from
// golang.org/x/crypto/bcrypt, which Go's enforcement does not cover, and the
// docs say they run normally in that mode: both must render a bcrypt hash.
// Revert-proof (run in the only-mode pass): add bcrypt and htpasswd to
// fipsGuardedTemplateFuncs and make the guard return its error whenever
// enforced; both subtests fail.
func TestTemplateBcryptUnderFIPSOnly(t *testing.T) {
	if !fips140.Enforced() {
		t.Skip("run with GODEBUG=fips140=only go test -count=1 -v -run TestTemplateBcryptUnderFIPSOnly ./internal/syver/")
	}
	filter, err := NewTemplateFilter(nil, "", nil)
	require.NoError(t, err)
	for name, tc := range map[string]struct{ src, prefix string }{
		"bcrypt":   {`{{ bcrypt "password" }}`, "$2a$"},
		"htpasswd": {`{{ htpasswd "user" "password" }}`, "user:$2a$"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := filter([]byte(tc.src))
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(string(out), tc.prefix), "%s rendered %q", name, out)
		})
	}
}
