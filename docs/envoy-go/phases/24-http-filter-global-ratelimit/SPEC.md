# Phase 24 SPEC — `envoy.filters.http.ratelimit` (global rate limit)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `24` already at `in-progress` (registered at the phase-24 BRAINSTORM commit per the phase-20-established BRAINSTORM-time row-registration precedent; per-cell narrative updated at this SPEC commit with the SPEC-done annotation; status stays `in-progress` until IMPL phase-done with all 6 gates GREEN). Per ADR-0045 the split disposition is **DEFERRED to PLAN time** (Q1-settled, BRAINSTORM §2.1): the SPEC is written full-surface; the PLAN author applies the split-gate. The post-empirical LoC envelope is **~1900–2700 LoC** (above the ADR-0045 ~1500 split-gate — see §1.2 + §16), so split-readiness is **HIGH/CONFIRMED**; the candidate 24.1/24.2 axis is recorded in §16 as a planning anchor. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–23 precedent. This SPEC is the authoritative input to the phase-24 PLAN.
>
> **PLAN-time SPLIT FIRED (2026-05-22; ADR-0201):** the PLAN author APPLIED the ADR-0045 split-gate (post-empirical LoC envelope ~1900–2700 > the ~1500 gate; §1.2 + §3.4) and SPLIT phase 24 — along the §16 candidate axis — into two sub-phases: **`24.1-global-ratelimit-core-and-route-table`** (the core decision path: DELTA 1 `RateLimitClient` + filter config + the descriptor engine for the CORE action subset [`generic_key`, `request_headers`, `remote_address`, `destination_cluster`, `header_value_match`] + `ShouldRateLimit` dispatch + OK/OVER_LIMIT/error dispositions + failure modes + the cluster-scoped 4-counter stat surface + DELTA 2 the route-table `rate_limits` exposure + the §5 PARSE-REJECT roster + the cross-side differential `0032` (b/c/d/e) + the boot-reject `0033`; ADRs ADR-0197[core] + ADR-0198 + ADR-0200) + **`24.2-global-ratelimit-perroute-and-headers`** (the remaining surface: the remaining actions [`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`] + the X-RateLimit DRAFT_VERSION_03 headers + `RateLimitPerRoute` [the 10th canonical + ADR-0125 9→10] + the `stage` multi-stage path + the `vh_rate_limits` Axis-B composition + the `0032` (f/g) scenarios; ADRs ADR-0199 + the X-RateLimit/remaining-actions slice of ADR-0197). **This parent SPEC is RETAINED as the master SPEC** (it carries the full §1.1 AMEND-1..11 catalog, the §4 descriptor engine, the §5 PARSE-REJECT roster, the §6 code shapes, the §7 differential, the §10 ADR anchor map, the §11 D1–D7 pin matrix, the §13 BEHAVIOR_CONTRACT bundle, the §14 testing taxonomy, the §15 acceptance checklist); each sub-phase SPEC REFERENCES this parent's §§ rather than repeating them. ROADMAP row `24` becomes the parent (`in-progress`, `sub-phases = 24.1, 24.2`); 24.1 ships first; the parent row flips `in-progress → done` at 24.2's phase-done (the rollup discipline per the 18/19/22 precedent). **Next-free ADR advances `ADR-0201` → `ADR-0202`** (ADR-0201 consumed by the split ADR; the §9-C D-hypothesis escape-valve reserve moves to ADR-0202 — hypothesized UNCONSUMED at phase-done, re-mapped across 24.1/24.2). See ADR-0201 (the split-application ADR, mirroring ADR-0164/ADR-0084).
>
> **Predecessor:** `docs/envoy-go/phases/24-http-filter-global-ratelimit/BRAINSTORM.md` (the 2 user-decided Q-decisions [Q1 full-surface single-phase + PLAN-time split-gate; Q2 10-action roster + extension/dynamic_metadata PARSE-REJECT] + the precedent-settled defaults §2.3–§2.7 + the §3 framework-survey [TWO deltas] + the §8 11-item deferred-items register + the §10 D1–D7 SPEC-time empirical pins). The §10 empirical pins are resolved in this SPEC's §11; the §1.1 amendment block records the BRAINSTORM corrections (AMEND-1..AMEND-11) driven by the empirical scrape. NO off-master prebrainstorm-notes branch was authored for phase 24 (the `reference_phase_11_local_ratelimit_prebrainstorm` memory concerns the LOCAL rate-limit filter at phase 11, NOT this global filter).
>
> **Scope (per BRAINSTORM §1.1 + the SPEC-time empirical-pin scrape):** phase 24 lands `envoy.extensions.filters.http.ratelimit.v3.RateLimit` (the canonical Envoy v1.37.2 GLOBAL rate-limit filter) as the SEVENTEENTH §9 family-row under the 07.1 framework, with TWO framework deltas (a NEW `internal/grpcclient` `RateLimitClient` typed wrapper; a NEW HCM route-table `rate_limits`-exposure capability via a chain-seeded decoder-callback accessor pair). MVP envelope: the FULL operator surface minus RTDS — the external-gRPC `RateLimitService/ShouldRateLimit` delegation + the route/vhost descriptor-action engine (10 canonical actions) + `RateLimitPerRoute` per-route override (the `vh_rate_limits` inclusion enum) + the X-RateLimit (DRAFT_VERSION_03) response headers + the OK / OVER_LIMIT / error-with-failure-mode dispositions + the cluster-scoped 4-counter stat surface. **PARSE-REJECT** the `extension` + deprecated `dynamic_metadata` descriptor actions (§5.2); honor static behavior for the route-level `disable_key` runtime key + the hardcoded `ratelimit.http_filter_enabled`/`ratelimit.http_filter_enforcing` runtime keys (always-enabled / always-enforcing — there are NO `filter_enabled`/`filter_enforced` proto fields per AMEND-2). **NEW 10th canonical per-route shape** (`RateLimitPerRoute`) + ADR-0125 amendment 9 → 10 (RE-AMENDS after phase-23's REUSE-by-absence skip). **FULLY-DETERMINISTIC two-directory differential** via a SHARED fake gRPC `RateLimitService` dialed by BOTH sides.
>
> **ADR continuity:** Phase 23 closed at ADR-0196 (full body; ADR-0196 was CONSUMED at phase-23 IMPL Task 9a — the planned-UNCONSUMED D-hypothesis did NOT hold); next-free `ADR-0197`. Phase 24 anticipates **4 NEW ADRs** (ADR-0197 + ADR-0198 + ADR-0199 + ADR-0200) + **ZERO IN-PLACE §Decision AMENDMENTs** + **ONE ADR-0125 amendment** (9 → 10, anchored in ADR-0199). §Context drafts anchor at this SPEC commit (appended to `DECISIONS.md` per ADR-0044 §Context-draft discipline); §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL. **Next-free ADR after phase-24 SPEC commit advances `ADR-0197` → `ADR-0201`** (4 numbers consumed). **D-style hypothesis** (BRAINSTORM §7.5): ADR-0201 stays UNCONSUMED at phase-24 phase-done — HOLD-with-known-risk, **larger surprise surface than phase-23** (DELTA 2 is a genuinely new chain-seeded framework primitive per AMEND-9; the cluster-scoped cross-namespace stat write is novel per AMEND-10; a PLAN-time 24.1/24.2 split would re-map ADR consumption across sub-phases).
>
> **Authored:** 2026-05-22. Empirical scrape executed this session via parallel-subagent fan-out against reference Envoy **v1.37.2** C++ source (raw GitHub at tag `v1.37.2`) + the **go-control-plane v1.32.4** proto bindings in the local module cache (`.../go-control-plane/envoy@v1.32.4/`), per ADR-0004 + ADR-0051.

---

## 1. Purpose

Phase 24 lands `envoy.extensions.filters.http.ratelimit.v3.RateLimit` — the canonical Envoy v1.37.2 GLOBAL rate-limit filter — under the 07.1 framework, as the SEVENTEENTH §9 production HTTP filter. Per request, the filter walks the matched Route's `rate_limits[]` policies and (subject to the per-route `vh_rate_limits` inclusion enum) the enclosing VirtualHost's `rate_limits[]`, filters them by `stage`, applies each policy's ordered `actions[]` to build descriptors, delegates the rate-limit DECISION to an external `RateLimitService` via `ShouldRateLimit` (gRPC), and dispositions the response: `OK` ⇒ continue; `OVER_LIMIT` ⇒ `SendLocalReply` (429 by default) with `x-envoy-ratelimited`; error/timeout ⇒ `failure_mode_deny ? status_on_error(500) : continue` (fail-open default). It establishes the entire `internal/filter/http/ratelimit/` package, the descriptor-action engine over 10 canonical actions, the NEW `internal/grpcclient` `RateLimitClient` typed wrapper (the THIRD ADR-0158 two-tier wrapper), the NEW HCM route-table `rate_limits`-exposure capability, the cluster-scoped 4-counter stat surface, the 33rd project-wide fuzzer `FuzzRateLimitConfigParse`, and the two differential fixture directories `0032-http-ratelimit` (cross-side, deterministic via a shared fake `RateLimitService`) + `0033-http-ratelimit-boot-reject` (boot-reject).

**4 architectural primitives that make this work:**

1. **NEW `internal/filter/http/ratelimit/` package** owning the filter + descriptor-action engine. Package directory + Go-package identifier are both `ratelimit` (the canonical extension is `envoy.filters.http.ratelimit`, single-word; distinct from phase-11's `localratelimit` per `envoy.filters.http.local_ratelimit`). ~9–12 production Go files + ~7 test files per §6.10. Exposes `TypeURL` (`"type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit"`) + `New` (the `HTTPFilterFactory`). ADR-0197 codifies the package shape + the 10-action descriptor engine + the `RateLimitClient` wrapper + the cluster-scoped stat surface + the deterministic cross-side differential strategy, line-cited against upstream `source/common/router/router_ratelimit.cc` + `source/extensions/filters/http/ratelimit/ratelimit.cc` + `source/extensions/filters/common/ratelimit/`.

2. **NEW `internal/grpcclient/ratelimit_client.go` typed wrapper** composing the existing generic `Dialer` (ADR-0158 Tier-1; the THIRD typed wrapper after `AuthClient` + `ProcessorClient`). `ShouldRateLimit` is a **unary** RPC, so the wrapper clones `AuthClient` (NOT the bidi `ProcessorClient`) verbatim: `NewRateLimitClient(d *Dialer, clusterName string, timeout time.Duration) (*RateLimitClient, error)` → `d.DialContext(...)` + `ratelimitv3.NewRateLimitServiceClient(conn)`; `(*RateLimitClient).ShouldRateLimit(ctx, *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error)` (per-call `context.WithTimeout` when `timeout > 0`); `Close() error` (`sync.Once`-guarded). The RLS stubs ship in go-control-plane v1.32.4 (`ratelimitv3.NewRateLimitServiceClient` + `RegisterRateLimitServiceServer`); no codegen. The per-request gRPC call + the OnDestroy cancellation live at the FILTER layer (a per-stream cancellable context, the ext_authz `callCtx`/`callCancel` precedent), threaded into `ShouldRateLimit(ctx, ...)`. ADR-0197 codifies it.

3. **NEW HCM route-table `rate_limits` exposure (DELTA 2 — the highest-risk delta; AMEND-9).** The route table (`internal/filter/hcm/`) parses + retains the matched Route's `RouteAction.rate_limits[]` + the VirtualHost's `rate_limits[]` (`config.route.v3.RateLimit` policy slices — **not parsed anywhere today**), compiles them, and seeds the compiled policies onto the per-stream `FilterChain` at HCM dispatch (the ADR-0165 `DownstreamRemoteAddr` set-once-by-dispatch pattern). A NEW `DecoderFilterCallbacks` accessor pair — `RouteRateLimits()` + `VirtualHostRateLimits()` (exact return type pinned at IMPL) — exposes them to the filter. **This is NOT a reuse of `RequestRouteConfig()`** (AMEND-9 REFUTES the BRAINSTORM §2.3 framing): `RequestRouteConfig()` resolves `typed_per_filter_config` keyed by FILTER NAME, whereas the route/vhost `rate_limits` are FIRST-CLASS Route/VirtualHost proto fields NOT under `typed_per_filter_config`, and the hcm `routeTable` is package-internal & unreachable from the filter package. So DELTA 2 is a genuinely new chain-seeded framework primitive akin to ADR-0196's `ResponseStatus()`. ADR-0198 codifies it. The descriptor-action INTERPRETATION (turning `RateLimit_Action` oneofs into descriptor entries) stays filter-owned — the framework surfaces raw compiled policy, the filter owns semantics.

4. **NEW cluster-scoped cross-namespace stat surface (AMEND-1 + AMEND-10).** The 4 upstream counters (`ok` / `error` / `over_limit` / `failure_mode_allowed`) anchor at the **upstream RLS cluster's** stat scope: `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` (NOT `http.<HCM_stat_prefix>.ratelimit.<stat>` — AMEND-1 REFUTES the BRAINSTORM §2.6 hypothesis). Because no existing filter writes outside its own `http.<prefix>.<filter>.*` namespace and `*Cluster` exposes no registry/scope helper, the filter self-registers these counters via `ctx.Stats.NewCounterIfAbsent("cluster." + clusterName + ".ratelimit." + [statPrefix + "."] + stat)`, building the name from `ctx.ClusterManager.Get(clusterName).Name()`. This is the SAME novel cross-namespace pattern that ext_authz's `charge_cluster_response_stats` DEFERRED (BEHAVIOR_CONTRACT §6 amendment 8: `cluster.<upstream>.ext_authz.{ok,denied,error}`); phase 24 LANDS the mechanism (the global filter's stats have NO `http.`-scope alternative — these are the filter's ONLY stats). ADR-0197 §Decision explicitly sanctions the cross-namespace write.

After phase 24, the project has the foundational external-gRPC global rate-limit filter: a both-sides per-request filter that, on `DecodeHeaders`, builds descriptors from the route/vhost policies (filtered by `stage`), and — if any descriptors were produced — issues an async `ShouldRateLimit` call, suspends iteration (the fault/ext_authz async-resume pattern), and on the response dispositions OK / OVER_LIMIT / error per §4. Observable-outcomes byte-equivalent to reference Envoy v1.37.2 on the FULL decision path (the rate-limit decision is delegated to a SHARED deterministic fake `RateLimitService` dialed by both sides — UNLIKE phase-23 admission_control's intrinsically un-matchable RNG), and stat-name byte-equivalent on the 4-counter cluster-scoped surface (per §11 D2).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §10 D1–D7 empirical pins (executed at this SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy source on GitHub at tag `v1.37.2` + the v1.32.4 go-control-plane proto bindings in the local module cache) generated the following **11 amendment-block entries** — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM. **Six are substantive REFUTATIONS** (AMEND-1 stat anchoring; AMEND-2 no runtime proto fields; AMEND-4 inert `override_option`; AMEND-6 descriptor `hits_addend` type; AMEND-9 DELTA-2 architecture; AMEND-10 cross-namespace stat write).

- **AMEND-1 (stat anchoring — REFUTES BRAINSTORM §2.6 + §5, driven by §11 D2):** Stats anchor at the **upstream RLS cluster's** stat scope, NOT the HCM stat_prefix. The roster is exactly **4 counters** — `ok`, `error`, `failure_mode_allowed`, `over_limit` — defined in the COMMON ratelimit lib `source/extensions/filters/common/ratelimit/stat_names.h:15-18,30-33` (NOT the http/ratelimit dir). The prefix is built by `createPoolStatName` (`stat_names.h:24-27`): `absl::StrCat("ratelimit", stat_prefix.empty() ? "" : StrCat(".", stat_prefix), ".", name)` → `ratelimit.<stat>` (default) or `ratelimit.<stat_prefix>.<stat>` (when the filter-config `stat_prefix` field is set), and this StatName is resolved against `cluster_->statsScope()` (`ratelimit.cc` `complete()` ~lines 213/217/220/235; `ratelimit.h:74`). The fully-qualified metric is therefore **`cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>`**. **NO `cluster_not_found` counter exists** (BRAINSTORM hypothesis refuted). No gauges/histograms. No per-descriptor/per-domain dimension (only the optional filter-config `stat_prefix`). Project stat count **110 → 114** (+4 counters; cluster-scoped).

- **AMEND-2 (no `filter_enabled`/`filter_enforced` proto fields — REFUTES BRAINSTORM §2.4 + ADR-0200 framing, driven by §11 D1):** The `RateLimit` filter-config message has **NO** `filter_enabled` / `filter_enforced` `RuntimeFractionalPercent` fields (`rate_limit.pb.go`; `[#next-free-field: 14]`). Enable/enforce are controlled by HARD-CODED runtime keys read at request time in `ratelimit.cc`: `ratelimit.http_filter_enabled` + `ratelimit.http_filter_enforcing` (both default 100%). Since envoy-go has no runtime layer (phase-20 S2 settled), the filter is ALWAYS-enabled / ALWAYS-enforcing (the static default). **There is nothing on the filter config to PARSE-REJECT for runtime keying** — the fields simply do not exist in the proto. The BRAINSTORM's "non-empty runtime key PARSE-REJECT on the filter config" framing is refuted; the only route-level runtime surface is `RateLimit.disable_key` (§5.2 + AMEND-7).

- **AMEND-3 (filter-config field roster — REFINES BRAINSTORM §1.1, driven by §11 D1):** The `RateLimit` message has **13 fields**. NEW fields not in the BRAINSTORM hypothesis: **`status_on_error`** (`type.v3.HttpStatus`, default **500**; clamp to 500 unless in `[100,511]` per `toRatelimitServerErrorCode`, `ratelimit.h:148-154`) + **`stat_prefix`** (string, the AMEND-1 modulator). REFINEMENTS: `request_type` is a **string** (NOT an enum), PGV `in:[internal,external,both,""]`, empty ⇒ `"both"` (`validate.go:362-367`; `ratelimit.h`). `rate_limited_status` defaults 429 and **clamps `<400` ⇒ 429** (`toErrorCode`, `ratelimit.h:140-146`). `response_headers_to_add` IS a filter-config field (repeated `HeaderValueOption`, PGV `max_items:10`) — CONFIRMED (the BRAINSTORM's "only on RLS response?" suspicion refuted). `domain` REQUIRED (`min_len 1`, `validate.go:61-70`). `stage` `lte:10` (`validate.go:72-81`). `rate_limit_service` REQUIRED (`validate.go:127-136`). `timeout` default **20ms**. `failure_mode_deny` default false ⇒ fail-OPEN. `enable_x_ratelimit_headers` enum `RateLimit_XRateLimitHeadersRFCVersion{OFF=0, DRAFT_VERSION_03=1}`, default OFF. `disable_x_envoy_ratelimited_header` default false. `rate_limited_as_resource_exhausted` default false. Full roster table in §6.1.

- **AMEND-4 (`override_option` is INERT — REFUTES BRAINSTORM §4 + §2.5, driven by §11 D3):** `RateLimitPerRoute.override_option` (`OverrideOptions{DEFAULT,OVERRIDE_POLICY,INCLUDE_POLICY,IGNORE_POLICY}`) is marked **`[#not-implemented-hide:]`** (`rate_limit.pb.go:410`) and is **NEVER read** by the C++ filter. The route-additional `rate_limits[]` precedence is NOT driven by `override_option`; it is an unconditional **early-return**: if the per-route TPFC `rate_limits` (or filter-level embedded `rate_limits`) is set, that list wins and the route-table RouteAction/VirtualHost policy + the `vh_rate_limits` switch are bypassed entirely (`ratelimit.cc` `populateRateLimitDescriptors` — Axis A). The BRAINSTORM's "route-additional `rate_limits[]` via `override_option`" is wrong. **NEW field:** `RateLimitPerRoute.domain` (field 4) — a per-route domain override (not in the hypothesis). envoy-go PARSE-ACCEPTS-but-IGNORES `override_option` (upstream-parity); see §4.3 + §6.7.

- **AMEND-5 (`vh_rate_limits` composition — RATIFIES-with-refinement, driven by §11 D3):** `vh_rate_limits` enum `RateLimitPerRoute_VhRateLimitsOptions{OVERRIDE=0(default), INCLUDE=1, IGNORE=2}` — exactly 3 values, OVERRIDE is the zero/default (no UNSPECIFIED sentinel). The cross-tier composition (when no embedded `rate_limits` per AMEND-4): the **route policy is ALWAYS walked**; the VirtualHost policy is conditionally added — `IGNORE`⇒never; `INCLUDE`⇒always; `OVERRIDE`(default)⇒only if the route has NO rate_limits. The legacy `RouteAction.include_vh_rate_limits=true` forces INCLUDE regardless of the enum (`ratelimit.cc` `initializeVirtualHostRateLimitOption`). Full decision table in §4.3.

- **AMEND-6 (descriptor `hits_addend` is `UInt64Value`; wire-ordering hazards — REFUTES §11 D6 hypothesis, driven by §11 D5+D6):** The descriptor-level `RateLimitDescriptor.hits_addend` is **`google.protobuf.UInt64Value`** (presence-tracked wrapper message), NOT a bare scalar (`extensions/common/ratelimit/v3/ratelimit.pb.go:192`); when present it OVERRIDES the request-level `RateLimitRequest.hits_addend` (bare `uint32`, field 3, default 0 sourced from filter state `envoy.ratelimit.hits_addend`). The `RateLimitResponse_RateLimit_Unit` enum is **non-monotonic** (`UNKNOWN=0,SECOND=1,MINUTE=2,HOUR=3,DAY=4,MONTH=5,YEAR=6,WEEK=7`; `rls.pb.go:88-102`), and `RateLimitResponse_RateLimit`'s field numbers are reordered (`requests_per_unit=1, unit=2, name=3`). **Consequence for the shared fake service (D5):** any hand-rolled fake `RateLimitService` MUST map by proto field NUMBER (not struct/array position), and MUST emit optional fields (`raw_body`, `dynamic_metadata`, `quota`, per-descriptor `hits_addend`) only-when-present, or the cross-side wire bytes diverge. Descriptor `entries` are appended in **action-list order** (`router_ratelimit.cc` `populateDescriptors`; the shared `descriptor.entries_` vector preserves action order 1:1).

- **AMEND-7 (route-level `disable_key` + runtime enforcement — REFINES §2.4 + ADR-0200, driven by §11 descriptor-engine pin):** The actual runtime-keyed surface is the route-level `config.route.v3.RateLimit.disable_key` (string; `route_components.pb.go:3283`, "NOT supported in `typed_per_filter_config`") + the hardcoded `ratelimit.http_filter_enabled`/`ratelimit.http_filter_enforcing` keys (AMEND-2). envoy-go honors static behavior: a route/vhost `RateLimit` policy carrying a non-empty `disable_key` PARSE-REJECTs at HCM-parse-time (forward-pointer to the Runtime/RTDS family; §5.2), and the filter is always-enforcing. The OVER_LIMIT enforcement is gated on `enforced()` in upstream (the `ratelimit.http_filter_enforcing` key + the per-descriptor status); envoy-go is unconditionally enforcing (the 100% static default). NOTE: `RateLimit.stage` is a `UInt32Value` wrapper (default 0, range 0-10; `route_components.pb.go:3275`), and the `RateLimit.Override` (`limit`) oneof has only ONE arm in v1.32.4 — `dynamic_metadata` (`RateLimit_Override_DynamicMetadata`); there is NO static-value override (§4.2 + §5.2).

- **AMEND-8 (X-RateLimit headers + OVER_LIMIT/error reply byte-pin — RATIFIES-with-refinement, driven by §11 D4):** OVER_LIMIT reply: status `rate_limited_status` (default **429**); body = the RLS `raw_body` (EMPTY by default — NOT a hardcoded string); `response_code_details = "request_rate_limited"` (`ratelimit.cc:45`). `x-envoy-ratelimited: true` added on the OverLimit case (suppressed by `disable_x_envoy_ratelimited_header`; `headers.h:184,296-298`), set BEFORE the `enforced()` gate. Both the RLS-response `response_headers_to_add` AND the filter-config `response_headers_to_add` are applied (descriptor-first, then filter-config; `ratelimit.cc:278-283`). The DRAFT_VERSION_03 X-RateLimit headers (`x-ratelimit-limit`, `x-ratelimit-remaining`, `x-ratelimit-reset`; `ratelimit_headers.h:15-22`) are emitted on **ALL dispositions** (OK / Error / OverLimit) when `enable_x_ratelimit_headers == DRAFT_VERSION_03`, driven by the **minimum `limit_remaining`** descriptor-status; `x-ratelimit-limit` carries a quota-policy suffix `, <rpu>;w=<window_sec>[;name="<n>"]` per descriptor (`ratelimit_headers.cc:13-65`). `rate_limited_as_resource_exhausted=true` ⇒ gRPC status **8 (RESOURCE_EXHAUSTED)**, else **14 (UNAVAILABLE)** for gRPC requests. Error/fail-closed reply uses status `status_on_error` (default 500), rc_details `"rate_limiter_error"` (`ratelimit.cc:47`), and a **nullptr mutate-callback** ⇒ NO x-ratelimit / config headers on the 500.

- **AMEND-9 (DELTA-2 architecture — REFUTES BRAINSTORM §2.3 + §3 DELTA-2 framing, driven by §11 reuse survey):** `RequestRouteConfig()` (`internal/filter/http/callbacks.go:40`; impl `chain.go:622`) resolves `typed_per_filter_config` keyed by FILTER NAME via `perRoute.Resolve(...)` — it CANNOT carry `RouteAction.rate_limits` / `VirtualHost.rate_limits`, which are first-class Route/VirtualHost proto fields. The hcm `routeTable` (`route.go:80` `type routeTable struct{ routes []routeEntry }`) is package-internal and unreachable from `internal/filter/http/...`. So DELTA 2 is NOT "mirror `RequestRouteConfig`" (BRAINSTORM §2.3 wording refuted) — it is a genuinely NEW chain-seeded framework primitive: parse `rate_limits[]` in `buildRouteTable`/vhost-parse (`config.go:221,379`), retain the compiled policies on `routeEntry` + the vhost, seed them onto the per-stream `FilterChain` at HCM dispatch (the ADR-0165 `DownstreamRemoteAddr` set-once-by-dispatch precedent; `SetRequestCtx`-style seeding), and expose via a NEW `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair on `DecoderFilterCallbacks`. Risk: HIGH (the single most novel surface in phase 24; comparable to the phase-23 ADR-0196 root-cause primitive). ADR-0198 codifies it.

- **AMEND-10 (cluster-scoped cross-namespace stat write — REFINES §2.6 + folds in the ext_authz precedent, driven by §11 D2 + reuse survey):** Pairs with AMEND-1. `internal/cluster.Cluster` exposes NO registry/scope/registration helper (`cluster.go`: only typed `IncUpstreamRqTotal()`/`IncStatusClass()` over its OWN fixed counters + `Name()`), and NO existing filter writes outside `http.<prefix>.<filter>.*`. To emit the cluster-scoped surface (AMEND-1) the filter self-registers via `ctx.Stats.NewCounterIfAbsent("cluster." + clusterName + ".ratelimit." + ...)` (idempotent across multiple listeners sharing one RLS cluster; `registry.go:157`). This is the SAME cross-namespace pattern ext_authz's `charge_cluster_response_stats` DEFERRED (BEHAVIOR_CONTRACT §6 amendment 8). Phase 24 LANDS it (the global filter's stats have NO `http.`-scope home). ADR-0197 §Decision explicitly authorizes the write and notes the cluster package offers no helper.

- **AMEND-11 (action roster + descriptor-key defaults — RATIFIES-with-refinement, driven by §11 descriptor-engine pin):** The `RateLimit_Action` oneof has exactly **12 arms**: the 10 canonical (`source_cluster`, `destination_cluster`, `request_headers`, `query_parameters`, `remote_address`, `generic_key`, `header_value_match`, `metadata`, `masked_remote_address`, `query_parameter_value_match`) + `dynamic_metadata` (field 7, **explicitly DEPRECATED in-proto** `route_components.pb.go:5777-5780`) + `extension` (field 9, `TypedExtensionConfig` for category `envoy.rate_limit_descriptors`). No other arms. Descriptor-key defaults (REFINES the BRAINSTORM): `generic_key`→`"generic_key"`, `header_value_match`→`"header_match"`, `query_parameters`→**`"query_param"`** (singular, NOT `"query_match"`), `query_parameter_value_match`→`"query_match"`; `request_headers` + `metadata` have NO default (`descriptor_key` required from config). `header_value_match`/`query_parameter_value_match` `expect_match` default **true**. The empty-action-drop discipline has TWO behaviors (an action returning `false` drops the WHOLE descriptor + breaks the action loop; an empty-key entry is skipped but the descriptor survives — `router_ratelimit.cc:21-39`). Full per-action table in §4.1.

### 1.2 Post-empirical LoC envelope + split-readiness (D7 planning anchor)

| Surface | Anticipated LoC (prod, excl. tests) |
|---|---|
| `internal/filter/http/ratelimit/` package (config + descriptor engine over 10 actions + decode/encode + dispositions + stats + headers) | ~1100–1500 |
| `internal/grpcclient/ratelimit_client.go` (DELTA 1) | ~60–90 |
| HCM route-table `rate_limits` parse/retain/seed + the `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair (DELTA 2) | ~250–400 |
| Differential fixtures `0032` + `0033` + the `test/helpers/ratelimitgrpc/` fake service + the `fixture.HTTPGlobalRateLimitGRPC` driver | ~400–650 |
| Boot-registration insertion (`cmd/envoy-go/main.go`) | ~1–3 |
| **GRAND TOTAL (prod + fixtures, excl. tests)** | **~1900–2700 LoC** |

This is **above the ADR-0045 split-gate** (`PLAN.md > ~25 tasks OR > ~1500 LoC estimated`). Per Q1 (BRAINSTORM §2.1) the SPEC is written full-surface single-phase; the **PLAN author applies the split-gate** (lifecycle-state 2 → 3 GATE). Split-readiness is **HIGH/CONFIRMED**. The candidate 24.1/24.2 axis is recorded in §16 as a planning anchor (it is NOT a SPEC-time split — the SPEC carries the whole surface so the PLAN author can carve at the empirically-confirmed seams).

---

## 2. Non-purposes

Phase 24 lands `envoy.filters.http.ratelimit` under the existing 07.1 framework + the TWO framework deltas (§3). It does NOT extend any other subsystem beyond the minimum needed.

- **2.1 RTDS / runtime keying OUT OF SCOPE + PARSE-REJECT (route-level `disable_key`) / honor-as-static (hardcoded enable/enforce keys).** Per BRAINSTORM §2.4 (Q1) + ADR-0200 + AMEND-2 + AMEND-7. A route/vhost `RateLimit` policy carrying a non-empty `disable_key` triggers HCM-parse-time PARSE-REJECT (forward-pointer to the Runtime/RTDS family). The hardcoded `ratelimit.http_filter_enabled`/`ratelimit.http_filter_enforcing` runtime keys are honored at their 100% static default (always-enabled / always-enforcing). There are NO `filter_enabled`/`filter_enforced` proto fields to reject (AMEND-2). The `disable_key` PARSE-REJECT is an envoy-go-strict departure (upstream consults the runtime layer); unit-tested + BEHAVIOR_CONTRACT-recorded (§13), NOT a differential boot-reject (upstream ACCEPTS `disable_key`). Closes after the Runtime/RTDS family phase.
- **2.2 `extension` + deprecated `dynamic_metadata` descriptor actions OUT OF SCOPE + PARSE-REJECT.** Per BRAINSTORM §2.2 (Q2) + ADR-0200 + AMEND-11. The `extension` action (`TypedExtensionConfig`, category `envoy.rate_limit_descriptors`) needs a descriptor-producer extension-point sub-framework with no second consumer (EXTRACT-NOW-only-when-≥2 discipline); the `dynamic_metadata` action is deprecated in-proto (superseded by `metadata`). Both PARSE-REJECT at HCM-parse-time with byte-stable wording + a forward-pointer ADR. envoy-go-strict departures (upstream accepts both); unit-tested + BEHAVIOR_CONTRACT-recorded, NOT differential boot-rejects (§5.2 + §13).
- **2.3 `RateLimit.Override` (the route-policy `limit` field) + the request-level filter-state `hits_addend` source OUT OF SCOPE (honor-as-absent).** Per AMEND-7. The `limit` override's only v1.32.4 oneof arm is `dynamic_metadata` (needs dynamic-metadata-keyed override resolution — defer); the request-level `hits_addend` is sourced from a filter-state object (`envoy.ratelimit.hits_addend`) that envoy-go does not populate ⇒ default 0. Both honored-as-absent (no override; `hits_addend = 0`). Forward-pointer (§8).
- **2.4 Descriptor-value FORMATTER (command-substitution) syntax OUT OF SCOPE (literal-only).** Per §11 descriptor-engine open uncertainty. Upstream `generic_key`/`header_value_match`/`query_parameter_value_match` choose a formatter ctor when `descriptor_value` contains `%...%` substitutions. envoy-go MVP treats `descriptor_value` as a LITERAL (no formatter); a config carrying `%...%` substitution is honored verbatim as a literal string (matching upstream's non-formatter path when no substitution is present). Forward-pointer (§8).
- **2.5 Upstream-cluster `cx_*` family NOT re-emitted by this filter.** The RLS cluster's connection stats (`cluster.<name>.upstream_cx_*`) are already emitted by `internal/cluster` for any cluster (REUSE 1); phase 24 adds only the 4 `ratelimit` counters under the same cluster scope (§2.6). No new gauges.
- **2.6 Extra-upstream observability gauges OUT OF SCOPE.** Per BRAINSTORM §2.6 + AMEND-1. Upstream publishes ONLY the 4 counters; the mission is byte-exact conformance, so envoy-go publishes the same 4 and NO extra gauge.
- **2.7 Multi-worker / cross-thread stat aggregation NOT modeled (single-instance parity).** envoy-go's per-HCM-instance filter model owns the gRPC client per filter instance; the cluster-scoped counters aggregate across instances via the shared `*stats.Registry` (`NewCounterIfAbsent` idempotency). No cross-worker semantics modeled.
- **2.8 NEVER-DEFERRED — Runtime feature-flag layer.** envoy-go has no runtime-features layer (phase-20 S2 settled). The two hardcoded runtime keys (AMEND-2) are consumed at their static 100% default; the route `disable_key` PARSE-REJECTs (§2.1 + ADR-0200).
- **2.9 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed. ADR-0188 `internal/lua/` NOT consumed. ADR-0190 `internal/dynamicmetadata/` NOT consumed (the `metadata` action reads dynamic metadata directly via the existing stream-info accessor, NOT the lua dynamic-metadata primitive — confirm at IMPL). ADR-0177 `internal/httpclient/` NOT consumed (RLS is gRPC, not HTTP). ADR-0178 `internal/sdsfile/` NOT consumed. ADR-0059 `*stats.Gauge` NOT consumed (counters only). The phase-23 ADR-0196 `ResponseStatus()` is NOT consumed (the X-RateLimit headers are added at encode time but keyed off the stored RLS descriptor-statuses, not the response status).

---

## 3. Framework survey result (TWO NEW deltas + 9 REUSES + ZERO IN-PLACE §Decision AMENDMENTs)

The framework survey evaluated REUSE of phase-04-through-23 primitives BEFORE proposing NEW (per the phase-16..23 discipline). Findings: phase-24 is NOT framework-lean — it introduces TWO deltas (a typed-wrapper file in the existing `internal/grpcclient/`; a route-table capability + accessor pair in `internal/filter/hcm/` + `internal/filter/http/callbacks.go`). NO new top-level `internal/` package beyond the `ratelimit` filter package.

### 3.1 DELTA 1 — `internal/grpcclient/ratelimit_client.go` `RateLimitClient` typed wrapper *(NEW file; ADR-0158 two-tier application; ADR-0197 §Decision)*

The THIRD ADR-0158 two-tier typed wrapper (after `AuthClient` + `ProcessorClient`, both in `internal/grpcclient/`). Composes the existing generic `Dialer` (`grpcclient.go:85` `New(mgr *cluster.Manager) *Dialer`; `:105` `DialContext` with the unknown-cluster + `!UseH2` gates). `ShouldRateLimit` is **unary** ⇒ clone `AuthClient` (`grpcclient.go:178` `NewAuthClient` / `:212` `Check` / `:231` `Close`), NOT the bidi `ProcessorClient`. Public surface (settled at SPEC; IMPL confirms exact signatures):

```go
// NewRateLimitClient composes the generic Dialer over the RLS cluster.
// Mirrors AuthClient (unary RPC). No Dialer API change (ADR-0158).
func NewRateLimitClient(d *Dialer, clusterName string, timeout time.Duration) (*RateLimitClient, error)

// ShouldRateLimit applies a per-call context.WithTimeout when timeout > 0,
// invokes the unary RPC, and propagates transport errors verbatim.
func (c *RateLimitClient) ShouldRateLimit(ctx context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error)

func (c *RateLimitClient) Close() error // sync.Once-guarded, idempotent
```

Stub: `ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"` → `ratelimitv3.NewRateLimitServiceClient(conn)` (v1.32.4; verified present + the server side `RegisterRateLimitServiceServer` for the fake service). The per-stream OnDestroy cancellation lives at the FILTER layer (the ext_authz `callCtx`/`callCancel` precedent, `extauthz.go:384-385`), NOT in the wrapper. The factory call-site mirrors `buildGRPCCheckFn` (`check.go:519,569-571`): `google_grpc`-arm reject; `envoy_grpc.cluster_name` non-empty; cluster-manager `Get` + `UseH2` gates; then `grpcclient.New` + `NewRateLimitClient`.

### 3.2 DELTA 2 — HCM route-table `rate_limits` exposure + the `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair *(NEW capability; AMEND-9; ADR-0198 §Decision)*

The FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to an HTTP filter. Mechanics (per AMEND-9):

1. **Parse + retain.** `internal/filter/hcm/config.go` parses `vh.GetRateLimits()` (vhost-level, at `:221`) + each route's `r.GetRoute().GetRateLimits()` (in `buildRouteTable`, `:379`); compiles them (the §4 descriptor-engine compile: bucket by `stage`, validate, PARSE-REJECT `disable_key`/`extension`/`dynamic_metadata`). Retains the compiled policies as NEW fields on `routeEntry` (`route.go:73`) + a vhost-level field on `routeTable` (`route.go:80`). **CONFIRMED: neither field is parsed today** (grep across `internal/` excl. localratelimit/tests = 0 hits).
2. **Seed onto the chain.** At HCM dispatch the matched route's + the vhost's compiled policies are seeded onto the per-stream `FilterChain` (the ADR-0165 `DownstreamRemoteAddr` set-once-by-dispatch pattern; the matched-route index is already seeded via `SetRequestCtx`). The hcm `routeTable` is package-internal, so the COMPILED policies (a filter-package-importable type, e.g. `[]ratelimit.CompiledPolicy` or an opaque `proto.Message` slice — exact type pinned at IMPL) cross the package boundary via the chain seed.
3. **Expose.** NEW accessor pair on `DecoderFilterCallbacks` (`internal/filter/http/callbacks.go`): `RouteRateLimits()` + `VirtualHostRateLimits()` (return type pinned at IMPL — likely `[]*routev3.RateLimit` raw proto slices so the filter owns ALL interpretation, keeping the framework dumb per §2.3 rationale). Plumbed like the ADR-0165 `DownstreamRemoteAddr()`/`DownstreamLocalAddr()` primitives (`callbacks.go:101,111`; chain field + setter + accessor).

The descriptor-action INTERPRETATION stays filter-owned (the framework surfaces raw/compiled policy; the filter's engine §4 turns actions into descriptors). NOT a new `internal/` package.

### 3.3 Framework REUSES — 9 reuses + NOT-CONSUMED items

- **REUSE 1: `internal/cluster` + the generic `Dialer`** — cluster lookup + HTTP/2 gate + `passthrough:///` resolver + endpoint pick + TLS-at-cluster-layer, reused unchanged for the RLS cluster. The cluster's `upstream_cx_*` family is already emitted (§2.5).
- **REUSE 2: `internal/stats/` Counter support** — `Registry.NewCounterIfAbsent` (`registry.go:157`; the post-Freeze-safe idempotent path used by data-driven filter stats) for the cluster-scoped 4-counter surface (§2.6 + AMEND-10). Gauge support exists but is NOT consumed.
- **REUSE 3: HTTPRegistry boot-time registration** — `ratelimit.New` wired at `cmd/envoy-go/main.go` alphabetically **between `oauth2` and `rbac`** (`main.go:144-145`). **19 HTTP filters wired post-phase-24** (18 → 19). If the route/vhost `rate_limits` validation needs a per-route validator hook, a `RegisterPerRouteValidator` call is added near the header_mutation/oauth2/lua validators (pre-`Freeze()`).
- **REUSE 4: Per-request filter interface (decode/encode hooks) + the async-resume pattern** — decode-side descriptor build + async `ShouldRateLimit` + suspend/resume (the fault/ext_authz `StopIteration`→`ContinueDecoding` precedent); encode-side X-RateLimit header injection (a both-sides filter, joining the encoder cohort). The OnDestroy cancellation is the ext_authz `callCancel` precedent.
- **REUSE 5: HCM-parse-time PARSE-REJECT path** — adds the ratelimit parse arms (§5): empty `domain`; missing `rate_limit_service`; `extension`/`dynamic_metadata` actions; non-empty route `disable_key`; `stage > 10`; `request_type` not in the allowed set; malformed action sub-messages. Byte-stable wording per ADR-0080.
- **REUSE 6: existing `typed_per_filter_config` 3-tier resolver** — `RateLimitPerRoute` is a TPFC message resolved via `RequestRouteConfig()`/`Resolve` (`chain.go:622`); the route/vhost `rate_limits` are the SEPARATE non-TPFC route-table data from DELTA 2 (§3.2). The two paths are independent (Axis A precedence per §4.3).
- **REUSE 7: existing differential-fixture framework + the ext_authz fake-gRPC-service test precedent** — fixture `0021-http-ext-authz-grpc` is the template (a fixed pre-allocated port shared via `host.docker.internal`/`127.0.0.1` templating + an in-process `test/helpers/extauthzgrpc/` server with a `NewAtAddr` driver arm). Phase 24 adds `test/helpers/ratelimitgrpc/` + a `fixture.HTTPGlobalRateLimitGRPC` BackendKind + a fixture-local `driver.go` following the allocate-port → render-both-YAMLs → `NewAtAddr` → `Script`-per-scenario → `Stop` pattern (§7).
- **REUSE 8: existing fuzzer-corpus framework** — `FuzzRateLimitConfigParse` as the 33rd project-wide fuzzer (32 → 33).
- **REUSE 9: `SendLocalReply` / `x-envoy-ratelimited` local-reply primitive** — the OVER_LIMIT 429 path reuses `SendLocalReply(status int, body string, headers OrderedHeaders)` (`callbacks.go:34`; the localratelimit 429 precedent `local_ratelimit.go:370`). X-RateLimit + `x-envoy-ratelimited` + RLS/config `response_headers_to_add` are appended to the `OrderedHeaders` in the AMEND-8 order.

NOT-CONSUMED (cross-phase audit clarity): ADR-0144 `DownstreamPrincipal()`; ADR-0059 `*stats.Gauge`; ADR-0188 `internal/lua/`; ADR-0190 `internal/dynamicmetadata/`; ADR-0177 `internal/httpclient/`; ADR-0178 `internal/sdsfile/`; ADR-0196 `ResponseStatus()`.

### 3.4 Total framework footprint table

| Surface | Items | Anticipated LoC |
|---|---|---|
| NEW `internal/filter/http/ratelimit/` package | ~9–12 production + ~7 test Go files per §6.10 | ~1100–1500 (prod) |
| DELTA 1 `internal/grpcclient/ratelimit_client.go` | 1 file + test | ~60–90 |
| DELTA 2 HCM route-table + accessor pair | `hcm/config.go` + `hcm/route.go` + `hcm/chain` seed + `http/callbacks.go` + `http/chain.go` | ~250–400 |
| Boot-registration insertion | `cmd/envoy-go/main.go` alphabetical | ~1–3 |
| Differential fixtures `0032` + `0033` + `test/helpers/ratelimitgrpc/` + driver | 2 dirs + 1 helper pkg | ~400–650 |
| **GRAND TOTAL phase 24 (prod + fixtures, excl. tests)** | | **~1900–2700 LoC** |

**Above the ADR-0045 split-gate** — split disposition DEFERRED to PLAN time per Q1 (§1.2 + §16).

---

## 4. Descriptor-action engine + invariants (`router_ratelimit.cc` + `ratelimit.cc` line-cited per the §11 descriptor-engine pin)

This section codifies the descriptor-action engine with line-exact citations against upstream `source/common/router/router_ratelimit.cc` + `source/extensions/filters/http/ratelimit/ratelimit.cc` at v1.37.2.

### 4.1 The 10 canonical actions + per-action descriptor production

Each route/vhost `RateLimit` policy has an ordered `actions[]`. For each policy, the engine builds ONE descriptor by applying every action in order, appending `{key, value}` entries. The per-action behavior (`router_ratelimit.cc`):

| action | entry key | value source | can drop the WHOLE descriptor? | citation |
|---|---|---|---|---|
| `source_cluster` | literal `"source_cluster"` | the filter's `local_service_cluster` (node service-cluster name) | NO (always true) | `rl.cc:89-90` |
| `destination_cluster` | literal `"destination_cluster"` | `routeEntry()->clusterName()` | YES (false if no route/routeEntry) | `rl.cc:96-100` |
| `request_headers` | configurable `descriptor_key` (REQUIRED, no default) | first matching header value | conditional: header absent ⇒ `skip_if_absent ? skip-entry : drop-descriptor` | `rl.cc:113-117` |
| `remote_address` | literal `"remote_address"` | downstream remote IP `addressAsString()` | YES (false if not an IP) | `rl.cc:126-131` |
| `masked_remote_address` | literal `"masked_remote_address"` | CIDR-masked remote IP (`v4/v6_prefix_mask_len`) | YES (false if not an IP) | `rl.cc:141,154-156` |
| `generic_key` | configurable `descriptor_key`, default `"generic_key"` | static `descriptor_value` (or `default_value`) | YES (false if value empty AND no `default_value`) | `rl.cc:163,166-183` |
| `header_value_match` | configurable `descriptor_key`, default `"header_match"` | `descriptor_value` (or `default_value`) | YES (false if `expect_match` [default true] ≠ headers-match) | `rl.cc:261,267-289` |
| `metadata` | configurable `descriptor_key` (REQUIRED, no default) | metadata lookup at `metadata_key` from `source` (DYNAMIC=0 / ROUTE_ENTRY=1), else `default_value` | conditional: absent & no default ⇒ `skip_if_absent ? skip-entry : drop-descriptor` | `rl.cc:187-227` |
| `query_parameters` | configurable `descriptor_key`, default **`"query_param"`** | first value of `query_param_name` | conditional: param absent ⇒ `skip_if_absent ? skip : drop` | `rl.cc:232-253` |
| `query_parameter_value_match` | configurable `descriptor_key`, default `"query_match"` | `descriptor_value` (or `default_value`) | YES (false if `expect_match` [default true] ≠ query-match) | `rl.cc:297,304-328` |

PARSE-REJECTED arms (§5.2): `dynamic_metadata` (deprecated; field 7) + `extension` (field 9). The `metadata` action's value-extraction path (`Metadata::metadataValue` over `metadata_key` segments; DYNAMIC ⇒ `streamInfo().dynamicMetadata()`, ROUTE_ENTRY ⇒ route metadata) is confirmed at IMPL against the existing stream-info dynamic-metadata accessor (§12).

### 4.2 The route/vhost `RateLimit` policy message (`config.route.v3.RateLimit`)

| field | proto type | default | semantics | citation |
|---|---|---|---|---|
| `stage` | `UInt32Value` (wrapped) | 0 (range 0-10) | policy applies only to a filter at the same stage (§4.4) | `route_components.pb.go:3275` |
| `disable_key` | string | `""` | runtime disable key — **PARSE-REJECT if non-empty** (§5.2 + AMEND-7) | `:3283` |
| `actions` | repeated `RateLimit.Action` | — | ordered; the §4.1 engine | `:3290` |
| `limit` | `RateLimit.Override` | nil | only v1.32.4 arm = `dynamic_metadata` — **honor-as-absent** (§2.3 + AMEND-7) | `:3301`; `:5835-5890` |
| `hits_addend` | `RateLimit.HitsAddend` | nil | request-level addend override — out of scope (§2.3) | `:3389` |
| `apply_on_stream_done` | bool | false | charge on stream completion — out of scope (deferred §8) | `:3396` |

### 4.3 Cross-tier composition — two axes (`ratelimit.cc` `populateRateLimitDescriptors`)

**Axis A (embedded-config precedence; AMEND-4):** if the per-route TPFC `RateLimitPerRoute.rate_limits` (or the filter-level embedded `rate_limits`) is non-empty, walk ONLY that list and RETURN — the route-table RouteAction/VirtualHost policy + the `vh_rate_limits` switch are bypassed. `override_option` is INERT (never read).

**Axis B (route-table tier; AMEND-5):** when no embedded `rate_limits`, the **route policy is ALWAYS walked**; the VirtualHost policy is conditionally added:

| `vh_rate_limits` | route has `rate_limits` | VH `rate_limits` walked? | route `rate_limits` walked? |
|---|---|---|---|
| `OVERRIDE` (0, default) | yes (non-empty) | NO | yes |
| `OVERRIDE` (0, default) | no (empty) | YES | yes (empty no-op) |
| `INCLUDE` (1) | any | YES (both tiers) | yes |
| `IGNORE` (2) | any | NO | yes |

Legacy override: `RouteAction.include_vh_rate_limits=true` forces `INCLUDE` regardless of the enum (`initializeVirtualHostRateLimitOption`).

### 4.4 Stage filtering

Each `RateLimit` policy carries `stage` (0-10). At config build, policies are bucketed by stage (upstream `references[stage]`, sized `MAX_STAGE_NUMBER+1 = 11`; `rl.cc:539-550`). At request time only policies whose `stage` equals the **filter's configured `stage`** (filter-config field; default 0) are evaluated (`getApplicableRateLimit(stage)`). envoy-go mirrors: the engine pre-buckets the compiled route + vhost policies by stage at parse time and selects the filter-stage bucket per request.

### 4.5 Empty-action-drop discipline (`router_ratelimit.cc:21-39`)

TWO behaviors: **(1)** an action whose `populateDescriptor` returns `false` sets `result=false`, **breaks** the action loop, and the WHOLE descriptor is discarded (`if (result) descriptors.emplace_back(descriptor)`; `rl.cc:483-485`); **(2)** an action returning true but producing an EMPTY key has its entry **skipped** (`if (!key.empty()) push_back`; `:34-36`) while the descriptor survives. The engine MUST honor both.

### 4.6 Request build + dispatch + dispositions (`ratelimit.cc`)

On `DecodeHeaders`: build descriptors (§4.1–§4.5); if ZERO descriptors ⇒ continue (no RLS call). Else build `RateLimitRequest{domain (per-route domain override wins, else filter `domain`), descriptors, hits_addend=0}` and issue async `ShouldRateLimit` (`StopIteration`, the async-resume pattern). On response: **OK** ⇒ `ok_.inc()`, apply RLS `request_headers_to_add`, continue; **OVER_LIMIT** ⇒ `over_limit_.inc()`, `SendLocalReply` per §4.7; **error/timeout** ⇒ `error_.inc()`, and `failure_mode_deny ? (SendLocalReply(status_on_error=500, rc_details="rate_limiter_error", nullptr-mutate)) : (failure_mode_allowed_.inc(), apply RLS request_headers_to_add, continue)` (fail-open default). X-RateLimit headers (if DRAFT_VERSION_03) are stored from the response descriptor-statuses and applied at encode time on ALL dispositions (§4.7 + AMEND-8).

### 4.7 OVER_LIMIT + error reply byte-pin (AMEND-8 + §11 D4)

- **OVER_LIMIT:** status = `rate_limited_status` (default 429; clamp `<400` ⇒ 429); body = RLS `raw_body` (empty default); `response_code_details = "request_rate_limited"`; headers (in order): RLS-response `response_headers_to_add`, then `x-envoy-ratelimited: true` (unless `disable_x_envoy_ratelimited_header`), then X-RateLimit-* (if enabled), then filter-config `response_headers_to_add`. gRPC status (gRPC requests): `rate_limited_as_resource_exhausted ? 8 (RESOURCE_EXHAUSTED) : 14 (UNAVAILABLE)`.
- **error (fail-closed):** status = `status_on_error` (default 500); `response_code_details = "rate_limiter_error"`; **no x-ratelimit / config headers** (nullptr mutate-callback).
- **X-RateLimit (DRAFT_VERSION_03), emitted on OK/Error/OverLimit:** `x-ratelimit-limit: <min-status.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` (driven by the MIN `limit_remaining` descriptor-status; quota-policy suffix per descriptor with non-zero window); `x-ratelimit-remaining: <min-status.limit_remaining>`; `x-ratelimit-reset: <min-status.duration_until_reset.seconds>`. Unit→seconds: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000, UNKNOWN/0 ⇒ no quota-policy segment for that descriptor.

---

## 5. PARSE-REJECT roster (HCM-parse-time) + boot-reject + per-route

### 5.1 RATIFIED-from-PGV / config-validation arms (upstream rejects too)

| Arm | upstream rule | envoy-go-error wording (byte-stable per ADR-0080; finalized at IMPL) |
|---|---|---|
| `domain` empty | PGV `min_len 1` (`validate.go:61-70`) + C++ `ASSERT(!domain().empty())` | `"ratelimit: domain is required"` |
| `rate_limit_service` absent | PGV `(message).required` (`validate.go:127-136`) | `"ratelimit: rate_limit_service is required"` |
| `stage > 10` | PGV `lte 10` (`validate.go:72-81`) | `"ratelimit: stage must be <= 10"` |
| `request_type` not in `{internal,external,both,""}` | PGV `in` (`validate.go:362-367`) | `"ratelimit: request_type must be one of internal|external|both"` |
| `rate_limit_service.grpc_service` not `envoy_grpc` / unknown cluster / non-HTTP2 cluster | the `grpcclient.DialContext` gates (`grpcclient.go:124-137`) | the existing cluster-load wording (REUSE 1; ext_authz `buildGRPCCheckFn` precedent) |
| `response_headers_to_add` > 10 entries | PGV `max_items 10` | `"ratelimit: response_headers_to_add accepts at most 10 items"` |

The boot-reject differential fixture (§7.2) exercises the **`domain` empty** arm (cleanest single-field shared reject; distinctive substring). Exact wording finalized at IMPL + asserted by a `TestParseRejectConstants_ByteStable` table.

### 5.2 envoy-go-strict project-local arms (stricter than upstream)

| Arm | envoy-go behavior | wording (finalized at IMPL) | ADR anchor |
|---|---|---|---|
| route/vhost `RateLimit.disable_key != ""` | PARSE-REJECT (defers route-level runtime disable) | `"ratelimit: rate_limits[].disable_key is not yet supported (runtime keying deferred)"` | ADR-0200 |
| `RateLimit_Action.extension` arm set | PARSE-REJECT (defers the descriptor-producer extension-point) | `"ratelimit: the 'extension' descriptor action is not yet supported"` | ADR-0200 |
| `RateLimit_Action.dynamic_metadata` arm set | PARSE-REJECT (deprecated; use `metadata`) | `"ratelimit: the deprecated 'dynamic_metadata' descriptor action is not supported; use 'metadata'"` | ADR-0200 |
| `RateLimit.limit` (Override) set | honor-as-absent (no override) — NOT a reject (upstream-parity for the unsupported dynamic_metadata override is a no-op override) | (no error) | §2.3 |

These are stricter than upstream (which accepts them); the deferrals are operator-visible via byte-stable wording + BEHAVIOR_CONTRACT records (§13). NOT differential boot-reject candidates (upstream ACCEPTS them — a matching reject diverges by design). Anticipated departure-record count **15 → 18** (the 3 PARSE-REJECT arms; SPEC notes some may consolidate into one descriptor-deferral record at IMPL).

### 5.3 Per-route — NEW 10th canonical (`RateLimitPerRoute`) + ADR-0125 amendment 9 → 10

`envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute` (a TPFC message resolved via `RequestRouteConfig()`) carries (per §11 D3): `vh_rate_limits` (`VhRateLimitsOptions{OVERRIDE=0,INCLUDE=1,IGNORE=2}`, field 1, default OVERRIDE), `override_option` (`OverrideOptions{DEFAULT,OVERRIDE_POLICY,INCLUDE_POLICY,IGNORE_POLICY}`, field 2, **`[#not-implemented-hide:]` — INERT per AMEND-4**), `rate_limits[]` (`[]config.route.v3.RateLimit`, field 3, the Axis-A embedded-policy list), `domain` (string, field 4, per-route domain override per AMEND-4). Classification: **NEW 10th canonical** ("data-only-with-vh-inclusion-enum" — the `vh_rate_limits` enum drives cross-tier descriptor composition, a per-route semantic absent from the 9 existing canonicals). **ADR-0125 roster amendment 9 → 10** (anchored in ADR-0199). RE-AMENDS after phase-23's REUSE-by-absence skip. envoy-go PARSE-ACCEPTS-but-IGNORES `override_option` (upstream-parity); honors `vh_rate_limits` (§4.3) + `rate_limits` (Axis A) + `domain`.

---

## 6. compiledConfig + code shapes (IMPL blueprint)

### 6.1 `compiledConfig` shape (filter config; per §11 D1 + AMEND-3)

```go
type compiledConfig struct {
    domain                        string        // REQUIRED (non-empty)
    stage                         uint32        // 0-10; selects the route/vhost policy stage bucket
    requestType                   requestType   // internal | external | both (default both)
    timeout                       time.Duration // default 20ms
    failureModeDeny               bool          // default false ⇒ fail-open
    rateLimitedAsResourceExhausted bool         // gRPC 8 vs 14 on OVER_LIMIT
    rlsClusterName                string        // from rate_limit_service.grpc_service.envoy_grpc.cluster_name
    enableXRateLimitHeaders       bool          // true iff enable_x_ratelimit_headers == DRAFT_VERSION_03 (faithful at the v1.32.4 pin — only OFF/DRAFT_VERSION_03 exist; IMPL note: promote to the enum if upstream adds a DRAFT_VERSION_NN)
    disableXEnvoyRateLimitedHeader bool
    rateLimitedStatus             int           // default 429; <400 ⇒ 429
    statusOnError                 int           // default 500; clamp to [100,511] else 500
    statPrefix                    string        // AMEND-1 stat modulator
    responseHeadersToAdd          []headerKV    // filter-config headers (max 10)
    // NOTE: embedded filter-level rate_limits[] (Axis A) compiled here if present
}
```

`buildCompiledConfig(typedConfig *anypb.Any, ctx FactoryCtx) (*compiledConfig, error)` performs the §5.1 + §5.2 PARSE-REJECT roster (byte-stable per ADR-0080), the cluster-load gates (REUSE 1), and the AMEND-3 defaults/clamps.

### 6.2 `RateLimitClient` (DELTA 1) — per §3.1.

### 6.3 Route-table `rate_limits` exposure (DELTA 2) — per §3.2. The compiled route/vhost policies are a filter-importable type seeded onto the chain; `RouteRateLimits()` + `VirtualHostRateLimits()` return them.

### 6.4 Descriptor engine (`descriptors.go`) — the §4.1 per-action interpreter + §4.3 cross-tier composition + §4.4 stage filter + §4.5 empty-action-drop. Produces `[]*ratelimitv3.RateLimitDescriptor` (entries in action order; AMEND-6 proto-number-faithful).

### 6.5 `decode_headers.go` — build descriptors; zero ⇒ continue; else async `ShouldRateLimit` + `StopIteration` (§4.6). OnDestroy cancels the per-stream context.

### 6.6 `encode.go` — X-RateLimit header injection on all dispositions when enabled (§4.7), keyed off the stored response descriptor-statuses.

### 6.7 `compiled_perroute.go` — `RateLimitPerRoute` TPFC compile (§5.3); `vh_rate_limits` honored, `override_option` accepted-but-ignored, `rate_limits` as Axis A, `domain` override.

### 6.8 `stats.go` — cluster-scoped 4-counter surface (AMEND-1 + AMEND-10):

```go
func newFilterStats(reg *stats.Registry, clusterName, statPrefix string) *filterStats {
    base := "cluster." + clusterName + ".ratelimit."
    if statPrefix != "" { base += statPrefix + "." }
    return &filterStats{
        ok:                 reg.NewCounterIfAbsent(base + "ok"),
        error:              reg.NewCounterIfAbsent(base + "error"),
        overLimit:          reg.NewCounterIfAbsent(base + "over_limit"),
        failureModeAllowed: reg.NewCounterIfAbsent(base + "failure_mode_allowed"),
    }
}
```

`NewCounterIfAbsent` (idempotent; safe across multiple listeners sharing one RLS cluster). Project stat count **110 → 114**.

### 6.9 `fuzz_test.go` — 33rd fuzzer `FuzzRateLimitConfigParse` (~50 LoC). Corpus ~30 seeds: a valid full config; each §5.1 + §5.2 PARSE-REJECT arm (empty domain; missing rate_limit_service; stage>10; bad request_type; disable_key; extension; dynamic_metadata; >10 response headers); embedded vs route/vhost rate_limits; each action type; empty config. Must-never-panic across `buildCompiledConfig` + the descriptor-engine compile.

### 6.10 Source-file roster (~9–12 production + ~7 test Go files)

| File | Purpose | Anticipated LoC |
|---|---|---|
| `ratelimit/doc.go` | package doc | ~25 |
| `ratelimit/ratelimit.go` | `TypeURL` + `New` factory + filter value | ~120–180 |
| `ratelimit/compiled_config.go` | `compiledConfig` + `buildCompiledConfig` + §5 PARSE-REJECT roster | ~250–350 |
| `ratelimit/descriptors.go` | the §4 descriptor-action engine (10 actions + composition + stage + drop) | ~350–500 |
| `ratelimit/compiled_perroute.go` | `RateLimitPerRoute` TPFC (§6.7) | ~80–120 |
| `ratelimit/decode_headers.go` | descriptor build + async dispatch (§6.5) | ~120–180 |
| `ratelimit/encode.go` | X-RateLimit header injection (§6.6) | ~80–140 |
| `ratelimit/dispositions.go` | OK/OVER_LIMIT/error + local-reply byte-shape (§4.6 + §4.7) | ~120–180 |
| `ratelimit/headers.go` | DRAFT_VERSION_03 header construction (§4.7) | ~80–120 |
| `ratelimit/stats.go` | cluster-scoped `filterStats` (§6.8) | ~40–60 |
| `internal/grpcclient/ratelimit_client.go` | DELTA 1 (§3.1) | ~60–90 |
| `ratelimit/*_test.go` | descriptor-engine + PARSE-REJECT + disposition + header + perroute (§14.1 Layer A) | ~900–1300 |

---

## 7. Differential fixture envelope — two directories (`0032` + `0033`)

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch, cross-side XOR boot-reject), the cross-side + boot-reject surfaces are SEPARATE directories from the start. Fixture directory count **33 → 35** (the next free numbers after `0031` are `0032` + `0033`).

### 7.1 `0032-http-ratelimit` (cross-side, FULLY deterministic via a shared fake `RateLimitService`)

A SHARED fake gRPC `RateLimitService` (a `test/helpers/ratelimitgrpc/` server implementing `ShouldRateLimit` with a deterministic descriptor → `RateLimitResponse{overall_code, statuses}` script map) is dialed by BOTH envoy-go and reference Envoy v1.37.2 (both configs point `rate_limit_service.grpc_service.envoy_grpc.cluster_name` at a cluster whose endpoint is the fake server). Reachability mirrors fixture `0021`: a fixed pre-allocated `127.0.0.1:<port>` shared via `host.docker.internal` (reference, dockerized, STRICT_DNS per ADR-0010) / `127.0.0.1` (subject, STATIC per ADR-0002) templating; a new `fixture.HTTPGlobalRateLimitGRPC` BackendKind + a fixture-local `driver.go` (allocate-port → render-both-YAMLs → `ratelimitgrpc.NewAtAddr` → `Script`-per-scenario → `Stop`). Per AMEND-6 the fake MUST emit responses by proto field NUMBER + omit unset optionals.

| Scenario | Disposition | Wire-level expectation |
|---|---|---|
| **(a) `parse_ok`** | REFERENCE-LESS subject-only structural | filter loads with a full config (route+vhost rate_limits, several actions); admin `/stats` exposes the 4 cluster-scoped counters; HTTP 200 to a normal GET (descriptors map to OK) |
| **(b) `ok_admit`** | **CROSS-SIDE byte-exact** | a request whose descriptors map to `overall_code=OK` ⇒ admitted; status + body byte-exact cross-side; `cluster.<rls>.ratelimit.ok` increments |
| **(c) `over_limit_429`** | **CROSS-SIDE byte-exact** | descriptors map to `OVER_LIMIT` ⇒ 429 + `x-envoy-ratelimited: true` + RLS `response_headers_to_add` + filter-config `response_headers_to_add` (AMEND-8 order); byte-exact cross-side; `over_limit` increments |
| **(d) `descriptor_actions`** | **CROSS-SIDE byte-exact** | requests exercising `generic_key` / `request_headers` / `remote_address` / `header_value_match` / `query_parameters` so the fake sees the descriptors BOTH sides built (the fake asserts descriptor equality or the resulting disposition); byte-exact |
| **(e) `failure_mode_open`** | **CROSS-SIDE byte-exact** | the fake returns a gRPC error (or the cluster is briefly unreachable) ⇒ `failure_mode_deny=false` admit (fail-open); byte-exact; `error` + `failure_mode_allowed` increment |
| **(f) `vh_inclusion`** | **CROSS-SIDE byte-exact** | INCLUDE vs OVERRIDE vs IGNORE drives which tier's descriptors are sent (§4.3); byte-exact cross-side |
| **(g) `x_ratelimit_headers`** | **CROSS-SIDE byte-exact** | `enable_x_ratelimit_headers: DRAFT_VERSION_03` + the fake returns descriptor-statuses with `current_limit`/`limit_remaining`/`duration_until_reset` ⇒ `x-ratelimit-limit/-remaining/-reset` byte-exact (AMEND-8) |
| **(h) `stat_surface`** | REFERENCE-LESS subject-only structural via `StatsAsserter.AssertStats` | the 4 counters under `cluster.<rls_cluster>.ratelimit.{ok,error,over_limit,failure_mode_allowed}` with expected values after a small burst; proven live via deliberate-break (per `reference_differential_asserter_dispatch` — subject-side assertions go in `StatsAsserter`, NOT `SubjectAsserter`) |

UNLIKE phase-23 (un-matchable RNG), the rate-limit decision is DELEGATED to the shared fake, so the FULL path is cross-side byte-exact. The X-RateLimit headers must be on the response header allow-list discipline (BEHAVIOR_CONTRACT §7.2 set-equal).

### 7.2 `0033-http-ratelimit-boot-reject` (boot-reject)

A SHARED config-load reject where upstream Envoy ALSO rejects at boot: **`domain` empty** (REQUIRED) — upstream PGV/`ASSERT` rejects; envoy-go's PGV-mirror rejects with the §5.1 wording; the fixture pins the common distinctive stderr substring. NOTE: the `disable_key` / `extension` / `dynamic_metadata` rejects are NOT boot-reject candidates (upstream ACCEPTS them — §5.2); those departures are unit-tested + BEHAVIOR_CONTRACT-recorded.

### 7.3 Listener topology

Single listener with a single HCM containing the ratelimit filter (alphabetical, between oauth2 + rbac) + router terminator, plus the RLS cluster pointing at the fake gRPC server, plus a synthetic always-200 backend cluster. No multi-listener topology (avoids the `freeTCPPort` combined-run flake per 22.2 REVIEW §7.4). The fake-service port is allocated once per fixture run + shared by both sides.

### 7.4 Six-gate checklist (A/B/C/D/E/F) — identical matrix to phase-09..23

- **Gate A — build**: `go build ./...` clean (incl. `internal/filter/http/ratelimit/` + `internal/grpcclient/ratelimit_client.go` + the HCM DELTA-2 changes).
- **Gate B — vet + lint**: `go vet ./...` + `golangci-lint run` clean; no new suppressions.
- **Gate C — race**: `go test -race ./...` clean (incl. the new package + the per-stream gRPC client cancellation + the chain-seeded route-table accessor).
- **Gate D — differential**: 35/35 fixtures GREEN (0000-0031 pre-existing + 0032 + 0033 new); cross-side byte-exact on `0032` (b)/(c)/(d)/(e)/(f)/(g); boot-reject substring on `0033`.
- **Gate E — fuzz**: `FuzzRateLimitConfigParse` clean at 30s/seed; no panics across the 33 project-wide fuzzers.
- **Gate F — h2spec**: 53/53 PASS at ADR-0051 v1.32.4 pin.

---

## 8. Deferred items (11 items)

1. **RTDS / runtime keying** — route `disable_key` PARSE-REJECT + the hardcoded `ratelimit.http_filter_enabled`/`http_filter_enforcing` keys honored-as-static (§2.1 + ADR-0200 + AMEND-2/7). Closes after the Runtime/RTDS family phase.
2. **`extension` descriptor action** — PARSE-REJECT (§2.2 + ADR-0200); needs a descriptor-producer extension-point sub-framework with no second consumer. Extract when ≥2 producers land.
3. **Deprecated `dynamic_metadata` descriptor action** — PARSE-REJECT (§2.2 + ADR-0200); superseded by `metadata`. Re-open only if a real corpus needs the deprecated form.
4. **`RateLimit.Override` (`limit`) + request-level `hits_addend` filter-state source** — honor-as-absent (§2.3 + AMEND-7); the only Override arm is `dynamic_metadata` (needs dynamic-metadata-keyed override resolution); `hits_addend` needs the `envoy.ratelimit.hits_addend` filter-state object.
5. **Descriptor-value FORMATTER (`%...%`) syntax** — literal-only at MVP (§2.4); a future formatter-support phase lifts it.
6. **Route-table exposure widening** — DELTA 2 exposes `rate_limits` only; a future filter needing OTHER matched Route/VirtualHost first-class fields triggers a widening decision (forward-pointer per §3.2 + ADR-0198 §Consequences).
7. **`apply_on_stream_done`** (route `RateLimit` policy field) — charge-on-completion deferred (§4.2); not modeled at MVP.
8. **Cross-namespace cluster-stat charging widening** — phase 24 LANDS the `cluster.<name>.ratelimit.<stat>` write (AMEND-10); the ext_authz `charge_cluster_response_stats` triple (`cluster.<upstream>.ext_authz.{ok,denied,error}`, BEHAVIOR_CONTRACT §6 amendment 8) can adopt the same mechanism when ext_authz is next touched (cross-phase forward-closure).
9. **Multi-worker stat aggregation** — single-instance parity (§2.7); a future fixture-extension could exercise multi-listener aggregation.
10. **PLAN-time ADR-0045 split decision** — single-phase through SPEC; the PLAN author applies the split-gate (candidate axis §16; LoC ~1900–2700 trips the gate).
11. **gRPC `quota` caching + `dynamic_metadata` response field** — the RLS `RateLimitResponse.quota` (response-side caching) + `dynamic_metadata` emission are honored-as-absent at MVP (the fake omits them; §7.1); a future caching/metadata-emit phase lifts them.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

Phase 24 PICKS UP several closures from phase-11 local_ratelimit's deferred-items list (which explicitly "couples to `global_ratelimit` future phase"):

- **descriptor-action cluster** — phase 11 has NO descriptor support (plain token-bucket); phase 24 lands the full route/vhost descriptor-action engine (§4). CLOSURE.
- **X-RateLimit headers + vh-policy cluster** — phase 11 deferred `enable_x_ratelimit_headers` + vh-policy; phase 24 lands DRAFT_VERSION_03 headers (§4.7) + `vh_rate_limits` inclusion (§4.3). CLOSURE.
- **multi-stage cluster** — phase 11 deferred `stage`; phase 24 lands the `stage` field + descriptor stage-filtering (§4.4). CLOSURE.

These convert from "deferred — couples to global_ratelimit" to "lifted at phase 24" at phase-24 IMPL's next-touchpoint (the cross-phase deferral-lift discipline, e.g. ADR-0190 §(v)). Phase 11's `gRPC trailer` + `xDS cluster-state` + `per-connection lifecycle` clusters are NOT picked up. Phase 24 ALSO establishes the cross-namespace cluster-stat-charging mechanism that ext_authz's `charge_cluster_response_stats` deferral anticipated (§8 item 8 — partial forward-closure). The ADR-0158 grpcclient two-tier "future consumers compose their own typed wrapper" forward-pointer IS consumed (the third typed wrapper, `RateLimitClient`).

---

## 10. ADR anchor map (4 NEW §Context drafts + 1 ADR-0125 amendment; ZERO IN-PLACE AMENDMENTs; D-hypothesis)

Per ADR-0044: the ADR-0197..ADR-0200 §Context drafts anchor at this SPEC commit (appended to `DECISIONS.md`); §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL.

### A. 4 NEW ADRs (ADR-0197 .. ADR-0200)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0197** | NEW `internal/filter/http/ratelimit/` package — the 10-action descriptor engine (empty-action-drop + stage + cross-tier composition) + the NEW `internal/grpcclient` `RateLimitClient` typed wrapper (ADR-0158 third application; unary `AuthClient` clone) + the OK/OVER_LIMIT/error-with-failure-mode dispositions + the OVER_LIMIT 429 / `x-envoy-ratelimited` / X-RateLimit DRAFT_VERSION_03 byte-shape (per AMEND-8) + the cluster-scoped cross-namespace 4-counter stat surface (`cluster.<rls>.ratelimit[.<stat_prefix>].<stat>`; per AMEND-1 + AMEND-10; explicitly sanctioning the cross-namespace write) + the FULLY-DETERMINISTIC cross-side differential via a shared fake `RateLimitService` + line-cited lemmata against `router_ratelimit.cc` / `ratelimit.cc` / `ratelimit_headers.cc` / `stat_names.h` | §1; §3.1; §4; §5; §6; §7; §1.1 AMEND-1/3/5/6/8/10/11 | IMPL: filter package + engine + client materialization |
| **ADR-0198** | NEW HCM route-table `rate_limits` exposure framework capability — parse + retain + compile the matched Route's + VirtualHost's `config.route.v3.RateLimit` policy slices; seed the compiled policies onto the per-stream `FilterChain` at HCM dispatch (the ADR-0165 set-once-by-dispatch precedent); expose via a NEW `RouteRateLimits()`/`VirtualHostRateLimits()` `DecoderFilterCallbacks` accessor pair. FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to a filter; NOT a reuse of `RequestRouteConfig()` (per AMEND-9 — TPFC is keyed by filter name; `rate_limits` are first-class fields) | §1; §3.2; §1.1 AMEND-9 | IMPL: route-table parse/seed + callbacks accessor pair |
| **ADR-0199** | `RateLimitPerRoute` NEW 10th canonical per-route shape ("data-only-with-vh-inclusion-enum") + ADR-0125 roster amendment 9 → 10 — `vh_rate_limits` (OVERRIDE/INCLUDE/IGNORE) drives cross-tier descriptor composition; `override_option` accepted-but-ignored (INERT per AMEND-4); `rate_limits[]` as Axis-A embedded policy; `domain` per-route override. RE-AMENDS after phase-23's REUSE-by-absence skip | §4.3; §5.3; §1.1 AMEND-4/5 | IMPL: perroute compile + ADR-0125 amendment |
| **ADR-0200** | Runtime/RTDS + action-deferral PARSE-REJECTs — route `RateLimit.disable_key` non-empty PARSE-REJECT (forward-pointer to the Runtime/RTDS family; the hardcoded `ratelimit.http_filter_enabled`/`http_filter_enforcing` keys honored-as-static per AMEND-2/7); the `extension` action PARSE-REJECT (descriptor-producer extension-point deferral); the deprecated `dynamic_metadata` action PARSE-REJECT (superseded by `metadata`); the ~3 envoy-go-strict departure records (count 15 → 18) | §2.1; §2.2; §5.2; §1.1 AMEND-2/7/11 | IMPL: compiled_config + PARSE-REJECT roster |

### B. ZERO IN-PLACE §Decision AMENDMENTs + ONE ADR-0125 amendment

No `internal/stats/` §Decision AMENDMENT (the cross-namespace write uses the existing `NewCounterIfAbsent` API unchanged — ADR-0197 §Decision sanctions the USAGE, not an API change). No `internal/grpcclient/` ADR-0158 AMENDMENT (the `Dialer` is unchanged; `RateLimitClient` is a new composing wrapper, the documented extension pattern). **ONE ADR-0125 amendment (9 → 10)**, anchored in ADR-0199 (the FIRST roster growth since phase-22's 8 → 9).

### C. ADR-0044 escape-valve reserve + D-hypothesis

~0–2 impl-time-unanticipated ADRs per phase. Phase-24's most-likely surprise surfaces (SPEC-time partially closed): the DELTA-2 chain-seed type + accessor return-type (the highest-risk; AMEND-9 — could force a second framework ADR if the seeding shape diverges from ADR-0165's, mirroring how phase-23's PD-5 forced ADR-0196); the `metadata`-action dynamic-metadata accessor path; the X-RateLimit MIN-status quota-policy byte-edge; the descriptor `entries` proto-number wire-ordering for the fake-service byte-exactness (AMEND-6). **D-style hypothesis:** ADR-0201 stays UNCONSUMED at phase-24 phase-done. **HOLD-with-known-risk, LARGER surprise surface than phase-23** (DELTA 2 + the cross-namespace stat write + a 10th-canonical amendment). A PLAN-time 24.1/24.2 split would re-map ADR consumption across sub-phases (the split axis §16 keeps each sub-phase ADR-coherent: 24.1 = ADR-0197 core + ADR-0198 DELTA-2 + ADR-0200; 24.2 = ADR-0199 perroute + the X-RateLimit/headers slice of ADR-0197).

### D. Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts | 4 | ADR-0197; ADR-0198; ADR-0199; ADR-0200 |
| IN-PLACE §Decision AMENDMENT-anticipation | 0 | NONE |
| ADR-0125 amendments | 1 | 9 → 10 (anchored in ADR-0199) |
| ADR-0044 escape-valve reserve | 0-2 | reserved at ADR-0201+ if fired |

**Next-free ADR post-SPEC commit: `ADR-0201`** (4 NEW consumed: ADR-0197 .. ADR-0200).

---

## 11. Empirical-pin block (D1–D7 resolved at this SPEC session)

### A. Pin disposition matrix

| Pin | Disposition | Wire-level finding | ADR anchor |
|---|---|---|---|
| **D1** Filter-config roster + defaults | RATIFIED-with-REFUTATION | 13 fields; NEW `status_on_error`(500) + `stat_prefix`; `request_type` is a STRING (empty⇒both); `rate_limited_status` clamps <400⇒429; `response_headers_to_add` IS on filter config; **NO `filter_enabled`/`filter_enforced` proto fields** (hardcoded runtime keys instead — AMEND-2) | ADR-0197; ADR-0200; AMEND-2/3/7 |
| **D2** Stat roster + prefix | REFUTED-on-anchoring | 4 counters `ok`/`error`/`over_limit`/`failure_mode_allowed` (NO `cluster_not_found`); **anchored at the CLUSTER scope** `cluster.<rls_cluster>.ratelimit[.<stat_prefix>].<stat>` (NOT `http.<HCM_prefix>.` — AMEND-1); novel cross-namespace write (AMEND-10); 110 → 114 | ADR-0197; AMEND-1/10 |
| **D3** `RateLimitPerRoute` shape | RATIFIED-with-REFUTATION | `vh_rate_limits{OVERRIDE=0,INCLUDE=1,IGNORE=2}`; `override_option` INERT (`[#not-implemented-hide:]` — AMEND-4); NEW `domain` field; Axis-A embedded `rate_limits` early-return; NEW 10th canonical + ADR-0125 9→10 | ADR-0199; AMEND-4/5 |
| **D4** X-RateLimit + local-reply byte-pin | RATIFIED-with-REFINEMENT | OVER_LIMIT rc_details `"request_rate_limited"`; body=RLS `raw_body`(empty default); `x-envoy-ratelimited:true`; X-RateLimit on ALL dispositions (MIN-status; `;w=`/`;name=` quota suffix); error path nullptr-mutate; resource_exhausted⇒grpc 8 else 14 | ADR-0197; AMEND-8 |
| **D5** Fake `RateLimitService` wire contract | RATIFIED | method `/envoy.service.ratelimit.v3.RateLimitService/ShouldRateLimit`; `RateLimitResponse{overall_code(UNKNOWN=0/OK=1/OVER_LIMIT=2), statuses, response_headers_to_add, request_headers_to_add, raw_body, dynamic_metadata, quota}`; fake must emit by proto NUMBER + omit unset optionals (AMEND-6) | ADR-0197; AMEND-6 |
| **D6** `RateLimitRequest` descriptor wire shape | RATIFIED-with-REFUTATION | `RateLimitRequest{domain, descriptors[], hits_addend(uint32, default 0)}`; `RateLimitDescriptor{entries[]{key,value}, limit, hits_addend}` — **descriptor `hits_addend` is `UInt64Value` wrapper** (AMEND-6); entries in action order; Unit enum non-monotonic (WEEK=7) | ADR-0197; AMEND-6/11 |
| **D7** PLAN-time split-gate | RECORDED | LoC ~1900–2700 (above the ~1500 gate); split-readiness HIGH/CONFIRMED; candidate 24.1/24.2 axis §16 | §1.2; §16 |

### B. Pin disposition summary

| Disposition | Count |
|---|---|
| RATIFIED-with-REFUTATION | 3 (D1, D3, D6) |
| REFUTED-on-anchoring | 1 (D2) |
| RATIFIED-with-REFINEMENT | 1 (D4) |
| RATIFIED | 1 (D5) |
| RECORDED | 1 (D7) |
| **TOTAL** | **7** |

All pins CLOSED at SPEC time. §12 residual byte-confirmations are SUB-PIN-LEVEL refinements.

### C. Pin-to-AMEND-block traceability

| AMEND-N | Sources | Recipient ADRs |
|---|---|---|
| AMEND-1 (cluster-scope stats) | D2 | ADR-0197 |
| AMEND-2 (no runtime proto fields) | D1 | ADR-0200 |
| AMEND-3 (filter-config roster) | D1 | ADR-0197 |
| AMEND-4 (inert override_option) | D3 | ADR-0199 |
| AMEND-5 (vh_rate_limits composition) | D3 | ADR-0199 |
| AMEND-6 (descriptor hits_addend UInt64Value + wire ordering) | D5 + D6 | ADR-0197 |
| AMEND-7 (disable_key + runtime enforcement) | D1 + descriptor pin | ADR-0200 |
| AMEND-8 (X-RateLimit + reply byte-pin) | D4 | ADR-0197 |
| AMEND-9 (DELTA-2 architecture) | reuse survey | ADR-0198 |
| AMEND-10 (cross-namespace stat write) | D2 + reuse survey | ADR-0197 |
| AMEND-11 (action roster + key defaults) | descriptor pin | ADR-0197/ADR-0200 |

---

## 12. Deferred decisions (the planner / implementer settles these)

Sub-pin-level refinements of already-closed pins; settled at IMPL Tasks + the six-gate verification. None block phase-24 phase-done.

### A. Wire-shape / API byte-confirmation
1. **DELTA-2 chain-seed type + accessor return type** — the exact filter-importable compiled-policy type seeded onto the chain + whether `RouteRateLimits()`/`VirtualHostRateLimits()` return raw `[]*routev3.RateLimit` or a pre-compiled type (§3.2). Settles at IMPL (the highest-risk item; could force an escape-valve ADR per §10.C).
2. **`metadata` action value-extraction accessor** — the `streamInfo().dynamicMetadata()` (DYNAMIC) vs route-metadata (ROUTE_ENTRY) accessor chain (§4.1). Settles at IMPL against the existing stream-info accessor.
3. **PARSE-REJECT byte-stable wording** — the §5.1 + §5.2 wording finalized + asserted by `TestParseRejectConstants_ByteStable` per ADR-0080.
4. **Boot-reject common stderr substring** — the exact shared substring for `0033` (`domain` empty). Settles at IMPL (fixture authoring).
5. **X-RateLimit MIN-status + quota-policy byte format** — the exact `, <rpu>;w=<sec>[;name="<n>"]` concatenation + the MIN `limit_remaining` selection (§4.7 + AMEND-8). Settles at IMPL `headers_test.go` against the D4 capture.

### B. Cross-side determinism (fake-service)
6. **Proto-number-faithful fake encoding** — the fake `RateLimitService` emits by proto field NUMBER + omits unset optionals (`raw_body`/`dynamic_metadata`/`quota`/per-descriptor `hits_addend`) per AMEND-6. Settles at IMPL (`test/helpers/ratelimitgrpc/`).
7. **Descriptor `entries` action-order** — the engine appends entries in action-list order (§4.5 + AMEND-6); the fake's script keys on the canonical descriptor string. Settles at IMPL.

### C. Cross-phase regression-window
8. **DELTA-2 framework regression** — the new chain field + accessor pair touch shared HCM dispatch; Gate C race + the full 35-fixture differential confirm zero regression to the existing 33 fixtures.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at phase-24 phase-done)

Edit-bundle landing at the IMPL phase-done commit. None at this SPEC commit.

### A. NEW top-level subsection (1)
1. **NEW `### envoy.filters.http.ratelimit` subsection** inserted after `### envoy.filters.http.admission_control` (current line 2548). Subsections: filter scope (SEVENTEENTH §9 row; external-gRPC global rate limit); the 10-action descriptor engine + empty-action-drop + stage + cross-tier composition (per AMEND-4/5/11); the OK/OVER_LIMIT/error dispositions + the 429 / `x-envoy-ratelimited` / X-RateLimit byte-shape (per AMEND-8); the cluster-scoped 4-counter stat surface (per AMEND-1); the DELTA-2 route-table exposure (per AMEND-9); the NEW 10th canonical per-route shape; the 3 envoy-go-strict departures. Anticipated ~180-280 LoC.

### B. NEW envoy-go-strict departure records (3, may consolidate to 1)
2. **route `disable_key` PARSE-REJECT** (upstream consults runtime; envoy-go rejects). 3. **`extension` action PARSE-REJECT**. 4. **deprecated `dynamic_metadata` action PARSE-REJECT**. Departure-record count **15 → 18** (SPEC notes the 3 may consolidate into one "descriptor-deferral" record at IMPL).

### C. Per-section additions (3)
5. **Stat-name mapping 110 → 114 table extension** — 4 NEW cluster-scoped counters `cluster.<rls_cluster>.ratelimit.{ok,error,over_limit,failure_mode_allowed}` (the FIRST landed cross-namespace cluster-stat-charge; cross-references the ext_authz §6 amendment-8 deferral).
6. **Per-route canonical patterns cross-reference** — caption "updated through phase 23" → "through phase 24"; the 9 → 10 amendment paragraph (`RateLimitPerRoute` 10th canonical).
7. **Response-header allow-list** — `x-ratelimit-limit`/`-remaining`/`-reset` + `x-envoy-ratelimited` added to the documented set-equal discipline (§7.1).

### D. Edit-bundle summary

| Category | Count |
|---|---|
| NEW top-level subsection | 1 |
| NEW envoy-go-strict departure records | 3 (may consolidate to 1) |
| Per-section additions | 3 |
| **TOTAL** | **5–7** |

All edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-24 paragraphs.

---

## 14. Testing strategy

### 14.1 Unit tests — two-layer taxonomy

Test surface at `internal/filter/http/ratelimit/*_test.go` + `internal/grpcclient/ratelimit_client_test.go` + the HCM DELTA-2 tests per §6.10.

**Layer A — subject-only descriptor-engine + config + disposition tests:**
1. **`TestDescriptors_PerAction_*`** — exact descriptor `{key,value}` per action type (§4.1; the configurable-key defaults per AMEND-11; `expect_match` default true).
2. **`TestDescriptors_EmptyActionDrop_*`** — the two behaviors (action returns false ⇒ whole descriptor dropped + loop breaks; empty-key entry skipped, descriptor survives — §4.5 + AMEND-11).
3. **`TestDescriptors_Composition_*`** — Axis A (embedded `rate_limits` early-return; `override_option` ignored) + Axis B (the §4.3 `vh_rate_limits` decision table + the legacy `include_vh_rate_limits` force-include).
4. **`TestDescriptors_StageFilter_*`** — only the filter-stage bucket evaluated (§4.4).
5. **`TestDispositions_*`** — OK/OVER_LIMIT/error legs + fail-open vs fail-closed + the 429 byte-shape (rc_details `"request_rate_limited"`; error rc_details `"rate_limiter_error"`; `x-envoy-ratelimited`; resource_exhausted grpc 8/14) per §4.7 + AMEND-8.
6. **`TestXRateLimitHeaders_*`** — DRAFT_VERSION_03 MIN-status selection + the `;w=`/`;name=` quota suffix + unit→seconds (§4.7 + AMEND-8); emitted on all dispositions when enabled.
7. **`TestBuildCompiledConfig_PARSE_REJECT_*`** — table-driven §5.1 + §5.2; byte-stable per ADR-0080.
8. **`TestPerRoute_*`** — `RateLimitPerRoute` compile: `vh_rate_limits` honored; `override_option` accepted-but-ignored; `domain` override (§5.3).
9. **`TestRateLimitClient_*`** (grpcclient) — unary call + per-call timeout + verbatim error propagation + idempotent Close (clone the `AuthClient` test shape).
10. **`TestRouteTableRateLimits_*`** (hcm) — parse/retain/seed + the `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair returns the matched policies; zero-regression to existing route-table tests.

**Layer B — cross-side + structural differential** (§7): `0032-http-ratelimit` (scenarios b/c/d/e/f/g cross-side byte-exact; a/h subject-only structural via `StatsAsserter` — proven live via deliberate-break per `reference_differential_asserter_dispatch`) + `0033-http-ratelimit-boot-reject`.

### 14.2 Race + lint
`go test -race ./...` clean (incl. the new package + the per-stream gRPC client + the chain-seeded accessor); `go vet ./...` + `golangci-lint run` clean (no new suppressions); `go build ./...` clean.

### 14.3 Fuzzer
33rd fuzzer `FuzzRateLimitConfigParse` per §6.9. Must-never-panic across `buildCompiledConfig` + the descriptor-engine compile. Clean at 30s/seed.

### 14.4 h2spec + differential
h2spec 53/53 PASS at ADR-0051 pin (no regression); differential 35/35 GREEN.

### 14.5 Six-gate checklist
Per §7.4 — gates A/B/C/D/E/F as the load-bearing IMPL verification. All MUST be GREEN for the row-24 status flip.

---

## 15. Acceptance checklist (for the reviewer)

The phase-24 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts. All 18 items MUST be GREEN for row-24 status flip from `in-progress` to `done`. **If the PLAN author splits into 24.1/24.2 (§16), this checklist is the UNION across the sub-phases** (each sub-phase's REVIEW confirms its slice; row-24 flips to `done` only when both sub-rows land — item 17).

### A. Six-gate verification (6 items)
1. **Gate A — build** clean across `internal/filter/http/ratelimit/` + `internal/grpcclient/ratelimit_client.go` + HCM DELTA-2 + pre-existing packages.
2. **Gate B — vet + lint** clean; no new suppressions.
3. **Gate C — race** clean (new package + per-stream gRPC client + chain-seeded accessor).
4. **Gate D — differential** 35/35 GREEN; `0032` b/c/d/e/f/g cross-side byte-exact; `0033` boot-reject substring.
5. **Gate E — fuzz** `FuzzRateLimitConfigParse` clean at 30s/seed; no panics across 33 fuzzers.
6. **Gate F — h2spec** 53/53 PASS at ADR-0051 pin.

### B. Fixture coverage (1 item)
7. **Two-directory differential** per §7 — `0032-http-ratelimit` (8 scenarios) + `0033-http-ratelimit-boot-reject` (`domain` empty); fixture dir count 33 → 35; the shared fake `RateLimitService` (`test/helpers/ratelimitgrpc/`) dialed by both sides; proto-number-faithful encoding (AMEND-6).

### C. Stat-surface verification (1 item)
8. **Cluster-scoped 4-counter stat surface** per AMEND-1 + AMEND-10 + §11 D2: `cluster.<rls_cluster>.ratelimit[.<stat_prefix>].{ok,error,over_limit,failure_mode_allowed}`; project stat count 110 → 114; NO gauges; the cross-namespace write via `NewCounterIfAbsent` sanctioned by ADR-0197.

### D. Descriptor-engine verification (1 item)
9. **Descriptor-action engine fidelity** per §4 + §14.1 Layer A: 10 actions (keys/values per AMEND-11) + empty-action-drop (two behaviors) + stage filter + cross-tier composition (Axis A early-return + the Axis B decision table + legacy force-include) — all GREEN.

### E. PARSE-REJECT roster verification (1 item)
10. **PARSE-REJECT roster** per §5: §5.1 RATIFIED-from-config arms (empty domain; missing rate_limit_service; stage>10; bad request_type; >10 response headers) + §5.2 envoy-go-strict arms (disable_key; extension; dynamic_metadata) — byte-stable per ADR-0080 + table-driven.

### F. Disposition + reply-shape verification (1 item)
11. **OK/OVER_LIMIT/error dispositions + reply byte-shape** per §4.6 + §4.7 + AMEND-8: 429 + `request_rate_limited` rc-details + `x-envoy-ratelimited` + the AMEND-8 header order; error 500 + `rate_limiter_error` + nullptr-mutate; fail-open default; resource_exhausted grpc 8/14.

### G. X-RateLimit header verification (1 item)
12. **DRAFT_VERSION_03 X-RateLimit headers** per §4.7 + AMEND-8: `x-ratelimit-limit/-remaining/-reset` (MIN-status; `;w=`/`;name=` quota suffix; unit→seconds), emitted on all dispositions when enabled.

### H. DELTA-1 + DELTA-2 verification (1 item)
13. **`RateLimitClient` (DELTA 1)** unary wrapper composing the `Dialer` (ADR-0158 third application; no `Dialer` change) + **route-table `rate_limits` exposure (DELTA 2)** the chain-seeded `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair (NOT a `RequestRouteConfig()` reuse per AMEND-9); 19 HTTP filters wired.

### I. Per-route verification (1 item)
14. **`RateLimitPerRoute` 10th canonical** per §5.3 + ADR-0199: `vh_rate_limits` honored; `override_option` accepted-but-ignored (INERT); `rate_limits` Axis-A; `domain` override; ADR-0125 amendment 9 → 10 landed.

### J. ADR landing (1 item)
15. **4 NEW ADR §Context drafts + §Decision + §Consequences bodies landed** at per-Task Lands-in-Tasks: ADR-0197 (package + engine + client + cluster-stats + differential) + ADR-0198 (DELTA-2 route-table exposure) + ADR-0199 (10th canonical + ADR-0125 9→10) + ADR-0200 (RTDS/action PARSE-REJECTs). ZERO in-place §Decision AMENDMENTs; ONE ADR-0125 amendment.

### K. BEHAVIOR_CONTRACT.md edit-bundle (1 item)
16. **5–7-edit BEHAVIOR_CONTRACT.md bundle landed** per §13 (NEW `### envoy.filters.http.ratelimit` subsection + up-to-3 envoy-go-strict departure records [count 15 → 18] + 3 per-section additions; atomic per ADR-0052).

### L. DECISIONS + STATE + ROADMAP advance (1 item)
17. **Doc-state alignment**: DECISIONS.md ADR-0197..0200 full bodies + the ADR-0125 amendment at final state; next-free ADR-0201 (D-hypothesis: ADR-0201 UNCONSUMED at phase-done — HOLD-with-known-risk); STATE.md re-advanced; ROADMAP row 24 flipped to `done` (IF single-row; if the PLAN split fired, the parent row + sub-rows reflect 24.1/24.2 per §16); 19 HTTP filters wired; §9 family at 1 remaining row (`wasm`).

### M. Audit-trail verification (1 item)
18. **End-to-end audit-trail**: SPEC → PLAN (+ any split) → PROGRESS → REVIEW chain landed; per-task PROGRESS records map 1:1 to PLAN tasks; each §11 pin + §12 item recorded; D-hypothesis disposition recorded (ADR-0201 UNCONSUMED at phase-done, or the split re-mapping); six-gate verbatim outputs at REVIEW.

---

## 16. Section closeout + PLAN-time split-gate planning anchor (D7)

This SPEC is **lifecycle-state 1 → 2 complete** for phase 24. The successor session (lifecycle-state 2 → 3, skill `superpowers:writing-plans`) authors `PLAN.md` and **applies the ADR-0045 split-gate** (the LoC envelope ~1900–2700 is above the ~1500 gate — §1.2 + §3.4).

**Candidate 24.1 / 24.2 split axis (planning anchor, NOT a SPEC-time split):**

- **24.1 — core decision path:** the `internal/grpcclient` `RateLimitClient` (DELTA 1) + the filter package config + the descriptor engine for a CORE action subset (`generic_key`, `request_headers`, `remote_address`, `destination_cluster`, `header_value_match`) + `ShouldRateLimit` dispatch + OK/OVER_LIMIT/error dispositions + failure modes + the cluster-scoped stat surface + DELTA 2 (the route-table `rate_limits` exposure — needed for ANY descriptor build) + the cross-side differential `0032` (b/c/d/e) + the boot-reject `0033`. ADRs: ADR-0197 (core) + ADR-0198 (DELTA 2) + ADR-0200 (PARSE-REJECTs).
- **24.2 — remaining surface:** the remaining actions (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`) + the X-RateLimit DRAFT_VERSION_03 headers + `RateLimitPerRoute` (the 10th canonical + ADR-0125 9→10) + the `stage` multi-stage path + the `vh_rate_limits` Axis-B composition + the `0032` (f/g) scenarios. ADRs: ADR-0199 (perroute + ADR-0125 amendment) + the X-RateLimit/headers + remaining-actions slice of ADR-0197.

DELTA 2 lands in 24.1 (the route-table exposure is a precondition for any descriptor build). If the PLAN author splits, ROADMAP row 24 becomes a parent (`in-progress`) with `sub-phases = 24.1, 24.2`; each sub-phase gets its own SPEC slice + row + STATE pointer per BOOTSTRAP §6.2. If the PLAN author judges the surface carve-able within one row (unlikely at this LoC), single-row landing stands.

**Hand-off:** SPEC-time scope complete (the §10 D1–D7 empirical pins resolved in §11; the AMEND-1..11 catalog in §1.1; the ADR-0197..0200 §Context drafts to be appended to DECISIONS.md at this SPEC commit per ADR-0044). PLAN author proceeds.
