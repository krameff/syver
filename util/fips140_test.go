package util

import (
	"crypto/fips140"
	"encoding/json"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFIPS140StateFrom drives the mode and module rules with the inputs each
// kind of build and GODEBUG setting produces. Inputs measured on go1.27 for
// each GOFIPS140 value: fips140.Version() is "latest" for a plain build and
// for GOFIPS140=latest, "v1.0.0" for v1.0.0, "v1.26.0" for inprocess; the
// GOFIPS140 build setting is absent on a plain build.
//
// Revert-proofs, one mutation each:
//   - report fips140.Version() as Module whatever the build setting says: the
//     plain and latest rows fail (module=latest);
//   - check enabled before enforced: the "enabled and enforced" row fails
//     (mode=on);
//   - accept any GOFIPS140 value, not only one starting "v": the latest row
//     fails (module=latest);
//   - omit module-id from String(): the frozen-module rows fail.
func TestFIPS140StateFrom(t *testing.T) {
	fipsSetting := func(v string) []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "-trimpath", Value: "true"}, {Key: "GOFIPS140", Value: v}}
	}
	plain := []debug.BuildSetting{{Key: "-trimpath", Value: "true"}, {Key: "CGO_ENABLED", Value: "0"}}

	for _, tc := range []struct {
		name              string
		enabled, enforced bool
		version           string
		settings          []debug.BuildSetting
		want              string
	}{
		{"plain build", false, false, "latest", plain, "mode=off module=none"},
		{"plain build, no build info", false, false, "latest", nil, "mode=off module=none"},
		{"plain build, GODEBUG=fips140=on", true, false, "latest", plain, "mode=on module=none"},
		// GODEBUG=fips140=debug sets the same two inputs as on.
		{"plain build, GODEBUG=fips140=debug", true, false, "latest", plain, "mode=on module=none"},
		{"plain build, enforced", false, true, "latest", plain, "mode=only module=none"},
		{"plain build, enabled and enforced", true, true, "latest", plain, "mode=only module=none"},
		{"GOFIPS140=v1.0.0", true, false, "v1.0.0", fipsSetting("v1.0.0-c2097c7c"), "mode=on module=v1.0.0 module-id=v1.0.0-c2097c7c"},
		{"GOFIPS140=v1.0.0, GODEBUG=fips140=off", false, false, "v1.0.0", fipsSetting("v1.0.0-c2097c7c"), "mode=off module=v1.0.0 module-id=v1.0.0-c2097c7c"},
		{"GOFIPS140=inprocess", true, false, "v1.26.0", fipsSetting("v1.26.0"), "mode=on module=v1.26.0 module-id=v1.26.0"},
		{"GOFIPS140=latest", true, false, "latest", fipsSetting("latest"), "mode=on module=none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := fips140StateFrom(tc.enabled, tc.enforced, tc.version, tc.settings)
			assert.Equal(t, tc.want, got.String())
		})
	}
}

// TestFIPS140StateJSON pins the JSON keys, which are hyphenated like syver's
// other JSON output, and that module-id is omitted when no module is linked.
// Revert-proof: drop omitempty from ModuleID; the "none" case gains
// "module-id":"".
func TestFIPS140StateJSON(t *testing.T) {
	b, err := json.Marshal(FIPS140State{Mode: "on", Module: "v1.26.0", ModuleID: "v1.26.0"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"mode":"on","module":"v1.26.0","module-id":"v1.26.0"}`, string(b))

	b, err = json.Marshal(FIPS140State{Mode: "off", Module: "none"})
	require.NoError(t, err)
	assert.Equal(t, `{"mode":"off","module":"none"}`, string(b))
}

// TestFIPS140MatchesTheRunningProcess checks FIPS140() against the real
// crypto/fips140 calls, so it holds in the default pass and in each FIPS pass.
// Revert-proof: hard-code Mode "off" in FIPS140(); the GODEBUG=fips140=on and
// only passes fail.
func TestFIPS140MatchesTheRunningProcess(t *testing.T) {
	got := FIPS140()
	want := "off"
	switch {
	case fips140.Enforced():
		want = "only"
	case fips140.Enabled():
		want = "on"
	}
	assert.Equal(t, want, got.Mode)
	if got.Module == "none" {
		assert.Empty(t, got.ModuleID)
	} else {
		assert.Equal(t, fips140.Version(), got.Module)
	}
}
