package cluster

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// DialH2 dials an upstream endpoint, confirms the TLS handshake's negotiated
// ALPN protocol is "h2", and wraps the conn in an *h2.ClientConn ready for
// one RoundTrip.
//
// Per ADR-0056, the returned *h2.ClientConn is per-request fresh; the caller
// (routerActionH2.do — landing in Task 11) closes it via defer after the
// response is consumed. Cross-request stream pooling is the upstream-
// robustness family's deliverable, not phase 05.2's.
//
// Each error branch closes the underlying conn explicitly because the function
// returns the conn-owning *h2.ClientConn on success (caller takes ownership);
// on error there is no caller-owned wrapper to defer-close, so the underlying
// conn would otherwise leak file descriptors.
func (c *Cluster) DialH2(ctx context.Context) (*h2.ClientConn, error) {
	raw, err := c.Dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("cluster: dial h2: %w", err)
	}
	tlsConn, ok := raw.(*stdtls.Conn)
	if !ok {
		_ = raw.Close()
		return nil, errors.New("cluster: dial h2: not a TLS conn")
	}
	// Defensive: ensure the handshake is complete so NegotiatedProtocol is
	// authoritative. HandshakeContext is idempotent on already-handshaken
	// conns (returns nil immediately); SPEC §11.3 mitigation against a future
	// Cluster.Dial refactor that might return a not-yet-handshaken *tls.Conn.
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: handshake: %w", err)
	}
	alpn := tlsConn.ConnectionState().NegotiatedProtocol
	if alpn != "h2" {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: alpn negotiated %q, want %q", alpn, "h2")
	}
	cc, err := h2.NewClientConn(ctx, tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("cluster: dial h2: client conn: %w", err)
	}
	return cc, nil
}
