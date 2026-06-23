# 0079-h2-multiplex-pool — H2 upstream multiplex connection pool (cross-side EXACT)

Phase 43.2a (`docs/envoy-go/phases/43.2-h2-connection-pool/`) — the BEHAVIORAL
proof of the HTTP/2 upstream multiplex connection pool. It validates, cross-side
against reference Envoy `contrib-v1.37.2` over a Docker bridge, that the pool
opens exactly **ceil(K/C)** upstream connections and enforces the stream-aware
pending queue.

## Purpose

Each downstream HTTP/1.1 request becomes ONE upstream HTTP/2 **stream**,
multiplexed onto a small pool of reusable upstream connections. The per-connection
stream budget is the cluster's OWN `http2_protocol_options.max_concurrent_streams`
(the **local cap** `C`). A new connection opens only when every existing
connection holds `C` in-flight streams. When both stream and connection capacity
are exhausted a request PENDS on a bounded wait-queue; queue-full ⇒ fail-fast 503.

## Cross-side EXACT — why (the local-cap-driven ceil)

The SPEC §11 D-H2-EXACT live probe (2026-06-23) confirmed the reference grows its
H2 pool off the cluster's **own** `max_concurrent_streams`, NOT the peer's
advertised SETTINGS: with `C` set and `K` fully-overlapping held streams, the
reference opens EXACTLY `ceil(K/C)` connections — deterministic, zero errors, all
200s. This is CLEAN flow-control enforcement, so the H2 conn/stream **counts are
asserted cross-side EXACT on both sides** — contrast 0078/43.1's SOFT
`max_connections`, which forced an exact-vs-robust prong split
(`reference_max_connections_soft_breaker`). See
`reference_h2_pool_local_cap_driven`.

The backend (`H2HoldResponder`, BackendKind 37) advertises
`SETTINGS_MAX_CONCURRENT_STREAMS=1000 >> C`, so the LOCAL cap binds and no
`REFUSED_STREAM` fires (AMEND-H2-1/H2-5).

## Topology — two clusters, one backend

The two prongs need irreconcilable single-cluster budgets, so the fixture uses
TWO h2c clusters BOTH pointing at the SAME backend host:

| Cluster   | `C` (max_concurrent_streams) | max_connections | max_pending_requests | Prong |
|-----------|------------------------------|-----------------|----------------------|-------|
| `c_h2mp`  | 2                            | 16 (non-binding)| 16 (non-binding)     | EXACT ceil: K=6 ⇒ ceil(6/2)=**3** conns |
| `c_h2of`  | 1                            | 1               | 1                    | overflow: 1 held + 1 pending + 1 → 503 |

Route selection: `/mp` → `c_h2mp`, `/of` → `c_h2of`. HTTP/1.1 downstream, h2c
upstream (no `transport_socket`, per ADR-0166). Subject is STATIC / `127.0.0.1`;
reference is STRICT_DNS / `host.docker.internal` (the shared-bridge shape,
`reference_docker_probe_bridge_network`).

## The staged drive (SLEEPLESS — poll-to-converge + release-barrier; sequential-per-side)

All work runs inside `AssertStats`, drive+assert the SUBJECT fully, then the
REFERENCE (`reference_concurrency_differential_release_barrier`). Synchronization
is poll-the-gauge until convergence + the backend's re-armable `/__release`
control path — NEVER a `time.Sleep`.

**Prong 1 — `c_h2mp` (the ceil prong):**
1. Fire K=6 fully-overlapping held `GET /mp/<i>` → poll
   `upstream_cx_total == 3` AND `upstream_cx_http2_total == 3` AND
   `http2.streams_active == 6`.
2. multiplex proof: `upstream_cx_total (==3) << K (==6)`.
3. `/__release` → all 6 drain to 200 (`backend-0:<seg>`) → poll
   `http2.streams_active == 0`.

**Prong 2 — `c_h2of` (the overflow prong):**
4. Fire 1 held `GET /of/0` → poll `http2.streams_active == 1`; fire a 2nd → it
   PENDS → poll `upstream_rq_pending_active == 1`; fire a 3rd SYNCHRONOUSLY → the
   queue is full → DOWNSTREAM **503** + `upstream_rq_pending_overflow` delta ≥ 1
   (assert the DOWNSTREAM class, NOT `upstream_rq_5xx` —
   `reference_concurrent_attempt_downstream_class_assertion`).
5. `/__release` → the held + woken pending drain to 200 → poll
   `http2.streams_active == 0` AND `upstream_rq_pending_active == 0`.

## Running

```bash
go test ./test/differential/ -run 'TestDifferential/0079' -count=1
```

(Requires Docker for the reference container. Use the
`-run 'TestDifferential/0079'` selector — a bare `-run '0079'` matches zero
subtests, `reference_differential_run_selector`. Always `-count=1`.)

## Config-as-Go-string

Per the differential-fixture convention there are NO standalone `envoy.yaml` /
`envoy-go.yaml` files — the two bootstraps are returned as Go string templates
from `driver.ReferenceBootstrap` / `driver.SubjectConfig` (the 0078 mechanism).
The single-sourced workload constants (`streamCapMP`, `heldK`, `expectedConnsMP`,
the overflow budgets) live in `driver/driver.go` and are pinned by
`driver/driver_test.go::TestConstants` (`reference_fixture_workload_constant_desync`).
