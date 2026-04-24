// Package tls parses Envoy v3 DownstreamTlsContext and UpstreamTlsContext
// protos into ready-to-use *crypto/tls.Config values, loads PEM material
// from DataSource envelopes (inline_bytes / inline_string / filename), maps
// TlsParameters fields to stdlib TLS config, and implements the SNI-wildcard
// match predicate used by the listener manager's GetConfigForClient callback.
//
// Phase 03 surface: see docs/envoy-go/phases/03-tls/SPEC.md §4.1. Doctrine:
// see docs/envoy-go/DECISIONS.md ADR-0029 (DataSource handling), ADR-0030
// (TLS parameter mapping), ADR-0031 (stdlib crypto/tls stack selection).
//
// Throughout this package, crypto/tls is imported as stdtls to avoid a name
// collision with the package itself. Every exported error begins with "tls: "
// to match the error-prefix discipline in sibling packages.
package tls
