# Phase 42 Brainstorm — retries + hedging (the FOURTH Upstream-robustness-family row; a re-attempt loop WRAPPING the existing single-attempt upstream call — the project's FIRST request-replay control plane)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 42 (`retries + hedging`), the **FOURTH Upstream-robustness-family row** (the family opened at phase 39 with active health checks, continued at phase 40 with passive outlier detection, and at phase 41 with circuit-breaker load-shedding). Phase 42 lands `RouteAction.retry_policy` (`route.v3.RetryPolicy`, `RouteAction` field 9; `VirtualHost.retry_policy` field 16 the route-absent fallback) plus `RouteAction.hedge_policy` (`route.v3.HedgePolicy`, field 27) — **re-attempting a failed upstream request** against a re-picked host (and, for hedging, racing a second attempt before the first completes). Where phase-39 active health checking *probes* hosts, phase-40 outlier detection *ejects* failing hosts, and phase-41 circuit breaking *caps concurrency* at a saturated cluster, retries *recover a single request* that hit a transiently-bad host — the fourth independent upstream-robustness dimension, and the first that **re-runs** the upstream call rather than routing around or shedding.

Phase 42 is the natural follow-on to the phase-41 circuit-breaker substrate: it ACTIVATES the `retry_budget` slot that phase 41 parsed-and-registered-but-deferred (`circuitbreaker.go:78,83` — the `rq_retry_open` gauge + `upstream_rq_retry_overflow` counter, both registered emit-0 at 41, flip LIVE at 42), and it REUSES the per-request upstream lifecycle that phases 40/41 built — the single-attempt driver already re-runs circuit-breaker admission (`TryAcquireRequest`) and the per-request outcome callback (`RecordUpstreamResult`, ADR-0245) on every call, so wrapping that driver in a re-attempt loop gives BOTH circuit breaking and outlier detection a correct view of EVERY attempt for free.

The load-bearing facts that shape this brainstorm:

- **`RouteAction.retry_policy` is a single `route.v3.RetryPolicy` message (`RouteAction` field 9; `VirtualHost.retry_policy` field 16 is the route-absent fallback).** VERIFIED present in the EXISTING core `/envoy` go-control-plane dep (`github.com/envoyproxy/go-control-plane/envoy v1.32.4`, `config/route/v3/route_components.pb.go`) — ZERO new go.mod MODULE. Its 42.1 MVP shape: `retry_on` (field 1, a freeform space/comma-delimited token string — **NO PGV**), `num_retries` (field 2, `*wrapperspb.UInt32Value`, default 1), `retry_back_off` (field 8, a `RetryPolicy_RetryBackOff{base_interval=1, max_interval=2}`), `retriable_status_codes` (field 7, `[]uint32`). The hedging surface — `HedgePolicy{initial_requests=1, additional_request_chance=2, hedge_on_per_try_timeout=3}` (`RouteAction` field 27; `VirtualHost.hedge_policy` field 17) and `RetryPolicy.per_try_timeout` (field 3) — is the **42.2** scope. The exact field roster + types + PGV constraints + reference defaults are SPEC-time obligations (§10, D-RT-PROTO). NB: the reference *Envoy binary* used for live probes is `contrib-v1.37.2` (ADR-0227); the *proto dep* is `go-control-plane v1.32.4` — two distinct version axes (the phase-41 lesson).
- **A retry is a re-run of the EXISTING single-attempt driver — NOT a new upstream path.** The single-attempt H1 driver `doH1ClusterAction` (`router.go:588`) and H2 driver `doH2ClusterAction` (`router_h2.go:57`) ALREADY buffer the full upstream response into `ActionResponse{Status, Headers, Body, Close}` (`router.go:119`) before returning, and ALREADY perform host-pick-via-LB + circuit-breaker admission (`TryAcquireRequest`, `router.go:596` / `router_h2.go:63`) + outcome recording (`RecordUpstreamResult`) inside one call. So a retry is exactly *call the driver again*: the settled architecture (Fork: ADR-0249) wraps the driver in a `retryExecutor` that classifies the returned `ActionResponse.Status` against a parsed `retry_on` bitset and, on a retriable outcome with attempts + budget remaining, re-invokes the driver (re-picking a host via the LB). No new dial path, no new buffering primitive — the loop sits ABOVE the existing per-attempt machinery.
- **The request body is already fully buffered — replay is a fresh `bytes.Reader`.** The HCM decode path buffers the full request body into `bodyBuf` (`internal/filter/hcm/connection.go:548-600`) under the 1 MiB `FilterBufferLimitBytes` cap (ADR-0076) and hands each attempt a `bytes.NewReader(bodyBuf)`. Replaying a retry is re-creating that reader — no re-read from the socket. The over-cap case (a body that exceeded the buffer cap and streamed) is therefore **non-retriable** (no buffered bytes to replay) — a SPEC pin (D-RT-DIFFERENTIAL/D-RT-PROTO boundary). This is the seam that makes retries cheap: the body is already in memory.
- **`retry_budget` activation flips the phase-41 emit-0 stats LIVE — no new accounting struct.** Phase 41 already PARSES `RetryBudget{budget_percent, min_retry_concurrency}` (the `budget_percent ∈ [0,100]` reject arm lives at `circuitbreaker.go:52-53`) and REGISTERS `circuit_breakers.<priority>.rq_retry_open` (`:78`) + `cluster.<n>.upstream_rq_retry_overflow` (`:83`) emit-0. Phase 42.1 adds a cluster-level atomic active-retry counter to the EXISTING `circuitBreaker` struct; a retry try-acquires a budget slot — `max(min_retry_concurrency, ⌈budget_percent% × activeRequests⌉)` — before each re-attempt; on exhaustion it does NOT retry, increments `upstream_rq_retry_overflow`, and the `rq_retry_open` gauge reads 1. Those two phase-41 stats flip from emit-0 to LIVE (no surface-count delta). Static `num_retries` caps INDEPENDENTLY: exhausting the static cap (no budget exhaustion) ends the loop with `upstream_rq_retry_limit_exceeded`++.
- **Determinism is again the novel differential risk — now RE-PICK-driven, not concurrency- or probe-driven.** Active HC ejects after failed probes; outlier detection ejects after failed real requests; circuit breaking trips on N concurrent in-flight; retries re-pick a host after a *single* failed attempt — the trigger is a deterministic per-host 503 under a deterministic LB pick order. The `0075` differential REUSES the existing per-host-503 topology (`HTTP503Responder`, BackendKind 35, `PerHostBackendKind`) under round-robin so the first pick deterministically hits the 503 host ⇒ the retry re-picks a healthy host ⇒ final 200 — a COUNT-based assertion (`upstream_rq_retry` delta, final status), NOT a timing one. **Backoff is delay-only and never perturbs a count-based differential** (the `reference_differential_band_sigma_margin` lesson inverted: there is no timing-margin assertion to flake). NO new BackendKind.

The next sessions author the 42.1 SPEC then the PLAN then the IMPL (42.2 follows as the pre-authorized second leg). The 42.1 SPEC executes the §10 empirical-pin obligations (D-RT-*) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0249 §Context draft.

**Brainstorm session:** worktree `phase-42-brainstorm` off master (per `feedback_git_worktrees`). Substantive predecessor on master: the phase-41 IMPL squash `7f3c7c15` (the `max_requests`-keystone circuit breaker; ADR-0248), with the docs-only cold-start router tip `9d0a3ae0` as the literal live tip. Counts at master tip: stat surface **1163**, differential fixtures **76** (tail `0074-circuit-breaker-max-requests`), fuzzers **42**, BackendKind tail **36** (`BlockingHoldResponder`), DECISIONS tail **ADR-0248** (next-free **ADR-0249**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the subject + settled the load-bearing forks via dialogue:

- **Q-subject** — the next subject is **retries + hedging** (chosen over the remaining Upstream-robustness candidate {per-protocol connection pooling} and the 3 health-gated Load-balancing candidates {locality-weighted LB, priority LB, panic-threshold refinements}). Rationale: retries compose directly on the phase-40/41 cluster-runtime substrate — they re-run the existing single-attempt driver (which already re-runs CB admission + `RecordUpstreamResult`), they ACTIVATE the `retry_budget` slot phase 41 left dormant, and the body is already buffered for replay; it is the recovery counterpart to phase 41's load-shedding.
- **Q-split (Fork 1)** — a **pre-authorized 42.1/42.2 by-feature split** (the phase-36/38/39 valve), a flat family row `42 | retries-and-hedging | 41 | in-progress`. **42.1 = the retry loop** (`retry_policy`: `retry_on` + `num_retries` + `retry_back_off` + `retry_budget` activation). **42.2 = hedging** (`HedgePolicy` + the project's FIRST `per_try_timeout`, reusing the 42.1 attempt substrate concurrently). `request_mirror_policies` (`RouteAction` field 30, shadow traffic) DEFERRED to a future row. §1.4.
- **Q-architecture (Fork → ADR-0249)** — a `retryExecutor` WRAPPING the existing single-attempt `doH1ClusterAction`/`doH2ClusterAction`; classify the buffered `ActionResponse.Status` against a parsed `retry_on` bitset; on a retriable outcome with attempts + budget remaining, sleep the exponential backoff, re-pick via the LB, replay the buffered body (`bytes.NewReader`), re-invoke the driver; byte-identical when no `retry_policy` (nil-guard). §2.2.
- **Q-retry-budget (Fork: ACTIVATE)** — activate the phase-41 `retry_budget` slot: the `circuitBreaker` gains an atomic active-retry counter; a retry try-acquires a slot before each attempt; on exhaustion ⇒ no retry + `upstream_rq_retry_overflow`++ + `rq_retry_open`=1 (the phase-41 emit-0 stats flip LIVE). §2.3.
- **Q-backoff (Fork: INCLUDE)** — exponential backoff (`retry_back_off` base/max interval, reference default ~25ms/250ms, full jitter); delay-only ⇒ never perturbs the count-based differential. §2.4.
- **Q-differential (Fork: REUSE per-host 503 topology — NO new BackendKind)** — `0075-retry-…` reuses `HTTP503Responder` (BackendKind 35, `PerHostBackendKind`) under deterministic round-robin: the first pick hits the 503 host ⇒ retry re-picks a healthy host ⇒ final 200; a second EXHAUSTION arm (single 503 host, `num_retries=N` ⇒ final 503 + `upstream_rq_retry==N` + `upstream_rq_retry_limit_exceeded==1`). Sleepless/count-based. §6.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0248 — especially ADR-0245 [the per-request `RecordUpstreamResult` seam — re-run on EVERY retry attempt so outlier detection sees each one], ADR-0248 [the phase-41 `circuitBreaker` struct + `retry_budget` parse/register — the dormant slot phase 42 activates; the `TryAcquireRequest` admission re-run on every attempt], ADR-0076 [the 1 MiB `FilterBufferLimitBytes` body-buffer cap — the retry-replay source], ADR-0106/0045/0044/0052/0080/0227), the as-built router upstream-call path (`internal/filter/http/router/router.go` [`ActionResponse` at `:119`, `doH1ClusterAction` at `:588`, the `TryAcquireRequest` admission at `:596`, the `RecordUpstreamResult` sites at `:622/:650/:663`] + `router_h2.go` [`doH2ClusterAction` at `:57`, admission at `:63`], the weighted-cluster dispatch `router_weighted.go:110/:123`), the route→action build (`internal/filter/hcm/config.go:536` `buildRouterAction`; the `clusterRouteAction` struct at `internal/filter/hcm/actions.go:201` — the `retryPolicy` carry site), the HCM body-buffer (`internal/filter/hcm/connection.go:548-600`), the `internal/cluster` circuit-breaker (`circuitbreaker.go` — the `retry_budget` parse at `:52`, the emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` at `:78/:83`), and `internal/admin/admin.go` (the `/stats` endpoint the differential retry-counter reads). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–41 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/41-circuit-breakers/BRAINSTORM.md` section-for-section, reframed for the FOURTH row of the Upstream-robustness family: a re-attempt loop WRAPPING the EXISTING single-attempt driver (NOT a new cluster-accounting struct — it REUSES the phase-41 one for the budget), classifying a buffered response, replaying a buffered body, activating the dormant phase-41 `retry_budget` slot, with a count-based per-host-503 differential (NO new BackendKind). Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-19.

---

## 1. Mission and scope confirmation (42 only)

ROADMAP row `42 | retries-and-hedging | 41 | in-progress | | …` (added by this brainstorm) is a **flat top-level Upstream-robustness-family row** (per ADR-0106 — sibling family rows are NOT pre-populated) WITH a **pre-authorized 42.1/42.2 by-feature split** (§1.4). The row's `depends-on` anchor is phase 41 (the last completed phase; substantive predecessor `7f3c7c15` — retries ACTIVATE the phase-41 `retry_budget` slot and REUSE the phase-40/41 cluster-runtime lifecycle seams).

This is the **fourth row of the Upstream-robustness family**. The family roster at `ROADMAP.md` (§ Feature Families → Upstream robustness): `{active health checks [DONE — phase 39], outlier detection [DONE — phase 40], circuit breakers [DONE — phase 41], retries + hedging, per-protocol connection pooling}`. Phase 42 lands **retries + hedging**. After phase 42 phase-done, **1** family candidate remains: {per-protocol connection pooling — the row where the phase-41-deferred `max_connections`/`max_pending_requests`/`max_connection_pools` enforcement lands} — its own future brainstorm (and the 3 health-gated Load-balancing candidates unblocked at phase 39 also remain).

Branch/directory identifiers: directory `42-retries-and-hedging/`. The work lands in the EXISTING `internal/filter/http/router` package (a new retry-executor sibling for the re-attempt loop + `retry_on` bitset + backoff), the EXISTING `internal/filter/hcm` package (the `retry_policy` parse in `buildRouterAction` + a `retryPolicy` field on `clusterRouteAction`), and the EXISTING `internal/cluster` package (the `retry_budget` active-retry counter on the phase-41 `circuitBreaker`). It is NOT a new Go package (the retry loop is a router subsystem, not a filter; the budget is a cluster subsystem).

Phase 42 is also: (i) the project's **FIRST request-replay control plane** (re-running an upstream call against a re-picked host with a buffered-body replay — distinct from every prior phase, which made each request exactly once). (ii) the **FIRST upstream-robustness feature that RECOVERS a single request** (active HC + outlier detection route *around* unhealthy hosts for *future* requests; circuit breaking *sheds* load; retries *re-attempt the same request*). (iii) a **wrapper over the EXISTING single-attempt driver** (no new dial/buffer path — the loop re-invokes `doH1ClusterAction`/`doH2ClusterAction`, which already re-run CB admission + `RecordUpstreamResult`; byte-neutral when no `retry_policy` is configured). (iv) the **ACTIVATION of the dormant phase-41 `retry_budget` slot** (the emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` stats flip LIVE). (v) anticipated a ONE-to-TWO-ADR phase (the retry-loop-architecture ADR primary).

### 1.1 What phase 42.1 delivers as a self-contained whole (envelope: the retry loop)

Phase 42.1 lands the retry loop over the live single-attempt substrate, as a self-contained whole:

1. **The retry-executor wrapper** (ADR-0249) — a `retryExecutor` in `internal/filter/http/router` that wraps the EXISTING single-attempt `doH1ClusterAction` (`router.go:588`) / `doH2ClusterAction` (`router_h2.go:57`) at their call sites (the H1/H2 main dispatch `router.go:566` / `router_h2.go:42` + the weighted-cluster dispatch `router_weighted.go:110/:123`). When the action carries no `retryPolicy` the executor is a direct pass-through (byte-identical). Otherwise it loops: invoke the driver (one attempt — host pick via LB + CB admission + dial + buffered response), classify, and re-attempt or return.
2. **The `retry_on` classifier** (ADR-0249) — a parsed bitset over the enforced token subset; the executor classifies the returned `ActionResponse.Status` (and the connection/local-origin failure shape) against it. MVP-ENFORCE the connection/5xx-class tokens (`5xx`, `gateway-error`, `connect-failure`, `reset`, `retriable-status-codes` + the explicit `retriable_status_codes[]` list); PARSE-ACCEPT-but-defer the rest (`retriable-headers`, `envoy-ratelimited`, the `grpc-*` tokens). §2.5.
3. **The `retry_policy` parse + reject surface** (ADR-0249) — `buildRouterAction` (`hcm/config.go:536`) reads `RouteAction.retry_policy` (field 9), falling back to `VirtualHost.retry_policy` (field 16) when the route omits it, parses `retry_on`/`num_retries`/`retry_back_off`/`retriable_status_codes`, and attaches a `retryPolicy` struct to `clusterRouteAction` (`hcm/actions.go:201`). `retry_on` is freeform (NO PGV) ⇒ a THIN reject surface (envoy-go's own byte-stable rejects per ADR-0080); exact arms a SPEC pin (D-RT-RETRYON).
4. **The buffered-body replay** (ADR-0249) — each attempt receives a fresh `bytes.NewReader(bodyBuf)` over the already-buffered request body (`connection.go:548-600`, 1 MiB `FilterBufferLimitBytes` cap, ADR-0076). A body that exceeded the cap (streamed, not buffered) is NON-RETRIABLE — a SPEC pin (D-RT-PROTO/D-RT-DIFFERENTIAL).
5. **The exponential backoff** (ADR-0249) — between attempts the executor sleeps `retry_back_off` (base/max interval, reference default ~25ms/250ms, full jitter). Delay-only — it perturbs no count-based assertion. Each attempt re-runs CB admission (`TryAcquireRequest`) + `RecordUpstreamResult` so BOTH circuit breaking and outlier detection observe every attempt. §2.4.
6. **The `retry_budget` activation** (ADR-0249, possibly ADR-0250) — the phase-41 `circuitBreaker` gains a cluster-level atomic active-retry counter; a retry try-acquires `max(min_retry_concurrency, ⌈budget_percent% × activeRequests⌉)`; on exhaustion ⇒ no retry + `upstream_rq_retry_overflow`++ + `rq_retry_open`=1 (the phase-41 emit-0 stats flip LIVE). Static `num_retries` caps independently ⇒ `upstream_rq_retry_limit_exceeded` on exhaustion. §2.3.
7. **The retry stat surface** (ADR-0249) — `cluster.<n>.upstream_rq_retry` / `_retry_success` / `_retry_limit_exceeded` / `_retry_backoff_exponential` (+ possibly `_retry_backoff_ratelimited`) counters, plus the phase-41 `rq_retry_open` / `upstream_rq_retry_overflow` flipping LIVE. §5.
8. **The differential fixture(s) `0075-retry-…`** (+ possibly `0076`) — a per-host-503 topology (`HTTP503Responder`, BackendKind 35) under round-robin: first pick hits 503 ⇒ retry re-picks healthy ⇒ final 200; a second EXHAUSTION arm (single 503 host, `num_retries=N` ⇒ final 503 + `upstream_rq_retry==N` + `upstream_rq_retry_limit_exceeded==1`). §6.
9. **The BEHAVIOR_CONTRACT 42.1 bundle** + the STATE/ROADMAP advance + the row-42 `in-progress` registration (the row flips `done` only when BOTH 42.1 and 42.2 land, per ADR-0106 + `reference_roadmap_split_phase_row_done`).

### 1.2 What phase 42 does NOT deliver (forward to §8)

See §8. Highlights: **hedging** (`HedgePolicy` + `per_try_timeout`) is the **42.2** leg, NOT 42.1; **`request_mirror_policies`** (shadow traffic, `RouteAction` field 30) DEFERRED to a future row; the **deferred `retry_on` tokens** (`retriable-headers`, `envoy-ratelimited`, the `grpc-*` tokens) parse-accept-but-defer; **`retriable_headers`/`retriable_request_headers`** (RetryPolicy fields 9/10, header-match retry gating) parse-accept-but-defer; **`per_try_idle_timeout`** (field 13); **`retry_priority`/`retry_host_predicate`/`host_selection_retry_max_attempts`** (the retry host-selection plugins); **`RX`/`URX` retry response-flag plumbing** (envoy-go has no response-flags surface — §2.6); **streamed (over-1-MiB-cap) body retries**; **retries on non-HTTP (TCP/network) upstreams**.

### 1.3 Phase-done as the FOURTH Upstream-robustness-family row landing

Phase 42 lands the family's fourth row (across the 42.1 + 42.2 legs). After phase 42, the family candidate count is 2 → **1** {per-protocol connection pooling}. The family heading gains a one-line `retries + hedging DONE` note when BOTH legs land. Sibling rows are NOT pre-populated (ADR-0106). The retry-executor wrapper + the activated `retry_budget` slot become durable family assets (the per-protocol-connection-pooling row will enforce the phase-41-deferred `max_connections`/`max_pending_requests` against the same lifecycle; the retry loop is reusable by hedging at 42.2).

### 1.4 ADR-0045 split readiness — a PRE-AUTHORIZED 42.1/42.2 by-feature split

The retries-and-hedging scope splits cleanly along the by-feature seam (the phase-36/38/39 valve): **42.1 = the retry loop** (`retry_policy` re-attempt + `retry_on` + `num_retries` + `retry_back_off` + `retry_budget` activation — a sequential loop) and **42.2 = hedging** (`HedgePolicy` racing concurrent attempts + the project's FIRST `per_try_timeout`, reusing the 42.1 attempt substrate). The two are genuinely separable: 42.1 stands alone as a complete sequential-retry feature; 42.2 layers concurrency + per-attempt timeout on top. Each leg is assessed as a SINGLE SPEC/PLAN/IMPL cycle under the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`) — 42.1 anticipated ~300–450 prod LoC / ~12–16 tasks. The brainstorm PRE-AUTHORIZES the split (the 36.1/36.2 + 38.1/38.2 + 39.1/39.2 precedent); the FINAL ADR-0045 split-gate re-check happens at the 42.1 SPEC + PLAN (the standard valve). Row 42 flips `done` only when BOTH legs land (`reference_roadmap_split_phase_row_done`; the row-36/row-39 precedent — NO parent rollup).

### 1.5 Seed-stub alignment + package placement

No seed stub. The 42.1 work is a new retry-executor file in the EXISTING `internal/filter/http/router` package (alongside `router.go`/`router_h2.go`/`router_weighted.go`), plus a `retryPolicy` field + parse in `internal/filter/hcm` (`actions.go`/`config.go`), plus the active-retry counter on the EXISTING `circuitBreaker` in `internal/cluster` (`circuitbreaker.go`). ZERO new Go packages.

### 1.6 No prebrainstorm-notes branch

There is no off-master prebrainstorm-notes branch for retries + hedging.

### 1.7 Phase 42's relationship to the existing seams (a WRAPPER over the EXISTING single-attempt driver + the EXISTING budget struct)

Retries compose the existing per-request seams and add one loop. It REUSES (a) the single-attempt driver `doH1ClusterAction`/`doH2ClusterAction` (which already does host-pick-via-LB + CB admission + dial + the FULL buffered `ActionResponse`) as the per-attempt unit, (b) the per-request circuit-breaker admission `TryAcquireRequest` (ADR-0248) — re-run on every attempt so circuit breaking caps total in-flight attempts correctly, (c) the per-request outcome seam `RecordUpstreamResult` (ADR-0245) — re-run on every attempt so outlier detection counts each one, (d) the HCM buffered body (`connection.go:548-600`, ADR-0076) — replayed per attempt as a fresh `bytes.Reader`, and (e) the phase-41 `circuitBreaker` struct + its dormant `retry_budget` parse/registration (ADR-0248). It ADDS (f) the `retryExecutor` loop + the `retry_on` bitset + the backoff timer + the active-retry counter. The retry loop is SEQUENTIAL and synchronous per request (one attempt at a time at 42.1; concurrency arrives at 42.2 with hedging); the only new state is the per-request attempt counter + the cluster-level active-retry atomic — NO background goroutine.

---

## 2. Design decisions

### 2.1 Subject confirmation: retries + hedging — the `route.v3.RetryPolicy` / `HedgePolicy` proto surface *(Q-subject → phase 42 row registered)*

`RouteAction.retry_policy` (`RouteAction` field 9; `VirtualHost.retry_policy` field 16 fallback) is a single `route.v3.RetryPolicy` message in the EXISTING go-control-plane dep (`v1.32.4`; ZERO new module). The 42.1 MVP consumes `retry_on`, `num_retries`, `retry_back_off`, `retriable_status_codes`, and the `retry_budget` activation. The hedging surface (`RouteAction.hedge_policy` field 27 = `route.v3.HedgePolicy`; `RetryPolicy.per_try_timeout` field 3) is the 42.2 leg. The full field roster + the default/PGV pins are SPEC obligations (§10, D-RT-PROTO).

### 2.2 Retry-loop architecture: a `retryExecutor` wrapping the existing single-attempt driver *(Q-architecture → Approach A → ADR-0249)*

**Approach A (chosen):** a `retryExecutor` in the router package that wraps `doH1ClusterAction`/`doH2ClusterAction` at their call sites; each loop iteration is one driver call (host pick via LB + CB admission + dial + buffered `ActionResponse`); the executor classifies `ActionResponse.Status` (+ the connection/local-origin failure shape) against the parsed `retry_on` bitset and, on a retriable outcome with `num_retries` + `retry_budget` remaining, sleeps the backoff, replays the buffered body, and re-invokes the driver; otherwise it returns the last `ActionResponse`. Nil `retryPolicy` ⇒ direct pass-through (byte-identical). Chosen over **Approach B** (push the retry loop DOWN into the driver itself — rejected: the driver is the single-attempt unit reused by weighted-cluster dispatch + both protocols; wrapping ABOVE it keeps the attempt unit clean + reuses the CB/outlier per-attempt seams untouched) and **Approach C** (a new parallel upstream path that re-buffers + re-dials independently — rejected: it would duplicate the host-pick + CB-admission + outcome-recording logic the driver already runs, risking a CB/outlier double-count or under-count; the whole point is that the driver ALREADY does the per-attempt accounting correctly). See §1 load-bearing facts.

### 2.3 retry_budget activation: an active-retry counter on the phase-41 circuitBreaker *(Q-retry-budget → Fork: ACTIVATE → ADR-0249, possibly ADR-0250)*

Phase 41 parses `RetryBudget` + registers `rq_retry_open`/`upstream_rq_retry_overflow` emit-0. Phase 42.1 ACTIVATES: the `circuitBreaker` gains a cluster-level `activeRetries atomic.Int64`; before each re-attempt the executor try-acquires a slot if `activeRetries < max(min_retry_concurrency, ⌈budget_percent% × activeRequests⌉)` (the active-request count the phase-41 struct already tracks); on success it increments (released at attempt completion), on exhaustion it does NOT retry, increments `upstream_rq_retry_overflow`, and `rq_retry_open` reads 1. Static `num_retries` is an INDEPENDENT per-request cap: exhausting it (no budget exhaustion) ends the loop with `upstream_rq_retry_limit_exceeded`++. The exact budget formula (ceil vs floor; whether `budget_percent` applies to active requests or active+pending) + the default `20%/3` are SPEC pins (D-RT-BUDGET). Whether the budget activation warrants its own ADR-0250 (vs folding into ADR-0249) is a SPEC call (the phase-39/40/41 multi-ADR-shape valve).

### 2.4 Backoff: exponential with full jitter, delay-only *(Q-backoff → Fork: INCLUDE → self-answered; pinned at SPEC, D-RT-PROTO)*

Between attempts the executor sleeps an exponential backoff derived from `retry_back_off{base_interval, max_interval}` (reference default ~25ms base / 250ms max, full jitter — exact algorithm a SPEC pin). Backoff is DELAY-ONLY: it changes when an attempt fires, never whether or how many. So the count-based differential (§6) is immune to it — there is no timing-margin assertion to flake (the `reference_differential_band_sigma_margin` lesson: never assert on a timing window). The jitter source is the standard library RNG; determinism of the differential comes from the COUNT assertions, not the delays.

### 2.5 retry_on enforcement: the connection/5xx-class subset, parse-accept the rest *(Q → self-answered; SPEC pins the exact enforced set, D-RT-RETRYON)*

MVP-ENFORCE the connection/5xx-class tokens that map onto envoy-go's current failure surface: `5xx` (any upstream 5xx), `gateway-error` (502/503/504), `connect-failure` (the dial-failure local-origin shape the router already records as a 502/503), `reset` (connection reset), `retriable-status-codes` + the explicit `retriable_status_codes[]` list. PARSE-ACCEPT-but-defer the tokens with no current substrate: `retriable-headers` + `retriable_request_headers` (header-match gating), `envoy-ratelimited` (the `x-envoy-ratelimited` header — no local-ratelimit-to-retry bridge yet), the `grpc-*` tokens (`cancelled`/`deadline-exceeded`/`resource-exhausted`/`unavailable`/`internal` — gRPC trailer inspection). `retry_on` is freeform (NO PGV) ⇒ unknown/deferred tokens parse-accept silently (the reference ignores unknown tokens); the reject surface is THIN (D-RT-RETRYON pins whether any token shape is a hard reject).

### 2.6 Response-flags posture: RX/URX are a RECORDED DEPARTURE *(self-answered — the phase-41 CB4 precedent)*

Envoy emits the `RX` (retried) / `URX` (retry-limit-exceeded) response flags on the access-log line. envoy-go has NO response-flags plumbing — `RESPONSE_FLAGS` is hardcoded to `-` (the phase-41 CB4 UO-flag precedent: the overflow path emits the dedicated counter, not a response flag). So the `RX`/`URX` retry flags are a RECORDED DEPARTURE until a response-flags surface lands. The differential asserts STATS (`upstream_rq_retry`/`_retry_limit_exceeded`) + the final status code, NEVER the access-log line. §5.2.

### 2.7 Deferred-policy posture: an additive config surface (`retry_policy`); a THIN reject surface *(self-answered; pinned at SPEC, D-RT-RETRYON)*

`retry_policy` is additive — routes without it are byte-identical (the `retryPolicy` field is nil ⇒ the executor is a pass-through). `retry_on` being freeform (no PGV) makes the reject surface THIN (envoy-go's own byte-stable rejects per ADR-0080); candidate arms are a malformed `retry_back_off` (e.g. `base_interval > max_interval`), a `num_retries` over a sanity bound, or a `retry_budget.budget_percent` outside [0,100] (the phase-41 reject already covers the last). Exact arms + whether a config-parse fuzzer is warranted are SPEC pins (D-RT-RETRYON).

### 2.8 Stat surface: anticipated ~+4–6 `upstream_rq_retry*` counters + 2 phase-41 stats flipping LIVE *(self-answered; SPEC pins, D-RT-STATS)*

§5.

---

## 3. Framework-survey result — a WRAPPER loop over EXISTING seams + 0 new accounting struct + 0 new packages + 0 new go.mod modules (42.1 anticipated)

### 3.1 Framework: a NEW retry-executor loop + an active-retry counter on the EXISTING circuitBreaker *(per §1.7)*

The one genuinely new structure is the `retryExecutor` loop (+ the `retry_on` bitset + the backoff timer) in the router package, plus a single `activeRetries atomic.Int64` added to the EXISTING phase-41 `circuitBreaker` struct. No new cluster-accounting struct (the budget reuses the phase-41 one); no new dial/buffer path (the driver + the buffered body already exist).

### 3.2 NEW packages: NONE

42.1 is a retry-executor file in `internal/filter/http/router` + edits to `internal/filter/hcm` (`actions.go`/`config.go`) + the active-retry counter in `internal/cluster/circuitbreaker.go`. No new package.

### 3.3 go.mod modules: anticipated ZERO new (42.1) *(verified at brainstorm; re-pinned at SPEC D-RT-PROTO)*

`route.v3.RetryPolicy` + `route.v3.HedgePolicy` are in the existing go-control-plane dep (`v1.32.4`, `config/route/v3/route_components.pb.go`); `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES

- The single-attempt driver `doH1ClusterAction` (`router.go:588`) / `doH2ClusterAction` (`router_h2.go:57`) — the per-attempt unit; re-invoked per retry.
- The buffered `ActionResponse` (`router.go:119`) — the classify-against-`retry_on` input.
- The circuit-breaker admission `TryAcquireRequest` (ADR-0248) — re-run per attempt (every attempt counts against `max_requests`).
- The per-request outcome seam `RecordUpstreamResult` (`router.go:622/:650/:663`, ADR-0245) — re-run per attempt (every attempt counts toward outlier detection).
- The HCM buffered body (`connection.go:548-600`, ADR-0076) — replayed per attempt via `bytes.NewReader`.
- The phase-41 `circuitBreaker` struct + the `retry_budget` parse + the emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` (ADR-0248) — the budget substrate + the stats that flip LIVE.
- The `buildRouterAction` route→action build (`hcm/config.go:536`) + the `clusterRouteAction` struct (`hcm/actions.go:201`) — the `retry_policy` parse + carry site.
- The per-host-503 differential topology (`HTTP503Responder`, BackendKind 35, `PerHostBackendKind`) — the deterministic retry trigger (NO new BackendKind).
- The `internal/admin` `/stats` endpoint (the differential retry-counter reads).
- The `reference_docker_probe_bridge_network` live-probe precedent (the SPEC retry-stat + retry-on-classification empirical pins).

---

## 4. Per-route applicability — a NEW per-route config surface (`RouteAction.retry_policy`)

`RouteAction.retry_policy` (with the `VirtualHost.retry_policy` fallback) is the new per-route config surface. A route opts in by setting it; absent, behavior is byte-identical (the `retryPolicy` field is nil ⇒ the executor is a pass-through). Retries compose with active HC + outlier detection + circuit breaking independently: a retry re-picks via the LB (which already filters to the `available` healthy+un-ejected host set), each attempt passes circuit-breaker admission, and the `retry_budget` caps concurrent retries cluster-wide — orthogonal dimensions that stack.

---

## 5. Stat surface hypothesis — anticipated ~+4–6 (`upstream_rq_retry*` counters + 2 phase-41 stats flipping LIVE)

### 5.1 New stat names (SPEC pins, D-RT-STATS)

Anticipated for 42.1, scoped to clusters whose routes carry `retry_policy` (existing fixtures unaffected). The cluster-level retry COUNTERS — **~+4–6**:
- `cluster.<n>.upstream_rq_retry` (counter — total retry attempts beyond the first)
- `cluster.<n>.upstream_rq_retry_success` (counter — requests that ultimately succeeded after ≥1 retry)
- `cluster.<n>.upstream_rq_retry_limit_exceeded` (counter — requests that exhausted the static `num_retries` cap)
- `cluster.<n>.upstream_rq_retry_backoff_exponential` (counter — attempts that waited an exponential backoff)
- `cluster.<n>.upstream_rq_retry_backoff_ratelimited` (counter — attempts backed off by a ratelimit header; LIKELY emit-0 at 42.1, no `x-envoy-ratelimited`→retry bridge — a recorded departure)

Plus the phase-41 emit-0 stats flipping LIVE (NO surface-count delta — already registered at 41):
- `cluster.<n>.circuit_breakers.<default|high>.rq_retry_open` (gauge — now driven by the active-retry counter)
- `cluster.<n>.upstream_rq_retry_overflow` (counter — now incremented on `retry_budget` exhaustion)

Anticipated surface **1163 → ~1168** (~+4–6 new; the 2 phase-41 stats flip without adding to the count). The EXACT retry-counter roster (which names Envoy emits, whether `_backoff_ratelimited` registers, whether a per-route stat tree appears) is a SPEC pin from a live `/stats` scrape of a retrying cluster (D-RT-STATS), NOT hard-committed here.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The `RX`/`URX` retry response flags (no response-flags plumbing — §2.6), the deferred `retry_on` tokens (`retriable-headers`/`envoy-ratelimited`/`grpc-*` — parse-accept, no enforcement), the `_retry_backoff_ratelimited` counter (likely emit-0), and the deferred `retriable_headers`/`retriable_request_headers`/`per_try_idle_timeout`/retry-host-selection-plugin surfaces are recorded departures until their substrate lands.

---

## 6. Differential fixture envelope — anticipated ONE (possibly TWO) directories (42.1)

### 6.1 Fixtures (+1, possibly +2)

`0075-retry-<scenario>` — an HTTP listener → a cluster with ≥2 endpoints where one is an `HTTP503Responder` (BackendKind 35, `PerHostBackendKind`) and the rest healthy, the route carrying `retry_policy{retry_on:"5xx", num_retries:N}`, on BOTH the subject and the reference (`contrib-v1.37.2`); under DETERMINISTIC round-robin the first pick hits the 503 host ⇒ the retry re-picks a healthy host ⇒ final **200**. Assert on BOTH sides: final status 200 + `upstream_rq_retry` delta == 1 + `upstream_rq_total` (counts each attempt). NO `time.Sleep` — the trigger is the deterministic per-host 503 + the deterministic LB pick order (count-based, the `reference_differential_band_sigma_margin` discipline). A SECOND arm (possibly a second dir `0076`) is the EXHAUSTION case: a cluster with a SINGLE 503 host + `num_retries=N` ⇒ every attempt 503 ⇒ final **503** + `upstream_rq_retry == N` + `upstream_rq_retry_limit_exceeded == 1`. The one-vs-two-dirs split (a single multi-arm fixture vs `0075`/`0076`) is a SPEC pin (D-RT-DIFFERENTIAL). RR-determinism cross-side (both sides pick the 503 host first) is a SPEC live-pin. Deliberate breaks: (A) the loop never retries (classify always returns non-retriable) ⇒ the recovery arm gets a 503 ⇒ fail; (B) the retry counter not wired ⇒ the cross-side delta assert fails.

### 6.2 Total

Differential fixtures **76 → 77 (or 78)** at the 42.1 IMPL.

### 6.3 New BackendKind: anticipated NONE

The fixture REUSES the existing `HTTP503Responder` (BackendKind 35, `PerHostBackendKind`) under a deterministic round-robin pick order — the per-host-503 topology the phase-39/40 fixtures already exercise. NO new BackendKind. BackendKind tail STAYS **36** (`BlockingHoldResponder`).

### 6.4 New fuzzer: anticipated NONE-to-ONE

No new wire decoder (retries read no new wire format). A candidate **+1 config-parse fuzzer** (the `retry_policy` parse/`retry_on`-tokenize path — the `parseCircuitBreakers`/`parseOutlierDetection` fuzzer precedent) IF the reject/tokenize surface warrants it; otherwise fuzzers STAY **42**. Pinned at SPEC (D-RT-RETRYON). Anticipated 42 → 42 (or 43).

---

## 7. Anticipated ADRs — 1 (possibly 2) ADR for 42.1 (ADR-0249)

- **ADR-0249** (the load-bearing one) — the retry-loop architecture: the `retryExecutor` wrapping the EXISTING single-attempt driver + the `retry_on` bitset classifier + the buffered-body replay + the exponential backoff + the per-attempt CB-admission/outcome re-run + the `retry_budget` activation on the phase-41 `circuitBreaker` + the `upstream_rq_retry*` stat block + the enforced-subset / parse-accept-rest `retry_on` posture. §Context at the 42.1 SPEC, §Decision/§Consequences at the 42.1 IMPL per ADR-0044.
- A possible **second ADR (ADR-0250)** for the `retry_budget` dynamic-concurrency model (the `max(min_retry_concurrency, budget_percent% × active)` formula + its interaction with the static `num_retries` cap + the two phase-41 stats flipping LIVE) if it warrants separation — finalized at the SPEC (the phase-39/40/41 multi-ADR-shape valve). Next-free is ADR-0249.

---

## 8. Deferred items

- **Hedging (`HedgePolicy` + `per_try_timeout`)** — the 42.2 leg (the pre-authorized second split leg); NOT 42.1.
- **`request_mirror_policies` (RouteAction field 30, shadow traffic)** — DEFERRED to a future Upstream-robustness/traffic-management row.
- **The deferred `retry_on` tokens** (`retriable-headers`, `envoy-ratelimited`, the `grpc-*` tokens) — parse-accept-but-defer (no current header-match / ratelimit-bridge / gRPC-trailer substrate).
- **`retriable_headers` + `retriable_request_headers` (RetryPolicy fields 9/10)** — header-match retry gating; parse-accept-but-defer.
- **`per_try_idle_timeout` (RetryPolicy field 13)** — parse-accept-but-defer (the per-try-timeout family is 42.2).
- **`retry_priority` / `retry_host_predicate` / `host_selection_retry_max_attempts`** — the retry host-selection plugins; deferred.
- **The `RX`/`URX` retry response flags** — blocked on the absent response-flags surface (§2.6); a recorded departure.
- **Streamed (over-1-MiB-cap) body retries** — 42.1 retries only buffered bodies (the cap is the boundary); a streamed body is non-retriable.
- **Retries on non-HTTP (TCP/network) upstreams.**
- **The `_retry_backoff_ratelimited` enforcement** — registered (likely emit-0) at 42.1; enforcement blocked on the ratelimit→retry bridge.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 41's deferred list named "**`max_retries` + `retry_budget` enforcement → the retries+hedging family candidate**" and parsed-and-registered the `retry_budget` + the emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` precisely so phase 42 could ACTIVATE them. Phase 42.1 REALIZES that handoff: the `retry_budget` slot binds, the two emit-0 stats flip LIVE, and the `num_retries` static cap enforces — the forward-compatibility the phase-41 brainstorm anticipated. The phase-40 note that `RecordUpstreamResult` "is the request-outcome observation point that retries may also consume" is REALIZED here: every retry attempt re-runs that seam (so outlier detection counts each attempt). The phase-41 note that the `max_requests` admission re-runs per upstream call is REALIZED: every retry attempt re-runs `TryAcquireRequest` (so circuit breaking caps total in-flight attempts). The phase-41-deferred `max_connections`/`max_pending_requests` enforcement remains for the per-protocol-connection-pooling family row (the last remaining candidate).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

- **D-RT-PROTO** — the exact `route.v3.RetryPolicy` + `RetryPolicy_RetryBackOff` + `HedgePolicy` field roster + types + PGV constraints + defaults (esp. `num_retries` default 1, `retry_back_off` default ~25ms/250ms, `HedgePolicy.initial_requests >= 1` the only real PGV arm, `retry_on` UNVALIDATED; the `RouteAction.retry_policy=9`/`hedge_policy=27`/`request_mirror_policies=30` + `VirtualHost.retry_policy=16`/`hedge_policy=17` + `RetryPolicy.retry_on=1`/`num_retries=2`/`per_try_timeout=3`/`retriable_status_codes=7`/`retry_back_off=8`/`retriable_headers=9`/`retriable_request_headers=10`/`per_try_idle_timeout=13` field numbers); the exact exponential-backoff-with-full-jitter algorithm; the buffered-vs-streamed (over-1-MiB-cap) retriability boundary; `go mod tidy -diff` EMPTY confirmation.
- **D-RT-RETRYON** — the exact enforced `retry_on` token set + the classification mapping (which token matches which `ActionResponse.Status`/failure shape: `5xx`/`gateway-error`/`connect-failure`/`reset`/`retriable-status-codes` + the `retriable_status_codes[]` list), which tokens parse-accept-but-defer, whether any token shape is a hard reject, and the `retry_policy` reject arms (malformed `retry_back_off`, `num_retries` bound; byte-stable wording per ADR-0080); whether a config-parse fuzzer is warranted.
- **D-RT-STATS** — the exact `upstream_rq_retry*` counter names + the precise counter→event mapping (which counter increments on an attempt vs a success vs a limit-exceeded vs a budget-overflow) from a live `/stats` scrape of a retrying cluster; whether `_retry_backoff_ratelimited` registers; whether a per-route retry stat tree appears; confirmation that `rq_retry_open`/`upstream_rq_retry_overflow` are the right phase-41 names to flip LIVE.
- **D-RT-BUDGET** — the exact `retry_budget` concurrency formula (`max(min_retry_concurrency, budget_percent% × active)`; ceil vs floor; active-requests vs active+pending denominator), the default `budget_percent 20%` / `min_retry_concurrency 3`, the interaction with the static `num_retries` cap (independent caps), and the precise `upstream_rq_retry_overflow`-vs-`upstream_rq_retry_limit_exceeded` discriminator (budget exhaustion vs static-cap exhaustion).
- **D-RT-DIFFERENTIAL** — the fixture split (one multi-arm fixture vs per-scenario dirs `0075`/`0076`), the RR-determinism cross-side guarantee (both sides pick the 503 host first), the recovery-arm + exhaustion-arm cross-side assertions (final status + `upstream_rq_retry` delta + `upstream_rq_total` + `upstream_rq_retry_limit_exceeded`), all sleepless/count-based; the deliberate-break shapes.

These are EXECUTED IN-SESSION at the 42.1 SPEC against `envoyproxy/envoy:contrib-v1.37.2` via the live-probe precedent (`reference_docker_probe_bridge_network`), anchoring the ADR-0249 §Context draft.

---

## 11. Prior-phase lessons applied

- **Reuse the forward-compatible seam, don't add a path** (the phase-40/41 lesson) — phase 42 wraps the EXISTING single-attempt driver (which already re-runs CB admission + `RecordUpstreamResult`) and activates the EXISTING phase-41 `retry_budget` slot; NO new dial/buffer path, NO new accounting struct.
- **Byte-stability via a nil-guard** (the 39.1/40.1/41 discipline) — the `retryPolicy` field is nil when no `retry_policy` is configured ⇒ the executor is a pass-through ⇒ byte-identical to today. Every existing fixture stays green (the full 76-dir byte-stability gate must hold throughout).
- **Activate-the-dormant-slot** (the phase-41→42 handoff) — the emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` phase-41 registered precisely for this; they flip LIVE with NO surface-count delta (the AMEND-OD3-4 emit-0-then-activate pattern realized across phases).
- **No timing-margin assertions; count-based determinism** (`reference_differential_band_sigma_margin`) — the retry differential triggers on a deterministic per-host 503 under a deterministic LB pick order; backoff is delay-only and asserted on NOTHING; the assertions are exact counts (`upstream_rq_retry` delta, final status), never a band or a timing window.
- **Record the response-flags departure** (the phase-41 CB4 UO-flag precedent) — `RX`/`URX` are recorded departures (no response-flags surface); the differential asserts stats + final status, never the access-log line.
- **`reference_docker_probe_bridge_network`** — the SPEC retry-stat roster + the `retry_on` classification mapping + the `retry_budget` formula are live-pinned from a `/stats` scrape + observed retry behavior of a retrying reference cluster over a shared bridge network; verify the retry path ran (`upstream_rq_retry > 0`).
- **The two-ADR-shape valve** (phase 36/38/39/40/41) — anticipate 1, possibly 2 ADRs for 42.1 (ADR-0249 + a possible ADR-0250 for the `retry_budget` model); finalize at SPEC.
- **A split-phase row flips `done` only when ALL legs land** (ADR-0106 + `reference_roadmap_split_phase_row_done` — no parent rollup; the row-36/row-39 precedent) — row 42 stays `in-progress` until BOTH 42.1 and 42.2 land, then flips `done`; 1 family candidate remains.
- **Digit-inclusive roster regexes** (`reference_proto_roster_extraction_digits`) — when scraping the `upstream_rq_retry*` stat roster + the proto field roster at SPEC, use digit-inclusive patterns (the `5xx`/`retry`/`grpc-*` token names + the field numbers carry digits).
- **The differential -run selector + `-count=1`** (`reference_differential_run_selector` + `reference_differential_break_protocol_count1`) — the 42.1 fixture subtests run as `TestDifferential/<NNNN>`; the deliberate-break protocol uses `-count=1` to defeat go-test caching (a temporarily-broken retry classifier could otherwise serve a stale PASS).

---

## 12. Section closeout

Phase 42 (`retries + hedging`) — REQUEST RECOVERY at the route boundary, the FOURTH Upstream-robustness-family row. A PRE-AUTHORIZED 42.1/42.2 by-feature split: **42.1 = the retry loop** (a `retryExecutor` WRAPPING the EXISTING single-attempt driver `doH1ClusterAction`/`doH2ClusterAction` + the `retry_on` bitset classifier over the buffered `ActionResponse.Status` + the buffered-body replay [`bytes.NewReader`, 1 MiB cap, ADR-0076] + the exponential backoff + the per-attempt CB-admission/`RecordUpstreamResult` re-run + the `retry_budget` ACTIVATION on the phase-41 `circuitBreaker` + the `num_retries` static cap); **42.2 = hedging** (`HedgePolicy` + the FIRST `per_try_timeout`). ENFORCE the connection/5xx-class `retry_on` subset; PARSE-accept the rest. REUSES the phase-40/41 per-request lifecycle seams + the buffered body + the dormant phase-41 `retry_budget` slot; ADDS the first request-replay control plane + the first feature that RECOVERS a single request. Byte-identical when no `retry_policy`. The phase-41 emit-0 `rq_retry_open`/`upstream_rq_retry_overflow` flip LIVE. Anticipated at the 42.1 IMPL: fixtures 76 → 77/78 (`0075` [+ `0076`]), BackendKind tail 36 UNCHANGED (REUSE `HTTP503Responder`), DECISIONS tail ADR-0248 → ADR-0249 (possibly ADR-0250; next-free ADR-0249), stat surface 1163 → ~1168 (~+4–6 + 2 flip LIVE), fuzzers 42 → 42 (or 43), ZERO new packages + ZERO new go.mod modules. ROADMAP row 42 registers `in-progress`; it flips `done` only when BOTH 42.1 + 42.2 land (NO parent rollup per ADR-0106 + `reference_roadmap_split_phase_row_done`). ALL counts UNCHANGED at this brainstorm commit. The next session authors the 42.1 SPEC (execute D-RT-* against `contrib-v1.37.2`; anchor the ADR-0249 §Context).
