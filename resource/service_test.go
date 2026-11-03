package resource

import (
	"context"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysService is a configurable system.Service for the FEAT-019 masked tests.
// It embeds conformanceService for every accessor these tests do not vary.
type fakeSysService struct {
	conformanceService
	masked    bool
	maskedErr error
}

func (f fakeSysService) Masked() (bool, error) { return f.masked, f.maskedErr }

func newFakeServiceSystem(svc system.Service) *system.System {
	return &system.System{
		NewService: func(_ context.Context, _ string, _ *system.System, _ util.Config) system.Service {
			return svc
		},
	}
}

func TestNewServiceEmitsMaskedOnlyWhenTrue(t *testing.T) {
	t.Run("masked unit emits masked: true", func(t *testing.T) {
		s, err := NewService(fakeSysService{masked: true}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, s.Masked, matcher(true))
	})
	t.Run("unmasked unit emits no masked key", func(t *testing.T) {
		s, err := NewService(fakeSysService{masked: false}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, s.Masked, nil)
	})
	t.Run("a host that cannot report masked emits no key and does not fail", func(t *testing.T) {
		s, err := NewService(fakeSysService{maskedErr: system.ErrServiceMaskedUnsupported}, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, s.Masked, nil)
	})
}

func TestServiceValidateMasked(t *testing.T) {
	cases := []struct {
		name   string
		want   bool
		sys    fakeSysService
		result int
	}{
		{"masked: true on a masked unit passes", true, fakeSysService{masked: true}, SUCCESS},
		{"masked: true on a disabled unit fails", true, fakeSysService{masked: false}, FAIL},
		{"masked: false on a disabled unit passes", false, fakeSysService{masked: false}, SUCCESS},
		{"masked: false on a masked unit fails", false, fakeSysService{masked: true}, FAIL},
		{"masked: false on an unsupported backend fails, not a false pass", false,
			fakeSysService{maskedErr: system.ErrServiceMaskedUnsupported}, FAIL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{Masked: tc.want}
			s.SetID("alsa-utils")
			results := s.Validate(t.Context(), newFakeServiceSystem(tc.sys))
			assert.Equal(t, len(results), 1)
			assert.Equal(t, results[0].Property, "masked")
			assert.Equal(t, results[0].Result, tc.result)
		})
	}

	t.Run("masked is not checked when not set", func(t *testing.T) {
		s := &Service{Enabled: true}
		s.SetID("alsa-utils")
		results := s.Validate(t.Context(), newFakeServiceSystem(fakeSysService{masked: true}))
		assert.Equal(t, len(results), 1)
		assert.Equal(t, results[0].Property, "enabled")
	})
}
