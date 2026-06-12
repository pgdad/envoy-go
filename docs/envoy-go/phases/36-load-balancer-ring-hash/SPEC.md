# Phase 36 SPEC — `ring_hash` load balancer (`Cluster.LbPolicy RING_HASH`): a Ketama consistent-hashing policy that EXTENDS the ADR-0232 LB seam (the PICK-INPUT half) via a ctx-carried hash key — the THIRD Load-balancing-family row; the family STAYS OPEN; the 36.1/36.2 by-plane split is CONSUMED

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3) for the **36.1** sub-phase (the seam extension + `ringHashLB` + manager + the tcp_proxy `source_ip` plane). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This SPEC covers the FULL phase-36 design (both planes) and makes the FINAL ADR-0045 split decision: **CONSUME the pre-authorized 36.1/36.2 by-plane split** (§3.0, D-RH7) — 36.1 lands first (its own PLAN→IMPL), then 36.2 (the HTTP route `hash_policy` plane; its own PLAN→IMPL). Phase 36 keeps the Load-balancing family OPEN; **5** candidates remain after 36 {maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds} (maglev the cheap follow-on reusing this seam).

**Goal:** Land `Cluster.LbPolicy RING_HASH` (`envoy.config.cluster.v3`, enum value 2) — the project's **fourth LB policy** and its **first consistent-hashing policy** — as a Ketama consistent-hash ring (`ringHashLB`) built once from the endpoint set, picking the first ring point `>= hashKey` (binary search, wrap), keyed by a request-derived `uint64` carried through `context.Context`. The seam is the ADR-0232 LB acquire/release seam **EXTENDED on its PICK-INPUT half** (the widening ADR-0232 §Consequences explicitly anticipated and deferred): the unexported `loadBalancer.Pick` widens to `Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)` (D-RH7); the three existing policies (roundRobin/leastRequest/randomLB) ignore the two new params behavior-neutrally; ringHashLB consumes them (no hash → random ring position, Envoy's documented fallback). The hash key is supplied by BOTH data planes: tcp_proxy `hash_policy: source_ip` (36.1) and the HTTP router route-level `hash_policy` (header + connection-properties source_ip; 36.2). The **exported `Cluster` surface stays BYTE-STABLE** — `Dial(ctx)`/`AcquireH1(ctx)`/`PickEndpoint()` keep their signatures (the OPTION-C discipline preserved); the ONE new exported symbol is the additive `cluster.WithHashKey(ctx, key)` helper. Two ADRs: **ADR-0235** (the seam extension) + **ADR-0236** (the ring_hash policy).

**Architecture:** A new `ringHashLB` type (`internal/cluster/ringhash.go`, same package) holds the sorted Ketama ring (`[]ringEntry{hash uint64; ep int}`), the endpoint slice, the parsed `hash_function`, and an injectable `rng func() uint64` for the no-hash fallback. The ring is built ONCE at construction: for each endpoint, `ceil(minRingSize/N)` (equal-weight; the running-sum formula for the general case) entries, each keyed `xxHash64("<addr>:<port>_<i>")` (the default `XX_HASH`; `MURMUR_HASH_2` the alternative), sorted ascending by hash. `Pick(hashKey, hasHash)` binary-searches for the first ring point with `ring[m-1].hash < hashKey <= ring[m].hash` (wrap to 0 past the end); `hasHash==false` → `hashKey = rng()` (a uniform ring position — Envoy's no-hash fallback). It REUSES the ADR-0232 RELEASE half UNCHANGED (returns the shared `noopRelease` — the ring is built-once, no per-pick state). A new **pure-Go `xxHash64`** (`internal/cluster/xxhash.go`, ~45 LoC — `cespare/xxhash` is absent from go.mod and a new dep is unwarranted for one call site; D-RH7) provides the seed-0 digest the cross-side ring reproduction needs. `Manager.buildCluster` gains a `case clusterv3.Cluster_RING_HASH` that parses `Cluster.RingHashLbConfig` (minimum/maximum ring size + hash_function) through a HAND-ROLLED gate mirroring BOTH the PGV `<= 8388608` bounds AND the runtime `minimum_ring_size > maximum_ring_size` reject (D-RH5), self-supplies the 1024/8388608 doc-comment defaults, and constructs `ringHashLB`; the `default` reject TEXT extends `… ROUND_ROBIN, LEAST_REQUEST, RANDOM` → `…, RANDOM, RING_HASH`. The hash key rides `ctx`: `cluster.go` gains an unexported `hashKeyCtxKey struct{}` + an exported `WithHashKey` + an unexported `hashKeyFrom`; `Dial`/`AcquireH1` extract it (2 lines each) and thread it to `c.lb.Pick(hk, ok)`. The two producers churn: tcp_proxy computes `xxHash64(downstream.RemoteAddr IP-only)` on new-conn (36.1); the HTTP router computes the route `hash_policy` key per-request (36.2). Three new per-cluster gauges (`ring_hash_lb.{size,min_hashes_per_host,max_hashes_per_host}`) mirror the reference (D-RH4).

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0f…`). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — `Cluster.LbPolicy RING_HASH` enum value 2 + `Cluster.RingHashLbConfig` + `type.v3.HashPolicy` + `RouteAction.HashPolicy` all already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` EMPTY — D-RH1). Reuses `internal/cluster/` (the 02/05.2/34/35 Manager + the ADR-0232 seam + the `newPCGRNG` RNG helper), the differential harness (`DistributionAsserter` + `StatsAsserter` + the `TCPEcho` BackendKind 0 + the existing HTTP backends), upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/ring_hash/` + `.../common/thread_aware_lb_impl.*` + `source/common/{network,http}/hash_policy.cc` + `source/common/common/hash.{h,cc}`) for the algorithm pins. ZERO new packages (a new `ringhash.go` + `xxhash.go` in `internal/cluster`; the two hash-compute sites in the existing `internal/filter/tcpproxy/` + `internal/filter/http/router/`).

**Authored:** 2026-06-11. **Empirical-pin probe date:** 2026-06-11.

---

## 1. Purpose / Mission

Phase 36 lands `ring_hash`, the **THIRD Load-balancing-family row** — the FIRST to EXTEND the ADR-0232 seam (phase 34 built it; phase 35 reused it unchanged). RING_HASH is the policy ADR-0232 §Consequences names by name: *"the hash-keyed RING_HASH/MAGLEV will additionally require widening `Pick` to receive a request-derived hash key (a signature extension this seam anticipates but does not provide at the MVP — the next LB phase that lands a hash policy must plan for it, NOT expect the zero-churn drop-in)."* Phase 36 IS that phase. It is also the project's fourth LB policy (after `roundRobin` [02], `leastRequest` [34], `randomLB` [35]); the `loadbalancer.go` doc comment ("Future phases that introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here") names it.

This SPEC refines the phase-36 BRAINSTORM (`docs/envoy-go/phases/36-load-balancer-ring-hash/BRAINSTORM.md`, Q0/Q1/Q-seam/Q-split) against the AS-BUILT `internal/cluster` package + the §11 D-RH1..D-RH8 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a RING_HASH affinity probe + a full `/stats` name-set diff + a 7-variant `--mode validate` matrix on a docker bridge network per `reference_docker_probe_bridge_network`), (2) go-control-plane `/envoy` v1.32.4 bindings + `.pb.validate.go`, and (3) upstream Envoy v1.37.2 source (the ring build, the hash compute, the multi-policy combine). It anchors the ADR-0235 + ADR-0236 §Context DRAFTS (§13) and makes the FINAL ADR-0045 split decision (§3.0).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-RH1..D-RH8 scrape CONFIRMED most BRAINSTORM anticipations but **REFUTED two** (the stat-delta and the cross-side assertion strength) and **refined** the PGV gate. The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-RH1 (D-RH1 — the v1.32.4 RING_HASH surface re-pinned; ZERO new dep; PGV has NO min≤max rule).** `Cluster_RING_HASH = 2` (`cluster.pb.go:121`). `Cluster.RingHashLbConfig` (`cluster.pb.go:2423`) carries `MinimumRingSize *wrapperspb.UInt64Value` (proto field 1), `HashFunction` (field 3 — `XX_HASH`=0 default / `MURMUR_HASH_2`=1), `MaximumRingSize *wrapperspb.UInt64Value` (field 4). **PGV** (`cluster.pb.validate.go:3181`) enforces ONLY two INDEPENDENT bounds — `MinimumRingSize` `value must be less than or equal to 8388608` and `MaximumRingSize` `value must be less than or equal to 8388608` (each skipped when its wrapper is nil) plus the `HashFunction` enum-membership check; **there is NO `min ≤ max` cross-field PGV rule** (the BRAINSTORM's "min ≤ max ≤ 8388608 PGV" was wrong on the cross-field part — the min>max reject is a RUNTIME check, D-RH5). The 1024/8388608 doc-comment defaults are NOT generated (the zero value is nil) — the manager self-supplies them. `ring_hash_lb_config` is `lb_config` oneof member 23 (`GetRingHashLbConfig()` nil-safe). `type.v3.HashPolicy` (tcp_proxy field 11; `SourceIp`/`FilterState` specifiers — NO cookie) + `RouteAction.HashPolicy` (route field 15; Header/Cookie/ConnectionProperties/QueryParameter/FilterState + `terminal`) both present. `go mod tidy -diff` EMPTY; `go build ./...` OK — **ZERO new go.mod dep.** See §5 / §11.1.
- **AMEND-RH2 (D-RH2 — the Ketama ring algorithm pinned from v1.37.2 source; byte-exact reproduction is FEASIBLE in isolation; xxHash64 seed 0 default).** Ring build (`ring_hash_lb.cc`): per-endpoint base key = `host->address()->asString()` = **`IP:port`** (no per-host `envoy.lb.hash_key` metadata, `use_hostname_for_hashing=false` → the defaults); per ring entry the key is `"<IP:port>_<i>"` (`_` literal + decimal index, no padding); the hash is **`HashUtil::xxHash64(key)` seed 0** for the default `XX_HASH` (`MurmurHash::murmurHash2(key, 0xc70f6907)` for `MURMUR_HASH_2`); entries per host via `scale = min(ceil(minNorm·minRing)/minNorm, maxRing)` driven by a running global sum (order-dependent ONLY under unequal weights). Lookup (`Ring::chooseHost`): upper-bound binary search for `ring[m-1].hash < h <= ring[m].hash`, **wrap to index 0** past the end. No-hash fallback (`thread_aware_lb_impl.cc`): `h = random_.random()` — "effectively the random LB." **Byte-exact reproduction is FEASIBLE** for the equal-weight / `XX_HASH` / no-metadata case (every ring input is data envoy-go has) — BUT see AMEND-RH8 for why the DIFFERENTIAL cross-side host-IDENTITY claim still does not follow. See §3.1 / §11.2.
- **AMEND-RH3 (D-RH3 — the per-plane hash COMPUTE pinned; source_ip is byte-identical across planes).** (tcp) `tcp_proxy hash_policy: source_ip` → `xxHash64(downstream.remoteAddress.ip().addressAsString())` seed 0 over the **bare client IP string, NO port** (`source/common/network/hash_policy.cc`). (HTTP) `RouteAction.hash_policy` header → `xxHash64(sorted header values)` seed 0 (single value → plain `xxHash64(value, 0)`); connection_properties source_ip → **identical compute to the tcp plane** (`xxHash64(bare IP)`). See §3.3 / §11.3.
- **AMEND-RH4 (D-RH4 — REFUTED: NOT zero stat delta; RING_HASH adds EXACTLY 3 `ring_hash_lb.*` gauges; surface 1116 → 1119; the 3 gauges are CROSS-SIDE-EXACT).** The full `/stats` name-set diff RING_HASH-vs-ROUND_ROBIN is NOT empty: RING_HASH introduces exactly THREE new gauge names — `cluster.<name>.ring_hash_lb.size`, `.min_hashes_per_host`, `.max_hashes_per_host` (live under default config / 3 equal hosts: 1026 / 342 / 342). Nothing else differs (the generic `lb_*` family is identical-and-present under both — never mirrored). **Decision: MIRROR the 3 gauges** (parity; surface 1116 → **1119** at the 36.1 IMPL) — they describe the ring envoy-go DOES build (unlike the un-mirrored `lb_*` health/zone/subset machinery), they are computable from the built ring at construction, and crucially they depend ONLY on `(minimum_ring_size, hash_function, host count, weights)` — NOT on endpoint ADDRESSES — so they are **cross-side-EXACT** (size 1026 / min 342 / max 342 on BOTH sides for a 3-equal-host RING_HASH cluster), giving the differential `StatsAsserter` a strong cross-equal prong. See §7 / §11.4.
- **AMEND-RH5 (D-RH5 — the accept/reject matrix pinned LIVE; the gate mirrors TWO validation layers; the lb-policy retarget RING_HASH → MAGLEV; NO boot-reject dir).** The 7-variant `--mode validate` matrix (§11.5): `RING_HASH` bare ACCEPT; full `ring_hash_lb_config` ACCEPT; **`minimum_ring_size: 5, maximum_ring_size: 2` (min>max) REJECT** with the RUNTIME message `ring hash: minimum_ring_size (5) > maximum_ring_size (2)`; `minimum_ring_size: 9000000` and `maximum_ring_size: 9000000` (> cap) REJECT with the PGV message `…RingHashLbConfigValidationError.{Minimum,Maximum}RingSize: value must be less than or equal to 8388608`; `MAGLEV` ACCEPT (envoy-go's continued rejection = recorded departure); a stray `ring_hash_lb_config` under `ROUND_ROBIN` ACCEPT silently. The HAND-ROLLED gate mirrors BOTH layers: the PGV `<= 8388608` wording on each bound AND the runtime `minimum_ring_size (N) > maximum_ring_size (M)` wording for min>max. **Blast radius of the lb-policy reject-text change** (full-repo grep): `internal/cluster/manager.go:257` (the only production string; extends `…, RANDOM` → `…, RANDOM, RING_HASH` + a new `case`) + `internal/cluster/manager_test.go:320-329` `TestManager_Error_UnsupportedLBPolicy` (DOUBLY hit — its trigger `Cluster_RING_HASH` becomes accepted → RETARGET to `Cluster_MAGLEV` + the substring `"… RANDOM, RING_HASH"`) + `docs/envoy-go/BEHAVIOR_CONTRACT.md:899/942`. **NO fixture pins the lb-policy text** → no cross-side lb-policy boot-reject dir. The RingHashLbConfig PGV/runtime rejects land UNIT-LEVEL in `manager_test.go` (the phase-34 `choice_count` precedent — config-parse rejects are unit-tested, not a fixture dir). Fixtures 62 → 64 cumulative (63 at the 36.1 IMPL [`0061`], 64 at 36.2 [`0062`]; §14 authoritative on staging; cross-side affinity only). See §6.
- **AMEND-RH6 (D-RH6 — the HTTP route `hash_policy` subset + the EXACT combine, 36.2 scope).** Land HEADER (`xxHash64` of the value) + connection-properties `source_ip` (the tcp-identical compute); DEFER cookie (incl. Set-Cookie generation/TTL) / query_parameter / filter_state. `terminal=true` short-circuits the combine ONCE a hash exists (`hash_impl->terminal() && hash`). The multi-policy COMBINE is **`hash = rotl64(prev, 1) ^ new_hash`** (rotate-left-1 then XOR, policy-list order; first contributor seeded from 0) — NOT a boost `hash_combine`, NOT a multiply (`source/common/http/hash_policy.cc:259-274`). See §3.3 / §11.6.
- **AMEND-RH7 (D-RH7 — the EXACT ctx-carry seam pinned; ~490 prod LoC / ~16–20 tasks; CONSUME the 36.1/36.2 split; xxHash64 hand-rolled).** The widened internal seam is **`Pick(hashKey uint64, hasHash bool)`** (the two-scalar form matching the project's `(value, ok)` idiom — chosen over `*uint64` [heap-escape/nil-aliasing] and a `pickOptions` struct [over-engineered for one optional uint64]); the three existing policies add `_ uint64, _ bool` ignored params (zero behavior change → all 62 fixtures byte-identical). The key rides `ctx` via an unexported `hashKeyCtxKey struct{}` + an EXPORTED additive `cluster.WithHashKey(ctx, key)` (the producers live in other packages) + an unexported `hashKeyFrom(ctx) (uint64, bool)`, all in `cluster.go` (the ADR-0219 `upstreamcluster.go` ctx-carry precedent). `Dial`/`AcquireH1` extract+thread in 2 changed lines each; their EXPORTED signatures are byte-stable; `PickEndpoint()` (no ctx) passes `(0, false)` → the no-hash fallback. Exactly TWO producer planes churn (tcp_proxy [1 site], the HTTP router [4 sites, one key-compute helper]); the other consumers (thriftproxy/redisproxy/grpcclient/httpclient/dial_h2 forwarder) thread `ctx` unchanged onto the no-hash path. **xxHash64 is ABSENT from go.mod** → a ~45-LoC pure-Go `xxHash64` (seed 0) is hand-rolled (needed for cross-side digest parity; a new dep for one call site is unwarranted). LoC envelope ~490 prod / ~16–20 tasks — UNDER the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`), so the split is DISCRETIONARY; **CONSUME it** on subsystem-coupling + fixture-asymmetry grounds (§3.0). ADR-0024 UNAMENDED (the ring is per-cluster LB state, the same scope discipline); NO `FuzzRingLookup` warranted (the ring decodes no untrusted wire bytes — a unit/property test suffices). See §3 / §11.7.
- **AMEND-RH8 (D-RH8 — REFUTED for cross-side host-identity: the harness forces PER-SIDE affinity+spread; the source_ip plane is subject-side-only; the HTTP header plane is the true cross-side proof).** A `net.Dialer{LocalAddr: 127.0.0.x}` multi-source-IP bind IS feasible on host loopback (proven — distinct source IPs observable; §11.8). BUT the reference proxy runs Docker-mapped (`host:MappedPort`), so Docker NATs EVERY client connection to ONE bridge-gateway source IP — a cross-side `source_ip` SPREAD fixture is INFEASIBLE (N keys on the subject, 1 key on the reference). AND even where keys survive, the two sides build the ring over DIFFERENT endpoint address strings (subject STATIC `127.0.0.1:port`; reference STRICT_DNS container-IP:port) → the ring layouts differ → cross-side host-IDENTITY (key X → the "same" backend index on both sides) does NOT hold, even though each side's ring is internally byte-exact (AMEND-RH2). **Therefore the differential asserts PER-SIDE deterministic affinity + spread + cross-side byte-equivalence + cross-side `StatsAsserter` (the 0059/0060 per-side precedent), NOT cross-side host identity.** 36.1's tcp `source_ip` plane uses a SUBJECT-SIDE affinity(exact)+spread(count) proof over `127.0.0.2..5`; 36.2's HTTP plane keys on a REQUEST HEADER (NAT-transparent → a true cross-side affinity/spread differential). The affinity leg is DETERMINISTIC/EXACT (same key → same backend, always) — NOT a σ-band — so the deliberate-breaks are sharp; only the coarse spread COUNT carries a loose threshold (`>= 2` distinct backends). See §8 / §11.8.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0234** (the random policy, ACCEPTED); next-free **ADR-0235**. Per the phase-36 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS tail **STAYS ADR-0234 at this SPEC** (counts UNCHANGED at the SPEC). The ADR-0235 (seam) + ADR-0236 (policy) §Context drafts are anchored as DRAFTS in §13; the full DECISIONS.md entries (§Context + §Decision + §Consequences) land at the 36.1 IMPL per ADR-0044. All eight D-RH pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **MAGLEV** (the second hash policy — `Cluster.LbPolicy MAGLEV` enum 5 + `Cluster.MaglevLbConfig.table_size`) — REUSES this phase-36 hash-input seam (ADR-0235) UNCHANGED, building only a different lookup table; the cheap follow-on family row. Stays byte-stable-rejected (the reference validate-accepts it — recorded departure, D-RH5).
- **The `Cluster.load_balancing_policy` extension point** — the typed-extension path (`load_balancing_policies.ring_hash.v3.RingHash`, richer knobs); shared with least_request's/random's deferred extension path. A future family row if/when needed.
- **The deferred HTTP hash-key sources (36.2)** — cookie (incl. Set-Cookie GENERATION with name/path/ttl/attributes), query_parameter, filter_state. The HEADER + connection-properties source_ip subset lands at 36.2 (D-RH6); the rest defer (they need cookie-generation callbacks / query-string parsing / filter-state plumbing — out of envelope).
- **Weighted-ring richness** — per-endpoint weight scaling beyond the equal-weight case; `use_hostname_for_hashing`; per-host `envoy.lb.hash_key` metadata; locality-weighted ring entries. The equal-weight case is the MVP (it also neutralizes the running-sum order dependence — AMEND-RH2).
- **`MURMUR_HASH_2` as the fixture path** — both hash functions are IMPLEMENTED (parse + ring build), but the differential fixtures use the `XX_HASH` default (the cross-side-reproducible path); a `MURMUR_HASH_2` arm is unit-level.
- **Cross-side host IDENTITY assertions** — INFEASIBLE in the harness (AMEND-RH8); the differential asserts per-side affinity+spread + cross-side byte-equivalence + cross-equal stats.
- **All other policies** — subset LB, locality-weighted LB, priority load balancing, panic thresholds — stay byte-stable-rejected; each a future family row.
- **Healthy-host filtering** — no active health checking (the Upstream-robustness family's territory); the ring is built over the full static endpoint set (Envoy's priority/healthy/panic machinery sits UPSTREAM of the ring pick — AMEND-RH2; with no health checking it degenerates to all-hosts).
- **lb-specific latency histograms** — deferred project-wide (ADR-0060). The 3 `ring_hash_lb.*` GAUGES ARE mirrored (AMEND-RH4 — they are gauges, not histograms).

---

## 3. The `ringHashLB` policy on the EXTENDED ADR-0232 seam (ADR-0235 + ADR-0236)

### 3.0 Split disposition — D-RH7 RESOLVED: **CONSUME the 36.1/36.2 by-plane split**

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-36 surface (§11.7 / AMEND-RH7):

| Unit | Anticipated production LoC | Sub-phase |
|---|---|---|
| Seam extension: `Pick(hashKey, hasHash)` interface widening + `hashKeyCtxKey`/`WithHashKey`/`hashKeyFrom` + `Dial`/`AcquireH1`/`PickEndpoint` threading + 3 incumbent policies' ignored params | ~55 | 36.1 |
| `ringHashLB`: Ketama ring build + binary-search lookup + no-hash fallback + `RingHashLbConfig` parse + the HAND-ROLLED two-layer gate + the manager `case` + the reject-text change + the 3 `ring_hash_lb.*` gauges | ~205 | 36.1 |
| pure-Go `xxHash64` (seed 0; new `xxhash.go`) | ~45 | 36.1 |
| tcp_proxy `source_ip` plane: `hash_policy` parse + `RemoteAddr` IP → `xxHash64` + `WithHashKey` stuff | ~60 | 36.1 |
| HTTP route `hash_policy` plane: route `hash_policy[]` parse + header/source_ip compute + the rotl-XOR combine + `terminal` + `WithHashKey` stuff | ~140 | 36.2 |
| Differential/test infra (the 0061 + 0062 fixtures + the multi-source-IP harness + ring-lookup unit/property tests) | test-side LoC, NOT counted | both |

Net production **~490 LoC / ~16–20 tasks** — BOTH ADR-0045 axes hold (under the ~1500-LoC / ~25-task gate), so the split is **DISCRETIONARY, not forced**. **DECISION: CONSUME it** (the FINAL ADR-0045 re-check), on three grounds (D-RH7.7):

1. **A clean two-subsystem coupling boundary.** 36.1 = {seam + `xxHash64` + `ringHashLB` + `RingHashLbConfig` parse + manager case + the 3 gauges + the tcp_proxy `source_ip` plane} delivers a COMPLETE, end-to-end-demonstrable consistent-hash LB on the tcp plane (the first proof). 36.2 = {the HTTP route `hash_policy` plane} adds a second producer against an ALREADY-LANDED seam. The cut is exactly the producer-plane boundary (D-RH7.5).
2. **The router `hash_policy` plane (~140 LoC) is the most intricate piece** (the rotl-XOR multi-policy combine + header-vs-source_ip specifiers + the terminal short-circuit) and benefits from landing on a PROVEN seam rather than co-developed with the seam churn.
3. **Fixture asymmetry forces distinct proof shapes (AMEND-RH8):** 36.1's tcp `source_ip` plane must use a SUBJECT-SIDE affinity+spread proof (Docker NAT defeats cross-side source_ip spread); 36.2's HTTP plane uses a NAT-transparent REQUEST-HEADER hash → a TRUE cross-side differential. Entangling two distinct fixture strategies in one flat row is worse than sequencing them.

**Consequence:** the IMMEDIATE next step after this SPEC is the **36.1 PLAN** (the seam + policy + tcp plane). 36.2 follows with its own PLAN→IMPL. ADR-0235 + ADR-0236 land at the 36.1 IMPL; 36.2 may add a small route-hash ADR or amend ADR-0236 (re-checked at the 36.2 SPEC/PLAN). ROADMAP row 36 keeps its pre-authorized `36.1, 36.2` split column; consuming it makes those the IMPL milestones (a flat family row — NO parent rollup per ADR-0106; each leg flips its own portion).

### 3.1 The `ringHashLB` policy (ADR-0236; Ketama consistent-hash ring)

`internal/cluster/ringhash.go` (NEW file, same package). The ring is built ONCE at construction and is immutable thereafter (no per-pick state → the shared `noopRelease`):

```go
// ringHashLB is a Ketama consistent-hash load balancer mirroring Envoy v1.37.2's
// RingHashLoadBalancer (SPEC §3.1 / AMEND-RH2): a ring of (hash,endpoint) points
// built once from the endpoint set; Pick binary-searches for the first ring point
// >= the request-derived hash key (wrap-around). It EXTENDS the ADR-0232 seam's
// PICK-INPUT half (it consumes hashKey) but REUSES the RELEASE half unchanged
// (returns the shared noopRelease — the ring is immutable, no per-pick state).
type ringHashLB struct {
	endpoints []Endpoint
	ring      []ringEntry  // sorted ascending by hash
	rng       func() uint64 // no-hash fallback (a uniform ring position); injectable for tests
	// gauges (AMEND-RH4): set once at build, mirrored from the reference.
	size, minPerHost, maxPerHost uint64
}

type ringEntry struct {
	hash uint64
	ep   int // index into endpoints
}
```

**The ring build** (`xxHash64` over `"<addr>:<port>_<i>"`, the default `XX_HASH`; AMEND-RH2):
- Per-endpoint entry count for the equal-weight MVP: `entriesPerHost = ceil(minRingSize / N)` (the running-sum `scale` formula collapses to this when all weights are equal; AMEND-RH2 — for the general weighted case the running-sum carry is order-dependent, DEFERRED §2). With the default `minimum_ring_size=1024` and N=3 → `ceil(1024/3)=342` entries each, ring size `3*342=1026` (matching the live reference: `ring_hash_lb.size=1026`, `min=max=342`).
- For endpoint `e` at index `j`, for `i` in `[0, entriesPerHost)`: `h = xxHash64(fmt.Sprintf("%s_%d", e.Addr(), i))` (the `XX_HASH` default; `murmurHash2(key, 0xc70f6907)` for `MURMUR_HASH_2`). `e.Addr()` is `"host:port"` (the as-built `Endpoint.Addr()` — matches Envoy's `address()->asString()` for IPv4/IPv6-bracketed forms).
- Append `ringEntry{h, j}`; after all endpoints, **sort ascending by `hash`** (ties broken deterministically — Envoy keeps insertion order on equal hashes, vanishingly rare for xxHash64; the SPEC pins a stable sort).
- The 3 gauges: `size = len(ring)`, `minPerHost`/`maxPerHost` = the min/max entry count across endpoints (equal-weight → both = `entriesPerHost`).

**The lookup** (`Pick`, the upper-bound binary search; AMEND-RH2):

```go
func (rh *ringHashLB) Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error) {
	if len(rh.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints // the roundRobin/leastRequest/random parity
	}
	if !hasHash {
		hashKey = rh.rng() // Envoy's no-hash fallback: a uniform ring position ("effectively random")
	}
	// First ring point with ring[m-1].hash < hashKey <= ring[m].hash; wrap to 0 past the end.
	m := sort.Search(len(rh.ring), func(i int) bool { return rh.ring[i].hash >= hashKey })
	if m == len(rh.ring) {
		m = 0 // wrap
	}
	return rh.endpoints[rh.ring[m].ep], noopRelease, nil
}
```

- `sort.Search` returns the first index with `ring[i].hash >= hashKey` — equivalent to Envoy's `ring[m-1].hash < h <= ring[m].hash` boundary (the `>=` matches Envoy's `h <= midval && h > midval1`). Past the largest hash → wrap to entry 0 (Envoy's `midp == size → midp = 0`).
- **No-hash fallback:** `hasHash==false` (the `PickEndpoint()` path + any consumer that supplies no key) → a uniform random ring position via the injectable `rng` (Envoy substitutes `random_.random()`; D-RH2.6). The RNG is the EXISTING `newPCGRNG()` helper (reused verbatim, like random; the injectable `newRingHashWithRNG` test seam mirrors the upstream mock posture).
- Empty-set → `Endpoint{}, noopRelease, errNoEndpoints` (the family parity; `buildCluster` already rejects zero-endpoint clusters).

### 3.2 The seam EXTENSION (the ADR-0232 PICK-INPUT half — ADR-0235)

The unexported `loadBalancer` interface widens (`loadbalancer.go`):

```go
type loadBalancer interface {
	// Pick selects an endpoint. hashKey carries a request-derived consistent-hash
	// key when hasHash is true (ring_hash); the non-hash policies ignore both args;
	// ring_hash with hasHash==false falls back to a random ring position. The
	// release func is the ADR-0232 RELEASE half (unchanged). ADR-0235 (the PICK-
	// INPUT-half extension; the hash key rides ctx, threaded in cluster.go).
	Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)
}
```

The three incumbent policies add two IGNORED params (behavior-neutral — all 62 fixtures byte-identical): `roundRobin.Pick(_ uint64, _ bool)`, `leastRequest.Pick(_ uint64, _ bool)`, `randomLB.Pick(_ uint64, _ bool)`. The ctx-carry plumbing (`cluster.go`):

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

`Dial`/`AcquireH1` change ONLY their `c.lb.Pick()` call (2 lines each): `hk, ok := hashKeyFrom(ctx)` then `ep, release, err := c.lb.Pick(hk, ok)`. Their EXPORTED signatures are UNCHANGED — byte-stable. `PickEndpoint()` (no ctx) passes `c.lb.Pick(0, false)` (no-hash) and keeps its exported signature. `DialH2` forwards through `c.Dial(ctx)` (zero churn). The OPTION-C discipline (the byte-stable exported `Cluster` surface) is preserved; only the two hash-producing consumers churn (§3.3). This is the seam's FIRST extension and the durable asset maglev reuses unchanged. ADR-0235 records it (the analogue of ADR-0232 for the PICK-INPUT half).

### 3.3 The two hash-input planes (the producer churn — D-RH7.5)

**tcp_proxy `source_ip` plane (36.1)** — `internal/filter/tcpproxy/filter.go`, the `Handle` path (the `eff.Dial(ctx)` call, filter.go:127). On a new downstream connection, if the (boot-parsed) `TcpProxy.hash_policy` carries a `source_ip` specifier: compute `key = xxHash64(ipOnly(downstream.RemoteAddr()))` (the bare client IP string, NO port — AMEND-RH3) and `ctx = cluster.WithHashKey(ctx, key)` before `eff.Dial(ctx)`. No hash_policy → no key → the cluster's LB sees `hasHash==false` (ring_hash → random fallback; the other policies unaffected).

**HTTP route `hash_policy` plane (36.2)** — `internal/filter/http/router/router.go` (the `AcquireH1(ctx)` [509] + `Dial(ctx)` [662] call sites) + `router_h2.go` (the `DialH2(ctx)` sites). Per request, if the matched route's `RouteAction.hash_policy[]` is non-empty: fold each policy's hash via `hash = rotl64(prev, 1) ^ new` (AMEND-RH6; first contributor seeded from 0; a `terminal=true` policy short-circuits once a hash exists), where each policy's `new` is `xxHash64(headerValue)` (header) or `xxHash64(ipOnly(remoteAddr))` (connection_properties source_ip — the tcp-identical compute); then `ctx = cluster.WithHashKey(ctx, hash)`. Cookie/query/filter_state specifiers are DEPARTURE-rejected at route parse (the reference parse-accepts them; silent acceptance would silently diverge — the thrift AMEND-T7 / least_request AMEND-L5 fail-fast lineage). A route with no `hash_policy` → no key (the existing behavior byte-stable).

### 3.4 Manager acceptance + the RingHashLbConfig gate (ADR-0236; D-RH1/D-RH5)

`manager.go buildCluster`: the `lb_policy` switch (line 235) gains one case; the construction parses the config through a hand-rolled two-layer gate:

```go
case clusterv3.Cluster_RING_HASH: // NEW (phase 36.1)
	cfg, err := parseRingHashLbConfig(c, name) // min/max ring size + hash_function; the two-layer gate
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

`parseRingHashLbConfig` (the `parseLeastRequestLbConfig` precedent; `GetRingHashLbConfig()` nil-safe → defaults on absent/mismatched-oneof):
- absent / mismatched oneof → `{min: 1024, max: 8388608, hash: XX_HASH}` (the self-supplied doc-comment defaults — AMEND-RH1).
- `minimum_ring_size` set & `> 8388608` → reject `value must be less than or equal to 8388608` (the PGV wording — AMEND-RH5).
- `maximum_ring_size` set & `> 8388608` → reject `value must be less than or equal to 8388608`.
- after defaulting unset bounds: `min > max` → reject `ring hash: minimum_ring_size (%d) > maximum_ring_size (%d)` (the RUNTIME wording — AMEND-RH5; a SECOND validation layer, distinct from PGV).
- `hash_function` ∈ {`XX_HASH`, `MURMUR_HASH_2`} accepted; any other enum value (none exist) → defensive reject.

The NEW reject text (`…, RING_HASH` appended) is the ONE deliberate byte-stable-reject change (§6; blast radius AMEND-RH5). A stray `ring_hash_lb_config` under a non-RING_HASH cluster is silently ignored (the manager reads `GetRingHashLbConfig()` only on the RING_HASH path — reference PARITY, §6.3).

---

## 4. Framework primitives — 1 seam EXTENSION + 0 new packages + 0 new go.mod deps (1 hand-rolled helper file)

Phase 36 EXTENDS the ADR-0232 LB acquire/release seam (the PICK-INPUT half — §3.2; ADR-0235). The RELEASE half is REUSED unchanged (ring_hash returns `noopRelease`). ZERO new packages (the `ringHashLB` + the seam widening + the manager case land in `internal/cluster`: new `ringhash.go` + `xxhash.go`; the two hash-compute sites in the existing consumer packages). ZERO new go.mod deps (AMEND-RH1 — the protos are in v1.32.4; `xxHash64` is hand-rolled, NOT a new dep — AMEND-RH7). ring_hash is not a filter — no builtins registration, no TypeURL factory, no bootstrap blank-import (the `clusterv3` + `type.v3` + route protos are already imported). The one NEW exported symbol is the additive `cluster.WithHashKey` (not a signature change to any existing method — the exported surface stays byte-stable).

---

## 5. Proto-field roster (per §11.1 D-RH1)

All from go-control-plane `/envoy` v1.32.4, verified in the module cache + `.pb.validate.go` this session.

### 5.1 `Cluster.LbPolicy` enum values at the guard

| Value | Enum | 36 disposition |
|---|---|---|
| 0 | `ROUND_ROBIN` (proto default; unset ≡ ROUND_ROBIN) | accepted (phase 02) |
| 1 | `LEAST_REQUEST` | accepted (phase 34) |
| **2** | **`RING_HASH`** | **accepted (THIS PHASE)** |
| 3 | `RANDOM` | accepted (phase 35) |
| 5 | `MAGLEV` | rejected (the second hash policy — reuses this seam; the reference accepts → departure) |
| 6 | `CLUSTER_PROVIDED` | rejected |
| 7 | `LOAD_BALANCING_POLICY_CONFIG` | rejected |

(Value 4 = the deleted `ORIGINAL_DST_LB`, absent from the generated enum.)

### 5.2 `Cluster.RingHashLbConfig` (the config-parse arm; AMEND-RH1)

`cluster.pb.go:2423`. Fields (note proto field numbers 1/3/4 — no field 2):

| Field | Go type | proto # | getter | gate |
|---|---|---|---|---|
| `MinimumRingSize` | `*wrapperspb.UInt64Value` | 1 | `GetMinimumRingSize()` (nil-safe) | unset → default 1024; set `> 8388608` → PGV reject; `> max` → runtime reject |
| `HashFunction` | `Cluster_RingHashLbConfig_HashFunction` | 3 | `GetHashFunction()` | `XX_HASH`(0,default) / `MURMUR_HASH_2`(1) |
| `MaximumRingSize` | `*wrapperspb.UInt64Value` | 4 | `GetMaximumRingSize()` (nil-safe) | unset → default 8388608; set `> 8388608` → PGV reject |

`ring_hash_lb_config` is `lb_config` oneof member 23 (wrapper `Cluster_RingHashLbConfig_`); `GetRingHashLbConfig()` nil-safe (silent-ignore parity on a mismatched-oneof or non-RING_HASH cluster).

### 5.3 The hash-input protos

- **tcp_proxy** — `TcpProxy.HashPolicy []*type.v3.HashPolicy` (field 11; `tcp_proxy.pb.go`). `type.v3.HashPolicy` specifiers: `SourceIp_` (an EMPTY message — presence = "hash on source IP") + `FilterState_` (deferred). NO cookie on the tcp variant.
- **HTTP route (36.2)** — `RouteAction.HashPolicy []*RouteAction_HashPolicy` (field 15; `route_components.pb.go`). Specifiers: `Header_` (`header_name string` + optional `regex_rewrite`), `ConnectionProperties_` (`source_ip bool`), `Cookie_`/`QueryParameter_`/`FilterState_` (deferred); `Terminal bool` (field 4).

---

## 6. PARSE-REJECT roster (per §11.5 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080: the reject-text changes are the deliberate contract-surface changes this phase. TWO families: (a) the lb-policy supported-list extension (`…, RANDOM` → `…, RANDOM, RING_HASH`) — ONE deliberate change, blast radius three sites (AMEND-RH5); (b) the RingHashLbConfig gate rejects — the hand-rolled wording mirrors the reference's TWO validation layers byte-for-byte (the PGV `value must be less than or equal to 8388608` + the runtime `ring hash: minimum_ring_size (N) > maximum_ring_size (M)`). All verified by table tests at the 36.1 IMPL.

### 6.2 Reject arms (UNIT-TESTED; NO cross-side boot-reject dir — AMEND-RH5)

- **`cluster-lb-policy-unsupported`** — the REPLACEMENT text for the still-rejected policies: `cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH)`. `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320`) RETARGETS its trigger from `Cluster_RING_HASH` (now accepted) to `Cluster_MAGLEV` and re-pins the new substring `"ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH"` (line 327). A DEPARTURE for MAGLEV (the reference accepts it). The phase-34/35 "doubly-hit" pattern recurs (the accept of policy N forces the reject test that used N as trigger to retarget to N+1).
- **`ring-hash-config-min-over-cap`** — `minimum_ring_size > 8388608` → `cluster: %q: ring_hash_lb_config.minimum_ring_size: value must be less than or equal to 8388608` (the PGV-parity wording; the manager calls no PGV — hand-rolled, the `choice_count` precedent).
- **`ring-hash-config-max-over-cap`** — `maximum_ring_size > 8388608` → `…maximum_ring_size: value must be less than or equal to 8388608`.
- **`ring-hash-config-min-over-max`** — `minimum_ring_size > maximum_ring_size` (post-default) → `cluster: %q: ring hash: minimum_ring_size (%d) > maximum_ring_size (%d)` (the RUNTIME-layer wording — distinct from PGV).

All four land UNIT-LEVEL in `manager_test.go` (the phase-34 config-reject precedent). **NO cross-side boot-reject dir** (no fixture pins these; the config-parse rejects are fully unit-coverable and the differential adds little — both sides reject identically; the phase-34/35 discipline).

### 6.3 NON-reject dispositions (parity)

- A stray `lb_config` oneof member (e.g. `least_request_lb_config`) under `lb_policy: RING_HASH`, or a stray `ring_hash_lb_config` under a non-RING_HASH cluster: **silent-ignore** (reference PARITY — §11.5 variant 13; behavior-inert — the manager reads `GetRingHashLbConfig()` only on the RING_HASH path).
- `lb_policy: RING_HASH` with NO `ring_hash_lb_config`: accepted, the defaults {1024, 8388608, XX_HASH} (reference parity — §11.5 variant 7).
- A non-default-but-valid config (e.g. `{min: 64, max: 128, hash: MURMUR_HASH_2}`): accepted (reference parity — §11.5 variant 8).

---

## 7. Stat surface — +3 `ring_hash_lb.*` gauges (per §11.4 D-RH4 + AMEND-RH4)

- **THREE new gauge names** (REFUTING the BRAINSTORM's anticipated zero delta): `cluster.<name>.ring_hash_lb.size`, `cluster.<name>.ring_hash_lb.min_hashes_per_host`, `cluster.<name>.ring_hash_lb.max_hashes_per_host`. Surface **1116 → 1119** at the 36.1 IMPL. Live-proven: the `/stats` name-set diff RING_HASH-vs-ROUND_ROBIN is exactly these three names (458 vs 455); nothing else differs (the generic `lb_*` family is identical-and-present under both — NOT mirrored).
- **MIRRORED (parity), set once at ring build.** They describe the ring envoy-go DOES build (unlike the un-mirrored `lb_*` health/zone/subset machinery), they are trivially computed from the built ring, and they are registered as per-cluster GAUGES via the existing `registerClusterMetrics` pattern (`membership_total`/`upstream_cx_active` are already cluster gauges). They have STATIC values (the ring is immutable) — `size = len(ring)`, `min/max = entry count extrema across endpoints`.
- **CROSS-SIDE-EXACT** — the three gauges depend ONLY on `(minimum_ring_size, hash_function, host count, weights)`, NOT on endpoint ADDRESSES → IDENTICAL on subject and reference for a given RING_HASH cluster (e.g. 1026 / 342 / 342 for a 3-equal-host default cluster). This makes them a STRONG cross-side `StatsAsserter` prong (unlike the per-side affinity distribution, which diverges by ring layout — AMEND-RH8).
- The `0061`/`0062` `StatsAsserter` set (D-RH4): cross-equal `cluster.<name>.ring_hash_lb.{size,min_hashes_per_host,max_hashes_per_host}` + `upstream_cx_total` + `membership_total` (=3) + quiesced `upstream_cx_active` (=0 post-drain); PER-SIDE `upstream_rq_total` (reference = conn count; subject = 0 on the tcp plane — the tcpproxy path never calls `IncUpstreamRqTotal`, the boundary 0059/0060 already pins; on the HTTP plane 36.2 the router DOES increment it → cross-equal there).

---

## 8. Differential fixture taxonomy (+2: 0061 tcp subject-side, 0062 HTTP cross-side)

Per `reference_differential_fixture_dispatch_constraint`: TWO cross-side dirs (no boot-reject dir — §6.2). Per `reference_differential_asserter_dispatch`: the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses the runner's `DistributionAsserter` hook (driver-side). Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0061'` (NOT `-run '0061'`, which matches zero subtests). Every assertion proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). The affinity leg is DETERMINISTIC/EXACT (same key → same backend always — NOT a σ-band; AMEND-RH8); only the coarse spread COUNT carries a loose threshold (per `reference_differential_band_sigma_margin`, the σ-margin discipline governs RNG-distributed bands — ring_hash affinity is not one).

### 8.1 `0061-lb-ring-hash` (36.1; tcp source_ip; SUBJECT-SIDE affinity+spread + cross-side stats/byte-equiv)

Chain `[tcp_proxy]` on BOTH sides over a 3-endpoint cluster with `lb_policy: RING_HASH` + `ring_hash_lb_config: {}` (defaults) + `hash_policy: [{source_ip: {}}]`. Backends: the plain `TCPEcho` BackendKind 0 (the 0001/0059/0060 backends REUSED — NO new BackendKind, tail STAYS 33).

**The workload (subject-side multi-source-IP; AMEND-RH8 / D-RH8):**
- The driver binds outgoing connections to source IPs `127.0.0.2, 127.0.0.3, 127.0.0.4, 127.0.0.5` (`net.Dialer{LocalAddr: …}` — proven feasible on host loopback, §11.8), 16 connections per source IP = 64 total (the 0059/0060 conservation target).
- **Affinity (SUBJECT-SIDE, EXACT):** for each source IP, all 16 of its connections land on EXACTLY ONE backend (the consistent-hash invariant — same key → same ring point → same backend; per-source-IP backend-set cardinality == 1). An EQUALITY assertion, NOT a band.
- **Spread (SUBJECT-SIDE, count):** the 4 source IPs collectively cover `>= 2` distinct backends (4 keys over 3 backends covering ≥2 is overwhelmingly likely; a coarse threshold, not a tight band).
- The REFERENCE side: Docker NATs all client conns to ONE gateway source IP → it pins to ONE backend (single-key affinity, the live D-RH4b observation) — so the reference is asserted on byte-equivalence + cross-side stats only, NOT on per-IP spread (AMEND-RH8). The subject-side affinity+spread runs via the `DistributionAsserter` hook on the SUBJECT path.

**The stats prong (cross-side `StatsAsserter`, post-drain):** §7's set — cross-equal `ring_hash_lb.{size=1026,min=342,max=342}` + `upstream_cx_total` + `membership_total=3` + `upstream_cx_active=0`; per-side `upstream_rq_total` (ref = conn count, subj = 0).

**Deliberate-break liveness (`-count=1`):** (i) scatter the key (make `Pick` ignore `hashKey` and draw random) → a source IP's 16 conns spread across backends → the per-source-IP cardinality-1 affinity leg FAILS; (ii) collapse the ring to one entry (all picks → `endpoints[0]`) → the spread leg (`>= 2` distinct) FAILS; (iii) drop a `StatsAsserter` Inc → the stats prong FAILS; (iv) corrupt a gauge value → the cross-equal `ring_hash_lb.*` prong FAILS. Recorded in driver comments + README.

### 8.2 `0062-lb-ring-hash-http` (36.2; HTTP route header hash; TRUE cross-side affinity+spread)

An HTTP listener routing to a 3-endpoint RING_HASH cluster with a route-level `hash_policy: [{header: {header_name: "x-hash"}}]`. The driver sends requests with distinct `X-Hash` values (the header survives the Docker NAT untouched → a SYMMETRIC cross-side proof — AMEND-RH8). Backends: existing HTTP backends (NO new BackendKind).

- **Affinity (BOTH sides, EXACT):** for each distinct `X-Hash` value, all K repeats hit ONE backend (per-value backend-set cardinality == 1) — asserted on BOTH subject and reference (the header value is NAT-transparent and each side's ring is internally deterministic).
- **Spread (BOTH sides, count):** N distinct header values collectively cover `>= 2` backends per side.
- **Cross-side byte-equivalence** of the HTTP responses + cross-side `StatsAsserter` (the §7 set; on the HTTP plane `upstream_rq_total` is cross-equal — the router increments it).
- NOTE: cross-side host IDENTITY (does `X-Hash: foo` hit the SAME backend index on both sides?) is NOT asserted — the two sides' rings are built over different endpoint address strings (AMEND-RH8). The live D-RH4b probe corroborated per-side determinism (alpha→BE1×3, delta→BE3×3, …) and spread.
- Deliberate-breaks: scatter (affinity fails both sides), collapse-ring (spread fails), drop-Inc (stats fails).

### 8.3 Total + no new BackendKind/fuzzer (family expectations)

Fixtures 62 → **64** (0061 at 36.1, 0062 at 36.2 — the split sequences them). BackendKind tail STAYS **33** (`TCPEcho` + existing HTTP backends reused — an LB phase exercises WHERE conns land). Fuzzers STAY **42** — ring_hash decodes no untrusted wire bytes (the hash key derives from a source IP / a parsed header value, not a wire frame); a ring-LOOKUP property test (random keys → always a valid endpoint, never panics, affinity holds) is UNIT-level, NOT a `Fuzz*` corpus entry (D-RH7 — a manufactured fuzzer over no decoder is vacuous, the phase-34/35 no-fuzzer precedent). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate(s). The REAL guard is the full differential re-verify: all prior dirs stay byte-exact through the seam extension + the new manager case (the three incumbent policies ignore the new params — behavior-neutrality is structural).

---

## 9. Behavior-contract delta (the 36 bundle; ADR-0052 atomic landing)

At the 36.1 IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains a NEW `### Load balancer — ring_hash (RING_HASH)` subsection (mirroring `### Load balancer — random (RANDOM)`):

- The RING_HASH acceptance (the `RingHashLbConfig` gate — min/max ring size defaults 1024/8388608, the two-layer reject [PGV `<= 8388608` + runtime min>max], `hash_function` XX_HASH/MURMUR_HASH_2); the Ketama ring semantics (build `xxHash64("addr:port_i")`, lookup first point `>= key` wrap, the no-hash random fallback — the upstream v1.37.2 mirror); the seam EXTENSION (the ctx-carried hash key; `WithHashKey` the new exported helper); the two planes (tcp `source_ip`, HTTP route `hash_policy` header + source_ip; cookie/query/filter_state DEPARTURE-rejected); the per-side affinity (deterministic) vs cross-side non-identity (different per-side ring layouts — the documented boundary); the 3 mirrored `ring_hash_lb.*` gauges (cross-side-exact); the healthy-set boundary (no health checking → all-hosts ring).
- The line-899 reject-text entry updates (RING_HASH retired from the rejected set → the FOURTH accepted policy; supported-list `…, RANDOM, RING_HASH`; MAGLEV stays the recorded departure).
- The line-942 deferred-LB-family entry updates (ring_hash retired; **5** candidates remain {maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}).
- Departure/coverage records: the mismatched-oneof silent-ignore (parity); cookie/query/filter_state DEPARTURE-reject; NO new fuzzer/BackendKind (family expectations); stat surface 1116 → 1119 (the 3 `ring_hash_lb.*` gauges — the FIRST non-zero LB-stat delta, mirrored); cross-side host-identity non-equivalence (AMEND-RH8).

(36.2 adds the HTTP `hash_policy` plane details to the same subsection at the 36.2 IMPL.)

---

## 10. Per-task structure (the PLAN decomposes; 36.1 first)

Indicative spine (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`). **36.1 is the immediate next PLAN; 36.2 follows with its own PLAN.**

### 36.1 (seam + policy + tcp plane; fixtures 0061; ADR-0235 + ADR-0236; surface → 1119)

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **62** (tail `0060`) + fuzzers **42** + stat surface **1116** + BackendKind tail **33** + DECISIONS tail **ADR-0234** via the canonical recipes; re-pin the as-built anchors (`loadbalancer.go` interface + `noopRelease`, `cluster.go` Pick funnel + `Dial`/`AcquireH1`/`PickEndpoint`, `manager.go:235` switch + `:257` reject text + `parseLeastRequestLbConfig`, `manager_test.go:320`, `newPCGRNG`, the `0059`/`0060` driver, the `tcpproxy` `Dial(ctx)` site); PROGRESS.md created | §11 / §3 |
| 2 | The pure-Go `xxHash64` (seed 0): TDD against published xxHash64 test vectors + a cross-check vs the live reference's observed key→host mapping (D-RH4b) | §3.1 / AMEND-RH7 |
| 3 | The seam extension: widen `loadBalancer.Pick(hashKey, hasHash)`; add the `_ uint64, _ bool` ignored params to roundRobin/leastRequest/randomLB (behavior-neutral); the `hashKeyCtxKey` + `WithHashKey` + `hashKeyFrom` helpers; thread through `Dial`/`AcquireH1`/`PickEndpoint` (exported sigs byte-stable). TDD: the 3 incumbent policies' pick sequences byte-unchanged | §3.2 / AMEND-RH7 |
| 4 | The `ringHashLB` type: the Ketama ring build (equal-weight `ceil(min/N)` entries, `xxHash64("addr:port_i")`, sorted) + the binary-search lookup (first `>= key`, wrap) + the no-hash `rng` fallback + the 3 `ring_hash_lb.*` gauge values. TDD with deterministic keys: same key → same endpoint; distinct keys spread; empty-set parity | §3.1 / AMEND-RH2 |
| 5 | Manager acceptance + the gate: the `case Cluster_RING_HASH` + `parseRingHashLbConfig` (defaults; the two-layer reject matrix) + the NEW reject text + `TestManager_Error_UnsupportedLBPolicy` retarget (RING_HASH → MAGLEV + substring) + the mismatched-oneof silent-ignore + register the 3 gauges. TDD: the §6 matrix, byte-stable table tests | §3.4 / §6 / §7 |
| 6 | The tcp_proxy `source_ip` plane: `TcpProxy.hash_policy` parse + `xxHash64(ipOnly(RemoteAddr))` + `cluster.WithHashKey` before `Dial(ctx)`. TDD: a source_ip → deterministic key; no hash_policy → no key | §3.3 |
| 7 | The `0061-lb-ring-hash` fixture: driver (multi-source-IP bind 127.0.0.2..5, 16/IP), the SUBJECT-SIDE affinity(cardinality-1)+spread(`>=2`) `DistributionAsserter`, the cross-side `StatsAsserter` prong (the 3 gauges cross-equal + cx/membership/quiesced + per-side rq) | §8.1 |
| 8 | Deliberate-break liveness (`-count=1`): scatter-the-key (affinity fails), collapse-the-ring (spread fails), drop-Inc (stats fails), corrupt-gauge (gauge prong fails); ≥20-run flake check | §8.1 |
| 9 | Full differential re-verify (the 62 prior dirs byte-exact through the seam extension + the new manager case + `0061` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected; Completion bundle: BEHAVIOR_CONTRACT 36 bundle (§9) + the ADR-0235 + ADR-0236 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0236) + STATE/ROADMAP row 36.1 (flat family row — NO parent rollup) + the six-gate evidence (surface 1119, fixtures 63) | §9 / §13 |

### 36.2 (HTTP route hash_policy plane; fixture 0062; its own PLAN→IMPL)

| # | Task | SPEC anchor |
|---|---|---|
| A | Re-confirm the 36.1 baseline (surface 1119, fixtures 63, DECISIONS tail ADR-0236) | §11 |
| B | The HTTP route `hash_policy` parse + compute: header (`xxHash64(value)`) + connection_properties source_ip (tcp-identical) + the `rotl64(prev,1)^new` combine + `terminal` short-circuit + cookie/query/filter_state DEPARTURE-reject; `cluster.WithHashKey` before `AcquireH1`/`Dial`/`DialH2` | §3.3 / AMEND-RH6 |
| C | The `0062-lb-ring-hash-http` fixture: distinct `X-Hash` values, cross-side affinity(cardinality-1, BOTH sides)+spread(`>=2`) + cross-side byte-equivalence + `StatsAsserter` | §8.2 |
| D | Deliberate-breaks + ≥20-run flake check; full differential re-verify (fixtures → 64) + six-gate; BEHAVIOR_CONTRACT 36.2 addendum + the optional route-hash ADR / ADR-0236 amend | §8.2 / §9 |

The PLAN re-checks the ADR-0045 gate per sub-phase (anticipated NO further split with margin).

---

## 11. SPEC-time empirical-pin block (D-RH1..D-RH8 — executed IN-SESSION 2026-06-11)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-11.** Reference corpus: (1) the live `envoyproxy/envoy:contrib-v1.37.2` docker image on a bridge network `rh36net` (`reference_docker_probe_bridge_network`) — a RING_HASH affinity probe (`downstream_cx_rx_bytes_total > 0` verified) + a full `/stats` name-set diff + a 7-variant `--mode validate` matrix; (2) go-control-plane `/envoy` v1.32.4 bindings + `config/cluster/v3/cluster.pb.validate.go`; `go mod tidy -diff` + `go build ./...` in the SPEC worktree; (3) upstream Envoy v1.37.2 source via raw.githubusercontent.com (`ring_hash_lb.{h,cc}`, `thread_aware_lb_impl.{h,cc}`, `common/network/hash_policy.cc`, `common/http/hash_policy.cc`, `common/common/hash.{h,cc}`); (4) the envoy-go codebase at master tip `1ec2a40`.

### Summary disposition table (8 pins)

| Pin | Topic | Disposition |
|---|---|---|
| §11.1 | D-RH1 (SPEC-BLOCKING) — proto/surface re-pin + tidy | **CONFIRMED w/ 1 REFUTATION** (enum 2; RingHashLbConfig min/max/hash_function; **NO min≤max PGV rule** — runtime-only; defaults self-supplied; ZERO new dep) |
| §11.2 | D-RH2 (SPEC-BLOCKING) — Ketama algorithm + byte-exact feasibility | **CONFIRMED** (xxHash64 seed 0 over `"addr:port_i"`; `ceil(min/N)` entries; first-`>=` wrap lookup; byte-exact reproducible in isolation) |
| §11.3 | D-RH3 — per-plane hash compute | **CONFIRMED** (tcp/HTTP-source_ip = `xxHash64(bare IP)`; HTTP header = `xxHash64(value)`) |
| §11.4 | D-RH4 — stat-surface delta | **REFUTED** (NOT zero — +3 `ring_hash_lb.*` gauges; surface → 1119; cross-side-exact; MIRROR decision) |
| §11.5 | D-RH5 (SPEC-BLOCKING) — accept/reject matrix + blast radius | **RESOLVED** (7-variant matrix; min>max runtime reject + over-cap PGV reject; retarget RING_HASH→MAGLEV; 3-site blast radius; no boot-reject dir) |
| §11.6 | D-RH6 (36.2) — HTTP hash_policy subset + combine | **RESOLVED** (header + conn-props source_ip land; cookie/query/filter_state defer; combine = `rotl64(prev,1)^new`; terminal short-circuit) |
| §11.7 | D-RH7 (SPEC-BLOCKING) — the seam surface + envelope + split | **RESOLVED** (`Pick(uint64,bool)`; ctx-carry via `WithHashKey`; ~490 LoC/~16–20 tasks; CONSUME the split; xxHash64 hand-rolled; ADR-0024 unamended; no FuzzRingLookup) |
| §11.8 | D-RH8 (SPEC-BLOCKING) — multi-source-IP harness + assertion strength | **RESOLVED w/ REFUTATION** (LocalAddr bind feasible; Docker NAT + per-side ring layouts → cross-side host-identity INFEASIBLE → per-side affinity+spread; header plane is the cross-side proof) |

### 11.1 D-RH1 (SPEC-BLOCKING) — the RING_HASH surface: CONFIRMED (1 refutation)

`Cluster_RING_HASH = 2` (`cluster.pb.go:121`). `Cluster.RingHashLbConfig` (`:2423`): `MinimumRingSize *wrapperspb.UInt64Value` (field 1), `HashFunction` (field 3: `XX_HASH`=0 default / `MURMUR_HASH_2`=1), `MaximumRingSize *wrapperspb.UInt64Value` (field 4). **PGV** (`cluster.pb.validate.go:3181`): `MinimumRingSize` and `MaximumRingSize` each `value must be less than or equal to 8388608` (skipped when nil) + the `HashFunction` enum check — **NO min≤max cross-field rule** (REFUTES the BRAINSTORM; the min>max reject is the runtime layer — §11.5). Defaults 1024/8388608 are doc-comment-only (not generated) → manager self-supplies. `ring_hash_lb_config` = `lb_config` oneof member 23 (`GetRingHashLbConfig()` nil-safe → silent-ignore parity). `type.v3.HashPolicy` (tcp field 11; `SourceIp`/`FilterState`) + `RouteAction.HashPolicy` (route field 15; Header/Cookie/ConnectionProperties/QueryParameter/FilterState + `terminal`) present. `go mod tidy -diff` EMPTY; `go build ./...` OK — **ZERO new go.mod dep.**

### 11.2 D-RH2 (SPEC-BLOCKING) — the Ketama ring algorithm: CONFIRMED

`ring_hash_lb.cc`: base key = `host->address()->asString()` = `IP:port` (no `envoy.lb.hash_key` metadata + `use_hostname_for_hashing=false` defaults); per-entry key = `"<IP:port>_<i>"` (`_` + decimal index); hash = `HashUtil::xxHash64(key)` seed 0 (`XX_HASH`) or `MurmurHash::murmurHash2(key, 0xc70f6907)` (`MURMUR_HASH_2`). Entries: `scale = min(ceil(minNorm·minRing)/minNorm, maxRing)` via a running global sum (order-dependent ONLY under unequal weights → equal-weight MVP = `ceil(minRing/N)` per host, order-independent). Lookup (`Ring::chooseHost`): upper-bound binary search `ring[m-1].hash < h <= ring[m].hash`, wrap to 0 past the end. No-hash fallback: `h = random_.random()` (`thread_aware_lb_impl.cc:182-198` — "effectively the random LB"). **Byte-exact reproduction FEASIBLE in isolation** (equal-weight, XX_HASH, no metadata) — every ring input is data envoy-go has. (But cross-side IDENTITY in the differential harness does not follow — §11.8.)

### 11.3 D-RH3 — the per-plane hash compute: CONFIRMED

(tcp) `source/common/network/hash_policy.cc` `SourceIpHashMethod`: `xxHash64(downstream_addr->ip()->addressAsString())` seed 0 — the bare IP string, **NO port** (distinct from `asString()`). (HTTP) `source/common/http/hash_policy.cc`: `HeaderHashMethod` → `xxHash64(sorted header values)` seed 0 (single value → plain `xxHash64(value, 0)`); `IpHashMethod` (connection_properties source_ip) → `xxHash64(addressAsString())` — **byte-identical to the tcp plane** (a source_ip key is the same uint64 on both planes).

### 11.4 D-RH4 — the stat-surface delta: REFUTED (+3 gauges)

Full `/stats` name-set diff RING_HASH (458 names) vs ROUND_ROBIN (455 names), bidirectional `comm`: RING_HASH-only = exactly `{cluster.<name>.ring_hash_lb.size, .min_hashes_per_host, .max_hashes_per_host}`; ROUND_ROBIN-only = NONE. Live values (default config, 3 equal hosts): `size=1026` (=3×342), `min_hashes_per_host=342`, `max_hashes_per_host=342`. The shared `lb_*` family identical-and-present under both (never mirrored). Cluster counters under the RING_HASH run (12 conns one source): `upstream_cx_total=12`, `upstream_rq_total=12`, `membership_total=3`, `upstream_cx_active=0`. **Decision: MIRROR the 3 gauges** (surface 1116 → 1119; cross-side-exact — they key only on ring-config + host count). LIVE AFFINITY (D-RH4b): tcp source_ip from one docker client → all 12 conns pinned to ONE backend (single-key affinity); HTTP header-hash → distinct keys SPREAD across all 3 backends with same-key STABILITY (alpha→BE1×3, delta→BE3×3, hotel→BE2×3) — corroborating the deterministic ring + the header plane's cross-side viability.

### 11.5 D-RH5 (SPEC-BLOCKING) — the validate matrix + the blast radius: RESOLVED

The 7-variant `--mode validate` table (contrib-v1.37.2):

| # | Variant | Verdict | Decisive fragment |
|---|---|---|---|
| 7 | RING_HASH, no ring_hash_lb_config | ACCEPT | `configuration … OK` |
| 8 | `{min 1024, max 8388608, XX_HASH}` | ACCEPT | `OK` |
| 9 | `{min 5, max 2}` (min>max) | **REJECT** | `ring hash: minimum_ring_size (5) > maximum_ring_size (2)` (RUNTIME layer) |
| 10 | `{minimum_ring_size 9000000}` | **REJECT** | `RingHashLbConfigValidationError.MinimumRingSize: value must be less than or equal to 8388608` (PGV layer) |
| 11 | `{maximum_ring_size 9000000}` | **REJECT** | `RingHashLbConfigValidationError.MaximumRingSize: value must be less than or equal to 8388608` (PGV layer) |
| 12 | `lb_policy: MAGLEV` | ACCEPT | `OK` (envoy-go's continued rejection = recorded departure) |
| 13 | ROUND_ROBIN + stray ring_hash_lb_config | ACCEPT, silent | `OK` |

**The gate mirrors TWO layers:** the PGV `value must be less than or equal to 8388608` on each over-cap bound AND the runtime `ring hash: minimum_ring_size (N) > maximum_ring_size (M)` for min>max. **Blast radius** of the lb-policy accept (full-repo grep): `internal/cluster/manager.go:257` (the only production reject string; + a new `case`) + `internal/cluster/manager_test.go:320-329` (`TestManager_Error_UnsupportedLBPolicy` trigger `Cluster_RING_HASH` → RETARGET to `Cluster_MAGLEV` + substring `"…, RANDOM, RING_HASH"`) + `docs/envoy-go/BEHAVIOR_CONTRACT.md:899` (the reject-text line) + `:942` (the deferred-family list). NO fixture pins the lb-policy text. The RingHashLbConfig rejects land UNIT-LEVEL (the phase-34 `choice_count` precedent) → **NO cross-side boot-reject dir** (fixtures 62 → 64 cumulative — 63 at the 36.1 IMPL, 64 at 36.2; §14 authoritative; affinity only).

### 11.6 D-RH6 (36.2) — the HTTP hash_policy subset + combine: RESOLVED

Supported specifiers (`hash_policy.cc:221-251`): header / cookie (incl. generation) / connection_properties (only if `source_ip`) / query_parameter / filter_state. **Land HEADER + connection-properties source_ip** (the two most self-contained); DEFER cookie/query/filter_state (DEPARTURE-reject, fail-fast — they alter the reference's pick). `terminal=true` short-circuits the combine ONCE a hash exists (`hash_impl->terminal() && hash`). **The combine** (`hash_policy.cc:259-274`): `hash = (prev ? rotl64(prev,1) : 0) ^ new` — rotate-left-1 then XOR, policy-list order; first contributor seeded from 0; nullopt policies skipped. NOT boost `hash_combine`, NOT a multiply.

### 11.7 D-RH7 (SPEC-BLOCKING) — the seam surface + envelope + split: RESOLVED

The widened internal interface is **`Pick(hashKey uint64, hasHash bool)`** (the `(value, ok)` idiom — the `UpstreamClusterOverride(ctx) (string, bool)` precedent; chosen over `*uint64` [heap-escape/nil-alias] and a `pickOptions` struct [over-engineered]). The 3 incumbent policies add `_ uint64, _ bool` (zero behavior change). The key rides `ctx` via unexported `hashKeyCtxKey struct{}` + EXPORTED additive `cluster.WithHashKey` + unexported `hashKeyFrom`, all in `cluster.go` (the ADR-0219 `upstreamcluster.go` ctx-carry precedent). `Dial`/`AcquireH1` extract+thread in 2 lines each (exported sigs byte-stable); `PickEndpoint()` passes `(0,false)`. **Call-site survey** (the "7 consumers, 2 churn" check): tcp_proxy (1 site) + the HTTP router (4 sites, one helper per plane) CHURN; thriftproxy/redisproxy/grpcclient/httpclient/dial_h2 thread ctx unchanged onto the no-hash path. **xxHash64 ABSENT from go.mod** → ~45-LoC pure-Go hand-roll (cross-side digest parity needs the real algorithm; a new dep for one call site is unwarranted). **LoC envelope** ~490 prod / ~16–20 tasks (table §3.0) — UNDER the ADR-0045 gate → the split is DISCRETIONARY; **CONSUME it** (subsystem-coupling + fixture-asymmetry — §3.0). ADR-0024 UNAMENDED (the ring is per-cluster LB state). NO `FuzzRingLookup` (no untrusted wire decode — a unit/property test suffices).

### 11.8 D-RH8 (SPEC-BLOCKING) — the multi-source-IP harness + assertion strength: RESOLVED (refutation)

A throwaway Go program (listen `127.0.0.1:0`; dial from `127.0.0.2..5` via `net.Dialer{LocalAddr}`) confirmed distinct source IPs ARE observable on host loopback (Linux 127.0.0.0/8 all-local; no `ip addr add`):
```
dial from 127.0.0.2 -> server observed RemoteAddr 127.0.0.2:57735
dial from 127.0.0.3 -> server observed RemoteAddr 127.0.0.3:45275
dial from 127.0.0.4 -> server observed RemoteAddr 127.0.0.4:43083
dial from 127.0.0.5 -> server observed RemoteAddr 127.0.0.5:58293
```
**BUT the bind only helps the SUBJECT.** The reference proxy is Docker-mapped (`host:MappedPort`) → Docker NATs every client conn to ONE bridge-gateway source IP → a cross-side `source_ip` SPREAD fixture is INFEASIBLE (N keys on subject, 1 on reference). AND the two sides build the ring over DIFFERENT endpoint address strings (subject STATIC `127.0.0.1:port`; reference STRICT_DNS container-IP:port) → the ring layouts differ → cross-side host-IDENTITY does NOT hold even though each side's ring is internally byte-exact. **→ The differential asserts PER-SIDE deterministic affinity + spread + cross-side byte-equivalence + cross-equal stats (the 0059/0060 precedent).** 36.1's tcp `source_ip` plane: a SUBJECT-SIDE affinity(cardinality-1, EXACT)+spread(`>=2`, count) proof over `127.0.0.2..5` (16/IP, 64 total); the reference asserted on byte-equiv + cross stats only. 36.2's HTTP plane: a NAT-transparent REQUEST-HEADER hash → a TRUE cross-side affinity+spread proof on BOTH sides. The affinity leg is DETERMINISTIC/EXACT (no σ-band); the spread count is a coarse `>= 2` threshold (`reference_differential_band_sigma_margin` governs RNG bands, which the affinity leg is not).

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S36-1** — the file placement inside `internal/cluster` (anticipated: `ringHashLB` + the ring build in a NEW `ringhash.go`; `xxHash64` in a NEW `xxhash.go`; the seam widening in `loadbalancer.go` + `cluster.go`; the manager case + gate + reject text in `manager.go`; the retarget in `manager_test.go` — the `random.go`/`leastrequest.go` precedent).
- **D-S36-2** — whether `newRingHash` reuses `newPCGRNG` verbatim for the no-hash fallback (the shared seed-error message `"cluster: least_request: seed rng"`) or wraps it (anticipated: call directly, accept the shared message — the random.go disposition; the path is reachable only on a crypto/rand failure).
- **D-S36-3** — the exact `xxHash64` implementation shape (a direct port of the XXH64 reference; unit-tested against published vectors AND the live reference's observed key→host mapping from D-RH4b) + whether `MurmurHash2` (the `MURMUR_HASH_2` arm) is implemented at 36.1 or deferred to a follow-up (anticipated: implement both at 36.1 — the parse arm accepts both; the fixtures use XX_HASH).
- **D-S36-4** — the multi-source-IP driver shape (does the existing differential client expose a `LocalAddr` hook, or does 0061 need a new driver primitive? — §11.8 proves the technique; the PLAN pins the harness extension) + the per-source-IP burst sizing + the ≥20-run flake protocol.
- **D-S36-5** — the equal-weight ring-build formula vs the general running-sum formula (anticipated: implement the equal-weight `ceil(min/N)` path; the weighted running-sum carry is DEFERRED §2 — but confirm the build matches the live `size=1026/min=max=342` for the default 3-host case).
- **D-S36-6** — whether the 3 `ring_hash_lb.*` gauges are registered unconditionally for every cluster (zero for non-ring_hash) or only for RING_HASH clusters (anticipated: only on the RING_HASH path — they are ring-specific; matching the reference, which emits them only under RING_HASH).
- **D-S36-7 (36.2)** — the route `hash_policy` parse location (the router's route-build path) + whether connection_properties source_ip reuses the 36.1 tcp compute helper verbatim (anticipated: shared `xxHash64(ipOnly(addr))` helper).
- ADR-0045 split-gate per-sub-phase re-check at each PLAN.

---

## 13. ADR continuity — the ADR-0235 + ADR-0236 §Context DRAFTS (anchored here; full entries at the 36.1 IMPL)

Per the phase-36 routing, the DECISIONS.md tail **STAYS ADR-0234 at this SPEC**. The §Context drafts are anchored HERE; the full ADR-0235 + ADR-0236 entries (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) land at the 36.1 IMPL per ADR-0044 (tail ADR-0234 → ADR-0236).

**ADR-0235 §Context DRAFT (the LB hash-key seam extension — the ADR-0232 PICK-INPUT half):** Phase 36 EXTENDS the ADR-0232 LB acquire/release seam on its PICK-INPUT half — the widening ADR-0232 §Consequences explicitly anticipated and deferred (*"the hash-keyed RING_HASH/MAGLEV will additionally require widening `Pick` to receive a request-derived hash key … the next LB phase that lands a hash policy must plan for it"*). The unexported `loadBalancer.Pick` widens from `Pick() (Endpoint, func(), error)` to `Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)` — the two-scalar `(value, ok)` form (chosen over `*uint64` and a `pickOptions` struct). The hash key rides `context.Context`: an unexported `hashKeyCtxKey struct{}` + an EXPORTED additive `cluster.WithHashKey(ctx, key)` (the producers — tcp_proxy, the HTTP router — live in other packages) + an unexported `hashKeyFrom(ctx) (uint64, bool)`, all in `cluster.go` (the ADR-0219 ctx-carry precedent). `Dial`/`AcquireH1` extract the key and thread it to `c.lb.Pick(hk, ok)` in 2 changed lines each; their EXPORTED signatures — and `PickEndpoint()` (which passes `(0,false)` → the no-hash fallback) — stay BYTE-STABLE (the OPTION-C discipline preserved). The three non-hash policies (roundRobin/leastRequest/randomLB) add two IGNORED params (behavior-neutral — all 62 fixtures byte-identical). Exactly two producer planes churn (tcp_proxy, the HTTP router); the other consumers (thriftproxy/redisproxy/grpcclient/httpclient/dial_h2) thread ctx unchanged onto the no-hash path. The RELEASE half is REUSED unchanged (ring_hash returns `noopRelease` — the ring is immutable, no per-pick state). This is the seam's FIRST extension and the durable asset maglev (and any future hash policy) reuses unchanged — the analogue of ADR-0232 (the RELEASE-half seam) for the PICK-INPUT half. ADR-0024 (per-cluster RR counter scope) is UNAMENDED.

**ADR-0236 §Context DRAFT (the `ring_hash` load-balancing policy):** Phase 36 lands `Cluster.LbPolicy RING_HASH` (`envoy.config.cluster.v3`, enum value 2) on the LEGACY enum path (the `Cluster.load_balancing_policy` extension point stays deferred) — the project's FOURTH LB policy and its FIRST consistent-hashing policy; the THIRD Load-balancing-family row. The `ringHashLB` is a Ketama consistent-hash ring mirroring upstream v1.37.2's `RingHashLoadBalancer` EXACTLY: a ring of (hash, endpoint) points built ONCE from the endpoint set — per ring entry `xxHash64("<addr>:<port>_<i>")` (the default `XX_HASH`; `murmurHash2(key, 0xc70f6907)` for `MURMUR_HASH_2`), `ceil(minimum_ring_size/N)` entries per endpoint for the equal-weight MVP, sorted ascending; `Pick(hashKey)` binary-searches for the first ring point `>= hashKey` (wrap), with `hasHash==false` → a uniform random ring position (Envoy's documented no-hash fallback). A pure-Go `xxHash64` (seed 0) is hand-rolled (`cespare/xxhash` is absent from go.mod; a new dep for one call site is unwarranted; the digest must match the reference for the ring to be a faithful reproduction). `cluster.Manager` accepts `RING_HASH` beside ROUND_ROBIN/LEAST_REQUEST/RANDOM (the ONE deliberate byte-stable-reject text change — `manager.go:257`'s supported-list extends `…, RANDOM` → `…, RANDOM, RING_HASH`; blast radius three sites, no fixture pins it — D-RH5), parses `Cluster.RingHashLbConfig` (minimum/maximum ring size defaults 1024/8388608 self-supplied + `hash_function`) through a HAND-ROLLED gate mirroring the reference's TWO validation layers (the PGV `value must be less than or equal to 8388608` on each over-cap bound + the runtime `ring hash: minimum_ring_size (N) > maximum_ring_size (M)` for min>max — D-RH5), and silently ignores a mismatched `lb_config` oneof member (reference PARITY). The hash key is supplied by BOTH data planes (the pre-authorized 36.1/36.2 split, CONSUMED — D-RH7): tcp_proxy `hash_policy: source_ip` (`xxHash64(bare client IP)`; 36.1) and the HTTP router route-level `hash_policy` (header `xxHash64(value)` + connection-properties source_ip, combined `rotl64(prev,1)^new` with a terminal short-circuit; cookie/query/filter_state DEPARTURE-rejected; 36.2). The differential proof is the AFFINITY + SPREAD arm: `0061-lb-ring-hash` (tcp source_ip; 36.1) — a SUBJECT-SIDE multi-`127.0.0.x` affinity(deterministic, cardinality-1)+spread(`>=2`) proof + cross-side byte-equivalence + a cross-side `StatsAsserter`; `0062-lb-ring-hash-http` (HTTP header; 36.2) — a NAT-transparent TRUE cross-side affinity+spread proof. Cross-side host IDENTITY is INFEASIBLE in the harness (Docker source-IP NAT + per-side ring layouts over different endpoint address strings) → per-side affinity (the 0059/0060 precedent). Stat surface 1116 → 1119: THREE new MIRRORED gauges `cluster.<name>.ring_hash_lb.{size,min_hashes_per_host,max_hashes_per_host}` (the FIRST non-zero LB-stat delta — they describe the ring envoy-go builds; cross-side-exact, keyed only on ring-config + host count). NO new fuzzer (no wire decode — a ring-lookup property test is unit-level) + NO new BackendKind (tail stays 33) + ZERO new packages + ZERO new go.mod deps.

§Decision/§Consequences bodies land at the 36.1 IMPL per ADR-0044 (next-free after phase 36.1 ≈ **ADR-0237**). The 36.2 SPEC/PLAN may surface a small route-hash ADR or amend ADR-0236 (re-checks).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — they advance at the IMPLs):

- stat surface **1116** (→ **1119** at the 36.1 IMPL — +3 `ring_hash_lb.*` gauges, AMEND-RH4; the FIRST non-zero LB-stat delta).
- differential fixtures **62** (→ **63** at the 36.1 IMPL [`0061-lb-ring-hash`] → **64** at the 36.2 IMPL [`0062-lb-ring-hash-http`]; NO boot-reject dir — AMEND-RH5).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0234** (STAYS at this SPEC — the ADR-0235 + ADR-0236 §Context are DRAFTS in §13; the full entries land at the 36.1 IMPL per ADR-0044; next-free **ADR-0235**).
- ROADMAP row 36 STAYS `in-progress` with its pre-authorized `36.1, 36.2` split column (the split is now CONSUMED — §3.0); it flips per-leg at the 36.1 then 36.2 IMPL six-gate(s) (a flat family row, NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (5 candidates remain after 36).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **36.1 PLAN** (`superpowers:writing-plans` — decompose §10's 36.1 spine into bite-sized TDD tasks; the ADR-0045 gate re-check per sub-phase). 36.2 follows with its own PLAN.
