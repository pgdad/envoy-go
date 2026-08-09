// Package driver registers the 0119-grpc-unary-trailers cross-side
// differential fixture (phase 84, IMPL-84.2).
//
// It proves H2 response-TRAILER forwarding on the gRPC unary path by speaking
// RAW HTTP/2 (golang.org/x/net/http2 Framer + hpack) through each side's
// TLS+ALPN-h2 listener to the runner's GRPCHealthResponder backend
// (BackendKind 34, a real grpc-go health server behind an h2c mux), and
// recording the observed response frame sequence — HEADERS / DATA / TRAILERS
// with end_stream flags and verbatim wire-order trailer fields — into a text
// transcript. BOTH Drive hooks call the SAME drive(); the runner byte-compares
// the two transcripts via CompareBytes.
//
// Four arms, in transcript order:
//
//   - success:       Check("")     -> 200 + DATA(SERVING) + TRAILERS grpc-status=0
//   - notfound:      Check("nope") -> TRAILERS grpc-message + grpc-status=5, no DATA
//   - unimplemented: /Nope         -> TRAILERS grpc-message + grpc-status=12
//   - plain:         GET /plain    -> 200 + DATA(end_stream) and NO trailers
//
// The plain arm is the D-84-ENDSTREAM gate: the three gRPC arms all carry a
// trailing HEADERS block, so they are structurally blind to an unconditional
// END_STREAM regression on the response headers; only a trailer-less response
// discriminates it.
//
// Canonicalization is CLOSED at three rules:
//  1. drop x-envoy-upstream-service-time from response HEADERS by name,
//  2. drop date from response HEADERS by name,
//  3. scrub the side-specific dial addr out of READ-ERR lines.
//
// There is NO sort anywhere: response HEADERS are recorded in WIRE ORDER after
// the two by-name drops, and TRAILERS are recorded VERBATIM in wire order,
// unfiltered and unsorted (grpc-go emits grpc-message BEFORE grpc-status; that
// order is part of what the fixture pins). Read errors are recorded INTO the
// transcript as READ-ERR lines and never returned as the hook's error — an
// early error return would make every later arm unreachable.
//
// Driver-side use of golang.org/x/net/http2.Framer is permitted in test code
// per D-3.2 (which scopes the no-stdlib-http2 rule to RUNTIME code).
package driver

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0119-grpc-unary-trailers"

	clusterName = "c_grpc"

	// In-container reference Envoy listener port (family-banded: 10<index>).
	refContainerListenerPort = 10119

	refAdminPort = 9901

	backendCount = 1

	// armDeadline bounds each arm's dial + write + read loop.
	armDeadline = 5 * time.Second

	// streamID is the single client-initiated stream each arm uses (one
	// fresh connection per arm, so stream 1 every time).
	streamID = 1

	// hpackTableSize is the decoder's dynamic-table size for response
	// header/trailer blocks.
	hpackTableSize = 4096
)

func init() {
	fixture.RegisterFixture(fixtureName, &grpcTrailersDriver{})
}

// grpcTrailersDriver is STATEFUL only insofar as it memoizes the parsed CA
// cert pool (ensureCertPool, the 0079 discipline).
type grpcTrailersDriver struct {
	mu      sync.Mutex
	rootCAs *x509.CertPool
}

func (*grpcTrailersDriver) BackendCount() int                { return backendCount }
func (*grpcTrailersDriver) BackendKind() fixture.BackendKind { return fixture.GRPCHealthResponder }
func (*grpcTrailersDriver) SubjectListenerName() string      { return "l_h2" }
func (*grpcTrailersDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// fixtureDir resolves the absolute path to the fixture directory (the parent
// of this driver/ package), regardless of the test's working directory.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

// readPEM reads a PEM file from the fixture's pki/ directory (copied
// verbatim from 0079-h2-multiplex-pool/pki: ca.pem, listener.pem,
// listener.key.pem).
func readPEM(name string) string {
	path := filepath.Join(fixtureDir(), "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return string(b)
}

// indentPEM returns the PEM body with every line after the first prefixed by
// `indent` spaces (for inline_string block-scalar embedding at a fixed
// depth).
func indentPEM(pem string, indent int) string {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(pem, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
			b.WriteString(pad)
		}
		b.WriteString(line)
	}
	return b.String()
}

// grpcClusterBlock renders the single H2 upstream cluster pointing at the
// GRPCHealthResponder backend (explicit_http_config.http2_protocol_options{}).
func grpcClusterBlock(clusterType, endpointAddr string, endpointPort int) string {
	return fmt.Sprintf(`    - name: %s
      type: %s
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
`, clusterName, clusterType, clusterName, endpointAddr, endpointPort)
}

// h2ListenerFilterChain renders the TLS+ALPN-h2 downstream filter chain (the
// 0004/0079 shape: DownstreamTlsContext, alpn ["h2","http/1.1"], codec_type
// AUTO, HCM + router). PEMs are embedded inline at the listener depth.
func h2ListenerFilterChain() string {
	certIndent := 24 // inline_string body depth under tls_certificates
	cert := indentPEM(readPEM("listener.pem"), certIndent)
	key := indentPEM(readPEM("listener.key.pem"), certIndent)
	return fmt.Sprintf(`        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                alpn_protocols: ["h2", "http/1.1"]
                tls_certificates:
                  - certificate_chain:
                      inline_string: |
                        %s
                    private_key:
                      inline_string: |
                        %s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: AUTO
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: %s }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`, cert, key, clusterName)
}

// ReferenceBootstrap renders the reference Envoy YAML: an l_h2 TLS+ALPN-h2
// listener routed to c_grpc (STRICT_DNS, host.docker.internal).
func (d *grpcTrailersDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	cl := grpcClusterBlock("STRICT_DNS", "host.docker.internal", backendPorts[0])
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
    - name: l_h2
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
%s
  clusters:
%s`, refAdminPort, refContainerListenerPort, h2ListenerFilterChain(), cl)
}

// SubjectConfig renders the subject's bootstrap: the same l_h2 TLS+ALPN-h2
// listener routed to c_grpc (STATIC, 127.0.0.1).
func (d *grpcTrailersDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	cl := grpcClusterBlock("STATIC", "127.0.0.1", backendPorts[0])
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0119, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h2
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
%s
  clusters:
%s`, subjAdminPort, subjListenerPort, h2ListenerFilterChain(), cl)
}

// ensureCertPool builds d.rootCAs from the committed CA PEM on the first
// call (the 0079 discipline).
func (d *grpcTrailersDriver) ensureCertPool() *x509.CertPool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rootCAs != nil {
		return d.rootCAs
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(readPEM("ca.pem"))) {
		panic("driver: failed to parse CA PEM from pki/ca.pem")
	}
	d.rootCAs = pool
	return d.rootCAs
}

// tlsConfig trusts the fixture-local CA and advertises NextProtos=["h2"] so
// the listener (offering ["h2","http/1.1"]) negotiates h2 via ALPN.
// ServerName "localhost" matches the listener cert's DNS SAN.
func (d *grpcTrailersDriver) tlsConfig() *tls.Config {
	return &tls.Config{
		RootCAs:    d.ensureCertPool(),
		NextProtos: []string{"h2"},
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}
}

// ---------------------------------------------------------------------------
// gRPC request framing
// ---------------------------------------------------------------------------

// grpcFrame wraps a serialized protobuf message in the gRPC length-prefixed
// message framing: 1 byte compressed-flag (0) + 4-byte big-endian length + msg.
// grpcFrame(nil) is the empty HealthCheckRequest: 00 00000000.
func grpcFrame(msg []byte) []byte {
	f := make([]byte, 5+len(msg))
	f[0] = 0
	f[1] = byte(len(msg) >> 24)
	f[2] = byte(len(msg) >> 16)
	f[3] = byte(len(msg) >> 8)
	f[4] = byte(len(msg))
	copy(f[5:], msg)
	return f
}

// ---------------------------------------------------------------------------
// Arms
// ---------------------------------------------------------------------------

// arm is one probe: a method+path and an optional request body. gRPC arms send
// content-type application/grpc + te trailers and a gRPC-framed DATA body that
// carries END_STREAM; the plain arm sends a bodyless GET whose request HEADERS
// carry END_STREAM.
type arm struct {
	name string
	meth string
	path string
	grpc bool
	body []byte // nil -> no DATA frame; END_STREAM rides on the request HEADERS
}

// arms returns the probe arms, in transcript order (shared by both sides).
//
// The response shapes each arm produces, MEASURED byte-identical on both sides
// at the 84.2 tip (the reference Envoy and the subject), are:
//
//	success        HEADERS  end_stream=false [:status=200 content-type=application/grpc
//	                        trailer=Grpc-Status trailer=Grpc-Message
//	                        trailer=Grpc-Status-Details-Bin server=envoy]
//	               DATA     end_stream=false len=7 hex=00000000020801   (SERVING)
//	               TRAILERS end_stream=true  [grpc-status=0]
//	notfound       HEADERS  (as above)
//	               TRAILERS end_stream=true  [grpc-message=unknown service grpc-status=5]
//	               (no DATA frame — this backend never emits a Trailers-Only response)
//	unimplemented  HEADERS  (as above)
//	               TRAILERS end_stream=true  [grpc-message=unknown method Nope for
//	                        service grpc.health.v1.Health grpc-status=12]
//	plain          HEADERS  end_stream=false [:status=200 content-type=text/plain;
//	                        charset=utf-8 content-length=16 server=envoy]
//	               DATA     end_stream=true  len=16 hex=6261636b656e642d303a2f706c61696e
//	               (no TRAILERS line at all — the D-84-ENDSTREAM gate)
//
// Note the trailer wire order on the two error arms: grpc-go emits grpc-message
// BEFORE grpc-status. That order is recorded verbatim and never sorted.
func arms() []arm {
	return []arm{
		// Check with an empty HealthCheckRequest (service "") -> SERVING.
		{"success", "POST", "/grpc.health.v1.Health/Check", true, grpcFrame(nil)},
		// Check with service "nope" (field 1, wire type 2: 0x0A 0x04 "nope")
		// -> NOT_FOUND (grpc-status 5), trailers only after HEADERS, no DATA.
		{"notfound", "POST", "/grpc.health.v1.Health/Check", true, grpcFrame([]byte{0x0A, 0x04, 'n', 'o', 'p', 'e'})},
		// Unknown method on a known service -> UNIMPLEMENTED (grpc-status 12).
		{"unimplemented", "POST", "/grpc.health.v1.Health/Nope", true, grpcFrame(nil)},
		// Plain (non-gRPC) H2 GET through the SAME listener: the backend's
		// h2c mux answers 200 + "backend-0:/plain" with NO trailers. This is
		// the D-84-ENDSTREAM gate — the only arm whose response has no
		// trailing HEADERS block.
		{"plain", "GET", "/plain", false, nil},
	}
}

// requestFields returns the request pseudo-header + header fields for an arm,
// in the order they are hpack-encoded onto the wire.
func requestFields(a arm) []hpack.HeaderField {
	fields := []hpack.HeaderField{
		{Name: ":method", Value: a.meth},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: a.path},
		{Name: ":authority", Value: "localhost"},
	}
	if a.grpc {
		fields = append(fields,
			hpack.HeaderField{Name: "content-type", Value: "application/grpc"},
			hpack.HeaderField{Name: "te", Value: "trailers"},
		)
	}
	return fields
}

// encodeHeaderBlock hpack-encodes fields into a single header block fragment.
func encodeHeaderBlock(fields []hpack.HeaderField) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	for _, hf := range fields {
		if err := enc.WriteField(hf); err != nil {
			return nil, fmt.Errorf("hpack encode %s: %w", hf.Name, err)
		}
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Transcript canonicalization (CLOSED at three rules — see the package doc)
// ---------------------------------------------------------------------------

// droppedHeaderNames are the response HEADER names dropped BY NAME before the
// headers line is written. Both are unavoidably side-specific:
// x-envoy-upstream-service-time is a per-request latency in milliseconds, and
// date is a wall-clock timestamp. Nothing else is dropped, and nothing is
// sorted (wire order is part of what the fixture pins).
var droppedHeaderNames = map[string]bool{
	"x-envoy-upstream-service-time": true,
	"date":                          true,
}

// formatFields renders header fields as "name=value name=value …" in WIRE
// ORDER. When filter is true (the response HEADERS leg), droppedHeaderNames
// are removed BY NAME. Trailers are rendered with filter=false: verbatim,
// unfiltered, unsorted.
func formatFields(fields []hpack.HeaderField, filter bool) string {
	parts := make([]string, 0, len(fields))
	for _, hf := range fields {
		if filter && droppedHeaderNames[hf.Name] {
			continue
		}
		parts = append(parts, hf.Name+"="+hf.Value)
	}
	return strings.Join(parts, " ")
}

// scrubAddr replaces the side-specific dial address with a fixed placeholder so
// READ-ERR lines stay cross-side comparable (the reference dials a mapped
// container port, the subject a local one).
func scrubAddr(msg, addr string) string {
	return strings.ReplaceAll(msg, addr, "<addr>")
}

// ---------------------------------------------------------------------------
// Drive
// ---------------------------------------------------------------------------

func (d *grpcTrailersDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr), nil
}

func (d *grpcTrailersDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr), nil
}

// drive runs every arm against one side's listener and returns the transcript.
// It NEVER returns an error: failures are recorded IN the transcript as
// READ-ERR lines so that both sides always produce comparable bytes and no arm
// is made unreachable by an earlier one.
func (d *grpcTrailersDriver) drive(ctx context.Context, addr string) []byte {
	var b bytes.Buffer
	for _, a := range arms() {
		fmt.Fprintf(&b, "ARM %s\n", a.name)
		d.driveArm(ctx, &b, addr, a)
	}
	return b.Bytes()
}

// recordErr appends a READ-ERR line with the dial addr scrubbed.
func recordErr(b *bytes.Buffer, addr, stage string, err error) {
	fmt.Fprintf(b, "  READ-ERR %s: %s\n", stage, scrubAddr(err.Error(), addr))
}

// driveArm opens a FRESH TLS(ALPN h2) connection, performs the request with a
// raw http2.Framer, and appends the observed response frame sequence to the
// transcript.
func (d *grpcTrailersDriver) driveArm(ctx context.Context, b *bytes.Buffer, addr string, a arm) {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: armDeadline},
		Config:    d.tlsConfig(),
	}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		recordErr(b, addr, "dial", err)
		return
	}
	conn, ok := raw.(*tls.Conn)
	if !ok {
		_ = raw.Close()
		fmt.Fprintf(b, "  READ-ERR dial: not a TLS connection\n")
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(armDeadline))
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "h2" {
		fmt.Fprintf(b, "  READ-ERR alpn: negotiated %q, want h2\n", proto)
		return
	}

	// h2 client preface + SETTINGS.
	if _, err := io.WriteString(conn, http2.ClientPreface); err != nil {
		recordErr(b, addr, "preface", err)
		return
	}
	fr := http2.NewFramer(conn, conn)
	fr.ReadMetaHeaders = hpack.NewDecoder(hpackTableSize, nil)
	if err := fr.WriteSettings(); err != nil {
		recordErr(b, addr, "settings", err)
		return
	}

	// Request HEADERS (+ DATA carrying END_STREAM for body-bearing arms).
	block, err := encodeHeaderBlock(requestFields(a))
	if err != nil {
		recordErr(b, addr, "hpack-encode", err)
		return
	}
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndHeaders:    true,
		EndStream:     a.body == nil,
	}); err != nil {
		recordErr(b, addr, "write-headers", err)
		return
	}
	if a.body != nil {
		if err := fr.WriteData(streamID, true, a.body); err != nil {
			recordErr(b, addr, "write-data", err)
			return
		}
	}

	// Read loop: record HEADERS / DATA / TRAILERS for the stream until
	// END_STREAM (or a terminal error, recorded as READ-ERR).
	sawHeaders := false
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			recordErr(b, addr, "read-frame", err)
			return
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				if err := fr.WriteSettingsAck(); err != nil {
					recordErr(b, addr, "settings-ack", err)
					return
				}
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				_ = fr.WritePing(true, f.Data)
			}
		case *http2.MetaHeadersFrame:
			if f.StreamID != streamID {
				continue
			}
			kind := "TRAILERS"
			filter := false
			if !sawHeaders {
				sawHeaders = true
				kind, filter = "HEADERS", true
			}
			fmt.Fprintf(b, "  %-8s end_stream=%t [%s]\n", kind, f.StreamEnded(), formatFields(f.Fields, filter))
			if f.StreamEnded() {
				return
			}
		case *http2.DataFrame:
			if f.StreamID != streamID {
				continue
			}
			fmt.Fprintf(b, "  %-8s end_stream=%t len=%d hex=%s\n", "DATA", f.StreamEnded(), len(f.Data()), hex.EncodeToString(f.Data()))
			if f.StreamEnded() {
				return
			}
		case *http2.RSTStreamFrame:
			if f.StreamID != streamID {
				continue
			}
			fmt.Fprintf(b, "  READ-ERR rst-stream code=%v\n", f.ErrCode)
			return
		case *http2.GoAwayFrame:
			fmt.Fprintf(b, "  READ-ERR goaway code=%v\n", f.ErrCode)
			return
		default:
			// WINDOW_UPDATE / PRIORITY / unknown: not recorded.
		}
	}
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the
// 0074/0079 raw /ready probe, verbatim).
func (*grpcTrailersDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// AssertStats is a committed-tree LIVENESS guard, not a behavioral proof — it
// is deliberately NON-DISCRIMINATING for this fixture's own seam. The
// reference books http.ingress_http.downstream_rq_2xx (and upstream_rq_200)
// for a stream even when its trailers are wrong or absent, so a
// stats-only leg stays GREEN under a broken response-trailer emit; e.g. an
// unconditional-END_STREAM regression on the "success" arm would still be a
// 2xx. Only CompareBytes over the two Drive transcripts discriminates that
// (the VACUITY/shape-31 measurement recorded in the package doc and README).
//
// What this DOES catch: drive() never returns an error — every per-arm
// failure is recorded INTO the transcript as a READ-ERR line
// (reference_gate_command_negative_control's target: the gate command
// itself). A defect that breaks BOTH sides identically the same way (e.g. a
// dial-level regression hit by both the reference and the subject) produces
// byte-equal READ-ERR-only transcripts and a vacuous CompareBytes green. The
// subject having served >= 4 real 2xx responses — one per arm, since
// success/notfound/unimplemented/plain all answer :status=200 even on the
// gRPC-error arms (the gRPC status rides in trailers/HEADERS-adjacent
// fields, not :status) — is the committed-tree signal that distinguishes
// "four arms were actually driven and compared" from "both sides failed
// identically before ever reaching the backend".
//
// This >= len(arms()) floor is a deliberate strengthening of the PLAN's
// >= 1: the Task 4-5 review (finding I-1) noted a >= 1 floor would still be
// satisfied by a single working arm out of four, which is a weaker liveness
// signal than this fixture's four-arm design supports.
func (d *grpcTrailersDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	_ = refAdminAddr // reference-side stats are not asserted (subject-only liveness guard)

	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	const key = "http.ingress_http.downstream_rq_2xx"
	wantMin := uint64(len(arms()))
	if got := subj[key]; got < wantMin {
		t.Errorf("subj %s = %d, want >= %d (one per arm — NON-discriminating liveness guard, see doc comment)", key, got, wantMin)
	}
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0057/0059 driver
// scrapeStats, verbatim.)
func scrapeStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}

	out := make(map[string]uint64)
	for _, line := range strings.Split(buf.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		valStr := strings.TrimSpace(line[idx+2:])
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue // skip non-numeric (histograms, special formats)
		}
		out[name] = v
	}
	return out, nil
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*grpcTrailersDriver)(nil)
	_ fixture.BackendKindAware = (*grpcTrailersDriver)(nil)
	_ fixture.StatsAsserter    = (*grpcTrailersDriver)(nil)
)
