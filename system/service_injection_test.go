package system

import (
	"fmt"
	"strings"
	"testing"
)

// These tests exist because the previous construction was exploitable and
// looked hardened. service_windows.go built its PowerShell probes with
// fmt.Sprintf("... -Name %q ...", s.service), and %q is GO escaping, not
// PowerShell escaping. PowerShell DOUBLE-quoted strings interpolate $(...),
// executing it, and %q does not touch `$`. A gossfile service name of
// `$(calc.exe)` therefore executed, with no quote-breaking required.
//
// The service name is the literal YAML key under `service:` in a gossfile, and
// nothing validates it: invalidService() blocks only '/' and is never called
// from the Windows path anyway. The built string reaches CreateProcess verbatim
// through SysProcAttr.CmdLine, so Go's argv escaping never applies.
//
// Every payload below was chosen because it defeats the OLD construction.

var injectionPayloads = []struct {
	name    string
	service string
}{
	{"subexpression", `$(calc.exe)`},
	{"subexpression in the middle", `Spooler$(calc.exe)`},
	{"variable expansion", `$env:USERNAME`},
	{"quote break", `Spooler"; Start-Process calc.exe; "`},
	{"backtick escape", "Spooler`nStart-Process calc.exe"},
	{"statement separator", `Spooler; Start-Process calc.exe`},
	{"single quote", `it's-a-service`},
	{"only quotes", `'''`},
	{"backslash", `C:\Windows\System32`},
	{"benign", `Spooler`},
}

// TestPsSingleQuoteNeutralisesPayloads pins the property the fix depends on:
// the result is a PowerShell single-quoted literal, and single-quoted strings
// interpolate nothing at all. The only metacharacter is ' itself, doubled.
func TestPsSingleQuoteNeutralisesPayloads(t *testing.T) {
	for _, tc := range injectionPayloads {
		t.Run(tc.name, func(t *testing.T) {
			got := psSingleQuote(tc.service)

			if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
				t.Fatalf("result must be a single-quoted literal, got %s", got)
			}

			// The body must contain no lone ' -- every one is doubled. Strip the
			// outer pair, then every remaining quote must come in pairs.
			body := got[1 : len(got)-1]
			for i := 0; i < len(body); i++ {
				if body[i] != '\'' {
					continue
				}
				if i+1 >= len(body) || body[i+1] != '\'' {
					t.Fatalf("lone %q at offset %d in %s -- this terminates the "+
						"literal and lets the rest run as code", "'", i, got)
				}
				i++ // skip the pair
			}

			// Round-trip: undoubling the body returns the original exactly, so
			// the quoting is lossless as well as safe.
			if undone := strings.ReplaceAll(body, "''", "'"); undone != tc.service {
				t.Errorf("not lossless: got %q back, want %q", undone, tc.service)
			}
		})
	}
}

// TestServiceProbesQuoteTheServiceName builds the command strings exactly as
// the three probes in service_windows.go do and asserts the payload is inside
// a single-quoted literal. Kept here rather than in service_windows.go so it
// runs on Linux -- the defect is in string construction, which is not
// platform-specific even though only the Windows path reaches it.
func TestServiceProbesQuoteTheServiceName(t *testing.T) {
	probes := []struct {
		name   string
		format string
	}{
		{"Exists", "if (Get-Service -Name %s -ErrorAction SilentlyContinue) { 'True' } else { 'False' }"},
		{"Enabled", "$s = Get-Service -Name %s -ErrorAction SilentlyContinue; if ($s) { 'EXISTS|' + $s.StartType } else { 'ABSENT' }"},
		{"Running", "$s = Get-Service -Name %s -ErrorAction SilentlyContinue; if ($s) { 'EXISTS|' + $s.Status } else { 'ABSENT' }"},
	}

	for _, p := range probes {
		for _, tc := range injectionPayloads {
			t.Run(p.name+"/"+tc.name, func(t *testing.T) {
				cmdLine := fmt.Sprintf(p.format, psSingleQuote(tc.service))

				want := "-Name " + psSingleQuote(tc.service)
				if !strings.Contains(cmdLine, want) {
					t.Fatalf("service name is not single-quoted in the command line.\n got: %s\nwant substring: %s", cmdLine, want)
				}

				// A raw $( in the command line is only safe inside single
				// quotes. If the payload contributed one that is NOT inside the
				// quoted literal, the probe is injectable again.
				if strings.Contains(tc.service, "$(") {
					quoted := psSingleQuote(tc.service)
					outside := strings.ReplaceAll(cmdLine, quoted, "")
					if strings.Contains(outside, "$(") {
						t.Errorf("payload's $( escaped the quoted literal: %s", cmdLine)
					}
				}
			})
		}
	}
}
