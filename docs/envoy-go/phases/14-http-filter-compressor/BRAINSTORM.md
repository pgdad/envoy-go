# Phase 14 Brainstorm — `envoy.filters.http.compressor`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 14 (`http-filter-compressor`), the SEVENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, and `buffer` at phase 13). The next session (lifecycle-state 1 → 2 for phase 14, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10/11/12/13 precedent) authors `docs/envoy-go/phases/14-http-filter-compressor/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-14-http-filter-compressor-brainstorm`, branch `phase-14-http-filter-compressor-brainstorm`, branched from master tip `fd976db` (the phase 13 REVIEW follow-up commit `phase 13 REVIEW follow-up: PROGRESS.md SHA-fill (TBD → 908f052)`). The phase 13 phase-done commit `a05bb6f` and the phase 13 REVIEW commit `908f052` precede the SHA-fill follow-up; `fd976db` is the SHA-fill-follow-up commit.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via 5-question dialogue (Q1 §9 family-row pick — `compression` chosen from `compression / rbac / bandwidth_limit / jwt_authn`; Q2 codec scope — `gzip-only` chosen from `gzip / gzip+brotli / gzip+brotli+zstd / gzip+zstd`; Q3 direction scope — `response-only with request_direction_config silent-ignored` chosen from `silent-ignore / parse-reject / response+request both`; Q4 body algorithm — `Path B buffer-then-compress with documented wire-shape divergence` chosen from `Path B / Path A streaming framework deltas / Path B + chunked hack`; Q5 field envelope — `full envelope; deprecated top-level mirrors silent-ignored` chosen from `full / slim / strict-deprecated-rejected`; Q6 stat surface — `full ~11 counters mirror Envoy` chosen from `full 11 / reduced 4 / vacuous 0`). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0128), and the just-shipped phase 13 + phase 12 + phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 + 12 + 13 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/13-http-filter-buffer/BRAINSTORM.md` section-for-section, reframed for the compressor scope and adapted for its specific surface area (the FIRST §9 row exercising the response-side body modification path; the FIRST §9 row whose differential equivalence on the body axis cannot be byte-exact at the wire level due to compression-library non-determinism between Go's `compress/gzip` and Envoy's libz; the FIRST §9 row whose codec extension surface requires an Any-unmarshal-and-dispatch mechanism inside the filter package). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 14 — this brainstorm cold-started fresh from the §9 heading + the phase 13 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-10.

---

## 1. Mission and scope confirmation (14 only)

ROADMAP row `14 | http-filter-compressor | 13 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 14 is the SEVENTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 56 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 13 phase-done commit `a05bb6f` (with REVIEW at `908f052` and SHA-fill follow-up at `fd976db`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 62: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114); `csrf` shipped in phase 12 (`internal/filter/http/csrf/` per ADR-0120); `buffer` shipped in phase 13 (`internal/filter/http/buffer/` per ADR-0125). Phase 14 ships **the response-side compressor** as the SEVENTH real filter — the canonical Envoy-style "compress upstream response body before forwarding downstream" filter. The chosen branch + directory + Go-package identifier are all `compressor` (matching the Envoy filter type-URL `envoy.filters.http.compressor`); the family-heading word "compression" is the umbrella concept covering this filter plus the future `envoy.filters.http.decompressor` (request-body decompression, a separate filter type, deferred to a future phase).

Phase 14 is also: (i) the FIRST §9 family-row to compress / mutate response body bytes; (ii) the FIRST §9 family-row whose codec extension surface requires an Any-unmarshal-and-dispatch mechanism inside the filter package (the `compressor_library` field is `TypedExtensionConfig` of type Any; even the gzip-only MVP must Any-unmarshal); (iii) the FIRST §9 family-row whose differential equivalence on the body-bytes axis is structurally non-byte-exact (Go's `compress/gzip` and Envoy's libz produce different compressed bytes from the same input — both decompress to the same plaintext, but the compressed bytes themselves diverge); (iv) the FIRST §9 family-row introducing a divergence-window on response wire shape (envoy-go's framework-as-is forces fixed Content-Length emission; Envoy's streaming compressor uses chunked transfer-encoding); (v) the SECOND §9 family-row using the disabled-OR-override 5th canonical per-route discipline (codified at ADR-0125 by phase 13 buffer; phase 14 compressor adds an amendment paragraph confirming the pattern's reusability).

### 1.1 What 14 delivers as a self-contained whole

Phase 14 lands `envoy.filters.http.compressor` (the canonical Envoy compressor filter, response-side, gzip-only) under the 07.1 framework. **Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):**

1. **New `internal/filter/http/compressor/` package** owning the filter implementation. Package directory + Go package identifier are both `compressor` (single token; mirrors the buffer/csrf/cors single-token precedent). Files mirror the `internal/filter/http/buffer/` shape from phase 13: `compressor.go` (filter type + factory + encode methods + per-route helper + filterStats struct + compiledConfig + codec-library dispatch helpers), `compressor_test.go` (unit tests), `fuzz_test.go` (the 18th fuzzer in the repo — `FuzzCompressorConfigParse`), `doc.go` (package overview + 8-consumed/9-deferred decomposition + per-route disabled-OR-override summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 8 entries: `router.New`, `buffer.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains a ninth `httpReg.Register(compressor.TypeURL, compressor.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → buffer → compressor → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → Freeze`. Compressor inserts between `buffer` and `cors` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing of `envoy.extensions.filters.http.compressor.v3.Compressor`,** the canonical filter-level config message. Per `go-control-plane`'s v1.32.4 module (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 9 top-level fields; phase 14 consumes a subset and silent-ignores the rest. **Consumed (8 effective fields, accounting for the response-side direction-config nesting):**

   - `compressor_library` (`TypedExtensionConfig`, REQUIRED) — Any-unmarshalled to a codec sub-config. MVP recognizes ONE TypeURL: `type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip`. Other TypeURLs are PARSE-rejected with envoy-go-own error wording `compressor: unsupported compressor_library TypeURL <url>; phase-14 MVP supports only envoy.extensions.compression.gzip.compressor.v3.Gzip`. Inside the Gzip sub-message, the consumed fields are: `compression_level` (mapped to Go `compress/gzip` Best/Default/HuffmanOnly/level-N levels per ADR-0130 mapping table) and `compression_strategy` (only `HUFFMAN_ONLY` is honored — maps to `gzip.HuffmanOnly`; all other strategies — `DEFAULT_STRATEGY`, `FILTERED`, `RLE`, `FIXED` — collapse to the gzip default since Go's `compress/gzip` does not expose libz-equivalent strategy knobs).

   - `response_direction_config.common_config.min_content_length` (`UInt32Value`) — minimum response body size to compress. Default unset → no minimum (compress regardless of size, subject to other gates). Honored at `EncodeData` time against the accumulated `resp.Body` length.

   - `response_direction_config.common_config.content_type` (`[]string`) — content-types eligible for compression. Default unset → Envoy's documented default list (per §11 empirical pin): `application/javascript`, `application/json`, `application/xhtml+xml`, `image/svg+xml`, `text/css`, `text/html`, `text/plain`, `text/xml`. Match algorithm and case-sensitivity confirmed by §11 pin (hypothesis: case-insensitive prefix-match; matches `text/html; charset=utf-8` against `text/html`).

   - `response_direction_config.disable_on_etag_header` (`bool`) — when true, compression is skipped if the upstream response carries an `ETag:` header (strong OR weak — §11 pin to confirm weak-ETag handling). Default false.

   - `response_direction_config.remove_accept_encoding_header` (`bool`) — when true, the compressor strips `Accept-Encoding` from the request before forwarding upstream. Useful for pin-compression-policy at the proxy (origin always sees identity request; compressor handles Accept-Encoding negotiation client-facing). Default false.

   - `response_direction_config.uncompressible_response_codes` (`[]uint32`) — response status codes for which compression is skipped. Default `[]` per Envoy v1.37.2 (the historical-default `[206]` was removed; §11 pin confirms exact default). MVP consumes the field as configured; if unset, falls back to the empirical default list.

   The 4 deprecated top-level mirrors of these fields — `Compressor.content_length`, `Compressor.content_type`, `Compressor.disable_on_etag_header`, `Compressor.remove_accept_encoding_header` — are silent-ignored at parse time. Operators who set both legacy + new will see the new (response_direction_config) values honored and the legacy values silent-discarded; documented as an operator-footgun in BEHAVIOR_CONTRACT phase-14 forward-pointer notes.

4. **Per-route TPFC: `disabled` boolean OR `CompressorOverrides` shape (5th canonical disabled-OR-override; ADR-0125 amendment).** Per the proto message `CompressorPerRoute` (the per-route-TPFC type, separate from the listener-level `Compressor` — UNLIKE phase 12 csrf where the same `CsrfPolicy` served both purposes; LIKE phase 13 buffer where the per-route type is separate), per-route entries carry a oneof with two cases: (a) `disabled: true` — the filter is wholly inactive on this route, no compression, no counter increments, response body forwards as-is to the downstream wire; (b) `overrides: { response_direction_config: { … } }` — a wholesale override of the listener-level `response_direction_config` (NOT a merge; mirrors phase 13 buffer override semantics + phase 12 csrf wholesale-override per ADR-0073). Both shapes are honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (disabled OR override) drives the disposition. **Phase 14 is the SECOND row to use the disabled-OR-override discipline** codified by ADR-0125 (phase 13 buffer was the FIRST). ADR-0125 gains an amendment paragraph confirming the pattern's reusability + noting the WHOLESALE-not-merge override semantic for `overrides.response_direction_config`. NO new per-route ADR for phase 14.

5. **Encode-side filter callbacks: `EncodeHeaders` + `EncodeData` body.** The filter implements `StreamEncoderFilter` (NOT `StreamDecoderFilter` — phase 14 is the FIRST §9 row to be encoder-only modulo the request-side `remove_accept_encoding_header` knob, which fires on the decode path). Both interfaces are statically asserted via blank-identifier compile-time checks (matching cors/fault/header_mutation/local_ratelimit/csrf/buffer precedents). The decode-side surface is minimal: `DecodeHeaders` strips `Accept-Encoding` from the request when `remove_accept_encoding_header=true` AND otherwise stores the resolved `compiledPerRoute` + the requested encoding (parsed from `Accept-Encoding`) in per-stream filter state. `DecodeData` / `DecodeTrailers` are pass-through. The encode-side surface is the algorithmic core: see §2.6 + §2.8.

6. **Body algorithm: Path B (buffer-then-compress); wire-shape divergence accepted (Decision 6 → ADR-0131).** envoy-go's encode-chain framework (`internal/filter/hcm/connection.go:467-475`) currently invokes `RunEncodeData(ctx, resp.Body, true)` ONCE with the full response body in a single call, `endStream=true`. The encode chain is not streaming today; HCM materializes `resp.Body []byte` upstream-side and the filter sees the whole body at once. Combined with `writeH1Reply` (`internal/filter/hcm/codec.go:74-119`) unconditionally emitting fixed `Content-Length: <len(body)>` (no chunked-encoded H1 response output is supported), the framework as-is forces **Path B (buffer-then-compress)**: the filter operates on the already-accumulated `resp.Body`. `EncodeHeaders` (a) computes the skip decision (Accept-Encoding missing, content-type mismatch, etag-disable, uncompressible-status, already-compressed, etc.); (b) on compress: sets `Content-Encoding: gzip`, sets/extends `Vary: Accept-Encoding`; otherwise falls through. `EncodeData` (a) on compress + endStream=true: gzip-encodes `resp.Body` in one shot via `gzip.NewWriter(...).Write(...).Close()`, replaces `resp.Body` with the compressed bytes via `cb.OverwriteBody` (or its phase-14 introduction; see §3); the framework's `writeH1Reply` then emits `Content-Length: <gzipped-length>` matching the new body length; (b) otherwise pass-through.

   **Wire-shape divergence from reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-14 forward-pointer notes): Envoy uses streaming compression and emits `Transfer-Encoding: chunked` (no `Content-Length`); envoy-go's MVP emits `Content-Length: <gzipped-length>` (no `Transfer-Encoding`). Decompressed body bytes are byte-equivalent (gzip is a well-defined format; both libraries decode to the same plaintext). Compressed body BYTES are NOT byte-equivalent (Go's `compress/gzip` and Envoy's libz make different block-boundary + Huffman-tree choices; this is by design of the gzip format spec — multiple valid encodings of the same input exist). §11 empirical pin §9.P9 confirms Envoy's exact wire shape under fixture conditions (small responses; whether Envoy emits CL when body is fully buffered upstream-side under `direct_response`, or always chunks). ADR-0131 records the divergence + the explicit forward-pointer to a future encode-side streaming framework phase that may revisit (analogous to phase-13's ADR-0127 v2 documenting body-counting algorithm divergence with forward-pointer to the cap-promotion phase). The forward-pointer phase would land symmetric encode-side primitives in `internal/filter/hcm/connection.go` (analogous to phase-13 ADR-0128's decode-side primitives) + `writeH1Reply` chunked-output mode + a new `EncoderFilterCallbacks.EmitChunk` API.

7. **Stat surface — 29→40-name extension (Decision 7 → ADR-0132).** **11 new counters** under `BEHAVIOR_CONTRACT.md ## Stat-name mapping` (extending the phase-13 29-name table to 40 names), mirroring Envoy's full compressor counter set as confirmed by §11 empirical pin (hypothesis; SPEC may amend exact names per §9.P5). Hypothesized names + scope (per Envoy v1.37.2 docs `configuration/http/http_filters/compressor_filter`):

   - `compressed` — counter; once per request whose response was compressed (gzip applied).
   - `not_compressed` — counter; once per request whose response went uncompressed (any reason).
   - `no_accept_header` — counter; client request had no `Accept-Encoding` header.
   - `header_compressor_used` — counter; this codec was the negotiated selection.
   - `header_compressor_overshadowed` — counter; this codec was selectable but overshadowed by a higher-q-value alternative.
   - `header_identity` — counter; client requested identity (no compression).
   - `header_not_valid` — counter; `Accept-Encoding` header was malformed (q-value parse error, unknown coding, etc.).
   - `header_wildcard` — counter; client sent `Accept-Encoding: *` (wildcard).
   - `total_compressed_bytes` — counter; cumulative compressed body bytes emitted.
   - `total_uncompressed_bytes` — counter; cumulative uncompressed body bytes consumed.
   - `content_length_too_small` — counter; response body below `min_content_length`.

   **Stat namespace + Prometheus tag-extractor:** §11 empirical pin §9.P5 confirms exact stat path. Hypothesis: `http.<HCM stat_prefix>.<filter stat_prefix>.compressor.gzip.<counter>` (codec library name embedded). New Prometheus tag-extractor **Rule SN10** (next-after-SN9 per ADR-0118 phase-11 SN9) — pattern `envoy_http_compressor_<filter_stat_prefix>_<counter>` extracting the filter-stat-prefix tag (and possibly the codec-library tag — §9.P5 confirms). ADR-0132 codifies the 11-counter surface + the new SN10 flattening rule. **Per-route stats SHARED with listener-level** (no independent per-route stat namespace; mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 SHARED-stats pattern; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats).

8. **No new framework primitive on the encode side; one new framework helper.** Phase 14 reuses (a) `SendLocalReply` from fault/local_ratelimit/csrf/buffer precedent (NOT used directly — compressor never short-circuits to a local reply; this is a contrast point with all 6 prior §9 rows); (b) the 3-tier `PerRouteConfig.Resolve` from phase 07.1; (c) the existing encode-chain shape at `connection.go:467-475` (one-shot full-body `RunEncodeData`); (d) the existing `writeH1Reply` fixed-Content-Length wire emission. Phase 14 introduces ONE new framework helper IF NEEDED (§3 evaluates): an `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive that lets a filter replace `resp.Body` mid-`EncodeData`. If `EncodeData` already supports body-mutation via the existing callback shape (e.g., the `data` slice passed in is the addressable body), no new primitive is needed and the filter mutates in-place. §3 below evaluates the encode-chain machinery in detail and commits to the precise primitive set — this is a brainstorm-time RISK that the SPEC author resolves IN-SESSION at the framework-survey step. Phase 14 adds NO new HTTPFilterFactoryCtx field, NO new HTTPRegistry method, NO new PerRouteConfig accessor.

**Plus three artifact-level deliverables:**

9. **Differential fixture `0016-http-compressor`** under `test/fixtures/0016-http-compressor/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising six scenarios per §6 below. The fixture asserts response status, **decompressed-body byte-exact** (the 1st §9 row to use a decompress-and-compare body assertion — codified at ADR-0133), header set lowercase wire-form (excluding the `Content-Length` value and `Transfer-Encoding` axis where Path B diverges from Envoy), counter deltas via `/stats/prometheus` scrape equivalence, and per-route-tier independent disposition (both `disabled` and `overrides` shapes exercised). NO timing-sensitive scenarios (compressor is purely synchronous — no analog to phase 11's `refill-after-fill_interval ±10ms` scenario).

10. **`BEHAVIOR_CONTRACT.md` 4-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing `### envoy.filters.http.fault`, `### envoy.filters.http.header_mutation`, `### envoy.filters.http.local_ratelimit`, `### envoy.filters.http.csrf`, `### envoy.filters.http.buffer` subsections): a NEW `### envoy.filters.http.compressor` subsection covering the 8-consumed / 9-ignored field map, the response wire shape (status passthrough, body bytes compressed via gzip, `Content-Encoding: gzip`, `Vary: Accept-Encoding`, `Content-Length: <gzipped>` rewrite, identity transfer-encoding), the per-route disabled-OR-override semantics, the gzip-only codec scope + parse-rejection of unknown TypeURLs, the Path B body algorithm + wire-shape divergence-window from Envoy. Plus the 29→40-name stat-table extension + the new SN10 Prometheus tag-extractor flattening rule. Plus a new equivalence-matrix row pointing at fixture 0016 with per-scenario tolerance discipline (decompress-and-compare body axis; CL-value + transfer-encoding excluded from byte-exact assertion). Plus a NEW `### Phase 14 forward-pointer notes` subsection under `## Forward-pointer notes` covering the ~6-item deferral list (per §8 below).

11. **Anticipated 5 ADRs (ADR-0129 through ADR-0133)** per §7 below. ADR-0128 is the highest-numbered ADR landed in phase 13; ADR-0129 is the next-free.

### 1.2 What 14 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12/13 precedent; deferrals are 6 items grouped by family-coupling). The summary: brotli + zstd codec extensions; request-side compression via `request_direction_config` activation; the future `envoy.filters.http.decompressor` filter (separate filter type; deferred); chunked-encoded response wire shape (couples to encode-side streaming framework primitives); Gzip codec sub-fields not expressible in Go's `compress/gzip` (`memory_level`, `window_bits`, `chunk_size`); `Compressor.choose_first` first-acceptable-not-q-value selection mode (always-q-value-based MVP); `Compressor.runtime_enabled` deprecated runtime gate (silent-ignored, always-100%); `response_direction_config.common_config.enabled` runtime gate (silent-ignored, always-100% per phase-12 csrf-style divergence-window). None are blockers for closing row 14 phase-done.

### 1.3 Phase-done as a §9 family-row landing

Phase 14's phase-done commit closes ROADMAP row `14` (single-row, no parent-child split anticipated; see §1.4). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships, but no row tracks that aggregate. Phase 14 is the SEVENTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, 12-csrf, 13-buffer). The next §9 family-row will be numbered `15` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness

The brainstorm's POSITION is that phase 14 is **single-row at brainstorm time** — a cohesive ~1100-1400 LoC implementation slice covering a single filter — but the planner-time release valve stays available. If the SPEC author finds the surface > 1500 LoC estimated or the PLAN > 25 tasks, the natural split would be:

- **14.1 = listener-level filter MVP**: the filter type + factory + `EncodeHeaders` + `EncodeData` body (gzip compress; skip predicates) + `compiledConfig` parsing (full field envelope; codec-library Any-unmarshal + gzip dispatch) + 11-counter stats + new SN10 tag-extractor. Differential fixture covers listener-only scenarios (1, 2, 3, 4 from §6.2). NO per-route. NO `disable_on_etag_header`. NO `uncompressible_response_codes`.
- **14.2 = per-route disabled-OR-overrides TPFC + remaining skip predicates**: per-route `CompressorPerRoute` parsing + 3-tier resolver wiring + per-route-disabled fixture scenario (5) + per-route-override fixture scenario (6) + ADR-0125 amendment paragraph + `disable_on_etag_header` + `uncompressible_response_codes`.

This split mirrors phase 10 + phase 11 + phase 12 + phase 13's anticipated-but-unused split. The brainstorm does NOT pre-commit to the split; that's the SPEC author's call. The single-row position is supported by the LoC estimate (~280-330 impl + ~100 codec adapter + ~500-600 tests + ~80 fuzzer + ~150-200 fixture-Go-driver/backend + ~150 fixture-yaml/README = ~1100-1400 total when including yaml configs and README; ~480-530 if counting Go production code alone). Task count estimate: ~12-16 tasks. Both estimates remain comfortably under ADR-0045's 1500 LoC / 25 task split-trigger upstream of either accounting. Phase 14 is structurally larger than phase 13 at the proto-surface level (8 consumed fields vs. 1 field) but comparable at the algorithmic level (compressor's body-algorithm is 1-shot gzip + Vary/Content-Encoding header injection vs. buffer's body-counting state machine + 413-overflow path).

### 1.5 Seed-stub alignment

Like phases 09, 10, 11, 12, and 13, phase 14 has NO sibling SPEC stub — phase 14 enters fresh after the phase 13 close. The §9 family-children list at ROADMAP line 62 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`, plus the future `envoy.filters.http.decompressor` companion to compressor). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 14 has NO such branch. The brainstorm dialogue (Q1-Q6 over the user-Claude exchange) was sufficient to settle filter pick + codec scope + direction scope + body algorithm + field envelope + stat surface without preliminary scoping notes. This matches the phase 09 / 10 / 12 / 13 cold-start precedent.

### 1.7 Phase 14's relationship to phase 13 framework deltas + ADR-0076 cap-promotion forward-pointer

Phase 13 introduced two framework primitives at `internal/filter/hcm/connection.go` per ADR-0128: synthetic empty-terminal `RunDecodeData` on chunked-body EOF + post-body Content-Length reconciliation propagating filter-set Content-Length into `req.ContentLength`. Phase 14 **does NOT consume** ADR-0128 directly — those primitives are decode-side; phase 14 is encoder-only on the algorithmic-core path. However, phase 14's wire-shape divergence (Path B fixed Content-Length vs. Envoy's chunked) sits in the SYMMETRIC position to ADR-0128's decode-side primitives: a future framework phase that adds encode-side streaming + chunked-output is the natural amender for phase 14's wire-shape divergence (mirroring how the future cap-promotion phase is the natural amender for phase 13's `max_request_bytes ≤ 1 MiB` + `per_connection_buffer_limit_bytes` silent-ignore per ADR-0076 + ADR-0126).

Phase 14 also **does NOT promote ADR-0076's `filterBufferLimitBytes` cap.** Phase 13 forward-pointer notes named compression as the natural amender of the per-connection / per-request buffer-limit fields; that hypothesis pre-supposed compression would consume the request-body modification path. Phase 14 MVP is response-only — response body sizing is governed by upstream's bytes, not by the `per_connection_buffer_limit_bytes` cap (which targets request-body buffering on the decode side). The cap-promotion phase therefore remains forward-pointed; phase 14 does NOT close it. This is a non-blocker brainstorm-time observation; the next §9 row under family-coupling for cap-promotion (e.g., a hypothetical `decompressor` row, OR a global `filterBufferLimitBytes` knob promotion phase outside §9) inherits the forward-pointer.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 8 decisions below are the phase-14-specific design choices. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 Filter package layout *(Decision 1 → ADR-0129)*

`internal/filter/http/compressor/` (single-token package directory; mirrors `buffer/`, `csrf/`, `cors/`, `fault/`). 4-file split (`compressor.go`, `compressor_test.go`, `fuzz_test.go`, `doc.go`), mirroring phase 13. The Go package identifier `compressor` matches the directory. Package-level constants: `TypeURL` (the canonical type-URL string `"type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"`), `filterName` (`"envoy.filters.http.compressor"`). Package-level types: `compiledConfig`, `compiledPerRoute`, `compiledOverrides`, `compiledGzipConfig`, `filter`, `filterStats`. Package-level functions: `New` (the public `HTTPFilterFactory`), plus unexported helpers (`buildCompiledConfig`, `parsePerRoute`, `unmarshalCompressorLibrary`, `negotiateEncoding`, `shouldCompress`, `computeContentTypeMatch`, etc.). NO additional sub-packages; the codec-library dispatch lives in the same package as `gzipCodec` helpers (no separate `internal/filter/http/compressor/gzip/` sub-package — keeps the single-codec MVP layout flat; future codec additions may migrate to a sub-package per ADR-0130 §Consequences).

### 2.2 Extension-registry registration *(Decision 2 → ADR-0129 consequence)*

`cmd/envoy-go/main.go` adds one new line: `httpReg.Register(compressor.TypeURL, compressor.New)`, alphabetical-after-`buffer.New` per the ADR-0072 stylistic convention. Insertion ordering: the existing 8-entry list (router, buffer, cors, csrf, envoygotest, fault, header_mutation, localratelimit) becomes a 9-entry list (router, buffer, **compressor**, cors, csrf, envoygotest, fault, header_mutation, localratelimit). NO stats-registry surgery (the stats are emitted via the existing `*stats.Registry` per ADR-0118 SN9 framework + the new SN10 rule per ADR-0132). NO bootstrap-loader surgery.

### 2.3 MVP envelope: 8-consumed/9-ignored field decomposition *(Decision 3 → ADR-0130)*

Per Q5 = "Full envelope; deprecated top-level mirrors silent-ignored", the field decomposition is:

**CONSUMED at runtime (8 effective fields):**
1. `Compressor.compressor_library` (TypedExtensionConfig; REQUIRED) — Any-unmarshal; gzip-only dispatch.
2. `Compressor.response_direction_config.common_config.min_content_length` (UInt32Value).
3. `Compressor.response_direction_config.common_config.content_type` ([]string).
4. `Compressor.response_direction_config.disable_on_etag_header` (bool).
5. `Compressor.response_direction_config.remove_accept_encoding_header` (bool).
6. `Compressor.response_direction_config.uncompressible_response_codes` ([]uint32).
7. `Gzip.compression_level` (mapped to `compress/gzip` levels).
8. `Gzip.compression_strategy` (`HUFFMAN_ONLY` only; others map to gzip default).

**SILENT-IGNORED at runtime (9 fields; behavior-divergence-windows documented per phase-12 csrf-style at BEHAVIOR_CONTRACT phase-14 forward-pointer notes):**
1. `Compressor.content_length` (UInt32Value, deprecated top-level mirror) — operator-footgun if both legacy + new are set.
2. `Compressor.content_type` ([]string, deprecated top-level mirror) — same operator-footgun.
3. `Compressor.disable_on_etag_header` (bool, deprecated top-level mirror) — same operator-footgun.
4. `Compressor.remove_accept_encoding_header` (bool, deprecated top-level mirror) — same operator-footgun.
5. `Compressor.runtime_enabled` (RuntimeFeatureFlag, deprecated) — always-100% (filter active); divergence-window if Envoy-side enabled at < 100%.
6. `Compressor.response_direction_config.common_config.enabled` (RuntimeFeatureFlag) — always-100%; divergence-window if Envoy-side enabled at < 100%. Mirrors phase-12 csrf `filter_enabled` posture per ADR-0121 — fixture configs MUST set explicitly to 100%/HUNDRED on Envoy side (§11 pin §9.P3 confirms parse-time PGV requirement).
7. `Compressor.request_direction_config` (RequestDirectionConfig) — always-disabled (response-only MVP); divergence-window if Envoy-side `enabled=true` (Envoy actively compresses request bodies upstream; envoy-go does not).
8. `Compressor.choose_first` (bool) — always-q-value-based selection; divergence-window if `choose_first=true` (Envoy uses first-acceptable not q-value).
9. `Gzip.{memory_level, window_bits, chunk_size}` (3 sub-fields) — Go's `compress/gzip` does not expose libz-equivalent knobs; silent-ignored. Compressed-byte output already differs from Envoy's libz regardless (compressed-byte equivalence is structurally non-byte-exact; see §6.3).

**PARSE-REJECTED (envoy-go-only validation):**
- `Compressor.compressor_library` with unknown TypeURL (anything other than `type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip`) — error wording `compressor: unsupported compressor_library TypeURL <url>; phase-14 MVP supports only envoy.extensions.compression.gzip.compressor.v3.Gzip`. Mirrors phase-13 buffer's `max_request_bytes > 1 MiB` envoy-go-only-check pattern (ADR-0126); future codec phases AMEND ADR-0130 to expand the recognized TypeURL set.

### 2.4 Codec-library dispatch shape *(Decision 4 → ADR-0130 consequence)*

The `compressor_library` field is a `TypedExtensionConfig` (Any-typed `typed_config` + `name` string). The MVP dispatch:

1. `unmarshalCompressorLibrary(library *corev3.TypedExtensionConfig) (*compiledGzipConfig, error)` — extracts `library.GetTypedConfig()`, checks `TypeUrl` against the gzip TypeURL constant, invokes `library.GetTypedConfig().UnmarshalTo(&gzipPB)`, then projects to `*compiledGzipConfig` via `buildCompiledGzipConfig`.
2. `buildCompiledGzipConfig(pb *gzipv3.Gzip) (*compiledGzipConfig, error)` — maps `compression_level` enum → Go gzip level int (per ADR-0130 mapping table); maps `compression_strategy` enum → Go gzip strategy. Validates `memory_level ∈ [1,9]` and `window_bits ∈ [9,15]` per Envoy PGV mirror BUT silent-ignores the values.

The dispatch is internal to the `compressor` package; future codec additions (brotli, zstd) extend the dispatch with new `unmarshalBrotliLibrary` / `unmarshalZstdLibrary` helpers + a registry pattern WITHIN the package. The SPEC author may choose to refactor to a registry-of-codecs pattern at MVP time IF the codec-library dispatch grows organically (e.g., if the SPEC author finds the gzip dispatch is best expressed as `codecRegistry.Lookup(typeURL)`). This is a SPEC-time judgment call; the brainstorm's position is "hardcoded gzip dispatch; future codec additions trigger registry pattern."

**Compression-level mapping table** (Envoy `Gzip.CompressionLevel` enum → Go `compress/gzip` level constant):

| Envoy enum | Go gzip constant | Numeric level |
|---|---|---|
| `DEFAULT_COMPRESSION` (0) | `gzip.DefaultCompression` | -1 |
| `BEST_SPEED` / `COMPRESSION_LEVEL_1` (1) | `gzip.BestSpeed` | 1 |
| `COMPRESSION_LEVEL_2` (2) | (no constant) | 2 |
| `COMPRESSION_LEVEL_3` (3) | (no constant) | 3 |
| `COMPRESSION_LEVEL_4` (4) | (no constant) | 4 |
| `COMPRESSION_LEVEL_5` (5) | (no constant) | 5 |
| `COMPRESSION_LEVEL_6` (6) | (no constant) | 6 |
| `COMPRESSION_LEVEL_7` (7) | (no constant) | 7 |
| `COMPRESSION_LEVEL_8` (8) | (no constant) | 8 |
| `BEST_COMPRESSION` / `COMPRESSION_LEVEL_9` (9) | `gzip.BestCompression` | 9 |

`gzip.NewWriterLevel` accepts any int ∈ [-1, 9]; numeric-passthrough is correct for levels 2-8. §11 empirical pin §9.P14 confirms the hypothesis that Envoy's libz default-level matches Go's gzip default (both compute the same compressed-bytes-decompresses-to-the-same-input invariant; level mapping is a hint to compression behavior, not a wire-format constraint).

### 2.5 Per-route TPFC discipline *(Decision 5 → reuses ADR-0125; amendment paragraph only)*

`CompressorPerRoute` proto:
```proto
message CompressorPerRoute {
  oneof override {
    bool disabled = 1;
    CompressorOverrides overrides = 2;
  }
}
message CompressorOverrides {
  ResponseDirectionConfig response_direction_config = 1;
}
```

This is **structurally the same shape as `BufferPerRoute`** (oneof of `disabled` boolean OR a typed override sub-message). Phase 14 is the **SECOND row** to use the disabled-OR-override 5th canonical per-route discipline codified at ADR-0125. The override sub-message (`CompressorOverrides.response_direction_config`) WHOLESALE-overrides the listener-level `response_direction_config` — per ADR-0073 wholesale-override semantic + phase-13 buffer's `BufferPerRoute.buffer.max_request_bytes` precedent (NOT a merge; the per-route entry's `response_direction_config` REPLACES the listener-level entirely).

`parsePerRoute` flow:
1. If `disabled: true` → produce `*compiledPerRoute{disabled: true}`.
2. If `overrides: { response_direction_config: { … } }` → produce `*compiledPerRoute{disabled: false, overrideConfig: <built compiledConfig from the override response_direction_config>}`.
3. Empty/oneof-not-set → reject at parse with envoy-go-own error `compressor: per-route entry has no override field set; expected one of {disabled, overrides}`.

Resolution flow at request time (mirrors phase 13 buffer):
1. `PerRouteConfig.Resolve(ctx)` → most-specific `*compiledPerRoute` for this route.
2. If `disabled=true` → set `f.passthrough=true`; `EncodeHeaders` + `EncodeData` short-circuit pass-through.
3. If `disabled=false` AND `overrideConfig != nil` → use `overrideConfig` for the response-direction-config; otherwise use the listener-level config.

**Per-route stats SHARED with listener-level** (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 SHARED-stats). Compressor's filterStats are scoped at HCM-stat-prefix + filter-stat-prefix (NOT per-route); the per-route override changes WHICH config drives compression decisions, but the counter-emission scope stays at filter-stat-prefix. ADR-0125 gains an amendment paragraph noting the SECOND ROW to use disabled-OR-override + the WHOLESALE-not-merge semantic for `overrides.response_direction_config`. NO new per-route ADR.

### 2.6 Body algorithm: Path B (buffer-then-compress) + wire-shape divergence *(Decision 6 → ADR-0131)*

Per Q4 = "Path B (buffer-then-compress); document wire-shape divergence", the algorithm:

**`EncodeHeaders(headers, endStream)` body:**
1. Resolve `*compiledPerRoute` via `f.resolvePerRoute()` cached at decode time. If `f.passthrough=true` → `Continue` (no headers mutation).
2. Compute the effective `compiledConfig` (per-route override OR listener-level).
3. Compute the skip decision (sequence in order):
   a. `f.acceptedEncoding == ""` (request had no `Accept-Encoding` OR wildcard with identity preferred) → `not_compressed +1`, `no_accept_header +1` (or `header_identity +1` / `header_wildcard +1` per request-header parse); `Continue` (no compression).
   b. Response status ∈ `compiledConfig.uncompressibleResponseCodes` → `not_compressed +1`; `Continue`.
   c. `headers.Get("Content-Encoding")` already set (not `identity`) → `not_compressed +1`; `Continue` (don't recompress).
   d. `compiledConfig.disableOnETag` AND `headers.Get("ETag") != ""` → `not_compressed +1`; `Continue`.
   e. `headers.Get("Cache-Control")` contains `no-transform` → `not_compressed +1`; `Continue` (RFC 7234 §5.2.2.4 compliance; §11 pin §9.P12 confirms Envoy honors this).
   f. `headers.Get("Content-Type")` does NOT match any entry in `compiledConfig.contentTypes` → `not_compressed +1`; `Continue`.
4. If all gates pass: `f.willCompress = true`; mutate headers in-place — set `Content-Encoding: gzip`; `Vary: Accept-Encoding` (append-or-set per phase-13 buffer's set-or-append precedent at ADR-0127 §Decision (iv); §11 pin §9.P10 confirms exact Envoy behavior on multi-Vary). Strip `Content-Length` from headers (the framework's `writeH1Reply` will rewrite to compressed-length at wire time per `codec.go:87-89`). `Continue`.

**`EncodeData(data, endStream)` body:**
1. If `f.passthrough` OR `!f.willCompress` → `DataContinue` (pass-through).
2. If `endStream=false`: `DataContinue` (HCM accumulates the body in `resp.Body` per the existing encode-chain flow; the filter will see the full body on the terminal call). NOTE: per `connection.go:472` analysis, `RunEncodeData` is invoked ONCE with `endStream=true` always; the `endStream=false` branch is defensive (for future framework changes).
3. If `endStream=true` AND `f.willCompress`:
   a. Check `len(data) >= compiledConfig.minContentLength` → if NOT, `not_compressed +1`, `content_length_too_small +1`; revert headers (un-set `Content-Encoding`, restore `Vary`); `DataContinue`. (NOTE: this is the late-stage min-content-length gate; if Content-Length was known in the upstream response and exceeded by the headers-time gate, the filter could short-circuit earlier — but Path B's late-binding to body length means the gate fires here.)
   b. Otherwise: gzip-compress `data` via `gzip.NewWriterLevel(buf, level).Write(data).Close()` with `level` per `compiledGzipConfig.level`; replace `data` with `buf.Bytes()` via `cb.OverwriteBody(buf.Bytes())` (or in-place mutation — §3 evaluates exact callback shape); emit counter increments `compressed +1`, `total_uncompressed_bytes += len(data)` (pre-compression), `total_compressed_bytes += len(buf.Bytes())` (post-compression), `header_compressor_used +1`. `DataContinue`.

**`EncodeTrailers` / `OnDestroy`:** pass-through.

**Decode side (minimal):**
- `DecodeHeaders(headers, endStream)`: resolve per-route → cache `*compiledPerRoute` on filter state; parse `Accept-Encoding` (q-value parser) → cache `acceptedEncoding string` ("gzip" / "" / "identity" / etc.); if `compiledConfig.removeAcceptEncodingHeader=true` → strip `Accept-Encoding` from headers. `Continue`.
- `DecodeData` / `DecodeTrailers`: pass-through.

**Wire-shape divergence summary** (ADR-0131 records):
- envoy-go: `Content-Encoding: gzip`, `Content-Length: <gzipped-length>`, no `Transfer-Encoding`, no chunked.
- Envoy: `Content-Encoding: gzip`, `Transfer-Encoding: chunked`, no `Content-Length` (streaming; compressed-length unknown at headers-time).
- Decompressed body bytes: byte-equivalent (gzip is well-defined; both libraries decode identically).
- Compressed body bytes: NOT byte-equivalent (Go `compress/gzip` and Envoy libz make different block-boundary + Huffman-tree choices).
- §11 empirical pin §9.P9 confirms Envoy's exact wire shape under fixture conditions.

### 2.7 Stat surface — 29→40-name extension *(Decision 7 → ADR-0132)*

Per Q6 = "Full ~11 counters mirror Envoy", the 11 new counters listed in §1.1 item 7 are added to the stat surface. **Hypothesis** (SPEC §11 pin §9.P5 confirms or amends): names match Envoy verbatim; scope is `http.<HCM stat_prefix>.<filter stat_prefix>.compressor.gzip.<counter>` (codec library name embedded in path; this is a strong hypothesis that would make compressor the FIRST row whose stats are codec-tagged).

**New Prometheus tag-extractor — Rule SN10** (next-after-SN9 per ADR-0118 phase-11): pattern `envoy_http_compressor_<filter_stat_prefix>_<codec>_<counter>` extracting BOTH the filter-stat-prefix tag AND the codec-library tag (e.g., `envoy_http_compressor_my_filter_gzip_compressed{filter_stat_prefix="my_filter", codec="gzip"}`). The SN10 rule extends the precedent of SN9 (`envoy_local_http_ratelimit_<filter_stat_prefix>_<counter>`) with a second extracted tag. ADR-0132 codifies the SN10 rule + the 11-counter surface.

If §11 pin §9.P5 finds that Envoy does NOT embed the codec-library name in the stat path (i.e., stats are at `http.<HCM stat_prefix>.<filter stat_prefix>.<counter>` without `.compressor.gzip.` infix), then SN10 simplifies to `envoy_http_compressor_<filter_stat_prefix>_<counter>` (single extracted tag; mirrors SN9 shape exactly). Brainstorm hypothesis is the codec-tagged path; SPEC pin resolves.

**Per-route stats discipline:** SHARED with listener-level (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT). No per-route stat namespace.

**Stat surface count summary:**
- Phase 11 (local_ratelimit): 22 → 26 names (4 new counters; SN9 introduced).
- Phase 12 (csrf): 26 → 29 names (3 new counters; reuses HCM-stat-prefix tag-extractor).
- Phase 13 (buffer): 29 → 29 names (0 new counters; vacuous; reuses HCM-stat-prefix per ADR-0125).
- Phase 14 (compressor): 29 → **40 names** (11 new counters; SN10 introduced).

### 2.8 Wire shape details + Accept-Encoding parsing *(Decision 8 → ADR-0131 consequence)*

**Vary header injection:** when compression is applied, the filter sets/extends `Vary: Accept-Encoding`. If `Vary` is already present:
- If existing value contains `Accept-Encoding` (case-insensitive token-match) → no change.
- If existing value is `*` → no change (wildcard already implies all headers vary).
- Otherwise → APPEND (single-header value extended with `, Accept-Encoding`; preserves existing tokens).
- §11 pin §9.P10 confirms Envoy's exact Vary semantics; brainstorm hypothesis matches RFC 7231 §7.1.4.

**Content-Encoding header:** set to `gzip` on the compressed path. If `Content-Encoding` was already present pre-compression (e.g., upstream set `Content-Encoding: identity`):
- `identity` (RFC default) → REPLACED with `gzip`.
- Anything else (e.g., `gzip`, `br`, `deflate`) → DOES NOT compress (skip per §2.6 step 3c). §11 pin §9.P11 confirms.

**Content-Length header:** stripped at `EncodeHeaders` time; framework's `writeH1Reply` (per `codec.go:87-89`) rewrites unconditionally to `len(body)` at wire time, which equals the compressed length post-`EncodeData`-mutation. NO `Content-Length` arithmetic in the filter; the framework owns the rewrite.

**Accept-Encoding parser (decode-side):**
- Parse the `Accept-Encoding` request header per RFC 7231 §5.3.4 — comma-separated list of codings with optional q-values.
- Default q-value is 1.0 if not specified; q=0 means "not acceptable".
- The parser produces a list of `(coding, qValue)` tuples sorted by q-value desc.
- Wildcard `*` matches any coding not explicitly named; q=0 on `*` blocks codings not in the list.
- The filter selects the highest-priority coding that matches a configured codec (gzip-only MVP). If no match → `not_compressed`; emit appropriate counter (`header_identity` if `identity` is highest-priority; `header_wildcard` if wildcard match used; `not_compressed` otherwise).
- `choose_first=true` (silent-ignored MVP) would override q-value priority with first-acceptable-in-declared-order — divergence-window.
- §11 pin §9.P4 confirms Envoy's exact selection algorithm; pin §9.P7 confirms `header_not_valid` trigger conditions on malformed Accept-Encoding.

---

## 3. Iteration protocol consequences

Phase 14 is the FIRST §9 row to participate primarily on the **encode side** (the only prior §9 row touching the encode side is phase 10 header_mutation, which mutates response headers via `EncodeHeaders`; phase 14 mutates both response headers AND response body). Iteration-protocol consequences:

**Encode-chain shape (current as of `internal/filter/hcm/connection.go:467-475`):**
```go
if rf.ActionRan() && status > 0 && actionErr == nil {
    merged := resp.Headers.ToHTTPHeader()
    if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil { ... }
    resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
    if len(resp.Body) > 0 {
        if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil { ... }
    }
}
```

The chain invokes `RunEncodeHeaders` once with the full header set + an `endStream` reflecting whether body is present, then (if body is present) invokes `RunEncodeData` once with the full body + `endStream=true`. There is no streaming; there is no chunked output. Phase 14's filter must operate within this constraint.

**Filter-callback API needed for body mutation:** the filter's `EncodeData(data []byte, endStream bool) FilterDataStatus` must, on the compress path, REPLACE `data` with the compressed bytes such that the framework's downstream wire-write picks up the new bytes. The current `chain.go:336` invocation is `status := f.EncodeData(data, endStream)` — `data` is passed by value (slice header). The slice header points at `resp.Body`'s underlying byte array; mutating elements in-place would NOT extend the length, and the filter's compressed bytes are typically a different length than the input.

**Resolution path** (SPEC author resolves at framework-survey step):
- **Path A:** Add an `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive that mutates the framework-side `resp.Body` directly. Filter calls `cb.OverwriteBody(compressedBytes)` from within `EncodeData`. Adds ~10-15 LoC framework delta in `internal/filter/hcm/connection.go` + `internal/filter/http/callbacks.go`. ONE new ADR (could be folded into ADR-0131 or stand alone).
- **Path B:** Pass `*[]byte` (pointer to slice) into `EncodeData` instead of `[]byte`. Breaks the current callback signature (`StreamEncoderFilter.EncodeData`). Higher LoC delta. Bigger ABI break.
- **Path C:** Have the filter signal "replace body" via a side-channel and the framework picks up the new bytes after the chain returns. Less invasive but requires a per-stream encoded-body-staging field on the filter framework state.

**Brainstorm position:** Path A is the natural pick — symmetric with phase-13 ADR-0128's decode-side framework deltas (which also added ~34 LoC HCM primitives), small, surgical. The SPEC author confirms at framework-survey step. IF the existing `EncoderFilterCallbacks` already exposes a body-mutation primitive (e.g., the existing `cb.AddData` / `cb.RemoveData` from envoyhttp library precedent), Path A reduces to "use existing primitive; ZERO framework deltas." §3 hypothesis: ZERO framework deltas needed; Path A only fires if framework survey finds no existing primitive.

**Encode-chain ordering:** phase 14 compressor SHOULD run AFTER cors's encode-side header mutations (cors injects 3 CORS headers on the actual-request encode path) so that the Vary header injection sees the final header set. The exact filter declaration order in fixture configs is a SPEC + fixture-author concern.

**Decode-chain ordering:** compressor's decode-side surface (Accept-Encoding strip + Accept-Encoding parse) runs BEFORE other request-side transformations (e.g., header_mutation request_mutations). Strip-first-then-mutate ordering matches Envoy's documented behavior.

---

## 4. Framework deltas — TBD pending §3 framework survey

Per §3 brainstorm hypothesis: **ZERO framework deltas** are anticipated if the existing `EncoderFilterCallbacks` already exposes a body-mutation primitive. **ONE framework delta** (Path A: `OverwriteBody(b []byte)` primitive at `EncoderFilterCallbacks` + matching HCM-side framework support) is anticipated as a worst-case fallback. The SPEC author resolves at the framework-survey step; if Path A fires, ADR-0131 grows a Decision (vi) clause covering the new primitive + cross-references to phase-13 ADR-0128 as the prior framework-delta precedent.

Phase 13 retired the "no framework deltas in §9 family-rows" invariant per its SPEC §4 amendment. Phase 14 inherits the precedent: framework deltas are permitted in §9 family-rows when load-bearing for the filter's algorithmic core. The compressor's body-mutation primitive (if needed) is exactly such a load-bearing delta.

**No framework deltas beyond OverwriteBody (if needed):** No new HCM hooks, no new chain-iteration disposition, no new HTTPRegistry method, no new PerRouteConfig accessor.

---

## 5. Stats — see §2.7

Per §2.7 (Decision 7 → ADR-0132): 11 new counters at `http.<HCM stat_prefix>.<filter stat_prefix>.compressor.gzip.<counter>` (hypothesis; §11 pin §9.P5 confirms). New Prometheus tag-extractor Rule SN10 extracting the filter-stat-prefix + codec-library tags. Per-route stats SHARED with listener-level. ADR-0132 codifies.

---

## 6. Differential fixture (`0016-http-compressor`)

### 6.1 Topology

Two listeners + two clusters (matches phase 11/12/13 fixture topology):

- **Listener `l_test_a`** (TCP plaintext on port `<envoy-go-test-port>` for envoy-go side; matching port on Envoy side per the `0015` template). Hosts an HCM with one filter-chain; the chain has filters: cors → compressor → router. Compressor's listener-level config is `Compressor{compressor_library: gzip{compression_level: BEST_COMPRESSION}, response_direction_config: { common_config: { enabled: { default_value: 100% }, min_content_length: 26, content_type: ["text/html", "application/json"] }, disable_on_etag_header: true, remove_accept_encoding_header: false }}`. Routes:
  - Route `/echo`: `direct_response` with body `<a 1024-byte deterministic ASCII payload>` and `content-type: text/html`. Default-route; receives compressed response.
  - Route `/echo-binary`: `direct_response` with body `<a 1024-byte payload>` and `content-type: image/png`. Receives non-compressed (content-type mismatch).
  - Route `/echo-small`: `direct_response` with body `<a 10-byte payload>` and `content-type: text/html`. Receives non-compressed (below `min_content_length`).
  - Route `/echo-etag`: `direct_response` with body `<1024-byte payload>`, `content-type: text/html`, `etag: "abc123"`. Receives non-compressed (`disable_on_etag_header=true`).
  - Route `/echo-disabled`: per-route TPFC `CompressorPerRoute{disabled: true}`. Receives non-compressed (route opted out).
  - Route `/echo-override`: per-route TPFC `CompressorPerRoute{overrides: {response_direction_config: {common_config: {enabled: {default_value: 100%}, min_content_length: 5}}}}`. Body `<10-byte payload>`, `content-type: text/html`. Receives compressed (per-route lower min_content_length).

- **Listener `l_test_b`** + cluster `c_backend_b`: standard backend cluster pair from `0015` template; not material to phase 14.

### 6.2 6 scenarios

Per 6 routes above, the differential fixture exercises 6 scenarios:

1. **Allow-compress (default route):** request `GET /echo` with `Accept-Encoding: gzip`. Expected response: status 200, `content-encoding: gzip`, `vary: Accept-Encoding`, `content-length: <gzipped-length>`, body decompresses to original 1024-byte payload.
2. **Skip-content-type-mismatch:** request `GET /echo-binary` with `Accept-Encoding: gzip`. Expected: status 200, no `content-encoding`, no `vary`, `content-length: 1024`, body identity 1024 bytes.
3. **Skip-content-length-too-small:** request `GET /echo-small` with `Accept-Encoding: gzip`. Expected: status 200, no `content-encoding`, no `vary`, `content-length: 10`, body identity.
4. **Skip-on-etag:** request `GET /echo-etag` with `Accept-Encoding: gzip`. Expected: status 200, no `content-encoding`, `etag: "abc123"`, body identity 1024 bytes.
5. **Per-route disabled:** request `GET /echo-disabled` with `Accept-Encoding: gzip`. Expected: status 200, no `content-encoding`, body identity. (Wholesale opt-out via `CompressorPerRoute.disabled=true`.)
6. **Per-route override:** request `GET /echo-override` with `Accept-Encoding: gzip`. Expected: status 200, `content-encoding: gzip`, decompressed body matches the 10-byte payload (the per-route lowered `min_content_length` to 5).

Skip-no-accept-header is exercised as a unit test (no fixture scenario; mirrors phase-13 buffer where header-only requests were unit-only).

### 6.3 Asserted equivalence

**Per-scenario assertions** (mirrors phase 11/12/13 scenario-by-scenario equivalence; see SPEC §3 acceptance review at SPEC drafting):

- **Status code:** byte-exact.
- **Headers:** lowercase wire-form byte-exact for ALL headers EXCEPT:
  - `content-length`: value MAY differ between envoy-go (compressed length) and Envoy (compressed length OR absent if Envoy uses chunked). §11 pin §9.P9 governs.
  - `transfer-encoding`: presence MAY differ (envoy-go: absent; Envoy: maybe `chunked`). §11 pin §9.P9 governs.
  - These two axes are EXCLUDED from byte-exact equivalence in scenarios 1 + 6 (the compressed-response scenarios). Scenarios 2-5 (uncompressed) MUST byte-exact match on both axes.
- **Body:**
  - On uncompressed scenarios (2, 3, 4, 5): byte-exact match.
  - On compressed scenarios (1, 6): **decompressed-byte-exact** match. The fixture driver decompresses both responses (envoy-go's gzip output + Envoy's gzip output) via Go's `compress/gzip.NewReader` and asserts byte-exact on the decompressed plaintexts. The compressed bytes themselves are NOT asserted byte-exact (structurally non-equivalent due to gzip-format multi-encoding spec; ADR-0133 codifies).
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the 11 phase-14 counters per scenario, plus existing HCM `downstream_rq_*` counters.
- **Per-route fixture-config disposition:** scenarios 5 + 6 exercise BOTH per-route shapes (`disabled` + `overrides`).

### 6.4 Driver shape

`inputs/driver.go` mirrors the `0015` driver shape:
- 6 scenarios, each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Decompression helper `decompressGzip(body []byte) ([]byte, error)` using `compress/gzip.NewReader`.
- Per-scenario assertion helper that distinguishes byte-exact-vs-decompressed-byte-exact based on response Content-Encoding.
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.

---

## 7. Anticipated ADRs (ADR-0129 through ADR-0133)

5 anticipated ADRs (consistent with phase 12's 5-ADR roster; one more than phase 13's 4-ADR roster). Phase 14 next-free is ADR-0129.

- **ADR-0129: `compressor` package shape + boot registration + 4-file split + zero-stats-path-for-non-compress + filterStats struct.** Mirrors phase-13 ADR-0125 + phase-12 ADR-0120 + phase-11 ADR-0114 + phase-10 ADR-0108 layout ADRs. Documents the package directory + extension-registry registration position + the StreamEncoderFilter-primarily nature (decode-side surface is minimal: `Accept-Encoding` strip + parse; encode-side is the algorithmic core).

- **ADR-0130: `compiledConfig` + full field envelope (8-consumed/9-ignored decomposition) + codec-library Any-unmarshal-and-dispatch + parse-rejection of unknown TypeURL + Gzip compression-level mapping table.** Documents the 8 consumed fields + 9 silent-ignored fields + 1 parse-rejected field family (unknown codec TypeURL). Includes the Gzip compression-level mapping table (Envoy enum → Go gzip int) + the `compression_strategy` HUFFMAN_ONLY-only honoring + the `memory_level`/`window_bits`/`chunk_size` silent-ignore rationale (Go `compress/gzip` doesn't expose libz-equivalent knobs). Includes parse-time validation of `compressor_library` REQUIRED + envoy-go-only error wording for unknown TypeURL.

- **ADR-0131: Body algorithm Path B (buffer-then-compress) + wire-shape divergence + maybe-OverwriteBody framework primitive + forward-pointer.** Documents Path B as the only feasible MVP under the encode-chain framework constraint at `connection.go:467-475` (one-shot full-body RunEncodeData + writeH1Reply unconditional fixed-CL emission). Records the wire-shape divergence (envoy-go: fixed CL identity; Envoy: chunked) + the explicit forward-pointer to a future encode-side streaming framework phase. Includes the `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework-delta IF the framework survey at SPEC time finds no existing body-mutation primitive (Decision (vi) clause; Decision (vi)-empty if existing primitive is sufficient). Cross-references phase-13 ADR-0128 as the prior decode-side framework-delta precedent + phase-13 ADR-0127 v2 as the prior algorithmic-divergence-with-forward-pointer precedent.

- **ADR-0132: 11-counter stat surface + new SN10 Prometheus tag-extractor + per-route SHARED stats discipline.** Documents the 11 counters + their hypothesized scope (`http.<HCM>.<filter>.compressor.gzip.<counter>`) + SN10 flattening rule (extracting `filter_stat_prefix` + `codec` tags). Cross-references phase-11 ADR-0118 SN9 as the precedent. SHARED-stats per-route discipline (NOT independent) cross-references phase-12 ADR-0124 + phase-13 ADR-0125.

- **ADR-0133: Differential-fixture decompress-and-compare body-assertion discipline.** Codifies the "compressed-bytes are NOT byte-exact; decompressed-bytes ARE byte-exact" pattern for filters whose output is a non-deterministic compression (or other lossy-but-reversible transform) of the input. Documents the fixture driver's decompression helper + the per-scenario assertion-mode-selection (byte-exact vs. decompressed-byte-exact based on `Content-Encoding` header). First filter exercising this discipline; codifies the pattern for future codec/transform filters (e.g., the future decompressor; future bandwidth-limit if it transforms body bytes).

**Plus an ADR-0125 amendment paragraph** (NOT a new ADR): noting phase 14 compressor as the SECOND row to use the disabled-OR-override 5th canonical per-route discipline + the WHOLESALE-not-merge semantic for `overrides.response_direction_config`. Authored at phase 14 SPEC drafting time per the ADR-0125 in-place-update precedent (mirrors phase-13 ADR-0127 v2 in-place update at Task 12).

---

## 8. Deferral list

Per phase 11/12/13 inline-deferral discipline (no omnibus ADR), the deferrals are 6 family-coupled items:

### 8.1 brotli + zstd codec extensions

`compressor_library` TypeURLs `envoy.extensions.compression.brotli.compressor.v3.Brotli` + `envoy.extensions.compression.zstd.compressor.v3.Zstd` are PARSE-rejected as unknown. Future codec phases (15.x or later) AMEND ADR-0130 to expand the recognized TypeURL set + add the corresponding codec-library dispatch + introduce new dependency closures (andybalholm/brotli + klauspost/compress/zstd or similar). The codec-library dispatch shape from ADR-0130 §Consequences is designed to admit additions without filter-package surgery.

### 8.2 Request-side compression (`request_direction_config`)

`Compressor.request_direction_config` is silent-ignored at runtime (always-disabled regardless of `common_config.enabled`). Couples to encode-side streaming framework primitives (since request-body modification interacts with phase-13 ADR-0128 framework deltas + the existing decode-chain `RunDecodeData` machinery). A future request-side compression phase activates this path; the natural phase is the future `envoy.filters.http.decompressor` (request-body decompression — separate filter type) which sits in the symmetric position to phase-14 compressor for request-side body modification. Operator divergence-window: configs that set `request_direction_config.common_config.enabled=true` against reference Envoy active-compress request bodies upstream; envoy-go does NOT.

### 8.3 The future `envoy.filters.http.decompressor` filter

A separate filter type (`envoy.filters.http.decompressor` proto message, separate TypeURL); deferred to a future §9 family-row. Decompresses request bodies (typically when downstream client sent `Content-Encoding: gzip`). NOT in scope for phase 14. The decompressor's framework-shape couples to phase-13 ADR-0128's decode-side framework deltas + the existing body-buffering machinery.

### 8.4 Chunked-encoded response wire shape

envoy-go's `writeH1Reply` does not support `Transfer-Encoding: chunked` output. Phase 14 emits fixed `Content-Length` on the compressed-response path; reference Envoy emits chunked (streaming-compressed). Couples to a future encode-side streaming framework phase that lands `writeH1Reply` chunked-mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation in HCM. ADR-0131 forward-points.

### 8.5 Gzip codec sub-fields not expressible in Go's `compress/gzip`

`Gzip.memory_level`, `Gzip.window_bits`, `Gzip.chunk_size` are silent-ignored. Go's `compress/gzip` (and its `flate` underlying) does NOT expose libz-equivalent knobs at these granularities. Re-activation requires either (a) switching to a third-party Go gzip library that exposes libz-knobs (e.g., klauspost/compress/gzip) — out-of-scope for MVP, or (b) using cgo + libz directly — explicitly out-of-scope per envoy-go's pure-Go discipline. Operator divergence-window: configs setting these fields against reference Envoy see the libz-tuned compression behavior; envoy-go uses Go's default-tuned behavior.

### 8.6 `Compressor.choose_first` first-acceptable selection mode

`choose_first=true` in reference Envoy overrides q-value priority with first-acceptable-in-declared-order. envoy-go MVP always-uses-q-value-priority (silent-ignore the field). Re-activation is a small future enhancement (the q-value parser already produces the in-declared-order list; toggle to first-acceptable is ~5 LoC). Couples to no other family. Operator divergence-window: configs setting `choose_first=true` AND multiple compressible codings in `Accept-Encoding` see first-coding-wins on Envoy; q-value-wins on envoy-go.

### 8.7 Runtime gates: `Compressor.runtime_enabled` + `response_direction_config.common_config.enabled`

Both `RuntimeFeatureFlag` fields silent-ignored at runtime; envoy-go is always-100%-active when the filter is configured. Mirrors phase-12 csrf `filter_enabled` posture (ADR-0121). `response_direction_config.common_config.enabled` is REQUIRED at parse time per Envoy PGV (§11 pin §9.P3 confirms or amends); fixture configs MUST set explicitly to 100%/HUNDRED on Envoy side. Couples to Runtime + hot restart family. Re-activation lands when the Runtime family phase brings RTDS / Runtime-layer support.

### 8.8 Deprecated top-level mirrors

`Compressor.{content_length, content_type, disable_on_etag_header, remove_accept_encoding_header}` (the deprecated top-level mirrors of the `response_direction_config` fields) are silent-ignored at parse time. Operators setting both legacy + new will see the new values honored and legacy values silent-discarded; documented as operator-footgun in BEHAVIOR_CONTRACT phase-14 forward-pointer notes. Couples to upstream Envoy's eventual removal of the deprecated fields (a future Envoy version may remove the proto fields entirely; envoy-go follows).

---

## 9. Empirical pins for SPEC §11

The SPEC author (lifecycle-state 1 → 2) executes these pins IN-SESSION against reference Envoy v1.37.2 per ADR-0004. Each pin either RATIFIES the brainstorm hypothesis (→ no SPEC §11 amendment) or AMENDS it (→ SPEC §11 amendment-block + possibly a §12 brainstorm-amendment cycle if the empirical re-frame is too large for the §11 amendment-block channel — phase 13 precedent).

**P1 — Default `content_type` list when `response_direction_config.common_config.content_type` is unset.** Hypothesis (per Envoy docs): `application/javascript`, `application/json`, `application/xhtml+xml`, `image/svg+xml`, `text/css`, `text/html`, `text/plain`, `text/xml`. Confirm: exact list, ordering, case-sensitivity, subtype-parameter handling (does `application/json; charset=utf-8` match `application/json` in the list?).

**P2 — Default `uncompressible_response_codes` value.** Hypothesis: empty `[]` per Envoy v1.37.2 (the historical-default `[206]` was removed circa v1.32; current default is empty and operators must opt-in). Confirm exact default + which response codes Envoy checks (primary status only).

**P3 — `response_direction_config.common_config.enabled` PGV requirement.** CRITICAL pin. Confirm parse-time PGV behavior: is the `enabled` `RuntimeFeatureFlag` REQUIRED at parse-time (mirrors phase-12 csrf P11)? What happens with unset / `default_value: 0/HUNDRED` / `default_value: 100/HUNDRED`? envoy-go silent-ignores at runtime → fixture must explicitly set `enabled.default_value: 100/HUNDRED` on Envoy side for byte-equivalent equivalence.

**P4 — `choose_first` selection algorithm under non-trivial Accept-Encoding.** When `Accept-Encoding: gzip;q=0.5, br;q=1.0` AND `choose_first=false` (default) AND brotli is not configured, does Envoy fall through to gzip (q=0.5)? When `choose_first=true`, does Envoy pick the first-listed acceptable? Confirm with multi-coding test inputs.

**P5 — Stat namespace + Prometheus tag pattern.** CRITICAL pin. Specifically: is the codec library name embedded in the stat path? Hypothesis: `http.<HCM scope>.compressor.gzip.<filter_prefix>.compressed`. Or is it: `http.<HCM scope>.<filter_prefix>.compressed` (no codec embedding)? Determines the new SN10 tag-extractor pattern + ADR-0132 codification.

**P6 — `content_type` matching algorithm.** Prefix-match (matches `text/html; charset=utf-8` against `text/html`)? Exact-match? Case-sensitive? Subtype parameter handling (e.g., `boundary` parameter on multipart)?

**P7 — `disable_on_etag_header` strong-vs-weak ETag handling.** Strong ETag (`"abc"`) clearly disables compression. Weak ETag (`W/"abc"`) — does Envoy treat as ETag-present and disable? Or only-strong-ETag-disables?

**P8 — `remove_accept_encoding_header` runtime behavior.** Confirm Envoy strips `Accept-Encoding` from upstream request before forwarding when `true`. Confirm forwarding leaves `Accept-Encoding` unchanged when `false`.

**P9 — Wire shape on small responses.** CRITICAL pin. Send a 500-byte text/html response via `direct_response`. Does Envoy emit `Content-Length: <compressed-length>` + identity transfer (matching envoy-go's Path B), or `Transfer-Encoding: chunked` + no `Content-Length`? Run with multiple body sizes (10 bytes, 100 bytes, 1024 bytes, 10240 bytes) to understand the threshold (if any). Outcome determines the fixture's `Content-Length`/`Transfer-Encoding` axis exclusions in §6.3.

**P10 — Vary header injection semantics.** When compressor compresses, does Envoy add `Vary: Accept-Encoding`? What if `Vary` is already present (e.g., `Vary: Origin`)? Envoy SHOULD append, not replace. Confirm exact behavior on `Vary: *` (wildcard).

**P11 — Already-compressed response handling.** Upstream sets `Content-Encoding: gzip` → Envoy skip-recompress? Upstream sets `Content-Encoding: identity` → Envoy compress (replacing identity with gzip)? Upstream sets `Content-Encoding: deflate` → Envoy skip (don't re-encode)?

**P12 — `Cache-Control: no-transform` semantics.** Response has `Cache-Control: no-transform` → Envoy MUST skip compression per RFC 7234 §5.2.2.4. Confirm Envoy v1.37.2 implementation honors. Test with `Cache-Control: no-transform`, `Cache-Control: max-age=3600, no-transform`, `Cache-Control: private, no-transform`.

**P13 — `CompressorPerRoute` semantics.** Three sub-pins:
- (a) `disabled: true` is wholly inactive (compression skipped, no counter increments, no header mutations on this route). Mirrors phase-13 buffer P4.
- (b) `CompressorOverrides.response_direction_config` WHOLESALE-overrides the listener-level (NOT a merge). E.g., listener-level `min_content_length: 1000` + per-route `min_content_length: 5` → per-route `5` is used; per-route `content_type` empty would NOT inherit listener-level `content_type` list (override is wholesale).
- (c) Per-route stats SHARED with listener-level (NOT independent stat namespace).

**P14 — Gzip wire compatibility.** Four sub-pins:
- (a) Gzip magic bytes `\x1f\x8b\x08` match between Go `compress/gzip` and Envoy libz.
- (b) Decompresses to byte-identical output (this is by gzip-format spec; both libraries decode identically).
- (c) Header fields (FLG, MTIME, XFL, OS) — Go default vs. Envoy default. MTIME may differ (Go zero-MTIME vs. Envoy zero-MTIME hopefully matching). XFL (extra-flags) and OS (3 = UNIX vs. 255 = unknown) may differ.
- (d) Footer (CRC32, ISIZE) — should match (both libraries compute the same CRC32 over the same input).

**P15 — `Content-Length` decision points + `Vary: Accept-Encoding` invariant on non-compressed paths.** When does Envoy emit/strip `Content-Length`? On non-compressed paths (skip predicates), does Envoy still inject `Vary: Accept-Encoding`? Hypothesis: Envoy injects `Vary` only on compressed paths; envoy-go's MVP follows.

---

## 10. ROADMAP delta

### 10.1 New row added by this brainstorm

A single ROW is added to `ROADMAP.md` `## MVP Trunk (phases 00–08)` table — wait, that's the wrong table; the §9 family-rows go into the SAME table per ADR-0106 (flat top-level rows at the end of the trunk table or in the family expansion). Looking at the existing ROADMAP layout: rows 09 + 10 + 11 + 12 + 13 are appended after row 08.2 in the MVP trunk table. Phase 14 follows the same pattern: row `14` appended after row `13`. (The MVP trunk table is misnamed per phase 09 forward-onward — it now contains all rows including §9 family-rows. ADR-0106 confirms the flat-top-level discipline; the table heading is structural, not a content gate.)

The new row:

| id | title | depends-on | status | sub-phases | summary |
|---|---|---|---|---|---|
| 14 | http-filter-compressor | 13 | planned |  | New `internal/filter/http/compressor/` package implementing `envoy.filters.http.compressor` (Envoy v1.37.2 canonical response-side compression filter, gzip-only MVP) under the 07.1 framework. SEVENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13). MVP envelope: 8 fields consumed (`compressor_library` gzip-only + `response_direction_config.{common_config.{min_content_length, content_type}, disable_on_etag_header, remove_accept_encoding_header, uncompressible_response_codes}` + `Gzip.{compression_level, compression_strategy}`); 9 fields silent-ignored (4 deprecated top-level mirrors + 2 runtime gates + `request_direction_config` + `choose_first` + 3 Gzip sub-knobs not expressible in Go gzip); `compressor_library` non-Gzip TypeURLs PARSE-rejected per ADR-0130. Body algorithm Path B (buffer-then-compress; framework as-is) per ADR-0131; wire-shape divergence (envoy-go: fixed `Content-Length`+identity; Envoy: streaming chunked) documented with forward-pointer to future encode-side streaming framework phase. Per-route TPFC `disabled`-OR-`overrides.response_direction_config` shape (SECOND row using ADR-0125 5th canonical disabled-OR-override; WHOLESALE-not-merge override semantic). Stat surface 29→40 names (11 new counters; new Prometheus tag-extractor Rule SN10 per ADR-0132). Differential fixture `0016-http-compressor` (6 scenarios: allow-compress, skip-content-type, skip-content-length-too-small, skip-on-etag, per-route-disabled, per-route-override). FIRST §9 row using decompress-and-compare body-assertion discipline per ADR-0133 (compressed bytes structurally non-byte-exact between Go `compress/gzip` and Envoy libz; decompressed bytes byte-equivalent). 18th fuzzer `FuzzCompressorConfigParse`. Anticipated 5 ADRs (ADR-0129 through ADR-0133) + ADR-0125 amendment paragraph. Per ADR-0106, §9 family-rows are flat top-level rows; phase 14 lands as row `14`. ADR-0045 surface-split release valve stays available if PLAN finds > ~1500 LoC / > ~25 tasks; SPEC's position is single-row at ~1100-1400 LoC / ~12-16 tasks.

### 10.2 §9 family heading at ROADMAP line 56 stays unchanged

Per ADR-0106(c). The line `### HTTP filters family` and the family-children enumeration at ROADMAP line 62 are unchanged across this brainstorm + the eventual phase-done landing.

### 10.3 No-sibling-stub discipline (per ADR-0106(b))

This brainstorm authors NO sibling stubs in ROADMAP for the 11 not-yet-brainstormed §9 family-children (`global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`) plus the future `envoy.filters.http.decompressor` companion. Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 11. Test scaffolding

### 11.1 Unit tests (`compressor_test.go`)

Test groups (mirrors phase-13 buffer's 6 test groups):

1. **Config parse + buildCompiledConfig** — `compressor_library` Any-unmarshal (gzip TypeURL), unknown-TypeURL parse-rejection, `response_direction_config.*` projection, Gzip compression-level mapping, deprecated top-level mirrors silent-ignore, missing `compressor_library` parse-rejection.
2. **buildCompiledPerRoute** — `disabled: true` path, `overrides.response_direction_config` path, oneof-empty parse-rejection, override WHOLESALE semantics (fields not set in override do NOT inherit listener-level).
3. **EncodeHeaders skip predicates** — exhaustive matrix of skip predicates: no-Accept-Encoding, content-type-mismatch, content-length-too-small, ETag, Cache-Control: no-transform, already-compressed (Content-Encoding present), uncompressible-response-code.
4. **EncodeData compression path** — gzip-encode small/medium/large body, level mapping (BEST_SPEED vs. BEST_COMPRESSION produce different compressed sizes), HUFFMAN_ONLY strategy, content-length-too-small late-stage gate (when CL was unknown at headers-time but reveals as < min after EncodeData accumulation).
5. **Vary + Content-Encoding header injection** — `Vary: Accept-Encoding` injection on empty / existing-Vary / wildcard-Vary; `Content-Encoding: gzip` set; `Content-Length` strip.
6. **Per-route resolver wiring + stats SHARED-with-listener** — most-specific resolution across Route > VirtualHost > RouteConfiguration tiers; counter-increment on listener-level scope when per-route is active; `disabled` short-circuit (no counters incremented on disabled routes).

Plus an `Accept-Encoding parser` test group covering RFC 7231 §5.3.4 q-value parsing edge cases (q=0 blocks; default q=1.0; wildcard handling).

### 11.2 Fuzzer (`fuzz_test.go`)

`FuzzCompressorConfigParse` — fuzzes the YAML→proto→`buildCompiledConfig` pipeline. Inputs are random bytes interpreted as YAML; errors-on-invalid-YAML are expected; the fuzzer asserts no panic + no nil-deref on the compilation path. 18th fuzzer in the repo (after `FuzzBufferConfigParse` from phase 13).

### 11.3 No new conformance suite

No `h2spec`-equivalent for compressor-specific RFC-conformance. Compressor's wire shape is governed by RFC 7231 (Vary semantics) + RFC 7234 (no-transform) + RFC 1952 (gzip format); envoy-go follows these but does not run a dedicated conformance suite.

### 11.4 Race-test discipline

`go test -race` on the `internal/filter/http/compressor/` package + the new framework-deltas (if Path A `OverwriteBody` lands per §3) + the existing HCM packages. Race-test surface unchanged from phase 13's 36-package green baseline.

---

## End of phase 14 brainstorm

Authored 2026-05-10 against master tip `fd976db`. Lifecycle-state-1 exit. Next session: SPEC drafting (skill `superpowers:writing-plans` routed through SPEC-authoring step per ADR-0005). SPEC author resolves §9 empirical pins IN-SESSION against reference Envoy v1.37.2.
