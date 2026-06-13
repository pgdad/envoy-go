# Phase 37 SPEC — `maglev` load balancer (`Cluster.LbPolicy MAGLEV`): a Maglev consistent-hashing policy that REUSES the ADR-0235 LB hash-key seam + BOTH ADR-0237 producers UNCHANGED, swapping the Ketama RING for a fixed-size lookup table — the FOURTH Load-balancing-family row; the family STAYS OPEN

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This is a SINGLE flat Load-balancing-family row (directly executable; SPEC → PLAN → IMPL), NOT a parent pre-split, and NO escape valve (there is no seam build to split off — §3.0). Phase 37 keeps the Load-balancing family OPEN; 4 candidates remain after 37 (subset LB, locality-weighted LB, priority load balancing, panic thresholds).

**Goal:** Land `Cluster.LbPolicy MAGLEV` (`envoy.config.cluster.v3`, enum value 5) — the project's **fifth LB policy** (after the phase-02 `roundRobin`, phase-34 `leastRequest`, phase-35 `randomLB`, phase-36 `ringHashLB`) and its **second consistent-hashing policy** — as a fixed-size Maglev lookup-table LB (`table[hashKey % M]`) that REUSES the ADR-0235 LB hash-key seam UNCHANGED — implementing the existing unexported `loadBalancer` interface (`Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)`) and returning the shared `noopRelease` (the seam's FIRST hash-policy REUSE, validating ADR-0235's "the durable asset maglev reuses unchanged" §Consequences claim, AMEND-M6). The hash key is supplied by the LANDED producers (tcp `source_ip` + HTTP route `hash_policy`, ADR-0237) UNCHANGED — phase 37 changes WHICH structure the key indexes (a Maglev table, not a Ketama ring), NOT how the key is produced (AMEND-M3). In ONE flat phase: ZERO new packages (a new `internal/cluster/maglev.go` sibling), ZERO new go.mod deps (the enum + config are in the pinned core `/envoy v1.32.4` module — AMEND-M1), ZERO new hash code (the table build reuses the existing `xxHash64`/`xxHash64Seed` in `hash.go` — AMEND-M2), ZERO seam churn, ZERO producer churn. The config-parse arm is a SINGLE bounded field (`table_size`) + a PRIMALITY gate + the PGV cap (AMEND-M1/M5). The differential proof is the header-key NAT-transparent cross-side AFFINITY + SPREAD arm (`0063-lb-maglev`, the `0062` convention reused verbatim).

**Architecture:** A new `maglevLB` type (`internal/cluster/maglev.go`, same package) mirrors `ringHashLB` structurally — built ONCE at construction, immutable thereafter (so `Pick` is concurrency-safe with no lock), with an injectable `rng func() uint64`. It holds `endpoints []Endpoint`, the fixed-size `table []int` (each slot an index into `endpoints`), `tableSize uint64`, and the `minPerHost`/`maxPerHost` gauge values. The build (AMEND-M2, pinned byte-for-byte from upstream v1.37.2): SORT endpoints by `Addr()` string ascending (the Envoy pre-build `std::sort`), derive per host `offset = xxHash64(addr) % M` + `skip = (xxHash64Seed(addr, 1) % (M-1)) + 1`, then run the canonical Maglev populate loop `(offset + skip·next) % M` until all `M` slots fill (terminates because `M` prime → each `skip` coprime to `M` → a full `M`-cycle). `Pick(hashKey, hasHash)`: `hasHash==true` → `table[hashKey % M]`; `hasHash==false` → a uniform random table index from the injected rng (the Envoy random-LB fallback — the `ringHashLB` precedent). `Manager.buildCluster` gains ONE `case clusterv3.Cluster_MAGLEV` parsing `Cluster.MaglevLbConfig` (`table_size` `UInt64Value`, self-supplied default 65537; the hand-rolled PGV cap `≤ 5000011` + the primality gate — AMEND-M1/M5) and constructing `maglevLB`; the `default` reject TEXT extends its supported-list `…, RING_HASH` → `…, RING_HASH, MAGLEV` (the ONE deliberate byte-stable-reject change; blast radius THREE sites — AMEND-M5). The ADR-0235 seam, `cluster.go`, the producers, and every pick-funnel consumer are UNTOUCHED (maglev consumes the seam exactly as `ringHashLB` does).

**Tech stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — `Cluster.LbPolicy MAGLEV` enum value 5 + `Cluster.MaglevLbConfig` are already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` empty — AMEND-M1). Reuses `internal/cluster/` (the 02/34/35/36 Manager + the ADR-0235 `Pick(hashKey, hasHash)` seam + the `ringHashLB` structural template + the `newPCGRNG` RNG helper + the `xxHash64`/`xxHash64Seed` digests), the LANDED producers (`internal/filter/tcpproxy/filter.go`, `internal/filter/http/router/router.go` — UNCHANGED), the differential harness (the `0062` HTTP-header NAT-transparent cross-side affinity+spread harness + `StatsAsserter` + `DistributionAsserter` + the `HTTPEcho` backend), upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/maglev/maglev_lb.{h,cc}` + `.../common/thread_aware_lb_impl.{h,cc}`) for the algorithm pins. ZERO new packages; ZERO `internal/filter/` touches.

**Authored:** 2026-06-13. **Empirical-pin probe date:** 2026-06-13.

---

## 1. Purpose / Mission

Phase 37 lands `maglev`, the **FOURTH Load-balancing-family row** — landed on the ADR-0235 LB hash-key seam that phase 36 (`ring_hash`) built, REUSING it UNCHANGED. Unlike phase 36, phase 37 builds NO seam and NO producer: it consummates the ADR-0235/ADR-0236 §Consequences anticipation (*"the durable asset maglev reuses unchanged"* / *"the cheap follow-on"*). `maglevLB` is the seam's **first hash-policy reuse** — a consistent-hash policy that swaps the Ketama RING for a fixed-size Maglev lookup table behind the SAME `Pick(hashKey, hasHash)` seam, returning the shared `noopRelease` (it holds no per-pick state — the table is built once). Phase 37 is also the project's fifth LB policy; the `loadbalancer.go` doc comment ("Future phases that introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here") names this exact phase — MAGLEV is the **last named-in-the-comment policy**.

This SPEC refines the phase-37 BRAINSTORM (`docs/envoy-go/phases/37-load-balancer-maglev/BRAINSTORM.md`, Q0/Q1) against the AS-BUILT `internal/cluster` package + the §11 D-M1..D-M6 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a `--mode validate` table_size matrix + a live MAGLEV-cluster `/stats` name-set diff on a docker bridge network per `reference_docker_probe_bridge_network`), (2) go-control-plane `/envoy v1.32.4` bindings, and (3) upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/maglev/` + the shared `thread_aware_lb_impl`). It anchors the ADR-0238 §Context DRAFT (§13).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-M1..D-M6 scrape CONFIRMED most BRAINSTORM anticipations with **ONE load-bearing refutation** (the primality reject is PARITY, not a DEPARTURE). The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-M1 (D-M1 — the v1.32.4 MAGLEV surface re-pinned; ONE bounded `table_size` field; default doc-comment-only; ZERO new dep).** `Cluster_MAGLEV` is `Cluster_LbPolicy` enum value **5** (`cluster.pb.go:128`). `Cluster.MaglevLbConfig` carries a SINGLE field `table_size` (`*wrapperspb.UInt64Value`, **proto field 1**, `cluster.pb.go:2497`); there is **NO `hash_function` field** (it exists only on `Cluster_RingHashLbConfig` — maglev always uses xxHash64). The PGV bound is ONLY `value must be less than or equal to 5000011` (`cluster.pb.validate.go:3333` — no lower bound, no primality in PGV). The default **65537 is doc-comment-only** (`cluster.pb.go:2504-2507`: *"If it is not specified, the default is 65537"*) — the getter returns `nil` on unset, so the manager SELF-SUPPLIES 65537 (the ring_hash `parseRingHashLbConfig` precedent — the v1.32.4 zero value is nil, NOT generated). The `lb_config` oneof has `maglev_lb_config` (wrapper `Cluster_MaglevLbConfig_`, field 52). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK in the SPEC worktree — **ZERO new go.mod dep**. See §5 / §11.1.
- **AMEND-M2 (D-M2 SPEC-BLOCKING — the Maglev table-build algorithm pinned byte-for-byte; ZERO new hash code; cross-side-deterministic).** From `maglev_lb.cc`/`thread_aware_lb_impl.h` at v1.37.2: the per-host key is `host->address()->asString()` = **`"IP:PORT"`** (port-inclusive; `use_hostname_for_hashing` defaults false) — which is EXACTLY envoy-go's `Endpoint.Addr()` (`cluster.go:39`, `fmt.Sprintf("%s:%d", e.Host, e.Port)`). `offset = xxHash64(key, seed 0) % M`; `skip = (xxHash64(key, seed 1) % (M-1)) + 1` (∈ `[1, M-1]`) — BOTH already available via `hash.go`'s `xxHash64`/`xxHash64Seed` (**no new hash code** — the ONE deviation from ring_hash's `addr_i`-suffixed key is that maglev hashes the bare `Addr()` with seeds 0 and 1, NOT a per-replica suffix). Hosts are SORTED by the key string ascending BEFORE the populate loop (the Envoy `std::sort` on `(key, host, weight)` — `maglev_lb.cc:107-120`; the Go port MUST replicate this or the table differs). The populate loop fills `table[(offset + skip·next) % M]` host-by-host (in sorted order, one slot per host per iteration under equal weight) until all `M` slots fill; it TERMINATES because `M` prime → each `skip` coprime to `M` → a full `M`-cycle reaches every empty slot. Lookup: `table[hashKey % M]` (the `attempt > 0` retry mutation is unused — envoy-go's `Pick` has no retry arg). The weighted fill (`iteration·weight < target_weight` gate) collapses to plain round-robin placement under equal weight → the equal-weight MVP omits the weight machinery (weighted maglev DEFERRED, the ring_hash weighted-ring analogue). Byte-exact cross-side table reproduction is FEASIBLE given identical xxHash64 + identical key string + identical pre-build sort + same `M`. See §3.1 / §11.2.
- **AMEND-M3 (D-M3 — the ADR-0237 producers feed maglev IDENTICALLY; a CONFIRMATION, not a build).** The HTTP route `hash_policy` producer (`cluster.HashHeaderValues` + the router's `applyHashKey` → `cluster.WithHashKey`) and the tcp `source_ip` producer are policy-AGNOSTIC: they stuff a `uint64` into `ctx`; `cluster.go`'s pick funnel reads it via `hashKeyFrom(ctx)` and calls `c.lb.Pick(hk, ok)` (`cluster.go:232-233`/`286-287`). `maglevLB.Pick(hashKey, hasHash)` receives the SAME key as `ringHashLB.Pick` does — VERBATIM, with NO producer change. Maglev is the FIRST policy to consume the ADR-0237 producer WITHOUT changing it, validating its generality. See §3.2 / §4.
- **AMEND-M4 (D-M4 — +2 `maglev_lb.*` gauges CONFIRMED LIVE; surface 1119 → 1121; cross-side-exact).** A live MAGLEV-cluster `/stats` name-set diff vs ROUND_ROBIN (bridge network; `downstream_cx_rx_bytes_total: 11824` > 0; `upstream_cx_total: 128`) found EXACTLY TWO new gauges: `cluster.<name>.maglev_lb.min_entries_per_host` + `cluster.<name>.maglev_lb.max_entries_per_host` (for a 3-host default-65537 cluster: 21845 / 21846). **NO `size` gauge** (the table size is config-known — confirmed absent). These are STATIC (set once at table build) and CROSS-SIDE-EXACT — `min = floor(M/N)`, `max = ceil(M/N)`, keyed ONLY on `(table_size, host count, weights)`, address-INDEPENDENT (the ring_hash gauge posture). Stat surface **1119 → 1121** at the IMPL. See §7 / §11.4.
- **AMEND-M5 (D-M5 SPEC-BLOCKING — the primality reject is PARITY, not a DEPARTURE; the cap reject is PGV parity; the blast radius is THREE sites; both rejects unit-level).** ⚠️ **REFUTATION of BRAINSTORM Q1's premise:** the reference Envoy DOES reject a non-prime `table_size` — at config-init (`source/server/.../server.cc`; `maglev_lb.cc:316-319` `if (!Primes::isPrime(table_size_)) throw EnvoyException(...)`) with the verbatim message **`The table size of maglev must be prime number`** (the `--mode validate` matrix: `table_size: 100` → REJECT, NOT accept). So envoy-go's primality reject is **PARITY** (a faithful reproduction), not the envoy-go-strict DEPARTURE the BRAINSTORM assumed — the *decision to reject* stands and is now BETTER justified (it mirrors the reference). The cap reject (`table_size > 5000011`) ALSO rejects at validate (the PGV `value must be less than or equal to 5000011`). The reject-text blast radius of the supported-list change (THREE sites, the phase-34/35/36 pattern): `manager.go:275` (`…, RING_HASH` → `…, RING_HASH, MAGLEV`) + `manager_test.go:320-329 TestManager_Error_UnsupportedLBPolicy` (which ALREADY triggers on `Cluster_MAGLEV` from the phase-36 retarget — the doubly-hit retarget MAGLEV → **`Cluster_CLUSTER_PROVIDED`** [enum 6] + the substring `"…, RING_HASH, MAGLEV"`) + `BEHAVIOR_CONTRACT.md` (MAGLEV moves OUT of the rejected set → the fifth accepted policy). Both the primality + cap rejects land UNIT-LEVEL in `manager_test.go` (the ring_hash/random precedent — no fixture pins them, both sides reject identically; a cross-side boot-reject dir is now FEASIBLE but NOT taken — §6/§8.2). See §6 / §11.5.
- **AMEND-M6 (D-M6 — the seam + producers REUSE unchanged; ~140–200 prod LoC / ~8–10 tasks; single ADR-0238; ADR-0024 unamended; the `0063` design pinned).** The ADR-0235 seam + the ADR-0237 producers are reused UNCHANGED: `maglevLB` returns the shared `noopRelease`, requires NO `Pick`-signature change, NO `loadBalancer` interface change, NO `cluster.go` change, NO producer touch, NO consumer touch — the same zero-churn posture `ringHashLB` occupies on the PICK side. Production footprint: `maglev.go` ~110–160 (the sort + offset/skip derivation + the populate loop + `Pick` + the `isPrime` helper + min/max) + the manager case + `parseMaglevLbConfig` ~30 + the 2 gauge registrations ~5 + the reject-text line + the `manager_test.go` retarget → **~140–200 prod LoC / ~8–10 tasks**, FAR under the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`); NO escape valve. ONE new ADR (ADR-0238, the policy ONLY — no seam ADR [ADR-0235 reused], no producer ADR [ADR-0237 reused]; the phase-35 single-ADR-on-reuse shape). ADR-0024 (per-cluster counter scope) is UNAMENDED — maglev holds no per-cluster counter state (only the table + the rng). The `0063-lb-maglev` design mirrors `0062` (N=16 × K=16 = 256; the modular invariant `≡ 0 mod K`; spread `≥ 2`; cross-side gauges-equal). See §3 / §8 / §11.6.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0237** (the HTTP route `hash_policy` producer, ACCEPTED); next-free **ADR-0238**. Per the phase-37 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS tail **STAYS ADR-0237 at this SPEC** (counts UNCHANGED at the SPEC); phase 37 has a single reuse-case ADR with no seam-build to pre-document, so its ADR-0238 §Context is anchored as a DRAFT in §13 and the full DECISIONS.md entry (§Context + §Decision + §Consequences) lands at the phase-37 IMPL per ADR-0044. All six D-M pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The `Cluster.load_balancing_policy` extension point** — the typed-extension LB config path (`load_balancing_policies.maglev.v3.Maglev`, with richer knobs incl `use_hostname_for_hashing`). A much larger config seam; shared with ring_hash's/least_request's/random's deferred extension path.
- **Weighted-table richness** — per-endpoint weight scaling beyond the equal-weight MVP (weighted maglev gives heavier hosts proportionally more table entries via the `iteration·weight < target_weight` gate — AMEND-M2); the ring_hash weighted-ring deferral analogue. The equal-weight MVP omits the weight machinery (it collapses to plain round-robin placement).
- **The `MURMUR_HASH_2`-style hash-function choice** — maglev has NO `hash_function` knob (it always uses xxHash64 — AMEND-M1); nothing to defer.
- **`use_hostname_for_hashing`** — the reference default is `false` → the key is `address()->asString()` (`"IP:PORT"`); the hostname-keyed variant lives on the deferred extension-point path.
- **The bounded-load (`hash_balance_factor`) wrapper** — default 0 ⇒ absent; the `BoundedLoadHashingLoadBalancer` overflow-probing wrapper is out of scope.
- **All other policies** — CLUSTER_PROVIDED, LOAD_BALANCING_POLICY_CONFIG, subset LB, locality-weighted LB, priority load balancing, panic thresholds — stay rejected with the NEW byte-stable text (§6.2). Each a future family row.
- **Healthy-host filtering / priority / panic** — the reference's pick-time host-set selection happens UPSTREAM of the table lookup; envoy-go has no health checking (the Upstream-robustness family's territory) → maglev builds the table over the full endpoint set; the boundary is recorded in BEHAVIOR_CONTRACT at IMPL.
- **lb-specific latency histograms** — the 2 `maglev_lb.*` GAUGES land (AMEND-M4); histograms deferred project-wide (ADR-0060).
- **Per-route applicability** — `lb_policy`/`MaglevLbConfig` are `Cluster` fields; cluster-scoped at bootstrap; the per-route hash-key SOURCE (`RouteAction.hash_policy`) was BUILT at 36.2 (ADR-0237) and is REUSED unchanged — phase 37 introduces NO new per-route surface (BRAINSTORM §4).
- **Any seam / producer change** — the ADR-0235 seam + the ADR-0237 producers are reused UNCHANGED (AMEND-M3/M6).

---

## 3. The `maglevLB` policy on the REUSED ADR-0235 seam (ADR-0238)

### 3.0 Split disposition — D-M6 RESOLVED (single flat phase; NO escape valve)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-37 surface (§11.6 / AMEND-M6):

| Unit | Anticipated production LoC |
|---|---|
| `maglev.go`: the `maglevLB` type (sort by `Addr()` + offset/skip derivation + the populate loop + `Pick` + the `isPrime` helper + min/max gauge fields) | ~110–160 |
| `manager.go`: the `case clusterv3.Cluster_MAGLEV` construction + `parseMaglevLbConfig` (the `table_size` cap + primality gate) + the ONE reject-text line edit | ~30 |
| `manager.go registerClusterMetrics`: the 2 `maglev_lb.*` gauge registrations (mirroring the `*ringHashLB` type-assert block) | ~5 |
| Consumer / seam / producer touches | **0** (all reused unchanged — AMEND-M3/M6) |
| The `0063` fixture driver + the affinity/spread asserter + unit/property tests | test-side LoC, NOT counted |

Net production **~140–200 LoC, ~8–10 tasks** — BOTH axes FAR under the gate (smaller than ring_hash's 36.1 leg, which BUILT the seam + a producer). **Single flat phase 37 — NO pre-split, NO escape valve** (the producers already exist — there is no second subsystem to couple, unlike ring_hash-36; the least_request-34/random-35 single-flat-row precedent). The PLAN re-checks the gate per ADR-0045 (anticipated NO SPLIT with an order-of-magnitude margin).

### 3.1 The `maglevLB` policy (ADR-0238; Maglev consistent-hash table — AMEND-M2)

`internal/cluster/maglev.go` (NEW file, same package — the `ringhash.go`/`random.go` precedent). The build is pinned byte-for-byte from upstream v1.37.2 (§11.2). Indicative shape (the PLAN/IMPL finalizes):

```go
// maglevLB is a per-cluster consistent-hash load balancer implementing the
// Maglev lookup-table algorithm, mirroring Envoy v1.37.2's MaglevTable
// (source/extensions/load_balancing_policies/maglev/maglev_lb.cc). The table is
// a fixed-size []int (each slot an endpoint index) built once at construction:
// hosts sorted by Addr() ascending, each with offset = xxHash64(addr) % M and
// skip = (xxHash64Seed(addr,1) % (M-1)) + 1, populated via (offset+skip*next)%M
// until all M slots fill (terminates: M prime => skip coprime to M => full cycle).
// Pick indexes table[hashKey % M]; with hasHash false it draws a random table
// index (rng() % M — the reference random() mirror). ADR-0238 (the maglev
// policy); ADR-0232 (the RELEASE half
// — reuses noopRelease unchanged); ADR-0235 (the PICK-INPUT-half hash key — reused).
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

var _ loadBalancer = (*maglevLB)(nil)

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

// Pick selects table[hashKey % M]. With hasHash false it draws a random table
// index (rng() % M — the Envoy random()-LB fallback; near-uniform, never on the
// differential path). The release func is
// noopRelease (maglevLB holds no per-pick state); non-nil on every path.
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

- The struct mirrors `ringHashLB`'s build-once + immutable + injected-rng shape (the no-hash fallback parity); it replaces the sorted `[]ringEntry` with a `[]int` lookup table.
- `newPCGRNG()` is REUSED verbatim (the `ringHashLB`/`randomLB` precedent — already package-private, shared-ready); the injectable `newMaglevWithRNG(endpoints, cfg, rng)` test seam mirrors `newRingHashWithRNG`.
- The **only deviation from ring_hash's key derivation** is the hashed string: ring uses `fmt.Sprintf("%s_%d", addr, i)` per replica; maglev uses the bare `Addr()` with seeds 0 and 1 (offset/skip). No new hash code (AMEND-M2).
- The **sort-by-`Addr()`** step is load-bearing for cross-side determinism of the TABLE (D-M2): omit it and the table differs from the reference. It is the one structural detail ring_hash did not need (the ring sorts by hash, not by host). NOTE the `minPerHost`/`maxPerHost` GAUGE values are sort-order-INVARIANT (`floor(M/N)`/`ceil(M/N)` regardless of host order) — unlike the table layout, which is sort-order-sensitive.
- Empty-set → `Endpoint{}, noopRelease, errNoEndpoints` (the ring_hash/random parity; `buildCluster` already rejects zero-endpoint clusters via `extractEndpoints` — defense in depth).
- The `attempt > 0` retry-hash mutation (`maglev_lb.cc:266`) is UNUSED — envoy-go's `Pick` has no `attempt` argument (the single-pick funnel; recorded BEHAVIOR_CONTRACT boundary, like ring_hash).

### 3.2 The seam REUSE (the ADR-0235 seam + the ADR-0237 producers UNCHANGED — AMEND-M3/M6)

`maglevLB` implements the existing unexported `loadBalancer` interface (`Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)`, `loadbalancer.go:20`) and returns the shared `noopRelease` — exactly as `ringHashLB` does. NO interface change, NO `cluster.go` / pick-funnel / `Dial` / `AcquireH1` change, NO producer-site change. The ADR-0235 PICK-INPUT seam (`hashKeyFrom`/`WithHashKey` + the `c.lb.Pick(hk, ok)` funnel — `cluster.go:232-233`/`286-287`) delivers the ctx-carried key to `maglevLB.Pick` IDENTICALLY to `ringHashLB.Pick`. The ADR-0237 HTTP `hash_policy` producer (`cluster.HashHeaderValues` + `applyHashKey`) and the 36.1 tcp `source_ip` producer (`cluster.HashSourceIP`) are policy-AGNOSTIC and UNTOUCHED — they stuff a key into `ctx`; maglev indexes a different structure with it. This is the seam's FIRST hash-policy REUSE (the hash-policy analogue of phase-35 reusing the ADR-0232 release seam) — ZERO seam code/ADR, ZERO producer code/ADR; the single ADR is the policy (ADR-0238).

### 3.3 Manager acceptance + the MaglevLbConfig gate (ADR-0238; D-M1/D-M5)

`manager.go buildCluster`: the `lb_policy` switch (line 243) gains one case; construction parses the bounded `table_size` + the primality gate:

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

- The cap check precedes the primality check (the reference order: PGV proto-constraint fires before the app-layer `isPrime` throw — AMEND-M5). The default 65537 (a known prime — the Fermat prime 2^16+1) bypasses both checks (self-supplied constant); `0`/`1`/composite explicit values are rejected by the primality gate (parity).
- `isPrime(uint64)` is a small new helper (trial division to `√n`; adequate for `n ≤ 5000011`). The exact reject wording for the primality leg is IMPL-finalizable within the pinned principle (§6.2) — it need not match the reference's app-layer string verbatim (no fixture pins it; the house `cluster: %q: …` prefix is the envoy-go convention, as ring_hash's PGV-mirror rejects already are).
- A stray `ring_hash_lb_config`/`least_request_lb_config` under a MAGLEV cluster is silently ignored (the manager reads `GetMaglevLbConfig()` only on the MAGLEV path — reference PARITY, §6.3; the validate matrix confirmed silent-accept).
- The 2 `maglev_lb.*` gauges are registered + Set ONCE at `registerClusterMetrics` via a `*maglevLB` type-assert mirroring the `*ringHashLB` block (`manager.go:110-117`) — MAGLEV-only, reference parity (§7).

---

## 4. Framework primitives — 0 new framework seams + 0 new packages + 0 new go.mod deps + 0 new hash code

Phase 37's framework delta is ZERO. It REUSES the ADR-0235 LB hash-key seam + the ADR-0237 producers UNCHANGED (§3.2 — the seam's first hash-policy reuse). ZERO new packages (the `maglevLB` type + the manager acceptance/parse land in the existing `internal/cluster`: a new `maglev.go` + `manager.go`); ZERO `internal/filter/` touches; ZERO new go.mod deps (AMEND-M1); ZERO new hash code (the table build reuses `hash.go`'s `xxHash64`/`xxHash64Seed` — AMEND-M2). maglev is not a filter — no builtins registration, no TypeURL factory, no bootstrap blank-import (the `clusterv3` proto is already imported by `internal/cluster`). The lone new internal helper is `isPrime` (the config-build primality gate).

---

## 5. Proto-field roster (per §11.1 D-M1)

All from go-control-plane `/envoy` v1.32.4 (`config/cluster/v3/cluster.pb.go` + `cluster.pb.validate.go`), verified in the module cache this session.

### 5.1 `Cluster.LbPolicy` enum values at the guard

| Value | Enum | 37 disposition |
|---|---|---|
| 0 | `ROUND_ROBIN` (proto default; unset ≡ ROUND_ROBIN) | accepted (phase 02) |
| 1 | `LEAST_REQUEST` | accepted (phase 34) |
| 2 | `RING_HASH` | accepted (phase 36) |
| 3 | `RANDOM` | accepted (phase 35) |
| **5** | **`MAGLEV`** | **accepted (THIS PHASE)** |
| 6 | `CLUSTER_PROVIDED` | rejected (the next reject-test trigger — the retarget target) |
| 7 | `LOAD_BALANCING_POLICY_CONFIG` | rejected |

### 5.2 `Cluster.MaglevLbConfig` (the config-parse arm; AMEND-M1)

| Field | Proto | Type | Disposition |
|---|---|---|---|
| `table_size` | field **1** | `google.protobuf.UInt64Value` | parsed; default 65537 (doc-comment, self-supplied on nil); PGV `value must be less than or equal to 5000011`; envoy-go ADDS the primality gate (reference parity — AMEND-M5) |

- There is **NO `hash_function` field** (maglev always uses xxHash64 — lighter than ring_hash's three-field arm). The `lb_config` oneof wrapper is `Cluster_MaglevLbConfig_` (field 52) — a stray non-maglev oneof member under a MAGLEV cluster is mismatched-but-valid (nil-safe getters; silently ignored — §6.3). NO new go.mod dep (the enum + config are in the pinned v1.32.4 module — `go mod tidy -diff` empty).

---

## 6. PARSE-REJECT roster (per §11.5 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080: the supported-list reject-text CHANGE at `manager.go:275` (the as-built / PRE-edit anchor — inserting the `case Cluster_MAGLEV` block shifts the `default` return down; Task 1 re-pins) is the ONE deliberate contract-surface change this phase (the §9 / phase-34/35/36 byte-stable-reject lineage: change it ONCE, with the blast radius enumerated — AMEND-M5: `manager.go:275` + `TestManager_Error_UnsupportedLBPolicy` + `BEHAVIOR_CONTRACT.md`; NO fixture pins the text). PLUS the two `table_size` reject arms (cap + primality). All verified by table tests at IMPL.

### 6.2 Reject arms (all UNIT-TESTED; NO cross-side boot-reject dir — AMEND-M5)

- `cluster-lb-policy-unsupported` — the REPLACEMENT supported-list text: `cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV)`. `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320`) RETARGETS its trigger from `Cluster_MAGLEV` (now accepted) to `Cluster_CLUSTER_PROVIDED` (enum 6, the next still-rejected policy) and re-pins the new substring `"ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV"`. The phase-34/35/36 "doubly-hit" pattern recurs (the accept of policy N forces the reject test that used N as its trigger to retarget to N+1).
- `maglev-table-size-cap` — `table_size > 5000011` → `cluster: %q: maglev_lb_config.table_size: value must be less than or equal to 5000011` (the hand-rolled PGV-parity mirror; the ring_hash `ringSizeCap` precedent). Reference PARITY (the reference rejects at validate via the PGV proto-constraint).
- `maglev-table-size-primality` — a configured composite `table_size` (≤ cap, but not prime, incl 0/1) → `cluster: %q: maglev_lb_config.table_size (%d) must be a prime number`. Reference PARITY (⚠️ REFUTATION of BRAINSTORM Q1 — the reference rejects with `The table size of maglev must be prime number`; envoy-go uses the house-prefixed form, no fixture pins it — AMEND-M5). The maglev permutation REQUIRES `M` prime for a terminating full-cycle fill (§3.1 / §11.2).
- There are NO other reject arms — maglev has no `hash_function` knob, no min/max cross-field rule (the ring_hash arms that do NOT recur).

### 6.3 NON-reject dispositions (parity)

- A stray `lb_config` oneof member under `lb_policy: MAGLEV` (e.g. `ring_hash_lb_config` or `least_request_lb_config`): **silent-ignore** — reference PARITY (the `--mode validate` matrix accepted `MAGLEV` + stray `ring_hash_lb_config` SILENTLY, zero warnings — §11.5) AND behavior-INERT (the manager reads `GetMaglevLbConfig()` only on the MAGLEV path).
- `lb_policy: MAGLEV` with NO `maglev_lb_config` (or with `maglev_lb_config: {}` / `table_size` unset): accepted, default 65537 (reference parity — the doc-comment default; §11.1).

---

## 7. Stat surface — +2 `maglev_lb.*` gauges (per §11.4 D-M4 + AMEND-M4)

- **TWO new stat names.** Surface **1119 → 1121** (at the IMPL). Live-proven: the full `/stats` name-set diff MAGLEV-vs-ROUND_ROBIN is EXACTLY two names — `cluster.<name>.maglev_lb.min_entries_per_host` + `cluster.<name>.maglev_lb.max_entries_per_host` (gauges). **NO `size` gauge** (the table size is config-known, unlike ring_hash's computed `ring_hash_lb.size`) — confirmed absent live. They describe maglev's hallmark even table distribution (per-host entry-count variance).
- Registered + Set ONCE at `registerClusterMetrics` via a `*maglevLB` type-assert (the `*ringHashLB` block precedent) — MAGLEV-only (reference parity). STATIC (the table is immutable) and **CROSS-SIDE-EXACT**: `min = floor(M/N)`, `max = ceil(M/N)`, keyed ONLY on `(table_size, host count, weights)`, address-INDEPENDENT (for a 3-host default-65537 cluster: 21845 / 21846 on BOTH sides). The `0063` `StatsAsserter` cross-equal prong.
- The `0063` `StatsAsserter` set (the `0062` precedent): cross-equal `cluster.<name>.upstream_cx_total` (= N·K = 256) + `upstream_rq_total` (= 256, the HTTP plane Inc's it both sides) + `membership_total` (= 3) + `upstream_cx_active` (= 0, quiesced post-drain) + `maglev_lb.min_entries_per_host` (= 21845) + `maglev_lb.max_entries_per_host` (= 21846). All cross-side-exact (the gauge cross-equality is the strong maglev-specific prong — keyed on config + host count, not addresses).

---

## 8. Differential fixture taxonomy (+1: `0063` HTTP cross-side affinity+spread)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (no boot-reject dir — §8.2). Per `reference_differential_asserter_dispatch`: the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the `0062` precedent). Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0063'` (NOT `-run '0063'`, which matches zero subtests). Numbering continues from `0062`; re-pinned at IMPL Task 1.

### 8.1 `0063-lb-maglev` (cross-side; HTTP route header hash; per-side affinity invariant + spread + cross-side gauges)

Chain `[http_connection_manager + router]` on BOTH sides (the `0062` shape: reference STRICT_DNS / `host.docker.internal`, subject STATIC / `127.0.0.1`) over ONE 3-endpoint cluster with `lb_policy: MAGLEV` + `maglev_lb_config: {}` (default 65537) + a **route-level** `hash_policy: [{header: {header_name: "x-hash"}}]` on BOTH sides (the ADR-0237 producer, REUSED). Backends: the existing **`HTTPEcho`** backend of `0062` REUSED. **NO new BackendKind** (tail STAYS 33).

**The workload (identical per side — the `0062` stimulus REUSED verbatim, retargeted to MAGLEV):** for each of **N=16** distinct `X-Hash` values (`hv-0` … `hv-15`) the driver sends **K=16** `GET /get` requests carrying that value in the `X-Hash` header → **16 × 16 = 256** routed requests (`totalReqs` DERIVED from `N·K`, never a literal — `reference_fixture_workload_constant_desync`). Each request is a fresh dial (`Connection: close`) → one upstream connection per request. Then **8 `GET /health`** round-trips (a listener `direct_response` `inline_string: "OK\n"` — address-independent → byte-equal; the runner's `CompareBytes` input).

**The affinity + spread arm (DETERMINISTIC/EXACT modular invariant, via `AssertDistribution` on the per-backend accept counts; the `0062` convention):**
- **BOTH-SIDE affinity** — each per-backend count `≡ 0 mod K` (K=16): one `X-Hash` value → one xxHash64 digest → one table slot → one backend, so a value contributes all 16 or 0 to a backend. (A per-request scatter break produces a non-multiple-of-16 total.)
- **BOTH-SIDE spread** — `≥ 2` backends nonzero (16 values over 3 backends; a degenerate single-entry table would collapse to 1).
- **BOTH-SIDE conservation** — `c1 + c2 + c3 == 256` (= N·K; hard equality).
- **NOT asserted: cross-side host IDENTITY** — the subject/reference tables are built over DIFFERENT endpoint address strings (`"IP:PORT"` differs cross-side; `reference_differential_hash_key_cross_side_infeasible`), so a given `X-Hash` value may land on a DIFFERENT backend idx per side. The modular invariant proves affinity WITHOUT host attribution; the routed bodies are NOT in the compared byte stream (only the `/health` `OK\n` stream is).

Asserted on BOTH sides (the X-Hash header is NAT-transparent — it survives the Docker hop verbatim, unlike a tcp `source_ip` key; each side independently satisfies the invariant). The invariant is DETERMINISTIC (not a σ-band) → no flake margin needed for affinity/conservation; spread is robust at N=16 (`P(all 16 values → 1 backend) ≈ 3·(1/3)^16 ≈ 7e-8` per side — the `0062` N=4→16 lesson REUSED).

- **Deliberate-break liveness (`-count=1`, `reference_differential_break_protocol_count1`):** (i) scatter the key — hash per-request (e.g. mix in a nonce) instead of per-value → at least one per-backend total stops being `≡ 0 mod 16` → the affinity leg FAILS (the canonical affinity break); (ii) collapse the BUILT table — force every `table[*] = endpoints[0]` AT BUILD → only 1 backend nonzero → the spread leg FAILS AND the gauge prong FAILS (the build slot-tally goes `max = 65537`/`min = 0` — the gauges are the per-host SLOT tally over M slots, NOT the per-request accept counts, so the figures are 65537/0, never 256/0); a WEAKER variant (short-circuit `Pick` to `endpoints[0]` WITHOUT touching the build) bites the spread leg ONLY — the table is still built normally → the gauges stay 21845/21846 (so name the break precisely to avoid a vacuous gauge assertion — `reference_differential_asserter_dispatch`); (iii) drop a `StatsAsserter` Inc / perturb a gauge → the stats prong FAILS. Recorded in driver comments + README per the `0030` lesson.

**The stats prong (cross-side `StatsAsserter`, post-drain):** §7's set — cross-equal `upstream_cx_total == 256` + `upstream_rq_total == 256` + `membership_total == 3` + `upstream_cx_active == 0` + `maglev_lb.min_entries_per_host == 21845` + `maglev_lb.max_entries_per_host == 21846` (the gauges cross-side-exact — the strong maglev-specific prong).

### 8.2 NO boot-reject dir (AMEND-M5)

The `table_size` cap + primality reject arms land UNIT-LEVEL in `manager_test.go` (the ring_hash/random precedent): both are reference PARITY (each side rejects identically) and NO fixture pins the text, so a cross-side boot-reject dir adds a runner branch for zero additional coverage. (A cross-side boot-reject dir is now FEASIBLE — the reference rejects both at validate, AMEND-M5 — but NOT taken; the unit tests are sufficient and lower-cost, and the dispatch-constraint keeps boot-reject separate from `0063`.) The supported-list reject arm (the still-rejected CLUSTER_PROVIDED departure) is also unit-level. Fixture count 64 → **65** (`0063-lb-maglev` only).

### 8.3 NO new BackendKind + NO new fuzzer (family expectations)

BackendKind tail STAYS **33** (`0063` reuses the `0062` `HTTPEcho` — an LB phase exercises WHERE requests land, not what the backend speaks; the phase-34/35/36 first recurs). Fuzzers STAY **42** — maglev decodes no wire bytes (the hash key derives from a parsed header value, not a wire frame). A table-BUILD/lookup property test (random keys → always a valid endpoint, never panics; the table is fully filled / no `-1` remains; equal-weight distribution within ±1 entry/host; affinity holds) is a strong UNIT-level candidate but is NOT a `Fuzz*` corpus entry — D-M6 decides a `FuzzMaglevLookup` (anticipated NO — no untrusted wire input). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate (maglev touches the cluster LB pick, NOT the h2 framing or the wasm path; the wire path is byte-identical when no `lb_policy: MAGLEV` cluster is configured — every conformance config). The six-gate's REAL guard is the full 64-dir differential re-verify: all 64 existing dirs must stay byte-exact through the new manager case (the seam is unchanged → behavior-neutrality is structural).

---

## 9. Behavior-contract delta (the 37 bundle; ADR-0052 atomic landing)

At IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### Load balancer — maglev (MAGLEV)` subsection beside the ring_hash entry: the MAGLEV acceptance (the `table_size` default 65537 + the cap + the primality gate); the Maglev table semantics (sort-by-Addr + offset/skip + the populate loop + `table[hashKey % M]`; the no-hash random fallback; the unused `attempt` retry boundary); the seam + producer REUSE (ADR-0235/ADR-0237 unchanged); the healthy-set boundary (no health checking → all-hosts table).
- The 2 `maglev_lb.*` gauges added to the `## Stat-name mapping` table (MAGLEV-only; Set once at table build; cross-side-exact `floor(M/N)`/`ceil(M/N)`; the stat surface 1119 → 1121).
- The reject-text entry updates: MAGLEV retired from the rejected set → the fifth accepted policy; the supported-set string `…, RING_HASH, MAGLEV`; the `table_size` cap + primality rejects (the primality is now PARITY — the reference rejects `The table size of maglev must be prime number`); CLUSTER_PROVIDED becomes the lone recorded LB-policy departure / retarget trigger.
- Departure/coverage records: the mismatched-oneof silent-ignore under MAGLEV (parity); NO new fuzzer/BackendKind (family expectations); the cross-side host-identity-infeasible-but-affinity-provable posture (the `0062` precedent).

---

## 10. Per-task structure (~8–10 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **64** (tail `0062`) + fuzzers **42** + stat surface **1119** + BackendKind tail **33** + DECISIONS tail **ADR-0237** via the canonical recipes; re-pin the as-built anchors (`loadbalancer.go` interface + `noopRelease`, `ringhash.go` template, `hash.go` `xxHash64`/`xxHash64Seed`, `cluster.go:39` `Addr()` + the `Pick` funnel, `manager.go:243` switch + `:275` reject + `:110-117` gauges, `manager_test.go:320`, the `0062` driver) against the IMPL-session tip; PROGRESS.md created | §11 / §3 |
| 2 | The `isPrime` helper (TDD: known primes incl 65537/5000011, composites incl 0/1/4/100, large near-cap) | §3.3 / §6.2 |
| 3 | The `maglevLB` build: sort-by-`Addr()` + offset/skip + the populate loop (TDD with a fixed endpoint set: full fill / no `-1`, terminates, min/max = floor/ceil(M/N); a small-M hand-computed table vector; reuse `xxHash64`/`xxHash64Seed`) | §3.1 / §11.2 |
| 4 | The `maglevLB.Pick`: `table[hashKey % M]` + the no-hash random fallback (injected rng) + the empty-set `errNoEndpoints` + `noopRelease` return; the affinity property (same key → same endpoint) | §3.1 / §11.2 |
| 5 | Manager acceptance: the `case clusterv3.Cluster_MAGLEV` + `parseMaglevLbConfig` (default 65537 + cap + primality + mismatched-oneof silent-ignore) + the NEW reject text + `TestManager_Error_UnsupportedLBPolicy` retarget (MAGLEV → CLUSTER_PROVIDED + the new substring) (TDD: the §6 matrix, byte-stable table test) | §3.3 / §6 |
| 6 | The 2 `maglev_lb.*` gauge registrations (the `*maglevLB` type-assert in `registerClusterMetrics`; min/max Set once; cross-side-exact) + a boot smoke | §7 |
| 7 | The `0063-lb-maglev` fixture: config (HCM+router, route header `hash_policy` → MAGLEV cluster, both sides) + driver (N=16 × K=16 + 8 `/health`) + the affinity/spread `AssertDistribution` (≡ 0 mod K + spread ≥ 2 + conservation 256) + the `StatsAsserter` prong (cross-equal cx/rq/membership/quiesced + the 2 gauges) + expectations.yaml | §8.1 |
| 8 | Deliberate-break liveness (`-count=1`): scatter-the-key (affinity), collapse-the-table (spread + gauges), stats-prong drop; ≥20-run flake check (`reference_fixture_workload_constant_desync` — N/K/totalReqs synced) | §8.1 |
| 9 | (optional) a table-build/lookup property test (no panic, full fill, ±1 distribution, affinity) — D-M6 decides standalone vs folded | §8.3 |
| 10 | Full differential re-verify (the 64 prior dirs byte-exact through the new manager case + `0063` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected; Completion bundle: BEHAVIOR_CONTRACT 37 bundle (§9) + the ADR-0238 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0238) + the 2 `maglev_lb.*` gauges (surface 1119 → 1121) + STATE/ROADMAP row 37 `in-progress → done` (flat family row — NO parent rollup) + the six-gate evidence | §9 / §13 |

The PLAN re-checks the ADR-0045 gate (anticipated NO SPLIT with margin); it may merge/split these indicative tasks (e.g. fold Task 2's `isPrime` into Task 5, or split the fixture from its tuning).

---

## 11. SPEC-time empirical-pin block (D-M1..D-M6 — executed IN-SESSION 2026-06-13)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-13.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image**: a 5-variant `--mode validate` `table_size` matrix; a live MAGLEV-cluster `/stats` name-set diff vs ROUND_ROBIN on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with three HTTP backends + a route header `hash_policy` (`downstream_cx_rx_bytes_total: 11824` / `upstream_cx_total: 128` verified live).
2. **go-control-plane `/envoy` v1.32.4 bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/config/cluster/v3/`; `go mod tidy -diff` + `go build ./...` in the SPEC worktree.
3. **Upstream Envoy v1.37.2 source** at tag v1.37.2: `source/extensions/load_balancing_policies/maglev/maglev_lb.{h,cc}` + `.../common/thread_aware_lb_impl.{h,cc}` + `api/.../maglev/v3/maglev.proto`.
4. **envoy-go codebase** at master tip `4de2769` (above the phase-37 BRAINSTORM squash `768ecfc`): `internal/cluster/{loadbalancer,ringhash,hash,manager,cluster}.go`, `internal/cluster/manager_test.go`, `test/fixtures/0062-lb-ring-hash-http/`.

### Summary disposition table (6 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-M1 (SPEC-BLOCKING) — proto/surface re-pin + tidy | **CONFIRMED** (enum 5; `table_size` field 1; NO hash_function; PGV ≤ 5000011; default 65537 doc-comment-only; ZERO new dep) | M1 |
| §11.2 | D-M2 (SPEC-BLOCKING) — the Maglev table-build algorithm | **PINNED** (sort-by-Addr; offset/skip seeds 0/1; populate loop; M-prime terminates; cross-side-deterministic; ZERO new hash code) | M2 |
| §11.3 | D-M3 — the producer reuse (confirmation) | **CONFIRMED** (the ADR-0237 producers feed `Pick` identically; no producer change) | M3 |
| §11.4 | D-M4 — the stat-surface delta | **CONFIRMED LIVE** (+2 `maglev_lb.*` gauges; no `size`; cross-side-exact; surface → 1121) | M4 |
| §11.5 | D-M5 (SPEC-BLOCKING) — the reject contract | **RESOLVED + REFUTATION** (primality is PARITY not DEPARTURE — the reference rejects; cap is PGV parity; 3-site blast radius; both unit-level) | M5 |
| §11.6 | D-M6 — the `0063` design + the envelope | **RESOLVED** (N=16 × K=16; ≡ 0 mod K + spread ≥ 2; ~140–200 LoC → single flat row; ADR-0024 unamended; no FuzzMaglevLookup) | M6 |

### 11.1 D-M1 (SPEC-BLOCKING) — the MAGLEV surface: CONFIRMED

`Cluster_MAGLEV Cluster_LbPolicy = 5` (`cluster.pb.go:128`). `MaglevLbConfig.TableSize *wrapperspb.UInt64Value` (proto field 1, `cluster.pb.go:2497`); the getter returns `nil` on unset (the default **65537 is doc-comment-only** — `cluster.pb.go:2504-2507`; the manager self-supplies it, the ring_hash precedent). NO `hash_function` field on `MaglevLbConfig` (it exists ONLY on `Cluster_RingHashLbConfig`). PGV (`cluster.pb.validate.go:3333`): ONLY `value must be less than or equal to 5000011` (no lower bound, no primality in PGV). The `lb_config` oneof: `maglev_lb_config` wrapper `Cluster_MaglevLbConfig_` (field 52) — a stray non-maglev member is mismatched-but-valid (silent-ignore, §11.5). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK. **ZERO new go.mod dep.**

### 11.2 D-M2 (SPEC-BLOCKING) — the Maglev table-build algorithm: PINNED

From `maglev_lb.cc`/`thread_aware_lb_impl.{h,cc}` at v1.37.2:

- **Table size M:** default `DefaultTableSize = 65537` (`maglev_lb.h:67`; selected via `PROTOBUF_GET_WRAPPED_OR_DEFAULT`); cap `5000011` (proto PGV `lte`, not a C++ const); **runtime primality check** `if (!Primes::isPrime(table_size_)) throw EnvoyException("The table size of maglev must be prime number")` (`maglev_lb.cc:316-319`). M prime ⇒ each `skip ∈ [1, M-1]` is coprime to M ⇒ `(offset + j·skip) mod M` is a full M-cycle ⇒ the populate loop reaches every empty slot and TERMINATES.
- **The hashed key:** `host->address()->asString()` (`thread_aware_lb_impl.h:40-53`; `use_hostname_for_hashing` default false) = `"IP:PORT"` (port-inclusive) = envoy-go's `Endpoint.Addr()` EXACTLY (`cluster.go:39`).
- **offset / skip** (`maglev_lb.cc:130-132`): `offset = HashUtil::xxHash64(key) % M` (seed 0); `skip = (HashUtil::xxHash64(key, 1) % (M-1)) + 1` (seed 1). xxHash64 hardwired (no hash_function). Both available via `hash.go`'s `xxHash64`/`xxHash64Seed` — **ZERO new hash code**.
- **Pre-build sort** (`maglev_lb.cc:107-120`): hosts `std::sort`-ed by `(key string, host, weight)` ascending BEFORE building → the build order. The Go port MUST sort endpoint indices by `Addr()` ascending or the table differs (the one structural detail ring_hash did not need).
- **Populate loop** (`maglev_lb.cc:152-182`): `permutation = (offset + skip·next) % M`; per iteration, each host (in sorted order, gated by `iteration·weight < target_weight` — vacuous under equal weight) probes from its `next` pointer to the first empty slot, claims it, increments `next`/`count`; loop until all M slots fill. Under EQUAL weight: one slot per host per iteration → each host ends with `floor(M/N)` or `ceil(M/N)` entries.
- **Lookup** (`maglev_lb.cc:263-276`): `table[hash % M]`; `attempt > 0` mutates `hash ^= (~0ULL - attempt + 1)` (UNUSED in envoy-go — no `attempt` arg). The request `hash` is the route `hash_policy` result / `computeHashKey()` (`thread_aware_lb_impl.cc:182-198`) — the ADR-0237 producer key (D-M3).
- **No-hash fallback** (`thread_aware_lb_impl.cc:198`): `hash = random_.random()` — a uniform random 64-bit value (effectively random-LB). Mirror with the injected `rng()` (the ring_hash precedent); non-deterministic, so differential fixtures always provide a key.
- **min/max entries-per-host** (`maglev_lb.cc:137-145`): computed from each entry's `count_`; emitted as gauges `maglev_lb.{min,max}_entries_per_host` (D-M4).

**Cross-side feasibility:** the equal-weight populated-table path is 100% DETERMINISTIC from `(sorted endpoint address strings, M)`. Byte-exact reproduction requires identical xxHash64 (seeds 0/1), identical key (`Addr()` = `"IP:PORT"`), identical pre-build sort, same M — all satisfied. (Cross-side host IDENTITY is still infeasible — the two sides have different `"IP:PORT"` strings → different tables — so `0063` asserts the per-side modular invariant, not host equality; §8.1.)

### 11.3 D-M3 — the producer reuse: CONFIRMED

The ADR-0237 HTTP producer (`cluster.HashHeaderValues` + the router's `applyHashKey` → `cluster.WithHashKey`) and the tcp `source_ip` producer (`cluster.HashSourceIP`) are policy-agnostic. `cluster.go`'s pick funnel reads `hk, ok := hashKeyFrom(ctx)` and calls `c.lb.Pick(hk, ok)` (`cluster.go:232-233`/`286-287`) — `maglevLB.Pick(hashKey, hasHash)` receives the SAME key as `ringHashLB.Pick`, VERBATIM. NO producer change. Maglev is the FIRST policy to consume the producer without changing it.

### 11.4 D-M4 — the stat-surface delta: CONFIRMED LIVE (+2 gauges)

Live MAGLEV-cluster `/stats` (3 HTTP backends on a bridge network; `downstream_cx_rx_bytes_total: 11824` > 0): the name-set diff vs ROUND_ROBIN (port-noise normalized) is EXACTLY TWO names — `cluster.<name>.maglev_lb.min_entries_per_host` (21845) + `cluster.<name>.maglev_lb.max_entries_per_host` (21846) for a 3-host default-65537 cluster (65537/3 ≈ 21845–21846). **NO `size` gauge** (`grep maglev_lb.size` → none). Both are STATIC + cross-side-exact (`floor(M/N)`/`ceil(M/N)`; address-independent). Surface **1119 → 1121**. (The live probe's `upstream_cx_total: 128` is the probe's incidental workload — 16×8; the `0063` FIXTURE design fixes the cross-equal value at N·K = 256.) The `0063` StatsAsserter set: cross-equal `upstream_cx_total`/`upstream_rq_total` (= 256) + `membership_total` (= 3) + `upstream_cx_active` (= 0) + the 2 gauges (= 21845/21846).

### 11.5 D-M5 (SPEC-BLOCKING) — the reject contract: RESOLVED + REFUTATION

The 5-variant `--mode validate` matrix (contrib-v1.37.2; base = STATIC cluster):

| Variant | Verdict | Reference output (decisive fragment) |
|---|---|---|
| `MAGLEV`, no maglev_lb_config | ACCEPT | `configuration … OK` |
| `MAGLEV` + `table_size: 65537` (prime) | ACCEPT | `OK` |
| `MAGLEV` + `table_size: 100` (composite) | **REJECT** | `The table size of maglev must be prime number` (app-layer critical) |
| `MAGLEV` + `table_size: 5000012` (> cap) | **REJECT** | `…MaglevLbConfigValidationError.TableSize: value must be less than or equal to 5000011` (PGV) |
| `MAGLEV` + stray `ring_hash_lb_config: {}` | ACCEPT, **silent** | `OK`; zero warnings |

⚠️ **REFUTATION of BRAINSTORM Q1:** the reference DOES reject a non-prime `table_size` (the BRAINSTORM assumed it accepts → a DEPARTURE). So envoy-go's primality reject is **PARITY** (a faithful reproduction). Both rejects fire at validate → both are cross-side-feasible — but they land UNIT-LEVEL (the ring_hash/random precedent; no fixture pins them; §8.2). The two rejects emit different strings: primality → app-layer `The table size of maglev must be prime number`; cap → PGV `value must be less than or equal to 5000011`. envoy-go uses the house `cluster: %q: maglev_lb_config.table_size …` prefix for both (the ring_hash PGV-mirror precedent — no fixture pins the exact bytes). **Blast radius** (full-repo greps): production string — `manager.go:275` only (the supported-list); unit pinner — `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320`; its trigger is ALREADY `Cluster_MAGLEV` from the phase-36 retarget → must retarget to `Cluster_CLUSTER_PROVIDED` + the new substring `"…, RING_HASH, MAGLEV"`); docs — `BEHAVIOR_CONTRACT.md` (the reject-text + the gauge-table entries). NO cross-side boot-reject dir (§8.2).

### 11.6 D-M6 — the `0063` design + the ADR-0045 envelope re-check: RESOLVED

The `0063-lb-maglev` design mirrors `0062` verbatim, retargeted to MAGLEV (N=16 × K=16 = 256 routed `GET /get` + 8 `/health`; the `≡ 0 mod K` affinity invariant + spread `≥ 2` + conservation 256, both sides; the gauges cross-side-exact). The live probe confirmed affinity HELD (each x-hash value pinned to one backend across its repeats) + spread (all 3 backends hit). Envelope: ~140–200 prod LoC / ~8–10 tasks (§3.0) — BOTH ADR-0045 legs hold by an order of magnitude. SINGLE FLAT ROW 37; NO escape valve. ADR-0024 (per-cluster counter scope) is UNAMENDED (maglev holds no per-cluster counter state — only the table + rng; ADR-0024 §Consequences already names MAGLEV as a zero-touch drop-in). A `FuzzMaglevLookup` is anticipated NOT warranted (no untrusted wire input — a table property test is unit-level; §8.3). The PLAN re-checks.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S37-1** — the file placement (anticipated: the new `maglevLB` type + `isPrime` in a NEW sibling `maglev.go`; the manager case + `parseMaglevLbConfig` + reject text in `manager.go`; the gauge type-assert in `registerClusterMetrics`; the retarget in `manager_test.go` — the `ringhash.go` precedent).
- **D-S37-2** — the exact primality reject wording (anticipated `cluster: %q: maglev_lb_config.table_size (%d) must be a prime number` — the house prefix; no fixture pins it, so IMPL-finalizable within the §6.2 principle). And whether `isPrime` is a standalone helper or folded into `parseMaglevLbConfig`.
- **D-S37-3** — whether the populate loop ships the equal-weight simplification (one slot per host per iteration) or the general weighted gate (anticipated: equal-weight MVP — the weighted form is DEFERRED §2; the IMPL keeps the loop honest enough that adding weights later is local).
- **D-S37-4** — the `0063` final constants (N=16 / K=16 / 8 health anticipated) + the deliberate-break protocol (scatter-the-key, collapse-the-table, stats-drop) with `-count=1`; ≥20-run flake check; the `reference_fixture_workload_constant_desync` guard (N/K/totalReqs synced with any hand-rolled count slices).
- **D-S37-5** — whether a standalone table-build/lookup property test ships (Task 9) or folds into the fixture's break proofs (anticipated: a small deterministic property test — full fill / no `-1` / ±1 distribution / affinity — distinct from the fixture).
- ADR-0045 split-gate FINAL re-check at PLAN.

---

## 13. ADR continuity — the ADR-0238 §Context DRAFT (anchored here; the full entry lands at the IMPL)

Per the phase-37 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS.md tail **STAYS ADR-0237 at this SPEC** (counts UNCHANGED — phase 37 has a single reuse-case ADR with no seam-build to pre-document). The ADR-0238 §Context is anchored as a DRAFT HERE; the full ADR-0238 entry (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) lands at the phase-37 IMPL per ADR-0044.

**ADR-0238 §Context DRAFT (the `maglev` load-balancing policy):** Phase 37 lands `Cluster.LbPolicy MAGLEV` (`envoy.config.cluster.v3`, enum value 5) on the LEGACY enum path (the same path ROUND_ROBIN + LEAST_REQUEST + RANDOM + RING_HASH acceptance uses; the `Cluster.load_balancing_policy` extension point stays deferred) — the project's FIFTH LB policy and the SECOND consistent-hashing policy. `MaglevLbConfig` carries a SINGLE bounded field `table_size` (`UInt64Value`, default 65537 self-supplied; PGV `≤ 5000011`; NO `hash_function` — maglev always uses xxHash64) → a config-parse arm LIGHTER than ring_hash's (one field, no hash_function, no min≤max) but WITH a PRIMALITY gate. The `maglevLB` is a fixed-size Maglev lookup-table LB mirroring upstream v1.37.2's `MaglevTable` EXACTLY: endpoints sorted by `Addr()` (`"IP:PORT"`) ascending, each with `offset = xxHash64(addr) % M` + `skip = (xxHash64Seed(addr,1) % (M-1)) + 1`, populated via `(offset + skip·next) % M` until all M slots fill (terminates because M prime → each skip coprime to M → a full M-cycle); `Pick(hashKey, hasHash)` indexes `table[hashKey % M]` (no-hash → a uniform random table index). It is built ONCE at construction (immutable → `Pick` is lock-free) and holds NO per-pick state (the contrast with least_request's P2C). It REUSES the ADR-0235 LB hash-key seam UNCHANGED — implementing the existing unexported `loadBalancer` interface (`Pick(hashKey uint64, hasHash bool)`) and returning the shared `noopRelease` (the seam's FIRST hash-policy reuse, validating its "the durable asset maglev reuses unchanged" §Consequences claim — ZERO seam churn, ZERO seam ADR; the phase-35 single-ADR-on-reuse shape applied to a hash policy). The hash key is supplied by the LANDED ADR-0237 producers (tcp `source_ip` + HTTP route `hash_policy`) UNCHANGED — maglev is the FIRST policy to consume the producer WITHOUT changing it (ZERO producer churn, ZERO producer ADR); it swaps the Ketama RING for a Maglev TABLE behind the SAME seam. The table build reuses the existing `xxHash64`/`xxHash64Seed` digests in `hash.go` (ZERO new hash code). `cluster.Manager` accepts `MAGLEV` beside ROUND_ROBIN + LEAST_REQUEST + RANDOM + RING_HASH (the ONE deliberate byte-stable-reject text change — `manager.go:275`'s supported-list extends `…, RING_HASH` → `…, RING_HASH, MAGLEV`; blast radius three sites, the doubly-hit retarget MAGLEV → CLUSTER_PROVIDED — D-M5) and rejects a non-prime / over-cap `table_size` (reference PARITY — the reference itself rejects `The table size of maglev must be prime number` + the PGV `≤ 5000011`; both unit-level) and silently ignores a mismatched `lb_config` oneof member under MAGLEV (reference PARITY — the manager reads `GetMaglevLbConfig()` only on the MAGLEV path). Two new MIRRORED gauges `cluster.<name>.maglev_lb.{min,max}_entries_per_host` (Set once at table build via a `*maglevLB` type-assert — MAGLEV-only, reference parity; cross-side-exact `floor(M/N)`/`ceil(M/N)`, address-independent; NO `size` gauge — the table size is config-known) move the stat surface 1119 → 1121. The differential proof is the header-key NAT-transparent cross-side AFFINITY + SPREAD arm: fixture `0063-lb-maglev` — an HTTP route header `hash_policy` feeding MAGLEV, N=16 distinct `X-Hash` values × K=16 repeats, asserting the per-side modular invariant (each per-backend count `≡ 0 mod K` = affinity, BOTH sides) + spread (`≥ 2` backends) + conservation (256) + cross-side `/health` byte-equivalence + a cross-side `StatsAsserter` (the 2 gauges cross-equal + `upstream_rq_total` cross-equal); cross-side host IDENTITY is NOT asserted (the tables are over different address strings — the `0062` posture). NO new fuzzer (no wire decode — a table property test is unit-level) + NO new BackendKind (tail stays 33; the `0062` `HTTPEcho` reused) + ZERO new packages + ZERO new go.mod deps + ZERO new hash code. ADR-0024 (per-cluster counter scope) is UNAMENDED (maglev holds no per-cluster counter state). Healthy-host filtering happens upstream of the table lookup in the reference's shared base → with no health checking it degenerates to all-hosts (the Upstream-robustness family's boundary).

§Decision/§Consequences bodies land at the phase-37 IMPL per ADR-0044 (next-free after phase 37 ≈ **ADR-0239**). The PLAN/IMPL may surface additional ADRs (each re-checks — anticipated none; the seam + producers are reused).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — including the DECISIONS tail; they advance at the IMPL):

- stat surface **1119** (→ **1121** at the IMPL — +2 `maglev_lb.*` gauges, AMEND-M4).
- differential fixtures **64** (→ **65** at the IMPL: `0063-lb-maglev`; NO boot-reject dir — AMEND-M5).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0237** (STAYS ADR-0237 at this SPEC — the ADR-0238 §Context is a DRAFT in §13; the full ADR-0238 entry lands at the IMPL per ADR-0044; next-free **ADR-0238**).
- ROADMAP row 37 STAYS `in-progress` (it flips `→ done` at the phase-37 IMPL six-gate — a flat family row, NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (4 candidates remain after 37).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-37 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check).
