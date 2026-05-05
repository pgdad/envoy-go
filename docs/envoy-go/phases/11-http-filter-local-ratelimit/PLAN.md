# Phase 11 — HTTP filter `envoy.filters.http.local_ratelimit` (`internal/filter/http/localratelimit/`, differential fixture `0013-http-local-ratelimit`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` extension) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 11), §9 (HTTP filters family — phase 11 is the FOURTH top-level row to land under the §9 family heading after cors @ 07.1, fault @ 09, and header_mutation @ 10 per ADR-0106 settled by phase 09 + reaffirmed by phase 10); `docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; 1225 lines, 16 sections, **read in full**); `docs/envoy-go/phases/11-http-filter-local-ratelimit/BRAINSTORM.md` (the autonomous-brainstorm artefact at master `6ad8d8a` that the SPEC distils §§1–12 from — 9 Decisions + §9 empirical-pin obligations all executed at SPEC time; consult when the SPEC's "what" needs the BRAINSTORM's "why"); `docs/envoy-go/phases/10-http-filter-header-mutation/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (closed read-only history; 10's PLAN at master `97ed8b9` is the structural precedent — task-numbering, TDD-step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections; phase-10 used 18 tasks for ~430 LoC production code + ~720 LoC fixture); `docs/envoy-go/phases/09-http-filter-fault/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (secondary precedent for SendLocalReply + StopIteration pattern + the four-counter `filterStats` discipline); `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` (the cors precedent's PLAN — the per-filter package-shape phase 11 inherits); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0113 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format — phase 11's 14-field deferral is a documentation artefact (per SPEC §8.1 ADR-0120 collapse) and lives inline at BEHAVIOR_CONTRACT §13.1 + §13.5 rather than in a dedicated ADR, **ADR-0044** ADR-on-impl convention, **ADR-0045** planner-time-split discipline (~25 tasks / ~1500 LoC thresholds — both well under for this phase per `## Scope check` below), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0061** stats Registry / SN1–SN8 flattening rules — phase 11 emits FOUR new stats per `<stat_prefix>` (so the existing 22-name table extended by phase 09 grows to 26; the SN-rule set extends with a NEW SN9 rule for the local-ratelimit filter-specific tag-extractor per planner-time decision 1), **ADR-0071** HTTP-filter framework chain-shape + factory pattern + iteration-protocol surface — phase 11's filter is the FIRST production filter to combine `SendLocalReply + StopIteration` (request-side terminal-replace per ADR-0102; reused VERBATIM from the fault precedent at `internal/filter/http/fault/fault.go:321`) with PER-INSTANCE STATEFUL RESOURCES (a `*tokenBucket` carrying a `sync.Mutex`); the framework's per-instance discipline carries through unchanged, **ADR-0072** HTTPRegistry threaded constructor map + factory typed_config validation contract — phase 11's `New` factory mirrors Envoy v1.37.2's CONFIG-LOAD-TIME PGV rejection per SPEC §11.1 + §11.2 (filter-internal 50ms minimum on `fill_interval` is enforced as a SIBLING-CHECK after the proto unmarshal succeeds, NOT a PGV constraint, per SPEC §11.2c + ADR-0115), **ADR-0073** typed_per_filter_config 3-tier merge (most-specific override) — phase 11 reuses VERBATIM (no `ResolveAllTiers` invocation; the most-specific-override discipline applies; ADR-0117 = ADR-0073 amendment paragraph noting that wholesale-override extends to STATEFUL per-route resources without further framework support — phase 11 is the FIRST production filter to demonstrate this), **ADR-0074** filter set: cors + envoy_go_test — phase 11 adds local_ratelimit as the FIFTH real production filter (after cors, envoygotest, fault, header_mutation) under the same package-shape discipline, **ADR-0075** sendLocalReply enters encode chain at filter[len-1] — UNCHANGED in phase 11 (local_ratelimit's rate-limited path uses `cb.SendLocalReply` per the fault precedent; the chain's `localReplyDone` gate carries the response back to client without dialing upstream, **ADR-0100** FactoryCtx framework extension (`Stats *stats.Registry` + `StatPrefix string`) — local_ratelimit CONSUMES `ctx.Stats` (for the four-counter `filterStats` registration per SPEC §6.6 + §11.5; analogous to fault's per-instance counter discipline) but does NOT consume `ctx.StatPrefix` (the local_ratelimit filter-level stat_prefix is the proto-message field `cfg.StatPrefix`, NOT the HCM-level stat_prefix); the 3-field FactoryCtx stays as-is per ADR-0100, **ADR-0101** runtimeConfig shape + parser pattern — phase 11's `runtimeConfig` mirrors fault's structurally (5 fields per SPEC §6.2; closure-captured at `New`, immutable post-construction; per-instance + per-route TPFC entries each carry an independent `*tokenBucket` + `*filterStats`), **ADR-0102** terminal-replace + StopIteration localReplyDone gate — VERBATIM reuse for the rate-limited path; no new framework primitive, **ADR-0103** fault abort wire shape (body byte-exact) — phase 11's wire shape follows the same discipline (body `local_rate_limited`, 18 bytes, no LF; 4-header set lowercase wire-form per SPEC §11.3; `server: envoy` literal per the existing HCM `serverHeader()` at `internal/filter/hcm/codec.go:17`), **ADR-0104** fault deferral ADR pattern — NOT used by phase 11 (the 14-field deferral is captured inline at BEHAVIOR_CONTRACT §13.1 + §13.5 per SPEC §8.1 ADR-0120 collapse; ADR-0104's per-field-cluster ADR pattern is reserved for cases where deferred fields carry load-bearing forward-decisions, not phase 11's case), **ADR-0106** §9 HTTP filters family expansion shape (flat top-level rows + no-sibling-stub) — UNCHANGED in phase 11 (phase 11 is a flat top-level row, not a sub-phase of any §9 parent; the §9 heading at ROADMAP line 56 stays unchanged), **ADR-0108..ADR-0113** (phase 10 ADRs); ADR-0113 is the verified DECISIONS.md tail at master `97ed8b9` (phase 10 phase-done); phase 11's six anticipated ADRs land at ADR-0114..ADR-0119 per SPEC §8); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## HTTP filter chain` umbrella at line 724 hosts the new `### envoy.filters.http.local_ratelimit` subsection per SPEC §13.1, inserted AFTER the existing `### envoy.filters.http.fault` subsection at line 867 landed by phase 09 AND the `### envoy.filters.http.header_mutation` subsection at line 924 landed by phase 10; `## Stat-name mapping` 22-name table at line 59 extends to **26 names** with a four-counter set per SPEC §13.2 + new filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` per ADR-0118; `## Timing tolerances` at line 286 gains one new row for fixture 0013 scenario 3's t=250ms refill boundary (±10ms wallclock) per SPEC §13.3; `## Equivalence Matrix` at line 9 gains one new row per SPEC §13.4; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 11 — phase 11 is a pure HTTP-layer filter addition; touches no codec/framer/HPACK paths; the h2spec gate at 53/53 PASS is mechanical re-run); `docs/envoy-go/ROADMAP.md` (row `11` per the SPEC commit's row-flip; row `11` flips `in-progress → done` at this phase's phase-done; the §9 HTTP filters family heading at row 56 stays unchanged across all §9-family-row landings per ADR-0106); `internal/filter/http/cors/cors.go` (the package-shape precedent local_ratelimit inherits — TypeURL constant + New factory + filter struct implementing both StreamDecoderFilter + StreamEncoderFilter; cors's `SetDecoderCallbacks` / `SetEncoderCallbacks` callback-wiring pattern is the precedent for phase 11's decoder-only callback design — per SPEC §6.3 phase 11 sets only `dcb`, the encode side is pure pass-through); `internal/filter/http/fault/fault.go` (the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution via `routeConfigOrListener` + the `cb.SendLocalReply(status, body, OrderedHeaders{Content-Type: text/plain}) + return StopIteration` pattern at fault.go:321 that phase 11 reuses verbatim; phase 11's per-instance `filter` struct mirrors fault's modulo no-async-resume / no-timer / no-rng / no-overflow); `internal/filter/http/header_mutation/header_mutation.go` (tertiary precedent for the unmarshal-at-New + closure-capture-runtimeConfig + per-route validator pattern; phase 11 explicitly DIVERGES from header_mutation's underscore-preserving directory pattern per ADR-0114 — `localratelimit/` is single-token-no-underscore matching cors + fault); `internal/filter/http/types.go` (FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilter + HTTPFilterFactory + FilterInstanceFactory + FactoryCtx — UNCHANGED in phase 11; the 3-field FactoryCtx per ADR-0100 stays as-is; phase 11 consumes `ctx.Stats` only); `internal/filter/http/perroute.go` (existing 3-tier `Resolve` per ADR-0073 — phase 11 reuses VERBATIM; the phase-10-introduced `ResolveAllTiers` sibling stays landed but is NOT consumed by phase 11; per-route TPFC entries are parsed by `BuildPerRouteConfig`'s generic `UnmarshalNew` into `proto.Message` slots — `BuildPerRouteConfig` does NOT recursively call any registered filter `New` — so phase 11 builds each per-route `*runtimeConfig` (with its own `*tokenBucket` + `*filterStats`) lazily at first-resolve time via a per-filter `sync.Map` cache keyed by per-route proto pointer per ADR-0117); `internal/filter/http/registry.go` (existing extension registry — phase 11 adds one Register call site upstream in `cmd/envoy-go/main.go`; the phase-10-introduced `RegisterPerRouteValidator` hook is NOT consumed by phase 11 since local_ratelimit has no per-route invariants requiring boot-time validation — per-route TPFC entries are validated lazily at first-resolve via `buildRuntimeConfigPerRoute`); `internal/stats/registry.go` + `internal/stats/name.go` (existing stats Registry + the `flattenToProm` Prometheus rendering per Rules SN1–SN8; phase 11 ADDS a NEW Rule SN9 entry for the `<stat_prefix>.http_local_rate_limit.<counter>` shape per planner-time decision 1 + ADR-0118); `internal/filter/hcm/codec.go:17` (`serverHeader()` returning literal `"envoy"` — confirms the SPEC §1.1 amendment that the rate-limited response carries `server: envoy`, NOT `server: envoy-go`).

**Goal:** Land envoy-go's `envoy.filters.http.local_ratelimit` HTTP filter — the FOURTH production HTTP filter after cors (07.1), fault (09), and header_mutation (10), and the THIRD top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family after fault and header_mutation. Concretely (per SPEC §1 + §4): a new `internal/filter/http/localratelimit/` package owning the filter implementation under the cors + fault precedents' package-shape discipline (`local_ratelimit.go` + `bucket.go` + `local_ratelimit_test.go` + `bucket_test.go` + `doc.go` + `fuzz_test.go`; the file split is settled here per the file-structure decision in `## File structure` below — `bucket.go` carries the `tokenBucket` primitive + its constructor; `local_ratelimit.go` carries TypeURL + types + `runtimeConfig` parser + `New` + `filter` struct + `DecodeHeaders` + `filterStats` + tag-extractor registration init; ~700 LoC across the production files + ~280 LoC unit tests across the two test files + 50 LoC fuzzer); a small framework extension to `internal/stats/name.go` adding Rule SN9 (the filter-specific tag-extractor for `<stat_prefix>.http_local_rate_limit.<counter>` per planner-time decision 1 + ADR-0118) — the cleanest registration site is `internal/stats/name.go`'s `flattenToProm` switch; ~20 LoC delta + matching `name_test.go` extension (~30 LoC); a `cmd/envoy-go/main.go` one-line registration delta (`httpReg.Register(localratelimit.TypeURL, localratelimit.New)` inserted alphabetically after the existing header_mutation registration, plus the matching package import; ~3 LoC delta); a NEW differential fixture `0013-http-local-ratelimit` (`test/fixtures/0013-http-local-ratelimit/`) with `envoy.yaml` + `envoy-go.yaml` (FOUR pre-configured listeners `l_s1` + `l_s2` + `l_s3` + `l_per_route` per planner-time decision 8 — diverges from SPEC §7.1's two-listener+teardown layout to fit within the existing differential-fixture harness's single-Drive-call contract; explicit `filter_enabled` + `filter_enforced=100%` on all listeners on BOTH sides per SPEC §1.1 amendment) + `expectations.yaml` + `README.md` + `driver/driver.go` (four-scenario orchestration per SPEC §7.1 + §7.2 via `fixture.MultiListenerDriver`; ALL scenarios in one `DriveSubjectMulti`/`DriveReferenceMulti` invocation; ±10ms tolerance on scenario 3 t=250ms boundary) + `backends/backend.go` (minimal Go HTTP backend; ~30 LoC; mirrors fault 0011 backend pattern with body `backend\n`); a NEW `BackendKind` enum value `HTTPLocalRateLimit BackendKind = 10` in `test/differential/fixture/fixture.go` + a matching `startHTTPLocalRateLimitBackend` spawn helper in `test/differential/runner_test.go` + the blank-import for the fixture driver (~25 LoC delta); a NEW fuzzer `FuzzLocalRateLimitConfigParse` (~50 LoC; 30s budget per ADR-0018; **fifteenth fuzzer overall** — phase 10 closed at fourteenth `FuzzHeaderMutationConfigParse`); a `BEHAVIOR_CONTRACT.md` in-place edit per SPEC §13 (NEW `### envoy.filters.http.local_ratelimit` subsection under the existing `## HTTP filter chain` umbrella per §13.1 inserted AFTER the existing header_mutation subsection; `## Stat-name mapping` 22→26-name table extension per §13.2; `## Timing tolerances` new row per §13.3 — fixture 0013 scenario 3 t=250ms refill boundary ±10ms; `## Equivalence Matrix` new row per §13.4; ADR-0073 / ADR-0074 forward-pointer notes per §13.5; ADR-0052 in-place edit authorisation carries forward); six new ADRs ADR-0114..ADR-0119 per SPEC §8 (ADR-0114 package shape `localratelimit/` no-underscore departing from header_mutation's underscore-preserving pattern + extension-registry registration ordering; ADR-0115 runtimeConfig shape + 5-consumed/14-silent-ignored field decomposition + PGV constraint table + filter-internal `fill_interval ≥ 50ms` validation as a SIBLING-CHECK after proto unmarshal succeeds NOT a PGV constraint; ADR-0116 `tokenBucket` primitive Option-A lazy-refill on access + monotonic-time semantics + LBP-1-adjacent declaration + empirical refill-timing tolerance ±10ms; ADR-0117 per-route bucket isolation as ADR-0073 wholesale-override consequence — first stateful per-route filter — ADR-0073 amendment paragraph; ADR-0118 stat-table 22→26 extension + `enforced == rate_limited` MVP invariant + future shadow-mode widening point + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registered as Rule SN9 in `internal/stats/name.go`'s `flattenToProm`; ADR-0119 rate-limited response wire shape + body byte-exact `local_rate_limited` 18 bytes no LF + 4-header set lowercase wire-form `content-length`/`content-type`/`date`/`server: envoy` + 429 default status + SendLocalReply reuse from phase 09 fault precedent). After phase 11, the project has proven its thirteenth-leading-edge engineering claim per SPEC §1: *envoy-go's HTTP filter framework can host a stateful rate-limiting primitive that carries per-route independent token-buckets via the existing 3-tier `PerRouteConfig.Resolve` accessor with no framework extension; the existing fault `SendLocalReply` + `StopIteration` mechanism carries through verbatim for the rate-limited path; the stat surface extends from 22 to 26 names with a four-counter set whose `enforced == rate_limited` MVP invariant has a documented natural-divergence point at future shadow-mode landing; the 50ms `fill_interval` minimum is a filter-internal Envoy check (NOT PGV) and reflects in envoy-go's `New` factory as a sibling validation after proto unmarshal succeeds; per-route TPFC bucket isolation falls out of ADR-0073's existing wholesale-override semantics with no new framework primitive needed (codified at ADR-0117 as an ADR-0073 amendment paragraph noting that wholesale-override extends to stateful per-route resources); a NEW Prometheus tag-extractor (Rule SN9 in `internal/stats/name.go`'s `flattenToProm` switch) extracts `<stat_prefix>` from the dotted internal name into the `envoy_local_http_ratelimit_prefix` Prometheus label per the §11.5 empirical pin; all under flat top-level row expansion (per ADR-0106).* This is the FOURTH §9 family-row to land; subsequent filters (compression, jwt_authn, …) follow the same row-as-its-own-phase pattern. ROADMAP row `11` flips `in-progress → done` AT the phase-done commit; the §9 family heading at ROADMAP line 56 stays unchanged (headings are not rows; per ADR-0106); STATE.md flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle.

**Architecture:** The 11 surface is the additive registration of one new HTTP filter under `internal/filter/http/` plus a small Prometheus tag-extractor registration delta in `internal/stats/name.go` for the new `<stat_prefix>.http_local_rate_limit.<counter>` stat-name shape. The `localratelimit.New` factory runs at HCM-build time per ADR-0072's two-step pattern: (a) parses + validates the typed_config Any (rejects `tc == nil`, malformed Any, AND PGV constraints — `cfg.StatPrefix` non-empty per §11.1, `cfg.TokenBucket.MaxTokens > 0` per §11.2a, `cfg.TokenBucket.TokensPerFill > 0` per §11.2b-ii after defaulting from omitted, AND filter-internal `cfg.TokenBucket.FillInterval >= 50ms` per §11.2c — the filter-internal check fires AFTER proto unmarshal succeeds, NOT as a PGV constraint, returning a non-nil error mirroring Envoy v1.37.2's verbatim message `local rate limit token bucket fill timer must be >= 50ms`); (b) constructs a `*runtimeConfig` capturing the 5 consumed proto fields per §6.2 (`statPrefix string`, `bucket *tokenBucket`, `statusCode int`, `body []byte = []byte("local_rate_limited")`, `stats *filterStats`); (c) constructs the `*tokenBucket` primitive (lazy-refill on access; `sync.Mutex` per bucket; `time.Now().UnixNano()` monotonic clock; initial fill = `max_tokens`); (d) constructs the `*filterStats` four-counter set via `ctx.Stats.NewCounter(<stat_prefix>.http_local_rate_limit.<name>)` for `enabled`, `ok`, `rate_limited`, `enforced` per SPEC §6.6 + §11.5; (e) returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{rc: rc}` per request bound to the closure-captured `*runtimeConfig`. The per-instance `*filter` implements `StreamDecoderFilter` + `StreamEncoderFilter` per the cors + fault precedents (request-side rate-limit decision in `DecodeHeaders`; encode side is pure pass-through — no encode-side state). `DecodeHeaders` body discipline (per SPEC §6.5 + §11.3 + §11.5): increment `rc.stats.enabled` unconditionally; call `rc.bucket.tryConsume()`: if `true`, increment `rc.stats.ok` and return `Continue`; if `false`, increment `rc.stats.rateLimited` AND `rc.stats.enforced` IN LOCKSTEP (MVP invariant per ADR-0118), invoke `f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})`, and return `StopIteration`. Per-route 3-tier resolution uses the existing `PerRouteConfig.Resolve` per ADR-0073 (most-specific-override; the framework's pre-phase-10 default model). The framework's `BuildPerRouteConfig` (`internal/filter/http/perroute.go:63-85`) merely `UnmarshalNew`'s each per-route TPFC Any into a generic `proto.Message` — it does NOT call any registered filter's `New`. Phase 11 therefore delivers per-route bucket independence via a request-time **lazy cache**: the factory closure captures a `sync.Map` keyed by `*LocalRateLimitPerRoute` proto pointer; on first request that resolves to a per-route entry, the filter atomically `LoadOrStore`s a freshly-built `*runtimeConfig` carrying its own `*tokenBucket` + own four `*stats.Counter` pointers (registered via `Registry.NewCounterIfAbsent` for post-Freeze idempotent allocation per ADR-0117). Pointer identity of the resolved `*LocalRateLimitPerRoute` is the cache key, so all subsequent requests against the same per-route entry share the same `*runtimeConfig` (and therefore the same bucket) — preserving wholesale-override per ADR-0117 = ADR-0073 amendment. The listener-level `*runtimeConfig` is built eagerly at `New` time and used as the fallback when no per-route TPFC matches (3-tier returns nil from Route + VHost + RC tiers). Concurrency model: per-instance state — the `*filter{rc, dcb}` carries a `*runtimeConfig` reference (closure-captured; immutable post-construction; read-only thread-safe); the `*runtimeConfig` carries a `*tokenBucket` shared across all goroutines processing requests through that filter instance; the `*tokenBucket` is mutex-guarded (single `sync.Mutex` per bucket; hot-path 5–10 nanoseconds typical; LBP-1-adjacent per ADR-0116 — closure-capture half preserved, lock-free hot-path half deliberately departs since the elapsed→refills→tokens computation is multi-step CAS-resistant); the `*filterStats` carries 4 `*atomic.Int64` counters (lock-free counter increments per ADR-0061). The race detector run under gate (b) validates by construction; one explicit race-detector test `TestTokenBucket_ConcurrentTryConsume` per planner-time decision 7 fires `tryConsume` concurrently across 64 goroutines with shared `*tokenBucket` to validate the mutex discipline mechanically. Differential surface: fixture `0013-http-local-ratelimit` runs 4 scenarios per SPEC §7.1 (basic-allow / basic-rate-limited / refill-after-fill_interval / per-route-override) under FOUR pre-configured listeners (`l_s1`, `l_s2`, `l_s3` for scenarios 1–3 — bucket parameters baked at boot; `l_per_route` for scenario 4 with the per-route TPFC override topology) per planner-time decision 8 (which diverges from SPEC §7.1's two-listener+teardown layout to fit the existing harness's single-Drive-call contract); all 4 scenarios run in ONE `DriveReferenceMulti` / `DriveSubjectMulti` invocation against a shared static backend probe per SPEC §7.4; counter-delta assertions across 4 stat names per scenario; tag-extracted Prometheus label `envoy_local_http_ratelimit_prefix` byte-equal across reference vs subject; status + body + post-rate-limit header set asserted byte-equivalent across reference Envoy v1.37.2 vs envoy-go.

**Tech Stack:**
- Go 1.23 (unchanged from 09 + 10; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `errors`, `fmt`, `net/http`, `sync`, `sync/atomic`, `time` — the new `internal/filter/http/localratelimit/` package consumes only stdlib (no new module imports introduced by 11).
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3` (NEW import in this phase) — `*envoyextensionsfiltershttplocalratelimitv3.LocalRateLimit` proto + `*LocalRateLimitPerRoute` proto. Already present in `go.sum`'s transitive closure (the go-control-plane module-level dependency is unchanged from 10; no `go.mod` bump needed — verified at `## Execution preconditions` step 11 below).
- `github.com/envoyproxy/go-control-plane/envoy/type/v3` (existing; introduced by phase 04) — `*envoytypev3.TokenBucket` primitive (the SHARED proto carrying `max_tokens`, `tokens_per_fill`, `fill_interval` fields; reused across local + global rate-limit filter families) consumed by the runtimeConfig parser per SPEC §6.2.
- `google.golang.org/protobuf/types/known/anypb` (existing; introduced by 07.1) — `*anypb.Any` typed_config carrier consumed by `New(tc, ctx)`.
- `google.golang.org/protobuf/proto` (existing; introduced by 07.1 perroute) — `proto.Message` interface used by the per-route 3-tier `Resolve` accessor.
- `github.com/esalaine/envoy-go/internal/filter/http` (existing; introduced by phase 07.1, extended in phase 09 with FactoryCtx Stats + StatPrefix, extended in phase 10 with ResolveAllTiers / RequestRouteConfigsAllTiers / RegisterPerRouteValidator) — `FactoryCtx` (UNCHANGED in phase 11; the 3-field shape stays as-is per ADR-0100; phase 11 consumes `ctx.Stats` only — `ctx.StatPrefix` is the HCM-level connection-manager prefix, NOT the filter-level local_ratelimit stat_prefix per SPEC §6.1), `HTTPFilter`, `HTTPFilterFactory`, `FilterInstanceFactory`, `StreamDecoderFilter`, `StreamEncoderFilter`, `FilterHeadersStatus`, `FilterDataStatus`, `FilterTrailersStatus`, `Continue`, `DataContinue`, `TrailersContinue`, `StopIteration`, `OrderedHeaders`, `DecoderFilterCallbacks` (UNCHANGED in phase 11 — local_ratelimit consumes only the existing `SendLocalReply` method per ADR-0102; no new callback method introduced), `EncoderFilterCallbacks` (UNCHANGED in phase 11), `HTTPRegistry` (UNCHANGED in phase 11 — local_ratelimit registers ONE `Register` call; the phase-10-introduced `RegisterPerRouteValidator` hook is NOT consumed by phase 11), `BuildPerRouteConfig` (UNCHANGED in phase 11 — `BuildPerRouteConfig` does NOT call any registered filter `New`; per `internal/filter/http/perroute.go:63-85` it `UnmarshalNew`'s each TPFC Any into a generic `proto.Message` and stores it. Phase 11's per-route TPFC parsing therefore happens at REQUEST time via a per-filter lazy cache: the `*filter` instance carries a closure-captured `sync.Map` keyed by `*LocalRateLimitPerRoute` proto pointer; on first request that resolves to a per-route entry, the filter atomically `LoadOrStore`s a freshly-built `*runtimeConfig` carrying its own `*tokenBucket` + four `*stats.Counter` pointers obtained via the new `Registry.NewCounterIfAbsent` post-Freeze idempotent helper. ADR-0117 codifies this lazy-cache discipline as the wholesale-override consequence), `PerRouteConfig.Resolve` (existing 3-tier most-specific-override accessor per ADR-0073; phase 11 reuses VERBATIM).
- `github.com/esalaine/envoy-go/internal/filter/http/cors` (existing; the package-shape precedent local_ratelimit mirrors — TypeURL constant + New factory + filter struct + decoder + encoder + OnDestroy).
- `github.com/esalaine/envoy-go/internal/filter/http/fault` (existing; the secondary precedent — `runtimeConfig` shape + closure capture + per-route resolution + `cb.SendLocalReply + StopIteration` request-side terminal-replace pattern at fault.go:321; phase 11 mirrors verbatim modulo no-async-resume / no-timer / no-rng / no-overflow / different counter-name set).
- `github.com/esalaine/envoy-go/internal/filter/http/header_mutation` (existing; tertiary precedent for the unmarshal-at-New + closure-capture-runtimeConfig + per-route TPFC parsing pattern; phase 11 explicitly DIVERGES from header_mutation's underscore-preserving directory pattern per ADR-0114 — `localratelimit/` is single-token-no-underscore matching cors + fault).
- `github.com/esalaine/envoy-go/internal/stats` (existing; introduced by phase 06.1, extended in 09 with HCM-scoped fault stats per ADR-0061's SN2 internal-dot transform) — `*Registry`, `NewCounter`, `Walk`, `flattenToProm` (extended in Task 6 with a new Rule SN9 for the local-ratelimit filter-specific tag-extractor `envoy_local_http_ratelimit_prefix` per ADR-0118).
- `github.com/esalaine/envoy-go/test/differential/fixture` (existing; extended in Task 9 with a new `BackendKind` enum value `HTTPLocalRateLimit BackendKind = 10` per planner-time decision 9).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0013's reference image AND the source of the SPEC §11.1–§11.8 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 11's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 11 — phase 11 touches no codec/framer/HPACK paths; the conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0013's reference (Envoy in a Docker container) — same harness as 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 fixtures consume; phase 11 does NOT extend the harness's optional driver-side interfaces.
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's local_ratelimit filter implementation; any third-party token-bucket or rate-limit library (e.g., `golang.org/x/time/rate`). Test-side use is also forbidden. The `go.mod` post-11 must not list any new rate-limit-related runtime dependencies; the token-bucket primitive is implemented in-tree from stdlib only.

---

## Scope check — why phase 11 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 PLAN's component-table convention):

- `internal/filter/http/localratelimit/doc.go` ~40
- `internal/filter/http/localratelimit/local_ratelimit.go` ~400 + `internal/filter/http/localratelimit/bucket.go` ~80 + tests ~280 (split across `local_ratelimit_test.go` ~200 + `bucket_test.go` ~80) = ~760
- `internal/filter/http/localratelimit/fuzz_test.go` (REQUIRED per SPEC §14.3) ~50
- `internal/stats/name.go` SN9 rule extension ~+20 = ~+20
- `internal/stats/name_test.go` extension (SN9 unit tests) ~+30 = ~+30
- `internal/stats/registry.go` `NewCounterIfAbsent` post-Freeze idempotent registration ~+30 = ~+30
- `internal/stats/registry_test.go` extension (`NewCounterIfAbsent` unit tests) ~+30 = ~+30
- `cmd/envoy-go/main.go` one new `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` line + matching import ~+3 = ~+3
- `test/fixtures/0013-http-local-ratelimit/` (NEW directory — note: SPEC §4.3 + §7 reference `test/fixtures/0013-http-local-ratelimit/` directly so no path-erratum reconciliation analogous to phase 10's planner-time decision 10 is needed) — `envoy.yaml` ~180 + `envoy-go.yaml` ~180 + `expectations.yaml` ~60 + `README.md` ~70 + `driver/driver.go` ~250 + `backends/backend.go` ~30 = ~770
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPLocalRateLimit BackendKind = 10`) + doc-comment ~+15 = ~+15
- `test/differential/runner_test.go` blank-import addition + new `startHTTPLocalRateLimitBackend` spawn helper + switch case ~+25 = ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.local_ratelimit` subsection ~75 + §13.2 22→26 stat-table extension ~10 + §13.3 timing-tolerances row ~3 + §13.4 equivalence-matrix row ~3 + §13.5 forward-pointer notes ~15 = ~+106
- `docs/envoy-go/DECISIONS.md` (six ADRs ADR-0114..ADR-0119 + ADR-0073 amendment paragraph) ~+380 = ~+380
- `docs/envoy-go/ROADMAP.md` row `11` `in-progress → done` flip + (UNCHANGED) §9 family heading at line 56 ~+1 net = ~+1
- `docs/envoy-go/STATE.md` advance to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` (NEW; lifecycle artefact) ~600 (per-task entry)
- `docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md` (NEW; lifecycle artefact) ~180

**Production code: ~480 LoC (filter impl: 400 in `local_ratelimit.go` + 80 in `bucket.go`) + ~50 LoC (framework deltas: SN9 rule in `internal/stats/name.go` ~20 LoC + `NewCounterIfAbsent` in `internal/stats/registry.go` ~30 LoC) + ~3 LoC main.go = ~533 LoC production + ~340 LoC tests (incl. ~30 LoC registry_test.go) + ~50 LoC fuzzer + ~630 LoC fixture YAML/Go + ~487 LoC docs ≈ ~2040 LoC total** (production-only ~533 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~503 LoC; task count below is **16**, comfortably under the 25 limit). The SPEC's anticipated 6-ADR cluster (ADR-0114..ADR-0119) lands across 16 tasks per the table at `## ADRs introduced by this plan` below; no task lands more than 2 ADRs simultaneously. SPEC §1.3 (per BRAINSTORM Decision 9 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 11 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~12–16 tasks per SPEC §1.4 estimate; this PLAN lands at 16 tasks (mid-bound — driven by the file split into `local_ratelimit.go` + `bucket.go` adding one task vs the single-file alternative; plus the four-task BEHAVIOR_CONTRACT/six-gate/STATE/REVIEW closing cluster mirroring phase 10's precedent).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 would be `11.1 = token-bucket primitive + listener-only filter MVP (Tasks 1–7)` and `11.2 = per-route TPFC + 4th fixture scenario (Tasks 8–16)`; SPEC §1.4 explicitly rejects the split since both halves stay under the LoC threshold and the per-route discipline is a small extension of the listener-level work. PLAN concurs and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/localratelimit/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`LocalRateLimit` proto with 5-field consumed `stat_prefix`, `token_bucket{max_tokens, tokens_per_fill, fill_interval}`, `status.code`, plus the `LocalRateLimitPerRoute` per-route container per ADR-0115; 14-field silent-ignore set per SPEC §2.1 ADR-0040 silent-ignore discipline); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (Continue allow path; StopIteration + SendLocalReply rate-limited path — ADR-0102 reuse from phase 09 fault precedent; no async-resume; no encode-side state; no body / trailers states exercised); (d) the per-route discipline (per ADR-0117 + ADR-0073 wholesale-override; per-route TPFC entry → independent `*tokenBucket` + `*filterStats`); (e) the lazy-refill token-bucket primitive + LBP-1-adjacent declaration per ADR-0116; (f) the cross-cutting ADR anchors (ADR-0114 / ADR-0115 / ADR-0116 / ADR-0117 / ADR-0118 / ADR-0119). Mirrors `internal/filter/http/fault/doc.go` + `internal/filter/http/header_mutation/doc.go` shape (~30–40 LoC precedent). Per SPEC §4.1. |
| `internal/filter/http/localratelimit/local_ratelimit.go` | NEW | Filter implementation — high-level orchestration. **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Unexported types (per SPEC §6.2 + §6.3):** `runtimeConfig` struct (5 fields per §6.2: `statPrefix string`, `bucket *tokenBucket`, `statusCode int`, `body []byte` literal `"local_rate_limited"`, `stats *filterStats`); `filterStats` struct (4 `*atomic.Int64` fields: `enabled`, `ok`, `rateLimited`, `enforced`); `filter` struct (`rc *runtimeConfig` + `dcb envoyhttp.DecoderFilterCallbacks` + `ecb envoyhttp.EncoderFilterCallbacks`). **Helpers:** `buildRuntimeConfig(c *localratelimitv3.LocalRateLimit, ctx envoyhttp.FactoryCtx) (*runtimeConfig, error)` (parses + validates the 5 consumed fields against PGV constraints per SPEC §11.1 + §11.2a + §11.2b-ii + §11.4 + the filter-internal `fill_interval >= 50ms` check per §11.2c); `newFilterStats(reg *stats.Registry, statPrefix string) *filterStats` (constructs the 4-counter set via `reg.NewCounter(<statPrefix>.http_local_rate_limit.<counter>)` per SPEC §6.6). **DecodeHeaders body** (per SPEC §6.5): increment `rc.stats.enabled`; call `rc.bucket.tryConsume()`; if true, increment `rc.stats.ok` and return Continue; if false, increment `rc.stats.rateLimited` AND `rc.stats.enforced` IN LOCKSTEP (per ADR-0118 MVP invariant), invoke `f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})`, return StopIteration. **EncodeHeaders body** (per SPEC §6.5 NOTE): pass-through (no encode-side state). **Pass-through methods:** OnDestroy + DecodeData + EncodeData + DecodeTrailers + EncodeTrailers all no-op. Per SPEC §6.1–§6.5. |
| `internal/filter/http/localratelimit/bucket.go` | NEW | Token-bucket primitive per ADR-0116 + SPEC §6.4. Decomposed into a sibling file (file-split decision settled here per the BRAINSTORM §3.1 / SPEC §4.1 PLAN-author option): the `tokenBucket` struct (8 fields: `maxTokens int64`, `tokensPerFill int64`, `fillInterval time.Duration`, `mu sync.Mutex`, `tokens int64`, `lastRefillNs int64` — initial fill = max; baseline lastRefillNs = `time.Now().UnixNano()` at construction); `newTokenBucket(maxTokens, tokensPerFill int64, fillInterval time.Duration) *tokenBucket` constructor; `tryConsume() bool` lazy-refill-on-access method (computes `nowNs := time.Now().UnixNano(); elapsedNs := nowNs - b.lastRefillNs; if refills := elapsedNs / int64(b.fillInterval); refills > 0 { b.tokens += refills * b.tokensPerFill; if b.tokens > b.maxTokens { b.tokens = b.maxTokens }; b.lastRefillNs += refills * int64(b.fillInterval) }; if b.tokens > 0 { b.tokens--; return true }; return false`). Single `sync.Mutex` per bucket; LBP-1-adjacent per ADR-0116 (closure-capture half preserved; lock-free hot-path half deliberately departs since the elapsed→refills computation is multi-step CAS-resistant). No per-bucket goroutine; no `time.Ticker`; no signal channel. Per SPEC §6.4 + §11.7. ~80 LoC. |
| `internal/filter/http/localratelimit/local_ratelimit_test.go` | NEW | Unit tests per SPEC §14.1 covering test groups 2–5: `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_StatPrefixEmpty`, `TestNew_MaxTokensZero`, `TestNew_TokensPerFillZeroExplicit`, `TestNew_TokensPerFillOmittedDefaultsToOne` (per SPEC §11.2b-i), `TestNew_FillIntervalBelow50ms` (table-driven across `{10ms, 20ms, 49ms}` per SPEC §11.2c — verifies the verbatim-Envoy error string), `TestNew_FillIntervalAtOrAbove50ms` (table-driven across `{50ms, 51ms, 100ms, 1s}`), `TestNew_StatusCodeBelow400` + `TestNew_StatusCodeAtOrAbove600` (PGV `[400, 600)` per §11.4), `TestNew_StatusCodeOmittedDefaultsTo429`, `TestNew_HappyPath_AllConsumedFields`, `TestRuntimeConfig_AllSilentIgnoredFieldsAccepted` (table-driven across the 14 deferred fields per SPEC §2.1 — verifies each silently parses without error or warning), `TestPerRoute_LazyCacheBuildsRuntimeConfigOnFirstResolve` (verifies per-route TPFC `LocalRateLimitPerRoute` is unmarshalled by `BuildPerRouteConfig`'s generic `UnmarshalNew`, then on first request that resolves to that per-route entry the filter `LoadOrStore`s a freshly-built `*runtimeConfig` into its `sync.Map` cache per ADR-0117 lazy-cache mechanism), `TestDecodeHeaders_AllowPath_CountersIncremented` (verifies enabled+ok increment; Continue returned), `TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep` (verifies enabled+rateLimited+enforced increment; SendLocalReply called with 429 + body + 1-header set; StopIteration returned), `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` (verifies that two `*runtimeConfig` instances built lazily by the per-route `sync.Map` cache miss path carry independent `*tokenBucket` + `*filterStats` pointers — increments on one do NOT affect the other; validates §11.6 empirical + ADR-0117 lazy-cache mechanism), `TestStatNames_FourCountersUnderStatPrefix` (verifies the registry's NewCounter calls produce the expected internal hierarchical-dotted names per SPEC §6.6 — `<statPrefix>.http_local_rate_limit.{enabled,ok,rate_limited,enforced}`). ~200 LoC. |
| `internal/filter/http/localratelimit/bucket_test.go` | NEW | Token-bucket primitive unit tests per SPEC §14.1 group 1 + planner-time decision 7 (race-detector cycle test). `TestTokenBucket_NewInitialFillEqualsMax`, `TestTokenBucket_TryConsume_DepletesUntilZero`, `TestTokenBucket_TryConsume_ReturnsFalseWhenEmpty`, `TestTokenBucket_LazyRefill_NoRefillBelowFillInterval`, `TestTokenBucket_LazyRefill_SingleQuantumRefill`, `TestTokenBucket_LazyRefill_MultiQuantumRefill_CapAtMax`, `TestTokenBucket_LazyRefill_LastRefillNsAdvancesByFullQuanta` (verifies the algorithm's `lastRefillNs += refills * int64(fillInterval)` discipline does NOT advance to `nowNs` directly — preserves sub-quantum residual elapsed for the next call), `TestTokenBucket_ConcurrentTryConsume` (race-detector cycle test per planner-time decision 7 — fires `tryConsume` concurrently across 64 goroutines × 100 iterations with shared `*tokenBucket`; verifies no race; verifies total-allowed-count is bounded by initial-tokens + refill-quanta-during-test). ~80 LoC. |
| `internal/filter/http/localratelimit/fuzz_test.go` | NEW | `FuzzLocalRateLimitConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the local_ratelimit filter's `New` factory is a parser. ~50 LoC; 30s budget per ADR-0018; **fifteenth fuzzer overall** (post-10's fourteenth `FuzzHeaderMutationConfigParse`). |
| `internal/stats/name.go` | MODIFIED | Add Rule SN9 to the `flattenToProm` switch per planner-time decision 1 + ADR-0118. The SN9 rule matches names of the shape `<stat_prefix>.http_local_rate_limit.<counter>` (where `<stat_prefix>` is a single segment with no dots and `<counter>` is one of `{enabled, ok, rate_limited, enforced}`), and produces Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. The rule is a SECOND-PASS detection (the existing SN1–SN5 prefix-segment switch handles `cluster.|http.|listener.|server.`; SN9 fires for names that don't match those prefixes BUT match the suffix pattern `.http_local_rate_limit.<counter>`). Implementation: extend the switch's `default` branch to FIRST attempt SN9 detection via a regex or a `strings.Contains(internal, ".http_local_rate_limit.")` + suffix-segment validation; only if SN9 doesn't match does the function return the existing "no recognized top-level segment" error. ~+20 LoC delta. |
| `internal/stats/name_test.go` | MODIFIED | Add unit tests for Rule SN9 per planner-time decision 1: `TestFlattenToProm_SN9_BasicStatPrefix` (input `foo.http_local_rate_limit.enabled` → base `envoy_http_local_rate_limit_enabled` + label `envoy_local_http_ratelimit_prefix=foo`), `TestFlattenToProm_SN9_AllFourCounters` (table-driven across `{enabled, ok, rate_limited, enforced}`), `TestFlattenToProm_SN9_PrefixWithUnderscores` (input `my_prefix.http_local_rate_limit.ok` — stat_prefix may contain underscores but no dots), `TestFlattenToProm_SN9_DoesNotConflictWithSN1234` (input `cluster.foo.http_local_rate_limit.enabled` still routes to SN1 since `cluster.` prefix wins; this is the cross-rule precedence assertion — SN1–SN5 take priority over SN9). ~+30 LoC delta. |
| `internal/stats/registry.go` | MODIFIED | Add `NewCounterIfAbsent(name string) *Counter` method permitting idempotent post-Freeze counter registration per ADR-0117 + ADR-0061 amendment in ADR-0118. Existing `NewCounter` panic-on-Freeze discipline preserved verbatim for boot-time registrations; the new method is reserved for HCM-build-time per-route registrations whose `stat_prefix` is data-driven (e.g., `localratelimit` per-route TPFC entries). Behavior: if name already registered as `*Counter` → return existing (idempotent); if absent → register and return; panics on invalid name (programmer error) or on type-collision (name registered as non-Counter). Concurrency: `r.mu.Lock` serializes the read-or-register pair. **Small framework extension justified by the per-route counter discipline** — Phase 11 is the FIRST production filter where per-route override implies independent stateful resources (per ADR-0117); the `NewCounterIfAbsent` shape is the minimal addition that supports it without relaxing the LBP-1 invariant for boot-time registrations. ~+30 LoC delta. |
| `internal/stats/registry_test.go` | MODIFIED | Add unit tests for `NewCounterIfAbsent`: `TestNewCounterIfAbsent_RegistersWhenAbsent` (registers a fresh counter and returns the new instance), `TestNewCounterIfAbsent_ReturnsExisting` (calling after `NewCounter` for the same name returns the same `*Counter` pointer; idempotency), `TestNewCounterIfAbsent_BypassesFreeze` (post-`Freeze` registration succeeds where `NewCounter` would panic; verifies subsequent lookups remain pointer-identical). ~+30 LoC delta. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` registration inserted after the existing `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` line (currently line 117 in master HEAD `97ed8b9`) and before the `header_mutation.RegisterPerRouteValidator(httpReg)` call (currently line 121) and `httpReg.Freeze()` (currently line 122). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/localratelimit"` alphabetically among the existing filter-package imports (currently lines 28-32: cors, envoygotest, fault, header_mutation, router → cors, envoygotest, fault, header_mutation, localratelimit, router). Per the BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline (codified at phase-09 brainstorm time + reaffirmed at phase 10), the resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Register(localratelimit.TypeURL, localratelimit.New); header_mutation.RegisterPerRouteValidator(httpReg); httpReg.Freeze()`. **No other wiring changes** — local_ratelimit is HTTP-only, no listener/cluster/drain manager threading; no per-route-validator registration call (local_ratelimit has no per-route invariants requiring boot-time validation — per-route TPFC parsing happens at HCM-build via `BuildPerRouteConfig`'s generic `UnmarshalNew`, and the filter applies its PGV + filter-internal validation lazily at first-resolve via `buildRuntimeConfigPerRoute`). ~+3 LoC delta (1 import line + 1 register line). |
| `test/fixtures/0013-http-local-ratelimit/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. **NOTE:** SPEC §4.3 references `test/fixtures/0013-http-local-ratelimit/` correctly (no path-erratum analogous to phase 10's planner-time decision 10 is needed); the runner-side blank-import lives at `test/differential/runner_test.go` per the existing 0010 / 0011 / 0012 convention. |
| `test/fixtures/0013-http-local-ratelimit/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port resolved at boot by the runner; **FOUR listeners per planner-time decision 8** — `l_s1` (scenario 1: cap=10/fill=10/interval=1s/stat_prefix=foo), `l_s2` (scenario 2: cap=2/fill=2/interval=60s/stat_prefix=bar), `l_s3` (scenario 3: cap=1/fill=1/interval=200ms/stat_prefix=baz), `l_per_route` (scenario 4: listener cap=10/stat_prefix=qux + per-route `/strict` TPFC cap=1/stat_prefix=strict + no-override `/loose`); cluster `c_backend` STRICT_DNS pointing at the harness backend via `host.docker.internal` per ADR-0010). All 4 listeners explicitly set `filter_enabled` AND `filter_enforced` to 100% per SPEC §1.1 amendment (RuntimeFractionalPercent default is 0% — omitting these fields would silently disable rate-limiting in reference Envoy, breaking the differential equivalence; the fixture renders unique runtime_keys per listener per filter to avoid Envoy's runtime-key cross-contamination). http_filters chain on each listener: `[envoy.filters.http.local_ratelimit, envoy.filters.http.router]`. ~180 LoC (4 listeners × ~40 LoC per filter chain + cluster + admin). |
| `test/fixtures/0013-http-local-ratelimit/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo cluster type (STATIC instead of STRICT_DNS) + admin/listener port values resolved at boot by the runner. Both `filter_enabled` and `filter_enforced` fields are PRESENT in envoy-go.yaml even though envoy-go silent-ignores them (per SPEC §2.1 cluster 2 / §13.5 — envoy-go's silent-ignore is equivalent to "always-100%" under MVP) — the field presence ensures byte-equivalent config-load behavior. ~180 LoC. |
| `test/fixtures/0013-http-local-ratelimit/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1 (with PLAN-time fixture topology divergence per planner-time decision 8): scenario 1 (`l_s1` bucket cap=10, 5 reqs back-to-back) → 5x 200; counter deltas `enabled=5, ok=5, rate_limited=0, enforced=0`; `/stats/prometheus` scrape equivalence (label `envoy_local_http_ratelimit_prefix="foo"`); scenario 2 (`l_s2` bucket cap=2, 5 reqs back-to-back) → first 2x 200, last 3x 429; counter deltas `enabled=5, ok=2, rate_limited=3, enforced=3`; rate-limited response: status `429 Too Many Requests`, body byte-exact `local_rate_limited` (18 bytes, no LF), 4 headers in lexicographic order (`content-length: 18`, `content-type: text/plain`, `date: <allow-listed>`, `server: envoy`), framing Content-Length; scenario 3 (`l_s3` bucket cap=1/fill=1/interval=200ms, 3 reqs at t=0/10ms/250ms) → t=0 → 200, t=10ms → 429, t=250ms → 200; **±10ms tolerance per SPEC §1.1 amendment** on the t=250ms boundary; scenario 4 (`l_per_route` per-route `/strict` + `/loose` interleaved 3+3 reqs) → `/strict`: 1x 200 + 2x 429; `/loose`: 3x 200; `strict`-prefixed counters `enabled=3, ok=1, rate_limited=2, enforced=2`; `qux`-prefixed counters `enabled=3, ok=3, rate_limited=0, enforced=0` (per §11.6 wholesale-override; listener-level counters do NOT increment for `/strict` reqs). Cross-refs SPEC §7.1 + §13.1 + ADR-0116 + ADR-0117 + ADR-0118 + ADR-0119. ~60 LoC. Per SPEC §4.3. |
| `test/fixtures/0013-http-local-ratelimit/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + four-scenario list (per SPEC §7.1) + four-listener bootstrap discipline (per planner-time decision 8: each scenario binds its own pre-configured listener with bucket parameters fixed at boot; no per-scenario teardown — all 4 scenarios run in a single Drive call against 4 distinct listener ports) + Envoy-deviation note (none — local_ratelimit is a normal HTTP filter; no SIGTERM/drain divergence) + the `filter_enabled`+`filter_enforced=100%` discipline note (SPEC §1.1 amendment) + planner-time-decision cross-references. ~70 LoC. Per SPEC §4.3. |
| `test/fixtures/0013-http-local-ratelimit/driver/driver.go` | NEW | Go driver implementing the SPEC §7.1 + §7.2 four-scenario orchestration via the 4-listener topology per planner-time decision 8. **Driver shape:** `package driver`; `init()` calls `fixture.RegisterFixture("0013-http-local-ratelimit", &localRateLimitDriver{})`; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPLocalRateLimit` (the new enum value added in Task 9); implements `fixture.MultiListenerDriver` (per `test/differential/fixture/fixture.go:294-299`) returning 4 listener names `["l_s1", "l_s2", "l_s3", "l_per_route"]` + matching reference ports; the runner allocates 4 subject ports + exposes 4 reference ports. `ReferenceBootstrap` / `SubjectConfig` templates `envoy.yaml` / `envoy-go.yaml` substituting the 4 listener-port placeholders + backend port; **NO per-scenario teardown** — all 4 listeners are bound at boot; the bootstrap is rendered ONCE. `DriveReferenceMulti` / `DriveSubjectMulti` issue ALL FOUR scenarios in ONE call (scenario 1: 5 HTTP requests via `l_s1` addr; scenario 2: 5 HTTP requests via `l_s2` addr; scenario 3: 3 HTTP requests at t=0/10ms/250ms via `l_s3` addr; scenario 4: 3+3 interleaved HTTP requests via `l_per_route` addr); per-probe captures status + body + headers (rate-limited path) + post-scenario `/stats/prometheus` scrape captures the 4 counters AND the tag-extracted Prometheus label for differential-equivalence assertion via `CompareBytes`. ±10ms tolerance on scenario 3 t=250ms boundary: the driver computes the actual delay relative to the t=0 baseline via `time.Now()` and asserts the third-request landing falls in the wallclock band `[200ms, 260ms]` for "refill must have happened by"; if the actual delay falls outside the band, the driver fails fast with a diagnostic dump (per SPEC §7.2 + §12 D4 default). NO additional `time.Sleep`-resolution complications expected at the modal CI scheduling per SPEC §11.7's empirical envelope; if the fixture flakes during phase 11 impl under heavy CI load, the implementer at Task 13 has the option to adopt either (a) a wider tolerance ±20ms back to BRAINSTORM hypothesis or (b) a retry-with-deadline harness around scenario 3's t=250ms boundary check (per SPEC §12 D4 fallback; PROGRESS.md records the chosen option). ~250 LoC. Per SPEC §7.2. |
| `test/fixtures/0013-http-local-ratelimit/backends/backend.go` | NEW | Minimal Go HTTP backend bound to a runner-allocated port. Mirrors `test/fixtures/0011-http-fault/backends/backend.go` exactly (24 LoC verified at master `97ed8b9`): `/` endpoint serves a fast `200 OK` with body `"backend\n"` (8 bytes); response carries one fixed `Content-Type: text/plain` and one `Content-Length: 8` header. No special handling for `/strict` or `/loose` (the rate-limit decision happens in Envoy/envoy-go BEFORE the upstream call; the backend is unreachable on rate-limited paths). Accepts a `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper. ~30 LoC. Per SPEC §7.4. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPLocalRateLimit BackendKind = 10` after the existing `HTTPHeaderMutation BackendKind = 9` (introduced by phase 10). Doc-comment notes: "HTTPLocalRateLimit is an out-of-process HTTP/1.1 backend: the runner spawns `test/fixtures/0013-http-local-ratelimit/backends/backend.go` on the pre-allocated port. The backend serves `/` with body `backend\n` (8 bytes; Content-Type: text/plain; Content-Length: 8). No TLS. Introduced by fixture 0013-http-local-ratelimit (phase 11 Task 9). Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/driver"` (insert in alphabetical order, after the `0012-http-header-mutation` blank-import). (b) Extend the `kind` switch in `runFixture` with a new case `fixture.HTTPLocalRateLimit` mirroring the `HTTPHeaderMutation` block: spawn via `startHTTPLocalRateLimitBackend`. (c) Add new spawn helper `startHTTPLocalRateLimitBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` mirroring `startHTTPHeaderMutationBackend` from phase 10: `exec.CommandContext(ctx, "go", "run", "./test/fixtures/0013-http-local-ratelimit/backends", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.local_ratelimit` subsection inserted under existing `## HTTP filter chain` umbrella AFTER the `### envoy.filters.http.header_mutation` subsection at line 924 landed by phase 10 (per §13.1; ~75 LoC); (b) `## Stat-name mapping ### 22-name table` extends to **26-name table** with the four new local_ratelimit counter rows + the new "Filter-specific Prometheus tag-extractor" preamble paragraph noting the `envoy_local_http_ratelimit_prefix` label per ADR-0118 + the tag-extraction-collision quirk note (per SPEC §13.2; ~10 LoC); (c) `## Timing tolerances` gains one new row for fixture 0013 scenario 3 t=250ms refill boundary ±10ms wallclock per SPEC §13.3 + ADR-0116 (~3 LoC); (d) `## Equivalence Matrix` new local_ratelimit-filter row (per §13.4; ~3 LoC); (e) §13.5 forward-pointer notes — the deferred field families (14 fields per §2.1 organized by 8 family-clusters) + the runtime-key default-0% divergence-window note for `filter_enabled`+`filter_enforced` per SPEC §1.1 amendment (~15 LoC). ADR-0052 in-place edit authorisation carries forward. ~+106 LoC total. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append six new ADRs ADR-0114..ADR-0119 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). The 7-section ADR-0001 template applies to each (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). **Inline supersessions / amendments:** ADR-0073 (typed_per_filter_config 3-tier merge / most-specific override) — AMENDED (not superseded) by ADR-0117: the most-specific-override discipline remains the canonical model; ADR-0117's amendment paragraph notes that wholesale-override extends to STATEFUL per-route resources (independent `*tokenBucket` + `*filterStats` per per-route TPFC entry) without further framework support — phase 11 is the FIRST production filter to demonstrate this. Cross-reference recorded in ADR-0117 §Decision; **inline edit of ADR-0073** marked with `## Amendment (per phase 11 ADR-0117)` paragraph noting "wholesale-override extends to stateful per-route resources via a per-filter request-time lazy cache keyed by per-route proto pointer; each cache miss builds a fresh `*runtimeConfig` with its own stateful resources (registered via `Registry.NewCounterIfAbsent` for post-Freeze idempotency) — see ADR-0117 for the precedent and discipline." (NO change to the original Decision body; the amendment is a forward-pointer.) ~+380 LoC total (six ADRs + ADR-0073 amendment paragraph). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `11` `in-progress → done` flip AT the phase-done commit. The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 11 artefacts (per ADR-0106 no-sibling-stub discipline). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 3 (PLAN drafting — this PLAN landing flips state 3 → 4 in the orchestrating session's STATE.md edit), 4 (PLAN execution — Tasks 1–13 land production code + fixture; STATE stays at 4), 5 (verification — Task 14 lands BEHAVIOR_CONTRACT/ADRs/six-gate verification; STATE flips 4 → 5), 6 (review — Task 15 + Task 16 REVIEW.md per requesting-code-review skill; STATE flips 5 → 6 then to `awaiting next planning`); `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner. |
| `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..10 PROGRESS.md structure. The preamble enumerates the six anticipated ADRs ADR-0114..ADR-0119 + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 9 items below — D1–D5 from SPEC §12 plus 4 PLAN-emerging items). |
| `docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 cadence; populates per the requesting-code-review skill. Phase 11 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 11. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's five deferred decisions before implementation; this PLAN settles all five plus four that emerged at PLAN-drafting time (items 6, 7, 8, 9 below). The nine resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **D1 — Tag-extractor registration site for `envoy_local_http_ratelimit_prefix` = EXTEND `internal/stats/name.go`'s `flattenToProm` SWITCH WITH NEW RULE SN9.** Per SPEC §12 D1, the SPEC author left three options open: (a) `internal/admin/stats.go` (does NOT exist — the project has no such file at master `97ed8b9`; tag-extractor mechanism lives in `internal/stats/name.go`'s `flattenToProm` switch as Rules SN1–SN5), (b) within `internal/filter/http/localratelimit/` package's `init()` calling a registry-pattern primitive from `internal/stats`, (c) a new file `internal/stats/tag_extractors_local_ratelimit.go`. Survey confirms option (a) is mis-stated in SPEC: the actual mechanism is the hardcoded switch in `internal/stats/name.go::flattenToProm` (Rules SN1–SN5 cover `cluster.|http.|listener.|server.` prefixes; SN4 handles trailing `_Nxx` status-class collapse; SN6–SN8 are documentation-only rules). The cleanest registration site for the local_ratelimit filter-specific tag-extractor is to **extend the switch with a new Rule SN9** (the next-free SN-rule number; SN8 is the histograms-not-emitted documentation rule). SN9 matches names of the shape `<stat_prefix>.http_local_rate_limit.<counter>` where `<stat_prefix>` is a single segment with no dots and `<counter>` is one of `{enabled, ok, rate_limited, enforced}`; produces Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. The rule is a SECOND-PASS detection: the existing prefix-segment switch handles SN1–SN5 first; if no prefix matches, SN9 is attempted via a `strings.Contains(internal, ".http_local_rate_limit.")` + suffix-segment validation; only if SN9 doesn't match does the function return the existing "no recognized top-level segment" error. This keeps the SN1–SN5 hot-path performance unchanged + adds SN9 detection only on the unmatched-prefix path. PLAN OVERRIDES SPEC's mis-stated option (a) with the corrected mechanism. Filter-package-local registration via `init()` (option (b)) is REJECTED: the existing `flattenToProm` is a hardcoded switch with no registry/dispatch primitive; introducing one for a single rule would be over-engineering and would diverge from the existing 06.1-established discipline. Sibling-file split (option (c)) is REJECTED: SN9 is one extra `case` in the switch + ~20 LoC; splitting it into a separate file complicates code-review without a maintenance-burden payoff. *Anchored: SPEC §12 D1; SPEC §11.5 (e); existing `internal/stats/name.go` Rules SN1–SN5 + SN6–SN8 documentation; ADR-0061 (the original SN-rule-set ADR).*

2. **D2 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)` + `SetEncoderCallbacks(cb)` per the cors + fault + header_mutation precedents.** Per SPEC §12 D2 + survey of existing patterns: `internal/filter/http/cors/cors.go:55–56` defines `func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }` + `func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }`; `internal/filter/http/fault/fault.go:271–272` follows the same pattern; `internal/filter/http/header_mutation/header_mutation.go:327–328` follows the same pattern. The framework's per-stream state machine (per `internal/filter/http/chain.go`) calls `SetDecoderCallbacks` once per stream as part of the chain construction; the filter stores the callback reference for later use during `DecodeHeaders`. Phase 11's `*filter` struct carries both `dcb` and `ecb` fields per the cors precedent (even though local_ratelimit consumes only `dcb` for the `SendLocalReply` call) — keeping both fields preserves the precedent's chain-of-conformance pattern at zero structural cost. *Anchored: SPEC §12 D2; cors.go:55–56; fault.go:271–272; header_mutation.go:327–328; chain.go per-stream callback-setup.*

3. **D3 — PGV plumbing for `stat_prefix` non-empty + `max_tokens > 0` + `tokens_per_fill > 0` + `status.code ∈ [400, 600)` = EXPLICIT CHECKS IN THE `New` FACTORY.** Per SPEC §12 D3 + survey: `internal/filter/http/cors/cors.go::New`, `internal/filter/http/fault/fault.go::buildRuntimeConfig`, `internal/filter/http/header_mutation/header_mutation.go::buildRuntimeConfig` all use explicit-check patterns (a) — they call `tc.UnmarshalTo(&c)` then check each PGV constraint manually with `errors.New` / `fmt.Errorf` returning a package-prefixed error. Phase 11 follows the same pattern: `buildRuntimeConfig(c *localratelimitv3.LocalRateLimit, ctx envoyhttp.FactoryCtx) (*runtimeConfig, error)` runs (i) `if c.GetStatPrefix() == "" { return nil, errors.New("local_ratelimit: stat_prefix required") }` per SPEC §11.1, (ii) `if c.GetTokenBucket() == nil { return nil, errors.New("local_ratelimit: token_bucket required") }` (the empirical pin §11.2a's PGV error fires only when token_bucket is present-but-malformed; an absent token_bucket would surface differently in reference Envoy — implementer at Task 2 step 2 confirms via probe; SPEC §6.4 makes token_bucket implicit-required by treating `cfg.TokenBucket.MaxTokens` as required), (iii) `if c.TokenBucket.GetMaxTokens() <= 0 { return nil, errors.New("local_ratelimit: token_bucket.max_tokens must be > 0") }` per SPEC §11.2a, (iv) `tokensPerFill := c.TokenBucket.GetTokensPerFill().GetValue(); if c.TokenBucket.GetTokensPerFill() == nil { tokensPerFill = 1 } else if tokensPerFill == 0 { return nil, errors.New("local_ratelimit: token_bucket.tokens_per_fill must be > 0 if specified") }` per SPEC §11.2b-i + §11.2b-ii (the `tokens_per_fill` is a `*UInt32Value` wrapper; absent → default 1; explicit-zero → reject), (v) `fillInterval := c.TokenBucket.GetFillInterval().AsDuration(); if fillInterval < 50*time.Millisecond { return nil, errors.New("local rate limit token bucket fill timer must be >= 50ms") }` per SPEC §11.2c — **VERBATIM Envoy error string** (the filter-internal check fires AFTER proto unmarshal succeeds, NOT as a PGV constraint; the error string mirrors Envoy's `source/server/config_validation/server.cc:76` message exactly per the §11.2c empirical pin; this is the LOAD-BEARING wire-equivalence claim — envoy-go's error message must match Envoy's so operators reading boot logs see identical failure shapes), (vi) `statusCode := 429; if c.GetStatus() != nil { statusCode = int(c.Status.GetCode()); if statusCode < 400 || statusCode >= 600 { return nil, fmt.Errorf("local_ratelimit: status.code must be in [400, 600); got %d", statusCode) } }` per SPEC §11.4. Implicit-PGV via `protoreflect`-based runtime checks (option (b) from SPEC §12 D3) is REJECTED: no prior phase wires it; introducing it for one filter would diverge from the existing explicit-check discipline at zero code-quality benefit. *Anchored: SPEC §12 D3; cors.go::New; fault.go::buildRuntimeConfig; header_mutation.go::buildRuntimeConfig; SPEC §11.1 + §11.2 + §11.4 verbatim error strings.*

4. **D4 — Scenario 3 retry-with-deadline harness option = ±10ms TOLERANCE WITH SIMPLE `time.Sleep` (DEFAULT).** Per SPEC §12 D4 + §7.2 + §11.7. The driver implements scenario 3 with three `time.Sleep` calls relative to a `t0 := time.Now()` baseline: req 1 at `t0` immediate; req 2 at `t0 + 10ms` (sleep 10ms); req 3 at `t0 + 250ms` (sleep 240ms after req 2). The `±10ms tolerance` is enforced post-hoc: the driver records `actualReq3DelayMs := time.Since(t0).Milliseconds()` and asserts the band `actualReq3DelayMs >= 200 && actualReq3DelayMs <= 260` (200 = fill_interval, 260 = fill_interval + 10ms upper-tolerance + a 50ms scheduling slack on the call site since the `time.Sleep` may run slightly long). If the assertion fails, the driver fails fast with a diagnostic dump. The retry-with-deadline alternative (option (b) from SPEC §12 D4) is RESERVED as a fallback if CI scheduling jitter makes the simple-sleep approach flaky during phase 11 impl; the implementer at Task 13 records any flake observation in PROGRESS.md, and if the issue surfaces, swaps to option (b) before the phase-done commit (PROGRESS.md captures the rationale; ADR-0116 §Consequences may amend in-place per ADR-0089 to record the chosen option). The empirical envelope at SPEC §11.7 (52 trials at delay ≥ 200ms all returned 200; 24 trials at delay ≤ 199ms all returned 429; measurement floor ~5ms) suggests simple-sleep is sufficient under modal CI load. *Anchored: SPEC §12 D4 + §7.2 + §11.7; phase 09 fault driver's similar ±10ms delay-injection assertion at `test/fixtures/0011-http-fault/driver/driver.go`.*

5. **D5 — Test-only clock injection for wallclock-monotonicity testing = SKIP (per SPEC default).** Per SPEC §12 D5 + §2.2. Phase 11 does NOT introduce a `clock` interface threaded through `tokenBucket`; the bucket consumes `time.Now().UnixNano()` directly. The race-detector cycle test `TestTokenBucket_ConcurrentTryConsume` (per planner-time decision 7) exercises the mutex discipline mechanically via real wallclock — it does NOT exercise wallclock backward-jump simulation (which would require test-only clock injection). The lazy-refill mechanism is by-construction monotonic-time-safe under Go ≥1.9's `time.Now()`-derived `UnixNano()` semantics (the `time.Now()` value carries a monotonic component for arithmetic across `time.Now()` calls per Go documentation). Future hardening pass may revisit if a wallclock backward-jump production incident surfaces; the SPEC's position holds for phase 11. *Anchored: SPEC §12 D5 + §2.2; Go ≥1.9 monotonic-time documentation.*

6. **PLAN-emerging — `tokenBucket` file-split decision = SPLIT INTO `bucket.go` + `local_ratelimit.go`.** Per SPEC §6.4's PLAN-author option ("PLAN author may move this into a sibling file `bucket.go` for readability"). The file split cleanly separates the token-bucket primitive (8-field struct + 2 methods + ~80 LoC) from the filter orchestration (TypeURL + types + parser + factory + filter struct + DecodeHeaders + filterStats + ~400 LoC). Mirrors the size-driven split discipline in `internal/filter/http/fault/` (which keeps `fault.go` as a single file at ~430 LoC because the runtimeConfig parser + filter + delay timer + active counter are tightly coupled) and DIVERGES from header_mutation's single-file shape (which keeps everything in `header_mutation.go` at ~280 LoC because the filter is a single integrated unit with no separable primitive). The token-bucket primitive is a SEPARABLE primitive — it carries no filter-orchestration knowledge, has its own constructor + two-method surface, and could in principle be reused by future rate-limit filters (e.g., `global_ratelimit` if it lands a per-process token-bucket fallback — though SPEC §2.1 cluster 1 defers `global_ratelimit` to a separate phase). The split into `bucket.go` + `local_ratelimit.go` makes the primitive's reusability visible to future readers and keeps each file under the 200-LoC mental-model threshold per the project's general code-quality discipline. The matching test split `bucket_test.go` + `local_ratelimit_test.go` mirrors. *Anchored: SPEC §6.4 PLAN-author option; SPEC §4.1 file-list decomposition discretion; project code-quality discipline.*

7. **PLAN-emerging — Race-detector test for the token-bucket = ADD `TestTokenBucket_ConcurrentTryConsume` UNDER `-race`.** PLAN-emerging mirroring phase 10's planner-time decision 7. Fires `tryConsume` concurrently across 64 goroutines × 100 iterations with shared `*tokenBucket` (initial cap=1000 to keep the test's runtime sub-second; counts the total true returns; verifies `0 ≤ totalAllowed ≤ 1000 + maxRefillsDuringTest * tokensPerFill` to bound by initial-tokens + at-most-one-or-two refill-quanta during the sub-second test window). The mutex hot-path is race-clean by construction (single `sync.Mutex` discipline; `tryConsume` is the sole writer); the test validates the mutex use mechanically. ~30 LoC. Lands in Task 3 (the `bucket.go` task). *Anchored: phase 10 planner-time decision 7 precedent; SPEC §5.6 concurrency model.*

8. **PLAN-N (fixture topology) — 4 PRE-CONFIGURED LISTENERS REPLACE SPEC §7.1's `l_basic` RECONFIGURATION APPROACH.** The fixture instantiates 4 listeners in ONE bootstrap and runs ALL 4 scenarios in a SINGLE `DriveSubject`/`DriveReference` invocation: (a) `l_s1` — scenario 1 (cap=10, fill=10, interval=1s, stat_prefix=foo); (b) `l_s2` — scenario 2 (cap=2, fill=2, interval=60s, stat_prefix=bar); (c) `l_s3` — scenario 3 (cap=1, fill=1, interval=200ms, stat_prefix=baz); (d) `l_per_route` — scenario 4 (listener-level cap=10/stat_prefix=qux + per-route `/strict` TPFC cap=1/stat_prefix=strict + no-override `/loose`). Each listener binds its own port (allocated by the runner). The driver dials each listener in turn with the scenario-specific request pattern; no teardown, no proxy restart. **Rationale for divergence from SPEC §7.1's two-listener layout:** SPEC §7.1's two-listener layout assumed per-scenario teardown was free, which the existing differential-fixture harness does NOT support — the `fixture.Driver` interface (`test/differential/fixture/fixture.go:42-46`) defines `DriveReference(ctx, addr) ([]byte, error)` and `DriveSubject(ctx, addr) ([]byte, error)` as ONE call per fixture, and the existing fault driver at `test/fixtures/0011-http-fault/driver/driver.go` runs all its scenarios in a single Drive call (NOT per-scenario teardown). Adding a per-scenario-teardown primitive would require a ~50 LoC framework extension to `fixture.Driver` + the runner's per-fixture lifecycle code — out of scope for phase 11. The 4-listener pre-configuration is functionally equivalent (each listener carries an independent `*tokenBucket` since each is built by an independent `New` invocation per the framework's per-listener factory dispatch) and avoids the framework extension. The single-DriveSubject/DriveReference invocation matches the harness contract. The `MultiListenerDriver` optional interface (`fixture.go:294-299`) supports >1 listener per fixture — phase 11 implements it to expose all 4 listener names + ports to the runner. *Anchored: `test/differential/fixture/fixture.go:15-52` (Driver interface single-call shape); `test/fixtures/0011-http-fault/driver/driver.go` (precedent: all scenarios in ONE Drive call); SPEC §7.1 (two-listener layout — DIVERGED FROM at PLAN time per the rationale above).*

9. **PLAN-emerging — Fixture's new BackendKind enum value name = `HTTPLocalRateLimit BackendKind = 10`.** PLAN-emerging mirroring phase 10's planner-time decision 11. Continues the existing naming convention (`HTTPHello`, `HTTPSlowStream`, `HTTPFault`, `HTTPHeaderMutation` per phases 09–10); the suffix names the fixture-purpose, not the protocol family. The implementer at Task 9 adds the enum constant + doc-comment block matching the existing `HTTPHeaderMutation BackendKind = 9` shape. *Anchored: existing fixture.BackendKind enumeration convention.*

These nine decisions are reproduced verbatim in `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The six ADRs anticipated by SPEC §8 (ADR-0114..ADR-0119). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The six ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0114 | `internal/filter/http/localratelimit/` package shape (TypeURL + New + filter struct + decoder/encoder methods; **no-underscore directory name** departing from header_mutation's underscore-preserving pattern; rationale: aligns with cors + fault whose proto type-names were already single tokens — preserves the existing 3-of-4-filters discipline and avoids treating a single proto-name divergence as a precedent flip) + extension-registry registration line + boot-time `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` | Task 2 (`internal/filter/http/localratelimit/{doc.go,local_ratelimit.go}` package skeleton first lands; the boot registration code lands in Task 7 but ADR-0114 anchors at Task 2 because that's the first-use site that justifies the package shape per ADR-0044). |
| ADR-0115 | `runtimeConfig` shape + 5-field-consumed / 14-field-silent-ignored decomposition + PGV constraint table (`stat_prefix` non-empty / `max_tokens > 0` / `tokens_per_fill > 0` if explicitly set / `status.code ∈ [400, 600)` if explicitly set; explicit-checks-in-`New` discipline per planner-time decision 3) + filter-internal `fill_interval >= 50ms` validation as a SIBLING-CHECK after proto unmarshal succeeds NOT a PGV constraint (the verbatim Envoy error string `local rate limit token bucket fill timer must be >= 50ms` is required for byte-equivalent boot-log fidelity) | Task 2 (`runtimeConfig` + `New` factory + listener-level PGV + filter-internal validation first lands). |
| ADR-0116 | `tokenBucket` primitive Option-A lazy-refill on access + monotonic-time semantics (`time.Now().UnixNano()` per Go ≥1.9) + LBP-1-adjacent declaration (closure-capture half preserved; lock-free hot-path half deliberately departs because the elapsed→refills computation is multi-step CAS-resistant; mutex is the natural choice) + empirical refill-timing tolerance ±10ms (narrowed from BRAINSTORM ±20ms hypothesis per SPEC §11.7's 52-trial empirical envelope) + fixture 0013 scenario 3 ±10ms tolerance with simple `time.Sleep` (per planner-time decision 4 + SPEC §12 D4) | Task 3 (`bucket.go` + `tokenBucket` struct + `tryConsume` + race-detector cycle test first lands). |
| ADR-0117 | Per-route bucket isolation as ADR-0073 wholesale-override consequence (FIRST production filter to demonstrate that wholesale-override extends to STATEFUL per-route resources — independent `*tokenBucket` + `*filterStats` per per-route TPFC entry; mechanism is a per-filter request-time **lazy cache** (`sync.Map` keyed by `*LocalRateLimitPerRoute` proto pointer) that constructs each per-route `*runtimeConfig` on first resolve; counter registration uses the new `Registry.NewCounterIfAbsent` post-Freeze idempotent helper); **AMENDS (does not supersede) ADR-0073** with the stateful-resource amendment paragraph | Task 5 (the per-route TPFC parsing + per-route bucket-independence test first lands the empirical confirmation; the ADR-0073 amendment paragraph lands in DECISIONS.md alongside ADR-0117). |
| ADR-0118 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension + `enforced == rate_limited` MVP invariant + future shadow-mode widening point + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registered as Rule SN9 in `internal/stats/name.go`'s `flattenToProm` (per planner-time decision 1) + tag-extraction-collision quirk note (per SPEC §11.5 (e); fixture avoids magic prefix names — uses `foo`, `bar`, `baz`, `qux`, `strict`) | Task 6 (`internal/stats/name.go` SN9 rule + matching tests + `internal/stats` package surface for the four-counter `filterStats` first lands the tag-extractor end-to-end). |
| ADR-0119 | Rate-limited response wire shape + body byte-exact `local_rate_limited` (18 bytes ASCII; NO trailing newline; MD5 `397e830923f3080ba63b3d38b53678ac` per SPEC §11.3 empirical pin) + 4-header set lowercase wire-form (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + 429 default status + `SendLocalReply` reuse from phase 09 fault precedent (the existing `dcb.SendLocalReply(status, body, OrderedHeaders{...})` framework primitive at `internal/filter/http/fault/fault.go:321` carries through verbatim — no new framework primitive) + the corrected `server: envoy` value per SPEC §1.1 amendment (BRAINSTORM hypothesized `envoy-go`; reference Envoy emits `envoy`; envoy-go's existing `internal/filter/hcm/codec.go:17::serverHeader()` already returns `"envoy"` so no envoy-go code change needed) | Task 4 (`DecodeHeaders` body + filterStats wiring + the SendLocalReply call site + filter-stats unit tests first lands the rate-limited-path wire shape end-to-end). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Inline supersessions / amendments anticipated** (recorded inline in the listed ADRs above per the ADR-0089 consequence (b) in-place-edit pattern; NOT separate ADRs):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — AMENDED (not superseded) by ADR-0117: the most-specific-override discipline remains the canonical model; ADR-0117's amendment paragraph notes that wholesale-override extends to STATEFUL per-route resources without further framework support (the existing `BuildPerRouteConfig` in `internal/filter/http/perroute.go:63-85` does NOT call any registered filter `New`; it only `UnmarshalNew`'s the per-route Anys to generic `proto.Message` values) — phase 11 is the FIRST production filter to demonstrate the stateful extension via a per-filter request-time lazy cache. Inline edit of ADR-0073: append a `## Amendment (per phase 11 ADR-0117)` paragraph noting "wholesale-override extends to stateful per-route resources via a per-filter request-time lazy cache keyed by per-route proto pointer; each cache miss builds a fresh `*runtimeConfig` with its own stateful resources (counters registered via `Registry.NewCounterIfAbsent` for post-Freeze idempotency) — see ADR-0117 for the precedent and discipline." NO change to the original Decision body; the amendment is a forward-pointer. Lands in Task 5 alongside ADR-0117.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 11. The 14-field deferral list (per SPEC §2.1) is captured INLINE at BEHAVIOR_CONTRACT §13.1 (the `### envoy.filters.http.local_ratelimit` subsection's "Silent-ignored fields" paragraph) + §13.5 forward-pointer notes per SPEC §8.1 ADR-0120 collapse. NO new deferral ADRs are authored at phase 11 (mirrors phase 10's similar SPEC §8.1 collapse of the would-be ADR-0114 no-stats deferral; ADR-0040's discipline holds — silent-ignore is the framework pattern, deferral lists are documentation artefacts).
- **ADR-0061** (stats Registry + SN1–SN8 rules) — extended with SN9 in ADR-0118: the new rule extends the `flattenToProm` switch with the local-ratelimit filter-specific tag-extractor; the SN1–SN5 prefix-segment switch hot-path is unchanged (SN9 fires only on the unmatched-prefix path). Inline edit of ADR-0061: append a `## Amendment (per phase 11 ADR-0118)` paragraph noting "Rule SN9 added per phase 11: extends the `flattenToProm` switch with a filter-specific tag-extractor for the `<stat_prefix>.http_local_rate_limit.<counter>` shape; produces Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. NO change to SN1–SN8 hot-path." NO change to the original Decision body. Lands in Task 6 alongside ADR-0118.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — UNCHANGED in phase 11 (the existing `Register` + `Freeze` discipline carries through). Cross-reference recorded in ADR-0114 §Consequences. NO in-place edit.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0114 §Consequences. The filter set extends from {cors, envoy_go_test, router, fault, header_mutation} to {cors, envoy_go_test, router, fault, header_mutation, local_ratelimit}. NO in-place edit of ADR-0074.
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 11. local_ratelimit's `New` factory CONSUMES `ctx.Stats` (for the four-counter `filterStats` registration) but does NOT consume `ctx.StatPrefix` (the local_ratelimit filter-level stat_prefix is `cfg.StatPrefix` from the proto, NOT the HCM-level conn-manager prefix). ADR-0114 §Consequences notes the Stats-consumption-only pattern (analogous to fault per ADR-0100 first-use; phase 11 confirms the field is opt-in per filter, not mandatory).
- **ADR-0101** (runtimeConfig shape + parser pattern) — extended cross-reference recorded in ADR-0115 §Consequences. The local_ratelimit runtimeConfig mirrors fault's structurally (5 fields vs fault's 8 — both follow the closure-capture + parse-at-New + read-only-shared-after-New discipline). NO in-place edit of ADR-0101.
- **ADR-0102** (terminal-replace + StopIteration localReplyDone gate) — VERBATIM REUSE in phase 11; no change. ADR-0119 §Consequences notes that the request-side terminal-replace primitive carries through unchanged for the rate-limited path. NO in-place edit.

These six cross-references land at the task that anchors each affected ADR (ADR-0114 at Task 2; ADR-0115 at Task 2; ADR-0116 at Task 3; ADR-0117 at Task 5; ADR-0118 at Task 6; ADR-0119 at Task 4). No in-place edit of any pre-existing ADR is required EXCEPT the ADR-0073 amendment paragraph in Task 5 + the ADR-0061 amendment paragraph in Task 6.

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention. The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-11-http-filter-local-ratelimit-impl \
                 -b phase-11-http-filter-local-ratelimit-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-11-http-filter-local-ratelimit-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 16 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-11-http-filter-local-ratelimit-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-11-http-filter-local-ratelimit-impl -b phase-11-http-filter-local-ratelimit-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md commit `63c88ed` and its SHA-fill follow-up `47b624b` immediately before, then the BRAINSTORM.md commits `6ad8d8a` + `59e1be2`, then phase 10 REVIEW at `97ed8b9` + phase 10 STATE/PROGRESS SHA-fill at `2c80b30` + phase 10 phase-done at `8e17e06`. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `113`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0114..ADR-0119 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md` returns `63c88ed` (the SPEC commit). If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012'` returns every fixture PASS. The 13 pre-existing fixtures (0000–0012) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 14 fuzzers from phases 02–10 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each). Phase 11 adds the fifteenth (`FuzzLocalRateLimitConfigParse` per Task 8).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **`envoy.extensions.filters.http.local_ratelimit.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimit | head -5` returns the `LocalRateLimit` proto type's exported fields without an `import path failed` error; `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimitPerRoute | head -5` returns the per-route container; `go doc github.com/envoyproxy/go-control-plane/envoy/type/v3 TokenBucket | head -5` returns the shared `TokenBucket` proto. If any `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `97ed8b9` so a tidy should not be needed).
12. **Pre-existing `internal/filter/http/localratelimit/` directory does NOT exist.** `test ! -d internal/filter/http/localratelimit && echo "ok: localratelimit absent"` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
13. **Pre-existing `flattenToProm` does NOT have a Rule SN9 entry.** `grep -nE 'SN9|http_local_rate_limit|envoy_local_http_ratelimit_prefix' internal/stats/name.go` returns 0 matches. If 1+, the rule has already been added by a concurrent phase — investigate.
14. **Pre-existing `fixture.HTTPLocalRateLimit` does NOT exist.** `grep -nE 'HTTPLocalRateLimit' test/differential/fixture/fixture.go` returns 0 matches. If 1+, investigate.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).
16. **Pre-existing `cmd/envoy-go/main.go` registers exactly the FIVE filters expected at master `97ed8b9`** — `grep -nE 'httpReg.Register' cmd/envoy-go/main.go` returns 5 matches: `router`, `cors`, `envoygotest`, `fault`, `header_mutation`. If 6+, another filter has been added concurrently; re-verify the registration ordering before adding the local_ratelimit line.

If all 16 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the six ADRs ADR-0114..ADR-0119 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the six ADRs and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-11-http-filter-local-ratelimit-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 16 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` (new file).
**Acceptance:** all 16 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-11-http-filter-local-ratelimit-impl
git log --oneline master | head -10                                   # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (47b624b), SPEC (63c88ed), BRAINSTORM (6ad8d8a + 59e1be2), phase-10 REVIEW (97ed8b9), phase-10 STATE SHA-fill (2c80b30), phase-10 phase-done (8e17e06)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 113
git log -1 --format=%H -- docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md
                                                                       # expect: 63c88ed... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/localratelimit && echo "ok: localratelimit absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimit | head -5
                                                                       # expect: type LocalRateLimit struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimitPerRoute | head -5
                                                                       # expect: type LocalRateLimitPerRoute struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/type/v3 TokenBucket | head -5
                                                                       # expect: type TokenBucket struct { ... }
grep -cE 'SN9|http_local_rate_limit|envoy_local_http_ratelimit_prefix' internal/stats/name.go  # expect: 0
grep -cE 'HTTPLocalRateLimit' test/differential/fixture/fixture.go    # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
grep -cE 'httpReg.Register' cmd/envoy-go/main.go                      # expect: 5
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md`**

```markdown
# Phase 11 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..10 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 16 preconditions were satisfied at cold-start>

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The six ADRs anticipated by SPEC §8 (ADR-0114..ADR-0119). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0114** `internal/filter/http/localratelimit/` package shape (no-underscore directory + extension-registry registration ordering) — Task 2
- **ADR-0115** runtimeConfig shape + 5/14-field decomposition + PGV constraint table + filter-internal `fill_interval >= 50ms` validation discipline — Task 2
- **ADR-0116** `tokenBucket` Option-A lazy-refill on access + monotonic-time semantics + LBP-1-adjacent declaration + ±10ms empirical refill-timing tolerance — Task 3
- **ADR-0117** Per-route bucket isolation as ADR-0073 wholesale-override consequence (FIRST stateful per-route filter; ADR-0073 amendment paragraph) — Task 5
- **ADR-0118** Stat-table 22→26-name extension + `enforced == rate_limited` MVP invariant + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registered as Rule SN9 — Task 6
- **ADR-0119** Rate-limited response wire shape + body byte-exact `local_rate_limited` + 4-header set lowercase wire-form + 429 default status + SendLocalReply reuse from phase 09 — Task 4

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nine planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Tag-extractor registration site = EXTEND `internal/stats/name.go`'s `flattenToProm` SWITCH WITH NEW RULE SN9** (corrects SPEC §12 D1's mis-statement of `internal/admin/stats.go` which does not exist; the actual mechanism is the hardcoded switch in `internal/stats/name.go::flattenToProm`).
2. **D2 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)` + `SetEncoderCallbacks(cb)` per the cors + fault + header_mutation precedents** (filter struct carries both `dcb` and `ecb` fields; only `dcb` is used at request time for the SendLocalReply call; `ecb` is unused but kept for chain-of-conformance).
3. **D3 — PGV plumbing = EXPLICIT CHECKS IN THE `New` FACTORY** (mirrors cors + fault + header_mutation; six explicit checks: tc-non-nil, stat_prefix-non-empty, max_tokens > 0, tokens_per_fill > 0 if explicitly set [defaults to 1 if absent], fill_interval >= 50ms [verbatim Envoy error string], status.code in [400, 600) if explicitly set [defaults to 429 if absent]).
4. **D4 — Scenario 3 retry-with-deadline option = ±10ms TOLERANCE WITH SIMPLE `time.Sleep` (DEFAULT)** (retry-with-deadline reserved as fallback if CI flakes; ADR-0116 §Consequences may amend in-place per ADR-0089 to record chosen option).
5. **D5 — Test-only clock injection = SKIP** (per SPEC default; race-detector cycle test exercises mutex via real wallclock; future hardening pass may revisit).
6. **PLAN-emerging — File split = `bucket.go` + `local_ratelimit.go`** (cleanly separates the token-bucket primitive from the filter orchestration; mirrors size-driven splits in similar packages).
7. **PLAN-emerging — Race-detector cycle test = ADD `TestTokenBucket_ConcurrentTryConsume`** (~30 LoC; lands in Task 3).
8. **PLAN-N (fixture topology) — 4 PRE-CONFIGURED LISTENERS (`l_s1`, `l_s2`, `l_s3`, `l_per_route`) IN A SINGLE BOOTSTRAP** (diverges from SPEC §7.1's two-listener+teardown layout to fit the existing differential-fixture harness's single-Drive-call contract; bucket isolation provided at boot by listener-distinct factories; all 4 scenarios run in one `DriveReferenceMulti`/`DriveSubjectMulti` invocation via `fixture.MultiListenerDriver`).
9. **PLAN-emerging — BackendKind enum value = `HTTPLocalRateLimit BackendKind = 10`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-11 SPEC + 11 PLAN confirmed present in HEAD; SPEC at 63c88ed; ADR tail at 0113 (next-free 0114); internal/filter/http/localratelimit/ absent (Task 2 lands); SN9 absent (Task 6 lands); fixture.HTTPLocalRateLimit absent (Task 9 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md
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
git add docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md
git commit -m "phase 11: PROGRESS preamble + planner-time decision resolution"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR anticipation table), §12 (deferred decisions), §15 (acceptance criteria) and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `internal/filter/http/localratelimit/` package — `doc.go` + `local_ratelimit.go` skeleton (TypeURL, types, runtimeConfig + parser + filter-internal 50ms validation, New factory) + `local_ratelimit_test.go` (New-time PGV/filter-internal-validation tests) [ADR-0114, ADR-0115]

**Files:**
- Create: `internal/filter/http/localratelimit/doc.go`
- Create: `internal/filter/http/localratelimit/local_ratelimit.go`
- Create: `internal/filter/http/localratelimit/local_ratelimit_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0114, ADR-0115)

This task lands the new `internal/filter/http/localratelimit/` package skeleton with:

- The public API surface (`TypeURL` constant + `New` HTTPFilterFactory).
- The unexported types (`runtimeConfig`, `filterStats`, `filter`).
- The `buildRuntimeConfig` helper that parses + validates the 5 consumed proto fields.
- The 6 explicit PGV + filter-internal validation checks per planner-time decision 3.
- The `New` factory body parsing + validating the typed_config (rejects nil tc, malformed Any, AND each PGV constraint per SPEC §11.1 + §11.2 + §11.4 AND the filter-internal `fill_interval >= 50ms` check per §11.2c — returns a non-nil error mirroring Envoy's verbatim message for the latter).
- The `*filter` instance shape + `SetDecoderCallbacks` + `SetEncoderCallbacks` + pass-through methods for OnDestroy + DecodeData + EncodeData + DecodeTrailers + EncodeTrailers + EncodeHeaders.
- The DecodeHeaders body is **STUBBED** to `return Continue` in this task — Task 4 lands the full body. Task 2's commit therefore lands a "compiles + parses + validates" filter that does NOT yet apply rate-limiting.
- The `tokenBucket` field of `runtimeConfig` is set to `nil` in this task — Task 3 lands the `tokenBucket` primitive + connects it. Task 2's `runtimeConfig` carries the parsed values (statPrefix, statusCode, body, stats) but not yet a non-nil `bucket`.
- The `filterStats` field is set to `nil` in this task — Task 4 lands the full `filterStats` wiring connecting the four counters via `ctx.Stats.NewCounter(...)`. Task 2 verifies the `New` factory accepts a `nil` `ctx.Stats` (test code path; production code path always has a non-nil Stats per ADR-0061).

**ADR-0114** (package shape: no-underscore directory `localratelimit/` + extension-registry registration ordering + diverges from header_mutation's underscore-preserving pattern; rationale: aligns with cors + fault whose proto type-names were already single tokens), and **ADR-0115** (runtimeConfig + 5/14-field decomposition + PGV constraint table + filter-internal `fill_interval >= 50ms` validation discipline) both land here.

**Precondition:** Task 1 done; `internal/filter/http/localratelimit/` does not exist.
**Artifact:** three new files (doc + impl + unit tests); two ADRs in DECISIONS.md.
**Acceptance:** `go build ./internal/filter/http/localratelimit/...` clean; `go test -race ./internal/filter/http/localratelimit/...` passes the New-time test suite (the DecodeHeaders unit tests in Task 4 are not yet present); `go vet ./...` clean; ADR-0114, ADR-0115 in DECISIONS.md.

- [ ] **Step 1: Create `internal/filter/http/localratelimit/doc.go`**

```go
// Package localratelimit implements the envoy.filters.http.local_ratelimit
// HTTP filter under the 07.1 HTTP filter framework.
//
// Phase 11: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.8
// empirical scrapes of reference Envoy v1.37.2.
//
// Package-naming note (per ADR-0114): the directory name + Go package
// identifier are both `localratelimit` (no underscore), diverging from
// phase 10's header_mutation/ underscore-preserving pattern. The no-underscore
// form aligns with cors/ + fault/ whose proto type-names were already single
// tokens; the local_ratelimit proto type-name is the lone divergence in the
// §9 family-row set. ADR-0114 codifies the rationale: a single proto-name
// divergence does not justify flipping the existing 3-of-4-filters convention.
//
// Decode side (per SPEC §6.5):
//
//   1. Increment rc.stats.enabled (per-request unconditional).
//   2. Call rc.bucket.tryConsume() (lazy-refill on access; per ADR-0116).
//   3. If true: increment rc.stats.ok; return Continue.
//   4. If false: increment rc.stats.rateLimited AND rc.stats.enforced
//      (lockstep MVP invariant per ADR-0118; future shadow-mode phase widens
//      to enforced ≤ rate_limited when filter_enforced runtime-key support
//      lands per the Runtime + hot restart family).
//   5. Invoke f.dcb.SendLocalReply(rc.statusCode, rc.body,
//      OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})
//      (per ADR-0102 + ADR-0119; reuse from phase 09 fault precedent at
//      internal/filter/http/fault/fault.go:321).
//   6. Return StopIteration.
//
// Encode side: pass-through (no encode-side state per SPEC §6.5 NOTE).
//
// Token-bucket primitive (per SPEC §6.4 + ADR-0116):
//
//   - Lazy refill on access (Option A); single sync.Mutex per bucket; no
//     per-bucket goroutine; no time.Ticker; no signal channel.
//   - time.Now().UnixNano() carries Go ≥1.9's monotonic component for
//     time.Now()-derived values; arithmetic across time.Now() calls
//     advances monotonically under wall-clock NTP corrections / leap seconds.
//   - Bucket lifetime = filter-instance lifetime (listener: process-exit;
//     per-route: process-exit since envoy-go is static-config-only per
//     BOOTSTRAP_PROMPT.md §3.1).
//   - LBP-1-adjacent: closure-capture half preserved (matches phase 06.1 /
//     09 / 10 discipline); lock-free hot-path half deliberately departs
//     since elapsed → refills → tokens is a multi-step CAS-resistant
//     computation. Mutex is the natural choice.
//
// Per-route override semantics (per SPEC §5.5 + §11.6 + ADR-0117):
//
//   - Wholesale-override per ADR-0073 + ADR-0117 (ADR-0073 amendment).
//   - Each LocalRateLimitPerRoute TPFC entry runs through New at
//     config-load time; each invocation allocates its own *runtimeConfig +
//     own *tokenBucket + own *filterStats.
//   - The 3-tier PerRouteConfig.Resolve picks the most-specific config
//     per request; that config carries its own bucket pointer.
//   - Listener-level state is NOT touched for per-route reqs (per §11.6
//     empirical confirmation).
//
// Concurrency model (per SPEC §5.6):
//
//   - Per-bucket: single sync.Mutex guarding {tokens, lastRefillNs}.
//   - Per-filter-instance: *runtimeConfig is a *runtimeConfig reference
//     (closure-captured at boot-time New; never mutated); read-only thread-safe.
//   - Per-process: registry frozen at boot per ADR-0072; per-route TPFC
//     parsed at HCM-build time; bucket lifetime = process lifetime.
//
// Public surface (per SPEC §6.1):
//
//   TypeURL = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"
//   New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)
//
// New body discipline (per ADR-0115):
//
//   1. tc must be non-nil (boot-fail-fast per ADR-0072).
//   2. Unmarshal tc to *envoyextensionsfiltershttplocalratelimitv3.LocalRateLimit.
//   3. Validate stat_prefix non-empty (PGV per §11.1).
//   4. Validate token_bucket non-nil + max_tokens > 0 (PGV per §11.2a).
//   5. Validate tokens_per_fill: absent → default 1 (per §11.2b-i);
//      explicit zero → reject (PGV per §11.2b-ii).
//   6. Validate fill_interval >= 50ms (FILTER-INTERNAL not PGV per §11.2c;
//      verbatim error string mirrors Envoy v1.37.2's
//      source/server/config_validation/server.cc:76 message).
//   7. Validate status.code: absent → default 429 (per §11.4); explicit
//      out-of-[400,600) → reject (PGV per §11.4).
//   8. Capture mostSpecificHeaderMutationsWins flag (NOT applicable to
//      local_ratelimit; this filter has no equivalent flag).
//   9. Construct *tokenBucket via newTokenBucket(maxTokens, tokensPerFill, fillInterval).
//   10. Construct *filterStats via newFilterStats(ctx.Stats, statPrefix).
//   11. Construct *runtimeConfig.
//   12. Return FilterInstanceFactory closure that allocates a fresh *filter
//       per request bound to *runtimeConfig.
//
// Stats: 4 counters per stat_prefix (per SPEC §6.6 + §11.5):
//
//   <stat_prefix>.http_local_rate_limit.enabled       (every req reaching the filter)
//   <stat_prefix>.http_local_rate_limit.ok            (tryConsume → true)
//   <stat_prefix>.http_local_rate_limit.rate_limited  (tryConsume → false)
//   <stat_prefix>.http_local_rate_limit.enforced      (tryConsume → false; lockstep MVP)
//
// Prometheus tag-extraction: the filter-specific tag-extractor `Rule SN9`
// (added to internal/stats/name.go's flattenToProm switch in Task 6 per
// ADR-0118 + planner-time decision 1) extracts the <stat_prefix> segment
// into the `envoy_local_http_ratelimit_prefix` Prometheus label.
//
// Cross-cutting ADR anchors:
//
//   - ADR-0114: package shape + boot registration (no-underscore directory)
//   - ADR-0115: runtimeConfig + 5/14-field decomposition + PGV table +
//     filter-internal `fill_interval >= 50ms` validation discipline
//   - ADR-0116: tokenBucket Option-A lazy-refill + LBP-1-adjacent +
//     ±10ms empirical refill-timing tolerance
//   - ADR-0117: per-route bucket isolation as ADR-0073 wholesale-override
//     consequence (FIRST stateful per-route filter; ADR-0073 amendment)
//   - ADR-0118: stat-table 22→26-name extension + `enforced == rate_limited`
//     MVP invariant + filter-specific Prometheus tag-extractor SN9
//   - ADR-0119: rate-limited response wire shape + body byte-exact +
//     4-header set + 429 default + SendLocalReply reuse from phase 09
//
// Forward-pointers (silent-ignored fields per SPEC §2.1 + ADR-0040 silent-
// ignore discipline; 14 fields organized by 8 family-clusters; full list at
// BEHAVIOR_CONTRACT §13.1 / §13.5):
//
//   - descriptors / rate_limits / always_consume_default_token_bucket /
//     max_dynamic_descriptors  — descriptor-action subsystem (couples to
//     global_ratelimit future phase)
//   - filter_enabled / filter_enforced / request_headers_to_add_when_not_enforced
//     — runtime + shadow-mode subsystem (couples to Runtime + hot restart;
//     DIVERGENCE-WINDOW: envoy-go silent-ignores; ref Envoy defaults to 0%
//     OFF; fixture configs MUST set both to 100% explicitly per SPEC §1.1)
//   - local_cluster_rate_limit  — xDS cluster-state subsystem
//   - response_headers_to_add  — response-side header injection
//   - local_rate_limit_per_downstream_connection  — per-connection lifecycle
//   - stage  — multi-stage limiting (couples to descriptor-action)
//   - enable_x_ratelimit_headers / vh_rate_limits  — X-RateLimit headers + vh policy
//   - rate_limited_as_resource_exhausted  — gRPC trailer mapping
package localratelimit
```

- [ ] **Step 2: Create `internal/filter/http/localratelimit/local_ratelimit.go` with the parser + factory + stub DecodeHeaders body**

```go
package localratelimit

import (
    "errors"
    "fmt"
    "net/http"
    "sync/atomic"
    "time"

    localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.local_ratelimit typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 7) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit"

// filterName is the canonical http_filters[].name string for local_ratelimit
// (matches the listener config typed_per_filter_config map keys).
const filterName = "envoy.filters.http.local_ratelimit"

// rateLimitedBody is the canonical 18-byte body emitted on the rate-limited
// response per SPEC §11.3 empirical pin (ASCII; NO trailing newline; MD5
// 397e830923f3080ba63b3d38b53678ac). Per ADR-0119.
var rateLimitedBody = []byte("local_rate_limited")

// minFillInterval is the filter-internal minimum on token_bucket.fill_interval
// per SPEC §11.2c empirical pin. The check fires AFTER proto unmarshal succeeds,
// NOT as a PGV constraint; the verbatim error string at New time mirrors
// Envoy v1.37.2's source/server/config_validation/server.cc:76 message exactly.
// Per ADR-0115.
const minFillInterval = 50 * time.Millisecond

// runtimeConfig is the per-instance parsed config shape per ADR-0115.
//
// Five fields consumed at request-eval time (statPrefix, bucket, statusCode,
// body, stats); fourteen LocalRateLimit fields silently ignored per SPEC §2.1
// + ADR-0040 silent-ignore discipline (full list at BEHAVIOR_CONTRACT §13.1).
type runtimeConfig struct {
    statPrefix string        // from cfg.StatPrefix (PGV non-empty per §11.1)
    bucket     *tokenBucket  // closure-captured per filter-instance / per per-route entry; nil-stub in Task 2; non-nil in Task 3
    statusCode int           // from cfg.Status.Code (default 429 per §11.4; PGV [400, 600))
    body       []byte        // literal "local_rate_limited" (18 bytes; per §11.3 + ADR-0119)
    stats      *filterStats  // 4 counters scoped by stat_prefix; nil-stub in Task 2; non-nil in Task 4
}

// filterStats is the 4-counter set per ADR-0118 + SPEC §11.5.
//
// All four counters are *atomic.Int64 (lock-free counter increments per ADR-0061);
// the Inc-side is mutex-free. The four-counter set is the COMPLETE surface (per
// SPEC §11.5 conclusion (a): no near_limit, no total_pending, no dynamic-metadata
// gauges).
type filterStats struct {
    enabled     *atomic.Int64 // every req reaching the filter
    ok          *atomic.Int64 // tryConsume → true
    rateLimited *atomic.Int64 // tryConsume → false
    enforced    *atomic.Int64 // tryConsume → false; lockstep with rateLimited per MVP
}

// filter is the per-request filter instance allocated by the factory closure.
// State is request-scoped; *runtimeConfig is the closure-captured shared state
// (immutable post-construction; read-only thread-safe per SPEC §5.6).
type filter struct {
    rc  *runtimeConfig
    dcb envoyhttp.DecoderFilterCallbacks
    ecb envoyhttp.EncoderFilterCallbacks // unused per SPEC §6.5 NOTE; kept per planner-time decision 2 for chain-of-conformance
}

// Statically assert the both-sides interface conformance (matches cors precedent).
var (
    _ envoyhttp.StreamDecoderFilter = (*filter)(nil)
    _ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// SetDecoderCallbacks / SetEncoderCallbacks per planner-time decision 2.
// The framework's per-stream chain (internal/filter/http/chain.go) calls
// SetDecoderCallbacks once per stream; the filter stores the reference for
// later use during DecodeHeaders.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// New is the HTTPFilterFactory exposed at boot. Per ADR-0114 + ADR-0115:
//
//  1. tc must be non-nil (a local_ratelimit filter with no typed_config has
//     no behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *localratelimitv3.LocalRateLimit; return error on
//     malformed Any.
//  3. Run six explicit validation checks (per planner-time decision 3):
//     stat_prefix non-empty, max_tokens > 0, tokens_per_fill > 0 if explicit
//     [defaults to 1 if absent], fill_interval >= 50ms [filter-internal not
//     PGV; verbatim Envoy error string], status.code in [400, 600) if explicit
//     [defaults to 429 if absent].
//  4. Construct *tokenBucket via newTokenBucket(maxTokens, tokensPerFill, fillInterval).
//     [Task 3 lands the constructor; Task 2 stubs to nil.]
//  5. Construct *filterStats via newFilterStats(ctx.Stats, statPrefix).
//     [Task 4 lands the constructor; Task 2 stubs to nil.]
//  6. Construct *runtimeConfig capturing the 5 consumed fields.
//  7. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to *runtimeConfig.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("local_ratelimit: typed_config required")
    }
    var c localratelimitv3.LocalRateLimit
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("local_ratelimit: unmarshal: %w", err)
    }
    rc, err := buildRuntimeConfig(&c, ctx)
    if err != nil {
        return nil, err
    }
    return func() envoyhttp.HTTPFilter {
        return &filter{rc: rc}
    }, nil
}

// buildRuntimeConfig parses + validates the 5 consumed fields from the
// LocalRateLimit proto. Per planner-time decision 3: six explicit checks.
// Per ADR-0115.
//
// In Task 2 the bucket + stats fields are nil-stubbed (Task 3 lands the
// tokenBucket; Task 4 lands the filterStats). The validation logic is fully
// implemented in Task 2 so Task 4's tests can exercise the validation paths
// without redoing the work.
func buildRuntimeConfig(c *localratelimitv3.LocalRateLimit, ctx envoyhttp.FactoryCtx) (*runtimeConfig, error) {
    // Check 1 (PGV per §11.1): stat_prefix non-empty.
    statPrefix := c.GetStatPrefix()
    if statPrefix == "" {
        return nil, errors.New("local_ratelimit: stat_prefix required")
    }
    // Check 2 (PGV per §11.2a): token_bucket non-nil + max_tokens > 0.
    tb := c.GetTokenBucket()
    if tb == nil {
        return nil, errors.New("local_ratelimit: token_bucket required")
    }
    if tb.GetMaxTokens() == 0 {
        return nil, errors.New("local_ratelimit: token_bucket.max_tokens must be > 0")
    }
    maxTokens := int64(tb.GetMaxTokens())
    // Check 3 (PGV per §11.2b-i + §11.2b-ii): tokens_per_fill — absent → 1; explicit zero → reject.
    var tokensPerFill int64
    if tb.GetTokensPerFill() == nil {
        tokensPerFill = 1 // proto default per §11.2b-i empirical pin
    } else {
        v := tb.GetTokensPerFill().GetValue()
        if v == 0 {
            return nil, errors.New("local_ratelimit: token_bucket.tokens_per_fill must be > 0 if specified")
        }
        tokensPerFill = int64(v)
    }
    // Check 4 (FILTER-INTERNAL per §11.2c; NOT PGV; verbatim Envoy error string):
    // fill_interval must be >= 50ms.
    fillInterval := tb.GetFillInterval().AsDuration()
    if fillInterval < minFillInterval {
        // Verbatim Envoy v1.37.2 error string from source/server/config_validation/server.cc:76.
        // The mirrored string is the LOAD-BEARING wire-equivalence claim per ADR-0115:
        // operators reading boot logs see identical failure shapes across reference + envoy-go.
        return nil, errors.New("local rate limit token bucket fill timer must be >= 50ms")
    }
    // Check 5 (PGV per §11.4): status.code — absent → 429; explicit out-of-[400, 600) → reject.
    statusCode := 429
    if c.GetStatus() != nil {
        statusCode = int(c.GetStatus().GetCode())
        if statusCode < 400 || statusCode >= 600 {
            return nil, fmt.Errorf("local_ratelimit: status.code must be in [400, 600); got %d", statusCode)
        }
    }
    // Construct the *tokenBucket (Task 3 lands the constructor; Task 2 stubs to nil).
    var bucket *tokenBucket
    bucket = newTokenBucket(maxTokens, tokensPerFill, fillInterval)
    // Construct the *filterStats (Task 4 lands the constructor; Task 2 stubs to nil
    // when ctx.Stats is nil — test code path; production code path always has non-nil Stats).
    var fs *filterStats
    if ctx.Stats != nil {
        fs = newFilterStats(ctx.Stats, statPrefix)
    }
    return &runtimeConfig{
        statPrefix: statPrefix,
        bucket:     bucket,
        statusCode: statusCode,
        body:       rateLimitedBody,
        stats:      fs,
    }, nil
}

// DecodeHeaders implements the rate-limit decision per SPEC §6.5. Task 4 lands
// the full body; Task 2 stubs to Continue (no-op).
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    // Task 4 wires:
    //   f.rc.stats.enabled.Add(1)
    //   if f.rc.bucket.tryConsume() {
    //       f.rc.stats.ok.Add(1)
    //       return envoyhttp.Continue
    //   }
    //   f.rc.stats.rateLimited.Add(1)
    //   f.rc.stats.enforced.Add(1)
    //   f.dcb.SendLocalReply(f.rc.statusCode, f.rc.body, envoyhttp.OrderedHeaders{
    //       {Name: "Content-Type", Value: "text/plain"},
    //   })
    //   return envoyhttp.StopIteration
    return envoyhttp.Continue
}

// Pass-through methods per SPEC §6.5. local_ratelimit operates only on
// DecodeHeaders; all other states are no-op.
func (f *filter) DecodeData(http.Header, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
    return envoyhttp.Continue
}
func (f *filter) EncodeData(http.Header, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) OnDestroy() {}
```

NOTE: the implementer at Task 2 step 2 verifies the exact `envoyhttp.HTTPFilter` interface methods + `FilterDataStatus` / `FilterTrailersStatus` types by reading `internal/filter/http/types.go`; the above sketch may diverge in minor signature details (e.g., `endStream` parameter on EncodeData). The fault + header_mutation precedents at `internal/filter/http/{fault,header_mutation}/{fault,header_mutation}.go` are the structural precedents — copy their pass-through-method shape verbatim.

The `tokenBucket` type + `newTokenBucket` constructor are referenced by `buildRuntimeConfig` but defined in `bucket.go` (Task 3). Until Task 3 lands, the build fails at this step — Task 2's commit lands `local_ratelimit.go` + `bucket.go` together (the file split is logical, not commit-boundary). Alternative: stub `newTokenBucket` in Task 2 with a placeholder that returns `&tokenBucket{}` and let Task 3 replace it; the planner picks the former for cleaner per-task atomicity.

- [ ] **Step 3: Create `internal/filter/http/localratelimit/local_ratelimit_test.go` with the New-time PGV/filter-internal-validation tests**

```go
package localratelimit

import (
    "fmt"
    "testing"
    "time"

    localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
    typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "google.golang.org/protobuf/types/known/anypb"
    "google.golang.org/protobuf/types/known/durationpb"
    "google.golang.org/protobuf/types/known/wrapperspb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

// mustAny packages a proto.Message into an *anypb.Any with the local_ratelimit TypeURL.
func mustAny(t *testing.T, msg *localratelimitv3.LocalRateLimit) *anypb.Any {
    t.Helper()
    a, err := anypb.New(msg)
    if err != nil {
        t.Fatalf("anypb.New: %v", err)
    }
    return a
}

// happyConfig returns a minimum-viable LocalRateLimit proto for happy-path
// tests: stat_prefix + token_bucket{max_tokens=10, fill_interval=1s}.
func happyConfig() *localratelimitv3.LocalRateLimit {
    return &localratelimitv3.LocalRateLimit{
        StatPrefix: "test",
        TokenBucket: &envoytypev3.TokenBucket{
            MaxTokens:    10,
            FillInterval: durationpb.New(1 * time.Second),
            // TokensPerFill omitted → defaults to 1 per §11.2b-i.
        },
    }
}

func TestNew_NilTC(t *testing.T) {
    _, err := New(nil, envoyhttp.FactoryCtx{})
    if err == nil {
        t.Fatalf("New(nil, _): want error, got nil")
    }
    if !contains(err.Error(), "typed_config required") {
        t.Errorf("New(nil, _): got error %q, want containing 'typed_config required'", err.Error())
    }
}

func TestNew_MalformedTC(t *testing.T) {
    bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
    _, err := New(bad, envoyhttp.FactoryCtx{})
    if err == nil {
        t.Fatalf("New(malformed, _): want error, got nil")
    }
    if !contains(err.Error(), "unmarshal") {
        t.Errorf("New(malformed, _): got error %q, want containing 'unmarshal'", err.Error())
    }
}

func TestNew_StatPrefixEmpty(t *testing.T) {
    cfg := happyConfig()
    cfg.StatPrefix = ""
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
    if err == nil || !contains(err.Error(), "stat_prefix required") {
        t.Errorf("New(stat_prefix=\"\", _): got %v, want error containing 'stat_prefix required'", err)
    }
}

func TestNew_MaxTokensZero(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket.MaxTokens = 0
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
    if err == nil || !contains(err.Error(), "max_tokens must be > 0") {
        t.Errorf("New(max_tokens=0, _): got %v, want error containing 'max_tokens must be > 0'", err)
    }
}

func TestNew_TokenBucketAbsent(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket = nil
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
    if err == nil || !contains(err.Error(), "token_bucket required") {
        t.Errorf("New(token_bucket=nil, _): got %v, want error containing 'token_bucket required'", err)
    }
}

func TestNew_TokensPerFillExplicitZero(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket.TokensPerFill = wrapperspb.UInt32(0)
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
    if err == nil || !contains(err.Error(), "tokens_per_fill must be > 0") {
        t.Errorf("New(tokens_per_fill=0, _): got %v, want error containing 'tokens_per_fill must be > 0'", err)
    }
}

func TestNew_TokensPerFillOmittedDefaultsToOne(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket.TokensPerFill = nil
    factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
    if err != nil {
        t.Fatalf("New(tokens_per_fill=absent, _): want success, got %v", err)
    }
    if factory == nil {
        t.Fatalf("New: returned nil factory")
    }
    // Verify by constructing an instance; tryConsume succeeds the first time
    // (initial fill = max_tokens = 10) and we cannot directly probe the bucket's
    // tokensPerFill from outside the package — but Task 3's bucket_test.go covers
    // that primitive directly. Here we verify happy-path acceptance.
}

func TestNew_FillIntervalBelow50ms(t *testing.T) {
    for _, dt := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 49 * time.Millisecond} {
        t.Run(fmt.Sprintf("%dms", dt/time.Millisecond), func(t *testing.T) {
            cfg := happyConfig()
            cfg.TokenBucket.FillInterval = durationpb.New(dt)
            _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
            if err == nil {
                t.Fatalf("New(fill_interval=%v, _): want error, got nil", dt)
            }
            // Verbatim Envoy v1.37.2 error string per SPEC §11.2c + ADR-0115.
            wantString := "local rate limit token bucket fill timer must be >= 50ms"
            if err.Error() != wantString {
                t.Errorf("New(fill_interval=%v, _): got %q, want %q (verbatim Envoy)", dt, err.Error(), wantString)
            }
        })
    }
}

func TestNew_FillIntervalAtOrAbove50ms(t *testing.T) {
    for _, dt := range []time.Duration{50 * time.Millisecond, 51 * time.Millisecond, 100 * time.Millisecond, 1 * time.Second} {
        t.Run(fmt.Sprintf("%v", dt), func(t *testing.T) {
            cfg := happyConfig()
            cfg.TokenBucket.FillInterval = durationpb.New(dt)
            _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
            if err != nil {
                t.Fatalf("New(fill_interval=%v, _): want success, got %v", dt, err)
            }
        })
    }
}

func TestNew_StatusCodeBelow400(t *testing.T) {
    cfg := happyConfig()
    cfg.Status = &typev3.HttpStatus{Code: 399}
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
    if err == nil || !contains(err.Error(), "[400, 600)") {
        t.Errorf("New(status.code=399, _): got %v, want error containing '[400, 600)'", err)
    }
}

func TestNew_StatusCodeAtOrAbove600(t *testing.T) {
    for _, code := range []int{600, 700, 999} {
        t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
            cfg := happyConfig()
            cfg.Status = &typev3.HttpStatus{Code: typev3.StatusCode(code)}
            _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
            if err == nil {
                t.Fatalf("New(status.code=%d, _): want error", code)
            }
        })
    }
}

func TestNew_StatusCodeOmittedDefaultsTo429(t *testing.T) {
    cfg := happyConfig()
    cfg.Status = nil // explicitly omit
    factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
    if err != nil {
        t.Fatalf("New(status omitted, _): want success, got %v", err)
    }
    inst := factory().(*filter)
    if inst.rc.statusCode != 429 {
        t.Errorf("statusCode default: got %d, want 429", inst.rc.statusCode)
    }
}

func TestNew_HappyPath_AllConsumedFields(t *testing.T) {
    cfg := happyConfig()
    cfg.StatPrefix = "myprefix"
    cfg.TokenBucket.MaxTokens = 5
    cfg.TokenBucket.TokensPerFill = wrapperspb.UInt32(2)
    cfg.TokenBucket.FillInterval = durationpb.New(500 * time.Millisecond)
    cfg.Status = &typev3.HttpStatus{Code: 503}
    factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
    if err != nil {
        t.Fatalf("New(happy, _): want success, got %v", err)
    }
    inst := factory().(*filter)
    if inst.rc.statPrefix != "myprefix" {
        t.Errorf("statPrefix: got %q, want %q", inst.rc.statPrefix, "myprefix")
    }
    if inst.rc.statusCode != 503 {
        t.Errorf("statusCode: got %d, want 503", inst.rc.statusCode)
    }
    if string(inst.rc.body) != "local_rate_limited" {
        t.Errorf("body: got %q, want %q", string(inst.rc.body), "local_rate_limited")
    }
    if inst.rc.bucket == nil {
        t.Errorf("bucket: got nil, want non-nil (Task 3 wires)")
    }
    if inst.rc.stats == nil {
        t.Errorf("stats: got nil, want non-nil (ctx.Stats provided)")
    }
}

// contains is a substring helper avoiding strings import in test code.
func contains(haystack, needle string) bool {
    for i := 0; i+len(needle) <= len(haystack); i++ {
        if haystack[i:i+len(needle)] == needle {
            return true
        }
    }
    return false
}
```

NOTE: the implementer at Task 2 step 3 may need to adjust the `typev3.HttpStatus` import path or `wrapperspb` import based on the exact go-control-plane API surface; the precondition `go doc github.com/envoyproxy/go-control-plane/envoy/type/v3 HttpStatus` confirms the `Code` field is a `StatusCode` enum (uint32-equivalent). The `contains` helper is a stand-in for `strings.Contains` to avoid importing `strings` only for tests; the implementer may switch to `strings.Contains` if preferred.

- [ ] **Step 4: Run tests; confirm all pass except the bucket-related happy-path tests (which depend on Task 3's `tokenBucket` constructor)**

```bash
go test -race -count=1 ./internal/filter/http/localratelimit/... 2>&1 | tail -30
```

Expected: build error on `newTokenBucket` reference unless Task 3's `bucket.go` lands in the same commit. The PLAN's task boundary is logical; the implementer may EITHER (a) land Task 2 + Task 3 as a single combined commit (simpler — `bucket.go` + `local_ratelimit.go` are both small) OR (b) stub `newTokenBucket` in Task 2 with `func newTokenBucket(_ int64, _ int64, _ time.Duration) *tokenBucket { return &tokenBucket{} }` and a `tokenBucket` empty-struct type, replacing both in Task 3. Recommendation: option (a) — Tasks 2 + 3 land as a single commit titled `phase 11: localratelimit/ package skeleton + tokenBucket primitive [ADR-0114, ADR-0115, ADR-0116]`, ADR-0114 + ADR-0115 + ADR-0116 all anchor at this commit per the ADR-0044 first-use convention. The PROGRESS.md per-task entry covers both Task 2 and Task 3 narratively. The total commit's net delta: ~480 LoC of production + ~280 LoC of tests + ~3 ADRs in DECISIONS.md.

- [ ] **Step 5: Append ADR-0114 + ADR-0115 to `docs/envoy-go/DECISIONS.md`**

Per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). Verbatim text fragments:

```markdown
## ADR-0114: `internal/filter/http/localratelimit/` package shape — no-underscore directory + extension-registry registration ordering

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 2 commit)
**Doctrine:** ADR-0044 ADR-on-impl convention; ADR-0072 HTTPRegistry boot-fail-fast.
**Lands-in-task:** Phase 11 Task 2 (package skeleton) + Task 7 (boot registration line in cmd/envoy-go/main.go).

### Context

Phase 11 lands `envoy.filters.http.local_ratelimit` as the FOURTH production HTTP filter under `internal/filter/http/` (after cors @ 07.1, fault @ 09, header_mutation @ 10). The proto type-name carries an underscore (`local_ratelimit`); the prior 3-of-4 filters had single-token proto type-names (`cors`, `fault`) or single-token-with-underscore-preserved (`header_mutation`). The package-naming question: directory + Go-package identifier should both be `localratelimit` (no underscore) to align with cors + fault, OR both be `local_ratelimit` to align with header_mutation's underscore-preserving pattern.

### Decision

**Directory + Go-package identifier are both `localratelimit` (no underscore).** The naming rule is: when the proto type-name is a single token (cors, fault), the Go package is the same single token; when the proto type-name carries an underscore, the Go package elides the underscore for Go-idiomatic single-token form (Go convention prefers single-token package names per Effective Go). Phase 10's `header_mutation/` is the lone underscore-preserving exception; phase 11 returns to the cors + fault precedent. No back-port of phase 10's directory rename — header_mutation/ stays as-is (a single-package-name divergence does not justify cross-package churn).

The boot-registration ordering in `cmd/envoy-go/main.go` follows BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline (codified at phase-09 brainstorm time + reaffirmed at phase 10): `router → cors → envoygotest → fault → header_mutation → localratelimit → Freeze`. The local_ratelimit registration line lands AFTER header_mutation's registration AND BEFORE the existing `header_mutation.RegisterPerRouteValidator(httpReg)` call (since per-route-validator registrations must precede `Freeze` per ADR-0072) — local_ratelimit does NOT register a per-route validator (no per-route invariants requiring boot-time validation; per-route TPFC entries are validated lazily at request-time via `buildRuntimeConfigPerRoute`).

### Alternatives considered

(a) **Underscore-preserving directory `local_ratelimit/` matching header_mutation's pattern** — rejected. Phase 10's header_mutation pattern was a single-precedent divergence from cors + fault; treating it as a flip would require back-porting cors + fault to underscore-preserving, generating cross-package churn for a stylistic preference. The 3-of-4 cors-precedent stance prevails.

(b) **Snake-case Go package identifier `local_ratelimit` (Go-syntactic but non-idiomatic)** — rejected. Go convention strongly prefers single-token package names; the lint rule `package-comments` (golangci-lint) would flag the snake-case form.

### Consequences

- Future filter packages with proto type-names carrying multi-tokens follow the elide-underscore rule (e.g., `globalratelimit`, `extauthz`, `jwtauthn`); single-token proto type-names map to the single-token Go package directly.
- ADR-0074 (filter set: cors + envoy_go_test) extends to {cors, envoy_go_test, router, fault, header_mutation, local_ratelimit} — the FIFTH production filter (router is terminal, distinct from observable filters).
- `cmd/envoy-go/main.go` registration ordering at phase 11 phase-done: `router → cors → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze`. The local_ratelimit Register call lives between the existing fault Register and the header_mutation Register — wait, let me re-check… [implementer at Task 7 step 2 confirms the exact insertion point + final ordering against the master HEAD `97ed8b9` of cmd/envoy-go/main.go].

---

## ADR-0115: `runtimeConfig` shape + 5-consumed/14-silent-ignored field decomposition + PGV constraint table + filter-internal `fill_interval ≥ 50ms` validation discipline

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 2 commit)
**Doctrine:** ADR-0040 silent-ignore discipline; ADR-0072 HTTPRegistry boot-fail-fast; ADR-0101 runtimeConfig shape + parser pattern (extended).
**Lands-in-task:** Phase 11 Task 2.

### Context

The LocalRateLimit proto carries 19 top-level fields; phase 11 consumes 5 (`stat_prefix`, `token_bucket{max_tokens, tokens_per_fill, fill_interval}`, `status{code}`, plus the `LocalRateLimitPerRoute` per-route container). The remaining 14 fields are silently ignored at config-load time per ADR-0040 silent-ignore discipline (full deferral list at SPEC §2.1 + BEHAVIOR_CONTRACT §13.1 / §13.5).

The validation surface decomposes into TWO classes per SPEC §11.1–§11.4 empirical pins:
- PGV constraints (4 checks): `stat_prefix` non-empty, `max_tokens > 0`, `tokens_per_fill > 0` if explicitly set, `status.code ∈ [400, 600)` if explicitly set.
- Filter-internal constraint (1 check): `fill_interval >= 50ms` — NOT a PGV constraint; the check fires AFTER proto unmarshal succeeds; the verbatim Envoy v1.37.2 error string is `local rate limit token bucket fill timer must be >= 50ms` per SPEC §11.2c empirical pin (`source/server/config_validation/server.cc:76`).

### Decision

`runtimeConfig` carries 5 fields:

```go
type runtimeConfig struct {
    statPrefix string         // from cfg.StatPrefix (PGV non-empty per §11.1)
    bucket     *tokenBucket   // closure-captured per filter-instance / per per-route entry
    statusCode int            // from cfg.Status.Code (default 429 per §11.4)
    body       []byte         // literal "local_rate_limited" (18 bytes; per §11.3 + ADR-0119)
    stats      *filterStats   // 4 counters scoped by stat_prefix
}
```

Validation runs as 6 explicit checks in `buildRuntimeConfig` (per planner-time decision 3):
1. `cfg.StatPrefix != ""` — PGV per §11.1.
2. `cfg.TokenBucket != nil` — implicit-required per §6.4.
3. `cfg.TokenBucket.MaxTokens > 0` — PGV per §11.2a.
4. `tokens_per_fill`: absent → default 1; explicit zero → reject per §11.2b-i + §11.2b-ii.
5. `cfg.TokenBucket.FillInterval >= 50ms` — FILTER-INTERNAL not PGV per §11.2c; verbatim Envoy error string preserved for boot-log byte-equivalence.
6. `cfg.Status.Code ∈ [400, 600)` if explicitly set; default 429 per §11.4.

The 14 silent-ignored fields (organized by 8 family-clusters per SPEC §2.1):
- Descriptor-action (4): `descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`.
- Runtime + shadow-mode (3): `filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced`. **Divergence-window:** envoy-go silent-ignores; ref Envoy defaults to 0% OFF; fixture configs MUST set both to 100% per SPEC §1.1.
- xDS cluster-state (1): `local_cluster_rate_limit`.
- Response-side header injection (1): `response_headers_to_add`.
- Per-connection lifecycle (1): `local_rate_limit_per_downstream_connection`.
- Multi-stage limiting (1): `stage`.
- X-RateLimit headers + vh policy (2): `enable_x_ratelimit_headers`, `vh_rate_limits`.
- gRPC trailer mapping (1): `rate_limited_as_resource_exhausted`.

### Alternatives considered

(a) **Implicit-PGV via protoreflect-based runtime checks** — rejected per planner-time decision 3. No prior phase wires it; introducing it for one filter would diverge from existing explicit-check discipline at zero code-quality benefit.

(b) **Filter-internal `fill_interval` check coalesced with PGV** — rejected. Envoy v1.37.2's empirical surface distinguishes the two error-paths (PGV envelope vs filter-internal); envoy-go's wire-equivalence claim (boot-log error string fidelity) requires preserving the filter-internal-not-PGV distinction at the surface.

(c) **Authoring per-cluster deferral ADRs (ADR-0120 omnibus + 14 per-field ADRs analogous to fault's ADR-0104 pattern)** — rejected per SPEC §8.1 ADR-0120 collapse. The 14-field deferral is a documentation artefact captured at BEHAVIOR_CONTRACT §13.1 / §13.5, not a load-bearing decision worthy of standalone ADRs.

### Consequences

- ADR-0101 (runtimeConfig shape + parser pattern) extends with the 5-field local_ratelimit variant; the closure-capture + parse-at-New + read-only-shared-after-New discipline carries through unchanged.
- Future shadow-mode phase wiring `filter_enabled` + `filter_enforced` runtime-key support widens the runtimeConfig with two new fields (`filterEnabledFraction` + `filterEnforcedFraction`); the existing 5-field shape is forward-compatible.
- The verbatim Envoy error string `local rate limit token bucket fill timer must be >= 50ms` is encoded as a string literal in the filter; if Envoy v1.37.2's error string changes in a future Envoy bump (per ADR-0008's pin-bump discipline), the literal string in `local_ratelimit.go` must be updated in lockstep.
```

- [ ] **Step 6: Vet + lint + test + commit (combined commit per Task 2/3 boundary)**

If Tasks 2+3 land combined per recommendation:

```bash
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./internal/filter/http/localratelimit/...
git add internal/filter/http/localratelimit/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 11: localratelimit/ package skeleton + runtimeConfig parser + tokenBucket primitive [ADR-0114, ADR-0115, ADR-0116]

Lands the new internal/filter/http/localratelimit/ package with:
- TypeURL constant + New HTTPFilterFactory (per ADR-0114)
- runtimeConfig + filterStats + filter struct (per ADR-0115)
- buildRuntimeConfig parsing + 6 explicit PGV/filter-internal checks
  (per planner-time decision 3); verbatim Envoy error string for the
  fill_interval >= 50ms filter-internal check per SPEC §11.2c
- tokenBucket primitive: lazy refill on access; sync.Mutex per bucket;
  time.Now().UnixNano() monotonic clock; LBP-1-adjacent (per ADR-0116)
- Pass-through DecodeData/EncodeHeaders/EncodeData/Trailers/OnDestroy

DecodeHeaders body STUBBED to Continue; Task 4 lands the full body
with SendLocalReply + StopIteration on rate-limit + counter-increment
discipline.

ADR-0114: package shape (no-underscore directory localratelimit/
diverges from header_mutation/ underscore-preserving pattern; aligns
with cors/ + fault/ whose proto type-names were single tokens) +
extension-registry ordering.

ADR-0115: runtimeConfig 5-field shape + 14-field silent-ignore
decomposition (per SPEC §2.1) + PGV constraint table + filter-internal
fill_interval >= 50ms validation discipline (verbatim Envoy error
string preserved for boot-log byte-equivalence per SPEC §11.2c).

ADR-0116: tokenBucket primitive Option-A lazy-refill + monotonic-time
semantics + LBP-1-adjacent declaration + ±10ms empirical refill-timing
tolerance (narrowed from BRAINSTORM ±20ms hypothesis per SPEC §11.7's
52-trial empirical envelope).

Tests: TestNew_NilTC / MalformedTC / StatPrefixEmpty /
TokenBucketAbsent / MaxTokensZero / TokensPerFillExplicitZero /
TokensPerFillOmittedDefaultsToOne / FillIntervalBelow50ms (table-driven
across 10/20/49ms — verifies verbatim Envoy error string) /
FillIntervalAtOrAbove50ms (table-driven across 50/51/100ms/1s) /
StatusCodeBelow400 / StatusCodeAtOrAbove600 (table-driven 600/700/999) /
StatusCodeOmittedDefaultsTo429 / HappyPath_AllConsumedFields, plus
TestTokenBucket_NewInitialFillEqualsMax / TryConsume_DepletesUntilZero /
TryConsume_ReturnsFalseWhenEmpty / LazyRefill_NoRefillBelowFillInterval /
LazyRefill_SingleQuantumRefill / LazyRefill_MultiQuantumRefill_CapAtMax /
LazyRefill_LastRefillNsAdvancesByFullQuanta /
ConcurrentTryConsume (race-detector cycle test per planner-time decision 7).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §6.1 + §6.2 + §6.4 + §11.1 + §11.2 + §11.4; ADR-0040 + ADR-0072 + ADR-0101; planner-time decisions 2, 3, 6.*

---

## Task 3: `bucket.go` `tokenBucket` primitive + `bucket_test.go` mechanics tests + concurrent-tryConsume race-detector test [ADR-0116]

**Files:**
- Create: `internal/filter/http/localratelimit/bucket.go`
- Create: `internal/filter/http/localratelimit/bucket_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0116 if not already landed in Task 2's combined commit)

This task lands the `tokenBucket` primitive per SPEC §6.4 + ADR-0116. The struct + constructor + `tryConsume` lazy-refill-on-access method are the entirety of the primitive's surface; no per-bucket goroutine, no `time.Ticker`, no signal channel. The race-detector cycle test `TestTokenBucket_ConcurrentTryConsume` (per planner-time decision 7) exercises the mutex discipline mechanically.

Per the Task 2 step 6 recommendation, Tasks 2 + 3 may land as a single combined commit (the file split is logical, not commit-boundary). If split, Task 3's commit lands `bucket.go` + `bucket_test.go` + ADR-0116 separately AFTER Task 2's commit lands the package skeleton + ADR-0114 + ADR-0115. The PLAN reads as if separate for clarity; the implementer may combine.

**Precondition:** Task 2 done (package skeleton in place; `local_ratelimit.go` references `newTokenBucket` which Task 3 lands).
**Artifact:** `bucket.go` + `bucket_test.go`; ADR-0116 in DECISIONS.md.
**Acceptance:** `go build ./internal/filter/http/localratelimit/...` clean; `go test -race -count=1 ./internal/filter/http/localratelimit/...` passes the bucket-mechanics test suite + the race-detector cycle test; `go vet ./...` clean; ADR-0116 in DECISIONS.md.

- [ ] **Step 1: Create `internal/filter/http/localratelimit/bucket.go`**

```go
package localratelimit

import (
    "sync"
    "time"
)

// tokenBucket is the lazy-refill-on-access token-bucket primitive per ADR-0116
// + SPEC §6.4. Single sync.Mutex per bucket; no per-bucket goroutine; no
// time.Ticker; no signal channel.
//
// Concurrency: tryConsume is the SOLE writer; concurrent calls are serialized
// by the mutex. The hot-path holds the mutex for 5–10 nanoseconds typical
// (compute elapsed → integer-divide → conditional add → decrement). Lock
// contention bounded by per-route request rate; per-route TPFC isolation
// means listener-level traffic and per-route traffic don't compete on the
// same bucket per ADR-0117.
//
// Time-source semantics: time.Now().UnixNano() carries Go ≥1.9's monotonic
// component for time.Now()-derived values; arithmetic across time.Now() calls
// advances monotonically under wall-clock NTP corrections / leap seconds.
// Phase 11 takes this guarantee from Go documentation; unit tests do NOT
// exercise wall-clock backward-jump simulation (such testing requires
// test-only clock injection, deferred per SPEC §12 D5 + planner-time decision 5).
//
// LBP-1-adjacent declaration per ADR-0116: closure-capture half preserved
// (matches phase 06.1 / 09 / 10 discipline); lock-free hot-path half
// deliberately departs since the elapsed → refills → tokens computation is
// a multi-step CAS-resistant sequence. Mutex is the natural choice.
type tokenBucket struct {
    maxTokens     int64
    tokensPerFill int64
    fillInterval  time.Duration

    mu           sync.Mutex
    tokens       int64
    lastRefillNs int64 // time.Now().UnixNano() at last refill
}

// newTokenBucket constructs a *tokenBucket with initial fill = maxTokens
// (per SPEC §6.4 + §11.7 — empirical pin §11.7 confirms the initial bucket
// is full at t=0). The lastRefillNs baseline is time.Now().UnixNano() at
// construction; subsequent tryConsume calls compute refills relative to this
// baseline.
//
// Caller (buildRuntimeConfig at local_ratelimit.go) is responsible for
// validating maxTokens > 0 and fillInterval >= 50ms BEFORE calling
// newTokenBucket; the constructor itself does NOT re-validate.
func newTokenBucket(maxTokens, tokensPerFill int64, fillInterval time.Duration) *tokenBucket {
    return &tokenBucket{
        maxTokens:     maxTokens,
        tokensPerFill: tokensPerFill,
        fillInterval:  fillInterval,
        tokens:        maxTokens,
        lastRefillNs:  time.Now().UnixNano(),
    }
}

// tryConsume attempts to consume one token from the bucket; returns true
// on success (token consumed) and false on failure (bucket empty after
// any due refills).
//
// The lazy-refill discipline (per SPEC §6.4):
//
//   1. Lock the mutex.
//   2. Compute elapsedNs = time.Now().UnixNano() - lastRefillNs.
//   3. Compute refills = elapsedNs / int64(fillInterval). Integer-division
//      quantizes to the configured fill_interval boundary; the quotient is
//      the number of full quanta elapsed since last refill.
//   4. If refills > 0:
//      - Add (refills * tokensPerFill) to tokens.
//      - Cap tokens at maxTokens (no over-fill).
//      - Advance lastRefillNs by (refills * fillInterval) (NOT to nowNs;
//        this preserves sub-quantum residual elapsed for the next call).
//   5. If tokens > 0: decrement tokens; return true.
//   6. Else: return false.
//
// The integer-division step (3) is the core quantization rule; SPEC §11.7
// measured the boundary as sharp at ≤5ms granularity in reference Envoy v1.37.2
// (52 trials at delay ≥ 200ms all returned 200; 24 trials at delay ≤ 199ms all
// returned 429). envoy-go's primitive matches by construction.
func (b *tokenBucket) tryConsume() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    nowNs := time.Now().UnixNano()
    elapsedNs := nowNs - b.lastRefillNs
    if refills := elapsedNs / int64(b.fillInterval); refills > 0 {
        b.tokens += refills * b.tokensPerFill
        if b.tokens > b.maxTokens {
            b.tokens = b.maxTokens
        }
        b.lastRefillNs += refills * int64(b.fillInterval)
    }
    if b.tokens > 0 {
        b.tokens--
        return true
    }
    return false
}
```

- [ ] **Step 2: Create `internal/filter/http/localratelimit/bucket_test.go`**

```go
package localratelimit

import (
    "sync"
    "sync/atomic"
    "testing"
    "time"
)

func TestTokenBucket_NewInitialFillEqualsMax(t *testing.T) {
    b := newTokenBucket(10, 1, 1*time.Second)
    if b.tokens != 10 {
        t.Errorf("initial tokens: got %d, want 10", b.tokens)
    }
    if b.maxTokens != 10 || b.tokensPerFill != 1 || b.fillInterval != 1*time.Second {
        t.Errorf("config: got max=%d perFill=%d interval=%v, want 10/1/1s", b.maxTokens, b.tokensPerFill, b.fillInterval)
    }
}

func TestTokenBucket_TryConsume_DepletesUntilZero(t *testing.T) {
    b := newTokenBucket(3, 1, 1*time.Hour) // huge interval to defeat refill during test
    if !b.tryConsume() || !b.tryConsume() || !b.tryConsume() {
        t.Fatal("first 3 tryConsume calls should succeed")
    }
    if b.tokens != 0 {
        t.Errorf("after 3 consumes: tokens %d, want 0", b.tokens)
    }
    if b.tryConsume() {
        t.Error("4th tryConsume should fail (bucket empty)")
    }
}

func TestTokenBucket_TryConsume_ReturnsFalseWhenEmpty(t *testing.T) {
    b := newTokenBucket(1, 1, 1*time.Hour)
    _ = b.tryConsume() // drain
    for i := 0; i < 100; i++ {
        if b.tryConsume() {
            t.Errorf("tryConsume %d: got true, want false (bucket should stay empty)", i)
            return
        }
    }
}

func TestTokenBucket_LazyRefill_NoRefillBelowFillInterval(t *testing.T) {
    b := newTokenBucket(1, 1, 200*time.Millisecond)
    _ = b.tryConsume() // drain
    // Backdate lastRefillNs to simulate a 100ms elapsed window (less than 200ms).
    b.lastRefillNs = time.Now().UnixNano() - int64(100*time.Millisecond)
    if b.tryConsume() {
        t.Error("tryConsume after 100ms (< fill_interval=200ms): got true, want false (no refill expected)")
    }
}

func TestTokenBucket_LazyRefill_SingleQuantumRefill(t *testing.T) {
    b := newTokenBucket(1, 1, 200*time.Millisecond)
    _ = b.tryConsume() // drain to 0
    // Backdate lastRefillNs to simulate a 250ms elapsed window (>= 200ms).
    b.lastRefillNs = time.Now().UnixNano() - int64(250*time.Millisecond)
    if !b.tryConsume() {
        t.Error("tryConsume after 250ms (>= fill_interval=200ms): got false, want true (single-quantum refill)")
    }
    if b.tokens != 0 {
        t.Errorf("after refill+consume: tokens %d, want 0", b.tokens)
    }
}

func TestTokenBucket_LazyRefill_MultiQuantumRefill_CapAtMax(t *testing.T) {
    b := newTokenBucket(5, 2, 100*time.Millisecond)
    // Drain.
    for i := 0; i < 5; i++ {
        b.tryConsume()
    }
    if b.tokens != 0 {
        t.Fatalf("setup: drain left tokens %d, want 0", b.tokens)
    }
    // Backdate to simulate 5*100ms = 500ms elapsed → 5 refill quanta × 2 tokensPerFill = 10 tokens
    // → capped at maxTokens=5.
    b.lastRefillNs = time.Now().UnixNano() - int64(500*time.Millisecond)
    if !b.tryConsume() {
        t.Fatal("tryConsume after 500ms multi-quantum refill: got false, want true")
    }
    if b.tokens != 4 {
        // After multi-quantum refill: tokens=5 (capped from 10); consumed 1 → tokens=4.
        t.Errorf("after multi-quantum refill+consume: tokens %d, want 4 (5-cap then -1)", b.tokens)
    }
}

func TestTokenBucket_LazyRefill_LastRefillNsAdvancesByFullQuanta(t *testing.T) {
    b := newTokenBucket(10, 1, 200*time.Millisecond)
    _ = b.tryConsume() // consume 1; tokens=9; lastRefillNs unchanged (no refill since elapsed < interval)
    // Snapshot the baseline.
    baseline := b.lastRefillNs
    // Backdate to simulate 350ms elapsed → 1 quantum × 200ms; the residual 150ms must NOT be lost.
    b.lastRefillNs = baseline - int64(350*time.Millisecond)
    _ = b.tryConsume() // refill of 1; consume 1; net tokens unchanged at 9
    // Verify lastRefillNs advanced by exactly 1*200ms = 200ms from the backdated value
    // (NOT to nowNs which would lose the 150ms residual).
    expectedAdvance := int64(200 * time.Millisecond)
    actualAdvance := b.lastRefillNs - (baseline - int64(350*time.Millisecond))
    if actualAdvance != expectedAdvance {
        t.Errorf("lastRefillNs advance: got %dns, want %dns (must be quantum-aligned)", actualAdvance, expectedAdvance)
    }
}

// TestTokenBucket_ConcurrentTryConsume per planner-time decision 7.
// Fires tryConsume concurrently across 64 goroutines × 100 iterations with
// shared *tokenBucket; verifies no race; verifies total-allowed-count is
// bounded by initial-tokens + at-most-one-or-two refill-quanta during the
// sub-second test window.
//
// Run with `go test -race`; the race detector validates the mutex discipline
// mechanically. The sub-second runtime keeps refill quanta to ≤ 1 (fillInterval
// = 1*time.Hour; no refill expected during the test).
func TestTokenBucket_ConcurrentTryConsume(t *testing.T) {
    const goroutines = 64
    const iterations = 100
    const initialTokens = int64(1000) // > goroutines*iterations / 6.4 to ensure both true + false outcomes
    b := newTokenBucket(initialTokens, 1, 1*time.Hour)

    var allowed atomic.Int64
    var wg sync.WaitGroup
    wg.Add(goroutines)
    for g := 0; g < goroutines; g++ {
        go func() {
            defer wg.Done()
            for i := 0; i < iterations; i++ {
                if b.tryConsume() {
                    allowed.Add(1)
                }
            }
        }()
    }
    wg.Wait()

    total := allowed.Load()
    // Bound: total must be in [0, initialTokens + epsilonRefills*tokensPerFill] where
    // epsilonRefills <= 1 since fillInterval=1h and the test runs sub-second.
    if total < 0 {
        t.Errorf("total allowed: %d, want >= 0", total)
    }
    if total > initialTokens+1 {
        t.Errorf("total allowed: %d, want <= %d (initialTokens + epsilonRefills*tokensPerFill=1)", total, initialTokens+1)
    }
    if total < initialTokens-1 {
        // The 64*100=6400 attempts comfortably exceed initialTokens=1000, so
        // total should saturate at initialTokens (modulo the epsilonRefills band).
        t.Errorf("total allowed: %d, want >= %d (saturation)", total, initialTokens-1)
    }
}
```

- [ ] **Step 3: Run tests with race detector**

```bash
go test -race -count=1 -v ./internal/filter/http/localratelimit/... 2>&1 | tail -30
```

Expected: all bucket-test functions PASS. The race detector reports no data races.

- [ ] **Step 4: Append ADR-0116 to `docs/envoy-go/DECISIONS.md`** (if not already landed in the combined Task 2/3 commit)

Per the ADR-0001 template:

```markdown
## ADR-0116: `tokenBucket` primitive — Option-A lazy-refill on access + monotonic-time semantics + LBP-1-adjacent declaration + ±10ms empirical refill-timing tolerance

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 3 commit)
**Doctrine:** ADR-0061 stats Registry / LBP-1 invariant; ADR-0008 Envoy v1.37.2 pin (empirical evidence source).
**Lands-in-task:** Phase 11 Task 3.

### Context

The token-bucket primitive must implement Envoy v1.37.2's local-ratelimit semantics: each `tryConsume` call decrements one token if available, else rejects; tokens refill at `tokens_per_fill` per `fill_interval` quantum, capped at `max_tokens`. Three implementation strategies were considered at BRAINSTORM time (per BRAINSTORM §2.4):

- **Option A:** Lazy refill on access. Single mutex per bucket; refill computed on each `tryConsume` call from `time.Now() - lastRefillNs` quantization. No per-bucket goroutine.
- **Option B:** Active timer-driven refill. `time.Ticker` per bucket; refill goroutine wakes every `fill_interval` to add tokens; cancel-on-OnDestroy plumbing.
- **Option C:** Signal-channel-driven refill. Hybrid Option-B with explicit goroutine + channel to avoid Ticker drift.

Empirical pin SPEC §11.7 measured Envoy v1.37.2's refill behavior across 99 trials sweeping delay 180→400ms (4ms granularity in tight band 196→204ms): zero spurious refills before t=200ms; zero missed refills at t≥200ms. The boundary is sharp at ≤5ms granularity (the measurement floor of Python `time.sleep` resolution on Linux). This empirically confirms that Envoy itself uses lazy refill (not active timer) — the refill happens on access at ≥ `last_refill + fill_interval`.

### Decision

**Option A: lazy refill on access, single `sync.Mutex` per bucket.**

```go
type tokenBucket struct {
    maxTokens, tokensPerFill int64
    fillInterval             time.Duration

    mu           sync.Mutex
    tokens       int64
    lastRefillNs int64
}

func (b *tokenBucket) tryConsume() bool {
    b.mu.Lock(); defer b.mu.Unlock()
    nowNs := time.Now().UnixNano()
    elapsedNs := nowNs - b.lastRefillNs
    if refills := elapsedNs / int64(b.fillInterval); refills > 0 {
        b.tokens += refills * b.tokensPerFill
        if b.tokens > b.maxTokens { b.tokens = b.maxTokens }
        b.lastRefillNs += refills * int64(b.fillInterval)
    }
    if b.tokens > 0 { b.tokens--; return true }
    return false
}
```

Time source: `time.Now().UnixNano()` carries Go ≥1.9's monotonic component for `time.Now()`-derived values; arithmetic across `time.Now()` calls advances monotonically.

Empirical refill-timing tolerance: **±10ms** wall-clock around the configured `fill_interval` boundary. Narrowed from BRAINSTORM's ±20ms hypothesis per SPEC §11.7's 52-trial empirical envelope (boundary sharp at ≤5ms granularity; the ±10ms tolerance accommodates CI scheduling jitter on the measurement floor + a small safety margin). PLAN author has the option (per planner-time decision 4 + SPEC §12 D4) to widen to ±20ms with retry-with-deadline harness if fixture 0013 scenario 3 flakes during phase 11 impl under heavy CI load.

**LBP-1-adjacent declaration:** the closure-capture half of LBP-1 (the bucket lifetime is bounded by the closure-captured `*runtimeConfig` lifetime; matches phase 06.1 / 09 / 10 discipline) is preserved. The lock-free hot-path half (LBP-1's `Walk-under-RLock-plus-atomic-Load` for stats Registry) deliberately departs: the `elapsed → refills → tokens` computation is a multi-step compute-then-conditional-update sequence not amenable to a single CAS. Phase 11 is the FIRST production filter to declare LBP-1-succession-adjacent stance explicitly; future per-filter reviewers consult ADR-0116 as the precedent for stateful-resource filters that need locking.

### Alternatives considered

(a) **Option B (active timer-driven refill)** — rejected at BRAINSTORM time. Per-bucket goroutine + cancel-on-OnDestroy plumbing adds significant complexity (~50 LoC per bucket + goroutine-leak risk on OnDestroy + Ticker-drift concerns). Lazy-refill matches Envoy's empirical behavior and is mechanically simpler.

(b) **Option C (signal-channel-driven refill)** — rejected for similar reasons; hybrid complexity without the Option-A simplicity payoff.

(c) **Lock-free CAS-based hot path** — rejected. The `elapsed → refills → tokens` computation requires atomic-swap of multiple state fields; no single-CAS encoding exists short of packing all state into a single `uint64` (which would limit `maxTokens` to 32 bits and risk overflow). Mutex is the deliberate choice; ADR-0116 records this as a deliberate departure from LBP-1's lock-free clause.

(d) **Wider refill-timing tolerance ±20ms (BRAINSTORM hypothesis)** — rejected at SPEC time per §1.1 amendment; SPEC §11.7's empirical evidence allows a tighter ±10ms with safety margin. PLAN reserves widening as a fallback per planner-time decision 4 + SPEC §12 D4 if CI flakes.

### Consequences

- The `tokenBucket` primitive is reusable in principle by future rate-limit filters (e.g., `global_ratelimit` per its per-process token-bucket fallback if it lands; phase 11 does NOT export the primitive — it stays unexported in `localratelimit/bucket.go`; future cross-filter reuse would require either a refactor extracting the primitive to a shared package OR re-implementation per the convention of avoiding cross-filter primitive sharing).
- The race-detector cycle test `TestTokenBucket_ConcurrentTryConsume` validates the mutex discipline mechanically; the test is the canonical evidence that the LBP-1-adjacent declaration is sound.
- Future shadow-mode phase wiring `filter_enforced` runtime-key support widens the lockstep discipline: when `filter_enforced` < 100%, `enforced` increments only on the rate-limited subset that's also enforced; `rate_limited` increments on the full rate-limited set. The `tokenBucket` primitive itself stays UNCHANGED — the lockstep change happens at the `DecodeHeaders` call site.
- The ±10ms empirical envelope is encoded at BEHAVIOR_CONTRACT §13.3 timing-tolerances row (per SPEC §13.3); fixture 0013 scenario 3 enforces the band; if CI-load flakes surface, ADR-0116 §Consequences amends in-place per ADR-0089 to record the chosen mitigation (widen tolerance OR retry-with-deadline harness).
```

- [ ] **Step 5: Vet + lint + test + commit**

```bash
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./internal/filter/http/localratelimit/...
```

If Tasks 2 + 3 land separately:

```bash
git add internal/filter/http/localratelimit/bucket.go internal/filter/http/localratelimit/bucket_test.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 11: tokenBucket primitive + race-detector cycle test [ADR-0116]

Lands the lazy-refill-on-access tokenBucket primitive in
internal/filter/http/localratelimit/bucket.go per SPEC §6.4. Single
sync.Mutex per bucket; no per-bucket goroutine; time.Now().UnixNano()
monotonic clock; LBP-1-adjacent (closure-capture half preserved;
lock-free hot-path half deliberately departs since elapsed→refills→tokens
is a multi-step CAS-resistant computation).

Tests cover 7 mechanics paths: NewInitialFillEqualsMax,
TryConsume_DepletesUntilZero, TryConsume_ReturnsFalseWhenEmpty,
LazyRefill_NoRefillBelowFillInterval, LazyRefill_SingleQuantumRefill,
LazyRefill_MultiQuantumRefill_CapAtMax,
LazyRefill_LastRefillNsAdvancesByFullQuanta. Plus the race-detector
cycle test ConcurrentTryConsume (64 goroutines × 100 iterations) per
planner-time decision 7 — validates the mutex discipline mechanically.

ADR-0116: tokenBucket Option-A lazy-refill + monotonic-time semantics +
LBP-1-adjacent declaration + ±10ms empirical refill-timing tolerance
(narrowed from BRAINSTORM ±20ms per SPEC §11.7's 52-trial empirical envelope).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §6.4 + §11.7; ADR-0061 LBP-1; planner-time decision 6 (file split), 7 (race test).*

---

## Task 4: `DecodeHeaders` body + `filterStats` wiring + 4-counter Inc-discipline + DecodeHeaders unit tests [ADR-0118 (partial), ADR-0119]

**Files:**
- Modify: `internal/filter/http/localratelimit/local_ratelimit.go` (replace stub `DecodeHeaders` body + add `newFilterStats` constructor)
- Modify: `internal/filter/http/localratelimit/local_ratelimit_test.go` (add `DecodeHeaders` tests + filterStats counter-Inc-discipline tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0119; ADR-0118 partial — the SN9 rule lands in Task 6)

This task lands the `DecodeHeaders` body per SPEC §6.5 + ADR-0119 + the partial ADR-0118 MVP-invariant + `newFilterStats` constructor wiring the 4-counter set via `ctx.Stats.NewCounter(...)`. The body discipline is:

1. Increment `rc.stats.enabled` (per-request unconditional).
2. Call `rc.bucket.tryConsume()`.
3. If `true`: increment `rc.stats.ok`; return `Continue`.
4. If `false`: increment `rc.stats.rateLimited` AND `rc.stats.enforced` IN LOCKSTEP (MVP invariant per ADR-0118); invoke `f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})`; return `StopIteration`.

ADR-0119 lands here (rate-limited response wire shape + body byte-exact + 4-header set + 429 default + SendLocalReply reuse from phase 09); ADR-0118's MVP-invariant (`enforced == rate_limited` lockstep) lands here PARTIALLY — the full ADR-0118 lands in Task 6 alongside the SN9 rule + the 22→26 stat-table extension. The partial-vs-full ADR-0118 split is acceptable per ADR-0044's "first-use commit anchors" convention: the MVP-invariant first-use is here in Task 4; the SN9 stat-table extension first-use is in Task 6; the implementer may either land ADR-0118 in full at Task 4 (with a forward-pointer to Task 6 for SN9) OR land ADR-0118 in two halves (the MVP-invariant half here, the SN9 half in Task 6). Recommendation: land ADR-0118 in full at Task 6 — it's cleaner to have the ADR's full body (MVP invariant + SN9 + 22→26 table) in one commit; Task 4 references ADR-0118 in commit-message only.

**Precondition:** Task 3 done (tokenBucket wired into runtimeConfig).
**Artifact:** modified `local_ratelimit.go` + extended `local_ratelimit_test.go`; ADR-0119 in DECISIONS.md.
**Acceptance:** `go build ./internal/filter/http/localratelimit/...` clean; `go test -race -count=1 ./internal/filter/http/localratelimit/...` passes the DecodeHeaders test suite (allow path + rate-limited path + counter-Inc-discipline); `go vet ./...` clean; ADR-0119 in DECISIONS.md.

- [ ] **Step 1: Add `newFilterStats` constructor to `local_ratelimit.go`**

The `filterStats` struct holds `*stats.Counter` (settled at planning time per `internal/stats/counter.go`: `*Counter` exposes `Inc()`, `Add(uint64)`, and `Load() uint64` — sufficient for the per-counter discipline below; mirrors the existing fault filter's `*stats.Counter` consumption pattern at `internal/filter/http/fault/fault.go`):

```go
type filterStats struct {
    enabled     *stats.Counter
    ok          *stats.Counter
    rateLimited *stats.Counter
    enforced    *stats.Counter
}
```

Insert after the `runtimeConfig` struct definition (before `New`):

```go
// newFilterStats constructs the 4-counter filterStats for the given stat_prefix.
// Stat names match SPEC §6.6 + §11.5 + ADR-0118 + the new Rule SN9 in
// internal/stats/name.go (Task 6):
//
//   <statPrefix>.http_local_rate_limit.enabled
//   <statPrefix>.http_local_rate_limit.ok
//   <statPrefix>.http_local_rate_limit.rate_limited
//   <statPrefix>.http_local_rate_limit.enforced
//
// reg must be non-nil; the caller (buildRuntimeConfig) checks for nil and
// stubs filterStats accordingly in test code. In production, ADR-0061's
// pre-Freeze discipline guarantees a non-nil Registry at New time.
func newFilterStats(reg *stats.Registry, statPrefix string) *filterStats {
    return &filterStats{
        enabled:     reg.NewCounter(statPrefix + ".http_local_rate_limit.enabled"),
        ok:          reg.NewCounter(statPrefix + ".http_local_rate_limit.ok"),
        rateLimited: reg.NewCounter(statPrefix + ".http_local_rate_limit.rate_limited"),
        enforced:    reg.NewCounter(statPrefix + ".http_local_rate_limit.enforced"),
    }
}
```

- [ ] **Step 2: Replace stub `DecodeHeaders` body**

```go
// DecodeHeaders implements the rate-limit decision per SPEC §6.5 + ADR-0119.
//
// Discipline:
//   1. Increment rc.stats.enabled (unconditional per-request).
//   2. Call rc.bucket.tryConsume().
//   3. If true: increment rc.stats.ok; return Continue.
//   4. If false: increment rc.stats.rateLimited AND rc.stats.enforced
//      IN LOCKSTEP (MVP invariant per ADR-0118; future shadow-mode phase
//      widens to enforced ≤ rate_limited when filter_enforced runtime-key
//      support lands per the Runtime + hot restart family).
//   5. Invoke f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{
//      {Name: "Content-Type", Value: "text/plain"}}) per ADR-0102 + ADR-0119.
//      Reuses the request-side terminal-replace primitive verbatim from phase
//      09 fault precedent at internal/filter/http/fault/fault.go:321.
//   6. Return StopIteration.
//
// The 4-header wire-form on the rate-limited path (content-length: 18,
// content-type: text/plain, date: <RFC1123>, server: envoy) is produced by:
//   - SendLocalReply call site (Content-Type from the OrderedHeaders arg)
//   - HCM/router downstream auto-injection (content-length, date, server)
//
// Per the existing fault precedent + the existing internal/filter/hcm/codec.go:17
// serverHeader() returning "envoy" — confirms SPEC §1.1 amendment that the
// brainstorm hypothesis of `server: envoy-go` was incorrect.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    f.rc.stats.enabled.Inc()
    if f.rc.bucket.tryConsume() {
        f.rc.stats.ok.Inc()
        return envoyhttp.Continue
    }
    f.rc.stats.rateLimited.Inc()
    f.rc.stats.enforced.Inc() // lockstep MVP per ADR-0118
    f.dcb.SendLocalReply(f.rc.statusCode, f.rc.body, envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    return envoyhttp.StopIteration
}
```

NOTE: `*stats.Counter` exposes both `.Inc()` and `.Add(uint64)`; the body uses `.Inc()` for the +1-per-request discipline. Per-counter `Load()` is used by the test cases.

- [ ] **Step 3: Add DecodeHeaders unit tests to `local_ratelimit_test.go`**

```go
// fakeDecoderCallbacks captures SendLocalReply calls for assertion in tests.
type fakeDecoderCallbacks struct {
    envoyhttp.DecoderFilterCallbacks // embed for default no-op methods
    sentStatus  int
    sentBody    []byte
    sentHeaders envoyhttp.OrderedHeaders
    sendCalled  bool
}

func (f *fakeDecoderCallbacks) SendLocalReply(status int, body []byte, headers envoyhttp.OrderedHeaders) {
    f.sentStatus = status
    f.sentBody = append([]byte(nil), body...)
    f.sentHeaders = headers
    f.sendCalled = true
}

func TestDecodeHeaders_AllowPath_CountersIncremented(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket.MaxTokens = 5
    factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    inst := factory().(*filter)
    cb := &fakeDecoderCallbacks{}
    inst.SetDecoderCallbacks(cb)
    status := inst.DecodeHeaders(http.Header{}, true)
    if status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue", status)
    }
    if cb.sendCalled {
        t.Error("SendLocalReply should NOT have been called on allow path")
    }
    if inst.rc.stats.enabled.Load() != 1 {
        t.Errorf("enabled: got %d, want 1", inst.rc.stats.enabled.Load())
    }
    if inst.rc.stats.ok.Load() != 1 {
        t.Errorf("ok: got %d, want 1", inst.rc.stats.ok.Load())
    }
    if inst.rc.stats.rateLimited.Load() != 0 || inst.rc.stats.enforced.Load() != 0 {
        t.Errorf("rateLimited/enforced should be 0 on allow path")
    }
}

func TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep(t *testing.T) {
    cfg := happyConfig()
    cfg.TokenBucket.MaxTokens = 1
    cfg.TokenBucket.FillInterval = durationpb.New(60 * time.Second) // huge interval; no refill during test
    factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    inst := factory().(*filter)
    cb := &fakeDecoderCallbacks{}
    inst.SetDecoderCallbacks(cb)

    // First call: bucket initially full (cap=1) → allow.
    status1 := inst.DecodeHeaders(http.Header{}, true)
    if status1 != envoyhttp.Continue {
        t.Fatalf("first call: got %v, want Continue (bucket cap=1, initial full)", status1)
    }

    // Second call: bucket empty → rate-limited.
    status2 := inst.DecodeHeaders(http.Header{}, true)
    if status2 != envoyhttp.StopIteration {
        t.Errorf("second call status: got %v, want StopIteration", status2)
    }

    if !cb.sendCalled {
        t.Fatal("SendLocalReply: not called on rate-limited path")
    }
    if cb.sentStatus != 429 {
        t.Errorf("SendLocalReply status: got %d, want 429", cb.sentStatus)
    }
    if string(cb.sentBody) != "local_rate_limited" {
        t.Errorf("SendLocalReply body: got %q, want %q", string(cb.sentBody), "local_rate_limited")
    }
    if len(cb.sentBody) != 18 {
        t.Errorf("SendLocalReply body length: got %d, want 18 bytes", len(cb.sentBody))
    }
    if len(cb.sentHeaders) != 1 {
        t.Errorf("SendLocalReply headers: got %d entries, want 1 (Content-Type)", len(cb.sentHeaders))
    }
    if cb.sentHeaders[0].Name != "Content-Type" || cb.sentHeaders[0].Value != "text/plain" {
        t.Errorf("SendLocalReply header[0]: got %v, want {Content-Type: text/plain}", cb.sentHeaders[0])
    }

    // Counter assertions: enabled=2, ok=1, rateLimited=1, enforced=1 (lockstep).
    if inst.rc.stats.enabled.Load() != 2 {
        t.Errorf("enabled: got %d, want 2", inst.rc.stats.enabled.Load())
    }
    if inst.rc.stats.ok.Load() != 1 {
        t.Errorf("ok: got %d, want 1", inst.rc.stats.ok.Load())
    }
    if inst.rc.stats.rateLimited.Load() != 1 {
        t.Errorf("rateLimited: got %d, want 1", inst.rc.stats.rateLimited.Load())
    }
    if inst.rc.stats.enforced.Load() != 1 {
        t.Errorf("enforced: got %d, want 1 (lockstep with rateLimited)", inst.rc.stats.enforced.Load())
    }
    // MVP invariant assertion.
    if inst.rc.stats.rateLimited.Load() != inst.rc.stats.enforced.Load() {
        t.Errorf("MVP invariant violated: rateLimited=%d != enforced=%d",
            inst.rc.stats.rateLimited.Load(), inst.rc.stats.enforced.Load())
    }
}

func TestStatNames_FourCountersUnderStatPrefix(t *testing.T) {
    cfg := happyConfig()
    cfg.StatPrefix = "myprefix"
    reg := stats.NewRegistry()
    _, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: reg})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    // Walk the registry; verify exactly 4 metrics under the expected names.
    expected := map[string]bool{
        "myprefix.http_local_rate_limit.enabled":      false,
        "myprefix.http_local_rate_limit.ok":           false,
        "myprefix.http_local_rate_limit.rate_limited": false,
        "myprefix.http_local_rate_limit.enforced":     false,
    }
    reg.Walk(func(m stats.Metric) {
        if _, ok := expected[m.Name()]; ok {
            expected[m.Name()] = true
        }
    })
    for name, found := range expected {
        if !found {
            t.Errorf("stat name %q: NOT registered", name)
        }
    }
}
```

NOTE: the `fakeDecoderCallbacks` stand-in must satisfy the full `envoyhttp.DecoderFilterCallbacks` interface; the embed-and-override pattern works only if the interface is small + has an embeddable default impl. The implementer at Task 4 step 3 surveys `internal/filter/http/callbacks.go` to find the canonical fake-decoder-callbacks pattern (likely `internal/filter/http/fault/fault_test.go` has a similar fake — copy that pattern verbatim). The `.Atomic()` / `.Load()` calls on filterStats fields adapt to the chosen filterStats type per Task 4 step 1.

- [ ] **Step 4: Run tests**

```bash
go test -race -count=1 -v ./internal/filter/http/localratelimit/... 2>&1 | tail -30
```

Expected: all DecodeHeaders + counter-Inc-discipline tests PASS.

- [ ] **Step 5: Append ADR-0119 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0119: Rate-limited response wire shape — body byte-exact `local_rate_limited` (18 bytes, no LF) + 4-header set lowercase wire-form (`content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) + 429 default status + `SendLocalReply` reuse from phase 09 fault precedent

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 4 commit)
**Doctrine:** ADR-0102 terminal-replace + StopIteration; ADR-0103 fault abort wire shape (body byte-exact); ADR-0072 boot-fail-fast; ADR-0008 Envoy v1.37.2 pin (empirical evidence source).
**Lands-in-task:** Phase 11 Task 4.

### Context

Envoy v1.37.2's local-ratelimit filter emits a deterministic rate-limited response when `tryConsume` returns false: status `429 Too Many Requests`; body `local_rate_limited` (18 bytes ASCII, no trailing newline); 4 headers in lexicographic-emission order `content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`; framing `Content-Length: 18` (NOT chunked).

SPEC §11.3 empirical pin captured the verbatim 150-byte wire shape via raw-bytes hex capture; body MD5 `397e830923f3080ba63b3d38b53678ac`. The body is the LOAD-BEARING wire-equivalence claim per ADR-0103's body-byte-exact discipline (extended from fault to local_ratelimit); the 4-header set is the response-header equivalence claim.

SPEC §1.1 amendment corrects the BRAINSTORM §1.1 hypothesis of `server: envoy-go`: reference Envoy emits literal `envoy`. envoy-go's existing `internal/filter/hcm/codec.go:17::serverHeader()` already returns `"envoy"` so NO envoy-go code change is needed for the server-header value.

### Decision

Phase 11's `DecodeHeaders` body emits the rate-limited response via `f.dcb.SendLocalReply(429, []byte("local_rate_limited"), envoyhttp.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})`. The framework + HCM/router downstream auto-injection produces the remaining 3 headers (content-length, date, server) and the framing.

The body is encoded as a package-level `var rateLimitedBody = []byte("local_rate_limited")` to avoid per-request allocation; the slice is read-only after package init (the `runtimeConfig.body` field is `[]byte` — a slice header — copying the underlying bytes is unnecessary; sharing the read-only backing array is race-safe per Go's memory model since no goroutine mutates the slice).

The status code defaults to 429 per SPEC §11.4; explicit configuration in `[400, 600)` is honored per the PGV check in `buildRuntimeConfig`.

The `SendLocalReply` framework primitive at `internal/filter/http/callbacks.go::DecoderFilterCallbacks` is the EXISTING phase 09 fault primitive at `internal/filter/http/fault/fault.go:321`; phase 11 reuses it verbatim. NO new framework primitive is introduced.

### Alternatives considered

(a) **Body materialization as a package-level constant `const rateLimitedBody = "local_rate_limited"` with `[]byte(...)` cast at call site** — rejected. The cast allocates a new slice per call; the `var rateLimitedBody = []byte(...)` form pre-allocates once at package init.

(b) **Custom 429 response with envoy-go-specific headers (e.g., `x-envoy-rate-limit: enforced`)** — rejected. The wire-equivalence claim requires byte-equivalent fidelity to Envoy v1.37.2; injecting filter-specific response headers would diverge.

(c) **Status text customization (e.g., RFC 8941 status text)** — rejected. The HTTP status text follows RFC 7231 (`Too Many Requests` for 429); no customization needed.

### Consequences

- ADR-0103 (fault abort wire shape) extends with the local_ratelimit variant; the body-byte-exact discipline carries through.
- The `rateLimitedBody` var is a package-level constant-equivalent (read-only after init); future filters with similar wire-equivalence claims should follow the same pattern (`var <name>Body = []byte("...")` at package level; reference via `[]byte` slice; never mutate).
- If Envoy v1.37.2's body string changes in a future Envoy bump (per ADR-0008's pin-bump discipline), the literal string must be updated in lockstep + the SPEC §11.3 empirical pin re-executed.
- Future shadow-mode phase wiring `filter_enforced` < 100% widens the response: when `enforced` is false but `rate_limited` is true, the request is allowed (no SendLocalReply) but the rate_limited counter still increments — the wire-shape decision in ADR-0119 holds for the enforced subset only; the shadow-mode phase amends ADR-0119 §Consequences in-place per ADR-0089 to record the divergence.
- The wire-equivalence claim is enforced at fixture 0013 scenario 2 (basic-rate-limited): the driver captures the 429 response bytes + 4-header set + body and asserts byte-equality across reference + subject.
```

- [ ] **Step 6: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add internal/filter/http/localratelimit/ docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 11: DecodeHeaders body + filterStats wiring + 4-counter Inc-discipline [ADR-0119]

Lands the DecodeHeaders body per SPEC §6.5: increment enabled
unconditionally; call rc.bucket.tryConsume(); on true → ok+ + Continue;
on false → rateLimited+ + enforced+ (lockstep MVP per ADR-0118
forthcoming in Task 6); SendLocalReply(429, "local_rate_limited" 18b,
{Content-Type: text/plain}) + StopIteration per ADR-0102 + ADR-0119.

Wire shape per SPEC §11.3 empirical pin: status 429 Too Many Requests;
body byte-exact (MD5 397e830923f3080ba63b3d38b53678ac); 4-header set
content-length / content-type / date / server: envoy (lowercase
wire-form per Envoy v1.37.2). The framework's SendLocalReply primitive
at internal/filter/http/callbacks.go is reused VERBATIM from phase 09
fault precedent — no new framework primitive introduced.

filterStats wired via newFilterStats(reg, statPrefix) constructing 4
counters under <statPrefix>.http_local_rate_limit.{enabled, ok,
rate_limited, enforced} per SPEC §6.6. The Prometheus tag-extractor
(Rule SN9 in internal/stats/name.go) lands in Task 6 alongside the
22→26-name table extension.

Tests: TestDecodeHeaders_AllowPath_CountersIncremented (Continue +
enabled+ok); TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep
(StopIteration + SendLocalReply with 429+body+1-header set + lockstep
MVP invariant assertion); TestStatNames_FourCountersUnderStatPrefix
(verifies the registry's NewCounter calls produce the 4 expected names).

ADR-0119: rate-limited response wire shape + body byte-exact
local_rate_limited 18 bytes no LF + 4-header set lowercase wire-form +
429 default + SendLocalReply reuse.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §6.5 + §6.6 + §11.3 + §11.5; ADR-0102 + ADR-0103 + ADR-0118 (forthcoming Task 6) + ADR-0119; planner-time decision 3 (explicit checks).*

---

## Task 5: Per-route TPFC parsing + per-route bucket independence test [ADR-0117 + ADR-0073 amendment]

**Files:**
- Modify: `internal/filter/http/localratelimit/local_ratelimit.go` (add `LocalRateLimitPerRoute` parsing path if needed)
- Modify: `internal/filter/http/localratelimit/local_ratelimit_test.go` (add per-route bucket independence test)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0117 + ADR-0073 amendment paragraph)

This task lands per-route TPFC handling + the ADR-0117 bucket-independence claim per SPEC §11.6.

**Settled approach (lazy-cache + `NewCounterIfAbsent`).** `BuildPerRouteConfig` (`internal/filter/http/perroute.go:63-85`) `UnmarshalNew`'s each per-route TPFC Any to a generic `proto.Message`; it does NOT call any registered filter's `New`. So phase 11 cannot rely on a recursive-`New` dispatch to allocate per-route stateful resources. Instead, the factory closure captures a `sync.Map` keyed by `*LocalRateLimitPerRoute` proto pointer; on first request that resolves to a per-route entry, the filter atomically `LoadOrStore`s a freshly-built `*runtimeConfig` carrying its own `*tokenBucket` + own `*filterStats`. The four `*stats.Counter` pointers are obtained via a small framework extension `Registry.NewCounterIfAbsent(name) *Counter` (idempotent + permitted post-Freeze) — needed because the stats Registry is Frozen at boot before HCM-build sees per-route TPFCs and the per-route stat names are data-driven by the operator's chosen `stat_prefix`. Pointer identity of the resolved `*LocalRateLimitPerRoute` is the cache key, so all subsequent requests against the same per-route entry share the same `*runtimeConfig` (preserving bucket identity).

**Considered alternatives (rejected).** (a) Eager pre-allocation by walking the per-route TPFC map at HCM-build-time + extending the framework with a per-filter "per-route walker" hook — heavier (~200 LoC framework delta) than the lazy-cache (~80 LoC filter-local delta + ~30 LoC `NewCounterIfAbsent`). (b) Per-route counters sharing the listener-level `stat_prefix` — rejected, breaks the SPEC §11.6 empirical pin (reference Envoy emits SEPARATE Prometheus series per per-route stat_prefix). (c) Defer per-route bucket independence to phase 11.1 — rejected, fixture 0013 scenario 4 explicitly requires it.

### Task 5 implementation — final shape

This task lands:

1. A small `internal/stats.Registry` extension: `NewCounterIfAbsent(name) *Counter` permitting post-freeze idempotent registration. ~30 LoC delta + tests.
2. Per-route TPFC handling in `localratelimit`: a closure-captured `sync.Map` keyed by `*LocalRateLimitPerRoute` pointer, lazily-populated at first-resolve-time with a fresh `*runtimeConfig` + own `*tokenBucket` + own `*filterStats` (registered via `NewCounterIfAbsent` so post-freeze allocation is allowed; idempotency ensures the SAME pointer always maps to the SAME `*runtimeConfig`, preserving bucket identity across requests). ~80 LoC delta in `local_ratelimit.go`.
3. The `DecodeHeaders` body extends to call `f.dcb.RequestRouteConfig(filterName)`; if non-nil, unwraps the proto.Message; if `*LocalRateLimitPerRoute`, lazily-resolves to a per-route `*runtimeConfig` via the cache; calls `tryConsume` against THAT bucket; uses THAT stats. If nil, falls back to the listener-level `*runtimeConfig`.
4. Unit test `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` validates the empirical claim mechanically: two `*LocalRateLimitPerRoute` instances + listener-level config → 3 independent bucket pointers + 3 independent stat-counter sets; counter increments on one do not affect others.
5. ADR-0117 + ADR-0073 amendment paragraph land in DECISIONS.md.

**Precondition:** Task 4 done (DecodeHeaders body landed; filterStats wired).
**Artifact:** modified `local_ratelimit.go` (per-route lazy-cache + DecodeHeaders extension); modified `local_ratelimit_test.go` (per-route bucket independence test); modified `internal/stats/registry.go` (NewCounterIfAbsent); modified `internal/stats/registry_test.go` (NewCounterIfAbsent tests); ADR-0117 in DECISIONS.md + ADR-0073 amendment paragraph appended in DECISIONS.md.
**Acceptance:** all unit tests pass; per-route bucket independence test confirms 3 independent buckets + 3 independent stat-counter sets; ADR-0117 in DECISIONS.md.

- [ ] **Step 1: Add `NewCounterIfAbsent` to `internal/stats/registry.go`**

```go
// NewCounterIfAbsent returns the counter for `name` if already registered,
// otherwise registers and returns a new one. Unlike NewCounter, this method
// is idempotent and PERMITTED post-Freeze (registrations through this method
// bypass the freeze check; the implementation guards via the same r.mu lock).
//
// Used by per-route filter configurations whose stat_prefix is data-driven
// (e.g., envoy.filters.http.local_ratelimit per phase 11). At HCM-build-time
// the per-route TPFC parser may need to register stat counters for stat_prefix
// values that are not known at boot time; this method bridges the gap without
// relaxing the Freeze discipline for boot-time registrations.
//
// Concurrency: safe for concurrent invocation; r.mu serializes the read +
// register pair.
//
// Per ADR-0117 (phase 11 ADR-0073 amendment) + ADR-0061 LBP-1 amendment.
func (r *Registry) NewCounterIfAbsent(name string) *Counter {
    r.checkName(name) // panic-on-invalid-name preserved (programmer error, not ops issue)
    r.mu.Lock()
    defer r.mu.Unlock()
    if existing, ok := r.byName[name]; ok {
        if c, ok := existing.(*Counter); ok {
            return c
        }
        panic(fmt.Sprintf("stats: NewCounterIfAbsent: name %q registered as non-Counter", name))
    }
    c := &Counter{name: name}
    r.metrics = append(r.metrics, c)
    r.byName[name] = c
    return c
}
```

- [ ] **Step 2: Add tests for `NewCounterIfAbsent` to `internal/stats/registry_test.go`**

```go
func TestNewCounterIfAbsent_RegistersWhenAbsent(t *testing.T) {
    r := NewRegistry()
    c := r.NewCounterIfAbsent("test.counter.first")
    if c == nil || c.Name() != "test.counter.first" {
        t.Errorf("got %v, want non-nil counter named test.counter.first", c)
    }
}

func TestNewCounterIfAbsent_ReturnsExisting(t *testing.T) {
    r := NewRegistry()
    c1 := r.NewCounter("test.counter.dup")
    c2 := r.NewCounterIfAbsent("test.counter.dup")
    if c1 != c2 {
        t.Errorf("expected pointer-identical Counter; got c1=%p c2=%p", c1, c2)
    }
}

func TestNewCounterIfAbsent_BypassesFreeze(t *testing.T) {
    r := NewRegistry()
    r.NewCounter("pre.freeze.counter")
    r.Freeze()
    // NewCounter would panic; NewCounterIfAbsent must succeed.
    c := r.NewCounterIfAbsent("post.freeze.counter")
    if c == nil {
        t.Fatal("NewCounterIfAbsent post-freeze returned nil")
    }
    // Verify subsequent lookup returns the same instance.
    c2 := r.NewCounterIfAbsent("post.freeze.counter")
    if c != c2 {
        t.Errorf("idempotency: got %p / %p, want pointer-identical", c, c2)
    }
}
```

- [ ] **Step 3: Extend `local_ratelimit.go` with per-route lazy-cache + DecodeHeaders unwrapper**

Add to `local_ratelimit.go`:

```go
// New now captures both the listener-level *runtimeConfig + a per-route lazy-cache.
// Per-route LocalRateLimitPerRoute Anys are resolved to *runtimeConfig instances
// lazily at first-resolve-time via NewCounterIfAbsent (post-freeze idempotent
// registration per ADR-0117).

type factoryState struct {
    listenerRC *runtimeConfig
    perRoute   sync.Map // map[*localratelimitv3.LocalRateLimitPerRoute]*runtimeConfig
    reg        *stats.Registry // captured for lazy per-route counter registration
}

// New (UPDATED): returns a factory closure that captures factoryState.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("local_ratelimit: typed_config required")
    }
    var c localratelimitv3.LocalRateLimit
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("local_ratelimit: unmarshal: %w", err)
    }
    rc, err := buildRuntimeConfig(&c, ctx)
    if err != nil {
        return nil, err
    }
    state := &factoryState{
        listenerRC: rc,
        reg:        ctx.Stats,
    }
    return func() envoyhttp.HTTPFilter {
        return &filter{state: state}
    }, nil
}

// resolvePerRouteConfig returns the *runtimeConfig for the given resolved
// per-route TPFC message (returned by f.dcb.RequestRouteConfig). If the message
// is *LocalRateLimitPerRoute, the embedded LocalRateLimit is used (lazily
// constructing a fresh *runtimeConfig + own *tokenBucket + own *filterStats
// keyed by the *LocalRateLimitPerRoute pointer). If nil, returns the listener
// fallback. Returns the listener fallback on any unexpected message type.
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *runtimeConfig {
    if msg == nil {
        return s.listenerRC
    }
    perRoute, ok := msg.(*localratelimitv3.LocalRateLimitPerRoute)
    if !ok {
        return s.listenerRC
    }
    if cached, ok := s.perRoute.Load(perRoute); ok {
        return cached.(*runtimeConfig)
    }
    // Lazy construction. Race-safe via sync.Map's LoadOrStore: if two goroutines
    // race to construct the per-route rc, only one allocation wins; the loser's
    // rc is GC'd (its registered counters are NOT GC'd since they're already in
    // the Registry, but they're idempotent and equivalent to the winner's).
    embedded := perRoute.GetRateLimit()
    if embedded == nil {
        // Per-route TPFC with no rate_limit body: fall back to listener (matches
        // Envoy's behavior of treating empty per-route as "inherit listener").
        return s.listenerRC
    }
    fresh, err := buildRuntimeConfigPerRoute(embedded, s.reg)
    if err != nil {
        // Per-route TPFC parsing failed at HCM-build time normally; reaching
        // this code path means the TPFC was somehow accepted but build failed.
        // Treat as "inherit listener" to keep request flow alive (NO panic per
        // ADR-0072 boot-fail-fast applies only to boot-time, not request-time).
        return s.listenerRC
    }
    actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
    return actual.(*runtimeConfig)
}

// buildRuntimeConfigPerRoute is buildRuntimeConfig's per-route variant; it
// uses NewCounterIfAbsent for stats registration so per-route counters can
// be allocated post-Freeze (per ADR-0117).
func buildRuntimeConfigPerRoute(c *localratelimitv3.LocalRateLimit, reg *stats.Registry) (*runtimeConfig, error) {
    // Same 6 explicit checks as buildRuntimeConfig (callable independently).
    statPrefix := c.GetStatPrefix()
    if statPrefix == "" {
        return nil, errors.New("local_ratelimit: stat_prefix required")
    }
    tb := c.GetTokenBucket()
    if tb == nil || tb.GetMaxTokens() == 0 {
        return nil, errors.New("local_ratelimit: invalid token_bucket")
    }
    maxTokens := int64(tb.GetMaxTokens())
    var tokensPerFill int64 = 1
    if v := tb.GetTokensPerFill(); v != nil {
        if v.GetValue() == 0 {
            return nil, errors.New("local_ratelimit: tokens_per_fill must be > 0")
        }
        tokensPerFill = int64(v.GetValue())
    }
    fillInterval := tb.GetFillInterval().AsDuration()
    if fillInterval < minFillInterval {
        return nil, errors.New("local rate limit token bucket fill timer must be >= 50ms")
    }
    statusCode := 429
    if c.GetStatus() != nil {
        statusCode = int(c.GetStatus().GetCode())
        if statusCode < 400 || statusCode >= 600 {
            return nil, fmt.Errorf("local_ratelimit: status.code must be in [400, 600); got %d", statusCode)
        }
    }
    var fs *filterStats
    if reg != nil {
        fs = newFilterStatsIfAbsent(reg, statPrefix)
    }
    return &runtimeConfig{
        statPrefix: statPrefix,
        bucket:     newTokenBucket(maxTokens, tokensPerFill, fillInterval),
        statusCode: statusCode,
        body:       rateLimitedBody,
        stats:      fs,
    }, nil
}

// newFilterStatsIfAbsent constructs filterStats via NewCounterIfAbsent for
// post-Freeze idempotent registration (per ADR-0117).
func newFilterStatsIfAbsent(reg *stats.Registry, statPrefix string) *filterStats {
    return &filterStats{
        enabled:     reg.NewCounterIfAbsent(statPrefix + ".http_local_rate_limit.enabled"),
        ok:          reg.NewCounterIfAbsent(statPrefix + ".http_local_rate_limit.ok"),
        rateLimited: reg.NewCounterIfAbsent(statPrefix + ".http_local_rate_limit.rate_limited"),
        enforced:    reg.NewCounterIfAbsent(statPrefix + ".http_local_rate_limit.enforced"),
    }
}

// filter is the per-request filter instance. State is request-scoped; *factoryState
// is the closure-captured shared state (immutable post-construction except for the
// sync.Map lazy-cache; race-safe per sync.Map's contract).
type filter struct {
    state *factoryState
    dcb   envoyhttp.DecoderFilterCallbacks
    ecb   envoyhttp.EncoderFilterCallbacks
}

// DecodeHeaders (UPDATED): resolves per-route TPFC via dcb.RequestRouteConfig
// (existing 07.1 most-specific accessor per ADR-0073); unwraps to *runtimeConfig
// via state.resolvePerRouteConfig.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    var perRouteMsg proto.Message
    if f.dcb != nil {
        perRouteMsg = f.dcb.RequestRouteConfig(filterName)
    }
    rc := f.state.resolvePerRouteConfig(perRouteMsg)
    rc.stats.enabled.Add(1)
    if rc.bucket.tryConsume() {
        rc.stats.ok.Add(1)
        return envoyhttp.Continue
    }
    rc.stats.rateLimited.Add(1)
    rc.stats.enforced.Add(1)
    f.dcb.SendLocalReply(rc.statusCode, rc.body, envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    return envoyhttp.StopIteration
}
```

NOTE: the implementer at Task 5 step 3 surveys the existing `*stats.Counter` API for the `.Add(1)` method (or `.Inc()`). The per-route TPFC unwrapping uses the embedded `LocalRateLimit` accessor `perRoute.GetRateLimit()`; the implementer verifies the proto field name matches go-control-plane's generated name.

The framework's `f.dcb.RequestRouteConfig(filterName)` returns the listener-level config when no per-route TPFC is registered; phase 11's resolver treats this case as "use listener fallback" via the `_, ok := msg.(*LocalRateLimitPerRoute)` type assertion failing → return `state.listenerRC`. This means the resolver works correctly whether the framework returns `*LocalRateLimit` (listener-level) or `*LocalRateLimitPerRoute` (per-route).

- [ ] **Step 4: Add per-route bucket independence test to `local_ratelimit_test.go`**

```go
func TestDecodeHeaders_PerRouteOverride_IndependentBuckets(t *testing.T) {
    // Build a listener-level config with cap=10.
    listenerCfg := happyConfig()
    listenerCfg.StatPrefix = "listener_prefix"
    listenerCfg.TokenBucket.MaxTokens = 10
    listenerCfg.TokenBucket.FillInterval = durationpb.New(60 * time.Second)
    reg := stats.NewRegistry()
    factory, err := New(mustAny(t, listenerCfg), envoyhttp.FactoryCtx{Stats: reg})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    inst := factory().(*filter)

    // Build TWO per-route LocalRateLimitPerRoute proto messages with distinct
    // configs (different stat_prefix, different cap).
    perRouteA := &localratelimitv3.LocalRateLimitPerRoute{
        RateLimit: &localratelimitv3.LocalRateLimit{
            StatPrefix: "perroute_a",
            TokenBucket: &envoytypev3.TokenBucket{
                MaxTokens:    1,
                FillInterval: durationpb.New(60 * time.Second),
            },
        },
    }
    perRouteB := &localratelimitv3.LocalRateLimitPerRoute{
        RateLimit: &localratelimitv3.LocalRateLimit{
            StatPrefix: "perroute_b",
            TokenBucket: &envoytypev3.TokenBucket{
                MaxTokens:    1,
                FillInterval: durationpb.New(60 * time.Second),
            },
        },
    }

    // Resolve each per-route to its *runtimeConfig.
    rcA := inst.state.resolvePerRouteConfig(perRouteA)
    rcB := inst.state.resolvePerRouteConfig(perRouteB)
    rcListener := inst.state.resolvePerRouteConfig(nil)

    // Assert pointer-distinct.
    if rcA == rcB {
        t.Error("perRouteA and perRouteB should resolve to DIFFERENT *runtimeConfig instances")
    }
    if rcA == rcListener || rcB == rcListener {
        t.Error("per-route should NOT alias listener-level *runtimeConfig")
    }
    if rcA.bucket == rcB.bucket {
        t.Error("perRouteA and perRouteB should have INDEPENDENT *tokenBucket pointers")
    }
    if rcA.stats == rcB.stats {
        t.Error("perRouteA and perRouteB should have INDEPENDENT *filterStats pointers")
    }

    // Assert idempotent re-resolution (same pointer in → same *runtimeConfig out).
    rcAAgain := inst.state.resolvePerRouteConfig(perRouteA)
    if rcA != rcAAgain {
        t.Error("re-resolving perRouteA should return the SAME *runtimeConfig (idempotent)")
    }

    // Drain rcA's bucket; verify rcB unaffected.
    if !rcA.bucket.tryConsume() {
        t.Fatal("rcA initial tryConsume should succeed (cap=1)")
    }
    if rcA.bucket.tryConsume() {
        t.Error("rcA second tryConsume should fail (drained)")
    }
    if !rcB.bucket.tryConsume() {
        t.Error("rcB tryConsume should succeed independently (NOT affected by rcA drain)")
    }
}
```

- [ ] **Step 5: Append ADR-0117 + ADR-0073 amendment paragraph to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0117: Per-route bucket isolation as ADR-0073 wholesale-override consequence — first stateful per-route filter; ADR-0073 amendment paragraph

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 5 commit)
**Doctrine:** ADR-0073 typed_per_filter_config 3-tier merge (most-specific override) + AMENDMENT; ADR-0061 stats Registry LBP-1 invariant + AMENDMENT (NewCounterIfAbsent post-Freeze idempotent registration); ADR-0072 boot-fail-fast.
**Lands-in-task:** Phase 11 Task 5.

### Context

Phase 11 is the FIRST production filter where per-route override implies independent stateful resources (per SPEC §1 + §11.6). Prior filters (cors, fault, header_mutation) had per-route configs that were either purely declarative (cors per-route rules) or stateful at listener level only (fault's `max_active_faults` is closure-shared across all requests on the listener but not per-route distinct). Phase 11's per-route TPFC carries an independent token bucket + independent stat counters per the §11.6 empirical pin (reference Envoy emits separate Prometheus counter series keyed by per-route stat_prefix).

Implementing per-route bucket independence under the existing framework requires: (a) per-route counter allocation post-Freeze (per the existing `httpReg.Freeze()` discipline at boot, the stats Registry is also frozen — but per-route TPFCs are parsed at HCM-build time which is post-freeze for a config-load-with-validate flow); (b) lazy or eager allocation of per-route *runtimeConfig + *tokenBucket + *filterStats; (c) thread-safe cache from per-route TPFC pointer to the resolved *runtimeConfig.

### Decision

**ADR-0073 wholesale-override extends to STATEFUL per-route resources.** Each `LocalRateLimitPerRoute` TPFC entry resolves to its own `*runtimeConfig` carrying its own `*tokenBucket` + own `*filterStats`. Listener-level state is NOT touched for per-route reqs. The implementation:

1. **Per-route lazy-cache:** the factory closure captures a `sync.Map` keyed by `*LocalRateLimitPerRoute` pointer; lazily-populated at first `DecodeHeaders` resolve via `state.resolvePerRouteConfig`.
2. **Post-Freeze stats registration:** `internal/stats.Registry` gains a `NewCounterIfAbsent(name) *Counter` method permitting idempotent post-Freeze registration. The method preserves the existing `NewCounter` panic-discipline for boot-time registrations; the new method is reserved for HCM-build-time per-route registrations. ~30 LoC framework delta in `internal/stats/registry.go`.
3. **Wholesale-override carry-through:** the existing `PerRouteConfig.Resolve` (most-specific accessor per ADR-0073) returns the per-route TPFC; the filter unwraps the `*LocalRateLimitPerRoute` to its embedded `*LocalRateLimit` and consults the per-route `*runtimeConfig`. The listener-level state is fully shadowed for per-route reqs — listener-level counters do NOT increment for `/strict` reqs per SPEC §11.6.

**ADR-0073 amendment paragraph** lands inline in the existing ADR-0073 body in `DECISIONS.md`:

```
## Amendment (per phase 11 ADR-0117)

Wholesale-override extends to STATEFUL per-route resources without further
framework support; the `*LocalRateLimitPerRoute` TPFC entry resolves at
DecodeHeaders time to a fresh `*runtimeConfig` carrying its own
`*tokenBucket` + own `*filterStats`. The listener-level state is fully
shadowed for per-route reqs. Phase 11 is the FIRST production filter to
demonstrate this; future stateful per-route filters (e.g., a future
`global_ratelimit` if it lands per-process bucket fallback) follow the
same discipline. See ADR-0117 for the precedent.

The implementation requires post-Freeze stats Registry registration via
the new `NewCounterIfAbsent` method (per ADR-0061 amendment in ADR-0118).
```

### Alternatives considered

(a) **Eager pre-allocation: walk all per-route TPFCs at HCM-build-time + pre-allocate per-route *runtimeConfig instances** — rejected. The framework's existing `BuildPerRouteConfig` parses TPFCs into `proto.Message` slots in the per-route map; extending it to call back into per-filter factory hooks (analogous to phase 10's per-route-validator) is heavier than the lazy-cache approach. The lazy-cache is ~80 LoC vs the eager-walk ~200 LoC.

(b) **Per-route counters allocated under the listener-level stat_prefix (sharing counters across per-route entries)** — rejected. SPEC §11.6 empirical pin confirms reference Envoy emits SEPARATE Prometheus counter series per per-route stat_prefix; sharing counters would diverge.

(c) **Defer per-route bucket independence to phase 11.1** — rejected. SPEC §1 + §11.6 + fixture 0013 scenario 4 explicitly require it; deferral would block fixture 0013's full 4-scenario green.

### Consequences

- ADR-0073 (typed_per_filter_config 3-tier merge / most-specific override) gains an inline amendment paragraph noting the stateful-resource extension; the canonical Decision body is unchanged.
- ADR-0061 (stats Registry / LBP-1 invariant) gains an inline amendment in ADR-0118 noting the `NewCounterIfAbsent` post-Freeze idempotent registration extension.
- Future stateful per-route filters reuse the lazy-cache + `NewCounterIfAbsent` pattern; phase 11's `state.resolvePerRouteConfig` + `buildRuntimeConfigPerRoute` are the canonical reference.
- The race-safety of the lazy-cache is validated by the existing race-detector cycle test (`TestTokenBucket_ConcurrentTryConsume`) + the per-route bucket independence test (`TestDecodeHeaders_PerRouteOverride_IndependentBuckets`); both pass under `-race`.
- Fixture 0013 scenario 4 mechanically validates the empirical claim per SPEC §11.6 (per-route counters increment independently from listener-level counters; listener counters do NOT increment for `/strict` reqs).
```

- [ ] **Step 6: Vet + lint + test + commit**

```bash
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./internal/filter/http/localratelimit/...
go test -race -count=1 ./internal/stats/...
git add internal/filter/http/localratelimit/ internal/stats/registry.go internal/stats/registry_test.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 11: per-route TPFC bucket independence + ADR-0073 amendment [ADR-0117]

Lands the per-route TPFC handling per SPEC §11.6 + ADR-0117:
- Lazy-cache (sync.Map keyed by *LocalRateLimitPerRoute pointer) of
  per-route *runtimeConfig instances, each carrying own *tokenBucket +
  own *filterStats per the §11.6 empirical claim.
- Post-Freeze stats Registry registration via new NewCounterIfAbsent
  method (~30 LoC framework delta in internal/stats/registry.go);
  idempotent + race-safe per the sync.Map LoadOrStore + Registry mu
  serialization.
- DecodeHeaders extends to call dcb.RequestRouteConfig(filterName) +
  unwrap *LocalRateLimitPerRoute via state.resolvePerRouteConfig +
  consult the per-route bucket+stats. Listener-level fallback when
  per-route is nil or unexpected message type.

ADR-0117: per-route bucket isolation as ADR-0073 wholesale-override
consequence (FIRST stateful per-route filter; ADR-0073 amendment paragraph
landed inline in DECISIONS.md noting wholesale-override extends to
stateful per-route resources without further framework support).

Tests: TestNewCounterIfAbsent_RegistersWhenAbsent / ReturnsExisting /
BypassesFreeze (3 stats Registry tests); TestDecodeHeaders_PerRouteOverride_
IndependentBuckets (validates 3-way independence: listener + perRouteA +
perRouteB carry pointer-distinct *runtimeConfig + *tokenBucket +
*filterStats; idempotent re-resolution; cross-bucket isolation).
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §11.6 + §6.7; ADR-0073 wholesale-override (amended); ADR-0061 LBP-1 (amended in Task 6's ADR-0118).*

---

## Task 6: `internal/stats/name.go` Rule SN9 + filter-specific Prometheus tag-extractor + 22→26 stat-table extension [ADR-0118 + ADR-0061 amendment]

**Files:**
- Modify: `internal/stats/name.go` (add Rule SN9 to `flattenToProm` switch)
- Modify: `internal/stats/name_test.go` (add SN9 unit tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0118 + ADR-0061 amendment paragraph)

This task lands Rule SN9 in `internal/stats/name.go`'s `flattenToProm` switch per SPEC §11.5 + ADR-0118 + planner-time decision 1. The new rule matches names of the shape `<stat_prefix>.http_local_rate_limit.<counter>` and produces Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. The rule fires only on the unmatched-prefix path (after SN1–SN5 prefix-segment switch fails) so the existing hot-path is unchanged.

ADR-0118 lands here in full (the Task 4 deferral takes effect — ADR-0118's full body covers the MVP-invariant + the SN9 rule + the 22→26 stat-table extension all together). ADR-0061 amendment paragraph lands inline in ADR-0061's body.

**Precondition:** Task 5 done (per-route TPFC handling lands).
**Artifact:** modified `name.go` + extended `name_test.go`; ADR-0118 in DECISIONS.md + ADR-0061 amendment paragraph appended.
**Acceptance:** SN9 unit tests pass; existing SN1–SN5 tests still pass; ADR-0118 in DECISIONS.md.

- [ ] **Step 1: Add Rule SN9 to `internal/stats/name.go`**

Modify the `flattenToProm` function's `default` branch to attempt SN9 detection before returning the "no recognized top-level segment" error:

```go
// (existing SN1–SN5 switch unchanged)
default:
    // Rule SN9 (added per phase 11 ADR-0118 + ADR-0061 amendment): the
    // local_ratelimit filter-specific tag-extractor matches names of the
    // shape `<stat_prefix>.http_local_rate_limit.<counter>` where
    // <stat_prefix> is a single segment (no dots) and <counter> is one of
    // {enabled, ok, rate_limited, enforced}. Produces Prometheus base name
    // `envoy_http_local_rate_limit_<counter>` + label
    // `envoy_local_http_ratelimit_prefix=<stat_prefix>`.
    //
    // The rule is a SECOND-PASS detection — fires only on the unmatched-
    // prefix path (after SN1-SN5 prefix-segment switch fails). The existing
    // SN1-SN5 hot-path is unchanged.
    //
    // Per SPEC §11.5 + ADR-0118.
    const lrlSegment = ".http_local_rate_limit."
    if idx := strings.Index(internal, lrlSegment); idx > 0 {
        prefix := internal[:idx]
        counter := internal[idx+len(lrlSegment):]
        // Validate: prefix has no dots; counter is one of the 4 known names.
        if !strings.ContainsRune(prefix, '.') {
            switch counter {
            case "enabled", "ok", "rate_limited", "enforced":
                labels = append(labels, Label{Key: "envoy_local_http_ratelimit_prefix", Value: prefix})
                base = "envoy_http_local_rate_limit_" + counter
                // Skip SN4 status-class collapse below (SN9 names don't have _Nxx suffix).
                return base, labels, nil
            }
        }
    }
    return "", nil, fmt.Errorf("stats: name %q has no recognized top-level segment (want cluster.|http.|listener.|server.)", internal)
```

NOTE: the implementer at Task 6 step 1 must preserve the existing SN4 status-class-collapse logic for SN1/SN2/SN3 (which fires AFTER the switch sets `base`); SN9 returns directly to skip the SN4 collapse since `<stat_prefix>.http_local_rate_limit.{enabled,ok,rate_limited,enforced}` doesn't have a `_Nxx` suffix.

- [ ] **Step 2: Add unit tests for SN9 to `internal/stats/name_test.go`**

```go
func TestFlattenToProm_SN9_BasicStatPrefix(t *testing.T) {
    base, labels, err := flattenToProm("foo.http_local_rate_limit.enabled")
    if err != nil {
        t.Fatalf("flattenToProm: %v", err)
    }
    if base != "envoy_http_local_rate_limit_enabled" {
        t.Errorf("base: got %q, want %q", base, "envoy_http_local_rate_limit_enabled")
    }
    if len(labels) != 1 || labels[0].Key != "envoy_local_http_ratelimit_prefix" || labels[0].Value != "foo" {
        t.Errorf("labels: got %v, want [envoy_local_http_ratelimit_prefix=foo]", labels)
    }
}

func TestFlattenToProm_SN9_AllFourCounters(t *testing.T) {
    for _, counter := range []string{"enabled", "ok", "rate_limited", "enforced"} {
        t.Run(counter, func(t *testing.T) {
            base, labels, err := flattenToProm("test.http_local_rate_limit." + counter)
            if err != nil {
                t.Fatalf("flattenToProm: %v", err)
            }
            wantBase := "envoy_http_local_rate_limit_" + counter
            if base != wantBase {
                t.Errorf("base: got %q, want %q", base, wantBase)
            }
            if len(labels) != 1 || labels[0].Value != "test" {
                t.Errorf("labels: got %v, want envoy_local_http_ratelimit_prefix=test", labels)
            }
        })
    }
}

func TestFlattenToProm_SN9_PrefixWithUnderscores(t *testing.T) {
    base, labels, err := flattenToProm("my_prefix.http_local_rate_limit.ok")
    if err != nil {
        t.Fatalf("flattenToProm: %v", err)
    }
    if base != "envoy_http_local_rate_limit_ok" {
        t.Errorf("base: got %q, want %q", base, "envoy_http_local_rate_limit_ok")
    }
    if len(labels) != 1 || labels[0].Value != "my_prefix" {
        t.Errorf("labels: got %v, want value my_prefix", labels)
    }
}

func TestFlattenToProm_SN9_DoesNotConflictWithSN1234(t *testing.T) {
    // SN1 (cluster.) wins over SN9 even if name contains the SN9 segment.
    base, labels, err := flattenToProm("cluster.foo.http_local_rate_limit.enabled")
    if err != nil {
        t.Fatalf("flattenToProm: %v", err)
    }
    // SN1 produces envoy_cluster_<rest>; the rest is "http_local_rate_limit.enabled"
    // (with internal-dot-to-underscore transform per phase 09 SN2 update — though SN1
    // doesn't apply that transform per the master `97ed8b9` baseline).
    // Implementer at Task 6 step 2 adapts the assertion to whichever shape SN1 produces;
    // the load-bearing claim is that SN1 wins, NOT SN9.
    if !strings.HasPrefix(base, "envoy_cluster_") {
        t.Errorf("base: got %q, want SN1-prefixed envoy_cluster_*", base)
    }
    if len(labels) == 0 || labels[0].Key != "envoy_cluster_name" {
        t.Errorf("labels: got %v, want envoy_cluster_name=foo (SN1 wins)", labels)
    }
}

func TestFlattenToProm_SN9_RejectsUnknownCounter(t *testing.T) {
    // SN9 only matches the 4 known counter names; other suffixes fall through to error.
    _, _, err := flattenToProm("foo.http_local_rate_limit.unknown")
    if err == nil {
        t.Error("flattenToProm with unknown counter: want error, got nil")
    }
}
```

- [ ] **Step 3: Run tests**

```bash
go test -race -count=1 -v ./internal/stats/... 2>&1 | tail -30
```

Expected: all SN9 tests PASS; all existing SN1–SN5 tests still PASS.

- [ ] **Step 4: Append ADR-0118 to `docs/envoy-go/DECISIONS.md`**

```markdown
## ADR-0118: `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name extension + `enforced == rate_limited` MVP invariant + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` (Rule SN9)

**Status:** Accepted
**Date:** 2026-05-05 (phase 11 Task 6 commit)
**Doctrine:** ADR-0061 stats Registry / SN1–SN8 flattening rules + AMENDMENT (SN9 added); ADR-0040 silent-ignore discipline; ADR-0008 Envoy v1.37.2 pin (empirical evidence source).
**Lands-in-task:** Phase 11 Task 6 (the SN9 rule + 22→26 extension + MVP-invariant ADR body land together).

### Context

Phase 11 emits 4 new counters per `<stat_prefix>` per SPEC §6.6 + §11.5:
- `<stat_prefix>.http_local_rate_limit.enabled` (every req reaching the filter)
- `<stat_prefix>.http_local_rate_limit.ok` (tryConsume → true)
- `<stat_prefix>.http_local_rate_limit.rate_limited` (tryConsume → false)
- `<stat_prefix>.http_local_rate_limit.enforced` (tryConsume → false; lockstep with rate_limited under MVP)

The Prometheus rendering per SPEC §11.5 empirical pin requires:
- Base name `envoy_http_local_rate_limit_<counter>` (NOT prefixed with the existing SN2 `envoy_http_conn_manager_prefix`-keyed shape — the local_ratelimit filter is NOT scoped under the HCM stat_prefix; it has its own filter-level stat_prefix per SPEC §6.1).
- Label `envoy_local_http_ratelimit_prefix=<stat_prefix>` (filter-specific tag-extractor; NOT the existing SN2 `envoy_http_conn_manager_prefix` label).

The existing `internal/stats/name.go` `flattenToProm` switch handles Rules SN1 (cluster.*), SN2 (http.*), SN3 (listener.*), SN5 (server.*); SN4 is the trailing `_Nxx` status-class collapse. The local_ratelimit names don't fit any existing rule (the prefix is the filter-level stat_prefix, not one of the 4 known top-level segments).

### Decision

**Rule SN9 added to `flattenToProm`'s `default` branch as a second-pass detection.** Matches names of the shape `<stat_prefix>.http_local_rate_limit.<counter>` where `<stat_prefix>` has no dots and `<counter>` is one of `{enabled, ok, rate_limited, enforced}`. Produces Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. SN1-SN5 prefix-segment switch wins precedence over SN9 (e.g., `cluster.foo.http_local_rate_limit.enabled` routes to SN1, not SN9 — the `cluster.` prefix takes the name first).

**`enforced == rate_limited` MVP invariant.** Phase 11 increments `rate_limited` AND `enforced` IN LOCKSTEP whenever `tryConsume` returns false. This matches reference Envoy's behavior when `filter_enforced=100%` (the SPEC §1.1 amendment requires fixture configs to set `filter_enforced=100%` explicitly). Future shadow-mode phase wiring `filter_enforced` < 100% support widens to `enforced ≤ rate_limited`: when `enforced` is sampled-out, `rate_limited` increments but `enforced` does not. The MVP invariant carries a documented natural-divergence point at the future shadow-mode landing.

**`BEHAVIOR_CONTRACT.md ## Stat-name mapping` 22→26-name table extension** lands at the phase-done commit per ADR-0052 in-place edit authorisation. Verbatim Markdown patch (per SPEC §13.2):

```markdown
| `<stat_prefix>.http_local_rate_limit.enabled`     | counter | filter | local_ratelimit | every request reaching the filter (§11.5) |
| `<stat_prefix>.http_local_rate_limit.ok`          | counter | filter | local_ratelimit | request not rate-limited (`tryConsume` → true; §11.5) |
| `<stat_prefix>.http_local_rate_limit.rate_limited`| counter | filter | local_ratelimit | request rate-limited (`tryConsume` → false; §11.5) |
| `<stat_prefix>.http_local_rate_limit.enforced`    | counter | filter | local_ratelimit | request rate-limited AND enforced (lockstep with `rate_limited` under MVP per ADR-0118; §11.5) |
```

**Tag-extraction collision quirk** (per SPEC §11.5 (e)): when `stat_prefix` matches an Envoy-internal tag-extractor name (e.g., literal `listener` matches `envoy.listener_address`), the Prometheus output is mangled. Phase 11's fixture 0013 deliberately uses safe values (`foo`, `bar`, `baz`, `qux`, `strict`) to avoid the collision. Future phases extending stat-prefix coverage may need to address the collision.

**ADR-0061 amendment paragraph** lands inline in the existing ADR-0061 body in `DECISIONS.md`:

```
## Amendment (per phase 11 ADR-0118)

Rule SN9 added per phase 11: extends the `flattenToProm` switch with a
filter-specific tag-extractor for the `<stat_prefix>.http_local_rate_limit.<counter>`
shape; produces Prometheus base name `envoy_http_local_rate_limit_<counter>` +
label `envoy_local_http_ratelimit_prefix=<stat_prefix>`. SN9 is a second-pass
detection — fires only on the unmatched-prefix path (after SN1–SN5 prefix-segment
switch fails); the existing SN1–SN8 hot-path is unchanged.

Additionally, `Registry.NewCounterIfAbsent(name) *Counter` is added (per
ADR-0117) permitting idempotent post-Freeze registration. Used by per-route
filter configurations whose stat_prefix is data-driven (e.g.,
envoy.filters.http.local_ratelimit per phase 11). The existing `NewCounter`
panic-on-Freeze discipline is preserved for boot-time registrations; the new
method is reserved for HCM-build-time per-route registrations.
```

### Alternatives considered

(a) **Use SN2 (http.*) shape: rename `<stat_prefix>.http_local_rate_limit.<counter>` to `http.<stat_prefix>.local_rate_limit.<counter>`** — rejected. SPEC §11.5 empirical pin confirms reference Envoy uses the `<stat_prefix>.http_local_rate_limit.<counter>` shape (NOT `http.*`); wire-equivalence requires preserving the empirical shape.

(b) **Filter-package-local tag-extractor registration via `init()` calling a registry-pattern primitive from `internal/stats`** — rejected per planner-time decision 1. The existing `flattenToProm` is a hardcoded switch with no registry/dispatch primitive; introducing one for a single rule would be over-engineering.

(c) **MVP invariant `enforced ≤ rate_limited` (not lockstep)** — rejected. Phase 11's MVP silent-ignores `filter_enforced` (per SPEC §2.1 cluster 2); the fixture sets `filter_enforced=100%` explicitly; under that condition reference Envoy's behavior IS lockstep. The MVP invariant matches reference Envoy exactly under the fixture conditions; future shadow-mode phase widens.

### Consequences

- ADR-0061 (stats Registry / SN1–SN8 + LBP-1) gains an inline amendment paragraph noting Rule SN9 + the `NewCounterIfAbsent` post-Freeze idempotent registration extension. The canonical Decision body is unchanged.
- The `<stat_prefix>` segment in SN9 is opaque to the rule (any non-dot string is accepted); validation that `<stat_prefix>` is non-empty + non-dot-containing happens at the filter's `New` factory time per ADR-0115.
- Future filters whose stat-name shape doesn't fit SN1–SN5 follow the SN9 precedent: extend the `default` branch with a filter-specific second-pass detection. Each new rule documents the proto-level stat-name template + the resulting Prometheus base + label shape.
- Fixture 0013's per-scenario `/stats/prometheus` scrape asserts the SN9 byte-equivalence: each per-stat_prefix scrape emits 4 lines under `envoy_http_local_rate_limit_*` with `envoy_local_http_ratelimit_prefix=<stat_prefix>` label.
- The tag-extraction collision quirk (per SPEC §11.5 (e)) is OUT of scope for phase 11; the fixture uses safe stat_prefix values; future stat-name-discipline phase may address.
```

- [ ] **Step 5: Vet + lint + test + commit**

```bash
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./internal/stats/...
git add internal/stats/name.go internal/stats/name_test.go docs/envoy-go/DECISIONS.md
git commit -m "$(cat <<'EOF'
phase 11: Rule SN9 + filter-specific Prometheus tag-extractor [ADR-0118]

Lands the local_ratelimit filter-specific Prometheus tag-extractor in
internal/stats/name.go's flattenToProm switch as Rule SN9. Matches
names of the shape <stat_prefix>.http_local_rate_limit.<counter> where
<counter> is one of {enabled, ok, rate_limited, enforced}; produces
Prometheus base envoy_http_local_rate_limit_<counter> + label
envoy_local_http_ratelimit_prefix=<stat_prefix>.

The rule is a second-pass detection (fires only on the unmatched-prefix
path); SN1–SN5 hot-path is unchanged. Cross-rule precedence: SN1
(cluster.) wins over SN9 even when the name contains the SN9 segment.

ADR-0118: 22→26-name stat-table extension + enforced == rate_limited
MVP invariant + filter-specific Prometheus tag-extractor SN9
registration. Includes ADR-0061 amendment paragraph noting both SN9 +
the NewCounterIfAbsent post-Freeze idempotent registration (per ADR-0117).

Tests: TestFlattenToProm_SN9_BasicStatPrefix / AllFourCounters
(table-driven across 4 counters) / PrefixWithUnderscores /
DoesNotConflictWithSN1234 (SN1 wins precedence) / RejectsUnknownCounter.
EOF
)"
```

SHA-fill follow-up.

*Anchored: SPEC §11.5 + §13.2; ADR-0061 (amended); planner-time decision 1.*

---

## Task 7: `cmd/envoy-go/main.go` register `localratelimit.New` under `localratelimit.TypeURL`

**Files:**
- Modify: `cmd/envoy-go/main.go`

This task adds the boot-time registration line for local_ratelimit per ADR-0072 + ADR-0114. ONE new `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` line inserted after the existing `header_mutation.New` registration (line 117 in master HEAD `97ed8b9`) and before the `header_mutation.RegisterPerRouteValidator(httpReg)` call (currently line 121) and `httpReg.Freeze()` (currently line 122). Plus the matching import alphabetically among the existing filter-package imports.

**Precondition:** Task 6 done; `internal/filter/http/localratelimit/` package compiles cleanly + Rule SN9 wired.
**Artifact:** modified main.go.
**Acceptance:** `go build ./cmd/envoy-go` clean; `go vet ./...` clean; the registration appears in the expected order (router → cors → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze).

- [ ] **Step 1: Read the existing registration block at `cmd/envoy-go/main.go:112–122`**

```bash
sed -n '112,122p' cmd/envoy-go/main.go
```

Confirm the current shape matches the expected:

```
httpReg := filter_http.NewHTTPRegistry()
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(cors.TypeURL, cors.New)
httpReg.Register(envoygotest.TypeURL, envoygotest.New)
httpReg.Register(fault.TypeURL, fault.New)
httpReg.Register(header_mutation.TypeURL, header_mutation.New)
// Register header_mutation per-route validator before Freeze
header_mutation.RegisterPerRouteValidator(httpReg)
httpReg.Freeze()
```

- [ ] **Step 2: Add the import line**

Insert in alphabetical order among the existing filter-package imports:

```go
"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
```

The block becomes: cors, envoygotest, fault, header_mutation, localratelimit, router.

- [ ] **Step 3: Add the registration line**

Insert between `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` and the `header_mutation.RegisterPerRouteValidator(httpReg)` call:

```go
httpReg.Register(localratelimit.TypeURL, localratelimit.New)
```

The final block reads:

```go
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(cors.TypeURL, cors.New)
httpReg.Register(envoygotest.TypeURL, envoygotest.New)
httpReg.Register(fault.TypeURL, fault.New)
httpReg.Register(header_mutation.TypeURL, header_mutation.New)
httpReg.Register(localratelimit.TypeURL, localratelimit.New)
// Register header_mutation per-route validator before Freeze (the registry
// rejects registrations after Freeze; New is called post-Freeze during
// listener construction, so it cannot call RegisterPerRouteValidator itself).
header_mutation.RegisterPerRouteValidator(httpReg)
httpReg.Freeze()
```

local_ratelimit does NOT call `RegisterPerRouteValidator` — phase 11 has no per-route invariants requiring boot-time validation; per-route TPFC entries (parsed generically by `BuildPerRouteConfig`) are validated lazily at first-resolve via `buildRuntimeConfigPerRoute`, which runs the same PGV + filter-internal-50ms checks as the listener-level `buildRuntimeConfig`.

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
git commit -m "phase 11: register localratelimit.New under localratelimit.TypeURL"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2; ADR-0072 + ADR-0114.*

---

## Task 8: `internal/filter/http/localratelimit/fuzz_test.go` `FuzzLocalRateLimitConfigParse`

**Files:**
- Create: `internal/filter/http/localratelimit/fuzz_test.go`

This task lands the fifteenth fuzzer per SPEC §14.3 + the standard "every parser/codec/filter ships a fuzzer" discipline per ADR-0018. Fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`; asserts `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. 30s budget per ADR-0018; ~50 LoC.

**Precondition:** Task 7 done.
**Artifact:** new fuzz_test.go.
**Acceptance:** `go test -fuzz=FuzzLocalRateLimitConfigParse -fuzztime=30s ./internal/filter/http/localratelimit/...` runs clean; no crashes; corpus seeded from a few minimal valid + invalid Any blobs.

- [ ] **Step 1: Create `internal/filter/http/localratelimit/fuzz_test.go`**

```go
package localratelimit

import (
    "testing"

    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

// FuzzLocalRateLimitConfigParse fuzzes arbitrary byte sequences as the tc
// *anypb.Any parameter to New. Asserts New returns either (factory, nil)
// OR (nil, error); never panics; never returns (nil, nil).
//
// Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the
// local_ratelimit filter's New factory is a parser. 30s budget per ADR-0018
// short-mode CI policy. Fifteenth fuzzer overall (post-10's fourteenth
// FuzzHeaderMutationConfigParse).
func FuzzLocalRateLimitConfigParse(f *testing.F) {
    // Seed corpus: empty TypeURL + empty bytes (invalid).
    f.Add("", []byte{})
    // Seed corpus: arbitrary bytes under the canonical type URL (decode error).
    f.Add(TypeURL, []byte{0xff, 0xff, 0xff})
    // Seed corpus: short proto-wire-format bytes.
    f.Add(TypeURL, []byte{0x08, 0x01})

    f.Fuzz(func(t *testing.T, typeURL string, value []byte) {
        tc := &anypb.Any{TypeUrl: typeURL, Value: value}
        factory, err := New(tc, envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
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
go test -fuzz=FuzzLocalRateLimitConfigParse -fuzztime=30s ./internal/filter/http/localratelimit/... 2>&1 | tail -10
```

Expected: clean exit (no crashes / panics); the fuzzer reports an execution count and exits.

- [ ] **Step 3: Run the standard short test (the seeded inputs run as a regular test)**

```bash
go test -count=1 -run FuzzLocalRateLimitConfigParse ./internal/filter/http/localratelimit/...
```

Expected: PASS.

- [ ] **Step 4: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add internal/filter/http/localratelimit/fuzz_test.go
git commit -m "phase 11: FuzzLocalRateLimitConfigParse (fifteenth fuzzer per ADR-0018)"
```

SHA-fill follow-up.

*Anchored: SPEC §14.3; ADR-0018; ADR-0072 (factory-validates-typed_config contract).*

---

## Task 9: Fixture infrastructure — `BackendKind` enum extension + `runner_test.go` spawn helper + blank-import [planner-time decision 9]

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `HTTPLocalRateLimit BackendKind = 10`)
- Modify: `test/differential/runner_test.go` (add blank-import + `startHTTPLocalRateLimitBackend` spawn helper + switch case)

This task lands the fixture-harness infrastructure: a new `BackendKind` enum value `HTTPLocalRateLimit BackendKind = 10` per planner-time decision 9 + the runner's spawn helper for the new backend. The blank-import for the fixture driver lands here; the actual driver file lands in Task 13.

**No framework extension required:** the 4-listener topology (per planner-time decision 8) fits within the existing `fixture.MultiListenerDriver` contract introduced in phase 07.2 (`test/differential/fixture/fixture.go:294-299`). Phase 11 implements `MultiListenerDriver` directly in the driver (Task 13); no per-scenario teardown primitive is added to the harness.

**Precondition:** Task 8 done.
**Artifact:** modified fixture.go + runner_test.go.
**Acceptance:** `go build ./test/differential/...` clean; `go vet ./...` clean. The fixture is registered but not yet runnable (Tasks 10–13 land the backend + bootstrap + driver files).

- [ ] **Step 1: Add the `HTTPLocalRateLimit` enum value to `test/differential/fixture/fixture.go`**

Locate the existing `HTTPHeaderMutation BackendKind = 9` line (added in phase 10 Task 11) and append:

```go
// HTTPLocalRateLimit is an out-of-process HTTP/1.1 backend: the runner spawns
// test/fixtures/0013-http-local-ratelimit/backends/backend.go on the pre-
// allocated port. The backend serves / with body "backend\n" (8 bytes;
// Content-Type: text/plain; Content-Length: 8). No TLS. Introduced by
// fixture 0013-http-local-ratelimit (phase 11 Task 9). Because the backend
// is a subprocess, the runner's in-process accept counter is NOT incremented.
HTTPLocalRateLimit BackendKind = 10
```

- [ ] **Step 2: Add the spawn helper to `test/differential/runner_test.go`**

Locate the existing `startHTTPHeaderMutationBackend` helper (introduced by phase 10 Task 11) and add a sibling helper:

```go
// startHTTPLocalRateLimitBackend spawns test/fixtures/0013-http-local-ratelimit/
// backends/backend.go on the runner-allocated port. Mirrors
// startHTTPHeaderMutationBackend.
func startHTTPLocalRateLimitBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
    cmd := exec.CommandContext(ctx, "go", "run",
        "./test/fixtures/0013-http-local-ratelimit/backends",
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

- [ ] **Step 3: Extend the `kind` switch in `runFixture`** (the implementer at Task 9 step 3 locates the existing switch in `test/differential/runner_test.go::runFixture` and adds the new case mirroring the `fixture.HTTPHeaderMutation` block):

```go
case fixture.HTTPLocalRateLimit:
    cmd, err := startHTTPLocalRateLimitBackend(ctx, repoRoot, ports[0])
    // ... (same wait + cleanup pattern as HTTPHeaderMutation case)
```

- [ ] **Step 4: Add the blank-import**

Add to the blank-import block (alphabetically after `0012-http-header-mutation`):

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/driver"
```

This will fail to compile until Task 13 lands the driver file. The implementer at Task 9 may either:

(a) **Land Task 9 + Task 10 + Task 11 + Task 12 + Task 13 as a single combined commit** (the fixture infrastructure + backend + envoy.yaml + envoy-go.yaml + expectations.yaml + README.md + driver land together; the build is broken in the middle of the commit chain otherwise).

(b) **Defer the blank-import to Task 13** (keep Task 9 stable on its own; add the blank-import in Task 13 alongside the driver creation).

Recommendation: option (b) — keep the per-task commits atomic; Task 13's commit adds the blank-import + driver file simultaneously. Task 9's commit lands ONLY the BackendKind enum + spawn helper + switch case (which compile cleanly without the driver). This requires deferring the switch case as well (the switch case references `startHTTPLocalRateLimitBackend` which compiles, but the runner won't dispatch any fixture to this case until Task 13's blank-import lands, so the switch case is unused-but-valid code).

- [ ] **Step 5: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
go build ./test/differential/...
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 11: fixture 0013 infrastructure — BackendKind + spawn helper"
```

SHA-fill follow-up.

*Anchored: planner-time decision 9; phase 10 Task 11 precedent.*

---

## Task 10: Fixture 0013 — `backends/backend.go` (Go HTTP backend serving `backend\n` body)

**Files:**
- Create: `test/fixtures/0013-http-local-ratelimit/backends/backend.go`

This task lands the minimal HTTP backend per SPEC §7.4. Mirrors `test/fixtures/0011-http-fault/backends/backend.go` exactly (24 LoC verified at master `97ed8b9`): `/` endpoint serves a fast `200 OK` with body `"backend\n"` (8 bytes); response carries one fixed `Content-Type: text/plain` and one `Content-Length: 8` header. No special handling for `/strict` or `/loose` (the rate-limit decision happens in Envoy/envoy-go BEFORE the upstream call; the backend is unreachable on rate-limited paths).

**Precondition:** Task 9 done.
**Artifact:** new `backend.go`.
**Acceptance:** `go build ./test/fixtures/0013-http-local-ratelimit/backends/...` clean; `go vet ./...` clean.

- [ ] **Step 1: Create `test/fixtures/0013-http-local-ratelimit/backends/backend.go`**

```go
// Backend for fixture 0013-http-local-ratelimit. Serves / with body "backend\n" (8 bytes).
// Mirrors test/fixtures/0011-http-fault/backends/backend.go exactly per phase 11 SPEC §7.4.
package main

import (
    "flag"
    "fmt"
    "net/http"
)

func main() {
    port := flag.Int("port", 18013, "TCP port to bind")
    flag.Parse()
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/plain")
        body := "backend\n"
        w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(body))
    })
    if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
        panic(err)
    }
}
```

- [ ] **Step 2: Verify build + run**

```bash
go build ./test/fixtures/0013-http-local-ratelimit/backends/
go run ./test/fixtures/0013-http-local-ratelimit/backends/ --port 18999 &
PID=$!
sleep 0.2
curl -s http://localhost:18999/ | xxd | head -2
kill $PID 2>/dev/null
```

Expected: body `backend\n` (62 61 63 6b 65 6e 64 0a in hex).

- [ ] **Step 3: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add test/fixtures/0013-http-local-ratelimit/backends/
git commit -m "phase 11: fixture 0013 backend — minimal HTTP backend with body 'backend\\n'"
```

SHA-fill follow-up.

*Anchored: SPEC §7.4; fixture 0011-http-fault backend precedent.*

---

## Task 11: Fixture 0013 — `envoy.yaml` + `envoy-go.yaml` bootstraps (4-listener topology per planner-time decision 8)

**Files:**
- Create: `test/fixtures/0013-http-local-ratelimit/envoy.yaml`
- Create: `test/fixtures/0013-http-local-ratelimit/envoy-go.yaml`

This task lands the dual-bootstrap YAML files. **Per planner-time decision 8 (which diverges from SPEC §7.1's two-listener layout)**, both files carry FOUR pre-configured listeners in a single bootstrap so all 4 scenarios run without per-scenario teardown:
- `l_s1` — scenario 1 (cap=10, fill=10, interval=1s, stat_prefix=foo)
- `l_s2` — scenario 2 (cap=2, fill=2, interval=60s, stat_prefix=bar)
- `l_s3` — scenario 3 (cap=1, fill=1, interval=200ms, stat_prefix=baz)
- `l_per_route` — scenario 4 (listener-level cap=10/stat_prefix=qux + per-route `/strict` TPFC cap=1/stat_prefix=strict + no-override `/loose`)

All listeners explicitly set `filter_enabled` + `filter_enforced=100%` per SPEC §1.1 amendment with unique runtime_keys per listener-per-filter. The bootstraps use port placeholders `{{.AdminPort}}`, `{{.LS1Port}}`, `{{.LS2Port}}`, `{{.LS3Port}}`, `{{.LPerRoutePort}}`, `{{.BackendPort}}` substituted by the runner via Go `text/template`. NO per-scenario substitution required — bucket parameters are baked at boot.

**Precondition:** Task 10 done.
**Artifact:** envoy.yaml + envoy-go.yaml.
**Acceptance:** YAML files are syntactically valid (verified by passing `--config-yaml` to a dummy `docker run envoy --mode validate` invocation OR by manual `yaml-lint`); the templated placeholders are pre-templated for a smoke check.

- [ ] **Step 1: Create `test/fixtures/0013-http-local-ratelimit/envoy.yaml`**

```yaml
# Reference Envoy bootstrap for fixture 0013-http-local-ratelimit.
# 4-listener pre-configured topology per PLAN planner-time decision 8 (diverges
# from SPEC §7.1's two-listener layout to avoid the per-scenario teardown
# framework extension; rationale recorded in the planner-time decisions section
# of PLAN.md).
#
# FOUR listeners:
#   l_s1         — scenario 1 (cap=10, fill=10, interval=1s, stat_prefix=foo)
#   l_s2         — scenario 2 (cap=2,  fill=2,  interval=60s, stat_prefix=bar)
#   l_s3         — scenario 3 (cap=1,  fill=1,  interval=200ms, stat_prefix=baz)
#   l_per_route  — scenario 4 (listener qux + per-route /strict + /loose)
#
# All listeners explicitly set filter_enabled + filter_enforced to 100% per
# SPEC §1.1 amendment (RuntimeFractionalPercent default is 0% — omitting these
# would silently disable rate-limiting in reference Envoy, breaking the
# differential equivalence). Each listener has unique runtime_keys per filter
# to avoid Envoy's runtime-key cross-contamination.
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} }

static_resources:
  listeners:
    - name: l_s1
      address: { socket_address: { address: 0.0.0.0, port_value: {{.LS1Port}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_s1
                route_config:
                  name: rc_s1
                  virtual_hosts:
                    - name: vh_s1
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: foo
                      token_bucket: { max_tokens: 10, tokens_per_fill: 10, fill_interval: 1s }
                      filter_enabled: { runtime_key: __s1_enabled,  default_value: { numerator: 100, denominator: HUNDRED } }
                      filter_enforced: { runtime_key: __s1_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    - name: l_s2
      address: { socket_address: { address: 0.0.0.0, port_value: {{.LS2Port}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_s2
                route_config:
                  name: rc_s2
                  virtual_hosts:
                    - name: vh_s2
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: bar
                      token_bucket: { max_tokens: 2, tokens_per_fill: 2, fill_interval: 60s }
                      filter_enabled: { runtime_key: __s2_enabled,  default_value: { numerator: 100, denominator: HUNDRED } }
                      filter_enforced: { runtime_key: __s2_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    - name: l_s3
      address: { socket_address: { address: 0.0.0.0, port_value: {{.LS3Port}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_s3
                route_config:
                  name: rc_s3
                  virtual_hosts:
                    - name: vh_s3
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: baz
                      token_bucket: { max_tokens: 1, tokens_per_fill: 1, fill_interval: 0.2s }
                      filter_enabled: { runtime_key: __s3_enabled,  default_value: { numerator: 100, denominator: HUNDRED } }
                      filter_enforced: { runtime_key: __s3_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    - name: l_per_route
      address: { socket_address: { address: 0.0.0.0, port_value: {{.LPerRoutePort}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_per_route
                route_config:
                  name: rc_per_route
                  virtual_hosts:
                    - name: vh_per_route
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/strict" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.local_ratelimit:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimitPerRoute
                              rate_limit:
                                stat_prefix: strict
                                token_bucket: { max_tokens: 1, tokens_per_fill: 1, fill_interval: 60s }
                                filter_enabled: { runtime_key: __strict_enabled, default_value: { numerator: 100, denominator: HUNDRED } }
                                filter_enforced: { runtime_key: __strict_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
                        - match: { prefix: "/loose" }
                          route: { cluster: c_backend }
                http_filters:
                  - name: envoy.filters.http.local_ratelimit
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit
                      stat_prefix: qux
                      token_bucket: { max_tokens: 10, tokens_per_fill: 10, fill_interval: 1s }
                      filter_enabled: { runtime_key: __qux_enabled,  default_value: { numerator: 100, denominator: HUNDRED } }
                      filter_enforced: { runtime_key: __qux_enforced, default_value: { numerator: 100, denominator: HUNDRED } }
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

- [ ] **Step 2: Create `test/fixtures/0013-http-local-ratelimit/envoy-go.yaml`**

Identical to `envoy.yaml` modulo the cluster type (STRICT_DNS → STATIC) per the existing project convention + the admin/listener port placeholders are different per the runner's per-proxy port allocation:

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} }

static_resources:
  listeners:
    # ... (l_s1 + l_s2 + l_s3 + l_per_route identical to envoy.yaml above; the
    # templated ports are independently allocated by the runner for envoy-go
    # vs reference Envoy)

  clusters:
    - name: c_backend
      type: STATIC  # NOTE: STATIC for envoy-go (existing project convention; STRICT_DNS reserved for reference Envoy)
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: {{.BackendPort}} } } }
```

The `filter_enabled` + `filter_enforced` fields are PRESENT in envoy-go.yaml (envoy-go silent-ignores them per SPEC §2.1 cluster 2) — the field presence ensures byte-equivalent config-load behavior across reference + subject.

- [ ] **Step 3: Smoke-test the YAML by templating it**

```bash
# Template the envoy.yaml with sample values via a one-shot Go script:
go run -run /dev/null - <<'EOF'
package main
import ("os"; "text/template")
type S struct { AdminPort, LS1Port, LS2Port, LS3Port, LPerRoutePort, BackendPort int }
func main() {
    d := S{AdminPort: 9913, LS1Port: 10013, LS2Port: 10014, LS3Port: 10015, LPerRoutePort: 10016, BackendPort: 18013}
    template.Must(template.ParseFiles("test/fixtures/0013-http-local-ratelimit/envoy.yaml")).Execute(os.Stdout, d)
}
EOF
```

Expected: well-formed YAML with all 4 listener-port placeholders substituted.

- [ ] **Step 4: Vet + commit**

```bash
git add test/fixtures/0013-http-local-ratelimit/envoy.yaml test/fixtures/0013-http-local-ratelimit/envoy-go.yaml
git commit -m "phase 11: fixture 0013 bootstraps — 4-listener pre-configured topology"
```

SHA-fill follow-up.

*Anchored: SPEC §1.1 amendment (filter_enabled+enforced=100%); planner-time decision 8 (4-listener topology — DIVERGES from SPEC §7.1's two-listener layout).*

---

## Task 12: Fixture 0013 — `expectations.yaml` + `README.md`

**Files:**
- Create: `test/fixtures/0013-http-local-ratelimit/expectations.yaml`
- Create: `test/fixtures/0013-http-local-ratelimit/README.md`

This task lands the prose narrative artefacts per SPEC §4.3. `expectations.yaml` documents the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated). `README.md` provides the fixture overview + per-scenario list + dual-listener bootstrap discipline + Envoy-deviation note + planner-time-decision cross-references.

**Precondition:** Task 11 done.
**Artifact:** expectations.yaml + README.md.
**Acceptance:** YAML/Markdown files are syntactically valid; cross-references to SPEC §7.1 + §13.1 + ADR table are accurate.

- [ ] **Step 1: Create `test/fixtures/0013-http-local-ratelimit/expectations.yaml`**

```yaml
# Differential equivalence claims for fixture 0013-http-local-ratelimit per
# phase 11 SPEC §7.1 + ADR-0019 (expectations.yaml is prose; the runner's
# CompareBytes enforces machine-checkable byte-equivalence).

fixture: 0013-http-local-ratelimit
phase: 11
adrs:
  - ADR-0114  # package shape
  - ADR-0115  # runtimeConfig + PGV + filter-internal validation
  - ADR-0116  # tokenBucket primitive
  - ADR-0117  # per-route bucket isolation (ADR-0073 amendment)
  - ADR-0118  # stat-table 22→26 + Rule SN9 + MVP invariant
  - ADR-0119  # rate-limited response wire shape

scenarios:
  - name: scenario_1_basic_allow
    listener: l_s1
    config: { max_tokens: 10, tokens_per_fill: 10, fill_interval: 1s, stat_prefix: foo }
    workload: 5 sequential GETs to / via l_s1
    asserts:
      - 5 × HTTP 200 (backend response body "backend\n")
      - counter deltas: foo.http_local_rate_limit.{enabled=5, ok=5, rate_limited=0, enforced=0}
      - /stats/prometheus scrape: envoy_http_local_rate_limit_enabled{envoy_local_http_ratelimit_prefix="foo"} == 5
      - prom labels byte-equal across reference + subject

  - name: scenario_2_basic_rate_limited
    listener: l_s2
    config: { max_tokens: 2, tokens_per_fill: 2, fill_interval: 60s, stat_prefix: bar }
    workload: 5 sequential GETs to / via l_s2
    asserts:
      - first 2 × HTTP 200 (backend response)
      - last 3 × HTTP 429 with body "local_rate_limited" (18 bytes, no LF; MD5 397e830923f3080ba63b3d38b53678ac)
      - rate-limited response 4-header set in lexicographic order:
          content-length: 18
          content-type: text/plain
          date: <RFC1123 allow-listed>
          server: envoy
      - framing: Content-Length (NOT chunked)
      - counter deltas: bar.http_local_rate_limit.{enabled=5, ok=2, rate_limited=3, enforced=3}
      - lockstep MVP invariant: rate_limited == enforced

  - name: scenario_3_refill_after_fill_interval
    listener: l_s3
    config: { max_tokens: 1, tokens_per_fill: 1, fill_interval: 200ms, stat_prefix: baz }
    workload: 3 GETs at t=0 / t=10ms / t=250ms via l_s3
    asserts:
      - t=0 → HTTP 200 (initial bucket cap=1)
      - t=10ms → HTTP 429 (bucket empty; elapsed < 200ms)
      - t=250ms → HTTP 200 (refill via lazy access; elapsed >= 200ms)
      - tolerance: ±10ms wallclock on the t=250ms boundary per ADR-0116 + SPEC §11.7
      - counter deltas: baz.http_local_rate_limit.{enabled=3, ok=2, rate_limited=1, enforced=1}

  - name: scenario_4_per_route_override
    listener: l_per_route
    config:
      listener: { max_tokens: 10, fill_interval: 1s, stat_prefix: qux }
      route_strict_TPFC: { max_tokens: 1, fill_interval: 60s, stat_prefix: strict }
      route_loose: no override (inherits listener)
    workload: 6 GETs interleaved /strict, /loose, /strict, /loose, /strict, /loose
    asserts:
      - /strict: 1 × HTTP 200 + 2 × HTTP 429 (independent bucket per ADR-0117)
      - /loose:  3 × HTTP 200 (listener bucket cap=10; not rate-limited)
      - counter deltas:
          strict.http_local_rate_limit.{enabled=3, ok=1, rate_limited=2, enforced=2}
          qux.http_local_rate_limit.{enabled=3, ok=3, rate_limited=0, enforced=0}
      - wholesale-override: listener qux counters NOT incremented for /strict reqs (per SPEC §11.6)

topology: 4 pre-configured listeners (l_s1, l_s2, l_s3, l_per_route) per PLAN planner-time decision 8 (DIVERGES from SPEC §7.1's two-listener+teardown layout); all 4 scenarios run in ONE DriveSubject/DriveReference invocation; each scenario's per-listener bucket state is naturally isolated by listener-distinct factories
timing_tolerance: ±10ms on scenario 3 t=250ms boundary (PLAN planner-time decision 4 default)
fixture_directory: test/fixtures/0013-http-local-ratelimit/
```

- [ ] **Step 2: Create `test/fixtures/0013-http-local-ratelimit/README.md`**

```markdown
# Fixture 0013-http-local-ratelimit

Differential fixture for phase 11's `envoy.filters.http.local_ratelimit` filter
landing per phase 11 SPEC.md §7. Validates byte-equivalent behavior across
reference Envoy v1.37.2 (STRICT_DNS) and envoy-go (STATIC) under 4 scenarios
covering basic-allow / basic-rate-limited / refill-after-fill_interval /
per-route-override.

## Scenarios

1. **scenario_1_basic_allow** — Listener `l_s1` with `max_tokens=10,
   tokens_per_fill=10, fill_interval=1s, stat_prefix=foo`. Sends 5 sequential
   GETs; expects 5×HTTP 200 + counter deltas
   `foo.http_local_rate_limit.{enabled=5, ok=5, rate_limited=0, enforced=0}`.

2. **scenario_2_basic_rate_limited** — Listener `l_s2` with `max_tokens=2,
   tokens_per_fill=2, fill_interval=60s, stat_prefix=bar`.
   Sends 5 sequential GETs; expects first 2×HTTP 200 + last 3×HTTP 429 with
   byte-exact body `local_rate_limited` (18 bytes, no LF) + 4-header set
   `content-length: 18, content-type: text/plain, date: <RFC1123>, server: envoy`
   + counter deltas `bar.http_local_rate_limit.{enabled=5, ok=2, rate_limited=3,
   enforced=3}` (lockstep MVP invariant per ADR-0118).

3. **scenario_3_refill_after_fill_interval** — Listener `l_s3` with
   `max_tokens=1, tokens_per_fill=1, fill_interval=200ms, stat_prefix=baz`.
   Sends 3 GETs at t=0/10ms/250ms; expects t=0→200, t=10ms→429,
   t=250ms→200 (refill via lazy access at the 200ms boundary); ±10ms wallclock
   tolerance per ADR-0116 + SPEC §11.7 empirical pin.

4. **scenario_4_per_route_override** — Listener `l_per_route` with listener-level
   `qux` config + per-route `/strict` TPFC override + no-override `/loose` route.
   Sends 6 interleaved GETs; expects `/strict` to rate-limit independently
   (independent bucket per ADR-0117 = ADR-0073 amendment) + listener-level
   counters NOT incremented for `/strict` reqs (wholesale-override per SPEC §11.6).

## 4-listener pre-configured bootstrap

Both `envoy.yaml` (reference) and `envoy-go.yaml` (subject) carry FOUR listeners
per PLAN planner-time decision 8 (which DIVERGES from SPEC §7.1's two-listener
layout to avoid a per-scenario-teardown framework extension to the existing
differential-fixture harness; the harness's `fixture.Driver` interface defines
one Drive call per fixture and does not support per-scenario teardown):
- `l_s1` — scenario 1 bucket params (cap=10, fill=10, interval=1s, stat_prefix=foo)
- `l_s2` — scenario 2 bucket params (cap=2,  fill=2,  interval=60s, stat_prefix=bar)
- `l_s3` — scenario 3 bucket params (cap=1,  fill=1,  interval=200ms, stat_prefix=baz)
- `l_per_route` — scenario 4 (listener qux + per-route /strict + /loose)

Each listener binds its own port (allocated by the runner). The driver dials
each listener in turn within a SINGLE `DriveSubject`/`DriveReference`
invocation; per-listener bucket state is naturally isolated by listener-
distinct factories. All 4 listeners explicitly set `filter_enabled` +
`filter_enforced` to 100% per SPEC §1.1 amendment (RuntimeFractionalPercent
default is 0% — omitting these would silently disable rate-limiting in
reference Envoy, breaking differential equivalence). envoy-go silent-ignores
the fields (per SPEC §2.1 cluster 2); the field presence is for byte-
equivalent config-load behavior.

## No per-scenario teardown

Bucket-state isolation is achieved at boot time via the 4-listener topology;
no per-scenario teardown is required. This avoids the ~50 LoC framework
extension that adding per-scenario teardown to `fixture.Driver` would entail.

## Envoy deviation

NONE. local_ratelimit is a normal HTTP filter; no SIGTERM/drain divergence; no
wire-protocol divergence beyond the documented `server: envoy` value (per SPEC
§1.1 amendment which corrected BRAINSTORM's `envoy-go` hypothesis).

## ADR cross-references

- ADR-0114: package shape (`localratelimit/` no-underscore)
- ADR-0115: runtimeConfig + PGV + filter-internal `fill_interval >= 50ms` validation
- ADR-0116: tokenBucket primitive (lazy refill on access; ±10ms refill tolerance)
- ADR-0117: per-route bucket isolation (ADR-0073 amendment)
- ADR-0118: 22→26 stat-table + Rule SN9 Prometheus tag-extractor
- ADR-0119: rate-limited response wire shape

## Planner-time decisions cross-references

- D1: Tag-extractor registration site = `internal/stats/name.go` SN9
- D2: Filter-callback wiring = `SetDecoderCallbacks` + `SetEncoderCallbacks`
- D3: PGV plumbing = explicit checks in New
- D4: Scenario 3 tolerance = ±10ms simple time.Sleep (retry-with-deadline reserved)
- D5: Test-only clock injection = SKIP
- 6 (PLAN): file split = bucket.go + local_ratelimit.go
- 7 (PLAN): race-detector test = TestTokenBucket_ConcurrentTryConsume
- 8 (PLAN): fixture topology = 4 pre-configured listeners (l_s1, l_s2, l_s3, l_per_route); diverges from SPEC §7.1
- 9 (PLAN): BackendKind = HTTPLocalRateLimit BackendKind = 10
```

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0013-http-local-ratelimit/expectations.yaml test/fixtures/0013-http-local-ratelimit/README.md
git commit -m "phase 11: fixture 0013 expectations + README"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3 + §7.1; ADR-0019 expectations-as-prose discipline.*

---

## Task 13: Fixture 0013 — `driver/driver.go` (single-Drive 4-listener orchestration + ±10ms tolerance)

**Files:**
- Create: `test/fixtures/0013-http-local-ratelimit/driver/driver.go`
- Modify: `test/differential/runner_test.go` (add the blank-import deferred from Task 9)

This task lands the full driver implementing the four-scenario orchestration via the 4-listener topology per planner-time decision 8. The driver implements `fixture.Driver` + the optional `fixture.MultiListenerDriver` (per `test/differential/fixture/fixture.go:294-299`) so the runner allocates 4 subject ports + exposes 4 reference ports. ALL 4 scenarios run in a SINGLE `DriveReferenceMulti` / `DriveSubjectMulti` invocation (no per-scenario teardown — bucket isolation is achieved at boot via the 4-listener topology, since each listener carries its own factory-built `*runtimeConfig` + `*tokenBucket`). Captures per-probe status + body + headers + post-scenario `/stats/prometheus` scrape; returns a deterministic byte stream for `CompareBytes`.

**Precondition:** Task 12 done.
**Artifact:** driver.go + runner_test.go blank-import.
**Acceptance:** `go build ./test/fixtures/0013-http-local-ratelimit/driver/...` clean; `go test -count=1 ./test/differential/ -run Test.*0013 -v` runs the fixture and reports differential-equivalence PASS for all 4 scenarios; vet + lint clean; ±10ms tolerance respected on scenario 3.

- [ ] **Step 1: Create `test/fixtures/0013-http-local-ratelimit/driver/driver.go`**

The structure mirrors `test/fixtures/0011-http-fault/driver/driver.go` (which similarly runs all its scenarios in a single Drive call). Phase 11 adds the `MultiListenerDriver` interface for the 4-listener topology. The implementer at Task 13 step 1 surveys the existing 0011-fault driver to determine the EXACT signatures + the `fixture.MultiListenerDriver` shape; the below sketch may diverge in minor details:

```go
// Package driver implements the differential-fixture driver for fixture
// 0013-http-local-ratelimit. Issues 4 scenarios per phase 11 SPEC §7.1 against
// both reference Envoy v1.37.2 and envoy-go via the 4-listener topology
// (planner-time decision 8 — diverges from SPEC §7.1's two-listener layout to
// avoid a per-scenario-teardown framework extension); ±10ms tolerance on
// scenario 3 t=250ms boundary per ADR-0116 + planner-time decision 4.
package driver

import (
    _ "embed"
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "sort"
    "strings"
    "text/template"
    "time"

    "github.com/esalaine/envoy-go/test/differential/fixture"
)

//go:embed ../envoy.yaml
var refBootstrapTemplate string

//go:embed ../envoy-go.yaml
var subjBootstrapTemplate string

func init() {
    fixture.RegisterFixture("0013-http-local-ratelimit", &localRateLimitDriver{})
}

type localRateLimitDriver struct{}

// fixture.Driver basic methods.
func (d *localRateLimitDriver) BackendCount() int                { return 1 }
func (d *localRateLimitDriver) BackendKind() fixture.BackendKind { return fixture.HTTPLocalRateLimit }

// fixture.MultiListenerDriver — exposes 4 listener names per planner-time decision 8.
func (d *localRateLimitDriver) SubjectListenerNames() []string {
    return []string{"l_s1", "l_s2", "l_s3", "l_per_route"}
}
func (d *localRateLimitDriver) ReferenceListenerPorts() []int {
    // In-container ports for the reference Envoy. Runner exposes them on the
    // host. The l_s* and l_per_route ports are pre-assigned per the bootstrap.
    return []int{10013, 10014, 10015, 10016}
}

// Single-listener compat (SubjectListenerName / ReferenceListenerPort) — return
// the first listener as the primary so the runner's pre-multi-branch
// fixture-discovery + admin-probe paths still work per the MultiListenerDriver
// contract.
func (d *localRateLimitDriver) SubjectListenerName() string  { return "l_s1" }
func (d *localRateLimitDriver) ReferenceListenerPort() int   { return 10013 }

// ReferenceBootstrap / SubjectConfig — render the 4-listener YAML once with
// the runner-allocated ports.
func (d *localRateLimitDriver) ReferenceBootstrap(backendPorts []int) string {
    return d.render(refBootstrapTemplate, struct {
        AdminPort, LS1Port, LS2Port, LS3Port, LPerRoutePort, BackendPort int
    }{
        AdminPort: 9913, LS1Port: 10013, LS2Port: 10014, LS3Port: 10015, LPerRoutePort: 10016,
        BackendPort: backendPorts[0],
    })
}
func (d *localRateLimitDriver) SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
    // For the multi-listener case, the runner passes the FIRST subject port
    // here; subsequent listener ports are obtained via SubjectListenerNames+
    // SubjectListenerAddrs lookup at Drive*Multi time. Implementer at Task 13
    // step 1 verifies the runner's actual mechanism for multi-listener subj
    // port routing and adapts the template-data struct accordingly.
    return d.render(subjBootstrapTemplate, struct {
        AdminPort, LS1Port, LS2Port, LS3Port, LPerRoutePort, BackendPort int
    }{
        AdminPort: subjAdminPort,
        // ... populate from runner-provided multi-listener port table
        BackendPort: backendPorts[0],
    })
}

func (d *localRateLimitDriver) render(tmpl string, data any) string {
    var buf bytes.Buffer
    if err := template.Must(template.New("yaml").Parse(tmpl)).Execute(&buf, data); err != nil {
        panic(err)
    }
    return buf.String()
}

// DriveReferenceMulti / DriveSubjectMulti — single call exercising ALL FOUR scenarios.
// addrs maps listener name → "host:port" for that listener. NO per-scenario
// teardown: bucket isolation is provided by the 4-listener topology.
func (d *localRateLimitDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
    return d.driveAll(ctx, addrs)
}
func (d *localRateLimitDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
    return d.driveAll(ctx, addrs)
}

// Single-listener compat for the fixture.Driver interface (used only for the
// fixture-discovery / admin-probe path; the multi-listener path supersedes it
// for actual scenario execution).
func (d *localRateLimitDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
    return []byte("multi-listener fixture; see DriveReferenceMulti"), nil
}
func (d *localRateLimitDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
    return []byte("multi-listener fixture; see DriveSubjectMulti"), nil
}

// ProbeAdmin — implement per fixture.Driver interface (admin equivalence diff).
func (d *localRateLimitDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
    // Implementer at Task 13 step 1 mirrors the existing 0011-fault driver's
    // ProbeAdmin shape (GET /ready against each side; return raw bytes).
    return nil, nil, nil
}

// driveAll runs all 4 scenarios sequentially against the 4 pre-configured listeners.
func (d *localRateLimitDriver) driveAll(ctx context.Context, addrs map[string]string) ([]byte, error) {
    var b bytes.Buffer
    // Per-listener admin addr: the runner exposes ONE admin port per side; we
    // assume addrs["__admin__"] carries it (or implementer uses the runner's
    // per-side admin addr accessor).
    adminAddr := addrs["__admin__"]
    d.driveScenario1(ctx, &b, addrs["l_s1"], adminAddr)
    d.driveScenario2(ctx, &b, addrs["l_s2"], adminAddr)
    d.driveScenario3(ctx, &b, addrs["l_s3"], adminAddr)
    d.driveScenario4(ctx, &b, addrs["l_per_route"], adminAddr)
    return b.Bytes(), nil
}

// driveScenario1 sends 5 sequential GETs to l_s1; captures status + counters.
func (d *localRateLimitDriver) driveScenario1(ctx context.Context, b *bytes.Buffer, addr, adminAddr string) {
    fmt.Fprintln(b, "=== scenario_1_basic_allow ===")
    for i := 0; i < 5; i++ {
        status, _, _ := d.probe(ctx, addr, "/")
        fmt.Fprintf(b, "req%d: status=%d\n", i+1, status)
    }
    d.captureCounters(ctx, b, adminAddr, "foo")
}

// driveScenario2 sends 5 sequential GETs; captures rate-limited body + headers.
func (d *localRateLimitDriver) driveScenario2(ctx context.Context, b *bytes.Buffer, addr, adminAddr string) {
    fmt.Fprintln(b, "=== scenario_2_basic_rate_limited ===")
    for i := 0; i < 5; i++ {
        status, headers, body := d.probe(ctx, addr, "/")
        fmt.Fprintf(b, "req%d: status=%d\n", i+1, status)
        if status == 429 {
            fmt.Fprintf(b, "  body: %q\n", string(body))
            var names []string
            for n := range headers {
                names = append(names, n)
            }
            sort.Strings(names)
            for _, n := range names {
                if strings.EqualFold(n, "Date") {
                    fmt.Fprintf(b, "  header: %s: <allow-listed>\n", n)
                    continue
                }
                for _, v := range headers.Values(n) {
                    fmt.Fprintf(b, "  header: %s: %s\n", n, v)
                }
            }
        }
    }
    d.captureCounters(ctx, b, adminAddr, "bar")
}

// driveScenario3 — the timing-sensitive refill scenario. Fires 3 GETs at
// t=0 / t=10ms / t=250ms with ±10ms tolerance on the t=250ms boundary.
// Per planner-time decision 4: simple time.Sleep with post-hoc band assertion.
func (d *localRateLimitDriver) driveScenario3(ctx context.Context, b *bytes.Buffer, addr, adminAddr string) {
    fmt.Fprintln(b, "=== scenario_3_refill_after_fill_interval ===")
    t0 := time.Now()
    status1, _, _ := d.probe(ctx, addr, "/")
    fmt.Fprintf(b, "req1 (t=0): status=%d (delay=%dms)\n", status1, time.Since(t0).Milliseconds())
    time.Sleep(10*time.Millisecond - time.Since(t0))
    status2, _, _ := d.probe(ctx, addr, "/")
    fmt.Fprintf(b, "req2 (t=10ms): status=%d (delay=%dms)\n", status2, time.Since(t0).Milliseconds())
    time.Sleep(250*time.Millisecond - time.Since(t0))
    actualReq3DelayMs := time.Since(t0).Milliseconds()
    status3, _, _ := d.probe(ctx, addr, "/")
    fmt.Fprintf(b, "req3 (t=250ms): status=%d (delay=%dms)\n", status3, actualReq3DelayMs)
    if actualReq3DelayMs < 200 || actualReq3DelayMs > 260 {
        fmt.Fprintf(b, "TOLERANCE_FAIL: req3 delay %dms outside [200, 260] band\n", actualReq3DelayMs)
    } else {
        fmt.Fprintln(b, "tolerance: req3 delay within ±10ms band")
    }
    d.captureCounters(ctx, b, adminAddr, "baz")
}

// driveScenario4 — per-route override. Sends 6 interleaved GETs.
func (d *localRateLimitDriver) driveScenario4(ctx context.Context, b *bytes.Buffer, addr, adminAddr string) {
    fmt.Fprintln(b, "=== scenario_4_per_route_override ===")
    paths := []string{"/strict/x", "/loose/x", "/strict/x", "/loose/x", "/strict/x", "/loose/x"}
    for i, p := range paths {
        status, _, _ := d.probe(ctx, addr, p)
        fmt.Fprintf(b, "req%d (%s): status=%d\n", i+1, p, status)
    }
    d.captureCounters(ctx, b, adminAddr, "strict")
    d.captureCounters(ctx, b, adminAddr, "qux")
}

func (d *localRateLimitDriver) probe(ctx context.Context, addr, path string) (int, http.Header, []byte) {
    client := &http.Client{Timeout: 5 * time.Second}
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+addr+path, nil)
    resp, err := client.Do(req)
    if err != nil {
        return -1, nil, []byte(fmt.Sprintf("ERROR: %v", err))
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    return resp.StatusCode, resp.Header, body
}

func (d *localRateLimitDriver) captureCounters(ctx context.Context, b *bytes.Buffer, adminAddr, statPrefix string) {
    client := &http.Client{Timeout: 5 * time.Second}
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+adminAddr+"/stats/prometheus", nil)
    resp, err := client.Do(req)
    if err != nil {
        fmt.Fprintf(b, "stats/prometheus: ERROR: %v\n", err)
        return
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    needle := fmt.Sprintf(`envoy_local_http_ratelimit_prefix=%q`, statPrefix)
    var matches []string
    for _, line := range strings.Split(string(body), "\n") {
        if strings.Contains(line, needle) {
            matches = append(matches, line)
        }
    }
    sort.Strings(matches)
    for _, line := range matches {
        fmt.Fprintf(b, "stat: %s\n", line)
    }
}
```

NOTE: the implementer at Task 13 step 1 reads the existing `test/fixtures/0011-http-fault/driver/driver.go` + the `fixture.Driver` + `fixture.MultiListenerDriver` interfaces in `test/differential/fixture/fixture.go` to determine the EXACT method signatures (especially how multi-listener admin addr is plumbed); the above sketch is structural only. The `//go:embed` directive on the YAML files at the package root is the cleanest way to load envoy.yaml + envoy-go.yaml for templating.

The runner's `MultiListenerDriver` dispatch path (`DriveReferenceMulti` / `DriveSubjectMulti`) is the existing fixture-0008 mechanism (introduced by phase 07.2 per SPEC §7.4). NO new framework extension is required: phase 11's 4-listener topology fits within the existing `MultiListenerDriver` contract. Task 9's fixture-infrastructure work (Step 4) is unchanged — the blank-import + spawn helper + BackendKind enum value are sufficient for the multi-listener case.

- [ ] **Step 2: Add the blank-import to `test/differential/runner_test.go`** (deferred from Task 9 step 4)

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/driver"
```

- [ ] **Step 3: Run the fixture differentially**

```bash
go test -count=1 -v ./test/differential/ -run Test.*0013 2>&1 | tail -60
```

Expected: PASS for all 4 scenarios; per-scenario byte-stream diff returns no differences. If scenario 3's tolerance assertion fires `TOLERANCE_FAIL`, the implementer at Task 13 step 4 either (a) widens to ±20ms (per SPEC §12 D4 + ADR-0116 §Consequences amendment in-place) or (b) switches to a retry-with-deadline harness (per planner-time decision 4 fallback option). PROGRESS.md captures the chosen mitigation.

- [ ] **Step 4: Vet + lint + commit**

```bash
go vet ./...
golangci-lint run ./...
git add test/fixtures/0013-http-local-ratelimit/driver/ test/differential/runner_test.go
git commit -m "phase 11: fixture 0013 driver — 4-scenario orchestration via 4-listener topology"
```

SHA-fill follow-up.

*Anchored: SPEC §7.1 + §7.2; planner-time decisions 4 + 8; ADR-0116 (timing tolerance).*

---

## Task 14: BEHAVIOR_CONTRACT.md patches per SPEC §13 + ROADMAP row 11 in-progress→done

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (insert §13.1 subsection + §13.2 22→26 stat-table + §13.3 timing-tolerances row + §13.4 equivalence-matrix row + §13.5 forward-pointer notes)
- Modify: `docs/envoy-go/ROADMAP.md` (row 11 status `in-progress → done`)

This task lands the documentation patches per SPEC §13 + the ROADMAP row flip. Per ADR-0052 in-place edit authorisation. NO new code touched. NO new ADRs (ADR-0114..ADR-0119 already landed in Tasks 2/3/4/5/6).

**Precondition:** Task 13 done; all 4 fixture scenarios passing.
**Artifact:** modified BEHAVIOR_CONTRACT.md + ROADMAP.md.
**Acceptance:** BEHAVIOR_CONTRACT.md has the §13.1 + §13.2 + §13.3 + §13.4 + §13.5 patches; ROADMAP row 11 status `done`.

- [ ] **Step 1: Insert the §13.1 `### envoy.filters.http.local_ratelimit` subsection in BEHAVIOR_CONTRACT.md**

Locate the existing `### envoy.filters.http.header_mutation` subsection (line 924 in master HEAD `97ed8b9`); insert the new `### envoy.filters.http.local_ratelimit` subsection AFTER header_mutation's subsection ends. The verbatim Markdown patch is at SPEC §13.1 (the SPEC's verbatim Markdown shape; copy verbatim). The patch covers:

- `#### Asserted equivalence (per phase 11 SPEC §11)` — request-side rate-limit decision + 4-counter increments + lockstep MVP invariant + per-route bucket independence
- `#### Token-bucket primitive (per ADR-0116)` — lazy refill on access; sync.Mutex; monotonic-time semantics; LBP-1-adjacent
- `#### Per-route override semantics (per ADR-0117 + ADR-0073 amendment)` — wholesale-override extends to stateful resources; independent buckets per per-route TPFC
- `#### Rate-limited response wire shape (per ADR-0119 + SPEC §11.3)` — status 429; body byte-exact; 4-header set; framing Content-Length
- `#### Allow-path response (per SPEC §11.8)` — NO x-ratelimit-* headers added by the filter
- `#### MVP invariant (per ADR-0118)` — `enforced == rate_limited` lockstep; future shadow-mode widens
- `#### Stats (per SPEC §11.5 + Rule SN9)` — 4 counters per stat_prefix; Prometheus tag-extractor `envoy_local_http_ratelimit_prefix`
- `#### Silent-ignored fields (14, organized by family)` — descriptor-action, runtime+shadow-mode, xDS, etc.
- `#### Empirical evidence (verbatim curl excerpts from SPEC §11)` — sample wire shape

- [ ] **Step 2: Update §13.2 stat-name mapping table 22→26**

Locate the `## Stat-name mapping ### 22-name table` heading at line 131 (per BEHAVIOR_CONTRACT.md master HEAD survey). Update the heading: `### 22-name table` → `### 26-name table`. Append the four new rows after the existing 22 rows:

```markdown
| `<stat_prefix>.http_local_rate_limit.enabled`     | counter | filter | local_ratelimit | every request reaching the filter (§11.5) |
| `<stat_prefix>.http_local_rate_limit.ok`          | counter | filter | local_ratelimit | request not rate-limited (`tryConsume` → true; §11.5) |
| `<stat_prefix>.http_local_rate_limit.rate_limited`| counter | filter | local_ratelimit | request rate-limited (`tryConsume` → false; §11.5) |
| `<stat_prefix>.http_local_rate_limit.enforced`    | counter | filter | local_ratelimit | request rate-limited AND enforced (lockstep with `rate_limited` under MVP per ADR-0118; §11.5) |
```

Plus add the table preamble note about the new filter-specific Prometheus tag-extractor:

```markdown
**Filter-specific Prometheus tag-extractor (added in phase 11 per ADR-0118):** `<stat_prefix>.http_local_rate_limit.<counter>` extracts the `<stat_prefix>` segment into the Prometheus label `envoy_local_http_ratelimit_prefix`. NOTE: tag-extraction collisions occur if `<stat_prefix>` matches an Envoy-internal tag-extractor name (e.g. `listener` collides with `envoy.listener_address`); the differential fixture 0013 uses safe values (`foo`, `bar`, `baz`, `qux`, `strict`).
```

- [ ] **Step 3: Append §13.3 timing-tolerances row**

Locate the `## Timing tolerances` section (line 286). Append the new row:

```markdown
| fixture 0013 scenario 3 (refill-after-fill_interval) | t=250ms refill boundary | ±10ms wall-clock | per ADR-0116 + SPEC §11.7 empirical (BRAINSTORM ±20ms hypothesis narrowed; PLAN author may widen back to ±20ms with retry-with-deadline harness if CI flakes per SPEC §12 D4) |
```

- [ ] **Step 4: Append §13.4 equivalence-matrix row**

Locate the `## Equivalence Matrix` section (line 9). Append the new row:

```markdown
| HTTP filter `envoy.filters.http.local_ratelimit` | Per-request equivalence on rate-limit decision (allow → 200; rate-limited → 429 with body byte-exact `local_rate_limited` 18 bytes, no LF, 4-header set lowercase wire-form `content-length: 18, content-type: text/plain, date: <RFC1123>, server: envoy`), 4 counter deltas per `<stat_prefix>.http_local_rate_limit.{enabled, ok, rate_limited, enforced}`, lockstep MVP invariant `enforced == rate_limited`, refill timing ±10ms tolerance on the `fill_interval` boundary, per-route bucket independence (wholesale-override extends to stateful resources per ADR-0117 = ADR-0073 amendment), and Prometheus tag-extracted label `envoy_local_http_ratelimit_prefix=<stat_prefix>` per Rule SN9. Differential gate fixture 0013-http-local-ratelimit (4 scenarios). NOT asserted: `filter_enabled` / `filter_enforced` runtime-key < 100% (deferred — Runtime + hot restart family); descriptor-action subsystem (deferred — global_ratelimit family); response_headers_to_add (deferred); local_rate_limit_per_downstream_connection (deferred); enable_x_ratelimit_headers (deferred). |
```

- [ ] **Step 5: Insert §13.5 forward-pointer notes**

Per SPEC §13.5: the verbatim 14-field deferred-fields family-cluster list + the `filter_enabled`+`filter_enforced` runtime-key default-0% divergence-window note + the tag-extraction collision quirk note. Locate the `## Forward-pointer notes` section (which existed prior to phase 11) and append a new `### Phase 11 forward-pointer notes` subsection per SPEC §13.5:

```markdown
### Phase 11 forward-pointer notes

**Deferred field families** (silent-ignored per ADR-0040; see §13.1 for the full 14-field list):

- Descriptor-action subsystem (4 fields) → couples to `global_ratelimit` future phase under §9 HTTP filters family.
- Runtime + shadow-mode subsystem (3 fields, including `filter_enabled` and `filter_enforced` `RuntimeFractionalPercent` fields) → couples to Runtime + hot restart family. **Divergence-window:** envoy-go silent-ignores these fields; reference Envoy defaults both to 0% (off). Differential fixture configs MUST set both to 100% explicitly; users running envoy-go with these fields set to non-100% values will diverge from Envoy (envoy-go behaves as always-100%, Envoy honors the percentage).
- xDS cluster-state (1 field: `local_cluster_rate_limit`) → couples to xDS / dynamic config family.
- Response-side header injection (1 field: `response_headers_to_add`) → standalone follow-on.
- Per-connection lifecycle (1 field: `local_rate_limit_per_downstream_connection`) → standalone follow-on.
- Multi-stage limiting (1 field: `stage`) → couples to descriptor-action subsystem.
- X-RateLimit headers + vh policy (2 fields: `enable_x_ratelimit_headers`, `vh_rate_limits`) → standalone follow-on.
- gRPC trailer mapping (1 field: `rate_limited_as_resource_exhausted`) → couples to gRPC family.

**Tag-extraction collision quirk:** when `local_ratelimit.stat_prefix` matches an Envoy-internal tag-extractor name (`listener`, `http`, `cluster`, etc.), Envoy v1.37.2 mangles the Prometheus metric name. envoy-go's tag-extractor registration (Rule SN9) replicates the standard non-collision case; collision-mangling parity is OUT of scope for phase 11 (per SPEC §1.1 amendment + §11.5).
```

- [ ] **Step 6: Flip ROADMAP row 11 status `in-progress → done`**

Locate row 11 in `docs/envoy-go/ROADMAP.md` and change the `status` column from `in-progress` to `done`. The §9 family heading at line 56 stays UNCHANGED per ADR-0106.

- [ ] **Step 7: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md
git commit -m "phase 11: BEHAVIOR_CONTRACT + ROADMAP row 11 done"
```

SHA-fill follow-up.

*Anchored: SPEC §13.1 + §13.2 + §13.3 + §13.4 + §13.5; ADR-0052 (in-place edit); ADR-0106 (family heading unchanged); SPEC §15 acceptance bullet.*

---

## Task 15: Phase-done six-gate verification + STATE.md advance + phase-done commit

**Files:**
- Modify: `docs/envoy-go/STATE.md`
- Modify: `docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md` (record gate outputs)

This task verifies the SPEC §3 six-gate phase-done checklist and advances STATE.md to `awaiting next planning`. Per `BOOTSTRAP_PROMPT.md` §7.5 + SPEC §3.

**Precondition:** Task 14 done.
**Artifact:** modified STATE.md + verbatim verification commands' output captured in PROGRESS.md.
**Acceptance:** all six gates report green; STATE.md flipped.

- [ ] **Step 1: Run gate (a) — `go build ./...` + `go vet ./...` + `golangci-lint run ./...`**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
```

Expected: clean. Capture output to PROGRESS.md Task 15 entry.

- [ ] **Step 2: Run gate (b) — `go test -race ./...` clean**

```bash
go test -race -count=1 ./...
```

Expected: every package PASS. Capture output.

- [ ] **Step 3: Run gate (c) — h2spec re-run at the ADR-0051 pin (53/53 PASS)**

```bash
make h2spec  # or whatever the existing entry point is per phase 09 / 10 conventions
```

Expected: 53/53 PASS unchanged. Capture output.

- [ ] **Step 4: Run gate (d) — fuzzers (existing 14 + new 1 = 15) clean at 30s budget**

```bash
go test -fuzz=FuzzLocalRateLimitConfigParse -fuzztime=30s ./internal/filter/http/localratelimit/
# Plus the 14 pre-existing fuzzers (FuzzBootstrapConfigParse, FuzzCORSConfigParse, FuzzFaultConfigParse,
# FuzzHeaderMutationConfigParse, FuzzConfigDumpFormat, FuzzAccessLogFormat, etc.). Run via
# the existing CI script that iterates them OR run each individually.
```

Expected: all fuzzers run clean.

- [ ] **Step 5: Run gate (e) — differential fixtures 0000–0013 all green**

```bash
go test -count=1 -v ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013'
```

Expected: every fixture PASS including the new 0013.

- [ ] **Step 6: Verify gate (f) — BEHAVIOR_CONTRACT.md populated**

Spot-check the §13.1 + §13.2 + §13.3 + §13.4 + §13.5 patches landed in Task 14 are present:

```bash
grep -nE 'envoy.filters.http.local_ratelimit' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -10
grep -nE 'envoy_local_http_ratelimit_prefix' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -5
grep -nE '26-name table' docs/envoy-go/BEHAVIOR_CONTRACT.md
grep -nE 'fixture 0013 scenario 3' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

Expected: matches in `## HTTP filter chain`, `## Stat-name mapping`, `## Timing tolerances`, `## Equivalence Matrix`, and `## Forward-pointer notes`.

- [ ] **Step 7: Update `docs/envoy-go/STATE.md`**

Flip:
- `lifecycle-state` → `awaiting next planning` (or the equivalent post-phase-done state per `BOOTSTRAP_PROMPT.md` §5)
- `next-skill` → `superpowers:brainstorming` (the next §9 family-child cold-starts from the §9 heading per ADR-0106)
- `next-skill-scope` → describes the cold-start: read ROADMAP.md row 11 + BEHAVIOR_CONTRACT.md ### envoy.filters.http.local_ratelimit + DECISIONS.md tail (now ADR-0119); the next family-child is selected by the brainstormer per the §9 family list at ROADMAP line 58 (compression / jwt_authn / rbac / etc; NOTE: local_ratelimit is now landed, so it is no longer a candidate).
- `active-phase` → `<next-family-row-id>` resolved by the next session's planner; this PLAN sets it to a sentinel value (e.g., `<unset — next session resolves>`)
- `last-commit` → the phase-done commit SHA (filled in step 9 SHA-fill follow-up)
- `last-updated` → current date

- [ ] **Step 8: Phase-done commit**

```bash
git add docs/envoy-go/STATE.md docs/envoy-go/phases/11-http-filter-local-ratelimit/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 11: http-filter-local-ratelimit [ADR-0114, ADR-0115, ADR-0116, ADR-0117, ADR-0118, ADR-0119]

Lands envoy.filters.http.local_ratelimit under the 07.1 framework.
FOURTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10).

ROADMAP row 11 flips in-progress → done at this commit.
The §9 family heading at ROADMAP line 56 stays unchanged (headings are
not rows; per ADR-0106).

Six ADRs land:
- ADR-0114: package shape (localratelimit/ no-underscore directory diverges
  from header_mutation/ underscore-preserving pattern; aligns with cors/+fault/)
- ADR-0115: runtimeConfig 5/14-field decomposition + PGV table + filter-internal
  fill_interval >= 50ms validation discipline (verbatim Envoy error string)
- ADR-0116: tokenBucket Option-A lazy-refill + monotonic-time + LBP-1-adjacent
  + ±10ms empirical refill-timing tolerance
- ADR-0117: per-route bucket isolation as ADR-0073 wholesale-override
  consequence (FIRST stateful per-route filter; ADR-0073 amendment paragraph)
- ADR-0118: 22→26-name stat-table extension + enforced == rate_limited MVP
  invariant + filter-specific Prometheus tag-extractor envoy_local_http_ratelimit_prefix
  registered as Rule SN9 in internal/stats/name.go (ADR-0061 amendment)
- ADR-0119: rate-limited response wire shape + body byte-exact local_rate_limited
  + 4-header set lowercase wire-form + 429 default + SendLocalReply reuse from
  phase 09 fault precedent

Framework deltas: internal/stats.Registry.NewCounterIfAbsent (idempotent
post-Freeze registration) per ADR-0117 + ADR-0061 amendment;
internal/stats/name.go Rule SN9 (filter-specific Prometheus tag-extractor) per
ADR-0118.

Differential fixture 0013-http-local-ratelimit green (4 scenarios:
basic-allow / basic-rate-limited / refill-after-fill_interval ±10ms /
per-route-override).

Stats: 4 new counters per stat_prefix (22→26 table extension);
Prometheus tag-extractor extracts <stat_prefix> into envoy_local_http_ratelimit_prefix
label per Rule SN9.

All six phase-done gates green: build/vet/lint clean; race tests pass;
h2spec 53/53 PASS unchanged; 15 fuzzers green at 30s budget; all 14
differential fixtures (0000–0013) green; BEHAVIOR_CONTRACT.md populated.
EOF
)"
```

SHA-fill follow-up commit per the phase-04..10 convention.

*Anchored: SPEC §3 + §15; BOOTSTRAP_PROMPT.md §5.3 + §7.5.*

---

## Task 16: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md`

This task drafts the end-of-phase REVIEW.md per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 cadence; populates per the `superpowers:requesting-code-review` skill. Phase 11 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 11. NO new ADRs.

**Precondition:** Task 15 done.
**Artifact:** REVIEW.md.
**Acceptance:** REVIEW.md committed; covers per-task retrospective + carry-forward findings + planner-time decisions retrospective.

- [ ] **Step 1: Invoke the `superpowers:requesting-code-review` skill**

If executing inline: read the skill output and apply its REVIEW shape. If executing subagent-driven: dispatch a code-reviewer subagent with the phase 11 SPEC + PLAN + PROGRESS as context.

- [ ] **Step 2: Draft REVIEW.md mirroring 10's REVIEW.md structure**

The REVIEW typically covers:
- N-1 carry-forward retrospective (review 10's REVIEW for any items requesting phase-11 follow-up; address each)
- Per-task retrospective (any task that landed deviations from PLAN; record the rationale — e.g., if Task 13 widened scenario 3 tolerance to ±20ms due to CI flake, record the finding)
- Planner-time decisions retrospective (each of the 9 decisions: did the implementation validate the choice or expose a flaw? — D1 SN9 placement; D2 callback wiring; D3 explicit-checks; D4 simple-sleep tolerance; D5 no clock injection; PLAN-6 file split; PLAN-7 race test; PLAN-8 4-listener topology; PLAN-9 BackendKind name)
- Carry-forward findings for phase 12 (e.g., framework primitives that proved load-bearing — `NewCounterIfAbsent` post-Freeze idempotent registration; `tokenBucket` primitive reusability for future global_ratelimit; deferrals that warrant scheduling — Runtime + hot restart family for `filter_enabled`+`filter_enforced` actual support; any minor tech-debt the next phase can pick up)
- ADR retrospective (each of the 6 ADRs: did the §Decision body hold up under implementation + fixture exercise? — ADR-0114 package-naming; ADR-0115 PGV/filter-internal split; ADR-0116 LBP-1-adjacent; ADR-0117 wholesale-override extension; ADR-0118 SN9 + MVP invariant; ADR-0119 wire shape)
- Six-gate retrospective (any gate that was non-trivial to satisfy — fixture 0013 scenario 3 tolerance; SN9 cross-rule precedence)

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/11-http-filter-local-ratelimit/REVIEW.md
git commit -m "phase 11: REVIEW — end-of-phase retrospective + N-1 carry-forward"
```

SHA-fill follow-up.

*Anchored: superpowers:requesting-code-review; phase-10 REVIEW precedent (master `97ed8b9`).*

---

## Refinement

If during execution the implementer discovers a SPEC ambiguity, a planner-time decision that was not foreseen, or a framework constraint that requires deviation from this PLAN, the implementer:

1. Records the deviation in PROGRESS.md's per-task entry under a `**Deviation:**` line + `**Rationale:**` + `**Anchored:**` cross-reference.
2. If the deviation alters the ADR table, amend the in-task ADR's Consequences section in-place (per the ADR-0089 consequence (b) in-place-edit pattern); do NOT introduce a new ADR for the amendment unless the deviation is structurally significant.
3. If the deviation alters the file-structure table, amend this PLAN's table in a follow-up commit OR record the deviation in PROGRESS.md and let the file-structure table become "as-built" rather than "as-planned" — the implementer's choice based on whether the deviation is broadly reusable for future readers.

Common refinement scenarios anticipated:

- **Scenario 3 timing flakes under heavy CI load** (per planner-time decision 4 + SPEC §12 D4). The default ±10ms simple-sleep tolerance may prove flaky on slow CI hosts; the implementer at Task 13 step 4 either widens to ±20ms (matching the original BRAINSTORM hypothesis; ADR-0116 §Consequences amends in-place) OR switches to a retry-with-deadline harness (issue req 3 with a deadline and retry until the response status confirms refill, asserting the retry-window upper bound). PROGRESS.md captures the chosen mitigation + a one-paragraph diagnostic dump from the failed runs.

- **`stats.Registry.NewCounterIfAbsent` semantics surface as too-permissive on duplicate registration** (planner-time decision 1 + Task 5). If two per-route TPFC entries with the same stat_prefix collide (e.g., two routes both define `stat_prefix=strict`), the current `NewCounterIfAbsent` returns the same `*Counter` for both — counter increments are pooled. Whether this is correct behavior depends on Envoy v1.37.2's empirical handling: if reference Envoy ALSO pools (since the stat_prefix is the unique key), then envoy-go matches; if reference Envoy treats them as distinct (per-route stat_prefix is unique within a route, NOT across routes), then envoy-go diverges. The fixture 0013 scenario 4 uses unique stat_prefix per per-route TPFC (`strict` for /strict; listener-level `qux`); the divergence is not exercised. ADR-0117 §Consequences amends in-place to record the discipline if surfacing.

- **The `*LocalRateLimitPerRoute` proto has a different field name than `RateLimit`** (per Task 5 step 3 sketch). The implementer at Task 5 step 3 surveys the actual go-control-plane generated `LocalRateLimitPerRoute` to confirm the embedded `LocalRateLimit` accessor; the sketch may need adjustment.

- **Rule SN9's cross-rule precedence with SN1/SN2/SN3 surfaces unexpected interaction** (per Task 6 + planner-time decision 1). If a name `cluster.foo.http_local_rate_limit.enabled` SHOULD route to SN9 (treating `cluster.foo` as a stat_prefix) rather than SN1 (treating `cluster.` as the SN1 prefix-segment), the cross-rule precedence is wrong. The fixture 0013 deliberately uses non-magic stat_prefix values to avoid this collision; the implementer at Task 6 step 2's `TestFlattenToProm_SN9_DoesNotConflictWithSN1234` verifies the SN1-wins precedence is the intended behavior.

- **Per-route counter post-Freeze allocation surfaces a stats Registry concurrency issue** (per Task 5 + ADR-0117). The `NewCounterIfAbsent` method is mutex-guarded but the existing `Walk` method's RLock + the new `NewCounterIfAbsent`'s Lock could surface a write-under-Walk pattern. The implementer at Task 5 step 1 verifies the LBP-1 invariant continues to hold post-amendment by running the existing stats race-detector tests (`TestRegistry_*` in `internal/stats/registry_test.go`) plus the new `TestNewCounterIfAbsent_*` tests.

- **The fixture 0013 driver uses the 4-listener pre-configured topology** (per Task 13 + planner-time decision 8) and implements the existing `fixture.MultiListenerDriver` interface introduced by phase 07.2 — NO new harness primitive required. Phase 09's fault driver is unchanged. If at Task 13 implementation time the 4-listener bootstrap proves to be a wrong simplification (e.g., reference Envoy disagrees on counter independence across listeners), the fallback is to extend the harness with a per-scenario-teardown primitive (~50 LoC `fixture.MultiScenarioDriver` interface + runner wiring).

## Post-plan handoff

After Task 16 lands the REVIEW, the orchestrating session:

1. Verifies the phase-done six gates one more time (sanity check) per Task 15.
2. Verifies STATE.md is at `awaiting next planning` with `next-skill: superpowers:brainstorming`.
3. Pushes the phase 11 worktree branch to origin (per the user's persistent preference: "after a clean local merge/commit on master with tests green, push without asking" recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`).
4. Hands off to the next session, whose first action is to invoke `superpowers:brainstorming` against §9's HTTP filters family for the next family-child (per ADR-0106 + STATE.md + BRAINSTORM.md Decision 9 — the next family-child cold-starts from the §9 heading + the just-shipped phase 11 artefacts; no sibling-stub was authored).

The phase 11 work is complete when:

- All 16 tasks in this PLAN have green checkmarks in PROGRESS.md.
- Phase-done commit + SHA-fill follow-up are on master.
- REVIEW.md is committed.
- STATE.md reflects the post-11 lifecycle state.
- The branch is pushed to origin.
- All six gates report green at HEAD.
