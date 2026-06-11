# 0060-lb-random

Cross-side `[tcp_proxy]` differential over ONE 3-endpoint cluster with
`lb_policy: RANDOM` on BOTH sides (the 0001 shape: reference STRICT_DNS /
`host.docker.internal`, subject STATIC / 127.0.0.1). This is the end-to-end proof
that envoy-go's `random` load balancer lands connections in the SAME distribution
shape (per-side anti-skew bands) and moves the SAME cluster stats (cross-equal /
per-side) as the reference Envoy `contrib-v1.37.2`.

It is a faithful TRANSPOSITION of `0059-lb-least-request`: the SAME hold-4 +
burst-60 + drain workload, the SAME 3 TCPEcho backends, the SAME `StatsAsserter`
— but the cluster uses `lb_policy: RANDOM` (no `lb_config` — AMEND-R1) and the
`AssertDistribution` is **INVERTED**.

Phase 35 SPEC §10 / PLAN Task 5.

## The workload (REUSED from 0059, identical per side, sequential)

1. **Hold phase (K = 4):** open 4 connections; on each, write one payload and
   read the echo (the **establishment witness** — AMEND-L2: an open TCP-proxied
   connection IS one active request, so the read confirms the upstream dial
   completed and the pick's active count is held), then KEEP the socket open.
2. **Burst phase (S = 60):** 60 sequential short round-trips
   (`helpers.TCPRoundTrip` — write, half-close, read echo, close), allowing
   close-accounting between picks. Under RANDOM the burst IGNORES the held load,
   so it spreads UNIFORMLY across all three endpoints.
3. **Drain:** close the 4 held conns; a 750 ms settle lets the upstream closes
   propagate so `upstream_cx_active` quiesces to 0 before the stats scrape.

Total connections per side: 4 + 60 = **64** (the conservation target).

The echo stream is deterministic (the streaming `TCPEcho` backend echoes each
read chunk immediately), so the concatenated bytes are byte-identical across the
two proxies regardless of WHICH endpoint serves WHICH connection — the runner's
`CompareBytes` gate passes cross-side.

## The band arm (`AssertDistribution`, PER SIDE) — the INVERSION of 0059

This is the **CONTRAPOSITIVE** of `0059`. Where `0059` (least_request) asserts
that the loaded backend is **AVOIDED** (starvation `c1 <= 12` + concentration
`c2 >= 16` — a SKEW), `0060` asserts the **OPPOSITE**: RANDOM ignores load, so
**every** backend stays in a fair-share **ANTI-SKEW** band DESPITE the identical
held-conn load.

The band is **SYMMETRIC** — checked on each of the three raw per-backend accept
counts (no sort needed; each `cᵢ` must independently land in `[floor, ceiling]`):

- **conservation:** `c1 + c2 + c3 == 64` (hard equality; catches drop /
  double-count);
- **uniform floor:** each `cᵢ >= uniformFloor` (=`6`) — VIOLATED by a
  load-skewing policy starving a loaded backend (0059's `c1 ≈ 2`) and by a
  single-host pin (the un-picked backends fall to 0);
- **uniform ceiling:** each `cᵢ <= uniformCeiling` (=`40`) — VIOLATED by a
  single-host pin (one backend ≈ 64).

The bounds are NAMED constants (`uniformFloor` / `uniformCeiling`) in the driver
— a future tune touches ONE place (the check AND its error message read the
constant).

### The σ-margin math

Per-backend accepts are ≈ `multinomial(64, 1/3)`. Mean per backend
`μ = 64/3 = 21.3`; per-backend stdev `σ = sqrt(64 · (1/3) · (2/3)) ≈ 3.77`.

The runner draws **6 independent** per-backend samples per run (3 backends × 2
sides), so a TIGHT band flakes on the natural ~2.5σ tail. **Task-6 tuning WIDENED
the band** from the originally-pinned `{12, 32}` to the present `{6, 40}` — the
old band EMPIRICALLY flaked 2/20 over a `-count=20` batch (a single `binomial(64,
1/3)` sample dipping to 9–10 < 12, and another reaching 33–34 > 32). The wide band
stays flake-free over repeat batches AND still bites every deliberate break:

- `floor 6` is `μ − 4.05σ` (`(21.3 − 6)/3.77 ≈ 4.05`): per-sample
  `P(binomial(64,1/3) < 6) ≈ 2e-6` → `< 0.03%` over a 120-sample batch. A
  fair-share backend never realistically dips below 6 — but a *load-skewing*
  policy puts the loaded backend at ≈ 2 deterministically, and a single-host pin
  drops the un-picked backends to 0; the floor catches both with a wide margin.
- `ceiling 40` is `μ + 4.95σ` (`(40 − 21.3)/3.77 ≈ 4.95`): per-sample
  `P(binomial(64,1/3) > 40) ≈ 3e-7` → effectively never on the fair-share path; a
  single-host pin (`c = 64`) blows through it.

Asserted **PER SIDE** — NEVER cross-side-exact, because the two sides run
independent RNG streams (the 0059 per-side-asymmetry precedent). Both sides must
land in the band.

### Contrast with 0059

| backend role under the held load | 0059 `least_request` | 0060 `random` |
|----------------------------------|----------------------|---------------|
| the most-held backend | STARVED (`c1 ≈ 2`) | fair-share (`≈ 21`) |
| the two least-loaded | absorb the burst (`c2,c3 ≈ 26–36`) | fair-share (`≈ 21`) |
| assertion direction | **SKEW** required | **ANTI-SKEW** required |

A distribution that PASSES 0059 (e.g. `{2, 26, 36}`) FAILS 0060's floor; a
distribution that PASSES 0060 (e.g. `{19, 22, 23}`) FAILS 0059's starvation leg.
The two fixtures are mutually exclusive — exactly the contrapositive relationship.

### Observed distribution (Task 5 + Task 6 flake check)

`go test ./test/differential/ -run 'TestDifferential/0060' -count=1` PASSED on
the first live-probe run (Task 5). At **Task 6** the band was re-tuned and the
flake check run as **two independent `-count=20` batches (40 runs total)** with
the final `{6, 40}` band: **40/40 PASS, zero band violations.** Observed extremes
over the 40 runs (each run draws 3 backends × 2 sides): per-backend min `14`,
max `31` — comfortably inside the `[6, 40]` band on every run. (The pre-tune `{12,
32}` band flaked 2/20 in an earlier batch — see the σ-margin note above.)

## The stats prong (`StatsAsserter`, post-drain) — IDENTICAL to 0059

SPEC §7 / §10 / AMEND-R4. The stat surface is UNCHANGED by RANDOM (zero stat
delta — total stays 1116). Observed (every run, both sides):

| stat                                  | reference | subject | disposition          |
|---------------------------------------|-----------|---------|----------------------|
| `cluster.c_echo.upstream_cx_total`    | 64        | 64      | **cross-equal** == 64 |
| `cluster.c_echo.membership_total`     | 3         | 3       | **cross-equal** == 3  |
| `cluster.c_echo.upstream_cx_active`   | 0         | 0       | **cross-equal** == 0 (quiesced) |
| `cluster.c_echo.upstream_rq_total`    | 64        | 0       | **PER-SIDE**          |

`upstream_rq_total` is PER-SIDE (NOT cross-equal): the reference's `tcp_proxy`
charges one rq per cx (rq-per-cx — AMEND-L2) → 64; envoy-go's tcpproxy path
NEVER calls `IncUpstreamRqTotal` (a pre-existing documented boundary — AMEND-R4)
→ 0.

`tcp_proxy` is 1:1 downstream-conn → upstream-dial on both sides, so
`upstream_cx_total` is cross-equal at exactly the 64-conn workload total.

## Deliberate-break liveness (Task 6 — `-count=1`)

The band + stats prongs are made non-vacuous by THREE deliberate breaks run with
`-count=1` per `reference_differential_break_protocol_count1` (go-test caching
defeated; a band that cannot fail is a dead assertion — the 0030 dead-assertion
lesson, generalized to bands). Each break was applied, run, observed to FAIL the
EXPECTED leg, then `git restore`d (production code byte-identical after revert).

**Task-6 break record (`-count=1`, band `{6, 40}`):**

| # | break (production code, transient) | expected leg | observed FAIL | reverted |
|---|------------------------------------|--------------|---------------|----------|
| i | `manager.go` `case Cluster_RANDOM`: `newRandom(endpoints)` → `newLeastRequest(endpoints, 10)` (the canonical anti-skew break — least_request consults the held-conn counters and skews picks AWAY from the most-held backend) | **uniform floor** | `subject: uniform floor: backend[1]=2 < 6 (load-skewing policy? single-host pin?)` | `git restore internal/cluster/manager.go`, diff empty |
| ii | `random.go` `Pick`: `i := int(r.rng() % uint64(n))` → `i := 0` (single-host pin — all 64 picks to endpoints[0]) | **uniform ceiling** (+ floor on the un-picked backends at 0) | `subject: uniform ceiling: backend[0]=64 > 40 (single-host pin?)` | `git restore internal/cluster/random.go`, diff empty |
| iii | `driver.go` `AssertStats`: cross-equal `upstream_cx_total` want `totalConns` (64) → `99` | **stats prong** | `ref … upstream_cx_total = 64, want 99` + `subj … upstream_cx_total = 64, want 99` | `git restore` driver.go (then band re-applied), diff to HEAD limited to band tuning |

All three breaks bit the expected leg. The floor break (i) is the symmetric
inverse of 0059's starvation break; the ceiling break (ii) trips first on the
pinned backend (the asserter returns on the first per-element violation, so the
ceiling on `backend[0]=64` reports before the loop reaches the `0 < 6` floor
violations on the un-picked backends — both legs are nonetheless proven live, the
floor by break (i) and the unit test `{62,1,1}`/`{2,26,36}` rows). The stats
prong (iii) is non-vacuous: both sides actually observed 64, so the corrupted
want=99 trips it on both.

## Firsts / non-additions

- **SECOND band-based `AssertDistribution`** (after 0059) — the FIRST *anti-skew*
  (symmetric floor/ceiling) band.
- **NO new BackendKind** — reuses `TCPEcho` (0); the tail STAYS unchanged. An LB
  phase exercises WHERE connections land, not what the backend speaks (AMEND-R1).
- **NO new fuzzer** — phase 35 decodes no wire bytes (config parse is
  proto-level, already fuzz-covered).
- **NO boot-reject dir** — the lb-policy / config reject arms land UNIT-LEVEL in
  `manager_test.go`.
- **ZERO stat delta** — the stat surface is unchanged (total stays 1116).
