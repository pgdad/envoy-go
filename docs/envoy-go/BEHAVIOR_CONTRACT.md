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
| Stats output | Per-stat behavioral delta after defined load is equal between envoy-go and reference Envoy. Gauges are snapshot-equal after drain. Names + label keys + types byte-equal; HELP text ignored. Allow-list: 17 stats listed in § Stat-name mapping. All other Envoy stat names in /stats/prometheus output are ignored by the differential. |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |

"Semantically equal" is defined per dimension in the subsections below. Where a dimension has no subsection yet, the matrix row is its complete definition and phases may only tighten (not relax) it.

---

## Header allow-list

The allow-list enumerates response headers whose values are permitted to differ between envoy-go and upstream Envoy without constituting a differential failure. Each entry names the header, the permitted divergence (presence-only / format-only / value-range), the phase that introduced the entry, and the ADR that justifies it.

| Header | Scope | Permitted divergence | Introduced by | Justifying ADR |
|---|---|---|---|---|
| `date` | Admin `/ready` response | Value is RFC 7231 IMF-fixdate, non-deterministic per request. Presence required on both upstream and subject responses; value NOT byte-compared. | Phase 01 | ADR-0015 |
| `Server` | HCM-locally-generated responses | Presence-only — Envoy: `envoy`; envoy-go: `envoy` (matches per ADR-0014 reaffirmation). Set-equality only. | Phase 04 | ADR-0044 |
| `Content-Length` | HTTP/1.1 responses | HTTP/1.1 framing-divergence-permitted (Envoy may choose Content-Length while envoy-go chooses Transfer-Encoding: chunked or vice versa; `http.ReadResponse` decodes both transparently). | Phase 04 | ADR-0044 |
| `Transfer-Encoding` | HTTP/1.1 responses | HTTP/1.1 framing-divergence-permitted (mirror of Content-Length). | Phase 04 | ADR-0044 |
| `x-envoy-*` | Routed-to-upstream HTTP/1.1 responses | Every header with this prefix; presence-not-required on subject (envoy-go does not inject these in phase 04; Envoy does). | Phase 04 | ADR-0044 |
| `x-forwarded-*` | Routed-to-upstream HTTP/1.1 responses | Every header with this prefix; presence-not-required on subject. | Phase 04 | ADR-0044 |
| `x-request-id` | Routed-to-upstream HTTP/1.1 responses | Presence-not-required on subject. | Phase 04 | ADR-0044 |
| `:status` | HCM-locally-generated H2 responses | Required + value-asserted | Phase 05.1 | ADR-0052 |
| `:method` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:path` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:scheme` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:authority` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |

---

## Stat-name mapping

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.

### Flattening rules SN1–SN8 (per ADR-0061; introduced by phase 06.1)

```
Rule SN1: Name segments matching `cluster.<n>.<rest>` extract `<n>` as label
          `envoy_cluster_name` and prefix `<rest>` with `envoy_cluster_`.

Rule SN2: Name segments matching `http.<stat_prefix>.<rest>` extract <stat_prefix>
          as label `envoy_http_conn_manager_prefix` and prefix <rest> with `envoy_http_`.

Rule SN3: Name segments matching `listener.<addr>.<rest>` extract <addr> as label
          `envoy_listener_address` and prefix <rest> with `envoy_listener_`.

Rule SN4: Names ending `_Nxx` where N ∈ {1..5} flatten to a base name with the
          trailing class digit STRIPPED (so the metric name ends in literal `_xx`),
          plus a label `envoy_response_code_class` whose value is the single class
          digit as a string (`"1"`, `"2"`, `"3"`, `"4"`, `"5"`). Empirically verified
          against reference Envoy v1.37.2 at the `ENVOY_TARGET.md`-pinned image
          (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) on 2026-04-27; see
          empirical evidence block below.

          Examples (canonical):
              cluster.foo.upstream_rq_2xx
                → envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="foo"}
              http.ingress_http.downstream_rq_5xx
                → envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"}

          Counter-examples (NOT what Envoy emits):
              ✗ envoy_cluster_upstream_rq_2xx{...}            -- digit suffix preserved (wrong)
              ✗ ...{envoy_response_code_class="2xx",...}       -- label value with literal "xx" (wrong)
              ✗ envoy_cluster_upstream_rq{envoy_response_code_class="2",...}  -- _xx stripped entirely (wrong)

          Empirical evidence (verbatim excerpt from reference-Envoy /stats/prometheus
          scrape under a 5-request load with statuses [200,200,404,200,500] through
          HCM stat_prefix=ingress_http to cluster c_backend):

              # TYPE envoy_cluster_upstream_rq_xx counter
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c_backend"} 3
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="c_backend"} 1
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="c_backend"} 1
              # TYPE envoy_http_downstream_rq_xx counter
              envoy_http_downstream_rq_xx{envoy_response_code_class="1",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_http"} 3
              envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_http"} 1
              envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"} 1

          Negative-confirmation grep (entire 1181-line scrape, no matches):
              grep -E 'envoy_[a-z_]*_(1xx|2xx|3xx|4xx|5xx)' /stats/prometheus  # -> 0 matches

          Tag-extractor regex source: Envoy v1.37.2
          source/common/config/well_known_names.cc, the `RESPONSE_CODE_CLASS`
          tag entry. Source-tree commit pin = the v1.37.2 release tag, server-side
          version-string SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches
          ENVOY_TARGET.md). The regex captures the inner `\dxx` token from the
          stat suffix `_<class>xx`, removes the entire `_<class>xx` from the stat
          name (yielding the base name ending `_xx` after the standard rename), and
          emits the captured digit as the `response_code_class` tag value.

Rule SN5: Server-scope names (`server.<rest>`) flatten to `envoy_server_<rest>`
          with no extracted labels.

Rule SN6: HELP text is best-effort English, NOT byte-equal to Envoy's HELP. The
          differential equivalence claim is on values + label keys + types only.

Rule SN7: Histograms are not emitted by 06.1. (Forward-looking.)

Rule SN8: Per-endpoint cluster stats are not emitted by 06.1. (Forward-looking.)
```

### 17-name table (introduced by phase 06.1)

`<stat_prefix>` is read from HCM config (already plumbed from phase 04). `<addr>` is the listener bind address normalized like Envoy does (e.g., `0.0.0.0:10000` → `0.0.0.0_10000`). `<n>` is the cluster name as configured in the bootstrap.

**Listener — 2 names:**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `listener.<addr>.downstream_cx_total` | counter | `envoy_listener_downstream_cx_total{envoy_listener_address="<addr>"}` |
| `listener.<addr>.downstream_cx_active` | gauge | `envoy_listener_downstream_cx_active{envoy_listener_address="<addr>"}` |

**HCM — 5 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `http.<stat_prefix>.downstream_rq_total` | counter | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_2xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_3xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_4xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_5xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="<stat_prefix>"}` |

**Cluster — 8 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `cluster.<n>.upstream_rq_total` | counter | `envoy_cluster_upstream_rq_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_2xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_3xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="3",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_4xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_5xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_total` | counter | `envoy_cluster_upstream_cx_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_active` | gauge | `envoy_cluster_upstream_cx_active{envoy_cluster_name="<n>"}` |
| `cluster.<n>.membership_total` | gauge | `envoy_cluster_membership_total{envoy_cluster_name="<n>"}` (Set once at register, equals N endpoints) |

**Server — 2 names (one EMITTED, one explicitly NOT-EMITTED):**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `server.live` | gauge | `envoy_server_live` (Set to 1 once admin `/ready` returns 200; never reset by 06.1) |
| `server.uptime` | — | **NOT EMITTED** — depends on monotonic-clock + per-scrape recompute; deferred with histograms (see SPEC §2.1) |

**Total: 17 internal names.** The four `downstream_rq_Nxx` and four `upstream_rq_Nxx` Prometheus exposition forms collapse to two base-name groups (one HCM, one cluster) per the Rule SN4 status-class flattening discipline.

### Twin-series filter discipline (per empirical-verification scrape)

> **Twin-series filter discipline (per empirical-verification scrape):** Envoy v1.37.2 ALSO emits two twin metric families that envoy-go does NOT emit and the differential fixture (§7) MUST filter out before per-counter delta comparison: (a) `envoy_cluster_external_upstream_rq_xx` (the "external" upstream-rq twin Envoy uses to split internal vs external traffic via `internal_traffic` config); (b) `envoy_listener_http_downstream_rq_xx` (a listener-scoped HCM-rq twin keyed by both listener address and HCM stat_prefix); plus the per-exact-status family `envoy_cluster_upstream_rq{envoy_response_code="200"}` (a separate metric family with `envoy_response_code` label, distinct from `envoy_cluster_upstream_rq_xx`'s `envoy_response_code_class` label). The fixture's allow-list enumerates exactly the 13 unique Prometheus names this SPEC ships; everything else in the Envoy scrape is ignored.

---

## Access log field mapping

*Introduced by phase 06.2. Justified by ADR-0066 (architecture: in-tree file sink + AsyncFileSink + drop-newest backpressure), ADR-0067 (reject log_format at parse), ADR-0068 (this subsection — three-tier per-record per-field equivalence matrix), ADR-0069 (server.accesslog_dropped counter naming).*

The access-log field mapping enumerates every operator in the Envoy default
access-log format (15 operators in identical positions on every record) per
ADR-0066, the per-operator equivalence tier per ADR-0068's three-tier matrix,
and the empirical-pin block recording the verbatim format-string shape from
reference Envoy v1.37.2. The differential equivalence claim (the
"Semantically equal after field-mapping" predicate from the Equivalence
Matrix row at line 18) IS the three-tier matrix below.

### 15-operator default format (per ADR-0066; empirical-pin in §11 of the 06.2 SPEC)

[<START_TIME>] "<:METHOD> <:PATH> <PROTOCOL>" <RESPONSE_CODE> <RESPONSE_FLAGS>
<BYTES_RECEIVED> <BYTES_SENT> <DURATION> <RESP-SVC-TIME> "<X-FORWARDED-FOR>"
"<USER-AGENT>" "<X-REQUEST-ID>" "<:AUTHORITY>" "<UPSTREAM_HOST>"

### Three-tier matrix (per ADR-0068)

Tier E (exact byte-equal cross-side; 7 operators):
  :METHOD, :PATH, PROTOCOL, RESPONSE_CODE, BYTES_SENT, USER-AGENT, :AUTHORITY

Tier F (format-only — parses to expected shape on both sides; cross-side value
not asserted equal; 3 operators):
  START_TIME (RFC3339 ms-precision UTC, within workload wall-clock window)
  DURATION (int ms ≥ 0)
  UPSTREAM_HOST (`<host>:<port>` for routed; `-` for direct_response)

Tier S (subject must emit `-`; reference unconstrained; 5 operators):
  RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
  X-FORWARDED-FOR, X-REQUEST-ID

Counts: 7 + 3 + 5 = 15 (= the operator count in the format).

### X-ENVOY-ORIGINAL-PATH?:PATH fallback note (per 06.2 SPEC §6.1)

Operator #3 in the format is %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% — emit the
original-path header if present, else fall through to :PATH. Neither side
emits X-ENVOY-ORIGINAL-PATH on fixture 0006's workload (envoy-go does not
inject it; reference Envoy doesn't either, because fixture 0006 has no
path_rewrite-bearing route); both sides emit :PATH via the fallback. A
future phase introducing path-rewriting must re-evaluate fixture 0006's
Tier-E/F expectations under the new behavior.

### Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log)

The 5-line empirical-pin scrape from `docs/envoy-go/phases/06.2-access-log/SPEC.md` §11
(captured 2026-04-30 from reference Envoy v1.37.2 image SHA
`c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`) is the
ground truth. SPEC §11 is the canonical location; this subsection mirrors it.

### Applies to

- Phase-06.2 envoy-go `internal/accesslog` package + the four HCM emit-deferral sites (`directResponseAction.do`, `routerAction.do`, `h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`), exercised via fixture `0006-access-log` (H1 differential) + `internal/filter/hcm/accesslog_emit_test.go` (H2 unit tests).
- The 15-operator Envoy default format only. Custom format strings (the `log_format`/`format_string`/`json_format` typed-config fields) are rejected at parse-time per ADR-0067.

### Does not yet apply to

- Operators not plumbed in 06.2 (5 of 15: RESPONSE_FLAGS, BYTES_RECEIVED, RESP-SVC-TIME, X-FORWARDED-FOR, X-REQUEST-ID — Tier S subject-emits-`-`).
- Sinks other than `envoy.access_loggers.file` (stdout / tcp_grpc (gRPC ALS) / open_telemetry — silently-ignored per ADR-0041 06.2 amendment).
- Per-route access-log filters (`access_log[].filter` — silently-ignored).
- Log rotation, fsync, durability ceilings (out of scope per SPEC §2.1).
- Trailers in access logs (deferred to gRPC family per ADR-0058).
- Access-log records for ctx-cancelled requests (skipped per the H2 zero-status sentinel, SPEC §2.1).
- SIGTERM-while-record-pending drain semantics (Phase 08's deliverable).

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

A phase that needs cross-proxy LB sequence equivalence (e.g., a hash-based LB phase under the load-balancing family) supersedes this subsection with a new ADR documenting the assertion mechanism. Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028.

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

---

## TLS

*Introduced by phase 03. Justified by ADR-0031 (stdlib crypto/tls stack selection), ADR-0030 (TLS parameter mapping scope), ADR-0033 (filter-chain subset supersedes ADR-0025), ADR-0032 (upstream TLS dialer), ADR-0035 (fixture-0002 differential scope), ADR-0036 (this subsection).*

Phase 03 introduces envoy-go's first cryptographic surface: downstream TLS termination, upstream TLS origination, and SNI-based filter-chain dispatch. This subsection codifies what the differential harness compares across the reference and subject proxies over TLS, and — importantly — what it does NOT compare.

Fixture `0002-tls-tcp` exercises downstream TLS termination + SNI routing + per-cluster RR distribution. Upstream TLS origination code paths are delivered in the runtime (ADR-0032's `Cluster.Dial` TLS branch; ADR-0031's stdlib `crypto/tls` stack) and unit-tested via `internal/cluster/cluster_test.go` + `internal/tls/config_test.go` + the `FuzzTLSContextParse` upstream seed, but are NOT exercised by a phase-03 differential fixture. See ADR-0035 for the scope-reduction rationale (the phase-03 harness has plain-TCP backends; upstream-TLS differential coverage awaits a later phase with TLS-capable backends).

### Asserted equivalence

**Plaintext-after-decryption byte equivalence.** For a TLS-terminated downstream connection, the response body observed by the fixture driver (after the tunnel is fully peeled) must be byte-exact between reference and subject. Fixture `0002-tls-tcp` exercises this surface with 9 TLS round-trips per SNI per side, 18 per proxy, over a plain-TCP echo upstream.

**Per-SNI chain-selection equivalence.** Given the same SNI on the ClientHello, both proxies must select the logically-equivalent filter chain and dispatch to the logically-equivalent upstream cluster. This is witnessed indirectly via the distribution assertion: fixture 0002's `[3,3,3]` per cluster per SNI per side implies the SNI → chain → cluster dispatch is consistent.

**Server-certificate identity by SNI.** For a given ClientHello SNI, the server certificate selected by each proxy must match on SAN identity. Phase 03 does not byte-compare the cert bytes (both proxies serve the same committed PEM in fixture 0002 — the byte-compare is trivially equal); the rule is semantic: both pick the cert whose SAN covers the SNI.

### Not asserted

**Upstream SNI + CA equivalence (unit-tested only, not differentially asserted).** Because fixture 0002 does not exercise upstream TLS (ADR-0035), the "both proxies send the same SNI / validate against the same trusted_ca" property is covered only by `internal/cluster/cluster_test.go` unit tests and `internal/tls/config_test.go`. A future phase with TLS-capable backends (or a later HTTPS fixture) will either extend this subsection with the assertion or supersede via a new ADR.

**Encrypted-side byte equivalence.** Neither the TLS record boundaries, the session-ticket material, session-ticket-key rotation timing, TLS 1.3 cipher selection (Go's `crypto/tls` and Envoy's BoringSSL have different defaults), handshake message byte ordering/timing, server random, session IDs, nor any other encrypted-side observable is compared. The differential harness diffs decrypted bytes, not TLS records.

**Negotiated ALPN value.** `alpn_protocols` on both sides is passed through to `stdtls.Config.NextProtos`, so both proxies advertise the same ALPN offers; the negotiated value (which wins the ALPN negotiation) is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection.

**Handshake-layer timing.** Not asserted. TLS handshake completion time varies with cipher selection, session resumption state, and handshake retries.

### Parameter mapping caveats

Two `tls_params` fields do not round-trip with full fidelity between Envoy's BoringSSL and Go's `crypto/tls` (see ADR-0030):

- `cipher_suites` with TLS-1.3 cipher names (e.g., `TLS_AES_128_GCM_SHA256`): Go's `crypto/tls` does not permit TLS-1.3 cipher selection. envoy-go logs a diagnostic and drops the entry. Negotiated TLS-1.3 cipher may differ between proxies; this is within the "encrypted-side not asserted" rule above.
- `signature_algorithms`: not publicly configurable in `crypto/tls`. envoy-go errors at parse if a fixture sets this field. Phase-03 fixtures do not set it.

### Scope boundaries

Phase 03 does NOT implement session resumption assertion, OCSP stapling, mTLS validation on the downstream side, SDS, SPIFFE / custom validators, post-quantum key exchange, ALPN-driven filter-chain selection, non-SNI filter-chain match fields, `Listener.default_filter_chain`, `listener_filters` (still silently skipped), HTTPS (HTTP over TLS — phase 04+), upstream TLS differential assertion (deferred per ADR-0035), or transport socket types beyond `tls`. See SPEC §2 for the full non-purposes list and the phase each is deferred to.

---

## HTTP/1.1

Phase 04 introduces HTTP/1.1 routing — the HCM network filter parses request lines off the downstream connection, matches against an inline route table (`match.prefix` bytewise OR `match.path` case-sensitive exact), dispatches through a `direct_response` (HCM-locally-generated reply) or a `route` (router action over a per-request fresh upstream dial). See ADR-0044.

### Asserted equivalence

- Response status code per request (across the full 27-request workload of fixture 0003).
- Decoded response body bytes for `direct_response` 2xx paths (the 9 × `/health` → 200 `OK\n` requests).
- Route-match selection: same method + path → same matched route on both proxies, witnessed by per-cluster RR distribution `[3,3,3]` over the router-action subset (the 9 × `/api/v1/<n>` requests on the subject side).
- Upstream-side request preservation: verbatim Host, method, path-with-query, body — except where stdlib HTTP/1.1 parsing on the subject side introduces a bounded, documented normalisation per ADR-0037.

### Not asserted

- Decoded response body bytes for routed-to-upstream requests. Rationale: the reference proxy (STRICT_DNS) and the subject proxy (STATIC) may start their RR distribution at different endpoint indices. Both produce `[3,3,3]` overall, but request[i] may hit a different backend on each side, yielding non-equal `backend-<idx>:v1/<n>` body bytes per request even though the routing behaviour is correct on both sides. Status code + per-side `[3,3,3]` distribution are the witnesses; per-request body equivalence would require synchronising RR start indices, which is out of scope.
- Local-reply body bytes for 4xx/5xx (Envoy emits HTML/JSON local replies; envoy-go emits plain-text bodies like `"not found\n"`. Status is asserted; body is relaxed).
- Response-header **value** equality (set-equality modulo allow-list only).
- `Content-Length` vs `Transfer-Encoding: chunked` framing per response (the harness decodes both via `http.ReadResponse`).
- Upstream connection re-use (envoy-go does not pool per ADR-0039; Envoy does).
- `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (envoy-go injects none; Envoy injects many — all in the allow-list).

### Header allow-list extensions

See the `## Header allow-list` table above, rows added by ADR-0044: `Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`.

### Applies to

- Phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture `0003-http11-routing`.
- The phase-04 HCM-filter chain shape `[router]` (ADR-0042).
- `match.prefix` (bytewise) and `match.path` (case-sensitive exact) only.

### Does not yet apply to

- HTTP/2 (phase 05).
- HTTP/3 (later).
- HCM filter chain beyond `[router]` (phase 07's filter-chain framework).
- Upstream connection pooling (upstream-robustness family).
- HTTPS (phase 04.x or 05.x or a dedicated HTTPS-fixture sub-phase).
- `match.regex` / `match.path_separated_prefix` / `match.connect_matcher` / header-aware match / query-parameter-aware match (subset enforcement — ADR-0038).
- HTTP-filter iteration protocol (decode-headers, decode-data, encode-headers, etc. — phase 07).

---

## HTTP/2

*Introduced by phase 05.1. Justified by ADR-0046 (codec source: x/net/http2.Framer + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec threshold + pin), ADR-0052 (this subsection — SCAFFOLD form for 05.1, in-place edited for 05.2).*

*Extended by phase 05.2. Justified by ADR-0055 (flow-control discipline), ADR-0056 (per-request fresh upstream H2 dial), ADR-0057 (closes ADR-0035 H2 leg via fixture 0004), ADR-0058 (trailers observed but not forwarded; carry-forwards M-4 + M-10).*

Phase 05.1 introduced envoy-go's downstream HTTP/2 dataplane; phase 05.2 closes the dataplane on the upstream side: cluster-side HttpProtocolOptions parsing, Cluster.DialH2 + ClientConn + RoundTrip, routerActionH2 action variant, and the project's first full-stack HTTPS h2 differential fixture (0004-h2-routing) closing ADR-0035 H2 leg. The flow-control discipline tightening per ADR-0055 makes the codec primitives load-bearing for realistic H2 workloads.

### Asserted equivalence (05.1 + 05.2 scope)

- `:status` per request: required + asserted on every fixture-0004 request (h2spec section 8 covers indirect coverage on the codec; fixture-0004 covers the full proxy+upstream surface).
- Decoded body bytes on `direct_response` 2xx paths: byte-equal to the configured body string. Witnessed by fixture 0004's 9 `/health` requests on both sides + envoy-go's hcm-package unit tests.
- Per-stream response header set-equality modulo allow-list: `:status` (required + asserted), `Server` (matched verbatim with upstream's `envoy` per ADR-0014), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared). NEW (05.2): routed-to-upstream H2 responses now in scope — `:status` required + asserted; `Server`/`Content-Type`/`Content-Length`/`Date` headers from the upstream backend forwarded verbatim (no router-injected headers); the per-stream response header set-equality between sides asserted modulo the same allow-list.
- NEW (05.2): routed-to-upstream H2 request preservation — `:method`/`:path`/`:scheme`/`:authority` forwarded verbatim from downstream to upstream (witnessed by the in-process backend in fixture 0004's tests asserting on received pseudo-headers). The path normalisation discussed in master phase-05 SPEC §5.7 is empty on the H2 side (the path is the bytes of the `:path` pseudo-header — there's no stdlib net/http parsing to inject normalisations).
- NEW (05.2): route-match selection equivalence on H2 — same method + path → same matched route on both proxies (witnessed indirectly by per-side `[3,3,3]` distribution + `:status` per request).
- NEW (05.2): per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K extended to H2).
- NEW (05.2): ALPN selection equivalence at the differential level — a downstream client advertising only `h2` reaches the H2 driver on both proxies (witnessed by fixture 0004's `:status 200` on every routed response).

### Not asserted (05.1 + 05.2 scope)

- Wire-byte H2 framing — unchanged from 05.1.
- SETTINGS values byte-for-byte — unchanged from 05.1.
- WINDOW_UPDATE timing or count — unchanged from 05.1; ADR-0055's tightening adds frame counts that depend on body size and peer window behaviour, which are inherently non-deterministic across the two proxies.
- Stream id allocation pattern — unchanged from 05.1.
- Trailers — observed but not forwarded per ADR-0058 (formalises the upstream-side discard rule; the 05.1 server-side rule was already trailers-not-forwarded).
- 0-RTT TLS early-data behaviour — unchanged from 05.1.
- NEW (05.2): connection re-use upstream (per ADR-0056) — Envoy pools, envoy-go does not; both produce the same per-request `:status`/body output; cross-conn frame counts differ.
- NEW (05.2): cross-side request body bytes for routed-to-upstream requests (mirror of phase-04's ADR-K relaxation extended to H2) — fixture 0004's 9 `/api` request bodies are bodyless GETs, so this rule is unexercised in 05.2; carried forward as the rule for any future POST/PUT-bearing fixture.

### Header allow-list extensions

See the `## Header allow-list` table above. Rows added by ADR-0052: `:status` (active in 05.1; locally-generated H2 responses), `:method`/`:path`/`:scheme`/`:authority` — phase 05.2 flips the latter four rows from "applies-to: phase 05.2 (forward-looking)" to **"applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)"**.

### h2spec threshold

Sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md` per ADR-0051.

**ADR-0055 prose extension (phase 05.2):** the from-scratch H2 codec respects `MaxFrameSize` chunking on outbound DATA, per-stream send-window enforcement, inbound WINDOW_UPDATE emission on a half-window high-water threshold, and overflow bounds-checks on WINDOW_UPDATE deltas. These properties are validated by the regression unit tests (per phase-05.2 PLAN Tasks 2-5) and by the existing h2spec section 5/6 coverage at the pinned SHA; no new section requirements are added.

### Applies to (05.1 + 05.2)

- Phase-05.1: `internal/filter/hcm/h2/` server-side codec (unchanged); the codec-neutral `directResponseAction` factoring; the `--allow-h2c` test-only flag; the conformance suite under `test/conformance/h2spec/`.
- Phase-05.2: `internal/filter/hcm/h2/client.go` (`ClientConn` + `RoundTrip` + `Close`); `internal/cluster/dial_h2.go`; `internal/cluster/manager.go HttpProtocolOptions` reader + validation; `Cluster.UseH2()` accessor; `routerActionH2` action variant in `internal/filter/hcm/actions.go`; fixture `0004-h2-routing` (full-stack HTTPS h2); `test/helpers/h2.go H2RoundTrip` helper.

### Does not yet apply to

- HTTP/3 (later).
- Server push (out of scope permanently in 05.1; potentially out of scope project-wide).
- gRPC framing.
- Trailer forwarding (deferred to phase 07 framework + gRPC family per ADR-0058).
- Upstream H2 stream pooling (upstream-robustness family per ADR-0056).
- h2c production fixtures (test-only path).
- mTLS over h2 (deferred).
- Mixed-codec clusters (a single cluster used by both H1 and H2 listeners — load-balancing family or a future phase explicitly adding mixed-codec clusters).

(REMOVED from this list — now active: routed-to-upstream H2 → active per ADR-0057; fixture 0004 → active per phase-05.2 Task 14.)
