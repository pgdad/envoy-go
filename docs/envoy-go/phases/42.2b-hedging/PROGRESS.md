# Phase 42.2b (hedging) — IMPL PROGRESS

Worktree: `/home/esa/git/envoy-go-42.2b-impl` (branch `phase-42.2b-impl`)
Plan: `docs/envoy-go/phases/42.2b-hedging/PLAN.md`

## Green baselines (Task 1, captured from worktree root)

```
$ go build ./... && go vet ./...
BUILD_VET_CLEAN   # build + vet clean, no output

$ gofmt -l internal/ test/        # expect empty
(empty — 0 lines)

$ go test ./internal/filter/http/router/... ./internal/filter/hcm/... ./internal/cluster/...
ok  github.com/esalaine/envoy-go/internal/filter/http/router  1.298s
ok  github.com/esalaine/envoy-go/internal/filter/hcm          0.013s
ok  github.com/esalaine/envoy-go/internal/filter/hcm/h2       2.471s
ok  github.com/esalaine/envoy-go/internal/cluster             0.139s
```

All baselines GREEN.

## Count baselines

| Surface          | Baseline | Anticipated at IMPL-done            |
|------------------|----------|-------------------------------------|
| stat surface     | 1181     | **1181 UNCHANGED** (AMEND-H5)       |
| fixtures         | 78       | 79 (new `0077`)                     |
| fuzzers          | 42       | 42 (unchanged)                      |
| BackendKind tail | 36       | 36 (unchanged)                      |
| DECISIONS tail   | ADR-0250 | ADR-0251 (next-free was ADR-0251)   |
| packages/modules | —        | ZERO new packages/modules           |

## As-built counts (IMPL-done, 2026-06-21)

| Surface          | Baseline | As built                            |
|------------------|----------|-------------------------------------|
| stat surface     | 1181     | **1181 UNCHANGED** (AMEND-H5 — a hedge REUSES the 42.1 `upstream_rq_retry*` counters; NO new stat name) |
| fixtures         | 78       | **79** (`0077-hedge-on-per-try-timeout`; the fan-out is subject-side UNIT-tested — NO `0078` dir) |
| fuzzers          | 42       | **42** (config-parse is unit-tested — no new fuzzer) |
| BackendKind tail | 36       | **36** (REUSE `BlockingHoldResponder` — no new BackendKind) |
| DECISIONS tail   | ADR-0250 | **ADR-0251** (accepted IN-PLACE per ADR-0044; next-free **ADR-0252**) |
| packages/modules | —        | **ZERO** new packages/modules (`go mod tidy -diff` zero-diff in Task 11) |

**FINAL ADR-0045 split-gate re-check:** **NO split.** The as-built LoC + the 13-task count stayed under the ADR-0045 gate (the SEPARATE `internal/filter/http/router/hedge.go` sibling reuses the 42.1/42.2a attempt substrate concurrently, so the executor surface stayed small); 42.2b lands as a single sub-leg.

## Tasks

- [x] Task 1: Baselines + PROGRESS scaffold
- [x] Task 2: HedgePolicy type + triggersConcurrency() + initial_requests>=1 reject
- [x] Task 3: hedgeExecutorH1 skeleton — hp struct field + goroutine + buffered collector + loser cancel
- [x] Task 4: per-attempt HEDGE-TRIGGER timer + hedge_on_per_try_timeout (AMEND-H1/H2)
- [x] Task 5: hedgeExecutorH2 (mirror; H2 ctx-cancel-aware)
- [x] Task 6: budget-counted fan-out (initial_requests + additional_request_chance)
- [x] Task 7: hedge_policy parse + vhost fallback + clusterRouteAction.hedgePolicy carry
- [x] Task 8: wire the dispatch — widen H{1,2}ClusterAction + closure-switch hedge branch
- [x] Task 9: 0077-hedge-on-per-try-timeout cross-side fixture (PASSES both sides)
- [x] Task 10: 0077 deliberate breaks (2) + 20-run flake + -race
- [x] Task 11: full 79-dir six-gate (all GREEN, +-race)
- [x] Task 12: ADR-0251 body + BEHAVIOR_CONTRACT hedging sub-block
- [x] Task 13: completion bundle + ROADMAP row 42 -> done + next-prompt

## Task 10 — 0077 deliberate-break proof (assertions are LIVE)

Both load-bearing assertions of `0077-hedge-on-per-try-timeout` were proven live via
deliberate breaks of `internal/filter/http/router/hedge.go`'s `hedgeExecutorH1`
collector (`case <-hedgeReq:`), each run with `-count=1` (cache-bypass), then restored
with `git restore` (HEAD stayed on `phase-42.2b-impl`; hedge.go byte-identical after —
`git diff --stat` empty).

- **Break A — neuter the hedge launch** (replaced the launch body with `continue`,
  keeping `limitExceededOnce` used so it compiled): the hedge never fires, so the
  driver's poll-to-converge on `upstream_rq_retry==3` trips its deadline.
  FAIL: `subject: hedge launch: subject: cluster.c_hedge.upstream_rq_retry delta did
  not converge to 3 within 10s (last delta 0)`. → proves assertion (A) (hedge launches
  on per-try-timeout) is LIVE.
  - First attempt at Break A (`continue` only) left `limitExceededOnce` declared-and-
    not-used → compile error (NOT an assertion bite); corrected to keep the var used.
- **Break B — the AMEND-H1 proof** (added `a.cluster.IncUpstreamRqPerTryTimeout()` on
  the hedge-launch path): the per_try_timeout delta becomes non-zero.
  FAIL: `subject: cluster.c_hedge.upstream_rq_per_try_timeout delta = 3, want 0`.
  → proves assertion (B) (a hedge does NOT increment `upstream_rq_per_try_timeout`,
  AMEND-H1) is LIVE.

Gates after the final restore:
- Restored single clean run: PASS.
- Flake gate: `-count=20` → **20/20 PASS** (no startup flakes encountered).
- Race gate: `go test ./internal/filter/http/router/ -run 'TestHedgeExecutor' -race
  -count=1` → **PASS** (concurrent collector is `-race`-clean).

## Task 11 — full 79-dir six-gate (whole repo GREEN)

Ran from the worktree root `/home/esa/git/envoy-go-42.2b-impl` (branch
`phase-42.2b-impl`). All six gates GREEN; no gofmt/lint fixes were required (this
commit records the gate results only).

- **Gate 1-2 (build + vet):** `go build ./... && go vet ./...` → clean.
- **Gate 3 (gofmt):** `gofmt -l internal/ test/` → **empty** (no drift).
- **Gate 4 (golangci-lint):** `golangci-lint run internal/filter/http/router/...
  internal/filter/hcm/... internal/cluster/...` → **clean** (zero findings).
- **Gate 5 (unit -race):** `go test ./internal/... -race -count=1` → **PASS**
  (59 `ok` packages, no FAIL, exit 0).
- **Gate 6 (full differential):** `go test ./test/differential/ -count=1` →
  **PASS** (`ok ... 240.199s`, exit 0). **79/79 fixture subtests PASS**, zero FAIL;
  `0077-hedge-on-per-try-timeout` PASS (3.73s) — the 79th dir. No startup flake
  encountered (single clean run; no isolate-re-run needed).
- **go.mod hygiene:** `go mod tidy -diff` → **zero diff** (no new module).
