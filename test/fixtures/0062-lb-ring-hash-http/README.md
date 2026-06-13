# 0062-lb-ring-hash-http

Cross-side `[http_connection_manager + router]` differential over ONE 3-endpoint
cluster with `lb_policy: RING_HASH` + `ring_hash_lb_config: {}` (defaults) + a
**route-level** `hash_policy: [{header: {header_name: "x-hash"}}]` on BOTH sides
(the 0003 HTTP shape: reference STRICT_DNS / `host.docker.internal`, subject
STATIC / `127.0.0.1`). This is the **end-to-end proof of Tasks 2-4** — the HTTP
route `hash_policy` producer → `cluster.HashHeaderValues` digest → `applyHashKey`
→ `cluster.WithHashKey` → `ringHashLB` pick → per-header-value affinity — that
envoy-go's Ketama consistent-hash load balancer threads a header-derived key into
the SAME ring with the SAME affinity shape and moves the SAME cluster stats
(cross-equal `ring_hash_lb.*` gauges) as the reference Envoy `contrib-v1.37.2`.

It is the **HTTP sibling of `0061-lb-ring-hash`** (the 36.1 tcp `source_ip`
plane). The crucial difference is the KEY's NAT behavior:

- `0061`'s `source_ip` key is **rewritten by Docker NAT**, so the reference
  collapsed all connections to a single key → one backend (cross-side host
  identity infeasible — subject-side affinity only).
- `0062`'s **`X-Hash` HEADER is NAT-transparent** — it survives the Docker hop
  verbatim — so the consistent-hash affinity invariant holds on **BOTH** the
  envoy-go subject AND the reference Envoy. This is a **TRUE cross-side affinity
  proof** (D-S362-6).

Phase 36.2 SPEC §10 (the HTTP route `hash_policy` plane) / §7 (the cross-side
stats set incl `upstream_rq_total`) / 36.2 PLAN Task 5.

## The workload (N=16 distinct X-Hash values × K=16; D-S362-6)

For each of **N=16** distinct `X-Hash` values (`hv-0` … `hv-15`) the driver sends
**K=16** `GET /get` requests carrying that value in the `X-Hash` header. **16 × 16
= 256** routed requests (`totalReqs`, DERIVED from `hashValues * repeatPerVal` —
the cx/rq stat expectations track the constants, never a literal). Each request is a
fresh dial (`HTTPRoundTrip` sets `Connection: close`), so each routed request is
one upstream connection → the `HTTPEcho` backend's accept counter increments once
per request, yielding per-backend request counts directly.

N was raised 4 → 16 in the Task-6 follow-up fix (the spread-prong flake below).

After the routed phase the driver sends **8 `GET /health`** round-trips. `/health`
is a `direct_response` (`inline_string: "OK\n"`) served by the listener — it does
NOT touch the backend (no accept, no `upstream_cx`), and its body is **address-
independent** → byte-equal across both proxies. That `OK\n`×8 stream is the
runner's `CompareBytes` input (the 0003 byte-equiv precedent).

### Why K=16 and N=16

K is the **modular base** of the affinity invariant: each distinct value is
internally deterministic (it hashes to ONE ring point → ONE backend) and is sent
exactly K times, so every backend's aggregate count is a sum of whole K-blocks →
`≡ 0 mod K`. K=16 (matching 0061's `burstPerIP`) makes a per-request scatter break
statistically certain to produce a non-multiple-of-16 backend total. Smaller K
weakens the discriminating power; larger K only lengthens the run.

N is the **spread robustness** knob. The Ketama ring is built over the endpoint
ADDRESS strings (`<addr>:<port>_i`) and the harness allocates the 3 backend ports
DYNAMICALLY per run, so the ring layout varies run-to-run. With N distinct values
over 3 backends, the per-side probability that ALL N values collapse onto a SINGLE
backend (failing spread) is `≈ 3·(1/3)^N`. The original **N=4** gave
`3·(1/3)^4 ≈ 3.7%` per side and flaked the spread prong (18/20 — see below). Raising
to **N=16** drops it to `3·(1/3)^16 ≈ 7e-8` per side — past the 5σ-equivalent
flake-free margin (`reference_differential_band_sigma_margin`, applied to a spread
threshold rather than a σ-band). Empirically 30/30 PASS at N=16.

### Why the routed bodies are NOT byte-compared

The `HTTPEcho` backend embeds its own idx in the body (`backend-<idx>:<seg>`). The
ref and subj rings are built over DIFFERENT endpoint address strings (STRICT_DNS
`host.docker.internal:<port>` vs STATIC `127.0.0.1:<port>`), so a given `X-Hash`
value may land on a DIFFERENT backend idx per side — the per-request routed bytes
diverge cross-side. Cross-side host **identity** is therefore **INFEASIBLE**
(`reference_differential_hash_key_cross_side_infeasible`); the routed bytes are
not concatenated. Affinity is proven by the aggregate-count modular invariant,
which needs no host attribution.

## The affinity+spread arm (`AssertDistribution`) — the D-S362-6 modular invariant

`AssertDistribution` is **DETERMINISTIC / EXACT** — NOT a σ-band
(`reference_differential_band_sigma_margin` governs RNG-distributed bands; ring_hash
affinity is not one). The runner snapshots `backend.accepts` after Drive (each
accept on the `HTTPEcho` backend counts one routed request; the 256 (`totalReqs`)
requests sum across the three backends). The invariant is checked on **BOTH** sides
(the NAT-transparent header is the whole point):

- **affinity:** each per-backend count `cᵢ ≡ 0 mod 16` — VIOLATED by a key-scatter
  break (a value splitting across backends produces a count NOT a multiple of 16);
- **spread:** `>= 2` backends nonzero — VIOLATED by a collapsed ring (all 256 on one
  backend);
- **conservation:** `c1 + c2 + c3 == 256` (`totalReqs`; catches drop / double-count).

## The stats prong (`StatsAsserter`, post-drive) — SPEC §7

The §7 cross-side set, EXTENDED with the **3 `ring_hash_lb.*` gauges** (shared with
0061) AND the **cross-equal `upstream_rq_total`** (the NEW prong vs 0061 — the HTTP
plane Inc's `upstream_rq_total` on BOTH sides, where 0061's `tcp_proxy` charged it
per-side). Observed (every run, both sides):

| stat                                              | reference | subject | disposition                     |
|---------------------------------------------------|-----------|---------|---------------------------------|
| `cluster.c_echo.upstream_cx_total`                | 256       | 256     | **cross-equal** == 256 (=`totalReqs`) |
| `cluster.c_echo.upstream_rq_total`                | 256       | 256     | **cross-equal** == 256 (NEW)    |
| `cluster.c_echo.membership_total`                 | 3         | 3       | **cross-equal** == 3            |
| `cluster.c_echo.upstream_cx_active`               | 0         | 0       | **cross-equal** == 0 (quiesced) |
| `cluster.c_echo.ring_hash_lb.size`                | 1026      | 1026    | **cross-equal** == 1026         |
| `cluster.c_echo.ring_hash_lb.min_hashes_per_host` | 342       | 342     | **cross-equal** == 342          |
| `cluster.c_echo.ring_hash_lb.max_hashes_per_host` | 342       | 342     | **cross-equal** == 342          |

The **3 `ring_hash_lb.*` gauges are cross-equal** because they depend ONLY on
`(minimum_ring_size, hash_function, host count, weights)`, NOT on endpoint
ADDRESSES — so they are IDENTICAL on subject and reference for a 3-equal-host
default RING_HASH cluster (`size = 1026`, `min = max = 342`). With the default
`minimum_ring_size = 1024` and 3 equal-weight hosts, the Ketama build rounds up to
the smallest size that gives each host its fair share: `342 × 3 = 1026 >= 1024`.
These values are inherited verbatim from 0061 (the ring is identical — same
config, same host count).

The `upstream_cx_total == upstream_rq_total == 256` cross-equality also proves the
decode actually ran on BOTH sides (non-zero upstream traffic — the
`reference_docker_probe_bridge_network` "decode ran" signal): had the reference
container failed to reach the backends, `upstream_cx_total` would be 0 and the
prong would bite.

## Deliberate-break liveness (Task 6 — `-count=1`)

Each prong was applied ONE AT A TIME to PRODUCTION code (or, for prong 4, the
driver's expected gauge), run with `-count=1` to defeat go-test caching
(`reference_differential_break_protocol_count1`), the named prong observed to
`--- FAIL`, then `git restore`d (production tree left UNCHANGED — only this README
+ PROGRESS are committed). Selector: `-run 'TestDifferential/0062-lb-ring-hash-http'`
(`reference_differential_run_selector`).

| # | break (file · edit)                                                                 | proves prong               | observed `--- FAIL` line |
|---|-------------------------------------------------------------------------------------|----------------------------|--------------------------|
| 1 | `internal/filter/http/router/router.go` — `applyHashKey` returns `ctx,0,false` unconditionally (key never contributes) | **affinity** (modular invariant `cᵢ ≡ 0 mod 16`) | `distribution: subject affinity: backend[0]=90 not a multiple of 16 (key scattered? an X-Hash value split across backends)` |
| 2 | `internal/cluster/ringhash.go` — `ringHashLB.Pick` returns `endpoints[0]` always (ring collapsed to backend 0) | **spread** (`>= 2` nonzero) | `distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` |
| 3 | `internal/filter/http/router/router.go` — drop `a.cluster.IncUpstreamRqTotal()` on the H1 dial site (`doH1ClusterAction`) | **`upstream_rq_total` cross-equal** | `cross-side mismatch cluster.c_echo.upstream_rq_total: ref=256 subj=0` · `subj cluster.c_echo.upstream_rq_total = 0, want 256` |
| 4 | `test/fixtures/0062-lb-ring-hash-http/driver/driver.go` — expected `ring_hash_lb.size` 1026 → 1025 | **`ring_hash_lb.*` gauge cross-equal** | `ref cluster.c_echo.ring_hash_lb.size = 1026, want 1025` · `subj cluster.c_echo.ring_hash_lb.size = 1026, want 1025` |

All four prongs bit the SPECIFIC named assertion. The `--- FAIL` lines above are at
the post-fix **N=16** workload: prongs 1 & 2 (affinity, spread) were **re-confirmed
to BITE at N=16** in the follow-up fix (`-count=1`); the affinity count rose
`backend[0]=25` → `backend[0]=90` (more requests scatter through the disabled key),
and the rq-break counts rose `64` → `256` (=`totalReqs`) with N=16. After every
break: `git status` clean, `git diff --stat -- internal/ test/.../driver/` EMPTY,
still on branch `phase-36.2-load-balancer-ring-hash-http-impl`.

### Flake check — 30/30 PASS at N=16 (the spread-prong flake is FIXED)

History: at the original **N=4** the spread prong flaked **18/20** —
`for i in $(seq 1 20); …` produced 2 FLAKEs (run 14 `reference spread: only 1
backend(s) nonzero`; run 15 `subject spread: only 1 backend(s) nonzero`), BOTH the
**spread** prong (affinity + conservation + all stats prongs held 20/20). Root
cause: the Ketama ring is built over the endpoint ADDRESS strings (`<addr>:<port>_i`)
and the harness allocates the 3 backend ports **dynamically per run**, so the ring
layout varies run-to-run; with only N=4 values over 3 backends the four occasionally
all land on ONE backend (`P(all N on 1 of 3) ≈ 3·(1/3)^4 ≈ 3.7%` per side).

**Fix (this follow-up):** raise **N=4 → 16** (K unchanged at 16). The per-side
collapse probability drops to `3·(1/3)^16 ≈ 7e-8` — past the 5σ-equivalent
flake-free margin (`reference_differential_band_sigma_margin`, applied to a spread
threshold rather than a σ-band). Verified empirically:

`pass=0; for i in $(seq 1 30); do go test ./test/differential/ -run 'TestDifferential/0062-lb-ring-hash-http' -count=1 && pass=$((pass+1)); done; echo $pass/30`
→ **30/30 PASS**.

The affinity (modular-invariant) and stats prongs — the producer-correctness proof
of Tasks 2-4 — were unaffected throughout (they are deterministic); only the
fixture's spread robustness changed.

## Firsts / non-additions

- **FIRST cross-side (both-side) consistent-hash affinity proof** — the
  NAT-transparent `X-Hash` header lets the modular invariant bite on BOTH the
  subject and the reference (0061 was subject-side only).
- **FIRST cross-equal `upstream_rq_total` on a ring_hash fixture** — the HTTP
  plane charges rq on both sides (the contrast with 0061's tcp_proxy boundary).
- **NO new BackendKind** — reuses `HTTPEcho` (the 0003 backend); the backend tail
  STAYS 33. An LB phase exercises WHERE requests land, not what the backend speaks.
- **NO new fuzzer** — the header hash key derives from a request header, not an
  untrusted wire frame; the hash-fold + ring-lookup property tests are UNIT-level.
- **NO boot-reject dir** — the route `hash_policy` config-reject arms
  (`parseRouteHashPolicies` unsupported-specifier / empty-header_name rejects) land
  UNIT-LEVEL in `config_test.go`.
