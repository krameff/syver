//go:build windows
// +build windows

package system

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"github.com/krameff/syver/util"
	"golang.org/x/sys/windows"
)

// FEAT-014. `service:` reads the Service Control Manager directly. It previously
// shelled out to `Get-Service` through PowerShell once per attribute.
//
// WHY NOT golang.org/x/sys/windows/svc/mgr, WHICH EXISTS AND WOULD BE SHORTER.
// Its constructors ask for far more access than a read-only check needs:
// mgr.Connect() opens the SCM with SC_MANAGER_ALL_ACCESS and mgr.OpenService
// opens the service with SERVICE_ALL_ACCESS. **Both require administrator.**
// `Get-Service`, which this code replaces, did not.
//
// So porting to the wrapper would make a read-only check demand elevation it has
// never needed, breaking every non-admin Windows host -- and **CI could not catch
// it, because the GitHub Actions Windows runner runs as administrator.** The
// failure would surface only in production, on exactly the locked-down machines
// syver exists to validate.
//
// This is not hypothetical caution: it was argued once and re-confirmed. Syver on
// Windows is run elevated as a recorded decision, and that decision deliberately
// does NOT extend here. See FEAT-014, "The constraint stands".
//
// The rights below are the minimum that answers the questions, and the
// non-admin path is proven rather than assumed -- see the spec's evidence.
const (
	// SCM: CONNECT ONLY. Not SC_MANAGER_ALL_ACCESS, and deliberately not
	// SC_MANAGER_ENUMERATE_SERVICE either.
	//
	// FEAT-014's spec prescribed CONNECT|ENUMERATE_SERVICE. That is wrong, and it
	// was caught by running as a standard user rather than by review. Measured on
	// Windows Server 2025, `sc sdshow scmanager` reads:
	//
	//	D:(A;;CC;;;AU)(A;;CCLCRPRC;;;IU)(A;;CCLCRPRC;;;SU)(A;;CCLCRPWPRC;;;SY)(A;;KA;;;BA)...
	//
	// Authenticated Users get `CC` alone -- SC_MANAGER_CONNECT. `LC`
	// (ENUMERATE_SERVICE) is granted to `IU`, INTERACTIVE Users, and to `SU`,
	// service logons. A check running over WinRM, as a scheduled task, or from any
	// other network logon is an Authenticated User and NOT an Interactive one, so
	// asking for ENUMERATE returned ERROR_ACCESS_DENIED from OpenSCManager itself
	// and every single assertion failed.
	//
	// Nothing here enumerates: syver is asked about named services, so it calls
	// OpenService, which needs only a CONNECT handle. Asking for more than the
	// work requires is what broke it.
	//
	// **NEITHER CI NOR AN INTERACTIVE TEST COULD HAVE FOUND THIS.** CI runs as an
	// administrator, and `BA` holds `KA` (all access). An interactive non-admin
	// session would have passed too, because `IU` does carry `LC`. It takes a
	// non-admin NETWORK logon, which is exactly the shape of a real audit run.
	serviceManagerAccess = windows.SC_MANAGER_CONNECT
	// Service: read configuration and read status. Not SERVICE_ALL_ACCESS. The
	// default service descriptor grants both to Authenticated Users.
	serviceAccess = windows.SERVICE_QUERY_CONFIG | windows.SERVICE_QUERY_STATUS
)

type ServiceWindows struct {
	// ctx is retained for symmetry with the other backends and for cancellation
	// checks around the syscalls. Unlike them it bounds no subprocess: there is
	// no longer one. FEAT-014 removed the PowerShell probes entirely.
	ctx     context.Context
	service string
}

func NewServiceWindows(ctx context.Context, service string, system *System, config util.Config) Service {
	return &ServiceWindows{
		ctx:     ctx,
		service: service,
	}
}

func (s *ServiceWindows) Service() string {
	return s.service
}

// handles opens the SCM and the service read-only. The caller closes both.
//
// A service name is passed as a UTF-16 string straight to the API, so a name
// containing a SPACE works -- it could not through the old PowerShell path, and
// that was a real defect rather than a limitation of Windows. There is no command
// line here, so there is nothing to quote and no injection surface: psSingleQuote
// exists for a problem this code no longer has.
func (s *ServiceWindows) handles() (mgr windows.Handle, svc windows.Handle, err error) {
	if err := s.ctx.Err(); err != nil {
		return 0, 0, err
	}
	name, err := windows.UTF16PtrFromString(s.service)
	if err != nil {
		return 0, 0, fmt.Errorf("service name %q is not a valid Windows string: %w", s.service, err)
	}
	mgr, err = windows.OpenSCManager(nil, nil, serviceManagerAccess)
	if err != nil {
		return 0, 0, fmt.Errorf("opening the service control manager: %w", err)
	}
	svc, err = windows.OpenService(mgr, name, serviceAccess)
	if err != nil {
		windows.CloseServiceHandle(mgr)
		return 0, 0, err
	}
	return mgr, svc, nil
}

func closeHandles(mgr, svc windows.Handle) {
	if svc != 0 {
		windows.CloseServiceHandle(svc)
	}
	if mgr != 0 {
		windows.CloseServiceHandle(mgr)
	}
}

// notFound reports whether err is the SCM's "no such service". Compared as a
// typed errno rather than by message text: the message is LOCALISED, and matching
// Windows' own prose is the bug FEAT-010 removed from this file.
func notFound(err error) bool {
	return errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST)
}

// queryConfig reads QUERY_SERVICE_CONFIG. The struct's string fields are pointers
// INTO the buffer allocated here, so the returned pointer keeps that buffer alive
// and the caller must not retain the strings beyond converting them.
func (s *ServiceWindows) queryConfig(svc windows.Handle) (*windows.QUERY_SERVICE_CONFIG, error) {
	n := uint32(1024)
	for i := 0; i < 3; i++ {
		b := make([]byte, n)
		cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&b[0]))
		err := windows.QueryServiceConfig(svc, cfg, n, &n)
		if err == nil {
			return cfg, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return nil, fmt.Errorf("querying service config: %w", err)
		}
		// n now holds the required size; loop and retry with it. Bounded rather
		// than `for {}` so a driver that keeps asking for more cannot hang a
		// validate run.
	}
	return nil, fmt.Errorf("querying service config: buffer still too small after 3 attempts (last request %d bytes)", n)
}

func (s *ServiceWindows) queryStatus(svc windows.Handle) (*windows.SERVICE_STATUS_PROCESS, error) {
	var st windows.SERVICE_STATUS_PROCESS
	var needed uint32
	err := windows.QueryServiceStatusEx(svc, windows.SC_STATUS_PROCESS_INFO,
		(*byte)(unsafe.Pointer(&st)), uint32(unsafe.Sizeof(st)), &needed)
	if err != nil {
		return nil, fmt.Errorf("querying service status: %w", err)
	}
	return &st, nil
}

// withConfig runs fn against the service's configuration, handling the open,
// the absence case and the close in one place so each accessor below is three
// lines rather than fifteen.
func (s *ServiceWindows) withConfig(fn func(*windows.QUERY_SERVICE_CONFIG) error) error {
	mgr, svc, err := s.handles()
	if err != nil {
		if notFound(err) {
			return ErrServiceNotFound
		}
		return err
	}
	defer closeHandles(mgr, svc)
	cfg, err := s.queryConfig(svc)
	if err != nil {
		return err
	}
	return fn(cfg)
}

// Exists distinguishes three outcomes, not two: the service is there, it is
// genuinely absent, or the lookup failed. Only the middle one is (false, nil).
// An access-denied or an SCM that will not open is an ERROR -- reporting it as
// "does not exist" is the shape FEAT-010 spent its time removing.
func (s *ServiceWindows) Exists() (bool, error) {
	mgr, svc, err := s.handles()
	if err != nil {
		if notFound(err) {
			return false, nil
		}
		return false, err
	}
	closeHandles(mgr, svc)
	return true, nil
}

// startTypeName maps the SCM's start type to the spelling a gossfile uses.
//
// Lowercase, matching how syver spells other enumerated values (`filetype: file`,
// `view: native`) rather than the .NET enum's capitalisation. "manual" is
// SERVICE_DEMAND_START, which is what the Services UI calls Manual; the API name
// is not a word anyone uses.
func startTypeName(startType uint32) (string, error) {
	switch startType {
	case windows.SERVICE_BOOT_START:
		return "boot", nil
	case windows.SERVICE_SYSTEM_START:
		return "system", nil
	case windows.SERVICE_AUTO_START:
		return "automatic", nil
	case windows.SERVICE_DEMAND_START:
		return "manual", nil
	case windows.SERVICE_DISABLED:
		return "disabled", nil
	default:
		// Reported, never guessed. A value Windows adds later must not be
		// rendered as one of the five above.
		return "", fmt.Errorf("unrecognised service start type %d", startType)
	}
}

// startsAtBoot is what `enabled:` means: does this service start without anyone
// asking. FEAT-014 FIXED A DEFECT HERE. The old implementation substring-matched
// "Automatic", so a BOOT_START or SYSTEM_START service -- every kernel driver and
// much of the early boot chain -- reported `enabled: false`, which is wrong and
// passed silently for anyone asserting it.
func startsAtBoot(startType uint32) bool {
	switch startType {
	case windows.SERVICE_BOOT_START, windows.SERVICE_SYSTEM_START, windows.SERVICE_AUTO_START:
		return true
	default:
		return false
	}
}

func (s *ServiceWindows) Enabled() (bool, error) {
	var enabled bool
	err := s.withConfig(func(cfg *windows.QUERY_SERVICE_CONFIG) error {
		// Validate the value before trusting it: an unrecognised start type must
		// error rather than fall through to "not enabled".
		if _, err := startTypeName(cfg.StartType); err != nil {
			return err
		}
		enabled = startsAtBoot(cfg.StartType)
		return nil
	})
	return enabled, err
}

func (s *ServiceWindows) Running() (bool, error) {
	mgr, svc, err := s.handles()
	if err != nil {
		if notFound(err) {
			return false, ErrServiceNotFound
		}
		return false, err
	}
	defer closeHandles(mgr, svc)
	st, err := s.queryStatus(svc)
	if err != nil {
		return false, err
	}
	return st.CurrentState == windows.SERVICE_RUNNING, nil
}

// RunLevels has no Windows equivalent -- there is no SysV/systemd-style runlevel
// concept here. A spec ported from Linux that sets `runlevels:` gets a sentinel
// rather than a ContainElements failure against a nil slice that never mentioned
// Windows. See FEAT-010 S-5. FEAT-014 deliberately did not change this.
func (s *ServiceWindows) RunLevels() ([]string, error) {
	return nil, ErrServiceRunLevelsUnsupported
}

func (s *ServiceWindows) StartType() (string, error) {
	var name string
	err := s.withConfig(func(cfg *windows.QUERY_SERVICE_CONFIG) error {
		var err error
		name, err = startTypeName(cfg.StartType)
		return err
	})
	return name, err
}

// DelayedStart is a separate configuration flag, not a sixth start type: the
// Services UI's "Automatic (Delayed Start)" is `start-type: automatic` plus this.
// Windows reports the flag for any service, so it is only meaningful alongside an
// automatic start type -- the docs say so rather than this code second-guessing
// what the caller asked.
func (s *ServiceWindows) DelayedStart() (bool, error) {
	mgr, svc, err := s.handles()
	if err != nil {
		if notFound(err) {
			return false, ErrServiceNotFound
		}
		return false, err
	}
	defer closeHandles(mgr, svc)

	var info windows.SERVICE_DELAYED_AUTO_START_INFO
	var needed uint32
	err = windows.QueryServiceConfig2(svc, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO,
		(*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &needed)
	if err != nil {
		return false, fmt.Errorf("querying delayed auto-start: %w", err)
	}
	return info.IsDelayedAutoStartUp != 0, nil
}

// RunAs is the account the service runs under, as the SCM stores it:
// `LocalSystem`, `NT AUTHORITY\NetworkService`, `.\svc-account`, and so on. It is
// NOT normalised, because the stored form is what a hardening control is written
// against and rewriting it would hide what the machine actually says.
func (s *ServiceWindows) RunAs() (string, error) {
	var account string
	err := s.withConfig(func(cfg *windows.QUERY_SERVICE_CONFIG) error {
		account = windows.UTF16PtrToString(cfg.ServiceStartName)
		return nil
	})
	return account, err
}

func (s *ServiceWindows) DisplayName() (string, error) {
	var name string
	err := s.withConfig(func(cfg *windows.QUERY_SERVICE_CONFIG) error {
		name = windows.UTF16PtrToString(cfg.DisplayName)
		return nil
	})
	return name, err
}

// Dependencies lists what must start first. The SCM stores it as a
// double-null-terminated sequence of UTF-16 strings, which is why this needs
// utf16DoubleNullToStrings rather than UTF16PtrToString.
//
// A name beginning with '+' is a load-order GROUP rather than a service, which is
// how Windows itself spells it. Kept verbatim: dropping the marker would make a
// group indistinguishable from a service of the same name, and a control that
// asserts a group dependency needs to be able to say so.
func (s *ServiceWindows) Dependencies() ([]string, error) {
	var deps []string
	err := s.withConfig(func(cfg *windows.QUERY_SERVICE_CONFIG) error {
		deps = utf16DoubleNullToStrings(cfg.Dependencies)
		return nil
	})
	return deps, err
}

// Pid is the process the service is running in, and is 0 when it is not running.
// Meaningful only while running, which is why `syver add service` does not emit
// it: a generated spec pinning a pid would fail at the next restart.
func (s *ServiceWindows) Pid() (int, error) {
	mgr, svc, err := s.handles()
	if err != nil {
		if notFound(err) {
			return 0, ErrServiceNotFound
		}
		return 0, err
	}
	defer closeHandles(mgr, svc)
	st, err := s.queryStatus(svc)
	if err != nil {
		return 0, err
	}
	return int(st.ProcessId), nil
}

// utf16DoubleNullToStrings walks a MULTI_SZ: consecutive null-terminated UTF-16
// strings ending with an extra null. Bounded by a generous element cap so a
// malformed buffer cannot spin forever.
func utf16DoubleNullToStrings(p *uint16) []string {
	if p == nil {
		return nil
	}
	var out []string
	for i := 0; i < 4096; i++ {
		s := windows.UTF16PtrToString(p)
		if s == "" {
			return out
		}
		out = append(out, s)
		// Advance past this string and its terminator.
		p = (*uint16)(unsafe.Add(unsafe.Pointer(p), len(windows.StringToUTF16(s))*2))
	}
	return out
}
