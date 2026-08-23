package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysProcess is a minimal system.Process fake so resource-level tests
// don't need a real OS process table.
type fakeSysProcess struct {
	executable string
	exists     bool
	running    bool
	pids       []int
	status     []string
	user       []string
	err        error
}

func (f *fakeSysProcess) Executable() string        { return f.executable }
func (f *fakeSysProcess) Exists() (bool, error)     { return f.exists, f.err }
func (f *fakeSysProcess) Running() (bool, error)    { return f.running, f.err }
func (f *fakeSysProcess) Pids() ([]int, error)      { return f.pids, f.err }
func (f *fakeSysProcess) Status() ([]string, error) { return f.status, f.err }
func (f *fakeSysProcess) User() ([]string, error)   { return f.user, f.err }

func TestNewProcess(t *testing.T) {
	t.Run("populates status and user from the system process", func(t *testing.T) {
		sysProcess := &fakeSysProcess{
			executable: "nginx",
			running:    true,
			status:     []string{"running"},
			user:       []string{"www-data"},
		}
		p, err := NewProcess(sysProcess, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.id, "nginx")
		assert.Equal(t, p.Running, matcher(true))
		assert.DeepEqual(t, p.Status, matcher([]string{"running"}))
		assert.DeepEqual(t, p.User, matcher([]string{"www-data"}))
	})

	t.Run("status/user in the ignore list are left unset", func(t *testing.T) {
		sysProcess := &fakeSysProcess{
			executable: "nginx",
			running:    true,
			status:     []string{"running"},
			user:       []string{"www-data"},
		}
		p, err := NewProcess(sysProcess, util.Config{IgnoreList: []string{"status", "user"}})
		assert.NilError(t, err)
		assert.Equal(t, p.Status, nil)
		assert.Equal(t, p.User, nil)
	})

	t.Run("error from Running propagates and aborts construction", func(t *testing.T) {
		sysProcess := &fakeSysProcess{executable: "nginx", err: errors.New("boom")}
		_, err := NewProcess(sysProcess, util.Config{})
		assert.ErrorContains(t, err, "boom")
	})
}

func TestProcessValidate(t *testing.T) {
	newFakeProcess := func(sysProcess system.Process) *system.System {
		return &system.System{
			NewProcess: func(_ context.Context, _ string, _ *system.System, _ util.Config) system.Process {
				return sysProcess
			},
		}
	}

	t.Run("only running is checked when status/user aren't configured", func(t *testing.T) {
		p := &Process{Running: true}
		sys := newFakeProcess(&fakeSysProcess{running: true})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 1)
		assert.Equal(t, results[0].Property, "running")
	})

	t.Run("status and user are checked when configured", func(t *testing.T) {
		p := &Process{Running: true, Status: []interface{}{"zombie"}, User: "root"}
		sys := newFakeProcess(&fakeSysProcess{running: true, status: []string{"zombie"}, user: []string{"root"}})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 3)
		assert.Equal(t, results[0].Property, "running")
		assert.Equal(t, results[1].Property, "status")
		assert.Equal(t, results[2].Property, "user")
		for _, r := range results {
			assert.Equal(t, r.Result, SUCCESS)
		}
	})

	t.Run("a failed running check skips subsequent status/user checks", func(t *testing.T) {
		p := &Process{Running: true, Status: []interface{}{"zombie"}}
		sys := newFakeProcess(&fakeSysProcess{running: false})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 2)
		assert.Equal(t, results[0].Result, FAIL)
		assert.Equal(t, results[1].Result, SKIP)
	})
}
