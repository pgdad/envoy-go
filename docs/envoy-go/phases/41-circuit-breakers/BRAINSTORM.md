# Phase 41 Brainstorm — circuit breakers (the THIRD Upstream-robustness-family row; per-priority connection/request OVERFLOW budgets — fail-fast load-shedding at the cluster boundary)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 41 (`circuit-breakers`), the **THIRD Upstream-robustness-family row** (the family opened at phase 39 with active health checks, continued at phase 40 with passive outlier detection). Phase 41 lands `Cluster.circuit_breakers` (`cluster.v3.CircuitBreakers`, `Cluster` field 10) — **load-shedding** at the cluster boundary: a per-priority set of budgets (`max_connections`, `max_pending_requests`, `max_requests`, `max_retries`, `max_connection_pools`) that, once exhausted, cause the cluster to *fail fast* — a new connection/request/retry that would exceed a budget is rejected immediately with a **503 upstream-overflow (UO)** rather than queued. Where phase-39 active health checking *probes* hosts and phase-40 outlier detection *ejects* failing hosts, circuit breaking *caps concurrency* against a healthy-but-saturated upstream — the third independent upstream-robustness dimension.

Phase 41 is the natural follow-on to the phase-39/40 cluster-runtime substrate: it REUSES the per-request upstream lifecycle seams those phases built — the connection-acquire paths (`Cluster.Dial`/`AcquireH1`/`DialH2`) and the per-request outcome callback (`Cluster.RecordUpstreamResult`, ADR-0245) — adding NO new router→cluster channel, only *counter try-acquire/release* hooks at the existing lifecycle points. Circuit breaking is the first upstream-robustness feature that REJECTS load (the prior two only *route around* unhealthy hosts); it introduces the project's first cluster-level concurrency accounting.

The load-bearing facts that shape this brainstorm:

- **`Cluster.circuit_breakers` is a single `cluster.v3.CircuitBreakers` message (Cluster field 10).** VERIFIED present in the EXISTING core `/envoy` go-control-plane dep (`github.com/envoyproxy/go-control-plane/envoy v1.32.4`, `config/cluster/v3/circuit_breaker.pb.go`) — ZERO new go.mod MODULE. Its shape: `CircuitBreakers{Thresholds[] (field 1), PerHostThresholds[] (field 2)}`; each `Thresholds` carries `priority` (`core.RoutingPriority` — DEFAULT=0 / HIGH=1) + five `*wrapperspb.UInt32Value` budgets `max_connections` / `max_pending_requests` / `max_requests` / `max_retries` / `max_connection_pools` + `track_remaining bool` + a nested `RetryBudget{budget_percent, min_retry_concurrency}`. The exact default pins (the well-known `1024/1024/1024/3/∞`, `track_remaining=false`, retry-budget `20%/3`) + the PGV constraints are SPEC-time obligations (§10, D-CB-PROTO). NB: the reference *Envoy binary* used for live probes is `contrib-v1.37.2` (ADR-0227); the *proto dep* is `go-control-plane v1.32.4` — two distinct version axes.
- **Circuit breaking is per-priority concurrency accounting — NOT per-host, NOT a health bit.** A `Thresholds` entry is keyed by `RoutingPriority`; its budgets are cluster-wide counters for that priority band (DEFAULT vs HIGH), not per-endpoint state. This is orthogonal to the phase-40 per-host ejection dimension: a cluster can be at its `max_requests` budget while every host is healthy and un-ejected. So circuit breaking is a NEW per-cluster (per-priority) accounting structure (§2.2, Approach A), not an extension of the `hostHealth`/`clusterHealth` registry.
- **Enforcement is NON-BLOCKING fail-fast, never queuing.** Envoy's pending-requests budget models a bounded queue in front of a connection pool; envoy-go's current upstream path is synchronous-acquire-per-request (no standing request queue). The settled architecture (Fork 2) enforces try-acquire/release at the lifecycle points and, on exhaustion, returns an immediate **503 with the UO response flag** + increments the matching cluster-level overflow counter — it never parks a request waiting for a slot. `max_pending_requests` binds in the narrow window where a request is admitted but its connection is not yet available (a saturated `max_connections`); the exact pending-window semantics against envoy-go's synchronous model are a SPEC empirical pin (D-CB-LIFECYCLE).
- **The lifecycle hook points already exist — no new seam.** `max_requests` try-acquires at request admission in the router (after the route resolves a cluster, before the upstream call begins — `router.go:610` H1 `AcquireH1`, `router_h2.go:76` H2 `DialH2`) and releases at request completion (the existing `RecordUpstreamResult` seam, `cluster.go:186`, fires on every completion path). `max_connections` try-acquires at `Cluster.Dial` (`cluster.go:284`, the single raw-dial chokepoint behind `AcquireH1`/`DialH2`) and releases on upstream-conn close (the existing `connWithGauge` close path that already drives the `upstream_cx_active` gauge). So phase 41 adds counters + try-acquire/release calls at points the code already passes through — no new function-from-router-to-cluster (contrast phase 40, which had to ADD `RecordUpstreamResult`).
- **Per-priority enforcement is DEFAULT-only today, but the stat surface is registered for BOTH priorities.** Nothing in envoy-go routes by `RoutingPriority` yet (no `RouteAction.priority` consumption) — every request is DEFAULT priority. So phase 41 ENFORCES the DEFAULT `Thresholds` only; a HIGH `Thresholds` entry parses and registers its `circuit_breakers.high.*` stat tree but never binds (emits 0) — surface parity with the reference, which always emits both trees (§2.3, the AMEND-OD3-4 emit-0-for-parity precedent). HIGH-priority binding is DEFERRED (§8) behind priority routing.
- **Determinism is again the novel differential risk — now CONCURRENCY-driven, not traffic- or probe-driven.** Active HC ejects after failed probes; outlier detection ejects after failed real requests; circuit breaking trips when *N concurrent in-flight* requests/connections hit a budget — a race-shaped trigger. The `0074` differential must fill a budget with N held-open concurrent requests (a backend that blocks until released), fire the N+1th, and assert the overflow on BOTH sides — WITHOUT a sleep/timing window (the `reference_differential_band_sigma_margin` lesson: no timing-margin assertions). The determinism comes from a *release barrier* — a backend that holds each request open until the driver has confirmed all N are in flight and then fires the overflow request — not from sleeps (§6, §2.5).

The next sessions author the 41 SPEC then the PLAN then the IMPL. The SPEC executes the §10 empirical-pin obligations (D-CB-*) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0248 §Context draft.

**Brainstorm session:** worktree `phase-41-brainstorm` off master (per `feedback_git_worktrees`). Substantive predecessor on master: the phase-40.3 IMPL squash `148f288` (the statistical passive-outlier detectors + the first per-interval aggregation goroutine; ADR-0247), with the docs-only routing tip `981e74f` (the in-flight-BRAINSTORM next-prompt roll) as the literal live tip. Counts at master tip: stat surface **1149**, differential fixtures **75** (tail `0073-outlier-detection-failure-percentage`), fuzzers **42**, BackendKind tail **35** (`HTTP503Responder`), DECISIONS tail **ADR-0247** (next-free **ADR-0248**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the subject + settled the four load-bearing forks via dialogue:

- **Q-subject** — the next subject is **circuit breakers** (chosen over the 2 remaining Upstream-robustness candidates {retries + hedging, per-protocol connection pooling} and the 3 now-unblocked health-gated Load-balancing candidates {locality-weighted LB, priority LB, panic-threshold refinements}). Rationale: circuit breaking composes directly on the phase-39/40 cluster-runtime substrate — it reuses the connection-acquire + request-outcome lifecycle seams those phases built, and it is the load-shedding counterpart to their host-routing-around; it also does NOT depend on the absent priority-routing surface for its DEFAULT-only MVP (unlike retries, which need the retry-policy substrate).
- **Q-scope (Fork 1)** — **the core overflow trio in a SINGLE phase, NO ADR-0045 split**: ENFORCE `max_connections` + `max_pending_requests` + `max_requests` (the three budgets with live request/connection substrate today); PARSE-and-register-but-DEFER enforcement on `max_retries`/`retry_budget` (no retry substrate — retries+hedging is a separate future family candidate), `max_connection_pools` (HTTP/2 multiplexed-pool semantics), and `per_host_thresholds` — emit-0 surface parity (the AMEND-OD3-4 shape). Chosen over a `max_connections`-only keystone (too thin — the three overflow budgets share one accounting structure and one fixture harness) and a full-enforcement scope (blocked on the absent retry/pool substrate).
- **Q-architecture (Fork 2 → ADR-0248)** — a `circuitBreaker` struct on `Cluster` holding per-priority atomic counters (`openConns`/`activeRequests`/`pendingRequests`), NON-BLOCKING try-acquire/release at the existing lifecycle points, fail-fast 503-UO + overflow-counter on exhaustion, `*_open` gauge reads 1 at/over limit. §2.2.
- **Q-per-priority + stats (Fork 3)** — ENFORCE DEFAULT-only, REGISTER both `default.*` + `high.*` trees for parity (HIGH emits 0). Sketch ~+13 stats (1149 → ~1162). §5.
- **Q-differential (Fork 4)** — a blocking backend (`BackendKind #36`, `BlockingHoldResponder`) + release barrier; `0074` (+ possibly `0075`): fill the budget with N held-open concurrent requests, fire the N+1th, assert cross-side 503-UO + `circuit_breakers.default.rq_open == 1` + the overflow-counter delta, then release. Deterministic over the 20-run gate (no sleep). §6.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0247 — especially ADR-0242 [the `clusterHealth`/`hostHealth` registry + the cluster runtime — CONTRASTED: circuit breaking is per-priority cluster accounting, NOT per-host], ADR-0245 [the per-request `RecordUpstreamResult` seam — the request-completion release point phase 41 REUSES], ADR-0247 [the first per-interval outlier goroutine — CONTRASTED: circuit breaking is synchronous try-acquire, NO goroutine], ADR-0106/0045/0044/0052/0080/0227), the as-built `internal/cluster` package (`cluster.go` [the `Cluster` struct at `:85` with `health`/`outlier` fields; `Dial` at `:284`; `AcquireH1` at `:338`; `UpstreamResult` at `:179` + `RecordUpstreamResult` at `:186`; the `connWithGauge` upstream-conn gauge path], `dial_h2.go` [`DialH2` at `:42`], `manager.go` [`buildCluster` at `:363`, parsing `health_checks` at `:407` + `outlier_detection` at `:416` — the `circuit_breakers` parse site]), the router upstream-call path (`internal/filter/http/router/router.go` [`AcquireH1` admission at `:610`] + `router_h2.go` [`DialH2` admission at `:76`]), the route→cluster resolution (`internal/filter/hcm/config.go:545`), and `internal/admin/admin.go` (the `/stats` endpoint the differential overflow-delta + gauge reads). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–40 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/40-outlier-detection/BRAINSTORM.md` section-for-section, reframed for the THIRD row of the Upstream-robustness family: a NEW per-priority cluster-accounting structure (NOT a per-host dimension), counter try-acquire/release hooks at the EXISTING lifecycle seams (NO new router→cluster channel), fail-fast 503-UO load-shedding (the first feature that REJECTS load), DEFAULT-only enforcement with both-priority stat registration, and a concurrency-driven (release-barrier, sleepless) cross-side differential. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-19.

---

## 1. Mission and scope confirmation (41 only)

ROADMAP row `41 | circuit-breakers | 40 | in-progress | | …` (added by this brainstorm) is a **flat top-level Upstream-robustness-family row** (per ADR-0106 — sibling family rows are NOT pre-populated) with **NO split** (the core-overflow-trio scope fits a single SPEC/PLAN/IMPL cycle under the ADR-0045 gate; §1.4). The row's `depends-on` anchor is phase 40 (the last completed phase; substantive predecessor `148f288` — circuit breaking REUSES the phase-39/40 cluster-runtime lifecycle seams).

This is the **third row of the Upstream-robustness family**. The family roster at `ROADMAP.md` (§ Feature Families → Upstream robustness): `{active health checks [DONE — phase 39], outlier detection [DONE — phase 40], circuit breakers, retries + hedging, per-protocol connection pooling}`. Phase 41 lands **circuit breakers**. After phase 41 phase-done, **2** family candidates remain: {retries + hedging, per-protocol connection pooling} — each its own future brainstorm (and the 3 health-gated Load-balancing candidates unblocked at phase 39 also remain).

Branch/directory identifiers: directory `41-circuit-breakers/`. The work lands in the EXISTING `internal/cluster` package (a new `circuitbreaker.go` sibling for the per-priority counter struct + try-acquire/release + stat handles; the `circuit_breakers` parse in `manager.go`'s `buildCluster`; a `circuitBreaker` field on the `Cluster` struct in `cluster.go`; the try-acquire/release call sites at `Dial`/`AcquireH1`/`DialH2`/`RecordUpstreamResult` + the conn-close path) and touches the router (`router.go` + `router_h2.go` — the admission-point try-acquire + the 503-UO short-circuit). It is NOT a new Go package (circuit breaking is a cluster subsystem, not a filter).

Phase 41 is also: (i) the project's **FIRST cluster-level concurrency-accounting structure** (per-priority connection/request budgets — distinct from the phase-40 per-host ejection dimension). (ii) the **FIRST upstream-robustness feature that REJECTS load** (active HC + outlier detection only route *around* unhealthy hosts; circuit breaking *sheds* load against a healthy-but-saturated cluster with a 503-UO). (iii) a **try-acquire/release overlay on the EXISTING lifecycle seams** (no new router→cluster channel — the hooks sit at `Dial`/`AcquireH1`/`DialH2`/`RecordUpstreamResult`, points the code already passes through; byte-neutral when no `circuit_breakers` is configured). (iv) anticipated a ONE-to-TWO-ADR phase (the enforcement-architecture ADR primary).

### 1.1 What phase 41 delivers as a self-contained whole (envelope: the core overflow trio)

Phase 41 lands circuit breaking on the three live-substrate budgets, with the full per-priority accounting structure + both-priority stat surface, as a self-contained whole:

1. **The per-priority circuit-breaker accounting structure** (ADR-0248) — a new `circuitBreaker` struct (new `internal/cluster/circuitbreaker.go`) holding, per `RoutingPriority` band (DEFAULT + HIGH), atomic counters `openConns atomic.Int64`, `activeRequests atomic.Int64`, `pendingRequests atomic.Int64` and the parsed budgets (`maxConnections`/`maxPendingRequests`/`maxRequests`, each an optional cap with the proto default). A `circuitBreaker *circuitBreaker` field on the `Cluster` struct (`cluster.go:85`, alongside `health`/`outlier`). Nil when no `circuit_breakers` is configured (byte-neutral).
2. **The `circuit_breakers` parse + reject surface** (ADR-0248) — `buildCluster` (`manager.go:363`, after the `outlier_detection` parse at `:416`) reads `Cluster.circuit_breakers.Thresholds[]`, keys each entry by `priority`, applies the proto defaults for absent budgets, and registers the per-priority budgets. A new `circuit_breakers`-validation reject surface (envoy-go's own byte-stable rejects per ADR-0080) for nonsensical budgets — exact arms a SPEC pin (D-CB-REJECT).
3. **The try-acquire/release enforcement at the existing lifecycle points** (ADR-0248) — NON-BLOCKING:
   - `max_requests` — try-acquire at request admission in the router (`router.go:610` H1 / `router_h2.go:76` H2, after the cluster resolves, before the upstream call begins); release at request completion via the existing `RecordUpstreamResult` (`cluster.go:186`, fires on every completion path). On exhaustion ⇒ 503-UO + `upstream_rq_*_overflow`++.
   - `max_connections` — try-acquire at `Cluster.Dial` (`cluster.go:284`, the raw-dial chokepoint behind `AcquireH1`/`DialH2`); release on upstream-conn close (the existing `connWithGauge` close path). On exhaustion ⇒ 503-UO + `upstream_cx_overflow`++.
   - `max_pending_requests` — bind in the admitted-but-awaiting-connection window (a saturated `max_connections`); exact window semantics a SPEC pin (D-CB-LIFECYCLE). On exhaustion ⇒ 503-UO + `upstream_rq_pending_overflow`++.
4. **The 503-upstream-overflow short-circuit** (ADR-0248) — on any budget exhaustion the router emits an immediate local 503 with the **UO response flag** (the existing local-reply path; the connection/request is never made), distinct from a real upstream 503. The reference's UO flag + the `upstream_*_overflow` counter pairing is the live-pinned shape (D-CB-LIFECYCLE).
5. **The per-priority `*_open` gauges + the cluster-level overflow counters** (ADR-0248) — the `circuit_breakers.<priority>.*_open` gauge reads 1 while the matching counter is at/over its budget, 0 otherwise; the `upstream_*_overflow` counters increment once per shed request/connection (§5). Registered for BOTH priorities; HIGH always emits 0 (DEFAULT-only enforcement).
6. **The differential fixture(s) `0074-circuit-breaker-...`** (+ possibly `0075`) — a cluster with `circuit_breakers{thresholds:[{priority:DEFAULT, max_requests:N (or max_connections:N)}]}` + a blocking backend, on BOTH sides; the driver fills the budget with N held-open concurrent requests, fires the N+1th, asserts cross-side 503-UO + `circuit_breakers.default.rq_open == 1` + the overflow-counter delta, then releases — §6.
7. **The BEHAVIOR_CONTRACT 41 bundle** + the STATE/ROADMAP advance + the row-41 `in-progress → done` flip at the IMPL six-gate (a flat family row — NO parent rollup per ADR-0106; NO split, so the row flips `done` when the single phase lands).

### 1.2 What phase 41 does NOT deliver (forward to §8)

See §8. Highlights: **`max_retries` + `retry_budget` enforcement** (blocked on the absent retry substrate — belongs to the retries+hedging family candidate; parsed + registered emit-0 at 41); **`max_connection_pools` enforcement** (HTTP/2 multiplexed-pool semantics — parsed + registered emit-0); **`per_host_thresholds`** (field 2 — per-host budget variant; parsed-but-deferred); **`track_remaining` / the `remaining_*` gauges** (the budget-headroom readout); **HIGH-priority binding** (blocked on priority routing — registered emit-0); **circuit breaking on non-HTTP (TCP/network) upstreams**; the `/clusters` admin per-priority budget readout enrichment; the retry-budget dynamic-concurrency model.

### 1.3 Phase-done as the THIRD Upstream-robustness-family row landing

Phase 41 lands the family's third row. After phase 41, the family candidate count is 3 → **2** {retries + hedging, per-protocol connection pooling}. The family heading gains a one-line `circuit-breakers DONE` note. Sibling rows are NOT pre-populated (ADR-0106). The per-priority circuit-breaker accounting structure + the try-acquire/release lifecycle overlay become durable family assets (the `max_retries`/`retry_budget` budgets the retries+hedging family will enforce already parse + register here; the overflow-counter pattern is reusable).

### 1.4 ADR-0045 split readiness — NO split (single phase)

The core-overflow-trio scope is bounded: one accounting struct + three try-acquire/release hooks at existing seams + the parse/reject surface + ~13 stats + 1–2 fixtures. The brainstorm assesses this as a SINGLE SPEC/PLAN/IMPL cycle WELL under the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`) — anticipated ~250–400 prod LoC / ~12–16 tasks. The FINAL ADR-0045 split-gate re-check happens at the SPEC + PLAN (the standard valve); the brainstorm pre-authorizes NO split. (Contrast phase 40, whose full-detector scope drove a 3-leg by-detector-class split; the deferral of retry/pool/per-host enforcement here keeps phase 41 single-leg.)

### 1.5 Seed-stub alignment + package placement

No seed stub. The work is a sibling `circuitbreaker.go` in the EXISTING `internal/cluster` package (alongside `health.go`/`outlier.go`), plus a `circuitBreaker` field + try-acquire/release call sites on the existing `Cluster` methods, plus the two router admission sites + the conn-close release. ZERO new Go packages.

### 1.6 No prebrainstorm-notes branch

There is no off-master prebrainstorm-notes branch for circuit breakers.

### 1.7 Phase 41's relationship to the existing seams (a NEW accounting structure over the EXISTING lifecycle seams)

Circuit breaking composes the existing cluster-runtime seams and adds one accounting structure. It REUSES (a) the connection-acquire chokepoints (`Cluster.Dial`/`AcquireH1`/`DialH2`) where `max_connections`/`max_pending_requests` hook, (b) the per-request completion seam (`RecordUpstreamResult`, ADR-0245) where `max_requests` releases, (c) the router admission point (post-route-resolve, pre-upstream-call) where `max_requests` try-acquires, and (d) the `connWithGauge` upstream-conn close path where `max_connections` releases. It ADDS (e) the per-priority `circuitBreaker` counter struct on `Cluster`. The enforcement is fully SYNCHRONOUS (atomic try-acquire/release on the request/connection path) — NO background goroutine (contrast the phase-40.3 per-interval aggregation goroutine, ADR-0247); the only state is the atomic counters, read at pick/admission and written at acquire/release.

---

## 2. Design decisions

### 2.1 Subject confirmation: circuit breakers — the `cluster.v3.CircuitBreakers` proto surface *(Q-subject → phase 41 row registered)*

`Cluster.circuit_breakers` (`Cluster` field 10) is a single `cluster.v3.CircuitBreakers` message in the EXISTING go-control-plane dep (`v1.32.4`; ZERO new module). The 41 MVP consumes the per-`Thresholds` overflow trio: `priority`, `max_connections`, `max_pending_requests`, `max_requests`. The full field roster (`max_retries`, `max_connection_pools`, `retry_budget`, `track_remaining`, `per_host_thresholds`) + the default/PGV pins are SPEC obligations (§10, D-CB-PROTO).

### 2.2 Enforcement architecture: a per-priority counter struct + non-blocking try-acquire/release *(Q-architecture → Approach A → ADR-0248)*

**Approach A (chosen):** a `circuitBreaker` struct on `Cluster` holding per-priority (DEFAULT/HIGH) atomic counters (`openConns`/`activeRequests`/`pendingRequests`) + the parsed budgets; NON-BLOCKING try-acquire/release at the existing lifecycle points (`max_requests` at router admission → release at `RecordUpstreamResult`; `max_connections` at `Dial` → release at conn-close; `max_pending_requests` in the admitted-but-no-connection window); on exhaustion an immediate 503-UO + the matching `upstream_*_overflow` counter; the `*_open` gauge reads 1 at/over the budget. Chosen over **Approach B** (a blocking/queuing pending model — a real bounded queue with backpressure, matching Envoy's pool-queue semantics — rejected: envoy-go's upstream path is synchronous-acquire-per-request with no standing queue; a true queue is a large new runtime [goroutines/channels/timeouts] far beyond the overflow-budget MVP, and the fail-fast 503-UO is the observable behavior the differential checks) and **Approach C** (enforce in a per-cluster background sweep reading aggregate gauges — rejected: circuit breaking must be SYNCHRONOUS at admission [reject the over-budget request itself, not a later sampled one]; a sweep is imprecise + cannot return the 503-UO on the offending request). See §1 load-bearing facts.

### 2.3 Scope + per-priority posture: the core overflow trio, DEFAULT-enforce + both-priority registration *(Q-scope + Q-per-priority → Fork 1 + Fork 3)*

Enforce `max_connections` + `max_pending_requests` + `max_requests` (DEFAULT priority only — nothing routes by `RoutingPriority` yet); parse-and-register-but-defer `max_retries`/`retry_budget` (no retry substrate), `max_connection_pools` (H2 pools), `per_host_thresholds`. Register BOTH `circuit_breakers.default.*` + `circuit_breakers.high.*` stat trees for surface parity (HIGH never binds, emits 0 — the AMEND-OD3-4 emit-0-for-parity precedent). §1.1, §5, §8.

### 2.4 The enforcement lifecycle: synchronous try-acquire at admission/dial + release at completion/close *(self-answered; pinned at SPEC, D-CB-LIFECYCLE)*

`max_requests`: try-acquire (atomic increment-if-under-budget) at router admission; release (decrement) at `RecordUpstreamResult`. `max_connections`: try-acquire at `Cluster.Dial`; release at upstream-conn close. `max_pending_requests`: increment when a request is admitted but its connection is not yet available (a saturated `max_connections`), decrement when the connection is acquired or the request fails. NO background goroutine. The precise `max_pending_requests` window against envoy-go's synchronous-acquire model + the exact release point on each failure path (the conn never opened, the request rejected pre-dial) + whether `max_connections` counts pooled-and-idle H1 conns or only in-use are SPEC pins (D-CB-LIFECYCLE).

### 2.5 Differential strategy: a blocking backend + release barrier, sleepless concurrency *(Q-differential → fixture envelope §6)*

Fill a budget with N held-open concurrent requests against a backend that blocks until released (`BlockingHoldResponder`, BackendKind #36); once all N are confirmed in flight (the backend signals receipt, or the driver polls the `*_active` gauge to N), fire the N+1th and assert the overflow on BOTH sides; then release the barrier and drain. The determinism comes from the release barrier (no `time.Sleep`, no timing-margin assertion — the `reference_differential_band_sigma_margin` lesson). §6.

### 2.6 Deferred-policy posture: an additive config surface (`circuit_breakers`); a NEW reject surface *(self-answered; pinned at SPEC, D-CB-REJECT)*

`circuit_breakers` is additive — clusters without it are byte-identical (the counter struct is nil ⇒ try-acquire is a no-op pass-through). A new `circuit_breakers`-validation reject surface (envoy-go's own byte-stable rejects per ADR-0080) is anticipated; its exact arms (e.g. the proto admits all-`UInt32Value` budgets so most values are valid — candidate arms are duplicate-priority `Thresholds`, an unset/zero budget interpretation, `per_host_thresholds` presence) are a SPEC pin (D-CB-REJECT).

### 2.7 Stat surface: anticipated ~+13 `circuit_breakers.*` + `upstream_*_overflow` cluster stats *(self-answered; SPEC pins, D-CB-STATS)*

§5.

---

## 3. Framework-survey result — a NEW per-priority accounting struct + try-acquire/release over EXISTING seams + 0 new packages + 0 new go.mod modules (41 anticipated)

### 3.1 Framework: a NEW per-priority counter struct + try-acquire/release hooks at existing lifecycle points *(per §1.7)*

The one genuinely new structure is the `circuitBreaker` per-priority counter struct on `Cluster`. The enforcement is try-acquire/release calls wired into the EXISTING `Dial`/`AcquireH1`/`DialH2` (max_connections), the router admission point (max_requests), the `RecordUpstreamResult` completion seam (max_requests release), and the conn-close path (max_connections release). NO new router→cluster channel.

### 3.2 NEW packages: NONE

41 is `circuitbreaker.go` + edits to `cluster.go`/`manager.go`/`dial_h2.go` + the two router files. No new package.

### 3.3 go.mod modules: anticipated ZERO new (41) *(verified at brainstorm; re-pinned at SPEC D-CB-PROTO)*

`cluster.v3.CircuitBreakers` is in the existing go-control-plane dep (`v1.32.4`, `config/cluster/v3/circuit_breaker.pb.go`); `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

- The connection-acquire chokepoints `Cluster.Dial` (`cluster.go:284`) / `AcquireH1` (`cluster.go:338`) / `DialH2` (`dial_h2.go:42`) — the `max_connections`/`max_pending_requests` hook points.
- The per-request completion seam `RecordUpstreamResult` (`cluster.go:186`, ADR-0245) — the `max_requests` release point.
- The router admission point (`router.go:610` / `router_h2.go:76`, post-route-resolve pre-upstream-call) — the `max_requests` try-acquire point.
- The `connWithGauge` upstream-conn close path (already driving `upstream_cx_active`) — the `max_connections` release point.
- The router local-reply 503 path — the UO short-circuit.
- The `internal/admin` `/stats` endpoint (the differential overflow-delta + `*_open` gauge reads).
- The `reference_docker_probe_bridge_network` live-probe precedent (the SPEC overflow-counter→budget empirical pin).

---

## 4. Per-cluster applicability — a NEW cluster-config surface (`Cluster.circuit_breakers`)

`Cluster.circuit_breakers` is the new per-cluster config surface. A cluster opts in by setting it; absent, behavior is byte-identical (the `circuitBreaker` field is nil ⇒ try-acquire is a pass-through no-op). The budgets compose with active HC + outlier detection independently: a cluster may have all three; circuit breaking caps concurrency on the `available` (healthy + un-ejected) host set the LB picks from — it sheds load when the cluster as a whole is saturated, orthogonal to which hosts are eligible.

---

## 5. Stat surface hypothesis — anticipated ~+13 (`circuit_breakers.<priority>.*_open` gauges + `upstream_*_overflow` counters)

### 5.1 New stat names (SPEC pins, D-CB-STATS)

Anticipated for 41, scoped to clusters with `circuit_breakers` (existing fixtures unaffected). The per-priority `*_open` gauge tree (Envoy emits both DEFAULT + HIGH), registered for both priorities — **+10 gauges** (5 names × 2 priorities):
- `cluster.<n>.circuit_breakers.<default|high>.cx_open` (gauge)
- `cluster.<n>.circuit_breakers.<default|high>.cx_pool_open` (gauge — `max_connection_pools`; emit-0, enforcement deferred)
- `cluster.<n>.circuit_breakers.<default|high>.rq_open` (gauge — `max_requests`)
- `cluster.<n>.circuit_breakers.<default|high>.rq_pending_open` (gauge — `max_pending_requests`)
- `cluster.<n>.circuit_breakers.<default|high>.rq_retry_open` (gauge — `max_retries`; emit-0, enforcement deferred)

Plus the cluster-level overflow COUNTERS that increment per shed request/connection — **~+3–4 counters**:
- `cluster.<n>.upstream_cx_overflow` (counter — `max_connections` shed)
- `cluster.<n>.upstream_rq_pending_overflow` (counter — `max_pending_requests` shed)
- `cluster.<n>.upstream_rq_retry_overflow` (counter — `max_retries`; emit-0, enforcement deferred)
- the exact `max_requests`-overflow counter name (Envoy pairs `max_requests` overflow with a specific counter — `upstream_rq_pending_overflow` vs a dedicated name is the empirical pin)

Anticipated surface **1149 → ~1162**. The EXACT overflow-counter→budget mapping (which counter pairs with `max_requests` vs `max_pending_requests`; whether `track_remaining` adds `remaining_*` gauges) is a SPEC pin from a live `/stats` scrape of a circuit-broken cluster (D-CB-STATS), NOT hard-committed here.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The deferred-enforcement stats (`cx_pool_open`/`rq_retry_open` + `upstream_rq_retry_overflow`, all emit-0; the entire `high.*` tree, emit-0; the `track_remaining`/`remaining_*` gauges, not registered) are recorded departures until their enforcement lands (retries+hedging family, H2-pool work, priority routing).

---

## 6. Differential fixture envelope — anticipated ONE (possibly TWO) directories (41)

### 6.1 Fixtures (+1, possibly +2)

`0074-circuit-breaker-<budget>` — an HTTP listener → a cluster `{1+ backends}` with `circuit_breakers{thresholds:[{priority:DEFAULT, max_requests:N}]}` (or `max_connections:N`), on BOTH the subject and the reference (`contrib-v1.37.2`); the backend is the new `BlockingHoldResponder` (holds each request open until released). The driver: fire N concurrent requests (each blocks at the backend), confirm all N in flight (poll the `circuit_breakers.default.rq_open`-feeding `*_active` state or the backend's receipt signal to N), fire the N+1th, assert on BOTH sides: the N+1th gets a **503 with the UO flag** + `circuit_breakers.default.rq_open == 1` + the `upstream_rq_*_overflow` counter delta == 1; then release the barrier, drain, assert the gauge returns to 0. NO `time.Sleep` — the release barrier provides determinism (the `reference_differential_band_sigma_margin` lesson). Deliberate breaks: (A) the budget never enforces (try-acquire always passes) ⇒ the N+1th succeeds ⇒ no overflow ⇒ fail; (B) the gauge/counter not wired ⇒ the cross-side parity assert fails. A possible SECOND fixture `0075` isolates a second budget (e.g. `max_connections` if `0074` does `max_requests`) — the exact split (one multi-budget fixture vs per-budget dirs) is pinned at SPEC.

### 6.2 Total

Differential fixtures **75 → 76 (or 77)** at the 41 IMPL.

### 6.3 New BackendKind: anticipated +1 (`BlockingHoldResponder`)

The fixture needs a backend that holds a request open (occupying a request/connection slot) until the driver releases it — the deterministic budget-fill mechanism. Anticipated **+1 BackendKind** (`BlockingHoldResponder` — a release-barrier handler; the `HTTP503Responder` 35 in-process-responder precedent) with a driver-side release channel/endpoint. BackendKind tail **35 → 36** (anticipated). The exact release mechanism (a second admin-style endpoint vs a shared channel vs a per-request unblock signal) is a SPEC pin (D-CB-BACKEND).

### 6.4 New fuzzer: anticipated NONE-to-ONE

No new wire decoder (circuit breaking reads no new wire format). A candidate **+1 config-parse fuzzer** (the `circuit_breakers` parse/reject path — the `parseOutlierDetection` fuzzer precedent) IF the reject surface warrants it; otherwise fuzzers STAY **42**. Pinned at SPEC (D-CB-REJECT). Anticipated 42 → 42 (or 43).

---

## 7. Anticipated ADRs — 1 (possibly 2) ADR for 41 (ADR-0248)

- **ADR-0248** (the load-bearing one) — the circuit-breaker enforcement architecture: the per-priority `circuitBreaker` counter struct on `Cluster` + the non-blocking try-acquire/release at the existing lifecycle seams (`Dial`/admission/`RecordUpstreamResult`/conn-close) + the fail-fast 503-UO + overflow-counter + the `*_open` gauge + the DEFAULT-only-enforce / both-priority-register surface posture. §Context at the 41 SPEC, §Decision/§Consequences at the 41 IMPL per ADR-0044.
- A possible **second ADR (ADR-0249)** for the defer-enforcement posture (the parsed-but-unenforced `max_retries`/`retry_budget`/`max_connection_pools`/`per_host_thresholds` — why they register emit-0 and where their enforcement lands) if it warrants separation — finalized at the SPEC (the phase-39/40 multi-ADR-shape valve). Next-free is ADR-0248.

---

## 8. Deferred items

- **`max_retries` + `retry_budget` enforcement** — blocked on the absent retry substrate; belongs to the retries+hedging family candidate. Parsed + registered emit-0 (`rq_retry_open`, `upstream_rq_retry_overflow`) at 41.
- **`max_connection_pools` enforcement** — HTTP/2 multiplexed-pool semantics; parsed + registered emit-0 (`cx_pool_open`) at 41.
- **`per_host_thresholds` (CircuitBreakers field 2)** — the per-host budget variant; parsed-but-deferred.
- **`track_remaining` + the `remaining_*` gauges** — the budget-headroom readout; not registered at 41.
- **HIGH-priority binding** — blocked on priority routing (no `RouteAction.priority` consumption); the `circuit_breakers.high.*` tree registers but emits 0.
- **A blocking/queuing `max_pending_requests` model** — 41 is fail-fast only; a true bounded backpressure queue is deferred (§2.2 Approach B).
- **Circuit breaking on non-HTTP (TCP/network) upstreams.**
- **The `/clusters` admin per-priority budget/remaining readout enrichment.**

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 39's and phase 40's deferred lists named "**circuit breakers (`Cluster.circuit_breakers`) → its own family row**" among the remaining Upstream-robustness candidates; phase 41 picks it up. The phase-40 note that the `RecordUpstreamResult` seam "is the request-outcome observation point that retries/circuit-breakers may also consume" is REALIZED here: phase 41 consumes that seam as the `max_requests` RELEASE point (not a new channel) — the forward-compatibility the phase-40 brainstorm anticipated. The `max_retries`/`retry_budget` budgets that phase 41 parses-but-defers are the bridge to the future retries+hedging family row (the parse + stat registration land now; the enforcement lands there).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

- **D-CB-PROTO** — the exact `cluster.v3.CircuitBreakers` + `Thresholds` + `RetryBudget` field roster + types + PGV constraints + defaults (esp. `max_connections`/`max_pending_requests`/`max_requests` default 1024, `max_retries` default 3, `max_connection_pools` default ∞/unset, `track_remaining` default false, `retry_budget{budget_percent 20%, min_retry_concurrency 3}`; the `priority` enum DEFAULT=0/HIGH=1); `go mod tidy -diff` EMPTY confirmation.
- **D-CB-LIFECYCLE** — the precise try-acquire/release semantics: when `max_requests` counts a request (admission inclusive of the over-budget one? reject-then-no-count vs count-then-reject), the `max_pending_requests` window against envoy-go's synchronous-acquire model (does it bind at all when there is no standing queue, or only in the dial-in-progress window), whether `max_connections` counts pooled-idle H1 conns or only in-use, the exact 503-UO emission point + whether the connection/request is fully un-made.
- **D-CB-STATS** — the exact `circuit_breakers.<priority>.*_open` gauge names + the `upstream_*_overflow` counter names + the precise overflow-counter→budget mapping (which counter pairs with `max_requests` vs `max_pending_requests`) from a live `/stats` scrape of a circuit-broken cluster; whether both priority trees always emit; the `remaining_*` gauge set under `track_remaining`.
- **D-CB-REJECT** — the envoy-go-strict reject arms for `circuit_breakers` config (duplicate-priority `Thresholds`, budget-value edge cases, `per_host_thresholds` presence; byte-stable wording per ADR-0080); whether a config-parse fuzzer is warranted.
- **D-CB-BACKEND** — the `BlockingHoldResponder` release mechanism (a release endpoint vs a shared channel vs a per-request unblock signal) + how the driver confirms N-in-flight deterministically (poll the `*_open`/`*_active` state vs a backend receipt signal).
- **D-CB-DIFFERENTIAL** — the fixture split (one multi-budget fixture vs per-budget dirs `0074`/`0075`), which budget(s) `0074` exercises, and the cross-side overflow-counter + UO-flag + gauge assertions, all sleepless via the release barrier.

These are EXECUTED IN-SESSION at the 41 SPEC against `envoyproxy/envoy:contrib-v1.37.2` via the live-probe precedent (`reference_docker_probe_bridge_network`), anchoring the ADR-0248 §Context draft.

---

## 11. Prior-phase lessons applied

- **Reuse the forward-compatible seam, don't add a channel** (the phase-40 `RecordUpstreamResult` lesson) — phase 41 hooks `max_requests` release onto the EXISTING completion seam and the try-acquires onto the EXISTING acquire chokepoints; NO new router→cluster function (contrast phase 40, which had to add the seam).
- **Byte-stability via a nil-guard** (the 39.1/40.1 discipline) — the `circuitBreaker` field is nil when no `circuit_breakers` is configured ⇒ try-acquire is a pass-through no-op ⇒ byte-identical to today. Every existing fixture stays green.
- **Emit-0 for surface parity** (the AMEND-OD3-4 outlier local-origin precedent) — the deferred-enforcement budgets (`high.*`, `cx_pool_open`, `rq_retry_open`) register their stat names and emit 0, matching the reference's always-present surface, recorded as departures until enforcement lands.
- **No timing-margin assertions; use a release barrier** (`reference_differential_band_sigma_margin`) — the concurrency-driven `0074` fills the budget with held-open requests and releases via a barrier, NEVER a `time.Sleep`; the overflow assertion is exact (counter delta == 1, gauge == 1), not a band.
- **`reference_docker_probe_bridge_network`** — the SPEC overflow-counter→budget mapping + the `*_open` gauge names are live-pinned from a `/stats` scrape of a circuit-broken reference cluster over a shared bridge network; verify the request path ran (`upstream_rq_total > 0`).
- **The two-ADR-shape valve** (phase 36/38/39/40) — anticipate 1, possibly 2 ADRs for 41 (ADR-0248 + a possible ADR-0249 for the defer-enforcement posture); finalize at SPEC.
- **A single (un-split) family row flips `done` at its IMPL** (ADR-0106 — no parent rollup; the family stays open) — row 41 stays `in-progress` until the single phase lands, then flips `done`; 2 family candidates remain.
- **Digit-inclusive roster regexes** (`reference_proto_roster_extraction_digits`) — when scraping the `circuit_breakers.*` stat roster + the proto field roster at SPEC, use digit-inclusive patterns (the `*_open` / `upstream_rq_*` names carry no digits, but the priority enum + budget defaults do).

---

## 12. Section closeout

Phase 41 (`circuit-breakers`) — LOAD-SHEDDING at the cluster boundary, the THIRD Upstream-robustness-family row. A single (un-split) phase: the per-priority `circuitBreaker` counter struct on `Cluster` + non-blocking try-acquire/release at the EXISTING lifecycle seams (`Dial`/`AcquireH1`/`DialH2` for `max_connections`; router admission + `RecordUpstreamResult` for `max_requests`; the admitted-but-no-connection window for `max_pending_requests`) + the fail-fast 503-UO + overflow counters + the per-priority `*_open` gauges. ENFORCE the core overflow trio (`max_connections`/`max_pending_requests`/`max_requests`) at DEFAULT priority; PARSE-and-register-emit-0 the deferred budgets (`max_retries`/`retry_budget`/`max_connection_pools`/`per_host_thresholds`) + the HIGH-priority tree. REUSES the phase-39/40 cluster-runtime lifecycle seams; ADDS the first cluster-level concurrency accounting; the first feature that REJECTS load. Byte-identical when no `circuit_breakers` is configured. Anticipated at the 41 IMPL: fixtures 75 → 76/77 (`0074` [+ `0075`]), BackendKind tail 35 → 36 (`BlockingHoldResponder`), DECISIONS tail ADR-0247 → ADR-0248 (possibly ADR-0249; next-free ADR-0248), stat surface 1149 → ~1162 (~+13), fuzzers 42 → 42 (or 43), ZERO new packages + ZERO new go.mod modules. ROADMAP row 41 registers `in-progress`; it flips `done` at the single-phase IMPL six-gate (NO split, NO parent rollup per ADR-0106). ALL counts UNCHANGED at this brainstorm commit. The next session authors the 41 SPEC (execute D-CB-* against `contrib-v1.37.2`; anchor the ADR-0248 §Context).
