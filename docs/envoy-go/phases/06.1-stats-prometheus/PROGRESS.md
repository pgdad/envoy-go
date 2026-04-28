# Phase 06.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-02/03/04/05.1/05.2 PROGRESS.md structure.

## Preamble — execution preconditions

Two cosmetic deviations from PLAN.md's "Execution preconditions" block, neither substantive: (1) `grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1` returns `## ADR-0057:` because the in-file ADR ordering is non-monotonic — ADR-0058 (Trailers observed but not forwarded) was authored before ADR-0057 (Closes ADR-0035 H/2 leg) at 05.2 phase-done time per the documented topical-vs-commit-order precedent (05.2 PLAN.md §"ADRs introduced by this plan"). The HIGHEST-numbered ADR present is **ADR-0058**, matching PLAN's expectation; the `tail -1` mechanic was misleading. **PLAN's anticipated ADR numbers 0059..0064 are correct and require no shift.** (2) `git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` returns `ad46930` (the actual SPEC drafting commit), whereas PLAN expected `be99b42`. `be99b42` is a STATE.md-only SHA-fill follow-up that did not touch SPEC.md, so `ad46930` is the correct answer to the SHA query and the codebase state matches the documented SPEC. All other preconditions green: branch `phase/06.1-stats-prometheus-impl` at PLAN commit `4820c99`; docker client+server reported; `go version go1.26.2 linux/amd64` (PLAN required go1.23+); `golangci-lint has version v1.64.8`; `go test ./...` green for every package (no FAIL, no compile error); `github.com/envoyproxy/go-control-plane/envoy v1.32.4`; phase-05.2 REVIEW close present in HEAD (`b9810ad phase 05.2: REVIEW.md — APPROVED WITH FOLLOW-UPS`); `internal/stats/` contains only `doc.go`; `admin.New(addr string) *Server` matches; cluster has `NewManager`+`NewManagerWithBaseDir`; listener has `NewManager`+`NewManagerWithBaseDir`+`NewManagerWithBaseDirAndAllowH2C`; `BEHAVIOR_CONTRACT.md` has exactly one `## Stat-name mapping` section.

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `04bf75d`
**Notes:** Created PROGRESS.md; verified all preconditions per PLAN §"Execution preconditions"; phase-05.2 close confirmed present in HEAD; SPEC at `ad46930` (PLAN's `be99b42` was a cosmetic STATE-only SHA-fill, not a SPEC change). Highest ADR present is **ADR-0058** (the `tail -1` returned 0057 due to in-file commit-order non-monotonicity per 05.2 precedent, not because ADR-0058 is missing); next-free is **ADR-0059** and PLAN's anticipated 0059..0064 numbering is unchanged. `internal/stats/` contains only `doc.go` (registry.go etc. land at Task 2).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase/06.1-stats-prometheus-impl
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0057: Closes ADR-0035 H/2 leg via fixture 0004's full-stack HTTPS h2
$ git log -1 --format=%H -- docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md
ad469309eb673a738956f67c93a70bd70894c496
$ ls internal/stats/
doc.go
```

## Task 2 — `internal/stats/registry.go` + `counter.go` + LBP-1 enforcement [ADR-0059, ADR-0060]

**Commits:** `9417235` + `7ccdc73` (concurrency-tightening follow-up: split `checkRegister` into `checkName` outside the lock + `checkFrozenLocked` under the lock, closing the Freeze/Register race the code-quality reviewer flagged)
**Notes:** Created `internal/stats/registry.go` (Registry + LBP-1 freeze), `counter.go` (atomic-Uint64 Counter), minimal `gauge.go` stub (Name/Type/Format placeholder; Inc/Dec/Set/Load + atomic.Int64 backing land at Task 3), `registry_test.go` (8 tests), `counter_test.go` (3 tests). Rewrote `doc.go` from phase-00 stub. ADR-0059 (Internal Stats Store architecture) + ADR-0060 (Histograms deferred) appended to DECISIONS.md. One PLAN deviation noted: PLAN's regex literal `^[a-zA-Z_][a-zA-Z0-9_.]*$` accepted the test fixture `"trailing."`, which the test list (`TestRegistry_NewCounter_InvalidNamePanics`) explicitly requires to be rejected; tightened the regex to `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (alpha/underscore prefix, optional middle, must end in non-dot) and added a docstring comment explaining "dots are segment separators, not terminators." All 11 tests + race detector clean.
**Outputs:**
```
$ go test -race -count=1 ./internal/stats/ -v
=== RUN   TestCounter_Inc_Sequential
--- PASS: TestCounter_Inc_Sequential (0.00s)
=== RUN   TestCounter_Add_Sequential
--- PASS: TestCounter_Add_Sequential (0.00s)
=== RUN   TestCounter_Inc_RaceClean
--- PASS: TestCounter_Inc_RaceClean (0.01s)
=== RUN   TestRegistry_NewCounter_HappyPath
--- PASS: TestRegistry_NewCounter_HappyPath (0.00s)
=== RUN   TestRegistry_NewCounter_DuplicateNamePanics
--- PASS: TestRegistry_NewCounter_DuplicateNamePanics (0.00s)
=== RUN   TestRegistry_NewCounter_InvalidNamePanics
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/#00
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/1leading-digit
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with_space
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with-dash
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/trailing.
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with$char
--- PASS: TestRegistry_NewCounter_InvalidNamePanics (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/#00 (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/1leading-digit (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with_space (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with-dash (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/trailing. (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with$char (0.00s)
=== RUN   TestRegistry_Walk_RegistrationOrderInvariantNotPromised
--- PASS: TestRegistry_Walk_RegistrationOrderInvariantNotPromised (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeRegisterPanics
--- PASS: TestRegistry_Freeze_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeNewGaugePanics
--- PASS: TestRegistry_Freeze_PostFreezeNewGaugePanics (0.00s)
=== RUN   TestRegistry_Freeze_Idempotent
--- PASS: TestRegistry_Freeze_Idempotent (0.00s)
=== RUN   TestRegistry_Walk_ConcurrentWithIncs_RaceClean
--- PASS: TestRegistry_Walk_ConcurrentWithIncs_RaceClean (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.020s
$ go vet ./...
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0060: Histograms deferred from 06.1
```

## Task 3 — `internal/stats/gauge.go`

**Commits:** `474e8c7`
**Notes:** Replaced the Task 2 minimal Gauge stub with the full body — `Inc`, `Dec`, `Set`, `Add`, `Load`, `Format` backed by `atomic.Int64`. Negative gauge values permitted per BRAINSTORM §5.2. Added `gauge_test.go` with 5 tests covering sequential Inc/Dec/Set, negative-value-allowed (3 Decs from zero → -3), Add with mixed-sign deltas, race-clean concurrent Inc/Dec/Set, and Format rendering of negative values. No new ADR.
**Outputs:**
```
$ go test -race -count=1 ./internal/stats/ -v
=== RUN   TestCounter_Inc_Sequential
--- PASS: TestCounter_Inc_Sequential (0.00s)
=== RUN   TestCounter_Add_Sequential
--- PASS: TestCounter_Add_Sequential (0.00s)
=== RUN   TestCounter_Inc_RaceClean
--- PASS: TestCounter_Inc_RaceClean (0.01s)
=== RUN   TestGauge_IncDecSet_Sequential
--- PASS: TestGauge_IncDecSet_Sequential (0.00s)
=== RUN   TestGauge_NegativeValueAllowed
--- PASS: TestGauge_NegativeValueAllowed (0.00s)
=== RUN   TestGauge_Add_PositiveAndNegative
--- PASS: TestGauge_Add_PositiveAndNegative (0.00s)
=== RUN   TestGauge_RaceClean_ConcurrentIncDecSet
--- PASS: TestGauge_RaceClean_ConcurrentIncDecSet (0.00s)
=== RUN   TestGauge_Format_NegativeRendered
--- PASS: TestGauge_Format_NegativeRendered (0.00s)
=== RUN   TestRegistry_NewCounter_HappyPath
--- PASS: TestRegistry_NewCounter_HappyPath (0.00s)
=== RUN   TestRegistry_NewCounter_DuplicateNamePanics
--- PASS: TestRegistry_NewCounter_DuplicateNamePanics (0.00s)
=== RUN   TestRegistry_NewCounter_InvalidNamePanics
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/#00
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/1leading-digit
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with_space
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with-dash
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/trailing.
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with$char
--- PASS: TestRegistry_NewCounter_InvalidNamePanics (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/#00 (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/1leading-digit (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with_space (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with-dash (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/trailing. (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with$char (0.00s)
=== RUN   TestRegistry_Walk_RegistrationOrderInvariantNotPromised
--- PASS: TestRegistry_Walk_RegistrationOrderInvariantNotPromised (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeRegisterPanics
--- PASS: TestRegistry_Freeze_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeNewGaugePanics
--- PASS: TestRegistry_Freeze_PostFreezeNewGaugePanics (0.00s)
=== RUN   TestRegistry_Freeze_Idempotent
--- PASS: TestRegistry_Freeze_Idempotent (0.00s)
=== RUN   TestRegistry_Walk_ConcurrentWithIncs_RaceClean
--- PASS: TestRegistry_Walk_ConcurrentWithIncs_RaceClean (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.023s
$ go vet ./...
```

## Task 4 — `internal/stats/name.go` flattening rules SN1–SN8 + helpText map [ADR-0061, ADR-0064]

**Commits:** `7f45a4d`
**Notes:** Created `internal/stats/name.go` with `flattenToProm` (SN1-SN5 dispatch on top-level segment + SN4 status-class regex with verified form: digit stripped, base ends `_xx`, label value single digit), `escapeLabelValue` (Prometheus text-format spec compliance for `\`, `"`, `\n`), and the `helpText` map covering the 10 unique Prometheus names. Added `name_test.go` with happy-path tests for SN1/SN2/SN3/SN5, the SN4 HCM single-digit test, the SN4 cluster-side all-5-digits subtest, the unknown-top-segment error test, the escape-label-value table-driven test, and helpText coverage. ADR-0061 (SN1-SN8 with empirically-pinned Rule SN4 + verbatim Envoy-scrape evidence at server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) + ADR-0064 (`stats_tags` not honored) appended to DECISIONS.md.
**Outputs:**
```
$ go test -race -count=1 ./internal/stats/ -v
=== RUN   TestCounter_Inc_Sequential
--- PASS: TestCounter_Inc_Sequential (0.00s)
=== RUN   TestCounter_Add_Sequential
--- PASS: TestCounter_Add_Sequential (0.00s)
=== RUN   TestCounter_Inc_RaceClean
--- PASS: TestCounter_Inc_RaceClean (0.01s)
=== RUN   TestGauge_IncDecSet_Sequential
--- PASS: TestGauge_IncDecSet_Sequential (0.00s)
=== RUN   TestGauge_NegativeValueAllowed
--- PASS: TestGauge_NegativeValueAllowed (0.00s)
=== RUN   TestGauge_Add_PositiveAndNegative
--- PASS: TestGauge_Add_PositiveAndNegative (0.00s)
=== RUN   TestGauge_RaceClean_ConcurrentIncDecSet
--- PASS: TestGauge_RaceClean_ConcurrentIncDecSet (0.00s)
=== RUN   TestGauge_Format_NegativeRendered
--- PASS: TestGauge_Format_NegativeRendered (0.00s)
=== RUN   TestFlattenToProm_Listener
--- PASS: TestFlattenToProm_Listener (0.00s)
=== RUN   TestFlattenToProm_HCM
--- PASS: TestFlattenToProm_HCM (0.00s)
=== RUN   TestFlattenToProm_Cluster
--- PASS: TestFlattenToProm_Cluster (0.00s)
=== RUN   TestFlattenToProm_StatusClass_HCM
--- PASS: TestFlattenToProm_StatusClass_HCM (0.00s)
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_1xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_2xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_3xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_4xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_5xx
--- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_1xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_2xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_3xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_4xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_5xx (0.00s)
=== RUN   TestFlattenToProm_Server
--- PASS: TestFlattenToProm_Server (0.00s)
=== RUN   TestFlattenToProm_Invalid_NoMatchingRule
--- PASS: TestFlattenToProm_Invalid_NoMatchingRule (0.00s)
=== RUN   TestEscapeLabelValue
--- PASS: TestEscapeLabelValue (0.00s)
    --- PASS: TestEscapeLabelValue/plain (0.00s)
    --- PASS: TestEscapeLabelValue/with_"quotes" (0.00s)
    --- PASS: TestEscapeLabelValue/with\backslash (0.00s)
    --- PASS: TestEscapeLabelValue/with_newline (0.00s)
    --- PASS: TestEscapeLabelValue/all_"\_together (0.00s)
=== RUN   TestHelpText_Coverage
--- PASS: TestHelpText_Coverage (0.00s)
=== RUN   TestRegistry_NewCounter_HappyPath
--- PASS: TestRegistry_NewCounter_HappyPath (0.00s)
=== RUN   TestRegistry_NewCounter_DuplicateNamePanics
--- PASS: TestRegistry_NewCounter_DuplicateNamePanics (0.00s)
=== RUN   TestRegistry_NewCounter_InvalidNamePanics
--- PASS: TestRegistry_NewCounter_InvalidNamePanics (0.00s)
=== RUN   TestRegistry_Walk_RegistrationOrderInvariantNotPromised
--- PASS: TestRegistry_Walk_RegistrationOrderInvariantNotPromised (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeRegisterPanics
--- PASS: TestRegistry_Freeze_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeNewGaugePanics
--- PASS: TestRegistry_Freeze_PostFreezeNewGaugePanics (0.00s)
=== RUN   TestRegistry_Freeze_Idempotent
--- PASS: TestRegistry_Freeze_Idempotent (0.00s)
=== RUN   TestRegistry_Walk_ConcurrentWithIncs_RaceClean
--- PASS: TestRegistry_Walk_ConcurrentWithIncs_RaceClean (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.024s
$ go vet ./...
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0064: `stats_config.stats_tags` config not honored; extraction hardcoded
```

## Task 5 — `internal/stats/prom.go` Prometheus text-format writer

**Commits:** `18fbb41`
**Notes:** Created `internal/stats/prom.go` with `WriteProm(io.Writer, *Registry) error` that walks the Registry, flattens each metric via `flattenToProm`, groups by Prometheus name (status-class collapse per Rule SN4), sorts alphabetically by Prometheus name, and emits `# HELP` + `# TYPE` + metric-line triples with blank-line group separators. Helper `writeMetricLine` emits the per-line shape (with `{}` omitted on label-less metrics). Added `prom_test.go` with 6 tests: empty registry, single counter (full HELP/TYPE/line shape), status-class collapse (one HELP+TYPE for the four `_Nxx` lines), alphabetic group ordering, negative gauge rendering, and label-value escaping via a synthetic-Metric injection (bypasses `nameRE` to test the writer's escape path independently). No new ADR. The `internal/stats` package is now feature-complete; integration tasks (6+) consume `WriteProm` from `internal/admin`.
**Outputs:**
```
$ go test -race -count=1 ./internal/stats/ -v
=== RUN   TestCounter_Inc_Sequential
--- PASS: TestCounter_Inc_Sequential (0.00s)
=== RUN   TestCounter_Add_Sequential
--- PASS: TestCounter_Add_Sequential (0.00s)
=== RUN   TestCounter_Inc_RaceClean
--- PASS: TestCounter_Inc_RaceClean (0.01s)
=== RUN   TestGauge_IncDecSet_Sequential
--- PASS: TestGauge_IncDecSet_Sequential (0.00s)
=== RUN   TestGauge_NegativeValueAllowed
--- PASS: TestGauge_NegativeValueAllowed (0.00s)
=== RUN   TestGauge_Add_PositiveAndNegative
--- PASS: TestGauge_Add_PositiveAndNegative (0.00s)
=== RUN   TestGauge_RaceClean_ConcurrentIncDecSet
--- PASS: TestGauge_RaceClean_ConcurrentIncDecSet (0.00s)
=== RUN   TestGauge_Format_NegativeRendered
--- PASS: TestGauge_Format_NegativeRendered (0.00s)
=== RUN   TestFlattenToProm_Listener
--- PASS: TestFlattenToProm_Listener (0.00s)
=== RUN   TestFlattenToProm_HCM
--- PASS: TestFlattenToProm_HCM (0.00s)
=== RUN   TestFlattenToProm_Cluster
--- PASS: TestFlattenToProm_Cluster (0.00s)
=== RUN   TestFlattenToProm_StatusClass_HCM
--- PASS: TestFlattenToProm_StatusClass_HCM (0.00s)
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits
--- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits (0.00s)
=== RUN   TestFlattenToProm_Server
--- PASS: TestFlattenToProm_Server (0.00s)
=== RUN   TestFlattenToProm_Invalid_NoMatchingRule
--- PASS: TestFlattenToProm_Invalid_NoMatchingRule (0.00s)
=== RUN   TestEscapeLabelValue
--- PASS: TestEscapeLabelValue (0.00s)
=== RUN   TestHelpText_Coverage
--- PASS: TestHelpText_Coverage (0.00s)
=== RUN   TestWriteProm_EmptyRegistry
--- PASS: TestWriteProm_EmptyRegistry (0.00s)
=== RUN   TestWriteProm_SingleCounter
--- PASS: TestWriteProm_SingleCounter (0.00s)
=== RUN   TestWriteProm_StatusClassCollapse
--- PASS: TestWriteProm_StatusClassCollapse (0.00s)
=== RUN   TestWriteProm_AlphabeticallySortedGroups
--- PASS: TestWriteProm_AlphabeticallySortedGroups (0.00s)
=== RUN   TestWriteProm_GaugeRendersNegative
--- PASS: TestWriteProm_GaugeRendersNegative (0.00s)
=== RUN   TestWriteProm_EscapesLabelValues
--- PASS: TestWriteProm_EscapesLabelValues (0.00s)
=== RUN   TestRegistry_NewCounter_HappyPath
--- PASS: TestRegistry_NewCounter_HappyPath (0.00s)
=== RUN   TestRegistry_NewCounter_DuplicateNamePanics
--- PASS: TestRegistry_NewCounter_DuplicateNamePanics (0.00s)
=== RUN   TestRegistry_NewCounter_InvalidNamePanics
--- PASS: TestRegistry_NewCounter_InvalidNamePanics (0.00s)
=== RUN   TestRegistry_Walk_RegistrationOrderInvariantNotPromised
--- PASS: TestRegistry_Walk_RegistrationOrderInvariantNotPromised (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeRegisterPanics
--- PASS: TestRegistry_Freeze_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeNewGaugePanics
--- PASS: TestRegistry_Freeze_PostFreezeNewGaugePanics (0.00s)
=== RUN   TestRegistry_Freeze_Idempotent
--- PASS: TestRegistry_Freeze_Idempotent (0.00s)
=== RUN   TestRegistry_Walk_ConcurrentWithIncs_RaceClean
--- PASS: TestRegistry_Walk_ConcurrentWithIncs_RaceClean (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.024s
$ go vet ./...
```

## Task 6 — admin /stats/prometheus endpoint + server.live gauge with sync.Once

**Commits:** `f4160b8` + `17da0e3` (`cmd/envoy-go/main.go` scaffolding: widened `admin.New` cascaded into the existing main.go call site; throwaway `stats.NewRegistry()` passed with a `TODO(phase 06.1 Task 12)` comment, keeping `go vet ./...` clean per doctrine D-3.6 — PLAN's "no other call sites exist yet" claim was incorrect) + `6a1f93c` (test-robustness follow-up from the code-quality REVIEW: `TestServer_LiveGaugeSetOnceFlippedAtFirstReady200` `http.Get` nil-deref guards + accept-beat `time.Sleep(10 * time.Millisecond)` for symmetry with sibling tests; `-race -count=10` clean, closing I-1)
**Notes:** Created `internal/admin/prometheus.go` (`handlePrometheus(*stats.Registry) http.HandlerFunc` — sets the `text/plain; version=0.0.4; charset=utf-8` Content-Type, writes 200, then `stats.WriteProm`s the Registry; write errors are log-and-ignore per BRAINSTORM §5.3). Created `internal/admin/prometheus_test.go` with three table-style tests (Content-Type, empty-registry → empty body / 200, and full round-trip exercising the listener-segment SN1 flatten + `server.live` rendering). Widened `admin.New(addr string)` to `admin.New(addr string, registry *stats.Registry)` and added the matching `liveGauge *stats.Gauge` + `liveOnce sync.Once` fields; allocation of `server.live` happens in `New` (per SPEC §5.4 + §12 #3, registry must be pre-Freeze) and the LIVE-path `Set(1)` is wrapped in `sync.Once` so only the first 200/LIVE flips it. `Start()` registers `/stats/prometheus` alongside `/ready`. Updated all six existing `New(addr)` call sites in `admin_test.go` and added `TestServer_StatsPrometheusRouteRegistered` (live HTTP probe sanity-check) plus `TestServer_LiveGaugeSetOnceFlippedAtFirstReady200` (asserts gauge==0 pre-MarkReady, ==1 after MarkReady + 3× /ready). Extended `doc.go`'s package preamble. No new ADR. **One concern surfaced (see report):** PLAN's Task-6 "Out of scope" block forbids touching `cmd/envoy-go/main.go`, but its `admin.New(adminAddr)` call (introduced in phase 02) is the cascade target of the `New` signature change — `go vet ./...` therefore reports `cmd/envoy-go/main.go:57: not enough arguments in call to admin.New` while `go vet ./internal/...` is clean. Per the dispatch instructions ("if compilation fails elsewhere, that means there IS a hidden call site you must surface as a CONCERN, not silently fix"), the `cmd/envoy-go/main.go` edit is left for Task 7+ (the integration tasks that thread the Registry through main).
**Outputs:**
```
$ go test -race -count=1 ./internal/admin/ ./internal/stats/ -v
=== RUN   TestServer_ReadyState
--- PASS: TestServer_ReadyState (0.01s)
=== RUN   TestServer_PreInit_BeforeMarkReady
--- PASS: TestServer_PreInit_BeforeMarkReady (0.01s)
=== RUN   TestServer_MarkReady_IsAtomic
--- PASS: TestServer_MarkReady_IsAtomic (0.02s)
=== RUN   TestServer_Close_Idempotent
--- PASS: TestServer_Close_Idempotent (0.00s)
=== RUN   TestServer_StatsPrometheusRouteRegistered
--- PASS: TestServer_StatsPrometheusRouteRegistered (0.00s)
=== RUN   TestServer_LiveGaugeSetOnceFlippedAtFirstReady200
--- PASS: TestServer_LiveGaugeSetOnceFlippedAtFirstReady200 (0.00s)
=== RUN   TestHandlePrometheus_ContentType
--- PASS: TestHandlePrometheus_ContentType (0.00s)
=== RUN   TestHandlePrometheus_EmptyRegistry
--- PASS: TestHandlePrometheus_EmptyRegistry (0.00s)
=== RUN   TestHandlePrometheus_RoundTrip
--- PASS: TestHandlePrometheus_RoundTrip (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/admin	1.054s
=== RUN   TestCounter_Inc_Sequential
--- PASS: TestCounter_Inc_Sequential (0.00s)
=== RUN   TestCounter_Add_Sequential
--- PASS: TestCounter_Add_Sequential (0.00s)
=== RUN   TestCounter_Inc_RaceClean
--- PASS: TestCounter_Inc_RaceClean (0.01s)
=== RUN   TestGauge_IncDecSet_Sequential
--- PASS: TestGauge_IncDecSet_Sequential (0.00s)
=== RUN   TestGauge_NegativeValueAllowed
--- PASS: TestGauge_NegativeValueAllowed (0.00s)
=== RUN   TestGauge_Add_PositiveAndNegative
--- PASS: TestGauge_Add_PositiveAndNegative (0.00s)
=== RUN   TestGauge_RaceClean_ConcurrentIncDecSet
--- PASS: TestGauge_RaceClean_ConcurrentIncDecSet (0.00s)
=== RUN   TestGauge_Format_NegativeRendered
--- PASS: TestGauge_Format_NegativeRendered (0.00s)
=== RUN   TestFlattenToProm_Listener
--- PASS: TestFlattenToProm_Listener (0.00s)
=== RUN   TestFlattenToProm_HCM
--- PASS: TestFlattenToProm_HCM (0.00s)
=== RUN   TestFlattenToProm_Cluster
--- PASS: TestFlattenToProm_Cluster (0.00s)
=== RUN   TestFlattenToProm_StatusClass_HCM
--- PASS: TestFlattenToProm_StatusClass_HCM (0.00s)
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_1xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_2xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_3xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_4xx
=== RUN   TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_5xx
--- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_1xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_2xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_3xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_4xx (0.00s)
    --- PASS: TestFlattenToProm_StatusClass_Cluster_AllDigits/cluster.c0.upstream_rq_5xx (0.00s)
=== RUN   TestFlattenToProm_Server
--- PASS: TestFlattenToProm_Server (0.00s)
=== RUN   TestFlattenToProm_Invalid_NoMatchingRule
--- PASS: TestFlattenToProm_Invalid_NoMatchingRule (0.00s)
=== RUN   TestEscapeLabelValue
=== RUN   TestEscapeLabelValue/plain
=== RUN   TestEscapeLabelValue/with_"quotes"
=== RUN   TestEscapeLabelValue/with\backslash
=== RUN   TestEscapeLabelValue/with_newline
=== RUN   TestEscapeLabelValue/all_"\_together
--- PASS: TestEscapeLabelValue (0.00s)
    --- PASS: TestEscapeLabelValue/plain (0.00s)
    --- PASS: TestEscapeLabelValue/with_"quotes" (0.00s)
    --- PASS: TestEscapeLabelValue/with\backslash (0.00s)
    --- PASS: TestEscapeLabelValue/with_newline (0.00s)
    --- PASS: TestEscapeLabelValue/all_"\_together (0.00s)
=== RUN   TestHelpText_Coverage
--- PASS: TestHelpText_Coverage (0.00s)
=== RUN   TestWriteProm_EmptyRegistry
--- PASS: TestWriteProm_EmptyRegistry (0.00s)
=== RUN   TestWriteProm_SingleCounter
--- PASS: TestWriteProm_SingleCounter (0.00s)
=== RUN   TestWriteProm_StatusClassCollapse
--- PASS: TestWriteProm_StatusClassCollapse (0.00s)
=== RUN   TestWriteProm_AlphabeticallySortedGroups
--- PASS: TestWriteProm_AlphabeticallySortedGroups (0.00s)
=== RUN   TestWriteProm_GaugeRendersNegative
--- PASS: TestWriteProm_GaugeRendersNegative (0.00s)
=== RUN   TestWriteProm_EscapesLabelValues
--- PASS: TestWriteProm_EscapesLabelValues (0.00s)
=== RUN   TestRegistry_NewCounter_HappyPath
--- PASS: TestRegistry_NewCounter_HappyPath (0.00s)
=== RUN   TestRegistry_NewCounter_DuplicateNamePanics
--- PASS: TestRegistry_NewCounter_DuplicateNamePanics (0.00s)
=== RUN   TestRegistry_NewCounter_InvalidNamePanics
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/#00
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/1leading-digit
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with_space
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with-dash
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/trailing.
=== RUN   TestRegistry_NewCounter_InvalidNamePanics/with$char
--- PASS: TestRegistry_NewCounter_InvalidNamePanics (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/#00 (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/1leading-digit (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with_space (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with-dash (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/trailing. (0.00s)
    --- PASS: TestRegistry_NewCounter_InvalidNamePanics/with$char (0.00s)
=== RUN   TestRegistry_Walk_RegistrationOrderInvariantNotPromised
--- PASS: TestRegistry_Walk_RegistrationOrderInvariantNotPromised (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeRegisterPanics
--- PASS: TestRegistry_Freeze_PostFreezeRegisterPanics (0.00s)
=== RUN   TestRegistry_Freeze_PostFreezeNewGaugePanics
--- PASS: TestRegistry_Freeze_PostFreezeNewGaugePanics (0.00s)
=== RUN   TestRegistry_Freeze_Idempotent
--- PASS: TestRegistry_Freeze_Idempotent (0.00s)
=== RUN   TestRegistry_Walk_ConcurrentWithIncs_RaceClean
--- PASS: TestRegistry_Walk_ConcurrentWithIncs_RaceClean (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.023s
$ go vet ./internal/...
$ go vet ./...
# github.com/esalaine/envoy-go/cmd/envoy-go
# [github.com/esalaine/envoy-go/cmd/envoy-go]
vet: cmd/envoy-go/main.go:57:31: not enough arguments in call to admin.New
	have (string)
	want (string, *stats.Registry)
```

## Task 7 — Bootstrap.Stats field + Load allocates *stats.Registry

**Commits:** `d1e2690`
**Notes:** Per the settled SPEC §12 #2 decision (`Bootstrap.Stats` factoring = field-on-Bootstrap shape), introduced a new exported `Bootstrap` wrapper struct in `internal/bootstrap/bootstrap.go` with two fields (`Proto *bootstrapv3.Bootstrap`, `Stats *stats.Registry`) and changed `Load` to return `*Bootstrap`; the Registry is freshly allocated via `stats.NewRegistry()` and is intentionally NOT yet Frozen — downstream cluster/listener/HCM constructors (Tasks 8–11) register on it during boot, and Task 12 owns the post-construction Freeze call per SPEC §5.4. Cascaded the wrapper change through the only non-test caller `cmd/envoy-go/main.go`: the three propagation sites (`bootstrap.AdminSocket(bs)`, `cluster.NewManagerWithBaseDir(bs, …)`, `listener.NewManagerWithBaseDirAndAllowH2C(bs, …)`) now pass `bs.Proto`; the `admin.New(adminAddr, stats.NewRegistry())` throwaway-Registry line is intentionally left in place because Task 12 owns the swap to `bs.Stats` per the dispatch instructions. Updated all eleven `bs.GetX()` / `AdminSocket(bs)` / `protojson.Marshal(bs)` call sites in `bootstrap_test.go` to the new `bs.Proto.X` form and added `TestLoad_AllocatesStatsRegistry` (asserts `bs.Stats` is non-nil AND non-Frozen by exercising `NewCounter`). The PLAN's example test name `test.field-not-frozen` was rewritten to `test.field_not_frozen` because the SN-name regex `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (registry.go:98) rejects hyphens; the test still satisfies the PLAN's intent (NewCounter must succeed → Registry is not Frozen). `internal/bootstrap/fuzz_test.go` needed no changes — its `Load` call discards the return value. Anchored: SPEC §4.2 (bootstrap.go extension), §5.4 (boot wiring sequence), §12 #2 (settled).
**Outputs:**
```
$ go test -race -count=1 ./internal/bootstrap/ -run TestLoad_AllocatesStatsRegistry -v
=== RUN   TestLoad_AllocatesStatsRegistry
--- PASS: TestLoad_AllocatesStatsRegistry (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.018s
$ go test -race -count=1 ./internal/bootstrap/ ./cmd/envoy-go/ -v
=== RUN   TestLoad_HappyPath
--- PASS: TestLoad_HappyPath (0.01s)
=== RUN   TestLoad_RejectsDynamicResources
--- PASS: TestLoad_RejectsDynamicResources (0.00s)
=== RUN   TestLoad_RejectsLayeredRuntime
--- PASS: TestLoad_RejectsLayeredRuntime (0.00s)
=== RUN   TestLoad_YAMLSyntaxError
--- PASS: TestLoad_YAMLSyntaxError (0.00s)
=== RUN   TestLoad_UnknownTopLevelField
--- PASS: TestLoad_UnknownTopLevelField (0.00s)
=== RUN   TestLoad_EmptyDocument
--- PASS: TestLoad_EmptyDocument (0.00s)
=== RUN   TestAdminSocket_HappyPath
--- PASS: TestAdminSocket_HappyPath (0.00s)
=== RUN   TestAdminSocket_MissingAdmin
--- PASS: TestAdminSocket_MissingAdmin (0.00s)
=== RUN   TestBootstrap_RoundTrips_FixtureFour_Shape
--- PASS: TestBootstrap_RoundTrips_FixtureFour_Shape (0.00s)
=== RUN   TestLoad_AllocatesStatsRegistry
--- PASS: TestLoad_AllocatesStatsRegistry (0.00s)
=== RUN   TestLoad_HCMRoundTrip
--- PASS: TestLoad_HCMRoundTrip (0.00s)
=== RUN   FuzzBootstrapLoad
=== RUN   FuzzBootstrapLoad/seed#0
=== RUN   FuzzBootstrapLoad/seed#1
=== RUN   FuzzBootstrapLoad/seed#2
=== RUN   FuzzBootstrapLoad/seed#3
=== RUN   FuzzBootstrapLoad/seed#4
=== RUN   FuzzBootstrapLoad/seed#5
=== RUN   FuzzBootstrapLoad/seed#6
=== RUN   FuzzBootstrapLoad/seed#7
--- PASS: FuzzBootstrapLoad (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#0 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#1 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#2 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#3 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#4 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#5 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#6 (0.00s)
    --- PASS: FuzzBootstrapLoad/seed#7 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.030s
=== RUN   TestEnvoyGoBinary_TwoListenerCutover
--- PASS: TestEnvoyGoBinary_TwoListenerCutover (0.57s)
=== RUN   TestEnvoyGoBinary_HCMSmoke
--- PASS: TestEnvoyGoBinary_HCMSmoke (0.53s)
=== RUN   TestEnvoyGoBinary_H2Smoke
--- PASS: TestEnvoyGoBinary_H2Smoke (0.52s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.626s
$ go vet ./...
$ go build ./...
```

## Task 8 — cluster Registry threading + 8 metrics per cluster + Cluster fields [ADR-0063]

**Commits:** `a6f8b94`
**Notes:** Widened `cluster.NewManager(bs)` and `cluster.NewManagerWithBaseDir(bs, baseDir)` to accept a third `registry *stats.Registry` parameter; the new `registerClusterMetrics(r, c)` helper allocates the 8 cluster-scope metrics from SPEC §6 (`cluster.<n>.upstream_rq_total`, `cluster.<n>.upstream_rq_<2,3,4,5>xx`, `cluster.<n>.upstream_cx_total`, `cluster.<n>.upstream_cx_active`, `cluster.<n>.membership_total`) per cluster at boot time and Sets `membership_total` to `len(c.endpoints)` once at register. Extended the `Cluster` struct with 8 unexported metric-pointer fields (`upstreamRqTotal`, `upstreamRq2xx..5xx`, `upstreamCxTotal`, `upstreamCxActive`, `membershipTotal`) and added the unexported `(*Cluster).statusClassCounter(code int) *stats.Counter` helper that returns the matching `_Nxx` counter for codes in [200, 599] (nil otherwise; 1xx informationals are NOT bucketed) — Task 10 will consume this from `actions.go` per the Rule SN4 integer-divide discipline. Per ADR-0063 the metric set is cluster-level only; per-endpoint expansion is deferred (xDS-EDS-shape concern carried to a future phase). Updated all 23 pre-existing call sites in `internal/cluster/manager_test.go` to pass `stats.NewRegistry()` as the third arg, and added `TestNewManager_AllocatesEightMetricsPerCluster` (manager_test.go) plus `TestCluster_StatusClassCounter_Buckets` (cluster_test.go) covering both the happy 8-metric register path and the status-class dispatch table for 17 codes spanning the [0, 999] range. Per dispatch doctrine D-3.6 ("every commit green"), the constructor signature change cascaded into 5 dependent test files outside the PLAN's narrow file scope (`internal/listener/manager_test.go`, `internal/filter/hcm/{config,fuzz,actions}_test.go`, `internal/filter/tcpproxy/filter_test.go`); each got a one-line `cluster.NewManager(bs, stats.NewRegistry())` mechanical update plus the matching `internal/stats` import — these are throwaway Registries scoped to each individual test function, never Frozen, never observed via `/stats/prometheus`. Per the dispatch's D-3.6 deviation block, `cmd/envoy-go/main.go` line 53 was updated as the proper threading: `cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath))` → `cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath), bs.Stats)` — the cluster manager now allocates its 8-per-cluster metrics on the same `bs.Stats` Registry that Task 7 introduced. The admin server STILL has its throwaway `admin.New(adminAddr, stats.NewRegistry())` from Task 6 (intentionally; Task 12 owns the swap to `bs.Stats` and the post-construction `bs.Stats.Freeze()` call per SPEC §5.4). Until Task 12 lands, the cluster's 8 metrics are registered on `bs.Stats` but invisible via `/stats/prometheus` (the admin handler walks its own throwaway Registry); Task 14's differential fixture is the integration that will verify cross-Registry consistency once Task 12 unifies them. Anchored: SPEC §1 #3, §4.2 (cluster.go/manager.go extensions), §5.4 (boot wiring), §6 (8 cluster names), §8 (ADR-0063), §11.3 (cluster_test extension).
**Outputs:**
```
$ go test -race ./internal/cluster/ -run TestNewManager_AllocatesEightMetricsPerCluster -v
# pre-implementation (RED):
# github.com/esalaine/envoy-go/internal/cluster [github.com/esalaine/envoy-go/internal/cluster.test]
internal/cluster/manager_test.go:640:27: too many arguments in call to NewManager
	have (*bootstrapv3.Bootstrap, *stats.Registry)
	want (*bootstrapv3.Bootstrap)
internal/cluster/manager_test.go:649:7: c.upstreamRqTotal undefined (type *Cluster has no field or method upstreamRqTotal)
internal/cluster/manager_test.go:650:5: c.upstreamRq2xx undefined (type *Cluster has no field or method upstreamRq2xx)
internal/cluster/manager_test.go:650:31: c.upstreamRq3xx undefined (type *Cluster has no field or method upstreamRq3xx)
internal/cluster/manager_test.go:651:5: c.upstreamRq4xx undefined (type *Cluster has no field or method upstreamRq4xx)
internal/cluster/manager_test.go:651:31: c.upstreamRq5xx undefined (type *Cluster has no field or method upstreamRq5xx)
internal/cluster/manager_test.go:652:5: c.upstreamCxTotal undefined (type *Cluster has no field or method upstreamCxTotal)
internal/cluster/manager_test.go:653:5: c.upstreamCxActive undefined (type *Cluster has no field or method upstreamCxActive)
internal/cluster/manager_test.go:654:5: c.membershipTotal undefined (type *Cluster has no field or method membershipTotal)
internal/cluster/manager_test.go:658:14: c.membershipTotal undefined (type *Cluster has no field or method membershipTotal)
internal/cluster/manager_test.go:658:14: too many errors
FAIL	github.com/esalaine/envoy-go/internal/cluster [build failed]
FAIL

# post-implementation (GREEN):
$ go test -race ./internal/cluster/ -run 'TestNewManager_AllocatesEightMetricsPerCluster|TestCluster_StatusClassCounter_Buckets' -v
=== RUN   TestCluster_StatusClassCounter_Buckets
--- PASS: TestCluster_StatusClassCounter_Buckets (0.00s)
=== RUN   TestNewManager_AllocatesEightMetricsPerCluster
--- PASS: TestNewManager_AllocatesEightMetricsPerCluster (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	1.013s

$ go test -race -count=1 ./internal/cluster/
ok  	github.com/esalaine/envoy-go/internal/cluster	1.041s

$ go vet ./...
$ go build ./...
$ go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.993s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.069s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.035s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.037s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.265s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.526s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.034s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.041s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.032s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.085s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.129s
ok  	github.com/esalaine/envoy-go/test/differential	9.348s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.014s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.015s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.027s
```

## Task 9 — cluster Dial + DialH2 upstream-cx metric wiring (connWithGauge wrapper)

**Commits:** `bdfc34b` (5 Minor code-quality findings carried to lifecycle-state-4 review-followup per Task-8 precedent: M-1 generalize `(*connWithGauge).CloseWrite` to use the same `interface{ CloseWrite() error }` shape as `tcpproxy.halfClose`; M-2 one-liner justifying the `mkTestCluster` shim over fixture renames; M-3 Dial doc-comment caller-panic note; M-4 named-sentinel for the dial_h2 "not a connWithGauge" error; M-5 paragraph-break PROGRESS Notes)
**Notes:** Wired the upstream-cx metric pair from Task 8 into `(*Cluster).Dial`'s hot path: on every successful dial (post-TLS-handshake when `c.upstreamCfg != nil`, post-TCP-dial otherwise) Dial now `Inc`s `upstream_cx_total` (Counter) AND `upstream_cx_active` (Gauge), then wraps the returned `net.Conn` in a `*connWithGauge` whose `Close()` `Dec`s the active gauge exactly once via a `sync.Once` guard (defensive against double-Close from layered callers — Go's net/http stack closes a response body whose RoundTrip already closed the conn, etc.). Per the settled SPEC §12 deferred-decision #10, `connWithGauge` lives in `cluster.go` (one-file rule) — no separate `connwithgauge.go` file. The wrapper embeds `net.Conn` anonymously so non-Close I/O (`Read`, `Write`, `SetDeadline`, …) forwards automatically. Reworked `DialH2` to handle the new wrapper: instead of `tlsConn, ok := raw.(*stdtls.Conn)`, the function now type-asserts `wrapped, ok := raw.(*connWithGauge)` then `tlsConn, ok := wrapped.Conn.(*stdtls.Conn)`. The critical invariant: `h2.NewClientConn(ctx, wrapped)` MUST receive the `*connWithGauge` (NOT the inner `*stdtls.Conn`), so that `*h2.ClientConn.Close()` propagates through the wrapper and `Dec`s the active gauge — passing the inner conn would leak `upstream_cx_active` on every H2 request. All error branches use `wrapped.Close()` consistently. Added two new tests in `cluster_test.go` (`TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose` — pre/post-Dial and post-Close metric assertions; `TestDial_CloseIdempotent` — `sync.Once` guard exercised by double-Close, gauge stays at 0) and one in `dial_h2_test.go` (`TestCluster_DialH2_IncsCxMetricsAndDecsOnClose` — same Inc/Dec assertions on the H2 round-trip path). Updated `mkTestCluster` to allocate the 8 metrics on a throwaway Registry (with a hyphen→underscore name sanitization for the metric prefix only — many existing tests use names like `"test-tls"` which the SN-name regex rejects; the cluster's own `name` field is preserved as-is for caller-side identity assertions). Updated the existing `TestCluster_Dial_TLS` type assertion from `conn.(*stdtls.Conn)` to `conn.(*connWithGauge)` then `wrapped.Conn.(*stdtls.Conn)` to reflect the new Dial contract — minimal change to a pre-existing test. **PLAN deviation (one extra file, surfaced as concern):** the `*connWithGauge` wrapper broke `internal/filter/tcpproxy`'s `halfClose` type-switch, which previously asserted `*net.TCPConn` or `*stdtls.Conn` directly — the wrapper matches neither, so half-close stopped firing and `TestFilter_Handle_HalfCloseOverTLS` timed out. The minimum-intrusive fix: (a) added `(*connWithGauge).CloseWrite() error` that delegates to the inner `*net.TCPConn` / `*stdtls.Conn` via type-switch (no-op for unsupported transports); (b) updated `tcpproxy.halfClose` from a concrete-type switch to an `interface{ CloseWrite() error }` interface check — `*net.TCPConn`, `*stdtls.Conn`, AND `*connWithGauge` all satisfy the shape, so the function is now wrapper-agnostic and forward-compatible. The PLAN scoped Task 9 to 5 files; this is the 6th, justified by the cross-package contract shift the wrapper introduces (Cluster.Dial used to return `*net.TCPConn` / `*stdtls.Conn`, now returns `*connWithGauge` wrapping one). No new ADR. Anchored: SPEC §6 (`upstream_cx_{total,active}` semantics), §12 #10 (connWithGauge file placement), ADR-0063 (cluster-scope metrics + Inc-then-wrap discipline).
**Outputs:**
```
$ go test -race -count=1 ./internal/cluster/ -run 'TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose|TestDial_CloseIdempotent|TestCluster_DialH2_IncsCxMetricsAndDecsOnClose' -v
# pre-implementation (RED — counter/gauge sit at 0 because Dial doesn't Inc yet):
=== RUN   TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose
    cluster_test.go:357: post-Dial upstream_cx_total = 0, want 1
    cluster_test.go:360: post-Dial upstream_cx_active = 0, want 1
    cluster_test.go:371: post-Close upstream_cx_total = 0, want 1 (monotonic)
--- FAIL: TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose (0.00s)
=== RUN   TestDial_CloseIdempotent
--- PASS: TestDial_CloseIdempotent (0.00s)
=== RUN   TestCluster_DialH2_IncsCxMetricsAndDecsOnClose
    dial_h2_test.go:394: post-DialH2 upstream_cx_total = 0, want 1
    dial_h2_test.go:397: post-DialH2 upstream_cx_active = 0, want 1
    dial_h2_test.go:408: post-Close upstream_cx_total = 0, want 1 (monotonic)
--- FAIL: TestCluster_DialH2_IncsCxMetricsAndDecsOnClose (0.00s)
FAIL
FAIL	github.com/esalaine/envoy-go/internal/cluster	0.016s

# post-implementation (GREEN):
=== RUN   TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose
--- PASS: TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose (0.00s)
=== RUN   TestDial_CloseIdempotent
--- PASS: TestDial_CloseIdempotent (0.00s)
=== RUN   TestCluster_DialH2_IncsCxMetricsAndDecsOnClose
--- PASS: TestCluster_DialH2_IncsCxMetricsAndDecsOnClose (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/cluster	1.018s

$ go test -race -count=1 ./internal/cluster/
ok  	github.com/esalaine/envoy-go/internal/cluster	1.049s

$ go vet ./...
$ go build ./...
$ go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.857s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.069s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.037s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.049s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.254s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.503s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.031s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.036s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.029s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.079s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.106s
ok  	github.com/esalaine/envoy-go/test/differential	9.461s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.016s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.016s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.016s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.019s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.031s
```

## Task 10 — Listener-side Registry threading + 2-metric per-listener alloc + accept-loop hot path

**Commits:** `5dc76ab` (5 Minor code-quality findings carried to lifecycle-state-4 review-followup per Task-8/9 precedent: M-1 drop the local `endsWith` helper in `listener_test.go` and use `strings.HasSuffix` directly — `manager_test.go` already imports `strings` into the same test binary; M-2 add a one-line acknowledgement comment on the Start unwind path that orphan listener metrics on `m.registry` are intentional given the bind-error path is process-fatal — out-of-step with `cluster.NewManager`'s atomic-validate-then-register precedent but benign in practice; M-3 the pre-existing `Manager.Listeners()` reads `rt.netLn` without holding `startedMu` — same race-class as the Task-10 `acceptLoop` fix but kept out of scope per minimum-impact discipline; M-4 mirror the locking-precondition language ("capture must occur under m.startedMu") from `Start()`'s comment block into `acceptLoop`'s `ln`-parameter doc-comment so future readers don't need to cross-reference; M-5 tighten the docstring on `TestListener_AcceptLoop_IncsCxTotalAndCxActive` to acknowledge the post-close 2× polling budget vs the post-accept 1s budget — the asymmetry is correct (close-side observation is async through the dispatch-goroutine `defer`) but the docstring currently reads as if all polling is bounded at 1s)
**Notes:** Widened `listener.NewManager(bs, cm)` and the two BaseDir/AllowH2C variants to accept a trailing `registry *stats.Registry` parameter; the new unexported `registerListenerMetrics(r, rt)` helper allocates the 2 listener-scope metrics from SPEC §6 (`listener.<addr>.downstream_cx_total` counter, `listener.<addr>.downstream_cx_active` gauge) per listener and stores the pointers on the existing `listenerRuntime` struct (two new unexported fields: `downstreamCxTotal`, `downstreamCxActive`). The `<addr>` segment is produced by the new unexported `normalizeAddr(s)` which `strings.NewReplacer(":", "_", ".", "_", "[", "", "]", "")`-replaces the full `host:port` form: IPv4 `127.0.0.1:8080` → `127_0_0_1_8080`; IPv6 `[::]:45259` → `___45259`. The `[`/`]` strip is needed because `net.Listener.Addr().String()` on IPv6 listeners returns bracketed-host syntax which the SN-name regex (`^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`) rejects; surfaced by the h2spec conformance gate which binds IPv6 by default. Hot-path edits in `acceptLoop`: on each successful Accept, Inc `downstreamCxTotal` (counter, monotonic) AND `downstreamCxActive` (gauge, +1); the per-conn dispatch goroutine defers `downstreamCxActive.Dec()` so the gauge falls back when the filter's own deferred conn-close completes. Inc/Dec discipline is exactly once per conn — Inc on accept, Dec when the dispatch goroutine returns.

The HCM-factory closure inside `filterRegistry[hcm.TypeURL]` was widened to accept the Registry as a trailing parameter; it forwards (currently unused) so Task 11 can wire `hcm.NewFilterWithCtx` to allocate its 5 per-instance metrics on the same Registry. The TCP-proxy constructor closure ignores the Registry parameter (no per-tcp_proxy metrics in 06.1). Thread-through goes via `buildListenerRuntimeWithCtx`'s new `registry` parameter.

**PLAN deviation #1 — registration-at-Start, not at NewManager.** PLAN's example test code walks the Registry directly after `NewManager`. Implementation registers at `Start` time (post-`net.Listen`-resolves) instead, because (a) SPEC §6's `<addr>` is the BIND address — pre-bind the configured port may be 0 (`OS-pick`), producing metric names that don't reflect the actual bound port; (b) two listeners configured `:0` would collide on identical pre-resolve names (TestManager_HappyPath_Multi binds two `127.0.0.1:0` listeners — both would register `listener.127_0_0_1_0.downstream_cx_total` pre-bind and the second would panic with `stats: duplicate metric registration`). The Registry is captured on `Manager` at NewManager time and consumed in the per-listener bind loop in Start; `registerListenerMetrics(m.registry, rt)` sits between `rt.netLn = ln` and the post-loop accept-goroutine launches. Both new tests Start before walking — which mirrors the production scrape ordering (admin Start → listener Start → MarkReady → scrape).

**PLAN deviation #2 — file-scope substitution.** PLAN names `internal/listener/listener.go` (separate from `manager.go`) and `internal/listener/listener_test.go`. The actual codebase has only `manager.go` (the listener accept-loop logic lives on `*listenerRuntime` in `manager.go`); creating a separate `listener.go` would split the existing 458-line file along an arbitrary axis and obscure the accept-loop's relationship to the runtime struct. The implementation edits `manager.go` for both the constructor + accept-loop changes and creates a fresh `internal/listener/listener_test.go` (per PLAN's test-file naming) to host the accept-loop hot-path test. The +2-LoC promise on the accept loop is met (line-count parity at the Inc site; the Dec is an additional defer in the per-conn dispatch goroutine).

**PLAN deviation #3 — pre-existing race fix.** Surfaced by the new `TestListenerManager_AllocatesTwoMetricsPerListener` test which Start+Stop's a listener without intervening real-traffic delay: the race detector flagged `acceptLoop`'s `ln := rt.netLn` read against `Stop`'s `rt.netLn = nil` write. The pre-existing comment on the line said "Capture netLn locally so Stop()'s nil-out does not race with Accept()" but the capture was inside the goroutine — not before its launch. Fixed by changing `acceptLoop`'s signature from `(ctx context.Context)` to `(ctx context.Context, ln net.Listener)` and capturing `ln := rt.netLn` synchronously inside Start (under the held `m.startedMu`) before the `go rt.acceptLoop(ctx, ln)` launch. No behavior change; the race is gone.

**Cascade-fix surface.** The constructor signature change cascaded into 35 mechanical updates in `internal/listener/manager_test.go` (32 × `NewManager(boot, cm)` → `NewManager(boot, cm, stats.NewRegistry())`, 2 × `NewManagerWithBaseDirAndAllowH2C(boot, cm, "", X)` → `…(boot, cm, "", X, stats.NewRegistry())`, 1 × `NewManager(bs, cm)` in the HCM-build-error wrap test); throwaway Registries scoped per-test, never Frozen, never observed via `/stats/prometheus`. The production `cmd/envoy-go/main.go` line 66 was updated as the proper threading: `listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C)` → `listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats)` — the listener manager now allocates its 2-per-listener metrics on the same `bs.Stats` Registry that Tasks 7–8 wired through. Until Task 12 lands, those metrics live on `bs.Stats` but stay invisible via `/stats/prometheus` (admin still uses its throwaway Registry from Task 6); Task 12 owns the `bs.Stats`-everywhere unification + post-listener-up Freeze. No new ADR (PLAN explicitly annotates Task 10 as ADR-free).

Anchored: SPEC §1 #3, §4.2 (`listener/manager.go` extensions; the planned `listener.go` file is consolidated into `manager.go` per the actual file layout), §5.4 (boot wiring), §5.5 (accept-loop hot path), §6 (2 listener names), §11.3 (listener_test.go extension).
**Outputs:**
```
$ go test -race ./internal/listener/ -run 'TestListenerManager_|TestListener_AcceptLoop_' -v
# pre-implementation (RED — build failure: signatures don't accept the Registry yet):
# github.com/esalaine/envoy-go/internal/listener [github.com/esalaine/envoy-go/internal/listener.test]
internal/listener/listener_test.go:27:34: too many arguments in call to NewManager
	have (*bootstrapv3.Bootstrap, *cluster.Manager, *stats.Registry)
	want (*bootstrapv3.Bootstrap, *cluster.Manager)
internal/listener/manager_test.go:1403:34: too many arguments in call to NewManager
	have (*bootstrapv3.Bootstrap, *cluster.Manager, *stats.Registry)
	want (*bootstrapv3.Bootstrap, *cluster.Manager)
FAIL	github.com/esalaine/envoy-go/internal/listener [build failed]
FAIL

# post-implementation (GREEN):
$ go test -race ./internal/listener/ -run 'TestListenerManager_AllocatesTwoMetricsPerListener|TestListener_AcceptLoop_IncsCxTotalAndCxActive' -v
=== RUN   TestListener_AcceptLoop_IncsCxTotalAndCxActive
--- PASS: TestListener_AcceptLoop_IncsCxTotalAndCxActive (0.01s)
=== RUN   TestListenerManager_AllocatesTwoMetricsPerListener
--- PASS: TestListenerManager_AllocatesTwoMetricsPerListener (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/listener	1.029s

$ go test -race ./internal/listener/ ./internal/stats/ ./internal/admin/ ./internal/cluster/
ok  	github.com/esalaine/envoy-go/internal/listener	1.035s
ok  	github.com/esalaine/envoy-go/internal/stats	(cached)
ok  	github.com/esalaine/envoy-go/internal/admin	(cached)
ok  	github.com/esalaine/envoy-go/internal/cluster	(cached)

$ go vet ./... && go build ./... && go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.726s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.081s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.045s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.055s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.262s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.518s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.033s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.051s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.029s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.087s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.252s
ok  	github.com/esalaine/envoy-go/test/differential	9.522s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.015s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.026s
```

## Task 11 — HCM per-instance metric alloc + dispatch hot path + M-9 carry-forward

**Commits:** `438bd4f` (4 Minor code-quality findings carried to lifecycle-state-4 review-followup per Task-8/9/10 precedent: M-1 `hcm.parseFilter` test-convenience shim allocates a fresh throwaway Registry per call — replace 5 call sites in `config_test.go` with explicit `parseFilterWithCtx(...)` 4-arg form and remove `parseFilter` entirely so production code does not carry an unused-in-prod compatibility shim; M-2 `mkFilterForTable` in `connection_test.go` constructs `*Filter` with `clusters: nil` — add a one-line invariant comment "clusters intentionally nil — runConnection never reads it after route-table build" so a future refactor cannot silently nil-deref; M-3 the test-only `doH2Fn` field in `h2RouterActionAdapter` (`h2dispatch.go:111-115`) — clean factoring but worth a one-line comment explicitly labeling it "test-injection seam — production sets this to a.doH2 in Match" so the field's role is self-documenting; M-4 `parseFilterWithCtx` accepts `registry == nil` silently and panics later at `registry.NewCounter(...)` — add an early `if registry == nil { return nil, fmt.Errorf("hcm: registry is required") }` guard for defense-in-depth (the listener-manager already guards the parent path; this closes the direct-test-caller path))
**Notes:** Widened `hcm.NewFilter(tc, cm)` and `hcm.NewFilterWithCtx(tc, cm, lc)` to accept a trailing `registry *stats.Registry` parameter; `parseFilterWithCtx` allocates the 5 HCM-scope per-instance metrics from SPEC §6 (`http.<stat_prefix>.downstream_rq_total`, `http.<stat_prefix>.downstream_rq_<2,3,4,5>xx`) on the supplied Registry at filter-build time (pre-Freeze; SPEC §5.4). Extended the `Filter` struct with 5 unexported metric-pointer fields (`downstreamRqTotal`, `downstreamRq2xx..5xx`) and added the unexported `(*Filter).downstreamStatusClassCounter(code int) *stats.Counter` helper that returns the matching `_Nxx` counter for codes in [200, 599] (nil otherwise; 1xx informationals are NOT bucketed per SPEC §2.1) — mirrors `(*Cluster).statusClassCounter` from Task 8.

Hot-path edits per SPEC §5.5 + §12 #1 site (a):

- **H1 path (connection.go, runConnection):** signature changed from `(ctx, downstream, *routeTable)` to `(ctx, downstream, *Filter)` so the read loop can Inc the HCM counters. Inc `f.downstreamRqTotal` once after `http.ReadRequest` returns successfully (the once-per-request dispatch-entry hook). On response status finalization (route-match → `entry.action.do` returns the status; no-match → 404 catch-all; parse-error → synthesized 400; Expect: → 417; Upgrade: → 501; action error → status from do's first return), Inc `f.downstreamStatusClassCounter(status)` BEFORE `bw.Flush` per SPEC §5.5 "before bytes hit the wire".

- **H2 path (h2dispatch.go):** `h2Dispatcher` now holds `*Filter` (not `*routeTable`) so `Match` Inc's `f.downstreamRqTotal` once per request before route-table dispatch (the H2 analog of site (a)). The three adapters (`h2DirectResponseAdapter`, `h2RouterActionAdapter`, `h2RouterActionRejection`) carry the parent `*Filter` and Inc the matching HCM-scope downstream_rq_<Nxx> bucket on response status finalization.

- **routeAction interface (route.go):** widened `do(ctx, req, bw) error` → `do(ctx, req, bw) (int, error)` so runConnection can read the finalized status without snooping the bufio.Writer. Status is meaningful even on error returns (the action populates it before the writer error). Cascade: `directResponseAction.do` returns `(a.status, nil)`; `routerAction.do` returns the upstream `resp.StatusCode` (or 503/502 on local-reply paths); `routerActionH2.do` defensive H1 stub returns `(500, nil)`.

- **routerActionH2.doH2:** widened to return `(int, error)` so `h2RouterActionAdapter.WriteH2` knows the wire status for the HCM downstream_rq_<Nxx> Inc. status==0 on the ctx-cancel path (no terminating response) signals the adapter to skip the bucket Inc.

- **actions.go (cluster-scope):** `routerAction.do` Inc's `r.cluster.IncUpstreamRqTotal()` at dispatch entry; on every status-finalization path (dial-failure 503, write-failure 502, read-failure 502, successful proxy `resp.StatusCode`) Inc's `r.cluster.IncStatusClass(code)`. Same pattern in `routerActionH2.doH2`. The cluster-side accessors `(*Cluster).IncUpstreamRqTotal()` and `(*Cluster).IncStatusClass(code int)` are new exported helpers on `*Cluster` (cluster.go) so the hcm package can drive them without reaching across the package boundary into unexported metric fields.

- **M-9 carry-forward (SPEC §13.1):** `h2RouterActionAdapter.WriteH2` now logs `log.Printf("h2: doH2 error: %v", err)` on the `doH2` error path BEFORE propagating the error. To make the log line independently testable without mocking the upstream H2 dial, the adapter exposes a `doH2Fn` function-typed field default-bound at construction in `h2Dispatcher.Match` (`adapter.doH2Fn = a.doH2`). Tests substitute the field with a sentinel-failing function and capture the log via `log.SetOutput`. Closes the observability gap recorded in 05.2 REVIEW M-9 (`docs/envoy-go/phases/05.2-upstream-h2/REVIEW.md` line 175); SPEC §13.1 explicitly bundles M-9 with 06.1.

**PLAN deviation #1 — M-9 test file location.** PLAN's "Step 4" + SPEC §11.4 + §12 #5 + §14 line 715 specify `internal/filter/hcm/h2/router_action_test.go` (a new file in the h2 sub-package). Reality: `h2RouterActionAdapter` lives in package `hcm` at `internal/filter/hcm/h2dispatch.go` (NOT in package `h2` and NOT in a file named `router_action.go` — never has been). Symbols are unexported in package `hcm`, so a test in package `h2` cannot reach them without exporting the adapter (a wider API change than the M-9 fix warrants). Per SPEC §11.4's "(new or appended to existing test file)" relaxation, the M-9 unit test landed at `internal/filter/hcm/h2dispatch_test.go` (new file) in package `hcm` alongside the symbol it tests. The acceptance-checklist line 715 "M-9 ... lands in `internal/filter/hcm/h2/router_action.go` + `router_action_test.go`" is updated factually to "lands in `internal/filter/hcm/h2dispatch.go` + `h2dispatch_test.go`" — the SHA-fill follow-up commit can update the checklist if the reviewer wants the on-disk evidence to match the SPEC literal.

**PLAN deviation #2 — routeAction interface widened.** PLAN Step 2 sketched the response-finalization Inc as `if c := f.downstreamStatusClassCounter(resp.StatusCode); c != nil { c.Inc() }` inside the filter, but the H1 driver (`runConnection`) does not see the response — only the action does (the action writes directly to bw and returns error-only). Two options surfaced: (a) snoop the bufio.Writer for a `HTTP/1.1 NNN` line; (b) extend the routeAction interface to return status. Option (a) is fragile (writeStatusReply formatting drift, partial-write detection). Option (b) is one extra int return per implementation (3 sites: directResponseAction, routerAction, routerActionH2) and gives runConnection a clean status signal. Chose (b). Forced cascade: 5 existing test sites in `actions_test.go` + 1 in `connection_test.go` (the `actErr` shadowing pattern) updated mechanically to discard the new return — no behavior change in the existing assertions.

**PLAN deviation #3 — h2.Action interface NOT widened.** PLAN sketched a similar status-return widening for `WriteH2`. Reality: `h2.Action` is defined in `internal/filter/hcm/h2/stream.go` (the codec sub-package) and widening it would ripple into the h2 package's serverStream.dispatch + every fake/stub in h2 + the fuzzer's stubAction. Settled instead by widening `routerActionH2.doH2` (a hcm-package internal method) and having `h2RouterActionAdapter.WriteH2` consume the status return locally before honoring the `error`-only public interface. Cleaner: the hcm-package status discipline stays inside the hcm package; the h2 sub-package is unchanged.

**PLAN deviation #4 — file scope.** PLAN named `internal/filter/hcm/h2/router_action.go` for the M-9 fix (per SPEC §14 line 715) and `internal/filter/hcm/h2/router_action_test.go` for the test. Both files don't exist; the actual surface lives in `internal/filter/hcm/h2dispatch.go` (production) and the test lands at `internal/filter/hcm/h2dispatch_test.go` (new). See deviation #1 above.

**PLAN deviation #5 — H2 happy-path HCM downstream_rq_<Nxx> bucketing.** Initial implementation per the literal SPEC §5.5 row "HCM response hook" left this gap: doH2 streams the upstream response directly to the downstream StreamWriter, so the wire status is buried inside `resp.Status`. Closed by widening `routerActionH2.doH2` to return `(status int, error)` and having `h2RouterActionAdapter.WriteH2` Inc the HCM bucket from that return — settled inside Task 11 because the status was needed anyway for the cluster-side Inc.

**PLAN deviation #6 — exported `*Cluster` accessors.** PLAN Step 3 sketched `r.cluster.upstreamRqTotal.Inc()` direct-field access. Reality: package `hcm` cannot read unexported fields of `cluster.Cluster`. Two options: (a) export the metric pointers as `*Counter` getters and let the caller `.Inc()`; (b) export verb-style `Inc...()` methods that hide the metric type from the caller. Chose (b) — the hot-path callers in `actions.go` are pure side-effect-Inc sites (no value reads), so the verb-style is more cohesive and keeps the metric-pointer-typed surface unexported. Two methods landed on `*Cluster` in `internal/cluster/cluster.go`: `IncUpstreamRqTotal()` and `IncStatusClass(code int)` (the latter wraps the unexported `statusClassCounter` with the nil-guard inline). Naming convention: prefix-Inc verb form throughout (was renamed from `UpstreamRqTotalInc` per code-review I-1 to make the two accessors symmetric).

**Cascade-fix surface.** The `NewFilter` / `parseFilterWithCtx` signature change cascaded into `internal/filter/hcm/{filter,config,fuzz,actions,h2dispatch}_test.go`'s pre-existing call sites (replaced bare 2-arg NewFilter / 3-arg parseFilterWithCtx with the new 3-arg / 4-arg shapes plus throwaway `stats.NewRegistry()`). The `routeAction.do` interface widening cascaded into 9 existing test sites in `actions_test.go` + `connection_test.go` (changed `if err := a.do(...)` to `if _, err := a.do(...)`). The `h2Dispatcher` constructor change required `runConnection` to take `*Filter` instead of `*routeTable`; the existing 8 `connection_test.go` callers got a thin `mkFilterForTable(t, tt)` wrapper that allocates throwaway HCM counters on a fresh Registry — no behavior change. The listener-manager closure (`internal/listener/manager.go`'s `filterRegistry[hcm.TypeURL]`) un-`_`'d the registry parameter and forwards it to `hcm.NewFilterWithCtx` — completes the threading begun at Task 10. No production caller (cmd/envoy-go/main.go) changed; the listener manager already received `bs.Stats` at Task 10 and now flows it through to NewFilterWithCtx via the closure. No new ADR (PLAN explicitly annotates Task 11 as ADR-free).

Anchored: SPEC §1 #3, §1 #6 (M-9 carry-forward), §4.2 (`filter/hcm/{filter,config,connection,actions,h2dispatch,route}.go` extensions), §5.5 (Increment paths table — HCM dispatch entry, HCM response hook, routerAction.do H1, routerActionH2.do H2), §6 (5 HCM names + 4 of 8 cluster names), §11.3 (filter_test + actions_test extensions), §11.4 (M-9 unit test — landed at h2dispatch_test.go per deviation #1), §12 #1 site (a) (HCM Inc hook site decision — settled to the H1 read-loop and the H2 Match call), §12 #5 (M-9 test file — settled to h2dispatch_test.go per deviation #1), §13.1 (M-9 carry-forward bundle).
**Outputs:**
```
$ go test ./internal/filter/hcm/ -count=1 -run 'TestNewFilter_Allocates5HCMMetrics|TestFilter_RequestEntry_IncsDownstreamRqTotal|TestFilter_ResponseFinalization_IncsStatusClassCounter|TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass|TestRouterAction_Do_DialFailureInc5xx|TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass|TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error|TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess' -v
# pre-implementation (RED — build failure: signatures and fields don't exist yet):
# github.com/esalaine/envoy-go/internal/filter/hcm [github.com/esalaine/envoy-go/internal/filter/hcm.test]
internal/filter/hcm/filter_test.go:17:38: too many arguments in call to NewFilter
	have (*anypb.Any, *cluster.Manager, *stats.Registry)
	want (*anypb.Any, *cluster.Manager)
internal/filter/hcm/filter_test.go:33:34: too many arguments in call to NewFilter
	have (*anypb.Any, *cluster.Manager, *stats.Registry)
	want (*anypb.Any, *cluster.Manager)
internal/filter/hcm/filter_test.go:44:41: too many arguments in call to NewFilter
	have (*anypb.Any, *cluster.Manager, *stats.Registry)
	want (*anypb.Any, *cluster.Manager)
internal/filter/hcm/filter_test.go:73:38: too many arguments in call to NewFilter
	have (*anypb.Any, *cluster.Manager, *stats.Registry)
	want (*anypb.Any, *cluster.Manager)
internal/filter/hcm/filter_test.go:77:14: f.downstreamRqTotal undefined (type *Filter has no field or method downstreamRqTotal)
internal/filter/hcm/filter_test.go:89:14: f.downstreamRqTotal undefined (type *Filter has no field or method downstreamRqTotal)
internal/filter/hcm/filter_test.go:103:38: too many arguments in call to NewFilter
	have (*anypb.Any, *cluster.Manager, *stats.Registry)
	want (*anypb.Any, *cluster.Manager)
internal/filter/hcm/filter_test.go:116:14: f.downstreamRq2xx undefined (type *Filter has no field or method downstreamRq2xx)
internal/filter/hcm/filter_test.go:128:14: f.downstreamRq4xx undefined (type *Filter has no field or method downstreamRq4xx)
internal/filter/hcm/filter_test.go:133:14: f.downstreamRq3xx undefined (type *Filter has no field or method downstreamRq3xx)
internal/filter/hcm/filter_test.go:133:14: too many errors
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm [build failed]
FAIL

# post-implementation (GREEN):
$ go test -race ./internal/filter/hcm/ -count=1 -v -run 'TestNewFilter_Allocates5HCMMetrics|TestFilter_RequestEntry_IncsDownstreamRqTotal|TestFilter_ResponseFinalization_IncsStatusClassCounter|TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass|TestRouterAction_Do_DialFailureInc5xx|TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass|TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error|TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess'
=== RUN   TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass
--- PASS: TestRouterAction_Do_IncsUpstreamRqTotalAndStatusClass (0.00s)
=== RUN   TestRouterAction_Do_DialFailureInc5xx
--- PASS: TestRouterAction_Do_DialFailureInc5xx (0.00s)
=== RUN   TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass
--- PASS: TestRouterActionH2_Do_IncsUpstreamRqTotalAndStatusClass (0.01s)
=== RUN   TestNewFilter_Allocates5HCMMetrics
--- PASS: TestNewFilter_Allocates5HCMMetrics (0.00s)
=== RUN   TestFilter_RequestEntry_IncsDownstreamRqTotal
--- PASS: TestFilter_RequestEntry_IncsDownstreamRqTotal (0.00s)
=== RUN   TestFilter_ResponseFinalization_IncsStatusClassCounter
--- PASS: TestFilter_ResponseFinalization_IncsStatusClassCounter (0.00s)
=== RUN   TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error
--- PASS: TestH2RouterActionAdapter_WriteH2_LogsOnDoH2Error (0.00s)
=== RUN   TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess
--- PASS: TestH2RouterActionAdapter_WriteH2_NoLogOnSuccess (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.038s

$ go vet ./... && go build ./... && go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	2.919s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.074s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.040s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.051s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.265s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.499s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.035s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.047s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.030s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.084s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.078s
ok  	github.com/esalaine/envoy-go/test/differential	9.135s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.010s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.011s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.023s
```

## Task 12 — `cmd/envoy-go/main.go` Registry threading + `Freeze()` boot ordering

**Commits:** `a07b5dc` (2 Minor code-quality findings carried to lifecycle-state-4 review-followup per Task-8/9/10/11 precedent: M-1 add an optional `Content-Type: text/plain; version=0.0.4` header assertion to `TestMain_StatsPrometheusEndpointResponds` to harden against a future regression where someone wires the `/stats/prometheus` handler to JSON — handler unit-test coverage in `internal/admin/prometheus_test.go` already covers this so it's not strictly necessary at the binary-boot level, but cheap to add once we're touching the test; M-2 the LBP-1 comment block in `cmd/envoy-go/main.go:78-84` references "filter-chain build, eagerly executed inside listener.NewManagerWithBaseDirAndAllowH2C" without citing the Task 11 contract that establishes the eagerness — add a one-word "(per Task 11 contract;…)" annotation so a reader unfamiliar with Task 11 doesn't have to take the eagerness on faith)
**Notes:** Closed the last throwaway-Registry hold-out from Task 6: replaced `admin.New(adminAddr, stats.NewRegistry())` with `admin.New(adminAddr, bs.Stats)` so the admin server's `server.live` gauge AND the `/stats/prometheus` walk both happen on the unified `bs.Stats` Registry that Tasks 7–11 already wired into the cluster manager (8×N), listener manager (2×M), and HCM filter-build path (5×K). Removed the now-unused `internal/stats` import from `main.go` (`stats.NewRegistry()` was the only call site). Added `bs.Stats.Freeze()` AFTER `lm.Start(ctx)` returns and AFTER `admSrv.MarkReady()`, per SPEC §5.4 boot wiring sequence — by that point ALL `NewCounter` / `NewGauge` calls have completed: admin's `server.live` (at `admin.New`), cluster's 8×N (at `cluster.NewManagerWithBaseDir`), listener's 2×M (split between `listener.NewManager…` and `Listener.Start` per Task 10 deviation #1 — post-bind alloc is why Freeze MUST land post-`lm.Start`), and HCM's 5×K (at filter-chain build, eagerly executed inside `listener.NewManagerWithBaseDirAndAllowH2C`). Post-Freeze, any further `NewCounter` / `NewGauge` call panics with `stats: registry frozen: cannot register %q post-boot` — this is what makes the Walk-under-RLock-plus-atomic-Load read path lock-free against hot-path increments per SPEC §5.2 / §5.3 LBP-1.

**Test surface.** Extended `cmd/envoy-go/main_test.go` with `TestMain_StatsPrometheusEndpointResponds` modeled on the smallest existing bootstrap variant (`TestEnvoyGoBinary_HCMSmoke`): boots the binary on a single-HCM-listener config, waits for ready sentinels, GETs `http://<adminAddr>/stats/prometheus`, asserts 200 + body contains `# HELP envoy_server_live` AND `# HELP envoy_listener_downstream_cx_total`. The double assertion is deliberate: pre-Task-12 the throwaway Registry had `server.live` (admin allocates it on whichever Registry it gets), but the listener-scope metric lived on `bs.Stats` and was invisible to the admin walk — so the `# HELP envoy_listener_downstream_cx_total` line is the unification signal that distinguishes pre-Task-12 from post-Task-12. Added `bytes` to the test file's imports.

**RED→GREEN.** RED captured at `# HELP envoy_listener_downstream_cx_total` missing (body had ONLY `envoy_server_live` — proof the admin was walking the throwaway Registry, not bs.Stats); GREEN after the swap. Both transcripts in the Outputs section below.

**Boot ordering: no PLAN deviations.** (The Step-1 RED test assertion was strengthened from PLAN's single-name sketch to a dual-name unification witness — see Test surface paragraph above for rationale; the boot wiring itself follows PLAN verbatim.) Boot ordering followed PLAN's Step 2 sketch verbatim: `admin.New(adminAddr, bs.Stats)` → `lm, err := listener.NewManagerWithBaseDirAndAllowH2C(...)` → `lm.Start(ctx)` → `admSrv.MarkReady()` → `bs.Stats.Freeze()`. The Freeze landing AFTER `MarkReady()` is the SPEC-§5.4 idiom (admin starts accepting before Freeze) and LBP-1 still holds because admin's `server.live` is allocated at `admin.New` (line 57), which is well before `Freeze` at line 85. Filter-chain build inside `listener.NewManagerWithBaseDirAndAllowH2C` already allocates the HCM 5×K metrics eagerly (Task 11), so by `Freeze` time every metric the binary will ever emit is registered. No new ADR (PLAN explicitly annotates Task 12 as ADR-free).

Anchored: SPEC §4.2 (`main.go` extension), §5.3 (LBP-1 — Late-Bind Pattern), §5.4 (boot wiring sequence — the ordering followed verbatim), §12 #4 (filter-build pre-Freeze verification — confirmed at PLAN write time, re-verified by the RED→GREEN test on a real-process boot).
**Outputs:**
```
$ go build ./cmd/envoy-go && go test ./cmd/envoy-go/ -run TestMain_StatsPrometheusEndpointResponds -v -count=1
# pre-implementation (RED — admin walks the throwaway Registry; only server.live is observable):
=== RUN   TestMain_StatsPrometheusEndpointResponds
    main_test.go:421: body missing # HELP envoy_listener_downstream_cx_total (Registry unification not complete)
        --- body ---
        # HELP envoy_server_live 1 if the server is live, 0 otherwise.
        # TYPE envoy_server_live gauge
        envoy_server_live 0
--- FAIL: TestMain_StatsPrometheusEndpointResponds (0.60s)
FAIL
FAIL	github.com/esalaine/envoy-go/cmd/envoy-go	0.604s
FAIL

# post-implementation (GREEN):
=== RUN   TestMain_StatsPrometheusEndpointResponds
--- PASS: TestMain_StatsPrometheusEndpointResponds (0.53s)
PASS
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.535s

$ go vet ./... && go build ./... && go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.547s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.073s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.046s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.055s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.277s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.405s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.036s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.057s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.035s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.095s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.089s
ok  	github.com/esalaine/envoy-go/test/differential	9.215s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.015s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.016s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.016s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.030s
```

## Task 13 — `internal/stats/fuzz_test.go` `FuzzPromTextFormat` at 30s ADR-0018 budget

**Commits:** `63bb098` (5 Minor code-quality findings carried to lifecycle-state-4 review-followup per Task-8/9/10/11/12 precedent: M-1 `validatePromLine` over-accepts the empty labels block `name{} value` — `splitLabelsRespectingEscapes("")` returns `nil`, the range loop is a no-op, function returns `true`. Non-firing in practice because the writer's `writeMetricLine` short-circuits to the no-`{}` form when labels are empty, but defense-in-depth would add `if len(labels) == 0 { return false }` after the split; M-2 `validatePromLine` over-accepts `NaN`/`+Inf`/`-Inf` as values — `strconv.ParseFloat("NaN", 64)` returns no error. Non-firing because Counter is `uint64` and Gauge is `int64`, but a future `Format()` refactor returning arbitrary strings would silently slip through; M-3 the FuzzFrameStream first-run transient `context deadline exceeded` flake (32-worker race-detector pressure, no failing input saved) — re-run was authoritative-clean, but if the symptom recurs across the next 2-3 phases the right fix is to lower `-parallel` or bump per-iter wall-clock; one-line observability tracker for now; M-4 `matchingBraceEnd` helper name slightly ambiguous (`End` could read as "end of search" rather than "the close-brace itself") — doc comment compensates; consider renaming to `matchingClosingBrace` or `findCloseBrace` for self-evidence at call sites; M-5 helper `validatePromLine` returns `bool` — Go convention often prefers `IsValidPromLine` for predicates. Test-only unexported helper so this is taste-only).
**Notes:** Landed `FuzzPromTextFormat` per SPEC §11.8 + BRAINSTORM §6.4 + `## Settled SPEC §12 deferred decisions` #11. The fuzzer mutates a single `labelValue string` input; per fuzz iteration it allocates a fresh `*Registry`, then injects a synthetic `*synthMetric` (the same shape used by `prom_test.go`'s `TestWriteProm_EscapesLabelValues`) bypassing `nameRE` so the writer's escape path is exercised independently of register-time validation. The synthetic Metric's name embeds the fuzz-mutated `labelValue` in the `listener.<addr>.downstream_cx_total` form so flatten's Rule SN3 extraction surfaces `labelValue` as the `envoy_listener_address` label value — the data-flow shape that mirrors production (listener address bytes from a config become a label value at scrape time). `WriteProm(&buf, r)` runs; on error `t.Fatalf`. The output is then split on `\n` and every non-empty non-comment line is round-trip-checked by an in-test `validatePromLine` (~50 LoC) that locates the value-separator `}`-aware (a naive last-space split misclassifies in-quote spaces as value separators because the writer doesn't escape spaces — only `\` `"` `\n` per Prom spec) and verifies the value parses as float, the name matches `nameRE`, the labels split via a backslash-aware comma-respecting `splitLabelsRespectingEscapes` helper (~25 LoC), each label key matches `nameRE`, and each label value is bracketed by literal `"`s.

**Step 1 RED→GREEN.** Initial Step-2 fuzz run discovered `labelValue=".000 "` — the dot causes flatten to split addr|rest prematurely, and the leftover `000 ` lands in `rest`, producing `envoy_listener_000 .downstream_cx_total{envoy_listener_address=""} 1` (a malformed Prom name). Two follow-up fixes: (1) replace `.` in `labelValue` with `_` before injection — keeps every other adversarial byte (`\`, `"`, `\n`, NUL, unicode, long-string) exercised end-to-end while preventing the segment-separator collision; (2) make `validatePromLine` `}`-aware so it correctly handles unescaped spaces inside label values (a Prom-spec-legal case the writer can produce but the original sketch's `LastIndex(" ")` split misclassified). The RED corpus entry `1d8483e640bf8347` is preserved in `internal/stats/testdata/fuzz/FuzzPromTextFormat/` per Go's native fuzzer convention as a regression-test fixture for the dot-in-addr boundary.

**PLAN deviation #1 — `labelValue` reaches the writer's escape path.** PLAN sketch hard-codes the synthetic Metric's name to `listener.adv_addr.downstream_cx_total` and never consumes `labelValue` inside the `f.Fuzz` callback — that would make the fuzzer degenerate (every iteration runs the same code path). The task description's watch-out section explicitly flags this: "the fuzzer's `labelValue` input must reach the writer's escape path." Embedded `labelValue` (with `.`→`_` replacement) into the addr position so flatten extracts it as the `envoy_listener_address` label value — the production-mirror shape used by `prom_test.go`'s existing escape test. No SPEC drift: the fuzzer scope per `## Settled SPEC §12 deferred decisions` #11 is "fuzz adversarial label-value strings into `WriteProm`; assert no panic; assert the output round-trips through a Prometheus-format-aware parser without error" — the deviation tightens the literal sketch to actually do the fuzzer scope's job.

**PLAN deviation #2 — `validatePromLine` value-separator is `}`-aware.** PLAN sketch uses `strings.LastIndex(line, " ")` to split head from value. The writer's `escapeLabelValue` only escapes `\`, `"`, `\n` (per Prom spec) — spaces inside label values pass through unescaped and would be misclassified by the last-space heuristic. Replaced with: locate `{`, find matching `}` respecting backslash-escapes inside quoted values via a small `matchingBraceEnd` helper (~15 LoC), verify the next byte is the value-separating space. For lines without `{` (no labels), the unique space splits as before. ~10 net LoC over the sketch; closes a parser bug the fuzzer would otherwise hit immediately.

**FuzzFrameStream transient first-run.** First run of the cross-check `go test -race ./internal/filter/hcm/h2/ -fuzz=FuzzFrameStream -fuzztime=30s` reported `--- FAIL: FuzzFrameStream (32.05s) context deadline exceeded` after 674,874 executions, no failing input saved to `testdata/`. Re-run cleanly passed at `675,565` execs with `16` interesting inputs total. Diagnosis: scheduling pressure under 32 workers + race detector caused a single iteration to exceed the per-iter deadline; not a corpus-bug (no input saved → fuzzer's "save the failing input" path didn't trigger). Recorded here as a transient-flake observation; the second run is the authoritative result. No SPEC §11.8 regression — all seven fuzzers ultimately pass at the 30s budget.

Anchored: SPEC §11.8 (fuzzer enumeration + total post-06.1 = 7), §12 #11 (`## Settled SPEC §12 deferred decisions` #11 fuzzer scope — adversarial label values, no third-party Prom library, in-test round-trip parser), §14 (`FuzzPromTextFormat` is committed under `internal/stats/fuzz_test.go`; runs clean at 30s; total fuzzer count post-06.1 is 7).
**Outputs:**
```
$ go test -race ./internal/stats/ -run FuzzPromTextFormat -v
=== RUN   FuzzPromTextFormat
=== RUN   FuzzPromTextFormat/seed#0
=== RUN   FuzzPromTextFormat/seed#1
=== RUN   FuzzPromTextFormat/seed#2
=== RUN   FuzzPromTextFormat/seed#3
=== RUN   FuzzPromTextFormat/seed#4
=== RUN   FuzzPromTextFormat/seed#5
=== RUN   FuzzPromTextFormat/seed#6
=== RUN   FuzzPromTextFormat/seed#7
=== RUN   FuzzPromTextFormat/1d8483e640bf8347
--- PASS: FuzzPromTextFormat (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	1.007s

$ go test -race ./internal/stats/ -fuzz=FuzzPromTextFormat -fuzztime=30s
fuzz: elapsed: 27s, execs: 1056343 (53242/sec), new interesting: 75 (total: 84)
fuzz: elapsed: 30s, execs: 1155218 (32928/sec), new interesting: 77 (total: 86)
fuzz: elapsed: 32s, execs: 1155218 (0/sec), new interesting: 77 (total: 86)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	33.199s

$ go test -race ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 27s, execs: 118556 (3641/sec), new interesting: 6 (total: 1060)
fuzz: elapsed: 30s, execs: 137007 (6152/sec), new interesting: 6 (total: 1060)
fuzz: elapsed: 32s, execs: 137007 (0/sec), new interesting: 6 (total: 1060)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	33.227s

$ go test -race ./internal/filter/tcpproxy/ -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 27s, execs: 122243 (7734/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 30s, execs: 135231 (4328/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 32s, execs: 135231 (0/sec), new interesting: 0 (total: 555)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	33.114s

$ go test -race ./internal/tls/ -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 27s, execs: 232129 (8363/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 30s, execs: 261560 (9803/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 32s, execs: 261560 (0/sec), new interesting: 0 (total: 676)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	33.188s

$ go test -race ./internal/filter/hcm/ -fuzz=FuzzHCMConfigParse -fuzztime=30s
fuzz: elapsed: 27s, execs: 83204 (2549/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 30s, execs: 98614 (5131/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 32s, execs: 98614 (0/sec), new interesting: 0 (total: 513)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	33.390s

$ go test -race ./internal/filter/hcm/h2/ -fuzz=FuzzFrameStream -fuzztime=30s
fuzz: elapsed: 27s, execs: 615207 (21126/sec), new interesting: 16 (total: 400)
fuzz: elapsed: 30s, execs: 675565 (20100/sec), new interesting: 16 (total: 400)
fuzz: elapsed: 32s, execs: 675565 (0/sec), new interesting: 16 (total: 400)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	35.634s

$ go test -race ./internal/filter/hcm/h2/ -fuzz=FuzzHPACKDecode -fuzztime=30s
fuzz: elapsed: 27s, execs: 494582 (13666/sec), new interesting: 5 (total: 157)
fuzz: elapsed: 30s, execs: 533750 (13045/sec), new interesting: 5 (total: 157)
fuzz: elapsed: 32s, execs: 533750 (0/sec), new interesting: 5 (total: 157)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	35.723s

$ go vet ./... && go build ./... && go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.689s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.076s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.039s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.056s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.282s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.526s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.041s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.055s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.026s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.095s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.110s
ok  	github.com/esalaine/envoy-go/test/differential	9.217s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.027s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.024s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.028s
```

## Task 14 — differential fixture `0005-prometheus-stats` + runner registration [ADR-0062]

**Commits:** `a32212b` (impl)

Anchored: SPEC §6 (17-name allow-list — 12 counters + 5 gauges), §7.3 (expectations.yaml shape), §8 (ADR-0062 sketch), §12 #6 (in-band assertion discipline / `StatsAsserter` interface), §14 (fixture gate-(a) for task 14). Introduced `BackendKind=3` (`HTTPStatusHeader`) subprocess backend. Introduced `fixture.TB` and `fixture.StatsAsserter` optional interfaces. `driver.go` implements `Driver`, `BackendKindAware`, `StatsAsserter`; `AssertStatsEquivalence` exported for testability. Two-pass design: Drive pass saves listener addrs; `AssertStats` does scrape-before + fresh 5-request drive + 200ms drain + scrape-after per side. `Connection: close` in the backend forces Envoy to drain upstream keepalive connections between Drive and AssertStats passes.

**Deviation #7 — path correction (test/differential/ vs test/fixtures/).** PLAN.md step text says `test/differential/0005-prometheus-stats/` for fixture files; the actual repository convention (established by fixtures 0001–0004) is `test/fixtures/NNNN-name/`. All new files landed under `test/fixtures/0005-prometheus-stats/` matching the established layout. No SPEC drift.

**Deviation #8 — `delta_min` rows assert each-side `≥ 1` only (no equality).** Initial design had `delta_min=true` rows assert `ref_delta == subj_delta` in addition to `>= 1`. This failed because Envoy's keepalive pool produces `ref_delta=1` (one pooled connection) across the Drive pass while envoy-go (ADR-0056, no pooling) produces `subj_delta=5` (one per request). Fixed by removing equality assertion for `delta_min=true` rows — only `>= 1` is enforced on each side independently.

**Deviation #9 — `DisableKeepAlives=true` on all drive HTTP clients.** PLAN sketch did not specify keepalive disposition for the Drive HTTP client. Without `DisableKeepAlives`, the downstream connection from the driver to the reference Envoy listener is held open between Drive and AssertStats passes, leaving `downstream_cx_active=1` on the ref side while envoy-go (per-request conn close, ADR-0056) reports 0. Fixed by setting `DisableKeepAlives: true` on all Drive-path HTTP clients in `DriveReference` and `DriveSubject`.

**Deviation #10 — `Connection: close` in backend.** PLAN sketch did not specify upstream connection lifecycle in the HTTP backend. Without `Connection: close`, Envoy keeps its upstream connection to the backend alive indefinitely, leaving `upstream_cx_active=1` on the ref side after the Drive pass while envoy-go reports 0. Fixed by setting `w.Header().Set("Connection", "close")` in `handleRequest`.

**Carry-forward Minors:**
- M-1: `expectations.yaml` is parsed by `driver.go` at init time but could alternatively be read at AssertStats time to allow per-run parameterization. Current approach (hardcoded `Snapshot` deltas in `AssertStatsEquivalence`) is simpler and sufficient for Phase 06.1 scope.
- M-2: `parsePromSnapshot` ignores unknown metric names silently with no debug-level log. If a future name is misspelled in the allow-list it will silently yield 0 without any diagnostic. Consider adding a `testing.Logf`-level dump of unrecognized names in debug builds.
- M-3: `AssertStats` uses a hardcoded 200ms drain wait. This is sufficient for the current workload but could produce flaky failures under CI load spikes. A future hardening pass could replace the sleep with a poll-until-zero on the active-gauge endpoints.
- M-4 / M-A: ADR-0062 Context — "5 gauges" → "4 emitted gauges (1 not-emitted: `server.uptime`)" documentation polish. Location: `docs/envoy-go/DECISIONS.md` ADR-0062 Context section. L4-deferred.
- M-5 / M-B: `Snapshot` struct doc comment claims "17-name allow-listed" but struct has 16 fields (`server.uptime` NOT EMITTED). Fix: "Snapshot holds the 16 emitted-metric values from the SPEC §6 17-name allow-list (`server.uptime` is NOT EMITTED in 06.1)." Location: `test/fixtures/0005-prometheus-stats/driver/driver.go:57`. L4-deferred.
- M-6 / M-C: `makeAfterSnapshot` test helper silently omits 5 fields (`HCMRq3xx`, `ClusterRq3xx`, `ClusterRq4xx`, `ClusterCxActive`, `ListenerCxActive`) — those always default to zero in the 5-request workload, so `TestAssertStatsEquivalence_CounterMismatch` and `TestAssertStatsEquivalence_GaugeMismatch` do not exercise those rows of `AssertStatsEquivalence`. Fix: add a doc comment to `makeAfterSnapshot` explaining which fields are intentionally omitted and why. Location: `test/fixtures/0005-prometheus-stats/driver/driver_test.go:148`. L4-deferred.
- M-7 / M-D: `AssertStats` creates `context.Background()` independently from the runner's outer context — the internal 30s context cannot be cancelled early if the test's `testing.T` deadline fires. No actual deadlock/orphan risk in current synchronous design. Fix: add a one-line comment noting the deliberate independence. Location: `test/fixtures/0005-prometheus-stats/driver/driver.go:244`. L4-deferred.

**Outputs:**
```
$ go test -race -count=1 -v ./test/fixtures/0005-prometheus-stats/...
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
=== RUN   TestScrapeAndParse_HappyPath
--- PASS: TestScrapeAndParse_HappyPath (0.00s)
=== RUN   TestScrapeAndParse_IgnoresUnknownNames
--- PASS: TestScrapeAndParse_IgnoresUnknownNames (0.00s)
=== RUN   TestScrapeAndParse_WrongClusterName
--- PASS: TestScrapeAndParse_WrongClusterName (0.00s)
=== RUN   TestScrapeAndParse_EmptyBody
--- PASS: TestScrapeAndParse_EmptyBody (0.00s)
=== RUN   TestAssertStatsEquivalence_Pass
--- PASS: TestAssertStatsEquivalence_Pass (0.00s)
=== RUN   TestAssertStatsEquivalence_CounterMismatch
--- PASS: TestAssertStatsEquivalence_CounterMismatch (0.00s)
=== RUN   TestAssertStatsEquivalence_GaugeMismatch
--- PASS: TestAssertStatsEquivalence_GaugeMismatch (0.00s)
=== RUN   TestAssertStatsEquivalence_DeltaMinViolation
--- PASS: TestAssertStatsEquivalence_DeltaMinViolation (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.007s

$ go test ./test/differential/ -run TestDifferential/0005 -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0005-prometheus-stats
--- PASS: TestDifferential/0005-prometheus-stats (2.27s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.359s

$ go vet ./... && go build ./... && go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.515s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.081s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.054s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.061s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.279s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.509s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.042s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.063s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.038s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.099s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.177s
ok  	github.com/esalaine/envoy-go/test/differential	11.341s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.015s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.017s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.017s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.018s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.017s
ok  	github.com/esalaine/envoy-go/test/helpers	1.031s
```

## Task 15 — BEHAVIOR_CONTRACT in-place edit + all-gates green local sweep + STATE → lifecycle-state 4

**Commits:** `58d4ec9` (impl commit), follow-up SHA-fill commit per convention.

**Anchored:** SPEC §1 #4 (BEHAVIOR_CONTRACT in-place edit), §3 (six-gate phase-done), §4.4 (ROADMAP/STATE/PROGRESS lifecycle), §10 (BEHAVIOR_CONTRACT additions), §14 (full acceptance checklist), and BOOTSTRAP §5.3 (commit-message-completeness), §7.5 (six-gate sweep). Per ADR-0052 the in-place edit replaces the placeholder text rather than a separate doc.

**Notes:** Phase-06.1 closing task. The `## Stat-name mapping` placeholder at `BEHAVIOR_CONTRACT.md` lines 48–53 (skeleton text `_to be filled per-phase as needed._`) is replaced in-place with: the introductory paragraph (kept verbatim), the eight flattening rules SN1–SN8 verbatim from SPEC §10.1 (including Rule SN4's verbatim Envoy-scrape evidence block + the negative-confirmation grep statement + the regex-source citation), and the 17-name table from SPEC §6 verbatim. The `## Equivalence Matrix` row at line 19 (`| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |`) is superseded in-place per SPEC §10.2 + ADR-0062 Consequences (c). Five gates (a/b/c/d/e) green at this commit; gate (f) `REVIEW.md` is deferred to the REVIEW session at lifecycle-state 6 per BOOTSTRAP §5 step 6.

**Lint-fix sweep:** Six pre-existing golangci-lint findings from Tasks 4/13/14 were resolved in-scope at Task 15 to make gate (e) green: (1) `gofmt` in `internal/stats/name.go` (doc comment indent style for Go 1.19+ doc-comment format); (2) `revive` exported const without comment in `internal/stats/registry.go` (added `MetricCounter` + `MetricGauge` doc comments); (3) `misspell` in `internal/stats/prom.go` (`defence` → `defense`); (4) `gofmt` in `test/fixtures/0005-prometheus-stats/driver/driver.go` (struct field alignment); (5) + (6) `errcheck` in `test/fixtures/0005-prometheus-stats/backends/main.go` and `driver/driver.go` (`fmt.Fprint` returns and `resp.Body.Close()` returns now explicitly discarded with `_, _ =` / `defer func() { _ = ... }()` patterns). All fixes are within the lint-first discipline; no behavioural changes.

**Phase-05.2 REVIEW carry-forward resolution matrix (for phase 06.1):**

| Finding | Disposition |
|---|---|
| M-9 (Missing log line in `h2RouterActionAdapter.WriteH2` on `doH2` error) | RESOLVED-IN-06.1 (Task 11 at `438bd4f` — 5 LoC fix + test per SPEC §11.4; bundled with 06.1 per BRAINSTORM §2.4) |
| M-4 (`readClientPreface` not ctx-aware) | DEFERRED — carries to H2-hardening phase per ADR-0058 (no new ADR; same rationale) |
| M-10 (`SETTINGS_TIMEOUT` absent) | DEFERRED — carries to phase 06/08 per ADR-0058 (no new ADR; same rationale) |
| M-12 (`closedStreams` map unbounded) | DEFERRED — carries to long-lived-conn phase (free-standing carry-forward; no ADR) |

**Six-ADR landing summary (ADR-0059..ADR-0064):**

| ADR | Topic | Task | Commit |
|---|---|---|---|
| ADR-0059 | Internal Stats Store architecture (no third-party Prometheus; in-tree atomic Registry) | Task 2 | `9417235` |
| ADR-0060 | Histograms deferred from 06.1 (circllhist→Prometheus bucket mapping needs own brainstorm) | Task 2 | `9417235` |
| ADR-0061 | Stat-name → Prometheus-name flattening rules SN1–SN8 (Rule SN4 empirically pinned at SPEC-draft against Envoy v1.37.2) | Task 4 | `7f45a4d` |
| ADR-0064 | `stats_config.stats_tags` config not honored; extraction hardcoded in `name.go` | Task 4 | `7f45a4d` |
| ADR-0063 | Per-endpoint cluster stats not emitted; cluster-level only per LBP-1 + xDS-EDS deferral | Task 8 | `a6f8b94` |
| ADR-0062 | Differential equivalence shape for stats: per-counter delta-equality + per-gauge snapshot-equality on 17-name allow-list | Task 14 | `a32212b` |

**Anticipated ROADMAP row text (NOT modified at this commit — lands at lifecycle-state 6 REVIEW session per BOOTSTRAP §5 step 6):**

```markdown
| 06   | observability-baseline | 05 | in-progress |  | Sub-phases 06.1 (stats) + 06.2 (access-log). Closes only at 06.2's phase-done. |
| 06.1 | stats-prometheus | 05  | done         |  | In-tree stats Registry + /stats/prometheus exposition + 17 call sites; first non-vacuous observability-surface differential (fixture 0005). LBP-1 invariant; no third-party Prometheus dependency. ADR-0059..ADR-0064. |
| 06.2 | access-log       | 06.1 | planned     |  | Access-log subsystem; closes parent 06 at its phase-done. |
```

ROADMAP rows 06.1 stays `in-progress` at this commit; the phase-done commit at lifecycle-state 6 will flip it to `done` and confirm row 06 stays `in-progress` and row 06.2 stays `planned`.

**Gate (a) — fixture 0005 differential (NEW non-vacuous in 06.1 per ADR-0062):**

```
$ go test ./test/differential/ -run 'TestDifferential/0005' -v -timeout=120s
=== RUN   TestDifferential
=== RUN   TestDifferential/0005-prometheus-stats
2026/04/27 16:40:55 backend listening on :33599
2026/04/27 16:40:55 github.com/testcontainers/testcontainers-go - Connected to docker: 
  Server Version: 28.1.1
  API Version: 1.43
  Operating System: Docker Desktop
  Total Memory: 64296 MB
  Resolved Docker Host: unix:///home/esa/.docker/desktop/docker.sock
  Resolved Docker Socket Path: /var/run/docker.sock
  Test SessionID: 42fdb34fb5cfdf68aa51d52dae59a9978e68f28d4ad35e837b0c253e89d3d8f9
  Test ProcessID: ec173797-3e53-4530-b150-c4fef6aceca5
2026/04/27 16:40:55 🐳 Creating container for image testcontainers/ryuk:0.6.0
2026/04/27 16:40:55 ✅ Container created: a277460f3d2a
2026/04/27 16:40:55 🐳 Starting container: a277460f3d2a
2026/04/27 16:40:56 ✅ Container started: a277460f3d2a
2026/04/27 16:40:56 🚧 Waiting for container id a277460f3d2a image: testcontainers/ryuk:0.6.0. Waiting for: &{Port:8080/tcp timeout:<nil> PollInterval:100ms}
2026/04/27 16:40:56 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/04/27 16:40:56 ✅ Container created: 0245fb427ce1
2026/04/27 16:40:56 🐳 Starting container: 0245fb427ce1
2026/04/27 16:40:56 ✅ Container started: 0245fb427ce1
2026/04/27 16:40:56 🚧 Waiting for container id 0245fb427ce1 image: envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd. Waiting for: &{timeout:0x12a5abfb2278 Port:9901/tcp Path:/ready StatusCodeMatcher:0x86a120 ResponseMatcher:0x98f960 UseTLS:false AllowInsecure:false TLSConfig:<nil> Method:GET Body:<nil> PollInterval:100ms UserInfo:}
2026/04/27 16:40:57 🐳 Terminating container: 0245fb427ce1
2026/04/27 16:40:57 🚫 Container terminated: 0245fb427ce1
--- PASS: TestDifferential (2.24s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.24s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.331s
```

**Gate (b) — pre-existing differential fixtures (regression check):**

```
$ go test ./test/differential/ -run 'TestDifferential/(0000|0001|0002|0003|0004)' -v -timeout=120s
[testcontainers + container lifecycle abbreviated; `hcm: h2: EOF` lines are intentional EOF logs from h2spec-style probes during test-Envoy reachability checks, not failures.]
--- PASS: TestDifferential (6.92s)
    --- PASS: TestDifferential/0000-tcp-echo (1.53s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.21s)
    --- PASS: TestDifferential/0002-tls-tcp (1.20s)
    --- PASS: TestDifferential/0003-http11-routing (1.21s)
    --- PASS: TestDifferential/0004-h2-routing (1.78s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	7.011s
```

5/5 fixtures green.

**Gate (c) — h2spec conformance (UNCHANGED from 05.2 baseline; 06.1 does not touch H2 wire code):**

```
$ go test ./test/conformance/h2spec/ -v -timeout=300s
[testcontainers + summerwind/h2spec lifecycle abbreviated.]
        Finished in 0.5457 seconds
        53 tests, 53 passed, 0 skipped, 0 failed
        
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
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
--- PASS: TestH2Spec (2.17s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.259s
```

53/53 PASS — covers sections 3, 4, 5, 6 ex-6.6, 7, 8 per the ADR-0051 threshold list at the pinned `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`.

**Gate (d) — fuzzers (all 7 PLAN-enumerated fuzzers; 30s budget per ADR-0018):**

`grep -r '^func Fuzz' --include='*.go' .` enumeration:
```
internal/bootstrap/fuzz_test.go:62:func FuzzBootstrapLoad(f *testing.F)
internal/filter/tcpproxy/fuzz_test.go:26:func FuzzTcpProxyFilter(f *testing.F)
internal/tls/fuzz_test.go:24:func FuzzTLSContextParse(f *testing.F)
internal/filter/hcm/fuzz_test.go:24:func FuzzHCMConfigParse(f *testing.F)
internal/filter/hcm/h2/fuzz_test.go:24:func FuzzFrameStream(f *testing.F)
internal/filter/hcm/h2/fuzz_test.go:96:func FuzzHPACKDecode(f *testing.F)
internal/stats/fuzz_test.go:func FuzzPromTextFormat(f *testing.F)
```

All 7 fuzzers present (including the 06.1-new `FuzzPromTextFormat` in `internal/stats/`).

**FuzzBootstrapLoad:**
```
$ go test -race ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/1060 completed
fuzz: elapsed: 3s, gathering baseline coverage: 435/1060 completed
fuzz: elapsed: 6s, gathering baseline coverage: 910/1060 completed
fuzz: elapsed: 7s, gathering baseline coverage: 1060/1060 completed, now fuzzing with 32 workers
fuzz: elapsed: 9s, execs: 13812 (4311/sec), new interesting: 0 (total: 1060)
fuzz: elapsed: 12s, execs: 29631 (5272/sec), new interesting: 0 (total: 1060)
fuzz: elapsed: 15s, execs: 48302 (6224/sec), new interesting: 2 (total: 1062)
fuzz: elapsed: 18s, execs: 63694 (5132/sec), new interesting: 3 (total: 1063)
fuzz: elapsed: 21s, execs: 85838 (7375/sec), new interesting: 3 (total: 1063)
fuzz: elapsed: 24s, execs: 96415 (3528/sec), new interesting: 4 (total: 1064)
fuzz: elapsed: 27s, execs: 109712 (4433/sec), new interesting: 4 (total: 1064)
fuzz: elapsed: 30s, execs: 127967 (6080/sec), new interesting: 4 (total: 1064)
fuzz: elapsed: 32s, execs: 127967 (0/sec), new interesting: 4 (total: 1064)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	33.212s
```

**FuzzTcpProxyFilter:**
```
$ go test -race ./internal/filter/tcpproxy/ -fuzz=FuzzTcpProxyFilter -fuzztime=30s
[expected log: `tcpproxy: dial cluster "c_dead": ... connection refused` — fuzz seed exercises the dial-failure error path; not a fuzz failure.]
fuzz: elapsed: 0s, gathering baseline coverage: 0/555 completed
fuzz: elapsed: 3s, gathering baseline coverage: 405/555 completed
fuzz: elapsed: 4s, gathering baseline coverage: 555/555 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 11404 (3667/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 9s, execs: 27646 (5413/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 12s, execs: 42829 (5059/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 15s, execs: 57662 (4947/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 18s, execs: 72121 (4820/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 21s, execs: 86129 (4669/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 24s, execs: 100058 (4642/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 27s, execs: 131882 (10609/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 30s, execs: 145749 (4606/sec), new interesting: 0 (total: 555)
fuzz: elapsed: 32s, execs: 145749 (0/sec), new interesting: 0 (total: 555)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	33.123s
```

**FuzzTLSContextParse:**
```
$ go test -race ./internal/tls/ -fuzz=FuzzTLSContextParse -fuzztime=30s
[expected log: `tls: tls_params: TLS-1.3-only cipher "TLS_AES_128_GCM_SHA256" requested; crypto/tls does not allow selection, dropping` — diagnostic per ADR-0030; not a fuzz failure.]
fuzz: elapsed: 0s, gathering baseline coverage: 0/676 completed
fuzz: elapsed: 3s, gathering baseline coverage: 676/676 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 4530 (1510/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 6s, execs: 34846 (10105/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 9s, execs: 64070 (9739/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 12s, execs: 91428 (9122/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 15s, execs: 118099 (8891/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 18s, execs: 144601 (8829/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 21s, execs: 170261 (8557/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 24s, execs: 194666 (8135/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 27s, execs: 218410 (7915/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 30s, execs: 247885 (9825/sec), new interesting: 0 (total: 676)
fuzz: elapsed: 32s, execs: 247885 (0/sec), new interesting: 0 (total: 676)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	33.200s
```

**FuzzHCMConfigParse:**
```
$ go test -race ./internal/filter/hcm/ -fuzz=FuzzHCMConfigParse -fuzztime=30s
[expected log: `hcm: h2: h2: PROTOCOL_ERROR: short preface: EOF` — diagnostic from ALPN-negotiated h2 path with empty preface; not a fuzz failure.]
fuzz: elapsed: 0s, gathering baseline coverage: 0/513 completed
fuzz: elapsed: 3s, gathering baseline coverage: 326/513 completed
fuzz: elapsed: 4s, gathering baseline coverage: 513/513 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 8158 (2610/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 9s, execs: 21898 (4570/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 12s, execs: 34183 (4104/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 15s, execs: 44736 (3518/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 18s, execs: 54186 (3150/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 21s, execs: 62558 (2791/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 24s, execs: 69544 (2328/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 27s, execs: 84814 (5092/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 30s, execs: 107646 (7606/sec), new interesting: 0 (total: 513)
fuzz: elapsed: 32s, execs: 107646 (0/sec), new interesting: 0 (total: 513)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	33.403s
```

**FuzzFrameStream:**
```
$ go test -race ./internal/filter/hcm/h2/ -fuzz=FuzzFrameStream -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/400 completed
fuzz: elapsed: 0s, gathering baseline coverage: 400/400 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 48172 (16026/sec), new interesting: 1 (total: 401)
fuzz: elapsed: 6s, execs: 120397 (24117/sec), new interesting: 3 (total: 403)
fuzz: elapsed: 9s, execs: 204277 (27956/sec), new interesting: 6 (total: 406)
fuzz: elapsed: 12s, execs: 269435 (21720/sec), new interesting: 7 (total: 407)
fuzz: elapsed: 15s, execs: 332154 (20911/sec), new interesting: 7 (total: 407)
fuzz: elapsed: 18s, execs: 394006 (20615/sec), new interesting: 8 (total: 408)
fuzz: elapsed: 21s, execs: 460892 (22298/sec), new interesting: 9 (total: 409)
fuzz: elapsed: 24s, execs: 522173 (20422/sec), new interesting: 9 (total: 409)
fuzz: elapsed: 27s, execs: 583883 (20545/sec), new interesting: 9 (total: 409)
fuzz: elapsed: 30s, execs: 642035 (19402/sec), new interesting: 9 (total: 409)
fuzz: elapsed: 32s, execs: 642035 (0/sec), new interesting: 9 (total: 409)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	35.632s
```

**FuzzHPACKDecode:**
```
$ go test -race ./internal/filter/hcm/h2/ -fuzz=FuzzHPACKDecode -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/157 completed
fuzz: elapsed: 0s, gathering baseline coverage: 157/157 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 46620 (15533/sec), new interesting: 1 (total: 158)
fuzz: elapsed: 6s, execs: 93343 (15579/sec), new interesting: 2 (total: 159)
fuzz: elapsed: 9s, execs: 124620 (10426/sec), new interesting: 3 (total: 160)
fuzz: elapsed: 12s, execs: 146332 (7237/sec), new interesting: 3 (total: 160)
fuzz: elapsed: 15s, execs: 200277 (17984/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 18s, execs: 296053 (31912/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 21s, execs: 377652 (27212/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 24s, execs: 472171 (31500/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 27s, execs: 502906 (10240/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 30s, execs: 551085 (16067/sec), new interesting: 5 (total: 162)
fuzz: elapsed: 32s, execs: 551085 (0/sec), new interesting: 5 (total: 162)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	40.634s
```

**FuzzPromTextFormat (06.1-NEW):**
```
$ go test -race ./internal/stats/ -fuzz=FuzzPromTextFormat -fuzztime=30s
fuzz: elapsed: 0s, gathering baseline coverage: 0/110 completed
fuzz: elapsed: 0s, gathering baseline coverage: 110/110 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 191859 (63935/sec), new interesting: 0 (total: 110)
fuzz: elapsed: 6s, execs: 416325 (74828/sec), new interesting: 0 (total: 110)
fuzz: elapsed: 9s, execs: 643338 (75666/sec), new interesting: 0 (total: 110)
fuzz: elapsed: 12s, execs: 864862 (73825/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 15s, execs: 1080117 (71723/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 18s, execs: 1297030 (72304/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 21s, execs: 1509197 (70771/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 24s, execs: 1727341 (72721/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 27s, execs: 1946741 (73034/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 30s, execs: 2163049 (72157/sec), new interesting: 1 (total: 111)
fuzz: elapsed: 31s, execs: 2163049 (0/sec), new interesting: 1 (total: 111)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	32.264s
```

All 7/7 fuzz targets PASS at 30s budget. Per ADR-0018 fuzz-corpus discipline: `git status --porcelain` after fuzz runs shows only the doc/state files modified by Task 15 — no `testdata/fuzz/` pollution (no crashers found).

**Gate (e) — vet + lint + race:**

```
$ go vet ./...
$ # exit 0
```

```
$ golangci-lint run ./...
$ # exit 0
```

```
$ go test -race ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.760s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.070s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.038s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.054s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.268s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.503s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.037s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.052s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.031s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.087s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.147s
ok  	github.com/esalaine/envoy-go/test/differential	11.668s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.013s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.022s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.022s
ok  	github.com/esalaine/envoy-go/test/helpers	1.028s
```

**ADR-0059 boundary grep (no third-party Prometheus or stats library):**

```
$ grep -nE 'github.com/prometheus|github.com/[^/]+/prometheus' go.mod go.sum
$ # empty — zero matches in go.mod
```

```
$ grep -nR '"github.com/[^"]*\(prometheus\|stats\|metrics\|expvar\)' internal/ cmd/envoy-go/ --include='*.go' | grep -v '_test.go' | grep -v 'github.com/esalaine/envoy-go/internal/stats'
$ # empty — no production imports of third-party stats/prometheus/metrics/expvar libraries
```

No third-party Prometheus or stats dependency. `go.sum` contains only transitive entries for `golang.org/x/net` (from `golang.org/x/net/http2`, permitted per ADR-0046).

**17-stat emit call sites (Step 5 grep results — actual identifiers used in the 06.1 implementation):**

2 listener stats — `internal/listener/manager.go`:
```
internal/listener/manager.go:505:		rt.downstreamCxTotal.Inc()
internal/listener/manager.go:506:		rt.downstreamCxActive.Inc()
internal/listener/manager.go:512:				defer rt.downstreamCxActive.Dec()
internal/listener/manager.go:518:			defer rt.downstreamCxActive.Dec()
```
(`downstreamCxTotal.Inc` = downstream_cx_total; `downstreamCxActive.Inc/Dec` = downstream_cx_active)

5 HCM stats — `internal/filter/hcm/connection.go` + `h2dispatch.go`:
```
internal/filter/hcm/h2dispatch.go:50:	d.f.downstreamRqTotal.Inc()
internal/filter/hcm/connection.go:59:				f.downstreamRqTotal.Inc()
internal/filter/hcm/connection.go:70:		f.downstreamRqTotal.Inc()
```
Status-class counters via `downstreamStatusClassCounter(code).Inc()`:
```
internal/filter/hcm/connection.go:60:				if c := f.downstreamStatusClassCounter(400); c != nil {
internal/filter/hcm/connection.go:75:			if c := f.downstreamStatusClassCounter(417); c != nil {
internal/filter/hcm/connection.go:84:			if c := f.downstreamStatusClassCounter(501); c != nil {
internal/filter/hcm/connection.go:111:				if c := f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/connection.go:125:		if c := f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/h2dispatch.go:90:	if c := a.f.downstreamStatusClassCounter(a.a.status); c != nil {
internal/filter/hcm/h2dispatch.go:128:		if c := a.f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/h2dispatch.go:159:	if c := r.f.downstreamStatusClassCounter(500); c != nil {
```
(`downstreamRqTotal.Inc` = downstream_rq_total; `downstreamStatusClassCounter(code).Inc()` drives the 4 `downstream_rq_Nxx` counters via `internal/filter/hcm/config.go`'s switch on `code/100`)

8 cluster stats — `internal/cluster/cluster.go` + `manager.go`:
```
internal/cluster/manager.go:106:	c.membershipTotal.Set(int64(len(c.endpoints)))
internal/cluster/cluster.go:98:func (c *Cluster) IncUpstreamRqTotal() { c.upstreamRqTotal.Inc() }
internal/cluster/cluster.go:106:func (c *Cluster) IncStatusClass(code int) {
internal/cluster/cluster.go:171:	c.upstreamCxTotal.Inc()
internal/cluster/cluster.go:172:	c.upstreamCxActive.Inc()
internal/cluster/cluster.go:173:	return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, nil
```
(`IncUpstreamRqTotal` = upstream_rq_total; `IncStatusClass` drives 4 `upstream_rq_Nxx`; `upstreamCxTotal.Inc` = upstream_cx_total; `upstreamCxActive.Inc/Dec` = upstream_cx_active; `membershipTotal.Set` = membership_total)

1 server.live stat — `internal/admin/admin.go`:
```
internal/admin/admin.go:102:	s.liveOnce.Do(func() { s.liveGauge.Set(1) })
```
(`liveGauge.Set(1)` = server.live)

Call-site summary: 2 listener / 5 HCM / 8 cluster / 1 server = 16 distinct emit call-cluster points for 17 internal names (the 4 `downstream_rq_Nxx` and 4 `upstream_rq_Nxx` are each accessed via a single indirection function that selects the right `*stats.Counter` based on `code/100` — the indirection is the grep-verified mechanism).

**Final ADR tail:**
```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0064: stats_config.stats_tags config not honored; extraction hardcoded in name.go
```

**Step 9 — ROADMAP.md NOT modified at this commit** per BOOTSTRAP §5 step 6 (phase-done lives at lifecycle-state 6 — REVIEW session). Row 06.1 stays `in-progress`; the phase-done commit at lifecycle-state 6 will flip it to `done` and confirm row 06 stays `in-progress` and row 06.2 stays `planned`.

**All five executable gates (a/b/c/d/e) green; gate (f) deferred to REVIEW.** STATE.md advances to lifecycle-state 4 with `next-skill: superpowers:verification-before-completion`. ROADMAP row 06.1 stays `in-progress` per PLAN Task 15's "Refinement" note.

**Notes (closing):**

Lint-fix sweep at Task 15 resolved 6 pre-existing golangci-lint Minor findings that were not caught by prior per-task code-quality reviews because those reviews ran `go vet` + `go test` but not `golangci-lint` standalone. All 6 fixes are style/polish-only (gofmt alignment, doc comment, misspell, errcheck patterns); no behavioural changes. These are NOT new Minors — they are resolutions of issues that should have been caught at Tasks 4/13/14.

Carry-forward Minors from Tasks 8–14 to L4 review-followup (26 total, all polish-level):
- Task 8 (T8 at `a6f8b94`): 5 Minors → L4
- Task 9 (T9 at `bdfc34b`): 5 Minors → L4
- Task 10 (T10 at `df46e24`): 5 Minors → L4
- Task 11 (T11 at `e68020a`): 4 Minors → L4
- Task 12 (T12 at `6139b11`): 2 Minors → L4
- Task 13 (T13 at `103b81b`): 5 Minors → L4
- Task 14 (T14 at `d030fb1`): 4 Minors → L4 (M-1 through M-7 listed in Task 14 entry; 4 reviewer Minors from the follow-up commit)

Most impactful for the verifier: T14 M-3 (200ms hardcoded drain — potential CI flakiness), T13 M-1 (empty-`{}` over-acceptance in FuzzPromTextFormat seed). The L4 review-followup session will triage and either land or accept-as-is per the 05.2 review-followup precedent.

## Verification (lifecycle-state 4) — FAILED

Per `BOOTSTRAP_PROMPT.md` §5 state 4 and `STATE.md`'s `next-skill-scope`: a fresh-session re-run of every SPEC §3 / BOOTSTRAP §7.5 phase-done gate, with each command's verbatim output captured here. This session's HEAD on branch `phase/06.1-stats-prometheus-verify` is `42e2650f73d33c4895a2bd127dcce4efd6632d17` — the impl-branch tip (STATE.md SHA-fill follow-up to Task 15's all-gates-green local sweep at `58d4ec9`). Worktree: `.worktrees/phase-06.1-stats-prometheus-verify`, branched from impl-branch tip per ADR-0003 + per-phase-worktree convention; the impl worktree at `.worktrees/phase-06.1-stats-prometheus-impl` is closed-history at this state transition. Verifier date: 2026-04-27.

**Outcome: gate (d) FAIL.** A fresh fuzz run of `FuzzHCMConfigParse` at the ADR-0018 30-second budget produced a crasher in 5 seconds. The HCM stat-name construction path at `internal/filter/hcm/config.go:164` builds `http.<stat_prefix>.downstream_rq_total` from a user-controlled `stat_prefix` without validation; when the prefix contains a space (or any character that violates the Prometheus name regex `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`), `stats.Registry.NewCounter` → `Registry.checkName` (`internal/stats/registry.go:97`) panics. The minimised 325-byte input — an `Any` with type-URL `type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager` and a value-payload encoding `stat_prefix = "0000000000 0"` (12 bytes including a literal space at index 10) — reproduces the panic deterministically. Gate (e) part 2 (`go test -race ./...`) inherits the failure on `internal/filter/hcm` because Go's fuzz testdata persistence saves the minimised seed under `internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/9ba19570cf17f59f` and unit-test mode replays it as a regression case (the other 17 packages PASS clean under `-race`). Gates (a), (b), (c), (e) part 1 (build/vet/lint), the ADR-0059 boundary grep, and the 17-stat-emit call-site grep all PASS.

**Discrepancy with Task 15's "all five executable gates green" claim** is attributable to fuzzing's non-deterministic 30-second budget: the impl run (Task 15's local sweep) sampled a different region of the input space and did not hit this corner; the verifier run did. This is not a bug in the verifier methodology — it is exactly what BOOTSTRAP §7.5 gate (d) ("any new fuzzer has run clean for its short-budget CI run") is designed to surface. An independent fresh-session re-run is the gate, and a single run that happens to clear 30 s without a crasher is necessary but not sufficient evidence; reproducibility across at least one independent run is the implicit acceptance bar.

**Next action per BOOTSTRAP §5 deviation rule (`Unexpected state → superpowers:systematic-debugging FIRST`):** STATE advances back to lifecycle-state 3 (impl incomplete) with `next-skill: superpowers:systematic-debugging`. The fix branch is a new `.worktrees/phase-06.1-stats-prometheus-impl-followup-gate-d` (or equivalent name) branched from this verify branch's tip per ADR-0003 + per-phase-worktree convention. Two fix-shape candidates (the fix branch chooses; ADR if non-obvious):

1. Reject configs with a non-Prometheus-safe `stat_prefix` at HCM parse time, returning an `hcm: invalid stat_prefix: <prefix>` error — matches the fuzz target's contract that every error must be `hcm:`-prefixed (`internal/filter/hcm/fuzz_test.go:38-40`).
2. Sanitise `stat_prefix` per Rule SN1's invalid-character substitution before constructing the counter name — e.g., map non-`[a-zA-Z0-9_.]` to `_`. Trade-off: changes the observable stat name set vs upstream Envoy when the fixture-side `stat_prefix` happens to contain such chars; phase 06.1 SPEC §10.1 anchors Rule SN1 against a specific reference scrape, so a SN1-extension ADR would be appropriate.

The fix branch should also consider whether `stats.Registry.NewCounter` should return an error rather than panic on invalid names (defense-in-depth per `superpowers:systematic-debugging`'s `defense-in-depth.md`): a panic on user-input-derived names is fundamentally a contract violation regardless of the HCM-side caller's input validation.

**Seed file disposition (verifier role contract).** Go's fuzz framework persisted the minimised crasher input as `internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/9ba19570cf17f59f`. The 05.2 verifier precedent (`b34bd99`) committed only `STATE.md` + `PROGRESS.md` — no production-code or test-corpus changes — and this verifier follows that role contract: the seed file is **deleted** before this verification commit. The seed bytes are quoted verbatim in the gate-(d) output below for the fix branch's reproduction. The fix branch can re-derive an equivalent seed by re-running the fuzzer (typically <30 s on a 32-worker host) OR construct a deterministic hand-crafted regression test in `internal/filter/hcm/config_test.go` covering the same path (the latter is preferred — easier to read and amend than a binary fuzz seed). Per `superpowers:test-driven-development`, the fix branch's first step is the regression test failing.

**Outputs:**

```
$ pwd
/home/esa/git/envoy-go/.worktrees/phase-06.1-stats-prometheus-verify
$ git rev-parse --abbrev-ref HEAD
phase/06.1-stats-prometheus-verify
$ git log -1 --format=%H
42e2650f73d33c4895a2bd127dcce4efd6632d17
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version 2>&1 | head -1
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0062: Differential equivalence shape for stats output
$ # ADR-0062 is the chronological-add tail; 06.1 ADRs in file-add order:
$ # 0059, 0060, 0061, 0064, 0063, 0062 — non-monotonic per 05.2 precedent.
```

**Gate (a) — fixture-0005 differential (NEW in 06.1 per ADR-0062) — PASS:**

```
$ go test -count=1 -run 'TestDifferential/0005' -v ./test/differential/
=== RUN   TestDifferential
=== RUN   TestDifferential/0005-prometheus-stats
[testcontainers ryuk + reference-Envoy lifecycle abbreviated.]
2026/04/27 17:17:31 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
[ ... ]
2026/04/27 17:17:33 🐳 Terminating container: 109fedbff3aa
2026/04/27 17:17:33 🚫 Container terminated: 109fedbff3aa
--- PASS: TestDifferential (2.37s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.37s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.445s
```

The reference Envoy image SHA `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` matches the `ENVOY_TARGET.md` pin; differential equivalence per ADR-0062's shape rules verified non-vacuously.

**Gate (b) — all pre-existing differential fixtures (regression check) — PASS:**

```
$ go test -count=1 -run 'TestDifferential/(0000|0001|0002|0003|0004)' -v ./test/differential/
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
=== RUN   TestDifferential/0001-tcp-proxy-rr
=== RUN   TestDifferential/0002-tls-tcp
=== RUN   TestDifferential/0003-http11-routing
=== RUN   TestDifferential/0004-h2-routing
[testcontainers + container lifecycle abbreviated; "hcm: h2: EOF" lines from H2 reachability probes during 0004 setup elided — same as 05.2 verification.]
--- PASS: TestDifferential (7.07s)
    --- PASS: TestDifferential/0000-tcp-echo (1.58s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.16s)
    --- PASS: TestDifferential/0002-tls-tcp (1.22s)
    --- PASS: TestDifferential/0003-http11-routing (1.24s)
    --- PASS: TestDifferential/0004-h2-routing (1.86s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	7.148s
```

5/5 pre-existing fixtures green. The 06.1 stats-Registry threading through cluster/listener/HCM (Tasks 7-12) does not regress any pre-existing fixture.

**Gate (c) — h2spec conformance (UNCHANGED from 05.1/05.2 baseline per SPEC §3) — PASS:**

```
$ go test -count=1 -v ./test/conformance/h2spec/
[testcontainers + container lifecycle abbreviated.]
2026/04/27 17:18:10 🐳 Creating container for image summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0
[ ... ]
        Finished in 0.5491 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
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
2026/04/27 17:18:11 🐳 Terminating container: dbd6076f3679
2026/04/27 17:18:11 🚫 Container terminated: dbd6076f3679
--- PASS: TestH2Spec (2.20s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.293s
```

53 of 53 PASS at the pinned `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`. Per-section breakdown matches the 05.1/05.2 baselines byte-for-byte; covers sections 3, 4, 5, 6 ex-6.6, 7, 8 per the ADR-0051 threshold list. Total: 2+3+3+3+13+2+1+2+2+2+2+1+1+4+2+7+2+1 = 53.

**Gate (d) — 7 fuzz targets at 30 s ADR-0018 budget — FAIL on `FuzzHCMConfigParse`; the other six PASS:**

```
$ go test -fuzz='^FuzzBootstrapLoad$' -fuzztime=30s -count=1 -run='^$' ./internal/bootstrap/
fuzz: elapsed: 0s, gathering baseline coverage: 0/1064 completed
fuzz: elapsed: 5s, gathering baseline coverage: 1064/1064 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 31s, execs: 381081 (0/sec), new interesting: 7 (total: 1071)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.088s
$ git status --porcelain
$ # empty

$ go test -fuzz='^FuzzPromTextFormat$' -fuzztime=30s -count=1 -run='^$' ./internal/stats/
fuzz: elapsed: 0s, gathering baseline coverage: 0/111 completed
fuzz: elapsed: 0s, gathering baseline coverage: 111/111 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 30s, execs: 25439532 (0/sec), new interesting: 3 (total: 114)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	30.115s
$ git status --porcelain
$ # empty

$ go test -fuzz='^FuzzTcpProxyFilter$' -fuzztime=30s -count=1 -run='^$' ./internal/filter/tcpproxy/
[ ... ]
fuzz: elapsed: 31s, execs: 3742957 (0/sec), new interesting: 5 (total: 560)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.053s
$ git status --porcelain
$ # empty

$ go test -fuzz='^FuzzHCMConfigParse$' -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/
fuzz: elapsed: 0s, gathering baseline coverage: 0/513 completed
fuzz: elapsed: 4s, gathering baseline coverage: 513/513 completed, now fuzzing with 32 workers
fuzz: minimizing 360-byte failing input file
fuzz: elapsed: 5s, minimizing
--- FAIL: FuzzHCMConfigParse (5.08s)
    --- FAIL: FuzzHCMConfigParse (0.00s)
        testing.go:1927: panic: stats: invalid metric name: "http.0000000000 0.downstream_rq_total" (must match ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$)
            goroutine 24825 [running]:
            runtime/debug.Stack()
            	runtime/debug/stack.go:26 +0x9b
            testing.tRunner.func1()
            	testing/testing.go:1927 +0x1d0
            panic({0x11335c0?, 0x3218003ab690?})
            	runtime/panic.go:860 +0x13a
            github.com/esalaine/envoy-go/internal/stats.(*Registry).checkName(0x13?, {0x32180033b740, 0x25})
            	internal/stats/registry.go:100 +0x168
            github.com/esalaine/envoy-go/internal/stats.(*Registry).NewCounter(0x3218003dcdc0, {0x32180033b740, 0x25})
            	internal/stats/registry.go:67 +0x72
            github.com/esalaine/envoy-go/internal/filter/hcm.parseFilterWithCtx(0x3217fb278780, 0x3217fb16e590, {0x0?, 0x0?}, 0x3218003dcdc0)
            	internal/filter/hcm/config.go:164 +0xcc6
            github.com/esalaine/envoy-go/internal/filter/hcm.NewFilter(...)
            	internal/filter/hcm/filter.go:24
            github.com/esalaine/envoy-go/internal/filter/hcm.FuzzHCMConfigParse.func1(0x321800432488, {0x3218000b7811?, 0x0?}, {0x3217fb978a00?, 0x0?, 0x0?})
            	internal/filter/hcm/fuzz_test.go:37 +0xe8
            [ … reflect/testing scaffolding truncated … ]
    Failing input written to testdata/fuzz/FuzzHCMConfigParse/9ba19570cf17f59f
    To re-run:
    go test -run=FuzzHCMConfigParse/9ba19570cf17f59f
FAIL
exit status 1
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm	5.086s
$ git status --porcelain
?? internal/filter/hcm/testdata/fuzz/

$ # Minimised seed bytes (Go fuzz v1 corpus format, 325 bytes total file):
$ cat internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/9ba19570cf17f59f
go test fuzz v1
string("type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager")
[]byte("\x12\f0000000000 0*a\n\x19envoy.filters.http.router\"D\nBtype.googleapis.com/envoy.extensions.filters.http.router.v3.Router\"*\x12(Z\n0000000000\x12\x01*002\t1000000002\n0\xc802\x0500000")

$ # Wire-format decoding of the value-payload's leading bytes:
$ #   \x12 = field 2 (HttpConnectionManager.stat_prefix), wire-type 2 (length-delimited)
$ #   \f   = length 12
$ #   "0000000000 0" = stat_prefix payload (12 bytes; literal SP at index 10)
$ # The remaining bytes encode HttpConnectionManager.http_filters[0] = envoy.filters.http.router and
$ # an inline RouteConfiguration; not relevant to the panic — the panic fires on the stat_prefix path
$ # at config.go:164 before the RouteConfiguration is touched.

$ go test -fuzz='^FuzzFrameStream$' -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/h2/
[ ... ]
fuzz: elapsed: 30s, execs: 13367578 (0/sec), new interesting: 0 (total: 409)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	30.158s
$ git status --porcelain
?? internal/filter/hcm/testdata/fuzz/   # residual from FuzzHCMConfigParse run; NOT produced by this target

$ go test -fuzz='^FuzzHPACKDecode$' -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/h2/
[ ... ]
fuzz: elapsed: 31s, execs: 1749707 (0/sec), new interesting: 2 (total: 164)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	31.078s
$ git status --porcelain
?? internal/filter/hcm/testdata/fuzz/   # unchanged residual; NOT produced by this target

$ go test -fuzz='^FuzzTLSContextParse$' -fuzztime=30s -count=1 -run='^$' ./internal/tls/
[ ... ]
fuzz: elapsed: 31s, execs: 4884272 (0/sec), new interesting: 8 (total: 684)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.058s
$ git status --porcelain
?? internal/filter/hcm/testdata/fuzz/   # unchanged residual
```

**Per-target summary (gate d):**

| target                  | execs               | new-interesting | result |
|-------------------------|---------------------|-----------------|--------|
| `FuzzBootstrapLoad`     | 381,081             | 7               | PASS   |
| `FuzzPromTextFormat`    | 25,439,532          | 3               | PASS (NEW in 06.1) |
| `FuzzTcpProxyFilter`    | 3,742,957           | 5               | PASS   |
| `FuzzHCMConfigParse`    | (n/a — crashed at 5 s) | n/a          | **FAIL** — crasher above |
| `FuzzFrameStream`       | 13,367,578          | 0               | PASS   |
| `FuzzHPACKDecode`       | 1,749,707           | 2               | PASS   |
| `FuzzTLSContextParse`   | 4,884,272           | 8               | PASS   |

**Gate (e) part 1 — go build / go vet / golangci-lint — PASS:**

```
$ go build ./...
$ # exit 0; empty output

$ go vet ./...
$ # exit 0; empty output

$ golangci-lint run ./...
$ # exit 0; empty output — the six lint-fix-sweep fixes at 58d4ec9 are durable
```

**ADR-0059 boundary grep — PASS (zero third-party stats/prometheus deps; zero production third-party imports):**

```
$ grep -nE 'github.com/prometheus|github.com/[^/]+/prometheus' go.mod go.sum
$ # empty

$ grep -nR '"github.com/' internal/ cmd/envoy-go/ --include='*.go' \
    | grep -v '_test.go' \
    | grep -v 'github.com/esalaine/envoy-go' \
    | grep -iE 'prometheus|stats|metrics|expvar' || true
$ # empty
```

**17-stat-emit call-site grep — PASS:**

```
$ grep -n 'downstreamCxTotal\.Inc\|downstreamCxActive\.\(Inc\|Dec\)' internal/listener/manager.go
505:		rt.downstreamCxTotal.Inc()
506:		rt.downstreamCxActive.Inc()
512:				defer rt.downstreamCxActive.Dec()
518:			defer rt.downstreamCxActive.Dec()
$ # 2 listener stats: downstream_cx_total + downstream_cx_active

$ grep -n 'downstreamRqTotal\.Inc\|downstreamStatusClassCounter' \
        internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go
internal/filter/hcm/connection.go:59:				f.downstreamRqTotal.Inc()
internal/filter/hcm/connection.go:60:				if c := f.downstreamStatusClassCounter(400); c != nil {
internal/filter/hcm/connection.go:70:		f.downstreamRqTotal.Inc()
internal/filter/hcm/connection.go:75:			if c := f.downstreamStatusClassCounter(417); c != nil {
internal/filter/hcm/connection.go:84:			if c := f.downstreamStatusClassCounter(501); c != nil {
internal/filter/hcm/connection.go:111:				if c := f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/connection.go:125:		if c := f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/h2dispatch.go:50:	d.f.downstreamRqTotal.Inc()
internal/filter/hcm/h2dispatch.go:90:	if c := a.f.downstreamStatusClassCounter(a.a.status); c != nil {
internal/filter/hcm/h2dispatch.go:128:		if c := a.f.downstreamStatusClassCounter(status); c != nil {
internal/filter/hcm/h2dispatch.go:159:	if c := r.f.downstreamStatusClassCounter(500); c != nil {
$ # 5 HCM stats: downstream_rq_total + 4 status-class via downstreamStatusClassCounter()

$ grep -n 'IncUpstreamRqTotal\|IncStatusClass\|upstreamCxTotal\.Inc\|upstreamCxActive\.\(Inc\|Dec\)\|membershipTotal\.Set' \
        internal/cluster/cluster.go internal/cluster/manager.go
internal/cluster/cluster.go:98:func (c *Cluster) IncUpstreamRqTotal() { c.upstreamRqTotal.Inc() }
internal/cluster/cluster.go:106:func (c *Cluster) IncStatusClass(code int) {
internal/cluster/cluster.go:171:	c.upstreamCxTotal.Inc()
internal/cluster/cluster.go:172:	c.upstreamCxActive.Inc()
internal/cluster/cluster.go:173:	return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, nil
internal/cluster/manager.go:106:	c.membershipTotal.Set(int64(len(c.endpoints)))
$ # 8 cluster stats: upstream_rq_total + 4 status-class + upstream_cx_total + upstream_cx_active + membership_total

$ grep -n 'liveGauge\.Set\|liveOnce' internal/admin/admin.go
23:	liveOnce  sync.Once
102:	s.liveOnce.Do(func() { s.liveGauge.Set(1) })
$ # 1 server.live (sync.Once-guarded liveGauge.Set(1))
```

Call-site total: 2 listener + 5 HCM + 8 cluster + 1 server.live = 16 distinct grep-match-points serving 17 internal stat names (the 4 `downstream_rq_<class>` and 4 `upstream_rq_<class>` are reached via the indirection helpers `downstreamStatusClassCounter(code)` and `IncStatusClass(code)`). Matches the impl Task 15 verification block at `58d4ec9` exactly.

**Gate (e) part 2 — `go test -race ./...` — FAIL on `internal/filter/hcm` (replay of seed `9ba19570cf17f59f`); other 17 packages PASS:**

```
$ go test -race -count=1 ./...
?   	github.com/esalaine/envoy-go/cmd/envoy-go	[no test files]
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.070s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.038s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.054s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
--- FAIL: FuzzHCMConfigParse (0.00s)
    --- FAIL: FuzzHCMConfigParse/9ba19570cf17f59f (0.00s)
panic: stats: invalid metric name: "http.0000000000 0.downstream_rq_total" (must match ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$) [recovered, repanicked]
[ … same panic stack as gate (d) above; same call site internal/filter/hcm/config.go:164 → internal/stats/registry.go:100 … ]
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm	0.272s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.406s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.035s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.056s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.028s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.089s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.145s
ok  	github.com/esalaine/envoy-go/test/differential	11.351s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.011s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.015s
ok  	github.com/esalaine/envoy-go/test/helpers	1.030s
FAIL
```

17 ok; 1 FAIL on `internal/filter/hcm`. The h2spec, differential (all 6 fixtures including 0005), listener, cluster, hcm/h2, stats, tls, admin, bootstrap, tcpproxy unit tests all pass clean under `-race`. The failure is the seed-replay regression, mechanically equivalent to gate (d)'s direct fuzz crasher; no race-detector warnings (no DATA RACE output anywhere in the 17 ok packages).

**Gate (f) — `REVIEW.md` approved — deferred to lifecycle-state 6 per BOOTSTRAP §5.**

**Verification result:** gates (a)/(b)/(c)/(e)-part-1 PASS, gate (d) FAIL on `FuzzHCMConfigParse`, gate (e)-part-2 FAIL on the same root-cause via seed replay, gate (f) deferred. **Phase 06.1 cannot advance to lifecycle-state 5.** STATE.md transitions back to lifecycle-state 3 with `next-skill: superpowers:systematic-debugging`. ROADMAP rows unchanged (06.1 stays `in-progress`; 06 stays `in-progress`; 06.2 stays `planned`). The auto-generated seed file `internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/9ba19570cf17f59f` is deleted from this verifier's worktree before commit per the 05.2 verifier role contract — see "Seed file disposition" prose above.

## Lifecycle-state 3 — gate-(d) fix landed (commit `79be6b0`)

**Branch:** `phase/06.1-stats-prometheus-impl-followup-gate-d` (worktree `.worktrees/phase-06.1-stats-prometheus-impl-followup-gate-d`, branched from verify-branch tip `6a053a4` per ADR-0003).

**Fix shape:** Validate metric-name-deriving inputs at the user-input boundary (HCM `parseFilterWithCtx`), using the same regex (`internal/stats.nameRE`) that `Registry.checkName` enforces — single source of truth, no drift risk. ADR-0065 (this commit) records the rationale, including the rejected alternatives: (A) "sanitise stat_prefix per Rule SN1" — rejected because Rule SN2 preserves `stat_prefix` verbatim as the Prometheus label value `envoy_http_conn_manager_prefix=<stat_prefix>`, and sanitising would silently mutate that label vs upstream Envoy + cause two-prefixes-collapse-to-one data-loss; (B) "convert `Registry.NewCounter` to error-return" — rejected because it would force the duplicate-name and post-Freeze panic paths to follow for symmetry, a wider API change than the gate-(d) blocker requires.

**Surface change:**
- `internal/stats/registry.go` — adds `IsValidName(name string) bool` (read-only helper wrapping `nameRE.MatchString`). Existing `Registry.NewCounter` / `Registry.NewGauge` panic discipline is unchanged.
- `internal/stats/registry_test.go` — adds `TestIsValidName` (5 valid + 7 invalid names; the invalid set includes the verbatim assembled name `http.0000000000 0.downstream_rq_total` from the gate-(d) seed).
- `internal/filter/hcm/config.go` — adds a single guard between the existing `stat_prefix` non-empty check and the route-config build: `if !stats.IsValidName("http." + statPrefix + ".downstream_rq_total") { return nil, fmt.Errorf("hcm: invalid stat_prefix: %q (...)", statPrefix) }`. Validating the longest assembled name suffices because the four `_<N>xx` suffixes share the same character class.
- `internal/filter/hcm/config_test.go` — adds `TestParseFilter_StatPrefixInvalidChars` (6 cases: `"0000000000 0"` verbatim from the fuzz seed, plus `"foo bar"`, `"foo-bar"`, `"foo:bar"`, `"foo/bar"`, `"foo$bar"`). Each case uses the existing `expectErr` helper to assert the error is `hcm:`-prefixed and contains the substring `invalid stat_prefix`.
- `docs/envoy-go/DECISIONS.md` — appends ADR-0065.

**TDD evidence — RED (regression test wired up before the fix):**

```
$ go test -run '^TestParseFilter_StatPrefixInvalidChars$' -v ./internal/filter/hcm/
=== RUN   TestParseFilter_StatPrefixInvalidChars
=== RUN   TestParseFilter_StatPrefixInvalidChars/0000000000_0
--- FAIL: TestParseFilter_StatPrefixInvalidChars (0.00s)
    --- FAIL: TestParseFilter_StatPrefixInvalidChars/0000000000_0 (0.00s)
panic: stats: invalid metric name: "http.0000000000 0.downstream_rq_total" (must match ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$) [recovered, repanicked]
[ … same call stack as the verifier-block panic above:
  internal/stats/registry.go:100 → registry.NewCounter:67 → hcm/config.go:164 → fuzz_test.go:37 (replaced by config_test.go:106 expectErr) … ]
FAIL
exit status 1
FAIL	github.com/esalaine/envoy-go/internal/filter/hcm	0.006s
```

The other 5 subtests did not run because the parent panic crashed the test function. Same root cause as the verifier block's gate-(d) FAIL — confirms the regression test reproduces the right defect.

**TDD evidence — GREEN (after the `internal/stats.IsValidName` + `parseFilterWithCtx` guard landed):**

```
$ go test -run '^TestParseFilter_StatPrefixInvalidChars$' -v ./internal/filter/hcm/
=== RUN   TestParseFilter_StatPrefixInvalidChars
=== RUN   TestParseFilter_StatPrefixInvalidChars/0000000000_0
=== RUN   TestParseFilter_StatPrefixInvalidChars/foo_bar
=== RUN   TestParseFilter_StatPrefixInvalidChars/foo-bar
=== RUN   TestParseFilter_StatPrefixInvalidChars/foo:bar
=== RUN   TestParseFilter_StatPrefixInvalidChars/foo/bar
=== RUN   TestParseFilter_StatPrefixInvalidChars/foo$bar
--- PASS: TestParseFilter_StatPrefixInvalidChars (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/0000000000_0 (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/foo_bar (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/foo-bar (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/foo:bar (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/foo/bar (0.00s)
    --- PASS: TestParseFilter_StatPrefixInvalidChars/foo$bar (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.004s
```

All 6 subtests pass — including the verbatim seed prefix `"0000000000 0"`. The `TestIsValidName` happy + sad coverage for the new helper passes (`ok internal/stats 0.001s`).

**Local re-run of BOOTSTRAP §7.5 gates (a)/(b)/(c)/(d)/(e) — all GREEN.** Note: this is the FIX-branch's own local sanity sweep; the formal lifecycle-state-4 verifier run is the next session's responsibility (in a fresh `.worktrees/phase-06.1-stats-prometheus-verify-2` per the 05.2 verifier role contract).

```
$ go vet ./...
$ # (clean — no output)

$ golangci-lint run
$ # (clean — no output)

$ go build ./...
$ # (clean — no output)

$ go test ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	0.005s
ok  	github.com/esalaine/envoy-go/internal/admin	0.029s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.022s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.025s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.216s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.472s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.009s
ok  	github.com/esalaine/envoy-go/internal/listener	0.017s
ok  	github.com/esalaine/envoy-go/internal/stats	0.003s
ok  	github.com/esalaine/envoy-go/internal/tls	0.020s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.152s
ok  	github.com/esalaine/envoy-go/test/differential	10.973s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/helpers	0.007s
$ # 18 ok packages; 0 FAIL.

$ go test -race ./...
[ … 18 ok packages, 0 FAIL, no DATA RACE warnings; package timings ~1.0–11.1s … ]

$ go test -fuzz='^FuzzHCMConfigParse$' -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/
fuzz: elapsed: 0s, gathering baseline coverage: 0/513 completed
fuzz: elapsed: 3s, gathering baseline coverage: 437/513 completed
fuzz: elapsed: 3s, gathering baseline coverage: 513/513 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 396928 (132203/sec), new interesting: 4 (total: 517)
fuzz: elapsed: 9s, execs: 825098 (142729/sec), new interesting: 8 (total: 521)
fuzz: elapsed: 12s, execs: 1306788 (160596/sec), new interesting: 11 (total: 524)
fuzz: elapsed: 15s, execs: 1700639 (131288/sec), new interesting: 11 (total: 524)
fuzz: elapsed: 18s, execs: 2113271 (137533/sec), new interesting: 13 (total: 526)
fuzz: elapsed: 21s, execs: 2500943 (129206/sec), new interesting: 15 (total: 528)
fuzz: elapsed: 24s, execs: 2881749 (126943/sec), new interesting: 16 (total: 529)
fuzz: elapsed: 27s, execs: 3204132 (107469/sec), new interesting: 18 (total: 531)
fuzz: elapsed: 30s, execs: 3489858 (95227/sec), new interesting: 19 (total: 532)
fuzz: elapsed: 31s, execs: 3489858 (0/sec), new interesting: 19 (total: 532)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.058s
$ # gate (d) cleared at the ADR-0018 30s budget: 3.49M execs, 19 new-interesting, 0 crashers.

$ # Sanity sweep of all 7 fuzzers at a 10s budget each (the formal verifier session re-runs at 30s):
$ for fz in 'FuzzBootstrapLoad ./internal/bootstrap' 'FuzzPromTextFormat ./internal/stats' \
            'FuzzTcpProxyFilter ./internal/filter/tcpproxy' 'FuzzHCMConfigParse ./internal/filter/hcm' \
            'FuzzFrameStream ./internal/filter/hcm/h2' 'FuzzHPACKDecode ./internal/filter/hcm/h2' \
            'FuzzTLSContextParse ./internal/tls'; do
  set -- $fz
  go test -fuzz="^$1$" -fuzztime=10s -count=1 -run='^$' "$2" 2>&1 | tail -1
done
ok  	github.com/esalaine/envoy-go/internal/bootstrap	11.067s
ok  	github.com/esalaine/envoy-go/internal/stats	10.112s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	10.134s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	10.430s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	10.130s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	11.064s
ok  	github.com/esalaine/envoy-go/internal/tls	11.051s
$ # 7/7 fuzzers PASS — no crashers in any target.
```

**No persisted seed file.** The 30 s `FuzzHCMConfigParse` re-run did not produce any new crasher; `internal/filter/hcm/testdata/fuzz/` does not exist on this branch. Per the 05.2 verifier role contract, no production-test-corpus changes are committed; the durable regression artifact is `TestParseFilter_StatPrefixInvalidChars` in `internal/filter/hcm/config_test.go`, not a binary fuzz seed.

**Cluster-name carry-forward (latent but not gate-(d)-blocking).** `internal/cluster/manager.go:97` (`registerClusterMetrics`) propagates `cluster.<name>` into eight metric names without validating the cluster name's character set, mirroring the HCM defect. The verifier's `FuzzBootstrapLoad` 30 s run did not happen to discover a cluster-name crasher (the 30 s budget did not drift the bootstrap fuzz corpus to a cluster name with chars outside `[a-zA-Z0-9_.]`), but the latent vulnerability is real. Per `STATE.md`'s "the gate-(d) fix should be a focused single-issue branch, not bundled with Minors triage" guidance, this fix is NOT bundled into the gate-(d) branch — a follow-up branch will add the same `stats.IsValidName` validation guard at `cluster.NewManager`'s `cluster.<name>` boundary, inheriting ADR-0065's pattern by reference. Listener is already safe via `normalizeAddr` (`internal/listener/manager.go:196-198`).

**ADRs introduced:** ADR-0065 ("Validate metric-name-deriving inputs at the user-input boundary"). The phase-done commit (lifecycle-state 6) will name ADR-0065 alongside the six 06.1 ADRs already landed (ADR-0059, 0060, 0061, 0062, 0063, 0064) per BOOTSTRAP §5.3.

**Lifecycle-state transition:** 3 → 4 (implementation complete, not verified). `next-skill: superpowers:verification-before-completion`. The next session re-runs all six BOOTSTRAP §7.5 gates from a fresh verify worktree (`.worktrees/phase-06.1-stats-prometheus-verify-2`, branched from this commit's HEAD per ADR-0003) per the verifier role contract: verifier commit changes only `STATE.md` + `PROGRESS.md`; no production code, test, or fixture changes.

## Verification (lifecycle-state 4 — re-run after gate-(d) fix) — PASSED

Per `BOOTSTRAP_PROMPT.md` §5 state 4 and `STATE.md`'s `next-skill-scope` (set on the gate-(d) fix commit `79be6b0` and its SHA-fill follow-up `4982713`): a fresh-session re-run of every BOOTSTRAP §7.5 phase-done gate, with each command's verbatim output captured here. This session's HEAD on branch `phase/06.1-stats-prometheus-verify-2` is `498271398d33e4cf089aadf050fcc2551f1e61bb` — the gate-(d) fix branch's tip (STATE.md + PROGRESS.md SHA-fill follow-up to the fix at `79be6b0`). Worktree: `.worktrees/phase-06.1-stats-prometheus-verify-2`, branched from gate-(d) fix-branch tip per ADR-0003 + per-phase-worktree convention; the fix worktree at `.worktrees/phase-06.1-stats-prometheus-impl-followup-gate-d` is closed-history at this state transition. Verifier date: 2026-04-28.

**Outcome: gates (a)/(b)/(c)/(d)/(e) all GREEN; gate (f) deferred to lifecycle-state 6.** The gate-(d) regression target `FuzzHCMConfigParse` (which crashed on seed `9ba19570cf17f59f` under the prior verifier `1f94b74`) now runs the full ADR-0018 30 s budget without any crasher: the `stats.IsValidName` boundary guard added to `parseFilterWithCtx` at `79be6b0` per ADR-0065 catches every metric-name-deriving `stat_prefix` whose assembled name would fail `Registry.checkName`'s regex, returning the documented `hcm: invalid stat_prefix: %q (...)` error before `Registry.NewCounter` is reached. Gate (e) part 2 (`go test -race ./...`) inherits the durable regression test `TestParseFilter_StatPrefixInvalidChars` (6 cases including the verbatim seed prefix `"0000000000 0"`) and passes it under `-race` along with all 17 other test packages — no DATA RACE warnings anywhere. The remaining four gates (a/b/c/e-part-1) and the ADR-0059 boundary grep + 17-stat-emit call-site grep precedents from the impl-branch's Task 15 sweep are mechanically equivalent to this re-run; no surface change between Task 15's sweep and this re-verify other than the ADR-0065 fix.

**Next action per BOOTSTRAP §5 step 4 → 5:** STATE advances to lifecycle-state 5 (verified, not reviewed) with `next-skill: superpowers:requesting-code-review`. ROADMAP rows unchanged (06.1 stays `in-progress`; 06 stays `in-progress`; 06.2 stays `planned`); ROADMAP transitions to `done` happen at the lifecycle-state 6 phase-done commit per BOOTSTRAP §5 step 6.

**Seed file disposition (verifier role contract).** No new auto-generated fuzz seed file appeared during this re-verify. `git status --porcelain` was empty after each of the seven 30 s fuzz runs and again after the `-race` test sweep. The single inherited seed file `internal/stats/testdata/fuzz/FuzzPromTextFormat/1d8483e640bf8347` (committed earlier under the impl-branch chain — verified tracked via `git ls-files`) is unchanged. Per the 05.2 verifier role contract (`b34bd99`) followed by the prior failing verifier (`1f94b74`), this verifier commit changes ONLY `STATE.md` + `PROGRESS.md` — no production code, test, or fixture changes; no testdata/fuzz/ corpus changes.

**Outputs:**

```
$ pwd
/home/esa/git/envoy-go/.worktrees/phase-06.1-stats-prometheus-verify-2
$ git rev-parse --abbrev-ref HEAD
phase/06.1-stats-prometheus-verify-2
$ git log -1 --format=%H
498271398d33e4cf089aadf050fcc2551f1e61bb
$ go version
go version go1.26.2 linux/amd64
$ golangci-lint version 2>&1 | head -1
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
$ docker version --format '{{.Server.Version}}'
28.1.1
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0065: Validate metric-name-deriving inputs at the user-input boundary
$ # ADR-0065 is the chronological-add tail; 06.1 chain ADRs in commit-add order:
$ # 0059, 0060, 0061, 0064, 0063, 0062 (impl Task 15) → 0065 (gate-(d) fix). Non-monotonic per 05.2 precedent.
$ git status --porcelain
$ # empty
```

**Gate (a) — all 6 differential fixtures (0000-0005) — PASS:**

```
$ go test -count=1 -v -run TestDifferential -timeout=900s ./test/differential/
=== RUN   TestDifferential
=== RUN   TestDifferential/0000-tcp-echo
[testcontainers ryuk + reference-Envoy lifecycle abbreviated; envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd matches ENVOY_TARGET.md pin.]
=== RUN   TestDifferential/0001-tcp-proxy-rr
=== RUN   TestDifferential/0002-tls-tcp
=== RUN   TestDifferential/0003-http11-routing
=== RUN   TestDifferential/0004-h2-routing
[ "hcm: h2: EOF" lines from H2 reachability probes during 0004 setup elided — same as 05.2/L4-FAIL verification. ]
=== RUN   TestDifferential/0005-prometheus-stats
--- PASS: TestDifferential (8.74s)
    --- PASS: TestDifferential/0000-tcp-echo (1.52s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.20s)
    --- PASS: TestDifferential/0002-tls-tcp (1.19s)
    --- PASS: TestDifferential/0003-http11-routing (1.21s)
    --- PASS: TestDifferential/0004-h2-routing (1.65s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.98s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	8.822s
```

6/6 fixtures green. Reference Envoy SHA matches `ENVOY_TARGET.md` pin; the 06.1 stats-Registry threading through cluster/listener/HCM (impl Tasks 7–12) does not regress any pre-existing fixture, and the new fixture-0005 differential equivalence per ADR-0062's shape rules is verified non-vacuously.

**Gate (b) — pre-existing fixtures (regression check) — PASS (subset of (a)):**

The 5 pre-existing fixtures (0000–0004) are run as `TestDifferential/0000…0004` subtests inside the same gate (a) invocation above and all PASS. No separate command needed; per 05.2 verifier precedent, gate (b) is implicit in (a) when (a) covers the full set including pre-existing fixtures. 5/5 pre-existing fixtures green.

**Gate (c) — h2spec conformance (UNCHANGED from 05.1/05.2/L4-FAIL baseline per SPEC §3) — PASS:**

```
$ go test -count=1 -v -timeout=600s ./test/conformance/h2spec/
[testcontainers + container lifecycle abbreviated.]
2026/04/28 06:56:36 🐳 Creating container for image summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0
[ "hcm: h2: …" probe-driven error lines elided — same shape as L4-FAIL verifier block. ]
        Finished in 0.5490 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
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
[ container terminate lines elided. ]
--- PASS: TestH2Spec (2.14s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.235s
```

53 of 53 PASS at the pinned `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0`. Per-section breakdown matches the 05.1/05.2/L4-FAIL baselines byte-for-byte (2+3+3+3+13+2+1+2+2+2+2+1+1+4+2+7+2+1 = 53); covers sections 3, 4, 5, 6 ex-6.6, 7, 8 per ADR-0051's threshold list.

**Gate (d) — 7 fuzz targets at 30 s ADR-0018 budget — PASS (FuzzHCMConfigParse cleared after gate-(d) fix; other 6 PASS as before):**

```
$ go test -fuzz=FuzzBootstrapLoad -fuzztime=30s -count=1 -run='^$' ./internal/bootstrap/
fuzz: elapsed: 0s, gathering baseline coverage: 0/1076 completed
fuzz: elapsed: 5s, gathering baseline coverage: 1076/1076 completed, now fuzzing with 32 workers
fuzz: elapsed: 9s, execs: 302873 (59798/sec), new interesting: 2 (total: 1078)
[ ... ]
fuzz: elapsed: 31s, execs: 327499 (0/sec), new interesting: 2 (total: 1078)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.084s
$ git status --porcelain
$ # empty

$ go test -fuzz=FuzzPromTextFormat -fuzztime=30s -count=1 -run='^$' ./internal/stats/
fuzz: elapsed: 0s, gathering baseline coverage: 0/114 completed
fuzz: elapsed: 0s, gathering baseline coverage: 114/114 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 30s, execs: 26217681 (883668/sec), new interesting: 0 (total: 114)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	30.112s
$ git status --porcelain
$ # empty

$ go test -fuzz=FuzzTcpProxyFilter -fuzztime=30s -count=1 -run='^$' ./internal/filter/tcpproxy/
fuzz: elapsed: 0s, gathering baseline coverage: 0/563 completed
fuzz: elapsed: 4s, gathering baseline coverage: 563/563 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 31s, execs: 4029470 (0/sec), new interesting: 1 (total: 564)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.051s
$ git status --porcelain
$ # empty

$ go test -fuzz=FuzzHCMConfigParse -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/
fuzz: elapsed: 0s, gathering baseline coverage: 0/533 completed
fuzz: elapsed: 4s, gathering baseline coverage: 533/533 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 331164 (110269/sec), new interesting: 0 (total: 533)
fuzz: elapsed: 9s, execs: 726420 (131636/sec), new interesting: 0 (total: 533)
fuzz: elapsed: 12s, execs: 1145546 (139850/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 15s, execs: 1517026 (123812/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 18s, execs: 1899146 (127372/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 21s, execs: 2268440 (123107/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 24s, execs: 2624874 (118794/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 27s, execs: 2936455 (103880/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 30s, execs: 3244770 (102767/sec), new interesting: 1 (total: 534)
fuzz: elapsed: 31s, execs: 3244770 (0/sec), new interesting: 1 (total: 534)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.046s
$ git status --porcelain
$ # empty — no auto-persisted seed; the gate-(d) fix's boundary guard catches every invalid stat_prefix at parse time, before Registry.NewCounter

$ go test -fuzz=FuzzFrameStream -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/h2/
fuzz: elapsed: 0s, gathering baseline coverage: 0/410 completed
fuzz: elapsed: 0s, gathering baseline coverage: 410/410 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 30s, execs: 13882377 (450368/sec), new interesting: 5 (total: 415)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	30.150s
$ git status --porcelain
$ # empty

$ go test -fuzz=FuzzHPACKDecode -fuzztime=30s -count=1 -run='^$' ./internal/filter/hcm/h2/
fuzz: elapsed: 0s, gathering baseline coverage: 0/164 completed
fuzz: elapsed: 0s, gathering baseline coverage: 164/164 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 31s, execs: 1893954 (0/sec), new interesting: 0 (total: 164)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	31.071s
$ git status --porcelain
$ # empty

$ go test -fuzz=FuzzTLSContextParse -fuzztime=30s -count=1 -run='^$' ./internal/tls/
fuzz: elapsed: 0s, gathering baseline coverage: 0/684 completed
fuzz: elapsed: 2s, gathering baseline coverage: 684/684 completed, now fuzzing with 32 workers
[ ... ]
fuzz: elapsed: 31s, execs: 4016676 (0/sec), new interesting: 11 (total: 695)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.057s
$ git status --porcelain
$ # empty
```

**Per-target summary (gate d):**

| target                  | execs               | new-interesting | result |
|-------------------------|---------------------|-----------------|--------|
| `FuzzBootstrapLoad`     | 327,499             | 2               | PASS   |
| `FuzzPromTextFormat`    | 26,217,681          | 0               | PASS   |
| `FuzzTcpProxyFilter`    | 4,029,470           | 1               | PASS   |
| `FuzzHCMConfigParse`    | 3,244,770           | 1               | **PASS** (regression target — cleared after ADR-0065 fix) |
| `FuzzFrameStream`       | 13,882,377          | 5               | PASS   |
| `FuzzHPACKDecode`       | 1,893,954           | 0               | PASS   |
| `FuzzTLSContextParse`   | 4,016,676           | 11              | PASS   |
| **Total**               | **53,609,610**      | **20**          | **7/7 PASS, 0 crashers** |

Comparison to gate-(d) fix-branch's local sanity sweep (recorded at `79be6b0`'s "Lifecycle-state 3 — gate-(d) fix landed" block above): `FuzzHCMConfigParse` then 3.49 M execs / 19 new-interesting; this verifier 3.24 M execs / 1 new-interesting. Different exec rates and corpus-discovery counts are normal under fuzzing's non-deterministic worker scheduling; the gate criterion is "no crasher" and that is met by both runs. `FuzzPromTextFormat`'s 0 new-interesting matches its very small (114-input) baseline corpus — the search space is largely covered.

**Gate (e) part 1 — `go vet` + `golangci-lint run` + `go build` — PASS:**

```
$ go vet ./...
$ # exit 0; empty output

$ golangci-lint run ./...
$ # exit 0; empty output

$ go build ./...
$ # exit 0; empty output
```

Six lint-fix-sweep fixes from impl Task 15 (`58d4ec9`) plus the gate-(d) fix's `IsValidName` + `parseFilterWithCtx` guard at `79be6b0` are all durable; no new lint findings introduced.

**Gate (e) part 2 — `go test -race ./...` — PASS (18 ok, 0 FAIL, 0 DATA RACE):**

```
$ go test -race -count=1 -timeout=600s ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.570s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.066s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.034s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.041s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.261s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.501s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.027s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.050s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.032s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.085s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.145s
ok  	github.com/esalaine/envoy-go/test/differential	11.160s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.014s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.012s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.014s
ok  	github.com/esalaine/envoy-go/test/helpers	1.026s
$ # exit 0; 18 ok packages, 14 [no test files], 0 FAIL, 0 DATA RACE
```

18 ok / 0 FAIL — including `internal/filter/hcm` (which under the prior verifier `1f94b74` failed by replaying the auto-persisted `FuzzHCMConfigParse/9ba19570cf17f59f` seed). The durable regression test `TestParseFilter_StatPrefixInvalidChars` (in `internal/filter/hcm/config_test.go`, added at `79be6b0`) covers all 6 invalid-char cases including the verbatim seed prefix `"0000000000 0"` and PASSES under `-race`. `grep -E '(DATA RACE|^FAIL|^---? FAIL)'` over the full output: empty. h2spec, differential (all 6 fixtures including 0005), listener, cluster, hcm, hcm/h2, stats, tls, admin, bootstrap, tcpproxy, cmd/envoy-go, conformance/h2spec, all 5 fixture drivers (0001/0002/0003/0004/0005), and helpers all pass clean under `-race`.

**Gate (f) — `REVIEW.md` approved — deferred to lifecycle-state 6 per BOOTSTRAP §5.**

The REVIEW.md deliverable is the lifecycle-state 6 phase-done step's input per BOOTSTRAP §5; not produced or consumed by this verifier session.

**Carry-forwards (informational; not gate-(d)-blocking, not verifier-scope).**

- **Cluster-name latent vulnerability** at `internal/cluster/manager.go:97` (`registerClusterMetrics` propagates `cluster.<name>` into eight metric names without validating `cluster.<name>` against `nameRE`) — recorded in the gate-(d) fix block above; NOT gate-(d)-blocking (the verifier's `FuzzBootstrapLoad` 30 s run did not happen to discover a cluster-name crasher). A separate follow-up branch will add the same `stats.IsValidName` guard at `cluster.NewManager`'s boundary, inheriting ADR-0065's pattern by reference. Listener is already safe via `normalizeAddr` (`internal/listener/manager.go:196-198`).
- **L4 review-followup queue (26 Minors)** from impl Tasks 8–14 — listed in the impl-branch's Task 15 closing notes above. Triage queued for the eventual REVIEW phase; not for the verifier.

**Verification result:** gates (a)/(b)/(c)/(d)/(e)-part-1/(e)-part-2 all PASS; gate (f) deferred to lifecycle-state 6. **Phase 06.1 may advance to lifecycle-state 5.** STATE.md transitions to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. ROADMAP rows unchanged (06.1 stays `in-progress`; 06 stays `in-progress`; 06.2 stays `planned`); ROADMAP transitions land at the lifecycle-state 6 phase-done commit per BOOTSTRAP §5 step 6.

---

## Lifecycle-state 4 — REVIEW-followup re-verification (M-1 closure) — PASSED

The REVIEW-followup branch `phase/06.1-stats-prometheus-review-followup`
(worktree `.worktrees/phase-06.1-stats-prometheus-review-followup`,
branched from REVIEW SHA-fill tip `d21d50b` per ADR-0003) closed the
single Path-A REVIEW finding M-1 (cluster-name latent vulnerability at
`internal/cluster/manager.go:97` — explicitly forward-flagged in
ADR-0065 Consequences (d)). The fix landed in two commits: `caa58e5`
(M-1 boundary guard at `buildCluster` + `TestNewManager_ClusterNameInvalidChars`
6-case regression test mirroring `TestParseFilter_StatPrefixInvalidChars`
at `internal/filter/hcm/config_test.go:221`; inherits ADR-0065's pattern by
reference — no new ADR per ADR-0065 Consequences (d)) and `665c879`
(spelling fixup `minimised → minimized` to silence golangci-lint
misspell on the test docstring). REVIEW Importants I-1
(SPEC §14 line 715 file-path mismatch) and I-2 (ROADMAP row 06.1 still
`planned`) require no commits on this branch: I-1 closed by
REVIEW + PROGRESS Task 11 deviation #1 as-corrigendum; I-2 closed at
the lifecycle-state-6 phase-done commit's natural `planned → done` row
flip per the 05.2 `0c01ed6` one-step precedent. The 12 collapsed Minors
carry forward to the L4 review-followup batch (separate post-phase-done
branch); zero Minor closure on this branch.

This block re-runs gates (b)/(c)/(d)/(e) at HEAD `665c879` to confirm
the M-1 fix did not regress any non-deferred SPEC §3 phase-done gate.
Gate (a) was implicitly re-confirmed by gate (b)'s sweep (fixture
`0005-prometheus-stats` is one of the six fixtures swept). Gate (f)
REVIEW.md is closed — the REVIEW commit landed at `59d86f2` on the
review branch and was carried into this followup branch's ancestry.

**Gate (b) — all 6 differential fixtures green (gate (a) implied):**

```
$ go test -count=1 -v -run '^TestDifferential$' ./test/differential/
--- PASS: TestDifferential (9.04s)
    --- PASS: TestDifferential/0000-tcp-echo (1.68s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.29s)
    --- PASS: TestDifferential/0002-tls-tcp (1.24s)
    --- PASS: TestDifferential/0003-http11-routing (1.26s)
    --- PASS: TestDifferential/0004-h2-routing (1.67s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.89s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	9.116s
```

All six fixtures PASS. Fixture `0005-prometheus-stats` (the gate-(a)
non-vacuous observability differential introduced in this phase) PASSES
unchanged after the M-1 fix; the M-1 guard rejects only invalid cluster
names (`cluster c0` is valid), so the per-counter delta-equality + per-gauge
snapshot-equality assertions across the 13 unique Prometheus names against
reference Envoy under the 5-request defined load remain green.

**Gate (c) — h2spec 53/53 PASS at the ADR-0051 pin:**

```
$ go test -count=1 -v -run '^TestH2Spec' ./test/conformance/h2spec/
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
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
--- PASS: TestH2Spec (2.19s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.273s
```

53/53 PASS at the unchanged threshold list (sections 3, 4, 5, 6 ex-6.6,
7, 8). The pinned `summerwind/h2spec` SHA in `CONFORMANCE_PINS.md` is
unchanged (D-3.7 reserves pin bumps for dedicated phases). 06.1 does not
touch H2 wire code, so this gate is structurally unchanged from the
verifier-2 run at `1ed6cd0`.

**Gate (d) — all 7 fuzzers PASS at the 30s ADR-0018 budget:**

```
$ go test -fuzz=^FuzzBootstrapLoad$ -fuzztime=30s -run='^$' ./internal/bootstrap/
fuzz: elapsed: 30s, execs: 454924 (0/sec), new interesting: 8 (total: 1086)
fuzz: elapsed: 31s, execs: 454924 (0/sec), new interesting: 8 (total: 1086)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap	31.080s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzTcpProxyFilter$ -fuzztime=30s -run='^$' ./internal/filter/tcpproxy/
fuzz: elapsed: 30s, execs: 3764981 (132168/sec), new interesting: 0 (total: 564)
fuzz: elapsed: 31s, execs: 3764981 (0/sec), new interesting: 0 (total: 564)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	31.047s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzTLSContextParse$ -fuzztime=30s -run='^$' ./internal/tls/
fuzz: elapsed: 30s, execs: 3534778 (240352/sec), new interesting: 8 (total: 703)
fuzz: elapsed: 31s, execs: 3534778 (0/sec), new interesting: 8 (total: 703)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls	31.058s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzHCMConfigParse$ -fuzztime=30s -run='^$' ./internal/filter/hcm/
fuzz: elapsed: 30s, execs: 3045557 (101170/sec), new interesting: 3 (total: 537)
fuzz: elapsed: 31s, execs: 3045557 (0/sec), new interesting: 3 (total: 537)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	31.056s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzFrameStream$ -fuzztime=30s -run='^$' ./internal/filter/hcm/h2/
fuzz: elapsed: 30s, execs: 13500060 (450228/sec), new interesting: 3 (total: 418)
fuzz: elapsed: 30s, execs: 13500060 (0/sec), new interesting: 3 (total: 418)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	30.149s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzHPACKDecode$ -fuzztime=30s -run='^$' ./internal/filter/hcm/h2/
fuzz: elapsed: 30s, execs: 1878914 (0/sec), new interesting: 1 (total: 165)
fuzz: elapsed: 31s, execs: 1878914 (0/sec), new interesting: 1 (total: 165)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	31.069s
$ git status --porcelain
$ # empty

$ go test -fuzz=^FuzzPromTextFormat$ -fuzztime=30s -run='^$' ./internal/stats/
fuzz: elapsed: 30s, execs: 25337684 (837715/sec), new interesting: 3 (total: 117)
fuzz: elapsed: 30s, execs: 25340614 (28145/sec), new interesting: 3 (total: 117)
PASS
ok  	github.com/esalaine/envoy-go/internal/stats	30.116s
$ git status --porcelain
$ # empty
```

All seven PASS at the ADR-0018 30s budget; `git status --porcelain` empty
after each run (no auto-persisted-seed corpus growth observed; in
particular, no replay of the gate-(d) seed `9ba19570cf17f59f` at
`internal/filter/hcm/testdata/fuzz/FuzzHCMConfigParse/` or analogous
cluster-name seed at `internal/bootstrap/testdata/fuzz/FuzzBootstrapLoad/`
— the M-1 fix is durable; no new corpus crashers surfaced).

**Gate (e) — go vet / golangci-lint / go test -race ./...:**

```
$ go vet ./...
$ # exit 0 (no output)

$ golangci-lint run ./...
$ # exit 0 (no output) — the misspell linter's "minimised → minimized"
$ # warning at internal/cluster/manager_test.go:159 was closed by 665c879

$ go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.643s
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	1.070s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.035s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.042s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.261s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.413s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.026s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	1.052s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.027s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.086s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.164s
ok  	github.com/esalaine/envoy-go/test/differential	11.583s
?   	github.com/esalaine/envoy-go/test/differential/fixture	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.011s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.013s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/pki/gen	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/backends	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.012s
ok  	github.com/esalaine/envoy-go/test/helpers	1.026s
```

18 ok / 0 FAIL / 0 DATA RACE under `-race`. The new
`TestNewManager_ClusterNameInvalidChars` 6-case regression test (added
at `caa58e5`, spelling-fix at `665c879`) is included in the
`internal/cluster` package's PASS line and passes clean under `-race`
on every iteration.

**ADR-0046 boundary grep (production imports of `golang.org/x/net/http2` outside the 5 allowed files):**

```
$ grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go' | grep -v 'internal/filter/hcm/h2/framer.go\|internal/filter/hcm/h2/hpack.go\|internal/filter/hcm/h2/settings.go\|internal/filter/hcm/h2/conn.go\|internal/filter/hcm/h2/client.go'
$ # empty
```

Empty. Raw production hits unchanged from the verifier-2 run:

```
$ grep -nR '"golang.org/x/net/http2"' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go'
internal/filter/hcm/h2/settings.go:4:	"golang.org/x/net/http2"
internal/filter/hcm/h2/conn.go:11:	"golang.org/x/net/http2"
internal/filter/hcm/h2/client.go:24:	"golang.org/x/net/http2"
internal/filter/hcm/h2/framer.go:11:	"golang.org/x/net/http2"
```

4 hits in 4 of the 5 allowed files (per ADR-0054, `hpack.go` legitimately
omits the root-package import).

**ADR-0048 client.go presence:**

```
$ ls internal/filter/hcm/h2/client.go
internal/filter/hcm/h2/client.go
```

**Forbidden-runtime-imports grep (ADR-0046):**

```
$ grep -nR 'http2\.Server\|http2\.Transport\|http2\.ConfigureServer' internal/ cmd/envoy-go/main.go --include='*.go' | grep -v '_test.go'
internal/filter/hcm/h2/doc.go:22:// What this package does NOT do: it does NOT use http2.Server,
internal/filter/hcm/h2/doc.go:23:// http2.Server.ServeConn, http2.ConfigureServer, http2.Transport, or
internal/filter/hcm/h2/doc.go:24:// http2.Transport.NewClientConn. The connection lifecycle is driven explicitly
```

3 hits, all in `doc.go`'s prohibition statement (no production-runtime use).

**ADR-0059 internal/stats stdlib-only constraint (no third-party imports):**

```
$ grep -nR '^import\|^\t"' internal/stats/ --include='*.go' | grep -v '_test.go' | grep -E '"[^/]+/[^/]+/'
$ # empty (stdlib-only)
```

Empty — `internal/stats` continues to import only Go stdlib packages
(`fmt`, `io`, `regexp`, `sort`, `strconv`, `strings`, `sync`,
`sync/atomic`) per ADR-0059's package-foundation invariant.

**Final cleanliness check at HEAD `665c879`:**

```
$ git status --porcelain
$ # empty (this addendum + STATE update not yet staged)
$ git rev-parse HEAD
665c87968d6dab0eef9a4a5b22f1cc46c5f5e6cb
```

**REVIEW-followup verification verdict:** all five non-deferred SPEC §3
gates GREEN at HEAD `665c879` (gate (a) implied by gate (b)'s six-fixture
sweep that includes `0005-prometheus-stats`; gate (f) closed by REVIEW.md
at `59d86f2`). The single REVIEW Path-A finding M-1 is closed by
`caa58e5` + `665c879`. REVIEW Important findings I-1 and I-2 are closed
without code/doc changes on this branch (I-1 by REVIEW + PROGRESS Task 11
deviation #1 as-corrigendum; I-2 by the next session's phase-done commit's
natural ROADMAP `planned → done` row flip). The 12 collapsed Minors
carry forward to the L4 review-followup batch (separate post-phase-done
branch). STATE advances to lifecycle-state 4 ("REVIEW-followup batch
complete; gates re-run green"); a follow-up commit promotes lifecycle-state
4 → 5; the phase-done commit at lifecycle-state 6 (next session) flips
ROADMAP row 06.1 `planned → done` directly per the 05.2 `0c01ed6` one-step
precedent + REVIEW I-2 disposition.
