# Phase 36.1 Implementation Plan — `ring_hash` load balancer, leg 1 (seam extension + `ringHashLB` + tcp_proxy `source_ip` plane)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.LbPolicy RING_HASH` (`envoy.config.cluster.v3`, enum value 2) — the project's FOURTH LB policy and its FIRST consistent-hashing policy — as a Ketama consistent-hash ring (`ringHashLB`) built once from the endpoint set, picking the first ring point `>= hashKey` (binary search, wrap), keyed by a request-derived `uint64` carried through `context.Context`. **36.1 delivers a COMPLETE end-to-end consistent-hash LB on the tcp plane** (the seam extension + `ringHashLB` + the manager case + the tcp_proxy `source_ip` producer); the HTTP route `hash_policy` producer is 36.2 (its own PLAN→IMPL, per the CONSUMED split — SPEC §3.0).

**Architecture:** The ADR-0232 LB acquire/release seam is EXTENDED on its PICK-INPUT half (ADR-0235): the unexported `loadBalancer.Pick` widens to `Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)`; the three incumbent policies (roundRobin/leastRequest/randomLB) add two IGNORED params (behavior-neutral → all 62 fixtures byte-identical); `ringHashLB` consumes them. The hash key rides `ctx` via an unexported `hashKeyCtxKey struct{}` + an EXPORTED additive `cluster.WithHashKey(ctx, key)` + an unexported `hashKeyFrom(ctx) (uint64, bool)` (all in `cluster.go`); `Dial`/`AcquireH1` extract+thread it in 2 changed lines each (exported signatures byte-stable); `PickEndpoint()` passes `(0, false)`. A new `ringHashLB` (`ringhash.go`) holds the sorted Ketama ring; a pure-Go `xxHash64` (seed 0) + `murmurHash2` (`hash.go`) provide the digests. `Manager.buildCluster` gains a `case Cluster_RING_HASH` parsing `RingHashLbConfig` through a hand-rolled two-layer gate; `registerClusterMetrics` registers 3 mirrored `ring_hash_lb.*` gauges on the RING_HASH path only. tcp_proxy gains a `source_ip` `hash_policy` producer.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `Cluster.LbPolicy RING_HASH` + `Cluster.RingHashLbConfig` + `type.v3.HashPolicy` all in the pinned module; **ZERO new go.mod dep**, `xxHash64`/`murmurHash2` hand-rolled — AMEND-RH1/RH7); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227). The differential proof is the AFFINITY+SPREAD arm: `0061-lb-ring-hash` — a SUBJECT-SIDE multi-source-IP affinity (every per-backend count a multiple of the per-IP burst) + spread (`>= 2` distinct backends) `DistributionAsserter` + a cross-side `StatsAsserter` (the 3 gauges cross-equal + cx/membership/quiesced).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/36-load-balancer-ring-hash/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-RH1..RH8; §3.0 the CONSUMED split; §3.1 the `ringHashLB` design (ring build + lookup code); §3.2 the seam extension (interface + ctx-carry code); §3.3 the tcp `source_ip` plane; §3.4 the manager gate; §5 the proto roster; §6 the reject matrix; §7 the +3 gauges; §8.1 the `0061` fixture design; §10 the 36.1 task spine; §11 the D-RH1..RH8 empirical pins; §12 the D-S36 questions; §13 the ADR-0235 + ADR-0236 DRAFTS.
- **BRAINSTORM:** `docs/envoy-go/phases/36-load-balancer-ring-hash/BRAINSTORM.md` — the charter (Q0/Q1/Q-seam/Q-split).
- **As-built seam + REUSE sites** (re-pin at Task 1 — line numbers shift):
  - `internal/cluster/loadbalancer.go:15-17` (the `loadBalancer` interface `Pick() (Endpoint, func(), error)` — WIDENED at Task 3) + `:21` (`noopRelease` — REUSED by ring_hash) + `:34` (`roundRobin.Pick` — gains 2 ignored params).
  - `internal/cluster/cluster.go:180-187` (`PickEndpoint()` → `Pick(0, false)`) + `:210-243` (`Dial(ctx)`; the `c.lb.Pick()` at `:217`) + `:262-324` (`AcquireH1(ctx)`; the `c.lb.Pick()` at `:270`). The `WithHashKey`/`hashKeyFrom`/`hashKeyCtxKey` helpers LAND in this file. `context` already imported.
  - `internal/cluster/leastrequest.go:63` (`newPCGRNG` — REUSED verbatim for the no-hash fallback; the shared seed-error message `"cluster: least_request: seed rng"`) + `:39`/`:49` (`newLeastRequest`/`newLeastRequestWithRNG`) + `:34` (`leastRequest.Pick` — gains 2 ignored params).
  - `internal/cluster/random.go:37` (`randomLB.Pick` — gains 2 ignored params) + `random_test.go` (`seqRNG`/`eps` helpers — REUSED by `ringhash_test.go`).
  - `internal/cluster/manager.go:99-110` (`registerClusterMetrics` — runs AFTER `buildCluster` per `:79`→`:86`; gains the conditional 3-gauge registration) + `:235` (the `lb_policy` switch — gains `case Cluster_RING_HASH`) + `:257` (the reject text — extends `…, RANDOM` → `…, RANDOM, RING_HASH`) + `:295` (`parseLeastRequestLbConfig` — the gate precedent `parseRingHashLbConfig` mirrors).
  - `internal/cluster/manager_test.go:320-329` (`TestManager_Error_UnsupportedLBPolicy` — DOUBLY-hit: trigger `Cluster_RING_HASH`:322 → `Cluster_MAGLEV` + substring:327).
  - `internal/filter/tcpproxy/filter.go:27-33` (the `Filter` struct — gains a parsed-hash-policy field) + `:49-73` (`NewFilter` — parses `msg.GetHashPolicy()`) + `:101-127` (`Handle`; the `eff.Dial(ctx)` at `:127` — gains the `WithHashKey` stuffing).
- **Differential harness:** `test/differential/runner_test.go` (the per-backend `accepts` atomic snapshot → `AssertDistribution(refCounts, subjCounts)` at `:1104`; the 3-snapshot discipline `:1037`/`:1065`/`:1069`/`:1091`; `acceptEchoCounting` `:1331` = PURE byte-echo, NO backend identifier) + `test/differential/fixture/fixture.go:15-76` (the `Driver` / `DistributionAsserter` / `StatsAsserter` interfaces; `TCPEcho BackendKind = 0`) + `test/fixtures/0060-lb-random/driver/driver.go` (the closest fixture template — band `AssertDistribution` + `StatsAsserter` + `scrapeStats`; as-built band floor 6 / ceiling 40, NOT the SPEC's 12/32).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` — subagent-driven execution.
- `feedback_git_worktrees` — this PLAN was authored in worktree `.worktrees/phase-36.1-plan`; the IMPL runs in its own worktree.
- `feedback_subagents_no_push` — **subagents commit LOCAL-ONLY**; the controller squash-merges + pushes at stage-close.
- `feedback_pertask_gofmt_lint` — **every task** runs `gofmt -l` + `golangci-lint run` on the touched packages (not just `go vet`).
- `feedback_subagent_worktree_path_targeting` — all paths below are repo-root-relative; the IMPL worktree is the canonical checkout; the controller verifies the main checkout stays clean. PROGRESS.md is at the pinned canonical path `docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md`.
- `reference_differential_break_protocol_count1` — every new differential assertion is proven live by a deliberate-break with `-count=1` (go test caching serves a stale PASS otherwise).
- `reference_differential_asserter_dispatch` — the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses `DistributionAsserter` (driver-side, runs on both paths).
- `reference_differential_run_selector` — targeted runs use `-run 'TestDifferential/0061'`, NEVER `-run '0061'` (which matches zero subtests → vacuous green).
- `reference_differential_band_sigma_margin` — governs RNG-distributed σ-bands; the ring-hash affinity leg is DETERMINISTIC/EXACT (no σ-band), so its breaks are sharp; only the coarse spread `>= 2` count is a loose threshold (no σ-margin needed).
- `reference_differential_hash_key_cross_side_infeasible` — ring_hash cross-side host identity + source_ip spread are INFEASIBLE (Docker NAT + per-side endpoint addrs); the proof is SUBJECT-SIDE affinity (this fixture's design).
- `reference_docker_probe_bridge_network` — the `0061` differential needs Docker + the contrib reference image on a bridge network; the controller runs that gate where Docker is present.
- ADR-0232 (the LB seam — its §Consequences PICK-INPUT-half anticipation this phase CONSUMMATES), ADR-0024 (per-cluster LB counter scope — UNAMENDED), ADR-0045 (the split-gate — FINAL re-check below), ADR-0044 (ADR bodies at the IMPL, in-place), ADR-0052 (the atomic-landing six-gate), ADR-0080 (byte-stable reject text), ADR-0106 (flat family row — NO parent rollup), ADR-0060 (histograms deferred; the 3 ring_hash_lb GAUGES are mirrored), ADR-0219 (the ctx-carry precedent for `WithHashKey`), ADR-0063 (the cluster-gauge registration pattern), ADR-0227 (the contrib reference image).

## D-question resolutions (SPEC §12)

- **D-S36-1 (file placement):** RESOLVED as anticipated. `ringHashLB` + the ring build + lookup → NEW `internal/cluster/ringhash.go`; `xxHash64` + `murmurHash2` → NEW `internal/cluster/hash.go`; the seam widening → `loadbalancer.go` (interface + the 3 incumbent `Pick` signatures) + `cluster.go` (the ctx-carry helpers + the `Dial`/`AcquireH1`/`PickEndpoint` threading); the manager `case` + `parseRingHashLbConfig` + the reject text + the conditional gauge registration → `manager.go`; the doubly-hit retarget → `manager_test.go`. (The `random.go`/`leastrequest.go` sibling-file precedent.)
- **D-S36-2 (no-hash-fallback RNG message):** RESOLVED — `newRingHash` calls `newPCGRNG()` **directly** and **accepts the shared seed-error message** (`"cluster: least_request: seed rng"`); NO wrapper. Same rationale as D-S35-2 (random): the path is reachable only on a `crypto/rand` read failure (effectively unreachable on Linux `getrandom` → boot-fail either way); a ring-flavored wrapper would double-wrap or force editing `newPCGRNG`. The slightly-misattributed `"least_request"` prefix on an effectively-unreachable boot-fail path is a recorded cosmetic note (PROGRESS.md + a BEHAVIOR_CONTRACT departure record). YAGNI.
- **D-S36-3 (hash impl shape + MURMUR_HASH_2 timing):** RESOLVED — implement BOTH `xxHash64` (the XXH64 reference port, seed 0) AND `murmurHash2(key, 0xc70f6907)` at 36.1 (SPEC §2: "both hash functions are IMPLEMENTED — parse + ring build"); the parse arm accepts both, the ring build dispatches on `hash_function`, the differential fixtures use the `XX_HASH` default (the cross-side-reproducible path), `MURMUR_HASH_2` is unit-level. `xxHash64` is verified against published XXH64 vectors AND the live reference's observed key→host mapping (D-RH4b).
- **D-S36-4 (multi-source-IP driver + affinity attribution):** RESOLVED — **NO new driver primitive, NO new BackendKind.** The harness (runner_test.go) provides per-backend AGGREGATE accept counts to `AssertDistribution(refCounts, subjCounts []uint64)`; TCPEcho carries NO backend identifier (pure byte-echo), so per-source-IP→backend attribution is NOT observable through the proxy. **BUT the affinity invariant manifests in the aggregate counts:** when the driver binds exactly `burstPerIP` (16) connections to each of 4 source IPs, the consistent-hash invariant (one source IP → one key → one ring point → one backend) forces **every subject per-backend count to be a multiple of `burstPerIP`** (each source IP contributes all 16 or 0 to each backend). The SUBJECT affinity assertion is therefore `count[i] % 16 == 0 for all i` (an EXACT equality, not a band); the SPREAD assertion is `>= 2` distinct nonzero backends; conservation is `sum == 64`. The SCATTER break (Pick ignores the key → random draw) yields counts ~21/21/22 (not multiples of 16) → the affinity leg FAILS; the COLLAPSE-ring break (all picks → endpoints[0]) → `< 2` nonzero → the spread leg FAILS. The driver binds via `net.Dialer{LocalAddr: &net.TCPAddr{IP: 127.0.0.x}}` (proven feasible on host loopback §11.8). The reference (Docker-NAT'd to one source IP → all 64 on ONE backend) is asserted on `sum == 64` only (single-key pin — AMEND-RH8); its real proof is cross-side byte-equivalence + the cross-side `StatsAsserter`. **Coverage boundary (documented):** the mod-16 invariant is necessary-and-overwhelmingly-discriminating against the specified random-scatter break, not a tight proof against an adversarial even-split; the realistic break is random scatter, which it catches flake-free (random multinomial(64, 1/3) landing on an all-multiples-of-16 tuple is < 1%).
- **D-S36-5 (ring-build formula):** RESOLVED — implement the equal-weight `entriesPerHost = ceil(minRingSize / N)` path (the running-sum `scale` formula collapses to this under equal weights; the weighted running-sum carry is DEFERRED per SPEC §2). Task 4 CONFIRMS the build matches the live reference for the default 3-host case: `size = 3 * ceil(1024/3) = 3 * 342 = 1026`, `min = max = 342`.
- **D-S36-6 (gauge registration scope):** RESOLVED — register the 3 `ring_hash_lb.*` gauges ONLY on the RING_HASH path (matching the reference, which emits them only under RING_HASH). Mechanism: `registerClusterMetrics` (which runs AFTER `buildCluster`, so `c.lb` is set) type-asserts `if rh, ok := c.lb.(*ringHashLB); ok { … }` and registers + `Set`s the 3 gauges from `rh.size`/`rh.minPerHost`/`rh.maxPerHost`. They are STATIC (immutable ring) — registered + Set once, NO held `*stats.Gauge` field on `Cluster` needed (unlike `upstreamCxActive`, which needs a pointer for Inc/Dec).
- **D-S36-7 (36.2):** OUT OF SCOPE for this PLAN (the HTTP route `hash_policy` plane lands at 36.2).
- **ADR-0045 split-gate FINAL re-check (per sub-phase):** **NO FURTHER SPLIT for 36.1.** This PLAN decomposes into **10 tasks** (≤ ~25) over **~365 production LoC** (seam ~55 + `ringHashLB` ~205 + hash funcs ~45 + tcp plane ~60; ≤ ~1500) — both ADR-0045 axes hold with a wide margin. (36.2 — the ~140-LoC HTTP plane — re-checks the gate at its own PLAN; anticipated NO split.)

### Decomposition note (10 tasks vs the SPEC's indicative 9)

SPEC §10 bundles the full differential re-verify and the completion bundle into a single Task 9. This PLAN splits them — **Task 9 (verification gate: run the six gates)** and **Task 10 (completion bundle: BEHAVIOR_CONTRACT delta + the ADR-0235 + ADR-0236 bodies + STATE/ROADMAP advance)** — because they are different kinds of work (running gates vs. authoring docs, ADR-0052 atomic landing — the phase-35 precedent). All other SPEC §10 tasks map 1:1.

| SPEC §10 (36.1) task | This plan |
|---|---|
| 1 baselines/anchors gate + PROGRESS | Task 1 |
| 2 pure-Go `xxHash64` (+ `murmurHash2`) | Task 2 |
| 3 seam extension | Task 3 |
| 4 `ringHashLB` (ring build + lookup + gauges) | Task 4 |
| 5 manager acceptance + gate + retarget + register gauges | Task 5 |
| 6 tcp_proxy `source_ip` plane | Task 6 |
| 7 `0061` fixture | Task 7 |
| 8 deliberate-break liveness + ≥20-run flake check | Task 8 |
| 9 full differential re-verify | **Task 9** |
| (9, cont.) completion bundle | **Task 10 (split out)** |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/hash.go` | **CREATE** (Task 2) | Pure-Go `xxHash64(b []byte) uint64` (seed 0; XXH64 port) + `murmurHash2(b []byte, seed uint64) uint64` (the `MURMUR_HASH_2` arm, seed `0xc70f6907`). |
| `internal/cluster/hash_test.go` | **CREATE** (Task 2) | `xxHash64` vs published XXH64 vectors + the D-RH4b key→host cross-check; `murmurHash2` vs a pinned vector. |
| `internal/cluster/loadbalancer.go` | MODIFY (Task 3) | Widen the `loadBalancer.Pick` interface; add 2 ignored params to `roundRobin.Pick`. |
| `internal/cluster/leastrequest.go` | MODIFY (Task 3) | Add 2 ignored params to `leastRequest.Pick`. |
| `internal/cluster/random.go` | MODIFY (Task 3) | Add 2 ignored params to `randomLB.Pick`. |
| `internal/cluster/cluster.go` | MODIFY (Task 3) | The `hashKeyCtxKey`/`WithHashKey`/`hashKeyFrom` ctx-carry helpers; thread the key through `Dial`/`AcquireH1` (2 lines each); `PickEndpoint` → `Pick(0, false)`. |
| `internal/cluster/ringhash.go` | **CREATE** (Task 4) | The `ringHashLB` type + `ringEntry` + `newRingHash`/`newRingHashWithRNG` (ring build) + `Pick` (binary-search lookup + no-hash fallback) + the 3 gauge values. |
| `internal/cluster/ringhash_test.go` | **CREATE** (Task 4) | Deterministic-key ring tests (same key → same endpoint; spread; wrap; no-hash fallback; empty-set parity; the `size/min/max == 1026/342/342` default-3-host build check; a random-keys property test). |
| `internal/cluster/manager.go` | MODIFY (Task 5) | The `case Cluster_RING_HASH` + `parseRingHashLbConfig` (two-layer gate) + the NEW reject text; the conditional 3-gauge registration in `registerClusterMetrics`. |
| `internal/cluster/manager_test.go` | MODIFY (Task 5) | RING_HASH accept (defaults + non-default valid) + the 3 reject arms + mismatched-oneof silent-ignore + the gauge-registration assertion; the doubly-hit retarget of `TestManager_Error_UnsupportedLBPolicy`. |
| `internal/filter/tcpproxy/filter.go` | MODIFY (Task 6) | The `Filter` hash-policy field; `NewFilter` parses `TcpProxy.hash_policy` (source_ip accept; unsupported specifiers DEPARTURE-reject); `Handle` computes `xxHash64(ipOnly(RemoteAddr))` + `cluster.WithHashKey` before `eff.Dial(ctx)`. |
| `internal/filter/tcpproxy/filter_test.go` | MODIFY (Task 6) | hash_policy parse-accept/reject + the source_ip→deterministic-key path + no-hash-policy→byte-stable. |
| `test/fixtures/0061-lb-ring-hash/` | **CREATE** (Task 7) | `driver/driver.go`, `driver/driver_test.go`, `README.md`, `expectations.yaml` (mirroring the `0060` dir layout). |
| `docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 10) | The `ring_hash` subsection + the reject-text line update + the deferred-family list + the departure/coverage records + the stat-surface doc count 1116 → 1119. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 10) | The full ADR-0235 (seam extension) + ADR-0236 (ring_hash policy) entries (§Context + §Decision + §Consequences; ADR-0044 in-place). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 10) | Active-phase + counts advance (fixtures 62 → 63; stat surface 1116 → 1119; DECISIONS tail → ADR-0236); ROADMAP row 36.1 leg done. |

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code (the established first-task discipline), re-pin the as-built line anchors, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

Run (from repo root):
```bash
ls -d test/fixtures/[0-9]* | wc -l        # expect 62
ls -d test/fixtures/[0-9]* | tail -1      # expect test/fixtures/0060-lb-random
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect TCPThriftResponder BackendKind = 33
grep -c "^## ADR-0" docs/envoy-go/DECISIONS.md   # informational; confirm tail ADR-0234, next-free ADR-0235
grep -n "1116" docs/envoy-go/BEHAVIOR_CONTRACT.md  # the stat-surface doc count (no programmatic golden — D-S35 note)
go build ./... && echo BUILD_OK
```
Expected: fixtures **62** (tail `0060-lb-random`), BackendKind tail **33**, fuzzers **42** (use the repo's established fuzzer-count recipe — do NOT hand-roll a `grep "func Fuzz"` count, which over-counts seed helpers), stat surface **1116** (a DOC count in BEHAVIOR_CONTRACT.md, NOT a programmatic test — the phase-35 PROGRESS note), DECISIONS tail **ADR-0234** (the ADR-0235 + ADR-0236 §Context are DRAFTS in SPEC §13 — NOT yet in DECISIONS.md).

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these line anchors still hold (they shift if the tip moved); record actual line numbers in PROGRESS.md:
```bash
grep -n "Pick() (Endpoint, func(), error)" internal/cluster/loadbalancer.go   # the interface to WIDEN + roundRobin.Pick
grep -n "noopRelease = func" internal/cluster/loadbalancer.go                 # the shared release ring_hash returns
grep -n "func (lr \*leastRequest) Pick\|func (r \*randomLB) Pick" internal/cluster/leastrequest.go internal/cluster/random.go  # the 2 incumbent Picks to widen
grep -n "func newPCGRNG" internal/cluster/leastrequest.go                     # REUSED for the no-hash fallback
grep -n "func (c \*Cluster) PickEndpoint\|c.lb.Pick()" internal/cluster/cluster.go   # PickEndpoint + the Dial/AcquireH1 Pick sites to thread
grep -n "func registerClusterMetrics" internal/cluster/manager.go            # the conditional gauge registration site
grep -n "switch c.GetLbPolicy()" internal/cluster/manager.go                 # the case to extend
grep -n "unsupported lb_policy" internal/cluster/manager.go                  # the reject text (ONE production string)
grep -n "func parseLeastRequestLbConfig" internal/cluster/manager.go         # the parseRingHashLbConfig precedent
grep -n "func TestManager_Error_UnsupportedLBPolicy" internal/cluster/manager_test.go  # the doubly-hit retarget
grep -n "func (f \*Filter) Handle\|eff.Dial(ctx)\|func NewFilter\|type Filter struct" internal/filter/tcpproxy/filter.go  # the tcp plane sites
grep -n "func seqRNG\|func eps" internal/cluster/*_test.go                    # REUSED by ringhash_test.go (do NOT redeclare)
```

- [ ] **Step 3: Confirm the reject-text blast radius is exactly THREE sites (AMEND-RH5)**

```bash
grep -rln "unsupported lb_policy" internal/ cmd/                          # expect ONLY internal/cluster/manager.go
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM" test/                      # expect EMPTY (no fixture pins the text → no boot-reject dir)
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM" docs/envoy-go/BEHAVIOR_CONTRACT.md  # expect a hit (~:899)
```
Expected: production string ONLY in `manager.go`; ZERO fixture hits (confirms NO boot-reject dir); the doc hit in BEHAVIOR_CONTRACT.md.

- [ ] **Step 4: Survey the seam consumers (the "7 consumers, 2 churn" check — AMEND-RH7)**

```bash
grep -rln "\.lb\.Pick(\|\.Dial(ctx\|\.AcquireH1(ctx\|\.PickEndpoint()" internal/   # confirm Dial/AcquireH1/PickEndpoint are the only Pick funnels
grep -rln "cluster.WithHashKey\|hashKeyFrom" internal/                             # expect EMPTY pre-impl (the new symbols)
```
Record: only `cluster.go` calls `c.lb.Pick()`; the producers that churn are tcp_proxy (Task 6, 36.1) + the HTTP router (36.2, NOT this PLAN); all other consumers (thriftproxy/redisproxy/grpcclient/httpclient/dial_h2) thread `ctx` unchanged onto the no-hash path (their `Dial`/`AcquireH1`/`PickEndpoint` calls are unaffected by the byte-stable exported signatures).

- [ ] **Step 5: Create PROGRESS.md**

Create `docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md` with: the 10-task table (status column), the count anchors from Step 1, the as-built line anchors from Step 2, the three-site blast radius from Step 3, the seam-consumer survey from Step 4, the D-S36-1..7 resolutions, and the ADR-0045 re-check verdict (NO FURTHER SPLIT for 36.1; ~365 LoC / 10 tasks). Mark Task 1 complete.

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md
git commit -m "phase 36.1 Task 1: baselines gate + PROGRESS.md (fixtures 62 / fuzzers 42 / stat surface 1116 / BackendKind 33 / DECISIONS tail ADR-0234 confirmed; reject-text blast radius 3 sites; seam-consumer survey 2-churn)"
```

---

## Task 2: The pure-Go hash functions — `xxHash64` (seed 0) + `murmurHash2` (`hash.go`)

**Goal:** Implement a faithful pure-Go `xxHash64` (the XXH64 reference, seed 0) and `murmurHash2(key, 0xc70f6907)` — the digests the Ketama ring and the source_ip key compute need (AMEND-RH2/RH7). `cespare/xxhash` is ABSENT from go.mod and a new dep for one call site is unwarranted (D-RH1); the digest MUST match the reference byte-for-byte for the ring to be a faithful reproduction.

**Files:**
- Create: `internal/cluster/hash.go`
- Create: `internal/cluster/hash_test.go`

- [ ] **Step 1: Pin the published test vectors**

`xxHash64` (XXH64, seed 0) is a standard algorithm with well-known vectors. The empty-input vector is canonical and load-bearing:
```
xxHash64("")  == 0xEF46DB3751D8E999
```
Generate 3–4 additional vectors from a TRUSTED oracle at IMPL time (do NOT invent constants): run a throwaway Go program importing `github.com/cespare/xxhash/v2` (in a scratch module OUTSIDE this repo's go.mod — it must NOT enter our go.mod) OR a `python3 -c 'import xxhash; print(hex(xxhash.xxh64(b"...", seed=0).intdigest()))'`, for inputs including a representative ring key (e.g. `"127.0.0.1:9001_0"`) and the bare-IP source_ip key (e.g. `"127.0.0.2"`). Record the oracle + the vectors in `hash_test.go` comments and PROGRESS.md. ADDITIONALLY pin the D-RH4b cross-check: a key→host mapping observed from the live reference (SPEC §11.4) must reproduce under our `xxHash64` + ring (deferred to Task 4's ring test, but record the raw key digests here).

For `murmurHash2`, pin one vector from the same oracle (a Go/C `MurmurHash64A` with seed `0xc70f6907` over a representative key); `MURMUR_HASH_2` is unit-level only (no fixture path — SPEC §2).

- [ ] **Step 2: Write the failing tests** (`hash_test.go`)

```go
package cluster

import "testing"

func TestXXHash64_PublishedVectors(t *testing.T) {
	// XXH64 seed 0. The empty-input vector is canonical; the rest are pinned from a
	// trusted oracle at IMPL (Task 2 Step 1) — DO NOT invent. Replace the ??? values.
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0xEF46DB3751D8E999},
		{"127.0.0.1:9001_0", 0x0}, // ??? pin from oracle
		{"127.0.0.2", 0x0},        // ??? pin from oracle (the bare-IP source_ip key)
		// + 1-2 more (e.g. "abc", a longer >32-byte string to exercise the stripe loop)
	}
	for _, c := range cases {
		if got := xxHash64([]byte(c.in)); got != c.want {
			t.Errorf("xxHash64(%q) = %#016x, want %#016x", c.in, got, c.want)
		}
	}
}

func TestXXHash64_LongInputStripeLoop(t *testing.T) {
	// A >32-byte input exercises the 4-lane accumulator stripe path (the part most
	// likely to be ported wrong); pin its digest from the oracle.
	in := "the quick brown fox jumps over the lazy dog 0123456789" // 54 bytes
	const want = 0x0 // ??? pin from oracle
	if got := xxHash64([]byte(in)); got != want {
		t.Errorf("xxHash64(long) = %#016x, want %#016x", got, want)
	}
}

func TestMurmurHash2_PinnedVector(t *testing.T) {
	const seed = 0xc70f6907 // Envoy's MURMUR_HASH_2 seed (AMEND-RH2)
	const want = 0x0        // ??? pin from oracle over "127.0.0.1:9001_0"
	if got := murmurHash2([]byte("127.0.0.1:9001_0"), seed); got != want {
		t.Errorf("murmurHash2 = %#016x, want %#016x", got, want)
	}
}
```

- [ ] **Step 3: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestXXHash64|TestMurmurHash2" ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`xxHash64`/`murmurHash2` undefined).

- [ ] **Step 4: Implement `hash.go`** (the XXH64 reference port + MurmurHash64A)

Port the XXH64 reference algorithm (the 5 prime constants, the 4-lane stripe loop for ≥32-byte inputs, the merge/avalanche finalization, the remaining-bytes tail). Seed is fixed 0 (Envoy's `XX_HASH` default). Keep it a faithful, side-effect-free `func xxHash64(b []byte) uint64`. Implement `murmurHash2` as MurmurHash64A with the caller-supplied seed. NO new imports beyond `encoding/binary` (or hand-rolled little-endian reads). Document each function with the upstream source pin (`source/common/common/hash.cc`) and the byte-exactness requirement.

> The IMPL writes the actual algorithm. Reference the well-known XXH64 spec; the unit tests (Step 2) are the correctness gate — if the empty-input vector `0xEF46DB3751D8E999` and the oracle-pinned vectors pass, the port is faithful.

- [ ] **Step 5: Run to verify they pass**

```bash
cd internal/cluster && go test -run "TestXXHash64|TestMurmurHash2" ./... 2>&1 | tail
```
Expected: PASS (all vectors match).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/hash.go internal/cluster/hash_test.go
git commit -m "phase 36.1 Task 2: pure-Go xxHash64 (seed 0; XXH64 port) + murmurHash2 (seed 0xc70f6907) — verified vs published XXH64 vectors + oracle-pinned ring/source_ip keys; ZERO new go.mod dep (cespare/xxhash NOT imported — AMEND-RH7)"
```

---

## Task 3: The seam EXTENSION (the ADR-0232 PICK-INPUT half — ADR-0235)

**Goal:** Widen the unexported `loadBalancer.Pick` to `Pick(hashKey uint64, hasHash bool)`; add the 2 ignored params to the three incumbent policies (behavior-neutral — all 62 fixtures byte-identical); add the `hashKeyCtxKey`/`WithHashKey`/`hashKeyFrom` ctx-carry helpers; thread the key through `Dial`/`AcquireH1` (2 lines each) and `PickEndpoint` (passes `(0, false)`). The exported `Cluster` surface stays BYTE-STABLE (the OPTION-C discipline). NO ring_hash yet — this task is the seam alone, proven behavior-neutral.

**Files:**
- Modify: `internal/cluster/loadbalancer.go` (the interface + `roundRobin.Pick`)
- Modify: `internal/cluster/leastrequest.go` (`leastRequest.Pick`)
- Modify: `internal/cluster/random.go` (`randomLB.Pick`)
- Modify: `internal/cluster/cluster.go` (the ctx-carry helpers + the 3 threading sites)
- Modify: `internal/cluster/loadbalancer_test.go` (assert the 3 incumbent Picks are byte-unchanged through the new params; assert `WithHashKey`/`hashKeyFrom` round-trip)

- [ ] **Step 1: Write the failing tests** (`loadbalancer_test.go` or a new `seam_test.go` — match the package convention)

```go
func TestWithHashKey_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if _, ok := hashKeyFrom(ctx); ok {
		t.Fatal("bare ctx must report hasHash==false")
	}
	ctx = WithHashKey(ctx, 0xDEADBEEF)
	v, ok := hashKeyFrom(ctx)
	if !ok || v != 0xDEADBEEF {
		t.Errorf("hashKeyFrom = (%#x, %v), want (0xDEADBEEF, true)", v, ok)
	}
}

func TestIncumbentPolicies_IgnoreHashParams(t *testing.T) {
	// The three non-hash policies must produce IDENTICAL picks regardless of the
	// hashKey/hasHash args (behavior-neutrality — the 62-fixture byte-identity proof
	// in miniature). roundRobin is deterministic; assert the same 4-pick sequence
	// with hasHash=false and with an arbitrary key+hasHash=true.
	mk := func() loadBalancer { return &roundRobin{endpoints: eps(3)} }
	seqNoHash := pick4(t, mk(), 0, false)
	seqWithHash := pick4(t, mk(), 0x12345, true)
	if !equalHosts(seqNoHash, seqWithHash) {
		t.Errorf("roundRobin pick sequence changed with hash args: %v vs %v", seqNoHash, seqWithHash)
	}
	// (leastRequest/randomLB use rng — assert via a shared seqRNG that the index math
	// is unchanged; or assert the param is structurally ignored by construction.)
}
```
(`pick4`/`equalHosts` are small local helpers; `eps`/`seqRNG` are REUSED from the existing `*_test.go` — verify with the Task 1 Step 2 grep, do NOT redeclare.)

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestWithHashKey|TestIncumbentPolicies" ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`WithHashKey`/`hashKeyFrom` undefined; and once the interface widens, the test's `Pick(...)` calls won't compile against the old signature — write the tests against the NEW signature).

- [ ] **Step 3: Widen the interface + the 3 incumbent Picks** (SPEC §3.2)

`loadbalancer.go` — widen the interface (keep the doc comment from SPEC §3.2):
```go
type loadBalancer interface {
	// Pick selects an endpoint. hashKey carries a request-derived consistent-hash
	// key when hasHash is true (ring_hash); the non-hash policies ignore both args;
	// ring_hash with hasHash==false falls back to a random ring position. The release
	// func is the ADR-0232 RELEASE half (unchanged). ADR-0235 (the PICK-INPUT-half
	// extension; the hash key rides ctx, threaded in cluster.go).
	Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)
}
```
`roundRobin.Pick(_ uint64, _ bool)`, `leastRequest.Pick(_ uint64, _ bool)`, `randomLB.Pick(_ uint64, _ bool)` — add the two IGNORED params; the bodies are UNCHANGED (zero behavior change).

- [ ] **Step 4: Add the ctx-carry helpers + thread the 3 sites** (`cluster.go`; SPEC §3.2)

```go
type hashKeyCtxKey struct{}

// WithHashKey returns ctx carrying a request-derived upstream-selection hash for a
// ring_hash cluster to consume at Dial/AcquireH1. Exported (the producers live in
// other packages — tcpproxy, http/router). Non-ring_hash clusters ignore it. The
// ONE new exported symbol on the cluster surface — additive, not a signature change.
func WithHashKey(ctx context.Context, key uint64) context.Context {
	return context.WithValue(ctx, hashKeyCtxKey{}, key)
}

func hashKeyFrom(ctx context.Context) (uint64, bool) {
	v, ok := ctx.Value(hashKeyCtxKey{}).(uint64)
	return v, ok
}
```
In `Dial` (the `c.lb.Pick()` at ~:217) and `AcquireH1` (~:270), change the pick to:
```go
	hk, ok := hashKeyFrom(ctx)
	ep, release, err := c.lb.Pick(hk, ok)
```
In `PickEndpoint` (no ctx — :181): `c.lb.Pick(0, false)`. The exported signatures of `Dial`/`AcquireH1`/`PickEndpoint` are UNCHANGED.

> `WithHashKey` is the first new exported symbol; a SECOND additive exported symbol (`cluster.HashSourceIP`) lands at Task 6 (the tcp producer needs the unexported `xxHash64`/`ipOnly` through a cluster-package surface). The exported-surface delta for the 36.1 leg is therefore **2 symbols** (both additive; no existing signature changes) — the exit criteria and the ADR-0235 surface note (Task 10) capture both.

- [ ] **Step 5: Run to verify the seam compiles + the package is behavior-neutral**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20         # the FULL package incl. all incumbent-policy tests
go build ./... 2>&1 | tail                                    # the whole repo compiles (consumers thread ctx unchanged)
```
Expected: PASS — the incumbent-policy tests are unchanged in behavior; every consumer (`tcp_proxy`/`thriftproxy`/`redisproxy`/`grpcclient`/`httpclient`/`dial_h2`/the router) compiles against the byte-stable exported `Dial`/`AcquireH1`/`PickEndpoint`.

- [ ] **Step 6: Full differential smoke (the 62-fixture byte-identity is the real seam-neutrality proof)**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -20   # all 62 byte-exact (the seam widening is behavior-neutral)
```
Expected: ALL 62 PASS. (If Docker is unavailable, the controller runs this gate where Docker is present — `reference_docker_probe_bridge_network`. At minimum run `-short`/non-Docker subsets locally and defer the full run to the controller.)

- [ ] **Step 7: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/loadbalancer.go internal/cluster/leastrequest.go internal/cluster/random.go internal/cluster/cluster.go internal/cluster/*_test.go
git commit -m "phase 36.1 Task 3: seam EXTENSION (ADR-0235) — widen loadBalancer.Pick(hashKey,hasHash); 3 incumbent policies gain ignored params (behavior-neutral, 62 fixtures byte-identical); WithHashKey/hashKeyFrom ctx-carry; thread Dial/AcquireH1 (2 lines each) + PickEndpoint(0,false); exported Cluster surface byte-stable"
```

---

## Task 4: The `ringHashLB` Ketama consistent-hash policy (`ringhash.go`)

**Goal:** Implement `ringHashLB` mirroring Envoy v1.37.2's `RingHashLoadBalancer` EXACTLY (AMEND-RH2): a sorted ring of `(hash, endpoint)` points built once (`ceil(minRingSize/N)` entries per endpoint for the equal-weight MVP, each keyed `xxHash64("addr:port_i")`); `Pick` binary-searches the first ring point `>= hashKey` (wrap), with `hasHash==false` → a uniform random ring position. REUSE the ADR-0232 RELEASE half unchanged (`noopRelease`). Compute the 3 gauge values at build.

**Files:**
- Create: `internal/cluster/ringhash.go`
- Create: `internal/cluster/ringhash_test.go`

- [ ] **Step 1: Write the failing tests** (`ringhash_test.go`)

```go
package cluster

import "testing"

func TestRingHash_SameKeySameEndpoint(t *testing.T) {
	// The consistent-hash invariant: the SAME key always picks the SAME endpoint.
	rh, err := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	if err != nil {
		t.Fatal(err)
	}
	ep1, _, _ := rh.Pick(0xABCDEF, true)
	ep2, _, _ := rh.Pick(0xABCDEF, true)
	if ep1 != ep2 {
		t.Errorf("same key picked different endpoints: %v vs %v", ep1, ep2)
	}
}

func TestRingHash_DistinctKeysSpread(t *testing.T) {
	// Enough distinct keys cover >= 2 endpoints (the ring is not degenerate).
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	seen := map[Endpoint]bool{}
	for k := uint64(0); k < 200; k++ {
		ep, _, _ := rh.Pick(k*2654435761, true) // spread the keys
		seen[ep] = true
	}
	if len(seen) < 2 {
		t.Errorf("200 distinct keys covered only %d endpoints (degenerate ring?)", len(seen))
	}
}

func TestRingHash_WrapAround(t *testing.T) {
	// A key larger than the max ring hash wraps to ring[0]. Build a tiny ring and
	// pick with key=MaxUint64 → must equal the ring[0] endpoint.
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 12, maxRingSize: 8388608, hashFunc: hashXX})
	epWrap, _, _ := rh.Pick(^uint64(0), true)
	epZero := rh.endpoints[rh.ring[0].ep]
	if epWrap != epZero {
		t.Errorf("wrap: key=MaxUint64 picked %v, want ring[0] endpoint %v", epWrap, epZero)
	}
}

func TestRingHash_NoHashFallbackUsesRNG(t *testing.T) {
	// hasHash==false draws a uniform ring position via the injected rng. With a
	// deterministic rng returning a known value, the pick is the ring point >= that.
	rh := newRingHashWithRNG(eps(3), ringHashCfg{minRingSize: 12, maxRingSize: 8388608, hashFunc: hashXX}, seqRNG(0))
	epA, _, errA := rh.Pick(0, false) // rng()=0 → ring[0] (first point >= 0)
	if errA != nil {
		t.Fatal(errA)
	}
	if epA != rh.endpoints[rh.ring[0].ep] {
		t.Errorf("no-hash fallback with rng()=0 picked %v, want ring[0]", epA)
	}
}

func TestRingHash_EmptySet(t *testing.T) {
	rh := newRingHashWithRNG(nil, ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX}, seqRNG(0))
	_, release, err := rh.Pick(123, true)
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestRingHash_DefaultBuildMatchesReference(t *testing.T) {
	// D-RH4b / D-S36-5: default minimum_ring_size=1024 over 3 equal hosts builds
	// size=3*ceil(1024/3)=3*342=1026, min=max=342 (the live reference values).
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	if rh.size != 1026 || rh.minPerHost != 342 || rh.maxPerHost != 342 {
		t.Errorf("gauges = {size:%d min:%d max:%d}, want {1026 342 342}", rh.size, rh.minPerHost, rh.maxPerHost)
	}
	if uint64(len(rh.ring)) != rh.size {
		t.Errorf("len(ring)=%d != size gauge %d", len(rh.ring), rh.size)
	}
	// sorted-ascending invariant
	for i := 1; i < len(rh.ring); i++ {
		if rh.ring[i].hash < rh.ring[i-1].hash {
			t.Fatalf("ring not sorted ascending at %d", i)
		}
	}
}

func TestRingHash_RandomKeysNeverPanicAlwaysValid(t *testing.T) {
	// The unit-level property test in lieu of a fuzzer (D-RH7: no untrusted wire
	// decode → no FuzzRingLookup). Random keys always yield a valid endpoint, never panic.
	rh, _ := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX})
	rng := seqRNG(1, 2, 3, 1<<63, ^uint64(0), 0)
	for i := 0; i < 1000; i++ {
		ep, rel, err := rh.Pick(rng(), true)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Port < 1000 { // eps(n) ports start at 1000
			t.Fatalf("pick %d: invalid endpoint %v", i, ep)
		}
		rel()
	}
}

func TestRingHash_MurmurArmBuilds(t *testing.T) {
	// MURMUR_HASH_2 is implemented (parse + ring build) though unit-level only (SPEC §2).
	rh, err := newRingHash(eps(3), ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashMurmur})
	if err != nil || rh.size != 1026 {
		t.Errorf("murmur arm: err=%v size=%d (want 1026)", err, rh.size)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestRingHash ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`ringHashLB`/`newRingHash`/`newRingHashWithRNG`/`ringHashCfg`/`hashXX`/`hashMurmur` undefined).

- [ ] **Step 3: Implement `ringhash.go`** (SPEC §3.1 verbatim)

Define `ringHashCfg` (the parsed config: `minRingSize, maxRingSize uint64; hashFunc hashFunc`), a `hashFunc` enum (`hashXX`/`hashMurmur`), `ringEntry{hash uint64; ep int}`, and `ringHashLB`:
```go
type ringHashLB struct {
	endpoints []Endpoint
	ring      []ringEntry   // sorted ascending by hash
	rng       func() uint64 // no-hash fallback (a uniform ring position); injectable for tests
	size, minPerHost, maxPerHost uint64 // gauges (AMEND-RH4): set once at build
}
```
- `newRingHash(endpoints, cfg)` → calls `newPCGRNG()` directly (D-S36-2 — accepts the shared seed-error message), then `newRingHashWithRNG`.
- `newRingHashWithRNG(endpoints, cfg, rng)` builds the ring: `entriesPerHost := ceil(cfg.minRingSize / N)` (equal-weight; D-S36-5); for each endpoint `j`, for `i` in `[0, entriesPerHost)`: `key := fmt.Sprintf("%s_%d", endpoints[j].Addr(), i)`; `h := xxHash64([]byte(key))` (or `murmurHash2([]byte(key), 0xc70f6907)` for `hashMurmur`); append `ringEntry{h, j}`. Then `sort.SliceStable(ring, …)` ascending by hash (stable — Envoy keeps insertion order on equal hashes, vanishingly rare). Set `size = len(ring)`, `minPerHost`/`maxPerHost` = entry-count extrema across endpoints (equal-weight → both `entriesPerHost`).
- `Pick(hashKey uint64, hasHash bool)` — SPEC §3.1 verbatim:
```go
func (rh *ringHashLB) Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error) {
	if len(rh.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if !hasHash {
		hashKey = rh.rng()
	}
	m := sort.Search(len(rh.ring), func(i int) bool { return rh.ring[i].hash >= hashKey })
	if m == len(rh.ring) {
		m = 0 // wrap
	}
	return rh.endpoints[rh.ring[m].ep], noopRelease, nil
}
```
Imports: `fmt`, `sort` (both stdlib; no new go.mod dep).

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestRingHash ./... 2>&1 | tail
cd internal/cluster && go test -run TestRingHash -race ./... 2>&1 | tail   # the ring is immutable post-build; Pick is read-only (rng is mutex-guarded)
```
Expected: PASS, no race. The `TestRingHash_DefaultBuildMatchesReference` green confirms the byte-faithful ring build (1026/342/342).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/ringhash.go internal/cluster/ringhash_test.go
git commit -m "phase 36.1 Task 4: ringHashLB Ketama ring (ADR-0236) — ceil(min/N) entries keyed xxHash64('addr:port_i') sorted; Pick binary-search first >= key + wrap; no-hash rng fallback; REUSES noopRelease; default 3-host build = size 1026/min,max 342 (matches live reference D-RH4b); murmur arm; random-keys property test in lieu of a fuzzer (D-RH7)"
```

---

## Task 5: Manager acceptance + the two-layer gate + the doubly-hit retarget + register the 3 gauges

**Goal:** `buildCluster` accepts `RING_HASH` via `parseRingHashLbConfig` (self-supplied 1024/8388608 defaults; the hand-rolled gate mirroring the PGV `<= 8388608` + the runtime `min > max` layers — AMEND-RH5) + `newRingHash`; the `default` reject text extends to `(…, RANDOM, RING_HASH)`; `TestManager_Error_UnsupportedLBPolicy` retargets RING_HASH → MAGLEV + re-pins the new substring; the mismatched-oneof under RING_HASH is silently ignored; `registerClusterMetrics` registers the 3 `ring_hash_lb.*` gauges on the RING_HASH path only.

**Files:**
- Modify: `internal/cluster/manager.go` (the `switch` at :235; the reject text at :257; `parseRingHashLbConfig`; the conditional gauge registration in `registerClusterMetrics`)
- Modify: `internal/cluster/manager_test.go` (accept + 3 reject arms + mismatched-oneof + gauge assertion; the doubly-hit retarget)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestManager_Accept_RingHash_Defaults(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RING_HASH // no ring_hash_lb_config → defaults {1024, 8388608, XX_HASH}
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("RING_HASH bare must be accepted: %v", err)
	}
	if _, ok := m.Get("c_rh"); !ok {
		t.Fatal("cluster c_rh not found")
	}
}

func TestManager_Accept_RingHash_NonDefaultValid(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(64), MaximumRingSize: wrapperspb.UInt64(128),
		HashFunction: clusterv3.Cluster_RingHashLbConfig_MURMUR_HASH_2,
	}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("valid non-default ring_hash_lb_config must be accepted: %v", err)
	}
}

func TestManager_Reject_RingHash_MinOverCap(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(9000000)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "minimum_ring_size: value must be less than or equal to 8388608") {
		t.Errorf("err = %v, want PGV min-over-cap reject", err)
	}
}

func TestManager_Reject_RingHash_MaxOverCap(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MaximumRingSize: wrapperspb.UInt64(9000000)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "maximum_ring_size: value must be less than or equal to 8388608") {
		t.Errorf("err = %v, want PGV max-over-cap reject", err)
	}
}

func TestManager_Reject_RingHash_MinOverMax(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(5), MaximumRingSize: wrapperspb.UInt64(2)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	// the RUNTIME-layer wording, distinct from PGV (AMEND-RH5)
	if err == nil || !strings.Contains(err.Error(), "ring hash: minimum_ring_size (5) > maximum_ring_size (2)") {
		t.Errorf("err = %v, want runtime min>max reject", err)
	}
}

func TestManager_Accept_RingHash_MismatchedOneof(t *testing.T) {
	// A stray least_request_lb_config under RING_HASH → silent-ignore (reference parity
	// §6.3; the manager reads GetRingHashLbConfig() only on the RING_HASH path).
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under RING_HASH must be silently accepted (defaults): %v", err)
	}
}

func TestManager_RingHash_RegistersGauges(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// the 3 ring_hash_lb.* gauges exist with the default-3-host values (use the repo's
	// registry-readback helper; mirror how an existing cluster-gauge test reads gauges).
	assertGauge(t, reg, "cluster.c_rh.ring_hash_lb.size", 1026)
	assertGauge(t, reg, "cluster.c_rh.ring_hash_lb.min_hashes_per_host", 342)
	assertGauge(t, reg, "cluster.c_rh.ring_hash_lb.max_hashes_per_host", 342)
}

func TestManager_NonRingHash_NoGauges(t *testing.T) {
	// the gauges are RING_HASH-only (D-S36-6) — a ROUND_ROBIN cluster registers none.
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 9001))
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	assertGaugeAbsent(t, reg, "cluster.c_rr.ring_hash_lb.size")
}
```
Then RETARGET `TestManager_Error_UnsupportedLBPolicy` (currently trigger `Cluster_RING_HASH`:322 — now ACCEPTED): change the trigger to `Cluster_MAGLEV` and extend the pinned substring to `"ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH"`:
```go
func TestManager_Error_UnsupportedLBPolicy(t *testing.T) {
	c := mkStaticCluster("c_x", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_MAGLEV // RING_HASH now accepted → retarget to a still-rejected policy (AMEND-RH5)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("MAGLEV must be rejected")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH") {
		t.Errorf("error %q missing new supported-set substring (…, RING_HASH)", err.Error())
	}
}
```

> **Both retarget edits are load-bearing** (the phase-35 doubly-hit lesson): the trigger change is required because `Cluster_RING_HASH` no longer errors; the substring extension is required so the assertion pins the NEW text (`"…, RANDOM"` is still a substring of `"…, RANDOM, RING_HASH"`, so without extending it the test would pass against the OLD text — vacuous w.r.t. the change). `assertGauge`/`assertGaugeAbsent` — use the repo's existing registry-readback test helper (grep `manager_test.go`/`cluster_test.go` for how `membership_total`/`upstream_cx_active` gauges are read in tests; reuse that, do not invent a new readback).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestManager_Accept_RingHash|TestManager_Reject_RingHash|TestManager_RingHash|TestManager_NonRingHash|TestManager_Error_UnsupportedLBPolicy" ./... 2>&1 | head -40
```
Expected: the accept/reject/gauge tests FAIL (current `default` rejects RING_HASH; no gauges); `TestManager_Error_UnsupportedLBPolicy` FAILS (RING_HASH now must be MAGLEV; old substring lacks `, RING_HASH`).

- [ ] **Step 3: Implement the manager change** (SPEC §3.4)

Add `parseRingHashLbConfig` (the `parseLeastRequestLbConfig` precedent; `GetRingHashLbConfig()` nil-safe → defaults). Define a small `ringHashCfg` result (reuse the Task-4 type). The gate (SPEC §3.4 / §6.2 verbatim wording):
```go
const (
	defaultMinRingSize = 1024
	defaultMaxRingSize = 8388608
	ringSizeCap        = 8388608
)

func parseRingHashLbConfig(c *clusterv3.Cluster, name string) (ringHashCfg, error) {
	cfg := ringHashCfg{minRingSize: defaultMinRingSize, maxRingSize: defaultMaxRingSize, hashFunc: hashXX}
	rhc := c.GetRingHashLbConfig()
	if rhc == nil {
		return cfg, nil // absent OR mismatched oneof → defaults (§6.3 silent-ignore parity)
	}
	if v := rhc.GetMinimumRingSize(); v != nil {
		if v.GetValue() > ringSizeCap {
			return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.minimum_ring_size: value must be less than or equal to 8388608", name)
		}
		cfg.minRingSize = v.GetValue()
	}
	if v := rhc.GetMaximumRingSize(); v != nil {
		if v.GetValue() > ringSizeCap {
			return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.maximum_ring_size: value must be less than or equal to 8388608", name)
		}
		cfg.maxRingSize = v.GetValue()
	}
	if cfg.minRingSize > cfg.maxRingSize {
		return ringHashCfg{}, fmt.Errorf("cluster: %q: ring hash: minimum_ring_size (%d) > maximum_ring_size (%d)", name, cfg.minRingSize, cfg.maxRingSize)
	}
	switch rhc.GetHashFunction() {
	case clusterv3.Cluster_RingHashLbConfig_XX_HASH:
		cfg.hashFunc = hashXX
	case clusterv3.Cluster_RingHashLbConfig_MURMUR_HASH_2:
		cfg.hashFunc = hashMurmur
	default:
		return ringHashCfg{}, fmt.Errorf("cluster: %q: ring_hash_lb_config.hash_function: unsupported value %v", name, rhc.GetHashFunction())
	}
	return cfg, nil
}
```
The switch case (between RANDOM and `default`):
```go
case clusterv3.Cluster_RING_HASH: // phase 36.1 (ADR-0236): Ketama consistent-hash ring
	cfg, err := parseRingHashLbConfig(c, name)
	if err != nil {
		return nil, err
	}
	lb, err := newRingHash(endpoints, cfg)
	if err != nil {
		return nil, err
	}
	cl.lb = lb
default:
	return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH)", name, c.GetLbPolicy())
```
In `registerClusterMetrics` (after the existing 8 metrics), register the 3 gauges on the RING_HASH path only (D-S36-6):
```go
	if rh, ok := c.lb.(*ringHashLB); ok {
		// AMEND-RH4: 3 mirrored static gauges, set once at register (the ring is immutable).
		// RING_HASH-only (reference parity); cross-side-exact (keyed on ring-config + host count).
		r.NewGauge(prefix + "ring_hash_lb.size").Set(int64(rh.size))
		r.NewGauge(prefix + "ring_hash_lb.min_hashes_per_host").Set(int64(rh.minPerHost))
		r.NewGauge(prefix + "ring_hash_lb.max_hashes_per_host").Set(int64(rh.maxPerHost))
	}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20   # the full package, incl. the unchanged incumbent tests
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 36.1 Task 5: manager accepts RING_HASH — parseRingHashLbConfig two-layer gate (PGV <=8388608 + runtime min>max, defaults 1024/8388608/XX_HASH self-supplied); NEW reject text (…, RANDOM, RING_HASH); doubly-hit retarget (RING_HASH→MAGLEV + substring); mismatched-oneof silent-ignore; register 3 ring_hash_lb.* gauges RING_HASH-only (AMEND-RH4/RH5, D-S36-6)"
```

---

## Task 6: The tcp_proxy `source_ip` hash plane

**Goal:** `tcp_proxy` parses `TcpProxy.hash_policy`; a `source_ip` specifier makes `Handle` compute `key = xxHash64(ipOnly(downstream.RemoteAddr()))` (the bare client IP, NO port — AMEND-RH3) and stuff `ctx = cluster.WithHashKey(ctx, key)` before `eff.Dial(ctx)`. No hash_policy → no key (the existing behavior byte-stable). Unsupported specifiers (`filter_state`) DEPARTURE-reject at parse (fail-fast — the thrift/least_request lineage).

**Files:**
- Modify: `internal/filter/tcpproxy/filter.go` (the `Filter` struct field; `NewFilter` parse; `Handle` compute)
- Modify: `internal/filter/tcpproxy/filter_test.go` (parse-accept/reject + the key path)

- [ ] **Step 1: Write the failing tests** (`filter_test.go`)

```go
func TestTcpProxy_HashPolicy_SourceIP_Parses(t *testing.T) {
	// A source_ip hash_policy parses and sets the filter to hash-on-source-ip.
	f := mkTcpProxyFilterWithHashPolicy(t, sourceIPHashPolicy()) // helper builds the TcpProxy any + a known cluster
	if !f.hashOnSourceIP {
		t.Error("source_ip hash_policy must set hashOnSourceIP")
	}
}

func TestTcpProxy_NoHashPolicy_ByteStable(t *testing.T) {
	f := mkTcpProxyFilter(t) // the existing no-hash-policy constructor
	if f.hashOnSourceIP {
		t.Error("no hash_policy → hashOnSourceIP must stay false (byte-stable behavior)")
	}
}

func TestTcpProxy_HashPolicy_FilterState_Rejected(t *testing.T) {
	// filter_state is DEPARTURE-rejected (deferred; fail-fast — the reference parse-accepts).
	_, err := NewFilter(filterStateHashPolicyAny(t), cm, nil)
	if err == nil || !strings.Contains(err.Error(), "hash_policy") {
		t.Errorf("filter_state hash_policy must be rejected: %v", err)
	}
}

func TestIpOnly(t *testing.T) {
	// ipOnly strips the port (AMEND-RH3): "127.0.0.2:57735" → "127.0.0.2".
	cases := map[string]string{"127.0.0.2:57735": "127.0.0.2", "[::1]:443": "::1"}
	for in, want := range cases {
		if got := ipOnly(in); got != want {
			t.Errorf("ipOnly(%q) = %q, want %q", in, got, want)
		}
	}
}
```
> The Handle-path key compute is hard to unit-test in isolation (it needs a live downstream conn with a known RemoteAddr); the END-TO-END proof is the `0061` fixture (Task 7). At unit level, prove (a) the parse sets `hashOnSourceIP`, (b) `ipOnly` strips the port correctly, (c) no-hash-policy is byte-stable. A focused Handle test MAY use a `net.Pipe`/loopback conn with a known RemoteAddr if the existing `filter_test.go` has that scaffolding — reuse it; otherwise defer the wiring proof to `0061`.

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/filter/tcpproxy/ -run "TestTcpProxy_HashPolicy|TestTcpProxy_NoHashPolicy|TestIpOnly" 2>&1 | head
```
Expected: COMPILE FAILURE (`hashOnSourceIP`/`ipOnly`/the helpers undefined).

- [ ] **Step 3: Implement the parse + the Handle compute** (SPEC §3.3)

In `filter.go`: add `hashOnSourceIP bool` to `Filter`. In `NewFilter`, after the cluster resolves, parse `msg.GetHashPolicy()`:
```go
	for _, hp := range msg.GetHashPolicy() {
		switch hp.GetPolicySpecifier().(type) {
		case *typev3.HashPolicy_SourceIp_:
			hashOnSourceIP = true
		default:
			return nil, fmt.Errorf("tcpproxy: hash_policy specifier %T is not supported (only source_ip)", hp.GetPolicySpecifier())
		}
	}
```
(`typev3` = `github.com/envoyproxy/go-control-plane/envoy/type/v3`; confirm the exact `SourceIp_` wrapper name from `type.v3.HashPolicy` — SPEC §5.3. Set `hashOnSourceIP` into the returned `Filter`.) Add the `ipOnly` helper:
```go
// ipOnly returns the bare IP from a "host:port" address, stripping the port
// (AMEND-RH3: Envoy hashes addressAsString() — the IP WITHOUT the port).
func ipOnly(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
```
In `Handle`, before `eff.Dial(ctx)` (the line at ~:127):
```go
	if f.hashOnSourceIP {
		ctx = cluster.WithHashKey(ctx, xxHash64([]byte(ipOnly(downstream.RemoteAddr().String()))))
	}
```
> NOTE: `xxHash64` lives in `internal/cluster` (unexported). tcp_proxy must NOT reach into the cluster package's unexported funcs. RESOLUTION: export a thin `cluster.HashSourceIP(ip string) uint64` helper (or `cluster.XXHash64([]byte) uint64`) at Task 2/3, OR compute the key via a small exported cluster surface. **Pick at IMPL:** the cleanest is an exported `cluster.HashSourceIP(addr string) uint64` that does `xxHash64([]byte(ipOnly(addr)))` — keeping the hash + ipOnly authority in the cluster package (the HTTP router reuses it at 36.2, D-S36-7). Then tcp_proxy calls `ctx = cluster.WithHashKey(ctx, cluster.HashSourceIP(downstream.RemoteAddr().String()))` and `ipOnly`/`xxHash64` stay unexported in cluster. Update Task 2/3's exported surface accordingly (this is the SECOND new exported symbol — additive — alongside `WithHashKey`; record both in the ADR-0235 surface note). The `TestIpOnly` test then lives in `cluster` against the (unexported) helper or against `HashSourceIP`.

- [ ] **Step 4: Run to verify they pass + the full package**

```bash
go test ./internal/filter/tcpproxy/ 2>&1 | tail
go test ./internal/cluster/ 2>&1 | tail   # if HashSourceIP/ipOnly moved here
go build ./... 2>&1 | tail
```
Expected: PASS; the whole repo compiles.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/filter/tcpproxy/ internal/cluster/ && golangci-lint run ./internal/filter/tcpproxy/... ./internal/cluster/...
git add internal/filter/tcpproxy/ internal/cluster/
git commit -m "phase 36.1 Task 6: tcp_proxy source_ip hash plane — parse TcpProxy.hash_policy (source_ip accept, filter_state DEPARTURE-reject); cluster.HashSourceIP(bare IP, no port) + WithHashKey before Dial(ctx); no hash_policy → byte-stable (AMEND-RH3)"
```

---

## Task 7: The `0061-lb-ring-hash` differential fixture

**Goal:** A cross-side `[tcp_proxy]` fixture over a 3-endpoint RING_HASH cluster with `hash_policy: [{source_ip: {}}]`. The driver binds 16 connections to each of 4 source IPs (`127.0.0.2..5`, 64 total). The SUBJECT affinity+spread is proven via the aggregate-count modular invariant (D-S36-4): every subject per-backend count is a multiple of 16 (affinity) and `>= 2` backends are nonzero (spread). The reference (Docker-NAT'd to one source IP) is asserted on `sum == 64` + cross-side byte-equivalence + the cross-side `StatsAsserter` (the 3 gauges cross-equal + cx/membership/quiesced). NO new BackendKind (reuses `TCPEcho = 0`). NO boot-reject dir.

**Files:**
- Create: `test/fixtures/0061-lb-ring-hash/driver/driver.go`
- Create: `test/fixtures/0061-lb-ring-hash/driver/driver_test.go`
- Create: `test/fixtures/0061-lb-ring-hash/README.md`
- Create: `test/fixtures/0061-lb-ring-hash/expectations.yaml`

- [ ] **Step 1: Write the driver** (`driver/driver.go`)

Start from a verbatim copy of `test/fixtures/0060-lb-random/driver/driver.go`, then:
1. `fixtureName = "0061-lb-ring-hash"`; pick a fresh `refContainerListenerPort` (grep `refContainerListenerPort` across `test/fixtures/` for a free value — `0060` uses 19149, so e.g. 19150 if free).
2. Rename the driver type (`randDriver` → `ringHashDriver`); update the `init()` registration + all receivers + the compile-time interface checks.
3. In BOTH `ReferenceBootstrap` and `SubjectConfig`: set `lb_policy: RING_HASH`, add `ring_hash_lb_config: {}` (defaults), and add the tcp_proxy `hash_policy: [{source_ip: {}}]` to the tcp_proxy filter config. Keep `BackendCount() == 3`, `SubjectListenerName() == "l_tcp"`, the STRICT_DNS (reference) / STATIC `127.0.0.1` (subject) split.
4. Replace the `drive` workload with the multi-source-IP affinity workload:
```go
const (
	sourceIPs   = 4   // 127.0.0.2 .. 127.0.0.5
	burstPerIP  = 16  // connections per source IP
	totalConns  = sourceIPs * burstPerIP // 64 (the 0059/0060 conservation target)
)

// drive opens burstPerIP connections from each of the 4 source IPs (bound via
// net.Dialer.LocalAddr — feasible on host loopback, §11.8), does one echo
// round-trip per conn, and closes so upstream_cx_active quiesces before AssertStats.
func drive(ctx context.Context, addr string) ([]byte, error) {
	var last []byte
	for s := 0; s < sourceIPs; s++ {
		local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, byte(2+s))}
		d := &net.Dialer{LocalAddr: local}
		for i := 0; i < burstPerIP; i++ {
			c, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				return nil, fmt.Errorf("dial from %v: %w", local.IP, err)
			}
			payload := []byte(fmt.Sprintf("rh-%d-%d\n", s, i))
			if _, err := c.Write(payload); err != nil {
				_ = c.Close()
				return nil, err
			}
			buf := make([]byte, len(payload))
			if err := readFull(c, buf, 2*time.Second); err != nil {
				_ = c.Close()
				return nil, err
			}
			last = buf
			_ = c.Close()
		}
	}
	// settle so upstream closes propagate before AssertStats scrapes.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return last, nil
}
```
(`readFull`/`sleepCtx`/`settleDelay` are REUSED from the `0060` copy. The byte-equivalence compare uses `last` — both sides echo identical payloads, so byte-equal holds regardless of which backend served.)

- [ ] **Step 2: Write the affinity+spread `AssertDistribution`** (the D-S36-4 modular invariant)

```go
// AssertDistribution: SUBJECT-SIDE affinity (every per-backend count is a multiple
// of burstPerIP — the consistent-hash invariant: one source IP → one key → one ring
// point → one backend, so each source IP contributes all 16 or 0 to each backend) +
// SPREAD (>= 2 distinct backends nonzero). DETERMINISTIC/EXACT — not a σ-band
// (reference_differential_band_sigma_margin governs RNG bands; affinity is not one).
// The REFERENCE is Docker-NAT'd to one source IP → all 64 on ONE backend; it is
// asserted on conservation only (single-key pin — AMEND-RH8 / reference_differential_
// hash_key_cross_side_infeasible). Its real proof is byte-equiv + the cross-side stats.
func (ringHashDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	if len(subjCounts) != 3 || len(refCounts) != 3 {
		return fmt.Errorf("expected 3 backend counts, got ref=%d subj=%d", len(refCounts), len(subjCounts))
	}
	// SUBJECT: affinity + spread.
	var subjSum, nonzero uint64
	for i, c := range subjCounts {
		subjSum += c
		if c%burstPerIP != 0 {
			return fmt.Errorf("subject affinity: backend[%d]=%d not a multiple of %d (key scattered? a source IP split across backends)", i, c, burstPerIP)
		}
		if c > 0 {
			nonzero++
		}
	}
	if subjSum != totalConns {
		return fmt.Errorf("subject conservation: sum %d != %d", subjSum, totalConns)
	}
	if nonzero < 2 {
		return fmt.Errorf("subject spread: only %d backend(s) nonzero, want >= 2 (ring collapsed?)", nonzero)
	}
	// REFERENCE: conservation only (single source IP via Docker NAT → single-key pin).
	var refSum uint64
	for _, c := range refCounts {
		refSum += c
	}
	if refSum != totalConns {
		return fmt.Errorf("reference conservation: sum %d != %d", refSum, totalConns)
	}
	return nil
}
```

- [ ] **Step 3: Write the `StatsAsserter`** (the §7 cross-vs-per-side set, EXTENDED with the 3 gauges)

Copy `0060`'s `AssertStats`, then ADD the 3 cross-equal `ring_hash_lb.*` gauge assertions (SPEC §7):
```go
	// cross-equal (both sides): ring_hash_lb gauges depend only on ring-config + host
	// count, NOT addresses → IDENTICAL on subject and reference (AMEND-RH4).
	assertCrossEqual(t, ref, subj, "cluster.c_echo.ring_hash_lb.size", 1026)
	assertCrossEqual(t, ref, subj, "cluster.c_echo.ring_hash_lb.min_hashes_per_host", 342)
	assertCrossEqual(t, ref, subj, "cluster.c_echo.ring_hash_lb.max_hashes_per_host", 342)
```
plus the inherited cross-equal `upstream_cx_total == 64` + `membership_total == 3` + `upstream_cx_active == 0` (quiesced); per-side `upstream_rq_total` (ref = conn count; subj = 0 — the tcpproxy path never calls `IncUpstreamRqTotal`, the 0059/0060 boundary). Match the as-built `0060` `AssertStats`/`scrapeStats` helper shapes (the `assertCrossEqual` form is whatever `0060` uses — reuse it verbatim). Add the compile-time interface checks:
```go
var (
	_ fixture.Driver               = (*ringHashDriver)(nil)
	_ fixture.DistributionAsserter = (*ringHashDriver)(nil)
	_ fixture.StatsAsserter        = (*ringHashDriver)(nil)
)
```

- [ ] **Step 4: Write README.md + expectations.yaml + driver_test.go**

- `README.md`: the multi-source-IP workload (16/IP × 4 IPs = 64; `net.Dialer.LocalAddr` 127.0.0.2..5); the affinity invariant (each subject backend count ≡ 0 mod 16, the consistent-hash one-key-one-backend property) + the spread (`>= 2`); the **reference asymmetry** (Docker NAT → single source IP → single-backend pin → conservation-only, the live D-RH4b observation; cross-side host identity INFEASIBLE — AMEND-RH8); the 3 cross-equal `ring_hash_lb.*` gauges (1026/342/342); the per-side `upstream_rq_total` boundary; the D-S36-4 modular-invariant coverage note (necessary-and-overwhelmingly-discriminating vs the scatter break, not a tight proof vs an adversarial even-split — the realistic break is random scatter, caught flake-free). The firsts-now-expectations: FIRST consistent-hash fixture; FIRST non-zero LB-stat delta (+3 gauges → surface 1119); NO new BackendKind; NO new fuzzer. The Task-8 deliberate-break records.
- `expectations.yaml`: mirror the `0060` shape (cross-side byte-equivalence + the asserter dispatch; document the affinity/spread + the 3-gauge stats prongs).
- `driver_test.go`: the registration smoke (mirror `0060`'s `driver_test.go`).

- [ ] **Step 5: Run the fixture (requires Docker + the contrib reference image)**

```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=1 -v 2>&1 | tail -40
```
Expected: PASS — byte-equivalence + the subject affinity/spread + the reference conservation + the cross-side stats (incl. the 3 gauges). **Use `-run 'TestDifferential/0061'`, NOT `-run '0061'`** (`reference_differential_run_selector`). Verify the decode ran (`downstream_cx_rx_bytes_total > 0` per `reference_docker_probe_bridge_network`). If Docker is unavailable in the IMPL environment, the controller runs this gate where Docker is present.

> **Source-IP-bind caveat:** if the runner/Docker environment cannot bind `127.0.0.2..5` (e.g. a restricted CI netns), the subject affinity needs those distinct source IPs to produce distinct keys — confirm the bind works in the controller's Docker environment (proven on host loopback §11.8). If a CI environment blocks it, document the constraint and run the gate where loopback aliasing is available.

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0061-lb-ring-hash/ && golangci-lint run ./test/...
git add test/fixtures/0061-lb-ring-hash/
git commit -m "phase 36.1 Task 7: 0061-lb-ring-hash differential fixture — multi-source-IP (127.0.0.2..5, 16/IP, 64 total) over a RING_HASH/source_ip cluster; SUBJECT affinity (per-backend count ≡ 0 mod 16) + spread (>=2) via the aggregate modular invariant (D-S36-4, no new BackendKind); reference conservation-only (Docker NAT single-key pin); cross-side StatsAsserter + 3 ring_hash_lb gauges (1026/342/342)"
```

---

## Task 8: Deliberate-break liveness (`-count=1`) + ≥20-run flake check

**Goal:** PROVE each `0061` prong is live (`reference_differential_break_protocol_count1` — go test caching serves a stale PASS otherwise) and flake-free.

**Files:** (break/revert only — no committed production change; record in README.md + PROGRESS.md)

- [ ] **Step 1: Repeat-run flake check (≥20)**

```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=20 2>&1 | tail
```
Expected: 20/20 PASS. The affinity leg is DETERMINISTIC (the ring is fixed; the source-IP keys are fixed) — it should NEVER flake. The spread leg (`>= 2`) is overwhelmingly stable (4 source-IP keys over 3 backends covering ≥2). If the spread ever flakes (a degenerate key collision sending all 4 source IPs to one backend — astronomically unlikely with the default 1026-point ring), record and investigate; do NOT widen below `>= 2` (that is the minimum meaningful spread).

- [ ] **Step 2: Deliberate break (i) — scatter the key → SUBJECT affinity FAILS**

In `ringHashLB.Pick`, force a random draw ignoring the key: replace the `if !hasHash { hashKey = rh.rng() }` guard so it ALWAYS draws (`hashKey = rh.rng()` unconditionally). Run:
```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=1 2>&1 | tail
```
Expected: FAIL on `subject affinity: backend[i]=N not a multiple of 16` (random scatter → ~21/21/22, not multiples of 16). REVERT.

- [ ] **Step 3: Deliberate break (ii) — collapse the ring → SUBJECT spread FAILS**

In `ringHashLB.Pick`, force `m = 0` always (every pick → `endpoints[ring[0].ep]`). Run `-count=1`:
```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=1 2>&1 | tail
```
Expected: FAIL on `subject spread: only 1 backend(s) nonzero` (all 64 on one backend — and incidentally affinity still holds since 64 % 16 == 0, so the spread leg is the one that bites; this proves spread is non-vacuous). REVERT.

- [ ] **Step 4: Deliberate break (iii) — stats prong (drop-Inc / cross-equal)**

Temporarily corrupt one cross-equal want (e.g. `upstream_cx_total` 64 → 99) — confirm the `StatsAsserter` FAILS with `-count=1`. REVERT.

- [ ] **Step 5: Deliberate break (iv) — corrupt a gauge → the `ring_hash_lb.*` prong FAILS**

In `registerClusterMetrics`, temporarily set `ring_hash_lb.size` to `rh.size + 1`. Run `-count=1`:
```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=1 2>&1 | tail
```
Expected: FAIL on the cross-equal `ring_hash_lb.size` assertion (subject 1027 != reference 1026). REVERT.

- [ ] **Step 6: Record + commit (LOCAL-ONLY)**

Record the four break results + the 20/20 flake check in README.md (driver comments) + PROGRESS.md (the `0030` dead-assertion lesson).
```bash
go test ./test/differential/ -run 'TestDifferential/0061' -count=1 2>&1 | tail   # confirm green after all reverts
git add test/fixtures/0061-lb-ring-hash/ docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md
git commit -m "phase 36.1 Task 8: 0061 deliberate-break liveness (-count=1) — scatter-key [affinity fails], collapse-ring [spread fails], stats-drop [stats fails], corrupt-gauge [ring_hash_lb prong fails]; 20/20 flake-free (affinity deterministic)"
```

---

## Task 9: Full differential re-verify + race + conformance unaffected

**Goal:** Prove the seam extension + the new manager case kept all 62 prior fixtures byte-exact (the three incumbent policies ignore the new params → behavior-neutrality is structural) and `0061` is green; run `-race`; assert h2spec/proxy-wasm unaffected; confirm zero new go.mod dep.

**Files:** none (verification only)

- [ ] **Step 1: Full differential suite (63 dirs)**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
Expected: ALL 63 PASS — the 62 prior dirs byte-exact through the seam widening + the new `case Cluster_RING_HASH` (the incumbent paths ignore the hash params; the seam exported surface is byte-stable) + `0061` green. **This is the six-gate REAL guard** (`reference_docker_probe_bridge_network` — the controller runs where Docker is present).

- [ ] **Step 2: Race + short across the repo**

```bash
go test -race -short ./... 2>&1 | tail -20
```
Expected: PASS, no race (the ring is immutable post-build; `Pick` is read-only; the no-hash-fallback rng is mutex-guarded via the REUSED `newPCGRNG`).

- [ ] **Step 3: Build + vet + gofmt + lint (full repo) + tidy**

```bash
go build ./... && go vet ./... && gofmt -l internal/ test/fixtures/0061-lb-ring-hash/ && golangci-lint run ./... 2>&1 | tail
go mod tidy -diff && echo "TIDY_CLEAN"   # ZERO new go.mod dep (AMEND-RH1/RH7 — xxHash64/murmurHash2 hand-rolled)
```
Expected: clean; `go mod tidy -diff` empty.

- [ ] **Step 4: Conformance unaffected**

Re-run (or assert-unaffected with rationale: phase 36.1 touches `internal/cluster` + the tcp_proxy filter — no HTTP/h2/proxy-wasm path) h2spec **53/53** + proxy-wasm **10/10** per the repo's conformance recipe. Record in PROGRESS.md.

- [ ] **Step 5: Commit (LOCAL-ONLY) — verification evidence into PROGRESS.md**

```bash
git add docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md
git commit -m "phase 36.1 Task 9: six-gate evidence — 63-dir differential byte-exact (62 prior unchanged through the seam extension + the new manager case + 0061), -race -short green, go mod tidy clean (zero new dep), h2spec 53/53 + proxy-wasm 10/10 unaffected"
```

---

## Task 10: Completion bundle (ADR-0052 atomic landing)

**Goal:** Land the BEHAVIOR_CONTRACT delta, the full ADR-0235 + ADR-0236 entries, and the STATE/ROADMAP advance — atomically with the code.

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/36-load-balancer-ring-hash/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md (SPEC §9)**

Add a `### Load balancer — ring_hash (RING_HASH)` subsection (mirroring `### Load balancer — random (RANDOM)`): the RING_HASH acceptance (the `RingHashLbConfig` gate — min/max defaults 1024/8388608, the two-layer reject [PGV `<= 8388608` + runtime min>max], `hash_function` XX_HASH/MURMUR_HASH_2); the Ketama ring semantics (build `xxHash64("addr:port_i")`, lookup first point `>= key` wrap, the no-hash random fallback — the v1.37.2 mirror); the seam EXTENSION (the ctx-carried hash key; `WithHashKey` + `HashSourceIP` the new exported helpers); the tcp `source_ip` plane (the HTTP route plane is 36.2); the per-side affinity (deterministic) vs cross-side non-identity (different per-side ring layouts — the documented boundary, AMEND-RH8); the 3 mirrored `ring_hash_lb.*` gauges (cross-side-exact); the healthy-set boundary (no health checking → all-hosts ring); the D-S36-2 shared-RNG-message cosmetic note; the D-S36-4 modular-invariant coverage note. Update the reject-text line (RING_HASH retired from the rejected set → the FOURTH accepted policy; supported-list `…, RANDOM, RING_HASH`; MAGLEV stays the recorded departure). Update the deferred-LB-family list (ring_hash retired; **5** candidates remain {maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}). Update the stat-surface doc count **1116 → 1119** (+3 `ring_hash_lb.*` gauges — the FIRST non-zero LB-stat delta).

- [ ] **Step 2: DECISIONS.md — the full ADR-0235 + ADR-0236 entries (ADR-0044 in-place)**

Author ADR-0235 (the LB hash-key seam extension): promote the SPEC §13 §Context DRAFT verbatim (status PROPOSED → ACCEPTED); §Decision (the widened `Pick(hashKey, hasHash)`; the ctx-carry `WithHashKey`/`hashKeyFrom`; the 2 exported additive symbols `WithHashKey` + `HashSourceIP`; the byte-stable exported `Cluster` surface; the 3 incumbent policies' ignored params); §Consequences (the seam's FIRST extension, the durable asset maglev reuses unchanged; ADR-0024 unamended; the 62-fixture byte-identity proof). Author ADR-0236 (the ring_hash policy): promote the SPEC §13 §Context DRAFT (PROPOSED → ACCEPTED); §Decision (the Ketama `ringHashLB`; the hand-rolled xxHash64/murmurHash2; the two-layer gate; the bare manager case + the reject-text change; the tcp `source_ip` plane [36.1]; the 36.2 HTTP plane deferred to its own PLAN; the `0061` affinity-modular-invariant proof shape); §Consequences (the FIRST consistent-hash policy + FIRST non-zero LB-stat delta [+3 gauges → 1119]; cross-side host-identity INFEASIBLE — the documented harness boundary; the 36.2 leg follows; NO new fuzzer/BackendKind/dep). Tail advances **ADR-0234 → ADR-0236**; next-free **ADR-0237**.

- [ ] **Step 3: STATE.md + ROADMAP.md**

STATE active-phase → `phase 36.1 (load-balancer-ring-hash) IMPL done`; lifecycle-state → the 36.1-done routing (next → the 36.2 SPEC → PLAN → IMPL). Counts: fixtures 62 → **63**; stat surface **1116 → 1119**; fuzzers **42**; BackendKind tail **33**; DECISIONS tail **ADR-0236**. ROADMAP row 36 — flip the `36.1` leg to done (a flat family row — NO parent rollup per ADR-0106; the `36.2` leg stays in-progress); the Load-balancing family stays OPEN (5 candidates remain after 36).

- [ ] **Step 4: Finalize PROGRESS.md** — all 10 tasks complete; the six-gate evidence; the D-S36-1..7 resolutions; the four deliberate-break records; the surface 1116 → 1119 / fixtures 62 → 63 deltas.

- [ ] **Step 5: Final six-gate re-run + commit (LOCAL-ONLY)**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./... && go test -race -short ./... 2>&1 | tail
go test ./test/differential/ -count=1 2>&1 | tail
git add docs/envoy-go/
git commit -m "phase 36.1 Task 10: completion bundle — BEHAVIOR_CONTRACT ring_hash delta (surface 1116->1119); ADR-0235 (seam extension) + ADR-0236 (ring_hash policy) full entries (ADR-0044 in-place; tail → ADR-0236); STATE/ROADMAP row 36.1 done (fixtures 62->63); the FIRST consistent-hash policy + FIRST non-zero LB-stat delta"
```

- [ ] **Step 6: Controller stage-close** — the controller squash-merges the 36.1 IMPL branch to master, runs the final six-gate on master, and (per `feedback_push_to_origin`, tests green) pushes to origin. Subagents NEVER push (`feedback_subagents_no_push`).

---

## Exit criteria (ADR-0052 atomic landing — the 36.1 leg)

- stat surface **1116 → 1119** (+3 `ring_hash_lb.{size,min_hashes_per_host,max_hashes_per_host}` gauges — the FIRST non-zero LB-stat delta; AMEND-RH4).
- differential fixtures **62 → 63** (`0061-lb-ring-hash`; NO boot-reject dir — AMEND-RH5).
- fuzzers **42** (NO new fuzzer — the ring decodes no untrusted wire bytes; the property test is unit-level, D-RH7); BackendKind tail **33** (NO new BackendKind — `TCPEcho` 0 reused).
- DECISIONS tail **ADR-0234 → ADR-0236** (ADR-0235 seam + ADR-0236 policy; full entries at this IMPL per ADR-0044; next-free ADR-0237).
- All 63 differential dirs byte-exact (62 prior unchanged through the seam extension + the new manager case + `0061` green); `-race -short` green; h2spec 53/53 + proxy-wasm 10/10 unaffected; `go mod tidy -diff` empty (ZERO new go.mod dep).
- 2 new exported symbols (`cluster.WithHashKey` + `cluster.HashSourceIP`) — additive; the existing exported `Cluster` method signatures byte-stable.
- ROADMAP row 36 — the `36.1` leg `done` (flat family row); the `36.2` leg in-progress; the Load-balancing family OPEN (5 candidates remain). **Next → the 36.2 SPEC → PLAN → IMPL** (the HTTP route `hash_policy` plane; its own lifecycle).
