package resource

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestTypeNameMatchesReflectType guards the FEAT-007 §4.5 fix: validate.go
// used to derive a resource's printed type via
// strings.Split(reflect.TypeOf(res).String(), ".")[1] instead of calling
// TypeName() directly. That reflect-based derivation breaks for any
// out-of-package type (an external Phase 2 plugin would print
// "myplugin.Foo" instead of its real dispatch name), so it was switched to
// TypeName().
//
// The swap is safe only because Syverfile's Go type name ("Syverfile") and
// its TypeName() ("Gossfile") diverge and nothing currently notices: this
// test doesn't just assume that, it asserts the divergence is exactly one
// type wide, and that the one divergent type's Validate() never actually
// reaches ValidateGomegaValue (so the pre-swap and post-swap code paths
// were always equivalent in practice, not just in theory).
func TestTypeNameMatchesReflectType(t *testing.T) {
	var diverged []string

	for key, d := range Descriptors() {
		res := d.New()
		reflectName := strings.Split(reflect.TypeOf(res).String(), ".")[1]
		if reflectName != d.Name {
			diverged = append(diverged, key)
		}
	}

	sort.Strings(diverged)
	assert.DeepEqual(t, diverged, []string{"gossfile"})

	// The one divergent type must be provably inert: its Validate() always
	// returns zero results, so it never calls ValidateGomegaValue -- the
	// only place the reflect-vs-TypeName() choice was ever observable.
	sf := &Syverfile{}
	assert.Equal(t, len(sf.Validate(t.Context(), nil)), 0)
}
