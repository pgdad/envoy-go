# 0069-outlier-detection-consecutive-5xx

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_od` (`lb_policy: ROUND_ROBIN`) with **passive outlier detection**
(`consecutive_5xx`) over THREE endpoints — **2 HEALTHY** HTTPEcho backends + **1
always-503** host (the `HTTP503Responder`, BackendKind 35) — on BOTH sides (the
0066 HTTP shape: reference `STRICT_DNS` / `host.docker.internal`, subject
`STATIC` / `127.0.0.1`).

It proves that an upstream host returning consecutive 5xx responses is
**detected by passive outlier detection and ejected from LB rotation** on BOTH
the envoy-go (subject) side and the reference-Envoy side. After ejection the
healthy fraction is 2/3 ≈ 66% > the 50% panic threshold, so the load lands
**exclusively** on the 2 healthy backends.

Phase 40.1 SPEC §8 / PLAN Task 10.

## Topology: 2 HEALTHY + 1 ALWAYS-503 (all runner-spawned)

| endpoint | backing                          | state    | role                                   |
|----------|----------------------------------|----------|----------------------------------------|
| 0        | runner HTTPEcho backend0         | HEALTHY  | 200s `/`; serves load                  |
| 1        | runner HTTPEcho backend1         | HEALTHY  | 200s `/`; serves load                  |
| 2        | runner HTTP503Responder backend2 | ALWAYS-503 | 503s every request → ejected         |

Unlike **0066** (whose unhealthy host is an **unbound** port detected by
**active** health checks), ALL THREE hosts here are runner-spawned **live**
listeners; host2 answers 503 and is ejected by **passive** consecutive-5xx
outlier detection. `BackendCount()` returns **3**; the runner selects host2's
kind via `PerHostBackendKind` (`BackendKindAt(2)` → `HTTP503Responder`).

## Outlier-detection config (identical on both sides — NAT-transparent static config)

```yaml
outlier_detection:
  consecutive_5xx: 5
  interval: 10s
  base_ejection_time: 30s
  max_ejection_percent: 100
```

`max_ejection_percent: 100` allows the single 503 host to be ejected; the
`interval`/`base_ejection_time` are parse-accepted (the recovery arm is DEFERRED
— see below).

## The driver: eject-drive + poll-to-converge + warmup (the determinism mechanism)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The ejection must be driven, observed, and the worker rotation drained
BEFORE the strict measured phase, so it ALL runs inside `AssertStats`. The
`Drive` hooks stash their listener addrs and return a fixed, address-independent
byte stream (`"READY\n"`) for the runner's `CompareBytes` gate.

1. **Ejection drive** — send `ejectDriveRequests` (24) 503-**tolerant** `GET /`
   round-robin to each side's listener. **KEY INSIGHT:** under strict
   round-robin over 3 hosts, host2 (the 503) is picked every 3rd request;
   `consec5xx` is **per-host** and is **never reset** (no 2xx ever comes FROM
   host2), so host2 accrues consecutive 5xx across its picks until it crosses
   `consecutive_5xx` (5) — roughly `5 * 3 = 15` requests; the 24 count carries a
   margin.
2. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_od.outlier_detection.ejections_active == 1` (deadline 30s, poll
   every 200ms). **NO fixed `time.Sleep`** — poll until the predicate holds or
   the deadline trips (fail with the last-seen value on timeout).
3. **Warmup phase** — after the gauge reads 1, send 503-tolerant `GET /` until
   `warmupStable` (10) CONSECUTIVE 200s prove the worker rotation has dropped
   host2, on BOTH sides (closes the main→worker propagation window, the 0066
   `reference_health_check_propagation_warmup` mechanism).
4. **Measured load phase** — baseline the per-request counters post-warmup, send
   `n = 60` `GET /` on each side; assert (delta) `upstream_rq_2xx == 60`,
   `upstream_rq_5xx == 0`, every body `backend-0:`/`backend-1:` (NEVER
   `backend-2:` — the ejected 503 host serves nothing), both healthy hosts
   touched.
5. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                                          | assertion | meaning                                 |
|--------------------------------------------------------------|-----------|-----------------------------------------|
| `cluster.c_od.outlier_detection.ejections_active`            | `== 1`    | host2 ejected and held                  |
| `cluster.c_od.outlier_detection.ejections_enforced_total`    | `>= 1`    | an ejection was enforced                |
| `cluster.c_od.outlier_detection.ejections_enforced_consecutive_5xx` | `>= 1` | enforced via the 5xx detector       |
| `cluster.c_od.outlier_detection.ejections_detected_consecutive_5xx` | `>= 1` | detected via the 5xx detector       |
| `cluster.c_od.upstream_rq_2xx` (delta, measured phase)       | `== 60`   | all measured load routed to a healthy host |
| `cluster.c_od.upstream_rq_5xx` (delta, measured phase)       | `== 0`    | no 5xx in the measured phase            |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_od.upstream_rq_total > 0` on the reference side before trusting the
readout.

The enforced/detected counters use `>= 1` (not `==`) — the exact count can
differ cross-side by detection timing; the `ejections_active == 1` gauge is the
exact-parity anchor.

## Deliberate non-assertions

- **`ejections_detected_consecutive_gateway_failure`** is NOT asserted — envoy-go
  has no gateway detector at 40.1; the reference trips it on 503 (**AMEND-OD4**).
- **Recovery / un-eject arm** (`ejections_active` → 0 after `base_ejection_time`)
  is **DEFERRED** — the lazy (subject) vs sweep (reference) un-eject timing
  diverges cross-side (**AMEND-OD1**).

## Non-additions

- **NO new BackendKind authored here** — the `HTTP503Responder` (= 35) was added
  at PLAN Task 9; this fixture only consumes it via `BackendKindAt(2)`.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the eject-drive + warmup traffic, so they cannot cleanly attribute
  the `n=60` measured requests. The per-side 100%-to-healthy tally is taken off
  the response **bodies** inside `AssertStats` instead, and the ejection is proven
  by `upstream_rq_5xx == 0` + the healthy-only body idxs + the outlier-detection
  counters (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the `outlier_detection` config is static YAML; the
  parse/threshold/eject logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `outlier_detection` config-reject arms land
  UNIT-LEVEL in `internal/cluster` (`parseOutlierDetection`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0069' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0069'` (NOT `-run '0069'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).

## The warmup gate (cross-thread propagation robustness)

Like 0066, the proxy updates the `ejections_active` gauge before the WORKER-thread
LB host-sets drop the ejected host (a small propagation window), so an early
request can still be round-robined to host2 → a transient 503 even after the
gauge reads 1. After the convergence poll, the driver runs a **warmup**: it sends
503-tolerant `GET /` until `warmupStable` (10) CONSECUTIVE 200s prove the worker
rotation has dropped host2, THEN runs the strict measured phase. The per-request
counters (`upstream_rq_2xx`/`_5xx`) are asserted as a **delta** over the measured
phase (baseline scraped post-warmup), so the eject-drive + warmup requests do not
over-count. Round-robin hits host2 every 3rd pick when it is NOT ejected, so an
un-ejected build can never reach 10 consecutive 200s — the gate still bites the
deliberate breaks.
