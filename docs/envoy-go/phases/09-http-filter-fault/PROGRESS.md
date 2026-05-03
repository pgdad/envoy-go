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
