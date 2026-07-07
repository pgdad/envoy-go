package httpclient_test

// httpclient_test.go — Group 6 unit tests per phase-20 SPEC §14.1 + ADR-0177.
//
// Coverage roster (planner-time D5 + SPEC §14.1 Group 6):
//   - Options zero-value: no timeout, no retries, nil TLSConfig — pass-through
//   - Client.Do happy-path 200 OK
//   - Zero-retry default: single attempt even on 5xx
//   - Retry envelope: status-driven retry; attempt count == Attempts + 1
//   - Context cancellation mid-Do via context.WithTimeout(1ms) → DeadlineExceeded
//   - TLSConfig wired through (httptest.NewTLSServer + InsecureSkipVerify)
//   - Request-error propagation
//   - New(zero Options) constructs a usable Client (no panic)
//   - Retry honors context cancellation between attempts (no retry-after-cancel)
//   - Non-retryable status passes through on first hit
//
// Implements the SPEC §3.1 + ADR-0177 §Context public surface:
//   Options{Timeout, RetryPolicy, TLSConfig}
//   RetryPolicy{Attempts, PerAttemptDelay, RetryOnStatus}
//   Client; New(opts); (*Client).Do(req)

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// Group 6 — internal/httpclient tests
// ----------------------------------------------------------------------------

// TestOptions_ZeroValue_NoOpDefaults verifies that the zero-Options Client has
// no client-imposed timeout, no retries, and accepts nil TLSConfig (delegating
// to the stdlib's default crypto/tls posture). The Client must construct
// without panic.
func TestOptions_ZeroValue_NoOpDefaults(t *testing.T) {
	t.Parallel()
	c := httpclient.New(httpclient.Options{})
	if c == nil {
		t.Fatalf("New(zero Options): want non-nil Client")
	}
	// A trivial GET against an httptest server should succeed without timeout
	// triggering.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(zero Options): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
}

// TestDo_HappyPath_200 verifies that a basic GET against a 200-OK server
// returns the response body verbatim.
func TestDo_HappyPath_200(t *testing.T) {
	t.Parallel()
	const want = "hello, httpclient"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, want)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != want {
		t.Errorf("body: want %q, got %q", want, string(body))
	}
}

// TestZeroRetry_Default_SingleAttempt verifies that with the zero-Options
// (no RetryPolicy), a 5xx response from the server is RETURNED VERBATIM as
// the response (no retry attempted; single attempt only). Per SPEC §20.P1
// RATIFIED — upstream Envoy v1.37.2 wire default is zero retry.
func TestZeroRetry_Default_SingleAttempt(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts: want 1 (zero-retry default), got %d", got)
	}
}

// TestRetry_StatusDriven_AttemptCount verifies that with a non-zero
// RetryPolicy{Attempts: 2, RetryOnStatus: [500, 502, 503, 504]}, a server
// returning 503 every time yields exactly Attempts+1 == 3 total attempts.
func TestRetry_StatusDriven_AttemptCount(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        2, // 2 retries → 3 total attempts
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{500, 502, 503, 504},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts: want 3 (Attempts=2 ⇒ Attempts+1 total), got %d", got)
	}
}

// TestRetry_NonRetryableStatus_NoRetry verifies that a status NOT in
// RetryOnStatus is RETURNED VERBATIM on first hit (no retry).
func TestRetry_NonRetryableStatus_NoRetry(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusNotFound) // 404 — NOT in RetryOnStatus
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        3,
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{500, 502, 503, 504},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts: want 1 (non-retryable status), got %d", got)
	}
}

// TestRetry_SucceedsOnRetry verifies that a server returning 503 once then
// 200 yields a 200 response after exactly 2 attempts.
func TestRetry_SucceedsOnRetry(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        2,
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{503},
		},
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts: want 2, got %d", got)
	}
}

// TestCtxCancellation_MidDo_ReturnsDeadlineExceeded verifies that an in-flight
// request whose context expires returns context.DeadlineExceeded (or a wrapped
// equivalent) and does not hang.
func TestCtxCancellation_MidDo_ReturnsDeadlineExceeded(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the per-request deadline so the request times out.
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Client canceled — best-effort cleanup.
			return
		}
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 0}) // no Client-level timeout; rely on req ctx
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("Do: want error from ctx cancellation, got nil")
	}
	// Accept either DeadlineExceeded directly or an error whose chain contains it.
	if !errors.Is(err, context.DeadlineExceeded) {
		// net/http often wraps it in a *url.Error whose Unwrap returns the
		// ctx error; errors.Is should unwrap. If not, accept the timeout text
		// match as a fallback (some Go versions surface i/o timeout).
		if !strings.Contains(err.Error(), "deadline exceeded") &&
			!strings.Contains(err.Error(), "context canceled") &&
			!strings.Contains(err.Error(), "Client.Timeout") {
			t.Errorf("err: want DeadlineExceeded-equivalent, got %v", err)
		}
	}
}

// TestRetry_HonorsCtxCancellation_BetweenAttempts verifies that an already-
// canceled context short-circuits the inter-attempt sleep — the loop does NOT
// continue retrying after the context is done.
func TestRetry_HonorsCtxCancellation_BetweenAttempts(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{
		Timeout: 0,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        5,
			PerAttemptDelay: 200 * time.Millisecond, // long enough that ctx expires first
			RetryOnStatus:   []int{503},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = c.Do(req)
	if err == nil {
		t.Fatal("Do: want error from ctx cancellation, got nil")
	}
	// Must have made at most 1 full attempt (the first request succeeds
	// before ctx expires; the inter-attempt sleep then trips the ctx check).
	if got := atomic.LoadInt32(&attempts); got > 1 {
		t.Errorf("attempts: want <=1 (ctx cancels inter-attempt sleep), got %d", got)
	}
}

// TestTLSConfig_WiredThrough verifies that a *tls.Config supplied via Options
// is actually applied to the underlying http.Transport — the test starts an
// httptest.NewTLSServer (self-signed cert) and uses InsecureSkipVerify=true
// to validate the TLS path is exercised end-to-end.
func TestTLSConfig_WiredThrough(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "tls-ok")
	}))
	defer srv.Close()

	// 1. Without TLSConfig: the request should FAIL (self-signed cert; default
	//    posture rejects).
	cReject := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req1, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp1, err := cReject.Do(req1)
	if err == nil {
		_ = resp1.Body.Close()
		t.Fatal("Do(no TLSConfig): want cert-verification error, got success")
	}

	// 2. With InsecureSkipVerify TLSConfig: the request should SUCCEED.
	cAccept := httpclient.New(httpclient.Options{
		Timeout:   5 * time.Second,
		TLSConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — test-only
	})
	req2, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp2, err := cAccept.Do(req2)
	if err != nil {
		t.Fatalf("Do(InsecureSkipVerify): %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp2.StatusCode)
	}
	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "tls-ok" {
		t.Errorf("body: want %q, got %q", "tls-ok", string(body))
	}
}

// TestDo_RequestError_Propagated verifies that a transport-level error (e.g.,
// connection refused against a closed server) propagates as an error from Do
// without panicking.
func TestDo_RequestError_Propagated(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := srv.URL
	srv.Close() // immediately close so connection is refused

	c := httpclient.New(httpclient.Options{Timeout: 1 * time.Second})
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.Do(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("Do(closed server): want transport error, got nil")
	}
}

// TestDo_PostBody verifies that a POST request with a body is sent and the
// body is observable on the server side. Anchors the synchronous-per-request
// POST shape that oauth2 token_endpoint will consume at Task 10.
func TestDo_PostBody(t *testing.T) {
	t.Parallel()
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	const payload = "grant_type=authorization_code&code=abc"
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	if string(got) != payload {
		t.Errorf("body: want %q, got %q", payload, string(got))
	}
}

// ----------------------------------------------------------------------------
// Phase 22.2 Task 4 — ClusterDispatch tests per SPEC §3.3 + §11.4 (IN-PLACE
// AMENDMENT on ADR-0177). The 6 test functions below exercise the NEW
// ClusterDispatch method covering cluster lookup, endpoint resolution, per-
// cluster TLS, retry inheritance from receiver's Options, context-driven
// timeout, and explicit context cancellation.
// ----------------------------------------------------------------------------

// splitHostPort parses a "host:port" into (host, portUint32) for plumbing into
// the cluster manager bootstrap proto.
func splitHostPort(t *testing.T, addr string) (string, uint32) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	return host, uint32(p)
}

// mkPlainClusterMgr builds a *cluster.Manager containing a single plaintext
// STATIC cluster `name` pointing at host:port. Mirrors the established pattern
// used in internal/grpcclient/grpcclient_test.go and internal/filter/http/
// extauthz/extauthz_test.go.
func mkPlainClusterMgr(t *testing.T, name, host string, port uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// httpclientTestPKI is a minimal in-memory CA + leaf keypair for TLS tests.
// Mirrors internal/grpcclient/grpcclient_test.go authTestPKI.
type httpclientTestPKI struct {
	caPEM       []byte
	caPool      *x509.CertPool
	leafCertPEM []byte
	leafKeyPEM  []byte
	serverCert  tls.Certificate // ready-to-serve for httptest.Server.TLS
}

// mkHTTPClientTestPKI creates a fresh in-memory CA + leaf keypair signed by
// the CA. The leaf is valid for "alpha.envoy-go.test" SNI and 127.0.0.1.
func mkHTTPClientTestPKI(t *testing.T) *httpclientTestPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "httpclient test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "alpha.envoy-go.test"},
		DNSNames:     []string{"alpha.envoy-go.test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatalf("leaf key marshal: %v", err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	serverCert, err := tls.X509KeyPair(leafCertPEM, leafKeyPEM)
	if err != nil {
		t.Fatalf("server cert: %v", err)
	}
	return &httpclientTestPKI{
		caPEM:       caPEM,
		caPool:      pool,
		leafCertPEM: leafCertPEM,
		leafKeyPEM:  leafKeyPEM,
		serverCert:  serverCert,
	}
}

// mkTLSClusterMgr builds a *cluster.Manager containing a single plaintext-on-
// the-wire-but-TLS-on-the-cluster STATIC cluster `name` pointing at host:port,
// with `validation_context.trusted_ca` set to pki.caPEM and SNI
// "alpha.envoy-go.test".
func mkTLSClusterMgr(t *testing.T, pki *httpclientTestPKI, name, host string, port uint32) *cluster.Manager {
	t.Helper()
	ctx := &tlsv3.UpstreamTlsContext{
		Sni: "alpha.envoy-go.test",
		CommonTlsContext: &tlsv3.CommonTlsContext{
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.caPEM},
					},
				},
			},
		},
	}
	tsAny, err := anypb.New(ctx)
	if err != nil {
		t.Fatalf("anypb.New(UpstreamTlsContext): %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
				TransportSocket: &corev3.TransportSocket{
					Name:       "envoy.transport_sockets.tls",
					ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: tsAny},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(tls): %v", err)
	}
	return cm
}

// TestClient_ClusterDispatch_cluster_not_found_returns_error verifies that
// ClusterDispatch returns an error when the named cluster is absent from the
// manager. The error MUST be non-nil and the returned *http.Response MUST be
// nil; no upstream dial is attempted.
func TestClient_ClusterDispatch_cluster_not_found_returns_error(t *testing.T) {
	t.Parallel()
	// Build a manager with cluster "exists" — we'll look up "missing".
	cm := mkPlainClusterMgr(t, "exists", "127.0.0.1", 1)

	c := httpclient.New(httpclient.Options{Timeout: time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(req.Context(), "missing", req, cm)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("ClusterDispatch(missing): want error, got nil")
	}
	if resp != nil {
		t.Errorf("ClusterDispatch(missing): want nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "missing") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("err: want substring 'missing' or 'not found', got %q", err.Error())
	}
}

// TestClient_ClusterDispatch_endpoint_resolution_success exercises the happy
// path: a real cluster pointing at an httptest.Server; ClusterDispatch resolves
// the endpoint and round-trips the request. Asserts the response body verbatim.
func TestClient_ClusterDispatch_endpoint_resolution_success(t *testing.T) {
	t.Parallel()
	const wantBody = "cluster-dispatch-ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkPlainClusterMgr(t, "c_happy", host, port)

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	// URL.Host is rewritten by ClusterDispatch to the picked endpoint's
	// host:port. We use a placeholder host here on purpose to verify rewrite.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder.invalid/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(req.Context(), "c_happy", req, cm)
	if err != nil {
		t.Fatalf("ClusterDispatch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != wantBody {
		t.Errorf("body: want %q, got %q", wantBody, string(body))
	}
}

// TestClient_ClusterDispatch_per_cluster_TLS_honored verifies that the per-
// cluster TLS config supplied via cluster.UpstreamTLSConfig() is honored at
// dispatch time. The TLS upstream uses a cert signed by the cluster's
// trusted_ca; the receiver's Options.TLSConfig is NIL so a default-TLS path
// would reject the leaf cert (no trust root); a SUCCESS therefore proves the
// per-cluster TLS config is the one that was applied.
func TestClient_ClusterDispatch_per_cluster_TLS_honored(t *testing.T) {
	t.Parallel()
	pki := mkHTTPClientTestPKI(t)

	const wantBody = "tls-cluster-ok"
	// Start a TLS server using OUR test CA's leaf cert; configure SNI hostname
	// "alpha.envoy-go.test" matching the cluster's SNI.
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pki.serverCert}}
	srv.StartTLS()
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkTLSClusterMgr(t, pki, "c_tls", host, port)

	// Receiver has NO TLS config — only the cluster's TLS config is in play.
	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(req.Context(), "c_tls", req, cm)
	if err != nil {
		t.Fatalf("ClusterDispatch(tls): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != wantBody {
		t.Errorf("body: want %q, got %q", wantBody, string(body))
	}

	// Negative-control: a Client with NO TLS config dispatching to the SAME
	// TLS server via a PLAINTEXT cluster MUST NOT return wantBody — the
	// server either rejects (TLS handshake error → dispatch err) OR replies
	// with the stdlib's "client sent an HTTP request to an HTTPS server"
	// 400-class response. Either outcome refutes the alternative hypothesis
	// that the TLS path was unused above.
	cmPlain := mkPlainClusterMgr(t, "c_plain", host, port)
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder/x", nil)
	resp2, err := c.ClusterDispatch(req2.Context(), "c_plain", req2, cmPlain)
	if err == nil {
		defer func() { _ = resp2.Body.Close() }()
		body2, _ := io.ReadAll(resp2.Body)
		if resp2.StatusCode == http.StatusOK && string(body2) == wantBody {
			t.Fatalf("negative-control: plaintext cluster against TLS server unexpectedly returned 200 OK with wantBody (proves TLS path is not honoring per-cluster config)")
		}
	}
}

// TestClient_ClusterDispatch_retry_inherits_Options verifies that
// ClusterDispatch's retry loop honors the receiver's Options.RetryPolicy —
// specifically that a status-driven retry on RetryOnStatus drives Attempts+1
// total attempts. Mirrors TestRetry_StatusDriven_AttemptCount for the Do path.
func TestClient_ClusterDispatch_retry_inherits_Options(t *testing.T) {
	t.Parallel()
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkPlainClusterMgr(t, "c_retry", host, port)

	c := httpclient.New(httpclient.Options{
		Timeout: 5 * time.Second,
		RetryPolicy: httpclient.RetryPolicy{
			Attempts:        2, // 2 retries → 3 total attempts
			PerAttemptDelay: 1 * time.Millisecond,
			RetryOnStatus:   []int{503},
		},
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(req.Context(), "c_retry", req, cm)
	if err != nil {
		t.Fatalf("ClusterDispatch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts: want 3 (Attempts=2 ⇒ Attempts+1 total), got %d", got)
	}
}

// TestClient_ClusterDispatch_timeout_via_context verifies that an in-flight
// ClusterDispatch whose request context expires mid-call returns a non-nil
// error and does not hang. Mirrors TestCtxCancellation_MidDo_ReturnsDeadlineExceeded
// for the Do path.
func TestClient_ClusterDispatch_timeout_via_context(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the request ctx deadline.
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkPlainClusterMgr(t, "c_timeout", host, port)

	c := httpclient.New(httpclient.Options{Timeout: 0}) // rely on req ctx
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://placeholder/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(ctx, "c_timeout", req, cm)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("ClusterDispatch: want error from ctx timeout, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		if !strings.Contains(err.Error(), "deadline exceeded") &&
			!strings.Contains(err.Error(), "context canceled") &&
			!strings.Contains(err.Error(), "Client.Timeout") {
			t.Errorf("err: want DeadlineExceeded-equivalent, got %v", err)
		}
	}
}

// TestClient_ClusterDispatch_context_cancellation_propagates verifies that
// context.WithCancel() + cancel() prior to the call surfaces the cancellation
// (no upstream dial succeeds; the error chain unwraps to context.Canceled).
func TestClient_ClusterDispatch_context_cancellation_propagates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkPlainClusterMgr(t, "c_cancel", host, port)

	c := httpclient.New(httpclient.Options{Timeout: 0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-canceled before dispatch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://placeholder/x", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := c.ClusterDispatch(ctx, "c_cancel", req, cm)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("ClusterDispatch: want error from canceled ctx, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("err: want context.Canceled-equivalent, got %v", err)
		}
	}
}

// TestClient_ClusterDispatch_does_not_mutate_caller_request verifies the
// documented "don't mutate the caller's request state in-place" contract:
// the LB-endpoint URL rewrite must land on ClusterDispatch's internal shallow
// copy (created by WithContext BEFORE the rewrite), leaving the caller's
// *http.Request — URL pointer, Host, Scheme — untouched so the request can be
// reused across calls. Before the fix, request.URL was overwritten before the
// WithContext copy, permanently pointing the caller's request at the picked
// endpoint.
func TestClient_ClusterDispatch_does_not_mutate_caller_request(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	host, port := splitHostPort(t, srv.Listener.Addr().String())
	cm := mkPlainClusterMgr(t, "c_nomut", host, port)

	c := httpclient.New(httpclient.Options{Timeout: 5 * time.Second})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://placeholder.invalid/x?q=1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	origURL := req.URL

	resp, err := c.ClusterDispatch(req.Context(), "c_nomut", req, cm)
	if err != nil {
		t.Fatalf("ClusterDispatch: %v", err)
	}
	_ = resp.Body.Close()

	if req.URL != origURL {
		t.Errorf("caller req.URL pointer was replaced (got %p, want %p)", req.URL, origURL)
	}
	if got := req.URL.Host; got != "placeholder.invalid" {
		t.Errorf("caller req.URL.Host = %q, want %q (must not observe the LB-picked endpoint)", got, "placeholder.invalid")
	}
	if got := req.URL.Scheme; got != "http" {
		t.Errorf("caller req.URL.Scheme = %q, want %q", got, "http")
	}

	// The caller's request must be reusable verbatim for a second dispatch.
	resp2, err := c.ClusterDispatch(req.Context(), "c_nomut", req, cm)
	if err != nil {
		t.Fatalf("second ClusterDispatch on the reused request: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("second dispatch status: want 200, got %d", resp2.StatusCode)
	}
}
