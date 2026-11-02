package system

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krameff/syver/util"
	"github.com/miekg/dns"
)

// startFakeDNS serves handler on a local UDP port and returns its address.
func startFakeDNS(t *testing.T, handler dns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: pc, Handler: handler, NotifyStartedFunc: func() { close(started) }}
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

// closedUDPAddr returns a local UDP address with nothing listening on it.
func closedUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()
	return addr
}

// An RFC 2317 classless reverse delegation answers a PTR query with a CNAME
// to the delegated name ahead of the PTR itself. The lookup must skip the
// CNAME rather than assert every answer is a PTR.
func TestLookupPTRSkipsCNAMEInAnswer(t *testing.T) {
	server := startFakeDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		q := req.Question[0].Name
		m.Answer = append(m.Answer,
			&dns.CNAME{Hdr: dns.RR_Header{Name: q, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 60}, Target: "5.0-25.2.0.192.in-addr.arpa."},
			&dns.PTR{Hdr: dns.RR_Header{Name: "5.0-25.2.0.192.in-addr.arpa.", Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: 60}, Ptr: "host.example."},
		)
		_ = w.WriteMsg(m)
	})

	addrs, err := DNSlookup("192.0.2.5", server, "PTR", 2000)
	if err != nil {
		t.Fatalf("DNSlookup returned error: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "host.example." {
		t.Fatalf("addrs = %v, want [host.example.]", addrs)
	}
}

// DNSlookup used to send an error and then the (empty) addresses on two
// buffered channels, so select picked either and an error came back as
// (nil, nil) whenever both were ready before select ran. That window is narrow
// (measured: roughly one call in a few thousand), so this repeats enough to hit
// it reliably while still taking only milliseconds.
func TestDNSlookupNeverDropsTheError(t *testing.T) {
	want := &net.DNSError{Err: "i/o timeout", Name: "example.test", IsTimeout: true}
	orig := lookupHost
	lookupHost = func(string) ([]string, error) { return nil, want }
	t.Cleanup(func() { lookupHost = orig })

	for i := 0; i < 20000; i++ {
		_, err := DNSlookup("example.test", "", "", 2000)
		if !errors.Is(err, want) {
			t.Fatalf("iteration %d: err = %v, want %v", i, err, want)
		}
	}
}

func TestDefDNSDefaultResolverErrors(t *testing.T) {
	tests := []struct {
		name           string
		lookupErr      error
		wantResolvable bool
		wantErr        bool
	}{
		{
			name:      "not found is unresolvable with no error",
			lookupErr: &net.DNSError{Err: "no such host", Name: "missing.test", IsNotFound: true},
		},
		{
			name:      "timeout is reported",
			lookupErr: &net.DNSError{Err: "i/o timeout", Name: "missing.test", IsTimeout: true},
			wantErr:   true,
		},
		{
			name:      "server failure is reported",
			lookupErr: &net.DNSError{Err: "server misbehaving", Name: "missing.test", IsTemporary: true},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := lookupHost
			lookupHost = func(string) ([]string, error) { return nil, tt.lookupErr }
			t.Cleanup(func() { lookupHost = orig })

			d := NewDefDNS(context.Background(), "missing.test", nil, util.Config{Timeout: 2 * time.Second})
			resolvable, err := d.Resolvable()
			if resolvable != tt.wantResolvable {
				t.Errorf("resolvable = %v, want %v", resolvable, tt.wantResolvable)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// With a server configured and no record type, the A and AAAA queries used to
// discard their errors, so an unreachable server read as "not resolvable".
func TestDefDNSUnreachableServerIsAnError(t *testing.T) {
	d := NewDefDNS(context.Background(), "example.test", nil, util.Config{
		Server:  closedUDPAddr(t),
		Timeout: 500 * time.Millisecond,
	})
	resolvable, err := d.Resolvable()
	if resolvable {
		t.Errorf("resolvable = true, want false")
	}
	if err == nil {
		t.Fatal("err = nil, want an error for an unreachable server")
	}
}

// A server that answers but has no such name is still a plain "not
// resolvable" with no error.
func TestDefDNSServerNXDOMAINIsNotAnError(t *testing.T) {
	server := startFakeDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeNameError)
		_ = w.WriteMsg(m)
	})
	d := NewDefDNS(context.Background(), "missing.test", nil, util.Config{Server: server, Timeout: 2 * time.Second})
	resolvable, err := d.Resolvable()
	if resolvable || err != nil {
		t.Fatalf("Resolvable() = %v, %v; want false, nil", resolvable, err)
	}
}

// The resource timeout bounds the lookup as a whole. Retrying a resolver that
// hangs used to take up to three times the timeout.
func TestDefDNSTimeoutBoundsAllAttempts(t *testing.T) {
	hang := make(chan struct{})
	orig := lookupHost
	lookupHost = func(string) ([]string, error) { <-hang; return nil, nil }
	t.Cleanup(func() { lookupHost = orig; close(hang) })

	const timeout = 300 * time.Millisecond
	d := NewDefDNS(context.Background(), "hangs.test", nil, util.Config{Timeout: timeout})
	start := time.Now()
	_, err := d.Resolvable()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("err = nil, want a timeout error")
	}
	if elapsed > 2*timeout {
		t.Fatalf("lookup took %s with a %s timeout", elapsed, timeout)
	}
}

// Failures that return quickly are still retried, and not-found is not.
func TestDefDNSRetries(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCalls int
	}{
		{"server failure retried", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, 3},
		{"not found answered at once", &net.DNSError{Err: "no such host", IsNotFound: true}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			orig := lookupHost
			lookupHost = func(string) ([]string, error) { calls.Add(1); return nil, tt.err }
			t.Cleanup(func() { lookupHost = orig })

			d := NewDefDNS(context.Background(), "x.test", nil, util.Config{Timeout: 2 * time.Second})
			_, _ = d.Resolvable()
			if got := int(calls.Load()); got != tt.wantCalls {
				t.Fatalf("lookups = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

// With a server configured, a response code other than NOERROR or NXDOMAIN
// means the question went unanswered. It used to read as an empty answer, so
// SERVFAIL and REFUSED passed as "not resolvable".
func TestDefDNSServerRcodes(t *testing.T) {
	qtypes := []string{"", "A", "AAAA", "CNAME", "MX", "NS", "PTR", "SRV", "TXT", "CAA"}
	rcodes := []struct {
		rcode   int
		wantErr bool
	}{
		{dns.RcodeSuccess, false},
		{dns.RcodeNameError, false},
		{dns.RcodeServerFailure, true},
		{dns.RcodeRefused, true},
		{dns.RcodeNotImplemented, true},
		{dns.RcodeFormatError, true},
	}
	for _, rc := range rcodes {
		server := startFakeDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
			m := new(dns.Msg)
			m.SetRcode(req, rc.rcode)
			_ = w.WriteMsg(m)
		})
		for _, qtype := range qtypes {
			host := "example.test"
			if qtype == "PTR" {
				host = "192.0.2.5"
			}
			if qtype != "" {
				host = qtype + ":" + host
			}
			t.Run(dns.RcodeToString[rc.rcode]+"/"+host, func(t *testing.T) {
				d := NewDefDNS(context.Background(), host, nil, util.Config{Server: server, Timeout: 2 * time.Second})
				resolvable, err := d.Resolvable()
				if resolvable {
					t.Errorf("resolvable = true, want false")
				}
				if (err != nil) != rc.wantErr {
					t.Fatalf("err = %v, wantErr %v", err, rc.wantErr)
				}
				if rc.wantErr && !strings.Contains(err.Error(), dns.RcodeToString[rc.rcode]) {
					t.Errorf("err %q does not name the rcode %s", err, dns.RcodeToString[rc.rcode])
				}
			})
		}
	}
}
