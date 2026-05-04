# Phase 10 — HTTP filter `envoy.filters.http.header_mutation` (`internal/filter/http/header_mutation/`, differential fixture `0012-http-header-mutation`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` extension) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 10), §9 (HTTP filters family — phase 10 is the THIRD top-level row to land under the §9 family heading after cors @ 07.1 and fault @ 09 per ADR-0106 settled by phase 09); `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; 1348 lines, 16 sections, **read in full**); `docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md` (the autonomous-brainstorm artefact at master `ad7c129` that the SPEC distils §§1–12 from — 13 Decisions + §9 empirical-pin obligations all executed at SPEC time; consult when the SPEC's "what" needs the BRAINSTORM's "why"); `docs/envoy-go/phases/09-http-filter-fault/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (closed read-only history; 09's PLAN at master `<phase-09 PLAN SHA>` is the structural precedent — task-numbering, TDD-step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections; phase-09 used 17 tasks for ~430 LoC production code + ~590 LoC fixture); `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` (the cors precedent's PLAN — the per-filter package-shape phase 10 inherits); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0107 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format — ADR-0112 + ADR-0113 in this phase follow the deferral-ADR format, **ADR-0044** ADR-on-impl convention, **ADR-0045** planner-time-split discipline (~25 tasks / ~1500 LoC thresholds — both well under for this phase per `## Scope check` below), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0061** stats Registry / SN1–SN8 flattening rules — phase 10 emits ZERO new stats (per SPEC §11.3 confirmation), the 22-name table extended by phase 09 stays UNCHANGED, **ADR-0071** HTTP-filter framework chain-shape + factory pattern + iteration-protocol surface — phase 10's filter is the FIRST production filter to perform PROGRAMMABLE encode-side state mutation on the non-error path (cors injects fixed 3-header set; fault never reaches encode on the normal path), **ADR-0072** HTTPRegistry threaded constructor map + factory typed_config validation contract — phase 10's `New` factory mirrors Envoy v1.37.2's CONFIG-LOAD-TIME protected-header rejection per SPEC §11.1 + ADR-0111 verbatim message format, **ADR-0073** typed_per_filter_config 3-tier merge (most-specific override) — **AMENDED (not superseded) by ADR-0110**: the most-specific-override discipline remains the DEFAULT model (used by cors + fault); filters whose proto semantics demand multi-tier evaluation (header_mutation per its `most_specific_header_mutations_wins` flag) opt into the new `ResolveAllTiers` accessor + per-filter accessor-choice discipline, **ADR-0074** filter set: cors + envoy_go_test — phase 10 adds header_mutation as the FOURTH real production filter (after cors, envoygotest, fault) under the same package-shape discipline, **ADR-0075** sendLocalReply enters encode chain at filter[len-1] — UNCHANGED in phase 10 (header_mutation never short-circuits via SendLocalReply; it's a synchronous Continue-only filter on both decode and encode paths), **ADR-0100** FactoryCtx framework extension (`Stats *stats.Registry` + `StatPrefix string`) — UNCHANGED in phase 10 (header_mutation does NOT consume `ctx.Stats` / `ctx.StatPrefix` per SPEC §11.3 zero-stats confirmation; the 3-field FactoryCtx stays as-is), **ADR-0101** runtimeConfig shape + parser pattern — phase 10's `runtimeConfig` mirrors fault's structurally (3 consumed fields + 1 silent-ignored field per SPEC §6.2), **ADR-0106** §9 HTTP filters family expansion shape (flat top-level rows + no-sibling-stub) — UNCHANGED in phase 10 (phase 10 is a flat top-level row, not a sub-phase of any §9 parent); ADR-0107 is the verified DECISIONS.md tail at master `f339c12` (10 SPEC commit); phase 10's six anticipated ADRs land at ADR-0108..ADR-0113 per SPEC §8); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## HTTP filter chain` umbrella at line 723 hosts the new `### envoy.filters.http.header_mutation` subsection per SPEC §13.1, inserted AFTER the existing `### envoy.filters.http.fault` subsection at line 862 landed by phase 09; `## Stat-name mapping ### 22-name table` at line 131 is UNCHANGED in phase 10 per SPEC §11.3; `## Timing tolerances` is UNCHANGED in phase 10 per SPEC §13.3 (synchronous filter; no time-bounded assertions); `## Equivalence Matrix` at line 9 gains one new row per SPEC §13.4; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 10 — phase 10 is a pure HTTP-layer filter addition; touches no codec/framer/HPACK paths; the h2spec gate at 53/53 PASS is mechanical re-run); `docs/envoy-go/ROADMAP.md` (row `10` per the SPEC commit's row-flip; row `10` flips `in-progress → done` at this phase's phase-done; the §9 HTTP filters family heading at row 56 stays unchanged across all §9-family-row landings per ADR-0106); `internal/filter/http/cors/cors.go` (the package-shape precedent header_mutation inherits — TypeURL constant + New factory + filter struct implementing both StreamDecoderFilter + StreamEncoderFilter; cors's encode-side pattern of calling `f.dcb.RequestRouteConfig()` from BOTH decode AND encode bodies is the precedent for phase 10's decoder-only callback design per planner-time decision 1 below); `internal/filter/http/fault/fault.go` (the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution via `routeConfigOrListener`; phase 10 mirrors the structure modulo no-async-resume, no-stats, no-concurrency-cap, multi-tier-instead-of-most-specific); `internal/filter/http/types.go` (FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilter + HTTPFilterFactory + FilterInstanceFactory + FactoryCtx — UNCHANGED in phase 10; the 3-field FactoryCtx per ADR-0100 stays as-is since header_mutation does not consume Stats/StatPrefix); `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + RequestRouteConfig — phase 10 ADDS one new method `RequestRouteConfigsAllTiers()` returning the 3-tuple `(route, vhost, rc proto.Message)` per ADR-0110); `internal/filter/http/registry.go` (HTTPRegistry — boot-time-populated, freeze-after-boot per ADR-0072; phase 10 ADDS one new method `RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)` per planner-time decision 3 below + ADR-0110); `internal/filter/http/perroute.go` (3-tier merge per ADR-0073; phase 10 ADDS `ResolveAllTiers` per ADR-0110 + threads the registry's per-route-validators into `BuildPerRouteConfig` per planner-time decision 3); `internal/filter/http/chain.go` (per-stream state machine that wires the new callback — phase 10 EXTENDS `decoderCB` with the new `RequestRouteConfigsAllTiers()` method).

**Goal:** Land envoy-go's `envoy.filters.http.header_mutation` HTTP filter — the THIRD production HTTP filter after cors (07.1) and fault (09), and the SECOND top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family after fault. Concretely (per SPEC §1 + §4): a new `internal/filter/http/header_mutation/` package owning the filter implementation under the cors + fault precedents' package-shape discipline (`header_mutation.go` + `header_mutation_test.go` + `doc.go` + `fuzz_test.go`; ~280 + ~320 + ~40 + ~50 = ~690 LoC; ADRs 0108, 0109, 0111); a small framework extension to `internal/filter/http/perroute.go` adding a sibling `ResolveAllTiers(filterName, routeIdx) (route, vhost, rc proto.Message)` method (~60 LoC delta) + matching `internal/filter/http/perroute_test.go` extension (~100 LoC delta) per ADR-0110 amending ADR-0073 (multi-tier evaluation accessor; cors + fault continue to use most-specific `Resolve` per ADR-0073's default model); a new framework callback `DecoderFilterCallbacks.RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)` on the existing decoder-callbacks interface (~30 LoC delta on `callbacks.go` + ~10 LoC delta on `chain.go` decoderCB impl) per ADR-0110 — DECODER-ONLY per planner-time decision 1 (mirrors the cors precedent which calls `f.dcb.RequestRouteConfig()` from BOTH decode AND encode bodies, avoiding two parallel callback surfaces); a new framework hook `HTTPRegistry.RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)` consumed by `BuildPerRouteConfig` to surface per-route protected-header violations as boot-time errors (~50 LoC framework delta on `registry.go` + `perroute.go`) per planner-time decision 3 + ADR-0110 (eager per-route validation lifecycle — surfaces operator errors at boot, mirroring Envoy v1.37.2's CONFIG-LOAD-TIME enforcement per SPEC §11.1 + ADR-0111); a `cmd/envoy-go/main.go` one-line registration delta (`httpReg.Register(header_mutation.TypeURL, header_mutation.New)` inserted alphabetically after the existing fault registration, plus the matching package import; ~3 LoC delta); a NEW differential fixture `0012-http-header-mutation` (`test/fixtures/0012-http-header-mutation/`) with `envoy.yaml` + `envoy-go.yaml` (per §7.4 verbatim with TWO listeners `l_lws` + `l_mws` per §11.5 precedent) + `expectations.yaml` + `README.md` + `driver/driver.go` (four-scenario orchestration per §7.1) + `backends/backend.go` (minimal Go HTTP backend on port 18012 per §7.5; ~590 LoC); a NEW `BackendKind` enum value `HTTPHeaderMutation BackendKind = 9` in `test/differential/fixture/fixture.go` + a matching `startHTTPHeaderMutationBackend` spawn helper in `test/differential/runner_test.go` + the blank-import for the fixture driver (~25 LoC delta — the SPEC §4.3 reference to `test/differential/runner.go` is a SPEC erratum: the actual fixture-registration site is `test/differential/runner_test.go`'s blank-import block per planner-time decision 11 below, mirroring 09 PLAN's identical erratum reconciliation); a NEW fuzzer `FuzzHeaderMutationConfigParse` (~50 LoC; 30s budget per ADR-0018; thirteenth fuzzer overall — SHIPPED per planner-time decision 6 below); a `BEHAVIOR_CONTRACT.md` in-place edit per SPEC §13 (NEW `### envoy.filters.http.header_mutation` subsection under the existing `## HTTP filter chain` umbrella per §13.1 inserted AFTER the existing fault subsection; `## Stat-name mapping ### 22-name table` UNCHANGED per §13.2; `## Timing tolerances` UNCHANGED per §13.3; `## Equivalence Matrix` new row per §13.4; ADR-0073 / ADR-0074 forward-pointer notes per §13.5; ADR-0052 in-place edit authorisation carries forward); six new ADRs ADR-0108..ADR-0113 per SPEC §8 (ADR-0108 package shape + boot registration; ADR-0109 runtimeConfig shape + 3-field-consumed / 1-field-silent-ignore decomposition + AppendAction × 4 mapping table + `keep_empty_value` semantics + multi-value collapse/preserve per §11.4; ADR-0110 multi-tier per-route evaluation framework extension + `ResolveAllTiers` accessor + `RequestRouteConfigsAllTiers` callback + per-route-validator hook + per-filter accessor-choice discipline + `most_specific_header_mutations_wins` cross-tier algorithm + amends ADR-0073; ADR-0111 protected-header set + CONFIG-LOAD-TIME rejection + verbatim error format mirroring Envoy v1.37.2 per §11.1 MAJOR amendment to BRAINSTORM Decision 11; **ADR-0112 `mutations.query_parameter_mutations[]` DEFERRED** per ADR-0040 deferral format coupled to KeyValueMutation triple + path-query rewriting subsystem; **ADR-0113 header-value formatter substitution DEFERRED** per ADR-0040 deferral format coupled to Envoy command-string subsystem). After phase 10, the project has proven its twelfth-leading-edge engineering claim per SPEC §1: *envoy-go's HTTP filter framework can host a programmable header-rewrite primitive that exercises both decode-side and encode-side state mutation under traffic; the framework's per-route accessor surface extends from a single `Resolve` (most-specific, per ADR-0073) to a sibling `ResolveAllTiers` (multi-tier, per ADR-0110) with no impact on existing cors/fault per-route discipline; per-filter accessor choice becomes the load-bearing model for filters whose proto semantics demand multi-tier vs. most-specific evaluation; the protected-header set is enforced at config-load time (boot-fail-fast per ADR-0072) — the registry gains a per-route-validator hook surface that future filters with similar invariants reuse; zero new stats are emitted (analogous to cors per ADR-0074 — not every HTTP filter is stat-bearing); all under flat top-level row expansion (per ADR-0106).* This is the THIRD §9 family-row to land; subsequent filters (compression, local_ratelimit, jwt_authn, …) follow the same row-as-its-own-phase pattern. ROADMAP row `10` flips `in-progress → done` AT the phase-done commit; the §9 family heading at ROADMAP line 56 stays unchanged (headings are not rows; per ADR-0106); STATE.md flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle.

**Architecture:** The 10 surface is the additive registration of one new HTTP filter under `internal/filter/http/` plus a small per-route framework extension threading multi-tier accessor + per-route-validator hooks into the existing `internal/filter/http/perroute.go` + `internal/filter/http/callbacks.go` + `internal/filter/http/registry.go` + `internal/filter/http/chain.go` set. The header_mutation filter's `New` factory runs at HCM-build time per ADR-0072's two-step pattern: (a) parses + validates the typed_config Any (rejects `tc == nil`, malformed Any, AND each mutation's `headerName` against the 6-name protected set per §11.1 — returns a non-nil error mirroring Envoy v1.37.2's verbatim message `:-prefixed or host headers may not be modified` formatted as `header_mutation: %q is :-prefixed or host; may not be modified`); (b) constructs a `*runtimeConfig` capturing the 3 consumed proto fields per §6.2 (`requestOps []compiledMutationOp`, `responseOps []compiledMutationOp`, `mostSpecificHeaderMutationsWins bool`); (c) registers a per-route-validator with the registry per planner-time decision 3 (`ctx.Registry.RegisterPerRouteValidator("envoy.filters.http.header_mutation", validatePerRouteHeaderMutation)` — the validator runs `compileOps` on each tier's request/response mutation lists at HCM-build time, inside `BuildPerRouteConfig` after the proto unmarshal, returning a non-nil error on the first protected-header violation); (d) returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{cfg: rc}` per request. The per-instance `*filter` implements both `StreamDecoderFilter` and `StreamEncoderFilter` per the cors + fault precedents (BOTH decode-side AND encode-side carry mutation logic; this is the FIRST production filter to perform programmable encode-side mutation per ADR-0109). `DecodeHeaders` body discipline (per §6.6): apply listener-level `cfg.requestOps` FIRST (per the proto comment at `header_mutation.pb.go:141–142`); if `f.dcb != nil`, call `f.dcb.RequestRouteConfigsAllTiers()` to retrieve the 3-tuple `(routeMsg, vhMsg, rcMsg)` of unmerged per-tier configs; compile each non-nil tier's request_mutations into `[]compiledMutationOp` via `compileForRequest`; apply the three tiers in flag-controlled order (default `mostSpecificHeaderMutationsWins=false` → Route → VirtualHost → RouteConfiguration; flipped → RouteConfiguration → VirtualHost → Route per §6.5 algorithm + §11.5 empirical confirmation); return `Continue`. `EncodeHeaders` body discipline (per §6.8): SYMMETRIC algorithm against response headers using the SAME `f.dcb.RequestRouteConfigsAllTiers()` callback (per planner-time decision 1 — DECODER-ONLY callback used from BOTH decode AND encode bodies, mirroring the cors precedent at `cors.go:163` which calls `f.dcb.RequestRouteConfig()` from EncodeHeaders); the same per-tier configs resolved fresh per encode call (no caching per planner-time decision 2); apply listener-level `cfg.responseOps` FIRST then per-tier `responseOps` in flag-controlled order; return `Continue`. `OnDestroy` is no-op (no timers, no async state — header_mutation is the project's most concurrency-trivial production filter after router). `DecodeData` / `EncodeData` / `DecodeTrailers` / `EncodeTrailers` are pass-through (DataContinue / TrailersContinue). Per-route 3-tier multi-evaluation uses the new framework method `PerRouteConfig.ResolveAllTiers(filterName, routeIdx)` per ADR-0110 — sibling to existing `Resolve` (most-specific override per ADR-0073), reading directly from the existing `p.scopes[routeIdx].route` / `p.scopes[routeIdx].vhost` / `p.rc` maps without the most-specific selection logic, returning the unmerged 3-tuple with nil entries for tiers without configs. The cache (`p.cache`) is NOT consulted by `ResolveAllTiers` (the existing cache key is `(filterName, routeIdx) → single proto.Message`; multi-tier returns 3 messages with different cache shape; per planner-time decision 2 the per-request map-read is sub-microsecond and not worth caching). Per-route protected-header validation runs at HCM-build time inside `BuildPerRouteConfig` (after the existing proto unmarshal step, before the function returns) per planner-time decision 3 + ADR-0110: the registry's per-route-validators map is consulted for each filter name in `chainNames`; the validator function (registered by header_mutation's `New` factory) runs `compileOps` against each tier's HeaderMutationPerRoute mutations slices, returning the first protected-header error. Concurrency model: per-instance state is race-free by the single-goroutine-per-stream invariant per ADR-0071 (no synchronization needed); `*runtimeConfig` is read-only after `New` (multiple per-request `*filter` instances share via closure capture — read-only sharing is race-free); no timer goroutines, no shared atomic state, no SendLocalReply path — the maximally simple concurrency model per SPEC §5.7. The race detector run under gate (b) validates by construction; one explicit race-detector test `TestHeaderMutation_MultiTierConcurrentRequests` per SPEC §12 deferred decision 7 fires DecodeHeaders concurrently with shared `*runtimeConfig` to validate the read-only-shared-cfg assumption mechanically. Differential surface: fixture `0012-http-header-mutation` runs 4 scenarios per §7.1 (listener-only, per-route override, multi-tier flag=false least-specific-wins, multi-tier flag=true most-specific-wins) under TWO listeners (`l_lws` flag=false on :10012; `l_mws` flag=true on :10013) sharing identical per-route tier configurations + a small static backend probe per §7.5; NO stat assertions (zero new stats per §11.3); NO timing assertions (synchronous filter); response status + body + post-mutation header set asserted byte-equivalent across reference Envoy v1.37.2 vs envoy-go.

**Tech Stack:**
- Go 1.23 (unchanged from 09; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `errors`, `fmt`, `net/http`, `strings` — the new `internal/filter/http/header_mutation/` package consumes only stdlib (no new module imports introduced by 10).
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3` (NEW import in this phase) — `*envoyextensionsfiltershttpheadermutationv3.HeaderMutation` proto + `*HeaderMutationPerRoute` proto. Already present in `go.sum`'s transitive closure (the go-control-plane module-level dependency is unchanged from 09; no `go.mod` bump needed — verified at `## Execution preconditions` step 11 below).
- `github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3` (NEW import in this phase) — `*HeaderMutation` (the per-mutation primitive with `Action.Remove | Action.Append` oneof) consumed by `compileOps`. Module already in closure.
- `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (existing; introduced by phase 04) — `HeaderValueOption_HeaderAppendAction` enum (4 values: APPEND_IF_EXISTS_OR_ADD, ADD_IF_ABSENT, OVERWRITE_IF_EXISTS_OR_ADD, OVERWRITE_IF_EXISTS) reused directly per SPEC §6.4 (NOT redefined locally — avoid drift).
- `google.golang.org/protobuf/types/known/anypb` (existing; introduced by 07.1) — `*anypb.Any` typed_config carrier consumed by `New(tc, ctx)`.
- `google.golang.org/protobuf/proto` (existing; introduced by 07.1 perroute) — `proto.Message` interface used for the per-route-validator function signature + `ResolveAllTiers` return type.
- `github.com/esalaine/envoy-go/internal/filter/http` (existing; introduced by phase 07.1, extended in phase 09 with FactoryCtx Stats + StatPrefix) — `FactoryCtx` (UNCHANGED in phase 10; the 3-field shape stays as-is per ADR-0100), `HTTPFilter`, `HTTPFilterFactory`, `FilterInstanceFactory`, `StreamDecoderFilter`, `StreamEncoderFilter`, `FilterHeadersStatus`, `FilterDataStatus`, `FilterTrailersStatus`, `Continue`, `DataContinue`, `TrailersContinue`, `DecoderFilterCallbacks` (EXTENDED in Task 3 with `RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message)`), `EncoderFilterCallbacks` (UNCHANGED — encode-side header_mutation reads per-tier configs via the decoder-only callback per planner-time decision 1), `HTTPRegistry` (EXTENDED in Task 4 with `RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)` + a `perRouteValidators map[string]func(proto.Message) error` field), `BuildPerRouteConfig` (EXTENDED in Task 4 to consult the registry's per-route-validators on each parsed proto.Message), `PerRouteConfig` (EXTENDED in Task 2 with `ResolveAllTiers` sibling method).
- `github.com/esalaine/envoy-go/internal/filter/http/cors` (existing; the package-shape precedent header_mutation mirrors — TypeURL constant + New factory + filter struct + decoder + encoder + OnDestroy + per-route resolution via cb.RequestRouteConfig).
- `github.com/esalaine/envoy-go/internal/filter/http/fault` (existing; the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution via `routeConfigOrListener`; phase 10 mirrors the structure modulo no-async-resume, no-stats, no-concurrency-cap, multi-tier-instead-of-most-specific).
- `github.com/esalaine/envoy-go/test/differential/fixture` (existing; extended in Task 11 with a new `BackendKind` enum value `HTTPHeaderMutation` per planner-time decision 11).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0012's reference image AND the source of the SPEC §11.1–§11.5 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 10's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 10 — phase 10 touches no codec/framer/HPACK paths; the conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0012's reference (Envoy in a Docker container) — same harness as 06.1/06.2/07.1/07.2/08.1/08.2/09's fixtures consume; phase 10 does NOT extend the harness's optional driver-side interfaces.
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's header_mutation filter implementation; any third-party header-mutation library. Test-side use is also forbidden. The `go.mod` post-10 must not list any new header-mutation-related runtime dependencies.

---

## Scope check — why phase 10 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 PLAN's component-table convention):

- `internal/filter/http/header_mutation/doc.go` ~40
- `internal/filter/http/header_mutation/header_mutation.go` ~280 + `header_mutation_test.go` ~320 = ~600
- `internal/filter/http/header_mutation/fuzz_test.go` (OPTIONAL → settled SHIP per planner-time decision 6) ~50
- `internal/filter/http/perroute.go` `ResolveAllTiers` method + per-route-validator integration in `BuildPerRouteConfig` ~+90 = ~+90
- `internal/filter/http/perroute_test.go` extension (`ResolveAllTiers` tests + per-route-validator integration tests) ~+150 = ~+150
- `internal/filter/http/callbacks.go` `RequestRouteConfigsAllTiers` method addition on DecoderFilterCallbacks ~+15 = ~+15
- `internal/filter/http/chain.go` `decoderCB.RequestRouteConfigsAllTiers` impl ~+15 = ~+15
- `internal/filter/http/chain_test.go` (or callbacks_test.go per the codebase's existing test-file split — implementer settles at Task 3 by `grep -l RequestRouteConfig internal/filter/http/*_test.go`) extension covering the new callback ~+40 = ~+40
- `internal/filter/http/registry.go` `RegisterPerRouteValidator` + `perRouteValidators` field ~+30 = ~+30
- `internal/filter/http/registry_test.go` extension (per-route-validator registration + freeze gate + lookup) ~+50 = ~+50
- `cmd/envoy-go/main.go` one new `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` line + matching import ~+3 = ~+3
- `test/fixtures/0012-http-header-mutation/` (NEW directory — note: SPEC §4.3 says `test/differential/0012-http-header-mutation/`, planner-time decision 10 corrects to `test/fixtures/0012-http-header-mutation/` per the existing 0010-/0011-precedent location) — `envoy.yaml` ~150 + `envoy-go.yaml` ~150 + `expectations.yaml` ~70 + `README.md` ~80 + `driver/driver.go` ~220 + `backends/backend.go` ~50 = ~720
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPHeaderMutation BackendKind = 9`) + doc-comment ~+15 = ~+15
- `test/differential/runner_test.go` blank-import addition + new `startHTTPHeaderMutationBackend` spawn helper ~+25 = ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.header_mutation` subsection ~95 + §13.4 equivalence-matrix row ~3 + §13.5 forward-pointer notes ~10 = ~+108 (§13.2 22-name table + §13.3 timing-tolerances UNCHANGED per SPEC) = ~+108
- `docs/envoy-go/DECISIONS.md` (six ADRs ADR-0108..ADR-0113) ~+360 = ~+360
- `docs/envoy-go/ROADMAP.md` row `10` `in-progress → done` flip + (UNCHANGED) §9 family heading at line 56 ~+1 net = ~+1
- `docs/envoy-go/STATE.md` advance to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` (NEW; lifecycle artefact) ~600 (per-task entry)
- `docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md` (NEW; lifecycle artefact) ~180

**Production code: ~280 LoC (filter impl) + ~150 LoC (framework deltas: 90 perroute + 30 registry + 15 callbacks + 15 chain) + ~3 LoC main.go = ~430 LoC production + ~370 LoC tests + ~50 LoC fuzzer + ~720 LoC fixture YAML/Go + ~470 LoC docs ≈ ~2050 LoC total** (production-only ~430 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~430 LoC; task count below is **18**, comfortably under the 25 limit). The SPEC's anticipated 6-ADR cluster (ADR-0108..ADR-0113) lands across 18 tasks per the table at `## ADRs introduced by this plan` below; no task lands more than 3 ADRs simultaneously. SPEC §1.3 (per BRAINSTORM Decisions 12, 13 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 10 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~12–18 tasks per SPEC §1.4 estimate; this PLAN lands at 18 tasks (the upper bound, driven by the framework-extension surface — 3 framework-piece tasks 2/3/4 vs phase 09's single FactoryCtx-extension Task 2 — plus the standard 5-task fixture cluster).

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/header_mutation/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`HeaderMutation` proto with 3-field consumed `mutations.request_mutations[]`, `mutations.response_mutations[]`, `most_specific_header_mutations_wins` + 1-field silent-ignore `mutations.query_parameter_mutations[]` per ADR-0109; `HeaderMutationPerRoute` proto with 1-field-consumed `mutations` decomposition); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (Continue on DecodeHeaders + EncodeHeaders only — no StopIteration; no SendLocalReply; no async-resume; no body / trailers states exercised); (d) the multi-tier per-route discipline (per ADR-0110); (e) the protected-header set + CONFIG-LOAD-TIME enforcement (per ADR-0111 + §11.1 verbatim); (f) the cross-cutting ADR anchors (ADR-0108 / ADR-0109 / ADR-0110 / ADR-0111 / ADR-0112 / ADR-0113). Mirrors `internal/filter/http/cors/doc.go` + `internal/filter/http/fault/doc.go` shape (~40 LoC precedent). Per SPEC §4.1. |
| `internal/filter/http/header_mutation/header_mutation.go` | NEW | Filter implementation. **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Unexported types (per SPEC §6.2 + §6.3 + §6.4):** `runtimeConfig` struct (3 fields per §6.2: `requestOps []compiledMutationOp`, `responseOps []compiledMutationOp`, `mostSpecificHeaderMutationsWins bool`); `mutationOpKind uint8` type with two values (`kindRemove`, `kindAppend`); `compiledMutationOp` struct (5 fields per §6.4: `kind`, `headerName`, `headerValue`, `appendAction`, `keepEmptyValue`); `filter` struct (`cfg *runtimeConfig` + `dcb envoyhttp.DecoderFilterCallbacks` + `ecb envoyhttp.EncoderFilterCallbacks`). **Helpers:** `compileOps(in []*commonmutationrulesv3.HeaderMutation) ([]compiledMutationOp, error)` (parses + validates against protected-header set; used by both New and the per-route-validator `validatePerRouteHeaderMutation`); `isProtectedHeader(name string) bool` (prefix-check on `:` + case-insensitive equality on `host` per planner-time decision 4); `applyOps(headers http.Header, ops []compiledMutationOp)` (the per-tier mutation-application loop); `applyAppendAction(headers http.Header, op compiledMutationOp)` (the AppendAction × 4 switch); `compileForRequest(msg proto.Message) []compiledMutationOp` (per-request type-assert + recompile of HeaderMutationPerRoute → request-mutations slice); `compileForResponse(msg proto.Message) []compiledMutationOp` (symmetric for response_mutations); `validatePerRouteHeaderMutation(msg proto.Message) error` (the per-route-validator registered with the framework via `ctx.Registry.RegisterPerRouteValidator` per planner-time decision 3 — runs `compileOps` on each tier's request + response mutations, returning first protected-header error). **DecodeHeaders body** (per SPEC §6.6): apply listener-level cfg.requestOps; if dcb non-nil, resolve all 3 tiers via dcb.RequestRouteConfigsAllTiers; compile each via compileForRequest; apply in flag-controlled order (false=Route→VHost→RC; true=RC→VHost→Route); return Continue. **EncodeHeaders body** (per SPEC §6.8): symmetric on response side using SAME dcb.RequestRouteConfigsAllTiers per planner-time decision 1. **Pass-through methods:** OnDestroy + DecodeData + EncodeData + DecodeTrailers + EncodeTrailers all no-op. Per SPEC §6.1–§6.8. |
| `internal/filter/http/header_mutation/header_mutation_test.go` | NEW | Unit tests per SPEC §14.1: `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_ProtectedHeader_*` (table-driven across the 6-name protected set: `:method`, `:path`, `:authority`, `:scheme`, `:status`, `host` lowercase, `Host` titlecase, `HOST` uppercase, plus response-side `:status` rejection per §11.1 (c)), `TestNew_HappyPath_ListenerLevelOnly`, `TestRuntimeConfig_FieldExtraction`, `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored`, `TestCompiledMutationOp_AllAppendActionsParse` (table-driven across the 4 enum values), `TestCompiledMutationOp_RemoveAndAppend`, `TestApplyOps_*` per the AppendAction × 4 + Remove + keep_empty_value boundary table per §11.2 + multi-value collapse/preserve per §11.4 (full table from SPEC §14.1), `TestDecodeHeaders_ListenerLevel_NoPerRoute`, `TestDecodeHeaders_PerRoute_RouteOnly`, `TestDecodeHeaders_MultiTier_FlagFalse`, `TestDecodeHeaders_MultiTier_FlagTrue`, `TestDecodeHeaders_MultiTier_TwoOfThree_*` (3 combinations: route+vhost, route+rc, vhost+rc), `TestEncodeHeaders_Symmetric`, `TestPerRouteProtectedHeader_RegistersValidator` (asserts that calling `New` registers the per-route-validator with the registry per planner-time decision 3), `TestHeaderMutation_MultiTierConcurrentRequests` (race-detector cycle test per SPEC §12 deferred decision 7). |
| `internal/filter/http/header_mutation/fuzz_test.go` | NEW (OPTIONAL → SHIPPED per planner-time decision 6) | `FuzzHeaderMutationConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the header_mutation filter's `New` factory is a parser. ~50 LoC; 30s budget per ADR-0018; thirteenth fuzzer overall (post-09's twelfth `FuzzFaultConfigParse`). |
| `internal/filter/http/perroute.go` | MODIFIED | (a) NEW method `ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)` per ADR-0110 — sibling to existing `Resolve`; reads directly from `p.scopes[routeIdx].route[filterName]` / `p.scopes[routeIdx].vhost[filterName]` / `p.rc[filterName]` without most-specific selection; cache NOT consulted (per planner-time decision 2). (b) `BuildPerRouteConfig` body extended to consult the registry's per-route-validators after the `parseMap` calls succeed: takes a new optional `*HTTPRegistry` parameter (or extracts from a new `BuildPerRouteConfigWithRegistry` variant — implementer settles at Task 4 — see planner-time decision 3); for each filter name in chainNames that has a registered validator, the validator runs against each parsed proto.Message at each tier (RC, VHost, Route), returning the first error. ~+90 LoC delta. |
| `internal/filter/http/perroute_test.go` | MODIFIED | NEW tests for `ResolveAllTiers` covering: (a) all-three-tiers-set returns the 3-tuple in correct positions; (b) two-of-three set (3 combinations: route+vhost / route+rc / vhost+rc); (c) one-tier-set (3 combinations); (d) no-tier-set returns 3-nil; (e) routeIdx out-of-range returns 3-nil; (f) filterName not present at any tier returns 3-nil; (g) `ResolveAllTiers` does NOT pollute or read from the existing `Resolve` cache (call `ResolveAllTiers` then `Resolve` and verify both return correct values independently); (h) nil-receiver returns 3-nil. PLUS new tests for `BuildPerRouteConfig` per-route-validator integration covering: (i) validator returns nil → BuildPerRouteConfig succeeds; (j) validator returns error on a per-route config → BuildPerRouteConfig returns the wrapped error with location prefix; (k) validator runs on all 3 tiers (RC, VHost, Route) — table-driven with the offending tier varied; (l) no validator registered → BuildPerRouteConfig succeeds for any per-route config (backwards-compatible with cors/fault). ~+150 LoC delta. |
| `internal/filter/http/callbacks.go` | MODIFIED | NEW method `RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message)` on `DecoderFilterCallbacks` interface per ADR-0110. The method returns the 3-tuple of unmerged per-tier configs `(route, vhost, rc)` for the calling filter at the matched route. Used by header_mutation's DecodeHeaders + EncodeHeaders (per planner-time decision 1: DECODER-ONLY callback used from BOTH decode AND encode bodies; mirrors cors precedent at `cors.go:163` which calls `dcb.RequestRouteConfig` from EncodeHeaders). NO symmetric `ResponseRouteConfigsAllTiers` on EncoderFilterCallbacks (rejected per planner-time decision 1: decoder-callback-from-encode-context already works in production via cors). ~+15 LoC delta (interface method + doc-comment). |
| `internal/filter/http/chain.go` | MODIFIED | `decoderCB` (the framework's concrete impl of `DecoderFilterCallbacks`) gains the `RequestRouteConfigsAllTiers` method body delegating to `chain.perRoute.ResolveAllTiers(chain.filters[d.idx].Name, chain.routeIdx)`. Mirrors the existing `RequestRouteConfig` method body (lines 426–435) verbatim modulo the 3-tuple return shape. Returns (nil, nil, nil) when `chain.perRoute == nil`. ~+15 LoC delta. |
| `internal/filter/http/chain_test.go` | MODIFIED | New test `TestDecoderCB_RequestRouteConfigsAllTiers` exercising the new callback through the chain's per-stream state machine: build a chain with a perRoute carrying route + vhost + rc tiers for a synthetic filter name; assert the callback returns the correct 3-tuple at the correct routeIdx; assert nil-perRoute returns 3-nil; assert filter-not-present returns 3-nil. ~+40 LoC delta. (If the codebase's chain tests live in `callbacks_test.go` instead of `chain_test.go`, the implementer at Task 3 step 1 grep-locates the natural test home and adapts.) |
| `internal/filter/http/registry.go` | MODIFIED | `*HTTPRegistry` gains a `perRouteValidators map[string]func(proto.Message) error` field + a `RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)` method (called by header_mutation's `New` factory) + a `PerRouteValidator(filterName string) func(proto.Message) error` accessor (consumed by `BuildPerRouteConfig`). The `RegisterPerRouteValidator` method panics if called after `Freeze()` (mirrors the existing `Register`/`Freeze` discipline per ADR-0072). ~+30 LoC delta. |
| `internal/filter/http/registry_test.go` | MODIFIED | New tests: `TestRegistry_RegisterPerRouteValidator_BeforeFreeze` (succeeds), `TestRegistry_RegisterPerRouteValidator_AfterFreezePanics`, `TestRegistry_PerRouteValidator_LookupAfterRegister`, `TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil`, `TestRegistry_PerRouteValidator_DoesNotConflictWithRegister` (independent maps; both register-paths usable for the same filterName). ~+50 LoC delta. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` registration inserted after the existing `httpReg.Register(fault.TypeURL, fault.New)` line (currently line 115 in master HEAD `f339c12`) and before `httpReg.Freeze()` (currently line 116). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/header_mutation"` alphabetically among the existing filter-package imports (currently lines 28-31: cors, envoygotest, fault, router → cors, envoygotest, fault, header_mutation, router). Per BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline (codified at phase-09 brainstorm time), the resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Freeze()`. **No other wiring changes** — header_mutation is HTTP-only, no listener/cluster/drain manager threading. ~+3 LoC delta (1 import line + 1 register line). |
| `test/fixtures/0012-http-header-mutation/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. **Note:** SPEC §4.3 references `test/differential/0012-http-header-mutation/`; planner-time decision 10 below corrects to `test/fixtures/0012-http-header-mutation/` per the existing 0010-graceful-drain + 0011-http-fault precedent locations. The 09 PLAN flagged the identical erratum for runner.go-vs-runner_test.go AND the directory-path; phase 10's PLAN flags the directory-path erratum analogously (the runner_test.go vs runner.go erratum carries forward — the actual fixture-registration site is `test/differential/runner_test.go`'s blank-import block per planner-time decision 10). |
| `test/fixtures/0012-http-header-mutation/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port 9912 in-container; TWO listeners `l_lws` on port 10012 + `l_mws` on port 10013 — for the two `most_specific_header_mutations_wins` flag values per SPEC §11.5 precedent; cluster `c_backend` STRICT_DNS pointing at the harness backend on port 18012 via `host.docker.internal` per ADR-0010). Per-route configs at all three tiers (RC, VirtualHost, Route) on the `/multi-tier` prefix; per-route override only on the `/route-override` prefix; listener-only mutations exercising all 4 AppendActions + Remove + keep_empty_value boundary on the `/listener-only` prefix. http_filters: `[envoy.filters.http.header_mutation, envoy.filters.http.router]`. Per SPEC §7.4 verbatim YAML expansion (the SPEC §7.4 example summarizes `l_mws.route_config` with `... (route_config body identical to rc_lws above) ...`; the fixture file's actual bytes contain the FULL expansion). |
| `test/fixtures/0012-http-header-mutation/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo admin/listener port values (admin :9911 → resolved at boot by the runner; listener :10011 + :10012 → resolved at boot). The shared `c_backend` cluster points at the harness backend port resolved at boot. Per SPEC §7.4. |
| `test/fixtures/0012-http-header-mutation/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1: scenario 1 (`/listener-only/anything` against `l_lws:10012`) → 200 + body byte-equal (echo backend reflects post-mutation request headers) + post-mutation response headers; scenario 2 (`/route-override/anything` against `l_lws:10012`) → 200 + layered-mutation result (listener applied first, then Route tier); scenario 3 (`/multi-tier/anything` against `l_lws:10012`) → 200 + final upstream `x-test: rc` (RouteConfiguration wins per default flag=false) + final response `x-resp-test: rc-resp`; scenario 4 (`/multi-tier/anything` against `l_mws:10013`) → 200 + final upstream `x-test: route` (Route wins per flag=true) + final response `x-resp-test: route-resp`. NO timing assertions (synchronous filter). NO stat assertions (zero new stats per §11.3). Status text byte-equal on stdlib codes (200 only — phase 10 doesn't exercise non-stdlib codes). Cross-refs SPEC §7.1 + §13.1 + ADR-0109 + ADR-0110 + ADR-0111. Per SPEC §4.3. |
| `test/fixtures/0012-http-header-mutation/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + four-scenario list (per §7.1) + dual-listener bootstrap discipline (port-disambiguated for dual-boot under `--network host` per the existing fixture pattern; TWO listeners per proxy for the flag-controlled cross-tier ordering test) + Envoy-deviation note (none — header_mutation is a normal HTTP filter; no SIGTERM/drain divergence) + planner-time-decision cross-references. Per SPEC §4.3. |
| `test/fixtures/0012-http-header-mutation/driver/driver.go` | NEW | Go driver implementing the §7.3 four-scenario orchestration. **Driver shape** (mirrors 0011-http-fault per planner-time decision 9): `package driver`; `init()` calls `fixture.RegisterFixture("0012-http-header-mutation", &headerMutationDriver{})`; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPHeaderMutation` (the new enum value added in Task 11); `SubjectListenerName() string` returns `"l_lws"` (the primary listener; `l_mws` accessed via fixed-port-offset); `SubjectListenerPort()` / `ReferenceListenerPort()` return the SPEC §7.4 port pair (10012 / 10013 / 10011 / 10012); `ReferenceBootstrap(backendPorts []int) string` templates `envoy.yaml` substituting `{{.BackendPort}}` with `host.docker.internal:` + `backendPorts[0]`; `SubjectConfig(...)` templates `envoy-go.yaml` with the runner-allocated subject ports + backend port; `DriveReference` / `DriveSubject` issue the four-scenario probe sequence (4 HTTP requests per proxy, sequenced) capturing per-probe status + body + headers; returns the captured per-probe assertion-log lines as a deterministic byte stream → CompareBytes between ref+subj passes when both emit the same log lines; `ProbeAdmin` issues `GET /ready` against both proxies + returns the bytes for the admin diff (per the existing 0010/0011 pattern); NO `AssertStats` (zero stats per §11.3 — the driver does NOT scrape stats endpoints or assert stat deltas; the runner's optional `StatsAsserter` interface is not implemented). **Synchronization:** event-based throughout (no hardcoded sleeps per the 08.2 SPEC §10 + 07.2 REVIEW M-8 carry-forward); the four scenarios run sequentially against each proxy. **Total per-proxy wall-clock:** <0.05s (synchronous filter; no delay, no abort; all 4 probes are sub-10ms). Per SPEC §7.3. |
| `test/fixtures/0012-http-header-mutation/backends/backend.go` | NEW | Minimal Go HTTP backend bound to a runner-allocated port. `/` endpoint serves a fast `200 OK` with body listing every received request header (one per line: `"Name: value\n"`, sorted for determinism since Go map iteration is non-deterministic); response carries one single-value header (`X-Resp-Test: backend-original`) and one multi-value header (`X-Multi: alpha`, `X-Multi: beta`) for OVERWRITE / APPEND multi-value testing per §11.4. Mirrors the §7.5 backend exactly. Accepts a `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper. ~50 LoC. Per SPEC §7.5. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPHeaderMutation BackendKind = 9` after the existing `HTTPFault BackendKind = 8` (introduced by phase 09). Doc-comment notes: "HTTPHeaderMutation is an out-of-process HTTP/1.1 backend: the runner spawns `test/fixtures/0012-http-header-mutation/backends/backend.go` on the pre-allocated port. The backend serves `/` reflecting received request headers into the response body (one header per line, sorted) plus a single-value `X-Resp-Test: backend-original` and multi-value `X-Multi: alpha, beta` response headers. No TLS. Introduced by fixture 0012-http-header-mutation (phase 10 Task 11) to provide the deterministic-body backend that surfaces post-mutation request headers via the response body. Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver"` (insert in alphabetical order, after the `0011-http-fault` blank-import). (b) Extend the `kind` switch in `runFixture` with a new case `fixture.HTTPHeaderMutation` mirroring the `HTTPFault` block: spawn via `startHTTPHeaderMutationBackend`. (c) Add new spawn helper `startHTTPHeaderMutationBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` mirroring `startHTTPFaultBackend`: `exec.CommandContext(ctx, "go", "run", "./test/fixtures/0012-http-header-mutation/backends", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.header_mutation` subsection inserted under existing `## HTTP filter chain` umbrella AFTER the `### envoy.filters.http.fault` subsection at line 862 landed by phase 09 (per §13.1; ~95 LoC); (b) `## Stat-name mapping ### 22-name table` UNCHANGED per §13.2 (header_mutation emits ZERO stats per §11.3); (c) `## Timing tolerances` UNCHANGED per §13.3 (synchronous filter; no time-bounded assertions); (d) `## Equivalence Matrix` new header_mutation-filter row (per §13.4; ~3 LoC); (e) two forward-pointer notes per §13.5: at `## HTTP filter chain ### typed_per_filter_config 3-tier merge` discussion (codifying ADR-0073's most-specific-override discipline) noting ADR-0110's multi-tier amendment; at `## HTTP filter chain ### envoy.filters.http.cors ### Asserted equivalence` noting that header_mutation is the SECOND production filter to mutate response headers in EncodeHeaders (~10 LoC total). ADR-0052 in-place edit authorisation carries forward. ~+108 LoC total. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append six new ADRs ADR-0108..ADR-0113 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). The 7-section ADR-0001 template applies to each (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0112 + ADR-0113 follow the ADR-0040 deferral-ADR format per §1.1 amendment. **Inline supersessions / amendments:** ADR-0073 (typed_per_filter_config 3-tier merge / most-specific override) — AMENDED (not superseded) by ADR-0110: the most-specific-override discipline remains the DEFAULT model (used by cors + fault); filters whose proto semantics demand multi-tier evaluation opt into `ResolveAllTiers` per ADR-0110. Cross-reference recorded in ADR-0110 §Decision; **inline edit of ADR-0073** marked with `## Amendment (per phase 10 ADR-0110)` paragraph noting "the most-specific-override discipline is now the DEFAULT model; filters that need multi-tier evaluation use `PerRouteConfig.ResolveAllTiers` per ADR-0110 — see that ADR for the per-filter accessor-choice discipline." (NO change to the original Decision body; the amendment is a forward-pointer.) ~+360 LoC total (six ADRs + ADR-0073 amendment paragraph). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `10` `in-progress → done` flip AT the phase-done commit. The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 10 artefacts (per ADR-0106 no-sibling-stub discipline). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 3 (PLAN drafting — this PLAN landing flips state 3 → 4 in the orchestrating session's STATE.md edit), 4 (PLAN execution — Tasks 1–15 land production code + fixture; STATE stays at 4), 5 (verification — Task 16 lands BEHAVIOR_CONTRACT/ADRs/six-gate verification; STATE flips 4 → 5), 6 (review — Task 17 + Task 18 REVIEW.md per requesting-code-review skill; STATE flips 5 → 6 then to `awaiting next planning`); `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner. |
| `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..09 PROGRESS.md structure. The preamble enumerates the six anticipated ADRs ADR-0108..ADR-0113 + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 11 items below). |
| `docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 cadence; populates per the requesting-code-review skill. Phase 10 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 10. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's eight deferred decisions before implementation; this PLAN settles all eight plus three that emerged at PLAN-drafting time (items 9, 10, 11 below). The eleven resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **`RequestRouteConfigsAllTiers` callback symmetry — Decoder vs. Decoder+Encoder = DECODER-ONLY.** Per SPEC §12 #1, the SPEC author's recommendation was "add to BOTH callback interfaces symmetrically." This PLAN OVERRIDES with **DECODER-ONLY**: the callback lives only on `DecoderFilterCallbacks`, and the header_mutation filter uses `f.dcb.RequestRouteConfigsAllTiers()` from BOTH `DecodeHeaders` AND `EncodeHeaders` bodies. Rationale: the cors precedent at `internal/filter/http/cors/cors.go:163 (routePolicy)` already calls `f.dcb.RequestRouteConfig()` from `EncodeHeaders` (lines 133–146 EncodeHeaders body invokes routePolicy which calls `f.dcb.RequestRouteConfig`); the `dcb` reference is set by the framework via `SetDecoderCallbacks` for any filter that implements StreamDecoderFilter regardless of which side fires; the per-stream `routeIdx` is stable across decode and encode (chain.go's `SetRequestCtx` sets it once at request start). Adding a symmetric `ResponseRouteConfigsAllTiers` to `EncoderFilterCallbacks` would create two parallel callback surfaces with identical semantics — pure code duplication. The SPEC default is rejected on parsimony grounds; the cors precedent is the existence proof that decoder-callback-from-encode-context is a load-bearing pattern. *Anchored: SPEC §12 #1; cors.go:163; chain.go:120–128 + 463–466.*

2. **Per-request per-tier cache = SKIP.** Per SPEC §12 #2 recommendation. The per-tier proto.Message lookup via `ResolveAllTiers` is sub-microsecond (3 map reads on already-parsed maps); the `compileForRequest`/`compileForResponse` projection is also sub-microsecond on small per-route configs (typical: <5 ops per tier). Adding a per-instance `*filter` cache field for the (route, vhost, rc) compiled triples + a sync.Once-style first-resolve guard is ~30 LoC of complexity against a sub-microsecond saving per request. The decode-side and encode-side both re-resolve fresh; if profiling under fixture 0012 shows measurable cost, planner-time decision 2 is revisited as an opt-in optimization in a follow-up phase. *Anchored: SPEC §12 #2.*

3. **Per-route protected-header validation lifecycle = EAGER (HCM-build-time validator hook).** Per SPEC §12 #3 recommendation + SPEC §11.1 (e) MAJOR-SURPRISE finding. The framework gains a per-route-validator hook on `*HTTPRegistry`: `RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)`. header_mutation's `New` factory calls `ctx.Registry.RegisterPerRouteValidator("envoy.filters.http.header_mutation", validatePerRouteHeaderMutation)` once at boot. `BuildPerRouteConfig` consults the registry's per-route-validators after the proto-unmarshal step succeeds: for each filter name in `chainNames` that has a registered validator, the validator runs against each parsed `proto.Message` at each tier (RC, VHost, Route), returning the first error wrapped with location prefix (e.g., `"hcm: route_config.virtual_hosts[2].routes[5]: typed_per_filter_config[\"envoy.filters.http.header_mutation\"]: header_mutation: %q is :-prefixed or host; may not be modified"`). **Mechanism choice:** the cleanest API surface is to extend `BuildPerRouteConfig`'s signature with an optional `*HTTPRegistry` parameter (or a helper variant `BuildPerRouteConfigWithRegistry(rcCfg, scopes, chainNames, reg *HTTPRegistry)`). Implementer at Task 4 settles which: (a) **widen the signature** of `BuildPerRouteConfig` to require the registry (smallest API change; one new required param; HCM caller already has the registry in scope at `internal/filter/hcm/config.go::parseHTTPFiltersChain`); (b) **add a new variant** `BuildPerRouteConfigWithRegistry` keeping the original signature for backwards compatibility. Recommendation: (a) widen the signature — there is exactly one production caller (`parseHTTPFiltersChain`) and ~3 test-only callers in `perroute_test.go` that pass `nil` for the registry; the test-only callers update mechanically. ~50 LoC framework delta total (~30 in registry.go + ~20 in perroute.go). Lazy-validation alternative (validate at first per-route resolution and panic) is REJECTED: surfaces errors at first request rather than at boot, which violates the ADR-0072 boot-time-fail-fast contract. *Anchored: SPEC §12 #3 + §11.1 (e); ADR-0072 boot-time-fail-fast contract.*

4. **`compiledMutationOp` slice element type — value vs. pointer = VALUE-TYPED.** Per SPEC §12 #4 recommendation. `compiledMutationOp` is small (~5 fields, ~40 bytes); value semantics improve cache locality during the apply-loop iteration (`for _, op := range ops { ... }`); pointer semantics only win if the slice is mutated post-construction, which it is not (read-only after `New` per ADR-0109). *Anchored: SPEC §12 #4.*

5. **Where to define the protected-header set constants = prefix-check on `:` + case-insensitive equality on `host`.** Per SPEC §12 #5 recommendation. Implementation: `func isProtectedHeader(name string) bool { if strings.HasPrefix(name, ":") { return true }; return strings.EqualFold(name, "host") }`. Rationale: the `:`-prefixed pseudo-header set is open-ended at the spec level (Envoy may add `:protocol` or `:upgrade` in future); a prefix check future-proofs against new pseudo-headers without requiring an envoy-go release to extend the protected list. The `host` case-insensitive equality matches §11.1 empirical finding (Envoy rejects `host`, `Host`, `HOST` symmetrically). The implementation is ~3 LoC; constants are not needed. *Anchored: SPEC §12 #5; SPEC §11.1 conclusion (b).*

6. **Fuzzer = SHIP.** Per SPEC §12 #6 recommendation. `FuzzHeaderMutationConfigParse` (~50 LoC; 30s budget per ADR-0018; thirteenth fuzzer overall — post-09's twelfth `FuzzFaultConfigParse`). Fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter; asserts `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the header_mutation filter's `New` factory is a parser. Lands in Task 10. *Anchored: SPEC §12 #6 + §14.3; ADR-0018; ADR-0072 (factory-validates-typed_config contract).*

7. **Race-detector test for the multi-tier evaluation = ADD `TestHeaderMutation_MultiTierConcurrentRequests` under `-race`.** Per SPEC §12 #7 recommendation. Fires DecodeHeaders concurrently with shared `*runtimeConfig` (multiple per-instance `*filter` instances spawned in parallel via `factory()` then `f.DecodeHeaders(headers, false)` in goroutines under `t.Parallel()` + `wg.Wait()`). The framework's per-instance discipline makes the race trivially safe by construction (the `*runtimeConfig` is read-only after `New`); the race detector run validates by construction. ~30 LoC. Lands in Task 8. *Anchored: SPEC §12 #7; SPEC §5.7.*

8. **Whether to expose `applyOps` as package-level helper for testing = KEEP unexported.** Per SPEC §12 #8 recommendation. Unit tests access `applyOps` indirectly via the public `New` + `DecodeHeaders` / `EncodeHeaders` surface, which is the canonical contract. Exposing `applyOps` as `ApplyOps` would tempt drive-by testing of internals + lock the helper signature into a public-API contract that future refactors must preserve. The `applyOps`-via-DecodeHeaders test surface (Tasks 6 + 7) covers all 4 AppendActions + Remove + keep_empty_value boundary + multi-value collapse/preserve mechanically. *Anchored: SPEC §12 #8.*

9. **Per-route-validator integration test fan-out = TABLE-DRIVEN per tier.** PLAN-emerging decision. The `TestBuildPerRouteConfig_PerRouteValidator_*` test suite (Task 4) covers: validator-returns-nil → success; validator-returns-error on RC tier → BuildPerRouteConfig returns error with `"route_config: typed_per_filter_config[<name>]: <validator-error>"` prefix; validator-returns-error on VHost tier → `"route_config.virtual_hosts[i]: typed_per_filter_config[<name>]: <error>"` prefix; validator-returns-error on Route tier → `"route_config.virtual_hosts[i].routes[j]: typed_per_filter_config[<name>]: <error>"` prefix; no-validator-registered → success regardless of per-route config (backwards-compatible with cors/fault). Mirrors the existing `parseMap` location-prefix discipline at `perroute.go:69–74`. ~30 LoC test code per tier × 3 tiers = ~90 LoC; consolidated into one table-driven test for compactness. Lands in Task 4. *Anchored: existing parseMap location-prefix pattern at perroute.go:69–74.*

10. **Fixture path = `test/fixtures/0012-http-header-mutation/` (NOT `test/differential/0012-http-header-mutation/`).** SPEC §4.3 + §7 reference `test/differential/0012-http-header-mutation/` as the fixture root; this is a SPEC erratum mirroring 09 PLAN's identical erratum reconciliation. The actual location convention per the existing 0010-graceful-drain + 0011-http-fault precedent (verified at master `f339c12`) is `test/fixtures/0012-http-header-mutation/`. The driver lives at `test/fixtures/0012-http-header-mutation/driver/`; the fixture-registration site is `test/differential/runner_test.go`'s blank-import block. The implementer at Task 12 + Task 13 + Task 14 + Task 15 uses the corrected path. *Anchored: 0010-graceful-drain + 0011-http-fault precedents at master `f339c12`; SPEC §4.3 + §7 erratum.*

11. **Fixture's new BackendKind enum value name = `HTTPHeaderMutation BackendKind = 9`.** PLAN-emerging decision. Continues the existing naming convention (`HTTPHello`, `HTTPSlowStream`, `HTTPFault` per phase 09); the suffix names the fixture-purpose, not the protocol family. The implementer at Task 11 adds the enum constant + doc-comment block matching the existing `HTTPFault BackendKind = 8` shape. *Anchored: existing fixture.BackendKind enumeration convention.*

These eleven decisions are reproduced verbatim in `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The six ADRs anticipated by SPEC §8 (ADR-0108..ADR-0113). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The six ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 / 08.2 / 09 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0108 | `internal/filter/http/header_mutation/` package shape (TypeURL + New + filter struct + decoder/encoder methods) + extension-registry registration line + boot-time `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` | Task 5 (`internal/filter/http/header_mutation/{doc.go,header_mutation.go,header_mutation_test.go}` first lands; the boot registration code lands in Task 9 but ADR-0108 anchors at Task 5 because that's the first-use site that justifies the package shape per ADR-0044). |
| ADR-0109 | `runtimeConfig` shape + 3-field-consumed / 1-field-silent-ignore decomposition + `compiledMutationOp` flat-struct + AppendAction × 4 mapping table + `keep_empty_value` semantics + multi-value collapse / preserve per §11.4 + the unmarshal-at-New discipline | Task 5 (`runtimeConfig` + `compiledMutationOp` + `compileOps` + New-time validation first lands). |
| ADR-0110 | Multi-tier per-route evaluation: framework extension `PerRouteConfig.ResolveAllTiers` + `DecoderFilterCallbacks.RequestRouteConfigsAllTiers` callback (DECODER-ONLY per planner-time decision 1) + `HTTPRegistry.RegisterPerRouteValidator` per-route-validator hook (per planner-time decision 3) + per-filter accessor-choice discipline + `most_specific_header_mutations_wins` cross-tier algorithm; **AMENDS (does not supersede) ADR-0073** with the multi-tier-vs-most-specific accessor-choice doctrine | Task 7 (DecodeHeaders body + multi-tier resolution + flag-controlled ordering first lands the algorithm; the framework pieces land in Tasks 2/3/4 but ADR-0110 anchors at Task 7 because that's the first-use-of-the-full-multi-tier-stack commit that justifies the framework deltas per ADR-0044). |
| ADR-0111 | Protected-header set per §11.1 (`{:method, :path, :authority, :scheme, :status, host}` case-insensitive on `host`; prefix-check on `:` future-proofs new pseudo-headers per planner-time decision 5) + **CONFIG-LOAD-TIME** rejection (NOT runtime silent-no-op as BRAINSTORM Decision 11 hypothesized — MAJOR amendment per §11.1 (e)) + verbatim error message format `"header_mutation: %q is :-prefixed or host; may not be modified"` mirroring Envoy v1.37.2's `:-prefixed or host headers may not be modified` (per `source/server/server.cc:453`) + EAGER per-route validation lifecycle (per planner-time decision 3) | Task 5 (the listener-level protected-header validation in `New` first lands; the per-route validator's HCM-build-time integration lands in Task 7 alongside the per-route-validator registration call from `New`). |
| ADR-0112 | `mutations.query_parameter_mutations[]` deferred — coupled to `KeyValueMutation` triple + path/query rewriting subsystem (deferral ADR per ADR-0040 format) | Task 16 (BEHAVIOR_CONTRACT umbrella — the §13.1 `### envoy.filters.http.header_mutation` subsection's `#### Does not yet apply to` paragraph IS the deferral table that ADR-0112 codifies). |
| ADR-0113 | Header-value formatter substitution (`%REQ(:path)%` etc) deferred — full Envoy command-string subsystem is its own multi-phase project (deferral ADR per ADR-0040 format) | Task 16 (BEHAVIOR_CONTRACT umbrella — same §13.1 deferral paragraph hosts the ADR-0113 forward-pointer). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Inline supersessions / amendments anticipated** (recorded inline in the listed ADRs above per the ADR-0089 consequence (b) in-place-edit pattern; NOT separate ADRs):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — AMENDED (not superseded) by ADR-0110: the most-specific-override discipline remains the DEFAULT model (used by cors + fault); filters whose proto semantics demand multi-tier evaluation (header_mutation per its `most_specific_header_mutations_wins` flag) opt into `PerRouteConfig.ResolveAllTiers` per ADR-0110. Inline edit of ADR-0073: append a `## Amendment (per phase 10 ADR-0110)` paragraph noting "the most-specific-override discipline is now the DEFAULT model; filters that need multi-tier evaluation use `PerRouteConfig.ResolveAllTiers` per ADR-0110 — see that ADR for the per-filter accessor-choice discipline." NO change to the original Decision body; the amendment is a forward-pointer. Lands in Task 7 alongside ADR-0110.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — extended cross-reference recorded in ADR-0110 §Consequences. The new `RegisterPerRouteValidator` method follows the same Register/Freeze discipline (panic-after-Freeze) and is purely additive. NO in-place edit of ADR-0072.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0108 §Consequences. The filter set extends from {cors, envoy_go_test, router, fault} to {cors, envoy_go_test, router, fault, header_mutation}. NO in-place edit of ADR-0074.
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 10. header_mutation's `New` factory does NOT consume `ctx.Stats` or `ctx.StatPrefix` (zero stats per §11.3); the 3-field FactoryCtx stays as-is. ADR-0108 §Consequences notes the no-consumption pattern (analogous to cors which also doesn't consume Stats per ADR-0074 — the phase-10 first-use discipline confirms that the FactoryCtx fields are opt-in per filter, not mandatory).
- **ADR-0101** (runtimeConfig shape + parser pattern) — extended cross-reference recorded in ADR-0109 §Consequences. The header_mutation runtimeConfig mirrors fault's structurally (3 fields vs fault's 8 — both follow the closure-capture pattern + parse-at-New + read-only-shared-after-New discipline). NO in-place edit of ADR-0101.

These five cross-references land at the task that anchors each affected ADR (ADR-0108 at Task 5; ADR-0109 at Task 5; ADR-0110 at Task 7; ADR-0111 at Task 5; ADR-0073 amendment at Task 7). No in-place edit of any pre-existing ADR is required EXCEPT the ADR-0073 amendment paragraph in Task 7.

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-10-http-filter-header-mutation-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003 + the per-phase-worktree convention: `git worktree add .worktrees/phase-10-http-filter-header-mutation-impl -b phase-10-http-filter-header-mutation-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -8` shows the PLAN.md commit (this plan) and (optionally) its SHA-fill follow-up at the head, with the SPEC.md commit `f339c12` and its SHA-fill follow-up `c6ee201` immediately before, then the BRAINSTORM.md commit `ad7c129` and its SHA-fill `71b6401`, then 09's REVIEW at `3066c72`. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `107`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0108..ADR-0113 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` returns `f339c12` (the SPEC commit). If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011'` returns every fixture PASS. The 13 pre-existing fixtures (0000–0011) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 12 fuzzers from phases 02–09 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each). Phase 10 adds the thirteenth (`FuzzHeaderMutationConfigParse` per Task 10).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **`envoy.extensions.filters.http.header_mutation.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3 HeaderMutation` returns the `HeaderMutation` proto type's exported fields without an `import path failed` error; `go doc github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3 HeaderMutation` returns the `HeaderMutation` primitive's exported fields. If either `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `f339c12` so a tidy should not be needed).
12. **Pre-existing `internal/filter/http/header_mutation/` directory does NOT exist.** `test ! -d internal/filter/http/header_mutation` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
13. **Pre-existing `PerRouteConfig` does NOT have a `ResolveAllTiers` method.** `grep -nE '^func.*PerRouteConfig.*ResolveAllTiers' internal/filter/http/perroute.go` returns 0 matches. If 1+, the framework has already been extended by a concurrent phase — investigate.
14. **Pre-existing `DecoderFilterCallbacks` does NOT have a `RequestRouteConfigsAllTiers` method.** `grep -nE 'RequestRouteConfigsAllTiers' internal/filter/http/callbacks.go` returns 0 matches. If 1+, investigate.
15. **Pre-existing `HTTPRegistry` does NOT have a `RegisterPerRouteValidator` method.** `grep -nE 'RegisterPerRouteValidator' internal/filter/http/registry.go` returns 0 matches. If 1+, investigate.
16. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).

If all 16 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the six ADRs ADR-0108..ADR-0113 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the six ADRs and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-10-http-filter-header-mutation-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 16 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` (new file).
**Acceptance:** all 16 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-10-http-filter-header-mutation-impl
git log --oneline master | head -8                                    # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (c6ee201), SPEC (f339c12), BRAINSTORM SHA-fill (71b6401), BRAINSTORM (ad7c129), 09 REVIEW (3066c72)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 107
git log -1 --format=%H -- docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md
                                                                       # expect: f339c12... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/header_mutation && echo "ok: header_mutation absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3 HeaderMutation | head -5
                                                                       # expect: type HeaderMutation struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3 HeaderMutation | head -5
                                                                       # expect: type HeaderMutation struct { ... }
grep -cE 'ResolveAllTiers' internal/filter/http/perroute.go           # expect: 0 (method does not exist yet)
grep -cE 'RequestRouteConfigsAllTiers' internal/filter/http/callbacks.go  # expect: 0
grep -cE 'RegisterPerRouteValidator' internal/filter/http/registry.go # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md`**

```markdown
# Phase 10 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..09 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 16 preconditions were satisfied at cold-start>

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The six ADRs anticipated by SPEC §8 (ADR-0108..ADR-0113). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0108** `internal/filter/http/header_mutation/` package shape + boot registration — Task 5 (ADR text + impl), Task 9 (boot registration code)
- **ADR-0109** runtimeConfig shape + 3/1-field decomposition + AppendAction × 4 mapping + keep_empty_value semantics + multi-value collapse/preserve — Task 5
- **ADR-0110** Multi-tier per-route evaluation + ResolveAllTiers + RequestRouteConfigsAllTiers + RegisterPerRouteValidator + accessor-choice discipline + cross-tier algorithm + amends ADR-0073 — Task 7 (ADR text + ADR-0073 amendment paragraph), Tasks 2/3/4 (framework piece commits)
- **ADR-0111** Protected-header set + CONFIG-LOAD-TIME rejection + verbatim error format + EAGER per-route validation lifecycle — Task 5
- **ADR-0112** mutations.query_parameter_mutations[] DEFERRED (per ADR-0040 deferral format) — Task 16
- **ADR-0113** Header-value formatter substitution DEFERRED (per ADR-0040 deferral format) — Task 16

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The eleven planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`RequestRouteConfigsAllTiers` callback symmetry = DECODER-ONLY** (mirrors cors precedent at cors.go:163; PLAN OVERRIDES SPEC default of "BOTH symmetric"; rationale: cors already calls dcb.RequestRouteConfig from EncodeHeaders so the pattern is in production).
2. **Per-request per-tier cache = SKIP** (sub-microsecond lookup + recompile; revisit only if profiling shows cost).
3. **Per-route protected-header validation lifecycle = EAGER** (HCM-build-time validator hook on *HTTPRegistry; BuildPerRouteConfig signature widens to take registry; ~50 LoC framework delta).
4. **compiledMutationOp slice element type = VALUE-TYPED** (cache locality during apply-loop; read-only after New).
5. **Protected-header set definition = prefix-check on `:` + case-insensitive equality on `host`** (`func isProtectedHeader(name string) bool { if strings.HasPrefix(name, ":") { return true }; return strings.EqualFold(name, "host") }`).
6. **Fuzzer = SHIP** (`FuzzHeaderMutationConfigParse`; ~50 LoC; 30s budget; thirteenth fuzzer; lands in Task 10).
7. **Race-detector cycle test = ADD `TestHeaderMutation_MultiTierConcurrentRequests`** (~30 LoC; lands in Task 8).
8. **applyOps exposure = KEEP unexported** (test via public DecodeHeaders/EncodeHeaders surface).
9. **Per-route-validator integration test fan-out = TABLE-DRIVEN per tier** (RC/VHost/Route × validator-error/no-validator/no-config; ~90 LoC; lands in Task 4).
10. **Fixture path = `test/fixtures/0012-http-header-mutation/`** (NOT `test/differential/0012-http-header-mutation/` per SPEC §4.3 erratum; mirrors 0010/0011 precedents).
11. **Fixture's new BackendKind enum value = `HTTPHeaderMutation BackendKind = 9`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-10 SPEC + 10 PLAN confirmed present in HEAD; SPEC at f339c12; ADR tail at 0107 (next-free 0108); internal/filter/http/header_mutation/ absent (Task 5 lands); ResolveAllTiers absent (Task 2 lands); RequestRouteConfigsAllTiers absent (Task 3 lands); RegisterPerRouteValidator absent (Task 4 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md
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
git add docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md
git commit -m "phase 10: PROGRESS preamble + planner-time decision resolution"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR anticipation table), §12 (deferred decisions), §15 (acceptance criteria) and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `PerRouteConfig.ResolveAllTiers` framework method + tests

**Files:**
- Modify: `internal/filter/http/perroute.go` (add `ResolveAllTiers` method)
- Modify: `internal/filter/http/perroute_test.go` (add `ResolveAllTiers` tests)

This task lands the first of three framework-extension pieces for ADR-0110: the multi-tier per-route accessor `PerRouteConfig.ResolveAllTiers(filterName, routeIdx) (route, vhost, rc proto.Message)`. The method reads directly from the existing `p.scopes[routeIdx].route` / `p.scopes[routeIdx].vhost` / `p.rc` maps without the most-specific selection logic in the existing `Resolve` method, returning the unmerged 3-tuple with nil entries for tiers without configs. The cache (`p.cache`) is NOT consulted (per planner-time decision 2). The ADR-0110 text itself does NOT land in this task (it lands in Task 7 alongside the first end-to-end use of the multi-tier stack); this task lands only the framework piece + tests.

**Precondition:** Task 1 done; `grep ResolveAllTiers internal/filter/http/perroute.go` returns 0.
**Artifact:** modified `perroute.go` + extended `perroute_test.go`.
**Acceptance:** `go build ./internal/filter/http/...` clean; `go test -race ./internal/filter/http/... -run TestResolveAllTiers` passes; `go vet ./...` clean.

- [ ] **Step 1: Write failing tests in `internal/filter/http/perroute_test.go`** (extend, do not replace)

Append after the existing `TestPerRoute_LazyCacheHitMiss` function:

```go
func TestResolveAllTiers_AllThreeSet(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc-level"))}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh-level"))}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route-level"))}
    pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
    if err != nil {
        t.Fatalf("BuildPerRouteConfig: %v", err)
    }
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if rs, ok := route.(*wrapperspb.StringValue); !ok || rs.GetValue() != "route-level" {
        t.Errorf("route: got %v, want route-level", route)
    }
    if vs, ok := vhost.(*wrapperspb.StringValue); !ok || vs.GetValue() != "vh-level" {
        t.Errorf("vhost: got %v, want vh-level", vhost)
    }
    if rcs, ok := rc.(*wrapperspb.StringValue); !ok || rcs.GetValue() != "rc-level" {
        t.Errorf("rc: got %v, want rc-level", rc)
    }
}

func TestResolveAllTiers_RouteAndVHostOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
    pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route == nil || vhost == nil {
        t.Errorf("route+vhost should be non-nil; got route=%v vhost=%v", route, vhost)
    }
    if rc != nil {
        t.Errorf("rc should be nil; got %v", rc)
    }
}

func TestResolveAllTiers_RouteAndRCOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route == nil || rc == nil {
        t.Errorf("route+rc should be non-nil; got route=%v rc=%v", route, rc)
    }
    if vhost != nil {
        t.Errorf("vhost should be nil; got %v", vhost)
    }
}

func TestResolveAllTiers_VHostAndRCOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if vhost == nil || rc == nil {
        t.Errorf("vhost+rc should be non-nil; got vhost=%v rc=%v", vhost, rc)
    }
    if route != nil {
        t.Errorf("route should be nil; got %v", route)
    }
}

func TestResolveAllTiers_RouteOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
    pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route == nil {
        t.Errorf("route should be non-nil")
    }
    if vhost != nil || rc != nil {
        t.Errorf("vhost + rc should be nil; got vhost=%v rc=%v", vhost, rc)
    }
}

func TestResolveAllTiers_VHostOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
    pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if vhost == nil {
        t.Errorf("vhost should be non-nil")
    }
    if route != nil || rc != nil {
        t.Errorf("route + rc should be nil; got route=%v rc=%v", route, rc)
    }
}

func TestResolveAllTiers_RCOnly(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if rc == nil {
        t.Errorf("rc should be non-nil")
    }
    if route != nil || vhost != nil {
        t.Errorf("route + vhost should be nil; got route=%v vhost=%v", route, vhost)
    }
}

func TestResolveAllTiers_NoneSet(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    pr, _ := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: nil}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route != nil || vhost != nil || rc != nil {
        t.Errorf("all should be nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
    }
}

func TestResolveAllTiers_RouteIdxOutOfRange(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 99)
    if route != nil || vhost != nil {
        t.Errorf("route+vhost should be nil for out-of-range routeIdx; got route=%v vhost=%v", route, vhost)
    }
    // RC is still consulted (not per-scope).
    if rc == nil {
        t.Errorf("rc should be non-nil even with out-of-range routeIdx")
    }
}

func TestResolveAllTiers_FilterNameNotPresent(t *testing.T) {
    chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("cors-rc"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames)
    // Look up a filter name not present at any tier.
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route != nil || vhost != nil || rc != nil {
        t.Errorf("all should be nil for absent filterName; got route=%v vhost=%v rc=%v", route, vhost, rc)
    }
}

func TestResolveAllTiers_DoesNotPolluteResolveCache(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
    pr, _ := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames)
    // Call ResolveAllTiers first.
    route, _, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route == nil || rc == nil {
        t.Fatalf("setup: route+rc should be non-nil")
    }
    // Then call Resolve and verify it returns route-level (most-specific override per ADR-0073).
    msg := pr.Resolve("envoy.filters.http.header_mutation", 0)
    if rs, ok := msg.(*wrapperspb.StringValue); !ok || rs.GetValue() != "route" {
        t.Errorf("Resolve after ResolveAllTiers should return route-level; got %v", msg)
    }
}

func TestResolveAllTiers_NilReceiver(t *testing.T) {
    var pr *PerRouteConfig
    route, vhost, rc := pr.ResolveAllTiers("envoy.filters.http.header_mutation", 0)
    if route != nil || vhost != nil || rc != nil {
        t.Errorf("nil receiver should return all-nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
    }
}
```

- [ ] **Step 2: Run tests; confirm they fail (compile error — `ResolveAllTiers` does not exist)**

```bash
go test ./internal/filter/http/... -run TestResolveAllTiers 2>&1 | head -10
```

Expected: `pr.ResolveAllTiers undefined (type *PerRouteConfig has no field or method ResolveAllTiers)`.

- [ ] **Step 3: Implement `ResolveAllTiers` in `internal/filter/http/perroute.go`**

Append after the existing `Resolve` method (line 128):

```go
// ResolveAllTiers returns the parsed per-route config at each tier, unmerged.
// Tiers are returned in canonical proto order: Route (most specific),
// VirtualHost (intermediate), RouteConfiguration (least specific). A tier
// with no config for filterName at the matched route is nil.
//
// Used by filters whose semantics require multi-tier evaluation rather than
// most-specific override (e.g., envoy.filters.http.header_mutation per its
// most_specific_header_mutations_wins flag — see ADR-0110). The default
// Resolve method (per ADR-0073) remains the canonical accessor for filters
// that use most-specific override (cors, fault).
//
// NOTE: ResolveAllTiers does NOT consult or pollute the existing p.cache
// (which is keyed by (filterName, routeIdx) returning a single proto.Message;
// multi-tier returns 3 messages with different cache shape). The map reads
// (p.scopes[routeIdx].route, p.scopes[routeIdx].vhost, p.rc) are sub-microsecond;
// per-request re-reads are acceptable per phase 10 PLAN planner-time decision 2.
func (p *PerRouteConfig) ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message) {
    if p == nil {
        return nil, nil, nil
    }
    if routeIdx >= 0 && routeIdx < len(p.scopes) {
        if m, ok := p.scopes[routeIdx].route[filterName]; ok {
            route = m
        }
        if m, ok := p.scopes[routeIdx].vhost[filterName]; ok {
            vhost = m
        }
    }
    if m, ok := p.rc[filterName]; ok {
        rc = m
    }
    return route, vhost, rc
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test -race ./internal/filter/http/... -run TestResolveAllTiers -v 2>&1 | tail -20
```

Expected: every TestResolveAllTiers_* test PASS.

- [ ] **Step 5: Run vet + lint**

```bash
go vet ./...                                                  # expect: clean
golangci-lint run ./internal/filter/http/...                  # expect: clean
```

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/perroute.go internal/filter/http/perroute_test.go
git commit -m "phase 10: framework — PerRouteConfig.ResolveAllTiers per ADR-0110"
```

SHA-fill follow-up.

*Anchored: SPEC §6.7; ADR-0110 framework piece (1 of 3); planner-time decision 2 (no cache).*

---

## Task 3: `DecoderFilterCallbacks.RequestRouteConfigsAllTiers` callback + chain wiring + tests

**Files:**
- Modify: `internal/filter/http/callbacks.go` (add interface method)
- Modify: `internal/filter/http/chain.go` (add `decoderCB.RequestRouteConfigsAllTiers` impl)
- Modify: `internal/filter/http/chain_test.go` (or appropriate test file; add callback test)

This task lands the second framework-extension piece for ADR-0110: the new callback method `DecoderFilterCallbacks.RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message)` returning the 3-tuple of unmerged per-tier configs for the calling filter at the matched route. The callback is DECODER-ONLY per planner-time decision 1 (header_mutation calls `f.dcb.RequestRouteConfigsAllTiers()` from BOTH DecodeHeaders AND EncodeHeaders bodies; mirrors cors precedent). The chain's `decoderCB` (concrete impl at `chain.go:410–435`) gains the method body delegating to `chain.perRoute.ResolveAllTiers(chain.filters[d.idx].Name, chain.routeIdx)`. ADR-0110 text does NOT land in this task (Task 7 anchors).

**Precondition:** Task 2 done.
**Artifact:** modified callbacks.go + chain.go + test file.
**Acceptance:** `go build ./internal/filter/http/...` clean; new callback test PASSES; existing callback tests still pass; `go vet ./...` clean.

- [ ] **Step 1: Locate the natural test home**

```bash
grep -l 'RequestRouteConfig' internal/filter/http/*_test.go
```

If `chain_test.go` exists and contains existing `RequestRouteConfig` tests, use it; otherwise use `callbacks_test.go` (or create `chain_callbacks_test.go` if neither exists). Implementer's call.

- [ ] **Step 2: Write failing test**

In the located test file, add:

```go
func TestDecoderCB_RequestRouteConfigsAllTiers(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
    pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames)
    if err != nil {
        t.Fatalf("BuildPerRouteConfig: %v", err)
    }
    // Build a chain with a single filter named header_mutation.
    fakeFilter := &fakeBothSidesFilter{}
    chain := NewFilterChain([]HTTPFilter{{Name: "envoy.filters.http.header_mutation", Decoder: fakeFilter, Encoder: fakeFilter}}, pr)
    chain.SetRequestCtx(context.Background(), 0)

    // Reach into the chain to grab the decoderCB the framework wired into the filter.
    cb := fakeFilter.dcb
    if cb == nil {
        t.Fatal("fakeFilter.dcb not set")
    }
    route, vhost, rc := cb.RequestRouteConfigsAllTiers()
    if r, ok := route.(*wrapperspb.StringValue); !ok || r.GetValue() != "route" {
        t.Errorf("route: got %v, want route", route)
    }
    if v, ok := vhost.(*wrapperspb.StringValue); !ok || v.GetValue() != "vh" {
        t.Errorf("vhost: got %v, want vh", vhost)
    }
    if c, ok := rc.(*wrapperspb.StringValue); !ok || c.GetValue() != "rc" {
        t.Errorf("rc: got %v, want rc", rc)
    }
}

func TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute(t *testing.T) {
    fakeFilter := &fakeBothSidesFilter{}
    chain := NewFilterChain([]HTTPFilter{{Name: "envoy.filters.http.header_mutation", Decoder: fakeFilter, Encoder: fakeFilter}}, nil)
    chain.SetRequestCtx(context.Background(), 0)
    cb := fakeFilter.dcb
    route, vhost, rc := cb.RequestRouteConfigsAllTiers()
    if route != nil || vhost != nil || rc != nil {
        t.Errorf("nil-perRoute should return all-nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
    }
}

// fakeBothSidesFilter is a test helper implementing both StreamDecoderFilter and StreamEncoderFilter
// to capture the dcb wiring. (If a similar helper exists in the test package, reuse it.)
type fakeBothSidesFilter struct {
    dcb DecoderFilterCallbacks
    ecb EncoderFilterCallbacks
}

func (f *fakeBothSidesFilter) SetDecoderCallbacks(cb DecoderFilterCallbacks) { f.dcb = cb }
func (f *fakeBothSidesFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks) { f.ecb = cb }
func (f *fakeBothSidesFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (f *fakeBothSidesFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (f *fakeBothSidesFilter) DecodeData([]byte, bool) FilterDataStatus              { return DataContinue }
func (f *fakeBothSidesFilter) EncodeData([]byte, bool) FilterDataStatus              { return DataContinue }
func (f *fakeBothSidesFilter) DecodeTrailers(http.Header) FilterTrailersStatus       { return TrailersContinue }
func (f *fakeBothSidesFilter) EncodeTrailers(http.Header) FilterTrailersStatus       { return TrailersContinue }
func (f *fakeBothSidesFilter) OnDestroy()                                            {}
```

- [ ] **Step 3: Run test; confirm compile error (method does not exist)**

```bash
go test ./internal/filter/http/... -run TestDecoderCB_RequestRouteConfigsAllTiers 2>&1 | head -10
```

Expected: `cb.RequestRouteConfigsAllTiers undefined`.

- [ ] **Step 4: Add method to `DecoderFilterCallbacks` interface in `internal/filter/http/callbacks.go`**

Insert after the existing `RequestRouteConfig() proto.Message` method (line 36):

```go
    // RequestRouteConfigsAllTiers returns the parsed per-route config at each
    // of the three tiers (Route, VirtualHost, RouteConfiguration), UNMERGED.
    // Used by filters whose semantics require multi-tier evaluation rather
    // than most-specific override — primarily envoy.filters.http.header_mutation
    // per its most_specific_header_mutations_wins flag (see ADR-0110 amending
    // ADR-0073). The default RequestRouteConfig method (per ADR-0073) remains
    // the canonical accessor for filters that use most-specific override
    // (cors, fault).
    //
    // Per phase 10 PLAN planner-time decision 1: this callback lives ONLY on
    // DecoderFilterCallbacks (NOT on EncoderFilterCallbacks). Filters that
    // need it on the encode side use the dcb reference set via
    // SetDecoderCallbacks (the framework wires both dcb and ecb on a both-
    // sides filter). The cors precedent at cors.go:163 (routePolicy) calls
    // f.dcb.RequestRouteConfig() from EncodeHeaders — same pattern applies.
    //
    // Returns (nil, nil, nil) when:
    //   - the chain has no perRoute config;
    //   - no scope at any tier carries an entry for the calling filter's name.
    RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)
```

- [ ] **Step 5: Add method body to `decoderCB` in `internal/filter/http/chain.go`**

Insert after the existing `decoderCB.RequestRouteConfig` method (line 435):

```go
// RequestRouteConfigsAllTiers returns the unmerged per-tier configs for the
// calling filter's name via the chain's perRoute lookup at the route-index
// supplied by HCM dispatch (chain.routeIdx, set by SetRequestCtx). Returns
// (nil, nil, nil) if the chain has no perRoute config or no scope carries
// an entry for this filter at any tier.
func (d *decoderCB) RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message) {
    if d.c.perRoute == nil {
        return nil, nil, nil
    }
    return d.c.perRoute.ResolveAllTiers(d.c.filters[d.idx].Name, d.c.routeIdx)
}
```

- [ ] **Step 6: Run tests; confirm pass**

```bash
go test -race ./internal/filter/http/... -v 2>&1 | tail -30
```

Expected: TestDecoderCB_RequestRouteConfigsAllTiers + TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute PASS; existing callback tests still PASS.

- [ ] **Step 7: Verify that any pre-existing mock implementations of `DecoderFilterCallbacks` either implement the new method OR are updated**

```bash
grep -rE 'DecoderFilterCallbacks' --include='*.go' internal/ | grep -v -E 'callbacks.go|chain.go' | head -20
```

If any test-only mock implements the interface, extend it with a stub `RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) { return nil, nil, nil }`. Mechanical sweep.

- [ ] **Step 8: Run full unit-test suite**

```bash
go test -race -count=1 ./...                                  # expect: all PASS
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
```

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/callbacks.go internal/filter/http/chain.go internal/filter/http/chain_test.go
git commit -m "phase 10: framework — DecoderFilterCallbacks.RequestRouteConfigsAllTiers per ADR-0110"
```

SHA-fill follow-up.

*Anchored: SPEC §6.7; ADR-0110 framework piece (2 of 3); planner-time decision 1 (decoder-only).*

---

## Task 4: `HTTPRegistry.RegisterPerRouteValidator` + `BuildPerRouteConfig` validator integration + tests [planner-time decision 3]

**Files:**
- Modify: `internal/filter/http/registry.go` (add `perRouteValidators` map + `RegisterPerRouteValidator` + `PerRouteValidator` accessor)
- Modify: `internal/filter/http/perroute.go` (extend `BuildPerRouteConfig` to consult registry; widen signature)
- Modify: `internal/filter/http/registry_test.go` (registry method tests)
- Modify: `internal/filter/http/perroute_test.go` (`BuildPerRouteConfig` validator integration tests)
- Modify: `internal/filter/hcm/config.go` (thread the registry into the `BuildPerRouteConfig` call site if widened-signature variant chosen)
- Modify: any test files calling `BuildPerRouteConfig` directly with the old signature (mechanical sweep — pass `nil` for the new parameter)

This task lands the third framework-extension piece for ADR-0110: the per-route-validator hook on `*HTTPRegistry` consumed by `BuildPerRouteConfig` to surface per-route protected-header violations as boot-time errors per planner-time decision 3 + ADR-0111. The hook is purely additive (cors + fault don't register validators, so their per-route configs continue to pass `BuildPerRouteConfig` unchanged). ADR-0110 text does NOT land in this task (Task 7 anchors).

**Mechanism settled per planner-time decision 3:** widen `BuildPerRouteConfig`'s signature to `BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string, reg *HTTPRegistry) (*PerRouteConfig, error)`. The new `reg` parameter MAY be nil (for test-only callers that don't exercise validators); when non-nil, `BuildPerRouteConfig` consults `reg.PerRouteValidator(filterName)` for each filter name in `chainNames` and runs the validator on each parsed proto.Message at each tier.

**Precondition:** Task 3 done.
**Artifact:** modified registry.go + perroute.go + tests + HCM call-site widening + sweep of test-only `BuildPerRouteConfig` callers.
**Acceptance:** `go build ./...` clean; `go test -race ./internal/filter/http/...` PASS including new tests; `go vet ./...` clean; `go test -race ./...` clean across the full unit test suite.

- [ ] **Step 1: Locate `BuildPerRouteConfig` callers**

```bash
grep -rnE 'BuildPerRouteConfig\(' --include='*.go' internal/ test/ cmd/ 2>&1 | head -20
```

Catalog the callers; expect 1 production caller (`internal/filter/hcm/config.go::parseHTTPFiltersChain`) + ~3-5 test-only callers in `perroute_test.go` and possibly in cors/fault test files.

- [ ] **Step 2: Write failing tests in `internal/filter/http/registry_test.go`** (extend, do not replace)

```go
func TestRegistry_RegisterPerRouteValidator_BeforeFreezeSucceeds(t *testing.T) {
    r := NewHTTPRegistry()
    called := 0
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
        called++
        return nil
    })
    r.Freeze()
    v := r.PerRouteValidator("envoy.filters.http.header_mutation")
    if v == nil {
        t.Fatal("validator not retrievable")
    }
    if err := v(nil); err != nil {
        t.Errorf("validator should succeed; got %v", err)
    }
    if called != 1 {
        t.Errorf("validator should have been called once; got %d", called)
    }
}

func TestRegistry_RegisterPerRouteValidator_AfterFreezePanics(t *testing.T) {
    r := NewHTTPRegistry()
    r.Freeze()
    defer func() {
        if r := recover(); r == nil {
            t.Fatal("expected panic on register-after-freeze")
        }
    }()
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error { return nil })
}

func TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil(t *testing.T) {
    r := NewHTTPRegistry()
    r.Freeze()
    if v := r.PerRouteValidator("envoy.filters.http.header_mutation"); v != nil {
        t.Errorf("unregistered validator should return nil; got %v", v)
    }
}

func TestRegistry_PerRouteValidator_DoesNotConflictWithRegister(t *testing.T) {
    r := NewHTTPRegistry()
    r.Register("envoy.filters.http.header_mutation", func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
        return nil, errors.New("test factory")
    })
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error { return nil })
    r.Freeze()
    if r.Lookup("envoy.filters.http.header_mutation") == nil {
        t.Error("Register and RegisterPerRouteValidator should be independent")
    }
    if r.PerRouteValidator("envoy.filters.http.header_mutation") == nil {
        t.Error("PerRouteValidator should be retrievable")
    }
}
```

- [ ] **Step 3: Write failing tests in `internal/filter/http/perroute_test.go`** (extend, do not replace)

```go
func TestBuildPerRouteConfig_PerRouteValidator_NilSucceeds(t *testing.T) {
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("ok"))}
    // nil registry → backwards-compatible (no validator consulted)
    _, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, nil)
    if err != nil {
        t.Errorf("nil registry should succeed; got %v", err)
    }
}

func TestBuildPerRouteConfig_PerRouteValidator_NoValidatorRegisteredSucceeds(t *testing.T) {
    r := NewHTTPRegistry()
    r.Freeze()
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("ok"))}
    _, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
    if err != nil {
        t.Errorf("no validator registered should succeed; got %v", err)
    }
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRouteTier(t *testing.T) {
    r := NewHTTPRegistry()
    sentinelErr := errors.New("validator-rejection")
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
        return sentinelErr
    })
    r.Freeze()
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
    _, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
    if err == nil {
        t.Fatal("expected error; got nil")
    }
    if !strings.Contains(err.Error(), "validator-rejection") {
        t.Errorf("error should wrap validator error; got %v", err)
    }
    if !strings.Contains(err.Error(), "routes[0]") {
        t.Errorf("error should carry route-tier location prefix; got %v", err)
    }
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnVHostTier(t *testing.T) {
    r := NewHTTPRegistry()
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
        return errors.New("validator-rejection")
    })
    r.Freeze()
    chainNames := []string{"envoy.filters.http.header_mutation"}
    vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
    _, err := BuildPerRouteConfig(nil, []routeScope{{VHost: vhCfg, Route: nil}}, chainNames, r)
    if err == nil || !strings.Contains(err.Error(), "virtual_hosts[0]") {
        t.Errorf("expected vhost-tier location prefix; got %v", err)
    }
}

func TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRCTier(t *testing.T) {
    r := NewHTTPRegistry()
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
        return errors.New("validator-rejection")
    })
    r.Freeze()
    chainNames := []string{"envoy.filters.http.header_mutation"}
    rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("triggers-error"))}
    _, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: nil, Route: nil}}, chainNames, r)
    if err == nil || !strings.Contains(err.Error(), "route_config") {
        t.Errorf("expected rc-tier location prefix; got %v", err)
    }
}

func TestBuildPerRouteConfig_PerRouteValidator_OnlyConsultedForRegisteredFilters(t *testing.T) {
    // Two filters in chain; only one has a validator. The non-validated one's
    // per-route configs are accepted unconditionally.
    r := NewHTTPRegistry()
    r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
        return errors.New("validator-rejection")
    })
    r.Freeze()
    chainNames := []string{"envoy.filters.http.cors", "envoy.filters.http.header_mutation"}
    // Cors per-route config — header_mutation NOT in any tier.
    rtCfg := map[string]*anypb.Any{"envoy.filters.http.cors": mustAny(t, wrapperspb.String("cors-policy"))}
    _, err := BuildPerRouteConfig(nil, []routeScope{{VHost: nil, Route: rtCfg}}, chainNames, r)
    if err != nil {
        t.Errorf("unrelated filter's config should not trigger validator; got %v", err)
    }
}
```

- [ ] **Step 4: Run failing tests; confirm compile errors (method + signature do not exist)**

```bash
go test ./internal/filter/http/... -run 'TestRegistry_RegisterPerRouteValidator|TestBuildPerRouteConfig_PerRouteValidator' 2>&1 | head -20
```

Expected: compile error mentioning `RegisterPerRouteValidator undefined` and/or signature mismatch on `BuildPerRouteConfig`.

- [ ] **Step 5: Implement registry hooks in `internal/filter/http/registry.go`**

Read the existing file first to find the natural insertion point:

```bash
go doc -all internal/filter/http HTTPRegistry | head -40
```

Then add the field + methods. The exact placement depends on the file's existing structure; insert the new field alongside the existing factories map and the new methods alongside `Register` / `Freeze` / `Lookup`:

```go
// Field added to *HTTPRegistry:
//   perRouteValidators map[string]func(proto.Message) error // per planner-time decision 3 + ADR-0110

// Initialise in NewHTTPRegistry():
//   perRouteValidators: make(map[string]func(proto.Message) error),

// New methods:

// RegisterPerRouteValidator registers a validator function for filterName that
// BuildPerRouteConfig invokes against each parsed proto.Message at each tier
// (Route, VirtualHost, RouteConfiguration) at HCM-build time. Used by filters
// like envoy.filters.http.header_mutation whose per-route configs need
// boot-time validation (e.g., the protected-header set check per ADR-0111).
//
// MUST be called BEFORE Freeze(); panics otherwise (mirrors the existing
// Register/Freeze discipline per ADR-0072).
//
// Per planner-time decision 3 + ADR-0110.
func (r *HTTPRegistry) RegisterPerRouteValidator(filterName string, validator func(proto.Message) error) {
    if r.frozen {
        panic(fmt.Sprintf("HTTPRegistry: RegisterPerRouteValidator(%q) after Freeze", filterName))
    }
    r.perRouteValidators[filterName] = validator
}

// PerRouteValidator returns the registered validator for filterName, or nil if
// none registered. Safe to call after Freeze. Consumed by BuildPerRouteConfig.
func (r *HTTPRegistry) PerRouteValidator(filterName string) func(proto.Message) error {
    if r == nil {
        return nil
    }
    return r.perRouteValidators[filterName]
}
```

If the existing `frozen` field is named differently or accessed via a getter, adapt; the `panic`-after-freeze discipline must match the existing `Register` method's semantics.

- [ ] **Step 6: Widen `BuildPerRouteConfig` signature in `internal/filter/http/perroute.go`**

The signature changes from:

```go
func BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string) (*PerRouteConfig, error)
```

to:

```go
func BuildPerRouteConfig(rcCfg map[string]*anypb.Any, scopes []routeScope, chainNames []string, reg *HTTPRegistry) (*PerRouteConfig, error)
```

Inside the body, AFTER the existing `parseMap` calls succeed and `out.rc` + `out.scopes` are populated, add a validator pass (BEFORE `return out, nil`):

```go
    // Per-route validation hook per ADR-0110 + planner-time decision 3:
    // for each filter name in chainNames that has a registered validator,
    // run the validator against each parsed proto.Message at each tier.
    // Surfaces per-route protected-header violations (per ADR-0111) as
    // boot-time errors mirroring Envoy v1.37.2's CONFIG-LOAD-TIME
    // enforcement per phase 10 SPEC §11.1.
    if reg != nil {
        for _, name := range chainNames {
            v := reg.PerRouteValidator(name)
            if v == nil {
                continue
            }
            if msg, ok := out.rc[name]; ok {
                if err := v(msg); err != nil {
                    return nil, fmt.Errorf("hcm: route_config: typed_per_filter_config[%q]: %w", name, err)
                }
            }
            for i, sp := range out.scopes {
                if msg, ok := sp.vhost[name]; ok {
                    if err := v(msg); err != nil {
                        return nil, fmt.Errorf("hcm: route_config.virtual_hosts[%d]: typed_per_filter_config[%q]: %w", i, name, err)
                    }
                }
                if msg, ok := sp.route[name]; ok {
                    if err := v(msg); err != nil {
                        return nil, fmt.Errorf("hcm: route_config.virtual_hosts[%d].routes[%d]: typed_per_filter_config[%q]: %w", i, i, name, err)
                    }
                }
            }
        }
    }
    return out, nil
```

NOTE: the route-index in the location prefix uses `i` for both the VHost index AND the route index because the existing `parseMap` does the same (see `perroute.go:91`: `fmt.Sprintf("route_config.virtual_hosts[%d].routes[%d]", i, i)`). This is a pre-existing pattern — preserve it.

- [ ] **Step 7: Sweep test-only callers — pass `nil` for the new parameter**

The 6 existing tests at `perroute_test.go:21–101` (TestPerRoute_BuildAndResolve_RouteWins / VHostFallback / RCFallback / NilOnAbsent / BuildRejectsUnknownFilterName / LazyCacheHitMiss) call `BuildPerRouteConfig(rcCfg, []routeScope{...}, chainNames)` — append `, nil` to each call. Mechanical sed-replace, but the implementer manually verifies each call site for context.

```bash
grep -nE 'BuildPerRouteConfig\(' internal/filter/http/perroute_test.go
# Update each call to thread nil as the fourth argument.
```

If cors/fault test files (or other test files surfaced in Step 1) call `BuildPerRouteConfig` directly, sweep those too.

The new tests added in Step 3 already call the 4-argument form correctly.

- [ ] **Step 8: Update HCM call site in `internal/filter/hcm/config.go`**

```bash
grep -nE 'BuildPerRouteConfig\(' internal/filter/hcm/config.go
```

The call site looks like (approximately):

```go
perRoute, err := filter_http.BuildPerRouteConfig(rcCfg, scopes, chainNames)
```

Widen to:

```go
perRoute, err := filter_http.BuildPerRouteConfig(rcCfg, scopes, chainNames, httpRegistry)
```

The `httpRegistry` local should already be in scope at the call site (the HCM passes it to `parseHTTPFiltersChain` per the phase-09 framework extension).

- [ ] **Step 9: Run tests; confirm pass**

```bash
go test -race ./internal/filter/http/... -v 2>&1 | tail -40
go test -race ./internal/filter/hcm/... -v 2>&1 | tail -20
```

Expected: every test PASS including the new TestRegistry_* and TestBuildPerRouteConfig_PerRouteValidator_* tests.

- [ ] **Step 10: Run full unit-test suite + vet + lint**

```bash
go test -race -count=1 ./...                                  # expect: all PASS
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
```

- [ ] **Step 11: Commit**

```bash
git add internal/filter/http/registry.go internal/filter/http/registry_test.go internal/filter/http/perroute.go internal/filter/http/perroute_test.go internal/filter/hcm/config.go
# Plus any other test files swept in Step 7.
git commit -m "phase 10: framework — HTTPRegistry.RegisterPerRouteValidator + BuildPerRouteConfig integration per ADR-0110"
```

SHA-fill follow-up.

*Anchored: SPEC §6.7 + §11.1 (e); ADR-0110 framework piece (3 of 3); planner-time decision 3 (eager); SPEC §12 deferred decision 3.*

---

## Task 5: `internal/filter/http/header_mutation/` package — doc.go + header_mutation.go skeleton (TypeURL, types, runtimeConfig + parser, New factory + protected-header validation, listener-level + per-route validator function) + header_mutation_test.go (New-time tests) [ADR-0108, ADR-0109, ADR-0111]

**Files:**
- Create: `internal/filter/http/header_mutation/doc.go`
- Create: `internal/filter/http/header_mutation/header_mutation.go`
- Create: `internal/filter/http/header_mutation/header_mutation_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0108, ADR-0109, ADR-0111)

This task lands the new `internal/filter/http/header_mutation/` package with:

- The public API surface (`TypeURL` constant + `New` HTTPFilterFactory).
- The unexported types (`runtimeConfig`, `mutationOpKind`, `compiledMutationOp`, `filter`).
- The `compileOps` helper + `isProtectedHeader` predicate per planner-time decision 5.
- The `validatePerRouteHeaderMutation` per-route-validator function (registered with `ctx.Registry.RegisterPerRouteValidator` at boot per planner-time decision 3).
- The `New` factory body parsing + validating the typed_config (rejects nil tc, malformed Any, AND each mutation's `headerName` against the 6-name protected set per §11.1 — returns a non-nil error mirroring Envoy's verbatim message).
- The `*filter` instance shape + `SetDecoderCallbacks` + `SetEncoderCallbacks` + pass-through methods for OnDestroy + DecodeData + EncodeData + DecodeTrailers + EncodeTrailers.
- The DecodeHeaders + EncodeHeaders bodies are **STUBBED** to `return Continue` in this task — Tasks 6 + 7 + 8 land the full bodies. Task 5's commit therefore lands a "compiles + parses + validates + registers per-route validator" filter that does NOT yet apply mutations.

**ADR-0108** (package shape + boot registration), **ADR-0109** (runtimeConfig + AppendAction × 4 + keep_empty_value semantics — partial; the apply-loop semantics land in Task 6), and **ADR-0111** (protected-header set + CONFIG-LOAD-TIME rejection — listener-level piece; the per-route-validator integration is empirical-tested at Task 7) all land here.

**Precondition:** Task 4 done; `internal/filter/http/header_mutation/` does not exist.
**Artifact:** three new files (doc + impl + unit tests); three ADRs in DECISIONS.md.
**Acceptance:** `go build ./internal/filter/http/header_mutation/...` clean; `go test -race ./internal/filter/http/header_mutation/...` passes the New-time test suite (the DecodeHeaders/EncodeHeaders unit tests in Tasks 6/7/8 are not yet present); `go vet ./...` clean; ADR-0108, ADR-0109, ADR-0111 in DECISIONS.md.

- [ ] **Step 1: Create `internal/filter/http/header_mutation/doc.go`**

```go
// Package header_mutation implements the envoy.filters.http.header_mutation
// HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 10: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.5
// empirical scrapes of reference Envoy v1.37.2.
//
// Decode side (per SPEC §6.6):
//
//   1. Apply listener-level cfg.requestOps in proto-declared order
//      (per the proto comment at header_mutation.pb.go:141–142:
//      "filter configuration will always be applied first").
//   2. Resolve all 3 per-route tiers via dcb.RequestRouteConfigsAllTiers
//      (Route, VirtualHost, RouteConfiguration; unmerged).
//   3. Compile each non-nil tier's request_mutations into compiledMutationOp
//      slices.
//   4. Apply tiers in flag-controlled order:
//        - mostSpecificHeaderMutationsWins=false (DEFAULT):
//            Route → VirtualHost → RouteConfiguration
//          (least-specific applied LAST, wins overlap)
//        - mostSpecificHeaderMutationsWins=true:
//            RouteConfiguration → VirtualHost → Route
//          (most-specific applied LAST, wins overlap)
//   5. Return Continue.
//
// Encode side (per SPEC §6.8): symmetric algorithm against response_mutations
// using the SAME dcb.RequestRouteConfigsAllTiers callback (per planner-time
// decision 1 — DECODER-ONLY callback used from BOTH decode AND encode bodies;
// mirrors cors precedent at cors.go:163).
//
// Concurrency model (per SPEC §5.7): per-instance state is race-free by the
// single-goroutine-per-stream invariant per ADR-0071 (no synchronization
// needed); *runtimeConfig is read-only after New (multiple per-request *filter
// instances share via closure capture — read-only sharing is race-free); no
// timer goroutines, no shared atomic state, no SendLocalReply path. The
// maximally simple concurrency model.
//
// Public surface (per SPEC §6.1):
//
//   TypeURL = "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"
//   New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
//
// New body discipline (per ADR-0109 + ADR-0111):
//
//   1. tc must be non-nil (boot-fail-fast per ADR-0072).
//   2. Unmarshal tc to *envoyextensionsfiltershttpheadermutationv3.HeaderMutation.
//   3. Compile listener-level mutations.{request,response}_mutations via
//      compileOps; each headerName validated against the protected set per
//      §11.1 (the 6-name set: ":method", ":path", ":authority", ":scheme",
//      ":status", "host" case-insensitive on host). Returns error on first
//      protected-header violation with verbatim format
//      "header_mutation: %q is :-prefixed or host; may not be modified"
//      mirroring Envoy v1.37.2's source/server/server.cc:453 message.
//   4. Capture mostSpecificHeaderMutationsWins flag.
//   5. Construct *runtimeConfig.
//   6. Register per-route validator via ctx.Registry.RegisterPerRouteValidator
//      (per ADR-0110 + planner-time decision 3) so HCM-build-time
//      BuildPerRouteConfig surfaces per-route protected-header violations
//      identical-in-effect to listener-level (boot-fail-fast).
//   7. Return FilterInstanceFactory closure.
//
// Stats: ZERO emitted (per SPEC §11.3 confirmation; analogous to cors per
// ADR-0074). The 3-field FactoryCtx per ADR-0100 stays as-is; phase 10 does
// not consume ctx.Stats or ctx.StatPrefix.
//
// Cross-cutting ADR anchors:
//
//   - ADR-0108: package shape + boot registration
//   - ADR-0109: runtimeConfig + compiledMutationOp + AppendAction × 4 +
//     keep_empty_value semantics + multi-value collapse / preserve per §11.4
//   - ADR-0110: multi-tier per-route evaluation (ResolveAllTiers +
//     RequestRouteConfigsAllTiers + RegisterPerRouteValidator + accessor-
//     choice discipline + cross-tier algorithm); amends ADR-0073
//   - ADR-0111: protected-header set + CONFIG-LOAD-TIME rejection (MAJOR
//     amendment to BRAINSTORM Decision 11)
//   - ADR-0112: mutations.query_parameter_mutations[] DEFERRED
//   - ADR-0113: header-value formatter substitution DEFERRED
//
// Forward-pointers (deferred per ADR-0040 format):
//
//   - mutations.query_parameter_mutations[] (KeyValueMutation triple +
//     path-query rewriting subsystem) — silently parsed; no behavioral
//     effect (ADR-0112).
//   - Header-value formatter substitution syntax (%REQ(:path)% etc) — values
//     materialized verbatim as static strings (ADR-0113).
package header_mutation
```

- [ ] **Step 2: Create `internal/filter/http/header_mutation/header_mutation.go` with the parser + factory + stub DecodeHeaders/EncodeHeaders bodies**

```go
package header_mutation

import (
    "errors"
    "fmt"
    "net/http"
    "strings"

    commonmutationrulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
    corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
    headermutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
    "google.golang.org/protobuf/proto"
    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.header_mutation typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 9) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation"

// filterName is the canonical http_filters[].name string for header_mutation
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.header_mutation"

// runtimeConfig is the per-instance parsed config shape per ADR-0109.
//
// Three fields consumed at request-eval time (listener-level requestOps,
// listener-level responseOps, mostSpecificHeaderMutationsWins); one
// HeaderMutation field silently ignored per ADR-0112 deferral
// (mutations.query_parameter_mutations).
type runtimeConfig struct {
    requestOps                       []compiledMutationOp // listener-level request mutations (proto-declared order)
    responseOps                      []compiledMutationOp // listener-level response mutations (proto-declared order)
    mostSpecificHeaderMutationsWins  bool                  // precedence-order flag (default false)
}

// mutationOpKind is the discriminator for compiledMutationOp.
type mutationOpKind uint8

const (
    kindRemove mutationOpKind = iota
    kindAppend
)

// compiledMutationOp is the flat per-mutation struct produced by compileOps.
// Value-typed per planner-time decision 4 for cache locality during the
// apply-loop iteration. Read-only after New / per-route compile.
type compiledMutationOp struct {
    kind           mutationOpKind                                    // kindRemove or kindAppend
    headerName     string                                            // canonicalized via http.CanonicalHeaderKey at parse time
    headerValue    string                                            // for kindAppend only ("" for kindRemove)
    appendAction   corev3.HeaderValueOption_HeaderAppendAction       // 4 variants; for kindAppend only
    keepEmptyValue bool                                              // for kindAppend only
}

// New is the HTTPFilterFactory exposed at boot. Per ADR-0108 + ADR-0109 + ADR-0111:
//
//  1. tc must be non-nil (a header_mutation filter with no typed_config has
//     no behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *headermutationv3.HeaderMutation; return error on
//     malformed Any.
//  3. Compile listener-level request + response mutations via compileOps;
//     each headerName validated against the protected set per §11.1.
//  4. Capture most_specific_header_mutations_wins from the proto.
//  5. Register per-route validator with ctx.Registry per planner-time
//     decision 3 (so BuildPerRouteConfig surfaces per-route protected-header
//     violations as boot-time errors, mirroring listener-level discipline).
//  6. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to *runtimeConfig.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("header_mutation: typed_config required")
    }
    var c headermutationv3.HeaderMutation
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("header_mutation: unmarshal: %w", err)
    }
    rc, err := buildRuntimeConfig(&c)
    if err != nil {
        return nil, err
    }
    // Register per-route validator (per planner-time decision 3 + ADR-0110).
    // Idempotent across multiple calls to New (same filter, multiple HCMs):
    // RegisterPerRouteValidator overwrites the entry, but the validator function
    // is identical so the overwrite is benign.
    if ctx.Registry != nil {
        ctx.Registry.RegisterPerRouteValidator(filterName, validatePerRouteHeaderMutation)
    }
    return func() envoyhttp.HTTPFilter {
        f := &filter{cfg: rc}
        return envoyhttp.HTTPFilter{
            Name:    filterName,
            Decoder: f,
            Encoder: f,
        }
    }, nil
}

// buildRuntimeConfig projects *HeaderMutation into the runtimeConfig shape per §6.2.
func buildRuntimeConfig(c *headermutationv3.HeaderMutation) (*runtimeConfig, error) {
    rc := &runtimeConfig{
        mostSpecificHeaderMutationsWins: c.GetMostSpecificHeaderMutationsWins(),
    }
    if m := c.GetMutations(); m != nil {
        ops, err := compileOps(m.GetRequestMutations())
        if err != nil {
            return nil, err
        }
        rc.requestOps = ops
        ops, err = compileOps(m.GetResponseMutations())
        if err != nil {
            return nil, err
        }
        rc.responseOps = ops
        // mutations.query_parameter_mutations silently ignored per ADR-0112.
    }
    return rc, nil
}

// compileOps projects []*HeaderMutation (the proto primitive in
// config/common/mutation_rules/v3) into []compiledMutationOp. Each input op
// must EITHER set Action.Remove (kindRemove) OR set Action.Append (kindAppend).
// Validates each headerName against the protected-header set per §11.1.
// Returns error on the first protected-header violation.
//
// Used by both:
//   - New: compiles listener-level mutations
//   - validatePerRouteHeaderMutation: compiles per-route HeaderMutationPerRoute
//     mutations to surface protected-header violations at HCM-build time
func compileOps(in []*commonmutationrulesv3.HeaderMutation) ([]compiledMutationOp, error) {
    if len(in) == 0 {
        return nil, nil
    }
    out := make([]compiledMutationOp, 0, len(in))
    for _, m := range in {
        switch a := m.GetAction().(type) {
        case *commonmutationrulesv3.HeaderMutation_Remove:
            name := a.Remove
            if isProtectedHeader(name) {
                return nil, fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", name)
            }
            out = append(out, compiledMutationOp{
                kind:       kindRemove,
                headerName: http.CanonicalHeaderKey(name),
            })
        case *commonmutationrulesv3.HeaderMutation_Append:
            hvo := a.Append
            if hvo == nil || hvo.GetHeader() == nil {
                continue // defensive: empty Append is a no-op
            }
            name := hvo.GetHeader().GetKey()
            if isProtectedHeader(name) {
                return nil, fmt.Errorf("header_mutation: %q is :-prefixed or host; may not be modified", name)
            }
            out = append(out, compiledMutationOp{
                kind:           kindAppend,
                headerName:     http.CanonicalHeaderKey(name),
                headerValue:    hvo.GetHeader().GetValue(),
                appendAction:   hvo.GetAppendAction(),
                keepEmptyValue: hvo.GetKeepEmptyValue(),
            })
        default:
            // Unknown / unset action — defensive skip.
            continue
        }
    }
    return out, nil
}

// isProtectedHeader returns true if name is in the 6-name protected set per §11.1.
//
// Per planner-time decision 5: prefix-check on `:` future-proofs against new
// pseudo-headers (Envoy may add `:protocol` or `:upgrade` later); equality
// on `host` is case-insensitive (Envoy rejects `host`, `Host`, `HOST`
// symmetrically per §11.1 conclusion (b)).
func isProtectedHeader(name string) bool {
    if strings.HasPrefix(name, ":") {
        return true
    }
    return strings.EqualFold(name, "host")
}

// validatePerRouteHeaderMutation is the per-route validator registered with
// the framework's *HTTPRegistry per planner-time decision 3 + ADR-0110.
// At HCM-build time, BuildPerRouteConfig invokes this against each parsed
// HeaderMutationPerRoute proto.Message at each tier (Route, VirtualHost,
// RouteConfiguration). Returns the first protected-header violation as an
// error; framework wraps with location prefix.
func validatePerRouteHeaderMutation(msg proto.Message) error {
    pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
    if !ok {
        // Defensive: should not happen if BuildPerRouteConfig parses the
        // typed_config Any to *HeaderMutationPerRoute correctly.
        return nil
    }
    m := pr.GetMutations()
    if m == nil {
        return nil
    }
    if _, err := compileOps(m.GetRequestMutations()); err != nil {
        return err
    }
    if _, err := compileOps(m.GetResponseMutations()); err != nil {
        return err
    }
    return nil
}

// filter is the per-request header_mutation instance. Per-instance state is
// race-free by the single-goroutine-per-stream invariant per ADR-0071.
//
// Per-route configs are resolved per-request via f.dcb.RequestRouteConfigsAllTiers
// (Tasks 7/8 land the resolution + apply-loop). Task 5 lands the stub
// DecodeHeaders/EncodeHeaders that simply return Continue.
type filter struct {
    cfg *runtimeConfig

    dcb envoyhttp.DecoderFilterCallbacks
    ecb envoyhttp.EncoderFilterCallbacks
}

// Statically assert the both-sides interface conformance (matches cors + fault precedents).
var (
    _ envoyhttp.StreamDecoderFilter = (*filter)(nil)
    _ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders is STUBBED in Task 5. Task 7 lands the full body per SPEC §6.6.
func (f *filter) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
    return envoyhttp.Continue
}

// EncodeHeaders is STUBBED in Task 5. Task 8 lands the full body per SPEC §6.8.
func (f *filter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
    return envoyhttp.Continue
}

// Pass-through methods (data + trailers + OnDestroy).
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) OnDestroy() {} // no timers, no async state — nothing to release
```

NOTE: import names like `headermutationv3` need to be settled at impl time per the actual go-control-plane proto package layout — `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3` and `go doc github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3` confirm the exact package paths at Task 5 step 0.

- [ ] **Step 3: Create `internal/filter/http/header_mutation/header_mutation_test.go` with the New-time tests**

```go
package header_mutation

import (
    "strings"
    "testing"

    commonmutationrulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
    corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
    headermutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
    "google.golang.org/protobuf/proto"
    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
    t.Helper()
    a, err := anypb.New(m)
    if err != nil {
        t.Fatalf("anypb.New: %v", err)
    }
    return a
}

func mkAppendOp(name, value string, action corev3.HeaderValueOption_HeaderAppendAction) *commonmutationrulesv3.HeaderMutation {
    return &commonmutationrulesv3.HeaderMutation{
        Action: &commonmutationrulesv3.HeaderMutation_Append{
            Append: &corev3.HeaderValueOption{
                Header:       &corev3.HeaderValue{Key: name, Value: value},
                AppendAction: action,
            },
        },
    }
}

func mkRemoveOp(name string) *commonmutationrulesv3.HeaderMutation {
    return &commonmutationrulesv3.HeaderMutation{
        Action: &commonmutationrulesv3.HeaderMutation_Remove{Remove: name},
    }
}

func TestNew_NilTC(t *testing.T) {
    _, err := New(nil, envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err == nil {
        t.Fatal("expected error for nil tc; got nil")
    }
    if !strings.Contains(err.Error(), "typed_config required") {
        t.Errorf("error: got %v, want 'typed_config required'", err)
    }
}

func TestNew_MalformedTC(t *testing.T) {
    bad := &anypb.Any{TypeUrl: "type.googleapis.com/garbage", Value: []byte{0xff, 0xff, 0xff}}
    _, err := New(bad, envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err == nil {
        t.Fatal("expected error for malformed tc; got nil")
    }
}

func TestNew_ProtectedHeader(t *testing.T) {
    cases := []struct {
        name       string
        headerName string
        side       string // "request" or "response"
    }{
        {"method-request", ":method", "request"},
        {"path-request", ":path", "request"},
        {"authority-request", ":authority", "request"},
        {"scheme-request", ":scheme", "request"},
        {"status-request", ":status", "request"},
        {"host-lower-request", "host", "request"},
        {"host-title-request", "Host", "request"},
        {"host-upper-request", "HOST", "request"},
        {"status-response", ":status", "response"},
        {"host-response", "host", "response"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}}
            op := mkAppendOp(tc.headerName, "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)
            switch tc.side {
            case "request":
                mut.Mutations.RequestMutations = []*commonmutationrulesv3.HeaderMutation{op}
            case "response":
                mut.Mutations.ResponseMutations = []*commonmutationrulesv3.HeaderMutation{op}
            }
            _, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
            if err == nil {
                t.Fatalf("%s: expected protected-header error; got nil", tc.headerName)
            }
            // Verbatim message format per ADR-0111 / SPEC §11.1 (f).
            wantPrefix := "header_mutation: "
            wantSuffix := " is :-prefixed or host; may not be modified"
            if !strings.Contains(err.Error(), wantPrefix) || !strings.Contains(err.Error(), wantSuffix) {
                t.Errorf("error: got %q, want '%s\"%s\"%s'", err, wantPrefix, tc.headerName, wantSuffix)
            }
        })
    }
}

func TestNew_ProtectedHeader_RemoveAlsoRejected(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkRemoveOp(":path")},
        },
    }
    _, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err == nil {
        t.Fatal("expected error for Remove of :path; got nil")
    }
}

func TestNew_HappyPath_ListenerLevelOnly(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-test", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
                mkAppendOp("x-add", "v2", corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD),
                mkRemoveOp("user-agent"),
            },
            ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-resp", "rv", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
        MostSpecificHeaderMutationsWins: true,
    }
    factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err != nil {
        t.Fatalf("happy path: %v", err)
    }
    inst := factory()
    if inst.Decoder == nil || inst.Encoder == nil {
        t.Errorf("expected both Decoder and Encoder set; got %+v", inst)
    }
    if inst.Name != filterName {
        t.Errorf("Name: got %q, want %q", inst.Name, filterName)
    }
}

func TestRuntimeConfig_FieldExtraction(t *testing.T) {
    c := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-req", "rv", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
            ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
                mkRemoveOp("x-rm"),
            },
        },
        MostSpecificHeaderMutationsWins: true,
    }
    rc, err := buildRuntimeConfig(c)
    if err != nil {
        t.Fatalf("buildRuntimeConfig: %v", err)
    }
    if !rc.mostSpecificHeaderMutationsWins {
        t.Error("flag should be true")
    }
    if len(rc.requestOps) != 1 || rc.requestOps[0].kind != kindAppend {
        t.Errorf("requestOps: got %+v", rc.requestOps)
    }
    if len(rc.responseOps) != 1 || rc.responseOps[0].kind != kindRemove {
        t.Errorf("responseOps: got %+v", rc.responseOps)
    }
}

func TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored(t *testing.T) {
    // Construct a HeaderMutation with non-empty query_parameter_mutations.
    // The field is deferred per ADR-0112 — should not error; should not produce ops.
    c := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            QueryParameterMutations: []*commonmutationrulesv3.KeyValueMutation{
                {AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD, Append: &corev3.HeaderValue{Key: "q", Value: "v"}},
            },
        },
    }
    rc, err := buildRuntimeConfig(c)
    if err != nil {
        t.Fatalf("query_parameter_mutations should be silently ignored; got %v", err)
    }
    if len(rc.requestOps) != 0 || len(rc.responseOps) != 0 {
        t.Errorf("ops should be empty; got requestOps=%d responseOps=%d", len(rc.requestOps), len(rc.responseOps))
    }
}

func TestCompiledMutationOp_AllAppendActionsParse(t *testing.T) {
    cases := []struct {
        action corev3.HeaderValueOption_HeaderAppendAction
        want   corev3.HeaderValueOption_HeaderAppendAction
    }{
        {corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD, corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD},
        {corev3.HeaderValueOption_ADD_IF_ABSENT, corev3.HeaderValueOption_ADD_IF_ABSENT},
        {corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD, corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD},
        {corev3.HeaderValueOption_OVERWRITE_IF_EXISTS, corev3.HeaderValueOption_OVERWRITE_IF_EXISTS},
    }
    for _, tc := range cases {
        t.Run(tc.action.String(), func(t *testing.T) {
            ops, err := compileOps([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x", "v", tc.action)})
            if err != nil || len(ops) != 1 {
                t.Fatalf("compileOps: err=%v ops=%d", err, len(ops))
            }
            if ops[0].appendAction != tc.want {
                t.Errorf("appendAction: got %v, want %v", ops[0].appendAction, tc.want)
            }
        })
    }
}

func TestCompiledMutationOp_RemoveAndAppend(t *testing.T) {
    in := []*commonmutationrulesv3.HeaderMutation{
        mkRemoveOp("x-rm"),
        mkAppendOp("x-add", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
    }
    ops, err := compileOps(in)
    if err != nil || len(ops) != 2 {
        t.Fatalf("compileOps: err=%v ops=%d", err, len(ops))
    }
    if ops[0].kind != kindRemove || ops[0].headerName != "X-Rm" {
        t.Errorf("ops[0]: got %+v", ops[0])
    }
    if ops[1].kind != kindAppend || ops[1].headerName != "X-Add" || ops[1].headerValue != "v" {
        t.Errorf("ops[1]: got %+v", ops[1])
    }
}

func TestNew_RegistersPerRouteValidator(t *testing.T) {
    r := envoyhttp.NewHTTPRegistry()
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
    }
    _, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: r})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    v := r.PerRouteValidator(filterName)
    if v == nil {
        t.Fatal("expected per-route validator registered after New; got nil")
    }
    // Sanity: validator accepts a valid HeaderMutationPerRoute.
    okMsg := &headermutationv3.HeaderMutationPerRoute{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
        },
    }
    if err := v(okMsg); err != nil {
        t.Errorf("validator should accept valid msg; got %v", err)
    }
    // Validator rejects protected-header mutation.
    badMsg := &headermutationv3.HeaderMutationPerRoute{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp(":path", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
        },
    }
    if err := v(badMsg); err == nil {
        t.Error("validator should reject protected-header mutation; got nil")
    }
}

func TestIsProtectedHeader(t *testing.T) {
    protected := []string{":method", ":path", ":authority", ":scheme", ":status", "host", "Host", "HOST", ":anything"}
    for _, n := range protected {
        if !isProtectedHeader(n) {
            t.Errorf("isProtectedHeader(%q): got false, want true", n)
        }
    }
    allowed := []string{"x-test", "user-agent", "content-length", "x-host-something"}
    for _, n := range allowed {
        if isProtectedHeader(n) {
            t.Errorf("isProtectedHeader(%q): got true, want false", n)
        }
    }
}
```

- [ ] **Step 4: Run tests; confirm pass**

```bash
go test -race ./internal/filter/http/header_mutation/... -v 2>&1 | tail -40
```

Expected: all tests PASS.

- [ ] **Step 5: Run full suite + lint**

```bash
go test -race -count=1 ./...                                  # expect: all PASS
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
```

- [ ] **Step 6: Append ADR-0108, ADR-0109, ADR-0111 to `docs/envoy-go/DECISIONS.md`**

Each ADR follows the 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). The implementer drafts the bodies with these key anchors:

- **ADR-0108** — package shape per cors + fault precedent; TypeURL constant; New factory; `internal/filter/http/header_mutation/` mirroring `internal/filter/http/fault/` 4-file split (doc.go + header_mutation.go + header_mutation_test.go + fuzz_test.go); registration line in cmd/envoy-go/main.go alphabetically after fault per the router-first-then-alphabetical convention codified at phase-09. §Context cites SPEC §1 + §4.1 + §6.1; §Decision cites SPEC §6.1; §Consequences notes (a) the filter set extends from {cors, envoygotest, router, fault} to {cors, envoygotest, router, fault, header_mutation} per ADR-0074 cross-reference, (b) the FactoryCtx Stats + StatPrefix fields per ADR-0100 are NOT consumed (zero stats per §11.3 — the FactoryCtx fields are opt-in per filter, analogous to cors per ADR-0074).
- **ADR-0109** — runtimeConfig 3-field shape + 1-field silent-ignore (query_parameter_mutations per ADR-0112); compiledMutationOp value-typed flat struct per planner-time decision 4 + AppendAction × 4 mapping table verbatim from SPEC §6.6 + keep_empty_value semantics per §11.2 + multi-value collapse/preserve per §11.4. §Context cites SPEC §6.2 + §6.4 + §6.6 + §11.2 + §11.4; §Decision codifies the apply-loop algorithm verbatim. §Consequences notes the cross-reference to ADR-0101 (fault's runtimeConfig precedent) — same structural pattern (closure-capture + parse-at-New + read-only-shared-after-New) — header_mutation is simpler (no async, no stats).
- **ADR-0111** — protected-header set per §11.1 + CONFIG-LOAD-TIME rejection (NOT runtime no-op as BRAINSTORM hypothesized — MAJOR amendment per §11.1 (e)); the 6-name set ({:method, :path, :authority, :scheme, :status, host} case-insensitive on host); verbatim error format `"header_mutation: %q is :-prefixed or host; may not be modified"` mirroring Envoy v1.37.2's `:-prefixed or host headers may not be modified` per source/server/server.cc:453; EAGER per-route validation lifecycle via the framework's `RegisterPerRouteValidator` hook per planner-time decision 3. §Context cites SPEC §1.1 + §6.1 step 3 + §6.7 + §11.1; §Decision codifies the protected-set predicate (prefix-check on `:` + case-insensitive equality on `host`) per planner-time decision 5; §Consequences notes the framework gains a per-route-validator hook that future filters with similar invariants reuse.

Format each per the template; cross-reference the SPEC section + the lands-in-task identifier.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/header_mutation/ docs/envoy-go/DECISIONS.md
git commit -m "phase 10: header_mutation package + parser + protected-header validation [ADR-0108, ADR-0109, ADR-0111]"
```

SHA-fill follow-up.

*Anchored: SPEC §1 + §4.1 + §6.1–§6.4 + §11.1 + §14.1; ADR-0108 + ADR-0109 + ADR-0111; planner-time decisions 4, 5.*

---

## Task 6: `header_mutation.go` `applyOps` + `applyAppendAction` + AppendAction × 4 unit tests + `keep_empty_value` boundary + multi-value collapse/preserve tests

**Files:**
- Modify: `internal/filter/http/header_mutation/header_mutation.go` (add `applyOps` + `applyAppendAction`)
- Modify: `internal/filter/http/header_mutation/header_mutation_test.go` (add the apply-loop tests)

This task lands the per-tier mutation-application algorithm: `applyOps(headers http.Header, ops []compiledMutationOp)` iterating the slice and dispatching to `applyAppendAction` for kindAppend or `headers.Del` for kindRemove. The `applyAppendAction` switch implements the 4 AppendAction variants per SPEC §6.6 + the keep_empty_value boundary per §11.2 + multi-value collapse (OVERWRITE) / preserve (APPEND) semantics per §11.4. ADR-0109's apply-loop algorithm is codified here. Tests cover every case from SPEC §14.1's Test* enumeration. NO new ADRs land in this task (ADR-0109 already landed in Task 5).

**Precondition:** Task 5 done.
**Artifact:** modified header_mutation.go + extended header_mutation_test.go.
**Acceptance:** all `TestApplyOps_*` tests PASS; race detector clean; vet + lint clean.

- [ ] **Step 1: Write failing tests** (extend header_mutation_test.go after the Step 3 tests of Task 5)

```go
func TestApplyOps_AppendIfExistsOrAdd_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
    if got := h.Values("X-Test"); len(got) != 1 || got[0] != "v" {
        t.Errorf("got %v, want [v]", got)
    }
}

func TestApplyOps_AppendIfExistsOrAdd_PresentMultiValue(t *testing.T) {
    h := http.Header{}
    h.Add("X-Test", "old1")
    h.Add("X-Test", "old2")
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
    if got := h.Values("X-Test"); len(got) != 3 || got[0] != "old1" || got[1] != "old2" || got[2] != "v" {
        t.Errorf("APPEND should preserve prior + add (per §11.4); got %v", got)
    }
}

func TestApplyOps_AddIfAbsent_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_ADD_IF_ABSENT}})
    if got := h.Get("X-Test"); got != "v" {
        t.Errorf("got %q, want v", got)
    }
}

func TestApplyOps_AddIfAbsent_PresentTarget(t *testing.T) {
    h := http.Header{}
    h.Set("X-Test", "old")
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_ADD_IF_ABSENT}})
    if got := h.Get("X-Test"); got != "old" {
        t.Errorf("got %q, want old (no-op)", got)
    }
}

func TestApplyOps_OverwriteIfExistsOrAdd_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}})
    if got := h.Get("X-Test"); got != "v" {
        t.Errorf("got %q, want v", got)
    }
}

func TestApplyOps_OverwriteIfExistsOrAdd_PresentMultiValue(t *testing.T) {
    h := http.Header{}
    h.Add("X-Test", "old1")
    h.Add("X-Test", "old2")
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD}})
    if got := h.Values("X-Test"); len(got) != 1 || got[0] != "v" {
        t.Errorf("OVERWRITE should collapse multi-value to single (per §11.4); got %v", got)
    }
}

func TestApplyOps_OverwriteIfExists_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
    if got := h.Get("X-Test"); got != "" {
        t.Errorf("got %q, want '' (no-op for absent target)", got)
    }
}

func TestApplyOps_OverwriteIfExists_PresentTarget(t *testing.T) {
    h := http.Header{}
    h.Set("X-Test", "old")
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "v", appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
    if got := h.Get("X-Test"); got != "v" {
        t.Errorf("got %q, want v", got)
    }
}

func TestApplyOps_Remove_PresentTarget(t *testing.T) {
    h := http.Header{}
    h.Set("X-Test", "old")
    applyOps(h, []compiledMutationOp{{kind: kindRemove, headerName: "X-Test"}})
    if h.Get("X-Test") != "" {
        t.Errorf("Remove should delete header")
    }
}

func TestApplyOps_Remove_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindRemove, headerName: "X-Test"}})
    if h.Get("X-Test") != "" {
        t.Errorf("Remove of absent header should be no-op")
    }
}

func TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions(t *testing.T) {
    actions := []corev3.HeaderValueOption_HeaderAppendAction{
        corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
        corev3.HeaderValueOption_ADD_IF_ABSENT,
        corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
        corev3.HeaderValueOption_OVERWRITE_IF_EXISTS,
    }
    for _, a := range actions {
        t.Run(a.String(), func(t *testing.T) {
            h := http.Header{}
            // Pre-existing target for OVERWRITE_IF_EXISTS to ensure even with EXISTS-true,
            // the keep_empty_value=false silent-skip fires FIRST per §11.2 conclusion (c).
            h.Set("X-Test", "original")
            applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: false, appendAction: a}})
            if got := h.Get("X-Test"); got != "original" {
                t.Errorf("%s: keep_empty_value=false + empty value should silent-skip; got %q want original", a, got)
            }
        })
    }
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_AppendIfExistsOrAdd(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD}})
    // Materialize empty value.
    if vs := h.Values("X-Test"); len(vs) != 1 || vs[0] != "" {
        t.Errorf("keep=true + empty + APPEND: got %v, want one empty value", vs)
    }
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_AbsentTarget(t *testing.T) {
    h := http.Header{}
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
    if got := h.Get("X-Test"); got != "" || len(h.Values("X-Test")) != 0 {
        t.Errorf("keep=true + empty + OVERWRITE_IF_EXISTS + absent target: should be no-op (EXISTS gate fires); got %v", h.Values("X-Test"))
    }
}

func TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_PresentTarget(t *testing.T) {
    h := http.Header{}
    h.Set("X-Test", "original")
    applyOps(h, []compiledMutationOp{{kind: kindAppend, headerName: "X-Test", headerValue: "", keepEmptyValue: true, appendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS}})
    if vs := h.Values("X-Test"); len(vs) != 1 || vs[0] != "" {
        t.Errorf("keep=true + empty + OVERWRITE_IF_EXISTS + present target: should replace with empty; got %v", vs)
    }
}
```

- [ ] **Step 2: Run failing tests; confirm compile error (`applyOps` undefined)**

```bash
go test ./internal/filter/http/header_mutation/... -run TestApplyOps 2>&1 | head -10
```

- [ ] **Step 3: Implement `applyOps` + `applyAppendAction` in `internal/filter/http/header_mutation/header_mutation.go`**

Insert after the existing `validatePerRouteHeaderMutation` function:

```go
// applyOps iterates ops in proto-declared order, applying each to headers.
// Per ADR-0109 + SPEC §6.6.
func applyOps(headers http.Header, ops []compiledMutationOp) {
    for _, op := range ops {
        switch op.kind {
        case kindRemove:
            headers.Del(op.headerName)
        case kindAppend:
            applyAppendAction(headers, op)
        }
    }
}

// applyAppendAction implements the AppendAction × 4 + keep_empty_value
// boundary per SPEC §6.6 + §11.2 + ADR-0109. The keep_empty_value=false
// silent-skip on empty value fires FIRST, BEFORE the AppendAction switch
// (per §11.2 conclusion (c)).
func applyAppendAction(headers http.Header, op compiledMutationOp) {
    if op.headerValue == "" && !op.keepEmptyValue {
        return // silent skip per §11.2
    }
    switch op.appendAction {
    case corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD:
        headers.Add(op.headerName, op.headerValue)
    case corev3.HeaderValueOption_ADD_IF_ABSENT:
        if headers.Get(op.headerName) == "" {
            headers.Add(op.headerName, op.headerValue)
        }
    case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD:
        headers.Set(op.headerName, op.headerValue)
    case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS:
        if headers.Get(op.headerName) != "" {
            headers.Set(op.headerName, op.headerValue)
        }
    }
}
```

- [ ] **Step 4: Run tests; confirm pass**

```bash
go test -race ./internal/filter/http/header_mutation/... -v -run TestApplyOps 2>&1 | tail -30
```

Expected: all TestApplyOps_* PASS.

- [ ] **Step 5: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./internal/filter/http/header_mutation/...
git add internal/filter/http/header_mutation/header_mutation.go internal/filter/http/header_mutation/header_mutation_test.go
git commit -m "phase 10: header_mutation applyOps + AppendAction × 4 + keep_empty_value boundary"
```

SHA-fill follow-up.

*Anchored: SPEC §6.6 + §11.2 + §11.4 + §14.1; ADR-0109 (already landed in Task 5; this task lands the apply-loop semantics codified by the ADR).*

---

## Task 7: `header_mutation.go` `DecodeHeaders` body + multi-tier resolution + flag-controlled ordering + `compileForRequest` helper + DecodeHeaders unit tests [ADR-0110, ADR-0073 amendment]

**Files:**
- Modify: `internal/filter/http/header_mutation/header_mutation.go` (add `compileForRequest` + replace stub `DecodeHeaders`)
- Modify: `internal/filter/http/header_mutation/header_mutation_test.go` (add DecodeHeaders tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0110; append amendment paragraph to ADR-0073)

This task lands the full `DecodeHeaders` body per SPEC §6.6: apply listener-level requestOps FIRST, resolve all 3 per-route tiers via `f.dcb.RequestRouteConfigsAllTiers()`, compile each non-nil tier via `compileForRequest`, apply tiers in flag-controlled order (false=Route→VHost→RC; true=RC→VHost→Route per §6.5 algorithm + §11.5 empirical confirmation), return Continue. ADR-0110 (the multi-tier per-route evaluation framework + per-filter accessor-choice discipline + cross-tier algorithm) lands here as the FIRST end-to-end use of the framework pieces from Tasks 2/3/4. ADR-0073 gains an in-place amendment paragraph noting "the most-specific-override discipline is now the DEFAULT model; filters that need multi-tier evaluation use ResolveAllTiers per ADR-0110."

**Precondition:** Task 6 done.
**Artifact:** modified header_mutation.go + extended header_mutation_test.go + DECISIONS.md (+ADR-0110 + ADR-0073 amendment).
**Acceptance:** all `TestDecodeHeaders_*` tests PASS; race detector clean; ADR-0110 + ADR-0073 amendment in DECISIONS.md.

- [ ] **Step 1: Write failing tests**

Append after the apply-loop tests in header_mutation_test.go:

```go
// fakeDecoderCB is a minimal test impl of DecoderFilterCallbacks supporting
// only RequestRouteConfigsAllTiers + RequestRouteConfig (returns nil) +
// the two no-op callbacks. The other methods panic if called.
type fakeDecoderCB struct {
    route, vhost, rc proto.Message
}

func (f *fakeDecoderCB) ContinueDecoding()                                {}
func (f *fakeDecoderCB) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (f *fakeDecoderCB) RequestRouteConfig() proto.Message                { return nil }
func (f *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
    return f.route, f.vhost, f.rc
}
func (f *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (f *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (f *fakeDecoderCB) EncodeTrailers(http.Header)      {}

func mkPerRoute(req, resp []*commonmutationrulesv3.HeaderMutation) *headermutationv3.HeaderMutationPerRoute {
    return &headermutationv3.HeaderMutationPerRoute{
        Mutations: &headermutationv3.Mutations{RequestMutations: req, ResponseMutations: resp},
    }
}

func mkFilterFromMutation(t *testing.T, mut *headermutationv3.HeaderMutation, dcb envoyhttp.DecoderFilterCallbacks) *filter {
    t.Helper()
    factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    inst := factory()
    f := inst.Decoder.(*filter)
    f.SetDecoderCallbacks(dcb)
    return f
}

func TestDecodeHeaders_ListenerLevel_NoPerRoute(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
                mkRemoveOp("user-agent"),
            },
        },
    }
    dcb := &fakeDecoderCB{} // no per-route configs
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    h.Set("user-agent", "curl/8.20")
    if status := f.DecodeHeaders(h, false); status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue", status)
    }
    if h.Get("X-Test") != "listener" {
        t.Errorf("x-test: got %q, want listener", h.Get("X-Test"))
    }
    if h.Get("User-Agent") != "" {
        t.Errorf("user-agent should be removed; got %q", h.Get("User-Agent"))
    }
}

func TestDecodeHeaders_PerRoute_RouteOnly(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
    }
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{
        mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
    }, nil)
    dcb := &fakeDecoderCB{route: routePR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    if got := h.Get("X-Test"); got != "route" {
        t.Errorf("got %q, want route (route applied after listener)", got)
    }
}

func TestDecodeHeaders_MultiTier_FlagFalse(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
        MostSpecificHeaderMutationsWins: false,
    }
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    // flag=false: Route → VHost → RC (RC applied LAST, wins overlap) per §11.5
    if got := h.Get("X-Test"); got != "rc" {
        t.Errorf("flag=false: got %q, want rc (least-specific wins per §11.5)", got)
    }
}

func TestDecodeHeaders_MultiTier_FlagTrue(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
        MostSpecificHeaderMutationsWins: true,
    }
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    // flag=true: RC → VHost → Route (Route applied LAST, wins overlap) per §11.5
    if got := h.Get("X-Test"); got != "route" {
        t.Errorf("flag=true: got %q, want route (most-specific wins per §11.5)", got)
    }
}

func TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndVHost(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    dcb := &fakeDecoderCB{route: routePR, vhost: vhPR} // rc nil
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    // flag=false: Route → VHost (RC nil — skipped); VHost wins
    if got := h.Get("X-Test"); got != "vh" {
        t.Errorf("got %q, want vh (route + vhost; VHost applied last)", got)
    }
}

func TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndRC(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    dcb := &fakeDecoderCB{route: routePR, rc: rcPR} // vhost nil
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    if got := h.Get("X-Test"); got != "rc" {
        t.Errorf("got %q, want rc (route + rc; RC applied last)", got)
    }
}

func TestDecodeHeaders_MultiTier_TwoOfThree_VHostAndRC(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{Mutations: &headermutationv3.Mutations{}, MostSpecificHeaderMutationsWins: false}
    vhPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "vh", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    rcPR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "rc", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    dcb := &fakeDecoderCB{vhost: vhPR, rc: rcPR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.DecodeHeaders(h, false)
    if got := h.Get("X-Test"); got != "rc" {
        t.Errorf("got %q, want rc (vh + rc; RC applied last)", got)
    }
}

func TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-test", "listener", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
        },
    }
    factory, _ := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    inst := factory()
    f := inst.Decoder.(*filter)
    // Do NOT SetDecoderCallbacks — exercise the dcb-nil branch.
    h := http.Header{}
    if status := f.DecodeHeaders(h, false); status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue", status)
    }
    if got := h.Get("X-Test"); got != "listener" {
        t.Errorf("listener-only path: got %q, want listener", got)
    }
}
```

- [ ] **Step 2: Run tests; confirm fail (DecodeHeaders is still the Task 5 stub returning Continue without applying anything)**

```bash
go test ./internal/filter/http/header_mutation/... -run TestDecodeHeaders -v 2>&1 | tail -30
```

- [ ] **Step 3: Implement `compileForRequest` + DecodeHeaders body in `internal/filter/http/header_mutation/header_mutation.go`**

Insert after `applyAppendAction`:

```go
// compileForRequest projects a per-route HeaderMutationPerRoute proto.Message
// into a request-mutations slice. Sub-microsecond on small per-route configs
// (typical: <5 ops per tier). Per planner-time decision 2 we re-compile fresh
// per request rather than caching; the cost is negligible.
//
// Returns nil for nil input or for messages that fail the type-assertion (the
// per-route validator at HCM-build time per ADR-0111 already rejects per-route
// configs containing protected-header mutations, so the compileOps call here
// is expected to succeed; defensive on error → return nil).
func (f *filter) compileForRequest(msg proto.Message) []compiledMutationOp {
    if msg == nil {
        return nil
    }
    pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
    if !ok {
        return nil
    }
    m := pr.GetMutations()
    if m == nil {
        return nil
    }
    ops, err := compileOps(m.GetRequestMutations())
    if err != nil {
        // Per-route validator at HCM-build time rejects this case; defensive return.
        return nil
    }
    return ops
}
```

Then replace the stub `DecodeHeaders` body:

```go
// DecodeHeaders implements the header_mutation filter's decode-side discipline
// per SPEC §6.6 + ADR-0109 + ADR-0110. Apply listener-level cfg.requestOps
// FIRST (per the proto comment at header_mutation.pb.go:141–142), then
// per-route tiers in flag-controlled order (per §6.5 algorithm + §11.5
// empirical confirmation).
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    applyOps(headers, f.cfg.requestOps)
    if f.dcb == nil {
        return envoyhttp.Continue
    }
    routeMsg, vhMsg, rcMsg := f.dcb.RequestRouteConfigsAllTiers()
    routeOps := f.compileForRequest(routeMsg)
    vhOps := f.compileForRequest(vhMsg)
    rcOps := f.compileForRequest(rcMsg)

    if !f.cfg.mostSpecificHeaderMutationsWins {
        // DEFAULT (flag=false): Route → VHost → RC; least-specific wins overlap
        applyOps(headers, routeOps)
        applyOps(headers, vhOps)
        applyOps(headers, rcOps)
    } else {
        // flag=true: RC → VHost → Route; most-specific wins overlap
        applyOps(headers, rcOps)
        applyOps(headers, vhOps)
        applyOps(headers, routeOps)
    }
    return envoyhttp.Continue
}
```

- [ ] **Step 4: Run tests; confirm pass**

```bash
go test -race ./internal/filter/http/header_mutation/... -v -run TestDecodeHeaders 2>&1 | tail -40
```

Expected: all TestDecodeHeaders_* PASS.

- [ ] **Step 5: Append ADR-0110 to DECISIONS.md**

Per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). Key anchors per the PLAN ADR table + SPEC §1 + §5.1 + §6.5 + §6.7 + §6.8 + §11.5:

- **Context:** the `most_specific_header_mutations_wins` proto field requires per-route configs at all 3 tiers be evaluated, not just most-specific (§11.5 empirical confirmation matches the proto comment verbatim). The existing `PerRouteConfig.Resolve` (per ADR-0073) returns most-specific-only and discards others.
- **Decision:** add framework method `PerRouteConfig.ResolveAllTiers(filterName, routeIdx) (route, vhost, rc proto.Message)` (sibling, not replacement); add callback method `DecoderFilterCallbacks.RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)` (DECODER-ONLY per planner-time decision 1 — used from BOTH decode AND encode bodies via `f.dcb`); add registry hook `HTTPRegistry.RegisterPerRouteValidator(filterName, validator)` consumed by `BuildPerRouteConfig` to surface per-route protected-header violations (per ADR-0111) as boot-time errors. Per-filter accessor-choice discipline: cors + fault continue to use `Resolve` (most-specific override per ADR-0073); header_mutation uses `ResolveAllTiers` (multi-tier per its proto semantics). The cross-tier ordering algorithm: listener-level mutations applied FIRST always; then per-route tiers in flag-controlled order (false=Route→VHost→RC; true=RC→VHost→Route).
- **Alternatives considered:** (A) keep using `Resolve` and pretend the flag flips between two single-tier behaviors — REJECTED (loses configuration fidelity); (B) generalize the framework merge function (per-filter merger interface) — REJECTED (300+ LoC framework refactor; forces re-verification of cors + fault per-route tests; out of scope); (C) push resolution into HCM-build time — REJECTED (per-route config is per-request; resolution must be at request time); (D) ADD `ResolveAllTiers` as proposed — ACCEPTED.
- **Consequences:** ADR-0073 amended (not superseded) — most-specific-override remains the DEFAULT model (used by cors + fault); filters whose proto semantics demand multi-tier evaluation use `ResolveAllTiers` per this ADR. The per-route-validator hook is reusable by future filters with similar boot-time validation invariants. The 3-tuple cache shape is incompatible with the existing single-Message cache; per-tuple caching is deferred per planner-time decision 2.

- [ ] **Step 6: Append the in-place amendment paragraph to ADR-0073**

Locate the existing ADR-0073 in DECISIONS.md (after the ## Decision / ## Consequences / ## Alternatives sections) and append:

```markdown
## Amendment (per phase 10 ADR-0110)

The most-specific-override discipline codified above is now the DEFAULT model;
filters whose proto semantics demand multi-tier evaluation (e.g.,
envoy.filters.http.header_mutation per its `most_specific_header_mutations_wins`
flag) use `PerRouteConfig.ResolveAllTiers` per ADR-0110 — see that ADR for the
per-filter accessor-choice discipline. ADR-0073's wholesale-override semantics
remain authoritative for filters that opt into the most-specific accessor
(cors @ 07.1, fault @ 09).
```

NO change to the original Decision body; the amendment is a forward-pointer.

- [ ] **Step 7: Commit**

```bash
go vet ./...
golangci-lint run ./...
git add internal/filter/http/header_mutation/ docs/envoy-go/DECISIONS.md
git commit -m "phase 10: header_mutation DecodeHeaders multi-tier + ADR-0110 + ADR-0073 amendment"
```

SHA-fill follow-up.

*Anchored: SPEC §6.5 + §6.6 + §6.7 + §11.5 + §14.1; ADR-0110; ADR-0073 amendment.*

---

## Task 8: `header_mutation.go` `EncodeHeaders` body symmetric + `compileForResponse` helper + EncodeHeaders unit tests + race-detector cycle test

**Files:**
- Modify: `internal/filter/http/header_mutation/header_mutation.go` (add `compileForResponse` + replace stub `EncodeHeaders`)
- Modify: `internal/filter/http/header_mutation/header_mutation_test.go` (add EncodeHeaders + race-detector tests)

This task lands the symmetric `EncodeHeaders` body per SPEC §6.8: same algorithm as `DecodeHeaders` modulo (a) reads `f.cfg.responseOps` instead of `requestOps`; (b) compiles `compileForResponse` (reads `mutations.response_mutations` not `request_mutations`); (c) uses the SAME `f.dcb.RequestRouteConfigsAllTiers()` callback (DECODER-ONLY per planner-time decision 1; the dcb is set during chain wiring regardless of decode vs encode firing). Plus the race-detector cycle test `TestHeaderMutation_MultiTierConcurrentRequests` per SPEC §12 deferred decision 7. NO new ADRs land in this task.

**Precondition:** Task 7 done.
**Artifact:** modified header_mutation.go + extended header_mutation_test.go.
**Acceptance:** all `TestEncodeHeaders_*` + `TestHeaderMutation_MultiTierConcurrentRequests` PASS; race detector clean; vet + lint clean.

- [ ] **Step 1: Write failing tests**

Append to header_mutation_test.go:

```go
func TestEncodeHeaders_Symmetric(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-resp", "listener-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
                mkAppendOp("x-multi", "APPENDED", corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD),
            },
        },
    }
    dcb := &fakeDecoderCB{}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    h.Add("X-Multi", "alpha")
    h.Add("X-Multi", "beta")
    if status := f.EncodeHeaders(h, false); status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue", status)
    }
    if got := h.Get("X-Resp"); got != "listener-resp" {
        t.Errorf("x-resp: got %q, want listener-resp", got)
    }
    if got := h.Values("X-Multi"); len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "APPENDED" {
        t.Errorf("x-multi APPEND should preserve + add (per §11.4); got %v", got)
    }
}

func TestEncodeHeaders_MultiTier_FlagFalse_ResponseSide(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            ResponseMutations: []*commonmutationrulesv3.HeaderMutation{
                mkAppendOp("x-resp", "listener-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD),
            },
        },
        MostSpecificHeaderMutationsWins: false,
    }
    routePR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "route-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    vhPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "vh-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    rcPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "rc-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.EncodeHeaders(h, false)
    if got := h.Get("X-Resp"); got != "rc-resp" {
        t.Errorf("flag=false response: got %q, want rc-resp", got)
    }
}

func TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide(t *testing.T) {
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{},
        MostSpecificHeaderMutationsWins: true,
    }
    routePR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "route-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    vhPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "vh-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    rcPR := mkPerRoute(nil, []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "rc-resp", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)})
    dcb := &fakeDecoderCB{route: routePR, vhost: vhPR, rc: rcPR}
    f := mkFilterFromMutation(t, mut, dcb)
    h := http.Header{}
    f.EncodeHeaders(h, false)
    if got := h.Get("X-Resp"); got != "route-resp" {
        t.Errorf("flag=true response: got %q, want route-resp", got)
    }
}

func TestHeaderMutation_MultiTierConcurrentRequests(t *testing.T) {
    // Race-detector cycle test per SPEC §12 deferred decision 7. Spawn many
    // goroutines that each construct a fresh *filter from the SAME factory
    // (sharing the closure-captured *runtimeConfig) and call DecodeHeaders +
    // EncodeHeaders concurrently. The framework's per-instance discipline +
    // *runtimeConfig read-only-after-New invariant make this safe by
    // construction; the race detector validates.
    mut := &headermutationv3.HeaderMutation{
        Mutations: &headermutationv3.Mutations{
            RequestMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-req", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
            ResponseMutations: []*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-resp", "v", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)},
        },
    }
    factory, err := New(mustAny(t, mut), envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    routePR := mkPerRoute([]*commonmutationrulesv3.HeaderMutation{mkAppendOp("x-tier", "route", corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD)}, nil)
    var wg sync.WaitGroup
    for i := 0; i < 64; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            inst := factory()
            f := inst.Decoder.(*filter)
            f.SetDecoderCallbacks(&fakeDecoderCB{route: routePR})
            h := http.Header{}
            f.DecodeHeaders(h, false)
            f.EncodeHeaders(h, false)
        }()
    }
    wg.Wait()
}
```

NOTE: requires `import "sync"` in header_mutation_test.go.

- [ ] **Step 2: Run failing tests; confirm fail**

```bash
go test ./internal/filter/http/header_mutation/... -run 'TestEncodeHeaders|TestHeaderMutation_MultiTier' 2>&1 | head -10
```

- [ ] **Step 3: Implement `compileForResponse` + EncodeHeaders body**

Insert after `compileForRequest`:

```go
// compileForResponse projects a per-route HeaderMutationPerRoute proto.Message
// into a response-mutations slice. Symmetric to compileForRequest.
func (f *filter) compileForResponse(msg proto.Message) []compiledMutationOp {
    if msg == nil {
        return nil
    }
    pr, ok := msg.(*headermutationv3.HeaderMutationPerRoute)
    if !ok {
        return nil
    }
    m := pr.GetMutations()
    if m == nil {
        return nil
    }
    ops, err := compileOps(m.GetResponseMutations())
    if err != nil {
        return nil
    }
    return ops
}
```

Replace the stub `EncodeHeaders` body:

```go
// EncodeHeaders implements the header_mutation filter's encode-side discipline
// per SPEC §6.8 — symmetric to DecodeHeaders modulo (a) reads cfg.responseOps;
// (b) compiles response-side mutations via compileForResponse; (c) uses the
// SAME f.dcb.RequestRouteConfigsAllTiers callback (DECODER-ONLY per planner-
// time decision 1; the dcb is set during chain wiring regardless of decode
// vs encode firing — mirrors cors precedent at cors.go:163).
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    applyOps(headers, f.cfg.responseOps)
    if f.dcb == nil {
        return envoyhttp.Continue
    }
    routeMsg, vhMsg, rcMsg := f.dcb.RequestRouteConfigsAllTiers()
    routeOps := f.compileForResponse(routeMsg)
    vhOps := f.compileForResponse(vhMsg)
    rcOps := f.compileForResponse(rcMsg)

    if !f.cfg.mostSpecificHeaderMutationsWins {
        applyOps(headers, routeOps)
        applyOps(headers, vhOps)
        applyOps(headers, rcOps)
    } else {
        applyOps(headers, rcOps)
        applyOps(headers, vhOps)
        applyOps(headers, routeOps)
    }
    return envoyhttp.Continue
}
```

- [ ] **Step 4: Run tests; confirm pass under `-race`**

```bash
go test -race ./internal/filter/http/header_mutation/... -v 2>&1 | tail -40
```

Expected: every test PASS, including the race-detector concurrent test.

- [ ] **Step 5: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add internal/filter/http/header_mutation/
git commit -m "phase 10: header_mutation EncodeHeaders symmetric + race-detector cycle test"
```

SHA-fill follow-up.

*Anchored: SPEC §6.8 + §14.1 + §12 deferred decision 7; ADR-0109 + ADR-0110 (already landed).*

---

## Task 9: `cmd/envoy-go/main.go` register `header_mutation.New` under `header_mutation.TypeURL`

**Files:**
- Modify: `cmd/envoy-go/main.go`

This task adds the boot-time registration line for header_mutation per ADR-0072 + ADR-0108. ONE new `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` line inserted after the existing `fault.New` registration (line 115) and before `httpReg.Freeze()` (line 116). Plus the matching import alphabetically among the existing filter-package imports.

**Precondition:** Task 8 done; `internal/filter/http/header_mutation/` package compiles cleanly.
**Artifact:** modified main.go.
**Acceptance:** `go build ./cmd/envoy-go` clean; `go vet ./...` clean; the registration appears in the expected order (router → cors → envoygotest → fault → header_mutation → Freeze).

- [ ] **Step 1: Read the existing registration block at `cmd/envoy-go/main.go:111–116`**

```bash
sed -n '111,117p' cmd/envoy-go/main.go
```

Confirm the current shape matches the SPEC §4.2 expectation:

```
httpReg := filter_http.NewHTTPRegistry()
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(cors.TypeURL, cors.New)
httpReg.Register(envoygotest.TypeURL, envoygotest.New)
httpReg.Register(fault.TypeURL, fault.New)
httpReg.Freeze()
```

- [ ] **Step 2: Add the import line**

Insert after the existing `fault` import:

```go
"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
```

Per the existing alphabetical ordering, the imports become: cors, envoygotest, fault, header_mutation, router.

- [ ] **Step 3: Add the registration line**

Insert between `httpReg.Register(fault.TypeURL, fault.New)` and `httpReg.Freeze()`:

```go
httpReg.Register(header_mutation.TypeURL, header_mutation.New)
```

- [ ] **Step 4: Verify build + vet + run unit tests**

```bash
go build ./cmd/envoy-go
go vet ./...
go test -race -count=1 ./...
```

All clean.

- [ ] **Step 5: Commit**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 10: register header_mutation.New under header_mutation.TypeURL"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2; ADR-0072 + ADR-0108.*

---

## Task 10: `internal/filter/http/header_mutation/fuzz_test.go` `FuzzHeaderMutationConfigParse`

**Files:**
- Create: `internal/filter/http/header_mutation/fuzz_test.go`

This task lands the thirteenth fuzzer per SPEC §14.3 + planner-time decision 6. Fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`; asserts `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. 30s budget per ADR-0018; ~50 LoC.

**Precondition:** Task 9 done.
**Artifact:** new fuzz_test.go.
**Acceptance:** `go test -fuzz=FuzzHeaderMutationConfigParse -fuzztime=30s ./internal/filter/http/header_mutation/...` runs clean; no crashes; corpus seeded from a few minimal valid + invalid Any blobs.

- [ ] **Step 1: Create `internal/filter/http/header_mutation/fuzz_test.go`**

```go
package header_mutation

import (
    "testing"

    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// FuzzHeaderMutationConfigParse fuzzes arbitrary byte sequences as the tc
// *anypb.Any parameter to New. Asserts New returns either (factory, nil) OR
// (nil, error); never panics; never returns (nil, nil).
//
// Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the
// header_mutation filter's New factory is a parser. 30s budget per ADR-0018
// short-mode CI policy. Thirteenth fuzzer overall (post-09's twelfth
// FuzzFaultConfigParse).
func FuzzHeaderMutationConfigParse(f *testing.F) {
    // Seed corpus: empty TypeURL + empty bytes (invalid).
    f.Add("", []byte{})
    // Seed corpus: arbitrary bytes under the canonical type URL (decode error).
    f.Add(TypeURL, []byte{0xff, 0xff, 0xff})
    // Seed corpus: short proto-wire-format bytes.
    f.Add(TypeURL, []byte{0x08, 0x01})

    f.Fuzz(func(t *testing.T, typeURL string, value []byte) {
        tc := &anypb.Any{TypeUrl: typeURL, Value: value}
        factory, err := New(tc, envoyhttp.FactoryCtx{Registry: envoyhttp.NewHTTPRegistry()})
        switch {
        case factory == nil && err == nil:
            t.Errorf("New returned (nil, nil); type=%q", typeURL)
        case factory != nil && err != nil:
            t.Errorf("New returned (factory, err) — should be (factory, nil) or (nil, err); type=%q err=%v", typeURL, err)
        }
    })
}
```

- [ ] **Step 2: Run the fuzzer at the 30s budget**

```bash
go test -fuzz=FuzzHeaderMutationConfigParse -fuzztime=30s ./internal/filter/http/header_mutation/... 2>&1 | tail -10
```

Expected: clean exit (no crashes / panics); the fuzzer reports an execution count and exits.

- [ ] **Step 3: Run the standard short test (the seeded inputs run as a regular test)**

```bash
go test -count=1 -run FuzzHeaderMutationConfigParse ./internal/filter/http/header_mutation/...
```

Expected: PASS.

- [ ] **Step 4: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add internal/filter/http/header_mutation/fuzz_test.go
git commit -m "phase 10: FuzzHeaderMutationConfigParse (thirteenth fuzzer per ADR-0018)"
```

SHA-fill follow-up.

*Anchored: SPEC §14.3; ADR-0018; planner-time decision 6.*

---

## Task 11: Fixture infrastructure — `BackendKind` enum extension + `runner_test.go` spawn helper + blank-import [planner-time decisions 10, 11]

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `HTTPHeaderMutation BackendKind = 9`)
- Modify: `test/differential/runner_test.go` (add blank-import + `startHTTPHeaderMutationBackend` spawn helper + switch case)

This task lands the fixture-harness infrastructure: a new `BackendKind` enum value `HTTPHeaderMutation BackendKind = 9` per planner-time decision 11 + the runner's spawn helper for the new backend per planner-time decision 10 (fixture path corrected to `test/fixtures/0012-http-header-mutation/` from SPEC §4.3's `test/differential/0012-http-header-mutation/` erratum). The blank-import for the fixture driver lands here; the actual driver file lands in Task 15.

**Precondition:** Task 10 done.
**Artifact:** modified fixture.go + runner_test.go.
**Acceptance:** `go build ./test/differential/...` clean; `go vet ./...` clean. The fixture is registered but not yet runnable (Tasks 12–15 land the backend + bootstrap + driver files).

- [ ] **Step 1: Add the `HTTPHeaderMutation` enum value to `test/differential/fixture/fixture.go`**

Locate the existing `HTTPFault BackendKind = 8` line and append:

```go
// HTTPHeaderMutation is an out-of-process HTTP/1.1 backend: the runner spawns
// test/fixtures/0012-http-header-mutation/backends/backend.go on the pre-
// allocated port. The backend serves / reflecting received request headers
// into the response body (one header per line, sorted for determinism since
// Go map iteration is non-deterministic) plus a single-value
// X-Resp-Test: backend-original and multi-value X-Multi: alpha, beta response
// headers (for OVERWRITE / APPEND multi-value testing per phase 10 SPEC §11.4).
// No TLS. Introduced by fixture 0012-http-header-mutation (phase 10 Task 11).
// Because the backend is a subprocess, the runner's in-process accept counter
// is NOT incremented.
HTTPHeaderMutation BackendKind = 9
```

- [ ] **Step 2: Add the spawn helper to `test/differential/runner_test.go`**

Locate the existing `startHTTPFaultBackend` helper (introduced by phase 09) and add a sibling helper:

```go
// startHTTPHeaderMutationBackend spawns test/fixtures/0012-http-header-mutation/
// backends/backend.go on the runner-allocated port. Mirrors startHTTPFaultBackend.
func startHTTPHeaderMutationBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
    cmd := exec.CommandContext(ctx, "go", "run",
        "./test/fixtures/0012-http-header-mutation/backends",
        "--port", fmt.Sprintf("%d", port))
    cmd.Dir = repoRoot
    cmd.Stdout = os.Stderr
    cmd.Stderr = os.Stderr
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    if err := cmd.Start(); err != nil {
        return nil, err
    }
    return cmd, nil
}
```

- [ ] **Step 3: Extend the `kind` switch in `runFixture` with the new case**

Locate the existing `case fixture.HTTPFault:` block in `runFixture` and add an analogous block for `HTTPHeaderMutation` directly after:

```go
case fixture.HTTPHeaderMutation:
    cmd, err := startHTTPHeaderMutationBackend(ctx, repoRoot, port)
    if err != nil {
        t.Fatalf("startHTTPHeaderMutationBackend: %v", err)
    }
    backendCmds = append(backendCmds, cmd)
```

(The exact shape — error handling, cleanup append slice — must match the pre-existing HTTPFault case verbatim modulo function name.)

- [ ] **Step 4: Add the blank-import for the fixture driver**

Append to the blank-import block (after the `0011-http-fault` blank-import):

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver"
```

NOTE: at this task the driver package does NOT yet exist; this import will fail to compile until Task 15. The implementer's choice: (a) defer adding the blank-import until Task 15; (b) add a stub `test/fixtures/0012-http-header-mutation/driver/doc.go` with `package driver` so the import resolves. Recommendation: (b) — add a stub `doc.go` here; the full driver lands in Task 15:

```bash
mkdir -p test/fixtures/0012-http-header-mutation/driver
cat > test/fixtures/0012-http-header-mutation/driver/doc.go <<'EOF'
// Package driver implements the differential-fixture driver for fixture
// 0012-http-header-mutation. The full driver is filled in at phase 10 Task 15;
// this stub exists so the runner_test.go blank-import resolves at Task 11.
package driver
EOF
```

- [ ] **Step 5: Verify build + lint**

```bash
go build ./test/differential/... ./test/fixtures/0012-http-header-mutation/...
go vet ./...
golangci-lint run ./...
```

Expected: clean (the stub driver compiles; the runner switch resolves).

- [ ] **Step 6: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go test/fixtures/0012-http-header-mutation/driver/doc.go
git commit -m "phase 10: fixture infrastructure — HTTPHeaderMutation BackendKind + spawn helper + driver stub"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3 (corrected per planner-time decision 10); planner-time decision 11.*

---

## Task 12: Fixture 0012 — `backends/backend.go` (Go HTTP backend serving header echo + multi-value response headers)

**Files:**
- Create: `test/fixtures/0012-http-header-mutation/backends/backend.go`

This task lands the minimal Go HTTP backend per SPEC §7.5: bound to a runner-allocated port; `/` endpoint serves a fast `200 OK` with body listing every received request header (one per line: `"Name: value\n"`, sorted for determinism); response carries one single-value header (`X-Resp-Test: backend-original`) and one multi-value header (`X-Multi: alpha`, `X-Multi: beta`) for OVERWRITE / APPEND multi-value testing per §11.4. ~50 LoC. NO new ADRs.

**Precondition:** Task 11 done.
**Artifact:** new backend.go.
**Acceptance:** `go build ./test/fixtures/0012-http-header-mutation/backends/...` clean; manual `curl` against the spawned backend returns the expected body + headers.

- [ ] **Step 1: Create `test/fixtures/0012-http-header-mutation/backends/backend.go`**

```go
// Package main implements the fixture 0012-http-header-mutation echo backend.
// Bound to a runner-allocated port via --port flag. The / endpoint reflects
// received request headers into the response body (one header per line,
// sorted for determinism) and emits a single-value X-Resp-Test header and a
// multi-value X-Multi header for OVERWRITE / APPEND multi-value testing per
// phase 10 SPEC §7.5 + §11.4.
package main

import (
    "flag"
    "fmt"
    "net/http"
    "sort"
    "strings"
)

func main() {
    port := flag.Int("port", 18012, "listen port")
    flag.Parse()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Reflect received request headers into the response body, sorted
        // for determinism (Go map iteration is non-deterministic; sort is
        // required so reference vs envoy-go body bytes compare equal).
        names := make([]string, 0, len(r.Header))
        for n := range r.Header {
            names = append(names, n)
        }
        sort.Strings(names)
        var b strings.Builder
        for _, n := range names {
            for _, v := range r.Header[n] {
                fmt.Fprintf(&b, "%s: %s\n", n, v)
            }
        }
        body := b.String()
        // Single-value response header for OVERWRITE_IF_EXISTS variants.
        w.Header().Set("X-Resp-Test", "backend-original")
        // Multi-value response header for APPEND/OVERWRITE multi-value testing per §11.4.
        w.Header().Add("X-Multi", "alpha")
        w.Header().Add("X-Multi", "beta")
        w.Header().Set("Content-Type", "text/plain")
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(body))
    })
    addr := fmt.Sprintf(":%d", *port)
    if err := http.ListenAndServe(addr, nil); err != nil {
        panic(err)
    }
}
```

- [ ] **Step 2: Verify build + a manual smoke test**

```bash
go build ./test/fixtures/0012-http-header-mutation/backends/
# Spawn briefly and curl:
go run ./test/fixtures/0012-http-header-mutation/backends/ --port 18099 &
PID=$!
sleep 0.3
curl -isS http://127.0.0.1:18099/ -H 'X-Probe: yes' | head -20
kill $PID
```

Expected: `200 OK`; response body lists the received request headers sorted; response carries `X-Resp-Test: backend-original`, `X-Multi: alpha`, `X-Multi: beta`.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0012-http-header-mutation/backends/backend.go
git commit -m "phase 10: fixture 0012 backend — header echo + multi-value response headers"
```

SHA-fill follow-up.

*Anchored: SPEC §7.5 + §11.4.*

---

## Task 13: Fixture 0012 — `envoy.yaml` + `envoy-go.yaml` bootstraps per SPEC §7.4

**Files:**
- Create: `test/fixtures/0012-http-header-mutation/envoy.yaml`
- Create: `test/fixtures/0012-http-header-mutation/envoy-go.yaml`

This task lands the dual-listener bootstrap per SPEC §7.4: TWO listeners (`l_lws` flag=false on :10012 + `l_mws` flag=true on :10013 for reference; :10011 + :10012 for subject) sharing IDENTICAL per-route tier configurations; only the listener-level `most_specific_header_mutations_wins` flag differs. Cluster `c_backend` STRICT_DNS pointing at the harness backend. Per SPEC §7.4 the YAML body summarises `l_mws.route_config` with `... (route_config body identical to rc_lws above) ...`; the actual fixture file MUST contain the FULL expansion (no `...` placeholder). NO new ADRs.

**Precondition:** Task 12 done.
**Artifact:** envoy.yaml + envoy-go.yaml; both syntactically valid Envoy bootstrap configs.
**Acceptance:** `docker run --rm -v "$(pwd)/test/fixtures/0012-http-header-mutation:/etc/envoy" envoyproxy/envoy:v1.37.2 envoy -c /etc/envoy/envoy.yaml --mode validate` returns success.

- [ ] **Step 1: Create `test/fixtures/0012-http-header-mutation/envoy.yaml`**

Use the SPEC §7.4 verbatim template (lines 604–760 of SPEC.md), expanding the `... (route_config body identical to rc_lws above) ...` placeholder in the `l_mws` listener with the full route_config body (identical to `l_lws`'s route_config). The implementer copies `l_lws.route_config` verbatim into `l_mws.route_config`, changing only the `name: rc_lws` → `name: rc_mws` line for clarity. The full file is approximately:

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9912 }
static_resources:
  listeners:
    - name: l_lws
      address:
        socket_address: { address: 0.0.0.0, port_value: 10012 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http_lws
                route_config:
                  name: rc_lws
                  typed_per_filter_config:
                    envoy.filters.http.header_mutation:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                      mutations:
                        request_mutations:
                          - append:
                              header: { key: "x-test", value: "rc" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                        response_mutations:
                          - append:
                              header: { key: "x-resp-test", value: "rc-resp" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      typed_per_filter_config:
                        envoy.filters.http.header_mutation:
                          "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                          mutations:
                            request_mutations:
                              - append:
                                  header: { key: "x-test", value: "vh" }
                                  append_action: OVERWRITE_IF_EXISTS_OR_ADD
                            response_mutations:
                              - append:
                                  header: { key: "x-resp-test", value: "vh-resp" }
                                  append_action: OVERWRITE_IF_EXISTS_OR_ADD
                      routes:
                        - match: { prefix: "/listener-only" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/route-override" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.header_mutation:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                              mutations:
                                request_mutations:
                                  - append:
                                      header: { key: "x-route-only", value: "yes" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                        - match: { prefix: "/multi-tier" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.header_mutation:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                              mutations:
                                request_mutations:
                                  - append:
                                      header: { key: "x-test", value: "route" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                                response_mutations:
                                  - append:
                                      header: { key: "x-resp-test", value: "route-resp" }
                                      append_action: OVERWRITE_IF_EXISTS_OR_ADD
                http_filters:
                  - name: envoy.filters.http.header_mutation
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation
                      mutations:
                        request_mutations:
                          # Listener-level: exercise all 4 AppendActions + Remove + keep_empty_value
                          - append:
                              header: { key: "x-test", value: "listener" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-listener-add", value: "added" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-add-if-absent-test", value: "added-if-absent" }
                              append_action: ADD_IF_ABSENT
                          - append:
                              header: { key: "x-overwrite-if-exists-test", value: "" }
                              append_action: OVERWRITE_IF_EXISTS  # absent target → no-op
                          - append:
                              header: { key: "x-empty-skip", value: "" }
                              append_action: APPEND_IF_EXISTS_OR_ADD  # keep_empty_value=false default → skip
                          - append:
                              header: { key: "x-empty-keep", value: "" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                              keep_empty_value: true                  # materialize empty value
                          - remove: "user-agent"                       # demonstrate Remove
                        response_mutations:
                          - append:
                              header: { key: "x-resp-test", value: "listener-resp" }
                              append_action: OVERWRITE_IF_EXISTS_OR_ADD
                          - append:
                              header: { key: "x-resp-multi", value: "appended" }
                              append_action: APPEND_IF_EXISTS_OR_ADD
                      most_specific_header_mutations_wins: false
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
    - name: l_mws
      address:
        socket_address: { address: 0.0.0.0, port_value: 10013 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http_mws
                route_config:
                  name: rc_mws
                  # IDENTICAL route_config body to l_lws above (per SPEC §7.4).
                  # Only the listener-level most_specific_header_mutations_wins flag differs.
                  typed_per_filter_config:
                    envoy.filters.http.header_mutation:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                      mutations:
                        request_mutations:
                          - append: { header: { key: "x-test", value: "rc" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                        response_mutations:
                          - append: { header: { key: "x-resp-test", value: "rc-resp" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      typed_per_filter_config:
                        envoy.filters.http.header_mutation:
                          "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                          mutations:
                            request_mutations:
                              - append: { header: { key: "x-test", value: "vh" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                            response_mutations:
                              - append: { header: { key: "x-resp-test", value: "vh-resp" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                      routes:
                        - match: { prefix: "/multi-tier" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.header_mutation:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutationPerRoute
                              mutations:
                                request_mutations:
                                  - append: { header: { key: "x-test", value: "route" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                                response_mutations:
                                  - append: { header: { key: "x-resp-test", value: "route-resp" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                http_filters:
                  - name: envoy.filters.http.header_mutation
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.header_mutation.v3.HeaderMutation
                      mutations:
                        request_mutations:
                          - append: { header: { key: "x-test", value: "listener" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                        response_mutations:
                          - append: { header: { key: "x-resp-test", value: "listener-resp" }, append_action: OVERWRITE_IF_EXISTS_OR_ADD }
                      most_specific_header_mutations_wins: true
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_backend
      type: STRICT_DNS
      connect_timeout: 0.25s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: host.docker.internal, port_value: 18012 }
```

NOTE: the implementer settles whether `host.docker.internal:18012` is the right address — the runner's existing 0011-http-fault precedent uses `host.docker.internal` per ADR-0010 + the harness host-networking discipline. If the existing 0011 fixture uses a different address (e.g., a templated `{{.BackendHost}}`), follow that convention. The `port_value: 18012` is fixed per SPEC §7.5; the driver's `ReferenceBootstrap` may template the port if the runner allocates dynamically.

- [ ] **Step 2: Create `test/fixtures/0012-http-header-mutation/envoy-go.yaml`**

Identical to envoy.yaml modulo admin/listener port values:
- admin: `port_value: 9911`
- l_lws listener port: `10011`
- l_mws listener port: `10012`

The `c_backend` cluster's address may also be templated for the subject side (the runner may pass `127.0.0.1` or a different hostname for envoy-go since it runs as a host-side process, not a container). Mirror the 0011-http-fault subject-side bootstrap precedent at `test/fixtures/0011-http-fault/envoy-go.yaml`.

- [ ] **Step 3: Validate both bootstraps via reference Envoy**

```bash
docker run --rm -v "$(pwd)/test/fixtures/0012-http-header-mutation:/etc/envoy" envoyproxy/envoy:v1.37.2 envoy -c /etc/envoy/envoy.yaml --mode validate 2>&1 | tail -10
```

Expected: success message; no "header_mutation: ... is :-prefixed or host" errors (the fixture mutates only non-protected headers per §11.1).

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0012-http-header-mutation/envoy.yaml test/fixtures/0012-http-header-mutation/envoy-go.yaml
git commit -m "phase 10: fixture 0012 envoy.yaml + envoy-go.yaml dual-listener bootstrap per SPEC §7.4"
```

SHA-fill follow-up.

*Anchored: SPEC §7.4; ADR-0010 (host networking); ADR-0072 (HTTPRegistry).*

---

## Task 14: Fixture 0012 — `expectations.yaml` + `README.md`

**Files:**
- Create: `test/fixtures/0012-http-header-mutation/expectations.yaml`
- Create: `test/fixtures/0012-http-header-mutation/README.md`

This task lands the prose narrative + per-scenario equivalence-claim documentation per SPEC §7.1. The expectations.yaml is prose per ADR-0019 (NOT machine-evaluated; the runner enforces via the driver's per-scenario assertions). NO new ADRs.

**Precondition:** Task 13 done.
**Artifact:** expectations.yaml + README.md.
**Acceptance:** files present; content cross-references SPEC §7.1; no schema validation needed.

- [ ] **Step 1: Create `test/fixtures/0012-http-header-mutation/expectations.yaml`**

```yaml
# Phase 10 SPEC §7.1 — per-scenario equivalence claims for fixture 0012-http-header-mutation.
#
# This file is PROSE per ADR-0019 (not machine-evaluated). The runner enforces
# via the driver's per-scenario assertions. Cross-refs SPEC §7.1 + §13.1 + ADR-0109 + ADR-0110 + ADR-0111.

scenario_1_listener_only:
  probe: GET /listener-only/anything against l_lws (flag=false)
  expects:
    status: 200
    body: byte-equal — echo backend reflects post-mutation request headers (sorted)
    response_headers:
      - x-resp-test: listener-resp        # OVERWRITE listener-level
      - x-resp-multi: appended            # APPEND adds one more (single-value initially)
      # Plus standard backend headers + any allow-listed framework injections (date, server)
    notes:
      - listener-level request_mutations exercise all 4 AppendActions + Remove + keep_empty_value boundary
      - x-test: listener (OVERWRITE)
      - x-listener-add: added (APPEND)
      - x-add-if-absent-test: added-if-absent (ADD_IF_ABSENT, target absent)
      - x-overwrite-if-exists-test: NOT PRESENT (OVERWRITE_IF_EXISTS, target absent + empty value → no-op via keep_empty_value=false silent skip)
      - x-empty-skip: NOT PRESENT (APPEND, empty value, keep_empty_value=false default → skip)
      - x-empty-keep: '' (APPEND, empty value, keep_empty_value=true → materialize)
      - User-Agent: REMOVED

scenario_2_route_override:
  probe: GET /route-override/anything against l_lws (flag=false)
  expects:
    status: 200
    body: byte-equal — backend echoes post-mutation headers (listener applied first, then Route tier)
    notes:
      - listener applies all the scenario_1 mutations FIRST
      - Route tier adds: x-route-only: yes (via OVERWRITE)
      - VHost tier sets x-test: vh (overwrites listener's x-test: listener)
      - Final upstream sees x-route-only AND the listener-applied set

scenario_3_multi_tier_flag_false:
  probe: GET /multi-tier/anything against l_lws (flag=false)
  expects:
    status: 200
    body: byte-equal — backend echoes post-mutation headers
    notes:
      - per §11.5: flag=false → Route → VHost → RC; least-specific (RC) wins overlap
      - final upstream x-test: rc (OVERWRITE chain: listener=listener → route=route → vh=vh → rc=rc)
      - final response x-resp-test: rc-resp (symmetric on response side)

scenario_4_multi_tier_flag_true:
  probe: GET /multi-tier/anything against l_mws (flag=true)
  expects:
    status: 200
    body: byte-equal — backend echoes post-mutation headers
    notes:
      - per §11.5: flag=true → RC → VHost → Route; most-specific (Route) wins overlap
      - final upstream x-test: route
      - final response x-resp-test: route-resp

# Stat assertions: NONE (header_mutation emits ZERO stats per SPEC §11.3 + ADR-0074 cors precedent).
# Timing assertions: NONE (synchronous filter; no time-bounded operations).
# Status text byte-equality: only stdlib codes (200) — phase 10 doesn't exercise non-stdlib codes.
```

- [ ] **Step 2: Create `test/fixtures/0012-http-header-mutation/README.md`**

```markdown
# Fixture 0012 — `envoy.filters.http.header_mutation`

Phase 10 differential fixture exercising envoy-go's `envoy.filters.http.header_mutation`
filter against reference Envoy v1.37.2 across four scenarios per SPEC §7.1:

1. **Listener-only mutations** (`/listener-only/anything` → `l_lws:10012`): all
   4 AppendActions + Remove + `keep_empty_value` boundary on the listener-level
   filter config; no per-route config on the matched route.
2. **Per-route override + listener interaction** (`/route-override/anything` → `l_lws:10012`):
   listener applied first, then Route tier adds an additional mutation.
3. **Multi-tier evaluation, flag=false, least-specific wins** (`/multi-tier/anything` → `l_lws:10012`):
   per-route configs at all 3 tiers (RC + VirtualHost + Route) all OVERWRITE the
   same header `x-test`; least-specific (RouteConfiguration) wins per SPEC §11.5.
4. **Multi-tier evaluation, flag=true, most-specific wins** (`/multi-tier/anything` → `l_mws:10013`):
   SAME per-route configs as scenario 3, but listener-level
   `most_specific_header_mutations_wins=true`; most-specific (Route) wins.

## Bootstrap shape

Each proxy boots TWO listeners with IDENTICAL per-route tier configurations;
only the listener-level `most_specific_header_mutations_wins` flag differs:

- `l_lws` (LWS = least-specific wins; flag=false): port 10012 (ref) / 10011 (subj)
- `l_mws` (MWS = most-specific wins; flag=true): port 10013 (ref) / 10012 (subj)

The dual-listener pattern is the project's preferred shape for testing
flag-controlled cross-tier ordering (TWO listeners with identical per-route tiers
and the flag as the distinguishing variable).

## Backend

Single Go HTTP backend (`backends/backend.go`) on port 18012 (ref-side) /
runner-allocated (subj-side). Reflects received request headers into the
response body (sorted for determinism); emits `X-Resp-Test: backend-original`
single-value + `X-Multi: alpha, beta` multi-value response headers for
OVERWRITE / APPEND multi-value testing per SPEC §11.4.

## What this fixture does NOT test

- **Stats:** header_mutation emits ZERO stats per SPEC §11.3; the driver does
  NOT scrape stats endpoints or assert stat deltas.
- **Timing:** synchronous filter; no time-bounded assertions.
- **Protected-header rejection:** CONFIG-LOAD-TIME per ADR-0111 — covered by
  unit tests at `internal/filter/http/header_mutation/header_mutation_test.go`,
  NOT by differential fixture (a config attempting protected-header mutation
  would refuse to boot — both reference + subject would refuse).
- **H2 differential:** fixture is HTTP/1.1-only per SPEC §2.3.
- **Cross-filter interaction** (header_mutation × cors / × fault): fixture is
  header_mutation + router only.
- **`mutations.query_parameter_mutations`** (deferred per ADR-0112).
- **Header-value formatter substitution syntax** (deferred per ADR-0113).

## Planner-time decision cross-references

- Fixture path corrected to `test/fixtures/0012-http-header-mutation/` (NOT
  `test/differential/0012-http-header-mutation/` per SPEC §4.3 erratum) per
  PLAN planner-time decision 10.
- New BackendKind enum value `HTTPHeaderMutation BackendKind = 9` per PLAN
  planner-time decision 11.

## Cross-references

- SPEC: `docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md` §7
- BEHAVIOR_CONTRACT: `docs/envoy-go/BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation`
- ADRs: ADR-0108 (package), ADR-0109 (parser + apply-loop), ADR-0110 (multi-tier framework), ADR-0111 (protected headers)
```

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0012-http-header-mutation/expectations.yaml test/fixtures/0012-http-header-mutation/README.md
git commit -m "phase 10: fixture 0012 expectations.yaml + README.md per SPEC §7.1"
```

SHA-fill follow-up.

*Anchored: SPEC §7.1 + §7.2; ADR-0019 (expectations as prose).*

---

## Task 15: Fixture 0012 — `driver/driver.go` (4-scenario orchestration; replaces stub `doc.go`)

**Files:**
- Create / Modify: `test/fixtures/0012-http-header-mutation/driver/driver.go` (replaces the Task 11 stub `doc.go`)
- Possibly delete: `test/fixtures/0012-http-header-mutation/driver/doc.go` (the stub)

This task lands the full driver implementing the §7.3 four-scenario orchestration. Mirrors 0011-http-fault per planner-time decision 9: `package driver`; `init()` calls `fixture.RegisterFixture`; implements the `fixture.Driver` interface; issues 4 HTTP requests per proxy (one per scenario) sequenced; captures per-probe status + body + headers; returns a deterministic byte stream for `CompareBytes`. NO stat scraping (zero stats per §11.3). NO timing assertions. NO new ADRs.

**Precondition:** Task 14 done.
**Artifact:** driver.go (+ removed doc.go stub).
**Acceptance:** `go build ./test/fixtures/0012-http-header-mutation/driver/...` clean; `go test -count=1 ./test/differential/ -run Test.*0012 -v` runs the fixture and reports differential-equivalence PASS for all 4 scenarios; vet + lint clean.

- [ ] **Step 1: Replace the Task 11 stub `doc.go` with the full driver `driver.go`**

```bash
rm test/fixtures/0012-http-header-mutation/driver/doc.go
```

Then create `test/fixtures/0012-http-header-mutation/driver/driver.go`. The structure mirrors `test/fixtures/0011-http-fault/driver/driver.go`. Concretely:

```go
// Package driver implements the differential-fixture driver for fixture
// 0012-http-header-mutation. Issues the 4 scenario probes per phase 10 SPEC
// §7.1 against both reference Envoy v1.37.2 and envoy-go; returns a
// deterministic per-probe byte stream for CompareBytes equivalence.
package driver

import (
    "bytes"
    "context"
    "fmt"
    "net/http"
    "sort"
    "strings"
    "text/template"
    "time"

    "github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() {
    fixture.RegisterFixture("0012-http-header-mutation", &headerMutationDriver{})
}

type headerMutationDriver struct{}

func (d *headerMutationDriver) BackendCount() int                          { return 1 }
func (d *headerMutationDriver) BackendKind() fixture.BackendKind           { return fixture.HTTPHeaderMutation }
func (d *headerMutationDriver) SubjectListenerName() string                { return "l_lws" }
func (d *headerMutationDriver) SubjectListenerPort() int                   { return 10011 }
func (d *headerMutationDriver) ReferenceListenerPort() int                 { return 10012 }
// l_mws (the flag=true listener) lives at SubjectListenerPort()+1 (subj 10012) /
// ReferenceListenerPort()+1 (ref 10013). The driver hardcodes this offset to
// match the SPEC §7.4 bootstrap.

// ReferenceBootstrap templates envoy.yaml substituting backendPorts[0] into
// the c_backend cluster's load_assignment endpoint.
func (d *headerMutationDriver) ReferenceBootstrap(backendPorts []int) string {
    // (read envoy.yaml as a template; substitute the backend port; return the rendered bytes)
    tmpl := refBootstrapTemplate // see envoy.yaml verbatim with {{.BackendPort}} placeholder
    var buf bytes.Buffer
    if err := template.Must(template.New("ref").Parse(tmpl)).Execute(&buf, struct{ BackendPort int }{backendPorts[0]}); err != nil {
        panic(err)
    }
    return buf.String()
}

// SubjectConfig templates envoy-go.yaml.
func (d *headerMutationDriver) SubjectConfig(subjectAdminPort, subjectListenerPort int, backendPorts []int) string {
    tmpl := subjBootstrapTemplate
    var buf bytes.Buffer
    if err := template.Must(template.New("subj").Parse(tmpl)).Execute(&buf, struct {
        AdminPort, LwsPort, MwsPort, BackendPort int
    }{subjectAdminPort, subjectListenerPort, subjectListenerPort + 1, backendPorts[0]}); err != nil {
        panic(err)
    }
    return buf.String()
}

// driveScenarios issues the 4 probes against `addr` (proxy address) and
// returns the deterministic byte stream of per-probe assertion-log lines.
func (d *headerMutationDriver) driveScenarios(ctx context.Context, lwsAddr, mwsAddr string) []byte {
    var b bytes.Buffer
    client := &http.Client{Timeout: 5 * time.Second}

    probes := []struct {
        name string
        addr string
        path string
    }{
        {"scenario_1_listener_only", lwsAddr, "/listener-only/anything"},
        {"scenario_2_route_override", lwsAddr, "/route-override/anything"},
        {"scenario_3_multi_tier_lws", lwsAddr, "/multi-tier/anything"},
        {"scenario_4_multi_tier_mws", mwsAddr, "/multi-tier/anything"},
    }
    for _, p := range probes {
        req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+p.addr+p.path, nil)
        // Add a deterministic User-Agent so listener-level Remove("user-agent") has a target.
        req.Header.Set("User-Agent", "fixture-0012")
        resp, err := client.Do(req)
        fmt.Fprintf(&b, "=== %s ===\n", p.name)
        if err != nil {
            fmt.Fprintf(&b, "ERROR: %v\n", err)
            continue
        }
        fmt.Fprintf(&b, "status: %d\n", resp.StatusCode)
        // Sort response header names for determinism.
        names := make([]string, 0, len(resp.Header))
        for n := range resp.Header {
            names = append(names, n)
        }
        sort.Strings(names)
        for _, n := range names {
            // Allow-list framework-injected variability per the existing
            // 0011-http-fault precedent: skip Date / Content-Length / Server.
            if strings.EqualFold(n, "Date") || strings.EqualFold(n, "Server") || strings.EqualFold(n, "Content-Length") {
                continue
            }
            for _, v := range resp.Header.Values(n) {
                fmt.Fprintf(&b, "header: %s: %s\n", n, v)
            }
        }
        // Body — already sorted by the backend.
        body := make([]byte, 8192)
        n, _ := resp.Body.Read(body)
        resp.Body.Close()
        // The body lists request headers; allow-list synthetic ones the proxies
        // inject (X-Forwarded-For, X-Forwarded-Proto, X-Request-Id,
        // X-Envoy-Expected-Rq-Timeout-Ms, X-Envoy-Internal). The driver strips
        // these from the body before equivalence-comparing.
        bodyStr := stripAllowListedRequestHeaders(string(body[:n]))
        fmt.Fprintf(&b, "body:\n%s\n", bodyStr)
    }
    return b.Bytes()
}

// stripAllowListedRequestHeaders removes proxy-injected request-header lines
// from the body string so reference vs subject diff cleanly.
func stripAllowListedRequestHeaders(body string) string {
    allowList := []string{"X-Forwarded-For:", "X-Forwarded-Proto:", "X-Request-Id:", "X-Envoy-", "Date:"}
    var out strings.Builder
    for _, line := range strings.Split(body, "\n") {
        skip := false
        for _, prefix := range allowList {
            if strings.HasPrefix(line, prefix) {
                skip = true
                break
            }
        }
        if !skip {
            out.WriteString(line)
            out.WriteString("\n")
        }
    }
    return out.String()
}

// DriveReference / DriveSubject implement the fixture.Driver interface for the runner.
func (d *headerMutationDriver) DriveReference(ctx context.Context, refAddr string) []byte {
    // refAddr is the l_lws address; l_mws is at the next port.
    lwsAddr := refAddr
    mwsAddr := strings.Replace(refAddr, fmt.Sprintf(":%d", d.ReferenceListenerPort()), fmt.Sprintf(":%d", d.ReferenceListenerPort()+1), 1)
    return d.driveScenarios(ctx, lwsAddr, mwsAddr)
}
func (d *headerMutationDriver) DriveSubject(ctx context.Context, subjAddr string) []byte {
    lwsAddr := subjAddr
    mwsAddr := strings.Replace(subjAddr, fmt.Sprintf(":%d", d.SubjectListenerPort()), fmt.Sprintf(":%d", d.SubjectListenerPort()+1), 1)
    return d.driveScenarios(ctx, lwsAddr, mwsAddr)
}

// ProbeAdmin issues GET /ready against the admin endpoint. Mirrors the existing
// fixture pattern (0011-http-fault).
func (d *headerMutationDriver) ProbeAdmin(ctx context.Context, adminAddr string) []byte {
    client := &http.Client{Timeout: 5 * time.Second}
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+adminAddr+"/ready", nil)
    resp, err := client.Do(req)
    if err != nil {
        return []byte(fmt.Sprintf("ERROR: %v\n", err))
    }
    body := make([]byte, 256)
    n, _ := resp.Body.Read(body)
    resp.Body.Close()
    return []byte(fmt.Sprintf("status: %d\nbody: %s\n", resp.StatusCode, string(body[:n])))
}

// refBootstrapTemplate is envoy.yaml verbatim with the backend port templated.
const refBootstrapTemplate = `<paste contents of envoy.yaml with {{.BackendPort}} substituted for 18012>`

// subjBootstrapTemplate is envoy-go.yaml verbatim with admin/listener/backend ports templated.
const subjBootstrapTemplate = `<paste contents of envoy-go.yaml with port placeholders>`
```

NOTE: the implementer at Task 15 step 1 grep-locates the existing `fixture.Driver` interface (at `test/differential/fixture/fixture.go`) and matches the signature precisely; the above sketch may diverge from the actual interface (e.g., the `ReferenceBootstrap`/`SubjectConfig` signature may take a different parameter set per the existing precedent). The 0011-http-fault driver at `test/fixtures/0011-http-fault/driver/driver.go` is the structural precedent — copy its scaffolding verbatim and adapt the four probes.

The `refBootstrapTemplate` + `subjBootstrapTemplate` constants are the verbatim YAML files from Task 13 with backend port substituted via `{{.BackendPort}}` placeholder. Embed via `//go:embed` directive if the existing 0011-http-fault driver uses that pattern.

- [ ] **Step 2: Run the fixture differentially**

```bash
go test -count=1 -v ./test/differential/ -run Test.*0012 2>&1 | tail -60
```

Expected: PASS; per-probe diff returns no differences.

- [ ] **Step 3: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add test/fixtures/0012-http-header-mutation/driver/
git rm test/fixtures/0012-http-header-mutation/driver/doc.go  # if not already removed by step 1
git commit -m "phase 10: fixture 0012 driver — 4-scenario orchestration"
```

SHA-fill follow-up.

*Anchored: SPEC §7.3; planner-time decision 9 (mirrors 0011-http-fault).*

---

## Task 16: BEHAVIOR_CONTRACT.md patches per SPEC §13 + ADR-0112 + ADR-0113 (deferrals) + ROADMAP row 10 in-progress→done

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (insert §13.1 subsection + §13.4 equivalence-matrix row + §13.5 forward-pointer notes)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0112, ADR-0113)
- Modify: `docs/envoy-go/ROADMAP.md` (row 10 status `in-progress → done`)

This task lands the documentation patches per SPEC §13 + the two deferral ADRs + the ROADMAP row flip. Per ADR-0052 in-place edit authorisation. NO new code touched.

**Precondition:** Task 15 done; all 4 fixture scenarios passing.
**Artifact:** modified BEHAVIOR_CONTRACT.md + DECISIONS.md + ROADMAP.md.
**Acceptance:** ADR-0112 + ADR-0113 in DECISIONS.md; BEHAVIOR_CONTRACT.md has the §13.1 + §13.4 + §13.5 patches; ROADMAP row 10 status `done`.

- [ ] **Step 1: Insert the §13.1 `### envoy.filters.http.header_mutation` subsection in BEHAVIOR_CONTRACT.md**

Locate the existing `### envoy.filters.http.fault` subsection (line 862 in the verified master HEAD); insert the new `### envoy.filters.http.header_mutation` subsection AFTER fault's subsection ends. The verbatim Markdown patch is at SPEC §13.1 lines 1122–1191; copy it verbatim. The patch covers:

- `#### Asserted equivalence (per phase 10 SPEC §11)` — request-side + response-side mutations + AppendAction × 4 + keep_empty_value semantics + multi-valued header behavior per §11.4
- `#### Multi-tier per-route evaluation (per ADR-0110 + phase 10 SPEC §11.5)` — flag-controlled cross-tier ordering algorithm
- `#### Protected-header set (per ADR-0111 + phase 10 SPEC §11.1)` — 6-name set + CONFIG-LOAD-TIME enforcement + verbatim error format
- `#### Stats — none emitted (per phase 10 SPEC §11.3)` — zero stats; analogous to cors per ADR-0074
- `#### Does not yet apply to (per phase 10 deferrals — ADRs 0112, 0113)` — the deferral table that ADR-0112 + ADR-0113 codify
- `#### Empirical evidence (verbatim curl excerpts from phase 10 SPEC §11)` — sample curl output

- [ ] **Step 2: Append §13.4 equivalence-matrix row**

Locate the `## Equivalence Matrix` section (line 9). Append the new row per SPEC §13.4 (lines 1206 verbatim).

- [ ] **Step 3: Insert §13.5 forward-pointer notes**

Per SPEC §13.5 (lines 1211–1214):
- After the `### typed_per_filter_config 3-tier merge` discussion (the section codifying ADR-0073's most-specific-override discipline): one paragraph noting ADR-0110's multi-tier amendment.
- After the `### envoy.filters.http.cors ### Asserted equivalence` block: one paragraph noting that header_mutation is the SECOND production filter to mutate response headers in EncodeHeaders.

- [ ] **Step 4: Append ADR-0112 + ADR-0113 to DECISIONS.md**

Per the ADR-0040 deferral-ADR format (Status: Deferred; Date; Doctrine; Lands-in-task; Context; Decision: deferred + rationale; Alternatives: implement-now (rejected); Consequences: forward-pointer to BEHAVIOR_CONTRACT §13.1's `#### Does not yet apply to` paragraph). Key anchors:

- **ADR-0112** (`mutations.query_parameter_mutations[]` deferred): coupled to `KeyValueMutation` triple + path/query rewriting subsystem; out of scope for phase 10 (header-only filter scope per SPEC §1.1 + §2.1). Forward-pointer: future `query_parameter_mutations` extension as a separate phase.
- **ADR-0113** (header-value formatter substitution deferred): full Envoy command-string subsystem (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, `%RESPONSE_CODE%`, `%START_TIME(...)%` etc) is its own multi-phase project; phase 10's runtimeConfig stores header values as static strings verbatim (a configured value of `"%REQ(:path)%"` produces the literal 11-byte string on the wire). Forward-pointer: future phase that lifts the access-log subset (per phase 06.2) into a header-value evaluator.

- [ ] **Step 5: Flip ROADMAP row 10 status `in-progress → done`**

Locate row 10 in `docs/envoy-go/ROADMAP.md` and change the `status` column from `in-progress` to `done`. The §9 family heading at line 56 stays UNCHANGED per ADR-0106.

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/ROADMAP.md
git commit -m "phase 10: BEHAVIOR_CONTRACT + ADR-0112 + ADR-0113 + ROADMAP row 10 done"
```

SHA-fill follow-up.

*Anchored: SPEC §13.1 + §13.4 + §13.5; ADR-0040 (deferral format); ADR-0052 (in-place edit); ADR-0106 (family heading unchanged); SPEC §15 acceptance bullet.*

---

## Task 17: Phase-done six-gate verification + STATE.md advance + phase-done commit

**Files:**
- Modify: `docs/envoy-go/STATE.md`

This task verifies the SPEC §3 six-gate phase-done checklist and advances STATE.md to `awaiting next planning`. Per `BOOTSTRAP_PROMPT.md` §7.5 + SPEC §3.

**Precondition:** Task 16 done.
**Artifact:** modified STATE.md + verbatim verification commands' output captured in PROGRESS.md.
**Acceptance:** all six gates report green; STATE.md flipped.

- [ ] **Step 1: Run gate (a) — `go build ./...` + `go vet ./...` + `golangci-lint run ./...`**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
```

Expected: clean. Capture output to PROGRESS.md Task 17 entry.

- [ ] **Step 2: Run gate (b) — `go test -race ./...` clean**

```bash
go test -race -count=1 ./...
```

Expected: every package PASS. Capture output.

- [ ] **Step 3: Run gate (c) — h2spec re-run at the ADR-0051 pin (53/53 PASS)**

```bash
# (run the existing h2spec gate; the exact command lives in the makefile or scripts/ directory)
make h2spec  # or whatever the existing entry point is
```

Expected: 53/53 PASS unchanged. Capture output.

- [ ] **Step 4: Run gate (d) — fuzzers (existing 12 + new 1 = 13) clean at 30s budget**

```bash
# Run each fuzzer for 30s (or use the existing CI script that iterates them).
# At minimum:
go test -fuzz=FuzzHeaderMutationConfigParse -fuzztime=30s ./internal/filter/http/header_mutation/
# Plus the 12 pre-existing fuzzers.
```

Expected: all fuzzers run clean.

- [ ] **Step 5: Run gate (e) — differential fixtures 0000–0012 all green**

```bash
go test -count=1 -v ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012'
```

Expected: every fixture PASS including the new 0012.

- [ ] **Step 6: Verify gate (f) — BEHAVIOR_CONTRACT.md populated**

Spot-check the §13.1 + §13.4 + §13.5 patches landed in Task 16 are present:

```bash
grep -nE 'envoy.filters.http.header_mutation' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -10
```

Expected: matches in `## HTTP filter chain`, `## Equivalence Matrix`, and forward-pointer locations.

- [ ] **Step 7: Update `docs/envoy-go/STATE.md`**

Flip:
- `lifecycle-state` → `awaiting next planning` (or the equivalent post-phase-done state per `BOOTSTRAP_PROMPT.md` §5)
- `next-skill` → `superpowers:brainstorming` (the next §9 family-child cold-starts from the §9 heading per ADR-0106)
- `next-skill-scope` → describes the cold-start: read ROADMAP.md row 10 + BEHAVIOR_CONTRACT.md ### envoy.filters.http.header_mutation + DECISIONS.md tail (now ADR-0113); the next family-child is selected by the brainstormer per the §9 family list at ROADMAP line 58 (compression / local_ratelimit / jwt_authn / rbac / etc).
- `active-phase` → `<next-family-row-id>` resolved by the next session's planner; this PLAN sets it to a sentinel value (e.g., `<unset — next session resolves>`)
- `last-commit` → the phase-done commit SHA (filled in step 9 SHA-fill follow-up)
- `last-updated` → current date

Per the user memory: surface the advisory off-master pre-brainstorm notes for `local_ratelimit` (branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes`) IF the next family-child target is `local_ratelimit`.

- [ ] **Step 8: Phase-done commit**

```bash
git add docs/envoy-go/STATE.md docs/envoy-go/phases/10-http-filter-header-mutation/PROGRESS.md
git commit -m "phase 10: http-filter-header-mutation [ADR-0108, ADR-0109, ADR-0110, ADR-0111, ADR-0112, ADR-0113]

Lands envoy.filters.http.header_mutation under the 07.1 framework.
THIRD §9 family-row to land (after cors @ 07.1 and fault @ 09).

ROADMAP row 10 flips in-progress → done at this commit.
The §9 family heading at ROADMAP line 56 stays unchanged (headings are
not rows; per ADR-0106).

Six ADRs land:
- ADR-0108: package shape + boot registration
- ADR-0109: runtimeConfig + AppendAction × 4 + keep_empty_value semantics
- ADR-0110: multi-tier per-route evaluation framework + amends ADR-0073
- ADR-0111: protected-header set + CONFIG-LOAD-TIME rejection
- ADR-0112: mutations.query_parameter_mutations[] deferred
- ADR-0113: header-value formatter substitution deferred

Framework deltas: PerRouteConfig.ResolveAllTiers (sibling to Resolve);
DecoderFilterCallbacks.RequestRouteConfigsAllTiers (decoder-only per
PLAN planner-time decision 1; used from both decode and encode bodies);
HTTPRegistry.RegisterPerRouteValidator + BuildPerRouteConfig hook
(per planner-time decision 3 — eager per-route validation lifecycle).

Differential fixture 0012-http-header-mutation green (4 scenarios:
listener-only, per-route override, multi-tier flag=false least-specific
wins, multi-tier flag=true most-specific wins).

Zero new stats (analogous to cors per ADR-0074); 22-name table UNCHANGED.

All six phase-done gates green: build/vet/lint clean; race tests pass;
h2spec 53/53 PASS unchanged; 13 fuzzers green at 30s budget; all 14
differential fixtures (0000–0012) green; BEHAVIOR_CONTRACT.md populated."
```

SHA-fill follow-up commit per the phase-04..09 convention.

*Anchored: SPEC §3 + §15; BOOTSTRAP_PROMPT.md §5.3 + §7.5.*

---

## Task 18: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md`

This task drafts the end-of-phase REVIEW.md per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 cadence; populates per the `superpowers:requesting-code-review` skill. Phase 10 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 10. NO new ADRs.

**Precondition:** Task 17 done.
**Artifact:** REVIEW.md.
**Acceptance:** REVIEW.md committed; covers per-task retrospective + carry-forward findings + planner-time decisions retrospective.

- [ ] **Step 1: Invoke the `superpowers:requesting-code-review` skill**

If executing inline: read the skill output and apply its REVIEW shape. If executing subagent-driven: dispatch a code-reviewer subagent with the phase 10 SPEC + PLAN + PROGRESS as context.

- [ ] **Step 2: Draft REVIEW.md mirroring 09's REVIEW.md structure**

The REVIEW typically covers:
- N-1 carry-forward retrospective (review 09's REVIEW for any items requesting phase-10 follow-up; address each)
- Per-task retrospective (any task that landed deviations from PLAN; record the rationale)
- Planner-time decisions retrospective (each of the 11 decisions: did the implementation validate the choice or expose a flaw?)
- Carry-forward findings for phase 11 (e.g., framework primitives that proved load-bearing; deferrals that warrant scheduling; any minor tech-debt the next phase can pick up)
- ADR retrospective (each of the 6 ADRs: did the §Decision body hold up under implementation + fixture exercise?)
- Six-gate retrospective (any gate that was non-trivial to satisfy)

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md
git commit -m "phase 10: REVIEW — end-of-phase retrospective + N-1 carry-forward"
```

SHA-fill follow-up.

*Anchored: superpowers:requesting-code-review; phase-09 REVIEW precedent (master `3066c72`).*

---

## Refinement

If during execution the implementer discovers a SPEC ambiguity, a planner-time decision that was not foreseen, or a framework constraint that requires deviation from this PLAN, the implementer:

1. Records the deviation in PROGRESS.md's per-task entry under a `**Deviation:**` line + `**Rationale:**` + `**Anchored:**` cross-reference.
2. If the deviation alters the ADR table, amend the in-task ADR's Consequences section in-place (per the ADR-0089 consequence (b) in-place-edit pattern); do NOT introduce a new ADR for the amendment unless the deviation is structurally significant.
3. If the deviation alters the file-structure table, amend this PLAN's table in a follow-up commit OR record the deviation in PROGRESS.md and let the file-structure table become "as-built" rather than "as-planned" — the implementer's choice based on whether the deviation is broadly reusable for future readers.

Common refinement scenarios anticipated:

- **The `BuildPerRouteConfig` signature widening (Task 4) breaks existing callers.** The HCM caller at `internal/filter/hcm/config.go::parseHTTPFiltersChain` thread the existing `httpRegistry` local; the test-only callers in `perroute_test.go` thread `nil`. The implementer extends affected test files inline in Task 4 — the existing tests at `perroute_test.go:21–101` (TestPerRoute_BuildAndResolve_RouteWins / VHostFallback / RCFallback / NilOnAbsent / BuildRejectsUnknownFilterName / LazyCacheHitMiss) all pass `nil` for the new registry param.
- **The `ResolveAllTiers` method's cache discipline interacts unexpectedly with the existing `Resolve` cache.** Per planner-time decision 2 + SPEC §6.7 NOTE: ResolveAllTiers does NOT consult or pollute the existing `p.cache` (the cache key is `(filterName, routeIdx) → single proto.Message`; multi-tier returns 3 messages with different cache shape). If profiling under fixture 0012 shows measurable cost, ResolveAllTiers gains its own cache as a follow-up phase.
- **The per-route protected-header validator's location-prefix format diverges from the existing `parseMap` pattern.** The existing `parseMap` at `perroute.go:69–74` uses `"hcm: %s: typed_per_filter_config[%q]: <error>"`. The validator wrapper should mirror this format exactly — implementer copies the format string verbatim. If the validator returns an error from a different location-format than expected, the integration test at Task 4 catches it.
- **The cors + fault tests regress when `BuildPerRouteConfig` widens.** The widening adds an optional registry parameter; cors + fault tests pass `nil` (no per-route-validator registered). If the cors test suite at `internal/filter/http/cors/cors_test.go` uses `BuildPerRouteConfig` directly, the implementer threads `nil` through the call site. Expected to be a one-line change per affected test.
- **Envoy v1.37.2's behavior on `most_specific_header_mutations_wins` cross-tier ordering diverges from the SPEC §6.5 algorithm.** The §11.5 empirical pin confirms the algorithm matches Envoy verbatim (with listener `x-test=listener`, RC `x-test=rc`, VHost `x-test=vh`, Route `x-test=route`: flag=false → final `x-test: rc`; flag=true → final `x-test: route`). If fixture 0012 shows divergence, re-execute the §11.5 probe against current Envoy v1.37.2 to confirm the pin is still valid.

## Post-plan handoff

After Task 18 lands the REVIEW, the orchestrating session:

1. Verifies the phase-done six gates one more time (sanity check) per Task 17.
2. Verifies STATE.md is at `awaiting next planning` with `next-skill: superpowers:brainstorming`.
3. Pushes the phase 10 worktree branch to origin (per the user's persistent preference: "after a clean local merge/commit on master with tests green, push without asking" recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`).
4. Hands off to the next session, whose first action is to invoke `superpowers:brainstorming` against §9's HTTP filters family for the next family-child (per ADR-0106 + STATE.md + BRAINSTORM.md Decision 13 — the next family-child cold-starts from the §9 heading + the just-shipped phase 10 artefacts; no sibling-stub was authored). Note: per `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md` reference entry, advisory off-master pre-brainstorm notes for `local_ratelimit` exist on branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (pushed to origin); surface those notes to the brainstormer if/when phase 11 targets `local_ratelimit`.

The phase 10 work is complete when:

- All 18 tasks in this PLAN have green checkmarks in PROGRESS.md.
- Phase-done commit + SHA-fill follow-up are on master.
- REVIEW.md is committed.
- STATE.md reflects the post-10 lifecycle state.
- The branch is pushed to origin.
- All six gates report green at HEAD.

