package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"gotest.tools/v3/assert"
)

// fakeSysPort is a minimal system.Port fake so resource-level tests don't
// need a real OS connection table.
type fakeSysPort struct {
	port      string
	exists    bool
	listening bool
	ip        []string
	pid       []int
	err       error
}

func (f *fakeSysPort) Port() string             { return f.port }
func (f *fakeSysPort) Exists() (bool, error)    { return f.exists, f.err }
func (f *fakeSysPort) Listening() (bool, error) { return f.listening, f.err }
func (f *fakeSysPort) IP() ([]string, error)    { return f.ip, f.err }
func (f *fakeSysPort) PID() ([]int, error)      { return f.pid, f.err }

func TestNewPort(t *testing.T) {
	t.Run("does not auto-populate pid: PIDs aren't stable across restarts", func(t *testing.T) {
		sysPort := &fakeSysPort{port: "tcp:8080", listening: true, pid: []int{1234}}
		p, err := NewPort(sysPort, util.Config{})
		assert.NilError(t, err)
		assert.Equal(t, p.id, "tcp:8080")
		assert.Equal(t, p.Listening, matcher(true))
		assert.Equal(t, p.PID, nil)
	})
}

func TestPortValidate(t *testing.T) {
	newFakePort := func(sysPort system.Port) *system.System {
		return &system.System{
			NewPort: func(_ context.Context, _ string, _ *system.System, _ util.Config) system.Port {
				return sysPort
			},
		}
	}

	t.Run("only listening is checked when ip/pid aren't configured", func(t *testing.T) {
		p := &Port{Listening: true}
		sys := newFakePort(&fakeSysPort{listening: true})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 1)
		assert.Equal(t, results[0].Property, "listening")
	})

	t.Run("ip and pid are checked when configured", func(t *testing.T) {
		p := &Port{Listening: true, IP: []interface{}{"127.0.0.1"}, PID: []interface{}{1234}}
		sys := newFakePort(&fakeSysPort{listening: true, ip: []string{"127.0.0.1"}, pid: []int{1234}})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 3)
		assert.Equal(t, results[0].Property, "listening")
		assert.Equal(t, results[1].Property, "ip")
		assert.Equal(t, results[2].Property, "pid")
		for _, r := range results {
			assert.Equal(t, r.Result, SUCCESS)
		}
	})

	t.Run("a failed listening check skips subsequent ip/pid checks", func(t *testing.T) {
		p := &Port{Listening: true, PID: []interface{}{1234}}
		sys := newFakePort(&fakeSysPort{listening: false})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, len(results), 2)
		assert.Equal(t, results[0].Result, FAIL)
		assert.Equal(t, results[1].Result, SKIP)
	})

	t.Run("underlying error surfaces as a failed result", func(t *testing.T) {
		p := &Port{Listening: true}
		sys := newFakePort(&fakeSysPort{err: errors.New("boom")})
		results := p.Validate(t.Context(), sys)
		assert.Equal(t, results[0].Result, FAIL)
	})
}

// A port key whose network GetPorts never records (here a typo) used to be
// "not listening", so `listening: false` passed without checking anything and
// `listening: true` failed without saying why (goss-org/goss#1006). Both must
// now fail and name the network. Uses the real system.NewDefPort, since the
// check lives there.
func TestPortUnknownNetworkFailsInsteadOfPassing(t *testing.T) {
	sys := &system.System{NewPort: system.NewDefPort}
	for _, want := range []bool{false, true} {
		p := &Port{id: "tpc:65535", Listening: want}
		results := p.Validate(t.Context(), sys)
		assert.Assert(t, len(results) > 0)
		r := results[0]
		assert.Equal(t, r.Result, FAIL, "listening: %v", want)
		assert.ErrorContains(t, r.Err, `unknown port network "tpc" in port "tpc:65535"`)
	}
}

// `syver add port tpc:22` must refuse, not write a `listening: false` entry
// that validate would then report as an error.
func TestPortAddRejectsUnknownNetwork(t *testing.T) {
	desc, ok := DescriptorByKey(PortResourceKey)
	assert.Assert(t, ok)
	sys := &system.System{NewPort: system.NewDefPort}
	_, err := desc.AppendSys(sys, "tpc:22", util.Config{})
	assert.Assert(t, errors.Is(err, system.ErrUnknownPortNetwork), "err = %v", err)
}

// autoadd tries a name as every auto-addable type. A name that is not a valid
// port key is simply not a port: skipped, with no error to abort the run.
func TestPortAutoaddSkipsUnknownNetwork(t *testing.T) {
	sys := &system.System{NewPort: system.NewDefPort}
	ports := PortMap{}
	_, _, found, err := ports.AppendSysResourceIfExists("foo:bar", sys)
	assert.NilError(t, err)
	assert.Assert(t, !found)
	assert.Equal(t, len(ports), 0)
}
