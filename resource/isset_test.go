package resource

import (
	"context"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// isSet is the single guard every optional attribute uses. These tests pin
// the two things that make it correct: an empty list is not an expectation
// (it is what `syver add`/`autoadd` emit for "nothing to assert"), and a
// zero value very much is -- `enabled: false` and `uid: 0` must still be
// checked.
func TestIsSet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"absent attribute", nil, false},
		{"empty list", []interface{}{}, false},
		{"nil-typed empty list", []interface{}(nil), false},
		{"populated list", []interface{}{"x"}, true},
		{"list holding an empty string", []interface{}{""}, true},
		{"false", false, true},
		{"zero", 0, true},
		{"empty string", "", true},
		{"empty matcher map", map[string]interface{}{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, isSet(tc.value), tc.want)
		})
	}
}

// The guard has to behave identically on every resource. Before this was
// made uniform, command.stdout/file.contents/http.body dropped an empty
// list while port.ip, user.groups, service.runlevels and the rest asserted
// it, producing a vacuous pass that inflated the count.
func TestEmptyListIsNotAnExpectation(t *testing.T) {
	newFakePort := func(sysPort system.Port) *system.System {
		return &system.System{
			NewPort: func(_ context.Context, _ string, _ *system.System, _ util.Config) system.Port {
				return sysPort
			},
		}
	}

	t.Run("an empty list produces no result for that attribute", func(t *testing.T) {
		p := &Port{Listening: true, IP: []interface{}{}, PID: []interface{}{}}
		results := p.Validate(newFakePort(&fakeSysPort{listening: true}))

		// Only the mandatory attribute reports.
		assert.Equal(t, len(results), 1)
		assert.Equal(t, results[0].Property, "listening")
	})

	t.Run("a populated list still produces a result", func(t *testing.T) {
		p := &Port{Listening: true, IP: []interface{}{"127.0.0.1"}}
		results := p.Validate(newFakePort(&fakeSysPort{listening: true, ip: []string{"127.0.0.1"}}))

		assert.Equal(t, len(results), 2)
		assert.Equal(t, results[1].Property, "ip")
	})

	t.Run("the mandatory attribute always reports, empty list or not", func(t *testing.T) {
		p := &Port{Listening: []interface{}{}}
		results := p.Validate(newFakePort(&fakeSysPort{listening: true}))

		assert.Equal(t, len(results), 1)
		assert.Equal(t, results[0].Property, "listening")
	})
}
