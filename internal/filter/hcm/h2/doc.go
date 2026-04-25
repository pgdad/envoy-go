// Package h2 implements envoy-go's downstream HTTP/2 server-side codec for
// phase 05.1. It drives golang.org/x/net/http2.Framer + golang.org/x/net/http2/hpack
// as low-level codec only (per doctrine D-3.2 and ADR-0046); the connection
// manager (ServerConn), per-stream state machine (serverStream), settings
// handshake, flow control, and error dispatch are all envoy-go-owned.
//
// This package is server-side only in 05.1. ClientConn / RoundTrip live in
// client.go and land in 05.2 per ADR-0045 — there is intentionally no
// client.go file in this package at phase 05.1 close.
//
// Phase 05.1 surface: see docs/envoy-go/phases/05.1-downstream-h2/SPEC.md §4.1.
// Doctrine: see docs/envoy-go/DECISIONS.md ADR-0046 (codec source: x/net/http2.Framer
// + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection
// manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec
// threshold + pin), ADR-0052 (BEHAVIOR_CONTRACT HTTP/2 subsection).
//
// Error-prefix discipline: every error returned by this package begins with
// "h2: ". Stream-scoped errors include " stream=<id>". The fuzz targets
// FuzzFrameStream and FuzzHPACKDecode grep for "h2:" to validate the
// discipline; do not break it without updating the fuzzers.
//
// What this package does NOT do: it does NOT use http2.Server,
// http2.Server.ServeConn, http2.ConfigureServer, http2.Transport, or
// http2.Transport.NewClientConn. The connection lifecycle is driven explicitly
// by ServerConn.Run. See doctrine D-3.2 and ADR-0048.
package h2
