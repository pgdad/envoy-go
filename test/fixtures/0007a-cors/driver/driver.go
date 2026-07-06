// Package driver registers the 0007a-cors fixture with the differential
// runner. This is the project's first cors-filter differential — it
// asserts wire-equivalent CORS response shape between envoy-go and
// reference Envoy v1.37.2 across a 4-request workload exercising the
// preflight-allowed / preflight-disallowed / actual-allowed / actual-no-
// origin matrix per SPEC §11.2 (probes a/b/c/d).
//
// Integration shape (SPEC §7.2 driver outline):
//
//  1. SubjectConfig templates the envoy-go bootstrap with the 1 backend
//     port; ReferenceBootstrap templates the reference bootstrap with the
//     same port via host.docker.internal (ADR-0010 STRICT_DNS).
//
//  2. Routes:
//     - /permissive: route to backend cluster; per-route cors policy
//     allowing https://example.test with the §11.2 allow-methods /
//     allow-headers / max-age / expose-headers / allow-credentials.
//     - /strict: direct_response 405 "method not allowed\n"; per-route
//     cors policy with the very-restrictive https://only.test (no
//     request origin matches).
//
//     The /strict 405 is a direct_response (not router fallthrough) per
//     this fixture's design deviation — envoy-go's router does not
//     reject OPTIONS by default the way reference Envoy v1.37.2 does in
//     the §11.2 probe-(b) empirical pin (which used a routed cluster
//     and observed 405). Direct_response 405 makes both sides 405 OPTIONS
//     /strict deterministically without depending on undocumented
//     router-side OPTIONS handling. Request 4 is therefore moved to
//     /permissive (no-Origin) to preserve the 4-request matrix; this
//     still maps to SPEC §11.2 probe (d) which uses the same route as
//     probes (a)/(c). See README.md for the full deviation rationale.
//
//  3. DriveReference / DriveSubject issue 4 sequential H1 GET/OPTIONS
//     round-trips with the §11.2 request shape (Origin / Access-Control-
//     Request-Method / Access-Control-Request-Headers as appropriate).
//     The driver returns a deterministic byte stream encoding status +
//     body + sorted-CORS-headers per request; the runner CompareBytes
//     pass enforces equivalence. CORS headers are sorted alphabetically
//     to side-step the actual-request 3-header order divergence pinned in
//     types.go (envoy-go's ReconcileOrderedHeaders alphabetises net-new
//     keys; reference Envoy emits source-order). Set-equality on CORS
//     headers is what §7.2 (4) requires; sorting is the carrier.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint.
package driver

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0007a-cors"

// refContainerListenerPort is the in-container port the reference Envoy
// listens on. Pinned to 15007 to match envoy.yaml; the runner's
// StartReferenceProxy publishes it to a host-side ephemeral port that the
// driver dials via the addr argument to DriveReference.
const refContainerListenerPort = 15007

func init() {
	fixture.RegisterFixture(fixtureName, &corsDriver{})
}

type corsDriver struct{}

// BackendCount returns 1: a single fixed-body backend serving "hello\n"
// is sufficient for actual-request body equivalence (no RR distribution
// is exercised in this fixture; cors gating + route selection are the
// behaviors under test).
func (corsDriver) BackendCount() int { return 1 }

// BackendKind selects the cors fixture's local subprocess backend
// (HTTPHello) which always returns 200 + "hello\n".
func (corsDriver) BackendKind() fixture.BackendKind { return fixture.HTTPHello }

func (corsDriver) SubjectListenerName() string { return "l_http" }
func (corsDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (corsDriver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl, backendPorts[0])
}

func (corsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0])
}

func (corsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}
func (corsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// drive issues 4 sequential H1 round-trips against addr and returns a
// deterministic byte-stream encoding of (status, body, sorted-CORS-headers)
// per request. The runner's CompareBytes pass enforces equivalence between
// reference Envoy and envoy-go on this stream.
//
// The four probes correspond to SPEC §11.2 probes (a)/(b)/(c)/(d):
//   - probe (a): OPTIONS /permissive Origin=example.test ACR-Method=POST
//   - probe (b): OPTIONS /strict     Origin=other.test   ACR-Method=POST
//   - probe (c): GET     /permissive Origin=example.test
//   - probe (d): GET     /permissive (no Origin)
//
// On the encode side, CORS headers (names with the access-control- prefix)
// are sorted alphabetically and value-included; non-CORS headers are
// dropped from the encoded stream because Date / Server / Content-Length /
// Content-Type / x-envoy-* differ between sides per the differential
// allow-list. Body bytes are included verbatim only on the 200/405 paths;
// body equivalence is implicit when the encoded body lines match.
func drive(ctx context.Context, addr string) ([]byte, error) {
	requests := []probe{
		{
			tag:    "probe-a OPTIONS /permissive (allowed origin)",
			method: "OPTIONS",
			path:   "/permissive",
			headers: http.Header{
				"Origin":                         []string{"https://example.test"},
				"Access-Control-Request-Method":  []string{"POST"},
				"Access-Control-Request-Headers": []string{"x-foo,x-bar"},
			},
		},
		{
			tag:    "probe-b OPTIONS /strict (disallowed origin)",
			method: "OPTIONS",
			path:   "/strict",
			headers: http.Header{
				"Origin":                        []string{"https://other.test"},
				"Access-Control-Request-Method": []string{"POST"},
			},
		},
		{
			tag:    "probe-c GET /permissive (allowed origin)",
			method: "GET",
			path:   "/permissive",
			headers: http.Header{
				"Origin": []string{"https://example.test"},
			},
		},
		{
			tag:     "probe-d GET /permissive (no Origin)",
			method:  "GET",
			path:    "/permissive",
			headers: nil,
		},
	}

	var out strings.Builder
	for i, p := range requests {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, p.method, p.path, p.headers, nil)
		if err != nil {
			return nil, fmt.Errorf("request %d (%s): %w", i+1, p.tag, err)
		}
		out.WriteString(encodeProbe(i+1, p, resp, body))
	}
	return []byte(out.String()), nil
}

type probe struct {
	tag     string
	method  string
	path    string
	headers http.Header
}

// encodeProbe renders one request's response into the deterministic byte-
// stream form the runner CompareBytes pass operates on. The form is:
//
//	=== request <n> <tag>
//	status: <code>
//	cors-headers (sorted):
//	  <name>: <value>   (one line per access-control-* header)
//	  ...
//	  (none)            (when no cors headers)
//	body: <quoted>
//
// Bodies are Go-quoted (%q) so non-printable bytes are visible in any
// future regression and so trailing newline differences are obvious.
// Non-CORS headers are intentionally NOT emitted: Date / Server /
// Content-Length / x-envoy-* / x-request-id all diverge by allow-list
// (helpers.PhaseFourHTTPAllowList).
func encodeProbe(n int, p probe, resp *http.Response, body []byte) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== request %d %s\n", n, p.tag)
	fmt.Fprintf(&sb, "status: %d\n", resp.StatusCode)

	type kv struct{ name, value string }
	var corsHeaders []kv
	for k, vv := range resp.Header {
		lk := strings.ToLower(k)
		if !strings.HasPrefix(lk, "access-control-") {
			continue
		}
		for _, v := range vv {
			corsHeaders = append(corsHeaders, kv{name: lk, value: v})
		}
	}
	sort.Slice(corsHeaders, func(i, j int) bool {
		if corsHeaders[i].name != corsHeaders[j].name {
			return corsHeaders[i].name < corsHeaders[j].name
		}
		return corsHeaders[i].value < corsHeaders[j].value
	})
	sb.WriteString("cors-headers (sorted):\n")
	if len(corsHeaders) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, h := range corsHeaders {
			fmt.Fprintf(&sb, "  %s: %s\n", h.name, h.value)
		}
	}

	fmt.Fprintf(&sb, "body: %q\n", string(body))
	return sb.String()
}

// HTTPExpectations exposes a per-request status assertion to the runner.
// The runner's HTTPExpectations branch issues plain GET round-trips
// without per-request custom headers, so it cannot exercise cors gating
// directly. We still expose the trivial expectations table so the
// /permissive route's GET-with-no-headers and /strict's GET-with-no-headers
// status codes are double-checked at the runner layer (in addition to the
// drive byte-stream comparison). This is defense-in-depth — the byte
// stream is the load-bearing assertion.
//
// For the /strict GET we expect 405 because the route is direct_response 405.
func (corsDriver) HTTPExpectations() []fixture.HTTPRequestExpectation {
	return []fixture.HTTPRequestExpectation{
		{Method: "GET", Path: "/permissive", ExpectStatus: 200, ExpectBodyEquivalent: true},
		{Method: "GET", Path: "/strict", ExpectStatus: 405, ExpectBodyEquivalent: true},
	}
}

func (corsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// Compile-time checks: driver implements all required and optional interfaces.
var (
	_ fixture.Driver           = (*corsDriver)(nil)
	_ fixture.HTTPExpectations = (*corsDriver)(nil)
	_ fixture.BackendKindAware = (*corsDriver)(nil)
)

const referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 0.0.0.0, port_value: 15007 }
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
                        - match: { path: "/permissive" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.cors:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy
                              allow_origin_string_match:
                                - exact: "https://example.test"
                              allow_methods: "GET, POST, OPTIONS"
                              allow_headers: "x-foo, x-bar"
                              max_age: "600"
                              expose_headers: "x-baz"
                              allow_credentials: true
                        - match: { path: "/strict" }
                          direct_response:
                            status: 405
                            body: { inline_string: "method not allowed\n" }
                          typed_per_filter_config:
                            envoy.filters.http.cors:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy
                              allow_origin_string_match:
                                - exact: "https://only.test"
                http_filters:
                  - name: envoy.filters.http.cors
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      connect_timeout: 0.25s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`

const subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
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
                        - match: { path: "/permissive" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.cors:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy
                              allow_origin_string_match:
                                - exact: "https://example.test"
                              allow_methods: "GET, POST, OPTIONS"
                              allow_headers: "x-foo, x-bar"
                              max_age: "600"
                              expose_headers: "x-baz"
                              allow_credentials: true
                        - match: { path: "/strict" }
                          direct_response:
                            status: 405
                            body: { inline_string: "method not allowed\n" }
                          typed_per_filter_config:
                            envoy.filters.http.cors:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy
                              allow_origin_string_match:
                                - exact: "https://only.test"
                http_filters:
                  - name: envoy.filters.http.cors
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors
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
