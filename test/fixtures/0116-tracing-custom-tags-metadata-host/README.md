# 0116-tracing-custom-tags-metadata-host

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 72 HOST `metadata` `custom_tags`** (ADR-0294). Cloned from
`0115-tracing-custom-tags-metadata-route` (the OTLP tracing custom_tags ROUTE
metadata chassis; itself cloned from `0114`/`0106`/`0105`/`0102`/`0087-tracing-otlp`)
with the static metadata block **relocated from the matched route onto the
cluster's `lb_endpoints[0]`**. This fixture exercises the **third** `metadata`
`MetadataKind` — a `host`-kind tag resolves a value out of the **selected
upstream endpoint's static config metadata**
(`lb_endpoints[].metadata.filter_metadata`) and emits it as an OTLP span
attribute on **every** exported span, cross-side.

## Why this needs NO writer (as in 0115)

`0114`'s REQUEST metadata tag reads the **per-request dynamic-metadata bucket**,
which only a runtime writer (there: a Lua `dynamicMetadata():set` filter) can
populate identically on both sides. This fixture's HOST metadata tag instead
reads the selected endpoint's **static config metadata** — a field parsed
straight out of the bootstrap YAML at cluster build time, present on **both**
sides' byte-identical `lb_endpoints[0].metadata.filter_metadata` block. No
runtime write is needed for the two sides to agree: the resolved span-attribute
value is cross-side EXACT by construction.

## Endpoint-metadata YAML placement (verified against the parser)

`metadata` is a **sibling** of `endpoint:` on the `LbEndpoint` message, at the
**same indent**:

```yaml
load_assignment:
  cluster_name: c_backend
  endpoints:
    - lb_endpoints:
        - endpoint:
            address:
              socket_address: { address: 127.0.0.1, port_value: 12345 }
          metadata:
            filter_metadata:
              envoy.test:
                host_k: v-host-0116
```

Verified against `internal/cluster/manager.go:884` —
`filterMetadata: lbe.GetMetadata().GetFilterMetadata()` retains **all**
namespaces (aliasing the already-parsed proto map, zero new allocation) on
`cluster.Endpoint` at cluster-build time. It is read back through
`(cluster.Endpoint).MetaLookup(ns)` (`internal/cluster/cluster.go`), threaded as
the **5th** `ResolveCustomTags` argument from the **selected** endpoint at the
three span-emit sites (`internal/filter/hcm/accesslog_emit.go:57/:118/:179`).
The phase-38 `envoy.lb` scalar projection (`Endpoint.Metadata`, the subset-LB
dimension) is a **different, byte-unchanged** field.

Note the route here carries **no** `metadata` block at all — `0115`'s source is
deliberately removed so a leftover ROUTE source cannot mask the ENDPOINT source
(break Q discriminates exactly that).

## ⚠️ New cross-side ground: a non-`envoy.lb` namespace

The landed endpoint-metadata cross-side precedent, **`0064-lb-subset`**, also
puts `lb_endpoints[].metadata` on a STRICT_DNS reference cluster — but covers
the **`envoy.lb` namespace only** (the subset-LB dimension). *(That fixture has
**no** `envoy.yaml`/`envoy-go.yaml`; it is driver-GENERATED with inline YAML
strings in its `driver/driver.go`.)* Namespace generality — that a HOST-kind
`custom_tag` can address an **arbitrary** `filter_metadata` namespace — rested
on the phase-72 SPEC's live probe **P2** alone. This fixture uses `envoy.test`
and is the **cross-side proof**: the reference resolves it, and the `host_hit`
VALUE-equality assertion is green on the reference side.

## ⚠️ String values only

A struct-valued or numeric metadata value is **not** cross-side comparable
(`reference_structpb_tag_cross_side_string_only`): the reference serializes
multi-key structs in an **arbitrary** key order while Go always sorts,
scalar-vs-nested numbers use **different** reference renderers, and top-level
scalar numbers render at ~6 significant digits on the reference vs full
precision in Go. (Envoy's YAML loader also coerces any non-integer scalar to a
string.) Both asserted values here are plain strings.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes), which carries
**exactly one** `lb_endpoint` (so the LB pick is deterministic and no
round-robin spread enters the assertion —
`reference_round_robin_offset_randomized`) with the static
`metadata.filter_metadata` block above. The HCM carries a `tracing` block (no
Lua filter — only the router):

```yaml
http_filters:
  - name: envoy.filters.http.router
    ...
tracing:
  provider: { name: envoy.tracers.opentelemetry, ... service_name: "0116" }
  random_sampling: { value: 100 }
  custom_tags:
  - tag: host_hit
    metadata:
      kind: { host: {} }
      metadata_key: { key: envoy.test, path: [ { key: host_k } ] }
      default_value: unused-default-0116
  - tag: host_default
    metadata:
      kind: { host: {} }
      metadata_key: { key: envoy.test, path: [ { key: absent_k } ] }
      default_value: fallback-0116
```

- **`host_hit`** resolves `metadata_key {key: envoy.test, path: [host_k]}` →
  the static endpoint value `"v-host-0116"`. The configured `default_value` is
  **never used** — the span carrying the static value (not the default) is the
  **host-metadata-served** proof.
- **`host_default`** points its path at an UNSET key (`absent_k`) → the
  `default_value` `"fallback-0116"` is emitted (the absent-path default rule).

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

The endpoint's static metadata is the same on EVERY request (both prongs, both
sides — it is config, not per-request state), so every span carries the same
resolved `host_hit`.

## Asserted — the two metadata tags (phase 72)

`assertHostTags` iterates every span (both prongs, both sides) and asserts, by
KEY (OTLP attribute order is non-deterministic) AND VALUE:

- `attrs["host_hit"]` == `"v-host-0116"` — the static endpoint-metadata value.
- `attrs["host_default"]` == `"fallback-0116"` — the absent-path `default_value`.

Each is an independent `t.Errorf` per property (continue past one span's failure
so one bad span does not mask assertion failures on the rest —
`reference_fatalf_makes_assertions_unreachable`). The `host_hit` VALUE-equality
is the **host-metadata-served** proof: the span carries the static endpoint
value, NOT the vacuous configured default `"unused-default-0116"`.

The remaining per-span structure/attribute subset, the continuation-prong
trace-id invariant, the `service.name` Resource attr, and the subject-side
`tracing.opentelemetry.spans_sent` / `spans_dropped` stats are inherited from the
`0115`/`0114` chassis (see `expectations.yaml`).

## `random_sampling: 100%` determinism / release barrier / decode-ran proof

Setting `random_sampling: { value: 100 }` makes the span count deterministic at
N+M=12 per side. The driver polls `srv.Count() >= 12` at 200ms intervals with a
30s deadline (`reference_concurrency_differential_release_barrier`) — never a
fixed sleep. The `Count()` poll guarantees spans > 0 on BOTH sides before
asserting — a 0-span pass is structurally impossible.

## Out of scope — the pick-time-vs-acquire-success departure

The reference resolves a HOST tag at **load-balancer pick time**, so a
picked-but-unreachable endpoint still contributes its metadata to the span;
envoy-go carries the ZERO `Endpoint` on every upstream-**ACQUIRE** failure
sub-path — circuit-breaker 503, pool-overflow 503, connect-failure (H1 503 /
H2 502) and the defensive H2 grant-race 503, because `doH1ClusterAction` /
`doH2ClusterAction` initialise `picked := cluster.Endpoint{}` and assign only
AFTER the acquire succeeds — so a HOST tag falls to `default_value`/omit
**there** (SPEC §B2, a NAMED departure, deliberately scoped out to its own
future row). Do not read that as "on any failure": **post-acquire failures are
PARITY, not departure** — the H1 upstream-write and read-response 502s
(`router.go:605` / `:618`) and the H2 `RoundTrip` 502 (`router_h2.go:181`)
return the REAL `picked` endpoint, so a HOST tag resolves normally from that
endpoint's own metadata, exactly as the reference does. This fixture drives
only **successful** upstream requests (200 on every probe), so neither arm is
exercised here; the zero-`Endpoint` arm is pinned by unit tests (the
zero-`picked` test landed at T4).

Separately: the cluster carries exactly **one** `lb_endpoint`, so this
fixture cannot discriminate "the **selected** endpoint's metadata" from "the
**only** endpoint's" / "the **first** endpoint's" / cluster-wide metadata —
per-endpoint **selection** is not exercised. A second `lb_endpoint` would
introduce load-balancer pick spread
(`reference_round_robin_offset_randomized`) and make the resolved value
non-deterministic across requests, so a two-endpoint variant is deliberately
not attempted.

## `upstream_cluster` / attribute value-type parity / SDK-scope

Same as `0115`: `upstream_cluster` / `upstream_cluster.name` values are a
documented framework gap (KEY present, VALUE UNasserted); `http.status_code` /
`request_size` / `response_size` use normalized (int-or-string) comparison;
`telemetry.sdk.*` + `ScopeSpans.scope.*` are impl-specific and UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- This fixture is the behavioral proof for phase 72 (ADR-0294 — HOST `metadata`
  `custom_tags` parse + per-request resolve out of the selected upstream
  endpoint's static `lb_endpoints[].metadata` + span-emit upsert), the THIRD
  `metadata` `MetadataKind` after REQUEST (phase 70, `0114`, ADR-0292) and ROUTE
  (phase 71, `0115`, ADR-0293), and the SIXTH `custom_tags` source/kind
  combination overall after phase 59 literal (`0102`, ADR-0277), phase 62
  request_header (`0105`, ADR-0283), phase 63 environment (`0106`, ADR-0284),
  phase 70 REQUEST metadata (`0114`, ADR-0292) and phase 71 ROUTE metadata
  (`0115`, ADR-0293). The tracing family traces back to phase 46/46.1a/46.1b
  (ADR-0260, `0087-tracing-otlp`).
- Do NOT mutate `0064`, `0087`, `0088`, `0102`, `0105`, `0106`, `0114`, `0115`
  or `0027` — this fixture is a full clone in its own directory, its own
  package, its own runner branch.
- **No `scripts/` directory, no Lua** — the resolved value is cross-side EXACT
  by static config alone.
