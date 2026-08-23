package resource

import (
	"reflect"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestConformance is FEAT-007's T-1: a single suite, driven by
// Descriptors(), that gives every registered type real coverage instead of
// just the 2 (port, process) that had any Go unit test before this spec.
// Each registered type gets the same set of generic assertions run against
// it -- these can't catch everything a hand-written test would, but they
// catch the class of bug this refactor is specifically worried about: a
// type wired inconsistently into the registry (mismatched Key/Name, a New()
// that isn't actually fresh, a SetSkip() that's a no-op -- see G2 -- an
// attribute that isn't isSet-guarded).
func TestConformance(t *testing.T) {
	matcherType := reflect.TypeOf((*matcher)(nil)).Elem()
	sys := newConformanceSystem()

	for key, d := range Descriptors() {
		t.Run(key, func(t *testing.T) {
			// Key/Name non-empty and consistent with TypeKey()/TypeName().
			assert.Assert(t, d.Key != "", "descriptor Key is empty")
			assert.Assert(t, d.Name != "", "descriptor Name is empty")

			res := d.New()
			assert.Equal(t, res.TypeKey(), d.Key)
			assert.Equal(t, res.TypeName(), d.Name)

			// New() returns a distinct, non-nil pointer.
			res2 := d.New()
			v1, v2 := reflect.ValueOf(res), reflect.ValueOf(res2)
			assert.Assert(t, !v1.IsNil(), "New() returned a nil pointer")
			assert.Assert(t, !v2.IsNil(), "New() returned a nil pointer")
			assert.Assert(t, v1.Pointer() != v2.Pointer(), "New() returned the same pointer twice -- not a fresh instance")

			// SetID -> ID() round-trips.
			rr, ok := res.(ResourceRead)
			assert.Assert(t, ok, "%s does not implement ResourceRead", d.Name)
			const wantID = "conformance-test-id"
			res.SetID(wantID)
			assert.Equal(t, rr.ID(), wantID)

			// SetSkip() makes Validate produce only SKIP results.
			skipped := d.New()
			skipped.SetSkip()
			for _, r := range skipped.Validate(t.Context(), sys) {
				assert.Equal(t, r.Result, SKIP, "property %q: SetSkip() did not skip it", r.Property)
			}

			// The mandatory attribute always yields >=1 result on a
			// completely zero-value resource, for every type except two
			// verified (not assumed) exceptions: gossfile's Validate() is
			// deliberately always empty (syverfile.go), and service is
			// structurally different from every other type -- "enabled"
			// and "running" are both isSet-guarded (service.go), so it has
			// no single unconditional attribute at all. Pre-existing,
			// correct behaviour; not something FEAT-007 changes.
			baseline := d.New()
			baseResults := baseline.Validate(t.Context(), sys)
			if d.Key != "gossfile" && d.Key != "service" {
				assert.Assert(t, len(baseResults) >= 1, "zero-value %s produced no results -- mandatory attribute isn't unconditional", d.Name)
			}
			baseCount := len(baseResults)

			// Every optional (matcher-typed) attribute is isSet-guarded:
			// setting it to an empty list must not change the result
			// count. Generalises isset_test.go's
			// TestEmptyListIsNotAnExpectation, which only asserted this
			// against Port.
			typ := reflect.TypeOf(res).Elem()
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Type != matcherType {
					continue
				}
				fresh := d.New()
				fv := reflect.ValueOf(fresh).Elem().FieldByIndex(f.Index)
				if !fv.CanSet() {
					continue
				}
				fv.Set(reflect.ValueOf([]any{}))
				results := fresh.Validate(t.Context(), sys)
				assert.Equal(t, len(results), baseCount, "field %s: setting to an empty list changed the result count -- not isSet-guarded", f.Name)
			}

			// Every system getter matches one of the 6 signatures
			// ValidateGomegaValue accepts (validate.go's type switch): set
			// every optional attribute to a non-empty value so its
			// ValidateValue call actually executes, then check no result
			// carries the type switch's default-case error. A getter with
			// an unsupported signature fails this way, not by panicking.
			full := d.New()
			fv := reflect.ValueOf(full).Elem()
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Type != matcherType {
					continue
				}
				fv.FieldByIndex(f.Index).Set(reflect.ValueOf([]any{"conformance-value"}))
			}
			for _, r := range full.Validate(t.Context(), sys) {
				if r.Err != nil {
					assert.Assert(t, !strings.Contains(string(*r.Err), "unknown method signature"),
						"property %q: system getter has an unsupported signature: %v", r.Property, *r.Err)
				}
			}
		})
	}
}
