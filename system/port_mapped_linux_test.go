package system

import (
	"errors"
	stdnet "net"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"gotest.tools/v3/assert"
)

// mappedAddr returns the IPv4-mapped IPv6 address ::ffff:a.b.c.d.
func mappedAddr(a, b, c, d byte) [16]byte {
	return [16]byte{10: 0xff, 11: 0xff, 12: a, 13: b, 14: c, 15: d}
}

// socket6 opens an AF_INET6 socket of type typ and closes it at cleanup. It
// skips the test only when the kernel has no IPv6 at all.
func socket6(t *testing.T, typ int) int {
	t.Helper()
	fd, err := unix.Socket(unix.AF_INET6, typ|unix.SOCK_CLOEXEC, 0)
	if errors.Is(err, unix.EAFNOSUPPORT) {
		t.Skipf("no IPv6 sockets on this kernel: %v", err)
	}
	assert.NilError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })
	return fd
}

// bind6 binds fd to addr:port (port 0 for any) and returns the bound port.
func bind6(t *testing.T, fd int, addr [16]byte, port int) int {
	t.Helper()
	assert.NilError(t, unix.Bind(fd, &unix.SockaddrInet6{Addr: addr, Port: port}))
	sa, err := unix.Getsockname(fd)
	assert.NilError(t, err)
	return sa.(*unix.SockaddrInet6).Port
}

// TestMappedSocketKernelInvariant pins the premise that lets addMapped count a
// mapped socket without asking sock_diag for its IPV6_V6ONLY. Two rules:
//   - an IPV6_V6ONLY=1 socket cannot bind an IPv4-mapped address (EINVAL), so
//     a mapped socket is always IPV6_V6ONLY=0 and accepts IPv4;
//   - once any IPv6 socket is bound, IPV6_V6ONLY can no longer be set (EINVAL),
//     so that cannot change later. This one is not specific to mapped
//     sockets, which the [::] control row shows.
//
// If a kernel ever breaks either rule this test fails, and addMapped then
// needs to read IPV6_V6ONLY per socket, as addDualStack does.
func TestMappedSocketKernelInvariant(t *testing.T) {
	for _, typ := range []struct {
		name string
		typ  int
	}{{"tcp", unix.SOCK_STREAM}, {"udp", unix.SOCK_DGRAM}} {
		t.Run(typ.name, func(t *testing.T) {
			// Revert-proof: assert nil here instead of EINVAL; it fails.
			t.Run("IPV6_V6ONLY=1 then a mapped bind is EINVAL", func(t *testing.T) {
				fd := socket6(t, typ.typ)
				assert.NilError(t, unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 1))
				err := unix.Bind(fd, &unix.SockaddrInet6{Addr: mappedAddr(127, 0, 0, 1)})
				assert.Assert(t, errors.Is(err, unix.EINVAL), "bind: %v", err)
			})

			// Revert-proof: expect 1 here instead of 0; it fails.
			t.Run("after a mapped bind IPV6_V6ONLY is 0", func(t *testing.T) {
				fd := socket6(t, typ.typ)
				bind6(t, fd, mappedAddr(127, 0, 0, 1), 0)
				v, err := unix.GetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY)
				assert.NilError(t, err)
				assert.Equal(t, v, 0)
			})

			// Revert-proof: assert nil here instead of EINVAL; both rows fail.
			for _, b := range []struct {
				name string
				addr [16]byte
			}{
				{"mapped", mappedAddr(127, 0, 0, 1)},
				{"[::] control", [16]byte{}},
			} {
				t.Run("after a "+b.name+" bind IPV6_V6ONLY cannot be set", func(t *testing.T) {
					fd := socket6(t, typ.typ)
					bind6(t, fd, b.addr, 0)
					err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 1)
					assert.Assert(t, errors.Is(err, unix.EINVAL), "setsockopt: %v", err)
				})
			}
		})
	}
}

// listenMapped opens a TCP listener on the IPv4-mapped addr and returns its fd
// and port. Go's net.Listen cannot make one: it opens an AF_INET socket for an
// IPv4 address.
func listenMapped(t *testing.T, addr [16]byte, port int) (int, int) {
	t.Helper()
	fd := socket6(t, unix.SOCK_STREAM)
	p := bind6(t, fd, addr, port)
	assert.NilError(t, unix.Listen(fd, 8))
	return fd, p
}

func portStr(p int) string { return strconv.Itoa(p) }

// TestGetPortsMappedRealSockets opens mapped sockets in this process and
// checks the real GetPorts against what an IPv4 client actually gets.
// Revert-proof: remove the addMapped calls from GetPorts; tcp:P and udp:P are
// then missing while the IPv4 dial and datagram still get through.
func TestGetPortsMappedRealSockets(t *testing.T) {
	for _, s := range []struct {
		name string
		addr [16]byte
		ip   string
	}{
		{"::ffff:127.0.0.1", mappedAddr(127, 0, 0, 1), "127.0.0.1"},
		{"::ffff:0.0.0.0", mappedAddr(0, 0, 0, 0), "0.0.0.0"},
	} {
		t.Run("tcp "+s.name, func(t *testing.T) {
			_, p := listenMapped(t, s.addr, 0)
			ports, err := GetPorts()
			assert.NilError(t, err)
			assert.Assert(t, hasIP(ports["tcp:"+portStr(p)], s.ip), "tcp:%d = %+v", p, ports["tcp:"+portStr(p)])
			assert.Assert(t, hasIP(ports["tcp6:"+portStr(p)], s.ip), "tcp6:%d = %+v", p, ports["tcp6:"+portStr(p)])

			c, err := stdnet.DialTimeout("tcp4", stdnet.JoinHostPort("127.0.0.1", portStr(p)), time.Second)
			assert.NilError(t, err, "IPv4 dial to the mapped socket")
			_ = c.Close()

			// The socket is listed under tcp6:P but accepts no IPv6 client.
			c, err = stdnet.DialTimeout("tcp6", stdnet.JoinHostPort("::1", portStr(p)), time.Second)
			if c != nil {
				_ = c.Close()
			}
			switch {
			case errors.Is(err, syscall.EADDRNOTAVAIL), errors.Is(err, syscall.ENETUNREACH):
				t.Skipf("no IPv6 loopback to dial (IPv6 disabled?): %v", err)
			default:
				assert.Assert(t, errors.Is(err, syscall.ECONNREFUSED), "IPv6 dial to a mapped socket: %v", err)
			}
		})
	}

	t.Run("udp ::ffff:127.0.0.1", func(t *testing.T) {
		fd := socket6(t, unix.SOCK_DGRAM)
		p := bind6(t, fd, mappedAddr(127, 0, 0, 1), 0)
		ports, err := GetPorts()
		assert.NilError(t, err)
		assert.Assert(t, hasIP(ports["udp:"+portStr(p)], "127.0.0.1"), "udp:%d = %+v", p, ports["udp:"+portStr(p)])

		c, err := stdnet.Dial("udp4", stdnet.JoinHostPort("127.0.0.1", portStr(p)))
		assert.NilError(t, err)
		_, err = c.Write([]byte("syver"))
		_ = c.Close()
		assert.NilError(t, err)
		assert.NilError(t, unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1}))
		buf := make([]byte, 16)
		n, _, err := unix.Recvfrom(fd, buf, 0)
		assert.NilError(t, err, "IPv4 datagram to the mapped socket")
		assert.Equal(t, string(buf[:n]), "syver")
	})

	// An IPv6-only [::] socket and a mapped wildcard share one port with no
	// SO_REUSEPORT. tcp:P must be the mapped socket alone: the [::] one is
	// not dual-stack and must not be counted. This also runs the real
	// sock_diag reader beside addMapped.
	// Revert-proof for the Fd check: make addDualStack treat v6onlyOn as
	// v6onlyOff; the copy then comes from the [::] socket and its Fd differs.
	t.Run("IPv6-only [::] beside a mapped wildcard", func(t *testing.T) {
		v6 := socket6(t, unix.SOCK_STREAM)
		assert.NilError(t, unix.SetsockoptInt(v6, unix.IPPROTO_IPV6, unix.IPV6_V6ONLY, 1))
		p := bind6(t, v6, [16]byte{}, 0)
		assert.NilError(t, unix.Listen(v6, 8))
		mfd, _ := listenMapped(t, mappedAddr(0, 0, 0, 0), p)

		ports, err := GetPorts()
		assert.NilError(t, err)
		v4 := ports["tcp:"+portStr(p)]
		assert.Equal(t, len(v4), 1, "tcp:%d = %+v", p, v4)
		assert.Equal(t, v4[0].Laddr.IP, "0.0.0.0")
		v6e := ports["tcp6:"+portStr(p)]
		assert.Assert(t, hasIP(v6e, "::") && hasIP(v6e, "0.0.0.0"), "tcp6:%d = %+v", p, v6e)
		if v4[0].Fd == 0 {
			t.Skip("gopsutil did not map the socket to its owner (Fd 0); Fd not checked")
		}
		assert.Equal(t, v4[0].Fd, uint32(mfd))
	})
}
