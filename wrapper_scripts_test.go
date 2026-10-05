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

// TestWrapperSyverTempDir proves SYVER_TEMP_DIR steers where each wrapper builds
// its working directory. That is otherwise invisible: the directory is removed
// on exit, and the only place it surfaces is the bind mount handed to the
// container runtime.
//
// It needs a test of its own because DGOSS_TEMP_DIR is the one variable the
// pairing loop cannot fold. Its legacy name carries the SCRIPT prefix rather
// than the product one, and GOSS_TEMP_DIR has never existed, so the loop would
// look for a name nothing sets. Both wrappers handle it separately and both are
// covered here: dgoss already folds the other nine SYVER_* names, so omitting
// this one would have been inconsistent inside dgoss itself, not merely against
// dsyver.
//
// REVERT-PROOF: delete the SYVER_TEMP_DIR block from either script and that
// script's "syver only" and "syver wins over dgoss" cases fail, because the
// wrapper falls back to /tmp and the mount no longer sits under the test's
// directory. "empty syver" keeps passing, which is the point of having it.
func TestWrapperSyverTempDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrappers are bash scripts driving a local container runtime")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	tests := []struct {
		name string
		// syver is the subdirectory SYVER_TEMP_DIR points at, or "" for unset.
		syver string
		// exportEmptySyver exports SYVER_TEMP_DIR with an empty value, which
		// must be treated as unset rather than shadowing DGOSS_TEMP_DIR.
		exportEmptySyver bool
		// dgoss is the subdirectory DGOSS_TEMP_DIR points at, or "" for unset.
		dgoss string
		// want is the subdirectory the bind mount must sit under.
		want string
	}{
		{name: "syver only", syver: "a", want: "a"},
		{name: "syver wins over dgoss", syver: "a", dgoss: "b", want: "a"},
		{name: "empty syver cannot shadow dgoss", exportEmptySyver: true, dgoss: "b", want: "b"},
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
				for _, d := range []string{binDir, specDir, filepath.Join(dir, "a"), filepath.Join(dir, "b")} {
					if err := os.Mkdir(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(binDir, "podman"), []byte(fakeRuntime), 0o755); err != nil {
					t.Fatal(err)
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

				cmd := exec.Command(bash, scriptPath, "run", "example/image")
				cmd.Env = append(os.Environ(),
					"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
					"CONTAINER_RUNTIME=podman",
					"GOSS_FILES_STRATEGY=mount",
					"GOSS_FILES_PATH="+specDir,
					"GOSS_PATH="+fakeSyver,
					"GOSS_SLEEP=0",
					"FAKE_RUNTIME_LOG="+callLog,
					"FAKE_ROOTLESS=0",
				)
				// Each variable is set only when the case asks for it, so
				// "unset" is genuinely unset rather than an empty string.
				if tc.syver != "" {
					cmd.Env = append(cmd.Env, "SYVER_TEMP_DIR="+filepath.Join(dir, tc.syver))
				}
				if tc.exportEmptySyver {
					cmd.Env = append(cmd.Env, "SYVER_TEMP_DIR=")
				}
				if tc.dgoss != "" {
					cmd.Env = append(cmd.Env, "DGOSS_TEMP_DIR="+filepath.Join(dir, tc.dgoss))
				}

				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("wrapper failed: %v\nstderr:\n%s", err, stderr.String())
				}

				callBytes, _ := os.ReadFile(callLog)
				calls := string(callBytes)
				mount := ""
				for line := range strings.SplitSeq(calls, "\n") {
					if strings.HasPrefix(line, "run -d -v ") {
						mount = line
						break
					}
				}
				if mount == "" {
					t.Fatalf("the runtime was never asked to mount anything\ncalls:\n%s", calls)
				}
				wantPrefix := filepath.Join(dir, tc.want) + string(os.PathSeparator) + "tmp."
				if !strings.Contains(mount, wantPrefix) {
					t.Errorf("mount does not sit under %s\n  mount: %s\n  calls:\n%s", wantPrefix, mount, calls)
				}
			})
		}
	}
}

// stagingRuntime stands in for docker, podman or kubectl and snapshots whatever
// the wrapper hands the container: the source of a `-v SRC:...` bind mount, or
// the local source of a `cp SRC/. ...`. The staging directory itself is removed
// when the wrapper exits, so this copy is the only way to see what it staged.
const stagingRuntime = `#!/bin/bash
echo "$*" >> "$FAKE_RUNTIME_LOG"
prev=
for a in "$@"; do
  if [ "$prev" = "-v" ]; then cp -r "${a%%:*}/." "$FAKE_STAGE/"; fi
  prev=$a
done
if [ "$1" = cp ] && [ "${2%/.}" != "$2" ]; then cp -r "$2" "$FAKE_STAGE/"; fi
case "$1" in
  run|create) echo fakecontainerid0 ;;
  inspect) echo true ;;
esac
exit 0
`

// TestWrapperSpecDiscovery proves every wrapper, the syver-named ones and the
// goss-named shims alike, finds a spec under the same names the binary does:
// syver.yaml, syver.yml, goss.yaml, goss.yml, in that order, and the same for
// the wait file. `syver add` writes syver.yaml by default, so a wrapper that
// only looks for goss.yaml stages no spec at all for a project made with the
// current CLI, and the run then fails inside the container.
//
// REVERT-PROOF: restoring the goss.yaml-only copy in any script fails that
// script's "syver.yaml only", "syver.yaml wins" and "syver_wait.yaml" cases.
func TestWrapperSpecDiscovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrappers are bash scripts driving a local container runtime")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	type wrapper struct {
		dir, script, runtime string
		args                 []string
		// fileKeepsName is true where an explicit GOSS_FILE is staged under its
		// own basename rather than as goss.yaml.
		fileKeepsName bool
		// noGossFile marks the Kubernetes pair, which has never read GOSS_FILE.
		noGossFile bool
	}
	wrappers := []wrapper{
		{dir: "dsyver", script: "dsyver", runtime: "podman", args: []string{"run", "example/image"}},
		{dir: "dsyver", script: "dgoss", runtime: "podman", args: []string{"run", "example/image"}},
		{dir: "dcsyver", script: "dcsyver", runtime: "docker", args: []string{"run", "svc"}, fileKeepsName: true},
		{dir: "dcsyver", script: "dcgoss", runtime: "docker", args: []string{"run", "svc"}, fileKeepsName: true},
		{dir: "ksyver", script: "ksyver", runtime: "kubectl", args: []string{"run", "-i", "example/image"}, noGossFile: true},
		{dir: "ksyver", script: "kgoss", runtime: "kubectl", args: []string{"run", "-i", "example/image"}, noGossFile: true},
	}

	tests := []struct {
		name     string
		files    []string // spec files to create, each containing its own name
		gossFile string   // GOSS_FILE, or "" for unset
		want     string   // which file's content must be staged as the spec
		wantWait string   // which file's content must be staged as the wait file
	}{
		{name: "syver.yaml only", files: []string{"syver.yaml"}, want: "syver.yaml"},
		{name: "syver.yml only", files: []string{"syver.yml"}, want: "syver.yml"},
		{name: "goss.yml only", files: []string{"goss.yml"}, want: "goss.yml"},
		{name: "syver.yaml wins over goss.yaml", files: []string{"goss.yaml", "syver.yaml"}, want: "syver.yaml"},
		{name: "explicit GOSS_FILE wins", files: []string{"syver.yaml", "custom.yaml"}, gossFile: "custom.yaml", want: "custom.yaml"},
		{
			name: "syver_wait.yaml is staged and waited on", files: []string{"goss.yaml", "syver_wait.yaml"},
			want: "goss.yaml", wantWait: "syver_wait.yaml",
		},
	}

	for _, w := range wrappers {
		scriptPath, err := filepath.Abs(filepath.Join("extras", w.dir, w.script))
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range tests {
			t.Run(w.script+"/"+tc.name, func(t *testing.T) {
				if tc.gossFile != "" && w.noGossFile {
					t.Skip("this wrapper does not read GOSS_FILE")
				}
				dir := t.TempDir()
				binDir := filepath.Join(dir, "bin")
				specDir := filepath.Join(dir, "spec")
				stage := filepath.Join(dir, "stage")
				for _, d := range []string{binDir, specDir, stage} {
					if err := os.Mkdir(d, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				fake := filepath.Join(binDir, w.runtime)
				if err := os.WriteFile(fake, []byte(stagingRuntime), 0o755); err != nil {
					t.Fatal(err)
				}
				for _, f := range tc.files {
					if err := os.WriteFile(filepath.Join(specDir, f), []byte(f+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				// The compose wrappers refuse to start without a compose file.
				if err := os.WriteFile(filepath.Join(specDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				fakeSyver := filepath.Join(dir, "syver")
				if err := os.WriteFile(fakeSyver, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				callLog := filepath.Join(dir, "calls.log")

				cmd := exec.Command(bash, append([]string{scriptPath}, w.args...)...)
				cmd.Dir = specDir
				cmd.Env = append(os.Environ(),
					"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
					"CONTAINER_RUNTIME="+w.runtime,
					"COMPOSE_BIN=docker compose",
					"GOSS_KUBECTL_BIN="+fake,
					"GOSS_FILES_PATH="+specDir,
					"GOSS_PATH="+fakeSyver,
					"GOSS_SLEEP=0",
					"DGOSS_TEMP_DIR="+dir,
					"FAKE_RUNTIME_LOG="+callLog,
					"FAKE_STAGE="+stage,
				)
				if tc.gossFile != "" {
					cmd.Env = append(cmd.Env, "GOSS_FILE="+tc.gossFile)
				}
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				runErr := cmd.Run()
				callBytes, _ := os.ReadFile(callLog)
				detail := "stderr:\n" + stderr.String() + "\nruntime calls:\n" + string(callBytes)
				if runErr != nil {
					t.Fatalf("wrapper failed: %v\n%s", runErr, detail)
				}

				staged := "goss.yaml"
				if tc.gossFile != "" && w.fileKeepsName {
					staged = tc.gossFile
				}
				got, err := os.ReadFile(filepath.Join(stage, staged))
				if err != nil {
					t.Fatalf("no spec staged as %s: %v\n%s", staged, err, detail)
				}
				if strings.TrimSpace(string(got)) != tc.want {
					t.Errorf("staged spec is %q, want the content of %s\n%s", got, tc.want, detail)
				}

				waited := strings.Contains(string(callBytes), "goss_wait.yaml")
				if tc.wantWait == "" {
					if waited {
						t.Errorf("waited on a wait file that does not exist\n%s", detail)
					}
					return
				}
				got, err = os.ReadFile(filepath.Join(stage, "goss_wait.yaml"))
				if err != nil {
					t.Fatalf("no wait file staged: %v\n%s", err, detail)
				}
				if strings.TrimSpace(string(got)) != tc.wantWait {
					t.Errorf("staged wait file is %q, want the content of %s\n%s", got, tc.wantWait, detail)
				}
				if !waited {
					t.Errorf("the wait file was staged but never run\n%s", detail)
				}
			})
		}
	}
}

// fakeSbx stands in for the sbx CLI. It records every invocation, answers the
// wrapper's `mktemp -d` with a fixed directory, snapshots whatever `sbx cp`
// copies in, and exits FAKE_VALIDATE_RC from the main validate run so a failing
// check can be simulated.
const fakeSbx = `#!/bin/bash
echo "$*" >> "$FAKE_RUNTIME_LOG"
case "$1" in
  exec)
    cmd="${@: -1}"
    case "$cmd" in
      "mktemp -d") echo /tmp/fake.remote ;;
      */goss.yaml\ *) exit "${FAKE_VALIDATE_RC:-0}" ;;
    esac ;;
  cp) cp -r "$2/." "$FAKE_STAGE/" ;;
esac
exit 0
`

// TestSbxsyver drives sbxsyver against a fake sbx on PATH: what it stages, how
// it builds the sbx exec calls, that the exit status is syver's, and that it
// cleans up inside the sandbox even when a check fails.
//
// REVERT-PROOF: dropping the `rm -rf` from cleanup() fails "failing check" and
// "passing run"; moving "${exec_flags[@]}" after the sandbox name fails "exec
// flags"; removing the uname guard fails "non-Linux host"; staging goss.yaml
// before syver.yaml fails "passing run".
func TestSbxsyver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sbxsyver is a bash script driving the sbx CLI")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	scriptPath, err := filepath.Abs(filepath.Join("extras", "sbxsyver", "sbxsyver"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		files      []string // spec files to create, each containing its own name
		vars       bool     // set SYVER_VARS=vars.yaml and create it
		uname      string   // "" leaves the real uname alone
		noPath     bool     // leave SYVER_PATH unset
		validateRC string
		wantRC     int
		wantErr    string
		wantSpec   string   // which file's content must be staged as goss.yaml
		wantCalls  []string // line prefixes that must appear in the sbx call log
	}{
		{
			name: "passing run", args: []string{"run", "box"},
			files: []string{"goss.yaml", "syver.yaml"}, vars: true, wantSpec: "syver.yaml",
			wantCalls: []string{
				"exec box sh -c mktemp -d",
				"cp ",
				"exec box sh -c /tmp/fake.remote/syver/syver -g /tmp/fake.remote/syver/goss.yaml --vars='/tmp/fake.remote/syver/vars.yaml' validate",
				"exec box rm -rf /tmp/fake.remote",
			},
		},
		{
			name: "exec flags", args: []string{"run", "box", "-u", "root"},
			files: []string{"syver.yaml"}, wantSpec: "syver.yaml",
			wantCalls: []string{"exec -u root box sh -c /tmp/fake.remote/syver/syver"},
		},
		{
			name: "failing check", args: []string{"run", "box"}, files: []string{"syver.yaml"},
			validateRC: "1", wantRC: 1,
			wantCalls: []string{"exec box rm -rf /tmp/fake.remote"},
		},
		{
			name: "no spec", args: []string{"run", "box"},
			wantRC: 1, wantErr: "no spec found",
		},
		{
			name: "no sandbox", args: []string{"run"}, files: []string{"syver.yaml"},
			wantRC: 1, wantErr: "USAGE",
		},
		{
			name: "non-Linux host", args: []string{"run", "box"}, files: []string{"syver.yaml"},
			uname: "Darwin", noPath: true, wantRC: 1, wantErr: "linux syver binary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binDir := filepath.Join(dir, "bin")
			specDir := filepath.Join(dir, "spec")
			stage := filepath.Join(dir, "stage")
			for _, d := range []string{binDir, specDir, stage} {
				if err := os.Mkdir(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(binDir, "sbx"), []byte(fakeSbx), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.uname != "" {
				if err := os.WriteFile(filepath.Join(binDir, "uname"), []byte("#!/bin/sh\necho "+tc.uname+"\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(specDir, f), []byte(f+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			fakeSyver := filepath.Join(dir, "syver")
			if err := os.WriteFile(fakeSyver, []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			callLog := filepath.Join(dir, "calls.log")

			cmd := exec.Command(bash, append([]string{scriptPath}, tc.args...)...)
			cmd.Env = append(os.Environ(),
				"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SYVER_FILES_PATH="+specDir,
				"SYVER_TEMP_DIR="+dir,
				"FAKE_RUNTIME_LOG="+callLog,
				"FAKE_STAGE="+stage,
				"FAKE_VALIDATE_RC="+tc.validateRC,
			)
			if !tc.noPath {
				cmd.Env = append(cmd.Env, "SYVER_PATH="+fakeSyver)
			}
			if tc.vars {
				if err := os.WriteFile(filepath.Join(specDir, "vars.yaml"), []byte("vars.yaml\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				cmd.Env = append(cmd.Env, "SYVER_VARS=vars.yaml")
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			runErr := cmd.Run()

			callBytes, _ := os.ReadFile(callLog)
			calls := string(callBytes)
			detail := "stderr:\n" + stderr.String() + "\nsbx calls:\n" + calls

			rc := 0
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				rc = exitErr.ExitCode()
			} else if runErr != nil {
				t.Fatalf("could not run the wrapper: %v", runErr)
			}
			if rc != tc.wantRC {
				t.Fatalf("exit status %d, want %d\n%s", rc, tc.wantRC, detail)
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Fatalf("stderr does not contain %q\n%s", tc.wantErr, detail)
			}
			for _, c := range tc.wantCalls {
				if !containsLinePrefix(calls, c) {
					t.Errorf("sbx was never called with %q\n%s", c, detail)
				}
			}
			if tc.wantSpec != "" {
				got, err := os.ReadFile(filepath.Join(stage, "goss.yaml"))
				if err != nil {
					t.Fatalf("no spec was staged: %v\n%s", err, detail)
				}
				if string(got) != tc.wantSpec+"\n" {
					t.Errorf("staged spec is %q, want the content of %s\n%s", got, tc.wantSpec, detail)
				}
			}
			if tc.vars {
				if _, err := os.Stat(filepath.Join(stage, "vars.yaml")); err != nil {
					t.Errorf("vars file was not staged\n%s", detail)
				}
			}
		})
	}
}
