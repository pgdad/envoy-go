# 0095-lb-locality-weighted

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_lw` with `common_lb_config.locality_weighted_lb_config: {}` and **TWO**
`LocalityLbEndpoints` groups — region **a** (`load_balancing_weight: 2`, 5 hosts)
and region **b** (`load_balancing_weight: 1`, 5 hosts) — plus an **active HTTP
health checker** (path `/healthz`, fast convergence: `interval: 0.2s`,
`timeout: 0.2s`, thresholds `1`/`1`).

Phase 52 SPEC §8.1/§11.3 / PLAN Tasks 7-9.

It proves that `locality_weighted_lb_config` distributes load across localities
proportional to each locality's **effective weight** (`declared_weight ×
healthy_fraction`), on BOTH the envoy-go (subject) side and the reference-Envoy
side, and that the split **shifts** when one locality's health degrades.

## Topology: region B (runner-spawned) + region A (driver-owned toggleable)

| region | hosts | backing                                    | health                              |
|--------|-------|---------------------------------------------|--------------------------------------|
| a      | 5     | driver-owned `toggleResponder`s (self-managed `net.Listener`s) | starts ALL healthy; 3 of 5 toggled to fail `/healthz` in arm (b) |
| b      | 5     | runner-spawned `HTTPEcho` backends (`BackendCount()==5`)        | ALWAYS healthy               |

Region B reuses the existing `HTTPEcho` `BackendKind` (the runner spawns and owns
these 5 backends; they always answer 200). Region A's 5 hosts are **NOT** runner
backends — they are spun up directly by the driver, generalizing the 0066
`allocDeadPort` precedent from "bind-then-permanently-close" to "bind, keep
serving, and flip its `/healthz` answer on command":

- Any path other than `/healthz` → `200 "region-a:<idx>"` (the host-attribution
  signal the load-phase tally classifies on, mirroring region B's
  `"backend-<idx>:..."` body).
- `/healthz` → `200` while the responder's `atomic.Bool` health flag is `true`,
  `503` once the driver calls `SetHealthy(false)` on it (arm (b)'s
  controlled-degradation trigger).

Both `ReferenceBootstrap` and `SubjectConfig` call the SAME memoized
`ensureRegionA()` so both sides' `load_assignment` reference the identical 5
region-A ports (reference via `host.docker.internal:<port>`, subject via
`127.0.0.1:<port>` — the established cross-side HTTP addressing shape).

## Health-check config (identical on both sides)

```yaml
health_checks:
  - interval: 0.2s
    timeout: 0.2s
    unhealthy_threshold: 1
    healthy_threshold: 1
    http_health_check:
      path: /healthz
```

`unhealthy_threshold: 1` / `healthy_threshold: 1` gives single-probe convergence
in both directions — needed because arm (b) must observe a clean transition from
10/10 healthy down to 7/10 healthy without a multi-probe debounce delay.

## The two arms (both run inside `AssertStats`, the only hook holding both admin addrs)

Per the `reference_differential_asserter_dispatch` / `reference_health_check_
propagation_warmup` precedent (0066), the poll-to-converge + warmup + load +
assert sequence for BOTH arms lives inside `AssertStats` — the `Drive` hooks only
stash listener addrs and return the fixed `"READY\n"` byte stream for the
runner's `CompareBytes` gate.

**arm (a) — static ratio (all 10 hosts healthy).** Poll `cluster.c_lw.
membership_healthy == 10` on both sides, warm up until the data path stabilizes,
then send `staticLoadCount = 900` `GET /` requests per side and classify each
response body as region "a" or "b". With all 10 hosts healthy the effective
locality weights are `2×1.0 = 2` (region a) and `1×1.0 = 1` (region b), so
region A's confirmed share (SPEC §11.3, AMEND-LW3) is **66.7%** (`2/(2+1)`).
The assertion is a **band**: `staticShareA (0.667) ± staticBandPct (8.0
percentage points)` — a ~5σ margin at `n=900, p=0.667` (std ≈ `sqrt(900 × 0.667 ×
0.333)` ≈ 14.1 requests ≈ 1.57pp, so 5σ ≈ 7.85pp; 8.0pp is the rounded-up pin per
`reference_differential_band_sigma_margin`).

**arm (b) — health-degradation shift.** The driver calls `SetHealthy(false)` on
3 of region A's 5 `toggleResponder`s (`degradedAHosts = 3`), dropping region A to
2/5 = 40% healthy. Poll `cluster.c_lw.membership_healthy == 7` (2 healthy in
region A + all 5 in region B) on both sides, re-warm-up, then send
`degradedLoadCount = 900` MORE requests per side. The effective locality weights
are now `2×0.4 = 0.8` (region a) and `1×1.0 = 1` (region b), so region A's
confirmed share is **52.8%** (`0.8/(0.8+1)`) — the SPEC §11.3 EXACT match at the
live probe. Band: `degradedShareA (0.528) ± degradedBandPct (8.5 percentage
points)` — 5σ at `n=900, p=0.528` (std ≈ `sqrt(900 × 0.528 × 0.472)` ≈ 14.98 ≈
1.66pp, 5σ ≈ 8.3pp; 8.5pp rounded up).

Arm (b) MUST run strictly after arm (a) in the same `AssertStats` invocation —
the degradation trigger (`SetHealthy(false)`) is a one-way, in-process mutation
of the shared `toggleResponder`s, so the two arms cannot be reordered or split
across separate driver instances.

## Cross-side deterministic stats (both sides, both arms)

| stat                                  | assertion                              |
|----------------------------------------|-----------------------------------------|
| `cluster.c_lw.membership_total`        | `== 10` always (filtering, not removal) |
| `cluster.c_lw.membership_healthy`      | `== 10` (arm a) / `== 7` (arm b, post-degrade) |
| `cluster.c_lw.upstream_rq_total`       | `>= 1800` (`staticLoadCount + degradedLoadCount`) |

Plus the "decode ran" guard (`reference_docker_probe_bridge_network`):
`cluster.c_lw.upstream_rq_total > 0` on the reference side before trusting the
readout.

## `response-body` scope

Byte-exact comparison applies ONLY to the fixed `"READY\n"` `Drive` stream
(address-independent). The load-phase `GET /` bodies are tallied for the
per-side region-share band assertions inside `AssertStats`, NOT byte-compared —
a randomized/weighted LB policy makes cross-side per-request identity
infeasible (the 0060/0065 lineage).

## Non-additions

- **NO new `BackendKind`** — region B reuses `HTTPEcho`; region A is
  driver-owned per `reference_differential_fixture_dispatch_constraint`.
- **NO `DistributionAsserter`** — the region-share bands live in `AssertStats`,
  off the same load-and-tally pass that must sequence arm (a) BEFORE arm (b)'s
  degradation trigger (`reference_differential_asserter_dispatch`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0095'` (NOT `-run '0095'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).

## Task 9: deliberate-break liveness + the `-count=N` singleton-driver fix

Both `AssertStats` bands were proven LIVE via the SPEC §8.1 deliberate breaks
(`reference_differential_break_protocol_count1`), each applied, run, observed
FAILING, then undone with `git restore` (never checkout-sha/amend —
`feedback_subagent_worktree_detach`):

**Break (i) — skip the `locality_weighted_lb_config` wrap.** Temporarily
commented out `manager.go`'s `case lwc != nil:` body's `lb = lw` (leaving the
flat, un-wrapped `buildLeafLB` output as `cl.lb`). The cluster falls back to
plain ROUND_ROBIN over all 10 hosts, collapsing region A's share to ~50%:

```
runner_test.go:1293: subject/static: region A share = 50.00% (a=450 b=450), want 66.7% ± 8.0pp
runner_test.go:1293: subject/degraded: region A share = 28.44% (a=256 b=644), want 52.8% ± 8.5pp
```

Both arms fail (arm (b) is built on the same unwrapped LB, so it fails too —
expected, not a coverage gap). `git restore internal/cluster/manager.go` →
re-run → PASS.

**Break (ii) — freeze the effective-weight computation at the 100%-healthy
value.** Temporarily hardcoded `frac := 1.0` unconditionally at the top of
`Pick`'s per-group loop in `locality.go` (commenting out the
`if lw.health != nil && len(g.endpoints) > 0 { frac = ... }` branch), so
`effectiveWeight` never sees live health. Arm (a) still passes (measured at
100% health, where the break is a no-op); arm (b)'s post-degradation share
stays pinned near the 100%-healthy ~66.7% instead of shifting to ~52.8%:

```
runner_test.go:1293: subject/degraded: region A share = 67.33% (a=606 b=294), want 52.8% ± 8.5pp
```

`git restore internal/cluster/locality.go` → re-run → PASS.

**A genuine bug found and fixed during the ≥20-run flake check** (NOT a flake
— deterministic 19/20 failure): `lwDriver` is a process-lifetime singleton
(`init()`-registered via `fixture.RegisterFixture`), and `d.regionA`'s 5
`toggleResponder`s are memoized once (`ensureRegionA`). Under `-count=N`, arm
(b)'s `SetHealthy(false)` on 3 of them was never reset, so run 2 onward
inherited run 1's degraded health state. Worse, resetting inside `AssertStats`
(tried first) was too late: Envoy's active health checker locks a
host observed-unhealthy-on-its-first-probe onto the `no_traffic_interval`
cadence (default 60s, unset in this fixture — only
`no_traffic_healthy_interval` is pinned to 0.2s, which applies to
ALREADY-healthy hosts only) until cluster traffic flows once, so the 30s
convergence-poll deadline was never enough:

```
runner_test.go:1293: arm(a) converge: reference: cluster.c_lw.membership_healthy did not converge to 10 within 30s (last seen 7)
```

**Fix:** reset every `toggleResponder` to healthy inside `ensureRegionA`
itself, on every call (not just the first) — before `ReferenceBootstrap`/
`SubjectConfig` return their YAML and the containers boot, so both sides see
all 10 hosts healthy from their very first probe. See
`driver/driver.go`'s `ensureRegionA` for the fix + full rationale comment.

**Verification after the fix:**
- Both deliberate breaks (i) and (ii) re-run and re-confirmed FAILING/restored
  exactly as above, post-fix.
- `-count=20`: **20/20 PASS** (89.4s total).
- `-race -count=1`: **PASS**, no data race (`toggleResponder.healthy` is an
  `atomic.Bool`; `lwDriver`'s stashed fields are `sync.Mutex`-guarded).
