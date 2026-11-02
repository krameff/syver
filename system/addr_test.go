package system

import (
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
)

// captureAddrLog redirects the standard logger for the rest of the test. Not
// parallel-safe: the logger is process-wide.
func captureAddrLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func newTestAddr(address string) Addr {
	return NewDefAddr(context.Background(), address, nil, util.Config{Timeout: time.Second})
}

func TestAddrReachable(t *testing.T) {
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	t.Cleanup(func() { _ = tcp.Close() })
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	t.Cleanup(func() { _ = udp.Close() })
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	closedAddr := closed.Addr().String()
	_ = closed.Close()

	tests := []struct {
		address       string
		wantReachable bool
		wantWarn      bool
	}{
		{"tcp://" + tcp.Addr().String(), true, false},
		{"tcp4://" + tcp.Addr().String(), true, false},
		{"udp://" + udp.LocalAddr().String(), true, false},
		{"udp4://" + udp.LocalAddr().String(), true, false},
		// Nothing listening, or a name that does not resolve, is a plain
		// "not reachable".
		{"tcp://" + closedAddr, false, false},
		{"tcp://name-that-does-not-exist.invalid:80", false, false},
		// No address in the requested family is "not reachable over this
		// network", not a mistake in the spec.
		{"tcp4://[::1]:22", false, false},
		{"tcp6://127.0.0.1:22", false, false},
		{"udp4://[::1]:53", false, false},
		// An address that can never be dialled as written is still not
		// reachable with no error, as `syver add addr 127.0.0.1` relies on,
		// but it is logged.
		{"tcp://127.0.0.1", false, true},
		{"tcp://::1:22", false, true},
		{"bogus://127.0.0.1:80", false, true},
		// A TCP local address on a unix socket: "mismatched local address type".
		{"unix://127.0.0.1:80", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			logs := captureAddrLog(t)
			reachable, err := newTestAddr(tt.address).Reachable()
			if reachable != tt.wantReachable {
				t.Errorf("reachable = %v, want %v", reachable, tt.wantReachable)
			}
			if err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			warned := strings.Contains(logs.String(), "[WARN] addr "+tt.address)
			if warned != tt.wantWarn {
				t.Errorf("WARN logged = %v, want %v; log: %q", warned, tt.wantWarn, logs.String())
			}
		})
	}
}

func TestAddrReachableUDP6(t *testing.T) {
	udp, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	}
	t.Cleanup(func() { _ = udp.Close() })

	reachable, err := newTestAddr("udp6://" + udp.LocalAddr().String()).Reachable()
	if !reachable || err != nil {
		t.Fatalf("Reachable() = %v, %v; want true, nil", reachable, err)
	}
}
