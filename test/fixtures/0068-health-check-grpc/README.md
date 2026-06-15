# 0068-health-check-grpc

Cross-side `[http_connection_manager + router]` differential over **one cluster**
`c_hc` (`lb_policy: ROUND_ROBIN`) with **active gRPC health checking**
(`grpc_health_check{}` → `grpc.health.v1.Health/Check` over H2) over THREE
endpoints — **2 LIVE** GRPCHealthResponder backends (BackendKind 34, h2c) +
**1 DEAD** host (a `host:port` with no listener → connect refused → the gRPC
probe's transport setup fails) — on BOTH sides (the 0065 HTTP shape: reference
`STRICT_DNS` / `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

It proves that an unhealthy (dead) upstream host is **detected by the gRPC health
checker and removed from LB rotation** on BOTH the envoy-go (subject) side and
the reference-Envoy side. The healthy fraction after convergence is 2/3 ≈ 66% >
the 50% panic threshold, so the cluster **filters** the dead host (it does NOT
enter panic mode and spray across all hosts) — the load lands **exclusively** on
the 2 live backends.

The gRPC checker speaks `grpc.health.v1` over HTTP/2, so the cluster **must** be
H2 (`http2_protocol_options` via `typed_extension_protocol_options`). The
downstream listener stays `codec_type: HTTP1`; the proxy translates
H1-downstream → H2-upstream. No H2 client helper is needed in the driver.

Phase 39.2 SPEC §8.2 / PLAN Task 9.

## Topology: 2 LIVE backends (runner-spawned) + 1 DEAD host (unbound port)

| endpoint | backing                                | state | role                                         |
|----------|----------------------------------------|-------|----------------------------------------------|
| 0        | runner GRPCHealthResponder backend0    | LIVE  | h2c; gRPC Check → SERVING; serves HTTP load  |
| 1        | runner GRPCHealthResponder backend1    | LIVE  | h2c; gRPC Check → SERVING; serves HTTP load  |
| 2        | an unbound host port (dead)            | DEAD  | connect refused → probe transport fails → filtered |

The DEAD host is **NOT** a runner backend — `BackendCount()` returns **2**, so the
runner spawns 2 live GRPCHealthResponder backends. The driver binds `0.0.0.0:0`,
captures the port, then **closes** the listener so the port stays unbound for the
run. Both sides reference that same port number (reference via
`host.docker.internal:<dead>`, subject via `127.0.0.1:<dead>`) — a gRPC probe to
it fails with a transport error on both sides.

The live `GRPCHealthResponder` backends (BackendKind 34, **+1 BackendKind** vs
0067) speak h2c. They respond `SERVING` to the gRPC `Health/Check` probe **and**
write `"backend-<idx>:<path>"` for the forwarded HTTP data-plane request — no
separate `HTTPEcho` backend is needed.

## Health-check config (identical on both sides — NAT-transparent static config)

```yaml
health_checks:
  - interval: 0.5s
    timeout: 0.5s
    unhealthy_threshold: 1
    healthy_threshold: 1
    grpc_health_check: {}
```

`grpc_health_check: {}` sends a `grpc.health.v1.Health/Check` RPC (unnamed
service, empty body) over the cluster's H2 upstream connection. The live backends
answer `SERVING`; a connect to the dead host is refused (transport failure → probe
failure). `unhealthy_threshold: 1` → one failed probe marks the dead host
unhealthy; the 0.5s interval keeps convergence fast.

## H2 upstream cluster (required by the gRPC checker)

```yaml
typed_extension_protocol_options:
  envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
    "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
    explicit_http_config:
      http2_protocol_options: {}
```

The cluster MUST be H2 (`http2_protocol_options`) — a gRPC HC on a non-H2 cluster
is rejected at boot (Task 7's `buildCluster` reject arm). Both sides carry the same
`typed_extension_protocol_options` block (the 0021 gRPC-upstream shape). The proxy
speaks cleartext-H2/h2c to the GRPCHealthResponder for **both** the HC probe AND
the forwarded data-plane request.

## The driver: poll-to-converge (the determinism mechanism)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). The load MUST NOT begin until the dead host is detected + filtered (an
early request could be round-robined to the dead host → 5xx), so the convergence
poll + the warmup + the load + the assertions ALL run inside `AssertStats`. The
`Drive` hooks stash their listener addrs and return a fixed, address-independent
byte stream (`"READY\n"`) for the runner's `CompareBytes` gate.

1. **Poll phase** — scrape `/stats` on BOTH sides until
   `cluster.c_hc.membership_healthy == 2` (deadline 30s, poll every 200ms). **NO
   fixed `time.Sleep`** — poll until the predicate holds or the deadline trips
   (fail with a clear message on timeout).
2. **Warmup phase** — send 503-tolerant `GET /` until `warmupStable` (10)
   CONSECUTIVE 200s close the gauge→worker-set propagation window (see below).
3. **Baseline scrape** — scrape `/stats` post-warmup; per-request counters are
   measured as a **DELTA** over the load phase so warmup requests don't over-count.
4. **Load phase** — send `n = 100` `GET /` to each side's listener.
5. **100%-to-live assertion** (per side) — every request served `200` by a live
   backend (the response body `backend-<idx>:` attributes the host; `idx ∈ {0,1}`);
   the tally sums to 100 and **both** live hosts are touched (ROUND_ROBIN over 2).
   A non-200 (a connection-failure 5xx from the dead host) is a hard `FAIL`.
6. **Cross-side stats** (both sides) — see below.

## The stats prong (`StatsAsserter`, cross-side)

On BOTH sides:

| stat                                   | assertion    | meaning                                          |
|----------------------------------------|--------------|--------------------------------------------------|
| `cluster.c_hc.membership_healthy`      | `== 2`       | dead host filtered, 2 live remain                |
| `cluster.c_hc.membership_total`        | `== 3`       | filtered, not removed (still 3 endpoints)        |
| `cluster.c_hc.health_check.attempt`    | `> 0`        | the checker ran                                  |
| `cluster.c_hc.health_check.success`    | `> 0`        | live hosts pass gRPC Check → SERVING             |
| `cluster.c_hc.health_check.failure`    | `> 0`        | the dead host fails every probe                  |
| `cluster.c_hc.upstream_rq_total` Δ    | `== 100`     | all load routed to a live host (DELTA)           |
| `cluster.c_hc.upstream_rq_2xx` Δ      | `== 100`     | all 200 (DELTA)                                  |
| `cluster.c_hc.upstream_rq_5xx` Δ      | `== 0`       | no dead-host connection failures (DELTA)         |
| `cluster.c_hc.upstream_cx_active`      | `<= 2`       | NO connection held to the dead host (upper bound) |

Plus the **"decode ran" guard** (`reference_docker_probe_bridge_network`):
`cluster.c_hc.upstream_rq_total > 0` on the reference side before trusting the
readout.

### The `upstream_cx_active <= backendCount` invariant

Unlike 0067 (HTTP/1.1 with `Connection: close` → each request is a fresh dial →
`upstream_cx_active` always quiesces to `0`), the H2 data plane multiplexes over a
connection pool whose **idle-retention policy differs per implementation**: the
reference Envoy keeps one persistent pooled H2 connection per live host
(`upstream_cx_active == 2` post-load), whereas envoy-go's subject pool may have
already torn the idle connections down (`== 0`). Both are valid H2 quiescence
states, so this is **NOT** a cross-side equality assertion. The invariant that
holds on BOTH sides and is **meaningful**: the active count never exceeds
`backendCount` — i.e. NO connection is ever held to the filtered dead host (a
value of 3 would mean the dead host got pooled). This still bites a build that
fails to filter the dead host.

## Non-additions (beyond the +1 BackendKind)

- **+1 BackendKind** — `GRPCHealthResponder` (kind 34); the backend tail is now
  **34** (up from 33 at 0067). This is the only addition.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are
  polluted by the continuous health-check **probe** traffic (each 0.5s gRPC Check
  to a live backend increments its accept counter), so they cannot cleanly attribute
  the n=100 **data** requests. The per-side 100%-to-live tally is taken off the
  response **bodies** inside `AssertStats` instead, and the dead-host filtering is
  proven by `upstream_rq_5xx Δ == 0` + the live-only body idxs
  (`reference_differential_asserter_dispatch`).
- **NO new fuzzer** — the health-check config is static YAML; the parse/threshold/
  membership logic is UNIT-covered in `internal/cluster`.
- **NO boot-reject dir** — the `health_checks` config-reject arms (including the
  gRPC-HC-requires-H2 reject) land UNIT-LEVEL in `internal/cluster`
  (`parseHealthChecks` / `buildCluster`).

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0068' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0068'` (NOT `-run '0068'`, which matches zero subtests
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
