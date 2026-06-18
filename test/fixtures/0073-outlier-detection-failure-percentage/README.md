# 0073-outlier-detection-failure-percentage

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_fp` (`lb_policy: ROUND_ROBIN`) with **statistical** outlier detection
(`failure_percentage`) over SIX endpoints — **5 HEALTHY** HTTPEcho backends + **1
always-503** host (the `HTTP503Responder`, BackendKind 35) — on BOTH sides (the
0069 HTTP shape: reference `STRICT_DNS` / `host.docker.internal`, subject
`STATIC` / `127.0.0.1`).

It proves that an upstream host whose **failure percentage crosses the configured
threshold** (the bad host's 100% failure vs the 85% `failure_percentage_threshold`)
is **detected and ejected via the failure_percentage detector** on BOTH the
envoy-go (subject) side and the reference-Envoy side. After ejection the healthy
fraction is 5/6 ≈ 83% > the 50% panic threshold, so the load lands
**exclusively** on the 5 healthy backends.

This is the **symmetric twin of 0072** (success_rate enforces): same topology,
but the **enforcer is flipped** — here `failure_percentage` is the SOLE enforcer
(`enforcing_failure_percentage: 100`) and `success_rate` is **detect-only**
(`enforcing_success_rate: 0`). The load-bearing isolation assertion is
`ejections_enforced_success_rate == 0`.

Phase 40.3 SPEC / PLAN Task 9.

## Topology: 5 HEALTHY + 1 ALWAYS-503 (all runner-spawned)

| endpoint | backing                          | state      | role                          |
|----------|----------------------------------|------------|-------------------------------|
| 0..4     | runner HTTPEcho backend0..4      | HEALTHY    | 200s `/`; serve load          |
| 5        | runner HTTP503Responder backend5 | ALWAYS-503 | 503s every request → ejected  |

ALL SIX hosts are runner-spawned **live** listeners; host5 answers 503 and is
ejected by **passive statistical** (failure-percentage) outlier detection.
`BackendCount()` returns **6**; the runner selects host5's kind via
`PerHostBackendKind` (`BackendKindAt(5)` → `HTTP503Responder`). The bad host's
100% failure crosses the 85% threshold cleanly, with all other hosts at 0%
failure.

## Outlier-detection config (identical on both sides — NAT-transparent static config)

```yaml
outlier_detection:
  failure_percentage_threshold: 85
  failure_percentage_minimum_hosts: 2
  failure_percentage_request_volume: 10
  enforcing_failure_percentage: 100
  success_rate_minimum_hosts: 2
  success_rate_request_volume: 10
  enforcing_success_rate: 0
  consecutive_5xx: 0
  consecutive_gateway_failure: 0
  interval: 1s
  base_ejection_time: 30s
  max_ejection_percent: 100
```

The consecutive detectors are explicitly **OFF** (`consecutive_5xx: 0`,
`consecutive_gateway_failure: 0`) so the ejection is attributable to the
`failure_percentage` detector ALONE. The `success_rate` detector is armed (its
`minimum_hosts`/`request_volume` match the FP detector) but **detect-only**
(`enforcing_success_rate: 0`). `max_ejection_percent: 100` allows the single 503
host to be ejected.

## The driver: eject-drive + poll-to-converge + warmup (the determinism mechanism)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The ejection must be driven, observed, and the worker rotation drained
BEFORE the strict measured phase, so it ALL runs inside `AssertStats`. The
`Drive` hooks stash their listener addrs and return a fixed, address-independent
byte stream (`"READY\n"`) for the runner's `CompareBytes` gate.

1. **Ejection drive** — send `ejectDriveRequests` (300) 503-**tolerant** `GET /`
   round-robin to each side's listener. **KEY INSIGHT:** under round-robin over 6
   hosts, host5 (the 503) gets ~50 picks ≫ `failure_percentage_request_volume`
   (10), all within one 1s `interval`; at the next sweep its failure percentage
   is 100% ≥ the 85% threshold, so the `failure_percentage` detector ejects it.
2. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_fp.outlier_detection.ejections_active == 1` (deadline 30s, poll
   every 200ms). **NO fixed `time.Sleep`** — poll until the predicate holds or
   the deadline trips (fail with the last-seen value on timeout).
3. **Warmup phase** — after the gauge reads 1, send 503-tolerant `GET /` until
   `warmupStable` (10) CONSECUTIVE 200s prove the worker rotation has dropped
   host5, on BOTH sides (closes the main→worker propagation window, the
   `reference_health_check_propagation_warmup` mechanism).
4. **Measured load phase** — baseline the per-request counters post-warmup, send
   `n = 60` `GET /` on each side; assert (delta) `upstream_rq_2xx == 60`,
   `upstream_rq_5xx == 0`, every body `backend-0:`..`backend-4:` (NEVER
   `backend-5:` — the ejected 503 host serves nothing), all 5 healthy hosts
   touched.
5. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                                                     | assertion | meaning                                              |
|--------------------------------------------------------------------------|-----------|------------------------------------------------------|
| `cluster.c_fp.outlier_detection.ejections_active`                        | `== 1`    | host5 ejected and held                               |
| `cluster.c_fp.outlier_detection.ejections_enforced_total`                | `== 1`    | exactly one ejection enforced                        |
| `cluster.c_fp.outlier_detection.ejections_detected_failure_percentage`   | `>= 1`    | detected via the failure_percentage detector         |
| `cluster.c_fp.outlier_detection.ejections_enforced_failure_percentage`   | `>= 1`    | enforced via the failure_percentage detector         |
| `cluster.c_fp.outlier_detection.ejections_enforced_success_rate`         | `== 0`    | SR is detect-only (`enforcing_success_rate: 0`) — LIVE == 0 (isolation) |
| `cluster.c_fp.outlier_detection.ejections_detected_consecutive_5xx`      | `== 0`    | consecutive detectors OFF — LIVE == 0                |
| `cluster.c_fp.upstream_rq_2xx` (delta, measured phase)                   | `== 60`   | all measured load routed to a healthy host           |
| `cluster.c_fp.upstream_rq_5xx` (delta, measured phase)                   | `== 0`    | no 5xx in the measured phase                         |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_fp.upstream_rq_total > 0` on the reference side before trusting the
readout.

The detected/enforced failure_percentage counters use `>= 1` (not `==`) — the
exact count can differ cross-side by detection timing; the `ejections_active ==
1` and `ejections_enforced_total == 1` are the exact-parity anchors.

### The LIVE `== 0` cross-detector assertions

- **`ejections_enforced_success_rate == 0`** — the **load-bearing isolation
  assertion**. The success_rate detector is armed + detect-only, so it does NOT
  enforce via that path — proving the `failure_percentage` detector (not
  `success_rate`) is the enforcer. A regression that enforced via the SR path
  would lift this off 0 and bite (the `assertEq` absent-as-0 accommodation
  swallows ABSENT, not present > 0).
- **`ejections_detected_consecutive_5xx == 0`** — the consecutive detectors are
  OFF (`consecutive_5xx: 0`), so the 5xx detector never fires — this proves the
  ejection is attributable to the STATISTICAL detector, not a consecutive one.

## Deliberate non-assertions

- **`ejections_detected_success_rate` is LEFT UNASSERTED** — it is detect-only
  and not the detector under test. Whether the bad host's 0% success rate crosses
  the success-rate threshold depends on the cross-host distribution at the sweep,
  so it is NOT pinned to an exact value or `>= 1` to avoid an unavoidable
  cross-side count mismatch. The load-bearing isolation is
  `ejections_enforced_success_rate == 0` (SR does not ENFORCE).
- **Recovery / un-eject arm** (`ejections_active` → 0 after `base_ejection_time`)
  is **DEFERRED** — the lazy (subject) vs sweep (reference) un-eject timing
  diverges cross-side (**AMEND-OD1**).

## Non-additions

- **NO new BackendKind authored here** — the `HTTP503Responder` (= 35) was added
  at phase 40.1; this fixture only consumes it via `BackendKindAt(5)`.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the eject-drive + warmup traffic, so they cannot cleanly attribute
  the `n=60` measured requests. The per-side 100%-to-healthy tally is taken off
  the response **bodies** inside `AssertStats` instead, and the ejection is proven
  by `upstream_rq_5xx == 0` + the healthy-only body idxs + the outlier-detection
  counters (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the `outlier_detection` config is static YAML; the
  parse/threshold/eval logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the statistical `outlier_detection` config-reject arms
  land UNIT-LEVEL in `internal/cluster` (`parseOutlierDetection`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0073' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0073'` (NOT `-run '0073'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).

## The warmup gate (cross-thread propagation robustness)

Like 0069/0072, the proxy updates the `ejections_active` gauge before the
WORKER-thread LB host-sets drop the ejected host (a small propagation window), so
an early request can still be round-robined to host5 → a transient 503 even after
the gauge reads 1. After the convergence poll, the driver runs a **warmup**: it
sends 503-tolerant `GET /` until `warmupStable` (10) CONSECUTIVE 200s prove the
worker rotation has dropped host5, THEN runs the strict measured phase. The
per-request counters (`upstream_rq_2xx`/`_5xx`) are asserted as a **delta** over
the measured phase (baseline scraped post-warmup), so the eject-drive + warmup
requests do not over-count. Round-robin hits host5 every 6th pick when it is NOT
ejected, so an un-ejected build can never reach 10 consecutive 200s — the gate
still bites the deliberate breaks.
