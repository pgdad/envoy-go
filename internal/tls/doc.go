// Package tls parses Envoy v3 DownstreamTlsContext and UpstreamTlsContext
// protos into ready-to-use *crypto/tls.Config values, loads PEM material
// from DataSource envelopes (inline_bytes / inline_string / filename), maps
// TlsParameters fields to stdlib TLS config. Since phase 07.2 (Task 10) there
// is no listener-level GetConfigForClient callback: chain selection happens
// BEFORE the TLS handshake and the selected chain's config is passed DIRECTLY
// to stdtls.Server. Phase 95 installs the tree's first real GetConfigForClient
// since phase 03 — a per-chain one that does ALPN mismatch fallback ONLY,
// never SNI dispatch, and that has no error path at all (ADR-0317).
//
// Phase 03 surface: see docs/envoy-go/phases/03-tls/SPEC.md §4.1. Doctrine:
// see docs/envoy-go/DECISIONS.md ADR-0029 (DataSource handling), ADR-0030
// (TLS parameter mapping), ADR-0031 (stdlib crypto/tls stack selection).
//
// Throughout this package, crypto/tls is imported as stdtls to avoid a name
// collision with the package itself. Every exported error begins with "tls: "
// to match the error-prefix discipline in sibling packages.
package tls
