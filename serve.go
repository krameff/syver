package syver

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/resource"
	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Serve(c *util.Config) error {
	err := setLogLevel(c)
	if err != nil {
		return err
	}
	endpoint := c.Endpoint
	health, err := newHealthHandler(c)
	if err != nil {
		return err
	}
	http.Handle(endpoint, health)
	// Serve the outputs package's private registry, not the default global one.
	// The goss_tests_* metrics are registered into the private registry via
	// promauto.With(registry), so promhttp.Handler() (default registry) exposed
	// none of them and /metrics always returned zero matches. Initialised eagerly
	// with the process-level format options so label cardinality is deterministic
	// rather than set by whichever request happens to arrive first.
	http.Handle("/metrics", promhttp.HandlerFor(
		outputs.MetricsRegistry(util.OutputConfig{FormatOptions: c.FormatOptions}),
		promhttp.HandlerOpts{},
	))
	log.Printf("[INFO] Starting to listen on: %s", c.ListenAddress)
	return http.ListenAndServe(c.ListenAddress, nil)
}

func newHealthHandler(c *util.Config) (*healthHandler, error) {
	color.NoColor = true
	cache := cache.New(c.Cache, 30*time.Second)

	cfg, err := getSyverConfig(c.VarsFiles, c.VarsInline, c.Spec, nil)
	if err != nil {
		return nil, err
	}

	output, err := getOutputer(c.NoColor, c.OutputFormat)
	if err != nil {
		return nil, err
	}

	health := &healthHandler{
		c:             c,
		syverConfig:   *cfg,
		sys:           system.New(c.PackageManager),
		outputer:      output,
		cache:         cache,
		syverMu:       &sync.Mutex{},
		maxConcurrent: c.MaxConcurrent,
	}
	return health, nil
}

type res struct {
	body       bytes.Buffer
	statusCode int
}
type healthHandler struct {
	c             *util.Config
	syverConfig   SyverConfig
	sys           *system.System
	outputer      outputs.Outputer
	cache         *cache.Cache
	syverMu       *sync.Mutex
	maxConcurrent int
}

func (h healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	outputFormat, outputer, matchedPrefix, err := h.negotiateResponseContentType(r)
	if err != nil {
		log.Printf("[DEBUG] Warn: Using process-level output-format. %s", err)
		outputFormat = h.c.OutputFormat
		outputer = h.outputer
		// matchedPrefix is preserved from negotiateResponseContentType's
		// error return (see comment there) -- it already reflects the
		// client's attempted prefix family, or syver- if none.
	}
	negotiatedContentType := h.responseContentType(outputFormat, matchedPrefix)

	log.Printf("[TRACE] %v: requesting health probe", r.RemoteAddr)
	resp := h.processAndEnsureCached(negotiatedContentType, outputer)
	w.Header().Set("Content-Type", negotiatedContentType)
	w.WriteHeader(resp.statusCode)
	resp.body.WriteTo(w)
	log.Printf("[DEBUG] %v: status %d", r.RemoteAddr, resp.statusCode)
}

func (h healthHandler) processAndEnsureCached(negotiatedContentType string, outputer outputs.Outputer) res {
	var tra [][]resource.TestResult
	cacheKey := "res"
	tmp, found := h.cache.Get(cacheKey)
	if found {
		log.Printf("[TRACE] Returning cached[%s].", cacheKey)
		tra = tmp.([][]resource.TestResult)
	} else {
		log.Printf("Stale cache[%s], running tests", cacheKey)
		h.sys = system.New(h.c.PackageManager)
		tra = h.validate()
		h.cache.SetDefault(cacheKey, tra)
	}
	trc := testResultArrayToChan(tra)
	return h.output(trc, outputer)
}

func (h healthHandler) output(trc <-chan []resource.TestResult, outputer outputs.Outputer) res {
	var b bytes.Buffer
	outputConfig := util.OutputConfig{
		FormatOptions: h.c.FormatOptions,
	}
	exitCode := outputer.Output(&b, trc, outputConfig)
	resp := res{
		body: b,
	}
	if exitCode == 0 {
		resp.statusCode = http.StatusOK
	} else {
		resp.statusCode = http.StatusServiceUnavailable
	}
	return resp
}
func (h healthHandler) validate() [][]resource.TestResult {
	h.sys = system.New(h.c.PackageManager)
	res := make([][]resource.TestResult, 0)
	tr, err := runValidation(h.sys, h.syverConfig, h.c.DisabledResourceTypes, h.maxConcurrent)
	if err != nil {
		return res
	}
	for i := range tr {
		res = append(res, i)
	}
	return res
}

func testResultArrayToChan(tra [][]resource.TestResult) <-chan []resource.TestResult {
	c := make(chan []resource.TestResult)
	go func(c chan []resource.TestResult) {
		defer close(c)

		for _, i := range tra {
			c <- i
		}
	}(c)

	return c
}

const (
	// https://en.wikipedia.org/wiki/Media_type
	mediaTypePrefixGoss  = "application/vnd.goss-"
	mediaTypePrefixSyver = "application/vnd.syver-"
)

// negotiateResponseContentType accepts both vnd.goss- and vnd.syver-
// prefixed Accept headers and remembers which one the client actually
// used, so responseContentType can echo it back rather than always
// emitting one or the other.
//
// A client that sent no vendor-specific Accept header expressed no
// preference, so there is nothing to echo. In that case we keep emitting
// the legacy goss- prefix: PLAN section 4's compatibility table marks this
// row "echo the client's" and does NOT mark it a hard break, and the most
// common real caller (a health probe that never sets Accept at all) would
// otherwise see its Content-Type change silently. Clients that do ask for
// vnd.syver- still get vnd.syver- echoed back.
func (h healthHandler) negotiateResponseContentType(r *http.Request) (string, outputs.Outputer, string, error) {
	acceptHeader := r.Header[http.CanonicalHeaderKey("Accept")]
	var outputer outputs.Outputer
	outputName := ""
	matchedPrefix := mediaTypePrefixGoss
	for _, acceptCandidate := range acceptHeader {
		acceptCandidate = strings.TrimSpace(acceptCandidate)
		switch {
		case strings.HasPrefix(acceptCandidate, mediaTypePrefixGoss):
			outputName = strings.TrimPrefix(acceptCandidate, mediaTypePrefixGoss)
			matchedPrefix = mediaTypePrefixGoss
		case strings.HasPrefix(acceptCandidate, mediaTypePrefixSyver):
			outputName = strings.TrimPrefix(acceptCandidate, mediaTypePrefixSyver)
			matchedPrefix = mediaTypePrefixSyver
		case strings.EqualFold("application/json", acceptCandidate) || strings.EqualFold("text/json", acceptCandidate):
			outputName = "json"
			matchedPrefix = mediaTypePrefixGoss
		default:
			outputName = ""
			matchedPrefix = mediaTypePrefixGoss
		}
		var err error
		candidate, err := outputs.GetOutputer(outputName)
		if err != nil {
			// Do not clobber an outputer an earlier candidate already
			// resolved -- a later invalid Accept value used to null it out
			// and force a fallback even though a valid format was found.
			continue
		}
		outputer = candidate
		break
	}
	if outputer == nil {
		// matchedPrefix still reflects whichever prefix family the client
		// attempted (or the syver- default if none was recognizable at
		// all), so a caller falling back to the process-level format can
		// still echo the client's attempted family rather than always
		// defaulting to syver-.
		return "", nil, matchedPrefix, fmt.Errorf("accept header on request missing or invalid")
	}

	return outputName, outputer, matchedPrefix, nil
}

func (h healthHandler) responseContentType(outputName, matchedPrefix string) string {
	if outputName == "json" {
		return "application/json"
	}
	if outputName == "prometheus" {
		return "text/plain; version=0.0.4"
	}

	return fmt.Sprintf("%s%s", matchedPrefix, outputName)
}
