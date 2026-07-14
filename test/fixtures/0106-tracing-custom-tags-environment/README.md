# 0106-tracing-custom-tags-environment

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 63 environment `custom_tags`** (ADR-0284). Cloned from
`0105-tracing-custom-tags-request-header` (phase 62 request_header custom_tags
precedent, itself cloned from `0102-tracing-custom-tags-literal` / phase 59
literal custom_tags precedent / `0087-tracing-otlp`, phase 46.1b span emission +
OTLP export); this fixture swaps the request_header custom-tag source for the
**environment** source — the HCM `tracing` block's `custom_tags` entry resolves
its value from a process env var (`PATH`) rather than an inbound request header
or a static config-time literal, and the resolved value is emitted as an OTLP
span attribute, by key, on **every** exported span, cross-side.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes). The HCM carries a
`tracing` block:

```yaml
tracing:
  provider:
    name: envoy.tracers.opentelemetry
    typed_config:
      "@type": type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig
      grpc_service:
        envoy_grpc: { cluster_name: c_otlp_collector }
      service_name: "0106"
  random_sampling: { value: 100 }
  custom_tags:
  - tag: env_path
    environment:
      name: PATH
```

`custom_tags` is a **sibling of `provider`** — a single environment entry with a
NON-colliding key (`env_path`), so both sides converge on a clean pass. `PATH`
is naturally present + non-empty in BOTH the reference container and the
subject subprocess (which inherits `os.Environ()`), but the VALUE differs per
side (container `PATH` != subject `PATH`). The driver drives a PLAIN GET (no
request-header manipulation — an environment tag needs no request header). The
`default_value` / absent / present-empty / dedup edges are exercised by the
deterministic `internal/tracing/resolve_test.go` / `config_test.go` unit tests,
not this fixture. The collision/override case (a `custom_tags` key matching a
well-known span attribute name) and the custom-vs-custom first-wins dedup case
are exercised by the phase 63 Zipkin/span encoder unit tests, not this
fixture.

`c_otlp_collector` is a **real** STRICT_DNS (reference) / STATIC (envoy-go) h2c
cluster pointing at the **driver-owned in-process `otlptrace.Server`** receiver.
The receiver is bound on `0.0.0.0:<port>` before either proxy starts, so the
reference container can reach it at `host.docker.internal:<port>` (per ADR-0010)
and envoy-go at `127.0.0.1:<port>`.

## Capture mechanism — driver-owned OTLP TraceService receiver

`test/helpers/otlptrace` (`otlptrace.NewAtAddr`) is a minimal in-process
`coltracepb.TraceServiceServer` that accumulates every `*tracepb.Span` across
all `Export` calls (flattening the `ResourceSpans/ScopeSpans` nesting). The driver
uses `Count()` to poll for convergence and `Spans()`/`ResourceAttributes()` to
snapshot before `Reset()` for per-side separation. This is the trace counterpart of
the `otlplogs` receiver used in fixture 0084.

## Workload — N + M = 12 requests per side

Each side fires, against the proxy under test:

- **N = 8 PLAIN** requests (`GET /trace`, `Host: trace.example`,
  `User-Agent: trace-probe/1`, no inbound trace context). Under
  `random_sampling: 100%` the decision is deterministic: each is a **fresh local
  sample** — a new trace-id, no parent span id.
- **M = 4 CONTINUATION** requests additionally carrying
  `Traceparent: 00-aaaa…aaaa-bbbb…bbbb-01` (a FIXED, non-zero inbound W3C trace
  context). The proxy **continues** the trace: the exported span carries
  `trace_id == aaaa…aaaa` (the fixed inbound id) and
  `parent_span_id == bbbb…bbbb` (the fixed inbound parent span id).

The environment custom tag applies uniformly to every span regardless of
prong (plain or continuation) — the tag is resolved from the process env var
`PATH`, not from anything in the request, so every span carries the same
(non-empty) resolved value on a given side.

## `random_sampling: 100%` determinism

Setting `random_sampling: { value: 100 }` ensures that EVERY request generates a
span. This makes the total span count deterministic at N+M=12 per side — no
sampling-based flakiness.

## Release barrier — POLL, never sleep

The reference Envoy buffers spans and flushes them on a timer / buffer-fill. The
driver polls `srv.Count() >= 12` at 200ms intervals with a 30s deadline (the
`reference_concurrency_differential_release_barrier` discipline). A fixed sleep
would be flaky (too short on slow CI) or slow (too long on fast hardware).

## Decode-ran proof

The `Count()` poll guarantees spans > 0 on BOTH sides before asserting. A
0-span "pass" (failed export, misconfigured cluster) is structurally impossible.

## Framing NOT asserted

The cross-side assertion is on the **per-span PAYLOAD aggregated across all
`Export` calls** — NOT the `Export`-call count, per-call batch sizes, connection
count, or flush cadence. These legitimately vary side-to-side
(`reference_streaming_sink_differential_framing`).

## Continuation prong — trace-id invariant

The M=4 continuation spans are identified by `trace_id == contTraceID` bytes
(`aaaa…aaaa`). For each, the driver asserts `parent_span_id == contParentID`
(`bbbb…bbbb`) — the cross-side EXACT continuation invariant: both sides CONTINUE
the inbound trace and set the parent span id to the inbound parent.

## Environment custom tag — asserted by KEY + value-non-empty, every span, both sides (phase 63)

`assertCustomTag` iterates every span (both prongs, both sides) and asserts
`attrs["env_path"]` is **present** and its string value is **non-empty**,
matched **by key** — OTLP attribute order is non-deterministic (SPEC §11), so
the assertion never depends on attribute position. The VALUE is NOT asserted
for equality: the resolved `PATH` differs between the reference Docker
container and the subject Go subprocess (D-ENV-HARNESS value-injection is
deferred, SPEC §8). Unlike the other `t.Fatalf`-based per-span checks in this
fixture, `assertCustomTag` uses `t.Errorf` per property (continues past a
single span's failure) so one bad span does not mask assertion failures on the
rest — useful when debugging a partial-rollout regression across N+M spans.

## `upstream_cluster` / `upstream_cluster.name` — framework-gap deviation

envoy-go's upstream cluster name is **not plumbed** to the request-completion
seam where the span is built (the same gap as the Lua bridge's `UpstreamCluster()`
which returns `""`). envoy-go therefore emits:

```
upstream_cluster       = ""
upstream_cluster.name  = ""
```

while the reference Envoy emits `"c_backend"`. The attribute **KEYS** are present
on both sides; the **VALUES** differ and are explicitly **UNasserted cross-side**.
The subject-side driver only asserts KEY presence (not value). This deviation is
intentional and documented as a known framework gap — it will be addressed in a
future phase that plumbs the cluster name through the request context.

## Attribute value-type parity (`http.status_code`, `request_size`, `response_size`)

envoy-go's `span.go` currently emits `http.status_code`, `request_size`, and
`response_size` as `AnyValue_IntValue` (INT). The reference Envoy cpp OTel tracer
may emit these as `AnyValue_StringValue` (STRING). The driver uses **normalized
comparison** (`assertAttrNormalized`) — converting both INT and STRING to their
decimal string form before comparison — so the assertion is type-agnostic and the
differential passes regardless of which form either side uses. If you observe that
the reference uses STRING, adjust `internal/tracing/span.go` to match.

## SDK / scope NOT asserted

`telemetry.sdk.*` resource attributes and `ScopeSpans.scope.name`/`version` are
impl-specific (envoy-go is not a cpp SDK). These are intentionally UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- This fixture CLOSES ADR-0284 at IMPL (phase 63 — environment `custom_tags`
  parse + per-request resolve via `os.LookupEnv` + span-emit upsert), the THIRD
  custom_tag source type after phase 59's literal
  (`0102-tracing-custom-tags-literal`, ADR-0277) and phase 62's request_header
  (`0105-tracing-custom-tags-request-header`, ADR-0283). The tracing family
  traces back to phase 46/46.1a/46.1b (which closed ADR-0260, tracked by
  sibling fixture `0087-tracing-otlp`).
- Do NOT mutate `0087-tracing-otlp`, `0088-tracing-zipkin`,
  `0102-tracing-custom-tags-literal`, or
  `0105-tracing-custom-tags-request-header` — this fixture is a full clone in
  its own directory, its own package, its own runner branch.
