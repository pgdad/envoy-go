# Phase 40.1 IMPL — PROGRESS

Passive outlier detection (`outlier_detection.consecutive_5xx`) — the FIRST passive-health construct over the phase-39 `clusterHealth`/`hostHealth` substrate. Executed subagent-driven per `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/PLAN.md` (14 tasks). **STATUS: IMPL DONE (2026-06-16)**

## IMPL base commit

`30abaed`

## Baselines captured (pre-IMPL, at worktree HEAD)

- **`go build ./...`** — PASS (clean)
- **`go vet ./...`** — PASS (clean)
- **`gofmt -l internal/ test/`** — PASS (empty output — no drift)
- **`go test ./internal/... 2>&1 | tail -20`** — PASS (all packages ok)
- **Full differential suite** (`go test ./test/differential/ -count=1`) — **70-dir GREEN** (224s)
- **Stat surface** — **1132** (carried forward from phase-39.2; verified arithmetically: 1132 + 5 = 1137 at exit)

## Starting counts (pre-IMPL)

- stat surface: **1132** · fixtures: **70** · fuzzers: **42** · BackendKind tail: **34** (`GRPCHealthResponder`) · DECISIONS tail: **ADR-0244** (next-free ADR-0245)

## Anticipated exit deltas (SPEC §14)

| Count | Before | After |
|---|---|---|
| Stat surface | 1132 | **1137** (+5 outlier_detection stats) |
| Fixtures | 70 | **71** (`0069-outlier-detection-consecutive-5xx`) |
| Fuzzers | 42 | **42** (unchanged) |
| BackendKind tail | 34 | **35** (`HTTP503Responder`) |
| DECISIONS tail | ADR-0244 | **ADR-0245** (next-free ADR-0246) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |

ROADMAP row 40 STAYS `in-progress` (40.2/40.3 legs pending — row flips `done` only when ALL three legs land).

## Task checklist

- [x] Task 1 — baselines/anchors + PROGRESS.md
- [x] Task 2 — ejection dimension on `hostHealth` + `isEjected`/`available`/`availableCount`
- [x] Task 3 — move LB pick filter `isHealthy → available` + panic denominator → `availableCount` (byte-stability gate)
- [x] Task 4 — `parseOutlierDetection` + reject roster + `clusterHealth`-creation widening
- [x] Task 5 — `outlierDetector` consecutive_5xx detect/eject/cap/enforce-roll + unit tests
- [x] Task 6 — `RecordUpstreamResult` seam + detector construction
- [x] Task 7 — wire `RecordUpstreamResult` into the live H1/H2 router success sites
- [x] Task 8 — +5 `outlier_detection` stat registrations (1132 → 1137)
- [x] Task 9 — `HTTP503Responder` BackendKind 35 + `PerHostBackendKind` runner override
- [x] Task 10 — `0069` cross-side outlier-detection-consecutive-5xx fixture
- [x] Task 11 — `0069` deliberate breaks + 20-run flake
- [x] Task 12 — full 71-dir differential + six-gate
- [x] Task 13 — ADR-0245 body + BEHAVIOR_CONTRACT passive-health subsection (stat 1132 → 1137)
- [x] Task 14 — completion bundle (STATE/ROADMAP/PROGRESS/README + next-prompt roll-forward)

## Final six-gate result (Task 12)

All six gates GREEN:

- **`go build ./...`** — PASS (clean)
- **`go vet ./...`** — PASS (clean)
- **`gofmt -l internal/ test/`** — PASS (empty output — no drift)
- **`golangci-lint run ./...`** — PASS (clean)
- **`go test ./internal/... 2>&1 | tail -20`** — PASS (all packages ok)
- **Full differential suite** (`go test ./test/differential/ -count=1`) — **71-dir GREEN**

## Task 11 deliberate-break results

- **Break A** (detector no-op — `outlierDetector.Record` returns immediately without tracking 5xx): `TestDifferential/0069` FAIL as required (the 503 host is never ejected; traffic still routes to it; `ejections_active` never converges to 1 on either side).
- **Break B** (LB ignores `available` — reverts the `available` predicate back to `isHealthy`): `TestDifferential/0069` FAIL as required (the ejected host stays in LB rotation; traffic continues hitting the 503 host after nominal ejection).
- Both breaks restored. **20/20 flake-free** on `TestDifferential/0069` (`-count=1`).

## As-built exit counts

| Count | Before | After | Delta |
|---|---|---|---|
| Stat surface | 1132 | **1137** | +5 (`ejections_active` gauge + `ejections_enforced_total` + `ejections_overflow` + `ejections_detected_consecutive_5xx` + `ejections_enforced_consecutive_5xx`) |
| Fixtures | 70 | **71** | +1 (`0069-outlier-detection-consecutive-5xx`) |
| Fuzzers | 42 | **42** | 0 (UNCHANGED) |
| BackendKind tail | 34 | **35** | +1 (`HTTP503Responder`) |
| DECISIONS tail | ADR-0244 | **ADR-0245** | +1 (next-free ADR-0246) |
| New Go packages | — | — | 0 |
| New go.mod modules | — | — | 0 |

ROADMAP row 40 STAYS `in-progress` (40.2/40.3 legs pending — row flips `done` only when ALL three legs land, per `reference_roadmap_split_phase_row_done`).
