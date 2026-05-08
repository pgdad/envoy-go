# Phase 12 — HTTP filter `envoy.filters.http.csrf` (`internal/filter/http/csrf/`, differential fixture `0014-http-csrf`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.csrf` extension) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 12), §9 (HTTP filters family — phase 12 is the FIFTH top-level row to land under the §9 family heading after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11 per ADR-0106 settled by phase 09 + reaffirmed by phases 10 + 11); `docs/envoy-go/phases/12-http-filter-csrf/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; 1597 lines, 15 sections, **read in full**); `docs/envoy-go/phases/12-http-filter-csrf/BRAINSTORM.md` (the autonomous-brainstorm artefact; SPEC §1.1's 4 amendments + 3 confirmations supersede where they diverge from BRAINSTORM hypotheses — the SPEC is authoritative; consult BRAINSTORM only for design-rationale provenance); `docs/envoy-go/phases/11-http-filter-local-ratelimit/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (closed read-only history; phase 11's PLAN at master `1de512d` is the structural precedent — task-numbering, TDD-step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections; phase 11 used 16 tasks for ~533 LoC production code + ~770 LoC fixture); `docs/envoy-go/phases/10-http-filter-header-mutation/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (secondary precedent for the per-route TPFC + multi-tier surface — phase 12 does NOT use multi-tier `ResolveAllTiers`; reuses the 3-tier `RequestRouteConfig` per ADR-0073); `docs/envoy-go/phases/09-http-filter-fault/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (tertiary precedent for `SendLocalReply` + `StopIteration` request-side terminal-replace pattern at `internal/filter/http/fault/fault.go:321` that phase 12 reuses verbatim for the rejection path); `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` (the cors precedent's PLAN — the per-filter package-shape phase 12 inherits with single-token directory `csrf/` matching cors/`cors/`); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0119 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format — phase 12's 3-item deferral list (per SPEC §2.1.1 / §2.1.2 / §2.1.3) lives inline at BEHAVIOR_CONTRACT §13.1 + §13.4 rather than in a dedicated ADR (mirrors phases 10 + 11 precedent), **ADR-0044** ADR-on-impl convention, **ADR-0045** planner-time-split discipline (~25 tasks / ~1500 LoC thresholds — both well under for this phase per `## Scope check` below), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0061** stats Registry / SN1–SN8 flattening rules + Rule SN9 from phase 11 — phase 12 emits THREE new stats per HCM `stat_prefix` (so the existing 26-name table extended by phase 11 grows to 29; **NO new SN flattening rule** — csrf reuses the existing HCM-namespace SN2 rule per §11.6 confirmation; UNLIKE phase 11 which introduced filter-specific Rule SN9), **ADR-0071** HTTP-filter framework chain-shape + factory pattern + iteration-protocol surface — phase 12's filter is the FIRST production filter to combine `SendLocalReply + StopIteration` (request-side terminal-replace per ADR-0102; reused VERBATIM from the fault precedent at `internal/filter/http/fault/fault.go:321`) with NO STATEFUL RESOURCES (purely synchronous + lock-free hot path; the `runtimeConfig` is read-only after `New`; counters use `*atomic.Int64`; per SPEC §5.9), **ADR-0072** HTTPRegistry threaded constructor map + factory typed_config validation contract — phase 12's `New` factory mirrors Envoy v1.37.2's CONFIG-LOAD-TIME PGV rejection per SPEC §11.11 (filter-internal validation that `cfg.FilterEnabled != nil && cfg.FilterEnabled.DefaultValue != nil` is enforced at parse time, mirroring Envoy's PGV envelope per the phase 11 ADR-0115 filter-internal-validation precedent), **ADR-0073** typed_per_filter_config 3-tier merge (most-specific override) — phase 12 reuses VERBATIM (no `ResolveAllTiers` invocation; the most-specific-override discipline applies; **NO amendment** required — phase 12 is data-only AND most-specific-override; phase 11's ADR-0117 amendment paragraph (stateful per-route extension) and phase 10's ADR-0110 amendment (multi-tier evaluation) both stay landed and unused by phase 12), **ADR-0074** filter set: cors + envoy_go_test — phase 12 adds csrf as the SIXTH real production filter (after cors, envoygotest, fault, header_mutation, local_ratelimit) under the same package-shape discipline, **ADR-0075** sendLocalReply enters encode chain at filter[len-1] — UNCHANGED in phase 12 (csrf's reject path uses `dcb.SendLocalReply` per the fault precedent; the chain's `localReplyDone` gate carries the response back to client without dialing upstream), **ADR-0100** FactoryCtx framework extension (`Stats *stats.Registry` + `StatPrefix string`) — csrf CONSUMES `ctx.Stats` (for the three-counter `filterStats` registration per SPEC §6.6) AND **CONSUMES `ctx.StatPrefix`** (the HCM-level stat_prefix per §11.6 confirmation; csrf has NO `stat_prefix` proto field — the namespace anchor is the HCM `stat_prefix`; UNLIKE local_ratelimit which has its own `cfg.StatPrefix` proto field). The 3-field FactoryCtx stays as-is per ADR-0100, **ADR-0101** runtimeConfig shape + parser pattern + StringMatcher non-exact variants dropped at PARSE time per §3 — phase 12's `runtimeConfig` mirrors fault's structurally (2 fields per SPEC §6.2; closure-captured at `New`, immutable post-construction; per-route TPFC entries each carry independent `additionalOrigins` slices but SHARE the listener-level `*filterStats` pointer per §11.9; ADR-0101 §3 parse-time-drop discipline applied verbatim to `additional_origins[].StringMatcher` non-exact variants), **ADR-0102** terminal-replace + StopIteration localReplyDone gate — VERBATIM reuse for the rejection path; no new framework primitive, **ADR-0103** fault abort wire shape (body byte-exact) — phase 12's wire shape follows the same discipline (body `Invalid origin`, 14 bytes, no LF; 4-header set lowercase wire-form per SPEC §11.10; `server: envoy` literal per the existing HCM `serverHeader()` at `internal/filter/hcm/codec.go:17`), **ADR-0106** §9 HTTP filters family expansion shape (flat top-level rows + no-sibling-stub) — UNCHANGED in phase 12 (phase 12 is a flat top-level row, not a sub-phase of any §9 parent; the §9 heading at ROADMAP line 56 stays unchanged), **ADR-0114..ADR-0119** (phase 11 ADRs); ADR-0119 is the verified DECISIONS.md tail at master `0f3a710` (phase 11 phase-done REVIEW); phase 12's five anticipated ADRs land at ADR-0120..ADR-0124 per SPEC §8); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## HTTP filter chain` umbrella at line 739 hosts the new `### envoy.filters.http.csrf` subsection per SPEC §13.1, inserted AFTER the existing `### envoy.filters.http.fault` subsection at line 882 landed by phase 09 AND the `### envoy.filters.http.header_mutation` subsection at line 939 landed by phase 10 AND the `### envoy.filters.http.local_ratelimit` subsection at line 1008 landed by phase 11; `## Stat-name mapping` 26-name table at line 60 extends to **29 names** with a three-counter set per SPEC §13.2; **NO new tag-extractor** + **NO new SN flattening rule** (csrf reuses the existing HCM-namespace `envoy_http_conn_manager_prefix` extractor per Rule SN2 from ADR-0061 — UNLIKE phase 11's filter-specific Rule SN9); `## Equivalence Matrix` at line 9 gains one new row per SPEC §13.3; `## Forward-pointer notes` at line 1455 gains a new `### Phase 12 forward-pointer notes` subsection per SPEC §13.4 — covering the 3-item deferral list (`filter_enabled` PGV-required + percentage-gating deferred; `shadow_enabled` shadow-mode deferred; `additional_origins[].StringMatcher` non-exact variants dropped at parse) + the operator footgun callout (per §11.7 + §11.8 amendments) + the per-route stats-shared note (per §11.9 amendment, divergence from phase 11 precedent); lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 12 — phase 12 is a pure HTTP-layer filter addition; touches no codec/framer/HPACK paths; the h2spec gate at 53/53 PASS is mechanical re-run); `docs/envoy-go/ROADMAP.md` (row `12` per the SPEC commit's row-flip; row `12` flips `in-progress → done` at this phase's phase-done; the §9 HTTP filters family heading at row 56 stays unchanged across all §9-family-row landings per ADR-0106); `internal/filter/http/cors/cors.go` (the package-shape precedent csrf inherits — `TypeURL` constant + `New` factory + `filter` struct; csrf's `SetDecoderCallbacks` callback-wiring pattern follows cors's pattern at `cors.go:55` — per SPEC §6.3 phase 12 sets only `dcb`, the encode side is pure pass-through. **HOWEVER**, csrf's `HTTPFilter` value sets `Decoder: f, Encoder: nil` — csrf is decoder-only and does NOT need to satisfy `StreamEncoderFilter` — see planner-time decision 2 below for the `HTTPFilter` shape choice); `internal/filter/http/fault/fault.go` (the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution via `f.dcb.RequestRouteConfig()` + the `cb.SendLocalReply(status, body, OrderedHeaders{Content-Type: text/plain}) + return StopIteration` pattern at fault.go:321 that phase 12 reuses verbatim; phase 12's per-instance `filter` struct mirrors fault's modulo no-async-resume / no-timer / no-rng / no-overflow / no-encode-side); `internal/filter/http/localratelimit/local_ratelimit.go` (tertiary precedent — closely-analogous `DecodeHeaders` body discipline modulo the algorithm; the `f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{...}) + return StopIteration` pattern at the rejection path; phase 12's filter struct is structurally simpler since there is no token-bucket primitive and per-route stats are SHARED rather than independent); `internal/filter/http/header_mutation/header_mutation.go` (quaternary precedent for the unmarshal-at-New + closure-capture-runtimeConfig + per-route TPFC parsing pattern; phase 12 explicitly does NOT use header_mutation's multi-tier `ResolveAllTiers` accessor since csrf is most-specific-override per ADR-0073); `internal/filter/http/types.go` (`FilterHeadersStatus` + `StreamDecoderFilter` + `StreamEncoderFilter` + `HTTPFilter` + `HTTPFilterFactory` + `FilterInstanceFactory` + `FactoryCtx` — UNCHANGED in phase 12; the 3-field `FactoryCtx` per ADR-0100 stays as-is; phase 12 consumes `ctx.Stats` AND `ctx.StatPrefix`); `internal/filter/http/callbacks.go` (`DecoderFilterCallbacks.SendLocalReply(status int, body string, headers OrderedHeaders)` per ADR-0075 — note `body` is `string` not `[]byte`; `DecoderFilterCallbacks.RequestRouteConfig() proto.Message` per ADR-0073 — the per-route accessor csrf calls from `DecodeHeaders`); `internal/filter/http/perroute.go` (existing 3-tier `Resolve` per ADR-0073 — phase 12 reuses VERBATIM via the chain-managed `f.dcb.RequestRouteConfig()` callback; the phase-10-introduced `ResolveAllTiers` sibling stays landed but is NOT consumed by phase 12); `internal/filter/http/registry.go` (existing extension registry — phase 12 adds one `Register` call site upstream in `cmd/envoy-go/main.go`; the phase-10-introduced `RegisterPerRouteValidator` hook is NOT consumed by phase 12 since csrf has no per-route invariants requiring boot-time validation — per-route TPFC entries are validated lazily via the same `New` factory path as listener-level entries); `internal/stats/registry.go` + `internal/stats/name.go` (existing stats Registry + the `flattenToProm` Prometheus rendering per Rules SN1–SN9; phase 12 ADDS NO NEW RULE — csrf reuses the existing HCM-namespace SN2 rule which covers `http.<HCM stat_prefix>.<rest>` extraction for the `envoy_http_conn_manager_prefix` Prometheus tag); `internal/filter/hcm/codec.go:17` (`serverHeader()` returning literal `"envoy"` — confirms the SPEC §11.10 empirical observation that the rejection response carries `server: envoy`); `internal/filter/hcm/h2dispatch.go:202-216` (the `:method` injection prereq — phase 12's csrf filter consumes `:method` via `headers.Get(":method")` per the cors precedent at `cors.go:206`; no new injection needed); `cmd/envoy-go/main.go:113-124` (the `httpReg.Register` call block + alphabetical-after-router ordering — phase 12 inserts `csrf.Register` between `cors` and `envoygotest`).

**Goal:** Land envoy-go's `envoy.filters.http.csrf` HTTP filter — the FIFTH production HTTP filter after cors (07.1), fault (09), header_mutation (10), and local_ratelimit (11), and the FIFTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Concretely (per SPEC §1 + §4): a new `internal/filter/http/csrf/` package owning the filter implementation under the cors + fault precedents' single-token-package-shape discipline (`csrf.go` + `csrf_test.go` + `doc.go` + `fuzz_test.go`; the file split is settled here per the file-structure decision in `## File structure` below — origin-parsing helpers stay in `csrf.go` rather than splitting into a sibling `origin.go` since the helpers are tightly coupled to the trichotomy + comparison algorithm and total ~80 LoC; the SPEC §4.1 PLAN-author option for split is exercised by NOT splitting; ~250-400 LoC across the production file + ~150-200 LoC unit tests + ~40 LoC fuzzer + ~25 LoC doc.go); a `cmd/envoy-go/main.go` one-line registration delta (`httpReg.Register(csrf.TypeURL, csrf.New)` inserted alphabetically after the existing `cors` registration and before the `envoygotest` registration, plus the matching package import; ~3 LoC delta); a NEW differential fixture `0014-http-csrf` (`test/fixtures/0014-http-csrf/`) with `envoy.yaml` + `envoy-go.yaml` (single listener `l_main` per planner-time decision 7 — diverges from phase 11's 4-listener topology since csrf's 6 scenarios all run against the same listener with two routes (`/` + `/route-only`); explicit `filter_enabled.default_value: 100/HUNDRED` on BOTH sides per SPEC §11.11 amendment + §1.1 amendment 3 — Envoy PGV-rejects boot if absent; `additional_origins` host:port form per §11.8 amendment) + `expectations.yaml` + `README.md` + `driver/driver.go` (six-scenario sequential orchestration per SPEC §7.1 + §7.2 — all 6 scenarios in one Drive call against a single listener) + `backends/backend.go` (minimal Go HTTP backend; ~30 LoC; mirrors fault 0011 + local_ratelimit 0013 backend pattern with body `backend\n`); a NEW `BackendKind` enum value `HTTPCsrf BackendKind = 11` in `test/differential/fixture/fixture.go` + a matching `startHTTPCsrfBackend` spawn helper in `test/differential/runner_test.go` + the blank-import for the fixture driver (~25 LoC delta); a NEW fuzzer `FuzzCsrfPolicyConfigParse` (~40 LoC; 30s budget per ADR-0018; **sixteenth fuzzer overall** — phase 11 closed at fifteenth `FuzzLocalRateLimitConfigParse`); a `BEHAVIOR_CONTRACT.md` in-place edit per SPEC §13 (NEW `### envoy.filters.http.csrf` subsection under the existing `## HTTP filter chain` umbrella per §13.1 inserted AFTER the existing local_ratelimit subsection; `## Stat-name mapping` 26→29-name table extension per §13.2 — **NO new tag-extractor preamble** since csrf reuses HCM-namespace SN2 per §11.6; `## Equivalence Matrix` new row per §13.3; NEW `### Phase 12 forward-pointer notes` subsection per §13.4 — operator footgun + 3-item deferral list + per-route stats-shared note); five new ADRs ADR-0120..ADR-0124 per SPEC §8 (ADR-0120 package shape `csrf/` single-token directory matching cors precedent + extension-registry registration ordering `router → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`; ADR-0121 runtimeConfig shape + 1-consumed/1-PGV-validated-not-honored/1-deferred field decomposition + PGV-mirror filter-internal validation discipline at `New` time for `filter_enabled` presence + `additional_origins[].StringMatcher` non-exact variants dropped at PARSE time per ADR-0101 §3; ADR-0122 origin extraction trichotomy (Origin: null literal → empty NO Referer fallback; Origin empty/absent → Referer fallback; Origin non-empty unparseable → verbatim string) + comparison algorithm host:port-only equality (scheme stripped on both sides; NO normalization — case preserved, default ports preserved; trailing slash stripped via URL parser) + method gate canonical 4-method set + `additional_origins[].exact` matched against `host[:port]` form NOT full URL with scheme — operator footgun; ADR-0123 rejection path wire shape `SendLocalReply(403)` + body byte-exact `Invalid origin` 14 bytes no LF + 4-header set lowercase wire-form `content-length`/`content-type`/`date`/`server: envoy` + `StopIteration` from `DecodeHeaders` reuses fault.abort/local_ratelimit primitive; ADR-0124 stat-table 26→29-name extension + 3 csrf counters + namespace anchor at HCM stat_prefix reusing existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor NO new SN flattening rule + drop `shadow_request_invalid` from MVP stat surface + per-route stats SHARED with listener-level diverging from phase 11 precedent). After phase 12, the project has proven its fourteenth-leading-edge engineering claim per SPEC §1: *envoy-go's HTTP filter framework hosts a synchronous, request-side-only origin-enforcement filter with no framework extension; the existing fault `SendLocalReply` + `StopIteration` mechanism carries through verbatim for the rejection path; the stat surface extends from 26 to 29 names with shared-with-listener per-route stat semantics; the host:port-only comparison discipline (no scheme, no normalization) is mirrored verbatim from upstream Envoy v1.37.2; the parse-time-drop discipline for `additional_origins[].StringMatcher` non-exact variants matches phase 09 fault `headers` discipline per ADR-0101 §3 verbatim; `filter_enabled` is PGV-required at parse-time — envoy-go validates field presence at `New` time mirroring Envoy's PGV envelope per the phase 11 ADR-0115 filter-internal-validation precedent (MAJOR REVISION from BRAINSTORM "silent-ignore" hypothesis per SPEC §11.11); per-route TPFC override is data-only AND stat-sharing — the FIRST production filter to demonstrate this pattern (DIVERGES from phase 11's stateful-per-route + independent-stats precedent per ADR-0117); all under flat top-level row expansion (per ADR-0106).* This is the FIFTH §9 family-row to land; subsequent filters (compression, jwt_authn, …) follow the same row-as-its-own-phase pattern. ROADMAP row `12` flips `in-progress → done` AT the phase-done commit; the §9 family heading at ROADMAP line 56 stays unchanged (headings are not rows; per ADR-0106); STATE.md flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle.

**Architecture:** The 12 surface is the additive registration of one new HTTP filter under `internal/filter/http/` with ZERO framework deltas (phase 12 is the structurally-thinnest §9 family-row to date — no new `FactoryCtx` field, no new `HTTPRegistry` method, no new `PerRouteConfig` accessor, no new `RegisterPerRouteValidator` hook, no `ADR-0073` amendment, no new SN flattening rule). The `csrf.New` factory runs at HCM-build time per ADR-0072's two-step pattern: (a) parses + validates the typed_config Any (rejects `tc == nil`, malformed Any, AND the §11.11 PGV-mirror filter-internal validation that `cfg.FilterEnabled != nil && cfg.FilterEnabled.DefaultValue != nil` — the `shadow_enabled` field is NOT validated since Envoy itself permits omission); (b) compiles `additional_origins[]`: iterates each repeated entry, drops non-exact `StringMatcher` variants at PARSE time per ADR-0101 §3 verbatim discipline ("only `HeaderMatcher_StringMatch` with non-empty `Exact` value is honored. All other variants … are silent-ignored at parse time"), drops empty-value `exact` entries, appends the verbatim `Exact` string (NO normalization — preserves operator's `host[:port]` form byte-for-byte per §11.7 + §11.8 amendments) into the runtime `additionalOrigins []string` slice; (c) constructs a `*runtimeConfig` capturing the 2 runtime fields per §6.2 (`additionalOrigins []string`, `stats *filterStats`) — the `filter_enabled` percentage value is NOT captured (silent-ignored at runtime per §1.1 amendment 3; the `shadow_enabled` value is also NOT captured); (d) constructs the `*filterStats` three-counter set via `ctx.Stats.NewCounter("http." + ctx.StatPrefix + ".csrf." + name)` for `request_valid`, `request_invalid`, `missing_source_origin` per SPEC §6.6 — namespace anchors at the HCM `stat_prefix` (which is `ctx.StatPrefix` per ADR-0100) NOT a filter-level proto field (csrf has NO `stat_prefix` proto field); (e) returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{rc: rc}` per request bound to the closure-captured `*runtimeConfig`. The per-instance `*filter` implements `StreamDecoderFilter` per the cors precedent (request-side rejection decision in `DecodeHeaders`; the encode side is structurally absent — the `HTTPFilter` value returned by the factory sets `Encoder: nil` per planner-time decision 2). `DecodeHeaders` body discipline (per SPEC §6.5 + §11.1 + §11.2 + §11.3 + §11.7 + §11.8): read `:method` via `headers.Get(":method")` (chain-injected per `internal/filter/hcm/h2dispatch.go:214` for H2; H1 path needs verification at Task 3 — see planner-time decision 8); short-circuit to `Continue` if method ∉ `{POST, PUT, DELETE, PATCH}` (no counter increment, no origin parse); resolve `runtimeConfig` via `f.dcb.RequestRouteConfig()` per ADR-0073 (returns most-specific per-route config OR the listener-level closure-captured config when no per-route entry matches); compute `target.hostAndPort` from `Host`/`:authority` header (prepending a synthetic `http://` scheme to the URL parser since the scheme is stripped per §11.3 amendment — see planner-time decision 8); compute `source.hostAndPort` per the §11.2 trichotomy (Origin: `null` literal → empty; Origin empty/absent → fall back to Referer's `hostAndPort`; Origin non-empty unparseable → verbatim string); evaluate via the §6.4 disposition table (source empty → `missing_source_origin +1` + reject; source matches any `additionalOrigins[]` entry → `request_valid +1` + Continue; source matches target → `request_valid +1` + Continue; otherwise → `request_invalid +1` + reject); on reject path call `f.dcb.SendLocalReply(403, "Invalid origin", OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})` and return `StopIteration` (the chain auto-injects `content-length: 14`, `date: <RFC1123>`, `server: envoy` per the existing fault precedent + HCM `serverHeader()` at `internal/filter/hcm/codec.go:17`); on Continue path return `Continue` directly. Per-route 3-tier resolution uses the existing `f.dcb.RequestRouteConfig()` callback per ADR-0073 (most-specific-override; the framework's pre-phase-10 default model). The framework's `BuildPerRouteConfig` (`internal/filter/http/perroute.go:63-85`) merely `UnmarshalNew`'s each per-route TPFC Any into a generic `proto.Message` — it does NOT call any registered filter's `New`. Phase 12 therefore delivers per-route data-only-with-shared-stats via a request-time **direct unmarshal**: when `f.dcb.RequestRouteConfig()` returns a non-nil `proto.Message` (which is a `*csrfv3.CsrfPolicy`), the filter type-asserts + builds a fresh `*runtimeConfig` on the fly carrying its own compiled `additionalOrigins` slice but SHARING the listener-level `*filterStats` pointer via the closure-captured `rc.stats` (per §11.9 amendment — see planner-time decision 6 for the exact mechanism). UNLIKE phase 11 (lazy cache + `NewCounterIfAbsent`), phase 12 uses a much simpler shared-stats model: the per-route runtime build is small (just compile `additional_origins`) and there is no caching benefit since the slice is small + the comparison loop is short. Per-route stats SHARING is the load-bearing simplification: the per-route `*runtimeConfig` builder takes the listener-level `*filterStats` as input and reuses it verbatim — there is no need for `NewCounterIfAbsent` at request time. Concurrency model: per-instance state — the `*filter{rc, dcb}` carries a `*runtimeConfig` reference (closure-captured at `New`; immutable post-construction; read-only thread-safe); the `*runtimeConfig` carries a `[]string` slice (immutable post-construction) + a `*filterStats` pointer (3-counter `*atomic.Int64`s, lock-free counter increments per ADR-0061). NO mutex. NO `sync.Map`. NO `sync.Once`. NO LBP-1-adjacent declaration (UNLIKE phase 11 which had `sync.Mutex` per `*tokenBucket` and ADR-0116's LBP-1-adjacent commentary). csrf is purely lock-free at the request hot path; the `additional_origins` slice iteration is a read-only loop over an immutable slice; the counter increments are atomic; the `*filterStats` is a struct of `*atomic.Int64`s allocated once at `New` time and never reallocated. Phase 12 is the FIRST production HTTP filter with stats that does NOT need any synchronization primitive at the request hot path. Captures naturally under the existing LBP-1 (closure-capture half preserved; lock-free half preserved — no departure required). Differential surface: fixture `0014-http-csrf` runs 6 scenarios per SPEC §7.1 (same-origin allowed / cross-origin rejected / additional_origins host:port match / missing source-origin rejected / Referer fallback allowed / per-route wholesale-override with shared stats) under ONE pre-configured listener (`l_main`) with two routes (`/` default + `/route-only` per-route TPFC) per planner-time decision 7; all 6 scenarios run in ONE `DriveReference` / `DriveSubject` invocation against a shared static backend probe per SPEC §7.4; counter-delta assertions across 3 stat names per scenario (per-route AGGREGATES per §11.9 — counters are summed across listener + per-route, not split into independent series); status + body + post-rejection header set asserted byte-equivalent across reference Envoy v1.37.2 vs envoy-go.

**Tech Stack:**
- Go 1.23 (unchanged from 09 + 10 + 11; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `errors`, `fmt`, `net/http`, `net/url` (NEW for csrf — used for `Origin`/`Referer`/target URL parsing), `strings`, `sync/atomic` — the new `internal/filter/http/csrf/` package consumes only stdlib (no new module imports introduced by 12).
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3` (NEW import in this phase) — `*envoyextensionsfiltershttpcsrfv3.CsrfPolicy` proto. Already present in `go.sum`'s transitive closure (the go-control-plane module-level dependency is unchanged from 11; no `go.mod` bump needed — verified at `## Execution preconditions` step 11 below). Note: csrf has NO `CsrfPolicyPerRoute` wrapper — the SAME `CsrfPolicy` proto type serves both listener-level + per-route TPFC purposes (UNLIKE local_ratelimit which has `LocalRateLimit` + `LocalRateLimitPerRoute` as distinct top-level proto messages).
- `github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3` (existing; introduced by phase 09) — `*envoytypematcherv3.StringMatcher` field type for `additional_origins[]`. The `Exact` variant + non-empty-value qualifier per ADR-0101 §3 is the only honored shape; non-exact variants dropped at PARSE.
- `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (existing; introduced by earlier phase) — `*RuntimeFractionalPercent` field type for `filter_enabled` + `shadow_enabled`.
- `google.golang.org/protobuf/types/known/anypb` (existing; introduced by 07.1) — `*anypb.Any` typed_config carrier consumed by `New(tc, ctx)`.
- `google.golang.org/protobuf/proto` (existing; introduced by 07.1 perroute) — `proto.Message` interface used by the per-route `f.dcb.RequestRouteConfig()` callback's return type.
- `github.com/esalaine/envoy-go/internal/filter/http` (existing; introduced by phase 07.1, extended in phase 09 with FactoryCtx Stats + StatPrefix, extended in phase 10 with ResolveAllTiers / RequestRouteConfigsAllTiers / RegisterPerRouteValidator) — `FactoryCtx` (UNCHANGED in phase 12; the 3-field shape stays as-is per ADR-0100; phase 12 consumes `ctx.Stats` + `ctx.StatPrefix` — `ctx.StatPrefix` is the HCM-level conn-manager prefix, which is the namespace anchor for csrf's three counters per §11.6 confirmation — UNLIKE local_ratelimit which uses its own proto `cfg.StatPrefix` field), `HTTPFilter`, `HTTPFilterFactory`, `FilterInstanceFactory`, `StreamDecoderFilter`, `FilterHeadersStatus`, `FilterDataStatus`, `FilterTrailersStatus`, `Continue`, `DataContinue`, `TrailersContinue`, `StopIteration`, `OrderedHeaders`, `DecoderFilterCallbacks` (UNCHANGED in phase 12 — csrf consumes only the existing `SendLocalReply` method per ADR-0102 + the existing `RequestRouteConfig` method per ADR-0073; no new callback method introduced; csrf does NOT consume `RequestRouteConfigsAllTiers` since it is most-specific-override per ADR-0073), `HTTPRegistry` (UNCHANGED in phase 12 — csrf registers ONE `Register` call; the phase-10-introduced `RegisterPerRouteValidator` hook is NOT consumed by phase 12), `BuildPerRouteConfig` (UNCHANGED in phase 12 — `BuildPerRouteConfig` does NOT call any registered filter `New`; per `internal/filter/http/perroute.go:63-85` it `UnmarshalNew`'s each TPFC Any into a generic `proto.Message` and stores it. Phase 12's per-route TPFC handling therefore happens at REQUEST time: the filter calls `f.dcb.RequestRouteConfig()` which returns the most-specific per-route `proto.Message` (a `*csrfv3.CsrfPolicy`) OR nil; on non-nil, the filter calls a `buildPerRouteRuntime(perRoute, listenerStats) *runtimeConfig` helper that compiles the per-route `additional_origins` and reuses the listener-level `*filterStats` pointer; on nil, the filter uses its closure-captured listener-level `*runtimeConfig`. Per-route RC is built fresh on every per-route request — no caching since the operation is small + the `additional_origins` slice is bounded by config size — see planner-time decision 6), `PerRouteConfig.Resolve` (existing 3-tier most-specific-override accessor per ADR-0073; phase 12 reuses VERBATIM via the chain-managed `f.dcb.RequestRouteConfig()` callback).
- `github.com/esalaine/envoy-go/internal/filter/http/cors` (existing; the package-shape precedent csrf mirrors — TypeURL constant + New factory + filter struct + decoder + OnDestroy).
- `github.com/esalaine/envoy-go/internal/filter/http/fault` (existing; the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution + `cb.SendLocalReply + StopIteration` request-side terminal-replace pattern at fault.go:321; phase 12 mirrors verbatim modulo no-async-resume / no-timer / no-rng / no-overflow / different counter-name set / no encode-side filter implementation).
- `github.com/esalaine/envoy-go/internal/filter/http/localratelimit` (existing; tertiary precedent — closely-analogous `DecodeHeaders` body discipline modulo the per-request algorithm; phase 12 explicitly DIVERGES from local_ratelimit's stateful-per-route + independent-per-route-stats precedent per §11.9 amendment — phase 12 is data-only AND stat-sharing).
- `github.com/esalaine/envoy-go/internal/filter/http/header_mutation` (existing; quaternary precedent for the unmarshal-at-New + closure-capture-runtimeConfig + per-route TPFC parsing pattern).
- `github.com/esalaine/envoy-go/internal/stats` (existing; introduced by phase 06.1, extended in 09 with HCM-scoped fault stats per ADR-0061's SN2 internal-dot transform, extended in 11 with Rule SN9 + `NewCounterIfAbsent`) — `*Registry`, `NewCounter`, `Walk`, `flattenToProm` (UNCHANGED in phase 12 — csrf reuses the existing HCM-namespace SN2 rule `http.<HCM stat_prefix>.<rest>` which produces Prometheus base `envoy_http_<rest>` + label `envoy_http_conn_manager_prefix=<HCM stat_prefix>`; phase 12 does NOT extend SN1–SN9). `NewCounterIfAbsent` exists per ADR-0117 + ADR-0061 amendment but is NOT consumed by phase 12 since per-route stats are SHARED with listener-level — no post-Freeze idempotent registration needed.
- `github.com/esalaine/envoy-go/test/differential/fixture` (existing; extended in Task 9 with a new `BackendKind` enum value `HTTPCsrf BackendKind = 11` per planner-time decision 9).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0014's reference image AND the source of the SPEC §11.1–§11.11 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 12's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 12 — phase 12 touches no codec/framer/HPACK paths; the conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0014's reference (Envoy in a Docker container) — same harness as 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 fixtures consume; phase 12 does NOT extend the harness's optional driver-side interfaces.
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's csrf filter implementation; any third-party CSRF/origin-validation library. Test-side use is also forbidden. The `go.mod` post-12 must not list any new csrf-related runtime dependencies; the origin-extraction + comparison primitive is implemented in-tree from stdlib only (`net/url` + `strings`).

---

## Scope check — why phase 12 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 PLAN's component-table convention):

- `internal/filter/http/csrf/doc.go` ~25
- `internal/filter/http/csrf/csrf.go` ~280 (filter + factory + origin-parsing helpers + runtimeConfig + DecodeHeaders + filterStats + per-route runtime build)
- `internal/filter/http/csrf/csrf_test.go` ~180 (6 test groups per SPEC §6.5 + §14.1)
- `internal/filter/http/csrf/fuzz_test.go` ~40 (16th fuzzer)
- `cmd/envoy-go/main.go` one new `httpReg.Register(csrf.TypeURL, csrf.New)` line + matching import ~+3 = ~+3
- `test/fixtures/0014-http-csrf/` (NEW directory) — `envoy.yaml` ~75 + `envoy-go.yaml` ~75 + `expectations.yaml` ~50 + `README.md` ~60 + `driver/driver.go` ~180 + `backends/backend.go` ~30 = ~470
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPCsrf BackendKind = 11`) + doc-comment ~+15 = ~+15
- `test/differential/runner_test.go` blank-import addition + new `startHTTPCsrfBackend` spawn helper + switch case ~+25 = ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.csrf` subsection ~75 + §13.2 26→29 stat-table extension ~5 + §13.3 equivalence-matrix row ~3 + §13.4 `### Phase 12 forward-pointer notes` subsection ~25 = ~+108
- `docs/envoy-go/DECISIONS.md` (five ADRs ADR-0120..ADR-0124) ~+280 = ~+280
- `docs/envoy-go/ROADMAP.md` row `12` `in-progress → done` flip + (UNCHANGED) §9 family heading at line 56 ~+1 net = ~+1
- `docs/envoy-go/STATE.md` advance to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (NEW; lifecycle artefact) ~450 (per-task entry)
- `docs/envoy-go/phases/12-http-filter-csrf/REVIEW.md` (NEW; lifecycle artefact) ~150

**Production code: ~280 LoC (filter impl in `csrf.go`) + ~3 LoC main.go = ~283 LoC production + ~220 LoC tests (180 unit + 40 fuzzer) + ~470 LoC fixture YAML/Go + ~389 LoC docs ≈ ~1362 LoC total** (production-only ~283 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~283 LoC; task count below is **13**, comfortably under the 25 limit). The SPEC's anticipated 5-ADR cluster (ADR-0120..ADR-0124) lands across 13 tasks per the table at `## ADRs introduced by this plan` below; no task lands more than 2 ADRs simultaneously. SPEC §1.3 (per BRAINSTORM Decision 9 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 12 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~10–14 tasks per SPEC §1.4 estimate; this PLAN lands at 13 tasks (mid-bound — driven by csrf's smaller surface vs phase 11 since: no token-bucket primitive (saves 1 task); no per-route stats independence (saves 1 task — no `NewCounterIfAbsent` framework delta); no new SN flattening rule (saves 1 task); but adds a per-route TPFC parsing test task since shared-stats per-route is a NEW pattern worth its own validation step).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 / SPEC §1.4 would be `12.1 = listener-level filter MVP (Tasks 1–7)` and `12.2 = per-route TPFC + 6th fixture scenario (Tasks 8–13)`; SPEC §1.4 explicitly rejects the split since both halves stay well under the LoC threshold and the per-route discipline is a small extension of the listener-level work (data-only with shared stats — much smaller delta than phase 11's stateful-per-route extension which still kept its single-row position). PLAN concurs and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/csrf/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`CsrfPolicy` proto with 1-actively-consumed (`additional_origins[].StringMatcher.exact` non-empty values; non-exact variants dropped at PARSE per ADR-0101 §3) / 1-PGV-validated-not-honored (`filter_enabled` per §11.11 amendment — REQUIRED at parse-time, silent-ignored at runtime) / 1-deferred (`shadow_enabled` — optional at parse, never-shadow at runtime) field decomposition per ADR-0121); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (Continue allow path; StopIteration + SendLocalReply rejection path — ADR-0102 reuse from phase 09 fault precedent; no async-resume; no encode-side state — `HTTPFilter` value sets `Encoder: nil` per planner-time decision 2; no body / trailers states exercised); (d) the per-route discipline (per ADR-0073 wholesale-override; per-route TPFC entry → independent `additionalOrigins` slice but SHARED listener-level `*filterStats` pointer per ADR-0124; **diverges from phase 11 ADR-0117 precedent** which had independent per-route stats); (e) the host:port-only comparison algorithm + origin-extraction trichotomy + operator footgun per ADR-0122; (f) the cross-cutting ADR anchors (ADR-0120 / ADR-0121 / ADR-0122 / ADR-0123 / ADR-0124). Mirrors `internal/filter/http/cors/doc.go`-style brevity + `internal/filter/http/fault/doc.go` shape (~25 LoC precedent). Per SPEC §4.1. |
| `internal/filter/http/csrf/csrf.go` | NEW | Filter implementation — single-file orchestration. **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Unexported types (per SPEC §6.2 + §6.3):** `runtimeConfig` struct (2 fields per §6.2: `additionalOrigins []string` — verbatim values from surviving `additional_origins[].StringMatcher.exact` entries, `stats *filterStats` — listener-level pointer; per-route runtimeConfig SHARES this pointer per §11.9); `filterStats` struct (3 `*atomic.Int64` fields: `requestValid`, `requestInvalid`, `missingSourceOrigin`); `filter` struct (`rc *runtimeConfig` (closure-captured listener-level reference) + `dcb envoyhttp.DecoderFilterCallbacks`). **Helpers:** `buildRuntimeConfig(c *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error)` (validates `c.FilterEnabled != nil && c.FilterEnabled.DefaultValue != nil` per §11.11 amendment; compiles `additional_origins[]` per §6.2 ADR-0101 §3 discipline; if `listenerStats` is non-nil reuses it (per-route build path), else allocates a fresh `*filterStats` (listener-level build path) — see planner-time decision 6); `newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats` (constructs the 3-counter set via `reg.NewCounter("http." + hcmStatPrefix + ".csrf." + counter)` per SPEC §6.6); `sourceOriginValue(headers http.Header) string` (origin-extraction trichotomy per §6.4 + §11.2 amendment); `targetOriginValue(headers http.Header) string` (constructs `<synthetic-scheme>://<:authority/Host>` per planner-time decision 8 — synthetic `http://` prefix is sufficient since scheme is stripped per §11.3 amendment); `hostAndPort(absoluteURL string) string` (parses URL via `net/url.Parse`; returns `u.Host` if parse OK + `u.Host != ""`, else returns `absoluteURL` verbatim per §6.4 + §11.3 source-of-truth); `evaluate(rc *runtimeConfig, source, target string) (allow bool, missing bool)` (the disposition table per §6.4 — empty source → missing; source matches any `additionalOrigins[]` entry → allow; source matches target → allow; else → reject); `buildPerRouteRuntime(perRoute *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error)` (the per-route TPFC builder reusing `buildRuntimeConfig` with the listener-level stats injected). **DecodeHeaders body** (per SPEC §6.5): read `:method` via `headers.Get(":method")` (per cors precedent at `cors.go:206` + chain-injection at `internal/filter/hcm/h2dispatch.go:214`); short-circuit `Continue` if method ∉ `{POST, PUT, DELETE, PATCH}`; resolve effective `*runtimeConfig` via `f.dcb.RequestRouteConfig()` (if non-nil, type-assert to `*csrfv3.CsrfPolicy` + call `buildPerRouteRuntime(perRoute, f.rc.stats)` to construct a per-route rc with shared stats; if nil, use `f.rc` directly); compute target + source per the helpers; evaluate; on allow → increment `rc.stats.requestValid` + return Continue; on missing → increment `rc.stats.missingSourceOrigin` + invoke `f.dcb.SendLocalReply(403, "Invalid origin", OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})` + return StopIteration; on reject → increment `rc.stats.requestInvalid` + same SendLocalReply + StopIteration. **Pass-through methods:** `SetDecoderCallbacks(cb)` stores `f.dcb = cb`; `OnDestroy()` no-op; `DecodeData([]byte, bool)` returns `DataContinue`; `DecodeTrailers(http.Header)` returns `TrailersContinue`. **NO encode-side methods** — the `HTTPFilter` value returned by the factory sets `Decoder: f, Encoder: nil` per planner-time decision 2. Per SPEC §6.1–§6.5. ~280 LoC. |
| `internal/filter/http/csrf/csrf_test.go` | NEW | Unit tests per SPEC §14.1 covering 6 test groups: **Group 1** — `TestNew_*` factory: `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_FilterEnabledNil_RejectAtParseTime` (verifies `cfg.FilterEnabled == nil` produces a non-nil error per §11.11), `TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime` (verifies nested `cfg.FilterEnabled.DefaultValue == nil` also rejects per §11.11), `TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored` (verifies `default_value=0/HUNDRED` boots successfully — the percentage value is silent-ignored at runtime per §1.1 amendment 3; runtime is always-100%-active), `TestNew_FilterEnabledHundredPercent_Accepted`, `TestNew_ShadowEnabledAbsent_Accepted` (no parse-time validation per §11.11 probe #3), `TestNew_ShadowEnabledPresent_SilentIgnored`, `TestNew_HappyPath_AdditionalOriginsCompiled`. **Group 2** — `additional_origins` parse-time discipline (per §11.7 + §11.8 + ADR-0101 §3): `TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse` (table-driven across `prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case` — verifies non-exact entries do NOT survive into the resulting `runtimeConfig.additionalOrigins`; runtime carries only surviving exact entries), `TestNew_AdditionalOrigins_EmptyExactValue_Dropped` (verifies empty-value `exact` entries are dropped per ADR-0101 §3 "non-empty Exact value is honored" qualifier), `TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm` (verifies surviving entries are stored verbatim — no normalization at parse time; the operator's `host[:port]` form is preserved byte-for-byte). **Group 3** — `DecodeHeaders` non-modifying methods (per §11.1): `TestDecodeHeaders_NonModifyingMethods` parametrized over `GET`, `HEAD`, `OPTIONS`, `TRACE` plus `PROPFIND` (custom verb); all return `Continue` immediately, no counter increments, no origin parsing invoked. **This subsumes scenario 6 from BRAINSTORM Q5 dialogue (the GET-passthrough not-in-fixture-0014 unit-only coverage).** **Group 4** — origin-extraction trichotomy (per §11.2): `TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback` (POST with `Origin: null` + valid Referer → `missing_source_origin +1` + reject; verifies NO Referer fallback); `TestDecodeHeaders_OriginEmpty_RefererFallback` (POST with empty `Origin:` + matching Referer → `request_valid +1` + Continue); `TestDecodeHeaders_OriginAbsent_RefererFallback` (POST with no `Origin:` header + matching Referer → `request_valid +1`); `TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin`; `TestDecodeHeaders_OriginUnparseable_VerbatimUsed` (POST with `Origin: not-a-url` + matching Referer → `request_invalid +1` + reject; verifies verbatim string `not-a-url` mismatches target so request rejects, AND verifies NO Referer fallback). **Group 5** — host:port-only equality (per §11.3 + §11.7 + §11.8): `TestDecodeHeaders_SameOrigin_HostPortMatch` (POST with `Origin: http://127.0.0.1:8080` against `Host: 127.0.0.1:8080` → allow); `TestDecodeHeaders_CrossOrigin_HostMismatch` (POST with `Origin: https://evil.test` → reject); `TestDecodeHeaders_AdditionalOriginsExactMatch` (POST with `Origin: https://app.example.test` against `additional_origins=["app.example.test"]` → allow; scheme stripped); `TestDecodeHeaders_NoCaseFolding_UppercaseRejected` (POST with `Origin: HTTPS://APP.EXAMPLE.TEST` → reject; case preserved); `TestDecodeHeaders_NoDefaultPortStripping_PortMismatch` (POST with `Origin: https://app.example.test:443` against `additional_origins=["app.example.test"]` → reject); `TestDecodeHeaders_TrailingSlashStripped_Allow` (POST with `Origin: https://app.example.test/` → matches `app.example.test` since URL parser drops path); `TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches` (POST with `Origin: https://app.example.test` against `additional_origins=["https://app.example.test"]` (full-URL form, the operator footgun) → reject; verifies the §11.8 amendment that `additional_origins[].exact` matches `host[:port]` form NOT full URL with scheme). **Group 6** — per-route override + shared stats (per §11.9): `TestDecodeHeaders_PerRouteOverride_DataReplaced` (per-route TPFC with `additional_origins=["route-only.test"]` against listener `additional_origins=["app.example.test"]`; verify per-route request with `Origin: https://route-only.test` allowed AND default-route request with same Origin rejected); `TestDecodeHeaders_PerRouteStatsShared` (verify counter increments on per-route path AGGREGATE with listener-level — same `*atomic.Int64` pointer; the shared-stats invariant per §11.9 amendment + ADR-0124); `TestStats_ThreeCountersUnderHCMStatPrefix` (verifies the registry's NewCounter calls produce the expected internal hierarchical-dotted names per SPEC §6.6 — `http.<HCM stat_prefix>.csrf.{request_valid, request_invalid, missing_source_origin}`). ~180 LoC. |
| `internal/filter/http/csrf/fuzz_test.go` | NEW | `FuzzCsrfPolicyConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the csrf filter's `New` factory is a parser. ~40 LoC; 30s budget per ADR-0018; **sixteenth fuzzer overall** (post-11's fifteenth `FuzzLocalRateLimitConfigParse`). Optional secondary corpus: pre-seeded `additional_origins` with malformed StringMatcher patterns (those dropped at parse time per ADR-0101 §3 discipline; should not panic during the parse-time filter step). |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(csrf.TypeURL, csrf.New)` registration inserted after the existing `httpReg.Register(cors.TypeURL, cors.New)` line (currently line 115) and before the `httpReg.Register(envoygotest.TypeURL, envoygotest.New)` line (currently line 116). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/csrf"` alphabetically among the existing filter-package imports (currently lines 28-33: `cors, envoygotest, fault, header_mutation, localratelimit, router` → `cors, csrf, envoygotest, fault, header_mutation, localratelimit, router`). Per the BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline (codified at phase-09 brainstorm time + reaffirmed at phases 10 + 11), the resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(csrf.TypeURL, csrf.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Register(localratelimit.TypeURL, localratelimit.New); header_mutation.RegisterPerRouteValidator(httpReg); httpReg.Freeze()`. **No other wiring changes** — csrf is HTTP-only, no listener/cluster/drain manager threading; no per-route-validator registration call (csrf has no per-route invariants requiring boot-time validation — per-route TPFC parsing happens at HCM-build via `BuildPerRouteConfig`'s generic `UnmarshalNew`, and the filter applies its PGV-mirror validation at `New` time for both listener-level and per-route entries via the same `buildRuntimeConfig` helper). ~+3 LoC delta (1 import line + 1 register line). |
| `test/fixtures/0014-http-csrf/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. The runner-side blank-import lives at `test/differential/runner_test.go` per the existing 0010 / 0011 / 0012 / 0013 convention. |
| `test/fixtures/0014-http-csrf/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port resolved at boot by the runner; **ONE listener `l_main` per planner-time decision 7** — single listener with two routes (`/route-only` per-route TPFC + `/` default); cluster `c_backend` STRICT_DNS pointing at the harness backend via `host.docker.internal` per ADR-0010). Listener-level `CsrfPolicy`: `additional_origins=[{exact: "app.example.test"}]` (host:port form per §11.8 amendment — NO scheme prefix); `filter_enabled.default_value: {numerator: 100, denominator: HUNDRED}` explicit per §11.11 amendment (Envoy PGV-rejects boot if absent — non-negotiable). Per-route TPFC on `/route-only`: `additional_origins=[{exact: "route-only.test"}]`; `filter_enabled.default_value: {numerator: 100, denominator: HUNDRED}` explicit (matches listener-level discipline). `shadow_enabled` OMITTED on both sides per §11.11 probe #3 baseline (Envoy permits omission; envoy-go also accepts). http_filters chain on the listener: `[envoy.filters.http.csrf, envoy.filters.http.router]`. ~75 LoC. |
| `test/fixtures/0014-http-csrf/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo cluster type (STATIC instead of STRICT_DNS) + admin/listener port values resolved at boot by the runner. Both `filter_enabled` fields are PRESENT in envoy-go.yaml even though envoy-go silent-ignores the percentage value (per SPEC §2.1 cluster filter-enabled clause / §13.4 — envoy-go's silent-ignore is equivalent to "always-100%" under MVP) — the field presence ensures byte-equivalent config-load behavior across Envoy and envoy-go (Envoy PGV-validates presence + envoy-go's `New` factory PGV-mirrors). `shadow_enabled` OMITTED on both sides. ~75 LoC. |
| `test/fixtures/0014-http-csrf/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1: scenario 1 (same-origin POST allowed) → 200 + backend body passthrough; counter delta `request_valid +1`; scenario 2 (cross-origin POST rejected) → 403 + body byte-exact `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <allow-listed>`, `server: envoy`); counter delta `request_invalid +1`; scenario 3 (additional_origins exact match) → 200 + backend body; counter delta `request_valid +1` (note: `additional_origins` entry MUST be `app.example.test` host:port form per §11.8 amendment, NOT `https://app.example.test`); scenario 4 (no source-origin) → 403 + `Invalid origin` + 4-header set; counter delta `missing_source_origin +1`; scenario 5 (Referer fallback) → 200 + backend body; counter delta `request_valid +1`; scenario 7 (per-route wholesale-override): (a) `POST /route-only Origin: https://route-only.test` → 200; (b) `POST / Origin: https://route-only.test` → 403 + `Invalid origin` (matches neither listener-default nor `app.example.test`); counter deltas: `request_valid +1` (from 7a) AND `request_invalid +1` (from 7b) — **counters AGGREGATE across listener + per-route per §11.9 amendment** (NO independent per-route counter series, diverges from phase 11). Final stats scrape after all 7 requests: `request_valid=4`, `request_invalid=2`, `missing_source_origin=1`; Prometheus form: `envoy_http_csrf_{request_valid|request_invalid|missing_source_origin}{envoy_http_conn_manager_prefix="ingress_csrf"}`. **No timing tolerances** (csrf is purely synchronous — no analog to phase 11 fixture 0013 scenario 3's ±10ms refill boundary). Cross-refs SPEC §7.1 + §13.1 + ADR-0122 + ADR-0123 + ADR-0124. ~50 LoC. Per SPEC §4.3. |
| `test/fixtures/0014-http-csrf/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + 6-scenario list (per SPEC §7.1) + single-listener bootstrap discipline (per planner-time decision 7: all 6 scenarios run against the single listener `l_main` with two routes — no per-scenario teardown) + Envoy-deviation note (none — csrf is a normal HTTP filter; no SIGTERM/drain divergence) + the `filter_enabled.default_value: 100/HUNDRED` discipline note (SPEC §11.11 amendment — Envoy PGV-rejects boot if absent) + the operator footgun note (`additional_origins[].exact` is host:port form NOT full URL with scheme per §11.8 amendment) + the per-route stats-shared note (counters AGGREGATE across listener + per-route per §11.9 amendment — diverges from phase 11) + planner-time-decision cross-references. ~60 LoC. Per SPEC §4.3. |
| `test/fixtures/0014-http-csrf/driver/driver.go` | NEW | Go driver implementing the SPEC §7.1 + §7.2 6-scenario sequential orchestration via the single-listener topology per planner-time decision 7. **Driver shape:** `package driver`; `init()` calls `fixture.RegisterFixture("0014-http-csrf", &csrfDriver{})`; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPCsrf` (the new enum value added in Task 9); implements the SINGLE-listener fixture interface (`fixture.Driver` per the cors / fault / header_mutation precedent — NOT the `MultiListenerDriver` introduced by phase 07.2 + used by phase 11). `ReferenceBootstrap` / `SubjectConfig` templates `envoy.yaml` / `envoy-go.yaml` substituting the listener-port placeholder + backend port; the bootstrap is rendered ONCE. `DriveReference` / `DriveSubject` issue ALL SIX scenarios (which expand to 7 HTTP requests since scenario 7 has two sub-requests 7a + 7b) in ONE call: scenario 1 (POST `/` with `Origin: http://127.0.0.1:<port>` — same-origin); scenario 2 (POST `/` with `Origin: https://evil.test` — cross-origin); scenario 3 (POST `/` with `Origin: https://app.example.test` — additional_origins match); scenario 4 (POST `/` with no Origin no Referer — missing); scenario 5 (POST `/` with no Origin + `Referer: http://127.0.0.1:<port>/somepage`); scenario 7a (POST `/route-only` with `Origin: https://route-only.test` — per-route allow); scenario 7b (POST `/` with `Origin: https://route-only.test` — listener reject); per-probe captures status + body + headers (rejection path) + post-scenario comparison via `CompareBytes`; final `/stats/prometheus` scrape captures the 3 csrf counters AND the tag-extracted Prometheus label `envoy_http_conn_manager_prefix="ingress_csrf"` for differential-equivalence assertion. **No timing tolerances** — all scenarios run in microseconds; counter scrape is post-hoc via the driver's standard `/stats/prometheus` capture. ~180 LoC. Per SPEC §7.2. |
| `test/fixtures/0014-http-csrf/backends/backend.go` | NEW | Minimal Go HTTP backend bound to a runner-allocated port. Mirrors `test/fixtures/0011-http-fault/backends/backend.go` exactly + `test/fixtures/0013-http-local-ratelimit/backends/backend.go`: `/` endpoint serves a fast `200 OK` with body `"backend\n"` (8 bytes); response carries one fixed `Content-Type: text/plain` and one `Content-Length: 8` header. No special handling for `/route-only` (the csrf decision happens in Envoy/envoy-go BEFORE the upstream call; the backend is unreachable on rejection paths). Accepts a `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper. ~30 LoC. Per SPEC §7.4. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPCsrf BackendKind = 11` after the existing `HTTPLocalRateLimit BackendKind = 10` (introduced by phase 11). Doc-comment notes: "HTTPCsrf is an out-of-process HTTP/1.1 backend: the runner spawns `test/fixtures/0014-http-csrf/backends/backend.go` on the pre-allocated port. The backend serves `/` with body `backend\n` (8 bytes; Content-Type: text/plain; Content-Length: 8). No TLS. Introduced by fixture 0014-http-csrf (phase 12 Task 9). Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0014-http-csrf/driver"` (insert in alphabetical order, after the `0013-http-local-ratelimit` blank-import). (b) Extend the `kind` switch in `runFixture` with a new case `fixture.HTTPCsrf` mirroring the `HTTPLocalRateLimit` block: spawn via `startHTTPCsrfBackend`. (c) Add new spawn helper `startHTTPCsrfBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` mirroring `startHTTPLocalRateLimitBackend` from phase 11: `exec.CommandContext(ctx, "go", "run", "./test/fixtures/0014-http-csrf/backends", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.csrf` subsection inserted under existing `## HTTP filter chain` umbrella AFTER the `### envoy.filters.http.local_ratelimit` subsection at line 1008 landed by phase 11 (per §13.1; ~75 LoC); (b) `## Stat-name mapping ### 26-name table` extends to **29-name table** with the three new csrf counter rows per §13.2 (~5 LoC; **NO new tag-extractor preamble** since csrf reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace SN2 extractor — UNLIKE phase 11 which added the `envoy_local_http_ratelimit_prefix` filter-specific tag-extractor as Rule SN9; **NO new SN flattening rule**); (c) `## Equivalence Matrix` new csrf-filter row (per §13.3; ~3 LoC); (d) NEW `### Phase 12 forward-pointer notes` subsection appended to existing `## Forward-pointer notes` section per §13.4 (~25 LoC) — covers the 3-item deferral list (`filter_enabled` PGV-required + percentage-gating deferred to Runtime + hot restart family per §11.11 amendment; `shadow_enabled` shadow-mode evaluation deferred per §2.1.3; `additional_origins[].StringMatcher` non-exact variants dropped at PARSE per ADR-0101 §3 per §2.1.1) + the operator footgun callout (`additional_origins[].exact` is host:port form NOT full URL with scheme per §11.8 amendment) + the no-new-tag-extractor note + the per-route stats-shared note (counters AGGREGATE; diverges from phase 11 per §11.9 amendment). ADR-0052 in-place edit authorisation carries forward. ~+108 LoC total. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append five new ADRs ADR-0120..ADR-0124 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). The 7-section ADR-0001 template applies to each (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). **NO inline supersessions / amendments** for phase 12 — UNLIKE phases 10 + 11 which each amended ADR-0073 (phase 10 ADR-0110 multi-tier; phase 11 ADR-0117 stateful-per-route), phase 12 inherits ADR-0073 verbatim with NO third amendment (csrf is data-only AND most-specific-override; falls within the original wholesale-override semantics). UNLIKE phase 11 which extended ADR-0061 with Rule SN9 (ADR-0118 amendment paragraph), phase 12 does NOT extend ADR-0061 (csrf reuses the existing SN2 rule for HCM-namespace stats — no new SN rule needed per §11.6 confirmation). UNLIKE phase 10 which dropped a would-be ADR-0114 stats-absence per SPEC §8.1 consolidation, phase 12 has NO consolidation candidates — all 5 ADRs are load-bearing per SPEC §8.1. ~+280 LoC total (five ADRs; no amendment paragraphs). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `12` `in-progress → done` flip AT the phase-done commit. The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 12 artefacts (per ADR-0106 no-sibling-stub discipline). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 3 (PLAN drafting — this PLAN landing flips state 3 → 4 in the orchestrating session's STATE.md edit), 4 (PLAN execution — Tasks 1–11 land production code + fixture; STATE stays at 4), 5 (verification — Task 12 lands BEHAVIOR_CONTRACT/ADRs/six-gate verification; STATE flips 4 → 5), 6 (review — Task 13 REVIEW.md per requesting-code-review skill; STATE flips 5 → 6 then to `awaiting next planning`); `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner. |
| `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..11 PROGRESS.md structure. The preamble enumerates the five anticipated ADRs ADR-0120..ADR-0124 + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 9 items below — D1–D4 from SPEC §12 plus 5 PLAN-emerging items). |
| `docs/envoy-go/phases/12-http-filter-csrf/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 cadence; populates per the requesting-code-review skill. Phase 12 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 12. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's four deferred decisions before implementation; this PLAN settles all four plus five that emerged at PLAN-drafting time (items 5, 6, 7, 8, 9 below). The nine resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)` per the cors + fault + header_mutation precedents; encode side ABSENT.** Per SPEC §12 D1 + survey of existing patterns: `internal/filter/http/cors/cors.go:55` defines `func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }`; `internal/filter/http/fault/fault.go` follows the same pattern; `internal/filter/http/header_mutation/header_mutation.go` follows the same pattern; `internal/filter/http/localratelimit/local_ratelimit.go` follows the same pattern. The framework's per-stream state machine (per `internal/filter/http/chain.go`) calls `SetDecoderCallbacks` once per stream as part of the chain construction; the filter stores the callback reference for later use during `DecodeHeaders` (for both `SendLocalReply` and `RequestRouteConfig`). **Phase 12 does NOT implement `SetEncoderCallbacks` or any encode-side methods** — the `HTTPFilter` value returned by the factory closure sets `Decoder: f, Encoder: nil` per planner-time decision 2. This is the first §9 production filter that is decoder-only (cors + local_ratelimit + fault + header_mutation all set both `Decoder: f, Encoder: f` even when only one side is needed, for chain-of-conformance). Phase 12 deliberately departs since: (a) csrf has zero encode-side responsibilities (the rejection path uses `SendLocalReply` which enters the encode chain at `filter[len-1]` per ADR-0075 — csrf's filter does NOT participate in that encode iteration; the chain framework handles the localReply chain entry); (b) saving the encode-side struct method implementations + callback field reduces the filter's surface area and makes the read-only-after-`New` invariant more obvious. *Anchored: SPEC §12 D1; cors.go:55; fault.go; header_mutation.go; localratelimit/local_ratelimit.go; types.go HTTPFilter struct allowing `Encoder: nil` per ADR-0071.*

2. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil` (decoder-only).** Per the `internal/filter/http/types.go` `HTTPFilter` struct definition: `type HTTPFilter struct { Name string; Decoder StreamDecoderFilter; Encoder StreamEncoderFilter }` — comment "nil for encoder-only filters" / "nil for decoder-only filters" makes both nilable. The chain framework's `RunDecodeHeaders` / `RunEncodeHeaders` iterators dispatch only on non-nil sides per ADR-0071. csrf has no encode-side state and no encode-side responsibilities; setting `Encoder: nil` (a) reduces struct surface area, (b) saves implementing the `StreamEncoderFilter` method set on `*filter` (no `EncodeHeaders` / `EncodeData` / `EncodeTrailers` / `SetEncoderCallbacks` / OnDestroy duplication), (c) makes the decoder-only nature self-documenting. cors / fault / header_mutation / localratelimit each set `Encoder: f` because each has SOME encode-side participation (cors injects response headers; fault has no encode but sets it for symmetry; header_mutation injects/removes response headers; localratelimit has no encode but sets it for symmetry). Phase 12 is the first §9 family-row that is structurally decoder-only AND chooses to express that structurally. *Anchored: types.go HTTPFilter; ADR-0071 iteration protocol; SPEC §6.3.*

3. **D2 — URL-parse semantics for `hostAndPort()` = `net/url.Parse` + verbatim-string-on-parse-failure (mirrors Envoy's `Http::Utility::Url::initialize` for common cases).** Per SPEC §12 D2 + §6.4. Phase 12 uses `net/url.Parse(absoluteURL)` from Go stdlib; if parse succeeds AND `u.Host != ""` → return `u.Host` (which is the `host[:port]` form per the URL's authority component); else return `absoluteURL` verbatim (matches Envoy's `hostAndPort()` source-of-truth quoted at SPEC §11.2 — `if (url.initialize(absolute_url, /*is_connect=*/false)) { return std::string(url.hostAndPort()); } return std::string(absolute_url);`). Edge cases settled: (i) `//host:port` (no scheme) — `net/url.Parse` returns Host="" → verbatim string (matches Envoy parse failure per §11.3 probe G); (ii) `127.0.0.1:port` (no `://`) — `net/url.Parse` parses as opaque (returns Host="") → verbatim string (matches Envoy per §11.3 probe K — the verbatim string then matches target hostAndPort if equal); (iii) IPv6 literal `[::1]:8080` — `net/url.Parse` parses correctly; `u.Host = "[::1]:8080"`. The unit-test group 5 (`TestDecodeHeaders_*HostPort*`) covers the common cases; the §11 empirical pins (probes K, G, J) are the regression baseline. PLAN does NOT introduce a custom hostAndPort helper; relies on stdlib + the verbatim-on-failure fallback. *Anchored: SPEC §12 D2 + §6.4 + §11.2 source-of-truth + §11.3 probe G/K observations; Go stdlib `net/url.Parse` documentation.*

4. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording (option (b)) per phase 11 ADR-0115 precedent.** Per SPEC §12 D3 + §6.1 + §11.11. Phase 12 emits its own clear-text error messages from the `New` factory rather than mirroring Envoy's PGV envelope verbatim (option (a)). Specifically: `if cfg.GetFilterEnabled() == nil { return nil, errors.New("csrf: filter_enabled is required") }` and `if cfg.GetFilterEnabled().GetDefaultValue() == nil { return nil, errors.New("csrf: filter_enabled.default_value is required") }`. Phase 11's ADR-0115 set the precedent — used option (b) for the 50ms `fill_interval` validation with the verbatim Envoy wire-equivalence note as a deliberate exception. Phase 12 has no analogous boot-log byte-equivalence claim (the differential fixture asserts request-time wire shape, NOT boot-log byte equivalence); Envoy's PGV envelope wording (`CsrfPolicyValidationError.FilterEnabled: value is required`) is descriptive but tied to PGV machinery envoy-go does not host. envoy-go's own wording is operator-friendlier (no opaque PGV-envelope prefix) and consistent with phase 11's ADR-0115 pattern. *Anchored: SPEC §12 D3; phase 11 ADR-0115 envoy-go-own-wording precedent; the 50ms fill_interval verbatim-wording exception is filter-specific to local_ratelimit and does NOT carry forward.*

5. **D4 — Per-route stats wiring mechanism = OPTION (b) per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper called from `DecodeHeaders` at request time.** Per SPEC §12 D4 + §6.6 + §11.9 + ADR-0124. Phase 12 builds the per-route `*runtimeConfig` AT REQUEST TIME (inside `DecodeHeaders`) when `f.dcb.RequestRouteConfig()` returns a non-nil `*csrfv3.CsrfPolicy`. The per-route builder is a small helper `buildPerRouteRuntime(perRoute *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error)` that: (a) PGV-mirror-validates the per-route entry's `filter_enabled` field (same discipline as listener-level — non-nil + non-nil DefaultValue); (b) compiles `additional_origins[]` per ADR-0101 §3 (drops non-exact + empty-exact); (c) constructs a fresh `*runtimeConfig{additionalOrigins: compiled, stats: listenerStats}` — the `stats` pointer is the listener-level closure-captured pointer per §11.9. **Why option (b) over (a) and (c):** option (a) — passing `*filterStats` via `FactoryCtx` extension — would require a new field on `FactoryCtx` and would force every filter to opt-in to the new field even though only csrf needs it; rejected as over-engineering. Option (c) — `NewCounterIfAbsent` re-registration at per-route — is what phase 11 used for INDEPENDENT per-route stats; phase 12 SHARES stats with listener-level so re-registration is the WRONG pattern (would create three counters at the listener-level prefix and re-fetch them via `NewCounterIfAbsent`, which would return the same pointers, but the round-trip is wasted work + obscures the shared-stats invariant). Option (b) makes the shared-stats invariant structurally explicit: the per-route builder takes the listener-level `*filterStats` as input and reuses it verbatim — there is no second registration call site. **Per-route validation timing:** per-route entries' `filter_enabled` PGV-mirror validation happens at REQUEST TIME (first request that resolves to that per-route entry — caught by the `buildPerRouteRuntime` check). This diverges from listener-level (parse-time at boot). The divergence is acceptable because: (i) Envoy itself validates per-route entries via PGV at boot (so misconfigured per-route entries fail boot on the reference side); (ii) envoy-go's `BuildPerRouteConfig` (`internal/filter/http/perroute.go:63-85`) is generic and does NOT re-invoke filter `New` at boot — phase 10's `RegisterPerRouteValidator` is the only mechanism for boot-time per-route validation, and it requires opt-in per filter; phase 12 chooses NOT to opt-in for simplicity since the request-time validation produces a 500 (or equivalent) on the FIRST request rather than a boot failure. The differential fixture 0014 has well-formed per-route configs on both sides; misconfigured per-route validation is OUT of fixture scope. **No caching needed** since the per-route `additional_origins` slice is bounded by config size (typically 1-10 entries) and the comparison loop is O(n) byte-equality — the request-time build cost is dominated by the URL parse, not the slice compile. (Future optimization: if profiling shows the per-route build is hot, add a `sync.Map` keyed by `*csrfv3.CsrfPolicy` proto pointer; but for MVP this is YAGNI.) *Anchored: SPEC §12 D4 + §6.6 + §11.9 + ADR-0124; phase 11 lazy-cache + `NewCounterIfAbsent` precedent (rejected as wrong pattern for shared-stats).*

6. **PLAN-emerging — File-split decision = SINGLE-FILE `csrf.go` (no `origin.go` split).** Per SPEC §4.1 + §6.1 PLAN-author option ("PLAN author may split `csrf.go` into `csrf.go` + `origin.go` (origin-parsing helpers) for readability"). The origin-parsing helpers (`sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`) are tightly coupled to the `DecodeHeaders` body discipline and total ~80 LoC across the 4 helpers. Splitting into a sibling `origin.go` would: (a) save the reader from scrolling within `csrf.go` (~280 LoC), (b) make the helpers' reusability visible (they could in principle be reused by future origin-aware filters like `cors` extensions or `compression` content-type validators). However, neither benefit applies to phase 12: (i) ~280 LoC stays under the project's general 200-300 LoC mental-model threshold (similar to fault.go ~430 LoC + cors.go ~250 LoC which both stay single-file); (ii) no future filter is anticipated to reuse the host:port-only equality helpers — if/when such a filter lands, the helpers can be promoted to a shared package then. The single-file approach mirrors cors.go (which keeps everything in one file at ~250 LoC because the filter is a single integrated unit with no separable primitive — same applies to csrf). DIVERGES from phase 11's `local_ratelimit.go` + `bucket.go` split because the `tokenBucket` was a separable primitive; csrf's helpers are not. *Anchored: SPEC §4.1 PLAN-author option; project code-quality discipline; cors single-file precedent.*

7. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with TWO ROUTES (`/` default + `/route-only` per-route TPFC).** Per SPEC §7.1 + §7.3. Phase 12's 6 scenarios all run against the SAME listener with the same `additional_origins` listener-level config; only scenario 7 (per-route override) consults the per-route TPFC on `/route-only`. UNLIKE phase 11's 4-listener topology (`l_s1`, `l_s2`, `l_s3`, `l_per_route`) which was driven by per-scenario distinct bucket parameters (each scenario needed its own `max_tokens` / `fill_interval` to demonstrate the rate-limit decision), phase 12's scenarios all use the same `additional_origins` config (the only varying input is the request's `Origin`/`Referer` headers + path). The single-listener topology fits the existing `fixture.Driver` contract (NOT `MultiListenerDriver`) — same as fault 0011 + cors 0007a/0007b + header_mutation 0012. Saves driver complexity (no per-scenario port allocation; no `DriveSubjectMulti` / `DriveReferenceMulti` orchestration). Differential gate scope is unchanged — all 6 scenarios run sequentially against the same boot. *Anchored: SPEC §7.1 + §7.3 + §7.2 (the SPEC's bootstrap fragment shows a single listener with two routes); phase 09 / 10 / 11 single-listener precedents (0011 / 0012; phase 11 deviated for per-scenario bucket distinctness which is not phase 12's case).*

8. **PLAN-emerging — `:scheme` synthesis for `targetOriginValue` = USE A SYNTHETIC `http://` PREFIX (no framework extension).** Per SPEC §6.4 + §11.3 amendment. The SPEC's pseudo-code for `targetOriginValue` reads `:scheme` via `headers.Get(":scheme")` and falls back to a TLS-derived scheme; this assumes a `DownstreamTLS()` callback OR a chain-injected `:scheme` header. Survey of the existing codebase: (a) `:method` IS chain-injected by `internal/filter/hcm/h2dispatch.go:214` for H2 (and presumably H1 via similar injection); (b) `:scheme` is NOT currently chain-injected — only present on H2 paths via `internal/filter/hcm/h2/stream.go:339` (handled inside the codec, NOT propagated to the filter chain headers map); (c) NO `DownstreamTLS()` callback exists on `DecoderFilterCallbacks` (verified at `internal/filter/http/callbacks.go`). **Resolution:** since the §11.3 amendment establishes that scheme is computed only to make the URL parseable and is then stripped via `hostAndPort()`, csrf does NOT need the actual scheme — it only needs ANY non-empty scheme prefix to make the URL parser accept the input. Phase 12's `targetOriginValue` therefore prepends a synthetic `http://` literal to the `:authority`/`Host` value: `targetURL := "http://" + hostHeader; return hostAndPort(targetURL)`. The URL parser sees `http://127.0.0.1:8080` and returns `Host = "127.0.0.1:8080"` (scheme stripped); the byte-equality check then compares against the source's `host[:port]`. **No framework extension needed.** **No `:scheme` injection needed.** **No `DownstreamTLS()` callback needed.** This is the cleanest possible resolution and produces byte-equivalent behavior with reference Envoy because reference Envoy ALSO discards the scheme via its own `hostAndPort()` helper (per §11.3 source-of-truth: `hostAndPort()` strips the scheme prefix). The synthetic `http://` prefix is a pure parsing convenience — it never appears in any output, never enters the equality check, never leaks to operators. The only requirement is that `Host`/`:authority` is a parseable host[:port] form (which it is for all real-world requests). *Anchored: SPEC §6.4 + §11.3 amendment; framework survey at `internal/filter/http/callbacks.go` + `internal/filter/hcm/h2dispatch.go:214`; cors precedent at `cors.go:206` for `:method` access.*

9. **PLAN-emerging — BackendKind enum value = `HTTPCsrf BackendKind = 11`** (continues existing naming convention; next value after phase 11's `HTTPLocalRateLimit BackendKind = 10`). Doc-comment matches the format used for `HTTPLocalRateLimit`. *Anchored: phase 11 PLAN planner-time decision 9 precedent; existing enum at `test/differential/fixture/fixture.go:191-211`.*

These nine decisions are reproduced verbatim in `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The five ADRs anticipated by SPEC §8 (ADR-0120..ADR-0124). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The five ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0120 | `internal/filter/http/csrf/` package shape (TypeURL + New + filter struct + decoder-only `HTTPFilter` value with `Encoder: nil`; **single-token directory matching cors precedent** — no underscore needed since the proto type-name is already a single token; mirrors cors/fault discipline; rationale: aligns with cors + fault whose proto type-names were already single tokens — preserves the existing 4-of-6-filters discipline; phase 11's `localratelimit/` no-underscore departure from header_mutation's underscore-preserving pattern was specific to the multi-token proto type-name `LocalRateLimit`; csrf's single-token `CsrfPolicy` removes the choice) + extension-registry registration line + boot-time `httpReg.Register(csrf.TypeURL, csrf.New)` | Task 2 (`internal/filter/http/csrf/{doc.go,csrf.go}` package skeleton first lands; the boot registration code lands in Task 6 but ADR-0120 anchors at Task 2 because that's the first-use site that justifies the package shape per ADR-0044). |
| ADR-0121 | `runtimeConfig` shape + 1-consumed/1-PGV-validated-not-honored/1-deferred field decomposition (`additional_origins[].StringMatcher.exact` non-empty values consumed; non-exact variants dropped at PARSE per ADR-0101 §3; `filter_enabled` PGV-validated at parse-time per §11.11 amendment but silent-ignored at runtime; `shadow_enabled` optional at parse + silent-ignored at runtime) + PGV-mirror filter-internal validation discipline at `New` time (envoy-go own-wording errors per planner-time decision 4 — phase 11 ADR-0115 precedent) + parse-time-drop discipline for non-exact StringMatcher variants per ADR-0101 §3 verbatim discipline (NOT match-time-keep-and-fail) | Task 2 (`runtimeConfig` + `New` factory + parse-time `filter_enabled` PGV-mirror validation + StringMatcher parse-time-drop first lands). |
| ADR-0122 | Origin extraction trichotomy (Origin: `null` literal → empty; Origin empty/absent → Referer fallback; Origin non-empty unparseable → verbatim string) + comparison algorithm host:port-only equality (scheme stripped on both sides; NO normalization — case preserved, default ports preserved; trailing slash stripped via URL parser) + method gate canonical 4-method set `{POST, PUT, DELETE, PATCH}` + `additional_origins[].exact` matched against `host[:port]` form (NOT full URL with scheme — operator footgun documented unmistakably) + scheme-strip discipline (synthetic `http://` prefix per planner-time decision 8 — no framework extension needed) | Task 3 (`DecodeHeaders` body + `sourceOriginValue` + `targetOriginValue` + `hostAndPort` + `evaluate` + method gate first lands). |
| ADR-0123 | Rejection path wire shape + body byte-exact `Invalid origin` (14 bytes ASCII; NO trailing newline; MD5 `7433f3a046afcebee10e455dd26b0eb6` per SPEC §11.5 empirical pin) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + 403 hardcoded status + `SendLocalReply` reuse from phase 09 fault precedent (the existing `dcb.SendLocalReply(status, body, OrderedHeaders{...})` framework primitive at `internal/filter/http/fault/fault.go:321` carries through verbatim — no new framework primitive) + `StopIteration` from `DecodeHeaders` returning the chain to the localReplyDone gate | Task 3 (`DecodeHeaders` body + the `SendLocalReply` call site + reject-path unit tests first lands; ADR-0123 anchors at Task 3 alongside ADR-0122 because the wire shape is part of the disposition table). |
| ADR-0124 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 26→29-name extension for 3 csrf counters (`request_valid`, `request_invalid`, `missing_source_origin`) + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor per SN2) + drop `shadow_request_invalid` from MVP stat surface (couples to deferred shadow mode; reference Envoy also does not emit it under all-defaults config per §11.6 confirmation) + per-route stats SHARED with listener-level (per §11.9 amendment — diverges from phase 11 local_ratelimit precedent which had INDEPENDENT per-route stats per ADR-0117; csrf's "wholesale data-only override + shared stats" is a NEW pattern, codified here; the per-route runtime build mechanism is the option (b) helper `buildPerRouteRuntime(perRoute, listenerStats)` per planner-time decision 5) + ADR-0073 wholesale-override applies as-is for data, stats simply not part of the override (NO ADR-0073 amendment paragraph required) | Task 4 (`filterStats` wiring + 3-counter Inc-discipline + per-route shared-stats build + per-route TPFC override unit tests first lands the stat-shape end-to-end). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Inline supersessions / amendments anticipated** (cross-references only; **NO in-place ADR edits required** — this is a notable simplification from phases 10 + 11 which each amended ADR-0073):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — UNCHANGED in phase 12. Phase 12's per-route is data-only AND most-specific-override; the wholesale-override discipline applies as-is. Phase 11's ADR-0117 amendment paragraph (stateful per-route extension) and phase 10's ADR-0110 amendment (multi-tier evaluation) both stay landed and unused by phase 12. Cross-reference recorded in ADR-0124 §Decision noting the contrast: "phase 11 demonstrated wholesale-override extends to STATEFUL per-route resources via independent stats per ADR-0117; phase 12 demonstrates wholesale-override extends to DATA-ONLY per-route resources with SHARED stats — the inverse pattern; both fall within ADR-0073's original wholesale-override semantics without further amendment." NO in-place edit of ADR-0073.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 12. The 3-item deferral list (per SPEC §2.1.1 / §2.1.2 / §2.1.3) is captured INLINE at BEHAVIOR_CONTRACT §13.4 (the `### Phase 12 forward-pointer notes` subsection). NO new deferral ADRs are authored at phase 12 (mirrors phase 10 / phase 11 SPEC §8.1 collapse precedent — silent-ignore + parse-time-drop are framework patterns, deferral lists are documentation artefacts).
- **ADR-0061** (stats Registry + SN1–SN9 rules) — UNCHANGED in phase 12. NO new SN flattening rule. csrf reuses the existing SN2 rule (HCM-namespace `http.<HCM stat_prefix>.<rest>` → `envoy_http_<rest>` + label `envoy_http_conn_manager_prefix=<HCM stat_prefix>`); the 3 csrf counters fall under SN2 via the `http.<HCM stat_prefix>.csrf.<counter>` shape. UNLIKE phase 11 which extended SN with Rule SN9 for the filter-specific `<stat_prefix>.http_local_rate_limit.<counter>` shape via ADR-0118 amendment. Cross-reference recorded in ADR-0124 §Decision. NO in-place edit.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — UNCHANGED in phase 12 (the existing `Register` + `Freeze` discipline carries through). Cross-reference recorded in ADR-0120 §Consequences. NO in-place edit.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0120 §Consequences. The filter set extends from {cors, envoy_go_test, router, fault, header_mutation, local_ratelimit} to {cors, csrf, envoy_go_test, router, fault, header_mutation, local_ratelimit}. NO in-place edit of ADR-0074.
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 12. csrf's `New` factory CONSUMES `ctx.Stats` (for the three-counter `filterStats` registration) AND `ctx.StatPrefix` (the HCM-level stat_prefix is the namespace anchor for csrf's stats per §11.6 — UNLIKE local_ratelimit which uses its own proto `cfg.StatPrefix` field). ADR-0120 §Consequences notes the StatPrefix-consumption pattern (analogous to phases that anchor stats at the HCM-level prefix). NO in-place edit.
- **ADR-0101** (runtimeConfig shape + parser pattern + StringMatcher non-exact dropped at PARSE per §3) — extended cross-reference recorded in ADR-0121 §Consequences. The csrf runtimeConfig mirrors fault's structurally (2 fields vs fault's 8 — both follow the closure-capture + parse-at-New + read-only-shared-after-New discipline); the StringMatcher §3 discipline applies verbatim to `additional_origins[]`. NO in-place edit of ADR-0101.
- **ADR-0102** (terminal-replace + StopIteration localReplyDone gate) — VERBATIM REUSE in phase 12; no change. ADR-0123 §Consequences notes that the request-side terminal-replace primitive carries through unchanged for the rejection path (same primitive used by phase 09 fault abort + phase 11 local_ratelimit). NO in-place edit.
- **ADR-0117** (per-route bucket isolation as ADR-0073 wholesale-override consequence — phase 11) — UNCHANGED in phase 12. ADR-0124 §Decision notes the contrast: phase 11's stateful + independent-stats pattern coexists with phase 12's data-only + shared-stats pattern under the unified ADR-0073 wholesale-override umbrella. NO in-place edit.

These eight cross-references land at the task that anchors each affected ADR (ADR-0120 at Task 2; ADR-0121 at Task 2; ADR-0122 at Task 3; ADR-0123 at Task 3; ADR-0124 at Task 4). **NO in-place edit of any pre-existing ADR is required by phase 12** — this is a notable simplification from phases 10 + 11 (each of which amended ADR-0073).

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (per the user's persistent preference for git worktrees recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`). The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-12-http-filter-csrf-impl \
                 -b phase-12-http-filter-csrf-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-12-http-filter-csrf-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 16 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-12-http-filter-csrf-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-12-http-filter-csrf-impl -b phase-12-http-filter-csrf-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md commit `a305b86` and its SHA-fill follow-up `fb4d582` immediately before, then the BRAINSTORM.md commits `7fd9213` + `ba58c7e` + `399532c` + `c2e7559` + `bb29bb0`, then phase 11 REVIEW at `0f3a710`. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `119`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0120..ADR-0124 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/12-http-filter-csrf/SPEC.md` returns `a305b86` (the SPEC commit) or descendant. If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013'` returns every fixture PASS. The 14 pre-existing fixtures (0000–0013) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 15 fuzzers from phases 02–11 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each). Phase 12 adds the sixteenth (`FuzzCsrfPolicyConfigParse` per Task 5).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **`envoy.extensions.filters.http.csrf.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3 CsrfPolicy | head -5` returns the `CsrfPolicy` proto type's exported fields without an `import path failed` error; `go doc github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3 StringMatcher | head -5` returns the `StringMatcher` proto. If any `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `0f3a710` so a tidy should not be needed).
12. **Pre-existing `internal/filter/http/csrf/` directory does NOT exist.** `test ! -d internal/filter/http/csrf && echo "ok: csrf absent"` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
13. **Pre-existing `fixture.HTTPCsrf` does NOT exist.** `grep -nE 'HTTPCsrf' test/differential/fixture/fixture.go` returns 0 matches. If 1+, investigate.
14. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).
15. **Pre-existing `cmd/envoy-go/main.go` registers exactly the SIX filters expected at master `0f3a710`** — `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` returns 6 matches: `router`, `cors`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`. If 7+, another filter has been added concurrently; re-verify the registration ordering before adding the csrf line.
16. **Pre-existing `BEHAVIOR_CONTRACT.md` carries the phase-11 `### envoy.filters.http.local_ratelimit` subsection at line 1008** — `grep -n '^### envoy.filters.http.local_ratelimit' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns line `1008`. If 0 matches or different line, the file has drifted; re-read SPEC §13.1 to re-anchor the new csrf subsection insertion point.

If all 16 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the five ADRs ADR-0120..ADR-0124 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the five ADRs and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-12-http-filter-csrf-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 16 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (new file).
**Acceptance:** all 16 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-12-http-filter-csrf-impl
git log --oneline master | head -10                                   # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (fb4d582), SPEC (a305b86), BRAINSTORM commits, phase-11 REVIEW (0f3a710)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 119
git log -1 --format=%H -- docs/envoy-go/phases/12-http-filter-csrf/SPEC.md
                                                                       # expect: a305b86... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/csrf && echo "ok: csrf absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3 CsrfPolicy | head -5
                                                                       # expect: type CsrfPolicy struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3 StringMatcher | head -5
                                                                       # expect: type StringMatcher struct { ... }
grep -cE 'HTTPCsrf' test/differential/fixture/fixture.go              # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
grep -cE 'httpReg.Register' cmd/envoy-go/main.go                      # expect: 6
grep -n '^### envoy.filters.http.local_ratelimit' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: 1008:### envoy.filters.http.local_ratelimit
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md`**

```markdown
# Phase 12 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..11 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 16 preconditions were satisfied at cold-start>

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The five ADRs anticipated by SPEC §8 (ADR-0120..ADR-0124). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0120** `internal/filter/http/csrf/` package shape (single-token directory matching cors precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value with `Encoder: nil`) — Task 2
- **ADR-0121** runtimeConfig shape + 1/1/1-field decomposition (1 consumed, 1 PGV-validated-not-honored, 1 deferred) + PGV-mirror filter-internal validation discipline at New time + StringMatcher non-exact variants dropped at PARSE per ADR-0101 §3 — Task 2
- **ADR-0122** Origin extraction trichotomy + comparison algorithm host:port-only equality + method gate + additional_origins host:port matching + scheme-strip discipline — Task 3
- **ADR-0123** Rejection path wire shape + body byte-exact `Invalid origin` + 4-header set lowercase wire-form + 403 hardcoded status + SendLocalReply reuse from phase 09 — Task 3
- **ADR-0124** Stat-table 26→29-name extension + 3 csrf counters + namespace anchor at HCM stat_prefix reusing existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (NO new SN flattening rule) + drop `shadow_request_invalid` from MVP + per-route stats SHARED with listener-level (diverges from phase 11 ADR-0117 precedent) — Task 4

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nine planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)`; encode side ABSENT** (decoder-only filter; HTTPFilter struct sets Decoder: f, Encoder: nil — first §9 production filter to express decoder-only structurally).
2. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil`** (saves implementing StreamEncoderFilter method set; makes decoder-only nature self-documenting).
3. **D2 — URL-parse semantics for `hostAndPort()` = `net/url.Parse` + verbatim-string-on-parse-failure** (mirrors Envoy's `Http::Utility::Url::initialize` for common cases; verified at unit-test group 5 + §11 empirical pins as regression baseline).
4. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording** (option (b); `csrf: filter_enabled is required` + `csrf: filter_enabled.default_value is required`; phase 11 ADR-0115 precedent for envoy-go-own-wording).
5. **D4 — Per-route stats wiring mechanism = OPTION (b) per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper** (called from DecodeHeaders at request time; per-route runtimeConfig SHARES the listener-level *filterStats pointer; no NewCounterIfAbsent re-registration; no caching for MVP).
6. **PLAN-emerging — File-split decision = SINGLE-FILE `csrf.go`** (no `origin.go` split; ~280 LoC stays under mental-model threshold; no future filter anticipated to reuse the host:port-only equality helpers).
7. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with TWO ROUTES** (`/` default + `/route-only` per-route TPFC; fits existing `fixture.Driver` contract; saves driver complexity vs phase 11's 4-listener topology).
8. **PLAN-emerging — `:scheme` synthesis for `targetOriginValue` = USE A SYNTHETIC `http://` PREFIX** (no framework extension; no `:scheme` injection; no `DownstreamTLS()` callback; the synthetic prefix is stripped via `hostAndPort()` per §11.3 amendment so byte-equivalence with reference Envoy is preserved).
9. **PLAN-emerging — BackendKind enum value = `HTTPCsrf BackendKind = 11`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-12 SPEC + 12 PLAN confirmed present in HEAD; SPEC at a305b86; ADR tail at 0119 (next-free 0120); internal/filter/http/csrf/ absent (Task 2 lands); fixture.HTTPCsrf absent (Task 9 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/12-http-filter-csrf/SPEC.md
<verbatim>
\`\`\`
```

- [ ] **Step 3: Run preconditions verbatim and confirm pristine state**

```bash
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
go test -race -count=1 -short ./...                           # expect: all PASS (short mode skips differential)
```

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: PROGRESS preamble + planner-time decision resolution"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR anticipation table), §12 (deferred decisions), §15 (acceptance criteria) and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `internal/filter/http/csrf/` package — `doc.go` + `csrf.go` skeleton (TypeURL, types, runtimeConfig + parser + filter_enabled PGV-mirror validation, New factory) + `csrf_test.go` Group 1 + Group 2 tests [ADR-0120, ADR-0121]

**Files:**
- Create: `internal/filter/http/csrf/doc.go`
- Create: `internal/filter/http/csrf/csrf.go` (skeleton — types + New factory + buildRuntimeConfig + filter struct + SetDecoderCallbacks + OnDestroy + DecodeData/DecodeTrailers pass-through; DecodeHeaders body deferred to Task 3)
- Create: `internal/filter/http/csrf/csrf_test.go` (Group 1 PGV tests + Group 2 StringMatcher parse-time-drop tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0120 + ADR-0121)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (append Task 2 entry)

**Precondition:** Task 1 done; `internal/filter/http/csrf/` does not exist.
**Artifact:** new package + skeleton csrf.go + Group 1 + Group 2 unit tests + 2 ADRs.
**Acceptance:** `go build ./internal/filter/http/csrf/...` clean; `go vet ./internal/filter/http/csrf/...` clean; `go test -race -count=1 ./internal/filter/http/csrf/` PASS for the Group 1 + Group 2 tests; ADR-0120 + ADR-0121 appear in DECISIONS.md with full Context/Decision/Consequences sections; PROGRESS.md Task 2 entry written.

- [ ] **Step 1: Write the failing tests (TDD)**

Create `internal/filter/http/csrf/csrf_test.go` with the Group 1 + Group 2 tests. Test bodies use the same proto types csrf.go will consume; the tests fail to compile until csrf.go exists.

```go
package csrf

import (
	"testing"

	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// helper: wrap a *CsrfPolicy in *anypb.Any with the canonical type URL.
func mustAnyFrom(t *testing.T, c *csrfv3.CsrfPolicy) *anypb.Any {
	t.Helper()
	a, err := anypb.New(c)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func newTestFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{
		Stats:      stats.NewRegistry(),
		StatPrefix: "ingress_csrf",
	}
}

// Group 1 — New factory PGV-mirror validation.

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on nil tc")
	}
}

func TestNew_MalformedTC(t *testing.T) {
	a := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(a, newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error on malformed Any")
	}
}

func TestNew_FilterEnabledNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled is required (per §11.11)")
	}
}

func TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{},
	}
	_, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err == nil {
		t.Fatal("expected error: filter_enabled.default_value is required (per §11.11)")
	}
}

func TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   0,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("expected accept (silent-ignore percentage); got %v", err)
	}
}

func TestNew_FilterEnabledHundredPercent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("happy-path New: %v", err)
	}
}

func TestNew_ShadowEnabledAbsent_Accepted(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled absent should be OK: %v", err)
	}
}

func TestNew_ShadowEnabledPresent_SilentIgnored(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		ShadowEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), newTestFactoryCtx()); err != nil {
		t.Fatalf("shadow_enabled present should be OK (silent-ignored): %v", err)
	}
}

// Group 2 — additional_origins parse-time discipline.

func TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse(t *testing.T) {
	tests := []struct {
		name    string
		matcher *matcherv3.StringMatcher
	}{
		{"prefix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "pfx-"}}},
		{"suffix", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Suffix{Suffix: "-sfx"}}},
		{"contains", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Contains{Contains: "mid"}}},
		{"safe_regex", &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_SafeRegex{SafeRegex: nil}}},
		{"ignore_case_with_exact", &matcherv3.StringMatcher{IgnoreCase: true, MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "case-folded.test"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &csrfv3.CsrfPolicy{
				FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
				AdditionalOrigins: []*matcherv3.StringMatcher{tt.matcher},
			}
			factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			// Use an unexported test-helper to inspect the runtimeConfig — see helper below.
			rc := mustGetRuntimeConfig(t, factory)
			if len(rc.additionalOrigins) != 0 {
				t.Errorf("non-exact StringMatcher %q must be dropped at parse; got %v",
					tt.name, rc.additionalOrigins)
			}
		})
	}
}

func TestNew_AdditionalOrigins_EmptyExactValue_Dropped(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: ""}},
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	if len(rc.additionalOrigins) != 0 {
		t.Errorf("empty-value exact entry must be dropped; got %v", rc.additionalOrigins)
	}
}

func TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm(t *testing.T) {
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "app.example.test"}},
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "Other.Test:8443"}}, // case + port preserved
		},
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rc := mustGetRuntimeConfig(t, factory)
	want := []string{"app.example.test", "Other.Test:8443"}
	if len(rc.additionalOrigins) != len(want) {
		t.Fatalf("len mismatch; got %v want %v", rc.additionalOrigins, want)
	}
	for i, w := range want {
		if rc.additionalOrigins[i] != w {
			t.Errorf("entry %d: got %q want %q (verbatim preservation)", i, rc.additionalOrigins[i], w)
		}
	}
}

// mustGetRuntimeConfig is a test-only helper that invokes the FilterInstanceFactory
// once and inspects the captured *runtimeConfig via the per-instance filter struct's
// rc field. csrf.go must expose this through the test-only file or via a package-
// internal accessor — implementer at Task 2 step 2 chooses the precise mechanism.
func mustGetRuntimeConfig(t *testing.T, factory envoyhttp.FilterInstanceFactory) *runtimeConfig {
	t.Helper()
	hf := factory()
	d, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("Decoder is not *filter; got %T", hf.Decoder)
	}
	return d.rc
}

// Compile-time check: SPEC's documented public surface.
var (
	_ proto.Message              = (*csrfv3.CsrfPolicy)(nil)
	_ envoyhttp.HTTPFilterFactory = New
)
```

- [ ] **Step 2: Run tests to verify they fail (compile failure expected)**

```bash
go test -count=1 ./internal/filter/http/csrf/
# expect: compile errors — package does not exist yet
```

- [ ] **Step 3: Create the minimal `csrf.go` skeleton + `doc.go`**

`internal/filter/http/csrf/doc.go`:

```go
// Package csrf implements envoy.filters.http.csrf — Envoy v1.37.2's canonical
// same-origin enforcement filter. The filter rejects modifying-method requests
// whose Origin/Referer-derived source-origin does not match the request's
// Host/:authority-derived target-origin (or any operator-supplied
// additional_origins[]).
//
// MVP envelope per phase 12 SPEC §1 + §1.1:
//   - 1 proto field actively consumed (additional_origins[].StringMatcher.exact
//     non-empty values; non-exact variants dropped at PARSE time per ADR-0101 §3).
//   - 1 proto field PGV-validated-not-honored at runtime (filter_enabled — REQUIRED
//     at parse-time per §11.11 amendment; runtime always-100%-active).
//   - 1 proto field deferred (shadow_enabled — optional at parse, never-shadow at
//     runtime; couples to Runtime + hot restart family).
//
// Comparison algorithm is host:port-only equality per §11.3 + §11.7 + §11.8
// amendments: scheme is computed only to make URLs parseable then stripped on
// both sides; NO case folding; NO default-port stripping; trailing slashes are
// stripped via the URL parser. additional_origins[].exact values are matched
// against the source's host[:port] form (NOT full URL with scheme — operator
// footgun documented at BEHAVIOR_CONTRACT §13.4).
//
// Origin-extraction trichotomy per §11.2 amendment: (i) Origin: null literal →
// empty source NO Referer fallback; (ii) Origin empty/absent → fall back to
// Referer's hostAndPort; (iii) Origin non-empty unparseable → verbatim string
// used as source.
//
// Method gate is canonical 4-method set {POST, PUT, DELETE, PATCH} per §11.1.
// Non-modifying methods short-circuit to Continue BEFORE any state touch.
//
// Per-route TPFC override is data-only with SHARED listener-level stats per
// §11.9 amendment — diverges from phase 11 local_ratelimit precedent (ADR-0117)
// which had INDEPENDENT per-route stats. csrf is the FIRST production filter
// to demonstrate the "wholesale data-only override + shared stats" pattern.
//
// Iteration-protocol coverage:
//   - DecodeHeaders runs the disposition table; on allow → Continue; on
//     missing/reject → SendLocalReply(403) + StopIteration (request-side
//     terminal-replace per ADR-0102; reused verbatim from phase 09 fault).
//   - DecodeData / DecodeTrailers / OnDestroy: pass-through / no-op.
//   - NO encode-side methods. The HTTPFilter value sets Decoder: f, Encoder: nil.
//
// Cross-cutting ADR anchors:
//   - ADR-0120 (package shape + boot registration ordering)
//   - ADR-0121 (runtimeConfig + 1/1/1-field decomposition + PGV-mirror discipline)
//   - ADR-0122 (origin trichotomy + host:port-only equality + method gate)
//   - ADR-0123 (rejection wire shape + SendLocalReply reuse)
//   - ADR-0124 (3-counter stat surface + namespace anchor at HCM stat_prefix +
//     per-route stats SHARED with listener-level)
package csrf
```

`internal/filter/http/csrf/csrf.go` — skeleton with types, New factory, helpers, but DecodeHeaders body deferred to Task 3:

```go
package csrf

import (
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"

	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.csrf typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go registers New under this key in the
// HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy"

// New is the HTTPFilterFactory exposed at boot. Per SPEC §11.11 amendment the
// CsrfPolicy.filter_enabled field is REQUIRED at parse-time — envoy-go
// PGV-mirrors Envoy's behavior by rejecting nil filter_enabled or nil
// filter_enabled.default_value with a non-nil error.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("csrf: typed_config required")
	}
	var c csrfv3.CsrfPolicy
	if err := tc.UnmarshalTo(&c); err != nil {
		return nil, err
	}
	listenerStats := newFilterStats(ctx.Stats, ctx.StatPrefix)
	rc, err := buildRuntimeConfig(&c, listenerStats)
	if err != nil {
		return nil, err
	}
	return func() envoyhttp.HTTPFilter {
		f := &filter{rc: rc}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.csrf",
			Decoder: f,
			Encoder: nil, // decoder-only per planner-time decision 2
		}
	}, nil
}

// runtimeConfig captures the 2 runtime fields per SPEC §6.2 (the proto's 3
// top-level fields decompose to: 1 consumed → additionalOrigins; 1
// PGV-validated-not-honored → filter_enabled (silent-ignored at runtime);
// 1 deferred → shadow_enabled). The stats pointer is the listener-level
// pointer; per-route runtimeConfig SHARES this pointer per §11.9 amendment.
type runtimeConfig struct {
	additionalOrigins []string     // verbatim host[:port] entries from surviving exact-with-non-empty StringMatcher
	stats             *filterStats // listener-level; per-route SHARES this
}

// filterStats is the 3-counter set per SPEC §6.6 + ADR-0124. Counters use
// *atomic.Int64 for lock-free increments per ADR-0061; allocated once at New
// time, shared across all goroutines processing requests through this filter
// instance (and across listener-level + per-route runtimeConfigs per §11.9).
type filterStats struct {
	requestValid        *atomic.Int64
	requestInvalid      *atomic.Int64
	missingSourceOrigin *atomic.Int64
}

// filter is the per-instance per-stream filter state. Per ADR-0071's single-
// goroutine-per-stream invariant, the per-instance state is race-free without
// synchronization. The rc pointer is closure-captured at New time and is
// immutable post-construction.
type filter struct {
	rc  *runtimeConfig
	dcb envoyhttp.DecoderFilterCallbacks
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) OnDestroy()                                              {}

// DecodeData / DecodeTrailers are pass-through (csrf settles disposition in
// DecodeHeaders).
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// DecodeHeaders body is implemented in Task 3.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Skeleton only — Task 3 lands the disposition table.
	return envoyhttp.Continue
}

// buildRuntimeConfig validates the proto + compiles additional_origins. If
// listenerStats is non-nil it is reused (per-route build path; per §11.9 +
// planner-time decision 5); else a fresh *filterStats has already been
// allocated by the caller (listener-level build path).
func buildRuntimeConfig(c *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error) {
	if c.GetFilterEnabled() == nil {
		return nil, errors.New("csrf: filter_enabled is required")
	}
	if c.GetFilterEnabled().GetDefaultValue() == nil {
		return nil, errors.New("csrf: filter_enabled.default_value is required")
	}
	// shadow_enabled is OPTIONAL — no validation per §11.11 probe #3.
	// filter_enabled.default_value's percentage value is silent-ignored at
	// runtime per §1.1 amendment 3 + planner-time decision below; we read it
	// for documentation but do not capture it into runtimeConfig.

	compiled := compileAdditionalOrigins(c.GetAdditionalOrigins())
	return &runtimeConfig{
		additionalOrigins: compiled,
		stats:             listenerStats,
	}, nil
}

// compileAdditionalOrigins applies the ADR-0101 §3 parse-time-drop discipline:
// only StringMatcher.exact variant with non-empty value is honored; all other
// variants (prefix, suffix, contains, safe_regex, ignore_case-with-anything)
// are dropped at PARSE time. Surviving entries are stored verbatim — NO
// normalization (NO case folding, NO default-port stripping) per §11.7 amendment.
func compileAdditionalOrigins(matchers []*matcherv3.StringMatcher) []string {
	out := make([]string, 0, len(matchers))
	for _, m := range matchers {
		if m == nil {
			continue
		}
		if m.GetIgnoreCase() {
			// ignore_case is a non-exact MODIFIER even on top of an exact
			// pattern; drop per ADR-0101 §3 ("only HeaderMatcher_StringMatch
			// with non-empty Exact value is honored").
			continue
		}
		exact, ok := m.GetMatchPattern().(*matcherv3.StringMatcher_Exact)
		if !ok {
			continue
		}
		if exact.Exact == "" {
			continue
		}
		out = append(out, exact.Exact)
	}
	return out
}

// newFilterStats registers the 3-counter set at the HCM-level stat_prefix per
// SPEC §6.6. Stats anchor at "http.<hcmStatPrefix>.csrf.<counter>" per the
// existing SN2 rule from ADR-0061; NO new SN flattening rule needed (UNLIKE
// phase 11 which introduced SN9 for filter-specific tag-extraction).
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := "http." + hcmStatPrefix + ".csrf."
	return &filterStats{
		requestValid:        reg.NewCounter(prefix + "request_valid"),
		requestInvalid:      reg.NewCounter(prefix + "request_invalid"),
		missingSourceOrigin: reg.NewCounter(prefix + "missing_source_origin"),
	}
}

// Stub helpers — Task 3 fills the bodies.
func sourceOriginValue(_ http.Header) string                                 { return "" }
func targetOriginValue(_ http.Header) string                                 { return "" }
func hostAndPort(_ string) string                                            { return "" }
func evaluate(_ *runtimeConfig, _, _ string) (allow bool, missing bool)       { return false, false }
func buildPerRouteRuntime(_ *csrfv3.CsrfPolicy, _ *filterStats) (*runtimeConfig, error) {
	return nil, nil
}

// Compile-time use of net/url to keep the import live for Task 3.
var _ = url.Parse
```

- [ ] **Step 4: Run the tests to verify Group 1 + Group 2 PASS**

```bash
go test -race -count=1 -v ./internal/filter/http/csrf/
# expect: TestNew_NilTC PASS, TestNew_MalformedTC PASS, TestNew_FilterEnabledNil_RejectAtParseTime PASS,
# TestNew_FilterEnabledDefaultValueNil_RejectAtParseTime PASS, TestNew_FilterEnabledZeroPercent_AcceptedSilentIgnored PASS,
# TestNew_FilterEnabledHundredPercent_Accepted PASS, TestNew_ShadowEnabledAbsent_Accepted PASS,
# TestNew_ShadowEnabledPresent_SilentIgnored PASS, TestNew_AdditionalOrigins_NonExactStringMatcher_DroppedAtParse/* PASS,
# TestNew_AdditionalOrigins_EmptyExactValue_Dropped PASS, TestNew_AdditionalOrigins_PreservesVerbatimHostPortForm PASS
```

- [ ] **Step 5: Run vet + lint clean**

```bash
go vet ./internal/filter/http/csrf/...
golangci-lint run ./internal/filter/http/csrf/...
# expect: clean
```

- [ ] **Step 6: Append ADR-0120 + ADR-0121 to `docs/envoy-go/DECISIONS.md`**

Both ADRs follow the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0120 anchors the package shape + boot registration ordering (cite cors/`cors/` precedent + the alphabetical-after-router ordering). ADR-0121 anchors the 1/1/1 field decomposition + the PGV-mirror filter-internal validation discipline + the StringMatcher non-exact parse-time-drop discipline (cite §11.11 amendment + ADR-0101 §3 + planner-time decision 4 for the envoy-go-own-wording choice).

Implementer drafts both ADR bodies. Acceptance: `grep -nE '^## ADR-0120|^## ADR-0121' docs/envoy-go/DECISIONS.md` returns 2 matches.

- [ ] **Step 7: Append PROGRESS.md Task 2 entry**

Per the PROGRESS structure: `**Commits:**` (one or two SHAs — feature commit + SHA-fill follow-up), `**Notes:**` (anything noteworthy), `**Outputs:**` (verbatim test + vet + lint output).

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/csrf/doc.go internal/filter/http/csrf/csrf.go \
         internal/filter/http/csrf/csrf_test.go \
         docs/envoy-go/DECISIONS.md docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: csrf package skeleton + New factory PGV-mirror + parse-time StringMatcher drop [ADR-0120, ADR-0121]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 §1.1 amendment 3 §2.1 §6.1 §6.2 §11.11 §14.1 group 1 + 2; ADR-0072 (factory validation contract); ADR-0101 §3 (StringMatcher non-exact parse-time-drop discipline); ADR-0100 (FactoryCtx.Stats + StatPrefix consumption); planner-time decision 1 (decoder-only filter), 2 (HTTPFilter Encoder: nil), 4 (envoy-go-own-wording errors).*

---

## Task 3: `DecodeHeaders` body — method gate + origin trichotomy + host:port-only equality + reject-path SendLocalReply [ADR-0122, ADR-0123]

**Files:**
- Modify: `internal/filter/http/csrf/csrf.go` (fill in `DecodeHeaders`, `sourceOriginValue`, `targetOriginValue`, `hostAndPort`, `evaluate`)
- Modify: `internal/filter/http/csrf/csrf_test.go` (add Group 3 + Group 4 + Group 5 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0122 + ADR-0123)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 3 entry)

This task lands the disposition table — method gate (4-method set), source-origin trichotomy (Origin null literal → empty NO Referer fallback / Origin empty/absent → Referer fallback / Origin non-empty unparseable → verbatim), host:port-only comparison (scheme stripped on both sides; NO normalization; trailing slash stripped via URL parser; `additional_origins[].exact` matched against host[:port] form), reject-path SendLocalReply(403, "Invalid origin", 1-header set). Per-route TPFC handling (`f.dcb.RequestRouteConfig()`) is implemented but the per-route shared-stats wiring assertion lives in Task 4 (Group 6). Per SPEC §6.4 + §6.5 + §11.1 + §11.2 + §11.3 + §11.7 + §11.8 + §11.10.

**Precondition:** Task 2 done; `New` factory + `runtimeConfig` + `filterStats` + skeleton DecodeHeaders + helper stubs all present.
**Artifact:** filled-in csrf.go DecodeHeaders body + helpers + Group 3/4/5 tests + ADR-0122 + ADR-0123 + PROGRESS entry.
**Acceptance:** all Group 1-5 tests PASS; `go test -race ./internal/filter/http/csrf/` clean; ADR-0122 + ADR-0123 in DECISIONS.md.

- [ ] **Step 1: Write failing tests for Group 3 + Group 4 + Group 5**

Append to `internal/filter/http/csrf/csrf_test.go`:

```go
// Group 3 — DecodeHeaders non-modifying methods.

func TestDecodeHeaders_NonModifyingMethods(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS", "TRACE", "PROPFIND"} {
		t.Run(method, func(t *testing.T) {
			factory := mustNewListenerFactory(t, []string{"app.example.test"})
			f := factory().Decoder.(*filter)
			f.SetDecoderCallbacks(&fakeCallbacks{})
			h := http.Header{}
			h.Set(":method", method)
			h.Set("Host", "127.0.0.1:8080")
			h.Set("Origin", "https://evil.test") // would normally reject — but method short-circuits before any check
			status := f.DecodeHeaders(h, true)
			if status != envoyhttp.Continue {
				t.Errorf("non-modifying method %s: got %v want Continue", method, status)
			}
			rc := f.rc
			if rc.stats.requestValid.Load() != 0 || rc.stats.requestInvalid.Load() != 0 || rc.stats.missingSourceOrigin.Load() != 0 {
				t.Errorf("non-modifying method %s: counters touched (rv=%d ri=%d mso=%d)",
					method, rc.stats.requestValid.Load(), rc.stats.requestInvalid.Load(), rc.stats.missingSourceOrigin.Load())
			}
		})
	}
}

// Group 4 — origin extraction trichotomy.

func TestDecodeHeaders_OriginNullLiteral_MissingSourceOrigin_NoRefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, cb := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "null")
	h.Set("Referer", "http://127.0.0.1:8080/page") // would otherwise rescue
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.missingSourceOrigin.Load() != 1 {
		t.Errorf("missing_source_origin should increment; got %d", f.rc.stats.missingSourceOrigin.Load())
	}
	if cb.localReply == nil || cb.localReply.status != 403 || cb.localReply.body != "Invalid origin" {
		t.Errorf("expected SendLocalReply(403, \"Invalid origin\"); got %+v", cb.localReply)
	}
}

func TestDecodeHeaders_OriginEmpty_RefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "")
	h.Set("Referer", "http://127.0.0.1:8080/page")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue (Referer rescues)", status)
	}
	if f.rc.stats.requestValid.Load() != 1 {
		t.Errorf("request_valid should increment; got %d", f.rc.stats.requestValid.Load())
	}
}

func TestDecodeHeaders_OriginAbsent_RefererFallback(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Referer", "http://127.0.0.1:8080/page")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
}

func TestDecodeHeaders_OriginAbsent_RefererAbsent_MissingSourceOrigin(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.missingSourceOrigin.Load() != 1 {
		t.Errorf("missing_source_origin should increment")
	}
}

func TestDecodeHeaders_OriginUnparseable_VerbatimUsed(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "not-a-url")
	h.Set("Referer", "http://127.0.0.1:8080/page") // would otherwise allow
	status := f.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (verbatim mismatch — no Referer fallback)", status)
	}
	if f.rc.stats.requestInvalid.Load() != 1 {
		t.Errorf("request_invalid should increment; got rv=%d ri=%d mso=%d",
			f.rc.stats.requestValid.Load(), f.rc.stats.requestInvalid.Load(), f.rc.stats.missingSourceOrigin.Load())
	}
}

// Group 5 — host:port-only equality.

func TestDecodeHeaders_SameOrigin_HostPortMatch(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "http://127.0.0.1:8080")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
	if f.rc.stats.requestValid.Load() != 1 {
		t.Errorf("request_valid should increment")
	}
}

func TestDecodeHeaders_CrossOrigin_HostMismatch(t *testing.T) {
	factory := mustNewListenerFactory(t, nil)
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://evil.test")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration", status)
	}
	if f.rc.stats.requestInvalid.Load() != 1 {
		t.Errorf("request_invalid should increment")
	}
}

func TestDecodeHeaders_AdditionalOriginsExactMatch(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test") // scheme stripped → app.example.test → matches additional_origins
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue", status)
	}
}

func TestDecodeHeaders_NoCaseFolding_UppercaseRejected(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "HTTPS://APP.EXAMPLE.TEST")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (case preserved per §11.7 A2/A3)", status)
	}
}

func TestDecodeHeaders_NoDefaultPortStripping_PortMismatch(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test:443") // hostAndPort = app.example.test:443 ≠ app.example.test
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("got %v want StopIteration (default port preserved per §11.7 A4)", status)
	}
}

func TestDecodeHeaders_TrailingSlashStripped_Allow(t *testing.T) {
	factory := mustNewListenerFactory(t, []string{"app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test/") // path "/" dropped via URL parser → app.example.test
	if status := f.DecodeHeaders(h, true); status != envoyhttp.Continue {
		t.Fatalf("got %v want Continue (trailing slash stripped per §11.7 A7)", status)
	}
}

func TestDecodeHeaders_OperatorFootgun_FullURLEntry_NeverMatches(t *testing.T) {
	// Per §11.8 amendment: additional_origins entry "https://app.example.test"
	// (full URL with scheme) NEVER matches a real Origin header — operator
	// footgun documented at BEHAVIOR_CONTRACT §13.4.
	factory := mustNewListenerFactory(t, []string{"https://app.example.test"})
	f, _ := freshFilter(t, factory)
	h := newPostHeaders("127.0.0.1:8080")
	h.Set("Origin", "https://app.example.test")
	if status := f.DecodeHeaders(h, true); status != envoyhttp.StopIteration {
		t.Fatalf("operator footgun: full-URL entry never matches; got %v want StopIteration", status)
	}
}

// Test helpers (lives at the end of csrf_test.go).

func newPostHeaders(host string) http.Header {
	h := http.Header{}
	h.Set(":method", "POST")
	h.Set("Host", host)
	return h
}

func mustNewListenerFactory(t *testing.T, additional []string) envoyhttp.FilterInstanceFactory {
	t.Helper()
	matchers := make([]*matcherv3.StringMatcher, 0, len(additional))
	for _, a := range additional {
		matchers = append(matchers, &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: a}})
	}
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		AdditionalOrigins: matchers,
	}
	factory, err := New(mustAnyFrom(t, c), newTestFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return factory
}

func freshFilter(t *testing.T, factory envoyhttp.FilterInstanceFactory) (*filter, *fakeCallbacks) {
	t.Helper()
	hf := factory()
	f := hf.Decoder.(*filter)
	cb := &fakeCallbacks{}
	f.SetDecoderCallbacks(cb)
	return f, cb
}

type localReplyArgs struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

type fakeCallbacks struct {
	localReply *localReplyArgs
	perRoute   proto.Message
}

func (c *fakeCallbacks) ContinueDecoding() {}
func (c *fakeCallbacks) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	c.localReply = &localReplyArgs{status: status, body: body, headers: headers}
}
func (c *fakeCallbacks) RequestRouteConfig() proto.Message            { return c.perRoute }
func (c *fakeCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeCallbacks) EncodeHeaders(_ http.Header, _ bool) {}
func (c *fakeCallbacks) EncodeData(_ []byte, _ bool)         {}
func (c *fakeCallbacks) EncodeTrailers(_ http.Header)        {}
```

- [ ] **Step 2: Run tests to verify they fail (skeleton DecodeHeaders returns Continue for all)**

```bash
go test -race -count=1 -v ./internal/filter/http/csrf/
# expect: Group 1+2 PASS; Group 3 PASS (skeleton returns Continue which is correct for non-modifying methods);
# Group 4 + 5 FAIL (skeleton returns Continue everywhere, no counter increments, no SendLocalReply)
```

- [ ] **Step 3: Replace the helper stubs + DecodeHeaders body with the real implementation**

Replace the stub bodies in `csrf.go` with real implementations:

```go
// modifyingMethods is the canonical 4-method set per §11.1 empirical pin.
// Case-sensitive uppercase string match against the :method pseudo-header.
var modifyingMethods = map[string]struct{}{
	"POST":   {},
	"PUT":    {},
	"DELETE": {},
	"PATCH":  {},
}

// DecodeHeaders implements the disposition table per SPEC §6.5 + ADR-0122 + ADR-0123.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	method := headers.Get(":method")
	if _, ok := modifyingMethods[method]; !ok {
		return envoyhttp.Continue // non-modifying: short-circuit BEFORE any state touch
	}

	// Resolve the effective runtimeConfig: per-route override OR listener-level fallback.
	rc := f.rc
	if perRoute := f.dcb.RequestRouteConfig(); perRoute != nil {
		if c, ok := perRoute.(*csrfv3.CsrfPolicy); ok {
			pr, err := buildPerRouteRuntime(c, f.rc.stats)
			if err == nil && pr != nil {
				rc = pr
			}
			// On err: fall through to listener-level rc. (Per-route invalid configs
			// are caught by reference Envoy at PGV; differential equivalence is
			// preserved for well-formed configs. See planner-time decision 5.)
		}
	}

	target := targetOriginValue(headers)
	source := sourceOriginValue(headers)
	allow, missing := evaluate(rc, source, target)
	switch {
	case allow:
		rc.stats.requestValid.Add(1)
		return envoyhttp.Continue
	case missing:
		rc.stats.missingSourceOrigin.Add(1)
	default:
		rc.stats.requestInvalid.Add(1)
	}
	f.dcb.SendLocalReply(403, "Invalid origin", envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
	})
	return envoyhttp.StopIteration
}

// sourceOriginValue implements the §11.2 trichotomy.
//   1. Origin: "null" literal → empty (NO Referer fallback).
//   2. Origin empty/absent OR yields empty hostAndPort → fall back to Referer's hostAndPort.
//   3. Origin non-empty, non-"null", URL parse fails → return verbatim (NO Referer fallback).
func sourceOriginValue(headers http.Header) string {
	originVal := headers.Get("Origin")
	if originVal == "null" {
		return "" // case 1: missing_source_origin path
	}
	if originVal != "" {
		// Case 3 OR successful parse: hostAndPort returns either u.Host or
		// the verbatim string on parse failure.
		hp := hostAndPort(originVal)
		if hp != "" {
			return hp
		}
		// hp == "" only if originVal was "" (already filtered above) — defensive.
	}
	// Case 2: Origin empty or absent → fall back to Referer.
	refererVal := headers.Get("Referer")
	if refererVal == "" {
		return ""
	}
	return hostAndPort(refererVal)
}

// targetOriginValue constructs the target host:port from the request's Host
// or :authority header. Per planner-time decision 8 + §11.3 amendment: scheme
// is computed only to make the URL parser accept the input then stripped via
// hostAndPort(); we use a synthetic "http://" prefix.
func targetOriginValue(headers http.Header) string {
	host := headers.Get(":authority")
	if host == "" {
		host = headers.Get("Host")
	}
	if host == "" {
		return ""
	}
	return hostAndPort("http://" + host)
}

// hostAndPort parses an absolute URL and returns the host[:port] portion.
// On parse failure (or empty u.Host), returns the verbatim input — mirrors
// Envoy's Http::Utility::Url::initialize fallback per §11.2 source-of-truth.
func hostAndPort(absoluteURL string) string {
	if absoluteURL == "" {
		return ""
	}
	u, err := url.Parse(absoluteURL)
	if err != nil || u.Host == "" {
		return absoluteURL
	}
	return u.Host
}

// evaluate implements the disposition algorithm per §6.4. Empty source →
// missing path. Non-empty source: check additionalOrigins[] first (any byte-
// equal match → allow), then target equality (byte-equal → allow), else reject.
func evaluate(rc *runtimeConfig, source, target string) (allow bool, missing bool) {
	if source == "" {
		return false, true // missing_source_origin
	}
	for _, additional := range rc.additionalOrigins {
		if source == additional {
			return true, false
		}
	}
	if source == target {
		return true, false
	}
	return false, false
}

// buildPerRouteRuntime is the per-route runtimeConfig builder (per §11.9 +
// planner-time decision 5). The per-route runtimeConfig SHARES the listener-
// level *filterStats pointer; only the additionalOrigins slice is independent.
// PGV-mirror validation runs the same as listener-level (filter_enabled
// non-nil + non-nil DefaultValue).
func buildPerRouteRuntime(perRoute *csrfv3.CsrfPolicy, listenerStats *filterStats) (*runtimeConfig, error) {
	return buildRuntimeConfig(perRoute, listenerStats)
}
```

- [ ] **Step 4: Run tests; verify Groups 3 + 4 + 5 PASS**

```bash
go test -race -count=1 -v ./internal/filter/http/csrf/
# expect: all Group 1-5 tests PASS
```

- [ ] **Step 5: Run vet + lint clean**

```bash
go vet ./internal/filter/http/csrf/...
golangci-lint run ./internal/filter/http/csrf/...
# expect: clean
```

- [ ] **Step 6: Append ADR-0122 + ADR-0123 to `docs/envoy-go/DECISIONS.md`**

ADR-0122 captures: origin-extraction trichotomy (Origin null literal → empty NO Referer fallback; Origin empty/absent → Referer fallback; Origin non-empty unparseable → verbatim) + comparison algorithm host:port-only equality (scheme stripped on both sides; NO normalization — case preserved, default ports preserved; trailing slash stripped via URL parser) + method gate canonical 4-method set + `additional_origins[].exact` matched against host[:port] form (operator footgun) + scheme-strip discipline (synthetic `http://` prefix per planner-time decision 8). Cite §11.1 / §11.2 / §11.3 / §11.7 / §11.8 empirical pins.

ADR-0123 captures: rejection path wire shape — `SendLocalReply(403)` + body byte-exact `Invalid origin` (14 bytes ASCII, no LF, MD5 `7433f3a046afcebee10e455dd26b0eb6`) + 4-header set lowercase wire-form (`content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + 403 hardcoded status + `StopIteration` from DecodeHeaders + `SendLocalReply` reuse from phase 09 fault precedent. Cite §11.4 / §11.5 / §11.10 empirical pins.

Acceptance: `grep -nE '^## ADR-0122|^## ADR-0123' docs/envoy-go/DECISIONS.md` returns 2 matches.

- [ ] **Step 7: Append PROGRESS.md Task 3 entry + commit**

```bash
git add internal/filter/http/csrf/csrf.go internal/filter/http/csrf/csrf_test.go \
         docs/envoy-go/DECISIONS.md docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: csrf DecodeHeaders body + origin trichotomy + host:port equality + reject path [ADR-0122, ADR-0123]"
```

SHA-fill follow-up.

*Anchored: SPEC §6.4 §6.5 §11.1 §11.2 §11.3 §11.7 §11.8 §11.10 §14.1 group 3+4+5; ADR-0102 (terminal-replace + StopIteration); ADR-0103 (fault wire-shape precedent); ADR-0075 (SendLocalReply encode-chain entry); planner-time decision 3 (`net/url.Parse` + verbatim-on-failure), 8 (synthetic `http://` prefix).*

---

## Task 4: `filterStats` wiring + 3-counter Inc-discipline + per-route shared-stats build + Group 6 unit tests [ADR-0124]

**Files:**
- Modify: `internal/filter/http/csrf/csrf_test.go` (add Group 6 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0124)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 4 entry)

This task validates the §11.9 amendment's load-bearing claim: per-route TPFC override carries its own `additionalOrigins` slice but SHARES the listener-level `*filterStats` pointer. Plus the stat-name discipline per SPEC §6.6 — counters anchor at `http.<HCM stat_prefix>.csrf.<name>` per the existing SN2 rule from ADR-0061; NO new SN flattening rule. NO production code changes (the per-route shared-stats wiring landed in Task 3); this task adds the unit-test confirmation + lands ADR-0124.

**Precondition:** Task 3 done; per-route runtime build invoked from DecodeHeaders.
**Artifact:** Group 6 tests + ADR-0124 + PROGRESS entry.
**Acceptance:** all Group 1-6 tests PASS; ADR-0124 in DECISIONS.md.

- [ ] **Step 1: Append Group 6 tests to `csrf_test.go`**

```go
// Group 6 — per-route override + shared stats.

func TestDecodeHeaders_PerRouteOverride_DataReplaced(t *testing.T) {
	listener := mustNewListenerFactory(t, []string{"app.example.test"})
	f, cb := freshFilter(t, listener)
	cb.perRoute = &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "route-only.test"}},
		},
	}

	// Per-route request with route-only origin → allowed by per-route TPFC.
	hA := newPostHeaders("127.0.0.1:8080")
	hA.Set("Origin", "https://route-only.test")
	if status := f.DecodeHeaders(hA, true); status != envoyhttp.Continue {
		t.Errorf("per-route Origin=route-only.test: got %v want Continue", status)
	}
	if f.rc.stats.requestValid.Load() != 1 {
		t.Errorf("listener-level stats.requestValid should AGGREGATE per-route increments; got %d",
			f.rc.stats.requestValid.Load())
	}
}

func TestDecodeHeaders_PerRouteStatsShared_AggregatesAcrossListenerAndPerRoute(t *testing.T) {
	listener := mustNewListenerFactory(t, []string{"app.example.test"})
	f, cb := freshFilter(t, listener)
	cb.perRoute = &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
		AdditionalOrigins: []*matcherv3.StringMatcher{
			{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "route-only.test"}},
		},
	}

	// 1: per-route hit + match → request_valid +1.
	h1 := newPostHeaders("127.0.0.1:8080")
	h1.Set("Origin", "https://route-only.test")
	f.DecodeHeaders(h1, true)

	// 2: per-route hit + miss (Origin: app.example.test does NOT match per-route's
	//    additionalOrigins=[route-only.test], does NOT match target 127.0.0.1:8080) → request_invalid +1.
	cb.localReply = nil
	h2 := newPostHeaders("127.0.0.1:8080")
	h2.Set("Origin", "https://app.example.test")
	f.DecodeHeaders(h2, true)

	// 3: listener-level (no per-route this time) + Origin app.example.test → match additional_origins → request_valid +1.
	cb.perRoute = nil
	cb.localReply = nil
	h3 := newPostHeaders("127.0.0.1:8080")
	h3.Set("Origin", "https://app.example.test")
	f.DecodeHeaders(h3, true)

	// Aggregate counters: rv=2 (h1+h3), ri=1 (h2), mso=0. Single counter series.
	if got, want := f.rc.stats.requestValid.Load(), int64(2); got != want {
		t.Errorf("requestValid AGGREGATE: got %d want %d", got, want)
	}
	if got, want := f.rc.stats.requestInvalid.Load(), int64(1); got != want {
		t.Errorf("requestInvalid AGGREGATE: got %d want %d", got, want)
	}
	if got := f.rc.stats.missingSourceOrigin.Load(); got != 0 {
		t.Errorf("missingSourceOrigin: got %d want 0", got)
	}
}

func TestStats_ThreeCountersUnderHCMStatPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	ctx := envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_csrf"}
	c := &csrfv3.CsrfPolicy{
		FilterEnabled: &corev3.RuntimeFractionalPercent{
			DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	if _, err := New(mustAnyFrom(t, c), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []string{
		"http.ingress_csrf.csrf.request_valid",
		"http.ingress_csrf.csrf.request_invalid",
		"http.ingress_csrf.csrf.missing_source_origin",
	}
	for _, n := range want {
		if c := reg.Counter(n); c == nil {
			t.Errorf("counter %q not registered", n)
		}
	}
}
```

- [ ] **Step 2: Run tests; verify Group 6 PASS**

```bash
go test -race -count=1 -v ./internal/filter/http/csrf/
# expect: all 6 groups PASS
```

- [ ] **Step 3: Append ADR-0124 to `docs/envoy-go/DECISIONS.md`**

ADR-0124 captures: BEHAVIOR_CONTRACT.md `## Stat-name mapping` 26→29-name extension for 3 csrf counters (`request_valid`, `request_invalid`, `missing_source_origin`) + namespace anchor at HCM stat_prefix (no new SN flattening rule; reuses existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor per Rule SN2 from ADR-0061) + drop `shadow_request_invalid` from MVP stat surface (couples to deferred shadow mode; reference Envoy also does not emit it under all-defaults config per §11.6 confirmation) + per-route stats SHARED with listener-level (per §11.9 amendment; **diverges from phase 11 ADR-0117 precedent** which had INDEPENDENT per-route stats; csrf is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern; per-route runtime built via `buildPerRouteRuntime` helper at request time per planner-time decision 5 — the per-route runtimeConfig SHARES the listener-level `*filterStats` pointer) + ADR-0073 wholesale-override applies as-is for data, stats simply not part of the override (NO ADR-0073 amendment paragraph required — phase 11's ADR-0117 amendment paragraph + phase 10's ADR-0110 amendment paragraph both stay landed and unused by phase 12).

The ADR §Decision section explicitly contrasts with phase 11: "phase 11 demonstrated wholesale-override extends to STATEFUL per-route resources via INDEPENDENT stats per ADR-0117; phase 12 demonstrates wholesale-override extends to DATA-ONLY per-route resources with SHARED stats — the inverse pattern; both fall within ADR-0073's original wholesale-override semantics without further amendment."

Acceptance: `grep -nE '^## ADR-0124' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 4: Append PROGRESS.md Task 4 entry + commit**

```bash
git add internal/filter/http/csrf/csrf_test.go \
         docs/envoy-go/DECISIONS.md docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: csrf per-route shared-stats unit tests + 3-counter stat-name discipline [ADR-0124]"
```

SHA-fill follow-up.

*Anchored: SPEC §6.6 §11.6 §11.9 §14.1 group 6; ADR-0061 SN2 rule (HCM-namespace stats); ADR-0117 (phase 11 precedent — explicit divergence noted); planner-time decision 5 (option (b) per-route helper).*

---

## Task 5: `FuzzCsrfPolicyConfigParse` fuzzer

**Files:**
- Create: `internal/filter/http/csrf/fuzz_test.go`
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 5 entry)

The 16th fuzzer in the repo (post-phase-11's `FuzzLocalRateLimitConfigParse`). Fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: never panic; never return `(nil, nil)`; on success the factory invokes without panic. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the csrf filter's `New` factory is a parser. Includes well-formed seeds + malformed-StringMatcher corpus to exercise the parse-time-drop path.

**Precondition:** Task 4 done.
**Artifact:** new `fuzz_test.go` + 30s budget run clean.
**Acceptance:** `go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=30s ./internal/filter/http/csrf/` runs clean.

- [ ] **Step 1: Write `fuzz_test.go`**

```go
package csrf

import (
	"testing"

	csrfv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/csrf/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func FuzzCsrfPolicyConfigParse(f *testing.F) {
	// Seeds: well-formed, malformed-StringMatcher, missing-filter_enabled, etc.
	seeds := []*csrfv3.CsrfPolicy{
		{
			FilterEnabled: &corev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			},
		},
		{
			FilterEnabled: &corev3.RuntimeFractionalPercent{
				DefaultValue: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			},
			AdditionalOrigins: []*matcherv3.StringMatcher{
				{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "app.example.test"}},
				{MatchPattern: &matcherv3.StringMatcher_Prefix{Prefix: "pfx-"}}, // dropped at parse
				{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: ""}},        // dropped (empty)
			},
		},
		{}, // missing filter_enabled — should reject cleanly
	}
	for _, s := range seeds {
		raw, err := proto.Marshal(s)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(any, newTestFactoryCtx())
		// Contract: never (nil, nil); never panic.
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) for input len=%d", len(raw))
		}
		if factory != nil {
			// Smoke: factory must not panic when invoked.
			hf := factory()
			if hf.Decoder == nil {
				t.Fatal("Decoder must be non-nil")
			}
		}
	})
}
```

- [ ] **Step 2: Run a quick verification (1s budget) to catch syntax errors**

```bash
go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=1s ./internal/filter/http/csrf/
```

- [ ] **Step 3: Run the gate budget (30s)**

```bash
go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=30s ./internal/filter/http/csrf/
# expect: clean exit; no crashers
```

- [ ] **Step 4: Append PROGRESS.md Task 5 entry + commit**

```bash
git add internal/filter/http/csrf/fuzz_test.go \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: FuzzCsrfPolicyConfigParse (16th fuzzer; 30s budget green)"
```

SHA-fill follow-up.

*Anchored: SPEC §14.3; ADR-0018 (30s fuzz budget).*

---

## Task 6: `cmd/envoy-go/main.go` register `csrf.New` under `csrf.TypeURL`

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 6 entry)

One-line registration delta + matching package import. Per the alphabetical-after-router ordering (router first; cors → csrf → envoygotest → fault → header_mutation → localratelimit). NO ADR (the registration line is consequence of ADR-0120 § Decision; recorded in PROGRESS).

**Precondition:** Task 5 done; csrf package compiles.
**Artifact:** modified main.go.
**Acceptance:** `go build ./cmd/envoy-go/...` clean; `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 7 (was 6 pre-task).

- [ ] **Step 1: Add the import**

Edit `cmd/envoy-go/main.go` import block (currently lines 28-33). Insert `"github.com/esalaine/envoy-go/internal/filter/http/csrf"` alphabetically between `cors` and `envoygotest`:

```go
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
```

- [ ] **Step 2: Add the Register call**

Edit the registration block (currently lines 114-119). Insert `httpReg.Register(csrf.TypeURL, csrf.New)` between the `cors` and `envoygotest` lines:

```go
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Register(cors.TypeURL, cors.New)
	httpReg.Register(csrf.TypeURL, csrf.New)
	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
	httpReg.Register(fault.TypeURL, fault.New)
	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
	httpReg.Register(localratelimit.TypeURL, localratelimit.New)
```

- [ ] **Step 3: Verify build + lint clean**

```bash
go build ./cmd/envoy-go/...
go vet ./cmd/envoy-go/...
golangci-lint run ./cmd/envoy-go/...
grep -cE 'httpReg.Register' cmd/envoy-go/main.go    # expect: 7
```

- [ ] **Step 4: Append PROGRESS.md Task 6 entry + commit**

```bash
git add cmd/envoy-go/main.go docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: register csrf.New in main.go (router → cors → csrf → envoygotest → ...)"
```

SHA-fill follow-up.

*Anchored: ADR-0120 § Decision; SPEC §4.2.*

---

## Task 7: Fixture infrastructure — `BackendKind` enum + `runner_test.go` spawn helper + blank-import [planner-time decision 9]

**Files:**
- Modify: `test/differential/fixture/fixture.go` (new `HTTPCsrf BackendKind = 11`)
- Modify: `test/differential/runner_test.go` (blank-import + spawn helper + switch case)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 7 entry)

**Precondition:** Task 6 done.
**Artifact:** updated fixture + runner.
**Acceptance:** `go build ./test/differential/...` clean; existing fixtures still pass.

- [ ] **Step 1: Add `HTTPCsrf BackendKind = 11` to `test/differential/fixture/fixture.go`**

Insert after the existing `HTTPLocalRateLimit BackendKind = 10` block (line 211):

```go
	// HTTPCsrf is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0014-http-csrf/backends/backend.go on the pre-allocated
	// port. The backend serves "/" with body "backend\n" (8 bytes;
	// Content-Type: text/plain; Content-Length: 8). No TLS. Introduced by
	// fixture 0014-http-csrf (phase 12 Task 7). Because the backend is a
	// subprocess, the runner's in-process accept counter is NOT incremented.
	HTTPCsrf BackendKind = 11
```

- [ ] **Step 2: Add the spawn helper + switch case + blank-import to `test/differential/runner_test.go`**

(a) Blank-import alphabetically after `0013-http-local-ratelimit`:

```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0014-http-csrf/driver"
```

(b) Switch case in `runFixture` mirroring the `HTTPLocalRateLimit` case:

```go
	case fixture.HTTPCsrf:
		cmd, err := startHTTPCsrfBackend(ctx, repoRoot, port)
		// ... rest of pattern from HTTPLocalRateLimit
```

(c) New spawn helper:

```go
func startHTTPCsrfBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx,
		"go", "run", "./test/fixtures/0014-http-csrf/backends",
		"--port", fmt.Sprintf("%d", port))
	cmd.Dir = repoRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd, cmd.Start()
}
```

- [ ] **Step 3: Verify build clean**

```bash
go build ./test/differential/...
go vet ./test/differential/...
```

The blank-import will fail until the driver package exists (Task 11); for this task, a TEMPORARY workaround is to add an empty-stub `test/fixtures/0014-http-csrf/driver/driver.go` containing only `package driver` so the import resolves. The full driver lands in Task 11.

```bash
mkdir -p test/fixtures/0014-http-csrf/driver
cat > test/fixtures/0014-http-csrf/driver/driver.go <<'EOF'
package driver
// Stub — full implementation in Task 11.
EOF
```

- [ ] **Step 4: Run the existing differential suite to confirm no regressions**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0011|Test.*0012|Test.*0013' -v
# expect: every existing fixture PASS
```

- [ ] **Step 5: Append PROGRESS.md Task 7 entry + commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go \
         test/fixtures/0014-http-csrf/driver/driver.go \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: BackendKind=HTTPCsrf + runner spawn helper + driver stub for blank-import"
```

SHA-fill follow-up.

*Anchored: planner-time decision 9; phase 11 Task 9 precedent.*

---

## Task 8: Fixture 0014 — `backends/backend.go` (Go HTTP backend serving `backend\n` body)

**Files:**
- Create: `test/fixtures/0014-http-csrf/backends/backend.go`
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 8 entry)

Minimal Go HTTP backend bound to a runner-allocated port. Mirrors `test/fixtures/0011-http-fault/backends/backend.go` + `test/fixtures/0013-http-local-ratelimit/backends/backend.go` exactly. ~30 LoC.

**Precondition:** Task 7 done.
**Artifact:** new backend.
**Acceptance:** `go build ./test/fixtures/0014-http-csrf/backends/...` clean.

- [ ] **Step 1: Write `backends/backend.go`**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	port := flag.Int("port", 0, "listen port")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "8")
		w.WriteHeader(200)
		fmt.Fprint(w, "backend\n")
	})
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("0014-http-csrf backend listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Verify build clean + smoke-test**

```bash
go build ./test/fixtures/0014-http-csrf/backends/...
# Smoke: spawn briefly, hit /, kill.
```

- [ ] **Step 3: Append PROGRESS.md Task 8 entry + commit**

```bash
git add test/fixtures/0014-http-csrf/backends/backend.go \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: fixture 0014 backend (HTTP server serving backend\\n body)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3 + §7.4; phase 11 Task 10 precedent.*

---

## Task 9: Fixture 0014 — `envoy.yaml` + `envoy-go.yaml` bootstraps (single-listener topology per planner-time decision 7)

**Files:**
- Create: `test/fixtures/0014-http-csrf/envoy.yaml`
- Create: `test/fixtures/0014-http-csrf/envoy-go.yaml`
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 9 entry)

Single listener `l_main` with two routes (`/` default + `/route-only` per-route TPFC). Listener-level `additional_origins=[app.example.test]` (host:port form per §11.8); per-route `additional_origins=[route-only.test]`. `filter_enabled.default_value: 100/HUNDRED` explicit on both sides per §11.11 amendment. `shadow_enabled` OMITTED on both sides per §11.11 probe #3 baseline. Reference uses STRICT_DNS pointing at `host.docker.internal`; subject uses STATIC. Driver-port placeholders: `{{.AdminPort}}`, `{{.ListenerPort}}`, `{{.BackendPort}}` (or whatever templates the existing harness uses).

**Precondition:** Task 8 done.
**Artifact:** two YAML configs.
**Acceptance:** YAML parses without error against `envoy --mode validate -c <yaml>` AND against envoy-go's bootstrap loader (verified at Task 11 driver invocation).

- [ ] **Step 1: Write `envoy.yaml`** (reference Envoy v1.37.2 STRICT_DNS)

Use the SPEC §7.3 fragment as the starting template; substitute templated ports per the harness convention. Listener filter chain: `[envoy.filters.http.csrf, envoy.filters.http.router]`. Route table: `/route-only` with TPFC + `/` default. Both `csrf` instances explicitly set `filter_enabled.default_value: {numerator: 100, denominator: HUNDRED}`.

```yaml
admin:
  address: { socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} } }
static_resources:
  listeners:
    - name: l_main
      address: { socket_address: { address: 0.0.0.0, port_value: {{.ListenerPort}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_csrf
                route_config:
                  name: rc_main
                  virtual_hosts:
                    - name: vh_main
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/route-only" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.csrf:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy
                              filter_enabled:
                                runtime_key: __route_only_enabled
                                default_value: { numerator: 100, denominator: HUNDRED }
                              additional_origins:
                                - exact: "route-only.test"
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.csrf
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.csrf.v3.CsrfPolicy
                      filter_enabled:
                        runtime_key: __l_main_csrf_enabled
                        default_value: { numerator: 100, denominator: HUNDRED }
                      additional_origins:
                        - exact: "app.example.test"
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: {{.BackendPort}} } } }
```

- [ ] **Step 2: Write `envoy-go.yaml`** (subject; STATIC cluster)

Same shape as envoy.yaml modulo cluster type:

```yaml
clusters:
  - name: c_backend
    type: STATIC
    load_assignment:
      cluster_name: c_backend
      endpoints:
        - lb_endpoints:
            - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: {{.BackendPort}} } } }
```

- [ ] **Step 3: Validate YAML against reference Envoy**

```bash
docker run --rm -v $(pwd)/test/fixtures/0014-http-csrf:/etc/envoy:ro \
  envoyproxy/envoy:v1.37.2 -c /etc/envoy/envoy.yaml --mode validate
# expect: "configuration '/etc/envoy/envoy.yaml' OK" — substituting templated ports with concrete values for the validate run
```

- [ ] **Step 4: Append PROGRESS.md Task 9 entry + commit**

```bash
git add test/fixtures/0014-http-csrf/envoy.yaml test/fixtures/0014-http-csrf/envoy-go.yaml \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: fixture 0014 bootstraps (single listener; filter_enabled=100% explicit per §11.11)"
```

SHA-fill follow-up.

*Anchored: SPEC §7.3 + §11.11 amendment; planner-time decision 7 (single-listener topology).*

---

## Task 10: Fixture 0014 — `expectations.yaml` + `README.md`

**Files:**
- Create: `test/fixtures/0014-http-csrf/expectations.yaml`
- Create: `test/fixtures/0014-http-csrf/README.md`
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 10 entry)

Per ADR-0019, `expectations.yaml` is prose narrative (NOT machine-evaluated; the runner enforces via the driver's per-scenario assertions). README documents fixture overview + 6-scenario list + single-listener bootstrap discipline + the `filter_enabled=100%` discipline note + operator-footgun callout + per-route stats-shared note.

**Precondition:** Task 9 done.
**Artifact:** two YAML/Markdown narrative documents.
**Acceptance:** files exist; cross-refs to SPEC sections present.

- [ ] **Step 1: Write `expectations.yaml`** (per SPEC §7.1)

```yaml
# Phase 12 fixture 0014 expectations (per ADR-0019 — prose; driver enforces).
#
# Topology (per planner-time decision 7): single listener l_main with two routes.
#   - Listener-level CsrfPolicy: additional_origins=[app.example.test], filter_enabled=100%
#   - /route-only TPFC: additional_origins=[route-only.test], filter_enabled=100%
#   - / default route: inherits listener-level
#
# Six scenarios (7 HTTP requests; scenario 7 has 7a + 7b sub-requests):
#
# Scenario 1: same-origin POST allowed
#   Request:  POST / Origin: http://127.0.0.1:<listener-port>
#   Response: 200 + backend body "backend\n" (8 bytes)
#   Counter:  request_valid +1
#
# Scenario 2: cross-origin POST rejected
#   Request:  POST / Origin: https://evil.test
#   Response: 403 + body byte-exact "Invalid origin" (14 bytes, no LF)
#             headers (lowercase wire-form, lexicographic):
#               content-length: 14
#               content-type: text/plain
#               date: <RFC1123>
#               server: envoy
#   Counter:  request_invalid +1
#
# Scenario 3: additional_origins exact-match allowed
#   Request:  POST / Origin: https://app.example.test
#   Response: 200 + backend body
#   Counter:  request_valid +1
#   Note:    additional_origins entry MUST be "app.example.test" host:port form
#            (NOT "https://app.example.test") per §11.8 amendment.
#
# Scenario 4: no source-origin rejected
#   Request:  POST / (no Origin, no Referer)
#   Response: 403 + "Invalid origin" + 4-header set
#   Counter:  missing_source_origin +1
#
# Scenario 5: Referer fallback allowed
#   Request:  POST / Referer: http://127.0.0.1:<listener-port>/somepage (no Origin)
#   Response: 200 + backend body
#   Counter:  request_valid +1
#
# Scenario 7: per-route wholesale-override (with shared stats per §11.9)
#   7a. POST /route-only Origin: https://route-only.test
#         → 200 + backend body
#         → counter: request_valid +1 (AGGREGATES with listener-level series)
#   7b. POST / Origin: https://route-only.test
#         → 403 + "Invalid origin" (matches neither listener-default nor app.example.test)
#         → counter: request_invalid +1 (AGGREGATES — per §11.9 amendment, NO independent
#           per-route counter series; diverges from phase 11 ADR-0117 precedent).
#
# Final counter snapshot (after all 7 requests):
#   request_valid        = 4   (scenarios 1, 3, 5, 7a)
#   request_invalid      = 2   (scenarios 2, 7b)
#   missing_source_origin = 1   (scenario 4)
#
# Prometheus form (after scrape):
#   envoy_http_csrf_request_valid{envoy_http_conn_manager_prefix="ingress_csrf"} 4
#   envoy_http_csrf_request_invalid{envoy_http_conn_manager_prefix="ingress_csrf"} 2
#   envoy_http_csrf_missing_source_origin{envoy_http_conn_manager_prefix="ingress_csrf"} 1
#
# No timing tolerances. csrf is purely synchronous; all responses dispatch within
# microseconds. NO analog to phase 11 fixture 0013 scenario 3's ±10ms refill boundary.
#
# Cross-refs: SPEC §7.1 + §13.1 + ADR-0122 + ADR-0123 + ADR-0124.
```

- [ ] **Step 2: Write `README.md`**

```markdown
# Fixture 0014 — `envoy.filters.http.csrf` differential equivalence

Six scenarios per phase 12 SPEC §7.1; sequential against a single listener `l_main`
with two routes (`/` default + `/route-only` per-route TPFC). Reference Envoy
v1.37.2 (STRICT_DNS) vs envoy-go (STATIC).

## Scenarios

1. **Same-origin POST allowed** — `POST / Origin: http://127.0.0.1:<port>` → 200; `request_valid +1`.
2. **Cross-origin POST rejected** — `POST / Origin: https://evil.test` → 403 + `Invalid origin` (14 bytes, no LF) + 4-header set lowercase wire-form (`content-length`, `content-type`, `date`, `server: envoy`); `request_invalid +1`.
3. **additional_origins exact match** — `POST / Origin: https://app.example.test` → 200; `request_valid +1`. (Entry is `app.example.test` host:port form per §11.8 amendment.)
4. **No source-origin rejected** — `POST /` (no Origin, no Referer) → 403; `missing_source_origin +1`.
5. **Referer fallback** — `POST / Referer: http://127.0.0.1:<port>/somepage` (no Origin) → 200; `request_valid +1`.
7. **Per-route wholesale-override** — (a) `POST /route-only Origin: https://route-only.test` → 200; (b) `POST / Origin: https://route-only.test` → 403. Counter increments AGGREGATE with listener-level series (per §11.9 amendment — diverges from phase 11 ADR-0117 precedent which had INDEPENDENT per-route stats).

(Scenario 6 — GET passthrough — is unit-only per SPEC §2.4 + §14.1 group 3; not in the differential fixture.)

## Single-listener bootstrap discipline (per planner-time decision 7)

All scenarios run against the same listener (single boot; no per-scenario teardown). Driver issues all 7 requests sequentially in one `DriveReference` / `DriveSubject` call. csrf is purely synchronous — no timing tolerances.

## `filter_enabled` PGV discipline (per §11.11 amendment)

Both `envoy.yaml` and `envoy-go.yaml` set `filter_enabled.default_value: {numerator: 100, denominator: HUNDRED}` explicitly on both the listener-level and per-route CsrfPolicy entries. Reference Envoy v1.37.2 PGV-rejects boot if `filter_enabled` is absent OR if `filter_enabled.default_value` is absent — non-negotiable. envoy-go's `New` factory PGV-mirrors per ADR-0121 (validates non-nil presence; the percentage value is silent-ignored at runtime per §1.1 amendment 3).

`shadow_enabled` is OMITTED on both sides per §11.11 probe #3 baseline (Envoy permits omission; envoy-go also accepts; runtime is always-never-shadow on both).

## Operator footgun (per §11.8 amendment)

`additional_origins[].exact` matches the source's `host[:port]` form — NOT the full URL with scheme. Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header. Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port). envoy-go faithfully replicates Envoy's behavior; this is a known footgun in the upstream spec.

## Per-route stats SHARED with listener-level (per §11.9 amendment)

csrf is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern. Phase 11's local_ratelimit precedent (ADR-0117) had INDEPENDENT per-route stats; phase 12 is the inverse pattern — per-route data REPLACES listener data, but counter increments AGGREGATE under the SAME `*filterStats` (one counter series per HCM scope). ADR-0124 captures this.

## Envoy deviation

None — csrf is a normal HTTP filter; no SIGTERM/drain divergence. Per-route TPFC handling is the existing 3-tier `Resolve` per ADR-0073 (most-specific-override).

## Planner-time decisions cross-references

- Decision 5: per-route runtime built via `buildPerRouteRuntime(perRoute, listenerStats)` helper at request time; SHARES listener-level `*filterStats` pointer.
- Decision 7: single-listener topology fits existing `fixture.Driver` contract.
- Decision 8: synthetic `http://` prefix for target-URL parsing (no framework extension).
```

- [ ] **Step 3: Append PROGRESS.md Task 10 entry + commit**

```bash
git add test/fixtures/0014-http-csrf/expectations.yaml test/fixtures/0014-http-csrf/README.md \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: fixture 0014 expectations narrative + README (6 scenarios; shared-stats per §11.9)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3 + §7 + §11.8 + §11.9 + §11.11; ADR-0019 (expectations.yaml prose discipline).*

---

## Task 11: Fixture 0014 — `driver/driver.go` (single-listener 6-scenario sequential orchestration)

**Files:**
- Modify: `test/fixtures/0014-http-csrf/driver/driver.go` (replace stub with full implementation)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 11 entry)

Go driver implementing the SPEC §7.1 + §7.2 6-scenario sequential orchestration via the single-listener topology per planner-time decision 7. ~180 LoC.

**Precondition:** Task 10 done.
**Artifact:** full driver replacing the Task 7 stub.
**Acceptance:** `go build ./test/fixtures/0014-http-csrf/driver/...` clean; `go test -count=1 ./test/differential/ -run 'Test.*0014'` PASS.

- [ ] **Step 1: Replace `test/fixtures/0014-http-csrf/driver/driver.go` stub with full implementation**

The driver shape mirrors the cors / fault / header_mutation precedent (single-listener `fixture.Driver` interface — NOT the `MultiListenerDriver` introduced by 07.2 + used by phase 11). Key methods: `init()` registers the fixture; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPCsrf`; `ReferenceBootstrap` / `SubjectConfig` template the YAMLs; `DriveReference` / `DriveSubject` issue all 7 HTTP requests sequentially + scrape `/stats/prometheus` once at the end.

```go
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() {
	fixture.RegisterFixture("0014-http-csrf", &csrfDriver{})
}

type csrfDriver struct{}

func (*csrfDriver) BackendCount() int                  { return 1 }
func (*csrfDriver) BackendKind() fixture.BackendKind   { return fixture.HTTPCsrf }

func (*csrfDriver) ReferenceBootstrap(ctx fixture.RenderContext) (string, error) {
	return renderTemplate("envoy.yaml", ctx)
}

func (*csrfDriver) SubjectConfig(ctx fixture.RenderContext) (string, error) {
	return renderTemplate("envoy-go.yaml", ctx)
}

func renderTemplate(name string, ctx fixture.RenderContext) (string, error) {
	repo, err := os.Getwd() // adjust per harness convention
	if err != nil {
		return "", err
	}
	path := filepath.Join(repo, "test", "fixtures", "0014-http-csrf", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	tpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// scenario represents one differential probe.
type scenario struct {
	name        string
	method      string
	path        string
	originHdr   string // "" means absent; explicit empty string is set via setEmptyOrigin
	setEmptyOrigin bool
	refererHdr  string
	wantStatus  int
	wantBody    string // "" → don't assert; else exact
}

func scenarios(listenerHost string) []scenario {
	host := listenerHost // e.g., "127.0.0.1:31415"
	return []scenario{
		{name: "1-same-origin", method: "POST", path: "/", originHdr: "http://" + host, wantStatus: 200, wantBody: "backend\n"},
		{name: "2-cross-origin", method: "POST", path: "/", originHdr: "https://evil.test", wantStatus: 403, wantBody: "Invalid origin"},
		{name: "3-additional-origins-match", method: "POST", path: "/", originHdr: "https://app.example.test", wantStatus: 200, wantBody: "backend\n"},
		{name: "4-no-source", method: "POST", path: "/", wantStatus: 403, wantBody: "Invalid origin"},
		{name: "5-referer-fallback", method: "POST", path: "/", refererHdr: "http://" + host + "/somepage", wantStatus: 200, wantBody: "backend\n"},
		{name: "7a-per-route-allow", method: "POST", path: "/route-only", originHdr: "https://route-only.test", wantStatus: 200, wantBody: "backend\n"},
		{name: "7b-per-route-listener-reject", method: "POST", path: "/", originHdr: "https://route-only.test", wantStatus: 403, wantBody: "Invalid origin"},
	}
}

// drive issues all 7 HTTP requests against the listener at host:port and
// returns the captured probe results. Final stats scrape is the caller's
// responsibility (against the admin port).
func drive(ctx context.Context, listenerHost string) ([]probeResult, error) {
	results := make([]probeResult, 0, 7)
	c := &http.Client{Timeout: 5 * time.Second}
	for _, s := range scenarios(listenerHost) {
		url := fmt.Sprintf("http://%s%s", listenerHost, s.path)
		req, err := http.NewRequestWithContext(ctx, s.method, url, nil)
		if err != nil {
			return results, err
		}
		if s.originHdr != "" {
			req.Header.Set("Origin", s.originHdr)
		}
		if s.refererHdr != "" {
			req.Header.Set("Referer", s.refererHdr)
		}
		resp, err := c.Do(req)
		if err != nil {
			return results, fmt.Errorf("scenario %s: %w", s.name, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		results = append(results, probeResult{
			scenario: s,
			status:   resp.StatusCode,
			body:     string(body),
			headers:  resp.Header.Clone(),
		})
	}
	return results, nil
}

type probeResult struct {
	scenario scenario
	status   int
	body     string
	headers  http.Header
}

func (*csrfDriver) DriveReference(ctx context.Context, env fixture.Env) error {
	results, err := drive(ctx, env.ListenerHost())
	if err != nil {
		return err
	}
	env.Capture("reference.results", encodeResults(results))
	stats, err := scrapeStats(ctx, env.ReferenceAdminHost())
	if err != nil {
		return err
	}
	env.Capture("reference.stats", stats)
	return nil
}

func (*csrfDriver) DriveSubject(ctx context.Context, env fixture.Env) error {
	results, err := drive(ctx, env.ListenerHost())
	if err != nil {
		return err
	}
	env.Capture("subject.results", encodeResults(results))
	stats, err := scrapeStats(ctx, env.SubjectAdminHost())
	if err != nil {
		return err
	}
	env.Capture("subject.stats", stats)
	return nil
}

// CompareBytes is the framework's hook for asserting reference == subject. The
// driver exposes the two captured maps via env; the harness compares them
// per-scenario (status + body byte-equal; headers via the existing allow-list).
// The 3 csrf counter deltas must be exactly: request_valid=4, request_invalid=2,
// missing_source_origin=1 on BOTH sides — asserted via the captured stats blobs.
func (*csrfDriver) CompareBytes(env fixture.Env) error {
	// Implementer at Task 11 step 2 fills this body per the harness's existing
	// CompareBytes pattern (extract reference.results vs subject.results,
	// compare per-probe status/body/header-allow-list; extract counter deltas
	// from reference.stats vs subject.stats and assert per-scenario aggregate).
	return nil
}

func encodeResults(results []probeResult) []byte {
	var out bytes.Buffer
	for _, r := range results {
		fmt.Fprintf(&out, "scenario=%s status=%d body=%q\n", r.scenario.name, r.status, r.body)
		// Filter headers per the existing allow-list before emit.
		for k, vs := range r.headers {
			fmt.Fprintf(&out, "  header %s: %s\n", strings.ToLower(k), strings.Join(vs, ", "))
		}
	}
	return out.Bytes()
}

func scrapeStats(ctx context.Context, adminHost string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://%s/stats/prometheus", adminHost)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Compile-time use of net to keep the import live.
var _ net.Listener = (*net.TCPListener)(nil)
```

(Implementer at Task 11 step 2 fills the `CompareBytes` body per the harness's existing pattern; the precise return-error shape and per-probe assertion machinery follows phase 11's `0013-http-local-ratelimit/driver/driver.go` precedent.)

- [ ] **Step 2: Verify build clean**

```bash
go build ./test/fixtures/0014-http-csrf/...
go vet ./test/fixtures/0014-http-csrf/...
```

- [ ] **Step 3: Run the fixture**

```bash
go test -count=1 -v ./test/differential/ -run 'Test.*0014'
# expect: PASS — all 6 scenarios match across reference Envoy + envoy-go
```

If FAIL: capture the diagnostic output (status/body/header diff per scenario; counter delta diff). Common failure modes: (a) `Host` header missing on H1 path → target hostAndPort is empty (cross-checks against probe test for HTTP/1.1 Host injection); (b) per-route TPFC entry missing required `filter_enabled.default_value` (Envoy boots fail PGV — caught at fixture validate); (c) backend body not byte-exact (verify backend.go Content-Length matches body length).

- [ ] **Step 4: Run regression suite**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0011|Test.*0012|Test.*0013|Test.*0014' -v
# expect: 4 fixtures all PASS
```

- [ ] **Step 5: Append PROGRESS.md Task 11 entry + commit**

```bash
git add test/fixtures/0014-http-csrf/driver/driver.go \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "phase 12: fixture 0014 driver — single-listener 6-scenario orchestration"
```

SHA-fill follow-up.

*Anchored: SPEC §7.1 + §7.2 + §7.4; planner-time decision 7 (single-listener) + 9 (HTTPCsrf BackendKind=11).*

---

## Task 12: BEHAVIOR_CONTRACT.md patches per SPEC §13 + ROADMAP row 12 in-progress→done + STATE.md advance + phase-done six-gate verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4-edit bundle per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (row 12 status `in-progress → done` + summary finalize)
- Modify: `docs/envoy-go/STATE.md` (advance to `awaiting next planning`)
- Modify: `docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md` (Task 12 entry — gate outputs)

This task lands the §13 4-edit bundle to `BEHAVIOR_CONTRACT.md` per ADR-0052, flips ROADMAP row 12 status, advances STATE.md, AND verifies the SPEC §3 six-gate phase-done checklist. Per `BOOTSTRAP_PROMPT.md` §7.5 + SPEC §3.

**Precondition:** Task 11 done; fixture 0014 PASS.
**Artifact:** modified BEHAVIOR_CONTRACT + ROADMAP + STATE + PROGRESS gate output.
**Acceptance:** all six gates report green; STATE.md flipped; ROADMAP row 12 = done.

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md §13.1 patch — NEW `### envoy.filters.http.csrf` subsection**

Insert AFTER the existing `### envoy.filters.http.local_ratelimit` subsection (currently at line 1008; verify with `grep -n '^### envoy.filters.http.local_ratelimit'`). Use the verbatim Markdown shape from SPEC §13.1 (the ~75 LoC block specifying field decomposition / method gate / origin trichotomy / comparison algorithm / operator footgun / per-route override semantics / per-route stats SHARED / rejection wire shape / allow-path / stat surface).

- [ ] **Step 2: Apply BEHAVIOR_CONTRACT.md §13.2 patch — `## Stat-name mapping ### 26-name table` 26→29 extension**

Update heading: `### 26-name table` → `### 29-name table`. Append 3 new rows for the csrf counters per SPEC §13.2:

```markdown
| `http.<stat_prefix>.csrf.request_valid`         | counter | filter | csrf | modifying request whose source origin matches target or `additional_origins[].exact` (§11.6) |
| `http.<stat_prefix>.csrf.request_invalid`       | counter | filter | csrf | modifying request whose source origin is determinable but matches neither (§11.6) |
| `http.<stat_prefix>.csrf.missing_source_origin` | counter | filter | csrf | modifying request whose source origin is undeterminable (§11.6) |
```

**NO new tag-extractor preamble** (UNLIKE phase 11 which added the `envoy_local_http_ratelimit_prefix` Rule SN9 preamble); csrf reuses the existing HCM-namespace SN2 extractor — no new pattern needed.

- [ ] **Step 3: Apply BEHAVIOR_CONTRACT.md §13.3 patch — `## Equivalence Matrix` new row**

Append the verbatim row from SPEC §13.3 (the long single-row Markdown patch covering fixture 0014's 6 scenarios + the no-timing-tolerance + no-H2-coverage notes + the per-route stats-SHARED divergence-from-phase-11 callout).

- [ ] **Step 4: Apply BEHAVIOR_CONTRACT.md §13.4 patch — NEW `### Phase 12 forward-pointer notes` subsection**

Append to the existing `## Forward-pointer notes` section. Use the verbatim Markdown from SPEC §13.4 covering: (a) deferred field families (`filter_enabled` PGV-required + percentage-gating deferred to Runtime + hot restart family per §11.11; `shadow_enabled` shadow-mode evaluation deferred per §2.1.3; `additional_origins[].StringMatcher` non-exact variants dropped at PARSE per ADR-0101 §3 per §2.1.1); (b) operator footgun (`additional_origins[].exact` is host:port form NOT full URL with scheme per §11.8 amendment); (c) no-new-tag-extractor note; (d) per-route stats-SHARED note (diverges from phase 11).

- [ ] **Step 5: Update ROADMAP row 12 status**

```bash
# in docs/envoy-go/ROADMAP.md, find row "| 12 | http-filter-csrf | 11 | in-progress | ... |"
# change status: in-progress → done; finalize summary if needed.
```

- [ ] **Step 6: Run gate (a) — `go build ./...` + `go vet ./...` + `golangci-lint run ./...`**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
```

Expected: clean. Capture output to PROGRESS.md Task 12 entry.

- [ ] **Step 7: Run gate (b) — `go test -race ./...` clean**

```bash
go test -race -count=1 ./...
```

Expected: every package PASS. Capture output.

- [ ] **Step 8: Run gate (c) — h2spec re-run at the ADR-0051 pin (53/53 PASS)**

```bash
make h2spec  # or whatever the existing entry point is per phases 09 / 10 / 11 conventions
```

Expected: 53/53 PASS unchanged. Capture output.

- [ ] **Step 9: Run gate (d) — fuzzers (existing 15 + new 1 = 16) clean at 30s budget**

```bash
go test -fuzz=FuzzCsrfPolicyConfigParse -fuzztime=30s ./internal/filter/http/csrf/
# Plus the 15 pre-existing fuzzers (FuzzBootstrapConfigParse, FuzzCORSConfigParse,
# FuzzFaultConfigParse, FuzzHeaderMutationConfigParse, FuzzLocalRateLimitConfigParse,
# FuzzConfigDumpFormat, FuzzAccessLogFormat, etc.). Run via the existing CI script
# that iterates them OR run each individually.
```

Expected: all 16 fuzzers run clean.

- [ ] **Step 10: Run gate (e) — differential fixtures 0000–0014 all green**

```bash
go test -count=1 -v ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014'
```

Expected: every fixture PASS including the new 0014.

- [ ] **Step 11: Verify gate (f) — BEHAVIOR_CONTRACT.md populated**

```bash
grep -nE 'envoy.filters.http.csrf' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -5
grep -nE '29-name table' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -nE 'Phase 12 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -nE 'operator footgun' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

Expected: matches in `## HTTP filter chain`, `## Stat-name mapping`, `## Equivalence Matrix`, and `## Forward-pointer notes`.

- [ ] **Step 12: Update `docs/envoy-go/STATE.md`**

Flip:
- `lifecycle-state` → `awaiting next planning` (or the equivalent post-phase-done state per `BOOTSTRAP_PROMPT.md` §5)
- `next-skill` → `superpowers:brainstorming` (the next §9 family-child cold-starts from the §9 heading per ADR-0106)
- `next-skill-scope` → describes the cold-start: read ROADMAP.md row 12 + BEHAVIOR_CONTRACT.md ### envoy.filters.http.csrf + DECISIONS.md tail (now ADR-0124); the next family-child is selected by the brainstormer per the §9 family list at ROADMAP line 58 (compression / jwt_authn / rbac / ext_authz / ext_proc / oauth2 / buffer / lua / wasm / adaptive_concurrency / admission_control / bandwidth_limit; NOTE: csrf is now landed, so it is no longer a candidate).
- `active-phase` → `<next-family-row-id>` resolved by the next session's planner; this PLAN sets it to a sentinel value (e.g., `<unset — next session resolves>`)
- `last-commit` → the phase-done commit SHA (filled in step 14 SHA-fill follow-up)
- `last-updated` → current date

- [ ] **Step 13: Phase-done commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md \
         docs/envoy-go/phases/12-http-filter-csrf/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 12: http-filter-csrf [ADR-0120, ADR-0121, ADR-0122, ADR-0123, ADR-0124]

Lands envoy.filters.http.csrf under the 07.1 framework.
FIFTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10,
local_ratelimit @ 11).

ROADMAP row 12 flips in-progress → done at this commit.
The §9 family heading at ROADMAP line 56 stays unchanged (headings are
not rows; per ADR-0106).

Five ADRs land:
- ADR-0120: package shape (csrf/ single-token directory matching cors precedent)
  + boot registration ordering (router → cors → csrf → envoygotest → fault →
  header_mutation → local_ratelimit) + decoder-only HTTPFilter (Encoder: nil)
- ADR-0121: runtimeConfig 1/1/1-field decomposition + PGV-mirror filter-internal
  validation discipline at New time (filter_enabled REQUIRED per §11.11
  amendment — MAJOR REVISION from BRAINSTORM "silent-ignore" hypothesis) +
  StringMatcher non-exact dropped at PARSE per ADR-0101 §3
- ADR-0122: origin extraction trichotomy (Origin null literal → empty NO Referer
  fallback; Origin empty/absent → Referer fallback; Origin non-empty unparseable
  → verbatim) + comparison algorithm host:port-only equality (scheme stripped;
  NO normalization; trailing slash stripped via URL parser) + method gate +
  additional_origins host:port matching (operator footgun)
- ADR-0123: rejection path wire shape + body byte-exact "Invalid origin" (14
  bytes, no LF) + 4-header set lowercase wire-form + 403 hardcoded status +
  SendLocalReply reuse from phase 09 fault precedent
- ADR-0124: 26→29-name stat-table extension + 3 csrf counters + namespace anchor
  at HCM stat_prefix (NO new SN flattening rule; reuses envoy_http_conn_manager_prefix
  Rule SN2) + drop shadow_request_invalid from MVP + per-route stats SHARED with
  listener-level (DIVERGES from phase 11 ADR-0117 precedent which had INDEPENDENT
  per-route stats — first production filter to demonstrate "wholesale data-only
  override + shared stats" pattern)

NO framework deltas (thinnest §9 family-row to date — no FactoryCtx field, no
HTTPRegistry method, no PerRouteConfig accessor, no ADR-0073 amendment, no SN
flattening rule).

Differential fixture 0014-http-csrf green (6 scenarios: same-origin allowed,
cross-origin rejected, additional_origins host:port match, missing source-origin
rejected, Referer fallback allowed, per-route wholesale-override with shared
stats; GET passthrough unit-only).

Stats: 3 new counters per HCM stat_prefix (26→29 table extension); reuses
existing envoy_http_conn_manager_prefix Prometheus tag-extraction (Rule SN2).

All six phase-done gates green: build/vet/lint clean; race tests pass; h2spec
53/53 PASS unchanged; 16 fuzzers green at 30s budget; all 15 differential
fixtures (0000–0014) green; BEHAVIOR_CONTRACT.md populated.
EOF
)"
```

- [ ] **Step 14: SHA-fill follow-up commit**

After the phase-done commit lands, capture its SHA and fill into STATE.md / PROGRESS.md (per the per-task SHA-fill discipline established in phases 04+).

```bash
git commit --allow-empty -m "phase 12 follow-up: STATE.md + PROGRESS.md SHA-fill (TBD → <phase-done SHA>)"
```

Or alternatively, edit the docs in-place + commit normally.

*Anchored: SPEC §13.1 + §13.2 + §13.3 + §13.4; ADR-0052 (in-place edit); ADR-0106 (family heading unchanged); SPEC §3 (6-gate checklist); SPEC §15 acceptance; BOOTSTRAP_PROMPT.md §5.3 + §7.5.*

---

## Task 13: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/12-http-filter-csrf/REVIEW.md`

This task drafts the end-of-phase REVIEW.md per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 cadence; populates per the `superpowers:requesting-code-review` skill. Phase 12 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 12. NO new ADRs.

**Precondition:** Task 12 done.
**Artifact:** REVIEW.md.
**Acceptance:** REVIEW.md committed; covers per-task retrospective + carry-forward findings + planner-time decisions retrospective.

- [ ] **Step 1: Invoke the `superpowers:requesting-code-review` skill**

If executing inline: read the skill output and apply its REVIEW shape. If executing subagent-driven: dispatch a code-reviewer subagent with the phase 12 SPEC + PLAN + PROGRESS as context.

- [ ] **Step 2: Draft REVIEW.md mirroring 11's REVIEW.md structure**

The REVIEW typically covers:
- N-1 carry-forward retrospective (review 11's REVIEW for any items requesting phase-12 follow-up; address each)
- Per-task retrospective (any task that landed deviations from PLAN; record the rationale — e.g., if Task 11 needed a CompareBytes refactor due to harness drift, record the finding)
- Planner-time decisions retrospective (each of the 9 decisions: did the implementation validate the choice or expose a flaw? — D1 decoder-only filter; D2 HTTPFilter Encoder: nil; D3 envoy-go-own-wording errors; D4 net/url.Parse; D5 option (b) per-route helper; PLAN-6 single-file `csrf.go`; PLAN-7 single-listener fixture topology; PLAN-8 synthetic `http://` prefix; PLAN-9 BackendKind name)
- Carry-forward findings for phase 13+ (e.g., framework primitives that proved load-bearing — the per-route shared-stats pattern as alternative to phase 11's independent-stats; the synthetic-scheme-prefix idiom for filters that need URL parsing without TLS state; deferrals that warrant scheduling — Runtime + hot restart family for `filter_enabled` percentage-gating + `shadow_enabled` shadow-mode; full StringMatcher engine for non-exact variants on `additional_origins`; any minor tech-debt the next phase can pick up)
- ADR retrospective (each of the 5 ADRs: did the §Decision body hold up under implementation + fixture exercise? — ADR-0120 package shape; ADR-0121 PGV-mirror; ADR-0122 host:port-only equality; ADR-0123 wire shape; ADR-0124 shared-stats)
- Six-gate retrospective (any gate that was non-trivial to satisfy — fixture 0014 single-listener orchestration; per-route shared-stats counter aggregation across reference + subject)
- §1.1 amendment retrospective (the 4 BRAINSTORM-amendments + 3 confirmations all held — note any surprises during implementation)

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/12-http-filter-csrf/REVIEW.md
git commit -m "phase 12: REVIEW — end-of-phase retrospective + N-1 carry-forward"
```

SHA-fill follow-up.

*Anchored: superpowers:requesting-code-review; phase-11 REVIEW precedent (master `0f3a710`).*

---

## Refinement

If during execution the implementer discovers a SPEC ambiguity, a planner-time decision that was not foreseen, or a framework constraint that requires deviation from this PLAN, the implementer:

1. Records the deviation in PROGRESS.md's per-task entry under a `**Deviation:**` line + `**Rationale:**` + `**Anchored:**` cross-reference.
2. If the deviation alters the ADR table, amend the in-task ADR's Consequences section in-place (per the ADR-0089 consequence (b) in-place-edit pattern); do NOT introduce a new ADR for the amendment unless the deviation is structurally significant.
3. If the deviation alters the file-structure table, amend this PLAN's table in a follow-up commit OR record the deviation in PROGRESS.md and let the file-structure table become "as-built" rather than "as-planned" — the implementer's choice based on whether the deviation is broadly reusable for future readers.

Common refinement scenarios anticipated:

- **Synthetic `http://` prefix interferes with an edge-case URL parse** (per planner-time decision 8). If a real-world `Host:` value contains characters that combine pathologically with the prefix (e.g., a URL-encoded `:` in the authority), the synthetic-scheme prefix may cause `net/url.Parse` to misparse. Mitigation: the implementer at Task 3 step 4 unit-tests the IPv6 literal case + URL-encoded edge cases; if a divergence from reference Envoy surfaces, ADR-0122 §Consequences amends in-place to record either (a) a custom hostAndPort helper that matches Envoy's `Http::Utility::Url::initialize` for the divergent case, OR (b) a deliberate divergence with the rationale documented in BEHAVIOR_CONTRACT §13.1's "edge-case fidelity" paragraph.

- **Per-route TPFC PGV-mirror validation fires at request time produces unexpected behavior** (per planner-time decision 5). If a misconfigured per-route entry surfaces in the fixture (which by design has well-formed entries), the request-time validation produces an internal error path. Mitigation: PLAN's position is that misconfigured per-route entries are caught by reference Envoy at boot (PGV) — envoy-go's request-time fallback to listener-level rc on per-route validation failure preserves differential equivalence for well-formed configs. If a deviation surfaces (e.g., the differential gate is sensitive to the fallback path), the implementer at Task 3 step 3 may opt-in to phase 10's `RegisterPerRouteValidator` hook to validate per-route entries at boot — ADR-0121 §Consequences amends.

- **`HTTPFilter` value with `Encoder: nil` surfaces an unanticipated chain framework path** (per planner-time decision 2). If the chain framework's `RunEncodeHeaders` iteration assumes every filter has a non-nil encoder side, the `Encoder: nil` choice could break the chain. Mitigation: the implementer at Task 2 step 3 spot-checks against `internal/filter/http/chain.go` to confirm the iterator skips nil Encoder fields; if not, fall back to the cors precedent (`Decoder: f, Encoder: f` with the encoder-side methods all returning Continue/no-op) and amend ADR-0120 §Consequences to record the choice.

- **Per-route stats-shared invariant surfaces a stats Registry concurrency issue under -race** (per planner-time decision 5 + ADR-0124). The shared `*filterStats` pointer is closure-captured at listener-level `New` and reused by the per-route runtimeConfig at request time; the `*atomic.Int64` increments are race-clean by construction. If the race detector surfaces an issue (e.g., the `runtimeConfig` pointer assignment in `DecodeHeaders` is not atomic), the implementer at Task 4 records the finding + adds the appropriate synchronization. Expected outcome: race-clean (per the lock-free hot-path discipline per SPEC §5.9).

- **fixture 0014 `CompareBytes` body needs harness-specific extension** (per Task 11 step 1). The driver's `CompareBytes` body is sketched — the implementer at Task 11 fills it per the harness's existing pattern. If the harness needs a new primitive (e.g., per-counter delta assertion machinery), the implementer extends `test/differential/fixture/` with the helper + records the extension in PROGRESS.md.

## Post-plan handoff

After Task 13 lands the REVIEW, the orchestrating session:

1. Verifies the phase-done six gates one more time (sanity check) per Task 12.
2. Verifies STATE.md is at `awaiting next planning` with `next-skill: superpowers:brainstorming`.
3. Pushes the phase 12 worktree branch to origin (per the user's persistent preference: "after a clean local merge/commit on master with tests green, push without asking" recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`).
4. Hands off to the next session, whose first action is to invoke `superpowers:brainstorming` against §9's HTTP filters family for the next family-child (per ADR-0106 + STATE.md + BRAINSTORM.md Decision 9 — the next family-child cold-starts from the §9 heading + the just-shipped phase 12 artefacts; no sibling-stub was authored).

The phase 12 work is complete when:

- All 13 tasks in this PLAN have green checkmarks in PROGRESS.md.
- Phase-done commit + SHA-fill follow-up are on master.
- REVIEW.md is committed.
- STATE.md reflects the post-12 lifecycle state.
- The branch is pushed to origin.





