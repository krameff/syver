package system

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file used to test psSingleQuote, the PowerShell escaper. FEAT-014 deleted
// that escaper along with the subprocess it protected: `service:` now reads the
// Service Control Manager through golang.org/x/sys/windows, passing the service
// name as a UTF-16 string argument. There is no command line, so there is nothing
// to quote and no injection surface.
//
// THE TESTS WERE NOT SIMPLY DELETED WITH IT. The vulnerability is worth
// remembering because the construction that carried it looked hardened:
// service_windows.go built its probes with
// fmt.Sprintf("... -Name %q ...", s.service), and %q is GO escaping, not
// PowerShell escaping. PowerShell double-quoted strings interpolate $(...) and
// execute it, and %q does not touch `$` -- so a gossfile service name of
// `$(calc.exe)` ran calc.exe with no quote-breaking required. The service name is
// the literal YAML key under `service:`, nothing validates it, and the built
// string reached CreateProcess verbatim through SysProcAttr.CmdLine, so Go's own
// argv escaping never applied.
//
// So the value-level tests are replaced by a SOURCE-level invariant, the same
// technique as windows_unsupported_guard_test.go and waitdelay_guard_test.go: it
// is checked from Linux, it needs no Windows host, and unlike the old tests it
// cannot be satisfied by a reimplementation that happens to quote correctly while
// reintroducing the surface.
//
// It also pins a FEAT-014 acceptance criterion directly: "No PowerShell
// subprocess remains in the `service:` path."
//
// If a PowerShell probe is ever genuinely needed again, this test failing is the
// intended outcome, not an obstacle: read the paragraph above, restore an escaper
// with single-quoted semantics (single quotes interpolate nothing; the only
// metacharacter is ' itself, doubled), and pin it with value-level tests as well
// as this one.
func TestNoPowershellSubprocessInTheServicePath(t *testing.T) {
	// Read the directory and parse file by file rather than with parser.ParseDir,
	// for the reason the other guards in this package record: per-GOOS resolution
	// would hide every *_windows.go file from a Linux run and defeat the check.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package directory: %v", err)
	}

	var scanned int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		scanned++

		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// util.NewCommandForWindowsPowershell / ...Context. The constructor is
			// still exported from util -- removing an exported symbol would be a
			// public API break in a minor release -- so the invariant is that
			// nothing in system/ CALLS it, not that it does not exist.
			if strings.HasPrefix(sel.Sel.Name, "NewCommandForWindowsPowershell") {
				t.Errorf("%s calls %s: the service path must not shell out to PowerShell. "+
					"Read this file's header before adding one back.",
					filepath.Base(name), sel.Sel.Name)
			}
			return true
		})
	}

	// A guard that scans nothing passes vacuously. This package has dozens of
	// non-test files; if the count collapses, the walk is broken, not the code.
	if scanned < 10 {
		t.Errorf("scanned only %d non-test files in system/, which suggests this guard "+
			"has lost its reach rather than the package having shrunk", scanned)
	}
}
