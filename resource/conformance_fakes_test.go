package resource

import (
	"context"
	"io"
	"strings"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
)

// This file is the FEAT-007 T-2 "shared system.System fake": one place that
// wires a fully-populated *system.System, instead of hand-rolling
// per-field structs. It's used by the conformance suite (conformance_test.go),
// which drives generically off Descriptors() and so needs every New<Type>
// field wired at once, not just one at a time.
//
// The existing single-field pattern in port_test.go/process_test.go
// (newFakePort, newFakeProcess) stays as-is deliberately: those tests need
// per-scenario configurable return values (a specific PID list, a specific
// error) to exercise specific Validate() branches, which a shared minimal
// fake set isn't the right tool for. This file's fakes return fixed,
// unconfigurable values -- good enough for "does every registered type's
// Validate() run without error and respect Skip", which is all the
// conformance suite needs.

type conformanceAddr struct{}

func (conformanceAddr) Address() string          { return "addr-fake" }
func (conformanceAddr) Exists() (bool, error)    { return true, nil }
func (conformanceAddr) Reachable() (bool, error) { return true, nil }

type conformanceInterface struct{}

func (conformanceInterface) Name() string             { return "interface-fake" }
func (conformanceInterface) Exists() (bool, error)    { return true, nil }
func (conformanceInterface) Addrs() ([]string, error) { return []string{"127.0.0.1"}, nil }
func (conformanceInterface) MTU() (int, error)        { return 1500, nil }

type conformanceService struct{}

func (conformanceService) Service() string              { return "service-fake" }
func (conformanceService) Exists() (bool, error)        { return true, nil }
func (conformanceService) Enabled() (bool, error)       { return true, nil }
func (conformanceService) Running() (bool, error)       { return true, nil }
func (conformanceService) RunLevels() ([]string, error) { return []string{}, nil }

type conformanceCommand struct{}

func (conformanceCommand) Command() string          { return "command-fake" }
func (conformanceCommand) Exists() (bool, error)    { return true, nil }
func (conformanceCommand) ExitStatus() (int, error) { return 0, nil }
func (conformanceCommand) Stdout() (io.Reader, error) {
	return strings.NewReader(""), nil
}
func (conformanceCommand) Stderr() (io.Reader, error) {
	return strings.NewReader(""), nil
}

type conformanceGroup struct{}

func (conformanceGroup) Groupname() string     { return "group-fake" }
func (conformanceGroup) Exists() (bool, error) { return true, nil }
func (conformanceGroup) GID() (int, error)     { return 0, nil }

type conformanceHTTP struct{}

func (conformanceHTTP) HTTP() string         { return "http-fake" }
func (conformanceHTTP) Status() (int, error) { return 200, nil }
func (conformanceHTTP) Headers() (io.Reader, error) {
	return strings.NewReader(""), nil
}
func (conformanceHTTP) Body() (io.Reader, error) {
	return strings.NewReader(""), nil
}
func (conformanceHTTP) Exists() (bool, error)     { return true, nil }
func (conformanceHTTP) SetAllowInsecure(bool)     {}
func (conformanceHTTP) SetNoFollowRedirects(bool) {}
func (conformanceHTTP) Close() error              { return nil }

type conformanceKernelParam struct{}

func (conformanceKernelParam) Key() string            { return "kernel-param-fake" }
func (conformanceKernelParam) Exists() (bool, error)  { return true, nil }
func (conformanceKernelParam) Value() (string, error) { return "1", nil }

type conformanceFile struct{}

func (conformanceFile) Path() string          { return "file-fake" }
func (conformanceFile) Exists() (bool, error) { return true, nil }
func (conformanceFile) Contents() (io.Reader, error) {
	return strings.NewReader(""), nil
}
func (conformanceFile) Mode() (string, error)     { return "0644", nil }
func (conformanceFile) Size() (int, error)        { return 0, nil }
func (conformanceFile) Filetype() (string, error) { return "file", nil }
func (conformanceFile) Owner() (string, error)    { return "root", nil }
func (conformanceFile) Uid() (int, error)         { return 0, nil }
func (conformanceFile) Group() (string, error)    { return "root", nil }
func (conformanceFile) Gid() (int, error)         { return 0, nil }
func (conformanceFile) LinkedTo() (string, error) { return "", nil }
func (conformanceFile) Md5() (string, error)      { return "", nil }
func (conformanceFile) Sha256() (string, error)   { return "", nil }
func (conformanceFile) Sha512() (string, error)   { return "", nil }

type conformanceMount struct{}

func (conformanceMount) MountPoint() string          { return "mount-fake" }
func (conformanceMount) Exists() (bool, error)       { return true, nil }
func (conformanceMount) Opts() ([]string, error)     { return []string{}, nil }
func (conformanceMount) VfsOpts() ([]string, error)  { return []string{}, nil }
func (conformanceMount) Source() (string, error)     { return "", nil }
func (conformanceMount) Filesystem() (string, error) { return "", nil }
func (conformanceMount) Usage() (int, error)         { return 0, nil }

type conformanceProcess struct{}

func (conformanceProcess) Executable() string        { return "process-fake" }
func (conformanceProcess) Exists() (bool, error)     { return true, nil }
func (conformanceProcess) Running() (bool, error)    { return true, nil }
func (conformanceProcess) Pids() ([]int, error)      { return []int{1}, nil }
func (conformanceProcess) Status() ([]string, error) { return []string{}, nil }
func (conformanceProcess) User() ([]string, error)   { return []string{}, nil }

type conformancePackage struct{}

func (conformancePackage) Name() string                { return "package-fake" }
func (conformancePackage) Exists() (bool, error)       { return true, nil }
func (conformancePackage) Installed() (bool, error)    { return true, nil }
func (conformancePackage) Versions() ([]string, error) { return []string{}, nil }

type conformanceRegistry struct{}

func (conformanceRegistry) Key() string            { return "registry-fake" }
func (conformanceRegistry) Exists() (bool, error)  { return true, nil }
func (conformanceRegistry) Value() (string, error) { return "", nil }
func (conformanceRegistry) Type() (string, error)  { return "", nil }
func (conformanceRegistry) SetView(string) error   { return nil }

type conformancePort struct{}

func (conformancePort) Port() string             { return "port-fake" }
func (conformancePort) Exists() (bool, error)    { return true, nil }
func (conformancePort) Listening() (bool, error) { return true, nil }
func (conformancePort) IP() ([]string, error)    { return []string{}, nil }
func (conformancePort) PID() ([]int, error)      { return []int{}, nil }

type conformanceSyverfile struct{}

func (conformanceSyverfile) Path() string          { return "syverfile-fake" }
func (conformanceSyverfile) Exists() (bool, error) { return true, nil }

type conformanceUser struct{}

func (conformanceUser) Username() string          { return "user-fake" }
func (conformanceUser) Exists() (bool, error)     { return true, nil }
func (conformanceUser) UID() (int, error)         { return 0, nil }
func (conformanceUser) GID() (int, error)         { return 0, nil }
func (conformanceUser) Groups() ([]string, error) { return []string{}, nil }
func (conformanceUser) Home() (string, error)     { return "/home/fake", nil }
func (conformanceUser) Shell() (string, error)    { return "/bin/bash", nil }

type conformanceDNS struct{}

func (conformanceDNS) Host() string              { return "dns-fake" }
func (conformanceDNS) Addrs() ([]string, error)  { return []string{}, nil }
func (conformanceDNS) Resolvable() (bool, error) { return true, nil }
func (conformanceDNS) Exists() (bool, error)     { return true, nil }
func (conformanceDNS) Server() string            { return "" }
func (conformanceDNS) Qtype() string             { return "" }

// newConformanceSystem builds a *system.System with every New<Type> field
// wired to one of the fakes above, ignoring the key/config arguments
// entirely -- good enough for "run Validate() and check Skip is honoured",
// which is all the conformance suite (T-1) needs from it.
func newConformanceSystem() *system.System {
	return &system.System{
		NewPackage: func(context.Context, string, *system.System, util.Config) system.Package { return conformancePackage{} },
		NewFile:    func(context.Context, string, *system.System, util.Config) system.File { return conformanceFile{} },
		NewAddr:    func(context.Context, string, *system.System, util.Config) system.Addr { return conformanceAddr{} },
		NewPort:    func(context.Context, string, *system.System, util.Config) system.Port { return conformancePort{} },
		NewService: func(context.Context, string, *system.System, util.Config) system.Service { return conformanceService{} },
		NewUser:    func(context.Context, string, *system.System, util.Config) system.User { return conformanceUser{} },
		NewGroup:   func(context.Context, string, *system.System, util.Config) system.Group { return conformanceGroup{} },
		NewCommand: func(context.Context, string, *system.System, util.Config) system.Command { return conformanceCommand{} },
		NewDNS:     func(context.Context, string, *system.System, util.Config) system.DNS { return conformanceDNS{} },
		NewProcess: func(context.Context, string, *system.System, util.Config) system.Process { return conformanceProcess{} },
		NewSyverfile: func(context.Context, string, *system.System, util.Config) system.Syverfile {
			return conformanceSyverfile{}
		},
		NewKernelParam: func(context.Context, string, *system.System, util.Config) system.KernelParam {
			return conformanceKernelParam{}
		},
		NewMount: func(context.Context, string, *system.System, util.Config) system.Mount { return conformanceMount{} },
		NewInterface: func(context.Context, string, *system.System, util.Config) system.Interface {
			return conformanceInterface{}
		},
		NewHTTP: func(context.Context, string, *system.System, util.Config) system.HTTP { return conformanceHTTP{} },
		NewRegistry: func(context.Context, string, *system.System, util.Config) system.Registry {
			return conformanceRegistry{}
		},
	}
}
