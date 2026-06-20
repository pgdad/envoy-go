# Phase 42.1 IMPL — PROGRESS

HTTP retries (`RouteAction.retry_policy`) — a re-attempt loop WRAPPING the existing single-attempt driver, the FIRST leg of the FOURTH Upstream-robustness-family row (row 42). A route configured with `retry_policy` re-attempts a failed upstream request against an LB-re-picked host: a `retryExecutor` classifies the buffered `ActionResponse.Status` against a parsed `retry_on` bitset and, on a retriable outcome with `num_retries` + `retry_budget` slots remaining, sleeps an exponential full-jitter backoff, replays the already-buffered request body, and re-invokes `doH{1,2}ClusterAction`. This is the project's FIRST request-replay control plane + its FIRST feature that RECOVERS a single request, and it ACTIVATES the dormant phase-41 `retry_budget` slot (the emit-0 `circuit_breakers.<priority>.rq_retry_open` gauge + `upstream_rq_retry_overflow` counter flip LIVE). Byte-identical when no `retry_policy` (nil-guard). Executed subagent-driven per `docs/envoy-go/phases/42-retries-and-hedging/PLAN.md` (13 tasks). **STATUS: IMPL DONE** — ALL 13 tasks LANDED; the six-gate GREEN; the full **77-dir differential GREEN**; the 76-dir byte-stability anchor GREEN throughout; **ADR-0249** landed IN-PLACE. As-built exit: **1173 / 77 / 42 / 36 / ADR-0249** (next-free ADR-0250). 42.2 (hedging) is the pre-authorized second leg; ROADMAP row 42 STAYS `in-progress` (it flips `done` only when BOTH legs land — ADR-0106 + `reference_roadmap_split_phase_row_done`; the rows-36/39 precedent).

## IMPL base commit

`2bf3a625` (`next-prompt.txt: route the next session to the phase-42.1 (retry loop) IMPL from the landed PLAN`) — master tip; worktree `phase-42.1-impl` branched from it, HEAD at Task-1 start. This is the squash anchor at stage-close.

## Baselines captured (pre-IMPL, at worktree HEAD `2bf3a625`, 2026-06-20)

- **`go build ./...`** — PASS (clean, exit 0)
- **`go vet ./...`** — PASS (clean, exit 0)
- **`gofmt -l internal/ test/`** — PASS (empty output — no drift, exit 0)
- **`go test ./internal/...`** — PASS **modulo an environmental port conflict** (see below). All packages `ok` EXCEPT `internal/admin`'s two hardcoded-port tests (`TestHandleListeners_HTTPSmoke200Text`, `TestHandleListeners_BodyExactByteLayout`), which both fail with `bind 127.0.0.1:10000: address already in use` because an UNRELATED user Docker container (`envoy-subset`, image `envoyproxy/envoy:v1.33.0`, publishing `0.0.0.0:10000->10000`) holds the port. `internal/admin/` is byte-identical to `master` (`git diff master -- internal/admin/` empty), the test code predates phase-42 (last touched at the 26.2 squash), and the worktree has ZERO production diff vs master at Task 1 — so this is an environmental contention flake, NOT a regression. No envoy-go code is at fault. The two tests are NOT in any phase-42 path.
- **Full differential suite** (`go test ./test/differential/ -count=1`) — **76-subtest GREEN on a clean re-run** (233.2s; `ok github.com/esalaine/envoy-go/test/differential`). The FIRST run failed two UNRELATED fixtures (`0012-http-header-mutation`, `0025-http-adaptive-concurrency`) with the `subject ready: EOF` startup-race flake (`reference_differential_fullsuite_startup_flake`) — both are assertion-clean EOF startup races (not retry paths), likely aggravated by host contention from the stray containers. Resolved per protocol: isolated re-run of both (`-run 'TestDifferential/0012-http-header-mutation|TestDifferential/0025-http-adaptive-concurrency'`) → PASS (`ok`, 7.2s), then a clean FULL re-run → **76/76 GREEN, exit 0**. Confirmed environmental, NOT a regression. The differential subject/backends bind ephemeral `0.0.0.0:0` ports (the reference uses fixed 19xxx) — it does NOT collide with port 10000.
- **Stat surface** — **1163** (SPEC §14 baseline; tracked as a documented running total — no count script). The 42.1 exit total is verified ARITHMETICALLY (1163 + 10 = 1173) against the Task 5 registration test (+5 per retry-policy cluster × the `0075` fixture's TWO retry clusters [recover + exhaust]).

Worktree started GREEN (modulo the external port-10000 container) — clean baseline to land the retry loop on. Docker available; `envoyproxy/envoy:contrib-v1.37.2` present.

## Starting counts (pre-IMPL)

- stat surface: **1163** · fixtures: **76** · fuzzers: **42** · BackendKind tail: **36** (`BlockingHoldResponder`) · DECISIONS tail: **ADR-0248** (next-free **ADR-0249**)

## Anticipated exit deltas (SPEC §14)

| Axis | Before | After |
|------|--------|-------|
| Stat surface | 1163 | **1173** (+10 = +5 `upstream_rq_retry*` counters per retry-policy cluster × the `0075` two retry clusters; scoped via `EnsureRetryStats` — a recorded departure) |
| Fixtures | 76 | **77** (`0075-retry-loop`) |
| Fuzzers | 42 | **42** (unchanged — config-parse retry tokenizer unit-tested, no new fuzzer) |
| BackendKind tail | 36 | **36** (UNCHANGED — `0075` REUSES `HTTP503Responder` (35) + `HTTPEcho`) |
| DECISIONS tail | ADR-0248 | **ADR-0249** (next-free ADR-0250) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |
| Phase-41 stats flipped LIVE | emit-0 | `circuit_breakers.<priority>.rq_retry_open` (gauge) + `upstream_rq_retry_overflow` (counter) — no surface delta (already registered) |
| ROADMAP row 42 | in-progress | **stays in-progress** (the 42.2 hedging leg follows; row flips `done` only when BOTH legs land) |

## Execution-order deviation (sanctioned)

Tasks run **T1 → T2 → T3 → T5 → T6 → T4 → T7 → T8 → T9 → T10 → T11 → T12 → T13** (T5 `EnsureRetryStats` + T6 `retry_budget` activation land BEFORE T4, which calls `cl.EnsureRetryStats()` — sanctioned by the PLAN's Task-4 ordering note: landing Task 5 before Task 4's Step 3 avoids a throwaway no-op `EnsureRetryStats` stub). The controller records the actual landing order + commits per row below.

## Task checklist (13 tasks)

- [x] **Task 1** — pre-IMPL baselines + PROGRESS.md scaffold (THIS commit). Six-gate baseline captured (build/vet/gofmt/internal/differential); counts pinned 1163 / 76 / 42 / 36 / ADR-0248.
- [x] **Task 2** — `retry_on` tokenizer + `retryOnBits` bitset + the `matches` classifier (`internal/filter/http/router/retry.go` + `retry_test.go`). Enforced subset `{5xx, gateway-error, connect-failure, reset, retriable-status-codes}`; `localOrigin` distinguishes a synthesized connect/reset from a 5xx response; unknown/deferred tokens silently ignored (no PGV).
- [x] **Task 3** — the `RetryPolicy` constructor (`NewRetryPolicy`) + the exponential full-jitter backoff (`base 25ms` / `max 10×base` defaults, sub-1ms rounds up to 1ms) + the `max_interval < base_interval` reject arm.
- [x] **Task 4** — the `retry_policy` parse in hcm (`buildRouterAction` + the vhost fallback `eff := route ?? vhost`) + the `num_retries` default (1) + the backoff extract (base-required reject) + carry `retryPolicy` on `clusterRouteAction` + thread the `rp *RetryPolicy` arg through `H{1,2}ClusterAction` signatures (router.go / router_h2.go / router_weighted.go / actions.go). Parse-only (executor not yet wired). BYTE-STABILITY GATE: 76 GREEN. **Lands AFTER T5/T6** (ordering deviation above) so `cl.EnsureRetryStats()` is real, not stubbed.
- [x] **Task 5** — the +5 `upstream_rq_retry*` counters on `Cluster`, scoped via `EnsureRetryStats()` (`upstream_rq_retry`, `_retry_success`, `_retry_limit_exceeded`, `_retry_backoff_exponential`, `_retry_backoff_ratelimited`). Registration test asserts exactly the 5 names + arithmetic 1163 → 1173 (×2 clusters).
- [x] **Task 6** — the `retry_budget` activation on `circuitBreaker`: a cluster-level `activeRetries atomic.Int64` + `tryAcquireRetry`/`releaseRetry` (`activeRetries < max(min_retry_concurrency, ⌈budget_percent% × activeRequests⌉)`); on failed try-acquire ⇒ no retry + `upstream_rq_retry_overflow`++ + `rq_retry_open`=1. Defaults `budget_percent 20%` / `min_retry_concurrency 3`. Flips the 2 phase-41 emit-0 stats LIVE.
- [x] **Task 7** — the `retryExecutorH1` loop + buffered-body capture/replay + the increments (`upstream_rq_retry`++ per retry, `_retry_success` on recovery, `_retry_limit_exceeded` on static-cap exhaustion, `_retry_backoff_exponential` per retry). Over-cap streamed bodies non-retriable.
- [x] **Task 8** — the `retryExecutorH2` loop + the H2 failure-site `localOrigin` synthesis + the weighted-path threading.
- [x] **Task 9** — the `0075-retry-loop` cross-side fixture (TWO retry clusters: `c_exhaust` single `HTTP503Responder` host — cross-side-EXACT `upstream_rq_retry==numRetries`/`_retry_limit_exceeded==1`/`upstream_rq_total==numRetries+1`/final 503; `c_recover` 2-host {503, echo} RR — offset-invariant `http.<prefix>.downstream_rq_2xx delta == recoverReqs` cross-side + subject-side `_retry_success == upstream_rq_retry > 0` + `_retry_limit_exceeded == 0`). `refContainerListenerPort=19164` (next-free). Sleepless/count-based (AMEND-RT4).
- [x] **Task 10** — `0075` deliberate breaks (prove the retry assertions are LIVE, not vacuous) + 20-run flake gate (count-based ⇒ deterministic).
- [x] **Task 11** — full **77**-dir differential + the six-gate (build/vet/gofmt/lint/unit/differential) over the complete implementation. ALL SIX GREEN; counts confirmed 1173 / 77 / 42 / 36.
- [x] **Task 12** — ADR-0249 §Decision/§Consequences body (IN-PLACE per ADR-0044; the retry-loop architecture + the budget activation absorbing the BRAINSTORM's anticipated ADR-0250) + the BEHAVIOR_CONTRACT retry subsection + the stat-count roll 1163→1173 + DECISIONS tail ADR-0248→ADR-0249 (next-free ADR-0250).
- [x] **Task 13** — completion bundle (README + STATE/ROADMAP/next-prompt roll; ROADMAP row 42 STAYS in-progress pending 42.2; controller squash + push at stage-close).

## Notes / recorded departures (from the SPEC §1.1 amendments)

- **AMEND-RT1 — enforced `retry_on` subset:** `{5xx, gateway-error, connect-failure, reset, retriable-status-codes (+ explicit `retriable_status_codes[]`)}` ENFORCED; `{retriable-headers, envoy-ratelimited, grpc-*}` parse-accept-but-defer; unknown tokens silently ignored (freeform, no PGV — AMEND-RT5).
- **AMEND-RT2 — stat scoping (a departure):** the +5 `upstream_rq_retry*` counters register SCOPED to retry-policy-bearing clusters (the outlier/circuit-breaker scoping precedent — keeps every existing fixture byte-stable); the reference emits them always-on. The phase-41 `rq_retry_open` + `upstream_rq_retry_overflow` flip LIVE (no new registration).
- **AMEND-RT4 — differential design:** the EXHAUSTION arm (single 503 host) is cross-side-EXACT; the RECOVER arm asserts the offset-invariant downstream 2xx delta cross-side (the reference RANDOMIZES the RR initial offset — `reference_round_robin_offset_randomized`), with the exact `upstream_rq_retry` count subject-side-only. NO new BackendKind (reuse `HTTP503Responder` + `HTTPEcho`). Sleepless/count-based.
- **AMEND-RT5 — THIN reject surface:** only `retry_back_off.base_interval` absent/≤0 (when `retry_back_off` set) + `max_interval < base_interval` (a runtime boot-reject, mirrored as `route: %q: retry_policy: <reason>`); `retry_on` never rejected; `num_retries` unbounded (0 ⇒ no retries, valid).
- **AMEND-RT6 — backoff:** exponential full-jitter, delay-only (changes WHEN, never WHETHER/HOW-MANY ⇒ count-based differential immune); `num_retries` default 1, `base_interval` ~25ms, `max_interval` 10×base.
- **AMEND-RT7 — response flags (a departure):** envoy-go has no response-flags plumbing (the phase-41 CB4 precedent); the reference sets `RX`/`URX` on the access log; the differential asserts retry STATS + final status, NEVER the log line.
- **Byte-stability:** byte-identical when no `retry_policy` (a nil-guard) — the 76-subtest differential is the regression anchor.

## CONCERN — external port-10000 container breaks the `internal/admin` baseline

An UNRELATED user Docker container (`envoy-subset`, `envoyproxy/envoy:v1.33.0`, publishing `0.0.0.0:10000->10000`) holds `127.0.0.1:10000`, so the two hardcoded-port `internal/admin` listener tests cannot bind and fail. This is NOT an envoy-go regression (admin pkg byte-identical to master; zero production diff at Task 1). The container is the user's own work — NOT removed by this task. **Future tasks that run `go test ./internal/...` will see the same two admin failures until the container is stopped** (`docker stop envoy-subset`) or the host frees port 10000. Filter expectations accordingly: a phase-42 internal-suite gate should treat ONLY non-`internal/admin`-port-bind failures as real.

## Task 10 — deliberate breaks + flake gate

Both retry assertions in the `0075-retry-loop` cross-side fixture proven LIVE by temporarily breaking production code (each run with `-count=1` per `reference_differential_break_protocol_count1`; the full `TestDifferential/0075-retry-loop` selector per `reference_differential_run_selector`). Each break compiled cleanly (`go vet` exit 0) and FAILED on the count ASSERTION (not a compile error). Both restored via `git restore`; production tree byte-identical to `d87e30c9` (`git diff d87e30c9 -- internal/ test/` empty); on branch `phase-42.1-impl` throughout (no detached HEAD).

- **Break A — `(*RetryPolicy).matches` → always `false`** (never retry; `internal/filter/http/router/retry.go`). `--- FAIL: TestDifferential (2.45s)`. Key assertion-mismatch lines:
  - `subject: cluster.c_exhaust.upstream_rq_retry delta = 0, want 3 (final 0, base 0)`
  - `subject: cluster.c_exhaust.upstream_rq_retry_limit_exceeded delta = 0, want 1 (final 0, base 0)`
  - `subject: cluster.c_exhaust.upstream_rq_total delta = 1, want 4 (final 1, base 0)`
  - `subject: GET /recover[0] status 503, want 200 (the 503-first request must retry onto the healthy host — retry host re-pick parity)` (recover arm 503s ⇒ `downstream_rq_2xx delta = 4, want 8`)
  - `subject: cluster.c_recover.upstream_rq_retry delta == 0, want > 0`

- **Break B — `IncUpstreamRqRetry` no-op** (retry counter not wired; commented `c.retry.rq.Inc()` in `internal/cluster/cluster.go`). `--- FAIL: TestDifferential (2.84s)`. Key assertion-mismatch lines:
  - `subject: cluster.c_exhaust.upstream_rq_retry delta = 0, want 3 (final 0, base 0)`
  - `subject: cluster.c_recover.upstream_rq_retry delta == 0, want > 0 (with 8 RR requests over 2 hosts, some must pick the 503 host first and retry)`
  - `subject: cluster.c_recover.upstream_rq_retry_success delta (8) != cluster.c_recover.upstream_rq_retry delta (0) — every retry must recover onto the healthy host (retry host re-pick)`

- **Both RESTORED:** `git status --short` empty, `git rev-parse --abbrev-ref HEAD` = `phase-42.1-impl`, `git diff d87e30c9 -- internal/ test/` empty. Restored fixture PASSES (`ok ... 2.514s`).

- **20-run flake gate (`-count=1` each iteration):** 20/20 PASS, 0 FAIL. No transient `subject ready: EOF` startup race observed; no isolate-re-run needed. Counts are deterministic (backoff is delay-only / count-based per AMEND-RT4/RT6).

## Task 11 — full 77-dir six-gate

Six-gate (ADR-0052) over the COMPLETE implementation (Tasks 1–10 landed), run from worktree `phase-42.1-impl` at HEAD `aa739d02` (Task 10). **ALL SIX GREEN.**

| # | Gate | Command | Result |
|---|------|---------|--------|
| 1 | build | `go build ./...` | **PASS** (exit 0, clean) |
| 2 | vet | `go vet ./...` | **PASS** (exit 0, clean) |
| 3 | gofmt | `gofmt -l internal/ test/` | **PASS** (empty output — no drift) |
| 4 | lint | `golangci-lint run ./...` | **PASS** (exit 0, no findings) |
| 5 | unit | `go test ./internal/... -count=1` | **PASS** — every package `ok`, ZERO failures |
| 6 | differential | `go test ./test/differential/ -count=1` | **PASS** — `ok ... 232.520s`, **77/77** subtests GREEN |

- **Gate 5 (internal):** every package `ok` including `internal/admin` (`ok ... 1.496s`) — the documented environmental port-10000 caveat (`envoy-subset` container holding `127.0.0.1:10000`, breaking `TestHandleListeners_HTTPSmoke200Text` / `TestHandleListeners_BodyExactByteLayout`) did NOT trigger this run; both admin smoke tests bound successfully. NO real or environmental failure. (Caveat retained for future tasks: if the container is republished on port 10000, ONLY those two admin port-binds are environmental; any other internal failure is a regression.)
- **Gate 6 (differential):** confirmed **77 subtests** (`grep -cE '^    --- (PASS|FAIL): TestDifferential/'` = 77), first `0000-tcp-echo`, last + new `0075-retry-loop` PASS (1.92s). The suite reported `ok` overall. Two benign log-string matches were investigated and cleared: the 2 `FAIL` greps are `INFO wasm: FAIL_CLOSED 503` info lines (the literal "FAIL" inside FAIL_CLOSED, expected wasm fail-closed behavior), NOT test failures; the 2 `subject ready: EOF` are the known ephemeral-port-bind startup race (`bind: address already in use` on a 4xxxx port) — BOTH self-resolved via the runner's built-in `retrying with fresh ports` (`runner_test.go:1101`), never surfacing as a subtest failure, so NO manual isolate-re-run was required. Clean pass as-is.

**Exit counts confirmed (1173 / 77 / 42 / 36):**

- **Stat surface 1173** — 1163 baseline + 10 = +5 `upstream_rq_retry*` counters per `EnsureRetryStats` call (verified arithmetically against the Task-5 registration test asserting EXACTLY those 5 names) × the `0075` fixture's TWO retry clusters (`c_exhaust` + `c_recover`). The 2 phase-41 stats (`circuit_breakers.<priority>.rq_retry_open` gauge + `upstream_rq_retry_overflow` counter) confirmed flipped LIVE (written in `internal/cluster/circuitbreaker.go`) with NO surface delta (already registered in the 1163 baseline). No other stat-surface change.
- **Fixtures 77** — `test/fixtures/` holds 77 fixture dirs (75 numeric `00NN-*` + the two lettered `0007a-cors` / `0007b-iteration-probe`); exactly matches the 77 differential subtests. `0075-retry-loop` present (fixtures 76 → 77).
- **Fuzzers 42** — UNCHANGED. Raw `grep '^func Fuzz' internal/` = 43 on BOTH `master` AND `phase-42.1-impl` (identical) → the retry IMPL added ZERO fuzzers; the documented running total of 42 carries forward with no net delta (config-parse retry tokenizer is unit-tested, no new fuzzer).
- **BackendKind tail 36** — UNCHANGED (`BlockingHoldResponder = 36`, the highest enum value in `test/differential/fixture/fixture.go`). `0075` overrides no `BackendKind()` and reuses existing kinds (`HTTP503Responder=35` + `HTTPEcho`), adding no new enum value.

## Task 12 — ADR-0249 body + BEHAVIOR_CONTRACT

ADR-0249 §Decision/§Consequences landed IN-PLACE (per ADR-0044; §Context promoted from the SPEC §13 draft) — the retry-loop architecture (the `retryExecutor` wrapper + the `retry_on` classifier + the exponential full-jitter backoff + the parse seam + the +5 scoped counters + the `retry_budget` activation + the `RX`/`URX` response-flags departure). The single ADR-0249 ABSORBED the BRAINSTORM's anticipated ADR-0250 (the `retry_budget` dynamic-concurrency model folds into the single ADR at the firmed scope; next-free ADR-0250). The BEHAVIOR_CONTRACT gained a retries subsection + the stat-count roll "Phase 42.1 — 1163 → 1173 (+10)"; DECISIONS tail rolled ADR-0248 → ADR-0249.

## Task 13 — completion bundle (THIS task)

The completion bundle (docs-only): this PROGRESS finalized (STATUS flipped IMPL IN-PROGRESS → **IMPL DONE**; all 13 task boxes checked; the exit-delta table shows the AS-BUILT counts; this summary + the commit landing order appended); the phase README created (`docs/envoy-go/phases/42-retries-and-hedging/README.md` — framed **42.1 (the retry loop) IMPL DONE / 42.2 (hedging) PENDING**, the phase NOT fully done until both legs land); STATE advanced (active-phase → `phase 42.1 (retries-and-hedging) IMPL done`; counts 1173 / 77 / 42 / 36 / ADR-0249, next-free ADR-0250); ROADMAP row 42 **STAYS `in-progress`** (the 42.1-IMPL-landed annotation appended to column 5 + the trailing lifecycle note; the split row flips `done` only when BOTH 42.1 + 42.2 land — ADR-0106 + `reference_roadmap_split_phase_row_done`; the rows-36/39 precedent); `next-prompt.txt` rolled forward to route the next session to the **42.2 hedging BRAINSTORM** (`HedgePolicy` field 27 + the FIRST `per_try_timeout` field 3, reusing the 42.1 `retryExecutor` attempt substrate).

**Cheap-gate re-confirm (post-docs, Step 3):** Task 11 established the full 77-dir six-gate GREEN (build/vet/gofmt/lint/unit/differential) over the complete implementation; the docs-only Task-13 changes touch NO production code, so the cheap gates re-confirm clean at the Task-13 worktree state:

| Gate | Command | Result |
|------|---------|--------|
| build | `go build ./...` | **PASS** (exit 0, clean) |
| vet | `go vet ./...` | **PASS** (exit 0, clean) |
| gofmt | `gofmt -l internal/ test/` | **PASS** (empty output — no drift) |

(The full differential is NOT re-run for docs-only changes — Task 11's 77/77 GREEN is the authoritative six-gate; the stray Docker container on port 10000 is irrelevant to docs + the cheap gates.)

## All 13 tasks landed — commit landing order

The 13 task commits on branch `phase-42.1-impl` (base `2bf3a625`), in landing order (the sanctioned T1→T2→T3→T5→T6→T4→T7→T8→T9→T10→T11→T12→T13 deviation — T5 `EnsureRetryStats` + T6 `retry_budget` land BEFORE T4's `cl.EnsureRetryStats()` call, per the PLAN's Task-4 ordering note):

| # | SHA | Commit |
|---|-----|--------|
| 1 | `5828a013` | Task 1: PROGRESS scaffold + pre-IMPL baselines |
| 2 | `7ab22c80` | Task 2: `retry_on` tokenizer + `matches` classifier |
| 3 | `0f2f26a9` | Task 3: `RetryPolicy` ctor + exponential full-jitter backoff + max<base reject |
| 5 | `b47aca85` | Task 5: +5 retry counters on `Cluster` via scoped `EnsureRetryStats` |
| 6 | `9f678ed8` | Task 6: `retry_budget` activation (`activeRetries` + `tryAcquireRetry`/`releaseRetry`; `rq_retry_open` + `upstream_rq_retry_overflow` LIVE) |
| 4 | `02d09adc` | Task 4: `retry_policy` parse + vhost fallback + `clusterRouteAction` carry + `H{1,2}ClusterAction` threading |
| 7 | `216725fc` | Task 7: `retryExecutorH1` loop + body replay + retry/success/limit/backoff increments |
| 8 | `a70561a9` | Task 8: `retryExecutorH2` + H2 `localOrigin` sites + weighted-cluster retry threading |
| 9 | `d87e30c9` | Task 9: `0075` cross-side retry-loop fixture (exhaustion-exact + recover-invariant) |
| 10 | `aa739d02` | Task 10: deliberate breaks (`matches`→false, retry-counter no-op) + 20-run flake gate |
| 11 | `a139ce72` | Task 11: full 77-dir six-gate GREEN (stat 1173 / fixtures 77 / fuzzers 42) |
| 12 | `c09752cc` | Task 12: ADR-0249 body + BEHAVIOR_CONTRACT retries subsection (stat 1163→1173) |
| 13 | _(this commit)_ | Task 13: completion bundle — retries (retry loop) landed; row 42 stays in-progress for 42.2 |

The controller squashes these 13 task commits at stage-close + pushes to origin/master (NOT this task — Task 13 commits locally only).

## Exit-delta table (AS-BUILT)

| Axis | Before | After | Note |
|------|--------|-------|------|
| Stat surface | 1163 | **1173** | +10 = +5 `upstream_rq_retry*` per retry cluster × the `0075` two retry clusters (scoped via `EnsureRetryStats`) |
| Fixtures | 76 | **77** | `0075-retry-loop` |
| Fuzzers | 42 | **42** | UNCHANGED (config-parse retry tokenizer unit-tested, no new fuzzer) |
| BackendKind tail | 36 | **36** | UNCHANGED (`0075` REUSES `HTTP503Responder` 35 + `HTTPEcho`) |
| DECISIONS tail | ADR-0248 | **ADR-0249** | next-free ADR-0250 (the single ADR absorbed the anticipated ADR-0250) |
| New Go packages | — | **0** | |
| New go.mod modules | — | **0** | |
| Phase-41 stats flipped LIVE | emit-0 | **LIVE** | `circuit_breakers.<priority>.rq_retry_open` (gauge) + `upstream_rq_retry_overflow` (counter) — no surface delta (already registered) |
| ROADMAP row 42 | in-progress | **in-progress** | STAYS in-progress (42.2 hedging is the remaining leg; flips `done` only when BOTH legs land) |
