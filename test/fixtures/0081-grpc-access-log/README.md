# Fixture 0081 — gRPC access-log (`envoy.access_loggers.http_grpc`) differential equivalence

The behavioral proof of the phase 44.1 core gRPC ALS streaming sink: cross-side
EXACT (subject envoy-go vs reference Envoy v1.37.2 in Docker) on the
deterministic structured-field subset of every streamed `HTTPAccessLogEntry`.

A single plaintext HTTP/1.1 listener (`l_test`) routes every request to a
fixed-body backend (`c_backend` — the phase-14/0006 `HTTPFixedBody` helper,
`backend:v1/fixed\n` = 17 bytes). The HCM carries one `access_log[]` entry of
type `HttpGrpcAccessLogConfig` (`log_name: "0081"`,
`grpc_service.envoy_grpc.cluster_name: c_als`) that streams one
`HTTPAccessLogEntry` proto per completed request to a **driver-owned in-process
`AccessLogService` receiver** (`test/helpers/accessloggrpc/`, new in phase 44.1).
Plaintext h2c — no TLS — per D-ALS-RECEIVER. Reference Envoy v1.37.2 (STRICT_DNS
via `host.docker.internal`) vs envoy-go (STATIC, in-process via `127.0.0.1`).

## Driver-owned-receiver lifecycle

The `accessloggrpc.Server` is NOT a runner `BackendKind` — it is a
driver-managed test helper the proxy DIALS
(`reference_differential_grpc_receiver_driver_owned`). The driver:

1. Allocates a free TCP port at `ReferenceBootstrap` time (Listen+Close) and
   binds the receiver on `0.0.0.0:<port>` BEFORE the reference container starts
   — so the reference Envoy reaches it via `host.docker.internal` (the Docker
   bridge alias, `reference_docker_probe_bridge_network`) AND the subject reaches
   it via `127.0.0.1`. The SAME port is baked into both bootstrap YAMLs.
2. `DriveReference` / `DriveSubject` each `Reset()` the receiver, fire N=8
   identical query-less requests, then POLL `Count()` to `>= 8` (poll-to-converge,
   never sleep — the reference buffers ALS entries and flushes on a ~1s timer;
   `reference_concurrency_differential_release_barrier`). Each side's entry set
   is snapshotted before the next side runs. Per-side separation is clean: the
   subject generates no access-log entries until its own `DriveSubject` window,
   so the post-`Reset()` accumulator holds exactly that side's entries.
3. `AssertStats` asserts the 7-field subset on every entry on BOTH sides, plus
   the subject-side `logs_written` stat. The receiver is then stopped in the
   background (GracefulStop blocks while the proxies hold their ALS streams open;
   the runner tears the proxies down immediately after).

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

## Framing NOT asserted (AMEND-ALS-3)

The cross-side assertion is on the per-entry PAYLOAD aggregated across all
received entries — NOT stream count, per-message batching, or flush cadence
(which legitimately vary side-to-side). The receiver accumulates `log_entry[]`
across all messages and streams; the driver asserts every accumulated entry.

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
`access_logs.grpc_access_log.logs_written == 8` (every send succeeded).

## UNasserted

- `common_properties.{start_time, duration, upstream_remote_address}` —
  populated but non-deterministic (AMEND-ALS-4).
- `identifier.node` — minimal node (Id+Cluster), D-ALS-NODE; UNasserted.
- stream / message / batch framing — AMEND-ALS-3.
- subject-absent reference fields: `request.scheme`, `request_id`,
  `upstream_cluster`, `access_log_type`, `response_code_details`, the wire-byte
  counts — not mapped by envoy-go's 10-field `Record` (SPEC §12 D-ALS-*).

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS via `host.docker.internal`;
  `c_als` carries `http2_protocol_options:{}` + plaintext h2c).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC via `127.0.0.1`).
- `driver/driver.go` — the single-listener driver: registers the fixture,
  manages the receiver lifecycle, drives both proxies (poll-to-converge), and
  asserts the 7-field subset cross-side + the subject `logs_written` stat.
- `expectations.yaml` — prose expectations (ADR-0019 — the driver is the
  enforcer; this file is documentation).

Cross-refs: phase 44.1 SPEC §12 + D-ALS-RECEIVER + AMEND-ALS-2/3/4 + D-ALS-NODE +
ADR-0255 + `reference_streaming_sink_differential_framing` +
`reference_differential_grpc_receiver_driver_owned`.
