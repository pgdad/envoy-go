# Phase 70 Brainstorm — tracing `custom_tags` (METADATA tag type, `REQUEST` MetadataKind only) (the SIXTEENTH Observability-family row; the FOURTH tracing `custom_tags` row and the LAST `CustomTag` source type; lifts the `custom_tags metadata type unsupported` reject and resolves a `REQUEST`-kind dynamic-metadata value onto the span over the LANDED `internal/dynamicmetadata.Bucket`; the `ROUTE`/`CLUSTER`/`HOST` MetadataKinds stay PARSE-REJECTED loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree `.worktrees/phase-70-brainstorm`, branch `phase-70-tracing-custom-tags-metadata-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 69 (`stats-sink-otlp`) landed COMPLETE (row 69 `done`, ADR-0291; every chartered ROADMAP row is now `done`). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at session start and does NOT fire — check (1) is SILENT (every row `done`), but check (2) prints THREE live `candidates:` sentences (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:'` ⇒ **3** — HTTP/3 + xDS + Observability) and check (3) prints `NEVER OPENED: gRPC/Runtime/WASM`; each independently BLOCKS `stop`. So the roller SELF-PICKED the next subject (§2.1): the **smallest defensible ROW-SIZED candidate** — tracing `custom_tags`, scoped to the **`metadata`** tag type with the **`REQUEST`** MetadataKind only — over a fully-adjudicated rejected roster (recorded §2.1, each candidate RE-DERIVED against source THIS session). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `33ed4c05` (the phase-69 IMPL squash + its docs router-roll):** stat surface **1201** · fixtures **115** (tail `0113-stats-sink-otlp-knobs`) · fuzzers **55** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0291** (COMPLETE; next-free **ADR-0292**) · new Go packages **0** · new go.mod modules **2** (the tracked `quic-go` + `qpack` lineage figure, unchanged). Counts are UNCHANGED at a BRAINSTORM (docs-only). **CANDIDATE COST WAS RE-DERIVED AT TIP, NOT INHERITED (`reference_deferred_candidate_cost_restale`): the roller's FIRST pick — the `environment` custom_tag type — was DISPROVEN by a direct read of `internal/tracing/config.go:208-213`, which showed `environment` (AND `request_header`) ALREADY LANDED (phase 62/63); the sole remaining `CustomTag` type is `metadata`.** All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (70 — the FOURTH tracing `custom_tags` row; a `metadata`/`REQUEST`-kind slice on the landed dynamic-metadata Bucket)

### 1.1 What phase 70 delivers as a self-contained whole (a dynamic-metadata value on the ingress span)

The HCM `tracing` `custom_tags` parser today STRICT-REJECTS the `metadata` tag type:

```go
// internal/tracing/config.go:214-215 (re-derived against master 33ed4c05)
case ct.GetMetadata() != nil:
    return nil, fmt.Errorf("tracing: custom_tags metadata type unsupported")
```

Phase 70 **lifts that reject** and supports the **`metadata`** `CustomTag` type scoped to the **`REQUEST`** `MetadataKind` — an operator-configured `{tag, metadata: {kind: REQUEST, metadata_key: {key: <filter-namespace>, path: [<segment>...]}, default_value: <fallback>}}` resolves the named value out of the request's **dynamic filter metadata** (envoy-go's landed `internal/dynamicmetadata.Bucket`, `dynamicmetadata.go:33` `Get(filterName, key)`) and emits it as a STRING span attribute on the ingress SERVER span, on **both** the OTLP and the Zipkin exporter (the phase-46 `Span.Attrs []KV` seam, consumed by both — §2.3). The other three `MetadataKind`s (`ROUTE`/`CLUSTER`/`HOST`) stay **PARSE-REJECTED loudly**, each with its own distinct substring (envoy-go-strict, ADR-0080 — §2.4).

This is the LAST `CustomTag` source type (after `literal` @ phase 59, `request_header` @ phase 62, `environment` @ phase 63); phase 70 completes the `custom_tags` feature surface for the `REQUEST`-kind common case. The delivery is a complete, differentially-provable slice: a Lua filter (or any dynamic-metadata writer both sides run) sets `md[ns][key] = "v"`; a `metadata` custom tag reading `{key: ns, path: [key]}` puts `{key: tag, value: "v"}` on every sampled span, cross-side; and a metadata-absent config emits the `default_value` (or omits — §2.3), fully deterministically (§2.6).

### 1.2 What phase 70 does NOT deliver (forward to §8)

NO `ROUTE`/`CLUSTER`/`HOST` MetadataKinds (deferred; rejected loudly — §2.4; their per-object metadata is not exposed at the span-emit seam, and only the `REQUEST` dynamic-metadata `Bucket` is landed — §3.4/§8). NO other tracing follow-on (`spawn_upstream_span` / `http_service` / force-trace — each larger, §2.1/§8; all three are un-honored loud-rejects today — §11-verified). NO `max_path_tag_length` change (a distinct, orthogonal knob already consumed at `config.go`; untouched). NO OTLP-metrics work (phase 69 landed that sink). NO new provider, transport, or stat. The four built-in span attributes currently emitted EMPTY (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — a `REQUEST`-kind metadata tag is populated from the per-request dynamic-metadata `Bucket`, which is a DIFFERENT, landed seam (§11).

### 1.3 Phase-done as the SIXTEENTH Observability-family row landing (family STAYS OPEN)

Row 70 is the SIXTEENTH Observability-family row and the FOURTH tracing `custom_tags` row. After phase 70 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (the `ssl` handshake-outcome stat family + `spawn_upstream_span`/`http_service`/force-trace + the three non-`REQUEST` MetadataKinds), so the sentinel check-(2) still prints ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered, SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~10–13 tasks — the parse accept/reject arms + the `Bucket`-resolve source in `ResolveCustomTags` + the MetadataKey path-walk + the value→string serialization + the three-seam `Bucket` threading + the fixture(s) + the fuzz seed + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0292), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse and the resolve sit on the SAME tracing engine (`internal/tracing`), the `Bucket` is a LANDED read-only substrate (`internal/dynamicmetadata`), and the reject-the-three-other-kinds arms live in the same parse function. The escape valve is documented ARMABLE and re-armed only if the SPEC's task count surprises upward (e.g. if the MetadataKey nested-path walk + the `structpb.Value`→string serialization + a SEPARATE Zipkin fixture push the count past ~15 — SPEC weighs it, §2.5/§2.6). This is the largest `custom_tags` slice of the four (it consumes a NEW per-request source rather than reusing headers), but it is still a bounded single-subsystem row.

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files, ZERO new packages

- Production parse accept + three reject arms: `internal/tracing/config.go` `parseCustomTags` (existing, `:182`; the metadata reject `:214-215` becomes an accept-`REQUEST` + reject-`ROUTE`/`CLUSTER`/`HOST` arm).
- Parsed-config home: extend the EXISTING `CustomTagSpec` struct (`config.go:63`) with a `kindMetadata` (`config.go:53-55` kind constants) + the metadata fields (namespace, path segments, default). NO new type at package scope beyond struct fields.
- Resolve source: `internal/tracing/resolve.go` `ResolveCustomTags` (existing, `:14`) gains a `kindMetadata` arm reading a NEW metadata-lookup input (the `Bucket` accessor) + the MetadataKey path-walk + `structpb.Value`→string.
- Call-site threading: `internal/filter/hcm/accesslog_emit.go` (existing) — the three `ResolveCustomTags(...)` call sites (`:55` H1, `:116` H2, `:177` H3) thread the per-request dynamic-metadata `Bucket` (reachable via the per-request `chain` built per-protocol at `connection.go:350` (H1) / `h2dispatch.go:327` (H2) / `h3dispatch.go:148` (H3) — SPEC pins the exact accessor, §2.5).
- Fuzz SEED: `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` (existing, `:28`) — a NEW seed (an accepted `REQUEST` metadata tag + a rejected `ROUTE`/`CLUSTER`/`HOST` kind), NOT a new fuzzer (§2.7).
- Fixture(s): anticipated ONE new `test/fixtures/NNNN-tracing-custom-tags-metadata` dir (OTLP-provider; a Lua-set dynamic-metadata positive arm + a metadata-absent `default_value` arm) — SPEC decides via the dispatch constraint (§2.6).
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing section) + ROADMAP/STATE/DECISIONS.

Possibly ONE new `.go` file only if the SPEC decides the MetadataKey path-walk / value-serialization warrants its own helper file (unlikely — it fits `resolve.go` beside the existing arms). ZERO new packages, ZERO new modules (`structpb` is already an `internal/dynamicmetadata` dependency — §3.3).

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject (`git branch -a | grep -iE 'custom.?tag|metadata|tracing'` ⇒ none). The `custom_tags` `metadata` type is a recorded deferred candidate in the ROADMAP Observability family's LIVE deferred sentence (`tracing custom_tags (`metadata`)/…`), not a stashed WIP.

### 1.7 Phase 70's relationship to the existing seams (a parse arm + a resolve source on landed engines)

The landed engines already expose the two seams this row needs plus the source it reads: (a) `parseCustomTags` (`config.go:182`), the `custom_tags` → `[]CustomTagSpec` parse, where the metadata reject lives and the accept-`REQUEST` + three reject arms land; (b) `ResolveCustomTags` (`resolve.go:14`), the per-request `[]CustomTagSpec` → `[]KV` resolve consumed by BOTH exporters via `Span.Attrs`; and (c) `internal/dynamicmetadata.Bucket` (`dynamicmetadata.go:15`/`:33`), the LANDED per-request dynamic-metadata store (`Get(filterName, key) (*structpb.Value, bool)`), WRITTEN in production by the Lua filter (`internal/filter/http/lua/metadata.go:315` `Bucket.Set`) and the mongo-proxy network filter (`internal/filter/network/mongoproxy/filter.go:176`), and exposed on the per-request HTTP chain callbacks (`internal/filter/http/chain.go:837`/`:963` `DynamicMetadata() *Bucket`). The ONLY novel production code is the metadata accept/reject arms in `config.go`, the `kindMetadata` resolve arm + MetadataKey path-walk + value-serialization in `resolve.go`, and the `Bucket` threading at the three `accesslog_emit.go` seams; everything else is doc/test/fixture reconciliation.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` (`metadata`/`REQUEST` only) *(SELF-PICKED per the standing directive → phase 70 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest defensible ROW-SIZED candidate** after INVESTIGATING each candidate's size against source THIS session (§11). Row 70 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `custom_tags` (`metadata`/`REQUEST`) is smallest-defensible:** it sits on landed substrate on THREE axes — the `parseCustomTags`/`ResolveCustomTags`/`Span.Attrs` custom-tags pipeline (phase 59/62/63), the `internal/dynamicmetadata.Bucket` read API (`Get`), and a cross-side-deterministic writer (the Lua filter, present in both envoy-go and the reference contrib image). It completes the `custom_tags` feature (the LAST source type), has a clean landed precedent to clone (phase 63 `environment` is the most recent `CustomTagSpec`-extending row), carries NO ADR headwind, and is **+0 stat** (a span attribute, not a stat — unlike the `ssl` rival). Scoping to the `REQUEST` kind keeps it a single flat row: that is the one MetadataKind whose source (`Bucket`) is landed.

**Rejected alternatives (recorded per the standing directive; each RE-DERIVED/SIZED against source this session — §11):**
- **`ssl.*` downstream handshake-outcome stat family** (the other live Observability candidate, ADR-0286 C3) — a SUCCESS-PATH SUBSET (`ssl.handshake` / `ssl.ciphers.*` / `ssl.versions.*`) is near-inline (`stdtls.Conn.ConnectionState()` after `HandshakeContext` at `internal/listener/manager.go:1174-1184` yields Version/CipherSuite; a per-listener stats scope already exists, `registerListenerMetrics` `manager.go:351-354`). BUT (a) **ADR-0286 C3 explicitly frames the `ssl` family as "a framework-surgery row of its own"** that "would blow the +0-stat envelope" — a documented boundary, so a shrunk subset invites the "shrinking-by-adjective what the ADR said isn't shrinkable" objection; (b) it OPENS A NEW STAT SURFACE (the long-standing +0-stat streak breaks; the dynamic per-cipher/per-version counter names need `stats.IsValidName` charset validation, `reference_dynamic_stat_name_charset_guard`); (c) the DISCRIMINATING part — `ssl.fail_verify_error` vs `ssl.fail_verify_no_cert` — needs classifying an OPAQUE Go `crypto/tls` handshake error, i.e. a `VerifyPeerCertificate`/`GetConfigForClient` callback wired into every per-chain `*stdtls.Config` (and `GetConfigForClient` was removed at 07.2, `manager.go:135-141`). MEDIUM→LARGE with an ADR headwind. Deferred; metadata is cleaner (lineage precedent, no ADR headwind, +0 stat).
- **`spawn_upstream_span`** — envoy-go emits ONE ingress SERVER span; this adds a second CLIENT span for the upstream leg with its own timing wired at the router/upstream seam. Touches the span model; medium-large. Deferred.
- **`http_service` (OTLP-over-HTTP tracer transport)** — a new HTTP exporter transport alongside the `envoy_grpc` OTLP path (rejected today at `config.go:236-237`). Reuses buffer/flush but a new transport + protobuf-over-HTTP encoding. Medium. Deferred.
- **force-trace (`x-envoy-force-trace`)** — un-honored today (`internal/tracing/stats.go:26` comment confirms; only `x-client-trace-id` force-tracing is implemented, `decision.go:66-100`). Honoring it needs internal-request detection + edge sanitization, which envoy-go has no concept of — a whole new subsystem. HIGH scope-risk. Deferred.
- **A SIXTH stats sink (`envoy.stat_sinks.hystrix` / `wasm`)** — hystrix is a per-cluster rolling-window SSE stream served over the ADMIN HTTP endpoint, NOT a periodic push of a registry snapshot, so it does not fit the landed `Flusher`/`Sink.Submit(batch)` model (`internal/statssink/sink.go:18-21`) — admin-streaming surgery; wasm needs the wasm runtime. LARGE/BLOCKED — no cheap 6th sink remains on the landed substrate. Deferred.
- **RBAC principal `require=false` fixture arm** (the roster's named "best RIDER") — the RBAC engine is COMPLETE (`internal/rbac/` principals fully built + consumed by both HTTP and network RBAC filters); this is a TEST-FIXTURE-ONLY rider with ZERO engine work ⇒ **rider-sized, NOT a standalone row-sized subject**. It may attach to a future RBAC-touching row (≤1 task); it does not open one. Deferred as not-row-sized.
- **DataSource `environment_variable`** — a ~1-line `os.Getenv` per site (`internal/tls/datasource.go:36-37` + `internal/xds/secret.go:44-45` both explicitly error today), but BLOCKED by the SPEC-63 harness-seam deferral decision (D-ENV-HARNESS) — a deliberate scope boundary, not a missing capability. Deferred (blocked-by-decision).
- **`watched_directory` / SDS cert rotation** — the `fsnotify` watcher substrate exists (`internal/sdsfile`), but consuming a rotation event needs live per-chain `*stdtls.Config` reconfig, which is UNBUILT (initial-fetch-only provider; `GetConfigForClient` removed at 07.2). MEDIUM→LARGE multi-leg live-reconfig row. Deferred. Sibling: **upstream SDS** stays a VALUE-level cycle BLOCKED (`reference_xds_config_seam_transitive_cycle_guard` passes it).
- **HTTP/3 candidates** (upstream H3 cluster / alt-svc / 0-RTT / h3spec gate / QuicProtocolOptions / full QUIC transport-socket options / QUIC robustness) — each needs new substrate (H3 cluster/pool, an alt-svc engine) or has near-zero differential value (QuicProtocolOptions). Deferred.
- **xDS families** (SDS rotation, CDS/EDS/LDS/RDS/ADS/Delta-xDS/RTDS/`google_grpc`) — each needs the live-reconfig substrate or is family-opening-sized. Deferred.
- **Opening a NEVER-OPENED family** (gRPC / Runtime / WASM) — the standing directive says smallest-defensible-first, and the Observability tail STILL holds a cheap ROW-SIZED candidate (`metadata`, the last custom_tag type), so smallest-first keeps us in Observability. Deferred; revisit when the Observability tail's cheap candidates are drained.

### 2.2 Scope: the `metadata` type, `REQUEST` MetadataKind ONLY; the other three kinds PARSE-REJECT loudly *(self-answered; the incremental-arm precedent)*

The `CustomTag.Metadata` proto (`type.tracing.v3.CustomTag_Metadata`, go-control-plane/envoy v1.32.4 — field SHAPE RE-DERIVED from the module this session; exact go-control-plane line numbers are SPEC-re-derived at the pin, not asserted here) is `kind` (field 1, `*type.metadata.v3.MetadataKind` — a oneof of `Request_`/`Route_`/`Cluster_`/`Host_` empty messages) + `metadata_key` (field 2, `*type.metadata.v3.MetadataKey{Key string; Path []*MetadataKey_PathSegment}`) + `default_value` (field 3, string). Phase 70 supports ONLY `kind == REQUEST` (the request-scoped dynamic filter metadata, envoy-go's `Bucket`); `ROUTE`/`CLUSTER`/`HOST` REJECT loudly with distinct substrings (§2.4). This mirrors the project's landed incremental-arm posture (the OTel-`envoy_grpc`-transport-only reject of `google_grpc`/`http_service`; the Zipkin-`HTTP_JSON`-only reject; the `custom_tags` types themselves landed one at a time — literal/59, request_header/62, environment/63). A `REQUEST`-kind slice is a complete, useful, differential-provable capability (a tag carrying an ext-authz/lua-emitted dynamic-metadata value onto the span). The SPEC probe confirms the reference emits it identically (D-CTM-WIRE).

### 2.3 The resolve seam: a NEW `kindMetadata` arm in `ResolveCustomTags` reading the `Bucket`; append to `Span.Attrs` — ONE change, TWO providers *(self-answered; the KV seam is landed)*

`ResolveCustomTags(specs, headerLookup) []KV` (`resolve.go:14`) resolves the ordered, first-wins-deduped specs into `[]KV`, which `BuildServerSpan` upserts into `Span.Attrs` (consumed by BOTH the OTLP `toProto` and the Zipkin encoder — the phase-46 seam). Phase 70 adds a `kindMetadata` arm to `ResolveCustomTags`, resolving `{namespace, path, default}` against a NEW per-request metadata source (the `Bucket`, threaded from the call sites — §2.5): `Get(namespace, path[0]) → *structpb.Value`, then WALK the remaining `path[1:]` segments into that `Value` (nested `StructValue` field access), then SERIALIZE the resolved `Value` to a string. Metadata tags are STRING-valued (`KV{Key: tag, Str: <serialized>}`), like the other three types. Two behavioral sub-questions the SPEC probes:
- **D-CTM-VALUE-SERIALIZE** — how the reference serializes a resolved `structpb.Value` to the span-tag string: a bare string Value → its raw string; a number/bool/struct/list Value → the reference's canonical form (likely the `ValueToString` / JSON-ish rendering). The fixture's positive arm SHOULD use a STRING metadata value (the simplest, unambiguous case) so the row does not hinge on serialization edge cases; the SPEC pins the string case and NAMES non-string serialization as a documented boundary if it is non-trivial.
- **D-CTM-DEFAULT** — the `default_value` / omit semantics when the path is absent or the resolved value is empty. Anticipated: absent path → emit `default_value` if non-empty, else OMIT (the `environment`-type omit-on-empty-resolved precedent, `resolve.go:36-52`); the SPEC probes whether the reference omits or emits an empty tag, and whether a present-but-empty metadata value differs from an absent one.

### 2.4 The three unsupported MetadataKinds reject loudly with DISTINCT substrings — an envoy-go-strict DEPARTURE, not a parity claim *(self-answered; ADR-0080)*

The reference SUPPORTS all four MetadataKinds; envoy-go rejecting three of them is a documented envoy-go-strict DEPARTURE (like the three landed `custom_tags`-type rejects that preceded each type's support). Each reject carries its own distinct substring (ADR-0080 anti-silent-divergence), anticipated:
- `tracing: custom_tags metadata route kind unsupported`
- `tracing: custom_tags metadata cluster kind unsupported`
- `tracing: custom_tags metadata host kind unsupported`
Plus the structural rejects RE-DERIVED at the SPEC: an absent/empty `metadata_key.key` (namespace), an empty `path`, an unset `kind` oneof, and the PGV-derived `CustomTag.tag` `min_len: 1` (already enforced at `config.go:190-192` for all types). The exact substrings + whether the reference PGV-boot-rejects any of these vs envoy-go's own reject are SPEC-pinned (D-CTM-REJECT). **D-CTM-REJECT** also confirms (one probe arm) the reference ACCEPTS a `ROUTE`/`CLUSTER`/`HOST` metadata tag — so the departure is real (the reference boots where envoy-go rejects), not a shared reject.

### 2.5 Config home + threading: extend `CustomTagSpec`; thread the `Bucket` to `ResolveCustomTags` at the three seams *(self-answered shape; SPEC pins the exact seam — D-CTM-CONFIG-SEAM / D-CTM-BUCKET-SEAM)*

The parsed metadata spec lives on the EXISTING `CustomTagSpec` (`config.go:63`) — a new `kindMetadata` (`config.go:53-55`) + the metadata fields (anticipated `MetaNamespace string`, `MetaPath []string`, reusing `DefaultValue`). Two seam decisions the SPEC pins:
- **D-CTM-CONFIG-SEAM** — the `CustomTagSpec` field shape for the namespace + path (`[]string` segments vs a small struct; the `MetadataKey_PathSegment` oneof is key-only in the tracing use, RE-DERIVE).
- **D-CTM-BUCKET-SEAM** — how the per-request dynamic-metadata `Bucket` reaches `ResolveCustomTags` at the three `accesslog_emit.go` call sites (`:55`/`:116`/`:177`). The `Bucket` is per-request state on the HTTP `chain` built per-protocol at `connection.go:350` (H1) / `h2dispatch.go:327` (H2) / `h3dispatch.go:148` (H3) (`chain.DynamicMetadata()` returns it via the callbacks accessor, `chain.go:837`/`:963`); the emit happens AFTER the chain runs, so the `Bucket` (or a `func(ns, key) (*structpb.Value, bool)` closure over it) must be in scope at the emit. The SPEC RE-DERIVES whether the `chain` is already reachable at the emit call or must be threaded through the emit helper's signature. Anticipated shape: `ResolveCustomTags(specs, headerLookup, metaLookup)` where `metaLookup` is nil-tolerant (nil ⇒ metadata specs use default/omit, mirroring `headerLookup`'s nil-tolerance) — keeps the three-type existing behavior byte-identical when no metadata tags are configured. A minor seam sub-note is anticipated FOLDED into ADR-0292 (no separate seam ADR — the phase-62/63 precedent).

### 2.6 Fixture posture: anticipated ONE new fixture (OTLP); a Lua-set positive arm + a `default_value` arm *(self-answered direction; SPEC confirms D-CTM-FIXTURE)*

A metadata custom tag is an OBSERVABLE span attribute, so it IS differential-provable. Per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch) the anticipated posture is a NEW `test/fixtures/NNNN-tracing-custom-tags-metadata` dir (OTLP-provider, asserting the metadata tag on the OTLP span via the `test/helpers/otlptrace` receiver), NOT a mutation of a baseline. The fixture needs a cross-side-deterministic dynamic-metadata WRITER for the POSITIVE arm: the **Lua filter** (`internal/filter/http/lua/metadata.go` — a script calling `streamInfo():dynamicMetadata():set(ns, key, "v")`, present in both envoy-go and `envoyproxy/envoy:contrib-v1.37.2`) is the anticipated source, writing a FIXED string value that a `metadata_key{key: ns, path: [key]}` tag reads. Plus a `default_value` arm (metadata absent → the `default_value` string emitted, deterministic, NO writer needed). Whether the Lua writer is cross-side wire-identical (the metadata namespace/value both sides see) and whether a SECOND Zipkin fixture is warranted are **D-CTM-FIXTURE** / **D-CTM-FIXTURE-SOURCE**; if a Zipkin dir or the Lua source materially grows the row, the SPEC may split the fixture leg (ADR-0045, §1.4). Anticipated: fixtures **115 → 116** (or **117** with a Zipkin dir) — SPEC pins.

### 2.7 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 55 → SPEC confirms D-CTM-FUZZSEED)*

The tracing config is parsed via the HCM config path, fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:28`) — there is NO dedicated `internal/tracing` config fuzzer. So the new metadata accept + three reject arms are exercised by adding a `custom_tags` SEED (an accepted `REQUEST` metadata tag + at least one rejected kind) to `FuzzHCMConfigParse` — NOT a new fuzzer. Fuzzer count STAYS **55** (`reference_fuzzer_count_docs_drift`: reconcile the documented total against actual `^func Fuzz` before AND after — the count must not move). SPEC confirms D-CTM-FUZZSEED (and dispatch-verifies the HCM fuzzer actually reaches the `parseCustomTags` metadata arm).

### 2.8 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. The existing HCM tracing counters and tracer counters are UNCHANGED. Anticipated stat surface **1201 (+0)**, UNCHANGED. No new registration path. (This is a POINT OF CONTRAST with the `ssl`-stat-family rival, which would be +N and break the streak — §2.1.)

---

## 3. Framework-survey result — a parse arm + a resolve source on landed engines; ZERO new packages/modules (70 anticipated)

### 3.1 Framework: a small seam at most (a `CustomTagSpec` extension + a `ResolveCustomTags` metadata-lookup parameter)

No new interface, no new package-level type beyond fields on the existing `CustomTagSpec` and a new nil-tolerant metadata-lookup parameter on `ResolveCustomTags`. The `Bucket` read API (`Get`) is pre-existing. Every other symbol is pre-existing.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. The `internal/dynamicmetadata` package is CONSUMED (its `Bucket.Get`), not modified. No new package.

### 3.3 go.mod modules: NONE

`type.tracing.v3.CustomTag_Metadata` + `type.metadata.v3.{MetadataKind,MetadataKey}` are already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (the same module the HCM tracing proto lives in). `google.golang.org/protobuf/types/known/structpb` is ALREADY imported by `internal/dynamicmetadata` (the `Bucket` stores `*structpb.Value`), so the value-serialization needs no new module. `go mod tidy -diff` anticipated EMPTY; go.mod modules stay **2** (the tracked lineage figure). (The `type/metadata/v3` package may need a named import in `config.go`/`resolve.go` — an EXISTING-module import, not a new module.)

### 3.4 REUSES

- **phase-46** the tracing engine: `Span`/`Span.Attrs []KV`/`BuildServerSpan` (the span-attribute seam), the OTLP + Zipkin exporters (both consume `Attrs`), the `test/helpers/otlptrace` receiver.
- **phase-59/62/63** the `custom_tags` pipeline: `parseCustomTags` (the parse home + the three landed types + the metadata reject to lift + first-wins dedup) and `ResolveCustomTags` (the resolve home).
- **the landed `internal/dynamicmetadata.Bucket`** (`Get`) as the `REQUEST`-kind source, and the Lua filter (`lua/metadata.go`) as the cross-side-deterministic fixture writer.
- **the incremental-reject precedent** (the three landed `custom_tags`-type rejects + the OTel/Zipkin rejects) as the template for the three unsupported-MetadataKind reject arms.
- **`FuzzHCMConfigParse`** as the fuzz host (a seed, no new fuzzer).

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config (NOT bootstrap `stats_sinks[]`)

Like the phase-59/62/63 `custom_tags` rows (and unlike the phase-47..69 stats-sink rows), `custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `parseCustomTags` from `HttpConnectionManager.tracing.custom_tags` when the HCM filter is built. No bootstrap change; the parse lands INSIDE the existing `parseCustomTags` (the same function that already rejects `metadata` wholesale). The fixture configures a `metadata` `custom_tag` + a Lua filter on the listener's HCM.

---

## 5. Stat surface hypothesis — +0 (70)

### 5.1 Stat names (SPEC confirms)

NONE. A span attribute registers no stat.

### 5.2 envoy-go-strict departure flags

The three unsupported MetadataKinds (`ROUTE`/`CLUSTER`/`HOST`) are a documented envoy-go-strict DEPARTURE (the reference supports them; envoy-go rejects loudly, §2.4) — the SAME posture as the pre-support `custom_tags`-type rejects. No new stat, no new flag; a parse-reject departure recorded in BEHAVIOR_CONTRACT.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-CTM-CONFIG-SEAM / D-CTM-BUCKET-SEAM / D-CTM-DOCSHAPE)

Each `file:line` RE-DERIVED against master `33ed4c05` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tracing/config.go`:**
1. **The metadata accept + three reject arms** — replace the wholesale metadata reject (`config.go:214-215`) with: on `ct.GetMetadata()`, switch on `md.GetKind()` — `REQUEST` → validate `metadata_key` (non-empty namespace + non-empty path) and build a `kindMetadata` spec; `ROUTE`/`CLUSTER`/`HOST` → reject with distinct substrings (§2.4); unset kind → reject. [EDIT/ADD]
2. **`CustomTagSpec` struct + kind constants** (`config.go:53-55`, `:63`) — add `kindMetadata` + the metadata fields (namespace, path, reuse `DefaultValue`). [EDIT]

**Production — `internal/tracing/resolve.go`:**
3. **`ResolveCustomTags`** (`resolve.go:14`) — add a `metaLookup` parameter (nil-tolerant) + a `kindMetadata` arm: `metaLookup(namespace, path[0])` → walk `path[1:]` → serialize `structpb.Value` → string; default/omit per D-CTM-DEFAULT. [EDIT — signature + one arm]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
4. **The three `ResolveCustomTags` call sites** (`:55` H1, `:116` H2, `:177` H3) — pass the per-request dynamic-metadata `Bucket` accessor (from the `chain`, D-CTM-BUCKET-SEAM). [EDIT — 3 sites]

**Test:**
5. **`internal/tracing/config_test.go`** — accept a `REQUEST` metadata tag; reject each of `ROUTE`/`CLUSTER`/`HOST` + empty-namespace + empty-path + unset-kind (distinct substrings). [ADD]
6. **`internal/tracing/resolve_test.go`** — a `REQUEST` metadata tag resolves a `Bucket` value (string case), the path-walk, the default/omit edges. [ADD]
7. **`internal/filter/hcm/fuzz_test.go`** `FuzzHCMConfigParse` — a `custom_tags` metadata SEED (accepted `REQUEST` + a rejected kind). [ADD — no new fuzzer]

**Fixture:**
8. **`test/fixtures/NNNN-tracing-custom-tags-metadata`** (new) — a `metadata` `custom_tag` on an OTLP-provider listener + a Lua filter setting dynamic metadata; assert the `{key, value}` attribute on both spans (positive arm) + a `default_value` arm. Possibly a second Zipkin dir (D-CTM-FIXTURE). [ADD]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
9. **the tracing `custom_tags` section** — flip `metadata` from "unsupported (rejected)" to "the `REQUEST` MetadataKind is consumed (resolved from request dynamic metadata, emitted as a span attribute on both exporters); `ROUTE`/`CLUSTER`/`HOST` reject loudly (envoy-go-strict departure)". SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
10. **ROADMAP** — row 70 `in-progress` at this BRAINSTORM (§Schema); the family prose gains a "phase 70 CHARTERED and BRAINSTORMED" sentence. The LIVE deferred sentence NARROWS `custom_tags (`metadata`)` at the phase-70 IMPL (NOT now — re-run the sentinel check-(2) grep after that edit, `reference_sentinel_deferred_sentence_live_vs_historical`, keeping EXACTLY ONE live "candidates:" match for Observability). [BRAINSTORM: row + prose; IMPL: deferred-list narrow]
11. **STATE.md** — active-phase header flips to phase 70 BRAINSTORM (this stage). [EDIT]
12. **DECISIONS.md** — ADR-0292 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-CTM-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-CTM-CONFIG-SEAM** + **D-CTM-BUCKET-SEAM**.

---

## 7. Anticipated ADRs — 1 at the phase-70 IMPL: ADR-0292 (tracing `custom_tags` metadata / `REQUEST`)

ADR-0292 (tracing `custom_tags` `metadata` type, `REQUEST` MetadataKind — lifting the metadata reject, the dynamic-metadata span-attribute resolve over the landed `Bucket`, the three-kind strict-reject departure). §Context drafted at the SPEC (the gap's provenance: the phase-59/62/63 lineage + the ROADMAP deferred sentence + the `Bucket` substrate), §Decision/§Consequences at the IMPL per ADR-0044. The DECISIONS tail flips **ADR-0291 → ADR-0292** at the phase-70 SPEC commit (next-free **ADR-0293** after). A minor seam sub-note (the `ResolveCustomTags` metadata parameter + the `Bucket` threading) is anticipated FOLDED into ADR-0292 (no separate seam ADR — the phase-62/63 precedent); the SPEC re-decides if it finds a genuine seam.

---

## 8. Deferred items

- **`custom_tags` `metadata` `ROUTE`/`CLUSTER`/`HOST` MetadataKinds** — per-object (route/cluster/host) metadata, not the request dynamic-metadata `Bucket`; not exposed at the span-emit seam. Rejected loudly this row; carries forward.
- **`custom_tags` `metadata` non-string value serialization** — number/bool/struct/list `structpb.Value` → string forms, if the SPEC finds the reference's rendering non-trivial (the fixture uses a string value). Carries forward as a documented boundary.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span. Carries forward.
- **`http_service`** — an OTLP-over-HTTP tracer transport. Carries forward.
- **force-trace (`x-envoy-force-trace`)** — needs internal-request detection + edge sanitization (§2.1). Carries forward.
- **The `ssl` downstream handshake-outcome stat family** (`ssl.handshake`/`fail_verify_error`/`fail_verify_no_cert`/`ciphers.*`/`versions.*`, ADR-0286 C3) — a framework-surgery row (the verify-failure taxonomy needs a per-chain TLS-callback), UNtouched here. Carries forward.
- **The 4 EMPTY built-in span attributes** (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) — a framework-surgery deferral, UNtouched here. Carries forward.
- **A SIXTH stats sink** (hystrix / wasm) — admin-streaming / runtime-dependent; off the landed `Flusher`/`Submit` substrate. Carries forward.

After row 70 the `custom_tags` candidate NARROWS (all four SOURCE types done; the three non-`REQUEST` MetadataKinds + the other tracing follow-ons remain) in the LIVE Observability deferred sentence (at the IMPL); the `ssl` family + `spawn_upstream_span`/`http_service`/force-trace remain, and the xDS/HTTP-3 sentences are untouched ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 70 PICKS UP tracing `custom_tags` (scoped to the `metadata` type, `REQUEST` kind) — named in the ROADMAP Observability family's LIVE deferred sentence (`… + tracing custom_tags (`metadata`)/`spawn_upstream_span`/`http_service`/force-trace`). This continues the phase-59 (literal) / 62 (request_header) / 63 (environment) lineage and CONSUMES the last `CustomTag` source type. After phase 70 the remaining Observability candidates are: the three non-`REQUEST` MetadataKinds + `spawn_upstream_span`/`http_service`/force-trace + the `ssl` stat family. The family STAYS OPEN. **Sentinel maintenance (at the IMPL):** after NARROWING `custom_tags (`metadata`)` in the deferred sentence, re-run the check-(2) grep — require EXACTLY ONE live Observability "candidates:" match with the intended content (`reference_sentinel_deferred_sentence_live_vs_historical`), and leave the HTTP/3 + xDS sentences untouched (three live matches total).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-CTM-SCOPE** — confirm the `REQUEST`-only slice (`ROUTE`/`CLUSTER`/`HOST` reject loudly with distinct substrings, §2.4). §2.2.
- **D-CTM-PROTO** — RE-DERIVE the exact go-control-plane types at tip: `CustomTag_Metadata{Kind *MetadataKind; MetadataKey *MetadataKey; DefaultValue}`, `MetadataKind` (Request/Route/Cluster/Host oneof), `MetadataKey{Key; Path []*MetadataKey_PathSegment}`, `MetadataKey_PathSegment` (key-oneof). §2.2/§2.5.
- **D-CTM-WIRE** — how the reference emits a `REQUEST` metadata custom tag on the OTLP span: key = the `tag` name, value = the serialized dynamic-metadata value as a STRING `AnyValue`; appended after the built-ins. ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` (`reference_probe_fresh_container_per_arm`, `reference_envoy_contrib_image_tagging`) with a Lua-set metadata value + a configured metadata `custom_tag`, observed via `test/helpers/otlptrace` (`reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop` for reachability). §2.3.
- **D-CTM-ZIPKIN-WIRE** — the same metadata tag on the Zipkin span (the `tags` map). Probe a Zipkin-provider config. §2.3/§2.6.
- **D-CTM-VALUE-SERIALIZE** — the reference's `structpb.Value` → span-tag string rendering (string case pinned; non-string named as a boundary if non-trivial). §2.3/§8.
- **D-CTM-DEFAULT** — `default_value` / omit semantics: absent path vs present-but-empty value vs absent-with-empty-default (anticipated the `environment` omit-on-empty-resolved precedent; probe pins it). §2.3.
- **D-CTM-PATHWALK** — the `MetadataKey.path` → `Bucket.Get(ns, path[0])` + nested-`StructValue` walk of `path[1:]` mapping; the single-segment common case + a multi-segment probe. §2.3/§2.5.
- **D-CTM-CONFIG-SEAM** — the `CustomTagSpec` field shape (namespace + `[]string` path + default). RE-DERIVE the `config.go:63` struct + `:53-55` kind constants. §2.5.
- **D-CTM-BUCKET-SEAM** — how the per-request `Bucket` reaches `ResolveCustomTags` at the three `accesslog_emit.go` call sites (`:55` H1/`:116` H2/`:177` H3) — RE-DERIVE whether the per-protocol `chain` (built at `connection.go:350`/`h2dispatch.go:327`/`h3dispatch.go:148`; `chain.DynamicMetadata()`, `chain.go:837`/`:963`) is already in scope at the emit or must be threaded; the nil-tolerant `metaLookup` shape. §2.5.
- **D-CTM-REJECT** — the three unsupported-kind reject substrings (ADR-0080 distinct) + the empty-namespace/empty-path/unset-kind handling (envoy-go's own reject vs the reference's PGV boot-reject; RE-DERIVE the relevant PGV lines). Confirm (one probe arm) the reference ACCEPTS `ROUTE`/`CLUSTER`/`HOST` (the departure is real). §2.4.
- **D-CTM-FIXTURE** / **D-CTM-FIXTURE-SOURCE** — ONE new fixture (OTLP) or TWO (OTLP + Zipkin)? The Lua-filter dynamic-metadata WRITER for the positive arm (cross-side wire-identical namespace/value?) + the `default_value` arm. New dir(s), NOT a baseline mutation (the dispatch constraint). Fixtures **115 → 116** (or **117**). Whether the Lua source / a Zipkin dir grows the row enough to split the fixture leg (ADR-0045). §2.6.
- **D-CTM-FUZZSEED** — a SEED to the EXISTING `FuzzHCMConfigParse` (NOT a new fuzzer); dispatch-verify the HCM fuzzer reaches the `parseCustomTags` metadata arm; fuzzer count STAYS 55 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.7.
- **D-CTM-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~10–13 tasks; escape-valve armable only if the path-walk + value-serialization + a Zipkin fixture surprise upward). §1.4.

---

## 11. Prior-phase lessons applied

- **`reference_deferred_candidate_cost_restale`** — the roller's FIRST pick (`environment`) was RE-DERIVED STALE at tip: `config.go:208-213` shows `environment` + `request_header` ALREADY LANDED (phase 62/63). Landed rows shrink deferred candidates; the pick was corrected to `metadata` (the sole remaining type) by DIRECT READ, not by trusting the deferred sentence's `(metadata)` parenthetical alone. §Preamble/§2.1.
- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go` metadata reject + `CustomTagSpec` struct + kind constants, `resolve.go` `ResolveCustomTags`, the `accesslog_emit.go` call sites, the `dynamicmetadata.Bucket` API, the Lua/mongo writers, the go-control-plane `CustomTag_Metadata`/`MetadataKey`/`MetadataKind` proto types, the fuzz host) was RE-DERIVED from source this session; the SPEC RE-DERIVES again. A scouting brief that attributed `environment` to "phase 62" was corrected against the git log (`environment` landed at phase **63**).
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (55) against actual `^func Fuzz` before AND after — the count must NOT move. §2.7.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-CTM-WIRE / D-CTM-ZIPKIN-WIRE / D-CTM-VALUE-SERIALIZE / D-CTM-DEFAULT / D-CTM-PATHWALK / D-CTM-REJECT) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`; the metadata WRITER (Lua) is served this-arm and asserted (`feedback_probe_fresh_container_per_arm` — a driver-owned server needs the same served-this-arm discipline). §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the OTLP/Zipkin span probes need a shared bridge network + a reachable receiver; verify the span decode ACTUALLY ran (not a vacuous empty capture). §10.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir per runner branch; do NOT mutate a tracing baseline. §2.6.
- **`reference_tracing_upstream_cluster_framework_gap`** — the 4 EMPTY built-in span attributes are a KNOWN framework gap; a `REQUEST`-kind metadata tag is populated from the LANDED per-request `Bucket`, INDEPENDENT of that un-plumbed seam (do NOT conflate). §1.2/§8.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after the IMPL NARROWS `custom_tags (`metadata`)` in the Observability deferred sentence, re-run the check-(2) grep; EXACTLY ONE live Observability "candidates:" match, HTTP/3 + xDS untouched (three total). §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — each of the three unsupported-kind rejects carries a DISTINCT substring so a future silent divergence surfaces. §2.4.
- **`reference_fatalf_makes_assertions_unreachable`** — the span-attribute fixture/unit tests assert each independent property with `Errorf` (not `Fatalf`), so a metadata-tag failure does not mask the built-in-attribute or default-arm assertions. §6.
- **`reference_dynamic_stat_name_charset_guard`** — cited only to CONTRAST: the rejected `ssl`-stat rival would need this guard for wire-derived per-cipher/version counter names; a span attribute registers NO stat, so this row does not (part of why `ssl` is +N and metadata is +0). §2.1/§2.8.

---

## 12. Section closeout

**Settled:** subject (tracing `custom_tags`, `metadata` type / `REQUEST` MetadataKind only, SELF-PICKED per the standing directive over a fully-adjudicated rejected roster, §2.1 — after the `environment` first-pick was RE-DERIVED STALE); scope (`REQUEST` accepted; `ROUTE`/`CLUSTER`/`HOST` reject loudly with distinct substrings, an envoy-go-strict departure, §2.2/§2.4); the resolve seam (a `kindMetadata` arm in `ResolveCustomTags` reading the landed `Bucket` + a MetadataKey path-walk + `structpb.Value`→string, appended to `Span.Attrs` — ONE change covers OTLP + Zipkin, §2.3); config home + threading (a `kindMetadata` `CustomTagSpec` extension + a nil-tolerant `metaLookup` threaded to the three `accesslog_emit.go` call sites, §2.5); fixture posture (anticipated ONE new OTLP fixture with a Lua-set positive arm + a `default_value` arm, a Zipkin dir weighed, §2.6); fuzz posture (a SEED to `FuzzHCMConfigParse`, no new fuzzer, §2.7); stat surface (+0, §2.8); envelope (SINGLE FLAT ROW anticipated, ~10–13 tasks — ADR-0292, §1.4). The novel production code is the metadata accept + three reject arms in `parseCustomTags`, the `kindMetadata` resolve arm (`Bucket` Get + path-walk + serialize) in `ResolveCustomTags`, and the `Bucket` threading at the three seams; the row's differential value is the OBSERVABLE dynamic-metadata span attribute, cross-side.

**Anticipated moves at the phase-70 IMPL (docs-only now):** the metadata accept + three reject arms in `parseCustomTags` + the `CustomTagSpec` extension + the `ResolveCustomTags` metadata arm + the three call-site threadings + config/resolve unit tests + a `FuzzHCMConfigParse` seed + the new OTLP fixture (and possibly a Zipkin fixture) + the BEHAVIOR_CONTRACT tracing edit + ADR-0292 + the ROADMAP deferred-sentence narrow. Counts: stat surface **1201 (+0)** · fixtures **115 → 116** (or **117** with a Zipkin dir) · fuzzers **55 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0292** (next-free **ADR-0293**) · new Go packages **0** · new go.mod modules **2 (+0)**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `33ed4c05`):** stat surface **1201** · fixtures **115** · fuzzers **55** · BackendKind **38** · DECISIONS tail **ADR-0291** (COMPLETE; next-free **ADR-0292**) · go.mod modules **2**. Row 70 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Sentinel re-run MECHANICALLY at this CLOSE (does NOT fire; `stop` NOT created) — the post-registration state, distinct from the session-START silent-(1) state cited in the preamble (which justified continuing the loop before row 70 existed):** (1) prints `NOT DONE: row 70` (the §Schema registration re-opens check (1) until the phase-70 IMPL flips it `done`); (2) prints THREE live `candidates:` sentences (`grep -cE 'remaining deferred \(not-yet-chartered\) candidates:' docs/envoy-go/ROADMAP.md` ⇒ **3** — HTTP/3 + xDS + Observability, the Observability `custom_tags (metadata)` candidate NOT narrowed at this stage, phase-57 precedent); (3) prints `NEVER OPENED: gRPC`, `NEVER OPENED: Runtime`, `NEVER OPENED: WASM`. All three print ⇒ the sentinel does NOT fire.

**Next → the phase-70 SPEC** (the D-CTM-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-CTM-WIRE / D-CTM-ZIPKIN-WIRE / D-CTM-VALUE-SERIALIZE / D-CTM-DEFAULT / D-CTM-PATHWALK / D-CTM-REJECT; re-derive every §6 edit site + the go-control-plane `CustomTag_Metadata`/`MetadataKey`/`MetadataKind` proto types + the relevant PGV lines; pin D-CTM-CONFIG-SEAM + D-CTM-BUCKET-SEAM + D-CTM-FIXTURE; draft ADR-0292 §Context).
