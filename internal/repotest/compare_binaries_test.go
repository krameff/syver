package repotest

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCompareBinaries drives ci/compare-binaries.sh with two stub "binaries"
// that print fixed validate JSON, and checks it reports exactly the checks
// whose result changed.
//
// REVERT-PROOF: with the status column dropped from the jq projection, the
// pass -> fail case reports nothing and exits 0; with the #n numbering
// removed, the duplicate-key case cross-joins and reports changes that did
// not happen.
func TestCompareBinaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ci/compare-binaries.sh is a bash script")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
	script, err := filepath.Abs(filepath.Join("ci", "compare-binaries.sh"))
	if err != nil {
		t.Fatal(err)
	}

	type check struct{ typ, id, prop, status string }
	stub := func(name string, checks ...check) string {
		t.Helper()
		var rs []string
		for _, c := range checks {
			rs = append(rs, `{"resource-type":"`+c.typ+`","resource-id":"`+c.id+
				`","property":"`+c.prop+`","skipped":`+boolStr(c.status == "skip")+
				`,"successful":`+boolStr(c.status == "pass")+`}`)
		}
		return writeStub(t, name, `{"results":[`+strings.Join(rs, ",")+`]}`)
	}
	run := func(old, new string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bash, append([]string{script, old, new}, args...)...)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		switch {
		case err == nil:
			return string(out), 0
		case errors.As(err, &ee):
			return string(out), ee.ExitCode()
		default:
			t.Fatalf("running compare-binaries: %v", err)
			return "", -1
		}
	}

	base := []check{
		{"File", "/etc/hosts", "exists", "pass"},
		{"File", "/etc/hosts", "mode", "pass"},
		{"Command", "echo", "exit-status", "fail"},
		{"Service", "sshd", "running", "skip"},
	}

	t.Run("identical results exit 0", func(t *testing.T) {
		out, code := run(stub("old", base...), stub("new", base...))
		if code != 0 || !strings.Contains(out, "compared 4 checks: 0 changed") {
			t.Fatalf("exit %d, output:\n%s", code, out)
		}
	})

	t.Run("changed and one-sided checks are reported", func(t *testing.T) {
		changed := []check{
			{"File", "/etc/hosts", "exists", "pass"},
			{"File", "/etc/hosts", "mode", "fail"},
			{"Command", "echo", "exit-status", "fail"},
			{"Service", "sshd", "running", "pass"},
			{"Port", "tcp:22", "listening", "pass"},
		}
		out, code := run(stub("old", base...), stub("new", changed...))
		if code != 1 {
			t.Fatalf("exit %d, want 1; output:\n%s", code, out)
		}
		for _, want := range []string{
			"File|/etc/hosts|mode: pass -> fail",
			"Service|sshd|running: skip -> pass",
			"Port|tcp:22|listening: missing -> pass",
			"compared 5 checks: 3 changed",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "exists:") || strings.Contains(out, "exit-status:") {
			t.Errorf("unchanged checks reported:\n%s", out)
		}
	})

	t.Run("repeated keys pair one to one", func(t *testing.T) {
		dup := []check{
			{"Command", "x", "stdout", "pass"},
			{"Command", "x", "stdout", "fail"},
		}
		out, code := run(stub("old", dup...), stub("new", dup...))
		if code != 0 || !strings.Contains(out, "compared 2 checks: 0 changed") {
			t.Fatalf("exit %d, output:\n%s", code, out)
		}
	})

	t.Run("no JSON is a usage error, not a pass", func(t *testing.T) {
		bad := writeStub(t, "bad", "Error: could not read spec")
		out, code := run(stub("old", base...), bad)
		if code != 2 || !strings.Contains(out, "produced no results JSON") {
			t.Fatalf("exit %d, want 2; output:\n%s", code, out)
		}
	})

	t.Run("flags reach both binaries ahead of validate", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "args")
		rec := writeStubScript(t, "rec", `echo "$*" >> `+shellQuote(log)+`
echo '{"results":[]}'`)
		if _, code := run(rec, rec, "-g", "spec.yaml", "--vars", "v.yaml"); code != 0 {
			t.Fatalf("exit %d", code)
		}
		got, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		want := "-g spec.yaml --vars v.yaml validate --format json --no-color\n"
		if string(got) != want+want {
			t.Fatalf("args seen:\n%s\nwant twice:\n%s", got, want)
		}
	})
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// writeStub writes an executable that ignores its arguments and prints body.
func writeStub(t *testing.T, name, body string) string {
	t.Helper()
	return writeStubScript(t, name, "cat <<'EOF'\n"+body+"\nEOF")
}

func writeStubScript(t *testing.T, name, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
