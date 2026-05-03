# Phase 09 — HTTP filter `envoy.filters.http.fault` (`internal/filter/http/fault/`, differential fixture `0011-http-fault`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` extension) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per ADR-0005 §4 and per the user's persistent preference for subagent-driven execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Project context (must read before executing):** `BOOTSTRAP_PROMPT.md` §3 (doctrine), §4 (invariants — particularly §4.1's ROADMAP-row-flips-at-SPEC-commit + at-phase-done discipline), §5 (state machine), §5.3 (commit-message-completeness — every ADR introduced or referenced is named in the phase-done commit message), §6 (split gates), §7 (differential contract), §7.5 (phase-done six-gate checklist that SPEC §3 specialises for 09), §9 (HTTP filters family — phase 09 is the FIRST top-level row to land under the §9 family heading per ADR-0106 settled by this phase); `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (the authoritative source — every PLAN task traces to one or more SPEC sections; 1305 lines, 16 sections, **read in full**); `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (the autonomous-brainstorm artefact at master `4f44a03` that the SPEC distils §§1–12 from — 13 Decisions + §11 empirical-pin obligations all executed at SPEC time; consult when the SPEC's "what" needs the BRAINSTORM's "why"); `docs/envoy-go/phases/08.2-graceful-drain/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (closed read-only history; 08.2's PLAN at master `ca9fc35` is the structural precedent — task-numbering, TDD-step layout, embedded-test-source convention, ADR-with-first-use-commit footer, "Anchored:" footer per task, "ADRs introduced by this plan" section, "Refinement" + "Post-plan handoff" closing sections); `docs/envoy-go/phases/07.1-http-filter-framework/PLAN.md` (the cors precedent's PLAN — Task 18 cors lands `internal/filter/http/cors/cors.go` per ADR-0072/ADR-0074; the per-filter package shape phase 09 inherits); `docs/envoy-go/DECISIONS.md` (ADR-0001…ADR-0099 — especially **ADR-0001** template, **ADR-0003** branch convention, **ADR-0004** autonomous-brainstorm hard-gate, **ADR-0005** subagent-driven preference, **ADR-0008** Envoy v1.37.2 pin, **ADR-0017** small-mechanical-fixes do not require ADRs, **ADR-0018** fuzz CI 30s short-budget policy, **ADR-0040** out-of-scope deferrals format — ADR-0104 in this phase follows the deferral-ADR format, **ADR-0044** ADR-on-impl convention, **ADR-0045** planner-time-split discipline (~25 tasks / ~1500 LoC thresholds — both well under for this phase per `## Scope check` below), **ADR-0051** h2spec pin SHA, **ADR-0052** BEHAVIOR_CONTRACT in-place edit authorisation, **ADR-0061** stats Registry / SN1–SN8 flattening rules — phase 09 extends the 17-name table to 22 names without revisiting the rules, **ADR-0071** HTTP-filter framework chain-shape + factory pattern + iteration-protocol surface — phase 09's fault filter is the first production exerciser of the async-resume primitive on the request side, **ADR-0072** HTTPRegistry threaded constructor map + factory typed_config validation contract — phase 09's `New` factory mirrors PGV `[200, 600)` validation per §11.1, **ADR-0073** typed_per_filter_config 3-tier merge (wholesale-override per §11.7 confirmation), **ADR-0074** filter set: cors + envoy_go_test — phase 09 adds fault as the third real production filter under the same package-shape discipline, **ADR-0075** sendLocalReply enters encode chain at filter[len-1] — phase 09's abort path uses this primitive verbatim; ADR-0099 is the verified DECISIONS.md tail at master `b33e04f` (08.2 phase-done close); phase 09's eight anticipated ADRs land at ADR-0100..ADR-0107 per SPEC §8); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the in-place-edit target — `## HTTP filter chain` umbrella at line 695 hosts the new `### envoy.filters.http.fault` subsection per SPEC §13.1; `## Stat-name mapping ### 17-name table` at line 130 extends to 22 names per SPEC §13.2; `## Timing tolerances` at line 266 gains a new fault-delay-accuracy bullet per SPEC §13.3; `## Equivalence Matrix` at line 9 gains one new row per SPEC §13.4; lands at the phase-done commit per ADR-0052); `docs/envoy-go/ENVOY_TARGET.md` (the v1.37.2 image pin SPEC §11 empirical pins cite); `docs/envoy-go/CONFORMANCE_PINS.md` (UNCHANGED in 09 — phase 09 is a pure HTTP-layer filter addition; touches no codec/framer/HPACK paths; the h2spec gate at 53/53 PASS is mechanical re-run); `docs/envoy-go/ROADMAP.md` (row `09` per the SPEC commit's row-flip; row `09` flips `in-progress → done` at this phase's phase-done; the §9 HTTP filters family heading at row 56 stays unchanged across all §9-family-row landings per ADR-0106 settled here); `internal/filter/http/cors/cors.go` (the package-shape precedent fault inherits — TypeURL constant + New factory + filter struct implementing both StreamDecoderFilter + StreamEncoderFilter + per-route 3-tier merge via `cb.RequestRouteConfig` + OrderedHeaders carrier for SendLocalReply per ADR-0075); `internal/filter/http/types.go` (FilterHeadersStatus + StreamDecoderFilter + StreamEncoderFilter + HTTPFilter + HTTPFilterFactory + FilterInstanceFactory + FactoryCtx — phase 09's Task 2 EXTENDS FactoryCtx with `Stats *stats.Registry` and `StatPrefix string` so the fault filter's `New` can register the 5 fault.* stats per HCM at filter-build time per ADR-0061's pre-Freeze discipline); `internal/filter/http/callbacks.go` (DecoderFilterCallbacks + SendLocalReply + ContinueDecoding + RequestRouteConfig — the framework primitives fault consumes); `internal/filter/http/registry.go` (HTTPRegistry — boot-time-populated, freeze-after-boot per ADR-0072); `internal/filter/http/perroute.go` (3-tier merge per ADR-0073).

**Goal:** Land envoy-go's `envoy.filters.http.fault` HTTP filter — the SECOND production HTTP filter after cors (07.1) and the FIRST top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Concretely (per SPEC §1 + §4): a new `internal/filter/http/fault/` package owning the filter implementation under the cors precedent's package shape (`fault.go` + `fault_test.go` + `doc.go` + `fuzz_test.go`; ~400 + ~280 + ~40 + ~50 = ~770 LoC; ADRs 0100, 0101, 0102, 0103, 0105, 0107); a small framework extension to `internal/filter/http/types.go::FactoryCtx` adding `Stats *stats.Registry` + `StatPrefix string` (~5 LoC delta) and the matching threading in `internal/filter/hcm/config.go::parseHTTPFiltersChain` (~5 LoC delta) so fault's `New` factory can register its 5 stats at HCM-build time per ADR-0061's pre-Freeze discipline (consequence of ADR-0100 — first-use anchored per ADR-0044); a `cmd/envoy-go/main.go` one-line registration delta (`httpReg.Register(fault.TypeURL, fault.New)` inserted alphabetically after the existing router/cors/envoygotest block, plus the matching package import; ~3 LoC delta); a NEW differential fixture `0011-http-fault` (`test/fixtures/0011-http-fault/`) with `envoy.yaml` + `envoy-go.yaml` (per §7.4 verbatim) + `expectations.yaml` + `README.md` + `driver/driver.go` (four-scenario orchestration per §7.1 with StatsAsserter for the 5-counter delta assertions) + `backends/backend.go` (minimal Go HTTP backend on port 18001 serving `backend\n` per §7.5; ~590 LoC); a NEW `BackendKind` enum value in `test/differential/fixture/fixture.go` + a matching `startHTTPFaultBackend` spawn helper in `test/differential/runner_test.go` + the blank-import for the fixture driver (~25 LoC delta — the SPEC §4.3 reference to `test/differential/runner.go` is a SPEC erratum: the actual fixture-registration site is `test/differential/runner_test.go`'s blank-import block per planner-time decision 11 below, mirroring 08.2 PLAN's identical erratum reconciliation); a NEW fuzzer `FuzzFaultConfigParse` (~50 LoC; 30s budget per ADR-0018; twelfth fuzzer overall — SHIPPED per planner-time decision 1 below); a `BEHAVIOR_CONTRACT.md` in-place edit per SPEC §13 (NEW `### envoy.filters.http.fault` subsection under the existing `## HTTP filter chain` umbrella per §13.1; `## Stat-name mapping ### 17-name table` extension to 22 names per §13.2; `## Timing tolerances` new fault-delay-accuracy bullet per §13.3; `## Equivalence Matrix` new row per §13.4; ADR-0015 / ADR-0061 / ADR-0073 forward-pointer notes per §13.5; ADR-0052 in-place edit authorisation carries forward); eight new ADRs ADR-0100..ADR-0107 per SPEC §8 (ADR-0100 package shape + boot registration + FactoryCtx extension; ADR-0101 runtimeConfig shape + 6-field-consumed / 11-field-silent-ignore decomposition + abort.http_status PGV [200, 600) validation + percentage-roll determinism; ADR-0102 delay async-resume mechanics; ADR-0103 abort terminal-replace mechanics + body byte-exact "fault filter abort" + 4-header set; **ADR-0104 header-driven fault path DEFERRED** per §11.5 empirical pin major surprise — request-header path coupled to delay.header_delay/abort.header_abort proto sub-messages, both deferred together; ADR-0105 max_active_faults concurrency cap + LBP-1 sixth application + markedActive Inc/Dec idempotency guard; ADR-0106 §9 HTTP filters family expansion shape — flat top-level rows + no-sibling-stub discipline; ADR-0107 BEHAVIOR_CONTRACT 17→22-name extension + response_rl_injected permanently-zero counter route A choice). After phase 09, the project has proven its eleventh-leading-edge engineering claim per SPEC §1: *envoy-go's HTTP filter framework can host a non-trivial production filter under the cors precedent's package-shape discipline; the framework's async-resume primitive is exercised in production for the first time; the per-route 3-tier merge (ADR-0073) carries through to a second filter under the wholesale-override discipline; the stats registry extends from 17 to 22 names without revisiting the SN1–SN8 flattening rules (per ADR-0061) — all under flat top-level row expansion (per ADR-0106) without a parent §9 row.* This is the FIRST §9 family-row to land; subsequent filters (header_mutation, buffer, local_ratelimit, …) follow the same row-as-its-own-phase pattern per BRAINSTORM Decisions 12, 13 + ADR-0106. ROADMAP row `09` flips `in-progress → done` AT the phase-done commit; the §9 family heading at ROADMAP line 56 stays unchanged (headings are not rows; per ADR-0106 settled here); STATE.md flips to `awaiting next planning` (against §9's family list for the next family-child) per `BOOTSTRAP_PROMPT.md` §5 lifecycle.

**Architecture:** The 09 surface is the additive registration of one new HTTP filter under `internal/filter/http/` plus a small FactoryCtx extension to thread the existing `*stats.Registry` + HCM `stat_prefix` into the per-filter factory invocation in `internal/filter/hcm/config.go::parseHTTPFiltersChain` (per ADR-0044 first-use anchored to fault's stats-registration need; ADR-0100 records the consequence). The fault filter's `New` factory runs at HCM-build time per ADR-0072's two-step pattern: (a) parses + validates the typed_config Any (rejects `tc == nil`, malformed Any, `abort.http_status` outside `[200, 600)` per §11.1 PGV mirror, and `delay.percentage > 0 && delay.fixed_delay <= 0`); (b) constructs a `*runtimeConfig` capturing the 6 consumed proto fields per §6.2; (c) allocates the closure-captured `*atomic.Int64` activeFaults counter shared across all per-instance `*filter` values from this factory; (d) registers the 5 fault.* stats (`http.<stat_prefix>.fault.aborts_injected` counter, `http.<stat_prefix>.fault.delays_injected` counter, `http.<stat_prefix>.fault.faults_overflow` counter, `http.<stat_prefix>.fault.active_faults` gauge, `http.<stat_prefix>.fault.response_rl_injected` counter — permanently-zero per route A of ADR-0107) on the registry from FactoryCtx; (e) returns a `FilterInstanceFactory` closure that allocates a fresh `*filter{cfg, active, faultStats}` per request. The per-instance `*filter` implements both `StreamDecoderFilter` and `StreamEncoderFilter` per the cors precedent (encode-side is no-op pass-through; decode-side carries all fault logic). `DecodeHeaders` body discipline (per §6.4): per-route 3-tier merge → headers-field exact-match gate → percentage-roll for delay + abort independently → max-active-faults check → start `time.AfterFunc` timer (delay-only or combined) OR fire `cb.SendLocalReply(http_status, "fault filter abort", OrderedHeaders{Content-Type: text/plain})` (abort-only) → return `StopIteration` if either fault triggered, else `Continue`. The timer's callback decides whether to fire abort (combined delay+abort case) or call `cb.ContinueDecoding()` (delay-only case); on either path it decrements `activeFaults` via the `markedActive` per-instance flag (sync.Once-equivalent guard ensuring exactly-one Inc/Dec balance under the OnDestroy-races-timer-callback case). `OnDestroy` calls `f.delayTimer.Stop()` if scheduled, then `decrementActive()` (idempotent via markedActive). `EncodeHeaders` / `DecodeData` / `EncodeData` / `DecodeTrailers` / `EncodeTrailers` are pass-through (Continue / DataContinue / TrailersContinue) — fault operates exclusively on the decode-headers phase. The per-route resolution (`routeConfigOrListener`) calls `cb.RequestRouteConfig()` and, when a per-route HTTPFault config is present, parses it WHOLESALE (NOT field-merge) per §11.7's empirical confirmation of ADR-0073's wholesale-override discipline — a per-route HTTPFault that omits `delay` does NOT inherit the listener-level `delay`. The fault filter is the first production exerciser of `cb.ContinueDecoding()` on the request side (the 07.1 `envoy_go_test` probe filter exercised it structurally; phase 09 makes it production). Concurrency model: per-instance state is race-free by the single-goroutine-per-stream invariant per ADR-0071 (no synchronization needed); the shared `activeFaults` counter is lock-free via `*atomic.Int64` per LBP-1 sixth application (after ADR-0072 HTTPRegistry, ADR-0079 ListenerFilterRegistry, ADR-0061 stats Registry, ADR-0091 drain Manager, ADR-0078 ChainBuilder closure capture); the timer goroutine and OnDestroy may race on the markedActive guard, mitigated by the single-goroutine-per-stream invariant making the read-modify-write race-free WITHIN an instance. Differential surface: fixture `0011-http-fault` runs 4 scenarios (delay-only listener-level-inheritance + combined delay+abort per-route + per-route wholesale-override demo + headers-field exact-match gate; per §7.1 refined per §11.5 from BRAINSTORM's original 5) under a small-static-backend probe + dual-proxy bootstrap per §7.4 verbatim YAML; stats assertions are exact-equality per the SN1–SN8 deterministic-flow rules per ADR-0061, with one allow-list extension for non-stdlib status text portions per planner-time decision 7 below.

**Tech Stack:**
- Go 1.23 (unchanged from 08.2; floor declared in `go.mod`'s `go 1.23.0` directive).
- Stdlib `errors`, `fmt`, `net/http`, `sync/atomic`, `time` — the new `internal/filter/http/fault/` package consumes only stdlib (no new module imports introduced by 09).
- `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3` (NEW import in this phase) — `*envoyextensionsfiltershttpfaultv3.HTTPFault` proto, `*FaultDelay`, `*FaultAbort`. Already present in `go.sum`'s transitive closure (the go-control-plane module-level dependency is unchanged from 08.2; no `go.mod` bump needed — verified at `## Execution preconditions` step 11 below).
- `github.com/envoyproxy/go-control-plane/envoy/config/route/v3` (existing; introduced by phase 04) — `*HeaderMatcher` for the headers-field exact-match gate per §11.8.
- `github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3` (existing; introduced by 07.1 cors) — `*StringMatcher` for the per-header `string_match.exact` matcher per §11.8.
- `google.golang.org/protobuf/types/known/anypb` (existing; introduced by 07.1) — `*anypb.Any` typed_config carrier consumed by `New(tc, ctx)`.
- `github.com/esalaine/envoy-go/internal/stats` (existing; introduced by phase 06.1) — `*stats.Registry` consumed by fault's `New` via FactoryCtx for the 5 new fault.* stat-name registrations per §13.2 + ADR-0107.
- `github.com/esalaine/envoy-go/internal/filter/http` (existing; introduced by phase 07.1) — `FactoryCtx` (extended in Task 2 with `Stats` + `StatPrefix` fields per ADR-0100), `HTTPFilter`, `HTTPFilterFactory`, `FilterInstanceFactory`, `OrderedHeaders`, `HeaderField`, `StreamDecoderFilter`, `StreamEncoderFilter`, `FilterHeadersStatus`, `FilterDataStatus`, `FilterTrailersStatus`, `Continue`, `StopIteration`, `DataContinue`, `TrailersContinue`, `DecoderFilterCallbacks` (with `SendLocalReply(status, body, OrderedHeaders)`, `ContinueDecoding()`, `RequestRouteConfig() proto.Message`), `EncoderFilterCallbacks`.
- `github.com/esalaine/envoy-go/internal/filter/http/cors` (existing; the package-shape precedent fault mirrors verbatim — TypeURL constant + New factory + filter struct + decoder + encoder + OnDestroy + per-route resolution via cb.RequestRouteConfig).
- `github.com/esalaine/envoy-go/test/differential/fixture` (existing; extended in Task 10 with a new `BackendKind` enum value `HTTPFault` per planner-time decision 13).
- `golangci-lint` v1.64.8 (ADR-0009, unchanged).
- Upstream Envoy `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008, unchanged) — fixture 0011's reference image AND the source of the SPEC §11.1–§11.8 empirical pins (all already executed at SPEC time and pinned verbatim in SPEC §11; no new empirical-pin work in 09's PLAN).
- `summerwind/h2spec` Docker image at the SHA pinned in `CONFORMANCE_PINS.md` (ADR-0051, unchanged in 09 — phase 09 touches no codec/framer/HPACK paths; the conformance gate (c) re-runs at the same pin and reports unchanged 53/53 PASS).
- `github.com/testcontainers/testcontainers-go` for the differential harness running fixture 0011's reference (Envoy in a Docker container) — same harness as 06.1/06.2/07.1/07.2/08.1/08.2's fixtures consume; phase 09 does NOT extend the harness's optional driver-side interfaces (the existing `Driver` + `BackendKindAware` + `StatsAsserter` shape is sufficient).
- **Forbidden runtime imports (D-3.2):** any C++/cgo binding to upstream Envoy's fault filter implementation; any third-party fault-injection library (`go-fault`, etc.). Test-side use is also forbidden. The `go.mod` post-09 must not list any new fault-related runtime dependencies.

---

## Scope check — why phase 09 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 PLAN's component-table convention):

- `internal/filter/http/fault/doc.go` ~40
- `internal/filter/http/fault/fault.go` ~400 + `fault_test.go` ~280 = ~680
- `internal/filter/http/fault/fuzz_test.go` (OPTIONAL → settled SHIP per planner-time decision 1) ~50
- `internal/filter/http/types.go` `FactoryCtx` extension (`Stats *stats.Registry` + `StatPrefix string` fields) ~+5 = ~+5
- `internal/filter/http/registry_test.go` or `types_test.go` extension (FactoryCtx field-coverage tests) ~+30 = ~+30
- `internal/filter/hcm/config.go::parseHTTPFiltersChain` `FactoryCtx` populate (Stats + StatPrefix from the existing `registry` + `statPrefix` locals) ~+5 = ~+5
- `internal/filter/hcm/config_test.go` extension (assert FactoryCtx threading via a stat-counting test factory) ~+40 = ~+40
- `cmd/envoy-go/main.go` one new `httpReg.Register(fault.TypeURL, fault.New)` line (alphabetical insert after envoygotest) + matching `import "github.com/esalaine/envoy-go/internal/filter/http/fault"` ~+3 = ~+3
- `test/fixtures/0011-http-fault/` (NEW directory — note: SPEC §4.3 says `test/differential/0011-http-fault/`, planner-time decision 11 corrects to `test/fixtures/0011-http-fault/` per the existing 0010-precedent location) — `envoy.yaml` ~70 + `envoy-go.yaml` ~70 + `expectations.yaml` ~80 + `README.md` ~80 + `driver/driver.go` ~250 + `backends/backend.go` ~40 = ~590
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPFault BackendKind = 8`) + doc-comment ~+15 = ~+15
- `test/differential/runner_test.go` blank-import addition + new `startHTTPFaultBackend` spawn helper ~+25 = ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.fault` subsection ~80 + §13.2 17→22-name table extension ~20 + §13.3 timing-tolerance bullet ~5 + §13.4 equivalence-matrix row ~3 + §13.5 forward-pointer notes ~10 = ~+120 = ~+120
- `docs/envoy-go/DECISIONS.md` (eight ADRs ADR-0100..ADR-0107) ~+450 = ~+450
- `docs/envoy-go/ROADMAP.md` row `09` `in-progress → done` flip + (UNCHANGED) §9 family heading at line 56 ~+1 net = ~+1
- `docs/envoy-go/STATE.md` advance to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` (NEW; lifecycle artefact) ~600 (per-task entry)
- `docs/envoy-go/phases/09-http-filter-fault/REVIEW.md` (NEW; lifecycle artefact) ~180

**Production code: ~430 LoC + ~370 LoC tests + ~50 LoC fuzzer + ~590 LoC fixture YAML/Go + ~620 LoC docs ≈ ~2060 LoC total** (production-only ~430 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~430 LoC; task count below is **17**, comfortably under the 25 limit). The SPEC's anticipated 8-ADR cluster (ADR-0100..ADR-0107) lands across 17 tasks per the table at `## ADRs introduced by this plan` below; no task lands more than 2 ADRs simultaneously. SPEC §1.3 (per BRAINSTORM Decisions 12, 13 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 09 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~15–25 tasks per ADR-0045; this PLAN lands at 17 tasks.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/fault/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`HTTPFault` proto with 6-field-consumed / 11-field-silent-ignore decomposition per ADR-0101); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (Continue + StopIteration on DecodeHeaders only — no data/trailers/encode-side states exercised — encode-side is no-op pass-through); (d) the cross-cutting ADR anchors (ADR-0100/0101/0102/0103/0104/0105/0107 — ADR-0106 is documentation-shape and not anchored in this package). Mirrors `internal/filter/http/cors/doc.go` shape (40 LoC precedent). Per SPEC §4.1. |
| `internal/filter/http/fault/fault.go` | NEW | Filter implementation. **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Unexported types (per SPEC §6.2 + §6.3):** `runtimeConfig` struct (8 fields per §6.2 — delayEnabled, delayPercentage, delayFixedDelay, abortEnabled, abortPercentage, abortHTTPStatus, matchHeaders []headerMatch, maxActiveFaults int64); `headerMatch` struct (canonicalized name + exactValue); `filter` struct (cfg *runtimeConfig + active *atomic.Int64 + faultStats + dcb + ecb + delayTimer *time.Timer + markedActive bool). **Helpers:** `parseRuntimeConfig(*HTTPFault) (*runtimeConfig, error)` (used by both New and routeConfigOrListener); `rollPercent(p float64) bool` (deterministic float64 percentage roll using crypto/rand-free `math/rand` seeded by the request's `f.dcb.RequestRouteConfig` proto pointer hash for differential-replay determinism, OR a per-instance `*rand.Rand` with documented seed-source choice — settled at planner-time decision 12); `matchesHeaders(http.Header, *runtimeConfig) bool`; `decrementActive()` (markedActive-guarded Inc/Dec balance). **DecodeHeaders body** (per SPEC §6.4): per-route 3-tier merge → matchesHeaders gate → percentage rolls (delay + abort independently) → max-active-faults cap check → start delay timer (delay-only OR combined) OR fire abort SendLocalReply (abort-only) → StopIteration if fault, else Continue. **Pass-through methods:** EncodeHeaders + DecodeData + EncodeData + DecodeTrailers + EncodeTrailers all no-op. **OnDestroy:** delayTimer.Stop() if scheduled; decrementActive(). Per SPEC §6.1–§6.6. |
| `internal/filter/http/fault/fault_test.go` | NEW | Unit tests per SPEC §14.1: `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_AbortHTTPStatusOutOfRange` (table-driven: 0, 9999, 100, 600 each → error mirroring §11.1 PGV constraint), `TestNew_DelayPercentageWithoutFixedDelay`, `TestNew_HappyPath`, `TestRuntimeConfig_FieldExtraction`, `TestNew_RegistersStats` (fault.New populates the 5 stats on the supplied Registry; stat-counter values start at 0), `TestDecodeHeaders_DelayOnly` (timer fires; ContinueDecoding called from timer goroutine after ≈ delay), `TestDecodeHeaders_AbortOnly` (SendLocalReply with status + "fault filter abort" + OrderedHeaders{Content-Type: text/plain}; return StopIteration), `TestDecodeHeaders_Combined` (timer fires; timer callback calls SendLocalReply NOT ContinueDecoding; assert timing), `TestDecodeHeaders_NoFaultPercentage0`, `TestDecodeHeaders_NoFaultHeaderMismatch`, `TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue` (uppercase value → no match), `TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName` (uppercase name → match), `TestDecodeHeaders_MaxActiveFaultsCapOverflow` (concurrent setup at cap → next fault skipped + faults_overflow stat increments), `TestOnDestroy_TimerStopped` (timer cancelled before fire; callback does not run; activeFaults decremented in OnDestroy), `TestOnDestroy_TimerAlreadyFired` (timer fires before OnDestroy; activeFaults decremented exactly once via markedActive guard), `TestPerRouteWholesaleOverride` (per-route HTTPFault wholesale-replaces listener-level config — confirms ADR-0073 wholesale-override discipline per §11.7), `TestFault_DelayTimerRace` (race-detector cycle: DecodeHeaders + OnDestroy fired concurrently in a goroutine pair under -race; markedActive guard ensures no double-Dec; per planner-time decision 10). |
| `internal/filter/http/fault/fuzz_test.go` | NEW (OPTIONAL → SHIPPED per planner-time decision 1) | `FuzzFaultConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the fault filter's `New` factory is a parser. ~50 LoC; 30s budget per ADR-0018; twelfth fuzzer overall (post-08.2's eleventh `FuzzDrainTransitions`). |
| `internal/filter/http/types.go` | MODIFIED | `FactoryCtx` struct extension per ADR-0100 first-use anchored framework consequence. Adds two new fields after the existing `Registry *HTTPRegistry`: `Stats *stats.Registry` (the `*stats.Registry` the per-filter factory uses for stat-name registration; non-nil at HCM-build time per ADR-0061's pre-Freeze discipline; may be nil in test code that does not exercise stat-bearing filters per the existing ADR-0085 nil-tolerance pattern carried forward); `StatPrefix string` (the HCM's `stat_prefix` per ADR-0061's `http.<stat_prefix>.<metric>` discipline; empty in test code that does not exercise stat-bearing filters). The doc-comment on each new field cross-refs ADR-0100 + ADR-0061 + the cors precedent's no-stats-needed history (07.1's two filters did not need stats; phase 09's fault is the first stats-bearing filter). Field doc-comment also notes that future stats-bearing filters (header_mutation per BRAINSTORM, jwt_authn per the gRPC family, ext_authz, etc.) consume the same fields — phase 09 is the first-mover; future filters reuse without further FactoryCtx extension. ~+5 LoC delta. |
| `internal/filter/http/registry_test.go` (or `types_test.go` per the codebase's existing test-file split — implementer settles at Task 2 by `grep -l FactoryCtx internal/filter/http/*_test.go` to locate the natural test home) | MODIFIED | New tests asserting FactoryCtx field-coverage: `TestFactoryCtx_StatsRegistryThreaded` (a test factory consumes `ctx.Stats` and `ctx.StatPrefix`, registers a counter, and asserts the registration happens on the supplied Registry); `TestFactoryCtx_NilStatsRegistryTolerated` (a test factory tolerates nil Stats — for legacy 07.1-style filters that do not need stats); `TestFactoryCtx_EmptyStatPrefixTolerated`. ~+30 LoC delta. |
| `internal/filter/hcm/config.go` | MODIFIED | `parseHTTPFiltersChain` body's `factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry})` call (line 297 in master HEAD `b33e04f`) widens to `factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry, Stats: registry, StatPrefix: statPrefix})`. The signature of `parseHTTPFiltersChain` itself widens to take `registry *stats.Registry` and `statPrefix string` parameters; the call site at line 199 (`chainConfig, err := parseHTTPFiltersChain(msg.GetHttpFilters(), httpRegistry)`) widens to thread `registry` (already present in `parseFilterWithCtx` scope) and `statPrefix` (already present and validated above). ~+5 LoC delta. The doc-comment on parseHTTPFiltersChain gains a paragraph noting that FactoryCtx now carries Stats + StatPrefix per ADR-0100 / phase-09 first-use anchor. |
| `internal/filter/hcm/config_test.go` | MODIFIED | New test `TestParseHTTPFiltersChain_FactoryCtxThreading` — registers a stat-counting test factory under a synthetic typeURL, parses an HCM config with that filter in the chain, asserts the test factory receives a non-nil `ctx.Stats` and a non-empty `ctx.StatPrefix` matching the HCM's `stat_prefix`. ~+40 LoC delta. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(fault.TypeURL, fault.New)` registration inserted after the existing `httpReg.Register(envoygotest.TypeURL, envoygotest.New)` line (currently line 113 in master HEAD `b33e04f`) and before `httpReg.Freeze()` (currently line 114). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/fault"` alphabetically among the existing filter-package imports (currently lines 28-30: cors, envoygotest, router → cors, envoygotest, fault, router). Per BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline, the resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Freeze()`. **No other wiring changes** — fault is HTTP-only, no listener/cluster/drain manager threading. ~+3 LoC delta (1 import line + 1 register line + the matching paragraph in the import block's paragraph order). |
| `test/fixtures/0011-http-fault/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go`, `backends/backend.go` per SPEC §7. **Note:** SPEC §4.3 references `test/differential/0011-http-fault/`; planner-time decision 11 below corrects to `test/fixtures/0011-http-fault/` per the existing 0010-graceful-drain precedent location. The 08.2 PLAN flagged the identical erratum for runner.go-vs-runner_test.go; phase 09's PLAN flags the directory-path erratum analogously. |
| `test/fixtures/0011-http-fault/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port 9902 in-container; listener port 10001; cluster `c_backend` STRICT_DNS pointing at the harness backend on port 18001 via `host.docker.internal` per ADR-0010). Single listener with the §7.4 SPEC-verbatim five-prefix layout: listener-level fault is `delay 100% 100ms` (no abort); per-route overrides on `/scenario2` (delay+abort 503), `/scenario3-wholesale` (abort 418 only), `/scenario3-baseline` (no override → inherit), `/scenario4` (abort 503 + headers gate). http_filters: `[envoy.filters.http.fault, envoy.filters.http.router]`. Per SPEC §7.4. |
| `test/fixtures/0011-http-fault/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo admin/listener port values (admin :9901 → resolved at boot by the runner; listener :10000 → resolved at boot). The shared `c_backend` cluster points at the harness backend port resolved at boot. Per SPEC §7.4. |
| `test/fixtures/0011-http-fault/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1: scenario 1 (`/scenario1/...`) → 200 + body byte-equal `backend\n`, time_total 100ms ±10ms, stat delta `delays_injected += 1`; scenario 2 (`/scenario2/...`) → 503 + body byte-equal `fault filter abort` (18 bytes, no newline) + 4-header set, time_total ≈ 100ms, stat delta `delays_injected += 1` AND `aborts_injected += 1`; scenario 3a (`/scenario3-wholesale/...`) → 418 + body `fault filter abort`, time_total < 50ms (NO inherited delay — wholesale-override per §11.7), stat delta `aborts_injected += 1`; scenario 3b (`/scenario3-baseline/...`) → 200 + body `backend\n`, time_total ≈ 100ms (inherited listener delay), stat delta `delays_injected += 1`; scenario 4 (`/scenario4/...`) → 4 sub-probes a/b/c/d per §7.1 scenario 4, stat delta `aborts_injected += 2` (probes 4b + 4c match; 4a + 4d miss). Status-text allow-list per planner-time decision 7: non-stdlib codes (e.g. 418 → `Unknown` upstream vs `I'm a teapot` envoy-go) compare on STATUS CODE only; status TEXT portion is allow-listed. The `fault.response_rl_injected` stat is allow-listed as `0 == 0` (envoy-go emits at zero per ADR-0107 route A; reference Envoy emits at zero because response_rate_limit is not configured). Cross-refs SPEC §7.1 + §13.1 + §13.2 + ADR-0103 + ADR-0107. Per SPEC §4.3. |
| `test/fixtures/0011-http-fault/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + four-scenario list (per §7.2) + dual-proxy bootstrap discipline (admin/listener ports disambiguated for dual-boot under `--network host` per the existing fixture pattern) + Envoy-deviation note (none — fault is a normal HTTP filter; no SIGTERM/drain divergence) + planner-time-decision cross-references. Per SPEC §4.3. |
| `test/fixtures/0011-http-fault/driver/driver.go` | NEW | Go driver implementing the §7.3 four-scenario orchestration. **Driver shape** (mirrors 0007a-cors per planner-time decision 8): `package driver`; `init()` calls `fixture.RegisterFixture("0011-http-fault", &faultDriver{})`; `BackendCount() int` returns 1; `BackendKind() fixture.BackendKind` returns `fixture.HTTPFault` (the new enum value added in Task 10); `SubjectListenerName() string` returns `"l_main"`; `SubjectListenerPort()` / `ReferenceListenerPort()` return the SPEC §7.4 port pair (10000 / 10001); `ReferenceBootstrap(backendPorts []int) string` templates `envoy.yaml` substituting `{{.BackendPort}}` with `host.docker.internal:` + `backendPorts[0]`; `SubjectConfig(...)` templates `envoy-go.yaml` with the runner-allocated subject ports + backend port; `DriveReference` / `DriveSubject` issue the four-scenario probe sequence (8 HTTP requests per proxy: scenario 1 = 1 + scenario 2 = 1 + scenario 3a + 3b = 2 + scenario 4a/b/c/d = 4) capturing per-probe status + body + headers + time_total; returns the captured per-probe assertion-log lines as a deterministic byte stream → CompareBytes between ref+subj passes when both emit the same log lines; `ProbeAdmin` issues `GET /ready` against both proxies + returns the bytes for the admin diff (per the existing 0007a/0010 pattern); `AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` scrapes both proxies' `/stats?filter=fault&format=prometheus` endpoints, parses the 5 fault.* stat values, and asserts per-scenario stat deltas per the §7.1 expectation matrix. **Synchronization:** event-based throughout (no hardcoded sleeps per the 08.2 SPEC §10 + 07.2 REVIEW M-8 carry-forward); the four scenarios run sequentially against each proxy. **Total per-proxy wall-clock:** <0.4s (dominated by scenario 1's 100ms + scenario 2's 100ms + scenario 3b's 100ms = 300ms of delay; remaining 5 probes are sub-10ms). Per SPEC §7.3. |
| `test/fixtures/0011-http-fault/backends/backend.go` | NEW | Minimal Go HTTP backend bound to a runner-allocated port. `/` endpoint serves a fast `200 OK` with body `backend\n` (8 bytes; matches the §11 empirical-pin backend used during SPEC drafting); accepts a `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper. ~40 LoC. Per SPEC §7.5. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPFault BackendKind = 8` after the existing `HTTPSlowStream BackendKind = 7`. Doc-comment notes: "HTTPFault is an out-of-process HTTP/1.1 backend: the runner spawns `test/fixtures/0011-http-fault/backends/backend.go` on the pre-allocated port. The backend serves `/` with body `backend\n` (8 bytes). No TLS. Introduced by fixture 0011-http-fault (phase 09 Task 10) to provide the deterministic-body backend the per-scenario equivalence assertions expect. Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver"` (insert in alphabetical order, after the `0010-graceful-drain` blank-import at line 35). (b) Extend the `kind` switch in `runFixture` (currently at lines ~94–222 in master HEAD) with a new case `fixture.HTTPFault` mirroring the `HTTPSlowStream` block: spawn via `startHTTPFaultBackend`. (c) Add new spawn helper `startHTTPFaultBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` mirroring `startHTTPSlowStreamBackend` (lines 781–793 in master HEAD): `exec.CommandContext(ctx, "go", "run", "./test/fixtures/0011-http-fault/backends", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.fault` subsection inserted under existing `## HTTP filter chain` umbrella (per §13.1; ~80 LoC); (b) `## Stat-name mapping ### 17-name table` heading renames to `### 22-name table (extended by phase 09)` with 5 new fault.* rows appended (per §13.2; ~20 LoC); (c) `## Timing tolerances` new fault-delay-accuracy bullet (per §13.3; ~5 LoC); (d) `## Equivalence Matrix` new fault-filter row (per §13.4; ~3 LoC); (e) three forward-pointer notes per §13.5: at `## HTTP filter chain ### Async resume mechanics`, at `## Stat-name mapping ### Twin-series filter discipline`, and at `## Equivalence Matrix` (~10 LoC total). ADR-0052 in-place edit authorisation carries forward. ~+120 LoC total. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | Append eight new ADRs ADR-0100..ADR-0107 per SPEC §8 (incrementally per task; each ADR's first-use commit anchors the addition per ADR-0044 ADR-on-impl convention). The 7-section ADR-0001 template applies to each (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0104 follows the ADR-0040 deferral-ADR format per §1.1 amendment. **Inline supersessions / amendments:** ADR-0061 (stats Registry / SN1–SN8 flattening) — purely additive extension recorded in ADR-0107 §Consequences cross-reference (no in-place edit of ADR-0061; the 22-name extension preserves SN1–SN8 unchanged); ADR-0073 (3-tier merge wholesale-override) — extended cross-reference recorded in ADR-0101 §Consequences (no in-place edit of ADR-0073; the wholesale-override is empirically confirmed via §11.7, which is documentation, not an amendment). ~+450 LoC total. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `09` `in-progress → done` flip AT the phase-done commit. The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106 settled by this phase). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 09 artefacts (per BRAINSTORM Decision 13 + ADR-0106 no-sibling-stub discipline). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 2 (PLAN drafting — this PLAN landing flips state 2 → 3 in the orchestrating session's STATE.md edit), 3 (PLAN execution — Tasks 1–14 land production code + fixture; STATE stays at 3), 4 (verification — Tasks 15–16 land BEHAVIOR_CONTRACT/ADRs/six-gate verification; STATE flips 3 → 4), 5 (review — Task 17 REVIEW.md per requesting-code-review skill; STATE flips 4 → 5), 6 (phase-done — STATE flips 5 → `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle; `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner). |
| `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..08.2 PROGRESS.md structure. The preamble enumerates the eight anticipated ADRs ADR-0100..ADR-0107 + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 13 items below). |
| `docs/envoy-go/phases/09-http-filter-fault/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 cadence; populates per the requesting-code-review skill. Phase 09 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 09. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's ten deferred decisions before implementation; this PLAN settles all ten plus three that emerged at PLAN-drafting time (items 11, 12, 13 below). The thirteen resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **`FuzzFaultConfigParse` ship-or-skip → SHIP.** Per SPEC §12 #1 recommendation. The fuzzer asserts that `New` returns either `(factory, nil)` OR `(nil, error)` for arbitrary byte sequences fed as the `tc *anypb.Any` parameter; never panics; never returns `(nil, nil)`. ~50 LoC; 30s budget per ADR-0018; twelfth fuzzer overall (post-08.2's eleventh `FuzzDrainTransitions`). Lands in Task 9. *Anchored: SPEC §12 #1 + §14.5; ADR-0018; ADR-0072 (factory-validates-typed_config contract).*

2. **`runtimeConfig` parser refactor (separate `parseRouteRuntimeConfig` vs single helper) → KEEP separate.** Per SPEC §12 #2 recommendation. The New-time variant has additional validation (`tc != nil` check, abort.http_status PGV mirror) that does NOT apply at per-route resolve time; consolidating would require either (a) duplicating the validation guards inside the consolidated helper conditional on a `isNewTime bool` flag or (b) exposing two entry points to the same internal helper. Both add complexity beyond the `parseRuntimeConfig` + `parseRouteRuntimeConfig` two-function split. *Anchored: SPEC §12 #2.*

3. **Stat-counter call-site organization → consolidate into a `recordFaultEvent(kind, increment bool)` helper.** Per SPEC §12 #3 recommendation. Cleaner test surface: tests can spy on a single helper invocation rather than asserting against three `stats.faultDelaysInjected.Inc()` / `stats.faultAbortsInjected.Inc()` / `stats.faultActiveFaults.Inc()` call sites. The helper takes an enum-shaped `kind` (delay / abort / overflow / activeInc / activeDec) and dispatches via switch. ~15 LoC; testability is positive. *Anchored: SPEC §12 #3.*

4. **Per-route runtimeConfig caching → SKIP.** Per SPEC §12 #4 recommendation. The chain's `RequestRouteConfig()` is already lazy-cached per-request (per `internal/filter/http/callbacks.go:35–36` + the PerRouteConfig.cache field per `internal/filter/http/perroute.go:38`). Adding a second cache layer at the parseRouteRuntimeConfig boundary adds ~200 LoC and saves a sub-microsecond proto-field-extraction pass per request. The chain's existing cache returns the parsed `*HTTPFault` proto; phase 09 calls parseRouteRuntimeConfig fresh on every call to project that proto into a `*runtimeConfig`. The per-request projection cost is negligible. *Anchored: SPEC §12 #4.*

5. **Fault stats → USE the existing `internal/stats.Registry` (06.1).** Per SPEC §12 #5 recommendation. Sub-registries are out of scope for phase 09; the existing Registry has the freeze-after-boot LBP-1 invariant (per ADR-0061) which fault's `New` factory respects (registers at HCM-build time, BEFORE Freeze in `cmd/envoy-go/main.go` flow). The per-HCM `stat_prefix` is plumbed into FactoryCtx per the framework extension Task 2 lands. *Anchored: SPEC §12 #5; ADR-0061.*

6. **`fault.response_rl_injected` route A vs route B → SETTLED at SPEC §1.1 amendment + ADR-0107 (route A: emit permanently-zero counter).** NOT a planner decision per SPEC §12 #6. The implementer at Task 3 registers the `response_rl_injected` counter alongside the other 4 fault.* stats; the counter is never Inc'd. *Anchored: SPEC §1.1 + §11.6 + ADR-0107.*

7. **Allow-list discipline for the abort-status text divergence → narrow allow-list scoped to non-stdlib status codes only.** Per SPEC §12 #7 recommendation. The differential equivalence is on STATUS CODE only for non-stdlib codes (e.g. 418: Envoy emits `HTTP/1.1 418 Unknown` because Envoy lacks a status-text table for non-RFC codes; envoy-go's `net/http` stdlib emits `HTTP/1.1 418 I'm a teapot`). Standard codes (200, 503, 404, 405) compare byte-equal on both status code AND status text. The expectations.yaml's allow-list documents this disposition explicitly. The driver's per-probe assertion compares the integer status code via `resp.StatusCode` (which strips the text portion) for non-stdlib codes; the raw status line is asserted byte-equal only for stdlib codes. Lands in Tasks 12–13. *Anchored: SPEC §12 #7 + §11.5 conclusion (d).*

8. **Fixture cluster type STATIC vs STRICT_DNS → STRICT_DNS pointing at the harness backend hostname.** Per SPEC §12 #8 recommendation. Mirrors 0007a-cors's STRICT_DNS pattern (the existing precedent for HTTP-filter fixtures). The harness's host-networking discipline per `BEHAVIOR_CONTRACT.md ## Test harness host networking` (line 489) requires `dns_lookup_family: V4_ONLY` for `host.docker.internal` resolution from inside the reference Envoy container. The envoy-go subject runs on the host (no Docker container) and resolves the hostname via Go's net resolver; both paths are tested in 0007a / 0010 and known-working. Lands in Task 12. *Anchored: SPEC §12 #8; ADR-0010; BEHAVIOR_CONTRACT.md ## Test harness host networking.*

9. **OrderedHeaders carrier from fault's SendLocalReply → SETTLED at SPEC §6.6 (option A: pass `OrderedHeaders{Content-Type: text/plain}` to override the chain's default `text/plain; charset=UTF-8`).** NOT a planner decision per SPEC §12 #9. The implementer at Task 4 invokes `f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})`. The chain's `reconcileOrderedHeaders` preserves the override + appends framework-injected `date` + `server` headers; the H1 wire writer auto-computes `content-length: 18` from the 18-byte body. *Anchored: SPEC §6.6; ADR-0075.*

10. **Race-detector cycle test for the timer-driven async-resume → ADD `TestFault_DelayTimerRace` under `-race`.** Per SPEC §12 #10 recommendation. Fires DecodeHeaders + OnDestroy concurrently in a goroutine pair to exercise the markedActive guard's read-modify-write under the race detector. ~30 LoC. Lands in Task 6. *Anchored: SPEC §12 #10; SPEC §5.7.*

11. **Fixture path → `test/fixtures/0011-http-fault/` (not `test/differential/0011-http-fault/`).** SPEC §4.3 + §7 reference `test/differential/0011-http-fault/` as the fixture root; this is a SPEC erratum (mirrors 08.2 PLAN's runner.go-vs-runner_test.go erratum reconciliation per its planner-time decision). The actual location convention per the existing 0010-graceful-drain precedent (verified at master `b33e04f`) is `test/fixtures/0011-http-fault/`. The driver lives at `test/fixtures/0011-http-fault/driver/`; the fixture-registration site is `test/differential/runner_test.go`'s blank-import block (per the same precedent). The implementer at Task 11 + Task 13 + Task 14 + Task 15 uses the corrected path. *Anchored: 0010-graceful-drain precedent at master `b33e04f`; SPEC §4.3 + §7 erratum.*

12. **Percentage-roll RNG source → per-instance `*math/rand.Rand` seeded by `time.Now().UnixNano()` at filter-instance allocation time (NOT at New-time).** New-time seed would share a deterministic seed across all per-request instances spawned from the same factory — defeating the percentage discipline. Per-request seed via `time.Now().UnixNano()` gives non-deterministic-across-requests rolls (the desired discipline for percentage gating). For the differential fixture's 0% / 100% scenarios, the percentage check short-circuits before the RNG is consulted (`p == 0 → false`; `p == 100 → true`); the RNG is only consulted for intermediate-percentage values which are unit-test-only per SPEC §2.3. The unit test `TestPercentageRollDeterminism_0_And_100` asserts the short-circuit; intermediate-percentage tests use a deterministic-seeded RNG injected via a test-only setter (`f.rng = rand.New(rand.NewSource(42))`). *Anchored: SPEC §2.3 + §6.4 (rollPercent body).*

13. **Fixture's new BackendKind enum value name → `HTTPFault` (BackendKind = 8).** Continues the existing naming convention (`HTTPHello`, `HTTPSlowStream`, etc.); the suffix names the fixture-purpose, not the protocol family. The implementer at Task 10 adds the enum constant + doc-comment block matching the existing `HTTPSlowStream BackendKind = 7` shape. *Anchored: existing fixture.BackendKind enumeration convention (lines 122–184 of `test/differential/fixture/fixture.go`).*

These thirteen decisions are reproduced verbatim in `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The eight ADRs anticipated by SPEC §8 (ADR-0100..ADR-0107). Each ADR's "Lands-in-task" anchor is fixed below per ADR-0044 ADR-on-impl convention; the implementer at the named task appends the ADR to `DECISIONS.md` per the ADR-0001 template. The eight ADRs land in topical-vs-commit-time-permuted order per the 07.1 / 07.2 / 08.1 / 08.2 PLAN convention; the per-task appendix records the ordering chosen by the implementer.

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0100 | `internal/filter/http/fault/` package shape (TypeURL + New + filter struct + decoder/encoder methods) + extension-registry registration line + boot-time `httpReg.Register(fault.TypeURL, fault.New)` + **FactoryCtx framework extension** (`Stats *stats.Registry` + `StatPrefix string` fields added per fault's first-use anchor) | Task 3 (`internal/filter/http/fault/{doc.go,fault.go,fault_test.go}` first lands; the FactoryCtx extension lands in Task 2 but ADR-0100 anchors at Task 3 because that's the first-use site that justifies the extension per ADR-0044). |
| ADR-0101 | `runtimeConfig` shape + 6-field-consumed / 11-field-silent-ignore decomposition + `abort.http_status` PGV `[200, 600)` validation at New time + `delay.percentage > 0 → delay.fixed_delay > 0` validation + percentage-roll determinism (per planner-time decision 12) | Task 3 (`parseRuntimeConfig` + New-time validation first lands). |
| ADR-0102 | Delay async-resume mechanics — `time.AfterFunc` timer-driven scheduling + `cb.ContinueDecoding()` from timer goroutine + cancel-on-OnDestroy + combined delay+abort via timer-callback decision (timer fires; callback calls SendLocalReply, NOT ContinueDecoding) | Task 5 (delay async-resume + combined path first lands). |
| ADR-0103 | Abort terminal-replace mechanics — `cb.SendLocalReply(http_status, "fault filter abort", OrderedHeaders{Content-Type: text/plain})` returning StopIteration; body byte-exact `"fault filter abort"` (18 bytes, NO trailing newline); 4-header set on the wire (`content-length: 18`, `content-type: text/plain` no charset, `date: <IMF-fixdate>`, `server: envoy`); status text allow-list for non-stdlib codes per planner-time decision 7 | Task 4 (abort terminal-replace path first lands). |
| ADR-0104 | **Header-driven fault path DEFERRED** (per ADR-0040 deferral-ADR format) — coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per SPEC §11.5 empirical-pin major surprise; phase 09 silently parses both sub-messages but does not honor them; the four documented request headers (`x-envoy-fault-{delay,abort}-request[-percentage]`) are silently ignored; future small follow-up phase (~150 LoC) lands the coupled pair in one coherent slice | Task 15 (BEHAVIOR_CONTRACT umbrella — the §13.1 `### envoy.filters.http.fault` subsection's `#### Does not yet apply to` paragraph IS the deferral table that ADR-0104 codifies). |
| ADR-0105 | `max_active_faults` concurrency cap + LBP-1 sixth application (after ADR-0072 HTTPRegistry, ADR-0079 ListenerFilterRegistry, ADR-0061 stats Registry, ADR-0091 drain Manager, ADR-0078 ChainBuilder closure capture) + `fault.faults_overflow` stat semantics + `markedActive` per-instance Inc/Dec idempotency guard (sync.Once-equivalent; race-clean by single-goroutine-per-stream invariant per ADR-0071) | Task 6 (max_active_faults atomic counter + markedActive guard + race-detector cycle test first lands). |
| ADR-0106 | §9 HTTP filters family expansion shape — flat top-level rows for §9 family-children + no-sibling-stub discipline + `BOOTSTRAP_PROMPT.md` §9 invariant 4 reading (the §9 heading at ROADMAP line 56 is a conceptual umbrella, not a row; its state stays unchanged across all family-row landings; future family-expansion brainstorms cold-start from the §9 heading + just-shipped artefacts) | Task 15 (BEHAVIOR_CONTRACT + ROADMAP cohesion — ADR-0106 governs the ROADMAP row 09 `in-progress → done` flip + the §9 heading's UNCHANGED status; lands alongside the BEHAVIOR_CONTRACT patches because both edits are one cohesive lifecycle disposition). |
| ADR-0107 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→22-name extension for FIVE `fault.*` stats (4 counters: aborts_injected, delays_injected, faults_overflow, response_rl_injected; 1 gauge: active_faults) + `response_rl_injected` permanently-zero counter discipline (route A — emit for differential parity; rationale: zero-cost + differential-parity-positive + forward-positive when response_rate_limit lands in a future phase) per SPEC §11.6 + §1.1 amendment | Task 7 (the 5 stat-name registrations land in fault's `New` factory consuming the FactoryCtx-supplied `*stats.Registry` — but ADR-0107 anchors at Task 7 because the Task 7 commit is where the BEHAVIOR_CONTRACT alignment between code-side stat registration and doc-side 22-name table commitment happens; the BEHAVIOR_CONTRACT patch itself lands in Task 15 but the ADR documents the route-A choice, which is a code-side decision realized in Task 3 + threaded into the doc in Task 15 — the implementer at Task 7 lands ADR-0107 alongside any minor stat-test refinement OR consolidates ADR-0107 into Task 3 if no Task 7 commit emerges; per the table here, the canonical anchor is **Task 3** alongside ADR-0100 + ADR-0101 to avoid an artificial Task 7 split — see Refinement note below). |

The implementer at each task drafts the ADR body following the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences); the per-task acceptance bullet "ADR-XXXX appears in DECISIONS.md with full Context/Decision/Consequences sections" enforces compliance.

**Refinement on ADR-0107 anchoring:** The table above shows ADR-0107 anchored at Task 7 for grouping clarity, but in practice the route-A decision is realized at the Task 3 commit (where fault.New registers the 5 stats). To avoid an artificial Task 7 commit-split for ADR documentation only, the implementer is AUTHORIZED to land ADR-0107 at Task 3 alongside ADR-0100 + ADR-0101. The Task 7 row in this PLAN's task list below is a STATS REFINEMENT slot (recordFaultEvent helper consolidation per planner-time decision 3 + minor test additions for stat-counter test coverage); ADR-0107 may consolidate INTO Task 3 if Task 7 produces no separate commit.

**Inline supersessions / amendments anticipated** (recorded inline in the listed ADRs above per the ADR-0089 consequence (b) in-place-edit pattern; NOT separate ADRs):

- **ADR-0061** (stats Registry / SN1–SN8 flattening rules) — purely additive 17→22-name extension recorded in ADR-0107 §Consequences cross-reference. The SN1–SN8 rules project unchanged onto the new fault.* names (`http.<stat_prefix>.fault.<counter>` → `envoy_http_fault_<counter>{envoy_http_conn_manager_prefix="<stat_prefix>"}` per SN1's "stat_prefix is a label, not part of the metric name"). NO in-place edit of ADR-0061; the 22-name extension preserves SN1–SN8 verbatim.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — extended cross-reference recorded in ADR-0100 §Consequences. The FactoryCtx extension (Stats + StatPrefix) is a strict superset of ADR-0072's existing FactoryCtx{Registry} shape; existing filter factories (router, cors, envoygotest) ignore the new fields gracefully. NO in-place edit of ADR-0072.
- **ADR-0073** (typed_per_filter_config 3-tier merge) — extended cross-reference recorded in ADR-0101 §Consequences. The wholesale-override discipline is empirically confirmed via SPEC §11.7 against the fault filter; no behavior change. NO in-place edit of ADR-0073.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0100 §Consequences. The filter set extends from {cors, envoy_go_test, router} to {cors, envoy_go_test, router, fault}. NO in-place edit of ADR-0074.
- **ADR-0075** (sendLocalReply enters encode chain at filter[len-1]) — extended cross-reference recorded in ADR-0103 §Consequences. The fault filter's abort path is the second production exerciser of the SendLocalReply primitive (cors's preflight is the first); the per-instance discipline is unchanged. NO in-place edit of ADR-0075.

These five cross-references land at the task that anchors each affected ADR (ADR-0100 at Task 3; ADR-0101 at Task 3; ADR-0103 at Task 4; ADR-0107 at Task 3 per the refinement above). No in-place edit of any pre-existing ADR is required.

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-09-http-filter-fault-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003 + the per-phase-worktree convention: `git worktree add .worktrees/phase-09-http-filter-fault-impl -b phase-09-http-filter-fault-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -8` shows the PLAN.md commit (this plan) and (optionally) its SHA-fill follow-up at the head, with the SPEC.md commit `da29807` and its SHA-fill follow-up `80b3f9f` immediately before, then the BRAINSTORM.md commit `4f44a03` and its SHA-fill `8506a3c`, then 08.2's phase-done at `b33e04f` (or its SHA-fill `14a68e7`). If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1` returns `ADR-0099:`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (ADR-0100..ADR-0107 may need bumping per ADR-0004).
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/SPEC.md` returns `da29807` (the SPEC commit). If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
7. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
8. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010'` returns every fixture PASS. The 12 pre-existing fixtures (0000–0010) are the regression baseline.
9. **Pre-existing fuzzers run clean at 30s.** The 11 fuzzers from phases 02–08.2 run clean (`go test -fuzz=Fuzz... -fuzztime=30s ./internal/...` for each). Phase 09 adds the twelfth (`FuzzFaultConfigParse` per Task 9).
10. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
11. **`envoy.extensions.filters.http.fault.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3 HTTPFault` returns the `HTTPFault` proto type's exported fields without an `import path failed` error. If `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `b33e04f` so a tidy should not be needed).
12. **Pre-existing `internal/filter/http/fault/` directory does NOT exist.** `test ! -d internal/filter/http/fault` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
13. **Pre-existing `FactoryCtx` has only the `Registry` field.** `grep -nE 'type FactoryCtx struct' internal/filter/http/types.go && sed -n '/type FactoryCtx struct/,/^}/p' internal/filter/http/types.go | grep -cE '^\s*\w+\s+\*?\w'` reports a single non-comment field line (the existing `Registry *HTTPRegistry`). If 2 or more, FactoryCtx has been extended by a concurrent phase — investigate.
14. **Pre-existing `parseHTTPFiltersChain` signature is the 2-param form.** `grep -nE '^func parseHTTPFiltersChain\(' internal/filter/hcm/config.go` returns 1 match; `grep -nE '^func parseHTTPFiltersChain\(filters \[\]\*hcmv3\.HttpFilter, httpRegistry \*filter_http\.HTTPRegistry\)' internal/filter/hcm/config.go` returns exactly 1 match. If 0, the signature has already been widened by a concurrent phase.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).

If all 15 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the eight ADRs ADR-0100..ADR-0107 are NOT all landed at Task 1 — each ADR lands at the task that anchors its first-use commit (per the table above). Task 1 lands NO ADR; the PROGRESS preamble simply ANTICIPATES the eight ADRs and records the planner-time decisions resolution. The PROGRESS preamble reproduces the thirteen planner-time deferred-decisions resolution items from this PLAN's `## Planner-time deferred-decision resolution` section verbatim, so any task-N reader has the full context without back-reading this PLAN.

**Precondition:** worktree exists at `phase-09-http-filter-fault-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 15 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-09-http-filter-fault-impl
git log --oneline master | head -8                                    # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (80b3f9f), SPEC (da29807), BRAINSTORM SHA-fill (8506a3c), BRAINSTORM (4f44a03), 08.2 SHA-fill (14a68e7), 08.2 phase-done (b33e04f)
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010' -v
                                                                       # expect: every fixture PASS
grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
                                                                       # expect: ADR-0099:
git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/SPEC.md
                                                                       # expect: da29807... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/fault && echo "ok: internal/filter/http/fault absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3 HTTPFault | head -5
                                                                       # expect: type HTTPFault struct { ... }
grep -cE '^\s*Registry \*HTTPRegistry' internal/filter/http/types.go  # expect: 1 (single field today)
grep -nE '^func parseHTTPFiltersChain\(filters \[\]\*hcmv3\.HttpFilter, httpRegistry \*filter_http\.HTTPRegistry\)' internal/filter/hcm/config.go
                                                                       # expect: 1 match
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Create `docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md`**

```markdown
# Phase 09 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..08.2 PROGRESS.md structure.

## Preamble — execution preconditions

<one paragraph: any deviation from PLAN.md's "Execution preconditions" block; "none" if all 15 preconditions were satisfied at cold-start>

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The eight ADRs anticipated by SPEC §8 (ADR-0100..ADR-0107). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0100** `internal/filter/http/fault/` package shape + boot registration + FactoryCtx framework extension — Task 3 (ADR text) + Task 2 (FactoryCtx extension code) + Task 8 (boot registration code)
- **ADR-0101** runtimeConfig shape + 6/11-field decomposition + abort.http_status PGV mirror + percentage-roll determinism — Task 3
- **ADR-0102** Delay async-resume mechanics + combined delay+abort timer-callback decision — Task 5
- **ADR-0103** Abort terminal-replace + body byte-exact + 4-header set + status-text allow-list — Task 4
- **ADR-0104** Header-driven fault path DEFERRED (per ADR-0040 deferral format) — Task 15
- **ADR-0105** max_active_faults concurrency cap + LBP-1 sixth + markedActive idempotency — Task 6
- **ADR-0106** §9 HTTP filters family expansion shape (flat top-level rows + no-sibling-stub) — Task 15
- **ADR-0107** 17→22-name extension + response_rl_injected route A — Task 3 (consolidated; per PLAN refinement note)

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The thirteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`FuzzFaultConfigParse` ship-or-skip = SHIP** (twelfth fuzzer; ~50 LoC; 30s budget per ADR-0018; lands in Task 9).
2. **`runtimeConfig` parser refactor = KEEP separate** (parseRuntimeConfig + parseRouteRuntimeConfig two-function split; New-time has additional validation that does not apply at per-route resolve time).
3. **Stat-counter call-site organization = consolidate into `recordFaultEvent(kind, increment bool)` helper** (cleaner test surface; ~15 LoC; lands in Task 3 alongside the stat registration).
4. **Per-route runtimeConfig caching = SKIP** (chain's RequestRouteConfig already lazy-cached; per-request projection cost is sub-microsecond).
5. **Fault stats = USE existing `internal/stats.Registry` (06.1)** (sub-registries out of scope; FactoryCtx extension threads *stats.Registry per Task 2).
6. **`fault.response_rl_injected` route A vs B = SETTLED at SPEC + ADR-0107 (route A: emit permanently-zero counter)** — not a planner decision.
7. **Allow-list discipline for abort-status text = narrow allow-list scoped to non-stdlib codes only** (200/503/404/405 byte-equal; 418 etc. compare on STATUS CODE only; lands in Task 12 expectations.yaml + Task 13 driver).
8. **Fixture cluster type = STRICT_DNS pointing at the harness backend hostname** (mirrors 0007a-cors precedent; ADR-0010 dns_lookup_family V4_ONLY).
9. **OrderedHeaders carrier from fault's SendLocalReply = SETTLED at SPEC §6.6 (option A: pass `OrderedHeaders{Content-Type: text/plain}`)** — not a planner decision.
10. **Race-detector cycle test for timer-driven async-resume = ADD `TestFault_DelayTimerRace`** (~30 LoC; lands in Task 6).
11. **Fixture path = `test/fixtures/0011-http-fault/`** (NOT `test/differential/0011-http-fault/` per SPEC §4.3 erratum; mirrors 0010-graceful-drain precedent).
12. **Percentage-roll RNG source = per-instance `*math/rand.Rand` seeded by `time.Now().UnixNano()` at filter-instance allocation time** (per-request seed for non-deterministic-across-requests rolls; 0% / 100% scenarios short-circuit before consulting RNG).
13. **Fixture's new BackendKind enum value name = `HTTPFault BackendKind = 8`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-09 SPEC + 09 PLAN confirmed present in HEAD; SPEC at da29807; ADR tail at 0099 (next-free 0100); internal/filter/http/fault/ absent (Task 3 lands); FactoryCtx single-field 2-param form (Task 2 widens); parseHTTPFiltersChain 2-param signature (Task 2 widens). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
\`\`\`
$ git rev-parse --abbrev-ref HEAD
<verbatim>
$ go version
<verbatim>
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
<verbatim>
$ git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/SPEC.md
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
git add docs/envoy-go/phases/09-http-filter-fault/PROGRESS.md
git commit -m "phase 09: PROGRESS preamble + planner-time decision resolution"
```

SHA-fill follow-up.

*Anchored: SPEC §8 (ADR anticipation table), §12 (deferred decisions), §15 (acceptance criteria) and BOOTSTRAP §5.3 (commit-message-completeness).*

---

## Task 2: `FactoryCtx` framework extension — `Stats *stats.Registry` + `StatPrefix string` fields + `parseHTTPFiltersChain` threading

**Files:**
- Modify: `internal/filter/http/types.go` (`FactoryCtx` struct extension)
- Modify: `internal/filter/http/types_test.go` (or `registry_test.go` per the file the codebase tests FactoryCtx in — implementer's `grep -l FactoryCtx internal/filter/http/*_test.go` settles)
- Modify: `internal/filter/hcm/config.go` (`parseHTTPFiltersChain` signature widening + FactoryCtx populate)
- Modify: `internal/filter/hcm/config_test.go` (FactoryCtx threading test)

This task lands the framework extension that fault's `New` factory needs to register its 5 stats at HCM-build time. Per ADR-0044 first-use anchored to fault's stats-registration need; ADR-0100 records the consequence at Task 3. The extension is a strict superset: existing filter factories (router, cors, envoygotest) ignore the new fields gracefully; no behavior change for the existing chain. Per planner-time decision 5 + the SPEC §4.2 "5 new stat-name registrations" requirement.

**Precondition:** Task 1 done; FactoryCtx single-field; parseHTTPFiltersChain 2-param.
**Artifact:** modified types.go + tests + config.go + tests; no ADR landed (ADR-0100 lands at Task 3).
**Acceptance:** `go build ./...` clean; `go test ./internal/filter/http/...` and `go test ./internal/filter/hcm/...` pass; existing 11 fixtures (0000–0010) re-run clean (the FactoryCtx extension is non-load-bearing for non-stat-bearing filters).

- [ ] **Step 1: Write failing test in `internal/filter/http/types_test.go` (or chosen file)**

```go
// TestFactoryCtx_StatsRegistryThreaded asserts a test factory can consume
// ctx.Stats and ctx.StatPrefix at filter-build time, register a counter on
// the supplied Registry, and have that counter survive into the per-request
// FilterInstanceFactory closure. Phase 09 fault filter is the first-use site
// per ADR-0100.
func TestFactoryCtx_StatsRegistryThreaded(t *testing.T) {
    reg := stats.NewRegistry()
    var capturedStats *stats.Registry
    var capturedPrefix string
    f := HTTPFilterFactory(func(_ *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
        capturedStats = ctx.Stats
        capturedPrefix = ctx.StatPrefix
        return func() HTTPFilter { return HTTPFilter{Name: "test"} }, nil
    })
    _, err := f(nil, FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
    if err != nil {
        t.Fatalf("factory: %v", err)
    }
    if capturedStats != reg {
        t.Errorf("Stats: got %p, want %p", capturedStats, reg)
    }
    if capturedPrefix != "ingress_http" {
        t.Errorf("StatPrefix: got %q, want %q", capturedPrefix, "ingress_http")
    }
}

// TestFactoryCtx_NilStatsRegistryTolerated ensures legacy 07.1-style filters
// (router, cors, envoygotest) that do not need stats can continue to receive
// FactoryCtx{} without the new fields populated.
func TestFactoryCtx_NilStatsRegistryTolerated(t *testing.T) {
    f := HTTPFilterFactory(func(_ *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
        if ctx.Stats != nil {
            t.Errorf("expected nil Stats, got %p", ctx.Stats)
        }
        if ctx.StatPrefix != "" {
            t.Errorf("expected empty StatPrefix, got %q", ctx.StatPrefix)
        }
        return func() HTTPFilter { return HTTPFilter{Name: "test"} }, nil
    })
    _, err := f(nil, FactoryCtx{})
    if err != nil {
        t.Fatalf("factory: %v", err)
    }
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/http/ -run TestFactoryCtx 2>&1 | head -10
```

Expected: compile error (`ctx.Stats undefined: FactoryCtx has no field or method Stats`).

- [ ] **Step 3: Extend `FactoryCtx` in `internal/filter/http/types.go`**

Modify the existing `FactoryCtx` struct (currently lines 248–253) to:

```go
// FactoryCtx carries the registry pointer + parsed proto-helpers + stat
// registration handle needed by per-filter parsers. Phase 09 (fault filter,
// per ADR-0100) is the first-use site for the Stats + StatPrefix fields:
// fault registers 5 counters/gauges on the Registry at HCM-build time per
// ADR-0061's pre-Freeze discipline. Existing 07.1 filters (router, cors,
// envoygotest) do not need Stats or StatPrefix and ignore them gracefully.
// Future stats-bearing filters (header_mutation, jwt_authn, ext_authz, etc.)
// reuse the same fields without further FactoryCtx extension.
type FactoryCtx struct {
    Registry *HTTPRegistry // optional reference for filter factories that need to look up sibling filters
    // Stats is the *stats.Registry the per-filter factory uses for stat-name
    // registration. Non-nil at HCM-build time per ADR-0061's pre-Freeze
    // discipline. May be nil in test code that does not exercise stat-bearing
    // filters; per ADR-0085 nil-tolerance pattern. Phase 09 first-use anchor;
    // ADR-0100 §Consequences records the framework consequence.
    Stats *stats.Registry
    // StatPrefix is the HCM's stat_prefix per ADR-0061's
    // "http.<stat_prefix>.<metric>" discipline. Empty in test code that does
    // not exercise stat-bearing filters. Phase 09 first-use anchor.
    StatPrefix string
}
```

Add the `"github.com/esalaine/envoy-go/internal/stats"` import to the import block.

- [ ] **Step 4: Run unit tests; confirm they pass**

```bash
go test ./internal/filter/http/ -run TestFactoryCtx -v
```

Expected: both new tests PASS.

- [ ] **Step 5: Write failing test in `internal/filter/hcm/config_test.go`**

```go
// TestParseHTTPFiltersChain_FactoryCtxThreading asserts parseHTTPFiltersChain
// populates FactoryCtx.Stats with the supplied Registry and FactoryCtx.StatPrefix
// with the HCM's stat_prefix when invoking each per-filter factory. Phase 09
// (ADR-0100) extends FactoryCtx with these fields so fault's New can register
// the 5 fault.* stats at HCM-build time.
func TestParseHTTPFiltersChain_FactoryCtxThreading(t *testing.T) {
    // Synthetic test factory that captures the FactoryCtx for assertion.
    var captured filter_http.FactoryCtx
    testFactory := filter_http.HTTPFilterFactory(func(_ *anypb.Any, ctx filter_http.FactoryCtx) (filter_http.FilterInstanceFactory, error) {
        captured = ctx
        return func() filter_http.HTTPFilter { return filter_http.HTTPFilter{Name: "test.factoryctx"} }, nil
    })
    httpReg := filter_http.NewHTTPRegistry()
    httpReg.Register("type.googleapis.com/test.FactoryCtxProbe", testFactory)
    httpReg.Register(router.TypeURL, router.New)
    httpReg.Freeze()

    reg := stats.NewRegistry()
    statPrefix := "ingress_http"

    filters := []*hcmv3.HttpFilter{
        {Name: "test.factoryctx", ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: &anypb.Any{TypeUrl: "type.googleapis.com/test.FactoryCtxProbe"}}},
        {Name: "envoy.filters.http.router", ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mustAny(t, &routerv3.Router{})}},
    }
    _, err := parseHTTPFiltersChain(filters, httpReg, reg, statPrefix)
    if err != nil {
        t.Fatalf("parseHTTPFiltersChain: %v", err)
    }
    if captured.Stats != reg {
        t.Errorf("FactoryCtx.Stats: got %p, want %p", captured.Stats, reg)
    }
    if captured.StatPrefix != statPrefix {
        t.Errorf("FactoryCtx.StatPrefix: got %q, want %q", captured.StatPrefix, statPrefix)
    }
}
```

- [ ] **Step 6: Run tests; confirm they fail**

```bash
go test ./internal/filter/hcm/ -run TestParseHTTPFiltersChain_FactoryCtxThreading 2>&1 | head -10
```

Expected: compile error (parseHTTPFiltersChain signature mismatch).

- [ ] **Step 7: Widen `parseHTTPFiltersChain` signature in `internal/filter/hcm/config.go`**

Modify line 273 from:

```go
func parseHTTPFiltersChain(filters []*hcmv3.HttpFilter, httpRegistry *filter_http.HTTPRegistry) ([]chainEntry, error) {
```

to:

```go
// Phase 09 (ADR-0100 first-use anchor): registry + statPrefix threaded into
// FactoryCtx so stats-bearing per-filter factories (fault, future header_mutation,
// jwt_authn, etc.) can register at HCM-build time per ADR-0061's pre-Freeze
// discipline. Existing 07.1 filters (router, cors, envoygotest) ignore the
// FactoryCtx Stats + StatPrefix fields gracefully.
func parseHTTPFiltersChain(filters []*hcmv3.HttpFilter, httpRegistry *filter_http.HTTPRegistry, registry *stats.Registry, statPrefix string) ([]chainEntry, error) {
```

Modify line 297 from:

```go
instanceFactory, err := factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry})
```

to:

```go
instanceFactory, err := factories[i](tcAny, filter_http.FactoryCtx{Registry: httpRegistry, Stats: registry, StatPrefix: statPrefix})
```

Modify the call site at line 199 from:

```go
chainConfig, err := parseHTTPFiltersChain(msg.GetHttpFilters(), httpRegistry)
```

to:

```go
chainConfig, err := parseHTTPFiltersChain(msg.GetHttpFilters(), httpRegistry, registry, statPrefix)
```

- [ ] **Step 8: Run unit tests; confirm they pass**

```bash
go test ./internal/filter/hcm/ -run TestParseHTTPFiltersChain_FactoryCtxThreading -v
go test ./internal/filter/hcm/ -count=1                                # full HCM suite
```

Expected: all PASS.

- [ ] **Step 9: Verify regressions**

```bash
go build ./...                                                # expect: clean
go vet ./...                                                  # expect: clean
golangci-lint run ./...                                       # expect: clean
go test -race -count=1 -short ./...                           # expect: all PASS
```

- [ ] **Step 10: Commit**

```bash
git add internal/filter/http/types.go internal/filter/http/types_test.go internal/filter/hcm/config.go internal/filter/hcm/config_test.go
git commit -m "phase 09: FactoryCtx extension — Stats + StatPrefix fields per ADR-0100 first-use anchor"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (5 new stat registrations), SPEC §6.1 (New factory signature), ADR-0100 §Consequences (FactoryCtx framework extension), ADR-0044 ADR-on-impl convention (ADR-0100 lands at Task 3 first-use), ADR-0061 (stats Registry pre-Freeze discipline), ADR-0085 (nil-tolerance pattern for FactoryCtx fields).*

---

## Task 3: `internal/filter/http/fault/` package — doc.go + fault.go core (TypeURL, types, runtimeConfig + parser, New factory + validation, stats registration) + fault_test.go (New-time tests) [ADR-0100, ADR-0101, ADR-0107]

**Files:**
- Create: `internal/filter/http/fault/doc.go`
- Create: `internal/filter/http/fault/fault.go`
- Create: `internal/filter/http/fault/fault_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0100, ADR-0101, ADR-0107)

This task lands the new `internal/filter/http/fault/` package with the public API surface (`TypeURL` constant + `New` HTTPFilterFactory) and the unexported types (`runtimeConfig`, `headerMatch`, `filter`). The `New` factory parses + validates the typed_config (rejects nil tc, malformed Any, abort.http_status outside [200, 600), delay.percentage > 0 without delay.fixed_delay > 0); constructs `*runtimeConfig` per §6.2; allocates the closure-captured `*atomic.Int64` activeFaults counter; registers the 5 fault.* stats on FactoryCtx.Stats keyed by `http.<FactoryCtx.StatPrefix>.fault.*`; returns the FilterInstanceFactory closure. The DecodeHeaders body is NOT yet implemented in this task (Task 4 lands abort path; Task 5 lands delay/combined paths; Tasks 6/7 land max-active + per-route refinements). The filter struct is allocated in the closure but its DecodeHeaders is a panic-stub or `return Continue` placeholder that subsequent tasks replace. **ADR-0100** (package shape + boot registration + FactoryCtx framework extension consequence), **ADR-0101** (runtimeConfig shape + abort.http_status PGV mirror + percentage-roll determinism), and **ADR-0107** (5-stat registration + response_rl_injected route A — consolidated per Refinement note above) all land here.

**Precondition:** Task 2 done; FactoryCtx widened.
**Artifact:** four new files (doc + impl + unit tests); three ADRs in DECISIONS.md.
**Acceptance:** `go build ./internal/filter/http/fault/...` clean; `go test ./internal/filter/http/fault/...` passes the New-time test suite; `go test -race ./internal/filter/http/fault/...` clean; ADR-0100, ADR-0101, ADR-0107 in DECISIONS.md.

- [ ] **Step 1: Write failing tests in `internal/filter/http/fault/fault_test.go`**

```go
package fault

import (
    "errors"
    "testing"

    faultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
    typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "google.golang.org/protobuf/types/known/anypb"
    "google.golang.org/protobuf/types/known/durationpb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
    t.Helper()
    a, err := anypb.New(m)
    if err != nil {
        t.Fatalf("anypb.New: %v", err)
    }
    return a
}

func TestNew_NilTC(t *testing.T) {
    _, err := New(nil, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err == nil {
        t.Fatal("expected error for nil tc; got nil")
    }
}

func TestNew_MalformedTC(t *testing.T) {
    bad := &anypb.Any{TypeUrl: "type.googleapis.com/garbage", Value: []byte{0xff, 0xff, 0xff}}
    _, err := New(bad, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err == nil {
        t.Fatal("expected error for malformed tc; got nil")
    }
}

func TestNew_AbortHTTPStatusOutOfRange(t *testing.T) {
    cases := []struct{ name string; status uint32 }{
        {"zero", 0}, {"too_high", 9999}, {"too_low", 100}, {"upper_exclusive", 600},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            f := &faultv3.HTTPFault{
                Abort: &faultv3.FaultAbort{
                    Percentage:    &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
                    ErrorType:     &faultv3.FaultAbort_HttpStatus{HttpStatus: tc.status},
                },
            }
            _, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
            if err == nil {
                t.Fatalf("status=%d: expected error; got nil", tc.status)
            }
        })
    }
}

func TestNew_DelayPercentageWithoutFixedDelay(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage: &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
        },
    }
    _, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err == nil {
        t.Fatal("expected error for delay.percentage > 0 without delay.fixed_delay; got nil")
    }
}

func TestNew_HappyPath(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:    &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(100 * time.Millisecond)},
        },
        Abort: &faultv3.FaultAbort{
            Percentage:    &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:     &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"})
    if err != nil {
        t.Fatalf("happy path: %v", err)
    }
    if factory == nil {
        t.Fatal("factory is nil")
    }
    inst := factory()
    if inst.Decoder == nil || inst.Encoder == nil {
        t.Errorf("expected both Decoder and Encoder set; got %+v", inst)
    }
    if inst.Name != "envoy.filters.http.fault" {
        t.Errorf("Name: got %q, want %q", inst.Name, "envoy.filters.http.fault")
    }
}

func TestNew_RegistersStats(t *testing.T) {
    reg := stats.NewRegistry()
    f := &faultv3.HTTPFault{
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
        },
    }
    _, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    expectedNames := []string{
        "http.ingress_http.fault.aborts_injected",
        "http.ingress_http.fault.delays_injected",
        "http.ingress_http.fault.faults_overflow",
        "http.ingress_http.fault.active_faults",
        "http.ingress_http.fault.response_rl_injected",
    }
    seen := map[string]bool{}
    reg.Walk(func(m stats.Metric) { seen[m.Name()] = true })
    for _, n := range expectedNames {
        if !seen[n] {
            t.Errorf("missing stat: %q", n)
        }
    }
}

func TestRuntimeConfig_FieldExtraction(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 25, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(50 * time.Millisecond)},
        },
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: 75, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 418},
        },
        MaxActiveFaults: wrapperspb.UInt32(5),
        Headers: []*routev3.HeaderMatcher{
            {Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
        },
    }
    rc, err := parseRuntimeConfig(f)
    if err != nil {
        t.Fatalf("parseRuntimeConfig: %v", err)
    }
    if !rc.delayEnabled || rc.delayPercentage != 25.0 || rc.delayFixedDelay != 50*time.Millisecond {
        t.Errorf("delay fields: got enabled=%v p=%v d=%v", rc.delayEnabled, rc.delayPercentage, rc.delayFixedDelay)
    }
    if !rc.abortEnabled || rc.abortPercentage != 75.0 || rc.abortHTTPStatus != 418 {
        t.Errorf("abort fields: got enabled=%v p=%v s=%v", rc.abortEnabled, rc.abortPercentage, rc.abortHTTPStatus)
    }
    if rc.maxActiveFaults != 5 {
        t.Errorf("maxActiveFaults: got %v, want 5", rc.maxActiveFaults)
    }
    if len(rc.matchHeaders) != 1 || rc.matchHeaders[0].name != "X-Fault-On" || rc.matchHeaders[0].exactValue != "yes" {
        t.Errorf("matchHeaders: got %+v", rc.matchHeaders)
    }
}
```

(Note: import names like `commonfaultv3` need to be settled at impl time per the actual go-control-plane proto package layout — the FaultDelay type lives in `envoy/extensions/filters/common/fault/v3` while HTTPFault + FaultAbort live in `envoy/extensions/filters/http/fault/v3`. The implementer's `go doc` confirms the exact package paths at Task 3 step 1.)

- [ ] **Step 2: Run tests; confirm they fail (compile error — package does not exist)**

```bash
go test ./internal/filter/http/fault/... 2>&1 | head -10
```

Expected: `no Go files in internal/filter/http/fault`.

- [ ] **Step 3: Write `internal/filter/http/fault/doc.go`**

```go
// Package fault implements the envoy.filters.http.fault HTTP filter.
//
// Phase 09: real Envoy filter, wire-shape pinned by SPEC §11.1–§11.8 empirical
// scrapes of reference Envoy v1.37.2.
//
// Decode side (per SPEC §6.4):
//
//   1. Per-route 3-tier merge resolves the listener-level OR per-route
//      *runtimeConfig (wholesale-override per ADR-0073 + §11.7).
//   2. Headers-field exact-match gate: if non-empty, ALL listed (name, value)
//      pairs must match; header NAME match is case-insensitive (per HTTP/1.1
//      RFC 7230); header VALUE match is case-sensitive byte-equality
//      (StringMatcher.exact only — non-exact matchers silent-ignored).
//   3. Percentage rolls: delay + abort independently; 0% short-circuits to
//      false; 100% short-circuits to true; intermediate values consult the
//      per-instance *math/rand.Rand seeded by time.Now().UnixNano().
//   4. max_active_faults check: if > 0 AND *atomic.Int64 active >= cap,
//      increment fault.faults_overflow and SKIP (no fault injected; return
//      Continue).
//   5. Fault path: fire delay timer (delay-only or combined) OR fire abort
//      SendLocalReply (abort-only); return StopIteration.
//
// Async-resume mechanics (per ADR-0102): the delay path uses time.AfterFunc
// to schedule a callback that calls cb.ContinueDecoding() (delay-only) OR
// cb.SendLocalReply() (combined delay+abort) from the timer goroutine. The
// chain parks at StopIteration and resumes from the timer goroutine. OnDestroy
// calls f.delayTimer.Stop() to cancel the timer on request teardown.
//
// Abort terminal-replace (per ADR-0103): the abort path calls
// cb.SendLocalReply(http_status, "fault filter abort", OrderedHeaders{
// {Name: "Content-Type", Value: "text/plain"}}) and returns StopIteration.
// Body is byte-exact "fault filter abort" (18 bytes, NO trailing newline).
// The OrderedHeaders carrier overrides the chain's default content-type
// charset modifier; the framework appends date + server + content-length.
//
// max_active_faults concurrency cap (per ADR-0105): a closure-captured
// *atomic.Int64 counter (LBP-1 sixth application) is shared across all
// per-instance *filter values from the same factory. Hot path is lock-free.
// The markedActive per-instance bool is a sync.Once-equivalent guard ensuring
// exactly-one Inc/Dec balance under the OnDestroy-races-timer-callback case;
// race-clean by the single-goroutine-per-stream invariant per ADR-0071.
//
// Per-route policy: resolved via DecoderFilterCallbacks.RequestRouteConfig()
// which returns the merged *faultv3.HTTPFault from the perRouteConfig 3-tier
// merge (Route > VirtualHost > RouteConfiguration; ADR-0073). When non-nil,
// the per-route config WHOLESALE-replaces the listener-level config — a
// per-route HTTPFault that omits delay does NOT inherit the listener-level
// delay (empirically confirmed at SPEC §11.7).
//
// Encode side: no-op pass-through. Fault operates exclusively on the decode-
// headers phase.
//
// Stats (per ADR-0107): 5 stats registered at HCM-build time on the
// *stats.Registry from FactoryCtx — 4 counters (aborts_injected,
// delays_injected, faults_overflow, response_rl_injected — last permanently
// zero per route A) + 1 gauge (active_faults).
//
// Deferrals (per ADR-0104 + SPEC §2): header-driven fault path
// (x-envoy-fault-{delay,abort}-request[-percentage]) is silently ignored;
// coupled to delay.header_delay / abort.header_abort proto sub-messages
// (deferred together per §11.5 empirical pin major surprise; future small
// follow-up phase ~150 LoC lands the coupled pair). response_rate_limit,
// abort.grpc_status, upstream_cluster, downstream_nodes,
// disable_downstream_cluster_stats, all four runtime-key fields,
// filter_enabled / filter_enabled_runtime: silently ignored at fault-eval
// time. HeaderMatcher non-exact variants (regex, prefix, suffix, contains,
// present-only): silently ignored.
//
// References:
//   - SPEC §1–§16 (full contract)
//   - ADR-0100 (package shape + boot registration + FactoryCtx framework
//     extension)
//   - ADR-0101 (runtimeConfig shape + PGV mirror + percentage-roll determinism)
//   - ADR-0102 (delay async-resume + combined-path timer-callback decision)
//   - ADR-0103 (abort terminal-replace + body byte-exact + 4-header set)
//   - ADR-0104 (header-driven fault path DEFERRED)
//   - ADR-0105 (max_active_faults + LBP-1 sixth + markedActive guard)
//   - ADR-0107 (17→22-name stat extension + response_rl_injected route A)
package fault
```

- [ ] **Step 4: Write `internal/filter/http/fault/fault.go` (Task 3 scope: types + parser + New + stats; DecodeHeaders is a stub; Tasks 4–7 fill in)**

```go
package fault

import (
    "errors"
    "fmt"
    "math/rand"
    "net/http"
    "sync/atomic"
    "time"

    commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
    faultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
    routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
    matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
    typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.fault typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 8) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"

// faultAbortBody is the byte-exact response body the abort path emits.
// 18 bytes; NO trailing newline (per SPEC §11.3/§11.4 empirical pin).
const faultAbortBody = "fault filter abort"

// faultStats holds the 5 fault.* stats registered at HCM-build time per
// ADR-0107. response_rl_injected is permanently zero in phase 09 (route A —
// future fault-extension phase or bandwidth_limit filter populates it).
type faultStats struct {
    abortsInjected     *stats.Counter
    delaysInjected     *stats.Counter
    faultsOverflow     *stats.Counter
    activeFaults       *stats.Gauge
    responseRLInjected *stats.Counter // permanently zero in phase 09
}

// runtimeConfig is the per-instance / per-route parsed config shape per ADR-0101.
type runtimeConfig struct {
    delayEnabled    bool
    delayPercentage float64       // [0, 100]
    delayFixedDelay time.Duration // 0 if delay.header_delay set (silent-ignore path)

    abortEnabled    bool
    abortPercentage float64 // [0, 100]
    abortHTTPStatus int     // PGV-validated [200, 600) at New time

    matchHeaders []headerMatch // empty = match-all; only string_match.exact honored

    maxActiveFaults int64 // 0 = no cap
}

// headerMatch is one canonical-name + exact-value entry for the headers field.
type headerMatch struct {
    name       string // canonicalized via http.CanonicalHeaderKey at parse time
    exactValue string // string_match.exact (only matcher variant honored per §11.8)
}

// New is the HTTPFilterFactory exposed at boot. Per ADR-0100 + ADR-0101:
//
//  1. tc must be non-nil (a fault filter with no typed_config has no
//     behavioral effect; surface configuration mistakes at boot per
//     ADR-0072 boot-time-fail-fast).
//  2. Unmarshal tc to *faultv3.HTTPFault; return error on malformed Any.
//  3. Validate abort.http_status ∈ [200, 600) when abort != nil per §11.1
//     PGV mirror.
//  4. Validate delay.fixed_delay > 0 when delay != nil AND delay.percentage > 0.
//  5. Construct *runtimeConfig per §6.2.
//  6. Allocate closure-captured *atomic.Int64 activeFaults counter (LBP-1
//     sixth; shared across per-instance values per ADR-0105).
//  7. Register the 5 fault.* stats on ctx.Stats keyed by
//     "http.<ctx.StatPrefix>.fault.<metric>" per ADR-0107.
//  8. Return FilterInstanceFactory closure that allocates a fresh *filter
//     per request bound to (cfg, active, stats).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("fault: typed_config required")
    }
    var c faultv3.HTTPFault
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("fault: unmarshal: %w", err)
    }
    rc, err := parseRuntimeConfig(&c)
    if err != nil {
        return nil, err
    }
    activeFaults := new(atomic.Int64)
    fs := registerFaultStats(ctx.Stats, ctx.StatPrefix)
    return func() envoyhttp.HTTPFilter {
        f := &filter{
            cfg:    rc,
            active: activeFaults,
            stats:  fs,
            rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
        }
        return envoyhttp.HTTPFilter{
            Name:    "envoy.filters.http.fault",
            Decoder: f,
            Encoder: f,
        }
    }, nil
}

// parseRuntimeConfig projects the proto into the runtimeConfig shape per §6.2.
// Used by both New (with full validation) and parseRouteRuntimeConfig (Task 7
// adds per-route variant; the validation guards in this function fire on
// per-route inputs too — wholesale-override per §11.7 means per-route configs
// are independently validated).
func parseRuntimeConfig(c *faultv3.HTTPFault) (*runtimeConfig, error) {
    rc := &runtimeConfig{}
    if d := c.GetDelay(); d != nil {
        rc.delayPercentage = percentageToFloat(d.GetPercentage())
        rc.delayFixedDelay = d.GetFixedDelay().AsDuration()
        rc.delayEnabled = rc.delayFixedDelay > 0 // header_delay deferred per ADR-0104
        if rc.delayPercentage > 0 && rc.delayFixedDelay <= 0 {
            return nil, errors.New("fault: delay.fixed_delay required when delay.percentage > 0")
        }
    }
    if a := c.GetAbort(); a != nil {
        rc.abortPercentage = percentageToFloat(a.GetPercentage())
        if hs := a.GetHttpStatus(); hs != 0 || a.GetErrorType() != nil {
            // Either HttpStatus is explicitly set or the proto field is present.
            // PGV mirror: must be in [200, 600).
            if hs < 200 || hs >= 600 {
                return nil, fmt.Errorf("fault: abort.http_status %d out of range [200, 600)", hs)
            }
            rc.abortHTTPStatus = int(hs)
            rc.abortEnabled = true
        }
        // header_abort + grpc_status deferred per ADR-0104 / ADR-0103.
    }
    if m := c.GetMaxActiveFaults(); m != nil {
        rc.maxActiveFaults = int64(m.GetValue())
    }
    if hs := c.GetHeaders(); len(hs) > 0 {
        rc.matchHeaders = make([]headerMatch, 0, len(hs))
        for _, h := range hs {
            sm := h.GetStringMatch()
            if sm == nil {
                continue // non-exact matchers silent-ignored per §11.8 deferral
            }
            exact := sm.GetExact()
            if exact == "" {
                continue // non-exact StringMatcher variants silent-ignored
            }
            rc.matchHeaders = append(rc.matchHeaders, headerMatch{
                name:       http.CanonicalHeaderKey(h.GetName()),
                exactValue: exact,
            })
        }
    }
    return rc, nil
}

// percentageToFloat projects FractionalPercent into a float64 in [0, 100].
// Envoy's FractionalPercent denominator is one of HUNDRED / TEN_THOUSAND / MILLION.
func percentageToFloat(p *typev3.FractionalPercent) float64 {
    if p == nil {
        return 0
    }
    num := float64(p.GetNumerator())
    switch p.GetDenominator() {
    case typev3.FractionalPercent_HUNDRED:
        return num
    case typev3.FractionalPercent_TEN_THOUSAND:
        return num / 100.0
    case typev3.FractionalPercent_MILLION:
        return num / 10000.0
    }
    return 0
}

// registerFaultStats registers the 5 fault.* stats on the supplied Registry
// per ADR-0107. Tolerates nil registry (test code per ADR-0085 nil-tolerance).
func registerFaultStats(reg *stats.Registry, prefix string) *faultStats {
    if reg == nil {
        return &faultStats{} // all-nil; recordFaultEvent guards on nil
    }
    p := "http." + prefix + ".fault."
    return &faultStats{
        abortsInjected:     reg.NewCounter(p + "aborts_injected"),
        delaysInjected:     reg.NewCounter(p + "delays_injected"),
        faultsOverflow:     reg.NewCounter(p + "faults_overflow"),
        activeFaults:       reg.NewGauge(p + "active_faults"),
        responseRLInjected: reg.NewCounter(p + "response_rl_injected"),
    }
}

// filter is the per-request fault-filter instance. Per-instance state is
// race-free by the single-goroutine-per-stream invariant per ADR-0071.
// Tasks 4–7 fill in the DecodeHeaders body, the timer wiring, the per-route
// resolution, and the markedActive guard.
type filter struct {
    cfg    *runtimeConfig
    active *atomic.Int64
    stats  *faultStats
    rng    *rand.Rand

    dcb envoyhttp.DecoderFilterCallbacks
    ecb envoyhttp.EncoderFilterCallbacks

    delayTimer   *time.Timer
    markedActive bool
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders is a stub at Task 3; Tasks 4–7 replace.
func (f *filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    return envoyhttp.Continue
}

// Encode-side and data/trailer methods are no-op pass-through.
func (f *filter) DecodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}
func (f *filter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus {
    return envoyhttp.Continue
}
func (f *filter) EncodeData([]byte, bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue
}

// OnDestroy is a stub at Task 3; Task 6 fills in the timer-cancel + Dec.
func (f *filter) OnDestroy() {}
```

- [ ] **Step 5: Run unit tests; confirm they pass**

```bash
go test ./internal/filter/http/fault/... -v
```

Expected: all New-time tests PASS; the DecodeHeaders/OnDestroy bodies are stubs and not exercised at Task 3.

- [ ] **Step 6: Verify build + race**

```bash
go build ./...
go test -race -count=1 ./internal/filter/http/fault/...
```

Expected: clean.

- [ ] **Step 7: Append ADR-0100, ADR-0101, ADR-0107 to `docs/envoy-go/DECISIONS.md`**

Each ADR follows the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). Anchored at SPEC §8 + the per-ADR §6 / §11 / §13.2 sections. Each ADR's Lands-in-task field names "Task 3 (phase 09)" + the commit SHA (TBD until Step 8 completes; SHA-fill follow-up commit lands the SHA per the phase-04..08.2 SHA-fill convention).

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/fault/ docs/envoy-go/DECISIONS.md
git commit -m "phase 09: fault package + runtimeConfig + 5-stat registration [ADR-0100, ADR-0101, ADR-0107]"
```

SHA-fill follow-up.

*Anchored: SPEC §4.1 (deliverables), §6.1 (New factory contract), §6.2 (runtimeConfig shape), §6.3 (filter struct), §11.1 (PGV mirror), §11.6 (5-stat extension), §13.2 (22-name table), §14.1 (unit-test list); ADR-0100 (package shape + FactoryCtx extension consequence), ADR-0101 (runtimeConfig + PGV + percentage-roll), ADR-0107 (17→22-name extension + response_rl_injected route A); ADR-0044 ADR-on-impl convention.*

---

## Task 4: `fault.go` DecodeHeaders abort terminal-replace path + headers-field gate + percentage-roll + recordFaultEvent helper [ADR-0103]

**Files:**
- Modify: `internal/filter/http/fault/fault.go` (DecodeHeaders body, abort path; recordFaultEvent helper; matchesHeaders helper; rollPercent helper; decrementActive helper stub)
- Modify: `internal/filter/http/fault/fault_test.go` (DecodeHeaders abort + percentage + headers tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0103)

This task lands the DecodeHeaders body for the **abort-only** scenario (the simplest synchronous path) plus the supporting helpers. Per SPEC §6.4 + §5.3 + §11.3: when `abort.percentage` rolls hit AND `headers` field matches AND `max_active_faults` cap not reached, fire `cb.SendLocalReply(cfg.abortHTTPStatus, "fault filter abort", OrderedHeaders{Content-Type: text/plain})` and return StopIteration. The 4-header set on the wire (content-length: 18, content-type: text/plain no charset, date, server) is reconciled by the chain framework — fault only contributes the OrderedHeaders override for content-type. Tasks 5–7 fill in the delay path, max_active_faults Inc/Dec, and per-route resolution. **ADR-0103** lands here.

**Precondition:** Task 3 done; fault package present; New + parseRuntimeConfig + stats wired.
**Artifact:** modified fault.go (DecodeHeaders + helpers); modified fault_test.go (abort/headers/percentage tests); ADR-0103 in DECISIONS.md.
**Acceptance:** `go test ./internal/filter/http/fault/... -v` passes the new abort/headers/percentage tests + the existing Task 3 tests; `go test -race ./internal/filter/http/fault/...` clean; ADR-0103 in DECISIONS.md.

- [ ] **Step 1: Write failing tests for the abort-only / headers / percentage paths**

```go
// recordingDCB captures SendLocalReply + ContinueDecoding invocations.
type recordingDCB struct {
    sentStatus  int
    sentBody    string
    sentHeaders envoyhttp.OrderedHeaders
    continued   atomic.Int32
    routeCfg    proto.Message
}

func (r *recordingDCB) SendLocalReply(s int, b string, h envoyhttp.OrderedHeaders) {
    r.sentStatus = s
    r.sentBody = b
    r.sentHeaders = h
}
func (r *recordingDCB) ContinueDecoding()                                    { r.continued.Add(1) }
func (r *recordingDCB) RequestRouteConfig() proto.Message                    { return r.routeCfg }
func (r *recordingDCB) EncodeHeaders(http.Header, bool)                      {}
func (r *recordingDCB) EncodeData([]byte, bool)                              {}
func (r *recordingDCB) EncodeTrailers(http.Header)                           {}

func makeFilter(t *testing.T, abortStatus uint32, abortPercent uint32, headers []*routev3.HeaderMatcher) (*filter, *recordingDCB) {
    t.Helper()
    f := &faultv3.HTTPFault{
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: abortPercent, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: abortStatus},
        },
        Headers: headers,
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"})
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    inst := factory()
    fl := inst.Decoder.(*filter)
    dcb := &recordingDCB{}
    fl.SetDecoderCallbacks(dcb)
    return fl, dcb
}

func TestDecodeHeaders_AbortOnly_100Percent(t *testing.T) {
    fl, dcb := makeFilter(t, 503, 100, nil)
    status := fl.DecodeHeaders(http.Header{}, true)
    if status != envoyhttp.StopIteration {
        t.Errorf("status: got %v, want StopIteration", status)
    }
    if dcb.sentStatus != 503 {
        t.Errorf("sentStatus: got %d, want 503", dcb.sentStatus)
    }
    if dcb.sentBody != "fault filter abort" {
        t.Errorf("sentBody: got %q, want %q", dcb.sentBody, "fault filter abort")
    }
    if got, want := len(dcb.sentBody), 18; got != want {
        t.Errorf("body length: got %d, want %d (no trailing newline)", got, want)
    }
    if len(dcb.sentHeaders) != 1 || dcb.sentHeaders[0].Name != "Content-Type" || dcb.sentHeaders[0].Value != "text/plain" {
        t.Errorf("sentHeaders: got %+v, want OrderedHeaders{Content-Type: text/plain}", dcb.sentHeaders)
    }
}

func TestDecodeHeaders_AbortOnly_0Percent(t *testing.T) {
    fl, dcb := makeFilter(t, 503, 0, nil)
    status := fl.DecodeHeaders(http.Header{}, true)
    if status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue (0%% should not fire)", status)
    }
    if dcb.sentStatus != 0 {
        t.Errorf("sentStatus: got %d, want 0 (no SendLocalReply at 0%%)", dcb.sentStatus)
    }
}

func TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName(t *testing.T) {
    headers := []*routev3.HeaderMatcher{
        {Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
    }
    fl, dcb := makeFilter(t, 503, 100, headers)
    h := http.Header{}
    h.Set("X-FAULT-ON", "yes") // uppercase name
    status := fl.DecodeHeaders(h, true)
    if status != envoyhttp.StopIteration {
        t.Errorf("status: got %v, want StopIteration (case-insensitive name match)", status)
    }
    if dcb.sentStatus != 503 {
        t.Errorf("sentStatus: got %d, want 503", dcb.sentStatus)
    }
}

func TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue(t *testing.T) {
    headers := []*routev3.HeaderMatcher{
        {Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
    }
    fl, dcb := makeFilter(t, 503, 100, headers)
    h := http.Header{}
    h.Set("x-fault-on", "YES") // uppercase value — should NOT match
    status := fl.DecodeHeaders(h, true)
    if status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue (case-sensitive value mismatch)", status)
    }
    if dcb.sentStatus != 0 {
        t.Errorf("sentStatus: got %d, want 0 (no fault on value mismatch)", dcb.sentStatus)
    }
}

func TestDecodeHeaders_NoFaultHeaderMismatch(t *testing.T) {
    headers := []*routev3.HeaderMatcher{
        {Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
    }
    fl, dcb := makeFilter(t, 503, 100, headers)
    status := fl.DecodeHeaders(http.Header{}, true) // empty headers → no match
    if status != envoyhttp.Continue {
        t.Errorf("status: got %v, want Continue", status)
    }
    if dcb.sentStatus != 0 {
        t.Errorf("sentStatus: got %d, want 0 (no fault when headers absent)", dcb.sentStatus)
    }
}

func TestDecodeHeaders_AbortStatRecorded(t *testing.T) {
    reg := stats.NewRegistry()
    f := &faultv3.HTTPFault{
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
    if err != nil { t.Fatalf("New: %v", err) }
    inst := factory()
    fl := inst.Decoder.(*filter)
    fl.SetDecoderCallbacks(&recordingDCB{})
    fl.DecodeHeaders(http.Header{}, true)
    // Walk to find the aborts_injected counter.
    var got int64
    reg.Walk(func(m stats.Metric) {
        if m.Name() == "http.ingress_http.fault.aborts_injected" {
            got, _ = strconv.ParseInt(m.Format(), 10, 64)
        }
    })
    if got != 1 {
        t.Errorf("aborts_injected: got %d, want 1", got)
    }
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/http/fault/ -run 'TestDecodeHeaders_AbortOnly|TestDecodeHeaders_HeadersFieldExactMatch|TestDecodeHeaders_NoFaultHeaderMismatch|TestDecodeHeaders_AbortStatRecorded' -v 2>&1 | head -30
```

Expected: tests FAIL because DecodeHeaders is still the Task-3 stub (returns Continue unconditionally).

- [ ] **Step 3: Replace `DecodeHeaders` body in `internal/filter/http/fault/fault.go`**

Replace the Task-3 stub with the abort-only path body + helpers. Per SPEC §6.4 (the delay path is added in Task 5; the per-route resolution is added in Task 7; max_active_faults Inc/Dec is added in Task 6; the markedActive guard is fully wired in Task 6 — Task 4 plumbs the recordFaultEvent helper but the Inc/Dec/markedActive bookkeeping is stubbed):

```go
// DecodeHeaders implements the fault filter's decode-side discipline per SPEC §6.4.
// Task 4 lands the abort-only path + headers gate + percentage roll. Tasks 5/6/7
// fill in delay async-resume, max_active_faults Inc/Dec, and per-route 3-tier merge.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
    cfg := f.cfg // Task 7 replaces with f.routeConfigOrListener()
    if !f.matchesHeaders(headers, cfg) {
        return envoyhttp.Continue
    }
    delayApplies := cfg.delayEnabled && f.rollPercent(cfg.delayPercentage)
    abortApplies := cfg.abortEnabled && f.rollPercent(cfg.abortPercentage)
    if !delayApplies && !abortApplies {
        return envoyhttp.Continue
    }
    // Task 6 inserts max_active_faults cap check here:
    //   if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults {
    //       f.recordFaultEvent(eventFaultsOverflow)
    //       return envoyhttp.Continue
    //   }
    //   f.markActive()
    if delayApplies && abortApplies {
        // Combined path lands in Task 5.
        return envoyhttp.Continue // placeholder; Task 5 replaces
    }
    if delayApplies {
        // Delay-only path lands in Task 5.
        return envoyhttp.Continue // placeholder; Task 5 replaces
    }
    // Abort-only path (Task 4 scope).
    f.recordFaultEvent(eventAbortsInjected)
    f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{
        {Name: "Content-Type", Value: "text/plain"},
    })
    return envoyhttp.StopIteration
}

// matchesHeaders returns true if cfg.matchHeaders is empty (match-all) OR
// every (canonical-name, exactValue) pair has a matching request-header
// VALUE under case-sensitive byte-equality (per §11.8 conclusion (a)+(b)).
// Header NAMES match case-insensitively via http.CanonicalHeaderKey.
func (f *filter) matchesHeaders(headers http.Header, cfg *runtimeConfig) bool {
    if len(cfg.matchHeaders) == 0 {
        return true
    }
    for _, hm := range cfg.matchHeaders {
        if headers.Get(hm.name) != hm.exactValue {
            return false
        }
    }
    return true
}

// rollPercent returns true iff a fresh random sample falls under p (in [0, 100]).
// Per planner-time decision 12: 0 short-circuits to false; 100 short-circuits
// to true; intermediate values consult the per-instance *rand.Rand seeded by
// time.Now().UnixNano() at filter-instance allocation time.
func (f *filter) rollPercent(p float64) bool {
    if p <= 0 {
        return false
    }
    if p >= 100 {
        return true
    }
    return f.rng.Float64()*100 < p
}

// faultEventKind enumerates stat-event kinds for recordFaultEvent.
type faultEventKind int

const (
    eventAbortsInjected    faultEventKind = iota
    eventDelaysInjected
    eventFaultsOverflow
    eventActiveFaultsInc
    eventActiveFaultsDec
)

// recordFaultEvent dispatches the stat-counter Inc/Dec per planner-time decision 3
// (consolidated stat-call-site). Tolerates nil stats (test code per ADR-0085).
func (f *filter) recordFaultEvent(k faultEventKind) {
    if f.stats == nil {
        return
    }
    switch k {
    case eventAbortsInjected:
        if f.stats.abortsInjected != nil { f.stats.abortsInjected.Inc() }
    case eventDelaysInjected:
        if f.stats.delaysInjected != nil { f.stats.delaysInjected.Inc() }
    case eventFaultsOverflow:
        if f.stats.faultsOverflow != nil { f.stats.faultsOverflow.Inc() }
    case eventActiveFaultsInc:
        if f.stats.activeFaults != nil { f.stats.activeFaults.Inc() }
    case eventActiveFaultsDec:
        if f.stats.activeFaults != nil { f.stats.activeFaults.Dec() }
    }
}

// decrementActive is the markedActive-guarded per-instance Inc/Dec balance helper.
// Task 6 wires the markedActive bool + the Inc/Dec calls; Task 4 lands the
// helper as a no-op-stub so the abort path can call it without sequencing
// concerns.
func (f *filter) decrementActive() {
    if f.markedActive {
        f.markedActive = false
        f.active.Add(-1)
        f.recordFaultEvent(eventActiveFaultsDec)
    }
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test ./internal/filter/http/fault/ -v
```

Expected: all Task-3 tests + new Task-4 abort/headers/percentage tests PASS.

- [ ] **Step 5: Verify race + lint**

```bash
go test -race -count=1 ./internal/filter/http/fault/...
go vet ./...
golangci-lint run ./internal/filter/http/fault/...
```

Expected: clean.

- [ ] **Step 6: Append ADR-0103 to `docs/envoy-go/DECISIONS.md`**

ADR-0103 covers: abort terminal-replace mechanics; body byte-exact "fault filter abort" (18 bytes, no newline); 4-header-set on the wire (with no charset modifier — distinct from admin endpoints' 6-header-set); OrderedHeaders carrier discipline (Content-Type override per ADR-0075's SendLocalReply ordered-headers contract); status-text allow-list for non-stdlib codes per planner-time decision 7. Lands-in-task: Task 4.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/fault/fault.go internal/filter/http/fault/fault_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 09: fault DecodeHeaders abort path + headers gate + percentage [ADR-0103]"
```

SHA-fill follow-up.

*Anchored: SPEC §5.3 (abort-only flow), §6.4 (DecodeHeaders body), §6.6 (SendLocalReply OrderedHeaders carrier), §11.3 (4-header set + body byte-exact), §11.4 (body byte-dump), §11.8 (headers-field exact-match semantics), §14.1 (unit-test list); ADR-0103 (abort terminal-replace + 4-header set + status-text allow-list); ADR-0075 (SendLocalReply enters encode chain at filter[len-1]); ADR-0072 (factory-validates-typed_config); ADR-0085 (nil-tolerance).*

---

## Task 5: `fault.go` delay async-resume + combined delay+abort timer-callback path [ADR-0102]

**Files:**
- Modify: `internal/filter/http/fault/fault.go` (DecodeHeaders delay-only and combined paths; time.AfterFunc wiring)
- Modify: `internal/filter/http/fault/fault_test.go` (delay/combined/timing tests)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0102)

This task lands the delay-only and combined delay+abort paths via `time.AfterFunc` per SPEC §5.2 + §5.4 + §11.2 + §11.3 (timing samples). The delay-only path schedules a callback that calls `cb.ContinueDecoding()` after the configured delay; the combined path schedules a callback that calls `cb.SendLocalReply()` instead. The chain parks at StopIteration per ADR-0071 + ADR-0075. The timing-tolerance contract (±10ms per §11.2 conclusion (c)) lands here. **ADR-0102** lands here.

**Precondition:** Task 4 done; abort-only path working.
**Artifact:** modified fault.go (delay + combined paths); modified fault_test.go (delay/combined/timing tests); ADR-0102 in DECISIONS.md.
**Acceptance:** `go test ./internal/filter/http/fault/... -v` passes the new delay/combined/timing tests; `go test -race ./internal/filter/http/fault/...` clean.

- [ ] **Step 1: Write failing tests for delay-only / combined / timing**

```go
func TestDecodeHeaders_DelayOnly(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(50 * time.Millisecond)},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    inst := factory()
    fl := inst.Decoder.(*filter)
    dcb := &recordingDCB{}
    fl.SetDecoderCallbacks(dcb)
    start := time.Now()
    status := fl.DecodeHeaders(http.Header{}, true)
    if status != envoyhttp.StopIteration {
        t.Errorf("status: got %v, want StopIteration", status)
    }
    // Wait for timer to fire (with generous bound for CI flake).
    deadline := time.After(500 * time.Millisecond)
    for dcb.continued.Load() == 0 {
        select {
        case <-deadline:
            t.Fatalf("ContinueDecoding never called; dcb=%+v", dcb)
        case <-time.After(2 * time.Millisecond):
        }
    }
    elapsed := time.Since(start)
    if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
        t.Errorf("elapsed: got %v, want ~50ms (within tolerance)", elapsed)
    }
    if dcb.sentStatus != 0 {
        t.Errorf("delay-only should NOT call SendLocalReply; got status=%d", dcb.sentStatus)
    }
}

func TestDecodeHeaders_Combined(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(50 * time.Millisecond)},
        },
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    inst := factory()
    fl := inst.Decoder.(*filter)
    dcb := &recordingDCB{}
    fl.SetDecoderCallbacks(dcb)
    start := time.Now()
    status := fl.DecodeHeaders(http.Header{}, true)
    if status != envoyhttp.StopIteration {
        t.Errorf("status: got %v, want StopIteration", status)
    }
    // Wait for timer to fire + SendLocalReply to land.
    deadline := time.After(500 * time.Millisecond)
    for dcb.sentStatus == 0 {
        select {
        case <-deadline:
            t.Fatalf("SendLocalReply never called; dcb=%+v", dcb)
        case <-time.After(2 * time.Millisecond):
        }
    }
    elapsed := time.Since(start)
    if elapsed < 40*time.Millisecond {
        t.Errorf("elapsed: got %v, want >= 40ms (delay should fire first)", elapsed)
    }
    if dcb.sentStatus != 503 {
        t.Errorf("sentStatus: got %d, want 503", dcb.sentStatus)
    }
    if dcb.sentBody != "fault filter abort" {
        t.Errorf("sentBody: got %q, want %q", dcb.sentBody, "fault filter abort")
    }
    if dcb.continued.Load() != 0 {
        t.Errorf("ContinueDecoding should NOT be called in combined path; got %d", dcb.continued.Load())
    }
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/http/fault/ -run 'TestDecodeHeaders_DelayOnly|TestDecodeHeaders_Combined' -v 2>&1 | head -30
```

Expected: tests FAIL because the delay-only and combined path bodies are still the Task-4 placeholders (return Continue).

- [ ] **Step 3: Replace the delay-only and combined placeholders in `DecodeHeaders`**

Replace the two `return envoyhttp.Continue // placeholder; Task 5 replaces` lines with the timer-driven paths. Final shape per SPEC §5.2 + §5.4:

```go
    if delayApplies && abortApplies {
        // Combined: timer fires; callback calls SendLocalReply (NOT ContinueDecoding).
        f.recordFaultEvent(eventDelaysInjected)
        f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
            f.recordFaultEvent(eventAbortsInjected)
            f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, envoyhttp.OrderedHeaders{
                {Name: "Content-Type", Value: "text/plain"},
            })
            f.decrementActive() // Task 6 wires markedActive guard
        })
        return envoyhttp.StopIteration
    }
    if delayApplies {
        // Delay-only: timer fires; callback calls ContinueDecoding.
        f.recordFaultEvent(eventDelaysInjected)
        f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() {
            f.dcb.ContinueDecoding()
            f.decrementActive() // Task 6 wires markedActive guard
        })
        return envoyhttp.StopIteration
    }
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test ./internal/filter/http/fault/ -v
```

Expected: all tests (Tasks 3, 4, 5) PASS.

- [ ] **Step 5: Verify race**

```bash
go test -race -count=1 ./internal/filter/http/fault/...
```

Expected: clean.

- [ ] **Step 6: Append ADR-0102 to DECISIONS.md**

ADR-0102 covers: time.AfterFunc-driven async-resume; cb.ContinueDecoding from timer goroutine; combined delay+abort via timer-callback decision (timer fires; callback calls SendLocalReply, NOT ContinueDecoding); cancel-on-OnDestroy mechanics (anchored at Task 6); ±10ms timing tolerance per §11.2 conclusion (c). Lands-in-task: Task 5.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/fault/fault.go internal/filter/http/fault/fault_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 09: fault DecodeHeaders delay async-resume + combined path [ADR-0102]"
```

SHA-fill follow-up.

*Anchored: SPEC §5.2 (delay-only flow), §5.4 (combined flow), §6.4 (DecodeHeaders body), §11.2 (timing samples + ±10ms tolerance), §11.3 (combined ordering 100ms+1.7ms overhead), §14.1 (unit-test list); ADR-0102 (delay async-resume + combined-path timer-callback decision); ADR-0071 (chain park-on-StopIteration + ContinueDecoding semantics); ADR-0075 (SendLocalReply chain entry).*

---

## Task 6: `fault.go` max_active_faults atomic counter + markedActive Inc/Dec idempotency guard + OnDestroy timer cancel + race-detector cycle test [ADR-0105]

**Files:**
- Modify: `internal/filter/http/fault/fault.go` (max_active_faults cap check; markActive helper; OnDestroy body)
- Modify: `internal/filter/http/fault/fault_test.go` (max-active overflow tests + race-detector cycle test per planner-time decision 10)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0105)

This task lands the LBP-1 sixth application: `*atomic.Int64` activeFaults counter (closure-captured at New time per Task 3; shared across per-instance *filter values from the same factory). Per SPEC §5.6 + §5.7 + §6.4: when `cfg.maxActiveFaults > 0 && f.active.Load() >= cap`, the fault is SKIPPED (not injected; `fault.faults_overflow` Inc; return Continue). When the fault IS injected, `markActive()` Inc's the counter + sets the per-instance `markedActive bool` (sync.Once-equivalent guard). The `decrementActive()` helper from Task 4 (currently a no-op when markedActive is false) becomes operative. OnDestroy calls `f.delayTimer.Stop()` (if scheduled) + `decrementActive()` (idempotent via markedActive). Per planner-time decision 10: a `TestFault_DelayTimerRace` cycle test under -race validates the markedActive guard against the OnDestroy-races-timer-callback case. **ADR-0105** lands here.

**Precondition:** Tasks 4 + 5 done; abort + delay + combined paths working.
**Artifact:** modified fault.go (cap check + markActive + OnDestroy); modified fault_test.go (overflow + race tests); ADR-0105 in DECISIONS.md.
**Acceptance:** `go test ./internal/filter/http/fault/... -v` passes new overflow + race tests; `go test -race -count=10 ./internal/filter/http/fault/...` clean (multiple iterations to surface race-detector flakes).

- [ ] **Step 1: Write failing tests**

```go
func TestDecodeHeaders_MaxActiveFaultsCapOverflow(t *testing.T) {
    reg := stats.NewRegistry()
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(200 * time.Millisecond)},
        },
        MaxActiveFaults: wrapperspb.UInt32(1),
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    // Allocate two instances from the same factory (sharing the activeFaults counter).
    fl1 := factory().Decoder.(*filter); fl1.SetDecoderCallbacks(&recordingDCB{})
    fl2 := factory().Decoder.(*filter); fl2.SetDecoderCallbacks(&recordingDCB{})

    s1 := fl1.DecodeHeaders(http.Header{}, true)
    if s1 != envoyhttp.StopIteration {
        t.Errorf("first request: got %v, want StopIteration (fault should fire)", s1)
    }
    s2 := fl2.DecodeHeaders(http.Header{}, true)
    if s2 != envoyhttp.Continue {
        t.Errorf("second request: got %v, want Continue (cap should skip fault)", s2)
    }
    var overflow int64
    reg.Walk(func(m stats.Metric) {
        if m.Name() == "http.x.fault.faults_overflow" {
            overflow, _ = strconv.ParseInt(m.Format(), 10, 64)
        }
    })
    if overflow != 1 {
        t.Errorf("faults_overflow: got %d, want 1", overflow)
    }
}

func TestOnDestroy_TimerStopped(t *testing.T) {
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(500 * time.Millisecond)},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    fl := factory().Decoder.(*filter)
    dcb := &recordingDCB{}
    fl.SetDecoderCallbacks(dcb)
    fl.DecodeHeaders(http.Header{}, true)
    fl.OnDestroy() // should cancel the timer before it fires
    time.Sleep(100 * time.Millisecond) // briefly wait; timer should NOT have fired
    if dcb.continued.Load() != 0 {
        t.Errorf("timer fired despite OnDestroy; ContinueDecoding called %d times", dcb.continued.Load())
    }
    // active counter should be balanced (Inc'd on DecodeHeaders, Dec'd on OnDestroy).
    if got := fl.active.Load(); got != 0 {
        t.Errorf("active counter: got %d, want 0", got)
    }
}

func TestFault_DelayTimerRace(t *testing.T) {
    // Per planner-time decision 10: race-detector cycle test for OnDestroy-races-
    // timer-callback. Many iterations under -race to surface flakes.
    if testing.Short() {
        t.Skip("race-cycle test skipped under -short")
    }
    f := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(1 * time.Millisecond)},
        },
    }
    factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    for i := 0; i < 100; i++ {
        fl := factory().Decoder.(*filter)
        fl.SetDecoderCallbacks(&recordingDCB{})
        fl.DecodeHeaders(http.Header{}, true)
        // Race OnDestroy against the timer firing.
        time.Sleep(time.Duration(i%2) * time.Millisecond) // 0 or 1ms — straddles the 1ms timer
        fl.OnDestroy()
    }
}
```

- [ ] **Step 2: Run tests; confirm they fail**

```bash
go test ./internal/filter/http/fault/ -run 'TestDecodeHeaders_MaxActiveFaultsCapOverflow|TestOnDestroy_TimerStopped|TestFault_DelayTimerRace' -v 2>&1 | head -30
```

Expected: TestDecodeHeaders_MaxActiveFaultsCapOverflow fails (no cap check); TestOnDestroy_TimerStopped fails (OnDestroy is no-op stub); TestFault_DelayTimerRace fails or races (no markedActive guard).

- [ ] **Step 3: Add max_active_faults cap check + markActive helper + OnDestroy body in `internal/filter/http/fault/fault.go`**

Insert the cap check in `DecodeHeaders` after the no-fault short-circuit and before the fault-injection paths:

```go
    // (after delayApplies/abortApplies check, before fault-path branches)
    if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults {
        f.recordFaultEvent(eventFaultsOverflow)
        return envoyhttp.Continue
    }
    f.markActive()
```

Add the `markActive` helper:

```go
// markActive Inc's the activeFaults counter and sets the markedActive flag.
// The flag is the sync.Once-equivalent guard ensuring decrementActive Dec's
// exactly once (race-clean by single-goroutine-per-stream invariant per
// ADR-0071: read-modify-write of markedActive within an instance is race-free).
func (f *filter) markActive() {
    f.active.Add(1)
    f.markedActive = true
    f.recordFaultEvent(eventActiveFaultsInc)
}
```

Replace the OnDestroy body:

```go
// OnDestroy cancels any pending delay timer and decrements activeFaults if
// the fault was marked active and not already decremented (per ADR-0105
// markedActive idempotency guard). Called by the chain teardown path
// (request completion or downstream-disconnect-induced reset).
func (f *filter) OnDestroy() {
    if f.delayTimer != nil {
        _ = f.delayTimer.Stop() // best-effort; ignore return; markedActive guard handles double-Dec
    }
    f.decrementActive()
}
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test ./internal/filter/http/fault/ -v
```

Expected: all tests (Tasks 3–6) PASS.

- [ ] **Step 5: Verify race-detector cleanliness across multiple iterations**

```bash
go test -race -count=10 ./internal/filter/http/fault/...
```

Expected: clean across all 10 iterations (no race detector flags).

- [ ] **Step 6: Append ADR-0105 to DECISIONS.md**

ADR-0105 covers: max_active_faults concurrency cap; LBP-1 sixth application; *atomic.Int64 closure-captured counter shared across per-instance values; markedActive per-instance bool sync.Once-equivalent guard; race-clean by single-goroutine-per-stream invariant per ADR-0071; OnDestroy timer-cancel discipline; faults_overflow stat semantics. Lands-in-task: Task 6.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/fault/fault.go internal/filter/http/fault/fault_test.go docs/envoy-go/DECISIONS.md
git commit -m "phase 09: fault max_active_faults cap + markedActive guard + OnDestroy [ADR-0105]"
```

SHA-fill follow-up.

*Anchored: SPEC §5.6 (max_active_faults overflow flow), §5.7 (concurrency model + markedActive guard), §6.4 (DecodeHeaders cap check), §14.1 (unit-test list); ADR-0105 (max_active_faults + LBP-1 sixth + markedActive); ADR-0071 (single-goroutine-per-stream invariant); ADR-0072/0079/0061/0091/0078 (LBP-1 first through fifth applications cross-referenced).*

---

## Task 7: `fault.go` per-route 3-tier merge (routeConfigOrListener + parseRouteRuntimeConfig) + tests

**Files:**
- Modify: `internal/filter/http/fault/fault.go` (routeConfigOrListener + parseRouteRuntimeConfig; replace `cfg := f.cfg` in DecodeHeaders with `cfg := f.routeConfigOrListener()`)
- Modify: `internal/filter/http/fault/fault_test.go` (per-route wholesale-override test)

This task lands the per-route 3-tier merge resolution per ADR-0073 + SPEC §6.5 + §11.7. When `cb.RequestRouteConfig()` returns a non-nil `*faultv3.HTTPFault` proto, `parseRouteRuntimeConfig` projects it to a fresh `*runtimeConfig` (wholesale-override per §11.7 — a per-route HTTPFault that omits delay does NOT inherit the listener-level delay). NO new ADR — reuses ADR-0073's existing 3-tier-merge contract; the cross-reference is recorded in ADR-0101 §Consequences (per the inline-cross-references list above).

**Precondition:** Task 6 done; max-active + markedActive working.
**Artifact:** modified fault.go (per-route helpers); modified fault_test.go (wholesale-override test); no ADR landed.
**Acceptance:** `go test ./internal/filter/http/fault/... -v` passes the new TestPerRouteWholesaleOverride; existing tests still pass; `go test -race ./internal/filter/http/fault/...` clean.

- [ ] **Step 1: Write failing test**

```go
func TestPerRouteWholesaleOverride(t *testing.T) {
    // Listener-level: delay 100% 200ms only.
    listenerCfg := &faultv3.HTTPFault{
        Delay: &commonfaultv3.FaultDelay{
            Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(200 * time.Millisecond)},
        },
    }
    // Per-route: abort 100% 418, NO delay (wholesale-override should drop the listener delay).
    routeCfg := &faultv3.HTTPFault{
        Abort: &faultv3.FaultAbort{
            Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
            ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 418},
        },
    }
    factory, err := New(mustAny(t, listenerCfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
    if err != nil { t.Fatalf("New: %v", err) }
    fl := factory().Decoder.(*filter)
    dcb := &recordingDCB{routeCfg: routeCfg}
    fl.SetDecoderCallbacks(dcb)
    start := time.Now()
    status := fl.DecodeHeaders(http.Header{}, true)
    elapsed := time.Since(start)
    if status != envoyhttp.StopIteration {
        t.Errorf("status: got %v, want StopIteration (per-route abort should fire)", status)
    }
    if dcb.sentStatus != 418 {
        t.Errorf("sentStatus: got %d, want 418 (per-route override)", dcb.sentStatus)
    }
    if elapsed > 50*time.Millisecond {
        t.Errorf("elapsed: got %v, want <50ms (no inherited delay — wholesale-override)", elapsed)
    }
}
```

- [ ] **Step 2: Run test; confirm it fails**

```bash
go test ./internal/filter/http/fault/ -run TestPerRouteWholesaleOverride -v
```

Expected: test fails (currently DecodeHeaders uses `f.cfg` listener-level only; route config ignored; the test sees 200ms delay then 418 instead of immediate 418).

- [ ] **Step 3: Add `routeConfigOrListener` + `parseRouteRuntimeConfig` helpers + replace `cfg := f.cfg` in DecodeHeaders**

Add helpers:

```go
// routeConfigOrListener resolves the per-route runtimeConfig if a per-route
// HTTPFault is present (wholesale-override per §11.7), else returns the
// listener-level config from the factory closure. Per ADR-0073 + SPEC §6.5.
func (f *filter) routeConfigOrListener() *runtimeConfig {
    if f.dcb == nil {
        return f.cfg
    }
    raw := f.dcb.RequestRouteConfig()
    if raw == nil {
        return f.cfg
    }
    routeProto, ok := raw.(*faultv3.HTTPFault)
    if !ok {
        return f.cfg // defensive; shouldn't happen if perroute parses anypb correctly
    }
    rc, err := parseRouteRuntimeConfig(routeProto)
    if err != nil {
        // Per-route config is invalid — silently fall through to listener config.
        // The boot-time factory would have rejected the listener config; per-route
        // configs are validated at HCM-build time too, so reaching this branch
        // means a runtime parse race or a defensively-malformed test fixture.
        return f.cfg
    }
    return rc
}

// parseRouteRuntimeConfig is the per-route projection of HTTPFault into
// runtimeConfig. Per planner-time decision 2 (KEEP separate): kept as its own
// function from parseRuntimeConfig so the New-time-only validations (the
// `tc != nil` guard upstream of parseRuntimeConfig in New) stay distinct from
// per-route-resolve-time validation. Today the body delegates to
// parseRuntimeConfig directly because the SPEC §11.7 wholesale-override
// discipline applies the same field validation to both inputs; if a future
// per-route deferral diverges (e.g., a per-route-only field set), this
// function diverges without affecting parseRuntimeConfig's New-time semantics.
func parseRouteRuntimeConfig(c *faultv3.HTTPFault) (*runtimeConfig, error) {
    return parseRuntimeConfig(c)
}
```

Replace the line `cfg := f.cfg // Task 7 replaces with f.routeConfigOrListener()` in DecodeHeaders with:

```go
    cfg := f.routeConfigOrListener()
```

- [ ] **Step 4: Run tests; confirm they pass**

```bash
go test ./internal/filter/http/fault/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Verify race**

```bash
go test -race -count=1 ./internal/filter/http/fault/...
```

Expected: clean.

- [ ] **Step 6: Commit**

No new ADR; cross-reference to ADR-0073 is recorded in ADR-0101 §Consequences (already present from Task 3).

```bash
git add internal/filter/http/fault/fault.go internal/filter/http/fault/fault_test.go
git commit -m "phase 09: fault per-route 3-tier merge wholesale-override (per ADR-0073 + §11.7)"
```

SHA-fill follow-up.

*Anchored: SPEC §6.5 (per-route 3-tier merge), §11.7 (wholesale-override empirical pin), §14.1 (TestPerRouteWholesaleOverride); ADR-0073 (existing 3-tier merge contract); ADR-0101 §Consequences cross-reference (no new ADR).*

---

## Task 8: `cmd/envoy-go/main.go` register fault.New under fault.TypeURL

**Files:**
- Modify: `cmd/envoy-go/main.go`

This task lands the one-line registration that wires the fault filter into the boot-time HTTP filter registry per BRAINSTORM Decision 2 + SPEC §4.2. After this commit, any HCM with `envoy.filters.http.fault` in `http_filters` resolves the typed_config to fault.New + invokes parseRuntimeConfig + (when valid) injects the filter into the chain. No ADR landed (ADR-0100 already covers the boot registration; landed at Task 3).

**Precondition:** Task 7 done; fault filter complete.
**Artifact:** modified main.go (one import + one Register line).
**Acceptance:** `go build ./...` clean; `cmd/envoy-go/main.go` boots a bootstrap with `envoy.filters.http.fault` in http_filters without error.

- [ ] **Step 1: Inspect current `cmd/envoy-go/main.go` registry block**

```bash
grep -nE 'httpReg|cors|envoygotest|router|fault' cmd/envoy-go/main.go | head -10
```

Expected: lines ~28–30 (imports for cors/envoygotest/router) + lines ~111–114 (httpReg.Register block + Freeze).

- [ ] **Step 2: Add fault import alphabetically**

In the import block (currently lines ~28–30), insert:

```go
"github.com/esalaine/envoy-go/internal/filter/http/fault"
```

Alphabetically between `cors` and `router`. Final order: cors, envoygotest, fault, router.

- [ ] **Step 3: Add registration line**

In the registry block (currently lines ~111–114), insert AFTER `httpReg.Register(envoygotest.TypeURL, envoygotest.New)` and BEFORE `httpReg.Freeze()`:

```go
httpReg.Register(fault.TypeURL, fault.New)
```

Final block reads:
```go
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(cors.TypeURL, cors.New)
httpReg.Register(envoygotest.TypeURL, envoygotest.New)
httpReg.Register(fault.TypeURL, fault.New)
httpReg.Freeze()
```

(Per BRAINSTORM Decision 2's "router-first-then-alphabetical" stylistic discipline.)

- [ ] **Step 4: Verify build**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
```

Expected: clean.

- [ ] **Step 5: Smoke test — boot envoy-go against a minimal bootstrap with fault in chain**

Create a temp file `/tmp/envoy-go-fault-smoke.yaml` with a minimal bootstrap (one listener, fault + router in http_filters, abort 100% 503). Run:

```bash
go run ./cmd/envoy-go --config-path /tmp/envoy-go-fault-smoke.yaml &
sleep 1
curl -isS http://127.0.0.1:10000/ | head -10
kill %1
```

Expected: response is `HTTP/1.1 503 Service Unavailable` with body `fault filter abort` (verifying end-to-end wiring through main.go → registry → fault.New → DecodeHeaders → SendLocalReply). If the smoke test fails, the bug is in the boot wiring — investigate before proceeding.

- [ ] **Step 6: Commit**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 09: cmd/envoy-go/main.go register fault.New (alphabetical insert)"
```

SHA-fill follow-up.

*Anchored: SPEC §4.2 (registration delta), BRAINSTORM Decision 2 (router-first-then-alphabetical); ADR-0100 §Decision (boot registration line) — no new ADR.*

---

## Task 9: `internal/filter/http/fault/fuzz_test.go` `FuzzFaultConfigParse`

**Files:**
- Create: `internal/filter/http/fault/fuzz_test.go`

This task lands the twelfth fuzzer per planner-time decision 1 + ADR-0018's "every parser/codec/filter ships a fuzzer". Fault's `New` factory is the parser; the fuzzer feeds arbitrary byte sequences as the `tc *anypb.Any` parameter and asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. ~50 LoC; 30s budget per ADR-0018 short-mode CI.

**Precondition:** Task 8 done.
**Artifact:** new fuzz_test.go.
**Acceptance:** `go test -fuzz=FuzzFaultConfigParse -fuzztime=30s ./internal/filter/http/fault/` runs clean (no panics, no `(nil, nil)` returns); the fuzzer's seed corpus covers the SPEC §11.1 PGV-validation cases (status=0/9999/100/600).

- [ ] **Step 1: Write `internal/filter/http/fault/fuzz_test.go`**

```go
package fault

import (
    "testing"

    "google.golang.org/protobuf/types/known/anypb"

    envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
    "github.com/esalaine/envoy-go/internal/stats"
)

// FuzzFaultConfigParse fuzzes the New factory's typed_config parameter against
// arbitrary byte sequences. Per ADR-0018: every parser/codec/filter ships a
// fuzzer. Fault's New is the parser. Asserts:
//   - New returns either (factory, nil) OR (nil, error); never panics.
//   - Never returns (nil, nil).
//
// Seed corpus per SPEC §11.1 PGV-validation cases:
//   - empty Any (TypeURL = "", Value = nil)
//   - Any with wrong TypeURL but valid Value bytes
//   - Any with right TypeURL but garbage Value
//   - HTTPFault with abort.http_status = 0 / 9999 / 100 / 599 / 600
func FuzzFaultConfigParse(f *testing.F) {
    seeds := [][]byte{
        nil,
        {},
        {0x00},
        {0xff, 0xff, 0xff, 0xff},
        []byte("not-a-proto"),
    }
    for _, s := range seeds {
        f.Add(s)
    }
    f.Fuzz(func(t *testing.T, b []byte) {
        tc := &anypb.Any{TypeUrl: TypeURL, Value: b}
        ctx := envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"}
        factory, err := New(tc, ctx)
        if factory == nil && err == nil {
            t.Fatalf("New returned (nil, nil) for input %x — must return either factory or error", b)
        }
        if factory != nil && err != nil {
            t.Fatalf("New returned both factory and error for input %x — exclusive", b)
        }
    })
}
```

- [ ] **Step 2: Run the fuzzer for the 30s budget**

```bash
go test -fuzz=FuzzFaultConfigParse -fuzztime=30s ./internal/filter/http/fault/
```

Expected: completes without finding any new failing input; no panics; no `(nil, nil)` returns. If the fuzzer DOES find a failing input, examine `testdata/fuzz/FuzzFaultConfigParse/<id>` for the seed and fix the underlying bug (likely a missing nil-guard or unhandled validation edge case).

- [ ] **Step 3: Verify the fuzzer also runs as a normal test (seed corpus only) under -short**

```bash
go test -count=1 -short ./internal/filter/http/fault/
```

Expected: PASS (the seed corpus runs as part of the normal test suite per `go test -fuzz` semantics).

- [ ] **Step 4: Commit**

```bash
git add internal/filter/http/fault/fuzz_test.go
git commit -m "phase 09: FuzzFaultConfigParse — twelfth fuzzer per ADR-0018"
```

SHA-fill follow-up.

*Anchored: SPEC §14.5 (fuzzer ship), planner-time decision 1 (SHIP), ADR-0018 (fuzz CI 30s short-budget policy); ADR-0072 (factory-validates-typed_config contract).*

---

## Task 10: Fixture infrastructure — `BackendKind` enum extension + `runner_test.go` spawn helper + blank-import

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add `HTTPFault BackendKind = 8`)
- Modify: `test/differential/runner_test.go` (add `startHTTPFaultBackend` helper + new switch case + blank-import)

This task lands the runner-side scaffolding that fixture 0011-http-fault depends on. Per planner-time decision 13: new BackendKind enum value `HTTPFault BackendKind = 8` (continues the existing convention; suffix names the fixture-purpose). The `startHTTPFaultBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` helper mirrors the existing `startHTTPSlowStreamBackend` pattern (lines 781–793 in master HEAD): `go run ./test/fixtures/0011-http-fault/backends --port <port>` + Setpgid + Stdout/Stderr to os.Stderr. The blank-import for the fixture driver lands here so the fixture is registered with the runner; the actual driver is created in Task 14 (a fixture-discoverable directory must exist at this point — Task 11 creates the backends/ subdir; Task 14 creates the driver/ subdir; the runner's `discoverFixtures` walks `test/fixtures/` for any `NNNN[a-z]?-name` pattern, so an empty/partial fixture directory is OK at intermediate Task-N states per the runner's `t.Skipf("no driver registered for fixture %q ...")` branch at lines 60–62).

**Precondition:** Task 9 done; fault filter complete; fault.New registered.
**Artifact:** modified fixture.go + runner_test.go.
**Acceptance:** `go build ./test/...` clean; `go test -count=1 -short ./test/differential/...` clean (the fixture's driver is not yet registered; the runner skips fixture 0011-http-fault with the documented `t.Skipf` log line until Task 14).

- [ ] **Step 1: Add `HTTPFault` to the `BackendKind` enum in `test/differential/fixture/fixture.go`**

After the existing `HTTPSlowStream BackendKind = 7` constant (currently lines 176–183), insert:

```go
// HTTPFault is an out-of-process HTTP/1.1 backend: the runner spawns
// test/fixtures/0011-http-fault/backends/backend.go on the pre-allocated
// port. The backend serves / with body "backend\n" (8 bytes). No TLS.
// Introduced by fixture 0011-http-fault (phase 09 Task 10) to provide the
// deterministic-body backend the per-scenario equivalence assertions
// expect. Because the backend is a subprocess, the runner's in-process
// accept counter is NOT incremented.
HTTPFault BackendKind = 8
```

- [ ] **Step 2: Add `startHTTPFaultBackend` helper + switch case in `test/differential/runner_test.go`**

(a) Add the spawn helper after `startHTTPSlowStreamBackend` (currently lines 781–793):

```go
// startHTTPFaultBackend spawns the fixture-0011 HTTP/1.1 backend subprocess on port.
// The backend serves / with body "backend\n" (8 bytes). No TLS. Introduced for
// fixture 0011-http-fault (phase 09 Task 10) for the per-scenario equivalence
// assertions. Because the backend is a subprocess, the runner's in-process
// accept counter is NOT incremented.
func startHTTPFaultBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
    cmd := exec.CommandContext(ctx, "go", "run", "./test/fixtures/0011-http-fault/backends",
        "--port", fmt.Sprintf("%d", port),
    )
    cmd.Dir = repoRoot
    cmd.Stdout = os.Stderr
    cmd.Stderr = os.Stderr
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start: %w", err)
    }
    return cmd, nil
}
```

(b) Add the switch case in `runFixture` (currently lines ~94–222 — find the existing `case fixture.HTTPSlowStream` block at lines 204–222 and add an analogous `case fixture.HTTPFault` block immediately after):

```go
case fixture.HTTPFault:
    port := freeTCPPort(t)
    bo.port = port
    cmd, err := startHTTPFaultBackend(ctx, root, port)
    if err != nil {
        t.Fatalf("backend[%d] start: %v", i, err)
    }
    bo.proc = cmd
    defer func(cmd *exec.Cmd) {
        if cmd.Process != nil {
            _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
        }
        _ = cmd.Process.Kill()
        _, _ = cmd.Process.Wait()
    }(cmd)
    if err := waitTCPDial(ctx, fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second); err != nil {
        t.Fatalf("backend[%d] not ready: %v", i, err)
    }
```

(c) Add the blank-import for the fixture driver. In the import block at lines 24–35, insert (alphabetically after the `0010-graceful-drain` import):

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver"
```

**Note:** the driver package is created in Task 14; until then, `go build` will fail. So the blank-import is added in Task 14 (NOT Task 10) — Task 10 only adds the BackendKind enum + spawn helper + switch case. Re-read this step's body: insert the blank-import in **Task 14**, not Task 10. The PLAN's File-structure table line at runner_test.go's "MODIFIED" row covers BOTH the spawn-helper-and-switch-case (Task 10) AND the blank-import (Task 14); they land in two separate commits.

Revised step 2(c) for Task 10: SKIP the blank-import; it lands in Task 14. Task 10 lands only the BackendKind enum + spawn helper + switch case.

- [ ] **Step 3: Verify build + short-mode tests**

```bash
go build ./...
go test -count=1 -short ./test/differential/...
```

Expected: clean. The runner's `discoverFixtures` does not yet see `test/fixtures/0011-http-fault/` (created in Tasks 11+); no skip log fires.

- [ ] **Step 4: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go
git commit -m "phase 09: BackendKind HTTPFault enum + startHTTPFaultBackend helper"
```

SHA-fill follow-up.

*Anchored: SPEC §4.3 (runner-side delta), planner-time decision 11 (path correction), planner-time decision 13 (BackendKind name); existing 0010-graceful-drain BackendKind precedent at master `b33e04f`.*

---

## Task 11: Fixture 0011 — `backends/backend.go` (Go HTTP backend serving `backend\n`)

**Files:**
- Create: `test/fixtures/0011-http-fault/backends/backend.go`

This task lands the minimal Go HTTP backend per SPEC §7.5 + planner-time decision 11. Single endpoint `/` serving `200 OK` with body `backend\n` (8 bytes; matches the §11 empirical-pin backend used during SPEC drafting). Accepts `--port` flag for the runner-allocated port; `package main` for `go run` invocation by the runner's spawn helper.

**Precondition:** Task 10 done.
**Artifact:** new backends/backend.go.
**Acceptance:** `go build ./test/fixtures/0011-http-fault/backends/...` clean; `go run ./test/fixtures/0011-http-fault/backends --port 18001` boots and serves `backend\n` on `/` (manual smoke test).

- [ ] **Step 1: Write `test/fixtures/0011-http-fault/backends/backend.go`**

```go
// Backend for fixture 0011-http-fault. Serves / with body "backend\n" (8 bytes).
// Matches the §11 empirical-pin backend used during phase 09 SPEC drafting.
package main

import (
    "flag"
    "fmt"
    "net/http"
)

func main() {
    port := flag.Int("port", 18001, "TCP port to bind")
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

- [ ] **Step 2: Verify build**

```bash
go build ./test/fixtures/0011-http-fault/backends/...
```

Expected: clean.

- [ ] **Step 3: Manual smoke test**

```bash
go run ./test/fixtures/0011-http-fault/backends --port 18001 &
sleep 1
curl -sS http://127.0.0.1:18001/
kill %1
```

Expected: response body is `backend\n` (8 bytes; trailing newline).

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0011-http-fault/backends/backend.go
git commit -m "phase 09: fixture 0011 backends/backend.go (serves backend\\n)"
```

SHA-fill follow-up.

*Anchored: SPEC §7.5 (backend shape), §11 (empirical-pin backend reference), planner-time decision 11 (path).*

---

## Task 12: Fixture 0011 — `envoy.yaml` + `envoy-go.yaml` bootstraps per SPEC §7.4

**Files:**
- Create: `test/fixtures/0011-http-fault/envoy.yaml`
- Create: `test/fixtures/0011-http-fault/envoy-go.yaml`

This task lands the dual-proxy bootstrap YAMLs per SPEC §7.4 verbatim (with port substitution per the runner's templating discipline). The reference Envoy bootstrap binds admin :9902 (in-container; published to a runner-allocated host port) and listener :10001 (in-container; published similarly). The envoy-go subject bootstrap uses runner-allocated host ports for admin and listener. Both bootstraps share the §7.4 verbatim listener config: listener-level fault `delay 100% 100ms` (no abort); per-route overrides on `/scenario2`, `/scenario3-wholesale`, `/scenario3-baseline`, `/scenario4`. The `c_backend` cluster is STRICT_DNS pointing at the harness backend hostname per planner-time decision 8 — for the reference container that's `host.docker.internal` per ADR-0010; for the envoy-go subject (host process), it's `127.0.0.1` (resolved by Go's net resolver). The driver in Task 14 renders both YAMLs with the runtime-substituted ports + backend hostname via Go templates.

**Precondition:** Task 11 done.
**Artifact:** two new bootstrap YAMLs (templated; runtime substitution happens in the driver).
**Acceptance:** `go build ./...` unaffected (YAMLs are not Go); `yamllint test/fixtures/0011-http-fault/*.yaml` (if available) passes; the YAML structure matches SPEC §7.4 verbatim modulo port substitution markers.

- [ ] **Step 1: Write `test/fixtures/0011-http-fault/envoy.yaml` (reference Envoy bootstrap)**

Per SPEC §7.4 verbatim. The driver's `ReferenceBootstrap(backendPorts []int)` Go-templates `{{.BackendHost}}` and `{{.BackendPort}}`:

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9902 }
static_resources:
  listeners:
    - name: l_main
      address:
        socket_address: { address: 0.0.0.0, port_value: 10001 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/scenario1" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario2" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              delay:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                fixed_delay: 0.1s
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                        - match: { prefix: "/scenario3-wholesale" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 418
                        - match: { prefix: "/scenario3-baseline" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario4" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                              headers:
                                - name: x-fault-on
                                  string_match: { exact: "yes" }
                http_filters:
                  - name: envoy.filters.http.fault
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                      delay:
                        percentage: { numerator: 100, denominator: HUNDRED }
                        fixed_delay: 0.1s
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
                    socket_address: { address: {{.BackendHost}}, port_value: {{.BackendPort}} }
```

- [ ] **Step 2: Write `test/fixtures/0011-http-fault/envoy-go.yaml` (envoy-go subject bootstrap)**

Identical to envoy.yaml modulo admin/listener ports. The driver's `SubjectConfig(refListenerPort, subjListenerPort, backendPorts, subjAdminPort)` Go-templates `{{.AdminPort}}`, `{{.ListenerPort}}`, `{{.BackendHost}}`, `{{.BackendPort}}`:

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} }
static_resources:
  listeners:
    - name: l_main
      address:
        socket_address: { address: 0.0.0.0, port_value: {{.ListenerPort}} }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/scenario1" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario2" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              delay:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                fixed_delay: 0.1s
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                        - match: { prefix: "/scenario3-wholesale" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 418
                        - match: { prefix: "/scenario3-baseline" }
                          route: { cluster: c_backend }
                        - match: { prefix: "/scenario4" }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.fault:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                              abort:
                                percentage: { numerator: 100, denominator: HUNDRED }
                                http_status: 503
                              headers:
                                - name: x-fault-on
                                  string_match: { exact: "yes" }
                http_filters:
                  - name: envoy.filters.http.fault
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault
                      delay:
                        percentage: { numerator: 100, denominator: HUNDRED }
                        fixed_delay: 0.1s
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
                    socket_address: { address: {{.BackendHost}}, port_value: {{.BackendPort}} }
```

(The HCM body is identical to Step 1's; copy it verbatim. The driver's render-time substitution is the only difference.)

- [ ] **Step 3: Verify YAML structural validity**

```bash
go run ./cmd/envoy-go --config-path test/fixtures/0011-http-fault/envoy-go.yaml &
# Note: this will FAIL because the {{.AdminPort}} templates are not yet
# substituted. The test is structural only — the parser should reject with
# a clear "address field invalid" error, not a YAML-syntax error.
```

Alternative: render the templates manually with sed and try a fresh boot:

```bash
sed -e 's/{{.AdminPort}}/9901/' -e 's/{{.ListenerPort}}/10000/' -e 's/{{.BackendHost}}/127.0.0.1/' -e 's/{{.BackendPort}}/18001/' test/fixtures/0011-http-fault/envoy-go.yaml > /tmp/rendered.yaml
go run ./test/fixtures/0011-http-fault/backends --port 18001 &
go run ./cmd/envoy-go --config-path /tmp/rendered.yaml &
sleep 1
curl -sS http://127.0.0.1:10000/scenario1/foo
# expect: 200 OK with body "backend\n" after ~100ms delay
kill %2 %1
```

Expected: scenario 1 fires through; the 100ms delay is honored; body is `backend\n`.

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0011-http-fault/envoy.yaml test/fixtures/0011-http-fault/envoy-go.yaml
git commit -m "phase 09: fixture 0011 envoy.yaml + envoy-go.yaml per SPEC §7.4"
```

SHA-fill follow-up.

*Anchored: SPEC §7.4 (verbatim YAML), planner-time decision 8 (STRICT_DNS), ADR-0010 (host.docker.internal + dns_lookup_family V4_ONLY), planner-time decision 11 (path).*

---

## Task 13: Fixture 0011 — `expectations.yaml` + `README.md`

**Files:**
- Create: `test/fixtures/0011-http-fault/expectations.yaml`
- Create: `test/fixtures/0011-http-fault/README.md`

This task lands the prose narrative documents per SPEC §4.3 + §7.1. expectations.yaml documents the per-scenario equivalence claims (the runner enforces via the driver's per-scenario assertions in Task 14, NOT via this YAML — per ADR-0019 expectations.yaml is documentation, not machine-evaluated). README.md is the fixture's overview + per-scenario equivalence-claim narrative + dual-proxy bootstrap discipline. Per planner-time decision 7: the status-text allow-list for non-stdlib codes is documented here.

**Precondition:** Task 12 done.
**Artifact:** two new docs.
**Acceptance:** content matches SPEC §7.1 four-scenario list + §13.1 4-header-set + §11.5 status-text allow-list disposition.

- [ ] **Step 1: Write `test/fixtures/0011-http-fault/expectations.yaml`**

```yaml
# Per-scenario equivalence claims for fixture 0011-http-fault (phase 09).
# Per ADR-0019 this YAML is prose documentation; the runner enforces via
# driver/driver.go's per-scenario assertions.

scenarios:
  - id: scenario1
    path_prefix: /scenario1
    description: |
      Listener-level inheritance: NO per-route override → request inherits
      listener fault delay 100% 100ms (no abort).
    expect:
      status: 200
      body_byte_equal: "backend\n"
      time_total_ms_min: 90
      time_total_ms_max: 110
      stat_deltas:
        delays_injected: +1
        aborts_injected: +0
        faults_overflow: 0
        active_faults: 0  # final
        response_rl_injected: 0  # always zero in phase 09 per ADR-0107 route A

  - id: scenario2
    path_prefix: /scenario2
    description: |
      Combined delay+abort per-route override: per-route HTTPFault re-includes
      the delay AND adds abort 503; wholesale-override per ADR-0073/§11.7 +
      timer-callback decision per ADR-0102 (delay fires, then abort fires).
    expect:
      status: 503
      body_byte_equal: "fault filter abort"  # 18 bytes, NO trailing newline
      headers:                                # 4-header set per §11.3 / ADR-0103
        content-length: "18"
        content-type: "text/plain"            # NO charset modifier
        date: "<allow-listed>"
        server: "envoy"
      time_total_ms_min: 90
      stat_deltas:
        delays_injected: +1
        aborts_injected: +1

  - id: scenario3a
    path_prefix: /scenario3-wholesale
    description: |
      Per-route wholesale-override (the canonical wholesale-override demo):
      per-route abort 418 with NO delay field; the per-route HTTPFault
      WHOLESALE-replaces the listener-level delay (ADR-0073 + §11.7).
    expect:
      status: 418
      status_text_allow_listed: true   # planner-time decision 7: non-stdlib codes
      body_byte_equal: "fault filter abort"
      time_total_ms_max: 50            # NO inherited delay
      stat_deltas:
        aborts_injected: +1

  - id: scenario3b
    path_prefix: /scenario3-baseline
    description: |
      Listener-level baseline (NO per-route override): inherits listener fault
      delay 100% 100ms only. Probe to confirm baseline-vs-wholesale delta.
    expect:
      status: 200
      body_byte_equal: "backend\n"
      time_total_ms_min: 90
      time_total_ms_max: 110
      stat_deltas:
        delays_injected: +1

  - id: scenario4
    path_prefix: /scenario4
    description: |
      Headers-field exact-match gate (4 sub-probes per §7.1):
        4a — no x-fault-on header → no fault → backend 200
        4b — x-fault-on: yes → fault fires → 503 abort
        4c — X-FAULT-ON: yes (uppercase NAME) → match (case-insensitive) → 503
        4d — x-fault-on: YES (uppercase VALUE) → no match (case-sensitive) → 200
      Listener-level delay is wholesale-replaced by the per-route fault config
      (which has no delay field), so the misses (4a, 4d) hit the backend
      immediately (no inherited delay).
    expect:
      probe_a: { status: 200, body_byte_equal: "backend\n" }
      probe_b: { status: 503, body_byte_equal: "fault filter abort" }
      probe_c: { status: 503, body_byte_equal: "fault filter abort" }
      probe_d: { status: 200, body_byte_equal: "backend\n" }
      stat_deltas:
        aborts_injected: +2  # probes b + c hit
```

- [ ] **Step 2: Write `test/fixtures/0011-http-fault/README.md`**

```markdown
# Fixture 0011 — http-fault

Differential gate for envoy-go's `envoy.filters.http.fault` HTTP filter against
reference Envoy v1.37.2 per phase 09 SPEC §7.

## Equivalence claims (4 scenarios per SPEC §7.1)

1. **scenario1** (`/scenario1`) — listener-level delay-only inheritance:
   200 OK + body `backend\n`, time_total ≈ 100ms, stat `delays_injected += 1`.
2. **scenario2** (`/scenario2`) — combined delay+abort per-route override:
   503 + body `fault filter abort` (18 bytes, no newline) + 4-header set,
   time_total ≈ 100ms, stats `delays_injected += 1`, `aborts_injected += 1`.
3. **scenario3** — per-route wholesale-override demo:
   - **3a** (`/scenario3-wholesale`) — abort 418 wholesale-replaces listener delay:
     418 + body `fault filter abort`, time_total < 50ms (NO inherited delay).
   - **3b** (`/scenario3-baseline`) — no per-route override, inherits listener:
     200 + body `backend\n`, time_total ≈ 100ms.
4. **scenario4** (`/scenario4`) — headers-field exact-match gate:
   4 sub-probes a/b/c/d testing case-insensitive header NAME + case-sensitive
   header VALUE per §11.8.

## Bootstrap discipline

- Reference: Envoy v1.37.2 in Docker; admin :9902, listener :10001 (in-container;
  published to runner-allocated host ports).
- Subject: envoy-go on the host; admin + listener on runner-allocated ports.
- Backend: `test/fixtures/0011-http-fault/backends/backend.go` (Go HTTP/1.1)
  bound to a runner-allocated port; serves `200 OK` + body `backend\n` on `/`.
- Cluster: STRICT_DNS pointing at the backend hostname (per planner-time
  decision 8 + ADR-0010); reference container resolves via
  `host.docker.internal`; subject resolves via Go's net resolver.

## Status-text allow-list (planner-time decision 7)

For the non-stdlib status code 418 (scenario 3a + scenario 4 if extended):
Envoy emits `HTTP/1.1 418 Unknown` (no built-in status-text table for non-RFC
codes); envoy-go's `net/http` stdlib emits `HTTP/1.1 418 I'm a teapot`. The
differential equivalence is on STATUS CODE only for non-stdlib codes; status
TEXT is allow-listed. Standard codes (200, 503, 404, 405) compare byte-equal
on both code AND text.

## Twin-stat-series allow-list

`fault.response_rl_injected` is emitted as a permanently-zero counter on
both proxies (per ADR-0107 route A). The differential diff sees `0 == 0` for
this counter on every probe; allow-listed only in the documentation sense.

## SIGTERM behavior

Phase 09 introduces no SIGTERM-related divergence; envoy-go's drain
discipline from phase 08.2 is unchanged. The fixture does NOT exercise the
drain path.

## Cross-references

- SPEC §7.1 (per-scenario equivalence claims)
- SPEC §11.1 (PGV abort.http_status validation), §11.2 (delay timing samples),
  §11.3 (4-header-set + body byte-exact), §11.5 (header-driven path deferred
  per ADR-0104), §11.6 (5-stat verification), §11.7 (wholesale-override),
  §11.8 (headers-field semantics)
- ADR-0103 (abort terminal-replace), ADR-0102 (delay async-resume),
  ADR-0107 (5-stat extension), ADR-0073 (3-tier merge), ADR-0010 (host
  networking discipline)
```

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0011-http-fault/expectations.yaml test/fixtures/0011-http-fault/README.md
git commit -m "phase 09: fixture 0011 expectations.yaml + README.md"
```

SHA-fill follow-up.

*Anchored: SPEC §7.1 (scenario list), §11.5 (status-text allow-list), §13.1 (4-header set), planner-time decision 7 (status-text allow-list narrowing); ADR-0019 (expectations.yaml is prose).*

---

## Task 14: Fixture 0011 — `driver/driver.go` (four-scenario orchestration + StatsAsserter) + runner blank-import

**Files:**
- Create: `test/fixtures/0011-http-fault/driver/driver.go`
- Modify: `test/differential/runner_test.go` (add the blank-import deferred from Task 10)

This task lands the four-scenario driver per SPEC §7.3 + planner-time decision 8 (STRICT_DNS) + planner-time decision 7 (status-text allow-list). The driver implements `fixture.Driver` (BackendCount + SubjectListenerName + ReferenceBootstrap + SubjectConfig + ReferenceListenerPort + DriveReference + DriveSubject + ProbeAdmin) + the optional `fixture.BackendKindAware` (returns HTTPFault) + `fixture.StatsAsserter` (asserts the per-scenario stat deltas). DriveReference + DriveSubject issue the same 8-probe sequence against each proxy and return identical per-probe assertion-log byte streams; the runner's CompareBytes passes when both proxies produce equal logs. ProbeAdmin issues `GET /ready` against both proxies for the admin-diff at runner step 9. AssertStats scrapes `/stats?filter=fault` after the drive completes, parses the 5 fault.* counter values, and asserts the per-scenario deltas per the §7.1 expectation matrix (sum across all probes: `delays_injected = 3` (scenarios 1, 2, 3b), `aborts_injected = 4` (scenarios 2, 3a, 4b, 4c), `faults_overflow = 0`, `active_faults = 0` final, `response_rl_injected = 0` always).

**Precondition:** Tasks 10 (BackendKind), 11 (backend), 12 (YAMLs), 13 (docs) done.
**Artifact:** new driver.go + modified runner_test.go (blank-import).
**Acceptance:** `go build ./test/fixtures/0011-http-fault/...` clean; `go test -count=1 ./test/differential/ -run 'TestDifferential/0011-http-fault'` PASSES end-to-end (the differential gate (e) fires for fixture 0011).

- [ ] **Step 1: Write `test/fixtures/0011-http-fault/driver/driver.go`**

The driver mirrors the 0007a-cors and 0010-graceful-drain driver shape. Key elements:

```go
// Package driver registers the 0011-http-fault fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.fault and reference Envoy v1.37.2 per phase 09 SPEC §7.
package driver

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "text/template"
    "time"

    "github.com/esalaine/envoy-go/test/differential/fixture"
)

const fixtureName = "0011-http-fault"

func init() {
    fixture.RegisterFixture(fixtureName, &faultDriver{})
}

type faultDriver struct{}

func (faultDriver) BackendCount() int                                  { return 1 }
func (faultDriver) BackendKind() fixture.BackendKind                   { return fixture.HTTPFault }
func (faultDriver) SubjectListenerName() string                        { return "l_main" }
func (faultDriver) ReferenceListenerPort() int                         { return 10001 }

// ReferenceBootstrap renders test/fixtures/0011-http-fault/envoy.yaml with the
// backend-host (= host.docker.internal per ADR-0010) and runner-allocated port.
func (faultDriver) ReferenceBootstrap(backendPorts []int) string {
    tpl := mustReadFixtureFile("envoy.yaml")
    return mustRender(tpl, map[string]any{
        "BackendHost": "host.docker.internal",
        "BackendPort": backendPorts[0],
    })
}

// SubjectConfig renders test/fixtures/0011-http-fault/envoy-go.yaml with the
// runner-allocated subject ports + backend port.
func (faultDriver) SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
    tpl := mustReadFixtureFile("envoy-go.yaml")
    return mustRender(tpl, map[string]any{
        "AdminPort":    subjAdminPort,
        "ListenerPort": subjListenerPort,
        "BackendHost":  "127.0.0.1",
        "BackendPort":  backendPorts[0],
    })
}

// DriveReference + DriveSubject issue the 8-probe sequence and return a
// deterministic assertion-log byte stream. CompareBytes passes when both
// proxies produce identical logs.
func (d faultDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
    return d.driveProxy(ctx, addr, "ref")
}

func (d faultDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
    return d.driveProxy(ctx, addr, "subj")
}

// driveProxy issues 8 probes (4 scenarios + scenario 4's 4 sub-probes) against
// addr and returns deterministic-format assertion-log lines. The "side" tag
// (ref vs subj) is INTENTIONALLY excluded from the log lines so the two sides
// produce identical byte streams when behavior is equivalent.
func (faultDriver) driveProxy(ctx context.Context, addr, _side string) ([]byte, error) {
    var out bytes.Buffer
    type probe struct {
        id      string
        path    string
        headers map[string]string
    }
    probes := []probe{
        {"scenario1", "/scenario1/anything", nil},
        {"scenario2", "/scenario2/anything", nil},
        {"scenario3-wholesale", "/scenario3-wholesale/anything", nil},
        {"scenario3-baseline", "/scenario3-baseline/anything", nil},
        {"scenario4-a", "/scenario4/anything", nil},
        {"scenario4-b", "/scenario4/anything", map[string]string{"x-fault-on": "yes"}},
        {"scenario4-c", "/scenario4/anything", map[string]string{"X-FAULT-ON": "yes"}},
        {"scenario4-d", "/scenario4/anything", map[string]string{"x-fault-on": "YES"}},
    }
    for _, p := range probes {
        status, body, elapsed, err := httpProbe(ctx, addr, p.path, p.headers)
        if err != nil {
            fmt.Fprintf(&out, "probe %s ERROR: %v\n", p.id, err)
            continue
        }
        // Per planner-time decision 7: status-text allow-list — log status code only.
        // Body is asserted byte-equal across sides; elapsed is bucketed (delay vs no-delay).
        // Threshold 80ms = midway between the 0ms wholesale-override-no-delay path and
        // the 100ms inherited-delay path; comfortable margin against the ±10ms timing
        // tolerance of §13.3 + small CI overhead. A flake here would surface as a
        // CompareBytes diff with the bucket strings differing — clearer than a missed
        // assertion. If timing flakes still surface in CI, widen the threshold (and
        // narrow the inherited-delay test to assert ≥80ms separately if needed).
        elapsedBucket := "fast"
        if elapsed > 80*time.Millisecond {
            elapsedBucket = "delayed"
        }
        fmt.Fprintf(&out, "probe %s status=%d body=%q elapsed=%s\n", p.id, status, body, elapsedBucket)
    }
    return out.Bytes(), nil
}

// AssertStats per SPEC §7.1 stat-delta matrix. Scrapes both proxies' /stats?filter=fault
// and asserts the 5 fault.* counters match.
func (d faultDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
    t.Helper()
    expected := map[string]int64{
        "fault.aborts_injected":     4, // scenarios 2, 3a, 4b, 4c
        "fault.delays_injected":     3, // scenarios 1, 2, 3b
        "fault.faults_overflow":     0,
        "fault.active_faults":       0, // final
        "fault.response_rl_injected": 0, // permanently zero per ADR-0107
    }
    refStats := scrapeFaultStats(t, refAdminAddr)
    subjStats := scrapeFaultStats(t, subjAdminAddr)
    for name, want := range expected {
        if got := refStats[name]; got != want {
            t.Errorf("ref %s = %d, want %d", name, got, want)
        }
        if got := subjStats[name]; got != want {
            t.Errorf("subj %s = %d, want %d", name, got, want)
        }
    }
}

// ProbeAdmin per the standard admin-diff pattern.
func (faultDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
    refBytes, err := scrapeReady(ctx, refAdminAddr)
    if err != nil { return nil, nil, fmt.Errorf("ref ready: %w", err) }
    subjBytes, err := scrapeReady(ctx, subjAdminAddr)
    if err != nil { return nil, nil, fmt.Errorf("subj ready: %w", err) }
    return refBytes, subjBytes, nil
}

// (helpers: mustReadFixtureFile, mustRender, httpProbe, scrapeFaultStats, scrapeReady
// — implementer fills in per the existing 0007a-cors / 0010-graceful-drain driver
// patterns. mustReadFixtureFile reads from the fixture directory at runtime via
// filepath.Join(repoRoot, "test/fixtures/0011-http-fault", name) — the runner
// passes the repo root via the standard fixture-driver convention.)

// Compile-time interface assertions.
var (
    _ fixture.Driver           = (*faultDriver)(nil)
    _ fixture.BackendKindAware = (*faultDriver)(nil)
    _ fixture.StatsAsserter    = (*faultDriver)(nil)
)
```

(The implementer at Task 14 fills in the helper functions per the existing 0007a-cors and 0010-graceful-drain precedents — `mustReadFixtureFile` mirrors the existing `loadFixtureYAML` helper if present, OR uses a simple `os.ReadFile`-based lookup; `httpProbe` issues `GET path` against `addr` with optional headers and returns `(status, body, elapsed, error)`; `scrapeFaultStats` issues `GET /stats?filter=fault&format=prometheus` and parses the 5 lines into a `map[string]int64`; `scrapeReady` issues `GET /ready` and returns the raw response bytes.)

- [ ] **Step 2: Add the deferred blank-import in `test/differential/runner_test.go`**

In the import block (currently lines 24–35), insert (alphabetically after the `0010-graceful-drain` blank-import):

```go
_ "github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver"
```

- [ ] **Step 3: Verify build**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 4: Run the new fixture**

```bash
go test -count=1 ./test/differential/ -run 'TestDifferential/0011-http-fault' -v
```

Expected: PASSES end-to-end. The runner spawns the backend + reference Envoy in Docker + the envoy-go subject as a host process, drives both via the 8-probe sequence, compares the byte streams, and runs AssertStats. Total wall-clock: <30s (dominated by reference container startup ~10s + the 300ms of in-fixture delays).

If the test fails, the bug is likely in: (a) the YAML template rendering (port substitution); (b) the SendLocalReply OrderedHeaders carrier (verify content-type header exact match between proxies); (c) the scrapeFaultStats parser (Prometheus exposition format edge cases); (d) the percentage-roll determinism for the 0%/100% cases (should be deterministic, but verify the rng short-circuit fires).

- [ ] **Step 5: Verify regressions**

```bash
go test -count=1 -short ./...
go test -count=1 ./test/differential/ -run 'TestDifferential/0000|TestDifferential/0001|TestDifferential/0002|TestDifferential/0003|TestDifferential/0004|TestDifferential/0005|TestDifferential/0006|TestDifferential/0007a|TestDifferential/0007b|TestDifferential/0008|TestDifferential/0009|TestDifferential/0010'
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add test/fixtures/0011-http-fault/driver/driver.go test/differential/runner_test.go
git commit -m "phase 09: fixture 0011 driver + StatsAsserter (4 scenarios, 8 probes)"
```

SHA-fill follow-up.

*Anchored: SPEC §7.1 (scenario list), §7.3 (driver outline), §7.4 (bootstrap rendering), planner-time decisions 7 (status-text allow-list) + 8 (STRICT_DNS) + 11 (path); ADR-0103 (4-header set assertion), ADR-0107 (5-stat delta assertion).*

---

## Task 15: BEHAVIOR_CONTRACT.md patches per SPEC §13 + ADR-0104 (deferral) + ADR-0106 (family-expansion shape) + ROADMAP row 09 in-progress→done

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (per SPEC §13.1, §13.2, §13.3, §13.4, §13.5)
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0104 + ADR-0106)
- Modify: `docs/envoy-go/ROADMAP.md` (row 09 in-progress→done)

This task lands the documentation patches that close the BEHAVIOR_CONTRACT alignment between code-side fault behavior + doc-side asserted-equivalence prose. Per ADR-0052 in-place edit authorisation. The two remaining ADRs (ADR-0104 deferral + ADR-0106 family-expansion shape) land here because both are documentation-anchored: ADR-0104 codifies the §13.1 "Does not yet apply to" deferral table; ADR-0106 governs the ROADMAP row 09 in-progress→done flip + the §9 family heading's UNCHANGED state.

**Precondition:** Task 14 done; differential gate green for fixture 0011.
**Artifact:** modified BEHAVIOR_CONTRACT.md + DECISIONS.md (ADR-0104 + ADR-0106) + ROADMAP.md.
**Acceptance:** BEHAVIOR_CONTRACT.md carries the 5 patches per SPEC §13.1–§13.5; DECISIONS.md tail is ADR-0107 (after ADR-0106 lands as the last new ADR; the in-task land order is ADR-0104 first, then ADR-0106 — final tail by number is ADR-0107 from Task 3); ROADMAP row 09 reads `done`; the §9 family heading at line 56 is UNCHANGED.

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md §13.1 patch**

Insert the new `### envoy.filters.http.fault` subsection under `## HTTP filter chain` umbrella (line 695). Use the SPEC §13.1 verbatim Markdown patch as the body. Insertion point: AFTER the existing `### Empirical evidence (cors preflight)` block (which ends near line 831) and BEFORE the next sibling subsection.

Per SPEC §13.1's verbatim block:

- `#### Asserted equivalence (per phase 09 SPEC §11)` — abort + delay + combined + headers paragraphs.
- `#### Per-route 3-tier merge (per ADR-0073 + phase 09 SPEC §11.7)` — wholesale-override prose.
- `#### max_active_faults concurrency cap` — cap behavior + LBP-1 sixth.
- `#### Async-resume mechanics (per ADR-0102)` — time.AfterFunc + ContinueDecoding + OnDestroy + markedActive.
- `#### Does not yet apply to (per phase 09 deferrals — ADRs 0101, 0103, 0104, 0107)` — header-driven path, response_rate_limit, abort.grpc_status, upstream_cluster, downstream_nodes, runtime-key fields, disable_downstream_cluster_stats, filter_enabled, HeaderMatcher non-exact, H2 differential testing.
- `#### Empirical evidence (verbatim curl excerpts from phase 09 SPEC §11.3)` — the abort-response capture.

- [ ] **Step 2: Apply BEHAVIOR_CONTRACT.md §13.2 patch — 17→22-name table extension**

Rename the heading at line 130 from `### 17-name table (introduced by phase 06.1)` to `### 22-name table (introduced by phase 06.1; extended by phase 09)`. Append after the existing "Server — 2 names" subsection:

```markdown
**Fault filter — 5 names (introduced by phase 09):**

| Internal name | Type | Prometheus name |
|---|---|---|
| `http.<stat_prefix>.fault.aborts_injected` | counter | `envoy_http_fault_aborts_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.delays_injected` | counter | `envoy_http_fault_delays_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.faults_overflow` | counter | `envoy_http_fault_faults_overflow{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.active_faults` | gauge | `envoy_http_fault_active_faults{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.response_rl_injected` | counter | `envoy_http_fault_response_rl_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |

`response_rl_injected` is emitted as a permanently-zero counter in phase 09
— Envoy emits it even when `response_rate_limit` is not configured (per
phase 09 §11.6 empirical pin); envoy-go matches the surface for differential
parity per ADR-0107 route A. When `response_rate_limit` lands in a future
phase, the same name carries the actual count.

**Total: 22 internal names** (17 from 06.1 + 5 from 09).
```

Update the existing total-line at line 171 from `Total: 17 internal names.` to `Total: 22 internal names.`

- [ ] **Step 3: Apply BEHAVIOR_CONTRACT.md §13.3 patch — Timing tolerances bullet**

Append a new bullet at the end of `## Timing tolerances` (line 266+):

```markdown
- **Fault filter delay accuracy: ±10ms (per phase 09 §11.2 empirical pin).**
  envoy-go's `time.AfterFunc` timer-driven async-resume matches Envoy v1.37.2's
  fault delay accuracy within ±10ms across the 50/100/200/500ms sweep.
  Empirical worst-case overhead observed: +3.6ms (Envoy v1.37.2 was tested;
  envoy-go's overhead is similar). The differential fixture 0011-http-fault's
  expectations.yaml asserts `time_total ∈ [delay - 10ms, delay + 10ms]` for
  delay scenarios.
```

- [ ] **Step 4: Apply BEHAVIOR_CONTRACT.md §13.4 patch — Equivalence Matrix new row**

Append one new row to the equivalence matrix table (around line 9–35):

```markdown
| HTTP filter `envoy.filters.http.fault` | Per-request equivalence on abort response shape (status + 4-header set + body byte-exact `fault filter abort`), delay timing (±10ms tolerance), per-route wholesale-override resolution, headers-field exact-match gate, and stat counter increments under the per-scenario differential gate (fixture 0011-http-fault). NOT asserted: header-driven fault path (deferred — ADR-0104), response_rate_limit (deferred), abort.grpc_status (deferred), HeaderMatcher non-exact variants. |
```

- [ ] **Step 5: Apply BEHAVIOR_CONTRACT.md §13.5 patches — three forward-pointer notes**

(a) After `## HTTP filter chain ### Async resume mechanics` (line ~720): add the note about phase 09 being the FIRST production exerciser of the async-resume primitive on the request side.

(b) After `## Stat-name mapping ### Twin-series filter discipline` (line 173): add the note about route A for `fault.response_rl_injected` per ADR-0107.

(c) After `## Equivalence Matrix` (line 9 area): the new HTTP-filter-fault row already lands per Step 4.

- [ ] **Step 6: Append ADR-0104 to DECISIONS.md (deferral-ADR per ADR-0040 format)**

ADR-0104 covers: header-driven fault path DEFERRED; coupled to delay.header_delay / abort.header_abort proto sub-messages per §11.5 empirical pin major surprise; phase 09 silently parses both sub-messages but does not honor them; the four documented request headers are silently ignored; future small follow-up phase (~150 LoC) lands the coupled pair in one coherent slice. Lands-in-task: Task 15. Format: ADR-0040 deferral-ADR (Status: Deferred; Forward-pointer to future phase).

- [ ] **Step 7: Append ADR-0106 to DECISIONS.md**

ADR-0106 covers: §9 HTTP filters family expansion shape — flat top-level rows for §9 family-children + no-sibling-stub discipline + BOOTSTRAP_PROMPT.md §9 invariant 4 reading (the §9 heading at ROADMAP line 56 is a conceptual umbrella, not a row; its state stays unchanged across all family-row landings; future family-expansion brainstorms cold-start from the §9 heading + just-shipped artefacts). Lands-in-task: Task 15.

- [ ] **Step 8: Apply ROADMAP.md row 09 flip**

Find the row `09` line (per the SPEC commit's row addition) and flip the status field from `in-progress` to `done`. Verify the §9 HTTP filters family heading at row 56 is UNCHANGED (no row state on a heading).

- [ ] **Step 9: Verify documentation cohesion**

```bash
grep -n 'fault' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -20
grep -n '## ADR-0104\|## ADR-0106' docs/envoy-go/DECISIONS.md
grep -n '^| 09' docs/envoy-go/ROADMAP.md
```

Expected: BEHAVIOR_CONTRACT.md carries 5 fault-related patches; DECISIONS.md has ADR-0104 + ADR-0106 sections; ROADMAP row 09 status reads `done`.

- [ ] **Step 10: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/ROADMAP.md
git commit -m "phase 09: BEHAVIOR_CONTRACT + ADR-0104 + ADR-0106 + ROADMAP row 09 done"
```

SHA-fill follow-up.

*Anchored: SPEC §13.1–§13.5 (verbatim patches), §8 (ADR table); ADR-0104 (deferral, ADR-0040 format), ADR-0106 (family-expansion shape), ADR-0052 (in-place edit authorisation), ADR-0061 (stat-name mapping unchanged); BOOTSTRAP_PROMPT.md §9 invariant 4 (heading vs row distinction).*

---

## Task 16: Phase-done six-gate verification + STATE.md advance + phase-done commit

**Files:**
- Modify: `docs/envoy-go/STATE.md` (advance to `awaiting next planning`; per `BOOTSTRAP_PROMPT.md` §5 lifecycle)

This task runs the six-gate checklist per SPEC §3 + BOOTSTRAP_PROMPT.md §7.5 and lands the phase-done commit. The six gates are all green at this point per Tasks 1–15:
- (a) `go build ./...` clean (Task 8 verified)
- (b) `go test ./...` clean including new fault unit/race tests (Tasks 3–9 verified)
- (c) h2spec re-run clean at 53/53 PASS (mechanical re-run; phase 09 touches no codec/framer/HPACK paths)
- (d) Existing 11 fuzzers + new `FuzzFaultConfigParse` clean at 30s budget (Task 9 verified for the new one; the 11 existing are mechanical re-run)
- (e) Differential fixtures all green including new 0011-http-fault (Task 14 verified)
- (f) BEHAVIOR_CONTRACT.md populated per §13 (Task 15 verified)

The phase-done commit message subject names all eight ADRs ADR-0100..ADR-0107 per `BOOTSTRAP_PROMPT.md` §5.3 commit-message-completeness; the body explicitly states ROADMAP row 09 flips `in-progress → done`, the §9 family heading at ROADMAP line 56 stays unchanged, and phase 09 is the FIRST §9 family-row to land.

**Precondition:** Task 15 done; all docs landed.
**Artifact:** modified STATE.md; phase-done commit.
**Acceptance:** all six gates report green; STATE.md flipped; phase-done commit lands with the ADR-naming subject + the row-flip body.

- [ ] **Step 1: Run all six gates**

```bash
# Gate (a)
go build ./... 2>&1 | tee /tmp/gate-a.txt
# Gate (b)
go test -race -count=1 ./... 2>&1 | tee /tmp/gate-b.txt
# Gate (c) — h2spec mechanical re-run
go test -count=1 ./test/conformance/h2spec/... 2>&1 | tee /tmp/gate-c.txt
# Gate (d) — 12 fuzzers mechanical re-run at 30s each
for fuzz in FuzzBootstrapParse FuzzHCMConfigParse FuzzClusterParse FuzzHTTP1Codec FuzzListenerParse FuzzAccessLogFormat FuzzPerRouteResolveStability FuzzCorsConfigParse FuzzAdminPathDispatch FuzzServerInfoStateClassification FuzzDrainTransitions FuzzFaultConfigParse; do
    echo "=== $fuzz ==="
    go test -fuzz=$fuzz -fuzztime=30s -run=$fuzz ./... 2>&1 | tail -3
done | tee /tmp/gate-d.txt
# Gate (e)
go test -count=1 ./test/differential/... 2>&1 | tee /tmp/gate-e.txt
# Gate (f) — verify BEHAVIOR_CONTRACT alignment
grep -c 'envoy.filters.http.fault' docs/envoy-go/BEHAVIOR_CONTRACT.md  # expect: >= 8 references
grep -c 'response_rl_injected' docs/envoy-go/BEHAVIOR_CONTRACT.md     # expect: >= 2 references
grep '^| 09' docs/envoy-go/ROADMAP.md                                  # expect: status `done`
```

If any gate fails, fix before proceeding.

- [ ] **Step 2: Update STATE.md**

Edit `docs/envoy-go/STATE.md` to flip:

- `active-phase: awaiting next planning` (per BOOTSTRAP_PROMPT.md §5 lifecycle — phase 09 is closed; the next session's planner selects the next §9 family-child via brainstorming).
- `phase-directory: ` (cleared; the next session populates).
- `lifecycle-state: awaiting`.
- `next-skill: superpowers:brainstorming` (against §9's family list per ADR-0106 — flat top-level rows; the next family-child is the next session's brainstorm scope).
- `last-commit: <phase-done commit SHA>` (TBD until Step 4; SHA-fill follow-up commit lands the SHA).
- `last-updated: <today's date>`.

- [ ] **Step 3: Commit STATE.md**

```bash
git add docs/envoy-go/STATE.md
git commit -m "$(cat <<'EOF'
phase 09: http-filter-fault [ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]

ROADMAP row 09 flips in-progress → done AT THIS COMMIT. The §9 HTTP filters
family heading at ROADMAP line 56 stays UNCHANGED (headings are not rows;
their state is implicit per ADR-0106 settled by this phase). Phase 09 is
the FIRST §9 family-row to land; subsequent filters (header_mutation,
buffer, local_ratelimit, ...) follow the same row-as-its-own-phase pattern
per BRAINSTORM Decision 12 + ADR-0106.

Lands envoy.filters.http.fault as the SECOND production HTTP filter after
cors (07.1) and the FIRST top-level row under BOOTSTRAP_PROMPT.md §9's
HTTP filters family. New internal/filter/http/fault/ package with
TypeURL + New + filter struct + decoder/encoder methods per the cors
precedent. Five new fault.* stats (4 counters + 1 gauge) extend the
17-name BEHAVIOR_CONTRACT table to 22 names. New differential fixture
0011-http-fault (4 scenarios per §7.1) is green at gate (e). Twelfth
fuzzer FuzzFaultConfigParse ships per ADR-0018. FactoryCtx framework
extension (Stats + StatPrefix fields) lands per ADR-0100 first-use anchor
so future stats-bearing filters reuse the same fields.

Eight new ADRs ADR-0100..ADR-0107: ADR-0100 package shape + boot
registration + FactoryCtx extension; ADR-0101 runtimeConfig + 6/11-field
decomposition + abort.http_status PGV mirror + percentage-roll
determinism; ADR-0102 delay async-resume + combined-path timer-callback
decision; ADR-0103 abort terminal-replace + body byte-exact + 4-header
set + status-text allow-list; ADR-0104 header-driven fault path DEFERRED
per §11.5 empirical-pin major surprise (coupled to delay.header_delay /
abort.header_abort proto sub-messages); ADR-0105 max_active_faults
concurrency cap + LBP-1 sixth + markedActive idempotency guard;
ADR-0106 §9 HTTP filters family expansion shape — flat top-level rows;
ADR-0107 17→22-name extension + response_rl_injected route A.

Six gates all green: (a) go build ./... clean; (b) go test -race
./... clean (new fault unit + race tests); (c) h2spec 53/53 PASS at
ADR-0051 pin (mechanical re-run); (d) 12 fuzzers clean at 30s; (e)
differential 0000-0011 all green; (f) BEHAVIOR_CONTRACT updated per
SPEC §13.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: SHA-fill STATE.md follow-up**

After the phase-done commit lands, fill in `last-commit: <SHA>` per the phase-04..08.2 SHA-fill convention:

```bash
SHA=$(git rev-parse HEAD)
sed -i "s|last-commit: TBD|last-commit: \`$SHA\`|" docs/envoy-go/STATE.md
git add docs/envoy-go/STATE.md
git commit -m "phase 09 follow-up: STATE.md SHA-fill for phase-done commit (TBD → $SHA)"
```

*Anchored: SPEC §3 (six-gate checklist), §15 (acceptance criteria); BOOTSTRAP_PROMPT.md §5.3 (commit-message-completeness), §7.5 (six gates), §5 (lifecycle); ADR-0051 (h2spec pin), ADR-0018 (fuzz CI 30s short-budget), ADR-0052 (in-place edit auth), ADR-0106 (ROADMAP row-flip + heading invariance).*

---

## Task 17: REVIEW.md — end-of-phase review per requesting-code-review skill

**Files:**
- Create: `docs/envoy-go/phases/09-http-filter-fault/REVIEW.md`

This task lands the end-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 cadence. The REVIEW evaluates the phase's deliverables against the SPEC + PLAN, identifies any carry-forward items for future phases, and records the implementer's assessment of the phase's strengths/weaknesses + lessons learned. Per the requesting-code-review skill, the REVIEW is the canonical post-phase artifact that informs both the phase's retrospective AND future cold-starts.

**Precondition:** Task 16 done; phase-done commit landed.
**Artifact:** new REVIEW.md.
**Acceptance:** REVIEW.md mirrors the structural shape of `docs/envoy-go/phases/08.2-graceful-drain/REVIEW.md`; covers the SPEC's 16 sections + PROGRESS.md's per-task entries; identifies any N-1 carry-forward items for the next §9 family-child phase.

- [ ] **Step 1: Invoke `superpowers:requesting-code-review` skill against the phase 09 changeset**

The skill produces a structured review covering: (a) SPEC↔code alignment (every SPEC section has a corresponding implementation/test); (b) ADR cohesion (every ADR's Lands-in-task is satisfied); (c) BEHAVIOR_CONTRACT cohesion; (d) test coverage assessment; (e) any minor (M-N) findings to carry forward; (f) any major (J-N) findings (none expected for phase 09). Save the review output to `docs/envoy-go/phases/09-http-filter-fault/REVIEW.md`.

- [ ] **Step 2: Identify N-1 carry-forward items for the next §9 family-child**

Per the 06.1 → 06.2 / 07.1 → 07.2 / 08.1 → 08.2 N-1 carry-forward convention, list any minor items the phase 09 implementer noticed during the work that would benefit the next family-child phase. Examples (illustrative, not actual findings — the implementer's review identifies the actual list):

- Filter-package documentation cross-reference convention (link from new package's doc.go to the cors precedent's doc.go).
- StatsAsserter helper extraction if multiple HTTP filter fixtures end up sharing the same fault-stat-scrape pattern.
- FactoryCtx field-coverage test pattern as a reusable shape for future framework extensions.

These N-1 items are recorded in REVIEW.md's "Carry-forward to next §9 family-child" section and become inputs to the next family-child phase's BRAINSTORM § "Inherited carry-forward" block.

- [ ] **Step 3: Commit REVIEW.md**

```bash
git add docs/envoy-go/phases/09-http-filter-fault/REVIEW.md
git commit -m "phase 09: REVIEW — end-of-phase retrospective + N-1 carry-forward"
```

SHA-fill follow-up if the REVIEW references the phase-done commit SHA.

*Anchored: 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 REVIEW.md cadence precedents; superpowers:requesting-code-review skill.*

---

## Refinement

If during execution the implementer discovers a SPEC ambiguity, a planner-time decision that was not foreseen, or a framework constraint that requires deviation from this PLAN, the implementer:

1. Records the deviation in PROGRESS.md's per-task entry under a `**Deviation:**` line + `**Rationale:**` + `**Anchored:**` cross-reference.
2. If the deviation alters the ADR table, amend the in-task ADR's Consequences section in-place (per the ADR-0089 consequence (b) in-place-edit pattern); do NOT introduce a new ADR for the amendment unless the deviation is structurally significant.
3. If the deviation alters the file-structure table, amend this PLAN's table in a follow-up commit OR record the deviation in PROGRESS.md and let the file-structure table become "as-built" rather than "as-planned" — the implementer's choice based on whether the deviation is broadly reusable for future readers.

Common refinement scenarios anticipated:

- **The `parseHTTPFiltersChain` signature widening (Task 2) breaks existing test files.** The implementer extends affected test files to thread `nil` for the new `registry` and `""` for `statPrefix` arguments per the ADR-0085 nil-tolerance pattern. Lands inline in Task 2.
- **The fixture's status-text allow-list (planner-time decision 7) misclassifies a stdlib code.** Re-examine the planner-time decision and either widen the allow-list with a documented exception OR fix the underlying behavior. Update expectations.yaml + driver in-place; record the deviation in PROGRESS.md Task 13/14 entry.
- **The fault filter's `time.AfterFunc` timer fires during chain teardown causing a callback-on-destroyed-instance panic.** The markedActive guard is the mitigation per ADR-0105; if the panic still surfaces, the fix is to swap the markedActive bool for an atomic.Bool (since the OnDestroy and timer-callback goroutines may straddle the single-goroutine-per-stream invariant in some chain-teardown paths). Re-examine the SPEC §5.7 concurrency-model claim against the actual chain teardown sequence per `internal/filter/http/chain.go`.
- **The `response_rl_injected` permanently-zero counter triggers a stat-name uniqueness panic on the second listener boot in differential-suite parallelism.** This indicates the fixture is registering stats in the global registry rather than a fresh per-test registry; verify the runner's per-fixture isolation discipline and ensure each fixture's subject envoy-go boot allocates a fresh `*stats.Registry`.

## Post-plan handoff

After Task 17 lands the REVIEW, the orchestrating session:

1. Verifies the phase-done six gates one more time (sanity check) per Task 16.
2. Verifies STATE.md is at `awaiting next planning` with `next-skill: superpowers:brainstorming`.
3. Pushes the phase 09 worktree branch to origin (per the user's persistent preference: "after a clean local merge/commit on master with tests green, push without asking" recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`).
4. Hands off to the next session, whose first action is to invoke `superpowers:brainstorming` against §9's HTTP filters family for the next family-child (per ADR-0106 + STATE.md + BRAINSTORM.md Decision 13 — the next family-child cold-starts from the §9 heading + the just-shipped phase 09 artefacts; no sibling-stub was authored).

The phase 09 work is complete when:

- All 17 tasks in this PLAN have green checkmarks in PROGRESS.md.
- Phase-done commit + SHA-fill follow-up are on master.
- REVIEW.md is committed.
- STATE.md reflects the post-09 lifecycle state.
- The branch is pushed to origin.
- All six gates report green at HEAD.

