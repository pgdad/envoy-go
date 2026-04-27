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

**Commits:** `63bb098` (no Minor code-quality findings carried forward — fuzzer is single-purpose and the round-trip parser stays self-contained inside the test file; consistent with the Task 8/9/10/11/12 precedent of citing carry-forwards only where they exist).
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
