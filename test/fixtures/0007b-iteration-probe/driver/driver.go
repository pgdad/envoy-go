// Package driver registers the 0007b-iteration-probe fixture with the
// differential runner. This is the project's first envoy-go-only structural
// fixture (RequiresReference: false per SPEC §7.4) — there is no reference
// Envoy in this fixture because the envoy.filters.http.envoy_go_test type
// URL is hand-rolled (Task 19's internal/filter/http/envoygotest/proto/
// envoygotest.pb.go) and not registered in upstream go-control-plane or
// upstream Envoy.
//
// The driver issues 8 sequential H1 requests, one per
// x-envoy-go-test-mode value, and asserts each response against an
// embedded per-mode expectation table per SPEC §7.3.
//
// 8-mode coverage (SPEC §7.3 table):
//
//  1. continue                    — pass-through
//  2. stop-and-resume-headers     — DecodeHeaders StopIteration + async resume (10ms tick)
//  3. stop-and-buffer-data        — DecodeData StopIterationAndBuffer + async resume
//  4. local-reply-decode          — SendLocalReply 418 on DecodeHeaders
//  5. local-reply-decode-data     — SendLocalReply 418 on DecodeData
//  6. modify-encode-headers       — EncodeHeaders sets x-envoy-go-test-encoded: yes
//  7. modify-encode-data          — EncodeData replaces body with "MODIFIED\n" prefix (in-place copy)
//  8. stop-trailers               — DecodeTrailers StopIteration + async resume
//
// All 8 modes also exercise the universal per-route count echo: every response
// gets x-envoy-go-test-route-count: 7 from the encode-side filter (the
// per-route count config is set to 7 in subjectTmpl).
//
// Mode 8 disposition: the chain framework's RunDecodeTrailers path is wired,
// but HCM dispatch (internal/filter/hcm/connection.go for H1) does NOT
// currently invoke RunDecodeTrailers from the H1 read loop — H1 chunked
// transfer-encoding trailer parsing was deferred at Task 15 close-out (see
// PROGRESS Task 15 deviation note). The probe filter's stop-trailers branch
// therefore does NOT fire end-to-end on H1 requests; the request flows
// through normally as if mode were "continue" (the filter's DecodeHeaders
// captures the mode but the trailers callback is never invoked). The
// expectation table reflects this honestly: mode 8's expected behavior is
// 200 + backend body + route-count header (same as mode 1), not a delayed
// resume. The unit test (filter_test.go: TestEnvoyGoTest_ModeStopTrailers)
// directly drives chain.RunDecodeTrailers to exercise the probe's branch
// at unit-test scope; the differential fixture covers only what the H1
// dispatch surface invokes.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0007b-iteration-probe"

// subjListenerName is the listener name in the subject bootstrap. The runner
// reads the corresponding ready-sentinel-published address via
// subj.ListenerAddr(d.SubjectListenerName()).
const subjListenerName = "l_h1"

// perRouteCount is the per-route count config interpolated into the subject
// bootstrap's typed_per_filter_config[envoy.filters.http.envoy_go_test] for
// route /. The probe filter echoes this on every response as
// x-envoy-go-test-route-count: <N>. Pinned to 7 per SPEC §7.3.
const perRouteCount = 7

// fixedBackendBody is what the fixture-0007b backend returns when the
// request body is empty (GET requests). Mirrors backends/main.go's
// `fixedBody` constant; duplicated here so the driver-side expectations are
// self-contained. 8 bytes intentionally — see the mode 7 entry in the
// embedded expectation table for why.
const fixedBackendBody = "backend\n"

func init() {
	fixture.RegisterFixture(fixtureName, &iterationProbeDriver{})
}

type iterationProbeDriver struct{}

// BackendCount returns 1: a single echo backend is enough for the 8-mode
// matrix. The 8 modes do not exercise distribution — they exercise the
// iteration-protocol state machine.
func (iterationProbeDriver) BackendCount() int { return 1 }

// BackendKind selects the fixture-0007b local subprocess backend
// (HTTPEchoBody) which echoes the request body if non-empty, else returns
// a fixed "backend\n" body. See test/differential/fixture/fixture.go's
// HTTPEchoBody documentation.
func (iterationProbeDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEchoBody }

// RequiresReference returns false — fixture 0007b is envoy-go-only.
// Implements fixture.ReferenceLessFixture per SPEC §7.4. The runner
// (test/differential/runner_test.go) reads this and short-circuits to
// runReferenceLessFixture, which spawns only the subject and runs
// DriveSubject + the optional SubjectAsserter. No reference Envoy
// container is spawned; no byte-stream comparison runs; no admin diff fires.
func (iterationProbeDriver) RequiresReference() bool { return false }

func (iterationProbeDriver) SubjectListenerName() string { return subjListenerName }

// ReferenceListenerPort + ReferenceBootstrap satisfy the fixture.Driver
// interface but are NEVER invoked by the runner for reference-less fixtures
// (the runner short-circuits before the reference-spawn branch). They
// return zero / empty defensively so that any future runner refactor that
// inadvertently calls them on a reference-less fixture surfaces immediately
// as a misconfigured-bootstrap error rather than silently using stale state.
func (iterationProbeDriver) ReferenceListenerPort() int        { return 0 }
func (iterationProbeDriver) ReferenceBootstrap(_ []int) string { return "" }
func (iterationProbeDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, nil
}

func (iterationProbeDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	// Argument order MUST match the %d positions in subjectTmpl:
	//   1: admin socket_address.port_value     ← subjAdminPort
	//   2: listener socket_address.port_value  ← subjListenerPort
	//   3: typed_per_filter_config.count       ← perRouteCount
	//   4: cluster endpoint socket_address.port_value ← backendPorts[0]
	// Earlier mistake: backendPorts[0] and perRouteCount were swapped, which
	// templated the route-count to the backend port (causing the encode-side
	// route-count header to surface as the dynamic backend port number, e.g.
	// 34859) and templated the cluster endpoint port to 7 (causing every
	// upstream request to fail with 503 because nothing was listening on :7).
	// Pinned by TestModeProbes_OrderMatchesExpectations + a manual
	// envoy-go subprocess run as part of the Task 22 sanity check.
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, perRouteCount, backendPorts[0])
}

// DriveSubject issues 8 sequential H1 round-trips against addr — one per
// x-envoy-go-test-mode value — and returns a deterministic byte stream
// encoding (status, mode-relevant headers, body) per request. The
// SubjectAsserter (AssertSubject) compares the stream against the embedded
// per-mode expectation table.
//
// The byte stream is also logged on Drive failure so a future maintainer
// has an audit trail of what envoy-go produced when the assertion fired.
func (iterationProbeDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	var out bytes.Buffer
	for i, p := range modeProbes {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, p.method, "/", p.headers, p.body)
		if err != nil {
			return out.Bytes(), fmt.Errorf("request %d (mode=%s): %w", i+1, p.mode, err)
		}
		out.WriteString(encodeProbe(i+1, p, resp, body))
	}
	return out.Bytes(), nil
}

// AssertSubject asserts each mode's response against the embedded expectation
// table per SPEC §7.3. Implements fixture.SubjectAsserter so the runner
// invokes this in-band per SPEC §12 #8 (the fixture-internal-assertion
// pattern that 05.2 / 06.1 / 06.2 / 07a all follow).
//
// The captured subjBytes argument is the byte stream returned by
// DriveSubject (one encoded probe per mode, in the order driven). We
// re-issue the 8 round-trips here? No — DriveSubject already drove and
// captured. AssertSubject is a pure analysis step on subjBytes. To keep the
// assertion exhaustive and per-mode, the driver re-issues each request
// here against the live subject as well — but that would double-drive. The
// settled pattern (mirrors 0007a's encodeProbe + driver_test.go pinning):
// AssertSubject does NOT re-drive; it inspects subjBytes for per-mode
// substring presence/absence. The probe-byte format encodes status +
// header presence + body — sufficient for the per-mode shape table.
func (iterationProbeDriver) AssertSubject(t fixture.TB, subjBytes []byte) {
	t.Helper()
	out := string(subjBytes)
	for i, exp := range modeExpectations {
		// Each per-mode block in the byte stream starts with "=== request <n> mode=<mode>\n".
		header := fmt.Sprintf("=== request %d mode=%s", i+1, exp.mode)
		if !strings.Contains(out, header) {
			t.Errorf("mode %d (%s): probe header %q not found in subj stream", i+1, exp.mode, header)
			continue
		}
		// Locate this probe's block in the stream so substring assertions
		// don't accidentally hit a neighbor probe.
		blockStart := strings.Index(out, header)
		blockEnd := len(out)
		nextHeader := fmt.Sprintf("=== request %d mode=", i+2)
		if i+1 < len(modeExpectations) {
			if idx := strings.Index(out[blockStart:], nextHeader); idx >= 0 {
				blockEnd = blockStart + idx
			}
		}
		block := out[blockStart:blockEnd]

		// Status assertion.
		statusLine := fmt.Sprintf("status: %d", exp.status)
		if !strings.Contains(block, statusLine) {
			t.Errorf("mode %d (%s): expected %q; got block:\n%s", i+1, exp.mode, statusLine, block)
		}

		// Per-mode header assertions.
		for _, hdr := range exp.headersPresent {
			if !strings.Contains(block, hdr) {
				t.Errorf("mode %d (%s): expected header %q in block:\n%s", i+1, exp.mode, hdr, block)
			}
		}
		for _, hdr := range exp.headersAbsent {
			if strings.Contains(block, hdr) {
				t.Errorf("mode %d (%s): unexpected header %q in block:\n%s", i+1, exp.mode, hdr, block)
			}
		}

		// Body assertion (Go-quoted form so trailing-newline / non-printable
		// divergences surface as visible failures).
		bodyLine := fmt.Sprintf("body: %q", exp.body)
		if !strings.Contains(block, bodyLine) {
			t.Errorf("mode %d (%s): expected %q in block:\n%s", i+1, exp.mode, bodyLine, block)
		}
	}
}

// ProbeAdmin is required by the fixture.Driver interface but the
// reference-less runner branch (runReferenceLessFixture) does not invoke
// it. Returns nil/nil defensively so any future runner refactor that
// inadvertently calls ProbeAdmin on a reference-less fixture surfaces as
// an empty-bytes diff (loud, not silent).
func (iterationProbeDriver) ProbeAdmin(context.Context, string, string) (refBytes, subjBytes []byte, err error) {
	return nil, nil, nil
}

// probe captures the per-request fields the driver issues against the
// subject. method + headers + body are passed straight to
// helpers.HTTPRoundTrip; mode is captured here so the encodeProbe output
// can label the block with the mode dispatch value (for stream-grep
// audits).
type probe struct {
	mode    string
	method  string
	headers http.Header
	body    []byte
}

// modeProbes is the fixed 8-request workload per SPEC §7.3, in the order
// the driver issues them. The order is the SPEC table order: continue,
// stop-and-resume-headers, stop-and-buffer-data, local-reply-decode,
// local-reply-decode-data, modify-encode-headers, modify-encode-data,
// stop-trailers.
var modeProbes = []probe{
	{
		mode:    "continue",
		method:  "GET",
		headers: modeHeader("continue"),
		body:    nil,
	},
	{
		mode:    "stop-and-resume-headers",
		method:  "GET",
		headers: modeHeader("stop-and-resume-headers"),
		body:    nil,
	},
	{
		// POST so DecodeData fires and the buffer-and-resume path is
		// exercised end-to-end.
		mode:    "stop-and-buffer-data",
		method:  "POST",
		headers: modeHeader("stop-and-buffer-data"),
		body:    []byte("payload"),
	},
	{
		mode:    "local-reply-decode",
		method:  "GET",
		headers: modeHeader("local-reply-decode"),
		body:    nil,
	},
	{
		// POST so DecodeData fires.
		mode:    "local-reply-decode-data",
		method:  "POST",
		headers: modeHeader("local-reply-decode-data"),
		body:    []byte("payload"),
	},
	{
		mode:    "modify-encode-headers",
		method:  "GET",
		headers: modeHeader("modify-encode-headers"),
		body:    nil,
	},
	{
		// GET so the backend returns the fixed 8-byte "backend\n" body.
		// EncodeData copies "MODIFIED\n" (9 bytes) into the 8-byte slice
		// → first 8 bytes "MODIFIED" land verbatim; the trailing \n is
		// truncated. SPEC §7.3 row 7 expected body is "MODIFIED" — the
		// short-slice-aware behavior pinned in
		// internal/filter/http/envoygotest/filter.go EncodeData.
		mode:    "modify-encode-data",
		method:  "GET",
		headers: modeHeader("modify-encode-data"),
		body:    nil,
	},
	{
		mode:    "stop-trailers",
		method:  "GET",
		headers: modeHeader("stop-trailers"),
		body:    nil,
	},
}

// modeExpectation is the embedded per-mode expectation table per SPEC §7.3.
// One entry per modeProbes entry, in the same order.
type modeExpectation struct {
	mode           string
	status         int
	body           string
	headersPresent []string // each substring must appear in the encoded probe block
	headersAbsent  []string // each substring must NOT appear in the encoded probe block
}

// modeExpectations is the 8-mode embedded expectation table.
//
// Per-mode rationale:
//
//  1. continue          — pure pass-through. Backend returns "backend\n" on GET.
//     Probe filter EncodeHeaders fires and adds the route-count header.
//  2. stop-and-resume-headers — DecodeHeaders returns StopIteration; goroutine resumes
//     after 10ms via dcb.ContinueDecoding. The chain framework's
//     parkDecode loop yields and the request flows through normally.
//     Same expected response as mode 1 (resume is transparent to
//     the wire).
//  3. stop-and-buffer-data — DecodeData returns DataStopIterationAndBuffer; resumes
//     after 10ms. The framework buffers the request body and
//     re-runs from the next filter on resume. POST body
//     "payload" reaches the backend; the backend echoes it.
//  4. local-reply-decode — SendLocalReply(418, "i am a teapot\n") on DecodeHeaders.
//     The chain transitions to encode synchronously inside
//     beginLocalReply; the encode chain runs in REVERSE
//     declaration order from filter[len-1], which means the
//     probe filter's EncodeHeaders fires AFTER the SendLocalReply
//     and sets x-envoy-go-test-route-count: 7 on the synthesized
//     response. Body is "i am a teapot\n" verbatim from the
//     SendLocalReply call.
//  5. local-reply-decode-data — same as mode 4 but the SendLocalReply fires from
//     DecodeData (after DecodeHeaders has already passed through).
//     The POST body is consumed before the SendLocalReply
//     synthesizes — same wire shape as mode 4 (418 +
//     "i am a teapot\n" + route-count header).
//  6. modify-encode-headers — EncodeHeaders sets x-envoy-go-test-encoded: yes.
//     The route-count header also lands per the universal
//     encode-side branch in the probe filter.
//  7. modify-encode-data — EncodeData replaces body bytes in-place via
//     copy(data, "MODIFIED\n"). The backend's GET body is
//     "backend\n" (8 bytes); copy writes the first 8 bytes
//     of "MODIFIED\n" → "MODIFIED" (no trailing newline,
//     the 9th byte of "MODIFIED\n" is dropped). Expected
//     body: "MODIFIED" (8 bytes). The route-count header
//     also lands per the universal encode-side branch.
//  8. stop-trailers     — DecodeTrailers returns TrailersStopIteration in the
//     probe filter, but H1 HCM dispatch does NOT currently
//     invoke RunDecodeTrailers (the H1 chunked-T-E trailer
//     parsing was deferred at Task 15 close-out per
//     PROGRESS notes). The probe filter's DecodeTrailers
//     branch therefore never fires end-to-end on this H1
//     fixture; the request flows through as if mode were
//     "continue". This is documented honestly here per
//     the Task 22 prompt's mode 8 caveat.
var modeExpectations = []modeExpectation{
	{
		mode:           "continue",
		status:         200,
		body:           fixedBackendBody, // "backend\n"
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		mode:           "stop-and-resume-headers",
		status:         200,
		body:           fixedBackendBody,
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		mode:           "stop-and-buffer-data",
		status:         200,
		body:           "payload", // backend echoes the POST body
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		mode:           "local-reply-decode",
		status:         418,
		body:           "i am a teapot\n",
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		mode:           "local-reply-decode-data",
		status:         418,
		body:           "i am a teapot\n",
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		mode:   "modify-encode-headers",
		status: 200,
		body:   fixedBackendBody,
		headersPresent: []string{
			"x-envoy-go-test-route-count: 7",
			"x-envoy-go-test-encoded: yes",
		},
	},
	{
		mode:   "modify-encode-data",
		status: 200,
		// "backend\n" (8 bytes) ← copy "MODIFIED\n" (9 bytes) ⇒ first 8
		// bytes "MODIFIED" land; trailing \n truncated. Pinned to the
		// EncodeData implementation in
		// internal/filter/http/envoygotest/filter.go.
		body:           "MODIFIED",
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
	{
		// Mode 8 disposition: H1 HCM dispatch does NOT invoke
		// RunDecodeTrailers, so the probe's stop-trailers branch never
		// fires end-to-end on this fixture. Expected response is the
		// continue-mode wire shape (200 + backend body + route-count).
		// See the package-level doc.go for the full disposition note.
		mode:           "stop-trailers",
		status:         200,
		body:           fixedBackendBody,
		headersPresent: []string{"x-envoy-go-test-route-count: 7"},
	},
}

// modeHeader builds a one-entry http.Header carrying the mode dispatch value.
func modeHeader(mode string) http.Header {
	h := http.Header{}
	h.Set("x-envoy-go-test-mode", mode)
	return h
}

// kv is a (lowercased name, value) header pair used by encodeProbe to
// emit a deterministic header listing in the per-probe block. Lifted to
// package scope so the sort helper can take a typed slice.
type kv struct{ name, value string }

// encodeProbe renders one request's response into the deterministic byte
// stream form the SubjectAsserter operates on. The form is:
//
//	=== request <n> mode=<mode>
//	status: <code>
//	header: <name>: <value>   (one line per response header — sorted)
//	body: <quoted>
//
// Bodies are Go-quoted (%q) so non-printable bytes and trailing-newline
// divergences are visible. Headers are emitted lowercase + sorted so the
// stream is deterministic regardless of map-iteration order.
func encodeProbe(n int, p probe, resp *http.Response, body []byte) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== request %d mode=%s\n", n, p.mode)
	fmt.Fprintf(&sb, "status: %d\n", resp.StatusCode)

	// Sorted headers for stream determinism.
	var headers []kv
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		for _, v := range vv {
			headers = append(headers, kv{name: lk, value: v})
		}
	}
	sortHeaders(headers)
	for _, h := range headers {
		fmt.Fprintf(&sb, "header: %s: %s\n", h.name, h.value)
	}

	fmt.Fprintf(&sb, "body: %q\n", string(body))
	return sb.String()
}

// sortHeaders sorts in-place by (name, value) lexicographic order. Tiny
// insertion sort — the slice is small (<10 entries per response in 0007b)
// so quadratic is fine and avoids an extra import.
func sortHeaders(s []kv) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0; j-- {
			if s[j].name < s[j-1].name || (s[j].name == s[j-1].name && s[j].value < s[j-1].value) {
				s[j], s[j-1] = s[j-1], s[j]
			} else {
				break
			}
		}
	}
}

// Compile-time checks: driver implements all required and selected optional
// interfaces. The interface set is the project's first reference-less
// structural fixture (RequiresReference: false + SubjectAsserter).
var (
	_ fixture.Driver               = (*iterationProbeDriver)(nil)
	_ fixture.BackendKindAware     = (*iterationProbeDriver)(nil)
	_ fixture.ReferenceLessFixture = (*iterationProbeDriver)(nil)
	_ fixture.SubjectAsserter      = (*iterationProbeDriver)(nil)
)

// subjectTmpl is the bootstrap template for the envoy-go subject. fmt args
// (in order):
//
//	1: admin port
//	2: subject listener port
//	3: backend port
//	4: per-route count
//
// The route table is a single prefix:/ → c_backend cluster route with the
// envoy_go_test typed_per_filter_config attached for the route-count echo.
// http_filters: [envoy.filters.http.envoy_go_test, envoy.filters.http.router]
// per SPEC §4.3 + §7.3 + ADR-0074.
const subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h1
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.envoy_go_test:
                              "@type": type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTestPerRoute
                              count: %d
                http_filters:
                  - name: envoy.filters.http.envoy_go_test
                    typed_config:
                      "@type": type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTest
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
