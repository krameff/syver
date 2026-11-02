package syver

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const allFormatOptions = "exclude_raw, perfdata, pretty, sort, verbose"

// An unknown -o value must not stop validate: a typo in a unit's
// SYVER_FMT_OPTIONS used to be silently accepted, and refusing to run over it
// in a patch release would turn that into an outage. It draws a WARN naming
// the option and listing every valid one, and the run goes ahead.
func TestValidateWarnsOnUnknownFormatOptionAndRuns(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "goss.yaml")
	require.NoError(t, os.WriteFile(spec, []byte("file:\n  "+filepath.ToSlash(spec)+":\n    exists: true\n"), 0o600))

	cfg, err := util.NewConfig(util.WithSpecFile(spec), util.WithFormatOptions("bogus"), util.WithResultWriter(io.Discard))
	require.NoError(t, err)
	cfg.LogLevel = "WARN"

	var code int
	_, stderr := captureStdoutStderr(t, func() { code, err = Validate(t.Context(), cfg) })
	assert.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, `[WARN] unknown format option "bogus" has no effect; valid options: `+allFormatOptions)
}

// An option another format honours is a warning under validate and silent
// under serve, which passes warnIgnored=false because a request may negotiate
// a format the option does apply to. An unknown name warns under both.
func TestCheckFormatOptionsWarnings(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	cfg := func(format string, opts ...string) *util.Config {
		return &util.Config{OutputFormat: format, FormatOptions: opts}
	}

	checkFormatOptions(cfg("rspecish", "pretty", "sort"), true)
	assert.Contains(t, logOutput.String(), `[WARN] format option "pretty" has no effect on the rspecish format`)
	assert.NotContains(t, logOutput.String(), `"sort"`)

	logOutput.Reset()
	checkFormatOptions(cfg("rspecish", "pretty"), false)
	assert.Empty(t, logOutput.String())

	logOutput.Reset()
	checkFormatOptions(cfg("json", "bogus"), false)
	assert.Contains(t, logOutput.String(), `[WARN] unknown format option "bogus" has no effect; valid options: `+allFormatOptions)
}

// Padded values, as "pretty, sort" in GOSS_FMT_OPTIONS produces, are trimmed
// in the config the outputters read, so " sort" takes effect rather than
// being reported as unknown.
func TestCheckFormatOptionsTrimsPaddedValues(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c := &util.Config{OutputFormat: "json", FormatOptions: []string{"pretty", " sort", "  ", "exclude_raw "}}
	checkFormatOptions(c, true)
	assert.Equal(t, []string{"pretty", "sort", "exclude_raw"}, c.FormatOptions)
	assert.Empty(t, logOutput.String())
}

// serve builds its handler despite an unknown option, warning with the full
// list rather than only the default format's options, and stays silent about
// an option the default format ignores.
func TestServeChecksFormatOptions(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	newConfig := func(opts ...string) *util.Config {
		c, err := util.NewConfig(
			util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
			util.WithOutputFormat("rspecish"),
			util.WithFormatOptions(opts...),
		)
		require.NoError(t, err)
		return c
	}

	h, err := newHealthHandler(t.Context(), newConfig("bogus"))
	require.NoError(t, err, "an unknown option must not stop serve")
	assert.NotNil(t, h)
	assert.Contains(t, logOutput.String(), `[WARN] unknown format option "bogus" has no effect; valid options: `+allFormatOptions)

	logOutput.Reset()
	_, err = newHealthHandler(t.Context(), newConfig("pretty"))
	require.NoError(t, err)
	assert.Empty(t, logOutput.String())

	padded := newConfig(" pretty", "sort ")
	_, err = newHealthHandler(t.Context(), padded)
	require.NoError(t, err)
	assert.Equal(t, []string{"pretty", "sort"}, padded.FormatOptions)
}
