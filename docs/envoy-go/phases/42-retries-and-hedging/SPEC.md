# Phase 42.1 SPEC — `retry_policy`: a re-attempt loop over the existing single-attempt driver — the FIRST leg of the FOURTH Upstream-robustness-family row

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-42 BRAINSTORM (`docs/envoy-go/phases/42-retries-and-hedging/BRAINSTORM.md`, commit `1aba3956`). This SPEC charters phase **42.1** — the HTTP retry loop (`RouteAction.retry_policy`): `retry_on` classification + `num_retries` + `retry_back_off` + the phase-41 `retry_budget` activation, as a `retryExecutor` WRAPPING the existing single-attempt `doH1ClusterAction`/`doH2ClusterAction`. The 42.2 hedging leg (`HedgePolicy` + `per_try_timeout`) follows in a later session. Counts at SPEC commit UNCHANGED (stat surface **1163** / fixtures **76** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0248**, next-free **ADR-0249**). The §11 D-RT-* empirical pins were EXECUTED IN-SESSION (2026-06-19) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land **request recovery** at the route boundary: a route configured with `retry_policy` re-attempts a failed upstream request against an LB-re-picked host. The single-attempt driver `doH1ClusterAction`/`doH2ClusterAction` already buffers the full upstream response into `ActionResponse{Status,Headers,Body,Close}` and already re-runs circuit-breaker admission (`TryAcquireRequest`) + the outcome seam (`RecordUpstreamResult`) on every call — so a retry is exactly *call the driver again*: a `retryExecutor` classifies the buffered `ActionResponse.Status` against a parsed `retry_on` bitset and, on a retriable outcome with `num_retries` + `retry_budget` slots remaining, sleeps an exponential backoff, replays the already-buffered request body, and re-invokes the driver. This is the project's FIRST request-replay control plane and its FIRST feature that RECOVERS a single request (active HC + outlier route *around* unhealthy hosts; circuit breaking *sheds* load). It ACTIVATES the dormant phase-41 `retry_budget` slot — the emit-0 `circuit_breakers.<priority>.rq_retry_open` gauge + the `upstream_rq_retry_overflow` counter (`circuitbreaker.go:78,83`) flip LIVE. Byte-identical when no `retry_policy` (nil-guard).

42.1 is the FIRST leg of the pre-authorized 42.1/42.2 by-feature split: the retry loop + the `retry_on` classifier + the `retry_policy` parse/reject + the `retry_budget` activation + the +5 retry stat surface + the `0075` differential. Hedging (`HedgePolicy`, `per_try_timeout`) is 42.2.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments/refinements to the BRAINSTORM design.

- **AMEND-RT1 (retry_on — the enforced subset + the transport-vs-response distinction).** Live finding (D-RT-RETRYON): under `retry_on`, a token matches a SPECIFIC failure kind. Confirmed by probe: `gateway-error` retries a `502` response (`c_gw502.upstream_rq_retry=2`); `5xx` retries a `500` (`c_s500b`); `retriable-status-codes` + `retriable_status_codes:[500]` retries a `500` (`c_s500`); `connect-failure` retries an ACTUAL connection refusal (`c_dead`, final 503) but does **NOT** retry a `502` *response* (`c_gw502b.upstream_rq_retry=0` — the backend connected fine and returned 502). So `connect-failure`/`reset` are TRANSPORT-level (the router's local-origin failure shape), distinct from a 5xx RESPONSE. 42.1 ENFORCES `{5xx, gateway-error, connect-failure, reset, retriable-status-codes (+ the explicit retriable_status_codes[] list)}`: `5xx` ⇒ any upstream `ActionResponse.Status ∈ [500,599]`; `gateway-error` ⇒ `{502,503,504}`; `retriable-status-codes` ⇒ `Status ∈ retriable_status_codes[]`; `connect-failure`/`reset` ⇒ the local-origin failure the router already records (the dial/connection-failure path that yields a synthesized 503/502). PARSE-ACCEPT-but-defer `{retriable-headers, envoy-ratelimited, grpc-*}` (no header-match / ratelimit-bridge / gRPC-trailer substrate). `retry_on` is freeform (NO PGV — confirmed §11 D-RT-PROTO); unknown/deferred tokens are SILENTLY IGNORED (the reference ignores unknown tokens), so the reject surface adds nothing for `retry_on` itself (AMEND-RT5).
- **AMEND-RT2 (stat surface — +5 new counters per retry cluster + the 2 phase-41 stats flip LIVE; SCOPED — a departure).** Live finding (D-RT-STATS): a retrying cluster emits exactly SIX retry stats — five COUNTERS `upstream_rq_retry` (total retries past the first attempt), `upstream_rq_retry_success` (requests that recovered after ≥1 retry), `upstream_rq_retry_limit_exceeded` (requests that exhausted the static `num_retries` cap), `upstream_rq_retry_backoff_exponential` (retries that waited an exponential backoff — one per retry here), `upstream_rq_retry_backoff_ratelimited` (rate-limited-backoff retries; 0 without `rate_limited_retry_back_off`) — plus the GAUGE `circuit_breakers.<priority>.rq_retry_open`. The reference emits these on ALL clusters (always-on, both `default`+`high` for the gauge). envoy-go REGISTERS the **+5 new counters SCOPED to retry-policy-bearing clusters** (a recorded departure — the outlier/circuit-breaker scoping precedent; keeps every existing fixture byte-stable), and the phase-41-already-registered `rq_retry_open` (gauge) + `upstream_rq_retry_overflow` (counter) FLIP LIVE on a `circuit_breakers`+`retry_budget` cluster (no new registration — they exist emit-0 since phase 41). `upstream_rq_total` counts EVERY attempt (original + retries — confirmed `c_bad.upstream_rq_total=4` for 1+3). Surface **1163 → 1173** (+10 = +5 per retry cluster × the `0075` fixture's TWO retry clusters [recover + exhaust]). 1173 is FIRM as long as `0075` keeps exactly two retry-policy clusters (the §8 topology); the PLAN re-confirms (+5 per retry-policy cluster).
- **AMEND-RT3 (retry_budget — the dynamic-concurrency formula + the overflow-vs-limit discriminator, LIVE-confirmed).** Live finding (D-RT-BUDGET): with `retry_budget{budget_percent:0, min_retry_concurrency:1}` (budget = `max(1, 0% × active)` = 1) under 120 concurrent requests against a single-503 cluster, the reference emitted `upstream_rq_retry:5`, `upstream_rq_retry_overflow:119`, `upstream_rq_retry_limit_exceeded:1`, `upstream_rq_total:125`. So the two exhaustion modes are DISTINCT counters: **budget exhaustion** (the active-retry count is at `max(min_retry_concurrency, budget_percent% × activeRequests)`) ⇒ the request does NOT retry, `upstream_rq_retry_overflow`++, and `rq_retry_open` reads 1 while at the cap (transient); **static-cap exhaustion** (a request that DID retry up to `num_retries` and still failed) ⇒ `upstream_rq_retry_limit_exceeded`++. 42.1 ACTIVATES the phase-41 slot: the `circuitBreaker` gains a cluster-level `activeRetries atomic.Int64`; the executor try-acquires a slot before each retry attempt (`activeRetries < max(min_retry_concurrency, ⌈budget_percent% × activeRequests⌉)`), releases on attempt completion; on a failed try-acquire ⇒ no retry + `upstream_rq_retry_overflow`++ + `rq_retry_open`=1. Defaults `budget_percent 20%` / `min_retry_concurrency 3` (proto/doc). `activeRequests` is the phase-41 per-priority counter already tracked.
- **AMEND-RT4 (differential — the recover arm is NOT first-pick-deterministic; the exhaustion arm is the cross-side-exact core).** Live finding (D-RT-DIFFERENTIAL): the BRAINSTORM assumed "deterministic round-robin so the first pick hits the 503 host." FALSE — the reference RANDOMIZES the round-robin initial offset per cluster (the very first `/recover` probe hit the healthy host: `upstream_rq_retry=0`, final 200, no retry). A retry consumes TWO host picks (advancing the RR counter by 2), so once aligned the phase locks, but the INITIAL offset differs cross-side (envoy-go is deterministic-from-0; the reference randomizes). Therefore the EXACT retry count over a 2-host {503, echo} cluster is offset-dependent and NOT cross-side-exact. The REFINED design: the **EXHAUSTION arm** (single `HTTP503Responder` host) is fully deterministic and CROSS-SIDE-EXACT (`upstream_rq_retry==N`, `upstream_rq_retry_limit_exceeded==1`, `upstream_rq_total==N+1`, final 503 — the robust core); the **RECOVER arm** (2-host {503, echo} RR cluster) asserts the OFFSET-INVARIANT downstream outcome (`http.<prefix>.downstream_rq_2xx` delta == K, all clients recover — confirmed `downstream_rq_2xx:21` for 21 recover requests) cross-side, plus subject-side `upstream_rq_retry_success == upstream_rq_retry > 0` + `upstream_rq_retry_limit_exceeded == 0`; the exact `upstream_rq_retry` count is NOT cross-side-asserted (the offset finding). NO new BackendKind — the fixture REUSES `HTTP503Responder` (35) + `HTTPEcho`. Sleepless/count-based (no `time.Sleep`; backoff is delay-only).
- **AMEND-RT5 (reject surface — THIN; one PGV-mirror + one runtime-mirror; NO retry_on reject).** Live finding (D-RT-PROTO/D-RT-RETRYON): `RetryPolicy` has ZERO scalar PGV constraints on `retry_on`/`num_retries`/`retriable_status_codes`; the only PGV arms are `RetryPolicy_RetryBackOff.base_interval` (required + `> 0s`) and `max_interval` (`> 0s` when set). The `max_interval >= base_interval` relationship is a RUNTIME boot-reject, NOT PGV (confirmed `--mode validate`: `retry_policy.max_interval must greater than or equal to the base_interval`). envoy-go MIRRORS (ADR-0080 byte-stable, house wording `route: %q: retry_policy: <reason>`): (a) `retry_back_off.base_interval` absent/≤0 when `retry_back_off` is set; (b) `retry_back_off.max_interval < base_interval`. `retry_on` tokens are NEVER rejected (freeform; unknown ignored). `num_retries` is UNBOUNDED (any value incl. 0 ⇒ no retries — a valid config). NO new fuzzer (config-parse, unit-tested — the 40.x/41 precedent); fuzzers STAY **42** unless the `retry_on` tokenizer warrants one (a candidate config-parse fuzzer — PLAN pin §12).
- **AMEND-RT6 (backoff + defaults — exponential full-jitter, delay-only).** Live finding (D-RT-PROTO + probe): `num_retries` defaults to **1** (confirmed `c_bad2` with no `num_retries` ⇒ `upstream_rq_retry:1`); `retry_back_off.base_interval` defaults ~**25ms**, `max_interval` defaults to **10× base** (~250ms) when unset; sub-1ms base rounds up to 1ms; the algorithm is exponential with FULL JITTER. Every retry waits a backoff (`upstream_rq_retry_backoff_exponential == upstream_rq_retry` in all probes). Backoff is DELAY-ONLY — it changes WHEN an attempt fires, never WHETHER or HOW MANY — so the count-based differential is immune (the `reference_differential_band_sigma_margin` discipline: no timing-margin assertion). The jitter RNG is the standard library; determinism comes from the COUNT assertions.
- **AMEND-RT7 (response-flags — RX/URX a recorded departure).** envoy-go has NO response-flags plumbing (`RESPONSE_FLAGS` hardcoded to `-` — the phase-41 CB4 precedent). The reference sets `RX` (retried) / `URX` (retry-limit-exceeded) on the access-log line; 42.1 records this as a DEPARTURE. The differential asserts the retry STATS + the final status code, NEVER the access-log line (fixtures compare `/stats` + response, not log output).

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0249 (the retry-loop architecture — the `retryExecutor` wrapping the single-attempt driver + the `retry_on` bitset classifier + the buffered-body replay + the exponential backoff + the per-attempt CB-admission/outcome re-run + the `retry_budget` activation on the phase-41 `circuitBreaker` + the `upstream_rq_retry*` stat block + the enforced-subset / parse-accept-rest `retry_on` posture) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 42.1 IMPL per ADR-0044. The clean budget activation FOLDS the BRAINSTORM's anticipated second ADR (ADR-0250, the retry_budget dynamic-concurrency model) INTO ADR-0249 (§7 — a single ADR suffices, the phase-41 precedent). DECISIONS tail STAYS ADR-0248 at this SPEC; next-free ADR-0249. The §10 BRAINSTORM D-RT pins are RESOLVED in §11; the PLAN/IMPL D-questions are §12.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **Hedging (`HedgePolicy` + `per_try_timeout`)** — the 42.2 leg (`HedgePolicy.initial_requests` [PGV ≥1], `additional_request_chance`, `hedge_on_per_try_timeout`; `RetryPolicy.per_try_timeout` field 3 / `per_try_idle_timeout` field 13). NOT 42.1.
- **`request_mirror_policies` (RouteAction field 30)** — shadow traffic; deferred to a future row.
- **The deferred `retry_on` tokens** (`retriable-headers`, `envoy-ratelimited`, the `grpc-*` tokens) — parse-accept-but-defer (AMEND-RT1).
- **`retriable_headers` + `retriable_request_headers` (RetryPolicy fields 9/10)** — header-match retry gating; parse-accept-but-defer.
- **`retry_priority` / `retry_host_predicate` / `retry_options_predicates` / `host_selection_retry_max_attempts` / `rate_limited_retry_back_off`** — the retry host-selection plugins + the rate-limited backoff; parse-accept-but-defer (`upstream_rq_retry_backoff_ratelimited` registers emit-0).
- **The `RX`/`URX` retry response flags** — blocked on the absent response-flags surface (AMEND-RT7); a recorded departure.
- **Streamed (over-1-MiB-cap) body retries** — 42.1 retries only buffered bodies (the `FilterBufferLimitBytes` cap, ADR-0076, is the boundary); an over-cap streamed body is NON-RETRIABLE.
- **Retries on non-HTTP (TCP/network) upstreams.**

---

## 3. The `retryExecutor` wrapper + the `retry_on` classifier + the `retry_budget` activation (ADR-0249)

### 3.0 Split disposition — the FIRST leg (42.1) of the pre-authorized 42.1/42.2 split

42.1 = the retry loop + the classifier + the parse/reject + the budget activation + the +5 stat block + `0075`. The ADR-0045 split-gate is re-checked at the PLAN (anticipated ~300–450 prod LoC / ~12–16 tasks — under `> ~25 tasks OR > ~1500 LoC`; the 42.1 leg stands alone). 42.2 (hedging) is the pre-authorized second leg; row 42 flips `done` only when BOTH land (ADR-0106 + `reference_roadmap_split_phase_row_done`).

### 3.1 The retry executor (wrapping the single-attempt driver)

A `retryExecutor` in `internal/filter/http/router` wraps the EXISTING single-attempt drivers at their call sites — `doH1ClusterAction` (`router.go:588`, called from the H1 dispatch `router.go:566` + the weighted dispatch `router_weighted.go:110`) and `doH2ClusterAction` (`router_h2.go:57`, from `router_h2.go:42` + `router_weighted.go:123`). Each loop iteration is ONE driver call (host pick via LB + CB admission via `TryAcquireRequest` + dial + the buffered `ActionResponse` + `RecordUpstreamResult`). The loop (schematically, per request, when `a.retryPolicy != nil`):
```
attempt := 0
for {
    resp, ep, err := do{H1,H2}ClusterAction(ctx, a, req)   // one attempt; CB-admit + outcome already run inside
    if attempt >= numRetries { return resp }               // static cap → caller; ++retry_limit_exceeded if retriable
    if !retryOn.matches(resp.Status, err) { return resp }  // non-retriable outcome → caller
    if !cb.tryAcquireRetry() { return resp }               // budget exhausted → ++retry_overflow, rq_retry_open=1, no retry
    sleep(backoff.next(attempt))                           // exponential full-jitter; delay-only
    req.Body = io.NopCloser(bytes.NewReader(bodyBuf))      // replay the buffered body (ADR-0076 cap)
    attempt++; ++retry; ++retry_backoff_exponential        // the retry is about to fire
    // cb.releaseRetry() deferred per attempt
}
// on a later success after ≥1 retry → ++retry_success
```
When `a.retryPolicy == nil` the executor is a DIRECT pass-through (a single driver call — byte-identical to today). NOTE: each attempt re-runs `TryAcquireRequest` (so circuit breaking caps total in-flight attempts) + `RecordUpstreamResult` (so outlier detection counts every attempt) — these seams ALREADY live inside the driver; the executor adds NOTHING to them (the load-bearing reuse).

### 3.2 The `retry_on` classifier + the buffered-body replay

`retryOn` is a parsed bitset over the enforced token subset (AMEND-RT1). `matches(status, err)`: `connect-failure`/`reset` ⇒ `err`/local-origin-failure shape; `5xx` ⇒ `status ∈ [500,599]`; `gateway-error` ⇒ `status ∈ {502,503,504}`; `retriable-status-codes` ⇒ `status ∈ retriable_status_codes[]`. The body is replayed from `bodyBuf` (buffered at `connection.go:548-600` under the 1 MiB `FilterBufferLimitBytes` cap, ADR-0076) as a fresh `bytes.NewReader` per attempt; an over-cap (streamed) body is non-retriable (no buffered bytes — AMEND-RT1 boundary; PLAN pins the exact guard).

### 3.3 The `retry_budget` activation on the phase-41 circuitBreaker

The phase-41 `circuitBreaker` (`internal/cluster/circuitbreaker.go`) — which already PARSES `retry_budget` (the `budget_percent ∈ [0,100]` reject at `:52-53`) and REGISTERS the emit-0 `rq_retry_open` (`:78`) + `upstream_rq_retry_overflow` (`:83`) — gains a cluster-level `activeRetries atomic.Int64` + `tryAcquireRetry()`/`releaseRetry()`. `tryAcquireRetry`: atomically, if `activeRetries >= max(minRetryConcurrency, ⌈budgetPercent% × activeRequests⌉)` ⇒ set `rqRetryOpen=1`, `upstreamRqRetryOverflow`++, return `false`; else `activeRetries++`, return `true`. `releaseRetry`: `activeRetries--`; clear `rqRetryOpen=0` when below the cap. `Cluster.TryAcquireRetry()`/`ReleaseRetry()` no-op when `circuitBreaker==nil` or no `retry_budget` (AMEND-RT3).

### 3.4 Byte-stability

When `a.retryPolicy == nil`, the executor is a single pass-through driver call ⇒ the upstream path is **byte-identical to today** (every existing fixture stays green; the full 76-dir byte-stability gate must hold). The +5 retry counters register ONLY on retry-policy clusters (AMEND-RT2 scoping).

---

## 4. Framework primitives — the executor wrapper + the budget activation over the phase-40/41 substrate + 0 new packages + 0 new go.mod deps

- NEW: the `retryExecutor` + `retry_on` bitset + the backoff timer in `internal/filter/http/router` (new file, e.g. `retry.go`); a `retryPolicy` struct on `clusterRouteAction` (`internal/filter/hcm/actions.go:201`), parsed in `buildRouterAction` (`internal/filter/hcm/config.go:536`), with the `VirtualHost.retry_policy` fallback threaded through `buildRouteTable` (`config.go:406`, the `includeVhRateLimits` precedent); the `activeRetries`/`tryAcquireRetry`/`releaseRetry` on the phase-41 `circuitBreaker` (`internal/cluster/circuitbreaker.go`); the +5 retry counter registrations (scoped); the `upstream_rq_retry*` increment sites in the executor.
- REUSED: the single-attempt drivers `doH1ClusterAction`/`doH2ClusterAction` + their call sites (router.go/router_h2.go/router_weighted.go); the buffered `ActionResponse` (`router.go:119`); the per-attempt `TryAcquireRequest` (ADR-0248) + `RecordUpstreamResult` (ADR-0245) seams; the HCM buffered body (`connection.go:548-600`, ADR-0076); the phase-41 `circuitBreaker` + the dormant `retry_budget` slot (ADR-0248); the `internal/admin` `/stats` endpoint; the `reference_docker_probe_bridge_network` differential pattern.
- ZERO new Go packages. ZERO new go.mod modules (`route.v3.RetryPolicy` is in the existing go-control-plane v1.32.4 dep; `go mod tidy -diff` EMPTY — §11 D-RT-PROTO, confirmed; route.v3 + cluster.v3 already imported).

---

## 5. Proto-field roster (per §11 D-RT-PROTO)

`RouteAction.retry_policy` = `RouteAction` field 9 → `route.v3.RetryPolicy` (`config/route/v3/route_components.pb.go`); `VirtualHost.retry_policy` = field 16 (the route-absent fallback). Carrier fields (42.2 / deferred): `RouteAction.hedge_policy` field 27, `request_mirror_policies` field 30; `VirtualHost.hedge_policy` field 17.

`RetryPolicy` (the `#` column is the PROTO FIELD TAG):

| # | Field | Type | 42.1 role |
|---|-------|------|-----------|
| 1 | `retry_on` | string (freeform, **NO PGV**) | the enforced-subset classifier (AMEND-RT1) |
| 2 | `num_retries` | UInt32Value (default **1**) | the static per-request cap |
| 7 | `retriable_status_codes` | []uint32 | the explicit retriable-codes list (`retriable-status-codes` token) |
| 8 | `retry_back_off` | `RetryPolicy_RetryBackOff` | the exponential backoff (base/max) |
| 3 | `per_try_timeout` | Duration | **42.2** (DEFER) |
| 13 | `per_try_idle_timeout` | Duration | **42.2** (DEFER) |
| 4/5/12/6 | `retry_priority` / `retry_host_predicate` / `retry_options_predicates` / `host_selection_retry_max_attempts` | plugins | PARSE-ACCEPT-DEFER |
| 9/10 | `retriable_headers` / `retriable_request_headers` | []HeaderMatcher | PARSE-ACCEPT-DEFER |
| 11 | `rate_limited_retry_back_off` | message | PARSE-ACCEPT-DEFER (`*_backoff_ratelimited` emit-0) |

`RetryPolicy_RetryBackOff`: `base_interval` (#1, Duration, **PGV required + >0s**, default ~25ms), `max_interval` (#2, Duration, **PGV >0s** when set, default 10×base; `max >= base` is a RUNTIME reject — AMEND-RT5). `RetryBudget` (on `cluster.v3.CircuitBreakers_Thresholds.retry_budget`, field 8): `budget_percent` (#1, Percent, default 20%), `min_retry_concurrency` (#2, UInt32Value, default 3). `go mod tidy -diff` EMPTY → ZERO new module.

---

## 6. PARSE-REJECT roster (per §11 D-RT-RETRYON + ADR-0080)

envoy-go hand-rolls its own byte-stable rejects (the `parseCircuitBreakers` precedent), mirroring the thin reference envelope. House wording `route: %q: retry_policy: <reason>`:
- `retry_back_off.base_interval` absent or ≤ 0 when `retry_back_off` is set (reference PGV: required + `value must be greater than 0s`).
- `retry_back_off.max_interval < base_interval` (reference RUNTIME boot-reject: `retry_policy.max_interval must greater than or equal to the base_interval` — confirmed §11).

NOT rejected: `retry_on` tokens (freeform; unknown SILENTLY IGNORED — NO PGV); `num_retries` (unbounded; 0 ⇒ no retries, a valid config); `retriable_status_codes[]` values (unbounded); the deferred plugin/header fields (parse-accept-ignore). All reject arms unit-level (no boot-reject dir — the 41 precedent). Exact house wording pinned at the PLAN (§12, D-S42-1).

---

## 7. Stat surface — +5 new retry counters (scoped) + 2 phase-41 stats flip LIVE (per §11 D-RT-STATS + AMEND-RT2)

The +5 NEW cluster counters `cluster.<n>.` — registered ONLY on retry-policy-bearing clusters (a recorded departure from the reference's always-on; the outlier/CB scoping precedent — existing fixtures unaffected):
1. `upstream_rq_retry` — counter — **LIVE** (++ per retry attempt past the first).
2. `upstream_rq_retry_success` — counter — **LIVE** (++ when a request recovers after ≥1 retry).
3. `upstream_rq_retry_limit_exceeded` — counter — **LIVE** (++ when a request exhausts the static `num_retries` cap).
4. `upstream_rq_retry_backoff_exponential` — counter — **LIVE** (++ per retry that waited an exponential backoff).
5. `upstream_rq_retry_backoff_ratelimited` — counter — emit-0 at 42.1 (`rate_limited_retry_back_off` deferred — a recorded departure).

The 2 phase-41-already-registered stats FLIP LIVE (no new registration — they exist emit-0 on `circuit_breakers` clusters since phase 41) when `retry_budget` is configured + retries occur:
6. `cluster.<n>.circuit_breakers.<default|high>.rq_retry_open` — gauge — LIVE (1 while `activeRetries` at the budget cap; `high` stays 0 — nothing routes by priority).
7. `cluster.<n>.upstream_rq_retry_overflow` — counter — LIVE (++ on a `retry_budget` exhaustion; AMEND-RT3).

`upstream_rq_total` counts EVERY attempt (already LIVE — no new stat). Surface **1163 → 1173** (+10 = +5 per retry cluster × the `0075` fixture's two retry clusters; FIRM given the §8 two-retry-cluster topology — +5 per retry-policy cluster, the PLAN re-confirms).

---

## 8. Differential fixture taxonomy (+1: `0075` cross-side retry loop — exhaustion-exact + recover-invariant)

### 8.1 `0075-retry-loop` (cross-side; NO new BackendKind)

An HTTP listener on BOTH the subject and the reference (`contrib-v1.37.2`), with TWO retry clusters (REUSING `HTTP503Responder` 35 + `HTTPEcho`):

**A. EXHAUSTION arm (cross-side EXACT — the deterministic core).** Cluster `c_exhaust` = {1 `HTTP503Responder`}, route `/exhaust` with `retry_policy{retry_on:"5xx", num_retries:N}`. Drive 1 request ⇒ final **503**; assert on BOTH sides: `cluster.c_exhaust.upstream_rq_retry == N`, `upstream_rq_retry_limit_exceeded == 1`, `upstream_rq_retry_success == 0`, `upstream_rq_total == N+1`. Fully deterministic (single 503 host; offset-irrelevant). Decode-ran guard: `ref[upstream_rq_total] > 0`.

**B. RECOVER arm (offset-invariant cross-side + subject-side consistency).** Cluster `c_recover` = {`HTTP503Responder` host0, `HTTPEcho` host1} ROUND_ROBIN, route `/recover` with `retry_policy{retry_on:"5xx", num_retries:1}`. Drive K requests ⇒ ALL final **200**; assert on BOTH sides: `http.<prefix>.downstream_rq_2xx` delta == K (every client recovered — OFFSET-INVARIANT, the live finding AMEND-RT4) + `cluster.c_recover.upstream_rq_retry_limit_exceeded == 0`; assert SUBJECT-side: `upstream_rq_retry_success == upstream_rq_retry > 0` (retries happened and all recovered). The EXACT `upstream_rq_retry` count is NOT cross-side-asserted (the reference randomizes the RR initial offset — AMEND-RT4).

SLEEPLESS (backoff is delay-only; no `time.Sleep`, no timing-margin assertion — `reference_differential_band_sigma_margin`). The `-count=1` + `TestDifferential/0075` selector discipline (`reference_differential_break_protocol_count1` + `reference_differential_run_selector`). 2 deliberate breaks: (A) the executor never retries (`retryOn.matches` always false) ⇒ the exhaustion arm gets `upstream_rq_retry==0` + the recover arm returns 503s ⇒ FAIL; (B) the retry counters not wired ⇒ the cross-side `upstream_rq_retry`/`downstream_rq_2xx` deltas FAIL. Constants (N / K / cluster topology) single-sourced (`reference_fixture_workload_constant_desync`). Fixtures **76 → 77**.

### 8.2 The `retry_budget` activation — UNIT-tested (not a differential arm)

The budget overflow is concurrency/timing-shaped (the live probe needed 120 concurrent for 119 overflows — not a deterministic count cross-side). 42.1 UNIT-tests the activation: the `activeRetries` try-acquire/release + the `max(min_retry_concurrency, budget_percent% × active)` formula + the `upstream_rq_retry_overflow`++/`rq_retry_open`=1 on a forced over-budget retry + the overflow-vs-limit-exceeded discriminator (AMEND-RT3). A differential budget arm (via the phase-41 `BlockingHoldResponder` release-barrier) is DEFERRED/optional (PLAN pin §12) — the deterministic core is the `0075` retry loop.

### 8.3 New fuzzer: NONE-to-ONE

`retry_policy` is config-parse (no new wire decoder); the parse/reject + the `retry_on` tokenizer are unit-tested (the 40.x/41 precedent). A candidate +1 config-parse/tokenize fuzzer is a PLAN pin (§12, D-S42-2); default fuzzers STAY **42** (AMEND-RT5).

---

## 9. Behavior-contract delta (the 42.1 bundle; ADR-0052 atomic landing)

A new `### Route — request recovery (retries)` subsection in BEHAVIOR_CONTRACT.md: the `retry_policy` re-attempt loop (classify the buffered `ActionResponse.Status` against the enforced `retry_on` subset; re-pick via the LB; replay the buffered body; exponential full-jitter backoff; the static `num_retries` cap); the per-attempt CB-admission + outcome re-run (every attempt counts against `max_requests` + outlier detection); the `retry_budget` activation (the dynamic-concurrency cap; overflow ⇒ `upstream_rq_retry_overflow` + `rq_retry_open`, distinct from `upstream_rq_retry_limit_exceeded`); the +5 retry counters (scoped) + the 2 phase-41 stats flipping LIVE; the enforced-subset / parse-accept-rest `retry_on` posture; the `RX`/`URX` response-flags departure (AMEND-RT7); the over-cap-body non-retriable boundary (ADR-0076); byte-identical when no `retry_policy`. The stat-surface block advances 1163 → 1173.

---

## 10. Per-task structure (~12–16 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) the `retry_policy` parse in `buildRouterAction` + the `VirtualHost` fallback + the `retryPolicy` struct on `clusterRouteAction` + the reject arms + unit tests; (3) the `retry_on` bitset tokenizer + the `matches(status,err)` classifier + unit tests (the enforced subset; connect-failure-vs-502-response; unknown-token-ignore); (4) the exponential full-jitter backoff + unit tests; (5) the `retryExecutor` loop wrapping `do{H1,H2}ClusterAction` at all call sites (H1 + H2 + weighted) + the buffered-body replay + unit tests; (6) the +5 retry counter registrations (scoped to retry-policy clusters; 1163→1173) + the increment sites; (7) the `retry_budget` activation on the `circuitBreaker` (`activeRetries`/`tryAcquireRetry`/`releaseRetry`) + the `rq_retry_open`/`upstream_rq_retry_overflow` flip-LIVE + unit tests (formula, overflow-vs-limit discriminator); (8) the `0075` fixture (exhaustion + recover arms); (9) `0075` deliberate-break + 20-run flake; (10) full 77-dir differential + six-gate; (11) ADR-0249 body + BEHAVIOR_CONTRACT; (12) completion bundle + ROADMAP row 42 SPEC→note (row stays `in-progress` until 42.2). The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO further split of 42.1).

---

## 11. SPEC-time empirical-pin block (D-RT-* — executed IN-SESSION 2026-06-19)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge network `retryprobe`; STRICT_DNS `direct_response` backends [bad-503 / good-200 / gw-502 / s-500]; a gateway Envoy with 9 retry routes; single-shot + concurrent load; `--mode validate` reject probes; `--concurrency 1`; request path verified `upstream_rq_total>0`) + the go-control-plane v1.32.4 module cache.

| Pin | Disposition |
|-----|-------------|
| **D-RT-PROTO** | CONFIRMED. `RouteAction.retry_policy=9` / `hedge_policy=27` / `request_mirror_policies=30`; `VirtualHost.retry_policy=16` / `hedge_policy=17`; `RetryPolicy{retry_on=1 (string,NO-PGV), num_retries=2 (default 1), per_try_timeout=3, retriable_status_codes=7, retry_back_off=8, retriable_headers=9, retriable_request_headers=10, rate_limited_retry_back_off=11, retry_options_predicates=12, per_try_idle_timeout=13}`; `RetryBackOff{base_interval=1 (req,>0s), max_interval=2 (>0s; default 10×base; max>=base is RUNTIME not PGV)}`; `RetryBudget{budget_percent=1 (default 20%), min_retry_concurrency=2 (default 3)}` on `Thresholds.retry_budget=8`; `HedgePolicy{initial_requests=1 (PGV>=1), additional_request_chance=2, hedge_on_per_try_timeout=3}`. `go mod tidy -diff` EMPTY → ZERO new module; route.v3+cluster.v3 already imported. |
| **D-RT-RETRYON** | PINNED. `gateway-error`→502 retried; `5xx`→500 retried; `retriable-status-codes`+`[500]`→500 retried; `connect-failure`→true conn-refusal retried (final 503) but NOT a 502 RESPONSE (`c_gw502b.upstream_rq_retry=0`). Enforce `{5xx, gateway-error, connect-failure, reset, retriable-status-codes(+list)}`; parse-accept-defer `{retriable-headers, envoy-ratelimited, grpc-*}`; unknown tokens IGNORED (no PGV). §AMEND-RT1. |
| **D-RT-STATS** | CONFIRMED. 5 counters `upstream_rq_retry`/`_retry_success`/`_retry_limit_exceeded`/`_retry_backoff_exponential`/`_retry_backoff_ratelimited` + the gauge `circuit_breakers.<default|high>.rq_retry_open` + the counter `upstream_rq_retry_overflow`. `upstream_rq_total` counts all attempts (1+3⇒4). Reference always-on; envoy-go scopes the +5 to retry clusters; the 2 phase-41 stats flip LIVE. §7/AMEND-RT2. |
| **D-RT-BUDGET** | CONFIRMED. `budget=max(min_retry_concurrency, budget_percent% × active)`; 120-concurrent / budget-1 ⇒ `retry:5`, `retry_overflow:119`, `retry_limit_exceeded:1`, `total:125`. overflow (budget) ⇒ `upstream_rq_retry_overflow` + `rq_retry_open=1` (transient, no retry); limit_exceeded (static cap) ⇒ a request that DID retry. defaults 20%/3. §AMEND-RT3. |
| **D-RT-DIFFERENTIAL** | PINNED (REFINED). The reference RANDOMIZES the RR initial offset ⇒ "first pick hits 503" is FALSE; the exhaustion arm (single 503 host) is cross-side-EXACT; the recover arm asserts `downstream_rq_2xx==K` (offset-invariant) + subject-side `retry_success==retry>0`. ONE fixture `0075` (two arms, two clusters); NO new BackendKind (reuse `HTTP503Responder`); sleepless. Budget overflow UNIT-tested (concurrency-shaped). §8/AMEND-RT4. |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S42-1** the exact envoy-go house reject wording for §6 (`route: %q: retry_policy: …` — base_interval-required / max<base).
- **D-S42-2** whether a `retry_policy`/`retry_on`-tokenize config-parse fuzzer is warranted (default: NO — fuzzers stay 42).
- **D-S42-3** the over-cap (streamed, >1 MiB) body non-retriable guard — the exact detection point (a buffered-flag on the request) vs a recorded departure.
- **D-S42-4** the `VirtualHost.retry_policy` (field 16) fallback threading through `buildRouteTable` (the `includeVhRateLimits` precedent) — route-level overrides vhost; absent-on-both ⇒ no retry.
- **D-S42-5** the retry-counter scoping predicate (a cluster registers the +5 when any route targeting it carries `retry_policy` [route or vhost]) — the route→cluster scan at build; the exact `0075` cluster count fixes the surface integer (+5 per retry cluster).
- **D-S42-6** `0075` constants (N / K / cluster topology / `refContainerListenerPort`) single-sourced; whether the recover arm reads the HCM downstream `http.<prefix>.downstream_rq_2xx` or the cluster `upstream_rq_2xx` (the downstream one is offset-invariant — §8B).
- **D-S42-7** the `retry_budget` activation shape on `circuitBreaker` (a cluster-level `activeRetries` vs per-priority) + whether the overflow path needs a per-attempt-vs-per-request increment; the ADR-0045 final split-gate re-check (anticipated NO further split).
- **D-S42-8** whether a deterministic differential budget-overflow arm (via `BlockingHoldResponder` release-barrier) is added at 42.1 or deferred (default: UNIT-tested at 42.1).

---

## 13. ADR continuity — the ADR-0249 §Context DRAFT (anchored here; full entry lands at the 42.1 IMPL)

**ADR-0249 §Context (draft).** Phases 39–41 established the cluster-runtime substrate for upstream robustness: active health checking (ADR-0242/0243) + passive outlier detection (ADR-0245/0246/0247) route AROUND unhealthy hosts; circuit breaking (ADR-0248) SHEDS load via a per-priority `max_requests` concurrency cap and registered (emit-0) the `retry_budget` slot. Retries (`RouteAction.retry_policy`) are the fourth dimension — they RECOVER a single request by re-attempting it against an LB-re-picked host, the project's first request-replay control plane. The phase-42 BRAINSTORM settled a `retryExecutor` wrapping the existing single-attempt driver; the §11 live pins then REFINED the design: the enforced `retry_on` subset distinguishes transport failures (`connect-failure`/`reset`) from 5xx responses (a `connect-failure` token does NOT retry a 502 response); the retry stat roster is exactly five new counters (scoped to retry clusters) plus the two phase-41 stats flipping LIVE; the `retry_budget` is a dynamic `max(min_retry_concurrency, budget_percent% × active)` cap whose overflow (`upstream_rq_retry_overflow`) is distinct from static-cap exhaustion (`upstream_rq_retry_limit_exceeded`); and the differential's recover arm cannot assume a deterministic first-pick (the reference randomizes the round-robin offset) so the deterministic cross-side core is the exhaustion arm with the recover arm asserting the offset-invariant downstream-200 count. The design: a `retryExecutor` re-invoking `doH1ClusterAction`/`doH2ClusterAction` (which already re-run CB admission + the outcome seam per attempt); a `retry_on` bitset classifier over the buffered `ActionResponse.Status`; a buffered-body replay (`bytes.Reader`, the ADR-0076 cap); an exponential full-jitter backoff (delay-only); the `retry_budget` activation on the phase-41 `circuitBreaker`; the +5 retry stat block + the 2 phase-41 stats flipping LIVE; byte-identical when no `retry_policy`. The single ADR-0249 absorbs the retry_budget dynamic-concurrency model (the BRAINSTORM's anticipated ADR-0250 is unnecessary at the clean activation scope). §Decision + §Consequences land at the 42.1 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1163** / fixtures **76** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0248** (next-free **ADR-0249**). ROADMAP row 42 STAYS `in-progress` (the SPEC note appended). Anticipated at the 42.1 IMPL: fixtures 76 → 77 (`0075`), BackendKind tail 36 UNCHANGED (REUSE `HTTP503Responder`), DECISIONS tail ADR-0248 → ADR-0249 (next-free ADR-0250), stat surface 1163 → 1173 (+5 per retry cluster × the `0075` two retry clusters — firmed at PLAN), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. The 2 phase-41 stats (`rq_retry_open`, `upstream_rq_retry_overflow`) flip LIVE (no surface delta — already registered). Next → the phase-42.1 PLAN (`superpowers:writing-plans`).
