package syver

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWithNoContentNegotiation(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
