// Package tls_inspector implements the envoy.filters.listener.tls_inspector
// concrete ListenerFilter. The filter peeks the connection's byte preamble
// (up to initial_read_buffer_size, default 4096), checks for a TLS
// ClientHello, and populates ChainMatchInputs.ServerName + .ApplicationProtocols
// + .TransportProtocol when a ClientHello is detected. Non-TLS preamble
// causes inputs.TransportProtocol to be set to "raw_buffer".
//
// The filter is allocated once per accepted connection by the listener
// manager (per ADR-0079 two-step factory pattern); the per-connection
// instance owns no per-connection mutable state beyond its config pointer
// (configs are immutable post-NewManager-build).
//
// Per D-3.2, this implementation does NOT bind cgo to upstream Envoy's
// tls_inspector implementation. The ClientHello parser is a hand-rolled
// minimal extractor adapted from crypto/tls/handshake_messages.go (see
// parser.go).
//
// Introduced by phase 07.2.
//
//nolint:revive // ADR-0079: package name follows envoy.filters.listener.tls_inspector type_url convention.
package tls_inspector
