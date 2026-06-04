// Package mongoproxy implements the envoy.filters.network.mongo_proxy network
// filter (ADR-0224) — a passive MongoDB legacy-wire-protocol sniffer. At phase
// 29.1 it lands the REQUEST side: the 5-field config parse (incl. the FaultDelay
// PGV arms, parsed-but-consumed-at-29.3), the in-house little-endian BSON parser
// (the 14-type upstream subset), the wire decoder (the exactly-7-opcode envelope;
// the 5 request opcodes body-decoded; OP_REPLY/OP_COMMANDREPLY recognized but not
// decoded until 29.2), the 23-stat fixed roster created eagerly under
// mongo.<stat_prefix>., the dynamic cmd/collection/callsite counter families, and
// the per-connection active-query list (written here, consumed at 29.2).
//
// It is consumer #2 of the ADR-0221 network.WriteFilter seam: it implements BOTH
// ReadFilter and WriteFilter, one instance per connection. The 29.1 OnWrite is a
// pure no-op Continue stub (the response decoder + correlation + the gauge
// increments land at 29.2; the async halt/resume seam + fault delay + access log
// land at 29.3).
package mongoproxy
