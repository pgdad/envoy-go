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
