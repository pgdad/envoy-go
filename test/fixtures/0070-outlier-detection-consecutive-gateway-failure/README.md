# 0070-outlier-detection-consecutive-gateway-failure

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_gw` (`lb_policy: ROUND_ROBIN`) with **passive outlier detection** over THREE
endpoints — **2 HEALTHY** HTTPEcho backends + **1 always-503** host (the
`HTTP503Responder`, BackendKind 35) — on BOTH sides (the 0069 HTTP shape:
reference `STRICT_DNS` / `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

It proves that an upstream host returning consecutive **gateway-class** 5xx (503)
responses is **detected + ejected by the `consecutive_gateway_failure`
detector** on BOTH the envoy-go (subject) side and the reference-Envoy side. It
ALSO proves the **gateway-first short-circuit**: with `consecutive_5xx` set HIGH
(100) the `consecutive_5xx` detector cannot fire, and a gateway eject
short-circuits before the 5xx detector runs, so **`detected_consecutive_5xx ==
0`** and **`enforced_consecutive_5xx == 0`** cross-side.

After ejection the healthy fraction is 2/3 ≈ 66% > the 50% panic threshold, so
the load lands **exclusively** on the 2 healthy backends.

Phase 40.2 SPEC §10 / PLAN Task 7. Sibling of **0069** (same flow; 0069 ejects
via the `consecutive_5xx` detector, 0070 ejects via the
`consecutive_gateway_failure` detector).

## Topology: 2 HEALTHY + 1 ALWAYS-503 (all runner-spawned)

| endpoint | backing                          | state      | role                          |
|----------|----------------------------------|------------|-------------------------------|
| 0        | runner HTTPEcho backend0         | HEALTHY    | 200s `/`; serves load         |
| 1        | runner HTTPEcho backend1         | HEALTHY    | 200s `/`; serves load         |
| 2        | runner HTTP503Responder backend2 | ALWAYS-503 | 503s every request → ejected  |

ALL THREE hosts are runner-spawned **live** listeners; host2 answers 503 and is
ejected by the **passive** `consecutive_gateway_failure` detector.
`BackendCount()` returns **3**; the runner selects host2's kind via
`PerHostBackendKind` (`BackendKindAt(2)` → `HTTP503Responder`).

## Outlier-detection config (identical on both sides — NAT-transparent static config)

```yaml
outlier_detection:
  consecutive_gateway_failure: 5
  enforcing_consecutive_gateway_failure: 100
  consecutive_5xx: 100          # SET HIGH — the consecutive_5xx detector cannot fire
  interval: 10s
  base_ejection_time: 30s
  max_ejection_percent: 100
```

`consecutive_5xx: 100` makes the 5xx detector unable to reach its threshold over
the ~24-request eject-drive, so the **gateway** detector is the sole ejection
trigger — and the gateway-first short-circuit leaves the 5xx counters at 0.
`max_ejection_percent: 100` allows the single 503 host to be ejected; the
`interval`/`base_ejection_time` are parse-accepted (recovery DEFERRED).

## The driver: eject-drive + poll-to-converge + warmup (the 0069 template)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The `Drive` hooks stash their listener addrs and return a fixed,
address-independent byte stream (`"READY\n"`) for the runner's `CompareBytes`
gate.

1. **Ejection drive** — send `ejectDriveRequests` (24) 503-**tolerant** `GET /`
   round-robin to each side. Under strict round-robin over 3 hosts host2 (the
   503) is picked every 3rd request; `consecGw` is **per-host** and **never
   reset** (no 2xx ever comes FROM host2), so host2 accrues consecutive
   gateway-class 5xx until it crosses `consecutive_gateway_failure` (5) —
   roughly `5 * 3 = 15` requests; the 24 count carries a margin and stays far
   below `consecutive_5xx` (100).
2. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_gw.outlier_detection.ejections_active == 1` (deadline 30s, poll
   200ms). **NO fixed `time.Sleep`** — poll until the predicate holds.
3. **Warmup phase** — after the gauge reads 1, send 503-tolerant `GET /` until
   `warmupStable` (10) CONSECUTIVE 200s prove the worker rotation has dropped
   host2, on BOTH sides (the `reference_health_check_propagation_warmup` gate).
4. **Measured load phase** — baseline the per-request counters post-warmup, send
   `n = 60` `GET /` on each side; assert (delta) `upstream_rq_2xx == 60`,
   `upstream_rq_5xx == 0`, every body `backend-0:`/`backend-1:` (NEVER
   `backend-2:`), both healthy hosts touched.
5. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                                                          | assertion | meaning                                |
|-------------------------------------------------------------------------------|-----------|----------------------------------------|
| `cluster.c_gw.outlier_detection.ejections_active`                             | `== 1`    | host2 ejected and held                 |
| `cluster.c_gw.outlier_detection.ejections_enforced_total`                     | `>= 1`    | an ejection was enforced               |
| `cluster.c_gw.outlier_detection.ejections_detected_consecutive_gateway_failure` | `>= 1`  | detected via the gateway detector      |
| `cluster.c_gw.outlier_detection.ejections_enforced_consecutive_gateway_failure` | `>= 1`  | enforced via the gateway detector      |
| `cluster.c_gw.outlier_detection.ejections_detected_consecutive_5xx`           | `== 0`    | the 5xx detector NEVER fired (short-circuit) |
| `cluster.c_gw.outlier_detection.ejections_enforced_consecutive_5xx`           | `== 0`    | the 5xx detector NEVER enforced        |
| `cluster.c_gw.upstream_rq_2xx` (delta, measured phase)                        | `== 60`   | all measured load routed to a healthy host |
| `cluster.c_gw.upstream_rq_5xx` (delta, measured phase)                        | `== 0`    | no 5xx in the measured phase           |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_gw.upstream_rq_total > 0` on the reference side before trusting the
readout.

The gateway enforced/detected counters use `>= 1` (the exact count can differ
cross-side by detection timing); the `ejections_active == 1` gauge is the
exact-parity anchor. The `consecutive_5xx` counters use `== 0` — a **live
equality** that bites if the gateway-first short-circuit regressed.

## Why `detected_consecutive_5xx == 0` is a LIVE assertion

A 503 is gateway-class, so `recordExternal5xx` runs the gateway detector first; a
gateway eject `return`s before the 5xx detector runs (see
`internal/cluster/outlier.go` `recordExternal5xx`). Combined with
`consecutive_5xx: 100` (unreachable over the eject-drive), the 5xx
detected/enforced counters MUST be exactly 0. If the short-circuit regressed (the
5xx detector also firing), `detected_consecutive_5xx` would lift off 0 and the
equality would fail — the check is not vacuous.

## Deliberate non-assertions

- **Recovery / un-eject arm** (`ejections_active` → 0 after `base_ejection_time`)
  is **DEFERRED** — the lazy (subject) vs sweep (reference) un-eject timing
  diverges cross-side (**AMEND-OD1**).

## Non-additions

- **NO new BackendKind authored here** — the `HTTP503Responder` (= 35) was added
  at phase 40.1; this fixture only consumes it via `BackendKindAt(2)`.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the eject-drive + warmup traffic. The per-side 100%-to-healthy
  tally is taken off the response **bodies** inside `AssertStats` instead, and the
  ejection is proven by `upstream_rq_5xx == 0` + the healthy-only body idxs + the
  outlier-detection counters (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the `outlier_detection` config is static YAML; the
  parse/threshold/eject logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `outlier_detection` config-reject arms land
  UNIT-LEVEL in `internal/cluster` (`parseOutlierDetection`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0070' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0070'` (NOT `-run '0070'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).
