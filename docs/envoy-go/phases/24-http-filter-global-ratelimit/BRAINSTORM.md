# Phase 24 Brainstorm — `envoy.filters.http.ratelimit` (global rate limit)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 24 (`http-filter-global-ratelimit`), the SEVENTEENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at 07.1, `fault` at 09, `header_mutation` at 10, `local_ratelimit` at 11, `csrf` at 12, `buffer` at 13, `compressor` at 14, `bandwidth_limit` at 15, `rbac` at 16, `jwt_authn` at 17, `ext_authz` at 18 with its ADR-0045 18.1+18.2 split, `ext_proc` at 19 with its ADR-0045 19.1+19.2 split, `oauth2` at 20, `adaptive_concurrency` at 21, `lua` at 22 with its ADR-0045 22.1+22.2+22.3 three-way split, and `admission_control` at 23). The next session (lifecycle-state 1 → 2 for phase 24, skill `superpowers:writing-plans` scoped to SPEC authoring per the phase 09..23 precedent) authors `docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Predecessor master tip:** `88101a7` (next-prompt.txt: advance to post-phase-23-IMPL cold-start; pushed to origin). Pre-existing baseline at master tip: **33 differential fixture directories** GREEN (0000-0031; the lua 0026-0029 + any multi-listener combined run may hit the documented `freeTCPPort` port-allocation flake per 22.2 REVIEW §7.4 — re-run in isolation; not a defect); **32 fuzzers** green; h2spec 53/53 PASS at ADR-0051 v1.32.4 pin; build/vet/lint clean; race-tests clean; **18 HTTP filters wired** through boot-registration; **110 stat names**; **15 envoy-go-strict departure records** at BEHAVIOR_CONTRACT. ADR tail at **ADR-0196 (full §Decision + §Consequences body)**; ADR-0194 + ADR-0195 + ADR-0196 all CONSUMED; **next-free ADR-0197**. The `internal/grpcclient/` two-tier framework (ADR-0158: generic `Dialer` + per-service typed wrappers `AuthClient` @ ext_authz + `ProcessorClient` @ ext_proc) is the reuse template for the new gRPC service-client wrapper. The phase-23 encode-side `ResponseStatus()` accessor (ADR-0196) is available. The canonical per-route roster stands at **9 shapes** (ADR-0125; last amended 8 → 9 at phase 22.3 / lua / ADR-0193).

**Phase 24 in one sentence:** Land `envoy.filters.http.ratelimit` as a single-phase §9 family-row exposing the FULL operator-visible global rate-limit surface byte-exactly against upstream Envoy v1.37.2 — the `RateLimitService/ShouldRateLimit` external-gRPC delegation + the route/virtual-host descriptor-action engine (10 canonical actions) + the `RateLimitPerRoute` per-route override with its `vh_rate_limits` inclusion enum + the X-RateLimit response headers + the OK / OVER_LIMIT / error-with-failure-mode dispositions — EXCEPT for RTDS/runtime keying (PARSE-REJECT) and the `extension` + deprecated `dynamic_metadata` actions (PARSE-REJECT), built on TWO framework deltas (a new `RateLimitClient` typed wrapper composing the existing `Dialer`; a new HCM route-table capability that parses + retains + exposes the matched Route's and VirtualHost's `rate_limits` policies to the filter at request time), with a FULLY-DETERMINISTIC cross-side differential against a shared fake gRPC `RateLimitService` that BOTH envoy-go and reference Envoy dial.

---

## 1. Mission and scope confirmation (24 only)

### 1.1 What 24 delivers as a self-contained whole

Phase 24 lands the full operator-visible surface of `envoy.filters.http.ratelimit`:

- **Filter proto surface** — `RateLimit` (filter config): `domain` (REQUIRED; PARSE-REJECT if empty), `rate_limit_service.grpc_service` (REQUIRED; cluster + HTTP/2 gate reusing the ext_authz config-load validation), `timeout` (default 20ms), `failure_mode_deny` (default false ⇒ **fail-open**), `rate_limited_as_resource_exhausted`, `stage` (uint32; the filter's stage; descriptors are matched against it), `request_type` (INTERNAL / EXTERNAL / BOTH), `enable_x_ratelimit_headers` (enum OFF / DRAFT_VERSION_03), `disable_x_envoy_ratelimited_header`, `rate_limited_status`, `response_headers_to_add`. Exact field roster + defaults + enum semantics are a SPEC-time §10 empirical-pin obligation per ADR-0004 (the list above is the BRAINSTORM hypothesis).
- **Descriptor-action engine (the heart)** — per request, the filter walks the matched Route's `rate_limits[]` and the enclosing VirtualHost's `rate_limits[]` (subject to the per-route `vh_rate_limits` inclusion enum), filters entries by `stage`, and applies each entry's `actions[]` to build a descriptor. Envoy's empty-action-drops-the-descriptor discipline is honored (if any action in an entry produces no entry, the whole descriptor is skipped). **10 canonical actions** landed: `source_cluster`, `destination_cluster`, `request_headers`, `remote_address`, `masked_remote_address`, `generic_key`, `header_value_match`, `metadata`, `query_parameters`, `query_parameter_value_match`. The `extension` and deprecated `dynamic_metadata` actions PARSE-REJECT (§2.2).
- **External-gRPC delegation** — build `RateLimitRequest{domain, descriptors, hits_addend}`, invoke `RateLimitService/ShouldRateLimit` via a NEW `internal/grpcclient` typed `RateLimitClient` wrapper (composing the existing `Dialer` per ADR-0158), with the per-request timeout + OnDestroy cancellation (the ext_authz pattern).
- **Dispositions** — `OK` ⇒ continue (+ optional X-RateLimit headers + descriptor-returned response headers); `OVER_LIMIT` ⇒ `SendLocalReply(rate_limited_status, default 429)` + `x-envoy-ratelimited` header (unless `disable_x_envoy_ratelimited_header`) + descriptor-returned response headers + `response_headers_to_add` (or a `RESOURCE_EXHAUSTED` gRPC status if `rate_limited_as_resource_exhausted`); error/timeout ⇒ `failure_mode_deny ? 500 : continue` (fail-open default).
- **Two framework deltas (§3)** — (1) a NEW `RateLimitClient` typed wrapper in `internal/grpcclient/` (ADR-0158 two-tier application); (2) a NEW HCM route-table capability parsing + retaining + exposing the matched Route's + VirtualHost's `rate_limits` policies to the filter at request time (the FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to a filter).
- **Per-route `RateLimitPerRoute`** — the `vh_rate_limits` inclusion enum (OVERRIDE / INCLUDE / IGNORE) + a route-additional `rate_limits[]` via `override_option`. Anticipated NEW **10th canonical** per-route shape + ADR-0125 roster amendment 9 → 10 (§4).
- **Stat surface 110 → ~114** — anticipated cluster-scoped counters (`ratelimit.<stat_prefix>.{ok, error, over_limit, failure_mode_allowed}` + the upstream-cluster `cx_*` family already emitted by the cluster layer). Byte-exact upstream parity; NO extra gauges. Exact roster + prefix template pinned at SPEC-time per ADR-0004 (§5).
- **FULLY-DETERMINISTIC two-directory differential** — `00NN-http-ratelimit` (cross-side: descriptor generation across the action set + OK / OVER_LIMIT / failure-mode legs, byte-exact, against a SHARED fake gRPC `RateLimitService` that BOTH sides dial) + `00NN+1-http-ratelimit-boot-reject` (boot-reject, PGV-mirror e.g. missing `domain`). **Fixture count 33 → 35.** Unlike phase-23 admission_control (whose probabilistic path was intrinsically un-matchable), the rate-limit DECISION is delegated to an external service, so a deterministic fake service makes the FULL decision path cross-side byte-exact.
- **33rd project-wide fuzzer** — `FuzzRateLimitConfigParse` at the standard ~30-corpus-seed baseline, with PARSE-REJECT arms for empty `domain`, missing `rate_limit_service`, the `extension` + `dynamic_metadata` actions, non-empty runtime keys, and malformed action sub-messages. **Fuzzer count 32 → 33.**
- **Subject-only descriptor-action interpreter unit tests** — exact descriptor construction per action type + the empty-action-drop discipline + the `vh_rate_limits` inclusion-enum tiering + the stage filter, in `internal/filter/http/ratelimit/*_test.go`.
- **6-gate phase-done verification** — build / vet+lint / race / differential / fuzzers / h2spec 53/53. Same matrix as phase-09..23.

### 1.2 What 24 does NOT deliver (forward to §8)

See §8 for the explicit deferred-items list. Highlights: RTDS/runtime keying (`disable_key`, runtime-referenced descriptor overrides, `filter_enabled`/`filter_enforced` runtime fractions — PARSE-REJECT/static-only, Runtime/RTDS family); the `extension` action (needs a descriptor-producer extension-point sub-framework with no second consumer — PARSE-REJECT) + the deprecated `dynamic_metadata` action (superseded by `metadata` — PARSE-REJECT); any future descriptor-producer plugin framework.

### 1.3 Phase-done as a §9 family-row landing

Phase 24 closes the SEVENTEENTH §9 family-row. The remaining §9 row count drops from 2 to **1** post-phase-24 (`wasm`). Phase 24 retires the `global rate limit` line item from the ROADMAP §9 HTTP-filters family list, leaving the WASM host sub-project as the sole remaining §9 row.

### 1.4 ADR-0045 split-by-surface readiness — HIGH anticipation for phase 24

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 24's anticipated surface (one filter package + a descriptor-action engine over 10 action types + a new gRPC typed wrapper + a new route-table framework capability + per-route `RateLimitPerRoute` with vh-inclusion semantics + X-RateLimit headers + two differential directories with a shared fake gRPC service + one new fuzzer) puts it at **HIGH** split-readiness — comparable to ext_authz (split 18.1+18.2) and lua (split 22.1+22.2+22.3). **The user's brainstorm decision is to proceed SINGLE-PHASE through SPEC and let the ADR-0045 split-gate fire at PLAN time** (lifecycle-state 2 → 3 GATE) if the task/LoC estimate trips the threshold. The SPEC author writes a single full-surface SPEC; the PLAN author applies the split-gate. A likely split axis (recorded as a planning anchor, NOT a brainstorm-time split): **24.1 = the gRPC client wrapper + filter config + descriptor engine for a core action subset + ShouldRateLimit + OK/OVER_LIMIT/error + failure modes + the cross-side differential**; **24.2 = the remaining action types + X-RateLimit headers + `RateLimitPerRoute` + stage + vh-override semantics**.

### 1.5 Seed-stub alignment

No seed-stub for ratelimit (global) exists in `internal/filter/http/` (consistent with the §9 family-row pattern; each row creates its own package). Phase 24 creates `internal/filter/http/ratelimit/` from scratch. NOTE the naming distinction from phase-11's `internal/filter/http/localratelimit/` (local rate limit, `envoy.filters.http.local_ratelimit`): the global filter is `envoy.filters.http.ratelimit` ⇒ package `ratelimit` (SPEC confirms the exact directory name per the phase-11 ADR-0114 no-underscore directory precedent).

### 1.6 No prebrainstorm-notes branch

No `phase-24-*-prebrainstorm-notes` branch exists. (NOTE: project memory `reference_phase_11_local_ratelimit_prebrainstorm` concerns the LOCAL rate-limit filter at phase 11, NOT this global filter — it does not apply here.) Phase 24 starts cleanly from this BRAINSTORM.md.

### 1.7 Framework-delta posture — TWO NEW deltas (1 typed wrapper + 1 route-table capability)

Phase 24 is NOT framework-lean (unlike phase 21 / phase 23). It introduces TWO framework deltas (§3):

1. A NEW `internal/grpcclient` typed `RateLimitClient` wrapper (the THIRD ADR-0158 two-tier typed wrapper, after `AuthClient` + `ProcessorClient`). Low risk; well-precedented.
2. A NEW HCM route-table capability: parse + retain + expose the matched Route's + VirtualHost's `rate_limits` policies to the filter at request time. This is the FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to an HTTP filter — today envoy-go parses neither field and exposes only the matched route *index*. It is NOT a new `internal/` package (route-table state + new decoder-callback accessors, structurally like the existing `RequestRouteConfig()`).

NO new `internal/` package beyond the `grpcclient` typed-wrapper file. NO new top-level primitive package.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 2 user-decided Q-decisions (scope/phasing + the awkward-action roster) and several precedent-settled defaults (framework-exposure design, RTDS deferral, per-route canonical, test taxonomy). Each is anchored here.

### 2.1 Scope ambition + phasing: Full surface, SINGLE PHASE, split-gate deferred to PLAN *(Q1, user-decided)*

**Decision:** Land the FULL operator-visible surface — the external-gRPC delegation + the route/vhost descriptor-action engine (10 actions) + `RateLimitPerRoute` with `vh_rate_limits` inclusion + X-RateLimit headers + all dispositions — as a SINGLE phase 24, deferring the ADR-0045 split decision to the PLAN-time gate (lifecycle-state 2 → 3). The SPEC is written full-surface; the PLAN author applies the split-gate and splits into 24.1/24.2 if the estimate trips ~25 tasks / ~1500 LoC.

**Rationale:** The user chose "full surface, single phase, let the split-gate decide at PLAN time" over a pre-split or a reduced-MVP. This keeps the brainstorm + SPEC holistic (the descriptor engine, per-route inclusion, and dispositions are tightly coupled and reason best together) while preserving the ADR-0045 release valve at the natural quantitative gate. Split-readiness is recorded as HIGH (§1.4) with a candidate split axis as a planning anchor.

**Anticipated ADRs:** ADR-0197 (filter package + descriptor engine + RateLimitClient + differential strategy); ADR-0198 (route-table rate_limits-exposure framework capability); ADR-0199 (RateLimitPerRoute 10th canonical + ADR-0125 amendment); ADR-0200 (RTDS/runtime + extension/dynamic_metadata deferral PARSE-REJECTs).

### 2.2 Action roster: 10 canonical actions; `extension` + `dynamic_metadata` PARSE-REJECT *(Q2, user-decided → ADR-0200/ADR-0197)*

**Decision:** Land the 10 canonical descriptor actions (`source_cluster`, `destination_cluster`, `request_headers`, `remote_address`, `masked_remote_address`, `generic_key`, `header_value_match`, `metadata`, `query_parameters`, `query_parameter_value_match`). PARSE-REJECT the `extension` action (a typed-extension descriptor-producer that would require a whole extension-registration sub-framework to host arbitrary producer plugins — large, no second consumer) and the deprecated `dynamic_metadata` action (superseded by the `metadata` action in upstream Envoy). Each PARSE-REJECT carries a forward-pointer ADR.

**Rationale:** The user chose "defer both, PARSE-REJECT." This keeps "full surface" meaning every NON-deprecated, NON-extension-point action while avoiding a speculative extension-point framework with no second consumer (the EXTRACT-NOW-only-when-≥2-consumers discipline) and avoiding deprecated-surface carry. The two PARSE-REJECTs are envoy-go-strict departures (upstream accepts both), so each gets a BEHAVIOR_CONTRACT record + forward-pointer.

**Anticipated ADR anchor:** ADR-0200 §Decision (the action-deferral PARSE-REJECTs); the 10-action interpreter shape lives in ADR-0197 §Decision.

### 2.3 Framework-exposure design: route-table parse/retain + decoder-callback accessors (NOT a new `internal/` package) *(default, precedent-settled → ADR-0198)*

**Decision:** Expose the matched Route's + VirtualHost's `rate_limits` policies to the filter by extending the HCM route table (`internal/filter/hcm/`) to parse + retain the `rate_limits[]` slices on each routeEntry and on the vhost, surfaced via NEW decoder-callback accessor(s) (working names `RouteRateLimits()` + `VirtualHostRateLimits()`; exact shape pinned at SPEC). Expose NARROWLY (the `rate_limits` policy data only, NOT the whole matched Route/VirtualHost object). NO new `internal/` package.

**Rationale:** The route table is framework-owned and already parses routes + the single vhost + matches at request time (returning the route index). Retaining `rate_limits` there and exposing it via callbacks mirrors the existing `RequestRouteConfig()` accessor pattern — the smallest delta that fits the existing boundaries. Narrow exposure (rate_limits only) follows the YAGNI / EXTRACT-NOW discipline: a future filter wanting OTHER route/vhost fields triggers a widening decision then, recorded as a forward-pointer (§8). The descriptor-action INTERPRETATION (turning `RateLimit_Action` oneofs into descriptor entries) stays in the filter package — the framework surfaces raw policy data, the filter owns the semantics.

### 2.4 RTDS/runtime deferral: static-only + PARSE-REJECT *(default, precedent-settled → ADR-0200)*

**Decision:** Per the phase-17..23 RTDS-deferral precedent, defer all runtime keying: `disable_key` (runtime disable), runtime-referenced descriptor overrides, and the `filter_enabled` / `filter_enforced` `RuntimeFractionalPercent` fields. Honor static behavior (the filter is always-enabled/always-enforced at its static default); any non-empty runtime key triggers HCM-parse-time PARSE-REJECT with a forward-pointer to the Runtime/RTDS family. (NOTE the phase-11 local_ratelimit precedent: `filter_enabled`/`filter_enforced` default to 0% in reference Envoy, so differential fixtures MUST set both to 100% explicitly on BOTH sides — SPEC verifies whether the global filter shares this default discipline.)

**Rationale:** Matches the §9 standard. Runtime live-reload is a cross-cutting Runtime/RTDS-family concern; static thresholds are the operator-visible MVP. Mirrors phase-21 ADR-0187 + phase-23 ADR-0195.

### 2.5 Per-route shape: NEW 10th canonical (`RateLimitPerRoute`) *(default, precedent-settled → ADR-0199 + ADR-0125 amendment 9 → 10)*

**Decision:** `envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute` carries a `vh_rate_limits` inclusion enum (OVERRIDE / INCLUDE / IGNORE — controls whether the VirtualHost's `rate_limits` apply for this route) + a route-additional `rate_limits[]` (via `override_option`). This does NOT match any of the 9 existing canonical per-route shapes (it is data-only but carries an INCLUSION-ENUM semantics that drives cross-tier descriptor composition), so phase 24 anticipates a NEW **10th canonical** ("data-only-with-vh-inclusion-enum") + an ADR-0125 roster amendment 9 → 10. Resolved via the existing `typed_per_filter_config` mechanism (the per-route config is TPFC; the route/vhost `rate_limits` are the NON-TPFC route-table data from §2.3).

**Rationale:** Phase-23 was the FIRST ADR-0125-skip since phase-22's amendment (REUSE-by-absence). Phase 24 RE-AMENDS — the `vh_rate_limits` inclusion enum is a genuinely new per-route semantic (no prior canonical drives cross-tier inclusion via an enum). The SPEC confirms the exact `RateLimitPerRoute` shape against v1.37.2 (incl. whether `override_option`/`vh_rate_limits` are the exact field names) before ratifying the 10th-canonical wording.

### 2.6 Stat surface: byte-exact upstream counter parity, NO extra gauges *(default, precedent-settled → ADR-0197)*

**Decision:** Expose ONLY the upstream stat surface — anticipated cluster-scoped counters (`ratelimit.<stat_prefix>.{ok, error, over_limit, failure_mode_allowed}` + the existing cluster-layer `cx_*` family) under the upstream-byte-exact prefix template. NO extra-upstream gauge. Project stat count 110 → ~114 (exact roster + prefix pinned at SPEC).

**Rationale:** Conformance/byte-exact parity is the mission (phase-22/23 lesson). Upstream global-ratelimit publishes counters; byte-exact parity means counters only. Exact roster + prefix template is a SPEC-time §10 empirical-pin obligation per ADR-0004.

### 2.7 Test taxonomy: subject-only interpreter + FULLY-DETERMINISTIC two-directory differential *(default, derived from Q1)*

**Decision:** Two-layer test strategy: (A) subject-only unit tests for the descriptor-action interpreter (exact descriptor construction per action + empty-action-drop + vh-inclusion tiering + stage filter); (B) a two-directory differential — `00NN-http-ratelimit` (cross-side: a SHARED fake gRPC `RateLimitService` that BOTH envoy-go and reference Envoy dial, returning deterministic OK/OVER_LIMIT, makes the FULL decision path byte-exact cross-side) + `00NN+1-http-ratelimit-boot-reject` (boot-reject, PGV-mirror reject e.g. missing `domain`). Per `reference_differential_fixture_dispatch_constraint`, the two surfaces are SEPARATE directories from the start. Per `reference_differential_asserter_dispatch`, any subject-side stat/structural assertion on the cross-side fixture goes in `StatsAsserter.AssertStats` (NOT `SubjectAsserter`) and is proven live via a deliberate-break test.

**Rationale:** UNLIKE phase-23 admission_control (whose probabilistic rejection was un-matchable against a foreign RNG), the global filter DELEGATES the decision to an external service. A deterministic fake service that both sides dial removes all nondeterminism, so the full descriptor → request → disposition path is cross-side byte-exact. This is the direct analog of the ext_authz differential (shared fake auth service). SPEC pins the fake-service wire contract + how both config sides reference the same cluster/endpoint.

---

## 3. Framework-survey result — TWO NEW deltas + REUSES

Phase 24 introduces **NO new `internal/` package** but TWO framework deltas (a typed wrapper file in the existing `internal/grpcclient/`; a route-table capability in `internal/filter/hcm/`).

- **DELTA 1: `internal/grpcclient` `RateLimitClient` typed wrapper** — composes the existing generic `Dialer` (ADR-0158 Tier-1). New file `internal/grpcclient/ratelimit_client.go`: `NewRateLimitClient(d *Dialer, clusterName string, timeout time.Duration) (*RateLimitClient, error)` + `(*RateLimitClient).ShouldRateLimit(ctx, *RateLimitRequest) (*RateLimitResponse, error)` + `Close()`. Mirrors `AuthClient` exactly. Proto stubs ready: `ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"` (vendored in go-control-plane v1.32.4); stub via `ratelimitv3.NewRateLimitServiceClient(conn)`. The THIRD ADR-0158 two-tier typed wrapper.
- **DELTA 2: HCM route-table rate_limits exposure** — extend the route table (`internal/filter/hcm/`) to parse + retain the `envoy.config.route.v3.RateLimit` policy slices from the matched Route + the VirtualHost; expose via NEW decoder-callback accessor(s). FIRST framework exposure of route-level NON-TPFC policy data to a filter. NOT a new `internal/` package.
- **REUSE 1: `internal/cluster` + the generic `Dialer`** — cluster lookup + HTTP/2 gate + `passthrough:///` resolver + endpoint pick + TLS-at-cluster-layer, all reused unchanged for the rate-limit service cluster.
- **REUSE 2: `internal/stats/` Counter support** — the counter surface via `Registry.NewCounter`. No framework work.
- **REUSE 3: HTTPRegistry boot-time registration** — `ratelimit.New` wired at `cmd/envoy-go/main.go` at its alphabetical position. **19 HTTP filters wired post-phase-24** (18 → 19).
- **REUSE 4: per-request filter interface (decode/encode hooks)** — decode-side descriptor build + async gRPC call + disposition; encode-side X-RateLimit header injection. Fits the existing per-request instance framework + the async-resume pattern (fault/ext_authz precedent).
- **REUSE 5: HCM-parse-time PARSE-REJECT path** — adds the ratelimit parse arms (empty `domain`; missing `rate_limit_service`; `extension`/`dynamic_metadata` actions; non-empty runtime keys; malformed action sub-messages).
- **REUSE 6: existing `typed_per_filter_config` 3-tier resolver** — `RateLimitPerRoute` is TPFC; resolved via the existing `RequestRouteConfig()`/`Resolve` mechanism (the route/vhost `rate_limits` are the SEPARATE non-TPFC route-table data from DELTA 2).
- **REUSE 7: existing differential-fixture framework + the ext_authz fake-gRPC-service test precedent** — two new directories + a shared fake `RateLimitService`.
- **REUSE 8: existing fuzzer-corpus framework** — `FuzzRateLimitConfigParse` as the 33rd fuzzer.
- **REUSE 9: SendLocalReply / `x-envoy-ratelimited` local-reply primitive** — the OVER_LIMIT 429 path reuses the SendLocalReply mechanism (fault/local_ratelimit/csrf precedent).

---

## 4. Per-route shape — NEW 10th canonical (`RateLimitPerRoute`) + ADR-0125 amendment 9 → 10

`RateLimitPerRoute` carries the `vh_rate_limits` inclusion enum (OVERRIDE / INCLUDE / IGNORE) + a route-additional `rate_limits[]` (via `override_option`). The inclusion enum drives cross-tier descriptor composition (whether the VirtualHost's `rate_limits` apply for the route) — a per-route semantic not present in any of the 9 existing canonicals. **Classification: NEW 10th canonical** ("data-only-with-vh-inclusion-enum"). ADR-0125 roster amendment 9 → 10 anticipated at phase 24. This RE-AMENDS after phase-23's REUSE-by-absence skip. The SPEC confirms the exact `RateLimitPerRoute` field names + enum values + `override_option` structure against reference Envoy v1.37.2 / go-control-plane v1.32.4 before ratifying the 10th-canonical wording.

---

## 5. Stat surface hypothesis

### 5.1 Counter surface roster (BRAINSTORM hypothesis; SPEC-time empirical pin)

| Name | Type | Semantics | Encoding |
|---|---|---|---|
| `ok` | Counter | `ShouldRateLimit` returned OK (under limit) | int64 monotonic |
| `over_limit` | Counter | `ShouldRateLimit` returned OVER_LIMIT (rejected) | int64 monotonic |
| `error` | Counter | gRPC call failed (transport / timeout / malformed) | int64 monotonic |
| `failure_mode_allowed` | Counter | error occurred AND `failure_mode_deny=false` ⇒ request allowed through | int64 monotonic |

(Upstream may also emit `cluster_not_found` and the X-RateLimit-related counters; the exact roster + the `ratelimit.<stat_prefix>.<stat>` prefix template + whether stats anchor at a filter `stat_prefix` or the HCM stat_prefix are SPEC-time §10 empirical-pin obligations per ADR-0004. The cluster-layer `cx_*` family for the rate-limit-service cluster is already emitted by `internal/cluster`.)

### 5.2 Project stat count delta

110 → ~114 (+4 counters hypothesized; SPEC refines; no gauges).

### 5.3 envoy-go-strict departure flags

Anticipated BEHAVIOR_CONTRACT records (a new `### envoy.filters.http.ratelimit` subsection): the RTDS/runtime-key PARSE-REJECT (§2.4) + the `extension` action PARSE-REJECT + the deprecated `dynamic_metadata` action PARSE-REJECT (§2.2). Departure-record count 15 → ~18 anticipated (SPEC refines; some may consolidate into one descriptor-deferral record).

---

## 6. Differential fixture envelope — two directories

Per project memory `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side and boot-reject surfaces are SEPARATE directories from the start.

### 6.1 `00NN-http-ratelimit` (cross-side)

A SHARED fake gRPC `RateLimitService` (a test server implementing `RateLimitService/ShouldRateLimit` with a deterministic descriptor → OK/OVER_LIMIT map) is dialed by BOTH envoy-go and reference Envoy v1.37.2 (both configs point the `rate_limit_service.grpc_service.envoy_grpc.cluster_name` at a cluster whose endpoint is the fake server). Cross-side legs:
- **OK leg** — a request whose descriptors map to OK ⇒ admitted; byte-exact cross-side (incl. X-RateLimit headers if enabled).
- **OVER_LIMIT leg** — a request whose descriptors map to OVER_LIMIT ⇒ deterministic 429 (`rate_limited_status`) + `x-envoy-ratelimited` + descriptor-returned headers + `response_headers_to_add`; byte-exact cross-side.
- **Descriptor-generation legs** — requests exercising several action types (generic_key, request_headers, remote_address, header_value_match, …) so the fake service sees the descriptors BOTH sides built (the fake service can assert descriptor equality, or the fixture asserts the resulting disposition).
- **Failure-mode leg** — fake service returns an error / the cluster is made unreachable ⇒ `failure_mode_deny=false` admit (fail-open); byte-exact cross-side.
- **Per-route `vh_rate_limits` inclusion leg** — INCLUDE vs OVERRIDE vs IGNORE drives which descriptors are sent; byte-exact cross-side.
- **Subject-only structural** — descriptor-action interpreter internals + stat-surface delta asserted via `StatsAsserter.AssertStats` (per `reference_differential_asserter_dispatch`; proven live via deliberate-break).

### 6.2 `00NN+1-http-ratelimit-boot-reject` (boot-reject)

A shared PGV-mirror reject where upstream Envoy ALSO rejects at boot (so the boot-reject is byte-comparable, NOT an envoy-go departure): e.g. missing `domain` (REQUIRED) or missing `rate_limit_service`. Common stderr substring pinned at SPEC. NOTE: the RTDS/runtime-key reject + the `extension`/`dynamic_metadata` action rejects are NOT boot-reject fixture candidates — upstream Envoy ACCEPTS them, so an envoy-go reject would DIVERGE by design; those departures are unit-tested + BEHAVIOR_CONTRACT-recorded.

### 6.3 Fixture count

33 → 35 (two directories: `00NN-http-ratelimit` + `00NN+1-http-ratelimit-boot-reject`; the next free numbers are SPEC-confirmed, hypothesized 0032 + 0033).

### 6.4 Listener topology

Single listener with a single HCM containing the ratelimit filter (alphabetical position) + router terminator, plus the rate-limit-service cluster pointing at the fake gRPC server. SPEC confirms whether a second listener/cluster is needed for the fake service (avoid the `freeTCPPort` combined-run flake surface where possible).

---

## 7. Anticipated ADRs — 4 ADRs (ADR-0197 .. ADR-0200)

### 7.1 ADR-0197 — global ratelimit filter package + descriptor engine + RateLimitClient + deterministic cross-side differential strategy

**Decision body summary:** The `internal/filter/http/ratelimit/` package shape; the descriptor-action engine (10-action interpreter; empty-action-drop discipline; stage filter; cross-tier composition driven by the per-route inclusion enum); the NEW `internal/grpcclient` `RateLimitClient` typed wrapper composing the `Dialer` (ADR-0158 application, the third typed wrapper); the both-sides decode-build/encode-X-RateLimit discipline; the OK / OVER_LIMIT / error-with-failure-mode dispositions + the `x-envoy-ratelimited` local-reply; the FULLY-DETERMINISTIC cross-side differential via a shared fake gRPC `RateLimitService`; the two-layer test taxonomy.

**Consequences summary:** 19 HTTP filters wired; ~4-counter byte-exact stat surface (no gauges); the third ADR-0158 typed wrapper (cadence note); SPEC-time D-question slate (stat roster/prefix pin; filter-config field roster + defaults pin; X-RateLimit header wire-shape pin; local-reply byte-pin; fake-service wire contract).

### 7.2 ADR-0198 — HCM route-table `rate_limits` exposure framework capability

**Decision body summary:** The route table parses + retains the matched Route's + VirtualHost's `envoy.config.route.v3.RateLimit` policy slices and exposes them via NEW decoder-callback accessor(s); FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to an HTTP filter; narrow exposure (rate_limits only).

**Consequences summary:** A new framework capability reusable by any future filter needing route-level rate-limit policy; a forward-pointer for a future widening if a 2nd consumer wants other route/vhost fields; the descriptor-action INTERPRETATION stays filter-owned (framework surfaces raw policy, filter owns semantics).

### 7.3 ADR-0199 — `RateLimitPerRoute` 10th canonical per-route shape + ADR-0125 amendment 9 → 10

**Decision body summary:** `RateLimitPerRoute` (`vh_rate_limits` inclusion enum + route-additional `rate_limits[]` via `override_option`) is the NEW 10th canonical per-route shape ("data-only-with-vh-inclusion-enum"); the inclusion enum drives cross-tier descriptor composition. ADR-0125 roster amendment 9 → 10. RE-AMENDS after phase-23's REUSE-by-absence skip.

**Consequences summary:** The canonical-per-route roster grows 9 → 10; future filters with a vh-inclusion-enum per-route shape compose against this canonical; the SPEC confirms the exact field/enum names against v1.37.2 before ratification.

### 7.4 ADR-0200 — RTDS/runtime + `extension`/`dynamic_metadata` action deferral PARSE-REJECTs

**Decision body summary:** Per Q1+Q2, runtime keying (`disable_key`, runtime-referenced descriptor overrides, `filter_enabled`/`filter_enforced` runtime fractions) is honored only for static behavior — non-empty runtime keys PARSE-REJECT (forward-pointer to the Runtime/RTDS family, mirrors phase-21 ADR-0187 + phase-23 ADR-0195); the `extension` action PARSE-REJECTs (forward-pointer to a future descriptor-producer extension-point); the deprecated `dynamic_metadata` action PARSE-REJECTs (superseded by `metadata`).

**Consequences summary:** Operators configure static descriptors/thresholds; runtime keying + extension producers + deprecated dynamic_metadata are forward-pointers; the PARSE-REJECTs use the existing HCM-parse-time framework; ~3 BEHAVIOR_CONTRACT departure records (each reject is an envoy-go departure since upstream accepts them).

### 7.5 Next-free-ADR hypothesis

Following the phase-20/21/23 D11-style hypothesis (next-free ADR stays UNCONSUMED at phase-done), phase-24 hypothesizes **ADR-0201 stays UNCONSUMED at phase-24 phase-done** (4 ADRs consumed: ADR-0197 .. ADR-0200; next-free advances 0197 → 0201). HOLD-with-known-risk, NOT GUARANTEED-HOLD: phase 24 has a HIGH split-readiness + a brand-new route-table framework capability + a third gRPC typed wrapper + a 10th-canonical amendment, so the surprise surface is larger than phase-23's. A PLAN-time split (24.1/24.2) would re-map the ADR consumption across sub-phases. The SPEC's §10 empirical pins (esp. the exact `RateLimitPerRoute` shape + the filter-config field roster + the fake-service wire contract) may force additional ADR consumption.

---

## 8. Deferred items

1. **RTDS/runtime keying** — `disable_key`, runtime-referenced descriptor overrides, `filter_enabled`/`filter_enforced` runtime fractions — PARSE-REJECT / static-only per Q1 + ADR-0200. Closes after the Runtime/RTDS family phase.
2. **`extension` descriptor action** — PARSE-REJECT per Q2 + ADR-0200; needs a descriptor-producer extension-point sub-framework with no second consumer. A future phase that lands ≥2 descriptor-producer extensions extracts the framework.
3. **Deprecated `dynamic_metadata` action** — PARSE-REJECT per Q2 + ADR-0200; superseded by the `metadata` action. Re-open only if a real config corpus needs the deprecated form.
4. **Route-table exposure widening** — DELTA 2 exposes `rate_limits` only; a future filter needing OTHER matched Route/VirtualHost fields triggers a widening decision (forward-pointer per §2.3 + ADR-0198 §Consequences).
5. **Filter-config field roster + defaults byte-exact pin** — SPEC-time §10 empirical-pin obligation per ADR-0004 (the §1.1 list is the BRAINSTORM hypothesis).
6. **Stat-roster + prefix-template byte-exact pin** — SPEC-time §10 empirical-pin obligation per ADR-0004.
7. **X-RateLimit response-header wire-shape pin** — the `enable_x_ratelimit_headers` DRAFT_VERSION_03 header set (`x-ratelimit-limit`, `x-ratelimit-remaining`, `x-ratelimit-reset`) wire shape pinned at SPEC-time per ADR-0004.
8. **OVER_LIMIT local-reply body/header byte-pin** — the 429 reject body + header set (incl. `x-envoy-ratelimited`) pinned at SPEC-time per ADR-0004.
9. **`RateLimitPerRoute` exact shape pin** — field names + `vh_rate_limits` enum values + `override_option` structure confirmed at SPEC-time before ratifying the 10th canonical.
10. **Fake gRPC `RateLimitService` wire contract** — the test-server descriptor → response map + how both config sides reference the same cluster/endpoint, pinned at SPEC-time (the ext_authz fake-service precedent).
11. **PLAN-time ADR-0045 split decision** — single-phase through SPEC; the PLAN author applies the split-gate (candidate axis 24.1/24.2 per §1.4).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 24 PICKS UP several closures from phase-11 local_ratelimit's deferred-items list (SPEC §2.1 + §13.5 + BEHAVIOR_CONTRACT). Phase 11 deferred — explicitly "couples to `global_ratelimit` future phase" — the following clusters that phase 24 now lands:

- **descriptor-action cluster** — phase 11 has NO descriptor support (plain token-bucket); phase 24 lands the full route/vhost descriptor-action engine. CLOSURE.
- **X-RateLimit headers + vh policy cluster** — phase 11 deferred `enable_x_ratelimit_headers` + vh-policy; phase 24 lands the X-RateLimit headers + `vh_rate_limits` inclusion. CLOSURE.
- **multi-stage cluster** — phase 11 deferred `stage`; phase 24 lands the `stage` field + descriptor stage-filtering. CLOSURE.

These phase-11 BEHAVIOR_CONTRACT deferral notes convert from "deferred — couples to global_ratelimit" to "lifted at phase 24" at phase-24 IMPL's next-touchpoint (per the cross-phase deferral-lift discipline, e.g. ADR-0190 §(v)). NOTE: phase 11's `gRPC trailer` + `xDS cluster-state` + `per-connection lifecycle` deferral clusters are NOT picked up by phase 24 (they remain deferred to their respective families). The phase-23 Clock-seam EXTRACT-NOW forward-pointer is NOT relevant to phase 24 (no timer/clock seam). The ADR-0158 grpcclient two-tier discipline's "future consumers compose their own typed wrapper" forward-pointer IS consumed (the third typed wrapper, `RateLimitClient`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D1 (Filter-config field roster + defaults empirical pin)** — verify the `RateLimit` filter-config field roster + defaults (`domain` required; `timeout` 20ms; `failure_mode_deny` false ⇒ fail-open; `request_type`; `rate_limited_status`; `enable_x_ratelimit_headers` enum; `disable_x_envoy_ratelimited_header`; `stage`; `rate_limited_as_resource_exhausted`; `response_headers_to_add`) IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 per ADR-0004.
- **D2 (Stat roster + prefix empirical pin)** — verify the counter roster (`ok`/`over_limit`/`error`/`failure_mode_allowed` + any `cluster_not_found`) + the `ratelimit.<stat_prefix>.<stat>` prefix template + whether stats anchor at a filter `stat_prefix` or the HCM stat_prefix per ADR-0004.
- **D3 (`RateLimitPerRoute` exact shape)** — confirm the `vh_rate_limits` enum values (OVERRIDE/INCLUDE/IGNORE) + `override_option` + route-additional `rate_limits[]` field names; ratify the 10th-canonical wording + the ADR-0125 amendment.
- **D4 (X-RateLimit + local-reply byte-pin)** — pin the `enable_x_ratelimit_headers` DRAFT_VERSION_03 header set wire shape + the OVER_LIMIT 429 local-reply body + header set (incl. `x-envoy-ratelimited`) byte-exactly per ADR-0004.
- **D5 (Fake gRPC `RateLimitService` wire contract)** — design the deterministic test-server descriptor → response map + the shared-cluster config both sides reference; confirm the cross-side determinism holds (the ext_authz fake-service precedent).
- **D6 (`RateLimitRequest` descriptor wire shape)** — confirm the `RateLimitRequest{domain, descriptors[], hits_addend}` wire shape + the descriptor entry `{key, value}` ordering both sides emit (so the fake service / differential can assert descriptor equality).
- **D7 (PLAN-time split-gate)** — the SPEC's task/LoC estimate feeds the PLAN-time ADR-0045 split-gate; the SPEC records the candidate 24.1/24.2 axis (§1.4) as a planning anchor.

Additional SPEC-time D-questions may surface during the SPEC's phase-24-specific D-question slate; these 7 are the BRAINSTORM-anchored set.

---

## 11. Prior-phase lessons applied

- **Phase-18 lesson — the ADR-0158 two-tier gRPC-client framework (generic `Dialer` + per-service typed wrapper) is the reuse template for external-service filters.** Applied to §3 DELTA 1: the `RateLimitClient` is the third typed wrapper composing the existing `Dialer`; no `Dialer` API change.
- **Phase-18/19/22 lesson — large filters pre-split (18.1/18.2, 19.1/19.2, 22.1/22.2/22.3); the split-gate is the quantitative arbiter.** Applied to §1.4 + §2.1: split-readiness HIGH; the user chose single-phase-through-SPEC + PLAN-time split-gate.
- **Phase-17..23 lesson — full proto surface with PARSE-REJECT for RTDS-coupled subfields is the standard.** Applied to §2.4 + ADR-0200.
- **Phase-23 lesson — when full cross-side parity is intrinsically blocked, split into subject-only + structural cross-side.** INVERTED here: the rate-limit decision is DELEGATED to an external service, so a deterministic shared fake service makes the FULL path cross-side byte-exact (§2.7) — phase 24 does NOT face phase-23's RNG-matching block.
- **Phase-22 lesson — the differential runner dispatches ONE branch per fixture directory (cross-side XOR boot-reject); plan SEPARATE directories from the start** (`reference_differential_fixture_dispatch_constraint`). Applied to §6 — two directories from the start.
- **Phase-23 lesson — cross-side fixtures put subject-side assertions in `StatsAsserter.AssertStats` (NOT `SubjectAsserter`) and prove them live via deliberate-break** (`reference_differential_asserter_dispatch`; bit fixture 0030). Applied to §2.7 + §6.1.
- **Phase-20/21/23 lesson — the next-free-ADR-stays-UNCONSUMED hypothesis is a useful planning anchor + phase-done check.** Applied to §7.5 — ADR-0201 hypothesized UNCONSUMED (HOLD-with-known-risk, larger surprise surface than phase-23).
- **Phase-22 lesson — conformance/byte-exact parity is the mission; don't add extra-upstream surface.** Applied to §2.6 — counters only, no extra gauge.
- **Phase-11 lesson — local_ratelimit explicitly deferred the descriptor-action / X-RateLimit / multi-stage clusters to "global_ratelimit future phase."** Applied to §9 — phase 24 picks up those closures.

---

## 12. Section closeout

This BRAINSTORM.md is **lifecycle-state 0 → 1 complete** for phase 24. The next session (lifecycle-state 1 → 2, skill `superpowers:writing-plans` scoped to SPEC authoring) authors `docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` based on:

1. The 2 user-decided Q-decisions (§2.1 full-surface single-phase + PLAN-time split-gate; §2.2 10-action roster + extension/dynamic_metadata PARSE-REJECT) + the precedent-settled defaults (§2.3-§2.7).
2. The framework-survey result (§3): TWO deltas (RateLimitClient typed wrapper; route-table rate_limits exposure) + REUSES; NO new `internal/` package.
3. The per-route NEW-10th-canonical classification (§4): `RateLimitPerRoute` vh-inclusion-enum shape; ADR-0125 amendment 9 → 10; RE-AMENDS after phase-23's skip.
4. The stat surface hypothesis (§5): 110 → ~114; counters only, no gauges; prefix template pinned at SPEC.
5. The two-directory differential envelope (§6): `00NN-http-ratelimit` cross-side (shared fake gRPC service ⇒ FULLY deterministic) + `00NN+1-http-ratelimit-boot-reject`; fixtures 33 → 35.
6. The anticipated ADRs (§7): ADR-0197 .. ADR-0200; ADR-0201 hypothesized UNCONSUMED at phase-done.
7. The deferred-items register (§8): 11 forward-pointers.
8. The cross-phase closure pickup (§9): phase-11 descriptor-action / X-RateLimit / multi-stage clusters lifted at phase 24.
9. The 7 SPEC-time D-questions (§10).
10. The prior-phase lessons applied (§11).

The SPEC author is responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004, including the filter-config field roster (D1), the stat-roster/prefix (D2), the `RateLimitPerRoute` shape (D3), the X-RateLimit + local-reply byte-pin (D4), the fake-service wire contract (D5), and the `RateLimitRequest` descriptor wire shape (D6). The SPEC's task/LoC estimate feeds the PLAN-time ADR-0045 split-gate (D7).

**Hand-off:** BRAINSTORM-time scope is complete. SPEC author proceeds.
