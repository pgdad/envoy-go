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

**Commits:** TBD (SHA-fill follow-up)
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
