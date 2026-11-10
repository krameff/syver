package syver

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A name that matches nothing must fail autoadd, not log a line and exit 0.
// Nothing is written, so a missing spec stays missing.
func TestAutoAddFailsWhenNothingMatches(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	spec := filepath.Join(t.TempDir(), "goss.yaml")
	skipAutoAddOffLinux(t)
	cfg, err := util.NewConfig()
	require.NoError(t, err)

	err = AutoAddResources(spec, []string{"syver-no-such-resource-xyz"}, cfg)
	assert.EqualError(t, err, `autoadd found no resource matching "syver-no-such-resource-xyz"; check the resource name(s)`)
	assert.NoFileExists(t, spec)
}

// When some names match and some do not, the matches are written and the
// call still fails, naming the names that matched nothing.
func TestAutoAddWritesMatchesAndFailsOnTheRest(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	dir := t.TempDir()
	spec := filepath.Join(dir, "goss.yaml")
	present := filepath.Join(dir, "present")
	require.NoError(t, os.WriteFile(present, []byte("x"), 0o600))
	skipAutoAddOffLinux(t)
	cfg, err := util.NewConfig()
	require.NoError(t, err)

	err = AutoAddResources(spec, []string{present, "syver-no-such-resource-xyz"}, cfg)
	assert.EqualError(t, err, `autoadd found no resource matching "syver-no-such-resource-xyz"; the other names were added to `+spec)
	written, readErr := os.ReadFile(spec)
	require.NoError(t, readErr)
	assert.Contains(t, string(written), present)
}

// WriteJSON refuses an empty config with an error rather than a log line, so
// every caller that reaches it with nothing to write fails.
func TestWriteJSONRejectsAnEmptyConfig(t *testing.T) {
	prev := outStoreFormat
	outStoreFormat = YAML
	t.Cleanup(func() { outStoreFormat = prev })
	spec := filepath.Join(t.TempDir(), "goss.yaml")

	err := WriteJSON(spec, *NewSyverConfig())
	assert.EqualError(t, err, "can't write empty configuration file "+spec+", please check resource name(s)")
	assert.NoFileExists(t, spec)
}

// Off Linux, autoadd's service and package lookups fail on their own before
// the no-match handling is reached: macOS has no `service` command, and on
// Windows the lookups and path-shaped keys return their own errors.
func skipAutoAddOffLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("autoadd lookups on %s return their own errors before the no-match handling is reached", runtime.GOOS)
	}
}
