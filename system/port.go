package system

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
// GetPorts fills "tcp" and "udp" from exactly those IPv4-only queries.
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

	return ports, errors.Join(errs...)
}
