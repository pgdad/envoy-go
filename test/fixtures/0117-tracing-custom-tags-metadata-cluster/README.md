# 0117-tracing-custom-tags-metadata-cluster

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 73 CLUSTER `metadata` `custom_tags`** (ADR-0295). Cloned from
`0116-tracing-custom-tags-metadata-host` (the OTLP tracing custom_tags HOST
metadata chassis; itself cloned from
`0115`/`0114`/`0106`/`0105`/`0102`/`0087-tracing-otlp`) with the static metadata
block **relocated from the cluster's `lb_endpoints[0]` onto the cluster itself**
— and with the single backend cluster **split in two**. This fixture exercises
the **fourth and last** `metadata` `MetadataKind` — a `cluster`-kind tag
resolves a value out of the **owning cluster's static config metadata**
(`clusters[].metadata.filter_metadata`) and emits it as an OTLP span attribute
on **every** exported span, cross-side.

## ⚠️ Two clusters — what this fixture proves that `0116` could not

`0116` carried a single cluster with a single `lb_endpoint`, so it had to
confess a limitation: it could not discriminate *"the **selected** source's
metadata"* from *"the **only** source's"* / *"the **first** source's"*. This
fixture is built to settle that **positively**, and confesses nothing:

`c_backend_a` and `c_backend_b` point at the **same single** `HTTPFixedBody`
backend `host:port` and are byte-identical **except** for `name:`,
`load_assignment.cluster_name:` and **one metadata value**. Two path-prefix
routes select between them:

```yaml
routes:
  - match: { prefix: "/a" }
    route: { cluster: c_backend_a }   # metadata … cl_k: v-cluster-a-0117
  - match: { prefix: "/b" }
    route: { cluster: c_backend_b }   # metadata … cl_k: v-cluster-b-0117
```

**One** `cl_hit` tag, on **one** HCM, therefore emits **two different values in
the same run** — `"v-cluster-a-0117"` on every `/a` span and
`"v-cluster-b-0117"` on every `/b` span, **on both sides**. That is a positive,
executed proof that the CLUSTER-kind resolve reads the **selected** cluster's
metadata.

Because the selection is by **path** and each cluster carries exactly one
`lb_endpoint`, the pick is deterministic and **no load-balancer spread enters
the assertion** (`reference_round_robin_offset_randomized` never engages), while
`BackendCount()` stays **1** (`reference_differential_backendcount_min_one`).

## Cluster-metadata YAML placement (verified against the parser)

`metadata` is a **sibling of `name:` / `type:`** on the `Cluster` message — it
is **not** on `lb_endpoints[]` (that is `0116`'s HOST source):

```yaml
clusters:
  - name: c_backend_a
    type: STATIC                    # STRICT_DNS on the reference side
    lb_policy: ROUND_ROBIN
    metadata:
      filter_metadata:
        envoy.test:
          cl_k: v-cluster-a-0117
    load_assignment:
      cluster_name: c_backend_a
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address: { address: 127.0.0.1, port_value: 12345 }
```

The owning cluster's **raw per-namespace** static metadata is retained on
`cluster.Endpoint.clusterFilterMetadata` by the `buildCluster` populate loop in
`internal/cluster/manager.go` — placed **before all LB construction**, because
three of the four LB shapes copy each `Endpoint` **by value** — and read back
through `(cluster.Endpoint).ClusterMetaLookup(ns)`
(`internal/cluster/cluster.go`), threaded as the **6th** `ResolveCustomTags`
argument from the **selected** endpoint at the three span-emit sites
(`internal/filter/hcm/accesslog_emit.go:57/:118/:179`).

The gate is therefore the **picked host**, not the matched route
(`reference_cluster_tag_gated_on_pick_not_route`) — exactly as the reference
gates it. Every probe here drives a **successful** upstream request, so a host
is always picked.

Note that **neither route carries a `metadata` block** (that is `0115`'s ROUTE
source) and **neither cluster's `lb_endpoints[]` carries one** (that is `0116`'s
HOST source). Both are deliberately absent so a leftover route/endpoint source
cannot mask the CLUSTER source — breaks **P** and **R** discriminate exactly
that.

## ⚠️ Per-path span partitioning — mechanism and evidence

The assertion is *"each span carries **its own** cluster's value"*, so every
span must be attributable to the path that produced it.

**Not `upstream_cluster`.** `0116` asserts it `assertAttrPresent` **only**,
because envoy-go emits it **empty**
(`reference_tracing_upstream_cluster_framework_gap`). Re-confirmed live by this
fixture's own dump: the reference emits
`upstream_cluster="c_backend_a"`/`"c_backend_b"`, envoy-go emits `""` on every
span. It carries no cross-side information at all.

**Used: the `http.url` span attribute, by suffix match.** Confirmed live on
**both** sides *before* any assertion was written against it
(`FIXTURE_0117_DUMP=1`): every span on both sides renders `http.url` as exactly
`"http://trace.example/a"` or `"http://trace.example/b"`, partitioning **6/6**
with **zero** unclassified spans on each side. The driver still uses a **suffix**
test rather than equality, because `0116` declined to assert `http.url` by value
(scheme/host encoding is not contractually pinned).

The partition **sizes are `Fatalf` preconditions** in `assertClusterTags`: an
unclassifiable span, or a partition that is not 6/6, fails **before** the value
assertions run, so a broken partition can never silently turn them into dead
code (`reference_fatalf_makes_assertions_unreachable` — everything that is an
independent *property* stays `Errorf`).

## Why this needs NO writer (as in `0115`/`0116`)

`0114`'s REQUEST metadata tag reads the **per-request dynamic-metadata bucket**,
which only a runtime writer (there: a Lua `dynamicMetadata():set` filter) can
populate identically on both sides. This fixture's CLUSTER metadata tag instead
reads the owning cluster's **static config metadata** — a field parsed straight
out of the bootstrap YAML at cluster build time, present on **both** sides'
byte-identical `clusters[].metadata.filter_metadata` blocks. No runtime write is
needed for the two sides to agree: the resolved span-attribute value is
cross-side EXACT by construction.

## ⚠️ The namespace is `envoy.test`, not `envoy.lb`

`envoy.lb` is **not** a privileged namespace
(`reference_envoy_lb_namespace_not_privileged`), and the phase-38 `envoy.lb`
scalar projection (`Endpoint.Metadata`, the subset-LB dimension) is a
**different, byte-unchanged** field that could not serve a CLUSTER tag anyway.
This fixture uses `envoy.test` and is the cross-side proof of namespace
generality for the CLUSTER source: the reference resolves it, and the `cl_hit`
VALUE-equality assertions are green on the reference side.

## ⚠️ String values only

A struct-valued or numeric metadata value is **not** cross-side comparable
(`reference_structpb_tag_cross_side_string_only`): the reference serializes
multi-key structs in an **arbitrary** key order while Go always sorts,
scalar-vs-nested numbers use **different** reference renderers, and top-level
scalar numbers render at ~6 significant digits on the reference — biting from
`1000000` → `1e+06` — vs full precision in Go. (Envoy's YAML loader also coerces
any non-integer scalar to a string.) All three asserted values here are plain
strings.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes `/a` → `c_backend_a` and `/b` → `c_backend_b`,
**both** of which reach the same **HTTPFixedBody** backend
(`"backend:v1/fixed\n"` = 17 bytes) and carry **exactly one** `lb_endpoint` each.
The HCM carries a `tracing` block (no Lua filter — only the router):

```yaml
http_filters:
  - name: envoy.filters.http.router
    ...
tracing:
  provider: { name: envoy.tracers.opentelemetry, ... service_name: "0117" }
  random_sampling: { value: 100 }
  custom_tags:
  - tag: cl_hit
    metadata:
      kind: { cluster: {} }
      metadata_key: { key: envoy.test, path: [ { key: cl_k } ] }
      default_value: unused-default-0117
  - tag: cl_default
    metadata:
      kind: { cluster: {} }
      metadata_key: { key: envoy.test, path: [ { key: absent_k } ] }
      default_value: fallback-0117
```

- **`cl_hit`** resolves `metadata_key {key: envoy.test, path: [cl_k]}` → the
  **owning cluster's** static value: `"v-cluster-a-0117"` on a `/a` span,
  `"v-cluster-b-0117"` on a `/b` span. The configured `default_value` is **never
  used** — the span carrying the static value (not the default) is the
  **cluster-metadata-served** proof, and the two values differing across paths
  is the **selected-not-only** proof.
- **`cl_default`** points its path at an UNSET key (`absent_k`, absent on
  **both** clusters) → the `default_value` `"fallback-0117"` is emitted (the
  absent-path default rule).

`c_otlp_collector` is a **real** STRICT_DNS (reference) / STATIC (envoy-go) h2c
cluster pointing at the **driver-owned in-process `otlptrace.Server`** receiver,
bound on `0.0.0.0:<port>` before either proxy starts. It carries **no** metadata
block.

Unlike `0116`, the metadata sits on the CLUSTER rather than on the endpoint, so
DNS resolution is not in the metadata path at all — strictly easier than `0116`.

## Capture mechanism — driver-owned OTLP TraceService receiver

`test/helpers/otlptrace` (`otlptrace.NewAtAddr`) is a minimal in-process
`coltracepb.TraceServiceServer` that accumulates every `*tracepb.Span` across all
`Export` calls. The driver uses `Count()` to poll for convergence and
`Spans()`/`ResourceAttributes()` to snapshot before `Reset()` for per-side
separation.

## Workload — 12 requests per side, split 6 + 6 across the two paths

Per path: **4 PLAIN** requests (`GET /a` or `GET /b`, `Host: trace.example`,
`User-Agent: trace-probe/1`, no inbound trace context; under
`random_sampling: 100%` each is a fresh local sample) + **2 CONTINUATION**
requests additionally carrying `Traceparent: 00-aaaa…aaaa-bbbb…bbbb-01` (the
proxy continues the trace).

The total is **held at 12** — `0116`'s value — deliberately, so the span-count
and `spans_sent` assertions stay consistent
(`reference_fixture_workload_constant_desync`: changing N/K desyncs count
slices and stays masked until `-count=1`).

Each cluster's static metadata is the same on every request to it (both prongs,
both sides — it is config, not per-request state), so every `/a` span carries
the same `cl_hit` value and every `/b` span carries the other. The per-request
byte stream returned by `driveSide` records the **path** per line, so the
cross-side `CompareBytes` itself pins that both sides drove the same per-path
workload in the same order.

## Asserted — the two metadata tags (phase 73)

`assertClusterTags` partitions the spans by path, `Fatalf`s if the partition is
not 6/6-with-zero-unknown, and then asserts on every span of each partition, by
KEY (OTLP attribute order is non-deterministic — **arbitrary per process**, not
merely non-config-order) **and** VALUE:

- `/a` spans: `attrs["cl_hit"]` == `"v-cluster-a-0117"`
- `/b` spans: `attrs["cl_hit"]` == `"v-cluster-b-0117"`
- both:      `attrs["cl_default"]` == `"fallback-0117"`

Each is an independent `t.Errorf` per property (continue past one span's failure
so one bad span does not mask the rest). The `cl_hit` VALUE-equalities are
simultaneously the **cluster-metadata-served** proof (the span carries the static
cluster value, NOT the vacuous configured default `"unused-default-0117"`), the
**selected-not-only** proof, and the **non-`envoy.lb`-namespace** cross-side
proof.

The remaining per-span structure/attribute subset, the continuation-prong
trace-id invariant, the `service.name` Resource attr, and the subject-side
`tracing.opentelemetry.spans_sent` / `spans_dropped` stats are inherited from the
`0116`/`0115`/`0114` chassis (see `expectations.yaml`).

## `random_sampling: 100%` determinism / release barrier / decode-ran proof

Setting `random_sampling: { value: 100 }` makes the span count deterministic at
12 per side. The driver polls `srv.Count() >= 12` at 200ms intervals with a
30s deadline (`reference_concurrency_differential_release_barrier`) — never a
fixed sleep. The `Count()` poll guarantees spans > 0 on BOTH sides before
asserting, and the partition-size `Fatalf` preconditions make an empty-slice
silent pass structurally impossible.

## Out of scope — the pick-time-vs-acquire-success departure

The reference resolves a CLUSTER tag at **load-balancer pick time**, exactly as
it does a HOST tag (`reference_cluster_tag_gated_on_pick_not_route`): a
picked-but-unreachable endpoint still contributes its **owning cluster's**
metadata to the span, while envoy-go carries the ZERO `Endpoint` on every
upstream-**ACQUIRE** failure sub-path — circuit-breaker 503, pool-overflow 503,
connect-failure (H1 503 / H2 502) and the defensive H2 grant-race 503, because
`doH1ClusterAction` / `doH2ClusterAction` initialise `picked := cluster.Endpoint{}`
and assign only AFTER the acquire succeeds — so a CLUSTER tag falls to
`default_value`/omit **there** (a NAMED departure, deliberately scoped out to
its own future row). Do not read that as "on any failure": **post-acquire
failures are PARITY, not departure** — the H1 upstream-write and read-response
502s and the H2 `RoundTrip` 502 return the REAL `picked` endpoint, so a CLUSTER
tag resolves normally from that endpoint's owning cluster, exactly as the
reference does. This fixture drives only **successful** upstream requests (200
on every probe), so neither arm is exercised here; the zero-`Endpoint` arm is
pinned by unit tests (the zero-`picked` test landed at T4).

## `upstream_cluster` / attribute value-type parity / SDK-scope

Same as `0115`/`0116`: `upstream_cluster` / `upstream_cluster.name` values are a
documented framework gap (KEY present, VALUE UNasserted — and, as above, that is
exactly why they cannot serve as the per-path partition key);
`http.status_code` / `request_size` / `response_size` use normalized
(int-or-string) comparison; `telemetry.sdk.*` + `ScopeSpans.scope.*` are
impl-specific and UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- Run with the **full** selector —
  `-run 'TestDifferential/0117-tracing-custom-tags-metadata-cluster'`. A bare
  numeric selector matches ZERO subtests and prints a false green
  (`reference_differential_run_selector`), and every run needs `-count=1`
  (`reference_differential_break_protocol_count1`).
- This fixture is the behavioral proof for phase 73 (ADR-0295 — CLUSTER
  `metadata` `custom_tags` parse + per-request resolve out of the selected
  upstream endpoint's **owning cluster's** static `clusters[].metadata` +
  span-emit upsert), the **fourth and last** `metadata` `MetadataKind` after
  REQUEST (phase 70, `0114`, ADR-0292), ROUTE (phase 71, `0115`, ADR-0293) and
  HOST (phase 72, `0116`, ADR-0294), and the SEVENTH `custom_tags` source/kind
  combination overall after phase 59 literal (`0102`, ADR-0277), phase 62
  request_header (`0105`, ADR-0283), phase 63 environment (`0106`, ADR-0284),
  phase 70 REQUEST metadata (`0114`), phase 71 ROUTE metadata (`0115`) and phase
  72 HOST metadata (`0116`). The tracing family traces back to phase
  46/46.1a/46.1b (ADR-0260, `0087-tracing-otlp`).
- Do NOT mutate `0064`, `0087`, `0088`, `0102`, `0105`, `0106`, `0114`, `0115`,
  `0116` or `0027` — this fixture is a full clone in its own directory, its own
  package, its own runner branch.
- **No `scripts/` directory, no Lua** — the resolved value is cross-side EXACT
  by static config alone.
