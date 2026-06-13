# 0063-lb-maglev

Cross-side `[http_connection_manager + router]` differential over ONE 3-endpoint
cluster with `lb_policy: MAGLEV` + `maglev_lb_config: {}` (default `table_size`
65537) + a **route-level** `hash_policy: [{header: {header_name: "x-hash"}}]` on
BOTH sides (the 0003 HTTP shape: reference STRICT_DNS / `host.docker.internal`,
subject STATIC / `127.0.0.1`). This is the **end-to-end proof of the maglev plane**
— the HTTP route `hash_policy` producer → `cluster.HashHeaderValues` digest →
`applyHashKey` → `cluster.WithHashKey` → `maglevLB` pick → per-header-value affinity
over a Maglev TABLE — that envoy-go's Maglev consistent-hash load balancer threads a
header-derived key into the SAME table with the SAME affinity shape and moves the
SAME cluster stats (cross-equal `maglev_lb.*` gauges) as the reference Envoy
`contrib-v1.37.2`.

It is the **MAGLEV transposition of `0062-lb-ring-hash-http`** (the ring_hash HTTP
plane): SAME `X-Hash` header workload, SAME both-side modular invariant, retargeted
from RING_HASH to MAGLEV. As in 0062, the **`X-Hash` HEADER is NAT-transparent** —
it survives the Docker hop verbatim — so the consistent-hash affinity invariant
holds on **BOTH** the envoy-go subject AND the reference Envoy. This is a **TRUE
cross-side affinity proof**.

Phase 37 SPEC §10 (the HTTP route `hash_policy` plane over MAGLEV) / §7 (the
cross-side stats set incl `upstream_rq_total`) / 37 PLAN Task 7.

## The workload (N=16 distinct X-Hash values × K=16)

For each of **N=16** distinct `X-Hash` values (`hv-0` … `hv-15`) the driver sends
**K=16** `GET /get` requests carrying that value in the `X-Hash` header. **16 × 16
= 256** routed requests (`totalReqs`, DERIVED from `hashValues * repeatPerVal` —
the cx/rq stat expectations track the constants, never a literal). Each request is a
fresh dial (`HTTPRoundTrip` sets `Connection: close`), so each routed request is
one upstream connection → the `HTTPEcho` backend's accept counter increments once
per request, yielding per-backend request counts directly.

After the routed phase the driver sends **8 `GET /health`** round-trips. `/health`
is a `direct_response` (`inline_string: "OK\n"`) served by the listener — it does
NOT touch the backend (no accept, no `upstream_cx`), and its body is **address-
independent** → byte-equal across both proxies. That `OK\n`×8 stream is the
runner's `CompareBytes` input (the 0003 byte-equiv precedent).

### Why K=16 and N=16

K is the **modular base** of the affinity invariant: each distinct value is
internally deterministic (it hashes to ONE table slot → ONE backend) and is sent
exactly K times, so every backend's aggregate count is a sum of whole K-blocks →
`≡ 0 mod K`. K=16 makes a per-request scatter break statistically certain to produce
a non-multiple-of-16 backend total. Smaller K weakens the discriminating power;
larger K only lengthens the run.

N is the **spread robustness** knob. The Maglev table is built over the endpoint
ADDRESS strings (`<addr>:<port>`) and the harness allocates the 3 backend ports
DYNAMICALLY per run, so the table layout varies run-to-run. With N distinct values
over 3 backends, the per-side probability that ALL N values collapse onto a SINGLE
backend (failing spread) is `≈ 3·(1/3)^N`; **N=16** gives `3·(1/3)^16 ≈ 7e-8` per
side — past the 5σ-equivalent flake-free margin
(`reference_differential_band_sigma_margin`, applied to a spread threshold rather
than a σ-band). This inherits 0062's N=16 choice verbatim.

### Why the routed bodies are NOT byte-compared

The `HTTPEcho` backend embeds its own idx in the body (`backend-<idx>:<seg>`). The
ref and subj tables are built over DIFFERENT endpoint address strings (STRICT_DNS
`host.docker.internal:<port>` vs STATIC `127.0.0.1:<port>`), so a given `X-Hash`
value may land on a DIFFERENT backend idx per side — the per-request routed bytes
diverge cross-side. Cross-side host **identity** is therefore **INFEASIBLE**
(`reference_differential_hash_key_cross_side_infeasible`); the routed bytes are
not concatenated. Affinity is proven by the aggregate-count modular invariant,
which needs no host attribution.

## The affinity+spread arm (`AssertDistribution`) — the modular invariant

`AssertDistribution` is **DETERMINISTIC / EXACT** — NOT a σ-band
(`reference_differential_band_sigma_margin` governs RNG-distributed bands; maglev
affinity is not one). The runner snapshots `backend.accepts` after Drive (each
accept on the `HTTPEcho` backend counts one routed request; the 256 (`totalReqs`)
requests sum across the three backends). The invariant is checked on **BOTH** sides
(the NAT-transparent header is the whole point):

- **affinity:** each per-backend count `cᵢ ≡ 0 mod 16` — VIOLATED by a key-scatter
  break (a value splitting across backends produces a count NOT a multiple of 16);
- **spread:** `>= 2` backends nonzero — VIOLATED by a collapsed table (all 256 on one
  backend);
- **conservation:** `c1 + c2 + c3 == 256` (`totalReqs`; catches drop / double-count).

This is the **consistent-hash one-value-one-backend property over a Maglev TABLE**:
the Maglev lookup table maps every hash value to exactly one host, so a given
`X-Hash` value (one digest) always indexes the same table slot → the same backend.

## The stats prong (`StatsAsserter`, post-drive) — SPEC §7

The §7 cross-side set, EXTENDED with the **2 `maglev_lb.*` gauges** AND the
**cross-equal `upstream_rq_total`** (the HTTP plane Inc's `upstream_rq_total` on BOTH
sides). Observed (every run, both sides):

| stat                                              | reference | subject | disposition                     |
|---------------------------------------------------|-----------|---------|---------------------------------|
| `cluster.c_echo.upstream_cx_total`                | 256       | 256     | **cross-equal** == 256 (=`totalReqs`) |
| `cluster.c_echo.upstream_rq_total`                | 256       | 256     | **cross-equal** == 256          |
| `cluster.c_echo.membership_total`                 | 3         | 3       | **cross-equal** == 3            |
| `cluster.c_echo.upstream_cx_active`               | 0         | 0       | **cross-equal** == 0 (quiesced) |
| `cluster.c_echo.maglev_lb.min_entries_per_host`   | 21845     | 21845   | **cross-equal** == 21845        |
| `cluster.c_echo.maglev_lb.max_entries_per_host`   | 21846     | 21846   | **cross-equal** == 21846        |

The **2 `maglev_lb.*` gauges are cross-equal** because they depend ONLY on
`(table_size, host count)`, NOT on endpoint ADDRESSES — so they are IDENTICAL on
subject and reference for a 3-equal-host default MAGLEV cluster. With the default
`table_size = 65537` and 3 equal-weight hosts, each host gets either `floor(65537/3)
= 21845` (`min_entries_per_host`) or `ceil(65537/3) = 21846`
(`max_entries_per_host`) table slots. (Unlike ring_hash there is **no** `maglev_lb.size`
gauge — the table size is the fixed `table_size`, not a derived ring size.)

The `upstream_cx_total == upstream_rq_total == 256` cross-equality also proves the
decode actually ran on BOTH sides (non-zero upstream traffic — the
`reference_docker_probe_bridge_network` "decode ran" signal): had the reference
container failed to reach the backends, `upstream_cx_total` would be 0 and the
prong would bite.

## Deliberate-break liveness (Task 8 — `-count=1`)

Each prong was applied ONE AT A TIME to PRODUCTION code (`maglev.go`) or the
driver's expected stat, run with `-count=1` to defeat go-test caching
(`reference_differential_break_protocol_count1`), the named prong observed to
`--- FAIL`, then `git restore`d (production tree left UNCHANGED — verified clean
`git status` after each revert). Selector: `-run 'TestDifferential/0063'`
(prefix-matches `TestDifferential/0063-lb-maglev`;
`reference_differential_run_selector`).

| # | break | site | prong bitten | observed `--- FAIL` |
|---|-------|------|--------------|---------------------|
| i | scatter the key (make `hashKey = mg.rng()` unconditional — ignore the key on every Pick) | `maglevLB.Pick` (`maglev.go`) | **affinity** | `distribution: subject affinity: backend[0]=83 not a multiple of 16 (key scattered? an X-Hash value split across backends)` |
| ii | collapse the BUILT table to host-0 AND corrupt the build gauge tallies (`min→0`, `max→65537`) | `newMaglevWithRNG` (`maglev.go`, after the populate loop) | **spread** + **both `maglev_lb.*` gauges** | spread: `distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (table collapsed?)`; gauge-max: `cross-side mismatch cluster.c_echo.maglev_lb.max_entries_per_host: ref=21846 subj=65537` + `subj … max_entries_per_host = 65537, want 21846`; gauge-min: `cross-side mismatch cluster.c_echo.maglev_lb.min_entries_per_host: ref=21845 subj=0` + `subj … min_entries_per_host = 0, want 21845` |
| iii | corrupt one cross-equal want (`upstream_cx_total` → `totalReqs+1`) | `AssertStats` (driver's `driver.go`) | **stats** (`StatsAsserter`) | `ref cluster.c_echo.upstream_cx_total = 256, want 257` + `subj … upstream_cx_total = 256, want 257` |

### BUILD-level vs Pick-level collapse — why break (ii) is BUILD-level (the key lesson)

Break (ii) MUST collapse the table **AT BUILD** (`reference_differential_asserter_dispatch`).
The `maglev_lb.{min,max}_entries_per_host` gauges are computed from the build-time
`count[]` tallies, NOT from Pick. A WEAKER **Pick-level** short-circuit (e.g.
returning `table[0]` in `Pick` without touching the build) would bite ONLY the
spread leg — the gauges would STAY 21845/21846, the cross-equal gauge assertion
would NOT fire, and the gauge prong would be VACUOUSLY green. The BUILD-level
collapse used here corrupts both the table layout (→ spread) AND the `count[]`
tallies (→ gauges min=0/max=65537), proving the gauge prong is LIVE. This contrast
is the reason break (ii) names its site as `newMaglevWithRNG` (build) and not `Pick`.

### Flake check

`go test ./test/differential/ -run 'TestDifferential/0063' -count=20` → **20/20 PASS**
(confirmed non-vacuous via `-v`: 20 `--- PASS: TestDifferential/0063-lb-maglev`
lines). The affinity leg is DETERMINISTIC (the table is fixed at build; the X-Hash
keys are fixed) → it can never flake; the spread leg (`>= 2` nonzero) is
overwhelmingly stable (`3·(1/3)^16 ≈ 7e-8` per-side collapse probability). After
all three reverts the differential is GREEN (`-count=1`), `git status` clean, branch
`phase-37-impl` (not detached).

## Firsts / non-additions

- **SECOND cross-side (both-side) consistent-hash affinity proof** — the
  NAT-transparent `X-Hash` header lets the modular invariant bite on BOTH the
  subject and the reference, now over a Maglev TABLE (the 0062 retarget).
- **SECOND non-zero LB-stat delta** — the +2 `maglev_lb.*` gauges (21845/21846)
  surface in the cluster stat set (surface 1121).
- **NO new BackendKind** — reuses `HTTPEcho` (the 0003 backend); the backend tail
  STAYS 33. An LB phase exercises WHERE requests land, not what the backend speaks.
- **NO new fuzzer** — the header hash key derives from a request header, not an
  untrusted wire frame; the hash-fold + table-lookup property tests are UNIT-level.
- **NO boot-reject dir** — the maglev `table_size` / route `hash_policy`
  config-reject arms land UNIT-LEVEL.
