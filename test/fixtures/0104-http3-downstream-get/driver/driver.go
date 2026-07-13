// Package driver registers the 0104-http3-downstream-get fixture with the
// differential runner. See ../README.md for the fixture's purpose. Phase
// 61.3 headline proof: envoy-go's QUIC/HTTP3 downstream listener serves a
// GET->200 identically to reference Envoy (contrib-v1.37.2).
package driver

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0104-http3-downstream-get"

func init() {
	fixture.RegisterFixture(fixtureName, &h3Driver{})
}

type h3Driver struct{}

// BackendCount is 1 — a throwaway backend the driver never dials (the route
// is a direct_response). The runner rejects BackendCount()==0
// (reference_differential_backendcount_min_one), so a fixture with no real
// backend still allocates (and ignores) one.
func (h3Driver) BackendCount() int           { return 1 }
func (h3Driver) SubjectListenerName() string { return "l_h3" }
func (h3Driver) ReferenceListenerPort() int  { return 15104 }

// ReferenceListenerIsUDP marks this fixture's reference listener as a
// UDP/QUIC listener (phase 61.3 — the runner exposes it as /udp, not /tcp,
// and passes the UDP-mapped host addr to DriveReference). See
// fixture.ReferenceListenerIsUDP (introduced by Task 5).
func (h3Driver) ReferenceListenerIsUDP() bool { return true }

// ReferenceBootstrap returns the reference contrib-Envoy H3/QUIC bootstrap.
// The fixed port 15104 appears once (no format args) — backendPorts is
// unused (direct_response route, no cluster).
func (h3Driver) ReferenceBootstrap(_ []int) string {
	return referenceTmpl
}

// SubjectConfig renders envoy-go's equivalent bootstrap: admin on
// 127.0.0.1:subjAdminPort, listener l_h3 on 127.0.0.1:subjListenerPort
// (protocol UDP). Unlike the reference bootstrap, envoy-go's cluster
// manager REJECTS boot with "cluster: zero clusters in bootstrap"
// (internal/cluster/manager.go NewManagerWithBaseDir) if
// static_resources.clusters is empty, so — despite the route being a pure
// direct_response with no cluster dial — the subject template MUST carry a
// throwaway c_backend cluster pointed at backendPorts[0]; the route never
// references it.
func (h3Driver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0])
}

// drive issues one H3 GET /health and returns the body (for the runner's
// CompareBytes). A non-200 status returns an error — the status assertion.
// One shared client (helpers.H3RoundTrip) drives BOTH sides.
func (h3Driver) drive(ctx context.Context, addr string) ([]byte, error) {
	tlsConf := &tls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // differential test
	status, _, body, err := helpers.H3RoundTrip(ctx, addr, tlsConf, http.MethodGet, "/health", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("H3 GET %s: %w", addr, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("H3 GET %s: status %d, want 200", addr, status)
	}
	return body, nil
}

func (d h3Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

func (d h3Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint. Admin
// stays HTTP-over-TCP even for a QUIC data listener (mirrors 0003).
func (h3Driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats scrapes both admins' /stats and asserts the NAMED
// codec-agnostic downstream-stat subset both sides emit
// (http.ingress_http.downstream_rq_2xx and .downstream_rq_total, both
// >=1 on both sides). It deliberately does NOT assert the http3-specific
// counters (e.g. downstream_cx_http3_total) — envoy-go omits
// never-incremented/unsupported stats from its registry emission
// (reference_stats_sink_emits_used_only) and this is a named-subset
// differential, not whole-map equality. Per-property Errorf (NOT Fatalf —
// reference_fatalf_makes_assertions_unreachable); Fatalf is reserved for a
// broken precondition (the scrape itself failing).
func (h3Driver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	refSt, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subjSt, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	for _, name := range []string{
		"http.ingress_http.downstream_rq_2xx",
		"http.ingress_http.downstream_rq_total",
	} {
		// fixture.TB has no Logf (only Errorf/Fatalf/Helper); log.Printf
		// records the observed values for smoke-run / debugging evidence
		// regardless of pass/fail (reference_fixture_tb_has_no_logf).
		log.Printf("0104-http3-downstream-get: ref %s=%d subj %s=%d", name, refSt[name], name, subjSt[name])
		if refSt[name] < 1 {
			t.Errorf("ref %s = %d, want >=1", name, refSt[name])
		}
		if subjSt[name] < 1 {
			t.Errorf("subj %s = %d, want >=1", name, subjSt[name])
		}
	}
}

var (
	_ fixture.Driver                 = h3Driver{}
	_ fixture.StatsAsserter          = h3Driver{}
	_ fixture.ReferenceListenerIsUDP = h3Driver{}
)

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text, NOT
// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
// Driver-side helpers are DUPLICATED locally (not imported from
// test/differential) to avoid an import cycle — verbatim pattern from
// test/differential/harness_h3_test.go's scrapeStats (itself the
// 0057/0055 driver scrapeStats lineage).
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

// referenceTmpl is the reference contrib-Envoy H3/QUIC bootstrap, copied
// verbatim from test/differential/harness_h3_test.go's referenceQUICBootstrap
// (proven: booted reference contrib-v1.37.2 first-try and served GET->200
// with downstream_rq_2xx=1 — see TestReferenceH3_ServesGET). A UDP listener
// carrying udp_listener_config.quic_options (the QUIC/H3 discriminant), a
// filter_chain whose transport_socket is envoy.transport_sockets.quic
// wrapping a DownstreamTlsContext (ALPN "h3", mandatory TLS for QUIC), and
// an HCM with codec_type HTTP3 routing GET /health to a direct_response 200
// "OK\n".
//
// The cert/key pair is the SAME testAlphaCertPEM/testAlphaKeyPEM ECDSA P-256
// self-signed pair used throughout internal/listener/manager_test.go (SAN
// alpha.envoy-go.test, serverAuth EKU, valid 2026-2046) — already proven
// against envoy-go's OWN QUIC listener.
const referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_h3
      address:
        socket_address: { address: 0.0.0.0, port_value: 15104, protocol: UDP }
      udp_listener_config:
        quic_options: {}
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.quic
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport
              downstream_tls_context:
                common_tls_context:
                  tls_certificates:
                    - certificate_chain:
                        inline_string: |
                          -----BEGIN CERTIFICATE-----
                          MIIBjzCCATagAwIBAgIBCjAKBggqhkjOPQQDAjAbMRkwFwYDVQQDExBlbnZveS1n
                          byB0ZXN0IENBMB4XDTI2MDEwMTAwMDAwMFoXDTQ2MDEwMTAwMDAwMFowHjEcMBoG
                          A1UEAxMTYWxwaGEuZW52b3ktZ28udGVzdDBZMBMGByqGSM49AgEGCCqGSM49AwEH
                          A0IABDWs3bNE9rkW6xWB5t7CZWQk86BFAngmNVeAJJdk4Jz5HdsgcMxmscDauk2b
                          bhaKg7T7QbL/P1ypOTYyd6fSbvmjaDBmMA4GA1UdDwEB/wQEAwIFoDATBgNVHSUE
                          DDAKBggrBgEFBQcDATAfBgNVHSMEGDAWgBSUoifyWR8KaOrc10lqG9D5Flw1JDAe
                          BgNVHREEFzAVghNhbHBoYS5lbnZveS1nby50ZXN0MAoGCCqGSM49BAMCA0cAMEQC
                          IAy8XOHKE+KCO6tqVXAKnuCZsohw/1BT5g0sIqdJfqm6AiBsHz8z5ivWuGSWeB4s
                          CJvpxa3L8kMVssG+jnUeLCfOXA==
                          -----END CERTIFICATE-----
                      private_key:
                        inline_string: |
                          -----BEGIN PRIVATE KEY-----
                          MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg4b9v5t4mnAX/Awgy
                          bgjQxpXS1a+CDJn8z5bF5frhPOyhRANCAAQ1rN2zRPa5FusVgebewmVkJPOgRQJ4
                          JjVXgCSXZOCc+R3bIHDMZrHA2rpNm24WioO0+0Gy/z9cqTk2Mnen0m75
                          -----END PRIVATE KEY-----
                  alpn_protocols: ["h3"]
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP3
                stat_prefix: ingress_http
                http3_protocol_options: {}
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// subjectTmpl is envoy-go's equivalent bootstrap: same listener/HCM/route
// shape as referenceTmpl, adapted to loopback addressing with the runner's
// allocated admin/listener ports, PLUS a throwaway c_backend cluster (see
// SubjectConfig doc — envoy-go's cluster manager rejects a bootstrap with
// zero clusters even though the route never dials one). %d placeholders in
// order: subjAdminPort, subjListenerPort, backendPorts[0] (matching the
// template's admin-then-listener-then-cluster field order, per the 0003
// precedent).
const subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h3
      address:
        socket_address: { address: 127.0.0.1, port_value: %d, protocol: UDP }
      udp_listener_config:
        quic_options: {}
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.quic
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport
              downstream_tls_context:
                common_tls_context:
                  tls_certificates:
                    - certificate_chain:
                        inline_string: |
                          -----BEGIN CERTIFICATE-----
                          MIIBjzCCATagAwIBAgIBCjAKBggqhkjOPQQDAjAbMRkwFwYDVQQDExBlbnZveS1n
                          byB0ZXN0IENBMB4XDTI2MDEwMTAwMDAwMFoXDTQ2MDEwMTAwMDAwMFowHjEcMBoG
                          A1UEAxMTYWxwaGEuZW52b3ktZ28udGVzdDBZMBMGByqGSM49AgEGCCqGSM49AwEH
                          A0IABDWs3bNE9rkW6xWB5t7CZWQk86BFAngmNVeAJJdk4Jz5HdsgcMxmscDauk2b
                          bhaKg7T7QbL/P1ypOTYyd6fSbvmjaDBmMA4GA1UdDwEB/wQEAwIFoDATBgNVHSUE
                          DDAKBggrBgEFBQcDATAfBgNVHSMEGDAWgBSUoifyWR8KaOrc10lqG9D5Flw1JDAe
                          BgNVHREEFzAVghNhbHBoYS5lbnZveS1nby50ZXN0MAoGCCqGSM49BAMCA0cAMEQC
                          IAy8XOHKE+KCO6tqVXAKnuCZsohw/1BT5g0sIqdJfqm6AiBsHz8z5ivWuGSWeB4s
                          CJvpxa3L8kMVssG+jnUeLCfOXA==
                          -----END CERTIFICATE-----
                      private_key:
                        inline_string: |
                          -----BEGIN PRIVATE KEY-----
                          MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg4b9v5t4mnAX/Awgy
                          bgjQxpXS1a+CDJn8z5bF5frhPOyhRANCAAQ1rN2zRPa5FusVgebewmVkJPOgRQJ4
                          JjVXgCSXZOCc+R3bIHDMZrHA2rpNm24WioO0+0Gy/z9cqTk2Mnen0m75
                          -----END PRIVATE KEY-----
                  alpn_protocols: ["h3"]
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP3
                stat_prefix: ingress_http
                http3_protocol_options: {}
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
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
`
