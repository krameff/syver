package system

import (
	"context"
	"errors"
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
	port     string
	sysPorts map[string][]net.ConnectionStat
	err      error
}

func NewDefPort(_ context.Context, port string, system *System, config util.Config) Port {
	p := normalizePort(port)
	sysPorts, err := system.Ports()
	return &DefPort{
		port:     p,
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
	if _, ok := p.sysPorts[p.port]; ok {
		return true, nil
	}
	return false, nil
}

func (p *DefPort) IP() ([]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	var ips []string
	for _, entry := range p.sysPorts[p.port] {
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
	for _, entry := range p.sysPorts[p.port] {
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
