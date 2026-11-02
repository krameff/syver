package system

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/krameff/syver/util"
	"github.com/miekg/dns"
)

type DNS interface {
	Host() string
	Addrs() ([]string, error)
	Resolvable() (bool, error)
	Exists() (bool, error)
	Server() string
	Qtype() string
}

type DefDNS struct {
	host       string
	resolvable bool
	addrs      []string
	Timeout    int
	loaded     bool
	err        error
	server     string
	qtype      string
}

func NewDefDNS(_ context.Context, host string, system *System, config util.Config) DNS {
	var h string
	var t string

	splitHost := strings.SplitN(host, ":", 2)
	if len(splitHost) == 2 && regexp.MustCompile(`^[A-Z]+$`).MatchString(splitHost[0]) {
		h = splitHost[1]
		t = splitHost[0]
	} else {
		h = host
	}

	return &DefDNS{
		host:    h,
		Timeout: config.TimeOutMilliSeconds(),
		server:  config.Server,
		qtype:   t,
	}
}

func (d *DefDNS) Host() string {
	return d.host
}

func (d *DefDNS) Server() string {
	return d.server
}

func (d *DefDNS) Qtype() string {
	return d.qtype
}

func (d *DefDNS) setup() error {
	if d.loaded {
		return d.err
	}
	d.loaded = true

	// The timeout bounds the whole lookup, every attempt together, not each
	// attempt: a resolver that hangs uses the budget on the first try, and
	// only failures that come back quickly (refused, SERVFAIL) are retried in
	// what is left.
	deadline := time.Now().Add(time.Duration(d.Timeout) * time.Millisecond)
	for i := 0; i < 3; i++ {
		budget := d.Timeout
		if i > 0 {
			budget = int(time.Until(deadline) / time.Millisecond)
			if budget <= 0 {
				break
			}
		}
		addrs, err := DNSlookup(d.host, d.server, d.qtype, budget)
		if err != nil || len(addrs) == 0 {
			d.resolvable = false
			d.addrs = []string{}
			// A name that does not exist is an answer, not a failure: it is
			// resolvable == false with no error. Any other resolver error (a
			// timeout, an unreachable or failing server) means the question
			// was never answered, so it is retried and then reported, rather
			// than passing as "not resolvable".
			if isNotFound(err) {
				d.err = nil
				return nil
			}
			d.err = err
			continue
		}
		sort.Strings(addrs)
		d.resolvable = true
		d.addrs = addrs
		d.err = nil
		return nil
	}
	return d.err
}

// isNotFound reports whether err is the resolver saying the name does not
// exist (NXDOMAIN, or no records of the requested type).
func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func (d *DefDNS) Addrs() ([]string, error) {
	err := d.setup()

	return d.addrs, err
}

func (d *DefDNS) Resolvable() (bool, error) {
	err := d.setup()

	return d.resolvable, err
}

// Stub out
func (d *DefDNS) Exists() (bool, error) {
	return false, nil
}

// lookupHost is the resolver used when no server is configured. A variable so
// tests can stand in for the system resolver.
var lookupHost = net.LookupHost

func DNSlookup(host string, server string, qtype string, timeout int) ([]string, error) {
	type result struct {
		addrs []string
		err   error
	}
	// One channel carrying both values: with separate channels for the
	// addresses and the error, both could be ready at once and select would
	// pick either, so an error could be dropped in favour of an empty result.
	c := make(chan result, 1)
	timeoutD := time.Duration(timeout) * time.Millisecond
	// Read once here, not in the goroutine, which may outlive this call.
	resolve := lookupHost

	go func() {
		// This goroutine has no caller above it to recover a panic, so a
		// malformed answer would otherwise kill the whole process, including
		// `syver serve`. The channel is buffered and the normal send has not
		// happened if we got here, so this send never blocks.
		defer func() {
			if rec := recover(); rec != nil {
				c <- result{err: fmt.Errorf("DNS lookup of %s failed: panic: %v", host, rec)}
			}
		}()
		addrs, err := dnsQuery(resolve, host, server, qtype, timeoutD)
		c <- result{addrs, err}
	}()
	select {
	case r := <-c:
		if r.err != nil {
			return nil, r.err
		}
		return r.addrs, nil
	case <-time.After(timeoutD):
		return nil, fmt.Errorf("DNS lookup timed out (%s)", timeoutD)
	}
}

func dnsQuery(resolve func(string) ([]string, error), host string, server string, qtype string, timeout time.Duration) ([]string, error) {
	if server == "" {
		return resolve(host)
	}
	c := new(dns.Client)
	c.Timeout = timeout
	m := new(dns.Msg)

	switch qtype {
	case "A":
		return LookupA(host, server, c, m)
	case "AAAA":
		return LookupAAAA(host, server, c, m)
	case "PTR":
		return LookupPTR(host, server, c, m)
	case "CNAME":
		return LookupCNAME(host, server, c, m)
	case "MX":
		return LookupMX(host, server, c, m)
	case "NS":
		return LookupNS(host, server, c, m)
	case "SRV":
		return LookupSRV(host, server, c, m)
	case "TXT":
		return LookupTXT(host, server, c, m)
	case "CAA":
		return LookupCAA(host, server, c, m)
	default:
		return LookupHost(host, server, c, m)
	}
}

// A and AAAA record lookup - similar to net.LookupHost
func LookupHost(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	a, errA := LookupA(host, server, c, m)
	aaaa, errAAAA := LookupAAAA(host, server, c, m)
	// Either family answering is enough. Only when both queries failed was
	// the server never heard from, and that is an error, not "no addresses".
	if errA != nil && errAAAA != nil {
		return nil, errA
	}
	addrs = append(a, aaaa...)

	return
}

// A record lookup
func LookupA(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeA)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.A); ok {
			addrs = append(addrs, t.A.String())
		}
	}

	return
}

// exchange sends m to server and checks the response code. NOERROR and
// NXDOMAIN are answers: the caller reads the answer section, and an empty one
// means not found. Any other code (SERVFAIL, REFUSED, NOTIMP, FORMERR, ...)
// means the server did not answer the question, which is an error rather than
// "not resolvable", matching how the system resolver path reports it.
func exchange(c *dns.Client, m *dns.Msg, server string) (*dns.Msg, error) {
	r, _, err := c.Exchange(m, parseServerString(server))
	if err != nil {
		return nil, err
	}
	if r.Rcode != dns.RcodeSuccess && r.Rcode != dns.RcodeNameError {
		var q string
		if len(m.Question) > 0 {
			q = " for " + dns.TypeToString[m.Question[0].Qtype] + " " + m.Question[0].Name
		}
		return nil, fmt.Errorf("DNS server %s answered %s%s", server, dns.RcodeToString[r.Rcode], q)
	}
	return r, nil
}

// parseServerString - Check if the DNS Server in server config has a port, if not ensure 53 is prefixed.
func parseServerString(server string) string {
	srvhost, srvport, err := net.SplitHostPort(server)
	if err != nil {
		srvport = "53"
		srvhost = server
	}
	return net.JoinHostPort(srvhost, srvport)
}

// AAAA (IPv6) record lookup
func LookupAAAA(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeAAAA)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.AAAA); ok {
			addrs = append(addrs, t.AAAA.String())
		}
	}

	return
}

// CNAME record lookup
func LookupCNAME(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeCNAME)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.CNAME); ok {
			addrs = append(addrs, t.Target)
		}
	}

	return
}

// MX record lookup
func LookupMX(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeMX)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.MX); ok {
			mxstring := strconv.Itoa(int(t.Preference)) + " " + t.Mx
			addrs = append(addrs, mxstring)
		}
	}

	return
}

// NS record lookup
func LookupNS(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeNS)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.NS); ok {
			addrs = append(addrs, t.Ns)
		}
	}

	return
}

// SRV record lookup
func LookupSRV(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeSRV)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.SRV); ok {
			prio := strconv.Itoa(int(t.Priority))
			weight := strconv.Itoa(int(t.Weight))
			port := strconv.Itoa(int(t.Port))
			srvrec := strings.Join([]string{prio, weight, port, t.Target}, " ")
			addrs = append(addrs, srvrec)
		}
	}

	return
}

// TXT record lookup
func LookupTXT(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeTXT)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.TXT); ok {
			addrs = append(addrs, t.Txt...)
		}
	}

	return
}

// PTR record lookup
func LookupPTR(addr string, server string, c *dns.Client, m *dns.Msg) (name []string, err error) {

	reverse, err := dns.ReverseAddr(addr)
	if err != nil {
		return nil, err
	}

	m.SetQuestion(reverse, dns.TypePTR)

	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	// The answer section can hold more than PTR records: RFC 2317 classless
	// reverse delegation answers with a CNAME to the delegated name first.
	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.PTR); ok {
			name = append(name, t.Ptr)
		}
	}

	return
}

// CAA record lookup
func LookupCAA(host string, server string, c *dns.Client, m *dns.Msg) (addrs []string, err error) {
	m.SetQuestion(dns.Fqdn(host), dns.TypeCAA)
	r, err := exchange(c, m, server)
	if err != nil {
		return nil, err
	}

	for _, ans := range r.Answer {
		if t, ok := ans.(*dns.CAA); ok {
			flag := strconv.Itoa(int(t.Flag))
			caarec := strings.Join([]string{flag, t.Tag, t.Value}, " ")
			addrs = append(addrs, caarec)
		}
	}

	return
}
