package differential

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/pgdad/envoy-go/test/helpers"
)

// referenceQUICPort is the fixed in-container UDP/QUIC listener port used by
// TestReferenceH3_ServesGET. Arbitrary but distinct from the harness's other
// fixed ports (avoids any accidental collision with a TCP fixture's listener
// port inside the SAME container — this test starts its own container, so
// collision is not actually possible, but a distinctive port keeps container
// logs/greps unambiguous).
const referenceQUICPort = 15104

// referenceQUICBootstrap is the reference contrib-Envoy H3/QUIC bootstrap
// (SPEC-61 §11 arm h3-get shape): a UDP listener carrying
// udp_listener_config.quic_options (the QUIC/H3 discriminant), a filter_chain
// whose transport_socket is envoy.transport_sockets.quic wrapping a
// DownstreamTlsContext (ALPN "h3", mandatory TLS for QUIC), and an HCM with
// codec_type HTTP3 routing GET /health to a direct_response 200 "OK\n".
//
// The cert/key pair is the SAME testAlphaCertPEM/testAlphaKeyPEM ECDSA P-256
// self-signed pair used throughout internal/listener/manager_test.go (SAN
// alpha.envoy-go.test, serverAuth EKU, valid 2026-2046) — already proven
// against envoy-go's OWN QUIC listener; reused here as inline_string PEM
// blocks so the reference container needs no bind-mounted files.
const referenceQUICBootstrap = `admin:
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

// TestReferenceH3_ServesGET is the leg-61.3 de-risk: it proves the reference
// contrib-Envoy H3 container is reachable from the host over a published
// /udp port and serves a GET->200 (host->container UDP publishing — the
// SPEC-61 §8 untested direction; PROVEN on this machine by the SPEC-61 §11
// probe, re-proven here in the harness). Per reference_docker_probe_bridge_
// network, a green client round-trip alone is not proof the request actually
// decoded server-side — this test ALSO scrapes the reference admin /stats
// and asserts http.ingress_http.downstream_rq_2xx >= 1 as the non-vacuous
// decode witness.
//
// If this fails with a UDP-reachability error (the H3 client cannot reach
// the published UDP port despite the container booting healthy), that is the
// SPEC-flagged risk this task exists to surface — ESCALATE rather than
// papering over it; the fallback is a ReferenceLess subject-only fixture.
func TestReferenceH3_ServesGET(t *testing.T) {
	if testing.Short() {
		t.Skip("differential test; skipped under -short")
	}
	ensureDocker(t)

	pin := loadPinFromRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ref, err := startReferenceProxy(ctx, pin, referenceQUICBootstrap, nil, nil, []int{referenceQUICPort})
	if err != nil {
		t.Fatalf("startReferenceProxy: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()

	addr := ref.ListenerUDPAddr(referenceQUICPort)
	if addr == "" {
		t.Fatalf("ListenerUDPAddr(%d) empty; /udp exposure did not map", referenceQUICPort)
	}
	t.Logf("reference H3 listener published at %s", addr)

	clientTLS := &tls.Config{
		NextProtos:         []string{"h3"},
		InsecureSkipVerify: true, //nolint:gosec // differential test
	}

	status, _, body, err := helpers.H3RoundTrip(ctx, addr, clientTLS, http.MethodGet, "/health", nil, nil)
	if err != nil {
		t.Fatalf("H3RoundTrip GET /health: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if got, want := string(body), "OK\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}

	// Non-vacuous decode proof: the reference admin /stats must show the
	// request actually landed on the HCM (per reference_docker_probe_bridge_
	// network — never trust a green client round-trip that never decoded).
	stats, err := scrapeStats(ref.AdminAddr())
	if err != nil {
		t.Fatalf("scrapeStats(%s): %v", ref.AdminAddr(), err)
	}
	const statName = "http.ingress_http.downstream_rq_2xx"
	got, ok := stats[statName]
	if !ok {
		t.Fatalf("stat %s ABSENT in reference /stats (non-vacuous decode proof failed)", statName)
	}
	if got < 1 {
		t.Errorf("stat %s = %d, want >= 1 (non-vacuous decode proof failed)", statName, got)
	}
	t.Logf("non-vacuous decode proof: %s = %d", statName, got)
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text, NOT
// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
// Verbatim pattern from test/fixtures/0057-thrift-roundtrip/driver/driver.go
// (itself the 0055 redis-driver scrapeStats, verbatim) — reimplemented here
// because test/differential has no shared stats-scrape helper of its own.
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
