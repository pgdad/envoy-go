# Phase 71 Brainstorm — tracing `custom_tags` (METADATA tag type, `ROUTE` MetadataKind) (the SEVENTEENTH Observability-family row; the FIFTH tracing `custom_tags` capability and the SECOND `metadata` MetadataKind; lifts the `custom_tags metadata route kind unsupported` reject and resolves a `ROUTE`-kind route-config metadata value onto the span over the LANDED `FilterChain.RouteMetadata() *corev3.Metadata`; the `CLUSTER`/`HOST` MetadataKinds stay PARSE-REJECTED loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

**Status: BRAINSTORM. Docs-only. ZERO production `.go`. Row 71 registered `in-progress` at this commit per the ROADMAP §Schema invariant.**

---

## 1. Mission and scope confirmation (71 — the SECOND `metadata` MetadataKind; a `ROUTE`-kind slice on the LANDED `RouteMetadata()` accessor)

### 1.1 What phase 71 delivers as a self-contained whole (a route-config metadata value on the ingress span)

Phase 70 landed the tracing `custom_tags` `metadata` type for the **`REQUEST`** MetadataKind (the per-request dynamic-metadata `Bucket`). Phase 71 lifts the **`ROUTE`** MetadataKind reject (`internal/tracing/config.go:245-246`, `case k.GetRoute() != nil: return ... "route kind unsupported"`) and resolves the **matched route's static config metadata** (`route.metadata.filter_metadata[ns]`) onto the ingress SERVER span. The source is ALREADY LANDED: `func (c *FilterChain) RouteMetadata() *corev3.Metadata` (`internal/filter/http/chain.go:1156`, phase 24.2 / ADR-0165), seeded per-request by `SetRouteMetadata(entry.metadata)` at the three dispatch sites (`connection.go:433` H1 / `h2dispatch.go:375` H2 / `h3dispatch.go:195` H3) and reachable at the exact emit call sites that already thread `chain.DynamicMetadata().Get`. The resolved value is appended to `Span.Attrs []KV`, emitted on BOTH the OTLP and Zipkin exporters via the phase-46 seam. `CLUSTER` and `HOST` stay PARSE-REJECTED loudly with distinct substrings (envoy-go-strict DEPARTURE, ADR-0080).

This is a complete, useful, differential-provable capability: a span tag carrying a value from the matched route's `filter_metadata` (e.g. a team/service label attached to a RouteConfiguration route) — the second of the four `metadata` MetadataKinds, on landed substrate.

### 1.2 What phase 71 does NOT deliver (forward to §8)

- The `CLUSTER` and `HOST` MetadataKinds (stay reject-only; `CLUSTER` is the documented `upstream_cluster` framework gap — the selected cluster's identity/metadata is NOT plumbed to the span-emit seam, `reference_tracing_upstream_cluster_framework_gap`; `HOST` has only the reduced `envoy.lb` scalar subset in scope at the seam, not the full `*corev3.Metadata`). §8.
- `spawn_upstream_span` / `http_service` / force-trace (the other live Observability candidates). §8.
- The `ssl` handshake-outcome stat family (ADR-0286 C3 framework-surgery + a NEW stat surface). §8.

### 1.3 Phase-done as the SEVENTEENTH Observability-family row landing (family STAYS OPEN)

Row 71 is the seventeenth Observability-family row and the fifth tracing `custom_tags` capability (literal @ 59, request_header @ 62, environment @ 63, metadata/`REQUEST` @ 70, metadata/`ROUTE` @ 71). After it lands, the Observability family STAYS OPEN — the `CLUSTER`/`HOST` MetadataKinds, `spawn_upstream_span`, `http_service`, force-trace, and the `ssl` stat family remain deferred candidates.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered, SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~8–10 tasks), the same shape as phase 70 (which landed as a single flat row of 9 tasks). The ADR-0045 split valve is armable-but-unconsumed: if the SPEC discovers the ROUTE default-rule probe or the fixture materially grows the row (e.g. a Zipkin dir + a route-metadata-writer wrinkle), it may split; the anticipated posture is single flat.

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files, ZERO new packages

Every edit lands in EXISTING files: `internal/tracing/config.go` (lift the ROUTE reject → accept arm), `internal/tracing/resolve.go` (a `kindMetadataRoute` arm + the new `routeMetaLookup` param), `internal/filter/hcm/accesslog_emit.go` (the `routeMetaLookup` param on the three emit helpers) + the 18 emit callers in `connection.go`/`h2dispatch.go`/`h3dispatch.go`, plus `test/fixtures/` + `docs/`. NO new package. `internal/dynamicmetadata` is untouched (ROUTE reads route config, not the Bucket). `descend` + `structpbValueToString` (`resolve.go:100`/`:123`) are REUSED VERBATIM.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch applies (`reference_phase_11_local_ratelimit_prebrainstorm` is `local_ratelimit`-only).

### 1.7 Phase 71's relationship to the existing seams (a reject-lift + a resolve source on landed engines)

Phase 71 is a strict incremental clone of the phase-70 seam: lift one reject arm, add one resolve arm reading a DIFFERENT (already-landed) source, thread one more nil-tolerant lookup closure to the emit sites. It touches the SAME functions phase 70 touched, one arm deeper.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` (`metadata`/`ROUTE`) *(SELF-PICKED per the standing directive → phase 71 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest defensible ROW-SIZED candidate** after INVESTIGATING each candidate's size against source THIS session (two independent read-only cost-assessment agents at tip `e2912f6f`; §11). Row 71 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `custom_tags` (`metadata`/`ROUTE`) is smallest-defensible:** it sits on landed substrate on THREE axes — (a) the phase-70 `kindMetadata` resolve machinery (`descend` + `structpbValueToString`, reused VERBATIM), which landed LAST SESSION (freshest in the codebase, lowest execution risk); (b) the `FilterChain.RouteMetadata() *corev3.Metadata` accessor (`chain.go:1156`, phase 24.2), reachable at the exact emit call sites that already thread `chain.DynamicMetadata().Get`; (c) a cross-side-deterministic source that needs NO runtime writer at all — the matched route's STATIC config metadata (simpler than phase 70, which needed a Lua writer for the dynamic Bucket). It is a near-exact clone of the phase-70 pattern (lift ONE reject, add ONE resolve arm, thread ONE more nil-tolerant closure), carries NO ADR headwind, and is **+0 stat** (a span attribute, not a stat). Scoping to the `ROUTE` kind keeps it a single flat row: `ROUTE` is the one non-`REQUEST` MetadataKind whose full source (`*corev3.Metadata`) is landed and reachable at the seam.

**⚠️ STALE-COST CORRECTION (this session, §11):** the phase-70 BRAINSTORM (§1.2, §8) lumped all three non-`REQUEST` MetadataKinds as "their per-object metadata is not exposed at the span-emit seam." That is **now partially STALE for `ROUTE`** — `chain.RouteMetadata()` (phase 24.2) is landed and reachable at the emit call sites (the cost-assessment agent RE-DERIVED this at tip). So the *smallest defensible* slice of the "non-`REQUEST` MetadataKinds" candidate is `ROUTE`-only, NOT the whole triple. `CLUSTER` remains the true framework gap; `HOST` is partial. Do not trust the deferral's own adjective (`reference_deferred_candidate_cost_restale`).

**Rejected alternatives (recorded per the standing directive; each RE-DERIVED/SIZED against source this session — §11):**
- **`CLUSTER` MetadataKind (the same candidate's cluster arm)** — a TRUE framework gap: the selected cluster's identity/metadata is NOT plumbed to the span-emit seam (`accesslog_emit.go` hard-codes `UpstreamCluster: ""` with a `// not available at this seam` note; `reference_tracing_upstream_cluster_framework_gap`, BEHAVIOR_CONTRACT / DECISIONS record the upstream cluster NAME itself unreachable at stream-completion). Accepting `CLUSTER` metadata requires building that plumbing first. LARGE. Deferred.
- **`HOST` MetadataKind (the same candidate's host arm)** — PARTIAL: the picked `cluster.Endpoint.Metadata` IS in scope at the seam, but it is the REDUCED `envoy.lb` scalar-subset map (`cluster.go:46-49`), not the full `*corev3.Metadata` with arbitrary filter namespaces the reference walks. Full parity needs the complete host metadata plumbed. MODERATE. Deferred (scope this row to `ROUTE`-only to stay small — the incremental-arm precedent).
- **reconnection-backoff / `initial_fetch_timeout` edges (xDS)** — SMALL in raw lines, but THIN/low-value: `initial_fetch_timeout` is already implemented (`internal/xds/config.go:13`/`provider.go:57-60`); the delta is a bounded retry/backoff loop, but the SDS substrate is initial-fetch-only with a per-fetch stream abandoned on return, so "there is little to reconnect to" (the cost agent's words) — the only meaningful edge is retrying the *initial* fetch when mgmt is down. Genuinely small but "arguably the least capability gained." Deferred: `ROUTE` metadata gains a genuine differential-provable feature at comparable size + lower execution risk (clones a just-landed seam vs a different subsystem).
- **upstream SDS (xDS)** — MODERATE, the strongest xDS "next": it reuses the landed SDS handshake/parser/stats machinery WHOLE (no new lifecycle, no mutable seam — still initial-fetch, still static `Certificates`), the delta being threading a `SecretProvider` into `NewUpstreamConfig` → `commonTLSContextToConfig` for the upstream side and lifting the two `side != "downstream"` rejects (`config.go:380-382`/`436`/`453`). But the provider must reach cluster-build time in `internal/cluster` (the `xdsgrpc` package exists precisely to dodge the `grpcclient→cluster→tls` cycle), so it is bigger than `ROUTE` metadata. Deferred; noted as the strongest xDS candidate when the Observability tail's cheap slices drain.
- **`http_service` (OTLP-over-HTTP tracer transport)** — MODERATE; a clean transport swap behind the landed `TracesClient` interface (`exporter.go:67`, reusing all `OTLPExporter` batching) with two precedents to clone (the Zipkin HTTP-POST exporter + the extproc `http_service` variant), but a new HTTP-protobuf transport + parse + wiring. The phase-70 reject-line cite (`config.go:236-237`) is now STALE — phase 70's growth of `parseCustomTags` shifted the `http_service` reject to `config.go:274-275`. Larger than `ROUTE`. Deferred.
- **`spawn_upstream_span`** — MODERATE-LARGE: envoy-go emits ONE SERVER span (`BuildServerSpan`, `internal/tracing/span.go` models only the server span); a second CLIENT span needs a client-span builder, an upstream-leg start/end timing seam that doesn't exist today, parent/child linkage, and a two-span export path. Touches the span model. Deferred.
- **force-trace (`x-envoy-force-trace`)** — LARGEST: ZERO handling anywhere in `internal/`; `DecideWithContext` (`decision.go:72-116`) handles continued / `x-client-trace-id` / random / overall only; the `ServiceForced` `SampleClass` is dead (`stats.go:26-27` confirms). Envoy honors `x-envoy-force-trace` only for INTERNAL requests, which needs internal-vs-external request classification (trusted-hop counting / `internal_address_config`) + edge header sanitization — a whole unbuilt subsystem. HIGH scope-risk. Deferred.
- **SDS rotation (xDS)** — LARGE: requires a retained long-lived stream + receive loop (today `fetchSecret` does exactly one `Recv` and returns, `stream.go:62`), a MUTABLE-cert seam (`cfg.Certificates` is a static slice, `internal/tls/config.go:495-496` — no `GetCertificate` callback, no atomic swap), AND a new running-subsystem lifecycle. The one candidate that forces a new mutable-cert seam. Disqualified as "smallest." Deferred.
- **`ssl.*` downstream handshake-outcome stat family** (ADR-0286 C3) — a framework-surgery row + a NEW stat surface breaking the long-standing +0-stat streak; the discriminating `fail_verify_error` vs `fail_verify_no_cert` classification needs an opaque `crypto/tls` handshake-error callback wired into every per-chain `*stdtls.Config`. MEDIUM→LARGE with an ADR headwind. Deferred (metadata is cleaner: lineage precedent, no ADR headwind, +0 stat).
- **A SIXTH stats sink (`envoy.stat_sinks.hystrix` / `wasm`)** — hystrix is admin-HTTP-endpoint SSE streaming (not a periodic push of a registry snapshot), off the landed `Flusher`/`Sink.Submit(batch)` model; wasm needs the wasm runtime. LARGE/BLOCKED. Deferred (carried from the phase-70 roster; re-confirmed not newly-cheap at tip).
- **RBAC principal `require=false` fixture arm** — the RBAC engine is COMPLETE; a TEST-FIXTURE-ONLY rider with ZERO engine work ⇒ rider-sized, NOT a standalone row. Deferred as not-row-sized.
- **DataSource `environment_variable`** — a ~1-line `os.Getenv` per site but BLOCKED by the SPEC-63 D-ENV-HARNESS harness-seam deferral decision. Deferred (blocked-by-decision).
- **`watched_directory` / live per-chain TLS reconfig** — the `fsnotify` watcher substrate exists (`internal/sdsfile`) but consuming a rotation event needs live per-chain `*stdtls.Config` reconfig (UNBUILT). MEDIUM→LARGE. Deferred.
- **HTTP/3 candidates** (upstream H3 cluster / alt-svc / 0-RTT / h3spec gate / QuicProtocolOptions / full QUIC transport-socket options / QUIC robustness) — each needs new substrate or has near-zero differential value. Deferred.
- **Opening a NEVER-OPENED family** (gRPC / Runtime / WASM) — the standing directive says smallest-defensible-first, and the Observability tail STILL holds a cheap ROW-SIZED candidate (`metadata`/`ROUTE`, on landed substrate), so smallest-first keeps us in Observability. Deferred; revisit when the Observability tail's cheap candidates are drained.

### 2.2 Scope: the `metadata` type, `ROUTE` MetadataKind ONLY; the other two remaining kinds PARSE-REJECT loudly *(self-answered; the incremental-arm precedent)*

Phase 71 supports `kind == ROUTE` (the matched route's static config metadata, envoy-go's `FilterChain.RouteMetadata()`); `CLUSTER`/`HOST` continue to REJECT loudly with distinct substrings (§2.4). This mirrors the project's landed incremental-arm posture — the `custom_tags` types landed one at a time (literal/59, request_header/62, environment/63), and the `metadata` MetadataKinds now land one at a time (`REQUEST`/70, `ROUTE`/71). A `ROUTE`-kind slice is a complete, useful, differential-provable capability (a tag carrying a route-config-attached label onto the span). The SPEC probe (D-CTMR-WIRE) confirms the reference emits it identically.

### 2.3 The resolve seam: a NEW `kindMetadataRoute` arm in `ResolveCustomTags` reading a route-metadata source; append to `Span.Attrs` — ONE change, TWO providers *(self-answered shape; SPEC pins — D-CTMR-RESOLVE-SEAM)*

`ResolveCustomTags(specs, headerLookup, metaLookup)` (`resolve.go:26`) currently resolves literal/header/environment/`kindMetadata`(REQUEST) specs. Phase 71 adds a `kindMetadataRoute` arm reading a NEW route-metadata source. **The source-shape asymmetry is the one genuine design question** (D-CTMR-RESOLVE-SEAM):
- The `REQUEST` source is the `Bucket`: `metaLookup(ns, key) → *structpb.Value` — the Bucket PRE-KEYS the first path segment, so the arm descends `MetaPath[1:]`.
- The `ROUTE` source is `*corev3.Metadata`: `md.GetFilterMetadata()[ns] → *structpb.Struct` — the namespace yields a STRUCT, so the arm wraps it as a `StructValue` and descends the **FULL** `MetaPath` (all segments), NOT `MetaPath[1:]`.

Anticipated seam (SPEC pins): a new nil-tolerant `routeMetaLookup func(ns string) (*structpb.Value, bool)` parameter on `ResolveCustomTags` (returning the namespace's struct wrapped as a `StructValue` via `structpb.NewStructValue`, or `(nil,false)` when the namespace is absent), symmetric with `metaLookup`. The `kindMetadataRoute` arm: `v, ok := routeMetaLookup(s.MetaNamespace)`; `if ok { v, ok = descend(v, s.MetaPath) }`; then `structpbValueToString(v)` — **`descend` and `structpbValueToString` REUSED VERBATIM** (they are proto-agnostic — `internal/tracing` stays filter-free). `routeMetaLookup` nil ⇒ route metadata specs use default/omit, keeping all existing behavior byte-identical when no ROUTE tags are configured. (An alternative single-combined-lookup seam is possible; the SPEC weighs the added-param clarity vs a combined closure — the anticipated shape is the symmetric second param, mirroring `metaLookup`.)

Two behavioral sub-questions the SPEC probes:
- **D-CTMR-VALUE-SERIALIZE** — the reference's `structpb.Value`→string serialization for a ROUTE-kind tag SHOULD match the phase-70 P3 table (string→raw / NullValue→omit / else `protojson.Marshal`+`json.Compact`), since it is the same `MetadataKey` resolution machinery on the reference side. The fixture's positive arm SHOULD use a STRING metadata value (the unambiguous case); the SPEC confirms the shared serialization via one probe arm.
- **D-CTMR-DEFAULT** — the `default_value` / omit semantics when the route metadata path is absent or the resolved value is empty. Anticipated: the SAME `request_header` default rule phase 70 landed for `REQUEST` (present-empty EMITS `""`; absent → `default_value` if non-empty, else omit; `HasDefault = DefaultValue != ""`). The SPEC probes whether the reference treats ROUTE identically to REQUEST here (the shared `MetadataKind` resolution suggests yes) and pins it.

### 2.4 The two remaining unsupported MetadataKinds reject loudly with DISTINCT substrings — the envoy-go-strict DEPARTURE narrows by one *(self-answered; ADR-0080)*

The reference SUPPORTS all four MetadataKinds; envoy-go now supports `REQUEST` (@70) + `ROUTE` (@71) and rejects `CLUSTER`/`HOST`. The two remaining rejects keep their distinct substrings (unchanged from phase 70):
- `tracing: custom_tags metadata tag %q cluster kind unsupported` (`config.go:247-248`, unchanged)
- `tracing: custom_tags metadata tag %q host kind unsupported` (`config.go:249-250`, unchanged)
The ROUTE reject (`config.go:245-246`) is REPLACED by an accept arm cloning the REQUEST accept (`config.go:228-244`): the same `mk := md.GetMetadataKey()` / `path` extraction + the same empty-namespace/empty-path/empty-segment PGV-parity rejects. The exact accept-arm shape + whether the reference PGV-boot-rejects any structural edge for ROUTE vs envoy-go's own reject are SPEC-pinned (D-CTMR-REJECT). D-CTMR-REJECT also confirms (one probe arm) the reference ACCEPTS a `ROUTE` metadata tag (proving the phase-70 departure was real) and still BOOTS `CLUSTER`/`HOST` (so those two rejects stay a real envoy-go-strict DEPARTURE).

### 2.5 Config home + threading: extend the `kindMetadata` machinery to a `kindMetadataRoute`; thread `RouteMetadata()` to the emit sites *(self-answered shape; SPEC pins the exact kind representation — D-CTMR-CONFIG-KIND / D-CTMR-BUCKET-SEAM)*

The parsed ROUTE spec lives on the EXISTING `CustomTagSpec` (`config.go:63-74`), reusing `MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault`. Two seam decisions the SPEC pins:
- **D-CTMR-CONFIG-KIND** — how to represent ROUTE vs REQUEST on the spec: a new `kindMetadataRoute` constant (the anticipated shape — keeps the resolve switch explicit and mirrors the one-constant-per-kind pattern) vs a `MetaSource` sub-enum field on `kindMetadata`. Anticipated: `kindMetadataRoute` (a new `customTagKind` constant beside `kindMetadata`).
- **D-CTMR-BUCKET-SEAM** (route variant) — how `chain.RouteMetadata()` reaches `ResolveCustomTags` at the emit sites. Every span-capable emit caller ALREADY has `chain` in scope (it threads `chain.DynamicMetadata().Get` as `metaLookup`), so `chain.RouteMetadata` is equally reachable. Anticipated: thread a `routeMetaLookup` closure over `chain.RouteMetadata()` as a NEW param on the three `emitAccessLog*` helpers + `ResolveCustomTags`, at all 18 callers — `nil` at exactly the 3 no-chain-404 sites (`connection.go`/`h2dispatch.go:...`/`h3dispatch.go:130`, where `traceDecision==nil` so `ResolveCustomTags` is provably never reached), the closure at the 15 span-capable sites. A minor seam sub-note FOLDED into ADR-0293 (no separate seam ADR — the phase-62/63/70 precedent). SPEC RE-DERIVES the exact caller count + the 3 nil sites at tip (the phase-70 count was 18 = 5+6+7; a PLAN is not evidence — re-derive).

### 2.6 Fixture posture: anticipated ONE new fixture (OTLP); a STATIC-route-metadata positive arm + a `default_value` arm — NO writer needed *(self-answered direction; SPEC confirms D-CTMR-FIXTURE)*

A ROUTE metadata custom tag is an OBSERVABLE span attribute, so it IS differential-provable. Per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch) the anticipated posture is a NEW `test/fixtures/0115-tracing-custom-tags-metadata-route` dir (OTLP-provider, asserting the ROUTE metadata tag on the OTLP span via the `test/helpers/otlptrace` receiver). **Unlike phase 70, NO runtime writer is needed** — the source is the matched route's STATIC config metadata, so the fixture configures a route with `metadata: {filter_metadata: {ns: {key: "value"}}}` + a `custom_tag{tag, metadata{kind: ROUTE, metadata_key: {key: ns, path: [{key: "key"}]}}}`. Both sides read the identical static route config ⇒ cross-side EXACT key+value, deterministic. Plus a `default_value` arm (a tag whose namespace/path is absent from the route metadata → the `default_value` string emitted, deterministic). Whether a SECOND Zipkin dir is warranted is D-CTMR-FIXTURE (anticipated NO — the shared `Span.Attrs` seam + unit tests prove both exporters, the 59/62/63/70 precedent). Anticipated: fixtures **116 → 117**.

### 2.7 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-CTMR-FUZZSEED)*

The tracing config is parsed via the HCM config path, fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:28`). The new ROUTE accept + the two remaining reject arms are exercised by extending the phase-70 `withMetaTags` seed (add an accepted `ROUTE` metadata tag + confirm `CLUSTER`/`HOST` still reject) — NOT a new fuzzer. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented total against actual `^func Fuzz` before AND after). SPEC confirms D-CTMR-FUZZSEED (and dispatch-verifies the HCM fuzzer reaches the `kindMetadataRoute` accept arm).

### 2.8 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. Anticipated stat surface **1201 (+0)**, UNCHANGED. (Point of contrast with the `ssl`-stat-family rival, which would be +N — §2.1.)

---

## 3. Framework-survey result — a reject-lift + a resolve source on landed engines; ZERO new packages/modules (71 anticipated)

### 3.1 Framework: a small seam at most (a `kindMetadataRoute` accept arm + a `routeMetaLookup` parameter)

No new interface, no new package-level type beyond a `kindMetadataRoute` constant (reusing the existing `CustomTagSpec` metadata fields) and a new nil-tolerant `routeMetaLookup` parameter on `ResolveCustomTags` + the three `emitAccessLog*` helpers. The `RouteMetadata()` accessor is pre-existing. Every other symbol (`descend`, `structpbValueToString`, the metadata spec fields) is pre-existing/reused.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. `internal/dynamicmetadata` is NOT touched (ROUTE reads route config, not the Bucket). No new package.

### 3.3 go.mod modules: NONE

`type.tracing.v3.CustomTag_Metadata` + `type.metadata.v3.{MetadataKind,MetadataKey}` are already reachable (consumed by the phase-70 REQUEST arm). `config/core/v3.Metadata` (`corev3`) is already imported by `internal/filter/http/chain.go` (the `RouteMetadata()` return type) and everywhere in `internal/filter/hcm`. `google.golang.org/protobuf/types/known/structpb` is already imported by `internal/tracing/resolve.go` (the phase-70 serializer). `go mod tidy -diff` anticipated EMPTY; go.mod modules stay **2** (the tracked lineage figure).

### 3.4 REUSES

- **phase-46** the tracing engine: `Span`/`Span.Attrs []KV`/`BuildServerSpan`, the OTLP + Zipkin exporters (both consume `Attrs`), the `test/helpers/otlptrace` receiver.
- **phase-59/62/63** the `custom_tags` pipeline: `parseCustomTags` (the parse home + first-wins dedup) and `ResolveCustomTags` (the resolve home).
- **phase-70** the `metadata`-kind machinery: `descend` (`resolve.go:100`) + `structpbValueToString` (`resolve.go:123`) — reused VERBATIM; the `CustomTagSpec.MetaNamespace`/`MetaPath`/`DefaultValue`/`HasDefault` fields; the four-kind reject switch (lift the ROUTE arm); the `metaLookup`-threading pattern across 18 callers (mirror it for `routeMetaLookup`).
- **phase-24.2 / ADR-0165** the `FilterChain.RouteMetadata() *corev3.Metadata` accessor (`chain.go:1156`) + `SetRouteMetadata` (seeded at the three dispatch sites) as the `ROUTE`-kind source.
- **`FuzzHCMConfigParse`** as the fuzz host (a seed extension, no new fuzzer).

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config (NOT bootstrap `stats_sinks[]`)

Like the phase-59/62/63/70 `custom_tags` rows, `custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `parseCustomTags` from `HttpConnectionManager.tracing.custom_tags`. No bootstrap change. The fixture configures a `metadata`/`ROUTE` `custom_tag` on the listener's HCM + `metadata` on a RouteConfiguration route.

---

## 5. Stat surface hypothesis — +0 (71)

A span attribute registers no stat. Anticipated stat surface **1201 (+0)**.

---

## 6. Anticipated edit sites (SPEC RE-DERIVES each at tip — a BRAINSTORM cite is not evidence, `feedback_brief_citations_not_evidence`)

- `internal/tracing/config.go:245-246` — LIFT the ROUTE reject → an accept arm cloning the REQUEST accept (`:228-244`), building `CustomTagSpec{Kind: kindMetadataRoute, MetaNamespace: mk.GetKey(), MetaPath: path, DefaultValue: dv, HasDefault: dv != ""}`; `CLUSTER`/`HOST` rejects (`:247-250`) UNCHANGED.
- `internal/tracing/config.go:57` — a new `kindMetadataRoute` `customTagKind` constant beside `kindMetadata`.
- `internal/tracing/resolve.go:26` — a new nil-tolerant `routeMetaLookup func(ns string) (*structpb.Value, bool)` parameter on `ResolveCustomTags`.
- `internal/tracing/resolve.go` (a new arm beside `case kindMetadata:` at `:65`) — `case kindMetadataRoute:` descending the FULL `MetaPath` from `routeMetaLookup(s.MetaNamespace)`, reusing `descend` + `structpbValueToString`, same default rule.
- `internal/filter/hcm/accesslog_emit.go:27/:87/:149` — the `routeMetaLookup` param on the three `emitAccessLog*` helpers; passed to `ResolveCustomTags` at `:57/:118/:179`.
- `internal/filter/hcm/{connection.go,h2dispatch.go,h3dispatch.go}` — the 18 emit callers: a `chain.RouteMetadata`-derived closure at the 15 span-capable sites, `nil` at the 3 no-chain-404 sites (RE-DERIVE the exact count + sites at the SPEC).
- `internal/tracing/{config_test.go,resolve_test.go}` — the ROUTE accept + reject + resolve/path-walk/default tests.
- `internal/filter/hcm/{accesslog_emit_test.go,span_emit_test.go}` — the `routeMetaLookup` threading + a live ROUTE-metadata-span test.
- `internal/filter/hcm/fuzz_test.go` — extend the `withMetaTags` seed with a ROUTE tag.
- `test/fixtures/0115-tracing-custom-tags-metadata-route/` — the new OTLP fixture (static-route-metadata positive arm + `default_value` arm).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the ROUTE-kind tracing edit.
- `docs/envoy-go/DECISIONS.md` — ADR-0293 (§Context at the SPEC, §Decision + §Consequences at the IMPL IN PLACE).
- `docs/envoy-go/ROADMAP.md` — the deferred-sentence narrow (the ROUTE candidate rolls OUT at the IMPL row-done edit, phase-57 precedent).

---

## 7. BRAINSTORM-time open questions to the SPEC (the D-CTMR-* docket)

- **D-CTMR-SCOPE** — `ROUTE` MetadataKind only; `CLUSTER`/`HOST` stay reject-only. (Anticipated confirmed.)
- **D-CTMR-PROTO** — the go-control-plane `CustomTag_Metadata`/`MetadataKind`/`MetadataKey` field shapes at the pin (RE-DERIVE; phase 70 confirmed the kind-getters live on `*metadatav3.MetadataKind`, `k.GetRoute()`).
- **D-CTMR-WIRE / D-CTMR-ZIPKIN-WIRE** — the reference emits `{key=literal tag name, value=serialized string}` on OTLP + in the Zipkin tags map (anticipated identical to REQUEST; probe confirms).
- **D-CTMR-VALUE-SERIALIZE** — the `structpb.Value`→string table matches phase-70 P3 (anticipated; probe confirms shared machinery).
- **D-CTMR-DEFAULT** — the default/omit rule matches phase-70's `request_header` rule (anticipated present-empty EMITS `""`; probe confirms).
- **D-CTMR-RESOLVE-SEAM** — the source-shape asymmetry (§2.3): `routeMetaLookup(ns)` returns the namespace struct wrapped as a `StructValue`; the arm descends the FULL `MetaPath` (not `[1:]`). Pin the exact param shape (symmetric second param vs combined closure).
- **D-CTMR-CONFIG-KIND** — `kindMetadataRoute` constant vs a `MetaSource` sub-enum (anticipated `kindMetadataRoute`).
- **D-CTMR-BUCKET-SEAM** (route variant) — the `routeMetaLookup` threading across the 18 callers; the 3 nil sites (RE-DERIVE count).
- **D-CTMR-REJECT** — the accept arm's structural edge rejects (empty ns/path/segment PGV-parity); the reference ACCEPTS ROUTE / BOOTS CLUSTER+HOST (probe).
- **D-CTMR-FIXTURE / D-CTMR-FIXTURE-SOURCE** — ONE new OTLP fixture, static-route-metadata (NO writer); Zipkin dir anticipated NO.
- **D-CTMR-FUZZSEED** — extend the `withMetaTags` seed (+0 fuzzers).
- **D-CTMR-SPLIT** — SINGLE FLAT ROW anticipated (~8–10 tasks; ADR-0045 armable).

---

## 8. What phase 71 does NOT deliver (forward)

- The `CLUSTER` MetadataKind (the `upstream_cluster` framework gap — `reference_tracing_upstream_cluster_framework_gap`; needs the selected-cluster metadata plumbed to the emit seam).
- The `HOST` MetadataKind (only the reduced `envoy.lb` scalar subset in scope at the seam; needs the full `*corev3.Metadata` for the picked host plumbed).
- `spawn_upstream_span` / `http_service` / force-trace (§2.1).
- The `ssl` handshake-outcome stat family (ADR-0286 C3).
- All xDS / HTTP-3 / never-opened-family candidates (§2.1).

---

## 9. ADR-0045 split readiness + ADR roster

Anticipated a SINGLE FLAT ROW (ADR-0045 valve armable-but-unconsumed). ADR-0293 (§Context drafted at the SPEC per ADR-0044; the DECISIONS tail flips ADR-0292 → ADR-0293 AT the SPEC commit; §Decision + §Consequences appended IN PLACE at the IMPL, no renumber; next-free ADR-0294). No new ADR beyond ADR-0293 anticipated (the seam sub-note folds into it, the phase-62/63/70 precedent).

---

## 10. Envelope + counts (anticipated at the phase-71 IMPL; docs-only at this BRAINSTORM)

- ZERO new production packages (edits in `internal/tracing` + `internal/filter/hcm`, both existing).
- +0 stats (a span attribute) → stat surface **1201 (+0)**.
- +0 fuzzers (a SEED extension to `FuzzHCMConfigParse`) → **55**.
- +0 BackendKinds → **38** (`0115` reuses `HTTPFixedBody`; the `otlptrace` receiver is driver-owned).
- +0 go.mod modules → **2** (`corev3`/`metadatav3`/`structpb`/`protojson` all already imported).
- fixtures **116 → 117** (`0115-tracing-custom-tags-metadata-route`, OTLP).
- DECISIONS tail **ADR-0292 → ADR-0293** (at the SPEC; next-free ADR-0294).

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-run MECHANICALLY in the worktree at close):** fixtures **116** (numeric tail `0114-tracing-custom-tags-metadata`) / fuzzers **55** / stat **1201** / BackendKind **38** / DECISIONS tail **ADR-0292** (next-free **ADR-0293**) / go.mod modules **2**.

---

## 11. Sized-against-source — the cost derivations (two independent read-only agents at tip `e2912f6f`)

Two read-only cost-assessment agents RE-DERIVED each candidate's size against source THIS session (NOT trusting the phase-70 deferral adjectives — `reference_deferred_candidate_cost_restale`). Findings:

- **`metadata`/`ROUTE` (the pick) — SMALL.** Source `FilterChain.RouteMetadata() *corev3.Metadata` (`chain.go:1156`) is LANDED (phase 24.2) and reachable at the emit call sites (which already thread `chain.DynamicMetadata().Get`). The resolve machinery (`descend`/`structpbValueToString`) landed LAST SESSION and is reused verbatim. A near-exact clone of the phase-70 seam: lift one reject arm, add one resolve arm (reusing the serializer, descending the FULL path), thread one more nil-tolerant closure. +0 stats/pkgs/modules. **The phase-70 "not exposed at the emit seam" adjective was STALE for ROUTE** (the discriminating finding).
- **`metadata`/`CLUSTER` — LARGE.** True framework gap (`upstream_cluster` unplumbed at the emit seam; `accesslog_emit.go` hard-codes `UpstreamCluster: ""`).
- **`metadata`/`HOST` — MODERATE.** Only the reduced `envoy.lb` scalar subset in scope; full parity needs the picked host's full `*corev3.Metadata`.
- **reconnection-backoff (xDS) — SMALL but THIN.** `initial_fetch_timeout` already done; initial-fetch-only substrate ⇒ "little to reconnect to"; least capability gained.
- **upstream SDS (xDS) — MODERATE.** Reuses the SDS machinery whole but threads a provider into `internal/cluster` across the `grpcclient→cluster→tls` cycle seam.
- **`http_service` (tracing) — MODERATE.** Clean transport swap behind the landed `TracesClient` seam; new HTTP-protobuf transport. (The phase-70 reject-line cite `config.go:236-237` is now STALE → `274-275`.)
- **`spawn_upstream_span` — MODERATE-LARGE.** New CLIENT-span model + upstream-timing seam.
- **force-trace — LARGEST.** Unbuilt internal-request-detection + edge-sanitization subsystem.
- **SDS rotation (xDS) — LARGE.** New mutable-cert seam + running-subsystem lifecycle.

**Two stale phase-70 costs corrected at tip:** (1) the blanket "ROUTE/CLUSTER/HOST metadata not exposed at the emit seam" — ROUTE IS exposed (`chain.RouteMetadata()`); (2) the `http_service` reject line ref (`config.go:236-237` → `274-275`).

`metadata`/`ROUTE` is the smallest row that reuses the proven, freshest substrate whole and adds a genuine differential-provable capability — smaller than every xDS candidate that gains real capability, and lower-risk than the raw-smaller-but-thinner reconnection-backoff.

---

## 12. Stage-close mechanics (this BRAINSTORM)

- Row 71 registered `in-progress` (ROADMAP §Schema).
- NO deferred-sentence narrow at the BRAINSTORM (the phase-57 precedent — the ROUTE candidate is NAMED in the live Observability `candidates:` sentence; it rolls OUT at the phase-71 IMPL row-done edit).
- DECISIONS UNTOUCHED (ADR-0293 §Context drafts at the SPEC per ADR-0044).
- STATE §Current rolled IN PLACE (lifecycle DONE → 1; lineage re-capped at five).
- next-prompt.txt rolled to the phase-71 SPEC.
- Sentinel re-run MECHANICALLY at close (does NOT fire; `stop` NOT created): (1) prints `NOT DONE: row 71` (the registration re-opens it); (2) STILL prints **3** (a named pickup does not drop a `candidates:` sentence — it narrows at the IMPL); (3) `NEVER OPENED: gRPC/Runtime/WASM`.
- **Next → the phase-71 SPEC** (the D-CTMR-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2`; re-derive every §6 edit site + the go-control-plane metadata proto types; draft ADR-0293 §Context).
