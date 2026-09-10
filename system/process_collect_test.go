package system

import (
	"errors"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/process"
)

// These cover FEAT-013 Task 3 / FEAT-011 W2-11 (SW-6): Status() returned an
// empty slice and a NIL ERROR on Windows, because gopsutil fails for every
// process there and the loop skipped them all. The check ran, found the
// process, said nothing about it, and passed.
//
// collectPerProcess is tested directly rather than through Status(), because
// the distinction being made is about how many reads failed, and driving that
// through the real process table would make it a test of the host rather than
// of the rule.

func TestCollectPerProcessToleratesSomeFailures(t *testing.T) {
	procs := []*process.Process{{Pid: 1}, {Pid: 2}, {Pid: 3}}
	calls := 0
	got, err := collectPerProcess(procs, "status", func(p *process.Process) ([]string, error) {
		calls++
		if p.Pid == 2 {
			return nil, errors.New("process vanished")
		}
		return []string{"running"}, nil
	})
	if err != nil {
		t.Fatalf("one failure out of three returned %v, want it tolerated", err)
	}
	if calls != 3 {
		t.Errorf("read called %d times, want 3", calls)
	}
	if len(got) != 1 || got[0] != "running" {
		t.Errorf("got %v, want the deduplicated results of the successful reads", got)
	}
}

func TestCollectPerProcessFailsWhenEveryReadFails(t *testing.T) {
	procs := []*process.Process{{Pid: 1}, {Pid: 2}}
	sentinel := errors.New("not implemented yet")
	got, err := collectPerProcess(procs, "status", func(*process.Process) ([]string, error) {
		return nil, sentinel
	})
	if err == nil {
		t.Fatalf("all reads failed but collectPerProcess returned %v with a nil error", got)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap the underlying cause; the operator needs to see why", err)
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error %q does not name the attribute that failed", err)
	}
	if got != nil {
		t.Errorf("got %v alongside the error, want nil", got)
	}
}

// TestCollectPerProcessCannotTellARaceFromSystemicAtOne pins the boundary the
// all-or-nothing rule cannot express, found by review rather than by design.
//
// With exactly one matching process -- the common case for a single-instance
// daemon -- "every read failed" and "the one read raced" are the same event.
// The transient failure the rule claims to tolerate is therefore NOT tolerated
// at n=1. That is accepted: the alternative is an empty result with a nil error
// for a process that demonstrably exists, which is the silent-nothing outcome
// this function exists to remove.
//
// This test exists so the boundary is stated rather than discovered. If a
// future change makes n=1 tolerant again, it fails and forces the choice to be
// deliberate.
func TestCollectPerProcessCannotTellARaceFromSystemicAtOne(t *testing.T) {
	race := errors.New("process vanished between listing and reading")
	got, err := collectPerProcess([]*process.Process{{Pid: 1}}, "status",
		func(*process.Process) ([]string, error) { return nil, race })

	if err == nil {
		t.Fatalf("n=1 with a racing read returned %v and a nil error; that is the "+
			"silent-nothing outcome, not tolerance", got)
	}
	if !errors.Is(err, race) {
		t.Errorf("error %v does not wrap the underlying cause", err)
	}
	if !strings.Contains(err.Error(), "all 1 matching processes") {
		t.Errorf("error %q should say how many processes it was speaking for", err)
	}
}

// An executable with no matching processes is not a failure. `Exists()` is what
// answers that question, and making Status() error here would turn an ordinary
// absent-process assertion into a hard failure.
func TestCollectPerProcessAcceptsNoProcesses(t *testing.T) {
	got, err := collectPerProcess(nil, "status", func(*process.Process) ([]string, error) {
		t.Fatal("read must not be called when there are no processes")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("no matching processes returned %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
