# 0115-tracing-custom-tags-metadata-route

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 71 ROUTE `metadata` `custom_tags`** (ADR-0293). Cloned from
`0114-tracing-custom-tags-metadata` (the OTLP tracing custom_tags REQUEST
metadata chassis; itself cloned from `0106`/`0105`/`0102`/`0087-tracing-otlp`)
**WITHOUT** the `0027-http-lua-full-bridge` Lua http_filter writer. This fixture
exercises the **second** `metadata` `MetadataKind` — a `route`-kind tag resolves
a value out of the matched route's **static config metadata**
(`route.metadata.filter_metadata`) and emits it as an OTLP span attribute on
**every** exported span, cross-side.

## Why this needs NO writer (unlike 0114)

`0114`'s REQUEST metadata tag reads the **per-request dynamic-metadata bucket**,
which only a runtime writer (there: a Lua `dynamicMetadata():set` filter) can
populate identically on both sides. This fixture's ROUTE metadata tag instead
reads the matched route's **static config metadata** — a field parsed straight
out of the bootstrap YAML at HCM build time, present on **both** sides'
byte-identical `route.metadata.filter_metadata` block. No runtime write is
needed for the two sides to agree: the resolved span-attribute value is
cross-side EXACT by construction.

## Route-metadata YAML placement (verified against the parser)

`metadata` is a **sibling** of `match:`/`route:` on the `Route` message:

```yaml
routes:
  - match: { prefix: "/" }
    metadata:
      filter_metadata:
        envoy.test:
          route_k: v-route-0115
    route: { cluster: c_backend }
```

Verified against `internal/filter/hcm/config.go:616` —
`metadata: r.GetMetadata()` populates `routeEntry.metadata` straight from the
parsed `*routepb.Route`'s `Metadata` field at HCM route-table build time. The
matched `routeEntry.metadata` is seeded onto the `FilterChain` via
`SetRouteMetadata` (`internal/filter/hcm/{connection,h2dispatch,h3dispatch}.go`)
before `RunDecodeHeaders`, and read by the ROUTE `metadata` custom_tags resolver
via `(*FilterChain).RouteMetaLookup` (`internal/filter/http/chain.go`).

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes). The matched route
carries the static `metadata.filter_metadata` block above. The HCM carries a
`tracing` block (no Lua filter — only the router):

```yaml
http_filters:
  - name: envoy.filters.http.router
    ...
tracing:
  provider: { name: envoy.tracers.opentelemetry, ... service_name: "0115" }
  random_sampling: { value: 100 }
  custom_tags:
  - tag: route_hit
    metadata:
      kind: { route: {} }
      metadata_key: { key: envoy.test, path: [ { key: route_k } ] }
      default_value: unused-default-0115
  - tag: route_default
    metadata:
      kind: { route: {} }
      metadata_key: { key: envoy.test, path: [ { key: absent_k } ] }
      default_value: fallback-0115
```

- **`route_hit`** resolves `metadata_key {key: envoy.test, path: [route_k]}` →
  the static route value `"v-route-0115"`. The configured `default_value` is
  **never used** — the span carrying the static value (not the default) is the
  **route-metadata-served** proof.
- **`route_default`** points its path at an UNSET key (`absent_k`) → the
  `default_value` `"fallback-0115"` is emitted (the absent-path default rule).

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

The route's static metadata is the same on EVERY request (both prongs, both
sides — it is config, not per-request state), so every span carries the same
resolved `route_hit`.

## Asserted — the two metadata tags (phase 71)

`assertRouteTags` iterates every span (both prongs, both sides) and asserts, by
KEY (OTLP attribute order is non-deterministic) AND VALUE:

- `attrs["route_hit"]` == `"v-route-0115"` — the static route-metadata value.
- `attrs["route_default"]` == `"fallback-0115"` — the absent-path `default_value`.

Each is an independent `t.Errorf` per property (continue past one span's failure
so one bad span does not mask assertion failures on the rest —
`reference_fatalf_makes_assertions_unreachable`). The `route_hit` VALUE-equality
is the **route-metadata-served** proof: the span carries the static route
value, NOT the vacuous configured default `"unused-default-0115"`.

The remaining per-span structure/attribute subset, the continuation-prong
trace-id invariant, the `service.name` Resource attr, and the subject-side
`tracing.opentelemetry.spans_sent` / `spans_dropped` stats are inherited from the
`0114` chassis (see `expectations.yaml`).

## `random_sampling: 100%` determinism / release barrier / decode-ran proof

Setting `random_sampling: { value: 100 }` makes the span count deterministic at
N+M=12 per side. The driver polls `srv.Count() >= 12` at 200ms intervals with a
30s deadline (`reference_concurrency_differential_release_barrier`) — never a
fixed sleep. The `Count()` poll guarantees spans > 0 on BOTH sides before
asserting — a 0-span pass is structurally impossible.

## `upstream_cluster` / attribute value-type parity / SDK-scope

Same as `0114`: `upstream_cluster` / `upstream_cluster.name` values are a
documented framework gap (KEY present, VALUE UNasserted); `http.status_code` /
`request_size` / `response_size` use normalized (int-or-string) comparison;
`telemetry.sdk.*` + `ScopeSpans.scope.*` are impl-specific and UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- This fixture is the behavioral proof for phase 71 (ADR-0293 — ROUTE
  `metadata` `custom_tags` parse + per-request resolve out of the matched
  route's static config metadata + span-emit upsert), the SECOND `metadata`
  `MetadataKind` after REQUEST (phase 70, `0114`, ADR-0292), and the FIFTH
  `custom_tags` source/kind combination overall after phase 59 literal
  (`0102`, ADR-0277), phase 62 request_header (`0105`, ADR-0283), phase 63
  environment (`0106`, ADR-0284), and phase 70 REQUEST metadata (`0114`,
  ADR-0292). The tracing family traces back to phase 46/46.1a/46.1b
  (ADR-0260, `0087-tracing-otlp`).
- Do NOT mutate `0087`, `0088`, `0102`, `0105`, `0106`, `0114`, or `0027` — this
  fixture is a full clone in its own directory, its own package, its own
  runner branch.
- **No `scripts/` directory, no Lua** — the resolved value is cross-side EXACT
  by static config alone.
