# Phase 39 SPEC — active health checks (`Cluster.health_checks`): the host health-state dimension + the cluster-level checker runtime + the health-aware LB pick — the FIRST Upstream-robustness-family row, the family keystone

> **Scope:** this SPEC charters the phase-39 work and EXECUTES the BRAINSTORM §10 D-HC1..D-HC8 empirical pins IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) per `reference_docker_probe_bridge_network`. The PLAN decomposes §10 into bite-sized TDD tasks. Read with the BRAINSTORM (`./BRAINSTORM.md`) — the SPEC AMENDS the BRAINSTORM anticipations where the live probes refined them (§11).

## 1. Purpose / Mission

Phase 39 lands `Cluster.health_checks` (`[]core.config.core.v3.HealthCheck`, proto field 8) — **active** upstream health checking — plus the project's FIRST per-host **health-state dimension** and the health-aware LB pick that makes all six LB constructs route only to healthy hosts (with a panic-threshold fallback). The family **keystone**: the host health-state primitive UNBLOCKS the open Load-balancing family's 3 health-gated candidates {locality-weighted LB, priority load balancing, panic thresholds} and is the substrate {outlier detection, circuit breakers, retries + hedging, connection pooling} compose with.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 in-session probes against `contrib-v1.37.2` produced these load-bearing refinements vs the BRAINSTORM (full detail §11):

- **AMEND-HC1** — the host transition model is CONFIRMED: a host's FIRST active-HC check applies its result IMMEDIATELY (both thresholds bypassed for the first check — a host becomes healthy on its first success and unhealthy on its first failure); `unhealthy_threshold`/`healthy_threshold` gate only SUBSEQUENT transitions. Host state is observable via `/clusters` `health_flags` (`healthy` / `/failed_active_hc`); an unhealthy host is excluded from `membership_healthy` and from LB rotation. The MVP models host state as a single healthy/unhealthy bit (DEGRADED/DRAINING/TIMEOUT deferred).
- **AMEND-HC2** — the `HealthCheck` PGV constraints are CONFIRMED REQUIRED (the reference rejects at config load): `timeout`, `interval`, `unhealthy_threshold`, `healthy_threshold`, the `health_checker` oneof, and (HTTP) a non-empty `path`. Exact wordings pinned (§6). The TCP checker PARSE-ACCEPTS at the reference; the gRPC checker requires cluster H2. `expected_statuses` default = `200`.
- **AMEND-HC3** — `interval`/`timeout`/`unhealthy_threshold`/`healthy_threshold` have NO defaults (PGV-required). `no_traffic_interval` default 60s (a cluster with no traffic slows its checks to 60s after the first). The first check runs ~immediately at startup → the dead host converges to unhealthy within ~1 check, making the `0066` poll-to-converge fast.
- **AMEND-HC4** — the reference's per-cluster active-HC stat surface is PINNED (§7): 8 `health_check.*` + 4 `membership_*` (one pre-exists) + `lb_healthy_panic` + `update_no_rebuild`. The envoy-go 39.1 MVP mirrors **+7** implemented stats (stat surface 1125 → **1132**); the deferred-feature stats are recorded departures.
- **AMEND-HC5** — `Cluster.common_lb_config.healthy_panic_threshold` default 50%; panic fires when healthy% **< threshold** (strict — exactly 50% does NOT panic); in panic mode the LB routes to ALL hosts and `lb_healthy_panic` increments per panic-routed request. `0066` uses a 2-live/1-dead cluster (66% > 50%) so it asserts FILTERING, not panic.
- **AMEND-HC6** — the health-aware pick is CONFIRMED LIVE: an unhealthy host is filtered from rotation (6/6 requests served by the live host, no 503) and the pick is behavior-neutral when all hosts are healthy (§11.6).
- **AMEND-HC7** — the `0066-health-check-http` design + the 39.1/39.2 split are RESOLVED: a 2-live/1-dead cluster, poll-`/stats`-to-convergence then a 100%-live assertion; the 39.1 envelope (~14–20 tasks) holds as a single leg (§8.1/§11.7).
- **AMEND-HC8** — `go mod tidy -diff` EMPTY → 39.1 adds ZERO new go.mod module. `google.golang.org/grpc v1.70.0` is ALREADY a direct dep → 39.2's gRPC checker uses `google.golang.org/grpc/health/grpc_health_v1` directly (no hand-roll, no new module). Oneof field numbers: http=8, tcp=9, grpc=11.

### 1.2 ADR continuity + D-disposition at SPEC commit

2 ADRs anticipated: **ADR-0242** (the host health-state dimension + the active-HC runtime — the state+runtime half) + **ADR-0243** (the health-aware LB pick — Approach A, the build-time-injected health view + panic threshold). §Context drafts anchored here (§13); §Decision/§Consequences bodies land at the 39.1 IMPL per ADR-0044. DECISIONS tail STAYS **ADR-0241** at this SPEC; next-free **ADR-0242**. The §10 D-HC1..D-HC8 pins are RESOLVED (§11); the FINAL ADR-0045 39.1/39.2 split is CONFIRMED (§3.0).

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The TCP + gRPC checkers → 39.2** (the pre-authorized by-codec split). 39.1 = HTTP checker only. A `tcp_health_check`/`grpc_health_check` config is DEPARTURE-rejected at config-build in 39.1 (§6, AMEND-HC2 — the reference parse-accepts TCP; envoy-go 39.1 fail-fast-rejects the unsupported checker, lifted at 39.2).
- **Passive health (outlier detection)** `Cluster.outlier_detection` — a sibling family row; feeds the same health registry later. The `health_check.passive_failure` stat (passive=outlier-driven) is therefore deferred.
- **Circuit breakers / retries + hedging / connection-pool refinement** — sibling family rows.
- **Rich interval/jitter variants** — `no_traffic_interval` (default 60s, honored implicitly), `no_traffic_healthy_interval`, `unhealthy_interval`, `unhealthy_edge_interval`, `healthy_edge_interval`, `initial_jitter`/`interval_jitter`/`interval_jitter_percent`. The MVP honors base `interval`/`timeout`/thresholds.
- **Host degraded/draining/timeout states** + `health_check.degraded`/`membership_degraded`/`membership_excluded` — HEALTHY/UNHEALTHY bit only.
- **The EDS `health_status` initial-health input** + `LbEndpoint.health_check_config` (`port_value`/`hostname`/`disable_active_health_check`); **HC event logging** (`event_log_path`/`event_logger`/`event_service`/`always_log_*`); **HC-specific TLS** (`tls_options`/`transport_socket_match_criteria`); **`alt_port`**; **`reuse_connection`**; **`service_name_matcher`** (the `health_check.verify_cluster` stat); **`request_headers_to_add`/`_remove`**; **`send`/`receive` payload matching**; **`response_buffer_size`**; **`method`**/non-default `codec_client_type`; **`retriable_statuses`**. The MVP HTTP checker = `host` + `path` + default-or-configured `expected_statuses`.
- **`update_no_rebuild`** (EDS-rebuild churn — no EDS) + **`membership_change`** beyond the MVP — recorded departures.
- **The `Cluster.common_lb_config` knobs beyond `healthy_panic_threshold`** (`zone_aware_lb_config`, `locality_weighted_lb_config`, `update_merge_window`, `ignore_new_hosts_until_first_hc`).

## 3. The host health-state dimension + the checker runtime + the health-aware pick (ADR-0242 + ADR-0243)

### 3.0 Split disposition — D-HC7 RESOLVED (the pre-authorized 39.1/39.2 by-codec split CONFIRMED)

39.1 = the keystone + the HTTP checker (this SPEC). 39.2 = the TCP + gRPC checkers reusing the 39.1 substrate. The 39.1 envelope (~500–800 prod LoC / ~14–20 tasks — the host-health registry + the checker runtime + the HTTP probe + the six-LB health-view + the `0066` differential) sits under the ADR-0045 hard gate as a single leg; the PLAN re-checks and may split 39.1 further (state+runtime / health-aware-LB) only if it trips — anticipated NOT. Both ADRs (ADR-0242 + ADR-0243) land at the 39.1 IMPL.

### 3.1 The host health-state dimension (ADR-0242; AMEND-HC1)

A per-cluster host-health registry on `Cluster`: `map[string]*hostHealth` keyed by `Endpoint.Addr()`; each `hostHealth` holds an atomic `healthy` bool + `consecutiveFailures`/`consecutiveSuccesses` uint counters. `Endpoint` (`cluster.go:33`) stays an immutable value type — health lives on the cluster, not the picked copy. Seeded at `extractEndpoints` (`manager.go:557`). The transition (AMEND-HC1): the FIRST check applies its result immediately (healthy on first success, unhealthy on first failure); thereafter `consecutiveFailures == unhealthy_threshold` → unhealthy (reset successes), `consecutiveSuccesses == healthy_threshold` → healthy (reset failures). A host pending its first check is treated as the cluster's startup state (the MVP: start unhealthy-pending, become routable on first success — or start healthy and fail down; the IMPL pins the exact pre-first-check routability, an edge the `0066` poll-to-converge subsumes).

### 3.2 The active-health-check runtime (ADR-0242; AMEND-HC3) — the project's FIRST cluster-level background runtime

Per cluster × per configured `health_check`, a background goroutine: a `time.Ticker` at `interval` (after the first ~immediate check); each tick, probe every host with a `timeout`-bounded context; classify the result (success / failure / network_failure); apply the AMEND-HC1 transition; Inc the §7 stats. **Lifecycle:** a new `Manager.StartHealthChecks(ctx context.Context)` called from `cmd/envoy-go/main.go` AFTER `Freeze` (so the host set + stats are immutable/allocated) — every existing unit/differential test that does not call it gets NO checkers (zero behavior change). `Manager.Drain()` (`manager.go:214`) cancels the root context and waits (a `sync.WaitGroup`) for the goroutines to exit (no goroutine leak). The HTTP probe (39.1) dials the host (reusing the existing H1/H2 upstream client per `codec_client_type` — default H1), sends `GET <path>` with the configured `host` header, reads the response, succeeds iff the status ∈ `expected_statuses` (default `[200,200]`; AMEND-HC2); a connection failure is a `network_failure` (a sub-class of `failure`).

### 3.3 The health-aware LB pick (ADR-0243; AMEND-HC6) — Approach A, build-time-injected

The six LB constructs (`roundRobin`/`leastRequest`/`randomLB`/`ringHashLB`/`maglevLB`/`subsetLB`) gain a **build-time-injected health view** (a reference to the cluster's `map[string]*hostHealth` + a cheap `isHealthy(Endpoint) bool` + a `healthyCount()/total()` accessor) — NOT a `Pick`-signature widening (host health is cluster state, not a per-request value — the deliberate CONTRAST with the ADR-0235 hash-key / ADR-0239 subset-match per-request ctx-carry pick-inputs; the `loadBalancer.Pick(hashKey, hasHash, match, hasMatch)` signature + the exported `Cluster` surface stay BYTE-STABLE). At `Pick`: the construct picks a candidate by its normal rule; if unhealthy, advances (round_robin: next index; least_request: choose the least-loaded HEALTHY host; random: re-draw; ring_hash/maglev: walk the ring/table to the next healthy host — preserving key→host stability, AMEND-HC6); if healthy% **< `healthy_panic_threshold`** (default 50%, strict; AMEND-HC5) it enters panic mode and picks from ALL hosts (Inc `lb_healthy_panic`). **Behavior-neutral when all hosts healthy** (the skip loop never fires, panic never triggers) → every existing fixture + h2spec 53/53 + proxy-wasm 10/10 stays byte-identical (the full 68-dir differential is the real guard).

## 4. Framework primitives — a NEW per-host state dimension + a NEW cluster-level runtime + a health-aware pick + 0 new packages + 0 new go.mod modules (39.1)

- The host-health registry + the checker runtime + the HTTP probe → a NEW `internal/cluster/health.go` (the `ringhash.go`/`maglev.go`/`subset.go` sibling-file precedent).
- The health view consulted in the six existing LB files; the `Manager.StartHealthChecks`/`Drain` lifecycle in `manager.go`; the `extractEndpoints` registry seed; the boot call in `cmd/envoy-go/main.go`.
- ZERO new package (health checking is not a filter); ZERO new go.mod module (AMEND-HC8 — `HealthCheck` is in the core dep; the HTTP probe reuses the H1/H2 client). The exported `Cluster` surface stays byte-stable.

## 5. Proto-field roster (per §11 D-HC2; verified in the `/envoy v1.32.4` module cache)

- `Cluster.health_checks` — `[]core.v3.HealthCheck`, proto field 8 (`cluster.pb.go:779`).
- `Cluster.common_lb_config.healthy_panic_threshold` — `*type.v3.Percent`, field 1, default 50% (`cluster.pb.go:2655`).
- `core.v3.HealthCheck` — `timeout`/`interval` (`*durationpb.Duration`), `unhealthy_threshold`/`healthy_threshold` (`*wrapperspb.UInt32Value`), the `HealthChecker` oneof {`HttpHealthCheck` field 8, `TcpHealthCheck` field 9, `GrpcHealthCheck` field 11}, `no_traffic_interval` + the deferred interval/jitter/event/TLS fields (§2).
- `core.v3.HealthCheck_HttpHealthCheck` — `host`, `path`, `expected_statuses` (`[]type.v3.Int64Range`), `codec_client_type`, `method`, the deferred send/receive/header fields (§2).
- `core.v3.HealthStatus` enum: UNKNOWN=0, HEALTHY=1, UNHEALTHY=2, DRAINING=3, TIMEOUT=4, DEGRADED=5 (MVP: HEALTHY/UNHEALTHY only).

## 6. PARSE-REJECT roster (per §11 D-HC2 + ADR-0080)

envoy-go does its OWN parse-reject (no PGV — the 38.2 precedent), byte-stable. The reference's PGV wordings (pinned live), which envoy-go reproduces or supplies a byte-stable equivalent (the IMPL pins exact strings against the reference):

1. **health_checker oneof missing** → reference: `field: "health_checker", reason: is required`.
2. **HTTP empty `path`** → reference: `HttpHealthCheckValidationError.Path: value length must be at least 1 characters`.
3. **`interval` missing** → reference: `HealthCheckValidationError.Interval: value is required`.
4. **`timeout` missing** → reference: `HealthCheckValidationError.Timeout: value is required`.
5. **`unhealthy_threshold` missing** (and `healthy_threshold`) → reference: `HealthCheckValidationError.UnhealthyThreshold: value is required`.
6. **`tcp_health_check`/`grpc_health_check` checker in 39.1** → envoy-go DEPARTURE-reject (config-build) as an unsupported checker (the reference parse-ACCEPTS TCP; gRPC additionally requires cluster H2 — `cluster must support HTTP/2 for gRPC healthchecking`). Lifted at 39.2. Recorded departure.

All are config-build (boot) rejects → a candidate cross-side boot-reject dir IF warranted (a SEPARATE dir per `reference_differential_fixture_dispatch_constraint`); anticipated a unit-level config-build reject test (the 38-precedent — the distribution/timing makes a boot-reject dir low-value). The IMPL pins exact byte-stable strings.

## 7. Stat surface — the reference set PINNED; envoy-go MVP +7 (1125 → 1132) (per §11 D-HC4 + AMEND-HC4)

**The reference's per-cluster active-HC stat surface (live):** `cluster.<name>.health_check.{attempt,success,failure,passive_failure,network_failure,verify_cluster}` (counters) + `.health_check.{healthy,degraded}` (gauges) + `.membership_{change(c),healthy(g),degraded(g),excluded(g),total(g)}` + `.lb_healthy_panic` (counter) + `.update_no_rebuild` (counter). `membership_total` PRE-EXISTS in envoy-go.

**envoy-go 39.1 MVP — +7 NEW stat-name templates** (emitted on clusters with `health_checks` configured → existing fixtures unaffected; cross-side-exact under `0066` once converged):

1. `health_check.attempt` (counter)
2. `health_check.success` (counter)
3. `health_check.failure` (counter)
4. `health_check.network_failure` (counter)
5. `health_check.healthy` (gauge — count of healthy hosts per checker)
6. `membership_healthy` (gauge)
7. `lb_healthy_panic` (counter)

Anticipated stat surface **1125 → 1132**. DEFERRED (recorded departures — tied to deferred features): `health_check.{passive_failure[outlier],verify_cluster[service_name_matcher],degraded[degraded-state]}`, `membership_{change,degraded,excluded}`, `update_no_rebuild[EDS]`. The `0066` `StatsAsserter` asserts the cross-equal subset {`membership_healthy`, `health_check.attempt/success/failure`} (the deterministically-converging values). The IMPL confirms the exact count + the asserted subset (D-HC4 final).

## 8. Differential fixture taxonomy (+1: `0066` HTTP cross-side poll-to-convergence)

### 8.1 `0066-health-check-http` (cross-side; the NEW poll-to-convergence determinism shape)

An HTTP listener routing to a cluster `c_hc` with `health_checks: [http_health_check{path}]` (short `interval`/`timeout`, low `unhealthy_threshold`) over endpoints = {2 LIVE HTTP backends, 1 DEAD unbound endpoint} (66% healthy > 50% → asserts FILTERING not panic; AMEND-HC5) — on BOTH sides. The driver: (1) **poll phase** — scrape `/stats` on BOTH sides until `membership_healthy == 2` AND `health_check.failure ≥ 1` on both (a generous deadline; no fixed `time.Sleep`); (2) **load phase** — send N requests; assert 100% land on the LIVE backends (the dead host filtered) on BOTH sides; (3) cross-side `health_check.*`/`membership_healthy` parity + `upstream_rq_total` cross-equal. Deliberate-breaks (`-count=1`): drop-the-health-filter → traffic leaks to the dead host → fails the 100%-live arm; break-the-poll-predicate → never converges. The dead-host-via-unbound-address design needs NO new BackendKind (the live hosts reuse the HTTP backends). The ≥20-run flake-free protocol; `-run 'TestDifferential/0066'`. A **flapping/recovery arm** (the `healthy_threshold` return path; a possible +1 BackendKind for a controllable backend) is DEFERRED to a second arm or 39.2 (D-HC7).

### 8.2 NO boot-reject dir (anticipated)

The §6 rejects land as unit-level config-build reject tests (the 38-precedent); a cross-side boot-reject dir only if the IMPL finds it warranted.

### 8.3 NO new BackendKind + NO new fuzzer at 39.1 (family expectations)

BackendKind tail STAYS 33 (the live hosts reuse the existing HTTP backends; the dead host is an unbound address). Fuzzers STAY 42 (the HTTP probe response reuses the already-fuzzed H1/H2 parser; a threshold-transition property test is unit-level, FOLDED into `health_test.go`). 39.2's gRPC checker is anticipated +1 BackendKind (a `grpc.health.v1` SERVING responder) + a candidate `FuzzGrpcHealthResponse`.

## 9. Behavior-contract delta (the 39.1 bundle; ADR-0052 atomic landing)

A new `### Cluster — active health checks` BEHAVIOR_CONTRACT subsection (the host health-state dimension; the HTTP checker; the health-aware pick + panic; the §6 rejects; the §7 stats + the deferred-departure list) + the stat-surface 1125 → 1132 note + the departure records (the §2 deferred surface; the §6.6 tcp/grpc-checker 39.1 departure). DECISIONS gains ADR-0242 + ADR-0243 (bodies at IMPL). STATE/ROADMAP/README/PROGRESS updated; row 39 (39.1 leg) flips `in-progress → done` at the IMPL six-gate.

## 10. Per-task structure (~14–20 tasks; PLAN decomposes)

Anticipated spine (the PLAN bite-sizes): (1) baselines/anchors + PROGRESS.md; (2) the `hostHealth` registry + transitions (AMEND-HC1) + a threshold-transition property test; (3) the `extractEndpoints` registry seed; (4) the HTTP probe codec (`GET path` + `expected_statuses`); (5) the checker runtime + the `interval`/`timeout` loop; (6) the `Manager.StartHealthChecks`/`Drain` lifecycle + the `cmd/envoy-go/main.go` boot call; (7) the build-time health-view injection into the LB factory; (8–9) the health-aware pick across the six constructs (skip + walk-to-next-healthy) + the panic-threshold fallback; (10) the §7 stat registrations + increments; (11) the §6 config parse + rejects (incl. the tcp/grpc 39.1 departure); (12) the `0066` fixture; (13) deliberate-break liveness + flake; (14) full differential re-verify gate; (15) the completion bundle. The FINAL ADR-0045 re-check at the PLAN.

## 11. SPEC-time empirical-pin block (D-HC1..D-HC8 — executed IN-SESSION 2026-06-14)

Probed live against `envoyproxy/envoy:contrib-v1.37.2` (@ `sha256:7edd5b0f…`, ADR-0227) on a Docker bridge network (`reference_docker_probe_bridge_network`): a STRICT_DNS cluster `c_hc` over `be_live` (nginx:alpine, 200 on `/`) + `be_dead` (alpine, no listener → connect-refused), `health_checks: [http{path:/}, interval 1s, timeout 1s, unhealthy/healthy_threshold 2]`, an HTTP listener → `c_hc`, admin :9901; plus `--mode validate` reject probes and a single-dead-host panic probe.

### Summary disposition table (8 pins)

| Pin | Disposition |
|---|---|
| D-HC1 initial state + transitions | RESOLVED — first-check-immediate; thresholds gate subsequent; `/clusters` health_flags; dead excluded |
| D-HC2 config surface + rejects | RESOLVED — PGV wordings pinned (§6); TCP parse-accepts; gRPC needs H2; `expected_statuses` default 200 |
| D-HC3 timing | RESOLVED — interval/timeout/thresholds PGV-required (no defaults); `no_traffic_interval` 60s; first check at startup |
| D-HC4 stat surface | RESOLVED — full reference set pinned; MVP +7 → 1132 (§7) |
| D-HC5 panic | RESOLVED — default 50%, strict `<`, `lb_healthy_panic`++ per panic request; 50% does not panic |
| D-HC6 per-LB skip | RESOLVED — unhealthy filtered, behavior-neutral when all healthy |
| D-HC7 `0066` + split | RESOLVED — 2-live/1-dead poll-to-converge; single 39.1 leg (~14–20 tasks) |
| D-HC8 deps | RESOLVED — ZERO new module; grpc v1.70.0 already a direct dep (39.2 uses grpc_health_v1) |

### 11.1 D-HC1 — initial state + transitions: CONFIRMED LIVE
With `unhealthy_threshold`/`healthy_threshold` = 2, `be_live` became `healthy` after a SINGLE success and `be_dead` became `/failed_active_hc` after a SINGLE failure (`health_check.attempt: 2, success: 1, failure: 1, network_failure: 1`) — the first check applies immediately; thresholds gate only subsequent transitions. `/clusters`: `c_hc::…be_live::health_flags::healthy`, `…be_dead::health_flags::/failed_active_hc`. `membership_healthy: 1` (be_dead excluded). MVP: a single healthy/unhealthy bit; DEGRADED/DRAINING/TIMEOUT deferred.

### 11.2 D-HC2 — config surface + rejects: PINNED LIVE
The §6 PGV wordings captured. The `health_checker` oneof, `interval`, `timeout`, `unhealthy_threshold`, `healthy_threshold`, and (HTTP) a non-empty `path` are all PGV-required (config-load rejects). A `tcp_health_check` config is VALID at the reference (parse-accepts); `grpc_health_check` requires cluster H2 (`c cluster must support HTTP/2 for gRPC healthchecking`). `expected_statuses` default is `200` (be_live nginx 200 → success with no `expected_statuses` set). envoy-go 39.1 hand-rolls byte-stable rejects + DEPARTURE-rejects tcp/grpc checkers.

### 11.3 D-HC3 — timing: PINNED LIVE
interval/timeout/unhealthy_threshold/healthy_threshold are PGV-required (no defaults). `health_check.attempt` froze at 2 across re-scrapes (`no_traffic_interval` default 60s slowed re-probes for the no-traffic cluster) — confirming the first check runs at startup and subsequent checks honor interval/no_traffic_interval. The MVP honors base `interval`/`timeout`/thresholds; `no_traffic_interval` (60s) is honored implicitly (the dead host fails at the startup check → converges in ~1 check).

### 11.4 D-HC4 — stat surface: PINNED LIVE
The full reference per-cluster set (types from `/stats/prometheus`): `health_check.attempt`(c)/`success`(c)/`failure`(c)/`passive_failure`(c)/`network_failure`(c)/`verify_cluster`(c)/`healthy`(g)/`degraded`(g); `membership_change`(c)/`healthy`(g)/`degraded`(g)/`excluded`(g)/`total`(g); `lb_healthy_panic`(c); `update_no_rebuild`(c). envoy-go MVP mirrors +7 (§7); the rest are deferred-feature departures.

### 11.5 D-HC5 — panic: PINNED LIVE
`healthy_panic_threshold` default 50%. At 1-of-2 healthy (50%) → `lb_healthy_panic: 0`, traffic stayed on the healthy host (50% does NOT trigger panic — strict `<`). A single-dead-host cluster (0% healthy < 50%) → a request returned HTTP 503 and `lb_healthy_panic` incremented 0 → 1 (panic routes to all = the dead host → connect fails → 503). `0066` uses 2-live/1-dead (66% > 50%) to assert FILTERING; a panic arm is a candidate.

### 11.6 D-HC6 — per-LB skip: CONFIRMED LIVE
6 requests to the 1-live/1-dead cluster all returned 200 (round_robin filtered the dead host — no 503s). The unhealthy host is excluded from rotation; the pick is behavior-neutral when all hosts are healthy.

### 11.7 D-HC7 — `0066` design + envelope + split: RESOLVED
`0066` = 2 live + 1 dead, poll-to-converge then 100%-live assertion (§8.1). The 39.1 envelope (~14–20 tasks) holds as a single leg; the PLAN re-checks the ADR-0045 gate. The 39.1/39.2 by-codec split is CONFIRMED.

### 11.8 D-HC8 — deps: CONFIRMED LIVE
`go mod tidy -diff` EMPTY → 39.1 adds ZERO new module. Oneof field numbers http=8/tcp=9/grpc=11. `google.golang.org/grpc v1.70.0` is ALREADY a direct dep → 39.2's gRPC checker imports `google.golang.org/grpc/health/grpc_health_v1` directly (no hand-roll, no new module).

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S39-1** file placement (`health.go` + the six-LB view-consult + the `Manager` lifecycle + the `main.go` boot call) + the health-view interface shape (a `healthView` interface vs a `*Cluster` back-pointer).
- **D-S39-2** the pre-first-check routability edge (start unhealthy-pending vs start healthy-and-fail-down) — pin against the reference startup window; the `0066` poll-to-converge subsumes it.
- **D-S39-3** the exact byte-stable reject strings (§6) + whether the tcp/grpc 39.1 departure is config-build-reject vs parse-accept-defer.
- **D-S39-4** the `0066` constants (host counts, `interval`/`timeout`/threshold, N, the poll predicate + deadline, the ≥20-run flake protocol) + the `fixture_workload_constant_desync` guard.
- **D-S39-5** the checker-runtime test hygiene (the `StartHealthChecks` context cancel in `Drain`; no goroutine leak in `-race` tests; an injectable clock/ticker for deterministic unit tests).
- **D-S39-6** the FINAL ADR-0045 re-check (single 39.1 leg vs a state+runtime / health-aware-LB sub-split).

## 13. ADR continuity — the ADR-0242 + ADR-0243 §Context DRAFTS (anchored here; full entries land at the 39.1 IMPL)

**ADR-0242 (§Context draft) — the host health-state dimension + the active-HC runtime.** Active health checking requires a per-host MUTABLE state the project has never had (`Endpoint` is an immutable value; `internal/cluster` has no background runtime). Decision (to be ratified at IMPL): a per-cluster `map[string]*hostHealth` registry keyed by `Endpoint.Addr()` (atomic healthy + consecutive-result counters; `Endpoint` unchanged); a per-cluster × per-`health_check` background goroutine runtime (`interval`/`timeout`-driven; the AMEND-HC1 first-check-immediate + threshold transitions; the HTTP probe reusing the H1/H2 client); the `Manager.StartHealthChecks(ctx)` post-`Freeze` boot start + the `Manager.Drain()` cancel/wait (the project's FIRST cluster-level background runtime; the ADR-0230 framework-runtime precedent, timer-driven). The §7 stats. Consequences: outlier detection feeds the same registry as a passive input; the 3 health-gated LB candidates consume it.

**ADR-0243 (§Context draft) — the health-aware LB pick.** The six LB constructs must route only to healthy hosts. Decision (to be ratified at IMPL): Approach A — a build-time-injected health view consulted at `Pick` (skip unhealthy + re-pick; ring_hash/maglev walk-to-next-healthy preserving key stability; the `healthy_panic_threshold` fallback-to-all, default 50%, strict `<`). The health view is build-time-injected because host health is CLUSTER state, NOT a per-request value — the deliberate CONTRAST with the ADR-0235/0239 per-request ctx-carry pick-inputs; the `Pick` signature + exported `Cluster` surface stay BYTE-STABLE; behavior-neutral when all healthy (every existing fixture byte-identical). Consequences: panic thresholds (a health-gated LB candidate) reuses the same healthy-fraction machinery.

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

ALL counts UNCHANGED at this SPEC (stat surface 1125 / fixtures 67 / fuzzers 42 / BackendKind 33 / DECISIONS tail ADR-0241, next-free ADR-0242). Anticipated at the 39.1 IMPL: fixtures 67 → 68 (`0066-health-check-http`), DECISIONS tail ADR-0241 → ADR-0243, **stat surface 1125 → 1132** (+7 — AMEND-HC4), fuzzers 42 → 42, BackendKind tail 33 → 33, ZERO new packages + ZERO new go.mod modules. Row 39 STAYS `in-progress` at the SPEC commit (it flips per-leg at each IMPL six-gate — NO parent rollup per ADR-0106). Next → the phase-39 PLAN (`superpowers:writing-plans` — decompose §10 into ~14–20 bite-sized TDD tasks; the FINAL ADR-0045 split-gate re-check).
