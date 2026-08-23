package syver

import (
	"bytes"
	"context"
	"errors"
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

func Serve(ctx context.Context, c *util.Config) error {
	err := setLogLevel(c)
	if err != nil {
		return err
	}
	endpoint := c.Endpoint
	health, err := newHealthHandler(ctx, c)
	if err != nil {
		return err
	}
	// A dedicated mux rather than DefaultServeMux: the global one is process-wide
	// shared state, so registering into it makes a second Serve call in the same
	// process panic on the duplicate pattern, and lets anything else linked in
	// register routes on our listener.
	mux := http.NewServeMux()
	mux.Handle(endpoint, health)
	// Serve the outputs package's private registry, not the default global one.
	// The goss_tests_* metrics are registered into the private registry via
	// promauto.With(registry), so promhttp.Handler() (default registry) exposed
	// none of them and /metrics always returned zero matches. Initialised eagerly
	// with the process-level format options so label cardinality is deterministic
	// rather than set by whichever request happens to arrive first.
	mux.Handle("/metrics", promhttp.HandlerFor(
		outputs.MetricsRegistry(util.OutputConfig{FormatOptions: c.FormatOptions}),
		promhttp.HandlerOpts{},
	))

	// http.ListenAndServe uses a zero-value Server, which has no deadlines at
	// all. On an endpoint that is unauthenticated by design, that lets a client
	// open a connection and dribble its request headers forever, pinning a
	// goroutine and a descriptor per connection (Slowloris).
	//
	// WriteTimeout is deliberately left unset. How long a response takes is
	// decided by the spec under test, and a `command` resource can legitimately
	// run for minutes; a write deadline would cut the body mid-flight and turn a
	// slow pass into a truncated failure that looks like the server broke. The
	// read-side and idle deadlines cost nothing here, because requests to this
	// endpoint carry no body.
	srv := &http.Server{
		Addr:              c.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe blocks and never looks at a context, so without this the
	// process ignores SIGINT/SIGTERM entirely (main's handler suppresses the
	// default disposition) and has to be SIGKILLed. Worse than a hang: baseCtx
	// is that same context, so every check after the signal fails with
	// "context canceled" and the endpoint serves 503 to every probe while
	// refusing to exit -- a container burning its whole termination grace
	// period while reporting itself unhealthy.
	shutdownErr := make(chan error, 1)
	go func() {
		<-ctx.Done()
		log.Printf("[INFO] Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		shutdownErr <- srv.Shutdown(shutdownCtx)
	}()

	log.Printf("[INFO] Starting to listen on: %s", c.ListenAddress)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-shutdownErr
}

// color.NoColor is a package global in fatih/color, and this used to assign it
// on every call. Serve() constructs exactly one handler so production never
// noticed, but any two callers racing here is a write-write data race on that
// global -- which is precisely what the parallel tests in this package do, and
// what `go test -race` reports against the unguarded version.
//
// The assignment is idempotent, so doing it once is equivalent. sync.Once also
// establishes happens-before for every later caller, so no output goroutine can
// be reading the flag while the first writer sets it.
var noColorOnce sync.Once

func newHealthHandler(ctx context.Context, c *util.Config) (*healthHandler, error) {
	noColorOnce.Do(func() { color.NoColor = true })
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
		baseCtx:       ctx,
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

	// baseCtx is the server's context, not any one request's. fillCache runs a
	// single shared sweep and hands the same result to every waiter, so scoping
	// it to r.Context() would let one client hanging up cancel the work the
	// other waiters are blocked on -- turning a disconnect into a failed probe
	// for everybody else. Cancelling the server cancels the sweep; cancelling a
	// request does not.
	baseCtx context.Context
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
		tra = h.fillCache(cacheKey)
	}
	trc := testResultArrayToChan(tra)
	return h.output(trc, outputer, anyFailed(tra))
}

// anyFailed reads the verdict from the results themselves.
//
// The health status must not be derived from the outputter's exit code. That
// number answers "did this format render successfully", which for most formats
// coincides with the verdict but for prometheus deliberately does not: it is an
// encoding, its outcome lives in a label value, and it correctly returns 0 even
// when every check failed. Deriving the status from it meant a client could ask
// for `Accept: application/vnd.goss-prometheus` and get 200 from a host where
// nothing passed -- the same inversion that `structured` had, one header over.
// Metrics have their own endpoint now (/metrics), so /healthz has no reason to
// inherit an encoder's return value.
func anyFailed(tra [][]resource.TestResult) bool {
	for _, group := range tra {
		for _, r := range group {
			if r.Result == resource.FAIL {
				return true
			}
		}
	}
	return false
}

// fillCache runs the validation and stores the result, serializing concurrent
// misses so that a burst of probes arriving on a cold or just-expired cache
// triggers one sweep of the system rather than one per request.
//
// syverMu has been carried since upstream (as gossMu) but was allocated and
// never taken, so that amplification was live: N simultaneous requests ran N
// full validations, each shelling out to the same package managers and
// services. It is a pointer, so every value-receiver copy of healthHandler
// shares the one mutex.
func (h healthHandler) fillCache(cacheKey string) [][]resource.TestResult {
	h.syverMu.Lock()
	defer h.syverMu.Unlock()

	// Re-check under the lock. Whichever request won the race has already
	// stored its result, and the rest should return it rather than redo the
	// work they queued for.
	if tmp, found := h.cache.Get(cacheKey); found {
		log.Printf("[TRACE] Returning cached[%s], filled while waiting.", cacheKey)
		return tmp.([][]resource.TestResult)
	}

	log.Printf("Stale cache[%s], running tests", cacheKey)
	h.sys = system.New(h.c.PackageManager)
	tra := h.validate(h.baseCtx)
	h.cache.SetDefault(cacheKey, tra)
	return tra
}

func (h healthHandler) output(trc <-chan []resource.TestResult, outputer outputs.Outputer, failed bool) res {
	var b bytes.Buffer
	outputConfig := util.OutputConfig{
		FormatOptions: h.c.FormatOptions,
	}
	// The outputter still writes the body; its return value is deliberately
	// ignored for the status. See anyFailed.
	_ = outputer.Output(&b, trc, outputConfig)
	resp := res{
		body: b,
	}
	if failed {
		resp.statusCode = http.StatusServiceUnavailable
	} else {
		resp.statusCode = http.StatusOK
	}
	return resp
}
func (h healthHandler) validate(ctx context.Context) [][]resource.TestResult {
	h.sys = system.New(h.c.PackageManager)
	res := make([][]resource.TestResult, 0)
	tr, err := runValidation(ctx, h.sys, h.syverConfig, h.c.DisabledResourceTypes, h.maxConcurrent)
	if err != nil {
		// Returning the empty set here used to answer 200 having run zero checks:
		// no results means no failures, so every verdict rule -- including the
		// results-based one -- reads it as healthy, and the error was dropped
		// without a log line, so nothing anywhere said otherwise. The triggers are
		// all static properties of the spec (unknown or ambiguous depends-on ref,
		// invalid ref syntax, duplicate resource ref, dependency cycle), so it is
		// permanent from process start, and fillCache caches the green answer.
		//
		// A synthetic failing result makes the endpoint 503 and puts the reason in
		// the body, where an operator looking at a failing probe will actually see
		// it. `validate` already exits 1 on the same error; this makes serve agree.
		log.Printf("[ERROR] Validation could not run: %s", err)
		return [][]resource.TestResult{{{
			Successful:   false,
			Result:       resource.FAIL,
			ResourceType: "Syverfile",
			ResourceId:   h.c.Spec,
			Property:     "validation",
			Err:          resource.NewValidateError(err),
		}}}
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
