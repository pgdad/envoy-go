# Fixture 0082 — gRPC access-log BUFFERING differential equivalence

The behavioral proof of the phase 44.2 gRPC ALS **buffering** extension on top of
the 44.1 core streaming sink: cross-side EXACT (subject envoy-go vs reference
Envoy v1.37.2 in Docker) on the deterministic structured-field subset of every
streamed `HTTPAccessLogEntry`, PLUS a **SUBJECT-side proof that the buffer
coalesced `>= 2` entries into at least one `StreamAccessLogsMessage`**.

This fixture is **0081 + the two `common_config` buffer fields**. A single
plaintext HTTP/1.1 listener (`l_test`) routes every request to a fixed-body
backend (`c_backend` — the phase-14/0006 `HTTPFixedBody` helper,
`backend:v1/fixed\n` = 17 bytes). The HCM carries one `access_log[]` entry of
type `HttpGrpcAccessLogConfig` (`log_name: "0082"`,
`grpc_service.envoy_grpc.cluster_name: c_als`) whose `common_config` now also
carries:

```yaml
buffer_size_bytes: 1048576      # 1 MiB — DORMANT (the SIZE trigger never fires)
buffer_flush_interval: 1s       # the deterministic TIMER flush lever
```

It streams one `HTTPAccessLogEntry` proto per completed request to a
**driver-owned in-process `AccessLogService` receiver**
(`test/helpers/accessloggrpc/`). Plaintext h2c — no TLS — per D-ALS-RECEIVER.
Reference Envoy v1.37.2 (STRICT_DNS via `host.docker.internal`) vs envoy-go
(STATIC, in-process via `127.0.0.1`).

## Buffer drive (D-BUF-DIFFERENTIAL-DRIVE / SPEC §8.1)

The two buffer fields are **LIVE**: the buffered sink flushes a batch on EITHER
the byte-cap OR the interval timer.

- `buffer_size_bytes: 1048576` (1 MiB) — **DORMANT**. The N small entries never
  reach the byte cap, so the SIZE trigger never fires; the byte-accounting-
  fragile size-cap path is deliberately AVOIDED cross-side.
- `buffer_flush_interval: 1s` — the deterministic **TIMER** flush lever.

The drive: **N=16** requests are fired **CONCURRENTLY** (a `sync.WaitGroup`
fan-out against the same listener `addr`) so the records queue into envoy-go's
single process-global buffer FASTER than the 1s timer elapses; the next tick
then flushes `>= 2` as one batch.

**Coalescence-determinism caveat (SPEC §8.1):** the subject `maxBatchSize >= 2`
only holds if `>= 2` entries land in one flush interval — the concurrent burst +
the wide 1s interval is the coalescence guarantee. If it ever flakes, widen the
interval to 2s and/or raise N. (Task 8 runs the 20/20 flake gate.)

## Cross-side batch counts infeasible (AMEND-BUF-3)

The cross-side assertion is on the per-entry **PAYLOAD** aggregated across all
received entries — NOT stream count, per-message batching, or flush cadence. The
reference buffers **PER-WORKER-THREAD** (un-pinned worker count) while envoy-go
uses one process-global buffer, so cross-side batch/message COUNTS are
infeasible. The buffering proof is therefore **SUBJECT-side ONLY**: aggregated
payload cross-side + a subject-side `maxBatchSize >= 2` proof. The reference's
per-worker batching is its own un-pinned business.

## Driver-owned-receiver lifecycle

The `accessloggrpc.Server` is NOT a runner `BackendKind` — it is a
driver-managed test helper the proxy DIALS
(`reference_differential_grpc_receiver_driver_owned`). The driver:

1. Allocates a free TCP port at `ReferenceBootstrap` time (Listen+Close) and
   binds the receiver on `0.0.0.0:<port>` BEFORE the reference container starts
   — so the reference Envoy reaches it via `host.docker.internal` (the Docker
   bridge alias, `reference_docker_probe_bridge_network`) AND the subject reaches
   it via `127.0.0.1`. The SAME port is baked into both bootstrap YAMLs.
2. `DriveReference` / `DriveSubject` each `Reset()` the receiver (clears entries
   AND batchSizes), fire N=16 identical query-less requests CONCURRENTLY, then
   POLL `Count()` to `>= 16` (poll-to-converge, never sleep — the reference
   buffers ALS entries and flushes on a ~1s timer;
   `reference_concurrency_differential_release_barrier`). Each side's entry set
   AND per-message batch sizes are snapshotted before the next side runs.
   Per-side separation is clean: the subject generates no access-log entries
   until its own `DriveSubject` window.
3. `AssertStats` asserts the 7-field subset on every entry on BOTH sides, the
   subject-side `logs_written` stat, AND the subject-side `maxBatchSize >= 2`
   buffering proof. The receiver is then hard-stopped (`Close()`).

## Host-reachability table

| consumer            | ALS host (`{{.ALSHost}}`) | backend host (`{{.BackendHost}}`) |
|---------------------|---------------------------|-----------------------------------|
| reference (Docker)  | `host.docker.internal`    | `host.docker.internal`            |
| subject (host)      | `127.0.0.1`               | `127.0.0.1`                       |

The receiver binds `0.0.0.0:<port>` so BOTH paths resolve to the same listener.

## Query-less path (AMEND-ALS-2)

The data-plane request uses `GET /health` (NO query string). envoy-go's
`Record.Path` is path-only while the reference's `request.path` carries the
query string — a query-bearing request would DIVERGE cross-side on
`request.path`. This is a documented faithful constraint; `/health` keeps
`request.path` EXACT on both sides.

## Asserted 7-field subset (cross-side EXACT, every entry, both sides)

| field | value |
|---|---|
| `request.request_method` | `GET` |
| `request.path` | `/health` |
| `request.authority` | `als.example` |
| `request.user_agent` | `als-probe/1` |
| `response.response_code` | `200` |
| `response.response_body_bytes` | `17` (the `HTTPFixedBody` fixed-body length) |
| `protocol_version` | `HTTP11` |

PLUS the subject-side flat `/stats` counter
`access_logs.grpc_access_log.logs_written == 16` (every send succeeded) AND the
SUBJECT-side buffering proof `max(subjBatchSizes) >= 2` (the buffer coalesced
`>= 2` entries into at least one message — BITES a regression to the 44.1
one-entry-per-message fixed flush).

Set `FIXTURE_0082_DUMP=1` to print the per-side entry counts +
`refBatchSizes`/`subjBatchSizes` to stderr for diagnostics.

## UNasserted

- `common_properties.{start_time, duration, upstream_remote_address}` —
  populated but non-deterministic (AMEND-ALS-4).
- `identifier.node` — minimal node (Id+Cluster), D-ALS-NODE; UNasserted.
- reference-side stream / message / batch framing counts — AMEND-ALS-3 /
  AMEND-BUF-3.
- subject-absent reference fields: `request.scheme`, `request_id`,
  `upstream_cluster`, `access_log_type`, `response_code_details`, the wire-byte
  counts — not mapped by envoy-go's 10-field `Record` (SPEC §12 D-ALS-*).

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS via `host.docker.internal`;
  `c_als` carries `http2_protocol_options:{}` + plaintext h2c; `common_config`
  carries the two buffer fields).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC via `127.0.0.1`; same buffer fields).
- `driver/driver.go` — the single-listener driver: registers the fixture,
  manages the receiver lifecycle, drives both proxies CONCURRENTLY
  (poll-to-converge), and asserts the 7-field subset cross-side + the subject
  `logs_written` stat + the subject `maxBatchSize >= 2` buffering proof.
- `expectations.yaml` — prose expectations (ADR-0019 — the driver is the
  enforcer; this file is documentation).

Cross-refs: phase 44.2 SPEC §8.1/§12 + D-BUF-DIFFERENTIAL-DRIVE + AMEND-BUF-3 +
D-ALS-RECEIVER + AMEND-ALS-2/3/4 + D-ALS-NODE + ADR-0256 +
`reference_streaming_sink_differential_framing` +
`reference_differential_grpc_receiver_driver_owned`.
