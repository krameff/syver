package system

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"github.com/krameff/syver/util"
)

type Addr interface {
	Address() string
	Exists() (bool, error)
	Reachable() (bool, error)
}

type DefAddr struct {
	address      string
	LocalAddress string
	Timeout      int
}

func NewDefAddr(_ context.Context, address string, system *System, config util.Config) Addr {
	addr := normalizeAddress(address)
	return &DefAddr{
		address:      addr,
		LocalAddress: config.LocalAddress,
		Timeout:      config.TimeOutMilliSeconds(),
	}
}

func (a *DefAddr) ID() string {
	return a.address
}
func (a *DefAddr) Address() string {
	return a.address
}
func (a *DefAddr) Exists() (bool, error) { return a.Reachable() }

func (a *DefAddr) Reachable() (bool, error) {
	network, address := splitAddress(a.address)

	// The local address must match the network's family: udp4 and udp6 with a
	// TCPAddr fail before any packet is sent, which used to read as
	// unreachable.
	var localAddr net.Addr
	if strings.HasPrefix(network, "udp") {
		localAddr = &net.UDPAddr{IP: net.ParseIP(a.LocalAddress)}
	} else {
		localAddr = &net.TCPAddr{IP: net.ParseIP(a.LocalAddress)}
	}
	d := net.Dialer{LocalAddr: localAddr, Timeout: time.Duration(a.Timeout) * time.Millisecond}
	conn, err := d.Dial(network, address)
	if err != nil {
		// Refused, timed out, no route, or a name that does not resolve:
		// that is the answer to "is it reachable", not a failure to ask.
		//
		// An address that can never be dialled as written is also reported
		// as not reachable, for compatibility: `syver add addr 127.0.0.1`
		// relies on it to write a `reachable: false` entry. It is logged so
		// it is no longer silent.
		if isAddrConfigError(err) {
			log.Printf("[WARN] addr %s can never be reached as written: %v", a.address, err)
		}
		return false, nil
	}
	conn.Close()
	return true, nil
}

// isAddrConfigError reports whether a dial failed because of the address as
// written rather than the state of the network: an unknown network name, a
// missing or invalid port, a malformed host, or a local address that does not
// fit the network. Those can never succeed, however the network behaves.
//
// One *net.AddrError is excluded: "no suitable address found" means the host
// has no address in the requested family (tcp4 to an IPv6 literal or an
// AAAA-only name, tcp6 to an IPv4 one). That is a real "not reachable over
// this network" answer. The net package does not export that error, so it is
// matched by its message.
func isAddrConfigError(err error) bool {
	var unknownNet net.UnknownNetworkError
	if errors.As(err, &unknownNet) {
		return true
	}
	var addrErr *net.AddrError
	return errors.As(err, &addrErr) && addrErr.Err != noSuitableAddress
}

const noSuitableAddress = "no suitable address found"

func splitAddress(fulladdress string) (network, address string) {
	split := strings.SplitN(fulladdress, "://", 2)
	if len(split) == 2 {
		return split[0], split[1]
	}
	return "tcp", fulladdress
}

func normalizeAddress(fulladdress string) string {
	net, addr := splitAddress(fulladdress)
	return net + "://" + addr
}
