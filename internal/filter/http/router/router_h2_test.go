package router

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	"github.com/pgdad/envoy-go/internal/stats"
)

// ---------------------------------------------------------------------------
// Phase 05.2 — routerActionH2 tests
// ---------------------------------------------------------------------------

// h2BackendPKI is the in-memory CA + leaf cert/key for the in-process H2
// backend tests. The mkH2BackendPKI helper mirrors the cluster-package's
// dial_h2_test.go pattern (P-256 keygen is cheap; per-test instead of
// package-level reduces parallel-test contention).
type h2BackendPKI struct {
	caPool      *x509.CertPool
	caPEM       []byte
	leafCertPEM []byte
	leafKeyPEM  []byte
}

func mkH2BackendPKI(t *testing.T) *h2BackendPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "envoy-go h2 backend test CA"},
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
	return &h2BackendPKI{caPool: pool, caPEM: caPEM, leafCertPEM: leafCertPEM, leafKeyPEM: leafKeyPEM}
}

// h2BackendBehavior controls what the in-process H2 backend does after it
// reads the client's HEADERS frame. Each test selects exactly one behavior.
type h2BackendBehavior int

const (
	// h2BackendOK responds with HEADERS(:status 200, content-type, content-length) + DATA(body) + END_STREAM.
	h2BackendOK h2BackendBehavior = iota
	// h2Backend503 responds with HEADERS(:status 503, ...) + DATA(body) + END_STREAM.
	h2Backend503
	// h2BackendMalformed writes a syntactically invalid HEADERS payload
	// (raw garbage bytes inside the frame) so RoundTrip surfaces a
	// COMPRESSION_ERROR-class protocol error.
	h2BackendMalformed
	// h2BackendHang reads the HEADERS but never responds; useful for the
	// ctx-cancel test (the client side cancels the ctx).
	h2BackendHang
)

// runH2Backend handles one connection: client preface + SETTINGS exchange,
// then waits for HEADERS, then dispatches per behavior. Returns when the
// client closes the conn or the test goroutine ends.
func runH2Backend(conn net.Conn, behavior h2BackendBehavior, body []byte) {
	defer func() { _ = conn.Close() }()
	// Read client preface.
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return
	}
	if string(prefaceBuf) != "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		return
	}
	fr := http2.NewFramer(conn, conn)
	// Read client SETTINGS.
	frame, err := fr.ReadFrame()
	if err != nil {
		return
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return
	}
	// Write server initial SETTINGS.
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return
	}
	// Read client's SETTINGS_ACK first to avoid synchronous-write deadlock.
	if _, err := fr.ReadFrame(); err != nil {
		return
	}
	// Write SETTINGS_ACK for client's SETTINGS.
	if err := fr.WriteSettingsAck(); err != nil {
		return
	}
	// Read the next frame — should be HEADERS from the client. The client may
	// also send WINDOW_UPDATE first; loop past those.
	var streamID uint32
	for {
		frame, err = fr.ReadFrame()
		if err != nil {
			return
		}
		if hf, ok := frame.(*http2.HeadersFrame); ok {
			streamID = hf.StreamID
			break
		}
		// Ignore WINDOW_UPDATE / PING etc.
	}
	switch behavior {
	case h2BackendHang:
		// Drain inbound bytes; never respond. The client will cancel ctx.
		_, _ = io.Copy(io.Discard, conn)
		return
	case h2BackendMalformed:
		// Write a HEADERS frame with garbage in the header block so the client's
		// hpack decoder errors out (COMPRESSION_ERROR).
		_ = fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			EndStream:     true,
			EndHeaders:    true,
		})
		// Drain rest.
		_, _ = io.Copy(io.Discard, conn)
		return
	case h2BackendOK, h2Backend503:
		status := "200"
		if behavior == h2Backend503 {
			status = "503"
		}
		// 43.2a Task 7: serve MULTIPLE sequential streams on this connection (the
		// pooled-conn reuse path; ADR-0253 supersedes the ADR-0056 one-stream-per-
		// conn assumption). The HPACK encoder is connection-scoped (its dynamic
		// table persists across streams), so it is allocated once outside the loop.
		// streamID is the HEADERS id of the current request (the first one was
		// already read above); after responding, loop to read the next request's
		// HEADERS (or return on conn close / GOAWAY).
		var hbuf bytes.Buffer
		henc := hpack.NewEncoder(&hbuf)
		for {
			hbuf.Reset()
			_ = henc.WriteField(hpack.HeaderField{Name: ":status", Value: status})
			_ = henc.WriteField(hpack.HeaderField{Name: "content-type", Value: "text/plain"})
			_ = henc.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(body))})
			if err := fr.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: hbuf.Bytes(),
				EndStream:     false,
				EndHeaders:    true,
			}); err != nil {
				return
			}
			if err := fr.WriteData(streamID, true /* endStream */, body); err != nil {
				return
			}
			// Read the next client HEADERS (a reused pooled conn sends another
			// stream). Ignore non-HEADERS frames (WINDOW_UPDATE / SETTINGS / PING /
			// GOAWAY-without-HEADERS); return on conn close / read error.
			for {
				frame, err = fr.ReadFrame()
				if err != nil {
					return // conn closed (client Close → GOAWAY) or read error
				}
				if hf, ok := frame.(*http2.HeadersFrame); ok {
					streamID = hf.StreamID
					break
				}
			}
		}
	}
}

// startH2Backend listens on a fresh TLS port with NextProtos=["h2"] and runs
// runH2Backend for each accepted conn. Returns the listener (caller defers
// Close) — the test gets the address via ln.Addr().
func startH2Backend(t *testing.T, pki *h2BackendPKI, behavior h2BackendBehavior, body []byte) net.Listener {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go runH2Backend(c, behavior, body)
		}
	}()
	return ln
}

// h2RecordingBackend is the H2 analog of scriptedBackend: a controllable
// in-process H2 backend that records the request body bytes it received on each
// accepted connection and scripts the response :status per connection index.
// Each retryExecutorH2 attempt re-dials (the no-keepalive per-request-dial retry
// shape, ADR-0056), so connection index N == attempt N, and recordedBodies()[N]
// is the body the backend observed on attempt N. Used to PIN the documented
// buffered-body replay claim: the same req.Body []byte must re-present in full on
// the retry attempt.
type h2RecordingBackend struct {
	mu        sync.Mutex
	bodies    [][]byte // request body bytes received, one entry per served conn
	conns     int64    // accepted-and-served conn count (atomic)
	statusFor func(connIndex int64) int
}

// startH2RecordingBackend listens on a fresh TLS port (NextProtos=["h2"]) and
// serves each accepted conn via runH2RecordingConn, scripting status via
// statusFor(connIndex). Returns the backend (for recordedBodies()) and the
// listener (caller defers Close).
func startH2RecordingBackend(t *testing.T, pki *h2BackendPKI, statusFor func(connIndex int64) int) (*h2RecordingBackend, net.Listener) {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	b := &h2RecordingBackend{statusFor: statusFor}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c)
		}
	}()
	return b, ln
}

// serve handles one connection, serving MULTIPLE sequential streams on it (the
// 43.2a pooled-conn reuse path; ADR-0253 supersedes the ADR-0056 one-stream-per-
// conn shape). For EACH stream it reads HEADERS, drains the request-body DATA
// frames into the recorder (one bodies[] entry per STREAM), and writes a
// HEADERS+DATA response whose :status is statusFor(streamIndex). The b.conns
// counter is now a STREAM counter (one Inc per served stream), so statusFor's
// index == the retry-attempt index regardless of how many physical conns the
// pool used. Mirrors runH2Backend's framing discipline.
func (b *h2RecordingBackend) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return
	}
	if string(prefaceBuf) != "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		return
	}
	fr := http2.NewFramer(conn, conn)
	frame, err := fr.ReadFrame()
	if err != nil {
		return
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return
	}
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return
	}
	if _, err := fr.ReadFrame(); err != nil { // client SETTINGS_ACK
		return
	}
	if err := fr.WriteSettingsAck(); err != nil {
		return
	}
	// The HPACK encoder is connection-scoped (its dynamic table persists across
	// streams); allocate it once outside the per-stream loop.
	var hbuf bytes.Buffer
	henc := hpack.NewEncoder(&hbuf)
	for {
		// Read frames until HEADERS (past any WINDOW_UPDATE / PING / SETTINGS).
		var streamID uint32
		headersEnded := false
		for {
			frame, err = fr.ReadFrame()
			if err != nil {
				return // conn closed (client Close → GOAWAY) or read error
			}
			if hf, ok := frame.(*http2.HeadersFrame); ok {
				streamID = hf.StreamID
				headersEnded = hf.StreamEnded()
				break
			}
		}
		// Drain the request-body DATA frames (if the request carried a body,
		// HEADERS did NOT end the stream) until END_STREAM, accumulating bytes.
		var reqBody bytes.Buffer
		for !headersEnded {
			frame, err = fr.ReadFrame()
			if err != nil {
				return
			}
			if df, ok := frame.(*http2.DataFrame); ok {
				reqBody.Write(df.Data())
				if df.StreamEnded() {
					break
				}
			}
			// Ignore WINDOW_UPDATE / PING etc. interleaved with DATA.
		}
		idx := atomic.AddInt64(&b.conns, 1) - 1
		recorded := append([]byte(nil), reqBody.Bytes()...)
		b.mu.Lock()
		b.bodies = append(b.bodies, recorded)
		b.mu.Unlock()

		status := strconv.Itoa(b.statusFor(idx))
		respBody := []byte("resp:" + status)
		hbuf.Reset()
		_ = henc.WriteField(hpack.HeaderField{Name: ":status", Value: status})
		_ = henc.WriteField(hpack.HeaderField{Name: "content-type", Value: "text/plain"})
		_ = henc.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(respBody))})
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hbuf.Bytes(),
			EndStream:     false,
			EndHeaders:    true,
		}); err != nil {
			return
		}
		if err := fr.WriteData(streamID, true /* endStream */, respBody); err != nil {
			return
		}
	}
}

// recordedBodies returns a snapshot copy of the per-connection request bodies.
func (b *h2RecordingBackend) recordedBodies() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([][]byte, len(b.bodies))
	copy(out, b.bodies)
	return out
}

// h2EndpointCluster builds a *cluster.Cluster pointing at addr, configured
// to use H2 (UseH2()==true) with the given pki for ALPN h2 verification.
// Mirrors singleEndpointCluster but routes through the manager's
// HttpProtocolOptions parser to set useH2.
// h2EndpointClusterWithRegistry is the Task-11 variant that ALSO returns the
// Registry the Manager registered the cluster's 8 metrics on. Same rationale
// as singleEndpointClusterWithRegistry above.
func h2EndpointClusterWithRegistry(t *testing.T, addr string, pki *h2BackendPKI) (*cluster.Cluster, *stats.Registry) {
	t.Helper()
	return h2EndpointClusterAndReg(t, addr, pki)
}

func h2EndpointCluster(t *testing.T, addr string, pki *h2BackendPKI) *cluster.Cluster {
	t.Helper()
	c, _ := h2EndpointClusterAndReg(t, addr, pki)
	return c
}

func h2EndpointClusterAndReg(t *testing.T, addr string, pki *h2BackendPKI) (*cluster.Cluster, *stats.Registry) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	port64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	hpoH2 := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpoH2)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	tlsCtx := &tlsv3.UpstreamTlsContext{
		Sni: "alpha.envoy-go.test",
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: []string{"h2"},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pemEncodeCAPool(t, pki)},
					},
				},
			},
		},
	}
	tlsAny, err := anypb.New(tlsCtx)
	if err != nil {
		t.Fatalf("anypb.New(UpstreamTlsContext): %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_h2_backend",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(2 * time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_h2_backend",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       host,
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(port64)},
									},
								}},
							}},
						}},
					}},
				},
				TransportSocket: &corev3.TransportSocket{
					Name:       "envoy.transport_sockets.tls",
					ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: tlsAny},
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": hpoAny,
				},
			}},
		},
	}
	reg := stats.NewRegistry()
	cm, err := cluster.NewManager(bs, reg)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	c, ok := cm.Get("c_h2_backend")
	if !ok {
		t.Fatal("cluster.Manager.Get(c_h2_backend) returned !ok")
	}
	if !c.UseH2() {
		t.Fatal("c.UseH2()=false; expected true")
	}
	return c, reg
}

// pemEncodeCAPool re-encodes the PKI's CA cert as PEM bytes for inline_bytes.
// The pki struct already carries leafCertPEM but the cluster manager's
// trusted_ca needs the CA cert specifically — mkH2BackendPKI builds the CA
// pool but doesn't retain the CA PEM separately, so we encode it again from
// the pool's first cert. Simpler: extract from caPool — but x509.CertPool
// has no public accessor. Workaround: regenerate the pool from leafCertPEM's
// chain; for self-signed CA-only this works.
//
// Pragmatic alternative: re-emit the CA PEM by extracting from the pool via
// reflection-free path — pki.caPool was built from the CA's PEM in
// mkH2BackendPKI; but we don't keep that PEM. The cleanest fix is to add a
// caPEM field to h2BackendPKI; do that.
func pemEncodeCAPool(t *testing.T, pki *h2BackendPKI) []byte {
	t.Helper()
	if pki.caPEM == nil {
		t.Fatal("pki.caPEM is nil — was mkH2BackendPKI updated to retain caPEM?")
	}
	return pki.caPEM
}

// h2RequestForTest builds a minimal H2Request the routerActionH2 can pass to
// the upstream client.RoundTrip.
func h2RequestForTest() h2.H2Request {
	return h2.H2Request{
		Method:    "GET",
		Path:      "/x",
		Scheme:    "https",
		Authority: "alpha.envoy-go.test",
		Headers:   nil,
		Body:      nil,
	}
}

// TestRouterActionH2_HappyPath verifies that an upstream H2 200 with a body
// is forwarded byte-for-byte through the H2 action driver's ActionResponse.
// 43.2a Task 7: retargeted from the removed per-request-fresh doH2 method onto
// the live pooled driver doH2ClusterAction (ADR-0253).
func TestRouterActionH2_HappyPath(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2Backend(t, pki, h2BackendOK, body)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("body = %q, want %q", resp.Body, body)
	}
}

// TestRouterActionH2_502OnDialFailure verifies that a closed-port cluster
// produces a 502 local-reply with the bad502Body. 43.2a Task 7: retargeted onto
// the live pooled driver doH2ClusterAction (the pool's dial failure surfaces the
// same 502 ActionResponse as the removed doH2 method did).
func TestRouterActionH2_502OnDialFailure(t *testing.T) {
	pki := mkH2BackendPKI(t)
	// Use port 1 (always rejected) so the pool's dial fails. The pki is unused
	// in the dial-failure path but plumbed through for shape symmetry.
	c := h2EndpointCluster(t, "127.0.0.1:1", pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 502 {
		t.Errorf("status = %d, want 502", resp.Status)
	}
	if string(resp.Body) != bad502Body {
		t.Errorf("body = %q, want %q", resp.Body, bad502Body)
	}
}

// TestRouterActionH2_502OnRoundTripProtocolError verifies that a malformed
// HEADERS frame from the upstream (so RoundTrip surfaces a COMPRESSION_ERROR-
// class error) produces a 502 local-reply.
func TestRouterActionH2_502OnRoundTripProtocolError(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendMalformed, nil)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 502 {
		t.Errorf("status = %d, want 502", resp.Status)
	}
}

// TestRouterActionH2_CtxCancelEmitsRSTStreamCancel verifies that ctx
// cancelation mid-RoundTrip surfaces a stream-scoped CANCEL error
// (which serverStream.dispatch translates to RST_STREAM(CANCEL)).
func TestRouterActionH2_CtxCancelEmitsRSTStreamCancel(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay so RoundTrip gets past the dial+settings
	// handshake and is blocked waiting for the response.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err == nil {
		t.Fatal("doH2ClusterAction returned nil err; want stream-scoped CANCEL error")
	}
	hErr, ok := err.(*h2.Error)
	if !ok {
		t.Fatalf("err is %T, want *h2.Error", err)
	}
	if hErr.Code != h2.ErrCancel {
		t.Errorf("err code = %v, want CANCEL", hErr.Code)
	}
	// Status=0 is the H2 ctx-cancel sentinel (no response is finalized; the
	// CANCEL is signaled via the returned *h2.Error, not via an ActionResponse).
	if resp.Status != 0 {
		t.Errorf("status = %d, want 0 (ctx-cancel sentinel)", resp.Status)
	}
}

// TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass — Phase 06.1 Task 11
// hot path (H2 router): driving routerActionH2.doH2 against a backend
// returning 200 Inc's c.upstreamRqTotal by 1 AND c.upstreamRq2xx by 1, per
// SPEC §5.5 (Increment paths table, "routerActionH2.do (H2)" row).
func TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2Backend(t, pki, h2BackendOK, body)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := doH2ClusterAction(ctx, a, h2RequestForTest()); err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_total"); got != 1 {
		t.Errorf("upstream_rq_total = %d, want 1", got)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_2xx"); got != 1 {
		t.Errorf("upstream_rq_2xx = %d, want 1", got)
	}
}

// TestRouterActionH2_Upstream5xxForwardedVerbatim verifies that an upstream
// 503 status is forwarded verbatim to the downstream (not translated to 502).
// Per the failure-class mapping: protocol errors become 502; upstream HTTP
// status codes (including 5xx) are forwarded.
func TestRouterActionH2_Upstream5xxForwardedVerbatim(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-503\n")
	ln := startH2Backend(t, pki, h2Backend503, body)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("status = %d, want 503 (NOT 502 — only protocol errors translate)", resp.Status)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("body forwarded = %q, want %q", resp.Body, body)
	}
}

// The pre-maintenance TestRouterActionH2_DefensiveDoEmits500AndLogs covered
// the *routerActionH2.do defensive stub for the legacy direct-write H1 path.
// That path (and the routeAction interface's do method) was deleted as
// production-unreachable: variant selection at filter-build time guarantees
// H2-clusters get the H2 closure, so the stub had no remaining caller.
