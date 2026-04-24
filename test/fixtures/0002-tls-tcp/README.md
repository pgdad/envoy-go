# Fixture 0002-tls-tcp

Differential fixture for phase-03 TLS. Exercises:

1. **Downstream TLS termination** — the proxy presents a server certificate and
   terminates the client's TLS session.
2. **SNI-indexed filter-chain dispatch** — a single listener (`l_tls`,
   `0.0.0.0:15002` for the reference proxy) hosts two filter chains selected by
   SNI: `alpha.envoy-go.test` and `beta.envoy-go.test`.
3. **Upstream TLS origination** — each cluster re-wraps outgoing connections in
   TLS using the cluster's `transport_socket` (UpstreamTlsContext).

## Purpose

This is the capstone fixture for phase 03. It lights up both differential gates
introduced in the phase:

- **Gate (a)**: byte-exact response equality — subject's TLS path produces the
  same echo bytes as the reference proxy.
- **Gate (b)**: distribution assertion — both proxies distribute 9 requests
  across each 3-endpoint cluster with a `[3,3,3]` split (round-robin,
  deterministic with `--concurrency 1`).

## Topology

```
client (test) ──TLS(alpha.envoy-go.test)──► l_tls ──► c_alpha[0..2] ──TLS──► echo backends 0,1,2
client (test) ──TLS(beta.envoy-go.test) ──► l_tls ──► c_beta [0..2] ──TLS──► echo backends 3,4,5
```

6 backends total: indices 0–2 serve c_alpha, indices 3–5 serve c_beta.

## STATIC vs STRICT_DNS divergence (ADR-0010, ADR-0027)

The reference proxy runs inside a Docker container and must reach host-side echo
backends via `host.docker.internal`. It therefore uses `STRICT_DNS` cluster
type with `dns_lookup_family: V4_ONLY` (ADR-0010) so the `host.docker.internal`
hostname resolves to the host's IPv4 address without triggering a dual-stack
lookup.

The subject proxy runs as a host subprocess and dials backends directly via
`127.0.0.1`. It therefore uses `STATIC` cluster type (ADR-0027) — no DNS
resolution required, and `dns_lookup_family` is inapplicable.

## PKI layout

All certificates live in `pki/`. See `pki/README.md` for full details.

| File | Role |
|------|------|
| `pki/ca.pem` | Self-signed ECDSA P-256 root CA trusted by both upstream and downstream |
| `pki/server-alpha.pem` + `.key.pem` | Downstream leaf for the `alpha.envoy-go.test` SNI chain |
| `pki/server-beta.pem` + `.key.pem` | Downstream leaf for the `beta.envoy-go.test` SNI chain |
| `pki/upstream-alpha.pem` + `.key.pem` | Upstream leaf for echo backends serving c_alpha |
| `pki/upstream-beta.pem` + `.key.pem` | Upstream leaf for echo backends serving c_beta |

Upstream leaves carry `DNS:alpha.envoy-go.test`, `DNS:localhost`, and
`IP:127.0.0.1` SANs so both the reference proxy (validates via DNS SAN `alpha.envoy-go.test`)
and the subject proxy (dials `127.0.0.1`, validated via IP SAN) accept them.

PEMs are deterministic — `go run ./pki/gen` re-issues them byte-for-byte; `git
diff pki/` is clean on regeneration.

## Distribution methodology

The driver issues 9 TLS round-trips per SNI (18 total per side). With
`--concurrency 1` (ADR-0028, inherited from the harness unconditionally) and
round-robin load balancing, each of the 3 backends in a cluster receives exactly
3 requests — a `[3,3,3]` distribution. The runner snapshot-diffs the accept
counters before and after each `Drive*` call; the driver's `AssertDistribution`
checks the delta.

## `--concurrency 1` inheritance (ADR-0028)

The harness passes `--concurrency 1` to the reference Envoy container
unconditionally (see `test/differential/harness.go`, `StartReferenceProxy` →
`Cmd`). This applies to fixture 0002 without any harness change — ADR-0028
applies to all fixtures.

## Running

The fixture runs as part of the differential gate:

```bash
go test ./test/differential/... -timeout=5m -run TestDifferential/0002-tls-tcp
```

To run all three fixtures:

```bash
go test ./test/differential/... -timeout=5m -run TestDifferential
```
