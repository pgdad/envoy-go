# 0096-lb-priority

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_pri` with **TWO** `LocalityLbEndpoints` groups at distinct `priority` values
(`0` and `1`), 5 hosts each, plus an **active HTTP health checker** (path
`/healthz`, fast convergence: `interval: 0.2s`, `timeout: 0.2s`, thresholds
`1`/`1`).

Phase 53 SPEC §8.1/§11.1/§11.4 / PLAN Tasks 8-10.

It proves that the priority LB waterfall sends **all** traffic to the
highest-priority tier that is fully healthy, and that traffic **fails over
completely** to the next tier the instant the top tier's capacity collapses —
on BOTH the envoy-go (subject) side and the reference-Envoy side.

## Topology: tier 1 (runner-spawned) + tier 0 (driver-owned toggleable)

| tier | priority | hosts | backing                                                         | health                                          |
|------|----------|-------|------------------------------------------------------------------|--------------------------------------------------|
| 0    | 0        | 5     | driver-owned `toggleResponder`s (self-managed `net.Listener`s)   | starts ALL healthy; ALL 5 toggled to fail `/healthz` in arm (b) |
| 1    | 1        | 5     | runner-spawned `HTTPEcho` backends (`BackendCount()==5`)         | ALWAYS healthy                                   |

Tier 1 reuses the existing `HTTPEcho` `BackendKind` (the runner spawns and owns
these 5 backends; they always answer 200). Tier 0's 5 hosts are **NOT** runner
backends — they are spun up directly by the driver, the 0095-lb-locality-
weighted `toggleResponder` precedent reused verbatim (only the response-body
prefix changes: `"tier0:"` here vs. `"region-a:"` there):

- Any path other than `/healthz` → `200 "tier0:<idx>"` (the host-attribution
  signal the load-phase tally classifies on, mirroring tier 1's
  `"backend-<idx>:..."` body).
- `/healthz` → `200` while the responder's `atomic.Bool` health flag is `true`,
  `503` once the driver calls `SetHealthy(false)` on it (arm (b)'s full-failover
  trigger).

Both `ReferenceBootstrap` and `SubjectConfig` call the SAME memoized
`ensureTier0()` so both sides' `load_assignment` reference the identical 5
tier-0 ports (reference via `host.docker.internal:<port>`, subject via
`127.0.0.1:<port>` — the established cross-side HTTP addressing shape;
tier 1's runner-spawned ports get the same addr split per side).

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

`unhealthy_threshold: 1` / `healthy_threshold: 1` gives single-probe
convergence in both directions — needed because arm (b) must observe a clean
transition from 10/10 healthy down to 5/10 healthy without a multi-probe
debounce delay.

## The two arms (both run inside `AssertStats`, the only hook holding both admin addrs)

Per the `reference_differential_asserter_dispatch` / `reference_health_check_
propagation_warmup` precedent (0066/0095), the poll-to-converge + warmup +
load + assert sequence for BOTH arms lives inside `AssertStats` — the `Drive`
hooks only stash listener addrs and return the fixed `"READY\n"` byte stream
for the runner's `CompareBytes` gate.

**arm (a) — static (all 10 hosts healthy).** Poll `cluster.c_pri.
membership_healthy == 10` on both sides, warm up until the data path
stabilizes, then send `staticLoadCount = 300` `GET /` requests per side and
classify each response body as tier "0" or "1". With tier 0 fully healthy, the
priority waterfall sends 100% of traffic to tier 0 and 0% to tier 1
(`capacitySum = 100 + 100 = 200 >= 100`, so the AMEND-P1 capacity-shortfall
bypass does NOT engage — SPEC §8.1/§11.1 scenario (a)).

**arm (b) — full failover.** The driver calls `SetHealthy(false)` on **all 5**
of tier 0's `toggleResponder`s — unlike 0095's partial 3-of-5 degradation
shift, this fixture proves a HARD 100%/0% failover, not a statistical share
shift. Poll `cluster.c_pri.membership_healthy == 5` (0 healthy in tier 0 + all
5 in tier 1) on both sides, re-warm-up, then send `degradedLoadCount = 300`
MORE requests per side. With tier 0 at 0% healthy, the waterfall flips: 0% of
traffic on tier 0, 100% on tier 1. `capacitySum = 0 + 100 = EXACTLY 100` — the
confirmed boundary that does **not** trigger the AMEND-P1 bypass (SPEC §11.1
scenario (f)): it is a genuine tier-1 takeover, not a flattened spray across
all 10 hosts.

Arm (b) MUST run strictly after arm (a) in the same `AssertStats` invocation —
the failover trigger (`SetHealthy(false)`) is a one-way, in-process mutation of
the shared `toggleResponder`s, so the two arms cannot be reordered or split
across separate driver instances.

## Why HARD boundaries, not statistical bands

0095's locality-weighted LB spreads load *proportionally* across localities,
so its per-arm assertion is necessarily a `±bandPct` statistical band around a
predicted share (the underlying RNG selection makes an exact split
infeasible). The priority LB waterfall is different: it is a **deterministic
selector**, not a weighted spread — every request is routed to the highest
non-empty-capacity tier, full stop. There is no RNG-driven blending between
tiers once a tier's capacity crosses the bypass threshold. That makes both
arms' expected split an exact, deterministic boundary (100%/0% and 0%/100%),
not a band: any request landing on the "wrong" tier in either arm is a
correctness bug, not sampling noise. `expectations.yaml` and `AssertStats`
(Task 9) therefore assert **hard equality** (`tier0Count == totalCount` /
`tier1Count == totalCount`), not a `±pp` margin.

## Cross-side deterministic stats (both sides, both arms)

| stat                                   | assertion                                        |
|------------------------------------------|----------------------------------------------------|
| `cluster.c_pri.membership_total`       | `== 10` always (filtering, not removal)            |
| `cluster.c_pri.membership_healthy`     | `== 10` (arm a) / `== 5` (arm b, post-failover)    |
| `cluster.c_pri.upstream_rq_total`      | `>= 600` (`staticLoadCount + degradedLoadCount`)   |

Plus the "decode ran" guard (`reference_docker_probe_bridge_network`):
`cluster.c_pri.upstream_rq_total > 0` on the reference side before trusting
the readout.

## `response-body` scope

Byte-exact comparison applies ONLY to the fixed `"READY\n"` `Drive` stream
(address-independent). The load-phase `GET /` bodies are tallied for the
per-side HARD tier-split assertions inside `AssertStats`, NOT byte-compared —
see `expectations.yaml` for the full rationale.

## NOT exercised by this fixture

Deliberately, per SPEC §8.1's own scope discipline:

- The **AMEND-P1 capacity-shortfall bypass** — neither arm's capacity sum ever
  drops below 100 (arm (a) is 200, arm (b) is exactly 100, the confirmed
  non-triggering boundary).
- The **AMEND-P1-COROLLARY per-tier no-local-panic property** — neither arm
  has an asymmetrically *partially*-degraded tier.

Both are covered instead by dedicated UNIT tests
(`internal/cluster/priority_test.go`, Tasks 5/7), which exercise the full
SPEC §11.4 scenario table (including partial-degradation and
below-100-capacitySum cases) exactly, not just the two hard-boundary arms this
live differential probes.

## Non-additions

- **NO new `BackendKind`** — tier 1 reuses `HTTPEcho`; tier 0 is driver-owned
  per `reference_differential_fixture_dispatch_constraint`.
- **NO separate boot-reject cross-side dir** (SPEC §8.2/AMEND-P2) — both
  composition rejects (duplicate-priority-values-with-conflicting-tier-shape /
  the manager.go THIRD wrap-after-switch rejects) are envoy-go-strict-only
  departures with no reference-Envoy analog to differential against; they are
  unit-tested at `internal/cluster/manager_test.go` (Task 6).

## Status: Tasks 8-10 complete

Task 8 landed the topology, bootstrap builders, `Drive*` hooks, `ProbeAdmin`,
and `classifyBody`. Task 9 added `AssertStats` (poll/warmup/load/tally helpers
+ the two-arm sequence + the cross-side stat assertions). Task 10 ran the
fixture live against Docker + the real `envoyproxy/envoy:contrib-v1.37.2`
reference image, proved both hard-split assertions are LIVE via the SPEC
§8.1 deliberate-break protocol, found and fixed a genuine residual flake via
a ≥20-run flake check, and confirmed `-race` clean. See "Task 10" below.

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0096'` (NOT `-run '0096'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available
(the reference image `envoyproxy/envoy:contrib-v1.37.2`).

## Task 10: deliberate-break liveness + the ≥20-run flake check

Per `reference_differential_break_protocol_count1`: every break below was run
with `-count=1` (never relying on Go's test cache) via
`-run 'TestDifferential/0096'`, observed FAILING, then undone with
`git restore` (breaks in `internal/cluster/manager.go`) or a manual inverse
edit (the break in this fixture's own `driver.go`, to avoid clobbering the
Task 10 flake-fix landed in the same file — see below), and re-confirmed
PASS.

### Break (i) — skip the priority-tier capture (defeats arm (a)'s hard split)

Edit (`internal/cluster/manager.go`, `extractEndpoints`): drop `Priority:
priority` from the `Endpoint{...}` literal (leaving every endpoint at the
zero-value tier 0), adding `_ = priority` to keep the file compiling.

```
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 -v 2>&1 | tail
```

Observed FAILURE (not the literal `assertHardSplit` line the SPEC brief
sketched, but a closely-matching, directly-caused symptom: with every
endpoint forced to tier 0, `distinctPriorities` reports a single tier, the
`priorityLB` wrap never fires, and the cluster falls back to plain
ROUND_ROBIN over all 10 hosts — so the data path never stabilizes to an
all-tier-0 steady state and the WARMUP gate itself times out first):

```
runner_test.go:1294: arm(a) warmup: subject: data path did not stabilize to 60 consecutive non-degraded 200s within 15s
--- FAIL: TestDifferential (16.94s)
    --- FAIL: TestDifferential/0096-lb-priority (16.93s)
```

`git restore internal/cluster/manager.go`; re-run: PASS, `git status --short`
clean.

### Break (ii) — skip the health-degradation step in arm (b)

Edit (this fixture's `driver/driver.go`, `AssertStats`): comment out
`r.SetHealthy(false)` inside arm (b)'s tier-0 fail-over loop (adding `_ = r`
to keep the file compiling), leaving `failedBodies` population intact.

```
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 -v 2>&1 | tail
```

Observed FAILURE (tier 0 never actually fails its health check, so
`membership_healthy` never drops — the earliest gate arm (b) hits, before
`assertHardSplit` even runs, but directly caused by the break and equally
conclusive):

```
runner_test.go:1294: arm(b) converge: reference: cluster.c_pri.membership_healthy did not converge to 5 within 30s (last seen 10)
--- FAIL: TestDifferential (32.30s)
    --- FAIL: TestDifferential/0096-lb-priority (32.29s)
```

Manually reverted (the exact inverse edit, NOT `git restore` — this file also
carries the Task 10 `warmupStable` fix below); re-run: PASS, `git diff`
against HEAD showed only the intentional `warmupStable` change.

### The ≥20-run flake check found — and fixed — a genuine residual flake

The first `-count=20` batch run against the Task 9 baseline (before any Task
10 changes) was NOT clean: 1 failure in 20, reproduced again over further
batches (about 1 failure per 20-60 runs total), always:

- on the **reference side only** (never the subject),
- in **arm (a)** only (the static, all-healthy split), never arm (b),
- `assertHardSplit`'s `tally.t1 != 0` check firing with a small,
  run-to-run-variable count (3, 4, 9, 17, 22, 27, 29 observed across
  different runs — the same 9-54/300 range this file's `tier1BackendBodies`
  doc comment already flagged as a *pre-Task-9* symptom of the same
  underlying class of gap).

Diagnostic instrumentation (temporary, not committed — scraping
`cluster.c_pri.health_check.{failure,success,degraded}`,
`outlier_detection.ejections_total`, and `membership_{healthy,degraded}`
immediately before and after arm (a)'s load, plus per-request tally+body
logging inside `loadAndTally`) was added live to pin down the mechanism:

- `health_check.failure` stayed at **0** in every run, including failing
  ones, and `membership_healthy` stayed at **10** throughout — conclusively
  **ruling out health-check flapping**.
- The leaked requests were tightly **clustered at the very start of the
  300-request load phase** (observed indices 0, 2, 3 in one captured run) —
  i.e., immediately *after* `warmupUntilStable`'s own 10-consecutive-success
  streak had just completed.

Two plausible fixes were tried live and **discarded** because they did not
reduce the failure rate over further ≥20-run batches:

- Widening the health-check `timeout` alone (0.2s → 2s) — made it *worse*
  (2/20), likely because `timeout > interval` lets probes pile up on the
  driver-owned toggleResponder.
- Widening `interval`+`timeout` proportionally (1s/0.9s) and separately
  pinning a long `dns_refresh_rate` (300s, to rule out STRICT_DNS periodic
  re-resolution, the only structural difference between the reference's
  STRICT_DNS cluster and the subject's STATIC one) — neither reduced the
  leak rate.

**The fix that worked:** `warmupStable` (the K-consecutive-non-tier1
threshold `warmupUntilStable` requires before the tallied load begins) was
raised from **10 to 60** in `driver/driver.go`. This widens the SAME
mechanism Task 9 already built (not a new one), giving the still-settling
selection state — a narrower instance of the same class of gap
`reference_health_check_propagation_warmup` describes — much more real
time+attempts to finish settling before the tallied load starts. Confirmed
clean over **4 further `-count=20` batches (80 invocations total, 80/80
PASS)** after the fix landed, plus the breaks above re-verified against this
same fixed state.

### Final ≥20-run flake check (post-fix)

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=20 -v 2>&1 | tail
```

Result: 20/20 PASS (one of 4 clean `-count=20` batches run after the
`warmupStable` fix; see above).

### `-race`

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -race -count=1 -v 2>&1 | tail
```

Result: PASS, no race (`toggleResponder.healthy` is an `atomic.Bool`;
`priDriver`'s stashed fields are `sync.Mutex`-guarded).

### Constant-sync guard

```bash
grep -n "staticLoadCount\|degradedLoadCount\|tier0Hosts\|tier1Hosts" test/fixtures/0096-lb-priority/driver/driver.go
```

Confirmed: every workload count (`staticLoadCount`, `degradedLoadCount`,
`tier0Hosts`, `tier1Hosts`) is a named constant, referenced everywhere it is
used — never a re-derived literal
(`reference_fixture_workload_constant_desync`).
