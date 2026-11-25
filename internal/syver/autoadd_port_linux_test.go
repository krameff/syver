package syver

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// Autoadd of a process that owns a socket on an IPv4-mapped address writes
// the port under both tcp:P and tcp6:P, each with the IPv4 address.
// Revert-proof: remove the addMapped calls from system.GetPorts; tcp:P is
// then missing from the written spec.
func TestAutoAddProcessWritesMappedPortUnderBothKeys(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	skipAutoAddOffLinux(t)

	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err == unix.EAFNOSUPPORT {
		t.Skipf("no IPv6 sockets on this kernel: %v", err)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })
	mapped := [16]byte{10: 0xff, 11: 0xff, 12: 127, 15: 1} // ::ffff:127.0.0.1
	require.NoError(t, unix.Bind(fd, &unix.SockaddrInet6{Addr: mapped}))
	require.NoError(t, unix.Listen(fd, 8))
	sa, err := unix.Getsockname(fd)
	require.NoError(t, err)
	port := strconv.Itoa(sa.(*unix.SockaddrInet6).Port)

	comm, err := os.ReadFile("/proc/self/comm")
	require.NoError(t, err)
	name := strings.TrimSpace(string(comm))

	spec := filepath.Join(t.TempDir(), "goss.yaml")
	cfg, err := util.NewConfig()
	require.NoError(t, err)
	require.NoError(t, AutoAddResources(spec, []string{name}, cfg))

	got, err := ReadJSON(spec)
	require.NoError(t, err)
	require.Contains(t, got.Processes, name)
	// Other listeners of this process, if any, are not asserted on.
	for _, key := range []string{"tcp:" + port, "tcp6:" + port} {
		p, ok := got.Ports[key]
		if !assert.True(t, ok, "%s missing from the written spec; ports: %v", key, got.Ports) {
			continue
		}
		assert.Equal(t, "true", fmt.Sprint(p.Listening), key)
		assert.Equal(t, "[127.0.0.1]", fmt.Sprint(p.IP), key)
	}
}
