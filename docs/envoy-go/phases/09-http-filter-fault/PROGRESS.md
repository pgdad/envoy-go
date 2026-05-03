# Phase 09 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..08.2 PROGRESS.md structure.

## Preamble — execution preconditions

None. All 15 preconditions were satisfied at cold-start: branch is `phase-09-http-filter-fault-impl`, master log matches expected sequence (9bd8d0d PLAN SHA-fill, b963c1b PLAN, 80b3f9f SPEC SHA-fill, da29807 SPEC, 8506a3c BRAINSTORM SHA-fill, 4f44a03 BRAINSTORM, 14a68e7 08.2 SHA-fill, b33e04f 08.2 phase-done), Docker client+server reported (28.4.0 / 28.1.1), Go 1.26.2, golangci-lint 1.64.8, all packages PASS short-mode, all 12 differential fixtures PASS (TestDifferential/0000..0010 subtests under TestDifferential parent), ADR tail is `ADR-0099:`, SPEC.md commit is da29807 (HEAD-of-master at PLAN.md SHA-fill follow-up; descendant of itself), `git status --porcelain` empty, `internal/filter/http/fault/` absent, FactoryCtx single-field 2-param form (one `Registry *HTTPRegistry` line in `internal/filter/http/types.go`), `parseHTTPFiltersChain` 2-param signature present at `internal/filter/hcm/config.go:273`, envoyproxy/envoy:v1.37.2 pull success (already up to date), CONFORMANCE_PINS.md diff vs master empty.

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

**Commits:** 29c0958 — this task's commit
**Notes:** Created PROGRESS.md; verified all 15 preconditions per PLAN §"Execution preconditions"; phase-09 SPEC + 09 PLAN confirmed present in HEAD; SPEC at da29807; ADR tail at 0099 (next-free 0100); internal/filter/http/fault/ absent (Task 3 lands); FactoryCtx single-field 2-param form (Task 2 widens); parseHTTPFiltersChain 2-param signature (Task 2 widens). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-09-http-filter-fault-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | awk '{print $2}' | sort -u | tail -1
ADR-0099:
$ git log -1 --format=%H -- docs/envoy-go/phases/09-http-filter-fault/SPEC.md
da29807d83db9fe1816a8f52c5e34c1fa3b602d7
```

## Task 2 — FactoryCtx extension

**Commits:** 2449939 — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 2 Steps 1–10. Step 1 added `TestFactoryCtx_StatsRegistryThreaded` and `TestFactoryCtx_NilStatsRegistryTolerated` to `internal/filter/http/types_test.go` (the natural FactoryCtx test home; pre-Task-2 the file held the FilterHeadersStatus / FilterDataStatus / FilterTrailersStatus / FilterInterfaces compile-only tests). Step 2 confirmed compile error (`ctx.Stats undefined ... ctx.StatPrefix undefined ... unknown field Stats in struct literal`). Step 3 extended `FactoryCtx` in `internal/filter/http/types.go` with `Stats *stats.Registry` and `StatPrefix string` fields, plus the `internal/stats` import; doc-comment paragraphs anchor ADR-0061 pre-Freeze + ADR-0085 nil-tolerance + ADR-0100 first-use. Step 4 confirmed both `TestFactoryCtx_*` tests pass. Step 5 added `TestParseHTTPFiltersChain_FactoryCtxThreading` to `internal/filter/hcm/config_test.go` with `filter_http` + `router` aliases (the file's first uses; reused existing `mkRouter()` helper instead of defining a `mustAny` local). Step 6 confirmed compile error (`too many arguments in call to parseHTTPFiltersChain`). Step 7 widened `parseHTTPFiltersChain` from 2-param to 4-param shape (`registry *stats.Registry, statPrefix string` appended), updated the call site in `parseFilterWithCtx` and the `FactoryCtx` populate inside the second loop. Doc-comment paragraph notes the Phase 09 ADR-0100 first-use anchor + ADR-0085 nil-tolerance for non-stat-bearing filters. Step 8 confirmed targeted test PASS + full hcm suite PASS. Step 9 confirmed `go build`, `go vet`, `golangci-lint`, `go test -race -count=1 -short ./...` all clean. No ADR landed (ADR-0100 lands at Task 3 per PLAN's ADR-on-impl convention; this task is the framework-extension code that ADR-0100's text references). The 11 differential fixtures (0000..0010) PASS unchanged — the FactoryCtx extension is non-load-bearing for non-stat-bearing filters (router, cors, envoygotest ignore the new fields per ADR-0085).
**Outputs:**
```
$ go test ./internal/filter/http/ -run TestFactoryCtx -v
=== RUN   TestFactoryCtx_StatsRegistryThreaded
--- PASS: TestFactoryCtx_StatsRegistryThreaded (0.00s)
=== RUN   TestFactoryCtx_NilStatsRegistryTolerated
--- PASS: TestFactoryCtx_NilStatsRegistryTolerated (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.002s
$ go test ./internal/filter/hcm/ -run TestParseHTTPFiltersChain_FactoryCtxThreading -v
=== RUN   TestParseHTTPFiltersChain_FactoryCtxThreading
--- PASS: TestParseHTTPFiltersChain_FactoryCtxThreading (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.003s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.045s
ok  	github.com/esalaine/envoy-go/internal/admin	1.551s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.082s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.082s
ok  	github.com/esalaine/envoy-go/internal/drain	1.147s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.083s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.545s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.169s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.033s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.062s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.271s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.214s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.088s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.069s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.034s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.040s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.112s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.145s
ok  	github.com/esalaine/envoy-go/test/differential	1.147s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.022s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.031s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.024s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.033s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.033s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.050s
```

## Task 3 — fault package core (doc.go + fault.go + fault_test.go) [ADR-0100, ADR-0101, ADR-0107]

**Commits:** TBD — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 3 Steps 1–8. Step 1 wrote `internal/filter/http/fault/fault_test.go` with seven tests (`TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_AbortHTTPStatusOutOfRange` with 4 subcases zero/too_high/too_low/upper_exclusive, `TestNew_DelayPercentageWithoutFixedDelay`, `TestNew_HappyPath`, `TestNew_RegistersStats`, `TestRuntimeConfig_FieldExtraction`); imports settled per `go doc` queries — `commonfaultv3 "envoy/extensions/filters/common/fault/v3"` for `FaultDelay`, `faultv3 "envoy/extensions/filters/http/fault/v3"` for `HTTPFault`/`FaultAbort`, `routev3 "envoy/config/route/v3"` for `HeaderMatcher`, `matcherv3 "envoy/type/matcher/v3"` for `StringMatcher`, `typev3 "envoy/type/v3"` for `FractionalPercent`, `wrapperspb` for `UInt32`, `durationpb` for `New(time.Duration)`, `proto.Message`. Confirmed PLAN's `FaultDelaySecifier` field name is the actual generated proto name (upstream typo "Secifier" not "Specifier"). Step 2 confirmed compile error: `undefined: New ... undefined: parseRuntimeConfig`. Step 3 wrote `internal/filter/http/fault/doc.go` verbatim from PLAN lines 717–796 (decode-side 5-step discipline; async-resume mechanics; abort terminal-replace; max_active_faults LBP-1 sixth; per-route policy wholesale-override; encode-side no-op; 5-stat list; deferral list; ADR cross-reference list 0100..0107). Step 4 wrote `internal/filter/http/fault/fault.go`: `TypeURL` const + `faultAbortBody = "fault filter abort"` (18 bytes, nolint:unused for Task 4) + `faultStats` 5-field struct + `runtimeConfig` 8-scalar+1-slice struct + `headerMatch{name, exactValue}` + `New(tc, ctx)` factory (8-step contract per ADR-0101) + `parseRuntimeConfig` (PGV [200, 600) gate + delay.fixed_delay > 0 gate + headers `string_match.exact` only) + `percentageToFloat` (HUNDRED/TEN_THOUSAND/MILLION denominators) + `registerFaultStats` (nil-tolerant per ADR-0085) + `filter` 8-field struct (with `delayTimer` + `markedActive` nolint:unused for Tasks 5/6) + static interface assertions `_ envoyhttp.StreamDecoderFilter / StreamEncoderFilter = (*filter)(nil)` + decoder/encoder method set with stub DecodeHeaders returning Continue + pass-through Data/Trailers + stub OnDestroy. Two PLAN refinements: (a) abort.error_type discrimination uses type-assertion `_, ok := a.GetErrorType().(*faultv3.FaultAbort_HttpStatus)` rather than the PLAN snippet's `hs != 0 || a.GetErrorType() != nil` — the type assertion correctly silent-ignores `header_abort` and `grpc_status` variants per ADR-0104 (the snippet's heuristic would have validated hs=0 against [200, 600) for header_abort variants and incorrectly errored). (b) Headers gate parses via `h.GetHeaderMatchSpecifier().(*routev3.HeaderMatcher_StringMatch)` rather than `h.GetStringMatch()` directly because the helper would coerce non-string-match variants. Step 5 confirmed all 7 tests PASS (4 subcases of TestNew_AbortHTTPStatusOutOfRange + 6 top-level = 10 PASS lines; 0.004s). Step 6 confirmed `go build ./...` clean, `go vet ./...` clean, `golangci-lint run ./...` clean, `go test -race -count=1 ./internal/filter/http/fault/...` clean (1.013s), `go test -race -count=1 -short ./...` clean across all 30 packages including the 11 differential fixtures unchanged. Step 7 appended ADR-0100, ADR-0101, ADR-0107 to `docs/envoy-go/DECISIONS.md` per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0100 anchors package shape + `FactoryCtx` framework extension consequences; cross-references ADR-0072 (HTTPRegistry threaded constructor extended to 4 entries), ADR-0074 (filter set extended to {cors, envoygotest, fault, router}), ADR-0085 (nil-tolerance), ADR-0061 (pre-Freeze stat registration). ADR-0101 anchors runtimeConfig 6-vs-11 decomposition + PGV [200, 600) mirror + delay.fixed_delay > 0 validation + per-instance RNG seeding; cross-references ADR-0073 (3-tier merge wholesale-override empirically confirmed at §11.7), ADR-0104 (header-driven fault path deferred). ADR-0107 anchors 17→22-name extension + response_rl_injected route A; cross-references ADR-0061 (SN1–SN8 flattening rules unchanged). Each ADR's Lands-in-task field reads "Task 3 (phase 09); commit TBD" — SHA-fill follow-up replaces TBD per the 08.2 precedent (PROGRESS.md Task 3 entry's `Commits:` line + DECISIONS.md three Lands-in-task lines updated together in the SHA-fill commit).
**Outputs:**
```
$ go test ./internal/filter/http/fault/... -v
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_AbortHTTPStatusOutOfRange
=== RUN   TestNew_AbortHTTPStatusOutOfRange/zero
=== RUN   TestNew_AbortHTTPStatusOutOfRange/too_high
=== RUN   TestNew_AbortHTTPStatusOutOfRange/too_low
=== RUN   TestNew_AbortHTTPStatusOutOfRange/upper_exclusive
--- PASS: TestNew_AbortHTTPStatusOutOfRange (0.00s)
    --- PASS: TestNew_AbortHTTPStatusOutOfRange/zero (0.00s)
    --- PASS: TestNew_AbortHTTPStatusOutOfRange/too_high (0.00s)
    --- PASS: TestNew_AbortHTTPStatusOutOfRange/too_low (0.00s)
    --- PASS: TestNew_AbortHTTPStatusOutOfRange/upper_exclusive (0.00s)
=== RUN   TestNew_DelayPercentageWithoutFixedDelay
--- PASS: TestNew_DelayPercentageWithoutFixedDelay (0.00s)
=== RUN   TestNew_HappyPath
--- PASS: TestNew_HappyPath (0.00s)
=== RUN   TestNew_RegistersStats
--- PASS: TestNew_RegistersStats (0.00s)
=== RUN   TestRuntimeConfig_FieldExtraction
--- PASS: TestRuntimeConfig_FieldExtraction (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.004s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 ./internal/filter/http/fault/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.013s
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.533s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.049s
ok  	github.com/esalaine/envoy-go/internal/admin	1.542s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.088s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.083s
ok  	github.com/esalaine/envoy-go/internal/drain	1.144s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.086s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.538s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.174s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.039s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.062s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.044s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.272s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.211s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.091s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.075s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.036s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.045s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.114s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.158s
ok  	github.com/esalaine/envoy-go/test/differential	1.159s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.029s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.029s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.026s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.037s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.036s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.034s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.049s
```
