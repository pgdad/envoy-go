# Phase 59 Brainstorm — tracing `custom_tags` (LITERAL tag type only) (the TWELFTH Observability-family row; the FIRST tracing follow-on since phase 46; lifts the wholesale `custom_tags unsupported` reject and supports the `literal` `CustomTag` type as a static span attribute — the other three types [`request_header`/`environment`/`metadata`] stay PARSE-REJECTED loudly (envoy-go-strict, ADR-0080); +0 stats / +0 packages / +0 modules; anticipated ONE new fixture)

> **Stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only; no `.go` changes at this stage. Fresh worktree `.worktrees/phase-59-brainstorm`, branch `phase-59-tracing-custom-tags-literal-brainstorm`, per `feedback_git_worktrees`.
>
> **Loop re-open (AUTONOMOUS — no human pick):** phase 58 (`stats-sink-dogstatsd-explicit-zero`) landed COMPLETE (row 58 `done`, ADR-0276). Per the **STANDING DIRECTIVE (human, 2026-07-12)** the loop runs AUTONOMOUSLY until the termination sentinel fires; the sentinel was re-checked MECHANICALLY at the phase-58 IMPL and does NOT fire (check (1) silent — every row `done` — but checks (2) [Observability + Operational-tooling deferred lists] and (3) [five never-opened families] each still print, and each independently blocks `stop`). So the roller SELF-PICKED the next subject (§2.1): the **smallest defensible candidate** from the live Observability deferred sentence — tracing `custom_tags`, scoped to the **literal** tag type only — over four declined larger alternatives (recorded §2.1). No human pause; no `stop` file.
>
> **Baselines re-verified against master tip `29953648` (the phase-58 IMPL squash):** stat surface **1201** · fixtures **103** (tail `0101-stats-sink-graphite`) · fuzzers **54** · BackendKind tail **38** (`H2GoawayResponder`) · DECISIONS tail **ADR-0276** (next-free **ADR-0277**) · new Go packages **0** · new go.mod modules **0**. Counts are UNCHANGED at a BRAINSTORM (docs-only). All `file:line` citations below were RE-DERIVED from source this session (`feedback_brief_citations_not_evidence`) — see §11.

---

## 1. Mission and scope confirmation (59 — the FIRST tracing follow-on; a literal-only `custom_tags` slice)

### 1.1 What phase 59 delivers as a self-contained whole (literal custom tags on the ingress span)

The HCM `tracing` config today rejects `custom_tags` WHOLESALE:

```go
// internal/tracing/config.go:82-83 (re-derived against master 29953648)
if len(t.GetCustomTags()) > 0 {
    return nil, fmt.Errorf("tracing: custom_tags unsupported")
}
```

Phase 59 **lifts that reject** and supports the **`literal`** `CustomTag` type — a static operator-configured `{tag, value}` pair emitted as a span attribute on the ingress SERVER span. The other three `CustomTag` types (`request_header`, `environment`, `metadata`) stay **PARSE-REJECTED loudly**, each with its own distinct substring (envoy-go-strict, ADR-0080). The literal is the foundational tag type: it needs NO per-request data (it is a fixed config value), which is exactly why it is the smallest defensible slice — the request-driven types layer on later rows.

The delivery is a complete, testable slice: an operator configuring `custom_tags: [{tag: "env", literal: {value: "prod"}}]` on the HCM `tracing` message gets a `{key: "env", value: "prod"}` STRING attribute on every sampled span, on **both** the OTLP and the Zipkin exporter (see §2.3 — the single span-emit seam covers both providers).

### 1.2 What phase 59 does NOT deliver (forward to §8)

NO `request_header` / `environment` / `metadata` custom-tag types (deferred; rejected loudly — §2.4). NO other tracing follow-on (`spawn_upstream_span` / `http_service` / force-trace — each larger, §2.1/§8). NO `max_path_tag_length` (a distinct, still-rejected knob, `config.go:79-80`; orthogonal, deferred). NO OTLP-metrics stats sink. NO new provider, transport, or stat. The four built-in span attributes currently emitted EMPTY (`upstream_cluster`/`node_id`/`zone`/`peer.address`, the `reference_tracing_upstream_cluster_framework_gap` framework gap) are UNTOUCHED — literal custom tags are static and independent of that gap (§11).

### 1.3 Phase-done as the TWELFTH Observability-family row landing (family STAYS OPEN)

Row 59 is the TWELFTH Observability-family row and the FIRST tracing follow-on since the phase-46 tracing engine landed. After phase 59 phase-done the family STAYS OPEN — the deferred candidates in §8 remain (OTLP-metrics sink + the three NON-literal custom-tag types + `spawn_upstream_span`/`http_service`/force-trace), so the sentinel check-(2) still prints ⇒ the loop continues.

### 1.4 ADR-0045 split readiness — anticipated a SINGLE FLAT ROW (escape-valve armable) *(self-answered, SPEC confirms)*

Anticipated a SINGLE FLAT ROW (~9–13 tasks — the parse arm + the span-emit append + the two-call-site config threading + the fixture(s) + the fuzz seed + the doc/BEHAVIOR_CONTRACT edits + verify + ADR-0277), comfortably under the ADR-0045 `~15` ceiling. There is no second subsystem to strand: the parse and the span-emit sit on the SAME tracing engine, and the reject-the-other-three-types arms live in the same parse function. The escape valve is documented ARMABLE and re-armed only if the SPEC's task count surprises upward (e.g. if a SEPARATE Zipkin fixture + Zipkin-encoder verification pushes the fixture leg into its own row — SPEC weighs it, §2.6).

### 1.5 Seed-stub alignment + package placement — ALL edits in EXISTING files, ZERO new packages

- Production parse arm + the three reject arms: `internal/tracing/config.go` `NewConfig` (existing, `config.go:71`).
- Parsed-config home: a NEW field on `tracing.TracingConfig` (`config.go:24`, existing struct).
- Span-emit append: `internal/tracing/span.go` `BuildServerSpan` (existing, `span.go:64`) + possibly its signature (§2.5, a small seam — SPEC pins).
- Call-site threading: `internal/filter/hcm/accesslog_emit.go` (existing) — the two `BuildServerSpan` call sites (`:55` H1, `:116` H2).
- Fuzz SEED: `internal/filter/hcm/fuzz_test.go` `FuzzHCMConfigParse` (existing, `:25`) — a NEW seed, NOT a new fuzzer (§2.7).
- Fixture(s): anticipated ONE new `test/fixtures/NNNN-tracing-custom-tags` dir (or an extend of `0087`/`0088` — SPEC decides via the dispatch constraint, §2.6).
- Docs: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (tracing section) + ROADMAP/STATE/DECISIONS.

Possibly ONE new `.go` file only if the SPEC decides a literal-tag type/helper warrants it (unlikely — a `[]KV` field + an append loop fits the existing files). ZERO new packages, ZERO new modules.

### 1.6 No prebrainstorm-notes branch

No off-master prebrainstorm-notes branch exists for this subject (the phase-46 tracing work is fully landed; `custom_tags` is a recorded deferred candidate in the ROADMAP Observability family's LIVE deferred sentence, not a stashed WIP).

### 1.7 Phase 59's relationship to the existing seams (a parse arm + one span-attribute append on the phase-46 engine)

The phase-46 tracing engine already exposes exactly the two seams this row needs: (a) `NewConfig` (`config.go:71`), the HCM-`tracing`-proto → `TracingConfig` parse, where the wholesale reject lives and the literal-parse + three reject arms land; and (b) `Span.Attrs []KV` (`span.go:12`, `:54`), the provider-neutral attribute list that BOTH exporters consume (OTLP `toProto` `span.go:112`; Zipkin `encodeZipkinSpans` `zipkin.go:78`). The ONLY novel production code is the literal parse/reject arms in `config.go` and the append loop in `BuildServerSpan`; everything else is config-threading + doc/test/fixture reconciliation.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with tracing `custom_tags` (literal only) *(SELF-PICKED per the standing directive → phase 59 row registered)*

The FIRST decision, made AUTONOMOUSLY (no human pick) per the 2026-07-12 standing directive. Picked as the **smallest defensible candidate** from the live Observability deferred sentence, after INVESTIGATING each candidate's size against source this session (§11). Row 59 registers `in-progress` AT this BRAINSTORM commit per the ROADMAP §Schema invariant.

**Why `custom_tags` (literal) is smallest-defensible:** the span-attribute seam (`Span.Attrs []KV`) already exists and is consumed by BOTH exporters, so ONE append covers OTLP + Zipkin; and literal tags need NO per-request data (they are static config), so no per-request plumbing beyond passing the parsed config to `BuildServerSpan`. The reference behavior is fully deterministic (a fixed literal string on the span).

**Rejected alternatives (recorded per the standing directive; each SIZED against source this session):**
- **force-trace (`x-envoy-force-trace`)** — the decision arm is tiny (one arm in `DecideWithContext` + a new `'a'` reason nibble + wiring the ALREADY-reserved `service_forced` counter, `stats.go:37`/`decision.go:27`). BUT the reference honors `x-envoy-force-trace` ONLY for INTERNAL requests and edge-sanitizes (STRIPS) it for external ones, and envoy-go has **no internal-request / edge-sanitization concept at all** (grep confirms only a DEFERRED `x-envoy-internal` gate stub in ratelimit `compiled_config.go:259-263`, unimplemented). Honoring it unconditionally would DIVERGE from the reference in a differential (different `x-request-id` nibble + `service_forced` count). So force-trace drags in a whole new internal-request subsystem — HIGH scope-risk, plausibly a split. Deferred.
- **`spawn_upstream_span`** — envoy-go emits ONE ingress SERVER span (`span.go:64` `Name: "ingress"`, `Kind: SERVER`); `spawn_upstream_span` adds a second CLIENT span for the upstream leg, with its own timing wired at the router/upstream seam. Medium-large; touches the span model. Deferred.
- **`http_service` (OTLP HTTP transport)** — a new HTTP exporter transport alongside the existing `envoy_grpc` OTLP path (`config.go:126-128` currently rejects it). Reuses the buffer/flush machinery but a new transport + protobuf-over-HTTP encoding. Medium. Deferred.
- **OTLP-metrics stats sink** — a full new gRPC `stats_sinks[]` consumer (new proto type-URL, a new streaming receiver, a new fixture). The largest remaining Observability follow-on. Deferred.
- **Opening a new family** (HTTP/3 / gRPC / xDS / Runtime / WASM never-opened; Operational-tooling open) — the standing directive says smallest-defensible-first, and the Observability tail STILL holds a cheap candidate (literal `custom_tags`), so smallest-first keeps us in Observability. Deferred; revisit when the Observability tail's cheap candidates are drained.

### 2.2 Scope: LITERAL tag type ONLY; the other three types PARSE-REJECT loudly *(self-answered; the incremental-arm precedent)*

The `CustomTag` proto (`type.tracing.v3.CustomTag`, go-control-plane/envoy v1.32.4) is `tag` (field 1, string) + a `type` oneof: `literal` (2, `CustomTag_Literal{value}`) / `environment` (3, `CustomTag_Environment{name, default_value}`) / `request_header` (4, `CustomTag_Header{name, default_value}`) / `metadata` (5). Phase 59 supports ONLY `literal`; `environment`/`request_header`/`metadata` REJECT loudly with distinct substrings (§2.4). This mirrors the project's landed incremental-arm posture (OTel `envoy_grpc`-transport-only rejecting `google_grpc`/`http_service`, `config.go:126-138`; Zipkin `HTTP_JSON`-only rejecting other endpoint versions, `config.go:165-167`). A literal-only slice is a complete, useful, deterministic capability (static labels like an environment/region name on every span), and the SPEC probe confirms the reference emits it identically (D-CT-LITERAL-WIRE).

### 2.3 The span-emit seam: append literal tags to `Span.Attrs` after the built-ins — ONE change, TWO providers *(self-answered; the KV seam is landed)*

`BuildServerSpan` (`span.go:64`) assembles the 16 built-in attributes + the optional `guid:x-client-trace-id` into `Span.Attrs []KV` (`span.go:65-93`). BOTH exporters consume `Attrs`: OTLP `toProto` (`span.go:112-134`, each `KV` → a `commonpb.KeyValue` string/int `AnyValue`) and the Zipkin encoder (`zipkin.go:78`, the tags map). So the literal custom tags append to `Attrs` in `BuildServerSpan` (after the built-ins) and BOTH providers emit them with NO exporter-specific code. Literal tags are STRING-valued (`KV{Key: tag, Str: value}`). **D-question D-CT-PRECEDENCE:** the append-vs-override behavior when a literal tag's `tag` collides with a built-in attribute key (e.g. `tag: "http.method"`) — does the reference emit both or override the built-in? SPEC probes; the anticipated posture is APPEND (the reference builds custom tags after built-ins), but the probe pins it.

### 2.4 The three unsupported tag types reject loudly with DISTINCT substrings — an envoy-go-strict DEPARTURE, not a parity claim *(self-answered; ADR-0080)*

The reference SUPPORTS all four `CustomTag` types; envoy-go rejecting three of them is a documented envoy-go-strict DEPARTURE (like the OTel-transport and Zipkin-version rejects), NOT a reference-parity behavior. Each reject carries its own distinct substring (ADR-0080 anti-silent-divergence), anticipated:
- `tracing: custom_tags request_header type unsupported`
- `tracing: custom_tags environment type unsupported`
- `tracing: custom_tags metadata type unsupported`
Plus the PGV-derived structural rejects: an empty `tag` (PGV `min_len: 1` on `CustomTag.tag`, re-derived from `custom_tag.pb.validate.go:61-64`) and a `CustomTag` with NO `type` oneof set (a typeless tag). The exact substrings + whether the empty-tag/typeless rejects are envoy-go's own or the reference's PGV boot-reject are SPEC-pinned (D-CT-REJECT). **D-CT-REJECT** also confirms (one probe arm) that the reference ACCEPTS a `request_header`/`environment`/`metadata` tag — so the departure is real (the reference boots where envoy-go rejects), not a shared reject.

### 2.5 Config home + threading: a `[]KV` field on `TracingConfig`, threaded to `BuildServerSpan` *(self-answered shape; SPEC pins the exact seam — D-CT-CONFIG-SEAM)*

The parsed literal tags live on `tracing.TracingConfig` (`config.go:24`) — a new field, anticipated `CustomTags []KV` (reusing the existing `span.KV` type, or a small local literal-tag struct). They thread to `BuildServerSpan` at the two `accesslog_emit.go` call sites (`:55` H1, `:116` H2), where `f.tracingConfig` is already in scope (the `Filter` holds it — cf. `connection.go:545` `f.tracingConfig`). The seam decision (extend `BuildServerSpan`'s signature to take the tags, vs carry them on the existing `SpanInputs` struct `span.go:21`, vs read them off a field) is a small design choice SPEC pins as **D-CT-CONFIG-SEAM**; the anticipated shape is a new `BuildServerSpan(d, in, customTags, start, end)` parameter (explicit, keeps `SpanInputs` per-request-only). A minor seam ADR is possible but likely folded into ADR-0277 (phase 58 similarly used no seam ADR).

### 2.6 Fixture posture: anticipated ONE new fixture (OTLP); a Zipkin fixture weighed *(self-answered direction; SPEC confirms D-CT-FIXTURE)*

Unlike a boot-reject (parse-time, unit-tested — phase 58), a literal custom tag is an OBSERVABLE span attribute, so it IS differential-provable: a fixture configures a literal `custom_tag` and asserts the `{key, value}` attribute on the reference AND subject span. The existing `0087-tracing-otlp` + `0088-tracing-zipkin` fixtures are pure baselines; per the differential dispatch constraint (`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch, and adding an assertion changes what a dir proves) the anticipated posture is a NEW `test/fixtures/NNNN-tracing-custom-tags` dir (OTLP-provider, asserting the literal tag on the OTLP span via the `test/helpers/otlptrace` receiver) rather than mutating `0087`. Whether a SECOND Zipkin fixture is also needed (the Zipkin encoder path is exercised only by a Zipkin-provider fixture) is **D-CT-FIXTURE**; if a Zipkin fixture materially grows the row, the SPEC may split the fixture leg (ADR-0045 escape-valve, §1.4). Anticipated: fixtures **103 → 104** (or **105** with a Zipkin dir) — SPEC pins.

### 2.7 Fuzz posture: a SEED to the EXISTING `FuzzHCMConfigParse` — NO new fuzzer *(self-answered; count stays 54 → SPEC confirms D-CT-FUZZSEED)*

The tracing config is parsed via `NewConfig` off the HCM proto, and the HCM config parse path is fuzzed by `FuzzHCMConfigParse` (`internal/filter/hcm/fuzz_test.go:25`) — there is NO dedicated `internal/tracing` config fuzzer (the tracing package's fuzzers are `FuzzExtractB3`/`FuzzExtractTraceparent`/`FuzzStampRequestID`, all propagation/request-id, NOT config). So the new literal-parse + reject arms are exercised by adding a `custom_tags` SEED (a literal tag + at least one rejected type) to `FuzzHCMConfigParse` — NOT a new fuzzer. Fuzzer count STAYS **54** (`reference_fuzzer_count_docs_drift`: reconcile the documented running total against actual `^func Fuzz` before AND after — the count must not move). SPEC confirms D-CT-FUZZSEED (and that the HCM fuzzer actually reaches the tracing-config parse arm).

### 2.8 Stat surface hypothesis: +0 *(self-answered; a span attribute registers no stat)*

A span attribute is emitted on the wire, not registered as a stat. The existing HCM tracing counters (`client_enabled`/`health_check`/`not_traceable`/`random_sampling`/`service_forced`, `stats.go:32-38`) and the tracer counters (`spans_sent`/`spans_dropped`) are UNCHANGED. Anticipated stat surface **1201 (+0)**, UNCHANGED. No new registration path.

---

## 3. Framework-survey result — a parse arm + one span-attribute append on the phase-46 engine; ZERO new packages/modules (59 anticipated)

### 3.1 Framework: a small seam at most (a `TracingConfig` field + possibly a `BuildServerSpan` parameter)

No new interface, no new package-level type beyond a `[]KV` (or a small literal-tag) field on the existing `TracingConfig`. `BuildServerSpan` may gain one parameter (§2.5). Every other symbol is pre-existing.

### 3.2 NEW packages: NONE

All edits land in `internal/tracing` + `internal/filter/hcm` (both existing) + `test/fixtures` + `docs/`. No new package.

### 3.3 go.mod modules: NONE

The `type.tracing.v3.CustomTag` proto is already reachable via the resolved `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module (the same module the HCM tracing proto lives in; `t.GetCustomTags()` returns `[]*<tracing/v3>.CustomTag`, `http_connection_manager.pb.go`). No new import of a new module. `go mod tidy -diff` anticipated EMPTY. (The `tracing/v3` package may need a blank/named import in `config.go` — an EXISTING-module import, not a new module.)

### 3.4 REUSES

- **phase-46** the whole tracing engine: `NewConfig` (the parse home), `TracingConfig` (the config home), `Span`/`Span.Attrs []KV`/`BuildServerSpan` (the span-attribute seam), the OTLP + Zipkin exporters (both consume `Attrs`), and the `0087`/`0088` tracing fixtures + the `test/helpers/otlptrace` receiver.
- **the incremental-reject precedent** (`config.go` OTel-transport + Zipkin-version rejects) as the template for the three unsupported-tag-type reject arms.
- **`FuzzHCMConfigParse`** as the fuzz host (a seed, no new fuzzer).

---

## 4. Bootstrap-level applicability — a PER-LISTENER HCM filter config (NOT bootstrap `stats_sinks[]`)

Unlike the phase-47..58 stats-sink rows (bootstrap `stats_sinks[]`), `custom_tags` is a PER-LISTENER HCM `tracing` sub-field, parsed by `NewConfig` from `HttpConnectionManager.tracing.custom_tags` when the HCM filter is built. No bootstrap change; the parse lands INSIDE the existing `NewConfig` (the same function that already rejects `custom_tags` wholesale). The fixture configures `custom_tags` on the listener's HCM tracing block.

---

## 5. Stat surface hypothesis — +0 (59)

### 5.1 Stat names (SPEC confirms)

NONE. A span attribute registers no stat.

### 5.2 envoy-go-strict departure flags

The three unsupported `CustomTag` types (`request_header`/`environment`/`metadata`) are a documented envoy-go-strict DEPARTURE (the reference supports them; envoy-go rejects loudly, §2.4) — the SAME posture as the OTel-transport / Zipkin-version rejects. No new stat, no new flag; it is a parse-reject departure recorded in BEHAVIOR_CONTRACT.

### 5.3 Anticipated surface arithmetic

Stat surface **1201 → 1201 (+0)**.

---

## 6. Edit-site enumeration — RE-DERIVED this session (SPEC re-derives + pins D-CT-CONFIG-SEAM / D-CT-DOCSHAPE)

Each `file:line` RE-DERIVED against master `29953648` this session (`feedback_brief_citations_not_evidence`); the SPEC re-derives again.

**Production — `internal/tracing/config.go`:**
1. **The literal-parse + three reject arms** — replace the wholesale reject (`config.go:82-83`) with: iterate `t.GetCustomTags()`; per tag, reject an empty `tag`/typeless tag (PGV-derived) + `request_header`/`environment`/`metadata` (distinct substrings, §2.4); accept `literal` → append `{tag, GetLiteral().GetValue()}` to the parsed list. [EDIT/ADD]
2. **`TracingConfig` struct** (`config.go:24-32`) — add the parsed-literal-tags field (anticipated `CustomTags []KV`). [EDIT]
3. **Both parse-return sites** (`parseOTel` `config.go:145-152`, `parseZipkin` `config.go:181-194`) — carry the parsed literal tags onto the returned `TracingConfig`. [EDIT — the tags are provider-neutral, parsed in `NewConfig` before the provider dispatch and threaded onto both returns.]

**Production — `internal/tracing/span.go`:**
4. **`BuildServerSpan`** (`span.go:64`) — after the built-ins + optional `guid:x-client-trace-id`, append the literal custom tags to `attrs`. Possibly a new parameter (§2.5, D-CT-CONFIG-SEAM). [EDIT]

**Production — `internal/filter/hcm/accesslog_emit.go`:**
5. **Both `BuildServerSpan` call sites** (`:55` H1, `:116` H2) — pass `f.tracingConfig`'s literal tags. [EDIT — 2 sites]

**Test:**
6. **`internal/tracing/config_test.go`** — accept a literal tag; reject each of the three unsupported types + empty-tag + typeless (distinct substrings). [ADD]
7. **`internal/tracing/span_test.go`** — a literal tag appears in `Span.Attrs` after the built-ins; precedence per D-CT-PRECEDENCE. [ADD]
8. **`internal/filter/hcm/fuzz_test.go`** `FuzzHCMConfigParse` — a `custom_tags` SEED (literal + a rejected type). [ADD — no new fuzzer]

**Fixture:**
9. **`test/fixtures/NNNN-tracing-custom-tags`** (new) — a literal `custom_tag` on an OTLP-provider listener; assert the `{key, value}` attribute on both spans. Possibly a second Zipkin dir (D-CT-FIXTURE). [ADD]

**BEHAVIOR_CONTRACT (`docs/envoy-go/BEHAVIOR_CONTRACT.md`):**
10. **the tracing section** — flip `custom_tags` from "unsupported (rejected)" to "the `literal` type is consumed (emitted as a span attribute on both exporters); `request_header`/`environment`/`metadata` reject loudly (envoy-go-strict departure)". SPEC RE-DERIVES the exact line(s). [EDIT]

**ROADMAP / STATE / DECISIONS:**
11. **ROADMAP** — row 59 `in-progress` at this BRAINSTORM (§Schema); the family prose gains a "phase 59 CHARTERED and BRAINSTORMED" sentence. The LIVE deferred sentence NARROWS `custom_tags` to the non-literal types at the phase-59 IMPL (NOT now — re-run the sentinel check-(2) grep after that edit, `reference_sentinel_deferred_sentence_live_vs_historical`, keeping EXACTLY ONE live "candidates:" match). [BRAINSTORM: row + prose; IMPL: deferred-list narrow]
12. **STATE.md** — active-phase header flips to phase 59 BRAINSTORM (this stage). [EDIT]
13. **DECISIONS.md** — ADR-0277 §Context drafts at the SPEC, §Decision/§Consequences at the IMPL (ADR-0044). NOT at this BRAINSTORM. [SPEC/IMPL]

SPEC pins **D-CT-DOCSHAPE** (this full edit-site roster, RE-DERIVED) + **D-CT-CONFIG-SEAM** (the `TracingConfig` field + `BuildServerSpan` threading shape).

---

## 7. Anticipated ADRs — 1 at the phase-59 IMPL: ADR-0277 (tracing `custom_tags` literal)

ADR-0277 (tracing `custom_tags` literal type — lifting the wholesale reject, the literal span-attribute emit, the three-type strict-reject departure). §Context drafted at the SPEC (the gap's provenance: the phase-46 wholesale reject + the ROADMAP deferred sentence), §Decision/§Consequences at the IMPL per ADR-0044. A minor seam sub-note (the `TracingConfig` field + `BuildServerSpan` parameter) is anticipated FOLDED into ADR-0277 (no separate seam ADR — the phase-58 precedent); the SPEC re-decides if it finds a genuine seam. Next-free after: **ADR-0278**.

---

## 8. Deferred items

- **`custom_tags` `request_header` type** — read a request header (with `default_value`) as a span tag; the immediate next custom-tag slice (needs per-request header access at both call sites). Carries forward.
- **`custom_tags` `environment` type** — an env-var value as a span tag. Carries forward.
- **`custom_tags` `metadata` type** — a dynamic-metadata value as a span tag. Carries forward.
- **`spawn_upstream_span`** — a second (upstream CLIENT) span. Carries forward.
- **`http_service`** — an OTLP HTTP exporter transport. Carries forward.
- **force-trace (`x-envoy-force-trace`)** — needs internal-request detection + edge sanitization (§2.1). Carries forward.
- **`max_path_tag_length`** — caps the `http.url` tag length; still rejected (`config.go:79-80`); orthogonal. Carries forward.
- **`OTLP-metrics` stats sink** — the largest remaining Observability sink follow-on. Carries forward.
- **`stats_flush_on_admin`** — still rejected (`bootstrap.go`); orthogonal. Carries forward.
- **The 4 EMPTY built-in span attributes** (`upstream_cluster`/`node_id`/`zone`/`peer.address`, `reference_tracing_upstream_cluster_framework_gap`) — a framework-surgery deferral, UNtouched here. Carries forward.

After row 59 the `custom_tags` candidate NARROWS (literal done; the three non-literal types remain) in the LIVE deferred sentence (at the IMPL); OTLP + the other tracing follow-ons remain ⇒ the sentinel check-(2) STILL prints ⇒ the loop continues.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup

Phase 59 PICKS UP tracing `custom_tags` (scoped to literal) — recorded in the ROADMAP Observability family's LIVE deferred sentence (`OTLP-metrics stats sink + tracing custom_tags/spawn_upstream_span/http_service/force-trace`) since the tracing follow-ons were enumerated. After phase 59 the remaining candidates are: the three non-literal custom-tag types + `spawn_upstream_span`/`http_service`/force-trace + OTLP-metrics + `max_path_tag_length` + `stats_flush_on_admin`. The family STAYS OPEN. **Sentinel maintenance (at the IMPL):** after NARROWING `custom_tags` in the deferred sentence, re-run the check-(2) grep — require EXACTLY ONE live "candidates:" match with the intended content (`reference_sentinel_deferred_sentence_live_vs_historical`).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

- **D-CT-SCOPE** — confirm the LITERAL-only slice (the three other types reject loudly with distinct substrings, §2.4). §2.2.
- **D-CT-LITERAL-WIRE** — how the reference emits a literal custom tag on the OTLP span: key = the `tag` name, value = the `literal.value` string as a STRING `AnyValue`; appended after the built-ins. ONE fresh-container probe against `envoyproxy/envoy:contrib-v1.37.2` (`reference_probe_fresh_container_per_arm`, `reference_envoy_contrib_image_tagging`) with a configured literal `custom_tag`, observed via the `test/helpers/otlptrace` receiver (`reference_docker_probe_bridge_network` for reachability). §2.3.
- **D-CT-ZIPKIN-WIRE** — the same literal tag on the Zipkin span (appears in the Zipkin `tags` map). Probe a Zipkin-provider config. §2.3/§2.6.
- **D-CT-PRECEDENCE** — append vs override when a literal tag's `tag` collides with a built-in attribute key (e.g. `tag: "http.method"`). Anticipated APPEND; probe pins it. §2.3.
- **D-CT-CONFIG-SEAM** — the `TracingConfig` field shape + how the tags thread to `BuildServerSpan` (a new parameter vs `SpanInputs` vs a field). RE-DERIVE the `accesslog_emit.go:55/:116` call sites + `connection.go` `f.tracingConfig` access. §2.5.
- **D-CT-REJECT** — the three unsupported types' exact reject substrings (ADR-0080 distinct) + the empty-tag/typeless handling (envoy-go's own reject vs the reference's PGV boot-reject; RE-DERIVE `CustomTag.tag` PGV `min_len: 1` from `custom_tag.pb.validate.go:61-64`). Confirm (one probe arm) the reference ACCEPTS `request_header`/`environment`/`metadata` (the departure is real). §2.4.
- **D-CT-FIXTURE** — ONE new fixture (OTLP) or TWO (OTLP + Zipkin)? New dir(s) vs extend `0087`/`0088` (the dispatch constraint, `reference_differential_fixture_dispatch_constraint`). Fixtures **103 → 104** (or **105**). Whether a Zipkin fixture grows the row enough to split the fixture leg (ADR-0045). §2.6.
- **D-CT-FUZZSEED** — a SEED to the EXISTING `FuzzHCMConfigParse` (NOT a new fuzzer); confirm the HCM fuzzer reaches the tracing-config parse arm; fuzzer count STAYS 54 (`reference_fuzzer_count_docs_drift` — reconcile before AND after). §2.7.
- **D-CT-SPLIT** — the ADR-0045 disposition (SINGLE FLAT ROW anticipated, ~9–13 tasks; escape-valve armable only if the fixture leg [a Zipkin dir] surprises upward). §1.4.

---

## 11. Prior-phase lessons applied

- **`feedback_brief_citations_not_evidence`** — EVERY `file:line` here (the `config.go` reject/parse sites, `span.go` `BuildServerSpan`, the `accesslog_emit.go` call sites, the `custom_tag.pb.validate.go` PGV line, the fuzzer host) is to be RE-DERIVED from source at the SPEC. (This session re-derived them live against master `29953648`.)
- **`reference_fuzzer_count_docs_drift`** — a SEED, not a fuzzer; reconcile the documented running total (54) against actual `^func Fuzz` before AND after — the count must NOT move. §2.7.
- **`reference_probe_fresh_container_per_arm`** + **`reference_envoy_contrib_image_tagging`** — each SPEC probe arm (D-CT-LITERAL-WIRE / D-CT-ZIPKIN-WIRE / D-CT-PRECEDENCE / D-CT-REJECT) runs on a FRESH container against `envoyproxy/envoy:contrib-v1.37.2`. §10.
- **`reference_docker_probe_bridge_network`** + **`reference_host_gateway_ip_docker_desktop`** — the OTLP/Zipkin span probes need a shared bridge network + a reachable receiver; verify the span decode ACTUALLY ran (not a vacuous empty capture). §10.
- **`reference_differential_fixture_dispatch_constraint`** — a new fixture dir per runner branch; do NOT mutate `0087`/`0088` (they are pure baselines). §2.6.
- **`reference_tracing_upstream_cluster_framework_gap`** — the 4 EMPTY built-in span attributes are a KNOWN framework gap; literal custom tags are static and INDEPENDENT of it (do NOT conflate — a literal tag is populated from config, not the un-plumbed per-request seam). §1.2/§8.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after the IMPL NARROWS `custom_tags` in the deferred sentence, re-run the check-(2) grep; EXACTLY ONE live "candidates:" match, correct content. §9.
- **`reference_strict_reject_sibling_typeurl_gap`** / **ADR-0080** — each of the three unsupported-tag-type rejects carries a DISTINCT substring so a future silent divergence surfaces. §2.4.
- **`reference_fatalf_makes_assertions_unreachable`** — the span-attribute fixture/unit tests assert each independent property with `Errorf` (not `Fatalf`), so a literal-tag failure does not mask the built-in-attribute assertions. §6.
- **Phase-57 final-review deferred fold-ins (F-1/F-2/F-3):** this row does NOT touch `udp.go` or the `0101` stats-sink driver, so F-1/F-2/F-3 are OUT of this row's blast radius; they remain fold-in candidates for the next `udp.go`/statssink-driver-touching row (the OTLP-metrics sink). Recorded so they are not lost. §8.

---

## 12. Section closeout

**Settled:** subject (tracing `custom_tags`, LITERAL-only, SELF-PICKED per the standing directive over four declined larger alternatives, §2.1); scope (literal accepted; `request_header`/`environment`/`metadata` reject loudly with distinct substrings, an envoy-go-strict departure, §2.2/§2.4); the span-emit seam (append to `Span.Attrs` in `BuildServerSpan` — ONE change covers OTLP + Zipkin, §2.3); config home (a `[]KV` field on `TracingConfig`, threaded to `BuildServerSpan` at the two `accesslog_emit.go` call sites, §2.5); fixture posture (anticipated ONE new OTLP fixture, a Zipkin dir weighed, §2.6); fuzz posture (a SEED to `FuzzHCMConfigParse`, no new fuzzer, §2.7); stat surface (+0, §2.8); envelope (SINGLE FLAT ROW anticipated, ~9–13 tasks — ADR-0277, §1.4). The novel production code is the literal-parse + three reject arms in `config.go` and the append loop in `BuildServerSpan`; the row's differential value is the OBSERVABLE literal span attribute (unlike phase 58's parse-reject).

**Anticipated moves at the phase-59 IMPL (docs-only now):** the literal-parse + three reject arms in `NewConfig` + the `TracingConfig` field + the `BuildServerSpan` append + the two call-site threadings + config/span unit tests + a `FuzzHCMConfigParse` seed + the new OTLP fixture (and possibly a Zipkin fixture) + the BEHAVIOR_CONTRACT tracing edit + ADR-0277 + the ROADMAP deferred-sentence narrow. Counts: stat surface **1201 (+0)** · fixtures **103 → 104** (or **105** with a Zipkin dir) · fuzzers **54 (+0, seed only)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0277** (next-free **ADR-0278**) · new Go packages **0** · new go.mod modules **0**.

**Counts UNCHANGED at this BRAINSTORM (docs-only; re-verified against master tip `29953648`):** stat surface **1201** · fixtures **103** · fuzzers **54** · BackendKind **38** · DECISIONS tail **ADR-0276** (next-free **ADR-0277**). Row 59 registers `in-progress` at this BRAINSTORM commit per the §Schema invariant.

**Next → the phase-59 SPEC** (the D-CT-* live-probe arms against `envoyproxy/envoy:contrib-v1.37.2` — D-CT-LITERAL-WIRE / D-CT-ZIPKIN-WIRE / D-CT-PRECEDENCE / D-CT-REJECT; re-derive every §6 edit site + the `custom_tag.pb.validate.go` PGV line; pin D-CT-CONFIG-SEAM + D-CT-FIXTURE; draft ADR-0277 §Context).
