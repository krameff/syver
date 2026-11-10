package syver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServeCacheMissLogRespectsLogLevel drives a real cache miss through the
// real level filter. The serve cache tests redirect the logger straight to a
// buffer with log.SetOutput, which bypasses the filter entirely, so they could
// not see that the cache-miss line printed at every -L level.
//
// REVERT-PROOF: with the [DEBUG] prefix removed from cacheMissLogFormat the
// INFO and WARN cases fail, because logutils passes a line with no recognised
// level whatever MinLevel is.
//
// Not t.Parallel(): it retargets the process-global logger, for the same
// reason given in TestServeWithNoContentNegotiation.
func TestServeCacheMissLogRespectsLogLevel(t *testing.T) {
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})

	tests := []struct {
		level string
		want  bool
	}{
		{level: "INFO", want: false},
		{level: "WARN", want: false},
		{level: "DEBUG", want: true},
		{level: "TRACE", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			var logOutput bytes.Buffer
			config, err := util.NewConfig(
				util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
				util.WithCache(time.Minute),
			)
			require.NoError(t, err)
			config.LogLevel = tc.level
			require.NoError(t, setLogLevelTo(config, &logOutput))

			hh, err := newHealthHandler(t.Context(), config)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			http.HandlerFunc(hh.ServeHTTP).ServeHTTP(rr, makeRequest(t, config, nil))
			require.Equal(t, http.StatusOK, rr.Code)

			got := strings.Contains(logOutput.String(), "Stale cache")
			assert.Equal(t, tc.want, got, "log output at -L %s:\n%s", tc.level, logOutput.String())
		})
	}
}
