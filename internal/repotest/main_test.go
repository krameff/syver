// Package repotest tests the repository's shell scripts (install.sh,
// ci/version.sh and the extras/ wrappers) rather than any Go package.
package repotest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain runs every test from the repository root, so the scripts are
// addressed by the same relative paths a user or CI would use.
func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// repoRoot walks up from the working directory to the directory holding
// go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repotest: no go.mod above %s", dir)
		}
		dir = parent
	}
}
