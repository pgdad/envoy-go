# 0074-circuit-breaker-max-requests

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_cb` (`lb_policy: ROUND_ROBIN`) with a **`circuit_breakers` `max_requests`**
threshold over a SINGLE endpoint — a `BlockingHoldResponder` (BackendKind 36)
that **holds** each `GET /` request open until the driver releases it via
`GET /__release` — on BOTH sides (the 0066/0069 HTTP shape: reference
`STRICT_DNS` / `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

It proves that the **`(max_requests + 1)`th concurrent in-flight request is
rejected with 503** + increments the cluster `upstream_rq_pending_overflow`
counter, and that `circuit_breakers.default.rq_open` tracks the open breaker
(**1** while the budget is full, **0** after release) — on BOTH the envoy-go
(subject) side and the reference-Envoy side.

Phase 41 SPEC §8 / PLAN Task 10.

## Topology: 1 BlockingHoldResponder (runner-spawned)

| endpoint | backing                              | role                                              |
|----------|--------------------------------------|---------------------------------------------------|
| 0        | runner BlockingHoldResponder backend0 | holds `GET /` until `/__release`, then 200 `backend-0:` |

`BackendCount()` returns **1**; the uniform `BackendKind()` is
`BlockingHoldResponder` (NO `PerHostBackendKind`).

## Circuit-breaker config (identical on both sides — NAT-transparent static config)

```yaml
circuit_breakers:
  thresholds:
    - priority: DEFAULT
      max_requests: 4
```

Only `max_requests` is enforced (phase 41 AMEND-CB1); the other budgets
register-for-parity-but-defer.

## The driver: fill-the-budget + probe + release (the determinism mechanism)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The budget-fill, the over-budget probe, and the release ALL run inside
`AssertStats`, **sequentially per side** (subject **fully**, then reference — the
shared in-process backend is idle between sides, so there is no cross-side release
interference). The `Drive` hooks stash their listener addrs and return a fixed,
address-independent byte stream (`"READY\n"`) for the runner's `CompareBytes`
gate; the config builders stash the backend host port for the `/__release` hit.

For each side (listener `listenerAddr`, admin `adminAddr`):

1. **Fill the budget** — fire `maxRequests` (4) **concurrent** `GET /` (each
   blocks at the `BlockingHoldResponder`), capturing each outcome via a
   `sync.WaitGroup`.
2. **Poll phase** — scrape `/stats` until
   `cluster.c_cb.circuit_breakers.default.rq_open == 1` (deadline 10s, poll every
   50ms). **NO fixed `time.Sleep`** — poll until the predicate holds or the
   deadline trips (fail with the last-seen value). This confirms all 4 slots are
   filled (the breaker is open).
3. **Over-budget probe** — baseline `upstream_rq_pending_overflow`, then fire the
   **`(N+1)`th** `GET /`. The breaker is full, so the proxy rejects it **before**
   the backend → assert **status 503**.
4. **Re-scrape** — `rq_open == 1` (still open) AND
   `(upstream_rq_pending_overflow - baseline) >= 1`; plus the **"decode ran"
   guard** `upstream_rq_total > 0`.
5. **Release** — `GET /__release` on `127.0.0.1:<backendPort>` (the **backend**
   control port, NOT the proxy listener — always loopback, the backend is
   in-process on this machine for both sides). This frees the 4 held requests.
6. **Drain** — `wg.Wait()`: the 4 held requests now return **200** `backend-0:`.
   Assert all 4 got 200. Poll `rq_open -> 0` (the breaker closes).

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                                  | assertion                  | meaning                                 |
|-------------------------------------------------------|----------------------------|-----------------------------------------|
| `cluster.c_cb.circuit_breakers.default.rq_open`       | `== 1` (full) → `== 0` (drained) | the breaker opens with the full budget, closes after drain |
| `cluster.c_cb.upstream_rq_pending_overflow` (delta)   | `>= 1`                     | the overflow counter increments on the rejected request |
| `cluster.c_cb.upstream_rq_total`                      | `> 0`                      | "decode ran" guard (reference forwarded traffic) |

Plus the data-path proof: the `(N+1)`th request status `== 503`, the `N` held
requests status `== 200` with `backend-0:` bodies.

## Why it is robust (no fixed sleep, sequential sides)

The `rq_open == 1` poll is the **synchronization barrier**: it proves all
`max_requests` slots are occupied (so the breaker is genuinely full) before the
over-budget probe fires — never a fixed sleep. The two sides run **sequentially**
(subject fully, including the release + `rq_open -> 0` drain, before the reference
fires), so the single shared in-process backend is clean between sides — the
subject's held requests are all released before the reference's fire, and the
`/__release` re-arms the backend's gate for the next batch.

## Deliberate non-assertions (recorded departures)

- **`UO` access-log response flag** is NOT asserted (**D-S41-3**) — envoy-go has
  no access-log response-flag plumbing. The 503-status + the
  `upstream_rq_pending_overflow` / `rq_open` stats pair is the proof.
- **`upstream_rq_5xx`** is NOT asserted — the overflow 503 is a local reply; the
  subject does not increment the cluster 5xx counter on the rejected path (a
  cross-side mismatch the fixture avoids).

## Non-additions

- **NO new BackendKind authored here** — the `BlockingHoldResponder` (= 36) was
  added at PLAN Task 9; this fixture only consumes it via the uniform
  `BackendKind()`.
- **NO `DistributionAsserter`** — the single-host topology + the circuit-breaker
  counters carry the proof (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the `circuit_breakers` config is static YAML; the
  parse/threshold/admission logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `circuit_breakers` config-reject arms land
  UNIT-LEVEL in `internal/cluster` (`parseCircuitBreakers`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0074' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0074'` (NOT `-run '0074'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).
