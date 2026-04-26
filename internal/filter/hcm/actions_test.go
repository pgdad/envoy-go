package hcm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

func TestDirectResponseAction_Do(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(context.Background(), &http.Request{}, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}
	if !strings.HasSuffix(out, "OK\n") {
		t.Errorf("expected body 'OK\\n' suffix, got: %q", out)
	}
}

func TestDirectResponseWriteH1_GoldenCompat(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.writeH1(bw); err != nil {
		t.Fatalf("writeH1 = %v", err)
	}
	_ = bw.Flush()
	got := regexp.MustCompile(`(?m)^Date: .+$`).ReplaceAllString(buf.String(), "Date: <DATE>")
	wantBytes, err := os.ReadFile("testdata/direct_response_h1.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(wantBytes) {
		t.Errorf("writeH1 output diverged from phase-04 golden:\nGOT:\n%s\nWANT:\n%s", got, wantBytes)
	}
}

type captureSW struct {
	headerCalls [][]hpack.HeaderField
	dataCalls   [][]byte
	endStream   []bool
}

func (c *captureSW) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	c.headerCalls = append(c.headerCalls, headers)
	c.endStream = append(c.endStream, endStream)
	return nil
}
func (c *captureSW) WriteData(b []byte, endStream bool) error {
	c.dataCalls = append(c.dataCalls, append([]byte(nil), b...))
	c.endStream = append(c.endStream, endStream)
	return nil
}

func TestDirectResponseWriteH2_HEADERSThenDATAEndStream(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	sw := &captureSW{}
	if err := a.writeH2(sw); err != nil {
		t.Fatalf("writeH2 = %v", err)
	}
	if len(sw.headerCalls) != 1 || len(sw.dataCalls) != 1 {
		t.Fatalf("got %d header calls + %d data calls; want 1 + 1", len(sw.headerCalls), len(sw.dataCalls))
	}
	hdrs := sw.headerCalls[0]
	if hdrs[0].Name != ":status" || hdrs[0].Value != "200" {
		t.Errorf("first header = %+v, want :status=200", hdrs[0])
	}
	// Verify regular headers are present and after pseudo-headers.
	wantNames := map[string]bool{"date": false, "server": false, "content-type": false, "content-length": false}
	for _, h := range hdrs[1:] {
		if h.Name[0] == ':' {
			t.Errorf("pseudo-header %q after regular headers (RFC 9113 §8.3 violation)", h.Name)
		}
		if _, want := wantNames[h.Name]; want {
			wantNames[h.Name] = true
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("missing regular header %q", name)
		}
	}
	if string(sw.dataCalls[0]) != "OK\n" {
		t.Errorf("data = %q, want %q", sw.dataCalls[0], "OK\n")
	}
	// END_STREAM must be set on the DATA frame (the last call), not on HEADERS
	// in this test (because there's a body).
	if sw.endStream[0] /* HEADERS endStream */ {
		t.Errorf("HEADERS frame had endStream=true; expected false (body follows)")
	}
	if !sw.endStream[1] /* DATA endStream */ {
		t.Errorf("DATA frame had endStream=false; expected true (last frame)")
	}
}

// loopbackHTTPEcho starts a tiny HTTP/1.1 echo server and returns its address
// + a stop function. The server reads one request, writes one response with
// body "echo:<URL.Path>", then closes. Used to exercise routerAction.
func loopbackHTTPEcho(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				body := "echo:" + req.URL.Path
				resp := fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body,
				)
				_, _ = c.Write([]byte(resp))
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

// singleEndpointCluster builds a *cluster.Cluster pointing at addr by going
// through cluster.NewManager with a minimal Bootstrap. Mirrors the
// mkClusterMgr helper in internal/listener/manager_test.go:59-93.
func singleEndpointCluster(t *testing.T, addr string) *cluster.Cluster {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	port64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		t.Fatalf("ParseUint %q: %v", portStr, err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_test",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
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
			}},
		},
	}
	cm, err := cluster.NewManager(bs)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	c, ok := cm.Get("c_test")
	if !ok {
		t.Fatal("cluster.Manager.Get(c_test) returned !ok")
	}
	return c
}

func TestRouterAction_DoHappy(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	c := singleEndpointCluster(t, addr)
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	// loopbackHTTPEcho writes `Connection: close` in its response, so the
	// router action correctly signals close via errCloseAfterAction (per
	// SPEC §5.3 / SPEC §10 #3 settled). Any other error is a real failure.
	if err := a.do(req.Context(), req, bw); err != nil && !errors.Is(err, errCloseAfterAction) {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()
	if !strings.Contains(buf.String(), "echo:/x") {
		t.Errorf("expected echo:/x in response, got: %q", buf.String())
	}
}

func TestRouterAction_DoDialFailureReturns503(t *testing.T) {
	// Cluster with an unreachable endpoint (port 1 is always rejected).
	c := singleEndpointCluster(t, "127.0.0.1:1")
	a := &routerAction{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(req.Context(), req, bw); err != nil {
		// dial-failure becomes a 503 LOCAL REPLY; do() should NOT error
		// (it writes the local reply and returns nil).
		if !errors.Is(err, errCloseAfterAction) {
			t.Errorf("dial failure should write 503 and return nil (or sentinel), got: %v", err)
		}
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("expected 503 local reply on dial failure, got: %q", buf.String())
	}
}

func TestRouterAction_DoCtxCancel(t *testing.T) {
	addr, stop := loopbackHTTPEcho(t)
	defer stop()

	c := singleEndpointCluster(t, addr)
	a := &routerAction{cluster: c}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before do — Cluster.Dial(ctx) should return ctx.Err()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://upstream/x", nil)
	req.URL.Path = "/x"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(ctx, req, bw); err != nil {
		t.Errorf("ctx-cancel should map to 503 local reply, not propagate err: %v", err)
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 503 ") {
		t.Errorf("ctx cancel should produce 503 local reply, got: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Phase 05.2 — routerActionH2 tests
// ---------------------------------------------------------------------------

// captureH2Writer is a fake h2.StreamWriter that records every call. Each
// recorded entry is an event with a kind discriminator + payload so test
// assertions can index calls in order.
type captureH2Writer struct {
	mu        sync.Mutex
	headers   [][]hpack.HeaderField
	data      [][]byte
	endStream []bool
	// kind entries are "headers" or "data"; len matches headers + data.
	order []string
}

func (c *captureH2Writer) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]hpack.HeaderField, len(headers))
	copy(cp, headers)
	c.headers = append(c.headers, cp)
	c.endStream = append(c.endStream, endStream)
	c.order = append(c.order, "headers")
	return nil
}
func (c *captureH2Writer) WriteData(b []byte, endStream bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := append([]byte(nil), b...)
	c.data = append(c.data, cp)
	c.endStream = append(c.endStream, endStream)
	c.order = append(c.order, "data")
	return nil
}

// statusOf returns the :status value from the first headers call, or "" if
// no headers were recorded.
func (c *captureH2Writer) statusOf() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.headers) == 0 {
		return ""
	}
	for _, h := range c.headers[0] {
		if h.Name == ":status" {
			return h.Value
		}
	}
	return ""
}

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
		// HPACK-encode the response headers with a fresh encoder (matches the
		// from-scratch codec's per-conn encoder discipline).
		var hbuf bytes.Buffer
		henc := hpack.NewEncoder(&hbuf)
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
		// Linger so the client can send GOAWAY on Close without write-side errors.
		_, _ = io.Copy(io.Discard, conn)
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

// h2EndpointCluster builds a *cluster.Cluster pointing at addr, configured
// to use H2 (UseH2()==true) with the given pki for ALPN h2 verification.
// Mirrors singleEndpointCluster but routes through the manager's
// HttpProtocolOptions parser to set useH2.
func h2EndpointCluster(t *testing.T, addr string, pki *h2BackendPKI) *cluster.Cluster {
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
	cm, err := cluster.NewManager(bs)
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
	return c
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
// is forwarded byte-for-byte to the captured downstream writer.
func TestRouterActionH2_HappyPath(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2Backend(t, pki, h2BackendOK, body)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.doH2(ctx, h2RequestForTest(), w); err != nil {
		t.Fatalf("doH2: %v", err)
	}
	if got := w.statusOf(); got != "200" {
		t.Errorf(":status = %q, want %q", got, "200")
	}
	if len(w.data) != 1 {
		t.Fatalf("data calls = %d, want 1", len(w.data))
	}
	if !bytes.Equal(w.data[0], body) {
		t.Errorf("data = %q, want %q", w.data[0], body)
	}
}

// TestRouterActionH2_502OnDialFailure verifies that a closed-port cluster
// produces a 502 local-reply with the bad502Body.
func TestRouterActionH2_502OnDialFailure(t *testing.T) {
	pki := mkH2BackendPKI(t)
	// Use port 1 (always rejected) so DialH2 fails. The pki is unused in
	// the dial-failure path but plumbed through for shape symmetry.
	c := h2EndpointCluster(t, "127.0.0.1:1", pki)
	a := &routerActionH2{cluster: c}

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.doH2(ctx, h2RequestForTest(), w); err != nil {
		t.Fatalf("doH2: %v", err)
	}
	if got := w.statusOf(); got != "502" {
		t.Errorf(":status = %q, want %q", got, "502")
	}
	if len(w.data) != 1 || string(w.data[0]) != bad502Body {
		t.Errorf("body = %q, want %q", w.data, bad502Body)
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

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.doH2(ctx, h2RequestForTest(), w); err != nil {
		t.Fatalf("doH2: %v", err)
	}
	if got := w.statusOf(); got != "502" {
		t.Errorf(":status = %q, want %q", got, "502")
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

	w := &captureH2Writer{}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay so RoundTrip gets past the dial+settings
	// handshake and is blocked waiting for the response.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	err := a.doH2(ctx, h2RequestForTest(), w)
	if err == nil {
		t.Fatal("doH2 returned nil; want stream-scoped CANCEL error")
	}
	hErr, ok := err.(*h2.Error)
	if !ok {
		t.Fatalf("err is %T, want *h2.Error", err)
	}
	if hErr.Code != h2.ErrCancel {
		t.Errorf("err code = %v, want CANCEL", hErr.Code)
	}
	// No headers/data should have been written on the captured writer (the
	// CANCEL is signaled via the returned error, not via writer calls).
	if len(w.headers) != 0 {
		t.Errorf("headers calls = %d, want 0 on CANCEL path", len(w.headers))
	}
	if len(w.data) != 0 {
		t.Errorf("data calls = %d, want 0 on CANCEL path", len(w.data))
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

	w := &captureH2Writer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.doH2(ctx, h2RequestForTest(), w); err != nil {
		t.Fatalf("doH2: %v", err)
	}
	if got := w.statusOf(); got != "503" {
		t.Errorf(":status = %q, want %q (NOT 502 — only protocol errors translate)", got, "503")
	}
	if len(w.data) != 1 || !bytes.Equal(w.data[0], body) {
		t.Errorf("body forwarded = %q, want %q", w.data, body)
	}
}

// TestRouterActionH2_DefensiveDoEmits500AndLogs verifies the H1-path
// defensive stub of *routerActionH2: in well-formed bootstraps, build-
// time variant selection in buildRouterAction routes H2-cluster routes
// to *routerActionH2 (consumed by the H2 driver via h2RouterActionAdapter)
// and H1-cluster routes to *routerAction. The H1 driver only invokes
// routeAction.do(...) — never routeAction.doH2(...) — so on the H1 path
// reaching *routerActionH2.do is a bootstrap-misconfiguration signal.
//
// Closes REVIEW I-2 (observability gap). The stub must:
//  1. emit a 500 status line on the bufio.Writer (not crash; not panic);
//  2. log a diagnostic naming the cluster and the misconfiguration so an
//     operator debugging an unexpected 500 can grep server logs for the
//     cause without spelunking the codec-dispatch code.
func TestRouterActionH2_DefensiveDoEmits500AndLogs(t *testing.T) {
	// Capture log output so we can assert the diagnostic line.
	var logBuf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	// Construct a minimal *routerActionH2 against any cluster (the cluster
	// is only used for its Name() in the log line; do() does not dial).
	// Use the existing singleEndpointCluster helper for an H1 cluster
	// because we explicitly want to exercise the H1-path stub.
	c := singleEndpointCluster(t, "127.0.0.1:1")
	a := &routerActionH2{cluster: c}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://x/", nil)
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.do(req.Context(), req, bw); err != nil {
		t.Fatalf("do: unexpected error %v", err)
	}
	_ = bw.Flush()
	if !strings.HasPrefix(buf.String(), "HTTP/1.1 500 ") {
		t.Errorf("expected 500 status line, got: %q", buf.String())
	}
	if !strings.Contains(logBuf.String(), "routerActionH2.do reached on H1 path") {
		t.Errorf("expected misconfiguration diagnostic in log, got: %q", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), `cluster=`) {
		t.Errorf("expected cluster name in log diagnostic, got: %q", logBuf.String())
	}
}
