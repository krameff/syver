package system

import (
	"context"
	"net"
	"strings"

	"github.com/krameff/syver/util"
)

type Interface interface {
	Name() string
	Exists() (bool, error)
	Addrs() ([]string, error)
	MTU() (int, error)
}

type DefInterface struct {
	name   string
	loaded bool
	exists bool
	iface  *net.Interface
	err    error
}

func NewDefInterface(_ context.Context, name string, systei *System, config util.Config) Interface {
	return &DefInterface{
		name: name,
	}
}

func (i *DefInterface) setup() error {
	if i.loaded {
		return i.err
	}
	i.loaded = true

	iface, err := net.InterfaceByName(i.name)
	if err != nil {
		i.exists = false
		i.err = err
		return i.err
	}
	i.iface = iface
	i.exists = true
	return nil
}

func (i *DefInterface) ID() string {
	return i.name
}

func (i *DefInterface) Name() string {
	return i.name
}

func (i *DefInterface) Exists() (bool, error) {
	if err := i.setup(); err != nil {
		if interfaceLookupRanAndFoundNothing(err) {
			return false, nil
		}
		return false, err
	}

	return i.exists, nil
}

// interfaceLookupRanAndFoundNothing distinguishes "net.InterfaceByName's
// platform syscall succeeded and simply found no interface with this name"
// from "the syscall itself failed" (permission, driver, or transport
// error). Only the former is safe to report as (false, nil) -- the latter
// means nothing was actually learned about whether the interface exists,
// which matters most on Windows, where interfaces use friendly names
// ("Ethernet", "Loopback Pseudo-Interface 1") rather than eth0-style names,
// so a ported Linux spec is likely to hit this path routinely. See FEAT-010
// SW-8 / Trap 1.
//
// The match is on Go's own error text ("no such network interface"), not an
// OS-provided message. net/interface.go produces that exact string in
// cross-platform Go code, after the platform-specific interfaceTable
// syscall has already succeeded and simply not found a match -- identically
// on every GOOS. That is not localised OS output (unlike the PowerShell
// case in FEAT-010 D-7), so the match is safe across locales.
func interfaceLookupRanAndFoundNothing(err error) bool {
	return strings.Contains(err.Error(), "no such network interface")
}

func (i *DefInterface) Addrs() ([]string, error) {
	if err := i.setup(); err != nil {
		return nil, err
	}

	addrs, err := i.iface.Addrs()
	if err != nil {
		return nil, err
	}

	var ret []string
	for _, addr := range addrs {
		ret = append(ret, addr.String())
	}
	return ret, nil
}

func (i *DefInterface) MTU() (int, error) {
	if err := i.setup(); err != nil {
		return 0, err
	}

	return i.iface.MTU, nil
}
