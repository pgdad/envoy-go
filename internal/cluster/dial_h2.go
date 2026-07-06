package cluster

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
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
// HISTORY: under ADR-0056 this was the live router H2 path (per-request fresh
// dial + defer Close). Phase 43.2a (ADR-0253) superseded that: the router now
// acquires a multiplexed stream on a POOLED conn via Cluster.AcquireH2Stream,
// and Task 7 removed BOTH per-request-fresh router sites — so DialH2 has no
// production callers anymore. It is retained as the single-shot dial entry that
// shares the post-dial finalizer (h2ConnFromDialed) with the pool's permit-less
// dialPooledH2To; its tests (dial_h2_test.go) thereby cover the TLS/ALPN/h2c
// finalization the pool depends on. The caller takes ownership and Closes the
// returned *h2.ClientConn.
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
	return c.h2ConnFromDialed(ctx, raw, ep)
}

// dialPooledH2To is the H2 multiplex-pool's permit-less dial path to an
// ALREADY-PICKED endpoint (phase 43.2a Task 7.5, ADR-0253). The pool picks the
// endpoint EXACTLY ONCE (ctx-aware: hash key + subset match) in AcquireH2Stream
// and reserves the connection-creation permit itself (tryAcquireConnSlot /
// h2PromoteLocked) BEFORE dialing — so this path does NOT pick and does NOT
// acquireConnSlot. It dials the GIVEN ep (via the shared dialPicked body,
// ownPermit=true because the caller's reserved permit is the one dialPicked
// manages) + the shared h2ConnFromDialed finalizer.
//
// LB-RELEASE + PERMIT CONTRACT: `release` is the LB pick's release (it accounts
// load, e.g. least_request active count). It is passed straight to dialPicked
// as the pick release. On ANY non-nil error dialPicked has ALREADY fired both
// `release` (the LB release) AND releaseConnSlot (the permit) before there is
// any connWithGauge wrapper to defer-close; the caller therefore NEVER
// double-releases either — it only promotes the next waiter. On success BOTH
// transfer to the connWithGauge dec closure (connDec → release + releaseConn on
// conn Close).
func (c *Cluster) dialPooledH2To(ctx context.Context, ep Endpoint, release func()) (*h2.ClientConn, error) {
	raw, _, err := c.dialPicked(ctx, ep, release, true /*ownPermit*/)
	if err != nil {
		// dialPicked already released the LB release + the permit on its failure
		// paths (its ownPermit contract).
		return nil, fmt.Errorf("cluster: dial h2: %w", err)
	}
	cc, _, ferr := c.h2ConnFromDialed(ctx, raw, ep,
		h2.WithResetHooks(c.h2RxResetInc, c.h2TxResetInc))
	if ferr != nil {
		// h2ConnFromDialed Closed the connWithGauge wrapper on its failure paths
		// (connDec → release + releaseConn), so both are already freed.
		return nil, ferr
	}
	return cc, nil
}

// h2ConnFromDialed is the shared post-dial H2 finalization: assert the
// *connWithGauge wrapper, run the TLS/ALPN-or-h2c branch (ADR-0166), and drive
// h2.NewClientConn's preface + SETTINGS exchange. On every error branch the
// wrapper is Closed (which runs its connDec → releaseConn, freeing the pool's
// permit for the dialPooledH2To caller, and Dec's upstream_cx_active for both
// callers). Factored out of DialH2 verbatim so DialH2 and dialPooledH2To share
// one finalization (phase 43.2a, ADR-0253).
func (c *Cluster) h2ConnFromDialed(ctx context.Context, raw net.Conn, ep Endpoint, opts ...h2.ClientConnOption) (*h2.ClientConn, Endpoint, error) {
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
	cc, err := h2.NewClientConn(ctx, wrapped, opts...)
	if err != nil {
		_ = wrapped.Close()
		return nil, ep, fmt.Errorf("cluster: dial h2: client conn: %w", err)
	}
	return cc, ep, nil
}
