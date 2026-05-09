// Package buffer implements envoy.filters.http.buffer — Envoy v1.37.2's
// canonical request-body buffering filter. The filter accumulates the full
// request body before passing it downstream to the next filter; it enforces
// a configurable byte cap and rejects oversized bodies with 413.
//
// MVP envelope per phase 13 SPEC §1 + §1.1:
//   - 1 proto field actively consumed (max_request_bytes — REQUIRED; validated
//     non-nil + > 0 + ≤ 1 MiB at New time per ADR-0126).
//   - 0 proto fields deferred at listener level (no idle_timeout, no
//     per_connection_buffer_limit_bytes at this stage).
//   - Per-route: BufferPerRoute oneof carries disabled: true (filter wholly
//     inactive on route) OR buffer: {max_request_bytes: ...} (wholesale
//     override of listener cap). parsePerRoute enforces oneof semantics +
//     PGV-mirror constraints per ADR-0125 5th canonical per-route discipline.
//
// Body-counting algorithm is STREAMING-CAP ONLY per ADR-0127 v2:
// DecodeHeaders stops iteration on bodied non-disabled requests; DecodeData
// accumulates chunks; overflow triggers DataStopIterationNoBuffer +
// SendLocalReply(413); on end_stream ContinueDecoding resumes the chain.
// maybeAddContentLength mirrors reference Envoy's Content-Length injection
// on the buffered path per SPEC §6.4 + ADR-0127 v2.
// Body-counting body lands in Tasks 3-4; Task 2 lands stubs only.
//
// Decoder-only HTTPFilter value: Encoder: nil (mirrors phase 12 csrf
// ADR-0120 precedent; saves StreamEncoderFilter method set).
//
// SHARED-vacuous stats: phase 13 emits no filter-specific counters per
// SPEC §1.1 amendment 5; no *filterStats field in compiledConfig.
//
// Cross-cutting ADR anchors:
//   - ADR-0125 (package shape + decoder-only HTTPFilter + 5th canonical
//     per-route discipline: disabled-OR-override)
//   - ADR-0126 (compiledConfig shape + parse-time max_request_bytes ≤ 1 MiB
//     validation + cap-layering rationale)
//   - ADR-0127 v2 (body-counting + 413-trigger algorithm — lands Task 3)
package buffer
