# Fixture 0045 — sni_cluster (cross-side; R-M-LIVE)

Cross-side differential fixture for the `envoy.filters.network.sni_cluster`
L4 network filter (phase 27 Task 7). It is the **first cross-side fixture for
sni_cluster**: the listener's filter chain is `[sni_cluster, tcp_proxy]`, so
sni_cluster reads the TLS ServerName and, when non-empty, publishes it verbatim
as the per-connection upstream-cluster-override (ADR-0220); the terminal
`tcp_proxy` consumes the override to route to the cluster named after the SNI,
or falls back to its configured cluster when no SNI is present. Both reference
Envoy v1.37.2 (dockerized) and envoy-go boot the same single-listener TLS
bootstrap; the driver asserts byte-exact parity across the route /
empty-SNI-fallback / unknown-cluster-close arms.

## Fixture type

Cross-side (`fixture.Driver`). The runner spawns BOTH proxies, drives traffic
against the single TLS listener (three TLS dials in sequence), diffs the
response byte streams (`CompareBytes`), and diffs the admin `/ready`
responses.

## Wire shape: single TLS listener + tls_inspector

A **single filter chain** (no `filter_chain_match` SNI routing) handles all
three arms. The listener declares `listener_filters: [tls_inspector]` so
reference Envoy can extract the SNI from the TLS ClientHello before
dispatching to the filter chain (required for Envoy's
`connection.requestedServerName()`). envoy-go extracts the SNI from
`*tls.Conn.ConnectionState().ServerName` after TLS termination — both
mechanisms produce identical `RequestedServerName` values for the sni_cluster
filter.

The `DownstreamTlsContext` uses a single server certificate whose SANs cover
`foo.example.com` and `unknown.example.com`. The fallback arm uses
`InsecureSkipVerify` (empty `ServerName`) since no SAN verification is
possible without an SNI value.

## Three-arm scenario partition (one listener, three TLS dials)

- **route arm** — TLS client sends SNI `foo.example.com`; sni_cluster sets
  the upstream-cluster-override to `"foo.example.com"`; tcp_proxy routes to
  cluster `foo.example.com` (the FOO echo backend). Client sends
  `"sni-route-foo\n"` which is echoed back verbatim. Verdict: `echo_ok`.
  Proves `SNI → override → route` (F-SNI, ADR-0220).

- **fallback arm** — TLS client sends NO SNI (empty `ServerName`,
  `InsecureSkipVerify=true`); sni_cluster observes `""` and sets no override;
  tcp_proxy falls back to its configured cluster `c_fallback` (the FALLBACK
  echo backend). Client sends `"sni-fallback\n"` which is echoed back
  verbatim. Verdict: `echo_ok`. Proves empty-SNI no-op + configured-fallback
  (F-RESOLVE, D27-S1).

- **unknown_close arm** — TLS client sends SNI `unknown.example.com` (no
  such cluster in the static resources); sni_cluster sets override
  `"unknown.example.com"`; tcp_proxy cannot find the cluster (F-NOROUTE,
  D27-4) and closes the downstream without forwarding any bytes. The client
  observes ZERO application bytes. Verdict: `closed_no_bytes`.

## Behavior-parity note on the unknown_close arm

Reference Envoy v1.37.2 and envoy-go close the unknown-SNI connection at
different points in the lifecycle, but both yield ZERO application bytes:

- **Reference Envoy**: The `tls_inspector` listener filter peeks at the SNI
  before the TLS handshake completes. After dispatch, sni_cluster sets the
  override; tcp_proxy (or the underlying cluster resolution) detects the
  missing cluster and aborts the connection **before** completing the TLS
  handshake — the client sees EOF during the handshake.
- **envoy-go**: The TLS handshake completes first (SNI is extracted from
  `*tls.Conn.ConnectionState().ServerName`); then sni_cluster sets the
  override in `OnNewConnection`; tcp_proxy's `Handle` looks up the cluster,
  fails, and closes the downstream — the client sees zero bytes after the
  handshake.

The driver's `tlsDial` function accepts a `closeOK` flag that normalizes both
behaviors (pre-handshake close and post-handshake close with zero bytes) to
the `closed_no_bytes` verdict. The CompareBytes differential gate fires on any
divergence: flipping the route arm's verdict on the subject side to
`DELIBERATE_BREAK_DO_NOT_COMMIT` fails the test immediately (verified in
Task-7 PROGRESS.md).

## Cluster-named-after-SNI convention

The cluster `foo.example.com` is named VERBATIM after the SNI value
`"foo.example.com"`. sni_cluster sets the override to the SNI string with NO
transform; tcp_proxy performs an exact-match lookup by the override string.
This is the idiomatic sni_cluster deployment pattern: name each upstream
cluster after the SNI value that should route to it.

## Bootstrap discipline (two clusters required)

`tcp_proxy` needs upstream clusters — `foo.example.com` (the SNI-named
override target) and `c_fallback` (the tcp_proxy configured cluster). Both are
TCPEcho backends spawned by the runner. A zero-cluster boot is rejected by
both sides (per the bootstrap discipline memory note), so two clusters satisfy
the boot requirement AND prove the routing behavior.

## PKI (test-only keys)

`pki/` contains **test-only** keys and certificates generated by
`pki/gen/main.go` (deterministic P-256; safe ONLY for local differential
testing). Never use these keys outside the test harness.

- `pki/ca.pem` — test CA certificate
- `pki/server.pem` — server leaf certificate (SANs: `foo.example.com`,
  `unknown.example.com`)
- `pki/server.key.pem` — server private key (**test-only**)

Regenerate with `go run ./pki/gen` from the fixture root.

## Distribution assertion (`fixture.DistributionAsserter` — LIVE)

The driver implements `AssertDistribution(refCounts, subjCounts []uint64) error`
to prove the route arm is **live and non-vacuous** as a routing proof. Because
both backends are TCPEcho, a broken override (no `SetUpstreamCluster` call)
would still produce `echo_ok` for the route arm — the byte-stream comparison
alone cannot distinguish "SNI correctly routed to FOO" from "everything fell
back to FALLBACK." The per-backend accept counts resolve this ambiguity.

Expected counts (same on both sides after each Drive pass):

| Backend index | Cluster           | Arm that dials it  | Expected accepts |
|---------------|-------------------|--------------------|-----------------|
| 0             | `foo.example.com` | route arm          | 1               |
| 1             | `c_fallback`      | fallback arm       | 1               |
| —             | —                 | unknown_close arm  | 0 (dials neither)|

A broken `SetUpstreamCluster` call routes ALL SNI traffic to `c_fallback`,
giving `backend[0]=0, backend[1]=2` on the subject side — the assertion fires
with `subject: backend[0] (foo.example.com) got 0 accepts, want 1`.
Liveness-break proof recorded in PROGRESS.md Task-7 "Review fix" subsection.

## Deliberate-break proof (cross-side CompareBytes — LIVE)

Injecting `DELIBERATE_BREAK_DO_NOT_COMMIT` as the route-arm verdict on the
subject side while leaving the reference side's `echo_ok` unmodified causes
a CompareBytes failure at byte offset 18 (the divergence point):
```
ref:  arm route verdict=echo_ok
subj: arm route verdict=DELIBERATE_BREAK_DO_NOT_COMMIT
```
This proves the byte-exact comparison is load-bearing (recorded in
PROGRESS.md Task 7).
