package system

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	util2 "github.com/krameff/syver/util"
)

type Resource interface {
	Exists() (bool, error)
}

type System struct {
	NewPackage     func(context.Context, string, *System, util2.Config) Package
	NewFile        func(context.Context, string, *System, util2.Config) File
	NewAddr        func(context.Context, string, *System, util2.Config) Addr
	NewPort        func(context.Context, string, *System, util2.Config) Port
	NewService     func(context.Context, string, *System, util2.Config) Service
	NewUser        func(context.Context, string, *System, util2.Config) User
	NewGroup       func(context.Context, string, *System, util2.Config) Group
	NewCommand     func(context.Context, string, *System, util2.Config) Command
	NewDNS         func(context.Context, string, *System, util2.Config) DNS
	NewProcess     func(context.Context, string, *System, util2.Config) Process
	NewSyverfile   func(context.Context, string, *System, util2.Config) Syverfile
	NewKernelParam func(context.Context, string, *System, util2.Config) KernelParam
	NewMount       func(context.Context, string, *System, util2.Config) Mount
	NewInterface   func(context.Context, string, *System, util2.Config) Interface
	NewHTTP        func(context.Context, string, *System, util2.Config) HTTP
	NewRegistry    func(context.Context, string, *System, util2.Config) Registry
	ports          map[string][]net.ConnectionStat
	portsErr       error
	portsOnce      sync.Once
	procMap        map[string][]*process.Process
	procOnce       sync.Once
}

func (s *System) Ports() (map[string][]net.ConnectionStat, error) {
	s.portsOnce.Do(func() {
		s.ports, s.portsErr = GetPorts()
	})

	return s.ports, s.portsErr
}

func (s *System) ProcMap() (map[string][]*process.Process, error) {
	var err error

	s.procOnce.Do(func() {
		s.procMap, err = GetProcs()
	})

	return s.procMap, err
}

func New(packageManager string) *System {
	sys := &System{
		NewFile:        NewDefFile,
		NewAddr:        NewDefAddr,
		NewPort:        NewDefPort,
		NewUser:        NewDefUser,
		NewGroup:       NewDefGroup,
		NewCommand:     NewDefCommand,
		NewDNS:         NewDefDNS,
		NewProcess:     NewDefProcess,
		NewSyverfile:   NewDefSyverfile,
		NewKernelParam: NewDefKernelParam,
		NewMount:       NewDefMount,
		NewInterface:   NewDefInterface,
		NewHTTP:        NewDefHTTP,
		NewRegistry:    NewDefRegistry,
	}

	sys.detectService()
	sys.detectPackage(packageManager)

	return sys
}

// detectPackage adds the correct package creation function to a System struct
func (sys *System) detectPackage(p string) {
	sys.NewPackage = resolveNewPackage(p, DetectPackageManager, runtime.GOOS)
}

// resolveNewPackage decides which Package constructor detectPackage assigns.
// The decision is split out as a pure function -- goos is a parameter rather
// than a direct read of runtime.GOOS, and detect is a callback rather than a
// direct call to DetectPackageManager -- so the Windows branch below is
// table-testable from Linux (see system/system_test.go).
//
// explicit is the --package flag value; detect is consulted only when
// explicit is not one of the four supported managers, matching the original
// (pre-Windows-fix) behaviour exactly. That ordering also satisfies the
// requirement that an explicit --package flag is never overridden by GOOS:
// "--package rpm" resolves to NewRpmPackage before goos is even considered.
func resolveNewPackage(explicit string, detect func() string, goos string) func(context.Context, string, *System, util2.Config) Package {
	p := explicit
	if p != "dpkg" && p != "apk" && p != "pacman" && p != "rpm" {
		p = detect()
	}
	switch p {
	case "dpkg":
		return NewDebPackage
	case "apk":
		return NewAlpinePackage
	case "pacman":
		return NewPacmanPackage
	case "rpm":
		return NewRpmPackage
	}
	// p is empty: nothing was detected (and no supported value was passed
	// explicitly either). On Windows there is no dpkg/apk/pacman/rpm to run
	// against -- package_rpm.go's setup() comment explains why a missing
	// package-manager binary silently means "not installed" on every other
	// GOOS, and on Windows rpm is *always* missing, so falling through to it
	// here would mean every `installed: false` assertion passes having
	// checked nothing. Route to NullPackage instead, so the check errors
	// with ErrNullPackage rather than silently answering false. On every
	// other GOOS this preserves the pre-existing default of RpmPackage.
	if goos == "windows" {
		return NewNullPackage
	}
	return NewRpmPackage
}

// detectService adds the correct service creation function to a System struct
func (sys *System) detectService() {
	switch DetectService() {
	case "upstart":
		sys.NewService = NewServiceUpstart
	case "systemd":
		sys.NewService = NewServiceSystemd
	case "systemdlegacy":
		sys.NewService = NewServiceSystemdLegacy
	case "alpineinit":
		sys.NewService = NewAlpineServiceInit
	case "windows":
		sys.NewService = NewServiceWindows
	default:
		sys.NewService = NewServiceInit
	}
}

// SupportedPackageManagers is a list of package managers we support
func SupportedPackageManagers() []string {
	return []string{"apk", "dpkg", "pacman", "rpm"}
}

// IsSupportedPackageManager determines if p is a supported package manager
func IsSupportedPackageManager(p string) bool {
	for _, m := range SupportedPackageManagers() {
		if m == p {
			return true
		}
	}

	return false
}

// detector holds everything the distro, package-manager and service-manager
// detection reads from the host, so tests can point it at a fixture tree and a
// fake PATH instead of the machine running the tests. root is prepended to
// every absolute path probed; the host detector uses "" so the paths read are
// exactly "/etc/...".
type detector struct {
	root       string
	goos       string
	hasCommand func(string) bool
}

func hostDetector() detector {
	return detector{root: "", goos: runtime.GOOS, hasCommand: HasCommand}
}

func (d detector) readFile(name string) ([]byte, error) {
	return os.ReadFile(d.root + name)
}

func (d detector) exists(name string) bool {
	_, err := os.Stat(d.root + name)
	return err == nil
}

// DetectPackageManager attempts to detect whether or not the system is using
// "dpkg", "rpm", "apk", or "pacman" package managers. It first attempts to
// detect the distro. If that fails, it falls back to finding package manager
// executables. If that fails, it returns the empty string.
func DetectPackageManager() string {
	return hostDetector().packageManager()
}

func (d detector) packageManager() string {
	switch d.distro() {
	case "ubuntu":
		return "dpkg"
	case "redhat":
		return "rpm"
	case "alpine":
		return "apk"
	case "arch":
		return "pacman"
	case "debian":
		return "dpkg"
	}
	for _, manager := range []string{"dpkg", "rpm", "apk", "pacman"} {
		if d.hasCommand(manager) {
			return manager
		}
	}
	return ""
}

// DetectService attempts to detect what kind of service management the system
// is using, "systemd", "upstart", "alpineinit", or "init". It looks for systemctl
// command to detect systemd, and falls back on DetectDistro otherwise. If it can't
// decide, it returns "init".
func DetectService() string {
	return hostDetector().service()
}

func (d detector) service() string {
	if d.goos == "windows" {
		return "windows"
	}
	if d.hasCommand("systemctl") {
		if d.isLegacySystemd() {
			return "systemdlegacy"
		}
		return "systemd"
	}
	// Centos Docker container doesn't run systemd, so we detect it or use init.
	switch d.distro() {
	case "ubuntu":
		return "upstart"
	case "alpine":
		return "alpineinit"
	case "arch":
		return "systemd"
	}
	return "init"
}

// DetectDistro attempts to detect which Linux distribution this computer is
// using. One of "ubuntu", "redhat" (including Centos), "alpine", "arch", or
// "debian". If it can't decide, it returns an empty string.
func DetectDistro() string {
	return hostDetector().distro()
}

func (d detector) distro() string {
	if b, e := d.readFile("/etc/lsb-release"); e == nil && bytes.Contains(b, []byte("Ubuntu")) {
		return "ubuntu"
	} else if d.isRedhat() {
		return "redhat"
	} else if d.exists("/etc/alpine-release") {
		return "alpine"
	} else if d.exists("/etc/arch-release") {
		return "arch"
	} else if d.exists("/etc/debian_version") {
		return "debian"
	}
	return ""
}

// HasCommand returns whether or not an executable by this name is on the PATH.
func HasCommand(cmd string) bool {
	if _, err := exec.LookPath(cmd); err == nil {
		return true
	}
	return false
}

func (d detector) isLegacySystemd() bool {
	if b, err := d.readFile("/etc/debian_version"); err == nil {
		return isLegacyDebianVersion(b)
	}
	return false
}

// isLegacyDebianVersion reports whether the contents of /etc/debian_version
// name a Debian release older than 9 (stretch), whose systemd needs the legacy
// service checks. Anything without a numeric major before a "." is not legacy.
func isLegacyDebianVersion(b []byte) bool {
	i := bytes.Index(b, []byte("."))
	if i < 0 {
		return false
	}
	if major, err := strconv.Atoi(string(b[:i])); err == nil {
		return major < 9
	}
	return false
}

func (d detector) isRedhat() bool {
	return d.exists("/etc/redhat-release") || d.exists("/etc/system-release")
}
