# 0086-tracing-request-id

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for the **phase 46.1a HCM-native request-tracing HEADER engine**. There
is **no span and no OTLP export at 46.1a** — that lands at 46.1b. This fixture
proves only the request-header-level engine: the sampling/request-id decision and
the upstream-forwarded `x-request-id` + W3C `traceparent` injection.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
existing **HTTPHeaderMutation echo backend** (Kind 9,
`test/fixtures/0012-http-header-mutation/backends/backend.go`). The HCM carries a
`tracing` block:

```yaml
tracing:
  provider:
    name: envoy.tracers.opentelemetry
    typed_config:
      "@type": type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig
      grpc_service:
        envoy_grpc: { cluster_name: c_otlp_collector }
      service_name: "0086"
  random_sampling: { value: 100 }
```

`c_otlp_collector` is a **DUMMY** STATIC h2c cluster with an unreachable
`127.0.0.1:1` endpoint. The reference boots permissively and silently fails any
span export; envoy-go at 46.1a parses the provider's `cluster_name` but never
dials it (no exporter until 46.1b). The cluster only satisfies the reference data
plane and keeps the config **46.1b-ready**.

## Capture mechanism — HTTPHeaderMutation echo (NO receiver)

The echo backend reflects every received request header into the response body as
sorted `"Canonical-Name: value"` lines (Go's `net/http` canonicalizes header
names, so `x-request-id` → `X-Request-Id`, `traceparent` → `Traceparent`). The
driver parses each response body to recover the **upstream-forwarded**
`x-request-id` + `traceparent` that the proxy injected. No driver-owned receiver
is needed.

## Workload (poll-free, fixed traffic)

Each side fires, against the proxy under test:

- **N = 8 PLAIN** requests (fixed `Host: trace.example`, `User-Agent:
  trace-probe/1`, query-less path `/trace`) — no inbound trace context. Under
  `random_sampling: 100%` the decision is deterministic: each is a **fresh local
  sample** (`x-request-id` REASON nibble `'9'` Sampled; `traceparent` flags `01`).
- **M = 4 CONTINUATION** requests additionally carrying
  `Traceparent: 00-aaaa…aaaa-bbbb…bbbb-01` (a FIXED, non-zero inbound W3C trace
  context). The proxy **continues** the trace: the upstream `traceparent` keeps
  the inbound trace-id; the `x-request-id` REASON nibble is `'9'` (Sampled — a
  continued+sampled trace's nibble reflects the inbound `01` sampled bit, matching
  the reference; the COUNTER class stays `not_traceable`). The inbound id is
  deliberately NOT all-zero (all-zero trace-id is treated as no-incoming-context).

No polling / no sleep — the traffic is fixed and synchronous.

## Asserted (cross-side, both sides)

- Each PLAIN request: `x-request-id` PRESENT, 36-char UUID-shaped, string
  index-14 == `'9'`; `traceparent` matches `00-<32hex>-<16hex>-01`. (A
  zero-header pass is vacuous — presence proves injection RAN on BOTH sides.)
- Each CONTINUATION request: `traceparent` trace-id == the FIXED inbound id —
  the **cross-side EXACT** continuation invariant (both sides CONTINUE the trace,
  preserving the inbound trace-id and the inbound `01` sampled flag) — AND the
  `x-request-id` REASON nibble == `'9'` (Sampled), also **cross-side EXACT**: a
  continued+sampled trace (inbound flags `01`) reports the inbound sampled bit in
  its nibble, and envoy-go matches the reference (the SPEC §11 D-TRACE-REQUESTID
  probe-error correction, re-probed at the 46.1a IMPL). The COUNTER class stays
  `not_traceable` (subject `/stats` `not_traceable == 4`). Both sides pinned to
  `'9'` so a regression on either side bites.

PLUS the **SUBJECT** `/stats` (the reference emits different tracing stat names —
subject-only, like 0084's subject-specific OTLP stat):

```
http.hcm_local.tracing.random_sampling == 8
http.hcm_local.tracing.not_traceable   == 4
http.hcm_local.tracing.client_enabled  == 0
http.hcm_local.tracing.health_check    == 0
http.hcm_local.tracing.service_forced  == 0
```

## NOT asserted (vary side-to-side / non-deterministic)

The `x-request-id` / `traceparent` random VALUES (except the continuation
trace-id); the span-id; the reference's default-injected extras (`x-envoy-*` /
`x-forwarded-*`); message/stream framing. The reference's tracing stats are NOT
asserted cross-side (different stat names). The cross-side assertion is on the
echoed upstream HEADERS, which both sides produce.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- `disable`/`overall_sampling`/`client_sampling` and the health-check /
  service-forced classes are covered by `internal/tracing` unit tests, not this
  fixture.
