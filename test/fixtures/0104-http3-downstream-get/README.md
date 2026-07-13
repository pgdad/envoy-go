# 0104-http3-downstream-get

Cross-side differential for phase 61's headline HTTP/3 proof (SPEC-61):
proves that envoy-go's (subject, in-process) QUIC/UDP downstream listener
serves an HTTP/3 `GET /health` → `200` identically to reference Envoy
(`contrib-v1.37.2`, Docker), including the response body and the
codec-agnostic HCM downstream stats.

This is Task 7 of the phase-61.3 (`http3-downstream-get-differential`) TDD
spine. It builds directly on:

- Task 2 — `helpers.H3RoundTrip` (the shared driver-side HTTP/3 client).
- Task 3 — the harness's UDP port exposure + `ReferenceProxy.ListenerUDPAddr`.
- Task 4 — the reference contrib-Envoy QUIC bootstrap (proven booting and
  serving GET→200 first-try; see `test/differential/harness_h3_test.go`'s
  `TestReferenceH3_ServesGET` / `referenceQUICBootstrap`).
- Task 5 — the `fixture.ReferenceListenerIsUDP` marker + the runner's
  UDP-addr dispatch branch (`ReferenceListenerIsUDP() == true` routes the
  reference container to `/udp` port exposure and `ListenerUDPAddr`).
- Task 6 — `writeH3Reply`'s response fidelity (`server`/`content-length`
  headers) on the subject side.

## Topology

Single QUIC/UDP listener (`l_h3`) on both sides:

```yaml
address:
  socket_address: { address: <bind>, port_value: <port>, protocol: UDP }
udp_listener_config:
  quic_options: {}
filter_chains:
  - transport_socket:
      name: envoy.transport_sockets.quic
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport
        downstream_tls_context:
          common_tls_context:
            tls_certificates: [<inlined ECDSA P-256 cert/key>]
            alpn_protocols: ["h3"]
    filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": .../HttpConnectionManager
          codec_type: HTTP3
          stat_prefix: ingress_http
          http3_protocol_options: {}
          route_config: [GET /health -> direct_response 200 "OK\n"]
      http_filters: [envoy.filters.http.router]
```

The cert/key pair is the SAME `testAlphaCertPEM`/`testAlphaKeyPEM` ECDSA
P-256 self-signed pair used throughout `internal/listener/manager_test.go`
(SAN `alpha.envoy-go.test`, `serverAuth` EKU, valid 2026-2046) and by Task
4's proven `referenceQUICBootstrap` — reused here verbatim as
`inline_string` PEM blocks (no bind-mounted files needed on either side).

## The subject-side cluster surprise

The route is a pure `direct_response` — no cluster is ever dialed. The
reference bootstrap indeed carries **no** `clusters:` section (contrib-Envoy
boots and serves fine with zero clusters). envoy-go does **not**:
`internal/cluster.NewManagerWithBaseDir` hard-rejects boot with `"cluster:
zero clusters in bootstrap"` when `static_resources.clusters` is empty. The
subject template therefore carries a throwaway `c_backend` STATIC cluster
pointed at the runner-allocated (but otherwise unused) backend port; the
route never references it. `BackendCount() == 1` exists solely to give the
runner something to allocate for this cluster
(`reference_differential_backendcount_min_one` — the runner rejects
`BackendCount() == 0`).

## Workload

The driver issues **one** HTTP/3 `GET /health` per side via
`helpers.H3RoundTrip` (a fresh `http3.Transport` per call, pinned to the
resolved UDP addr, `InsecureSkipVerify` client TLS). A non-200 status is a
hard driver error (the runner fails the fixture); the response body is
returned for the runner's `CompareBytes` byte-exact comparison.

Per `reference_http_expectations_tcp_only`: this fixture does **not**
implement `fixture.HTTPExpectations` — the runner's `HTTPExpectations`
re-drive uses its own internal HTTP/1-over-TCP client, which cannot reach a
QUIC/UDP listener. Status is asserted in the drive itself; body via
`CompareBytes`; stats via `AssertStats`.

## Asserted

- **Body** (cross-side EXACT via `CompareBytes`): `"OK\n"` on both sides.
- **Status**: `200` on both sides (asserted in-band by the driver's `drive`
  helper — a non-200 aborts the fixture before `CompareBytes` even runs).
- **Stats** (`AssertStats`, named subset — NOT whole-map equality): both
  `http.ingress_http.downstream_rq_2xx >= 1` and
  `http.ingress_http.downstream_rq_total >= 1`, scraped from each side's
  admin `/stats` and asserted independently per side (`Errorf`, not
  `Fatalf`, so both properties are checked even if one is already broken —
  `reference_fatalf_makes_assertions_unreachable`). This is the non-vacuous
  decode witness: a green client round-trip alone does not prove the
  request landed on the HCM (`reference_docker_probe_bridge_network`).
- **Admin** `/ready` (`ProbeAdmin`, mirrors `0003`): admin stays
  HTTP-over-TCP even for a QUIC data listener.

## UNasserted

- HTTP/3-specific counters (e.g. `downstream_cx_http3_total`,
  `listener.<addr>.*`) — envoy-go emits only the stats it actually
  increments, so a whole-map / codec-specific comparison is not meaningful
  here (`reference_stats_sink_emits_used_only`).
- Response header set equality (`server`/`content-length` exact wire
  fidelity is covered by Task 6's subject-side unit test, not re-driven
  here to avoid duplicate/flaky coverage).
- QUIC/H3 transport-level framing (ALPN negotiation cadence, QUIC version
  negotiation, 0-RTT) — only the decoded HTTP semantics (status + body +
  HCM stats) are in scope for this differential.

## Smoke-run evidence

`go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v`
passes and logs (via `log.Printf`, since `fixture.TB` has no `Logf` —
`reference_fixture_tb_has_no_logf`) the live scraped stat values on both
sides, e.g.:

```
0104-http3-downstream-get: ref http.ingress_http.downstream_rq_2xx=1 subj http.ingress_http.downstream_rq_2xx=1
0104-http3-downstream-get: ref http.ingress_http.downstream_rq_total=1 subj http.ingress_http.downstream_rq_total=1
```

confirming the request actually decoded on **both** the reference and
subject HCMs (not a vacuous pass).

The full break protocol (deliberately breaking each assertion to prove it is
live) is Task 8, not this task.
