# Fixture 0085 — OTLP access-log OPERATOR ENGINE differential equivalence

The behavioral proof of the phase 45.2 OTLP access-log operator engine:
cross-side EXACT (subject envoy-go vs reference Envoy v1.37.2 in Docker) on the
command-operator-TEMPLATED OTLP `LogRecord` — the substituted `body`, the
substituted per-record `attributes` (including a nested kvlist and an array),
and the LITERAL `resource_attributes` pass-through — aggregated across every
exported record.

The topology is identical to 0084 (a single plaintext HTTP/1.1 listener `l_test`
routing to the `c_backend` `HTTPFixedBody` helper, `backend:v1/fixed\n` = 17
bytes; one `OpenTelemetryAccessLogConfig` access_log entry exporting to the
**driver-owned in-process `LogsService` receiver** `test/helpers/otlplogs/`;
plaintext h2c, no TLS, per D-OTLP-RECEIVER). The ONLY difference is the
access-logger config, which here carries operator-templated content.

## Access-logger templating (D-OTLP-2-FIXTURE-SHAPE)

| field | shape |
|---|---|
| `body` | `string_value: "%REQ(:METHOD)% %REQ(:PATH)% %PROTOCOL% %RESPONSE_CODE% %BYTES_SENT%"` |
| `attributes.op_method` | `string_value: "%REQ(:METHOD)%"` |
| `attributes.nested` | `kvlist_value` { `inner_code: "%RESPONSE_CODE%"`, `inner_authority: "%REQ(:AUTHORITY)%"` } |
| `attributes.arr` | `array_value` [ `"%REQ(:METHOD)%"`, `"literal-elem"` ] |
| `resource_attributes.service_name` | `string_value: "envoy-go-test"` (static literal) |
| `resource_attributes.authority_literal` | `string_value: "%REQ(:AUTHORITY)%"` (literal pass-through) |

The `body`/`attributes` operators are evaluated per record. The
`resource_attributes` are LITERAL: `authority_literal` carries a
`%REQ(:AUTHORITY)%`-LOOKING string that is emitted VERBATIM (NOT
operator-substituted) — request operators in `resource_attributes` pass through
unchanged (**AMEND-OPS-1**).

## Driver-owned-receiver lifecycle

Identical to 0084: the `otlplogs.Server` is a driver-managed test helper the
proxy DIALS (`reference_differential_grpc_receiver_driver_owned`). The driver
allocates a free TCP port at `ReferenceBootstrap` time, binds the receiver on
`0.0.0.0:<port>` BEFORE the reference container starts (so the reference reaches
it via `host.docker.internal` and the subject via `127.0.0.1`), fires N=8
identical query-less requests per side, POLLS `Count()` to `>= 8`
(poll-to-converge, never sleep), snapshots each side's records +
`Resource.attributes`, and asserts cross-side in `AssertStats`. The receiver is
hard-stopped via `Close()` for deterministic teardown.

## Host-reachability table

| consumer            | OTLP host (`{{.OTLPHost}}`) | backend host (`{{.BackendHost}}`) |
|---------------------|-----------------------------|-----------------------------------|
| reference (Docker)  | `host.docker.internal`      | `host.docker.internal`            |
| subject (host)      | `127.0.0.1`                 | `127.0.0.1`                       |

The receiver binds `0.0.0.0:<port>` so BOTH paths resolve to the same listener.

## Query-less path (AMEND-OPS-6)

The data-plane request uses `GET /health` (NO query string). envoy-go's H1
`Record.Path` strips the query, so cross-side EXACT on the
`%REQ(:PATH)%`-substituted `body` REQUIRES a query-less path — a query would
desync the subject (path stripped) vs the reference (full target).

## Deterministic operators only (cross-side note)

Only deterministic operators are templated. `%START_TIME%`, `%DURATION%`, and
`%UPSTREAM_HOST%` are EXCLUDED from the config — they are non-deterministic /
connection-keyed and would break cross-side EXACT
(`reference_streaming_sink_differential_framing`).

## Framing NOT asserted

The cross-side assertion is on the per-record PAYLOAD aggregated across all
received records — NOT the Export-call count, per-call batch sizes, connection
count, or flush cadence (which legitimately vary side-to-side).

## Asserted (cross-side EXACT, both sides)

| assertion | value |
|---|---|
| `len(records)` | `8` (zero-record pass is vacuous — proves the engine ran on BOTH sides) |
| `body` (every record) | `"GET /health HTTP/1.1 200 <bytesSent>"` — method/path/protocol/code literal; `%BYTES_SENT%` asserted **ref==subj** (cross-side EXACT, robust vs hardcoding the byte count); all 8 records on a side identical |
| `attributes.op_method` | `"GET"` |
| `attributes.nested.inner_code` | `"200"` |
| `attributes.nested.inner_authority` | `"otlp.example"` |
| `attributes.arr` | `["GET", "literal-elem"]` |
| `Resource.attributes` built-in keys | `{log_name, zone_name, cluster_name, node_name}` all present |
| `Resource.attributes.log_name` | `"0085"` |
| `Resource.attributes.service_name` | `"envoy-go-test"` |
| `Resource.attributes.authority_literal` | `"%REQ(:AUTHORITY)%"` (LITERAL, un-substituted) |

PLUS the subject-side flat `/stats` counter
`access_logs.open_telemetry_access_log.logs_written == 8`.

## UNasserted

- `LogRecord.time_unix_nano` VALUE — non-deterministic.
- Export-call count / batch sizes / connection count — framing.
- `%START_TIME%` / `%DURATION%` / `%UPSTREAM_HOST%` — excluded from the config.
- The node-derived Resource label VALUES (`zone_name`, `cluster_name`,
  `node_name`) — may be empty on BOTH sides under the no-node config; only KEY
  presence asserted.

## Subject-side unit coverage (NOT a second fixture)

One fixture dir = one runner branch
(`reference_differential_fixture_dispatch_constraint`), so the non-cross-side
behaviors are unit-tested rather than re-fixtured:

- `disable_builtin_labels` + `resource_attributes` survival — the
  `internal/accesslog` `otlpsink_test.go`
  `TestOTLPSink_DisableBuiltinResourceAttrsSurvive` case (**AMEND-OPS-5**: the 4
  built-in Resource labels drop but the literal `resource_attributes` SURVIVE,
  and the templated body/attributes still land).
- The unsupported-operator + bad-value-type boot-rejects — the
  `internal/bootstrap` `bootstrap_test.go` `reject-unknown-operator` /
  `reject-bad-value-type-{body,attribute,resource}` cases (the envoy-go-strict
  mirror of the reference's own unknown-operator boot-reject).

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS via `host.docker.internal`).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC via `127.0.0.1`).
- `driver/driver.go` — the single-listener driver: registers the fixture, manages
  the receiver lifecycle, drives both proxies (poll-to-converge), and asserts the
  templated body + attributes (incl. nested kvlist / array) + the literal
  resource_attributes + the built-in Resource labels cross-side, plus the subject
  `logs_written` stat.
- `expectations.yaml` — prose expectations (ADR-0019 — the driver is the enforcer).

Cross-refs: phase 45.2 SPEC §12 D-OTLP-2-* + ADR-0259 +
`reference_streaming_sink_differential_framing` +
`reference_differential_grpc_receiver_driver_owned`.
