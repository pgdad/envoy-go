# envoy-go Behavior Contract

This file is the canonical reference for differential equivalence between envoy-go and the upstream Envoy image pinned in `ENVOY_TARGET.md` (see doctrine `D-3.3` and `D-3.7`). When a phase introduces new features, it extends the per-layer subsections below with concrete rules. When a phase's observed behavior diverges from this contract, either the contract is updated *via ADR* or the implementation is fixed — never both silently.

The contract is the contract. Do **not** consult Envoy C++ source to resolve ambiguity; settle ambiguities with an ADR in `DECISIONS.md` that extends the relevant subsection here.

---

## Equivalence Matrix (§7.2 of BOOTSTRAP_PROMPT.md)

| Dimension | Required equivalence |
|---|---|
| Response status | Exact |
| Response body | Byte-exact for deterministic handlers; semantically equal for filter-modified bodies |
| Response headers | Set-equal modulo documented allow-list (`server`, `date`, timing/identity headers explicitly listed) |
| Response trailers | Set-equal under the same allow-list discipline |
| HTTP/2 & HTTP/3 framing | Structurally equivalent (same frame types/order on equivalent events); not byte-equal |
| Access log records | Semantically equal after field-mapping |
| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |

"Semantically equal" is defined per dimension in the subsections below. Where a dimension has no subsection yet, the matrix row is its complete definition and phases may only tighten (not relax) it.

---

## Header allow-list

The allow-list enumerates response headers whose values are permitted to differ between envoy-go and upstream Envoy without constituting a differential failure. Each entry names the header, the permitted divergence (presence-only / format-only / value-range), the phase that introduced the entry, and the ADR that justifies it.

| Header | Scope | Permitted divergence | Introduced by | Justifying ADR |
|---|---|---|---|---|
| `date` | Admin `/ready` response | Value is RFC 7231 IMF-fixdate, non-deterministic per request. Presence required on both upstream and subject responses; value NOT byte-compared. | Phase 01 | ADR-0015 |

---

## Stat-name mapping

_to be filled per-phase as needed._

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.

---

## Access log field mapping

_to be filled per-phase as needed._

The access-log field mapping enumerates every field that must appear (and the field it maps to on upstream Envoy), the ignore-list for values that are inherently non-deterministic (timestamps, connection ids, durations), and the format normalization rules used by the differential harness before comparison.

---

## xDS wire state machine

_to be filled per-phase as needed._

This subsection captures the ADS/delta state machine as the contract expects: allowed message orderings, ACK/NACK semantics, version/nonce rules, resource-subscription handling, reconnection behavior, and initial-fetch-timeout semantics. It is extended as xDS-family phases land.

---

## Timing tolerances

_to be filled per-phase as needed._

Timing is not compared by default. A phase may opt in to latency bounds (p50 / p95 / p99 vs upstream, wall-clock or CPU-normalized) by adding a subsection here naming the fixture, the bound, and the measurement methodology. Opt-ins are additive; removing one requires a superseding ADR.

---

## Admin API — /ready

*Introduced by phase 01. Justified by ADR-0015 (pre-init contract) and ADR-0014 (Server header value). Captured evidence: `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md`.*

### Ready-state response (authoritative)

Upstream Envoy v1.37.2 emits (in wire order, lowercase header names):

- **Status line:** `HTTP/1.1 200 OK`
- **Body:** `LIVE\n` (5 bytes: `0x4c 0x49 0x56 0x45 0x0a` — trailing LF, NOT CRLF)
- **content-type:** `text/plain; charset=UTF-8`
- **cache-control:** `no-cache, max-age=0`
- **x-content-type-options:** `nosniff`
- **date:** RFC 7231 IMF-fixdate (non-deterministic; see allow-list above)
- **server:** `envoy` (no version suffix — per ADR-0014)
- **transfer-encoding:** `chunked` (upstream framing)

The phase-01 envoy-go subject emits byte-exact equivalent headers + body, with ONE documented framing deviation: the subject emits `Content-Length: 5` instead of `transfer-encoding: chunked`. The differential harness (Task 14) dechunks upstream responses to the raw body bytes before comparison, neutralising the framing difference. Upgrading the subject to chunked framing is a phase-02+ follow-up, not a phase-01 gate.

Header name case: HTTP/1.1 header names are case-insensitive per RFC 7230 §3.2. Upstream emits lowercase (`content-type`); the envoy-go subject emits Go `net/http` canonical case (`Content-Type`). The differential diff (Task 14) compares header names case-insensitively.

### Pre-init response

Per ADR-0015, the pre-init `/ready` window is not exercised by the phase-01 differential test — `cmd/envoy-go` fires `admin.MarkReady` before printing the harness readiness sentinel, so the harness only observes the ready state. The subject emits a documented-but-test-irrelevant pre-init response:

- **Status line:** `HTTP/1.1 503 Service Unavailable`
- **Body:** `PRE_INITIALIZING\n` (17 bytes)
- **Content-Length:** `17`
- Other headers as in the ready response (`Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server`, `Date`).

Upstream v1.37.2's actual pre-init bytes were unobservable from the minimal bootstrap used in Task 7 (60 probes across two tight loops captured no non-200 response). A later phase that successfully captures upstream pre-init bytes supersedes this subsection via a new ADR.

### Applies to

- Phase-01 envoy-go `admin` subsystem.
- Ready-state responses only. Pre-init is documented but not exercised by the phase-01 differential test.

### Does not yet apply to

- HTTP/2 over admin (phase 01 is HTTP/1.1 only).
- Admin endpoints other than `/ready` (phase 08: `config_dump`, `stats`, `clusters`, `listeners`, `server_info`, drain).
- Byte-exact framing (`transfer-encoding: chunked` vs `Content-Length: 5`) — documented deviation; phase-02+ follow-up.

---

## Test harness host networking

Rules governing how fixtures reach a host-side backend from inside the reference Envoy container. These rules are harness-runtime constraints (not equivalence-matrix dimensions), but they are part of the contract because they determine whether a fixture is valid on the pinned reference image.

### DNS — `host.docker.internal` requires `dns_lookup_family: V4_ONLY`

Every fixture whose reference bootstrap uses `host.docker.internal` as a cluster endpoint address MUST set `dns_lookup_family: V4_ONLY` on that cluster.

- **Introduced by:** phase 00 (fixture `0000-tcp-echo`).
- **Justification ADR:** `ADR-0010` in `DECISIONS.md`.
- **Rule:** a `STRICT_DNS` (or other DNS-resolving) cluster whose `socket_address.address` is the literal string `host.docker.internal` must carry `dns_lookup_family: V4_ONLY` at the cluster level.
- **Reason:** Docker Desktop (Linux) resolves `host.docker.internal` to both IPv4 and IPv6 records. Envoy's DNS resolver, left to default (`AUTO`), may select the IPv6 record, which Docker Desktop does not bridge from container to host — the connection fails with "Network is unreachable" and the differential run stalls until the context deadline. `V4_ONLY` eliminates the selection and keeps the fixture deterministic across Docker Desktop, CI runners, and docker-ce hosts.
- **Applies to:** TCP, HTTP/1.1, and HTTP/2 fixtures on this pin.
- **Does not yet apply to:** HTTP/3 / QUIC fixtures — QUIC is UDP and the dual-stack routing behavior differs; the rule must be re-evaluated (via a superseding ADR) when the first QUIC fixture lands.
- **Host-side backend bind address:** because the Docker-provided host gateway on Docker Desktop is a non-loopback IP, host-side test backends MUST bind to `0.0.0.0` (not `127.0.0.1`). A loopback-only bind is unreachable from the container.
- **`extra_hosts` wiring:** CI and developer environments typically need `--add-host=host.docker.internal:host-gateway` on the reference container (set via `testcontainers-go`'s `HostConfigModifier` in `test/differential/harness.go`). Fixtures inherit this from the harness; they do not re-declare it.

Future fixtures that need a different reachability pattern (e.g. container-to-container, or IPv6-on-purpose) add a new rule under this heading via ADR. The V4_ONLY rule above is never silently relaxed — any deviation is an ADR that explicitly supersedes ADR-0010 on the relevant scope.

## TCP proxy

*Introduced by phase 02. Justified by ADR-0024 (per-cluster RR scope) and SPEC §5.4 / §5.5 / §5.8.*

### Response-body byte-equivalence (asserted)

For any fixture whose subject and reference both terminate a TCP connection through `envoy.filters.network.tcp_proxy` (proto `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) backed by a STATIC (subject) or STRICT_DNS (reference) cluster of echo backends, the differential harness compares the concatenated **response bodies** byte-for-byte. Trivially holds for echo backends (each backend reflects the request bytes); for non-echo backends a phase-specific subsection extends this rule.

### Half-close propagation (asserted)

Both proxies must propagate `CloseWrite()` (FIN on the write side) from downstream-to-upstream and from upstream-to-downstream independently — i.e., the dataplane is a true bidirectional pipe with independent half-closes, not a request-response pair. Phase 02 inherits this property from phase 00's `netConn` + `halfClose` byte pump (lifted verbatim per ADR-0023).

### Load-balancer endpoint-selection sequence (NOT asserted)

Cross-proxy LB endpoint-selection sequence is **not** a differential equivalence dimension. Each proxy must be RR-correct **in its own right** (per-proxy distribution): for an N-pick run against a cluster of M endpoints, the per-backend accept count distribution must equal a perfect mod-M partition when `M | N`. This is a **local correctness property** asserted via the optional `DistributionAsserter` interface in `test/differential/fixture/fixture.go`.

The cross-proxy sequence is not asserted because:
- Upstream Envoy's RR LB is per-worker-thread with a randomized starting offset; the absolute sequence of endpoints selected for N consecutive connections is not reproducible across runs or workers.
- The envoy-go subject's RR is per-cluster with a deterministic starting point at index 0 (ADR-0024 + SPEC §5.4 sequence-starts-at-0 invariant); the sequence is reproducible within a single subject process but does not match upstream's randomized sequence.

A phase that needs cross-proxy LB sequence equivalence (e.g., a hash-based LB phase under the load-balancing family) supersedes this subsection with a new ADR documenting the assertion mechanism.

### Listener-bind error semantics (asserted)

If any listener fails to bind, neither proxy should partially serve. Upstream Envoy aborts startup with a non-zero exit and a diagnostic on stderr; phase-02 envoy-go's `cmd/envoy-go/main.go` calls `log.Fatalf("listener start: %v", err)` with the same effect. The two are not byte-compared (`log.Fatalf` and Envoy's startup-error format differ visibly), but both proxies' `/ready` admin endpoint never reports ready in this case (the subject's `MarkReady()` is never reached).

### Applies to

- Phase-02 envoy-go `internal/listener` + `internal/cluster` + `internal/filter/tcpproxy` packages, exercised via fixtures `0000-tcp-echo` and `0001-tcp-proxy-rr`.

### Does not yet apply to

- Filter chain matching (`filter_chain_match` non-empty) — phase 07.
- Multiple filters in a chain — phase 07.
- TLS — phase 03.
- HTTP-aware proxying (`HttpConnectionManager`) — phase 04+.
- LB policies other than ROUND_ROBIN — load-balancing family.
- Cluster types other than STATIC (subject side) / STRICT_DNS (reference side, per ADR-0010) — later phases.
- Health-check-driven endpoint selection — upstream-robustness family.
