# Phase 34 Implementation Plan — `least_request` load balancer (`Cluster.LbPolicy LEAST_REQUEST` + `Cluster.LeastRequestLbConfig`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.LbPolicy LEAST_REQUEST` (config message `Cluster.LeastRequestLbConfig`, `envoy.config.cluster.v3`) as un-weighted power-of-two-choices (P2C) over a NEW LB acquire/release seam in `internal/cluster` — the project's first LB policy beyond ROUND_ROBIN and its first framework seam OUTSIDE `internal/filter/network`.

**Architecture:** The unexported `loadBalancer` interface (`internal/cluster/loadbalancer.go`) reshapes `Pick() (Endpoint, error)` → `Pick() (Endpoint, func(), error)` (the ADR-0232 seam, OPTION C — AMEND-L7). `roundRobin` returns a shared no-op release (provably behavior-neutral — all 60 existing fixtures stay byte-exact). A NEW `leastRequest` type (`leastrequest.go`) owns per-endpoint `atomic.Int64` active counters, mirrors Envoy v1.37.2's P2C semantics exactly (AMEND-L6), and returns a `sync.Once`-guarded decrement release. The release is threaded INSIDE `cluster.go` only: `PickEndpoint` releases immediately; `Dial`/`AcquireH1` compose it into the existing ADR-0063 `connWithGauge` `dec` closure and release-on-dial/TLS-failure. The EXPORTED `Cluster` surface stays byte-stable (ZERO consumer churn). `Manager.buildCluster` accepts `LEAST_REQUEST`, parses `choice_count` (hand-rolled `>= 2` reject — the manager calls no PGV), and departure-rejects `active_request_bias`/`slow_start_config`.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `Cluster.LeastRequestLbConfig` already in the pinned module; **ZERO new go.mod dep**); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); `math/rand/v2` + `crypto/rand` (stdlib — the injectable P2C RNG). The differential proof is a band-based per-side `DistributionAsserter` skew arm + a cross-side `StatsAsserter` prong (`0059-lb-least-request`).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/34-load-balancer-least-request/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-L1..L7; §3 the OPTION-C seam + the `leastRequest` design; §5/§6 the proto + reject matrices; §7 the zero-delta stat surface; §8 the `0059` band-arm design; §10 the ~10-task spine; §11 the D-L empirical pins; §12 the D-S34 questions; Appendix A the live probe basis.
- **BRAINSTORM:** `docs/envoy-go/phases/34-load-balancer-least-request/BRAINSTORM.md` — the charter (Q0/Q1/Q-seam/Q-split).
- **As-built seam sites:** `internal/cluster/loadbalancer.go` (the interface + `roundRobin`), `internal/cluster/cluster.go` (`PickEndpoint`:173, `Dial`:198, `AcquireH1`:242, `connWithGauge`:333-347, `closePool`:367), `internal/cluster/manager.go` (the `buildCluster` lb_policy guard:215-217 + the `cl :=` literal:230-235).
- **Differential harness:** `test/differential/fixture/fixture.go` (the `Driver`/`DistributionAsserter`/`StatsAsserter` interfaces; `BackendKind` enum — `TCPEcho = 0` is the default), `test/differential/runner_test.go` (the `DistributionAsserter` hook:1102, the `StatsAsserter` hook:1158, `acceptEchoCounting`:1329 — STREAMING echo), `test/fixtures/0001-tcp-proxy-rr/driver/driver.go` (the harness shape `0059` transposes), `test/fixtures/0057-thrift-roundtrip/driver/driver.go` (`AssertStats`:378 + `scrapeStats`:536 — the cross-equal-vs-per-side pattern).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` — subagent-driven execution.
- `feedback_git_worktrees` — this PLAN was authored in worktree `.worktrees/phase-34-plan`; the IMPL runs in its own worktree.
- `feedback_subagents_no_push` — **subagents commit LOCAL-ONLY**; the controller squash-merges + pushes at stage-close.
- `feedback_pertask_gofmt_lint` — **every task** runs `gofmt -l` + `golangci-lint run` on the touched packages (not just `go vet`).
- `feedback_subagent_worktree_path_targeting` — all paths below are repo-root-relative; the IMPL worktree is the canonical checkout; the controller verifies the main checkout stays clean.
- `reference_differential_break_protocol_count1` — every new differential assertion is proven live by a deliberate-break with `-count=1` (go test caching serves a stale PASS otherwise). **Generalized to BAND assertions here: a band wide enough to never fail is a dead assertion.**
- `reference_differential_asserter_dispatch` — the stats prong uses `StatsAsserter` (cross-side path); the distribution prong uses `DistributionAsserter` (driver-side, runs on both paths).
- ADR-0024 (per-cluster LB counter scope — unamended), ADR-0045 (the split-gate — FINAL re-check below), ADR-0044 (ADR bodies at the IMPL), ADR-0052 (the atomic-landing six-gate), ADR-0063 (`connWithGauge` — the release attach-point), ADR-0080 (byte-stable reject text, substring-pinned by table tests), ADR-0106 (flat family row — NO sub-rows), ADR-0060 (histograms deferred).

## D-question resolutions (SPEC §12)

- **D-S34-1 (file split):** RESOLVED as anticipated — the reshaped interface + `roundRobin` stay in `loadbalancer.go`; the `leastRequest` type lands in a NEW sibling `leastrequest.go`; `parseLeastRequestLbConfig` + `defaultChoiceCount` land in `manager.go`.
- **D-S34-2 (production RNG):** RESOLVED — `newLeastRequest` crypto-seeds a `math/rand/v2` PCG once per construction (two `uint64` seed words from `crypto/rand`), wrapped behind a **mutex-guarded `rng func() uint64`** closure (Pick is called concurrently across downstream conns; `math/rand/v2.Rand` is NOT per-source concurrency-safe). The `crypto/rand` read error is threaded out of `newLeastRequest` and rejected at boot (effectively unreachable on Linux `getrandom`; boot-fail is the safe disposition). Unit tests inject a deterministic sequence via `newLeastRequestWithRNG` (the upstream mock-RNG posture, AMEND-L6).
- **D-S34-3 (band constants + tuning protocol):** RESOLVED — `K=4` held / `S=60` burst / `c1 <= 12` (starvation) / `c2 >= 16` (concentration) / `c1+c2+c3 == 64` (conservation); the tuning protocol is Task 7 (≥20 local repeat runs flake-free **per side** + the three `-count=1` deliberate-break proofs).
- **D-S34-4 (held-conn establishment witness):** RESOLVED — each held conn writes one byte and reads its echo BEFORE holding (`acceptEchoCounting` is a streaming echo — the round-trip proves the upstream dial completed and the pick's active count is held on both the synchronous-increment subject and the assignment-increment reference). Confirmed sufficient at Task 7 by the band margins.
- **D-S34-5 (`connWithGauge` release composition):** RESOLVED as anticipated — compose into the existing `dec func()` closure: `dec: func() { c.upstreamCxActive.Dec(); release() }`. The `connWithGauge` struct is UNCHANGED (its `sync.Once` already guards the composed `dec`).
- **ADR-0045 split-gate FINAL re-check:** **NO SPLIT.** This PLAN decomposes into **9 tasks** (≤ ~25) over **~155–255 production LoC** (≤ ~1500) — both legs FAR under the gate. The pre-authorized **34.1-seam / 34.2-policy** escape valve STAYS UNCONSUMED (the kafka-31/thrift-33 precedent).

### Decomposition note (why 9, not the SPEC's indicative 10)

SPEC §10 lists the interface reshape (its Task 2) and the `cluster.go` release threading (its Task 3) separately. The Go compiler couples them: changing the `loadBalancer.Pick` signature forces every call site in `cluster.go` to change in the SAME commit or the package will not build. They are therefore merged into **Task 2 (seam reshape)** — doing the final threading immediately avoids a throwaway behaviorally-identical intermediate. All other SPEC §10 tasks map 1:1.

| SPEC §10 task | This plan |
|---|---|
| 1 baselines gate | Task 1 |
| 2 interface reshape + 3 cluster.go threading | **Task 2 (merged)** |
| 4 `leastRequest` type | Task 3 |
| 5 manager acceptance + reject matrix | Task 4 |
| 6 skew integration + boot smoke | Task 5 |
| 7 `0059` fixture | Task 6 |
| 8 band tuning + deliberate breaks | Task 7 |
| 9 full differential re-verify | Task 8 |
| 10 completion bundle | Task 9 |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/loadbalancer.go` | MODIFY | The reshaped `loadBalancer` interface; `roundRobin` no-op release; the package-level `noopRelease` var. |
| `internal/cluster/leastrequest.go` | **CREATE** | The `leastRequest` P2C type + `newLeastRequest`/`newLeastRequestWithRNG`/`newPCGRNG`. |
| `internal/cluster/cluster.go` | MODIFY | `PickEndpoint` immediate-release; `Dial`/`AcquireH1` release-on-failure + `connWithGauge` dec composition. |
| `internal/cluster/manager.go` | MODIFY | The two-policy accept switch in `buildCluster`; `parseLeastRequestLbConfig`; `defaultChoiceCount`; the NEW reject text. |
| `internal/cluster/loadbalancer_test.go` | MODIFY | Update existing `rr.Pick()` call sites to the 3-return form; add `roundRobin` no-op-release + behavior-neutrality tests. |
| `internal/cluster/leastrequest_test.go` | **CREATE** | The deterministic-RNG P2C unit tests + the skew integration test. |
| `internal/cluster/cluster_test.go` | MODIFY | The stub-LB counter-balance tests (Dial/AcquireH1/closePool release threading). |
| `internal/cluster/manager_test.go` | MODIFY | The §6 accept/reject matrix table tests; retarget `TestManager_Error_NonRoundRobinLB`; the boot-smoke test. |
| `test/fixtures/0059-lb-least-request/` | **CREATE** | The differential fixture: `driver/driver.go`, `driver/driver_test.go`, `README.md`, `expectations.yaml` (mirroring the `0001` dir layout). |
| `docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 9) | The least_request subsection + the line-897 deferral update + the departure/coverage records. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 9) | ADR-0232 + ADR-0233 §Decision/§Consequences bodies (ADR-0044 in-place). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 9) | Row 34 `in-progress → done`; counts advance (fixtures 60 → 61). |

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code (the established first-task discipline), and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

Run (from repo root):
```bash
ls -d test/fixtures/[0-9]* | wc -l        # expect 60 (tail: 0058-thrift-boot-reject)
ls -d test/fixtures/[0-9]* | tail -1      # expect test/fixtures/0058-thrift-boot-reject
grep -rn "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect TCPThriftResponder BackendKind = 33
grep -c "^ADR-0" docs/envoy-go/DECISIONS.md   # informational; confirm ADR-0232/0233 §Context present, next-free ADR-0234
go build ./... && echo BUILD_OK
```
Expected: fixtures **60**, BackendKind tail **33**, fuzzers **42** (use the repo's established fuzzer-count recipe — do NOT hand-roll a `grep "func Fuzz"` count, which over-counts seed helpers), stat surface **1116** (confirm via the repo's existing stat-surface golden/recipe), DECISIONS tail **ADR-0233** (the §Context drafts landed at the SPEC).

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these line anchors still hold (they shift if the SPEC tip moved); record actual line numbers in PROGRESS.md:
```bash
grep -n "Pick() (Endpoint, error)" internal/cluster/loadbalancer.go         # the interface to reshape
grep -n "only ROUND_ROBIN lb_policy supported" internal/cluster/manager.go   # the guard to replace
grep -n "lb:             &roundRobin" internal/cluster/manager.go            # the literal lb construction
grep -n "dec: c.upstreamCxActive.Dec" internal/cluster/cluster.go            # the connWithGauge attach-points (×2: Dial, AcquireH1)
grep -n "cl.PickEndpoint()" internal/httpclient/httpclient.go internal/filter/network/thriftproxy/filter.go  # the direct-pick consumers (must stay source-compatible)
grep -n "func acceptEchoCounting" test/differential/runner_test.go           # the streaming echo backend
```

- [ ] **Step 3: Create PROGRESS.md**

Create `docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md` with: the 9-task table (status column), the count anchors from Step 1, the as-built line anchors from Step 2, and the ADR-0045 re-check verdict (NO SPLIT). Mark Task 1 complete.

- [ ] **Step 4: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md
git commit -m "phase 34 Task 1: baselines gate + PROGRESS.md (fixtures 60 / fuzzers 42 / stat surface 1116 / BackendKind 33 confirmed)"
```

---

## Task 2: Seam reshape — `loadBalancer` interface + `roundRobin` no-op release + `cluster.go` release threading

**Goal:** Reshape `Pick() (Endpoint, error)` → `Pick() (Endpoint, func(), error)` (ADR-0232 OPTION C), keep `roundRobin` behavior byte-identical, and thread release through `cluster.go`'s conn-producing paths — all behavior-neutral (only `roundRobin` exists; release is a no-op).

**Files:**
- Modify: `internal/cluster/loadbalancer.go`
- Modify: `internal/cluster/cluster.go:173-175` (`PickEndpoint`), `:198-223` (`Dial`), `:242-295` (`AcquireH1`)
- Modify: `internal/cluster/loadbalancer_test.go` (existing `rr.Pick()` call sites at :20/:40/:72/:91 → 3-return form)
- Test: `internal/cluster/cluster_test.go` (NEW stub-LB counter-balance tests)

- [ ] **Step 1: Write the failing tests**

In `internal/cluster/loadbalancer_test.go`, FIRST update the existing call sites (the package won't compile against the new interface otherwise) — change every `ep, err := rr.Pick()` to `ep, release, err := rr.Pick()` and add `_ = release` (or assert it is non-nil). Then ADD:

```go
func TestRoundRobin_ReleaseIsNonNilNoop(t *testing.T) {
	rr := &roundRobin{endpoints: []Endpoint{{Host: "10.0.0.1", Port: 1}}}
	ep, release, err := rr.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "10.0.0.1" {
		t.Errorf("ep.Host = %q, want 10.0.0.1", ep.Host)
	}
	if release == nil {
		t.Fatal("release must be non-nil (interface contract)")
	}
	release() // must not panic and must be safe to call twice
	release()
}

func TestRoundRobin_PickSequenceUnchanged(t *testing.T) {
	// Behavior-neutrality of the reshape: first pick is endpoints[0], then mod-index.
	rr := &roundRobin{endpoints: []Endpoint{{Host: "a"}, {Host: "b"}, {Host: "c"}}}
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i, w := range want {
		ep, _, err := rr.Pick()
		if err != nil {
			t.Fatalf("pick %d: %v", i, err)
		}
		if ep.Host != w {
			t.Errorf("pick %d: got %q, want %q", i, ep.Host, w)
		}
	}
}
```

In `internal/cluster/cluster_test.go`, add a test-only stub LB whose release is OBSERVABLE (decoupled from `leastRequest`, which does not exist yet), and assert counter balance through the `cluster.go` threading. Use the package's existing cluster-construction test helper (or build a minimal `*Cluster` directly):

```go
// stubLB is a test-only loadBalancer whose pick increments an observable
// counter and whose release decrements it, so the cluster.go release threading
// (Dial / AcquireH1 / dial-failure / double-Close / closePool drain) is
// testable in isolation from leastRequest.
type stubLB struct {
	ep     Endpoint
	active atomic.Int64
}

func (s *stubLB) Pick() (Endpoint, func(), error) {
	s.active.Add(1)
	var once sync.Once
	return s.ep, func() { once.Do(func() { s.active.Add(-1) }) }, nil
}

func TestDial_ReleasesOnConnClose(t *testing.T) {
	// Listener that accepts and immediately closes — Dial succeeds.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() { for { c, err := ln.Accept(); if err != nil { return }; _ = c.Close() } }()

	stub := &stubLB{ep: addrToEndpoint(ln.Addr().String())}
	c := newTestCluster(t, stub) // helper: *Cluster with registered metrics + this lb
	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if got := stub.active.Load(); got != 1 {
		t.Fatalf("after Dial: active = %d, want 1 (held until Close)", got)
	}
	_ = conn.Close()
	if got := stub.active.Load(); got != 0 {
		t.Fatalf("after Close: active = %d, want 0", got)
	}
	_ = conn.Close() // double-Close must NOT underflow
	if got := stub.active.Load(); got != 0 {
		t.Fatalf("after double-Close: active = %d, want 0", got)
	}
}

func TestDial_ReleasesOnDialFailure(t *testing.T) {
	// Point at a port nothing listens on → dial fails → release MUST fire.
	stub := &stubLB{ep: Endpoint{Host: "127.0.0.1", Port: 1}} // port 1: refused
	c := newTestCluster(t, stub)
	_, _, err := c.Dial(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
	if got := stub.active.Load(); got != 0 {
		t.Errorf("after dial failure: active = %d, want 0 (release-on-failure)", got)
	}
}

func TestAcquireH1_PoolHitReleasesImmediately(t *testing.T) {
	// First AcquireH1 dials fresh (active=1 held by the conn). PutIdleH1 returns
	// it to the pool (still active=1 — cx-as-rq). Second AcquireH1 is a POOL HIT:
	// its fresh pick releases immediately, so active stays 1 (the pooled conn's
	// dial-time hold persists). Final Close → active 0.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() { for { c, err := ln.Accept(); if err != nil { return }; go io.Copy(io.Discard, c) } }()
	stub := &stubLB{ep: addrToEndpoint(ln.Addr().String())}
	c := newTestCluster(t, stub)

	p1, err := c.AcquireH1(context.Background())
	if err != nil { t.Fatalf("AcquireH1 miss: %v", err) }
	if got := stub.active.Load(); got != 1 { t.Fatalf("after dial: active=%d want 1", got) }
	c.PutIdleH1(p1)
	if got := stub.active.Load(); got != 1 { t.Fatalf("after PutIdle: active=%d want 1 (cx-as-rq hold persists)", got) }

	p2, err := c.AcquireH1(context.Background())
	if err != nil { t.Fatalf("AcquireH1 hit: %v", err) }
	if got := stub.active.Load(); got != 1 { t.Fatalf("after pool hit: active=%d want 1 (fresh pick released immediately)", got) }
	_ = p2.Conn.Close()
	if got := stub.active.Load(); got != 0 { t.Fatalf("after Close: active=%d want 0", got) }
}
```

(Add `newTestCluster`/`addrToEndpoint` helpers to `cluster_test.go` if not already present — `newTestCluster` builds a `*Cluster{name, endpoints, lb}` and calls `registerClusterMetrics(stats.NewRegistry(), c)` so the `upstreamCx*` gauges are non-nil.)

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd internal/cluster && go test ./... 2>&1 | head -30
```
Expected: COMPILE FAILURE first (`roundRobin.Pick` returns 2 values, interface wants 3) — that IS the red. After the interface edit in Step 3 the stub-LB tests fail until threading lands.

- [ ] **Step 3: Reshape the interface + thread release**

`loadbalancer.go` — the interface, the shared no-op, and `roundRobin.Pick`:
```go
// loadBalancer is the unexported per-cluster LB interface. Pick returns the
// selected endpoint plus a release func the caller MUST invoke exactly once
// when the picked unit of work completes (conn-producing paths: at final conn
// Close; non-conn paths: immediately). release is always non-nil; implementations
// guard against double-release. ADR-0232 (the LB acquire/release seam; OPTION C —
// the exported Cluster surface stays byte-stable).
type loadBalancer interface {
	Pick() (Endpoint, func(), error)
}

// noopRelease is the shared release for LB policies that hold no per-pick state
// (roundRobin). It is a no-op; the ADR-0024 per-cluster counter is untouched.
var noopRelease = func() {}
```
```go
func (rr *roundRobin) Pick() (Endpoint, func(), error) {
	if len(rr.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	i := rr.counter.Add(1) - 1
	return rr.endpoints[int(i)%len(rr.endpoints)], noopRelease, nil
}
```

`cluster.go` `PickEndpoint` (:173) — immediate release (D-S34-5; the two direct consumers stay source-compatible):
```go
// PickEndpoint selects the next upstream endpoint per the cluster's LB policy.
// Safe for concurrent use. The picked unit is released IMMEDIATELY: direct-pick
// consumers (httpclient ClusterDispatch, the thriftproxy no-healthy-host probe)
// have no observable conn lifecycle, so their load is invisible to least_request
// (a documented coverage note — SPEC §2 / §3.2). Dial / AcquireH1 do NOT route
// through here; they call c.lb.Pick() directly so they can hold the release.
func (c *Cluster) PickEndpoint() (Endpoint, error) {
	ep, release, err := c.lb.Pick()
	if err != nil {
		return Endpoint{}, err
	}
	release()
	return ep, nil
}
```

`Dial` (:198) — replace `ep, err := c.PickEndpoint()` with the direct `c.lb.Pick()`; release-on-failure; compose into `connWithGauge.dec`:
```go
	ep, release, err := c.lb.Pick()
	if err != nil {
		return nil, Endpoint{}, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		release()
		return nil, Endpoint{}, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			release()
			return nil, Endpoint{}, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	// D-S34-5: compose release into the existing connWithGauge dec closure. The
	// connWithGauge sync.Once guards BOTH the gauge Dec and the LB release, so
	// double-Close cannot double-release. The struct is unchanged.
	return &connWithGauge{Conn: final, dec: func() { c.upstreamCxActive.Dec(); release() }}, ep, nil
```

`AcquireH1` (:242) — pool-HIT branch releases immediately; pool-MISS mirrors `Dial`:
```go
	ep, release, err := c.lb.Pick()
	if err != nil {
		return nil, err
	}
	addr := ep.Addr()

	c.h1PoolMu.Lock()
	list := c.h1Pool[addr]
	n := len(list)
	if n > 0 {
		p := list[n-1]
		c.h1Pool[addr] = list[:n-1]
		c.h1PoolMu.Unlock()
		_ = p.Conn.SetDeadline(time.Time{})
		// Pool HIT: the pooled conn carries its DIAL-TIME hold (cx-as-rq — it
		// persists until final close, incl. PutIdleH1-overflow drop and closePool
		// drain). The fresh pick is redundant → release it immediately.
		release()
		return p, nil
	}
	c.h1PoolMu.Unlock()

	// Slow path: dial fresh (mirrors Dial verbatim — release-on-failure + dec composition).
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		release()
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			release()
			return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	wrapped := &connWithGauge{Conn: final, dec: func() { c.upstreamCxActive.Dec(); release() }}
	return &PooledH1Conn{
		Conn: wrapped,
		Br:   bufio.NewReaderSize(wrapped, 4096),
		ep:   ep,
	}, nil
```

(`DialH2`, tcpproxy, router H1/H2, grpcclient, the ADR-0230 pool dial closures need ZERO changes — they all close the `connWithGauge` wrapper, which now releases. Add a one-line doc comment at each if helpful, but no logic change.)

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS (roundRobin + stub-LB threading tests green).

- [ ] **Step 5: gofmt + lint (per-task discipline)**

```bash
gofmt -l internal/cluster/
golangci-lint run ./internal/cluster/...
```
Expected: empty `gofmt -l` output; clean lint.

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add internal/cluster/loadbalancer.go internal/cluster/cluster.go internal/cluster/loadbalancer_test.go internal/cluster/cluster_test.go
git commit -m "phase 34 Task 2: reshape loadBalancer seam to Pick() (Endpoint, func(), error); roundRobin no-op release; cluster.go release threading (ADR-0232 OPTION C; behavior-neutral)"
```

---

## Task 3: The `leastRequest` P2C type (`leastrequest.go`)

**Goal:** Implement un-weighted P2C mirroring Envoy v1.37.2 EXACTLY (AMEND-L6), with per-endpoint `atomic.Int64` counters, an injectable RNG, and a `sync.Once`-guarded release.

**Files:**
- Create: `internal/cluster/leastrequest.go`
- Create: `internal/cluster/leastrequest_test.go`

- [ ] **Step 1: Write the failing tests** (`leastrequest_test.go`)

```go
package cluster

import (
	"sync"
	"testing"
)

// seqRNG returns a deterministic rng closure yielding the given values in order,
// then repeating — the upstream mock-RNG posture (AMEND-L6).
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

func TestLeastRequest_FirstDrawnWinsTies(t *testing.T) {
	// All counters 0; choiceCount 2; draws {0, 1}. Strict < keeps the first-drawn
	// (index 0) on a tie. winner = endpoints[0].
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 1))
	ep, _, err := lr.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "a" {
		t.Errorf("tie: got %q, want a (first-drawn wins)", ep.Host)
	}
}

func TestLeastRequest_PicksFewestActive(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 2))
	lr.active[0].Store(5) // endpoint a is loaded
	ep, _, err := lr.Pick()
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if ep.Host != "c" { // draws {0(active 5), 2(active 0)} → strict < → c
		t.Errorf("got %q, want c (fewest active)", ep.Host)
	}
}

func TestLeastRequest_WithReplacementNoClamp(t *testing.T) {
	// choiceCount 5 > n 2: draws sample WITH replacement, no clamp, no panic.
	lr := newLeastRequestWithRNG(eps(2), 5, seqRNG(0, 0, 1, 1, 0))
	if _, _, err := lr.Pick(); err != nil {
		t.Fatalf("Pick with choiceCount>n: %v", err)
	}
}

func TestLeastRequest_IncDecBalance(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 0))
	_, release, _ := lr.Pick()
	if got := lr.active[0].Load(); got != 1 {
		t.Fatalf("after Pick: active[0] = %d, want 1", got)
	}
	release()
	if got := lr.active[0].Load(); got != 0 {
		t.Fatalf("after release: active[0] = %d, want 0", got)
	}
}

func TestLeastRequest_DoubleReleaseGuard(t *testing.T) {
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 0))
	_, release, _ := lr.Pick()
	release()
	release() // sync.Once: second call is a no-op
	if got := lr.active[0].Load(); got != 0 {
		t.Errorf("after double-release: active[0] = %d, want 0 (no underflow)", got)
	}
}

func TestLeastRequest_NoEndpoints(t *testing.T) {
	lr := newLeastRequestWithRNG(nil, 2, seqRNG(0))
	_, release, err := lr.Pick()
	if err != errNoEndpoints {
		t.Errorf("err = %v, want errNoEndpoints", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error (interface contract)")
	}
}

func TestNewLeastRequest_ProductionRNGSeeds(t *testing.T) {
	// Smoke: the crypto-seeded production constructor succeeds and Pick works.
	lr, err := newLeastRequest(eps(3), 2)
	if err != nil {
		t.Fatalf("newLeastRequest: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ { // concurrency smoke (mutex-guarded rng)
		wg.Add(1)
		go func() { defer wg.Done(); _, rel, _ := lr.Pick(); rel() }()
	}
	wg.Wait()
	for i := range lr.active {
		if got := lr.active[i].Load(); got != 0 {
			t.Errorf("active[%d] = %d, want 0 after balanced pick/release", i, got)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestLeastRequest ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`leastRequest`/`newLeastRequest` undefined).

- [ ] **Step 3: Implement `leastrequest.go`**

```go
package cluster

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	mathrand "math/rand/v2"
	"sync"
	"sync/atomic"
)

// leastRequest is an un-weighted power-of-two-choices (P2C) load balancer that
// mirrors Envoy v1.37.2's LeastRequestLoadBalancer::unweightedHostPickNChoices
// (SPEC §3.1 / AMEND-L6): choiceCount independent draws of rng()%n WITH
// replacement (no dedup, no clamp at choiceCount >= n); pick the host with the
// fewest outstanding active counts; STRICT < comparison on the RAW active count;
// the FIRST-DRAWN candidate wins ties. The per-endpoint active counters are
// LB-internal state, NOT registry stats (SPEC §7 — zero stat delta). cx-as-rq
// (AMEND-L2): each pick holds +1 on its endpoint from selection until the
// returned release fires (conn-producing paths: at final conn Close).
//
// The weighted bias formula (weight/(active+1)^bias) belongs to Envoy's EDF
// path, which engages ONLY with unequal host weights or active slow-start —
// the equal-weight static MVP takes pure P2C and active_request_bias never
// enters the computation (AMEND-L6; bias/slow_start are DEPARTURE-rejected at
// parse time — manager.go).
type leastRequest struct {
	endpoints   []Endpoint
	active      []atomic.Int64 // index-aligned with endpoints
	choiceCount int
	rng         func() uint64 // injectable for deterministic tests (the upstream mock posture)
}

// newLeastRequest constructs a leastRequest over endpoints with the given P2C
// choiceCount, seeding a crypto-seeded math/rand/v2 PCG once at construction
// (D-S34-2). The crypto/rand read error is threaded out (effectively unreachable
// on Linux getrandom; boot-fail is the safe disposition). Callers in
// manager.buildCluster surface the error as a cluster build error.
func newLeastRequest(endpoints []Endpoint, choiceCount int) (*leastRequest, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newLeastRequestWithRNG(endpoints, choiceCount, rng), nil
}

// newLeastRequestWithRNG is the injectable constructor used by unit tests to
// supply a deterministic draw sequence (AMEND-L6).
func newLeastRequestWithRNG(endpoints []Endpoint, choiceCount int, rng func() uint64) *leastRequest {
	return &leastRequest{
		endpoints:   endpoints,
		active:      make([]atomic.Int64, len(endpoints)),
		choiceCount: choiceCount,
		rng:         rng,
	}
}

// newPCGRNG seeds a math/rand/v2 PCG from two crypto/rand uint64 words and
// returns a MUTEX-GUARDED draw closure. Pick is called concurrently across
// downstream connections; math/rand/v2.Rand is not per-source concurrency-safe,
// so the mutex serializes draws (cheap; per-cluster contention only). Each
// cluster gets its own independent stream.
func newPCGRNG() (func() uint64, error) {
	var seed [16]byte
	if _, err := cryptorand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("cluster: least_request: seed rng: %w", err)
	}
	r := mathrand.New(mathrand.NewPCG(
		binary.LittleEndian.Uint64(seed[0:8]),
		binary.LittleEndian.Uint64(seed[8:16]),
	))
	var mu sync.Mutex
	return func() uint64 {
		mu.Lock()
		defer mu.Unlock()
		return r.Uint64()
	}, nil
}

// Pick implements loadBalancer. See the type doc for the P2C semantics.
func (lr *leastRequest) Pick() (Endpoint, func(), error) {
	n := len(lr.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	best := int(lr.rng() % uint64(n))
	bestActive := lr.active[best].Load()
	for i := 1; i < lr.choiceCount; i++ {
		cand := int(lr.rng() % uint64(n))
		candActive := lr.active[cand].Load()
		if candActive < bestActive { // STRICT <: first-drawn keeps ties (AMEND-L6)
			best, bestActive = cand, candActive
		}
	}
	lr.active[best].Add(1)
	var once sync.Once
	release := func() { once.Do(func() { lr.active[best].Add(-1) }) }
	return lr.endpoints[best], release, nil
}
```

Notes:
- `choiceCount` is `>= 2` by construction (parse-gated in Task 4); the loop runs `choiceCount-1` comparison draws after the initial draw. For the defensive `choiceCount < 1` case the loop simply doesn't execute (best = the single initial draw).
- The `sync.Once` here is independent of `connWithGauge.once` — defense in depth (PickEndpoint's immediate-release path has no connWithGauge wrapper).

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestLeastRequest ./... 2>&1 | tail
cd internal/cluster && go test -run TestNewLeastRequest -race ./... 2>&1 | tail   # -race exercises the mutex-guarded rng
```
Expected: PASS, no race.

- [ ] **Step 5: gofmt + lint**

```bash
gofmt -l internal/cluster/
golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add internal/cluster/leastrequest.go internal/cluster/leastrequest_test.go
git commit -m "phase 34 Task 3: leastRequest P2C type (with-replacement, strict <, first-drawn ties, no-clamp; injectable crypto-seeded RNG; sync.Once release) — mirrors v1.37.2 source (AMEND-L6)"
```

---

## Task 4: Manager acceptance + `parseLeastRequestLbConfig` + the §6 reject matrix

**Goal:** `buildCluster` accepts `LEAST_REQUEST`, constructs `leastRequest`, applies the §6 reject matrix (the NEW lb-policy reject text; the hand-rolled `choice_count >= 2` gate; the `active_request_bias`/`slow_start_config` DEPARTURE rejects; the mismatched-oneof silent-ignore).

**Files:**
- Modify: `internal/cluster/manager.go` (the guard at :215-217 + the lb literal at :230-235; add `parseLeastRequestLbConfig` + `defaultChoiceCount`)
- Modify: `internal/cluster/manager_test.go` (retarget `TestManager_Error_NonRoundRobinLB`; add the accept/reject matrix)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

Add `wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"` to the imports. Helper to set the oneof (verified generated names: `Cluster_LeastRequestLbConfig_` wraps field `LeastRequestLbConfig *Cluster_LeastRequestLbConfig`; `ChoiceCount *wrapperspb.UInt32Value`):

```go
func mkLeastRequest(name string, cc *wrapperspb.UInt32Value, eps ...*endpointv3.LbEndpoint) *clusterv3.Cluster {
	c := mkStaticCluster(name, eps...)
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: cc},
	}
	return c
}

func TestManager_Accept_LeastRequest_NoConfig(t *testing.T) {
	c := mkStaticCluster("c_lr", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST // no lb_config → default choice_count 2
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("LEAST_REQUEST bare must be accepted (default cc=2): %v", err)
	}
}

func TestManager_Accept_LeastRequest_ChoiceCounts(t *testing.T) {
	for _, cc := range []uint32{2, 100} { // 100 = no clamp (reference parity, AMEND-L3)
		c := mkLeastRequest("c_lr", wrapperspb.UInt32(cc), mkLbEndpoint("127.0.0.1", 8080))
		if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
			t.Errorf("choice_count %d must be accepted: %v", cc, err)
		}
	}
}

func TestManager_Error_LeastRequest_ChoiceCountTooSmall(t *testing.T) {
	for _, cc := range []uint32{0, 1} {
		c := mkLeastRequest("c_lr", wrapperspb.UInt32(cc), mkLbEndpoint("127.0.0.1", 8080))
		_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
		if err == nil {
			t.Fatalf("choice_count %d must be rejected", cc)
		}
		if !strings.Contains(err.Error(), "value must be greater than or equal to 2") {
			t.Errorf("cc=%d: error %q missing PGV-parity substring", cc, err.Error())
		}
	}
}

func TestManager_Error_LeastRequest_BiasUnsupported(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(2), mkLbEndpoint("127.0.0.1", 8080))
	c.GetLeastRequestLbConfig().ActiveRequestBias = &corev3.RuntimeDouble{DefaultValue: 1.5, RuntimeKey: "arb"}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "active_request_bias") {
		t.Errorf("active_request_bias under LEAST_REQUEST must be rejected; got %v", err)
	}
}

func TestManager_Error_LeastRequest_SlowStartUnsupported(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(2), mkLbEndpoint("127.0.0.1", 8080))
	c.GetLeastRequestLbConfig().SlowStartConfig = &clusterv3.Cluster_SlowStartConfig{}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "slow_start_config") {
		t.Errorf("slow_start_config under LEAST_REQUEST must be rejected; got %v", err)
	}
}

func TestManager_Accept_MismatchedOneof_RoundRobin(t *testing.T) {
	// least_request_lb_config under ROUND_ROBIN → silent-ignore (reference parity, §6.3).
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 8080)) // LbPolicy ROUND_ROBIN
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)},
	}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under ROUND_ROBIN must be silently accepted: %v", err)
	}
}

func TestManager_Error_UnsupportedLBPolicy(t *testing.T) { // RETARGET of TestManager_Error_NonRoundRobinLB
	c := mkStaticCluster("c_x", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RANDOM // LEAST_REQUEST now accepted → retarget to a still-rejected policy
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("RANDOM must be rejected")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST") {
		t.Errorf("error %q missing new supported-set substring", err.Error())
	}
}
```

DELETE the old `TestManager_Error_NonRoundRobinLB` (replaced by `TestManager_Error_UnsupportedLBPolicy`).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run "TestManager_(Accept|Error)" ./... 2>&1 | head -30
```
Expected: the LEAST_REQUEST accept tests FAIL (current guard rejects it); `TestManager_Error_UnsupportedLBPolicy` FAILS the substring (old text lacks "LEAST_REQUEST").

- [ ] **Step 3: Implement the manager changes**

In `manager.go`, add the constant + parser (near the other build helpers):
```go
// defaultChoiceCount is the P2C sample size when least_request_lb_config is
// absent or its choice_count is unset (the proto doc-comment default; reference
// parity — SPEC §5.1 / §6.3).
const defaultChoiceCount = 2

// parseLeastRequestLbConfig extracts the P2C choice_count for a LEAST_REQUEST
// cluster and applies the SPEC §6 reject matrix. GetLeastRequestLbConfig() is
// nil-safe (nil on an absent OR a mismatched lb_config oneof member — AMEND-L1):
// both fall to defaultChoiceCount (§6.3 silent-ignore parity). The choice_count
// gate is HAND-ROLLED to mirror the PGV string (the manager calls no PGV —
// AMEND-L1). active_request_bias / slow_start_config set under LEAST_REQUEST are
// behavior-bearing DEPARTURE rejects (AMEND-L5/L6 — the reference parse-accepts).
func parseLeastRequestLbConfig(c *clusterv3.Cluster, name string) (int, error) {
	lrc := c.GetLeastRequestLbConfig()
	if lrc == nil {
		return defaultChoiceCount, nil
	}
	if lrc.GetActiveRequestBias() != nil {
		return 0, fmt.Errorf("cluster: %q: least_request_lb_config.active_request_bias is not supported", name)
	}
	if lrc.GetSlowStartConfig() != nil {
		return 0, fmt.Errorf("cluster: %q: least_request_lb_config.slow_start_config is not supported", name)
	}
	if cc := lrc.GetChoiceCount(); cc != nil {
		if v := cc.GetValue(); v < 2 {
			return 0, fmt.Errorf("cluster: %q: least_request_lb_config.choice_count: value must be greater than or equal to 2", name)
		} else { //nolint:revive // explicit branch reads clearer alongside the reject above
			return int(v), nil // no clamp at > len(endpoints) — reference parity (AMEND-L3)
		}
	}
	return defaultChoiceCount, nil
}
```

Replace the guard (current :215-217) + remove `lb:` from the `cl :=` literal (:234), then set `cl.lb` via a switch AFTER the literal (after `cl := &Cluster{...}` at ~:230-235, before the transport_socket block at :236):
```go
	cl := &Cluster{
		name:           name,
		endpoints:      endpoints,
		connectTimeout: timeout,
		// lb set by the policy switch below (ADR-0233; SPEC §3.4)
	}
	switch c.GetLbPolicy() {
	case clusterv3.Cluster_ROUND_ROBIN:
		cl.lb = &roundRobin{endpoints: endpoints}
	case clusterv3.Cluster_LEAST_REQUEST:
		cc, err := parseLeastRequestLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		lb, err := newLeastRequest(endpoints, cc)
		if err != nil {
			return nil, err
		}
		cl.lb = lb
	default:
		// The ONE deliberate byte-stable-reject change this phase (ADR-0080;
		// blast radius AMEND-L5). An envoy-go-strict DEPARTURE for RANDOM/
		// RING_HASH/MAGLEV (the reference validate-accepts them — recorded in
		// BEHAVIOR_CONTRACT).
		return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST)", name, c.GetLbPolicy())
	}
```

**Ordering note:** the policy switch now sits AFTER `extractEndpoints` (it needs `endpoints`), so for a cluster with BOTH a bad lb_policy AND a bad load_assignment the load_assignment error fires first (previously the lb_policy error fired first). Grep confirms no test pins that combined ordering (`TestManager_Error_NonRoundRobinLB`/its replacement uses a valid endpoint). Update the stale doc comment at `manager.go:42-43` ("lb_policy must be unset ... or explicitly ROUND_ROBIN. Anything else errors.") to name LEAST_REQUEST, and refresh the `loadbalancer.go:5-7` comment.

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint**

```bash
gofmt -l internal/cluster/
golangci-lint run ./internal/cluster/...
```

- [ ] **Step 6: Commit (LOCAL-ONLY)**

```bash
git add internal/cluster/manager.go internal/cluster/manager_test.go internal/cluster/loadbalancer.go
git commit -m "phase 34 Task 4: manager accepts LEAST_REQUEST; parseLeastRequestLbConfig (hand-rolled cc>=2; bias/slow_start DEPARTURE rejects; mismatched-oneof silent-ignore); NEW lb-policy reject text; retarget the unsupported-policy test (SPEC §6)"
```

---

## Task 5: In-process skew integration test + LEAST_REQUEST boot smoke

**Goal:** Prove end-to-end that held picks elevate counters and subsequent picks avoid the loaded endpoint (deterministic RNG), and that a LEAST_REQUEST bootstrap builds a working Manager.

**Files:**
- Modify: `internal/cluster/leastrequest_test.go` (the skew integration test)
- Modify: `internal/cluster/manager_test.go` (the boot smoke test)

- [ ] **Step 1: Write the failing tests**

```go
// leastrequest_test.go
func TestLeastRequest_SkewAvoidsLoadedEndpoint(t *testing.T) {
	// Deterministic RNG cycling 0,1,2,0,1,2,...; choiceCount 2. Hold 3 picks on
	// endpoint a (index 0) by NOT releasing, then verify the next picks land on
	// the under-loaded endpoints b/c, never a (a's active count dominates every
	// 2-draw sample that includes it).
	// Every sample draws indices {0,1} (choiceCount 2). Holding picks WITHOUT
	// releasing accumulates load on the tied-then-heaviest endpoints:
	//   pick 1: {0:0, 1:0} tie → a (first-drawn);    active=[1,0,0]
	//   pick 2: {0:1, 1:0}     → b (strict <);        active=[1,1,0]
	//   pick 3: {0:1, 1:1} tie → a (first-drawn);     active=[2,1,0]
	lr := newLeastRequestWithRNG(eps(3), 2, seqRNG(0, 1, 0, 1, 0, 1))
	held := make([]func(), 0, 3)
	for i := 0; i < 3; i++ {
		_, rel, _ := lr.Pick()
		held = append(held, rel) // hold (never release) to keep the load elevated
	}
	if got := lr.active[0].Load(); got < 1 {
		t.Fatalf("endpoint a should carry held load, active[0]=%d", got)
	}
	// Now a (index 0, active 2) is heaviest. The next sample {0:2, 1:1} → strict <
	// selects b. The load-bearing assertion: the heaviest endpoint is NOT re-picked.
	ep, rel, _ := lr.Pick()
	rel()
	if ep.Host == "a" {
		t.Errorf("loaded endpoint a was re-picked over lighter b; skew not working")
	}
	for _, r := range held {
		r()
	}
}
```

(Tune the `seqRNG` sequence and assertions during IMPL so the test is deterministic and non-vacuous — verify it FAILS if the strict-`<` comparison is inverted. This in-process test is the unit-level analogue of the `0059` band; the differential band is Task 6/7.)

```go
// manager_test.go — boot smoke from a realistic LEAST_REQUEST cluster.
func TestManager_LeastRequest_BootSmoke(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(10), // cc=10 (the 0059 config)
		mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cl, ok := m.Get("c_lr")
	if !ok {
		t.Fatal("cluster c_lr not found")
	}
	ep, err := cl.PickEndpoint() // exercises the immediate-release path
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if ep.Port < 9001 || ep.Port > 9003 {
		t.Errorf("picked out-of-range endpoint %v", ep)
	}
}
```

- [ ] **Step 2: Run to verify they fail / pass-by-design**

```bash
cd internal/cluster && go test -run "TestLeastRequest_Skew|TestManager_LeastRequest_BootSmoke" -count=1 ./... 2>&1 | tail
```
Expected: green once the deterministic sequence is tuned (Step 1 may pass immediately given Tasks 3-4 landed — that is acceptable for an integration test; the liveness proof is the `-count=1` inverted-comparison break in Step 3).

- [ ] **Step 3: Prove the skew test is live (`-count=1`)**

Temporarily invert the `leastRequest.Pick` comparison (`candActive > bestActive`), run `go test -run TestLeastRequest_Skew -count=1`, confirm it FAILS, then REVERT. Record in PROGRESS.md (`reference_differential_break_protocol_count1`).

- [ ] **Step 4: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/leastrequest_test.go internal/cluster/manager_test.go
git commit -m "phase 34 Task 5: in-process P2C skew integration test (heaviest endpoint stops being picked; -count=1 inverted-comparison liveness proven) + LEAST_REQUEST boot smoke"
```

---

## Task 6: The `0059-lb-least-request` differential fixture

**Goal:** A cross-side `[tcp_proxy]` fixture over a 3-endpoint LEAST_REQUEST cluster (`choice_count: 10`), with a hold-4 + burst-60 + drain workload, a band-based `AssertDistribution`, and a cross-side `StatsAsserter` prong. NO new BackendKind (reuses `TCPEcho = 0`). NO boot-reject dir.

**Files:**
- Create: `test/fixtures/0059-lb-least-request/driver/driver.go`
- Create: `test/fixtures/0059-lb-least-request/driver/driver_test.go` (the per-fixture registration smoke, mirroring `0001`)
- Create: `test/fixtures/0059-lb-least-request/README.md`
- Create: `test/fixtures/0059-lb-least-request/expectations.yaml`

- [ ] **Step 1: Write the driver** (`driver/driver.go`)

Model on `0001`'s driver. Key differences: `LbPolicy: LEAST_REQUEST` + `least_request_lb_config: { choice_count: 10 }` on BOTH bootstraps; a hold-4 + burst-60 + drain `DriveReference`/`DriveSubject`; a band `AssertDistribution`; a `StatsAsserter`.

```go
package driver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const fixtureName = "0059-lb-least-request"
const refContainerListenerPort = 15001
const (
	heldConns  = 4  // K (D-S34-3)
	burstConns = 60 // S (D-S34-3)
	totalConns = heldConns + burstConns // 64 (conservation target)
	clusterName = "c_echo"
)

func init() { fixture.RegisterFixture(fixtureName, &lrDriver{}) }

type lrDriver struct{}

func (lrDriver) BackendCount() int           { return 3 }
func (lrDriver) SubjectListenerName() string { return "l_tcp" }
func (lrDriver) ReferenceListenerPort() int  { return refContainerListenerPort }

func (lrDriver) ReferenceBootstrap(backendPorts []int) string {
	// STRICT_DNS + host.docker.internal (the 0001 reference shape) + LEAST_REQUEST.
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 0.0.0.0, port_value: 15001 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: LEAST_REQUEST
      least_request_lb_config: { choice_count: 10 }
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`, backendPorts[0], backendPorts[1], backendPorts[2])
}

func (lrDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	// STATIC + 127.0.0.1 (the 0001 subject shape) + LEAST_REQUEST.
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0059, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      lb_policy: LEAST_REQUEST
      least_request_lb_config: { choice_count: 10 }
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`, subjAdminPort, subjListenerPort, backendPorts[0], backendPorts[1], backendPorts[2])
}

// drive runs the hold-4 + burst-60 + drain workload against addr and returns the
// concatenated echo bytes. Identical per side (deterministic payloads → byte-equal).
func drive(ctx context.Context, addr string) ([]byte, error) {
	var sb strings.Builder
	// 1. HOLD phase: open K conns; write+read-echo each (the establishment
	//    witness — D-S34-4: proves the upstream dial completed and the pick's
	//    active count is held), then KEEP the socket open.
	held := make([]net.Conn, 0, heldConns)
	defer func() { for _, c := range held { _ = c.Close() } }() // 3. DRAIN (deferred)
	for i := 0; i < heldConns; i++ {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return nil, fmt.Errorf("hold dial[%d]: %w", i, err)
		}
		payload := []byte(fmt.Sprintf("hold-%d\n", i))
		if _, err := c.Write(payload); err != nil {
			return nil, fmt.Errorf("hold write[%d]: %w", i, err)
		}
		buf := make([]byte, len(payload))
		if _, err := readFull(ctx, c, buf); err != nil { // establishment witness
			return nil, fmt.Errorf("hold echo[%d]: %w", i, err)
		}
		sb.Write(buf)
		held = append(held, c)
	}
	// 2. BURST phase: S sequential short round-trips (close-accounting between picks).
	for i := 0; i < burstConns; i++ {
		b, err := helpers.TCPRoundTrip(ctx, addr, []byte(fmt.Sprintf("burst-%d\n", i)), time.Second)
		if err != nil {
			return nil, fmt.Errorf("burst[%d]: %w", i, err)
		}
		sb.Write(b)
	}
	return []byte(sb.String()), nil
}

func (lrDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) { return drive(ctx, addr) }
func (lrDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error)   { return drive(ctx, addr) }
```

(`readFull` is a small helper reading exactly `len(buf)` bytes under a deadline — or reuse an existing `helpers.*` read. Verify the right helper at IMPL; `helpers.TCPRoundTrip` half-closes and reads to EOF, so it cannot be reused for a held conn — the hold phase needs a write + bounded read that LEAVES the socket open.)

- [ ] **Step 2: Write the band `AssertDistribution`**

```go
// AssertDistribution: PER-SIDE band check on the sorted per-backend accept counts
// (the runner snapshots backend.accepts after Drive). NEVER cross-side-exact
// (independent RNG streams — the BRAINSTORM band-semantics decision; the 0003
// per-side-asymmetry precedent). The FIRST band-based AssertDistribution.
//
//   conservation:  c1 + c2 + c3 == 64
//   starvation:    c1 <= 12   (the most-held backend gets ~its held conns + ~0 burst;
//                              under ROUND_ROBIN c1 == 21 → BITES the no-op-release break)
//   concentration: c2 >= 16   (the two least-loaded split the burst; catches an
//                              INVERTED comparison, where c2 would be ~1)
func (lrDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != 3 {
			return fmt.Errorf("%s: expected 3 backend counts, got %d", sd.side, len(sd.counts))
		}
		c := []uint64{sd.counts[0], sd.counts[1], sd.counts[2]}
		sortAsc(c) // c[0] <= c[1] <= c[2]
		if c[0]+c[1]+c[2] != totalConns {
			return fmt.Errorf("%s: conservation: sum %d != %d", sd.side, c[0]+c[1]+c[2], totalConns)
		}
		if c[0] > 12 {
			return fmt.Errorf("%s: starvation: c1=%d > 12 (no skew?)", sd.side, c[0])
		}
		if c[1] < 16 {
			return fmt.Errorf("%s: concentration: c2=%d < 16 (inverted comparison?)", sd.side, c[1])
		}
	}
	return nil
}
```

- [ ] **Step 3: Write the `StatsAsserter`** (the §7 cross-vs-per-side set; pattern from `0057`'s `scrapeStats`)

```go
// AssertStats (post-drain): SPEC §7 — cross-equal upstream_cx_total==64 +
// membership_total==3 + upstream_cx_active==0 (quiesced); PER-SIDE
// upstream_rq_total (ref=64 [tcp_proxy charges rq-per-cx, AMEND-L2]; subj=0
// [the tcpproxy path never calls IncUpstreamRqTotal — a pre-existing boundary]).
func (lrDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ref, err := scrapeStats(refAdminAddr) // copy 0057's scrapeStats verbatim
	if err != nil { t.Fatalf("scrape ref: %v", err) }
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil { t.Fatalf("scrape subj: %v", err) }

	pfx := "cluster." + clusterName + "."
	// Cross-equal.
	for _, p := range []struct{ name string; want uint64 }{
		{pfx + "upstream_cx_total", totalConns},
		{pfx + "membership_total", 3},
		{pfx + "upstream_cx_active", 0},
	} {
		rv, subjv := ref[p.name], subj[p.name]
		if rv != subjv { t.Errorf("cross-side mismatch %s: ref=%d subj=%d", p.name, rv, subjv) }
		if rv != p.want { t.Errorf("ref %s = %d, want %d", p.name, rv, p.want) }
		if subjv != p.want { t.Errorf("subj %s = %d, want %d", p.name, subjv, p.want) }
	}
	// Per-side upstream_rq_total (NOT cross-equal — AMEND-L4).
	const rqKey = "cluster." + clusterName + ".upstream_rq_total"
	if got := ref[rqKey]; got != totalConns {
		t.Errorf("ref %s = %d, want %d (rq-per-cx)", rqKey, got, totalConns)
	}
	if got := subj[rqKey]; got != 0 {
		t.Errorf("subj %s = %d, want 0 (tcpproxy never Inc's upstream_rq_total)", rqKey, got)
	}
}
```

Add `ProbeAdmin` (copy `0001`'s verbatim), the `sortAsc` helper, the `scrapeStats` helper (copy `0057`'s verbatim), and the compile-time interface checks:
```go
var (
	_ fixture.Driver               = (*lrDriver)(nil)
	_ fixture.DistributionAsserter = (*lrDriver)(nil)
	_ fixture.StatsAsserter        = (*lrDriver)(nil)
)
```

- [ ] **Step 4: Write README.md + expectations.yaml + driver_test.go**

- `README.md`: the workload (hold-4/burst-60/drain), the band rationale (conservation/starvation/concentration), the cc=10 choice (legal > endpoint-count, reference-parity, robust band), the per-side RNG non-equivalence, the deliberate-break record (Task 7), and the per-side `upstream_rq_total` boundary. Note the firsts: FIRST band-based `AssertDistribution`; NO new BackendKind; NO new fuzzer.
- `expectations.yaml`: mirror the `0001` shape (the cross-side byte-equivalence + the asserter dispatch are in-band; document the band + stats prongs).
- `driver_test.go`: the registration smoke (mirror `0001`'s `driver_test.go`).

- [ ] **Step 5: Run the fixture (requires Docker + the contrib reference image)**

```bash
go test ./test/differential/ -run '0059' -count=1 -v 2>&1 | tail -40
```
Expected: PASS (byte-equivalence + band per side + stats prong). If Docker is unavailable in the IMPL environment, the controller runs this gate where Docker is present (per `reference_docker_probe_bridge_network`).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0059-lb-least-request/ && golangci-lint run ./test/...
git add test/fixtures/0059-lb-least-request/
git commit -m "phase 34 Task 6: 0059-lb-least-request differential fixture — hold-4/burst-60/drain over LEAST_REQUEST cc=10; FIRST band-based AssertDistribution (conservation/starvation/concentration) + cross-vs-per-side StatsAsserter; reuses TCPEcho (no new BackendKind)"
```

---

## Task 7: Band-constant tuning + deliberate-break liveness (`-count=1`)

**Goal:** Finalize the band constants against repeat runs and PROVE each prong is live (a band that cannot fail is a dead assertion — `reference_differential_break_protocol_count1` generalized).

**Files:** (tuning only — adjust constants in `driver/driver.go`; record in `README.md` + PROGRESS.md)

- [ ] **Step 1: Repeat-run flake check (≥20 per side)**

```bash
go test ./test/differential/ -run '0059' -count=20 2>&1 | tail
```
Expected: 20/20 PASS. If any run flakes, widen the band within the pinned PRINCIPLE {conservation + starvation + concentration} (e.g. `c1 <= 14`, `c2 >= 14`) — never so wide it stops biting the breaks in Steps 2-4. Record the final constants + the observed min/max per side.

- [ ] **Step 2: Deliberate break (i) — inverted comparison → concentration leg FAILS**

In `leastRequest.Pick`, flip `candActive < bestActive` to `>`. Run:
```bash
go test ./test/differential/ -run '0059' -count=1 2>&1 | tail
```
Expected: FAIL on the `concentration: c2 < 16` leg (picks concentrate on the heaviest → the two light backends barely move). REVERT.

- [ ] **Step 3: Deliberate break (ii) — no-op release → starvation leg FAILS (the canonical break)**

In `leastRequest.Pick`, make `release` a no-op (`release := func() {}`) so counters never decrement → cumulative picks level toward uniform {~21,~21,~22}. Run with `-count=1`:
```bash
go test ./test/differential/ -run '0059' -count=1 2>&1 | tail
```
Expected: FAIL on the `starvation: c1 > 12` leg (no skew → c1 ≈ 21). REVERT. (This is the canonical deterministic break; the never-incremented-counter variant degenerates to ~multinomial and only fails ~97% of runs — NOT used as the proof.)

- [ ] **Step 4: Deliberate break (iii) — stats prong**

Temporarily corrupt one cross-equal want (e.g. `upstream_cx_total` want 64 → 99) OR drop an Inc — confirm the `StatsAsserter` FAILS with `-count=1`, then REVERT. (Verifies the stats prong is non-vacuous.)

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the three break results + the final constants in `README.md` (driver comments) and PROGRESS.md (the `0030` dead-assertion lesson).
```bash
go test ./test/differential/ -run '0059' -count=1 2>&1 | tail   # confirm green after all reverts
git add test/fixtures/0059-lb-least-request/
git commit -m "phase 34 Task 7: 0059 band tuning (K=4/S=60/c1<=12/c2>=16; 20/20 flake-free per side) + 3 deliberate-break liveness proofs (inverted-comparison/no-op-release/stats-prong, -count=1)"
```

---

## Task 8: Full differential re-verify + race + conformance unaffected (the seam-reshape REAL guard)

**Goal:** Prove the seam reshape kept all 60 prior fixtures byte-exact (the behavior-neutrality guard) and `0059` is green; run `-race`; assert h2spec/proxy-wasm unaffected.

**Files:** none (verification only)

- [ ] **Step 1: Full differential suite (61 dirs)**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -30
```
Expected: ALL 61 PASS — the 60 prior dirs byte-exact through the `roundRobin` no-op-release adoption + `0059` green. **This is the six-gate REAL guard for the reshape** (any RR pick-sequence drift would surface here).

- [ ] **Step 2: Race + short across the repo**

```bash
go test -race -short ./... 2>&1 | tail -20
```
Expected: PASS, no race (the mutex-guarded RNG + the atomic counters + the connWithGauge sync.Once).

- [ ] **Step 3: Build + vet + gofmt + lint (full repo)**

```bash
go build ./... && go vet ./... && gofmt -l internal/cluster/ test/fixtures/0059-lb-least-request/ && golangci-lint run ./... 2>&1 | tail
go mod tidy -diff && echo "TIDY_CLEAN"   # ZERO new go.mod dep (AMEND-L1)
```
Expected: clean; `go mod tidy -diff` empty.

- [ ] **Step 4: Conformance unaffected**

Re-run (or assert-unaffected with rationale: phase 34 touches no HTTP/h2/proxy-wasm path) h2spec **53/53** + proxy-wasm **10/10** per the repo's conformance recipe. Record in PROGRESS.md.

- [ ] **Step 5: Commit (LOCAL-ONLY) — verification evidence into PROGRESS.md**

```bash
git add docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md
git commit -m "phase 34 Task 8: six-gate evidence — 61-dir differential byte-exact (60 prior unchanged + 0059), -race -short green, go mod tidy clean, h2spec 53/53 + proxy-wasm 10/10 unaffected"
```

---

## Task 9: Completion bundle (ADR-0052 atomic landing)

**Goal:** Land the BEHAVIOR_CONTRACT delta, the ADR-0232/0233 bodies, and the STATE/ROADMAP advance — atomically with the code.

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/34-load-balancer-least-request/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md (SPEC §9)**

Add a least_request subsection beside the TCP-proxy/RR LB boundary: LEAST_REQUEST acceptance (`choice_count` default 2 / `>= 2` when set / no clamp); the P2C semantics (with-replacement, strict `<`, first-drawn ties — the v1.37.2 mirror); cx-as-rq active-count semantics (TCP exact; the HTTP idle-pooled-conn approximation boundary); the per-side RNG non-equivalence (band-proven). Update the line-897 deferral list (retire LEAST_REQUEST; the 7 candidates remain). Add departure/coverage records: the NEW lb-policy reject text (RANDOM/RING_HASH/MAGLEV rejected where the reference accepts); the bias/slow_start DEPARTURE rejects; the mismatched-oneof silent-ignore (parity); the direct-pick consumers' load invisibility; `upstream_rq_total` per-side on the TCP path; NO new fuzzer/BackendKind (deliberate firsts); stat surface UNCHANGED at 1116.

- [ ] **Step 2: DECISIONS.md — ADR-0232 + ADR-0233 §Decision/§Consequences bodies (ADR-0044 in-place)**

Complete the §Context drafts anchored at the SPEC: **ADR-0232** (the LB acquire/release seam — OPTION C; ADR-0024 cross-referenced not amended; the ADR-0063 `connWithGauge` `dec` composition; the family's durable asset) + **ADR-0233** (the least_request policy — un-weighted P2C; the accept/reject matrix; the band-based differential proof shape; the no-fuzzer/no-BackendKind firsts). Tail stays **ADR-0233**; next-free **ADR-0234**.

- [ ] **Step 3: STATE.md + ROADMAP.md**

STATE active-phase → `phase 34 (load-balancer-least-request) done`; lifecycle-state → next-phase routing. Counts: fixtures 60 → **61**; stat surface **1116** (zero delta); fuzzers **42**; BackendKind tail **33**; DECISIONS tail **ADR-0233**. ROADMAP row 34 `in-progress → done` (flat family row — NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (7 candidates remain).

- [ ] **Step 4: Finalize PROGRESS.md** — all 9 tasks complete; the six-gate evidence; the D-question resolutions.

- [ ] **Step 5: Final six-gate re-run + commit (LOCAL-ONLY)**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./... && go test -race -short ./... 2>&1 | tail
go test ./test/differential/ -count=1 2>&1 | tail
git add docs/envoy-go/
git commit -m "phase 34 Task 9: completion bundle — BEHAVIOR_CONTRACT least_request delta; ADR-0232/0233 bodies (ADR-0044 in-place); STATE/ROADMAP row 34 done (fixtures 60->61; stat surface 1116 zero-delta; the first no-fuzzer/no-BackendKind phase)"
```

- [ ] **Step 6: Controller stage-close** — the controller squash-merges the IMPL branch to master, runs the final six-gate on master, and (per `feedback_push_to_origin`, tests green) pushes to origin. Subagents NEVER push (`feedback_subagents_no_push`).

---

## Exit criteria (ADR-0052 atomic landing)

- stat surface **1116** (ZERO delta — the first zero-delta phase, deliberate).
- differential fixtures **60 → 61** (`0059-lb-least-request`; NO boot-reject dir).
- fuzzers **42** (NO new fuzzer — deliberate first); BackendKind tail **33** (NO new BackendKind — deliberate first).
- DECISIONS tail **ADR-0233** (bodies complete; next-free ADR-0234).
- All 61 differential dirs byte-exact (60 prior unchanged through the reshape + `0059` green); `-race -short` green; h2spec 53/53 + proxy-wasm 10/10 unaffected; `go mod tidy -diff` empty.
- ROADMAP row 34 `done` (flat family row); the Load-balancing family OPEN (7 candidates remain).
