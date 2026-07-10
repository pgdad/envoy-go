# Phase 56 Brainstorm — the HTTP tap filter (`envoy.filters.http.tap`), the NINTH row of the Observability family; a predicate-tree-matched stream recorder that emits a buffered `TraceWrapper` to a `file_per_tap` sink; SPLIT BY CONCERN into 56.1 headers / 56.2 bodies (ADR-0045 gate CONSUMED)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, worktree `.worktrees/phase-56-brainstorm`, branch `phase-56-http-tap-filter-brainstorm` (`feedback_git_worktrees`). Row 56 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated). Row 56 flips `done` only when the FINAL leg (56.2) lands (`reference_roadmap_split_phase_row_done` + ADR-0106: per-leg ADRs, no parent rollup).
>
> **The pick is already made.** The human RE-OPENED the loop and picked **the tap filter** from the Observability family's deferred-candidate list. This BRAINSTORM settles the SCOPE — the trailers boundary, the sink, the trace shape, the match-arm coverage, and the split — not the "which row" question.
>
> **User dialogue (7 pins, 2026-07-09). Record each verbatim as a *(Qn)* pin:**
> - **Q0 (candidate pick)** → the human RE-OPENED the loop and picked **the tap filter** from the Observability family's deferred-candidate list. Options offered and DECLINED: opening the xDS family with a narrow CDS first row; the `graphite` sink; an Operational-tooling admin-API reload/validate endpoint.
> - **Q1 (trailers) → SCOPE TRAILERS OUT; stay framework-zero-touch.** Boot-reject `http_request_trailers_match` / `http_response_trailers_match`; never populate the `trailers` field; record the omission as an explicit `BEHAVIOR_CONTRACT` coverage boundary; defer the HCM "Task 18" trailer plumbing to its own future framework-surgery row. Rejected alternatives: close Task 18 as leg 56.1; close Task 18 as its own phase before tap. **LOAD-BEARING (see §2.7).**
> - **Q2 (output sink) → `file_per_tap` ONLY.** Boot-reject the other four `OutputSink` arms. `streaming_grpc` + the two admin sinks become named deferred candidates.
> - **Q3 (trace shape + format) → BUFFERED trace, JSON format.** Honor `streaming: false` only; boot-reject `streaming: true` (that arm emits `HttpStreamedTraceSegment`, a different message and a different lifecycle). Exactly which of `JSON_BODY_AS_BYTES` / `JSON_BODY_AS_STRING` is a SPEC probe — and at 56.1 they are INDISTINGUISHABLE (no body captured).
> - **Q4 (MatchPredicate arms) → the logical + header arms; reject the body + trailer arms.** Support 6 of 10: `and_match`, `or_match`, `not_match`, `any_match`, `http_request_headers_match`, `http_response_headers_match`. Boot-reject the 2 trailer arms (forced by Q1) and the 2 `generic_body` arms (which would need a search-pattern engine over body bytes).
> - **Q5 (HeaderMatcher) → a NEW shared exported package; MIGRATE NOBODY.** Write one `internal/headermatch`, exported, fully-armed, unit-tested; ONLY tap (via `matchpredicate`) consumes it. Leave `rbac`/`ratelimit`/`oauth2` untouched (zero regression risk), with a documented migration candidate for a future row. Rejected alternatives: extract + migrate all three callers (they disagree on arm coverage — 11 vs 7 vs 7 arm references — so migration means reconciling semantics in three filters unrelated to tap, risking byte-stability); a fifth private copy.
> - **Q6 (ADR-0045 split) → SPLIT BY CONCERN: 56.1 headers / 56.2 bodies.** Rejected alternatives: split by LAYER (56.1 would ship dead library code with no differential surface, against this project's every-row-is-differentially-proven grain); one FLAT row (~25 tasks; phase 55 already TOUCHED the gate at 14). **The ADR-0045 gate is CONSUMED.**

---

## 1. Mission and scope confirmation (56 — a TWO-LEG by-concern split)

### 1.1 What phase 56 delivers as a self-contained whole (the HTTP tap filter)
Implements Envoy's HTTP tap filter `envoy.filters.http.tap` — a per-stream HTTP filter that matches the stream against a **predicate tree** (`config/common/matcher/v3.MatchPredicate`) and, on a match, emits a **buffered trace** (`data/tap/v3.TraceWrapper` carrying an `HttpBufferedTrace`) of what crossed the wire to an output sink. The tap `Tap` message is `{CommonConfig *common/tap/v3.CommonExtensionConfig, RecordHeadersReceivedTime bool, RecordDownstreamConnection bool}` (VERIFIED); its `CommonExtensionConfig` is a oneof `AdminConfig | StaticConfig` (VERIFIED, `common.pb.go:96,101`), and this row honors the `static_config` arm only (a `config/tap/v3.TapConfig`).

The full phase spans two legs, split BY CONCERN *(Q6)*:
- **56.1 `http-tap-filter-headers`** — `internal/headermatch` + `internal/matchpredicate` + `internal/filter/http/tap` + a per-stream `file_per_tap` sink; emits HEADERS-ONLY buffered traces. A `TapConfig` whose predicate references only request/response HEADERS, whose sink is `file_per_tap`, whose trace is buffered JSON.
- **56.2 `http-tap-filter-bodies`** — body capture, `max_buffered_rx_bytes` / `max_buffered_tx_bytes`, `Body.truncated`, and the `JSON_BODY_AS_BYTES` vs `JSON_BODY_AS_STRING` distinction (INDISTINGUISHABLE until a body exists to render — hence its own leg).

### 1.2 What phase 56 does NOT deliver (forward to §8)
The `streaming_grpc` sink; the `streaming_admin` + `buffered_admin` sinks; the `admin_config` (dynamic `POST /tap`) lifecycle; `streaming: true` / `HttpStreamedTraceSegment`; the 2 `generic_body` match arms; the 2 trailer match arms + `Message.trailers`; the tap TRANSPORT SOCKET (the real L4 tap); migrating `rbac`/`ratelimit`/`oauth2` onto the new `internal/headermatch`; the `PROTO_BINARY` / `PROTO_BINARY_LENGTH_DELIMITED` / `PROTO_TEXT` formats; `OutputSink.custom_sink`. Each is named precisely in §8.

### 1.3 Phase-done as the NINTH Observability-family row (family STAYS OPEN)
Row 56 is the NINTH Observability-family row (after gRPC-ALS @ 44, OTLP-log @ 45, tracing @ 46, metrics_service @ 47, statsd @ 48, dog_statsd @ 49, dog_statsd-batching @ 50, statsd-tcp @ 55). The family STAYS OPEN — remaining deferred candidates after this row: the `graphite`/OTLP-metrics sinks, the tracing extras (`custom_tags`/`spawn_upstream_span`/`http_service`/force-trace), and — newly minted here — the tap deferrals of §8 (streaming/admin sinks, generic-body arms, the trailer arms, the tap transport socket). NO parent rollup (ADR-0106); row 56 flips `done` at the **56.2 IMPL** six-gate, not the 56.1 one.

### 1.4 ADR-0045 split readiness — the gate is CONSUMED *(Q6)*
Phase 55 TOUCHED the ADR-0045 gate at 14 tasks and stayed a single flat row; phase 56 CONSUMES it. A flat tap row is ~25 tasks (two new library packages each fully unit-tested, a dual-sided filter, a per-stream file sink, plus the differential), which is past the point where a single row stays reviewable. The split is BY CONCERN, deliberately NOT by layer:
- **Rejected — split by LAYER (56.1 library / 56.2 filter+differential):** 56.1 would land `internal/headermatch` + `internal/matchpredicate` as dead library code with NO differential surface at its close — precisely the shape ADR-0080 exists to prevent (every row is differentially proven). A header-only tap, by contrast, exercises the whole library stack against a live reference at 56.1's close.
- **Rejected — one FLAT row:** ~25 tasks; phase 55 already touched the 14-task gate, and tap is strictly larger.
- **Chosen — split by CONCERN (56.1 headers / 56.2 bodies):** each leg has an end-to-end differential surface (`0099-http-tap-headers`, `0100-http-tap-bodies`). 56.1 proves the predicate engine + the buffered-trace lifecycle + the file sink over a headers-only trace; 56.2 adds only the body-capture concern (buffering caps, truncation, the two JSON body renderings) on top of a proven spine. The `JSON_BODY_AS_BYTES` vs `JSON_BODY_AS_STRING` distinction is INVISIBLE without a body, which is itself the argument for deferring it to 56.2 rather than splitting by layer.

### 1.5 Seed-stub alignment + package placement
NEW packages anticipated (ANTICIPATED — the SPEC/PLAN confirm placement): `internal/headermatch` (a shared exported HeaderMatcher evaluator, *(Q5)*), `internal/matchpredicate` (the `MatchPredicate`-tree compiler+evaluator), and the filter package `internal/filter/http/tap`. The `file_per_tap` sink's placement is a **SPEC/PLAN question, DO NOT SETTLE (D-TAP-SINKPLACEMENT):** candidates are inside the `internal/filter/http/tap` package, or a sibling `internal/tapsink`. This BRAINSTORM does not decide it.

### 1.6 No prebrainstorm-notes branch
No off-master prebrainstorm-notes branch exists for this row.

### 1.7 Phase 56's relationship to the existing seams
REUSES, UNCHANGED: the HTTP filter interface `internal/filter/http/types.go` (`StreamDecoderFilter` types.go:54-60, `StreamEncoderFilter` types.go:65-71, `HTTPFilter{Name,Decoder,Encoder}` types.go:77-81 — tap populates BOTH sides); the two-step ADR-0071 factory (`HTTPFilterFactory` + `FilterInstanceFactory`, types.go:245-251) with its `FactoryCtx{Registry, Stats, StatPrefix, ClusterManager}`; the registration seam `HTTPRegistry.Register(typeURL, HTTPFilterFactory)` (registry.go:35), bulk-registered in `internal/filter/http/builtins/builtins.go:42-63` then `Freeze`d by main.go; the `DecoderFilterCallbacks` connection accessors (`internal/filter/http/callbacks.go`) for `downstream_connection`; the async-file-sink SHAPE from `internal/accesslog` (a *shape* precedent only — see §3.4).

NEW: `internal/headermatch` (exported HeaderMatcher, *(Q5)*); `internal/matchpredicate` (the predicate-tree compiler+evaluator); `internal/filter/http/tap` (the filter); a `file_per_tap` sink (placement DEFERRED, §1.5); the `builtins.go` registration arm; the `0099-http-tap-headers` (56.1) and `0100-http-tap-bodies` (56.2) fixtures.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the HTTP tap filter *(Q0 → phase 56 row registered)*
The loop RE-OPENED and the human picked the tap filter over three declined alternatives (a narrow CDS xDS-family opener; the `graphite` sink; an admin-API reload/validate endpoint). Tap is the first Observability row that is a per-listener HTTP FILTER rather than a bootstrap-level `stats_sinks[]`/tracing surface — a structurally different integration point (§4).

### 2.2 Trace shape + format: BUFFERED trace, JSON *(Q3)*
Honor `OutputConfig.streaming == false` only, emitting an `HttpBufferedTrace` inside a `TraceWrapper` (wrapper.pb.go:27, VERIFIED). Boot-reject `streaming: true` — that arm emits `HttpStreamedTraceSegment` (a DIFFERENT `TraceWrapper` oneof arm, a different message, and a different per-segment lifecycle). The buffered `HttpBufferedTrace{Request *Message, Response *Message, DownstreamConnection *Connection}` (http.pb.go:33-37, VERIFIED) with `Message{Headers []*core/v3.HeaderValue (f1), Body *Body (f2), Trailers []*core/v3.HeaderValue (f3), HeadersReceivedTime *timestamppb.Timestamp (f4)}` is what 56 populates — at 56.1, `Headers` (and, per `Tap.record_headers_received_time`, optionally `HeadersReceivedTime`) only; NEVER `Trailers` (§2.7); `Body` is a 56.2 concern.

The output FORMAT is JSON, one of `JSON_BODY_AS_BYTES`=0 / `JSON_BODY_AS_STRING`=1 (the exact `OutputSink_Format` enum: `JSON_BODY_AS_BYTES`=0, `JSON_BODY_AS_STRING`=1, `PROTO_BINARY`=2, `PROTO_BINARY_LENGTH_DELIMITED`=3, `PROTO_TEXT`=4, VERIFIED `common.pb.go:43-64`). **Which JSON variant 56.1 must accept/emit is a SPEC probe (D-TAP-JSON), and at 56.1 the two are INDISTINGUISHABLE** because no `Body` is captured — the `body_type` oneof that the two formats select between is never populated. 56.2 is where the distinction becomes observable. The PROTO_* formats are DEFERRED (§8).

### 2.3 Output sink: `file_per_tap` ONLY *(Q2)*
Honor `OutputSink.file_per_tap` only. `FilePerTapSink` has exactly ONE field, `PathPrefix string` (VERIFIED), doc-commented "The file per tap sink outputs a discrete file for every tapped stream." Boot-reject the other four `OutputSink` oneof arms — `StreamingAdmin`, `StreamingGrpc`, `BufferedAdmin`, `CustomSink` (VERIFIED the five-arm oneof). Per `reference_strict_reject_sibling_typeurl_gap`, lifting the ONE `file_per_tap` arm requires an EXPLICIT per-sibling reject for each of the other four, not a silent ignore/default arm. `streaming_grpc` + the two admin sinks + `custom_sink` become named deferred candidates (§8).

`OutputConfig.sinks` is a REPEATED field (`OutputConfig{Sinks []*OutputSink, MaxBufferedRxBytes, MaxBufferedTxBytes, Streaming bool}`, VERIFIED). Whether the reference accepts >1 sink is a SPEC probe (D-TAP-NOSINK / D-TAP-MULTISINK) — Envoy historically allows exactly one; envoy-go rejects what it does not honor.

### 2.4 MatchPredicate arms: the logical + header arms *(Q4)*
`MatchPredicate` has exactly 10 oneof arms (VERIFIED, matcher.pb.go:268-318). Support 6:
- `and_match`, `or_match` — both `*MatchPredicate_MatchSet` (matcher.pb.go:1071);
- `not_match` — `*MatchPredicate`;
- `any_match` — `bool`;
- `http_request_headers_match`, `http_response_headers_match` — both `*HttpHeadersMatch`, whose single field is `Headers []*route/v3.HeaderMatcher` (VERIFIED).

Boot-reject the other 4:
- `http_request_trailers_match`, `http_response_trailers_match` — FORCED by Q1 (§2.7); envoy-go cannot observe trailers.
- `http_request_generic_body_match`, `http_response_generic_body_match` — both `*HttpGenericBodyMatch`; would need a body search-pattern engine with `bytes_limit` semantics (DEFERRED, §8).

Each of the 4 rejects is an EXPLICIT per-arm reject (`reference_strict_reject_sibling_typeurl_gap`), not a fall-through. This is compiled at config time (see §2.5's precedent posture).

### 2.5 The MatchPredicate compiler: a NEW `internal/matchpredicate` package, `internal/matcher` is precedent-ONLY *(self-answered)*
No `MatchPredicate` evaluator exists in the tree: `grep -rniE "MatchPredicate|config/common/matcher" internal/ cmd/` → ZERO hits (VERIFIED this session). The existing `internal/matcher` evaluates a DIFFERENT proto — `xds.type.matcher.v3.Matcher` (doc.go:1, `New` matcher.go:115, `Evaluate` matcher.go:139) — and is NOT reusable; it is a DESIGN PRECEDENT ONLY for the shape "compile-at-config-time, then `Evaluate(ctx)` at request time." The new `internal/matchpredicate` compiles a `MatchPredicate` proto tree into an evaluable node tree once at config time (rejecting the 4 unsupported arms during compile) and evaluates it incrementally as stream events arrive (§2.6). It consumes `internal/headermatch` (§2.9) for the two header arms.

### 2.6 The end-of-stream-artifact model — a buffered trace is emitted at stream end, only once the predicate resolves *(self-answered; LOAD-BEARING — the single most likely implementation error)*
**`http_response_headers_match` CANNOT be decided at decode time.** The tap match is a predicate tree evaluated INCREMENTALLY as stream events arrive; a buffered trace is therefore inherently an **END-OF-STREAM ARTIFACT**. The filter must accumulate request headers (at `DecodeHeaders`) and response headers (at `EncodeHeaders`) into a pending `HttpBufferedTrace`, and emit the assembled `TraceWrapper` to the sink at STREAM END (`OnDestroy` / the final encode event) **only once the predicate resolves to a MATCH**. Emitting at `DecodeHeaders` is WRONG — it cannot have seen the response headers a `http_response_headers_match` arm requires, and it cannot know whether the whole-tree predicate matched.

Concretely, the filter's dual-sided obligations:
- `DecodeHeaders(http.Header, bool)` — capture the request headers into the pending trace's `Request.Headers`; feed the request-headers evaluation of the predicate tree; do NOT emit.
- `EncodeHeaders(http.Header, bool)` — capture the response headers into `Response.Headers`; feed the response-headers evaluation; do NOT emit.
- `OnDestroy` (and/or the final encode event) — resolve the predicate; if it MATCHED, serialize the assembled `TraceWrapper` and hand it to the sink; if it did NOT match, emit NOTHING (no file written). Whether a file is written at all on a non-match is D-TAP-EMIT-TIMING.

This is why the filter populates BOTH `Decoder` and `Encoder` sides of `HTTPFilter` (types.go:77-81) even though at 56.1 it mutates no bytes: it is an OBSERVER on both directions whose EMIT decision is deferred to the latest point at which the predicate can flip. The SPEC pins the exact emit site and the non-match file behavior empirically (D-TAP-EMIT-TIMING); D-TAP-RESPMATCH pins whether a response-only predicate still populates `Request.headers` (i.e. is the emitted trace the WHOLE stream, or only the matched half — this governs what the differential compares).

**Corollary — the tri-state predicate.** For `not_match` / `and_match` over a response arm, a node can flip from "undetermined" to "no-match" only AFTER the response headers arrive: e.g. `not_match(http_response_headers_match(...))` is UNDETERMINED at decode time (the response has not been seen) and resolves only at encode. So the compiled tree needs a **tri-state per node — match / no-match / undetermined — not a bool.** A request-only subtree may resolve early; a subtree containing any response arm cannot resolve before `EncodeHeaders`. Whether short-circuit evaluation (an `or_match` with one already-true request arm) may emit BEFORE the response is a mechanism nuance for the SPEC/PLAN — **flag it, DO NOT SETTLE it here.** The safe default (resolve everything at stream end) is always correct; early emission is an optimization the PLAN may or may not take.

### 2.7 Trailers are structurally invisible to envoy-go's HTTP filters — the never-done HCM "Task 18" *(Q1; LOAD-BEARING)*
**THE LOAD-BEARING GAP.** envoy-go's HTTP filters CANNOT observe trailers. `FilterChain.RunDecodeTrailers` (chain.go:454) and `RunEncodeTrailers` (chain.go:621) EXIST but have **ZERO non-test callers anywhere in `internal/` or `cmd/`** (grep-verified this session). The dispatch sites say so in as many words:
- `internal/filter/hcm/connection.go:565` — "the FilterChain does not yet expose a `RunDecodeTrailers` method (Task 18 will add it for the cors/envoygotest filters). For Task 15 we DO NOT branch on `req.Trailer`".
- `internal/filter/hcm/h2dispatch.go:501-503` — "`RunDecodeTrailers` is not invoked: SPEC §2.1 observes-and-discards request trailers in the codec layer (per ADR-0058)".
- Response trailers are handled NOWHERE in `internal/filter/hcm/`.

"Task 18" was written during phase 04 and NEVER happened. **Tap is the first feature whose very purpose collides with it** — a `Message.trailers` field exists in the proto (`HttpBufferedTrace_Message.Trailers []*core/v3.HeaderValue (f3)`, VERIFIED) and two match arms (`http_request_trailers_match` / `http_response_trailers_match`) key on trailers, but envoy-go has no seam through which a filter can see a trailer.

The decision *(Q1)*, mirroring the `reference_close_direction_framework_gap` posture exactly:
1. **Boot-reject** `http_request_trailers_match` / `http_response_trailers_match` (the two rejected `MatchPredicate` arms of §2.4). An explicit per-arm reject.
2. **NEVER populate** the `Message.trailers` field on the emitted trace.
3. **Stay framework-zero-touch** — do NOT attempt Task 18 in this row.
4. **Record the omission as an explicit `BEHAVIOR_CONTRACT` coverage boundary** at the SPEC/IMPL — a named gap, not a silent one.
5. **Defer the HCM "Task 18" trailer plumbing to its own future framework-surgery row** (§8), proposed as a standalone phase.

Rejected alternatives *(recorded for the SPEC's re-check)*: (a) close Task 18 AS leg 56.1 — bundles unbounded framework surgery (both dispatch paths, H1 and H2, decode and encode) into an Observability feature row, against the every-row-is-differentially-proven grain and blowing the leg's scope; (b) close Task 18 as its OWN phase BEFORE tap — front-loads framework surgery the human did not ask for and delays the picked feature behind a large unrelated prerequisite. The chosen posture ships a genuinely useful headers-tap now and names the trailer capability as bounded future debt.

### 2.8 HeaderMatcher: a NEW shared exported `internal/headermatch`, MIGRATE NOBODY *(Q5)*
There are THREE divergent UNEXPORTED HeaderMatcher evaluators today, verified by counting `HeaderMatcher_*` arm references: `internal/rbac/evaluator.go:855` (11 arms), `internal/filter/http/ratelimit/descriptors.go:785` (7 arms), `internal/filter/http/oauth2/compiled_config.go:864` (7 arms) — also used in `csrf`, `fault`, `hcm/config.go`. NONE is shared or exported (VERIFIED). The decision: write ONE new `internal/headermatch`, **exported, fully-armed, unit-tested**, consumed ONLY by tap (via `internal/matchpredicate`). Leave `rbac`/`ratelimit`/`oauth2` UNTOUCHED — zero regression risk to three filters unrelated to tap.

Rejected alternatives: (a) extract + MIGRATE all three callers — they DISAGREE on arm coverage (11 vs 7 vs 7 arm references), so migration means reconciling semantics across three filters whose byte-stability is already differentially proven, risking regressions in code this row has no business touching; (b) a fifth PRIVATE copy — adds a fourth divergent evaluator, compounding the very debt Q5 names. The chosen path leaves a DOCUMENTED migration candidate (§8): a future row may migrate the three private evaluators onto `internal/headermatch` once its arm coverage is proven a superset — but that is explicitly NOT this row.

### 2.9 Connection info for `downstream_connection`: the landed `DecoderFilterCallbacks` accessors *(self-answered; SPEC pins which fields, D-TAP-CONN)*
`HttpBufferedTrace.downstream_connection` (a `data/tap/v3.Connection`, common.pb.go:127) is populated — WHEN `Tap.record_downstream_connection` is set — from the connection accessors ALREADY on `DecoderFilterCallbacks` (`internal/filter/http/callbacks.go`): `DownstreamRemoteAddr()` (:103), `DownstreamLocalAddr()` (:113), `DownstreamTLSServerName()` (:211), `DownstreamTLSPeerCertDER()` (:223), `DownstreamTLSConnectionState()` (:269), `DownstreamProtocol()` (:233), all seeded once at chain-build time. WHICH `Connection` proto fields the reference actually populates, and the precise semantics of `record_downstream_connection` + `record_headers_received_time`, are a SPEC probe (D-TAP-CONN). Per `reference_tracing_upstream_cluster_framework_gap`: if envoy-go cannot plumb a field, UNassert it cross-side rather than emitting a wrong value.

### 2.10 The deprecated `match_config` vs `match`: NOT SETTLED — a SPEC probe *(D-TAP-DEPRECATED)*
`TapConfig` carries BOTH `MatchConfig *tapv3.MatchPredicate` (field 1, package-local `config/tap/v3` type, DEPRECATED) and `Match *commonmatcherv3.MatchPredicate` (field 4, the `config/common/matcher/v3` type this row compiles) (VERIFIED, common.pb.go:125,131). Does the reference accept the deprecated `match_config`? What happens when BOTH are set (reject? precedence?)? envoy-go must be byte-stable either way (ADR-0080). **DO NOT SETTLE** — D-TAP-DEPRECATED pins it. NOTE `reference_strict_reject_sibling_typeurl_gap`: whichever way it resolves, each unhandled combination needs an explicit arm, not a silent default.

### 2.11 Stat surface: a SPEC probe, no guess *(D-TAP-STATS)*
A filter registers stats via `FactoryCtx.Stats` + `FactoryCtx.StatPrefix`, then `reg.NewCounter(name)` (`internal/stats/registry.go:84`), naming-convention `http.<stat_prefix>.<metric>` (the `fault` precedent, `internal/filter/http/fault/fault.go:238-246`). What stats the reference tap filter emits and under what names (`rq_tapped`? — anticipated, NOT verified) is D-TAP-STATS. envoy-go registers ONLY what it can honor. **DO NOT GUESS a stat name or a surface number.** Also confirm `reference_dynamic_stat_name_charset_guard` applicability: does tap derive any stat name from WIRE BYTES? Anticipated NO (stat names are config-static, not derived from matched headers) — say so explicitly rather than omitting the check; if the SPEC finds otherwise, the `stats.IsValidName`-guard-before-`NewCounterIfAbsent` discipline applies.

---

## 3. Framework-survey result — 2 new packages, 0 new go.mod modules

### 3.1 Framework: no new framework piece; a dual-sided observer filter over the LANDED HTTP filter seam
No new dispatch layer, no new callback surface. Tap is the 20th production HTTP filter (ANTICIPATED, §3.5), plugging into the existing `StreamDecoderFilter` + `StreamEncoderFilter` interfaces (types.go:54-60, 65-71) via the two-step ADR-0071 factory (types.go:245-251) and the `builtins.go:42-63` registration seam. It is the first filter whose emit decision is deferred to stream end across BOTH directions (§2.6) — a usage pattern, not a new framework primitive.

### 3.2 NEW packages: anticipated TWO (+ the filter package + the sink placement)
- `internal/headermatch` — the shared exported HeaderMatcher evaluator *(Q5)*.
- `internal/matchpredicate` — the `MatchPredicate`-tree compiler+evaluator (§2.5), consuming `internal/headermatch`.
- `internal/filter/http/tap` — the filter itself.
- The `file_per_tap` sink — placement a SPEC/PLAN question (D-TAP-SINKPLACEMENT): inside `internal/filter/http/tap`, or a sibling `internal/tapsink`. **DO NOT SETTLE.**

Anticipated NEW-package count: **2** (`internal/headermatch`, `internal/matchpredicate`) plus the filter package, plus wherever the sink lands. ANTICIPATED — the SPEC confirms.

### 3.3 go.mod modules: anticipated ZERO new
All protos are already in the `github.com/envoyproxy/go-control-plane/envoy v1.32.4` module the tree already depends on: `filters/http/tap/v3`, `common/tap/v3`, `config/tap/v3`, `config/common/matcher/v3`, `data/tap/v3`, and (for the DEFERRED gRPC sink) `service/tap/v3` are all PRESENT (VERIFIED). `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES
The HTTP filter interface + two-step factory + registration seam (§3.1); the `DecoderFilterCallbacks` connection accessors (§2.9); `internal/matcher` as a **compile-then-`Evaluate` SHAPE precedent ONLY** (§2.5 — it evaluates a different proto, NOT reusable); the async-file-sink SHAPE from `internal/accesslog` — `accesslog.go:18` `Sink{Submit(any); Close() error}` and `writer.go:26` `type AsyncFileSink` (constructor `NewAsyncFileSink` at writer.go:41; bounded channel, drop-on-full, background `run()`, `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0o644)` at writer.go:56).

### 3.5 What is NOT reused (say so explicitly)
- **`AsyncFileSink` is a SHAPE precedent, NOT directly reusable.** `AsyncFileSink` opens ONE append-only file for the process lifetime; `file_per_tap` writes **a DISCRETE FILE PER TAPPED STREAM** (VERIFIED doc comment). The channel + background-drain + drop-on-full shape transfers; the single-file `os.OpenFile` at construction does not. The tap sink opens a NEW file per emitted trace, named by the D-TAP-FILENAME scheme.
- **`internal/matcher` is a design precedent, not code reuse** (§2.5): different proto, different arms.
- **The three private HeaderMatcher evaluators are NOT reused** (§2.8): `internal/headermatch` is written fresh, exported, from the proto — not extracted from `rbac`/`ratelimit`/`oauth2`.
- **`internal/statssink/sink.go:18` + `internal/grpcclient/grpcclient.go:82`** are relevant ONLY to the DEFERRED `streaming_grpc` sink; not touched this row.
- **`EncoderFilterCallbacks.BufferEncodedBody()` (callbacks.go:350) + the decode-side self-accumulate pattern** (`ext_authz`: `f.body = append(f.body, data...)`, extauthz.go:1278, returning `DataStopIterationAndBuffer` at endStream, extauthz.go:1317) are a **56.2 concern, NOT 56.1** — body is observable (`DecodeData`/`EncodeData` receive raw chunks; HCM streams them at connection.go:637 and h2dispatch.go:540 on the DECODE side, connection.go:743 and h2dispatch.go:582 on the ENCODE side; there is NO `BufferDecodedBody()` on `DecoderFilterCallbacks`, so decode-side consumers self-accumulate), but 56.1 captures headers only.

---

## 4. Bootstrap-level applicability — a per-listener HCM `http_filters[]` entry, NOT a bootstrap `stats_sinks[]` surface
Unlike phases 47-55 (bootstrap-level `stats_sinks[]`), tap is a **per-listener HTTP filter**: a `Tap` message wrapped in an `http_filters[]` entry inside an HCM filter chain, TypeURL `envoy.filters.http.tap`, dispatched through the standard two-step ADR-0071 factory (`HTTPFilterFactory func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error)` → `FilterInstanceFactory func() HTTPFilter`, types.go:245-251) and registered in `builtins.go:42-63`. This is the first Observability-family row to integrate at the HTTP-filter layer rather than the bootstrap-sink layer — the differential drives real HTTP requests through the filter, not a periodic flush loop. The `FactoryCtx` supplies `Stats`, `StatPrefix`, `Registry`, and `ClusterManager` at config time; the tap instance is minted per-stream by the returned `FilterInstanceFactory`.

---

## 5. Stat surface hypothesis — SPEC pins (D-TAP-STATS)

### 5.1 Stat names (SPEC pins, D-TAP-STATS)
ANTICIPATED unknown. The reference likely emits at least one counter (a `rq_tapped`-shaped counter is a natural guess but NOT verified). **DO NOT GUESS the name or the count.** envoy-go registers only what it can honor via `FactoryCtx.Stats` + `StatPrefix` (§2.11). D-TAP-STATS pins the exact names live.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)
Every reject arm is an envoy-go-strict boundary that the SPEC must pin against the reference and the IMPL must implement as an EXPLICIT reject (`reference_strict_reject_sibling_typeurl_gap`):
- The 4 unsupported `MatchPredicate` arms (2 trailer, 2 generic_body) — §2.4.
- The 4 non-`file_per_tap` `OutputSink` arms (`streaming_admin`, `streaming_grpc`, `buffered_admin`, `custom_sink`) — §2.3.
- `streaming: true` — §2.2.
- The `admin_config` arm of `CommonExtensionConfig` (only `static_config` honored) — §1.1.
- The PROTO_* formats — §2.2.
- Whatever D-TAP-DEPRECATED (§2.10), D-TAP-NOSINK/MULTISINK (§2.3) resolve to.

### 5.3 Anticipated surface arithmetic
**SPEC PROBE (D-TAP-STATS). DO NOT GUESS a number.** The stat surface baseline is 1200 (RE-VERIFIED this session, §baselines); the delta is whatever the honored-stats subset works out to, pinned live.

---

## 6. Differential fixture envelope — `0099-http-tap-headers` (56.1), `0100-http-tap-bodies` (56.2)

### 6.1 Fixtures
- **56.1 → `0099-http-tap-headers`** (fixtures 100 → 101). A listener with an HCM whose `http_filters[]` carries a `Tap` matching on request+response HEADERS, a `file_per_tap` sink writing to a `path_prefix` under a driver-owned temp dir, buffered JSON. The driver drives HTTP requests, then GLOBS the sink directory and decodes the emitted `TraceWrapper`(s).
- **56.2 → `0100-http-tap-bodies`** (fixtures 101 → 102). Adds body capture, `max_buffered_rx_bytes`/`max_buffered_tx_bytes`, `Body.truncated`, and a `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` discriminating assertion.

### 6.2 The glob-and-compare-decoded-traces design
Per `reference_streaming_sink_differential_framing`, assert the **PAYLOAD** (the decoded trace set — the `HttpBufferedTrace` headers/body content), NEVER the framing or the FILENAMES. The `file_per_tap` filename embeds a proxy-internal id (D-TAP-FILENAME: anticipated `<path_prefix>_<some id>.<ext>`), so **filenames will NOT agree cross-side** — the fixture globs `<path_prefix>*` and compares the decoded content, not the paths. Per `reference_stats_sink_emits_used_only` + `reference_differential_reference_parses_full_message`, the two sides' emitted sets differ structurally (fields envoy-go cannot plumb, per D-TAP-CONN / `reference_tracing_upstream_cluster_framework_gap`); assert a NAMED SUBSET of trace fields, decoded field-by-field (D-TAP-JSON pins the exact protojson rendering — camelCase vs original, empties omitted or not, the concrete `core/v3.HeaderValue` shape — which governs whether a field-by-field decode-then-compare is even feasible). D-TAP-RESPMATCH governs whether a response-only-predicate trace still carries `request.headers` — which decides what the differential can compare.

### 6.3 Deliberate breaks (≥3 per leg, each `-count=1`)
Per `reference_differential_break_protocol_count1`, every deliberate-break run is `-count=1` (go-test caching serves a stale PASS otherwise). Per `reference_deliberate_break_wrong_assertion`, a break FAILING is NOT proof the intended assertion is live — confirm WHICH assertion fired and add an isolating break for any masked one; per `reference_fatalf_makes_assertions_unreachable`, use `t.Errorf` per independent property so a failed (ii) does not leave (iii)+ unreachable. Anticipated 56.1 breaks (the SPEC finalizes them):
- **(a) Emit at `DecodeHeaders` instead of at stream end** (the §2.6 error): a trace missing `response.headers` (or a wrong/absent match on a response-arm predicate) — proves the end-of-stream-artifact model is LIVE.
- **(b) Treat an unmatched stream as a match (predicate always-true):** a spurious trace file appears where none should — proves the predicate gates emission.
- **(c) Populate a rejected arm / accept a rejected sink instead of booting a reject:** boot succeeds where it must fail — proves the strict-reject arms are LIVE.

Anticipated 56.2 breaks: truncation-flag correctness, the buffering-cap boundary, and the `JSON_BODY_AS_BYTES`-vs-`STRING` rendering — finalized at the 56.2 SPEC.

Differential discipline carries: `-run 'TestDifferential/0099-http-tap-headers'` NEVER bare `0099` (`reference_differential_run_selector` — a bare selector matches ZERO subtests → vacuous green); Docker bridge network for the reference (`reference_docker_probe_bridge_network`); a `subject ready: EOF` on an UNRELATED fixture in a full-suite run is the known startup flake, isolate-re-run to discriminate (`reference_differential_fullsuite_startup_flake`).

### 6.4 BackendCount +0
BackendKind stays 38 (ANTICIPATED). The tap output is a FILE the driver reads — no new backend kind. The fixture is otherwise driver-owned, so it still needs `BackendCount() >= 1` (`reference_differential_backendcount_min_one`): a plain HTTP backend for the proxied request satisfies it (the `0018-http-rbac` precedent for an all-driver-owned fixture).

### 6.5 New fuzzer: a SPEC probe (D-TAP-FUZZER)
Fuzzers baseline 52 (RE-VERIFIED). A `MatchPredicate`-tree PARSE fuzzer is a STRONG candidate — a recursive proto (`and_match`/`or_match`/`not_match` nest arbitrarily) compiled by a recursive `internal/matchpredicate` compiler is exactly the parse-a-recursive-structure shape prior rows fuzzed — **but DO NOT COMMIT.** D-TAP-FUZZER pins it at the SPEC. NOTE `reference_fuzzer_count_docs_drift`: the running total has been off-by-one before — the IMPL reconciles `grep -c '^func Fuzz'` against the doc, not the doc against itself.

---

## 7. Anticipated ADRs — ADR-0273 (56.1), ADR-0274 (56.2)
Per ADR-0106 (per-leg ADRs, no parent rollup) and ADR-0044 (§Context at SPEC, §Decision/§Consequences at IMPL):
- **ADR-0273** at the 56.1 IMPL — the HTTP tap filter headers leg: the `internal/headermatch` + `internal/matchpredicate` + `internal/filter/http/tap` stack, the `file_per_tap` per-stream sink, the end-of-stream-artifact / tri-state-predicate lifecycle (§2.6), the trailers coverage boundary (§2.7), and the strict-reject arm set (§5.2).
- **ADR-0274** at the 56.2 IMPL — the body-capture leg: `max_buffered_rx_bytes`/`max_buffered_tx_bytes`, `Body.truncated`, the `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` distinction.

next-free DECISIONS tail is ADR-0272 → next-free **ADR-0273** (RE-VERIFIED, §baselines). Whether a separate SEAM ADR is warranted for `internal/headermatch`/`internal/matchpredicate` (new exported shared packages) is a SPEC/PLAN call — flagged, not settled.

---

## 8. Deferred items (named precisely for a future session)
- **The `streaming_grpc` sink** — `envoy.service.tap.v3.TapSinkService`/`StreamTaps` is CONFIRMED PRESENT in the module (tap_grpc.pb.go:22,28), so a future row need not re-probe availability. Uses `internal/statssink/sink.go` + `internal/grpcclient` shapes.
- **The `streaming_admin` + `buffered_admin` sinks** — BLOCKED on `internal/admin` gaining chunked/flush semantics. Every handler in `internal/admin/admin.go:91-99` is fully buffered today; there is NO `http.Flusher` use anywhere in the tree. A concrete prerequisite, not a vague one.
- **`admin_config`** (the dynamic `POST /tap` lifecycle, the `AdminConfig` arm of `CommonExtensionConfig`) — BLOCKED on the same admin-streaming work.
- **`streaming: true` / `HttpStreamedTraceSegment`** — a different `TraceWrapper` oneof arm and a different per-segment lifecycle (§2.2).
- **The 2 `generic_body` match arms** (`http_request_generic_body_match` / `http_response_generic_body_match`, `*HttpGenericBodyMatch`) — need a body search-pattern engine with `bytes_limit` semantics.
- **The 2 trailer match arms + `Message.trailers`** — BLOCKED on **HCM "Task 18"** trailer plumbing (`connection.go:565`, `h2dispatch.go:501-503`; response trailers handled nowhere). **Proposed as its own future FRAMEWORK-SURGERY row** — the first feature to need it (§2.7).
- **The tap TRANSPORT SOCKET** (`envoy/extensions/transport_sockets/tap`, PRESENT) — the REAL L4 tap. There is **no network tap FILTER in Envoy at all** (`envoy/extensions/filters/network/tap/v3` is ABSENT, and correctly so); L4 tap is a transport socket. This scopes L4 tap out BY CONSTRUCTION, not by choice.
- **Migrating `rbac`/`ratelimit`/`oauth2` onto `internal/headermatch`** — Q5's documented debt (§2.8); a future row once `internal/headermatch`'s arm coverage is proven a superset of all three.
- **`PROTO_BINARY` / `PROTO_BINARY_LENGTH_DELIMITED` / `PROTO_TEXT` formats** (`OutputSink_Format` 2/3/4) — JSON-only this row.
- **`OutputSink.custom_sink`** — the extension-point sink arm.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup
The tap filter has been named in the Observability family's deferred-candidate sentence since the family opened at phase 44, and appears explicitly in the phase-55 BRAINSTORM's deferred list ("the tap filter", `55-stats-sink-statsd-tcp/BRAINSTORM.md:33,266`) and its §12 closeout ("the tap filter remain deferred", :324). Phase 56 PICKS IT UP as the NINTH Observability row *(Q0)*. The remaining Observability deferred candidates (`graphite`/OTLP-metrics sinks; the tracing extras `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace) carry forward UNbrainstormed, joined now by this row's own §8 tap deferrals (the streaming/admin sinks, the generic-body arms, the trailer arms, the tap transport socket, the HeaderMatcher migration).

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against `envoyproxy/envoy:contrib-v1.37.2` per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network`, a fresh probe container per arm per `feedback_probe_fresh_container_per_arm`)
- **D-TAP-STATS** — what stats does the reference tap filter emit, under what names (`rq_tapped`?)? Determines the stat-surface delta (§5). envoy-go registers only what it can honor.
- **D-TAP-FILENAME** — the exact `file_per_tap` filename scheme and extension per `Format`. Anticipated `<path_prefix>_<some id>.<ext>`; the id is proxy-internal, so filenames will NOT agree cross-side. Pin the scheme so the fixture globs correctly (§6.2).
- **D-TAP-JSON** — the exact protojson rendering: field naming (camelCase vs original), whether empty/zero fields are omitted, and the concrete `core/v3.HeaderValue` shape. Determines whether the differential can compare decoded traces field-by-field (§6.2).
- **D-TAP-DEPRECATED** — does the reference accept the deprecated `TapConfig.match_config` (field 1)? What when BOTH `match_config` and `match` are set (reject? precedence?)? envoy-go must be byte-stable either way (ADR-0080). NOTE `reference_strict_reject_sibling_typeurl_gap` (§2.10).
- **D-TAP-EMIT-TIMING** — when EXACTLY is the trace emitted, and is a file written at all when the predicate does NOT match? Confirms the end-of-stream-artifact model (§2.6).
- **D-TAP-RESPMATCH** — with a predicate over response headers only, does the reference still populate `request.headers` in the emitted trace? (The whole stream, or only the matched half?) Governs the differential's comparison set (§6.2).
- **D-TAP-CONN** — the precise semantics of `Tap.record_downstream_connection` and `Tap.record_headers_received_time`; which `Connection` fields the reference actually populates. Cross-ref `reference_tracing_upstream_cluster_framework_gap`: UNassert any field envoy-go cannot plumb (§2.9).
- **D-TAP-NOSINK / D-TAP-MULTISINK** — `OutputConfig.sinks` is REPEATED. Does the reference accept >1 sink (or exactly one, historically)? envoy-go rejects what it does not honor (§2.3).
- **D-TAP-FUZZER** — does the recursive `MatchPredicate`-tree parse warrant a dedicated fuzzer (§6.5)? Reconcile the count against source per `reference_fuzzer_count_docs_drift`.
- **D-TAP-SINKPLACEMENT (design, not empirical)** — package placement for the file sink (inside `internal/filter/http/tap` vs a sibling `internal/tapsink`). Resolve at SPEC/PLAN (§1.5).

---

## 11. Prior-phase lessons applied
- `reference_streaming_sink_differential_framing` — assert the PAYLOAD (the decoded trace set), NEVER the framing/filenames (§6.2). The sharpest one for this row's differential shape.
- `reference_stats_sink_emits_used_only` + `reference_differential_reference_parses_full_message` — the two sides' emitted SETS differ structurally; assert NAMED SUBSETS of trace fields, not whole-set equality (§6.2).
- `reference_deliberate_break_wrong_assertion` — a break failing is NOT evidence the intended assertion is live; confirm WHICH assertion fired, add an isolating break for any masked one (§6.3).
- `reference_fatalf_makes_assertions_unreachable` — `t.Errorf` per independent property; `t.Fatalf` only for a broken harness precondition (§6.3).
- `reference_differential_backendcount_min_one` — a file-output/driver-owned fixture still needs `BackendCount() >= 1` (§6.4).
- `reference_differential_run_selector` — `-run 'TestDifferential/0099-http-tap-headers'`, never bare `0099` (§6.3).
- `reference_differential_break_protocol_count1` — deliberate-break runs ALWAYS `-count=1` (§6.3).
- `reference_close_direction_framework_gap` — the PRECEDENT for Q1: a framework gap is named as a COVERAGE BOUNDARY and deferred to a framework-surgery row, and the affected row stays framework-zero-touch (§2.7).
- `reference_dynamic_stat_name_charset_guard` — confirm applicability: does tap derive any stat name from wire bytes? Anticipated NO (config-static names) — stated explicitly, not omitted (§2.11). If the SPEC finds otherwise, guard with `stats.IsValidName` before `NewCounterIfAbsent`.
- `reference_strict_reject_sibling_typeurl_gap` — every unhandled `OutputSink` arm AND every unhandled `MatchPredicate` arm needs an EXPLICIT reject, not a silent ignore (§2.3, §2.4, §5.2); D-TAP-DEPRECATED's outcome likewise (§2.10).
- `reference_tracing_upstream_cluster_framework_gap` — UNassert cross-side any `Connection`/trace field envoy-go cannot plumb rather than emitting a wrong value (§2.9, §6.2).
- `reference_fuzzer_count_docs_drift` — reconcile `grep -c '^func Fuzz'` against the doc (§6.5).
- `reference_docker_probe_bridge_network` + `feedback_probe_fresh_container_per_arm` — the SPEC probes run on a shared Docker bridge, a FRESH probe container per arm, with a decode-ran proof (§10 preamble).
- `reference_differential_fullsuite_startup_flake` — a `subject ready: EOF` on an unrelated fixture is the harness startup race, isolate-re-run to discriminate (§6.3).
- `reference_roadmap_split_phase_row_done` + ADR-0106 — row 56 flips `done` at the 56.2 (final-leg) IMPL, no parent rollup (§1.3).
- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; subagents commit locally only; the controller verifies each commit, re-runs gates on the frozen HEAD, does deliberate-break verification ITSELF, and squashes + pushes at stage-close.

---

## 12. Section closeout
- **Subject:** the HTTP tap filter `envoy.filters.http.tap` — a predicate-tree-matched dual-sided observer filter that emits a buffered `TraceWrapper` (`HttpBufferedTrace`) to a `file_per_tap` sink at stream end, only on a match.
- **Q0 pick:** the tap filter, re-opening the loop; a narrow-CDS xDS opener, `graphite`, and an admin reload/validate endpoint were declined.
- **Q1 trailers (LOAD-BEARING):** SCOPE OUT — boot-reject both trailer match arms, never populate `Message.trailers`, stay framework-zero-touch, record the coverage boundary, defer HCM "Task 18" to its own framework-surgery row (§2.7). Tap is the first feature to collide with the never-done Task 18.
- **Q2 sink:** `file_per_tap` ONLY; explicit reject for the other four `OutputSink` arms (§2.3).
- **Q3 shape+format:** BUFFERED trace, JSON; boot-reject `streaming: true`; the `JSON_BODY_AS_BYTES`/`JSON_BODY_AS_STRING` pick is a SPEC probe (D-TAP-JSON) and is INDISTINGUISHABLE at 56.1 (§2.2).
- **Q4 arms:** the 6 logical+header arms (`and`/`or`/`not`/`any`/`http_request_headers`/`http_response_headers`); explicit reject for the 2 trailer + 2 generic_body arms (§2.4).
- **Q5 HeaderMatcher:** a NEW exported `internal/headermatch`, consumed only by tap; MIGRATE NOBODY (rbac/ratelimit/oauth2 untouched, migration documented as future debt) (§2.8).
- **Q6 split (ADR-0045 gate CONSUMED):** 56.1 headers / 56.2 bodies, BY CONCERN not by layer (a by-layer split would strand dead library code with no differential surface) (§1.4).
- **The load-bearing centrepiece (§2.6):** a buffered trace is an END-OF-STREAM ARTIFACT — accumulate request + response headers, emit at stream end ONLY once the predicate resolves to a match; emitting at `DecodeHeaders` is the single most likely bug. Corollary: the compiled predicate tree needs a TRI-STATE (match/no-match/undetermined) per node, since a response-arm node cannot resolve before `EncodeHeaders` — the short-circuit/early-emit nuance is flagged, NOT settled.
- **Scope:** `internal/headermatch` (exported HeaderMatcher) + `internal/matchpredicate` (recursive tree compiler+evaluator, tri-state) + `internal/filter/http/tap` (dual-sided observer filter) + a `file_per_tap` per-stream file sink (placement DEFERRED, D-TAP-SINKPLACEMENT) + the `builtins.go` registration arm + the `0099-http-tap-headers` (56.1) / `0100-http-tap-bodies` (56.2) differentials.
- **Untouched:** the HTTP filter dispatch layer, the callback surface, the trailer seams (framework-zero-touch), `rbac`/`ratelimit`/`oauth2` (their private HeaderMatchers), and every other filter.
- **Anticipated counts (ANTICIPATIONS — the SPEC pins them):** HTTP filters 19 → **20** (`envoy.filters.http.tap`); NEW packages **2** (`internal/headermatch`, `internal/matchpredicate`) + the filter package + the sink placement (DEFERRED); go.mod modules **0 new** (`go mod tidy -diff` anticipated EMPTY); fixtures 100 → **101** at 56.1 (`0099-http-tap-headers`) → **102** at 56.2 (`0100-http-tap-bodies`); BackendKind **38 (+0)**; ADRs **ADR-0273** (56.1 IMPL) + **ADR-0274** (56.2 IMPL), no parent rollup; stat surface **SPEC PROBE (D-TAP-STATS)** — NOT guessed; fuzzers **SPEC PROBE (D-TAP-FUZZER)** — a `MatchPredicate`-tree parse fuzzer a strong candidate, NOT committed. Baselines RE-VERIFIED this session against master tip `a7a7c2fb`: fixtures 100, fuzzers 52, DECISIONS tail ADR-0272, BackendKind 38, stat surface 1200.
- **Load-bearing SPEC probes:** D-TAP-EMIT-TIMING (confirm the end-of-stream-artifact model) + D-TAP-RESPMATCH (whole stream or matched half?) + D-TAP-JSON (the protojson rendering) + D-TAP-FILENAME (the glob scheme) + D-TAP-DEPRECATED (`match_config` vs `match`) + D-TAP-STATS (the stat surface) + D-TAP-CONN (which `Connection` fields) + D-TAP-NOSINK/MULTISINK (repeated `sinks`).
- **Load-bearing DESIGN pins:** D-TAP-SINKPLACEMENT (sink package placement) + the tri-state short-circuit/early-emit nuance (§2.6).
- **Row 56** registers `in-progress` at this BRAINSTORM commit; flips `done` at the **56.2 IMPL** six-gate (NO parent rollup — ADR-0106). The Observability FAMILY STAYS OPEN (`graphite`/OTLP-metrics sinks, tracing extras, and this row's own §8 tap deferrals remain).
- **Next → the phase-56.1 SPEC** (`SPEC.md` — execute the §10 D-TAP-* live pins against `envoyproxy/envoy:contrib-v1.37.2`; anchor the ADR-0273 §Context draft).
