// Package-level doc lives in doc.go; this file adds the client-side surface.
//
// client.go is the ONE new file in the h2 sub-package for phase 05.2 per
// ADR-0048's reservation. It carries the from-scratch H2 client connection
// manager (ClientConn), per-call request/response value types
// (H2Request/H2Response), and the symmetric mirror of ServerConn's
// surface for upstream H2 origination per ADR-0056.
//
// Per ADR-0046, this file is permitted to import golang.org/x/net/http2
// directly (the fourth file in the h2 sub-package with that allowance,
// after framer.go, settings.go, and conn.go).

package h2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// H2Request is the codec-internal request shape passed from routerActionH2
// to ClientConn.RoundTrip. The pseudo-headers are split out so the codec
// can encode them in the RFC 9113 §8.3-required order (:method, :path,
// :scheme, :authority first, then regular headers).
//
// The H2-prefixed name is mandated by ADR-0048's symbol reservation and
// matches H2Response below; the parallel naming is more readable than
// h2.Request/h2.Response from caller sites.
//
//nolint:revive // ADR-0048 reserves the H2Request name.
type H2Request struct {
	Method    string
	Path      string
	Scheme    string
	Authority string
	Headers   []hpack.HeaderField // regular headers; pseudo-headers excluded
	Body      []byte              // nil for bodyless GETs (the fixture-0004 case)
}

// H2Response is the codec-internal response shape returned by RoundTrip.
//
//nolint:revive // ADR-0048 reserves the H2Response name.
type H2Response struct {
	Status  int
	Headers []hpack.HeaderField // includes :status as the first element
	Body    []byte
}

// ClientConn is the per-upstream-conn H2 connection manager. One ClientConn
// per upstream *stdtls.Conn after Cluster.DialH2 confirms ALPN h2 (see
// internal/cluster/dial_h2.go in phase 05.2's later tasks).
//
// Per ADR-0056, phase 05.2 uses ClientConn for exactly one RoundTrip per
// instance (per-request fresh dial). The conn supports multi-RT in principle
// (the stream-id allocator is monotonic, not reset per call) but the router
// does not exploit it.
type ClientConn struct {
	ctx           context.Context // conn-lifetime ctx
	cancel        context.CancelFunc
	conn          net.Conn // the underlying TLS conn
	fr            *framer
	hp            *hpackState
	sendW         *window        // conn-level send window
	recvW         *window        // conn-level recv window
	clientS       ServerSettings // our settings (advertised)
	serverS       clientSettings // peer-advertised settings
	nextStreamID  uint32         // atomic; allocated odd from 1 per RFC 9113 §5.1.1
	streams       sync.Map       //nolint:unused // map[uint32]*clientStream — populated by Task 8
	mu            sync.Mutex     // serializes framer writes
	closeOnce     sync.Once
	goawayCh      chan struct{} // closed when peer GOAWAY observed
	settingsAckCh chan struct{} // closed when peer ACKs our SETTINGS
	// recvDebitSinceLastUpdate accumulates inbound DATA bytes consumed against
	// the connection-level recv window since the last conn-level WINDOW_UPDATE
	// was emitted. Used by Task 8's recv-side flow-control mirror of ServerConn.
	//
	//nolint:unused // populated by Task 8's RoundTrip + recv-side flow control.
	recvDebitSinceLastUpdate int32
}

// NewClientConn writes the client preface + initial SETTINGS, exchanges
// SETTINGS_ACKs synchronously with the peer, and returns a ready-to-RoundTrip
// conn. Per SPEC §10 #5: the synchronous wait surfaces handshake errors as
// constructor errors instead of mid-request errors.
//
// NewClientConn does NOT take ownership of upstream's TLS handshake — the
// caller (Cluster.DialH2) is expected to have verified ALPN h2 already.
func NewClientConn(ctx context.Context, upstream net.Conn) (*ClientConn, error) {
	ctx, cancel := context.WithCancel(ctx)
	cc := &ClientConn{
		ctx:           ctx,
		cancel:        cancel,
		conn:          upstream,
		fr:            newFramer(upstream, DefaultServerSettings.MaxFrameSize),
		hp:            newHPACKState(DefaultServerSettings.HeaderTableSize),
		sendW:         newWindow(int32(DefaultServerSettings.InitialWindowSize)),
		recvW:         newWindow(int32(DefaultServerSettings.InitialWindowSize)),
		clientS:       DefaultServerSettings,
		nextStreamID:  0, // atomic increments by 2; first stream allocates 1 (Task 8)
		goawayCh:      make(chan struct{}),
		settingsAckCh: make(chan struct{}),
	}
	// Step 1: write the client preface.
	if _, err := upstream.Write(clientPrefaceBytes); err != nil {
		cancel()
		return nil, fmt.Errorf("h2: client: write preface: %w", err)
	}
	// Step 2: write client initial SETTINGS.
	if err := writeClientInitialSettings(cc.fr, cc.clientS); err != nil {
		cancel()
		return nil, fmt.Errorf("h2: client: write SETTINGS: %w", err)
	}
	// Step 3: read peer SETTINGS, apply, write SETTINGS_ACK.
	if err := cc.readPeerSettingsAndAck(); err != nil {
		cancel()
		return nil, err
	}
	// Step 4: spawn the frame-read goroutine BEFORE waiting for SETTINGS_ACK
	// (the ACK arrives as a SETTINGS frame with the ACK flag set; the goroutine
	// closes settingsAckCh when it sees that frame).
	go cc.readLoop()
	// Step 5: wait synchronously for the peer's SETTINGS_ACK for our SETTINGS.
	select {
	case <-cc.settingsAckCh:
		return cc, nil
	case <-ctx.Done():
		cc.cancel()
		return nil, fmt.Errorf("h2: client: SETTINGS_ACK wait: %w", ctx.Err())
	}
}

// readPeerSettingsAndAck reads exactly one SETTINGS frame from the peer
// (which MUST NOT be an ACK; the peer's first frame is the peer's initial
// SETTINGS per RFC 9113 §6.5), applies the values to cc.serverS, and writes
// SETTINGS_ACK back. readClientSettings is reused here regardless of conn
// role: it reads the peer's initial SETTINGS frame and rejects an ACK on the
// first read with PROTOCOL_ERROR (the same RFC 9113 §6.5 rule applies in
// both directions).
func (cc *ClientConn) readPeerSettingsAndAck() error {
	if err := readClientSettings(cc.fr, &cc.serverS); err != nil {
		return err
	}
	cc.mu.Lock()
	err := cc.fr.WriteSettingsAck()
	cc.mu.Unlock()
	if err != nil {
		return fmt.Errorf("h2: client: write SETTINGS_ACK: %w", err)
	}
	return nil
}

// readLoop runs the conn-level frame-read goroutine. Per SPEC §10 #7, this
// is structured separately from ServerConn.Run because of role asymmetries
// (no PUSH_PROMISE acceptance; client allocates stream ids; settings
// application is peer's-not-ours).
//
// On any read or dispatch error the loop cancels cc.ctx (which surfaces to
// any pending RoundTrip via ctx.Done()) but does NOT close the underlying
// conn — the caller (or Close) owns the TCP FIN.
func (cc *ClientConn) readLoop() {
	for {
		f, err := cc.fr.readFrameCtx(cc.ctx)
		if err != nil {
			cc.cancel()
			return
		}
		if err := cc.dispatchFrame(f); err != nil {
			cc.cancel()
			return
		}
	}
}

// dispatchFrame routes a single inbound frame. The full implementation
// lands in Task 8; this skeleton handles only the SETTINGS_ACK signal
// needed for NewClientConn's synchronous wait, plus GOAWAY observation.
func (cc *ClientConn) dispatchFrame(f http2.Frame) error {
	switch fr := f.(type) {
	case *http2.SettingsFrame:
		if fr.IsAck() {
			select {
			case <-cc.settingsAckCh:
				// already closed
			default:
				close(cc.settingsAckCh)
			}
			return nil
		}
		// Mid-stream SETTINGS update (peer changing window sizes etc.) —
		// apply and ACK. Task 8 expands this.
		return nil
	case *http2.GoAwayFrame:
		select {
		case <-cc.goawayCh:
		default:
			close(cc.goawayCh)
		}
		return nil
	default:
		// Stream-routed frames: handled by Task 8's per-stream channels.
		// Skeleton ignores them (will become correct when Task 8 lands).
		return nil
	}
}

// RoundTrip is stubbed in this task. Task 8 lands the full implementation
// (HEADERS encode + DATA chunking + HEADERS+DATA decode + flow control).
func (cc *ClientConn) RoundTrip(_ context.Context, _ H2Request) (H2Response, error) {
	return H2Response{}, errors.New("h2: client: RoundTrip not implemented (Task 8)")
}

// Close emits a graceful GOAWAY(NO_ERROR) with the highest allocated stream
// id as last-stream-id and closes the underlying conn. Idempotent — safe to
// call from multiple goroutines.
func (cc *ClientConn) Close() error {
	var closeErr error
	cc.closeOnce.Do(func() {
		lastID := atomic.LoadUint32(&cc.nextStreamID)
		cc.mu.Lock()
		_ = cc.fr.WriteGoAway(lastID, http2.ErrCode(ErrNoError), []byte("client close"))
		cc.mu.Unlock()
		cc.cancel()
		closeErr = cc.conn.Close()
	})
	return closeErr
}
