# 0078-connection-pool-max-connections

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_cp` (`lb_policy: ROUND_ROBIN`) with a **`circuit_breakers`
`max_connections` + `max_pending_requests`** threshold over a SINGLE endpoint — a
`BlockingHoldResponder` (BackendKind 36) that **holds** each `GET /` request open
until the driver releases it — on BOTH sides (the 0066/0069/0074 HTTP shape:
reference `STRICT_DNS` / `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

It proves the connection-pool **CONNECTION budget**: the `max_connections`
**HARD-CAP** + the `max_pending_requests` **bounded wait-queue** — on BOTH the
envoy-go (subject) side and the reference-Envoy side.

Phase 43.1 SPEC / PLAN Task 8.

## The soft-breaker DEPARTURE (`reference_max_connections_soft_breaker`)

The reference Envoy's `max_connections` is a **SOFT breaker** — `upstream_cx_active`
can **exceed** the cap with timing slop. A cross-side **EXACT** connection-pool
differential is therefore **infeasible**. envoy-go implements a **clean HARD-CAP +
bounded-queue DEPARTURE** (ADR-0252). The fixture splits into **two prongs**, run
**sequentially per side** inside `AssertStats` (subject **fully**, then reference;
the shared in-process backend is idle between sides):

| prong | side | assertion class |
|-------|------|-----------------|
| EXACT | subject (envoy-go) | `cx_active` never exceeds `N`; queue peaks at `M`; **exactly** `J` overflow 503s; `upstream_rq_pending_total == M`; overflow delta `== J` |
| ROBUST | reference (envoy) | `upstream_rq_pending_overflow` delta `>= 1`; `>= 1` downstream 503; decode-ran guard `upstream_cx_total > 0`; final gauges settle at 0. (`cx_open` + `upstream_cx_overflow` NOT asserted — soft-breaker racy) |

## Topology: 1 BlockingHoldResponder (runner-spawned)

| endpoint | backing | role |
|----------|---------|------|
| 0 | runner BlockingHoldResponder backend0 | holds `GET /` until a release, then 200 `backend-0:` |

`BackendCount()` returns **1**; the uniform `BackendKind()` is
`BlockingHoldResponder`. The BackendKind tail is **UNCHANGED at 36** (REUSE).

## Circuit-breaker config (identical on both sides — NAT-transparent static config)

```yaml
circuit_breakers:
  thresholds:
    - priority: DEFAULT
      max_connections: 2       # N — the HARD-CAP
      max_pending_requests: 2  # M — the bounded wait-queue depth
```

Workload constants (single-sourced in the driver —
`reference_fixture_workload_constant_desync`): `N = 2`, `M = 2`, `J (oversub) = 2`,
`refOversub = M + J + 4 = 8` (the reference soft-breaker slack).

## The driver: staged drive (fill / pend / oversub / sticky-drain)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv `"READY\n"`
stream, run **first**) then `AssertStats` (run **last**, the only hook holding
**both** admin addrs). The fill / pend / oversub / drain ALL run inside
`AssertStats`. The `Drive` hooks stash listener addrs; the config builders stash
the backend host port for the `/__release_sticky` hit.

### SUBJECT (exact) per side

1. **Saturate** — fire `N` concurrent held `GET /`; poll
   `circuit_breakers.default.cx_open == 1` **and** `upstream_cx_active == N`.
2. **Fill the queue** — fire `M` further held `GET /` (they **pend**); poll
   `circuit_breakers.default.rq_pending_open == 1` **and**
   `upstream_rq_pending_active == M` **and** `upstream_cx_active == N` (the hard
   cap — `cx_active` must **never** exceed `N`).
3. **Oversubscribe** — fire `J` more `GET /`; each finds the queue full → **503**
   (counted from the **downstream** status codes, NOT `upstream_rq_5xx` —
   `reference_concurrent_attempt_downstream_class_assertion`).
4. **Re-scrape (exact)** — `cx_active == N`; `rq_pending_active == M`;
   `got503 == J`; `upstream_rq_pending_overflow` delta `== J`;
   `upstream_rq_pending_total == M` **exactly**; `upstream_cx_total > 0`.
5. **Sticky drain** — `GET /__release_sticky` on the backend → the `N` held + the
   `M` woken (which dial **fresh** connections) drain to **200** `backend-0:`;
   poll `cx_open == 0` **and** `rq_pending_open == 0`.

### REFERENCE (robust) per side

The reference `max_connections` is a **SOFT breaker**
(`reference_max_connections_soft_breaker`): `cx_active` can **exceed** `N`, the
`circuit_breakers.default.cx_open` **gauge** does not reliably latch (only
momentarily set while admitting a connection over the cap — a 50ms poll misses
it), and the `upstream_cx_overflow` **counter** is timing-racy (it depends on the
connection-establishment-vs-request-arrival race — observed `0` on some runs while
`upstream_rq_pending_overflow` always fired). So **neither `cx_open` nor
`upstream_cx_overflow` is asserted on the reference** — only the reliably
observable signals.

1+3. **Burst** — fire the **full** burst (`N` + `refOversub`) concurrently. The
   `N` saturate the cap; the surplus drives the bounded pending-queue overflow.
   Poll `upstream_rq_pending_overflow` delta `>= 1` (the reliably-observable
   robust signal; the soft breaker needs the heavier oversubscription to
   **guarantee** overflow).
4. **Re-scrape (robust)** — `upstream_rq_pending_overflow` delta `>= 1`;
   `upstream_cx_total > 0` (decode-ran guard,
   `reference_docker_probe_bridge_network`).
5. **Sticky drain** — `GET /__release_sticky` → drain; poll `cx_open == 0` **and**
   `rq_pending_open == 0` (gauges settle); assert `>= 1` downstream 503 across the
   fired burst.

## The sticky-release control path (D-S431-5)

The `M` woken pending waiters dial **fresh** connections after the `N` held conns
release. The re-armable `/__release` (0074) would re-block those fresh dials on a
new gate → the drain would **stall** (`cx_open` never returns to 0). 0078 uses the
**STICKY** `/__release_sticky` path on the `BlockingHoldResponder` (an **additive**
control path on BackendKind 36 — the tail STAYS 36, **NO new kind**): it
**permanently** opens the gate so all current AND future requests pass
immediately. 0074's `/__release` is **unchanged** (byte-stable).

## Why it is robust (no fixed sleep, sequential sides)

Every barrier is a **poll-to-converge** on the admin `/stats` (never a fixed
`time.Sleep` — `reference_concurrency_differential_release_barrier`): the `cx_open
== 1` + `cx_active == N` poll proves the pool is genuinely saturated before the
pend step; the `rq_pending_open == 1` poll proves the queue is full before the
oversub step. The two sides run **sequentially** (subject fully, including the
sticky drain back to gauges-at-0, before the reference fires), so the single
shared in-process backend is clean between sides.

## Deliberate non-assertions (recorded departures)

- **Cross-side EXACT `cx_active`** is NOT asserted on the reference (the soft
  breaker overshoots the cap — `reference_max_connections_soft_breaker`).
- **The reference `cx_open` gauge + `upstream_cx_overflow` counter** are NOT
  asserted on the reference — both are timing-racy under the soft `max_connections`
  (the connection-cap signals do not reliably latch / increment). The reliably
  observable `upstream_rq_pending_overflow` + downstream 503 carry the robust proof.
- **`upstream_rq_5xx`** is NOT asserted — the overflow 503 is a local reply; the
  503 detection observes the **downstream** status codes of the fired requests.
- **The `UO` access-log response flag** is NOT asserted — envoy-go has no
  response-flag plumbing.

## Non-additions

- **NO new BackendKind** — the `BlockingHoldResponder` (= 36) is REUSED, with the
  additive `/__release_sticky` control path.
- **NO `DistributionAsserter`** — the single-host topology + the connection-pool
  counters carry the proof (`reference_differential_asserter_dispatch`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0078' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0078'` (NOT `-run '0078'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).
