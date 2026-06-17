package cluster

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// DialH2 dials an upstream endpoint and wraps the conn in an *h2.ClientConn
// ready for one RoundTrip. Two transport shapes are supported:
//
//   - TLS+h2 upstream (cluster.upstreamCfg != nil): the post-dial *stdtls.Conn
//     completes its handshake (idempotently re-asserted here) and the
//     negotiated ALPN MUST be "h2"; otherwise the dial errors with the
//     diagnostics enumerated in SPEC §11.3 / §10 #5. This is the
//     phase-05.2 baseline.
//   - Plaintext h2c upstream (cluster.upstreamCfg == nil): no TLS wrap is
//     present (Cluster.Dial returned a raw *net.TCPConn inside the gauge
//     wrapper); the conn is handed directly to h2.NewClientConn for h2c
//     prior-knowledge per RFC 7540 §3.4. ADR-0166 anchors the relaxation;
//     the build-time gate (extractH2Mode in manager.go) permits the
//     transport_socket-absent shape symmetrically.
//
// See phase-05.2 SPEC §5.3 step 2 for the full upstream H2 lifecycle:
// dial → (ALPN-h2 confirmation when TLS) → preface + initial SETTINGS
// exchange (driven by h2.NewClientConn) → RoundTrip → Close. h2.NewClientConn
// performs the preface + initial SETTINGS exchange synchronously, surfacing
// handshake errors at constructor time.
//
// Per ADR-0056, the returned *h2.ClientConn is per-request fresh; the caller
// (routerActionH2.doH2 in internal/filter/hcm/actions.go) closes it via
// defer after the response is consumed. Cross-request stream pooling is the
// upstream-robustness family's deliverable, not phase 05.2's.
//
// Each error branch closes the underlying conn explicitly because the function
// returns the conn-owning *h2.ClientConn on success (caller takes ownership);
// on error there is no caller-owned wrapper to defer-close, so the underlying
// conn would otherwise leak file descriptors.
func (c *Cluster) DialH2(ctx context.Context) (*h2.ClientConn, Endpoint, error) {
	raw, ep, err := c.Dial(ctx)
	if err != nil {
		return nil, ep, fmt.Errorf("cluster: dial h2: %w", err)
	}
	// Phase 06.1 Task 9: Cluster.Dial wraps every successful dial in a
	// *connWithGauge whose Close Decs the upstream_cx_active gauge. We pass
	// the WRAPPER (not any unwrapped inner conn) into h2.NewClientConn so
	// the *h2.ClientConn.Close path closes the wrapper and Decs the gauge.
	wrapped, ok := raw.(*connWithGauge)
	if !ok {
		_ = raw.Close()
		return nil, ep, errors.New("cluster: dial h2: not a connWithGauge")
	}
	// ADR-0166: branch on cluster.upstreamCfg to decide TLS+h2 vs plaintext
	// h2c prior-knowledge. The TLS branch is preserved bit-identical with
	// the phase-05.2 baseline; the plaintext branch skips the TLS-conn
	// assertion and ALPN check.
	if c.upstreamCfg != nil {
		tlsConn, ok := wrapped.Conn.(*stdtls.Conn)
		if !ok {
			_ = wrapped.Close()
			return nil, ep, errors.New("cluster: dial h2: not a TLS conn")
		}
		// Defensive: ensure the handshake is complete so NegotiatedProtocol is
		// authoritative. HandshakeContext is idempotent on already-handshaken
		// conns (returns nil immediately); SPEC §11.3 mitigation against a future
		// Cluster.Dial refactor that might return a not-yet-handshaken *tls.Conn.
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = wrapped.Close()
			return nil, ep, fmt.Errorf("cluster: dial h2: handshake: %w", err)
		}
		alpn := tlsConn.ConnectionState().NegotiatedProtocol
		if alpn != "h2" {
			_ = wrapped.Close()
			return nil, ep, fmt.Errorf("cluster: dial h2: alpn negotiated %q, want %q", alpn, "h2")
		}
	}
	// Plaintext h2c (c.upstreamCfg == nil): no TLS, no ALPN — h2.NewClientConn
	// drives the preface + initial SETTINGS exchange over the raw conn per
	// RFC 7540 §3.4.
	cc, err := h2.NewClientConn(ctx, wrapped)
	if err != nil {
		_ = wrapped.Close()
		return nil, ep, fmt.Errorf("cluster: dial h2: client conn: %w", err)
	}
	return cc, ep, nil
}
