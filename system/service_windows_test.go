//go:build windows
// +build windows

package system

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// FEAT-014. These run only on Windows and exercise the Service Control Manager
// path that replaced the PowerShell probes. They assert against services that
// exist on any Windows rather than on how this host happens to be configured:
// service STATE is a property of how a machine was built, which is why
// integration-tests/syver/windows/tests/service.goss.yaml has no negative case.
func newTestService(name string) *ServiceWindows {
	return &ServiceWindows{ctx: context.Background(), service: name}
}

// A name containing a SPACE is the regression this replaces. The old
// implementation interpolated the name into a PowerShell command line, where
// `-Name Windows Event Log` becomes three arguments and the probe output is
// unparseable -- so a spaced name could not work at all. Through the SCM the name
// is a UTF-16 string argument, so it reaches the API intact and Windows answers
// the question that was actually asked.
//
// Asserted as (false, nil) rather than an error: no service on a stock Windows has
// a space in its KEY name, so the correct answer here is "no such service", and
// that is precisely what distinguishes this implementation from the old one. An
// error would mean the name never got through cleanly.
func TestServiceNameWithASpaceReachesTheSCM(t *testing.T) {
	exists, err := newTestService("Windows Event Log").Exists()
	if err != nil {
		t.Fatalf("Exists() on a spaced name returned an error, so the name did not reach the SCM intact: %v", err)
	}
	if exists {
		t.Errorf("Exists() = true for a display name used as a key name; the SCM keys on the short name")
	}
}

// The three-way distinction FEAT-010 introduced and FEAT-014 had to preserve:
// present, genuinely absent, or the lookup failed. Only the middle one is
// (false, nil), and a typo must never be reported as "exists but disabled".
func TestServiceExistsDistinguishesAbsentFromFailure(t *testing.T) {
	t.Run("a real service exists", func(t *testing.T) {
		exists, err := newTestService("EventLog").Exists()
		if err != nil {
			t.Fatalf("Exists(EventLog) error = %v, want nil", err)
		}
		if !exists {
			t.Error("Exists(EventLog) = false; EventLog is present on every Windows")
		}
	})

	t.Run("an absent service is (false, nil), not an error", func(t *testing.T) {
		exists, err := newTestService("syver-no-such-service-ffffffff").Exists()
		if err != nil {
			t.Fatalf("Exists() on an absent name error = %v, want nil", err)
		}
		if exists {
			t.Error("Exists() = true for a name that cannot exist")
		}
	})

	t.Run("Enabled on an absent service reports not-found, never false", func(t *testing.T) {
		// This is the FEAT-010 defect that made `service: TypoedName: {enabled:
		// false}` PASS. It must stay an error.
		_, err := newTestService("syver-no-such-service-ffffffff").Enabled()
		if !errors.Is(err, ErrServiceNotFound) {
			t.Errorf("Enabled() on an absent service error = %v, want ErrServiceNotFound", err)
		}
	})

	t.Run("Running on an absent service reports not-found", func(t *testing.T) {
		_, err := newTestService("syver-no-such-service-ffffffff").Running()
		if !errors.Is(err, ErrServiceNotFound) {
			t.Errorf("Running() on an absent service error = %v, want ErrServiceNotFound", err)
		}
	})
}

// The six new attributes against EventLog, whose configuration is stable across
// Windows builds. Values cross-checked against `sc.exe qc EventLog`.
func TestServiceWindowsAttributes(t *testing.T) {
	s := newTestService("EventLog")

	startType, err := s.StartType()
	if err != nil {
		t.Fatalf("StartType() error = %v", err)
	}
	if startType != "automatic" {
		t.Errorf("StartType() = %q, want %q (sc.exe qc reports START_TYPE 2 AUTO_START)", startType, "automatic")
	}

	// enabled is derived from the same field, and FEAT-014 widened it to include
	// boot and system. automatic must still be enabled.
	enabled, err := s.Enabled()
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if !enabled {
		t.Error("Enabled() = false for an automatic service")
	}

	runAs, err := s.RunAs()
	if err != nil {
		t.Fatalf("RunAs() error = %v", err)
	}
	// Not compared case-sensitively to a literal: the SCM stores this verbatim and
	// spellings vary by service (Winmgmt reports "localSystem" with a lowercase
	// l). The claim worth pinning is that a real account came back.
	if !strings.Contains(strings.ToLower(runAs), "localservice") {
		t.Errorf("RunAs() = %q, want the LocalService account", runAs)
	}

	displayName, err := s.DisplayName()
	if err != nil {
		t.Fatalf("DisplayName() error = %v", err)
	}
	if displayName == "" {
		t.Error("DisplayName() returned empty with a nil error")
	}

	// Running, so it has a process. Asserted as a range because a pid changes at
	// every restart -- the same reason `syver add service` does not emit one.
	running, err := s.Running()
	if err != nil {
		t.Fatalf("Running() error = %v", err)
	}
	pid, err := s.Pid()
	if err != nil {
		t.Fatalf("Pid() error = %v", err)
	}
	if running && pid <= 0 {
		t.Errorf("Pid() = %d for a running service, want > 0", pid)
	}

	if _, err := s.DelayedStart(); err != nil {
		t.Errorf("DelayedStart() error = %v", err)
	}
}

// Dependencies is a MULTI_SZ: consecutive null-terminated UTF-16 strings with a
// trailing null, walked with pointer arithmetic. That walk is the riskiest code in
// this file, so it is exercised against a service with a REAL multi-element list
// rather than only against one with none. Cross-checked: `sc.exe qc Dnscache`
// reports nsi and Afd.
func TestServiceDependenciesParsesMultiSz(t *testing.T) {
	deps, err := newTestService("Dnscache").Dependencies()
	if err != nil {
		t.Fatalf("Dependencies(Dnscache) error = %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("Dependencies(Dnscache) returned nothing; sc.exe qc reports at least nsi")
	}
	for _, d := range deps {
		if d == "" {
			t.Errorf("Dependencies() contains an empty string, so the MULTI_SZ walk mis-stepped: %q", deps)
		}
	}

	// A service with NO dependencies must return an empty list, not a one-element
	// list containing "". EventLog has none.
	none, err := newTestService("EventLog").Dependencies()
	if err != nil {
		t.Fatalf("Dependencies(EventLog) error = %v", err)
	}
	if len(none) != 0 {
		t.Errorf("Dependencies(EventLog) = %q, want empty", none)
	}
}

// RunLevels stays unsupported. FEAT-014 deliberately did not change it: Windows
// has no runlevel concept and inventing one would be a fabricated answer.
func TestServiceRunLevelsStillUnsupported(t *testing.T) {
	if _, err := newTestService("EventLog").RunLevels(); !errors.Is(err, ErrServiceRunLevelsUnsupported) {
		t.Errorf("RunLevels() error = %v, want ErrServiceRunLevelsUnsupported", err)
	}
}
