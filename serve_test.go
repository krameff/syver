package syver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWithNoContentNegotiation(t *testing.T) {
	// Not t.Parallel(): this test calls log.SetOutput, which retargets the
	// process-global logger at a buffer local to this function. Run in
	// parallel with any other test that does the same, each redirects the
	// global logger out from under the other and they race on the buffer --
	// `go test -race` reports it. Building a handler also drives the
	// package-level config globals (outStoreFormat, currentTemplateFilter,
	// quietDecode, color.NoColor), which assume no concurrent load.
	// With these two serial, `go test -race ./...` is clean.
	tests := map[string]struct {
		outputFormat        string
		specFile            string
		expectedHTTPStatus  int
		expectedContentType string
	}{
		"passing-json": {
			outputFormat:        "json",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		"failing-json": {
			outputFormat:        "json",
			specFile:            filepath.Join("testdata", "failing.goss.yaml"),
			expectedHTTPStatus:  http.StatusServiceUnavailable,
			expectedContentType: "application/json",
		},
		"failing-default-output": {
			outputFormat:        "rspecish",
			specFile:            filepath.Join("testdata", "failing.goss.yaml"),
			expectedHTTPStatus:  http.StatusServiceUnavailable,
			expectedContentType: "",
		},
	}
	for testName := range tests {
		tc := tests[testName]
		t.Run(testName, func(t *testing.T) {
			var logOutput bytes.Buffer
			log.SetOutput(&logOutput)

			config, err := util.NewConfig(
				util.WithSpecFile(tc.specFile),
				util.WithOutputFormat(tc.outputFormat),
			)
			require.NoError(t, err)

			hh, err := newHealthHandler(config)
			require.NoError(t, err)

			req := makeRequest(t, config, nil)
			rr := httptest.NewRecorder()

			handler := http.HandlerFunc(hh.ServeHTTP)

			handler.ServeHTTP(rr, req)

			t.Logf("testName %q log output:\n%s", testName, logOutput.String())
			assert.Equal(t, tc.expectedHTTPStatus, rr.Code)
			if tc.expectedContentType != "" {
				assert.Equal(t, tc.expectedContentType, rr.Result().Header.Get("Content-Type"))
			}
		})
	}
}

func TestServeNegotiatingContent(t *testing.T) {
	// Not t.Parallel(): this test calls log.SetOutput, which retargets the
	// process-global logger at a buffer local to this function. Run in
	// parallel with any other test that does the same, each redirects the
	// global logger out from under the other and they race on the buffer --
	// `go test -race` reports it. Building a handler also drives the
	// package-level config globals (outStoreFormat, currentTemplateFilter,
	// quietDecode, color.NoColor), which assume no concurrent load.
	// With these two serial, `go test -race ./...` is clean.
	tests := map[string]struct {
		acceptHeader        []string
		outputFormat        string
		specFile            string
		expectedHTTPStatus  int
		expectedContentType string
	}{
		// A blank/unrecognized Accept header expresses no preference, so
		// there is nothing to echo. The fallback deliberately keeps the
		// legacy goss- prefix: PLAN section 4 does not mark this row a hard
		// break, and a health probe that never sets Accept must not see its
		// Content-Type change. Clients that ask for vnd.syver- still get it
		// echoed back -- see the two echo cases further down.
		"accept {blank} returns process-level format-option": {
			acceptHeader: []string{
				"",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-structured",
		},
		"accept application/json": {
			acceptHeader: []string{
				"application/json",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		"accept text/json translates to application/json": {
			acceptHeader: []string{
				"text/json",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		"when accept is application/vnd.goss-json, return more widely known application/json": {
			acceptHeader: []string{
				"application/vnd.goss-json",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		"accept prometheus": {
			acceptHeader: []string{
				"text/plain; version=0.0.4",
			},
			outputFormat:        "prometheus",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "text/plain; version=0.0.4",
		},
		"accept header contains vendor-specific output format different from process-level": {
			acceptHeader: []string{
				"application/vnd.goss-rspecish",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-rspecish",
		},
		"accept header contains nonsense": {
			acceptHeader: []string{
				"application/vnd.goss-nonexistent",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-structured",
		},
		"accept header contains nonsense then valid": {
			acceptHeader: []string{
				"application/vnd.goss-nonexistent",
				"application/json",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		// §5.6: application/vnd.syver-json must now resolve (previously
		// silently fell back to the process-level format with HTTP 200
		// and no diagnostic).
		"when accept is application/vnd.syver-json, return more widely known application/json": {
			acceptHeader: []string{
				"application/vnd.syver-json",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/json",
		},
		"accept header contains vnd.syver- prefix, echoes vnd.syver-": {
			acceptHeader: []string{
				"application/vnd.syver-rspecish",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.syver-rspecish",
		},
		"accept header contains vnd.goss- prefix, echoes vnd.goss- not vnd.syver-": {
			acceptHeader: []string{
				"application/vnd.goss-rspecish",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-rspecish",
		},
		// No Accept header AT ALL is a distinct case from a blank one: the
		// negotiation loop never runs, so this asserts the initial value of
		// matchedPrefix directly. This is the common health-probe shape and
		// must keep emitting the legacy goss- prefix.
		"no accept header at all keeps the legacy goss- prefix": {
			acceptHeader:        nil,
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-structured",
		},
		// A later invalid candidate must not discard a format an earlier
		// candidate already resolved. Before the first-match break this
		// fell all the way back to the process-level format ("structured").
		"valid accept followed by nonsense still honours the valid one": {
			acceptHeader: []string{
				"application/vnd.goss-rspecish",
				"application/vnd.goss-nonexistent",
			},
			outputFormat:        "structured",
			specFile:            filepath.Join("testdata", "passing.goss.yaml"),
			expectedHTTPStatus:  http.StatusOK,
			expectedContentType: "application/vnd.goss-rspecish",
		},
	}
	for testName := range tests {
		tc := tests[testName]
		t.Run(testName, func(t *testing.T) {
			var logOutput bytes.Buffer
			log.SetOutput(&logOutput)

			config, err := util.NewConfig(
				util.WithSpecFile(tc.specFile),
				util.WithOutputFormat(tc.outputFormat),
			)
			require.NoError(t, err)

			hh, err := newHealthHandler(config)
			require.NoError(t, err)

			req := makeRequest(t, config, map[string][]string{
				"accept": tc.acceptHeader,
			})
			rr := httptest.NewRecorder()

			handler := http.HandlerFunc(hh.ServeHTTP)

			handler.ServeHTTP(rr, req)

			t.Logf("testName %q log output:\n%s", testName, logOutput.String())
			assert.Equal(t, tc.expectedHTTPStatus, rr.Code)
			if tc.expectedContentType != "" {
				assert.Equal(t, tc.expectedContentType, rr.Result().Header.Get("Content-Type"))
			}
		})
	}
}

func TestServeCacheWithNoContentNegotiation(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	const cache = time.Duration(time.Millisecond * 100)
	config, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithCache(cache),
	)
	require.NoError(t, err)

	hh, err := newHealthHandler(config)
	require.NoError(t, err)

	req := makeRequest(t, config, nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(hh.ServeHTTP)

	t.Run("fresh cache", func(t *testing.T) {
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.Contains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})

	t.Run("immediately re-request, cache should be warm", func(t *testing.T) {
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.NotContains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})

	t.Run("allow cache to expire, cache should be cold", func(t *testing.T) {
		time.Sleep(cache + 5*time.Millisecond)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.Contains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})
}

func TestServeCacheNegotiatingContent(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	const cache = time.Duration(time.Millisecond * 100)
	config, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithCache(cache),
		util.WithOutputFormat("structured"),
	)
	require.NoError(t, err)

	hh, err := newHealthHandler(config)
	require.NoError(t, err)

	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(hh.ServeHTTP)

	t.Run("fresh cache", func(t *testing.T) {
		req := makeRequest(t, config, map[string][]string{
			"accept": {"application/json"},
		})
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.Contains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})

	t.Run("immediately re-request, cache should be warm", func(t *testing.T) {
		req := makeRequest(t, config, map[string][]string{
			"accept": {"application/json"},
		})
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.NotContains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})

	t.Run("immediately re-request but different accept header, cache should be warm", func(t *testing.T) {
		req := makeRequest(t, config, map[string][]string{
			"accept": {"application/vnd.goss-rspecish"},
		})
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.NotContains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})

	t.Run("allow cache to expire, cache should be cold", func(t *testing.T) {
		time.Sleep(cache + 5*time.Millisecond)
		req := makeRequest(t, config, map[string][]string{
			"accept": {"application/json"},
		})
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		assert.Contains(t, logOutput.String(), "Stale cache")
		t.Log(logOutput.String())
		logOutput.Reset()
	})
}

func makeRequest(t *testing.T, config *util.Config, headers map[string][]string) *http.Request {
	req, err := http.NewRequest("GET", config.Endpoint, nil)
	require.NoError(t, err)
	for header, vals := range headers {
		for _, v := range vals {
			req.Header.Add(header, v)
		}
	}
	return req
}

// TestFillCacheReturnsExistingEntry covers the double-check inside fillCache:
// a request that queued on syverMu while another was validating must return
// what the winner stored, not run the validation over again.
//
// The cache is seeded with a sentinel that no real validation could produce, so
// getting it back is proof the re-check fired rather than the spec happening to
// yield the same thing.
func TestFillCacheReturnsExistingEntry(t *testing.T) {
	// Deliberately not t.Parallel(): constructing a health handler goes through
	// loadSyverConfig, which drives package-level globals (outStoreFormat,
	// currentTemplateFilter, quietDecode). Those assume no concurrent loads --
	// true of the CLI, not of two parallel tests -- so running in parallel here
	// would trip the race detector on pre-existing state rather than on anything
	// this test is about.

	config, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithOutputFormat("json"),
	)
	require.NoError(t, err)

	hh, err := newHealthHandler(config)
	require.NoError(t, err)

	sentinel := [][]resource.TestResult{{{
		Successful: true,
		ResourceId: "sentinel-not-produced-by-validation",
	}}}
	hh.cache.SetDefault("res", sentinel)

	got := hh.fillCache("res")

	require.Len(t, got, 1)
	require.Len(t, got[0], 1)
	assert.Equal(t, "sentinel-not-produced-by-validation", got[0][0].ResourceId,
		"fillCache re-ran validation instead of returning the entry already in the cache")
}

// TestServeConcurrentCacheMisses fires a burst of requests at one handler with
// a cold cache -- the case syverMu exists for. Before the lock was taken, every
// one of these ran its own full validation.
//
// Run under -race (CI does), this also covers the handler for data races on the
// shared cache and mutex. The assertion is deliberately on outcome rather than
// on a validation count: the latter is only observable through the global
// logger, which other parallel tests in this package also write to.
func TestServeConcurrentCacheMisses(t *testing.T) {
	// Deliberately not t.Parallel(): constructing a health handler goes through
	// loadSyverConfig, which drives package-level globals (outStoreFormat,
	// currentTemplateFilter, quietDecode). Those assume no concurrent loads --
	// true of the CLI, not of two parallel tests -- so running in parallel here
	// would trip the race detector on pre-existing state rather than on anything
	// this test is about.

	config, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithOutputFormat("json"),
	)
	require.NoError(t, err)

	hh, err := newHealthHandler(config)
	require.NoError(t, err)

	const concurrency = 16
	var wg sync.WaitGroup
	codes := make([]int, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			http.HandlerFunc(hh.ServeHTTP).ServeHTTP(rr, makeRequest(t, config, nil))
			codes[idx] = rr.Code
		}(i)
	}
	wg.Wait()

	for i, code := range codes {
		assert.Equal(t, http.StatusOK, code, "request %d", i)
	}
}
