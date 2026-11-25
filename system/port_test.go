package system

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"

	"github.com/shirou/gopsutil/v4/net"
	"gotest.tools/v3/assert"

	"github.com/krameff/syver/util"
)

// fakeConnections stubs out connectionsByKind so tests never touch the real
// OS connection table. errs, if non-nil, maps a kind ("tcp4", "tcp6", "udp4",
// "udp6") to the error connectionsByKind should return for that kind. It also
// stubs wildcardV6OnlyFn to know no socket's IPV6_V6ONLY, so a fake [::]
// entry is never matched against the host's real sockets; fakeV6Only
// overrides that.
func fakeConnections(t *testing.T, byKind map[string][]net.ConnectionStat, errs map[string]error) {
	t.Helper()
	orig := connectionsByKind
	connectionsByKind = func(kind string) ([]net.ConnectionStat, error) {
		return byKind[kind], errs[kind]
	}
	t.Cleanup(func() {
		connectionsByKind = orig
	})
	fakeV6Only(t, nil, nil, nil)
}

// fakeV6Only stubs wildcardV6OnlyFn: byProto maps protoTCP/protoUDP to the
// states to return, err is returned for every call, and calls, if non-nil,
// counts the calls per protocol.
func fakeV6Only(t *testing.T, byProto map[uint8]map[uint32]v6onlyState, err error, calls map[uint8]int) {
	t.Helper()
	orig := wildcardV6OnlyFn
	wildcardV6OnlyFn = func(proto uint8) (map[uint32]v6onlyState, error) {
		if calls != nil {
			calls[proto]++
		}
		return byProto[proto], err
	}
	t.Cleanup(func() { wildcardV6OnlyFn = orig })
}

// capturePortLog redirects the standard logger and forgets the DEBUG lines
// already logged, so a test sees debugOnce's output.
func capturePortLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	debugLogged = sync.Map{}
	t.Cleanup(func() {
		log.SetOutput(prev)
		debugLogged = sync.Map{}
	})
	return &buf
}

// TestGetPortsDualStack covers the synthesis of a dual-stack [::] socket into
// its IPv4 key, through the connection-table and sock_diag seams.
func TestGetPortsDualStack(t *testing.T) {
	ollama := net.ConnectionStat{Fd: 3, Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "::", Port: 11434}, Pid: 4242}
	tcp6Wild := map[string][]net.ConnectionStat{"tcp6": {ollama}}

	// Revert-proof: remove the addDualStack calls from GetPorts; tcp:11434
	// is then absent.
	t.Run("dual-stack tcp6 [::] counts for tcp with ip 0.0.0.0", func(t *testing.T) {
		fakeConnections(t, tcp6Wild, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, nil)

		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["tcp:11434"]), 1)
		got := ports["tcp:11434"][0]
		assert.Equal(t, got.Laddr.IP, "0.0.0.0")
		assert.Equal(t, got.Laddr.Port, uint32(11434))
		assert.Equal(t, got.Pid, int32(4242))
		assert.Equal(t, got.Family, uint32(10))
		// The IPv6 key is unchanged.
		assert.DeepEqual(t, ports["tcp6:11434"], []net.ConnectionStat{ollama})
	})

	// Revert-proof: synthesise for v6onlyUnknown as well as v6onlyOff; the
	// unknown row then gains tcp:11434.
	for _, st := range []struct {
		name  string
		state map[uint32]v6onlyState
	}{
		{"IPv6-only", map[uint32]v6onlyState{11434: v6onlyOn}},
		{"unknown (attribute missing)", map[uint32]v6onlyState{11434: v6onlyUnknown}},
		{"unknown (port not in the dump)", map[uint32]v6onlyState{}},
	} {
		t.Run(st.name+" [::] does not count for tcp", func(t *testing.T) {
			fakeConnections(t, tcp6Wild, nil)
			fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: st.state}, nil, nil)
			ports, err := GetPorts()
			assert.NilError(t, err)
			_, ok := ports["tcp:11434"]
			assert.Equal(t, ok, false)
		})
	}

	// Revert-proof: drop the hasIP guard; tcp:11434 then holds two 0.0.0.0
	// entries.
	t.Run("an existing IPv4 0.0.0.0 entry suppresses the copy", func(t *testing.T) {
		v4 := net.ConnectionStat{Family: 2, Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 11434}, Pid: 4242}
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp4": {v4}, "tcp6": {ollama}}, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:11434"], []net.ConnectionStat{v4})
	})

	// Revert-proof: make the guard any existing IPv4 entry
	// (len(ports[v4key]) > 0); the copy is then suppressed.
	t.Run("an IPv4 entry on another address does not suppress the copy", func(t *testing.T) {
		v4 := net.ConnectionStat{Family: 2, Status: "LISTEN", Laddr: net.Addr{IP: "127.0.0.1", Port: 11434}}
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp4": {v4}, "tcp6": {ollama}}, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["tcp:11434"]), 2)
		assert.Equal(t, ports["tcp:11434"][1].Laddr.IP, "0.0.0.0")
	})

	// Revert-proof: drop the "::" filter that collects wild; the fake is then
	// called for a [::1] entry.
	t.Run("no [::] entry: sock_diag is not asked", func(t *testing.T) {
		loop := net.ConnectionStat{Status: "LISTEN", Laddr: net.Addr{IP: "::1", Port: 11434}}
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {loop}, "udp6": {loop}}, nil)
		calls := map[uint8]int{}
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, calls)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(calls), 0)
		_, ok := ports["tcp:11434"]
		assert.Equal(t, ok, false)
	})

	// Revert-proof: append the sock_diag error to errs in addDualStack;
	// GetPorts then returns it.
	t.Run("a sock_diag error is logged once at DEBUG, never returned", func(t *testing.T) {
		buf := capturePortLog(t)
		fakeConnections(t, tcp6Wild, nil)
		fakeV6Only(t, nil, errors.New("no inet_diag"), nil)
		for range 2 {
			ports, err := GetPorts()
			assert.NilError(t, err)
			_, ok := ports["tcp:11434"]
			assert.Equal(t, ok, false)
			assert.DeepEqual(t, ports["tcp6:11434"], []net.ConnectionStat{ollama})
		}
		assert.Equal(t, strings.Count(buf.String(), "[DEBUG] port: could not read IPV6_V6ONLY for tcp sockets on [::]: no inet_diag; "+
			"dual-stack listeners are not counted for tcp:N, so those results are unchanged (check tcp6:N)"), 1, buf.String())
	})

	// Revert-proof: drop the debugOnce call for v6onlyUnknown; the line is
	// then missing.
	t.Run("a [::] port sock_diag does not describe is logged once at DEBUG", func(t *testing.T) {
		for _, st := range []map[uint32]v6onlyState{{11434: v6onlyUnknown}, {}} {
			buf := capturePortLog(t)
			fakeConnections(t, tcp6Wild, nil)
			fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: st}, nil, nil)
			for range 2 {
				_, err := GetPorts()
				assert.NilError(t, err)
			}
			assert.Equal(t, strings.Count(buf.String(), "[DEBUG] port: [::]:11434 has no IPV6_V6ONLY from sock_diag; "+
				"not counted for tcp:11434 (check tcp6:11434)"), 1, buf.String())
		}
	})

	// Revert-proof: log the unknown-port line for v6onlyOn too; this test
	// then sees it.
	t.Run("an IPv6-only [::] port logs nothing", func(t *testing.T) {
		buf := capturePortLog(t)
		fakeConnections(t, tcp6Wild, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOn}}, nil, nil)
		_, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, buf.String(), "")
	})

	// Revert-proof: remove the HOST_PROC gate; the fake is then called and
	// tcp:11434 appears.
	t.Run("HOST_PROC set elsewhere: sock_diag is not asked", func(t *testing.T) {
		buf := capturePortLog(t)
		t.Setenv("HOST_PROC", "/some/other/proc")
		fakeConnections(t, tcp6Wild, nil)
		calls := map[uint8]int{}
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, calls)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(calls), 0)
		_, ok := ports["tcp:11434"]
		assert.Equal(t, ok, false)
		assert.Assert(t, strings.Contains(buf.String(), "[DEBUG] port: HOST_PROC is set"), buf.String())
	})

	// Revert-proof: compare HOST_PROC without path.Clean, or with
	// filepath.Clean on Windows; "/proc/" is then gated and tcp:11434 is
	// missing.
	t.Run("HOST_PROC=/proc is no gate", func(t *testing.T) {
		t.Setenv("HOST_PROC", "/proc/")
		fakeConnections(t, tcp6Wild, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["tcp:11434"]), 1)
	})

	// Revert-proof: pass protoTCP for udp in GetPorts; udp:5353 is then absent.
	t.Run("dual-stack udp6 [::] counts for udp", func(t *testing.T) {
		mdns := net.ConnectionStat{Family: 10, Status: "NONE", Laddr: net.Addr{IP: "::", Port: 5353}, Pid: 77}
		fakeConnections(t, map[string][]net.ConnectionStat{"udp6": {mdns}}, nil)
		calls := map[uint8]int{}
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoUDP: {5353: v6onlyOff}}, nil, calls)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(ports["udp:5353"]), 1)
		assert.Equal(t, ports["udp:5353"][0].Laddr.IP, "0.0.0.0")
		assert.Equal(t, ports["udp:5353"][0].Pid, int32(77))
		assert.DeepEqual(t, calls, map[uint8]int{protoUDP: 1})
	})
}

// TestDefPortDualStack reads a synthesised entry back through DefPort, as a
// spec does. Revert-proof: as the first row of TestGetPortsDualStack.
func TestDefPortDualStack(t *testing.T) {
	fakeConnections(t, map[string][]net.ConnectionStat{
		"tcp6": {{Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "::", Port: 11434}, Pid: 4242}},
	}, nil)
	fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {11434: v6onlyOff}}, nil, nil)
	sys := &System{}

	p := NewDefPort(t.Context(), "11434", sys, util.Config{})
	listening, err := p.Listening()
	assert.NilError(t, err)
	assert.Equal(t, listening, true)
	ips, err := p.IP()
	assert.NilError(t, err)
	assert.DeepEqual(t, ips, []string{"0.0.0.0"})
	pids, err := p.PID()
	assert.NilError(t, err)
	assert.DeepEqual(t, pids, []int{4242})

	p6 := NewDefPort(t.Context(), "tcp6:11434", sys, util.Config{})
	listening, err = p6.Listening()
	assert.NilError(t, err)
	assert.Equal(t, listening, true)
	ips, err = p6.IP()
	assert.NilError(t, err)
	assert.DeepEqual(t, ips, []string{"::"})
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

// TestMappedIPv4 covers the address classes the IPv6 connection table can
// hold. Only an IPv4-mapped address (::ffff:0:0/96) is IPv4; every other form
// that embeds IPv4 bits is an IPv6 address.
func TestMappedIPv4(t *testing.T) {
	tests := []struct {
		name, ip, want string
		ok             bool
	}{
		// Revert-proof: return true for "0.0.0.0" only; the 127.0.0.1 row
		// then fails.
		{"mapped wildcard as gopsutil prints it", "0.0.0.0", "0.0.0.0", true},
		{"mapped specific as gopsutil prints it", "127.0.0.1", "127.0.0.1", true},
		// Revert-proof: drop a.Is4In6(); this row then returns false.
		{"the ::ffff: form", "::ffff:127.0.0.1", "127.0.0.1", true},
		// Revert-proof: accept any address that parses; every row here then
		// returns true.
		{"unspecified", "::", "", false},
		{"loopback", "::1", "", false},
		{"link-local", "fe80::1", "", false},
		{"global", "2001:db8::1", "", false},
		{"IPv4-compatible", "::7f00:1", "", false},
		{"NAT64", "64:ff9b::7f00:1", "", false},
		{"SIIT", "::ffff:0:7f00:1", "", false},
		// Revert-proof: replace netip with strings.Count(ip, ".") == 3; the
		// zero-padded row then returns true.
		{"three octets", "1.2.3", "", false},
		{"zero-padded octets", "001.002.003.004", "", false},
		// No killing mutation: with the parse error ignored the zero
		// netip.Addr is neither Is4 nor Is4In6, so the result is still false.
		// These rows guard a rewrite of mappedIPv4 against a panic.
		{"empty", "", "", false},
		{"not an address", "not-an-ip", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mappedIPv4(tt.ip)
			assert.Equal(t, ok, tt.ok)
			assert.Equal(t, got, tt.want)
		})
	}
}

// TestGetPortsMapped covers the synthesis of a socket on an IPv4-mapped
// address into its IPv4 key, through the connection-table and sock_diag
// seams. No case builds a filesystem path.
func TestGetPortsMapped(t *testing.T) {
	mappedWild := net.ConnectionStat{Fd: 5, Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 7003}, Pid: 501}
	mappedLoop := net.ConnectionStat{Fd: 6, Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "127.0.0.1", Port: 7003}, Pid: 502}
	withIP := func(e net.ConnectionStat, ip string) net.ConnectionStat {
		e.Laddr.IP = ip
		return e
	}

	// Revert-proof: remove the addMapped calls from GetPorts; tcp:7003 is
	// then absent.
	t.Run("mapped wildcard counts for tcp with ip 0.0.0.0", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {mappedWild}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{mappedWild})
		assert.Equal(t, ports["tcp:7003"][0].Family, uint32(10))
		assert.DeepEqual(t, ports["tcp6:7003"], []net.ConnectionStat{mappedWild})
	})

	// Revert-proof: copy only entries whose address is 0.0.0.0; tcp:7003 is
	// then absent.
	t.Run("mapped specific counts for tcp with its IPv4 address", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {mappedLoop}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{mappedLoop})
	})

	// Revert-proof: in addMapped, skip an entry unless wildcardV6OnlyFn says
	// its port is v6onlyOff; the call counter is then non-zero and the
	// error and unknown rows lose tcp:7003.
	t.Run("sock_diag is not consulted for mapped entries", func(t *testing.T) {
		byKind := map[string][]net.ConnectionStat{"tcp6": {mappedLoop}, "udp6": {withIP(mappedLoop, "127.0.0.1")}}
		calls := map[uint8]int{}
		fakeConnections(t, byKind, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {7003: v6onlyOn}}, nil, calls)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Equal(t, len(calls), 0)
		assert.Equal(t, len(ports["tcp:7003"]), 1)

		for name, fake := range map[string]func(){
			"sock_diag error": func() { fakeV6Only(t, nil, errors.New("no inet_diag"), nil) },
			"all unknown": func() {
				fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {7003: v6onlyUnknown}, protoUDP: {7003: v6onlyUnknown}}, nil, nil)
			},
		} {
			fakeConnections(t, byKind, nil)
			fake()
			ports, err := GetPorts()
			assert.NilError(t, err, name)
			assert.Equal(t, len(ports["tcp:7003"]), 1, name)
			assert.Equal(t, len(ports["udp:7003"]), 1, name)
		}
	})

	// Revert-proof: return from addMapped when HOST_PROC is set to anything
	// but /proc; tcp:7003 is then absent.
	t.Run("HOST_PROC set elsewhere: still counted, nothing logged", func(t *testing.T) {
		buf := capturePortLog(t)
		t.Setenv("HOST_PROC", "/some/other/proc")
		noPid := withIP(mappedLoop, "127.0.0.1")
		noPid.Pid = 0
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {noPid}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		p := &DefPort{port: "tcp:7003", key: "tcp:7003", sysPorts: ports}
		listening, err := p.Listening()
		assert.NilError(t, err)
		assert.Equal(t, listening, true)
		ips, err := p.IP()
		assert.NilError(t, err)
		assert.DeepEqual(t, ips, []string{"127.0.0.1"})
		assert.Assert(t, !strings.Contains(buf.String(), "7003"), buf.String())
	})

	// Revert-proof: drop the hasIP guard in addMapped; tcp:7003 then holds
	// two 0.0.0.0 entries.
	t.Run("an existing IPv4 entry on the same address suppresses the copy", func(t *testing.T) {
		v4 := net.ConnectionStat{Family: 2, Status: "LISTEN", Laddr: net.Addr{IP: "0.0.0.0", Port: 7003}, Pid: 9}
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp4": {v4}, "tcp6": {mappedWild}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{v4})
	})

	// Revert-proof: test the guard against a snapshot of the IPv4 key taken
	// before the pass; tcp:7003 then holds both copies.
	t.Run("two mapped entries on one address give one copy, the first", func(t *testing.T) {
		second := mappedLoop
		second.Pid = 503
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {mappedLoop, second}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{mappedLoop})
	})

	// Revert-proof: skip when the IPv4 key holds any entry
	// (len(ports[key]) > 0); the mapped copy is then missing.
	t.Run("an IPv4 entry on another address does not suppress the copy", func(t *testing.T) {
		v4 := net.ConnectionStat{Family: 2, Status: "LISTEN", Laddr: net.Addr{IP: "127.0.0.2", Port: 7003}, Pid: 9}
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp4": {v4}, "tcp6": {mappedLoop}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{v4, mappedLoop})
	})

	ipv6Only := net.ConnectionStat{Fd: 3, Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "::", Port: 7003}, Pid: 400}

	// Revert-proof: in addMapped, copy every entry of a key that holds a
	// mapped one; tcp:7003 then also holds the [::] entry.
	t.Run("an IPv6-only [::] beside a mapped wildcard: only the mapped one counts", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {ipv6Only, mappedWild}}, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {7003: v6onlyOn}}, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{mappedWild})
	})

	// Revert-proof: call addMapped before addDualStack in GetPorts; the Pid
	// is then the mapped entry's. Or drop the hasIP guard in addMapped;
	// tcp:7003 then holds two entries.
	t.Run("a dual-stack [::] beside a mapped wildcard: 0.0.0.0 once, the [::] owner", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {ipv6Only, mappedWild}}, nil)
		fakeV6Only(t, map[uint8]map[uint32]v6onlyState{protoTCP: {7003: v6onlyOff}}, nil, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["tcp:7003"], []net.ConnectionStat{withIP(ipv6Only, "0.0.0.0")})
	})

	// Revert-proof: as TestMappedIPv4's "accept any address that parses";
	// tcp:7003 then holds both entries.
	t.Run("native IPv6 entries are not copied", func(t *testing.T) {
		fakeConnections(t, map[string][]net.ConnectionStat{"tcp6": {
			withIP(ipv6Only, "::1"), withIP(ipv6Only, "fe80::1"),
		}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		_, ok := ports["tcp:7003"]
		assert.Equal(t, ok, false)
	})

	// Revert-proof: remove the udp addMapped call; udp:7004 is then absent.
	t.Run("mapped udp counts for udp", func(t *testing.T) {
		u := net.ConnectionStat{Fd: 7, Family: 10, Status: "NONE", Laddr: net.Addr{IP: "127.0.0.1", Port: 7004}, Pid: 504}
		fakeConnections(t, map[string][]net.ConnectionStat{"udp6": {u}}, nil)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.DeepEqual(t, ports["udp:7004"], []net.ConnectionStat{u})
		assert.DeepEqual(t, ports["udp6:7004"], []net.ConnectionStat{u})
	})
}

// TestDefPortMapped reads a mapped entry back through DefPort, as a spec
// does. Revert-proof: as the first row of TestGetPortsMapped.
func TestDefPortMapped(t *testing.T) {
	fakeConnections(t, map[string][]net.ConnectionStat{
		"tcp6": {{Family: 10, Status: "LISTEN", Laddr: net.Addr{IP: "127.0.0.1", Port: 7003}, Pid: 502}},
	}, nil)
	sys := &System{}

	for _, port := range []string{"tcp:7003", "tcp4:7003", "tcp6:7003"} {
		p := NewDefPort(t.Context(), port, sys, util.Config{})
		listening, err := p.Listening()
		assert.NilError(t, err, port)
		assert.Equal(t, listening, true, port)
		ips, err := p.IP()
		assert.NilError(t, err, port)
		assert.DeepEqual(t, ips, []string{"127.0.0.1"})
		pids, err := p.PID()
		assert.NilError(t, err, port)
		assert.DeepEqual(t, pids, []int{502})
	}
}
