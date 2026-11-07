package system

import (
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/net"
	"gotest.tools/v3/assert"

	"github.com/krameff/syver/util"
)

// fakeConnections stubs out connectionsByKind so tests never touch the real
// OS connection table. errs, if non-nil, maps a kind ("tcp4", "tcp6", "udp4",
// "udp6") to the error connectionsByKind should return for that kind.
func fakeConnections(t *testing.T, byKind map[string][]net.ConnectionStat, errs map[string]error) {
	t.Helper()
	orig := connectionsByKind
	connectionsByKind = func(kind string) ([]net.ConnectionStat, error) {
		return byKind[kind], errs[kind]
	}
	t.Cleanup(func() {
		connectionsByKind = orig
	})
}

func TestGetPorts(t *testing.T) {
	t.Run("listening tcp port is captured, non-listening is not", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{
			"tcp4": {
				{Laddr: net.Addr{IP: "0.0.0.0", Port: 8080}, Status: "LISTEN"},
				{Laddr: net.Addr{IP: "0.0.0.0", Port: 9090}, Status: "ESTABLISHED"},
			},
		}, nil)

		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["tcp:8080"]), 1)
		_, ok := ports["tcp:9090"]
		assert.Equal(t, ok, false)
	})

	t.Run("multiple protocols on the same port number are kept distinct", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{
			"tcp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 53}, Status: "LISTEN"}},
			"tcp6": {{Laddr: net.Addr{IP: "::", Port: 53}, Status: "LISTEN"}},
			"udp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 53}}},
			"udp6": {{Laddr: net.Addr{IP: "::", Port: 53}}},
		}, nil)

		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["tcp:53"]), 1)
		assert.Equal(t, len(ports["tcp6:53"]), 1)
		assert.Equal(t, len(ports["udp:53"]), 1)
		assert.Equal(t, len(ports["udp6:53"]), 1)
	})

	t.Run("udp entries are kept regardless of status (no LISTEN concept)", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{
			"udp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 123}, Status: "NONE"}},
		}, nil)

		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["udp:123"]), 1)
	})

	t.Run("a bad protocol lookup does not mask the others (error-joining regression)", func(t *testing.T) {
		tcp6Err := errors.New("could not parse /proc/net/tcp6")
		fakeConnections(t, map[string][]net.ConnectionStat{
			"tcp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 22}, Status: "LISTEN"}},
			"udp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 53}}},
		}, map[string]error{
			"tcp6": tcp6Err,
		})

		ports, err := GetPorts()
		assert.ErrorIs(t, err, tcp6Err)
		// tcp4 and udp4 results still come through despite tcp6 failing.
		assert.Equal(t, len(ports["tcp:22"]), 1)
		assert.Equal(t, len(ports["udp:53"]), 1)
	})

	t.Run("no ports at all is not an error", func(t *testing.T) {
		fakeConnections(t, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports), 0)
	})
}

func TestDefPortListening(t *testing.T) {
	tests := []struct {
		name          string
		port          string
		sysPorts      map[string][]net.ConnectionStat
		err           error
		wantListening bool
		wantErr       bool
	}{
		{
			name:          "listening",
			port:          "tcp:8080",
			sysPorts:      map[string][]net.ConnectionStat{"tcp:8080": {{}}},
			wantListening: true,
		},
		{
			name:          "not listening",
			port:          "tcp:9090",
			sysPorts:      map[string][]net.ConnectionStat{"tcp:8080": {{}}},
			wantListening: false,
		},
		{
			name:    "underlying error propagates",
			port:    "tcp:8080",
			err:     errors.New("boom"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &DefPort{port: tt.port, key: tt.port, sysPorts: tt.sysPorts, err: tt.err}

			listening, err := p.Listening()
			if tt.wantErr {
				assert.ErrorContains(t, err, "boom")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, listening, tt.wantListening)

			exists, err := p.Exists()
			assert.NilError(t, err)
			assert.Equal(t, exists, tt.wantListening)
		})
	}
}

func TestDefPortIP(t *testing.T) {
	t.Run("returns all ips for the port", func(t *testing.T) {
		p := &DefPort{
			port: "tcp:8080",
			key:  "tcp:8080",
			sysPorts: map[string][]net.ConnectionStat{
				"tcp:8080": {
					{Laddr: net.Addr{IP: "127.0.0.1"}},
					{Laddr: net.Addr{IP: "0.0.0.0"}},
				},
			},
		}
		ips, err := p.IP()
		assert.NilError(t, err)
		assert.DeepEqual(t, ips, []string{"127.0.0.1", "0.0.0.0"})
	})

	t.Run("underlying error propagates", func(t *testing.T) {
		p := &DefPort{port: "tcp:8080", err: errors.New("boom")}
		_, err := p.IP()
		assert.ErrorContains(t, err, "boom")
	})
}

func TestDefPortPID(t *testing.T) {
	t.Run("returns the owning pid of every connection on the port", func(t *testing.T) {
		p := &DefPort{
			port: "tcp:8080",
			key:  "tcp:8080",
			sysPorts: map[string][]net.ConnectionStat{
				"tcp:8080": {
					{Pid: 1234},
					{Pid: 5678},
				},
			},
		}
		pids, err := p.PID()
		assert.NilError(t, err)
		assert.DeepEqual(t, pids, []int{1234, 5678})
	})

	t.Run("an unresolved owner (pid 0) is omitted, not reported as pid 0", func(t *testing.T) {
		p := &DefPort{
			port: "tcp:8080",
			key:  "tcp:8080",
			sysPorts: map[string][]net.ConnectionStat{
				"tcp:8080": {
					{Pid: 0},
					{Pid: 42},
				},
			},
		}
		pids, err := p.PID()
		assert.NilError(t, err)
		assert.DeepEqual(t, pids, []int{42})
	})

	t.Run("underlying error propagates", func(t *testing.T) {
		p := &DefPort{port: "tcp:8080", err: errors.New("boom")}
		_, err := p.PID()
		assert.ErrorContains(t, err, "boom")
	})
}

func TestNormalizePort(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8080", "tcp:8080"},
		{"tcp:8080", "tcp:8080"},
		{"udp:53", "udp:53"},
	}
	for _, tt := range tests {
		got := normalizePort(tt.in)
		assert.Equal(t, got, tt.want)
	}
}

// TestNewDefPortNetworks covers the network half of a port key end to end,
// through the real GetPorts with a stubbed connection table: the six accepted
// networks, the tcp4/udp4 aliases finding what tcp/udp find, the bare-port
// default, and an unknown network failing on every method rather than reading
// as "not listening" (goss-org/goss#1006).
func TestNewDefPortNetworks(t *testing.T) {
	fakeConnections(t, map[string][]net.ConnectionStat{
		"tcp4": {{Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 80}, Pid: 10}},
		"tcp6": {{Status: "LISTEN", Laddr: net.Addr{IP: "::", Port: 443}, Pid: 11}},
		"udp4": {{Laddr: net.Addr{IP: "0.0.0.0", Port: 53}, Pid: 12}},
		"udp6": {{Laddr: net.Addr{IP: "::", Port: 5353}, Pid: 13}},
	}, nil)
	sys := &System{}

	listening := []struct {
		port, wantPort string
		want           bool
	}{
		{"80", "tcp:80", true},
		{"tcp:80", "tcp:80", true},
		{"tcp4:80", "tcp4:80", true}, // alias keeps the key as written
		{"tcp6:80", "tcp6:80", false},
		{"tcp6:443", "tcp6:443", true},
		{"tcp4:443", "tcp4:443", false},
		{"udp:53", "udp:53", true},
		{"udp4:53", "udp4:53", true},
		{"udp6:5353", "udp6:5353", true},
		{"udp4:5353", "udp4:5353", false},
	}
	for _, tt := range listening {
		p := NewDefPort(t.Context(), tt.port, sys, util.Config{})
		got, err := p.Listening()
		assert.NilError(t, err, tt.port)
		assert.Equal(t, got, tt.want, tt.port)
		assert.Equal(t, p.Port(), tt.wantPort, tt.port)
	}
	ips, err := NewDefPort(t.Context(), "tcp4:80", sys, util.Config{}).IP()
	assert.NilError(t, err)
	assert.DeepEqual(t, ips, []string{"0.0.0.0"})

	for _, port := range []string{"tpc:80", "TCP:80", "tcp46:80", "sctp:80", ":80"} {
		p := NewDefPort(t.Context(), port, sys, util.Config{})
		_, lerr := p.Listening()
		_, eerr := p.Exists()
		_, ierr := p.IP()
		_, perr := p.PID()
		for name, err := range map[string]error{"Listening": lerr, "Exists": eerr, "IP": ierr, "PID": perr} {
			assert.Assert(t, errors.Is(err, ErrUnknownPortNetwork), "%s(%q) = %v", name, port, err)
			assert.ErrorContains(t, err, "must be one of tcp, tcp4, tcp6, udp, udp4, udp6")
		}
	}
}

// An unknown network is reported as itself even when the connection table
// also failed: the spec error is the one the user can act on.
func TestNewDefPortUnknownNetworkBeatsTableError(t *testing.T) {
	fakeConnections(t, nil, map[string]error{"tcp4": errors.New("boom")})
	_, err := NewDefPort(t.Context(), "tpc:22", &System{}, util.Config{}).Listening()
	assert.Assert(t, errors.Is(err, ErrUnknownPortNetwork), "%v", err)
}
