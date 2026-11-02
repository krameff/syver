package system

import (
	"context"
	"strings"
	"testing"

	"github.com/krameff/syver/util"
)

func TestNewDefHTTPParsesRequestHeaders(t *testing.T) {
	tests := []struct {
		header    string
		name      string
		wantValue string
	}{
		{"X-Foo: bar", "X-Foo", "bar"},
		{"X-Foo:bar", "X-Foo", "bar"},
		{"  X-Foo  :  bar  ", "X-Foo", "bar"},
		{"Host: example.com:8080", "Host", "example.com:8080"},
		{"X-Empty:", "X-Empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			h := NewDefHTTP(context.Background(), "http://127.0.0.1:1", nil, util.Config{RequestHeader: []string{tt.header}}).(*DefHTTP)
			if h.err != nil {
				t.Fatalf("unexpected error: %v", h.err)
			}
			got, ok := h.RequestHeader[tt.name]
			if !ok || len(got) != 1 || got[0] != tt.wantValue {
				t.Fatalf("header %q = %q, want [%q]", tt.name, got, tt.wantValue)
			}
		})
	}
}

// A header with no colon at all is a spec error. It must come back as the
// resource's error when the check runs, not panic in the constructor.
func TestNewDefHTTPMalformedHeaderIsAnError(t *testing.T) {
	for _, header := range []string{"X-Foo", ": no name"} {
		t.Run(header, func(t *testing.T) {
			h := NewDefHTTP(context.Background(), "http://127.0.0.1:1", nil, util.Config{RequestHeader: []string{header}})
			_, err := h.Status()
			if err == nil || !strings.Contains(err.Error(), "invalid request header") {
				t.Fatalf("Status() err = %v, want an invalid request header error", err)
			}
		})
	}
}

// The error must never carry the header's value: a malformed Authorization
// header would otherwise put the credential into results and logs.
func TestNewDefHTTPMalformedHeaderErrorOmitsValue(t *testing.T) {
	const secret = "s3cr3t-token-value"
	tests := []struct {
		header  string
		secret  string
		wantPos string
	}{
		{"Bearer " + secret, secret, "header 2"},
		{": Bearer " + secret, secret, "header 2"},
		// Short enough that a truncated prefix would still be all of it.
		{"k3y9", "k3y9", "header 2"},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			h := NewDefHTTP(context.Background(), "http://127.0.0.1:1", nil,
				util.Config{RequestHeader: []string{"X-Ok: fine", tt.header}})
			_, err := h.Status()
			if err == nil {
				t.Fatal("want an error")
			}
			if strings.Contains(err.Error(), tt.secret) || strings.Contains(err.Error(), "Bear") ||
				strings.Contains(err.Error(), tt.secret[:2]) {
				t.Fatalf("error leaks the header value: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantPos) {
				t.Fatalf("error %q does not name %s", err, tt.wantPos)
			}
		})
	}
}
