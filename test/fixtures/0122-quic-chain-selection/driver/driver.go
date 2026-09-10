// Package driver registers the 0122-quic-chain-selection fixture with the
// differential runner. See ../README.md for the fixture's purpose. Phase 97
// headline proof: on a QUIC/UDP listener carrying BOTH an eligible
// (empty-match) filter_chains[0] AND a default_filter_chain, envoy-go now
// serves from the INDEXED chain — exactly as reference Envoy
// (contrib-v1.37.2) does — instead of letting the LAST-RESORT default slot
// pre-empt it.
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

const fixtureName = "0122-quic-chain-selection"

// refListenerPort is the in-container reference listener port.
//
// ⚠️ CENSUSED, NOT INHERITED. The `15xxx` band was enumerated at this tip
// with `git grep -hoE '\b15[0-9]{3}\b' -- 'test/fixtures/*/driver/*'
// 'test/fixtures/*/inputs/*' | sort -u`, which reads 28 distinct literals:
// 15000-15011, 15042-15056, 15104 and 15360. Of those, 15360 is NOT a port
// — it is 0017-http-bandwidth-limit's `request_*_total_size` byte figure —
// so 15104 (fixture 0104) is the only reference PORT at or above 15100. The
// producing convention is `15000 + <fixture index>`, a derived observation no
// document states; 0122 therefore takes 15122, verified free
// (`git grep -n 15122` reads zero hits under test/, and `ss -uan`/`ss -tan`
// read zero live sockets).
const refListenerPort = 15122

// Chain-discriminating response bodies. The two chains answer the SAME path
// with the SAME status (222) and DIFFERENT bodies, so the body alone names
// which chain the connection landed on. A cross-side CompareBytes cannot see
// a defect both sides share, so the driver ALSO asserts the body absolutely,
// per side, in drive().
const (
	wantBody      = "chain-indexed\n"
	defaultBody   = "chain-default\n"
	wantStatus    = 222
	drivenPath    = "/health"
	listenerName  = "l_qcs"
	indexedPrefix = "chain_indexed"
	defaultPrefix = "chain_default"
)

func init() {
	fixture.RegisterFixture(fixtureName, &qcsDriver{})
}

type qcsDriver struct{}

// BackendCount is 1 — a throwaway backend the driver never dials (both
// chains' routes are direct_responses). The runner rejects
// BackendCount()==0 (reference_differential_backendcount_min_one), and
// envoy-go's cluster manager additionally boot-rejects a bootstrap whose
// static_resources.clusters key is absent, so the subject template carries a
// c_backend STATIC cluster pointed at this allocated-but-unused port.
func (qcsDriver) BackendCount() int           { return 1 }
func (qcsDriver) SubjectListenerName() string { return listenerName }
func (qcsDriver) ReferenceListenerPort() int  { return refListenerPort }

// ReferenceListenerIsUDP marks this fixture's reference listener as a
// UDP/QUIC listener: the runner exposes it as `<port>/udp` (NOT `/tcp`) and
// passes the UDP-mapped host addr to DriveReference. `--network host` is
// what does NOT work here; the runner's port publishing does. 0104 is the
// only other implementor.
func (qcsDriver) ReferenceListenerIsUDP() bool { return true }

// ReferenceBootstrap returns the reference contrib-Envoy QUIC bootstrap. The
// fixed port 15122 is hard-coded (no format args) and backendPorts is unused
// — both routes are direct_responses and the reference boots fine with zero
// clusters.
func (qcsDriver) ReferenceBootstrap(_ []int) string {
	return referenceTmpl
}

// SubjectConfig renders envoy-go's equivalent bootstrap. The %d placeholders
// appear in the template's own field order — admin, then listener, then
// cluster — so the Sprintf argument order is subjAdminPort,
// subjListenerPort, backendPorts[0] (the 0104 precedent, re-read at this tip
// rather than assumed).
func (qcsDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0])
}

// drive issues one HTTP/3 GET /health and returns the body (for the runner's
// CompareBytes). It asserts BOTH properties absolutely, per side, before
// returning:
//
//	status == 222     — the shared direct_response status
//	body   == wantBody — the INDEXED chain's body
//
// A body of defaultBody is the phase-97 defect itself (the last-resort
// default slot pre-empting an eligible indexed chain) and is reported by
// name, not as a generic mismatch.
func (qcsDriver) drive(ctx context.Context, side, addr string) ([]byte, error) {
	tlsConf := &tls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // differential test
	status, _, body, err := helpers.H3RoundTrip(ctx, addr, tlsConf, http.MethodGet, drivenPath, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: H3 GET %s %s: %w", side, addr, drivenPath, err)
	}
	if status != wantStatus {
		return nil, fmt.Errorf("%s: H3 GET %s %s: status %d, want %d", side, addr, drivenPath, status, wantStatus)
	}
	if got := string(body); got != wantBody {
		if got == defaultBody {
			return nil, fmt.Errorf(
				"%s: H3 GET %s %s: served by the DEFAULT filter chain (body %q) — the last-resort "+
					"default slot pre-empted the eligible empty-match filter_chains[0] (phase-97 defect)",
				side, addr, drivenPath, got)
		}
		return nil, fmt.Errorf("%s: H3 GET %s %s: body %q, want %q", side, addr, drivenPath, got, wantBody)
	}
	return body, nil
}

func (d qcsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, "ref", addr)
}

func (d qcsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, "subj", addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint. Admin
// stays HTTP-over-TCP even for a QUIC data listener (the 0003/0104 shape).
func (qcsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// AssertStats scrapes both admins' /stats and pins exactly TWO names per
// side — a NAMED SUBSET, never a whole map. Measured on this shape: the
// reference emits ~156 stat lines across the two HCM scopes while envoy-go
// emits an order of magnitude fewer, and the reference ADDITIONALLY emits a
// `listener.<addr>.http.<prefix>.*` scope envoy-go does not emit at all —
// whose address token differs cross-side (`0.0.0.0_15122` vs
// `127.0.0.1_<port>`), making any full-name assertion on it cross-side
// infeasible.
//
//	http.chain_indexed.downstream_rq_total >= 1  (both sides)
//	http.chain_default.downstream_rq_total == 0  (both sides)
//
// ⚠️ THE `== 0` PIN IS GUARDED BY AN EXPLICIT PRESENCE CHECK. A scrape map
// returns the zero value for a MISSING key, so `== 0` on a name the side
// never emits would be silently vacuous rather than red. The presence check
// asserts the name is actually in the scrape before its value is believed;
// if a side stops emitting the default chain's scope, that presence
// assertion — not the value assertion — is what goes red.
//
// Per-property Errorf (NOT Fatalf —
// reference_fatalf_makes_assertions_unreachable); Fatalf is reserved for a
// broken precondition (the scrape itself failing).
func (qcsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	refSt, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subjSt, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}

	const (
		indexedTotal = "http." + indexedPrefix + ".downstream_rq_total"
		defaultTotal = "http." + defaultPrefix + ".downstream_rq_total"
	)

	// fixture.TB has no Logf (only Errorf/Fatalf/Helper); log.Printf records
	// the observed values regardless of pass/fail
	// (reference_fixture_tb_has_no_logf).
	for _, s := range []struct {
		side string
		m    map[string]uint64
	}{{"ref", refSt}, {"subj", subjSt}} {
		iv, iok := s.m[indexedTotal]
		dv, dok := s.m[defaultTotal]
		log.Printf("%s: %s %s=%d(present=%t) %s=%d(present=%t)",
			fixtureName, s.side, indexedTotal, iv, iok, defaultTotal, dv, dok)

		// PROPERTY 1 — the eligible indexed chain served.
		if !iok {
			t.Errorf("%s %s: ABSENT from scrape, want present and >=1", s.side, indexedTotal)
		} else if iv < 1 {
			t.Errorf("%s %s = %d, want >=1", s.side, indexedTotal, iv)
		}

		// PROPERTY 2 — the last-resort default slot did NOT serve. The
		// presence check is what keeps the `== 0` non-vacuous.
		if !dok {
			t.Errorf("%s %s: ABSENT from scrape, want present (a missing key reads as 0 and would make the ==0 pin vacuous)", s.side, defaultTotal)
		} else if dv != 0 {
			t.Errorf("%s %s = %d, want 0 (the default filter chain must not have served)", s.side, defaultTotal, dv)
		}
	}
}

var (
	_ fixture.Driver                 = qcsDriver{}
	_ fixture.StatsAsserter          = qcsDriver{}
	_ fixture.ReferenceListenerIsUDP = qcsDriver{}
)

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text, NOT
// /stats/prometheus) and parses "name: value" lines into a map[name]uint64.
// Driver-side helpers are DUPLICATED locally (not imported from
// test/differential) to avoid an import cycle — verbatim pattern from 0104's
// driver, itself the 0057/0055 lineage.
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

// referenceTmpl is the reference contrib-Envoy QUIC bootstrap. Shape (the
// proposition under test):
//
//	ONE UDP/QUIC listener l_qcs carrying BOTH
//	  filter_chains[0]       — NO filter_chain_match key at all, so it is
//	                           UNIVERSALLY ELIGIBLE; stat_prefix
//	                           chain_indexed; direct_response 222
//	                           "chain-indexed\n"
//	  default_filter_chain   — the LAST-RESORT slot, with its OWN QUIC
//	                           transport socket; stat_prefix chain_default;
//	                           direct_response 222 "chain-default\n"
//
// An eligible indexed chain must serve, and the last-resort default slot must
// not pre-empt it.
//
// 🔴 THE TWO stat_prefix VALUES MUST DIFFER, AND THAT IS LOAD-BEARING, NOT
// COSMETIC. Two HCMs sharing one stat_prefix — in ONE listener across two
// filter chains, exactly this shape — panic envoy-go at boot with
// `panic: stats: duplicate metric registration:
// "http.<prefix>.downstream_rq_total"`, rc=2 in 0 seconds. This fixture sits
// ONE IDENTIFIER away from that banked defect.
//
// The listener port is hard-coded (15122) — the reference container publishes
// it as 15122/udp, so there is nothing to template. The reference container
// CANNOT read `filename:` cert paths, so the cert/key are delivered
// `inline_string:`, indented one level deeper than the `inline_string:` key,
// with no PEM text inside YAML comments. The pair is the same
// testAlphaCertPEM/testAlphaKeyPEM ECDSA P-256 self-signed pair
// (SAN alpha.envoy-go.test) 0104 uses and internal/listener's tests prove.
const referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_qcs
      address:
        socket_address: { address: 0.0.0.0, port_value: 15122, protocol: UDP }
      udp_listener_config:
        quic_options: {}
      filter_chains:
        - name: fc_indexed
          transport_socket:
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
                stat_prefix: chain_indexed
                http3_protocol_options: {}
                route_config:
                  name: local_route_indexed
                  virtual_hosts:
                    - name: vh_indexed
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 222
                            body: { inline_string: "chain-indexed\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        transport_socket:
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
              stat_prefix: chain_default
              http3_protocol_options: {}
              route_config:
                name: local_route_default
                virtual_hosts:
                  - name: vh_default
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 222
                          body: { inline_string: "chain-default\n" }
              http_filters:
                - name: envoy.filters.http.router
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// subjectTmpl is envoy-go's equivalent bootstrap: the SAME two-chain
// listener/HCM/route shape as referenceTmpl, adapted to loopback addressing
// with the runner's allocated admin/listener ports, PLUS a throwaway
// c_backend cluster (envoy-go's cluster manager rejects a bootstrap with an
// absent/empty static_resources.clusters even though neither route dials
// one). %d placeholders in the template's own field order: subjAdminPort,
// subjListenerPort, backendPorts[0].
const subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_qcs
      address:
        socket_address: { address: 127.0.0.1, port_value: %d, protocol: UDP }
      udp_listener_config:
        quic_options: {}
      filter_chains:
        - name: fc_indexed
          transport_socket:
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
                stat_prefix: chain_indexed
                http3_protocol_options: {}
                route_config:
                  name: local_route_indexed
                  virtual_hosts:
                    - name: vh_indexed
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response:
                            status: 222
                            body: { inline_string: "chain-indexed\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      default_filter_chain:
        name: fc_default
        transport_socket:
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
              stat_prefix: chain_default
              http3_protocol_options: {}
              route_config:
                name: local_route_default
                virtual_hosts:
                  - name: vh_default
                    domains: ["*"]
                    routes:
                      - match: { prefix: "/" }
                        direct_response:
                          status: 222
                          body: { inline_string: "chain-default\n" }
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
