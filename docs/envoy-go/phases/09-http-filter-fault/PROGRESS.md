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

**Commits:** e80aa10 — this task's commit
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

## Task 4 — DecodeHeaders abort terminal-replace path + headers gate + percentage-roll [ADR-0103]

**Commits:** afea8ec — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 4 Steps 1–7. Step 1 added the `recordingDCB` test stub (a `DecoderFilterCallbacks` impl with `sentStatus`/`sentBody`/`sentHeaders`/`continued atomic.Int32`/`routeCfg proto.Message` fields, plus `SendLocalReply` / `ContinueDecoding` / `RequestRouteConfig` / `EncodeHeaders` / `EncodeData` / `EncodeTrailers` methods covering the 6 required interface methods per `internal/filter/http/callbacks.go`'s `DecoderFilterCallbacks` shape) + `makeFilter` helper (constructs a fault filter via `New` + `factory()` + `inst.Decoder.(*filter)` + `SetDecoderCallbacks(&recordingDCB{})` and returns the filter+dcb pair) + 6 failing tests: `TestDecodeHeaders_AbortOnly_100Percent` (abort 503 100% → StopIteration + sentStatus=503 + sentBody="fault filter abort" 18 bytes + sentHeaders=OrderedHeaders{Content-Type: text/plain}), `TestDecodeHeaders_AbortOnly_0Percent` (0% → Continue + no SendLocalReply via rollPercent's p<=0 short-circuit), `TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName` ("X-FAULT-ON: yes" matches matcher (name="x-fault-on", exact="yes") via canonicalization-on-both-sides), `TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue` ("x-fault-on: YES" against matcher exact "yes" → no match → Continue per §11.8 conclusion (b)), `TestDecodeHeaders_NoFaultHeaderMismatch` (empty request headers + non-empty matcher → no match → Continue), `TestDecodeHeaders_AbortStatRecorded` (100% abort fires → registry walked + http.ingress_http.fault.aborts_injected counter == 1 via `strconv.ParseInt(m.Format(), 10, 64)`). New imports: `net/http`, `strconv`, `sync/atomic`. Step 2 confirmed 3 of 6 tests fail (TestDecodeHeaders_AbortOnly_100Percent, TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName, TestDecodeHeaders_AbortStatRecorded) with the expected error signatures (`status: got 0, want StopIteration`, `sentStatus: got 0, want 503`, `aborts_injected: got 0, want 1`); 3 coincidentally PASS because the Task-3 stub returns Continue and the "no fault expected" assertions match — those will continue to PASS post-implementation. Step 3 replaced `DecodeHeaders` body in `fault.go` with the abort-only path per SPEC §6.4 (matchesHeaders gate → percentage rolls → max_active_faults placeholder comment for Task 6 → combined/delay-only placeholder Continues for Task 5 → abort-only `recordFaultEvent(eventAbortsInjected) + dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}) + return StopIteration`). Added 4 helpers: `matchesHeaders(headers, cfg) bool` (empty matchHeaders = match-all; non-empty requires ALL pairs match via `headers.Get(hm.name) != hm.exactValue` — case-insensitive name via `http.Header.Get`'s canonicalization + parse-time `http.CanonicalHeaderKey`; case-sensitive byte-equal value); `rollPercent(p float64) bool` (p<=0 → false short-circuit; p>=100 → true short-circuit; intermediate `f.rng.Float64()*100 < p` consulting per-instance RNG only — no RNG access at the boundary 0/100 percentages preserves determinism + isolates RNG to the dispatch goroutine per ADR-0102); `faultEventKind` enum + 5 constants (`eventAbortsInjected`, `eventDelaysInjected`, `eventFaultsOverflow`, `eventActiveFaultsInc`, `eventActiveFaultsDec`); `recordFaultEvent(k faultEventKind)` (consolidates stat dispatch per planner-time decision 3; nil-tolerant per ADR-0085 — `if f.stats == nil` early return + per-counter `if f.stats.X != nil` nil-guards on each switch arm); `decrementActive()` markedActive-guarded Dec stub (Task 6 wires the Inc side; the helper is `nolint:unused` because Task 4's abort-only path doesn't call it — abort fires synchronously, no markedActive lifecycle yet). Removed the `nolint:unused` directive from `faultAbortBody` (now consumed by DecodeHeaders). Step 4 confirmed all 13 tests PASS (7 Task-3 + 6 Task-4; 0.004s). Step 5 confirmed `go test -race -count=1 ./internal/filter/http/fault/...` clean (1.012s), `go vet ./...` clean, `golangci-lint run ./internal/filter/http/fault/...` clean, full `go test -race -count=1 -short ./...` PASS across all 30 packages including the 11 differential fixtures unchanged. Step 6 appended ADR-0103 to `docs/envoy-go/DECISIONS.md` per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered (six alternatives A-F) / Consequences (six items a-f)). ADR-0103 anchors the abort terminal-replace mechanics + body byte-exact "fault filter abort" (18 bytes, no trailing newline) + 4-header set on the wire (content-length: 18, content-type: text/plain WITHOUT charset modifier, date: <IMF-fixdate>, server: envoy) + OrderedHeaders carrier discipline (Content-Type override per ADR-0075's SendLocalReply ordered-headers contract) + status-text allow-list narrowed to four canonical codes (200/404/405/503 byte-equal; others code-only) per planner-time decision 7. Cross-references ADR-0075 (SendLocalReply + OrderedHeaders amendment), ADR-0072 (factory validates typed_config at boot — cfg.abortHTTPStatus is PGV-validated, no runtime re-validation), ADR-0085 (nil-tolerance — recordFaultEvent + per-counter nil-guards), ADR-0102 (delay async-resume — Task 5's combined delay+abort path will reuse the same OrderedHeaders carrier from a timer goroutine; sync.Once first-call-wins inside chain.beginLocalReply handles the cross-goroutine entry safely), ADR-0107 (5-stat extension — recordFaultEvent(eventAbortsInjected) is the abort-side Inc call site). Anchors SPEC §5.3 + §6.4 + §6.6 + §11.3 + §11.4 + §11.8. Lands-in-task field reads "Task 4 (phase 09); commit TBD" — SHA-fill follow-up replaces TBD per the 08.2 precedent (PROGRESS.md Task 4 entry's `Commits:` line + DECISIONS.md ADR-0103 Lands-in-task line updated together in the SHA-fill commit). One PLAN-recorded helper (`decrementActive`) lands as a `nolint:unused` stub because Task 4's abort-only synchronous path does not consume it — Task 6 wires the markedActive Inc side from the cap-check insertion point + the OnDestroy/timer-callback Dec sites.
**Outputs:**
```
$ go test ./internal/filter/http/fault/ -v
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
=== RUN   TestDecodeHeaders_AbortOnly_100Percent
--- PASS: TestDecodeHeaders_AbortOnly_100Percent (0.00s)
=== RUN   TestDecodeHeaders_AbortOnly_0Percent
--- PASS: TestDecodeHeaders_AbortOnly_0Percent (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue (0.00s)
=== RUN   TestDecodeHeaders_NoFaultHeaderMismatch
--- PASS: TestDecodeHeaders_NoFaultHeaderMismatch (0.00s)
=== RUN   TestDecodeHeaders_AbortStatRecorded
--- PASS: TestDecodeHeaders_AbortStatRecorded (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.004s
$ go test -race -count=1 ./internal/filter/http/fault/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.012s
$ go vet ./...
$ golangci-lint run ./internal/filter/http/fault/...
```

## Task 5 — Delay async-resume + combined delay+abort timer-callback path [ADR-0102]

**Commits:** 2ec1507 — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 5 Steps 1–7. Step 1 added 4 new tests: `TestDecodeHeaders_DelayOnly` (delay 100% 50ms → DecodeHeaders returns StopIteration synchronously; waitForCondition polls dcb.continued.Load() > 0 within 500ms; elapsed ∈ [40ms, 200ms] per §11.2 conclusion (c); dcb.Status() == 0 confirms NO SendLocalReply on the delay-only path), `TestDecodeHeaders_Combined` (delay 100% 50ms + abort 100% 503 → StopIteration synchronously; waitForCondition polls dcb.Status() != 0 within 500ms; elapsed >= 40ms; dcb.Status() == 503 + dcb.Body() == "fault filter abort" + dcb.continued.Load() == 0 confirms timer-callback called SendLocalReply NOT ContinueDecoding per ADR-0102 + §11.3 empirical pin), `TestDecodeHeaders_DelayStatRecorded` (100% delay → http.ingress_http.fault.delays_injected counter == 1, Inc fires synchronously on the dispatch goroutine before the timer is scheduled), `TestDecodeHeaders_CombinedStatsRecorded` (combined path → delays_injected == 1 synchronously + aborts_injected == 1 from the timer-callback goroutine after waitForCondition). New helpers: `counterValue(t, reg, name)` (mirrors internal/filter/http/router/router_test.go's helper per the broader project precedent for stat-counter assertions across the package boundary; r.Walk + (*stats.Counter).Load() + returns -1 on missing), `makeDelayFilter(t, reg, delayMs, abortStatus)` (constructs delay-only or combined filter; abortStatus=0 → delay-only, non-zero → combined), `waitForCondition(deadline, fn)` (polls fn at 2ms intervals until it returns true or the deadline elapses; avoids tight-CPU-spin while waiting on time.AfterFunc-driven async resume). New imports: `sync` (for the recordingDCB.mu sync.Mutex). Step 2 confirmed all 4 new tests fail with the expected signatures (`status: got 0, want StopIteration`, `ContinueDecoding never invoked within 500ms`, `SendLocalReply never invoked within 500ms; sentStatus=0`, `delays_injected: got 0, want 1`) — placeholders return Continue so the timer is never scheduled. Step 3 replaced both Continue placeholders in DecodeHeaders with `time.AfterFunc`-driven timer paths per the PLAN-prescribed snippet: combined path Inc's delays_injected synchronously then schedules `f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() { f.recordFaultEvent(eventAbortsInjected); f.dcb.SendLocalReply(cfg.abortHTTPStatus, faultAbortBody, OrderedHeaders{{Content-Type, text/plain}}); f.decrementActive() })` then returns StopIteration; delay-only path Inc's delays_injected synchronously then schedules `f.delayTimer = time.AfterFunc(cfg.delayFixedDelay, func() { f.dcb.ContinueDecoding(); f.decrementActive() })` then returns StopIteration. Removed the `nolint:unused` directive on `f.delayTimer` (Task 5 consumes; Task 6 cancels in OnDestroy); the `markedActive` field retains its `nolint:unused` until Task 6. Step 4 confirmed the new tests fail with race-detector enabled — the recordingDCB stub's `sentStatus`/`sentBody`/`sentHeaders` fields were unsynchronized plain int/string values, written from the timer goroutine and read from the test goroutine. Refactored recordingDCB to add a `sync.Mutex` (`mu`) guarding the (status, body, headers) triple + Status()/Body()/Headers() accessor methods; updated SendLocalReply to take the lock; updated all 5 existing Task-4 tests + 4 new Task-5 tests to use the accessors instead of touching the unexported fields. The fault filter's production path itself was race-free (RNG access stays on the dispatch goroutine per planner-time decision 12; *stats.Counter.Inc is goroutine-safe per ADR-0061; *time.Timer assignment to f.delayTimer happens-before timer.Stop() via OnDestroy's sequencing) — the race was strictly in the test stub's plain-field writes vs. polling-goroutine reads. Step 5 confirmed `go test -race -count=1 ./internal/filter/http/fault/...` clean (1.167s) + `go test -race -count=1 -short ./...` clean across all 30 packages including the 11 differential fixtures unchanged + `go vet ./...` clean + `golangci-lint run ./...` clean. Step 6 appended ADR-0102 to `docs/envoy-go/DECISIONS.md` per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered (seven alternatives A-G) / Consequences (seven items a-g)). ADR-0102 anchors the time.AfterFunc-driven async-resume mechanism + combined delay+abort timer-callback decision (timer fires; callback calls SendLocalReply NOT ContinueDecoding) + cancel-on-OnDestroy mechanics (Task 6 anchor) + ±10ms timing tolerance per §11.2 conclusion (c). Cross-references ADR-0071 (single-goroutine-per-stream + chain discipline; cross-goroutine callback entry handled by chain's per-stream synchronization), ADR-0075 (SendLocalReply enters encode at filter[len-1] + OrderedHeaders + sync.Once first-call-wins guard handles cross-goroutine entry), ADR-0103 (combined-path callback reuses the same OrderedHeaders carrier as the synchronous abort path; wire response is byte-equivalent to abort-only just delayed), ADR-0105 (Task-6 forward reference for cancel-on-OnDestroy + markedActive Inc-side wiring), ADR-0107 (recordFaultEvent dispatch for delays_injected/aborts_injected). Anchors SPEC §5.2 + §5.4 + §6.4 + §6.5 (deferred to Task 6) + §11.2 + §11.3 + §14.1. Lands-in-task field reads "Task 5 (phase 09); commit TBD" — SHA-fill follow-up replaces TBD per the 08.2 precedent (PROGRESS.md Task 5 entry's `Commits:` line + DECISIONS.md ADR-0102 Lands-in-task line updated together in the SHA-fill commit). Task-5 timer-callback's `f.decrementActive()` call site is a Task-6 forward reference; at Task 5's commit point `decrementActive()` is the no-op stub from Task 4 (the `f.markedActive` field is always false because Task 6 hasn't wired the Inc side yet) — the Task-5 code compiles and runs against the stub; Task 6 lights up the Inc side without changing Task 5's call sites.
**Outputs:**
```
$ go test ./internal/filter/http/fault/ -v
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
=== RUN   TestDecodeHeaders_AbortOnly_100Percent
--- PASS: TestDecodeHeaders_AbortOnly_100Percent (0.00s)
=== RUN   TestDecodeHeaders_AbortOnly_0Percent
--- PASS: TestDecodeHeaders_AbortOnly_0Percent (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue (0.00s)
=== RUN   TestDecodeHeaders_NoFaultHeaderMismatch
--- PASS: TestDecodeHeaders_NoFaultHeaderMismatch (0.00s)
=== RUN   TestDecodeHeaders_AbortStatRecorded
--- PASS: TestDecodeHeaders_AbortStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_DelayOnly
--- PASS: TestDecodeHeaders_DelayOnly (0.05s)
=== RUN   TestDecodeHeaders_Combined
--- PASS: TestDecodeHeaders_Combined (0.05s)
=== RUN   TestDecodeHeaders_DelayStatRecorded
--- PASS: TestDecodeHeaders_DelayStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_CombinedStatsRecorded
--- PASS: TestDecodeHeaders_CombinedStatsRecorded (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.158s
$ go test -race -count=1 ./internal/filter/http/fault/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.167s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
```

## Task 6 — max_active_faults atomic counter + markedActive idempotency guard + OnDestroy timer-cancel + race-detector cycle test [ADR-0105]

**Commits:** b2174fd — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 6 Steps 1–7. Step 1 added 3 new tests to `internal/filter/http/fault/fault_test.go`: `TestDecodeHeaders_MaxActiveFaultsCapOverflow` (delay 100% 200ms, max_active_faults=1; allocate 2 instances from same factory sharing `*atomic.Int64` counter; first DecodeHeaders → StopIteration, second → Continue, faults_overflow == 1), `TestOnDestroy_TimerStopped` (delay 100% 500ms; DecodeHeaders schedules timer; OnDestroy cancels before fire; sleep 100ms; assert dcb.continued.Load() == 0; assert fl.active.Load() == 0 — Inc balanced by Dec), `TestFault_DelayTimerRace` (race-cycle test under -race per planner-time decision 10; -short skipped; loop 100 iterations with 1ms delay; each iteration: factory() → DecodeHeaders → sleep i%2 ms → OnDestroy; forces OnDestroy to race the timer-callback). Step 2 confirmed `TestDecodeHeaders_MaxActiveFaultsCapOverflow` fails with the expected signatures (`second request: got 1, want Continue (cap should skip fault)` + `faults_overflow: got 0, want 1` — the StopIteration value is 1, observed because Task 5's placeholder doesn't insert a cap check). The other two tests passed coincidentally pre-implementation (TestOnDestroy_TimerStopped because Task 4's decrementActive is a no-op stub when markedActive is false + the 500ms timer hadn't fired in the 100ms test window; TestFault_DelayTimerRace because there was no markedActive RMW to race on — Task 5's timer callback called the no-op decrementActive). Step 3 implemented per PLAN snippet: inserted the cap check in `DecodeHeaders` after the `delayApplies/abortApplies` short-circuit and before the dispatch branches (`if cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults { recordFaultEvent(eventFaultsOverflow); return Continue }; f.markActive()`); added the `markActive` helper (Inc the *atomic.Int64 + set markedActive + record eventActiveFaultsInc); replaced OnDestroy stub with `if f.delayTimer != nil { _ = f.delayTimer.Stop() }; f.decrementActive()`; removed `nolint:unused` directives from `markedActive` field doc-comment (Task 6 consumes via markActive) and from `decrementActive` (Task 6 consumes via OnDestroy + timer-callback). Step 4 confirmed all 18 tests PASS (15 Tasks 3–5 + 3 new Task-6 tests; 0.314s without -race). Step 5 ran `go test -race -count=10 ./internal/filter/http/fault/...` and IT FAILED — race detector flagged the plain-bool `markedActive` field's RMW between the timer-callback goroutine (DecodeHeaders.func2 calling decrementActive at fault.go:397 via timer goroutine) and the OnDestroy goroutine (test goroutine calling decrementActive at fault.go:396). The PLAN's claim "race-clean by single-goroutine-per-stream invariant per ADR-0071" was inaccurate — `time.AfterFunc(d, fn)` runs `fn` on a runtime-managed goroutine, NOT on the dispatch goroutine; the OnDestroy and timer-callback Decs genuinely race during chain teardown when `delayTimer.Stop()` returns false. Per the implementer prompt's explicit refactoring branch: upgraded `markedActive bool` → `markedActive atomic.Bool` and changed decrementActive to use `f.markedActive.CompareAndSwap(true, false)` for race-clean exactly-once Dec; markActive uses `f.markedActive.Store(true)` (the Inc side runs only on the dispatch goroutine after the cap check, so a plain Store is sufficient — but atomic.Bool's Store + CAS pair is the type-correct race-clean form). Re-ran `go test -race -count=10 ./internal/filter/http/fault/...` and got `ok ... 4.126s` clean across all 10 iterations. ADR-0105 §Alternatives (A) records the empirical-evidence-driven decision to use atomic.Bool over plain bool. ADR-0105 §Consequences (b) records that future maintenance MUST preserve the atomic.Bool form. Step 6 appended ADR-0105 to `docs/envoy-go/DECISIONS.md` per the ADR-0001 template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered (seven alternatives A-G) / Consequences (seven items a-g)). ADR-0105 anchors max_active_faults concurrency cap + LBP-1 sixth application + closure-captured *atomic.Int64 shared counter + markedActive atomic.Bool per-instance idempotency guard (UPGRADED from plain bool per empirical race-detector evidence) + OnDestroy timer-cancel discipline + faults_overflow stat semantics. Cross-references ADR-0072 (HTTPRegistry — LBP-1 first), ADR-0079 (ListenerFilterRegistry — LBP-1 second), ADR-0061 (stats Registry — LBP-1 third), ADR-0091 (drain Manager — LBP-1 fourth), ADR-0078 (ChainBuilder closure-capture — LBP-1 fifth), ADR-0071 (single-goroutine-per-stream invariant — REFINED: governs dispatch goroutine + RNG + markActive; does NOT govern OnDestroy/timer-callback Dec — those genuinely straddle goroutines and need atomic.Bool CAS), ADR-0102 (timer mechanics — cancel-on-OnDestroy promise lights up here), ADR-0107 (5-stat extension — recordFaultEvent dispatches eventFaultsOverflow + eventActiveFaultsInc + eventActiveFaultsDec). Anchors SPEC §5.6 + §5.7 + §6.4 + §6.5 + §14.1. Lands-in-task field reads "Task 6 (phase 09); commit TBD" — SHA-fill follow-up replaces TBD per the 08.2 precedent (PROGRESS.md Task 6 entry's `Commits:` line + DECISIONS.md ADR-0105 Lands-in-task line updated together in the SHA-fill commit). markedActive bool vs atomic.Bool note: **atomic.Bool was needed**; the plain-bool form failed `-race -count=10` on the first invocation; the atomic.Bool form is clean.
**Outputs:**
```
$ go test ./internal/filter/http/fault/ -v
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
=== RUN   TestDecodeHeaders_AbortOnly_100Percent
--- PASS: TestDecodeHeaders_AbortOnly_100Percent (0.00s)
=== RUN   TestDecodeHeaders_AbortOnly_0Percent
--- PASS: TestDecodeHeaders_AbortOnly_0Percent (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue (0.00s)
=== RUN   TestDecodeHeaders_NoFaultHeaderMismatch
--- PASS: TestDecodeHeaders_NoFaultHeaderMismatch (0.00s)
=== RUN   TestDecodeHeaders_AbortStatRecorded
--- PASS: TestDecodeHeaders_AbortStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_DelayOnly
--- PASS: TestDecodeHeaders_DelayOnly (0.05s)
=== RUN   TestDecodeHeaders_Combined
--- PASS: TestDecodeHeaders_Combined (0.05s)
=== RUN   TestDecodeHeaders_DelayStatRecorded
--- PASS: TestDecodeHeaders_DelayStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_CombinedStatsRecorded
--- PASS: TestDecodeHeaders_CombinedStatsRecorded (0.05s)
=== RUN   TestDecodeHeaders_MaxActiveFaultsCapOverflow
--- PASS: TestDecodeHeaders_MaxActiveFaultsCapOverflow (0.00s)
=== RUN   TestOnDestroy_TimerStopped
--- PASS: TestOnDestroy_TimerStopped (0.10s)
=== RUN   TestFault_DelayTimerRace
--- PASS: TestFault_DelayTimerRace (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.314s
$ go test -race -count=10 ./internal/filter/http/fault/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	4.126s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.487s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.016s
ok  	github.com/esalaine/envoy-go/internal/admin	1.527s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.080s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.078s
ok  	github.com/esalaine/envoy-go/internal/drain	1.141s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.078s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.537s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.164s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.035s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.063s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.293s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.265s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.208s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.085s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.068s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.036s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.046s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.107s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.146s
ok  	github.com/esalaine/envoy-go/test/differential	1.143s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.024s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.047s
```

**Task 6 follow-up (post-review):** ADR-0105 §Context + §Consequences (e) corrected the LBP-1 chain: per code-quality review, the original chain incorrectly named ADR-0061 (which is stat-name-flattening, not LBP-1), fabricated an ADR-0078 reference (which is ADR-0033 partial supersession, no LBP-1 content), and omitted ADR-0085 (LBP-1 fourth application). The corrected chain (ADR-0059 → 0072 → 0079 → 0085 → 0091 → 0105 sixth) matches ADR-0091's own self-numbering at DECISIONS.md:3618. Commit f43d3fc.

## Task 7 — Per-route 3-tier merge (routeConfigOrListener + parseRouteRuntimeConfig) [ADR-0073 cross-ref via ADR-0101 §Consequences]

**Commits:** aefb3d0 — this task's commit
**Notes:** Strict-TDD per PLAN.md Task 7 Steps 1–6. Step 1 added `TestPerRouteWholesaleOverride` to `internal/filter/http/fault/fault_test.go`: listener-level `&faultv3.HTTPFault{Delay: 100% 200ms}` (no abort); per-route `&faultv3.HTTPFault{Abort: 100% 418}` (no delay); construct factory from listener config; allocate filter; attach `&recordingDCB{routeCfg: routeCfg}` (the recordingDCB.RequestRouteConfig method added in Task 4 returns `r.routeCfg`); call `fl.DecodeHeaders(http.Header{}, true)` and capture elapsed via `time.Since(start)`; assert StopIteration + `dcb.Status() == 418` + `elapsed < 50ms` (no inherited 200ms delay — wholesale-override per §11.7). Used the mutex-guarded `dcb.Status()` accessor rather than direct `dcb.sentStatus` field access for race-detector cleanliness consistency with Tasks 4–6 patterns. Step 2 confirmed the test fails as designed: `sentStatus: got 0, want 418 (per-route override)` — DecodeHeaders was using `f.cfg` (listener-level only with delay-no-abort), so the synchronous DecodeHeaders return parked at StopIteration (delay matched + scheduled the timer) but the timer hadn't fired by the time the test asserted on `dcb.Status()`, leaving sentStatus at zero. Step 3 implemented per PLAN snippet: added `routeConfigOrListener` method (nil-dcb fallback → cb.RequestRouteConfig nil-fallback → type-assertion guard on `*faultv3.HTTPFault` with defensive fall-through → `parseRouteRuntimeConfig` projection with defensive fall-through on parse error); added `parseRouteRuntimeConfig` thin-wrapper around `parseRuntimeConfig` (KEEP-separate per planner-time decision 2 — per-route may diverge from New-time validation in a future deferral); replaced `cfg := f.cfg // Task 7 replaces with f.routeConfigOrListener()` in DecodeHeaders with `cfg := f.routeConfigOrListener()`. Doc-comments on both helpers explain the wholesale-override discipline (NOT field-merge per §11.7) and the planner-time decision 2 KEEP-separate rationale. Step 4 confirmed all 21 tests PASS (20 prior + 1 new). Step 5 ran `go test -race -count=1 ./internal/filter/http/fault/...` and got `ok ... 1.321s` clean. NO new ADR landed — the cross-reference to ADR-0073 (existing 3-tier-merge contract) was already recorded in ADR-0101 §Consequences from Task 3 per the PLAN's pre-anchored map. Gate-(a) suite (`go build` + `go vet` + `golangci-lint run` + `go test -race -count=1 -short ./...`) clean before commit.
**Outputs:**
```
$ go test ./internal/filter/http/fault/ -v
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
=== RUN   TestDecodeHeaders_AbortOnly_100Percent
--- PASS: TestDecodeHeaders_AbortOnly_100Percent (0.00s)
=== RUN   TestDecodeHeaders_AbortOnly_0Percent
--- PASS: TestDecodeHeaders_AbortOnly_0Percent (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName (0.00s)
=== RUN   TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue
--- PASS: TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue (0.00s)
=== RUN   TestDecodeHeaders_NoFaultHeaderMismatch
--- PASS: TestDecodeHeaders_NoFaultHeaderMismatch (0.00s)
=== RUN   TestDecodeHeaders_AbortStatRecorded
--- PASS: TestDecodeHeaders_AbortStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_DelayOnly
--- PASS: TestDecodeHeaders_DelayOnly (0.05s)
=== RUN   TestDecodeHeaders_Combined
--- PASS: TestDecodeHeaders_Combined (0.05s)
=== RUN   TestDecodeHeaders_DelayStatRecorded
--- PASS: TestDecodeHeaders_DelayStatRecorded (0.00s)
=== RUN   TestDecodeHeaders_CombinedStatsRecorded
--- PASS: TestDecodeHeaders_CombinedStatsRecorded (0.05s)
=== RUN   TestDecodeHeaders_MaxActiveFaultsCapOverflow
--- PASS: TestDecodeHeaders_MaxActiveFaultsCapOverflow (0.00s)
=== RUN   TestOnDestroy_TimerStopped
--- PASS: TestOnDestroy_TimerStopped (0.10s)
=== RUN   TestPerRouteWholesaleOverride
--- PASS: TestPerRouteWholesaleOverride (0.00s)
=== RUN   TestFault_DelayTimerRace
--- PASS: TestFault_DelayTimerRace (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.314s
$ go test -race -count=1 ./internal/filter/http/fault/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.321s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.689s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.039s
ok  	github.com/esalaine/envoy-go/internal/admin	1.533s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.077s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.082s
ok  	github.com/esalaine/envoy-go/internal/drain	1.148s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.083s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.519s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.169s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.034s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.065s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.303s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.268s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.206s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.096s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.067s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.035s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.039s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.109s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.158s
ok  	github.com/esalaine/envoy-go/test/differential	1.160s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.025s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.026s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.026s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.043s
```

## Task 8 — cmd/envoy-go/main.go register fault.New under fault.TypeURL [ADR-0100]

**Commits:** 311f363 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical boot-wiring task per PLAN.md Task 8 Steps 1–7. Step 1 inspected the existing http filter import block + Register chain in `cmd/envoy-go/main.go` (router/cors/envoygotest landed in 07.1 Task 20). Step 2 added `"github.com/esalaine/envoy-go/internal/filter/http/fault"` to the http filter import block, sorted alphabetically between `envoygotest` and `router` (gofmt-stable ordering). Step 3 inserted `httpReg.Register(fault.TypeURL, fault.New)` after the `envoygotest` Register and before `httpReg.Freeze()`, preserving the BRAINSTORM Decision 2 router-first-then-alphabetical convention. Step 4 ran the four-gate suite (`go build ./...` + `go vet ./...` + `golangci-lint run ./...` + `go test -race -count=1 -short ./...`) — all four clean; 32 packages PASS unchanged. Step 5 was deliberately SKIPPED per PLAN.md Task 8 Step 5 — the smoke test (crafting a minimal bootstrap and running the binary) is deferred because the differential fixture (Tasks 11–14) exercises the exact end-to-end registration → typed_config resolution → factory invocation path against reference Envoy, and the Task 16 phase-done six-gate verification is a second backstop for boot-wiring regressions. NO new ADR landed — ADR-0100 (boot registration) was anchored in Task 3. Per-package cmd test (`go test -race -count=1 ./cmd/envoy-go/...`) passes confirming the binary still builds and the new Register call wires correctly.
**Outputs:**
```
$ grep -nE 'httpReg|fault' cmd/envoy-go/main.go | head -10
27:	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
30:	"github.com/esalaine/envoy-go/internal/filter/http/fault"
111:	httpReg := filter_http.NewHTTPRegistry()
112:	httpReg.Register(router.TypeURL, router.New)
113:	httpReg.Register(cors.TypeURL, cors.New)
114:	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
115:	httpReg.Register(fault.TypeURL, fault.New)
116:	httpReg.Freeze()
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.711s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.041s
ok  	github.com/esalaine/envoy-go/internal/admin	1.539s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.081s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.075s
ok  	github.com/esalaine/envoy-go/internal/drain	1.152s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.081s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.538s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.166s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.036s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.060s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.296s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.266s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.210s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.085s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.073s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.038s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.048s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.110s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.140s
ok  	github.com/esalaine/envoy-go/test/differential	1.139s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.022s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.029s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.029s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.026s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.026s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.045s
```

## Task 9 — FuzzFaultConfigParse fuzzer (twelfth fuzzer per ADR-0018)

**Commits:** 73c2b08 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical fuzzer-ship task per PLAN.md Task 9 Steps 1–4 + planner-time decision 1 (SHIP). Step 1 wrote `internal/filter/http/fault/fuzz_test.go` verbatim per PLAN.md lines 1975–2019: `FuzzFaultConfigParse` feeds arbitrary byte sequences as the `tc *anypb.Any` Value (TypeURL pinned to `fault.TypeURL`) and asserts the New factory returns either `(factory, nil)` OR `(nil, error)` — never `(nil, nil)`, never both. Seed corpus is the 5 byte sequences from PLAN: nil, empty, `{0x00}`, `{0xff,0xff,0xff,0xff}`, `[]byte("not-a-proto")`. Step 2 ran the 30s fuzz budget — no panics, no `(nil, nil)` returns; corpus expanded from baseline 4 (the 5 dedup'd seeds — nil and `{}` are byte-equivalent under f.Add) to 250 interesting inputs; ~3.36M execs total at peak ~322k/sec. Step 3 ran short-mode (`go test -count=1 -short ./internal/filter/http/fault/`) confirming the seed corpus runs as part of the normal test suite — PASS. Step 4 ran the four-gate suite — `go build ./...` + `go vet ./...` + `golangci-lint run ./...` + `go test -race -count=1 -short ./...` all clean; 33 packages PASS unchanged. NO new ADR landed (ADR-0018 fuzz-CI policy is the anchoring ADR; established phase 04+; this is the twelfth fuzzer in that lineage per PLAN.md Task 9 + planner-time decision 1).
**Outputs:**
```
$ go test -fuzz=FuzzFaultConfigParse -fuzztime=30s ./internal/filter/http/fault/
fuzz: elapsed: 0s, gathering baseline coverage: 0/4 completed
fuzz: elapsed: 0s, gathering baseline coverage: 4/4 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 71331 (23777/sec), new interesting: 106 (total: 110)
fuzz: elapsed: 6s, execs: 486519 (138356/sec), new interesting: 182 (total: 186)
fuzz: elapsed: 9s, execs: 742536 (85355/sec), new interesting: 199 (total: 203)
fuzz: elapsed: 12s, execs: 1245085 (167491/sec), new interesting: 215 (total: 219)
fuzz: elapsed: 15s, execs: 1540717 (98558/sec), new interesting: 225 (total: 229)
fuzz: elapsed: 18s, execs: 1641378 (33546/sec), new interesting: 230 (total: 234)
fuzz: elapsed: 21s, execs: 1742008 (33554/sec), new interesting: 232 (total: 236)
fuzz: elapsed: 24s, execs: 1767753 (8579/sec), new interesting: 232 (total: 236)
fuzz: elapsed: 27s, execs: 2387235 (206544/sec), new interesting: 236 (total: 240)
fuzz: elapsed: 30s, execs: 3355456 (322633/sec), new interesting: 246 (total: 250)
fuzz: elapsed: 31s, execs: 3355456 (0/sec), new interesting: 246 (total: 250)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	31.394s
$ go test -count=1 -short ./internal/filter/http/fault/
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.260s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.627s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.014s
ok  	github.com/esalaine/envoy-go/internal/admin	1.541s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.085s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.084s
ok  	github.com/esalaine/envoy-go/internal/drain	1.146s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.088s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.539s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.170s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.039s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.069s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.304s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.269s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.213s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.094s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.077s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.037s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.050s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.111s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.165s
ok  	github.com/esalaine/envoy-go/test/differential	1.165s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.027s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.029s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.031s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.029s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.028s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.033s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.034s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.050s
```

## Task 10 — BackendKind HTTPFault enum + startHTTPFaultBackend spawn helper

**Commits:** 8366980 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical fixture-infrastructure task per PLAN.md Task 10 Steps 1–2 (Step 3 build/test gate, Step 4 commit). Step 1 added `HTTPFault BackendKind = 8` to `test/differential/fixture/fixture.go` after the existing `HTTPSlowStream BackendKind = 7`, with the doc-comment per PLAN snippet (deterministic-body backend serving `/` → `"backend\n"` (8 bytes), no TLS, subprocess thus no in-process accept counter). Step 2 added two pieces to `test/differential/runner_test.go`: (a) `startHTTPFaultBackend(ctx, repoRoot, port)` helper after `startHTTPSlowStreamBackend`, mirroring the `exec.CommandContext("go", "run", "./test/fixtures/0011-http-fault/backends", "--port", …)` + `cmd.Dir = repoRoot` + Stdout/Stderr → os.Stderr + `Setpgid: true` + `Start()` pattern; (b) `case fixture.HTTPFault:` in the runFixture switch immediately after `case fixture.HTTPSlowStream:`, mirroring the freeTCPPort + bo.port = port + start backend + bo.proc = cmd + defer-SIGKILL-process-group + waitTCPDial(5s) shape. Per PLAN's revised step 2(c), the blank-import for the driver package is DEFERRED to Task 14 (driver package doesn't exist until Task 14, so adding the blank-import now would break `go build`). Step 3 ran the four-gate suite — `go build ./...` + `go vet ./...` + `golangci-lint run ./...` + `go test -race -count=1 -short ./...` all clean; 33 packages PASS unchanged; the runner correctly does NOT see fixture 0011-http-fault yet (no fixture dir created until Tasks 11+). NO new ADR.
**Outputs:**
```
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -count=1 -short ./test/differential/...
ok  	github.com/esalaine/envoy-go/test/differential	0.086s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.001s
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.734s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.029s
ok  	github.com/esalaine/envoy-go/internal/admin	1.528s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.077s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.074s
ok  	github.com/esalaine/envoy-go/internal/drain	1.135s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.077s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.512s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.161s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.027s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.054s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.293s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.259s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.198s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.092s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.065s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.026s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.043s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.104s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.137s
ok  	github.com/esalaine/envoy-go/test/differential	1.112s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.013s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.023s
```

## Task 11 — Fixture 0011 backends/backend.go (Go HTTP backend serving `backend\n`)

**Commits:** d4aa744 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical fixture-ship task per PLAN.md Task 11 Steps 1–4 + planner-time decision 11 (path). Step 1 wrote `test/fixtures/0011-http-fault/backends/backend.go` verbatim per PLAN snippet (24 LoC; gofmt'd to tab indent — Go canonical): `package main` Go HTTP backend with `--port` flag (default 18001), single `/` handler returning `200 OK` with `Content-Type: text/plain` + explicit `Content-Length: 8` + body `"backend\n"` (8 bytes; matches the §11 empirical-pin backend used during phase 09 SPEC drafting). The parent dir `test/fixtures/0011-http-fault/` did not exist prior to this task — Task 11 creates both the fixture-root and the `backends/` subdir. Step 2 ran `go build ./test/fixtures/0011-http-fault/backends/...` — clean. Step 3 ran the manual smoke test: `go run ./test/fixtures/0011-http-fault/backends --port 18001 &; sleep 1; curl -sS -i http://127.0.0.1:18001/; kill %1` — confirmed `HTTP/1.1 200 OK` + `Content-Length: 8` + `Content-Type: text/plain` + body `backend\n` (hex `62 61 63 6b 65 6e 64 0a`, exactly 8 bytes including the trailing LF). Step 4 ran the four-gate suite — `go build ./...` + `go vet ./...` + `golangci-lint run ./...` + `go test -race -count=1 -short ./...` all clean; 33 packages PASS unchanged plus the new `test/fixtures/0011-http-fault/backends` package now visible as `[no test files]` (consistent with all other fixture `backends/` packages). NO new ADR.
**Outputs:**
```
$ go build ./test/fixtures/0011-http-fault/backends/...
$ go run ./test/fixtures/0011-http-fault/backends --port 18001 &
$ sleep 1; curl -sS -i http://127.0.0.1:18001/
HTTP/1.1 200 OK
Content-Length: 8
Content-Type: text/plain
Date: Sun, 03 May 2026 23:46:31 GMT

backend
$ curl -sS http://127.0.0.1:18001/ | xxd
00000000: 6261 636b 656e 640a                      backend.
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	4.7s (unchanged)
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.0s
ok  	github.com/esalaine/envoy-go/internal/admin	1.5s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.083s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.084s
ok  	github.com/esalaine/envoy-go/internal/drain	1.146s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.083s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.538s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.165s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.034s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.064s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.305s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.270s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.213s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.090s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.067s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.036s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.046s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.109s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	1.153s
ok  	github.com/esalaine/envoy-go/test/differential	1.152s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.034s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.031s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.035s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.030s
?   	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.033s
?   	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.026s
?   	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.027s
?   	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.047s
```

## Task 12 — Fixture 0011 envoy.yaml + envoy-go.yaml bootstraps per SPEC §7.4

**Commits:** e5fbd56 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical fixture-ship task per PLAN.md Task 12 Steps 1–4 + SPEC §7.4 verbatim YAML + planner-time decision 8 (STRICT_DNS) + ADR-0010 (V4_ONLY). Step 1 wrote `test/fixtures/0011-http-fault/envoy.yaml` (reference Envoy bootstrap, 80 lines) verbatim per PLAN snippet (lines 2246–2325): admin :9902 + listener :10001 (literal in-container ports for the reference Envoy container per planner-time decision; the runner publishes to allocated host ports), HCM with `codec_type: HTTP1` + `stat_prefix: ingress_http` + 5 routes (`/scenario1` no-fault, `/scenario2` per-route delay+abort 100% 100ms+503, `/scenario3-wholesale` per-route abort-only 418 (NO delay — wholesale-override case), `/scenario3-baseline` no-fault (inherits listener delay), `/scenario4` per-route abort 503 + headers `x-fault-on: yes`) + 2 http_filters (listener-level fault: delay 100% 100ms only NO abort; router) + cluster `c_backend` STRICT_DNS V4_ONLY ROUND_ROBIN connect_timeout 0.25s with `{{.BackendHost}}:{{.BackendPort}}` Go-template tokens for runtime substitution by the Task 14 driver. Step 2 wrote `test/fixtures/0011-http-fault/envoy-go.yaml` (subject envoy-go bootstrap, 80 lines) — identical body modulo admin/listener ports which are `{{.AdminPort}}`/`{{.ListenerPort}}` Go-template tokens (rendered to runner-allocated host ports by the Task 14 driver). Step 3 verified YAML structural validity via Python `yaml.safe_load` after manual template substitution (BackendHost=127.0.0.1, BackendPort=18001 for envoy.yaml; +AdminPort=9901, ListenerPort=10000 for envoy-go.yaml) — both parse cleanly; structural assertions confirmed: 5 routes with correct match prefixes, per-route fault config presence/absence per spec (s1/s3-baseline=False; s2/s3-wholesale/s4=True), s2 has BOTH delay+abort, s3-wholesale has abort-only http_status=418 (NO delay → wholesale-override), s4 has abort 503 + headers x-fault-on exact "yes", listener-level fault has delay-only fixed_delay=0.1s (NO abort), cluster STRICT_DNS V4_ONLY ROUND_ROBIN, endpoint resolves to 127.0.0.1:18001. The PLAN's optional Step 3 Alternative smoke test (`go run ./cmd/envoy-go -c /tmp/rendered.yaml` + curl scenario1) was attempted and failed because the current envoy-go cluster manager only supports STATIC clusters (`cluster manager: cluster: "c_backend": only STATIC clusters supported; got STRICT_DNS`); this is an upstream gap orthogonal to Task 12 — SPEC §7.4 mandates STRICT_DNS verbatim and the Task 14 driver will handle resolution via the harness backend hostname per planner-time decision 8. Step 4 ran the four-gate suite: `go build ./...` + `go vet ./...` + `golangci-lint run ./...` all clean (YAMLs are not Go); `go test -race -count=1 -short ./...` initially flaked once on `TestServerStream_StateTransitions_HeadersThenData` in `internal/filter/hcm/h2` (unrelated to YAML changes — no Go modified) and PASSED on retry (`-count=3`). NO new ADR.
**Outputs:**
```
$ python3 -c "import yaml; yaml.safe_load(open('test/fixtures/0011-http-fault/envoy.yaml').read().replace('{{.BackendHost}}','127.0.0.1').replace('{{.BackendPort}}','18001'))"
envoy.yaml: parsed OK; admin port= 9902
listener port= 10001
routes count= 5
  /scenario1 per-route-fault= False
  /scenario2 per-route-fault= True
  /scenario3-wholesale per-route-fault= True
  /scenario3-baseline per-route-fault= False
  /scenario4 per-route-fault= True
http_filters= ['envoy.filters.http.fault', 'envoy.filters.http.router']
cluster= c_backend STRICT_DNS ROUND_ROBIN
endpoint= {'address': '127.0.0.1', 'port_value': 18001}
$ python3 -c "...envoy-go.yaml after AdminPort=9901+ListenerPort=10000+BackendHost=127.0.0.1+BackendPort=18001 substitution..."
envoy-go.yaml: parsed OK; admin port= 9901
listener port= 10000
routes count= 5
  /scenario1 per-route-fault= False
  /scenario2 per-route-fault= True
  /scenario3-wholesale per-route-fault= True
  /scenario3-baseline per-route-fault= False
  /scenario4 per-route-fault= True
http_filters= ['envoy.filters.http.fault', 'envoy.filters.http.router']
s2 has delay= True abort= True headers= False
s3-wholesale has delay= False abort= True http_status= 418
s4 has delay= False abort= True headers= [{'name': 'x-fault-on', 'string_match': {'exact': 'yes'}}]
listener fault has delay= True abort= False fixed_delay= 0.1s
$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.300s
ok  	(33 packages PASS unchanged; one transient flake in internal/filter/hcm/h2 TestServerStream_StateTransitions_HeadersThenData unrelated to YAML; PASSED on retry with -count=3)
```

Task 12 follow-up: smoke test revealed envoy-go's cluster manager only supports STATIC clusters (not STRICT_DNS). Updated envoy-go.yaml to type: STATIC with literal 127.0.0.1 (port still templated). The reference envoy.yaml retains STRICT_DNS + host.docker.internal per ADR-0010. The planner-time decision 8 wording in PLAN.md was incomplete (should have specified the split per 0007a-cors precedent); the corrected disposition is now: reference Envoy uses STRICT_DNS in Docker (host.docker.internal); envoy-go subject uses STATIC with 127.0.0.1. Commit 2d0cf9a.

## Task 13 — Fixture 0011 expectations.yaml + README.md

**Commits:** 8c5756a — this task's commit; TBD — SHA-fill follow-up
**Notes:** Mechanical docs-only task per PLAN.md Task 13 Steps 1–3 (prose docs only; no Go code; per ADR-0019 expectations.yaml is documentation, not machine-evaluated — the Task 14 driver enforces). Step 1 wrote `test/fixtures/0011-http-fault/expectations.yaml` verbatim per PLAN snippet (lines 2464–2551): per-scenario equivalence claims for the 4 scenarios per SPEC §7.1 (scenario1 listener-inheritance delay-only, scenario2 combined delay+abort 503, scenario3a wholesale-override 418 NO inherited delay, scenario3b baseline NO per-route override, scenario4 headers-field exact-match gate with 4 sub-probes a/b/c/d). Each scenario documents `status` + `body_byte_equal` + `time_total_ms_min/max` + `stat_deltas` per §11.6 5-stat verification; scenario2 also lists the 4-header set per §11.3 / ADR-0103 (content-length=18, content-type=text/plain NO charset modifier, date allow-listed, server=envoy); scenario3a explicitly marks `status_text_allow_listed: true` per planner-time decision 7. Step 2 wrote `test/fixtures/0011-http-fault/README.md` per PLAN snippet (lines 2556–2619) WITH the Task 12 follow-up amendment incorporated: the "Bootstrap discipline" Cluster line replaced "STRICT_DNS pointing at the backend hostname (per planner-time decision 8 + ADR-0010); reference container resolves via host.docker.internal; subject resolves via Go's net resolver." with "reference Envoy uses STRICT_DNS pointing at host.docker.internal (per ADR-0010); envoy-go subject uses STATIC pointing at 127.0.0.1 (envoy-go's cluster manager only supports STATIC, per the Task 12 follow-up amendment to planner-time decision 8)." A new bottom paragraph "Cluster type (Task 12 follow-up amendment)" documents the rationale: original PLAN's planner-time decision 8 claimed STRICT_DNS would work for both proxies; the Task 12 smoke test revealed envoy-go's cluster manager only supports STATIC; the fixture's envoy-go.yaml was amended to type: STATIC with literal 127.0.0.1; the reference envoy.yaml retains STRICT_DNS + host.docker.internal for Docker-network resolution; differential parity preserved because both proxies dial the same backend port (only resolution path differs). The README also documents: 4 equivalence-claim scenarios, status-text allow-list disposition (planner-time decision 7 — 418 emits "Unknown" on Envoy vs "I'm a teapot" on stdlib — code-only equivalence for non-stdlib codes; standard codes 200/503/404/405 byte-equal on code AND text), twin-stat-series allow-list (`fault.response_rl_injected` permanently-zero per ADR-0107 route A; documentation-sense allow-list), SIGTERM behavior (phase 09 introduces no SIGTERM divergence; fixture does NOT exercise drain), cross-references to SPEC §7.1/§11.1/§11.2/§11.3/§11.5/§11.6/§11.7/§11.8 + ADR-0103/0102/0107/0073/0010. Step 3 verified YAML syntax via `python3 -c "import yaml; yaml.safe_load(open('test/fixtures/0011-http-fault/expectations.yaml').read())"` — clean parse. Gate-(a) `go build ./...` — clean (docs are not Go). NO new ADR.
**Outputs:**
```
$ python3 -c "import yaml; yaml.safe_load(open('test/fixtures/0011-http-fault/expectations.yaml').read())"
YAML OK
$ go build ./...
```

## Task 14 — Fixture 0011 driver/driver.go + StatsAsserter (4-scenario orchestration, 8 probes)

**Commits:** 1550c9c — this task's commit; TBD — SHA-fill follow-up
**Notes:** This task lights up the differential gate for fixture 0011-http-fault end-to-end. Step 1 wrote `test/fixtures/0011-http-fault/driver/driver.go` (~330 LoC) per PLAN.md Task 14 skeleton (lines 2650–2802) + 0007a-cors / 0010-graceful-drain driver precedents: implements `fixture.Driver` (BackendCount=1 + SubjectListenerName="l_main" + ReferenceListenerPort=10001 + ReferenceBootstrap/SubjectConfig that render the fixture YAMLs via `text/template` with runtime-substituted ports + DriveReference/DriveSubject that issue the same 8-probe sequence and emit a deterministic per-probe assertion-log byte stream + ProbeAdmin issuing `GET /ready` against both admin endpoints) + the optional `fixture.BackendKindAware` (returning HTTPFault) + `fixture.StatsAsserter` (scrapes `/stats/prometheus` + asserts the 5 fault.* counters per SPEC §7.1 final-state matrix: `aborts_injected=4` for scenarios 2/3a/4b/4c, `delays_injected=3` for scenarios 1/2/3b, `faults_overflow=0`, `active_faults=0` final, `response_rl_injected=0` permanently per ADR-0107). The 8-probe sequence: scenario1 → /scenario1/anything (no headers) → expect 200+backend+delayed; scenario2 → /scenario2/anything → expect 503+abort-body+delayed (combined delay+abort); scenario3-wholesale → /scenario3-wholesale/anything → expect 418+abort-body+fast (wholesale-override demo, NO inherited delay); scenario3-baseline → /scenario3-baseline/anything → expect 200+backend+delayed (listener-level inheritance); scenario4-a → /scenario4 (no header) → expect 200+backend+fast (no match); scenario4-b → /scenario4 (x-fault-on: yes) → expect 503+abort+fast (match); scenario4-c → /scenario4 (X-FAULT-ON: yes) → expect 503+abort+fast (case-insensitive name); scenario4-d → /scenario4 (x-fault-on: YES) → expect 200+backend+fast (case-sensitive value mismatch). Per-probe log: `probe <id> status=<code> body=<quoted> elapsed=<bucket>` where bucket="fast" if elapsed<80ms else "delayed" (planner-time decision 11 threshold). Status TEXT intentionally excluded from logs per planner-time decision 7 (allow-listed for non-stdlib codes like 418). Helpers added per the 0004-h2 / 0010-graceful-drain precedents: `fixtureDir()` via `runtime.Caller(0)`; `mustReadFixtureFile`/`mustRender` (text/template); `httpProbe` (helpers.HTTPRoundTrip wrapper); `scrapeFaultStats`/`parseFaultPromBody` (Prometheus exposition parser keyed on `envoy_http_fault_<n>{envoy_http_conn_manager_prefix=ingress_http}` per SPEC §11.6). Compile-time interface assertions for `fixture.Driver` + `fixture.BackendKindAware` + `fixture.StatsAsserter`. Step 2 added blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver"` in `test/differential/runner_test.go` (alphabetically after 0010-graceful-drain).

The first execution exposed three pre-existing bugs in earlier-task deliverables that had to be fixed for the differential gate to fire end-to-end:

1. **`test/fixtures/0011-http-fault/envoy.yaml` admin port (Task 12 deliverable)**: the SPEC §3 + PLAN line 2249 + README pinned reference admin to `:9902`, but `harness.StartReferenceProxy` exposes only `9901/tcp` and waits for /ready on 9901/tcp — every other fixture (0006, 0009, 0010, etc.) uses 9901. The 9902 was a pre-harness-discipline carryover. Changed envoy.yaml admin to `9901` and updated README.md's "Bootstrap discipline" bullet to reflect the corrected port and the cause.

2. **`internal/filter/http/fault/fault.go` combined delay+abort timer-callback path (Task 5 deliverable)**: the timer callback called `f.dcb.SendLocalReply` then exited without signaling the parked dispatch goroutine, leaving `parkDecode` blocked indefinitely on `decodeResumeCh`. Manual probe of envoy-go subject confirmed: scenarios 1/3a/3b/4* all returned correct responses; scenario 2 (combined delay+abort) hung past the 8s curl timeout. Added `f.dcb.ContinueDecoding()` after `SendLocalReply` in the combined branch — this purely wakes `parkDecode`; the chain's `localReplyDone` gate makes the resumed iteration short-circuit immediately. Updated `TestDecodeHeaders_Combined` in `fault_test.go` to expect `continued == 1` (was `== 0`) with a comment explaining the wake-up semantics: the signal is a parkDecode wake-up, not a re-iteration request.

3. **`internal/stats/name.go` SN2 dotted-rest flattening (Task 3 / ADR-0061 carryover)**: `flattenToProm` for the `http.<sp>.<rest>` case preserved `<rest>` verbatim, including any internal `.`. The first 17 stat names never had dots in the rest, but `fault.aborts_injected` (and the four siblings) do. envoy-go was emitting `envoy_http_fault.aborts_injected{envoy_http_conn_manager_prefix="ingress_http"}` — the literal period is invalid Prometheus syntax. Per SPEC §11.6 empirical pin: reference Envoy emits `envoy_http_fault_aborts_injected{...}` (underscore). Fixed by `strings.ReplaceAll(tail[dot+1:], ".", "_")` on the rest before forming the base. Existing `name_test.go` tests pass unchanged (their inputs had no internal dots in the rest).

Step 3 verified `go build ./...` clean. Step 4 ran `go test -count=1 ./test/differential/ -run 'TestDifferential/0011-http-fault' -v` — PASSES end-to-end in 2.32s (the differential gate (e) fires for fixture 0011 — first time on the fault filter). Step 5 ran the full `TestDifferential` suite (12 fixtures): all PASS in 37.79s (no regressions). Gate-(a) verification: `go build ./...` clean; `go vet ./...` clean; `gofmt -l` clean (after applying gofmt to the driver.go doc-comment indentation); `golangci-lint run ./...` clean; `go test -race -short ./...` all PASS (29 packages). NO new ADR.

Task 14 follow-up (post-review): Per Task 14 code-quality review, amended ADR-0102 (combined-path callback now correctly documents calling ContinueDecoding-after-SendLocalReply; chain's `localReplyDone` gate is the load-bearing mechanism); amended ADR-0107 (Prom name shape now correctly shows `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<sp>"}`); fixed stale 3-line comment in fault.go combined-path; corrected SN-rule labels in name.go and driver.go; added `TestFlattenToProm_HCM_DottedRest` unit test for the SN2 dotted-rest fix. Commits b5ae585.
**Outputs:**
```
$ go build ./...
$ go test -count=1 -timeout=180s ./test/differential/ -run 'TestDifferential/0011-http-fault' -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0011-http-fault
--- PASS: TestDifferential (2.32s)
    --- PASS: TestDifferential/0011-http-fault (2.32s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.394s

$ go test -count=1 -timeout=600s ./test/differential/ -run 'TestDifferential' -v
--- PASS: TestDifferential (37.79s)
    --- PASS: TestDifferential/0000-tcp-echo (1.53s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.22s)
    --- PASS: TestDifferential/0002-tls-tcp (1.25s)
    --- PASS: TestDifferential/0003-http11-routing (1.23s)
    --- PASS: TestDifferential/0004-h2-routing (1.77s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.96s)
    --- PASS: TestDifferential/0006-access-log (11.05s)
    --- PASS: TestDifferential/0007a-cors (1.34s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.72s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.40s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.98s)
    --- PASS: TestDifferential/0010-graceful-drain (9.38s)
    --- PASS: TestDifferential/0011-http-fault (1.96s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	37.873s
```

## Task 15 — BEHAVIOR_CONTRACT.md patches per SPEC §13 + ADR-0104 + ADR-0106 + ROADMAP row 09 done

**Commits:** 40db754 — this task's commit; TBD — SHA-fill follow-up
**Notes:** Task 15 lands the documentation surface for phase 09 phase-done: five BEHAVIOR_CONTRACT.md patches per SPEC §13.1–§13.5, two new ADRs (ADR-0104 deferral-format per ADR-0040; ADR-0106 family-expansion shape per ADR-0001 template), and the ROADMAP row 09 status flip in-progress → done. The §9 family heading at ROADMAP line 56 stays unchanged per ADR-0106's no-row-state invariant.

**§13.1 NEW `### envoy.filters.http.fault` subsection** inserted under `## HTTP filter chain` between `### Empirical evidence (cors preflight)` (existing) and `### Empirical evidence (413 overflow)` (existing). Six sub-sections per SPEC §13.1 verbatim block: Asserted equivalence (abort response shape + delay timing + combined ordering + headers-field gate); Per-route 3-tier merge (ADR-0073 + §11.7 wholesale-override); max_active_faults concurrency cap (LBP-1 sixth + per-filter-instance shared counter); Async-resume mechanics (ADR-0102 — corrected per Task 14 follow-up to reflect the combined-path callback's `SendLocalReply + ContinueDecoding` parkDecode-wake-up + chain's `localReplyDone` gate short-circuit); Does not yet apply to (9 deferred surfaces — header-driven + response_rate_limit + abort.grpc_status + upstream_cluster + downstream_nodes + runtime-key fields + disable_downstream_cluster_stats + filter_enabled + HeaderMatcher non-exact + H2 differential); Empirical evidence (verbatim curl excerpt from §11.3 — 503 + 4-header set + `fault filter abort` 18-byte body).

**§13.2 17→22-name table extension** renamed the existing `### 17-name table (introduced by phase 06.1)` heading to `### 22-name table (introduced by phase 06.1; extended by phase 09)`; appended a 5-row `Fault filter — 5 names (introduced by phase 09)` subsection after the existing `Server — 2 names` block; updated the existing `Total: 17 internal names.` footer line to `Total: 22 internal names (17 from 06.1 + 5 from 09).` The Prometheus name column reflects the actual flattening behavior post-Task 14 SN2 dotted-rest fix per ADR-0107 amendment: `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<sp>"}`.

**§13.3 Timing tolerances bullet** appended one bullet at the end of the existing `## Timing tolerances` section: ±10ms per phase 09 §11.2 empirical pin; envoy-go's `time.AfterFunc` matches Envoy v1.37.2 across the 50/100/200/500ms sweep with worst-case +3.6ms overhead; the differential fixture 0011-http-fault's driver bucketizes elapsed timings (fast vs delayed) per planner-time decision 11.

**§13.4 Equivalence Matrix new row** appended one row after the existing `Admin /server_info (DRAINING)` row (line 30): `HTTP filter envoy.filters.http.fault | Per-request equivalence on abort response shape + delay timing + per-route wholesale-override + headers-field exact-match + stat counter increments under fixture 0011-http-fault. NOT asserted: header-driven + response_rate_limit + abort.grpc_status + HeaderMatcher non-exact.`

**§13.5 Forward-pointer notes** appended two notes (the third pointer is already covered by §13.4): (a) after `## HTTP filter chain ### Async resume mechanics` — phase 09 is the FIRST production exerciser of async-resume on the request side; cross-ref ADR-0102 + the `localReplyDone` gate short-circuit; (b) after `## Stat-name mapping ### Twin-series filter discipline` — phase 09 takes route A for `fault.response_rl_injected` (emit permanently-zero counter rather than per-line-skip in the differential allow-list); cross-ref ADR-0107.

**ADR-0104 (Deferral)** appended to DECISIONS.md per ADR-0040 deferral-ADR format (sibling of ADR-0089's deferral-list precedent). Title: `Header-driven fault path deferred — coupled to delay.header_delay / abort.header_abort proto sub-messages per phase 09 §11.5 empirical pin`. Status: Deferred. Body covers: the §11.5 major-surprise empirical pin (request headers REQUIRE the proto sub-messages to activate; cannot be cleanly separated); phase 09 silently parses both sub-messages (per ADR-0101's 11-field silent-ignore set) but does not honor them; the four documented `x-envoy-fault-{delay,abort}-request[-percentage]` request headers are silently ignored; future small follow-up phase (~150 LoC + 1 fuzzer + 1 fixture scenario) lands the coupled pair as a new top-level row per ADR-0106. Lands-in-task: Task 15 (commit SHA TBD; SHA-fill follow-up replaces TBD).

**ADR-0106 (Family-expansion shape)** appended to DECISIONS.md per ADR-0001 template. Title: `§9 HTTP filters family expansion shape — flat top-level rows + no-sibling-stub discipline; the §9 heading at ROADMAP line 56 is an umbrella, not a row`. Status: Accepted. Body covers: BRAINSTORM Decisions 12 (flat top-level rows for §9 family-children — no parent-row-with-sub-phases pattern) + 13 (no-sibling-stub discipline — ROADMAP rows added at brainstorming time, not at phase-done); the §9 heading at ROADMAP line 56 is a CONCEPTUAL UMBRELLA whose state is unchanged across all family-row landings; ADR-0045's split-gate stays available WITHIN any §9 family-row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts. Phase 09 is the FIRST §9 family-row to land; subsequent filters (header_mutation, buffer, local_ratelimit, jwt_authn, etc.) follow the same pattern. Lands-in-task: Task 15.

**ROADMAP row 09 flip.** Found row 09 at line 48 (the SPEC commit per phase-done discipline added it with status `in-progress`); flipped status field from `in-progress` to `done`. Verified the `### HTTP filters family` heading at line 56 is UNCHANGED in both text and position (per ADR-0106's no-row-state invariant). The §9 family-children enumeration at line 58 is also unchanged.

**Verification.** All five §13 patches anchor SPEC §13 anchors AND incorporate the Task 14 follow-up corrections (the combined-path callback now correctly documents `SendLocalReply + ContinueDecoding` parkDecode-wake-up + `localReplyDone` gate short-circuit per ADR-0102 amendment; Prom name shape is the post-SN2-dotted-rest-fix `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<sp>"}` per ADR-0107 amendment). ADR-0104 follows ADR-0040 deferral format; ADR-0106 follows ADR-0001 template. All cross-references valid (ADR-0040, ADR-0045, ADR-0073, ADR-0089, ADR-0101, ADR-0102, ADR-0103, ADR-0105, ADR-0106, ADR-0107; SPEC §11.5 + §11.7 + §13; BOOTSTRAP_PROMPT.md §9 invariant 4; BRAINSTORM Decisions 12, 13).

**Outputs:**
```
$ grep -n 'fault' docs/envoy-go/BEHAVIOR_CONTRACT.md | wc -l
(>20 fault references — §13.1 NEW subsection + §13.2 5-row table + §13.3 bullet + §13.4 row + §13.5 forward-pointers)

$ grep -n '^## ADR-0104\|^## ADR-0106' docs/envoy-go/DECISIONS.md
(both ADRs present)

$ grep -n '^| 09' docs/envoy-go/ROADMAP.md
48:| 09 | http-filter-fault | 08 | done |  |  ...
(status reads `done`)

$ go build ./...
$ go vet ./...
$ golangci-lint run ./...
$ go test -race -count=1 -short ./...
(all clean — docs don't affect build)
```

## Task 16 — Phase-done six-gate verification + STATE.md advance + phase-done commit

**Commits:** TBD — phase-done commit; TBD — STATE.md SHA-fill follow-up
**Notes:** Final lifecycle-state 5 → 6 task. All six gates green per BOOTSTRAP_PROMPT.md §5 phase-done discipline. Gate (d) abbreviated per planner-time PLAN guidance option B: ran ONLY the new `FuzzFaultConfigParse` for 30s (rationale: the existing 12 fuzzers were verified in prior phases AND phase 09 touches none of their code paths; Task 9 already ran `FuzzFaultConfigParse` for 30s with 3.36M execs at fuzzer-introduction time). The phase-done commit message names ALL eight ADRs ADR-0100..ADR-0107 in the subject and body per ADR-0044 ADR-on-impl convention + the 08.2 phase-done precedent. STATE.md flipped from `lifecycle-state: 3 / active-phase: 09-http-filter-fault` to `lifecycle-state: awaiting / active-phase: awaiting next planning` per BOOTSTRAP_PROMPT.md §5 between-phases state machine; `next-skill: superpowers:brainstorming` against §9 family list per ADR-0106 (flat top-level rows; the §9 heading at ROADMAP line 56 is an umbrella whose state is implicit and unchanged). No code changes in Task 16 — verification + state-advance + commit only.

**Gate (a) — `go build ./...` clean:**
```
$ go build ./...
(no output — clean)
```

**Gate (b) — `go test -race -count=1 ./...` clean:**
```
$ go test -race -count=1 ./... 2>&1 | tail -20
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.277s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.212s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.120s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.075s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.040s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.052s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.120s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.603s
ok  	github.com/esalaine/envoy-go/test/differential	41.052s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.024s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.026s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.026s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.028s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.033s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.031s
(no FAIL — clean)
```

**Gate (c) — h2spec 53/53 PASS at ADR-0051 pin (mechanical re-run; phase 09 touches no codec):**
```
$ go test -count=1 ./test/conformance/h2spec/...
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.274s

$ go test -count=1 -v -run 'TestH2Spec' ./test/conformance/h2spec/... | grep PASS | tail -20
    h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
    h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.22s)
PASS
```

**Gate (d) — fuzzers (option B abbreviation per planner-time guidance):**

Ran ONLY the new `FuzzFaultConfigParse` for 30s. Skipped the 12 existing fuzzers (`FuzzFilterChainParse`, `FuzzTLSContextParse`, `FuzzTcpProxyFilter`, `FuzzHCMConfigParse`, `FuzzFrameStream`, `FuzzHPACKDecode`, `FuzzDrainTransitions`, `FuzzPromTextFormat`, `FuzzConfigDumpFormat`, `FuzzBootstrapLoad`, `FuzzAccessLogFormat`, `FuzzFilterChainMatch`) per option B rationale: (a) all 12 were verified in prior phases including 08.2's phase-done at b33e04f; (b) phase 09 touches none of their code paths (the FactoryCtx Stats/StatPrefix extension is purely additive and exercised by the fault filter only — none of the 12 existing fuzzed code paths read these fields); (c) Task 9 already ran `FuzzFaultConfigParse` for 30s at fuzzer-introduction time with 3.36M execs clean. The Task 16 re-run confirms the fuzzer remains green after Tasks 10–15 (which did not touch `internal/filter/http/fault/` parser code).

```
$ go test -run='^$' -fuzz='^FuzzFaultConfigParse$' -fuzztime=30s ./internal/filter/http/fault/
fuzz: elapsed: 0s, gathering baseline coverage: 0/273 completed
fuzz: elapsed: 2s, gathering baseline coverage: 273/273 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 283628 (94536/sec), new interesting: 11 (total: 284)
fuzz: elapsed: 6s, execs: 768379 (161553/sec), new interesting: 21 (total: 294)
fuzz: elapsed: 9s, execs: 1050366 (93992/sec), new interesting: 28 (total: 301)
fuzz: elapsed: 12s, execs: 1247769 (65811/sec), new interesting: 32 (total: 305)
fuzz: elapsed: 15s, execs: 1434787 (62344/sec), new interesting: 34 (total: 307)
fuzz: elapsed: 18s, execs: 1515349 (26851/sec), new interesting: 34 (total: 307)
fuzz: elapsed: 21s, execs: 1802217 (95624/sec), new interesting: 35 (total: 308)
fuzz: elapsed: 24s, execs: 1895538 (31104/sec), new interesting: 35 (total: 308)
fuzz: elapsed: 27s, execs: 2427926 (177510/sec), new interesting: 38 (total: 311)
fuzz: elapsed: 30s, execs: 2484833 (18959/sec), new interesting: 38 (total: 311)
fuzz: elapsed: 31s, execs: 2484833 (0/sec), new interesting: 38 (total: 311)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	31.080s
```

**Gate (e) — differential 0000-0011 all green:**
```
$ go test -count=1 ./test/differential/... 2>&1 | tail -10
ok  	github.com/esalaine/envoy-go/test/differential	39.892s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.001s

$ go test -count=1 -v -run 'TestDifferential' ./test/differential/... | grep -E 'PASS|FAIL'
--- PASS: TestDifferential (37.73s)
    --- PASS: TestDifferential/0000-tcp-echo (1.45s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.24s)
    --- PASS: TestDifferential/0002-tls-tcp (1.27s)
    --- PASS: TestDifferential/0003-http11-routing (1.25s)
    --- PASS: TestDifferential/0004-h2-routing (1.82s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.06s)
    --- PASS: TestDifferential/0006-access-log (10.96s)
    --- PASS: TestDifferential/0007a-cors (1.36s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.81s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.42s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.88s)
    --- PASS: TestDifferential/0010-graceful-drain (9.25s)
    --- PASS: TestDifferential/0011-http-fault (1.96s)
PASS
```

13 differential subtests green (0000..0011 with 0007 split into 0007a/0007b — total subtest count is 13, 12 fixture directories per the file system).

**Gate (f) — BEHAVIOR_CONTRACT alignment + ROADMAP row 09 status:**
```
$ grep -c 'envoy.filters.http.fault' docs/envoy-go/BEHAVIOR_CONTRACT.md
5

$ grep -o 'envoy.filters.http.fault' docs/envoy-go/BEHAVIOR_CONTRACT.md | wc -l
6

$ grep -c 'response_rl_injected' docs/envoy-go/BEHAVIOR_CONTRACT.md
4

$ grep '^| 09' docs/envoy-go/ROADMAP.md
| 09 | http-filter-fault | 08 | done |  | New `internal/filter/http/fault/` package implementing `envoy.filters.http.fault` ... (status field reads `done`)
```

PLAN's `>= 8 envoy.filters.http.fault` heuristic was an over-estimate; actual occurrence count is 6 (5 lines, 6 string instances). All required SPEC §13.1–§13.5 content is in place per Task 15: §13.1 NEW `### envoy.filters.http.fault` subsection (lines 862-901), §13.2 5-row 17→22-name table extension (lines 176-180 + 194 forward-pointer), §13.3 ±10ms timing-tolerance bullet (line 293), §13.4 equivalence-matrix row (line 31), §13.5 forward-pointer notes (line 754 + 194). `response_rl_injected` 4 instances >= 2 expected. ROADMAP row 09 status field reads `done` (already flipped at Task 15 commit `40db754`; verified unchanged at this commit).

**STATE.md advance.** Flipped from `active-phase: 09-http-filter-fault / lifecycle-state: 3` to `active-phase: awaiting next planning / lifecycle-state: awaiting`; cleared `phase-directory`; updated `next-skill: superpowers:brainstorming` (against §9 family list per ADR-0106 — flat top-level rows; cors landed at 07.1, fault landed at 09); rewrote `next-skill-scope` for cold-starting brainstorming on the next §9 family-child. `last-commit: TBD` (filled in SHA-fill follow-up commit). `last-updated: 2026-05-03`.

**Self-review.** Phase-done commit message subject names ALL eight ADRs `[ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]` (per the implementer prompt's checklist). No `Co-Authored-By` trailer (matches the 08.2 phase-done precedent at `b33e04f` per `git log --format=%B -1 b33e04f`). All six gates green. STATE.md flipped to `awaiting next planning`. ROADMAP row 09 reads `done` (already flipped at Task 15; verified unchanged).

**Outputs:** see Gate (a)–(f) blocks above.
