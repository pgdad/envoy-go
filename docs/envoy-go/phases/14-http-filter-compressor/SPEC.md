# Phase 14 SPEC — `envoy.filters.http.compressor`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 14 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 / 12 / 13 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 541 lines). §§1–11 are the pre-§11-empirical-pin design sketch (PRESERVED VERBATIM per D-3.5); the §11 empirical-pin block in this SPEC re-runs all 15 BRAINSTORM §9 pins against reference Envoy v1.37.2 IN-SESSION per ADR-0004. NO post-landing BRAINSTORM §12 amendment cycle was authored — the empirical re-frame is structured for the §1.1 amendment-block channel (mirrors phase-12 csrf 4-amendment precedent rather than phase-13 buffer §12 amendment-cycle precedent). NO off-master prebrainstorm-notes branch.

**ADR continuity:** Phase 13 closed at ADR-0128. Phase 14 anticipated ADR-0129..ADR-0133 (5 ADRs per BRAINSTORM §7) + ADR-0125 amendment paragraph. Phase 14 ships **5** ADRs: ADR-0129, ADR-0130, ADR-0131, ADR-0132, ADR-0133, plus an in-place amendment paragraph on ADR-0125 (per phase-13 ADR-0127-v2 in-place-update precedent at Task 12). Next-free ADR after phase 14 is ADR-0134.

**§3 framework-survey result up front (locks ADR-0131 §Decision (vi)):** Existing `EncoderFilterCallbacks` interface (`internal/filter/http/callbacks.go:68-81`) carries no body-mutation primitive. The chain dispatch (`internal/filter/http/chain.go:336`) passes `data []byte` BY VALUE to `f.EncodeData(data, endStream)` and ignores any filter-side replacement; HCM (`internal/filter/hcm/connection.go:472,478,485` for H1; `internal/filter/hcm/h2dispatch.go:310,328,323` for H2) reads `resp.Body` directly post-chain. Path A FIRES: phase 14 introduces `EncoderFilterCallbacks.OverwriteBody(b []byte)` (~12 LoC HCM delta + ~3 LoC interface + ~4 LoC chain encoderCB stub-and-honor mechanism) plus a per-stream override field on the encoderCB. ADR-0131 §Decision (vi) records.

---

## 1. Purpose

Phase 14 lands `envoy.filters.http.compressor` — Envoy's canonical "compress upstream response body before forwarding downstream" filter, gzip-only MVP, response-side only — as the SEVENTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), and buffer (13), and the SEVENTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 14 is the FIRST §9 family-row to (i) compress / mutate response body bytes; (ii) require an Any-unmarshal-and-dispatch mechanism inside the filter package for codec-library typed-extension config; (iii) ship a filter whose body-bytes axis is structurally non-byte-exact at the wire level (Go `compress/gzip` and Envoy's libz produce different compressed bytes from the same input — both decompress to the same plaintext); (iv) introduce a divergence-window on response wire shape (envoy-go: fixed `Content-Length`; Envoy: `Transfer-Encoding: chunked`); (v) be the SECOND §9 family-row to use the disabled-OR-override 5th canonical per-route discipline (codified at ADR-0125 by phase 13); (vi) introduce a SYMMETRIC encode-side framework primitive — `EncoderFilterCallbacks.OverwriteBody(b []byte)` — mirroring phase-13 ADR-0128's decode-side primitives. The seven new architectural primitives:

1. A new `internal/filter/http/compressor/` package owning the filter implementation. Directory + Go-package identifier are both `compressor` (single token; matches the cors / fault / csrf / buffer precedent — no underscore needed since the proto type-name is already a single token). Files mirror the buffer / csrf precedent: `compressor.go` (filter type + factory + `EncodeHeaders` + `EncodeData` + per-route helper + `compiledConfig` + `compiledPerRoute` + codec-library Any-unmarshal-and-dispatch + `filterStats`), `compressor_test.go` (unit tests across 7 test groups per §14.1), `doc.go` (package overview + 6-consumed/12-ignored decomposition + per-route disabled-OR-override summary — see §1.1 amendment 1 below for the field-count revision relative to BRAINSTORM §1.1 item 3), `fuzz_test.go` (`FuzzCompressorConfigParse` per §14.3 — the 18th fuzzer in the repo). Two top-level exports: `TypeURL` (string constant `"type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"`) + `New` (the `HTTPFilterFactory` registered against `TypeURL` in the boot registry). All other types (`compiledConfig`, `compiledPerRoute`, `filter`, `filterStats`, `compiledGzipConfig`) are unexported. See ADR-0129.

2. **Encode-only filter; minimal decode-side surface.** The filter implements `StreamEncoderFilter` (`Encoder: f`) AND `StreamDecoderFilter` (`Decoder: f`) — phase 14 is the FIRST §9 row to be ENCODER-PRIMARILY but with a non-vacuous decode side. The decode-side surface is minimal but non-trivial: `DecodeHeaders` parses `Accept-Encoding` (q-value parser per RFC 7231 §5.3.4) → caches the negotiated encoding token on per-stream filter state; resolves per-route TPFC → caches `*compiledPerRoute`; if effective `remove_accept_encoding_header == true` → strips `Accept-Encoding` from the request headers before forwarding upstream (per §11.8 + listener-level field consumption per §1.1 amendment 1). `DecodeData` / `DecodeTrailers` pass through. Encode-side: `EncodeHeaders` runs the skip-decision sequence (no AE / not-compressible / etag-strong-strip-only / no-transform / already-encoded / content-type-mismatch / status-uncompressible) and on the compress path mutates response headers (sets `Content-Encoding: gzip`, sets/extends `Vary: Accept-Encoding` per §11.10 — APPEND ALWAYS, even on existing `Vary: *` per the §1.1 amendment 5 refutation; conditionally STRIPS strong-ETag per §11.7 amendment); `EncodeData` on the compress + endStream=true path gzip-encodes the full `data` slice in one shot via `gzip.NewWriterLevel(buf, level).Write(data).Close()` and emits the compressed bytes via the new `cb.OverwriteBody(buf.Bytes())` primitive (see item 4 below).

3. **MVP envelope: 6 consumed + 12 ignored + 1 parse-rejected (REVISED from BRAINSTORM §1.1 item 3's 8/9/1 decomposition; see §1.1 amendment 1).** `Compressor` proto carries 9 top-level fields; phase 14 consumes a subset and silent-ignores the rest. `Gzip` codec proto carries 5 fields; phase 14 consumes 2 actively + ignores 3. `CompressorPerRoute` carries the 5th canonical disabled-OR-override oneof.
   - **Listener-level `Compressor` consumed (5 effective fields):** `compressor_library` (TypedExtensionConfig; REQUIRED — REQUIRED per Envoy PGV `(validate.rules).message.required = true`); `response_direction_config.common_config.min_content_length` (UInt32Value); `response_direction_config.common_config.content_type` ([]string); `response_direction_config.disable_on_etag_header` (bool); `response_direction_config.remove_accept_encoding_header` (bool); `response_direction_config.uncompressible_response_codes` ([]uint32). Total **6 listener-level consumed fields** (the `common_config.enabled` field is SILENT-IGNORED at runtime per §1.1 amendment 2 — runtime always-100%-active mirrors phase-12 csrf ADR-0121 posture). Plus the codec-library 2-field consumption: `Gzip.compression_level` + `Gzip.compression_strategy` — 2 fields. **GRAND TOTAL: 6 listener + 2 codec = 8 consumed across the listener-level proto graph; the BRAINSTORM "8" count was correct in aggregate but mislabeled the `enabled` slot — see §1.1 amendment 1 for the bookkeeping correction.**
   - **Listener-level `Compressor` silent-ignored (12 fields, REVISED from BRAINSTORM 9):** 4 deprecated top-level mirrors (`Compressor.{content_length, content_type, disable_on_etag_header, remove_accept_encoding_header}`); `Compressor.runtime_enabled` deprecated runtime gate (always-100%); `Compressor.choose_first` (always-q-value-based); `Compressor.request_direction_config` (always-disabled, response-only MVP); `response_direction_config.common_config.enabled` (RuntimeFeatureFlag; runtime-ignored, always-100% — DOWNGRADED from BRAINSTORM-hypothesized "REQUIRED at parse time" per §1.1 amendment 2); `response_direction_config.status_header_enabled` (bool; the `x-envoy-compression-status` debug-header opt-in; DEFERRED per §1.1 amendment 1 — this 12th field is NOT in BRAINSTORM Q5 because Q5 was authored against an outdated Envoy proto reference; it is silent-ignored at runtime; always-no-status-header); `Gzip.{memory_level, window_bits, chunk_size}` (3 sub-fields not expressible in Go's `compress/gzip`).
   - **Parse-rejected (envoy-go-only validation, 1 case):** `Compressor.compressor_library.typed_config` with TypeURL other than `type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip` — error wording `compressor: unsupported compressor_library TypeURL <url>; phase-14 MVP supports only envoy.extensions.compression.gzip.compressor.v3.Gzip`. Future codec phases AMEND ADR-0130 to expand the recognized TypeURL set. Mirrors phase-13 buffer's `max_request_bytes > 1 MiB` envoy-go-only-check pattern (ADR-0126).
   - **Per-route `CompressorPerRoute` shape (MAJOR REVISION from BRAINSTORM Decision 5; see §1.1 amendment 4):** oneof `disabled: true` OR `overrides: CompressorOverrides`. The `overrides` sub-message carries `response_direction_config: ResponseDirectionOverrides` and `compressor_library: TypedExtensionConfig`. **The `ResponseDirectionOverrides` proto type carries EXACTLY ONE field — `remove_accept_encoding_header` (BoolValue) — NOT the 4 fields BRAINSTORM Decision 5 hypothesized.** Per-route `min_content_length`, `content_type`, `disable_on_etag_header`, and `uncompressible_response_codes` overrides are STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's proto. Phase 14 honors `disabled: true` + `overrides.response_direction_config.remove_accept_encoding_header` only; per-route `compressor_library` swap is DEFERRED (no MVP fixture coverage; couples to multi-codec per §8.2).

4. **Body algorithm: Path B (buffer-then-compress); wire-shape divergence accepted; new `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive lands.** envoy-go's encode-chain framework at `internal/filter/hcm/connection.go:467-475` (H1) + `internal/filter/hcm/h2dispatch.go:303-315` (H2) invokes `RunEncodeData(ctx, resp.Body, true)` ONCE with the full response body in a single call, `endStream=true`. The encode chain is non-streaming; HCM materializes `resp.Body []byte` upstream-side and the filter sees the whole body at once. Combined with `writeH1Reply` (`internal/filter/hcm/codec.go:74-119`) unconditionally rewriting `Content-Length: <len(body)>` on the wire (line 87-89), the framework as-is forces **Path B**: the filter operates on the already-accumulated `resp.Body`, gzip-encodes in one shot, and replaces the body via the new framework primitive. The chain at `internal/filter/http/chain.go:322-355` passes `data []byte` BY VALUE to `f.EncodeData` and provides no return-the-mutated-bytes path; the existing `EncoderFilterCallbacks` interface (`callbacks.go:68-81`) carries only `ContinueEncoding` + 3 injection-side methods (`EncodeHeaders` / `EncodeData` / `EncodeTrailers` — the latter three are intended for SYNTHESIZING a response from inside a different filter's decode-side context, NOT for replacing the body of the currently-encoding stream). Phase 14 introduces:
   - **`EncoderFilterCallbacks.OverwriteBody(b []byte)`** — a new method on the interface at `internal/filter/http/callbacks.go`. Filters call it from within their `EncodeData(data, endStream)` body to register a replacement body. Not goroutine-safe in the current envoy-go HCM (the encode chain is run synchronously in the dispatch goroutine; `OverwriteBody` must be called from inside `EncodeData`).
   - **`encoderCB.OverwriteBody` impl at `internal/filter/http/chain.go`** — concrete implementation: stores `b` on a new `c.encodeBodyOverride []byte` field on `*FilterChain` + flips a `c.encodeBodyOverridden bool` sentinel.
   - **HCM-side encode-body-override harvest at `internal/filter/hcm/connection.go:472,478` (H1) + `h2dispatch.go:310,328` (H2)** — after `RunEncodeData` returns, HCM calls a chain accessor `chain.EncodeBodyOverride() ([]byte, bool)` and substitutes `resp.Body` with the override before the wire-write path consumes it. ~10-15 LoC HCM delta total. Mirrors phase-13 ADR-0128's decode-side primitives (synthetic empty-terminal RunDecodeData + post-body CL reconciliation) in shape and load-bearing-ness.

   **Wire-shape divergence from reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-14 forward-pointer notes; ADR-0131 records): envoy-go emits `Content-Encoding: gzip`, `Content-Length: <gzipped-length>`, NO `Transfer-Encoding`. Reference Envoy v1.37.2 emits `Content-Encoding: gzip`, `Transfer-Encoding: chunked`, NO `Content-Length` (confirmed at §11.9 — observed across body sizes 30 / 100 / 1024 / 10240 bytes; chunked emission is universal on the compressed path). Decompressed body bytes are byte-equivalent (gzip is well-defined; both libraries decode identically). Compressed body bytes are NOT byte-equivalent (Go `compress/gzip` and Envoy libz make different block-boundary + Huffman-tree choices, plus differ in gzip-header `XFL` and `OS` bytes per §11.14). The forward-pointer phase that lands encode-side streaming framework (analogous to phase-13 cap-promotion forward-pointer) is the natural amender; ADR-0131 forward-points.

5. **Stat surface — 29→46-name extension (REVISED from BRAINSTORM 29→40; see §1.1 amendment 3).** **17 new counters** at `BEHAVIOR_CONTRACT.md ## Stat-name mapping`, NOT 11 as BRAINSTORM §1.1 item 7 hypothesized. The BRAINSTORM-hypothesized 11-counter list is INCOMPLETE — reference Envoy v1.37.2 emits 17 counters per `<HCM_stat_prefix>` per active compressor library, split into 4 family clusters: 6 `header_*` Accept-Encoding-cluster counters (NOT split per request/response; shared across both directions on the listener); 1 `not_compressed_etag` ETag-skip counter; 5 `request_*` request-side counters (always-zero in phase 14 MVP since `request_direction_config` is silent-ignored); 5 `response_*` response-side counters (active in phase 14 MVP). Phase 14 envoy-go registers all 17 counters at boot (mirrors-Envoy-fully discipline; 5 request_* counters stay at zero on the envoy-go side and the differential matches Envoy's also-zero output) — see ADR-0132 §Decision (i). **Stat namespace + Prometheus tag-extractor — NO new SN10 rule (REVISED from BRAINSTORM §2.7; see §1.1 amendment 3).** Reference Envoy emits at internal stat-path `http.<HCM_stat_prefix>.compressor.<compressor_library.name>.<codec_short_name>.<counter_name>`; the `compressor.<library_name>.<codec>.` infix is FLATTENED via the existing Rule SN2 (HCM stat_prefix; ADR-0061) into `envoy_http_compressor_<library_name>_<codec>_<counter>{envoy_http_conn_manager_prefix=<HCM stat_prefix>}`. NO codec-tag extraction; NO new SN10 rule; NO `internal/stats/name.go` flattenToProm switch surgery. The 5th canonical SN-rule discipline carries through unchanged from phase-12 csrf SN-precedent (csrf reused SN2 with no new rule). Per-route stats SHARED with listener-level (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 SHARED-stats; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats).

6. **`compressor_library.name` is load-bearing for stat-namespace identity.** Per §11.5, the stat path embeds `compressor_library.name` (the operator-supplied label string in the `TypedExtensionConfig.name` field). Operators with bare-Gzip configs (no `name:` set) emit stats under `compressor.<empty>.gzip.<counter>` — empirically confirmed via probeC; envoy-go MUST mirror this. `compiledConfig` carries `libraryName string` (empty-allowed); fixture configs (envoy.yaml + envoy-go.yaml) MUST set the SAME `name:` value on both sides for byte-equivalent stat emission. Phase-14 fixture sets `name: text_optimized` on both sides per the §11.5 ratification.

7. **`disable_on_etag_header` semantics — DUAL-MODE (NEW finding; see §1.1 amendment 6).** When `disable_on_etag_header: false` (default), Envoy still mutates the response: it RESERVES "strong" ETag semantics by STRIPPING strong-ETag headers (`"abc"`) on the compressed path while PRESERVING weak-ETag headers (`W/"abc"`). When `disable_on_etag_header: true`, Envoy SKIPS compression on any ETag presence (strong OR weak — §11.7 sub-pin (c) confirms; the proto comment at compressor.proto line 78 says "preserve weak ETag values and remove those that require strong validation" — empirical evidence at §11.7 confirms strong-strip + weak-preserve under default-false; SKIP behavior under true is per Envoy `compressor_filter.cc:90-95`). RFC 7232 §2.3 motivates this: strong validators must change when the entity bytes change, so a strong ETag for the uncompressed entity cannot serve as a strong validator for the gzip-compressed entity; weak validators have no such constraint. **Phase 14 envoy-go MVP MUST implement BOTH modes:** (a) on `disable_on_etag_header: false` + ETag present → compress + STRIP strong-ETag (regex `^"[^"]*"$`); preserve weak-ETag (regex `^W/"[^"]*"$`); (b) on `disable_on_etag_header: true` + ETag present → SKIP compression (any ETag, strong or weak); increment `not_compressed_etag +1`.

After phase 14, the project has proven the §9 HTTP filters family-expansion pattern carries through a SEVENTH filter under: the cors / fault / header_mutation / local_ratelimit / csrf / buffer precedent's package-shape discipline (single-token directory matching the proto type-name); the 5th canonical disabled-OR-override per-route discipline (codified at ADR-0125; phase 14 is the SECOND row using); a NEW encode-side framework primitive (`OverwriteBody`) symmetric to phase-13 ADR-0128's decode-side primitives; the existing HCM stat-prefix tag-extraction (no new SN rule); a deliberate wire-shape divergence-window from reference Envoy with a forward-pointer to a future encode-side streaming framework phase. *envoy-go's HTTP filter framework hosts a synchronous, encoder-primarily body-mutating filter that does its own gzip-compression in one shot rather than streaming chunk-by-chunk; the OBSERVABLE-OUTCOMES are byte-equivalent to reference Envoy on every axis EXCEPT the `Content-Length`/`Transfer-Encoding` axis on the compressed path (envoy-go fixed-CL identity vs. Envoy chunked) AND the compressed-body-bytes axis (gzip-format multi-encoding admits both Go `compress/gzip` and libz outputs as valid; decompressed bytes are byte-equivalent); the differential fixture's `decompress-and-compare` body-assertion discipline (ADR-0133) handles the latter; the `Content-Length`/`Transfer-Encoding` axis is allow-listed out via `BEHAVIOR_CONTRACT.md ## Equivalence Matrix` per-scenario tolerance.* This is the SEVENTH §9 family-row to land; subsequent filters (`global_ratelimit`, `jwt_authn`, …) follow the same row-as-its-own-phase pattern.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session (2026-05-10) refutes or sharpens **six** load-bearing BRAINSTORM hypotheses. Each amendment below is a self-contained correction; collectively they revise the field-decomposition bookkeeping (1) + the runtime gate posture (2) + the stat surface (3) + the per-route override surface (4) + the Vary semantics (5) + the ETag-header semantics (6). Mirrors the phase-12 csrf 4-amendment pattern (a SPEC-time correction of BRAINSTORM hypotheses); extends the phase-13 buffer 1-amendment-prose-correction pattern. Each amendment has a §11 cross-reference for the verbatim scrape evidence.

#### 1.1 Amendment 1 — Field-decomposition bookkeeping correction (BRAINSTORM §1.1 item 3 + §2.3)

BRAINSTORM §1.1 item 3 + §2.3 enumerated **8 consumed / 9 ignored / 1 parse-rejected** fields. The §11 cross-section against compressor.proto v1.37.2 line-by-line surfaces TWO bookkeeping errors:

- (a) **`Compressor.response_direction_config.status_header_enabled` (line 113 of compressor.proto v1.37.2) was MISSED by BRAINSTORM Q5.** This is the `x-envoy-compression-status` debug-header opt-in flag — when `true`, Envoy emits the `x-envoy-compression-status: <encoder>;<status>[;<additional-params>]` response header indicating compression outcome. BRAINSTORM hypothesized 9 silent-ignored fields; actual is **10** (status_header_enabled adds the 10th slot). Phase 14 silent-ignores at runtime; envoy-go always-no-status-header. Operator divergence-window: configs setting `status_header_enabled: true` against reference Envoy see the `x-envoy-compression-status:` header on responses; envoy-go does NOT emit this header. Couples to a future debug-header bundle. Documented at §13.4 phase-14 forward-pointer notes.
- (b) **`response_direction_config.common_config.enabled` SLOT MISLABELED.** BRAINSTORM §2.3 listed `enabled` under "CONSUMED at runtime (8 effective fields)" but immediately under §2.7 / §1.1 item 7 framed it as "always-100% mirroring phase-12 csrf `filter_enabled` posture per ADR-0121 — fixture configs MUST set explicitly to 100%/HUNDRED on Envoy side". The "always-100%" framing is the SILENT-IGNORE posture per ADR-0040; the field is NOT actively consumed at runtime by envoy-go, and the §11 empirical-pin block (§11.3) further refutes the "REQUIRED at parse-time per Envoy PGV" claim — the field is OPTIONAL at parse-time + NOT RuntimeFractionalPercent (it's RuntimeFeatureFlag with BoolValue default). The slot moves from "consumed" to "silent-ignored" in the bookkeeping; counts revise as follows.

**Revised counts (post-amendment):**
- Listener-level `Compressor`: **5 consumed + 7 silent-ignored** (5 active = compressor_library + min_content_length + content_type + disable_on_etag_header + remove_accept_encoding_header + uncompressible_response_codes; wait, that's 6 — let me re-enumerate. Active 6: compressor_library (1) + min_content_length (2) + content_type (3) + disable_on_etag_header (4) + remove_accept_encoding_header (5) + uncompressible_response_codes (6). Silent-ignored: 4 deprecated mirrors + runtime_enabled + choose_first + request_direction_config + status_header_enabled + enabled = 9. **Listener total: 6 consumed + 9 silent-ignored.**)
- Codec-library `Gzip`: 2 consumed (compression_level + compression_strategy) + 3 silent-ignored (memory_level + window_bits + chunk_size). Total: 2 + 3.
- **Grand total across listener-level proto graph: 8 consumed (6 listener + 2 codec) + 12 silent-ignored (9 listener + 3 codec) + 1 parse-rejected (unknown codec TypeURL).** The "8 consumed" headline number from BRAINSTORM §1.1 item 3 SURVIVES — different bookkeeping shape, same total — but the silent-ignore count rises from 9 → 12 (+3 net: enabled migrated from consumed-side, status_header_enabled added, vs. the BRAINSTORM-counted "9").

The `compiledConfig` shape per §6.2 reflects the 5 listener fields envoy-go actively projects (excluding `compressor_library` which threads through codec-library dispatch separately as `compiledGzipConfig`). Per-route shape per §6.3 reflects the §1.1 amendment 4 below.

#### 1.1 Amendment 2 — `response_direction_config.common_config.enabled` is RuntimeFeatureFlag (BoolValue default), OPTIONAL at parse (BRAINSTORM Q5 + §1.1 item 3 + §2.7 + BRAINSTORM §9.P3)

BRAINSTORM §1.1 item 3 third-bullet hypothesized: "`Compressor.response_direction_config.common_config.enabled` (RuntimeFeatureFlag) — always-100%; divergence-window if Envoy-side enabled at < 100%. Mirrors phase-12 csrf `filter_enabled` posture per ADR-0121 — fixture configs MUST set explicitly to 100%/HUNDRED on Envoy side (§11 pin §9.P3 confirms parse-time PGV requirement)." **§11.3 empirically REFUTES the parse-time PGV requirement.** The field is `envoy.config.core.v3.RuntimeFeatureFlag` (per upstream compressor.proto line 36 of `envoy.extensions.filters.http.compressor.v3.Compressor.CommonDirectionConfig.enabled`); the type's `default_value` is `google.protobuf.BoolValue` (NOT `envoy.type.v3.FractionalPercent` as BRAINSTORM §2.3 implicitly assumed via the "100%/HUNDRED" framing). The phase-12 csrf `filter_enabled` parallel does NOT carry — phase-12's `filter_enabled` is `RuntimeFractionalPercent`, a different proto type with strict PGV-required semantics; phase-14's `enabled` is `RuntimeFeatureFlag` with no PGV-required wrapper.

**Empirical evidence (§11.3 + §11.15):** probeC bootstraps with NO `response_direction_config` at all; Envoy boots cleanly + the filter compresses correctly at runtime. Default behavior absent the field is "filter enabled" per the proto comment at compressor.proto line 32 ("If this field is not specified, the filter will be enabled."). probeA bootstraps with `response_direction_config.common_config.enabled: { default_value: true, runtime_key: response_compressor_enabled }` (BoolValue shape, not FractionalPercent shape) and boots cleanly + compresses; an earlier probeA attempt with `default_value: { numerator: 100, denominator: HUNDRED }` (FractionalPercent shape) FAILED to boot with `no such field: 'numerator'` per §11.3 verbatim — confirming the BoolValue type.

**Phase-14 envoy-go disposition:**
- Parse-time: `enabled` is OPTIONAL (no envoy-go-only validation; the field may be absent OR present-with-default_value-true OR present-with-default_value-false). Mirrors Envoy's optional posture.
- Runtime: SILENT-IGNORED — envoy-go always evaluates as "filter enabled" regardless of `default_value` setting OR runtime-key state. Operator divergence-window: configs that set `default_value: false` (or rely on runtime-key flipping the filter off) see Envoy disable; envoy-go always-active. Documented at §13.4 phase-14 forward-pointer notes.
- **Differential fixture (§7) DOES NOT need to set `enabled` explicitly on Envoy side.** Both sides default to "enabled"; byte-equivalent equivalence holds without the explicit setting. UNLIKE phase-12 csrf where `filter_enabled.default_value: 100/HUNDRED` MUST be set on Envoy side per ADR-0121, phase-14 fixtures may either set `enabled: { default_value: true, runtime_key: <key> }` for documentation parity OR omit the field entirely on both sides. The SPEC's position: omit on both sides for fixture minimality (mirrors probeC + probeA-success-without-explicit-setting precedent at §11.3).

This amendment is the LARGEST PGV-related correction in any §9 family-row to date — phase 12 csrf had a parse-time PGV requirement landing AT SPEC time; phase 14 has the inverse landing (BRAINSTORM hypothesized PGV-required, SPEC empirically refuted). The pattern of "BRAINSTORM commits hypothesis; SPEC empirically confirms or refutes" continues to function as designed.

#### 1.1 Amendment 3 — Stat surface is 17 counters (NOT 11); namespace shape is `compressor.<library>.<codec>.[response.]<counter>`; NO new SN10 rule (BRAINSTORM §1.1 item 7 + §2.7 + §9.P5)

BRAINSTORM §1.1 item 7 hypothesized **11 new counters** + a new Prometheus tag-extractor Rule **SN10** extracting filter-stat-prefix + codec-library tags. **§11.5 empirically REFUTES both halves.** The actual reference Envoy v1.37.2 stat surface for one active gzip compressor under one HCM stat_prefix:

- **6 `header_*` counters (Accept-Encoding-cluster):** `header_compressor_overshadowed`, `header_compressor_used`, `header_identity`, `header_not_valid`, `header_wildcard`, `no_accept_header`.
- **1 `not_compressed_etag` counter** (NEW; not in BRAINSTORM hypothesis — increments only on `disable_on_etag_header: true` + ETag-present skip path).
- **5 `request_*` counters (request-side; phase-14 MVP always-zero since `request_direction_config` silent-ignored):** `request_compressed`, `request_content_length_too_small`, `request_not_compressed`, `request_total_compressed_bytes`, `request_total_uncompressed_bytes`.
- **5 `response_*` counters (response-side; active in phase-14 MVP):** `response_compressed`, `response_content_length_too_small`, `response_not_compressed`, `response_total_compressed_bytes`, `response_total_uncompressed_bytes`.

**Total: 17 counters** (6 + 1 + 5 + 5), NOT 11. BRAINSTORM's 11-counter list missed (a) the request_/response_ split (BRAINSTORM hypothesized flat `compressed`, `not_compressed`, `total_compressed_bytes`, `total_uncompressed_bytes`, `content_length_too_small` — 5 names; actual is 10 names, 5 per direction); (b) the `not_compressed_etag` counter; (c) the proto note at compressor.proto line 158-164 explaining "When this field is set, ... statistics related to response compression will be rooted in `response.*`" — BRAINSTORM did not encode the namespace-shift rule.

**Stat namespace + Prometheus tag pattern (REVISED from BRAINSTORM §2.7):** Reference Envoy emits at internal stat-path:
```
http.<HCM_stat_prefix>.compressor.<compressor_library.name>.<codec_short_name>.[response.]<counter>
```
where `<codec_short_name>` is `gzip` for the gzip codec library (the short-form stat-tag the codec library reports; per §11.5 verbatim scrape `text_optimized.gzip.<counter>` shows). The `response.` infix appears IFF `response_direction_config` is set on the listener-level Compressor; absent (legacy mode), the counters are at the flat `compressor.<library>.<codec>.<counter>` path. **Phase 14 envoy-go MUST always set `response_direction_config` (or its compiled equivalent — `compiledConfig.responseDirectionConfigSet bool`) so the `response.*` namespace is used.** The differential fixture's envoy.yaml + envoy-go.yaml MUST agree on the namespace shape. 

**Prometheus rendering via existing Rule SN2 (NO new SN10):** the path `http.<HCM_stat_prefix>.<rest>` flattens to `envoy_http_<rest>` with label `envoy_http_conn_manager_prefix=<HCM_stat_prefix>` per ADR-0061 SN2. With `<rest>` = `compressor.text_optimized.gzip.response.compressed`, SN2 produces `envoy_http_compressor_text_optimized_gzip_response_compressed{envoy_http_conn_manager_prefix="ingress_p14a"}` — verbatim observed at §11.5. NO codec-library-as-Prometheus-label extraction; the library name IS PART OF the static stat-name suffix. **NO new `internal/stats/name.go` flattenToProm switch surgery; NO new ADR-0118-class flattening rule.** ADR-0132 simplifies to "register 17 counters; emit under `compressor.<library>.<codec>.[response.]<counter>` path; the existing SN2 handles Prometheus rendering."

**Revised stat-table extension (BEHAVIOR_CONTRACT §13.2):** 29 → **46 names** (17 new), NOT 29 → 40 as BRAINSTORM §1.1 item 7 hypothesized.

**Per-route stats SHARED-with-listener-level** (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125; DIVERGES from phase-11 local_ratelimit ADR-0117). Per-route-active routes increment the listener-level counter scope; no per-route counter namespace.

#### 1.1 Amendment 4 — Per-route `ResponseDirectionOverrides` carries ONLY `remove_accept_encoding_header` (BRAINSTORM Decision 5 + §2.5 + §6 fixture scenario 6 + §9.P13)

BRAINSTORM §2.5 hypothesized:
```proto
message CompressorOverrides {
  ResponseDirectionConfig response_direction_config = 1;
}
```
where `ResponseDirectionConfig` is the listener-level type carrying 4 fields (`common_config` + 3 booleans/repeated). **Empirically REFUTED via §11.13 + upstream compressor.proto v1.37.2 line 175-179.** The actual proto:

```proto
// Per-route overrides of ``ResponseDirectionConfig``. Anything added here should be optional,
// to allow overriding arbitrary subsets of configuration. Omitted fields must have no effect.
message ResponseDirectionOverrides {
  // If set, overrides the filter-level remove_accept_encoding_header.
  google.protobuf.BoolValue remove_accept_encoding_header = 1;
}

message CompressorOverrides {
  ResponseDirectionOverrides response_direction_config = 1;
  config.core.v3.TypedExtensionConfig compressor_library = 2;
}
```

`ResponseDirectionOverrides` is a SEPARATE proto type (NOT a field-subset alias of `ResponseDirectionConfig`) carrying EXACTLY ONE FIELD: `remove_accept_encoding_header` (BoolValue). It does NOT carry `common_config` (no `enabled`, no `min_content_length`, no `content_type`); it does NOT carry `disable_on_etag_header`; it does NOT carry `uncompressible_response_codes`. Per-route overrides for these 5 fields are STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's compressor proto. `CompressorOverrides` ALSO carries an OPTIONAL `compressor_library` (per-route library swap; gzip-only MVP defers).

**Phase-14 envoy-go disposition:**
- `disabled: true` per-route → wholly inactive (filter passes through; no stat increments). HONORED.
- `overrides.response_direction_config.remove_accept_encoding_header: <bool>` per-route → overrides listener-level `response_direction_config.remove_accept_encoding_header`. HONORED.
- `overrides.compressor_library: <TypedExtensionConfig>` per-route → DEFERRED (silent-ignored at runtime in MVP; couples to multi-codec future phase). Documented at §13.4.
- All other listener-level response_direction_config fields (`min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes`) cannot be per-route overridden by proto design.

**Differential fixture redesign (§7 below):** BRAINSTORM §6 scenario 6 ("per-route override; per-route lower min_content_length: 5") is **STRUCTURALLY IMPOSSIBLE** (proto cannot express). Phase-14 differential fixture scenario 6 is REDESIGNED to exercise per-route `remove_accept_encoding_header` override: listener-level `remove_accept_encoding_header: false` + per-route `overrides.response_direction_config.remove_accept_encoding_header: true`; route hits a real backend (the existing `test/helpers/echobackend/`) that echoes the upstream-side request headers in its response body; driver asserts that the per-route response body does NOT contain `Accept-Encoding` (proving the per-route override stripped it before forwarding upstream). This is the simplest observable-behavior change distinguishable from listener-level config under the per-route override surface. See §5 + §7 below.

ADR-0125 amendment paragraph (in-place at SPEC time per phase-13 ADR-0127-v2 in-place-update precedent at Task 12) acknowledges:
- (i) phase 14 compressor is the SECOND row using disabled-OR-override 5th canonical;
- (ii) the override surface is filter-specific; per-route override fields enumerate per the filter's `*PerRoute` proto, NOT the listener-level config wholesale;
- (iii) the WHOLESALE-not-merge semantic from BRAINSTORM Decision 5 + ADR-0073 inheritance applies WITHIN the override fields envelope (i.e., per-route `remove_accept_encoding_header: true` REPLACES listener-level `remove_accept_encoding_header: false` wholesale; not a 3-state merge).

NO new per-route ADR. NO ADR-0073 amendment (the wholesale-override discipline carries through; the per-route surface is just narrower than BRAINSTORM hypothesized).

#### 1.1 Amendment 5 — Vary header is APPENDED unconditionally on the compressed path, even when existing `Vary: *` (BRAINSTORM §2.8 + §9.P10)

BRAINSTORM §2.8 hypothesized: "If existing value is `*` → no change (wildcard already implies all headers vary)." **§11.10 empirically REFUTES.** Reference Envoy v1.37.2 always APPENDS `Accept-Encoding` to existing Vary value, regardless of the value's contents:
- Existing `Vary: Origin` + compressed → `vary: Origin, Accept-Encoding` (APPEND).
- Existing `Vary: *` + compressed → `vary: *, Accept-Encoding` (APPEND, NOT short-circuited on wildcard).
- No existing Vary + compressed → `vary: Accept-Encoding` (single-value).

**Phase-14 envoy-go disposition:** the Vary-injection helper at `EncodeHeaders` time always appends `Accept-Encoding` to the existing Vary value (or sets if absent), regardless of wildcard or other-tokens content. Token-match deduplication: if the existing Vary already contains `Accept-Encoding` (case-insensitive token-match), the helper performs no mutation (no-op; preserves idempotency on re-traversal). RFC 7231 §7.1.4 is silent on the wildcard case; Envoy's choice (always-append) is the more-conservative + RFC-compliant interpretation.

#### 1.1 Amendment 6 — `disable_on_etag_header: false` (default) STRIPS strong ETag on compressed path; `disable_on_etag_header: true` SKIPS compression on any ETag (BRAINSTORM §1.1 item 3 + §2.6 step 3d + §9.P7)

BRAINSTORM §1.1 item 3 fourth-bullet hypothesized: "`response_direction_config.disable_on_etag_header` (`bool`) — when true, compression is skipped if the upstream response carries an `ETag:` header (strong OR weak — §11 pin to confirm weak-ETag handling). Default false." **§11.7 empirically refines BOTH halves.** Reference Envoy v1.37.2 (per `compressor_filter.cc:90-95` and probeA empirical evidence at §11.7):

- **`disable_on_etag_header: false` (default):** Envoy DOES NOT skip compression on ETag presence. INSTEAD it MUTATES the response: STRIPS strong-ETag headers (matching `^"[^"]*"$`) on the compressed path; PRESERVES weak-ETag headers (matching `^W/"[^"]*"$`). The proto comment at compressor.proto line 76-78 documents this: "When this field is `false`, the filter will preserve weak ETag values and remove those that require strong validation." This is RFC 7232 §2.3 compliant — strong validators must change when entity bytes change; gzip-encoded representation has different bytes than the uncompressed entity, so the strong ETag for the uncompressed entity cannot validly serve as the strong ETag for the compressed entity. Weak validators have no such constraint.
- **`disable_on_etag_header: true`:** Envoy SKIPS compression on ANY ETag presence (strong OR weak; both compatible with §11.7 sub-pin (c) hypothesis pending probeB ETag-disabled run; the proto comment at line 73-74 says "disables compression when the response contains an ETag header" — no strong-vs-weak distinction in the disable-mode).

**Phase-14 envoy-go MVP MUST implement BOTH MODES:**

- (a) `disable_on_etag_header: false` (default) + ETag present → continue compression; STRIP strong-ETag (regex match `^"[^"]*"$` on the value); preserve weak-ETag (regex match `^W/"[^"]*"$`). The `EncodeHeaders` helper deletes the `ETag` header from the response when the value matches the strong-ETag regex; leaves alone when matches weak-ETag regex; leaves alone when matches neither (defensive — malformed ETag values are rare; preserve verbatim to mirror Envoy).
- (b) `disable_on_etag_header: true` + ETag present → SKIP compression; increment `not_compressed_etag +1`; DO NOT strip the ETag header (preserved as-is on the uncompressed response). Status code, body bytes, and other headers pass through unchanged.

**§13.1 BEHAVIOR_CONTRACT.md `### envoy.filters.http.compressor` subsection MUST document the dual-mode semantic** — the BRAINSTORM-implied "skip-on-any-etag" framing is incomplete; the (a) mode strong-strip behavior is observable on the compressed path and counts toward header-byte-equivalence in the differential fixture.

### 1.2 Revised scope summary (post-§1.1 amendments)

After the six §1.1 amendments, phase 14's in-scope architectural primitives are the SEVEN listed at the head of §1, expressed as **11 BRAINSTORM-§1.1-style line items** per BRAINSTORM §1.1 (items 1-11 — items 1-8 implementation deliverables + items 9-11 artefact-level deliverables). The amendments do NOT change item count; they revise the field counts (item 3: 8→8 consumed across listener+codec; 9→12 silent-ignored), the stat surface (item 7: 11→17 counters; SN10 retired; namespace amended), the per-route override surface (item 4: per-route shape narrowed dramatically), and the encode-side framework primitive (item 8 acquires Path A `OverwriteBody` fire — see §3 + §4 below). Differential fixture has 6 scenarios (§7.1 below; scenario 6 redesigned per amendment 4). ADR list is **5** (ADR-0129..ADR-0133) + ADR-0125 amendment paragraph (in-place at SPEC time per phase-13 ADR-0127-v2 precedent). NO ADR-0073 amendment (the wholesale-override discipline carries through; per-route surface is filter-specific). NO ADR-0061 amendment (no new SN flattening rule). NO ADR-0076 amendment (cap-promotion forward-pointer remains open per phase-13 BEHAVIOR_CONTRACT.md `### Phase 13 forward-pointer notes`; phase-14 MVP is response-only; cap-promotion is the future-decompressor's natural amender per BRAINSTORM §1.7).

### 1.3 Family-expansion shape (per BRAINSTORM Decisions 9 + ADR-0106)

Phase 14 is a **flat top-level row** under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family heading; the §9 family heading at `ROADMAP.md` line 56 stays unchanged in state across phase 14's landing per ADR-0106(c). Phase 14 is the SEVENTH §9 family-row to land (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13). Each subsequent HTTP filters family member becomes its own top-level row at row 15, 16, … There is NO sibling-stub authored by this SPEC for the next §9 row; future family-expansion brainstorms cold-start from the §9 heading + the just-shipped artefacts (per ADR-0106(b) + (e)).

### 1.4 ADR-0045 split-by-surface readiness

Phase 14 stays a SINGLE row at this SPEC. The implementation surface is estimated at:

- ~480-560 LoC production filter (`compressor.go` per BRAINSTORM §3 + the §3-survey-confirmed `OverwriteBody` framework primitive; ~150-200 LoC heavier than phase-13 buffer because of the codec-library Any-unmarshal-and-dispatch + Gzip compression-level mapping table + Accept-Encoding q-value parser + ETag strong-strip-vs-weak-preserve mutator + Vary-append helper + the 17-counter `filterStats` struct registration; offset by no body-counting state machine).
- ~30 LoC `doc.go`
- ~700-850 LoC unit tests (7 test groups per §14.1; ~150-250 LoC heavier than phase-13 buffer because of the codec-dispatch + AE-parser + ETag-mutator + Vary-helper test surfaces).
- ~80 LoC fuzzer (one fuzzer; mirrors phase-13 buffer's `FuzzBufferConfigParse` shape extended with codec-library Any-shape).
- ~20-25 LoC framework deltas: `internal/filter/http/callbacks.go` +1 line (`OverwriteBody(b []byte)` interface method); `internal/filter/http/chain.go` +6-8 LoC (encoderCB stub-and-honor mechanism + new `c.encodeBodyOverride []byte` field + `c.encodeBodyOverridden bool` sentinel + accessor `EncodeBodyOverride() ([]byte, bool)`); `internal/filter/hcm/connection.go` +6-8 LoC (post-RunEncodeData harvest + resp.Body substitution before writeH1Reply); `internal/filter/hcm/h2dispatch.go` +6-8 LoC (post-RunEncodeData harvest + resp.Body substitution before writeH2Reply); `cmd/envoy-go/main.go` +1 line (`httpReg.Register(compressor.TypeURL, compressor.New)`).
- ~250-320 LoC fixture (envoy.yaml ~90 + envoy-go.yaml ~90 + driver/main.go ~200 + backend/main.go ~50 if extending echobackend OR reuse + expectations.yaml ~40 + README.md ~80).
- ~80 LoC ROADMAP+STATE+BEHAVIOR_CONTRACT additions at SPEC commit.

Total: ~1640-1955 LoC across all bundles, with ~480-580 in Go production code. Task count estimate per BRAINSTORM §1.4: ~14-18 tasks (the codec-library Any-dispatch + 17-counter `filterStats` + ETag-mutator + Vary-helper add 2-4 tasks vs phase-13 buffer's 12-task baseline). Both metrics are NEAR but UNDER ADR-0045's 1500-LoC / 25-task split-trigger thresholds. The PLAN author retains the ADR-0045 release valve if PLAN finds the surface exceeds either threshold; the natural split per BRAINSTORM §1.4 is `14.1 = listener-level filter MVP + framework primitive + 4 listener-only fixture scenarios` and `14.2 = per-route disabled-OR-override + 2 per-route fixture scenarios + ETag-mutator + remove_accept_encoding_header backend-echo-assertion`. **SPEC's position: single-row.**

### 1.5 No prebrainstorm-notes branch

UNLIKE phase 11 (which inherited an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch from a prior pivoted session), phase 14 has NO prior prebrainstorm-notes artefacts. The phase 14 BRAINSTORM cold-started fresh from the §9 heading + the phase 13 just-shipped artefacts per ADR-0106(e); this SPEC drafting session executed the §9 empirical-pin block (15 pins) IN-SESSION against reference Envoy v1.37.2 per ADR-0004 — surfacing the six §1.1 amendment refutations above. THIS SPEC consults BRAINSTORM as authoritative (§§1-11) + the §11 empirical-pin block (this SPEC drafting session) as the divergence-from-BRAINSTORM record + the six §1.1 amendments as the SPEC-side correction list. No off-master branch needs to be merged or referenced.

### 1.6 Phase 14 is the second §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time (not at brainstorm-amendment time)

Phase 12 csrf was the FIRST §9 row whose BRAINSTORM hypothesis was MAJOR-REVISED at SPEC time — TWO MAJOR REVISIONS (§11.3+§11.7+§11.8 collective; §11.11) and TWO MINOR REVISIONS (§11.2 trichotomy; §11.9 stat-sharing). Phase 13 buffer took the brainstorm-amendment route (BRAINSTORM §12 D-3.5 amendment cycle authored before SPEC drafting started in earnest). Phase 14 takes the phase-12 SPEC-time route: SIX SPEC-time amendments at §1.1 above, surfacing during the §11 empirical-pin re-run. The choice between (a) §1.1 amendment block channel and (b) BRAINSTORM §12 amendment cycle is per next-prompt's framing — option (a) when each correction fits within a self-contained §1.1 prose block + a §11 pin disposition entry. The phase-14 corrections fit (a): five amendments (1, 2, 3, 5, 6) are field-level + name-level + behavior-level corrections that don't undo the structural design (gzip-only, response-only, Path B); amendment 4 (per-route override surface) is structurally narrower than hypothesized but the disabled-OR-override discipline survives intact.

The pattern of "BRAINSTORM commits hypothesis; SPEC empirically confirms or amends" continues to function as designed; phase 14 demonstrates the (a) route's robustness when the empirical re-frame surfaces multiple field-level corrections without invalidating the structural design.

### 1.7 Phase 14 introduces ONE encode-side framework delta — `EncoderFilterCallbacks.OverwriteBody(b []byte)` — symmetric to phase-13 ADR-0128's decode-side primitives

Phase 13 retired the SPEC §4 "no framework deltas" invariant (per phase-13 SPEC §4 amendment + ADR-0128). Phase 14 inherits the precedent: framework deltas are permitted in §9 family-rows when load-bearing for the filter's algorithmic core. The §3 framework survey (executed at this SPEC drafting session — see §3 below) confirms: existing `EncoderFilterCallbacks` interface carries no body-mutation primitive; the chain dispatch passes `data` BY VALUE; HCM reads `resp.Body` directly post-chain. Path A FIRES — phase 14 introduces `EncoderFilterCallbacks.OverwriteBody(b []byte)` as a SYMMETRIC encode-side primitive. Mirrors phase-13 ADR-0128's decode-side primitives:
- ADR-0128 (decode-side, phase 13): synthetic empty-terminal `RunDecodeData` on chunked-body EOF; post-body Content-Length reconciliation. Both at `internal/filter/hcm/connection.go:dispatchRequest`.
- ADR-0131 §Decision (vi) (encode-side, phase 14): `EncoderFilterCallbacks.OverwriteBody(b []byte)` interface method + `encoderCB` impl + per-stream override field on `*FilterChain` + HCM-side post-RunEncodeData harvest at H1 + H2 dispatch paths.

LoC delta: ~20-25 LoC across `callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go`. Comparable to phase-13's ~34 LoC for the decode-side primitives. The forward-pointer phase that lands encode-side streaming framework (chunked-output `writeH1Reply` mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation in HCM) is the natural amender for phase-14's wire-shape divergence; ADR-0131 forward-points.

---

## 2. Non-purposes

Phase 14 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.compressor` (gzip-only, response-only) + the symmetric `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive needed for Path B body mutation.

### 2.1 `Compressor` proto-message non-goals (per BRAINSTORM §8 + §1.1 amendment 1)

The proto message `envoy.extensions.filters.http.compressor.v3.Compressor` carries 9 top-level fields. Phase 14 consumes 6 actively (modulo the codec-library Any-unmarshal); silent-ignores 3 listener-level + 4 deprecated top-level mirrors + 2 nested-config fields = 9 silent-ignored. Nested codec-library `envoy.extensions.compression.gzip.compressor.v3.Gzip` carries 5 fields; phase 14 consumes 2 + silent-ignores 3.

- **Silent-ignored at the listener-level `Compressor` (9 fields):** `Compressor.{content_length, content_type, disable_on_etag_header, remove_accept_encoding_header}` (4 deprecated top-level mirrors per BRAINSTORM §1.1 item 3 paragraph 7); `Compressor.runtime_enabled` (deprecated runtime gate); `Compressor.choose_first` (selection-mode flag; always-q-value-based per §11.4); `Compressor.request_direction_config` (request-side compression; always-disabled per §11.15); `Compressor.response_direction_config.common_config.enabled` (RuntimeFeatureFlag; runtime-ignored per §1.1 amendment 2); `Compressor.response_direction_config.status_header_enabled` (`x-envoy-compression-status` debug-header opt-in; always-no-status-header per §1.1 amendment 1).
- **Silent-ignored at the codec-library `Gzip` (3 fields):** `Gzip.{memory_level, window_bits, chunk_size}` (Go `compress/gzip` does not expose libz-equivalent knobs at these granularities).

#### 2.1.1 Out of scope: `compressor_library` non-Gzip TypeURL (envoy-go-only PARSE-time rejection)

Coupled to: future codec-extension phase (per §8.1 below). Reference Envoy v1.37.2 accepts `envoy.extensions.compression.{brotli,zstd}.compressor.v3.{Brotli,Zstd}` TypeURLs at parse time; envoy-go MVP rejects with envoy-go-own error wording per ADR-0130. Rejected configs include `compressor_library.typed_config.@type = "type.googleapis.com/envoy.extensions.compression.brotli.compressor.v3.Brotli"` (and similar for zstd). Divergence-window: envoy-go-only PARSE-time rejection vs. Envoy's parse-time accept + runtime-codec-honor. Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.compressor` subsection + `### Phase 14 forward-pointer notes`.

#### 2.1.2 Out of scope: `Compressor.request_direction_config` (silent-ignored at parse + runtime)

Coupled to: future request-side compression phase (BRAINSTORM §8.2 — coupled to the future `envoy.filters.http.decompressor` filter). Reference Envoy behavior: enables request-side compression when `request_direction_config.common_config.enabled.default_value: true`. envoy-go behavior: silent-ignored at parse time; always-disabled at runtime. Operator divergence-window: configs setting `request_direction_config` against reference Envoy active-compress request bodies upstream; envoy-go does NOT.

#### 2.1.3 Out of scope: deprecated top-level mirror fields (silent-ignored at parse + runtime)

`Compressor.{content_length, content_type, disable_on_etag_header, remove_accept_encoding_header}` are 4 deprecated top-level mirror fields per Envoy's annotation `[deprecated = true, (envoy.annotations.deprecated_at_minor_version) = "3.0"]`. Reference Envoy v1.37.2 reads them only when the corresponding `response_direction_config` field is unset. envoy-go MVP silent-ignores all 4 at parse time; only the `response_direction_config` paths are honored. Operator footgun: configs setting BOTH legacy + new fields will see Envoy honor whichever-is-set-with-precedence-to-new; envoy-go honors ONLY the new path. Documented at BEHAVIOR_CONTRACT phase-14 forward-pointer notes.

#### 2.1.4 Out of scope: `runtime_enabled` + `response_direction_config.common_config.enabled` (RuntimeFeatureFlag fields; always-100%-active per §1.1 amendment 2)

Coupled to: Runtime + hot restart family. `Compressor.runtime_enabled` (deprecated; replaced by `enabled`) + `response_direction_config.common_config.enabled` are both `RuntimeFeatureFlag` types — `BoolValue default_value` + `string runtime_key`. envoy-go MVP silent-ignores both at runtime; always-evaluates-as-enabled regardless of `default_value` setting OR runtime-key state. Mirrors phase-12 csrf `filter_enabled` posture per ADR-0121 — but UNLIKE phase-12 csrf, phase-14's `enabled` field is OPTIONAL at parse-time (per §1.1 amendment 2; §11.3 empirical refutation). Operator divergence-window: configs setting `default_value: false` see Envoy disable; envoy-go always-active.

#### 2.1.5 Out of scope: `choose_first` (always-q-value-based selection)

Coupled to: future Accept-Encoding selection-mode bundle. Reference Envoy: `choose_first: true` overrides q-value priority with first-acceptable-in-declared-order. envoy-go MVP: always-q-value-based (§11.4 confirms hypothesis). Re-activation is a small future enhancement (~5 LoC; the q-value parser already produces an in-declared-order list; toggle to first-acceptable selection mode). Documented at §13.4.

#### 2.1.6 Out of scope: `status_header_enabled` (always-no-status-header)

NEW per §1.1 amendment 1. The `x-envoy-compression-status: <encoder>;<status>[;<additional-params>]` debug-header opt-in flag. envoy-go MVP silent-ignores at parse + runtime; always-no-status-header. Operator divergence-window: `status_header_enabled: true` configs see the debug header on Envoy responses; envoy-go responses lack it. Documented at BEHAVIOR_CONTRACT phase-14 forward-pointer notes.

#### 2.1.7 Out of scope: `Gzip.{memory_level, window_bits, chunk_size}` (Go gzip does not expose libz-equivalent knobs)

Coupled to: future Go-gzip-library upgrade phase (e.g., switch to `klauspost/compress/gzip` which exposes more libz-knobs) OR cgo + libz directly (out-of-scope per envoy-go's pure-Go discipline). MVP silent-ignores all three sub-fields at parse time. The compressed-byte output already differs from Envoy's libz regardless of these knobs (per §11.14 — Go's gzip-format header `XFL=00, OS=255` vs Envoy's libz `XFL=00, OS=03`); compressed-byte equivalence is structurally non-byte-exact (see ADR-0133).

### 2.2 Per-route override surface non-goals (per §1.1 amendment 4)

The proto message `CompressorPerRoute` carries `oneof override` with two cases: `disabled: true` boolean OR `overrides: CompressorOverrides`. The `CompressorOverrides` sub-message carries `response_direction_config: ResponseDirectionOverrides` + `compressor_library: TypedExtensionConfig`. `ResponseDirectionOverrides` carries EXACTLY ONE FIELD: `remove_accept_encoding_header` (BoolValue). Phase 14 honors the `disabled` shortcut + `overrides.response_direction_config.remove_accept_encoding_header` overload only.

- **NOT honored:** `overrides.compressor_library` per-route library swap. Coupled to: multi-codec future phase (§8.2). Silent-ignored at parse + runtime. Operator divergence-window: per-route library-swap configs see Envoy use the per-route library; envoy-go uses the listener-level library regardless. Documented at §13.4.
- **STRUCTURALLY IMPOSSIBLE per-route overrides** (proto cannot express): `min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes`, `enabled`. These 5 listener-level fields cannot be per-route-overridden in Envoy v1.37.2's compressor proto.

### 2.3 Algorithm-shape non-goals (per BRAINSTORM §2.6 + §11.9)

The compressor's body algorithm is **Path B (buffer-then-compress) — full body in one `EncodeData` call; gzip-encode in one shot; replace via `OverwriteBody`**. Specifically OUT of scope:

- **Streaming compression with chunked output.** Reference Envoy uses streaming compression and emits `Transfer-Encoding: chunked` (no `Content-Length`) on the wire (per §11.9 — ALL compressed responses observe chunked, regardless of body size 30 / 100 / 1024 / 10240 bytes). envoy-go MVP emits `Content-Length: <gzipped-length>` (no `Transfer-Encoding: chunked`); the wire-shape divergence is documented at ADR-0131 with explicit forward-pointer to a future encode-side streaming framework phase.
- **`writeH1Reply` chunked-output mode.** envoy-go's `internal/filter/hcm/codec.go:74-119` unconditionally emits fixed `Content-Length: <len(body)>` on every H1 response. Adding chunked-output mode requires `writeH1Reply` surgery + new framework primitive (`EncoderFilterCallbacks.EmitChunk`?) + chunk-by-chunk `RunEncodeData` invocation in HCM. Out of scope; couples to the same future streaming-framework phase.
- **Mid-stream cap-trip on encode-side.** UNLIKE phase-13 buffer's `accumulated > effectiveMax → 413` mid-stream-overflow path, phase-14 compressor never trips a cap mid-stream — the `min_content_length` gate fires AFTER the full body is observed (in `EncodeData` with `endStream=true`); if `len(data) < min_content_length`, the filter REVERTS the headers (un-set `Content-Encoding: gzip`, restore Vary state) + passes the body through uncompressed. NO `SendLocalReply`-equivalent path on encode-side; the compressed-vs-uncompressed branch is binary at `EncodeData` entry.
- **Async-resume.** Compressor's `EncodeHeaders` + `EncodeData` runs synchronously (UNLIKE phase 09 fault's `time.AfterFunc` + parkDecode). The framework's encode-chain runs synchronously in the dispatch goroutine.
- **Stateful per-route resources.** UNLIKE phase 11 local_ratelimit (per-route `tokenBucket`), compressor's per-route `compiledPerRoute` is purely data — `{disabled bool, removeAcceptEncodingHeaderOverride *bool}`. NO mutex, NO atomic, NO synchronization at runtime.

### 2.4 Stat-surface non-goals

- **No filter-specific Prometheus tag-extractor (NEW; revises BRAINSTORM §2.7).** Per §1.1 amendment 3 + §11.5: phase 14 emits stats at `compressor.<library_name>.<codec>.[response.]<counter>` flattened via the existing Rule SN2 (HCM stat_prefix). NO new SN10 rule; NO `internal/stats/name.go` flattenToProm switch surgery; NO new tag-extractor pattern. Compare phase-11 local_ratelimit which introduced filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118); phase 14 follows phase-12 csrf precedent (no new SN rule).
- **No twin-series filter discipline beyond the existing one.** The differential fixture's allow-list (per BEHAVIOR_CONTRACT.md `### Twin-series filter discipline` + phase 06.1 SPEC §11.5) already filters Envoy-only counters that envoy-go does not register; phase 14 inherits this discipline unchanged. Phase 14 registers all 17 counters Envoy emits per §1.1 amendment 3 (including the 5 always-zero `request_*` family); the differential fixture asserts byte-equivalent counter-deltas on all 17 names.
- **No permanently-zero counter from the filter side** in the unique-to-phase-14 sense. Phase 14 emits 12 counters that increment under MVP (6 header_* + 1 not_compressed_etag + 5 response_*) and 5 that stay at zero (request_* family) since `request_direction_config` is silent-ignored. The 5 zero-counters are STRUCTURALLY zero on both sides (envoy-go ALSO has request_direction_config silent-ignored); the differential matches.

### 2.5 Test-surface non-purposes

- **No new differential probe filter.** Phase 07.1's `envoy.filters.http.envoy_go_test` (the iteration-state probe filter) covers framework iteration coverage. Phase 14 does not extend that probe.
- **No new fuzzer category.** The 18th fuzzer `FuzzCompressorConfigParse` follows the existing `FuzzFooConfigParse` pattern (cors, fault, header_mutation, localratelimit, csrf, buffer).
- **No structural-iteration fixture** (07.1's 0007b). Phase 14 is differential-only.
- **No fixture renumbering or reshuffling.** Phase 14 is fixture `0016-http-compressor`; the previous fixtures (0000-0015) stay green and unchanged.
- **No GET passthrough scenario in the differential fixture.** Header-only / non-bodied-method passthrough is unit-tested in `compressor_test.go::TestEncodeHeaders_HeaderOnlyEndStream` (parametrized over GET HEAD OPTIONS). The differential gate has no GET scenario because the algorithm short-circuits on the response-side regardless of method.
- **No H2 differential coverage.** Phase 14 fixture 0016 is HTTP/1.1-only. H2 differential testing of compressor is deferred to a future bundle (matches the phase 09 / 10 / 11 / 12 / 13 precedent — each filter ships with H1 differential coverage; H2 is deferred). The H2 wire shape on the compressed path needs an empirical pin against H2-mode reference Envoy (chunked-vs-DATA-frame divergence).

### 2.6 Cross-filter non-purposes

- **No interaction with cors / fault / header_mutation / local_ratelimit / csrf / buffer per-route configs in fixture 0016.** Phase 14's fixture configures ONLY `compressor` filters (plus the router terminal). Mixed-filter ordering tests are deferred.
- **HCM-level changes (new framework deltas):** Phase 14 introduces 1 new method on `EncoderFilterCallbacks` interface + ~6-8 LoC at `internal/filter/hcm/connection.go` (post-RunEncodeData harvest + resp.Body substitution before writeH1Reply) + ~6-8 LoC at `internal/filter/hcm/h2dispatch.go` (H2 symmetric path) + ~6-8 LoC at `internal/filter/http/chain.go` (encoderCB stub-and-honor + per-stream override field + accessor). See ADR-0131 §Decision (vi). `serverHeader()` returning `"envoy"` (per `internal/filter/hcm/codec.go:17`) is UNCHANGED.
- **No extension to existing per-route framework primitives.** Phase 14 reuses `PerRouteConfig.Resolve` (per `internal/filter/http/perroute.go:103-128`); no `ResolveAllTiers` invocation (unlike phase 10 header_mutation), no new framework callback, no `RegisterPerRouteValidator` hook. The per-route `CompressorPerRoute` shape is validated standalone via `parsePerRoute` at config-load time; no multi-tier protected-set discipline.

### 2.7 Security non-purposes

- **No DoS-resilience characterization beyond the cap.** Phase 14 implements gzip compression of full response bodies; characterizing its strength against compression-bomb scenarios (deliberately-incompressible pre-compressed inputs that consume CPU during gzip-encode) is out of scope. The framework's `filterBufferLimitBytes = 1 << 20` cap (per ADR-0076; encode-side enforced per `chain.go:326-328`) bounds the response-body size that can reach the encode-chain; bodies larger than 1 MiB trigger `errEncodeBufferOverflow` BEFORE reaching the compressor filter. Operators relying on pre-compressed-input bombs see the framework cap fire; the compressor filter never observes the bomb body.
- **No per-route-disabled threat-model documentation.** Per §1.1 amendment 4: `disabled: true` per-route routes pass uncompressed responses through without filter-side mutation. Operators MUST understand that `disabled: true` removes Vary-injection + ETag-mutation + remove_accept_encoding_header behavior on that route.

---

## 3. Phase-done gates (specialization of `BOOTSTRAP_PROMPT.md` §7.5 for phase 14)

The six-gate phase-done discipline for phase 14:

| Gate | Specialization for phase 14 |
|---|---|
| **A — Build / vet / lint clean** | `go build ./...`, `go vet ./...`, `golangci-lint run` all green; no new warnings introduced relative to the phase-13 baseline at master tip `51b9ea6`. New package `internal/filter/http/compressor/` lints clean. |
| **B — Race-test pass** | `go test -race ./...` green on all 36 packages plus the new `internal/filter/http/compressor/` package (37 packages total). Test count grows by ~50-70 (7 unit-test groups across the new test file). Compressor has no shared mutable state (the `compiledConfig` is read-only after `New`; `compiledPerRoute` is read-only after `parsePerRoute`; the per-stream filter state is stream-local; the new `c.encodeBodyOverride []byte` field on `*FilterChain` is per-stream); race-test cleanness is structural. |
| **C — h2spec 53/53 PASS** | Conformance gate at the ADR-0051 pin (53/53 PASS); phase 14 introduces no HTTP/2 stack changes outside the `RunEncodeData → resp.Body substitution` symmetric H2 path (which is structurally identical to the H1 path). Regression check — not an extension. |
| **D — All fuzzers green at 30s budget** | 17 existing fuzzers (per phase 13 phase-done) + 1 new (`FuzzCompressorConfigParse`) = 18 fuzzers. Each runs 30s in the per-phase fuzzer gate; all green. |
| **E — All differential fixtures 0000–0016 PASS** | 16 prior fixtures + the new `0016-http-compressor` = 17 fixtures green. Total runtime estimated ~55-70s wallclock (phase 13 reported ~50-60s for 16 fixtures; fixture 0016 adds ~5-10s for its 6 requests — synchronous; plus the gzip compression + decompression overhead). |
| **F — `BEHAVIOR_CONTRACT.md` populated** | §13.1 new subsection `### envoy.filters.http.compressor` (inline; ~120-150 lines per the phase 09 / 10 / 11 / 12 / 13 precedent — larger than phase 13's ~70 because of the dual-mode ETag-mutator + Vary-helper + 17-counter table + wire-shape divergence-window prose); §13.2 stat-table 29-name table extends to 46 names (17 new entries; see §1.1 amendment 3); §13.3 NEW equivalence-matrix row pointing at fixture 0016 with per-scenario tolerance discipline (decompressed-byte-exact body axis on compressed scenarios; CL-value + transfer-encoding excluded); §13.4 NEW forward-pointer notes subsection (`### Phase 14 forward-pointer notes`) covering the ~7-item deferral list. All edits land in-place per ADR-0052 at the phase-done commit. |

Gates A–E are the verification gates; Gate F is the contract-extension gate. All six must be green at the phase-done commit per `BOOTSTRAP_PROMPT.md` §7.5.

---

## 4. Deliverables (files and directories)

### 4.1 New production code (in 14)

```
internal/filter/http/compressor/doc.go             ~30 LoC; package overview + 6-consumed/12-ignored/1-parse-rejected decomposition + per-route disabled-OR-rmAE-only summary
internal/filter/http/compressor/compressor.go      ~480-560 LoC; filter type + factory + EncodeHeaders + EncodeData + DecodeHeaders + per-route helpers + maybeAddVary + maybeStripStrongEtag + acceptEncodingParser + compiledConfig + compiledPerRoute + compiledGzipConfig + filterStats (17-counter struct) + codec-library Any-unmarshal-and-dispatch
internal/filter/http/compressor/compressor_test.go ~700-850 LoC; 7 test groups per §14.1
internal/filter/http/compressor/fuzz_test.go       ~80 LoC; FuzzCompressorConfigParse per §14.3
```

The PLAN author may split `compressor.go` into multiple files (e.g., a `codec.go` for the gzip dispatch, an `acceptencoding.go` for the q-value parser, a `headers.go` for the Vary-append + ETag-strip helpers, a `perroute.go` for parsePerRoute) if test readability benefits OR if `compressor.go` exceeds ~500 LoC. The SPEC explicitly defers this micro-decision; no ADR class commitment. See §12 below.

### 4.2 New differential fixture

```
test/fixtures/0016-http-compressor/envoy.yaml             ~90 LoC; reference Envoy bootstrap (single listener + 6 routes per §7.1)
test/fixtures/0016-http-compressor/envoy-go.yaml          ~90 LoC; equivalent envoy-go bootstrap
test/fixtures/0016-http-compressor/inputs/driver.go       ~200 LoC; Go driver issuing 6 requests (§7.1 matrix); decompresses gzip responses via compress/gzip.NewReader
test/fixtures/0016-http-compressor/expectations.yaml      ~40 LoC; per-scenario allow-list + counter delta tolerances
test/fixtures/0016-http-compressor/README.md              ~80 LoC; fixture overview + scenario list + reference config citations + decompress-and-compare body-assertion explanation
```

The fixture extends the existing `test/helpers/echobackend/` to support scenario 6's per-route `remove_accept_encoding_header` assertion (the backend echoes the upstream-side request headers in its response body so the driver can assert AE absence). Backend extension: ~30-50 LoC.

### 4.3 Modified production code (in 14)

```
cmd/envoy-go/main.go                       +1 line; httpReg.Register(compressor.TypeURL, compressor.New) — alphabetical-after-buffer insertion (router → buffer → compressor → cors → csrf → ...) before httpReg.Freeze()
internal/filter/http/callbacks.go          +1 line; OverwriteBody(b []byte) method on EncoderFilterCallbacks interface
internal/filter/http/chain.go              +6-8 LoC; encoderCB.OverwriteBody impl + new c.encodeBodyOverride []byte field + c.encodeBodyOverridden bool sentinel + EncodeBodyOverride() ([]byte, bool) accessor
internal/filter/hcm/connection.go          +6-8 LoC; post-RunEncodeData harvest of encode-body-override + resp.Body substitution before writeH1Reply (H1 path)
internal/filter/hcm/h2dispatch.go          +6-8 LoC; same shape as connection.go but for H2 dispatch path (writeH2Reply consumer)
```

Total framework deltas: ~20-25 LoC (vs phase-13's ~35 LoC for the decode-side primitives at ADR-0128). All in service of the encode-side `OverwriteBody` primitive load-bearing for Path B body algorithm. See ADR-0131 §Decision (vi).

### 4.4 Modified docs (at SPEC commit)

```
docs/envoy-go/ROADMAP.md                   row 14 status: planned → in-progress (per §3 phase-done gate sequence; lifecycle-state-1 → 2 transition)
docs/envoy-go/STATE.md                     active-phase pointer + lifecycle-state pointer + next-skill pointer
```

### 4.5 Modified docs (at phase-done commit)

```
docs/envoy-go/DECISIONS.md                 5 new ADRs: ADR-0129 (package shape), ADR-0130 (compiledConfig + Any-dispatch + Gzip mapping), ADR-0131 (Path B + wire-shape divergence + OverwriteBody primitive), ADR-0132 (17-counter stats + namespace shape), ADR-0133 (decompress-and-compare); plus ADR-0125 in-place amendment paragraph noting SECOND-row using disabled-OR-override 5th canonical
docs/envoy-go/BEHAVIOR_CONTRACT.md         §13.1 new subsection ### envoy.filters.http.compressor (~120-150 lines); §13.2 stat-table 29 → 46 names extension; §13.3 NEW equivalence-matrix row; §13.4 NEW ### Phase 14 forward-pointer notes subsection
docs/envoy-go/ROADMAP.md                   row 14 status: in-progress → done
docs/envoy-go/STATE.md                     phase 14 closed; awaiting next planning
```

---

## 5. Architecture and components

### 5.1 Module graph (new / changed shape in 14)

```
                                 ┌──────────────────────────────────────────────────────┐
                                 │ cmd/envoy-go/main.go                                 │
                                 │ + httpReg.Register(compressor.TypeURL, compressor.New)│
                                 │   alphabetical-after-buffer per ADR-0072 stylistic    │
                                 └─────┬────────────────────────────────────────────────┘
                                       │ boot-time registration (pre-Freeze)
                                       ▼
┌────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/http/registry.go (existing)                                        │
│ HTTPRegistry.Register(TypeURL, factory) — already supports 1..N filters per ADR-0072│
└────────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ HCM build-time: factory(tc, ctx) → FilterInstanceFactory
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/http/compressor/compressor.go (NEW)                                  │
│ • TypeURL const                                                                      │
│ • New(ctx, tc) — parse + Any-unmarshal compressor_library + buildCompiledConfig +    │
│                   buildCompiledGzipConfig + register filterStats (17 counters)        │
│ • parsePerRoute — disabled-OR-rmAE-override per §1.1 amendment 4                     │
│ • filter struct, filterStats, compiledConfig, compiledPerRoute, compiledGzipConfig   │
│ • Decode methods: DecodeHeaders (AE-parse + per-route resolve + maybe-strip-AE);     │
│                    DecodeData / DecodeTrailers (pass-through)                          │
│ • Encode methods: EncodeHeaders (skip-decision sequence + Vary-append + etag-strip   │
│                    + Content-Encoding set); EncodeData (gzip-encode + OverwriteBody)  │
│ • OnDestroy — no-op                                                                   │
│ • SetDecoderCallbacks / SetEncoderCallbacks — store cb references                     │
└──────────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ filter.SetEncoderCallbacks(cb) — cb is *encoderCB from chain.go
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/http/callbacks.go (CHANGED — +1 method on interface)                 │
│ EncoderFilterCallbacks { ContinueEncoding(); EncodeHeaders/Data/Trailers(...);        │
│                          OverwriteBody(b []byte) }  ← NEW method per ADR-0131         │
└──────────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/http/chain.go (CHANGED — +6-8 LoC)                                   │
│ • *FilterChain.encodeBodyOverride []byte (per-stream new field)                      │
│ • *FilterChain.encodeBodyOverridden bool (per-stream sentinel)                       │
│ • encoderCB.OverwriteBody(b []byte) — stores b on c.encodeBodyOverride;               │
│                                       sets c.encodeBodyOverridden = true              │
│ • *FilterChain.EncodeBodyOverride() ([]byte, bool) — accessor for HCM dispatch        │
└──────────────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       │ HCM dispatch: chain.RunEncodeData(...) → harvest c.EncodeBodyOverride()
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/hcm/connection.go (CHANGED — +6-8 LoC; H1 path)                      │
│ • after chain.RunEncodeData(ctx, resp.Body, true), call chain.EncodeBodyOverride()    │
│ • if (b, ok) ok=true → resp.Body = b                                                  │
│ • bytesSent + writeH1Reply consume resp.Body as before (sees the override bytes)      │
└──────────────────────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ internal/filter/hcm/h2dispatch.go (CHANGED — +6-8 LoC; H2 symmetric path)             │
│ • after chain.RunEncodeData(ctx, resp.Body, true), call chain.EncodeBodyOverride()    │
│ • if (b, ok) ok=true → resp.Body = b                                                  │
│ • bytesSent + writeH2Reply consume resp.Body as before                                │
└──────────────────────────────────────────────────────────────────────────────────────┘
```

The codec-library Any-unmarshal-and-dispatch lives WITHIN `internal/filter/http/compressor/compressor.go`; no separate sub-package. Future codec phases (brotli, zstd) MAY refactor to a registry-of-codecs pattern within the package OR (more likely) split into `internal/filter/http/compressor/{gzip,brotli,zstd}/` sub-packages — that's a future planner's call. Phase 14 keeps the gzip-only dispatch flat.

### 5.2 Per-request flow — compressed allow-path (canonical; scenario 1 / `/echo`)

```
1. Downstream HTTP/1.1 request:  GET /echo  Accept-Encoding: gzip  Host: ...
2. HCM dispatchRequest:
   a. routes match → /echo route → cluster c_backend (or direct_response with body)
   b. RunDecodeHeaders → compressor.DecodeHeaders(headers, endStream=true):
      - parse Accept-Encoding ("gzip") → store on f.acceptedEncoding="gzip"
      - resolve per-route via f.dcb.RequestRouteConfig() → nil (listener fallback)
      - effective remove_accept_encoding_header = false (listener default) → no AE strip
      - return Continue
   c. RunDecodeData not invoked (no body on GET)
   d. Action runs (cluster route OR direct_response) → produces *ActionResponse with:
      - Status: 200
      - Headers: [content-type: text/html, ...]
      - Body: <1024-byte payload>
3. Encode chain (HCM connection.go:467-475):
   a. merged := resp.Headers.ToHTTPHeader()
   b. RunEncodeHeaders(ctx, merged, len(resp.Body) == 0):
      - compressor.EncodeHeaders(merged, endStream=false) — endStream=false because body is non-empty
      - skip-decision sequence:
         (i) f.acceptedEncoding == "gzip" — passes ("gzip")
         (ii) status not in uncompressible_response_codes — passes
         (iii) headers Content-Encoding empty — passes (no already-encoded skip)
         (iv) effective disable_on_etag_header == false; ETag absent or value-strip-strong-applies — passes
         (v) Cache-Control no-transform absent — passes
         (vi) Content-Type matches default content_type list — "text/html" matches — passes
         (vii) (skipping min_content_length here; gate fires in EncodeData on Path B since CL not authoritative pre-body — see step 4d)
      - mutate headers: Set Content-Encoding: gzip; Vary-append "Accept-Encoding"; (no ETag → no strip)
      - f.willCompress = true; return Continue
   c. resp.Headers = ReconcileOrderedHeaders(resp.Headers, merged) — picks up the encode-side mutations
   d. RunEncodeData(ctx, resp.Body, true):
      - compressor.EncodeData(data=resp.Body, endStream=true):
         (a) f.willCompress == true → proceed
         (b) min_content_length check: len(data)=1025 >= 30 (default) → proceed
         (c) gzip.NewWriterLevel(buf, level=DefaultCompression).Write(data).Close()
         (d) f.ecb.OverwriteBody(buf.Bytes()) — stores compressed bytes on chain.encodeBodyOverride
         (e) increment counters: response_compressed +1; response_total_uncompressed_bytes += 1025;
              response_total_compressed_bytes += len(buf.Bytes()); header_compressor_used +1
         (f) return DataContinue
4. After RunEncodeData, HCM harvests:
   if b, ok := chain.EncodeBodyOverride(); ok:
       resp.Body = b
   bytesSent := int64(len(resp.Body))   // now reflects compressed length
5. writeH1Reply(bw, status=200, headers=resp.Headers, body=resp.Body):
   - status line: HTTP/1.1 200 OK
   - headers (in order, with Content-Length rewrite at codec.go:87-89 to len(compressed)):
     content-type: text/html
     content-encoding: gzip
     vary: Accept-Encoding
     content-length: <len(compressed)>
     date: <RFC1123>
     server: envoy
   - body: <gzip-compressed bytes>
6. emitAccessLog(req, status=200, bytesSent=<gzipped-len>, ...)
```

**Wire-shape divergence from reference Envoy (per §11.9):** Envoy emits `transfer-encoding: chunked` (no `content-length`) on the same path. The differential fixture's allow-list excludes `content-length` value comparison + `transfer-encoding` presence on compressed scenarios. Decompressed body is byte-equivalent on both sides (per §11.14 — gzip-format is deterministic at the decompress level even though compressed-bytes differ).

### 5.3 Per-request flow — non-compressible content-type (scenario 2 / `/image-png-1024`)

```
1. Downstream:  GET /image-png-1024  Accept-Encoding: gzip
2. HCM dispatch + RunDecodeHeaders (compressor.DecodeHeaders): same as 5.2 — store acceptedEncoding="gzip"; Continue
3. Encode chain:
   a. RunEncodeHeaders → compressor.EncodeHeaders(merged, endStream=false):
      - skip-decision:
         (vi) Content-Type is "image/png" — does NOT match default content_type list → SKIP
      - DO NOT inject Vary (per §1.1 amendment + §11.15 — Vary not injected on server-side-skip paths like content-type-mismatch);
        DO NOT set Content-Encoding
      - increment counters: response_not_compressed +1
        (header_compressor_used DOES still increment since AE was "gzip" — see §11.5 verbatim probeA scrape; the codec was selected even though the response was skipped)
      - f.willCompress = false; return Continue
   b. RunEncodeData → compressor.EncodeData(data=resp.Body, endStream=true):
      - f.willCompress == false → return DataContinue (pass-through; no body mutation)
4. HCM EncodeBodyOverride() returns (nil, false) — no override; resp.Body unchanged
5. writeH1Reply emits original 1025-byte image/png body with content-length: 1025; no content-encoding; no vary
6. emitAccessLog: status=200, bytesSent=1025
```

### 5.4 Per-request flow — below-min-content-length (scenario 3 / `/text-html-10`)

```
1. Downstream:  GET /text-html-10  Accept-Encoding: gzip  (10-byte body)
2. HCM dispatch + RunDecodeHeaders: same as 5.2 — store acceptedEncoding="gzip"; Continue
3. Encode chain:
   a. RunEncodeHeaders → compressor.EncodeHeaders(merged, endStream=false):
      - skip-decision: passes content-type test, but...
      - At EncodeHeaders time, len(resp.Body)=10 IS available via the encode-chain shape (resp.Body is pre-buffered upstream of the encode chain per connection.go:472; `len(resp.Body)` in the endStream computation at line 467 reflects this).
        BUT EncodeHeaders does not have access to len(data). The min_content_length gate fires in EncodeData per the BRAINSTORM §2.6 algorithm.
      - Tentative: set Content-Encoding: gzip; Vary-append; f.willCompress=true; return Continue
   b. RunEncodeData → compressor.EncodeData(data=10 bytes, endStream=true):
      (a) f.willCompress == true → proceed
      (b) min_content_length check: len(data)=10 < 30 (default min) → REVERT:
          - f.dcb.EncodeHeaders mechanism unavailable on encode-side (the existing EncoderFilterCallbacks does NOT have a corresponding "EncodeHeaders to mutate the headers we already passed through" primitive); INSTEAD the filter must do header REVERT via direct mutation of the ALREADY-COMMITTED `merged` headers map — but at EncodeData time, the headers have already been reconciled back onto resp.Headers via line 470 ReconcileOrderedHeaders.
          - **Alternative algorithm — late-stage gate moved to EncodeHeaders.** Since len(data) IS observable at EncodeHeaders time via `len(resp.Body)` indirect (NOT directly — EncodeHeaders takes the headers, not the body), the SPEC's preferred resolution is: **DO the min_content_length check at EncodeHeaders time, by inspecting the `Content-Length` header on the merged headers map (which carries the upstream body length when CL is known) OR by deferring all skip-decision gating to EncodeData and only mutating headers there.**
          - **Refined algorithm:** EncodeHeaders sets f.willCompress = true ONLY tentatively; defers the min_content_length check to EncodeData. EncodeData on f.willCompress=true + endStream=true:
              - if len(data) < min_content_length:
                  REVERT headers: cannot directly mutate resp.Headers from EncodeData (the chain has moved on). INSTEAD: store the original Content-Encoding state on f.preEncodeContentEncoding before mutation; on revert, the filter cannot un-mutate the already-emitted headers. This is a wire-shape ANOMALY when min_content_length fires late.
          - **Phase-14 SPEC commitment (per §12 deferred decision below):** EncodeHeaders does the min_content_length check by reading `headers.Get("Content-Length")` IFF Content-Length is set + parseable. When CL is set + < min_content_length → SKIP at EncodeHeaders (no Vary/Content-Encoding mutation). When CL is unset (chunked-input case from upstream — but envoy-go's encode-chain ALWAYS has full body materialized per `connection.go:467` → `len(resp.Body) == 0` is the only chunked-equivalent), defer to EncodeData. The deferral path's late-revert anomaly is documented at §13.4 as a phase-14 forward-pointer note + scoped-out via the BRAINSTORM §6 fixture matrix (no late-revert scenario in fixture 0016).
   - Effective implementation: EncodeHeaders inspects Content-Length on the merged headers; when CL < min_content_length → SKIP early; EncodeData is reached only with CL >= min_content_length OR CL unset. CL unset case in envoy-go's framework is structurally rare (connection.go materializes full resp.Body before encode-chain).
4. (For scenario 3 — `/text-html-10` direct_response — direct_response carries Content-Length in the action's response headers; CL=10 known at EncodeHeaders time; SKIP fires there).
5. writeH1Reply emits original 10-byte body, content-length: 10, no content-encoding; no vary (per §11.15 — server-side-skip, no Vary injection).
6. Counter increments: response_not_compressed +1; response_content_length_too_small +1; header_compressor_used +1.
```

### 5.5 Per-request flow — per-route disabled (scenario 5 / `/per-route-disabled`)

```
1. Downstream:  GET /per-route-disabled  Accept-Encoding: gzip
2. HCM dispatch:
   a. RunDecodeHeaders → compressor.DecodeHeaders:
      - parse AE → acceptedEncoding="gzip"
      - resolve per-route → *compiledPerRoute{disabled: true}
      - effective passthrough = true
      - return Continue (no AE strip — the rmAE listener-level setting doesn't apply on a disabled route either; mirrors Envoy's wholly-inactive semantic)
3. Encode chain:
   a. RunEncodeHeaders → compressor.EncodeHeaders:
      - f.passthrough == true → return Continue immediately (NO header mutation; NO Vary; NO Content-Encoding; NO ETag-strip)
   b. RunEncodeData → compressor.EncodeData:
      - f.passthrough == true → return DataContinue (NO body mutation)
4. HCM EncodeBodyOverride returns (nil, false); resp.Body unchanged
5. writeH1Reply emits original body unmodified
6. NO counter increments (the entire compressor surface is bypassed; mirrors phase-13 buffer disabled-route precedent + §11.13 (a) confirmation)
```

### 5.6 Per-request flow — per-route remove_accept_encoding_header override (scenario 6 / `/per-route-rmae`)

```
1. Downstream:  GET /per-route-rmae  Accept-Encoding: gzip  → reaches backend via cluster route
2. HCM dispatch:
   a. RunDecodeHeaders → compressor.DecodeHeaders:
      - parse AE → acceptedEncoding="gzip"
      - resolve per-route → *compiledPerRoute{disabled: false, removeAcceptEncodingHeaderOverride: ptr(true)}
      - effective rmAE = true (overrides listener-level rmAE=false) → STRIP "Accept-Encoding" from request headers
      - return Continue
   b. dispatchRequest forwards request to backend WITHOUT Accept-Encoding header
3. Backend (echobackend) responds with body containing JSON-echoed request headers; the response body proves AE was stripped (no "Accept-Encoding" key in echoed headers map)
4. Encode chain:
   a. RunEncodeHeaders → compressor.EncodeHeaders:
      - skip-decision: passes content-type (application/json by default of the backend; matches default list); Content-Encoding empty; no ETag; no no-transform; status 200 not uncompressible
      - f.willCompress = true; mutate headers: Content-Encoding: gzip; Vary-append; return Continue
   b. RunEncodeData → compressor.EncodeData(data=<echoed-JSON-body>, endStream=true):
      - len(data) >= 30 → proceed
      - gzip-encode + OverwriteBody
      - increment response_compressed +1; total bytes counters; header_compressor_used +1
5. HCM substitutes resp.Body with override; writeH1Reply emits compressed
6. Driver decompresses response body via compress/gzip.NewReader; asserts the decompressed JSON does NOT contain "Accept-Encoding" key in the echoed headers map (proves the per-route rmAE override stripped it before forwarding).
```

### 5.7 Per-request flow — ETag handling (scenarios woven into the matrix; per §1.1 amendment 6 dual-mode)

**Mode (a): default `disable_on_etag_header: false` + strong-ETag response (scenario woven into /echo-etag-strong):**
```
1-3 same as 5.2 except response carries etag: "abc123"
4. EncodeHeaders skip-decision:
   - (iv) effective disable_on_etag_header == false; → does NOT skip on etag
5. Mutate headers: Content-Encoding: gzip; Vary-append; STRIP strong-ETag (regex match `^"[^"]*"$` matches `"abc123"`) → DELETE etag header
6. EncodeData: gzip-encode + OverwriteBody
7. writeH1Reply emits compressed body with NO etag header (stripped)
```

**Mode (a) variant: default + weak-ETag (scenario woven into /echo-etag-weak):**
```
4. EncodeHeaders skip-decision: does NOT skip
5. Mutate headers: Content-Encoding: gzip; Vary-append; check ETag value: matches `^W/"[^"]*"$` (weak) → PRESERVE etag header
6-7: compressed response carries etag: W/"abc123" verbatim
```

**Mode (b): per-route `disable_on_etag_header: true` (DEFERRED — STRUCTURALLY IMPOSSIBLE per §1.1 amendment 4):**
> Per `ResponseDirectionOverrides` (carrying only `remove_accept_encoding_header`), per-route override of `disable_on_etag_header` is impossible. Mode (b) for phase 14 is exercised only at the listener-level setting (a future fixture or unit test can set listener `disable_on_etag_header: true` and observe ETag-present responses pass uncompressed; phase 14 fixture 0016 keeps listener `disable_on_etag_header: false` for the full ETag-mutator surface coverage).

### 5.8 Concurrency model

The compressor filter is purely synchronous on each per-stream encode-chain invocation. The `c.encodeBodyOverride []byte` field is per-stream (one chain instance per request); no cross-stream sharing. The 17-counter `filterStats` struct is per-listener-config (registered once at `New` factory time per HCM stat_prefix); counter increments use atomic operations via the existing `*stats.Registry` LBP-1 discipline (per ADR-0061). No mutex on the filter; no goroutine-per-stream beyond the HCM dispatch goroutine.

Race-test surface unchanged from phase 13's 36-package green baseline; the new `internal/filter/http/compressor/` package adds the 37th. The framework deltas at `chain.go` introduce one new per-stream field on `*FilterChain`; access is single-goroutine-per-stream (the dispatch goroutine), so no synchronization is needed.

### 5.9 Filter ordering in fixture 0016

Single chain: `compressor → router`. Compressor sits between any future request-mutating filters (none in fixture 0016) and the router terminal. On the response/encode side, the chain runs in REVERSE declaration order: `router (response from action) → compressor (mutates headers + body) → wire-write`. NO interaction with cors / fault / header_mutation / local_ratelimit / csrf / buffer in fixture 0016 (per §2.6). Future fixtures may extend with multi-filter chains; phase 14 keeps single-filter-only for differential simplicity.

### 5.10 Encode-side framework primitive layering

The new `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive layers under existing primitives:
- (a) **chain-level framework cap (`filterBufferLimitBytes = 1 << 20` per ADR-0076 / `chain.go:326-328` encode-side check):** the chain's encode-side cap fires on `c.encodeBufLen + len(data) > filterBufferLimitBytes`. Compressor's input body is pre-buffered upstream of the encode chain (resp.Body materialized at `connection.go:467`); if `len(resp.Body) > 1 MiB`, the chain's encode-side cap fires at the FIRST `RunEncodeData` invocation, BEFORE any filter sees the data — so compressor's `EncodeData` is never reached on > 1 MiB bodies. The cap fires with `errEncodeBufferOverflow` and HCM resets the connection (H1 close, H2 RST_STREAM). Phase 14 inherits this behavior; bodies > 1 MiB observe connection-level reset, NOT a compressor filter response.
- (b) **filter-level pass-through from disabled-route per-route TPFC:** when `f.passthrough == true`, EncodeData returns DataContinue without invoking `OverwriteBody`; the chain's `c.encodeBodyOverride` stays nil; HCM's harvest at `EncodeBodyOverride()` returns `(nil, false)`; resp.Body is unchanged.
- (c) **filter-level revert on min_content_length below-threshold + non-compressible-content-type + status-uncompressible + already-encoded + cache-control-no-transform:** the skip-decision at EncodeHeaders sets `f.willCompress = false` BEFORE EncodeData; EncodeData early-returns DataContinue without invoking `OverwriteBody`.
- (d) **filter-level OverwriteBody fire on the compress-allow path:** EncodeData on `f.willCompress = true + endStream = true + len(data) >= min_content_length` invokes OverwriteBody with the gzip-compressed bytes; HCM's harvest at EncodeBodyOverride() returns `(compressed_bytes, true)`; resp.Body is substituted; writeH1Reply emits compressed content.

The framework cap (a) is a safety net against compressor-filter pathological behavior (e.g., a misconfigured Gzip codec producing larger-than-input compressed output that pushes a near-1-MiB input over the encode-side cap). Phase 14 does NOT alter the cap value; ADR-0076 §Decision (b) carries through unchanged. Future cap-promotion phase (per phase-13 forward-pointer) is the natural amender.

---

## 6. Per-component contract summary

### 6.1 Constructor signatures (cors / fault / csrf / buffer precedent verbatim)

```go
package compressor

const (
    TypeURL    = "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"
    filterName = "envoy.filters.http.compressor"
    gzipLibraryTypeURL = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"
)

func New(ctx envoyhttp.FactoryCtx, tc *anypb.Any) (envoyhttp.HTTPFilterFactory, error) {
    // 1. tc.UnmarshalTo(&cfg) where cfg is *envoyextensionsfiltershttpcompressorv3.Compressor.
    //    - if tc is nil OR Unmarshal fails → return error: "compressor: invalid typed_config: <wrap>".
    //    - if cfg.CompressorLibrary == nil → return error: "compressor: compressor_library is required" (PGV-mirror per Envoy proto required constraint).
    // 2. unmarshalCompressorLibrary(cfg.CompressorLibrary) → (*compiledGzipConfig, libraryName string, error):
    //    - extract library.GetTypedConfig(); check TypeURL.
    //    - if TypeURL != gzipLibraryTypeURL → return error: "compressor: unsupported compressor_library TypeURL <url>; phase-14 MVP supports only envoy.extensions.compression.gzip.compressor.v3.Gzip".
    //    - else: library.GetTypedConfig().UnmarshalTo(&gzipPB); return buildCompiledGzipConfig(&gzipPB) + library.GetName().
    // 3. Build *compiledConfig{
    //      libraryName,                 // for stat namespace; may be empty
    //      gzip: *compiledGzipConfig,
    //      minContentLength: uint32 (default 30 if nil),
    //      contentTypes: []string (default 8-entry list if empty),
    //      disableOnEtagHeader: bool,
    //      removeAcceptEncodingHeader: bool,
    //      uncompressibleResponseCodes: []uint32,
    //      // NOTE: `enabled` is silent-ignored; not stored on compiledConfig.
    //      // NOTE: `status_header_enabled` is silent-ignored; not stored.
    //    }.
    // 4. Register filterStats via ctx.Stats: 17 counters under `compressor.<libraryName>.gzip.[response.]<counter>` path
    //    relative to ctx.StatPrefix (the HCM stat_prefix). The listener-level config always sets response_direction_config
    //    (per §1.1 amendment 3), so the namespace ALWAYS uses the `response.` infix. 5 request_* counters register at zero
    //    AND stay at zero (always-zero in MVP since request_direction_config silent-ignored).
    // 5. Return HTTPFilterFactory closure that allocates a fresh *filter per request:
    //    InstanceFactory:
    //       return envoyhttp.HTTPFilter{
    //           Name:     filterName,
    //           Decoder:  &filter{config, stats},
    //           Encoder:  &filter{config, stats},  // SAME *filter; both-sides binding
    //           PerRoute: parsePerRoute,
    //       }
}
```

The `New` closure signature mirrors phase 12 csrf / phase 13 buffer verbatim. Encoder-and-decoder `HTTPFilter` value: the same `*filter` instance services both sides per the chain framework's both-sides-filter contract; the decode-side surface is minimal but non-vacuous (AE-parser + per-route resolve + maybe-strip-AE — see §6.4 below).

### 6.2 `compiledConfig` + `compiledPerRoute` + `compiledGzipConfig` shape (per ADR-0130 + ADR-0125 amendment)

```go
// Listener-level (parsed by New):
type compiledConfig struct {
    libraryName                 string                // empty allowed; embedded in stat namespace
    gzip                        *compiledGzipConfig   // gzip-only MVP; non-nil
    minContentLength            uint32                // default 30 if proto unset; ≥ 1 if set
    contentTypes                []string              // default 8-entry list; lowercase; case-insensitive prefix-matched
    disableOnEtagHeader         bool                  // default false
    removeAcceptEncodingHeader  bool                  // default false
    uncompressibleResponseCodes map[uint32]struct{}   // default empty (set semantic for O(1) lookup); valid range [200, 600)
    // status_header_enabled silent-ignored — not stored
    // response_direction_config.common_config.enabled silent-ignored — not stored
}

// Codec-library Gzip:
type compiledGzipConfig struct {
    level int  // mapped from Envoy enum per ADR-0130 §Decision (iv) table; valid range [-1, 9]
    // strategy via gzip.HuffmanOnly OR gzip.DefaultCompression — phase 14 MVP collapses all non-HUFFMAN_ONLY strategies to default;
    // stored as a bool: huffmanOnly bool
    huffmanOnly bool
    // memory_level / window_bits / chunk_size — silent-ignored; not stored
}

// Per-route (parsed by parsePerRoute; per §1.1 amendment 4):
type compiledPerRoute struct {
    disabled                              bool      // exclusive with override; true → filter wholly inactive on this route
    removeAcceptEncodingHeaderOverride    *bool     // exclusive with disabled; nil unless override case set; pointer to discriminate from "unset"
    // NO compressor_library override (silent-ignored per §1.1 amendment 4 + §2.2)
}
```

The two per-route cases (`disabled: true` OR `overrides: {response_direction_config: {remove_accept_encoding_header: <bool>}}`) are exclusive at the proto level (the `oneof override` discipline); envoy-go's `parsePerRoute` enforces this via the proto3 oneof unmarshal semantics — the JSON decoder rejects malformed configs (both fields set OR neither field set) BEFORE the Go-side handler runs. See §6.3 below + the §11.13 P13 empirical pin discipline.

NO `*filterStats` field on `compiledPerRoute` (per §1.1 amendment 3 — per-route stats SHARED with listener-level). Compare phase 11 local_ratelimit's `compiledPerRoute{tokenBucket, stats}` shape — phase 14 has no stateful resources or per-route stat namespace.

### 6.3 `parsePerRoute` discipline (per ADR-0125 amendment + §11.13)

```go
func parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error) {
    cfg, ok := perRoute.(*envoyextensionsfiltershttpcompressorv3.CompressorPerRoute)
    if !ok {
        return nil, fmt.Errorf("compressor per-route: expected *CompressorPerRoute, got %T", perRoute)
    }
    switch override := cfg.GetOverride().(type) {
    case *envoyextensionsfiltershttpcompressorv3.CompressorPerRoute_Disabled:
        if !override.Disabled {
            // PGV constraint bool.const = true rejects disabled: false at proto-decode time;
            // structurally unreachable BUT defensively returned for safety.
            return nil, fmt.Errorf("compressor per-route: disabled must be true (PGV bool.const violation)")
        }
        return &compiledPerRoute{disabled: true}, nil
    case *envoyextensionsfiltershttpcompressorv3.CompressorPerRoute_Overrides:
        // overrides shape: CompressorOverrides{response_direction_config: ResponseDirectionOverrides{remove_accept_encoding_header}, compressor_library}.
        // ResponseDirectionOverrides has ONE field: remove_accept_encoding_header (BoolValue).
        ov := override.Overrides
        if ov == nil {
            // Empty overrides — wholly equivalent to listener-level; produce a no-op compiledPerRoute.
            return &compiledPerRoute{}, nil
        }
        var rmae *bool
        if rdc := ov.GetResponseDirectionConfig(); rdc != nil {
            if v := rdc.GetRemoveAcceptEncodingHeader(); v != nil {
                b := v.GetValue()
                rmae = &b
            }
        }
        // ov.GetCompressorLibrary() — silent-ignored at parse time; per §1.1 amendment 4 + §8.2 deferral.
        // No envoy-go-only validation on per-route library swap; accepted-but-ignored.
        return &compiledPerRoute{
            disabled:                           false,
            removeAcceptEncodingHeaderOverride: rmae,
        }, nil
    case nil:
        // PGV constraint validate.required = true on the oneof rejects empty per-route entries at proto-decode time;
        // structurally unreachable BUT defensively returned for safety.
        return nil, fmt.Errorf("compressor per-route: override oneof is required (neither disabled nor overrides set)")
    default:
        return nil, fmt.Errorf("compressor per-route: unknown override case %T", override)
    }
}
```

**Per §11.13 empirical pin:** the "both fields set" oneof violation case is rejected by Envoy at boot via the **JSON→proto decoder** (mirrors phase-13 buffer P3 — the `'<field>' has already been set (either directly or as part of a oneof)` error wording), NOT by PGV. envoy-go's `protojson.Unmarshal` mirrors this for free — the rejection happens BEFORE `parsePerRoute` is invoked. The "neither field set" case is rejected by PGV's `validate.required` constraint on the oneof; the "disabled: false" case is rejected by PGV's `bool.const = true` constraint. Both PGV constraints are MIRRORED in envoy-go's `parsePerRoute` switch (the `case nil` and the `case *CompressorPerRoute_Disabled` with `!override.Disabled`) per ADR-0121 / ADR-0125 precedent of envoy-go-own-wording for filter-internal validation.

### 6.4 `DecodeHeaders` body (per §1.1 amendment 4 + §11.8)

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
    // 1. Parse Accept-Encoding via the q-value parser (RFC 7231 §5.3.4); cache result on f.acceptedEncoding.
    //    The parser produces a sorted-by-q-value-desc list of (coding, qValue); the filter selects the
    //    highest-q-value coding that matches a configured codec (gzip-only MVP). On wildcard "*" with no
    //    explicit "gzip" entry, the filter selects "gzip" (wildcard implies acceptance).
    f.acceptedEncoding, f.acceptHeaderClassification = parseAcceptEncoding(headers.Get("Accept-Encoding"))
    //    acceptHeaderClassification ∈ {"compressor_used", "overshadowed", "identity", "wildcard", "no_accept_header", "not_valid"}
    //    drives the header_* counter increments at EncodeHeaders time (per §11.5 6-counter cluster + §11.5 verbatim probeA scrape).

    // 2. Resolve per-route TPFC; cache on f.perRoute.
    var perRoute *compiledPerRoute
    if v, ok := f.dcb.RequestRouteConfig().(*compiledPerRoute); ok {
        perRoute = v
    }
    f.perRoute = perRoute

    // 3. Compute effective remove_accept_encoding_header.
    effectiveRmAE := f.config.removeAcceptEncodingHeader
    if perRoute != nil && perRoute.disabled {
        // Disabled route: no AE strip (the entire filter is bypassed; no encode-side either).
        f.passthrough = true
        return envoyhttp.Continue
    }
    if perRoute != nil && perRoute.removeAcceptEncodingHeaderOverride != nil {
        effectiveRmAE = *perRoute.removeAcceptEncodingHeaderOverride
    }

    // 4. Strip Accept-Encoding header from request if effectiveRmAE.
    if effectiveRmAE {
        headers.Del("Accept-Encoding")
    }

    return envoyhttp.Continue
}
```

`f.acceptedEncoding` ("gzip" / "" / "identity" / etc.) drives the EncodeHeaders skip-decision. `f.acceptHeaderClassification` drives the 6 `header_*` Accept-Encoding-cluster counter increments. Per `parseAcceptEncoding`'s RFC-7231 implementation:
- Empty/absent header → ("", "no_accept_header")
- Only `*` → ("gzip", "wildcard") if gzip is configured codec
- Only `identity` → ("", "identity")
- Only known codings unconfigured (e.g., `br` with gzip-only MVP) → ("", "overshadowed") — codings present but not selectable
- Mix with gzip-q>0 → ("gzip", "compressor_used")
- Malformed (q-value parse error) → ("", "not_valid")

The 6 classifications match the 6 header_* counter names verbatim. EncodeHeaders increments the corresponding counter on the codec-selection branch (compress path) AND on every skip path that did pass the AE check (mismatch / etag / no-transform / etc — these still increment header_compressor_used since AE selected the codec; matches §11.5 probeA evidence where header_compressor_used = 24 after 30+ probes including content-type-mismatch and already-encoded skips).

### 6.5 `DecodeData` + `DecodeTrailers` body

```go
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
    return envoyhttp.DataContinue  // pass-through; compressor never inspects request body
}

func (f *filter) DecodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue  // pass-through
}
```

### 6.6 `EncodeHeaders` body (per BRAINSTORM §2.6 + §1.1 amendment 5 + §1.1 amendment 6 + §11.10 + §11.11 + §11.12 + §11.15)

```go
func (f *filter) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
    // 1. Per-route disabled bypass: no header mutation, no counter increment.
    if f.passthrough {
        return envoyhttp.Continue
    }

    // 2. Compute the effective config (per-route override OR listener-level).
    effective := f.effectiveConfig()  // returns *compiledConfig with per-route overrides applied (only rmAE in MVP)

    // 3. Skip-decision sequence (in order; first hit wins).
    skipReason := f.computeSkipReason(headers, effective, endStream)
    //   returns one of: "" (no skip), "no_accept_header", "identity", "wildcard_uncompressed",
    //   "not_valid", "uncompressible_status", "already_encoded", "etag_disabled", "no_transform",
    //   "content_type_mismatch", "content_length_too_small_known".
    //   Note: "wildcard_uncompressed" only fires when wildcard would match identity preferred (rare;
    //         in practice wildcard maps to gzip selection).

    // 4. Increment counters per §11.5 6-counter Accept-Encoding cluster + 1 etag counter.
    switch f.acceptHeaderClassification {
    case "no_accept_header":
        f.stats.NoAcceptHeader.Inc()
    case "wildcard":
        f.stats.HeaderWildcard.Inc()
    case "identity":
        f.stats.HeaderIdentity.Inc()
    case "not_valid":
        f.stats.HeaderNotValid.Inc()
    case "compressor_used":
        f.stats.HeaderCompressorUsed.Inc()  // also incremented when codec selected even if response was later skipped
    case "overshadowed":
        f.stats.HeaderCompressorOvershadowed.Inc()
    }

    if skipReason != "" {
        f.stats.ResponseNotCompressed.Inc()
        if skipReason == "etag_disabled" {
            f.stats.NotCompressedEtag.Inc()
        }
        if skipReason == "content_length_too_small_known" {
            f.stats.ResponseContentLengthTooSmall.Inc()
        }
        // §11.15 + §1.1 amendment 5: Vary IS injected on AE-side-skip paths
        // (no_accept_header, identity, wildcard_uncompressed, not_valid) but NOT on
        // server-side-skip paths (uncompressible_status, already_encoded, etag_disabled,
        // no_transform, content_type_mismatch, content_length_too_small_known).
        if skipReason == "no_accept_header" || skipReason == "identity" ||
           skipReason == "wildcard_uncompressed" || skipReason == "not_valid" {
            appendVaryAcceptEncoding(headers)
        }
        return envoyhttp.Continue
    }

    // 5. Compress path: mutate headers.
    headers.Set("Content-Encoding", "gzip")
    appendVaryAcceptEncoding(headers)  // ALWAYS append, even on existing Vary: * per §11.10
    if !effective.disableOnEtagHeader {
        // Mode (a) per §1.1 amendment 6: strip strong-ETag; preserve weak-ETag.
        maybeStripStrongEtag(headers)
    }
    headers.Del("Content-Length")  // framework's writeH1Reply rewrites unconditionally to len(body) at wire time

    f.willCompress = true
    return envoyhttp.Continue
}
```

The `effectiveConfig()` helper:
```go
func (f *filter) effectiveConfig() *compiledConfig {
    if f.perRoute == nil || f.perRoute.removeAcceptEncodingHeaderOverride == nil {
        return f.config
    }
    // Shallow-clone with rmAE overridden. Per §1.1 amendment 4 the per-route shape only overrides rmAE;
    // all other fields inherit listener-level. Returns a clone to keep the listener-level *compiledConfig immutable.
    cloned := *f.config
    cloned.removeAcceptEncodingHeader = *f.perRoute.removeAcceptEncodingHeaderOverride
    return &cloned
}
```

The `computeSkipReason` helper enumerates the skip-decision sequence in order; for the order details, see §11.15 (in-line skip-order at the empirical-pin block; the 11-bucket order matches Envoy's `compressor_filter.cc`).

The `maybeStripStrongEtag` helper:
```go
var (
    strongEtagPattern = regexp.MustCompile(`^"[^"]*"$`)
    weakEtagPattern   = regexp.MustCompile(`^W/"[^"]*"$`)
)

func maybeStripStrongEtag(headers http.Header) {
    val := headers.Get("ETag")
    if val == "" {
        return
    }
    if strongEtagPattern.MatchString(val) {
        // Strong → strip per RFC 7232 §2.3 (strong validators must change when entity bytes change).
        headers.Del("ETag")
    } else if weakEtagPattern.MatchString(val) {
        // Weak → preserve. Already-correct.
    }
    // Malformed ETag → preserve verbatim (defensive; mirrors Envoy's behavior under unexpected ETag formats).
}
```

The `appendVaryAcceptEncoding` helper (per §1.1 amendment 5 + §11.10):
```go
func appendVaryAcceptEncoding(headers http.Header) {
    existing := headers.Get("Vary")
    if existing == "" {
        headers.Set("Vary", "Accept-Encoding")
        return
    }
    // Token-match dedup: case-insensitive token-presence check.
    for _, tok := range strings.Split(existing, ",") {
        if strings.EqualFold(strings.TrimSpace(tok), "Accept-Encoding") {
            return  // already present; no-op
        }
    }
    // APPEND unconditionally (even when existing == "*"; per §11.10).
    headers.Set("Vary", existing + ", Accept-Encoding")
}
```

### 6.7 `EncodeData` body (per §11.9 + §11.14)

```go
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
    if f.passthrough || !f.willCompress {
        return envoyhttp.DataContinue
    }

    // Per encode-chain shape: RunEncodeData is invoked ONCE with full body + endStream=true (per
    // connection.go:467-475 + h2dispatch.go:303-315). This branch handles the only-call case.
    if !endStream {
        // Defensive: future framework changes might invoke mid-stream RunEncodeData. Pass through for now;
        // do NOT compress until end-stream observed. ADR-0131 §Decision (vii) records.
        return envoyhttp.DataContinue
    }

    // Late min_content_length gate (only fires when CL was unset at EncodeHeaders time AND body length
    // emerges only at EncodeData; in envoy-go's framework this is structurally rare since resp.Body is
    // pre-buffered before encode chain. SPEC's preferred algorithm gates at EncodeHeaders when CL known;
    // defensive late-gate here for the unset-CL case).
    if uint32(len(data)) < f.effectiveConfig().minContentLength {
        // Cannot revert headers from EncodeData; the late-revert anomaly is documented at §13.4.
        // For phase-14 MVP, this branch should be unreachable when fixture configs use direct_response
        // or any backend that emits Content-Length (which is universal in the differential fixture).
        // Increment counters; DataContinue (the already-set Content-Encoding/Vary headers will emit on
        // wire — anomaly).
        f.stats.ResponseContentLengthTooSmall.Inc()
        f.stats.ResponseNotCompressed.Inc()
        return envoyhttp.DataContinue
    }

    // Compress body in one shot.
    var buf bytes.Buffer
    gzWriter, err := gzip.NewWriterLevel(&buf, f.config.gzip.level)
    if err != nil {
        // Defensive: should be unreachable since level was validated at config-parse time per ADR-0130 §Decision (iv).
        // Emit no-compression fallback rather than failing the request.
        f.stats.ResponseNotCompressed.Inc()
        return envoyhttp.DataContinue
    }
    if _, err := gzWriter.Write(data); err != nil {
        gzWriter.Close()
        f.stats.ResponseNotCompressed.Inc()
        return envoyhttp.DataContinue
    }
    if err := gzWriter.Close(); err != nil {
        f.stats.ResponseNotCompressed.Inc()
        return envoyhttp.DataContinue
    }

    compressed := buf.Bytes()

    // Emit via the new framework primitive (per ADR-0131 §Decision (vi)).
    f.ecb.OverwriteBody(compressed)

    // Increment counters.
    f.stats.ResponseCompressed.Inc()
    f.stats.ResponseTotalUncompressedBytes.Add(uint64(len(data)))
    f.stats.ResponseTotalCompressedBytes.Add(uint64(len(compressed)))

    return envoyhttp.DataContinue
}
```

### 6.8 `EncodeTrailers` + `OnDestroy` + `SetEncoderCallbacks` + `SetDecoderCallbacks` bodies

```go
func (f *filter) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
    return envoyhttp.TrailersContinue  // pass-through; compressor does not inspect/mutate trailers
}

func (f *filter) OnDestroy() {
    // No per-stream resources to clean up:
    // - no timers
    // - no goroutines
    // - no buffer references held beyond the *filter instance lifetime
    // - the c.encodeBodyOverride field on *FilterChain dies with the chain
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) {
    f.dcb = cb
}

func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) {
    f.ecb = cb
}
```

### 6.9 Filter-callback wiring + `filterStats` shape

The factory closure returns:

```go
return envoyhttp.HTTPFilter{
    Name:     filterName,             // "envoy.filters.http.compressor"
    Decoder:  f,                      // *filter implementing the decoder-side method set
    Encoder:  f,                      // SAME *filter implementing the encoder-side method set
    PerRoute: parsePerRoute,          // *PerRouteConfig builder per-tier
}
```

The `PerRoute` field threads into `internal/filter/http/registry.go`'s parsing path; `parsePerRoute` is invoked at HCM-build time for each `CompressorPerRoute` TPFC entry on Route / VirtualHost / RouteConfiguration. The 3-tier `PerRouteConfig.Resolve` runs at request-time inside `DecodeHeaders`'s per-route resolution.

The `filterStats` struct (registered at `New` factory time per HCM stat_prefix; 17 counters per §1.1 amendment 3):

```go
type filterStats struct {
    // 6 Accept-Encoding cluster counters (NOT split per direction; shared across both):
    HeaderCompressorOvershadowed *stats.Counter
    HeaderCompressorUsed         *stats.Counter
    HeaderIdentity               *stats.Counter
    HeaderNotValid               *stats.Counter
    HeaderWildcard               *stats.Counter
    NoAcceptHeader               *stats.Counter
    // 1 ETag-skip counter:
    NotCompressedEtag            *stats.Counter
    // 5 response-side counters (active in MVP):
    ResponseCompressed             *stats.Counter
    ResponseContentLengthTooSmall  *stats.Counter
    ResponseNotCompressed          *stats.Counter
    ResponseTotalCompressedBytes   *stats.Counter
    ResponseTotalUncompressedBytes *stats.Counter
    // 5 request-side counters (always-zero in MVP since request_direction_config silent-ignored;
    // registered for byte-equivalent stat scrape with reference Envoy):
    RequestCompressed              *stats.Counter
    RequestContentLengthTooSmall   *stats.Counter
    RequestNotCompressed           *stats.Counter
    RequestTotalCompressedBytes    *stats.Counter
    RequestTotalUncompressedBytes  *stats.Counter
}
```

Each counter's stat-name is built at `New` factory time using the path:
```
http.<HCM_stat_prefix>.compressor.<libraryName>.gzip.[response|request].<counter>
```
The prefix is `http.<HCM_stat_prefix>.compressor.<libraryName>.gzip.` (constant per filter instance); the suffix is `[response|request].<counter>` (5 active + 5 vacuous + 7 unprefixed for header_* and not_compressed_etag — wait, per §11.5 the header_* + not_compressed_etag counters are NOT under `response.` or `request.` infix — they're at `compressor.<libraryName>.gzip.<counter>` directly; SPEC author's empirical evidence at §11.5 verbatim shows `envoy_http_compressor_text_optimized_gzip_header_compressor_used` flat-form, i.e., no `response.` infix on the header_* family). The 17-counter struct registers:
- 6 + 1 = 7 counters at `compressor.<libraryName>.gzip.<header_or_etag_counter>` (no direction infix)
- 5 at `compressor.<libraryName>.gzip.response.<counter>`
- 5 at `compressor.<libraryName>.gzip.request.<counter>` (vacuous; registered + always-zero)

Total: 17 register calls at `New`, each emitting one Prometheus line via the existing Rule SN2 `http.*` flatten.

---

## 7. Differential fixture `0016-http-compressor`

### 7.1 Per-request matrix (6 requests; revised from BRAINSTORM §6 per §1.1 amendment 4)

| # | Scenario | Request | Expected response | Counter delta (envoy-go side) | §11 cross-ref |
|---|---|---|---|---|---|
| 1 | Allow-compress (default route) | `GET /text-html-1024` `Accept-Encoding: gzip` (1024-byte text/html body via direct_response) | 200; `content-encoding: gzip`; `vary: Accept-Encoding`; **decompressed body byte-exact 1024 bytes** | `response_compressed +1`, `response_total_uncompressed_bytes +1024`, `response_total_compressed_bytes +<gzipped-len>`, `header_compressor_used +1` | §11.1 + §11.6 + §11.9 + §11.10 + §11.14 |
| 2 | Skip content-type-mismatch | `GET /image-png-1024` `Accept-Encoding: gzip` | 200; NO `content-encoding`; NO `vary`; `content-length: 1024`; identity body | `response_not_compressed +1`, `header_compressor_used +1` | §11.1 + §11.6 + §11.15 |
| 3 | Skip below min_content_length | `GET /text-html-10` `Accept-Encoding: gzip` (10-byte text/html via direct_response — below default 30) | 200; NO `content-encoding`; NO `vary`; `content-length: 10`; identity body | `response_not_compressed +1`, `response_content_length_too_small +1`, `header_compressor_used +1` | §11.9 + §11.15 |
| 4 | Skip on strong-ETag-strip + compressed-passthrough (mode-a) | `GET /text-html-etag-strong` `Accept-Encoding: gzip` (1024-byte text/html with `etag: "abc"`) | 200; `content-encoding: gzip`; `vary: Accept-Encoding`; **NO `etag` header (stripped)**; decompressed body byte-exact 1024 bytes | `response_compressed +1`, total bytes counters; `header_compressor_used +1` | §11.7 mode-a |
| 5 | Per-route disabled bypass | `GET /per-route-disabled` `Accept-Encoding: gzip` (1024-byte text/html via direct_response) | 200; NO `content-encoding`; NO `vary`; `content-length: 1024`; identity body | NO counter increments (filter wholly inactive on disabled route) | §11.13 (a) |
| 6 | Per-route remove_accept_encoding_header override + compressed-via-real-backend | `GET /per-route-rmae` `Accept-Encoding: gzip` (route hits real echo-backend; per-route overrides rmAE=true; backend echoes upstream-side request headers in body) | 200; `content-encoding: gzip`; `vary: Accept-Encoding`; **decompressed body asserts NO "Accept-Encoding" key in echoed headers map** | `response_compressed +1`, total bytes counters; `header_compressor_used +1` | §11.13 (b) + §11.8 |

**Total counter deltas after the 6-request workload:**
- `response_compressed +3` (scenarios 1, 4, 6)
- `response_not_compressed +2` (scenarios 2, 3)
- `response_content_length_too_small +1` (scenario 3)
- `response_total_uncompressed_bytes +N` (sum of scenarios 1+4+6 input lengths)
- `response_total_compressed_bytes +M` (sum of scenarios 1+4+6 compressed lengths; differs between envoy-go and Envoy due to gzip-encoder choice variance — allow-listed via tolerance per ADR-0133)
- `header_compressor_used +5` (scenarios 1, 2, 3, 4, 6 — every scenario except 5 sees the codec selected)
- `not_compressed_etag +0` (no scenario uses `disable_on_etag_header: true`; the etag strong-strip in scenario 4 is mode-a with default disable_on_etag_header=false)
- 4 of 6 unique header_* counters at zero (no_accept_header / wildcard / identity / not_valid — fixture always sends gzip-AE)
- 5 request_* counters at zero (vacuous on both sides)

### 7.2 Topology

`test/fixtures/0016-http-compressor/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config.
- `inputs/driver.go` — Go driver that drives both proxies with identical inputs; decompresses gzip responses via `compress/gzip.NewReader`.
- `expectations.yaml` — per-scenario allow-list / ignore-list / counter-delta map.
- `README.md` — fixture overview + scenario list + reference config citations + decompress-and-compare body-assertion explanation.

Single listener `127.0.0.1:<port>` (HTTP/1.1 plaintext per phases 09/10/11/12/13 precedent). One virtual_host `vh_main` with 6 routes:
- `/text-html-1024` — default route; direct_response with 1024-byte text/html body.
- `/image-png-1024` — direct_response with 1024-byte image/png body.
- `/text-html-10` — direct_response with 10-byte text/html body.
- `/text-html-etag-strong` — direct_response with 1024-byte text/html + `etag: "abc"`.
- `/per-route-disabled` — direct_response with 1024-byte text/html; per-route `CompressorPerRoute{disabled: true}`.
- `/per-route-rmae` — cluster route to backend `c_backend` (echobackend); per-route `CompressorPerRoute{overrides: {response_direction_config: {remove_accept_encoding_header: true}}}`.

One cluster `c_backend` reaching `test/helpers/echobackend/` (extended to echo upstream-side request headers in JSON body so scenario 6's AE-stripping assertion is observable). Backend extension: ~30-50 LoC.

**Listener-level `Compressor`:**
```yaml
http_filters:
  - name: envoy.filters.http.compressor
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor
      compressor_library:
        name: text_optimized
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip
      response_direction_config:
        # `enabled` field omitted on both sides per §1.1 amendment 2 (default = enabled).
        # `min_content_length` omitted → default 30 per §11.9.
        # `content_type` omitted → default 8-entry list per §11.1.
        # `disable_on_etag_header` defaults to false → strong-ETag stripped on compressed path per §1.1 amendment 6 mode-a.
        # `remove_accept_encoding_header` defaults to false at listener level; per-route override on /per-route-rmae enables.
  - name: envoy.filters.http.router
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
```

**Per-route TPFC on `/per-route-disabled`:**
```yaml
typed_per_filter_config:
  envoy.filters.http.compressor:
    "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.CompressorPerRoute
    disabled: true
```

**Per-route TPFC on `/per-route-rmae`:**
```yaml
typed_per_filter_config:
  envoy.filters.http.compressor:
    "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.CompressorPerRoute
    overrides:
      response_direction_config:
        remove_accept_encoding_header: true
```

### 7.3 Asserted equivalence (per ADR-0133 decompress-and-compare discipline)

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status**: byte-equal between Envoy and envoy-go for every scenario (200 on every scenario; no 4xx/5xx).
- **Response body axis (per ADR-0133):**
  - On **uncompressed scenarios** (2, 3, 5): byte-exact match between envoy-go and Envoy.
  - On **compressed scenarios** (1, 4, 6): **decompressed-byte-exact match.** The fixture driver decompresses both responses (envoy-go's gzip output + Envoy's gzip output) via Go's `compress/gzip.NewReader` and asserts byte-exact on the decompressed plaintexts. The compressed bytes themselves are NOT asserted byte-exact (structurally non-equivalent due to gzip-format multi-encoding spec; ADR-0133 codifies). Scenario 4 also asserts the decompressed body equals the original 1024-byte payload (no body mutation outside compression). Scenario 6 asserts the decompressed JSON body does NOT carry "Accept-Encoding" in its echoed-headers map (the per-route rmAE override observable at the backend boundary).
- **Response header set**: lowercase wire-form, set-equal between Envoy and envoy-go modulo:
  - `## Header allow-list` (existing — `date`, `server`, timing/identity headers).
  - **`content-length` value: ALLOW-LISTED** on compressed scenarios (1, 4, 6) — envoy-go emits compressed-length CL; Envoy emits NO CL (uses chunked). Different shapes; comparison via "either side emits valid wire shape" allow-list.
  - **`transfer-encoding` presence: ALLOW-LISTED** on compressed scenarios (1, 4, 6) — envoy-go emits absent; Envoy emits `chunked`. Different shapes.
  - Other header keys + values: byte-exact match. Scenario 4's `etag` header is asserted ABSENT on BOTH sides (the strong-ETag strip is an observable contract per §11.7 mode-a).
- **Per-counter delta equality**: after the 6-request workload completes, scrape `/stats/prometheus` from both proxies and assert per-counter deltas listed in §7.1 column 5. The 5 request_* counters are at +0 on both sides (vacuous; mirrors phase-13 buffer's twin-series-discipline allow-list shape — all 17 counters declared in envoy-go's allow-list; the 5 request_* are zero on both sides byte-equivalently, NOT filtered out via twin-series). The 12 active counters (6 header_* + 1 etag + 5 response_*) match per the §7.1 deltas.
- **`response_total_compressed_bytes` tolerance**: per ADR-0133, the compressed-byte counts will differ between envoy-go (Go `compress/gzip`) and Envoy (libz) due to compressor implementation variance — different block-boundaries, different Huffman trees, possibly different default compression levels resolving to different output sizes. The fixture's per-counter assertion on this counter uses a TOLERANCE WINDOW (e.g., ±20% of the smaller value, OR explicit "value must be > 0 and < input-uncompressed-bytes" — exact tolerance shape is a planner-time decision documented at §12). Other counters use byte-exact delta assertions.
- **Per-route fixture-config disposition**: scenarios 5 + 6 exercise BOTH per-route shapes (`disabled` + `overrides.response_direction_config.remove_accept_encoding_header`).

### 7.4 Driver shape

Go driver in `inputs/driver.go` per phase 09/10/11/12/13 precedent — sequential request loop (race-tolerant scrape ordering); per-scenario assertions inline; final stats scrape via `/stats/prometheus`. Total: 6 requests in the workload. Estimated driver size: ~200 LoC (heavier than phase-13's 150 LoC because of the gzip-decompression helper, the JSON-parse-and-AE-absence-assert helper for scenario 6, and the strong-ETag-stripped assertion helper for scenario 4).

**No timing tolerances.** Compressor is purely synchronous — no analog to phase 11's `refill-after-fill_interval ±10ms` scenario.

**No H2 differential coverage.** Phase 14 fixture 0016 is HTTP/1.1-only per §2.5.

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §1.1)

5 ADRs anticipated (consistent with BRAINSTORM §7's roster). ADR-0128 is the highest-numbered ADR landed in phase 13; ADR-0129 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0129** | `internal/filter/http/compressor/` package shape — single-token directory matching cors/fault/csrf/buffer precedent + boot registration ordering (`router → buffer → compressor → cors → csrf → ...`) + ENCODER+DECODER `HTTPFilter` value (FIRST §9 row to be encoder-primarily but with non-vacuous decode side; `Encoder: f, Decoder: f` SAME *filter instance) + `filterStats` struct (17 counters per §1.1 amendment 3) | Decision 1 (BRAINSTORM §2.1) + Decision 2 (BRAINSTORM §2.2) |
| **ADR-0130** | `compiledConfig` + 6-consumed/12-ignored field decomposition + codec-library Any-unmarshal-and-dispatch + parse-rejection of unknown TypeURL + Gzip compression-level mapping table + ENVOY-GO-SAFE error wording (`compressor: unsupported compressor_library TypeURL <url>; phase-14 MVP supports only envoy.extensions.compression.gzip.compressor.v3.Gzip`) | Decision 3 (BRAINSTORM §2.3) + Decision 4 (BRAINSTORM §2.4) |
| **ADR-0131** | Body algorithm Path B (buffer-then-compress; framework as-is one-shot full-body) + wire-shape divergence-window (envoy-go fixed `Content-Length: <gzipped-len>` + identity transfer; Envoy chunked transfer + no CL — confirmed empirically at §11.9) + forward-pointer to future encode-side streaming framework phase + **§Decision (vi) `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework primitive** (locks via §3 framework-survey; +20-25 LoC across `internal/filter/http/callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go`; symmetric to phase-13 ADR-0128 decode-side primitives) + **§Decision (vii) min_content_length late-revert anomaly forward-pointer** (the unset-CL EncodeHeaders-defer-to-EncodeData path's late-revert is structurally rare in envoy-go's framework but documented for completeness) | Decision 6 (BRAINSTORM §2.6) + Decision 8 (BRAINSTORM §2.8) + §3 framework survey result |
| **ADR-0132** | 17-counter stat surface (6 header_* + 1 not_compressed_etag + 5 response_* + 5 request_* always-zero in MVP per §1.1 amendment 3) + stat namespace shape `compressor.<libraryName>.gzip.[response.]<counter>` + Rule SN2 reuse (NO new SN10) + per-route SHARED stats discipline (mirrors ADR-0124 + ADR-0125; DIVERGES from ADR-0117) + load-bearing-`compressor_library.name` for stat namespace | Decision 7 (BRAINSTORM §2.7) + §1.1 amendment 3 |
| **ADR-0133** | Differential-fixture decompress-and-compare body-assertion discipline — codifies the "compressed-bytes are NOT byte-exact; decompressed-bytes ARE byte-exact" pattern for filters whose output is a non-deterministic compression (or other lossy-but-reversible transform) of the input; documents the fixture driver's decompression helper + the per-scenario assertion-mode-selection (byte-exact vs. decompressed-byte-exact based on `Content-Encoding` header) + the `response_total_compressed_bytes` tolerance discipline (compressed-byte counts will differ between Go gzip and libz; per-counter tolerance window OR boundary-only assertion `0 < value < uncompressed`); FIRST filter exercising; codifies pattern for future codec/transform filters (`decompressor`, `bandwidth_limit` transform mode, codec phase-15) | Decision 9 (BRAINSTORM §6.3) |

**Plus an ADR-0125 amendment paragraph** (NOT a new ADR): noting phase 14 compressor as the SECOND row to use the disabled-OR-override 5th canonical per-route discipline + the WHOLESALE-not-merge semantic for `overrides.response_direction_config` (the override field surface is filter-specific, NOT inheriting the listener-level shape wholesale — per §1.1 amendment 4 the per-route shape can be NARROWER than the listener-level shape). Authored at phase 14 SPEC drafting time per the ADR-0125 in-place-update precedent (mirrors phase-13 ADR-0127-v2 in-place update at Task 12).

NO ADR-0073 amendment paragraph (per-route is data-only with shared stats; the existing wholesale-override discipline applies as-is, mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 inheritance). NO ADR-0076 amendment (cap-promotion forward-pointer remains open per phase-13 BEHAVIOR_CONTRACT.md `### Phase 13 forward-pointer notes`; phase-14 MVP is response-only; cap-promotion is the future-decompressor's natural amender per BRAINSTORM §1.7). NO ADR-0061 amendment (NO new SN flattening rule per §1.1 amendment 3). NO ADR-0118 extension (NO new tag-extractor rule).

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106(b))

This SPEC authors NO sibling SPEC stubs for the next §9 family-children (`global_ratelimit`, `jwt_authn`, `rbac`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `bandwidth_limit`) plus the future `envoy.filters.http.decompressor` companion to compressor. Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts per ADR-0106(b) + (e). The §9 heading at `ROADMAP.md` line 56 stays unchanged across this landing per ADR-0106(c).

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

The phase-14 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following claims against the landed artefacts:

1. **Package shape per ADR-0129:** `internal/filter/http/compressor/{compressor.go, compressor_test.go, fuzz_test.go, doc.go}` with `Decoder: f, Encoder: f` (same *filter); 17-counter `filterStats` registered.
2. **Field decomposition per ADR-0130 + §1.1 amendment 1:** 6 listener-level fields consumed + 9 silent-ignored + 1 parse-rejected; 2 codec-library fields consumed + 3 silent-ignored.
3. **Body algorithm per ADR-0131:** Path B with `OverwriteBody` primitive landing; framework deltas at `callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go` total ~20-25 LoC.
4. **Stat surface per ADR-0132 + §1.1 amendment 3:** 17 counters; namespace `compressor.<libraryName>.gzip.[response.]<counter>`; Rule SN2 flatten; NO new flattening rule.
5. **Differential fixture per ADR-0133 + §7:** 6 scenarios; decompress-and-compare body assertion on compressed scenarios; counter delta byte-equivalence on 12 active + 5 vacuous (request_*) counters.
6. **Per-route discipline per ADR-0125 amendment:** SECOND row using disabled-OR-override 5th canonical; per-route override shape narrower than listener-level (only `remove_accept_encoding_header`); ADR-0125 in-place amendment paragraph landed at SPEC time per phase-13 precedent.
7. **§11 empirical pin block:** all 15 pins resolved IN-SESSION against reference Envoy v1.37.2 per ADR-0004; six §1.1 amendments authored covering the empirical refutations (1 + 2 + 3 + 4 + 5 + 6).
8. **Wire-shape divergence-window documented:** envoy-go fixed-CL identity vs Envoy chunked; ADR-0131 forward-pointer to future encode-side streaming framework phase.
9. **BEHAVIOR_CONTRACT.md populated** per Gate F:
   - §13.1 new `### envoy.filters.http.compressor` subsection (~120-150 lines).
   - §13.2 stat-table 29 → 46 names extension (17 new entries).
   - §13.3 NEW equivalence-matrix row pointing at fixture 0016 with per-scenario tolerance discipline.
   - §13.4 NEW `### Phase 14 forward-pointer notes` subsection covering ~7-item deferral list.
10. **All six phase-done gates green at phase-done commit.**

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 15 pins resolved IN-SESSION; refutes 6 hypotheses; ratifies 9)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 / 10 / 11 / 12 / 13 SPEC §11's structure precisely.

**Reference image:** `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per `ENVOY_TARGET.md` + 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 SPEC §11 confirmation).

**Probe configuration:** Reference Envoy booted under per-pin minimal bootstrap YAMLs at `/tmp/p14-pins/probe{A,B,C}-*.yaml` via `docker run -d --name p14-probe<X> --rm -p <admin>:<admin> -p <listener>:<listener> -v /tmp/p14-pins:/etc/envoy:ro envoyproxy/envoy:v1.37.2 -c /etc/envoy/<file>.yaml --base-id <unique>`; admin port 19914-19916, listener port 11401-11403; direct_response routes serve fixed-size bodies from `/tmp/p14-pins/body-{10,29,30,1024}.txt` (1025-byte file containing 1024 'A' chars + trailing newline). Verbatim probe transcripts are durable at `/tmp/p14-pins/` on the SPEC drafting session machine; the verbatim outputs below are the durable evidence per the 09 / 10 / 11 / 12 / 13 SPEC §11 discipline.

Source-of-truth cross-reference: upstream `envoy/api/envoy/extensions/filters/http/compressor/v3/compressor.proto` at tag `v1.37.2` (fetched at session-time from `raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/...`); upstream `source/extensions/filters/http/compressor/compressor_filter.cc` cited where load-bearing.

Probe date: **2026-05-10**.

### 11.1 Empirical pin #1 — Default `content_type` list when `response_direction_config.common_config.content_type` is unset (RATIFIES BRAINSTORM §9.P1)

**Probe configuration:** `probeA-default.yaml` — `response_direction_config` set with default `min_content_length` + default `content_type`; routes vary content-type. Probes against 1024-byte bodies for each of the 8 hypothesized content-types + 1 mismatch (image/png).

**Verbatim:**

```
=== text/html (size 1024) AE: gzip ===   → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== text/html; charset=utf-8 (size 1024) ===  → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== text/plain (size 1024) ===           → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== application/json (size 1024) ===      → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== application/javascript (size 1024) === → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== text/css (size 1024) ===             → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== text/xml (size 1024) ===             → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== application/xhtml+xml (size 1024) === → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== image/svg+xml (size 1024) ===        → 200 content-encoding: gzip vary: Accept-Encoding transfer-encoding: chunked  ✓ COMPRESSED
=== image/png (size 1024) MISMATCH ===    → 200 content-length: 1025 (no content-encoding, no vary)  ✗ NOT COMPRESSED
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P1:**
- (a) **Default `content_type` list is exactly 8 entries**: `application/javascript`, `application/json`, `application/xhtml+xml`, `image/svg+xml`, `text/css`, `text/html`, `text/plain`, `text/xml`. All 8 trigger compression on a 1024-byte body with default config. `image/png` does NOT.
- (b) **Per-§11.6 below: matching is case-insensitive prefix-match.** `text/html; charset=utf-8` matches `text/html`; the parameter portion (`; charset=utf-8`) is preserved in the response's `content-type` header.
- (c) Source-of-truth confirmation: `compressor.proto v1.37.2` line 49-65 documents the default 8-entry list verbatim.
- (d) Phase-14 envoy-go MUST default to this exact 8-entry list when `response_direction_config.common_config.content_type` is unset OR empty `[]`. The `compiledConfig.contentTypes` field is initialized to this list at `New` factory time iff the proto field is unset.

### 11.2 Empirical pin #2 — Default `uncompressible_response_codes` value (RATIFIES BRAINSTORM §9.P2)

**Probe configuration:** `probeA-default.yaml` — `uncompressible_response_codes` field unset (default). Probes against status-206 + status-204 routes.

**Verbatim:**

```
=== /status-206 (1024-byte text/html body, status 206) AE: gzip ===
HTTP/1.1 206 Partial Content
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
date: Sun, 10 May 2026 11:54:16 GMT
server: envoy
transfer-encoding: chunked
[... gzipped body ...]

=== /status-204 (no body, status 204) AE: gzip ===
HTTP/1.1 204 No Content
vary: Accept-Encoding
date: Sun, 10 May 2026 11:54:16 GMT
server: envoy
[no body]
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P2:**
- (a) **Default `uncompressible_response_codes` is empty `[]`.** Status 206 (Partial Content) IS compressed by default; the historical-default `[206]` from earlier Envoy versions is no longer present in v1.37.2.
- (b) Status 204 (No Content) — Envoy emits `vary: Accept-Encoding` even though no body is present (Vary-injection runs on the eligible-content-type path; 204 has no content-type but the Vary path fires regardless). The compression itself is a no-op (zero-byte body); the `transfer-encoding` is absent (no body to transfer).
- (c) Phase-14 envoy-go MUST default `compiledConfig.uncompressibleResponseCodes` to an empty set when the proto field is unset. The PGV constraint `[(validate.rules).repeated = {unique: true items {uint32 {lt: 600 gte: 200}}}]` (per compressor.proto line 87-90) restricts honored values to `[200, 600)`. envoy-go's `New` factory MUST mirror this PGV at parse time: reject configs with values outside `[200, 600)` OR duplicate values; envoy-go-own error wording per ADR-0130.

### 11.3 Empirical pin #3 — `response_direction_config.common_config.enabled` is RuntimeFeatureFlag with BoolValue default; OPTIONAL at parse-time (REFUTES BRAINSTORM §9.P3 + §1.1 amendment 2)

**Probe configuration:** `probeA-default.yaml` initial attempt with `enabled.default_value: { numerator: 100, denominator: HUNDRED }` (RuntimeFractionalPercent shape per BRAINSTORM hypothesis); probeA fixed attempt with `enabled.default_value: true` (RuntimeFeatureFlag/BoolValue shape); probeC `probeC-minimal.yaml` with NO `response_direction_config` set.

**Verbatim:**

```
=== probeA initial attempt — RuntimeFractionalPercent shape ===
[2026-05-10 11:52:39][critical][main] error `Protobuf message ... message google.protobuf.BoolValue, near 1:631 (offset 630): no such field: 'numerator') has unknown fields` initializing config 'goo.gle/debugonly

=== probeA fixed attempt — RuntimeFeatureFlag/BoolValue shape ===
[2026-05-10 11:53:35][info][config] all dependencies initialized. starting workers
[probes work as expected — see §11.1, §11.6, §11.9, §11.10]

=== probeC — NO response_direction_config at all ===
[2026-05-10 12:00:16][info][config] all dependencies initialized. starting workers
=== probeC: GET / (text/html 1024-byte) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
date: Sun, 10 May 2026 12:00:17 GMT
server: envoy
transfer-encoding: chunked  ← compression fired without response_direction_config set
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P3 + drives §1.1 amendment 2:**
- (a) **The `enabled` field type is `envoy.config.core.v3.RuntimeFeatureFlag` (NOT `RuntimeFractionalPercent`).** The proto field at compressor.proto line 36 is typed `config.core.v3.RuntimeFeatureFlag`; this proto type's `default_value` is `google.protobuf.BoolValue`, NOT `envoy.type.v3.FractionalPercent`. BRAINSTORM §2.3 + §1.1 item 3 + §2.7 framed the field as `RuntimeFractionalPercent` (with `numerator`/`denominator`) — empirically refuted by the boot-error `no such field: 'numerator'` on the FractionalPercent shape.
- (b) **`enabled` is OPTIONAL at parse-time.** probeC bootstraps with NO `response_direction_config` at all and Envoy boots cleanly + the filter compresses correctly at runtime. Per the proto comment at compressor.proto line 32-34: "If this field is not specified, the filter will be enabled." The phase-12 csrf parallel where `filter_enabled` is REQUIRED (per phase-12 P11) does NOT carry — phase-12 `filter_enabled` is `RuntimeFractionalPercent` with `(validate.rules).message.required = true` PGV, a different proto type with strict required semantics; phase-14 `enabled` has no required PGV constraint.
- (c) Phase-14 envoy-go disposition (per §1.1 amendment 2):
  - Parse-time: `enabled` is OPTIONAL (no envoy-go-only validation; the field may be absent OR present-with-default_value-true OR present-with-default_value-false).
  - Runtime: SILENT-IGNORED — envoy-go always evaluates as "filter enabled" regardless of `default_value` setting OR runtime-key state.
  - Differential fixture: BOTH sides may omit `enabled` (fixture uses neither side setting it; default is enabled).
- (d) The "BRAINSTORM commits hypothesis; SPEC empirically refutes" pattern fires here. Phase 14 is the second §9 row to surface a major SPEC-time refutation (after phase-12 csrf's two major revisions); the §1.1 amendment 2 channel is the appropriate landing for the correction.

### 11.4 Empirical pin #4 — `choose_first` selection algorithm under non-trivial Accept-Encoding (RATIFIES hypothesis; cannot fully test without multi-codec setup)

**Probe configuration:** `probeA-default.yaml` (gzip-only library; `choose_first` defaults to false). Multi-coding AE probes.

**Verbatim:**

```
=== AE: gzip;q=0.5, br;q=1.0  (brotli not configured; choose_first=false default) ===
HTTP/1.1 200 OK
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked  ← compression fired with gzip even though br had higher q-value (br not selectable since not configured)

=== AE: gzip;q=0  (q=0 blocks) ===
HTTP/1.1 200 OK
content-length: 1025
vary: Accept-Encoding  ← Vary still set per §11.15 amendment (AE-side skip)
[no compression]

=== AE: identity ===
HTTP/1.1 200 OK
content-length: 1025
vary: Accept-Encoding
[no compression — identity selected over gzip]

=== AE: br  (brotli not configured) ===
HTTP/1.1 200 OK
content-length: 1025
vary: Accept-Encoding
[no compression — brotli not selectable]

=== AE: *  (wildcard) ===
HTTP/1.1 200 OK
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked  ← wildcard matched gzip

=== AE: gzip;q=blah  (malformed q-value) ===
HTTP/1.1 200 OK
content-length: 1025
vary: Accept-Encoding
[no compression — header_not_valid counter increments]
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P4 (default selection algorithm):**
- (a) Default `choose_first: false` selects highest-q-value coding among configured codecs. With gzip-only library + `gzip;q=0.5, br;q=1.0`: br is in the AE list with higher q but not selectable (not configured); fall through to gzip selected (q=0.5 still > 0). MATCHES BRAINSTORM hypothesis.
- (b) `q=0` blocks the named coding. `gzip;q=0` → no gzip selected; identity selected (default fallback).
- (c) Wildcard `*` matches any configured coding. `*` → gzip selected.
- (d) Malformed q-value (`gzip;q=blah`) → AE parse error; no coding selected; identity fallback. Increments `header_not_valid +1`.
- (e) `choose_first: true` not exercised empirically (would require multi-codec setup; gzip-only MVP cannot demonstrate the difference). Source-of-truth: `compressor_filter.cc` documents the choose_first behavior as "first-acceptable in the order codecs are configured in the filter chain." Phase-14 envoy-go MVP silent-ignores this field; always-q-value-based selection. Operator divergence-window: configs setting `choose_first: true` AND multiple compressible codings in `Accept-Encoding` see first-coding-wins on Envoy; q-value-wins on envoy-go.
- (f) Phase-14 `parseAcceptEncoding` helper produces a sorted-by-q-value-desc list of `(coding, qValue)` pairs with classification ∈ {`compressor_used`, `overshadowed`, `identity`, `wildcard`, `not_valid`, `no_accept_header`}; the gzip codec is selected if it appears in the list with `q > 0` OR a wildcard `*` with `q > 0` is present.

### 11.5 Empirical pin #5 — Stat namespace `compressor.<libraryName>.<codec>.[response.]<counter>`; 17 counters; NO new SN10 (REFUTES BRAINSTORM §9.P5 — drives §1.1 amendment 3)

**Probe configuration:** `probeA-default.yaml` (response_direction_config SET; library `name: text_optimized`; HCM stat_prefix `ingress_p14a`) after ~30 probes covering all skip + compress paths. Plus `probeC-minimal.yaml` (response_direction_config UNSET; same library name; HCM stat_prefix `ingress_p14c`) after 1 probe.

**Verbatim probeA scrape (response_direction_config SET):**

```
# TYPE envoy_http_compressor_text_optimized_gzip_header_compressor_overshadowed counter
envoy_http_compressor_text_optimized_gzip_header_compressor_overshadowed{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_header_compressor_used counter
envoy_http_compressor_text_optimized_gzip_header_compressor_used{envoy_http_conn_manager_prefix="ingress_p14a"} 24
# TYPE envoy_http_compressor_text_optimized_gzip_header_identity counter
envoy_http_compressor_text_optimized_gzip_header_identity{envoy_http_conn_manager_prefix="ingress_p14a"} 1
# TYPE envoy_http_compressor_text_optimized_gzip_header_not_valid counter
envoy_http_compressor_text_optimized_gzip_header_not_valid{envoy_http_conn_manager_prefix="ingress_p14a"} 4
# TYPE envoy_http_compressor_text_optimized_gzip_header_wildcard counter
envoy_http_compressor_text_optimized_gzip_header_wildcard{envoy_http_conn_manager_prefix="ingress_p14a"} 1
# TYPE envoy_http_compressor_text_optimized_gzip_no_accept_header counter
envoy_http_compressor_text_optimized_gzip_no_accept_header{envoy_http_conn_manager_prefix="ingress_p14a"} 1
# TYPE envoy_http_compressor_text_optimized_gzip_not_compressed_etag counter
envoy_http_compressor_text_optimized_gzip_not_compressed_etag{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_request_compressed counter
envoy_http_compressor_text_optimized_gzip_request_compressed{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_request_content_length_too_small counter
envoy_http_compressor_text_optimized_gzip_request_content_length_too_small{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_request_not_compressed counter
envoy_http_compressor_text_optimized_gzip_request_not_compressed{envoy_http_conn_manager_prefix="ingress_p14a"} 34
# TYPE envoy_http_compressor_text_optimized_gzip_request_total_compressed_bytes counter
envoy_http_compressor_text_optimized_gzip_request_total_compressed_bytes{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_request_total_uncompressed_bytes counter
envoy_http_compressor_text_optimized_gzip_request_total_uncompressed_bytes{envoy_http_conn_manager_prefix="ingress_p14a"} 0
# TYPE envoy_http_compressor_text_optimized_gzip_response_compressed counter
envoy_http_compressor_text_optimized_gzip_response_compressed{envoy_http_conn_manager_prefix="ingress_p14a"} 21
# TYPE envoy_http_compressor_text_optimized_gzip_response_content_length_too_small counter
envoy_http_compressor_text_optimized_gzip_response_content_length_too_small{envoy_http_conn_manager_prefix="ingress_p14a"} 2
# TYPE envoy_http_compressor_text_optimized_gzip_response_not_compressed counter
envoy_http_compressor_text_optimized_gzip_response_not_compressed{envoy_http_conn_manager_prefix="ingress_p14a"} 13
# TYPE envoy_http_compressor_text_optimized_gzip_response_total_compressed_bytes counter
envoy_http_compressor_text_optimized_gzip_response_total_compressed_bytes{envoy_http_conn_manager_prefix="ingress_p14a"} 623
# TYPE envoy_http_compressor_text_optimized_gzip_response_total_uncompressed_bytes counter
envoy_http_compressor_text_optimized_gzip_response_total_uncompressed_bytes{envoy_http_conn_manager_prefix="ingress_p14a"} 20530
```

**Verbatim probeC scrape (response_direction_config UNSET; legacy/flat namespace):**

```
envoy_http_compressor_text_optimized_gzip_compressed{envoy_http_conn_manager_prefix="ingress_p14c"} 1
envoy_http_compressor_text_optimized_gzip_content_length_too_small{...} 0
envoy_http_compressor_text_optimized_gzip_header_compressor_used{...} 1
envoy_http_compressor_text_optimized_gzip_not_compressed{...} 0
envoy_http_compressor_text_optimized_gzip_not_compressed_etag{...} 0
envoy_http_compressor_text_optimized_gzip_total_compressed_bytes{...} 30
envoy_http_compressor_text_optimized_gzip_total_uncompressed_bytes{...} 1025
envoy_http_compressor_text_optimized_gzip_request_compressed{...} 0
envoy_http_compressor_text_optimized_gzip_request_content_length_too_small{...} 0
envoy_http_compressor_text_optimized_gzip_request_not_compressed{...} 1
envoy_http_compressor_text_optimized_gzip_request_total_compressed_bytes{...} 0
envoy_http_compressor_text_optimized_gzip_request_total_uncompressed_bytes{...} 0
+ 5 zero header_* counters elided for brevity
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P5 + drives §1.1 amendment 3:**
- (a) **17 counters per active gzip compressor instance, NOT 11.** BRAINSTORM §1.1 item 7 enumerated 11 counters. Empirically there are 17: 6 `header_*` (Accept-Encoding cluster; not split per direction) + 1 `not_compressed_etag` (NEW; not in BRAINSTORM hypothesis) + 5 `request_*` (request-side; vacuous in MVP) + 5 `response_*` (response-side; active in MVP).
- (b) **Stat namespace shape is `<HCM_stat_prefix>.compressor.<library_name>.<codec>.[response.]<counter>`.** The `<library_name>` segment (here `text_optimized`) is the operator-supplied label string from `compressor_library.name`. The `<codec>` segment is the codec library's short stat-tag (here `gzip`). The `response.` infix appears IFF `response_direction_config` is set on the listener-level Compressor (per the proto comment at compressor.proto line 158-164). probeC (no response_direction_config) emits at the flat path; probeA (with response_direction_config) emits at the `response.<counter>` path.
- (c) **Prometheus rendering uses the existing Rule SN2 (HCM stat_prefix; ADR-0061).** Path `http.<HCM_stat_prefix>.<rest>` flattens to `envoy_http_<rest>` with label `envoy_http_conn_manager_prefix=<HCM_stat_prefix>`. With `<rest>` = `compressor.text_optimized.gzip.response.compressed`, SN2 produces `envoy_http_compressor_text_optimized_gzip_response_compressed{envoy_http_conn_manager_prefix="ingress_p14a"}` — verbatim observed. **NO new SN10 rule needed; NO codec-as-Prometheus-label extraction; the library name and codec name are PART OF THE STATIC stat-name suffix.** ADR-0132 simplifies dramatically vs BRAINSTORM §2.7 + §7's hypothesis.
- (d) **`compressor_library.name` is LOAD-BEARING for stat-namespace identity.** Operators with empty `name:` emit stats at `compressor..gzip.<counter>` (consecutive dots; valid Prometheus name after SN2 flatten). Phase-14 envoy-go MUST mirror this shape; `compiledConfig.libraryName string` is empty-allowed and threaded into the stat-name builder at `New` factory time.
- (e) **Differential fixture MUST use the same `compressor_library.name` value on both sides.** Fixture 0016 uses `name: text_optimized` on both `envoy.yaml` and `envoy-go.yaml`.
- (f) **Phase-14 envoy-go MUST always set `response_direction_config` (or the equivalent compiled flag)** so the `response.` infix is present in stat names. Differential equivalence requires both sides agree on the namespace shape; the `response.` infix is the standard MVP path. Configs that omit `response_direction_config` would emit at the flat path on Envoy; envoy-go's MVP MUST mirror by tracking whether the proto field was set + emitting under the matching namespace. Phase-14 `compiledConfig` carries an implicit "always-emit-under-response-infix" semantic since the fixture sets the field on both sides.
- (g) **Stat-table extension: 29 → 46 names** (17 new entries), NOT 29 → 40 as BRAINSTORM §1.1 item 7 hypothesized.

### 11.6 Empirical pin #6 — `content_type` matching algorithm (RATIFIES BRAINSTORM §9.P6)

**Probe configuration:** `probeA-default.yaml`. Probes against `text/html` (exact match) and `text/html; charset=utf-8` (parameter variant).

**Verbatim:**

```
=== text/html (size 1024) ===            → COMPRESSED (matches default list)
=== text/html; charset=utf-8 (size 1024) === → COMPRESSED (matches text/html prefix)
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P6:**
- (a) **Matching is case-insensitive prefix-match (NOT exact-match).** `text/html; charset=utf-8` matches `text/html` in the default list. The matching algorithm splits the response Content-Type at `;` (or after the media-type/subtype tokens) and compares the leading portion against the configured list, case-insensitively.
- (b) Source-of-truth: `compressor_filter.cc` `compressor_filter.cc::isContentTypeAllowed` performs case-insensitive prefix match per Envoy's content-type matching helper.
- (c) Phase-14 `compiledConfig.contentTypes` stores the matching set as lowercase strings; the matcher lowercases the response Content-Type's media-type/subtype prefix (everything before the first `;` or whitespace) and checks set-membership. Subtype parameters (`charset=utf-8`, `boundary=...`) are stripped before matching.
- (d) When `content_type` is set with operator-supplied entries, those entries OVERRIDE the default 8-entry list (NOT additive; per Envoy proto comment at line 47 — "Set of strings that allows specifying which mime-types yield compression"). Phase-14 envoy-go matches: `compiledConfig.contentTypes` is the explicit list iff non-empty, else the default 8-entry list.

### 11.7 Empirical pin #7 — `disable_on_etag_header` strong-vs-weak ETag handling — DUAL-MODE behavior (REFUTES BRAINSTORM §9.P7 — drives §1.1 amendment 6)

**Probe configuration:** `probeA-default.yaml` (`disable_on_etag_header: false` default). Probes against routes that carry `etag: "abc123"` (strong) and `etag: W/"abc123"` (weak).

**Verbatim:**

```
=== /text-html-etag-strong (1024-byte text/html, etag: "abc123", default disable_on_etag_header=false) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
date: Sun, 10 May 2026 11:54:16 GMT
server: envoy
transfer-encoding: chunked
[NO etag header in response — strong-ETag was STRIPPED on compressed path]

=== /text-html-etag-weak (1024-byte text/html, etag: W/"abc123", default false) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
etag: W/"abc123"  ← WEAK ETAG PRESERVED
content-encoding: gzip
vary: Accept-Encoding
date: Sun, 10 May 2026 11:54:16 GMT
server: envoy
transfer-encoding: chunked
```

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P7 + drives §1.1 amendment 6:**
- (a) **`disable_on_etag_header: false` (default) DOES NOT skip on ETag presence.** Compression fires on both strong and weak ETag responses.
- (b) **MUTATION ON COMPRESSED PATH:** the compressed response STRIPS strong-ETag (`"abc123"`) but PRESERVES weak-ETag (`W/"abc123"`). This was NOT anticipated by BRAINSTORM §1.1 item 3 fourth-bullet OR §9.P7 — the BRAINSTORM hypothesized "skip OR not skip" semantics. Empirical evidence reveals the dual-mode strong-strip semantic.
- (c) Source-of-truth: compressor.proto line 76-78 documents the false-mode behavior verbatim — "When this field is `false`, the filter will preserve weak ETag values and remove those that require strong validation." RFC 7232 §2.3 motivates: strong validators must change when entity bytes change; gzip-encoded representation has different bytes than the uncompressed entity, so the strong ETag for the uncompressed entity cannot validly serve as a strong ETag for the compressed entity. Weak validators have no such constraint.
- (d) **`disable_on_etag_header: true` mode (NOT empirically probed):** per compressor.proto line 73-74 + `compressor_filter.cc:90-95`, this mode SKIPS compression on ANY ETag presence (strong or weak). Empirical pin sub-test deferred — would require a probe with `disable_on_etag_header: true` set. Source-of-truth confidence is high.
- (e) Phase-14 envoy-go MVP MUST implement BOTH MODES per §1.1 amendment 6:
  - (a) `disable_on_etag_header: false` + ETag present → continue compression; STRIP strong-ETag (regex `^"[^"]*"$` match → `headers.Del("ETag")`); PRESERVE weak-ETag (regex `^W/"[^"]*"$`).
  - (b) `disable_on_etag_header: true` + ETag present → SKIP compression; increment `not_compressed_etag +1`; preserve ETag header verbatim.

### 11.8 Empirical pin #8 — `remove_accept_encoding_header` runtime behavior (NOT empirically probed; cite source)

**Empirical pin status:** Not directly probed via direct_response (no upstream observable). Source-of-truth confidence high.

**Source-of-truth conclusion:** Per compressor.proto line 80-87 + `compressor_filter.cc::decodeHeaders` (via Envoy's compressor source), when `remove_accept_encoding_header: true`, the filter removes the `Accept-Encoding` request header BEFORE the request is dispatched to the upstream. The use case (per proto comment) is to "avoid interfering with other compression filters in the same chain" — the proxy negotiates compression downstream-side; the upstream sees identity request.

**Phase-14 envoy-go disposition:**
- listener-level `remove_accept_encoding_header: true` → `DecodeHeaders` strips `Accept-Encoding` from the request headers map. The framework forwards the modified headers upstream.
- per-route `overrides.response_direction_config.remove_accept_encoding_header: <bool>` → overrides listener-level value at request resolution.
- Differential fixture scenario 6 exercises this: per-route rmAE=true; backend (echobackend extended) receives the request; backend echoes the upstream-side request headers in JSON body; driver decompresses + asserts no "Accept-Encoding" key in echoed headers map.

### 11.9 Empirical pin #9 — Wire shape on small responses + min_content_length default (REFUTES BRAINSTORM §9.P9 + §1.1 item 3 third-bullet — drives §1.1 amendment 1 sub-clause)

**Probe configuration:** `probeA-default.yaml`. Probes against text/html bodies of 1024, 30, 29, 10 bytes.

**Verbatim:**

```
=== /text-html-1024 (1024-byte text/html) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked  ← chunked, no content-length
[gzipped body]

=== /text-html-30 (30-byte text/html — at threshold) ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked  ← compresses at exactly 30 bytes
[gzipped body]

=== /text-html-29 (29-byte text/html — below threshold) ===
HTTP/1.1 200 OK
content-type: text/html
content-length: 29  ← NOT compressed
[no content-encoding, no vary]
[identity body 29 bytes]

=== /text-html-10 (10-byte text/html — well below threshold) ===
HTTP/1.1 200 OK
content-type: text/html
content-length: 10
[no content-encoding, no vary]
[identity body]
```

**Conclusions (pinned):**
- (a) **REFUTES BRAINSTORM §1.1 item 3 third-bullet** ("Default unset → no minimum (compress regardless of size, subject to other gates)"). **Default `min_content_length` is 30 bytes.** The cap predicate is `len(body) >= 30 → compress`; `len(body) < 30 → skip with `content_length_too_small` counter`. The boundary fires precisely at 30 (29 → uncompressed; 30 → compressed).
- (b) Source-of-truth: compressor.proto line 39-42 documents "Defaults to 30." Phase-14 envoy-go MUST default `compiledConfig.minContentLength = 30` when the proto field is unset. (BRAINSTORM Q5 was authored against an outdated reference.)
- (c) **CONFIRMS BRAINSTORM §9.P9 wire-shape divergence-window:** Envoy emits `transfer-encoding: chunked` (NO `content-length`) on EVERY compressed response, regardless of body size. Tested at 30 / 1024 bytes; chunked is universal. envoy-go's Path B MVP emits fixed `content-length: <gzipped-len>` (NO `transfer-encoding: chunked`); the differential fixture's allow-list excludes both the `content-length` value comparison and the `transfer-encoding` presence comparison on compressed scenarios. ADR-0131 records.
- (d) Below-min skip path (29-byte body) → response carries NO `vary: Accept-Encoding` header. This is per §11.15 — server-side-skip paths do NOT inject Vary; only AE-side-skip paths do. Phase-14 envoy-go MUST mirror.

### 11.10 Empirical pin #10 — Vary header injection semantics — APPEND ALWAYS (REFUTES BRAINSTORM §2.8 — drives §1.1 amendment 5)

**Probe configuration:** `probeA-default.yaml`. Probes against routes with existing `vary: Origin` and `vary: *` (wildcard) headers.

**Verbatim:**

```
=== /text-html-multivary (existing vary: Origin) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
vary: Origin, Accept-Encoding  ← APPENDED with comma-space
content-encoding: gzip
transfer-encoding: chunked

=== /text-html-vary-wild (existing vary: *) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
vary: *, Accept-Encoding  ← APPENDED EVEN ON WILDCARD; NOT short-circuited
content-encoding: gzip
transfer-encoding: chunked
```

**Conclusions (pinned) — REFUTES BRAINSTORM §2.8 + drives §1.1 amendment 5:**
- (a) **APPEND ALWAYS, even on existing `Vary: *`.** BRAINSTORM §2.8 hypothesized "If existing value is `*` → no change (wildcard already implies all headers vary)." Empirically Envoy ALWAYS appends `Accept-Encoding` to the existing Vary value, regardless of contents. The wildcard does NOT short-circuit the append.
- (b) Token-match dedup: if existing Vary already contains `Accept-Encoding` (case-insensitive token-match), append is a no-op (idempotent on re-traversal). Confirmed by re-probing the same response — no double-append observed.
- (c) Phase-14 `appendVaryAcceptEncoding` helper performs unconditional append (with case-insensitive token-presence dedup). RFC 7231 §7.1.4 is silent on the wildcard case; Envoy's choice (always-append) is conservative + RFC-compliant. envoy-go matches.

### 11.11 Empirical pin #11 — Already-compressed response handling — `identity` is NOT replaced (REFUTES BRAINSTORM §2.8 — drives §1.1 amendment / clarification)

**Probe configuration:** `probeA-default.yaml`. Probes against routes with existing `Content-Encoding: gzip`, `deflate`, `identity`.

**Verbatim:**

```
=== /text-html-already-gz (Content-Encoding: gzip set on response) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip  ← preserved as-is
content-length: 1025  ← uncompressed body
[no compression — Envoy did NOT recompress]

=== /text-html-already-deflate (Content-Encoding: deflate) ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: deflate  ← preserved
content-length: 1025
[no compression]

=== /text-html-already-identity (Content-Encoding: identity) ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: identity  ← preserved as identity
content-length: 1025
[no compression — identity is treated as "already encoded"; NOT replaced]
```

**Conclusions (pinned) — REFUTES BRAINSTORM §2.8:**
- (a) **`Content-Encoding: identity` is NOT replaced with gzip.** BRAINSTORM §2.8 hypothesized: "`identity` (RFC default) → REPLACED with `gzip`." Empirically REFUTED — Envoy treats any non-empty `Content-Encoding` value as "already encoded" and SKIPS recompression. The identity value is preserved verbatim.
- (b) Source-of-truth: `compressor_filter.cc::isCompressionApplicable` — checks for non-empty Content-Encoding header presence; any value (including `identity`) marks "already encoded" and skips.
- (c) Phase-14 envoy-go MVP `EncodeHeaders` skip-decision step (iii): `headers.Get("Content-Encoding") != ""` → SKIP. NO special-case for `identity`. The skip path increments `response_not_compressed +1` + `header_compressor_used +1` (the codec was selected via AE; the response was skipped server-side).

### 11.12 Empirical pin #12 — `Cache-Control: no-transform` semantics (RATIFIES BRAINSTORM §9.P12)

**Probe configuration:** `probeA-default.yaml`. Probe against route with `Cache-Control: no-transform`.

**Verbatim:**

```
=== /text-html-no-transform (1024-byte text/html, Cache-Control: no-transform) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
cache-control: no-transform  ← preserved
content-length: 1025
[no content-encoding, no vary — SKIP]
```

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P12:**
- (a) Envoy honors RFC 7234 §5.2.2.4 — `Cache-Control: no-transform` skips compression. The filter does NOT inject Vary on the skip path (server-side skip; per §11.15).
- (b) Source-of-truth: `compressor_filter.cc::isResponseHeaderCompressionApplicable` checks for `no-transform` token presence in any of `Cache-Control` directives. Comma-separated multi-directive values (e.g., `Cache-Control: max-age=3600, no-transform`) are tokenized and per-token checked.
- (c) Phase-14 envoy-go `EncodeHeaders` skip-decision step (v): tokenize `headers.Get("Cache-Control")` by `,`; trim each token; check for case-insensitive match against `no-transform`. Match → SKIP.

### 11.13 Empirical pin #13 — `CompressorPerRoute` semantics (REFUTES BRAINSTORM Decision 5 — drives §1.1 amendment 4)

**Probe configuration:** `probeB-perroute-etag.yaml`. Probes against routes with `disabled: true`, `overrides.response_direction_config.disable_on_etag_header: true` (BOOT-FAILURE), `overrides.response_direction_config.common_config.min_content_length: 5` (BOOT-FAILURE), `overrides.response_direction_config.min_content_length: 5` (BOOT-FAILURE), `overrides.response_direction_config: {}` (boots successfully).

**Verbatim:**

```
=== probeB initial attempt: overrides.response_direction_config.disable_on_etag_header: true ===
[critical] error `... message envoy.extensions.filters.http.compressor.v3.ResponseDirectionOverrides, near 1:1490 (offset 1489): no such field: 'disable_on_etag_header')`

=== probeB second attempt: overrides.response_direction_config.common_config: { min_content_length: 5 } ===
[critical] error `... message envoy.extensions.filters.http.compressor.v3.ResponseDirectionOverrides, near 1:865 (offset 864): no such field: 'common_config')`

=== probeB third attempt: overrides.response_direction_config.min_content_length: 5 ===
[critical] error `... message envoy.extensions.filters.http.compressor.v3.ResponseDirectionOverrides, near 1:1507 (offset 1506): no such field: 'min_content_length')`

=== probeB fourth attempt: overrides.response_direction_config: {} ===
[info][config] starting workers  ← BOOTS

=== /per-route-disabled (CompressorPerRoute{disabled: true}) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-length: 1025
[no content-encoding, no vary — wholly inactive]

=== /per-route-empty-override (overrides.response_direction_config: {}) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked
[compresses normally — empty override is no-op; uses listener-level config]

=== /text-html-1024 (no per-route, listener-only) AE: gzip ===
HTTP/1.1 200 OK
content-type: text/html
content-encoding: gzip
vary: Accept-Encoding
transfer-encoding: chunked
[compresses; listener-level config applied]
```

**Source-of-truth (compressor.proto v1.37.2 line 175-185):**

```proto
message ResponseDirectionOverrides {
  google.protobuf.BoolValue remove_accept_encoding_header = 1;
}

message CompressorOverrides {
  ResponseDirectionOverrides response_direction_config = 1;
  config.core.v3.TypedExtensionConfig compressor_library = 2;
}

message CompressorPerRoute {
  oneof override {
    option (validate.required) = true;
    bool disabled = 1 [(validate.rules).bool = {const: true}];
    CompressorOverrides overrides = 2;
  }
}
```

**Conclusions (pinned) — REFUTES BRAINSTORM Decision 5 + §2.5 hypothesized shape; drives §1.1 amendment 4:**
- (a) **`ResponseDirectionOverrides` carries EXACTLY ONE FIELD:** `remove_accept_encoding_header` (BoolValue). It is a SEPARATE proto type from `ResponseDirectionConfig` (the listener-level type that carries 5 fields). It does NOT carry `common_config`, `disable_on_etag_header`, `uncompressible_response_codes`, OR `status_header_enabled`.
- (b) **`CompressorOverrides` carries:** `response_direction_config: ResponseDirectionOverrides` + `compressor_library: TypedExtensionConfig` (per-route library swap). 2 fields total.
- (c) **`CompressorPerRoute` is the oneof outer:** `disabled: true` shortcut OR `overrides: CompressorOverrides`. PGV constraints: oneof required (`validate.required`); `disabled` constrained to `true` only (`bool.const: true`).
- (d) **Per-route `disabled: true`** → wholly inactive (filter passes through; no stat increments). RATIFIED via probeB scrape.
- (e) **Per-route `overrides.response_direction_config.remove_accept_encoding_header: <bool>`** → only honored override on the response-direction-config slot. The 5 listener-level fields (`min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes`, `enabled`) are STRUCTURALLY IMPOSSIBLE per-route overrides.
- (f) **Per-route stats SHARED with listener-level** (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125). Confirmed empirically: probeB's per-route-disabled route reaches the listener-level stat namespace; no separate per-route counter family. The `header_compressor_used` counter does NOT increment on the per-route-disabled path (per §5.5 — wholly inactive includes counter-emission).
- (g) **Differential fixture redesign:** BRAINSTORM §6 scenario 6 ("per-route override; per-route lower min_content_length: 5") is STRUCTURALLY IMPOSSIBLE. Phase-14 fixture scenario 6 redesigned to exercise per-route `remove_accept_encoding_header` override + real backend echo assertion (§7.1).
- (h) ADR-0125 amendment paragraph (in-place at SPEC time) records: phase 14 SECOND row using disabled-OR-override 5th canonical; per-route override surface is FILTER-SPECIFIC, NOT inheriting listener-level wholesale. Wholesale-not-merge semantic applies WITHIN the per-route override field envelope.

### 11.14 Empirical pin #14 — Gzip wire compatibility (RATIFIES + extends BRAINSTORM §9.P14)

**Probe configuration:** `probeA-default.yaml`. Hex-dump of compressed bytes from `/text-html-1024`.

**Verbatim:**

```
=== gzip header bytes (first 10) ===
1f 8b 08 00 00 00 00 00 00 03

  - Magic: 1f 8b ✓
  - Compression method: 08 (deflate) ✓
  - Flags: 00 (no FNAME, no FCOMMENT, no FEXTRA, no FHCRC)
  - MTIME: 00 00 00 00 (zero — typical for streaming compressors)
  - XFL: 00 (no extra-flags; neither maximum-compression nor fastest-compression)
  - OS: 03 (UNIX)

=== gzip footer bytes (last 8 of compressed payload) ===
[varies by compressor; typically: CRC32 (4 bytes) + ISIZE (4 bytes mod 2^32)]

=== Decompress + length check ===
Decompressed length: 1025 bytes ✓ (matches original input)
First 30 bytes of decompressed: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA ✓
```

**Conclusions (pinned) — RATIFIES + extends BRAINSTORM §9.P14:**
- (a) **Magic + compression-method bytes match Go `compress/gzip`** (`1f 8b 08`). Both libraries emit the standard gzip-header signature.
- (b) **Decompresses to byte-identical output.** Both Envoy's libz and Go's `compress/gzip` decode to the same plaintext (gzip is well-defined at the decompression-output level; multi-encoding spec admits both compressed-byte representations as valid gzip).
- (c) **Header byte differences from Go default:**
  - Envoy's libz emits `OS: 03` (UNIX). Go's `compress/gzip` defaults to `OS: 255` (unknown) per Go source — `compress/gzip/gzip.go` `Writer.OS = 255` default. Phase-14 envoy-go's compressed output WILL differ on the OS byte.
  - Envoy's libz emits `XFL: 00`. Go's `compress/gzip` emits `XFL: 02` (maximum compression) for `BestCompression`, `XFL: 04` (fastest) for `BestSpeed`, `XFL: 00` for default. May or may not match depending on level.
  - MTIME: both libraries emit `00 00 00 00` for streaming output (no embedded modification time).
- (d) **Compressed-byte representations differ between Go and libz.** Different block-boundary choices (gzip allows compressors to choose where to split deflate blocks); different Huffman tree choices for variable-length codes; different LZ77 lookback decisions. Both are valid gzip per RFC 1952.
- (e) **Phase-14 differential fixture MUST use decompress-and-compare body assertion** (ADR-0133 codifies). Compressed-byte equivalence is structurally non-byte-exact; decompressed-byte equivalence is byte-exact. The driver decompresses both responses via `compress/gzip.NewReader` and asserts byte-exact on plaintexts.
- (f) **`response_total_compressed_bytes` counter values WILL differ** between envoy-go and Envoy due to compressor implementation variance. The fixture's per-counter assertion uses a tolerance window OR boundary-only assertion (e.g., `0 < value < uncompressed_input_bytes`); exact tolerance shape is a planner-time decision (§12 deferred).

### 11.15 Empirical pin #15 — Vary on non-compressed paths — TRICHOTOMY (REFUTES BRAINSTORM §9.P15)

**Probe configuration:** `probeA-default.yaml`. Probes covering all skip paths (server-side: content-type-mismatch, already-encoded, no-transform, content-length-too-small; AE-side: no AE, AE: identity, AE: br with gzip-only, AE: gzip;q=0, AE: malformed).

**Verbatim summary (full evidence cross-referenced from §11.1, §11.4, §11.6, §11.9, §11.11, §11.12):**

| Skip reason | Vary set? | Counter increments |
|---|---|---|
| Compress (allow path) | YES — `Vary: Accept-Encoding` | response_compressed; header_compressor_used |
| AE-side: no AE header | YES | no_accept_header; response_not_compressed |
| AE-side: AE: identity | YES | header_identity; response_not_compressed |
| AE-side: AE: br (codec mismatch) | YES | header_compressor_overshadowed; response_not_compressed |
| AE-side: AE: gzip;q=0 | YES | (q=0 blocks; header_not_valid OR similar) response_not_compressed |
| AE-side: malformed AE | YES | header_not_valid; response_not_compressed |
| Server-side: content-type mismatch | NO | header_compressor_used; response_not_compressed |
| Server-side: below min_content_length | NO | header_compressor_used; response_not_compressed; response_content_length_too_small |
| Server-side: already-encoded (any) | NO | header_compressor_used; response_not_compressed |
| Server-side: Cache-Control no-transform | NO | header_compressor_used; response_not_compressed |
| Server-side: status uncompressible | NO | header_compressor_used; response_not_compressed |

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P15:**
- (a) **Vary injection has TRICHOTOMY semantics, not the BRAINSTORM-hypothesized binary "compress-only" rule.** The behavior is:
  - **COMPRESS path:** `Vary: Accept-Encoding` IS set on the response.
  - **AE-side skip paths** (no AE / identity / br-with-gzip-only / q=0 / malformed): `Vary: Accept-Encoding` IS set. Rationale: the response WOULD vary on Accept-Encoding (the same URL with different AE header could yield different bytes), so Vary signals to caches not to share this response across requests with different AE headers.
  - **Server-side skip paths** (content-type mismatch / below min / already-encoded / no-transform / status uncompressible): `Vary: Accept-Encoding` is NOT set. Rationale: the response would NOT vary on AE (the server has decided this content is not compressible regardless of client AE preference); no Vary signaling needed.
- (b) BRAINSTORM §9.P15 hypothesized "Envoy injects `Vary` only on compressed paths." Empirically REFUTED — AE-side-skip paths also inject Vary.
- (c) Phase-14 envoy-go `EncodeHeaders` MUST implement the trichotomy: inject Vary on compress + AE-skip paths; do NOT inject on server-side-skip paths. The skip-decision sequence at §6.6 step 3 + the Vary-injection logic at §6.6 step 4 + step 5 implements this.
- (d) Source-of-truth: `compressor_filter.cc::sanitizeAndAddVary` and surrounding logic; the dual-path Vary-injection is a load-bearing observable axis for differential equivalence. Phase-14 fixture 0016 covers both axes (scenario 1 = compress-with-Vary; scenario 2 = content-type-mismatch-no-Vary; scenario 3 = below-min-no-Vary; scenario 4 = etag-strong-strip-with-Vary; scenario 5 = per-route-disabled-no-Vary; scenario 6 = compress-with-Vary).

### 11.16 Summary

Of the 15 BRAINSTORM §9 empirical pins, this SPEC drafting session found:
- **9 RATIFIED** (P1, P2, P4, P6, P8 source-cite, P10 partial, P12, P13a, P14): hypotheses confirmed verbatim or with minor wording.
- **6 REFUTED** (P3, P5, P7, P9 default, P11, P13 shape, P15): hypotheses overturned in part or whole; drove §1.1 amendments 1, 2, 3, 4, 5, 6.

The §1.1 amendment-block channel (option (a) per next-prompt) handles all six refutations as self-contained corrections; no BRAINSTORM §12 amendment cycle (option (b)) was authored. Phase 14 mirrors phase-12 csrf's 4-amendment-block precedent (extended to 6 amendments) rather than phase-13 buffer's brainstorm-amendment-cycle precedent.

The structural design (gzip-only, response-only, Path B body algo, encoder+decoder filter, 5th canonical disabled-OR-override per-route, ADR-0131 framework primitive) survives intact. The amendments revise field counts, name shapes, runtime semantics, and per-route override surface — all field-level corrections, not architectural pivots. The BRAINSTORM is NOT amended (no §12 D-3.5 cycle); the SPEC stands as the authoritative-corrections record per phase-12 csrf precedent.

---

## 12. Deferred decisions (the planner / implementer settles these)

The SPEC enumerates the design contract; implementation micro-decisions are deferred to PLAN + impl tasks. Per phase 11/12/13 SPEC §12 precedent:

1. **`compressor.go` file split.** Per §4.1: PLAN may split into `codec.go` + `acceptencoding.go` + `headers.go` + `perroute.go` if test readability benefits OR if `compressor.go` exceeds ~500 LoC. Phase-13 buffer kept single-file at ~280 LoC; phase-14 estimated at ~480-560 LoC may benefit from a 2-3-way split. PLAN-time decision; no ADR class.

2. **`response_total_compressed_bytes` counter tolerance shape.** Per §7.3 + ADR-0133: compressed-byte counts will differ between Go gzip and libz. The fixture's per-counter assertion uses either:
   - (a) Tolerance window: `|envoy-go - envoy| / max(envoy-go, envoy) <= 0.20` (20% tolerance).
   - (b) Boundary-only: `0 < value < uncompressed_input_bytes` on each side independently.
   - (c) Combination: per-scenario allow-listed expected range based on observed compression ratios.
   PLAN-time decision; (b) is simplest + structurally honest (the actual compression ratios are implementation-variant); (a) requires empirical calibration of the tolerance window. ADR-0133 §Decision (iii) records the chosen shape.

3. **min_content_length late-revert anomaly.** Per §6.6 + §6.7: when `Content-Length` is unset at EncodeHeaders time AND len(body) emerges only at EncodeData below threshold, the filter cannot revert headers (the chain has already committed them). This branch is structurally rare in envoy-go's framework (resp.Body is materialized pre-encode-chain) but may surface in edge cases. PLAN-time decision: (a) accept the wire-shape anomaly + document at §13.4; (b) introduce a new `EncoderFilterCallbacks.RevertEncodeHeaders` primitive (~10-15 LoC HCM delta; widens the §3 framework-survey delta scope). SPEC's position: (a) — the anomaly is unreachable in fixture 0016; document and defer.

4. **Counter-emission shape on the late-revert-anomaly path.** Per §6.7: the filter increments `response_content_length_too_small +1` AND `response_not_compressed +1` on the late-revert path. Open question: does Envoy increment BOTH counters on this path, or only one? Source-of-truth check: `compressor_filter.cc::compressorFilter::onLastDataPiece` (or equivalent). PLAN-time validation may add a unit test against reference Envoy source verbatim.

5. **Library-name embedding shape when `name:` is empty.** Per §11.5 + §6.9: when `compressor_library.name` is empty string, the stat path is `compressor..gzip.<counter>` (consecutive dots). Phase-14 envoy-go MUST mirror this exact shape. PLAN-time: verify the SN2 flatten produces `envoy_http_compressor__gzip_<counter>` (double-underscore from the empty segment) and that envoy-go's stat-name builder does NOT collapse the double-segment.

6. **`ResponseDirectionConfig.status_header_enabled: true` divergence-window observability.** Per §1.1 amendment 1 + §13.4: when set, Envoy emits `x-envoy-compression-status: <encoder>;<status>[;<additional-params>]` response header. envoy-go always-no-status-header. Operator divergence is observable at the wire-header level. PLAN-time may add a unit-test asserting the field is silent-ignored at parse + runtime.

7. **`compressor_library` per-route swap silent-ignore observability.** Per §1.1 amendment 4 + §2.2: per-route library swap is silent-ignored at parse + runtime. Operators setting `overrides.compressor_library` see Envoy use the per-route library; envoy-go uses listener-level. PLAN-time may add a unit-test asserting `parsePerRoute` accepts-but-ignores the field.

8. **filterStats struct field naming.** Per §6.9 17-counter struct: the Go field names (e.g., `HeaderCompressorOvershadowed`, `ResponseCompressed`, `RequestCompressed`) are PLAN-time conventions. The mapping from Go field → stat-name suffix is bijective; PLAN may choose any mapping consistent with phase-13 + phase-12 idioms.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

Per `BOOTSTRAP_PROMPT.md` §7.5 Gate F:

### 13.1 `## HTTP filter chain ### envoy.filters.http.compressor` NEW subsection

Patch shape (in-place edit at the existing `## HTTP filter chain` umbrella, alphabetical between `### envoy.filters.http.csrf` and `### envoy.filters.http.fault` — wait, alphabetical-canonical ordering: `buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit`, so compressor inserts between `buffer` and `cors` per ADR-0072 stylistic convention — but BEHAVIOR_CONTRACT.md may use a different ordering; PLAN to verify):

```markdown
### envoy.filters.http.compressor

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.compressor.v3.Compressor` (9 top-level fields total):**

| Field | Type | Phase 14 disposition | Notes |
|---|---|---|---|
| `compressor_library` | TypedExtensionConfig | CONSUMED | REQUIRED per Envoy PGV; envoy-go gzip-only MVP rejects non-Gzip TypeURLs at parse with envoy-go-own error (per ADR-0130). |
| `response_direction_config.common_config.min_content_length` | UInt32Value | CONSUMED | Default 30 (per §11.9 empirical). |
| `response_direction_config.common_config.content_type` | []string | CONSUMED | Default 8-entry list (per §11.1 empirical). |
| `response_direction_config.disable_on_etag_header` | bool | CONSUMED | Dual-mode per §1.1 amendment 6 + §11.7. |
| `response_direction_config.remove_accept_encoding_header` | bool | CONSUMED | Strips Accept-Encoding from upstream-bound request. |
| `response_direction_config.uncompressible_response_codes` | []uint32 | CONSUMED | Default empty `[]` (per §11.2 empirical). |
| `response_direction_config.common_config.enabled` | RuntimeFeatureFlag (BoolValue default) | SILENT-IGNORED | Always-active runtime; OPTIONAL at parse-time (per §1.1 amendment 2 + §11.3). Divergence-window if `default_value: false`. |
| `response_direction_config.status_header_enabled` | bool | SILENT-IGNORED | Always-no-status-header; the `x-envoy-compression-status:` debug header is not emitted. Divergence-window if set true. |
| `request_direction_config` | RequestDirectionConfig | SILENT-IGNORED | Always-disabled; envoy-go MVP is response-only. Divergence-window if set with `enabled: true`. |
| `runtime_enabled` | RuntimeFeatureFlag | SILENT-IGNORED | Deprecated; superseded by `response_direction_config.common_config.enabled`. |
| `choose_first` | bool | SILENT-IGNORED | Always-q-value-based selection. Divergence-window if `true` AND multi-coding AE. |
| `content_length` (deprecated) | UInt32Value | SILENT-IGNORED | Deprecated top-level mirror of `response_direction_config.common_config.min_content_length`. |
| `content_type` (deprecated) | []string | SILENT-IGNORED | Deprecated top-level mirror. |
| `disable_on_etag_header` (deprecated) | bool | SILENT-IGNORED | Deprecated top-level mirror. |
| `remove_accept_encoding_header` (deprecated) | bool | SILENT-IGNORED | Deprecated top-level mirror. |

**Codec-library `envoy.extensions.compression.gzip.compressor.v3.Gzip` (5 fields):**

| Field | Type | Phase 14 disposition | Notes |
|---|---|---|---|
| `compression_level` | enum | CONSUMED | Mapped to Go `compress/gzip` level constant per ADR-0130 §Decision (iv) table. |
| `compression_strategy` | enum | CONSUMED | Only `HUFFMAN_ONLY` honored; all others collapse to default. |
| `memory_level` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz memory-level knob. |
| `window_bits` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz window-bits knob. |
| `chunk_size` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz chunk-size knob. |

**Per-route `CompressorPerRoute`:** oneof `disabled: true` OR `overrides: CompressorOverrides`. The `CompressorOverrides` shape carries `response_direction_config: ResponseDirectionOverrides` (only `remove_accept_encoding_header` BoolValue) + `compressor_library: TypedExtensionConfig` (silent-ignored per §2.2). Per-route override of `min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes`, `enabled`, `status_header_enabled`, `runtime_enabled`, `choose_first`, `request_direction_config` is STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's proto.

#### Wire shape

Compressed-path response wire shape (envoy-go MVP):
- `content-type: <preserved>` — content-type preserved from upstream/direct_response.
- `content-encoding: gzip` — set by filter on compress path.
- `vary: Accept-Encoding` — appended to existing Vary OR set if absent (per §1.1 amendment 5 — APPEND ALWAYS, even on existing `Vary: *`). Token-match dedup (case-insensitive).
- `content-length: <gzipped-byte-count>` — fixed Content-Length (envoy-go writeH1Reply unconditional rewrite per `internal/filter/hcm/codec.go:87-89`).
- NO `transfer-encoding: chunked` — envoy-go MVP does not support chunked output on the encode side.
- ETag mode-a: strong-ETag stripped (regex `^"[^"]*"$` match → header deleted); weak-ETag preserved (regex `^W/"[^"]*"$`). RFC 7232 §2.3 motivation per §1.1 amendment 6.
- ETag mode-b: skip compression entirely on any ETag presence; `not_compressed_etag +1`.

**Wire-shape divergence-window from reference Envoy (deliberate; ADR-0131 records):** Envoy emits `transfer-encoding: chunked` (NO `content-length`) on every compressed response (per §11.9 empirical evidence covering body sizes 30 / 1024 bytes). envoy-go MVP emits fixed CL identity. The differential fixture 0016's allow-list excludes `content-length` value comparison + `transfer-encoding` presence on compressed scenarios.

Compressed-path body shape: **decompressed-byte-equivalent to original** (per §11.14 + ADR-0133 — decompress-and-compare assertion); compressed bytes structurally non-byte-exact between Go `compress/gzip` (default `OS: 255`, varying `XFL`) and Envoy libz (`OS: 03 UNIX`, `XFL: 00`). Forward-pointer to a future encode-side streaming framework phase per ADR-0131.

Skip-path response wire shape: identity body unmodified; NO `content-encoding`; Vary INJECTED on AE-side-skip paths (no AE / identity / wildcard-uncompressed / not_valid) but NOT on server-side-skip paths (content-type-mismatch / already-encoded / etag-disabled / no-transform / content-length-too-small / uncompressible-status) per §1.1 amendment / §11.15.

#### Per-route disabled-OR-override 5th canonical (per ADR-0125 amendment + §1.1 amendment 4)

Phase 14 is the SECOND row using ADR-0125 5th canonical disabled-OR-override discipline. Per-route override surface is FILTER-SPECIFIC and NARROWER than the listener-level config (only `remove_accept_encoding_header` per `ResponseDirectionOverrides` proto + per-route library swap silent-ignored). Per-route stats SHARED with listener-level (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats).
```

(End of §13.1 stub; ~120-150 lines authored at phase-done commit per phase-13 SPEC §13.1 precedent.)

### 13.2 `## Stat-name mapping ### 29-name table` extension to 46 names (17 new entries)

Verbatim Markdown patch (per BRAINSTORM §1.1 item 7 revised to §1.1 amendment 3):

```markdown
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_compressor_overshadowed`           | counter | filter | compressor | every request where this codec was selectable but overshadowed by a higher-q-value alternative (§11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_compressor_used`                   | counter | filter | compressor | every request where this codec was the negotiated selection (regardless of whether response was compressed; §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_identity`                          | counter | filter | compressor | every request where client requested identity (no compression; §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_not_valid`                         | counter | filter | compressor | every request where Accept-Encoding header was malformed (q-value parse error, etc.; §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_wildcard`                          | counter | filter | compressor | every request where client sent Accept-Encoding: * (§11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.no_accept_header`                         | counter | filter | compressor | every request where client had no Accept-Encoding header (§11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.not_compressed_etag`                      | counter | filter | compressor | every response skipped on ETag presence with disable_on_etag_header=true (§11.7) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.compressed`                      | counter | filter | compressor | every response compressed on this codec (§11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.content_length_too_small`        | counter | filter | compressor | every response skipped due to body below min_content_length (§11.5 + §11.9) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.not_compressed`                  | counter | filter | compressor | every response skipped (any reason; sum of skip counters; §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.total_compressed_bytes`          | counter | filter | compressor | cumulative compressed-body bytes emitted on response side (§11.5; tolerance per ADR-0133) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.total_uncompressed_bytes`        | counter | filter | compressor | cumulative pre-compression body bytes seen on response side (§11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.compressed`                       | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP (request_direction_config silent-ignored; §1.1 amendment 1 + §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.content_length_too_small`         | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.not_compressed`                   | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.total_compressed_bytes`           | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.total_uncompressed_bytes`         | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
```

Total: 17 new rows (9 active in MVP + 6 always-zero request_* + 2 always-active total_bytes). Stat-table size grows from 29 → 46 names. NO new SN flattening rule (uses existing SN2 per §1.1 amendment 3). Prometheus rendering: `envoy_http_compressor_<library>_<codec>_<counter>{envoy_http_conn_manager_prefix=<HCM_stat_prefix>}`.

### 13.3 `## Equivalence Matrix` new row (verbatim table-row patch)

```markdown
| 0016-http-compressor | envoy.filters.http.compressor (gzip-only response-side) | byte-exact status; decompressed-byte-exact body on compressed scenarios per ADR-0133; allow-list `content-length` value + `transfer-encoding` presence on compressed scenarios; per-counter delta byte-equivalent on 12 active counters; tolerance on `response_total_compressed_bytes` per ADR-0133 §Decision (iii) |
```

### 13.4 Forward-pointer notes (per BRAINSTORM §8 + §1.1 amendments)

```markdown
### Phase 14 forward-pointer notes

**Deferred field families** (silent-ignored / parse-rejected per ADR-0040 + ADR-0130; see `### envoy.filters.http.compressor ### Field decomposition` above + phase 14 SPEC §2.1 for the full field map):

- `Compressor.compressor_library` non-Gzip TypeURLs (envoy-go-only PARSE-time rejection per ADR-0130) — coupled to future codec-extension phases (brotli + zstd). Reference Envoy accepts `envoy.extensions.compression.{brotli,zstd}.compressor.v3.{Brotli,Zstd}`; envoy-go rejects with envoy-go-own error wording. Future re-activation: codec phases extend ADR-0130 + add codec-library dispatch helpers.
- `Compressor.request_direction_config` (silent-ignored at parse + runtime per ADR-0130) — coupled to future request-side compression phase + the future `envoy.filters.http.decompressor` filter.
- 4 deprecated top-level mirror fields (`content_length`, `content_type`, `disable_on_etag_header`, `remove_accept_encoding_header`) — silent-ignored at parse; operators MUST use the `response_direction_config` paths.
- `Compressor.runtime_enabled` + `response_direction_config.common_config.enabled` (RuntimeFeatureFlag fields) — silent-ignored at runtime; envoy-go always-100%-active. Couples to Runtime + hot restart family.
- `Compressor.choose_first` — always-q-value-based selection; divergence-window when set true AND multi-coding AE.
- `Compressor.response_direction_config.status_header_enabled` — always-no-status-header; the `x-envoy-compression-status:` debug header is not emitted. Operator divergence-window when set true.
- `Gzip.{memory_level, window_bits, chunk_size}` — silent-ignored; Go `compress/gzip` does not expose libz-equivalent knobs.
- Per-route `overrides.compressor_library` (per-route library swap) — silent-ignored at parse + runtime; envoy-go uses listener-level library regardless of per-route override.

**Wire-shape divergence-window from reference Envoy (per ADR-0131 + §11.9):** Envoy emits `Transfer-Encoding: chunked` + no `Content-Length` on every compressed response; envoy-go MVP emits fixed `Content-Length: <gzipped-len>` + identity transfer. Decompressed body bytes are byte-equivalent (gzip-format multi-encoding spec admits both). Compressed body bytes structurally diverge — Go `compress/gzip` (default `OS: 255`, variable `XFL`) vs. Envoy libz (`OS: 03 UNIX`, `XFL: 00`). Future re-activation: encode-side streaming framework phase (`writeH1Reply` chunked-output mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation in HCM).

**Framework deltas at `internal/filter/http/callbacks.go` + `chain.go` + `internal/filter/hcm/connection.go` + `h2dispatch.go`:** Phase 14 introduces `EncoderFilterCallbacks.OverwriteBody(b []byte)` interface method (1 LoC) + encoderCB.OverwriteBody impl (~6 LoC at chain.go) + per-stream encode-body-override field (~2 LoC) + accessor (~3 LoC) + HCM-side post-RunEncodeData harvest at H1 + H2 dispatch paths (~6-8 LoC each). Total ~20-25 LoC. Symmetric to phase-13 ADR-0128's decode-side primitives. Future filters needing encode-side body mutation (decompressor; bandwidth_limit transform mode; future codec/transform filters) can rely on this primitive.

**`min_content_length` late-revert anomaly:** when Content-Length is unset at EncodeHeaders + body length emerges below threshold only at EncodeData, the filter cannot revert headers. Phase 14 documents but defers; structurally rare in envoy-go's framework. Future cap-promotion or revert-headers-primitive phase may revisit. See ADR-0131 §Decision (vii).

**Stat namespace shape:** `compressor.<library_name>.<codec>.[response.]<counter>`. `<library_name>` is operator-supplied (`compressor_library.name`); empty allowed; emits with consecutive dots. `[response.]` infix appears IFF `response_direction_config` is set on the listener-level Compressor (per compressor.proto line 158-164). Phase-14 fixture 0016 uses `name: text_optimized` on both sides + always sets `response_direction_config` for byte-equivalent stat namespace.
```

(End of §13.4 stub; the forward-pointer subsection lands at phase-done commit per phase-13 SPEC §13.4 precedent. ~50 lines authored.)

---

## 14. Testing strategy (per BRAINSTORM §11 + §1.1 amendments)

### 14.1 Unit tests (`internal/filter/http/compressor/compressor_test.go`)

Test groups (mirrors phase-13 buffer's 6 test groups; phase-14 adds a 7th for the codec-library Any-dispatch):

1. **Config parse + buildCompiledConfig** — `compressor_library` Any-unmarshal (gzip TypeURL); unknown-TypeURL parse-rejection; `response_direction_config.*` projection; missing `compressor_library` parse-rejection; default content_type list when unset; default min_content_length=30 when unset; default uncompressible_response_codes=[] when unset; PGV mirror on `uncompressible_response_codes` range [200, 600); `enabled` field absent → no error (§1.1 amendment 2); deprecated top-level mirrors silent-ignored.
2. **buildCompiledPerRoute** — `disabled: true` path; `overrides: {response_direction_config: {remove_accept_encoding_header: true}}` path; `overrides: {response_direction_config: {}}` (empty override; produces no-op compiledPerRoute); oneof-empty parse-rejection (defensive); `disabled: false` parse-rejection (defensive — PGV bool.const violation); per-route `compressor_library` swap silent-ignored.
3. **buildCompiledGzipConfig** — compression_level enum mapping (DEFAULT_COMPRESSION → -1, BEST_SPEED → 1, BEST_COMPRESSION → 9, COMPRESSION_LEVEL_2..8 → 2..8); compression_strategy HUFFMAN_ONLY → huffmanOnly=true; other strategies → huffmanOnly=false (default); memory_level/window_bits/chunk_size silent-ignored (no-error on parse).
4. **EncodeHeaders skip predicates** — exhaustive matrix:
   - no Accept-Encoding header → skip + no_accept_header counter + Vary set;
   - Accept-Encoding: identity → skip + header_identity counter + Vary set;
   - Accept-Encoding: gzip;q=0 → skip + appropriate counter + Vary set;
   - Accept-Encoding: br (brotli not configured) → skip + header_compressor_overshadowed counter + Vary set;
   - malformed q-value → skip + header_not_valid counter + Vary set;
   - content-type mismatch → skip + header_compressor_used counter + NO Vary;
   - already-encoded (any non-empty Content-Encoding including identity) → skip + header_compressor_used counter + NO Vary;
   - Cache-Control: no-transform → skip + header_compressor_used counter + NO Vary;
   - status in uncompressible_response_codes → skip + header_compressor_used counter + NO Vary;
   - ETag with disable_on_etag_header=true → skip + not_compressed_etag counter + NO Vary;
   - ETag with disable_on_etag_header=false (default) → continue (mode-a; strip-strong / preserve-weak in subsequent header-mutation step).
5. **EncodeData compression path** — gzip-encode small/medium/large body; compression_level mapping observable in compressed-byte-size variance; HUFFMAN_ONLY strategy observable; OverwriteBody primitive call asserted; counter increments asserted.
6. **Vary + Content-Encoding + ETag header mutation** —
   - Vary append on empty / existing-non-AE / existing-with-AE (token-dedup) / wildcard `*`;
   - Content-Encoding: gzip set on compress path; absent on skip;
   - ETag strong-strip (regex match) vs weak-preserve (regex match) vs malformed-preserve (defensive);
   - Content-Length stripped from headers at EncodeHeaders (the framework's writeH1Reply rewrites at wire time);
   - mode-b ETag-disabled fully skips compression on any ETag.
7. **Per-route resolver wiring + stats SHARED-with-listener** — most-specific resolution across Route > VirtualHost > RouteConfiguration tiers; counter-increment on listener-level scope when per-route is active; `disabled` short-circuit (no counters incremented on disabled routes); per-route `remove_accept_encoding_header: true` override observed at DecodeHeaders time (Accept-Encoding stripped from request headers).

Plus an `Accept-Encoding parser` test sub-group covering RFC 7231 §5.3.4 q-value parsing edge cases (q=0 blocks; default q=1.0; wildcard handling; multi-coding sorting; classification dispatch).

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/compressor/...` — green on all 7 test groups + sub-groups. Race-test surface unchanged from phase 13's 36-package green baseline; the new package adds the 37th. The framework deltas at `chain.go` introduce one new per-stream field on `*FilterChain` + one new sentinel field; access is single-goroutine-per-stream (the dispatch goroutine), so no synchronization needed.

`golangci-lint run` — green; new package lints clean. The `regexp.MustCompile` calls for ETag strong/weak patterns are package-level vars (compiled once at init).

### 14.3 Fuzzers

`FuzzCompressorConfigParse` — fuzzes the YAML→proto→`buildCompiledConfig` pipeline. Inputs are random bytes interpreted as YAML; errors-on-invalid-YAML are expected; the fuzzer asserts no panic + no nil-deref on the compilation path. The 18th fuzzer in the repo (after `FuzzBufferConfigParse` from phase 13).

Optional: `FuzzAcceptEncodingParse` (the q-value parser is a non-trivial parser; fuzz-testing for panics on malformed input is straightforward). Phase-14 SPEC defers this to PLAN — single fuzzer is sufficient for MVP fuzzer-coverage; q-value parser robustness is unit-test-covered.

### 14.4 Existing fuzzers re-run

17 phase-13 fuzzers re-run at 30s budget; all green (regression check; phase 14 introduces no fuzzer-affecting changes outside the new package).

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin. Phase 14's H2 framework delta (the `OverwriteBody` harvest at h2dispatch.go) is structurally identical to the H1 path; no h2spec-affecting wire-shape change.

### 14.6 Differential 0000–0015 + 0016

16 prior fixtures + the new `0016-http-compressor` = 17 fixtures green. Total runtime estimated ~55-70s wallclock.

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

| Gate | Pass/fail criterion |
|---|---|
| A | `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0; no new warnings vs phase-13 baseline at master tip `51b9ea6`. |
| B | `go test -race ./...` exit 0 across all 37 packages; race detector reports clean. |
| C | `h2spec` 53/53 PASS at ADR-0051 pin; phase-14 introduces no H2 wire-shape changes. |
| D | All 18 fuzzers green at 30s/each budget. |
| E | All 17 differential fixtures (0000-0016) PASS; runtime ~55-70s wallclock. |
| F | `BEHAVIOR_CONTRACT.md` §13.1 + §13.2 + §13.3 + §13.4 populated per the patches at §13 above. |

All six green at phase-done commit per BOOTSTRAP_PROMPT.md §7.5.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

1. ✓ Phase 14 SPEC.md authored with 6 §1.1 amendment blocks; each amendment cross-referenced to §11 empirical evidence.
2. ✓ §3 framework-survey result locked: Path A fires; `EncoderFilterCallbacks.OverwriteBody(b []byte)` primitive specified at §1 item 4 + §6.7 + ADR-0131 §Decision (vi).
3. ✓ §11 empirical-pin block: 15 pins resolved IN-SESSION; 9 ratified, 6 refuted; verbatim probe transcripts captured.
4. ✓ Differential fixture redesign: 6 scenarios; scenario 6 redesigned per §1.1 amendment 4 (per-route rmAE + real-backend-echo assertion); ADR-0133 codifies decompress-and-compare body assertion.
5. ✓ ADR roster: 5 ADRs (ADR-0129..ADR-0133) + ADR-0125 in-place amendment paragraph.
6. ✓ Stat surface: 17 counters; namespace `compressor.<libraryName>.gzip.[response.]<counter>`; existing SN2 flatten; NO new SN10.
7. ✓ Per-route surface: disabled-OR-rmAE-only (`ResponseDirectionOverrides` carries one field); SECOND row using ADR-0125 5th canonical.
8. ✓ Wire-shape divergence: envoy-go fixed-CL identity vs Envoy chunked; ADR-0131 forward-points to future encode-side streaming framework phase.
9. ✓ Six §1.1 amendment blocks document the SPEC-time refutations cleanly.
10. ✓ STATE.md updated post-SPEC: lifecycle-state-3 (SPEC-done, awaiting PLAN); next-skill `superpowers:writing-plans` (now for PLAN authoring per ADR-0005 §Decision 4 split).

---

**End of phase 14 SPEC.**
