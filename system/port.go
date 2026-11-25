package system

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/net"

	"github.com/krameff/syver/util"
)

type Port interface {
	Port() string
	Exists() (bool, error)
	Listening() (bool, error)
	IP() ([]string, error)
	PID() ([]int, error)
}

type DefPort struct {
	port     string // as written in the spec, with the default network filled in
	key      string // the GetPorts key to look up: port with its network resolved
	sysPorts map[string][]net.ConnectionStat
	err      error
}

// ErrUnknownPortNetwork is returned by every DefPort method when the port's
// network is not one GetPorts can answer for, such as a typo like "tpc:22".
// Before it existed such a port could never be found, so `listening: false`
// passed without checking anything (goss-org/goss#1006).
var ErrUnknownPortNetwork = errors.New("unknown port network")

// portNetworks maps each network a spec may name to the GetPorts key prefix
// that holds its listeners. "tcp4" and "udp4" are aliases, not approximations:
// GetPorts fills "tcp" and "udp" from the IPv4-only queries, plus any
// dual-stack [::] socket, which accepts IPv4 too (addDualStack).
var portNetworks = map[string]string{
	"tcp": "tcp", "tcp4": "tcp", "tcp6": "tcp6",
	"udp": "udp", "udp4": "udp", "udp6": "udp6",
}

// portNetworkNames is the list the error message offers, in a fixed order.
const portNetworkNames = "tcp, tcp4, tcp6, udp, udp4, udp6"

func NewDefPort(_ context.Context, port string, system *System, config util.Config) Port {
	p := normalizePort(port)
	network, addr := splitPort(p)
	prefix, ok := portNetworks[network]
	if !ok {
		// Checked before system.Ports() so a mistyped spec is reported as
		// itself, never masked by an unrelated connection-table error.
		return &DefPort{
			port: p,
			err: fmt.Errorf("%w %q in port %q: must be one of %s",
				ErrUnknownPortNetwork, network, port, portNetworkNames),
		}
	}
	sysPorts, err := system.Ports()
	return &DefPort{
		port:     p,
		key:      prefix + ":" + addr,
		sysPorts: sysPorts,
		err:      err,
	}
}

func splitPort(fullport string) (network, port string) {
	split := strings.SplitN(fullport, ":", 2)
	if len(split) == 2 {
		return split[0], split[1]
	}
	return "tcp", fullport

}

func normalizePort(fullport string) string {
	net, addr := splitPort(fullport)
	return net + ":" + addr
}

func (p *DefPort) Port() string {
	return p.port
}

func (p *DefPort) Exists() (bool, error) { return p.Listening() }

func (p *DefPort) Listening() (bool, error) {
	if p.err != nil {
		return false, p.err
	}
	if _, ok := p.sysPorts[p.key]; ok {
		return true, nil
	}
	return false, nil
}

func (p *DefPort) IP() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	var ips []string
	for _, entry := range p.sysPorts[p.key] {
		ips = append(ips, entry.Laddr.IP)
	}
	return ips, nil
}

// PID returns the owning PID of every connection bound to this port. A PID
// of 0 means gopsutil couldn't resolve an owner (e.g. insufficient
// permissions to map the socket inode to a process) and is omitted rather
// than reported as a spurious "pid: 0".
func (p *DefPort) PID() ([]int, error) {
	if p.err != nil {
		return nil, p.err
	}
	var pids []int
	for _, entry := range p.sysPorts[p.key] {
		if entry.Pid == 0 {
			continue
		}
		pids = append(pids, int(entry.Pid))
	}
	return pids, nil
}

// connectionsByKind is indirected through a package-level var so tests can
// substitute canned data without touching the real OS connection table.
var connectionsByKind = net.ConnectionsWithoutUids

// GetPorts returns the set of listening TCP/UDP ports known to the OS's
// connection-table backend. Failures from individual protocol lookups are
// joined and returned rather than silently discarded: a missing/unreadable
// /proc/net/{tcp,udp}{,6} file is treated as "no ports" with no error, but a
// file that's readable yet contains a line that can't be parsed (e.g. an
// unexpected IP/port encoding) does return an error, which would otherwise
// present as every port on that protocol simply not listening. Each protocol
// is queried separately (rather than via a single "all" call) specifically
// to preserve this per-protocol error isolation.
func GetPorts() (map[string][]net.ConnectionStat, error) {
	ports := make(map[string][]net.ConnectionStat)
	var errs []error

	addConns := func(kind, prefix string, listenOnly bool) {
		conns, err := connectionsByKind(kind)
		errs = append(errs, err)
		for _, entry := range conns {
			if listenOnly && entry.Status != "LISTEN" {
				continue
			}
			port := strconv.FormatUint(uint64(entry.Laddr.Port), 10)
			ports[prefix+":"+port] = append(ports[prefix+":"+port], entry)
		}
	}

	addConns("tcp4", "tcp", true)
	addConns("tcp6", "tcp6", true)
	addConns("udp4", "udp", false)
	addConns("udp6", "udp6", false)

	addDualStack(ports, "tcp", "tcp6", protoTCP)
	addDualStack(ports, "udp", "udp6", protoUDP)

	return ports, errors.Join(errs...)
}

// v6onlyState says whether a socket on the IPv6 unspecified address "::"
// also accepts IPv4.
type v6onlyState int

const (
	v6onlyUnknown v6onlyState = iota // the kernel could not say
	v6onlyOff                        // dual-stack: accepts IPv4 too
	v6onlyOn                         // IPv6 only
)

// IP protocol numbers passed to wildcardV6Only.
const (
	protoTCP uint8 = 6
	protoUDP uint8 = 17
)

// wildcardV6OnlyFn is indirected through a package-level var so tests can
// substitute canned states without asking the kernel.
var wildcardV6OnlyFn = wildcardV6Only

// addDualStack counts a dual-stack socket for its IPv4 key. On Linux a socket
// on [::] with IPV6_V6ONLY=0 also accepts IPv4, but the kernel lists it only
// in the IPv6 table. For every port in v6 with such a socket, a copy of each
// [::] entry is added under v4 with the address 0.0.0.0, unless v4 already
// holds a 0.0.0.0 entry for that port. Where the kernel cannot say whether a
// socket is dual-stack, nothing is added and the reason is logged at DEBUG;
// that is never an error.
func addDualStack(ports map[string][]net.ConnectionStat, v4, v6 string, proto uint8) {
	var wild []string
	for key, entries := range ports {
		if !strings.HasPrefix(key, v6+":") {
			continue
		}
		for _, e := range entries {
			if e.Laddr.IP == "::" {
				wild = append(wild, key)
				break
			}
		}
	}
	if len(wild) == 0 {
		return
	}
	if hp := os.Getenv("HOST_PROC"); hp != "" && path.Clean(hp) != "/proc" {
		// gopsutil reads that /proc, possibly another network namespace,
		// while sock_diag answers for this process's own. HOST_PROC is a
		// Linux path, so path.Clean: filepath.Clean turns "/proc/" into
		// "\proc" on Windows.
		debugOnce("[DEBUG] port: HOST_PROC is set; dual-stack listeners are not counted for tcp:/udp:")
		return
	}
	states, err := wildcardV6OnlyFn(proto)
	if err != nil {
		debugOnce(fmt.Sprintf("[DEBUG] port: could not read IPV6_V6ONLY for %s sockets on [::]: %v; "+
			"dual-stack listeners are not counted for %s:N, so those results are unchanged (check %s:N)", v4, err, v4, v6))
		return
	}
	for _, key := range wild {
		port := strings.TrimPrefix(key, v6+":")
		v4key := v4 + ":" + port
		if hasIP(ports[v4key], "0.0.0.0") {
			continue
		}
		for _, e := range ports[key] {
			if e.Laddr.IP != "::" {
				continue
			}
			switch states[e.Laddr.Port] {
			case v6onlyOn:
				continue
			case v6onlyUnknown:
				debugOnce(fmt.Sprintf("[DEBUG] port: [::]:%s has no IPV6_V6ONLY from sock_diag; not counted for %s (check %s)",
					port, v4key, key))
				continue
			}
			c := e
			c.Laddr.IP = "0.0.0.0"
			ports[v4key] = append(ports[v4key], c)
		}
	}
}

func hasIP(entries []net.ConnectionStat, ip string) bool {
	for _, e := range entries {
		if e.Laddr.IP == ip {
			return true
		}
	}
	return false
}

// debugLogged holds the DEBUG lines already written, so each is logged once
// per process even when GetPorts runs more than once (add and autoadd).
var debugLogged sync.Map

func debugOnce(msg string) {
	if _, dup := debugLogged.LoadOrStore(msg, true); !dup {
		log.Print(msg)
	}
}
