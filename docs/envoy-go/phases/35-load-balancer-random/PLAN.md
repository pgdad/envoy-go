# Phase 35 Implementation Plan — `random` load balancer (`Cluster.LbPolicy RANDOM`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.LbPolicy RANDOM` (`envoy.config.cluster.v3`, enum value 3) — the project's THIRD LB policy (after `roundRobin` [02] and `leastRequest` [34]) — as a STATELESS uniform-pick LB (`rng() % len(endpoints)`, ONE draw, INSENSITIVE to load) that REUSES the ADR-0232 LB acquire/release seam UNCHANGED, returning the shared `noopRelease`.

**Architecture:** A new `randomLB` type (`internal/cluster/random.go`, same package) holds only `endpoints []Endpoint` + an injectable `rng func() uint64` — NO `active` slice, NO `sync.Once` release. `Pick()` draws ONE `rng() % n`, returns `endpoints[i]` + the shared `noopRelease` + nil (empty set → `Endpoint{}, noopRelease, errNoEndpoints`). The RNG is the EXISTING package-private `newPCGRNG()` helper (`leastrequest.go:63`), called DIRECTLY with NO extraction; `newRandomWithRNG(endpoints, rng)` mirrors `newLeastRequestWithRNG` minus `choiceCount`. `Manager.buildCluster` gains ONE `case clusterv3.Cluster_RANDOM` (bare construction — NO `parseRandomLbConfig`, RANDOM has no config message); the `default` reject text extends `(supported: ROUND_ROBIN, LEAST_REQUEST)` → `(…, RANDOM)`. The ADR-0232 seam, `cluster.go`, and every consumer are UNTOUCHED (random consumes the seam exactly as `roundRobin` does — the seam's FIRST reuse, validating its "the non-keyed RANDOM drops in at zero cost" §Consequences claim).

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `Cluster.LbPolicy RANDOM` already in the pinned module; **ZERO new go.mod dep**); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); `math/rand/v2` + `crypto/rand` (stdlib — the REUSED injectable PCG RNG). The differential proof is the ANTI-SKEW arm: a band-based per-side `AssertDistribution` on the REUSED `0059` held-conn stimulus asserting RANDOM STAYS uniform despite the load imbalance (the contrapositive of least_request) + a cross-side `StatsAsserter` prong (`0060-lb-random`).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/35-load-balancer-random/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-R1..R6; §3 the `randomLB` design + the seam REUSE; §5/§6 the proto + reject matrices; §7 the zero-delta stat surface; §8 the `0060` anti-skew band-arm design; §10 the ~4–7-task spine; §11 the D-R empirical pins; §12 the D-S35 questions; §13 the ADR-0234 §Context DRAFT.
- **BRAINSTORM:** `docs/envoy-go/phases/35-load-balancer-random/BRAINSTORM.md` — the charter (Q0/Q1/Q-seam/Q-split).
- **As-built REUSE sites:** `internal/cluster/loadbalancer.go` (the `loadBalancer` interface `Pick() (Endpoint, func(), error)`:15-17 + the shared `noopRelease`:21 + `roundRobin.Pick`:34 — the zero-churn posture random copies), `internal/cluster/leastrequest.go:63` (`newPCGRNG` — REUSED verbatim; `newLeastRequestWithRNG`:49 — the constructor `newRandomWithRNG` mirrors minus `choiceCount`), `internal/cluster/manager.go:234` (the `lb_policy` switch) + `:252` (the reject text), `internal/cluster/manager_test.go:320` (`TestManager_Error_UnsupportedLBPolicy` — the DOUBLY-hit retarget: trigger `Cluster_RANDOM`:322 + substring `"ROUND_ROBIN, LEAST_REQUEST"`:327).
- **Differential harness:** the REUSED `0059` fixture `test/fixtures/0059-lb-least-request/{driver/driver.go,driver/driver_test.go,README.md,expectations.yaml}` — `0060` transposes it with the assertion INVERTED (anti-skew band) and the `least_request_lb_config` dropped. The `DistributionAsserter` / `StatsAsserter` hooks + `acceptEchoCounting` (STREAMING echo) + `TCPEcho` BackendKind 0 are REUSED unchanged.

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` — subagent-driven execution.
- `feedback_git_worktrees` — this PLAN was authored in worktree `.worktrees/phase-35-plan`; the IMPL runs in its own worktree.
- `feedback_subagents_no_push` — **subagents commit LOCAL-ONLY**; the controller squash-merges + pushes at stage-close.
- `feedback_pertask_gofmt_lint` — **every task** runs `gofmt -l` + `golangci-lint run` on the touched packages (not just `go vet`).
- `feedback_subagent_worktree_path_targeting` — all paths below are repo-root-relative; the IMPL worktree is the canonical checkout; the controller verifies the main checkout stays clean.
- `reference_differential_break_protocol_count1` — every new differential assertion is proven live by a deliberate-break with `-count=1` (go test caching serves a stale PASS otherwise). **Generalized to BAND assertions here: a band wide enough to never fail is a dead assertion.**
- `reference_differential_asserter_dispatch` — the stats prong uses `StatsAsserter` (cross-side path); the distribution prong uses `DistributionAsserter` (driver-side, runs on both paths).
- `reference_differential_run_selector` — targeted runs use `-run 'TestDifferential/0060'`, NEVER `-run '0060'` (which matches zero subtests → vacuous green).
- ADR-0024 (per-cluster LB counter scope — UNAMENDED; random holds no per-cluster counter state), ADR-0045 (the split-gate — FINAL re-check below), ADR-0044 (ADR bodies at the IMPL, in-place), ADR-0052 (the atomic-landing six-gate), ADR-0080 (byte-stable reject text, substring-pinned by table tests), ADR-0106 (flat family row — NO parent rollup), ADR-0060 (histograms deferred), ADR-0227 (the contrib reference image; live probes per `reference_docker_probe_bridge_network`).

## D-question resolutions (SPEC §12)

- **D-S35-1 (file placement):** RESOLVED as anticipated — the `randomLB` type + `newRandom`/`newRandomWithRNG` land in a NEW sibling `random.go`; the manager `case` + the reject-text edit land in `manager.go`; the doubly-hit retarget lands in `manager_test.go`. (The `leastrequest.go` precedent.)
- **D-S35-2 (RNG message):** RESOLVED — `newRandom` calls `newPCGRNG()` **directly** and **accepts the shared seed-error message** (`"cluster: least_request: seed rng"`); NO wrapper. Rationale: the error is reachable ONLY on a `crypto/rand` read failure (effectively unreachable on Linux `getrandom` → boot-fail either way); a random-flavored wrapper would either double-wrap the message (`"cluster: random: seed rng: cluster: least_request: seed rng: …"` — ugly) or require editing `newPCGRNG` (which the SPEC forbids — AMEND-R5 "needs NO change"). The slightly-misattributed `"least_request"` prefix on an effectively-unreachable boot-fail path is a recorded cosmetic note (PROGRESS.md + BEHAVIOR_CONTRACT departure record), not worth the churn. YAGNI.
- **D-S35-3 (band constants + tuning protocol):** RESOLVED — `K=4` held / `S=60` burst / `c1+c2+c3 == 64` (conservation) / each `cᵢ >= 12` (uniform floor) / each `cᵢ <= 32` (uniform ceiling); the tuning protocol is Task 6 (≥20 local repeat runs flake-free **per side** + the three `-count=1` deliberate-break proofs). Margins justified by the live three-burst probe (16–25 around mean 21.3, σ ≈ 3.77; floor 12 ≈ 2.47σ below, ceiling 32 ≈ 2.83σ above — SPEC §11.2).
- **D-S35-4 (anti-skew integration test):** RESOLVED — a standalone deterministic-RNG unit test (Task 4) proving RANDOM does NOT avoid a repeatedly-picked/held endpoint (the in-process contrapositive of `TestLeastRequest_SkewAvoidsLoadedEndpoint`), distinct from the `0060` fixture.
- **ADR-0045 split-gate FINAL re-check:** **NO SPLIT.** This PLAN decomposes into **8 tasks** (≤ ~25) over **~45–60 production LoC** (≤ ~1500) — both legs FAR under the gate by an order of magnitude. **NO escape valve** (there is no seam build to split off — the seam is reused unchanged).

### Decomposition note (why 8, not the SPEC's indicative 7)

SPEC §10 bundles the full differential re-verify and the completion bundle into a single Task 7. This PLAN splits them — **Task 7 (verification gate: run the six gates)** and **Task 8 (completion bundle: write the BEHAVIOR_CONTRACT delta + the ADR-0234 body + the STATE/ROADMAP advance)** — because they are different kinds of work (running gates vs. authoring docs, ADR-0052 atomic landing). All other SPEC §10 tasks map 1:1. NO merge is needed (unlike phase 34, there is no compiler-coupled interface reshape — the seam is reused unchanged).

| SPEC §10 task | This plan |
|---|---|
| 1 baselines gate | Task 1 |
| 2 `randomLB` type | Task 2 |
| 3 manager acceptance + reject + retarget | Task 3 |
| 4 boot smoke + anti-skew integration | Task 4 |
| 5 `0060` fixture | Task 5 |
| 6 band tuning + deliberate breaks | Task 6 |
| 7 full differential re-verify | **Task 7** |
| (7, cont.) completion bundle | **Task 8 (split out)** |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/random.go` | **CREATE** | The stateless `randomLB` type + `newRandom`/`newRandomWithRNG` (REUSE `newPCGRNG`) + `Pick()`. |
| `internal/cluster/random_test.go` | **CREATE** | The deterministic-RNG unit tests (Task 2) + the anti-skew integration test (Task 4). |
| `internal/cluster/manager.go` | MODIFY | The `case clusterv3.Cluster_RANDOM` bare construction in `buildCluster`; the NEW reject text. |
| `internal/cluster/manager_test.go` | MODIFY | The RANDOM accept + mismatched-oneof tests; the doubly-hit retarget of `TestManager_Error_UnsupportedLBPolicy`; the boot-smoke test (Task 4). |
| `test/fixtures/0060-lb-random/` | **CREATE** | The differential fixture: `driver/driver.go`, `driver/driver_test.go`, `README.md`, `expectations.yaml` (mirroring the `0059` dir layout). |
| `docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 8) | The `random` subsection + the line-899 reject-text update + the departure/coverage records. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 8) | The full ADR-0234 entry (§Context + §Decision + §Consequences; ADR-0044 in-place). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 8) | Row 35 `in-progress → done`; counts advance (fixtures 61 → 62; DECISIONS tail → ADR-0234). |

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code (the established first-task discipline), and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

Run (from repo root):
```bash
ls -d test/fixtures/[0-9]* | wc -l        # expect 61
ls -d test/fixtures/[0-9]* | tail -1      # expect test/fixtures/0059-lb-least-request
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect TCPThriftResponder BackendKind = 33
grep -c "^## ADR-0" docs/envoy-go/DECISIONS.md   # informational; confirm tail ADR-0233, next-free ADR-0234
go build ./... && echo BUILD_OK
```
Expected: fixtures **61** (tail `0059-lb-least-request`), BackendKind tail **33**, fuzzers **42** (use the repo's established fuzzer-count recipe — do NOT hand-roll a `grep "func Fuzz"` count, which over-counts seed helpers), stat surface **1116** (confirm via the repo's existing stat-surface golden/recipe), DECISIONS tail **ADR-0233** (the ADR-0234 §Context is a DRAFT in SPEC §13 — NOT yet in DECISIONS.md). Reconcile the SPEC's one non-blocking fuzzer-ledger advisory here via the canonical recipe.

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these line anchors still hold (they shift if the SPEC tip moved); record actual line numbers in PROGRESS.md:
```bash
grep -n "func (rr \*roundRobin) Pick" internal/cluster/loadbalancer.go      # the zero-churn posture random copies
grep -n "noopRelease = func" internal/cluster/loadbalancer.go              # the shared release random returns
grep -n "func newPCGRNG" internal/cluster/leastrequest.go                  # REUSED verbatim
grep -n "func newLeastRequestWithRNG" internal/cluster/leastrequest.go     # newRandomWithRNG mirrors it minus choiceCount
grep -n "switch c.GetLbPolicy()" internal/cluster/manager.go              # the case to extend
grep -n "unsupported lb_policy" internal/cluster/manager.go               # the reject text (ONE production string)
grep -n "func TestManager_Error_UnsupportedLBPolicy" internal/cluster/manager_test.go  # the doubly-hit retarget
grep -n "func acceptEchoCounting" test/differential/runner_test.go        # the streaming echo backend (0060 reuses)
```

- [ ] **Step 3: Confirm the reject-text blast radius is exactly THREE sites (AMEND-R2)**

```bash
grep -rln "unsupported lb_policy" internal/ cmd/                          # expect ONLY internal/cluster/manager.go
grep -rln "ROUND_ROBIN, LEAST_REQUEST" test/                             # expect EMPTY (no fixture pins the text → no boot-reject dir)
grep -rln "ROUND_ROBIN, LEAST_REQUEST" docs/envoy-go/BEHAVIOR_CONTRACT.md # expect a hit at :899
```
Expected: production string ONLY in `manager.go`; ZERO fixture hits (confirms NO boot-reject dir); the doc line at `BEHAVIOR_CONTRACT.md:899`.

- [ ] **Step 4: Create PROGRESS.md**

Create `docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md` with: the 8-task table (status column), the count anchors from Step 1, the as-built line anchors from Step 2, the three-site blast radius from Step 3, the D-S35-1..4 resolutions, and the ADR-0045 re-check verdict (NO SPLIT). Mark Task 1 complete.

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md
git commit -m "phase 35 Task 1: baselines gate + PROGRESS.md (fixtures 61 / fuzzers 42 / stat surface 1116 / BackendKind 33 / DECISIONS tail ADR-0233 confirmed; reject-text blast radius 3 sites)"
```

---

## Task 2: The `randomLB` stateless uniform-pick type (`random.go`)

**Goal:** Implement the stateless uniform-pick LB mirroring Envoy v1.37.2's `RandomLoadBalancer::peekOrChoose` EXACTLY (AMEND-R5): ONE draw `rng() % n`, pick `endpoints[i]`, return the shared `noopRelease`. REUSE `newPCGRNG` verbatim.

**Files:**
- Create: `internal/cluster/random.go`
- Create: `internal/cluster/random_test.go`

- [ ] **Step 1: Write the failing tests** (`random_test.go`)

```go
package cluster

import (
	"sync"
	"testing"
)

// seqRNG returns a deterministic rng closure yielding the given values in order,
// then repeating — the upstream mock-RNG posture (AMEND-R5). (If leastrequest_test.go
// already defines seqRNG/eps in this package, REUSE those and delete these — do not
// redeclare. Verify at IMPL: grep -n "func seqRNG\|func eps" internal/cluster/*_test.go)
func seqRNG(vals ...uint64) func() uint64 {
	i := 0
	return func() uint64 {
		v := vals[i%len(vals)]
		i++
		return v
	}
}

func eps(n int) []Endpoint {
	out := make([]Endpoint, n)
	for i := range out {
		out[i] = Endpoint{Host: string(rune('a' + i)), Port: uint32(1000 + i)}
	}
	return out
}

func TestRandom_FollowsDrawExactly(t *testing.T) {
	// pick i == endpoints[draw % n] for each draw — the load-bearing correctness
	// test (the pin-to-endpoints[0] deliberate break fails this). n=3, draws map:
	//   0%3=0→a, 4%3=1→b, 8%3=2→c, 2%3=2→c, 3%3=0→a.
	r := newRandomWithRNG(eps(3), seqRNG(0, 4, 8, 2, 3))
	want := []string{"a", "b", "c", "c", "a"}
	for i, w := range want {
		ep, release, err := r.Pick()
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != w {
			t.Errorf("pick %d: got %q, want %q (rng()%%n)", i, ep.Host, w)
		}
		if release == nil {
			t.Fatalf("pick %d: release must be non-nil (interface contract)", i)
		}
		release()
		release() // noopRelease: safe to call twice
	}
}

func TestRandom_ReleaseIsSharedNoop(t *testing.T) {
	// random holds NO per-pick state → it returns the shared noopRelease.
	r := newRandomWithRNG(eps(2), seqRNG(0))
	_, release, err := r.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if release == nil {
		t.Fatal("release must be non-nil")
	}
	release() // must not panic
}

func TestRandom_NoEndpoints(t *testing.T) {
	r := newRandomWithRNG(nil, seqRNG(0))
	_, release, err := r.Pick()
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestNewRandom_ProductionRNGSeeds(t *testing.T) {
	// Smoke: the crypto-seeded production constructor (REUSED newPCGRNG) succeeds
	// and Pick is concurrency-safe (the mutex-guarded rng).
	r, err := newRandom(eps(3))
	if err != nil {
		t.Fatalf("newRandom: %v", err)
	}
	var wg sync.WaitGroup
	seen := make([]int, 3)
	var mu sync.Mutex
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep, rel, perr := r.Pick()
			if perr != nil {
				return
			}
			rel()
			mu.Lock()
			seen[int(ep.Port)-1000]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	for i, n := range seen {
		if n == 0 {
			t.Errorf("endpoint %d never picked over 60 draws (rng stuck?)", i)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestRandom|TestNewRandom" ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`randomLB`/`newRandom`/`newRandomWithRNG` undefined). (If `seqRNG`/`eps` collide with `leastrequest_test.go`, the compile error names the redeclaration — delete the duplicates and REUSE the existing ones.)

- [ ] **Step 3: Implement `random.go`** (SPEC §3.1 verbatim)

```go
package cluster

// randomLB is a stateless uniform-pick load balancer mirroring Envoy v1.37.2's
// RandomLoadBalancer::peekOrChoose (SPEC §3.1 / AMEND-R5): ONE draw rng()%n,
// pick endpoints[i]; it consults NO active counters and holds NO per-pick state
// (the contrast with leastRequest's P2C). It REUSES the ADR-0232 seam UNCHANGED,
// returning the shared noopRelease (the seam's first reuse — ADR-0232 §Consequences
// "the non-keyed RANDOM drops in at zero cost").
type randomLB struct {
	endpoints []Endpoint
	rng       func() uint64 // injectable for deterministic tests (the upstream mock posture)
}

// newRandom constructs a randomLB over endpoints, REUSING the package-private
// newPCGRNG (leastrequest.go) verbatim — a mutex-guarded crypto-seeded
// math/rand/v2 PCG (AMEND-R5; the helper needs NO change). The crypto/rand read
// error threads out → boot-fail (the leastRequest disposition). NOTE (D-S35-2):
// newPCGRNG's seed-error message reads "cluster: least_request: seed rng" — we
// accept the shared message rather than wrap it; the path is reachable only on a
// crypto/rand failure (effectively unreachable on Linux getrandom).
func newRandom(endpoints []Endpoint) (*randomLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newRandomWithRNG(endpoints, rng), nil
}

// newRandomWithRNG is the injectable constructor used by unit tests to supply a
// deterministic draw sequence (mirrors newLeastRequestWithRNG minus choiceCount).
func newRandomWithRNG(endpoints []Endpoint, rng func() uint64) *randomLB {
	return &randomLB{endpoints: endpoints, rng: rng}
}

// Pick implements loadBalancer. One uniformly-random draw; no active-count
// consultation; the shared noopRelease (random holds no per-pick state).
func (r *randomLB) Pick() (Endpoint, func(), error) {
	n := len(r.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints // the roundRobin/leastRequest parity (AMEND-R5)
	}
	i := int(r.rng() % uint64(n))
	return r.endpoints[i], noopRelease, nil
}
```

Note: no new imports — `randomLB` references only package-local `Endpoint`, `noopRelease`, `errNoEndpoints`, and `newPCGRNG` (all in-package). `gofmt`/`goimports` will leave the import block empty.

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run "TestRandom|TestNewRandom" ./... 2>&1 | tail
cd internal/cluster && go test -run TestNewRandom -race ./... 2>&1 | tail   # -race exercises the mutex-guarded rng
```
Expected: PASS, no race.

- [ ] **Step 5: gofmt + lint (per-task discipline)**

```bash
gofmt -l internal/cluster/
golangci-lint run ./internal/cluster/...
```
Expected: empty `gofmt -l`; clean lint.

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add internal/cluster/random.go internal/cluster/random_test.go
git commit -m "phase 35 Task 2: randomLB stateless uniform-pick type (one draw rng()%n; shared noopRelease; REUSES newPCGRNG verbatim; empty-set errNoEndpoints parity) — mirrors v1.37.2 RandomLoadBalancer::peekOrChoose (AMEND-R5)"
```

---

## Task 3: Manager acceptance + the NEW reject text + the doubly-hit retarget

**Goal:** `buildCluster` accepts `RANDOM` with a bare `newRandom(endpoints)` construction (NO config parse — RANDOM has no config message, AMEND-R1); the `default` reject text extends to `(…, RANDOM)`; the mismatched-oneof under RANDOM is silently ignored (reference parity); `TestManager_Error_UnsupportedLBPolicy` retargets its trigger off the now-accepted `Cluster_RANDOM` AND re-pins the new substring (the doubly-hit retarget — AMEND-R2).

**Files:**
- Modify: `internal/cluster/manager.go` (the `switch c.GetLbPolicy()` at :234; the reject text at :252)
- Modify: `internal/cluster/manager_test.go` (add RANDOM accept + mismatched-oneof tests; retarget `TestManager_Error_UnsupportedLBPolicy`)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestManager_Accept_Random_NoConfig(t *testing.T) {
	c := mkStaticCluster("c_rand", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RANDOM // RANDOM has no lb_config — bare construction
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("RANDOM bare must be accepted: %v", err)
	}
}

func TestManager_Accept_Random_MismatchedOneof(t *testing.T) {
	// A stray least_request_lb_config under RANDOM → silent-ignore (reference parity,
	// SPEC §6.3 / AMEND-R1: the manager never reads the oneof on the RANDOM path).
	c := mkStaticCluster("c_rand", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RANDOM
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)},
	}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under RANDOM must be silently accepted: %v", err)
	}
}
```

Then RETARGET the existing `TestManager_Error_UnsupportedLBPolicy` (currently at :320). Change the trigger off the now-accepted `Cluster_RANDOM` to a still-rejected policy, and extend the pinned substring to the NEW supported-set text:

```go
func TestManager_Error_UnsupportedLBPolicy(t *testing.T) {
	c := mkStaticCluster("c_x", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RING_HASH // RANDOM now accepted → retarget to a still-rejected policy (AMEND-R2)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("RING_HASH must be rejected")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST, RANDOM") {
		t.Errorf("error %q missing new supported-set substring (…, RANDOM)", err.Error())
	}
}
```

> **Both edits are load-bearing.** The trigger change is required because `Cluster_RANDOM` no longer errors (the old assertion `if err == nil` would fail). The substring extension is required to make the assertion pin the NEW text — `"ROUND_ROBIN, LEAST_REQUEST"` is still a substring of `"ROUND_ROBIN, LEAST_REQUEST, RANDOM"`, so without extending it the test would pass against the OLD text too (vacuous w.r.t. the change). `wrapperspb` is already imported (phase 34); if `goimports` flags it unused after edits, leave it — the mismatched-oneof test uses it.

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestManager_Accept_Random|TestManager_Error_UnsupportedLBPolicy" ./... 2>&1 | head -30
```
Expected: the RANDOM accept tests FAIL (current `default` rejects RANDOM); `TestManager_Error_UnsupportedLBPolicy` FAILS the new substring (old text lacks `, RANDOM`).

- [ ] **Step 3: Implement the manager change** (SPEC §3.3)

In `manager.go`, add the `case clusterv3.Cluster_RANDOM` to the `switch c.GetLbPolicy()` (between the LEAST_REQUEST case and `default`), and extend the reject text:
```go
	case clusterv3.Cluster_RANDOM: // phase 35 (ADR-0234): stateless uniform pick; NO config parse (RANDOM has no config message)
		lb, err := newRandom(endpoints)
		if err != nil {
			return nil, err
		}
		cl.lb = lb
	default:
		// The ONE deliberate byte-stable-reject change this phase (ADR-0080;
		// blast radius AMEND-R2: this string + the manager_test retarget +
		// BEHAVIOR_CONTRACT.md:899). RING_HASH/MAGLEV stay an envoy-go-strict
		// DEPARTURE (the reference validate-accepts them — recorded in
		// BEHAVIOR_CONTRACT).
		return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)", name, c.GetLbPolicy())
```

There is NO `parseRandomLbConfig` (RANDOM has no config message — AMEND-R1); the mismatched-oneof under RANDOM is silently ignored structurally (the RANDOM case never reads `c.GetLbConfig()`).

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS (the full package, incl. the unchanged LEAST_REQUEST/ROUND_ROBIN tests).

- [ ] **Step 5: gofmt + lint**

```bash
gofmt -l internal/cluster/
golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 35 Task 3: manager accepts RANDOM (bare construction, no config parse); NEW reject text (…, RANDOM); doubly-hit retarget of TestManager_Error_UnsupportedLBPolicy (trigger RANDOM→RING_HASH + substring extended); mismatched-oneof silent-ignore (SPEC §6 / AMEND-R2)"
```

---

## Task 4: In-process anti-skew integration test + RANDOM boot smoke

**Goal:** Prove end-to-end that RANDOM follows the draw and does NOT avoid a repeatedly-picked/held endpoint (the in-process contrapositive of least_request's skew test — D-S35-4), and that a RANDOM bootstrap builds a working Manager.

**Files:**
- Modify: `internal/cluster/random_test.go` (the anti-skew integration test)
- Modify: `internal/cluster/manager_test.go` (the boot smoke test)

- [ ] **Step 1: Write the failing tests**

```go
// random_test.go
func TestRandom_DoesNotAvoidHeldEndpoint(t *testing.T) {
	// The anti-skew property in-process (the contrapositive of least_request's
	// TestLeastRequest_SkewAvoidsLoadedEndpoint): randomLB holds NO active counters,
	// so repeatedly picking AND holding an endpoint does NOT make a later pick avoid
	// it — the draw alone decides. With a deterministic RNG drawing index 0 every
	// time, all three picks land on endpoint "a" even though the first two are still
	// held. (least_request would have routed picks 2/3 AWAY from the loaded "a".)
	r := newRandomWithRNG(eps(3), seqRNG(0))
	held := make([]func(), 0, 3)
	for i := 0; i < 3; i++ {
		ep, rel, err := r.Pick()
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != "a" {
			t.Errorf("pick %d: got %q, want a (RANDOM follows the draw, ignores held load)", i, ep.Host)
		}
		held = append(held, rel) // hold (noopRelease — nothing to elevate; the point is there is no counter)
	}
	for _, rel := range held {
		rel()
	}
}
```

```go
// manager_test.go — boot smoke from a realistic 3-endpoint RANDOM cluster.
func TestManager_Random_BootSmoke(t *testing.T) {
	c := mkStaticCluster("c_rand",
		mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RANDOM
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cl, ok := m.Get("c_rand")
	if !ok {
		t.Fatal("cluster c_rand not found")
	}
	for i := 0; i < 10; i++ { // exercises the immediate-release PickEndpoint path
		ep, perr := cl.PickEndpoint()
		if perr != nil {
			t.Fatalf("PickEndpoint: %v", perr)
		}
		if ep.Port < 9001 || ep.Port > 9003 {
			t.Errorf("picked out-of-range endpoint %v", ep)
		}
	}
}
```

- [ ] **Step 2: Run to verify they pass-by-design**

```bash
cd internal/cluster && go test -run "TestRandom_DoesNotAvoidHeldEndpoint|TestManager_Random_BootSmoke" -count=1 ./... 2>&1 | tail
```
Expected: green (Tasks 2-3 landed the production code — these are integration assertions over it). The liveness proof is Step 3.

- [ ] **Step 3: Prove the anti-skew test is live (`-count=1`)**

Temporarily give `randomLB.Pick` a load-avoidance that, if RANDOM accidentally behaved like a least-loaded policy, would skip the just-drawn index. Add a transient field `lastPicked int` and after computing `i`, insert: `if i == r.lastPicked { i = (i + 1) % n }; r.lastPicked = i`. Run `go test -run TestRandom_DoesNotAvoidHeldEndpoint -count=1` and confirm it FAILS (pick 2 → "b" instead of "a"). Then REVERT (delete the field + the skip). Record in PROGRESS.md (`reference_differential_break_protocol_count1`). This proves the test would catch an accidental load-avoidance — the in-process analogue of the `0060` floor-leg break.

- [ ] **Step 4: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/random_test.go internal/cluster/manager_test.go
git commit -m "phase 35 Task 4: in-process anti-skew integration test (RANDOM follows the draw, does NOT avoid a held endpoint; -count=1 load-avoidance liveness proven) + RANDOM boot smoke (D-S35-4)"
```

---

## Task 5: The `0060-lb-random` differential fixture

**Goal:** A cross-side `[tcp_proxy]` fixture over a 3-endpoint RANDOM cluster, with the REUSED `0059` hold-4 + burst-60 + drain workload but the assertion INVERTED — a band-based ANTI-SKEW `AssertDistribution` (conservation + uniform-floor 12 + uniform-ceiling 32, PER SIDE) + a cross-side `StatsAsserter` prong. NO new BackendKind (reuses `TCPEcho = 0`). NO boot-reject dir.

**Files:**
- Create: `test/fixtures/0060-lb-random/driver/driver.go`
- Create: `test/fixtures/0060-lb-random/driver/driver_test.go` (the per-fixture registration smoke, mirroring `0059`)
- Create: `test/fixtures/0060-lb-random/README.md`
- Create: `test/fixtures/0060-lb-random/expectations.yaml`

- [ ] **Step 1: Write the driver** (`driver/driver.go`)

Start from a verbatim copy of `test/fixtures/0059-lb-least-request/driver/driver.go`, then make these changes:
1. `fixtureName = "0060-lb-random"`; pick a fresh `refContainerListenerPort` (confirm no collision with other fixtures — grep `refContainerListenerPort` across `test/fixtures/` and choose a free value; `0059` uses 19148, so e.g. 19149 if free).
2. In BOTH `ReferenceBootstrap` and `SubjectConfig`: change `lb_policy: LEAST_REQUEST` → `lb_policy: RANDOM` and **DELETE** the `least_request_lb_config: { choice_count: 10 }` line (RANDOM has no config message — AMEND-R1).
3. The `drive` workload (hold-4 + burst-60 + drain), `DriveReference`/`DriveSubject`, `ProbeAdmin`, `scrapeStats`, and the registration are REUSED UNCHANGED from `0059`.

The `drive` workload is byte-for-byte the `0059` workload (the SPEC §8.1 "the REUSED `0059` stimulus"):
```go
const (
	heldConns   = 4                   // K (D-S35-3)
	burstConns  = 60                  // S (D-S35-3)
	totalConns  = heldConns + burstConns // 64 (conservation target)
	clusterName = "c_echo"
)
```
Keep `BackendCount() == 3`, `SubjectListenerName() == "l_tcp"`, and the `host.docker.internal` (reference STRICT_DNS) / `127.0.0.1` (subject STATIC) split exactly as `0059`.

- [ ] **Step 2: Write the ANTI-SKEW band `AssertDistribution`** (the INVERSION of `0059`)

`0059` asserts STARVATION (`c1 <= 12`) + CONCENTRATION (`c2 >= 16`) — the loaded backend is AVOIDED. `0060` asserts the OPPOSITE: every backend stays in the fair-share band {floor 12, ceiling 32} DESPITE the same held-conn load. The band is SYMMETRIC (no sort-asymmetry needed — check each of the three raw counts):

```go
// AssertDistribution: PER-SIDE anti-skew band on the per-backend accept counts
// (the runner snapshots backend.accepts after Drive). NEVER cross-side-exact
// (independent RNG streams — the 0059 per-side precedent). The SECOND band-based
// AssertDistribution use; the CONTRAPOSITIVE of 0059 (least_request).
//
//   conservation:    c1 + c2 + c3 == 64           (hard equality; catches drop/double-count)
//   uniform floor:   each cᵢ >= 12   (mean 21.3 − ~2.5σ; VIOLATED by a load-skewing
//                                     [least_request-like] policy starving a loaded
//                                     backend, and by single-host pinning [one host ≈ 0])
//   uniform ceiling: each cᵢ <= 32   (mean 21.3 + ~2.8σ; VIOLATED by single-host
//                                     pinning [one host ≈ 64])
func (randDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	const (
		floor   = 12
		ceiling = 32
	)
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", sd.side, len(sd.counts))
		}
		var sum uint64
		for i, c := range sd.counts {
			sum += c
			if c < floor {
				return fmt.Errorf("%s: uniform floor: backend[%d]=%d < %d (load-skewing policy? single-host pin?)", sd.side, i, c, floor)
			}
			if c > ceiling {
				return fmt.Errorf("%s: uniform ceiling: backend[%d]=%d > %d (single-host pin?)", sd.side, i, c, ceiling)
			}
		}
		if sum != totalConns {
			return fmt.Errorf("%s: conservation: sum %d != %d", sd.side, sum, totalConns)
		}
	}
	return nil
}
```
(`randDriver` is this fixture's driver type — rename the `0059` `lrDriver` to `randDriver` when copying; use it consistently as the receiver here and in the `StatsAsserter` + interface checks below.)

- [ ] **Step 3: Write the `StatsAsserter`** (the §7 cross-vs-per-side set — IDENTICAL to `0059`)

Copy `0059`'s `AssertStats` verbatim (the set is unchanged — SPEC §7 / AMEND-R4): cross-equal `upstream_cx_total == 64` + `membership_total == 3` + `upstream_cx_active == 0` (quiesced post-drain); per-side `upstream_rq_total` (ref = 64 [tcp_proxy charges rq-per-cx]; subj = 0 [the tcpproxy path never calls `IncUpstreamRqTotal`]). Reuse `0059`'s `scrapeStats` helper verbatim.

Add the compile-time interface checks:
```go
var (
	_ fixture.Driver               = (*randDriver)(nil)
	_ fixture.DistributionAsserter = (*randDriver)(nil)
	_ fixture.StatsAsserter        = (*randDriver)(nil)
)
```

- [ ] **Step 4: Write README.md + expectations.yaml + driver_test.go**

- `README.md`: the workload (hold-4/burst-60/drain — REUSED from `0059`), the anti-skew band rationale (conservation/uniform-floor 12/uniform-ceiling 32; the σ ≈ 3.77 / floor 2.47σ / ceiling 2.83σ math from SPEC §11.2), the **contrast with `0059`** (least_request AVOIDS the loaded backend; random STAYS uniform DESPITE it — the anti-skew positive separation), the per-side RNG non-equivalence (never cross-side-exact), the per-side `upstream_rq_total` boundary, and the Task-6 deliberate-break record. Note the firsts-now-expectations: SECOND band-based `AssertDistribution`; NO new BackendKind; NO new fuzzer; ZERO stat delta (1116).
- `expectations.yaml`: mirror the `0059` shape (cross-side byte-equivalence + the asserter dispatch in-band; document the band + stats prongs).
- `driver_test.go`: the registration smoke (mirror `0059`'s `driver_test.go`).

- [ ] **Step 5: Run the fixture (requires Docker + the contrib reference image)**

```bash
go test ./test/differential/ -run 'TestDifferential/0060' -count=1 -v 2>&1 | tail -40
```
Expected: PASS (byte-equivalence + the anti-skew band per side + the stats prong). **Use `-run 'TestDifferential/0060'`, NOT `-run '0060'`** (`reference_differential_run_selector` — the bare form matches zero subtests → vacuous green). If Docker is unavailable in the IMPL environment, the controller runs this gate where Docker is present (per `reference_docker_probe_bridge_network`; verify decode ran via `downstream_cx_rx_bytes_total > 0`).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0060-lb-random/ && golangci-lint run ./test/...
git add test/fixtures/0060-lb-random/
git commit -m "phase 35 Task 5: 0060-lb-random differential fixture — REUSED 0059 hold-4/burst-60/drain over a RANDOM cluster with the assertion INVERTED; SECOND band-based AssertDistribution (anti-skew: conservation/uniform-floor 12/uniform-ceiling 32) + cross-vs-per-side StatsAsserter; reuses TCPEcho (no new BackendKind)"
```

---

## Task 6: Band-constant tuning + deliberate-break liveness (`-count=1`)

**Goal:** Finalize the band constants against repeat runs and PROVE each prong is live (a band that cannot fail is a dead assertion — `reference_differential_break_protocol_count1` generalized to BAND assertions, DOUBLY load-bearing here per SPEC §8).

**Files:** (tuning only — adjust constants in `driver/driver.go`; record in `README.md` + PROGRESS.md)

- [ ] **Step 1: Repeat-run flake check (≥20 per side)**

```bash
go test ./test/differential/ -run 'TestDifferential/0060' -count=20 2>&1 | tail
```
Expected: 20/20 PASS. If any run flakes, widen the band within the pinned PRINCIPLE {conservation + uniform-floor + uniform-ceiling} (e.g. floor 11 / ceiling 33) — never so wide it stops biting the breaks in Steps 2-4. Record the final constants + the observed min/max per side.

- [ ] **Step 2: Deliberate break (i) — consult-the-counters (the canonical anti-skew break) → floor leg FAILS**

Temporarily make the RANDOM cluster behave like least_request: in `manager.go`'s `case clusterv3.Cluster_RANDOM`, construct `newLeastRequest(endpoints, 10)` instead of `newRandom(endpoints)` (this is the faithful "accidentally implement a load-skewing policy" break). Run:
```bash
go test ./test/differential/ -run 'TestDifferential/0060' -count=1 2>&1 | tail
```
Expected: FAIL on the `uniform floor: backend[i] < 12` leg (the held-conn load skews picks AWAY from the most-held backend → it drops below the floor). REVERT (restore `newRandom`). This proves the uniform band bites against load-sensitivity — the anti-skew separation from least_request.

- [ ] **Step 3: Deliberate break (ii) — pin-to-endpoints[0] → floor AND ceiling legs FAIL**

In `randomLB.Pick`, replace `i := int(r.rng() % uint64(n))` with `i := 0`. Run with `-count=1`:
```bash
go test ./test/differential/ -run 'TestDifferential/0060' -count=1 2>&1 | tail
```
Expected: FAIL — the two un-picked backends hit 0 (< floor 12) and the pinned backend hits 64 (> ceiling 32). REVERT.

- [ ] **Step 4: Deliberate break (iii) — stats prong**

Temporarily corrupt one cross-equal want (e.g. `upstream_cx_total` want 64 → 99) OR drop an Inc — confirm the `StatsAsserter` FAILS with `-count=1`, then REVERT. (Verifies the stats prong is non-vacuous.)

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the three break results + the final constants in `README.md` (driver comments) and PROGRESS.md (the `0030` dead-assertion lesson).
```bash
go test ./test/differential/ -run 'TestDifferential/0060' -count=1 2>&1 | tail   # confirm green after all reverts
git add test/fixtures/0060-lb-random/
git commit -m "phase 35 Task 6: 0060 band tuning (K=4/S=60/floor 12/ceiling 32; 20/20 flake-free per side) + 3 deliberate-break liveness proofs (consult-the-counters [floor leg, the canonical anti-skew break]/pin-to-endpoints[0] [floor+ceiling]/stats-prong, -count=1)"
```

---

## Task 7: Full differential re-verify + race + conformance unaffected

**Goal:** Prove the new manager case kept all 61 prior fixtures byte-exact (the seam is reused unchanged → behavior-neutrality is structural) and `0060` is green; run `-race`; assert h2spec/proxy-wasm unaffected.

**Files:** none (verification only)

- [ ] **Step 1: Full differential suite (62 dirs)**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
Expected: ALL 62 PASS — the 61 prior dirs byte-exact through the new `case Cluster_RANDOM` (the ROUND_ROBIN/LEAST_REQUEST paths are untouched; the seam is unchanged) + `0060` green. **This is the six-gate REAL guard.**

- [ ] **Step 2: Race + short across the repo**

```bash
go test -race -short ./... 2>&1 | tail -20
```
Expected: PASS, no race (the mutex-guarded RNG via the REUSED `newPCGRNG`).

- [ ] **Step 3: Build + vet + gofmt + lint (full repo) + tidy**

```bash
go build ./... && go vet ./... && gofmt -l internal/cluster/ test/fixtures/0060-lb-random/ && golangci-lint run ./... 2>&1 | tail
go mod tidy -diff && echo "TIDY_CLEAN"   # ZERO new go.mod dep (AMEND-R1)
```
Expected: clean; `go mod tidy -diff` empty.

- [ ] **Step 4: Conformance unaffected**

Re-run (or assert-unaffected with rationale: phase 35 touches no HTTP/h2/proxy-wasm path — only `internal/cluster`) h2spec **53/53** + proxy-wasm **10/10** per the repo's conformance recipe. Record in PROGRESS.md.

- [ ] **Step 5: Commit (LOCAL-ONLY) — verification evidence into PROGRESS.md**

```bash
git add docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md
git commit -m "phase 35 Task 7: six-gate evidence — 62-dir differential byte-exact (61 prior unchanged + 0060), -race -short green, go mod tidy clean, h2spec 53/53 + proxy-wasm 10/10 unaffected"
```

---

## Task 8: Completion bundle (ADR-0052 atomic landing)

**Goal:** Land the BEHAVIOR_CONTRACT delta, the full ADR-0234 entry, and the STATE/ROADMAP advance — atomically with the code.

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/35-load-balancer-random/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md (SPEC §9)**

Add a `random` subsection beside the least_request boundary text: RANDOM acceptance (no config message; bare construction); the uniform-pick semantics (one draw `rng() % n`, no active-count consult — the v1.37.2 mirror; the contrast with least_request's P2C); the per-side RNG non-equivalence (anti-skew band-proven, never cross-side-exact); the healthy-set boundary (no health checking → all-hosts sampling). Update the line-899 reject-text entry (RANDOM retired from the rejected set → the THIRD accepted policy; the supported-set string `… ROUND_ROBIN, LEAST_REQUEST, RANDOM`; RING_HASH/MAGLEV stay the recorded departure). Add departure/coverage records: the mismatched-oneof silent-ignore under RANDOM (parity); the D-S35-2 shared-RNG-message cosmetic note; NO new fuzzer/BackendKind (now family expectations); stat surface UNCHANGED at 1116 (the SECOND zero-delta phase).

- [ ] **Step 2: DECISIONS.md — the full ADR-0234 entry (ADR-0044 in-place)**

Author the complete ADR-0234 (the `random` load-balancing policy): §Context (from the SPEC §13 DRAFT — promote it verbatim, status PROPOSED → ACCEPTED), §Decision (the stateless `randomLB`; the seam REUSE unchanged returning `noopRelease`; the bare manager case + the reject-text change; the REUSED `newPCGRNG`; the `0060` anti-skew band proof shape), §Consequences (the seam's first-reuse validation of ADR-0232's "zero cost" claim; ADR-0024 unamended; the second zero-delta/no-fuzzer/no-BackendKind phase; the hash policies RING_HASH/MAGLEV remain the seam-EXTENDING future rows). Tail advances **ADR-0233 → ADR-0234**; next-free **ADR-0235**. NO seam ADR (reuse — the thrift-33 single-ADR-on-reuse precedent).

- [ ] **Step 3: STATE.md + ROADMAP.md**

STATE active-phase → `phase 35 (load-balancer-random) done`; lifecycle-state → the phase-35 phase-done routing. Counts: fixtures 61 → **62**; stat surface **1116** (zero delta); fuzzers **42**; BackendKind tail **33**; DECISIONS tail **ADR-0234**. ROADMAP row 35 `in-progress → done` (flat family row — NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (6 candidates remain after 35: ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds).

- [ ] **Step 4: Finalize PROGRESS.md** — all 8 tasks complete; the six-gate evidence; the D-S35-1..4 resolutions; the three deliberate-break records.

- [ ] **Step 5: Final six-gate re-run + commit (LOCAL-ONLY)**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./... && go test -race -short ./... 2>&1 | tail
go test ./test/differential/ -count=1 2>&1 | tail
git add docs/envoy-go/
git commit -m "phase 35 Task 8: completion bundle — BEHAVIOR_CONTRACT random delta; ADR-0234 full entry (ADR-0044 in-place; tail → ADR-0234); STATE/ROADMAP row 35 done (fixtures 61->62; stat surface 1116 zero-delta; the SECOND no-fuzzer/no-BackendKind phase)"
```

- [ ] **Step 6: Controller stage-close** — the controller squash-merges the IMPL branch to master, runs the final six-gate on master, and (per `feedback_push_to_origin`, tests green) pushes to origin. Subagents NEVER push (`feedback_subagents_no_push`).

---

## Exit criteria (ADR-0052 atomic landing)

- stat surface **1116** (ZERO delta — the SECOND zero-delta phase, AMEND-R4).
- differential fixtures **61 → 62** (`0060-lb-random`; NO boot-reject dir — AMEND-R2).
- fuzzers **42** (NO new fuzzer — deliberate); BackendKind tail **33** (NO new BackendKind — `TCPEcho` 0 reused).
- DECISIONS tail **ADR-0233 → ADR-0234** (the full entry lands at this IMPL per ADR-0044; next-free ADR-0235).
- All 62 differential dirs byte-exact (61 prior unchanged through the new manager case + `0060` green); `-race -short` green; h2spec 53/53 + proxy-wasm 10/10 unaffected; `go mod tidy -diff` empty.
- ROADMAP row 35 `done` (flat family row); the Load-balancing family OPEN (6 candidates remain).
