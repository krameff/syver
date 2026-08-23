package syver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/krameff/syver/outputs"
	"github.com/krameff/syver/util"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// Serve must return when its context is cancelled. Without this, main's
// signal.NotifyContext -- which suppresses the default "signal terminates the
// process" disposition -- turns SIGINT/SIGTERM into a no-op for `syver serve`,
// leaving a container that ignores docker stop, burns its whole termination
// grace period, and gets SIGKILLed. Worse, baseCtx IS that context, so every
// check after the signal fails with "context canceled" and the endpoint answers
// 503 to every liveness probe the entire time it refuses to die.
//
// Nothing at the cmd layer is covered by tests, which is exactly why the
// original empirical "Ctrl-C works" check passed: it exercised plain validate,
// where cancellation happens to let the run finish naturally.
func TestServeReturnsWhenContextCancelled(t *testing.T) {
	port := freePort(t)
	c, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithNoColor(),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	// No ConfigOption exists for these two; Serve reads them off the struct.
	c.ListenAddress = fmt.Sprintf("127.0.0.1:%d", port)
	c.Endpoint = "/healthz"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, c) }()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	waitUntilServing(t, url)

	cancel()
	select {
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve returned %v, want nil or ErrServerClosed", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return within 15s of cancellation: it is not watching ctx, " +
			"so the process would ignore SIGINT/SIGTERM and have to be SIGKILLed")
	}
}

func waitUntilServing(t *testing.T, url string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		resp, err := http.Get(url) //nolint:noctx // short-lived readiness poll
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server never became ready")
}

// fillCache runs ONE shared sweep and hands the same result to every waiter, so
// it deliberately uses the SERVER's context rather than r.Context(). Scoping it
// to the request would let a single client hanging up cancel work the other
// waiters are blocked on, turning one disconnect into a failed probe for
// everybody else. Switching to r.Context() looks like an obvious correctness
// improvement to anyone who has not read the comment on baseCtx, and every
// other test in this package stays green if you make it. This one does not.
//
// WithOutputFormat is load-bearing, not boilerplate: without an output format
// the outputer returns exit code 0 for every result set, so the handler answers
// 200 no matter what the checks did and the assertion below cannot fail.
func TestSweepUsesTheServerContextNotTheRequests(t *testing.T) {
	newHandler := func(t *testing.T, ctx context.Context) *healthHandler {
		t.Helper()
		c, err := util.NewConfig(
			util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
			util.WithOutputFormat("rspecish"),
			util.WithNoColor(),
		)
		if err != nil {
			t.Fatalf("config: %v", err)
		}
		h, err := newHealthHandler(ctx, c)
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		return h
	}
	probe := func(h *healthHandler, reqCtx context.Context) int {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		if reqCtx != nil {
			req = req.WithContext(reqCtx)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("cancelling the server reaches the sweep", func(t *testing.T) {
		serverCtx, cancelServer := context.WithCancel(context.Background())
		h := newHandler(t, serverCtx)
		if got := probe(h, nil); got != http.StatusOK {
			t.Fatalf("baseline: got %d, want 200", got)
		}
		cancelServer()
		h.cache.Flush()
		if got := probe(h, nil); got == http.StatusOK {
			t.Fatalf("got 200 after cancelling the server: the sweep is not using baseCtx")
		}
	})

	t.Run("a client hanging up does not fail the sweep", func(t *testing.T) {
		h := newHandler(t, context.Background())
		dead, cancel := context.WithCancel(context.Background())
		cancel()
		if got := probe(h, dead); got != http.StatusOK {
			t.Fatalf("got %d, want 200: the sweep inherited the dead REQUEST context, "+
				"so one client disconnecting fails the check for every waiter", got)
		}
	})
}

// /healthz's status must come from the RESULTS, never from the outputter's exit
// code. Those answer different questions: the exit code says "did this format
// render", which for prometheus is deliberately 0 even when every check failed,
// because it is an encoding whose outcome lives in a label value rather than a
// status. Deriving the status from it let a client ask for
// Accept: application/vnd.goss-prometheus and receive 200 from a host where
// nothing passed -- the same inversion `structured` had, one header over.
//
// Driven off Outputers() so a new format cannot quietly reintroduce it.
func TestHealthStatusIgnoresTheOutputtersExitCode(t *testing.T) {
	for _, spec := range []struct {
		file string
		want int
	}{
		{"failing.goss.yaml", http.StatusServiceUnavailable},
		{"passing.goss.yaml", http.StatusOK},
	} {
		for _, format := range outputs.Outputers() {
			if format == "discovery" {
				continue // not a real Outputer; GetOutputer rejects it
			}
			t.Run(spec.file+"/"+format, func(t *testing.T) {
				c, err := util.NewConfig(
					util.WithSpecFile(filepath.Join("testdata", spec.file)),
					util.WithOutputFormat(format),
					util.WithNoColor(),
				)
				if err != nil {
					t.Fatalf("config: %v", err)
				}
				h, err := newHealthHandler(context.Background(), c)
				if err != nil {
					t.Fatalf("handler: %v", err)
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
				if rec.Code != spec.want {
					t.Errorf("format %q on %s: got %d, want %d -- the status is tracking the "+
						"outputter's exit code instead of the results",
						format, spec.file, rec.Code, spec.want)
				}
			})
		}
	}
}
