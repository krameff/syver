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

// go-cache treats a zero or negative expiration as "never expire", so
// `--cache 0s` used to serve the first result for the life of the process.
// A non-positive duration must run the checks on every request instead.
func TestServeNonPositiveCacheRunsEveryRequest(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		t.Run(d.String(), func(t *testing.T) {
			var logOutput bytes.Buffer
			log.SetOutput(&logOutput)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			config, err := util.NewConfig(
				util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
				util.WithCache(d),
			)
			require.NoError(t, err)
			hh, err := newHealthHandler(t.Context(), config)
			require.NoError(t, err)

			for i := range 3 {
				rr := httptest.NewRecorder()
				http.HandlerFunc(hh.ServeHTTP).ServeHTTP(rr, makeRequest(t, config, nil))
				assert.Equal(t, http.StatusOK, rr.Code, "request %d", i)
			}

			assert.Equal(t, 3, strings.Count(logOutput.String(), cacheDisabledLog),
				"every request should have run the checks")
			assert.NotContains(t, logOutput.String(), "Stale cache",
				"there is no cache to be stale")
			assert.Equal(t, 0, hh.cache.ItemCount(), "nothing should be stored in the cache")
		})
	}
}
