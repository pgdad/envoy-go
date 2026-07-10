# Phase 56.1 SPEC — the HTTP tap filter, headers leg (`envoy.filters.http.tap`): `internal/headermatch` + `internal/matchpredicate` (a tri-state predicate tree) + `internal/filter/http/tap` (a dual-sided end-of-stream observer) + an in-package `filePerTapSink` emitting a byte-exact buffered-JSON `TraceWrapper` + the `0099-http-tap-headers` differential — the FIRST leg of the NINTH Observability-family row (ANCHORS ADR-0273)

> **Lifecycle stage:** SPEC (lifecycle-state 1 → 2). Docs-only, worktree `.worktrees/phase-56.1-spec`, branch `phase-56.1-http-tap-filter-spec`. NO production code. Row 56 STAYS `in-progress` (flips `done` only at the 56.2 IMPL six-gate — `reference_roadmap_split_phase_row_done` + ADR-0106).
>
> **The BRAINSTORM's seven settled pins (Q0..Q6) are the input.** The §10/§11 D-TAP-* empirical pins were EXECUTED IN-SESSION 2026-07-10 against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0004/ADR-0227) on a Docker **bridge** network with `--concurrency 1`, a **FRESH probe container per arm** (`feedback_probe_fresh_container_per_arm`), a `mccutchen/go-httpbin` backend, and a `curlimages/curl` sidecar driver (the envoy image ships no curl/wget/nc). **All ten D-TAP-* pins are RESOLVED here.** They CONFIRM the buffered-trace lifecycle and CORRECT the charter on twelve counts (§1.1): the charter's "short-circuit/early-emit is unsettled" nuance is dissolved (the reference NEVER early-emits), the JSON rendering is pinned byte-exact, a whole class of pseudo-header asymmetries the BRAINSTORM missed is surfaced (including a wire-leak trap), an f3 field (`tap_enabled`) the charter never mentioned is caught, the reject taxonomy is split PARITY-vs-DEPARTURE (with the reference ABORTING ITS OWN PROCESS on `streaming_grpc`), and three charter miscounts (HeaderMatcher arms, filter count, the registry-freeze site) are corrected.

---

## 1. Purpose / Mission

Implement Envoy's HTTP tap filter `envoy.filters.http.tap` (headers concern only) — a per-stream dual-sided observer filter that compiles a `config/common/matcher/v3.MatchPredicate` proto tree into a tri-state node tree at config time, evaluates it incrementally as request+response headers arrive, and — **at stream end, on a match** — assembles a buffered `data/tap/v3.TraceWrapper` (carrying an `HttpBufferedTrace`) and writes it as one byte-exact protojson document to a per-stream `file_per_tap` sink file. It composes on the LANDED HTTP-filter seam (the `StreamDecoderFilter` + `StreamEncoderFilter` interfaces + the two-step ADR-0071 factory + the `builtins.go` registration seam), adds TWO new library packages (`internal/headermatch`, `internal/matchpredicate`) and the filter package `internal/filter/http/tap` (which houses the sink — D-TAP-SINKPLACEMENT), and adds ZERO new go.mod modules (all tap protos resolve at the pinned `go-control-plane/envoy v1.32.4`). It is the FIRST Observability-family row to integrate at the HTTP-filter layer rather than the bootstrap-sink layer, and the FIRST filter whose emit decision is deferred to stream end across BOTH directions. Byte-identical when no tap filter is configured. ANCHORS ADR-0273; the family STAYS OPEN.

The full phase spans two legs, split BY CONCERN *(Q6)*: **56.1** (this leg) ships the predicate engine + the buffered-trace lifecycle + the file sink over a **headers-only** trace; **56.2** adds only the body-capture concern (`max_buffered_rx_bytes`/`max_buffered_tx_bytes`, `Body.truncated`, the `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` distinction) on top of a proven spine.

### 1.1 Empirical-finding-driven scope — the `AMEND-TAP-*` block (per ADR-0044)

Twelve amendments, ordered by consequence. **Seven are PROBE-DERIVED** — AMEND-TAP-{BODY, NOEARLYEMIT, JSON, PRECEDENCE, TAPENABLED, REJECTS, SINKS} — and each carries its verbatim reference output in §11. **Five are SOURCE-DERIVED** — AMEND-TAP-{PSEUDO, ARMCOUNT, FILTERCOUNT, FREEZE, PROTOPATHS} — re-derived from the envoy-go tree and the `go-control-plane/envoy v1.32.4` protos, with no §11 subsection (there is nothing to probe: they correct the charter's reading of code, not of the reference's behavior).

- **AMEND-TAP-BODY (LOAD-BEARING — it decides the fixture).** The reference **ALWAYS captures bodies**; there is no config to suppress capture (`max_buffered_rx_bytes: 0` + `max_buffered_tx_bytes: 0` still emits `{"as_string": "", "truncated": true}`). **BUT a ZERO-LENGTH body message OMITS the `body` field entirely** — measured two independent ways: a bodyless `GET` request → `request keys: ['headers','trailers']` (no `body`); a `204 No Content` response → `response keys: ['headers','trailers']` (no `body`). ⇒ **A bodyless request + a 204 response yields a structurally HEADERS-ONLY trace on BOTH sides** — this is the key that makes `0099` a genuine field-by-field cross-side comparison instead of a body-excluding subset, and it is why the 56.1 fixture MUST drive `GET → 204`. Without this finding the differential could only assert a subset that excludes the `body` field; with it, `request.body`/`response.body` are asserted ABSENT (a positive, breakable assertion).
- **AMEND-TAP-NOEARLYEMIT (dissolves the charter's flagged-but-unsettled §2.6 nuance).** The BRAINSTORM flagged "whether short-circuit evaluation may emit BEFORE the response is a mechanism nuance — flag it, DO NOT SETTLE." The `orshort` probe SETTLES it: an `or_match{[http_request_headers_match(x-tap==yes), http_response_headers_match(:status==999)]}` — request arm TRUE at decode, response arm never true — still emitted a trace with BOTH `request` and `response` top keys (`response keys: ['headers','body','trailers']`). If the reference short-circuited and emitted at decode, the trace would lack `response`. ⇒ **The trace is ALWAYS the WHOLE stream, assembled and emitted at stream end; NEVER early-emit.** The tri-state (§3.3) is still needed to RESOLVE the tree — it is NOT needed to decide WHEN to emit. Emission is unconditionally at stream end.
- **AMEND-TAP-JSON (byte-exact; `EmitDefaultValues`, NOT `EmitUnpopulated`).** The reference's `file_per_tap` JSON is exactly `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitDefaultValues: true}` applied to the `TraceWrapper`, plus a trailing `"\n"`. Measured BYTE-EXACT: read a real reference trace, `protojson.Unmarshal` into `data/tap/v3.TraceWrapper`, re-`Marshal` with those options → `EXACT BYTE MATCH (ignoring trailing \n): true`; the reference file ends with `}\n }\n}\n`. `EmitUnpopulated: true` is WRONG (it emits `"body": null`, `"headers_received_time": null`, `"downstream_connection": null`, which the reference never does); `EmitDefaultValues` reproduces the reference's `"raw_value": ""` (scalar default) and `"trailers": []` (empty repeated) while OMITTING nil message fields — exactly the reference. Rendering is snake_case (`UseProtoNames`) with a **1-space** indent, NOT protojson's default camelCase.
- **AMEND-TAP-PSEUDO (a REAL framework asymmetry the charter missed; contains the single most dangerous IMPL trap).** Four sub-findings on the filter-visible header maps: (a) **`:scheme` is NEVER injected** into the filter-visible request header map — HCM injects `:method`/`:authority`/`:path` (`connection.go:481-482`/`:495-496`/`:506,:516` H1; `h2dispatch.go:466-467`/`:478-479`/`:488-493` H2) but the `:scheme` hits in `h2/stream.go` are the WIRE DECODER, not the filter map — so envoy-go filters CANNOT see `:scheme` while the reference emits it in `request.headers`; UNassert it cross-side. (b) **`:status` is NOT in the encode header map** — `connection.go:736` comment: "which the encode header map does not carry (:status is not present)"; the response status is obtained from `EncoderFilterCallbacks.ResponseStatus()` (ADR-0196), seeded by `chain.SetEncodeResponseStatus` (`connection.go:737`; `chain.go:1178`/`:1250`). (c) **THE WIRE-LEAK TRAP:** `connection.go:738` passes the encode header map `merged` (built at :733) to `RunEncodeHeaders`, then `resp.Headers = ReconcileOrderedHeaders(resp.Headers, merged)` at `:741` — **adding a synthetic `:status` key to `merged` would emit a literal `:status` header ON THE WIRE.** The tap filter MUST build a COPY for matching/emission and NEVER mutate the map handed to `EncodeHeaders`. (d) Go's `http.Header` canonicalizes keys (`User-Agent`) and has no stable iteration order; the reference emits lowercase codec order — so tap must LOWERCASE every key when building `HeaderValue.key`.
- **AMEND-TAP-PRECEDENCE (`match` wins; NEITHER-set is a NEW parity reject).** Three-part measured answer to D-TAP-DEPRECATED: (i) `match_config` (f1, DEPRECATED) alone → reference ACCEPTS and taps; (ii) BOTH `match_config` and `match` (f4) set → reference ACCEPTS, **`match` WINS, `match_config` is IGNORED** (arm `prec`: `match_config: any_match:true` [would tap] + `match: http_request_headers_match(x-tap=="NOPE")` [would not], request carried `x-tap: yes` → `TAP FILE COUNT: 0`); (iii) NEITHER set → reference **BOOT-REJECTS** (`Neither match nor match_config is set in TapConfig`) — a reject arm the BRAINSTORM never anticipated. envoy-go's disposition (§6): reject `match_config` at all (it cannot faithfully honor the DEPRECATED-wins-when-alone-but-loses-to-`match` precedence without implementing the deprecated `config/tap/v3.MatchPredicate` proto too); mirror the NEITHER-set parity reject.
- **AMEND-TAP-TAPENABLED (an unhandled f3 the charter never mentioned).** `TapConfig.tap_enabled` (f3, a `config/core/v3.RuntimeFractionalPercent`, `config/tap/v3/common.pb.go:143`) exists and the charter's §2 field list omits it entirely. Probed INERT across three arms (`default_value 0/HUNDRED` with and without a `runtime_key`, and `100/HUNDRED` with a key) — all tapped. Report the OBSERVATION only; assert nothing about Envoy internals. envoy-go cannot honor a runtime-fractional gate faithfully ⇒ EXPLICIT reject when `tap_enabled` is set (§6).
- **AMEND-TAP-REJECTS (the PARITY-vs-DEPARTURE taxonomy; the reference ABORTS on `streaming_grpc`).** The reject roster splits into arms where the reference ALSO boot-rejects (PARITY) and arms where the reference ACCEPTS or CRASHES but envoy-go rejects anyway per ADR-0080 (DEPARTURE). The sharpest: a `streaming_grpc` sink makes **the reference ABORT ITS OWN PROCESS** (exit 139, `tap_config_base.cc:119 panic: not implemented`), reproduced TWICE — so envoy-go's reject is strictly SAFER, and this is recorded precisely for the future `streaming_grpc` row (it is NOT merely "unimplemented in envoy-go" — it is unimplemented in contrib-v1.37.2's HTTP tap path). Full taxonomy in §6.
- **AMEND-TAP-SINKS (PGV: exactly ONE sink).** `OutputConfig.sinks` is repeated, but PGV constrains it to exactly 1 item: both `sinks: []` and two sinks boot-reject identically (`OutputConfigValidationError.Sinks: value must contain exactly 1 item(s)`). Resolves D-TAP-NOSINK / D-TAP-MULTISINK: envoy-go mirrors the exactly-1 requirement (parity).
- **AMEND-TAP-ARMCOUNT (HeaderMatcher = 8 arms; rbac handles 8 NOT 11; csrf/hcm are comments-only).** The charter's §2.8 arm counts were derived from whole-file `grep -c HeaderMatcher_` and are WRONG. Corrected: `config/route/v3.HeaderMatcher`'s `HeaderMatchSpecifier` oneof has **8** arms (+ `invert_match` + `treat_missing_header_as_empty`); `internal/rbac/evaluator.go:855` handles **8** (not 11 — the extra 3 were a comment + two `compileHeaderMatcher` refs); `ratelimit` and `oauth2` handle **7** each (verified); real `routev3.HeaderMatcher` CODE use exists only in `fault` (the `StringMatch` arm), while the charter's cited `csrf`/`hcm/config.go` uses are COMMENTS not code. ⇒ `internal/headermatch` must cover 8 oneof arms + `invert_match` + `treat_missing_header_as_empty`, and carry its OWN `StringMatcher` arm evaluation (no shared `StringMatcher` evaluator exists either).
- **AMEND-TAP-FILTERCOUNT (20 registered → 21; 19 production → 20 — the charter's "19 → 20" is wrong as stated).** `builtins.go`'s bulk registration block is `:44-63` and registers **20** filters today, one of which (`envoygotest`) is a test-support filter (package doc: "twenty built-in HTTP filters"). Precise statement: **20 registered → 21 registered; 19 production → 20 production** (`envoygotest` excluded).
- **AMEND-TAP-FREEZE (the HTTP registry is frozen at `internal/boot/boot.go:65`, NOT main.go).** The charter's §1.7 (echoing `registry.go:58`'s STALE doc comment) says the HTTP registry is `Freeze`d by `cmd/envoy-go/main.go`. It is not: `httpReg.Freeze()` is at `internal/boot/boot.go:65`, right after `RegisterBuiltins(httpReg)` (:64); `main.go:322`'s `Freeze` is the STATS registry.
- **AMEND-TAP-PROTOPATHS (three `common.pb.go`s; two `MatchPredicate` protos; `TraceWrapper` is 4-arm).** A systemic citation hazard: a bare `common.pb.go` names THREE different files — `extensions/common/tap/v3` (`CommonExtensionConfig`), `config/tap/v3` (`TapConfig`, `OutputSink`, and a package-local `config/tap/v3.MatchPredicate` DISTINCT from the `config/common/matcher/v3.MatchPredicate` this row compiles), and `data/tap/v3` (`Connection`, `Body`). And `data/tap/v3.TraceWrapper` is a **4-arm** oneof (`HttpBufferedTrace`, `HttpStreamedTraceSegment`, `SocketBufferedTrace`, `SocketStreamedTraceSegment`), not the 2 the charter implied. Every proto citation in this SPEC is fully qualified (§5).

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0272** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0273 §Context DRAFT** only (the SOLE anticipated ADR for this leg; §Decision/§Consequences land at the 56.1 IMPL). **All ten BRAINSTORM §10 D-TAP-* questions are RESOLVED here** (§11): D-TAP-STATS, D-TAP-FILENAME, D-TAP-JSON, D-TAP-DEPRECATED, D-TAP-EMIT-TIMING, D-TAP-RESPMATCH, D-TAP-CONN, D-TAP-NOSINK/MULTISINK (as AMEND-TAP-SINKS), D-TAP-FUZZER (YES, §3.3), and D-TAP-SINKPLACEMENT (the design pin, §3.0 → in-package). The SIX NEW PLAN-level questions the amendments raise are collected in §12 (the depth-cap value; the trace-id source; whether `record_downstream_connection`/`record_headers_received_time` are honored at 56.1; the exact asserted header subset; the emit-site ONE-SHARED-VALUE constraint; `path_prefix` directory semantics).

---

## 2. Non-purposes (per BRAINSTORM §1.2 + §8 + the §1.1 amendments)

Deferred, each named precisely in §8 of the BRAINSTORM and carried forward here:

- **Body capture** (56.2): `Body`, `max_buffered_rx_bytes`/`max_buffered_tx_bytes`, `Body.truncated`, the `JSON_BODY_AS_BYTES`-vs-`JSON_BODY_AS_STRING` distinction (INDISTINGUISHABLE until a body exists — AMEND-TAP-BODY forces the 0099 fixture to have NO body, so 56.1 never renders one).
- **The `streaming_grpc` sink** — CONFIRMED PRESENT in the module (`service/tap/v3.TapSinkService_StreamTaps`, `tap_grpc.pb.go:22`), but **the reference ABORTS its own process on it** (AMEND-TAP-REJECTS) — a future row must implement it against a reference that does not.
- **The `streaming_admin` + `buffered_admin` sinks + the `admin_config` (dynamic `POST /tap`) lifecycle** — BLOCKED on `internal/admin` gaining chunked/flush semantics.
- **`streaming: true` / `HttpStreamedTraceSegment`** — a different `TraceWrapper` oneof arm; the reference writes 3 concatenated JSON documents into one file (AMEND-TAP-REJECTS DEPARTURE), envoy-go boot-rejects it.
- **The 2 `generic_body` match arms** (reference BOOTS and matches them — DEPARTURE); the 2 **trailer** match arms + `Message.trailers` CONTENT (Q1; reference BOOTS and honors trailer arms — DEPARTURE; envoy-go is framework-zero-touch on the never-done HCM "Task 18").
- **The `PROTO_BINARY`/`PROTO_BINARY_LENGTH_DELIMITED`/`PROTO_TEXT` formats** (reference BOOTS and honors — DEPARTURE) and **`OutputSink.custom_sink`**.
- **The tap TRANSPORT SOCKET** (`extensions/transport_sockets/tap/v3`, PRESENT) — the real L4 tap; there is no network tap FILTER in Envoy (`extensions/filters/network/tap/v3` ABSENT, confirmed).
- **Migrating `rbac`/`ratelimit`/`oauth2` onto `internal/headermatch`** — Q5's documented debt; a future row once the arm coverage is proven a superset.
- **`tap_enabled` (f3)** honoring (AMEND-TAP-TAPENABLED) — envoy-go rejects it rather than emulating a runtime-fractional gate.

No change to any landed filter, the HTTP-filter dispatch layer, the callback surface, or the trailer seams (framework-zero-touch).

---

## 3. The tap filter (ADR-0273) — the architecture

Tap is the 20th PRODUCTION HTTP filter (AMEND-TAP-FILTERCOUNT), a dual-sided observer plugging into the existing `StreamDecoderFilter` (`internal/filter/http/types.go:54-60`) + `StreamEncoderFilter` (`:65-71`) interfaces via the two-step ADR-0071 factory (`HTTPFilterFactory func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error)` :245 → `FilterInstanceFactory func() HTTPFilter` :249), registered in `builtins.go` (the bulk block `:44-63`) and frozen at `internal/boot/boot.go:65` (AMEND-TAP-FREEZE). No new dispatch layer, no new callback surface — a usage pattern, not a new framework primitive.

### 3.0 ADR-0045 split re-check — the by-concern split STANDS; the escape valve is re-armed (D-TAP-SINKPLACEMENT resolved: in-package)

The controller's task sketch for 56.1 is **~14 tasks** (§10). Phase 55 TOUCHED the ~14-task gate and stayed flat; tap is strictly larger, so the by-concern 56.1/56.2 split was already CONSUMED at the BRAINSTORM *(Q6)*. This SPEC's task decomposition lands AT ~14, not past it. **Verdict: the chartered 56.1/56.2 by-concern split STANDS; do NOT re-split.** Escape valve, explicit: *if the PLAN's decomposition exceeds ~15 tasks, re-open ADR-0045 before writing code.*

**D-TAP-SINKPLACEMENT → the sink lives INSIDE `internal/filter/http/tap`** (an unexported `filePerTapSink` in its own file within the filter package), NOT a sibling `internal/tapsink`. Rationale: `file_per_tap` has exactly ONE consumer and 56.2 does not change that; a sibling `internal/tapsink` would be a package with a single importer. Extraction is the right move only when the deferred `streaming_grpc`/admin sinks land (they need a shared sink abstraction; `file_per_tap` alone does not). ⇒ NEW packages this leg = **2** (`internal/headermatch`, `internal/matchpredicate`) + the filter package `internal/filter/http/tap` (which houses the sink).

### 3.1 The config parse — the two-step ADR-0071 factory + `FactoryCtx`

The `HTTPFilterFactory` for TypeURL `envoy.filters.http.tap` unmarshals the `Tap` message (`extensions/filters/http/tap/v3.Tap`, exactly 3 fields: `CommonConfig` f1, `RecordHeadersReceivedTime` f2, `RecordDownstreamConnection` f3), reads `CommonConfig.static_config` (the `CommonExtensionConfig_StaticConfig` oneof arm f2 → a `config/tap/v3.TapConfig`; rejecting the `admin_config` arm f1), compiles `TapConfig.match` (f4) into a `*matchpredicate.Tree` (rejecting every unsupported arm at compile — §6), validates the single `OutputConfig.sinks[0]` is a `file_per_tap` (§6), registers the `http.<stat_prefix>.tap.rq_tapped` counter (§7) so it reads 0 with no taps, and returns a `FilterInstanceFactory` closure. `FactoryCtx` (`types.go:253-304`) supplies `Stats *stats.Registry` + `StatPrefix string` (for the counter) and `Registry`/`ClusterManager`/`HTTPClient` (unused this leg). The returned `FilterInstanceFactory` mints one `*tapFilter` per stream, installed as BOTH `HTTPFilter.Decoder` and `HTTPFilter.Encoder` (`types.go:77-81`).

### 3.2 `internal/headermatch` — the shared exported HeaderMatcher evaluator *(Q5; AMEND-TAP-ARMCOUNT)*

A NEW exported package evaluating one `config/route/v3.HeaderMatcher` (`route_components.pb.go:3432`) against an `http.Header`. It must cover the **8** `HeaderMatchSpecifier` oneof arms — `ExactMatch` f4, `SafeRegexMatch` f11, `RangeMatch` f6, `PresentMatch` f7, `PrefixMatch` f9, `SuffixMatch` f10, `ContainsMatch` f12, `StringMatch` f13 (the four DEPRECATED string arms are still accepted) — plus the two non-oneof modifiers `InvertMatch` f8 and `TreatMissingHeaderAsEmpty` f14. It carries its OWN `type/matcher/v3.StringMatcher` arm evaluation (exact/prefix/suffix/contains/safe_regex + `ignore_case`), because no shared `StringMatcher` evaluator exists in the tree (six private copies do). Compiled at config time, evaluated at request time. Consumed ONLY by tap (via `internal/matchpredicate`); `rbac`/`ratelimit`/`oauth2`/`fault` stay UNTOUCHED (MIGRATE NOBODY — Q5). Shape precedent (compile-then-evaluate, different proto): `internal/matcher` (`New` `matcher.go:115`, `Evaluate` `matcher.go:139`).

### 3.3 `internal/matchpredicate` — the tri-state predicate tree (compiler + incremental evaluator + depth cap)

A NEW package compiling a `config/common/matcher/v3.MatchPredicate` proto tree (`matcher.pb.go:135`, 10 oneof arms) into an evaluable node tree once at config time, then evaluating it incrementally as stream events arrive. Nothing evaluates this proto today (`grep -rniE "MatchPredicate|config/common/matcher" internal/ cmd/` → 0 hits).

**Supported: 6 of 10 arms** *(Q4)* — `or_match` f1 / `and_match` f2 (both `*MatchPredicate_MatchSet`, whose `Rules []*MatchPredicate` f1 recurse), `not_match` f3 (`*MatchPredicate`), `any_match` f4 (`bool`), `http_request_headers_match` f5 / `http_response_headers_match` f7 (both `*HttpHeadersMatch`, whose `Headers []*route/v3.HeaderMatcher` f1 feed `internal/headermatch`). **Rejected: 4 of 10** — the 2 trailer arms (f6/f8, Q1) and the 2 generic_body arms (f9/f10); each an EXPLICIT per-arm compile-time reject (§6, `reference_strict_reject_sibling_typeurl_gap`), never a fall-through default.

**The tri-state, pinned.** Node value ∈ {True, False, Undetermined}:
- `any_match` → True immediately. `http_request_headers_match` → Undetermined until `DecodeHeaders`, then True/False. `http_response_headers_match` → Undetermined until `EncodeHeaders`, then True/False.
- `not_match(x)` → Undetermined if x Undetermined, else the negation.
- `and_match` → False if ANY child False; True if ALL True; else Undetermined. `or_match` → True if ANY child True; False if ALL False; else Undetermined.
- **At stream end, any still-Undetermined node resolves to False.** This is a total-function guard, near-unreachable for HTTP (Envoy/envoy-go always synthesize a response — even a local-reply 5xx — so `EncodeHeaders` always runs and every response arm resolves), NOT an observed behavior. State it as such.

**The tri-state governs the RESOLVE only. Emission is unconditionally at stream end** (AMEND-TAP-NOEARLYEMIT). No early emit — the `orshort` probe proves the reference assembles and emits the WHOLE stream even when a request arm is already True at decode.

**The recursion depth cap.** The compiler is recursive over an attacker-influenceable proto (a deeply-nested `not_match` chain risks stack exhaustion). The compiler MUST carry a recursion DEPTH CAP and boot-reject beyond it. The exact cap value is a PLAN question (§12); this SPEC RECOMMENDS a cap of **32** (deep enough for any realistic predicate, shallow enough to bound the compile-time and the `FuzzMatchPredicateCompile` stack — §10 task 5). This is a recommendation, not a pin.

### 3.4 The dual-sided observer + the ONE-SHARED-VALUE constraint on the emit site

One `*tapFilter` value per stream serves as both `Decoder` and `Encoder`. Its obligations:
- `DecodeHeaders(http.Header, bool)` — build a lowercased, order-normalized COPY of the request headers, capture it into the pending `HttpBufferedTrace.Request.Headers`, feed the request-headers evaluation of the tree; do NOT emit; return `Continue`.
- `EncodeHeaders(http.Header, bool)` — build a COPY, inject a synthetic `:status` from `EncoderFilterCallbacks.ResponseStatus()` (§3.5), capture into `Response.Headers`, feed the response-headers evaluation; do NOT emit; return `Continue`.
- `OnDestroy()` — resolve the tree; if it MATCHED, assemble the `TraceWrapper` and hand it to the `filePerTapSink`; else emit NOTHING (no file). **`rq_tapped` increments on the MATCH decision, NOT on a successful file write** — the reference registers the counter at config time and increments on the tap decision, independent of sink-write success. (The `0099` fixture never exercises a write failure, so this coupling is stated here rather than discovered later.)

**The ONE-SHARED-VALUE constraint (LOAD-BEARING — and the INVERSE of the obvious guess).** There is NO `OnStreamEnd` hook; the once-per-stream teardown is `OnDestroy()`, declared on BOTH interfaces (`types.go:59`, `:70`). The naive reading — "`Destroy()` calls both, so a both-sided filter's `OnDestroy` fires twice, so guard it" — **is FALSE.** `FilterChain.Destroy()` (`chain.go:665`) is `destroyOnce sync.Once`-guarded (`chain.go:292`, `:666`) and its loop is:

```go
if f.Decoder != nil {
    f.Decoder.OnDestroy()      // chain.go:669
} else if f.Encoder != nil {   // chain.go:670 — ELSE IF
    f.Encoder.OnDestroy()      // chain.go:671
}
```

The two branches are **mutually exclusive**. For a filter with both fields set, only the Decoder branch runs ⇒ **`OnDestroy()` fires EXACTLY ONCE.** No once-guard is required for correctness (one is at most defensive). `Destroy()` is deferred-invoked once from `connection.go:447` (H1) and `h2dispatch.go:383` (H2), and `chain.go:669`/`:671` are the ONLY non-test callers of an HTTP-filter `OnDestroy` (grep-proven).

**The REAL hazard is the inverse: an Encoder-only `OnDestroy` is UNREACHABLE whenever a Decoder is present.** If tap were decomposed into TWO distinct values (`HTTPFilter{Decoder: &decodeSide{}, Encoder: &encodeSide{}}` — a natural-looking split), the encode side's `OnDestroy` would **never be invoked**, and a stream-end emit hung off it would silently never fire. ⇒ **tap MUST install ONE shared `*tapFilter` value in BOTH `HTTPFilter.Decoder` and `HTTPFilter.Encoder`** and hang the emit off that value's single `OnDestroy`. The `compressor` filter is the landed precedent for exactly this shape (`internal/filter/http/compressor/doc.go:43` — "ENCODER+DECODER with the SAME `*filter` instance"; `compressor.go:304`).

*(This SPEC's first draft asserted the double-fire hazard. Adversarial review re-derived `chain.go:668-672` from source and refuted it — a `feedback_brief_citations_not_evidence` catch on the controller's own brief. The corrected constraint above is the one the PLAN must honor.)*

### 3.5 The `:status` synthesis on a COPY — the wire-leak trap *(AMEND-TAP-PSEUDO)*

The response-headers match AND the emitted `response.headers` both need `:status`, which is NOT in the encode header map (`connection.go:736` states it verbatim). Take it from `EncoderFilterCallbacks.ResponseStatus()` (ADR-0196, `internal/filter/http/callbacks.go`), seeded by `chain.SetEncodeResponseStatus` (`chain.go:1178`; called at `connection.go:737` H1, `h2dispatch.go:574` H2, and `chain.go:1250` for local replies). **Inject it into a COPY of the header map, NEVER into the map handed to `EncodeHeaders`:** `connection.go:738` passes that map (`merged`, built :733) to `RunEncodeHeaders` and then `ReconcileOrderedHeaders(resp.Headers, merged)` at `:741` — a synthetic `:status` key added to `merged` would be emitted as a literal `:status` header ON THE WIRE. This is the single most dangerous IMPL trap of the leg; the IMPL must have a unit test proving no synthetic pseudo-header reaches the wire.

### 3.6 Header rendering — lowercase, sorted, `RawValue` nil *(AMEND-TAP-PSEUDO / AMEND-TAP-JSON)*

Emit each header as a `config/core/v3.HeaderValue{Key, Value}` with `RawValue` left nil (protojson `EmitDefaultValues` renders `"raw_value": ""`, reference-identical — measured). **LOWERCASE every key** (Go canonicalizes `User-Agent`; directly-set pseudo-header keys like `req.Header[":method"]` bypass canonicalization; the reference emits lowercase). Emit headers **sorted by (key, value)** for determinism — a DOCUMENTED departure from the reference's codec order (`:authority, :path, :method, :scheme, user-agent, …`), invisible to the differential which compares SETS (§8). `http.Header` is a Go map with no stable iteration order, so a sort is required regardless.

### 3.7 The `filePerTapSink` + the trace-id source + the pinned protojson options *(D-TAP-FILENAME / D-TAP-JSON)*

An unexported `filePerTapSink` in the `internal/filter/http/tap` package. On each emitted trace it opens a NEW file named `<path_prefix>_<trace_id>.<ext>` and writes ONE protojson document. `AsyncFileSink` (`internal/accesslog/writer.go:26`, ctor `NewAsyncFileSink` :41, its single-file `os.OpenFile(...O_APPEND|O_CREATE|O_WRONLY, 0o644)` at :56) is a SHAPE precedent ONLY — it opens ONE append-only file for the process lifetime; `file_per_tap` opens a DISCRETE FILE PER STREAM (`FilePerTapSink` doc, `config/tap/v3/common.pb.go:879`). The `<ext>` is format-dependent (measured, §11): `.json` for `JSON_BODY_AS_STRING`/`JSON_BODY_AS_BYTES`, `.pb`/`.pb_length_delimited`/`.pb_text` for the deferred PROTO formats. The `<trace_id>` is a 64-bit value, NOT sequential, NOT cross-side-stable, and equals the streamed-trace `trace_id` (measured). Its source in envoy-go (an atomic counter vs `crypto/rand`) is a PLAN question (§12); a process-local monotonic source suffices (the id is never asserted — §8).

**The pinned marshal (byte-exact, AMEND-TAP-JSON):** `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitDefaultValues: true}.Marshal(traceWrapper)` + a trailing `"\n"`. A byte-stability unit test is MANDATORY at the IMPL: `protojson`'s detrand did not perturb this option set at this Go version (6 separate processes → 1 sha256), but the IMPL must still land the test to catch a future toolchain change.

### 3.8 The trailers COVERAGE BOUNDARY *(Q1; §2.7 carried forward)*

envoy-go's HTTP filters CANNOT observe trailers: `FilterChain.RunDecodeTrailers` (`chain.go:454`) and `RunEncodeTrailers` (`chain.go:621`) EXIST but have ZERO non-test callers (grep-proven — only `internal/filter/http/envoygotest/filter_test.go:332` + `internal/filter/http/chain_test.go:293`); the never-done HCM "Task 18" (`connection.go:562-568`, `h2dispatch.go:501-503`) is the gap; response trailers are handled nowhere in `internal/filter/hcm/`. Tap therefore: (1) boot-rejects the 2 trailer match arms (§6); (2) NEVER populates `Message.trailers`; (3) stays framework-zero-touch; (4) records the boundary in `BEHAVIOR_CONTRACT` (§9); (5) defers Task 18 to its own future framework-surgery row. **The boundary is INVISIBLE in the 0099 trace:** a trailer-less stream renders `"trailers": []` (empty repeated, `EmitDefaultValues`) byte-identically on both sides, so the `trailers == []` assertion (§8) is cross-side-EXACT despite envoy-go never seeing a trailer.

---

## 4. Framework primitives — 2 new packages, 0 new seams, 0 new go.mod deps

| Primitive | Disposition |
|---|---|
| `StreamDecoderFilter` / `StreamEncoderFilter` interfaces | REUSED unchanged (`types.go:54-60`, `:65-71`) |
| two-step ADR-0071 factory + `FactoryCtx` | REUSED unchanged (`types.go:245`, `:249`, `:253-304`) |
| `HTTPRegistry.Register` + `builtins.go` bulk block + `boot.go:65` freeze | REUSED (a new registration arm) |
| `EncoderFilterCallbacks.ResponseStatus()` (ADR-0196) | REUSED (the `:status` source, §3.5) |
| `DecoderFilterCallbacks` connection accessors | REUSED IF `record_downstream_connection` honored (§12) |
| `internal/headermatch` | **NEW package** (§3.2) |
| `internal/matchpredicate` | **NEW package** (§3.3) |
| `internal/filter/http/tap` (incl. the `filePerTapSink`) | **NEW package** (§3.1/§3.7) |
| `internal/matcher`, `internal/accesslog` `AsyncFileSink` | SHAPE precedents ONLY, not reused |
| the 3 private HeaderMatcher evaluators (rbac/ratelimit/oauth2) | NOT reused (MIGRATE NOBODY) |
| new go.mod module | **NONE** (`go mod tidy -diff` anticipated EMPTY — all tap protos in `go-control-plane/envoy v1.32.4`) |

NEW packages = **2** (`internal/headermatch`, `internal/matchpredicate`) + the filter package `internal/filter/http/tap`. NEW seams = **0**.

---

## 5. Proto-field roster consumed at 56.1 (fully qualified per AMEND-TAP-PROTOPATHS)

| Message (package) | Field | # | Disposition |
|---|---|---|---|
| `extensions/filters/http/tap/v3.Tap` | `common_config` | 1 | **CONSUMED** (→ `CommonExtensionConfig`) |
| `Tap` | `record_headers_received_time` | 2 | honor OR defer/reject (§12 — RECOMMEND defer/UNassert) |
| `Tap` | `record_downstream_connection` | 3 | honor (§12 — RECOMMEND honor; both addr fields plumbable) |
| `extensions/common/tap/v3.CommonExtensionConfig` | `admin_config` | 1 | **REJECT** (§6) |
| `CommonExtensionConfig` | `static_config` | 2 | **CONSUMED** (→ `config/tap/v3.TapConfig`) |
| `config/tap/v3.TapConfig` | `match_config` (DEPRECATED) | 1 | **REJECT** (§6, AMEND-TAP-PRECEDENCE) |
| `TapConfig` | `output_config` | 2 | **CONSUMED** |
| `TapConfig` | `tap_enabled` | 3 | **REJECT** (§6, AMEND-TAP-TAPENABLED) |
| `TapConfig` | `match` | 4 | **CONSUMED** (→ `internal/matchpredicate`) |
| `config/tap/v3.OutputConfig` | `sinks` | 1 | **CONSUMED** (exactly 1, §6) |
| `OutputConfig` | `max_buffered_rx_bytes` | 2 | never-populate at 56.1 (56.2) |
| `OutputConfig` | `max_buffered_tx_bytes` | 3 | never-populate at 56.1 (56.2) |
| `OutputConfig` | `streaming` | 4 | **REJECT** if true (§6); honor `false` |
| `config/tap/v3.OutputSink` | `format` | 1 | honor `JSON_BODY_AS_STRING`; **REJECT** PROTO_* (§6) |
| `OutputSink` | `streaming_admin`/`file_per_tap`/`streaming_grpc`/`buffered_admin`/`custom_sink` | 2/3/4/5/6 | **CONSUME `file_per_tap` (3); REJECT the other 4** (§6) |
| `config/tap/v3.FilePerTapSink` | `path_prefix` | 1 | **CONSUMED** |
| `config/common/matcher/v3.MatchPredicate` | 6 supported / 4 rejected arms | 1-10 | §3.3, §6 |
| `config/route/v3.HeaderMatcher` | 8 oneof arms + `invert_match` (8) + `treat_missing_header_as_empty` (14) | — | **CONSUMED** via `internal/headermatch` (§3.2) |
| `data/tap/v3.TraceWrapper` | `http_buffered_trace` | 1 | **POPULATED**; the other 3 arms never populated |
| `data/tap/v3.HttpBufferedTrace` | `request`/`response` | 1/2 | **POPULATED** (headers only) |
| `HttpBufferedTrace` | `downstream_connection` | 3 | populate IFF `record_downstream_connection` (§12) |
| `data/tap/v3.HttpBufferedTrace_Message` | `headers` | 1 | **POPULATED** (lowercased, sorted) |
| `Message` | `body` | 2 | **never populated** (56.2; AMEND-TAP-BODY: absent when zero-length) |
| `Message` | `trailers` | 3 | **never populated** (§3.8; renders `[]`) |
| `Message` | `headers_received_time` | 4 | populate IFF `record_headers_received_time` (§12) |
| `config/core/v3.HeaderValue` | `key`/`value` | 1/2 | **POPULATED**; `raw_value` left nil → `""` |

---

## 6. PARSE-REJECT roster (ADR-0080) — PARITY vs DEPARTURE

Per `reference_strict_reject_sibling_typeurl_gap`, every unhandled arm gets an EXPLICIT reject, never a silent ignore or fall-through default. The reference's measured behavior is verbatim from §11.

| Condition | envoy-go | Reference behavior (measured) | Parity? |
|---|---|---|---|
| `match_config` set at all (f1 DEPRECATED) | **REJECT** | ACCEPTS alone (taps); IGNORED when `match` also set | **DEPARTURE** |
| NEITHER `match` nor `match_config` set | **REJECT** | BOOT-REJECT: `Neither match nor match_config is set in TapConfig` | **PARITY** |
| `tap_enabled` (f3) set | **REJECT** | ACCEPTS, observed INERT | **DEPARTURE** |
| `admin_config` arm (CommonExtensionConfig f1) | **REJECT** | BOOTS, inert (`rq_tapped: 0`, no file) | **DEPARTURE** |
| `sinks` count != 1 | **REJECT** | BOOT-REJECT (PGV): `Sinks: value must contain exactly 1 item(s)` | **PARITY** |
| `streaming_admin` sink | **REJECT** | BOOT-REJECT w/o admin: `Specifying admin streaming output without configuring admin.` | **PARITY** (w/o admin) |
| `buffered_admin` sink | **REJECT** | BOOT-REJECT w/o admin: `Output sink type BufferedAdmin requires that the admin output will be configured via admin` | **PARITY** (w/o admin) |
| `streaming_grpc` sink | **REJECT** | **ABORTS the reference process** (exit 139, `tap_config_base.cc:119 panic: not implemented`) | **DEPARTURE** (envoy-go strictly SAFER) |
| `custom_sink` (unregistered) | **REJECT** | BOOT-REJECT: `Didn't find a registered implementation for '…'` | **PARITY** (for an unregistered impl) |
| `streaming: true` | **REJECT** | BOOTS, writes 3 concatenated JSON docs | **DEPARTURE** |
| `PROTO_BINARY`/`_LENGTH_DELIMITED`/`_TEXT` format | **REJECT** | BOOTS and honors (`.pb`/`.pb_length_delimited`/`.pb_text`) | **DEPARTURE** |
| 2 trailer match arms (f6/f8) | **REJECT** | BOOTS and honors | **DEPARTURE** (§3.8 framework gap) |
| 2 generic_body match arms (f9/f10) | **REJECT** | BOOTS and matches | **DEPARTURE** |

The `file_per_tap` sink (arm 3), the 6 supported `MatchPredicate` arms, `JSON_BODY_AS_STRING`, and `streaming: false` are the ACCEPT set. **The strict-reject arms are proven by UNIT TESTS, not the fixture** — `reference_differential_fixture_dispatch_constraint` forbids mixing cross-side and boot-reject in one fixture dir, and phase 55's two parse-rejects set the unit-test precedent (§8.3).

---

## 7. Stat surface — 1200 → 1201 (+1); zero new SN rule

**One name shape:** `http.<stat_prefix>.tap.rq_tapped` (counter), registered at filter-parse (not per-stream) so it reads `0` with no taps — confirmed by the reference: the `admin_config` and no-match arms show `http.hcm_probe.tap.rq_tapped: 0` at CONFIG time. The `.tap.` segment is HARDCODED, NOT derived from the filter name (renaming the `http_filters[]` entry to `my-custom-tap-name` still yields `http.hcm_probe.tap.rq_tapped`). Register via `FactoryCtx.Stats.NewCounter("http." + StatPrefix + ".tap.rq_tapped")` (the `fault` precedent, `internal/filter/http/fault/fault.go:242-248`; `internal/stats/registry.go:84`).

**Zero new SN rule** (measured). envoy-go's EXISTING extractor already produces the reference's Prometheus rendering — running `stats.ExtractTags` (`internal/stats/name.go:47`) in-tree on `http.hcm_probe.tap.rq_tapped` yields `name="http.tap_rq_tapped" tags=[{envoy_http_conn_manager_prefix hcm_probe}]` via the generic `http.` arm (`name.go:61`), matching the reference's `envoy_http_tap_rq_tapped{envoy_http_conn_manager_prefix="hcm_probe"}`. No new stat-name rule is needed.

**`reference_dynamic_stat_name_charset_guard` is NOT applicable** — stated explicitly. NO tap stat name is derived from wire bytes (the `.tap.` segment is hardcoded, not from a matched header or config string), so the `stats.IsValidName`-guard-before-`NewCounterIfAbsent` discipline (`registry.go:60`/`:161`) does not apply here.

Surface: **1200 → 1201** (H2 cluster); non-H2 **1196 → 1197**. One name SHAPE added to `BEHAVIOR_CONTRACT` (§9).

---

## 8. Differential fixture taxonomy (+1: `0099-http-tap-headers`)

### 8.1 `0099-http-tap-headers` — the GET→204 headers-only design *(AMEND-TAP-BODY)*

One HCM + one tap filter per side. `match: and_match{rules: [http_request_headers_match(x-tap exact "yes"), http_response_headers_match(:status exact "204")]}`; sink `file_per_tap{path_prefix: <per-side temp dir>/out}`; `format: JSON_BODY_AS_STRING`; `streaming` unset. Backend returns **204 No Content** plus a fixed header. The driver drives **N matching** (`x-tap: yes`) + **M non-matching** (`x-tap: no`) **bodyless GET**s ⇒ headers-only traces on BOTH sides (AMEND-TAP-BODY: a bodyless request + a 204 response omits the `body` field entirely). `BackendCount()` returns **≥1** (`reference_differential_backendcount_min_one`; the `0018-http-rbac` all-driver-owned precedent). BackendKind stays **38**.

### 8.2 The glob-and-decode comparison *(D-TAP-FILENAME / D-TAP-JSON / D-TAP-RESPMATCH)*

Per `reference_streaming_sink_differential_framing`, assert the decoded PAYLOAD, never filenames/framing (filenames embed a proxy-internal `<trace_id>` that will NOT agree cross-side). Glob `<path_prefix>*`, decode each file as a `data/tap/v3.TraceWrapper`. Cross-side assertions (each `t.Errorf`-per-property, `reference_fatalf_makes_assertions_unreachable`):
- trace COUNT == N (one per matching stream; NO trace for the M non-matching streams — AMEND-TAP-NOEARLYEMIT / D-TAP-EMIT-TIMING: no file on non-match).
- `http.<prefix>.tap.rq_tapped` == N.
- per trace, `request.headers` ⊇ a named lowercased key/value subset (set-compared) — D-TAP-RESPMATCH confirms `request.headers` IS fully populated even under a response-arm predicate (the trace is the WHOLE stream).
- per trace, `response.headers` ⊇ `{:status: "204", <fixed backend header>}`.
- `request.body` / `response.body` ABSENT (AMEND-TAP-BODY — a POSITIVE breakable assertion, enabled by the GET→204 shape).
- `trailers == []` (§3.8 — cross-side-EXACT despite the framework gap, because `EmitDefaultValues` renders empty repeated as `[]` byte-identically).
- `downstream_connection` ABSENT (both `record_*` flags unset in the baseline fixture).

**UNasserted coverage boundaries** (`reference_tracing_upstream_cluster_framework_gap`): `:scheme` (never plumbed to the filter map — AMEND-TAP-PSEUDO); header ORDER (envoy-go sorts, reference emits codec order — §3.6); the filenames (proxy-internal trace ids); the reference-only request headers (`x-request-id`, `x-forwarded-proto`, `x-envoy-expected-rq-timeout-ms`) and response headers (`date`, `server`, `x-envoy-upstream-service-time`); `Message.trailers` CONTENT (Q1 — invisible here anyway).

### 8.3 Why the rejects are unit-tested, not fixtured

`reference_differential_fixture_dispatch_constraint` — one fixture dir = ONE runner branch (cross-side XOR boot-reject); the §6 reject roster is a set of boot-rejects that cannot share the `0099` cross-side dir. Phase 55's two parse-rejects set the unit-test precedent. The IMPL proves the full §6 reject roster via `internal/filter/http/tap` parse unit tests (each asserting a specific reject error), plus a renderer byte-stability test (§3.7) — NOT differential dirs.

### 8.4 Deliberate breaks (≥4; each `-count=1`, each `t.Errorf`-per-property isolated)

Per `reference_differential_break_protocol_count1` (go-test caching serves a stale PASS otherwise), `reference_deliberate_break_wrong_assertion` (confirm WHICH assertion fired; add an isolating break for any masked one), and `reference_fatalf_makes_assertions_unreachable` (Errorf per independent property). Use `-run 'TestDifferential/0099-http-tap-headers'`, NEVER bare `0099` (`reference_differential_run_selector` — a bare selector matches zero subtests → vacuous green).

- **(a) Emit at `DecodeHeaders` instead of stream end** → `response.headers` assertion fires (the trace lacks response headers / the `:status==204` arm never resolved). Proves the end-of-stream-artifact model is LIVE.
- **(b) Predicate always-true (treat every stream as a match)** → BOTH the trace-count assertion AND the `rq_tapped` assertion fire (two independent properties: spurious files for the M non-matching streams; `rq_tapped` overcounts).
- **(c) `EmitUnpopulated` instead of `EmitDefaultValues`** → `"body": null` appears ⇒ the `request.body`/`response.body`-ABSENT assertion fires. Proves the AMEND-TAP-JSON rendering is LIVE.
- **(d) Populate `Message.trailers`** → the `trailers == []` assertion fires. Proves the trailers boundary (§3.8) is LIVE.

Full-suite discipline: a `subject ready: EOF` on an UNRELATED fixture is the known startup flake (`reference_differential_fullsuite_startup_flake`), isolate-re-run to discriminate; Docker bridge network for the reference (`reference_docker_probe_bridge_network`).

---

## 9. Behavior-contract delta (the 56.1 bundle; ADR-0052 atomic landing)

`BEHAVIOR_CONTRACT.md` gains, landed atomically at the 56.1 IMPL:
- the tap filter's buffered end-of-stream-artifact model (accumulate request+response headers, emit at stream end on a match, NEVER early-emit);
- the tri-state predicate semantics + the depth cap;
- the `http.<stat_prefix>.tap.rq_tapped` counter (ONE name shape — the BEHAVIOR_CONTRACT counts name SHAPES, e.g. the `fault.aborts_injected` row at `BEHAVIOR_CONTRACT.md:208`);
- the `file_per_tap` byte-exact JSON rendering (`EmitDefaultValues` + trailing `\n`);
- the trailers COVERAGE BOUNDARY (§3.8) and the `:scheme` coverage boundary (§3.5/§8.2);
- the full §6 reject roster with its PARITY-vs-DEPARTURE dispositions.

---

## 10. Per-task structure (~14 tasks; the PLAN decomposes)

1. `internal/headermatch` — 8 oneof arms + `invert_match` + `treat_missing_header_as_empty` + unit tests.
2. `internal/headermatch` — its own `StringMatcher` arms (exact/prefix/suffix/contains/safe_regex + `ignore_case`) + unit tests.
3. `internal/matchpredicate` — the tri-state node types + the compiler (6 accept / 4 reject arms / the depth cap).
4. `internal/matchpredicate` — the incremental evaluator (request-headers feed, response-headers feed, stream-end resolve) + unit tests.
5. `FuzzMatchPredicateCompile` over the compiler (§3.3 — it must exercise the depth cap) + reconcile `grep -c '^func Fuzz'` against the doc (`reference_fuzzer_count_docs_drift`).
6. `internal/filter/http/tap` — config parse + the full §6 reject roster + `rq_tapped` registration.
7. The dual-sided capture: the `:status` synthesis on a COPY, lowercasing, sorting (§3.5/§3.6).
8. The stream-end emit in `OnDestroy` (§3.4) — ONE shared `*tapFilter` in both `HTTPFilter.Decoder` and `.Encoder` (the `compressor` shape), with a unit test proving the emit fires exactly once and that an encoder-only decomposition would NOT fire.
9. `filePerTapSink` — per-stream file, `<prefix>_<id>.json`, the pinned protojson opts + trailing `\n`, a process-local trace-id source (§3.7).
10. `builtins.go` registration arm + boot wiring (freeze at `boot.go:65`).
11. Unit tests for the reject roster + the renderer byte-stability test (§3.7).
12. Fixture `0099-http-tap-headers` config + driver (GET→204, N match + M non-match).
13. Differential assertions + the 4 deliberate breaks (§8.4), controller-reperformed.
14. Docs bundle: `BEHAVIOR_CONTRACT` delta + ADR-0273 body + PROGRESS/STATE/ROADMAP.

If the PLAN's decomposition exceeds ~15 tasks, re-open ADR-0045 before writing code (§3.0).

---

## 11. SPEC-time empirical-pin block — executed IN-SESSION 2026-07-10 against `envoyproxy/envoy:contrib-v1.37.2`

**Harness:** `--concurrency 1`, Docker **bridge** network `tapprobe-net`, a **FRESH probe container per arm** (`feedback_probe_fresh_container_per_arm` — `docker logs` accumulates across `docker restart`), backend `mccutchen/go-httpbin`, driver a `curlimages/curl` sidecar (the envoy image has NO curl/wget/nc — verified). HCM `stat_prefix: hcm_probe`. Tap output dir bind-mounted to the host.

### Summary disposition table (10 pins)

| Pin | Disposition |
|---|---|
| D-TAP-STATS | **RESOLVED** — exactly ONE counter `http.hcm_probe.tap.rq_tapped`; `.tap.` hardcoded; zero new SN rule; charset guard N/A |
| D-TAP-FILENAME | **RESOLVED** — `<path_prefix>_<trace_id>.<ext>`; `<ext>` format-dependent; `<trace_id>` == streamed `trace_id`, not cross-side stable |
| D-TAP-JSON | **RESOLVED** — `EmitDefaultValues` (NOT `EmitUnpopulated`) + trailing `\n`; BYTE-EXACT |
| D-TAP-DEPRECATED | **RESOLVED** — `match` wins; `match_config` alone taps; NEITHER-set boot-rejects (NEW parity reject) |
| D-TAP-EMIT-TIMING | **RESOLVED** — end-of-stream artifact; NO file / NO counter on no-match; NEVER early-emit |
| D-TAP-RESPMATCH | **RESOLVED** — the WHOLE stream; `request.headers` fully populated under a response-only predicate |
| D-TAP-CONN | **RESOLVED** — 2 address fields + μs RFC3339 timestamps, gated by the two `record_*` flags; both ABSENT when unset |
| D-TAP-NOSINK / D-TAP-MULTISINK | **RESOLVED** (AMEND-TAP-SINKS) — PGV exactly 1 sink; 0 and 2 boot-reject identically |
| D-TAP-FUZZER | **RESOLVED** — YES, `FuzzMatchPredicateCompile`; fuzzers 52 → 53; depth cap required |
| D-TAP-SINKPLACEMENT (design) | **RESOLVED** — in-package (`internal/filter/http/tap`), not a sibling (§3.0) |

### 11.1 D-TAP-STATS — exactly ONE counter

`/stats` grep for `tap` yields exactly one line in every arm: `http.hcm_probe.tap.rq_tapped: 1`. Registered at CONFIG time (the `admin_config` and no-match arms show `: 0`). The `.tap.` segment is HARDCODED (renaming the `http_filters[]` entry to `my-custom-tap-name` still yields `http.hcm_probe.tap.rq_tapped: 1`). Prometheus: `envoy_http_tap_rq_tapped{envoy_http_conn_manager_prefix="hcm_probe"} 1`. In-tree `stats.ExtractTags`: `http.hcm_probe.tap.rq_tapped => name="http.tap_rq_tapped" tags=[{envoy_http_conn_manager_prefix hcm_probe}] err=<nil>` (the generic `http.` arm, `name.go:61`) — **zero new SN rule.** No stat name is wire-derived ⇒ `reference_dynamic_stat_name_charset_guard` N/A.

### 11.2 D-TAP-FILENAME — `<path_prefix>_<trace_id>.<ext>`

Measured extensions, one arm per `Format`:
```
JSON_BODY_AS_BYTES            -> out_16095354870259260568.json
JSON_BODY_AS_STRING           -> out_7696813714577153902.json
PROTO_BINARY                  -> out_840800694095554995.pb
PROTO_BINARY_LENGTH_DELIMITED -> out_12820821274359038872.pb_length_delimited
PROTO_TEXT                    -> out_7537661629024799669.pb_text
```
The `streaming: true` arm wrote `out_1783586668515541935.json` whose first doc is `{"http_streamed_trace_segment": {"trace_id": "1783586668515541935", ...}}` — the `<id>` equals the trace's `trace_id`. Not sequential, not cross-side stable.

### 11.3 D-TAP-JSON — `EmitDefaultValues` + trailing `\n`, BYTE-EXACT

Method: read a real reference trace, `protojson.Unmarshal` into `data/tap/v3.TraceWrapper`, re-`Marshal` with `{Multiline: true, Indent: " ", UseProtoNames: true, EmitDefaultValues: true}`:
```
ref len=1705  go len=1704
EXACT BYTE MATCH (ignoring trailing \n): true
ref ends with newline: true | go ends with newline: false
```
(ref last bytes `20 7d 0a 20 7d 0a 7d 0a` = `}\n }\n}\n`.) `EmitUnpopulated: true` is WRONG (emits `"body": null`, `"headers_received_time": null`, `"downstream_connection": null` — the reference never does). `EmitDefaultValues` reproduces `"raw_value": ""` and `"trailers": []` while omitting nil message fields. 6 separate processes → 1 distinct sha256 (detrand did not perturb this option set at this Go version); the IMPL must still land a byte-stability test. Rendering is snake_case, 1-space indent.

### 11.4 D-TAP-EMIT-TIMING — end-of-stream artifact; NO file/counter on no-match

Arm `reqhdr` (`http_request_headers_match` on `x-tap: yes`), one matching + one non-matching request: `TAP FILE COUNT: 1`, `http.hcm_probe.tap.rq_tapped: 1`. **NO EARLY EMIT, EVER.** Arm `orshort` (`or_match{[http_request_headers_match(x-tap==yes), http_response_headers_match(:status==999)]}` — request arm already TRUE at decode, response arm never true): `files=1`, `trace top keys: ['request', 'response']`, `response keys: ['headers','body','trailers']`. ⇒ the trace is ALWAYS the WHOLE stream, assembled and emitted at stream end. The §2.6 short-circuit/early-emit nuance is RESOLVED: never early-emit.

### 11.5 D-TAP-RESPMATCH — the WHOLE stream

Arm `respmatch` (`http_response_headers_match` on `:status==200` only), 2 requests → 2 files, `rq_tapped: 2`: `request keys: ['headers','trailers']` fully populated (`request first hdr: {'key': ':authority', 'value': 'tapenvoy-respmatch:10000', 'raw_value': ''}`); `response keys: ['headers','body','trailers']`. ⇒ `request.headers` IS fully populated under a response-only predicate.

### 11.6 D-TAP-DEPRECATED — three-part answer

(i) `match_config` (f1) ALONE → ACCEPTS and taps (arm `deprecated`: 1 file, `rq_tapped: 1`). (ii) BOTH set → ACCEPTS, `match` WINS (arm `prec`: `match_config: any_match:true` + `match: http_request_headers_match(x-tap=="NOPE")`, request carried `x-tap: yes` → `TAP FILE COUNT: 0`, `rq_tapped: 0`). (iii) NEITHER set → BOOT-REJECTS: `error 'Neither match nor match_config is set in TapConfig'`.

### 11.7 D-TAP-CONN — 2 address fields + μs RFC3339 timestamps

Arm `conn` (`record_downstream_connection: true`, `record_headers_received_time: true`): `TOP-LEVEL KEYS: ['request','response','downstream_connection']`; `downstream_connection` = `{local_address: {socket_address: {…172.22.0.3:10000…}}, remote_address: {socket_address: {…172.22.0.4:38216…}}}` (TWO address fields); `request` has `headers_received_time = 2026-07-10T11:33:08.539671Z`, `response` `…540061Z`. With both flags unset (baseline), `downstream_connection` and `headers_received_time` are ABSENT.

### 11.8 D-TAP-NOSINK / D-TAP-MULTISINK — PGV exactly 1 (AMEND-TAP-SINKS)

Both `sinks: []` and two sinks boot-reject identically: `TapConfigValidationError.OutputConfig … OutputConfigValidationError.Sinks: value must contain exactly 1 item(s)`.

### 11.9 The body-capture reality (AMEND-TAP-BODY — the charter did not anticipate this)

The reference ALWAYS captures bodies (no suppression): `max_buffered_rx_bytes: 0` + `max_buffered_tx_bytes: 0` → `response body = {"as_string": "", "truncated": true}`; `max_buffered_tx_bytes: 4` → `{"as_string": "{\n  ", "truncated": true}`; `JSON_BODY_AS_BYTES` → `{"as_bytes": "ewogICJhcmdz..."}`. **BUT a ZERO-LENGTH body message OMITS `body` entirely** — a bodyless `GET` → `request keys: ['headers','trailers']`; a `204 No Content` response → `response keys: ['headers','trailers']` (arm `emptybody`: `request hdrs: 10`, `response hdrs: 7`, no `body` either side). ⇒ GET→204 yields a structurally headers-only trace on BOTH sides — the 0099 fixture MUST drive GET→204.

### 11.10 `tap_enabled` (f3) appears INERT (AMEND-TAP-TAPENABLED)

Three arms, all tapped (`rq_tapped: 1`): `te_zero_nokey` (default_value 0/HUNDRED, no runtime_key), `te_zero_withkey` (0/HUNDRED + runtime_key), `te_hundred_withkey` (100/HUNDRED + runtime_key). Observation only; envoy-go rejects `tap_enabled` rather than emulating it.

### 11.11 The reject taxonomy — PARITY vs DEPARTURE (§6)

**PARITY** (reference ALSO boot-rejects): NEITHER match set (`Neither match nor match_config is set in TapConfig`); `sinks != 1` (PGV `exactly 1 item(s)`); `streaming_admin` w/o admin (`Specifying admin streaming output without configuring admin.`); `buffered_admin` w/o admin (`requires that the admin output will be configured via admin`); `custom_sink` unregistered (`Didn't find a registered implementation…`). **DEPARTURE** (reference accepts or crashes; envoy-go rejects per ADR-0080): `admin_config` (BOOTS inert); `streaming: true` (BOOTS, 3 concatenated docs — `TOTAL DOCS: 3`); the 2 trailer arms (BOOT + honor); the 2 generic_body arms (BOOT + match — arm `genericbody`: `http_response_generic_body_match{patterns:[{string_match:"args"}]}` → 1 file); PROTO_* formats (BOOT + honor); **`streaming_grpc` (ABORTS the reference — exit 139, `tap_config_base.cc:119 panic: not implemented`; reproduced TWICE with a well-formed HTTP/2 sink cluster)**; `tap_enabled` (accepts, inert); `match_config` (honors alone, ignores when `match` set).

---

## 12. PLAN / IMPL D-questions (not empirical pins)

- **D-TAP-DEPTHCAP.** The `internal/matchpredicate` recursion depth cap value (§3.3). RECOMMEND 32 (a recommendation, not a pin) — deep enough for any realistic predicate, shallow enough to bound the `FuzzMatchPredicateCompile` stack.
- **D-TAP-TRACEID.** The trace-id source (an atomic counter vs `crypto/rand`). The id is never asserted (§8), so a process-local monotonic counter suffices; the PLAN pins it.
- **D-TAP-RECORDFLAGS.** Whether `record_downstream_connection` / `record_headers_received_time` are honored at 56.1 or deferred to 56.2. **RECOMMENDATION (not a pin):** honor `record_downstream_connection` — both `Connection` address fields are plumbable via the landed `DecoderFilterCallbacks.DownstreamLocalAddr()` (`callbacks.go:113`) / `DownstreamRemoteAddr()` (`:103`); DEFER/UNassert `record_headers_received_time` unless a clean per-direction timestamp seam exists (the `Message.headers_received_time` needs the exact header-arrival instant per direction, which no landed accessor exposes). The baseline `0099` fixture leaves both flags unset regardless (§8.1), so this recommendation affects only whether the IMPL lands the `record_downstream_connection` plumbing now or at 56.2.
- **D-TAP-SUBSET.** The exact asserted request-header key/value subset for `0099` (§8.2) — must be cross-side-deterministic (avoid `x-request-id`, `x-forwarded-proto`, etc.).
- **D-TAP-EMITSITE.** (Supersedes the draft's "D-TAP-GUARDSITE", which rested on a refuted double-fire premise — §3.4.) `OnDestroy()` fires exactly once for a both-sided filter, so no guard is needed for correctness. The PLAN decides only whether to add a **defensive** `sync.Once`/bool anyway, and must pin the ONE-SHARED-VALUE constraint (both `HTTPFilter` fields point at the same `*tapFilter`) with a unit test — because an encoder-only `OnDestroy` is unreachable (`chain.go:670`'s `else if`).
- **D-TAP-PATHPREFIX.** `path_prefix` parent-directory semantics: does `filePerTapSink` `os.MkdirAll` the parent, or assume it exists and fail the write? The `0099` fixture supplies an existing per-side temp dir, so it is not blocking — but the IMPL must choose, and the choice belongs in the PLAN.

---

## 13. ADR continuity — the ADR-0273 §Context DRAFT (anchored here; body at the IMPL per ADR-0044)

DECISIONS.md tail stays **ADR-0272** at this SPEC. next-free is **ADR-0273**. **Recommend a SINGLE absorbing ADR-0273** for this leg — no separate SEAM ADR for `internal/headermatch`/`internal/matchpredicate`: the phase-41/42.1 precedent (a single ADR absorbed the anticipated second) applies; the two library packages are the tap filter's private substrate (one consumer), not a general framework seam. This is a recommendation; the PLAN confirms.

> **ADR-0273 — the HTTP tap filter, headers leg (§Context DRAFT).**
>
> The Observability family's NINTH row (after gRPC-ALS @44, OTLP-log @45, tracing @46, metrics_service @47, statsd @48, dog_statsd @49/@50, statsd-tcp @55) picks up the tap filter *(Q0)*. Unlike phases 44-55 (bootstrap-level sinks), tap is a per-listener HTTP filter — the first Observability row to integrate at the HTTP-filter layer, driving real HTTP requests through a dual-sided observer rather than a periodic flush loop.
>
> A live probe against `envoyproxy/envoy:contrib-v1.37.2` (2026-07-10) corrected the charter on twelve counts and pinned the lifecycle. The load-bearing correction: a buffered tap trace is an END-OF-STREAM ARTIFACT assembled from request headers (at `DecodeHeaders`) and response headers (at `EncodeHeaders`) and emitted at stream end ONLY on a match — the reference NEVER early-emits even when a request arm is already true at decode (the `orshort` probe), so the compiled predicate tree needs a tri-state (match/no-match/undetermined) per node to RESOLVE, but emission timing is unconditional. The trace JSON is byte-exact `protojson` with `EmitDefaultValues` (not `EmitUnpopulated`) + a trailing newline. The emit hangs off `OnDestroy`, which — because `FilterChain.Destroy()`'s loop is `if Decoder != nil { … } else if Encoder != nil { … }` — fires exactly ONCE for a both-sided filter, and would never fire at all on an encoder-only value; so tap installs ONE shared `*tapFilter` in both `HTTPFilter` fields (the `compressor` precedent). The response `:status` — absent from the encode header map — is taken from the ADR-0196 accessor and injected into a COPY, never the wire-bound map (a `ReconcileOrderedHeaders` wire-leak trap). Trailers are structurally invisible to envoy-go's filters (the never-done HCM "Task 18"), so the trailer match arms and `Message.trailers` are boot-rejected/never-populated as a coverage boundary — invisible in the differential because `EmitDefaultValues` renders empty trailers `[]` byte-identically. The row adds two library packages (`internal/headermatch` — a fresh exported 8-arm HeaderMatcher, MIGRATE NOBODY; `internal/matchpredicate` — the recursive tri-state tree with a depth cap) and one filter package housing an in-package `file_per_tap` sink; it rejects every unsupported sink/format/arm per ADR-0080, mirroring the reference's own boot-rejects where they exist (PARITY) and rejecting where the reference accepts-or-crashes (DEPARTURE — notably `streaming_grpc`, which ABORTS the reference process). It adds one static counter (`http.<stat_prefix>.tap.rq_tapped`, zero new stat-name rule) and one fuzzer (`FuzzMatchPredicateCompile`).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

| Surface | At SPEC (docs-only) | Anticipated at 56.1 IMPL |
|---|---|---|
| stat surface | 1200 | **1201** (+1; `http.<stat_prefix>.tap.rq_tapped`) |
| fixtures | 100 | **101** (`0099-http-tap-headers`) |
| fuzzers | 52 | **53** (`FuzzMatchPredicateCompile`) |
| BackendKind tail | 38 | **38** (+0) |
| DECISIONS tail | ADR-0272 | **ADR-0273** (next-free ADR-0274) |
| new Go packages | 0 | **2** (`internal/headermatch`, `internal/matchpredicate`) + the filter package |
| new go.mod modules | 0 | **0** (`go mod tidy -diff` anticipated EMPTY) |

Counts at THIS SPEC are UNCHANGED (docs-only). Row 56 STAYS `in-progress` (flips `done` only at the 56.2 IMPL six-gate — `reference_roadmap_split_phase_row_done` + ADR-0106; no parent rollup). **Next → the phase-56.1 PLAN** (decompose §10 into a TDD spine; resolve the six §12 D-questions; FINAL ADR-0045 split-gate re-check against the concrete decomposition).
