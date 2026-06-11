# 0059-lb-least-request

Cross-side `[tcp_proxy]` differential over ONE 3-endpoint cluster with
`lb_policy: LEAST_REQUEST` + `least_request_lb_config: { choice_count: 10 }` on
BOTH sides (the 0001 shape: reference STRICT_DNS / `host.docker.internal`,
subject STATIC / 127.0.0.1). This is the end-to-end proof that envoy-go's
`leastRequest` P2C load balancer lands connections in the SAME distribution
shape (per-side bands) and moves the SAME cluster stats (cross-equal /
per-side) as the reference Envoy `contrib-v1.37.2`.

Phase 34 SPEC §8.1 / PLAN Task 6.

## The workload (identical per side, sequential)

1. **Hold phase (K = 4):** open 4 connections; on each, write one payload and
   read the echo (the **establishment witness** — AMEND-L2: an open TCP-proxied
   connection IS one active request, so the read confirms the upstream dial
   completed and the pick's active count is held), then KEEP the socket open.
   The held picks self-spread under least-loaded filling; with only 3 endpoints
   at least one holds 2 conns.
2. **Burst phase (S = 60):** 60 sequential short round-trips
   (`helpers.TCPRoundTrip` — write, half-close, read echo, close), allowing
   close-accounting between picks. Under P2C the burst AVOIDS the held-heavy
   endpoints, producing a SKEWED distribution.
3. **Drain:** close the 4 held conns; a 750 ms settle lets the upstream closes
   propagate so `upstream_cx_active` quiesces to 0 before the stats scrape.

Total connections per side: 4 + 60 = **64** (the conservation target).

The echo stream is deterministic (the streaming `TCPEcho` backend echoes each
read chunk immediately), so the concatenated bytes are byte-identical across the
two proxies regardless of WHICH endpoint serves WHICH connection — the runner's
`CompareBytes` gate passes cross-side.

## The band arm (`AssertDistribution`, PER SIDE)

The **FIRST band-based `AssertDistribution`** (every prior use — 0001/0002/0003/
0004/0045 — asserts EXACT counts). The per-backend accept counts (each accept on
the streaming `TCPEcho` backend counts one connection: hold + burst) are sorted
`c1 ≤ c2 ≤ c3` and checked per side:

- **conservation:** `c1 + c2 + c3 == 64`;
- **starvation:** `c1 <= starvationMax` (=`12`) — the most-held backend gets ≈ its
  held conns + ~0 burst landings. Under ROUND_ROBIN `c1 == 21` → BITES the
  no-op-release break (Task 7 leg (ii));
- **concentration:** `c2 >= concentrationMin` (=`16`) — the two least-loaded
  backends split the burst. Catches an INVERTED P2C comparison, where `c2` would
  be ≈ 0/1 (Task 7 leg (i)).

The bounds are NAMED constants (`starvationMax` / `concentrationMin`) in the
driver — a future tune touches ONE place (the check AND its error message read the
constant).

Asserted **PER SIDE** — NEVER cross-side-exact, because the two sides run
independent RNG streams (the BRAINSTORM band-semantics decision; the 0003
per-side-asymmetry precedent). Both sides must land in the band.

### Why `choice_count: 10` (legal > endpoint-count)

`choice_count: 10` over 3 endpoints is ACCEPTED silently at boot on both sides
(no clamp — AMEND-L3). It makes the skew quasi-deterministic (the loaded host
takes ~0 burst landings — P(one of 10 sampled draws hits the loaded host and it
wins) ≈ tie-window transients only) and the band ROBUST. `cc=2`'s skew is real
but too noisy to band tightly.

### Observed distribution + flake check (Task 7 — `-count=20`, 20/20 PASS)

`go test ./test/differential/ -run 'TestDifferential/0059' -count=20` booted the
reference container 20× → **20/20 PASS**. Each run exercises BOTH sides, so 40
sorted `[c1 c2 c3]` observations (20 per side):

| side      | runs | c1 (min/max) | c2 (min/max) | c3 (min/max) | sum |
|-----------|------|--------------|--------------|--------------|-----|
| reference | 20   | 2 / 2        | 22 / 31      | 31 / 40      | 64  |
| subject   | 20   | 2 / 2        | 21 / 31      | 31 / 41      | 64  |

`c1` was **always exactly 2** (margin to `starvationMax=12`: 10); `c2` ranged
**21–31** (min margin to `concentrationMin=16`: 5); sum always 64. The plan's
anticipated constants (K=4 / S=60 / `c1<=12` / `c2>=16`) are confirmed by reality
with large margins — **the constants are UNCHANGED from 12/16; no widening was
needed**. (The constants stay tight enough that all three Task-7 deliberate breaks
still bite — a band widened past the break points would be a dead assertion.)

## The stats prong (`StatsAsserter`, post-drain)

SPEC §7 / §8.1 / AMEND-L4. Observed (every run, both sides):

| stat                                  | reference | subject | disposition          |
|---------------------------------------|-----------|---------|----------------------|
| `cluster.c_echo.upstream_cx_total`    | 64        | 64      | **cross-equal** == 64 |
| `cluster.c_echo.membership_total`     | 3         | 3       | **cross-equal** == 3  |
| `cluster.c_echo.upstream_cx_active`   | 0         | 0       | **cross-equal** == 0 (quiesced) |
| `cluster.c_echo.upstream_rq_total`    | 64        | 0       | **PER-SIDE**          |

`upstream_rq_total` is PER-SIDE (NOT cross-equal): the reference's `tcp_proxy`
charges one rq per cx (rq-per-cx — AMEND-L2) → 64; envoy-go's tcpproxy path
NEVER calls `IncUpstreamRqTotal` (only the HCM router + the ADR-0230 seam
consumers do — a pre-existing documented boundary this fixture now pins) → 0.

`tcp_proxy` is 1:1 downstream-conn → upstream-dial on both sides, so
`upstream_cx_total` is cross-equal at exactly the 64-conn workload total. All
observed stat values match the plan's pinned numbers exactly.

## Deliberate-break liveness (Task 7 — `-count=1`)

The band + stats prongs are made non-vacuous by three deliberate breaks recorded
at Task 7 per `reference_differential_break_protocol_count1` (go-test caching
defeated with `-count=1`; a band that cannot fail is a dead assertion):

- **(i) invert the P2C comparison** (`candActive < bestActive` → `>`, pick MOST
  loaded) → the burst concentrates on one host → the **concentration** leg FAILS
  deterministically.
- **(ii) make `leastRequest`'s release a NO-OP** (`release := func() {}` — counters
  never decrement → cumulative-pick leveling ≈ uniform `{21,21,22}`) → `c1 ≈ 21`
  → the **starvation** leg FAILS deterministically. (The never-incremented-counter
  variant is NOT canonical — it degenerates to random picks ≈ multinomial(64,
  1/3), which leaves `c1 > 12` only ~97% of the time per run; D-S34-3.)
- **(iii) corrupt a cross-equal stat want** (`upstream_cx_total` want 64 → 99) →
  the `StatsAsserter` prong FAILS.

**Task-7 break record (`-count=1`, contrib-v1.37.2):**

| break | edit (REVERTED) | failing leg / output |
|-------|-----------------|----------------------|
| (i) inverted comparison | `leastrequest.go`: `<` → `>` | `distribution: subject: concentration: c2=0 < 16 (inverted comparison?)` |
| (ii) no-op release | `leastrequest.go`: `release := func() {}` | `distribution: subject: starvation: c1=21 > 12 (no skew? round-robin?)` |
| (iii) corrupt stat want | `driver.go`: `upstream_cx_total` 64 → 99 | `ref/subj cluster.c_echo.upstream_cx_total = 64, want 99` |

Each break failed the EXACT predicted leg, then was REVERTED (`git diff
internal/cluster/leastrequest.go` EMPTY; the fixture re-runs GREEN). Each leg is
PROVEN live — none is a dead assertion (the `0030` lesson).

## Firsts / non-additions

- **FIRST band-based `AssertDistribution`** (conservation / starvation /
  concentration).
- **NO new BackendKind** — reuses `TCPEcho` (0); the tail STAYS 33. An LB phase
  exercises WHERE connections land, not what the backend speaks (SPEC §8.3).
- **NO new fuzzer** — phase 34 decodes no wire bytes (config parse is
  proto-level, already fuzz-covered); fuzzers STAY 42 (SPEC §8.3).
- **NO boot-reject dir** — the `choice_count < 2` + lb-policy reject arms land
  UNIT-LEVEL in `manager_test.go` (SPEC §8.2 / AMEND-L5). Fixture count 60 → 61.
