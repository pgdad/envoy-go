# Phase 34 SPEC — `least_request` load balancer (`Cluster.LbPolicy LEAST_REQUEST` + `Cluster.LeastRequestLbConfig`): un-weighted P2C on the NEW LB acquire/release seam in `internal/cluster` — the Load-balancing family OPENS; the project's first framework seam OUTSIDE `internal/filter/network`

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This is a SINGLE flat Load-balancing-family row (directly executable; SPEC → PLAN → IMPL), NOT a parent pre-split. The pre-authorized 34.1-seam/34.2-policy split axis stays unconsumed (§3.0 / D-L7a). Phase 34 OPENS the Load-balancing family.

**Goal:** Land `Cluster.LbPolicy LEAST_REQUEST` (config message `Cluster.LeastRequestLbConfig`, `envoy.config.cluster.v3`) — the project's first LB policy beyond ROUND_ROBIN — as un-weighted P2C (sample `choice_count` random hosts WITH replacement, pick the host with the fewest outstanding active counts, strict `<`, first-drawn wins ties — the upstream v1.37.2 semantics pinned from source, AMEND-L6) over a NEW LB acquire/release seam in `internal/cluster` (the reshaped unexported `loadBalancer` interface — the EXPORTED `Cluster` surface stays byte-stable, ZERO consumer churn, AMEND-L7), with cx-as-rq active-count semantics (an open TCP-proxied connection IS one active request — live-pinned, AMEND-L2), in ONE flat phase: ZERO new packages, ZERO new go.mod deps, ZERO new stat names (AMEND-L4), NO new fuzzer, NO new BackendKind — the differential proof is the band-based per-side `DistributionAsserter` skew arm + a cross-side `StatsAsserter` prong (`0059-lb-least-request`).

**Architecture:** The unexported `loadBalancer` interface (`internal/cluster/loadbalancer.go`, currently `Pick() (Endpoint, error)`, sole impl `roundRobin`) reshapes to `Pick() (Endpoint, release func(), error)` — the ADR-0232 seam. The LB owns per-endpoint `atomic.Int64` active counters; `leastRequest.Pick` increments the winner and returns a `sync.Once`-guarded decrement; `roundRobin.Pick` returns a no-op release (provably behavior-neutral). The release is threaded INSIDE `cluster.go` only: `Dial`/`AcquireH1` compose it into the existing `connWithGauge` Close-once hook (ADR-0063); dial/TLS failure releases before the error return; the exported `PickEndpoint()` keeps its signature and releases immediately (direct-pick consumers' load is invisible to least_request — a documented coverage note); `AcquireH1`'s pool-hit branch releases its fresh pick immediately (the pooled conn's dial-time hold persists — cx-as-rq, the conn IS the active unit). `Manager.buildCluster` accepts `LEAST_REQUEST` (the lb_policy reject TEXT changes — the D-L5 blast radius is three sites), parses `choice_count` (hand-rolled `>= 2` reject — the manager calls NO PGV today, AMEND-L1), and departure-rejects the behavior-bearing deferred knobs (`active_request_bias`/`slow_start_config` set under LEAST_REQUEST).

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — `Cluster.LeastRequestLbConfig` is already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` empty — AMEND-L1). Reuses `internal/cluster/` (the 02/05.2/06.1 Manager + Dial/AcquireH1 funnel + the ADR-0063 `connWithGauge`), the differential harness (`DistributionAsserter` + `StatsAsserter` + the `TCPEcho` BackendKind 0 streaming echo), upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/least_request/`) for the algorithm pins. ZERO new packages (the work lands in `internal/cluster`); ZERO `internal/filter/` touches.

**Authored:** 2026-06-11. **Empirical-pin probe date:** 2026-06-11.

---

## 1. Purpose / Mission

Phase 34 lands `least_request`, the **FIRST Load-balancing-family row** — the row that OPENS the Load-balancing family (the first new family since the §9 Network-filters family closed at phase 33). Unlike every §9 row the subject is not a filter: it is a **cluster-scoped endpoint-selection policy**, and its structural piece — the LB acquire/release seam (ADR-0232) — is the project's **first framework seam OUTSIDE `internal/filter/network`** (every prior seam: 07.1 HTTP chain, 26.1/26.2 network chain + terminal, 28.1a/b write/read, 29.3 halt/resume, 32.1 upstream pool — lives under `internal/filter/`). Every future family row ({random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds} — 7 candidates remain after 34) implements the same interface (the ADR-0230 build-at-first-consumer logic, validated at thrift-33's clean reuse).

This SPEC refines the phase-34 BRAINSTORM (`docs/envoy-go/phases/34-load-balancer-least-request/BRAINSTORM.md`, Q0/Q1/Q-seam/Q-split) against the AS-BUILT `internal/cluster` package + the §11 D-L1..D-L7 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (LB probes on a docker bridge network + a `--mode validate` matrix), (2) go-control-plane `/envoy` v1.32.4 bindings, and (3) upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/least_request/` + `.../common/load_balancer_impl.cc`). It anchors the ADR-0232 + ADR-0233 §Context drafts into DECISIONS.md (§Decision/§Consequences bodies land at the IMPL per ADR-0044).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-L1..D-L7 scrape CONFIRMED the SPEC-blocking pins (D-L2 cx-as-rq; D-L1 the three-knob surface) and SHARPENED several BRAINSTORM anticipations. The load-bearing amendments, each carried into the relevant §§ below:

- **AMEND-L1 (D-L1 — the three-knob surface re-pinned + TWO parse findings; CONFIRMS + SHARPENS the BRAINSTORM).** `Cluster_LeastRequestLbConfig` (v1.32.4, `cluster.pb.go:2330`) carries EXACTLY `ChoiceCount *wrapperspb.UInt32Value` (tag 1; doc-comment default 2) + `ActiveRequestBias *corev3.RuntimeDouble` (tag 2) + `SlowStartConfig *Cluster_SlowStartConfig` (tag 3); `EnableFullScan`/`SelectionMethod` ABSENT (zero grep hits). PGV (`cluster.pb.validate.go:3005`): `choice_count >= 2` **only when the wrapper is set** (unset → valid → default 2); error string `value must be greater than or equal to 2`. `go mod tidy -diff` EMPTY + `go build ./...` green in the SPEC worktree — **ZERO new go.mod dep**. TWO sharpenings: (i) **the manager calls NO PGV today** (zero `.Validate()`/`.ValidateAll()` calls across `internal/cluster` + `internal/bootstrap` — every existing reject is hand-rolled) → the `choice_count >= 2` gate must be HAND-ROLLED, mirroring the PGV string (§6.2); (ii) **`least_request_lb_config` is a member of the `lb_config` ONEOF on `Cluster`** (field 37; siblings `ring_hash_lb_config` 23 / `maglev_lb_config` 52 / `original_dst_lb_config` 34 / `round_robin_lb_config` 56); `GetLeastRequestLbConfig()` is nil-safe (returns nil on a mismatched member) → the mismatched-oneof disposition must be pinned (§6.3: silent-ignore, reference PARITY per D-L5 variant 8). See §5 / §11.1.
- **AMEND-L2 (D-L2 SPEC-BLOCKING — cx-as-rq CONFIRMED LIVE; an open TCP-proxied connection IS one active request).** With 8 conns held open through a reference `tcp_proxy → LEAST_REQUEST` listener, per-host `/clusters` showed `rq_active == cx_active` 1:1 on every endpoint (quoted §11.2) for the connection's whole lifetime, dropping together to 0 at close; cluster-level `upstream_rq_active == upstream_cx_active` and `upstream_rq_total == upstream_cx_total` (tcp_proxy charges one rq per cx — the rq and cx families are fully degenerate for TCP). **Consequence:** the per-endpoint counter counts CONNECTIONS (acquire at pick, release at final conn close) — exactly the reference's TCP semantics — and the `0059` skew stimulus is held-open conns through the proxy. For HTTP upstreams the cx-scoped counter is an APPROXIMATION (an idle pooled H1 conn retains its hold; the reference's HTTP rq_active is request-scoped) — a documented behavioral boundary; the rq-scoped HTTP refinement is DEFERRED (§2). See §3.3 / §8.
- **AMEND-L3 (D-L3 — FULL_SCAN DEFERRED with reference-side proof; choice_count == N is NOT deterministic; choice_count > N is legal and quasi-deterministic; the deterministic fixture arm is DROPPED).** THREE legs: (i) even the v1.37.2 REFERENCE's own `Cluster.LeastRequestLbConfig` REJECTS `enable_full_scan: true` as an unknown field (`--mode validate`: `no such field: 'enable_full_scan'` — §11.5 variant 9), and the legacy-config converter (`config.cc TypedLeastRequestLbConfig`) copies ONLY choice_count/bias/slow_start, NEVER `selection_method` → the legacy enum path **cannot reach full-scan at all** in v1.37.2 (§11.3); (ii) sampling is WITH replacement (§11.3) → `choice_count == N` does NOT degenerate to a deterministic least-loaded scan (live: the most-loaded host still took 3/30 picks at cc=3 over 3 endpoints — all-same-host draws at P=1/27); (iii) `choice_count > N` (e.g. 10 over 3) is ACCEPTED silently at boot (no clamp — §11.5 variant 5) and behaves quasi-deterministically (live: the loaded host took 0/30 at cc=10; P(all 10 draws hit one host) ≈ 1.7e-5). **Disposition:** FULL_SCAN DEFERRED; the `0059` deterministic exact-count arm DROPPED; the fixture's band arm uses `choice_count: 10` (legal, reference-parity, robust margins — §8.1). See §11.3 / §11.5.
- **AMEND-L4 (D-L4 — ZERO stat-name delta CONFIRMED; surface STAYS 1116; the `0059` StatsAsserter disposition pinned, with ONE per-side counter).** Full `/stats` name-set diff LEAST_REQUEST-vs-ROUND_ROBIN: EMPTY in both directions (455 names each); `/stats/prometheus` carries no lb-policy-specific metric; the `cluster.<name>.lb_*` family (panic/zone/subset names) is identical-at-0 under both policies. envoy-go mirrors NOTHING new. StatsAsserter disposition for `0059`: `cluster.<name>.upstream_cx_total` CROSS-EQUAL (tcp_proxy is 1:1 downstream-conn→upstream-dial on both sides) + `membership_total == 3` cross-equal + `upstream_cx_active == 0` quiesced post-workload cross-equal; `upstream_rq_total` is **PER-SIDE** (reference = conn count — tcp_proxy charges rq-per-cx, AMEND-L2; subject = 0 — envoy-go's tcpproxy path NEVER calls `IncUpstreamRqTotal`, only the HCM router + the ADR-0230 seam consumers do — a pre-existing documented boundary this fixture now pins). See §7 / §8.1.
- **AMEND-L5 (D-L5 — the accept/reject matrix pinned live + the blast radius is THREE sites; deferred-knob disposition split by behavior-bearing-ness; NO boot-reject fixture dir).** The `--mode validate` matrix (§11.5): LEAST_REQUEST accepted bare and with `choice_count: 2`; `choice_count` 0/1 PGV-rejected (`value must be greater than or equal to 2`); `choice_count: 100` accepted (no clamp); `active_request_bias`/`slow_start_config` parse-ACCEPTED by the reference; a MISMATCHED lb_config (`least_request_lb_config` under ROUND_ROBIN) accepted SILENTLY (zero warnings); RANDOM/RING_HASH validate-accept on the reference (envoy-go's continued rejection of them is a recorded departure). Blast radius of the reject-text change (full-repo grep): **`internal/cluster/manager.go:216`** (the only production string) + **`internal/cluster/manager_test.go` `TestManager_Error_NonRoundRobinLB`** (DOUBLY hit — its trigger value LEAST_REQUEST becomes accepted, so it must retarget to a still-rejected policy AND it pins the substring `"ROUND_ROBIN"`) + **`docs/envoy-go/BEHAVIOR_CONTRACT.md:897`** (the deferral line); plus comment refreshes (`manager.go:42`, `loadbalancer.go:6`). **NO fixture/expectations file pins the text** → no cross-side boot-reject dir is warranted; the `choice_count` PGV arm lands UNIT-LEVEL (fixtures 60 → 61 only). Deferred-knob disposition (the thrift AMEND-T7 fail-fast lineage, split by whether silent acceptance would DIVERGE from the reference at runtime): `active_request_bias`/`slow_start_config` SET under LEAST_REQUEST → **envoy-go-strict DEPARTURE reject** (behavior-bearing: the reference would alter picks; ignoring them silently diverges); a mismatched `lb_config` oneof member → **silent-ignore** (reference PARITY; behavior-INERT — both sides default identically). See §6.
- **AMEND-L6 (D-L6 — the P2C semantics pinned from v1.37.2 source, every leg).** `unweightedHostPickNChoices` (`least_request_lb.cc`): `choice_count` independent draws `random_.random() % hosts.size()` — **WITH replacement, no dedup, no clamp at k ≥ n**; comparison `sampled_active_rq < candidate_active_rq` — **strict `<`, first-drawn wins ties**; the metric is **raw `host->stats().rq_active_.value()`** (the weighted `weight/(active+1)^bias` formula belongs to the EDF path, which engages ONLY when host weights are unequal or slow-start is active — `EdfLoadBalancerBase::refresh` skips EDF creation when `hostWeightsAreEqual && noHostsAreInSlowStart`, so the MVP's equal-weight STATIC clusters take pure P2C and `active_request_bias` NEVER enters the computation; default bias 1.0 confirmed). The RNG is the server-injected crypto generator (per-thread-buffered BoringSSL `RAND_bytes`; upstream's own tests inject a mock) → envoy-go's **injectable-RNG** unit-test posture mirrors upstream. Healthy-host filtering happens UPSTREAM of the pick (priority/healthy-tier/panic selection in the shared base); with no health checking it degenerates to sample-over-all-hosts — envoy-go's boundary (no health-checking — the Upstream-robustness family's territory). FULL_SCAN's tie-break (uniform-random via reservoir sampling) differs from P2C's first-drawn — recorded for the future row; moot here (deferred). See §11.3.
- **AMEND-L7 (D-L7 — the seam is OPTION C: reshape the UNEXPORTED interface only; the exported `Cluster` surface stays byte-stable; ZERO consumer churn; ~155–255 prod LoC → the single flat row HOLDS with margin; ADR-0024 needs NO amendment).** The complete pick-funnel consumer table (§11.4) shows every conn-producing path already funnels Close through the ADR-0063 `connWithGauge` `sync.Once` hook (`cluster.go:333-347`) — the natural release attach-point — while the two NON-conn consumers (httpclient's direct `PickEndpoint` at `httpclient.go:280`, which only rewrites `request.URL.Host` and has NO observable conn lifecycle; thriftproxy's pick-without-dial probe at `filter.go:90`) have NOTHING to release. So: keep `PickEndpoint() (Endpoint, error)` exported AS-IS (it releases immediately inside — those consumers' load is invisible to least_request, a documented coverage note); reshape only the unexported `loadBalancer` interface to `Pick() (Endpoint, release func(), error)`; thread release inside `Dial`/`AcquireH1` (compose into `connWithGauge.dec`; release-before-error on dial/TLS failure; `AcquireH1` pool-hit releases the fresh pick immediately — cx-as-rq, the pooled conn's dial-time hold persists until final close, including `PutIdleH1`-overflow and `closePool` drains). `DialH2`/tcpproxy/router/grpcclient/the ADR-0230 pool dial closures need ZERO changes. ADR-0024 (per-cluster RR counter scope) quoted §11.4: per-endpoint counters owned by the per-cluster LB instance are the SAME per-cluster scope discipline — no amendment; its "no existing code changes when they land" sentence is mildly superseded by the interface reshape (a one-line cross-reference in ADR-0232, not an amendment). LoC ~155–255 / ~10 tasks → BOTH ADR-0045 legs hold with margin; the 34.1/34.2 escape valve STAYS UNCONSUMED. See §3 / §11.4 / §11.6.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0231** (the thrift_proxy filter, ACCEPTED); next-free **ADR-0232**. This SPEC anchors TWO §Context drafts: **ADR-0232** (the LB acquire/release seam — the family's durable structural asset) + **ADR-0233** (the least_request policy). §Decision/§Consequences bodies land at the phase-34 IMPL per ADR-0044. The ADR-0209 escape-valve reserve STANDS-UNCONSUMED. All seven D-L pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **`active_request_bias`** (the weighted P2C variant) — on the reference it engages the EDF path only with unequal host weights (AMEND-L6); the MVP's STATIC clusters are equal-weight. SET under LEAST_REQUEST → DEPARTURE reject (AMEND-L5); a future family sub-phase (with weighted endpoints).
- **`slow_start_config`** — warm-up weighting (also an EDF-path trigger). SET under LEAST_REQUEST → DEPARTURE reject; a future row (shared with ROUND_ROBIN, whose `RoundRobinLbConfig.slow_start_config` is equally unconsumed).
- **FULL_SCAN / `selection_method`** — NOT expressible on the settled v1.32.4 surface NOR on the v1.37.2 reference's own legacy surface (AMEND-L3 — `enable_full_scan` is an unknown-field reject even there; the converter never sets `selection_method`). Deferred with the `Cluster.load_balancing_policy` extension point. The FULL_SCAN tie-break (uniform-random reservoir, ≠ P2C's first-drawn) is recorded at AMEND-L6 for that future row.
- **The `Cluster.load_balancing_policy` extension point** — the typed-extension LB config path (where `least_request.v3.LeastRequest` with `selection_method` lives). A much larger config seam; a future family row.
- **All other policies** — RANDOM, RING_HASH, MAGLEV (+ subset LB, locality-weighted LB, priority load balancing, panic thresholds as modifiers) — stay rejected with the NEW byte-stable text (§6.2); the reference validate-accepts RANDOM/RING_HASH (a recorded departure, AMEND-L5). Each a future family row implementing the ADR-0232 seam.
- **Locality-weighted interplay + panic thresholds + healthy-host filtering** — the reference's pick-time host-set selection (priority tiers / healthy hosts / panic) happens upstream of P2C (AMEND-L6); envoy-go has no health checking (the Upstream-robustness family's territory) → P2C samples over the full endpoint set; the boundary is recorded in BEHAVIOR_CONTRACT at IMPL.
- **Request-scoped HTTP active accounting** — the cx-scoped counter (acquire at pick, release at FINAL conn close) matches the reference exactly for TCP (AMEND-L2) but approximates for HTTP upstreams (an idle pooled H1 conn retains its hold; the reference's HTTP `rq_active` is request-scoped). The rq-scoped refinement (release at `PutIdleH1`, re-acquire at pool-pop) is DEFERRED — more churn + double-release surface for a path no fixture exercises under LEAST_REQUEST yet (D-L7's recommendation). A documented behavioral boundary.
- **Direct-pick consumer load visibility** — `PickEndpoint()`'s immediate-release means httpclient (`httpclient.go:280`) and the thriftproxy no-healthy-host probe contribute NO active load (the probe correctly never should; httpclient's conns live inside a stdlib transport the cluster cannot observe). A documented coverage note (§3.2), NOT a counter bug.
- **lb-specific stats / latency histograms** — zero stat delta CONFIRMED (AMEND-L4); histograms deferred project-wide (ADR-0060).
- **Per-route applicability** — `lb_policy`/`LeastRequestLbConfig` are `Cluster` fields; cluster-scoped at bootstrap; no `typed_per_filter_config` surface, no per-route override (BRAINSTORM §4). The ADR-0125 roster is untouched.

---

## 3. The LB acquire/release seam + the `leastRequest` policy (ADR-0232 + ADR-0233)

### 3.0 Split disposition — D-L7a RESOLVED (single flat phase; the pre-authorized 34.1/34.2 split stays unconsumed)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-34 surface (re-estimated against §11.4/§11.6; OPTION C makes consumer touches ~zero):

| Unit | Anticipated production LoC |
|---|---|
| `loadbalancer.go` reshape: interface → `Pick() (Endpoint, func(), error)`; `roundRobin` no-op release; the `leastRequest` type (P2C loop, per-endpoint `atomic.Int64`, `sync.Once` release, injectable RNG) | ~80–130 |
| `cluster.go` release threading: `PickEndpoint` immediate-release; `Dial`/`AcquireH1` release-on-failure + `connWithGauge` dec composition; pool-hit immediate-release | ~30–50 |
| `manager.go`: accept LEAST_REQUEST + the lb construction switch; `choice_count` parse + hand-rolled `>= 2` reject; the bias/slow_start DEPARTURE rejects; the NEW reject text | ~45–75 |
| Consumer touches (tcpproxy/router/httpclient/grpcclient/dial_h2/pool closures) | **~0** (OPTION C — doc comments only) |
| The `0059` fixture driver + the band asserter + unit/integration tests | test-side LoC, NOT counted |

Net production **~155–255 LoC, ~10 tasks** — BOTH axes FAR under the gate (the BRAINSTORM's ~300–600 anticipation was itself conservative; Option C halves it). **Single flat phase 34 — no pre-split.** The pre-authorized **34.1-seam / 34.2-policy** split axis STAYS UNCONSUMED (the kafka-31/thrift-33 precedent). The PLAN re-checks the gate per ADR-0045.

### 3.1 The seam: the unexported `loadBalancer` interface reshape (ADR-0232; OPTION C per AMEND-L7)

`internal/cluster/loadbalancer.go`:

```go
// loadBalancer is the unexported per-cluster LB interface. Pick returns the
// selected endpoint plus a release func the caller MUST invoke exactly once
// when the picked unit of work completes (for conn-producing paths: when the
// dialed connection finally closes; for non-conn paths: immediately).
// release is always non-nil; implementations guard against double-release.
type loadBalancer interface {
	Pick() (Endpoint, func(), error)
}
```

- **`roundRobin`** adopts the shape and returns a shared no-op release (`func() {}`) — its pick sequence is byte-for-byte unchanged (the ADR-0024 per-cluster `atomic.Uint64` counter untouched; the phase-02 unit-pinned first-pick-is-`endpoints[0]` property and ALL 60 existing fixtures must pass byte-identically — the behavior-neutrality proof).
- **`leastRequest`** (NEW type, same package — D-S34-1 pins the file: anticipated a sibling `leastrequest.go`) owns `active []atomic.Int64` (index-aligned with `endpoints`), `choiceCount int`, and an injectable `rng func() uint64` (production: a `math/rand/v2` PCG seeded from crypto/rand at construction — D-S34-2 finalizes; unit tests inject a deterministic sequence — the upstream mock-RNG posture, AMEND-L6). `Pick()` mirrors upstream EXACTLY (AMEND-L6): `choiceCount` draws `rng() % n` WITH replacement; strict `<` on the raw active count; first-drawn wins ties; no clamp at `choiceCount >= n`; winner's counter `.Add(1)`; returns a `sync.Once`-guarded `.Add(-1)` release.
- The seam is the Load-balancing family's durable asset: random/ring_hash/maglev implement the same interface and ignore the counters at zero cost; the active-load-aware future variants reuse the counters.

### 3.2 The release threading in `cluster.go` (exported surface BYTE-STABLE)

- **`PickEndpoint() (Endpoint, error)`** — signature UNCHANGED. Internally: `ep, release, err := c.lb.Pick(); if err == nil { release() }; return ep, err`. The two direct consumers (httpclient `httpclient.go:280` — rewrites `request.URL.Host` only, conns invisible inside the stdlib transport; thriftproxy `filter.go:90` — a pick-without-dial no-healthy-host probe that MUST NOT hold load) stay source-compatible and leak-free; their load is invisible to least_request (a documented coverage note — §2).
- **`Dial`** — `ep, release, err := c.lb.Pick()`; on dial error or TLS-handshake error: `release()` before the error return (release-on-dial-failure — the BRAINSTORM anticipation confirmed); on success: compose into the existing wrapper — `&connWithGauge{Conn: final, dec: func() { c.upstreamCxActive.Dec(); release() }}`. The existing `sync.Once` in `connWithGauge.Close` (`cluster.go:344-347`) gives double-release protection FOR FREE (the ADR-0063 Inc-then-wrap discipline at rest; caller-side double-Close already safe).
- **`AcquireH1`** — pool-HIT branch: `release()` immediately after the pick (cx-as-rq: the pooled conn's DIAL-TIME hold persists — each live conn holds exactly +1 on its endpoint from dial until FINAL close, whether that close comes from the caller, the `PutIdleH1` overflow drop, or the `closePool` drain; idle pooled conns retain their hold — real upstream load, the documented HTTP approximation, §2). Pool-MISS branch: mirrors `Dial` verbatim (release-on-failure + dec composition).
- **`DialH2`**, tcpproxy, the HTTP router (H1 + H2 paths), grpcclient, and the ADR-0230 upstream-pool dial closures: **ZERO changes** — every error branch already closes the `connWithGauge` wrapper, which now releases (§11.4 consumer table).

### 3.3 cx-as-rq: what the counter counts (AMEND-L2)

The per-endpoint active counter counts **open connections attributable to a pick**: +1 at `Pick` (conn-producing paths), −1 at the conn's final Close (or immediately on dial failure / non-conn picks). For TCP-proxied traffic this is EXACTLY the reference's semantics (live-pinned: per-host `rq_active == cx_active` 1:1; tcp_proxy charges one rq per cx — §11.2). The `0059` skew stimulus follows directly: held-open downstream conns through the proxy elevate the picked endpoints' counters; subsequent picks skew away (live: the most-loaded host took 0/30 new conns at cc=10 — §11.2).

### 3.4 Manager acceptance + the lb construction switch (ADR-0233)

`manager.go buildCluster`: the `lb_policy` guard (line 215) becomes a two-policy accept; construction switches on the policy:

```go
switch c.GetLbPolicy() {
case clusterv3.Cluster_ROUND_ROBIN:
	cl.lb = &roundRobin{endpoints: endpoints}
case clusterv3.Cluster_LEAST_REQUEST:
	cc, err := parseLeastRequestLbConfig(c, name) // §6.2/§6.3 arms
	if err != nil { return nil, err }
	cl.lb = newLeastRequest(endpoints, cc)
default:
	return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST)", name, c.GetLbPolicy())
}
```

The NEW reject text is the ONE deliberate byte-stable-reject change (§6.2; blast radius AMEND-L5). `parseLeastRequestLbConfig`: `GetLeastRequestLbConfig()` nil (absent OR a mismatched oneof member — §6.3) → default `choiceCount = 2`; `choice_count` set `< 2` → the hand-rolled PGV-parity reject; set `>= 2` → use it verbatim (NO clamp at `> len(endpoints)` — reference parity, AMEND-L3); `active_request_bias`/`slow_start_config` set → the DEPARTURE rejects (§6.2).

---

## 4. Framework primitives — 1 NEW framework seam (in `internal/cluster`) + 0 new packages + 0 new go.mod deps

Phase 34's framework delta is the ADR-0232 seam — the FIRST outside `internal/filter/network`, scoped to what least_request needs (per-endpoint active counters + release-on-close; NOT a speculative weighted/locality/priority framework — the ADR-0230 build-at-first-consumer discipline transplanted to a new family). ZERO new packages (the work lands in the existing `internal/cluster`: `loadbalancer.go` + a sibling `leastrequest.go` + `manager.go` + `cluster.go`); ZERO `internal/filter/` touches; ZERO new go.mod deps (AMEND-L1). least_request is not a filter — no builtins registration, no TypeURL factory, no bootstrap blank-import (the `clusterv3` proto is already imported by `internal/cluster`).

---

## 5. Proto-field roster (per §11.1 D-L1)

All from go-control-plane `/envoy` v1.32.4 (`config/cluster/v3/cluster.pb.go` + `cluster.pb.validate.go`), verified in the module cache this session (re-confirming the BRAINSTORM §2.1 verification).

### 5.1 `Cluster.LeastRequestLbConfig` (member of the `lb_config` oneof, field 37)

| Go field | proto field | tag | Go type | PGV | 34 disposition |
|---|---|---|---|---|---|
| `ChoiceCount` | `choice_count` | 1 | `*wrapperspb.UInt32Value` | `>= 2` **when set** (unset valid → default 2) | CONSUMED (the P2C sample size; hand-rolled reject §6.2) |
| `ActiveRequestBias` | `active_request_bias` | 2 | `*corev3.RuntimeDouble` | embedded-recurse only | SET → DEPARTURE reject (§6.2; behavior-bearing — AMEND-L5/L6) |
| `SlowStartConfig` | `slow_start_config` | 3 | `*Cluster_SlowStartConfig` | embedded-recurse only | SET → DEPARTURE reject (§6.2) |

`EnableFullScan`/`SelectionMethod`: ABSENT on this surface (AND on the v1.37.2 reference's own legacy surface — AMEND-L3). The PGV error string (generated code, `cluster.pb.validate.go:3005`): `value must be greater than or equal to 2` (the C++ reference emits the IDENTICAL string — §11.5 variants 3/4).

### 5.2 The `lb_config` oneof (matters for the parse — AMEND-L1)

`Cluster.LbConfig isCluster_LbConfig` carries ONE of: `ring_hash_lb_config` (23) / `maglev_lb_config` (52) / `original_dst_lb_config` (34) / **`least_request_lb_config` (37)** / `round_robin_lb_config` (56). `GetLeastRequestLbConfig()` is nil-safe (nil on absent OR mismatched member). Disposition §6.3.

### 5.3 `Cluster.LbPolicy` enum values at the guard

Accepted: `ROUND_ROBIN` (0 — the proto default; unset ≡ ROUND_ROBIN, the existing posture) + `LEAST_REQUEST` (1). Everything else (`RING_HASH` 2, `RANDOM` 3, `MAGLEV` 5, `CLUSTER_PROVIDED` 6, `LOAD_BALANCING_POLICY_CONFIG` 7) → the §6.2 reject. (The reference validate-accepts RANDOM/RING_HASH — envoy-go's rejection is a recorded departure, AMEND-L5.)

---

## 6. PARSE-REJECT roster (per §11.5 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080: each arm is a named byte-stable constant verified by a table test at IMPL. The reject-text CHANGE at `manager.go:216` is the ONE deliberate contract-surface change this phase (the §9 byte-stable-reject lineage: change it ONCE, with the blast radius enumerated — AMEND-L5: `manager.go:216` + `TestManager_Error_NonRoundRobinLB` + `BEHAVIOR_CONTRACT.md:897` + comment refreshes at `manager.go:42`/`loadbalancer.go:6`; NO fixture pins the text).

### 6.2 Reject arms (all UNIT-TESTED; no cross-side boot-reject dir — AMEND-L5)

- `cluster-lb-policy-unsupported` — the REPLACEMENT text for still-rejected policies: `cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST)`. `TestManager_Error_NonRoundRobinLB` RETARGETS its trigger to `RANDOM` (its current trigger LEAST_REQUEST becomes accepted) and re-pins the new text. An envoy-go-strict DEPARTURE relative to the reference for RANDOM/RING_HASH/MAGLEV (the reference accepts them — recorded in BEHAVIOR_CONTRACT).
- `cluster-least-request-choice-count-too-small` — hand-rolled PGV-PARITY reject (the manager calls no PGV — AMEND-L1): `choice_count` set and `< 2` → `cluster: %q: least_request_lb_config.choice_count: value must be greater than or equal to 2` (mirrors the PGV string both bindings emit — §5.1/§11.5). Arms: 0 rejected, 1 rejected, unset → default 2 accepted, 2 accepted, 100 accepted (no clamp — reference parity).
- `cluster-least-request-active-request-bias-unsupported` / `cluster-least-request-slow-start-unsupported` — **envoy-go-strict DEPARTURE** rejects (the reference parse-accepts both — §11.5 variants 6/7. `slow_start_config` genuinely engages the reference's EDF path; `active_request_bias` is INERT on the equal-weight MVP path per AMEND-L6 but goes live the instant host weights become unequal — a dimension envoy-go silently ignores entirely [it does not parse `load_balancing_weight`]; fail-fast rejects both rather than silently diverging in those regimes — the thrift AMEND-T7 logic). Fire ONLY when the knob is set under `lb_policy: LEAST_REQUEST`. UNIT-TESTED; CANNOT be cross-side boot-rejects (the reference boots fine).
- Exact wording of all three NEW constants finalized at IMPL per ADR-0080's table-test discipline (the strings above are the SPEC's anticipated shapes).

### 6.3 NON-reject dispositions (parity)

- A MISMATCHED `lb_config` oneof member (e.g. `ring_hash_lb_config` under `LEAST_REQUEST`, or `least_request_lb_config` under `ROUND_ROBIN`): **silent-ignore** — reference PARITY (validate-accepts silently, zero warnings — §11.5 variant 8 probed the `least_request_lb_config`-under-ROUND_ROBIN direction; the converse follows by the oneof's mechanical symmetry — nil-safe getters on both sides, each policy reads only its own member) AND behavior-INERT (both sides fall to identical defaults: `GetLeastRequestLbConfig()` returns nil → choice_count 2). This also keeps the EXISTING under-ROUND_ROBIN posture byte-stable (the manager has never read the oneof; all 60 fixtures unaffected).
- `lb_policy: LEAST_REQUEST` with NO lb config: accepted, choice_count defaults to 2 (reference parity — §11.5 variant 1).

---

## 7. Stat surface — ZERO delta (per §11.2/§11.5 D-L4 + AMEND-L4)

- **NO new stat names.** Surface STAYS **1116**. Live-proven: the full `/stats` name-set diff between LEAST_REQUEST and ROUND_ROBIN configs is EMPTY both directions; `/stats/prometheus` carries no lb-policy metric; the reference's `cluster.<name>.lb_*` roster (14 panic/zone/subset names, quoted §11.2) exists identically-at-0 under BOTH policies and is NOT mirrored (envoy-go has never mirrored it — unchanged posture). The per-endpoint active counters are LB-internal state, NOT registry stats (no `/stats` exposure — the reference's per-host counters surface via `/clusters`, which envoy-go's ADR-0087 handler already stubs per-endpoint stats at 0; unchanged).
- **The first anticipated-zero-delta phase since the stat surface began moving** — confirmed empirically, recorded deliberately.
- The `0059` StatsAsserter set (AMEND-L4): CROSS-EQUAL `cluster.<name>.upstream_cx_total` (= total driver conns; tcp_proxy is 1:1 both sides) + `membership_total` (= 3) + `upstream_cx_active` (= 0 post-workload, quiesced); PER-SIDE `upstream_rq_total` (reference = conn count [rq-per-cx, AMEND-L2]; subject = 0 [the tcpproxy path never calls `IncUpstreamRqTotal` — a pre-existing boundary this fixture pins; the §9 per-side-stats lineage]).

---

## 8. Differential fixture taxonomy (+1)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (no boot-reject dir — AMEND-L5). Per `reference_differential_asserter_dispatch`: the stats prong uses `StatsAsserter` (cross-side path); the distribution prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the 0001/0003 precedent). Every assertion proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1` — DOUBLY load-bearing here: a band wide enough to never fail is a dead assertion). Numbering continues from `0058`; re-pinned at IMPL Task 1.

### 8.1 `0059-lb-least-request` (cross-side; the band-based skew arm + the stats prong)

Chain `[tcp_proxy]` on BOTH sides (the 0001 shape: reference STRICT_DNS/`host.docker.internal`, subject STATIC/127.0.0.1) over ONE 3-endpoint cluster with `lb_policy: LEAST_REQUEST` + `least_request_lb_config: { choice_count: 10 }` (legal > endpoint-count config — reference-accepted with no clamp, AMEND-L3 — chosen because it makes the skew quasi-deterministic and the band robust; cc=2's skew is real but too noisy to band tightly: live 16/30 vs 10 uniform). Backends: the plain **`TCPEcho` BackendKind 0** (STREAMING echo — `acceptEchoCounting` echoes each read chunk immediately, so a held conn CAN confirm end-to-end establishment with a write+read before holding; verified in the runner source). **NO new BackendKind** (tail STAYS 33).

**The workload (identical per side, sequential):**
1. **Hold phase:** open K=4 connections; on each, write one byte and read the echo (the establishment witness — the upstream dial completed and the pick's active count is held), then KEEP the socket open. The picks self-spread ≈ {2,1,1} (least-loaded filling — live-observed).
2. **Burst phase:** S=60 sequential short round-trips (`helpers.TCPRoundTrip` — write, half-close, read echo, close), allowing close-accounting between picks.
3. **Drain:** close the 4 held conns.

**The band arm (per-side, via `AssertDistribution` on the per-backend accept counts; sorted c1 ≤ c2 ≤ c3 of the 64 total accepts):**
- `c1 + c2 + c3 == 64` (conservation);
- `c1 <= 12` (STARVATION: the most-held backend gets ≈ its 2 held conns + ~0 burst landings — live cc=10 analogue {18, 2, 14}; under ROUND_ROBIN c1 = 21 → BITES);
- `c2 >= 16` (CONCENTRATION SHAPE: the two least-loaded backends split the burst ≈ 31 ± 4 each — catches an INVERTED comparison, where c2 would be ≈ 1).

Asserted **PER SIDE** (both sides must land in the band; NEVER cross-side-exact — independent RNG streams, the BRAINSTORM's band-semantics decision). This is the **FIRST band-based `AssertDistribution`** (every prior use — 0001/0002/0003/0004/0045 — asserts exact counts; the interface supports the band unchanged, and 0003's subject-only assertion is the asymmetric-per-side precedent — §11.4). The exact constants (K/S/c1/c2 bounds) are IMPL-tunable within the pinned principle {conservation + starvation + concentration-shape}; margins justified by the live cc=10 probe (the loaded host took 0/30; P(one burst conn lands on the loaded host) ≈ tie-window transients only) — D-S34-3 records the tuning protocol.
- **Deliberate-break liveness (`-count=1`):** (i) invert the P2C comparison (pick MOST loaded) → c2 ≈ 1 → the concentration leg FAILS deterministically; (ii) make `leastRequest`'s release a NO-OP (counters never decrement → cumulative-pick leveling ≈ uniform {21,21,22}) → c1 ≈ 21 → the starvation leg FAILS deterministically (the never-incremented-counter variant is NOT the canonical break — it degenerates to random picks ≈ multinomial(64, 1/3), which leaves c1 > 12 only ~97% of the time per run; D-S34-3); (iii) drop a `StatsAsserter` Inc → the stats prong FAILS. Recorded in driver comments + README per the `0030` lesson.

**The stats prong (cross-side `StatsAsserter`, post-drain):** §7's set — cross-equal `upstream_cx_total == 64` + `membership_total == 3` + `upstream_cx_active == 0`; per-side `upstream_rq_total` (ref 64 / subj 0 — AMEND-L4).

### 8.2 NO boot-reject dir (AMEND-L5)

The `choice_count < 2` PGV arm + the lb-policy/departure rejects land UNIT-LEVEL in `manager_test.go` (§6.2): no NEW config field is REQUIRED (LEAST_REQUEST with no lb config is valid on both sides — there is no missing-required-field arm to pin cross-side, contrast `0056`/`0058`), no fixture pins the old reject text, and the departure arms CANNOT be cross-side (the reference accepts them). Fixture count 60 → **61** (the BRAINSTORM's 62-only-if branch is NOT taken).

### 8.3 NO new BackendKind (a family-level first) + NO new fuzzer (a project-level first)

BackendKind tail STAYS **33** (`0059` reuses `TCPEcho` 0 — an LB phase exercises WHERE connections land, not what the backend speaks). Fuzzers STAY **42** — phase 34 decodes no wire bytes (config parse is proto-level, already fuzz-covered at the bootstrap surface); the FIRST no-fuzzer phase since the fuzzing regime began — DELIBERATE (the BRAINSTORM §2.5 dead-assertion logic: a manufactured fuzzer over no decoder is vacuous), recorded here and at the BEHAVIOR_CONTRACT bundle. No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate. The six-gate's REAL guard for the seam reshape is the full 61-dir differential re-verify: all 60 existing dirs must stay byte-exact through the `roundRobin` adoption (behavior-neutrality proven by the existing suite).

---

## 9. Behavior-contract delta (the 34 bundle; ADR-0052 atomic landing)

At IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW least_request subsection beside the TCP-proxy/RR LB boundary text: the LEAST_REQUEST acceptance (`choice_count` default 2 / `>= 2` when set / no clamp); the P2C semantics (with-replacement, strict `<`, first-drawn ties — the upstream v1.37.2 mirror); cx-as-rq active-count semantics (TCP exact; the HTTP idle-pooled-conn approximation boundary); the per-side RNG non-equivalence (band-proven, never cross-side-exact).
- The line-897 deferral list updates (LEAST_REQUEST retired from "LB policies other than ROUND_ROBIN"; the remaining 7 candidates stay).
- Departure/coverage records: the NEW lb-policy reject text (RANDOM/RING_HASH/MAGLEV rejected where the reference accepts); the bias/slow_start DEPARTURE rejects; the mismatched-oneof silent-ignore (parity); the direct-pick consumers' load invisibility (httpclient + the thriftproxy probe); `upstream_rq_total` per-side on the TCP path (subject's tcpproxy never increments it); NO new fuzzer/BackendKind (deliberate firsts); stat surface UNCHANGED at 1116.

---

## 10. Per-task structure (~10 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **60** (tail `0058`) + fuzzers **42** + stat surface **1116** + BackendKind tail **33** + DECISIONS tail (ADR-0232/0233 §Context from this SPEC) via the canonical recipes; re-pin the as-built anchors (`loadbalancer.go` interface, `manager.go:215` guard, `cluster.go` `Dial`/`AcquireH1`/`connWithGauge`, `httpclient.go:280`, thriftproxy `filter.go:90`, the 0001 driver + `acceptEchoCounting`) against the IMPL-session tip; PROGRESS.md created | §11 / §3 |
| 2 | The `loadBalancer` interface reshape + `roundRobin` no-op release (TDD: the first-pick-is-`endpoints[0]` property + the pick sequence byte-unchanged) | §3.1 |
| 3 | `cluster.go` release threading: `PickEndpoint` immediate-release; `Dial` release-on-dial/TLS-failure + the `connWithGauge` dec composition; `AcquireH1` pool-hit immediate-release + pool-miss mirror (TDD: counter balance through dial failure / double-Close / pool reuse / `closePool` drain) | §3.2 |
| 4 | The `leastRequest` type: P2C with the injectable RNG (TDD with a deterministic RNG: with-replacement sampling, strict `<`, first-drawn tie-win, `choiceCount >= n` no-clamp, counter inc/dec balance, `sync.Once` double-release guard) | §3.1 / AMEND-L6 |
| 5 | Manager acceptance + `parseLeastRequestLbConfig`: the construction switch + the NEW reject text + the `choice_count` arms (0/1 reject, unset→2, 2/100 accept) + the bias/slow_start DEPARTURE rejects + the mismatched-oneof silent-ignore + `TestManager_Error_NonRoundRobinLB` retarget (TDD: the full §6 matrix, byte-stable table test) | §3.4 / §6 |
| 6 | An in-process skew integration test (deterministic RNG; held picks elevate counters; subsequent picks avoid the loaded endpoint) + boot smoke with a LEAST_REQUEST bootstrap | §3.3 |
| 7 | The `0059-lb-least-request` fixture: driver (hold-4 + burst-60 + drain workload), the band `AssertDistribution` (conservation/starvation/concentration), the `StatsAsserter` prong (cross-equal cx/membership/quiesced + per-side rq) | §8.1 |
| 8 | Band-constant tuning + deliberate-break liveness (`-count=1`): the inverted-comparison break (c2 leg), the no-op-release break (c1 leg — canonical, deterministic), the stats-prong break; repeat-run flake check (D-S34-3) | §8.1 |
| 9 | Full differential re-verify (the 60 prior dirs byte-exact through the seam reshape + `0059` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected | §8.3 |
| 10 | Completion bundle: BEHAVIOR_CONTRACT 34 bundle (§9) + the ADR-0232/0233 §Decision/§Consequences bodies (ADR-0044 in-place; tail → ADR-0233) + STATE/ROADMAP row 34 `in-progress → done` (flat family row — NO parent rollup) + the six-gate evidence | §9 / §13 |

The PLAN re-checks the ADR-0045 gate; if it trips, consume the pre-authorized 34.1/34.2 split (§3.0).

---

## 11. SPEC-time empirical-pin block (D-L1..D-L7 — executed IN-SESSION 2026-06-11)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-11.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image**: a `tcp_proxy → STRICT_DNS LEAST_REQUEST` listener on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with THREE streaming-echo backends; held-open-conn probes + short-conn distribution bursts + `/clusters` per-host scrapes + full `/stats`//`/stats/prometheus` name-set diffs (`downstream_cx_rx_bytes_total > 0` verified after EVERY traffic-bearing probe); a 10-variant `--mode validate` matrix.
2. **go-control-plane `/envoy` v1.32.4 bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/config/cluster/v3/` (`cluster.pb.go` + `cluster.pb.validate.go`); `go mod tidy -diff` + `go build ./...` in the SPEC worktree.
3. **Upstream Envoy v1.37.2 source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/load_balancing_policies/least_request/least_request_lb.{h,cc}` + `config.{h,cc}` + `source/extensions/load_balancing_policies/common/load_balancer_impl.{h,cc}` + `api/envoy/config/cluster/v3/cluster.proto` + the extension proto + `source/common/common/random_generator.{h,cc}`.
4. **envoy-go codebase** at master tip `a463fe8`: `internal/cluster/{loadbalancer,manager,cluster,dial_h2}.go`, every pick-funnel consumer, `test/differential/{runner_test.go,fixture/fixture.go}`, `test/fixtures/0001-tcp-proxy-rr/`, full-repo reject-text greps.

### Summary disposition table (7 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-L1 (SPEC-BLOCKING) — proto/PGV re-pin + tidy | **CONFIRMED + 2 SHARPENINGS** (three knobs; PGV ≥2-when-set; NO PGV call in the manager → hand-rolled; the `lb_config` ONEOF) | L1 |
| §11.2 | D-L2 (SPEC-BLOCKING) — "active request" for TCP | **CONFIRMED LIVE** (cx-as-rq; `rq_active == cx_active` per host 1:1; skew live-observed) | L2 |
| §11.3 | D-L3 — FULL_SCAN + tie-break | **DEFERRED with reference-side proof** (unknown-field reject on the reference itself; with-replacement → cc==N not deterministic; cc>N legal + quasi-deterministic) | L3 |
| §11.3 | D-L6 — P2C sampling semantics | RESOLVES (with replacement; strict `<`; first-drawn ties; raw rq_active; equal-weights short-circuit; injectable RNG; healthy-set boundary) | L6 |
| §11.4 + §11.6 | D-L7 — the seam surface + consumers + ADR-0024 + the ADR-0045 envelope re-check (D-L7a) | **OPTION C** (unexported reshape; exported byte-stable; zero consumer churn; ADR-0024 unamended; ~155–255 LoC → single flat row HOLDS) | L7 |
| §11.5 | D-L5 — accept/reject matrix + blast radius | RESOLVES (the 10-variant validate table; 3-site blast radius; NO fixture pins; bias/slow_start DEPARTURE; mismatched-oneof parity-ignore; no boot-reject dir) | L5 |
| §11.7 | D-L4 — stat-surface delta | **ZERO delta CONFIRMED** (empty name-set diff; no prom delta; the StatsAsserter cross-vs-per-side set pinned) | L4 |

### 11.1 D-L1 (SPEC-BLOCKING) — the proto surface: CONFIRMED + two sharpenings

`Cluster_LeastRequestLbConfig` (cluster.pb.go:2330): exactly `ChoiceCount`/`ActiveRequestBias`/`SlowStartConfig` (tags 1/2/3; full Go declarations quoted in the probe record); `EnableFullScan`/`SelectionMethod` zero grep hits. PGV (cluster.pb.validate.go:3005): `if wrapper := m.GetChoiceCount(); wrapper != nil { if wrapper.GetValue() < 2 { ... reason: "value must be greater than or equal to 2" } }` — fires ONLY when set. The doc comment pins default 2 ("Defaults to 2 so that we perform two-choice selection if the field is not set"). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK (the worktree untouched). Sharpening (i): `grep '\.Validate()\|\.ValidateAll()'` over `internal/cluster/` + `internal/bootstrap/` → ZERO hits — every existing manager reject is hand-rolled → the `choice_count` gate must be hand-rolled too (§6.2). Sharpening (ii): `least_request_lb_config` is `lb_config` ONEOF member 37 (siblings 23/52/34/56); `GetLeastRequestLbConfig()` nil-safe → the mismatched-member disposition is a REAL parse decision (§6.3).

### 11.2 D-L2 (SPEC-BLOCKING) — cx-as-rq: CONFIRMED LIVE (+ the skew + degeneracy probes)

Bridge network, 3 socat streaming-echo backends, `tcp_proxy → lbcluster {LEAST_REQUEST, choice_count: 2}`; `tcp.tcp.downstream_cx_rx_bytes_total > 0` verified after every probe. **With 8 conns held open**, `/clusters` per-host (exact lines): `cx_active::2|3|3`, `rq_active::2|3|3`, `cx_total::2|3|3`, `rq_total::2|3|3` — `rq_active == cx_active` 1:1 per host; `/stats`: `upstream_cx_active: 8 == upstream_rq_active: 8`, `upstream_cx_total: 8 == upstream_rq_total: 8`. After closing all: every active stat → 0, totals frozen. **The rq and cx families are fully degenerate for TCP-proxied traffic — the per-endpoint counter counts CONNECTIONS.** **Skew (cc=2):** 8 held (spread 2/3/3) + 30 sequential short conns → per-host deltas +16/+4/+10 — the least-loaded host took 16/30 (~53% vs 33% uniform; consistent with P2C theory 1−(2/3)² ≈ 0.556). **Degeneracy (cc=3 == N):** with 4 held (1/1/2), 30 short conns → deltas +16/+11/+3 — the MOST-loaded host still took 3 (all-same-host draws, P=1/27 ≈ 1.1 expected/30) → **cc == N is NOT a deterministic scan** (with-replacement). **cc=10 > N=3:** boots clean (no warning), deltas +17/+0/+13 — the loaded host took ZERO; the two tied least-loaded split 17/13. The full probe YAML is Appendix A.

### 11.3 D-L3 + D-L6 — the v1.37.2 algorithm, pinned from source

`LeastRequestLoadBalancer::unweightedHostPickNChoices` (`least_request_lb.cc`): `for (choice_idx < choice_count_) { const int rand_idx = random_.random() % hosts_to_use.size(); ... }` — WITH replacement, no dedup, no clamp (`choice_count_` fixed at construction: `PROTOBUF_GET_WRAPPED_OR_DEFAULT(..., choice_count, 2)`); the comparison `if (sampled_active_rq < candidate_active_rq) candidate_host = sampled_host;` over `host->stats().rq_active_.value()` — strict `<`, first-drawn keeps ties, raw active count. **Equal-weights short-circuit** (`EdfLoadBalancerBase::refresh`, `load_balancer_impl.cc:988`): `if (hostWeightsAreEqual(hosts) && noHostsAreInSlowStart()) { /* Skip edf creation */ }` → `chooseHostOnce` falls to `unweightedHostPick` — pure P2C; the weighted formula (`hostWeight` = `weight/(rq_active+1)^bias`, bias default 1.0 via `Runtime::Double`, negative/NaN clamped to 1.0) belongs ONLY to the EDF path (unequal weights / slow-start) — `active_request_bias` NEVER enters the equal-weight computation. **FULL_SCAN:** dispatched on `selection_method_` (extension-proto enum, default `N_CHOICES`); the legacy converter (`config.cc TypedLeastRequestLbConfig`) maps ONLY `choice_count`/`active_request_bias`/`slow_start_config` — `selection_method` never set → **the legacy path cannot reach full-scan**; the v1.37.2 `cluster.proto` `LeastRequestLbConfig` itself has exactly the three fields (`choice_count = 1 [(validate.rules).uint32 = {gte: 2}]`); the extension proto's `enable_full_scan = 5` is `deprecated`/`[#not-implemented-hide:]` dead config. Full-scan tie-break (for the future row): uniform-random among ties via reservoir sampling (`random_tied_host_index == 0`), DIFFERENT from P2C's first-drawn. **RNG:** the server-injected `Random::RandomGenerator` (per-thread-buffered BoringSSL `RAND_bytes`); upstream tests inject a mock → envoy-go's injectable-RNG posture mirrors upstream. **Host-set:** priority/healthy/panic selection happens in the shared base BEFORE the pick; with no health checking it degenerates to all-hosts (envoy-go's boundary — no health checking exists; recorded at BEHAVIOR_CONTRACT).

### 11.4 D-L7 — the seam: OPTION C; the consumer table; ADR-0024; the DistributionAsserter precedent

**The complete pick-funnel table** (every consumer, greps over `internal/`+`cmd/`): direct `PickEndpoint` — `Dial` (cluster.go:202), `AcquireH1` (cluster.go:246), httpclient `ClusterDispatch` (httpclient.go:280 — rewrites `request.URL.Host` only; conns pooled/closed invisibly inside a stdlib `http.Transport`; NO observable release point), thriftproxy `resolveCluster` (filter.go:90 — `if _, err := cl.PickEndpoint(); err != nil { ... resolveNoHealthyUpstream }`, pick-without-dial, Endpoint discarded). `Dial` consumers — `DialH2` (dial_h2.go:43, every error branch closes the wrapper), tcpproxy (filter.go:127, `defer upstream.Close()`), redisproxy/thriftproxy ADR-0230 dial closures (lazily dialed, `up.Close()` in Handle's defer), grpcclient (grpcclient.go:133, transport-owned), router H1 bw-path (router.go:662, `defer upstream.Close()`). `AcquireH1`/`DialH2` consumers — router.go:509 (`PutIdleH1` if reusable else Close), router_h2.go:62/:233 (`defer cc.Close()`). **The H1-pool subtlety:** `AcquireH1` ALWAYS Picks first, THEN pool-pops by the picked `ep.Addr()` — a pool hit is NOT pick-free; the pooled conn carries its DIAL-TIME endpoint + `connWithGauge`. Under cx-as-rq the hit-path's fresh pick releases IMMEDIATELY (the conn's original dial-time hold persists to final close — incl. `PutIdleH1`-overflow close and `closePool` drain); leak-free by construction. **OPTION C** chosen (vs A exported-release-handle / B PickAcquire-Release): A forces a ceremonial release onto the two consumers with nothing to release + churns the exported API; B spreads a stateful two-call protocol across nine consumer packages; C keeps the exported surface byte-stable, puts the release exactly where `connWithGauge` already proves the closure-plus-`sync.Once` idiom, and costs only the documented load-invisibility note for direct-pick consumers. **ADR-0024** (DECISIONS.md:591): "Each `*Cluster` owns its own `atomic.Uint64` counter. The `roundRobin` LB consults only that cluster's counter." + its Consequences pre-authorize this phase ("Future LB policies ... will be added alongside `roundRobin` as new types implementing the unexported `loadBalancer` interface; each owns its own state per-cluster. No existing code changes when they land."). NO amendment — per-endpoint counters on the per-cluster LB instance are the same scope discipline; the "no existing code changes" sentence is mildly superseded by the interface reshape → a one-line cross-reference in ADR-0232. **The DistributionAsserter precedent:** runner hook at runner_test.go:1102 (`AssertDistribution(refCounts, subjCounts)` fed per-backend accept counts); 0001 asserts EXACT `[3,3,3]` BOTH sides; 0003 asserts EXACT subject-only (`_ = refCounts` — "ref and subj RR may start from different endpoints"); NO band/tolerance use exists anywhere → `0059` is the FIRST band-based use (interface unchanged; 0003 legitimizes per-side asymmetry). **LoC:** ~155–255 prod (§3.0 table) — the single flat row HOLDS with margin.

### 11.5 D-L5 — the validate matrix + the blast radius

The 10-variant `--mode validate` table (contrib-v1.37.2; base = tcp_proxy + STATIC cluster):

| Variant | Verdict | Reference output (decisive fragment) |
|---|---|---|
| LEAST_REQUEST, no lb config | ACCEPT | `configuration ... OK` |
| + `choice_count: 2` | ACCEPT | `OK` |
| `choice_count: 0` | REJECT | `...LeastRequestLbConfigValidationError.ChoiceCount: value must be greater than or equal to 2)` |
| `choice_count: 1` | REJECT | identical chain |
| `choice_count: 100` | ACCEPT | `OK` (no clamp/warn) |
| `active_request_bias: {default_value: 1.5, runtime_key: "arb"}` | ACCEPT | `OK` |
| `slow_start_config: {slow_start_window: 10s}` | ACCEPT | `OK` |
| ROUND_ROBIN + `least_request_lb_config` (MISMATCH) | ACCEPT silently | `OK`; zero `[warning]`/`[error]` lines |
| `enable_full_scan: true` | REJECT (unknown field) | `no such field: 'enable_full_scan'` — ABSENT on the reference's own legacy surface too |
| RANDOM / RING_HASH (bare) | ACCEPT both | `OK` (envoy-go's continued rejection = recorded departure) |

**Blast radius** (full-repo greps): production string — ONLY `manager.go:216`; unit pinners — ONLY `TestManager_Error_NonRoundRobinLB` (manager_test.go:245-255; trigger `LEAST_REQUEST` → must retarget to RANDOM; asserts substring `"ROUND_ROBIN"`); fixtures — ZERO hits (`grep -rln "only ROUND_ROBIN\|NonRoundRobin" test/` empty; all fixture `lb_policy` lines are happy-path ROUND_ROBIN); docs — `BEHAVIOR_CONTRACT.md:897` (the deferral line) + comment sites `manager.go:42`/`loadbalancer.go:6`; ~25 test scaffolds set `LbPolicy: ROUND_ROBIN` (unaffected). NO cross-side boot-reject dir warranted (§8.2).

### 11.6 D-L7a — the ADR-0045 envelope re-check

~155–255 production LoC / ~10 tasks (§3.0) — BOTH legs FAR under the gate (`> ~25 tasks OR > ~1500 LoC`). SINGLE FLAT ROW 34 holds; the pre-authorized 34.1-seam/34.2-policy escape valve STAYS UNCONSUMED. The PLAN re-checks.

### 11.7 D-L4 — zero stat delta: CONFIRMED

Full `/stats` after traffic under LEAST_REQUEST vs ROUND_ROBIN: 455 names each; sorted name-set `comm` diff EMPTY both directions. `/stats/prometheus` under LEAST_REQUEST (686 `envoy_` lines): `grep -iE 'least|choice|p2c'` → no match. The `cluster.<name>.lb_*` family identical-at-0 under both policies: `lb_healthy_panic`, `lb_local_cluster_not_ok`, `lb_recalculate_zone_structures`, `lb_subsets_{active,created,fallback,fallback_panic,removed,selected}`, `lb_zone_{cluster_too_small,no_capacity_left,routing_all_directly,routing_cross_zone,routing_sampled}` — pre-existing cluster-scope names envoy-go has never mirrored (unchanged posture). Surface STAYS **1116**. The StatsAsserter cross-vs-per-side set: §7.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S34-1** — the exact file split inside `internal/cluster` (anticipated: the reshaped interface + `roundRobin` stay in `loadbalancer.go`; the `leastRequest` type in a NEW sibling `leastrequest.go`; `parseLeastRequestLbConfig` in `manager.go`).
- **D-S34-2** — the production RNG source + seeding (anticipated: `math/rand/v2` PCG seeded from `crypto/rand` once per `leastRequest` construction; the interface is `rng func() uint64` for deterministic unit injection — the upstream mock posture, AMEND-L6).
- **D-S34-3** — the final band constants (K=4/S=60/c1≤12/c2≥16 anticipated) + the tuning protocol: N≥20 local repeat runs of `0059` flake-free per side BEFORE landing, plus the three deliberate-break proofs with `-count=1` (§8.1) — a band that cannot fail is a dead assertion (`reference_differential_break_protocol_count1` generalized to BAND assertions).
- **D-S34-4** — whether the held-conn establishment witness (write+read-echo per held conn before holding) suffices to serialize pick-visibility on the REFERENCE side too (the subject increments at Pick synchronously; the reference increments on upstream-conn assignment — the echo round-trip proves the upstream dial completed; anticipated sufficient, confirmed at IMPL by the band margins).
- **D-S34-5** — the release-composition shape in `connWithGauge` (anticipated: compose into the existing `dec func()` closure — `dec: func() { gauge.Dec(); release() }` — keeping the struct unchanged; alternative: a second field; PLAN picks).
- ADR-0045 split-gate FINAL re-check at PLAN.

---

## 13. ADR continuity

This SPEC anchors **TWO §Context drafts** into DECISIONS.md: **ADR-0232** (the LB acquire/release seam — the project's first framework seam outside `internal/filter/network`; the family's durable asset; Option C; ADR-0024 cross-referenced not amended; the ADR-0063 `connWithGauge` composition) + **ADR-0233** (the least_request policy — un-weighted P2C mirroring v1.37.2 source semantics; the accept/reject matrix; the band-based differential proof shape; the no-fuzzer/no-BackendKind family-level firsts). Tail ADR-0231 → **ADR-0233** at this SPEC commit (next-free → ADR-0234). §Decision/§Consequences bodies land at the phase-34 IMPL per ADR-0044. The ADR-0209 escape-valve reserve STANDS-UNCONSUMED.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (counts UNCHANGED at the SPEC except the DECISIONS tail; the rest advance at the IMPL):

- stat surface **1116** (→ **1116** at the IMPL — ZERO delta, AMEND-L4; the first zero-delta phase, deliberate).
- differential fixtures **60** (→ **61** at the IMPL: `0059-lb-least-request`; NO boot-reject dir — AMEND-L5).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0231 → ADR-0233** at this SPEC commit (the ADR-0232 + ADR-0233 §Context drafts; next-free **ADR-0234**); bodies at the IMPL per ADR-0044.
- ROADMAP row 34 STAYS `in-progress` (it flips `→ done` at the phase-34 IMPL six-gate — a flat family row, NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (7 candidates remain after 34).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-34 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; re-check the ADR-0045 gate).

---

## Appendix A — the live LB probe config + the observed numbers (the `0059` design basis)

The reference YAML (bridge network `dl2probe-net`; 3 streaming-echo backends; the `0059` fixture transposes this to the 0001 harness shape — reference STRICT_DNS/`host.docker.internal`, subject STATIC/127.0.0.1):

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: tcp_listener
      address:
        socket_address: { address: 0.0.0.0, port_value: 10000 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp
                cluster: lbcluster
  clusters:
    - name: lbcluster
      type: STRICT_DNS
      lb_policy: LEAST_REQUEST
      least_request_lb_config:
        choice_count: 2          # variants: 3, 10; RR control: lb_policy ROUND_ROBIN, no lb config
      load_assignment:
        cluster_name: lbcluster
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: dl2probe-be1, port_value: 9000 } } }
              - endpoint: { address: { socket_address: { address: dl2probe-be2, port_value: 9000 } } }
              - endpoint: { address: { socket_address: { address: dl2probe-be3, port_value: 9000 } } }
```

**The observed distributions** (per-host deltas over 30 sequential short conns, after the stated held-open load):

| Config | Held (per-host active) | Burst deltas | Reading |
|---|---|---|---|
| cc=2 | 8 held (2/3/3) | +16/+4/+10 | least-loaded took 53% (vs 33% uniform) — real but NOISY (the dropped exact-arm rationale) |
| cc=3 (== N) | 4 held (1/1/2) | +16/+11/+3 | the MOST-loaded still took 3 — with-replacement, NOT deterministic |
| cc=10 (> N) | 4 held (1/2/1) | +17/+0/+13 | loaded host took ZERO; tied pair split 17/13 — the `0059` band basis |
| RR control | — | ~uniform | the deliberate-break expectation (c1 ≈ N/3) |

cx-as-rq held-conn evidence (8 held): per-host `/clusters` `rq_active::2|3|3 == cx_active::2|3|3`; cluster `upstream_rq_active: 8 == upstream_cx_active: 8`; all → 0 at close; totals frozen (`upstream_rq_total: 8 == upstream_cx_total: 8` — rq-per-cx).
