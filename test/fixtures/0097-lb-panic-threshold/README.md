# 0097-lb-panic-threshold

Cross-side `[http_connection_manager + router]` differential over **one HTTP
listener** (`l_http`) path-routing to **THREE** `STATIC` clusters (`c_pt_a` /
`c_pt_b` / `c_pt_c`), 5 hosts each, each with an **active HTTP health
checker** (path `/healthz`, fast convergence: `interval`/`timeout` `1s`,
thresholds `1`/`1`).

Phase 54 SPEC §10 / PLAN Task 7.

It is the FIRST cross-side proof of the `healthy_panic_threshold` construct:
three clusters degraded to the SAME 60% healthy, differing ONLY in their
configured panic threshold, demonstrating that the threshold — not some other
per-cluster difference — is what flips a cluster's routing behavior between
"panic: serve every host regardless of health" and "no panic: exclude the
unhealthy hosts as usual".

## Topology: 3 clusters x 5 driver-owned toggleable hosts

All 15 hosts across all three clusters are **driver-owned** `toggleResponder`s
(self-managed `net.Listener`s) — the 0095/0096 `toggleResponder` precedent,
extended here to carry a **cluster label** alongside the host index (this
fixture has three clusters, not one/two tiers, so the response body must
identify BOTH dimensions):

- Any path other than `/healthz` -> `200 "<cluster>:<idx>"` (e.g. `c_pt_a:3`)
  — the host-attribution signal `classifyBody` parses for the load-phase
  tally.
- `/healthz` -> `200` while the responder's `atomic.Bool` health flag is
  `true`, `503` once the driver calls `SetHealthy(false)` on it.

Both `ReferenceBootstrap` and `SubjectConfig` call the SAME memoized
`ensureBackends()` so both sides' `load_assignment`s reference the identical
15 ports (reference via `host.docker.internal:<port>`, subject via
`127.0.0.1:<port>`).

`BackendCount()` returns `1`: a single throwaway runner-spawned `HTTPEcho`
backend that **no cluster in this fixture's bootstrap references** — the
established "spawn-but-don't-use" pattern (the runner requires
`BackendCount() >= 1`, `runner_test.go:221`; see the 0018-http-rbac
precedent for a backend spawned but only conditionally used). `BackendKind()`
stays `HTTPEcho` — **no new `BackendKind`** is introduced; the tail stays at
`38` (`H2GoawayResponder`).

## Routing

```yaml
routes:
  - match: { prefix: "/a" }
    route: { cluster: c_pt_a }
  - match: { prefix: "/b" }
    route: { cluster: c_pt_b }
  - match: { prefix: "/c" }
    route: { cluster: c_pt_c }
```

## Health-check config (identical on all three clusters, both sides)

```yaml
health_checks:
  - interval: 1s
    timeout: 1s
    unhealthy_threshold: 1
    healthy_threshold: 1
    no_traffic_healthy_interval: 1s
    http_health_check:
      path: /healthz
```

## The ONLY per-cluster difference: `common_lb_config.healthy_panic_threshold`

| cluster  | `healthy_panic_threshold`        | healthy % | panics? |
|----------|-----------------------------------|-----------|---------|
| `c_pt_a` | `{ value: 80 }`                    | 60%       | **YES** (60 < 80) |
| `c_pt_b` | absent (the 50% reference default) | 60%       | no (60 >= 50) |
| `c_pt_c` | `{ value: 60.9 }`                   | 60%       | no (`floor(60.9)=60`; `60 < 60` is FALSE) |

`c_pt_c` is the **AMEND-PT1 integer-truncation discriminator**: the reference
Envoy floors a `Percent`-typed panic threshold to an INTEGER percent before
comparing (the observed percentage stays a real double, but the *threshold*
gets truncated toward zero). A naive float-fraction compare (`60.0% <
60.9%`) would have panicked here; the correct integer cross-multiply
(`100*3 < 60*5` -> `300 < 300` -> false) does not. This fixture is the
cross-side proof that envoy-go's `parsePanicThreshold` (floor) +
`inPanic`'s integer cross-multiply (`internal/cluster/health.go`) match the
reference bit-for-bit at this exact boundary.

Every cluster is degraded to the SAME **fixed 2-of-5 hosts** (indices `3` and
`4` — `degradedPerCluster`), giving `3/5 = 60%` healthy in every cluster —
the ONLY variable across the three clusters is the panic-threshold
configuration.

## `AssertStats` sequence (both sides, all three arms in-band)

Per the `reference_differential_asserter_dispatch` precedent, the
degrade + poll-to-converge + warmup + load + assert sequence for all three
arms lives inside `AssertStats` — the only hook holding both admin addrs.

1. `ensureBackends()`, then degrade hosts `3` and `4` in EACH of `c_pt_a` /
   `c_pt_b` / `c_pt_c` (toggle `/healthz` to `503`).
2. `pollMembershipHealthy(side, adminAddr, cluster, 3)` for each of the three
   clusters, both sides -> `cluster.<name>.membership_healthy == 3`.
   `membership_healthy` reflects the ACTUAL healthy count regardless of panic
   mode (panic only changes which hosts get PICKED, not which are marked
   healthy) — so this gate converges identically for all three clusters,
   panicking or not.
3. `warmupUntilStable` per cluster path. `c_pt_b`/`c_pt_c` (the no-panic
   arms) exclude the degraded hosts' response bodies — proof the
   health-check-driven exclusion has propagated to the per-worker LB host
   set (`reference_health_check_propagation_warmup`). `c_pt_a` (the panic
   arm) uses NO exclusion: panic mode serves ALL hosts by design, so "wait
   for the degraded hosts to stop appearing" would never converge there — the
   warmup instead just proves the config has settled to a steady state
   before the tallied load begins.
4. Drive `loadPerCluster = 200` `GET` requests to each of `/a`, `/b`, `/c`;
   tally per (cluster, host) via `classifyBody`.
5. `scrapeStats` both admin endpoints.

## Assertions (both sides, exact counts via `assertEq`)

- Decode-ran guard first: `cluster.c_pt_a.upstream_rq_total > 0` on the
  reference side (`reference_docker_probe_bridge_network`).
- **`c_pt_a` PANICS**: every one of the 5 hosts (INCLUDING the 2 degraded)
  has a nonzero tally — offset-invariant (`reference_round_robin_offset_
  randomized`: assert all-hosts->0, NEVER host identity/sequence, since the
  reference randomizes the RR initial offset).
  `cluster.c_pt_a.lb_healthy_panic == loadPerCluster` (200 — the counter
  increments once per pick while panicking, SPEC §10/D-PT1(iii)).
- **`c_pt_b` NO panic**: the 2 degraded hosts (`3`, `4`) have tally `== 0`.
  `cluster.c_pt_b.lb_healthy_panic == 0`.
- **`c_pt_c` NO panic** (the integer-truncation discriminator): the 2
  degraded hosts have tally `== 0`. `cluster.c_pt_c.lb_healthy_panic == 0`.
- `cluster.c_pt_{a,b,c}.membership_healthy == 3` on both sides.

## Why cluster names are DISTINCT (`c_pt_a`/`c_pt_b`/`c_pt_c`)

Per `reference_admin_interface_wire_name_collision`: three separate cluster
names give three separate `/stats` wire-name prefixes
(`cluster.c_pt_a.*`/`cluster.c_pt_b.*`/`cluster.c_pt_c.*`), so each arm's
assertions read an unambiguous, per-cluster stat — no shared/collapsed wire
name across clusters to disambiguate.

## `response-body` scope

Byte-exact comparison applies ONLY to the fixed `"READY\n"` `Drive` stream
(address-independent). The load-phase `GET /a|/b|/c` bodies are tallied for
the per-side per-host assertions inside `AssertStats`, NOT byte-compared — a
randomized RR selection makes cross-side per-request host identity
infeasible (`reference_round_robin_offset_randomized`).

## NOT exercised by this fixture

Deliberately, per SPEC §10's own scope discipline:

- The locality/subset double-increment + retrofit interactions with panic
  mode — those are UNIT-tested (SPEC §8.2 / `internal/cluster/priority_test.
  go`, `internal/cluster/locality_test.go`).
- The out-of-range `healthy_panic_threshold` boot-reject
  (`validatePanicThresholdRange`, `[0,100]`) — a `manager_test.go` UNIT test,
  not a separate cross-side boot-reject fixture directory (no reference-Envoy
  differential is needed to prove a pure config-validation reject; envoy-go's
  own PGV-mirroring range check is exercised directly against the parsed
  proto).

## Non-additions

- **NO new `BackendKind`** — the tail stays at `38` (`H2GoawayResponder`);
  all 15 real hosts are driver-owned, and the one runner-spawned backend is
  the unreferenced `HTTPEcho` throwaway.
- **NO separate boot-reject cross-side dir** — see above.

## Run it

```bash
go test ./test/fixtures/0097-lb-panic-threshold/driver/ -run 'TestClassifyBody|TestConstants' -count=1
go test ./test/differential/ -run 'TestDifferential/0097' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0097'` (NOT `-run '0097'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available
(the reference image `envoyproxy/envoy:contrib-v1.37.2`).

## Deliberate-break protocol record (Task 8, controller-verified, `-count=1`)

All three breaks were applied to the frozen HEAD, run with `-count=1`, then reverted via `git restore` (`reference_differential_break_protocol_count1`):

- **Break (a) — hardcode `parsePanicThreshold` to `return 50`** (ignore the field). FAILED as required: c_pt_a (60% healthy) no longer panics at threshold 50, so `subject c_pt_a host 3/4 got 0 -- panic must serve ALL hosts` AND `cluster.c_pt_a.lb_healthy_panic delta (post-load 0 - baseline 0) = 0, want 200`. Proves the A-arm panic + delta assertions are live.
- **Break (b) — no-op `degradeAll`** (skip the per-cluster degradation). FAILED as required: `converge: reference: cluster.c_pt_a.membership_healthy did not converge to 3 within 30s (last seen 5)`. Proves the degradation + poll-to-converge gate is live.
- **Break (c) — revert the AMEND-PT1 floor (`math.Floor` → `math.Round`, so `60.9 → 61`)**. FAILED as required: c_pt_c wrongly panics (300 < 61*5=305), so `warmup: subject: /c did not stabilize to 60 consecutive non-degraded 200s within 15s` (a panicking c_pt_c keeps serving its degraded hosts). This is THE differential proof of the integer-truncation fix (SPEC §8.1 break (c)).

Soak: **20/20** flake-free (`-count=20`, 63s). `-race` clean (`-count=1`).
