# Phase 35 — `random` load balancer — IMPL progress ledger

Worktree: `/home/esa/git/envoy-go/.worktrees/phase-35-impl` (branch `phase-35-impl`).
IMPL session of the 8-task TDD plan (`PLAN.md`). Commits are LOCAL-ONLY; the
controller pushes at stage-close.

## Task table

| # | Task | Status |
|---|------|--------|
| 1 | Baselines gate + PROGRESS.md | done |
| 2 | `randomLB` stateless type (`random.go`) | done |
| 3 | Manager acceptance + reject text + doubly-hit retarget | done |
| 4 | Anti-skew integration test + RANDOM boot smoke | done |
| 5 | `0060-lb-random` differential fixture | done |
| 6 | Band tuning + 3 deliberate-break liveness proofs | done |
| 7 | Full 62-dir differential re-verify + race + conformance | done |
| 8 | Completion bundle (BEHAVIOR_CONTRACT + ADR-0234 + STATE/ROADMAP) | done |

**ALL 8 TASKS COMPLETE.** The phase-35 {random} IMPL is done; the controller
squash-merges + pushes at stage-close (`feedback_subagents_no_push` /
`feedback_push_to_origin`). (Tasks 2–5 were executed in the IMPL session ahead of
the Task-6 band-tune + Task-7 six-gate records below; this table flips them to
`done` for the final ledger.)

## Step 1 — count anchors (re-confirmed against the IMPL-session tip)

Re-confirmed BEFORE touching any code (the established first-task discipline). Each
row records the EXACT recipe used + its output + whether it matched the plan's
expected value.

| Anchor | Recipe (run from the worktree root) | Output | Expected | Match |
|--------|-------------------------------------|--------|----------|-------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `61` | 61 | YES |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0059-lb-least-request` | `0059-lb-least-request` | YES |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 | YES |
| stat surface | BEHAVIOR_CONTRACT doc count — `grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md` (line 466: "Phase 33 extension — 1091 → 1116 internal names") | `1116` | 1116 | YES |
| BackendKind tail | `grep -n "BackendKind = " test/differential/fixture/fixture.go \| tail -1` | `TCPThriftResponder BackendKind = 33` (line 562) | 33 | YES |
| DECISIONS tail | `grep -n "^## ADR-0" docs/envoy-go/DECISIONS.md \| tail -1` | `ADR-0233` (line 14972; next-free ADR-0234) | ADR-0233 | YES |
| ADR count | `grep -c "^## ADR-0" docs/envoy-go/DECISIONS.md` | `232` | informational | — |
| `go build ./...` | `go build ./... && echo BUILD_OK` | `BUILD_OK` | green | YES |

### Recipe notes (canonical-recipe discovery)

- **fuzzers (42):** the canonical recipe from phase-34 PROGRESS.md —
  `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`.
  Counts ONLY `fuzz_test.go` files (excludes seed/corpus helper funcs; does NOT
  over-count). No new fuzzer in phase 35 (RANDOM: no wire decode — the SECOND
  no-fuzzer phase after phase 34). Tail stays at the 42nd `FuzzThriftDecode`.
- **stat surface (1116):** there is NO programmatic test asserting the number. The
  canonical source-of-truth is the BEHAVIOR_CONTRACT doc count at line 466
  (`grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md` → line 466: "Phase 33
  extension — 1091 → 1116 internal names"). Phase 35 is the SECOND zero-delta phase
  (stat surface UNCHANGED at 1116).
- **DECISIONS tail (ADR-0233):** `grep -c "^## ADR-0"` yields 232 headings
  (ADR-0002..ADR-0233; ADR-0000/0001 omitted from the running count). The ADR-0234
  §Context is a DRAFT in SPEC §13 — NOT yet in DECISIONS.md at this IMPL tip.

## Step 2 — as-built line anchors (re-confirmed against IMPL-session tip)

All grepped from the worktree root. ACTUAL line numbers recorded.

| Symbol | File | Line | Notes |
|--------|------|------|-------|
| `func (rr *roundRobin) Pick` | `internal/cluster/loadbalancer.go` | 34 | zero-churn posture `randomLB` copies |
| `noopRelease = func` | `internal/cluster/loadbalancer.go` | 21 | shared release `randomLB` returns |
| `func newPCGRNG` | `internal/cluster/leastrequest.go` | 63 | REUSED verbatim by `newRandom` |
| `func newLeastRequestWithRNG` | `internal/cluster/leastrequest.go` | 49 | `newRandomWithRNG` mirrors it minus choiceCount |
| `switch c.GetLbPolicy()` | `internal/cluster/manager.go` | 234 | the case to extend with `Cluster_RANDOM` |
| `unsupported lb_policy` | `internal/cluster/manager.go` | 252 | the reject text (ONE production string) |
| `func TestManager_Error_UnsupportedLBPolicy` | `internal/cluster/manager_test.go` | 320 | the doubly-hit retarget |
| `func acceptEchoCounting` | `test/differential/runner_test.go` | 1330 | streaming echo backend (`0060` reuses) |

### seqRNG / eps pre-check (load-bearing for Task 2)

```
grep -n "func seqRNG\|func eps" internal/cluster/*_test.go
```
Result:
```
internal/cluster/leastrequest_test.go:10:func seqRNG(vals ...uint64) func() uint64 {
internal/cluster/leastrequest_test.go:19:func eps(n int) []Endpoint {
```

**Both `seqRNG` and `eps` already exist in `internal/cluster/leastrequest_test.go`
(lines 10 and 19).** Task 2's `random_test.go` MUST REUSE them (same package
`package cluster`), NOT redeclare them (would be a compile error).

## Step 3 — reject-text blast radius (AMEND-R2 / D-L5 discipline)

Expected: production string ONLY in `manager.go`; ZERO fixture hits; ONE doc hit in
`BEHAVIOR_CONTRACT.md`.

| Site | File | Line | Notes |
|------|------|------|-------|
| production string | `internal/cluster/manager.go` | 252 | `"… unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST)"` — the ONE production string |
| test assertion | `internal/cluster/manager_test.go` | 327 | `strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST")` — unit-level only |
| doc pin | `docs/envoy-go/BEHAVIOR_CONTRACT.md` | 899 | `"… (supported: ROUND_ROBIN, LEAST_REQUEST)"` departure record |

**ZERO fixture hits** (`grep -rln "ROUND_ROBIN, LEAST_REQUEST" test/` → empty).
Confirms NO boot-reject fixture pins the text. Blast radius at Task 3:
- `internal/cluster/manager.go` line 252: change to `(supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)`
- `internal/cluster/manager_test.go` line 327: extend substring check + retarget trigger from `Cluster_RANDOM` → `Cluster_RING_HASH`
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` line 899: update the departure record (Task 8)

## D-S35 resolutions

| Decision | Resolution |
|----------|-----------|
| D-S35-1: file placement | `random.go` NEW sibling under `internal/cluster/`; manager `case clusterv3.Cluster_RANDOM` + reject text update in `manager.go`; `TestManager_Error_UnsupportedLBPolicy` retarget in `manager_test.go` — the `leastrequest.go` precedent |
| D-S35-2: newRandom seeding | `newRandom` calls `newPCGRNG()` DIRECTLY, accepts the shared seed-error message path (`"cluster: least_request: seed rng"` — cosmetic note only, behavior-neutral), NO wrapper |
| D-S35-3: band parameters | PLAN-proposed `{floor 12, ceiling 32}` was **re-tuned at Task 6 to `{floor 6, ceiling 40}`** (the `{12,32}` band flaked 2/20 — see Task 6 below). Final: `{K=4 held, S=60 burst, sum==64, floor 6, ceiling 40}` + tuning protocol (40/40 flake-free over two `-count=20` batches + 3 `-count=1` breaks) |
| D-S35-4: anti-skew test shape | Standalone deterministic-RNG anti-skew unit test: RANDOM does NOT avoid a held endpoint (the contrapositive of least_request) |

## ADR-0045 split-gate FINAL re-check

**Verdict: NO SPLIT.** 8 tasks / ~45–60 prod LoC (1 new file `random.go` ~25 LoC +
~10 LoC across `manager.go` + no new packages, no new BackendKind, no new fuzzers,
no new stat names). Well within the single flat-row envelope.

## Anticipated end-of-IMPL count moves

| Counter | Before | After | Delta |
|---------|--------|-------|-------|
| fixtures | 61 | 62 | +1 (`0060-lb-random`) |
| DECISIONS tail | ADR-0233 | ADR-0234 | +1 |
| stat surface | 1116 | 1116 | 0 (SECOND zero-delta phase) |
| fuzzers | 42 | 42 | 0 (SECOND no-fuzzer phase) |
| BackendKind tail | 33 | 33 | 0 (`TCPEcho` reused; no new BackendKind) |

## Task 6 — band tuning + 3 deliberate-break liveness proofs

Cites the **0030 dead-assertion lesson, generalized to bands**: a band wide enough
that it can NEVER fail is a dead assertion. The dual hazard for a *random*
distribution band is the OPPOSITE — a band tight enough to bite the breaks may
flake on the natural multinomial tail. Task 6 resolved both directions.

### Step 1 — flake check + band re-tune (the PLAN `{12,32}` was flaky)

The PLAN-proposed `{floor 12, ceiling 32}` band is **NOT flake-free**. The runner
draws 6 independent `binomial(64, 1/3)` samples per run (3 backends × 2 sides); a
`-count=20` batch is 120 samples. Per-sample tail probs: `P(X < 12) ≈ 0.0032`,
`P(X > 32) ≈ 0.0020` → ~30% / ~20% chance of ≥1 flake per batch. **Empirically
observed a `-count=20` batch FAIL 2/20** (`subj=[25 10 29]` floor 10<12; `ref=[9
34 21]` floor 9<12). The PLAN itself authorized widening within the pinned
PRINCIPLE {conservation + uniform-floor + uniform-ceiling}, so the band was
**re-tuned to `{floor 6, ceiling 40}`** (per-sample `P(X<6) ≈ 2e-6`, `P(X>40) ≈
3e-7` → `<0.03%` over 120 samples). Re-ran **two `-count=20` batches = 40 runs:
40/40 PASS, zero band violations.** Observed per-backend extremes over the 40
runs: min `14`, max `31` — comfortably inside `[6, 40]`. (Per-side: ref 14–31,
subj 14–31.) The wide band still bites all three breaks (Steps 2–4).

Final constants: `K=4`, `S=60`, `sum==64`, `uniformFloor=6`, `uniformCeiling=40`.

### Steps 2–4 — three deliberate breaks (`-count=1`, go-test caching defeated)

Each break: applied → `-count=1` run → confirmed the EXPECTED leg FAILS → `git
restore` → confirmed `git diff <file>` empty. NEVER committed a broken state.

| # | break | expected leg | observed FAIL output | revert clean |
|---|-------|--------------|----------------------|--------------|
| i | `manager.go` `case Cluster_RANDOM`: `newRandom(endpoints)` → `newLeastRequest(endpoints, 10)` (the canonical anti-skew break — least_request consults the held-conn counters) | uniform floor | `subject: uniform floor: backend[1]=2 < 6 (load-skewing policy? single-host pin?)` | diff empty |
| ii | `random.go` `Pick`: `i := int(r.rng() % uint64(n))` → `i := 0` (single-host pin) | uniform ceiling (+ floor on un-picked) | `subject: uniform ceiling: backend[0]=64 > 40 (single-host pin?)` | diff empty |
| iii | `driver.go` `AssertStats`: `upstream_cx_total` want `totalConns`(64) → `99` | stats prong | `ref … upstream_cx_total = 64, want 99` + `subj … = 64, want 99` | diff to HEAD = band tuning only |

Break (i) is the symmetric inverse of 0059's starvation break and proves the
**floor** leg live; break (ii) proves the **ceiling** leg live (it returns on the
first per-element violation — the pinned backend's `64 > 40` — before the loop
reaches the `0 < 6` floor violations on the un-picked backends; the floor-on-pin
path is covered by break (i) and the `{62,1,1}`/`{2,26,36}` unit rows); break
(iii) proves the **stats** prong non-vacuous (both sides genuinely observed 64).

### Step 5 — post-revert state + code-review nit

After all three reverts, production code (`manager.go`, `random.go`) is
**byte-identical to HEAD** (`git diff` empty on both). The only working-tree
changes are docs/comments + the band-constant tuning in the fixture driver:
- `test/fixtures/0060-lb-random/driver/driver.go` — band constants `{12,32}` →
  `{6,40}` + the surrounding comment/σ-margin updates (fixture, not production).
- `test/fixtures/0060-lb-random/driver/driver_test.go` — comment-only updates to
  the new bounds + the code-review nit fix on
  `TestAssertDistribution_CeilingBitesOnPin` ("backend[0]=62 passes the floor but
  trips the ceiling" — the floor is checked before the ceiling within each loop
  element, so the prior "ceiling is checked first" comment was inaccurate).
- `README.md` + this `PROGRESS.md` — the break record + final constants.

All 5 `driver_test.go` unit tests PASS under `{6,40}`; gofmt clean.

## Task 7 — full six-gate verification (NO code changes; evidence only)

Branch `phase-35-impl`, tree clean at start (HEAD `cd6d480` — Task 6). All gates
run from the worktree `/home/esa/git/envoy-go/.worktrees/phase-35-impl`. This is
the six-gate REAL guard: prove the new manager `case Cluster_RANDOM` kept all 61
prior fixtures byte-exact (the seam is REUSED unchanged → behavior-neutrality is
structural) and `0060` is green, with no race, clean build/vet/gofmt/lint/tidy,
and conformance unaffected.

### Gate 1 — full differential suite (62 dirs), byte-exact

```
go test ./test/differential/ -count=1
ok  github.com/esalaine/envoy-go/test/differential  214.636s   (exit 0)
```

Re-run with `-v` to count per-fixture subtests:
```
go test ./test/differential/ -count=1 -v
ok  github.com/esalaine/envoy-go/test/differential  205.406s   (exit 0)
```
- **62** `--- PASS: TestDifferential/<fixture>` lines (`grep -c` = 62; `sort -u`
  = 62 unique fixtures, `0000-tcp-echo` … `0060-lb-random`).
- **`--- PASS: TestDifferential/0060-lb-random (3.26s)`** — the new fixture is
  among them (green).
- **Zero** `--- FAIL` lines. The 61 prior dirs passed byte-exact through the new
  `case Cluster_RANDOM` (the ROUND_ROBIN / LEAST_REQUEST paths are untouched; the
  pick-loop seam is unchanged → structural behavior-neutrality CONFIRMED).

### Gate 2 — race + short across the repo

```
go test -race -short ./...        (exit 0)
```
- **PASS, no race.** 85 `ok` package lines; zero `DATA RACE` / `FAIL` markers
  (the grep hit only the literal-substring "fixtures" in `[no test files]` lines,
  not a real failure). `0060-lb-random/driver` ok 1.007s under `-race`. The
  mutex-guarded RNG via the REUSED `newPCGRNG` is race-clean.

### Gate 3 — build + vet + gofmt + lint + tidy (full repo)

```
go build ./...                                       exit 0
go vet ./...                                          exit 0, empty
gofmt -l internal/cluster/ test/fixtures/0060-lb-random/   EMPTY
gofmt -l .                                            EMPTY (whole repo)
golangci-lint run ./...                              exit 0, EMPTY
go mod tidy -diff                                    EMPTY → TIDY_CLEAN (exit 0)
```
- All five gates clean. `gofmt -l`, `golangci-lint`, and `go mod tidy -diff` each
  produced **empty** output (the success condition). **ZERO new go.mod dep**
  (AMEND-R1 holds — `go mod tidy -diff` empty).

### Gate 4 — conformance unaffected (RAN, not asserted)

The repo's conformance suites are `go test`-runnable (per
`docs/envoy-go/CONFORMANCE_PINS.md` + `test/conformance/{h2spec,proxy-wasm}/`), so
both were RUN rather than assert-unaffected:

```
go test -run TestH2Spec ./test/conformance/h2spec/ -count=1 -v
  53 tests, 53 passed, 0 skipped, 0 failed
  --- PASS: TestH2Spec (2.90s)     ok  …/h2spec  2.994s   →  h2spec 53/53

go test -run TestProxyWasmConformance ./test/conformance/proxy-wasm/ -count=1 -v
  --- PASS: TestProxyWasmConformance (0.27s)     ok  …/proxy-wasm  0.273s
  10 top-level families all PASS: logging, stop_iteration, shared_data,
  endianness, exports, security, runtime, wasm_vm, bytecode_util, pairs_util
    →  proxy-wasm 10/10
```

**Changed-files evidence** (`git diff --stat a9b26cf..HEAD` — `a9b26cf` is the
master tip the IMPL branched from) substantiating the no-touch claim — phase 35
modifies ONLY the LB layer (`internal/cluster/`) + the differential/fixture
harness + docs; it touches **no** HTTP / HTTP-2 / proxy-wasm code path, so the
conformance pins are structurally unaffected (the RUN above confirms it
empirically):

```
 docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md   | 181 +
 internal/cluster/manager.go                                |  19 +-
 internal/cluster/manager_test.go                           |  55 +-
 internal/cluster/random.go                                 |  44 +
 internal/cluster/random_test.go                            | 110 +
 test/differential/runner_test.go                           |   1 +
 test/fixtures/0060-lb-random/README.md                     | 162 +
 test/fixtures/0060-lb-random/driver/driver.go              | 438 +
 test/fixtures/0060-lb-random/driver/driver_test.go         |  54 +
 test/fixtures/0060-lb-random/expectations.yaml             |  40 +
 10 files changed, 1093 insertions(+), 11 deletions(-)
```

### Six-gate verdict

| gate | result |
|------|--------|
| differential (62 dirs) | **62/62 PASS** byte-exact (61 prior unchanged + `0060` green); 0 FAIL |
| `-race -short ./...`   | **PASS**, no race |
| build / vet            | clean |
| gofmt (`internal/cluster/`, `0060`, whole repo) | EMPTY (clean) |
| golangci-lint `./...`  | EMPTY (clean) |
| `go mod tidy -diff`    | EMPTY — TIDY_CLEAN (0 new dep, AMEND-R1) |
| h2spec                 | **53/53** unaffected (RAN) |
| proxy-wasm             | **10/10** unaffected (RAN) |

All gates GREEN. The new manager case is behavior-neutral on the 61 prior
fixtures and `0060` is the live RANDOM-LB cross-side check. Phase-35 IMPL Task 7
COMPLETE.

## Task 8 — completion bundle (ADR-0052 atomic landing; NO code changes — docs only)

The completion docs landed ATOMICALLY per ADR-0052. NO production-code change
since Task 7 — only `docs/envoy-go/` edits — so the differential is structurally
unaffected; it was re-run once for the atomic-landing guarantee (identical green).

### Step 1 — BEHAVIOR_CONTRACT.md (SPEC §9)

- Added the `### Load balancer — random (RANDOM)` subsection beside the
  least_request boundary: RANDOM acceptance (NO config message — bare
  construction); the uniform-pick semantics (ONE draw `rng() % n`, no active-count
  consult, no tie-break — the v1.37.2 `peekOrChoose` mirror; the contrast with
  least_request's P2C); the per-side RNG non-equivalence (anti-skew band-proven,
  never cross-side-exact); the healthy-set boundary (no health checking →
  all-hosts sampling).
- Updated the lb-policy reject-text entry: RANDOM retired from the rejected set →
  the THIRD accepted policy; the supported-set string
  `… ROUND_ROBIN, LEAST_REQUEST, RANDOM`; RING_HASH/MAGLEV stay the recorded
  DEPARTURE. **Byte-stable-reject (ADR-0080):** the doc string EXACTLY matches the
  production string in `internal/cluster/manager.go:257`
  (`unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)`).
- Departure/coverage records added: the mismatched-oneof silent-ignore under
  RANDOM (parity); the D-S35-2 shared-RNG-message cosmetic note; **the 0060
  anti-skew band re-tune** (recorded as an auditable IMPL-time empirical amendment
  to D-S35-3: `{12,32}` ~2.5σ flaked 2/20 → `{6,40}` ~4–5σ 60/60 flake-free,
  within the pinned PRINCIPLE; the 3 deliberate breaks still bite); NO new
  fuzzer/BackendKind (now family expectations); stat surface UNCHANGED at 1116
  (the SECOND zero-delta phase).
- Updated the §-deferral list: 6 candidates remain (RANDOM retired).

### Step 2 — DECISIONS.md: the full ADR-0234 entry (ADR-0044 in-place)

Authored the complete ADR-0234 (the `random` load-balancing policy): §Context
promoted from the SPEC §13 DRAFT (status PROPOSED → ACCEPTED); §Decision (the
stateless `randomLB`; the seam REUSE returning `noopRelease`; the bare manager
case + the reject-text change; the REUSED `newPCGRNG`; the `0060` anti-skew band
proof recording the as-built `{6,40}` band, noting the SPEC anticipated `{12,32}`);
§Consequences (the seam's first-reuse validation of ADR-0232's "zero cost" claim;
ADR-0024 UNAMENDED; the second zero-delta/no-fuzzer/no-BackendKind phase; the
band-re-tune amendment record; RING_HASH/MAGLEV remain the seam-EXTENDING future
rows). **DECISIONS.md tail advances ADR-0233 → ADR-0234; next-free ADR-0235.** NO
seam ADR (reuse — the thrift-33 single-ADR-on-reuse precedent).

### Step 3 — STATE.md + ROADMAP.md

- STATE active-phase → `phase 35 (load-balancer-random) done`; lifecycle-state →
  the phase-35 phase-done routing (controller squash-merges + pushes at
  stage-close; the Load-balancing family stays OPEN — 6 candidates remain). The
  prior PLAN/SPEC/BRAINSTORM active-phase + lifecycle + next-skill + last-commit
  entries DEMOTED to "Superseded (… preserved)" per the existing pattern.
- Counts: fixtures **61 → 62**; stat surface **1116** (zero delta); fuzzers
  **42**; BackendKind tail **33**; DECISIONS tail **ADR-0234** (next-free
  ADR-0235).
- ROADMAP row 35 `in-progress → done` (flat family row — NO parent rollup per
  ADR-0106); the Load-balancing family-§ `random` heading flipped IN-PROGRESS →
  DONE (6 candidates remain).

### D-S35-1..4 resolutions (final)

| Decision | Resolution (as-built) |
|----------|-----------------------|
| D-S35-1 file placement | `random.go` NEW sibling under `internal/cluster/`; manager `case clusterv3.Cluster_RANDOM` + reject text in `manager.go`; the `TestManager_Error_UnsupportedLBPolicy` doubly-hit retarget (`Cluster_RANDOM` → `Cluster_RING_HASH`) in `manager_test.go` — the `leastrequest.go` precedent. |
| D-S35-2 newRandom seeding | `newRandom` calls `newPCGRNG()` DIRECTLY, accepting the shared seed-error message (`"cluster: least_request: seed rng"` — a recorded cosmetic note on an effectively-unreachable boot-fail path), NO wrapper. |
| D-S35-3 band parameters | PLAN-proposed `{floor 12, ceiling 32}` re-tuned at Task 6 to **`{floor 6, ceiling 40}`** (the `{12,32}` band flaked 2/20; `{6,40}` is 60/60 flake-free within the pinned PRINCIPLE); final constants `{K=4, S=60, sum==64, floor 6, ceiling 40}`; the 3 `-count=1` deliberate breaks still bite. |
| D-S35-4 anti-skew test shape | A standalone deterministic-RNG anti-skew unit test (RANDOM does NOT avoid a held endpoint — the contrapositive of least_request), distinct from the `0060` fixture. |

### The three deliberate-break liveness proofs (Task 6, recorded here for the ledger)

| # | break | expected leg | result |
|---|-------|--------------|--------|
| i | `case Cluster_RANDOM`: `newRandom(endpoints)` → `newLeastRequest(endpoints, …)` (consult-the-counters — the canonical anti-skew break) | uniform FLOOR | FAIL `backend[1]=2 < 6`; reverted clean |
| ii | `random.go` `Pick`: `rng() % n` → `0` (single-host pin) | uniform CEILING | FAIL `backend[0]=64 > 40`; reverted clean |
| iii | `StatsAsserter` `upstream_cx_total` want 64 → 99 | stats prong | FAIL both sides observe 64; reverted clean |

### Step 5 — final six-gate re-run (docs-only delta since Task 7)

Re-ran build / vet / gofmt / lint / `-race -short` (all clean) + the full
differential suite once per ADR-0052 (identical green — 62/62 byte-exact; the only
delta since Task 7 is documentation, which the differential does not exercise).
Committed LOCAL-ONLY; the controller squash-merges + pushes at stage-close.

**Phase-35 IMPL Task 8 COMPLETE — all 8 tasks done.**
