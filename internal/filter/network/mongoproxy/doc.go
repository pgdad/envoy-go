// Package mongoproxy implements the envoy.filters.network.mongo_proxy network
// filter (ADR-0224/0225/0226) — a passive MongoDB legacy-wire-protocol sniffer.
// Phase 29 (the FOURTH §9 Network-filters-family row) is CLOSED; the filter lands
// at FULL counter+gauge+fault-delay+access-log+drain+close-direction parity:
//
//   - 29.1 REQUEST side (ADR-0224): the 5-field config parse (incl. the FaultDelay
//     PGV arms), the in-house little-endian BSON parser (the 14-type upstream
//     subset), the wire decoder (the exactly-7-opcode envelope; the 5 request
//     opcodes body-decoded), the 23-stat fixed roster created eagerly under
//     mongo.<stat_prefix>., the dynamic cmd/collection/callsite counter families,
//     and the per-connection active-query list.
//   - 29.2 RESPONSE side + correlation (ADR-0225): the OP_REPLY/OP_COMMANDREPLY
//     decode in OnWrite, requestID<->responseTo correlation (first-match-erase),
//     the op_query_active gauge (the project's first differentially-mirrored
//     gauge) under the per-connection mutex, and emit_dynamic_metadata.
//   - 29.3 the async halt/resume seam + fault delay + access log + drain +
//     close-direction (ADR-0226) — all LANDED:
//   - fault-delay injection: maybeInjectDelay at the five request decoders;
//     delays_injected at timer-arm; StopIteration-while-pending; the timer
//     resumes via the framework's async ContinueReading halt/resume seam.
//   - the mongo access log: gated on cfg.accessLog; one JSON line per decoded
//     message both directions via the internal/accesslog pluggable Formatter
//     seam (differential-invisible — timing-bearing; unit goldens + a
//     coverage boundary, no fixture dir).
//   - cx_drain_close: reply-completion list-empty while Draining() ->
//     Connection().Close(FlushWrite).
//   - the close-direction seam D-P4 CLOSED: OnDestroy reads CloseDirection()
//     and increments cx_destroy_local/remote_with_active_rq (value parity).
//
// It is consumer #2 of the ADR-0221 network.WriteFilter seam: it implements BOTH
// ReadFilter and WriteFilter, one instance per connection. It opts into the 29.3
// async halt/resume seam via MayHalt() (true only when a delay is configured —
// never-halting chains stay byte-identical, R1).
package mongoproxy
