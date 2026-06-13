# Phase 37 — load-balancer-maglev — IMPL PROGRESS ledger

Branch: `phase-37-impl` (worktree `.worktrees/phase-37-impl`). Commits LOCAL-ONLY; the controller pushes.

ADR-0045 re-check verdict: **NO SPLIT** — ~140–200 LoC across 10 bite-sized TDD tasks; a single self-contained LB policy (`maglev.go` + manager case + 2 gauges + 1 fixture). No new packages, no go.mod deps.

## Task table

| # | Task | Status |
|---|------|--------|
| 1 | baselines/anchors gate + PROGRESS.md | **complete** |
| 2 | isPrime helper (maglev.go) | **complete** |
| 3 | maglevLB table build (sort + offset/skip + populate) | **complete** |
| 4 | maglevLB.Pick (+ folded property test, D-S37-5) | **complete** |
| 5 | manager acceptance + parse + retarget | **complete** |
| 6 | the 2 maglev_lb.* gauge registrations | **complete** |
| 7 | the 0063-lb-maglev fixture | **complete** |
| 8 | deliberate-break liveness + ≥20-run flake | **complete** |
| 9 | full differential re-verify | **complete** |
| 10 | completion bundle (ADR-0238 + the landing discipline ADR-0052) | **complete** |

## Count anchors (Step 1 — confirmed against IMPL-session tip)

| Anchor | Expected | Observed | Recipe |
|--------|----------|----------|--------|
| fixtures count | 64 | **64** | `ls -d test/fixtures/[0-9]* \| wc -l` |
| fixtures tail | 0062-lb-ring-hash-http | **test/fixtures/0062-lb-ring-hash-http** | `ls -d test/fixtures/[0-9]* \| tail -1` |
| BackendKind tail | 33 | **TCPThriftResponder BackendKind = 33** (line 562) | `grep -n "BackendKind = " test/differential/fixture/fixture.go \| tail -1` |
| fuzzers | 42 | **42** | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` |
| DECISIONS tail | ADR-0237 (next-free ADR-0238) | **ADR-0237** | `grep "^## ADR-0" docs/envoy-go/DECISIONS.md \| tail -1` |
| stat surface | 1119 (DOC count, no programmatic golden) | **1119** (BEHAVIOR_CONTRACT.md lines 953, 974, 4078) | `grep -n "1119" docs/envoy-go/BEHAVIOR_CONTRACT.md` |
| build | BUILD_OK | **BUILD_OK** | `go build ./...` |

Note: the ADR-0238 §Context was a DRAFT in SPEC §13; Task 10 promoted it to DECISIONS.md (§Context + §Decision + §Consequences, PROPOSED → ACCEPTED per ADR-0044) — the completion ADR is **ADR-0238** (the next-free number; tail advances ADR-0237 → ADR-0238, next-free now ADR-0239). RECONCILED: the PLAN's task name "completion bundle (ADR-0052)" refers to FOLLOWING the existing process ADR-0052 (atomic six-gate landing discipline), NOT to creating/editing an ADR-0052 entry. The new policy ADR IS ADR-0238; no ADR-0052 entry is created.

## As-built line anchors (Step 2 — actual line numbers observed)

| Symbol | File | Line(s) |
|--------|------|---------|
| `Pick(hashKey uint64, hasHash bool)` (loadBalancer iface) | internal/cluster/loadbalancer.go | 21 |
| `noopRelease = func() {}` | internal/cluster/loadbalancer.go | 26 |
| `errNoEndpoints` (return) | internal/cluster/loadbalancer.go | 41 |
| `func xxHash64` | internal/cluster/hash.go | 40 |
| `func xxHash64Seed` | internal/cluster/hash.go | 46 |
| `func (e Endpoint) Addr` | internal/cluster/cluster.go | 39 |
| `func newPCGRNG` | internal/cluster/leastrequest.go | 63 |
| `var _ loadBalancer = (*ringHashLB)(nil)` | internal/cluster/ringhash.go | 60 |
| `func newRingHashWithRNG` | internal/cluster/ringhash.go | 74 |
| `func (rh *ringHashLB) Pick` | internal/cluster/ringhash.go | 129 |
| `switch c.GetLbPolicy()` | internal/cluster/manager.go | 243 |
| `case clusterv3.Cluster_RING_HASH` | internal/cluster/manager.go | 262 |
| `unsupported lb_policy` (reject text) | internal/cluster/manager.go | 275 |
| `func registerClusterMetrics` | internal/cluster/manager.go | 99 |
| `(*ringHashLB); ok` (gauge type-assert) | internal/cluster/manager.go | 110 |
| `func parseRingHashLbConfig` | internal/cluster/manager.go | 347 |
| `func TestManager_Error_UnsupportedLBPolicy` | internal/cluster/manager_test.go | 320 |
| `Cluster_MAGLEV` (retarget reject policy) | internal/cluster/manager_test.go | 322 |
| `func gaugeValue` | internal/cluster/manager_test.go | 1051 |
| `internal/cluster/maglev.go` | — | **ABSENT** (expected — Task 3 creates it) |

## Reject-text blast radius (Step 3 — exactly THREE sites, AMEND-M5)

1. Production string: **ONLY** `internal/cluster/manager.go` (line 275) — `grep -rln "unsupported lb_policy" internal/ cmd/` returns only this file.
2. Fixture pins: **ZERO** — `grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH" test/` is EMPTY (exit 1). No fixture pins the text → no boot-reject dir.
3. Doc line: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the reject-text doc line).

The third *code* site is the unit pinner `TestManager_Error_UnsupportedLBPolicy` (manager_test.go:320, currently retargeted to `Cluster_MAGLEV`). Task 5 must re-retarget this away from MAGLEV (which becomes accepted) and update the reject-text suffix to include MAGLEV in the supported list.

## Seam + producer REUSE-only confirmation (Step 4 — AMEND-M3/M6)

- `c.lb.Pick(` funnel: **ONLY** `internal/cluster/cluster.go` — the single Pick funnel is UNCHANGED. maglevLB plugs into the same `c.lb.Pick(hk, ok)` funnel exactly as ringHashLB does; the loadBalancer interface (loadbalancer.go:21) is unchanged.
- LANDED producers (`cluster.HashSourceIP` / `cluster.HashHeaderValues` / `WithHashKey`): present in cluster.go (+ seam_test.go), tcpproxy/filter.go, http/router/router.go, router_h2.go (+ router_test.go). **Phase 37 touches NONE of these** — the ADR-0237 producers (tcp_proxy source_ip plane + HTTP router hash_policy plane) feed the same hash key into the unchanged dial path; maglev is a NEW LB *consumer* of the existing key, not a new producer.

## D-S37 resolutions (from SPEC §13 / PLAN)

- **D-S37-1 (file placement):** `maglevLB` + table build + `Pick` + `isPrime` → NEW `internal/cluster/maglev.go`; manager `case Cluster_MAGLEV` + `parseMaglevLbConfig` + reject-text edit → `manager.go`; the gauge type-assert → `registerClusterMetrics`; the doubly-hit retarget → `manager_test.go`. **ZERO new packages.**
- **D-S37-2 (primality reject):** house-prefixed `cluster: %q: maglev_lb_config.table_size (%d) must be a prime number`; `isPrime` is a standalone helper (trial division to √n).
- **D-S37-3 (weighting):** equal-weight MVP; weighted maglev DEFERRED.
- **D-S37-4 (fixture):** `0063-lb-maglev` constants N=16 × K=16 = 256 + 8 `/health`; break protocol = scatter-key / collapse-table / stats-drop, run with `-count=1` (defeat go-test caching); ≥20-run flake check.
- **D-S37-5 (property test):** the maglev property test is FOLDED into Task 4's `maglev_test.go` (NOT a `Fuzz*` entry; fuzzers STAY 42).

## Task 2 — isPrime oracle record (D-S37-2)

`isPrime(uint64)` (trial division to √n) committed verbatim from the PLAN. Test vectors oracle-verified with an independent trial-division check (sympy unavailable in env):

| n | oracle | note |
|---|--------|------|
| 100 | composite | reference's rejected composite (AMEND-M5) |
| 65537 | prime | default table_size (Fermat prime 2^16+1) |
| 5000011 | prime | the PGV cap (a faithful cap is itself prime) |
| 5000012 | composite | cap+1 |
| 4999999 | **prime** | **PLAN test vector `{4999999, false}` was WRONG** |
| 5000009 | composite | = 7 × 714287 — **substituted** for 4999999 per the PLAN's explicit contingency |

The PLAN-vector correction is the only deviation; it follows the PLAN's own instruction ("If 4999999 turns out prime, substitute another pinned near-cap composite"). Controller re-verified the oracle independently.

## Anchor verdict

ALL anchors MATCH the PLAN's assumptions. No drift detected. Safe to proceed to Task 2.

## Task 8 — deliberate-break liveness (`-count=1`)

PROVED each 0063 prong is LIVE (go-test caching serves a stale PASS otherwise —
`reference_differential_break_protocol_count1`; every run used `-count=1` and the
`-run 'TestDifferential/0063'` prefix selector, NOT `-run '0063'` which matches zero
subtests → `reference_differential_run_selector`). Each break was applied to
PRODUCTION code (or the driver's expected stat), observed `--- FAIL`, then `git
restore`d — production tree left UNCHANGED (no `git checkout <sha>`, no
`--amend`; clean `git status` verified after each revert). Branch stayed
`phase-37-impl` (not detached) throughout.

**Flake check:** `-count=20` → **20/20 PASS** (non-vacuous: 20 `--- PASS:
TestDifferential/0063-lb-maglev` lines under `-v`). The affinity leg is
DETERMINISTIC (fixed table + fixed X-Hash keys) → cannot flake; the spread leg
(`>= 2` nonzero) per-side collapse probability `3·(1/3)^16 ≈ 7e-8` → flake-free.

| # | break | site | prong(s) bitten | observed `--- FAIL` |
|---|-------|------|-----------------|---------------------|
| i | make `hashKey = mg.rng()` unconditional (scatter the key — ignore it on every Pick) | `maglevLB.Pick` (`maglev.go`) | **affinity** | `distribution: subject affinity: backend[0]=83 not a multiple of 16 (key scattered? an X-Hash value split across backends)` |
| ii | collapse the BUILT table to host-0 AND corrupt the build gauge tallies (min→0, max→65537) — inserted after the populate loop, before the min/max tally | `newMaglevWithRNG` (`maglev.go`) | **spread** + **both `maglev_lb.*` gauges** | spread: `distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (table collapsed?)`; gauge-max: `cross-side mismatch cluster.c_echo.maglev_lb.max_entries_per_host: ref=21846 subj=65537` + `subj … max_entries_per_host = 65537, want 21846`; gauge-min: `cross-side mismatch cluster.c_echo.maglev_lb.min_entries_per_host: ref=21845 subj=0` + `subj … min_entries_per_host = 0, want 21845` |
| iii | corrupt one cross-equal want (`upstream_cx_total` → `totalReqs+1`) | `AssertStats` (`driver/driver.go`) | **stats** (`StatsAsserter`) | `ref cluster.c_echo.upstream_cx_total = 256, want 257` + `subj … upstream_cx_total = 256, want 257` |

### BUILD-level vs Pick-level collapse contrast (the key lesson — `reference_differential_asserter_dispatch`)

Break (ii) is intentionally **BUILD-level**, not Pick-level. The
`maglev_lb.{min,max}_entries_per_host` gauges are computed from the build-time
`count[]` tallies (in `newMaglevWithRNG`), NOT from `Pick`. A WEAKER **Pick-level**
short-circuit (returning `table[0]` in `Pick` without touching the build) would bite
ONLY the spread leg; the gauges would STAY 21845/21846, the cross-equal gauge
assertion would NOT fire, and the gauge prong would be VACUOUSLY green. The
BUILD-level collapse used here corrupts the table layout (→ spread) AND the `count[]`
tallies (→ gauges 0/65537), so the gauge prong is PROVEN live. This is precisely why
break (ii) targets `newMaglevWithRNG` (build) and not `Pick`.

After all three reverts: differential GREEN (`-count=1`), `git status` clean (only
the README.md + this PROGRESS.md doc edits staged — NO `maglev.go`, NO `driver.go`),
branch `phase-37-impl`.

## Task 9 — six-gate verification evidence

Run by the controller (the IMPL session has Docker 28.1.1 + `envoyproxy/envoy:contrib-v1.37.2`).

| Gate | Command | Result |
|------|---------|--------|
| differential (full) | `go test ./test/differential/ -count=1` | **`ok ... 213.913s` (exit 0)** — all 65 dirs byte-exact (64 prior unchanged through the new `case Cluster_MAGLEV` + `0063` green). The MAGLEV arm is reached only for MAGLEV clusters, so the 64 prior fixtures are structurally untouched. |
| race + short | `go test -race -short ./...` | **clean** — no race, no failures across the repo |
| build | `go build ./...` | **BUILD_OK** |
| vet | `go vet ./...` | clean |
| gofmt | `gofmt -l internal/ test/fixtures/0063-lb-maglev/` | clean (no files listed) |
| lint | `golangci-lint run ./...` (full repo) | clean |
| tidy | `go mod tidy -diff` | **TIDY_CLEAN** — ZERO new go.mod dep (AMEND-M1; the enum + config are in the pinned `/envoy v1.32.4`) |

### Gate 4 — h2spec / proxy-wasm conformance — ASSERTED-UNAFFECTED (with rationale)

**Disposition: asserted-unaffected** (the phase-35/36 precedent). No Makefile / fast
self-contained recipe exists; the h2spec gate requires pulling a heavy external
Docker image, so the plan's assert-with-rationale arm applies. The rationale is
grounded in the verified change scope (`git diff --name-only master...HEAD`): phase 37
modifies ONLY `internal/cluster/*` (`maglev.go`/`maglev_test.go` + the `manager.go`
MAGLEV case/parse/reject + the 2 `maglev_lb.*` gauges + the `manager_test.go`
retarget), the one-line `0063` registration in `test/differential/runner_test.go`,
and the `0063` fixture + docs. It touches **NO HTTP/2, h2spec-gate, or proxy-wasm
code path** — so the **h2spec 53/53** and **proxy-wasm 10/10** baselines are
**unaffected by construction**.

### Task 9 — verdict

All six gates GREEN: 65-dir differential byte-exact (64 prior unchanged through the
new MAGLEV case + `0063`), `-race -short` clean (no race), build/vet/gofmt/lint clean,
`go mod tidy -diff` empty (zero new dep), h2spec 53/53 + proxy-wasm 10/10
asserted-unaffected by change-scope construction. NO production or test code touched.
**Task 9 COMPLETE.**

## Task 10 — completion bundle (docs-only; ADR-0238 + the ADR-0052 landing discipline)

The completion docs authored (LOCAL commit; the controller does the final six-gate
re-run + squash-merge + push). Five docs touched:

- **BEHAVIOR_CONTRACT.md** — a NEW `### Load balancer — maglev (MAGLEV)` subsection
  beside ring_hash (acceptance + the cap/primality gate [both reference PARITY] +
  the Maglev table semantics [sort-by-`Addr()` + offset/skip + the populate loop +
  `table[hashKey % M]` + the no-hash random fallback + the unused `attempt`
  boundary] + the seam/producer REUSE + the healthy-set boundary + the 3
  deliberate-break records + the cross-side-host-identity-infeasible posture); the 2
  `maglev_lb.{min,max}_entries_per_host` gauges added to the `## Stat-name mapping`
  table + a "Phase 37 extension — 1119 → 1121" block; the reject-text line updated
  (MAGLEV → the FIFTH accepted policy; supported-list `…, RING_HASH, MAGLEV`;
  CLUSTER_PROVIDED the lone departure); the deferred-LB-family list updated (maglev
  retired; **4** candidates remain). Surface count **1119 → 1121** at every site
  denoting the current total (the two historical 36.1/36.2 "1119" records left
  intact).
- **DECISIONS.md** — the full **ADR-0238** entry (the maglev policy): §Context
  promoted verbatim from SPEC §13 (PROPOSED → ACCEPTED) + §Decision (the `isPrime`
  helper, the `maglevLB` build/`Pick`, the manager accept + cap/primality gate +
  retarget, the 2 gauges, the `0063` proof) + §Consequences (the seam's FIRST
  hash-policy reuse; the SECOND non-zero LB-stat delta → 1121; cross-side host
  identity infeasible; ADR-0024/0235/0237 UNAMENDED; the family stays OPEN with 4
  candidates). Tail advances **ADR-0237 → ADR-0238**; next-free **ADR-0239**.
  Modeled on ADR-0236 (the ring_hash policy entry).
- **STATE.md** — active-phase → `phase 37 (load-balancer-maglev) IMPL done`;
  lifecycle-state → the 37-done routing (next: the next LB candidate / ROADMAP
  family); counts fixtures **64 → 65**, stat surface **1119 → 1121**, fuzzers
  **42**, BackendKind tail **33**, DECISIONS tail **ADR-0238** (next-free ADR-0239).
- **ROADMAP.md** — row 37 `in-progress → done` + completion date **2026-06-13** (a
  flat family row — NO parent rollup per ADR-0106); the Load-balancing family stays
  OPEN (4 candidates remain).
- **PROGRESS.md** (this file) — Task-10 row flipped; the stale ADR-0052/ADR-0238
  parenthetical RECONCILED (the completion ADR is ADR-0238; ADR-0052 is the process
  being followed, not an entry to create).

`go build ./...` re-run GREEN (docs-only sanity). **Task 10 COMPLETE.**
