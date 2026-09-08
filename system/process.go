package system

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/krameff/syver/util"
)

type Process interface {
	Executable() string
	Exists() (bool, error)
	Running() (bool, error)
	Pids() ([]int, error)
	Status() ([]string, error)
	User() ([]string, error)
}

type DefProcess struct {
	executable string
	procMap    map[string][]*process.Process
	err        error
}

func NewDefProcess(_ context.Context, executable string, system *System, config util.Config) Process {
	pmap, err := system.ProcMap()
	return &DefProcess{
		executable: executable,
		procMap:    pmap,
		err:        err,
	}
}

func (p *DefProcess) Executable() string {
	return p.executable
}

func (p *DefProcess) Exists() (bool, error) { return p.Running() }

func (p *DefProcess) Pids() ([]int, error) {
	var pids []int
	if p.err != nil {
		return pids, p.err
	}
	for _, proc := range p.procMap[p.executable] {
		pids = append(pids, int(proc.Pid))
	}
	return pids, nil
}

func (p *DefProcess) Running() (bool, error) {
	if p.err != nil {
		return false, p.err
	}
	if _, ok := p.procMap[p.executable]; ok {
		return true, nil
	}
	return false, nil
}

// Status returns the distinct process states (e.g. "running", "zombie") seen
// across every PID matching this executable. A process that disappears or
// can't be read between the snapshot and this call is skipped rather than
// failing the whole result, same as GetProcs -- except when every one of them
// fails. See collectPerProcess.
func (p *DefProcess) Status() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	return collectPerProcess(p.procMap[p.executable], "status", processStatus)
}

// User returns the distinct usernames owning every PID matching this
// executable. Same all-or-nothing rule as Status; see collectPerProcess.
func (p *DefProcess) User() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	return collectPerProcess(p.procMap[p.executable], "user", func(proc *process.Process) ([]string, error) {
		u, err := processUser(proc)
		if err != nil {
			return nil, err
		}
		return []string{u}, nil
	})
}

// collectPerProcess gathers one attribute across every PID matching an
// executable, tolerating individual failures but not universal ones.
//
// Skipping a single failure is deliberate and predates this: a process can
// exit between the snapshot and the read, and one such race should not fail an
// assertion about the others.
//
// Failing when EVERY read fails is FEAT-013 / FEAT-011 W2-11 (SW-6). On
// Windows gopsutil returns a not-implemented error for `status` on every
// process, so the loop skipped all of them and returned an empty slice with a
// NIL ERROR. The check ran, found the process, reported nothing about it and
// passed -- the silent-wrongness class FEAT-010 exists to remove, and
// indistinguishable from an honest empty result unless the failures are
// counted.
//
// The rule is deliberately "all of them" rather than "any of them": a systemic
// failure is universal, whereas the race this tolerates is not. It needs no
// platform check and no gopsutil sentinel, which matters because gopsutil's
// ErrNotImplementedError lives in an internal package and cannot be compared
// against from here.
//
// User() gets the same treatment as Status(), not because it is known broken
// anywhere, but because the shape was identical and leaving one of two
// adjacent silent-pass paths in place is an arbitrary line to draw.
func collectPerProcess(procs []*process.Process, attr string, read func(*process.Process) ([]string, error)) ([]string, error) {
	var out []string
	var lastErr error
	failed := 0
	for _, proc := range procs {
		v, err := read(proc)
		if err != nil {
			failed++
			lastErr = err
			continue
		}
		out = append(out, v...)
	}
	if len(procs) > 0 && failed == len(procs) {
		return nil, fmt.Errorf("reading %s failed for all %d matching processes: %w", attr, len(procs), lastErr)
	}
	return lo.Uniq(out), nil
}

// listProcesses, processName, processStatus, and processUser are indirected
// through package-level vars so tests can substitute canned data without
// touching the real OS process table (gopsutil's *process.Process methods
// always do real /proc I/O, so they can't be faked once constructed).
var listProcesses = process.Processes

var processName = func(p *process.Process) (string, error) {
	return p.Name()
}

var processStatus = func(p *process.Process) ([]string, error) {
	return p.Status()
}

var processUser = func(p *process.Process) (string, error) {
	return p.Username()
}

func GetProcs() (map[string][]*process.Process, error) {
	pmap := make(map[string][]*process.Process)
	processes, err := listProcesses()
	if err != nil {
		return pmap, err
	}
	for _, p := range processes {
		// A process can legitimately disappear between listing and reading
		// its name (it exited), or its /proc entry may be transiently
		// unreadable. Skip it rather than failing the whole snapshot,
		// mirroring go-ps's behavior of silently ignoring per-process
		// read errors.
		name, err := processName(p)
		if err != nil {
			continue
		}
		pmap[name] = append(pmap[name], p)
	}

	return pmap, nil
}
