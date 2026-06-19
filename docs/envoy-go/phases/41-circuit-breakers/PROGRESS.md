# Phase 41 IMPL — PROGRESS

Circuit breakers (`max_requests` keystone) — per-priority fail-fast overflow budgets, the THIRD Upstream-robustness-family row. A cluster with `circuit_breakers` fails a request fast with a `503` (over-budget) + increments `upstream_rq_pending_overflow` while `circuit_breakers.default.rq_open` reads 1, with the full per-priority `circuit_breakers.*` stat surface (+14) registered for parity. Executed subagent-driven per `docs/envoy-go/phases/41-circuit-breakers/PLAN.md` (12 tasks). **STATUS: IMPL DONE (2026-06-19)** — six-gate GREEN; full **76-dir differential GREEN**; ADR-0248 IN-PLACE; the `0074` cross-side fixture GREEN both sides + 2 deliberate breaks + 20/20 flake-free; the 75-dir byte-stability gate held GREEN through every code task. As-built exactly the anticipated exit: **1163 / 76 / 42 / 36 / ADR-0248** (next-free ADR-0249). ROADMAP row 41 flipped `in-progress → done`.

## IMPL base commit

`53a86e15` (`phase 41 (circuit-breakers) PLAN: the 12-task TDD spine for the max_requests keystone`)

## Baselines captured (pre-IMPL, at worktree HEAD `53a86e15`)

- **`go build ./...`** — PASS (clean, exit 0)
- **`go vet ./...`** — PASS (clean, exit 0)
- **`gofmt -l internal/ test/`** — PASS (empty output — no drift, exit 0)
- **`go test ./internal/... 2>&1 | tail -20`** — PASS (all packages `ok`; no FAIL/panic)
- **Full differential suite** (`go test ./test/differential/ -count=1`) — **75-subtest GREEN** (239.9s; `ok github.com/esalaine/envoy-go/test/differential`). No `subject ready: EOF` flake observed; single clean pass.
- **Stat surface** — **1149** (SPEC §14 baseline; tracked as a documented running total — no count script). The 41 exit total is verified ARITHMETICALLY (1149 + 14 = 1163) against the Task 4 registration test.

Worktree started GREEN — clean baseline to land circuit-breaker code on.

## Starting counts (pre-IMPL)

- stat surface: **1149** · fixtures: **75** · fuzzers: **42** · BackendKind tail: **35** (`HTTP503Responder`) · DECISIONS tail: **ADR-0247** (next-free **ADR-0248**)

## Anticipated exit deltas (SPEC §14)

| Axis | Before | After |
|------|--------|-------|
| Stat surface | 1149 | **1163** (+14 `circuit_breakers.*` per-priority stats) |
| Fixtures | 75 | **76** (`0074-circuit-breaker-max-requests`) |
| Fuzzers | 42 | **42** (unchanged) |
| BackendKind tail | 35 | **36** (`BlockingHoldResponder`) |
| DECISIONS tail | ADR-0247 | **ADR-0248** (next-free ADR-0249) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |
| ROADMAP row 41 | in-progress | **done** (flat un-split family row; NO parent rollup) |

## Task checklist

NOTE on numbering: the PLAN's 12 logical tasks were executed across subagent dispatches whose internal numbering drifted (Task 4's subagent pulled the `buildCluster` wiring forward from PLAN-Task-5; the router-admission H1+H2 landed as one unit; the deliberate-breaks/flake + the ADR/contract were split). All 12 PLAN tasks are DONE — the mapping is recorded per-row below. Commits (local, on `phase-41-impl`): T1 `26a4b397` · T2 `48e0f0bb` · T3 `ef0b9878` · T4 `b6ff7e2d` · T5 `467e49e9` · T6(admission) `bc32c04e` · T7(backend) `e22fd0f5` · T8(0074) `11d40133` · T9(breaks/flake) `204bb292` · T10(six-gate) `bc872b13` · T11(ADR/contract) `daf14775` · T12(completion) = this commit.

- [x] **Task 1** — pre-IMPL baselines + PROGRESS.md scaffold (`26a4b397`).
- [x] **Task 2** — `parseCircuitBreakers` + the 3 reject arms (priority-range / retry_budget-percent / strict duplicate-priority) + `per_host_thresholds` silent-ignore + the `circuitBreaker`/`cbPriority` structs + 8 unit tests (`48e0f0bb`).
- [x] **Task 3** — the non-blocking `tryAcquire`/`release` CAS + the `rq_open` gauge + the `upstream_rq_pending_overflow` counter (incl. `max_requests:0` reject-all + a `-race` concurrency peak test) (`ef0b9878`).
- [x] **Task 5 (PLAN)** — `Cluster.TryAcquireRequest`/`ReleaseRequest` + the `buildCluster` parse+attach (verified) + **the 75-dir byte-stability gate 75/75 GREEN** (`467e49e9`); the wiring + scoped stat block were pulled forward into Task 4 (`b6ff7e2d`).
- [x] Task 4 — +14 `circuit_breakers.*` stat registrations (1149 → **1163**); `registerStats` on `circuitBreaker` (2 LIVE handles: `default.rq_open` + `upstream_rq_pending_overflow`; 12 emit-0 bare-call registrations) + scoped `if c.circuitBreaker != nil` block in `registerClusterMetrics` + `circuitBreaker` field on `Cluster` + `parseCircuitBreakers` wired into `buildCluster` (pulled forward from Task 6 so the present/absent registration tests can drive through `NewManager`); tests `TestRegisterCircuitBreakerStats_{Present,Absent}` assert exactly the 14 names via `reg.Walk`/`hasMetric`; package green, gofmt/vet/golangci-lint clean. NOTE: Task 6's `buildCluster` wiring + scoped block now land here — Task 6 reduces to verification.
- [x] Task 5 — exported router-facing seam: `Cluster.TryAcquireRequest()` (DEFAULT-priority `tryAcquire(0)`; nil-guard returns true) + `Cluster.ReleaseRequest()` (nil-guard no-op) in `cluster.go`; tests `TestClusterTryAcquireReleaseRequest` (budget 1 → true/false/release/true) + `TestClusterTryAcquireRequestNoCircuitBreaker` (nil-guard always-true + no-panic release). VERIFIED Task-4's `buildCluster` wiring: `parseCircuitBreakers(c, name)` called, error propagated (`return nil, err`), result assigned to `cl.circuitBreaker` — correct, no fix needed (Task 6 wiring already satisfied). **BYTE-STABILITY GATE: full differential `go test ./test/differential/ -count=1` → 75/75 GREEN (231s, no flake, no regression)** — proves the additions are inert for all non-cb clusters (nil-guard ⇒ no admission change + scoped stat registration). gofmt/vet/golangci-lint clean.
- [x] Task 6 — wire `parseCircuitBreakers` into `buildCluster` + scoped `registerStats` in `registerClusterMetrics` (ABSORBED into Task 4 `b6ff7e2d` + verified in Task 5 `467e49e9` — the error is propagated, the result assigned to `cl.circuitBreaker`).
- [x] Task 7 — H1 router admission `TryAcquireRequest` + `defer ReleaseRequest` + overflow 503 (landed together with Task 8 as the IMPL "Task 6" router-admission unit). `doH1ClusterAction` (`router.go`): inserted immediately AFTER `a.cluster.IncUpstreamRqTotal()` and BEFORE the `applyHashKey` block — `if !a.cluster.TryAcquireRequest() { return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}, picked, nil }` THEN `defer a.cluster.ReleaseRequest()`. **Critical ordering verified:** the overflow early-return precedes the defer, so an overflow request (which never acquired a slot) does NOT release one. NO extra stat call on the overflow path (the LIVE `upstream_rq_pending_overflow` set inside `TryAcquireRequest` is the only signal — avoids a speculative `upstream_rq_5xx` cross-side mismatch the 0074 fixture does not assert). Cluster-backed unit test `TestDoH1ClusterAction_CircuitBreakerOverflow503` (new `singleEndpointClusterCB` + `blockingHTTPBackend` harness, `max_requests:1`): held request acquires the only slot (`default.rq_open`→1) → concurrent admission returns Status 503 + zero `picked` + `upstream_rq_pending_overflow`==1 → release → `rq_open` clears → 3rd admission returns 200, overflow stays at 1 (no double-release, no leak). Red-before-green confirmed (pre-impl: `rq_open` never flips).
- [x] Task 8 — H2 router admission `TryAcquireRequest` + `defer ReleaseRequest` + overflow 503. `doH2ClusterAction` (`router_h2.go`): inserted AFTER `a.cluster.IncUpstreamRqTotal()` — `if !a.cluster.TryAcquireRequest() { return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders(), Body: nil}, picked, nil }` THEN `defer a.cluster.ReleaseRequest()`. Same early-return-before-defer ordering. The H2 integrated admission is covered end-to-end by the 0074 cross-side fixture (Task 10) + the H1 unit test exercises the shared `Cluster.TryAcquireRequest`/`ReleaseRequest` seam. **Full internal suite `go test ./internal/... -count=1` GREEN; byte-stability gate `go test ./test/differential/ -count=1` → GREEN (233s, no flake) — nil-guard ⇒ no fixture configures circuit_breakers, admission path byte-identical.** gofmt/vet/golangci-lint clean.
- [x] Task 9 — `BlockingHoldResponder` BackendKind 36 + `acceptBlockingHold` runner arm. New `fixture.BlockingHoldResponder BackendKind = 36` (BackendKind tail **35 → 36**; doc-comment in the HTTP503Responder=35 style). `acceptBlockingHold(ln, idx)` in `runner_test.go`: re-armable gate — accepts one HTTP/1.1 request per conn; a normal `GET /<seg>` blocks on a shared `gate` channel until a `GET /__release` control request closes-and-swaps the gate (frees the current batch + re-arms for the next), then answers 200 `backend-<idx>:<seg>`; `/__release` answers 200 `released`. Spawn `switch` arm `case fixture.BlockingHoldResponder:` beside the `HTTP503Responder` arm — `net.Listen("tcp","0.0.0.0:0")` + `go acceptBlockingHold(ln, bo.idx)` (uniform `BackendKind()` dispatch; no `PerHostBackendKind` needed for 0074). Imports: all present except bare `sync` (only `sync/atomic` was imported) — added `"sync"` for `sync.Mutex`. Purely additive (no existing fixture uses kind 36): `go build ./...` + `go vet ./test/...` clean; **full differential `go test ./test/differential/ -count=1` → 75/75 GREEN (231s, no flake)**; gofmt clean; golangci-lint clean. Sanity test skipped — 0074 (Task 10) exercises the gate end-to-end.
- [x] Task 10 — `0074` cross-side circuit-breaker-max-requests fixture. New `test/fixtures/0074-circuit-breaker-max-requests/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}` + blank-import in `runner_test.go` (after the 0073 line). Cross-side `[http_connection_manager + router]` over ONE cluster `c_cb` (`lb_policy ROUND_ROBIN`), ONE `BlockingHoldResponder` (BackendKind 36) endpoint, `circuit_breakers: { thresholds: [{ priority: DEFAULT, max_requests: 4 }] }` on BOTH sides (reference `STRICT_DNS`/`host.docker.internal`, subject `STATIC`/`127.0.0.1`). `StatsAsserter` (cross-side, NOT SubjectAsserter), `BackendKind()` uniform = `BlockingHoldResponder` (NO `PerHostBackendKind`), `BackendCount()==1`. ALL constants single-sourced (`fixtureName`/`clusterName=c_cb`/`refContainerListenerPort=19163` [next-free; 0073 family took up to 19162]/`refAdminPort=9901`/`maxRequests=4`/`backendCount=1`/`convergeDeadline=10s`/`convergePoll=50ms`). **AssertStats runs SEQUENTIALLY per side** (subject FULLY incl. release + `rq_open→0` drain, THEN reference — shared in-process backend clean between sides): fire `maxRequests` CONCURRENT `GET /` (held at the responder) via `sync.WaitGroup` → poll `rq_open→1` (all slots filled; NO fixed sleep) → baseline `upstream_rq_pending_overflow` + fire `(N+1)`th `GET /` → assert **503** → re-scrape `rq_open==1` + `(overflow-baseline)>=1` + `upstream_rq_total>0` (decode-ran guard) → `GET /__release` on `127.0.0.1:<backendPort>` (BACKEND control port, loopback) → `wg.Wait()` all N held → **200** `backend-0:` → poll `rq_open→0`. **Unit test** (`backendIdxFromBody` table + `TestConstants`) PASS. **Cross-side `go test ./test/differential/ -run 'TestDifferential/0074' -count=1` → PASS** (both sides: over-budget=503, rq_open 1→0, overflow-delta=1; rq_total subject 5 / reference 4 — both >0). **Liveness proven:** a deliberate-break (`tryAcquire` always-true) FAILED at the subject `rq_open→1` poll (never converges); a temp per-side probe confirmed BOTH sides genuinely run + assert (not vacuous). `rq_open` emits on the reference WITHOUT `track_remaining` (only `remaining_*` needs it) — no quirk. UO access-log flag NOT asserted (D-S41-3); `upstream_rq_5xx` NOT asserted (overflow 503 is a local reply). gofmt/vet/golangci-lint clean.
- [x] Task 11 — `0074` deliberate breaks + 20-run flake (`204bb292`) + ADR-0248 body + BEHAVIOR_CONTRACT load-shedding subsection + stat-count 1149→1163 (`daf14775` — ADR-0248 §Decision/§Consequences IN-PLACE per ADR-0044; the UO-flag departure recorded; DECISIONS tail ADR-0247→ADR-0248, next-free ADR-0249)
  - **VERIFICATION (deliberate-break liveness + flake, 2026-06-19; verification-only, no production change):** Break (A) — `tryAcquire` `return true` as first stmt (breaker never enforces) → FAIL: `subject: fill-the-budget: subject: cluster.c_cb.circuit_breakers.default.rq_open did not converge to 1 within 10s (last seen 0)` (the 4 held requests never fill the budget). Break (B) — overflow branch keeps `return false` but skips `upstreamRqPendingOverflow.Inc()` + `rqOpen.Set(1)` → FAIL: `subject: cluster.c_cb.upstream_rq_pending_overflow delta = 0, want >= 1` (the rq_open poll still converged via the acquire-path `Set(1)`, then the overflow-delta assertion tripped — proves the overflow-counter assertion is LIVE, not vacuous). Both breaks reverted; `git diff` clean; restore-confirm `TestDifferential/0074 -count=1` PASS. **20-run flake gate: 20/20 PASS** (sleepless `/__release` barrier + poll-to-converge ⇒ deterministic; `-count=1` every run). ADR-0248 body + BEHAVIOR_CONTRACT (the remainder of Task 11) NOT done in this verification pass.
- [x] Task 12 — full 76-dir differential + six-gate (`bc872b13`, GREEN — see the table below) + completion bundle (README + STATE/ROADMAP/next-prompt roll + this PROGRESS finalization; ROADMAP row 41 → `done`; controller squash + push at stage-close).

## Notes / recorded departures (from PLAN D-questions)

- **D-S41-3 — `UO` response flag DEFERRED (recorded departure):** envoy-go has no access-log response-flag plumbing; the `0074` differential asserts the 503-status + `upstream_rq_pending_overflow`/`rq_open` stats pair, NEVER the access-log line. Record in ADR-0248 §Consequences + BEHAVIOR_CONTRACT (Task 11).
- **D-S41-1 — enforcement scope (AMEND-CB1):** only `max_requests` is enforced; `max_connections`/`max_pending_requests` register-for-parity-but-defer.
- **Byte-stability:** byte-identical when no `circuit_breakers` (a nil-guard) — the 75-subtest differential is the regression anchor.

## Task 10 — full 76-dir differential + six-gate (ADR-0052), 2026-06-19 (verification-only, no production change)

Comprehensive six-gate over the complete phase-41 implementation (all production code + the `0074` fixture landed). All gates GREEN.

| # | Gate | Command | Result |
|---|------|---------|--------|
| 1 | build | `go build ./...` | **PASS** (clean, exit 0) |
| 2 | vet | `go vet ./...` | **PASS** (clean, exit 0) |
| 3 | gofmt | `gofmt -l internal/ test/` | **PASS** (empty output — no drift) |
| 4 | lint | `golangci-lint run ./...` | **PASS** (exit 0, no findings) |
| 5 | unit | `go test ./internal/... -count=1` | **PASS** (59 packages `ok`; no FAIL/panic) |
| 6 | differential | `go test ./test/differential/ -count=1` | **PASS — 76/76 GREEN** (231s) |

**Differential flake note (`reference_differential_fullsuite_startup_flake`):** the FIRST full-suite run failed a single UNRELATED dir `0012-http-header-mutation` with the `subject ready: EOF` startup race (3 attempts, all EOF — not an assertion mismatch; not a phase-41-touched path). Resolved per protocol: isolated re-run of `0012` → PASS (`ok`, exit 0), then a clean FULL-suite re-run → **76/76 GREEN, exit 0**. Confirmed environmental startup flake, NOT a regression. `0074-circuit-breaker-max-requests` PASS (verified in isolation + in the full suite).

**Verified counts:**
- **stat surface = 1163** (1149 + 14; Task 4 registration test re-run `go test ./internal/cluster/ -run CircuitBreaker -count=1 -v` → PASS; `TestRegisterCircuitBreakerStats_Present` asserts EXACTLY the 14 `circuit_breakers.*` names — 10 per-priority `*_open` gauges [default+high × {cx_open, cx_pool_open, rq_open, rq_pending_open, rq_retry_open}] + 4 cluster overflow counters).
- **differential fixtures = 76** (`ls -d test/fixtures/[0-9]* | wc -l`).
- **fuzzers = 42** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` — unchanged).
- **BackendKind tail = 36** (`BlockingHoldResponder`).

Six-gate GREEN over the complete implementation — **1163 / 76 / 42 / 36**.
