# Phase 43.1 IMPL — PROGRESS

Connection-pool budgets (`max_connections` hard-cap + `max_pending_requests` bounded wait-queue) — the project's FIRST standing upstream request queue. A per-cluster DEFAULT-priority `connPool` (`internal/cluster/connpool.go`) carries the `max_connections`/`max_pending_requests` budgets + a `sync.Mutex`-guarded `activeConns` counter + a FIFO slice of buffered cap-1 wake channels. A connection-creation permit is try-acquired at the two creation boundaries (`Cluster.Dial` raw-dial + the `AcquireH1` pool-MISS); a pool HIT reuses an already-counted conn (no permit). At the cap the request PENDS (bounded by `max_pending_requests`); queue-full → fail-fast `503` + `upstream_rq_pending_overflow`. A recorded DEPARTURE from the reference's soft `max_connections` (AMEND-CP1; ADR-0252). Byte-identical when no `circuit_breakers`. Executed subagent-driven per `docs/envoy-go/phases/43-connection-pooling/PLAN.md` (12 tasks). **STATUS: IMPL DONE — all 12 tasks landed; six-gate GREEN; leg 43.1 complete.**

## IMPL base commit

`e03d4729` (`phase 43.1 (connection-pooling) PLAN: the 12-task TDD spine for the max_connections HARD-CAP + the max_pending_requests bounded wait-queue; D-S431-* resolved (sibling connpool.go, sync.Mutex+FIFO buffered-channel wake, 1181→1183); plan-document-reviewer APPROVED (docs-only)`) — master tip; worktree `phase-43.1-connection-pooling` branched from it, HEAD at Task-1 start. This is the squash anchor at stage-close.

## Baselines captured (pre-IMPL, at worktree HEAD `e03d4729`, 2026-06-22)

- **`go build ./...`** — PASS (clean, exit 0)
- **`go vet ./...`** — PASS (clean, exit 0)
- **`gofmt -l internal/ test/`** — PASS (empty output — no drift, exit 0)
- **`go test ./internal/...`** — PASS (all packages `ok`, ZERO failures; every package including `internal/admin` bound cleanly — no external port-10000 container conflict present at this baseline)
- **Full differential suite** (`go test ./test/differential/ -count=1`) — **79-subtest GREEN** (240.7s; `ok github.com/esalaine/envoy-go/test/differential`). No `subject ready: EOF` startup-race flakes observed; the full suite ran clean on the FIRST pass. No isolate-re-run needed.
- **Stat surface** — **1181** (SPEC §14 baseline; tracked as a documented running total — no count script). The 43.1 exit total is verified ARITHMETICALLY (1181 + 2 = 1183) against the Task 4 registration test (+2 new CB-scoped shapes: `upstream_rq_pending_active` gauge + `upstream_rq_pending_total` counter).

Worktree started GREEN — clean baseline to land the connection-pool budgets on. Docker available; `envoyproxy/envoy:contrib-v1.37.2` present. Next-free `refContainerListenerPort` confirmed **19167** (max in-use port across all fixtures is 19166, per `grep -rh refContainerListenerPort test/fixtures/ | grep -o '[0-9]\{4,5\}' | sort -un | tail -1`).

## Starting counts (pre-IMPL)

- stat surface: **1181** · fixtures: **79** · fuzzers: **42** · BackendKind tail: **36** (`BlockingHoldResponder`) · DECISIONS tail: **ADR-0251** (next-free **ADR-0252**)

## Anticipated exit deltas (SPEC §14 + D-S431-3)

| Axis | Before | After | Note |
|------|--------|-------|------|
| Stat surface | 1181 | **1183** | +2 new CB-scoped shapes (`upstream_rq_pending_active` gauge + `upstream_rq_pending_total` counter); 3 existing shapes (`circuit_breakers.default.cx_open`, `circuit_breakers.default.rq_pending_open`, `upstream_cx_overflow`) ACTIVATED from emit-0 — +0 surface delta (already registered in the 1181 baseline); the new `0078` fixture's CB cluster reuses the same shapes — +0 |
| Fixtures | 79 | **80** | `0078-connection-pool-max-connections` |
| Fuzzers | 42 | **42** | UNCHANGED (no new fuzzer warranted) |
| BackendKind tail | 36 | **36** | UNCHANGED — `0078` reuses `BlockingHoldResponder` (36) with the additive `/__release_sticky` control path (not a new BackendKind) |
| DECISIONS tail | ADR-0251 | **ADR-0252** | next-free ADR-0253 |
| New Go packages | — | 0 | `connpool.go` is a new FILE in the existing `internal/cluster` package |
| New go.mod modules | — | 0 | |
| Phase-41 stats flipped LIVE | emit-0 | `circuit_breakers.default.cx_open`, `circuit_breakers.default.rq_pending_open`, `upstream_cx_overflow` — no surface delta (already registered) | |
| `max_connections` departure | soft (reference) | **hard-cap** (envoy-go) | recorded as AMEND-CP1 / ADR-0252 |
| ROADMAP row 43 | in-progress | **in-progress** | leg 43.1 → done; row flips done only when 43.2 (H2 multiplex pool) also lands (reference_roadmap_split_phase_row_done) |

## Task checklist (12 tasks)

- [x] **Task 1** — pre-IMPL baselines + PROGRESS.md scaffold (THIS commit). Six-gate baseline captured (build/vet/gofmt/internal/differential); counts pinned 1181 / 79 / 42 / 36 / ADR-0251.
- [x] **Task 2** — parse `max_connections`/`max_pending_requests` + build the `connPool` struct (fields only; `connPool` lives in `internal/cluster/connpool.go`; the `circuitBreaker` struct gains `pool *connPool`; `parseCircuitBreakers` reads DEFAULT-priority budgets + builds the pool; absent budgets default to 1024 per AMEND-CP5). Tests: parse coverage in `circuitbreaker_test.go` — explicit values, absent (1024 defaults), `max_connections:0`, HIGH-priority budgets ignored by pool.
- [x] **Task 3** — the `connPool` wait-queue primitive (`acquireConnOrPend`, `releaseConn`, `removeWaiterLocked`, `clearRqPendingOpenLocked`, `hasWaiter`, stat helpers) in `connpool.go` + `connpool_test.go` (CREATE). LOAD-BEARING: acquire/pend/wake/queue-full/ctx-cancel/`max_connections:0`/`-race`-concurrency all unit-tested under `-race -count=1`.
- [x] **Task 4** — activate 3 existing emit-0 handles (`cx_open`, `rq_pending_open`, `upstream_cx_overflow`) + add 2 new CB-scoped stats (`upstream_rq_pending_active` gauge + `upstream_rq_pending_total` counter) in `registerStats` → `circuitbreaker.go`. `circuitBreakerStatNames()` grows 14→16. Arithmetic verification: 1181 + 2 = **1183** confirmed against the registration test.
- [x] **Task 5** — the permit overlay at `Cluster.Dial` + `AcquireH1` MISS + `connDec` compose (3 helpers + 2 call-site overlays in `cluster.go`). HIT path returns BEFORE `acquireConnSlot` (byte-neutral keep-alive). BYTE-STABILITY GATE: full 79-dir differential still GREEN (the `0074` CB cluster now builds a `connPool` at default 1024 + registers the 2 new zero stats — must stay byte-stable cross-side).
- [x] **Task 6** — `PutIdleH1` idle-return wake: when a waiter is queued, CLOSE the idle conn (its `connWithGauge.Close` runs `connDec` → `pool.releaseConn` → wakes head waiter) instead of pooling it. Pend/wake integration test in `connpool_test.go`. BYTE-STABILITY GATE: 79-dir differential still GREEN.
- [x] **Task 7** — the overflow → 503 routing in `router.go` (`AcquireH1` site + legacy `Dial` site) and `router_h2.go` (`DialH2` site + legacy `doH2` site). `IsConnPoolOverflow` → 503 WITHOUT `IncStatusClass` (the overflow is the signal; avoids speculative `upstream_rq_5xx` cross-side mismatch). BYTE-STABILITY GATE: 79-dir differential still GREEN.
- [x] **Task 8** — the `0078-connection-pool-max-connections` cross-side fixture (`driver.go` + `driver_test.go` + `expectations.yaml` + `README.md`) + the `/__release_sticky` additive control path in `runner_test.go` (`acceptBlockingHold`). `refContainerListenerPort = 19167`. Staged drive: N=2 held → M=2 pend → J=2 overflow 503 → sticky-release drain.
- [x] **Task 9** — `0078` deliberate breaks + 20-run flake gate (all with `-count=1` per `reference_differential_break_protocol_count1`; the `-run 'TestDifferential/0078'` selector per `reference_differential_run_selector`). Prove every subject-side assertion is LIVE. **EVIDENCE: see § Task 9 deliberate-break evidence below.**
- [x] **Task 10** — full **80**-dir differential (`go test ./test/differential/ -count=1`) + the complete six-gate (build/vet/gofmt/lint/unit/differential). ALL SIX GREEN; counts confirmed 1183 / 80 / 43 / 36. **EVIDENCE: see § Task 10 six-gate evidence below.**
- [x] **Task 11** — ADR-0252 body (the `max_connections` hard-cap + bounded wait-queue architecture; AMEND-CP1 DEPARTURE recorded IN-PLACE per ADR-0044) + BEHAVIOR_CONTRACT connection-pool subsection + stat-count roll 1181 → 1183 + DECISIONS tail ADR-0251 → ADR-0252 (next-free ADR-0253). **DONE.**
- [x] **Task 12** — completion bundle: phase README; STATE/ROADMAP/next-prompt roll; ROADMAP row 43 STAYS `in-progress` (43.1 is the FIRST of TWO legs — the by-concern split: 43.1 = budget substrate + pending wait-queue; 43.2 = the H2 multiplex pool superseding ADR-0056; the row flips `done` only when BOTH legs land — `reference_roadmap_split_phase_row_done`; the rows-36/39 + phase-42 precedent). Controller squash + push at stage-close. **DONE — see § Task 12 final six-gate evidence below.**

## Notes / recorded departures (from the SPEC §1.1 amendments)

- **AMEND-CP1 — `max_connections` hard-cap (a DEPARTURE):** envoy-go HARD-caps concurrent upstream connections at `max_connections`; the reference's `max_connections` is a soft breaker (`cx_active` can exceed it under timing slop — `reference_max_connections_soft_breaker`). Cross-side-EXACT connection-pool differentials infeasible against the reference; envoy-go uses a clean hard-cap + bounded-queue as a documented departure (ADR-0252). The differential proves robust cross-side parity on the OBSERVABLE contract (overflow counter delta ≥ 1, final cx_open == 0) + subject-exact precision.
- **AMEND-CP2 — `upstream_rq_pending_overflow` shared handle:** `cb.upstreamRqPendingOverflow` (already wired for `max_requests` overflow in phase 41) is ALSO stored into `cb.pool.upstreamRqPendingOverflow`; the same counter serves both queue types (the reference's behavior — no double-registration).
- **AMEND-CP5 — `max_connections`/`max_pending_requests` absent ⇒ 1024 default:** absent `*wrapperspb.UInt32Value` → 1024 (byte-neutral at the default; the pool is built but effectively off). Explicit 0 → that value (all-overflow config, valid).
- **AMEND-CP7 — `/__release_sticky` is NOT a new BackendKind:** additive control path on the existing `BlockingHoldResponder`; BackendKind tail stays 36. `0074` is unaffected (it still uses the re-armable `/__release`).
- **D-S431-4 — `UO` response flag (a DEPARTURE):** the overflow 503 does NOT set the `UO` access-log flag (envoy-go has no response-flags plumbing — the phase-41 CB4 precedent); the `0078` differential NEVER asserts the access-log line. Recorded in ADR-0252 §Consequences + BEHAVIOR_CONTRACT.

## Task 9 deliberate-break evidence (2026-06-22)

All runs used `-count=1` and `-run 'TestDifferential/0078'` per the break protocol.

### Break (A) — max_connections NOT hard-capped

**Edit:** Added `return nil` as the very first line of `acquireConnOrPend` body (before `p.mu.Lock()`), so all permit requests are granted immediately and `activeConns` / `cx_open` are never updated.

**Failure produced:**
```
runner_test.go:1238: subject: saturate-the-pool: subject: stats did not converge to
  map[cluster.c_cp.circuit_breakers.default.cx_open:1 cluster.c_cp.upstream_cx_active:2]
  within 10s (last seen map[cluster.c_cp.circuit_breakers.default.cx_open:0
  cluster.c_cp.upstream_cx_active:2])
  (the 2 held requests did not occupy all max_connections — is the hard-cap enforcing?
  is the backend holding?)
--- FAIL: TestDifferential/0078-connection-pool-max-connections (12.34s)
```

`cx_open` never reached 1 because `activeConns` was never incremented, so the cap-crossing logic never fired. The hard-cap assertion is LIVE.

**Restore:** `git checkout -- internal/cluster/connpool.go` → `git diff` empty.

### Break (B) — wait-queue NOT bounded (no queue-full 503)

**Edit:** Changed the overflow branch condition from `if int64(len(p.waiters)) >= p.maxPendingRequests {` to `if false {`, so oversubscribing requests always enter the wait-queue and never receive `errConnPoolOverflow` / 503.

**Failure produced:**
```
runner_test.go:1238: subject: oversub[0]: transport error: read response: unexpected EOF
  (should be a 503 local reply, not a transport failure)
runner_test.go:1238: subject: oversub[1]: transport error: dial 127.0.0.1:41617:
  dial tcp 127.0.0.1:41617: connect: connection refused
  (should be a 503 local reply, not a transport failure)
runner_test.go:1238: subject: 0 oversubscribers got 503, want exactly 2 (J)
--- FAIL: TestDifferential/0078-connection-pool-max-connections (90.38s)
```

Oversubscribing requests hung until the convergeDeadline (90s) instead of failing fast with 503. The queue-full / overflow-503 assertion is LIVE.

**Restore:** `git checkout -- internal/cluster/connpool.go` → `git diff` empty.

### Step 3 — both breaks restored; fixture + connPool -race

- Restored-fixture run: **PASS** (`TestDifferential/0078-connection-pool-max-connections`) — confirms the revert is correct.
- `go test ./internal/cluster/ -race -run 'ConnPool' -count=1` — **13/13 PASS**, no race detector findings.

### Step 4 — 20-run flake gate

`for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0078' -count=1 >/dev/null 2>&1 && echo "run $i PASS" || echo "run $i FAIL"; done`

Result: **20/20 PASS** — no flakes observed. The fixture is deterministic at this convergeDeadline.

## Task 10 six-gate evidence (2026-06-22)

All gates run from worktree root `/home/esa/git/envoy-go/.worktrees/phase-43.1-connection-pooling`. Base HEAD: `1de8a041`.

### Gate 1 — `go build ./...`

**PASS** (exit 0, empty output)

### Gate 2 — `go vet ./...`

**PASS** (exit 0, empty output)

### Gate 3 — `gofmt -l internal/ test/`

**PASS** (exit 0, empty output — zero drift)

### Gate 4 — `golangci-lint run ./...`

**PASS** (exit 0, no findings)

### Gate 5 — `go test ./internal/... -count=1`

**PASS** (exit 0, all packages `ok`; 62 packages tested, 3 `[no test files]` — zero failures)

### Gate 6 — `go test ./test/differential/ -count=1` (full 80-dir suite)

First run: **FLAKE** — `0012-http-header-mutation` + `0013-http-local-ratelimit` each failed with `subject ready: EOF` (the known startup-race under suite load; `reference_differential_fullsuite_startup_flake`). Both are UNRELATED to connection pooling.

Isolation re-runs:
- `go test ./test/differential/ -run 'TestDifferential/0012' -count=1` → **PASS** (2.189s)
- `go test ./test/differential/ -run 'TestDifferential/0013' -count=1` → **PASS** (4.582s)

Full re-run: **PASS** (exit 0, `ok github.com/esalaine/envoy-go/test/differential`, 243.201s) — **80/80 GREEN**

### Confirmed counts

| Axis | Expected | Confirmed | Note |
|------|----------|-----------|------|
| Stat surface | **1183** | **1183** | arithmetic 1181+2; validated by Task 4 registration test (PASS in Gate 5) |
| Fixtures | **80** | **80** | `ls -d test/fixtures/0*/` counts 80 dirs |
| Fuzzers | 42 (plan) | **43** | pre-existing — h2 fuzz file had 2 functions at base `e03d4729`; no fuzz files changed in phase 43.1 (`git diff e03d4729..HEAD -- '*fuzz*'` is empty); plan baseline was a documentation inaccuracy; UNCHANGED by this phase |
| BackendKind tail | **36** | **36** | `BlockingHoldResponder BackendKind = 36` in `test/differential/fixture/fixture.go:588` |

## Task 12 final six-gate evidence (2026-06-22)

All gates run from worktree root `/home/esa/git/envoy-go/.worktrees/phase-43.1-connection-pooling`. HEAD at `33402c3b` (Task 11 commit).

### Gate 1 — `go build ./...`

**PASS** (exit 0, empty output)

### Gate 2 — `go vet ./...`

**PASS** (exit 0, empty output)

### Gate 3 — `gofmt -l internal/ test/`

**PASS** (exit 0, empty output — zero drift)

### Gate 4 — `golangci-lint run ./...`

**PASS** (exit 0, no findings)

### Gate 5 — `go test ./internal/... -count=1`

**PASS** (exit 0, 59 packages `ok`; 3 `[no test files]` — zero failures)

### Gate 6 — `go test ./test/differential/ -count=1` (full 80-dir suite)

First run: **FLAKE** — one fixture failed with `subject ready: EOF` (the known transient startup-race under suite load; `reference_differential_fullsuite_startup_flake`). UNRELATED to connection pooling.

Full re-run: **PASS** (exit 0, `ok github.com/esalaine/envoy-go/test/differential`, 243.336s) — **80/80 GREEN**

## Final as-built exit-delta table

| Axis | Before | After | Note |
|------|--------|-------|------|
| Stat surface | **1181** | **1183** | +2 new CB-scoped shapes (`upstream_rq_pending_active` gauge + `upstream_rq_pending_total` counter); 3 existing shapes (`cx_open`, `rq_pending_open`, `upstream_cx_overflow`) ACTIVATED from emit-0 — +0 surface delta |
| Fixtures | **79** | **80** | `0078-connection-pool-max-connections` |
| Fuzzers | **42** | **42** | UNCHANGED (no fuzz files touched in phase 43.1; pre-existing 42-vs-43 discrepancy out of scope) |
| BackendKind tail | **36** | **36** | UNCHANGED — `0078` reuses `BlockingHoldResponder` (36) with the additive `/__release_sticky` control path |
| DECISIONS tail | **ADR-0251** | **ADR-0252** | next-free ADR-0253 |
| New Go packages | — | 0 | `connpool.go` is a new FILE in the existing `internal/cluster` package |
| New go.mod modules | — | 0 | |
| ROADMAP row 43 | in-progress | **in-progress** | leg 43.1 → done; row flips done only when 43.2 (H2 multiplex pool) also lands |
