# Phase 24.1 SPEC — `envoy.filters.http.ratelimit` (core decision path + route-table exposure)

> **Lifecycle state:** SPEC.md authored (carved from the phase-24 parent master SPEC at the PLAN-time ADR-0045 split, ADR-0201); ROADMAP row `24.1` added `in-progress` at this split commit (parent row `24` stays `in-progress` with `sub-phases = 24.1, 24.2`; row `24.2` added `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/PLAN.md` per the phase-09..23 + 18.1 + 19.1 + 22.1 precedent. This SPEC is the authoritative input to the 24.1 PLAN.
>
> **Parent:** `docs/envoy-go/phases/24-http-filter-global-ratelimit/SPEC.md` (the RETAINED parent master SPEC — carries the full §1.1 **AMEND-1..11 catalog**, the §4 descriptor-action engine, the §5 PARSE-REJECT roster, the §6 code shapes + §6.10 file roster, the §7 differential envelope, the §10 ADR anchor map, the §11 D1–D7 empirical-pin matrix, the §13 BEHAVIOR_CONTRACT edit-bundle, the §14 testing taxonomy, the §15 acceptance checklist, the §16 split axis). **This 24.1 SPEC details the core-decision-path + route-table-exposure surface only; it REFERENCES the parent's §1.1/§3/§4/§5/§6/§7/§10/§11/§13/§14/§15 rather than repeating them.** The empirical AMENDs are SETTLED — do NOT re-derive.
>
> **Predecessors:** the parent SPEC + `docs/envoy-go/phases/24-http-filter-global-ratelimit/BRAINSTORM.md` (the 2 settled Q-decisions + the §3 framework deltas). NO off-master prebrainstorm-notes branch. The §10 D1–D7 pins were resolved at the parent SPEC session (parent §11); the §1.1 AMEND-1..11 catalog records the BRAINSTORM corrections.
>
> **Authored:** 2026-05-22 (at the phase-24 PLAN-time split commit; ADR-0201).

---

## 1. Purpose (24.1 slice)

Phase 24.1 lands the **core decision path** of `envoy.extensions.filters.http.ratelimit.v3.RateLimit` — enough of the global rate-limit filter to be a working, differential-green, both-sides-byte-exact filter on the common path, plus the framework primitive (DELTA 2) that ANY descriptor build depends on. It is the foundational sub-phase; 24.2 (`24.2-global-ratelimit-perroute-and-headers`) builds the remaining actions + X-RateLimit headers + `RateLimitPerRoute` on top.

24.1 delivers:

1. **The NEW `internal/filter/http/ratelimit/` package skeleton** — `TypeURL` + `New` (`HTTPFilterFactory`) + the filter value + boot-registration (alphabetical between `oauth2` and `rbac`, `cmd/envoy-go/main.go`; **19 HTTP filters wired** at 24.1). Package dir + Go-package identifier both `ratelimit`. (Parent §1 primitive 1; ADR-0197 package shape.)
2. **The filter `compiledConfig` + `buildCompiledConfig`** — the AMEND-3 13-field roster + defaults/clamps (`status_on_error` 500/[100,511]; `rate_limited_status` 429/<400⇒429; `request_type` string empty⇒both; `timeout` 20ms; `failure_mode_deny` false⇒fail-open; `stat_prefix`; `rlsClusterName`) + the cluster-load gates (REUSE 1, the ext_authz `buildGRPCCheckFn` precedent). (Parent §6.1 + AMEND-3.)
3. **DELTA 1 — `internal/grpcclient/ratelimit_client.go` `RateLimitClient`** — the THIRD ADR-0158 two-tier typed wrapper; `ShouldRateLimit` UNARY ⇒ clones `AuthClient` verbatim; `NewRateLimitClient(d, clusterName, timeout)` + `ShouldRateLimit(ctx, *RateLimitRequest)` + `sync.Once` `Close()`; no `Dialer` API change. (Parent §3.1; ADR-0197.)
4. **The descriptor-action engine for the CORE action subset** — `generic_key`, `request_headers`, `remote_address`, `destination_cluster`, `header_value_match` (5 of the 10 canonical actions) + the empty-action-drop discipline's TWO behaviors (parent §4.5) + the descriptor build/dispatch (parent §4.6). The remaining 5 actions (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`) + `stage` filtering + the Axis-B `vh_rate_limits` composition land at 24.2. (Parent §4.1 rows for the 5 core actions + AMEND-11 key defaults: `generic_key`→"generic_key", `header_value_match`→"header_match", `request_headers` requires a config key, `expect_match` default true.)
5. **`ShouldRateLimit` dispatch + OK/OVER_LIMIT/error dispositions** — async dispatch (`StopIteration`, the fault/ext_authz async-resume pattern) + OnDestroy per-stream cancellation (the ext_authz `callCtx`/`callCancel` precedent) + the parent §4.6 dispositions + the parent §4.7 byte-pinned OVER_LIMIT (429 / `request_rate_limited` rc-details / `x-envoy-ratelimited` / RLS + filter-config `response_headers_to_add` in the AMEND-8 order / gRPC 8 vs 14) + error (`status_on_error` 500 / `rate_limiter_error` / nullptr-mutate) + fail-open default. (Parent §4.6/§4.7 + AMEND-8.) **NOTE:** the X-RateLimit DRAFT_VERSION_03 headers are DEFERRED to 24.2 — 24.1's `enable_x_ratelimit_headers` parses to `compiledConfig` but the encode-side header injection (parent §6.6) lands at 24.2.
6. **The cluster-scoped cross-namespace 4-counter stat surface** — `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].{ok,error,over_limit,failure_mode_allowed}` self-registered via `ctx.Stats.NewCounterIfAbsent(...)` (parent §6.8 + AMEND-1 + AMEND-10). Project stat count **110 → 114**. The FIRST landed cross-namespace cluster-stat-charge.
7. **DELTA 2 — the HCM route-table `rate_limits` exposure** (AMEND-9; ADR-0198) — parse + retain the matched Route's `RouteAction.rate_limits[]` + the VirtualHost's `rate_limits[]`; seed the compiled policies onto the per-stream `FilterChain` at HCM dispatch (the ADR-0165 set-once-by-dispatch pattern); expose via the NEW `RouteRateLimits()` + `VirtualHostRateLimits()` `DecoderFilterCallbacks` accessor pair. **This is the highest-risk surface in phase 24** (parent §12 item 1 — the exact chain-seed type + accessor return-type could force an escape-valve ADR-0202). It lands in 24.1 because the route-table exposure is a precondition for ANY descriptor build. (Parent §3.2 + AMEND-9.)
8. **The §5 PARSE-REJECT roster** — the §5.1 RATIFIED-from-config arms (empty `domain`; missing `rate_limit_service`; `stage > 10`; bad `request_type`; >10 `response_headers_to_add`; cluster-load gates) + the §5.2 envoy-go-strict arms (route `disable_key` non-empty; `extension` action; deprecated `dynamic_metadata` action) — byte-stable per ADR-0080, asserted by `TestParseRejectConstants_ByteStable`. (Parent §5; ADR-0200.) Departure-record count **15 → 18**.
9. **The 33rd fuzzer `FuzzRateLimitConfigParse`** (parent §6.9) — must-never-panic across `buildCompiledConfig` + the (24.1-subset) descriptor-engine compile. Fuzzer count **32 → 33**.
10. **The cross-side differential `0032-http-ratelimit` scenarios (a)/(b)/(c)/(d-core)/(e)/(h)** + the boot-reject `0033-http-ratelimit-boot-reject`. Fixture dir count **33 → 35** lands at 24.1 (24.2 ADDS scenarios f/g to the existing `0032`, not new dirs — see §3).

## 2. Non-purposes (deferred to 24.2)

Per the parent §16 axis, these are explicitly **NOT** in 24.1 (they land at 24.2):

- The remaining 5 descriptor actions (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`) — parent §4.1.
- The X-RateLimit DRAFT_VERSION_03 response headers (`x-ratelimit-limit/-remaining/-reset`; `encode.go` + `headers.go`; parent §4.7 + §6.6) — emitted on all dispositions when enabled. 24.1 parses `enable_x_ratelimit_headers` into `compiledConfig` but does NOT emit the headers.
- `RateLimitPerRoute` (the 10th canonical + ADR-0125 9→10; parent §5.3; ADR-0199) — `compiled_perroute.go`.
- The `stage` multi-stage filtering (parent §4.4) — 24.1 evaluates the filter's default stage 0 bucket only; the parse-time stage bucketing + multi-stage selection lands at 24.2. (24.1 still PARSE-REJECTs `stage > 10` per §5.1.)
- The Axis-B `vh_rate_limits` cross-tier composition decision table (parent §4.3 Axis B) — 24.1 walks the route policy + (when present) the vhost policy under the OVERRIDE default only; the full INCLUDE/IGNORE/legacy-force-include table lands at 24.2.
- The `0032` scenarios (f) `vh_inclusion` + (g) `x_ratelimit_headers` — added to the existing `0032` dir at 24.2.

All parent §2 non-purposes (RTDS/runtime keying; `extension`/`dynamic_metadata` actions; `RateLimit.Override`; formatter syntax; gauges; multi-worker aggregation) apply to BOTH sub-phases.

## 3. Differential envelope (24.1 slice)

Per `reference_differential_fixture_dispatch_constraint` (one fixture dir = ONE runner branch) + `reference_differential_asserter_dispatch` (subject-side assertions in `StatsAsserter`, NOT `SubjectAsserter`). 24.1 CREATES both directories (33 → 35); 24.2 ADDS scenarios to the existing `0032`:

- **`0032-http-ratelimit`** (cross-side, FULLY deterministic via the SHARED fake `RateLimitService` in `test/helpers/ratelimitgrpc/` + a `fixture.HTTPGlobalRateLimitGRPC` BackendKind + a fixture-local `driver.go` following the ext_authz `0021` fixed-pre-allocated-port pattern; the fake emits by proto field NUMBER + omits unset optionals per AMEND-6). **24.1 scenarios:** (a) `parse_ok` [subject-only structural], (b) `ok_admit` [cross-side byte-exact], (c) `over_limit_429` [cross-side byte-exact], (d) `descriptor_actions` [cross-side — restricted to the 24.1 core actions: `generic_key`/`request_headers`/`remote_address`/`header_value_match`; 24.2 extends (d) with the remaining actions], (e) `failure_mode_open` [cross-side byte-exact], (h) `stat_surface` [subject-only via `StatsAsserter.AssertStats`, proven live via deliberate-break]. (Parent §7.1 scenario table.)
- **`0033-http-ratelimit-boot-reject`** — the `domain`-empty shared reject (parent §7.2). Created in full at 24.1.

24.2 ADDS (f) `vh_inclusion` + (g) `x_ratelimit_headers` to `0032` and EXTENDS (d) with the remaining 5 actions; it creates NO new fixture directory.

## 4. Source-file roster (24.1 subset of parent §6.10)

| File | 24.1 scope | Anticipated LoC |
|---|---|---|
| `ratelimit/doc.go` | package doc | ~25 |
| `ratelimit/ratelimit.go` | `TypeURL` + `New` factory + filter value | ~120–180 |
| `ratelimit/compiled_config.go` | `compiledConfig` + `buildCompiledConfig` + §5 PARSE-REJECT roster (FULL roster — both §5.1 + §5.2 land at 24.1) | ~250–350 |
| `ratelimit/descriptors.go` | the §4 engine for the 5 CORE actions + empty-action-drop + Axis-A early-return + OVERRIDE-default vhost walk (the full 10-action + Axis-B table extends at 24.2) | ~200–300 (of the parent's ~350–500) |
| `ratelimit/decode_headers.go` | descriptor build + async `ShouldRateLimit` + `StopIteration` + OnDestroy cancel | ~120–180 |
| `ratelimit/dispositions.go` | OK/OVER_LIMIT/error + the §4.7 local-reply byte-shape (X-RateLimit injection stubbed; lands 24.2) | ~120–180 |
| `ratelimit/stats.go` | cluster-scoped `filterStats` (parent §6.8) | ~40–60 |
| `internal/grpcclient/ratelimit_client.go` | DELTA 1 (parent §3.1) | ~60–90 |
| `internal/filter/hcm/{config.go,route.go,chain.go}` + `internal/filter/http/{callbacks.go,chain.go}` | DELTA 2 route-table parse/retain/seed + the `RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair (parent §3.2) | ~250–400 |
| `cmd/envoy-go/main.go` | boot-registration (alphabetical oauth2↔rbac) | ~1–3 |
| `ratelimit/*_test.go` + `ratelimit_client_test.go` + HCM DELTA-2 tests | parent §14.1 Layer A for the 24.1 surface | ~600–900 |

Files DEFERRED to 24.2: `ratelimit/encode.go`, `ratelimit/headers.go`, `ratelimit/compiled_perroute.go` (parent §6.6/§6.7/§6.10).

## 5. ADR landing (24.1)

Per the parent §10 anchor map + ADR-0201 split mapping, 24.1 lands:

- **ADR-0197 (CORE portion)** — §Decision + §Consequences for the package shape + the 5-core-action engine + `RateLimitClient` + the OK/OVER_LIMIT/error dispositions + the OVER_LIMIT/error byte-shape + the cluster-scoped 4-counter stat surface + the deterministic shared-fake differential. (The X-RateLimit-header + remaining-actions slice of ADR-0197 lands at 24.2.)
- **ADR-0198 (FULL)** — §Decision + §Consequences for the DELTA-2 route-table exposure (the precondition for any descriptor build).
- **ADR-0200 (FULL)** — §Decision + §Consequences for the RTDS/action-deferral PARSE-REJECTs.

The §Context drafts for ADR-0197/0198/0200 are already anchored at the parent SPEC commit (DECISIONS.md). **Escape-valve reserve: ADR-0202** (the highest-risk DELTA-2 chain-seed type per parent §12 item 1 could fire it at 24.1 IMPL; hypothesized UNCONSUMED).

ADR-0199 (`RateLimitPerRoute` + ADR-0125 9→10) and the X-RateLimit slice of ADR-0197 land at **24.2** — the canonical-per-route roster STAYS 9 until 24.2 IMPL.

## 6. Six-gate checklist (24.1)

Identical matrix to the parent §7.4 / phase-09..23, scoped to the 24.1 surface:

- **Gate A — build:** `go build ./...` clean (incl. `internal/filter/http/ratelimit/` + `internal/grpcclient/ratelimit_client.go` + the HCM DELTA-2 changes).
- **Gate B — vet + lint:** `go vet ./...` + `golangci-lint run` clean; no new suppressions.
- **Gate C — race:** `go test -race ./...` clean (incl. the new package + the per-stream gRPC client cancellation + the chain-seeded route-table accessor).
- **Gate D — differential:** **35/35** fixtures GREEN (0000-0031 pre-existing + 0032 [scenarios a/b/c/d-core/e/h] + 0033 new); cross-side byte-exact on 0032 (b)/(c)/(d-core)/(e); boot-reject substring on 0033.
- **Gate E — fuzz:** `FuzzRateLimitConfigParse` clean at 30s/seed; no panics across the 33 project-wide fuzzers.
- **Gate F — h2spec:** 53/53 PASS at the ADR-0051 v1.32.4 pin.

All six MUST be GREEN for the row-24.1 status flip `in-progress → done`. The parent row `24` stays `in-progress` (closes at 24.2 phase-done).

## 7. Acceptance checklist (24.1 — subset of the parent §15 UNION)

The 24.1 reviewer confirms (the parent §15 items that map to the 24.1 slice): the six gates (parent §15 items 1–6, scoped); the two-directory differential at 24.1 scenario coverage (item 7, partial — f/g + d-extension at 24.2); the cluster-scoped 4-counter surface 110 → 114 (item 8); the descriptor-engine fidelity for the 5 core actions + empty-action-drop (item 9, partial); the FULL PARSE-REJECT roster byte-stable (item 10); the OK/OVER_LIMIT/error dispositions + reply byte-shape (item 11); DELTA-1 + DELTA-2 + 19 HTTP filters (item 13); ADR-0197[core] + ADR-0198 + ADR-0200 bodies landed (item 15, partial); the BEHAVIOR_CONTRACT departure records 15 → 18 (item 16, partial). Items 12 (X-RateLimit), 14 (`RateLimitPerRoute`), and the remaining slices of items 7/9/15/16 land at **24.2**; the parent row-24 `done` flip + the full §15 UNION verification happen at 24.2 phase-done (item 17).
