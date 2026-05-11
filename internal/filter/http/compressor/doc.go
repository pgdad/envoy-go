// Package compressor implements envoy.filters.http.compressor — Envoy
// v1.37.2's canonical "gzip-compress upstream response body before forwarding
// downstream" filter (gzip-only MVP, response-side only) under the 07.1
// HTTP filter framework.
//
// Listener-level Compressor proto fields envoy-go consumes:
//   - compressor_library (TypedExtensionConfig REQUIRED; gzip-only MVP per
//     ADR-0130 §Decision (ii)-(iii))
//   - response_direction_config.{common_config.{min_content_length,
//     content_type}, disable_on_etag_header, remove_accept_encoding_header}
//
// Silent-ignored at parse (ADR-0040 silent-ignore discipline):
//   - 4 deprecated top-level mirrors (content_length, content_type,
//     disable_on_etag_header, remove_accept_encoding_header) — Envoy
//     ignores them whenever response_direction_config is present (see
//     Compressor proto v3 doc comment line 76-85)
//   - runtime_enabled (deprecated runtime gate)
//   - choose_first (always-q-value selection per §1.1 amendment / D-PLAN)
//   - request_direction_config (always-disabled, response-only MVP)
//   - response_direction_config.common_config.enabled (RuntimeFeatureFlag
//     with BoolValue default; always-active per §1.1 amendment 2)
//
// Parse-rejected (envoy-go-only validation per ADR-0130 §Decision (vi)):
//   - compressor_library.typed_config with TypeURL != Gzip — Envoy accepts
//     other codecs; envoy-go's gzip-only MVP rejects at parse with explicit
//     error wording.
//
// Codec-library Gzip proto (5 fields):
//   - Consumed: compression_level (mapped to Go compress/gzip levels per
//     ADR-0130 §Decision (iv) table) + compression_strategy (HUFFMAN_ONLY
//     honored / others collapse to default per §Decision (v))
//   - Silent-ignored: memory_level, window_bits, chunk_size (Go gzip does
//     not expose libz-equivalent knobs)
//
// Per-route CompressorPerRoute proto: oneof override carrying disabled: true
// shortcut OR overrides: CompressorOverrides. CompressorOverrides carries
// response_direction_config: ResponseDirectionOverrides (which holds ONE field:
// remove_accept_encoding_header BoolValue per §1.1 amendment 4 — per-route
// override of min_content_length / content_type / disable_on_etag_header / etc.
// is STRUCTURALLY IMPOSSIBLE in the proto). Per ADR-0125 amendment §(viii):
// 5th canonical disabled-OR-override per-route discipline.
//
// HTTPFilter value: ENCODER+DECODER with the SAME *filter instance — Decoder: f,
// Encoder: f (FIRST §9 row using this shape with non-vacuous both paths
// structurally). Per ADR-0129 §Decision (iv). The decode side parses
// Accept-Encoding + resolves per-route + optionally strips AE from upstream
// request headers; the encode side runs the skip-decision sequence and
// (on the compress path) gzip-encodes the response body via the framework
// OverwriteBody primitive (lands at Task 4 per ADR-0131 §Decision (vi)).
//
// Per-route discipline: 5th canonical disabled-OR-override per ADR-0125
// amendment §(viii); per-route override surface is FILTER-SPECIFIC and
// NARROWER than listener-level (only remove_accept_encoding_header).
// Per-route stats SHARED with listener-level per ADR-0132 §Decision (iv).
//
// Stat surface: 17 counters per HCM stat_prefix per ADR-0132 §Decision (i).
// Namespace: compressor.<library_name>.gzip.[response.]<counter>; Prometheus
// rendering via existing Rule SN2 (NO new SN10 per §1.1 amendment 3).
//
// Cross-cutting ADR anchors:
//   - ADR-0129 (package shape + ENCODER+DECODER HTTPFilter + 17-counter
//     filterStats + boot-registration ordering)
//   - ADR-0130 (compiledConfig shape + codec-library Any-unmarshal-and-dispatch,
//     parse-rejection of unknown TypeURL + Gzip mapping table + envoy-go-only
//     error wording)
//   - ADR-0131 (Path B body algorithm + OverwriteBody framework primitive —
//     lands at Task 4)
//   - ADR-0132 (17-counter stat surface + namespace shape — wiring lands at
//     Task 8)
//   - ADR-0133 (differential-fixture decompress-and-compare body assertion —
//     lands at Task 11)
package compressor
