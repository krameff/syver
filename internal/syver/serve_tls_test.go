package syver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/fips140"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/krameff/syver/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// None of the tests in this file are t.Parallel(): Serve and newHealthHandler
// retarget the process-global logger and drive package-level globals (see
// TestServeWithNoContentNegotiation).

// writeSelfSignedPair writes a fresh ECDSA P-256 self-signed certificate for
// 127.0.0.1 and ::1, and its key, under t.TempDir(). With crlf set, both PEM
// files are rewritten with \r\n line endings, as a Windows editor leaves them.
// No key or certificate is ever checked in.
func writeSelfSignedPair(t *testing.T, crlf bool) (certPath, keyPath string, cert *x509.Certificate) {
	t.Helper()
	now := time.Now()
	return writePairValidBetween(t, crlf, now.Add(-time.Hour), now.Add(time.Hour))
}

// writePairValidBetween is writeSelfSignedPair with the validity window and any
// DNS names given, for the expired, not-yet-valid and logging cases.
func writePairValidBetween(t *testing.T, crlf bool, notBefore, notAfter time.Time, dnsNames ...string) (certPath, keyPath string, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "syver serve test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if crlf {
		certPEM = bytes.ReplaceAll(certPEM, []byte("\n"), []byte("\r\n"))
		keyPEM = bytes.ReplaceAll(keyPEM, []byte("\n"), []byte("\r\n"))
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "server.crt")
	keyPath = filepath.Join(dir, "server.key")
	require.NoError(t, os.WriteFile(certPath, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))
	return certPath, keyPath, cert
}

// tlsClientFor trusts exactly cert, and has a timeout so a readiness poll or a
// hung handshake cannot stall the test.
func tlsClientFor(cert *x509.Certificate) *http.Client {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
}

// TestServeTLSConfig covers serveTLSConfig's accept and refuse paths. The
// revert-proof for each row is in its comment.
func TestServeTLSConfig(t *testing.T) {
	certPath, keyPath, _ := writeSelfSignedPair(t, false)
	_, otherKey, _ := writeSelfSignedPair(t, false)
	missing := filepath.Join(t.TempDir(), "absent.key")

	// Revert-proof: return a non-nil config unconditionally.
	t.Run("neither set", func(t *testing.T) {
		cfg, err := serveTLSConfig(&util.Config{})
		require.NoError(t, err)
		assert.Nil(t, cfg, "no TLS files must mean plain HTTP")
	})

	// Revert-proof: delete the MinVersion line.
	t.Run("both set", func(t *testing.T) {
		cfg, err := serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: keyPath})
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Len(t, cfg.Certificates, 1)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	})

	// Revert-proof: drop the one-flag check; LoadX509KeyPair on "" gives a
	// file error instead.
	t.Run("cert only", func(t *testing.T) {
		_, err := serveTLSConfig(&util.Config{ServeCertFile: certPath})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs both --tls-cert and --tls-key")
		assert.Contains(t, err.Error(), "only --tls-cert is set")
	})

	// Revert-proof: as for "cert only".
	t.Run("key only", func(t *testing.T) {
		_, err := serveTLSConfig(&util.Config{ServeKeyFile: keyPath})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs both --tls-cert and --tls-key")
		assert.Contains(t, err.Error(), "only --tls-key is set")
	})

	// The wrapped *os.PathError already names the missing key, so only the
	// format string puts the EXISTING certificate's path in the message.
	// Revert-proof: drop the %q paths from the format string.
	t.Run("key path missing", func(t *testing.T) {
		_, err := serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: missing})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading tls key pair")
		assert.Contains(t, err.Error(), strconv.Quote(certPath))
	})

	// Revert-proof (this row and the two below): replace LoadX509KeyPair with
	// loading the certificate alone.
	t.Run("mismatched pair", func(t *testing.T) {
		_, err := serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: otherKey})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading tls key pair")
	})

	t.Run("swapped files", func(t *testing.T) {
		_, err := serveTLSConfig(&util.Config{ServeCertFile: keyPath, ServeKeyFile: certPath})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading tls key pair")
	})

	t.Run("unreadable key", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows: mode bits from chmod do not deny read, so the key stays readable")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root: root reads a mode-000 file anyway, so it is not unreadable")
		}
		dir := t.TempDir()
		locked := filepath.Join(dir, "locked.key")
		data, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(locked, data, 0o600)) //nolint:gosec // locked is under t.TempDir()
		require.NoError(t, os.Chmod(locked, 0o000))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
		_, err = serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: locked})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "loading tls key pair")
	})

	// The standard library's error for an encrypted key never says
	// "encrypted" (PKCS#8 gives an ASN.1 error, the legacy form a length
	// error), so serveTLSConfig names it. The key bytes here are random: only
	// the block type or header marks a key as encrypted, and that is all the
	// check reads.
	// Revert-proof: remove the keyFileIsEncrypted branch from serveTLSConfig.
	for name, block := range map[string]*pem.Block{
		"encrypted PKCS#8 key": {Type: "ENCRYPTED PRIVATE KEY", Bytes: randomBytes(t, 96)},
		"encrypted legacy key": {
			Type: "EC PRIVATE KEY",
			Headers: map[string]string{
				"Proc-Type": "4,ENCRYPTED",
				"DEK-Info":  "AES-128-CBC,00112233445566778899AABBCCDDEEFF",
			},
			Bytes: randomBytes(t, 96),
		},
	} {
		t.Run(name, func(t *testing.T) {
			enc := filepath.Join(t.TempDir(), "encrypted.key")
			require.NoError(t, os.WriteFile(enc, pem.EncodeToMemory(block), 0o600))
			_, err := serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: enc})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the private key is encrypted")
			assert.Contains(t, err.Error(), strconv.Quote(certPath))
			assert.Contains(t, err.Error(), strconv.Quote(enc))
			assert.NotContains(t, err.Error(), "BEGIN", "file contents must never reach the message")
		})
	}

	// PEM files saved by a Windows editor have CRLF endings. This proves the
	// standard library's encoding/pem accepts them, so the docs need no
	// warning. Revert-proof: none in Syver code, it is a property of the
	// standard library; the row guards against a future custom PEM reader.
	t.Run("CRLF pair", func(t *testing.T) {
		crlfCert, crlfKey, _ := writeSelfSignedPair(t, true)
		raw, err := os.ReadFile(crlfCert)
		require.NoError(t, err)
		require.Contains(t, string(raw), "\r\n", "the fixture must really be CRLF")
		cfg, err := serveTLSConfig(&util.Config{ServeCertFile: crlfCert, ServeKeyFile: crlfKey})
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

// stderrToFile points os.Stderr at a file for the rest of the test, so the
// level-filtered output Serve's setLogLevel installs can be read back, and
// returns a function that reads it. Tests using it must not be parallel.
func stderrToFile(t *testing.T) func() string {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stderr.log"))
	require.NoError(t, err)
	prev := os.Stderr
	os.Stderr = f
	t.Cleanup(func() {
		os.Stderr = prev
		log.SetOutput(os.Stderr)
		_ = f.Close()
	})
	return func() string {
		b, err := os.ReadFile(f.Name())
		require.NoError(t, err)
		return string(b)
	}
}

// insecureReadinessClient polls a server whose certificate the test made
// invalid on purpose; it checks only that serve answers.
func insecureReadinessClient() *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // the certificate is expired or not yet valid by design
			MinVersion:         tls.VersionTLS12,
		}},
	}
}

// newTLSServeConfig is a serve config for the passing spec, answering json,
// with the given pair.
func newTLSServeConfig(t *testing.T, certPath, keyPath string) *util.Config {
	t.Helper()
	c, err := util.NewConfig(
		util.WithSpecFile(filepath.Join("testdata", "passing.goss.yaml")),
		util.WithOutputFormat("json"),
		util.WithNoColor(),
	)
	require.NoError(t, err)
	c.Endpoint = "/healthz"
	c.ServeCertFile = certPath
	c.ServeKeyFile = keyPath
	return c
}

// checkHealthOverTLS GETs the endpoint over https and asserts a 200 served
// with the generated certificate. It is T2's first row, shared with the FIPS
// run.
func checkHealthOverTLS(t *testing.T, client *http.Client, url string, cert *x509.Certificate) {
	t.Helper()
	resp, err := client.Get(url) //nolint:noctx // test request
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.TLS, "the response did not arrive over TLS")
	require.NotEmpty(t, resp.TLS.PeerCertificates)
	assert.Equal(t, cert.Raw, resp.TLS.PeerCertificates[0].Raw, "served a certificate other than the one given")
}

// checkMetricsOverTLS populates the prometheus counters with a request for the
// prometheus format first (they are incremented only inside Prometheus.Output,
// and an untouched vec is not exposed), so the /metrics assertion does not
// depend on what an earlier test in the binary happened to do.
func checkMetricsOverTLS(t *testing.T, client *http.Client, url, metricsURL string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/vnd.syver-prometheus")
	resp, err := client.Do(req)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = client.Get(metricsURL) //nolint:noctx // test request
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "syver_tests_outcomes_total")
}

// TestServeOverTLS drives the real Serve on a real socket. Upstream's test
// wrapped the handler in httptest instead, so reverting the TLS listen branch
// left it green and /metrics was never requested.
func TestServeOverTLS(t *testing.T) {
	certPath, keyPath, cert := writeSelfSignedPair(t, false)
	c := newTLSServeConfig(t, certPath, keyPath)
	client := tlsClientFor(cert)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, done := startServe(t, ctx, c, "https", client)
	base := strings.TrimSuffix(url, c.Endpoint)

	// Revert-proof: change the TLS branch in Serve to srv.ListenAndServe();
	// the client handshake fails and startServe never sees it answer.
	t.Run("endpoint over https", func(t *testing.T) {
		checkHealthOverTLS(t, client, url, cert)
	})

	// Revert-proof: remove the /metrics mux.Handle (404).
	t.Run("metrics over https", func(t *testing.T) {
		checkMetricsOverTLS(t, client, url, base+"/metrics")
	})

	// A custom TLSClientConfig turns off the transport's HTTP/2 unless it is
	// forced back on.
	// Revert-proof: set TLSConfig.NextProtos = []string{"http/1.1"} in
	// serveTLSConfig (ProtoMajor becomes 1).
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	h2Transport := &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
	}
	defer h2Transport.CloseIdleConnections()
	h2Client := &http.Client{Timeout: 5 * time.Second, Transport: h2Transport}
	t.Run("HTTP/2 negotiated", func(t *testing.T) {
		resp, err := h2Client.Get(url) //nolint:noctx // test request
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, 2, resp.ProtoMajor, "an h2-capable client should get HTTP/2")
	})

	// Go answers plain HTTP on a TLS listener with a 400.
	// Revert-proof: as for "endpoint over https" (a plain listener answers 200).
	t.Run("plain http to the https port", func(t *testing.T) {
		plain := &http.Client{Timeout: 5 * time.Second}
		resp, err := plain.Get("http" + strings.TrimPrefix(url, "https")) //nolint:noctx // test request
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	// The h2 client still holds an idle connection here.
	// Revert-proof: remove the shutdown goroutine from Serve.
	cancel()
	select {
	case err := <-done:
		// Serve filters out ErrServerClosed itself; a clean shutdown is nil.
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Serve over TLS did not return within 15s of cancellation with an idle HTTP/2 connection open")
	}
}

// TestServeRefusesTLS11 needs both client bounds: a client with only
// MaxVersion below its own default minimum fails before it sends anything, so
// it would pass whatever the server did. The assertion is on the server's
// alert, not a client-side config error.
// Revert-proof: set the server's MinVersion to tls.VersionTLS10 in
// serveTLSConfig (the handshake then succeeds at TLS 1.1).
func TestServeRefusesTLS11(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("in FIPS mode the test client cannot offer TLS 1.0/1.1; the refusal is crypto/tls's own FIPS policy and is covered by the non-FIPS run")
	}
	certPath, keyPath, cert := writeSelfSignedPair(t, false)
	c := newTLSServeConfig(t, certPath, keyPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, done := startServe(t, ctx, c, "https", tlsClientFor(cert))

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", c.ListenAddress, &tls.Config{ //nolint:gosec // a TLS 1.0-1.1 client is the point of the test
		RootCAs:    pool,
		MinVersion: tls.VersionTLS10,
		MaxVersion: tls.VersionTLS11,
	})
	if err == nil {
		v := conn.ConnectionState().Version
		_ = conn.Close()
		t.Fatalf("handshake succeeded at version 0x%04x; serve must refuse anything below TLS 1.2", v)
	}
	assert.Contains(t, err.Error(), "remote error", "the refusal must come from the server: %v", err)
	assert.Contains(t, err.Error(), "protocol version", "want the server's protocol_version alert: %v", err)

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
}

// TestServeTLSErrorBeforeDiscovery: a bad TLS setting must stop serve before it
// reads the spec or runs discovery, which can execute commands. Serve is called
// directly, not through startServe, which would retry the error as a bind race.
// Revert-proof: move the serveTLSConfig call after newHealthHandler (the spec
// file error comes back instead).
func TestServeTLSErrorBeforeDiscovery(t *testing.T) {
	certPath, _, _ := writeSelfSignedPair(t, false)
	c, err := util.NewConfig(util.WithNoColor())
	require.NoError(t, err)
	c.Spec = filepath.Join(t.TempDir(), "does-not-exist.yaml")
	c.ServeCertFile = certPath
	c.ListenAddress = "127.0.0.1:0"

	err = Serve(t.Context(), c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "needs both --tls-cert and --tls-key")
}

// TestServeErrorLogLevels pins the levels http.Server's own messages get. They
// used to be unprefixed, and an unprefixed line passes the level filter at
// every -L, so a handshake error from each TCP-only probe would have printed
// continuously over HTTPS.
func TestServeErrorLogLevels(t *testing.T) {
	const (
		handshake = "http: TLS handshake error from 127.0.0.1:1: EOF"
		accept    = "http: Accept error: boom"
		panicLine = "http: panic serving 127.0.0.1:1: boom\ngoroutine 1 [running]:"
		// Go's HTTP/2 server logs its handler panics through the same ErrorLog.
		panicH2 = "http2: panic serving 127.0.0.1:1: boom\ngoroutine 2 [running]:"
	)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	emit := func(t *testing.T, level string) string {
		t.Helper()
		var buf bytes.Buffer
		// Order matters: newServeErrorLog captures log.Writer(), which is the
		// level filter only after setLogLevelTo has installed it.
		require.NoError(t, setLogLevelTo(&util.Config{LogLevel: level}, &buf))
		buf.Reset() // drop setLogLevel's own DEBUG line
		l := newServeErrorLog()
		l.Print(handshake)
		l.Print(accept)
		l.Print(panicLine)
		l.Print(panicH2)
		return buf.String()
	}

	// Revert-proof: make newServeErrorLog return an unprefixed logger (the
	// handshake line appears at INFO).
	t.Run("INFO", func(t *testing.T) {
		out := emit(t, "INFO")
		assert.NotContains(t, out, "TLS handshake error")
		assert.Contains(t, out, "[WARN] "+accept)
		assert.Contains(t, out, "[ERROR] http: panic serving")
		assert.Contains(t, out, "[ERROR] http2: panic serving")
	})

	// Revert-proof: tag panic lines [WARN] (it disappears). For the HTTP/2
	// row: match "http: panic serving" instead of "panic serving" (the http2
	// line is then [WARN] and hidden at ERROR).
	t.Run("ERROR", func(t *testing.T) {
		out := emit(t, "ERROR")
		assert.NotContains(t, out, "TLS handshake error")
		assert.NotContains(t, out, "Accept error")
		assert.Contains(t, out, "[ERROR] http: panic serving")
		assert.Contains(t, out, "goroutine 1 [running]:", "the stack must travel with the panic line")
		assert.Contains(t, out, "[ERROR] http2: panic serving", "an HTTP/2 handler panic must survive -L ERROR too")
		assert.Contains(t, out, "goroutine 2 [running]:")
	})

	// Revert-proof: tag handshake lines [TRACE].
	t.Run("DEBUG", func(t *testing.T) {
		out := emit(t, "DEBUG")
		assert.Contains(t, out, "[DEBUG] "+handshake)
		assert.Contains(t, out, "[WARN] "+accept)
		assert.Contains(t, out, "[ERROR] http: panic serving")
	})

	// Pins the construction-order rule for the next reader: a logger built
	// before setLogLevel writes wherever the standard logger pointed then, past
	// the filter. No revert-proof; it documents why Serve builds it after.
	t.Run("built before setLogLevel bypasses the filter", func(t *testing.T) {
		var before, buf bytes.Buffer
		log.SetOutput(&before)
		l := newServeErrorLog()
		require.NoError(t, setLogLevelTo(&util.Config{LogLevel: "ERROR"}, &buf))
		l.Print(accept)
		assert.NotContains(t, buf.String(), "Accept error")
		assert.Contains(t, before.String(), "[WARN] "+accept)
	})
}

// TestServePlainHTTPUnchanged: without the TLS flags serve is plain HTTP, with
// the same start-up line as before.
// Revert-proof: make the plain branch log "(https)" or call ListenAndServeTLS.
//
// Not t.Parallel(): it swaps the process-global os.Stderr (and so the standard
// logger's destination), which any concurrent test would write into or lose.
func TestServePlainHTTPUnchanged(t *testing.T) {
	// Serve's setLogLevel writes to os.Stderr as it is when called, so point
	// that at a file to read the start-up line back.
	logFile, err := os.Create(filepath.Join(t.TempDir(), "stderr.log"))
	require.NoError(t, err)
	prev := os.Stderr
	os.Stderr = logFile
	t.Cleanup(func() {
		os.Stderr = prev
		log.SetOutput(os.Stderr)
		_ = logFile.Close()
	})

	c := newTLSServeConfig(t, "", "")
	c.LogLevel = "INFO"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, done := startServe(t, ctx, c, "http", nil)

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(url) //nolint:noctx // test request
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, resp.TLS)

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
	os.Stderr = prev
	log.SetOutput(os.Stderr)
	logged, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(logged), "Starting to listen on:")
	assert.NotContains(t, string(logged), "(https)")
}

// TestServeErrorLogWiredThroughServe drives the real Serve: the levels
// TestServeErrorLogLevels pins only matter if Serve actually installs that
// logger, after setLogLevel. A bare TCP connect and a plain-HTTP request to the
// TLS port each make http.Server log a TLS handshake error.
// Revert-proof: delete `ErrorLog:` from srv in Serve (the unprefixed line
// appears at INFO and carries no [DEBUG]), or build it before setLogLevel (it
// then writes to the discarded pre-Serve output, so DEBUG finds nothing).
func TestServeErrorLogWiredThroughServe(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  bool
	}{
		{"INFO", false},
		{"DEBUG", true},
	} {
		t.Run(tc.level, func(t *testing.T) {
			readLog := stderrToFile(t)
			// Wherever the standard logger points before Serve's setLogLevel is
			// a sink, so a logger captured too early writes nowhere visible.
			log.SetOutput(io.Discard)

			certPath, keyPath, cert := writeSelfSignedPair(t, false)
			c := newTLSServeConfig(t, certPath, keyPath)
			c.LogLevel = tc.level
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			url, done := startServe(t, ctx, c, "https", tlsClientFor(cert))

			conn, err := net.DialTimeout("tcp", c.ListenAddress, 5*time.Second)
			require.NoError(t, err)
			_ = conn.Close()
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Get("http" + strings.TrimPrefix(url, "https")) //nolint:noctx // test request
			require.NoError(t, err)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)

			// The server logs each handshake error from its connection
			// goroutine, so give it time to arrive. In the INFO case this is
			// the window in which a wrongly levelled line would show up.
			const marker = "[DEBUG] http: TLS handshake error"
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) && !strings.Contains(readLog(), marker) {
				time.Sleep(50 * time.Millisecond)
			}
			out := readLog()
			if tc.want {
				assert.Contains(t, out, marker)
			} else {
				assert.NotContains(t, out, "TLS handshake error", "a handshake error must not print at -L INFO")
			}

			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(15 * time.Second):
				t.Fatal("Serve did not return after cancellation")
			}
		})
	}
}

// TestServeLogsCertificateAtStartup: serve says which certificate it presents
// and until when, and warns, without refusing to start, when the certificate is
// outside its validity window, since every client will then fail a handshake
// that is logged only at DEBUG.
func TestServeLogsCertificateAtStartup(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name                string
		notBefore, notAfter time.Time
		wantWarn            string
	}{
		// Revert-proof: remove the logServeCertificate call from serveTLSConfig.
		{"valid", now.Add(-time.Hour), now.Add(time.Hour), ""},
		// Revert-proof: remove the expired case from logServeCertificate.
		{"expired", now.Add(-2 * time.Hour), now.Add(-time.Hour), "[WARN] The tls certificate expired at"},
		// Revert-proof: remove the not-yet-valid case from logServeCertificate.
		{"not yet valid", now.Add(time.Hour), now.Add(2 * time.Hour), "[WARN] The tls certificate is not valid until"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readLog := stderrToFile(t)
			certPath, keyPath, cert := writePairValidBetween(t, false, tc.notBefore, tc.notAfter, "localhost")
			c := newTLSServeConfig(t, certPath, keyPath)
			c.LogLevel = "INFO"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// startServe returning at all means serve started and answered.
			_, done := startServe(t, ctx, c, "https", insecureReadinessClient())

			out := readLog()
			// Revert-proof: drop "(https)" from the TLS start-up line in Serve.
			assert.Contains(t, out, "Starting to listen on: "+c.ListenAddress+" (https)")
			// Asserted on the certificate line itself: "127.0.0.1" alone is
			// also in the "Starting to listen on" line.
			// Revert-proof: drop the DNS names, or the IP addresses, from the
			// log line in logServeCertificate.
			var certLine string
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, "Serving https with certificate") {
					certLine = l
				}
			}
			require.NotEmpty(t, certLine, "no certificate line in:\n%s", out)
			assert.Contains(t, certLine, "[INFO] Serving https with certificate subject")
			assert.Contains(t, certLine, `subject "CN=syver serve test"`)
			assert.Contains(t, certLine, `DNS names ["localhost"]`)
			assert.Contains(t, certLine, "IP addresses [127.0.0.1 ::1]")
			assert.Contains(t, certLine, "valid until "+cert.NotAfter.UTC().Format(time.RFC3339))
			if tc.wantWarn == "" {
				assert.NotContains(t, out, "[WARN] The tls certificate")
			} else {
				assert.Contains(t, out, tc.wantWarn)
			}

			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(15 * time.Second):
				t.Fatal("Serve did not return after cancellation")
			}
		})
	}
}

// TestServeCertificateLogQuotesDNSNames: the x509 parser accepts a newline in
// a DNS SAN, so a certificate could carry a name that, printed raw, forges a
// second log line. The name must be quoted, keeping the log to one line.
// Revert-proof: log the DNS names with %v instead of %q.
func TestServeCertificateLogQuotesDNSNames(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	const forged = "evil\n2026-01-01T00:00:00Z [ERROR] forged line"
	now := time.Now()
	certPath, keyPath, _ := writePairValidBetween(t, false, now.Add(-time.Hour), now.Add(time.Hour), forged)

	var buf bytes.Buffer
	require.NoError(t, setLogLevelTo(&util.Config{LogLevel: "INFO"}, &buf))
	buf.Reset()
	cfg, err := serveTLSConfig(&util.Config{ServeCertFile: certPath, ServeKeyFile: keyPath})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, []string{forged}, cfg.Certificates[0].Leaf.DNSNames, "the parser must have kept the newline for this test to mean anything")

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "\n"), "the certificate log must be exactly one line:\n%s", out)
	assert.Contains(t, out, `"evil\n2026-01-01T00:00:00Z [ERROR] forged line"`)
}

// h2Frame writes one HTTP/2 frame: 24-bit length, type, flags, stream id.
func h2Frame(t *testing.T, w io.Writer, typ, flags byte, stream uint32, payload []byte) {
	t.Helper()
	hdr := []byte{
		byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload)), //nolint:gosec // truncation to the 24-bit wire field is the encoding
		typ, flags,
		byte(stream >> 24), byte(stream >> 16), byte(stream >> 8), byte(stream), //nolint:gosec // truncation to the 32-bit wire field is the encoding
	}
	_, err := w.Write(append(hdr, payload...))
	require.NoError(t, err)
}

// hpackLiteral encodes a header as "literal without indexing" with an indexed
// name from the HPACK static table and a plain (not Huffman) value. Values
// here are short, so a one-byte length is enough.
func hpackLiteral(nameIndex byte, value string) []byte {
	return append([]byte{nameIndex, byte(len(value))}, value...) //nolint:gosec // values here are under 127 bytes
}

// TestServeBoundsAStalledHTTP2Write: an HTTP/2 client that advertises a zero
// flow-control window lets the server send the response headers but never a
// byte of body, so without a write deadline the handler goroutine blocks
// forever. golang.org/x/net/http2 is not a direct dependency, so the client
// here is a few hand-written frames over crypto/tls: the connection preface,
// SETTINGS with INITIAL_WINDOW_SIZE=0, and one GET. The write deadline is
// shortened so the test is quick; the server must then reset the stream.
// Revert-proof: remove the SetWriteDeadline line from ServeHTTP (no
// RST_STREAM arrives within the bound).
//
// It also checks the cut-off write is logged as a failure, at DEBUG, and not
// as "status 200".
// Revert-proof: ignore the write and flush errors again (the failure line is
// missing and the success line appears).
func TestServeBoundsAStalledHTTP2Write(t *testing.T) {
	readLog := stderrToFile(t)
	prevTimeout := serveResponseWriteTimeout
	serveResponseWriteTimeout = 200 * time.Millisecond

	certPath, keyPath, cert := writeSelfSignedPair(t, false)
	c := newTLSServeConfig(t, certPath, keyPath)
	c.LogLevel = "DEBUG"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, done := startServe(t, ctx, c, "https", tlsClientFor(cert))
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("Serve did not return after cancellation")
		}
		serveResponseWriteTimeout = prevTimeout
	})

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", c.ListenAddress, &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2"},
	})
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.Equal(t, "h2", conn.ConnectionState().NegotiatedProtocol)

	const (
		frameHeaders   = 0x1
		frameRSTStream = 0x3
		frameSettings  = 0x4
		frameGoAway    = 0x7
		flagEndStream  = 0x1
		flagAck        = 0x1
		flagEndHeader  = 0x4
	)
	_, err = io.WriteString(conn, "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	require.NoError(t, err)
	// SETTINGS_INITIAL_WINDOW_SIZE (0x4) = 0: no stream may send DATA.
	h2Frame(t, conn, frameSettings, 0, 0, []byte{0x00, 0x04, 0, 0, 0, 0})
	block := []byte{0x82, 0x87}                                   // :method GET, :scheme https (static table)
	block = append(block, hpackLiteral(0x04, c.Endpoint)...)      // :path
	block = append(block, hpackLiteral(0x01, c.ListenAddress)...) // :authority
	h2Frame(t, conn, frameHeaders, flagEndStream|flagEndHeader, 1, block)

	// Read frames until stream 1 is reset. The bound is generous against the
	// shortened deadline; without the deadline the reset never comes.
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	var sawHeaders bool
	hdr := make([]byte, 9)
	for {
		_, err := io.ReadFull(conn, hdr)
		require.NoError(t, err, "no RST_STREAM for the stalled stream: the response write is not bounded")
		length := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
		typ, flags := hdr[3], hdr[4]
		stream := uint32(hdr[5]&0x7f)<<24 | uint32(hdr[6])<<16 | uint32(hdr[7])<<8 | uint32(hdr[8])
		payload := make([]byte, length)
		_, err = io.ReadFull(conn, payload)
		require.NoError(t, err)
		switch {
		case typ == frameSettings && flags&flagAck == 0:
			h2Frame(t, conn, frameSettings, flagAck, 0, nil)
		case typ == frameHeaders && stream == 1:
			sawHeaders = true
		case typ == frameGoAway:
			t.Fatalf("server sent GOAWAY instead of resetting the stream: % x", payload)
		case typ == frameRSTStream && stream == 1:
			assert.True(t, sawHeaders, "the request should have been answered (headers sent) before the body stalled")
			// The server sees this connection's local address as RemoteAddr.
			addr := conn.LocalAddr().String()
			failed := addr + ": writing response failed (status 200)"
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) && !strings.Contains(readLog(), failed) {
				time.Sleep(50 * time.Millisecond)
			}
			out := readLog()
			assert.Contains(t, out, "[DEBUG] "+failed)
			assert.NotContains(t, out, addr+": status 200", "a response the deadline cut off must not be logged as served")
			return
		}
	}
}

// TestServeWriteDeadlineDoesNotLimitTheChecks: the response-write deadline is
// set only after the checks have run. Set before them, a suite slower than the
// deadline would have its own response cut off. The slow check is an http
// check against a local server that waits before answering, so the test runs
// the same on every platform (no shell sleep command needed). The cache is
// off so the request under test runs the checks itself.
// Revert-proof: move the SetWriteDeadline line in ServeHTTP to before
// processAndEnsureCached.
func TestServeWriteDeadlineDoesNotLimitTheChecks(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	spec := filepath.Join(t.TempDir(), "slow.yaml")
	require.NoError(t, os.WriteFile(spec, []byte("http:\n  "+slow.URL+"/slow:\n    status: 200\n    timeout: 5000\n"), 0o600))

	prevTimeout := serveResponseWriteTimeout
	serveResponseWriteTimeout = 200 * time.Millisecond
	c, err := util.NewConfig(util.WithSpecFile(spec), util.WithOutputFormat("json"), util.WithNoColor())
	require.NoError(t, err)
	c.Endpoint = "/healthz"
	c.Cache = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The readiness client's timeout must outlast the slow check.
	url, done := startServe(t, ctx, c, "http", &http.Client{Timeout: 5 * time.Second})
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("Serve did not return after cancellation")
		}
		serveResponseWriteTimeout = prevTimeout
	})

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Get(url) //nolint:noctx // test request
	require.NoError(t, err, "the response was cut off: the write deadline is limiting the checks")
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "the body was cut off: the write deadline is limiting the checks")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var parsed struct {
		Summary struct {
			TestCount   int `json:"test-count"`
			FailedCount int `json:"failed-count"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed), "incomplete body: %s", body)
	assert.Equal(t, 1, parsed.Summary.TestCount)
	assert.Equal(t, 0, parsed.Summary.FailedCount)
}

// TestServeOverTLSUnderFIPS is a one-off proof, not a regression guard: it
// skips unless the binary runs in Go's FIPS 140 mode, which CI and host-gate
// do not set. Run it with
//
//	GODEBUG=fips140=on go test -race -v -run 'TestServeOverTLS' ./internal/syver/
//
// and check it reports PASS, not SKIP; a SKIP there means the mode was not
// active. It proves Go's FIPS mode is on and serve still handshakes under it,
// and nothing about a real FIPS host.
func TestServeOverTLSUnderFIPS(t *testing.T) {
	if !fips140.Enabled() {
		t.Skip("run with GODEBUG=fips140=on")
	}
	certPath, keyPath, cert := writeSelfSignedPair(t, false)
	c := newTLSServeConfig(t, certPath, keyPath)
	client := tlsClientFor(cert)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url, done := startServe(t, ctx, c, "https", client)
	checkHealthOverTLS(t, client, url, cert)
	checkMetricsOverTLS(t, client, url, strings.TrimSuffix(url, c.Endpoint)+"/metrics")

	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
}
