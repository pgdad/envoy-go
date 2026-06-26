# Fixture 0083 — gRPC access-log header capture differential equivalence

The behavioral proof of the phase 44.3 gRPC ALS **header-capture** feature:
cross-side EXACT (subject envoy-go vs reference Envoy v1.37.2 in Docker) on the
captured `request.request_headers` / `response.response_headers` maps of every
streamed `HTTPAccessLogEntry`, on top of the 0081 deterministic 7-field subset.

Topology is identical to 0081: a single plaintext HTTP/1.1 listener (`l_test`)
routes every request to a fixed-body backend (`c_backend` — the phase-14/0006
`HTTPFixedBody` helper, `backend:v1/fixed\n` = 17 bytes, `Content-Type:
text/plain`). The HCM carries one `access_log[]` entry of type
`HttpGrpcAccessLogConfig` (`log_name: "0083"`,
`grpc_service.envoy_grpc.cluster_name: c_als`) that streams one
`HTTPAccessLogEntry` proto per completed request to a **driver-owned in-process
`AccessLogService` receiver** (`test/helpers/accessloggrpc/`). Plaintext h2c —
no TLS — per D-ALS-RECEIVER. Reference Envoy v1.37.2 (STRICT_DNS via
`host.docker.internal`) vs envoy-go (STATIC, in-process via `127.0.0.1`).

## The only delta vs 0081: header-capture lists (44.3)

The OUTER `HttpGrpcAccessLogConfig` message (alongside `common_config`, NOT
inside it) carries the two header-capture lists — the feature under proof:

```yaml
additional_request_headers_to_log: ["x-req-foo", "x-req-missing", "x-req-multi"]
additional_response_headers_to_log: ["content-type"]
```

Both lists are LIVE. The capture is: lowercase key (AMEND-HDR-1), verbatim value,
multi-value comma-join (AMEND-HDR-3), omit-absent (a configured header that is
not present on the request/response is absent from the captured map, AMEND-HDR-2).

## Drive (N=8 sequential, header set)

Each side fires N=8 identical query-less probes:
`GET /health`, Host `als.example`, `User-Agent: als-probe/1`, plus the
header-capture set:

- `X-Req-Foo: bar` — the single-value capture.
- `X-Req-Multi: m1` + `X-Req-Multi: m2` — two values → captured as the
  comma-joined `"m1,m2"`.
- `X-Req-Missing` — configured in the request list but **NEVER set** on the
  probe: the configured-but-absent OMIT proof. Its key must NOT appear in the
  captured request map on either side (asserted explicitly).

The backend's `Content-Type: text/plain` response header is surfaced verbatim
downstream and captured into `response.response_headers`.

## Asserted captured maps (cross-side EXACT, every entry, both sides)

| map | value |
|---|---|
| `request.request_headers` | `{"x-req-foo": "bar", "x-req-multi": "m1,m2"}` (x-req-missing ABSENT) |
| `response.response_headers` | `{"content-type": "text/plain"}` |

PLUS the 0081 7-field subset (`request.request_method=GET`, `request.path=/health`,
`request.authority=als.example`, `request.user_agent=als-probe/1`,
`response.response_code=200`, `response.response_body_bytes=17`,
`protocol_version=HTTP11`) and the subject-side flat `/stats` counter
`access_logs.grpc_access_log.logs_written == 8`.

A `FIXTURE_0083_DUMP=1` env gate dumps the per-entry request/response header maps
on both sides — the non-vacuity proof (the maps are non-empty, so the
`reflect.DeepEqual` assertions are live).

## Backend-origin response headers only (AMEND-HDR-4)

The captured response header is `content-type` — a **backend-origin** header that
both reference Envoy and envoy-go pass through verbatim. Proxy-synthesized
response headers (`server`, `date`, `x-envoy-*`) are deliberately AVOIDED: their
values vary side-to-side (version strings, timestamps, proxy-specific) and would
diverge the cross-side response-map assertion.

## Host-reachability table

| consumer            | ALS host (`{{.ALSHost}}`) | backend host (`{{.BackendHost}}`) |
|---------------------|---------------------------|-----------------------------------|
| reference (Docker)  | `host.docker.internal`    | `host.docker.internal`            |
| subject (host)      | `127.0.0.1`               | `127.0.0.1`                       |

The receiver binds `0.0.0.0:<port>` so BOTH paths resolve to the same listener.

## Query-less path (AMEND-ALS-2) / framing NOT asserted (AMEND-ALS-3)

`GET /health` (no query) keeps `request.path` EXACT on both sides (envoy-go's
`Record.Path` is path-only). The cross-side assertion is on the per-entry PAYLOAD
aggregated across all received entries — NOT stream/message/batch framing, which
legitimately varies side-to-side.

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS via `host.docker.internal`;
  `c_als` carries `http2_protocol_options:{}` + plaintext h2c; the two
  header-capture lists on the OUTER `HttpGrpcAccessLogConfig`).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC via `127.0.0.1`; same lists).
- `driver/driver.go` — registers the fixture, manages the receiver lifecycle,
  drives both proxies (poll-to-converge), asserts the 7-field subset + the two
  captured maps cross-side + the subject `logs_written` stat.
- `expectations.yaml` — prose expectations (ADR-0019 — the driver is the
  enforcer; this file is documentation).

Cross-refs: phase 44.1 SPEC §12 + phase 44.3 PLAN (D-HDR-*) + AMEND-HDR-1/2/3/4 +
D-ALS-RECEIVER + AMEND-ALS-2/3/4 + D-ALS-NODE +
`reference_streaming_sink_differential_framing` +
`reference_differential_grpc_receiver_driver_owned`.
