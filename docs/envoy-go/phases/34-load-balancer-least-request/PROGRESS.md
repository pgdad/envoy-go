# Phase 34 — `least_request` load balancer — IMPL progress ledger

Worktree: `/home/esa/git/envoy-go/.worktrees/phase-34-impl` (branch `phase-34-impl`).
IMPL session of the 9-task TDD plan (`PLAN.md`). Commits are LOCAL-ONLY; the
controller pushes at stage-close.

## Task table

| # | Task | Status |
|---|------|--------|
| 1 | Baselines gate + PROGRESS.md | done |
| 2 | Reshape `loadBalancer` seam + `cluster.go` threading (SPEC Tasks 2+3 MERGED — the `Pick` signature change is compiler-coupled to its `cluster.go` call sites) | done |
| 3 | `leastRequest` P2C type (`leastrequest.go`) | done |
| 4 | Manager acceptance + `parseLeastRequestLbConfig` + reject matrix | done |
| 5 | In-process skew integration test + boot smoke | done |
| 6 | `0059-lb-least-request` differential fixture | done |
| 7 | Band tuning + 3 deliberate-break liveness proofs | done |
| 8 | Full 61-dir differential re-verify + race + conformance | done |
| 9 | Completion bundle (ADR-0052 atomic landing) | done |

**All 9 tasks complete. Phase 34 phase-done.** Commits (local-only; the controller squash-merges + pushes at stage-close): Task 1 `6aec7bf`, Task 2 `0635ac4`, Task 3 `ece00c4`, Task 4 `e34d647`, Task 5 `56869bb`, Task 6 `3c1db1a`, Task 7 `a1de642`, Task 8 `f3dc6d7`, Task 9 (this commit).

## Step 1 — count anchors (re-confirmed against the IMPL-session tip)

Re-confirmed BEFORE touching any code (the established first-task discipline). Each
row records the EXACT recipe used + its output + whether it matched the plan's
expected value.

| Anchor | Recipe (run from the worktree root) | Output | Expected | Match |
|--------|-------------------------------------|--------|----------|-------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `60` | 60 | YES |
| fixtures tail | `ls -d test/fixtures/[0-9]* \| tail -1` | `test/fixtures/0058-thrift-boot-reject` | `0058-thrift-boot-reject` | YES |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 | YES |
| stat surface | BEHAVIOR_CONTRACT doc count — `grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md` (line 466: "Phase 33 extension — 1091 → 1116 internal names") | `1116` | 1116 | YES |
| BackendKind tail | `grep -rn "BackendKind = " test/differential/fixture/fixture.go \| tail -1` | `TCPThriftResponder BackendKind = 33` | 33 | YES |
| DECISIONS tail | `grep -oE "ADR-[0-9]{4}" docs/envoy-go/DECISIONS.md \| sort -u \| sort -t- -k2 -n \| tail -1` | `ADR-0234` (cross-ref); last ADR with a body heading is `ADR-0233` | ADR-0233 (next-free ADR-0234) | YES |
| `go build ./...` | `go build ./... && echo BUILD_OK` | `BUILD_OK` | green | YES |

### Recipe notes (canonical-recipe discovery)

- **fuzzers (42):** the canonical recipe is the one pinned in the phase-33 PLAN
  count table — `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`.
  This counts ONLY `fuzz_test.go` files (so it excludes seed/corpus helper funcs that
  live elsewhere — it does NOT over-count). Tail fuzzers present: `FuzzOAuth2ConfigParse`,
  `FuzzAdmissionControlConfigParse`, `FuzzMongoDecode` (the 42 are spread across packages;
  the most-recently-added is the 42nd `FuzzThriftDecode` per ADR-0231). No new fuzzer in
  phase 34 (ADR-0233 — the FIRST no-fuzzer phase; no wire decode), so the tip stays 42.
- **stat surface (1116):** there is NO programmatic test asserting the number
  (`grep -rn "1116" --include="*.go"` finds zero hits). The canonical source-of-truth is
  the **BEHAVIOR_CONTRACT doc count** (exactly as the phase-33 PLAN count table names it:
  recipe column = "BEHAVIOR_CONTRACT doc count"). The tip value is pinned at
  `docs/envoy-go/BEHAVIOR_CONTRACT.md:466` ("Phase 33 extension — 1091 → 1116 internal
  names"). ADR-0233 holds the surface at 1116 (ZERO stat-name delta in phase 34).
- **DECISIONS tail (ADR-0233):** `grep -c "^ADR-0"` returns 63, which is NOT the tail
  number — it counts only the body-INLINE `^ADR-0` lines, and ADR ids also appear in
  multi-line headings. The reliable tail recipe is the numeric sort above. The last ADR
  with a `## ADR-NNNN:` heading is **ADR-0233** (the `least_request` policy); ADR-0232
  (the LB acquire/release seam) precedes it. Both ADR-0232 and ADR-0233 §Context drafts
  landed at the SPEC. ADR-0234 appears only as a forward cross-reference ("next-free
  ADR-0234"), so the next-free ADR is **0234**.

## Step 2 — as-built line anchors (re-pinned against the IMPL-session tip)

Every plan pattern matched VERBATIM at the tip (no deviations). Actual line numbers and
matched text recorded below for Tasks 2-4 to target.

| Anchor | Pattern | File:line | Matched text |
|--------|---------|-----------|--------------|
| A — interface to reshape | `Pick() (Endpoint, error)` | `internal/cluster/loadbalancer.go:9` | `Pick() (Endpoint, error)` (the `loadBalancer` interface method; `roundRobin` impl at `:23`) |
| B — guard to replace | `only ROUND_ROBIN lb_policy supported` | `internal/cluster/manager.go:216` | `return nil, fmt.Errorf("cluster: %q: only ROUND_ROBIN lb_policy supported; got %s", name, c.GetLbPolicy())` |
| C — lb literal construction | `lb:             &roundRobin` | `internal/cluster/manager.go:234` | `lb:             &roundRobin{endpoints: endpoints},` |
| D — connWithGauge dec (Dial) | `dec: c.upstreamCxActive.Dec` | `internal/cluster/cluster.go:222` | `return &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}, ep, nil` |
| D — connWithGauge dec (AcquireH1) | `dec: c.upstreamCxActive.Dec` | `internal/cluster/cluster.go:289` | `wrapped := &connWithGauge{Conn: final, dec: c.upstreamCxActive.Dec}` |
| E — direct-pick consumer (thriftproxy) | `cl.PickEndpoint()` | `internal/filter/network/thriftproxy/filter.go:90` | `if _, err := cl.PickEndpoint(); err != nil {` |
| E — direct-pick consumer (httpclient) | `cl.PickEndpoint()` | `internal/httpclient/httpclient.go:280` | `ep, err := cl.PickEndpoint()` |
| F — streaming echo backend | `func acceptEchoCounting` | `test/differential/runner_test.go:1329` | `func acceptEchoCounting(ln net.Listener, counter *atomic.Uint64) {` |

Deviations from the plan's expected patterns: **NONE.** Every Step-2 grep matched
verbatim (no whitespace/fallback substitution needed). The `dec: c.upstreamCxActive.Dec`
attach-points are confirmed ×2 (Dial `:222`, AcquireH1 `:289`) exactly as the plan
anticipated.

## ADR-0045 split-gate re-check

**Verdict: NO SPLIT.** 9 tasks ≤ ~25 (the task-count ceiling) and ~155–255 prod LoC ≤
~1500 (the LoC ceiling). The 34.1/34.2 escape valve remains **UNCONSUMED**. (Re-confirmed
at the PLAN; this is the IMPL-tip re-check.)

## Task 5 — in-process skew integration test + boot smoke

Two test-only additions (NO production .go change in this commit — `leastrequest.go`
is byte-identical before/after; the Step-3 inversion was temporary and reverted).

- **Skew test** (`leastrequest_test.go:TestLeastRequest_SkewAvoidsLoadedEndpoint`):
  deterministic `seqRNG(0,1)` cycling, choiceCount 2 → every Pick draws indices {0,1}.
  Holding the first 3 picks (never releasing) accumulates load:
  pick1 {0:0,1:0} tie → a (active=[1,0,0]); pick2 {0:1,1:0} → b (active=[1,1,0]);
  pick3 {0:1,1:1} tie → a (active=[2,1,0]). The 4th (assertion) Pick draws {0:2,1:1}
  → strict < selects index 1 (host "b") ≠ "a" → PASS. The sequence is both passing
  AND break-sensitive (see Step-3).
- **Boot smoke** (`manager_test.go:TestManager_LeastRequest_BootSmoke`): cc=10 (the 0059
  config), 3 endpoints (127.0.0.1:9001-9003); `NewManager` + `m.Get("c_lr")` +
  `cl.PickEndpoint()` (immediate-release path) returns an in-range port. API matched the
  plan verbatim (`m.Get(name) (*Cluster, bool)`; `Cluster.PickEndpoint() (Endpoint, error)`;
  `Endpoint.Port` is `uint32`) — no adaptation needed.

### Step-3 inverted-comparison liveness (`-count=1`, `reference_differential_break_protocol_count1`)

Temporarily inverted `leastRequest.Pick`'s comparison `candActive < bestActive` →
`candActive > bestActive` in `leastrequest.go`. Under the inversion the 4th Pick keeps
index 0 (`2 > 1` is false; best stays 0) → host "a" → the skew test FAILED:

```
--- FAIL: TestLeastRequest_SkewAvoidsLoadedEndpoint (0.00s)
    leastrequest_test.go:126: loaded endpoint a was re-picked over lighter b; skew not working
```

Then REVERTED the comparison back to strict `<` (`git diff internal/cluster/leastrequest.go`
is EMPTY — byte-identical) and re-ran: GREEN. The skew test is non-vacuous — it
discriminates the strict-< skew from its inversion. `-count=1` defeated test caching.

gofmt clean; golangci-lint clean (the misspell linter required "analog" over the British
"analogue" in the doc comment).

## Task 7 — band-constant tuning + 3 deliberate-break liveness proofs

Test-only commit (driver band-constant promotion + README + this ledger). NO production
.go change: `internal/cluster/leastrequest.go` is byte-identical before/after — every
deliberate break was temporary and REVERTED (`git diff` EMPTY).

### Code-quality carry-forward (Task-6 review)

- **Band bounds promoted to named constants** in `driver.go` — `starvationMax = 12` and
  `concentrationMin = 16`, each with a one-line rationale, used in BOTH the check AND the
  error message (a future tune touches ONE place; was triplicated as inline `12`/`16`).
- **Stale `driver_test.go` comment aligned** — `TestAssertDistribution_InBand` now cites a
  real Task-6-observed sorted row `{2, 26, 36}` (was the non-matching `{18,2,14}`→`{6,27,31}`
  provenance); the in-band inputs are valid points under the final constants.

### Step 1 — flake check (`-count=20`, 20/20 PASS)

`go test ./test/differential/ -run 'TestDifferential/0059' -count=20` → 20/20 PASS (boots
the reference container 20×). 40 sorted-row observations (20 per side; a temporary
`fmt.Printf("DIST_OBS …")` line captured them, then REVERTED):

| side      | c1 (min/max) | c2 (min/max) | c3 (min/max) | sum |
|-----------|--------------|--------------|--------------|-----|
| reference | 2 / 2        | 22 / 31      | 31 / 40      | 64  |
| subject   | 2 / 2        | 21 / 31      | 31 / 41      | 64  |

`c1` ALWAYS 2 (margin 10 to `<=12`); `c2` 21–31 (min margin 5 to `>=16`); sum always 64.
**Final constants UNCHANGED from the plan's 12/16 — no widening needed.**

### Steps 2-4 — deliberate breaks (`-count=1`, `reference_differential_break_protocol_count1`)

Each break targets ONE assertion leg; each failed the EXACT predicted leg, then was
REVERTED. A leg that cannot fail is a dead assertion (the `0030` lesson —
`reference_differential_asserter_dispatch`).

| break | edit (REVERTED) | failing leg / `-count=1` output |
|-------|-----------------|---------------------------------|
| (i) inverted comparison | `leastrequest.go`: `candActive < bestActive` → `>` | `distribution: subject: concentration: c2=0 < 16 (inverted comparison?)` |
| (ii) no-op release | `leastrequest.go`: `release := func() {}` | `distribution: subject: starvation: c1=21 > 12 (no skew? round-robin?)` |
| (iii) corrupt stat want | `driver.go`: `upstream_cx_total` want 64 → 99 | `ref/subj cluster.c_echo.upstream_cx_total = 64, want 99` |

All three legs PROVEN live. After all reverts: `git diff internal/cluster/leastrequest.go`
EMPTY; the fixture re-runs GREEN (`-count=1`). gofmt clean; golangci-lint clean.

## Task 8 — full six-gate re-verify (the seam-reshape REAL guard)

VERIFICATION-only commit (this ledger entry; NO production `.go` change — `git status` clean
before the edit). The seam reshape (Task 2: `loadBalancer.Pick` → acquire/release with a
`roundRobin` no-op release) is proven behavior-neutral: all 60 PRIOR fixtures stay byte-exact
and `0059` is green, for a 61/61 clean differential.

### Gate 1 — full differential suite (61 dirs)

Recipe: `go test ./test/differential/ -count=1 -v` (FULL suite, NO `-run` filter — every
fixture subtest executes; `reference_differential_break_protocol_count1` `-count=1`). Fixture
dirs: `ls -d test/fixtures/[0-9]* | wc -l` = **61** (the 60 prior + `0059-lb-least-request`).

| run | top-level | PASS | FAIL | SKIP | wall-clock | `0059` |
|-----|-----------|------|------|------|------------|--------|
| #1  | FAIL (exit 1) | 60 | 1 (`0025-http-adaptive-concurrency`) | 0 | 203.2s | PASS |
| #2  | **ok** (exit 0) | **61** | **0** | **0** | 203.7s | PASS |

**Run #1's single FAIL was a transient host-port-collision flake, NOT a seam regression.** The
failure signature was a bind error, not a byte/assertion mismatch:
`listener "l_b_overflow": bind 0.0.0.0:39314: ... address already in use` →
`runner_test.go:939: subj start: subject ready: EOF`. A seam-reshape regression would surface as
a distribution/stat assertion mismatch (RR pick-sequence drift), which it did NOT. Proof it is a
flake: `0025` re-run in isolation (`-run 'TestDifferential/0025-http-adaptive-concurrency'
-count=1`) → **PASS** (5.10s). Run #2 (full suite, fresh) → **61/61 clean, exit 0**; that run
ALSO hit a port collision (`l_s2` :37968) but the runner's retry-with-fresh-ports recovered it
(`runner_test.go:1026: subj start attempt 1 failed (subject ready: EOF); retrying with fresh
ports`) — confirming the root cause is host-port races under heavy parallel container churn, not
the LB seam. **REAL-guard verdict: the 60 prior fixtures (incl. all HTTP/h2/wasm/protocol
fixtures) are byte-exact through the `roundRobin` no-op-release adoption; `0059` green. 61/61.**

### Gate 2 — race + short across the repo

Recipe: `go test -race -short ./...`. Result: **PASS, no DATA RACE / panic / FAIL** (grep over the
full output for `FAIL|panic|DATA RACE|WARNING` → zero hits). Covers the mutex-guarded RNG, the
atomic active-request counters, and the `connWithGauge` `sync.Once` dec. (`-short` skips the Docker
differential suite — fast unit-level race check.)

### Gate 3 — build / vet / gofmt / lint / tidy (full repo)

| check | recipe | result |
|-------|--------|--------|
| build | `go build ./...` | `BUILD_OK` |
| vet | `go vet ./...` | clean (`VET_OK`) |
| gofmt | `gofmt -l internal/cluster/ test/fixtures/0059-lb-least-request/` | empty (clean) |
| lint | `golangci-lint run ./...` | exit 0, zero findings |
| tidy | `go mod tidy -diff && echo TIDY_CLEAN` | `TIDY_CLEAN` — ZERO new go.mod dep (AMEND-L1) |

### Gate 4 — conformance unaffected (RAN, not merely asserted)

The repo's conformance recipes (`docs/envoy-go/CONFORMANCE_PINS.md`) are runnable Go tests in
this environment, so both were RUN for actual evidence (phase 34 also independently asserts-unaffected:
it touches only `internal/cluster` (the LB seam + `leastRequest`) + adds a tcp_proxy fixture,
NO HTTP/h2/proxy-wasm code path — and the prior HTTP/h2/wasm differential fixtures stayed
byte-exact in Gate 1).

- **h2spec — 53/53.** `go test -count=1 ./test/conformance/h2spec/ -run TestH2Spec` → `ok` (2.66s);
  verbose report line: `53 tests, 53 passed, 0 skipped, 0 failed` / `h2spec conformance report:
  53 total tests, 0 failures` (sum of per-section `N/N passed` = 53). **53/53.**
- **proxy-wasm — 10/10 families.** `go test -count=1 ./test/conformance/proxy-wasm/
  -run TestProxyWasmConformance` → `ok` (0.255s); 10 top-level family subtests all PASS
  (logging, stop_iteration, shared_data, endianness, exports/security/runtime/wasm_vm,
  bytecode_util, pairs_util). **10/10.**

### Six-gate verdict

ALL GREEN. 61/61 differential byte-exact (60 prior unchanged + `0059`); `-race -short` clean;
build/vet/gofmt/lint clean; `go mod tidy -diff` empty; h2spec 53/53 + proxy-wasm 10/10 (RAN).
The Task-2 seam reshape is proven behavior-neutral.

## Task 9 — completion bundle (ADR-0052 atomic landing; DOCS-ONLY)

The documentation completion bundle, landed atomically with the code in this commit (DOCS-ONLY —
no `.go` change). The five durable docs updated:

- **`docs/envoy-go/BEHAVIOR_CONTRACT.md`** — NEW subsection `### Load balancer — least_request (LEAST_REQUEST)`
  in the `## TCP proxy` section (acceptance + `choice_count` default-2/`>= 2`/no-clamp; the P2C semantics
  [with-replacement, strict `<`, first-drawn ties — the v1.37.2 mirror]; cx-as-rq active-count semantics
  [TCP exact / the H1 idle-pooled-conn approximation boundary]; the per-side RNG non-equivalence [band-proven];
  the departure/coverage records — NEW lb-policy reject text, bias/slow_start DEPARTURE rejects, mismatched-oneof
  silent-ignore, direct-pick load invisibility, `upstream_rq_total` per-side, NO new fuzzer/BackendKind, stat
  surface UNCHANGED at 1116). The deferral list (the `LB policies other than ROUND_ROBIN` line — verified at the
  current anchor, line ~897) RETIRED LEAST_REQUEST (the 7 remaining candidates stay).
- **`docs/envoy-go/DECISIONS.md`** — **ADR-0232** (the LB acquire/release seam) + **ADR-0233** (the least_request policy)
  §Decision + §Consequences bodies landed IN-PLACE per ADR-0044 (Status PROPOSED → ACCEPTED; NO new ADR number —
  tail STAYS ADR-0233, next-free ADR-0234). §Decision/§Consequences structure mirrors the completed ADR-0231.
- **`docs/envoy-go/STATE.md`** — active-phase → `phase 34 (load-balancer-least-request) done` (the PLAN-done entry
  demoted into the superseded chain); lifecycle-state/next-skill → next-phase routing; the count bullets updated
  (fixtures 60 → **61**; stat surface **1116** zero-delta; fuzzers **42**; BackendKind tail **33**; DECISIONS tail
  **ADR-0233** bodies-landed).
- **`docs/envoy-go/ROADMAP.md`** — row 34 `in-progress → done` (a flat family row — NO parent rollup per ADR-0106);
  the `### Load balancing family` heading updated (least_request DONE; the family STAYS OPEN — 7 candidates remain).
- **`docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md`** — this file: the task table all-done + this section.

### Count verification (re-confirmed against reality at Task 9, not copied)

| Anchor | Recipe | Output | Expected |
|--------|--------|--------|----------|
| fixtures | `ls -d test/fixtures/[0-9]* \| wc -l` | `61` (tail `0059-lb-least-request`) | 61 |
| fuzzers | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` | `42` | 42 |
| stat surface | BEHAVIOR_CONTRACT doc count (`grep -n "1116" …BEHAVIOR_CONTRACT.md` line 466) | `1116` | 1116 |
| BackendKind tail | `grep -rn "BackendKind = " test/differential/fixture/fixture.go \| tail -1` | `TCPThriftResponder BackendKind = 33` | 33 |
| DECISIONS tail | last `## ADR-NNNN:` heading | `ADR-0233` (next-free ADR-0234) | ADR-0233 |

All match — the deliberate zero-stat-delta / no-fuzzer / no-BackendKind firsts HELD.

### Final-gate re-run (Task 9)

DOCS-ONLY change → the fast gates + the new fixture are sufficient (the docs cannot affect differential results;
the full 61-dir suite was already verified GREEN at Task 8, commit `f3dc6d7`). Ran: `go build ./...` + `go vet ./...` +
`gofmt -l .` (empty) + `golangci-lint run ./...` (clean) + `go test -race -short ./...` (green) + a single
`go test ./test/differential/ -run 'TestDifferential/0059' -count=1` (PASS). `git diff --stat` confirmed ONLY
`docs/` files staged.

### D-question resolutions (final)

The §12 PLAN/IMPL D-questions all RESOLVED as the PLAN anticipated (no deviations): D-S34-1 the file split
(`leastrequest.go` NEW sibling; the interface + `roundRobin` in `loadbalancer.go`; `parseLeastRequestLbConfig` +
`defaultChoiceCount` in `manager.go`); D-S34-2 the mutex-guarded crypto-seeded `math/rand/v2` PCG (injectable via
`newLeastRequestWithRNG`; the `crypto/rand` read error threads out → boot-fail); D-S34-3 the band constants
(K=4/S=60/`c1<=12`/`c2>=16`/`sum==64`) + the 20/20-flake protocol + the 3 `-count=1` breaks; D-S34-4 the write+read-echo
establishment witness (`acceptEchoCounting` streaming echo); D-S34-5 release composed into the ADR-0063 `connWithGauge`
`dec` closure (struct unchanged). The SPEC's D-L1..D-L7 empirical pins held as built (cx-as-rq, FULL_SCAN deferred,
zero stat delta, OPTION C, the P2C source-mirror, the blast radius).
