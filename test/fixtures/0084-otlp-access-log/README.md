# Fixture 0084 — OTLP access-log (`envoy.access_loggers.open_telemetry`) differential equivalence

The behavioral proof of the phase 45.1 core OTLP access-log streaming sink:
cross-side EXACT (subject envoy-go vs reference Envoy v1.37.2 in Docker) on the
bare built-in OTLP `LogRecord` — the record COUNT, the `time_unix_nano`
PRESENCE, and the four built-in Resource label keys (with the `log_name` VALUE),
aggregated across every exported record.

A single plaintext HTTP/1.1 listener (`l_test`) routes every request to a
fixed-body backend (`c_backend` — the phase-14/0006 `HTTPFixedBody` helper,
`backend:v1/fixed\n` = 17 bytes). The HCM carries one `access_log[]` entry of
type `OpenTelemetryAccessLogConfig` (`log_name: "0084"`,
`grpc_service.envoy_grpc.cluster_name: c_otlp`) with NO `body` / `attributes` and
builtin labels ENABLED — the pure built-in record. It exports one OTLP
`LogRecord` per completed request to a **driver-owned in-process `LogsService`
receiver** (`test/helpers/otlplogs/`). Plaintext h2c — no TLS — per
D-OTLP-RECEIVER. Reference Envoy v1.37.2 (STRICT_DNS via `host.docker.internal`)
vs envoy-go (STATIC, in-process via `127.0.0.1`).

## Driver-owned-receiver lifecycle

The `otlplogs.Server` is NOT a runner `BackendKind` — it is a driver-managed test
helper the proxy DIALS (`reference_differential_grpc_receiver_driver_owned`). The
driver:

1. Allocates a free TCP port at `ReferenceBootstrap` time (Listen+Close) and
   binds the receiver on `0.0.0.0:<port>` BEFORE the reference container starts
   — so the reference Envoy reaches it via `host.docker.internal` (the Docker
   bridge alias, `reference_docker_probe_bridge_network`) AND the subject reaches
   it via `127.0.0.1`. The SAME port is baked into both bootstrap YAMLs.
2. `DriveReference` / `DriveSubject` each `Reset()` the receiver, fire N=8
   identical query-less requests, then POLL `Count()` to `>= 8` (poll-to-converge,
   never sleep — the reference buffers OTLP records and flushes them on a timer /
   buffer-fill; `reference_concurrency_differential_release_barrier`). Each side's
   record set AND per-`ResourceLogs` `Resource.attributes` snapshots are captured
   before the next side runs. Per-side separation is clean: the subject generates
   no records until its own `DriveSubject` window, so the post-`Reset()`
   accumulator holds exactly that side's records.
3. `AssertStats` asserts the record count, `time_unix_nano` presence, and the
   four Resource label keys on BOTH sides, plus the subject-side `logs_written`
   stat. The receiver is then hard-stopped via `Close()` (immediate
   `grpc.Server.Stop`) for deterministic teardown — the records are already
   snapshotted.

## Host-reachability table

| consumer            | OTLP host (`{{.OTLPHost}}`) | backend host (`{{.BackendHost}}`) |
|---------------------|-----------------------------|-----------------------------------|
| reference (Docker)  | `host.docker.internal`      | `host.docker.internal`            |
| subject (host)      | `127.0.0.1`                 | `127.0.0.1`                       |

The receiver binds `0.0.0.0:<port>` so BOTH paths resolve to the same listener.

## Query-less path

The data-plane request uses `GET /health` (NO query string). The OTLP built-in
record carries no request fields, so the path itself is not asserted on the
record — but the query-less constraint is kept consistent with the 0081 ALS
precedent to keep the cross-side data-plane behavior identical.

## Framing NOT asserted

The cross-side assertion is on the per-record PAYLOAD aggregated across all
received records — NOT the Export-call count, per-call batch sizes, connection
count, or flush cadence (which legitimately vary side-to-side: the subject may
send one record per Export; the reference batches on its buffer-fill / flush
timer). The receiver accumulates `LogRecord`s across all Export calls and
streams; the driver asserts every accumulated record.

## Asserted (cross-side EXACT, both sides)

| assertion | value |
|---|---|
| `len(records)` | `8` (a zero-record pass is vacuous — proves decode ran on BOTH sides) |
| `LogRecord.time_unix_nano` (every record) | `!= 0` (PRESENCE, not value) |
| `Resource.attributes` keys (every `ResourceLogs`) | `{log_name, zone_name, cluster_name, node_name}` all present |
| `Resource.attributes` `log_name` value | `"0084"` |

PLUS the subject-side flat `/stats` counter
`access_logs.open_telemetry_access_log.logs_written == 8` (every export
succeeded).

## UNasserted

- `LogRecord.time_unix_nano` VALUE — non-deterministic.
- Export-call count / per-call batch sizes / connection count — framing, varies
  side-to-side.
- `LogRecord.severity` / `body` / `attributes` — absent on BOTH sides (the bare
  built-in record carries no body or per-record attributes).
- The node-derived Resource label VALUES (`zone_name`, `cluster_name`,
  `node_name`) — may be empty on BOTH sides under the no-node config; only their
  KEY presence is asserted.

## `disable_builtin_labels` — covered by a unit test

This fixture exercises the builtin-labels-ENABLED branch only. The
`disable_builtin_labels: true` branch is covered by a unit test, not a second
fixture: one fixture dir = one runner branch
(`reference_differential_fixture_dispatch_constraint`).

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS via `host.docker.internal`;
  `c_otlp` carries `http2_protocol_options:{}` + plaintext h2c).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC via `127.0.0.1`).
- `driver/driver.go` — the single-listener driver: registers the fixture,
  manages the receiver lifecycle, drives both proxies (poll-to-converge), and
  asserts the record count + `time_unix_nano` presence + the four Resource label
  keys + the `log_name` value cross-side + the subject `logs_written` stat.
- `expectations.yaml` — prose expectations (ADR-0019 — the driver is the
  enforcer; this file is documentation).

Cross-refs: phase 45.1 SPEC §12 D-OTLP-* + ADR-0258 +
`reference_streaming_sink_differential_framing` +
`reference_differential_grpc_receiver_driver_owned`.
