package system

import (
	"encoding/binary"
	"errors"
	stdnet "net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"gotest.tools/v3/assert"
)

// inetDiagMsg builds an inet_diag_msg payload: the 72-byte fixed part with
// family, port and src set, followed by attrs.
func inetDiagMsg(family uint8, port uint16, src [16]byte, attrs ...[]byte) []byte {
	b := make([]byte, inetDiagMsgLen)
	b[0] = family
	binary.BigEndian.PutUint16(b[4:6], port)
	copy(b[8:24], src[:])
	for _, a := range attrs {
		b = append(b, a...)
	}
	return b
}

// rtattr builds one rtattr with its header in order, padded to 4 bytes.
func rtattr(order binary.ByteOrder, typ uint16, val ...byte) []byte {
	b := make([]byte, 4, 4+len(val)+3)
	order.PutUint16(b[0:2], uint16(4+len(val)))
	order.PutUint16(b[2:4], typ)
	b = append(b, val...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// TestDecodeInetDiag is T1: the decoder on hand-built bytes, no sockets.
func TestDecodeInetDiag(t *testing.T) {
	le, be := binary.LittleEndian, binary.BigEndian
	other := rtattr(le, 1, 0, 0, 0, 0) // INET_DIAG_MEMINFO-shaped, ignored

	tests := []struct {
		name   string
		b      []byte
		order  binary.ByteOrder
		want   v6onlyState
		wantOK bool
	}{
		{"value 0 is dual-stack", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, rtattr(le, inetDiagSkV6Only, 0)), le, v6onlyOff, true},
		{"value 1 is IPv6-only", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, rtattr(le, inetDiagSkV6Only, 1)), le, v6onlyOn, true},
		// Revert-proof: hard-code binary.LittleEndian in the attribute walk;
		// these two rows then read length 0x0500 and stop, giving Unknown.
		{"big-endian rtattr headers, value 0", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, rtattr(be, inetDiagSkV6Only, 0)), be, v6onlyOff, true},
		{"big-endian rtattr headers, value 1", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, rtattr(be, 1, 9, 9, 9, 9), rtattr(be, inetDiagSkV6Only, 1)), be, v6onlyOn, true},
		{"found after another attribute", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, other, rtattr(le, inetDiagSkV6Only, 0)), le, v6onlyOff, true},
		// Revert-proof: default st to v6onlyOff.
		{"no type-11 attribute is unknown", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, other), le, v6onlyUnknown, true},
		{"type 11 with no value byte is unknown", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, rtattr(le, inetDiagSkV6Only)), le, v6onlyUnknown, true},
		// Revert-proof, one bound at a time: drop "l < 4" (length 2 advances
		// to a zero header that never advances again: the test hangs until
		// -timeout); drop "l > len(attrs)" (attrs[4] is read past the end:
		// index panic); drop "len(attrs) >= 4" (index panic on the 3-byte
		// fragment).
		{"rtattr length 2", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, []byte{2, 0, inetDiagSkV6Only, 0, 0, 0, 0, 0}), le, v6onlyUnknown, true},
		{"rtattr length past the end", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, []byte{64, 0, inetDiagSkV6Only, 0}), le, v6onlyUnknown, true},
		{"rtattr length past the end, value byte present", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, []byte{9, 0, inetDiagSkV6Only, 0}), le, v6onlyUnknown, true},
		{"trailing 3-byte fragment", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{}, []byte{5, 0, inetDiagSkV6Only}), le, v6onlyUnknown, true},
		// Revert-proof: drop the length check; b[8:24] or b[72:] then panics.
		{"71 bytes is an error", inetDiagMsg(unix.AF_INET6, 11434, [16]byte{})[:71], le, v6onlyUnknown, false},
		{"empty is an error", nil, le, v6onlyUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, port, st, err := decodeInetDiag(tt.b, tt.order)
			if !tt.wantOK {
				assert.ErrorContains(t, err, "inet_diag_msg of")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, src, [16]byte{})
			// Revert-proof: decode the port with attrOrder; the little-endian
			// rows then read 11434 byte-swapped.
			assert.Equal(t, port, uint16(11434))
			assert.Equal(t, st, tt.want)
		})
	}

	// Revert-proof: drop the src filter in foldInetDiag; port 80 then appears.
	t.Run("a non-zero src is excluded", func(t *testing.T) {
		states := map[uint32]v6onlyState{}
		loop := [16]byte{15: 1} // ::1
		assert.NilError(t, foldInetDiag(states, inetDiagMsg(unix.AF_INET6, 80, loop, rtattr(le, inetDiagSkV6Only, 0)), le))
		assert.Equal(t, len(states), 0)
	})

	// Revert-proof: drop the AF_INET6 check in foldInetDiag; port 80 then
	// appears.
	t.Run("an AF_INET row is excluded", func(t *testing.T) {
		states := map[uint32]v6onlyState{}
		assert.NilError(t, foldInetDiag(states, inetDiagMsg(unix.AF_INET, 80, [16]byte{}, rtattr(le, inetDiagSkV6Only, 0)), le))
		assert.Equal(t, len(states), 0)
	})

	// Revert-proof: drop "st == v6onlyOff" from the merge in foldInetDiag;
	// the {on, off} order then ends On.
	t.Run("dual-stack wins whatever order the sockets on one port arrive in", func(t *testing.T) {
		on := inetDiagMsg(unix.AF_INET6, 80, [16]byte{}, rtattr(le, inetDiagSkV6Only, 1))
		off := inetDiagMsg(unix.AF_INET6, 80, [16]byte{}, rtattr(le, inetDiagSkV6Only, 0))
		unknown := inetDiagMsg(unix.AF_INET6, 80, [16]byte{})
		for _, order := range [][][]byte{{on, off}, {off, on}, {off, unknown}, {unknown, off}} {
			states := map[uint32]v6onlyState{}
			for _, m := range order {
				assert.NilError(t, foldInetDiag(states, m, le))
			}
			assert.Equal(t, states[80], v6onlyOff)
		}
	})

	// Revert-proof: have foldInetDiag return nil when decodeInetDiag fails;
	// the error is then lost.
	t.Run("a short message is an error from foldInetDiag too", func(t *testing.T) {
		assert.ErrorContains(t, foldInetDiag(map[uint32]v6onlyState{}, make([]byte, 10), le), "inet_diag_msg of")
	})
}

// TestInetDiagRequest checks the request layout. Revert-proof: drop the
// TCP-only branch so UDP asks for TCP_LISTEN only; the last assertion fails.
func TestInetDiagRequest(t *testing.T) {
	b := inetDiagRequest(protoTCP, 7)
	assert.Equal(t, len(b), 72)
	ne := binary.NativeEndian
	assert.Equal(t, ne.Uint32(b[0:4]), uint32(72))
	assert.Equal(t, ne.Uint16(b[4:6]), uint16(unix.SOCK_DIAG_BY_FAMILY))
	assert.Equal(t, ne.Uint16(b[6:8]), uint16(unix.NLM_F_REQUEST|unix.NLM_F_DUMP))
	assert.Equal(t, ne.Uint32(b[8:12]), uint32(7))
	assert.Equal(t, b[16], byte(unix.AF_INET6))
	assert.Equal(t, b[17], protoTCP)
	assert.Equal(t, ne.Uint32(b[20:24]), uint32(1<<10))
	assert.Equal(t, ne.Uint32(inetDiagRequest(protoUDP, 7)[20:24]), ^uint32(0))
}

// TestDoneError covers the errno a kernel puts in an NLMSG_DONE payload when
// it ends a dump early. Revert-proof: make doneError return nil; the ENOENT
// row then reads as a complete dump.
func TestDoneError(t *testing.T) {
	enoent := make([]byte, 4)
	code := -int32(unix.ENOENT)
	binary.NativeEndian.PutUint32(enoent, uint32(code))
	assert.Assert(t, errors.Is(doneError(enoent), unix.ENOENT))
	assert.ErrorContains(t, doneError(enoent), "sock_diag: kernel returned")
	assert.NilError(t, doneError(nil), "a DONE with no payload")
	assert.NilError(t, doneError([]byte{0, 0, 0, 0}), "a zero code")
	assert.NilError(t, doneError([]byte{0xfe, 0xff, 0xff}), "a payload too short to hold a code")
}

// TestWildcardV6OnlyDoneErrno asks for a protocol no kernel has a sock_diag
// handler for; the kernel answers with NLMSG_DONE carrying -ENOENT, which
// must come back as an error, not as an empty, complete dump. Revert-proof:
// return states on NLMSG_DONE without calling doneError; err is then nil.
func TestWildcardV6OnlyDoneErrno(t *testing.T) {
	states, err := wildcardV6Only(250)
	assert.Assert(t, errors.Is(err, unix.ENOENT), "states=%v err=%v", states, err)
}

// TestReadInetDiagDumpDeadline reads a netlink socket that was sent nothing,
// so no reply comes. The whole-dump deadline must bound the wait, not the
// longer per-receive timeout, and the error must say no reply came.
// Revert-proof: use recvTimeout alone as the receive timeout (no min with the
// remaining time); the read then waits 10s and the elapsed check fails.
func TestReadInetDiagDumpDeadline(t *testing.T) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	assert.NilError(t, err)
	defer unix.Close(fd)

	start := time.Now()
	_, err = readInetDiagDump(fd, 1, start.Add(200*time.Millisecond), 10*time.Second)
	elapsed := time.Since(start)
	assert.ErrorContains(t, err, "sock_diag: no reply within")
	assert.Assert(t, elapsed < 5*time.Second, "waited %v for a 200ms deadline", elapsed)
}

// requireIPv6 skips a real-socket test on a host without IPv6.
func requireIPv6(t *testing.T) {
	t.Helper()
	l, err := stdnet.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 unavailable: %v", err)
	}
	_ = l.Close()
}

type realSocket struct {
	name string
	port uint32
	want v6onlyState
	udp  stdnet.PacketConn
}

// openRealSockets opens the T2 sockets and closes them at cleanup.
func openRealSockets(t *testing.T) (tcp, udp []realSocket) {
	t.Helper()
	requireIPv6(t)
	for _, s := range []struct {
		network, addr string
		want          v6onlyState
	}{
		{"tcp", ":0", v6onlyOff},
		{"tcp", "0.0.0.0:0", v6onlyOff},
		{"tcp6", "[::]:0", v6onlyOn},
	} {
		l, err := stdnet.Listen(s.network, s.addr)
		assert.NilError(t, err)
		t.Cleanup(func() { _ = l.Close() })
		a := l.Addr().(*stdnet.TCPAddr)
		if a.IP.To4() != nil {
			t.Skipf("Listen(%q, %q) returned the IPv4 address %v: no IPv4-mapped IPv6", s.network, s.addr, a)
		}
		tcp = append(tcp, realSocket{name: s.network + " " + s.addr, port: uint32(a.Port), want: s.want})
	}
	for _, s := range []struct {
		network, addr string
		want          v6onlyState
	}{
		{"udp", ":0", v6onlyOff},
		{"udp6", "[::]:0", v6onlyOn},
	} {
		c, err := stdnet.ListenPacket(s.network, s.addr)
		assert.NilError(t, err)
		t.Cleanup(func() { _ = c.Close() })
		a := c.LocalAddr().(*stdnet.UDPAddr)
		if a.IP.To4() != nil {
			t.Skipf("ListenPacket(%q, %q) returned the IPv4 address %v", s.network, s.addr, a)
		}
		udp = append(udp, realSocket{name: s.network + " " + s.addr, port: uint32(a.Port), want: s.want, udp: c})
	}
	return tcp, udp
}

// readUDPStates reads UDP states, skipping only when the kernel says it has no
// UDP sock_diag handler.
func readUDPStates(t *testing.T) map[uint32]v6onlyState {
	t.Helper()
	states, err := wildcardV6Only(protoUDP)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.EPROTONOSUPPORT) {
		t.Skipf("kernel has no UDP sock_diag: %v", err)
	}
	assert.NilError(t, err)
	return states
}

// TestWildcardV6OnlyRealSockets is T2: sock_diag against sockets this test
// opened. Revert-proof: change inetDiagSkV6Only to 10; every state is then
// Unknown and each row fails.
func TestWildcardV6OnlyRealSockets(t *testing.T) {
	tcp, udp := openRealSockets(t)

	states, err := wildcardV6Only(protoTCP)
	assert.NilError(t, err)
	for _, s := range tcp {
		assert.Equal(t, states[s.port], s.want, s.name)
	}

	ustates := readUDPStates(t)
	for _, s := range udp {
		assert.Equal(t, ustates[s.port], s.want, s.name)
	}
}

// TestWildcardV6OnlyIPv4Oracle is T3: what the reader says is checked against
// what an IPv4 client actually gets, not against ss (which prints from the
// same attribute). Revert-proof: swap the 0 and 1 cases in decodeInetDiag; the
// first dial then disagrees with the reader.
func TestWildcardV6OnlyIPv4Oracle(t *testing.T) {
	tcp, udp := openRealSockets(t)

	states, err := wildcardV6Only(protoTCP)
	assert.NilError(t, err)
	for _, s := range tcp {
		st := states[s.port]
		assert.Assert(t, st == v6onlyOff || st == v6onlyOn, "%s: reader said %d", s.name, st)
		c, err := stdnet.DialTimeout("tcp4", stdnet.JoinHostPort("127.0.0.1", itoa(s.port)), time.Second)
		if c != nil {
			_ = c.Close()
		}
		if st == v6onlyOff {
			assert.NilError(t, err, "%s: reader said dual-stack, IPv4 dial failed", s.name)
		} else {
			assert.Assert(t, err != nil, "%s: reader said IPv6-only, IPv4 dial succeeded", s.name)
		}
	}

	ustates := readUDPStates(t)
	for _, s := range udp {
		st := ustates[s.port]
		assert.Assert(t, st == v6onlyOff || st == v6onlyOn, "%s: reader said %d", s.name, st)
		c, err := stdnet.Dial("udp4", stdnet.JoinHostPort("127.0.0.1", itoa(s.port)))
		assert.NilError(t, err)
		_, err = c.Write([]byte("syver"))
		_ = c.Close()
		assert.NilError(t, err)
		assert.NilError(t, s.udp.SetReadDeadline(time.Now().Add(time.Second)))
		buf := make([]byte, 16)
		n, _, rerr := s.udp.ReadFrom(buf)
		if st == v6onlyOff {
			assert.NilError(t, rerr, "%s: reader said dual-stack, IPv4 datagram not received", s.name)
			assert.Equal(t, string(buf[:n]), "syver")
		} else {
			assert.Assert(t, rerr != nil, "%s: reader said IPv6-only, IPv4 datagram received", s.name)
		}
	}
}

func itoa(p uint32) string { return strconv.FormatUint(uint64(p), 10) }
