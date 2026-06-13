# Phase 37 Implementation Plan — `maglev` load balancer (`Cluster.LbPolicy MAGLEV`): a Maglev lookup-table consistent-hash policy that REUSES the ADR-0235 hash-key seam + BOTH ADR-0237 producers UNCHANGED

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.LbPolicy MAGLEV` (`envoy.config.cluster.v3`, enum value 5) — the project's FIFTH LB policy and its SECOND consistent-hashing policy — as a fixed-size Maglev lookup table (`table[hashKey % M]`), built once from the endpoint set, that REUSES the ADR-0235 LB hash-key seam UNCHANGED (the seam's FIRST hash-policy reuse) and consumes BOTH ADR-0237 producers (tcp `source_ip` + HTTP route `hash_policy`) UNCHANGED.

**Architecture:** A new `maglevLB` type (`internal/cluster/maglev.go`, same package) mirrors `ringHashLB` structurally — built ONCE at construction (sort endpoints by `Addr()` ascending; per host `offset = xxHash64(addr) % M`, `skip = (xxHash64Seed(addr,1) % (M-1)) + 1`; populate `table[(offset + skip·next) % M]` host-by-host until all `M` slots fill), immutable thereafter (so `Pick` is lock-free), with an injectable `rng func() uint64`. `Pick(hashKey, hasHash)` indexes `table[hashKey % M]`; `hasHash==false` → a uniform random table index from the rng. `Manager.buildCluster` gains ONE `case clusterv3.Cluster_MAGLEV` parsing `Cluster.MaglevLbConfig` (`table_size` `UInt64Value`, self-supplied default 65537; the PGV cap `≤ 5000011` + a primality gate); the `default` reject text extends its supported-list `…, RING_HASH` → `…, RING_HASH, MAGLEV`. The ADR-0235 seam, `cluster.go`, the producers, and every pick-funnel consumer are UNTOUCHED. ZERO new packages, ZERO new go.mod deps (the enum + config are in the pinned core `/envoy v1.32.4`), ZERO new hash code (the table build reuses `hash.go`'s `xxHash64`/`xxHash64Seed`).

**Tech Stack:** Go 1.26.x; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `Cluster.LbPolicy MAGLEV` enum 5 + `Cluster.MaglevLbConfig` in the pinned module; ZERO new dep, `go mod tidy -diff` empty); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). Reuses `internal/cluster/` (the 02/34/35/36 Manager + the ADR-0235 `Pick(hashKey, hasHash)` seam + the `ringHashLB` structural template + `newPCGRNG` + `xxHash64`/`xxHash64Seed`), the LANDED producers (`internal/filter/tcpproxy/filter.go`, `internal/filter/http/router/router.go` — UNCHANGED), and the `0062` differential harness (the HTTP-header NAT-transparent cross-side affinity+spread harness + `StatsAsserter` + `DistributionAsserter` + the `HTTPEcho` backend). The differential proof is `0063-lb-maglev` — the `0062` convention reused verbatim, retargeted to MAGLEV.

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/37-load-balancer-maglev/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-M1..M6; §3.0 the split disposition (single flat row, NO escape valve); §3.1 the `maglevLB` design (the build + lookup code, VERBATIM); §3.2 the seam REUSE; §3.3 the manager case + `parseMaglevLbConfig` (code VERBATIM); §5 the proto roster; §6 the reject roster; §7 the +2 gauges; §8.1 the `0063` fixture design; §10 the ~8–10-task spine; §11 the D-M1..D-M6 empirical pins; §12 the D-S37 questions; §13 the ADR-0238 §Context DRAFT.
- **BRAINSTORM:** `docs/envoy-go/phases/37-load-balancer-maglev/BRAINSTORM.md` — the charter (Q0/Q1). NOTE the §2.3 "primality DEPARTURE" framing is SUPERSEDED by SPEC AMEND-M5 → PARITY (the reference itself rejects a non-prime `table_size`).
- **As-built anchors** (re-pinned at the SPEC; re-confirm at Task 1 — line numbers shift on the IMPL-session tip):
  - `internal/cluster/maglev.go` — **does NOT yet exist** (Task 3 creates it; the `ringhash.go` sibling precedent).
  - `internal/cluster/ringhash.go:19-141` — the STRUCTURAL TEMPLATE (`ringHashLB` build-once + immutable + injected-rng shape; `newRingHash`/`newRingHashWithRNG`; the `var _ loadBalancer = (*ringHashLB)(nil)` assert; the `Pick` empty-set/`noopRelease`/no-hash-fallback shape). `maglevLB` mirrors this.
  - `internal/cluster/loadbalancer.go:15-21` — the `loadBalancer` interface (`Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)`:21 — REUSED unchanged) + `noopRelease`:26 (REUSED) + `errNoEndpoints` (REUSED; defined in this package).
  - `internal/cluster/hash.go:40` (`xxHash64(b []byte) uint64`) + `:46` (`xxHash64Seed(b []byte, seed uint64) uint64`) — REUSED VERBATIM for offset/skip; **ZERO new hash code**.
  - `internal/cluster/cluster.go:39` (`func (e Endpoint) Addr() string` = `"IP:PORT"` = the maglev key) + `:182` (`hashKeyFrom`) + `:196`/`:233`/`:287` (the `c.lb.Pick(hk, ok)` funnels — UNCHANGED; maglev consumes them as `ringHashLB` does).
  - `internal/cluster/leastrequest.go:63` (`newPCGRNG` — REUSED for the no-hash fallback; the shared seed-error message).
  - `internal/cluster/manager.go:243` (the `lb_policy` switch — gains `case clusterv3.Cluster_MAGLEV` after the `case Cluster_RING_HASH`:262) + `:275` (the `default` reject text — extends `…, RING_HASH` → `…, RING_HASH, MAGLEV`) + `:347` (`parseRingHashLbConfig` — the `parseMaglevLbConfig` precedent) + `:99` (`registerClusterMetrics`) + `:110-114` (the `if rh, ok := c.lb.(*ringHashLB); ok { … }` gauge-registration block — the `*maglevLB` type-assert mirrors it).
  - `internal/cluster/manager_test.go:320-328` (`TestManager_Error_UnsupportedLBPolicy` — trigger CURRENTLY `Cluster_MAGLEV`:322, pins `"ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH"`:327 → DOUBLY-hit retarget MAGLEV → `Cluster_CLUSTER_PROVIDED` + substring `"…, RING_HASH, MAGLEV"`) + the ring_hash gauge-test precedent (`gaugeValue(reg, name)` readback, ~`:1140`).
- **Differential harness / the `0063` template:** `test/fixtures/0062-lb-ring-hash-http/driver/driver.go` (the closest template — `clusterName = "c_echo"`:88, `refContainerListenerPort = 19151`:84, `hashValues = 16`/`repeatPerVal = 16`/`totalReqs = N*K`:91-93, `healthReqs = 8`:95, the route header `hash_policy`, `AssertDistribution`:307 [the `% repeatPerVal` modular invariant + spread + conservation, BOTH sides], `AssertStats`:343 [the cross-equal set + `scrapeStats`], the `HTTPEcho` backend) + `test/differential/fixture/fixture.go` (the `Driver`/`DistributionAsserter`/`StatsAsserter` interfaces; `BackendKind` tail `TCPThriftResponder = 33`:562).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` — subagent-driven execution (the IMPL runs subagent-per-task).
- `feedback_git_worktrees` — this PLAN was authored in worktree `.worktrees/phase-37-plan`; the IMPL runs in its own worktree.
- `feedback_subagents_no_push` — **subagents commit LOCAL-ONLY**; the controller squash-merges + pushes at stage-close.
- `feedback_pertask_gofmt_lint` — **every task** runs `gofmt -l` + `golangci-lint run` on the touched packages (not just `go vet`).
- `feedback_subagent_worktree_path_targeting` / `_detach` — all paths below are repo-root-relative; the IMPL worktree is the canonical checkout; the controller verifies the main checkout stays clean and re-verifies the branch each task (deliberate-break tasks can detach HEAD — restore, never checkout-sha/amend). PROGRESS.md is at the pinned canonical path `docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md`.
- `reference_differential_break_protocol_count1` — every new differential assertion is proven live by a deliberate-break with `-count=1` (go test caching serves a stale PASS otherwise).
- `reference_differential_asserter_dispatch` — the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses `DistributionAsserter` (driver-side, runs on both paths). **Name the collapse-table break PRECISELY** (build-level vs `Pick`-level) to avoid a vacuous gauge assertion — Task 8.
- `reference_differential_run_selector` — targeted runs use `-run 'TestDifferential/0063'`, NEVER `-run '0063'` (which matches zero subtests → vacuous green).
- `reference_differential_hash_key_cross_side_infeasible` — cross-side host IDENTITY is INFEASIBLE (the two tables are over different `"IP:PORT"` address strings); the proof is the per-side modular invariant, NOT host equality.
- `reference_fixture_workload_constant_desync` — N/K/totalReqs MUST stay synced with any hand-rolled count slices; go-test caching masks a desync until `-count=1`.
- `reference_docker_probe_bridge_network` — the `0063` differential needs Docker + the contrib reference image on a bridge network; the controller runs that gate where Docker is present (verify the decode ran: `downstream_cx_rx_bytes_total > 0`).
- ADR-0235 (the LB hash-key seam — REUSED unchanged; its §Consequences "the durable asset maglev reuses unchanged" claim this phase CONSUMMATES), ADR-0236 (the ring_hash policy — the STRUCTURAL + ADR STYLE TEMPLATE), ADR-0237 (the HTTP producer — REUSED unchanged), ADR-0024 (per-cluster LB counter scope — UNAMENDED; maglev holds no per-cluster counter state), ADR-0044 (ADR §Context at SPEC, body at IMPL, in-place), ADR-0052 (the atomic-landing six-gate), ADR-0080 (byte-stable reject text), ADR-0106 (flat family row — NO parent rollup), ADR-0060 (histograms deferred; the 2 `maglev_lb.*` are GAUGES), ADR-0045 (the split-gate — FINAL re-check below), ADR-0004 (the empirical-pin discipline), ADR-0227 (the contrib reference image).

## D-question resolutions (SPEC §12)

- **D-S37-1 (file placement):** RESOLVED as anticipated. The new `maglevLB` type + the build + `Pick` + the `isPrime` helper → NEW `internal/cluster/maglev.go` (the `ringhash.go`/`random.go` sibling precedent); the manager `case Cluster_MAGLEV` + `parseMaglevLbConfig` + the reject-text edit → `manager.go`; the gauge type-assert → `registerClusterMetrics`; the doubly-hit retarget → `manager_test.go`. ZERO new packages.
- **D-S37-2 (primality reject wording + `isPrime` shape):** RESOLVED — the house-prefixed `cluster: %q: maglev_lb_config.table_size (%d) must be a prime number` (NO fixture pins it; IMPL-finalizable within the ADR-0080 / §6.2 principle — the reference's app-layer string `The table size of maglev must be prime number` need not match byte-for-byte, as ring_hash's PGV-mirror rejects already use the house prefix). `isPrime(uint64)` is a STANDALONE helper in `maglev.go` (Task 2 — TDD-first; trial division to `√n`, adequate for `n ≤ 5000011`).
- **D-S37-3 (equal-weight simplification vs weighted gate):** RESOLVED — the equal-weight MVP (one slot per host per iteration; weighted maglev DEFERRED per SPEC §2). Keep the populate loop honest enough that adding the `iteration·weight < target_weight` gate later is a local change (the `next`/`count` per-host arrays are already the general shape).
- **D-S37-4 (`0063` constants + break protocol):** RESOLVED — N=16 × K=16 = 256 routed `GET /get` + 8 `/health` (the `0062` constants, retargeted); the deliberate-break protocol scatter-the-key (affinity) / collapse-the-table (spread + gauges, build-level) / stats-drop, all with `-count=1`; ≥20-run flake check; the `reference_fixture_workload_constant_desync` guard (N/K/totalReqs DERIVED, never literals).
- **D-S37-5 (standalone property test vs folded):** RESOLVED — FOLDED into Task 3/4's `maglev_test.go` as a deterministic property test (random keys → always a valid endpoint, never panics; the table is fully filled / no `-1` remains; equal-weight distribution within ±1 entry/host; affinity holds) — the `ringhash_test.go` `TestRingHash_RandomKeysNeverPanicAlwaysValid` precedent. NOT a `Fuzz*` corpus entry (no untrusted wire input — fuzzers STAY 42).
- **ADR-0045 split-gate FINAL re-check:** **NO SPLIT.** This PLAN decomposes into **10 tasks** (≤ ~25) over **~140–200 production LoC** (`maglev.go` ~110–160 [the sort + offset/skip + the populate loop + `Pick` + `isPrime` + min/max] + the manager case + `parseMaglevLbConfig` ~30 + the 2 gauge registrations ~5 + the reject-text line; ≤ ~1500). BOTH ADR-0045 axes hold by an order of magnitude — smaller than ring_hash's 36.1 leg (which BUILT the seam + a producer). SINGLE flat phase 37 — NO pre-split, NO escape valve (the producers already exist; there is no second subsystem to couple, unlike ring_hash-36 — the least_request-34/random-35 single-flat-row precedent).

### Decomposition note (10 tasks vs the SPEC's indicative ~8–10)

SPEC §10 lists Task 9 as an OPTIONAL standalone table property test and bundles the full differential re-verify + the completion bundle into Task 10. This PLAN: (a) FOLDS the property test into Task 3/4's `maglev_test.go` (D-S37-5 — the ring_hash precedent; no standalone task), and (b) SPLITS the SPEC's Task 10 into **Task 9 (verification gate)** and **Task 10 (completion bundle)** — different kinds of work (running gates vs. authoring ADR-0052 docs; the phase-35/36 precedent). All other SPEC §10 tasks map 1:1.

| SPEC §10 task | This plan |
|---|---|
| 1 baselines/anchors gate + PROGRESS | Task 1 |
| 2 `isPrime` helper | Task 2 |
| 3 `maglevLB` build (sort + offset/skip + populate) | Task 3 |
| 4 `maglevLB.Pick` (+ the folded property test, D-S37-5) | Task 4 |
| 5 manager acceptance + parse + retarget | Task 5 |
| 6 the 2 `maglev_lb.*` gauge registrations | Task 6 |
| 7 the `0063-lb-maglev` fixture | Task 7 |
| 8 deliberate-break liveness + ≥20-run flake | Task 8 |
| 9 (optional) property test | FOLDED into Task 4 |
| 10 full differential re-verify | **Task 9** |
| (10, cont.) completion bundle | **Task 10 (split out)** |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/maglev.go` | **CREATE** (Tasks 2–4) | `isPrime(uint64) bool` (Task 2) + `maglevLB` + `maglevCfg` + `newMaglev`/`newMaglevWithRNG` (the table build: sort-by-`Addr()` + offset/skip + the populate loop + min/max — Task 3) + `Pick` (`table[hashKey % M]` + the no-hash rng fallback — Task 4) + the `var _ loadBalancer = (*maglevLB)(nil)` assert. |
| `internal/cluster/maglev_test.go` | **CREATE** (Tasks 2–4) | `isPrime` vectors (Task 2); the build tests (full fill / no `-1` / terminates / min,max = floor,ceil(M/N); a small-M hand-computed table vector — Task 3); the `Pick` tests (affinity / no-hash fallback / empty-set / `noopRelease`) + the folded property test (D-S37-5 — Task 4). |
| `internal/cluster/manager.go` | MODIFY (Tasks 5–6) | The `case clusterv3.Cluster_MAGLEV` + `parseMaglevLbConfig` (default 65537 + cap + primality) + the NEW reject text (Task 5); the 2 `maglev_lb.*` gauge registrations in `registerClusterMetrics` (Task 6). |
| `internal/cluster/manager_test.go` | MODIFY (Tasks 5–6) | MAGLEV accept (defaults + non-default-prime valid) + the cap/primality/mismatched-oneof arms + the doubly-hit retarget (Task 5); the gauge-registration assertion + the non-MAGLEV-no-gauges assertion (Task 6). |
| `test/fixtures/0063-lb-maglev/` | **CREATE** (Task 7) | `driver/driver.go`, `driver/driver_test.go`, `README.md`, `expectations.yaml` (mirroring the `0062` dir layout). |
| `docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 10) | The `maglev` subsection + the reject-text line update + the deferred-family list + the 2 `maglev_lb.*` gauges + the stat-surface doc count 1119 → 1121. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 10) | The full ADR-0238 (maglev policy) entry (§Context + §Decision + §Consequences; ADR-0044 in-place). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 10) | Active-phase + counts advance (fixtures 64 → 65; stat surface 1119 → 1121; DECISIONS tail → ADR-0238); ROADMAP row 37 `in-progress → done`. |

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code (the established first-task discipline), re-pin the as-built line anchors, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

Run (from repo root):
```bash
ls -d test/fixtures/[0-9]* | wc -l                       # expect 64
ls -d test/fixtures/[0-9]* | tail -1                     # expect test/fixtures/0062-lb-ring-hash-http
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect TCPThriftResponder BackendKind = 33
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l        # expect 42
grep "^## ADR-0" docs/envoy-go/DECISIONS.md | tail -1    # expect tail ADR-0237, next-free ADR-0238
grep -n "1119" docs/envoy-go/BEHAVIOR_CONTRACT.md        # the stat-surface DOC count (no programmatic golden)
go build ./... && echo BUILD_OK
```
Expected: fixtures **64** (tail `0062-lb-ring-hash-http`), BackendKind tail **33**, fuzzers **42**, stat surface **1119** (a DOC count in BEHAVIOR_CONTRACT.md, NOT a programmatic test — the phase-35/36 PROGRESS note), DECISIONS tail **ADR-0237** (the ADR-0238 §Context is a DRAFT in SPEC §13 — NOT yet in DECISIONS.md).

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these line anchors still hold (they shift if the tip moved); record actual line numbers in PROGRESS.md:
```bash
grep -n "Pick(hashKey uint64, hasHash bool)\|noopRelease = func\|errNoEndpoints" internal/cluster/loadbalancer.go   # the seam REUSED unchanged
grep -n "^func xxHash64\|^func xxHash64Seed" internal/cluster/hash.go                 # REUSED for offset/skip (no new hash code)
grep -n "func (e Endpoint) Addr" internal/cluster/cluster.go                          # the maglev key = "IP:PORT"
grep -n "func newPCGRNG" internal/cluster/leastrequest.go                             # REUSED for the no-hash fallback
grep -n "var _ loadBalancer = (\*ringHashLB)\|func newRingHashWithRNG\|func (rh \*ringHashLB) Pick" internal/cluster/ringhash.go   # the STRUCTURAL TEMPLATE
grep -n "switch c.GetLbPolicy()\|case clusterv3.Cluster_RING_HASH\|unsupported lb_policy" internal/cluster/manager.go   # the case to add + the reject text
grep -n "func parseRingHashLbConfig\|func registerClusterMetrics\|(\*ringHashLB); ok" internal/cluster/manager.go      # the parse precedent + the gauge-register site
grep -n "func TestManager_Error_UnsupportedLBPolicy\|Cluster_MAGLEV" internal/cluster/manager_test.go   # the doubly-hit retarget target
grep -n "func gaugeValue" internal/cluster/manager_test.go                            # the gauge readback helper (REUSED; do NOT reinvent)
test -f internal/cluster/maglev.go && echo "WARN maglev.go exists" || echo "maglev.go ABSENT (expected — Task 3 creates it)"
```

- [ ] **Step 3: Confirm the reject-text blast radius is exactly THREE sites (AMEND-M5)**

```bash
grep -rln "unsupported lb_policy" internal/ cmd/                          # expect ONLY internal/cluster/manager.go
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH" test/           # expect EMPTY (no fixture pins the text → no boot-reject dir)
grep -rln "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH" docs/envoy-go/BEHAVIOR_CONTRACT.md  # expect a hit (the doc reject-text line)
```
Expected: the production string ONLY in `manager.go`; ZERO fixture hits (confirms NO boot-reject dir — AMEND-M5); the doc hit in BEHAVIOR_CONTRACT.md. (The unit pinner `TestManager_Error_UnsupportedLBPolicy` is the third site.)

- [ ] **Step 4: Confirm the seam + producers are REUSE-only (zero churn — AMEND-M3/M6)**

```bash
grep -rln "c.lb.Pick(" internal/                                   # confirm cluster.go is the ONLY Pick funnel (unchanged)
grep -rln "cluster.HashSourceIP\|cluster.HashHeaderValues\|WithHashKey" internal/   # the LANDED producers — phase 37 touches NONE of them
```
Record: maglev plugs into the UNCHANGED `c.lb.Pick(hk, ok)` funnel exactly as `ringHashLB` does; the ADR-0237 producers (`tcpproxy/filter.go`, `http/router/router.go`) are NOT touched this phase.

- [ ] **Step 5: Create PROGRESS.md**

Create `docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md` with: the 10-task table (status column), the count anchors from Step 1, the as-built line anchors from Step 2, the three-site blast radius from Step 3, the seam/producer reuse confirmation from Step 4, the D-S37-1..5 resolutions, and the ADR-0045 re-check verdict (NO SPLIT; ~140–200 LoC / 10 tasks). Mark Task 1 complete.

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md
git commit -m "phase 37 Task 1: baselines gate + PROGRESS.md (fixtures 64 / fuzzers 42 / stat surface 1119 / BackendKind 33 / DECISIONS tail ADR-0237 confirmed; reject-text blast radius 3 sites; seam+producers REUSE-only)"
```

---

## Task 2: The `isPrime` helper (`maglev.go`)

**Goal:** Implement `isPrime(uint64) bool` (trial division to `√n`) — the config-build primality gate (AMEND-M5; the maglev permutation REQUIRES `M` prime for a terminating full-cycle fill). Standalone TDD-first (D-S37-2).

**Files:**
- Create: `internal/cluster/maglev.go`
- Create: `internal/cluster/maglev_test.go`

- [ ] **Step 1: Write the failing test** (`maglev_test.go`)

```go
package cluster

import "testing"

func TestIsPrime(t *testing.T) {
	cases := []struct {
		n    uint64
		want bool
	}{
		{0, false}, {1, false}, {2, true}, {3, true}, {4, false},
		{100, false},        // the reference's rejected composite (AMEND-M5)
		{65537, true},        // the default table_size (Fermat prime 2^16+1)
		{5000011, true},      // the PGV cap — verified prime (a faithful cap is itself prime)
		{5000012, false},     // cap+1, composite
		{4999999, false},     // a near-cap composite (= 53 * 94339...; pin via the oracle below)
	}
	for _, c := range cases {
		if got := isPrime(c.n); got != c.want {
			t.Errorf("isPrime(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}
```
> Verify the non-obvious vectors (`5000011` prime, `4999999` composite) with a TRUSTED oracle at IMPL (`python3 -c 'import sympy; print(sympy.isprime(5000011), sympy.isprime(4999999))'` or a throwaway program) — do NOT invent. Record the oracle + results in PROGRESS.md. If `4999999` turns out prime, substitute another pinned near-cap composite.

- [ ] **Step 2: Run to verify it fails**

```bash
cd internal/cluster && go test -run TestIsPrime ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`isPrime` undefined).

- [ ] **Step 3: Implement `isPrime` in `maglev.go`**

Create `internal/cluster/maglev.go` with the package clause + the `isPrime` helper (the type + build + Pick land in Tasks 3–4):
```go
package cluster

// isPrime reports whether n is prime by trial division to sqrt(n). Maglev
// requires a prime table_size so that each per-host skip (in [1, M-1]) is
// coprime to M, giving the populate loop a full M-cycle that fills every slot
// (SPEC §3.1). Adequate for n <= 5000011 (the PGV cap). The reference itself
// rejects a non-prime table_size ("The table size of maglev must be prime
// number", maglev_lb.cc) — envoy-go's reject is reference PARITY (AMEND-M5).
func isPrime(n uint64) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for d := uint64(3); d*d <= n; d += 2 {
		if n%d == 0 {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
cd internal/cluster && go test -run TestIsPrime ./... 2>&1 | tail
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/maglev.go internal/cluster/maglev_test.go
git commit -m "phase 37 Task 2: isPrime(uint64) trial-division helper (maglev.go) — gates table_size primality (reference PARITY, AMEND-M5); vectors incl 65537/5000011 prime, 100/0/1 composite, oracle-pinned near-cap"
```

---

## Task 3: The `maglevLB` table build (`maglev.go`)

**Goal:** Implement `maglevLB` + `maglevCfg` + `newMaglev`/`newMaglevWithRNG` mirroring Envoy v1.37.2's `MaglevTable` EXACTLY (AMEND-M2; SPEC §3.1 verbatim): SORT endpoint indices by `Addr()` ascending (the load-bearing pre-build `std::sort` — omit it and the table differs cross-side), derive per host `offset = xxHash64(addr) % M` + `skip = (xxHash64Seed(addr,1) % (M-1)) + 1`, populate `table[(offset + skip·next) % M]` host-by-host until all `M` slots fill (terminates: `M` prime → each `skip` coprime → a full `M`-cycle). Compute `minPerHost`/`maxPerHost` (= floor/ceil(M/N) under equal weight). REUSE `hash.go`'s `xxHash64`/`xxHash64Seed` (ZERO new hash code) + `newPCGRNG`.

**Files:**
- Modify: `internal/cluster/maglev.go` (add the type + build)
- Modify: `internal/cluster/maglev_test.go` (the build tests)

- [ ] **Step 1: Write the failing tests** (`maglev_test.go`)

```go
func TestMaglev_BuildFullyFilledNoMinusOne(t *testing.T) {
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	if uint64(len(mg.table)) != 65537 {
		t.Fatalf("table len = %d, want 65537", len(mg.table))
	}
	for i, idx := range mg.table {
		if idx < 0 || idx >= len(mg.endpoints) {
			t.Fatalf("table[%d] = %d, want a valid endpoint index (no -1 / out of range)", i, idx)
		}
	}
}

func TestMaglev_MinMaxEntriesPerHost_Default3Host(t *testing.T) {
	// D-M4 / D-M2: 65537 slots over 3 equal hosts → min=floor(65537/3)=21845,
	// max=ceil(65537/3)=21846 (the live reference gauge values).
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	if mg.minPerHost != 21845 || mg.maxPerHost != 21846 {
		t.Errorf("min/max per host = %d/%d, want 21845/21846", mg.minPerHost, mg.maxPerHost)
	}
	// the per-host tallies sum to M (every slot claimed exactly once)
}

func TestMaglev_SortByAddrIsLoadBearing(t *testing.T) {
	// The table layout is determined by the pre-build sort on Addr() (AMEND-M2).
	// Two endpoint SLICES that are permutations of each other must build the SAME
	// table (the build sorts internally) — proving the sort, not slice order, fixes
	// the layout (cross-side determinism over identical address sets).
	a := []Endpoint{{Host: "127.0.0.1", Port: 9003}, {Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}}
	b := []Endpoint{{Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}, {Host: "127.0.0.1", Port: 9003}}
	ma := newMaglevWithRNG(a, maglevCfg{tableSize: 65537}, seqRNG(0))
	mb := newMaglevWithRNG(b, maglevCfg{tableSize: 65537}, seqRNG(0))
	// Compare the tables AS ADDRESS STRINGS (the slices differ in index order, so
	// index equality would be wrong; the picked Addr() per slot must match).
	for i := range ma.table {
		if ma.endpoints[ma.table[i]].Addr() != mb.endpoints[mb.table[i]].Addr() {
			t.Fatalf("table[%d] addr mismatch under permuted input: %q vs %q",
				i, ma.endpoints[ma.table[i]].Addr(), mb.endpoints[mb.table[i]].Addr())
		}
	}
}

func TestMaglev_SmallPrimeHandComputed(t *testing.T) {
	// A tiny M with a hand-checkable property: with M=7 prime and 1 host, every
	// slot maps to host 0 and the table is fully filled (the single-host degenerate
	// build still terminates and fills). Larger hand-vectors are recorded in
	// PROGRESS.md from a reference cross-check at IMPL.
	mg := newMaglevWithRNG(eps(1), maglevCfg{tableSize: 7}, seqRNG(0))
	if len(mg.table) != 7 {
		t.Fatalf("table len = %d, want 7", len(mg.table))
	}
	for i, idx := range mg.table {
		if idx != 0 {
			t.Fatalf("single-host table[%d] = %d, want 0", i, idx)
		}
	}
	if mg.minPerHost != 7 || mg.maxPerHost != 7 {
		t.Errorf("single-host min/max = %d/%d, want 7/7", mg.minPerHost, mg.maxPerHost)
	}
}
```
(`eps(n)` is the existing `*_test.go` endpoint helper [ports start at 1000 — REUSED, do NOT redeclare]; `seqRNG` is REUSED from `random_test.go`/`ringhash_test.go`. Verify with the Task 1 grep.)

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestMaglev ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`maglevLB`/`maglevCfg`/`newMaglevWithRNG` undefined).

- [ ] **Step 3: Implement the type + build** (SPEC §3.1 VERBATIM)

Add to `maglev.go` (the SPEC §3.1 code, verbatim — the struct, `maglevCfg`, the `var _ loadBalancer` assert, `newMaglev`, `newMaglevWithRNG`):
```go
import (
	"sort"
)

// maglevLB is a per-cluster consistent-hash load balancer implementing the
// Maglev lookup-table algorithm, mirroring Envoy v1.37.2's MaglevTable
// (source/extensions/load_balancing_policies/maglev/maglev_lb.cc). The table is
// a fixed-size []int (each slot an endpoint index) built once at construction:
// hosts sorted by Addr() ascending, each with offset = xxHash64(addr) % M and
// skip = (xxHash64Seed(addr,1) % (M-1)) + 1, populated via (offset+skip*next)%M
// until all M slots fill (terminates: M prime => skip coprime to M => full cycle).
// Pick indexes table[hashKey % M]; with hasHash false it draws a random table
// index (rng() % M — the reference random() mirror). ADR-0238 (the maglev
// policy); ADR-0232 (the RELEASE half — reuses noopRelease unchanged);
// ADR-0235 (the PICK-INPUT-half hash key — reused).
type maglevLB struct {
	endpoints []Endpoint
	table     []int // table[slot] = index into endpoints; len == tableSize
	rng       func() uint64
	tableSize uint64

	// Gauge values computed at build (registered by the manager — AMEND-M4).
	minPerHost uint64 // = floor(M/N) under equal weight
	maxPerHost uint64 // = ceil(M/N)  under equal weight
}

// maglevCfg carries the parsed maglev policy parameters (the single table_size).
type maglevCfg struct {
	tableSize uint64
}

// var-assert: maglevLB satisfies the loadBalancer interface.
var _ loadBalancer = (*maglevLB)(nil)

// newMaglev builds a maglevLB seeded from a fresh PCG rng. Like newRingHash it
// accepts newPCGRNG's shared seed-error message verbatim (no wrap).
func newMaglev(endpoints []Endpoint, cfg maglevCfg) (*maglevLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newMaglevWithRNG(endpoints, cfg, rng), nil
}

// newMaglevWithRNG builds the table with an injected rng (tests). The table is
// immutable after construction so Pick is safe for concurrent use.
func newMaglevWithRNG(endpoints []Endpoint, cfg maglevCfg, rng func() uint64) *maglevLB {
	m := cfg.tableSize
	n := len(endpoints)
	if n == 0 {
		return &maglevLB{endpoints: endpoints, rng: rng, tableSize: m}
	}

	// Sort endpoint INDICES by Addr() ascending — Envoy std::sorts the build
	// entries by the key string before populating (maglev_lb.cc:107-120). This
	// determines the build order, hence the table layout (D-M2).
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return endpoints[order[a]].Addr() < endpoints[order[b]].Addr()
	})

	offset := make([]uint64, n)
	skip := make([]uint64, n)
	next := make([]uint64, n)
	count := make([]uint64, n)
	for k, ei := range order {
		key := []byte(endpoints[ei].Addr()) // "IP:PORT" == address()->asString()
		offset[k] = xxHash64(key) % m
		skip[k] = (xxHash64Seed(key, 1) % (m - 1)) + 1
	}

	table := make([]int, m)
	for i := range table {
		table[i] = -1
	}
	var filled uint64
	for filled < m { // equal-weight populate: one slot per host per iteration
		for k := range order {
			if filled == m {
				break
			}
			c := (offset[k] + skip[k]*next[k]) % m
			for table[c] != -1 {
				next[k]++
				c = (offset[k] + skip[k]*next[k]) % m
			}
			table[c] = order[k]
			next[k]++
			count[k]++
			filled++
		}
	}

	minPerHost, maxPerHost := count[0], count[0]
	for _, c := range count[1:] {
		if c < minPerHost {
			minPerHost = c
		}
		if c > maxPerHost {
			maxPerHost = c
		}
	}
	return &maglevLB{
		endpoints:  endpoints,
		table:      table,
		rng:        rng,
		tableSize:  m,
		minPerHost: minPerHost,
		maxPerHost: maxPerHost,
	}
}
```
Imports: `sort` (stdlib; no new go.mod dep). `xxHash64`/`xxHash64Seed`/`newPCGRNG` are already in the package.

> **Build-time guard note:** `newMaglevWithRNG` assumes `cfg.tableSize` is prime and `≥ 2` (so `m-1 ≥ 1` and the cycle terminates). The manager guarantees this via `parseMaglevLbConfig` (Task 5 — cap + primality, default 65537). The tests above always pass a prime `tableSize`. (Defense-in-depth against `m < 2` is unnecessary: the only construction path is through the validated manager parse; the default is 65537.)

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestMaglev ./... 2>&1 | tail
cd internal/cluster && go test -run TestMaglev -race ./... 2>&1 | tail   # the table is immutable post-build; build is single-goroutine
```
Expected: PASS, no race. `TestMaglev_MinMaxEntriesPerHost_Default3Host` green (21845/21846) confirms the byte-faithful build.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/maglev.go internal/cluster/maglev_test.go
git commit -m "phase 37 Task 3: maglevLB table build (ADR-0238) — sort endpoints by Addr() + offset=xxHash64(addr)%M / skip=(xxHash64Seed(addr,1)%(M-1))+1, populate (offset+skip*next)%M until full (M prime => full cycle); REUSES xxHash64/xxHash64Seed (ZERO new hash code); default 3-host build = min/max 21845/21846 (matches live reference D-M4)"
```

---

## Task 4: The `maglevLB.Pick` + the folded property test (`maglev.go`)

**Goal:** Implement `Pick(hashKey, hasHash)`: `table[hashKey % M]`; `hasHash==false` → a uniform random table index (`rng() % M` — the Envoy random-LB fallback); empty-set → `errNoEndpoints`; `noopRelease` on every path (maglevLB holds no per-pick state). Add the folded property test (D-S37-5).

**Files:**
- Modify: `internal/cluster/maglev.go` (add `Pick`)
- Modify: `internal/cluster/maglev_test.go` (the `Pick` tests + the property test)

- [ ] **Step 1: Write the failing tests** (`maglev_test.go`)

```go
func TestMaglev_SameKeySameEndpoint(t *testing.T) {
	// The consistent-hash invariant: the SAME key always picks the SAME endpoint.
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	ep1, _, _ := mg.Pick(0xABCDEF, true)
	ep2, _, _ := mg.Pick(0xABCDEF, true)
	if ep1 != ep2 {
		t.Errorf("same key picked different endpoints: %v vs %v", ep1, ep2)
	}
}

func TestMaglev_PickIndexesTable(t *testing.T) {
	// Pick(hashKey, true) returns exactly endpoints[table[hashKey % M]].
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	for _, hk := range []uint64{0, 1, 65536, 65537, 1 << 40, ^uint64(0)} {
		ep, _, err := mg.Pick(hk, true)
		if err != nil {
			t.Fatalf("Pick(%d): %v", hk, err)
		}
		want := mg.endpoints[mg.table[hk%mg.tableSize]]
		if ep != want {
			t.Errorf("Pick(%d) = %v, want table[%d]=%v", hk, ep, hk%mg.tableSize, want)
		}
	}
}

func TestMaglev_NoHashFallbackUsesRNG(t *testing.T) {
	// hasHash==false draws a table index via rng() % M. With a deterministic rng
	// returning a known value, the pick is endpoints[table[that % M]].
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(7))
	ep, _, err := mg.Pick(0, false) // hashKey ignored; rng()=7 → table[7]
	if err != nil {
		t.Fatal(err)
	}
	if ep != mg.endpoints[mg.table[7]] {
		t.Errorf("no-hash fallback with rng()=7 picked %v, want table[7]=%v", ep, mg.endpoints[mg.table[7]])
	}
}

func TestMaglev_EmptySet(t *testing.T) {
	mg := newMaglevWithRNG(nil, maglevCfg{tableSize: 65537}, seqRNG(0))
	_, release, err := mg.Pick(123, true)
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestMaglev_RandomKeysNeverPanicAlwaysValid(t *testing.T) {
	// D-S37-5: the unit-level property test in lieu of a fuzzer (no untrusted wire
	// decode → no FuzzMaglevLookup). Random keys always yield a valid endpoint,
	// never panic; release is callable.
	mg := newMaglevWithRNG(eps(3), maglevCfg{tableSize: 65537}, seqRNG(0))
	rng := seqRNG(1, 2, 3, 1<<63, ^uint64(0), 0)
	for i := 0; i < 1000; i++ {
		ep, rel, err := mg.Pick(rng(), true)
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Port < 1000 { // eps(n) ports start at 1000
			t.Fatalf("pick %d: invalid endpoint %v", i, ep)
		}
		rel()
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestMaglev ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`Pick` method undefined on `*maglevLB`).

- [ ] **Step 3: Implement `Pick`** (SPEC §3.1 VERBATIM)

Add to `maglev.go`:
```go
// Pick selects table[hashKey % M]. With hasHash false it draws a random table
// index (rng() % M — the Envoy random()-LB fallback; near-uniform, never on the
// differential path). The release func is noopRelease (maglevLB holds no
// per-pick state); non-nil on every path.
func (mg *maglevLB) Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error) {
	if len(mg.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if !hasHash {
		hashKey = mg.rng()
	}
	return mg.endpoints[mg.table[hashKey%mg.tableSize]], noopRelease, nil
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestMaglev -race ./... 2>&1 | tail
```
Expected: PASS, no race (the table is immutable; `Pick` is read-only; the rng is mutex-guarded via the REUSED `newPCGRNG`).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/maglev.go internal/cluster/maglev_test.go
git commit -m "phase 37 Task 4: maglevLB.Pick — table[hashKey % M] + no-hash rng() % M fallback + empty-set errNoEndpoints + noopRelease (REUSED, ADR-0232); folded random-keys property test in lieu of a fuzzer (D-S37-5; fuzzers stay 42)"
```

---

## Task 5: Manager acceptance + the table_size gate + the doubly-hit retarget

**Goal:** `buildCluster` accepts `MAGLEV` via `parseMaglevLbConfig` (self-supplied 65537 default; the PGV cap `≤ 5000011` then the primality gate — AMEND-M5) + `newMaglev`; the `default` reject text extends to `(…, RING_HASH, MAGLEV)`; `TestManager_Error_UnsupportedLBPolicy` retargets MAGLEV → `Cluster_CLUSTER_PROVIDED` + re-pins the new substring; the mismatched-oneof under MAGLEV is silently ignored.

**Files:**
- Modify: `internal/cluster/manager.go` (the `switch` at :243; the reject text at :275; `parseMaglevLbConfig`)
- Modify: `internal/cluster/manager_test.go` (accept + cap/primality/mismatched-oneof arms; the doubly-hit retarget)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestManager_Accept_Maglev_Defaults(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_MAGLEV // no maglev_lb_config → default table_size 65537
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("MAGLEV bare must be accepted: %v", err)
	}
}

func TestManager_Accept_Maglev_NonDefaultPrime(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(127)}} // 127 is prime, <= cap
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("valid prime table_size must be accepted: %v", err)
	}
}

func TestManager_Reject_Maglev_TableSizeOverCap(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(5000012)}} // > cap
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "table_size: value must be less than or equal to 5000011") {
		t.Errorf("err = %v, want PGV cap reject", err)
	}
}

func TestManager_Reject_Maglev_TableSizeNotPrime(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(100)}} // composite — the reference rejects (AMEND-M5)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "must be a prime number") {
		t.Errorf("err = %v, want primality reject", err)
	}
}

func TestManager_Accept_Maglev_MismatchedOneof(t *testing.T) {
	// A stray ring_hash_lb_config under MAGLEV → silent-ignore (reference parity
	// §6.3; the manager reads GetMaglevLbConfig() only on the MAGLEV path).
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(64)}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under MAGLEV must be silently accepted (default table_size): %v", err)
	}
}
```
Then RETARGET `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320`; trigger CURRENTLY `Cluster_MAGLEV`:322 — now ACCEPTED): change the trigger to `Cluster_CLUSTER_PROVIDED` and extend the pinned substring to `"ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV"`:
```go
func TestManager_Error_UnsupportedLBPolicy(t *testing.T) {
	c := mkStaticCluster("c_x", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED // MAGLEV now accepted → retarget to the next still-rejected policy (AMEND-M5)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("CLUSTER_PROVIDED must be rejected")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV") {
		t.Errorf("error %q missing new supported-set substring (…, MAGLEV)", err.Error())
	}
}
```
> **Both retarget edits are load-bearing** (the phase-34/35/36 doubly-hit lesson): the trigger change is required because `Cluster_MAGLEV` no longer errors; the substring extension is required so the assertion pins the NEW text (`"…, RING_HASH"` is still a substring of `"…, RING_HASH, MAGLEV"`, so without extending it the test would pass against the OLD text — vacuous w.r.t. the change).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestManager_Accept_Maglev|TestManager_Reject_Maglev|TestManager_Error_UnsupportedLBPolicy" ./... 2>&1 | head -40
```
Expected: the accept/reject tests FAIL (current `default` rejects MAGLEV); `TestManager_Error_UnsupportedLBPolicy` FAILS (MAGLEV now accepted → must be CLUSTER_PROVIDED; old substring lacks `, MAGLEV`).

- [ ] **Step 3: Implement the manager change** (SPEC §3.3 VERBATIM)

Add `parseMaglevLbConfig` (the `parseRingHashLbConfig` precedent; `GetMaglevLbConfig()` nil-safe → default):
```go
// Maglev table-size default / cap (proto doc-comment default; PGV lte cap — §6).
const (
	defaultMaglevTableSize = 65537   // doc-comment default (self-supplied; nil getter)
	maglevTableSizeCap     = 5000011 // PGV: value must be less than or equal to 5000011
)

// parseMaglevLbConfig extracts the table_size for a MAGLEV cluster.
// GetMaglevLbConfig() is nil-safe (nil on an absent OR a mismatched lb_config
// oneof member): both fall to the self-supplied default (§6.3 silent-ignore
// parity). The gate is hand-rolled (the manager calls no PGV): the PGV cap
// mirror (<= 5000011) THEN the primality check (reference parity — the reference
// throws "The table size of maglev must be prime number"; AMEND-M5).
func parseMaglevLbConfig(c *clusterv3.Cluster, name string) (maglevCfg, error) {
	cfg := maglevCfg{tableSize: defaultMaglevTableSize}
	mlc := c.GetMaglevLbConfig()
	if mlc == nil {
		return cfg, nil // absent OR mismatched oneof → default (§6.3 silent-ignore parity)
	}
	if v := mlc.GetTableSize(); v != nil {
		ts := v.GetValue()
		if ts > maglevTableSizeCap {
			return maglevCfg{}, fmt.Errorf("cluster: %q: maglev_lb_config.table_size: value must be less than or equal to 5000011", name)
		}
		if !isPrime(ts) {
			return maglevCfg{}, fmt.Errorf("cluster: %q: maglev_lb_config.table_size (%d) must be a prime number", name, ts)
		}
		cfg.tableSize = ts
	}
	return cfg, nil
}
```
The switch case (between `case Cluster_RING_HASH` and `default`):
```go
case clusterv3.Cluster_MAGLEV: // phase 37 (ADR-0238): Maglev consistent-hash table
	cfg, err := parseMaglevLbConfig(c, name)
	if err != nil {
		return nil, err
	}
	lb, err := newMaglev(endpoints, cfg)
	if err != nil {
		return nil, err
	}
	cl.lb = lb
default:
	return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV)", name, c.GetLbPolicy())
```
> The cap check precedes the primality check (the reference order: the PGV proto-constraint fires before the app-layer `isPrime` throw — AMEND-M5). The default 65537 (a known prime) bypasses both checks. Confirm the exact `endpoints`/`cl`/`name` variable names against the as-built `case Cluster_RING_HASH` block (Task 1 anchor) — match them verbatim.

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20   # the full package, incl. the unchanged incumbent + ring_hash tests
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 37 Task 5: manager accepts MAGLEV — parseMaglevLbConfig (default 65537 self-supplied; PGV cap <=5000011 THEN primality gate, reference PARITY AMEND-M5); NEW reject text (…, RING_HASH, MAGLEV); doubly-hit retarget (MAGLEV→CLUSTER_PROVIDED + substring); mismatched-oneof silent-ignore"
```

---

## Task 6: The 2 `maglev_lb.*` gauge registrations

**Goal:** Register + Set the 2 static gauges `cluster.<name>.maglev_lb.min_entries_per_host` + `max_entries_per_host` ONCE at `registerClusterMetrics` via a `*maglevLB` type-assert (mirroring the `*ringHashLB` block at `manager.go:110`) — MAGLEV-only (reference parity; D-M4). NO `size` gauge (the table size is config-known). Surface 1119 → 1121.

**Files:**
- Modify: `internal/cluster/manager.go` (the `registerClusterMetrics` type-assert)
- Modify: `internal/cluster/manager_test.go` (the gauge-registration assertion + the non-MAGLEV-no-gauges assertion)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestManager_Maglev_RegistersGauges(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// 65537 slots over 3 hosts → 21845 / 21846 (D-M4); NO maglev_lb.size gauge.
	for name, want := range map[string]int64{
		"cluster.c_mg.maglev_lb.min_entries_per_host": 21845,
		"cluster.c_mg.maglev_lb.max_entries_per_host": 21846,
	} {
		got, ok := gaugeValue(reg, name) // the REUSED ring_hash readback helper
		if !ok {
			t.Errorf("gauge %q not registered", name)
		} else if got != want {
			t.Errorf("gauge %q = %d, want %d", name, got, want)
		}
	}
	if _, ok := gaugeValue(reg, "cluster.c_mg.maglev_lb.size"); ok {
		t.Error("maglev must register NO size gauge (the table size is config-known — D-M4)")
	}
}

func TestManager_NonMaglev_NoGauges(t *testing.T) {
	// the gauges are MAGLEV-only (reference parity) — a ROUND_ROBIN cluster registers none.
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 9001))
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	if _, ok := gaugeValue(reg, "cluster.c_rr.maglev_lb.min_entries_per_host"); ok {
		t.Error("ROUND_ROBIN cluster must register NO maglev_lb gauges (MAGLEV-only)")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestManager_Maglev_RegistersGauges|TestManager_NonMaglev_NoGauges" ./... 2>&1 | head
```
Expected: FAIL (`min_entries_per_host`/`max_entries_per_host` not registered).

- [ ] **Step 3: Implement the gauge registration** (mirror the `*ringHashLB` block at `manager.go:110-114`)

In `registerClusterMetrics`, after the existing `if rh, ok := c.lb.(*ringHashLB); ok { … }` block:
```go
	if mg, ok := c.lb.(*maglevLB); ok {
		// AMEND-M4: 2 mirrored static gauges, set once at register (the table is
		// immutable). MAGLEV-only (reference parity); cross-side-exact (floor(M/N)/
		// ceil(M/N), keyed on table_size + host count + weights, address-independent).
		// NO size gauge — the table size is config-known (D-M4).
		r.NewGauge(prefix + "maglev_lb.min_entries_per_host").Set(int64(mg.minPerHost))
		r.NewGauge(prefix + "maglev_lb.max_entries_per_host").Set(int64(mg.maxPerHost))
	}
```
> Match the as-built `prefix`/`r` variable names + the `.NewGauge(...).Set(...)` call shape against the `*ringHashLB` block (Task 1 anchor) verbatim.

- [ ] **Step 4: Run to verify they pass + a boot smoke**

```bash
cd internal/cluster && go test ./... 2>&1 | tail
go build ./... && echo BUILD_OK
```
Expected: PASS; the whole repo builds.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 37 Task 6: register 2 maglev_lb.* gauges (min/max_entries_per_host) MAGLEV-only via *maglevLB type-assert (the *ringHashLB precedent); set once at build; NO size gauge (config-known, D-M4); surface 1119 -> 1121"
```

---

## Task 7: The `0063-lb-maglev` differential fixture

**Goal:** A cross-side `[http_connection_manager + router]` fixture over a 3-endpoint MAGLEV cluster with `maglev_lb_config: {}` (default 65537) + a route-level `hash_policy: [{header: {header_name: "x-hash"}}]` on BOTH sides (the ADR-0237 producer, REUSED). N=16 distinct `X-Hash` values × K=16 = 256 routed `GET /get` + 8 `/health`. Proven via the `0062` BOTH-SIDE modular invariant (every per-backend count `≡ 0 mod K` = affinity; spread `≥ 2`; conservation 256) + a cross-side `StatsAsserter` (the 2 gauges cross-equal + cx/rq/membership/quiesced). NO new BackendKind (reuses `HTTPEcho`). NO boot-reject dir.

**Files:**
- Create: `test/fixtures/0063-lb-maglev/driver/driver.go`
- Create: `test/fixtures/0063-lb-maglev/driver/driver_test.go`
- Create: `test/fixtures/0063-lb-maglev/README.md`
- Create: `test/fixtures/0063-lb-maglev/expectations.yaml`

- [ ] **Step 1: Write the driver** (`driver/driver.go`)

Start from a VERBATIM copy of `test/fixtures/0062-lb-ring-hash-http/driver/driver.go`, then:
1. `fixtureName = "0063-lb-maglev"`; pick `refContainerListenerPort = 19152` (the next free value — `0062` uses 19151; confirm free via `grep -rh "refContainerListenerPort\s*=" test/fixtures/*/driver/driver.go`).
2. Rename the driver type (`ringHashHTTPDriver` → `maglevDriver`); update the `init()` registration, all receivers, and the compile-time interface checks (`var _ fixture.Driver = (*maglevDriver)(nil)` etc.).
3. In BOTH `ReferenceBootstrap` and `SubjectConfig`: change `lb_policy: RING_HASH` → `lb_policy: MAGLEV` and `ring_hash_lb_config: {}` → `maglev_lb_config: {}` (default 65537). Keep the route-level `hash_policy: [{header: {header_name: "x-hash"}}]`, `BackendCount() == 3`, `SubjectListenerName() == "l_http"`, the `/health` `direct_response` `inline_string: "OK\n"`, the STRICT_DNS (reference, `host.docker.internal`) / STATIC `127.0.0.1` (subject) split, and `clusterName = "c_echo"`.
4. Keep the workload constants `hashValues = 16` (N) / `repeatPerVal = 16` (K) / `totalReqs = hashValues * repeatPerVal` (DERIVED — `reference_fixture_workload_constant_desync`) / `healthReqs = 8`, and the `drive`/`DriveReference`/`DriveSubject`/`ProbeAdmin` bodies VERBATIM (the X-Hash header workload is policy-agnostic — it feeds the same key to MAGLEV that it fed to RING_HASH).

- [ ] **Step 2: Keep the affinity+spread `AssertDistribution`** (the `0062` modular invariant — UNCHANGED)

The `0062` `AssertDistribution` (the `% repeatPerVal` affinity + spread `≥ 2` + conservation `== totalReqs`, BOTH sides) ports VERBATIM — the consistent-hash invariant (one X-Hash value → one digest → one table slot → one backend) holds identically for a Maglev table. Update only the doc comment's "ring point" → "table slot" and "ring collapsed" → "table collapsed". (Re-confirm `len(...) != 3` guard + the BOTH-SIDE loop are intact.)

- [ ] **Step 3: Rewrite the `StatsAsserter`'s gauge prong** (the §7 set — the 2 maglev gauges replace the 3 ring_hash gauges)

In `AssertStats`, replace the 3 `ring_hash_lb.*` cross-equal entries with the 2 `maglev_lb.*` entries (SPEC §7); keep `upstream_cx_total`/`upstream_rq_total`/`membership_total`/`upstream_cx_active`:
```go
	}{
		{pfx + "upstream_cx_total", totalReqs},
		{pfx + "upstream_rq_total", totalReqs},
		{pfx + "membership_total", 3},
		{pfx + "upstream_cx_active", 0},
		{pfx + "maglev_lb.min_entries_per_host", 21845}, // floor(65537/3) — cross-side-exact (D-M4)
		{pfx + "maglev_lb.max_entries_per_host", 21846}, // ceil(65537/3)
	} {
```
Update the `AssertStats` doc comment (the 2 maglev gauges depend ONLY on `table_size` + host count, NOT addresses → cross-equal). Keep `scrapeStats` verbatim.

- [ ] **Step 4: Write README.md + expectations.yaml + driver_test.go**

- `README.md`: the X-Hash header workload (N=16 × K=16 = 256 + 8 `/health`); the BOTH-SIDE affinity invariant (each per-backend count `≡ 0 mod 16` — the consistent-hash one-value-one-backend property over a Maglev TABLE) + spread (`≥ 2`); the NAT-transparency rationale (the X-Hash header survives the Docker hop verbatim — both sides satisfy the invariant); the cross-side host-identity-INFEASIBLE-but-affinity-provable posture (the tables are over different `"IP:PORT"` strings — `reference_differential_hash_key_cross_side_infeasible`); the 2 cross-equal `maglev_lb.*` gauges (21845/21846; the strong maglev-specific prong); the `upstream_rq_total` cross-equal (the HTTP plane Inc's it both sides). The firsts/expectations: SECOND consistent-hash fixture; the SECOND non-zero LB-stat delta (+2 gauges → surface 1121); NO new BackendKind (reuses `HTTPEcho`, tail STAYS 33); NO new fuzzer (a table property test is unit-level). The Task-8 deliberate-break records (added at Task 8).
- `expectations.yaml`: mirror the `0062` shape (cross-side byte-equivalence of the `/health` `OK\n` stream + the asserter dispatch; document the affinity/spread + the 2-gauge stats prongs).
- `driver_test.go`: the registration smoke (mirror `0062`'s `driver_test.go`, renamed).

- [ ] **Step 5: Run the fixture (requires Docker + the contrib reference image)**

```bash
go test ./test/differential/ -run 'TestDifferential/0063' -count=1 -v 2>&1 | tail -40
```
Expected: PASS — byte-equivalence + the BOTH-SIDE affinity/spread/conservation + the cross-side stats (incl. the 2 gauges). **Use `-run 'TestDifferential/0063'`, NOT `-run '0063'`** (`reference_differential_run_selector`). Verify the decode ran (`downstream_cx_rx_bytes_total > 0` per `reference_docker_probe_bridge_network`). If Docker is unavailable in the IMPL environment, the controller runs this gate where Docker is present.

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0063-lb-maglev/ && golangci-lint run ./test/...
git add test/fixtures/0063-lb-maglev/
git commit -m "phase 37 Task 7: 0063-lb-maglev differential fixture — route header hash_policy → MAGLEV (maglev_lb_config: {} default 65537), N=16 x K=16 = 256 GETs + 8 /health; BOTH-SIDE affinity (per-backend count ≡ 0 mod 16) + spread (>=2) + conservation 256 (the 0062 modular invariant, retargeted); cross-side StatsAsserter + 2 maglev_lb gauges (21845/21846); reuses HTTPEcho (BackendKind tail 33)"
```

---

## Task 8: Deliberate-break liveness (`-count=1`) + ≥20-run flake check

**Goal:** PROVE each `0063` prong is live (`reference_differential_break_protocol_count1` — go test caching serves a stale PASS otherwise) and flake-free. **Name the collapse break PRECISELY** (build-level vs `Pick`-level) so the gauge prong is non-vacuous (`reference_differential_asserter_dispatch`).

**Files:** (break/revert only — no committed production change; record in README.md + PROGRESS.md). **GIT HYGIENE** (`feedback_subagent_worktree_detach`): use `git restore` to revert each break — never `git checkout <sha>`/`--amend`; the controller re-verifies the branch after this task.

- [ ] **Step 1: Repeat-run flake check (≥20)**

```bash
go test ./test/differential/ -run 'TestDifferential/0063' -count=20 2>&1 | tail
```
Expected: 20/20 PASS. The affinity leg is DETERMINISTIC (the table is fixed; the X-Hash keys are fixed) — it should NEVER flake. The spread leg (`≥ 2`) is overwhelmingly stable (16 distinct X-Hash values over 3 backends; `P(all 16 → 1 backend) ≈ 3·(1/3)^16 ≈ 7e-8` per side — the `0062` N=16 margin). If spread ever flakes, record and investigate; do NOT widen below `≥ 2`.

- [ ] **Step 2: Deliberate break (i) — scatter the key → affinity FAILS**

In `maglevLB.Pick`, force a random draw ignoring the key (replace the `if !hasHash { hashKey = mg.rng() }` guard so it ALWAYS draws: `hashKey = mg.rng()` unconditionally). Run:
```bash
go test ./test/differential/ -run 'TestDifferential/0063' -count=1 2>&1 | tail
```
Expected: FAIL on `affinity: backend[i]=N not a multiple of 16` (per-request random scatter → counts not multiples of 16). `git restore` the file.

- [ ] **Step 3: Deliberate break (ii) — collapse the BUILT table → spread AND gauges FAIL**

In `newMaglevWithRNG`, AT BUILD force every slot to host 0 (after the populate loop: `for i := range table { table[i] = order[0] }` AND set `count[order[0]] = m`, `count[others] = 0` before the min/max tally — so the gauges go `min=0`/`max=65537`). Run `-count=1`:
```bash
go test ./test/differential/ -run 'TestDifferential/0063' -count=1 2>&1 | tail
```
Expected: FAIL on BOTH the `spread: only 1 backend(s) nonzero` leg AND the `maglev_lb.max_entries_per_host` cross-equal (subject 65537 != want 21846). This is the BUILD-level collapse — it bites the gauge prong. `git restore` the file.

> **Contrast (the precise-naming lesson, `reference_differential_asserter_dispatch`):** a WEAKER `Pick`-level short-circuit (`return mg.endpoints[mg.table[0]], …` WITHOUT touching the build) would bite the spread leg ONLY — the table is still built normally → the gauges STAY 21845/21846, so the gauge assertion would NOT fire. Use the BUILD-level collapse above to prove the gauge prong is live; a `Pick`-level collapse proves only spread. Record both in PROGRESS.md.

- [ ] **Step 4: Deliberate break (iii) — stats prong (cross-equal)**

Temporarily corrupt one cross-equal want (e.g. `upstream_cx_total` `totalReqs` → `totalReqs+1` in `AssertStats`) — confirm the `StatsAsserter` FAILS with `-count=1`. `git restore` the file.

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the three break results (with the build-vs-Pick collapse contrast) + the 20/20 flake check in README.md (driver comments) + PROGRESS.md (the `0030` dead-assertion lesson).
```bash
go test ./test/differential/ -run 'TestDifferential/0063' -count=1 2>&1 | tail   # confirm green after all restores
git status --short   # confirm clean (no stray break left in the tree)
git add test/fixtures/0063-lb-maglev/ docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md
git commit -m "phase 37 Task 8: 0063 deliberate-break liveness (-count=1) — scatter-key [affinity fails], collapse-table-AT-BUILD [spread + maglev_lb gauge prongs fail], stats-corrupt [stats fails]; 20/20 flake-free (affinity deterministic); build-vs-Pick collapse contrast recorded"
```

---

## Task 9: Full differential re-verify + race + conformance unaffected

**Goal:** Prove the new manager case kept all 64 prior fixtures byte-exact (the seam is UNCHANGED → behavior-neutrality is structural) and `0063` is green; run `-race`; assert h2spec/proxy-wasm unaffected; confirm zero new go.mod dep.

**Files:** none (verification only)

- [ ] **Step 1: Full differential suite (65 dirs)**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
Expected: ALL 65 PASS — the 64 prior dirs byte-exact through the new `case Cluster_MAGLEV` (the seam + producers are UNCHANGED; only an unconfigured-by-them new switch arm was added) + `0063` green. **This is the six-gate REAL guard** (`reference_docker_probe_bridge_network` — the controller runs where Docker is present).

- [ ] **Step 2: Race + short across the repo**

```bash
go test -race -short ./... 2>&1 | tail -20
```
Expected: PASS, no race (the table is immutable post-build; `Pick` is read-only; the no-hash-fallback rng is mutex-guarded via the REUSED `newPCGRNG`).

- [ ] **Step 3: Build + vet + gofmt + lint (full repo) + tidy**

```bash
go build ./... && go vet ./... && gofmt -l internal/ test/fixtures/0063-lb-maglev/ && golangci-lint run ./... 2>&1 | tail
go mod tidy -diff && echo "TIDY_CLEAN"   # ZERO new go.mod dep (AMEND-M1 — the enum + config are in the pinned v1.32.4)
```
Expected: clean; `go mod tidy -diff` empty.

- [ ] **Step 4: Conformance unaffected**

Re-run (or assert-unaffected with rationale: phase 37 touches `internal/cluster` only — no HTTP/h2/proxy-wasm path; the wire path is byte-identical when no `lb_policy: MAGLEV` cluster is configured) h2spec **53/53** + proxy-wasm **10/10** per the repo's conformance recipe. Record in PROGRESS.md.

- [ ] **Step 5: Commit (LOCAL-ONLY) — verification evidence into PROGRESS.md**

```bash
git add docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md
git commit -m "phase 37 Task 9: six-gate evidence — 65-dir differential byte-exact (64 prior unchanged through the new MAGLEV case + 0063), -race -short green, go mod tidy clean (zero new dep), h2spec 53/53 + proxy-wasm 10/10 unaffected"
```

---

## Task 10: Completion bundle (ADR-0052 atomic landing)

**Goal:** Land the BEHAVIOR_CONTRACT delta, the full ADR-0238 entry, and the STATE/ROADMAP advance — atomically with the code.

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/37-load-balancer-maglev/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md (SPEC §9)**

Add a `### Load balancer — maglev (MAGLEV)` subsection beside the ring_hash entry: the MAGLEV acceptance (the `table_size` default 65537 + the cap + the primality gate — now PARITY, the reference rejects `The table size of maglev must be prime number`); the Maglev table semantics (sort-by-`Addr()` + offset/skip + the populate loop + `table[hashKey % M]`; the no-hash random fallback; the unused `attempt` retry boundary); the seam + producer REUSE (ADR-0235/ADR-0237 unchanged); the healthy-set boundary (no health checking → all-hosts table). Add the 2 `maglev_lb.{min,max}_entries_per_host` gauges to the `## Stat-name mapping` table (MAGLEV-only; Set once at table build; cross-side-exact `floor(M/N)`/`ceil(M/N)`; NO `size` gauge). Update the reject-text line (MAGLEV retired from the rejected set → the FIFTH accepted policy; supported-list `…, RING_HASH, MAGLEV`; CLUSTER_PROVIDED becomes the lone recorded LB-policy departure / retarget trigger). Update the deferred-LB-family list (maglev retired; **4** candidates remain {subset LB, locality-weighted LB, priority load balancing, panic thresholds}). Record the mismatched-oneof silent-ignore under MAGLEV (parity) + the cross-side host-identity-infeasible-but-affinity-provable posture. Update the stat-surface doc count **1119 → 1121**.

- [ ] **Step 2: DECISIONS.md — the full ADR-0238 entry (ADR-0044 in-place)**

Author ADR-0238 (the maglev policy): promote the SPEC §13 §Context DRAFT verbatim (status PROPOSED → ACCEPTED); §Decision (the fixed-size `maglevLB` table; the sort-by-`Addr()` + offset/skip + the populate loop; `Pick(hashKey, hasHash)` = `table[hashKey % M]` + the no-hash rng fallback; the hand-rolled cap + primality gate; the bare manager case + the reject-text change [MAGLEV → CLUSTER_PROVIDED retarget]; ZERO new hash code [reuses `xxHash64`/`xxHash64Seed`]; the seam + producer REUSE unchanged — NO seam ADR, NO producer ADR, the phase-35 single-ADR-on-reuse shape); §Consequences (the seam's FIRST hash-policy reuse — validates ADR-0235's "the durable asset maglev reuses unchanged"; the SECOND consistent-hash policy + SECOND non-zero LB-stat delta [+2 gauges → 1121]; cross-side host IDENTITY INFEASIBLE — the documented harness boundary; ADR-0024 UNAMENDED [no per-cluster counter state]; NO new fuzzer/BackendKind/dep; the Load-balancing family stays OPEN [4 candidates remain]). Tail advances **ADR-0237 → ADR-0238**; next-free **ADR-0239**. (Use ADR-0236 as the STYLE TEMPLATE.)

- [ ] **Step 3: STATE.md + ROADMAP.md**

STATE active-phase → `phase 37 (load-balancer-maglev) IMPL done`; lifecycle-state → the 37-done routing (next → the next family-row brainstorm or the next-prompt's onward routing). Counts: fixtures 64 → **65**; stat surface **1119 → 1121**; fuzzers **42**; BackendKind tail **33**; DECISIONS tail **ADR-0238**. ROADMAP row 37 `in-progress → done` (a flat family row — NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (4 candidates remain after 37).

- [ ] **Step 4: Finalize PROGRESS.md** — all 10 tasks complete; the six-gate evidence; the D-S37-1..5 resolutions; the three deliberate-break records (with the build-vs-Pick collapse contrast); the surface 1119 → 1121 / fixtures 64 → 65 deltas.

- [ ] **Step 5: Final six-gate re-run + commit (LOCAL-ONLY)**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./... && go test -race -short ./... 2>&1 | tail
go test ./test/differential/ -count=1 2>&1 | tail
git add docs/envoy-go/
git commit -m "phase 37 Task 10: completion bundle — BEHAVIOR_CONTRACT maglev delta (surface 1119->1121); ADR-0238 (maglev policy) full entry (ADR-0044 in-place; tail → ADR-0238); STATE/ROADMAP row 37 done (fixtures 64->65); the SECOND consistent-hash policy + the seam's FIRST hash-policy reuse"
```

- [ ] **Step 6: Controller stage-close** — the controller squash-merges the phase-37 IMPL branch to master, runs the final six-gate on master, and (per `feedback_push_to_origin`, tests green) pushes to origin. Subagents NEVER push (`feedback_subagents_no_push`).

---

## Exit criteria (ADR-0052 atomic landing)

- stat surface **1119 → 1121** (+2 `maglev_lb.{min,max}_entries_per_host` gauges — the SECOND non-zero LB-stat delta; AMEND-M4; NO `size` gauge).
- differential fixtures **64 → 65** (`0063-lb-maglev`; NO boot-reject dir — AMEND-M5).
- fuzzers **42** (NO new fuzzer — maglev decodes no untrusted wire bytes; the property test is unit-level, D-S37-5); BackendKind tail **33** (NO new BackendKind — `HTTPEcho` reused).
- DECISIONS tail **ADR-0237 → ADR-0238** (the maglev policy; full entry at this IMPL per ADR-0044; next-free ADR-0239).
- All 65 differential dirs byte-exact (64 prior unchanged through the new manager case + `0063` green); `-race -short` green; h2spec 53/53 + proxy-wasm 10/10 unaffected; `go mod tidy -diff` empty (ZERO new go.mod dep).
- ZERO new exported symbols (the seam + producers are REUSED unchanged; the existing exported `Cluster` surface byte-stable); ZERO new packages; ZERO new hash code.
- ROADMAP row 37 `done` (flat family row — NO parent rollup); the Load-balancing family OPEN (4 candidates remain: subset LB, locality-weighted LB, priority load balancing, panic thresholds).
