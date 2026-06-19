# Phase 41 SPEC — `circuit_breakers`: per-priority fail-fast overflow budgets — the `max_requests` keystone of the THIRD Upstream-robustness-family row

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-41 BRAINSTORM (`docs/envoy-go/phases/41-circuit-breakers/BRAINSTORM.md`, commit `306e26a`). This SPEC charters phase **41** — circuit breaking on the `max_requests` budget (the cleanly-modelable keystone), with the full per-priority `circuit_breakers.*` stat surface registered for parity. Counts at SPEC commit UNCHANGED (stat surface **1149** / fixtures **75** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0247**, next-free **ADR-0248**). The §11 D-CB-* empirical pins were EXECUTED IN-SESSION (2026-06-19) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land **load-shedding** at the cluster boundary: a cluster configured with `circuit_breakers` caps the number of concurrent in-flight upstream requests at `max_requests`; a request that would exceed the budget is **rejected fail-fast** with a `503` + the `UO` (UpstreamOverflow) response flag — never queued — and increments the cluster's `upstream_rq_pending_overflow` counter while the `circuit_breakers.<priority>.rq_open` gauge reads 1. This is the project's FIRST feature that REJECTS load (active HC + outlier detection only route *around* unhealthy hosts) and its FIRST cluster-level concurrency accounting. It REUSES the phase-39/40 cluster-runtime substrate — the router admission path and the per-request completion lifecycle — adding only a per-priority counter struct and a try-acquire/release overlay; NO new router→cluster channel, NO background goroutine.

41 is a self-contained single phase (NO ADR-0045 split): the `circuitBreaker` accounting struct + the `max_requests` try-acquire/release + the 503-UO overflow + the parse/reject surface + the full-parity stat block + `0074`.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments to the BRAINSTORM design. The headline: the BRAINSTORM settled Fork 1 as "the core overflow trio" (`max_connections` + `max_pending_requests` + `max_requests`); the live probe revealed that only `max_requests` maps cleanly onto envoy-go's synchronous, queue-less upstream model, so the enforcement scope was narrowed to the `max_requests` keystone (human-confirmed, 2026-06-19), with the cx/pending budgets registered-for-parity-but-enforcement-deferred.

- **AMEND-CB1 (scope — the `max_requests` keystone; `max_connections` + `max_pending_requests` enforcement DEFERRED).** Live finding (D-CB-LIFECYCLE): in the reference the fail-fast `503/UO` ONLY ever comes from a *request* budget. `max_connections` saturation does NOT fail-fast — it pushes the request into the pending queue (a soft throttle: `cx_open` → 1, `upstream_cx_overflow`++, but the request proceeds to pend, not 503). `max_pending_requests` bounds that connection-wait queue. envoy-go's upstream path is synchronous-acquire-per-request with NO standing pending queue (the BRAINSTORM §8 deferred a queuing model), so `max_pending_requests` ("requests waiting for a connection") has no faithful expression, and a fail-fast `max_connections` cap would be a behavioral DEPARTURE (the reference pends). Phase 41 therefore ENFORCES **`max_requests` ONLY** — the active-in-flight-request concurrency cap, which envoy-go models EXACTLY (clean cross-side `503/UO` parity). `max_connections` + `max_pending_requests` are PARSE-ACCEPTED + their stat names REGISTERED (emit-0) for surface parity; their enforcement is DEFERRED to the future **per-protocol connection pooling** Upstream-robustness family row (where the pool/queue semantics belong) — a recorded departure. *(Human-confirmed scope decision; the BRAINSTORM Fork-1 trio narrowed to the keystone.)*
- **AMEND-CB2 (the shared overflow counter — there is NO `upstream_rq_overflow`).** Live finding (D-CB-STATS): a `max_requests` overflow increments **`upstream_rq_pending_overflow`** — the SAME counter as a `max_pending_requests` overflow. `cluster.<n>.upstream_rq_overflow` does NOT exist in the reference (a full cluster-stat-tree grep returns nothing). The two request budgets are distinguished ONLY by their gauges (`rq_open` vs `rq_pending_open`), never by the counter. envoy-go REPLICATES: the `max_requests` breaker increments `upstream_rq_pending_overflow` (NOT a phantom `upstream_rq_overflow`) and sets the `rq_open` gauge.
- **AMEND-CB3 (stat surface — +14, the full parity block, both priorities, emit-0 for the deferred budgets).** Live finding (D-CB-STATS): a circuit-breaker cluster emits, per `RoutingPriority` subtree, FIVE `*_open` gauges (`cx_open`, `cx_pool_open`, `rq_open`, `rq_pending_open`, `rq_retry_open`) — all gauges, emitted as soon as `circuit_breakers` is set on the cluster, BOTH `default.*` AND `high.*` (nothing needs to route by priority) — plus FOUR cluster-level overflow COUNTERS (`upstream_cx_overflow`, `upstream_cx_pool_overflow`, `upstream_rq_pending_overflow`, `upstream_rq_retry_overflow`). envoy-go registers the full block: **+10 gauges** (5 × {default, high}) + **+4 counters** = **+14**, scoped to clusters with `circuit_breakers` (existing fixtures unaffected — a recorded departure: the reference emits the overflow counters on ALL clusters; envoy-go scopes them to circuit-breaker clusters, the outlier-stat-scoping precedent). Only `circuit_breakers.default.rq_open` (set 1 at/over `max_requests`) + `upstream_rq_pending_overflow` (++ on `max_requests` overflow) go LIVE at 41; the other 12 emit 0 (deferred enforcement / deferred priority binding — the AMEND-OD3-4 emit-0-for-parity precedent). Surface **1149 → 1163**.
- **AMEND-CB4 (the overflow response — `503` + `UO` + the code-details).** Live finding (D-CB-LIFECYCLE): a fast-fail overflow yields `RESPONSE_CODE 503`, `RESPONSE_FLAGS UO`, `RESPONSE_CODE_DETAILS upstream_reset_before_response_started{overflow}`. envoy-go emits the `503` via the existing local-reply path (the `AcquireH1`-failure `ActionResponse{Status: 503, Headers: localReplyHeaders(0)}` shape, `router.go:610`); envoy-go HAS a `%RESPONSE_FLAGS%` access-log token (`internal/accesslog/format.go`), so it SETS the `UO` flag on the overflow path where the surface allows (a PLAN/IMPL pin — §12). The cross-side DIFFERENTIAL asserts the robust pair (the `503` status + the `upstream_rq_pending_overflow` counter delta + the `rq_open` gauge), NOT the access-log line (fixtures compare `/stats` + response, not log output); the `UO` flag + the code-details string are an internal-correctness target, not a cross-side assertion.
- **AMEND-CB5 (effective defaults — 1024 / 1024 / 1024 / 3 / ∞).** Live finding (D-CB-PROTO, confirmed via the `remaining_*` gauges under `track_remaining`): unset budgets default to `max_connections 1024`, `max_pending_requests 1024`, `max_requests 1024`, `max_retries 3`, `max_connection_pools UINT64_MAX` (unlimited). So `max_requests` defaults to 1024 — the breaker is effectively OFF unless a smaller value is configured. envoy-go applies the 1024 default for an absent `max_requests` on a `circuit_breakers`-configured cluster (the breaker only bites when `max_requests` is explicitly small).
- **AMEND-CB6 (reject surface — THIN; PGV-mirrors + one house reject; NO new fuzzer).** Live finding (D-CB-REJECT, `--mode validate`): the reference's PGV reject surface on `CircuitBreakers` is thin — only `Thresholds.priority` enum-range (`ThresholdsValidationError.Priority: value must be one of the defined enum values`) and `RetryBudget.budget_percent ∈ [0,100]` (`PercentValidationError.Value: value must be inside range [0, 100]`) bite; the budget `UInt32Value`s are UNBOUNDED, and duplicate-priority `Thresholds` + `per_host_thresholds` are ACCEPTED. envoy-go MIRRORS the two PGV rejects (priority range; retry_budget percent — validated even though `retry_budget` enforcement is deferred, the 40.1 `enforcing_* > 100`-validated-though-deferred precedent) with house wording (§6), ADDS its own strict **duplicate-priority** reject (ADR-0080 — cleaner than guessing the reference's silent merge), and SILENT-IGNORES `per_host_thresholds` (deferred, additive). NO new fuzzer — `circuit_breakers` is a config-parse surface (no hand-rolled wire decoder), unit-tested not fuzzed (the 40.x precedent); fuzzers STAY **42**.
- **AMEND-CB7 (+1 BackendKind — `BlockingHoldResponder`).** D-CB-BACKEND: the differential must DETERMINISTICALLY fill `max_requests` with N concurrent in-flight requests, which needs a backend that HOLDS each request open until released (no existing BackendKind blocks). `0074` adds a +1 in-process `BlockingHoldResponder` (BackendKind **35 → 36**) — accepts a request, blocks until the driver signals release, then responds 200. The driver fills N slots, polls `rq_open` to 1, fires the N+1th (asserts 503/overflow), then releases — SLEEPLESS (no `time.Sleep`; the release barrier + poll-to-converge provide determinism, the `reference_differential_band_sigma_margin` lesson). The shared backend serves BOTH sides over the bridge network; the release-signal mechanism is a PLAN pin (§12).

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0248 (the per-priority circuit-breaker enforcement architecture — the `circuitBreaker` accounting struct + the `max_requests` try-acquire/release at the admission/completion lifecycle + the 503-UO overflow + the full-parity stat surface + the DEFAULT-only-enforce / both-priority-register posture, incl. the cx/pending defer) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 41 IMPL per ADR-0044. The narrowed scope FOLDS the BRAINSTORM's anticipated second ADR (ADR-0249, the defer-enforcement posture) INTO ADR-0248 (§7 — a single ADR now suffices). DECISIONS tail STAYS ADR-0247 at this SPEC; next-free ADR-0248. The §10 BRAINSTORM D-CB pins are RESOLVED in §11; the PLAN/IMPL D-questions are §12.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **`max_connections` + `max_pending_requests` ENFORCEMENT** (AMEND-CB1) — the connection-pool/queue semantics; deferred to the future per-protocol connection pooling family row. Their gauges `cx_open` + `rq_pending_open` register emit-0, and `upstream_cx_overflow` registers emit-0, for parity. (NOTE: `upstream_rq_pending_overflow` is NOT emit-0 — it is the LIVE counter that `max_requests` drives at 41 [AMEND-CB2 — the same counter `max_pending_requests` would share]; it is named here only to flag the shared-counter overlap, not as a deferred stat.)
- **`max_retries` + `retry_budget` enforcement** — blocked on the absent retry substrate (the retries+hedging family candidate); `rq_retry_open` + `upstream_rq_retry_overflow` registered emit-0; `retry_budget.budget_percent` parse-validated.
- **`max_connection_pools` enforcement** (HTTP/2 multiplexed pools) — `cx_pool_open` + `upstream_cx_pool_overflow` registered emit-0.
- **`per_host_thresholds` (CircuitBreakers field 2)** — silent-ignored (additive, deferred).
- **`track_remaining` + the `remaining_*` gauges** (`remaining_cx`/`remaining_pending`/`remaining_rq`/`remaining_retries`/`remaining_cx_pools`) — the budget-headroom readout; NOT registered at 41 (only emit under `track_remaining`).
- **HIGH-priority binding** — blocked on priority routing (no `RouteAction.priority` consumption); the `circuit_breakers.high.*` subtree registers but emits 0.
- A blocking/queuing pending model; circuit breaking on non-HTTP (TCP/network) upstreams; the `/clusters` admin per-priority budget readout; the retry-budget dynamic-concurrency model.

---

## 3. The `circuitBreaker` accounting struct + the `max_requests` try-acquire/release + the 503-UO overflow (ADR-0248)

### 3.0 Split disposition — single phase (NO ADR-0045 split)

41 = the accounting struct + the `max_requests` enforcement + the parse/reject + the +14 stat block + `0074`. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~250–400 prod LoC / ~12–16 tasks — comfortably under `> ~25 tasks OR > ~1500 LoC`; single flat phase). The cx/pending defer (AMEND-CB1) keeps the envelope small.

### 3.1 The per-priority accounting struct

A new `circuitBreaker` on `Cluster` (`cluster.go:85`, alongside `health`/`outlier`; new file `internal/cluster/circuitbreaker.go`):
```go
// circuitBreaker holds per-RoutingPriority concurrency budgets + live counters.
// Nil when no circuit_breakers is configured (try-acquire is a pass-through). (ADR-0248)
type circuitBreaker struct {
	// indexed by RoutingPriority (0=DEFAULT, 1=HIGH); HIGH parsed+registered but
	// never bound at 41 (nothing routes by priority — AMEND-CB3).
	prio [2]cbPriority
}

type cbPriority struct {
	maxRequests    int64        // the enforced budget (default 1024 — AMEND-CB5)
	activeRequests atomic.Int64 // in-flight upstream requests for this priority

	// the +14 stat handles (registered for parity; only default.rq_open +
	// the cluster-level upstream_rq_pending_overflow go live at 41 — AMEND-CB3).
	rqOpen  *stats.Gauge   // 1 while activeRequests >= maxRequests
	// cxOpen/cxPoolOpen/rqPendingOpen/rqRetryOpen registered emit-0 (deferred).
}
```
The cluster-level overflow counters (`upstream_*_overflow`) live on `Cluster` beside the existing `upstream_rq_*` handles (§5; `manager.go:114`). The struct carries NO endpoint/host state — circuit breaking is cluster-wide per-priority accounting, NOT per-host (the deliberate CONTRAST with the phase-40 `hostHealth` ejection dimension).

### 3.2 The `max_requests` try-acquire / release lifecycle (synchronous, fail-fast)

The enforcement is an overlay on the EXISTING admission path — `doH1ClusterAction` (`router.go:588`) + `doH2ClusterAction` (`router_h2.go:57`) — the points where the route has resolved a cluster and the upstream call is about to begin:

1. **Try-acquire at admission (top of `do{H1,H2}ClusterAction`, before `AcquireH1`/`DialH2`):** if `c.circuitBreaker != nil`, call `cb.tryAcquireRequest(DEFAULT)`:
   - atomically: if `activeRequests >= maxRequests` ⇒ set `rqOpen = 1`, increment `c.upstreamRqPendingOverflow`, return `false` (REJECTED); else `activeRequests++`, return `true`.
   - on `false` ⇒ the action returns the overflow local-reply IMMEDIATELY: `ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}` (the `router.go:610` shape), with the `UO` response flag set (AMEND-CB4) and NO upstream connection/request made. (`IncStatusClass(503)` per the existing 503 paths.)
2. **Release on completion (a `defer` at the same admission scope):** on `true`, `defer cb.releaseRequest(DEFAULT)` — atomically `activeRequests--`; if `activeRequests < maxRequests` ⇒ clear `rqOpen = 0`. The `defer` fires on EVERY exit path (success, upstream 5xx, dial/acquire failure) — so the budget is released unconditionally for every admitted request. NOTE: release is endpoint-free (cluster-wide accounting) so it does NOT piggyback on `RecordUpstreamResult` (which is per-host + skips the no-host paths) — the `defer` at the admission scope is the load-bearing release point (a refinement of the BRAINSTORM's "release at RecordUpstreamResult").

`maxRequests` is the parsed `Thresholds[DEFAULT].max_requests` (or the 1024 default). `activeRequests` counts only DEFAULT-priority requests (every request is DEFAULT at 41). The HIGH budget parses but is never consulted (no HIGH traffic).

### 3.3 Byte-stability

When `c.circuitBreaker == nil` (no `circuit_breakers` configured), `tryAcquireRequest`/`releaseRequest` are not called (the `nil`-guard at the admission top) ⇒ the admission path is **byte-identical to today** (every existing fixture stays green). The +14 stats register ONLY on `circuit_breakers` clusters (the scoped `registerClusterMetrics` block — `manager.go:160` health-scoped precedent).

---

## 4. Framework primitives — the accounting struct + the try-acquire overlay over the phase-39/40 substrate + 0 new packages + 0 new go.mod deps

- NEW: the `circuitBreaker`/`cbPriority` struct + `tryAcquireRequest`/`releaseRequest` in `internal/cluster/circuitbreaker.go`; a `circuitBreaker` field on `Cluster`; the +14 stat handles; the two admission call sites (`do{H1,H2}ClusterAction`); the `parseCircuitBreakers` in `buildCluster` (`manager.go:363`, after `outlier_detection` `:416`); the overflow 503-UO local-reply.
- REUSED: the router admission path (`do{H1,H2}ClusterAction`) + the local-reply 503 shape (`router.go:610`); the `registerClusterMetrics` stat-injection (`manager.go:112`); the `%RESPONSE_FLAGS%` access-log token (`internal/accesslog/format.go`) for the `UO` flag; the `reference_docker_probe_bridge_network` differential pattern.
- ZERO new Go packages. ZERO new go.mod modules (`cluster.v3.CircuitBreakers` is in the existing go-control-plane v1.32.4 dep; `go mod tidy -diff` EMPTY — D-CB-PROTO, confirmed).

---

## 5. Proto-field roster (per §11 D-CB-PROTO)

`Cluster.circuit_breakers` = `Cluster` field 10 (confirmed `protobuf:"bytes,10,...`) → `cluster.v3.CircuitBreakers` (`config/cluster/v3/circuit_breaker.pb.go`). The message:

| # | Field | Type | 41 role |
|---|-------|------|---------|
| 1 | `thresholds` | `[]Thresholds` | the per-priority budget list (keyed by `priority`) |
| 2 | `per_host_thresholds` | `[]Thresholds` | SILENT-IGNORE (deferred — AMEND-CB1) |

`Thresholds` (the `#` column is the PROTO FIELD TAG, not listing order):

| # | Field | Type | Default | 41 role |
|---|-------|------|---------|---------|
| 1 | `priority` | `RoutingPriority` enum (DEFAULT=0/HIGH=1) | DEFAULT | the subtree key; PGV enum-range reject (§6) |
| 2 | `max_connections` | UInt32Value | 1024 | PARSE-ACCEPT; enforcement DEFERRED, `cx_open` emit-0 |
| 3 | `max_pending_requests` | UInt32Value | 1024 | PARSE-ACCEPT; enforcement DEFERRED, `rq_pending_open` emit-0 |
| 4 | `max_requests` | UInt32Value | 1024 | **the ENFORCED budget** (AMEND-CB1); `rq_open` LIVE |
| 5 | `max_retries` | UInt32Value | 3 | PARSE-ACCEPT; enforcement DEFERRED, `rq_retry_open` emit-0 |
| 6 | `track_remaining` | bool | false | the `remaining_*` toggle — NOT consumed at 41 (deferred) |
| 7 | `max_connection_pools` | UInt32Value | ∞ (UINT64_MAX) | PARSE-ACCEPT; enforcement DEFERRED, `cx_pool_open` emit-0 |
| 8 | `retry_budget` | `RetryBudget{budget_percent Percent, min_retry_concurrency UInt32Value}` | 20% / 3 | PARSE-VALIDATE percent∈[0,100] (§6); enforcement DEFERRED |

Defaults confirmed LIVE via the `remaining_*` gauges (AMEND-CB5). All non-`max_requests` fields PARSE-ACCEPTED-and-(IGNORED|emit-0). `go mod tidy -diff` EMPTY → ZERO new module.

---

## 6. PARSE-REJECT roster (per §11 D-CB-REJECT + ADR-0080)

envoy-go hand-rolls its own byte-stable rejects (the `parseOutlierDetection` precedent), mirroring the thin reference PGV envelope. House wording `cluster: %q: circuit_breakers: <reason>`:
- `priority` not in {DEFAULT, HIGH} (reference PGV: `ThresholdsValidationError.Priority: value must be one of the defined enum values`).
- `retry_budget.budget_percent` outside `[0, 100]` (reference PGV: `PercentValidationError.Value: value must be inside range [0, 100]`) — validated even though `retry_budget` enforcement is deferred (the 40.1 deferred-but-validated precedent).
- **duplicate `priority`** across `thresholds` (envoy-go's OWN strict reject per ADR-0080 — the reference SILENTLY accepts; envoy-go rejects rather than guess a merge; house wording at PLAN).

The budget `UInt32Value`s are UNBOUNDED (no reject — any value including 0 accepted; `max_requests: 0` ⇒ the breaker rejects ALL requests, a valid config). `per_host_thresholds` is SILENT-IGNORED (NOT rejected — additive deferral). All reject arms unit-level (no boot-reject dir — the `0069`/`0064` precedent). The exact house wording is pinned at the PLAN (§12).

---

## 7. Stat surface — +14 (1149 → 1163) (per §11 D-CB-STATS + AMEND-CB3)

Emitted ONLY on clusters with `circuit_breakers` (existing fixtures unaffected — the outlier-scoping precedent; a recorded departure from the reference's always-on cluster overflow counters). Two groups:

**A. Per-priority `*_open` gauges — `cluster.<n>.circuit_breakers.<default|high>.` (10 = 5 × 2):**
1. `cx_open` — gauge (emit-0; `max_connections` enforcement deferred).
2. `cx_pool_open` — gauge (emit-0; `max_connection_pools` deferred).
3. `rq_open` — gauge — **LIVE for `default`** (set 1 while `activeRequests >= max_requests`, else 0); `high.rq_open` emit-0.
4. `rq_pending_open` — gauge (emit-0; `max_pending_requests` deferred).
5. `rq_retry_open` — gauge (emit-0; `max_retries` deferred).

**B. Cluster-level overflow counters — `cluster.<n>.` (4):**
6. `upstream_cx_overflow` — counter (emit-0; `max_connections` deferred).
7. `upstream_cx_pool_overflow` — counter (emit-0; `max_connection_pools` deferred).
8. `upstream_rq_pending_overflow` — counter — **LIVE** (++ on a `max_requests` overflow; AMEND-CB2 — the SAME counter `max_pending_requests` would use; there is NO `upstream_rq_overflow`).
9. `upstream_rq_retry_overflow` — counter (emit-0; `max_retries` deferred).

LIVE at 41: `circuit_breakers.default.rq_open` + `upstream_rq_pending_overflow`. The other 12 register + emit 0 (deferred enforcement / deferred HIGH binding — the AMEND-OD3-4 emit-0-for-parity precedent). DEFERRED departures (NOT registered): the `remaining_*` gauges (`track_remaining`-gated). Surface **1149 → 1163**.

---

## 8. Differential fixture taxonomy (+1: `0074` cross-side concurrency-driven overflow)

### 8.1 `0074-circuit-breaker-max-requests` (cross-side; +1 BackendKind)

An HTTP listener → a cluster `c_cb {1 `BlockingHoldResponder` backend}` (the +1 BackendKind) with `circuit_breakers: { thresholds: [ { priority: DEFAULT, max_requests: <N> } ] }`, on BOTH the subject and the reference (`contrib-v1.37.2`). The driver (SLEEPLESS — the release-barrier + poll-to-converge pattern, NO `time.Sleep`):
1. Fire **N concurrent** requests; each blocks at the `BlockingHoldResponder` (held open) ⇒ occupies one `max_requests` slot.
2. **POLL `/stats` until `cluster.c_cb.circuit_breakers.default.rq_open == 1` on BOTH sides** (the breaker is open — N slots filled; no fixed sleep).
3. Fire the **(N+1)th** request ⇒ assert on BOTH sides: `503` status + the `cluster.c_cb.upstream_rq_pending_overflow` delta `>= 1` + `circuit_breakers.default.rq_open == 1`. Verify `upstream_rq_total > 0` reference-side (decode-ran guard).
4. **Release the barrier** ⇒ the N held requests complete (200) ⇒ poll `rq_open` back to 0 ⇒ cross-side parity on the final `rq_open == 0`.

The shared `BlockingHoldResponder` serves BOTH the in-process subject AND the reference container over the bridge network (the release-signal mechanism — a control endpoint vs a shared channel — is a PLAN pin, §12). 2 `-count=1` deliberate breaks: (A) `max_requests` never enforced (`tryAcquireRequest` always returns true) ⇒ the (N+1)th succeeds ⇒ no 503 / no overflow delta ⇒ FAIL; (B) the `rq_open` gauge / `upstream_rq_pending_overflow` counter not wired ⇒ the cross-side parity assert FAILS. The constants (N / backendCount / convergeDeadline / the held-request count) single-sourced (`reference_fixture_workload_constant_desync`). Only ONE fixture — phase 41 enforces only `max_requests`, so there is NO `max_connections`/`max_pending_requests` fixture (their enforcement is deferred — AMEND-CB1).

### 8.2 The +1 BackendKind (`BlockingHoldResponder`)

`BlockingHoldResponder` BackendKind 36 (name pinned at PLAN) — an in-process `net.Listen` + `go accept…` responder (the `HTTP503Responder`/`acceptHTTPEchoCounting` precedent, NOT a subprocess): per request, BLOCK on a per-backend release signal, then write `HTTP/1.1 200 OK` + a `backend-<idx>:` body (host attribution via the `backendIdxFromBody` precedent). The driver controls the release (D-CB-BACKEND / §12). BackendKind tail **35 → 36**.

### 8.3 NO new fuzzer

`circuit_breakers` is config-parse (no new wire decoder); the parse/reject is unit-tested (the 40.x precedent). Fuzzers STAY **42** (AMEND-CB6).

---

## 9. Behavior-contract delta (the 41 bundle; ADR-0052 atomic landing)

A new `### Cluster — load-shedding (circuit breakers)` subsection in BEHAVIOR_CONTRACT.md: the `max_requests` per-priority concurrency cap (try-acquire at admission, defer-release on completion; DEFAULT-only enforcement); the fail-fast 503 + `UO` flag + the `upstream_rq_pending_overflow` counter (AMEND-CB2 — the shared counter, no `upstream_rq_overflow`) + the `rq_open` gauge; the full-parity +14 stat block (emit-0 for the deferred cx/pending/pool/retry budgets + the HIGH subtree); the deferred-enforcement departures (cx/pending → connection-pooling row; retry → retries family; the reference's max_connections-pends-not-fails semantics); byte-identical when no `circuit_breakers`. The stat-surface block advances 1149 → 1163.

---

## 10. Per-task structure (~12–16 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) `parseCircuitBreakers` (the `thresholds[]` keyed by priority + defaults + the 3 reject arms + per_host_thresholds silent-ignore) + unit tests; (3) the `circuitBreaker`/`cbPriority` struct + `tryAcquireRequest`/`releaseRequest` + the `rq_open`-gauge/`upstream_rq_pending_overflow`-counter logic + unit tests (acquire/reject/release/gauge-toggle/`max_requests:0`); (4) the +14 stat registrations (scoped to `circuit_breakers` clusters; 1149→1163; the 12 emit-0); (5) the admission try-acquire + defer-release at `do{H1,H2}ClusterAction` + the overflow 503-UO local-reply + the `UO` flag; (6) the `circuitBreaker` field + the `buildCluster` wiring; (7) the +1 `BlockingHoldResponder` BackendKind + the release mechanism; (8) the `0074` fixture; (9) `0074` deliberate-break + 20-run flake; (10) full 76-dir differential + six-gate; (11) ADR-0248 body + BEHAVIOR_CONTRACT; (12) completion bundle + ROADMAP row 41 → `done`. The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO split).

---

## 11. SPEC-time empirical-pin block (D-CB-* — executed IN-SESSION 2026-06-19)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge network; a slot-holding backend; concurrent load; `--mode validate` reject probes; request path verified `upstream_rq_total>0`) + the go-control-plane v1.32.4 module cache.

| Pin | Disposition |
|-----|-------------|
| **D-CB-PROTO** | CONFIRMED. `circuit_breakers` = `Cluster` field 10; `CircuitBreakers{thresholds[] (1), per_host_thresholds[] (2)}`; `Thresholds{priority, max_connections, max_pending_requests, max_requests, max_retries, retry_budget, track_remaining, max_connection_pools}`; effective defaults 1024/1024/1024/3/∞ (via `remaining_*`); `go mod tidy -diff` EMPTY → ZERO new module. |
| **D-CB-STATS** | CONFIRMED. Per-priority (BOTH default+high) 5 `*_open` GAUGES (`cx_open`/`cx_pool_open`/`rq_open`/`rq_pending_open`/`rq_retry_open`) + 4 cluster COUNTERS (`upstream_cx_overflow`/`upstream_cx_pool_overflow`/`upstream_rq_pending_overflow`/`upstream_rq_retry_overflow`); `upstream_rq_overflow` does NOT exist (AMEND-CB2); `remaining_*` gauges are `track_remaining`-gated. 41 registers the +14 parity block; LIVE = `default.rq_open` + `upstream_rq_pending_overflow` (§7). |
| **D-CB-LIFECYCLE** | PINNED. Overflow ⇒ `503` + `UO` + `upstream_reset_before_response_started{overflow}`. `max_requests` overflow ⇒ `rq_open=1` + `upstream_rq_pending_overflow`++ (AMEND-CB2/CB4). `max_connections` saturation ⇒ `cx_open=1` + `upstream_cx_overflow`++ but NO 503 (requests pend — the soft throttle) ⇒ envoy-go enforces `max_requests` ONLY (AMEND-CB1). |
| **D-CB-REJECT** | PINNED. PGV thin: only `ThresholdsValidationError.Priority: value must be one of the defined enum values` + `PercentValidationError.Value: value must be inside range [0, 100]` (retry_budget) bite; budget values unbounded; duplicate-priority + `per_host_thresholds` ACCEPTED. envoy-go mirrors the 2 PGV rejects + adds a strict duplicate-priority reject + silent-ignores per_host_thresholds (§6, AMEND-CB6). NO new fuzzer. |
| **D-CB-BACKEND** | PINNED. No existing BackendKind blocks ⇒ +1 (`BlockingHoldResponder`, in-process, holds-until-released). The driver fills N slots, polls `rq_open`→1, fires N+1, releases — sleepless (AMEND-CB7). |
| **D-CB-DIFFERENTIAL** | PINNED. ONE fixture `0074-circuit-breaker-max-requests` (max_requests only — cx/pending enforcement deferred); cross-side assert `503` + `upstream_rq_pending_overflow` delta + `rq_open` gauge; 2 deliberate breaks; the shared `BlockingHoldResponder` over the bridge net (§8). |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S41-1** the exact envoy-go house reject wording for §6 (the `cluster: %q: circuit_breakers: …` strings — priority-range / retry_budget-percent / duplicate-priority).
- **D-S41-2** the `max_requests` absent-vs-0 disposition (absent ⇒ the 1024 default per AMEND-CB5; explicit 0 ⇒ reject-all-requests — confirm the reference treats 0 as a hard cap, not "unset").
- **D-S41-3** the `UO` response-flag emission mechanism (whether the overflow local-reply can set the `%RESPONSE_FLAGS%` `UO` bit through the existing accesslog surface, or it is a recorded departure at 41 — the differential does NOT depend on it, AMEND-CB4).
- **D-S41-4** the `BlockingHoldResponder` release mechanism (a control endpoint the driver hits vs a shared in-process channel) + how the driver confirms N-in-flight (poll `rq_open`→1 vs a backend receipt signal) for BOTH the in-process subject AND the reference container.
- **D-S41-5** `0074` constants (N / backendCount / convergeDeadline / held-count / refContainerListenerPort) single-sourced.
- **D-S41-6** the priority-indexing of the `circuitBreaker.prio[2]` + whether HIGH parses into a never-consulted slot or is dropped-with-a-registered-emit-0-stat-tree (the latter, per AMEND-CB3).
- **D-S41-7** the ADR-0045 final split-gate re-check (anticipated NO SPLIT).

---

## 13. ADR continuity — the ADR-0248 §Context DRAFT (anchored here; full entry lands at the 41 IMPL)

**ADR-0248 §Context (draft).** Phases 39–40 established the cluster-runtime substrate for upstream robustness: active health checking (ADR-0242/0243) and passive outlier detection (ADR-0245/0246/0247), both of which route AROUND unhealthy hosts via the build-time-injected health-aware LB pick. Circuit breaking (`Cluster.circuit_breakers`) is the third dimension — it SHEDS load against a healthy-but-saturated cluster, the project's first feature that rejects load and its first cluster-level concurrency accounting. The phase-41 BRAINSTORM settled a per-priority fail-fast architecture over the existing lifecycle seams; the §11 live pins then NARROWED the enforced scope from the "core overflow trio" to the `max_requests` keystone (human-confirmed): the reference's fail-fast 503/UO comes ONLY from a request budget, `max_connections` is a soft throttle that pends rather than fails, and `max_pending_requests` needs the connection-pool queue that envoy-go's synchronous model lacks — so those two are registered-for-parity-but-enforcement-deferred to the future connection-pooling row, while `max_requests` (the active-request concurrency cap) maps EXACTLY onto envoy-go's model. The design: a per-priority `circuitBreaker` counter struct on `Cluster`; a synchronous try-acquire at router admission (`do{H1,H2}ClusterAction`) + a defer-release on completion; a fail-fast `503` + `UO` + `upstream_rq_pending_overflow` (the shared counter — there is no `upstream_rq_overflow`) + the `rq_open` gauge on exhaustion; the full per-priority `*_open` + cluster `*_overflow` stat block (+14) registered for parity, DEFAULT-enforce only with the HIGH subtree emit-0; byte-identical when no `circuit_breakers`. The single ADR-0248 absorbs the defer-enforcement posture (the BRAINSTORM's anticipated ADR-0249 is no longer needed at the narrowed scope). §Decision + §Consequences land at the 41 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1149** / fixtures **75** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0247** (next-free **ADR-0248**). ROADMAP row 41 STAYS `in-progress` (the SPEC note appended). Anticipated at the 41 IMPL: fixtures 75 → 76 (`0074`), BackendKind tail 35 → 36 (`BlockingHoldResponder`), DECISIONS tail ADR-0247 → ADR-0248 (next-free ADR-0249), stat surface 1149 → 1163 (+14), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Next → the phase-41 PLAN (`superpowers:writing-plans`).
