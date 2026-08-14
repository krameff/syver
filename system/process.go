package system

import (
	"context"

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
// failing the whole result, same as GetProcs.
func (p *DefProcess) Status() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	var statuses []string
	for _, proc := range p.procMap[p.executable] {
		s, err := processStatus(proc)
		if err != nil {
			continue
		}
		statuses = append(statuses, s...)
	}
	return lo.Uniq(statuses), nil
}

// User returns the distinct usernames owning every PID matching this
// executable.
func (p *DefProcess) User() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	var users []string
	for _, proc := range p.procMap[p.executable] {
		u, err := processUser(proc)
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	return lo.Uniq(users), nil
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
