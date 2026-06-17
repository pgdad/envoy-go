# 0071-outlier-detection-local-origin

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_lo` (`lb_policy: ROUND_ROBIN`) with **passive outlier detection** over THREE
endpoints — **2 LIVE** HTTPEcho backends + **1 DEAD** host (a host:port with no
listener → connect refused) — on BOTH sides (the 0066/0070 HTTP shape: reference
`STRICT_DNS` / `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

It proves that an upstream host whose connects are **refused** is **detected +
ejected by the `consecutive_local_origin_failure` detector** on BOTH the envoy-go
(subject) side and the reference-Envoy side, under
`split_external_local_origin_errors: true`. With **split** enabled each
connect-refused is a **local-origin** failure (`LocalOriginErr: true`), routed to
the **local-origin detector ONLY**. The **split invariant**: the
`consecutive_5xx` / `consecutive_gateway_failure` detectors NEVER fire, so
**`detected_consecutive_5xx == 0`** cross-side.

After ejection the live fraction is 2/3 ≈ 66% > the 50% panic threshold, so the
load lands **exclusively** on the 2 live backends.

Phase 40.2 SPEC §10 / PLAN Task 8. Sibling of **0070** (same eject-drive flow;
0070 ejects via the `consecutive_gateway_failure` detector over an always-503
host, 0071 ejects via the `consecutive_local_origin_failure` detector over a dead
host under split=true).

## Topology: 2 LIVE backends (runner-spawned) + 1 DEAD host (unbound port)

| endpoint | backing                          | state | role                                  |
|----------|----------------------------------|-------|---------------------------------------|
| 0        | runner HTTPEcho backend0         | LIVE  | 200s `/`; serves load                 |
| 1        | runner HTTPEcho backend1         | LIVE  | 200s `/`; serves load                 |
| 2        | `allocDeadPort` (unbound)        | DEAD  | connect refused → `LocalOriginErr` → ejected |

The DEAD host is **NOT a runner backend** — `BackendCount()` returns **2** (only
the 2 LIVE HTTPEcho backends are spawned). The driver binds `0.0.0.0:0`, captures
the port, **closes** the listener (so the port stays unbound → a connect is
refused), and **memoizes** it so both sides reference the **same** dead port
(reference via `host.docker.internal:<dead>`, subject via `127.0.0.1:<dead>`).
This is the **0066 dead-host mechanism** (`allocDeadPort`) — **NO
`PerHostBackendKind`**. On the subject side the refused connect reaches the H1
`AcquireH1` connect-failure seam (Task 6) →
`RecordUpstreamResult{LocalOriginErr: true}` → the local-origin detector.

## Outlier-detection config (identical on both sides — NAT-transparent static config)

```yaml
outlier_detection:
  consecutive_local_origin_failure: 5
  enforcing_consecutive_local_origin_failure: 100
  split_external_local_origin_errors: true
  interval: 10s
  base_ejection_time: 30s
  max_ejection_percent: 100
```

`split_external_local_origin_errors: true` is the load-bearing knob: it routes
the dead-host connect-refused to the **local-origin** detector, the sole ejection
trigger here. `max_ejection_percent: 100` allows the single dead host to be
ejected; the `interval`/`base_ejection_time` are parse-accepted (recovery
DEFERRED).

## The driver: eject-drive + poll-to-converge + warmup (the 0070 template)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The `Drive` hooks stash their listener addrs and return a fixed,
address-independent byte stream (`"READY\n"`) for the runner's `CompareBytes`
gate.

1. **Ejection drive** — send `ejectDriveRequests` (24) 5xx-**tolerant** `GET /`
   round-robin to each side. Under strict round-robin over 3 endpoints the dead
   host is picked every 3rd request; `consecLO` is **per-host** and **never
   reset** (no completed external response ever comes FROM the dead host), so it
   accrues consecutive local-origin failures until it crosses
   `consecutive_local_origin_failure` (5) — roughly `5 * 3 = 15` requests; the 24
   count carries a margin.
2. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_lo.outlier_detection.ejections_active == 1` (deadline **45s**, poll
   200ms). **NO fixed `time.Sleep`** — poll until the predicate holds. The
   deadline carries headroom over 0070: a dead-host connect-refused can be slower
   to accrue than an HTTP 503 (connect attempt + `connect_timeout`).
3. **Warmup phase** — after the gauge reads 1, send 5xx-tolerant `GET /` until
   `warmupStable` (10) CONSECUTIVE 200s prove the worker rotation has dropped the
   dead host, on BOTH sides (the `reference_health_check_propagation_warmup`
   gate).
4. **Measured load phase** — baseline the per-request counters post-warmup, send
   `n = 60` `GET /` on each side; assert (delta) `upstream_rq_2xx == 60`,
   `upstream_rq_5xx == 0`, every body `backend-0:`/`backend-1:` (the dead host
   serves nothing), both live hosts touched.
5. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                                                                  | assertion | meaning                                |
|---------------------------------------------------------------------------------------|-----------|----------------------------------------|
| `cluster.c_lo.outlier_detection.ejections_active`                                     | `== 1`    | dead host ejected and held             |
| `cluster.c_lo.outlier_detection.ejections_enforced_total`                             | `>= 1`    | an ejection was enforced               |
| `cluster.c_lo.outlier_detection.ejections_detected_consecutive_local_origin_failure`  | `>= 1`    | detected via the local-origin detector |
| `cluster.c_lo.outlier_detection.ejections_enforced_consecutive_local_origin_failure`  | `>= 1`    | enforced via the local-origin detector |
| `cluster.c_lo.outlier_detection.ejections_detected_consecutive_5xx`                   | `== 0`    | the 5xx detector NEVER fired (split)   |
| `cluster.c_lo.upstream_rq_2xx` (delta, measured phase)                                | `== 60`   | all measured load routed to a live host |
| `cluster.c_lo.upstream_rq_5xx` (delta, measured phase)                                | `== 0`    | no 5xx in the measured phase           |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_lo.upstream_rq_total > 0` on the reference side before trusting the
readout.

The local-origin enforced/detected counters use `>= 1` (the exact count can
differ cross-side by detection timing); the `ejections_active == 1` gauge is the
exact-parity anchor. The `consecutive_5xx` counter uses `== 0` — a **live
equality** that bites if the split branch regressed.

## Why `detected_consecutive_5xx == 0` is a LIVE assertion

With `split_external_local_origin_errors: true`, `recordLocalOrigin` feeds the
connect-refused (`LocalOriginErr`) to the **local-origin** detector ONLY (see
`internal/cluster/outlier.go` `recordLocalOrigin` / `record`). The
`consecutive_5xx` / gateway detectors never see it, so the 5xx detected counter
MUST be exactly 0. If the split branch regressed (the failure mapped instead to a
gateway-class 5xx — the split=false path), `detected_consecutive_5xx` would lift
off 0 and the equality would fail — the check is not vacuous. The
absent-counter-reads-0 carve-out applies ONLY to the `want==0` case (the
reference lazily allocates per-detector counters) — it swallows absence, **not** a
present-but-nonzero value.

## Deliberate non-assertions

- **Recovery / un-eject arm** (`ejections_active` → 0 after `base_ejection_time`)
  is **DEFERRED** — the lazy (subject) vs sweep (reference) un-eject timing
  diverges cross-side (**AMEND-OD1**).
- **`enforced_consecutive_5xx`** is not asserted separately — `detected==0`
  already implies the 5xx detector never reached its detect step.

## Non-additions

- **NO new BackendKind** — the dead host is an injected **unbound** endpoint, NOT
  a runner backend (the 0066 shape: `BackendCount()==2`, NO `PerHostBackendKind`).
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the eject-drive + warmup traffic. The per-side 100%-to-live tally is
  taken off the response **bodies** inside `AssertStats` instead, and the ejection
  is proven by `upstream_rq_5xx == 0` + the live-only body idxs + the
  outlier-detection counters (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the `outlier_detection` config is static YAML; the
  parse/threshold/eject logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `outlier_detection` config-reject arms land
  UNIT-LEVEL in `internal/cluster` (`parseOutlierDetection`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0071' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0071'` (NOT `-run '0071'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).
