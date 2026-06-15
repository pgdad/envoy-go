# 0067-health-check-tcp

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_hc` (`lb_policy: ROUND_ROBIN`) with **active TCP health checking (connect-only)**
over THREE endpoints — **2 LIVE** HTTPEcho backends + **1 DEAD** host (a `host:port`
with no listener → connect refused → the probe fails) — on BOTH sides (the 0065 HTTP
shape: reference `STRICT_DNS` / `host.docker.internal`, subject `STATIC` /
`127.0.0.1`).

It proves that an unhealthy (dead) upstream host is **detected by the connect-only
TCP health checker and removed from LB rotation** on BOTH the envoy-go (subject)
side and the reference-Envoy side. The healthy fraction after convergence is 2/3 ≈
66% > the 50% panic threshold, so the cluster **filters** the dead host (it does NOT
enter panic mode and spray across all hosts) — the load lands **exclusively** on the
2 live backends.

Phase 39.2 SPEC §8.1 / PLAN Task 4.

## Topology: 2 LIVE backends (runner-spawned) + 1 DEAD host (unbound port)

| endpoint | backing                       | state | role                                   |
|----------|-------------------------------|-------|----------------------------------------|
| 0        | runner HTTPEcho backend0      | LIVE  | accepts TCP connect (HC) + serves HTTP load |
| 1        | runner HTTPEcho backend1      | LIVE  | accepts TCP connect (HC) + serves HTTP load |
| 2        | an unbound host port (dead)   | DEAD  | connect refused → probe fails → filtered |

The DEAD host is **NOT** a runner backend — `BackendCount()` returns **2**, so the
runner spawns 2 live HTTPEcho backends. The driver binds `0.0.0.0:0`, captures the
port, then **closes** the listener so the port stays unbound for the run. Both
sides reference that same port number (reference via `host.docker.internal:<dead>`,
subject via `127.0.0.1:<dead>`) — a TCP connect to it is refused on both sides.

The live `HTTPEcho` backends answer a bare TCP connect for the HC **AND** serve the
HTTP data-plane load — no new `BackendKind` is needed (reuses `HTTPEcho`).

## Health-check config (identical on both sides — NAT-transparent static config)

```yaml
health_checks:
  - interval: 0.5s
    timeout: 0.5s
    unhealthy_threshold: 1
    healthy_threshold: 1
    tcp_health_check: {}
```

`tcp_health_check: {}` is the connect-only form — Envoy opens a TCP connection
and marks the host healthy if the connect succeeds (no payload sent/received).
`unhealthy_threshold: 1` → one failed connect marks the dead host unhealthy; the
0.5s interval keeps convergence fast.

## The driver: poll-to-converge (the determinism mechanism)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The load MUST NOT begin until the dead host is detected + filtered (an
early request could be round-robined to the dead host → 5xx), so the convergence
poll + the load + the assertions ALL run inside `AssertStats`. The `Drive` hooks
stash their listener addrs and return a fixed, address-independent byte stream
(`"READY\n"`) for the runner's `CompareBytes` gate.

1. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_hc.membership_healthy == 2` (deadline 30s, poll every 200ms). **NO
   fixed `time.Sleep`** — poll until the predicate holds or the deadline trips
   (fail with a clear message on timeout).
2. **Load phase** — send `n = 100` `GET /` to each side's listener.
3. **100%-to-live assertion** (per side) — every request served `200` by a live
   backend (the response body `backend-<idx>:` attributes the host; `idx ∈ {0,1}`);
   the tally sums to 100 and **both** live hosts are touched (ROUND_ROBIN over 2).
   A non-200 (a connection-failure 5xx from the dead host) is a hard `FAIL`.
4. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                   | assertion | meaning                                   |
|----------------------------------------|-----------|-------------------------------------------|
| `cluster.c_hc.membership_healthy`      | `== 2`    | dead host filtered, 2 live remain         |
| `cluster.c_hc.membership_total`        | `== 3`    | filtered, not removed (still 3 endpoints) |
| `cluster.c_hc.health_check.attempt`    | `> 0`     | the checker ran                           |
| `cluster.c_hc.health_check.success`    | `> 0`     | live hosts pass TCP connect               |
| `cluster.c_hc.health_check.failure`    | `> 0`     | the dead host fails every probe           |
| `cluster.c_hc.upstream_rq_total`       | `== 100`  | all load routed to a live host            |
| `cluster.c_hc.upstream_rq_2xx`         | `== 100`  | all 200                                   |
| `cluster.c_hc.upstream_rq_5xx`         | `== 0`    | no dead-host connection failures          |
| `cluster.c_hc.upstream_cx_active`      | `== 0`    | quiesced (`Connection: close`)            |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_hc.upstream_rq_total > 0` on the reference side before trusting the
readout.

## Non-additions

- **NO new BackendKind** — reuses `HTTPEcho` (the 0003/0064/0065/0066 backend; the
  live backends accept a bare TCP connect for the HC AND serve the HTTP data-plane
  load); the backend tail STAYS at 33.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the continuous health-check **probe** traffic (each 0.5s TCP connect
  to a live backend increments its accept counter), so they cannot cleanly attribute
  the n=100 **data** requests. The per-side 100%-to-live tally is taken off the
  response **bodies** inside `AssertStats` instead, and the dead-host filtering is
  proven by `upstream_rq_5xx == 0` + the live-only body idxs
  (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the health-check config is static YAML; the parse/threshold/
  membership logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `health_checks` config-reject arms land UNIT-LEVEL
  in `internal/cluster` (`parseHealthChecks`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0067' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0067'` (NOT `-run '0067'`, which matches zero subtests
— `reference_differential_run_selector`). Docker must be available (the reference
image `envoyproxy/envoy:contrib-v1.37.2`).

## The warmup gate (cross-thread propagation robustness)

Reference Envoy updates the `membership_healthy` gauge on the MAIN thread before
the WORKER-thread LB host-sets drop the dead host (a small propagation window),
so an early request can still be round-robined to the dead host → a transient 503
even after the gauge reads 2. After the convergence poll, the driver runs a
**warmup**: it sends 503-tolerant `GET /` until `warmupStable` (10) CONSECUTIVE
200s prove the worker rotation has dropped the dead host, THEN runs the strict
measured phase. The per-request counters (`upstream_rq_total`/`_2xx`/`_5xx`) are
asserted as a **delta** over the measured phase (baseline scraped post-warmup),
so the convergence-poll + warmup requests do not over-count. Round-robin hits the
dead host every 3rd pick when it is NOT filtered, so an unfiltered build can never
reach 10 consecutive 200s — the gate still bites the deliberate breaks.

## Deliberate-break liveness (`-count=1`; reverted, not committed) + flake

| Break | Mutation | Result |
|---|---|---|
| A — health state | `isHealthy` always returns `true` (dead host never marked down) | `converge:` poll times out (membership never reaches 2 within 30s) — FAIL |
| B — pick filter | `roundRobin.Pick` accepts any host (`isHealthy(ep) \|\| true`) | `warmup:` never reaches 10 consecutive 200s (dead host every 3rd pick) — FAIL |

Both breaks bite under `-count=1` (`reference_differential_break_protocol_count1`).
