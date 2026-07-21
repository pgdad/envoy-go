# 0114-tracing-custom-tags-metadata

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 70 REQUEST `metadata` `custom_tags`** (ADR-0292). Cloned from
`0106-tracing-custom-tags-environment` (the OTLP tracing custom_tags chassis;
itself cloned from `0105`/`0102`/`0087-tracing-otlp`) + the
`0027-http-lua-full-bridge` Lua http_filter (a `dynamicMetadata():set` writer).
This fixture exercises the **fourth and last** `CustomTag` source type — a
`metadata`-kind tag resolves a value out of the per-request dynamic-metadata
bucket and emits it as an OTLP span attribute on **every** exported span,
cross-side.

## Why this is STRONGER than the 0106 environment tag

`0106` asserts the environment tag `env_path` only by **key + value-non-empty**
because the process `PATH` differs between the reference Docker container and the
subject Go subprocess. This fixture asserts **key AND value, cross-side EXACT**:
a Lua http_filter runs **before the router on both sides** and writes a FIXED,
cross-side-identical value into the dynamic-metadata bucket, so the resolved
span-attribute value is byte-identical on both sides.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes). The HCM carries a
`tracing` block and a Lua http_filter ahead of the router:

```yaml
http_filters:
  - name: envoy.filters.http.lua
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
      stat_prefix: meta_writer
      default_source_code:
        inline_string: |
          function envoy_on_request(handle)
            handle:streamInfo():dynamicMetadata():set("envoy.test", "meta_k", "v-meta-0114")
          end
  - name: envoy.filters.http.router
    ...
tracing:
  provider: { name: envoy.tracers.opentelemetry, ... service_name: "0114" }
  random_sampling: { value: 100 }
  custom_tags:
  - tag: meta_hit
    metadata:
      kind: { request: {} }
      metadata_key: { key: envoy.test, path: [ { key: meta_k } ] }
      default_value: unused-default-0114
  - tag: meta_default
    metadata:
      kind: { request: {} }
      metadata_key: { key: envoy.test, path: [ { key: absent_k } ] }
      default_value: fallback-0114
```

The Lua source lives in `scripts/writer.lua` and is embedded into **both**
bootstraps via `strconv.Quote` (a valid double-quoted YAML scalar) at render
time — so there is **no container bind-mount** and both sides run byte-identical
script.

- **`meta_hit`** resolves `metadata_key {key: envoy.test, path: [meta_k]}` → the
  Lua-set value `"v-meta-0114"`. The configured `default_value` is **never used**
  — the span carrying the Lua value (not the default) is the
  **writer-served-this-arm** proof (`feedback_probe_fresh_container_per_arm`).
- **`meta_default`** points its path at an UNSET key (`absent_k`) → the
  `default_value` `"fallback-0114"` is emitted (the `request_header` default
  rule: absent/unresolvable → `default_value`).

`c_otlp_collector` is a **real** STRICT_DNS (reference) / STATIC (envoy-go) h2c
cluster pointing at the **driver-owned in-process `otlptrace.Server`** receiver,
bound on `0.0.0.0:<port>` before either proxy starts.

## Capture mechanism — driver-owned OTLP TraceService receiver

`test/helpers/otlptrace` (`otlptrace.NewAtAddr`) is a minimal in-process
`coltracepb.TraceServiceServer` that accumulates every `*tracepb.Span` across all
`Export` calls. The driver uses `Count()` to poll for convergence and
`Spans()`/`ResourceAttributes()` to snapshot before `Reset()` for per-side
separation.

## Workload — N + M = 12 requests per side

- **N = 8 PLAIN** requests (`GET /trace`, `Host: trace.example`,
  `User-Agent: trace-probe/1`, no inbound trace context). Under
  `random_sampling: 100%` each is a fresh local sample.
- **M = 4 CONTINUATION** requests additionally carrying
  `Traceparent: 00-aaaa…aaaa-bbbb…bbbb-01`. The proxy continues the trace.

The Lua filter writes the same dynamic-metadata value on EVERY request, so every
span (both prongs, both sides) carries the same resolved `meta_hit`.

## Asserted — the two metadata tags (phase 70)

`assertMetaTags` iterates every span (both prongs, both sides) and asserts, by
KEY (OTLP attribute order is non-deterministic) AND VALUE:

- `attrs["meta_hit"]` == `"v-meta-0114"` — the Lua-set dynamic-metadata value.
- `attrs["meta_default"]` == `"fallback-0114"` — the absent-path `default_value`.

Each is an independent `t.Errorf` per property (continue past one span's failure
so one bad span does not mask assertion failures on the rest —
`reference_fatalf_makes_assertions_unreachable`). The `meta_hit` VALUE-equality
is the **writer-served-this-arm** proof: the span carries the Lua value, NOT the
vacuous configured default `"unused-default-0114"`.

The remaining per-span structure/attribute subset, the continuation-prong
trace-id invariant, the `service.name` Resource attr, and the subject-side
`tracing.opentelemetry.spans_sent` / `spans_dropped` stats are inherited from the
`0106` chassis (see `expectations.yaml`).

## `random_sampling: 100%` determinism / release barrier / decode-ran proof

Setting `random_sampling: { value: 100 }` makes the span count deterministic at
N+M=12 per side. The driver polls `srv.Count() >= 12` at 200ms intervals with a
30s deadline (`reference_concurrency_differential_release_barrier`) — never a
fixed sleep. The `Count()` poll guarantees spans > 0 on BOTH sides before
asserting — a 0-span pass is structurally impossible.

## `upstream_cluster` / attribute value-type parity / SDK-scope

Same as `0106`: `upstream_cluster` / `upstream_cluster.name` values are a
documented framework gap (KEY present, VALUE UNasserted); `http.status_code` /
`request_size` / `response_size` use normalized (int-or-string) comparison;
`telemetry.sdk.*` + `ScopeSpans.scope.*` are impl-specific and UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- This fixture is the behavioral proof for phase 70 (ADR-0292 — REQUEST
  `metadata` `custom_tags` parse + per-request resolve out of the
  dynamic-metadata `Bucket` + span-emit upsert), the FOURTH `custom_tags` source
  type after phase 59 literal (`0102`, ADR-0277), phase 62 request_header
  (`0105`, ADR-0283), and phase 63 environment (`0106`, ADR-0284). The Lua
  `dynamicMetadata():set` writer traces to phase 22.2 (`0027`). The tracing
  family traces back to phase 46/46.1a/46.1b (ADR-0260, `0087-tracing-otlp`).
- Do NOT mutate `0087`, `0088`, `0102`, `0105`, `0106`, or `0027` — this fixture
  is a full clone in its own directory, its own package, its own runner branch.
