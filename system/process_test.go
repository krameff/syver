package system

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/process"
	"gotest.tools/v3/assert"

	"github.com/krameff/syver/util"
)

// fakeProcesses stubs out listProcesses/processName so tests never touch the
// real OS process table (gopsutil's *process.Process.Name() always does real
// /proc I/O once called, so it can't be faked after construction).
func fakeProcesses(t *testing.T, procs []*process.Process, names map[*process.Process]string, listErr error) {
	t.Helper()
	origList := listProcesses
	origName := processName
	listProcesses = func() ([]*process.Process, error) {
		return procs, listErr
	}
	processName = func(p *process.Process) (string, error) {
		if name, ok := names[p]; ok {
			return name, nil
		}
		return "", errors.New("no such process")
	}
	t.Cleanup(func() {
		listProcesses = origList
		processName = origName
	})
}

func TestGetProcs(t *testing.T) {
	t.Run("empty process list", func(t *testing.T) {
		fakeProcesses(t, nil, nil, nil)
		pmap, err := GetProcs()
		assert.NilError(t, err)
		assert.Equal(t, len(pmap), 0)
	})

	t.Run("error propagation from underlying lister", func(t *testing.T) {
		listErr := errors.New("boom")
		fakeProcesses(t, nil, nil, listErr)
		_, err := GetProcs()
		assert.ErrorIs(t, err, listErr)
	})

	t.Run("multiple pids for the same executable name", func(t *testing.T) {
		p1 := &process.Process{Pid: 100}
		p2 := &process.Process{Pid: 200}
		p3 := &process.Process{Pid: 300}
		fakeProcesses(t, []*process.Process{p1, p2, p3}, map[*process.Process]string{
			p1: "sshd",
			p2: "sshd",
			p3: "nginx",
		}, nil)

		pmap, err := GetProcs()
		assert.NilError(t, err)
		assert.Equal(t, len(pmap["sshd"]), 2)
		assert.Equal(t, len(pmap["nginx"]), 1)
	})

	t.Run("a process that disappears (errors on name) is skipped, not fatal", func(t *testing.T) {
		gone := &process.Process{Pid: 1}
		alive := &process.Process{Pid: 2}
		fakeProcesses(t, []*process.Process{gone, alive}, map[*process.Process]string{
			alive: "cron",
		}, nil)

		pmap, err := GetProcs()
		assert.NilError(t, err)
		assert.Equal(t, len(pmap), 1)
		assert.Equal(t, len(pmap["cron"]), 1)
	})
}

func TestDefProcessRunning(t *testing.T) {
	tests := []struct {
		name       string
		executable string
		procMap    map[string][]*process.Process
		err        error
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "process found",
			executable: "sshd",
			procMap:    map[string][]*process.Process{"sshd": {{Pid: 1}}},
			wantExists: true,
		},
		{
			name:       "process not found",
			executable: "does-not-exist",
			procMap:    map[string][]*process.Process{"sshd": {{Pid: 1}}},
			wantExists: false,
		},
		{
			name:       "underlying error propagates",
			executable: "sshd",
			procMap:    nil,
			err:        errors.New("boom"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &DefProcess{executable: tt.executable, procMap: tt.procMap, err: tt.err}

			running, err := p.Running()
			if tt.wantErr {
				assert.ErrorContains(t, err, "boom")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, running, tt.wantExists)

			exists, err := p.Exists()
			assert.NilError(t, err)
			assert.Equal(t, exists, tt.wantExists)
		})
	}
}

// fakeProcessStatusUser stubs out processStatus/processUser so Status()/User()
// tests never touch the real OS (gopsutil's Status()/Username() always do
// real /proc I/O once called, so they can't be faked after construction).
func fakeProcessStatusUser(t *testing.T, statuses map[*process.Process][]string, statusErrs map[*process.Process]error, users map[*process.Process]string, userErrs map[*process.Process]error) {
	t.Helper()
	origStatus := processStatus
	origUser := processUser
	processStatus = func(p *process.Process) ([]string, error) {
		if err, ok := statusErrs[p]; ok {
			return nil, err
		}
		return statuses[p], nil
	}
	processUser = func(p *process.Process) (string, error) {
		if err, ok := userErrs[p]; ok {
			return "", err
		}
		return users[p], nil
	}
	t.Cleanup(func() {
		processStatus = origStatus
		processUser = origUser
	})
}

func TestDefProcessStatus(t *testing.T) {
	t.Run("aggregates and dedupes statuses across all matching pids", func(t *testing.T) {
		p1 := &process.Process{Pid: 1}
		p2 := &process.Process{Pid: 2}
		fakeProcessStatusUser(t, map[*process.Process][]string{
			p1: {"running"},
			p2: {"zombie"},
		}, nil, nil, nil)

		proc := &DefProcess{
			executable: "sshd",
			procMap:    map[string][]*process.Process{"sshd": {p1, p2}},
		}
		statuses, err := proc.Status()
		assert.NilError(t, err)
		assert.DeepEqual(t, statuses, []string{"running", "zombie"})
	})

	t.Run("a pid that errors on status is skipped, not fatal", func(t *testing.T) {
		gone := &process.Process{Pid: 1}
		alive := &process.Process{Pid: 2}
		fakeProcessStatusUser(t, map[*process.Process][]string{
			alive: {"running"},
		}, map[*process.Process]error{
			gone: errors.New("no such process"),
		}, nil, nil)

		proc := &DefProcess{
			executable: "cron",
			procMap:    map[string][]*process.Process{"cron": {gone, alive}},
		}
		statuses, err := proc.Status()
		assert.NilError(t, err)
		assert.DeepEqual(t, statuses, []string{"running"})
	})

	t.Run("underlying error propagates", func(t *testing.T) {
		p := &DefProcess{executable: "sshd", err: errors.New("boom")}
		_, err := p.Status()
		assert.ErrorContains(t, err, "boom")
	})
}

func TestDefProcessUser(t *testing.T) {
	t.Run("aggregates and dedupes usernames across all matching pids", func(t *testing.T) {
		p1 := &process.Process{Pid: 1}
		p2 := &process.Process{Pid: 2}
		p3 := &process.Process{Pid: 3}
		fakeProcessStatusUser(t, nil, nil, map[*process.Process]string{
			p1: "root",
			p2: "root",
			p3: "www-data",
		}, nil)

		proc := &DefProcess{
			executable: "nginx",
			procMap:    map[string][]*process.Process{"nginx": {p1, p2, p3}},
		}
		users, err := proc.User()
		assert.NilError(t, err)
		assert.DeepEqual(t, users, []string{"root", "www-data"})
	})

	t.Run("underlying error propagates", func(t *testing.T) {
		p := &DefProcess{executable: "sshd", err: errors.New("boom")}
		_, err := p.User()
		assert.ErrorContains(t, err, "boom")
	})
}

func TestDefProcessPids(t *testing.T) {
	t.Run("returns all pids for the executable, not just one", func(t *testing.T) {
		p := &DefProcess{
			executable: "sshd",
			procMap: map[string][]*process.Process{
				"sshd": {{Pid: 100}, {Pid: 200}, {Pid: 300}},
			},
		}
		pids, err := p.Pids()
		assert.NilError(t, err)
		assert.DeepEqual(t, pids, []int{100, 200, 300})
	})

	t.Run("no pids for an unknown executable", func(t *testing.T) {
		p := &DefProcess{executable: "nope", procMap: map[string][]*process.Process{}}
		pids, err := p.Pids()
		assert.NilError(t, err)
		assert.Equal(t, len(pids), 0)
	})

	t.Run("underlying error propagates", func(t *testing.T) {
		p := &DefProcess{executable: "sshd", err: errors.New("boom")}
		_, err := p.Pids()
		assert.ErrorContains(t, err, "boom")
	})
}

// A failed process snapshot must reach every caller, not only the one that ran
// the sync.Once. Before the error was stored on System, the second and later
// callers (every other worker in validateParallel) got a nil map and a nil
// error, so `process: running: false` passed on a host where listing failed.
func TestProcMapErrorReachesEveryCaller(t *testing.T) {
	listErr := errors.New("process table unreadable")
	fakeProcesses(t, nil, nil, listErr)
	sys := &System{}

	for i := 1; i <= 3; i++ {
		_, err := sys.ProcMap()
		assert.ErrorIs(t, err, listErr, "call %d", i)
	}

	// Through the resource constructor, as a second process: entry would be.
	_ = NewDefProcess(context.Background(), "first", sys, util.Config{})
	second := NewDefProcess(context.Background(), "second", sys, util.Config{})
	running, err := second.Running()
	assert.ErrorIs(t, err, listErr)
	assert.Equal(t, running, false)
}
