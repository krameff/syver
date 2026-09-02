//go:build windows
// +build windows

package system

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// TestClassifyRegistryError is FEAT-010 Task 3's unit-level proof of SW-3's
// fix: not-found, exists, and access-denied must produce three genuinely
// different answers, not the same (false, nil) for two of them.
//
// classifyRegistryError is exercised directly (rather than through Exists,
// which would need a real registry key this process cannot read) because
// arranging a genuinely access-denied key reliably across every CI runner is
// not guaranteed -- per FEAT-010 §8 Task 3, injecting the error at the seam
// is the documented fallback when that arrangement cannot be made reliable.
func TestClassifyRegistryError(t *testing.T) {
	t.Run("nil error means the key/value exists", func(t *testing.T) {
		exists, err := classifyRegistryError(nil, "opening registry key")
		if !exists || err != nil {
			t.Errorf("classifyRegistryError(nil, ...) = (%v, %v), want (true, nil)", exists, err)
		}
	})

	t.Run("registry.ErrNotExist means the key/value does not exist, and that is not an error", func(t *testing.T) {
		exists, err := classifyRegistryError(registry.ErrNotExist, "opening registry key")
		if exists || err != nil {
			t.Errorf("classifyRegistryError(ErrNotExist, ...) = (%v, %v), want (false, nil)", exists, err)
		}
	})

	t.Run("ERROR_ACCESS_DENIED is an error, not a false 'does not exist'", func(t *testing.T) {
		exists, err := classifyRegistryError(windows.ERROR_ACCESS_DENIED, "opening registry key")
		if exists {
			t.Errorf("classifyRegistryError(ERROR_ACCESS_DENIED, ...) reported exists=true")
		}
		if err == nil {
			t.Fatal("classifyRegistryError(ERROR_ACCESS_DENIED, ...) returned a nil error -- " +
				"this is exactly SW-3: an unreadable key must not be reported as absent")
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Errorf("returned error %v does not wrap ERROR_ACCESS_DENIED", err)
		}
	})
}

// TestDefRegistryWindowsExists_LiveKeys is an end-to-end check against real
// registry keys that are universally present/absent on any Windows host, so
// it needs no elevated privilege and no fixture setup.
func TestDefRegistryWindowsExists_LiveKeys(t *testing.T) {
	t.Run("a key that genuinely exists", func(t *testing.T) {
		r := &defRegistryWindows{key: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\`}
		exists, err := r.Exists()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Error("expected HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion to exist")
		}
	})

	t.Run("a key that genuinely does not exist", func(t *testing.T) {
		r := &defRegistryWindows{key: `HKLM\SOFTWARE\FEAT-010-syver-does-not-exist-12345\`}
		exists, err := r.Exists()
		if err != nil {
			t.Fatalf("unexpected error for a genuinely absent key: %v", err)
		}
		if exists {
			t.Error("expected a made-up key to report as not existing")
		}
	})
}
