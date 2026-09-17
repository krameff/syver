package syver

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVersionScriptFindsReleasesTaggedOnMain builds a throwaway repository with
// this project's release model -- work on devel, a release merged into main as a
// merge commit, the tag on that merge commit -- and checks ci/version.sh stamps
// devel builds from the release they contain.
//
// REVERT-PROOF: with the second-parent check removed, every devel case falls
// back to plain `git describe`, which cannot reach the tag, and fails.
func TestVersionScriptFindsReleasesTaggedOnMain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ci/version.sh is a bash script for local builds")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	script, err := filepath.Abs(filepath.Join("ci", "version.sh"))
	if err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		// Isolate from the developer's config: no signing prompts, no hooks.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(msg string) string {
		t.Helper()
		git("commit", "-q", "--allow-empty", "-m", msg)
		return git("rev-parse", "--short", "HEAD")
	}
	stamp := func() string {
		t.Helper()
		cmd := exec.Command(bash, script)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ci/version.sh: %v\n%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}

	git("init", "-q", "-b", "main")
	commit("root")
	git("tag", "v1.0.0")
	git("switch", "-q", "-c", "devel")
	commit("feature one")

	// Release 1.1.0: devel merged into main with a merge commit, tagged there.
	git("switch", "-q", "main")
	git("merge", "-q", "--no-ff", "-m", "release 1.1.0", "devel")
	git("tag", "v1.1.0")
	// A pre-release tag sorting above it must never be chosen.
	git("tag", "v1.1.0_alpha")
	git("switch", "-q", "devel")

	if got := stamp(); got != "v1.1.0" {
		t.Errorf("devel at the released commit: got %q, want %q", got, "v1.1.0")
	}

	sha := commit("after the release")
	if want := "v1.1.0-1-g" + sha; stamp() != want {
		t.Errorf("devel one commit past the release: got %q, want %q", stamp(), want)
	}

	git("switch", "-q", "main")
	if got := stamp(); got != "v1.1.0" {
		t.Errorf("main at the tag: got %q, want %q", got, "v1.1.0")
	}
}
