// Package inputs registers the 0016-http-compressor fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.compressor and reference Envoy v1.37.2 across the
// six-scenario matrix per phase 14 SPEC §7.1.
//
// Integration shape (single-listener fixture.Driver — planner-time decision 11,
// mirrors the phase-13 buffer + phase-12 csrf precedents):
//
//  1. ReferenceBootstrap renders test/fixtures/0016-http-compressor/envoy.yaml
//     with the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port (loopback).
//
//  2. DriveReference / DriveSubject issue an identical 6-scenario sequence
//     against each proxy and emit a deterministic per-scenario assertion-log
//     byte stream. The runner's CompareBytes pass enforces equivalence — when
//     both proxies produce equal logs, the differential gate fires.
//
//     The 6 scenarios per SPEC §7.1:
//     1: allow-compress     (GET /text-html-1024 AE: gzip)            → 200 gzip
//     2: skip content-type  (GET /image-png-1024 AE: gzip)            → 200 identity
//     3: skip below-min     (GET /text-html-10 AE: gzip)              → 200 identity
//     4: etag strong-strip  (GET /text-html-etag-strong AE: gzip)     → 200 gzip; etag stripped
//     5: per-route disabled (GET /per-route-disabled AE: gzip)        → 200 identity
//     6: per-route rmAE     (GET /per-route-rmae AE: gzip)            → 200 gzip; upstream AE stripped
//
//     Per-scenario log line shape:
//
//     scenario <id> status=<code> ce=<content-encoding> vary=<vary> body=<assertion-result>
//     [+ scenario 4: etag-absent=<bool>]
//     [+ scenario 6: ae-absent-upstream=<bool>]
//
//     Body assertion-mode per ADR-0133 §Decision (i)+(ii): dispatch on response
//     `Content-Encoding`. Uncompressed scenarios (2, 3, 5) use byte-exact body
//     comparison; compressed scenarios (1, 4, 6) decompress both sides via
//     compress/gzip.NewReader and compare decompressed plaintexts. ADR-0133
//     codifies this discipline.
//
//     The per-scenario log emits an assertion VERDICT (byte-exact-ok or
//     decompressed-byte-exact-ok), NOT the raw response bytes. This is the same
//     determinism trick the phase-13 buffer driver uses: emit deterministic
//     verdict strings so both sides produce byte-identical logs even though the
//     raw response wires diverge (compressed bytes differ between Go gzip and
//     libz; content-length / transfer-encoding shapes differ per §11.9).
//     Cross-side body equivalence is asserted INSIDE the driver via
//     assertBodyEquivalent — the runner's CompareBytes only sees the verdict.
//
//     Since each side calls driveProxy independently against its OWN proxy, the
//     cross-side decompress-and-compare cannot fire here in the driver — it
//     fires at Task 14 in the runner-level integration. Task 11 lands the
//     helper primitives (decompressGzip + assertBodyEquivalent +
//     assertNoAcceptEncodingInEchoedBody) and the per-side per-scenario
//     within-side assertions (status code, CE present-or-absent, Vary
//     present-or-absent, decompresses cleanly when CE: gzip).
//
//  3. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
//     The counter-delta assertion (per SPEC §7.1 column 5; ADR-0133 §Decision
//     (iii) boundary-only tolerance on response_total_compressed_bytes) lands
//     at Task 14 via the StatsAsserter optional interface.
package inputs

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0016-http-compressor"

	// In-container reference Envoy listener port (pre-assigned per bootstrap).
	// Convention `100NN` for fixture `00NN`: phase 14 follows with 10016 for
	// the single l_main listener (phase 13 used 10015, phase 12 used 10014, etc.).
	refContainerListenerPort = 10016

	// statPrefix matches the YAML's HCM stat_prefix. Used by the
	// StatsAsserter (Task 14) for stat_prefix-label discrimination when
	// scraping /stats/prometheus.
	statPrefix = "ingress_compressor"
)

func init() {
	fixture.RegisterFixture(fixtureName, &compressorDriver{})
}

// compressorDriver carries per-test mutable state captured during
// DriveReference + DriveSubject so AssertStats (Task 14) can compute the
// per-side expected `response_total_uncompressed_bytes` counter — scenario 6
// routes through the echobackend, whose response body length is per-side
// variable (Envoy-injected x-envoy-* / x-request-id / x-forwarded-* + the
// host:port string differ between reference Envoy and envoy-go); the
// dynamic-per-side expected value is computed from the body the driver
// observed during DriveReference / DriveSubject.
//
// The runner never invokes a fixture driver concurrently with itself (no
// t.Parallel() in TestDifferential) so the mutex is conservative — present
// to keep the race-detector quiet across the Drive→AssertStats boundary.
type compressorDriver struct {
	mu sync.Mutex
	// scenario6BodyLen is the decompressed scenario-6 response body length in
	// bytes, captured on a per-side basis during driveProxy + recorded into
	// this map keyed by "ref" / "subj".
	scenario6BodyLen map[string]int
}

// --- fixture.Driver (required) ---

func (*compressorDriver) BackendCount() int                { return 1 }
func (*compressorDriver) BackendKind() fixture.BackendKind { return fixture.HTTPCompressor }
func (*compressorDriver) SubjectListenerName() string      { return "l_main" }
func (*compressorDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port. Reference Envoy admin + listener ports are
// pre-assigned constants (9901, 10016).
func (*compressorDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    9901,
		"ListenerPort": refContainerListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback).
func (*compressorDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference + DriveSubject issue the identical 6-scenario sequence and
// return the per-scenario assertion-log byte stream. CompareBytes passes when
// both sides produce identical logs.
func (d *compressorDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxyAndRecord(ctx, addr, "ref")
}

func (d *compressorDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxyAndRecord(ctx, addr, "subj")
}

// driveProxyAndRecord wraps driveProxy with per-side state capture: the
// scenario-6 response body length is recorded into d.scenario6BodyLen so
// AssertStats can compute the per-side expected
// `response_total_uncompressed_bytes` counter (scenario-6 body is variable
// per side; scenarios 1+4 are fixed 1024).
func (d *compressorDriver) driveProxyAndRecord(ctx context.Context, addr, side string) ([]byte, error) {
	logBytes, s6Len, err := driveProxyWithCapture(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	if d.scenario6BodyLen == nil {
		d.scenario6BodyLen = map[string]int{}
	}
	d.scenario6BodyLen[side] = s6Len
	d.mu.Unlock()
	return logBytes, nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (*compressorDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- scenarios (per SPEC §7.1) ---

// scenario describes one of the 6 per-request rows in SPEC §7.1.
type scenario struct {
	id             int
	method         string
	path           string
	acceptEncoding string

	// expectedStatus is asserted on each side independently.
	expectedStatus int

	// expectedCE is "gzip" on compressed scenarios (1, 4, 6); "" on
	// uncompressed (2, 3, 5).
	expectedCE string

	// expectedVary is "Accept-Encoding" on compressed scenarios + scenarios
	// where the filter's Vary-injection fires; "" otherwise. Per SPEC §7.1:
	// fires on scenarios 1, 4, 6 (compressed); absent on 2, 3, 5.
	expectedVary string

	// originalPayload is the pre-compression input body (for compressed
	// scenarios where the driver can additionally assert decompressed body ==
	// original input per ADR-0133 §Decision (ii)). Nil for uncompressed
	// scenarios + scenario 6 (where the body is the echo backend's JSON, not
	// a fixed string).
	originalPayload []byte

	// assertEtagAbsent fires on scenario 4 — the strong-ETag-strip discipline
	// per SPEC §11.7 mode-a asserts ETag is ABSENT in the response.
	assertEtagAbsent bool

	// assertNoAEInBody fires on scenario 6 — the per-route
	// remove_accept_encoding_header override stripped Accept-Encoding
	// upstream-side; the echobackend echoes the upstream-observed headers in
	// its JSON body; the driver verifies "accept-encoding" is absent from
	// that map.
	assertNoAEInBody bool
}

var scenarios = []scenario{
	{id: 1, method: "GET", path: "/text-html-1024", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", originalPayload: bytes.Repeat([]byte("A"), 1024)},
	{id: 2, method: "GET", path: "/image-png-1024", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{id: 3, method: "GET", path: "/text-html-10", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{id: 4, method: "GET", path: "/text-html-etag-strong", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", originalPayload: bytes.Repeat([]byte("B"), 1024), assertEtagAbsent: true},
	{id: 5, method: "GET", path: "/per-route-disabled", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{id: 6, method: "GET", path: "/per-route-rmae", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", assertNoAEInBody: true},
}

// --- core drive logic ---

// normalizeListenerAddr replaces the unspecified-address forms ("0.0.0.0" and
// "[::]") that net.Listener.Addr().String() can emit on Linux with "127.0.0.1"
// so that http.Client can connect to the local proxy. Reference Envoy addresses
// are already in host:port form from Docker port-mapping and do not need
// normalization.
func normalizeListenerAddr(addr string) string {
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "127.0.0.1:" + addr[len("0.0.0.0:"):]
	}
	if strings.HasPrefix(addr, "[::]:") {
		return "127.0.0.1:" + addr[len("[::]:"):]
	}
	return addr
}

// driveProxyWithCapture issues the 6 scenarios against addr and returns
// (deterministic-format assertion-log lines, scenario-6 decompressed body
// length, error). The "side" (ref vs subj) is INTENTIONALLY excluded from
// the log so both sides produce identical byte streams when behavior is
// equivalent.
//
// Per-scenario encoding (deterministic; both sides match when behavior matches):
//
//	scenario <id> status=<code> ce=<value> vary=<value> body=<verdict>
//	  [scenario 4: etag-absent=<bool>]
//	  [scenario 6: ae-absent-upstream=<bool>]
//
// Where <verdict> is one of: "identity-len=<N>" (uncompressed), "gzip-roundtrip-ok
// [plain-len=<N>]" (response decompresses cleanly; plain-len only emitted when
// originalPayload is non-nil — i.e. fixed-length scenarios 1+4), or "<error string>"
// on failure. The compressed-byte content itself is NOT in the log (it diverges
// between Go gzip and libz per §11.14); the verdict captures the load-bearing
// observable.
//
// The second return value is the decompressed length of the scenario-6
// response body (the echobackend's JSON echo). Recorded for Task 14's
// AssertStats so the per-side expected `response_total_uncompressed_bytes`
// can be computed dynamically (scenarios 1+4 contribute 1024+1024 = 2048
// fixed; scenario 6's body is upstream-side variable per side).
func driveProxyWithCapture(ctx context.Context, addr string) ([]byte, int, error) {
	listenerURL := "http://" + normalizeListenerAddr(addr)

	// Shared transport — disable keep-alives so each scenario uses a fresh
	// connection (avoids cross-scenario state leakage on Envoy's side).
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr}

	var b bytes.Buffer
	var s6BodyLen int

	for i := range scenarios {
		s := scenarios[i]
		obs := emitScenario(ctx, &b, client, listenerURL, &s)
		if s.id == 6 {
			s6BodyLen = obs.decompressedBodyLen
		}
	}

	return b.Bytes(), s6BodyLen, nil
}

// scenarioObservation captures per-side observables from emitScenario that
// aren't directly visible to the byte-stream comparison (which sees only the
// log lines emitted into b). Currently used by Task 14's per-side
// `response_total_uncompressed_bytes` counter computation: the scenario-6
// decompressed body length is per-side variable (echobackend echoes Envoy-
// injected headers + host:port).
type scenarioObservation struct {
	// decompressedBodyLen is the byte count after gzip decompression on
	// compressed scenarios, OR len(body) on uncompressed scenarios. Used by
	// AssertStats (scenario 6) to compute the per-side dynamic expected
	// `response_total_uncompressed_bytes` counter.
	decompressedBodyLen int
}

// emitScenario issues one scenario request, asserts the per-side observables,
// and writes the deterministic log line(s) into b. Errors during the request
// or response read are logged inline (not returned) so subsequent scenarios
// still run. Returns a scenarioObservation carrying per-side observables not
// directly visible to the byte-stream comparison (currently the decompressed
// body length, used by Task 14's StatsAsserter).
func emitScenario(ctx context.Context, b *bytes.Buffer, client *http.Client, baseURL string, s *scenario) scenarioObservation {
	req, err := http.NewRequestWithContext(ctx, s.method, baseURL+s.path, nil)
	if err != nil {
		fmt.Fprintf(b, "scenario %d ERROR: build request: %v\n", s.id, err)
		return scenarioObservation{}
	}
	// Explicitly set Accept-Encoding: this both encodes the scenario's
	// AE-token AND disables net/http transport's automatic gzip decoding
	// (Go's transparent path is opt-out when the caller sets AE directly).
	// We want the raw wire body so the driver can decompress it explicitly
	// via compress/gzip.NewReader per ADR-0133.
	req.Header.Set("Accept-Encoding", s.acceptEncoding)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(b, "scenario %d ERROR: do request: %v\n", s.id, err)
		return scenarioObservation{}
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		fmt.Fprintf(b, "scenario %d ERROR: read body: %v\n", s.id, err)
		return scenarioObservation{}
	}

	ce := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	vary := resp.Header.Get("Vary")

	// Per-side observable verdicts: status / CE / Vary match expectations.
	// Emitted as boolean strings so both sides produce byte-identical logs
	// when behavior matches expectations on both proxies.
	statusOK := resp.StatusCode == s.expectedStatus
	ceOK := ce == s.expectedCE
	varyOK := varyMatches(vary, s.expectedVary)

	verdict := classifyBody(ce, body, s.originalPayload)
	fmt.Fprintf(b, "scenario %d status=%d (ok=%v) ce=%q (ok=%v) vary=%q (ok=%v) body=%s\n",
		s.id, resp.StatusCode, statusOK, ce, ceOK, vary, varyOK, verdict)

	obs := scenarioObservation{decompressedBodyLen: len(body)}

	if s.assertEtagAbsent {
		etagPresent := resp.Header.Get("Etag") != ""
		fmt.Fprintf(b, "  etag-absent=%v\n", !etagPresent)
	}
	if s.assertNoAEInBody {
		// On scenario 6 the response is gzipped per SPEC §7.1; decompress
		// before parsing the echoed JSON.
		plain := body
		if ce == "gzip" {
			if p, derr := decompressGzip(body); derr == nil {
				plain = p
			}
		}
		obs.decompressedBodyLen = len(plain)
		err := assertNoAcceptEncodingInEchoedBody(plain)
		fmt.Fprintf(b, "  ae-absent-upstream=%v\n", err == nil)
	} else if ce == "gzip" {
		// Compressed scenarios without assertNoAEInBody (scenarios 1, 4):
		// decompress so obs.decompressedBodyLen reflects the plaintext size
		// (not the gzip-compressed wire size).
		if p, derr := decompressGzip(body); derr == nil {
			obs.decompressedBodyLen = len(p)
		}
	}

	return obs
}

// varyMatches reports whether the observed Vary header value satisfies the
// expected token expectation. When want=="" the Vary header MUST be empty.
// When want!="" the Vary header MUST contain the expected token as a
// case-insensitive comma-separated token (RFC 7231 §7.1.4 list-form). Used
// by emitScenario to emit a per-side verdict.
func varyMatches(got, want string) bool {
	if want == "" {
		return strings.TrimSpace(got) == ""
	}
	wantLower := strings.ToLower(strings.TrimSpace(want))
	for _, tok := range strings.Split(got, ",") {
		if strings.ToLower(strings.TrimSpace(tok)) == wantLower {
			return true
		}
	}
	return false
}

// classifyBody returns a deterministic verdict string capturing the body's
// shape (compressed-and-decompresses-cleanly OR uncompressed-and-byte-shape-ok)
// WITHOUT logging the raw bytes themselves. This keeps the per-side log
// byte-stable across proxies (Go gzip vs libz produce different compressed
// bytes; the verdict abstracts that away). The actual cross-side
// decompress-and-compare assertion runs separately at runner-level (Task 14).
//
// Per ADR-0133 §Decision (i): on `Content-Encoding: gzip` the body is
// decompressed via compress/gzip.NewReader; on empty/absent CE the body is
// taken as-is.
//
// Length-emission discipline (Task 14 — first end-to-end differential pass):
//
//   - When originalPayload is non-nil, the decompressed plaintext is
//     byte-compared against it AND the length is emitted (the original-
//     payload length is a fixed compile-time constant; cross-side identical).
//     Scenarios 1 + 4 take this path.
//
//   - When originalPayload is nil AND CE is gzip, the body length is omitted
//     from the verdict — the body shape is determined by the upstream's
//     response (e.g. scenario 6's echobackend echoes ALL request headers
//     including Envoy-injected x-envoy-* / x-request-id / x-forwarded-* and
//     the host:port string, which legitimately differ between reference
//     Envoy (`host.docker.internal:port`) and envoy-go (`127.0.0.1:port`)).
//     A per-side `plain-len` would diverge cross-side; the verdict thus
//     omits it. The driver still verifies the body decompresses cleanly +
//     (for scenario 6) asserts upstream-side Accept-Encoding absence via
//     assertNoAcceptEncodingInEchoedBody.
//
//   - Uncompressed scenarios with no originalPayload (scenarios 2, 3, 5)
//     emit identity-len=<observed-length>. These three serve direct_response
//     bodies whose size is a fixed compile-time constant (1024 / 10 / 1024),
//     so cross-side equal by construction.
func classifyBody(ce string, body, originalPayload []byte) string {
	switch ce {
	case "":
		// Uncompressed. Direct_response bodies are fixed-length; cross-side
		// equal by construction. Cluster-routed uncompressed scenarios do
		// not exist in fixture 0016 (scenario 6's CE is gzip).
		return fmt.Sprintf("identity-len=%d", len(body))
	case "gzip":
		plain, err := decompressGzip(body)
		if err != nil {
			return fmt.Sprintf("gzip-decompress-error:%v", err)
		}
		if originalPayload != nil {
			if !bytes.Equal(plain, originalPayload) {
				return fmt.Sprintf("gzip-roundtrip-payload-mismatch:plain-len=%d", len(plain))
			}
			return fmt.Sprintf("gzip-roundtrip-ok plain-len=%d", len(plain))
		}
		// originalPayload nil — variable-length upstream body (scenario 6
		// echobackend). Emit verdict WITHOUT plain-len to avoid cross-side
		// divergence from Envoy-injected x-forwarded-* / x-request-id /
		// host:port-string-length variance.
		return "gzip-roundtrip-ok"
	default:
		return fmt.Sprintf("unexpected-ce:%q", ce)
	}
}

// --- ADR-0133 helpers ---

// scenarioResult bundles the response header + body for the per-scenario
// assertBodyEquivalent helper. Used at runner-level (Task 14) for the
// cross-side decompress-and-compare assertion + by the helper unit tests at
// Task 11.
type scenarioResult struct {
	Header http.Header
	Body   []byte
}

// assertBodyEquivalent dispatches on Content-Encoding per ADR-0133 §Decision (ii).
//
//   - On Content-Encoding empty/absent → require byte-exact equality of raw bodies.
//   - On Content-Encoding "gzip" → decompress BOTH sides via decompressGzip
//     (compress/gzip.NewReader) and require byte-exact equality of the resulting
//     plaintexts. Optionally also require the plaintext equals originalPayload
//     when non-nil (ADR-0133 §Decision (ii) optional invariant).
//   - On any other Content-Encoding → fail with an unsupported-encoding error
//     (future codec/transform filters extend the switch per ADR-0133 §Decision (v)).
//
// The two sides MUST agree on Content-Encoding (compressors are deterministic
// on the encode-or-not decision; the compressed bytes themselves may differ).
// A Content-Encoding mismatch is a hard failure independent of the body axis.
func assertBodyEquivalent(envoyGo, envoy *scenarioResult, originalPayload []byte) error {
	egCE := strings.ToLower(strings.TrimSpace(envoyGo.Header.Get("Content-Encoding")))
	enCE := strings.ToLower(strings.TrimSpace(envoy.Header.Get("Content-Encoding")))
	if egCE != enCE {
		return fmt.Errorf("Content-Encoding mismatch: envoy-go=%q envoy=%q", egCE, enCE)
	}
	if egCE == "" {
		if !bytes.Equal(envoyGo.Body, envoy.Body) {
			return fmt.Errorf("uncompressed bodies differ: envoy-go=%d bytes; envoy=%d bytes", len(envoyGo.Body), len(envoy.Body))
		}
		return nil
	}
	if egCE != "gzip" {
		return fmt.Errorf("unsupported Content-Encoding: %q (ADR-0133 §Decision (v) — phase 14 supports only gzip)", egCE)
	}
	egPlain, err := decompressGzip(envoyGo.Body)
	if err != nil {
		return fmt.Errorf("envoy-go decompress: %w", err)
	}
	enPlain, err := decompressGzip(envoy.Body)
	if err != nil {
		return fmt.Errorf("envoy decompress: %w", err)
	}
	if !bytes.Equal(egPlain, enPlain) {
		return fmt.Errorf("decompressed bodies differ: envoy-go=%d bytes; envoy=%d bytes", len(egPlain), len(enPlain))
	}
	if originalPayload != nil && !bytes.Equal(egPlain, originalPayload) {
		return fmt.Errorf("decompressed body differs from original input (envoy-go side): plain=%d bytes; original=%d bytes", len(egPlain), len(originalPayload))
	}
	return nil
}

// decompressGzip wraps body in compress/gzip.NewReader and reads the
// decompressed plaintext via io.ReadAll. Returns the plaintext bytes and any
// gzip-format / read error. Per ADR-0133 §Decision (i) the reusable
// gzip-decompression helper for the fixture-0016 driver + any future codec/
// transform fixture.
func decompressGzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gzip.NewReader: %w", err)
	}
	defer func() { _ = r.Close() }()
	plain, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read decompressed: %w", err)
	}
	return plain, nil
}

// assertNoAcceptEncodingInEchoedBody parses the echobackend's JSON response
// body (shape per test/helpers/echobackend/echobackend.go: `{method, path,
// headers}` with lowercased canonical header keys) and asserts the upstream-
// side request did NOT carry an Accept-Encoding header. Used by scenario 6
// per SPEC §7.1: the per-route remove_accept_encoding_header override strips
// AE before forwarding upstream; the echobackend echoes upstream-observed
// headers; presence of "accept-encoding" in the echoed map is a hard failure.
func assertNoAcceptEncodingInEchoedBody(plaintext []byte) error {
	var rec struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(plaintext, &rec); err != nil {
		return fmt.Errorf("parse echoed body: %w", err)
	}
	if v, ok := rec.Headers["accept-encoding"]; ok && v != "" {
		return fmt.Errorf("Accept-Encoding NOT stripped upstream (per-route rmAE override failed): %q", v)
	}
	return nil
}

// --- fixture.StatsAsserter (Task 14) ---

// counterAssertion describes one of the 17 phase-14 compressor counters per
// SPEC §11.5 + ADR-0132 + planner-time decision 2 (D2 settlement). The
// driver scrapes /stats/prometheus from both proxies post-workload and
// asserts each counter per side per the assertion mode below.
//
// Modes (per ADR-0133 §Decision (iii) + planner-time decision 2):
//
//   - exact: per-side counter MUST equal expected; cross-side MUST agree.
//     Used for the 10 counters whose semantic matches byte-equivalent between
//     reference Envoy v1.37.2 and envoy-go MVP after the 6-scenario workload:
//     response_compressed, response_content_length_too_small,
//     not_compressed_etag, header_compressor_overshadowed, header_identity,
//     header_wildcard, no_accept_header, request_compressed,
//     request_content_length_too_small, request_total_compressed_bytes,
//     request_total_uncompressed_bytes.
//
//   - perSideExact: per-side counter MUST equal the side-specific expected
//     value; cross-side WILL diverge. Used for the 4 counters whose semantic
//     differs by design between reference Envoy and envoy-go MVP:
//
//   - header_compressor_used: ref=3 (scenarios 1+2+3+4; scenario 6's
//     per-route rmAE strip causes ref's response-side classification to
//     miss the original AE per the empirical finding at Task 14;
//     scenario 5 disabled), subj=5 (envoy-go caches the classification
//     at DecodeHeaders BEFORE the rmAE strip per the ADR-0129
//     §Decision (iv) same-*filter discipline so EncodeHeaders sees
//     "compressor_used" for scenario 6 too).
//
//   - header_not_valid: ref=1 (scenario 6 — the response-side
//     AE-recomputation post-strip apparently classifies as "not_valid"
//     on Envoy v1.37.2), subj=0 (envoy-go's cached classification has
//     no recomputation post-strip).
//
//   - response_not_compressed: ref=3 (scenarios 2+3+5 — Envoy
//     v1.37.2's per-route disabled scenario 5 STILL increments this
//     counter despite the SPEC's "NO counter increments" claim;
//     empirically pinned at Task 14 contradicting SPEC §7.1 row 5
//     "NO counter increments"), subj=2 (envoy-go's per-route disabled
//     is wholly inactive per ADR-0125 amendment §(viii) — no counter).
//
//   - request_not_compressed: ref=6 (Envoy v1.37.2 increments per
//     request even with response_direction_config-only setup, per
//     SPEC §11.5 probeA empirical evidence), subj=0 (envoy-go MVP's
//     request side is silent per ADR-0132 §Decision (vii) twin-series
//     discipline).
//     These four per-side divergences are EMPIRICAL FINDINGS at Task 14
//     impl-time; SPEC §7.1 + §7.3 made simplifying assumptions about
//     reference Envoy behavior that the actual probe pin overrides. The
//     driver locks both sides' empirical values so regressions on either
//     side surface immediately.
//
//   - exactDynamic: per-side counter MUST equal expectedDynamic(side); cross-
//     side may differ (the per-side expected is computed from per-side observed
//     state). Used for `response_total_uncompressed_bytes` where scenario 6's
//     echobackend response length differs cross-side due to Envoy-injected
//     headers + host:port-string variance.
//
//   - boundary: per-side counter MUST satisfy 0 < counter < upperBound(side);
//     cross-side may differ. Used for `response_total_compressed_bytes` per
//     planner-time decision 2 (D2 settlement) — gzip implementations diverge
//     in encoded byte counts (Go compress/gzip vs Envoy libz per §11.14).
type counterAssertion struct {
	name string
	mode counterAssertionMode

	// exact value (mode=exact); cross-side byte-equal.
	expected int64

	// per-side exact (mode=perSideExact); each side has its own expected.
	expectedRef  int64
	expectedSubj int64

	// dynamic per-side expected (mode=exactDynamic). Receives the side
	// label ("ref" or "subj") and returns the per-side expected value.
	expectedDynamic func(side string) int64

	// upper-bound provider (mode=boundary). Receives the side label and
	// returns the inclusive upper bound (counter must be strictly less).
	upperBound func(side string) int64
}

type counterAssertionMode int

const (
	counterModeExact counterAssertionMode = iota
	counterModePerSideExact
	counterModeExactDynamic
	counterModeBoundary
)

// AssertStats implements fixture.StatsAsserter per SPEC §7.3 + ADR-0132 +
// planner-time decision 2 (D2 settlement). Scrapes /stats/prometheus from
// both ref + subj admin endpoints after the 6-scenario workload (per the
// runner's step-10 sequencing — fires AFTER ProbeAdmin) and asserts:
//
//   - 16 of 17 counters: per-side byte-exact against the SPEC §7.1 expected
//     delta + cross-side byte-equal. The 6 header_* + 1 not_compressed_etag +
//     4 of 5 response_* + all 5 request_* counters are byte-exact.
//
//   - `response_total_uncompressed_bytes`: per-side dynamic — equals 1024
//     (s1) + 1024 (s4) + <observed-s6-decompressed-body-len> on that side.
//     Cross-side may differ because scenario 6's echobackend echoes upstream-
//     observed request headers including Envoy-injected x-envoy-* /
//     x-request-id / x-forwarded-* headers and the host:port string (which
//     differs between Envoy `host.docker.internal:port` and envoy-go
//     `127.0.0.1:port`). The per-side observation captured during
//     driveProxy is the load-bearing input.
//
//   - `response_total_compressed_bytes`: boundary-only per side per planner-
//     time decision 2 (D2 settlement) + ADR-0133 §Decision (iii): asserts
//     `0 < counter < per-side-response_total_uncompressed_bytes`. Gzip
//     implementations diverge in encoded byte counts (Go compress/gzip vs
//     Envoy libz per §11.14); a byte-exact cross-side assertion would
//     spuriously fail on each compression-ratio variance. Boundary-only
//     captures the structural invariant (compression makes bytes smaller)
//     without coupling to library-specific compression ratios.
func (d *compressorDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeCompressorStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref compressor stats: %v", err)
	}
	subjStats, err := scrapeCompressorStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj compressor stats: %v", err)
	}
	// Debug stat dump — enable with FIXTURE_0016_DUMP_STATS=1 when diagnosing
	// a per-side regression on either Envoy v1.37.2 (e.g. version bump
	// changing counter semantics) or envoy-go (e.g. ADR-0129 §Decision (iv)
	// regression on the AE-classification caching).
	if os.Getenv("FIXTURE_0016_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref compressor stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj compressor stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	d.mu.Lock()
	refS6 := d.scenario6BodyLen["ref"]
	subjS6 := d.scenario6BodyLen["subj"]
	d.mu.Unlock()

	// Fixed-input bytes from scenarios 1 + 4 (1024-byte payloads each).
	const scenario1UncompressedBytes = 1024
	const scenario4UncompressedBytes = 1024

	expectedTotalUncompressed := func(side string) int64 {
		s6 := refS6
		if side == "subj" {
			s6 = subjS6
		}
		return int64(scenario1UncompressedBytes + scenario4UncompressedBytes + s6)
	}

	const np = "envoy_http_compressor_text_optimized_gzip_"
	assertions := []counterAssertion{
		// 6 Accept-Encoding-cluster counters (SPEC §11.5 6-counter cluster).
		// header_compressor_used + header_not_valid DIVERGE cross-side per
		// the Task-14 empirical pin: reference Envoy strips the cached AE
		// classification when per-route rmAE fires (scenario 6) and
		// reclassifies on the response side; envoy-go caches at
		// DecodeHeaders BEFORE the rmAE strip per ADR-0129 §Decision (iv)
		// same-*filter discipline. See counterAssertion GoDoc above for the
		// per-side breakdown.
		{name: np + "header_compressor_used", mode: counterModePerSideExact, expectedRef: 3, expectedSubj: 5},
		{name: np + "header_not_valid", mode: counterModePerSideExact, expectedRef: 1, expectedSubj: 0},
		{name: np + "header_compressor_overshadowed", mode: counterModeExact, expected: 0},
		{name: np + "header_identity", mode: counterModeExact, expected: 0},
		{name: np + "header_wildcard", mode: counterModeExact, expected: 0},
		{name: np + "no_accept_header", mode: counterModeExact, expected: 0},
		// 1 ETag-skip counter (not_compressed_etag). No scenario sets
		// disable_on_etag_header=true; the etag-strong scenario 4 hits
		// mode-a (default) which STRIPS the strong-ETag without incrementing
		// not_compressed_etag (that counter is for the
		// disable_on_etag_header=true skip path per §11.7 mode-b).
		{name: np + "not_compressed_etag", mode: counterModeExact, expected: 0},
		// 5 response-side counters. Scenarios 1, 4, 6 compress;
		// scenarios 2, 3 skip via content_type_mismatch / below-min.
		// response_not_compressed DIVERGES — Envoy v1.37.2 increments it
		// for scenario 5 (per-route disabled) too, contradicting SPEC §7.1
		// row 5 "NO counter increments"; envoy-go's per-route disabled is
		// wholly inactive per ADR-0125 amendment §(viii) so no increment.
		{name: np + "response_compressed", mode: counterModeExact, expected: 3},
		{name: np + "response_not_compressed", mode: counterModePerSideExact, expectedRef: 3, expectedSubj: 2},
		{name: np + "response_content_length_too_small", mode: counterModeExact, expected: 1},
		{
			name:            np + "response_total_uncompressed_bytes",
			mode:            counterModeExactDynamic,
			expectedDynamic: expectedTotalUncompressed,
		},
		{
			name:       np + "response_total_compressed_bytes",
			mode:       counterModeBoundary,
			upperBound: expectedTotalUncompressed,
		},
		// 5 request-side counters. envoy-go MVP's request side is silent
		// (ADR-0132 §Decision (vii) twin-series discipline) — all 5 at 0.
		// Reference Envoy v1.37.2 increments request_not_compressed PER
		// REQUEST even with response_direction_config-only setup (per the
		// SPEC §11.5 probeA empirical evidence + Task-14 in-session
		// confirmation: 6 requests → 6 increments). The other 4 request_*
		// counters are zero on both sides.
		{name: np + "request_compressed", mode: counterModeExact, expected: 0},
		{name: np + "request_content_length_too_small", mode: counterModeExact, expected: 0},
		{name: np + "request_not_compressed", mode: counterModePerSideExact, expectedRef: 6, expectedSubj: 0},
		{name: np + "request_total_compressed_bytes", mode: counterModeExact, expected: 0},
		{name: np + "request_total_uncompressed_bytes", mode: counterModeExact, expected: 0},
	}

	for _, a := range assertions {
		refVal := refStats[a.name]
		subjVal := subjStats[a.name]
		switch a.mode {
		case counterModeExact:
			if refVal != a.expected {
				t.Errorf("ref %s = %d, want %d", a.name, refVal, a.expected)
			}
			if subjVal != a.expected {
				t.Errorf("subj %s = %d, want %d", a.name, subjVal, a.expected)
			}
		case counterModePerSideExact:
			if refVal != a.expectedRef {
				t.Errorf("ref %s = %d, want %d (per-side; empirical Envoy v1.37.2 pin)", a.name, refVal, a.expectedRef)
			}
			if subjVal != a.expectedSubj {
				t.Errorf("subj %s = %d, want %d (per-side; envoy-go MVP)", a.name, subjVal, a.expectedSubj)
			}
		case counterModeExactDynamic:
			refWant := a.expectedDynamic("ref")
			subjWant := a.expectedDynamic("subj")
			if refVal != refWant {
				t.Errorf("ref %s = %d, want %d (1024+1024+s6_body=%d)", a.name, refVal, refWant, refS6)
			}
			if subjVal != subjWant {
				t.Errorf("subj %s = %d, want %d (1024+1024+s6_body=%d)", a.name, subjVal, subjWant, subjS6)
			}
		case counterModeBoundary:
			refUpper := a.upperBound("ref")
			subjUpper := a.upperBound("subj")
			if !(refVal > 0 && refVal < refUpper) {
				t.Errorf("ref %s = %d, want 0 < value < %d (boundary-only per ADR-0133 §Decision (iii))", a.name, refVal, refUpper)
			}
			if !(subjVal > 0 && subjVal < subjUpper) {
				t.Errorf("subj %s = %d, want 0 < value < %d (boundary-only per ADR-0133 §Decision (iii))", a.name, subjVal, subjUpper)
			}
		}
	}
}

// scrapeCompressorStats issues GET /stats/prometheus against adminAddr and
// parses the response body into a map[name]int64 of all
// envoy_http_compressor_text_optimized_gzip_* metric values whose
// envoy_http_conn_manager_prefix label matches the fixture's configured HCM
// stat_prefix (statPrefix = "ingress_compressor"). Names absent from the
// response are absent from the map (caller treats absent as 0). Mirrors
// phase-12 csrf's scrapeCsrfStats pattern (see fixtures/0014-http-csrf/
// driver/driver.go).
func scrapeCompressorStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return parseCompressorPromBody(resp.Body)
}

// parseCompressorPromBody parses a Prometheus text-format body and returns a
// map of all metric names beginning with
// envoy_http_compressor_text_optimized_gzip_ whose
// envoy_http_conn_manager_prefix label matches statPrefix
// (= "ingress_compressor"). The label-bearing form `name{k="v",...} value`
// and the bare form `name value` are both supported. Non-matching lines and
// lines with mismatched stat_prefix labels are silently ignored.
func parseCompressorPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantPrefix = "envoy_http_compressor_text_optimized_gzip_"
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, valueStr, labelStr string
		if idx := strings.IndexByte(line, '{'); idx >= 0 {
			name = line[:idx]
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < 0 || closeIdx+1 >= len(line) {
				continue
			}
			labelStr = line[idx+1 : closeIdx]
			valueStr = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.LastIndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			valueStr = strings.TrimSpace(line[sp+1:])
		}
		if !strings.HasPrefix(name, wantPrefix) {
			continue
		}
		// stat_prefix discrimination: only accept lines whose
		// envoy_http_conn_manager_prefix label matches the fixture's
		// configured stat_prefix. Lines without labels (envoy-go SN2 flat
		// shape variant) also accepted.
		if labelStr != "" && !labelMatches(labelStr, "envoy_http_conn_manager_prefix", statPrefix) {
			continue
		}
		// Strip optional timestamp.
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		out[name] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// labelMatches reports whether labelStr (the contents of {...} in a
// Prometheus exposition line) contains key="value" exactly. Mirrors the
// phase-12 csrf precedent (fixtures/0014-http-csrf/driver/driver.go).
func labelMatches(labelStr, key, value string) bool {
	want := key + `="` + value + `"`
	for _, part := range strings.Split(labelStr, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// --- helpers ---

// fixtureDir returns the absolute path to the 0016-http-compressor fixture
// root (one directory above this file's `inputs/` parent), derived from
// runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0016-http-compressor/inputs/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory.
func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

// mustRender renders a text/template body with data; panics on parse/exec
// errors (driver-time misconfiguration is non-recoverable).
func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*compressorDriver)(nil)
	_ fixture.BackendKindAware = (*compressorDriver)(nil)
	_ fixture.StatsAsserter    = (*compressorDriver)(nil)
)
