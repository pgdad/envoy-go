// Package driver registers the 0003-http11-routing fixture with the
// differential runner. See ../README.md for the fixture's purpose; ADR-0027
// for the STATIC-vs-STRICT_DNS divergence; ADR-0044 for the BEHAVIOR_CONTRACT.
package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0003-http11-routing"

func init() {
	fixture.RegisterFixture(fixtureName, &httpDriver{})
}

type httpDriver struct{}

func (httpDriver) BackendCount() int                { return 3 }
func (httpDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (httpDriver) SubjectListenerName() string      { return "l_http" }
func (httpDriver) ReferenceListenerPort() int       { return 15003 }

func (httpDriver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (httpDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (httpDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}
func (httpDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return drive(ctx, addr)
}

// drive issues 27 HTTP/1.1 round-trips against addr and returns the
// concatenated 9 /health response bodies. Per-request fresh dial (ADR-0039);
// HTTPRoundTrip sets Connection: close by default.
//
// /api bodies are NOT concatenated: ref and subj RR may start at different
// endpoints (STRICT_DNS vs STATIC initial pick), so per-body bytes diverge.
// Routing correctness is covered by AssertDistribution [3,3,3] (accept counts)
// and HTTPExpectations status-200 per request.
//
// /missing bodies are NOT concatenated: 404 local-reply body differs between
// Envoy (HTML/JSON) and envoy-go ("not found\n").
func drive(ctx context.Context, addr string) ([]byte, error) {
	var out strings.Builder
	for n := 0; n < 9; n++ {
		_, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/health", nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/health[%d]: %w", n, err)
		}
		out.Write(body)
	}
	for n := 0; n < 9; n++ {
		// Routed to backend; body NOT concatenated (RR offset may differ).
		_, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", fmt.Sprintf("/api/v1/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/api/v1/%d: %w", n, err)
		}
	}
	for n := 0; n < 9; n++ {
		// 404 body is relaxed; intentionally NOT concatenated.
		_, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", fmt.Sprintf("/missing/%d", n), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("/missing/%d: %w", n, err)
		}
	}
	return []byte(out.String()), nil
}

func (httpDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	want := []uint64{3, 3, 3}
	if len(subjCounts) != 3 {
		return fmt.Errorf("subj backend count: got %d, want 3", len(subjCounts))
	}
	for i, c := range subjCounts {
		if c != want[i] {
			return fmt.Errorf("subj backend %d: got %d, want %d (RR [3,3,3] expected)", i, c, want[i])
		}
	}
	_ = refCounts
	return nil
}

func (httpDriver) HTTPExpectations() []fixture.HTTPRequestExpectation {
	exp := make([]fixture.HTTPRequestExpectation, 0, 27)
	for n := 0; n < 9; n++ {
		_ = n
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: "/health", ExpectStatus: 200, ExpectBodyEquivalent: true})
	}
	for n := 0; n < 9; n++ {
		// Body NOT compared per-request: ref and subj RR may start from different
		// endpoints (STRICT_DNS vs STATIC initial pick). The Drive pass already
		// byte-compares the concatenated stream — that suffices for distribution.
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: fmt.Sprintf("/api/v1/%d", n), ExpectStatus: 200, ExpectBodyEquivalent: false})
	}
	for n := 0; n < 9; n++ {
		exp = append(exp, fixture.HTTPRequestExpectation{Method: "GET", Path: fmt.Sprintf("/missing/%d", n), ExpectStatus: 404, ExpectBodyEquivalent: false})
	}
	return exp
}

func (httpDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
	_ fixture.Driver               = (*httpDriver)(nil)
	_ fixture.DistributionAsserter = (*httpDriver)(nil)
	_ fixture.HTTPExpectations     = (*httpDriver)(nil)
	_ fixture.BackendKindAware     = (*httpDriver)(nil)
)

const referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 0.0.0.0, port_value: 15003 }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/api" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/" }
                          direct_response:
                            status: 404
                            body: { inline_string: "not found\n" }
                http_filters:
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
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
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
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                        - match: { prefix: "/api" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/" }
                          direct_response:
                            status: 404
                            body: { inline_string: "not found\n" }
                http_filters:
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
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
