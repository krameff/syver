package util

import (
	"crypto/fips140"
	"runtime/debug"
	"strings"
)

// FIPS140State reports how this syver binary uses the Go Cryptographic
// Module: whether a frozen, validated module version was linked at build time,
// and which FIPS 140-3 mode the process is running in. It describes the syver
// process, not the host: a host in FIPS mode does not put syver in FIPS mode.
type FIPS140State struct {
	// Mode is "off", "on" or "only". GODEBUG=fips140=debug reports "on".
	Mode string `json:"mode"`
	// Module is the frozen Go Cryptographic Module version linked at build
	// time (e.g. "v1.26.0"), or "none" when the binary uses the standard
	// library's unfrozen copy.
	Module string `json:"module"`
	// ModuleID is the exact GOFIPS140 build setting (e.g. "v1.0.0-c2097c7c"),
	// empty when Module is "none".
	ModuleID string `json:"module-id,omitempty"`
}

// FIPS140 returns the state of the running process.
func FIPS140() FIPS140State {
	var settings []debug.BuildSetting
	if bi, ok := debug.ReadBuildInfo(); ok {
		settings = bi.Settings
	}
	return fips140StateFrom(fips140.Enabled(), fips140.Enforced(), fips140.Version(), settings)
}

// fips140StateFrom is FIPS140's logic, fed its inputs so the tests can drive
// every combination of mode and build.
//
// fips140.Version cannot say "none": it returns "latest" both for a plain
// build and for GOFIPS140=latest. Only the GOFIPS140 build setting shows that
// a frozen module was linked, and it is present only on a GOFIPS140 build.
// A frozen module's setting starts with "v" ("v1.0.0-c2097c7c", "v1.26.0");
// "latest" is the unfrozen copy and so reports "none".
func fips140StateFrom(enabled, enforced bool, version string, settings []debug.BuildSetting) FIPS140State {
	s := FIPS140State{Mode: "off", Module: "none"}
	switch {
	case enforced:
		s.Mode = "only"
	case enabled:
		s.Mode = "on"
	}
	for _, bs := range settings {
		if bs.Key == "GOFIPS140" && strings.HasPrefix(bs.Value, "v") {
			s.Module = version
			s.ModuleID = bs.Value
		}
	}
	return s
}

// String renders the state as space-separated key=value pairs in a fixed
// order: "mode=on module=v1.0.0 module-id=v1.0.0-c2097c7c", or
// "mode=off module=none".
func (s FIPS140State) String() string {
	out := "mode=" + s.Mode + " module=" + s.Module
	if s.ModuleID != "" {
		out += " module-id=" + s.ModuleID
	}
	return out
}
