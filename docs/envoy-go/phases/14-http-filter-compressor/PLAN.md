# Phase 14 — HTTP filter `envoy.filters.http.compressor` (`internal/filter/http/compressor/`, differential fixture `0016-http-compressor`, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.compressor` extension, `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework primitive, `test/helpers/echobackend/` new shared helper) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory user preference) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.compressor` — Envoy v1.37.2's canonical "gzip-compress upstream response body before forwarding downstream" filter (gzip-only MVP, response-side only) — as the SEVENTH production HTTP filter in envoy-go, with byte-equivalent wire outcomes against reference Envoy on every observable axis EXCEPT the deliberately allow-listed `content-length` value + `transfer-encoding` presence axis on compressed responses (envoy-go: fixed-CL identity vs. Envoy: chunked) and the compressed-bytes axis (gzip-format multi-encoding spec admits both Go `compress/gzip` and libz outputs as valid; decompressed bytes are byte-equivalent per ADR-0133), under the 07.1 framework, with ONE new framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` (~20-25 LoC across `callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go` per ADR-0131 §Decision (vi)).

**Architecture:** New `internal/filter/http/compressor/` package owning the filter implementation; ENCODER+DECODER `HTTPFilter` value with SAME `*filter` instance servicing both sides (FIRST §9 row to use `Decoder: f, Encoder: f` with non-vacuous both paths structurally; per ADR-0129 §Decision (iv)); body algorithm Path B (buffer-then-compress) per ADR-0131 §Decision (i) — `EncodeData` on `f.willCompress=true + endStream=true + len(data) >= min_content_length` gzip-encodes via `gzip.NewWriterLevel(buf, level).Write(data).Close()` and emits via the new `cb.OverwriteBody(buf.Bytes())` primitive; `DecodeHeaders` parses Accept-Encoding (RFC 7231 §5.3.4 q-value parser) → caches negotiated encoding + classification on per-stream filter state + resolves per-route TPFC + maybe-strips `Accept-Encoding` from upstream-bound request when effective `remove_accept_encoding_header == true`; `EncodeHeaders` runs the 11-bucket skip-decision sequence (per §6.6 + §11.15 trichotomy: compress / AE-side-skip injects Vary / server-side-skip does NOT inject Vary) + on compress path mutates response headers (set `Content-Encoding: gzip`; append `Vary: Accept-Encoding` UNCONDITIONALLY even on existing `Vary: *` per §11.10; mode-a strong-ETag strip via `^"[^"]*"$` regex / weak-ETag preserve via `^W/"[^"]*"$` regex per §11.7 + §1.1 amendment 6; strip Content-Length so `writeH1Reply` rewrites at wire time); per-route `CompressorPerRoute` proto carries `oneof override` with `disabled: true` shortcut OR `overrides.response_direction_config.remove_accept_encoding_header` BoolValue (5th canonical disabled-OR-override per ADR-0125 §amendment + ADR-0125 §Decision (vi); SECOND row using); per-route override field surface FILTER-SPECIFIC and NARROWER than listener-level (per ADR-0125 amendment §(viii) — `min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes` per-route overrides are STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's `ResponseDirectionOverrides` proto); 17-counter `filterStats` struct registered at `New` factory time per HCM stat_prefix (12 active in MVP + 5 always-zero `request_*` since `request_direction_config` silent-ignored; per ADR-0132 §Decision (i)) under namespace `compressor.<libraryName>.gzip.[response.]<counter>` flattened via existing Rule SN2 (NO new SN10 rule per §1.1 amendment 3); codec-library `compressor_library` Any-unmarshal-and-dispatch in `New` factory rejects non-Gzip TypeURLs at parse-time with envoy-go-own error wording per ADR-0130 §Decision (ii)-(iii); differential fixture 0016 single-listener with 6 routes (4 direct_response + 1 cluster + 1 disabled per SPEC §7.2) using NEW shared `test/helpers/echobackend/` for scenario 6's per-route rmAE assertion; decompress-and-compare body-assertion discipline per ADR-0133 (driver dispatches on response `Content-Encoding` header — byte-exact for uncompressed; decompressed-byte-exact for `Content-Encoding: gzip`; `response_total_compressed_bytes` boundary-only `0 < value < uncompressed_input_bytes` tolerance per planner-time decision settling SPEC §12 deferred 2).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008); `protojson.Unmarshal` for `CompressorPerRoute` oneof discipline; `compress/gzip` Go stdlib for the codec library (gzip-only MVP per ADR-0130); `regexp` Go stdlib for ETag strong/weak pattern match; reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per ADR-0008 + ENVOY_TARGET.md); golangci-lint 1.64.8 (ADR-0009 pin); Docker for differential harness; HTTP/1.1 plaintext fixture (no H2 differential coverage per SPEC §2.5).

---

## Scope check — why phase 14 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 PLAN's component-table convention):

- `internal/filter/http/compressor/doc.go` ~30
- `internal/filter/http/compressor/compressor.go` ~380–440 (filter + factory + types + DecodeHeaders + DecodeData + DecodeTrailers + EncodeHeaders + EncodeData + EncodeTrailers + OnDestroy + Set{Decoder,Encoder}Callbacks + compiledConfig + compiledPerRoute + compiledGzipConfig + filterStats + parsePerRoute + codec-library Any-unmarshal-and-dispatch + maybeStripStrongEtag + appendVaryAcceptEncoding + computeSkipReason + effectiveConfig)
- `internal/filter/http/compressor/acceptencoding.go` ~100–130 (q-value parser + classification dispatch per RFC 7231 §5.3.4; per planner-time decision 1 file split)
- `internal/filter/http/compressor/compressor_test.go` ~750–900 (8 unit-test groups per planner-time-extended SPEC §14.1: Group 1 config + Group 2 perroute + Group 3 codec + Group 4 AE-parser + Group 5 EncodeHeaders skip + Group 6 EncodeData + Group 7 Vary/ETag/CE mutators + Group 8 stats namespace)
- `internal/filter/http/compressor/fuzz_test.go` ~80 (18th fuzzer in repo)
- `internal/filter/http/callbacks.go` +1 LoC (`OverwriteBody(b []byte)` interface method on `EncoderFilterCallbacks`)
- `internal/filter/http/chain.go` ~+8 LoC (`encoderCB.OverwriteBody` impl + new `c.encodeBodyOverride []byte` per-stream field + `c.encodeBodyOverridden bool` sentinel + `*FilterChain.EncodeBodyOverride() ([]byte, bool)` accessor)
- `internal/filter/http/chain_test.go` ~+60 LoC (probe-filter-driven OverwriteBody primitive integration tests covering: probe sets override → harvest returns the bytes; probe does not set → harvest returns `(nil, false)`; passthrough behavior preserved per ADR-0131 §Decision (vi))
- `internal/filter/hcm/connection.go` ~+8 LoC (post-`RunEncodeData` harvest of `chain.EncodeBodyOverride()` + `resp.Body` substitution before `writeH1Reply`)
- `internal/filter/hcm/h2dispatch.go` ~+8 LoC (post-`RunEncodeData` harvest + `resp.Body` substitution before `writeH2Reply`; symmetric to H1 path)
- `cmd/envoy-go/main.go` +1 import line + 1 register line ~+3 (`httpReg.Register(compressor.TypeURL, compressor.New)` inserted alphabetical-after-buffer per ADR-0129 §Decision (v))
- `test/helpers/echobackend/` (NEW DIRECTORY; planner-time decision 6) — `echobackend.go` ~80 + `doc.go` ~25 + `echobackend_test.go` ~80 = ~185
- `test/fixtures/0016-http-compressor/` (NEW DIRECTORY) — `envoy.yaml` ~95 + `envoy-go.yaml` ~95 + `expectations.yaml` ~50 + `README.md` ~85 + `inputs/driver.go` ~220 = ~545
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPCompressor BackendKind = 13`) + doc-comment ~+15
- `test/differential/runner_test.go` blank-import addition + new `startEchoBackend` spawn helper + switch case ~+25
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.compressor` subsection ~140 + §13.2 stat-table 29→46 names extension (17 new rows) ~25 + §13.3 equivalence-matrix row ~3 + §13.4 `### Phase 14 forward-pointer notes` subsection ~55 = ~+225
- `docs/envoy-go/ROADMAP.md` row `14` `in-progress → done` flip + summary sharpening (post-amendment counts) ~+1 net
- `docs/envoy-go/STATE.md` advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` (NEW; lifecycle artefact) ~600
- `docs/envoy-go/phases/14-http-filter-compressor/REVIEW.md` (NEW; lifecycle artefact) ~180

**Production code: ~480–570 LoC (filter impl in `compressor.go` + `acceptencoding.go`) + ~25 LoC framework deltas across 4 files (`callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go`) + ~3 LoC main.go + ~80 LoC echobackend helper = ~588–678 LoC production + ~830 LoC tests (~750-900 unit + 80 fuzzer + ~60 chain-integration) + ~545 LoC fixture YAML/Go + ~185 LoC echobackend helper + ~785 LoC docs ≈ ~2225-2435 LoC total** (production-only ~588–678 LoC, well below the ADR-0045 ~1500 LoC threshold). Both ADR-0045 thresholds — ~25 tasks AND ~1500 LoC of production code — are well under (production ~588–678 LoC; task count below is **16**, comfortably under the 25 limit). The 5 anticipated ADRs (ADR-0129..ADR-0133) all have their `Lands-in-task` anchors set in the table at `## ADRs introduced by this plan` below; ADR-0125 amendment paragraph (§(viii)-(x)) ALREADY landed at the SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent (no PLAN-time re-anchor needed). SPEC §1.3 (per BRAINSTORM Decisions 9 + ADR-0106) settled the family-expansion shape as flat top-level rows; phase 14 is a SINGLE coherent row, no parent-and-sub-phases split. STATE.md `next-skill-scope` projected ~14–18 tasks; this PLAN lands at **16 tasks** (mid-bound — driven by compressor's larger surface than buffer at every axis: 6 listener fields consumed vs 1 + codec-library Any-dispatch + AE q-value parser + ETag mode-a mutator + Vary trichotomy + 17-counter filterStats vs 0 + framework primitive + 6-scenario fixture vs 6 with new shared echobackend helper).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 / SPEC §1.4 would be `14.1 = listener-level filter MVP (Tasks 1–9) + framework primitive + 4 listener-only fixture scenarios` and `14.2 = per-route disabled-OR-rmAE (Tasks 10–16) + 2 per-route fixture scenarios + ETag-mutator + remove_accept_encoding_header backend-echo`; SPEC §1.4 explicitly rejects the split since both halves stay well under the LoC threshold and the per-route discipline is a small extension of the listener-level work (data-only + shared stats + filter-internal helpers). PLAN concurs and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/compressor/doc.go` | NEW | Package doc enumerating: (a) the typed_config surface (`Compressor` proto with **6 listener-level fields actively consumed** per SPEC §1.1 amendment 1 — `compressor_library` (TypedExtensionConfig REQUIRED; PGV-mirror) + `response_direction_config.{common_config.{min_content_length, content_type}, disable_on_etag_header, remove_accept_encoding_header, uncompressible_response_codes}`; **9 listener-level fields silent-ignored** — 4 deprecated top-level mirrors (`content_length`, `content_type`, `disable_on_etag_header`, `remove_accept_encoding_header`) + `runtime_enabled` (deprecated runtime gate) + `choose_first` (always-q-value) + `request_direction_config` (always-disabled response-only MVP) + `response_direction_config.common_config.enabled` (RuntimeFeatureFlag with BoolValue default; OPTIONAL at parse per §1.1 amendment 2 + §11.3) + `response_direction_config.status_header_enabled` (`x-envoy-compression-status:` debug header opt-in; always-no-status-header per §1.1 amendment 1); **1 case parse-rejected** — `compressor_library.typed_config` with non-Gzip TypeURL (envoy-go-only validation; envoy-go-own error wording per ADR-0130 §Decision (vi)); **codec-library `Gzip` proto** with 2 fields consumed (`compression_level` mapped to Go `compress/gzip` levels per ADR-0130 §Decision (iv); `compression_strategy` HUFFMAN_ONLY honored / others collapse to default per §Decision (v)) + 3 silent-ignored (`memory_level`, `window_bits`, `chunk_size` — Go gzip does not expose libz-equivalent knobs); **`CompressorPerRoute` proto** with `oneof override` carrying `disabled: true` shortcut OR `overrides: CompressorOverrides`; `CompressorOverrides` carries `response_direction_config: ResponseDirectionOverrides` (only `remove_accept_encoding_header` BoolValue per §1.1 amendment 4 — per-route override of `min_content_length`/`content_type`/`disable_on_etag_header`/`uncompressible_response_codes`/`enabled`/`status_header_enabled` is STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's proto) + `compressor_library: TypedExtensionConfig` (per-route library swap; silent-ignored at parse + runtime per §1.1 amendment 4); (b) the public API surface (`TypeURL` const, `New` HTTPFilterFactory); (c) the iteration-protocol coverage (DECODER-side: `DecodeHeaders` parses Accept-Encoding via the q-value parser + caches `acceptedEncoding` + `acceptHeaderClassification` + resolves per-route TPFC + sets `passthrough` flag if disabled + maybe-strips `Accept-Encoding` from request when effective rmAE; `DecodeData` / `DecodeTrailers` pass-through; ENCODER-side: `EncodeHeaders` runs 11-bucket skip-decision sequence + Vary trichotomy + ETag mode-a strong-strip / weak-preserve + Content-Encoding set + Content-Length strip; `EncodeData` on compress path gzip-encodes via `gzip.NewWriterLevel(buf, level)` + emits via `cb.OverwriteBody(buf.Bytes())`; `EncodeTrailers` pass-through; ENCODER+DECODER `HTTPFilter` value sets `Decoder: f, Encoder: f` SAME instance per planner-time decision 5 + ADR-0129 §Decision (iv) — FIRST §9 row to use this shape with non-vacuous both paths); (d) the per-route discipline (per ADR-0073 wholesale-override + ADR-0125 5th canonical disabled-OR-override sum-type + ADR-0125 amendment §(viii) — per-route override surface FILTER-SPECIFIC and NARROWER than listener-level + SHARED stats per §(ix) — 17 counters live at listener-level scope; per-route-active routes increment listener-level counters; disabled-true routes increment NOTHING); (e) the body algorithm — Path B per ADR-0131 §Decision (i) — buffer-then-compress in one shot via the framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` (per ADR-0131 §Decision (vi) — the FIRST encode-side framework primitive in envoy-go; ~20-25 LoC across `callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go`; symmetric to phase-13 ADR-0128 decode-side primitives); (f) the wire-shape divergence-window from reference Envoy (envoy-go: `Content-Encoding: gzip` + fixed `Content-Length: <gzipped-len>` + identity transfer; Envoy: `Content-Encoding: gzip` + `Transfer-Encoding: chunked` + no CL — confirmed at SPEC §11.9 across body sizes 30 / 1024 bytes; deliberate per ADR-0131 §Decision (ii); decompressed body bytes are byte-equivalent — gzip multi-encoding spec admits both Go `compress/gzip` and libz outputs as valid per §11.14); (g) the cross-cutting ADR anchors (ADR-0125 amendment §(viii)-(x) / ADR-0129 / ADR-0130 / ADR-0131 / ADR-0132 / ADR-0133). Mirrors `internal/filter/http/buffer/doc.go`-style structure (~25-36 LoC precedent extended to ~30 LoC for the heavier scope). Per SPEC §4.1. |
| `internal/filter/http/compressor/compressor.go` | NEW | Filter implementation — main file per planner-time decision 1 (2-way split: `compressor.go` + `acceptencoding.go`; the AE q-value parser is the most-self-contained primitive and benefits most from isolation). **Public surface (per SPEC §6.1):** `TypeURL` string constant (`"type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"`); `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory matching `envoyhttp.HTTPFilterFactory`. **Internal package consts:** `filterName = "envoy.filters.http.compressor"`; `gzipLibraryTypeURL = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"`. **Unexported types (per SPEC §6.2):** `compiledConfig` struct (6 fields per §6.2 + ADR-0130 §Decision (i): `libraryName string` (empty allowed; embedded in stat namespace per ADR-0132 §Decision (v)) + `gzip *compiledGzipConfig` (gzip-only MVP; non-nil) + `minContentLength uint32` (default 30 per §11.9) + `contentTypes []string` (default 8-entry list per §11.1; lowercase prefix-matched per §11.6) + `disableOnEtagHeader bool` (default false; dual-mode per §1.1 amendment 6) + `removeAcceptEncodingHeader bool` (default false) + `uncompressibleResponseCodes map[uint32]struct{}` (default empty per §11.2; valid range [200, 600) per PGV)); `compiledGzipConfig` struct (2 fields per §Decision (i): `level int` (mapped from Envoy enum per ADR-0130 §Decision (iv)) + `huffmanOnly bool` (true if `compression_strategy == HUFFMAN_ONLY` per §Decision (v))); `compiledPerRoute` struct (2 fields per §6.2 + ADR-0125 amendment §(viii): `disabled bool` — exclusive with override; true → filter wholly inactive; `removeAcceptEncodingHeaderOverride *bool` — exclusive with disabled; nil unless override set; pointer to discriminate from "unset"); `filter` struct (8 fields: `config *compiledConfig` (closure-captured listener-level reference) + `stats *filterStats` (closure-captured listener-level pointer; SHARED with per-route per ADR-0132 §Decision (iv)) + `dcb envoyhttp.DecoderFilterCallbacks` (set by SetDecoderCallbacks) + `ecb envoyhttp.EncoderFilterCallbacks` (set by SetEncoderCallbacks) + `acceptedEncoding string` (per-stream parsed AE — "gzip" or "" or "identity" etc.) + `acceptHeaderClassification string` (per-stream classification ∈ {compressor_used, overshadowed, identity, wildcard, no_accept_header, not_valid}) + `perRoute *compiledPerRoute` (per-stream resolved per-route config) + `passthrough bool` (per-stream disabled flag) + `willCompress bool` (per-stream EncodeHeaders→EncodeData signal)); `filterStats` struct (17 fields per ADR-0132 §Decision (i) + SPEC §6.9 — 6 header_* + 1 not_compressed_etag + 5 response_* + 5 request_* — see filterStats GoDoc below for full enumeration). **Helpers (per SPEC §6.x):** `unmarshalCompressorLibrary(library *corev3.TypedExtensionConfig) (*compiledGzipConfig, libraryName string, error)` (codec-library Any-unmarshal-and-dispatch per ADR-0130 §Decision (ii) — checks library != nil → checks TypeURL == gzipLibraryTypeURL → UnmarshalTo &gzipPB → buildCompiledGzipConfig); `buildCompiledGzipConfig(g *gzipv3.Gzip) (*compiledGzipConfig, error)` (level enum mapping per ADR-0130 §Decision (iv); strategy mapping per §Decision (v); silent-ignore memory_level/window_bits/chunk_size); `parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error)` (oneof discipline per §6.3 + §11.13 — handles `*CompressorPerRoute_Disabled` (defensive PGV bool.const mirror), `*CompressorPerRoute_Overrides` (extracts overrides.response_direction_config.remove_accept_encoding_header BoolValue → *bool; silent-ignores overrides.compressor_library per §1.1 amendment 4), nil PGV-required mirror, wrong-type assertion guard); `(f *filter) effectiveConfig() *compiledConfig` (returns shallow-clone with rmAE overridden if perRoute.removeAcceptEncodingHeaderOverride != nil; else returns listener-level *config; per §6.6 effectiveConfig helper); `(f *filter) computeSkipReason(headers http.Header, effective *compiledConfig, endStream bool) string` (returns one of: "" (no skip), "no_accept_header", "identity", "wildcard_uncompressed", "not_valid", "uncompressible_status", "already_encoded", "etag_disabled", "no_transform", "content_type_mismatch", "content_length_too_small_known"; the 11-bucket skip-decision sequence per §6.6 + §11.15); `appendVaryAcceptEncoding(headers http.Header)` (per §1.1 amendment 5 + §11.10 + §6.6 — token-match dedup case-insensitive; APPEND ALWAYS even on existing `Vary: *`); `maybeStripStrongEtag(headers http.Header)` (per §1.1 amendment 6 + §11.7 + §6.6 — package-level vars `strongEtagPattern = regexp.MustCompile(\`^"[^"]*"$\`)` + `weakEtagPattern = regexp.MustCompile(\`^W/"[^"]*"$\`)`; on strong match → headers.Del("ETag"); on weak match → preserve; on malformed → preserve verbatim defensive); `newFilterStats(reg *stats.Registry, hcmStatPrefix string, libraryName string) *filterStats` (registers 17 counters under `compressor.<libraryName>.gzip.[response.]<counter>` path per ADR-0132 §Decision (i)); package-level var `defaultContentTypes = []string{"application/javascript", "application/json", "application/xhtml+xml", "image/svg+xml", "text/css", "text/html", "text/plain", "text/xml"}` (8-entry default per §11.1; case-insensitive prefix-matched). **DecodeHeaders body** (per SPEC §6.4 + §11.8 + §1.1 amendment 4): step 1 parse Accept-Encoding via `parseAcceptEncoding(headers.Get("Accept-Encoding"))` → cache `f.acceptedEncoding` + `f.acceptHeaderClassification`; step 2 resolve per-route via `f.dcb.RequestRouteConfig().(*compiledPerRoute)` → cache `f.perRoute`; step 3 if perRoute != nil && perRoute.disabled → set `f.passthrough = true` + return `Continue` (no AE strip on disabled-route — wholly inactive); step 4 compute effective rmAE (perRoute override wins over listener-level); step 5 if effectiveRmAE → `headers.Del("Accept-Encoding")`; step 6 return `Continue`. **DecodeData / DecodeTrailers**: pass-through (`DataContinue` / `TrailersContinue`). **EncodeHeaders body** (per SPEC §6.6 + §11.5 + §11.10 + §11.15 + §1.1 amendments 5-6): step 1 if f.passthrough → return `Continue` (no header mutation; no counter); step 2 compute effective config; step 3 compute skipReason; step 4 increment header_* / not_compressed_etag / response_content_length_too_small / response_not_compressed counters per the §6.6 switch; step 5 on AE-side-skip paths inject Vary; on server-side-skip paths do NOT inject Vary (per §11.15 trichotomy + §1.1 amendment 5); on skip → return `Continue`; step 6 on compress path: set Content-Encoding=gzip + appendVaryAcceptEncoding + maybeStripStrongEtag (mode-a; if !effective.disableOnEtagHeader) + Content-Length strip + f.willCompress=true + return `Continue`. **EncodeData body** (per SPEC §6.7 + §11.9 + §11.14): step 1 if passthrough OR !willCompress → return `DataContinue`; step 2 if !endStream → defensive `DataContinue` (current framework always invokes once with endStream=true; defensive for future); step 3 late min_content_length gate — if uint32(len(data)) < effective.minContentLength → increment response_content_length_too_small + response_not_compressed + DataContinue (late-revert anomaly; structurally rare per §6.7 + ADR-0131 §Decision (vii); fixture 0016 sidesteps via direct_response routes carrying CL on action headers); step 4 gzip-encode in one shot via `gzip.NewWriterLevel(&buf, f.config.gzip.level).Write(data).Close()`; step 5 emit via `f.ecb.OverwriteBody(buf.Bytes())`; step 6 increment response_compressed + response_total_uncompressed_bytes(+len(data)) + response_total_compressed_bytes(+len(compressed)); step 7 return `DataContinue`. **EncodeTrailers**: pass-through. **OnDestroy** (per §6.8): no-op (no per-stream resources; the chain.go `c.encodeBodyOverride` field dies with the chain). **SetDecoderCallbacks(cb)** stores `f.dcb = cb`; **SetEncoderCallbacks(cb)** stores `f.ecb = cb` (BOTH sides per planner-time decision 5; SAME *filter instance services both per ADR-0129 §Decision (iv)). ~380-440 LoC. |
| `internal/filter/http/compressor/acceptencoding.go` | NEW | Accept-Encoding q-value parser per RFC 7231 §5.3.4 + classification dispatch per planner-time decision 1 file split. **Public-within-package surface:** `parseAcceptEncoding(header string) (selected string, classification string)` — the entry-point invoked from `(f *filter) DecodeHeaders` step 1. **Algorithm:** (1) if header == "" → return ("", "no_accept_header"); (2) parse comma-separated coding entries; for each entry split on `;` → coding-token + parameters; q-value parameter parsed as float in [0.0, 1.0]; default q=1.0 if absent; reject malformed q-value (non-numeric / out-of-range) → return ("", "not_valid"); (3) build sorted-by-q-value-desc list of (coding, qValue) pairs; ties broken by declared order; (4) walk the list: first selectable coding (q > 0 AND coding ∈ {"gzip", "*"}) → return ("gzip", classification); coding=="identity" first non-zero-q → return ("", "identity"); coding=="*" with q > 0 → return ("gzip", "wildcard"); no selectable coding (all q=0 or all unconfigured) → return ("", "overshadowed") if any non-empty entries with q>0 were present and matched neither gzip nor wildcard; else ("", "no_accept_header") fallback. **Classification dispatch (per SPEC §6.4 + §11.5 verbatim probeA evidence):** the 6 classifications match the 6 `header_*` counter names verbatim — `compressor_used` (codec selected via gzip; explicit gzip;q>0); `overshadowed` (codec was selectable but a non-configured codec had higher q OR the AE list contained recognized codings none of which were gzip or wildcard with q>0); `identity` (identity selected — explicit identity entry with q>0 outranks gzip OR identity-only AE); `wildcard` (wildcard `*` selected gzip; no explicit gzip in AE); `no_accept_header` (empty AE header); `not_valid` (q-value parse error or other malformed token). **Helper sub-functions:** `parseQValue(s string) (float64, bool)` (parses `q=<float>` form; returns (q, ok)); `parseEncoding(s string) (token string, q float64, valid bool)` (parses one coding+parameters chunk; lowercases the token); `selectByQValue(parsed []encodingEntry) (selected string, classification string)` (walks the sorted list applying the dispatch rules). **No exported names** (entry point is package-local). **Test surface (Group 4 in `compressor_test.go`):** ~25 unit-test cases per SPEC §14.1 final paragraph covering: empty AE; gzip explicit; identity; wildcard; gzip;q=0.5,br;q=1.0 (br not configured → gzip wins despite lower q); gzip;q=0 blocks; malformed q (`q=blah`); multi-coding sorting; case-insensitive token matching; whitespace tolerance; trailing semicolon edge cases. ~100-130 LoC. |
| `internal/filter/http/compressor/compressor_test.go` | NEW | Unit tests per SPEC §14.1 (extended to 8 groups: 7 SPEC-named groups + 1 stats-namespace integration sub-group surfaced at planner-time per planner-time decision 8) covering: **Group 1 — `New` factory + buildCompiledConfig PGV-mirror (per §6.1 + §11.1 + §11.2 + §11.3 + §11.6):** `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_MissingCompressorLibrary_RejectAtParseTime`, `TestNew_NonGzipLibrary_RejectAtParseTime` (brotli + zstd + made-up TypeURL), `TestNew_GzipLibrary_Accepted`, `TestNew_DefaultMinContentLength_Is30`, `TestNew_ExplicitMinContentLength`, `TestNew_DefaultContentTypes_Is8EntryList`, `TestNew_ExplicitContentTypes_OverridesDefault`, `TestNew_DefaultUncompressibleResponseCodes_IsEmpty`, `TestNew_UncompressibleResponseCodes_OutsideRange_RejectsAtParseTime` (199, 600, 800), `TestNew_UncompressibleResponseCodes_DuplicateValues_RejectsAtParseTime`, `TestNew_EnabledFieldAbsent_NoError` (§1.1 amendment 2), `TestNew_EnabledFieldPresent_Stored_RuntimeIgnored`, `TestNew_StatusHeaderEnabled_True_SilentIgnored` (per planner-time decision settling SPEC §12 deferred 6), `TestNew_DeprecatedTopLevelMirrors_SilentIgnored` (4 fields: content_length, content_type, disable_on_etag_header, remove_accept_encoding_header), `TestNew_RuntimeEnabled_SilentIgnored`, `TestNew_ChooseFirst_SilentIgnored`, `TestNew_RequestDirectionConfig_SilentIgnored`. **Group 2 — `parsePerRoute` PGV-mirror discipline (per §6.3 + §11.13):** `TestParsePerRoute_Disabled_Parses`, `TestParsePerRoute_Overrides_RmAE_True_Parses`, `TestParsePerRoute_Overrides_RmAE_False_Parses`, `TestParsePerRoute_Overrides_Empty_Parses_NoOpEffect`, `TestParsePerRoute_Overrides_LibrarySwap_SilentIgnored` (per planner-time decision settling SPEC §12 deferred 7), `TestParsePerRoute_OneofUnset_Rejects` (PGV-required mirror), `TestParsePerRoute_DisabledFalse_Rejects` (PGV bool.const mirror; structurally unreachable post-decode but defensively covered), `TestParsePerRoute_WrongType_Rejects` (Go-side type assertion guard). **Group 3 — `buildCompiledGzipConfig` codec mapping (per ADR-0130 §Decision (iv)-(v)):** `TestBuildGzipConfig_DefaultLevel`, `TestBuildGzipConfig_BestSpeed`, `TestBuildGzipConfig_BestCompression`, `TestBuildGzipConfig_Levels2to8`, `TestBuildGzipConfig_HuffmanOnly_StrategyHonored`, `TestBuildGzipConfig_OtherStrategies_CollapseToDefault`, `TestBuildGzipConfig_MemoryLevelWindowBitsChunkSize_SilentIgnored`. **Group 4 — Accept-Encoding parser (per SPEC §6.4 + §11.4):** `TestParseAcceptEncoding_Empty`, `TestParseAcceptEncoding_GzipExplicit`, `TestParseAcceptEncoding_Identity`, `TestParseAcceptEncoding_Wildcard`, `TestParseAcceptEncoding_MultiCodingSortedByQValue` (gzip;q=0.5, br;q=1.0 → gzip wins because br not configured), `TestParseAcceptEncoding_GzipQ0_Blocks`, `TestParseAcceptEncoding_MalformedQValue_NotValid`, `TestParseAcceptEncoding_BrOnly_Overshadowed` (br configured? No — phase-14 MVP gzip-only; "overshadowed" means recognized-but-not-configured), `TestParseAcceptEncoding_CaseInsensitiveToken`, `TestParseAcceptEncoding_WhitespaceTolerance`, `TestParseAcceptEncoding_TrailingSemicolon`, `TestParseAcceptEncoding_MultipleEntriesSameCoding`. **Group 5 — `EncodeHeaders` skip-decision matrix (per §6.6 + §11.15):** `TestEncodeHeaders_NoAcceptHeader_Skip_VarySet_NoAcceptHeaderCounter`, `TestEncodeHeaders_AEIdentity_Skip_VarySet_HeaderIdentityCounter`, `TestEncodeHeaders_AEGzipQ0_Skip_VarySet`, `TestEncodeHeaders_AEBrUnconfigured_Skip_VarySet_HeaderOvershadowed`, `TestEncodeHeaders_AEMalformed_Skip_VarySet_HeaderNotValid`, `TestEncodeHeaders_ContentTypeMismatch_Skip_NoVary_HeaderCompressorUsed`, `TestEncodeHeaders_AlreadyEncodedGzip_Skip_NoVary`, `TestEncodeHeaders_AlreadyEncodedDeflate_Skip_NoVary`, `TestEncodeHeaders_AlreadyEncodedIdentity_Skip_NoVary` (per §11.11 — identity is treated as already-encoded), `TestEncodeHeaders_CacheControlNoTransform_Skip_NoVary`, `TestEncodeHeaders_StatusUncompressible_Skip_NoVary`, `TestEncodeHeaders_ETagWithDisableTrue_Skip_NotCompressedEtag` (mode-b), `TestEncodeHeaders_ETagWithDisableFalse_Continue_StripStrong` (mode-a strong-strip; default disable_on_etag_header=false), `TestEncodeHeaders_ETagWithDisableFalse_Continue_PreserveWeak` (mode-a weak-preserve), `TestEncodeHeaders_ContentLengthBelowMin_KnownAtHeaders_Skip_NoVary_ContentLengthTooSmall`, `TestEncodeHeaders_AllowPath_SetCEGzip_AppendVary_StripCL_WillCompressTrue`, `TestEncodeHeaders_PassthroughBypass_NoMutation_NoCounter`. **Group 6 — `EncodeData` compression path (per §6.7 + §11.14):** `TestEncodeData_PassthroughOrWillCompressFalse_DataContinue_NoOverwrite`, `TestEncodeData_NotEndStream_DataContinue_NoOverwrite_Defensive`, `TestEncodeData_LateMinContentLength_RevertSkip_DataContinue_CountersIncremented`, `TestEncodeData_AllowPath_GzipEncodes_OverwriteBodyCalled_CountersIncremented` (small/medium/large bodies parametrized; verifies len(compressed) > 0 + decompresses to original), `TestEncodeData_HuffmanOnlyStrategy_DifferentBytesObservable` (compares Huffman-only output bytes to default-strategy output for the same input). **Group 7 — Header mutation helpers (per §6.6 + §1.1 amendments 5-6):** `TestAppendVaryAcceptEncoding_NoExisting_SetsAccept`, `TestAppendVaryAcceptEncoding_ExistingNonAE_AppendsCommaSpaceAccept`, `TestAppendVaryAcceptEncoding_ExistingWildcard_AppendsCommaSpaceAccept` (§11.10 — even wildcard gets append), `TestAppendVaryAcceptEncoding_TokenAlreadyPresent_NoOp` (case-insensitive dedup), `TestAppendVaryAcceptEncoding_TokenAlreadyPresentMixedCase_NoOp` (`accept-encoding` matches `Accept-Encoding`), `TestMaybeStripStrongEtag_NoEtag_NoOp`, `TestMaybeStripStrongEtag_StrongEtag_Stripped`, `TestMaybeStripStrongEtag_WeakEtag_Preserved`, `TestMaybeStripStrongEtag_MalformedEtag_PreservedDefensive`. **Group 8 — Stats namespace integration (per planner-time decision 8 + ADR-0132 §Decision (i)+(v)):** `TestStatsNamespace_LibraryNameSet_StatPathCorrect`, `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` (per planner-time decision settling SPEC §12 deferred 5 — verifies SN2 flatten produces `envoy_http_compressor__gzip_<counter>` double-underscore form), `TestStatsNamespace_AllSeventeenCountersRegistered`, `TestStatsNamespace_ResponseInfixPresent_WhenResponseDirectionConfigSet`, `TestStatsNamespace_RequestCountersRegisteredAtZero` (5 always-zero request_* per §1.1 amendment 3 + ADR-0132 §Decision (vii)). ~750-900 LoC total. |
| `internal/filter/http/compressor/fuzz_test.go` | NEW | `FuzzCompressorConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. The fuzzer's interesting axes: random bytes vs. partial proto-shaped bytes vs. valid proto with random typed_config Any inside `compressor_library`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the compressor `New` factory is a multi-stage parser (outer Compressor proto + nested Gzip Any proto + per-route oneof). ~80 LoC; 30s budget per ADR-0018; **eighteenth fuzzer overall** (post-13's seventeenth `FuzzBufferConfigParse`). Seed corpus: 8 valid-config seeds (default-everything; explicit min_content_length=30; explicit content_type=[text/html]; uncompressible_response_codes=[404]; disable_on_etag_header=true; remove_accept_encoding_header=true; gzip compression_level=BEST_SPEED; gzip compression_strategy=HUFFMAN_ONLY) + 4 invalid-config seeds (nil typed_config; missing compressor_library; non-Gzip TypeURL; uncompressible_response_codes=[100]). |
| `internal/filter/http/callbacks.go` | MODIFIED | NEW one-line addition to `EncoderFilterCallbacks` interface: `OverwriteBody(b []byte)` per ADR-0131 §Decision (vi). GoDoc note: "OverwriteBody registers a replacement encode-side body. Filters MUST call this only from inside their EncodeData(data, endStream) implementation; the chain dispatch substitutes resp.Body before the wire-write path consumes it. Not goroutine-safe — the encode chain runs synchronously in the dispatch goroutine." +1 LoC. Per SPEC §4.3 + ADR-0131. |
| `internal/filter/http/chain.go` | MODIFIED | Three additions per ADR-0131 §Decision (vi): (a) new per-stream field `encodeBodyOverride []byte` on `*FilterChain`; (b) new per-stream sentinel field `encodeBodyOverridden bool` on `*FilterChain`; (c) new method on `encoderCB` struct: `func (c *encoderCB) OverwriteBody(b []byte) { c.chain.encodeBodyOverride = b; c.chain.encodeBodyOverridden = true }` (where `c.chain` is the back-reference to `*FilterChain` per existing encoderCB shape — implementer adapts to the actual encoderCB struct shape per the existing `chain.go` precedent); (d) new accessor method on `*FilterChain`: `func (c *FilterChain) EncodeBodyOverride() ([]byte, bool) { return c.encodeBodyOverride, c.encodeBodyOverridden }`. Total ~+8 LoC delta. Per SPEC §4.3 + ADR-0131. |
| `internal/filter/http/chain_test.go` | MODIFIED | NEW probe-filter-driven OverwriteBody primitive integration tests per Task 4 TDD discipline. Probe filter `overwriteBodyProbe` implements `StreamEncoderFilter` with `EncodeData(data, endStream)` calling `f.cb.OverwriteBody([]byte("REPLACED"))` then returning `DataContinue`. Test cases: `TestEncoderCB_OverwriteBody_StoresBytes_AccessorReflects` (probe sets override → chain.EncodeBodyOverride() returns ([]byte("REPLACED"), true)); `TestEncoderCB_NoOverwriteBody_AccessorReturnsFalse` (probe does NOT call OverwriteBody → chain.EncodeBodyOverride() returns (nil, false)); `TestEncoderCB_OverwriteBody_PassthroughOnSubsequentInvocations` (override survives to encoder dispatch even on subsequent EncodeData chunks — relevant for future non-MVP scenarios). ~+60 LoC. |
| `internal/filter/hcm/connection.go` | MODIFIED | NEW post-`RunEncodeData` harvest of encode-body-override per ADR-0131 §Decision (vi): after `chain.RunEncodeData(ctx, resp.Body, true)` returns, call `chain.EncodeBodyOverride()` and substitute `resp.Body` with the override bytes IFF `ok=true`. Inserted at H1 dispatch path between line ~472 (RunEncodeData call) and line ~478 (writeH1Reply call) per SPEC §3 + §6.7. ~+8 LoC. Per SPEC §4.3 + ADR-0131. |
| `internal/filter/hcm/h2dispatch.go` | MODIFIED | NEW post-`RunEncodeData` harvest symmetric to `connection.go` H1 path per ADR-0131 §Decision (vi). Inserted at H2 dispatch path between line ~310 (RunEncodeData call) and line ~328 (writeH2Reply call) per SPEC §3 + §6.7. ~+8 LoC. Per SPEC §4.3 + ADR-0131. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(compressor.TypeURL, compressor.New)` registration inserted IMMEDIATELY AFTER the existing `httpReg.Register(buffer.TypeURL, buffer.New)` line at `cmd/envoy-go/main.go:117` (and BEFORE the existing `httpReg.Register(cors.TypeURL, cors.New)` line at `cmd/envoy-go/main.go:118`). Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/compressor"` alphabetically among the existing filter-package imports (currently `cmd/envoy-go/main.go:28-35`: `buffer, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router` → `buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router`). Per ADR-0129 §Decision (v) router-first-then-alphabetical stylistic discipline (codified at phase 9 brainstorm time + reaffirmed at phases 10-13). The resulting block reads: `httpReg.Register(router.TypeURL, router.New); httpReg.Register(buffer.TypeURL, buffer.New); httpReg.Register(compressor.TypeURL, compressor.New); httpReg.Register(cors.TypeURL, cors.New); httpReg.Register(csrf.TypeURL, csrf.New); httpReg.Register(envoygotest.TypeURL, envoygotest.New); httpReg.Register(fault.TypeURL, fault.New); httpReg.Register(header_mutation.TypeURL, header_mutation.New); httpReg.Register(localratelimit.TypeURL, localratelimit.New); header_mutation.RegisterPerRouteValidator(httpReg); httpReg.Freeze()`. **No other wiring changes** — compressor is HTTP-only, no listener/cluster/drain manager threading; no per-route-validator registration call (compressor's per-route TPFC parsing happens at HCM-build via `BuildPerRouteConfig`'s generic `UnmarshalNew`, and the filter applies its PGV-mirror validation in `parsePerRoute` for per-route entries — same discipline as buffer phase 13 + csrf phase 12). ~+3 LoC delta (1 import line + 1 register line). Per SPEC §4.3 + ADR-0129. |
| `test/helpers/echobackend/` | NEW DIRECTORY | NEW shared helper package per planner-time decision 6 — phase 14 introduces this shared helper (the SPEC §4.2 + §1.1 amendment 4 references "the existing `test/helpers/echobackend/`" which does NOT exist at master tip; phase-13 buffer used a per-fixture `backends/backend.go`; phase 14 introduces the shared helper as the SPEC's design intent). Future filter fixtures needing echo-backend behavior MAY use this helper; phase-13 buffer's backend MAY be migrated in a future cleanup (out of scope for phase 14). Contents: `echobackend.go` + `doc.go` + `echobackend_test.go`. |
| `test/helpers/echobackend/doc.go` | NEW | Package doc — `// Package echobackend implements a minimal HTTP/1.1 echo backend that echoes inbound request method + path + headers as a JSON body in its response. Used by differential fixtures whose driver needs to assert on upstream-side request shape (e.g., per-route remove_accept_encoding_header verification — phase 14 fixture 0016 scenario 6).` ~25 LoC. |
| `test/helpers/echobackend/echobackend.go` | NEW | Echo-backend implementation. Single function `New() *http.Server` returns a configured `*http.Server` with handler that, for any inbound request: (1) reads request method + URL.Path + Header (canonical form lowercased per Envoy wire-form discipline per ADR-0072 + phase 04 lowercase-header pattern); (2) writes response with `Content-Type: application/json` and JSON body `{"method": "POST", "path": "/", "headers": {"host": "...", ...}}` (key/value pairs of inbound canonical headers — phase-13 buffer's per-fixture pattern carried through); (3) Status 200; `Content-Length` set to JSON body's byte length. Plus `package main` cmdline-tool wrapper file `cmd/echobackend/main.go` accepting a `--port` flag for the runner-allocated port and binding via `New()` + `srv.Serve(listener)`. ~80 LoC main library + tests. |
| `test/helpers/echobackend/echobackend_test.go` | NEW | Unit tests covering: header echo correctness; method echo correctness; body Content-Type=application/json; lowercased keys; multi-value headers serialized as comma-joined OR list (per implementer's choice; documented in echobackend.go); large header set tolerance; empty header set tolerance. ~80 LoC. |
| `test/fixtures/0016-http-compressor/` | NEW DIRECTORY | Fixture root carrying `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `inputs/driver.go`. The runner-side blank-import lives at `test/differential/runner_test.go` per the existing 0010 / 0011 / 0012 / 0013 / 0014 / 0015 convention. The shared backend at `test/helpers/echobackend/` (NEW; planner-time decision 6) — NO per-fixture `backends/` directory in 0016-http-compressor (departs from phase-13 buffer's per-fixture `backends/backend.go`; planner-time decision 7). |
| `test/fixtures/0016-http-compressor/envoy.yaml` | NEW | Reference Envoy bootstrap (admin port resolved at boot by the runner; **ONE listener `l_main` per planner-time decision 9** — single listener with six routes (`/text-html-1024` direct_response 1024-byte text/html; `/image-png-1024` direct_response 1024-byte image/png; `/text-html-10` direct_response 10-byte text/html; `/text-html-etag-strong` direct_response 1024-byte text/html with `etag: "abc"`; `/per-route-disabled` direct_response 1024-byte text/html with per-route TPFC `disabled: true`; `/per-route-rmae` cluster route to backend `c_backend` with per-route TPFC `overrides.response_direction_config.remove_accept_encoding_header: true`); cluster `c_backend` STRICT_DNS pointing at the harness backend via `host.docker.internal` per ADR-0010). Listener-level `Compressor`: `compressor_library: {name: text_optimized, typed_config: {@type: ...Gzip}}` per §11.5 load-bearing name + `response_direction_config: {}` (defaults: min_content_length=30, content_type=8-entry list, disable_on_etag_header=false, remove_accept_encoding_header=false, uncompressible_response_codes=[]; `enabled` field omitted on both sides per §1.1 amendment 2 — default = enabled). http_filters chain: `[envoy.filters.http.compressor, envoy.filters.http.router]`. ~95 LoC. Per SPEC §7.2. |
| `test/fixtures/0016-http-compressor/envoy-go.yaml` | NEW | Subject envoy-go bootstrap. Identical to `envoy.yaml` modulo cluster type (STATIC instead of STRICT_DNS) + admin/listener port values resolved at boot by the runner. Both sides use `compressor_library.name: text_optimized` per §11.5 + ADR-0132 §Decision (v) (load-bearing for stat namespace identity). Both sides set `response_direction_config: {}` per §1.1 amendment 3 + ADR-0132 §Decision (ii) for byte-equivalent stat namespace. ~95 LoC. Per SPEC §7.2. |
| `test/fixtures/0016-http-compressor/expectations.yaml` | NEW | Prose narrative of the per-scenario equivalence claims (per ADR-0019 — expectations.yaml is prose, not machine-evaluated; the runner enforces via the driver's per-scenario assertions). Documents per SPEC §7.1: scenario 1 (allow-compress) → 200 + `content-encoding: gzip` + `vary: Accept-Encoding` + decompressed body byte-exact 1024 bytes + counter delta `response_compressed +1, response_total_uncompressed_bytes +1024, response_total_compressed_bytes +<gzipped>, header_compressor_used +1`; scenario 2 (skip content-type-mismatch) → 200 + NO `content-encoding`/`vary` + `content-length: 1024` + identity body + counter delta `response_not_compressed +1, header_compressor_used +1`; scenario 3 (skip below-min) → 200 + NO `content-encoding`/`vary` + `content-length: 10` + identity body + counter delta `response_not_compressed +1, response_content_length_too_small +1, header_compressor_used +1`; scenario 4 (strong-ETag-strip + compressed-passthrough mode-a) → 200 + `content-encoding: gzip` + `vary: Accept-Encoding` + NO `etag` header + decompressed body byte-exact 1024 bytes + counter delta `response_compressed +1, totals, header_compressor_used +1`; scenario 5 (per-route disabled) → 200 + NO `content-encoding`/`vary` + `content-length: 1024` + identity body + NO counter increments (filter wholly inactive); scenario 6 (per-route rmAE override + compressed-via-real-backend) → 200 + `content-encoding: gzip` + `vary: Accept-Encoding` + decompressed body asserts NO "Accept-Encoding" key in echoed-headers JSON map + counter delta `response_compressed +1, totals, header_compressor_used +1`. Counter total deltas after the 6-request workload (envoy-go side): `response_compressed +3` (scenarios 1, 4, 6), `response_not_compressed +2` (scenarios 2, 3), `response_content_length_too_small +1` (scenario 3), `response_total_uncompressed_bytes +N` (sum of scenarios 1+4+6 input lengths), `response_total_compressed_bytes +M` (sum of scenarios 1+4+6 compressed lengths; **boundary-only assertion `0 < value < uncompressed_input_bytes` per planner-time decision 2 settling SPEC §12 deferred 2**), `header_compressor_used +5` (every scenario except 5), `not_compressed_etag +0` (no scenario uses `disable_on_etag_header: true`), 4 of 6 unique header_* counters at zero (no_accept_header / wildcard / identity / not_valid — fixture always sends gzip-AE), 5 request_* counters at zero (vacuous on both sides per ADR-0132 §Decision (vii)). Wire-shape divergence allow-list per ADR-0131 §Decision (ii) + ADR-0133 §Decision (iv): `content-length` value + `transfer-encoding` presence on compressed scenarios 1, 4, 6 — envoy-go fixed-CL identity vs Envoy chunked. Header allow-list: `date`, `server`, timing/identity headers per BEHAVIOR_CONTRACT.md `## Header allow-list`. Body axis per ADR-0133 §Decision (i)-(ii): byte-exact for uncompressed (2, 3, 5); decompressed-byte-exact for compressed (1, 4, 6). Cross-refs SPEC §7.1 + §13.1 + ADR-0125 + ADR-0129..ADR-0133. ~50 LoC. Per SPEC §4.2. |
| `test/fixtures/0016-http-compressor/README.md` | NEW | Fixture overview + per-scenario equivalence-claim narrative + 6-scenario list (per SPEC §7.1) + single-listener bootstrap discipline (per planner-time decision 9: all 6 scenarios run against the single listener `l_main` with six routes — no per-scenario teardown) + Envoy-deviation note (none — compressor is a normal HTTP filter; no SIGTERM/drain divergence) + the wire-shape divergence-window from reference Envoy (per ADR-0131 §Decision (ii) — envoy-go fixed-CL identity vs Envoy chunked; documented at BEHAVIOR_CONTRACT phase-14 forward-pointer notes) + the decompress-and-compare body-assertion discipline (ADR-0133 — driver decompresses gzip responses via `compress/gzip.NewReader` and asserts byte-exact on plaintexts; compressed bytes are STRUCTURALLY non-byte-exact per §11.14) + the per-route disabled-OR-rmAE 5th canonical discipline note (SPEC §1.3 + ADR-0125 amendment §(viii)) + the per-route SHARED stats note (per ADR-0125 amendment §(ix) + ADR-0132 §Decision (iv)) + the new `test/helpers/echobackend/` shared helper note (used by scenario 6 for upstream-side AE-absence assertion; planner-time decision 6) + the `compressor_library.name: text_optimized` load-bearing-for-stat-namespace note (per §11.5 + ADR-0132 §Decision (v)) + planner-time-decision cross-references. ~85 LoC. Per SPEC §4.2. |
| `test/fixtures/0016-http-compressor/inputs/driver.go` | NEW | Go driver implementing the SPEC §7.1 + §7.2 6-scenario sequential orchestration via the single-listener topology per planner-time decision 9. **Driver shape:** `package driver`; `init()` calls `fixture.RegisterFixture("0016-http-compressor", &compressorDriver{})`; `BackendCount() int` returns 1 (the echobackend serves scenario 6); `BackendKind() fixture.BackendKind` returns `fixture.HTTPCompressor` (the new enum value added in Task 10); implements the SINGLE-listener fixture interface (`fixture.Driver` per the buffer / fault / cors / header_mutation / csrf precedent — NOT the `MultiListenerDriver` introduced by phase 07.2 + used by phase 11). `ReferenceBootstrap` / `SubjectConfig` templates `envoy.yaml` / `envoy-go.yaml` substituting the listener-port placeholder + backend port; the bootstrap is rendered ONCE. `DriveReference` / `DriveSubject` issue ALL SIX scenarios in ONE call: scenario 1 (GET `/text-html-1024` with `Accept-Encoding: gzip`); scenario 2 (GET `/image-png-1024` with `Accept-Encoding: gzip`); scenario 3 (GET `/text-html-10` with `Accept-Encoding: gzip`); scenario 4 (GET `/text-html-etag-strong` with `Accept-Encoding: gzip`); scenario 5 (GET `/per-route-disabled` with `Accept-Encoding: gzip`); scenario 6 (GET `/per-route-rmae` with `Accept-Encoding: gzip`) — backend assertion of inbound NO `Accept-Encoding` via JSON-echo + driver-side parse per planner-time decision 10 (D7 settlement). Per-probe captures status + body + headers + post-scenario comparison via `CompareBytes` for uncompressed scenarios (2, 3, 5) and per the **decompressGzip(body []byte) ([]byte, error)** helper for compressed scenarios (1, 4, 6) per ADR-0133 §Decision (i)-(ii); the driver exports `assertBodyEquivalent(envoyGoResponse, envoyResponse *http.Response, originalPayload []byte) error` per ADR-0133 §Decision (ii). Final `/stats/prometheus` scrape captures the 17 filter-specific counters AND the existing in-table HCM counters for differential-equivalence assertion. **No timing tolerances** — all scenarios run in microseconds. **`response_total_compressed_bytes` boundary-only assertion** per planner-time decision 2 (settles SPEC §12 deferred 2): on each side independently, assert `0 < counter_value < uncompressed_input_bytes` (where `uncompressed_input_bytes = sum(scenario1 + scenario4 + scenario6 uncompressed payload sizes)`); other counters use byte-exact delta assertions. ~220 LoC. Per SPEC §7.4 + ADR-0133. |
| `test/differential/fixture/fixture.go` | MODIFIED | New `BackendKind` enum value `HTTPCompressor BackendKind = 13` after the existing `HTTPBuffer BackendKind = 12` (introduced by phase 13). Doc-comment notes: "HTTPCompressor is an out-of-process HTTP/1.1 echo backend: the runner spawns `test/helpers/echobackend/cmd/echobackend/main.go` on the pre-allocated port. The backend echoes the inbound request method + path + headers as a JSON object in the response body (load-bearing for fixture scenario 6's `Accept-Encoding`-absence assertion at the backend boundary per the per-route remove_accept_encoding_header override per ADR-0125 amendment §(viii) + SPEC §11.8); status 200, Content-Type: application/json. No TLS. Introduced by fixture 0016-http-compressor (phase 14 Task 10). Reuses the new `test/helpers/echobackend/` shared helper introduced at the same task per planner-time decision 6. Because the backend is a subprocess, the runner's in-process accept counter is NOT incremented." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | (a) Add blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs"` (insert in alphabetical order, after the `0015-http-buffer` blank-import). (b) Extend the `kind` switch in `runFixture` with a new case `fixture.HTTPCompressor` mirroring the `HTTPBuffer` block: spawn via `startEchoBackend`. (c) Add new spawn helper `startEchoBackend(ctx, repoRoot, port int) (*exec.Cmd, error)` per planner-time decision 6 (uses the new shared helper at `test/helpers/echobackend/cmd/echobackend/main.go`): `exec.CommandContext(ctx, "go", "run", "./test/helpers/echobackend/cmd/echobackend", "--port", fmt.Sprintf("%d", port))` + Setpgid process-group + Stdout/Stderr to os.Stderr + Start. ~+25 LoC delta total. Per SPEC §4.3. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 verbatim Markdown patches: (a) NEW `### envoy.filters.http.compressor` subsection inserted under existing `## HTTP filter chain` umbrella AFTER the existing `### envoy.filters.http.buffer` subsection (alphabetical: `buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit`) per §13.1 (~140 LoC verbatim); (b) `## Stat-name mapping ### 29-name table` extends to **46 names** (17 new rows verbatim from §13.2 — 6 header_* + 1 not_compressed_etag + 5 response_* + 5 request_*) per §13.2; (c) `## Equivalence Matrix` new compressor-filter row (per §13.3; ~3 LoC); (d) NEW `### Phase 14 forward-pointer notes` subsection appended to existing `## Forward-pointer notes` section per §13.4 (~55 LoC) — covers the 8-item deferral list (4 deprecated top-level mirrors silent-ignore; `runtime_enabled` + `enabled` runtime gates; `choose_first` always-q-value; `status_header_enabled` always-no-status-header; `request_direction_config` always-disabled; non-Gzip codec-library TypeURLs PARSE-rejected per ADR-0130; per-route `compressor_library` swap silent-ignored per ADR-0125 amendment §(viii); `Gzip.{memory_level, window_bits, chunk_size}` not expressible in Go gzip) + the wire-shape divergence-window from reference Envoy note (envoy-go fixed-CL identity vs Envoy chunked per ADR-0131 §Decision (ii)) + the framework primitive note (`EncoderFilterCallbacks.OverwriteBody(b []byte)` per ADR-0131 §Decision (vi)) + the min_content_length late-revert anomaly note (per ADR-0131 §Decision (vii)) + the stat namespace note (`compressor.<library_name>.<codec>.[response.]<counter>` per ADR-0132 §Decision (i)+(v)). ADR-0052 in-place edit authorisation carries forward. ~+225 LoC total. |
| `docs/envoy-go/DECISIONS.md` | UNCHANGED at PLAN commit; MODIFIED at impl commits | Phase 14's **5 ADRs (ADR-0129..ADR-0133) ALREADY landed at the SPEC commit `073cb88`** in their final form per ADR-0044 ADR-on-impl convention's SPEC-time-anticipation discipline; the impl tasks DO NOT re-author or amend these ADRs. Each ADR's `Lands-in-task:` field already records the impl-task anchor (per the table at `## ADRs introduced by this plan` below); the impl-task implementer references the ADR in the commit message + `acceptance` bullet but the ADR text itself is untouched. Per ADR-0044 the `Lands-in-task` field is the load-bearing anchor; the ADR body is final at SPEC time. **ADR-0125 amendment paragraph §(viii)-(x) ALREADY landed at SPEC commit** per phase-13 ADR-0127-v2 in-place-update precedent; PLAN does not re-anchor. **NO new ADRs anticipated at PLAN time.** **NO inline supersessions / amendments anticipated** (phase 14 inherits the existing ADR landscape verbatim modulo the 5 new ADRs already landed). |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `14` `in-progress → done` flip AT the phase-done commit. Row 14's summary text needs sharpening at the phase-done commit to reflect the SPEC's §1.1 amendments (the row currently summarizes the BRAINSTORM-time pre-amendment scope: "29→40 names" stat-table extension + "11 new counters" + "Rule SN10" + "8 fields consumed + 9 silent-ignored"; the actual landing per §1.1 amendment 3 is "29→46 names" + "17 new counters" + "NO new SN rule (uses existing SN2)" + "6 listener consumed + 9 silent-ignored + 2 codec consumed + 3 codec silent-ignored = 8 grand-total consumed + 12 silent-ignored + 1 parse-rejected"). The §9 HTTP filters family heading at row 56 stays UNCHANGED (headings are not rows; their state is implicit; per ADR-0106). No new row authored for the next §9 family-child; future family-expansion brainstorms cold-start from the §9 heading + just-shipped phase 14 artefacts (per ADR-0106 no-sibling-stub discipline). |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance through lifecycle-states 3 (PLAN drafting — this PLAN landing flips state 2 → 3 in the orchestrating session's STATE.md edit, completed by the PLAN-authoring session at `lifecycle-state: phase 14 PLAN.md authored; impl pending`), 4 (PLAN execution — Tasks 1–14 land production code + fixture; STATE stays at 4), 5 (verification — Task 15 lands BEHAVIOR_CONTRACT/ROADMAP/STATE/six-gate verification; STATE flips 4 → 5), 6 (review — Task 16 REVIEW.md per requesting-code-review skill; STATE flips 5 → 6 then to `awaiting next planning`); `next-skill: superpowers:brainstorming` against §9's family list for the next family-child; `active-phase: <next-family-row-id>` resolved by the next session's planner. |
| `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` | NEW | Append-only log; one entry per task; verbatim command outputs. Mirrors phase-04..13 PROGRESS.md structure. The preamble enumerates the 5 anticipated ADRs ADR-0129..ADR-0133 (already landed at SPEC commit per ADR-0044 ADR-on-impl SPEC-time-anticipation discipline) + the per-task ADR anchor table + the planner-time deferred-decisions resolution (the 16 items below — 8 from SPEC §12 plus 8 PLAN-emerging items). |
| `docs/envoy-go/phases/14-http-filter-compressor/REVIEW.md` | NEW | End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 cadence; populates per the requesting-code-review skill. Phase 14 has NO parent row (it is a top-level §9 family-child per ADR-0106), so the REVIEW closes only row 14. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's eight deferred decisions before implementation; this PLAN settles all eight plus eight that emerged at PLAN-drafting time (items 9–16 below). The sixteen resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

1. **D1 — `compressor.go` file split = TWO-WAY SPLIT: `compressor.go` (main filter + types + factory + helpers + Encode/Decode methods) + `acceptencoding.go` (q-value parser + classification dispatch).** Per SPEC §12 D1 + §4.1 PLAN-author option. The total filter surface estimate of ~480-560 LoC exceeds the project's general 200-300 LoC mental-model threshold (csrf single-file at ~280 LoC + buffer single-file at ~250 LoC). The natural split shape per SPEC §4.1 is into `codec.go` + `acceptencoding.go` + `headers.go` + `perroute.go`; the AE q-value parser is the most-self-contained primitive (RFC 7231 §5.3.4 conformance; ~100-130 LoC; 25+ unit-test cases) and benefits most from isolation (own test sub-section; reusable in future codec phases that may share the parser). `headers.go` (Vary-append + ETag-strip helpers; ~50-80 LoC) and `perroute.go` (~30 LoC) are tightly coupled to the EncodeHeaders body where they're called and stay in `compressor.go`; codec helpers (`unmarshalCompressorLibrary` + `buildCompiledGzipConfig`; ~50 LoC) are tightly coupled to the `New` factory and stay in `compressor.go`. The 2-way split keeps `compressor.go` at ~380-440 LoC (acceptable mental-model load) while extracting the AE parser into `acceptencoding.go` at ~100-130 LoC (tight, focused). Mirrors phase-11 `local_ratelimit.go` + `bucket.go` split rationale (the `tokenBucket` was a separable primitive; phase-14's AE parser is similarly separable). DIVERGES from phase-13 buffer + phase-12 csrf single-file precedents because phase-14's surface is materially larger. *Anchored: SPEC §12 D1; SPEC §4.1 file-split shape; phase-11 `local_ratelimit.go` + `bucket.go` precedent; project mental-model threshold.*

2. **D2 — `response_total_compressed_bytes` counter tolerance shape = (b) BOUNDARY-ONLY: `0 < counter_value < uncompressed_input_bytes` on each side independently.** Per SPEC §12 D2 + §7.3 + ADR-0133 §Decision (iii). SPEC's preferred shape is (b) boundary-only because: (i) it captures the structural invariant — gzip compression makes bytes smaller; both libraries respect this; (ii) it does NOT couple the assertion to specific compression ratios (Go `compress/gzip` and Envoy libz produce different ratios on the same input per §11.14; calibrating a tolerance window requires empirical pinning that may drift over time); (iii) it is the structurally honest assertion — operators care that compression occurred AND was lossless (the decompress-and-compare body assertion catches losslessness); the byte-count is supplementary. ADR-0133 §Decision (iii) records boundary-only as the default; PLAN may switch to tolerance window if empirical compression-ratio variance is small enough — PLAN settles boundary-only. The driver's per-counter assertion implements: `for each side independently, assert 0 < counter_value < uncompressed_input_bytes` where `uncompressed_input_bytes = sum(scenario1 + scenario4 + scenario6 uncompressed payload sizes) = 1024 + 1024 + N_backend_echo_body`. *Anchored: SPEC §12 D2 + §7.3; ADR-0133 §Decision (iii); §11.14 empirical evidence on compression-byte variance.*

3. **D3 — min_content_length late-revert anomaly disposition = (a) ACCEPT THE WIRE-SHAPE ANOMALY + DOCUMENT AT §13.4.** Per SPEC §12 D3 + §6.6 + §6.7 + ADR-0131 §Decision (vii). The unset-CL EncodeHeaders-defer-to-EncodeData path's late-revert anomaly is structurally rare in envoy-go's framework (resp.Body is pre-buffered before encode-chain per `connection.go:467` — `len(resp.Body) == 0` is the only chunked-equivalent state); fixture 0016 sidesteps via direct_response routes that always carry Content-Length on the action's headers (the gate fires at EncodeHeaders, never at late-EncodeData). PLAN concurs with SPEC's preferred (a) — accept the wire-shape anomaly + document at BEHAVIOR_CONTRACT.md §13.4 phase-14 forward-pointer notes per ADR-0131 §Decision (vii). The (b) alternative (introduce `EncoderFilterCallbacks.RevertEncodeHeaders` primitive ~10-15 LoC) widens the §3 framework-survey delta scope unnecessarily for a structurally-rare edge case. PLAN settles (a). *Anchored: SPEC §12 D3 + §6.6 + §6.7; ADR-0131 §Decision (vii); fixture 0016 sidestepping discipline.*

4. **D4 — Counter-emission shape on the late-revert-anomaly path = INCREMENT BOTH `response_content_length_too_small` AND `response_not_compressed`.** Per SPEC §12 D4 + §6.7 pseudocode. The SPEC author's "Open question" was whether Envoy increments BOTH counters on this path or only one. PLAN settles based on SPEC §6.7 + §11.5 reasoning: the §11.5 stat scrape's empirical counter ratios show `response_not_compressed = response_content_length_too_small + (other skip reasons)` — i.e., `response_not_compressed` is the SUM of skip-reason counters; the `response_content_length_too_small` counter is a SUB-sum specific to the below-threshold cause; both increment together on the below-threshold skip path. PLAN settles **both increment together** per the SPEC §6.7 pseudocode. If subsequent unit-test against reference Envoy source diverges, the impl-task author adapts (the late-revert path is structurally unreachable in fixture 0016 so no differential observability). *Anchored: SPEC §12 D4 + §6.7 + §11.5 empirical evidence on counter ratios.*

5. **D5 — Library-name-empty embedded-namespace shape = MIRROR ENVOY VERBATIM with consecutive dots → `compressor..gzip.<counter>` → SN2 flatten produces `envoy_http_compressor__gzip_<counter>` (double-underscore).** Per SPEC §12 D5 + §11.5 + §6.9 + ADR-0132 §Decision (v). When `compressor_library.name` is empty string, Envoy emits stats at the consecutive-dots path; envoy-go MUST mirror exactly. The Go-side stat-name builder MUST NOT collapse the empty segment (test in Group 8 verifies the double-underscore Prometheus form). The fixture uses `name: text_optimized` so this edge case is unit-test-only (Group 8's `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath`). PLAN settles **mirror Envoy verbatim**. *Anchored: SPEC §12 D5 + §11.5 + §6.9; ADR-0132 §Decision (v).*

6. **D6 — `status_header_enabled: true` divergence-window observability = SILENT-IGNORE AT PARSE + RUNTIME with unit-test asserting field is ignored.** Per SPEC §12 D6 + §1.1 amendment 1 + §13.4. The field is silent-ignored at parse-time (the field is OPTIONAL in the Compressor proto; envoy-go's `New` accepts the field but does NOT store it on `compiledConfig`); silent-ignored at runtime (envoy-go always-no-status-header — `x-envoy-compression-status:` debug header is not emitted). PLAN settles **silent-ignore + Group 1 unit test** (`TestNew_StatusHeaderEnabled_True_SilentIgnored` in Task 2's test-file landing). The runtime divergence is operator-visible and documented at BEHAVIOR_CONTRACT.md phase-14 forward-pointer notes (§13.4) per ADR-0040 silent-ignore discipline. *Anchored: SPEC §12 D6 + §1.1 amendment 1 + §13.4 + ADR-0040.*

7. **D7 — `compressor_library` per-route swap silent-ignore observability = ACCEPT-BUT-IGNORE AT PARSE with Group 2 unit test.** Per SPEC §12 D7 + §1.1 amendment 4 + §2.2 + ADR-0125 amendment §(viii). The per-route `overrides.compressor_library` field is silent-ignored at parse + runtime (`parsePerRoute` accepts the field's presence on the proto but does not store it on `compiledPerRoute`; no envoy-go-only validation; envoy-go uses the listener-level library regardless of per-route override). PLAN settles **accept-but-ignore + Group 2 unit test** (`TestParsePerRoute_Overrides_LibrarySwap_SilentIgnored`). Operator divergence-window is documented at BEHAVIOR_CONTRACT.md phase-14 forward-pointer notes (§13.4). *Anchored: SPEC §12 D7 + §1.1 amendment 4 + §2.2 + ADR-0125 amendment §(viii) + ADR-0040.*

8. **D8 — filterStats struct field naming = Go-PASCALCASE matching counter-name suffix; bijective mapping documented inline at the struct definition.** Per SPEC §12 D8 + §6.9. Field names: `HeaderCompressorOvershadowed`, `HeaderCompressorUsed`, `HeaderIdentity`, `HeaderNotValid`, `HeaderWildcard`, `NoAcceptHeader`, `NotCompressedEtag`, `ResponseCompressed`, `ResponseContentLengthTooSmall`, `ResponseNotCompressed`, `ResponseTotalCompressedBytes`, `ResponseTotalUncompressedBytes`, `RequestCompressed`, `RequestContentLengthTooSmall`, `RequestNotCompressed`, `RequestTotalCompressedBytes`, `RequestTotalUncompressedBytes`. Mapping bijective; Go field name ↔ stat-name suffix is one-to-one. Mirrors phase-12 csrf's `filterStats{requestValid, requestInvalid, missingSourceOrigin}` lowercased-camelCase precedent extended to PascalCase for phase-14's larger surface (PascalCase is canonical Go for cross-package callers; phase-12 used lowercase camelCase because the struct is package-local; phase-14 follows phase-12 — lowercase camelCase for unexported field consistency, BUT PASCAL-leading initial letter for the first letter of each word for readability — implementer's call between camelCase / PascalCase). PLAN settles **Go-canonical PascalCase if exported / camelCase if unexported** with the actual field set being unexported (lowercase first letter); names match the counter-name suffix bijectively. *Anchored: SPEC §12 D8 + §6.9; phase-12 csrf precedent.*

9. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: f` SAME *filter instance.** Per ADR-0129 §Decision (iv). Phase 14 is the FIRST §9 row to use `Decoder: f, Encoder: f` SAME-instance with non-vacuous both paths structurally. The decode-side surface is non-vacuous (Accept-Encoding parser + per-route resolve + maybe-strip-AE per §1.1 amendment 4 + §6.4); the encode-side surface is the algorithmic core (skip-decision + Vary-append + ETag-strip + gzip-encode + OverwriteBody). The SAME *filter instance services both sides; per-stream state (acceptedEncoding, perRoute, willCompress, passthrough) lives on the *filter struct so the encode-side can read what the decode-side parsed. Setting `Encoder: nil` would force AE-parsing into encode-side at EncodeHeaders, but the AE header is on the REQUEST not the response — by the time encode-side fires, the request headers are no longer accessible; decode-side AE-parse + state cache is the structurally-honest shape. Setting `Decoder: nil` would skip the AE parsing entirely. ADR-0129 §Decision (iv) + §Alternatives (b) + (c) settle this. *Anchored: ADR-0129 §Decision (iv) + §Alternatives (b) + (c); SPEC §6.1 + §6.4.*

10. **PLAN-emerging — Filter-callback wiring hooks = BOTH `SetDecoderCallbacks(cb)` AND `SetEncoderCallbacks(cb)`; both store on the SAME *filter instance.** Per the decoder-only / encoder-only / both-sides discipline at `internal/filter/http/types.go:75-78` and the `Decoder: f, Encoder: f` shape settled at decision 9. Implementer adds two methods on `*filter`: `func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }` and `func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }`. The framework's per-stream state machine calls both methods once per stream as part of chain construction; the filter stores both references for later use during DecodeHeaders (uses `f.dcb` for `RequestRouteConfig` per §6.4) and EncodeData (uses `f.ecb` for `OverwriteBody` per §6.7). DIVERGES from phase-12 csrf + phase-13 buffer (which set `Encoder: nil` and only implement decoder-side methods) per decision 9 rationale. *Anchored: types.go HTTPFilter struct (lines 75-78); ADR-0071 iteration protocol; SPEC §6.1 + §6.8; decision 9 above.*

11. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with SIX ROUTES.** Per SPEC §7.2. Phase 14's 6 scenarios split across the same listener: scenarios 1, 2, 3, 4 against direct_response routes (`/text-html-1024`, `/image-png-1024`, `/text-html-10`, `/text-html-etag-strong`); scenario 5 against `/per-route-disabled` direct_response with per-route TPFC `disabled: true`; scenario 6 against `/per-route-rmae` cluster route (to the new shared echobackend per decision 6) with per-route TPFC `overrides.response_direction_config.remove_accept_encoding_header: true`. All scenarios run against the SAME listener with the same listener-level config; the only varying inputs are the request path (driving per-route resolution + direct_response action selection) + listener-default settings (rmAE=false; per-route override changes effective rmAE). UNLIKE phase 11's 4-listener topology (driven by per-scenario distinct bucket parameters), phase 14's scenarios all share the listener-level Compressor config; the only varying inputs are the request path. The single-listener topology fits the existing `fixture.Driver` contract (NOT `MultiListenerDriver`) — same as buffer 0015 + csrf 0014 + header_mutation 0012 + cors 0007a/0007b + fault 0011. Saves driver complexity (no per-scenario port allocation; no `DriveSubjectMulti` orchestration). *Anchored: SPEC §7.2 (the SPEC's bootstrap fragment shows a single listener with six routes); phase 09 / 10 / 12 / 13 single-listener precedents (0011 / 0012 / 0014 / 0015).*

12. **PLAN-emerging — Echo-backend = NEW SHARED `test/helpers/echobackend/` (NOT a per-fixture backend).** Per SPEC §1.1 amendment 4 + §4.2 design intent ("the existing `test/helpers/echobackend/`"). At master tip `2b262b8`, NO `test/helpers/echobackend/` exists — phase-13 buffer used a per-fixture `test/fixtures/0015-http-buffer/backends/backend.go` instead. The SPEC's reference to "the existing `test/helpers/echobackend/`" is structurally inaccurate at master tip but reflects the SPEC's design intent that this helper SHOULD exist as a shared resource. PLAN settles **introduce the shared helper at `test/helpers/echobackend/`** (Task 10) per the SPEC's design intent, mirroring the existing `test/helpers/{tcp,http,h2,tls,http_diff,http_response}.go` shared-helper pattern. Phase-13 buffer's per-fixture backend MAY be migrated to use the new shared helper in a future cleanup (out of scope for phase 14). The shared helper exposes a `package echobackend` Go library + a `package main` cmdline wrapper at `test/helpers/echobackend/cmd/echobackend/main.go`. *Anchored: SPEC §1.1 amendment 4 + §4.2 design intent; existing `test/helpers/` shared-pattern precedent.*

13. **PLAN-emerging — BackendKind enum value = `HTTPCompressor BackendKind = 13`** (continues existing naming convention; next value after phase 13's `HTTPBuffer BackendKind = 12` at `test/differential/fixture/fixture.go:218`). Doc-comment matches the format used for `HTTPBuffer` mentioning the new shared helper at `test/helpers/echobackend/cmd/echobackend/main.go` per decision 12. *Anchored: phase 13 PLAN planner-time decision 7 precedent; existing enum at `test/differential/fixture/fixture.go:129-218`.*

14. **PLAN-emerging — Framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` lands at TASK 4 (FIRST among the impl tasks that consume it).** Per cold-start prompt Critical PLAN-time obligation 2 + ADR-0131 §Decision (vi) + Lands-in-task field. The framework primitive load-bearing for Path B body algorithm; Tasks 5-7 (DecodeHeaders + EncodeHeaders + EncodeData) consume the primitive. Task 4's TDD discipline: write failing chain-integration test using a probe filter that calls `OverwriteBody` from inside its `EncodeData` body; verify the chain dispatch substitutes resp.Body before wire-write; THEN land the +1 LoC interface method on `EncoderFilterCallbacks` + ~6-8 LoC `encoderCB.OverwriteBody` impl + per-stream override field on `*FilterChain` + `EncodeBodyOverride() ([]byte, bool)` accessor + the H1 + H2 HCM-side post-RunEncodeData harvest. The probe filter is a test-only filter inside `internal/filter/http/chain_test.go` (NOT the compressor filter — the compressor filter doesn't exist yet at this stage; it gets implemented in Tasks 5-9). DIVERGES from phase-13 ADR-0128 timing (which retroactively-anchored at Task 12 because the framework deltas were unanticipated by phase-13 SPEC §4); phase-14 SPEC §3 framework-survey explicitly anticipated the framework primitive at SPEC time + ADR-0131 anchored Lands-in-task: Task 4 at SPEC commit. PLAN concurs and lands at Task 4. *Anchored: cold-start prompt Critical PLAN-time obligation 2; ADR-0131 §Decision (vi) + Lands-in-task field; phase-13 ADR-0128 retroactive-anchor anti-pattern lesson.*

15. **PLAN-emerging — ADR anchor schedule per ADR-0044 ADR-on-impl convention = ADR-0129 + ADR-0130 at Task 2; ADR-0131 at Task 4; ADR-0132 at Task 8; ADR-0133 at Task 11.** Per SPEC §8 + ADR-0044 + the per-ADR Lands-in-task fields ALREADY landed at SPEC commit. Tasks 2, 4, 8, 11 are the first-use commits per ADR-0044; the implementer at each task references the ADR in the commit message ("phase 14: ... [ADR-XXXX]") and confirms the ADR's Context/Decision/Consequences sections are intact in DECISIONS.md (no in-place edit needed since the ADRs were authored in final form at SPEC commit). PLAN does not re-anchor any ADR; the anchor schedule is fixed by the per-ADR Lands-in-task fields at SPEC commit. ADR-0125 amendment paragraph §(viii)-(x) ALREADY landed at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent — no PLAN re-anchor for ADR-0125 either. *Anchored: SPEC §8 + ADR-0044; per-ADR Lands-in-task fields at SPEC commit `073cb88`.*

16. **PLAN-emerging — Acceptance discipline at the per-task level = each task's acceptance bullet enumerates the verbatim verification commands AND the expected-output anchors (verbatim file contents OR command exit codes); per-task ADR-anchor verification (each task referencing an ADR confirms the ADR's `Lands-in-task: Task N` field matches AND the ADR text is intact at HEAD).** Per phase-13 PLAN per-task acceptance precedent. Each impl task carries an `acceptance` paragraph naming the verbatim post-conditions (e.g., `go test -race -count=1 ./internal/filter/http/compressor/` exit 0; `grep -nE '^## ADR-0131' docs/envoy-go/DECISIONS.md` returns 1 match; `git log -1 --format=%H -- ...` returns the just-committed SHA). The implementer copies the verbatim acceptance commands into PROGRESS.md per task. *Anchored: phase-13 PLAN per-task acceptance precedent; ADR-0044 ADR-on-impl convention's first-use-commit reference discipline.*

These sixteen decisions are reproduced verbatim in `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The five ADRs anticipated by SPEC §8 (ADR-0129..ADR-0133). **ALL FIVE ALREADY LANDED at the SPEC commit `073cb88` in their final form** per ADR-0044 ADR-on-impl convention's SPEC-time-anticipation discipline (each ADR's `Lands-in-task:` field already records the impl-task anchor; the ADR body is final at SPEC time and the impl-task implementer references the ADR in the commit message + acceptance bullet without re-authoring or amending). The per-ADR Lands-in-task anchors per the per-ADR fields at SPEC commit:

| ADR | Title | Lands-in-task (per ADR Lands-in-task field at SPEC commit) |
|---|---|---|
| ADR-0129 | `internal/filter/http/compressor/` package shape — single-token directory + ENCODER+DECODER `HTTPFilter` value + 17-counter `filterStats` + boot-registration ordering | Task 2 (per ADR-0129 Lands-in-task field at SPEC commit `073cb88`) |
| ADR-0130 | `compiledConfig` shape + 8 consumed/12 ignored field decomposition + codec-library Any-unmarshal-and-dispatch + parse-rejection of unknown TypeURL + Gzip compression-level mapping table + envoy-go-only error wording | Task 2 (per ADR-0130 Lands-in-task field at SPEC commit) |
| ADR-0131 | Body algorithm Path B (buffer-then-compress) + wire-shape divergence + `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework primitive + min_content_length late-revert anomaly forward-pointer | Task 4 (per ADR-0131 Lands-in-task field at SPEC commit; framework primitive lands FIRST per cold-start prompt Critical PLAN-time obligation 2) |
| ADR-0132 | 17-counter stat surface + namespace shape `compressor.<library_name>.<codec>.[response.]<counter>` + Rule SN2 reuse (NO new SN10) + per-route SHARED stats discipline | Task 8 (per ADR-0132 Lands-in-task field at SPEC commit; the 17-counter filterStats wiring lands together with Stats namespace tests at Task 8; the BEHAVIOR_CONTRACT.md 29→46 stat-table extension lands at Task 15 per ADR-0052 in-place edit authorisation) |
| ADR-0133 | Differential-fixture decompress-and-compare body-assertion discipline | Task 11 (per ADR-0133 Lands-in-task field at SPEC commit; the decompress-and-compare driver helpers land at Task 11 — fixture infrastructure with ADR-0133 helpers; subsequent fixture tasks 12-14 consume the helpers) |

**Plus ADR-0125 amendment paragraph §(viii)-(x)** — ALREADY landed at SPEC commit `073cb88` per phase-13 ADR-0127-v2 in-place-update precedent. No PLAN-time re-anchor; impl tasks reference §(viii) per ADR-0125's existing amendment text. The amendment notes phase 14 compressor as the SECOND row using disabled-OR-override 5th canonical + per-route override surface FILTER-SPECIFIC and NARROWER than listener-level + per-route stats SHARED with listener-level (extended to disabled-OR-override discipline) + future per-route disciplines using the 5th canonical SHOULD enumerate override-field-surface explicitly in their own ADR.

The implementer at each impl-anchor task verifies the ADR's `Lands-in-task: Task N` field matches AND that the ADR text is intact at HEAD via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning 1 match (the canonical SHA-fill discipline).

**Inline supersessions / amendments anticipated** (cross-references only; **NO in-place ADR edits required by phase 14** — this is consistent with phases 12 + 13; UNLIKE phases 10 + 11 which each amended ADR-0073):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — UNCHANGED in phase 14. Phase 14's per-route is data-only AND most-specific-override per the ADR-0125 5th canonical disabled-OR-override sum-type (per-route shape narrower than listener-level wholesale per ADR-0125 amendment §(viii); WITHIN the override field envelope, wholesale-not-merge applies). The wholesale-override discipline applies as-is. ADR-0125 amendment §(viii)-(x) (already landed at SPEC commit) extends ADR-0125 §Decision (vi) without amending ADR-0073. NO in-place edit of ADR-0073.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 14. The 8-item deferral list (per SPEC §2.1) is captured INLINE at BEHAVIOR_CONTRACT §13.4 (the `### Phase 14 forward-pointer notes` subsection). NO new deferral ADRs are authored at phase 14 (mirrors phase 10 / 11 / 12 / 13 SPEC §8.1 collapse precedent — silent-ignore + parse-time-rejection are framework patterns, deferral lists are documentation artefacts).
- **ADR-0044** (ADR-on-impl convention) — UNCHANGED in phase 14. The 5 ADRs (ADR-0129..ADR-0133) each carry a `Lands-in-task` field anchored at the first-use impl-task; the ADR text is final at SPEC commit; impl-tasks reference the ADR via commit message + acceptance bullet without re-authoring.
- **ADR-0061** (stats Registry + SN1–SN9 rules) — UNCHANGED in phase 14. NO new SN flattening rule per SPEC §1.1 amendment 3 + §11.5 + ADR-0132 §Decision (iii). Compressor reuses the existing SN2 rule (HCM-namespace `http.<HCM stat_prefix>.<rest>` → `envoy_http_<rest>` + label `envoy_http_conn_manager_prefix=<HCM stat_prefix>`); the library name + codec are PART OF THE STATIC stat-name suffix. UNLIKE phase 11 which extended SN with Rule SN9 for `envoy_local_http_ratelimit_prefix` via ADR-0118. Cross-reference recorded in ADR-0132 §Decision (iii). NO in-place edit.
- **ADR-0072** (HTTPRegistry threaded constructor map + factory typed_config validation contract) — UNCHANGED in phase 14 (the existing `Register` + `Freeze` discipline carries through). Cross-reference recorded in ADR-0129 §Consequences. NO in-place edit.
- **ADR-0074** (filter set: cors + envoy_go_test) — purely additive expansion recorded in ADR-0129 §Consequences. The filter set extends from {buffer, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router} to {buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router}. NO in-place edit of ADR-0074.
- **ADR-0075** (HCM dispatch — wire-write path) — UNCHANGED in phase 14 (the existing wire-write paths at `connection.go` + `h2dispatch.go` carry through; the new ~+8 LoC at each is post-RunEncodeData harvest, not wire-write surgery). Cross-reference recorded in ADR-0131 §Consequences. NO in-place edit.
- **ADR-0076** (framework body-buffer cap — `filterBufferLimitBytes = 1 << 20`) — UNCHANGED in phase 14. The cap stays armed on the encode-side; bodies > 1 MiB observe connection-level reset before reaching compressor's EncodeData per §5.10 (a). The cap-promotion forward-pointer remains open for a future row (likely the symmetric decompressor or a global cap-promotion phase). NO in-place edit of ADR-0076.
- **ADR-0100** (FactoryCtx framework extension — Stats + StatPrefix) — UNCHANGED in phase 14. Compressor's `New` factory CONSUMES `ctx.Stats` + `ctx.StatPrefix` to register the 17-counter filterStats per ADR-0132 §Decision (i); the FactoryCtx interface is unchanged. ADR-0129 §Consequences notes the 17-counter filterStats as the largest stat surface per filter to date in §9 family-rows (phase 11 had 4; phase 12 had 3; phase 13 had 0; phase 14 has 17). NO in-place edit.
- **ADR-0101** (runtimeConfig shape + parser pattern) — extended cross-reference recorded in ADR-0130 §Consequences. The compressor `compiledConfig` mirrors fault/csrf/buffer structurally (6 listener fields + nested `compiledGzipConfig` + `compiledPerRoute` — heavier than buffer's 1 field per ADR-0126 but lighter than fault's 8 fields). Closure-capture + parse-at-New + read-only-shared-after-New discipline applies as-is. NO in-place edit of ADR-0101.
- **ADR-0102** (terminal-replace + StopIteration localReplyDone gate) — UNCHANGED in phase 14. Compressor does NOT use SendLocalReply (the encode-side flow is non-terminating; the EncodeData OverwriteBody primitive is body mutation, not response synthesis). NO in-place edit.
- **ADR-0117** + **ADR-0120** + **ADR-0124** + **ADR-0125** (the prior 5 canonical per-route disciplines) — UNCHANGED at the §Decision sections; ADR-0125 amendment paragraph §(viii)-(x) extends ADR-0125 with the SECOND-row discipline per the ALREADY-landed amendment at SPEC commit. ADR-0129 §Decision (vii) references ADR-0125 amendment explicitly. NO further in-place edits.
- **ADR-0128** (HCM framework primitives — synthetic empty-terminal RunDecodeData + post-body CL reconciliation) — UNCHANGED in phase 14. ADR-0128 retired phase-13's "no framework deltas" SPEC §4 invariant; phase 14 inherits the precedent (framework deltas are permitted in §9 family-rows when load-bearing). ADR-0131 §Decision (vi)+(vii) is the symmetric encode-side analog at ~20-25 LoC (vs phase-13 ADR-0128's ~34 LoC for decode-side). NO in-place edit.

These eleven cross-references land at the task that anchors each affected ADR (ADR-0129 + ADR-0130 at Task 2; ADR-0131 at Task 4; ADR-0132 at Task 8; ADR-0133 at Task 11). **NO in-place edit of any pre-existing ADR is required by phase 14** — this is consistent with phase 12 + phase 13, divergent from phases 10 + 11 (each of which amended ADR-0073).

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (per the user's persistent preference for git worktrees recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`). The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-14-http-filter-compressor-impl \
                 -b phase-14-http-filter-compressor-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-14-http-filter-compressor-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-14-http-filter-compressor-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-14-http-filter-compressor-impl -b phase-14-http-filter-compressor-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md commit `073cb88` and its SHA-fill follow-up `2b262b8` immediately before, then the BRAINSTORM.md commits `643294f` (initial brainstorm + ROADMAP row 14 + STATE.md flip) + `51b9ea6` (phase 13 squash-merge) + earlier phase 13 commits. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.23.0` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `133`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (the 5 anticipated ADRs ADR-0129..ADR-0133 already landed at SPEC commit; next-free is `134`). If it returns `128` or lower, the SPEC commit is missing — investigate.
5. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/14-http-filter-compressor/SPEC.md` returns `073cb88` (the SPEC commit) or descendant. If it returns a different SHA, the SPEC has been amended; re-read SPEC and re-verify §11 empirical pins are still valid.
6. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/14-http-filter-compressor/PLAN.md` returns the PLAN commit's SHA (filled at PLAN-session end) or descendant. If it returns a different SHA OR an earlier SHA than the SPEC, the PLAN has been amended — re-read PLAN.
7. **Pristine tree.** `git status --porcelain` returns empty. If not, commit or stash the uncommitted state before starting.
8. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
9. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014|Test.*0015'` returns every fixture PASS. The 16 pre-existing fixtures (0000–0015) are the regression baseline.
10. **Pre-existing fuzzers run clean at 30s.** The 17 fuzzers from phases 02–13 run clean. Phase 14 adds the eighteenth (`FuzzCompressorConfigParse` per Task 9).
11. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
12. **`envoy.extensions.filters.http.compressor.v3` proto package present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3 Compressor | head -5` returns the `Compressor` proto type's exported fields without an `import path failed` error; `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3 CompressorPerRoute | head -5` returns the `CompressorPerRoute` proto. Plus `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3 Gzip | head -5` returns the `Gzip` codec proto. If any `go doc` fails, the go-control-plane module needs `go mod download` (or `go mod tidy` if a version bump is needed; the SPEC reports the module is already in the closure at master `2b262b8`).
13. **Pre-existing `internal/filter/http/compressor/` directory does NOT exist.** `test ! -d internal/filter/http/compressor && echo "ok: compressor absent"` returns success. If non-empty, the package has been added by a concurrent phase — investigate before proceeding.
14. **Pre-existing `fixture.HTTPCompressor` does NOT exist.** `grep -nE 'HTTPCompressor' test/differential/fixture/fixture.go` returns 0 matches. If 1+, investigate.
15. **CONFORMANCE_PINS.md UNCHANGED.** `git diff master -- docs/envoy-go/CONFORMANCE_PINS.md` reports zero changes (D-3.7).
16. **Pre-existing `cmd/envoy-go/main.go` registers exactly the EIGHT filters expected at master `2b262b8`** — `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `8` matches: `router`, `buffer`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`. If 9+, another filter has been added concurrently; re-verify the registration ordering before adding the compressor line.
17. **Pre-existing `BEHAVIOR_CONTRACT.md` carries the phase-13 `### envoy.filters.http.buffer` subsection** — `grep -n '^### envoy.filters.http.buffer' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns 1 match. If 0 matches or different line, the file has drifted; re-read SPEC §13.1 to re-anchor the new compressor subsection insertion point.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the 5 ADRs ADR-0129..ADR-0133 ALREADY landed at SPEC commit in their final form; no ADR is anchored at Task 1; the PROGRESS preamble simply ANTICIPATES the 5 ADRs (with each ADR's Lands-in-task anchor reproduced from the per-ADR field) and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-14-http-filter-compressor-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition**

Run, in the worktree root:

```bash
git rev-parse --abbrev-ref HEAD                                       # expect: phase-14-http-filter-compressor-impl
git log --oneline master | head -10                                   # expect: PLAN SHA-fill, PLAN, SPEC SHA-fill (2b262b8), SPEC squash (073cb88), BRAINSTORM commits (643294f), phase-13 squash (51b9ea6), ...
docker version                                                        # expect: client + server reported
go version                                                            # expect: go1.23+
golangci-lint version                                                 # expect: 1.64.8
go test -count=1 -short ./...                                         # expect: every package PASS
go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|Test.*0002|Test.*0003|Test.*0004|Test.*0005|Test.*0006|Test.*0007a|Test.*0007b|Test.*0008|Test.*0009|Test.*0010|Test.*0011|Test.*0012|Test.*0013|Test.*0014|Test.*0015' -v
                                                                       # expect: every fixture PASS (16 fixtures)
grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
                                                                       # expect: 133
git log -1 --format=%H -- docs/envoy-go/phases/14-http-filter-compressor/SPEC.md
                                                                       # expect: 073cb88... or descendant
git status --porcelain                                                # expect: empty
test ! -d internal/filter/http/compressor && echo "ok: compressor absent"
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3 Compressor | head -5
                                                                       # expect: type Compressor struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3 CompressorPerRoute | head -5
                                                                       # expect: type CompressorPerRoute struct { ... }
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3 Gzip | head -5
                                                                       # expect: type Gzip struct { ... }
grep -cE 'HTTPCompressor' test/differential/fixture/fixture.go        # expect: 0
docker pull envoyproxy/envoy:v1.37.2                                  # expect: pull success
git diff master -- docs/envoy-go/CONFORMANCE_PINS.md                  # expect: empty
grep -cE 'httpReg.Register' cmd/envoy-go/main.go                      # expect: 8
grep -cn '^### envoy.filters.http.buffer' docs/envoy-go/BEHAVIOR_CONTRACT.md
                                                                       # expect: 1
```

If any line fails, stop and follow the precondition's "if fails" guidance.

- [ ] **Step 2: Author `PROGRESS.md` preamble**

Create `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` with the following structure:

````markdown
# Phase 14 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..13 PROGRESS.md structure.

## Preamble — execution preconditions

(Verbatim 17-precondition output captured during Task 1; all 17 green.)

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The 5 ADRs anticipated by SPEC §8 (ADR-0129..ADR-0133). **ALL FIVE ALREADY LANDED at SPEC commit `073cb88`** in their final form per ADR-0044 ADR-on-impl SPEC-time-anticipation discipline. Each Lands-in-task anchor per the per-ADR field at SPEC commit:

- **ADR-0129** `internal/filter/http/compressor/` package shape (single-token directory + ENCODER+DECODER `HTTPFilter` value with SAME `*filter` instance + 17-counter `filterStats` + boot-registration ordering router→buffer→compressor→cors→csrf→...) — Task 2
- **ADR-0130** `compiledConfig` shape + 8 consumed/12 ignored field decomposition + codec-library Any-unmarshal-and-dispatch + parse-rejection of unknown TypeURL + Gzip compression-level mapping table + envoy-go-only error wording — Task 2
- **ADR-0131** Body algorithm Path B (buffer-then-compress) + wire-shape divergence + `EncoderFilterCallbacks.OverwriteBody(b []byte)` framework primitive + min_content_length late-revert anomaly forward-pointer — Task 4 (framework primitive lands FIRST per cold-start prompt Critical PLAN-time obligation 2)
- **ADR-0132** 17-counter stat surface + namespace shape `compressor.<library_name>.<codec>.[response.]<counter>` + Rule SN2 reuse (NO new SN10) + per-route SHARED stats discipline — Task 8 (filterStats wiring; BEHAVIOR_CONTRACT 29→46 stat-table extension lands at Task 15)
- **ADR-0133** Differential-fixture decompress-and-compare body-assertion discipline — Task 11 (fixture infrastructure with ADR-0133 helpers; subsequent fixture tasks 12-14 consume the helpers)

**Plus ADR-0125 amendment paragraph §(viii)-(x)** — ALREADY landed at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent.

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The sixteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — `compressor.go` file split = TWO-WAY** (`compressor.go` + `acceptencoding.go`; the AE q-value parser is the most-self-contained primitive at ~100-130 LoC).
2. **D2 — `response_total_compressed_bytes` counter tolerance shape = (b) BOUNDARY-ONLY** (`0 < value < uncompressed_input_bytes` per side independently; structurally honest; no empirical calibration).
3. **D3 — min_content_length late-revert anomaly disposition = (a) ACCEPT THE WIRE-SHAPE ANOMALY + DOCUMENT AT §13.4** (structurally rare in envoy-go's framework; fixture 0016 sidesteps via direct_response routes carrying CL).
4. **D4 — Counter-emission shape on the late-revert-anomaly path = INCREMENT BOTH `response_content_length_too_small` AND `response_not_compressed`** (per SPEC §6.7 pseudocode + §11.5 empirical evidence on counter ratios).
5. **D5 — Library-name-empty embedded-namespace shape = MIRROR ENVOY VERBATIM** (consecutive dots → SN2 flatten produces `envoy_http_compressor__gzip_<counter>` double-underscore; Group 8 unit test verifies).
6. **D6 — `status_header_enabled: true` divergence-window observability = SILENT-IGNORE** (Group 1 unit test asserts; runtime divergence documented at §13.4).
7. **D7 — `compressor_library` per-route swap silent-ignore observability = ACCEPT-BUT-IGNORE AT PARSE** (Group 2 unit test asserts; documented at §13.4).
8. **D8 — filterStats struct field naming = Go-PASCALCASE matching counter-name suffix; bijective mapping** (HeaderCompressorOvershadowed, ..., RequestTotalUncompressedBytes; 17 fields total).
9. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: f` SAME *filter instance** (per ADR-0129 §Decision (iv); FIRST §9 row to use this shape with non-vacuous both paths structurally).
10. **PLAN-emerging — Filter-callback wiring hooks = BOTH SetDecoderCallbacks AND SetEncoderCallbacks; both store on the SAME *filter instance** (`f.dcb` for RequestRouteConfig per §6.4; `f.ecb` for OverwriteBody per §6.7).
11. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with SIX ROUTES** (4 direct_response + 1 cluster + 1 disabled per SPEC §7.2; mirrors phase 13 buffer's 3-route single-listener topology).
12. **PLAN-emerging — Echo-backend = NEW SHARED `test/helpers/echobackend/`** (NOT per-fixture; SPEC §1.1 amendment 4 + §4.2 design intent — phase-13 buffer's per-fixture backend MAY be migrated in future cleanup).
13. **PLAN-emerging — BackendKind enum value = `HTTPCompressor BackendKind = 13`** (continues existing naming convention; doc-comment notes shared echobackend helper).
14. **PLAN-emerging — Framework primitive `OverwriteBody` lands at TASK 4** (FIRST among impl tasks that consume; Tasks 5-7 consume; per cold-start prompt Critical PLAN-time obligation 2 + ADR-0131 Lands-in-task field).
15. **PLAN-emerging — ADR anchor schedule per ADR-0044** (ADR-0129+ADR-0130 at Task 2; ADR-0131 at Task 4; ADR-0132 at Task 8; ADR-0133 at Task 11; ALL 5 ADRs ALREADY LANDED at SPEC commit in final form).
16. **PLAN-emerging — Acceptance discipline at the per-task level = each task's acceptance bullet enumerates verbatim verification commands AND expected-output anchors** (per phase-13 PLAN per-task acceptance precedent).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `<SHA>` — `phase 14: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified all 17 preconditions per PLAN §"Execution preconditions"; phase-14 SPEC + PLAN confirmed present in HEAD; SPEC at 073cb88; ADR tail at 0133 (next-free 0134; the 5 phase-14 ADRs already landed at SPEC commit per ADR-0044 SPEC-time-anticipation); `internal/filter/http/compressor/` absent (Task 2 lands); `fixture.HTTPCompressor` absent (Task 10 lands); `test/helpers/echobackend/` absent (Task 10 lands the new shared helper). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs already landed at SPEC commit in final form per per-ADR Lands-in-task anchors).

**Outputs:** (17 verbatim command outputs captured.)
````

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: PROGRESS preamble + planner-time decision resolution

Authors PROGRESS.md per ADR-0044 ADR-on-impl convention; verifies all 17
preconditions per PLAN §"Execution preconditions"; preamble enumerates
the 5 anticipated ADRs (ADR-0129..ADR-0133, ALL ALREADY LANDED at SPEC
commit 073cb88 per ADR-0044 SPEC-time-anticipation discipline) + the
ADR-0125 amendment §(viii)-(x) (also already landed at SPEC commit) +
the 16 planner-time decisions resolution (D1-D8 from SPEC §12 + 8
PLAN-emerging items). No ADR landed in Task 1; ADRs anchor at first-use
impl tasks per per-ADR Lands-in-task fields (Task 2: ADR-0129+ADR-0130;
Task 4: ADR-0131 framework primitive; Task 8: ADR-0132 stats; Task 11:
ADR-0133 fixture).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
git log -1 --format=%H -- docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
                                                                       # expect: the just-committed SHA
git status --porcelain                                                 # expect: empty
```

---

## Task 2: `internal/filter/http/compressor/` package — `doc.go` + `compressor.go` skeleton (TypeURL, types, compiledConfig + compiledPerRoute + compiledGzipConfig + parsePerRoute + New factory + codec-library Any-unmarshal-and-dispatch + 17-counter filterStats registration) + `compressor_test.go` Group 1 + Group 2 + Group 3 tests [ADR-0129, ADR-0130]

**Files:**
- Create: `internal/filter/http/compressor/doc.go`
- Create: `internal/filter/http/compressor/compressor.go`
- Create: `internal/filter/http/compressor/compressor_test.go`
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md` (append Task 2 entry)

This task lands the package skeleton + `New` factory PGV-mirror + `parsePerRoute` + `unmarshalCompressorLibrary` + `buildCompiledGzipConfig` + 17-counter `filterStats` struct + `newFilterStats` registration helper per SPEC §6.1-§6.3 + ADR-0129 §Decision (i)-(vii) + ADR-0130 §Decision (i)-(vi). Per TDD discipline: write Group 1 + Group 2 + Group 3 tests FIRST; verify they FAIL (no package exists); then land doc.go + compressor.go skeleton. ADR-0129 + ADR-0130 reference at this commit per per-ADR Lands-in-task field at SPEC commit (the ADRs themselves already exist verbatim at SPEC commit `073cb88`; this task does NOT re-author).

**Precondition:** Task 1 commit on HEAD; pristine tree; `internal/filter/http/compressor/` does not exist.
**Artifacts:** doc.go, compressor.go (skeleton with `compiledConfig`, `compiledPerRoute`, `compiledGzipConfig`, `filterStats`, `filter`, `TypeURL`, `New`, `unmarshalCompressorLibrary`, `buildCompiledGzipConfig`, `parsePerRoute`, `newFilterStats`, stubs for `DecodeHeaders` / `DecodeData` / `DecodeTrailers` / `EncodeHeaders` / `EncodeData` / `EncodeTrailers` / `SetDecoderCallbacks` / `SetEncoderCallbacks` / `OnDestroy`, `defaultContentTypes` package-level var), compressor_test.go (Group 1 + 2 + 3), Task 2 PROGRESS entry.
**Acceptance:** `go build ./internal/filter/http/compressor/...` clean; `go vet ./internal/filter/http/compressor/...` clean; `golangci-lint run ./internal/filter/http/compressor/...` clean; `go test -race -count=1 -v ./internal/filter/http/compressor/` shows Group 1 + 2 + 3 tests PASS; `grep -nE '^## ADR-0129' docs/envoy-go/DECISIONS.md` returns 1 match (verifying ADR text intact at HEAD); `grep -nE '^## ADR-0130' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0129 + ADR-0130 `Lands-in-task: Task 2` fields verified intact via `grep -nE 'Lands-in-task' docs/envoy-go/DECISIONS.md | grep -E '0129|0130'` returning 2 matches each naming Task 2; Task 2 entry appended to PROGRESS.md.

- [ ] **Step 1: Write the failing tests (Group 1 + Group 2 + Group 3)**

Create `internal/filter/http/compressor/compressor_test.go` with the test groups per SPEC §14.1 + planner-time decision 1 split. Group 1 covers New factory + buildCompiledConfig (≈19 tests); Group 2 covers parsePerRoute (≈8 tests); Group 3 covers buildCompiledGzipConfig (≈7 tests). Skeleton:

```go
package compressor

import (
	"testing"

	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// --- Group 1: New factory + buildCompiledConfig ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("expected error on nil tc")
	}
}

// (additional Group 1 tests per SPEC §14.1 Group 1: 19 tests covering
// PGV-mirror discipline + default values + silent-ignore semantics — see
// PLAN.md §"compressor_test.go" Group 1 enumeration for the full list.)

// --- Group 2: parsePerRoute ---

func TestParsePerRoute_Disabled_Parses(t *testing.T) {
	cpr, err := parsePerRoute(&compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Disabled{Disabled: true},
	})
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if !cpr.disabled { t.Error("expected disabled=true") }
}

// (additional Group 2 tests: 8 total per SPEC §14.1 Group 2 + planner-time
// decision 7 — `TestParsePerRoute_Overrides_LibrarySwap_SilentIgnored` etc.)

// --- Group 3: buildCompiledGzipConfig ---

func TestBuildGzipConfig_DefaultLevel(t *testing.T) {
	g := &gzipv3.Gzip{} // all defaults
	cfg, err := buildCompiledGzipConfig(g)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if cfg.level != -1 { t.Errorf("expected level=-1 (default); got %d", cfg.level) }
	if cfg.huffmanOnly { t.Error("expected huffmanOnly=false on default strategy") }
}

// (additional Group 3 tests: 7 total per SPEC §14.1 Group 3 — level
// enum mapping per ADR-0130 §Decision (iv) verbatim; HUFFMAN_ONLY honored
// per §Decision (v); memory_level/window_bits/chunk_size silent-ignored.)
```

Implementer expands the test skeleton to the full ~34-test surface enumerated in PLAN's `compressor_test.go` row of the file-structure table (Groups 1-3); test helpers `mustAny(t, msg proto.Message) *anypb.Any` + `freshFactoryCtx() envoyhttp.FactoryCtx` mirror the phase-13 buffer / phase-12 csrf precedents. Reference: phase-13 buffer's `buffer_test.go` Group 1 + 2 (~32 test cases) + phase-12 csrf's PGV-mirror discipline.

- [ ] **Step 2: Run tests; verify Groups 1-3 FAIL (no package exists)**

```bash
go test -race -count=1 -v ./internal/filter/http/compressor/
# expect: BUILD FAIL (package does not exist) — every test fails to compile
```

- [ ] **Step 3: Author `doc.go`**

Create `internal/filter/http/compressor/doc.go` per the file-structure table row above (~30 LoC):

```go
// Package compressor implements envoy.filters.http.compressor — Envoy
// v1.37.2's canonical "gzip-compress upstream response body before forwarding
// downstream" filter (gzip-only MVP, response-side only) under the 07.1
// HTTP filter framework.
//
// Listener-level Compressor proto (9 top-level fields):
//   - 6 consumed: compressor_library (TypedExtensionConfig REQUIRED) +
//     response_direction_config.{common_config.{min_content_length,
//     content_type}, disable_on_etag_header, remove_accept_encoding_header,
//     uncompressible_response_codes}.
//   - 9 silent-ignored: 4 deprecated top-level mirrors (content_length,
//     content_type, disable_on_etag_header, remove_accept_encoding_header) +
//     runtime_enabled (deprecated runtime gate) + choose_first (always-q-
//     value) + request_direction_config (always-disabled, response-only MVP)
//     + response_direction_config.common_config.enabled (RuntimeFeatureFlag
//     with BoolValue default; always-active per §1.1 amendment 2) +
//     response_direction_config.status_header_enabled (always-no-status-
//     header per §1.1 amendment 1).
//   - 1 case parse-rejected: compressor_library.typed_config with non-Gzip
//     TypeURL (envoy-go-only validation per ADR-0130 §Decision (vi)).
//
// Codec-library Gzip proto (5 fields):
//   - 2 consumed: compression_level (mapped to Go compress/gzip levels per
//     ADR-0130 §Decision (iv)) + compression_strategy (HUFFMAN_ONLY honored
//     / others collapse to default per §Decision (v)).
//   - 3 silent-ignored: memory_level, window_bits, chunk_size (Go gzip does
//     not expose libz-equivalent knobs).
//
// Per-route CompressorPerRoute proto: oneof override carrying disabled: true
// shortcut OR overrides: CompressorOverrides. CompressorOverrides carries
// response_direction_config: ResponseDirectionOverrides (only
// remove_accept_encoding_header BoolValue per §1.1 amendment 4 — per-route
// override of min_content_length/content_type/disable_on_etag_header/
// uncompressible_response_codes/enabled/status_header_enabled is
// STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's proto) + compressor_library
// TypedExtensionConfig (silent-ignored at parse + runtime per §1.1 amendment
// 4 + ADR-0125 amendment §(viii)).
//
// HTTPFilter value: ENCODER+DECODER with SAME *filter instance — Decoder: f,
// Encoder: f (FIRST §9 row to use this shape with non-vacuous both paths
// structurally). Per ADR-0129 §Decision (iv).
//
// Body algorithm: Path B (buffer-then-compress) per ADR-0131 §Decision (i) —
// EncodeData on willCompress=true + endStream=true + len(data) >=
// min_content_length gzip-encodes via gzip.NewWriterLevel(buf, level) and
// emits via cb.OverwriteBody(buf.Bytes()). The OverwriteBody primitive is
// the FIRST encode-side framework primitive in envoy-go (~20-25 LoC across
// callbacks.go + chain.go + connection.go + h2dispatch.go per ADR-0131
// §Decision (vi); symmetric to phase-13 ADR-0128 decode-side primitives).
//
// Wire-shape divergence-window from reference Envoy (deliberate per ADR-0131
// §Decision (ii)): envoy-go emits Content-Encoding: gzip + fixed Content-
// Length: <gzipped-len> + identity transfer; Envoy emits Content-Encoding:
// gzip + Transfer-Encoding: chunked + no CL. Decompressed body bytes are
// byte-equivalent (gzip multi-encoding spec admits both Go compress/gzip and
// libz outputs as valid per §11.14); compressed body bytes are NOT byte-
// equivalent. Differential fixture 0016 uses decompress-and-compare body
// assertion per ADR-0133.
//
// Per-route discipline: 5th canonical disabled-OR-override per ADR-0125 §
// Decision (vi) + ADR-0125 amendment §(viii) (SECOND row using; per-route
// override surface FILTER-SPECIFIC and NARROWER than listener-level).
// Per-route stats SHARED with listener-level per ADR-0132 §Decision (iv).
//
// Stat surface: 17 counters per HCM stat_prefix per ADR-0132 §Decision (i).
// Namespace: compressor.<library_name>.gzip.[response.]<counter>; Prometheus
// rendering via existing Rule SN2 (NO new SN10 per §1.1 amendment 3).
package compressor
```

- [ ] **Step 4: Author `compressor.go` skeleton**

Create `internal/filter/http/compressor/compressor.go` with the public surface + types + helpers + stub method bodies per SPEC §6.1-§6.3 + §6.9 + ADR-0129 §Decision (i)-(vii) + ADR-0130 §Decision (i)-(vi). The skeleton (Encode/Decode method bodies are STUBS returning Continue/DataContinue/TrailersContinue; Tasks 5-7 land the real bodies):

```go
package compressor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"regexp"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical envoy.filters.http.compressor typed_config type URL.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"

const filterName = "envoy.filters.http.compressor"

const gzipLibraryTypeURL = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"

// defaultContentTypes is the 8-entry default content_type list per SPEC
// §11.1 (compressor.proto v1.37.2 line 49-65 verbatim).
var defaultContentTypes = []string{
	"application/javascript",
	"application/json",
	"application/xhtml+xml",
	"image/svg+xml",
	"text/css",
	"text/html",
	"text/plain",
	"text/xml",
}

// strongEtagPattern + weakEtagPattern per SPEC §6.6 + §1.1 amendment 6.
var (
	strongEtagPattern = regexp.MustCompile(`^"[^"]*"$`)
	weakEtagPattern   = regexp.MustCompile(`^W/"[^"]*"$`)
)

// (compiledConfig, compiledPerRoute, compiledGzipConfig, filter, filterStats
// types + New + unmarshalCompressorLibrary + buildCompiledGzipConfig +
// parsePerRoute + newFilterStats + Encode/Decode method stubs go here.)

// (use suppressed-unused-imports placeholders to keep skeleton compiling
// before Tasks 5-7 land the real method bodies; remove placeholders as the
// methods get fleshed out.)
var _ = bytes.NewBuffer
var _ = gzip.DefaultCompression
var _ = (*compressorv3.Compressor)(nil)
var _ = (*gzipv3.Gzip)(nil)
var _ = (*corev3.TypedExtensionConfig)(nil)
var _ = proto.Message(nil)
var _ = (*anypb.Any)(nil)
var _ = (*stats.Registry)(nil)
var _ = (*envoyhttp.FactoryCtx)(nil)
var _ = http.Header(nil)
var _ = fmt.Errorf

func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, fmt.Errorf("compressor: invalid typed_config: nil")
	}
	var cfg compressorv3.Compressor
	if err := tc.UnmarshalTo(&cfg); err != nil {
		return nil, fmt.Errorf("compressor: invalid typed_config: %w", err)
	}
	gzipCfg, libraryName, err := unmarshalCompressorLibrary(cfg.GetCompressorLibrary())
	if err != nil { return nil, err }
	compiled, err := buildCompiledConfig(&cfg, gzipCfg, libraryName)
	if err != nil { return nil, err }
	listenerStats := newFilterStats(ctx.Stats, ctx.StatPrefix, libraryName)
	return func() envoyhttp.HTTPFilter {
		f := &filter{config: compiled, stats: listenerStats}
		return envoyhttp.HTTPFilter{
			Name:     filterName,
			Decoder:  f,
			Encoder:  f,
			PerRoute: parsePerRoute,
		}
	}, nil
}

// (parsePerRoute, unmarshalCompressorLibrary, buildCompiledConfig,
// buildCompiledGzipConfig, newFilterStats helpers + Encode/Decode method
// stubs land per SPEC §6 verbatim per the file-structure table.)
```

NOTE: the skeleton above is a SHELL; the implementer fleshes it out per SPEC §6.1-§6.3 + §6.9 with the full `compiledConfig`/`compiledPerRoute`/`compiledGzipConfig`/`filter`/`filterStats` struct definitions + `parsePerRoute` body per §6.3 + `unmarshalCompressorLibrary` + `buildCompiledConfig` + `buildCompiledGzipConfig` + `newFilterStats` per ADR-0130 §Decision (ii)-(v) + the stub Encode/Decode methods. The factory signature returns `(envoyhttp.FilterInstanceFactory, error)` to match `envoyhttp.HTTPFilterFactory`. Reference: phase-13 buffer's `buffer.go` Task 2 skeleton + phase-12 csrf's `csrf.go` Task 2 for the stylistic discipline.

- [ ] **Step 5: Run tests; verify Groups 1-3 PASS**

```bash
go vet ./internal/filter/http/compressor/...
golangci-lint run ./internal/filter/http/compressor/...
go test -race -count=1 -v ./internal/filter/http/compressor/
# expect: all Group 1 + 2 + 3 tests PASS
```

- [ ] **Step 6: Verify ADR-0129 + ADR-0130 intact at HEAD**

```bash
grep -nE '^## ADR-0129' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE '^## ADR-0130' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task.*Task 2' docs/envoy-go/DECISIONS.md | head -5   # expect: ADR-0129 + ADR-0130 lines
```

- [ ] **Step 7: Append Task 2 entry to PROGRESS.md**

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor package skeleton — doc.go + compressor.go (TypeURL, types, factory, parsePerRoute, codec dispatch, 17-counter filterStats) + Group 1+2+3 tests [ADR-0129, ADR-0130]

Lands the package skeleton + New factory PGV-mirror per SPEC §6.1 +
ADR-0130 §Decision (i)-(vi); parsePerRoute oneof discipline per §6.3 +
ADR-0125 amendment §(viii) (per-route override surface NARROWER than
listener-level — only remove_accept_encoding_header BoolValue);
unmarshalCompressorLibrary codec-library Any-unmarshal-and-dispatch per
ADR-0130 §Decision (ii)-(iii) (envoy-go-only parse-rejection of non-Gzip
TypeURLs with envoy-go-own error wording); buildCompiledGzipConfig
compression-level enum mapping per ADR-0130 §Decision (iv); 17-counter
filterStats struct registered at New factory time per ADR-0132 §Decision
(i) (full registration deferred to Task 8 — this task just declares the
struct + skeleton newFilterStats helper). doc.go + compressor.go
skeleton + Group 1+2+3 unit tests (~34 tests total) PASS.

ADR-0129 + ADR-0130 already exist at SPEC commit 073cb88 in final form
per ADR-0044 ADR-on-impl SPEC-time-anticipation; this task references via
commit message + verifies Lands-in-task: Task 2 anchor intact at HEAD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
go test -race -count=1 ./internal/filter/http/compressor/   # expect: Groups 1+2+3 PASS
git log -1 --format=%H -- internal/filter/http/compressor/compressor.go   # expect: just-committed SHA
```

---

## Task 3: Accept-Encoding q-value parser — `acceptencoding.go` + Group 4 tests

**Files:**
- Create: `internal/filter/http/compressor/acceptencoding.go`
- Modify: `internal/filter/http/compressor/compressor_test.go` (append Group 4 tests)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the Accept-Encoding q-value parser per RFC 7231 §5.3.4 + classification dispatch per planner-time decision 1 file split + SPEC §6.4 + §11.4. NO new ADR (the parser is internal to the filter; ADR-0130 §Decision (i) covers; the file-split is per planner-time decision 1).

**Precondition:** Task 2 commit on HEAD; Groups 1-3 PASS; pristine tree.
**Artifacts:** acceptencoding.go (q-value parser + classification dispatch + helper sub-functions per the file-structure table row), Group 4 tests (~12 unit-test cases), PROGRESS Task 3 entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/compressor/` shows Groups 1-4 PASS; `go vet` + `golangci-lint` clean.

- [ ] **Step 1: Append Group 4 tests**

Append to `compressor_test.go`:

```go
// --- Group 4: Accept-Encoding parser (per SPEC §6.4 + §11.4) ---

func TestParseAcceptEncoding_Empty(t *testing.T) {
	enc, cls := parseAcceptEncoding("")
	if enc != "" || cls != "no_accept_header" {
		t.Errorf("expected (\"\", \"no_accept_header\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipExplicit(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_Identity(t *testing.T) {
	enc, cls := parseAcceptEncoding("identity")
	if enc != "" || cls != "identity" {
		t.Errorf("expected (\"\", \"identity\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_Wildcard(t *testing.T) {
	enc, cls := parseAcceptEncoding("*")
	if enc != "gzip" || cls != "wildcard" {
		t.Errorf("expected (\"gzip\", \"wildcard\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_MultiCodingSortedByQValue(t *testing.T) {
	// gzip;q=0.5, br;q=1.0 — br has higher q but is NOT configured (gzip-only MVP);
	// gzip wins. Per §11.4 confirmation.
	enc, cls := parseAcceptEncoding("gzip;q=0.5, br;q=1.0")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipQ0_Blocks(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=0")
	if enc != "" {
		t.Errorf("expected blocked gzip on q=0; got %q", enc)
	}
	// classification is implementation-detail: SPEC §11.4 verbatim shows q=0 → "no compression";
	// the parser MAY classify as overshadowed OR not_valid OR identity-fallback per RFC.
	// Per SPEC §11.15 trichotomy, the AE-side-skip path classification triggers Vary injection.
	_ = cls
}

func TestParseAcceptEncoding_MalformedQValue_NotValid(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=blah")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_BrOnly_Overshadowed(t *testing.T) {
	enc, cls := parseAcceptEncoding("br")
	if enc != "" || cls != "overshadowed" {
		t.Errorf("expected (\"\", \"overshadowed\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_CaseInsensitiveToken(t *testing.T) {
	enc, _ := parseAcceptEncoding("GZIP")
	if enc != "gzip" {
		t.Errorf("expected case-insensitive token match; got %q", enc)
	}
}

func TestParseAcceptEncoding_WhitespaceTolerance(t *testing.T) {
	enc, cls := parseAcceptEncoding("  gzip  ; q=0.5  ,  br  ; q=1.0  ")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected whitespace-tolerant parsing; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_TrailingSemicolon(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected trailing-semicolon tolerance; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_MultipleEntriesSameCoding(t *testing.T) {
	// "gzip, gzip;q=0.5" — first entry wins by declared order (q-value tie at 1.0 for first; explicit 0.5 for second).
	enc, _ := parseAcceptEncoding("gzip, gzip;q=0.5")
	if enc != "gzip" {
		t.Errorf("expected gzip selection; got %q", enc)
	}
}
```

- [ ] **Step 2: Run tests; verify Group 4 FAILS (parser not implemented)**

```bash
go test -race -count=1 -v ./internal/filter/http/compressor/
# expect: BUILD FAIL (parseAcceptEncoding not defined) — every Group 4 test fails to compile
```

- [ ] **Step 3: Author `acceptencoding.go`**

Create `internal/filter/http/compressor/acceptencoding.go` per the file-structure table row above (~100-130 LoC):

```go
package compressor

import (
	"strconv"
	"strings"
)

// encodingEntry captures one (coding, q-value) pair parsed from an
// Accept-Encoding header.
type encodingEntry struct {
	coding string  // lowercase token; "*" preserved as wildcard
	qValue float64 // [0.0, 1.0]; default 1.0 absent
	valid  bool    // false on parse error
}

// parseAcceptEncoding parses an Accept-Encoding header per RFC 7231 §5.3.4
// and returns the negotiated encoding ("gzip" / "" / "identity") + the
// classification token used by the 6 header_* counter increments (per SPEC
// §6.4 + §11.5 verbatim probeA evidence). Empty header → ("", "no_accept_header").
func parseAcceptEncoding(header string) (selected string, classification string) {
	if strings.TrimSpace(header) == "" {
		return "", "no_accept_header"
	}
	entries := []encodingEntry{}
	for _, raw := range strings.Split(header, ",") {
		entry := parseEncoding(raw)
		if !entry.valid {
			return "", "not_valid"
		}
		entries = append(entries, entry)
	}
	return selectByQValue(entries)
}

// parseEncoding parses one chunk like "gzip;q=0.5" or " *; q=1.0 " or
// "identity" into an encodingEntry. Returns valid=false on q-value parse
// error.
func parseEncoding(raw string) encodingEntry {
	parts := strings.Split(raw, ";")
	token := strings.ToLower(strings.TrimSpace(parts[0]))
	if token == "" {
		return encodingEntry{valid: false}
	}
	q := 1.0
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "q=") {
			continue
		}
		val, ok := parseQValue(p[2:])
		if !ok {
			return encodingEntry{valid: false}
		}
		q = val
	}
	return encodingEntry{coding: token, qValue: q, valid: true}
}

// parseQValue parses the q= parameter value as float64 in [0.0, 1.0].
// Returns (q, true) on success; (0, false) on parse error or out-of-range.
func parseQValue(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if v < 0 || v > 1 {
		return 0, false
	}
	return v, true
}

// selectByQValue walks the parsed entries and returns the negotiated
// encoding + classification per the dispatch rules at SPEC §6.4 + §11.5:
//   - identity selected (q>0 with no higher-q gzip / wildcard)        → ("", "identity")
//   - gzip explicit (q>0)                                              → ("gzip", "compressor_used")
//   - wildcard "*" (q>0; no explicit gzip)                             → ("gzip", "wildcard")
//   - all q=0 / nothing selectable but recognized non-configured codecs → ("", "overshadowed")
//   - all q=0 / nothing recognized                                      → ("", "no_accept_header")
func selectByQValue(parsed []encodingEntry) (selected string, classification string) {
	// (full selection algorithm per SPEC §6.4 + §11.5; implementer fleshes
	// out the dispatch table here. Reference: §11.4 verbatim probe transcripts
	// for the boundary cases.)
	// ...
	return "", "no_accept_header" // skeleton placeholder
}
```

NOTE: implementer fleshes out the `selectByQValue` body per the SPEC §6.4 + §11.5 + §11.4 dispatch rules verbatim. The 6 classifications match the 6 `header_*` counter names.

- [ ] **Step 4: Run tests; verify Groups 1-4 PASS**

```bash
go vet ./internal/filter/http/compressor/...
golangci-lint run ./internal/filter/http/compressor/...
go test -race -count=1 -v ./internal/filter/http/compressor/
# expect: all Group 1, 2, 3, 4 tests PASS
```

- [ ] **Step 5: Append Task 3 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor Accept-Encoding q-value parser — acceptencoding.go + Group 4 tests

Lands acceptencoding.go per planner-time decision 1 file split (compressor.go
+ acceptencoding.go); parser implements RFC 7231 §5.3.4 q-value parsing per
SPEC §6.4 + §11.4 + §11.5; the 6 classification tokens (compressor_used,
overshadowed, identity, wildcard, no_accept_header, not_valid) map verbatim
to the 6 header_* counter names per ADR-0132 §Decision (i). Group 4 tests
(12 unit-test cases) covering empty AE / explicit gzip / identity / wildcard
/ multi-coding sorting / q=0 blocking / malformed q-value / br-overshadowed
/ case-insensitive / whitespace-tolerance / trailing-semicolon / multiple-
same-coding PASS.

NO new ADR (parser is internal to the filter; ADR-0130 §Decision (i) covers).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
go test -race -count=1 ./internal/filter/http/compressor/   # expect: Groups 1+2+3+4 PASS
git log -1 --format=%H -- internal/filter/http/compressor/acceptencoding.go   # expect: just-committed SHA
```

---

## Task 4: Framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` — interface method + chain.go impl + connection.go H1 harvest + h2dispatch.go H2 harvest + chain_test.go probe-filter integration tests [ADR-0131 §Decision (vi)]

**Files:**
- Modify: `internal/filter/http/callbacks.go` (+1 LoC interface method)
- Modify: `internal/filter/http/chain.go` (+8 LoC impl + per-stream field + accessor)
- Modify: `internal/filter/http/chain_test.go` (+60 LoC probe-filter integration tests)
- Modify: `internal/filter/hcm/connection.go` (+8 LoC H1 harvest)
- Modify: `internal/filter/hcm/h2dispatch.go` (+8 LoC H2 harvest)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the encode-side framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` per ADR-0131 §Decision (vi). Per cold-start prompt Critical PLAN-time obligation 2 + planner-time decision 14: **the framework primitive lands FIRST** (Task 4) so subsequent tasks (5-7 EncodeHeaders/EncodeData/DecodeHeaders) can consume it; mirrors phase-13 ADR-0128 timing-shift lesson (phase-13 retroactively-anchored at Task 12 because the framework deltas were unanticipated by phase-13 SPEC §4; phase-14 SPEC §3 framework-survey explicitly anticipated the framework primitive at SPEC time and ADR-0131 anchored Lands-in-task: Task 4 at SPEC commit). ADR-0131 reference at this commit per the per-ADR Lands-in-task field at SPEC commit.

**Precondition:** Task 3 commit on HEAD; pristine tree; Groups 1-4 PASS.
**Artifacts:** Updated callbacks.go (+1 LoC interface method) + chain.go (+8 LoC impl + per-stream field + accessor) + chain_test.go (+60 LoC probe-filter integration tests) + connection.go (+8 LoC H1 harvest) + h2dispatch.go (+8 LoC H2 harvest), PROGRESS Task 4 entry.
**Acceptance:** `go vet ./...` + `golangci-lint run ./...` + `go test -race -count=1 ./internal/filter/http/...` + `go test -race -count=1 ./internal/filter/hcm/...` all PASS; the new probe-filter integration tests in chain_test.go PASS (3 cases: probe sets override → harvest returns the bytes; probe does not set → harvest returns (nil, false); override survives subsequent EncodeData chunks); `grep -nE '^## ADR-0131' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0131 `Lands-in-task: Task 4` field verified intact via `grep -nE 'Lands-in-task' docs/envoy-go/DECISIONS.md | grep 0131`.

- [ ] **Step 1: Write the failing tests (probe-filter chain integration)**

Append to `internal/filter/http/chain_test.go`:

```go
// --- ADR-0131 §Decision (vi) OverwriteBody primitive integration tests ---

// overwriteBodyProbe is a test-only StreamEncoderFilter that calls
// OverwriteBody on its callback during EncodeData. Used to verify the
// chain dispatch substitutes resp.Body before wire-write.
type overwriteBodyProbe struct {
	cb       envoyhttp.EncoderFilterCallbacks
	override []byte
	called   bool
}

func (p *overwriteBodyProbe) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}
func (p *overwriteBodyProbe) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if p.override != nil {
		p.cb.OverwriteBody(p.override)
		p.called = true
	}
	return envoyhttp.DataContinue
}
func (p *overwriteBodyProbe) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (p *overwriteBodyProbe) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { p.cb = cb }
func (p *overwriteBodyProbe) OnDestroy() {}

func TestEncoderCB_OverwriteBody_StoresBytes_AccessorReflects(t *testing.T) {
	probe := &overwriteBodyProbe{override: []byte("REPLACED")}
	chain := newTestChain(t, []envoyhttp.HTTPFilter{{Name: "probe", Encoder: probe}})
	chain.RunEncodeData(nil, []byte("ORIGINAL"), true)
	body, ok := chain.EncodeBodyOverride()
	if !ok {
		t.Fatal("expected EncodeBodyOverride() to return ok=true")
	}
	if string(body) != "REPLACED" {
		t.Errorf("expected body=\"REPLACED\"; got %q", body)
	}
	if !probe.called {
		t.Error("expected probe to have called OverwriteBody")
	}
}

func TestEncoderCB_NoOverwriteBody_AccessorReturnsFalse(t *testing.T) {
	probe := &overwriteBodyProbe{override: nil} // does NOT call OverwriteBody
	chain := newTestChain(t, []envoyhttp.HTTPFilter{{Name: "probe", Encoder: probe}})
	chain.RunEncodeData(nil, []byte("ORIGINAL"), true)
	body, ok := chain.EncodeBodyOverride()
	if ok {
		t.Errorf("expected EncodeBodyOverride() to return ok=false; got body=%q", body)
	}
	if body != nil {
		t.Errorf("expected nil body when not overridden; got %q", body)
	}
}

func TestEncoderCB_OverwriteBody_PassthroughOnSubsequentInvocations(t *testing.T) {
	// override survives on a chain that runs RunEncodeData multiple times
	// (relevant for future non-MVP scenarios; current MVP invokes once).
	probe := &overwriteBodyProbe{override: []byte("REPLACED")}
	chain := newTestChain(t, []envoyhttp.HTTPFilter{{Name: "probe", Encoder: probe}})
	chain.RunEncodeData(nil, []byte("CHUNK1"), false)
	chain.RunEncodeData(nil, []byte("CHUNK2"), true)
	body, ok := chain.EncodeBodyOverride()
	if !ok || string(body) != "REPLACED" {
		t.Errorf("expected ok=true + REPLACED; got (ok=%v, body=%q)", ok, body)
	}
}
```

NOTE: the `newTestChain` test-helper lives in chain_test.go's existing test infrastructure; if not present, implementer adds it per the existing chain_test.go test-helpers precedent (mirrors phase-13's ADR-0128 chain_test.go helpers).

- [ ] **Step 2: Run tests; verify the new tests FAIL (OverwriteBody primitive does not exist)**

```bash
go test -race -count=1 -v ./internal/filter/http/ -run TestEncoderCB
# expect: BUILD FAIL (OverwriteBody method on EncoderFilterCallbacks does not exist) — tests fail to compile
```

- [ ] **Step 3: Add `OverwriteBody(b []byte)` interface method to `EncoderFilterCallbacks`**

Edit `internal/filter/http/callbacks.go` to add the new method on the `EncoderFilterCallbacks` interface (lines 68-81 in the existing file):

```go
// EncoderFilterCallbacks is the framework-supplied callback shape for
// encode-side filters.
type EncoderFilterCallbacks interface {
	// ContinueEncoding wakes the dispatch goroutine if it is parked on an
	// encode-side StopIteration return. Same coalescing discipline as
	// ContinueDecoding.
	ContinueEncoding()

	// EncodeHeaders / EncodeData / EncodeTrailers are encode-side injection
	// methods (rare).
	EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
	EncodeData(data []byte, endStream bool) FilterDataStatus
	EncodeTrailers(trailers http.Header)

	// OverwriteBody registers a replacement encode-side body. Filters MUST
	// call this only from inside their EncodeData(data, endStream)
	// implementation; the chain dispatch substitutes resp.Body before the
	// wire-write path consumes it. Not goroutine-safe — the encode chain
	// runs synchronously in the dispatch goroutine.
	//
	// Per ADR-0131 §Decision (vi); first encode-side framework primitive in
	// envoy-go (phase 14; symmetric to phase-13 ADR-0128 decode-side
	// primitives).
	OverwriteBody(b []byte)
}
```

- [ ] **Step 4: Add `encodeBodyOverride` field + sentinel + `OverwriteBody` impl + accessor on `*FilterChain`**

Edit `internal/filter/http/chain.go` per ADR-0131 §Decision (vi):

```go
// (in *FilterChain struct definition, add):
encodeBodyOverride   []byte // per-stream encode-body override per ADR-0131
encodeBodyOverridden bool   // sentinel flag to discriminate (nil-bytes, set) from (no override)

// (encoderCB struct already exists; add OverwriteBody method on it):
func (c *encoderCB) OverwriteBody(b []byte) {
	c.chain.encodeBodyOverride = b
	c.chain.encodeBodyOverridden = true
}

// (new accessor method on *FilterChain):

// EncodeBodyOverride returns the registered encode-side body override (if any).
// Returns (override, true) if a filter called OverwriteBody during encoding;
// (nil, false) otherwise. Callers (HCM dispatch) use this to substitute
// resp.Body before the wire-write path. Per ADR-0131 §Decision (vi).
func (c *FilterChain) EncodeBodyOverride() ([]byte, bool) {
	return c.encodeBodyOverride, c.encodeBodyOverridden
}
```

NOTE: the `encoderCB` struct's `chain` back-reference is presumed; implementer adapts to the actual chain.go encoderCB struct shape (mirrors the existing decoderCB's discipline per phase-13 ADR-0128).

- [ ] **Step 5: Run tests; verify chain_test.go probe tests PASS**

```bash
go vet ./internal/filter/http/...
golangci-lint run ./internal/filter/http/...
go test -race -count=1 -v ./internal/filter/http/ -run TestEncoderCB
# expect: 3 new tests PASS
```

- [ ] **Step 6: Add HCM-side post-RunEncodeData harvest at H1 path**

Edit `internal/filter/hcm/connection.go` between line ~472 (RunEncodeData call) and line ~478 (writeH1Reply call) per ADR-0131 §Decision (vi):

```go
// After chain.RunEncodeData returns, harvest any encode-body override.
// Per ADR-0131 §Decision (vi): filters that compress / transform the
// response body register the new bytes via cb.OverwriteBody(b); the chain
// stores them on c.encodeBodyOverride; HCM substitutes resp.Body before
// writeH1Reply consumes it.
if override, ok := chain.EncodeBodyOverride(); ok {
	resp.Body = override
}
```

- [ ] **Step 7: Add HCM-side post-RunEncodeData harvest at H2 path (symmetric)**

Edit `internal/filter/hcm/h2dispatch.go` between line ~310 (RunEncodeData call) and line ~328 (writeH2Reply call) per ADR-0131 §Decision (vi):

```go
// Symmetric to connection.go H1 path; same primitive harvest.
if override, ok := chain.EncodeBodyOverride(); ok {
	resp.Body = override
}
```

- [ ] **Step 8: Run all tests; verify clean**

```bash
go vet ./...
golangci-lint run ./...
go test -race -count=1 ./internal/filter/http/...
go test -race -count=1 ./internal/filter/hcm/...
# expect: every test PASS
go test -race -count=1 -short ./...
# expect: every package PASS
```

- [ ] **Step 9: Verify ADR-0131 intact at HEAD**

```bash
grep -nE '^## ADR-0131' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task.*Task 4' docs/envoy-go/DECISIONS.md | head -3   # expect: ADR-0131 line
```

- [ ] **Step 10: Append Task 4 entry to PROGRESS.md**

- [ ] **Step 11: Commit**

```bash
git add internal/filter/http/callbacks.go internal/filter/http/chain.go internal/filter/http/chain_test.go internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: framework primitive EncoderFilterCallbacks.OverwriteBody — interface + chain.go impl + H1 + H2 HCM-side harvest [ADR-0131]

Lands the encode-side framework primitive per ADR-0131 §Decision (vi).
+1 LoC interface method on EncoderFilterCallbacks at callbacks.go;
~+8 LoC impl + per-stream encodeBodyOverride []byte field + sentinel +
EncodeBodyOverride() ([]byte, bool) accessor on *FilterChain at chain.go;
~+8 LoC post-RunEncodeData harvest + resp.Body substitution at
hcm/connection.go (H1 path); ~+8 LoC symmetric harvest at
hcm/h2dispatch.go (H2 path). Total ~25 LoC framework delta. Symmetric to
phase-13 ADR-0128 decode-side primitives (~34 LoC) in shape and
load-bearing-ness.

Lands FIRST among impl tasks per cold-start prompt Critical PLAN-time
obligation 2 + planner-time decision 14 — Tasks 5-7 (DecodeHeaders +
EncodeHeaders + EncodeData) consume this primitive. Mirrors phase-13
ADR-0128 timing-shift lesson: phase-14 SPEC §3 framework-survey
anticipated the primitive at SPEC time; ADR-0131 anchored Lands-in-task:
Task 4 at SPEC commit; PLAN concurs.

3 chain_test.go probe-filter integration tests PASS:
  TestEncoderCB_OverwriteBody_StoresBytes_AccessorReflects
  TestEncoderCB_NoOverwriteBody_AccessorReturnsFalse
  TestEncoderCB_OverwriteBody_PassthroughOnSubsequentInvocations

ADR-0131 already exists at SPEC commit 073cb88 in final form per ADR-0044
ADR-on-impl SPEC-time-anticipation; this task references via commit
message + verifies Lands-in-task: Task 4 anchor intact at HEAD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
grep -nE '^## ADR-0131' docs/envoy-go/DECISIONS.md   # expect: 1 match
go test -race -count=1 ./internal/filter/http/ -run TestEncoderCB   # expect: 3 tests PASS
go test -race -count=1 -short ./...   # expect: every package PASS
```

---

## Task 5: `DecodeHeaders` body — Accept-Encoding parse + per-route resolve + maybe-strip-AE + `DecodeData` / `DecodeTrailers` pass-through + `SetDecoderCallbacks` + `SetEncoderCallbacks` + `OnDestroy` skeletons

**Files:**
- Modify: `internal/filter/http/compressor/compressor.go` (replace stub `DecodeHeaders` body + add `SetDecoderCallbacks` + `SetEncoderCallbacks` + `OnDestroy`)
- Modify: `internal/filter/http/compressor/compressor_test.go` (append decode-side test sub-group within Group 5; or create Group 5a)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the decode-side filter surface per SPEC §6.4 + §6.5 + §1.1 amendment 4 + §11.8. The filter parses Accept-Encoding via `parseAcceptEncoding` (landed in Task 3); resolves per-route TPFC via `f.dcb.RequestRouteConfig().(*compiledPerRoute)`; sets `passthrough` flag on disabled-route; computes effective `remove_accept_encoding_header` (per-route override wins over listener-level); strips `Accept-Encoding` from request headers when effective rmAE=true. NO new ADR (ADR-0129 + ADR-0130 cover the decode-side surface; ADR-0125 amendment §(viii) covers the per-route effective config).

**Precondition:** Task 4 commit on HEAD; pristine tree; framework primitive available.
**Artifacts:** Updated compressor.go (DecodeHeaders body + DecodeData/DecodeTrailers pass-through + SetDecoderCallbacks + SetEncoderCallbacks + OnDestroy), decode-side unit tests, PROGRESS Task 5 entry.
**Acceptance:** `go test -race -count=1 -v ./internal/filter/http/compressor/` shows Groups 1-4 + new decode-side tests PASS.

- [ ] **Step 1: Append decode-side tests**

Append decode-side tests to `compressor_test.go`. Per SPEC §6.4 the decode-side surface is small (4 algorithm steps); ~6-8 unit tests cover:
- `TestDecodeHeaders_NoAE_StoresEmptyEncoding_ContinueNoAEStrip` — empty AE; classification "no_accept_header"; rmAE inactive; no AE header to strip.
- `TestDecodeHeaders_GzipAE_StoresGzip_Continue` — `Accept-Encoding: gzip`; classification "compressor_used"; rmAE=false default; AE NOT stripped.
- `TestDecodeHeaders_PerRouteDisabled_PassthroughTrue_NoAEStrip_Continue` — perRoute.disabled=true; passthrough=true; no AE strip even when listener-level rmAE=true (mirrors Envoy's wholly-inactive semantic per SPEC §5.5).
- `TestDecodeHeaders_ListenerLevelRmAE_True_StripsAE` — listener rmAE=true; `Accept-Encoding: gzip` in request; after DecodeHeaders, `headers.Get("Accept-Encoding") == ""`.
- `TestDecodeHeaders_PerRouteRmAEOverride_True_StripsAE_EvenWhenListenerFalse` — listener rmAE=false; perRoute.removeAcceptEncodingHeaderOverride=ptr(true); AE stripped.
- `TestDecodeHeaders_PerRouteRmAEOverride_False_DoesNotStrip_EvenWhenListenerTrue` — listener rmAE=true; perRoute.removeAcceptEncodingHeaderOverride=ptr(false); AE NOT stripped (per-route override wins).
- `TestDecodeData_PassThrough_DataContinue` — pass-through verification.
- `TestDecodeTrailers_PassThrough_TrailersContinue` — pass-through verification.

- [ ] **Step 2: Run tests; verify decode-side tests FAIL (DecodeHeaders body is stub)**

```bash
go test -race -count=1 -v ./internal/filter/http/compressor/ -run TestDecodeHeaders
# expect: most tests FAIL — DecodeHeaders skeleton does not parse AE / does not strip / does not set passthrough
```

- [ ] **Step 3: Implement `DecodeHeaders` body + helpers**

Replace the skeleton `DecodeHeaders` body in `compressor.go` per SPEC §6.4 verbatim:

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Parse Accept-Encoding via the q-value parser; cache result.
	f.acceptedEncoding, f.acceptHeaderClassification = parseAcceptEncoding(headers.Get("Accept-Encoding"))

	// Step 2: Resolve per-route TPFC; cache.
	if f.dcb != nil {
		if v, ok := f.dcb.RequestRouteConfig().(*compiledPerRoute); ok {
			f.perRoute = v
		}
	}

	// Step 3: Per-route disabled bypass — no AE strip, no encode-side either.
	if f.perRoute != nil && f.perRoute.disabled {
		f.passthrough = true
		return envoyhttp.Continue
	}

	// Step 4: Compute effective rmAE (per-route override wins over listener).
	effectiveRmAE := f.config.removeAcceptEncodingHeader
	if f.perRoute != nil && f.perRoute.removeAcceptEncodingHeaderOverride != nil {
		effectiveRmAE = *f.perRoute.removeAcceptEncodingHeaderOverride
	}

	// Step 5: Strip Accept-Encoding header from request if effective.
	if effectiveRmAE {
		headers.Del("Accept-Encoding")
	}

	return envoyhttp.Continue
}

func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

func (f *filter) DecodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }
func (f *filter) OnDestroy()                                              {}
```

- [ ] **Step 4: Run tests; verify decode-side tests PASS**

```bash
go vet ./internal/filter/http/compressor/...
golangci-lint run ./internal/filter/http/compressor/...
go test -race -count=1 -v ./internal/filter/http/compressor/
# expect: every Group + decode-side test PASS
```

- [ ] **Step 5: Append Task 5 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor DecodeHeaders body + Decode pass-through + callback wiring

Lands DecodeHeaders body per SPEC §6.4 + §1.1 amendment 4 + §11.8:
parses Accept-Encoding via parseAcceptEncoding (q-value parser landed
Task 3); resolves per-route TPFC via f.dcb.RequestRouteConfig() →
*compiledPerRoute; sets passthrough on disabled-route per ADR-0125
amendment §(viii); computes effective rmAE (per-route override wins);
strips Accept-Encoding from request headers when effective rmAE=true.
DecodeData + DecodeTrailers pass-through. SetDecoderCallbacks +
SetEncoderCallbacks store both callback references on SAME *filter
instance per planner-time decision 10. OnDestroy no-op (no per-stream
resources). Decode-side unit tests (8 cases) PASS.

NO new ADR (ADR-0129 §Decision (iv) + ADR-0125 amendment §(viii) cover).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
go test -race -count=1 ./internal/filter/http/compressor/   # expect: Groups + decode-side PASS
git log -1 --format=%H -- internal/filter/http/compressor/compressor.go   # expect: just-committed SHA
```

---

## Task 6: `EncodeHeaders` body — 11-bucket skip-decision sequence + Vary trichotomy + ETag mode-a strong-strip / weak-preserve + Content-Encoding set + Content-Length strip + helpers `appendVaryAcceptEncoding` + `maybeStripStrongEtag` + `computeSkipReason` + `effectiveConfig` + Group 5 + Group 7 tests

**Files:**
- Modify: `internal/filter/http/compressor/compressor.go` (replace stub `EncodeHeaders` body + add helpers)
- Modify: `internal/filter/http/compressor/compressor_test.go` (append Group 5 + Group 7 tests)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the encode-side header-mutation surface per SPEC §6.6 + §11.5 + §11.10 + §11.15 + §1.1 amendments 5-6. The filter runs the 11-bucket skip-decision sequence (bucket order matches Envoy's `compressor_filter.cc`); on AE-side-skip paths injects Vary; on server-side-skip paths does NOT inject Vary (per §11.15 trichotomy + §1.1 amendment 5); on compress path mutates response headers (Content-Encoding=gzip; appendVary; mode-a strong-strip / weak-preserve; Content-Length strip). NO new ADR (ADR-0129 + ADR-0132 cover via the filterStats stat-emission discipline; ADR-0125 amendment §(viii) covers the effective-config helper).

**Precondition:** Task 5 commit on HEAD; pristine tree.
**Artifacts:** Updated compressor.go (EncodeHeaders body + appendVaryAcceptEncoding + maybeStripStrongEtag + computeSkipReason + effectiveConfig helpers), Group 5 (~17 tests) + Group 7 (~9 tests) tests, PROGRESS Task 6 entry.
**Acceptance:** all unit-test groups PASS; race-test clean.

- [ ] **Step 1: Append Group 5 + Group 7 tests**

Append per the file-structure table row above (Group 5 = 17 EncodeHeaders skip-decision matrix tests; Group 7 = 9 header-mutation helper tests). Skeleton:

```go
// --- Group 5: EncodeHeaders skip-decision matrix ---

func TestEncodeHeaders_NoAcceptHeader_Skip_VarySet_NoAcceptHeaderCounter(t *testing.T) {
	f := freshFilterWithDefault(t)
	f.acceptHeaderClassification = "no_accept_header" // pre-set per DecodeHeaders semantics
	headers := http.Header{}
	headers.Set("content-type", "text/html")
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary set on AE-side-skip; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "" {
		t.Errorf("expected no Content-Encoding on skip; got %q", v)
	}
	// counter assertion via mock filterStats (implementer adapts).
}

// (additional Group 5 tests: 16 more covering identity / wildcard-uncompressed /
// br-overshadowed / not_valid / content-type-mismatch / already-encoded gzip
// + deflate + identity / cache-control no-transform / status-uncompressible /
// etag-disabled mode-b / etag-strong-strip mode-a / etag-weak-preserve mode-a /
// content-length-too-small-known / allow-path / passthrough-bypass.)

// --- Group 7: Header-mutation helpers (per §1.1 amendments 5-6) ---

func TestAppendVaryAcceptEncoding_NoExisting_SetsAccept(t *testing.T) {
	headers := http.Header{}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=\"Accept-Encoding\"; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_ExistingWildcard_AppendsCommaSpaceAccept(t *testing.T) {
	// §11.10 — even wildcard gets append (NOT short-circuited).
	headers := http.Header{"Vary": []string{"*"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "*, Accept-Encoding" {
		t.Errorf("expected Vary=\"*, Accept-Encoding\"; got %q", v)
	}
}

func TestMaybeStripStrongEtag_StrongEtag_Stripped(t *testing.T) {
	headers := http.Header{"ETag": []string{`"abc123"`}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("ETag"); v != "" {
		t.Errorf("expected strong ETag stripped; got %q", v)
	}
}

func TestMaybeStripStrongEtag_WeakEtag_Preserved(t *testing.T) {
	headers := http.Header{"ETag": []string{`W/"abc123"`}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("ETag"); v != `W/"abc123"` {
		t.Errorf("expected weak ETag preserved; got %q", v)
	}
}

// (additional Group 7 tests: 5 more covering existing-non-AE-vary append +
// case-insensitive token dedup + mixed-case dedup + no-etag no-op + malformed-
// etag-preserved-defensive.)
```

- [ ] **Step 2: Run tests; verify Group 5 + 7 FAIL (EncodeHeaders body is stub; helpers absent)**

- [ ] **Step 3: Implement helpers + EncodeHeaders body**

Add to `compressor.go` per SPEC §6.6 verbatim:

```go
func (f *filter) effectiveConfig() *compiledConfig {
	if f.perRoute == nil || f.perRoute.removeAcceptEncodingHeaderOverride == nil {
		return f.config
	}
	cloned := *f.config
	cloned.removeAcceptEncodingHeader = *f.perRoute.removeAcceptEncodingHeaderOverride
	return &cloned
}

func (f *filter) computeSkipReason(headers http.Header, effective *compiledConfig, endStream bool) string {
	// 11-bucket skip-decision sequence per SPEC §6.6 + §11.15 + Envoy
	// compressor_filter.cc bucket order:
	// (a) no_accept_header / identity / wildcard_uncompressed / not_valid (AE-side)
	// (b) uncompressible_status / already_encoded / etag_disabled / no_transform /
	//     content_type_mismatch / content_length_too_small_known (server-side)
	// (Implementer fleshes out the dispatch table here per §6.6.)
	return ""
}

func appendVaryAcceptEncoding(headers http.Header) {
	existing := headers.Get("Vary")
	if existing == "" {
		headers.Set("Vary", "Accept-Encoding")
		return
	}
	for _, tok := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "Accept-Encoding") {
			return
		}
	}
	headers.Set("Vary", existing+", Accept-Encoding")
}

func maybeStripStrongEtag(headers http.Header) {
	val := headers.Get("ETag")
	if val == "" {
		return
	}
	if strongEtagPattern.MatchString(val) {
		headers.Del("ETag")
	}
	// weak match → preserve (no-op); malformed → preserve (no-op defensive).
}

func (f *filter) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Per-route disabled bypass — no header mutation, no counter.
	if f.passthrough {
		return envoyhttp.Continue
	}
	effective := f.effectiveConfig()
	skipReason := f.computeSkipReason(headers, effective, endStream)

	// Step 2: header_* counter dispatch per §11.5 6-counter cluster.
	switch f.acceptHeaderClassification {
	case "no_accept_header":  f.stats.NoAcceptHeader.Inc()
	case "wildcard":          f.stats.HeaderWildcard.Inc()
	case "identity":          f.stats.HeaderIdentity.Inc()
	case "not_valid":         f.stats.HeaderNotValid.Inc()
	case "compressor_used":   f.stats.HeaderCompressorUsed.Inc()
	case "overshadowed":      f.stats.HeaderCompressorOvershadowed.Inc()
	}

	if skipReason != "" {
		f.stats.ResponseNotCompressed.Inc()
		switch skipReason {
		case "etag_disabled":
			f.stats.NotCompressedEtag.Inc()
		case "content_length_too_small_known":
			f.stats.ResponseContentLengthTooSmall.Inc()
		}
		// §11.15 trichotomy: AE-side-skip paths inject Vary; server-side-skip do NOT.
		if skipReason == "no_accept_header" || skipReason == "identity" ||
			skipReason == "wildcard_uncompressed" || skipReason == "not_valid" {
			appendVaryAcceptEncoding(headers)
		}
		return envoyhttp.Continue
	}

	// Step 3: Compress path — mutate headers.
	headers.Set("Content-Encoding", "gzip")
	appendVaryAcceptEncoding(headers)
	if !effective.disableOnEtagHeader {
		maybeStripStrongEtag(headers)
	}
	headers.Del("Content-Length")
	f.willCompress = true
	return envoyhttp.Continue
}
```

NOTE: implementer fleshes out `computeSkipReason` per the SPEC §6.6 + §11.15 11-bucket order verbatim. The `import "strings"` is added if not already present.

- [ ] **Step 4: Run tests; verify Groups 1-7 PASS**

- [ ] **Step 5: Append Task 6 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor EncodeHeaders body — 11-bucket skip-decision + Vary trichotomy + ETag mode-a strong-strip + Content-Encoding set + Content-Length strip

Lands the encode-side header-mutation surface per SPEC §6.6 + §11.5 +
§11.10 + §11.15 + §1.1 amendments 5-6. 11-bucket skip-decision sequence
per Envoy compressor_filter.cc bucket order; Vary trichotomy per §11.15
(AE-side-skip injects Vary; server-side-skip does NOT inject); ETag
mode-a strong-strip via ^"[^"]*"$ regex / weak-preserve via ^W/"[^"]*"$
regex per §11.7 + §1.1 amendment 6; Vary append always per §11.10 (even
on existing Vary: *); Content-Length strip (writeH1Reply rewrites at
wire time). Group 5 EncodeHeaders skip-decision matrix (17 tests) +
Group 7 header-mutation helpers (9 tests) PASS.

NO new ADR (ADR-0129 + ADR-0132 + ADR-0125 amendment §(viii) cover).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `EncodeData` body — gzip-encode + `OverwriteBody` invocation + counter increments + `EncodeTrailers` pass-through + Group 6 tests

**Files:**
- Modify: `internal/filter/http/compressor/compressor.go` (replace stub `EncodeData` body + `EncodeTrailers`)
- Modify: `internal/filter/http/compressor/compressor_test.go` (append Group 6 tests)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the encode-side body-mutation surface per SPEC §6.7 + §11.9 + §11.14 + ADR-0131 §Decision (i)-(ii). Path B body algorithm: gzip-encode in one shot via `gzip.NewWriterLevel(buf, level).Write(data).Close()` and emit via the framework primitive `f.ecb.OverwriteBody(buf.Bytes())` (landed at Task 4). Late min_content_length gate per planner-time decision 4 (D4 settlement) increments BOTH `response_content_length_too_small` AND `response_not_compressed` on the below-threshold late-revert anomaly path. NO new ADR (ADR-0131 §Decision (i) covers).

**Precondition:** Task 6 commit on HEAD; framework primitive available (Task 4); pristine tree.
**Artifacts:** Updated compressor.go (EncodeData body + EncodeTrailers pass-through), Group 6 tests (~5 cases per the file-structure table), PROGRESS Task 7 entry.
**Acceptance:** all unit-test groups PASS; race-test clean; the OverwriteBody primitive call is asserted in Group 6 via mock encoder callback that records the override bytes.

- [ ] **Step 1: Append Group 6 tests**

Append to `compressor_test.go` per the file-structure table row (5 tests):
- `TestEncodeData_PassthroughOrWillCompressFalse_DataContinue_NoOverwrite`
- `TestEncodeData_NotEndStream_DataContinue_NoOverwrite_Defensive`
- `TestEncodeData_LateMinContentLength_RevertSkip_DataContinue_CountersIncremented` (per D4 settlement: BOTH counters increment)
- `TestEncodeData_AllowPath_GzipEncodes_OverwriteBodyCalled_CountersIncremented` (parametrized over small/medium/large bodies; verifies `len(compressed) > 0` AND `compress/gzip.NewReader(compressed)` decompresses to original AND counters incremented correctly)
- `TestEncodeData_HuffmanOnlyStrategy_DifferentBytesObservable` (compares HUFFMAN_ONLY output vs default-strategy output for same input; bytes differ).

- [ ] **Step 2: Run tests; verify Group 6 FAILS**

- [ ] **Step 3: Implement `EncodeData` body**

Replace the stub `EncodeData` body in `compressor.go` per SPEC §6.7 verbatim:

```go
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	if f.passthrough || !f.willCompress {
		return envoyhttp.DataContinue
	}
	// Defensive: future framework changes might invoke mid-stream. Pass through
	// for now; do NOT compress until end-stream observed. Per ADR-0131 §Decision (vii).
	if !endStream {
		return envoyhttp.DataContinue
	}
	// Late min_content_length gate per planner-time decision 4 (D4 settlement)
	// — BOTH counters increment on below-threshold late-revert.
	effective := f.effectiveConfig()
	if uint32(len(data)) < effective.minContentLength {
		f.stats.ResponseContentLengthTooSmall.Inc()
		f.stats.ResponseNotCompressed.Inc()
		return envoyhttp.DataContinue
	}
	// Compress in one shot.
	var buf bytes.Buffer
	gzWriter, err := gzip.NewWriterLevel(&buf, f.config.gzip.level)
	if err != nil {
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
	// Emit via the framework primitive landed at Task 4 (ADR-0131 §Decision (vi)).
	f.ecb.OverwriteBody(compressed)
	// Increment counters.
	f.stats.ResponseCompressed.Inc()
	f.stats.ResponseTotalUncompressedBytes.Add(uint64(len(data)))
	f.stats.ResponseTotalCompressedBytes.Add(uint64(len(compressed)))
	return envoyhttp.DataContinue
}

func (f *filter) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
```

- [ ] **Step 4: Run tests; verify Groups 1-7 PASS**

- [ ] **Step 5: Append Task 7 entry to PROGRESS.md**

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor EncodeData body — gzip-encode + OverwriteBody + counter increments + late-MCL anomaly

Lands the encode-side body-mutation surface per SPEC §6.7 + §11.9 + §11.14
+ ADR-0131 §Decision (i)-(ii). Path B body algorithm: gzip-encode in one
shot via gzip.NewWriterLevel(buf, level).Write(data).Close(); emit via
f.ecb.OverwriteBody(buf.Bytes()) (the framework primitive landed Task 4).
Late min_content_length gate per planner-time decision 4 (D4 settlement)
increments BOTH response_content_length_too_small AND
response_not_compressed on the below-threshold late-revert anomaly path
(structurally rare per ADR-0131 §Decision (vii); fixture 0016 sidesteps
via direct_response routes carrying CL on action headers per
planner-time decision 3 (D3 settlement)). EncodeTrailers pass-through.
Group 6 tests (5 cases) PASS including OverwriteBody primitive
invocation assertion + decompressed-byte equivalence check + HUFFMAN_ONLY
strategy observable.

NO new ADR (ADR-0131 §Decision (i)+(vi)+(vii) cover).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `filterStats` 17-counter wiring + `newFilterStats` registration helper + namespace shape `compressor.<library>.gzip.[response.]<counter>` + Group 8 stats integration tests [ADR-0132]

**Files:**
- Modify: `internal/filter/http/compressor/compressor.go` (flesh out `filterStats` struct per §6.9 + `newFilterStats` registration per ADR-0132 §Decision (i))
- Modify: `internal/filter/http/compressor/compressor_test.go` (append Group 8 stats integration tests)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the 17-counter `filterStats` struct + `newFilterStats` registration helper per SPEC §6.9 + ADR-0132 §Decision (i)+(ii)+(v). The 17 counters split: 6 header_* + 1 not_compressed_etag (no direction infix) + 5 response_* + 5 request_* (always-zero in MVP). Stat path: `compressor.<libraryName>.gzip.[response|request].<counter>` flattened via existing Rule SN2 (NO new SN10 per §1.1 amendment 3). Group 8 stats integration tests verify the namespace shape end-to-end including the library-name-empty consecutive-dots edge case per planner-time decision 5 (D5 settlement). ADR-0132 reference at this commit per the per-ADR Lands-in-task field at SPEC commit. The BEHAVIOR_CONTRACT.md 29→46 stat-table extension lands at Task 15 per ADR-0052 in-place edit authorisation.

**Precondition:** Task 7 commit on HEAD; pristine tree.
**Artifacts:** Updated compressor.go (filterStats struct fleshed out per §6.9 + newFilterStats registration), Group 8 tests (~5 cases per the file-structure table), PROGRESS Task 8 entry.
**Acceptance:** all 8 test groups PASS; race-test clean; `grep -nE '^## ADR-0132' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0132 `Lands-in-task: Task 8` field verified intact.

- [ ] **Step 1: Append Group 8 tests**

Per the file-structure table row (5 tests):
- `TestStatsNamespace_LibraryNameSet_StatPathCorrect` — verifies `compressor.text_optimized.gzip.response.compressed` → SN2 flatten produces `envoy_http_compressor_text_optimized_gzip_response_compressed{envoy_http_conn_manager_prefix="ingress_p14"}`.
- `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` — per planner-time decision 5 (D5 settlement); verifies empty `name:` produces consecutive dots → `envoy_http_compressor__gzip_<counter>` double-underscore Prometheus form.
- `TestStatsNamespace_AllSeventeenCountersRegistered` — verifies all 17 counter names registered with the *stats.Registry; iterates over expected names + asserts each is non-nil.
- `TestStatsNamespace_ResponseInfixPresent_WhenResponseDirectionConfigSet` — phase-14 always sets response_direction_config (per planner-time + ADR-0132 §Decision (ii)); 5 response_* counters carry the `response.` infix; 5 request_* counters carry the `request.` infix; 6 header_* + 1 not_compressed_etag carry NO direction infix.
- `TestStatsNamespace_RequestCountersRegisteredAtZero` — 5 always-zero request_* per ADR-0132 §Decision (vii) twin-series-discipline: counters registered + observable as zero on both sides.

- [ ] **Step 2: Run tests; verify Group 8 FAILS**

- [ ] **Step 3: Implement `filterStats` struct + `newFilterStats` helper per §6.9 + ADR-0132 §Decision (i)+(v)**

Replace the skeleton `filterStats` struct + `newFilterStats` helper in `compressor.go` per SPEC §6.9 verbatim:

```go
// filterStats is the 17-counter set per SPEC §6.9 + ADR-0132 §Decision (i).
// Lock-free *stats.Counter per ADR-0061; allocated once at New factory time
// per HCM stat_prefix.
type filterStats struct {
	// 6 Accept-Encoding cluster counters (NOT split per direction).
	HeaderCompressorOvershadowed *stats.Counter
	HeaderCompressorUsed         *stats.Counter
	HeaderIdentity               *stats.Counter
	HeaderNotValid               *stats.Counter
	HeaderWildcard               *stats.Counter
	NoAcceptHeader               *stats.Counter
	// 1 ETag-skip counter.
	NotCompressedEtag *stats.Counter
	// 5 response-side counters (active in MVP).
	ResponseCompressed             *stats.Counter
	ResponseContentLengthTooSmall  *stats.Counter
	ResponseNotCompressed          *stats.Counter
	ResponseTotalCompressedBytes   *stats.Counter
	ResponseTotalUncompressedBytes *stats.Counter
	// 5 request-side counters (always-zero in MVP since request_direction_config silent-ignored;
	// registered for byte-equivalent stat scrape with reference Envoy per ADR-0132 §Decision (vii)).
	RequestCompressed              *stats.Counter
	RequestContentLengthTooSmall   *stats.Counter
	RequestNotCompressed           *stats.Counter
	RequestTotalCompressedBytes    *stats.Counter
	RequestTotalUncompressedBytes  *stats.Counter
}

// newFilterStats registers 17 counters at the namespace path
//   compressor.<libraryName>.gzip.[response|request].<counter>
// per ADR-0132 §Decision (i)+(v). Empty libraryName produces consecutive
// dots (compressor..gzip.<counter>) per planner-time decision 5; SN2 flatten
// produces double-underscore Prometheus form (envoy_http_compressor__gzip_<counter>).
//
// Returns a fully-populated *filterStats. Nil-tolerant: returns nil if
// reg == nil (per ADR-0085 nil-tolerance pattern; test code paths that don't
// exercise stat-bearing filters).
func newFilterStats(reg *stats.Registry, hcmStatPrefix string, libraryName string) *filterStats {
	if reg == nil {
		return nil
	}
	prefix := fmt.Sprintf("http.%s.compressor.%s.gzip.", hcmStatPrefix, libraryName)
	return &filterStats{
		HeaderCompressorOvershadowed:   reg.Counter(prefix + "header_compressor_overshadowed"),
		HeaderCompressorUsed:           reg.Counter(prefix + "header_compressor_used"),
		HeaderIdentity:                 reg.Counter(prefix + "header_identity"),
		HeaderNotValid:                 reg.Counter(prefix + "header_not_valid"),
		HeaderWildcard:                 reg.Counter(prefix + "header_wildcard"),
		NoAcceptHeader:                 reg.Counter(prefix + "no_accept_header"),
		NotCompressedEtag:              reg.Counter(prefix + "not_compressed_etag"),
		ResponseCompressed:             reg.Counter(prefix + "response.compressed"),
		ResponseContentLengthTooSmall:  reg.Counter(prefix + "response.content_length_too_small"),
		ResponseNotCompressed:          reg.Counter(prefix + "response.not_compressed"),
		ResponseTotalCompressedBytes:   reg.Counter(prefix + "response.total_compressed_bytes"),
		ResponseTotalUncompressedBytes: reg.Counter(prefix + "response.total_uncompressed_bytes"),
		RequestCompressed:              reg.Counter(prefix + "request.compressed"),
		RequestContentLengthTooSmall:   reg.Counter(prefix + "request.content_length_too_small"),
		RequestNotCompressed:           reg.Counter(prefix + "request.not_compressed"),
		RequestTotalCompressedBytes:    reg.Counter(prefix + "request.total_compressed_bytes"),
		RequestTotalUncompressedBytes:  reg.Counter(prefix + "request.total_uncompressed_bytes"),
	}
}
```

NOTE: the `reg.Counter(name)` accessor is presumed; implementer adapts to the actual `*stats.Registry` API per the existing `internal/stats/` package surface.

- [ ] **Step 4: Run tests; verify Groups 1-8 PASS**

- [ ] **Step 5: Verify ADR-0132 intact at HEAD**

```bash
grep -nE '^## ADR-0132' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task.*Task 8' docs/envoy-go/DECISIONS.md | grep 0132   # expect: 1 match
```

- [ ] **Step 6: Append Task 8 entry to PROGRESS.md**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: compressor 17-counter filterStats wiring + namespace shape compressor.<library>.gzip.[response.]<counter> [ADR-0132]

Lands the 17-counter filterStats struct + newFilterStats registration helper
per SPEC §6.9 + ADR-0132 §Decision (i)+(ii)+(v). 17 counters split: 6
header_* + 1 not_compressed_etag (no direction infix) + 5 response_* +
5 request_* (always-zero in MVP per ADR-0132 §Decision (vii) twin-series
discipline). Stat path: compressor.<libraryName>.gzip.[response|request].
<counter> flattened via existing Rule SN2 (NO new SN10 per §1.1 amendment
3). Empty libraryName produces consecutive dots per planner-time decision
5 (D5 settlement); SN2 flatten produces envoy_http_compressor__gzip_<counter>
double-underscore Prometheus form. Group 8 stats integration tests (5
cases) PASS verifying namespace shape end-to-end + library-name-empty
edge case + all 17 counters registered + response/request infix correctness.

The BEHAVIOR_CONTRACT.md 29→46 stat-table extension lands at Task 15 per
ADR-0052 in-place edit authorisation; this task lands only the Go-side
filterStats wiring.

ADR-0132 already exists at SPEC commit 073cb88 in final form per ADR-0044
ADR-on-impl SPEC-time-anticipation; this task references via commit
message + verifies Lands-in-task: Task 8 anchor intact at HEAD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `FuzzCompressorConfigParse` fuzzer — 18th fuzzer in repo

**Files:**
- Create: `internal/filter/http/compressor/fuzz_test.go`
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the 18th fuzzer per ADR-0018 + SPEC §14.3 — `FuzzCompressorConfigParse` fuzzes the `New` factory's typed_config Any-unmarshal pipeline. Inputs: random bytes interpreted as Compressor proto bytes; the fuzzer asserts no panic + no nil-deref + no `(nil, nil)` return. NO new ADR.

**Precondition:** Task 8 commit on HEAD; pristine tree.
**Artifacts:** fuzz_test.go (~80 LoC), PROGRESS Task 9 entry.
**Acceptance:** `go test -fuzz=FuzzCompressorConfigParse -fuzztime=30s ./internal/filter/http/compressor/` runs clean for 30s; `go test -count=1 ./internal/filter/http/compressor/` shows the seed-corpus tests PASS.

- [ ] **Step 1: Author `fuzz_test.go`**

```go
package compressor

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

func FuzzCompressorConfigParse(f *testing.F) {
	// 8 valid-config seeds + 4 invalid-config seeds per the file-structure table row.
	seeds := []*compressorv3.Compressor{
		// (a) default-everything (gzip library, no response_direction_config)
		{CompressorLibrary: gzipLibrary(t)},
		// (b) explicit min_content_length=30
		{
			CompressorLibrary: gzipLibrary(t),
			ResponseDirectionConfig: &compressorv3.ResponseDirectionConfig{
				CommonConfig: &compressorv3.CommonDirectionConfig{
					MinContentLength: wrapperspb.UInt32(30),
				},
			},
		},
		// (c) explicit content_type=[text/html]
		// (d) uncompressible_response_codes=[404]
		// (e) disable_on_etag_header=true
		// (f) remove_accept_encoding_header=true
		// (g) gzip compression_level=BEST_SPEED
		// (h) gzip compression_strategy=HUFFMAN_ONLY
		// (implementer fleshes out the 8 valid + 4 invalid seeds per the
		// file-structure table row.)
	}
	for _, s := range seeds {
		any, err := anypb.New(s)
		if err != nil {
			f.Fatalf("anypb.New: %v", err)
		}
		f.Add(any.Value) // raw bytes from anypb wire-form
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		any := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(any, envoyhttp.FactoryCtx{})
		if err != nil {
			// invalid input → error returned; never panic.
			return
		}
		if factory == nil {
			t.Fatal("expected non-nil factory on err==nil")
		}
		// successful parse → factory invocation must not panic.
		_ = factory()
	})
}

// gzipLibrary returns a TypedExtensionConfig for a default gzip library.
// Helper used by the seed corpus.
func gzipLibrary(...) *corev3.TypedExtensionConfig {
	g := &gzipv3.Gzip{}
	any, _ := anypb.New(g)
	return &corev3.TypedExtensionConfig{
		Name:        "test_lib",
		TypedConfig: any,
	}
}
```

NOTE: implementer adapts the helper signature + fleshes out the 4 invalid-config seeds (nil typed_config; missing compressor_library; non-Gzip TypeURL; uncompressible_response_codes=[100]).

- [ ] **Step 2: Run the fuzzer for 30s; verify clean**

```bash
go test -fuzz=FuzzCompressorConfigParse -fuzztime=30s ./internal/filter/http/compressor/
# expect: clean exit; "fuzz: elapsed 30s, execs ..." with no failure messages
```

- [ ] **Step 3: Run seed-corpus tests in normal test mode**

```bash
go test -count=1 -run FuzzCompressorConfigParse ./internal/filter/http/compressor/
# expect: PASS — seed corpus runs as regression tests
```

- [ ] **Step 4: Append Task 9 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/compressor/fuzz_test.go docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: FuzzCompressorConfigParse fuzzer — 18th fuzzer in repo

Adds the 18th fuzzer per ADR-0018 + SPEC §14.3. Fuzzes the New factory's
typed_config Any-unmarshal pipeline: outer Compressor proto +
nested Gzip Any proto + per-route oneof. Seed corpus: 8 valid-config
seeds + 4 invalid-config seeds per the PLAN.md file-structure table row.
Asserts: never panic; never (nil, nil); on success the factory()
invocation also does not panic. Cleanly exits at 30s budget.

NO new ADR (single-file fuzzer follows existing FuzzFooConfigParse
pattern from cors / fault / header_mutation / localratelimit / csrf /
buffer phases).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `cmd/envoy-go/main.go` register `compressor.New` under `compressor.TypeURL` + fixture infrastructure (`BackendKind=HTTPCompressor` enum + runner spawn helper) + NEW shared `test/helpers/echobackend/` helper [ADR-0133 anchor begins here for the decompress helper landing at Task 11]

**Files:**
- Modify: `cmd/envoy-go/main.go` (+1 import + 1 register line)
- Create: `test/helpers/echobackend/doc.go`
- Create: `test/helpers/echobackend/echobackend.go`
- Create: `test/helpers/echobackend/echobackend_test.go`
- Create: `test/helpers/echobackend/cmd/echobackend/main.go` (cmdline wrapper)
- Modify: `test/differential/fixture/fixture.go` (+15 LoC for `HTTPCompressor BackendKind = 13`)
- Modify: `test/differential/runner_test.go` (+25 LoC for blank-import + spawn helper + switch case)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the boot-time compressor registration + the fixture infrastructure (BackendKind enum + runner spawn helper) + the NEW shared `test/helpers/echobackend/` helper per planner-time decision 6 + planner-time decision 12 (D7 settlement). The echobackend helper is shared infrastructure for future fixtures that need echo-backend behavior; phase 14 fixture 0016 scenario 6 uses it for the per-route rmAE assertion. NO new ADR.

**Precondition:** Task 9 commit on HEAD; pristine tree.
**Artifacts:** Updated main.go + new echobackend helper + updated fixture.go + runner_test.go, PROGRESS Task 10 entry.
**Acceptance:** `go build ./...` clean; `go test -race -count=1 -short ./...` clean; the new `test/helpers/echobackend/` package builds and tests pass; `grep -nE 'HTTPCompressor BackendKind = 13' test/differential/fixture/fixture.go` returns 1 match.

- [ ] **Step 1: Edit `cmd/envoy-go/main.go` to register `compressor.New`**

Add the import (alphabetical-after-buffer per ADR-0129 §Decision (v)) at line ~28-35:

```go
import (
	// ... existing imports ...
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"   // NEW (alphabetical-after-buffer)
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	// ... rest unchanged ...
)
```

Add the registration line at line ~117-128 (alphabetical-after-buffer; between buffer and cors):

```go
httpReg.Register(router.TypeURL, router.New)
httpReg.Register(buffer.TypeURL, buffer.New)
httpReg.Register(compressor.TypeURL, compressor.New)   // NEW
httpReg.Register(cors.TypeURL, cors.New)
// ... rest unchanged ...
httpReg.Freeze()
```

- [ ] **Step 2: Create `test/helpers/echobackend/doc.go`**

```go
// Package echobackend implements a minimal HTTP/1.1 echo backend that echoes
// inbound request method + URL.Path + Header (lowercase canonical form per
// ADR-0072 + phase-04 lowercase-header pattern) as a JSON body in its response.
//
// Used by differential fixtures whose driver needs to assert on upstream-side
// request shape (e.g., per-route remove_accept_encoding_header verification —
// phase 14 fixture 0016 scenario 6).
//
// Introduced by phase 14 per planner-time decision 6 + planner-time decision
// 12 (D7 settlement). Future filter fixtures needing echo-backend behavior MAY
// use this shared helper. Phase 13 buffer's per-fixture backend at
// test/fixtures/0015-http-buffer/backends/backend.go MAY be migrated to use
// this helper in a future cleanup (out of scope for phase 14).
package echobackend
```

- [ ] **Step 3: Create `test/helpers/echobackend/echobackend.go`**

```go
package echobackend

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// New returns a configured *http.Server that, for any inbound request, writes
// a JSON body containing {"method": "...", "path": "...", "headers": {"k": "v"}}.
// Headers are echoed with lowercased canonical keys per Envoy wire-form
// discipline. Status 200; Content-Type: application/json.
//
// The caller binds via srv.Serve(listener); the listener allocation is the
// caller's responsibility (the runner allocates the port + binds upfront).
func New() *http.Server {
	return &http.Server{
		Handler: http.HandlerFunc(handle),
	}
}

func handle(w http.ResponseWriter, req *http.Request) {
	body := buildEcho(req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// echoRecord is the JSON shape echoed in the response body.
type echoRecord struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

func buildEcho(req *http.Request) []byte {
	rec := echoRecord{
		Method:  req.Method,
		Path:    req.URL.Path,
		Headers: map[string]string{},
	}
	for k, vs := range req.Header {
		// Lowercase canonical key per ADR-0072 + phase-04 lowercase-header pattern.
		// Multi-valued headers are comma-joined per RFC 7230 §3.2.2.
		rec.Headers[strings.ToLower(k)] = strings.Join(vs, ", ")
	}
	// Always include Host (req.Host is the Host header per net/http convention).
	if req.Host != "" {
		rec.Headers["host"] = req.Host
	}
	bytes, _ := json.Marshal(rec)
	return bytes
}

// Listen returns a TCP listener bound to the requested port. Helper for the
// cmdline wrapper at cmd/echobackend/main.go.
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("0.0.0.0", itoa(port)))
}

func itoa(n int) string {
	// Avoid pulling strconv into this file's imports; tiny inline implementation.
	if n == 0 { return "0" }
	digits := []byte{}
	for n > 0 { digits = append([]byte{byte('0' + n%10)}, digits...); n /= 10 }
	return string(digits)
}
```

- [ ] **Step 4: Create `test/helpers/echobackend/echobackend_test.go`**

Per the file-structure table row (~80 LoC; covers header echo correctness; method echo correctness; body Content-Type; lowercased keys; multi-value headers; large + empty header set tolerance).

- [ ] **Step 5: Create `test/helpers/echobackend/cmd/echobackend/main.go`**

```go
package main

import (
	"flag"
	"log"

	"github.com/esalaine/envoy-go/test/helpers/echobackend"
)

func main() {
	port := flag.Int("port", 0, "TCP port to bind")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port is required")
	}
	listener, err := echobackend.Listen(*port)
	if err != nil { log.Fatalf("listen: %v", err) }
	srv := echobackend.New()
	log.Printf("echobackend listening on %d", *port)
	if err := srv.Serve(listener); err != nil { log.Fatalf("serve: %v", err) }
}
```

- [ ] **Step 6: Edit `test/differential/fixture/fixture.go` to add `HTTPCompressor BackendKind = 13`**

Per planner-time decision 13 — add after the existing `HTTPBuffer BackendKind = 12` enum value with the doc-comment per the file-structure table row.

- [ ] **Step 7: Edit `test/differential/runner_test.go` to add blank-import + spawn helper + switch case**

```go
import (
	// ... existing blank-imports ...
	_ "github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs"   // NEW
	// ...
)

// New spawn helper:
func startEchoBackend(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/helpers/echobackend/cmd/echobackend", "--port", fmt.Sprintf("%d", port))
	cmd.Dir = repoRoot
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd, cmd.Start()
}

// In runFixture's `kind` switch, add:
case fixture.HTTPCompressor:
    backendCmd, err = startEchoBackend(ctx, repoRoot, backendPort)
```

- [ ] **Step 8: Run all builds + tests; verify clean**

```bash
go build ./...
go vet ./...
golangci-lint run ./...
go test -race -count=1 -short ./...
go test -race -count=1 ./test/helpers/echobackend/
# expect: every package PASS; echobackend tests green
```

- [ ] **Step 9: Append Task 10 entry to PROGRESS.md**

- [ ] **Step 10: Commit**

```bash
git add cmd/envoy-go/main.go test/helpers/echobackend/ test/differential/fixture/fixture.go test/differential/runner_test.go docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: register compressor + new shared echobackend helper + fixture infra

cmd/envoy-go/main.go: alphabetical-after-buffer registration of
compressor.New under compressor.TypeURL per ADR-0129 §Decision (v).
NEW shared test/helpers/echobackend/ package per planner-time decision 6
+ 12 (D7 settlement) — echo-backend that echoes inbound method + path +
headers as JSON; future fixtures may use; phase-13 buffer's per-fixture
backend may be migrated in a future cleanup (out of scope for phase 14).
test/differential/fixture/fixture.go: HTTPCompressor BackendKind = 13
per planner-time decision 13. test/differential/runner_test.go:
blank-import + startEchoBackend spawn helper + switch case.

NO new ADR (the decompress-and-compare body-assertion discipline ADR-0133
anchors at Task 11 fixture-driver per per-ADR Lands-in-task field).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Fixture 0016 — `inputs/driver.go` (single-listener 6-scenario sequential orchestration; decompress-and-compare body assertion via `compress/gzip.NewReader`; `assertBodyEquivalent` + `decompressGzip` helpers per ADR-0133 §Decision (i)+(ii)) [ADR-0133]

**Files:**
- Create: `test/fixtures/0016-http-compressor/inputs/driver.go`
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the fixture driver per SPEC §7.4 + ADR-0133 §Decision (i)-(iii). The driver implements the SINGLE-listener fixture interface (`fixture.Driver` per planner-time decision 11; mirrors phase-13 buffer + phase-12 csrf), drives 6 scenarios sequentially, dispatches body-assertion mode based on response `Content-Encoding` header per ADR-0133 §Decision (i)-(ii), and asserts per-counter deltas with `response_total_compressed_bytes` boundary-only tolerance per planner-time decision 2 (D2 settlement). ADR-0133 reference at this commit per the per-ADR Lands-in-task field at SPEC commit. Note the driver uses the YAML files at `envoy.yaml` + `envoy-go.yaml` which DO NOT YET EXIST at this task — Task 12 lands them. Therefore this task lands ONLY the driver + helper functions; the YAML rendering will template-substitute placeholder names that Task 12 fills in. The driver tests via `runFixture` will FAIL until Task 12 lands the YAMLs; this is expected per the task ordering (the driver framework + helpers come first; the configs come second).

Alternative: PLAN MAY swap the order (Task 11 = YAMLs first; Task 12 = driver second) but the cold-start prompt's task-anchor schedule places ADR-0133 at Task 11 (driver + decompress helpers). PLAN preserves the per-ADR Lands-in-task field by keeping ADR-0133 at the driver task.

**Precondition:** Task 10 commit on HEAD; pristine tree.
**Artifacts:** driver.go (~220 LoC per the file-structure table row), PROGRESS Task 11 entry.
**Acceptance:** `go build ./test/fixtures/0016-http-compressor/inputs/` clean; `go vet` + `golangci-lint` clean; `grep -nE '^## ADR-0133' docs/envoy-go/DECISIONS.md` returns 1 match; ADR-0133 `Lands-in-task: Task 11` field verified intact. The driver itself does not yet drive a real fixture (YAMLs land Task 12); the function bodies must compile and unit-test in isolation where possible.

- [ ] **Step 1: Author `inputs/driver.go`**

Per the file-structure table row + SPEC §7.4 + ADR-0133 §Decision (ii) verbatim. Skeleton (~220 LoC):

```go
package driver

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

//go:embed envoy.yaml
var envoyYAML string

//go:embed envoy-go.yaml
var envoyGoYAML string

func init() {
	fixture.RegisterFixture("0016-http-compressor", &compressorDriver{})
}

type compressorDriver struct{}

func (d *compressorDriver) BackendCount() int             { return 1 }
func (d *compressorDriver) BackendKind() fixture.BackendKind { return fixture.HTTPCompressor }

func (d *compressorDriver) ReferenceBootstrap(rt fixture.Runtime) (string, error) {
	return rt.Render(envoyYAML), nil
}
func (d *compressorDriver) SubjectConfig(rt fixture.Runtime) (string, error) {
	return rt.Render(envoyGoYAML), nil
}

// 6 scenarios per SPEC §7.1.
type scenario struct {
	name              string
	method, path      string
	acceptEncoding    string
	expectedStatus    int
	expectedCE        string // "gzip" or ""
	expectedVary      string // "Accept-Encoding" or ""
	originalPayload   []byte // for decompress-and-compare assertion
	assertEtagAbsent  bool   // scenario 4
	assertNoAEInBody  bool   // scenario 6 — backend echoes headers; assert no Accept-Encoding
}

var scenarios = []scenario{
	{name: "1-allow-compress", method: "GET", path: "/text-html-1024", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", originalPayload: bytes.Repeat([]byte("A"), 1024)},
	{name: "2-content-type-skip", method: "GET", path: "/image-png-1024", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{name: "3-below-min", method: "GET", path: "/text-html-10", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{name: "4-etag-strong-strip", method: "GET", path: "/text-html-etag-strong", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", originalPayload: bytes.Repeat([]byte("B"), 1024), assertEtagAbsent: true},
	{name: "5-per-route-disabled", method: "GET", path: "/per-route-disabled", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "", expectedVary: ""},
	{name: "6-per-route-rmae", method: "GET", path: "/per-route-rmae", acceptEncoding: "gzip", expectedStatus: 200, expectedCE: "gzip", expectedVary: "Accept-Encoding", assertNoAEInBody: true},
}

func (d *compressorDriver) DriveReference(ctx context.Context, rt fixture.Runtime) (fixture.Result, error) {
	return d.run(ctx, rt.ListenerURL("l_main"), rt)
}
func (d *compressorDriver) DriveSubject(ctx context.Context, rt fixture.Runtime) (fixture.Result, error) {
	return d.run(ctx, rt.ListenerURL("l_main"), rt)
}

func (d *compressorDriver) run(ctx context.Context, baseURL string, rt fixture.Runtime) (fixture.Result, error) {
	client := &http.Client{}
	results := fixture.Result{}
	for _, s := range scenarios {
		req, _ := http.NewRequestWithContext(ctx, s.method, baseURL+s.path, nil)
		req.Header.Set("Accept-Encoding", s.acceptEncoding)
		resp, err := client.Do(req)
		if err != nil { return results, fmt.Errorf("%s: %w", s.name, err) }
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil { return results, err }
		results.AddScenario(s.name, resp.StatusCode, resp.Header, body)
	}
	results.AddStatsScrape(rt.PrometheusScrape())
	return results, nil
}

// assertBodyEquivalent dispatches on Content-Encoding per ADR-0133 §Decision (ii).
// On Content-Encoding: gzip → decompress both sides via compress/gzip.NewReader
// and assert byte-exact on plaintexts. On empty/absent → byte-exact on raw bodies.
func assertBodyEquivalent(envoyGo, envoy *fixture.ScenarioResult, originalPayload []byte) error {
	egEnc, enEnc := envoyGo.Header.Get("Content-Encoding"), envoy.Header.Get("Content-Encoding")
	if egEnc != enEnc {
		return fmt.Errorf("Content-Encoding mismatch: envoy-go=%q envoy=%q", egEnc, enEnc)
	}
	if egEnc == "" {
		if !bytes.Equal(envoyGo.Body, envoy.Body) {
			return fmt.Errorf("uncompressed bodies differ")
		}
		return nil
	}
	if egEnc != "gzip" {
		return fmt.Errorf("unsupported Content-Encoding: %q", egEnc)
	}
	egPlain, err := decompressGzip(envoyGo.Body)
	if err != nil { return fmt.Errorf("envoy-go decompress: %w", err) }
	enPlain, err := decompressGzip(envoy.Body)
	if err != nil { return fmt.Errorf("envoy decompress: %w", err) }
	if !bytes.Equal(egPlain, enPlain) {
		return fmt.Errorf("decompressed bodies differ: envoy-go=%d bytes; envoy=%d bytes", len(egPlain), len(enPlain))
	}
	if originalPayload != nil && !bytes.Equal(egPlain, originalPayload) {
		return fmt.Errorf("decompressed body differs from original input (envoy-go side)")
	}
	return nil
}

func decompressGzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil { return nil, err }
	defer r.Close()
	return io.ReadAll(r)
}

// assertNoAcceptEncodingInEchoedBody parses the JSON-echoed body from the
// echobackend and asserts the upstream-side request did NOT carry an
// Accept-Encoding header (the per-route rmAE override stripped it before
// forwarding upstream). Used by scenario 6.
func assertNoAcceptEncodingInEchoedBody(plaintext []byte) error {
	var rec struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(plaintext, &rec); err != nil {
		return fmt.Errorf("parse echoed body: %w", err)
	}
	if v, ok := rec.Headers["accept-encoding"]; ok && v != "" {
		return fmt.Errorf("Accept-Encoding NOT stripped (per-route rmAE override failed): %q", v)
	}
	return nil
}

// (Per-counter delta assertion + boundary-only tolerance for
// response_total_compressed_bytes per planner-time decision 2 land in the
// helper assertCounterDeltas; implementer fleshes out per the file-structure
// table row + ADR-0133 §Decision (iii).)
```

- [ ] **Step 2: Build the driver in isolation; verify clean**

```bash
go build ./test/fixtures/0016-http-compressor/inputs/
# expect: clean build (the package compiles even without the YAML files since
# go:embed is lazy at compile-time IFF the embedded files exist; this task
# WILL fail compilation until the YAMLs are in place — Task 12 lands them.
# Implementer may stub the YAMLs as empty files at this task to satisfy
# go:embed and unblock the build, then Task 12 fills them in.)
```

NOTE: empty stub YAMLs at this task; Task 12 lands real content. Alternatively the driver lands without `//go:embed` (renders templates from constants in this file) and Task 12 swaps to embed. PLAN settles **stub-YAMLs-first** for build-cleanness.

- [ ] **Step 3: Verify ADR-0133 intact at HEAD**

```bash
grep -nE '^## ADR-0133' docs/envoy-go/DECISIONS.md   # expect: 1 match
grep -nE 'Lands-in-task.*Task 11' docs/envoy-go/DECISIONS.md | grep 0133   # expect: 1 match
```

- [ ] **Step 4: Append Task 11 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0016-http-compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: fixture 0016 driver + decompress-and-compare body assertion [ADR-0133]

Lands the fixture 0016 driver per SPEC §7.4 + ADR-0133 §Decision (i)-(iii).
Single-listener 6-scenario sequential orchestration per planner-time
decision 11. Body-assertion mode dispatched on response Content-Encoding
header: byte-exact on uncompressed (scenarios 2, 3, 5);
decompressed-byte-exact on compressed (1, 4, 6) via compress/gzip.NewReader
+ assertBodyEquivalent helper per ADR-0133 §Decision (ii). Scenario 6
asserts upstream-side Accept-Encoding stripped via JSON-echo backend
(echobackend helper landed Task 10) parse + assertNoAcceptEncodingInEchoedBody
helper. Per-counter delta assertion uses byte-exact on 16 counters +
boundary-only tolerance (0 < value < uncompressed_input_bytes) on
response_total_compressed_bytes per planner-time decision 2 (D2
settlement; ADR-0133 §Decision (iii)). YAML files stubbed empty at this
commit; Task 12 lands real content.

ADR-0133 already exists at SPEC commit 073cb88 in final form per ADR-0044
ADR-on-impl SPEC-time-anticipation; this task references via commit
message + verifies Lands-in-task: Task 11 anchor intact at HEAD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Fixture 0016 — `envoy.yaml` + `envoy-go.yaml` bootstraps (single-listener with 6 routes per SPEC §7.2)

**Files:**
- Create (or fill): `test/fixtures/0016-http-compressor/inputs/envoy.yaml` (~95 LoC; reference Envoy bootstrap)
- Create (or fill): `test/fixtures/0016-http-compressor/inputs/envoy-go.yaml` (~95 LoC; equivalent envoy-go bootstrap)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the fixture YAMLs per SPEC §7.2 + planner-time decision 11. The single listener `l_main` carries 6 routes (4 direct_response + 1 disabled-direct-response + 1 cluster-route to echobackend). Both sides use `compressor_library.name: text_optimized` per §11.5 + ADR-0132 §Decision (v) (load-bearing for stat namespace identity); both sides set `response_direction_config: {}` per §1.1 amendment 3 + ADR-0132 §Decision (ii) for byte-equivalent stat namespace. NO new ADR.

**Precondition:** Task 11 commit on HEAD; pristine tree.
**Artifacts:** envoy.yaml + envoy-go.yaml, PROGRESS Task 12 entry.
**Acceptance:** `go test -count=1 ./test/fixtures/0016-http-compressor/inputs/` builds clean (driver + embedded YAMLs); the differential fixture is NOT YET runnable end-to-end until Task 14 lands expectations + driver counter assertions.

- [ ] **Step 1: Author `envoy.yaml`**

Per SPEC §7.2 + planner-time decision 11. Single listener `l_main`; 6 routes; HCM with `[envoy.filters.http.compressor, envoy.filters.http.router]` filter chain; cluster `c_backend` STRICT_DNS to `host.docker.internal:{{.BackendPort}}`. Listener-level Compressor: `compressor_library: {name: text_optimized, typed_config: {@type: ...Gzip}}` + `response_direction_config: {}` (defaults: min_content_length=30, content_type=8-entry list, disable_on_etag_header=false, remove_accept_encoding_header=false, uncompressible_response_codes=[]; `enabled` field omitted per §1.1 amendment 2 — default = enabled). Direct_response routes serve fixed-size bodies + content-types; the etag-strong route also carries `response_headers_to_add: [{header: {key: ETag, value: "\"abc\""}}]`. Per-route TPFC on `/per-route-disabled`: `disabled: true`; per-route TPFC on `/per-route-rmae`: `overrides.response_direction_config.remove_accept_encoding_header: true`.

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: {{.AdminPort}} }
static_resources:
  listeners:
    - name: l_main
      address: { socket_address: { address: 0.0.0.0, port_value: {{.ListenerPort}} } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_p14
                http_filters:
                  - name: envoy.filters.http.compressor
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor
                      compressor_library:
                        name: text_optimized
                        typed_config:
                          "@type": type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip
                      response_direction_config: {}
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh_main
                      domains: ["*"]
                      routes:
                        - match: { path: /text-html-1024 }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PayloadA1024}}" }
                          response_headers_to_add:
                            - { header: { key: content-type, value: "text/html" } }
                        - match: { path: /image-png-1024 }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PayloadA1024}}" }
                          response_headers_to_add:
                            - { header: { key: content-type, value: "image/png" } }
                        - match: { path: /text-html-10 }
                          direct_response:
                            status: 200
                            body: { inline_string: "AAAAAAAAAA" }
                          response_headers_to_add:
                            - { header: { key: content-type, value: "text/html" } }
                        - match: { path: /text-html-etag-strong }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PayloadB1024}}" }
                          response_headers_to_add:
                            - { header: { key: content-type, value: "text/html" } }
                            - { header: { key: etag, value: "\"abc\"" } }
                        - match: { path: /per-route-disabled }
                          direct_response:
                            status: 200
                            body: { inline_string: "{{.PayloadA1024}}" }
                          response_headers_to_add:
                            - { header: { key: content-type, value: "text/html" } }
                          typed_per_filter_config:
                            envoy.filters.http.compressor:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.CompressorPerRoute
                              disabled: true
                        - match: { path: /per-route-rmae }
                          route: { cluster: c_backend }
                          typed_per_filter_config:
                            envoy.filters.http.compressor:
                              "@type": type.googleapis.com/envoy.extensions.filters.http.compressor.v3.CompressorPerRoute
                              overrides:
                                response_direction_config:
                                  remove_accept_encoding_header: true
  clusters:
    - name: c_backend
      type: STRICT_DNS
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: host.docker.internal, port_value: {{.BackendPort}} } }
```

- [ ] **Step 2: Author `envoy-go.yaml`**

Identical to `envoy.yaml` modulo cluster type (STATIC instead of STRICT_DNS) + admin/listener port rendering. Both sides use the SAME `compressor_library.name: text_optimized` per §11.5 + ADR-0132 §Decision (v).

- [ ] **Step 3: Build + smoke-test**

```bash
go build ./test/fixtures/0016-http-compressor/inputs/
# expect: clean build (the embedded YAMLs are present)
```

- [ ] **Step 4: Append Task 12 entry to PROGRESS.md**

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0016-http-compressor/inputs/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: fixture 0016 envoy.yaml + envoy-go.yaml bootstraps (single-listener 6 routes)

Single listener l_main with 6 routes per SPEC §7.2 + planner-time
decision 11: 4 direct_response routes (/text-html-1024 +
/image-png-1024 + /text-html-10 + /text-html-etag-strong) + 1 per-route-
disabled direct_response + 1 cluster route /per-route-rmae to
c_backend (echobackend helper landed Task 10). Both sides use
compressor_library.name: text_optimized per §11.5 + ADR-0132 §Decision
(v) (load-bearing for stat namespace identity); both sides set
response_direction_config: {} per §1.1 amendment 3 + ADR-0132 §Decision
(ii) for byte-equivalent stat namespace. envoy.yaml uses STRICT_DNS
cluster type (host.docker.internal); envoy-go.yaml uses STATIC.

NO new ADR.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Fixture 0016 — `expectations.yaml` + `README.md` (narrative-only documentation per ADR-0019)

**Files:**
- Create: `test/fixtures/0016-http-compressor/expectations.yaml`
- Create: `test/fixtures/0016-http-compressor/README.md`
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands the fixture's prose documentation per ADR-0019 + the file-structure table rows. NO new ADR.

**Precondition:** Task 12 commit on HEAD; pristine tree.
**Artifacts:** expectations.yaml (~50 LoC) + README.md (~85 LoC), PROGRESS Task 13 entry.
**Acceptance:** files exist; content matches the file-structure table row narratives.

- [ ] **Step 1: Author `expectations.yaml`**

Per the file-structure table row above. Documents per-scenario equivalence claims + counter delta deltas + wire-shape divergence allow-list + body axis dispatch (byte-exact for uncompressed; decompressed-byte-exact for compressed via ADR-0133) + cross-refs.

- [ ] **Step 2: Author `README.md`**

Per the file-structure table row above. Fixture overview + 6-scenario narrative + single-listener bootstrap + wire-shape divergence-window + decompress-and-compare body-assertion discipline + per-route disabled-OR-rmAE 5th canonical + per-route SHARED stats + new shared echobackend helper + `compressor_library.name: text_optimized` load-bearing-for-stat-namespace + planner-time-decision cross-references.

- [ ] **Step 3: Append Task 13 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0016-http-compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: fixture 0016 expectations.yaml + README.md (narrative-only docs)

expectations.yaml documents the 6-scenario equivalence claims +
counter delta deltas + wire-shape divergence allow-list (content-length +
transfer-encoding on compressed scenarios per ADR-0131 §Decision (ii) +
ADR-0133 §Decision (iv)) + body axis dispatch (byte-exact for uncompressed;
decompressed-byte-exact for compressed via ADR-0133 §Decision (i)-(ii)) +
boundary-only tolerance for response_total_compressed_bytes per
planner-time decision 2 (D2 settlement). README.md narrative + 6-scenario
list + single-listener bootstrap discipline + wire-shape divergence-window
narrative + decompress-and-compare discipline + per-route disabled-OR-rmAE
5th canonical narrative + per-route SHARED stats narrative + new shared
echobackend helper note + compressor_library.name load-bearing-for-stat-
namespace note + planner-time-decision cross-references.

NO new ADR.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Fixture 0016 — driver counter-assertion fleshing + end-to-end differential pass

**Files:**
- Modify: `test/fixtures/0016-http-compressor/inputs/driver.go` (flesh out per-counter delta assertions)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task completes the driver's per-counter delta assertion machinery + runs the full differential pass against both reference Envoy and envoy-go. The 16 counters byte-exact + 1 counter (`response_total_compressed_bytes`) boundary-only per planner-time decision 2 (D2 settlement). NO new ADR.

**Precondition:** Tasks 11 + 12 + 13 committed; pristine tree; both bootstraps build clean.
**Artifacts:** Updated driver.go (per-counter delta assertions); first end-to-end differential PASS for fixture 0016, PROGRESS Task 14 entry.
**Acceptance:** `go test -count=1 -v ./test/differential/ -run Test.*0016` PASS; fixture 0016 runtime ~5-10s wallclock per SPEC §7.1.

- [ ] **Step 1: Flesh out the driver's `assertCounterDeltas` helper**

```go
// expectedCounterDeltas after the 6-request workload per SPEC §7.1 + planner-
// time decision 2 (D2 settlement boundary-only tolerance for response_total_
// compressed_bytes).
type counterDelta struct {
	name      string
	expected  int64  // exact-match expected value; -1 = boundary-only
	min, max  int64  // bounds for boundary-only assertion
}

var expectedDeltas = []counterDelta{
	{name: "envoy_http_compressor_text_optimized_gzip_response_compressed", expected: 3},
	{name: "envoy_http_compressor_text_optimized_gzip_response_not_compressed", expected: 2},
	{name: "envoy_http_compressor_text_optimized_gzip_response_content_length_too_small", expected: 1},
	{name: "envoy_http_compressor_text_optimized_gzip_response_total_uncompressed_bytes", expected: 1024 + 1024 + -1 /* + scenario6 echo body length, computed dynamically */},
	// boundary-only:
	{name: "envoy_http_compressor_text_optimized_gzip_response_total_compressed_bytes", expected: -1, min: 1, max: 1024 + 1024 + 4096 /* upper bound estimate */},
	{name: "envoy_http_compressor_text_optimized_gzip_header_compressor_used", expected: 5},
	{name: "envoy_http_compressor_text_optimized_gzip_header_compressor_overshadowed", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_header_identity", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_header_not_valid", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_header_wildcard", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_no_accept_header", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_not_compressed_etag", expected: 0},
	// 5 always-zero request_* counters (twin-series-discipline allow-list per ADR-0132 §Decision (vii)).
	{name: "envoy_http_compressor_text_optimized_gzip_request_compressed", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_request_content_length_too_small", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_request_not_compressed", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_request_total_compressed_bytes", expected: 0},
	{name: "envoy_http_compressor_text_optimized_gzip_request_total_uncompressed_bytes", expected: 0},
}
```

- [ ] **Step 2: Run the differential pass against the reference + subject pair**

```bash
go test -count=1 -v ./test/differential/ -run Test.*0016
# expect: PASS for fixture 0016 (~5-10s wallclock)
```

- [ ] **Step 3: Append Task 14 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0016-http-compressor/ docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: fixture 0016 driver counter-assertion fleshing + first differential pass

Lands the driver's per-counter delta assertion machinery: 16 counters
byte-exact + 1 counter (response_total_compressed_bytes) boundary-only
(0 < value < uncompressed_input_bytes) per planner-time decision 2 (D2
settlement). 5 always-zero request_* counters asserted as such per
ADR-0132 §Decision (vii) twin-series-discipline. End-to-end differential
pass green for fixture 0016 (~5-10s wallclock).

NO new ADR.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: BEHAVIOR_CONTRACT.md 4-edit bundle + ROADMAP row 14 in-progress→done + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4 in-place edits per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (row 14 in-progress → done; sharpen summary text)
- Modify: `docs/envoy-go/STATE.md` (advance lifecycle to phase-done)
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

This task lands Gate F (BEHAVIOR_CONTRACT populated) + the ROADMAP flip + STATE.md advance + the verbatim 6-gate verification. NO new ADR (all 5 ADRs already landed at SPEC commit per ADR-0044 SPEC-time-anticipation; ADR-0125 amendment §(viii)-(x) already landed at SPEC commit).

**Precondition:** Task 14 commit on HEAD; pristine tree; all unit + fuzzer + differential fixtures green.
**Artifacts:** BEHAVIOR_CONTRACT.md updated, ROADMAP.md updated, STATE.md advanced, PROGRESS Task 15 entry.
**Acceptance:** All 6 gates verbatim-pass per `BOOTSTRAP_PROMPT.md` §7.5: A (build/vet/lint clean), B (race-test pass on 37 packages), C (h2spec 53/53), D (18 fuzzers green at 30s), E (17 differential fixtures green), F (BEHAVIOR_CONTRACT populated).

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13**

The 4 edits are (per the verbatim Markdown patches in SPEC §13.1-§13.4):
- (a) NEW `### envoy.filters.http.compressor` subsection inserted under `## HTTP filter chain` umbrella, AFTER the existing `### envoy.filters.http.buffer` subsection (alphabetical: `buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit`) per SPEC §13.1; ~140 LoC verbatim from SPEC §13.1's Markdown block.
- (b) `## Stat-name mapping ### 29-name table` extends to **46 names** (17 new rows verbatim from SPEC §13.2 — 6 header_* + 1 not_compressed_etag + 5 response_* + 5 request_*) per SPEC §13.2; ~25 LoC.
- (c) NEW row appended to `## Equivalence Matrix` table (per SPEC §13.3; ~3 LoC verbatim).
- (d) NEW `### Phase 14 forward-pointer notes` subsection appended to `## Forward-pointer notes` section per SPEC §13.4 (~55 LoC verbatim).

The Markdown content for each patch is documented verbatim in SPEC §13 — copy verbatim with no edits.

- [ ] **Step 2: Apply ROADMAP.md row 14 flip + summary sharpening**

Row 14's status changes `in-progress → done` with a date column populated. The summary text is sharpened to align with SPEC §1.1 amendments — replace "29→40 names" with "29→46 names"; replace "11 new counters" with "17 new counters"; replace "Rule SN10" with "NO new SN rule (uses existing SN2)"; replace "8 fields consumed + 9 silent-ignored" with "6 listener consumed + 9 listener silent-ignored + 2 codec consumed + 3 codec silent-ignored = 8 grand-total consumed + 12 silent-ignored + 1 parse-rejected"; mention the framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` per ADR-0131 §Decision (vi); mention ADR-0125 amendment §(viii)-(x) for the per-route override surface FILTER-SPECIFIC and NARROWER-than-listener-level discipline. The implementer crafts a verbatim row replacement preserving all the load-bearing facts.

- [ ] **Step 3: Apply STATE.md advance**

Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated`. Set `lifecycle-state` to `awaiting next planning` (per phase 13 phase-done STATE.md precedent); set `next-skill` to `superpowers:brainstorming` against §9's family list for the next family-child; set `active-phase` to a placeholder pending the next session's selection (e.g., `<next-§9-family-row>` resolved by the next planner).

- [ ] **Step 4: Run all 6 gates verbatim**

```bash
# Gate A: build + vet + lint clean
go build ./...
go vet ./...
golangci-lint run ./...

# Gate B: race-test pass on all 37 packages (36 prior + new compressor)
go test -race -count=1 ./...
# expect: every package PASS, no -race violations

# Gate C: h2spec 53/53 PASS (per phases 04+ precedent; phase 14's H2 framework
# delta at h2dispatch.go is structurally identical to the H1 path — regression check)
make h2spec    # or whatever the project's h2spec entry-point is
# expect: 53/53 PASS

# Gate D: 18 fuzzers green at 30s budget
for fuzzer in $(go test -list 'Fuzz.*' ./internal/... 2>/dev/null | grep -E '^Fuzz'); do
    pkg=$(go test -list "$fuzzer" ./internal/... 2>/dev/null | grep -B1 "$fuzzer" | head -1)
    go test -fuzz="$fuzzer" -fuzztime=30s "$pkg"
done
# expect: all 18 fuzzers (17 prior + new FuzzCompressorConfigParse) clean exit

# Gate E: 17 differential fixtures 0000-0016 PASS
go test -count=1 -v ./test/differential/
# expect: all 17 PASS; total wallclock ~55-70s

# Gate F: BEHAVIOR_CONTRACT.md populated (verified by file diff)
git diff master -- docs/envoy-go/BEHAVIOR_CONTRACT.md | head -100
# expect: 4 patches landed
```

- [ ] **Step 5: Append Task 15 entry to PROGRESS.md**

Capture verbatim outputs for each of the 6 gates.

- [ ] **Step 6: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: phase-done — BEHAVIOR_CONTRACT 4-edit bundle + ROADMAP row 14 done + STATE.md advance + 6 gates green

Lands Gate F per SPEC §13: NEW ### envoy.filters.http.compressor
subsection under ## HTTP filter chain (~140 LoC); 29→46 stat-name
table extension (17 new rows: 6 header_* + 1 not_compressed_etag +
5 response_* + 5 request_* per ADR-0132 §Decision (i)+(vii)); NEW
Equivalence Matrix row pointing at fixture 0016 with decompress-and-
compare body-assertion discipline + boundary-only response_total_
compressed_bytes tolerance per ADR-0133; NEW ### Phase 14 forward-
pointer notes subsection (8-item deferral list + wire-shape divergence-
window note + framework primitive OverwriteBody note + min_content_
length late-revert anomaly note + stat namespace note). ROADMAP row 14
in-progress → done with sharpened summary aligning to SPEC §1.1
amendments (17 counters / NO new SN rule / 6 listener + 2 codec consumed
+ ADR-0125 amendment §(viii)-(x)). STATE.md advances lifecycle to
awaiting next planning.

All 6 gates green:
  A — build / vet / lint clean
  B — race-test pass on 37 packages
  C — h2spec 53/53 PASS
  D — 18 fuzzers green at 30s budget (17 prior + FuzzCompressorConfigParse)
  E — 17 differential fixtures 0000-0016 PASS (~55-70s wallclock)
  F — BEHAVIOR_CONTRACT populated

5 ADRs anchored (ALL ALREADY LANDED at SPEC commit 073cb88 per ADR-0044
SPEC-time-anticipation): ADR-0129 (Task 2; package + ENCODER+DECODER
HTTPFilter + 17-counter filterStats), ADR-0130 (Task 2; compiledConfig
+ codec-library Any-dispatch + Gzip level mapping), ADR-0131 (Task 4;
Path B + wire-shape divergence + EncoderFilterCallbacks.OverwriteBody
framework primitive), ADR-0132 (Task 8; 17-counter stat surface +
SN2 reuse + per-route SHARED stats), ADR-0133 (Task 11; differential-
fixture decompress-and-compare body-assertion discipline). Plus
ADR-0125 amendment §(viii)-(x) (already landed at SPEC commit).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Verify:

```bash
grep -n '^### envoy.filters.http.compressor' docs/envoy-go/BEHAVIOR_CONTRACT.md   # expect: 1 match (newly inserted)
grep -nE '^\| 14 \| http-filter-compressor .* \| done' docs/envoy-go/ROADMAP.md   # expect: 1 match
go test -count=1 ./test/differential/                                              # expect: all 17 PASS
```

---

## Task 16: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/14-http-filter-compressor/REVIEW.md`
- Modify: `docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md`

End-of-phase review per the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 cadence. Phase 14 has NO parent row (it is a top-level §9 family-child per ADR-0106), so REVIEW closes only row 14.

**Precondition:** Task 15 commit on HEAD; pristine tree; phase 14 is functionally complete + 6 gates green.
**Artifacts:** REVIEW.md (~180 LoC), PROGRESS Task 16 entry.
**Acceptance:** REVIEW landed; row 14 closed; phase 14 lifecycle complete.

- [ ] **Step 1: Invoke the requesting-code-review skill**

The skill drives the REVIEW.md authoring. Per phase 13 REVIEW.md precedent, the document covers:
- §1: Phase summary (the SPEC §4.1 deliverables + SPEC §4.2 fixture deliverables + SPEC §4.3 framework deltas; what was added vs. modified vs. retired).
- §2: ADR roster — ADR-0129..ADR-0133 (5 ADRs anchored at Tasks 2 + 4 + 8 + 11 per ADR-0044 + per-ADR Lands-in-task fields at SPEC commit) + ADR-0125 amendment §(viii)-(x) (ALREADY landed at SPEC commit per phase-13 ADR-0127-v2 in-place-update precedent).
- §3: Empirical pins outcome — all 15 §11 pins resolved at SPEC drafting; 9 ratified, 6 refuted (driving the 6 §1.1 amendment blocks); no new divergences during impl.
- §4: Gate-by-gate evidence (verbatim from PROGRESS Task 15 outputs).
- §5: Acceptance checklist confirmation (per SPEC §15 + this PLAN's per-task acceptance bullets).
- §6: Forward-pointer roster (the 8-item BEHAVIOR_CONTRACT §13.4 deferrals + the wire-shape divergence-window + the min_content_length late-revert anomaly + the per-route compressor_library swap silent-ignored).
- §7: Phase-done lessons learned (e.g., the SPEC-time §1.1 amendment-block channel as a release-valve precedent for cases where empirical findings invalidate multiple BRAINSTORM hypotheses without invalidating the structural design — phase 14 lands SIX amendments, the highest single-row count to date, and the structural design (gzip-only, response-only, Path B, ENCODER+DECODER, 5th canonical disabled-OR-override) survived intact; the framework primitive `EncoderFilterCallbacks.OverwriteBody(b []byte)` as the SECOND framework delta in §9 family-rows after phase-13 ADR-0128's decode-side primitives — establishing that encode-side framework deltas are permitted when load-bearing for the filter's algorithmic core; the decompress-and-compare body-assertion discipline as the FIRST non-byte-exact body axis in differential fixtures, generalizable to future codec/transform-filter fixtures via ADR-0133; the 17-counter filterStats as the largest stat surface per filter to date in §9 family-rows).

- [ ] **Step 2: Author REVIEW.md per skill output**

(~180 LoC mirroring phase 13 REVIEW.md structure, scaled up for phase 14's larger surface.)

- [ ] **Step 3: Append Task 16 entry to PROGRESS.md**

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/14-http-filter-compressor/REVIEW.md docs/envoy-go/phases/14-http-filter-compressor/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 14: REVIEW — end-of-phase retrospective + N-1 carry-forward

End-of-phase review per the 06.1..13 cadence. Phase 14 closed row 14.
5 ADRs anchored (ADR-0129..ADR-0133, ALL ALREADY LANDED at SPEC commit
073cb88 per ADR-0044 SPEC-time-anticipation; per-ADR Lands-in-task
fields verified intact at HEAD); ADR-0125 amendment §(viii)-(x) (already
landed at SPEC commit). 17 differential fixtures green; 18 fuzzers
green; h2spec 53/53; build/vet/lint/race-test all clean.

Lessons learned:
  - The SPEC-time §1.1 amendment-block channel handles SIX BRAINSTORM
    hypothesis refutations without invalidating the structural design
    (gzip-only, response-only, Path B, ENCODER+DECODER, 5th canonical
    disabled-OR-override survived intact). Highest single-row amendment
    count to date.
  - Framework primitive EncoderFilterCallbacks.OverwriteBody is the
    SECOND framework delta in §9 family-rows after phase-13 ADR-0128
    decode-side primitives. Encode-side framework deltas permitted when
    load-bearing for the filter's algorithmic core.
  - Decompress-and-compare body-assertion discipline (ADR-0133) is the
    FIRST non-byte-exact body axis in differential fixtures, generalizable
    to future codec/transform-filter fixtures.
  - 17-counter filterStats is the largest stat surface per filter to date
    in §9 family-rows (phase 11: 4; phase 12: 3; phase 13: 0; phase 14: 17).
  - SPEC-time anticipation of 5 ADRs in final form per ADR-0044 SPEC-time-
    anticipation discipline saves PLAN-time + impl-time ADR re-authoring;
    per-ADR Lands-in-task fields fix the impl-anchor schedule at SPEC commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## End of phase 14 implementation plan

16 tasks total. Production code ~588-678 LoC (filter + framework deltas + main.go + echobackend) + tests ~830 LoC + fixture ~545 LoC + echobackend helper ~185 LoC + docs ~785 LoC ≈ ~2935-3025 LoC across all bundles. 5 ADRs ALREADY LANDED at SPEC commit (ADR-0129..ADR-0133, per ADR-0044 SPEC-time-anticipation discipline) + ADR-0125 amendment §(viii)-(x) (also already landed at SPEC commit). 6 gates green at phase-done. 17 differential fixtures (0000-0016) PASS. 18 fuzzers green at 30s budget. h2spec 53/53. Phase 14 row closed; §9 family heading at ROADMAP line 56 stays unchanged. Next §9 family-child is row 15 — selection deferred to the next BRAINSTORM session per ADR-0106 no-sibling-stub discipline.
