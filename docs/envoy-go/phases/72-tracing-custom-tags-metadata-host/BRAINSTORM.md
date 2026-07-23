# Phase 72 Brainstorm — tracing `custom_tags` (METADATA tag type, `HOST` MetadataKind) (the EIGHTEENTH Observability-family row; the SIXTH tracing `custom_tags` capability and the THIRD of the four `metadata` MetadataKinds; lifts the `tracing: custom_tags metadata tag %q host kind unsupported` reject — the message VERBATIM as it appears at `config.go:268`, NOT the compressed paraphrase — and resolves the SELECTED UPSTREAM ENDPOINT's metadata onto the span from the `picked cluster.Endpoint` ALREADY IN SCOPE at the three emit seams; the `CLUSTER` MetadataKind + the unset kind stay PARSE-REJECTED loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules / +0 fuzzers; anticipated ONE new fixture)

**Status: BRAINSTORM. Docs-only. ZERO production `.go`. Row 72 registered `in-progress` at this commit per the ROADMAP §Schema invariant.**

---

## 1. Mission and scope confirmation (72 — the THIRD `metadata` MetadataKind; a `HOST`-kind slice off the `picked` Endpoint already threaded to the emit seam)

### 1.1 What phase 72 delivers as a self-contained whole (the selected upstream endpoint's metadata on the ingress span)

Phase 70 landed the tracing `custom_tags` `metadata` type for the **`REQUEST`** MetadataKind (the per-request dynamic-metadata `Bucket`); phase 71 landed the **`ROUTE`** MetadataKind (the matched route's static config metadata, over `FilterChain.RouteMetaLookup`). Phase 72 lifts the **`HOST`** MetadataKind reject — RE-DERIVED at tip as `internal/tracing/config.go:267-268`:

```go
case k.GetHost() != nil:
	return nil, fmt.Errorf("tracing: custom_tags metadata tag %q host kind unsupported", tag)
```

— and resolves the **SELECTED UPSTREAM ENDPOINT's metadata** (`lb_endpoints[].metadata.filter_metadata[ns]`, walked by `MetadataKey.path`) onto the ingress SERVER span as a `{tag, value}` attribute, on BOTH the OTLP and Zipkin exporters via the shared `ResolveCustomTags` → `Span.Attrs []KV` seam (phase 46). `CLUSTER` and the unset kind stay PARSE-REJECTED loudly with distinct substrings (envoy-go-strict DEPARTURE, ADR-0080 — the reference BOOTS them).

This is a complete, useful, differential-provable capability: a span tag carrying a label attached to the endpoint the load balancer actually picked (canary/version/rack labels are the canonical reference use). It is the third of the four `metadata` MetadataKinds, and after it only `CLUSTER` remains.

### 1.2 What phase 72 does NOT deliver (forward to §8)

- The `CLUSTER` MetadataKind (stays reject-only — the selected CLUSTER is not reachable at the emit seam, and `cluster.Cluster` does not retain its `metadata`; §2.1 records the STALE-COST CORRECTION on why it is MODERATE, not LARGE). §8.
- `spawn_upstream_span` / OTel `http_service` / OTel `sampler` / OTel `resource_detectors` / tracing `verbose` / force-trace. §8.
- The downstream TLS handshake-outcome `ssl` stat family (ADR-0286 C3 framework surgery + a NEW stat surface). §8.
- `access_log[].filter` and bootstrap `stats_flush_on_admin` — two candidates NEWLY SURFACED by this session's sweep and recorded as deferred (§2.1, §8). `access_log[].filter` is flagged as a strong FUTURE pick.

### 1.3 Phase-done as the EIGHTEENTH Observability-family row landing (family STAYS OPEN)

Row 72 is the eighteenth Observability-family row *(the CHAIN ordinal — row 69 carries none, so the chain runs one short of the mechanical count of 19; kept as-is deliberately, §2.1)* and the sixth tracing `custom_tags` capability (literal @ 59, request_header @ 62, environment @ 63, metadata/`REQUEST` @ 70, metadata/`ROUTE` @ 71, metadata/`HOST` @ 72). After it lands, the Observability family STAYS OPEN — the `CLUSTER` MetadataKind, `spawn_upstream_span`, `http_service`, force-trace, and the `ssl` stat family remain deferred candidates.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered, SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~9–11 tasks), the same shape as phases 70 and 71 (each landed as a single flat row of 9 tasks). Phase 72 trades one cost for another versus its two predecessors: it **DROPS the 18-caller threading task entirely** (§1.7 / §2.3 — the decisive finding) but **ADDS one `internal/cluster` widening task** (§2.5). Net: comparable, plausibly +1–2 tasks for the cluster-side widening plus its ripple test. The ADR-0045 split valve is armable-but-unconsumed: if the SPEC's D-CTMH-ENDPOINT-METADATA probe finds the `Endpoint` widening disturbs the phase-38 subset projection or the locality/priority grouping, it may split; the anticipated posture is single flat.

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files, ZERO new packages; ONE genuinely new blast-radius package

Every edit lands in EXISTING files: `internal/tracing/config.go` (lift the HOST reject → an accept arm), `internal/tracing/resolve.go` (a `kindMetadataHost` arm + a new `hostMetaLookup` param), `internal/cluster/{cluster.go,manager.go}` (retain the endpoint's RAW metadata alongside the phase-38 `envoy.lb` scalar projection), `internal/filter/hcm/accesslog_emit.go` (build the `hostMetaLookup` closure LOCALLY from the in-scope `picked`), plus `test/fixtures/` + `docs/`. NO new package.

**`internal/cluster` is phase 72's ONE genuinely new blast-radius surface.** Mechanically re-derived at tip: `git log --name-only e2912f6f~1..HEAD -- internal/` lists exactly 13 files across phases 70+71 — `internal/tracing/{config,resolve}{,_test}.go`, `internal/filter/http/chain{,_test}.go`, and `internal/filter/hcm/{accesslog_emit,connection,h2dispatch,h3dispatch,accesslog_emit_test,span_emit_test,fuzz_test}.go`. **`internal/cluster` appears ZERO times.** Neither predecessor touched it. The envelope-audit implication is FORWARDED to the PLAN: the phase-70/71 audits could assert `internal/cluster` BYTE-UNTOUCHED; phase 72's cannot, and must instead pin the *shape* of the change (a single added field + a single added populate line, with the `ScalarsFromStruct` subset projection BYTE-UNCHANGED).

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch applies (`reference_phase_11_local_ratelimit_prebrainstorm` is `local_ratelimit`-only).

### 1.7 Phase 72's relationship to the existing seams — a reject-lift + a resolve source that is ALREADY AT THE SEAM (the decisive asymmetry vs 70/71)

Phases 70 and 71 each had to *transport* a new source to the emit seam, and each paid the same tax: a new lookup parameter threaded through the three `emitAccessLog*` helpers **and all 18 emit callers** in `connection.go`/`h2dispatch.go`/`h3dispatch.go`. Phase 72 does NOT pay that tax. The source is already a parameter of all three emit functions. RE-DERIVED at tip:

```
internal/filter/hcm/accesslog_emit.go:27   func (f *Filter) emitAccessLog(  r *http.Request,   ..., picked cluster.Endpoint, ... )
internal/filter/hcm/accesslog_emit.go:87   func (f *Filter) emitAccessLogH2(req h2.H2Request,  ..., picked cluster.Endpoint, ... )
internal/filter/hcm/accesslog_emit.go:149  func (f *Filter) emitAccessLogH3(r *http.Request,   ..., picked cluster.Endpoint, ... )
```

and the three `ResolveCustomTags` call sites sit INSIDE those same three functions:

```
internal/filter/hcm/accesslog_emit.go:57   tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r),   metaLookup, routeMetaLookup)
internal/filter/hcm/accesslog_emit.go:118  tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH2(req), metaLookup, routeMetaLookup)
internal/filter/hcm/accesslog_emit.go:179  tracing.ResolveCustomTags(f.tracingConfig.CustomTags, reqHeaderLookupH1(r),   metaLookup, routeMetaLookup)
```

`picked` is already CONSUMED in each of the three (`upstreamHostString(picked)` at `:72` / `:133` / `:194`). So the `hostMetaLookup` closure is constructed LOCALLY from the in-scope `picked` value at exactly three places. **The 18 callers in `connection.go` (5) / `h2dispatch.go` (6) / `h3dispatch.go` (7) — count re-derived mechanically at tip, `grep -cE "emitAccessLog(H2|H3)?\(" ⇒ 5+6+7 = 18` — are BYTE-UNTOUCHED by phase 72.** This is the single biggest reason `HOST` beats `CLUSTER`, and it is why the row stays flat despite adding a second package.

**⚠️ CORRECTION FOLDED IN (adversarial verifier V1, MODERATE) — the zero-`picked` exposure is a MAJORITY, not an edge.** An earlier draft of this section framed the no-endpoint case as an artifact of "the 3 nil-lookup sites, two of which pass the zero value". That materially UNDERSTATES it. RE-DERIVED MECHANICALLY at tip, per-site, with the `picked` argument and the two metadata-lookup arguments read off each call:

| file | site | 4th arg (`picked`) | metadata lookups |
|---|---|---|---|
| `connection.go` | `:330` | `cluster.Endpoint{}` | `nil, nil` |
| `connection.go` | `:464` | `cluster.Endpoint{}` | real |
| `connection.go` | `:597` | `cluster.Endpoint{}` | real |
| `connection.go` | `:699` | `cluster.Endpoint{}` | real |
| `connection.go` | `:777` | **`picked`** | real |
| `h2dispatch.go` | `:313` | **`picked`** | `nil, nil` |
| `h2dispatch.go` | `:396` | `cluster.Endpoint{}` | real |
| `h2dispatch.go` | `:530` | `cluster.Endpoint{}` | real |
| `h2dispatch.go` | `:577` | **`picked`** | real |
| `h2dispatch.go` | `:584` | **`picked`** | real |
| `h2dispatch.go` | `:613` | **`picked`** | real |
| `h3dispatch.go` | `:130` | `cluster.Endpoint{}` | `nil, nil` |
| `h3dispatch.go` | `:210` | `cluster.Endpoint{}` | real |
| `h3dispatch.go` | `:280` | `cluster.Endpoint{}` | real |
| `h3dispatch.go` | `:341` | `cluster.Endpoint{}` | real |
| `h3dispatch.go` | `:367` | **`picked`** | real |
| `h3dispatch.go` | `:373` | **`picked`** | real |
| `h3dispatch.go` | `:395` | **`picked`** | real |

**10 of the 18 emit call sites pass a zero-value `cluster.Endpoint{}`; only 8 carry a real `picked`** (H1 4-zero/1-real · H2 2-zero/4-real · H3 4-zero/3-real). Note the zero-`picked` set is NOT the nil-lookup set: the two dimensions cross (`h2dispatch.go:313` passes a REAL `picked` with `nil, nil` lookups; `connection.go:464`/`:597`/`:699` pass the ZERO endpoint with REAL lookups).

**The consequence, stated plainly for the SPEC author:** a HOST-kind custom tag silently falls to `default_value`/omit on **every local-reply, 500, and no-route path on all three protocols** — for H1 only **1 of 5** sites carries a real endpoint. That is not an exotic edge; it is the majority arm by site count and is trivially reachable in production.

**This does NOT change the row's size.** The closure still needs no threading (the source is a parameter at all 18 sites either way), and a zero `Endpoint`'s namespace lookup is nil-safe — a nil raw-metadata field yields `(nil, false)`, which routes into exactly the same default/omit rule as an unresolvable path. The zero-value case is still handled ONCE, inside the closure. **And the 18 callers remain BYTE-UNTOUCHED by this row** — the classification above is read-only evidence about what those untouched sites already pass, not a work item. What changes is the SPEC's *sizing of the no-host behavioral arm*: it must be pinned as a first-class behavior (D-CTMH-NOHOST, strengthened in §7), not as a 2-site edge.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` (`metadata`/`HOST`) *(SELF-PICKED per the standing directive → phase 72 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest defensible ROW-SIZED candidate** after INVESTIGATING each candidate's size against source THIS session (two independent read-only cost-assessment agents at tip `4238a4d3`; §11). Row 72 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `custom_tags` (`metadata`/`HOST`) is smallest-defensible:** it sits on landed substrate on FOUR axes — (a) the phase-70/71 `metadata`-kind resolve machinery (`descend` at `resolve.go:128` + `structpbValueToString` at `:151`), reused **VERBATIM**, with `kindMetadataRoute`'s full-`MetaPath` descent (`resolve.go:95-118`) as a byte-level clone template that landed LAST SESSION; (b) the config parse arm — the `k.GetRoute() != nil` accept at `config.go:248-264` is a drop-in clone (namespace/path/segment PGV-parity rejects + a `CustomTagSpec{...}` build) with `kindMetadataRoute` swapped for a new `kindMetadataHost`; (c) **the source is ALREADY AT THE SEAM** — `picked cluster.Endpoint` is the 4th parameter of all three emit functions, so the 18-caller threading tax that phases 70 and 71 each paid is SKIPPED (§1.7, the load-bearing finding); (d) the fixture chassis is the just-landed `0115-tracing-custom-tags-metadata-route` with the metadata block relocated from the route to `lb_endpoints[]` — static on both sides, NO writer. It carries NO ADR headwind and is **+0 stat** (a span attribute, not a stat).

**The ONE genuine gap (and it is bounded):** `cluster.Endpoint` retains only a LOSSY projection of the endpoint's metadata — see §2.5 and D-CTMH-ENDPOINT-METADATA. That is the whole delta versus a "pure clone" row, and it is an added struct field plus a one-line populate.

**⚠️ STALE-COST CORRECTION #1 (this session, §11) — `HOST` was under-costed as MODERATE, and the reason given was wrong-shaped.** The phase-70 and phase-71 BRAINSTORMs recorded `HOST` as MODERATE because "only the reduced `envoy.lb` scalar subset is in scope at the seam". The *fact* holds exactly (re-derived — `cluster.go:46-49`; `manager.go:883`), and so does the *in-scope* observation: phase 71's BRAINSTORM `:55` says in as many words that *"the picked `cluster.Endpoint.Metadata` IS in scope at the seam"*. What was never derived is the **COST CONSEQUENCE** of that in-scope-ness: because the endpoint is already a parameter of all three emit functions, a `HOST` row skips the 18-caller thread entirely — the single largest task in both predecessor rows. Phase 71 saw the fact and did not price it. Re-derived at tip, `HOST` is SMALL, not MODERATE (`reference_deferred_candidate_cost_restale`: do not trust a deferral's own adjective — and re-price the fact, not just re-read it).

**⚠️ STALE-COST CORRECTION #2 (this session, §11) — the `CLUSTER` "LARGE because of the `upstream_cluster` framework gap" adjective is a CATEGORY ERROR, not merely stale.** The gap itself is REAL and re-verified: `accesslog_emit.go:51` / `:112` / `:173` each hard-code `UpstreamCluster: "", // not available at this seam`. But that is the gap for the **`upstream_cluster` span TAG / access-log field** — a DIFFERENT feature. A `CLUSTER`-kind metadata custom tag does not need that tag emitted at all; it needs the selected cluster's `metadata`, which is a different plumbing problem. (**No claim is made that it is a *smaller* one** — an earlier draft said "smaller" and that is unsupported: both are blocked by the SAME missing seam, the selected cluster's identity at the emit seam [gap (b) below], and resolving cluster *metadata* is if anything a SUPERSET of resolving the cluster *name*. The correction here is the CATEGORY, not the magnitude.) `CLUSTER` is **MODERATE**, not LARGE. It is rejected on size (still bigger than `HOST`), NOT on impossibility. Recorded so a future roller does not re-cite the wrong reason a third time.

**Rejected alternatives (recorded per the standing directive; each RE-DERIVED/SIZED against source this session — §11):**

- **`CLUSTER` MetadataKind (the nearest rival)** — MODERATE, ~11–13 tasks. THREE gaps, all bounded but additive:
  - (a) **`cluster.Cluster` does not retain its `metadata`.** Re-derived: `internal/cluster/cluster.go:132-…` `type Cluster struct` carries `name`/`endpoints`/`connectTimeout`/`lb`/`upstreamCfg`/the H1+H2 pools/`useH2`/`h2MaxConcurrentStreams`/… and **no metadata field at all**. A `CLUSTER` row must add one plus an accessor and populate it in `buildCluster`.
  - (b) **The SELECTED CLUSTER is not reachable at the emit seam.** `internal/filter/http/router/router.go:255` publishes exactly `func (f *Filter) Picked() cluster.Endpoint` — the ENDPOINT, not the cluster. A `CLUSTER` row must add a `routeAction` interface method (`internal/filter/hcm/route.go:52-59` — today just `asRouterAction`/`asRouterActionH2`) plus a `chain.SetClusterMetadata` / `ClusterMetaLookup` pair mirroring the phase-71 `SetRouteMetadata`/`RouteMetaLookup` shape — **which DOES re-pay the 18-caller thread**, the exact tax `HOST` skips.
  - (c) **A real semantic wrinkle:** `weightedClusterRouteAction` (`internal/filter/hcm/actions.go:234-245`) is *"the per-request weighted-random cluster-SELECTION route action (ADR-0241)"* — the producer pre-builds the H1/H2 closures carrying the N entries plus a SHARED selector, so selection happens per-request INSIDE the closure. A statically-resolved cluster metadata therefore yields nothing for weighted routes, needing a documented departure or a cross-cutting reject. Deferred — the LAST remaining `metadata` MetadataKind, and the natural phase-73 candidate once `HOST` lands.
- **`spawn_upstream_span`** — LARGE. envoy-go emits ONE SERVER span; `BuildServerSpan` is the only builder in `internal/tracing/span.go`. A second CLIENT span needs a client-span model, an upstream-leg start/end timing seam that does not exist, parent/child linkage, and a two-span export path. Deferred.
- **OTel `http_service`** — MODERATE. Re-derived at tip: the reject is live at `internal/tracing/config.go` inside `parseOTel` (`if otel.GetHttpService() != nil { return nil, fmt.Errorf("tracing: http_service unsupported") }`). A whole new protobuf-over-HTTP transport beside the landed `envoy_grpc` one — clean behind the `TracesClient` seam, but a new transport + parse + wiring. Deferred.
- **OTel `sampler`** — MODERATE. Same `parseOTel` block (`otel.GetSampler() != nil → "sampler unsupported"`). It is a `TypedExtensionConfig` extension point: needs a registry plus at least one concrete sampler implementation, and it intersects the landed `DecideWithContext` sampling ladder. Deferred.
- **OTel `resource_detectors`** — MODERATE. Same block (`len(otel.GetResourceDetectors()) > 0 → "resource_detectors unsupported"`). Also a `TypedExtensionConfig` extension point (registry + ≥1 impl), plus a resource-attribute merge into the OTLP `Resource` message. Deferred.
- **tracing `verbose`** — MODERATE. Needs per-stream span *logs/events* — a surface `Span` has no field for. Adding one ripples both exporters plus the receiver-side assertions. Deferred.
- **force-trace / internal-request detection (`x-envoy-force-trace`)** — LARGEST. ZERO `x-envoy-internal` / `x-envoy-force-trace` handling anywhere; Envoy honors force-trace only for INTERNAL requests, which needs internal-vs-external classification (trusted-hop counting / `internal_address_config`) plus edge header sanitization — an unbuilt subsystem. HIGH scope-risk. Deferred.
- **the downstream TLS handshake-outcome `ssl` stat family** — LARGE, framework surgery **CONFIRMED** (ADR-0286 C3's adjective HOLDS, unlike the two corrections above). Zero `ssl.*` stats exist. `fail_verify_error` vs `fail_verify_no_cert` requires classifying opaque Go `crypto/tls` / `crypto/x509` error values through a handshake-error callback wired into every per-chain `*stdtls.Config`; `ssl.ciphers.*` / `ssl.versions.*` are DYNAMIC stat names needing an IANA→OpenSSL cipher-name translation table for cross-side parity (and would have to pass `stats.IsValidName`, `reference_dynamic_stat_name_charset_guard`). Plus it breaks the long +0-stat streak. Deferred.
- **A SIXTH stats sink** — LARGE / effectively **VACUOUS**. Re-derived: all five non-`wasm` `stats_sinks[]` extensions in the pinned proto are LANDED (statsd / dog_statsd / graphite_statsd / metrics_service / open_telemetry — `internal/bootstrap/bootstrap.go:252`/`:263`/`:396` and the `internal/statssink` roster). The ONLY unconsumed `stats_sinks[]` extension is `wasm`. **There is no cheap sixth sink.** Deferred as not-available rather than merely large.
- **xDS reconnection-backoff / `initial_fetch_timeout` edges** — the two cost agents DISAGREED here, and the disagreement resolves AGAINST the candidate. Re-derived at tip: (i) `initial_fetch_timeout` is **already landed** — `internal/xds/config.go:11-13` defines `defaultInitialFetchTimeout = 15 * time.Second` and `:74-75` honors `cs.GetInitialFetchTimeout()`; (ii) `internal/xds/stream.go` contains **no loop at all** — `grep -n "for {"` returns NOTHING; the file has exactly two fetchers (`fetchSecret` at `:50`, `fetchValidationSecret` at `:123`), each doing one `Send` → one `Recv()` (`:62` / `:134`) → ACK/NACK → return. So **there is no persistent watch stream to reconnect TO**; a "reconnection" row must FIRST build the long-lived stream plus a rotation applier. The ROADMAP's own *"SMALL but THIN, initial-fetch-only ⇒ little to reconnect to"* adjective HOLDS. REJECTED as larger than it looks.
- **QUIC `QuicProtocolOptions` tuning** — SMALL, ~9–11 tasks, and it LOSES to `HOST` on execution risk rather than size. Re-derived: both `quic.Config` construction sites in `internal/listener/quic.go` (`:37` `quic.Listen(udpConn, tlsCfg, &quic.Config{})` and `:145` `QUICConfig: &quic.Config{}`) are EMPTY literals, so every tuning knob is silently ignored — a real divergence. But it needs ~2 tasks of NEW differential harness: `test/helpers/h3.go:28-31` documents *"A fresh Transport is constructed per call (no connection caching) so consecutive invocations never reuse a QUIC connection"* (`func H3RoundTrip` itself opens at `:32`), so a multi-stream / connection-reusing helper must be written before `max_concurrent_streams`-style knobs are observable at all — and the natural observables are timing-flaky (`reference_differential_http_expectations_tcp_only`, `reference_differential_band_sigma_margin`). Rejected on execution risk + new-harness cost.
- **upstream SDS** — LARGE, with a **boot-ORDER blocker**: clusters (including upstream TLS) are built before the dialer and before the SDS provider exists, so it needs two-pass cluster construction on top of threading a `SecretProvider` across the `grpcclient → cluster → tls` seam the `xdsgrpc` package exists to dodge (`reference_xds_config_seam_transitive_cycle_guard`). Deferred.
- **CDS/EDS, LDS/RDS, ADS, Delta xDS, `google_grpc`, upstream H3 cluster, h3spec conformance gate, QUIC robustness (migration/retry), 0-RTT/early-data, alt-svc advertisement** — all LARGE or multi-row; each needs new substrate (a long-lived watch stream + appliers, a second gRPC client stack, or a new upstream transport). Deferred wholesale.
- **Opening a NEVER-OPENED family (gRPC / Runtime / WASM)** — DEFENSIBLE in principle but larger than a follow-on within an already-open family, and the sentinel's own labels need reading with care:
  - **`WASM` is a BOOKKEEPING ARTIFACT, not unbuilt work.** Re-derived: `internal/wasm/` is a full wazero-based host — `find internal/wasm -name '*.go' | xargs wc -l` ⇒ **21,550 lines** across `abi/`, `compile.go`, `bytecode_util.go`, `dynamic_stats.go`, `env_vars.go`, … — and the `envoy.filters.http.wasm` filter landed as HTTP-filters-family rows **25 / 25.1 / 25.2 / 25.3, all `done`** (ROADMAP line 73: *"§9 HTTP-filters family CLOSED … phase 25 was the EIGHTEENTH and FINAL §9 HTTP-filters row"*). The `### WASM host family` heading therefore describes work that is substantially BUILT while the sentinel prints `NEVER OPENED: WASM`. **Flagged as a ROADMAP bookkeeping defect worth a future correction — deliberately NOT acted on at this row** (phase 72 is docs-only and must not alter that heading's semantics; the sentinel's check-(3) output is depended on by the stage-close mechanics of §12).
  - **`Runtime` IS genuinely unbuilt** — `ls internal/runtime/` returns exactly `doc.go`. A real never-opened family, and a real future option.
  - **`gRPC` is gated on a FRAMEWORK row.** Re-derived: `FilterChain.RunDecodeTrailers` (`internal/filter/http/chain.go:455`) and `RunEncodeTrailers` (`:622`) exist but have **ZERO production callers** — `connection.go:565` and `h2dispatch.go:501-503` both carry live comments saying the dispatch path *"does not yet expose"* / *"does not yet invoke"* them, and the only driver of `RunDecodeTrailers` is the `0007b-iteration-probe` fixture driver. gRPC status/trailer propagation is therefore blocked behind an HCM response-trailer framework row. Deferred.
- **UNLISTED candidates surfaced by this session's sweep (recorded as NEWLY-NAMED deferred candidates for a FUTURE row — NOT picked here):**
  - **`access_log[].filter`** — field 2 of `envoy.config.accesslog.v3.AccessLog`, a 13-arm oneof (`status_code_filter` / `duration_filter` / `not_health_check_filter` / `traceable_filter` / `runtime_filter` / `and_filter` / `or_filter` / `header_filter` / `response_flag_filter` / `grpc_status_filter` / `extension_filter` / `metadata_filter` / `log_type_filter`). Re-derived at tip: `parseOneAccessLog` (`internal/bootstrap/bootstrap.go:937`) consumes only the `typed_config`, and a repo-wide `grep -rn "GetFilter()" --include=*.go .` (excluding tests) returns **ZERO hits**. The field is a KNOWN proto field, so the whole-doc `DiscardUnknown:false` strict pass ACCEPTS it and it is then **silently ignored** — i.e. a configured `access_log[].filter` changes reference behavior and does NOT change envoy-go's. **This is a LIVE BEHAVIORAL DIVERGENCE, not merely an unimplemented knob** (contrast `reference_pinned_dep_missing_field_free_reject`, which is about fields ABSENT from the pinned proto boot-rejecting for free — this one is present and swallowed). A single-arm slice (e.g. `status_code_filter` only, siblings reject-loud per ADR-0080) is exactly the incremental-arm shape phases 59/62/63/70/71 used. **Flagged in §8 as a strong FUTURE pick.**
  - **`stats_flush_on_admin`** — re-derived at `internal/bootstrap/bootstrap.go:606-607`: `if bs.GetStatsFlushOnAdmin() { return fmt.Errorf("bootstrap: stats_flush_on_admin is not supported (envoy-go ships only the periodic stats_flush_interval sink loop)") }`, documented as a deliberate ADR-0080 strict-reject at `:593`. Lifting it means an admin-triggered flush path beside the landed periodic `Flusher` loop. Recorded as a named deferred candidate; smaller than most of the above but a new trigger seam, and it must not disturb the five landed sinks.
- **⚠️ ROADMAP BOOKKEEPING — TWO inherited artifacts, NEITHER acted on at this row.** Both are recorded here as FUTURE-correction candidates so a later roller finds them already diagnosed, and both are deliberately left alone because phase 72 is docs-only and must not perturb the stage-close mechanics of §12.
  - **`NEVER OPENED: WASM` is a bookkeeping artifact, not unbuilt work** — see the never-opened-family bullet above (21,550 LOC of wazero host; rows 25/25.1/25.2/25.3 all `done`). Sentinel check (3) still prints it; left AS-IS.
  - **The Observability-family ORDINAL is ONE LOW, inherited (verifier V2).** A mechanical count of ROADMAP §Schema rows that self-declare as Observability-family rows gives **19**: 44, 45, 46, 47, 48, 49, 50, 55, 56, 57, 58, 59, 62, 63, 64, 69, 70, 71, 72. But **row 69 (`stats-sink-otlp`) carries NO ordinal word at all**, so phase 70 restarted the chain one short: row 70 claims **SIXTEENTH** (truly 17th), row 71 claims **SEVENTEENTH** (truly 18th), and row 72 claims **EIGHTEENTH** (truly **19th**). **Phase 72 KEEPS `EIGHTEENTH`, deliberately** — renumbering 72 alone would make it inconsistent with two LANDED `done` rows (70, 71) that carry the same off-by-one, and a docs-only BRAINSTORM is the wrong place to rewrite landed rows. The true mechanical count is recorded here so the eventual correction (re-numbering 70/71/72 together, or back-filling row 69's ordinal) is a single deliberate edit rather than a rediscovery. Every "EIGHTEENTH" in this document is the CHAIN ordinal, not the mechanical one.
- **For the record — HCM `tracing` is otherwise EXHAUSTED.** All eight `HttpConnectionManager.Tracing` fields are handled, and `ZipkinConfig` / `OpenTelemetryConfig` are fully covered apart from the three `parseOTel` reject arms named above. After the four `metadata` MetadataKinds are consumed, **every remaining tracing candidate is one of the LARGE ones** (`spawn_upstream_span`, `http_service`, `sampler`, `resource_detectors`, `verbose`, force-trace). Phase 72 (`HOST`) and a phase-73-shaped `CLUSTER` row are the last two cheap tracing slices in the family.

### 2.2 Scope: the `metadata` type, `HOST` MetadataKind ONLY; `CLUSTER` + the unset kind PARSE-REJECT loudly *(self-answered; the incremental-arm precedent)*

Phase 72 supports `kind == HOST` (the selected upstream endpoint's metadata); `CLUSTER` and the two unset-kind arms continue to REJECT loudly with distinct substrings (§2.4). This mirrors the project's landed incremental-arm posture — the `custom_tags` TYPES landed one at a time (literal/59, request_header/62, environment/63), and the `metadata` MetadataKinds are landing one at a time (`REQUEST`/70, `ROUTE`/71, `HOST`/72). A `HOST`-kind slice is a complete, useful, differential-provable capability. The SPEC probe (D-CTMH-WIRE) confirms the reference emits it identically.

### 2.3 The resolve seam: a NEW `kindMetadataHost` arm in `ResolveCustomTags`; a FIFTH nil-tolerant one-arg param; the closure built LOCALLY at three sites *(self-answered shape; SPEC pins — D-CTMH-RESOLVE-SEAM)*

`ResolveCustomTags` at tip (`internal/tracing/resolve.go:32`, RE-DERIVED verbatim):

```go
func ResolveCustomTags(specs []CustomTagSpec, headerLookup func(string) ([]string, bool), metaLookup func(ns, key string) (*structpb.Value, bool), routeMetaLookup func(ns string) (*structpb.Value, bool)) []KV
```

Phase 72 anticipates a **FIFTH nil-tolerant parameter `hostMetaLookup func(ns string) (*structpb.Value, bool)`**, shaped IDENTICALLY to `routeMetaLookup` (one-arg, namespace → the whole namespace struct wrapped as a `StructValue`). The `kindMetadataHost` arm is then a byte-level clone of the landed `kindMetadataRoute` arm (`resolve.go:95-118`):

- `v, ok := hostMetaLookup(s.MetaNamespace)`
- `if ok { v, ok = descend(v, s.MetaPath) }` — the **FULL** `MetaPath`, NOT `[1:]`. The `[1:]` at `resolve.go:84` lives ONLY in the `kindMetadata`/REQUEST arm (case label `:71`) and is a **Bucket-pre-keying artifact**, not a general rule (`reference_route_metadata_resolve_full_metapath`; the phase-71 SPEC EXECUTION-CONFIRMED the equivalence).
- `structpbValueToString(v)` — `descend` (`resolve.go:128`) and `structpbValueToString` (`resolve.go:151`) **REUSED VERBATIM**. Both are proto-agnostic, so `internal/tracing` stays filter-free and cluster-free (the cycle guard is unaffected).
- `hostMetaLookup == nil` ⇒ host specs use default/omit, keeping all existing behavior byte-identical when no HOST tags are configured.

**The threading difference from 70/71 (§1.7):** the closure is NOT a new parameter on the three `emitAccessLog*` helpers threaded from 18 call sites. It is built LOCALLY inside each of the three helpers from the already-present `picked cluster.Endpoint`, at the three `ResolveCustomTags` call sites (`accesslog_emit.go:57` / `:118` / `:179`). Whether `internal/filter/hcm` builds the closure inline or calls a small accessor method on `cluster.Endpoint` (the tidier option — it also localizes the `structpb` import) is D-CTMH-RESOLVE-SEAM; the anticipated shape is an `Endpoint`-side accessor mirroring phase-71's `FilterChain.RouteMetaLookup` (`chain.go:1033-1039`), which is the seam the project already ratified.

Two behavioral sub-questions the SPEC probes:
- **D-CTMH-VALUE-SERIALIZE** — the reference's `structpb.Value`→string serialization for a HOST-kind tag SHOULD match the phase-70 P3 table (string→raw / `NullValue`→omit / else `protojson.Marshal` + `json.Compact`), since it is the same `MetadataKey` resolution machinery on the reference side. The fixture's positive arm SHOULD use a STRING metadata value (the unambiguous case).
- **D-CTMH-DEFAULT** — the `default_value` / omit semantics when the host metadata path is absent or the resolved value is empty. Anticipated: the SAME `request_header` default rule phases 70/71 landed (present-empty EMITS `""`; absent → `default_value` if non-empty, else omit; `HasDefault = DefaultValue != ""`). Probe confirms.

### 2.4 The remaining unsupported MetadataKinds reject loudly with DISTINCT substrings — the envoy-go-strict DEPARTURE narrows by one *(self-answered; ADR-0080)*

The reference SUPPORTS all four MetadataKinds; envoy-go will support `REQUEST` (@70) + `ROUTE` (@71) + `HOST` (@72) and reject `CLUSTER` plus the unset kind. RE-DERIVED at tip:

| arm | line | disposition at 72 |
|---|---|---|
| `k == nil` → `"... kind required"` | `config.go:229-230` | UNCHANGED (nil `*MetadataKind`; the phase-71 S3 split calls this the DEPARTURE half) |
| `k.GetRequest() != nil` accept | `config.go:231-247` | UNCHANGED (phase 70) |
| `k.GetRoute() != nil` accept | `config.go:248-264` | UNCHANGED (phase 71) — **the clone source for phase 72** |
| `k.GetCluster() != nil` → `"... cluster kind unsupported"` | `config.go:265-266` | UNCHANGED (stays a departure) |
| `k.GetHost() != nil` → `"... host kind unsupported"` | `config.go:267-268` | **REPLACED by an accept arm** |
| `default:` → `"... kind required"` | `config.go:269-270` | UNCHANGED (present-empty oneof; the PARITY half) |

The accept arm clones `config.go:248-264` exactly: `mk := md.GetMetadataKey()`, the empty-namespace / empty-path / empty-path-segment PGV-parity rejects, the `path` extraction loop, `dv := md.GetDefaultValue()`, then `spec = CustomTagSpec{Key: tag, Kind: kindMetadataHost, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}`. Note the receiver fact that cost phase 70 a SEVERE: the four kind-getters live on `*metadatav3.MetadataKind` (the bound `k := md.GetKind()` at `config.go:227`), **NOT** on `*tracingv3.CustomTag_Metadata` — `md.GetHost()` does not compile. D-CTMH-REJECT also confirms (one probe arm) the reference ACCEPTS a `HOST` metadata tag and still BOOTS `CLUSTER`, so that reject stays a real envoy-go-strict DEPARTURE.

### 2.5 Config home + the ONE genuine gap: `cluster.Endpoint` retains a LOSSY metadata projection *(the load-bearing SPEC question — D-CTMH-ENDPOINT-METADATA)*

The parsed HOST spec lives on the EXISTING `CustomTagSpec` (`config.go:62-77`), reusing `MetaNamespace` / `MetaPath` / `DefaultValue` / `HasDefault`; the only config-side addition is a `kindMetadataHost` constant beside `kindMetadataRoute` (`config.go:54-60`, currently 5 constants at `iota` 0–4 ⇒ the new one is `iota == 5`).

The gap is on the cluster side. RE-DERIVED at tip:

```go
// internal/cluster/cluster.go:43-49
type Endpoint struct {
	Host string
	Port uint32
	// Metadata is the parsed envoy.lb scalar key→value namespace (the subset
	// dimension, phase 38). nil when absent. NOT part of the dial identity:
	// Addr() ignores it, so ring_hash/maglev table keys stay "IP:PORT".
	Metadata map[string]SubsetValue
	...
}
```

populated at `internal/cluster/manager.go:883-884`:

```go
scalars, _ := ScalarsFromStruct(lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]) // drop non-scalar keys
e := Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, Locality: loc, LocalityWeight: weight, Priority: priority}
```

So the retained projection is **the `envoy.lb` namespace ONLY, scalars ONLY**. `ScalarsFromStruct` (`internal/cluster/subset.go:263-285`, re-opened) keeps exactly `Value_StringValue` → `subsetString`, `Value_NumberValue` → `subsetNumber`, `Value_BoolValue` → `subsetBool`, and routes **`Value_StructValue` / `Value_ListValue` / `Value_NullValue` / nil** into the `nonScalar []string` return — which `manager.go:883` DISCARDS with `_`. A HOST-kind custom tag must (i) address a **caller-chosen namespace** (not just `envoy.lb`) and (ii) **walk a nested path** (which requires exactly the `StructValue` kind the projection drops). Both requirements are unmeetable from the existing field.

**Anticipated fix (SPEC pins the exact shape):** retain the RAW `*corev3.Metadata` (or, narrower, the per-namespace `*structpb.Struct` map) on `Endpoint` **alongside** the existing subset projection — an added field plus a one-line populate adjacent to `manager.go:883`, leaving the phase-38 `ScalarsFromStruct` projection **BYTE-UNCHANGED**. Widening rather than replacing is deliberate: it keeps the subset LB, the `defaultSubset` path (`manager.go:754`), and the two HCM `ScalarsFromStruct` consumers (`internal/filter/hcm/config.go:977`/`:993`/`:997`) untouched.

**The "no new ripple" claim, VERIFIED rather than asserted.** The claim is that adding another reference-typed field to `Endpoint` costs nothing structurally because `Endpoint` is already non-comparable. Evidence re-derived at tip, three ways:
1. The type's own doc comment states it: `cluster.go:88-92` — *"`Endpoint` is NOT comparable (it carries a `Metadata` map), so the 'a host was picked' guard at the connect-failure seam sites uses `!ep.IsZero()` rather than `ep != Endpoint{}`."* So the codebase has ALREADY paid the non-comparability cost and routed around it.
2. Every identity/keying use goes through the precomputed STRING `Addr()`, not the struct: the health-check state map is *"keyed by `Endpoint.Addr()`"* (`health.go:72`, `:90`, `:96`, `:119`, `:148`, `:380`, `:402-403`), and the same comment at `cluster.go:46-48` (the comment proper; `:49` is the `Metadata map[string]SubsetValue` field line itself — the quoted `:43-49` block above is the struct-head-through-field range and is exact) records that `Addr()` ignores `Metadata` *"so ring_hash/maglev table keys stay 'IP:PORT'"*. A new metadata field is therefore invisible to the dial identity, the hash-ring identity, and the health-state identity.
3. `Endpoint` is constructed by keyed composite literal (36 non-test literal sites repo-wide), so an added field defaults to its zero value at every existing site — no positional breakage.
The residual risks that the SPEC must still probe are BEHAVIORAL, not structural: does widening disturb (a) the phase-38 subset-selection semantics, (b) the ring_hash/maglev dial identity, (c) locality/priority grouping, or (d) memory-per-endpoint at scale? All four are D-CTMH-ENDPOINT-METADATA.

### 2.6 Fixture posture: anticipated ONE new fixture (OTLP); a STATIC `lb_endpoints[].metadata` positive arm + a `default_value` arm — NO writer *(self-answered direction; SPEC confirms D-CTMH-FIXTURE)*

A HOST metadata custom tag is an OBSERVABLE span attribute, so it IS differential-provable. Per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch) the anticipated posture is a NEW `test/fixtures/0116-tracing-custom-tags-metadata-host` dir (OTLP provider, asserting the HOST metadata tag on the OTLP span via the `test/helpers/otlptrace` receiver), modelled on the just-landed `0115-tracing-custom-tags-metadata-route` chassis (`driver/`, `envoy-go.yaml`, `envoy.yaml`, `expectations.yaml`, `README.md`).

**Like 0115 and unlike 0114, NO runtime writer is needed** — the source is a STATIC `lb_endpoints[].metadata.filter_metadata{ns: {...}}` block in the cluster config, read identically by subject and reference. Two HOST `custom_tags`: one resolving to the static value, one whose namespace/path is absent so it falls through to `default_value` — both asserted **cross-side EXACT key+value**, deterministic (`reference_differential_asserter_dispatch`: prove the new assertion is live; `reference_deliberate_break_wrong_assertion`: confirm WHICH assertion fires on each break). Single-endpoint cluster so the pick is deterministic and no LB spread enters the assertion (`reference_round_robin_offset_randomized`).

Whether a SECOND Zipkin dir is warranted is D-CTMH-FIXTURE (anticipated **NO** — the shared `Span.Attrs` seam plus unit tests prove both exporters; the 59/62/63/70/71 precedent, and phase 71's P3 probe already proved the Zipkin emit LIVE for the sibling kind). Anticipated: fixtures **117 → 118**.

### 2.7 Fuzz posture: a SEED ADDITION to the EXISTING `FuzzHCMConfigParse` — NO repoint, NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-CTMH-FUZZSEED)*

The tracing config is parsed via the HCM config path, fuzzed by `FuzzHCMConfigParse`. RE-DERIVED at tip, the `withMetaTags` seed (`internal/filter/hcm/fuzz_test.go:97-115`) already carries three tags:

| seed tag | kind | arm exercised at tip |
|---|---|---|
| `meta_ok` (`:100-104`) | `MetadataKind_Request_` | the REQUEST accept (phase 70) |
| `meta_route_ok` (`:105-109`) | `MetadataKind_Route_` | the ROUTE accept (phase 71) |
| `meta_bad` (`:110-112`) | `MetadataKind_Cluster_` | the CLUSTER reject |

**Phase 72 needs NO repoint.** Phase 71 had to repoint `meta_bad` (Route → Cluster) because the row REMOVED the very arm that seed exercised; phase 72 removes the HOST arm, which no seed points at. So the delta is purely ADDITIVE: add a `meta_host_ok` (`MetadataKind_Host_` with a valid `MetadataKey`) accept seed and leave `meta_bad` on CLUSTER untouched — CLUSTER stays the live reject-arm seed. This is a cheaper fuzz delta than either predecessor, and it is a second reason `HOST` beats `CLUSTER` (a CLUSTER row would have to repoint `meta_bad` yet again, to `Host` or to a nil kind).

Fuzzer count STAYS **55** — re-derived mechanically at tip with the CANONICAL, `internal/`-scoped command from STATE.md `:30`, `grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` ⇒ **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented total against actual `^func Fuzz` before AND after). SPEC confirms D-CTMH-FUZZSEED and dispatch-verifies the HCM fuzzer reaches the `kindMetadataHost` accept arm.

### 2.8 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. Anticipated stat surface **1201 (+0)**, UNCHANGED. (Point of contrast with the `ssl`-stat-family rival, which would be +N — §2.1.) Per the phase-71 V2 finding, tracing rows carry **no `TestNoNewStat`-style guard obligation** — do not manufacture a cited-but-unwritten one.

---

## 3. Framework-survey result — a reject-lift + a resolve source ALREADY at the seam; ZERO new packages/modules (72 anticipated)

### 3.1 Framework: a small seam at most (a `kindMetadataHost` accept arm + a `hostMetaLookup` parameter + one widened `Endpoint`)

No new interface, no new package-level type beyond a `kindMetadataHost` constant (reusing the existing `CustomTagSpec` metadata fields), a new nil-tolerant `hostMetaLookup` parameter on `ResolveCustomTags`, and one added field (plus likely one accessor method) on `cluster.Endpoint`. Every other symbol (`descend`, `structpbValueToString`, `ScalarsFromStruct`, the metadata spec fields, `upstreamHostString`) is pre-existing/reused. **The three `emitAccessLog*` signatures are UNCHANGED** — the only phase in this lineage where they are.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` + `internal/cluster` + `internal/filter/hcm` (all existing) + `test/fixtures` + `docs/`. `internal/dynamicmetadata` is NOT touched (HOST reads endpoint config, not the Bucket). `internal/filter/http/chain.go` is NOT anticipated to be touched either (unlike 70 and 71, which each added an accessor there) — the source reaches the seam via `picked`, not via the chain. NO new package (`reference_new_subpackage_pulls_transitive_module` therefore does not bite; still re-check `git diff go.mod` after tidy at the IMPL).

### 3.3 go.mod modules: NONE

`type.tracing.v3.CustomTag_Metadata` + `type.metadata.v3.{MetadataKind,MetadataKey}` are already reachable (consumed by the 70/71 arms).

**⚠️ EVIDENCE CORRECTED (adversarial verifier V1, MODERATE) — the conclusion survives, the premise did not.** An earlier draft asserted that `corev3` "is already imported across `internal/cluster` (`manager.go` reads `lbe.GetMetadata()`)". That is **FALSE** and was re-derived at tip: `internal/cluster/manager.go`'s import block names `bootstrapv3` / `clusterv3` / `endpointv3` / `upstreamshttpv3` and **NOT `corev3`** — it only CHAINS getters (`lbe.GetMetadata().GetFilterMetadata()[...]`) and never names the type, so no import is needed there. The correct evidence for `corev3` already being an `internal/cluster` dependency is **`internal/cluster/circuitbreaker.go:8`** (`corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"`; also `_test.go` files). Sharper still: **`internal/cluster/cluster.go` — the very file the new `Endpoint` field lands in — imports NEITHER `corev3` NOR `structpb`** (`grep -cE 'corev3|structpb' internal/cluster/cluster.go` ⇒ **0**), so this row DOES add import line(s) there. `structpb` is likewise present in the package only via `subset.go` (+ tests), not `cluster.go`.

So the accurate statement is: `cluster.go` **gains new import lines**, but they resolve to a module the repo **ALREADY REQUIRES** (`github.com/envoyproxy/go-control-plane` for `corev3`; `google.golang.org/protobuf` for `structpb` — both consumed elsewhere in `internal/cluster` and repo-wide). A new import LINE is not a new go.mod MODULE. That is precisely why **`+0 go.mod modules` still holds**, and it is execution-verified rather than asserted (`go mod tidy -diff` ⇒ EMPTY under V1's scratch build of the row's full claimed shape — §11). Cross-checked against `reference_new_subpackage_pulls_transitive_module`: that hazard is about a NEW sub-package pulling a transitive module, and this row creates **no new sub-package** (§3.2), so it does not bite. Still re-run `git diff go.mod` after tidy at the IMPL.

`google.golang.org/protobuf/types/known/structpb` is already imported by `internal/tracing/resolve.go`, `internal/cluster/subset.go`, and `internal/filter/http/chain.go`. `go mod tidy -diff` anticipated EMPTY; the tracked go.mod-modules lineage figure stays **2** (per STATE.md `:34` this is the phase-61.2 figure — `quic-go` direct + `qpack` indirect — **NOT a repo total**; the single `go.mod` requires 67 modules).

### 3.4 REUSES

- **phase-46** the tracing engine: `Span` / `Span.Attrs []KV` / `BuildServerSpan`, the OTLP + Zipkin exporters (both consume `Attrs`), the `test/helpers/otlptrace` receiver.
- **phase-59/62/63** the `custom_tags` pipeline: `parseCustomTags` (the parse home + first-wins dedup) and `ResolveCustomTags` (the resolve home).
- **phase-70** the `metadata`-kind machinery: `descend` (`resolve.go:128`) + `structpbValueToString` (`resolve.go:151`) — reused VERBATIM; the `CustomTagSpec.MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault` fields; the six-arm kind switch (lift the HOST arm); the `k := md.GetKind()` receiver fact.
- **phase-71** the FULL-`MetaPath` one-arg-lookup pattern: the `kindMetadataRoute` config accept arm (`config.go:248-264`) and resolve arm (`resolve.go:95-118`) are the byte-level clone templates; `RouteMetaLookup` (`chain.go:1033-1039`) is the accessor-shape precedent; `0115-tracing-custom-tags-metadata-route` is the fixture chassis.
- **phase-38** the endpoint-metadata substrate: `ScalarsFromStruct` (`subset.go:263`) and the `manager.go:883` populate site — WIDENED beside, not replaced.
- **`FuzzHCMConfigParse`** as the fuzz host (a seed ADDITION, no new fuzzer, no repoint).

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config, with the SOURCE in a STATIC CLUSTER

Like the phase-59/62/63/70/71 `custom_tags` rows, `custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `parseCustomTags` from `HttpConnectionManager.tracing.custom_tags`. No bootstrap change.

Phase 72 introduces ONE structural difference from its predecessors worth naming here: the **SOURCE** now lives in `static_resources.clusters[].load_assignment.endpoints[].lb_endpoints[].metadata`, not in the listener/HCM/route block. So the fixture's config surface spans both the listener and the cluster — but both remain static, both sides read identical YAML, and no bootstrap-level field is newly consumed.

---

## 5. Stat surface hypothesis — +0 (72)

A span attribute registers no stat. Anticipated stat surface **1201 (+0)** (per STATE.md `:31`, a BEHAVIOR_CONTRACT doc count — there is NO mechanical counting command). No `stats.IsValidName` exposure either: the tag key is a literal config string used as a span-attribute key, not a stat name (`reference_dynamic_stat_name_charset_guard` does not apply).

---

## 6. Anticipated edit sites (SPEC RE-DERIVES each at tip — a BRAINSTORM cite is not evidence, `feedback_brief_citations_not_evidence`)

All line numbers below were RE-OPENED at tip `4238a4d3` while writing this document (§11 records the two cites that needed correcting).

- `internal/tracing/config.go:267-268` — LIFT the HOST reject → an accept arm cloning the ROUTE accept (`:248-264`), building `CustomTagSpec{Key: tag, Kind: kindMetadataHost, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}`; the CLUSTER reject (`:265-266`) and both unset-kind rejects (`:229-230`, `:269-270`) UNCHANGED. Anticipated ZERO new imports (`k.GetHost()` on the already-bound `k`).
- `internal/tracing/config.go:54-60` — a new `kindMetadataHost` `customTagKind` constant beside `kindMetadataRoute` (`iota == 5`); the `CustomTagSpec` field comments at `:62-77` extended.
- `internal/tracing/resolve.go:32` — a FIFTH nil-tolerant `hostMetaLookup func(ns string) (*structpb.Value, bool)` parameter on `ResolveCustomTags`, appended after `routeMetaLookup`; the doc comment at `:12-31` extended.
- `internal/tracing/resolve.go` (a new arm after `case kindMetadataRoute:` at `:95-118`) — `case kindMetadataHost:` descending the **FULL** `MetaPath` from `hostMetaLookup(s.MetaNamespace)`, reusing `descend` + `structpbValueToString`, same default rule. NO new import.
- `internal/cluster/cluster.go:43-75` — a NEW raw-metadata field on `type Endpoint struct` beside the phase-38 `Metadata map[string]SubsetValue` (`:46-49`; the comment proper is `:46-48`), plus (anticipated) a small nil-safe namespace accessor mirroring `RouteMetaLookup`'s shape. The `Addr()` (`:81-86`) and `IsZero()` (`:92`) contracts stay UNCHANGED. **NEW IMPORT LINE(S) here** — `cluster.go` imports neither `corev3` nor `structpb` today (§3.3); both resolve to already-required modules, so `+0 go.mod modules` holds.
- `internal/cluster/manager.go:883-884` — populate the new field from `lbe.GetMetadata()` alongside the UNCHANGED `ScalarsFromStruct(...["envoy.lb"])` line.
- `internal/filter/hcm/accesslog_emit.go:57`, `:118`, `:179` — pass a `hostMetaLookup` closure built LOCALLY from the in-scope `picked` as the 5th `ResolveCustomTags` argument. **The three function SIGNATURES (`:27`, `:87`, `:149`) are UNCHANGED, and the 18 callers in `connection.go`/`h2dispatch.go`/`h3dispatch.go` are BYTE-UNTOUCHED** (§1.7) — re-derive and re-confirm this at the SPEC, since it is the row's central cost claim.
- `internal/tracing/config_test.go` — **the TWO tests the row MUST FLIP, named exactly** (they were the ONLY failures under V1's execution probe, §11): the `/host` reject-kind table cases at `config_test.go:639` and `:765`, whose assertions fire at **`:648`** and **`:774`** (`t.Fatalf("NewConfig(%s) err = nil, want reject; ...")` in each table loop). Both flip from "want reject" to an accept assertion; the sibling `/cluster` and unset-kind rows in the SAME two tables stay reject and are the live regression guard.
- `internal/tracing/resolve_test.go` — resolve/path-walk/default/nil-lookup tests, PLUS a trailing `nil` fifth argument at every existing `ResolveCustomTags` call. **Count re-derived at tip: 32 REAL callers** (up from phase-71's 20 — the row grew by 12 last session). The phase-71 informational finding is STILL LIVE: a raw `grep -c 'ResolveCustomTags('` returns **33** because `resolve_test.go:106` is the string literal `t.Errorf("ResolveCustomTags(nil, ...) = %+v, want nil", got)` — exactly ONE non-call line, 33 − 1 = **32**. Re-grep at the PLAN; do not adopt the raw count.
- `internal/tracing/{config_test.go,resolve_test.go}` (additions beyond the two flips + the 32 trailing `nil`s) — the HOST accept, the still-live CLUSTER/unset rejects, and the HOST path-walk / default / absent-namespace cases.
- `internal/cluster/cluster_test.go` / `manager_test.go` — the widened-`Endpoint` populate test; a guard that the `envoy.lb` subset projection is unchanged.
- `internal/filter/hcm/{accesslog_emit_test.go,span_emit_test.go}` — a live HOST-metadata-span test (a picked endpoint carrying metadata → the attribute on the emitted span) + the zero-`picked` default/omit case (which §1.7 now shows is the MAJORITY production arm, so it earns a first-class test, not an afterthought). And re-derived mechanically at tip: the **29** EXISTING `emitAccessLog*` test callers in these two files (13 in `accesslog_emit_test.go` + 16 in `span_emit_test.go`) are **BYTE-UNTOUCHED**, because the three helper SIGNATURES do not change (§1.7) — the test-side mirror of the 18-production-caller claim, confirmed by V1's execution probe (§11). Contrast phases 70/71, where every test caller had to grow a trailing argument.
- `internal/filter/hcm/fuzz_test.go:97-115` — ADD a `meta_host_ok` seed tag to `withMetaTags`; `meta_bad` stays on CLUSTER (no repoint).
- `internal/filter/hcm/fuzz_test.go:92-93` — **a STALE COMMENT the IMPL must update.** The `withMetaTags` doc comment reads *"one REJECTED CLUSTER-kind tag (CLUSTER/**HOST** stay envoy-go-strict departures, ADR-0080)"*. That sentence becomes FALSE the moment the HOST accept arm lands. Narrow it to CLUSTER-only. Cheap, but it is exactly the class of uncited prose that later reads as authority (`reference_code_comment_not_evidence`).
- `test/fixtures/0116-tracing-custom-tags-metadata-host/` — the new OTLP fixture (static `lb_endpoints[].metadata` positive arm + a `default_value` arm), cloning the `0115` chassis; + the one-line `runner_test.go` registration.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the HOST-kind tracing edit.
- `docs/envoy-go/DECISIONS.md` — ADR-0294 (§Context at the SPEC, §Decision + §Consequences at the IMPL IN PLACE).
- `docs/envoy-go/ROADMAP.md` — the deferred-sentence narrow (the `HOST` candidate rolls OUT of the live Observability `candidates:` sentence at the IMPL row-done edit, phase-57 precedent; the sentence currently reads *"…for the non-`REQUEST`/non-`ROUTE` MetadataKinds (`CLUSTER`/`HOST`, reject-only…)"* and narrows to `CLUSTER`-only).

---

## 7. BRAINSTORM-time open questions to the SPEC (the D-CTMH-* docket)

Marked **[PROBE]** where a live arm against `envoyproxy/envoy:contrib-v1.37.2` is REQUIRED, **[CODE]** where a read-only re-derivation suffices. Phase 71 ran six probe arms (P1–P6); a similar set is anticipated. Probe hygiene carries forward: a fresh container per arm (`feedback_probe_fresh_container_per_arm`), a bridge network (`reference_docker_probe_bridge_network`), the host-gateway literal for reference→host reachability plus `dns_lookup_family: V4_ONLY` on `host.docker.internal` clusters (`reference_host_gateway_ip_docker_desktop`), decode-ran asserted per arm, and every arm must DISCRIMINATE between the competing hypotheses rather than merely run (`reference_probe_must_discriminate`).

- **D-CTMH-SCOPE** **[CODE]** — `HOST` MetadataKind only; `CLUSTER` + the unset kind stay reject-only. *Anticipated: confirmed.* Matters because it fixes the row's boundary and the §2.4 reject table.
- **D-CTMH-PROTO** **[CODE]** — the go-control-plane `CustomTag_Metadata` / `MetadataKind` / `MetadataKey` field shapes at the pin, and specifically that `GetHost()` lives on `*metadatav3.MetadataKind` (the phase-70 V1 SEVERE receiver fact). *Anticipated: `k.GetHost()` on the bound `k := md.GetKind()`; `md.GetHost()` does not compile.* Matters because getting the receiver wrong produces unbuildable planned code.
- **D-CTMH-WIRE** **[PROBE]** — does the reference emit the HOST tag as `{key = the literal tag name, value = the serialized string}` on the OTLP span, appended after the built-ins? *Anticipated: identical to REQUEST/ROUTE.* Matters because the fixture asserts key+value cross-side exactly.
- **D-CTMH-ZIPKIN-WIRE** **[PROBE]** — the same tag in the Zipkin `tags` map. *Anticipated: identical.* Matters because envoy-go covers both exporters from ONE `Attrs` seam and the row ships no Zipkin fixture, so the probe is the evidence.
- **D-CTMH-VALUE-SERIALIZE** **[PROBE]** — the `structpb.Value` → string table for a HOST-kind tag. *Anticipated: the phase-70 P3 table exactly (string→raw; `NullValue`→omit; number/bool/struct/list→`protojson` + compact).* Matters because `structpbValueToString` is being reused VERBATIM — if HOST diverged, the reuse would be wrong.
- **D-CTMH-DEFAULT** **[PROBE]** — the `default_value` / omit rule when the namespace or path is absent, and the present-empty-string edge. *Anticipated: the `request_header` rule (present-empty EMITS `""`; absent → default if non-empty, else omit).* Matters because the fixture's second arm IS the default path.
- **D-CTMH-RESOLVE-SEAM** **[CODE]** — the exact shape of the FIFTH `hostMetaLookup func(ns string) (*structpb.Value, bool)` parameter, and where the closure is constructed: inline at the three `ResolveCustomTags` call sites, or behind an `Endpoint`-side accessor (the `RouteMetaLookup` precedent, which also localizes the `structpb` import). *Anticipated: an `Endpoint`-side accessor, threaded as a 5th arg at three sites only.* Matters because it is the row's whole threading story — and because the SPEC must re-confirm the "18 callers untouched" claim by execution, not by citation (`reference_quoting_is_not_executing`).
  **HOT-PATH CONSIDERATION the SPEC should settle (V1 informational).** If the accessor is an `Endpoint` METHOD and it is passed as a **method value** (`picked.MetaLookup`), Go copies the receiver into the closure — and `cluster.Endpoint` is a wide struct (~112 bytes: `Host`/`Port`/the subset map/`Locality`/`LocalityWeight`/`Priority`/… plus the new metadata field), so every emit heap-allocates that copy. This is the per-request span-emit path. Two cheaper shapes: an INLINE closure over `picked` (captures by reference to the existing local), or a package-level `func(ep Endpoint) func(string) (*structpb.Value, bool)` adapter. Not a correctness issue and not a reason to change the seam's SHAPE — but pick deliberately rather than by default, and say which in the SPEC.
- **D-CTMH-ENDPOINT-METADATA** **[CODE + PROBE]** — **THE LOAD-BEARING QUESTION.** What exactly must `Endpoint` retain: the whole `*corev3.Metadata`, or a `map[string]*structpb.Struct` of `filter_metadata` namespaces, or a lazily-built per-namespace `*structpb.Value`? And does widening it disturb (a) the phase-38 subset projection and `defaultSubset` (`manager.go:754`), (b) the ring_hash / maglev / health-state dial identity (all keyed on the precomputed `Addr()` string — §2.5 evidence), (c) locality/priority grouping, or (d) per-endpoint memory at scale? **[PROBE]** half: which namespace(s) does the reference read for a HOST-kind `MetadataKey` — is `envoy.lb` privileged, or is any `filter_metadata` namespace addressable? *Anticipated: any namespace addressable (the same `MetadataKey.key` semantics as ROUTE), retention widened beside the projection, ZERO behavioral disturbance because identity is `Addr()`-keyed — but every clause of that must be re-derived and, for the probe half, EXECUTED.*
  **[CODE] ADDED OBLIGATION — RE-WALK ENDPOINT PROVENANCE THROUGH BOTH CONNECTION POOLS.** This is the most plausible way the row fails SILENTLY: if either pool RECONSTRUCTED an `Endpoint` from the cached addr string, a newly-added metadata field would arrive at the emit seam ZEROED on pool hits, and every test that dials fresh would still pass. V1 walked it at tip and it is CLEAN (§11) — but the SPEC must re-walk it, because a positive finding is exactly the kind that goes stale unnoticed. Re-derive: (H1) `internal/filter/hcm/connection.go:717 picked := rf.Picked()` → `router.go:255 func (f *Filter) Picked() cluster.Endpoint { return f.actionPicked }` → `router.go:307`/`:317` assign it → `router.go:592 picked = pooled.Endpoint()` → `cluster.go:106 func (p *PooledH1Conn) Endpoint() Endpoint { return p.ep }`, where `p.ep` is the WHOLE dial-time struct copy stashed at `cluster.go:709` inside `AcquireH1` (`:657`); (H2) `pooledH2Conn` (`h2pool.go:31-33`) stores `cc` + `inFlight` and **NO endpoint at all**, and `AcquireH2Stream` (`h2pool.go:298`) returns the FRESH `c.lb.Pick(...)` result bound at `:313` — the addr string at `:317` is only the pool MAP KEY. **Neither path reconstructs an `Endpoint` from the addr string**, so a new metadata field survives the pool to the emit seam on both hits and misses.
- **D-CTMH-CONFIG-KIND** **[CODE]** — `kindMetadataHost` as a sixth `customTagKind` constant vs a `MetaSource` sub-enum on the existing kinds. *Anticipated: `kindMetadataHost` at `iota == 5`, matching the one-constant-per-kind pattern 70/71 established.* **COLLISION CHECK ALREADY RUN at tip** (`reference_spec_drafted_identifier_collision_check`): `grep -rn 'kindMetadataHost' internal/` ⇒ **ZERO** occurrences repo-wide, so the name is free — re-run at the SPEC tip anyway. **PLACEMENT is load-bearing: APPEND the constant AFTER `kindMetadataRoute` (⇒ `iota == 5`), never INSERT before it.** Inserting would silently renumber the landed `kindMetadataRoute` value; the constants are unexported and never serialized, so nothing would fail loudly — exactly the kind of change that is free today and a trap the moment anything persists or compares them.
- **D-CTMH-NOHOST** **[PROBE + CODE]** — **UPGRADED to a first-class SPEC obligation by the V1 MODERATE fold (§1.7).** What happens when **no endpoint was selected**: a 404/no-route, a local reply, a 500, a connect failure — any path where `picked` is the zero `cluster.Endpoint{}`. Does the reference omit the tag, emit the `default_value`, or emit empty? *Anticipated: treat it as "namespace absent" ⇒ the default/omit rule, i.e. `default_value` when configured and non-empty, else omit — matching an unresolvable path.* **Two OBLIGATIONS the SPEC must discharge, not one:**
  1. **[CODE] ENUMERATE all 18 emit call sites at the SPEC's OWN tip, each with its `picked` argument, and classify every one as real-endpoint or no-endpoint-selected.** The §1.7 table is BRAINSTORM-time evidence, not a licence to skip the re-derivation (`feedback_brief_citations_not_evidence`); anchors shift. At tip the split is **10 zero-value / 8 real** — a MAJORITY, not an edge, and the H1 split is 4-zero/1-real.
  2. **[PROBE] PIN the reference's behavior PER CLASS** — one live arm driving a request that DOES select an upstream endpoint, and one driving a local-reply / no-route path that does NOT, with the same HOST custom tag configured. The two arms must DISCRIMINATE (`reference_probe_must_discriminate`): an input consistent with both dispositions proves nothing. If the classes differ, that difference is a BEHAVIOR_CONTRACT clause, not a footnote.
  Matters because it is the one behavior with NO analogue in phases 70/71 (both of their sources exist independently of upstream selection), because it is the arm most production traffic on an error path will hit, and because getting it wrong is silent — a tag that quietly stops appearing. NOTE the two dimensions cross: the zero-`picked` set is NOT the nil-lookup set (`h2dispatch.go:313` passes a real `picked` with `nil, nil` lookups; `connection.go:464`/`:597`/`:699` pass the zero endpoint with real lookups), so neither can be used as a proxy for the other.
- **D-CTMH-REJECT** **[PROBE]** — the accept arm's structural edge rejects (empty `metadata_key.key` / empty `path` / empty path segment — PGV parity, as phase-71 P1 established for ROUTE), plus one arm confirming the reference ACCEPTS a `HOST` metadata tag (`--mode validate` OK) and still BOOTS `CLUSTER`, so that reject stays a real documented DEPARTURE. *Anticipated: accept HOST, boot CLUSTER, PGV-reject the three empty-structural edges.* Also re-confirm the phase-71 S3 split wording (nil `*MetadataKind` = DEPARTURE; present-empty oneof = PARITY) is unaffected. And note `reference_pgv_forecloses_go_hazard`: probe the reference before recording any Go-derived divergence.
- **D-CTMH-FIXTURE / D-CTMH-FIXTURE-SOURCE** **[CODE]** — ONE new OTLP dir `0116-tracing-custom-tags-metadata-host` on the `0115` chassis; a static `lb_endpoints[].metadata` block as the source, NO writer; two HOST tags (static hit + `default_value`) asserted cross-side EXACT key+value; a single-endpoint cluster for a deterministic pick; a separate Zipkin dir anticipated **NO**. Also pin `BackendCount >= 1` (`reference_differential_backendcount_min_one`) and the full-subtest `-run` selector for isolate-runs (`reference_differential_run_selector`), and run breaks with `-count=1` AFTER committing (`reference_differential_break_protocol_count1`, `reference_break_protocol_commit_first`).
- **D-CTMH-FUZZSEED** **[CODE]** — ADD a `meta_host_ok` seed to `withMetaTags` (`fuzz_test.go:97-115`); `meta_bad` stays on CLUSTER, **no repoint** (§2.7); dispatch-verify the HCM fuzzer actually reaches the `kindMetadataHost` accept arm. +0 fuzzers (55).
- **D-CTMH-SPLIT** **[CODE]** — SINGLE FLAT ROW anticipated (~9–11 tasks; ADR-0045 valve armable-but-unconsumed). The one thing that could force a split is D-CTMH-ENDPOINT-METADATA turning up behavioral disturbance in `internal/cluster`.

---

## 8. What phase 72 does NOT deliver (forward)

- **The `CLUSTER` MetadataKind** — the LAST of the four. MODERATE (~11–13 tasks), NOT large: it needs a metadata field + accessor on `cluster.Cluster` (`cluster.go:132-…` has none), a `routeAction` interface method (`route.go:52-59`) + a `chain.SetClusterMetadata`/`ClusterMetaLookup` pair re-paying the 18-caller thread, and a documented disposition for `weightedClusterRouteAction`'s per-request in-closure selection (`actions.go:234-245`). **The natural phase-73 candidate**, and the row that CLOSES the `metadata` MetadataKind quartet.
- `spawn_upstream_span` (LARGE — a second CLIENT span model) / OTel `http_service` (MODERATE — a new protobuf-over-HTTP transport) / OTel `sampler` + `resource_detectors` (MODERATE each — `TypedExtensionConfig` registries) / tracing `verbose` (MODERATE — per-stream span logs, a surface `Span` lacks) / force-trace (LARGEST — unbuilt internal-request detection). §2.1.
- The downstream TLS handshake-outcome `ssl` stat family (ADR-0286 C3 — framework surgery CONFIRMED; a NEW stat surface). §2.1.
- A sixth stats sink — **unavailable**, not merely deferred: the only unconsumed `stats_sinks[]` extension is `wasm`. §2.1.
- All xDS candidates (reconnection-backoff — rejected as larger-than-it-looks, §2.1; upstream SDS with its boot-ORDER blocker; CDS/EDS, LDS/RDS, ADS, Delta xDS, `google_grpc`) and all HTTP/3 candidates (`QuicProtocolOptions` — SMALL but needs new harness; upstream H3 cluster; alt-svc; 0-RTT; h3spec; QUIC robustness). §2.1.
- Opening a never-opened family (gRPC — gated on an HCM response-trailer framework row; Runtime — genuinely unbuilt; WASM — a ROADMAP bookkeeping artifact, substantially BUILT). §2.1.
- **`access_log[].filter`** — NEWLY NAMED this session and **flagged as a strong FUTURE pick**: a 13-arm oneof that is a KNOWN proto field and therefore accepted by the strict `DiscardUnknown:false` pass, yet has ZERO production `GetFilter()` consumers — a LIVE behavioral divergence, not merely an unimplemented knob. A single-arm slice (e.g. `status_code_filter`, siblings reject-loud per ADR-0080) is exactly the incremental-arm shape this lineage uses, and the access-log substrate is mature.
- **`stats_flush_on_admin`** — NEWLY NAMED this session; today an explicit ADR-0080 strict-reject at `internal/bootstrap/bootstrap.go:606-607`. Lifting it needs an admin-triggered flush path beside the landed periodic `Flusher` loop.

---

## 9. ADR-0045 split readiness + ADR roster

Anticipated a **SINGLE FLAT ROW** of ~9–11 tasks (ADR-0045 valve armable-but-unconsumed). The shape versus its predecessors: **−1 task** (no 18-caller threading task) and **+1–2 tasks** (the `internal/cluster` `Endpoint` widening plus its ripple/guard tests) — net comparable to phases 70 and 71, which each landed 9 tasks.

**ONE ADR anticipated: ADR-0294.** Re-derived from STATE.md `:21` and `:33`: next-free is **`ADR-0294`**; the DECISIONS.md tail is **ADR-0293** with STATUS **COMPLETE** (§Context/§Decision/§Consequences all landed; §Decision + §Consequences appended IN PLACE at the phase-71 IMPL, no renumber). ADR-0294's §Context drafts at the **SPEC** per ADR-0044 (the DECISIONS tail flips ADR-0293 → ADR-0294 AT that commit); §Decision + §Consequences complete **IN PLACE at the IMPL**, no renumber; next-free after it is ADR-0295. **DECISIONS.md is UNTOUCHED at this BRAINSTORM.**

No new ADR beyond ADR-0294 is anticipated. In particular the `Endpoint`-widening seam sub-note FOLDS INTO ADR-0294 rather than taking its own ADR — the phase-62/63/70/71 precedent (phase 71 folded the `RouteMetaLookup` accessor add into ADR-0293 the same way).

---

## 10. Envelope + counts (anticipated at the phase-72 IMPL; docs-only at this BRAINSTORM)

- ZERO new production packages. Edits in EXISTING files only: `internal/tracing/{config.go,resolve.go}` · `internal/cluster/{cluster.go,manager.go}` · `internal/filter/hcm/accesslog_emit.go` (+ their tests, the fixture, and docs).
- **`internal/cluster` is the ONE genuinely new blast-radius surface vs phases 70/71** (§1.5 — mechanically re-derived: neither predecessor touched it). The envelope-audit implication is FORWARDED to the PLAN: phase 72 cannot assert `internal/cluster` BYTE-UNTOUCHED and must instead pin the change's SHAPE (one added field + one added populate line; `ScalarsFromStruct` and the `envoy.lb` subset projection BYTE-UNCHANGED). The audit's other BYTE-UNTOUCHED assertions carry forward unchanged: `internal/xds` / `internal/tls` / `internal/boot` / `internal/listener` / `internal/bootstrap` / `validate/` / `internal/dynamicmetadata` / `internal/filter/http/ratelimit` / `internal/filter/http/lua`. Anticipated ALSO byte-untouched, and NEWLY so for this lineage: `internal/filter/http/chain.go` and the three HCM dispatch files (`connection.go`, `h2dispatch.go`, `h3dispatch.go`).
- Cycle guard: `go list -deps ./internal/tracing` must still show **no `internal/filter` and no `internal/cluster` edge** — the reused `descend`/`structpbValueToString` are proto-agnostic and the new arm adds no import, so `internal/tracing` stays filter-free AND cluster-free. Re-run at the IMPL (`reference_xds_config_seam_transitive_cycle_guard`: `go list -deps`, no `...`).
- +0 stats (a span attribute) → stat surface **1201 (+0)**.
- +0 fuzzers (a SEED ADDITION to `FuzzHCMConfigParse`, no repoint) → **55**.
- +0 BackendKinds → **38** (`0116` reuses `HTTPFixedBody`; the `otlptrace` receiver is driver-owned; NO writer).
- +0 go.mod modules → **2** (the tracked lineage figure). NOTE the precision §3.3 now carries: `internal/cluster/cluster.go` imports NEITHER `corev3` NOR `structpb` today, so the row DOES add import LINE(s) there — but to modules the repo ALREADY REQUIRES, so the MODULE count is unmoved (`go mod tidy -diff` EMPTY, execution-verified — §11). An import line is not a module.
- fixtures **117 → 118** (`0116-tracing-custom-tags-metadata-host`, OTLP).
- DECISIONS tail **ADR-0293 → ADR-0294** (at the SPEC; next-free ADR-0295).

**Counts UNCHANGED at this BRAINSTORM (docs-only; ALL re-run MECHANICALLY in the worktree at this close, using the CANONICAL commands recorded in STATE.md `:29`/`:30` — an earlier draft cited two non-canonical variants that happen to agree today [`ls test/fixtures | wc -l`, a repo-wide rather than `internal/`-scoped fuzz grep]; both returned the same values, but the fixture variant would OVER-COUNT the moment a non-directory entry appeared under `test/fixtures/`, so the canonical forms are used here):** fixtures **117** (`ls -d test/fixtures/[0-9]*/ | wc -l` ⇒ 117; numeric tail `0115-tracing-custom-tags-metadata-route`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l` ⇒ 55) · stat surface **1201** (BEHAVIOR_CONTRACT doc count; NO mechanical command) · BackendKind **38** (tail `H2GoawayResponder`) · DECISIONS tail **ADR-0293** COMPLETE (next-free **ADR-0294**) · go.mod modules **2** (the phase-61.2 lineage figure; the single `go.mod` requires 67).

---

## 11. Sized-against-source — the cost derivations (two independent read-only agents at tip `4238a4d3`)

Two read-only cost-assessment agents RE-DERIVED each candidate's size against source THIS session, explicitly NOT trusting the phase-70/71 deferral adjectives (`reference_deferred_candidate_cost_restale`). Findings:

- **`metadata`/`HOST` (the pick) — SMALL.** THE DECISIVE FACT: the source is already a parameter of all three emit functions (`accesslog_emit.go:27` / `:87` / `:149` each take `picked cluster.Endpoint`), and the three `ResolveCustomTags` call sites (`:57` / `:118` / `:179`) sit INSIDE those same functions — so the closure is built LOCALLY and **the 18-caller threading task that phases 70 AND 71 each paid is SKIPPED entirely** (18 = 5 in `connection.go` + 6 in `h2dispatch.go` + 7 in `h3dispatch.go`, re-derived mechanically). The config accept arm and the resolve arm are byte-level clones of phase 71's `ROUTE` pair (`config.go:248-264`, `resolve.go:95-118`), `descend`/`structpbValueToString` are reused VERBATIM, and the fuzz delta is purely additive (no `meta_bad` repoint). The ONE real cost is the `Endpoint` metadata widening (§2.5) — an added field plus a one-line populate at `manager.go:883`, with the phase-38 projection byte-unchanged and NO identity ripple (identity is `Addr()`-keyed; `Endpoint` is already non-comparable per its own `cluster.go:88-92` doc comment). +0 stats/pkgs/modules/fuzzers/BackendKinds.
- **`metadata`/`CLUSTER` — MODERATE (~11–13 tasks), the nearest rival.** Three additive gaps: no metadata field on `cluster.Cluster` (`cluster.go:132`); the selected CLUSTER unreachable at the seam (`router.go:255` publishes only `Picked() cluster.Endpoint`) ⇒ a `routeAction` method (`route.go:52-59`) + a `SetClusterMetadata`/`ClusterMetaLookup` pair **re-paying the 18-caller thread**; and `weightedClusterRouteAction`'s per-request in-closure selection (`actions.go:234-245`) needing a departure or a reject. Rejected on SIZE, not impossibility.
- **`spawn_upstream_span` — LARGE.** `BuildServerSpan` is the only span builder; a CLIENT span needs a new model + an upstream-leg timing seam.
- **OTel `http_service` — MODERATE.** A whole protobuf-over-HTTP transport beside the landed `envoy_grpc`.
- **OTel `sampler` / `resource_detectors` — MODERATE each.** Both `TypedExtensionConfig` extension points (registry + ≥1 impl).
- **tracing `verbose` — MODERATE.** Needs per-stream span logs/events; `Span` has no such field.
- **force-trace / internal-request detection — LARGEST.** No `x-envoy-internal` / `x-envoy-force-trace` handling anywhere; an unbuilt subsystem.
- **the `ssl` stat family — LARGE, framework surgery CONFIRMED.** Zero `ssl.*` stats; opaque Go `tls`/`x509` error classification; dynamic cipher/version stat names needing an IANA→OpenSSL translation table for cross-side parity.
- **a 6th stats sink — LARGE / effectively VACUOUS.** All five non-`wasm` `stats_sinks[]` extensions are landed; the only unconsumed one is `wasm`. **There is no cheap sixth.**
- **xDS reconnection-backoff / `initial_fetch_timeout` edges — REJECTED, larger than it looks.** The two agents DISAGREED; the disagreement resolves AGAINST the candidate. `initial_fetch_timeout` is ALREADY landed (`internal/xds/config.go:11-13` 15s default, honored at `:74-75`), and `internal/xds/stream.go` has **NO loop at all** — two single-shot fetchers (`fetchSecret` `:50`, `fetchValidationSecret` `:123`), each one `Send` → one `Recv()` (`:62` / `:134`) → ACK/NACK → return. There is no persistent watch stream to reconnect TO; the row must FIRST build the long-lived stream + a rotation applier. The ROADMAP's own *"SMALL but THIN, initial-fetch-only ⇒ little to reconnect to"* adjective HOLDS.
- **QUIC `QuicProtocolOptions` tuning — SMALL (~9–11) but LOSES on execution risk.** Both `quic.Config` sites are empty literals (`internal/listener/quic.go:37`, `:145`) so the knobs are silently ignored — a real divergence — but ~2 tasks of NEW differential harness are needed first (`test/helpers/h3.go:28-31` builds a fresh `http3.Transport` per call with no connection caching; `func H3RoundTrip` at `:32`), and the natural observables are timing-flaky.
- **upstream SDS / CDS-EDS / LDS-RDS / ADS / Delta xDS / `google_grpc` / upstream H3 / h3spec / QUIC robustness / 0-RTT — LARGE or multi-row.** Upstream SDS additionally carries a boot-ORDER blocker (clusters, including upstream TLS, build before the dialer and before the SDS provider exists ⇒ two-pass cluster construction).
- **Opening a never-opened family — DEFENSIBLE but larger** than a follow-on inside an open family, and the sentinel's labels need care: `WASM` is a bookkeeping artifact (21,550 LOC of wazero host + rows 25/25.1/25.2/25.3 all `done`), `Runtime` is genuinely unbuilt (`internal/runtime/` = `doc.go` only), `gRPC` is gated on an HCM response-trailer framework row (`RunDecodeTrailers` `chain.go:455` / `RunEncodeTrailers` `:622` have ZERO production callers).

### 11.1 EXECUTION EVIDENCE — the row was BUILT in scratch, not merely re-read (adversarial verifier V1)

`reference_quoting_is_not_executing` is the standing rule, and V1 honored it rather than citing around it. V1 did not stop at re-opening citations: it **built the row's full claimed shape in a scratch copy of the tree OUTSIDE the repo** and ran the toolchain over it. The shape built was exactly the one this document anticipates —

- a `RawMetadata *corev3.Metadata` field on `cluster.Endpoint` plus a `MetaLookup(ns)` accessor, populated at `manager.go:883`;
- `kindMetadataHost` plus the config accept arm REPLACING the HOST reject;
- a FIFTH `hostMetaLookup` parameter on `ResolveCustomTags` plus the `kindMetadataHost` resolve arm;
- `picked.MetaLookup` passed as the 5th argument at the three `ResolveCustomTags` call sites,

**with `connection.go` / `h2dispatch.go` / `h3dispatch.go` / `chain.go` NOT TOUCHED AT ALL.** Results:

| gate | result |
|---|---|
| `go build ./...` | **EXIT 0** |
| `go test ./internal/... ./cmd/... ./validate/...` | **ALL PASS** except the two EXPECTED reject-arm tests (§11.2) |
| `go test -race -count=1 ./internal/cluster/` | **ok** |
| `go mod tidy -diff` | **EMPTY** ⇒ +0 modules, execution-verified |
| `go list -deps ./internal/tracing` | **NO `internal/filter` edge, NO `internal/cluster` edge** — the cycle guard holds with the new arm in place |
| `gofmt -l` | **clean** |
| production files differing | **exactly 5** |
| the 18 emit callers | **provably BYTE-UNTOUCHED** |
| the 29 `emitAccessLog*` TEST callers (`accesslog_emit_test.go` 13 + `span_emit_test.go` 16) | **byte-untouched too** — a bonus finding this document had NOT claimed |

This is the strongest evidence the row carries: the central cost claim (§1.7 — the 18-caller thread is SKIPPED) and the central envelope claim (§3.3 — +0 modules) are both **execution-confirmed at BRAINSTORM time**, not asserted. **It is NOT a licence to skip re-derivation.** The standing rule is unchanged: the SPEC RE-DERIVES every anchor at its OWN tip, and the PLAN re-derives again at the IMPL tip. A probe proves the shape is BUILDABLE as scoped today; it says nothing about where the lines will be tomorrow (`feedback_parallel_stream_mints_fresh_drift`).

### 11.2 POSITIVE re-derivation finding — ENDPOINT PROVENANCE THROUGH BOTH POOLS IS CLEAN

Recorded because it is **the most plausible way this row could have failed SILENTLY, and this document never raised it.** If either connection pool reconstructed a `cluster.Endpoint` from its cached addr STRING, a newly-added raw-metadata field would arrive at the emit seam ZEROED on every pool HIT — and every test that dials fresh would still pass, so the defect would ship green. V1 walked both paths at tip:

- **H1 (5 emit sites):** `internal/filter/hcm/connection.go:717 picked := rf.Picked()` → `internal/filter/http/router/router.go:255 func (f *Filter) Picked() cluster.Endpoint { return f.actionPicked }` (assigned at `:307`/`:317`) → `router.go:592 picked = pooled.Endpoint()` → `internal/cluster/cluster.go:106 func (p *PooledH1Conn) Endpoint() Endpoint { return p.ep }`. `p.ep` is the WHOLE dial-time struct copy, stashed at `cluster.go:709` inside `AcquireH1` (`:657`). **On a pool HIT the dial-time `ep` is returned intact.**
- **H2 (6 emit sites):** `pooledH2Conn` (`internal/cluster/h2pool.go:31-33`) holds `cc *h2.ClientConn` + `inFlight int64` and **NO endpoint field**. `AcquireH2Stream` (`h2pool.go:298`) returns the FRESH `c.lb.Pick(...)` result bound at `:313`; the `addr := ep.Addr()` at `:317` is only the pool MAP KEY, never a reconstruction source.

**Neither path reconstructs an `Endpoint` from the addr string**, so a new metadata field survives the pool to the emit seam on hits and misses alike. Folded into D-CTMH-ENDPOINT-METADATA (§7) as an explicit SPEC re-derivation obligation — a positive finding is exactly the kind that goes stale unnoticed.

**The two tests the row must flip, named:** `internal/tracing/config_test.go:648` and `:774` (the assertion lines of the two `/host` reject-kind table cases, whose table rows sit at `:639` and `:765`). Under V1's probe these were the **ONLY** failures across `./internal/... ./cmd/... ./validate/...` — a precise, pre-known red set for the TDD spine.

### 11.3 Cite-drift found and corrected against the INCOMING brief

Recorded per `feedback_brief_citations_not_evidence` — every `file:line` below was RE-OPENED at tip:

1. **`internal/tracing/config.go:268` (the HOST reject) — the LINE is exact, but the ARM spans `:267-268`** (`case k.GetHost() != nil:` at `:267`, the `return` at `:268`). The sibling CLUSTER arm is likewise `:265-266`, not the bare `:266`. This document uses the ARM ranges throughout, since the accept-arm replacement consumes both lines.
2. **The clone-source accept arm is `config.go:248-264`** (`case k.GetRoute() != nil:` at `:248`). The brief cited it only as "immediately above the CLUSTER reject", which is exactly right; the range is recorded here so the SPEC can diff it byte-for-byte.
3. **`resolve.go` — the `[1:]` slice is at `:84`, inside the `kindMetadata` arm whose `case` label is at `:71`.** The brief attributed the `[1:]` to `:71`; the arm attribution is correct, the literal line is `:84`. Recorded so the PLAN's break-instructions target the right line.
4. **`ScalarsFromStruct` is `internal/cluster/subset.go:263-285`** (the brief said "lives in `internal/cluster/subset.go`" without a line). Its drop set, now stated exactly: `Value_StructValue`, `Value_ListValue`, `Value_NullValue`, and nil kinds all route to the discarded `nonScalar` return.
5. **`withMetaTags` is `internal/filter/hcm/fuzz_test.go:97-115`**, with `meta_ok` at `:100-104`, `meta_route_ok` at `:105-109`, `meta_bad` (CLUSTER) at `:110-112` — the brief's description of the seed contents HOLDS exactly; only the line ranges are added here.
6. **"go.mod modules 2" is a LINEAGE figure, not a repo total.** Re-derived: `find . -name go.mod` returns exactly ONE file, which requires 67 modules; STATE.md `:34` records that the tracked **2** is the phase-61.2 count (`quic-go` direct + `qpack` indirect). Stated explicitly here so the SPEC/PLAN do not "correct" it.
7. **"fixtures 117"** is the CANONICAL count from STATE.md `:29` — `ls -d test/fixtures/[0-9]*/ | wc -l` ⇒ **117**. Not the count of `^[0-9]{4}-`-prefixed dirs under a stricter regex (115 — `0007a`/`0007b`-style suffixed siblings share a numeric prefix). Both figures re-derived; the project's tracked count is **117**.

**No cite in the incoming brief was found FALSE.** Every load-bearing claim — the HOST reject text, the ROUTE clone source, the FULL-`MetaPath` rule, the four-param `ResolveCustomTags` signature, the `picked` 4th parameter on all three emit functions, the three in-function `ResolveCustomTags` call sites, the `envoy.lb`-scalars-only `Endpoint` projection and its `manager.go:883` populate, the CLUSTER-pointed `meta_bad` seed, the `0115` chassis, and every count — RE-DERIVED and HELD.

### 11.4 Verifier fold — corrections to THIS DOCUMENT'S OWN earlier draft (2026-07-23)

Two independent adversarial verifiers (V1 code-claims + by-execution; V2 process/consistency) read this document at `fafcf051` before the squash. Their findings are folded in ABOVE at the point of use; the ledger is here so the SPEC can see what moved and why. **One of them found a claim in this document FALSE** — the sentence above about the incoming brief is not a claim of this document's own infallibility.

| # | severity | finding | disposition |
|---|---|---|---|
| 1 | MODERATE | §1.7/§2.1 materially UNDERSTATED the zero-`picked` exposure ("the 3 nil-lookup sites, two of which pass the zero value"). Mechanically: **10 of 18** emit sites pass `cluster.Endpoint{}`; only **8** carry a real `picked` (H1 **1 of 5**). | §1.7 REWRITTEN with the full 18-row classification table; the consequence stated plainly (a HOST tag falls to `default_value`/omit on every local-reply / 500 / no-route path on all three protocols); **D-CTMH-NOHOST UPGRADED in §7** into a two-part SPEC obligation (enumerate all 18 with their `picked` arg at the SPEC tip; PROBE the reference per class). Row SIZE unchanged — no threading, and the zero-`Endpoint` lookup is nil-safe. 18 callers still BYTE-UNTOUCHED. |
| 2 | MODERATE | §3.3's `corev3` evidence was **FALSE**: `internal/cluster/manager.go` does NOT import `corev3` (only chains getters). And `internal/cluster/cluster.go` — the file the new field lands in — imports **neither** `corev3` nor `structpb`. | §3.3 CORRECTED: the real evidence is `internal/cluster/circuitbreaker.go:8`; `cluster.go` DOES gain import line(s), from ALREADY-REQUIRED modules. The **`+0 go.mod modules` conclusion SURVIVES** and is execution-verified (`go mod tidy -diff` EMPTY, §11.1). `reference_new_subpackage_pulls_transitive_module` cross-checked: no new sub-package ⇒ no transitive module. §10 note aligned. |
| 3 | MINOR | §2.1 CORRECTION #2 called the `CLUSTER`-metadata plumbing "a different (and **smaller**) problem" — unsupported; both are blocked by the SAME missing seam (gap (b)), and cluster *metadata* is if anything a SUPERSET of the cluster *name*. | "smaller" DROPPED. The **category-error** framing (the `upstream_cluster` TAG gap is a different feature) is sound and STAYS. |
| 4 | MINOR | §2.1 CORRECTION #1 said phase 71 "never noticed that the endpoint is already a parameter of the emit functions" — self-inconsistent: phase-71 BRAINSTORM `:55` says the picked `Endpoint.Metadata` **IS in scope at the seam**. | REWORDED to claim only what is true: phase 71 saw the FACT and never derived its **COST CONSEQUENCE**. Cite re-opened and confirmed. |
| 5 | MINOR | The TITLE quoted `custom_tags metadata host kind unsupported` — a compressed form that is **NOT a substring** of the real message, so a literal grep returns zero hits. §1.1 quoted it correctly. | TITLE fixed to the verbatim `tracing: custom_tags metadata tag %q host kind unsupported` (`config.go:268`). Rest of the document swept: line 1 was the ONLY occurrence. |
| 6 | MINOR | §12 under-declared the ROADMAP edit actually performed — the envelope commit ALSO appended a phase-72 charter sentence to the `### Observability family` heading paragraph. | §12 now DECLARES both edits. Precedent-backed (phase 70's BRAINSTORM did the same; phase 71's did not) and verified harmless: additions only, deferred sentence byte-unchanged, sentinel still **3**. |
| 7 | MINOR | The Observability ordinal is ONE LOW (inherited): mechanically **19** rows, but row 69 carries no ordinal so 70/71/72 each run one short. | **NOT renumbered** — recorded in §2.1 as a ROADMAP bookkeeping artifact beside the `NEVER OPENED: WASM` one. Phase 72 keeps `EIGHTEENTH` for chain consistency with two landed `done` rows; the true count (19) is on the record for a future single deliberate correction. |
| 8 | MINOR | Two non-canonical count commands in §10/§2.7 (`ls test/fixtures \| wc -l`; a repo-wide rather than `internal/`-scoped fuzz grep). Both returned the SAME values today (117/117, 55/55) so no wrong number was published. | SWAPPED to the canonical STATE.md `:29`/`:30` forms. The fixture variant would over-count if a non-directory entry appeared. |
| 9 | MINOR | Cosmetic line drift: §2.5 point 2 cited `cluster.go:46-49` for the comment (it is `:46-48`; `:49` is the field line — the quoted `:43-49` block cite is exact); `test/helpers/h3.go:30-50` for the fresh-Transport sentence (it is `:28-31`; `func H3RoundTrip` opens at `:32`). | Both fixed in place, at both occurrences of the h3 cite. |
| — | INFORMATIONAL | V1's execution probe; endpoint provenance through both pools; the two tests to flip; the 32-vs-33 `resolve_test.go` caller count; the `kindMetadataHost` collision check + iota placement; the method-value allocation nit; the stale `fuzz_test.go:92-93` comment. | ADDED as §11.1, §11.2, and folded into §6 and the §7 docket (D-CTMH-NOHOST, D-CTMH-ENDPOINT-METADATA, D-CTMH-CONFIG-KIND, D-CTMH-RESOLVE-SEAM). |

`metadata`/`HOST` is the smallest row that reuses the proven, freshest substrate whole, is the ONLY remaining `metadata` MetadataKind that escapes the 18-caller threading tax, adds a genuine differential-provable capability, and leaves exactly one cheap tracing slice (`CLUSTER`) behind it for phase 73.

---

## 12. Stage-close mechanics (this BRAINSTORM; the CONTROLLER executes these, not this document)

- **Row 72 registered `in-progress`** in ROADMAP §Schema at the stage-close commit (the §Schema invariant — a new subject re-opens sentinel check (1)). Flips `done` at the phase-72 IMPL six-gate (ADR-0106, the SOLE leg — a SINGLE FLAT ROW, `reference_roadmap_split_phase_row_done`).
- **ROADMAP takes a SECOND edit at the same commit: a phase-72 charter sentence APPENDED to the `### Observability family` heading paragraph** (naming the row, its anticipated envelope, the newly-surfaced `access_log[].filter` / `stats_flush_on_admin` deferred candidates, and that the family STAYS OPEN). Declared here because declared mechanics must match performed mechanics — an earlier draft of this section named only the §Schema row registration. **Precedent-backed and verified harmless:** phase 70's BRAINSTORM made the same paired edit (phase 71's did not, so both shapes are live in the lineage); the edit is ADDITIONS ONLY; the live Observability **deferred sentence is BYTE-UNCHANGED** (it narrows at the IMPL, not here); and sentinel check (2) still returns exactly **3**. `git diff master...HEAD -- docs/envoy-go/ROADMAP.md` is 2 insertions / 1 deletion across exactly these two hunks.
- **NO deferred sentence is narrowed at the BRAINSTORM** (the phase-57 precedent). The live Observability `candidates:` sentence names *"tracing custom_tags `metadata` for the non-`REQUEST`/non-`ROUTE` MetadataKinds (`CLUSTER`/`HOST`, reject-only …)"*; the `HOST` candidate rolls OUT at the **phase-72 IMPL row-done edit**, narrowing it to `CLUSTER`-only. Keeping EXACTLY ONE live match through the pre-IMPL stages is what makes sentinel check (2) meaningful (`reference_sentinel_deferred_sentence_live_vs_historical` — the live grep needs `candidates:` adjacent; historical recaps say `candidates were:`).
- **DECISIONS UNTOUCHED** (ADR-0294 §Context drafts at the SPEC per ADR-0044; §Decision + §Consequences complete IN PLACE at the IMPL, no renumber).
- **STATE §Current rolled IN PLACE** (lifecycle DONE → 1; lineage re-capped at five). Never prepend a new block above §Current (the ADR-0288 rule).
- **next-prompt.txt rolled to the phase-72 SPEC** (it is TRACKED despite `.gitignore`; edit it in the stage worktree — `reference_next_prompt_tracked_despite_gitignore`).
- **Sentinel re-run MECHANICALLY at close** (does NOT fire; `stop` NOT created): (1) prints `NOT DONE: row 72` (the registration re-opens check (1)); (2) `grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` ⇒ **3**; (3) prints `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`. Checks (2)+(3) print ⇒ abundant work remains. (Check (3)'s `WASM` line is the bookkeeping artifact documented in §2.1 — it is left AS-IS at this row.)
- **Next → the phase-72 SPEC** (the D-CTMH-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2`; RE-DERIVE every §6 edit site + the go-control-plane metadata proto types at the SPEC tip; resolve D-CTMH-ENDPOINT-METADATA, the load-bearing one; draft ADR-0294 §Context).
