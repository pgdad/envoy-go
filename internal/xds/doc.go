// Package xds implements the envoy-go xDS / dynamic-config substrate. Phase 60.1
// opens the family with the Secret Discovery Service (SDS): a client that dials a
// named static SDS cluster (via internal/grpcclient.Dialer), opens a dedicated
// SecretDiscoveryService/StreamSecrets State-of-the-World stream, runs the
// version/nonce ACK/NACK handshake, and parses a delivered Secret{tls_certificate}
// into a *crypto/tls.Certificate exposed through the blocking SecretProvider seam
// (bounded by initial_fetch_timeout). Initial-fetch only — no rotation. This
// package does NOT import internal/tls (internal/tls imports this package at 60.2
// for the SecretProvider seam; an xds->tls edge would cycle). See ADR-0278.
package xds
