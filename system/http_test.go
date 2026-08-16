package system

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
)

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestDefHTTPCloseClosesBodyAfterBodyRead(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("ok")}
	httpResource := &DefHTTP{
		loaded: true,
		resp: &http.Response{
			Body: body,
		},
	}

	reader, err := httpResource.Body()
	if err != nil {
		t.Fatalf("Body() returned error: %v", err)
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("ReadAll() returned error: %v", err)
	}
	if err := httpResource.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if !body.closed {
		t.Fatal("Close() did not close the response body")
	}
}

// TestDefaultUserAgentPrefixIsSyver covers §5.6: outbound http resource
// checks send `User-Agent: syver/<version>`, not `goss/<version>`. This
// is a deliberate hard break, not a compat-shimmed alias.
func TestDefaultUserAgentPrefixIsSyver(t *testing.T) {
	if DEFAULT_USER_AGENT_PREFIX != "syver/" {
		t.Fatalf("DEFAULT_USER_AGENT_PREFIX = %q, want %q", DEFAULT_USER_AGENT_PREFIX, "syver/")
	}
}

// TestNewDefHTTPSetsSyverUserAgent covers the same requirement at the
// NewDefHTTP construction point: when the caller supplies no explicit
// User-Agent header, the auto-populated one must start with "syver/".
func TestNewDefHTTPSetsSyverUserAgent(t *testing.T) {
	h := NewDefHTTP(context.Background(), "http://example.com", nil, util.Config{})
	def, ok := h.(*DefHTTP)
	if !ok {
		t.Fatalf("NewDefHTTP did not return *DefHTTP")
	}
	ua := def.RequestHeader.Get("User-Agent")
	if !strings.HasPrefix(ua, "syver/") {
		t.Fatalf("User-Agent = %q, want prefix %q", ua, "syver/")
	}
	if strings.HasPrefix(ua, "goss/") {
		t.Fatalf("User-Agent = %q still has the legacy goss/ prefix", ua)
	}
}
