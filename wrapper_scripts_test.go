package syver

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRuntime stands in for docker, nerdctl or podman. It records every
// invocation, one per line, and answers just enough for the wrapper to walk
// its happy path. FAKE_ROOTLESS=1 makes `info` report a rootless daemon the
// way both docker and nerdctl do, through SecurityOptions.
const fakeRuntime = `#!/bin/bash
echo "$*" >> "$FAKE_RUNTIME_LOG"
case "$1" in
  info)
    if [ "$FAKE_ROOTLESS" = 1 ]; then
      echo '["name=seccomp,profile=builtin","name=rootless"]'
    else
      echo '["name=seccomp,profile=builtin"]'
    fi ;;
  run|create) echo fakecontainerid0 ;;
  inspect) echo true ;;
esac
exit 0
`

// TestWrapperContainerRuntimes drives the dsyver and dgoss wrappers against a
// fake runtime on PATH, so the runtime allowlist and the rootless nerdctl guard
// are exercised without a container engine.
//
// REVERT-PROOF: dropping nerdctl from the allowlist fails the nerdctl cases;
// removing the rootless guard fails "nerdctl rootless cp", because the wrapper
// then goes on to `create`.
func TestWrapperContainerRuntimes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrappers are bash scripts driving a local container runtime")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	tests := []struct {
		name      string
		runtime   string
		strategy  string
		rootless  bool
		wantOK    bool
		wantErr   string
		wantCalls []string
		denyCalls []string
	}{
		{
			name: "unknown runtime", runtime: "bogus", strategy: "mount",
			wantErr: "Runtime must be one of docker, nerdctl or podman",
		},
		{
			name: "nerdctl mount", runtime: "nerdctl", strategy: "mount", wantOK: true,
			wantCalls: []string{"run -d -v", "exec fakecontainerid0"},
		},
		{
			name: "nerdctl rootful cp", runtime: "nerdctl", strategy: "cp", wantOK: true,
			wantCalls: []string{"create", "cp ", "start fakecontainerid0"},
		},
		{
			name: "nerdctl rootless cp", runtime: "nerdctl", strategy: "cp", rootless: true,
			wantErr:   "not supported with rootless nerdctl",
			denyCalls: []string{"create", "cp "},
		},
		{
			// Rootless docker copies into a created container without trouble,
			// so the guard must not reach beyond nerdctl.
			name: "docker rootless cp", runtime: "docker", strategy: "cp", rootless: true, wantOK: true,
			wantCalls: []string{"create", "cp ", "start fakecontainerid0"},
			denyCalls: []string{"info"},
		},
		{
			name: "podman mount", runtime: "podman", strategy: "mount", wantOK: true,
			wantCalls: []string{"run -d -v", "exec fakecontainerid0"},
		},
	}

	for _, script := range []string{"dsyver", "dgoss"} {
		scriptPath, err := filepath.Abs(filepath.Join("extras", "dsyver", script))
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range tests {
			t.Run(script+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				binDir := filepath.Join(dir, "bin")
				specDir := filepath.Join(dir, "spec")
				for _, d := range []string{binDir, specDir} {
					if err := os.Mkdir(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if tc.runtime != "bogus" {
					if err := os.WriteFile(filepath.Join(binDir, tc.runtime), []byte(fakeRuntime), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(specDir, "goss.yaml"), []byte("file: {}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				// Stands in for the syver binary: the wrapper only copies it.
				fakeSyver := filepath.Join(dir, "syver")
				if err := os.WriteFile(fakeSyver, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				callLog := filepath.Join(dir, "calls.log")
				rootless := "0"
				if tc.rootless {
					rootless = "1"
				}

				cmd := exec.Command(bash, scriptPath, "run", "example/image")
				cmd.Env = append(os.Environ(),
					"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
					"CONTAINER_RUNTIME="+tc.runtime,
					"GOSS_FILES_STRATEGY="+tc.strategy,
					"GOSS_FILES_PATH="+specDir,
					"GOSS_PATH="+fakeSyver,
					"GOSS_SLEEP=0",
					"DGOSS_TEMP_DIR="+dir,
					"FAKE_RUNTIME_LOG="+callLog,
					"FAKE_ROOTLESS="+rootless,
				)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				runErr := cmd.Run()

				callBytes, _ := os.ReadFile(callLog)
				calls := string(callBytes)
				detail := "stderr:\n" + stderr.String() + "\nruntime calls:\n" + calls

				if tc.wantOK && runErr != nil {
					t.Fatalf("wrapper failed: %v\n%s", runErr, detail)
				}
				if !tc.wantOK {
					if runErr == nil {
						t.Fatalf("wrapper succeeded, want failure\n%s", detail)
					}
					if !strings.Contains(stderr.String(), tc.wantErr) {
						t.Fatalf("stderr does not contain %q\n%s", tc.wantErr, detail)
					}
				}
				for _, c := range tc.wantCalls {
					if !containsLinePrefix(calls, c) {
						t.Errorf("runtime was never called with %q\n%s", c, detail)
					}
				}
				for _, c := range tc.denyCalls {
					if containsLinePrefix(calls, c) {
						t.Errorf("runtime was called with %q and should not have been\n%s", c, detail)
					}
				}
			})
		}
	}
}

func containsLinePrefix(s, prefix string) bool {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
