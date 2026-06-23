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
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
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
	streams       sync.Map       // map[uint32]*clientStream
	mu            sync.Mutex     // serializes framer writes
	closeOnce     sync.Once
	goawayCh      chan struct{} // closed when peer GOAWAY observed
	settingsAckCh chan struct{} // closed when peer ACKs our SETTINGS
	// recvDebitSinceLastUpdate accumulates inbound DATA bytes consumed against
	// the connection-level recv window since the last conn-level WINDOW_UPDATE
	// was emitted. Mutated only from the readLoop goroutine (in dispatchFrame's
	// DATA case), so no separate mutex.
	recvDebitSinceLastUpdate int32
	goawaySent               bool // true after we've emitted our own GOAWAY
}

// clientStream is the per-stream state for a single in-flight RoundTrip.
// Its life cycle: created by RoundTrip, registered in cc.streams, mutated
// by both RoundTrip (caller goroutine: encoder + DATA writer) and the
// readLoop goroutine (response HEADERS + DATA + RST_STREAM dispatch).
//
// Synchronization: RoundTrip touches respHeaders / respStatus / respBody
// only AFTER receiving on doneCh, which provides the happens-before edge
// from the readLoop's writes via channel send. closedOnce guards finish
// against duplicate close-of-doneCh.
type clientStream struct {
	id              uint32
	cc              *ClientConn
	sendW           *window
	recvW           *window
	respHeaders     []hpack.HeaderField // populated on inbound HEADERS (first block)
	respHeadersSeen bool                // true after the first HEADERS block has been observed
	respStatus      int                 // populated on inbound HEADERS (parsed from :status)
	respBody        bytes.Buffer
	doneCh          chan error // buffered 1; receives nil on END_STREAM, *Error on stream-error
	closedOnce      sync.Once
	// recvDebitSinceLastUpdate accumulates inbound DATA bytes consumed against
	// the per-stream recv window since the last stream-level WINDOW_UPDATE
	// was emitted. Mutated only from the readLoop goroutine.
	recvDebitSinceLastUpdate int32
}

// newClientStream constructs a per-stream state value with windows seeded
// from the current SETTINGS values. Per RFC 9113 §6.9.2, peer-advertised
// SETTINGS_INITIAL_WINDOW_SIZE governs the per-stream SEND window (what we
// can put into the stream); our own advertised value governs the per-stream
// RECV window (what the peer can put into the stream before we ack with
// WINDOW_UPDATE). On the client side these are stored in cc.serverS and
// cc.clientS respectively.
func newClientStream(id uint32, cc *ClientConn) *clientStream {
	peerInit := int32(cc.serverS.InitialWindowSize)
	if peerInit <= 0 {
		peerInit = 65535
	}
	return &clientStream{
		id:     id,
		cc:     cc,
		sendW:  newWindow(peerInit),
		recvW:  newWindow(int32(cc.clientS.InitialWindowSize)),
		doneCh: make(chan error, 1),
	}
}

// finish delivers err on doneCh exactly once, then closes the channel so
// any later receivers see the zero value (nil error). closedOnce guards
// against both duplicate sends (which would block on a buffered-1 channel)
// and duplicate closes (which would panic).
func (cs *clientStream) finish(err error) {
	cs.closedOnce.Do(func() {
		cs.doneCh <- err
		close(cs.doneCh)
	})
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
		nextStreamID:  0, // atomic increments by 2; first stream allocates 1
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

// lookupStream returns the *clientStream registered under id, or
// (nil, false) if no such stream exists (typically because RoundTrip's
// defer cleanup has already removed it).
func (cc *ClientConn) lookupStream(id uint32) (*clientStream, bool) {
	v, ok := cc.streams.Load(id)
	if !ok {
		return nil, false
	}
	cs, ok := v.(*clientStream)
	if !ok {
		return nil, false
	}
	return cs, true
}

// emitGoaway sends a GOAWAY frame with the given error code, once. Mirrors
// (*ServerConn).emitGoaway. The mutex is held across the framer write so
// the GOAWAY is serialized against any concurrent encodeAndWriteHeaders or
// writeData on the conn.
func (cc *ClientConn) emitGoaway(code ErrCode) {
	cc.mu.Lock()
	if cc.goawaySent {
		cc.mu.Unlock()
		return
	}
	cc.goawaySent = true
	lastID := atomic.LoadUint32(&cc.nextStreamID)
	_ = cc.fr.WriteGoAway(lastID, http2.ErrCode(code), nil)
	cc.mu.Unlock()
}

// dispatchFrame routes a single inbound frame. SETTINGS / PING / GOAWAY are
// conn-level; HEADERS / DATA / RST_STREAM / WINDOW_UPDATE for non-zero
// stream ids are routed to the per-stream state via lookupStream.
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
		// Mid-stream SETTINGS update (peer changing window sizes etc.).
		// Apply and ACK.
		_ = fr.ForeachSetting(func(s http2.Setting) error {
			switch s.ID {
			case http2.SettingMaxConcurrentStreams:
				cc.serverS.MaxConcurrentStreams = s.Val
			case http2.SettingInitialWindowSize:
				cc.serverS.InitialWindowSize = s.Val
			case http2.SettingMaxFrameSize:
				cc.serverS.MaxFrameSize = s.Val
			case http2.SettingHeaderTableSize:
				cc.serverS.HeaderTableSize = s.Val
				cc.hp.updateMaxTableSize(s.Val)
			case http2.SettingEnablePush:
				cc.serverS.EnablePush = s.Val
			}
			return nil
		})
		cc.mu.Lock()
		err := cc.fr.WriteSettingsAck()
		cc.mu.Unlock()
		return translateFramerErr(err)

	case *http2.HeadersFrame:
		cs, ok := cc.lookupStream(fr.StreamID)
		if !ok {
			// Server-initiated HEADERS (PUSH_PROMISE-class) — we advertise
			// ENABLE_PUSH=0 so this is a peer protocol violation.
			cc.emitGoaway(ErrProtocolError)
			return connError(ErrProtocolError, "client: server-initiated HEADERS with ENABLE_PUSH=0")
		}
		decoded, err := cc.hp.decodeBlock(fr.HeaderBlockFragment(), fr.HeadersEnded())
		if err != nil {
			cc.emitGoaway(ErrCompressionError)
			return err
		}
		if !cs.respHeadersSeen {
			cs.respHeadersSeen = true
			cs.respHeaders = decoded
			for _, hf := range decoded {
				if hf.Name == ":status" {
					cs.respStatus, _ = strconv.Atoi(hf.Value)
					break
				}
			}
		}
		// else: trailing HEADERS — observed-and-discarded per ADR-0058.
		if fr.StreamEnded() {
			cs.finish(nil)
		}
		return nil

	case *http2.DataFrame:
		cs, ok := cc.lookupStream(fr.StreamID)
		if !ok {
			// Stream gone (we already finished it) — DATA after END_STREAM.
			// Returning a connection-level STREAM_CLOSED here is symmetric
			// with ServerConn's recently-closed handling and forces the
			// readLoop to cancel cc.ctx.
			return connError(ErrStreamClosed, "client: DATA on closed stream")
		}
		dataLen := int32(len(fr.Data()))
		if dataLen > 0 {
			cs.respBody.Write(fr.Data())
			// Recv-window debit + half-window WINDOW_UPDATE emission per
			// ADR-0055 / I-3 (mirror of ServerConn.onData).
			cc.recvW.replenish(-dataLen)
			cs.recvW.replenish(-dataLen)
			// Conn-level debit + WINDOW_UPDATE(0, delta). Mutates cc.recvDebitSinceLastUpdate.
			cc.recvDebitSinceLastUpdate += dataLen
			if cc.recvDebitSinceLastUpdate >= recvWindowUpdateThreshold {
				delta := cc.recvDebitSinceLastUpdate
				cc.recvDebitSinceLastUpdate = 0
				cc.recvW.replenish(delta)
				cc.mu.Lock()
				werr := cc.fr.WriteWindowUpdate(0, uint32(delta))
				cc.mu.Unlock()
				if werr != nil {
					return translateFramerErr(werr)
				}
			}
			// Stream-level debit + WINDOW_UPDATE(streamID, delta). Independent
			// from the conn-level block above; mutates cs.recvDebitSinceLastUpdate
			// (different field, same spelling). Skipped on END_STREAM since the
			// stream is about to be finished and a stream-level WINDOW_UPDATE
			// after END_STREAM would be wasted work.
			cs.recvDebitSinceLastUpdate += dataLen
			if !fr.StreamEnded() && cs.recvDebitSinceLastUpdate >= recvWindowUpdateThreshold {
				delta := cs.recvDebitSinceLastUpdate
				cs.recvDebitSinceLastUpdate = 0
				cs.recvW.replenish(delta)
				cc.mu.Lock()
				werr := cc.fr.WriteWindowUpdate(fr.StreamID, uint32(delta))
				cc.mu.Unlock()
				if werr != nil {
					return translateFramerErr(werr)
				}
			}
		}
		if fr.StreamEnded() {
			cs.finish(nil)
		}
		return nil

	case *http2.RSTStreamFrame:
		cs, ok := cc.lookupStream(fr.StreamID)
		if !ok {
			return nil // stream gone; ignore
		}
		cs.finish(streamError(ErrCode(fr.ErrCode), fr.StreamID, "client: peer RST_STREAM"))
		return nil

	case *http2.WindowUpdateFrame:
		// Mirror ServerConn.onWindowUpdate per ADR-0055 / M-9.
		delta := int32(fr.Increment)
		if fr.StreamID == 0 {
			if delta == 0 {
				return connError(ErrProtocolError, "client: WINDOW_UPDATE delta 0 on connection")
			}
			if _, ok := safeAddInt32(cc.sendW.available(), delta); !ok {
				cc.emitGoaway(ErrFlowControlError)
				return connError(ErrFlowControlError, "client: WINDOW_UPDATE conn-level overflow")
			}
			cc.sendW.replenish(delta)
			return nil
		}
		// Stream-level WINDOW_UPDATE.
		if delta == 0 {
			// Stream-level zero-delta: stream error per RFC 9113 §6.9.
			// The peer expects RST_STREAM(PROTOCOL_ERROR); finishing the
			// stream with that error code is sufficient (RoundTrip will
			// return; we don't separately RST since the peer is the violator).
			cs, ok := cc.lookupStream(fr.StreamID)
			if !ok {
				return nil
			}
			cs.finish(streamError(ErrProtocolError, fr.StreamID, "client: WINDOW_UPDATE delta 0 on stream"))
			return nil
		}
		cs, ok := cc.lookupStream(fr.StreamID)
		if !ok {
			return nil
		}
		if _, ok := safeAddInt32(cs.sendW.available(), delta); !ok {
			cs.finish(streamError(ErrFlowControlError, fr.StreamID, "client: WINDOW_UPDATE stream-level overflow"))
			return nil
		}
		cs.sendW.replenish(delta)
		return nil

	case *http2.PingFrame:
		if !fr.IsAck() {
			cc.mu.Lock()
			err := cc.fr.WritePing(true, fr.Data)
			cc.mu.Unlock()
			return translateFramerErr(err)
		}
		return nil

	case *http2.GoAwayFrame:
		select {
		case <-cc.goawayCh:
		default:
			close(cc.goawayCh)
		}
		// Streams above last-stream-id should be finished with the GOAWAY
		// error code per RFC 9113 §6.8.
		cc.streams.Range(func(k, v any) bool {
			id, _ := k.(uint32)
			cs, _ := v.(*clientStream)
			if cs == nil {
				return true
			}
			if id > fr.LastStreamID {
				cs.finish(streamError(ErrCode(fr.ErrCode), id, "client: peer GOAWAY"))
			}
			return true
		})
		return nil

	default:
		// Unknown frame types are silently ignored per RFC 9113 §4.1.
		return nil
	}
}

// RoundTrip allocates a fresh stream id, encodes the request HEADERS (with
// optional DATA + END_STREAM), waits on cs.doneCh for the response, and
// returns the assembled H2Response.
//
// Per RFC 9113 §5.1.1, client-initiated streams use odd-numbered ids
// starting at 1. The atomic allocator stores nextID×2 (initialized to 0);
// each RoundTrip computes id = atomic.AddUint32(&cc.nextStreamID, 2) - 1,
// returning 1, 3, 5, ... per call.
func (cc *ClientConn) RoundTrip(ctx context.Context, req H2Request) (H2Response, error) {
	if err := cc.ctx.Err(); err != nil {
		return H2Response{}, fmt.Errorf("h2: client: RoundTrip on closed conn: %w", err)
	}
	id := atomic.AddUint32(&cc.nextStreamID, 2) - 1
	cs := newClientStream(id, cc)
	cc.streams.Store(id, cs)
	defer cc.streams.Delete(id)

	// Encode HEADERS: pseudo-headers first per RFC 9113 §8.3.
	headers := make([]hpack.HeaderField, 0, 4+len(req.Headers))
	headers = append(headers,
		hpack.HeaderField{Name: ":method", Value: req.Method},
		hpack.HeaderField{Name: ":path", Value: req.Path},
		hpack.HeaderField{Name: ":scheme", Value: req.Scheme},
		hpack.HeaderField{Name: ":authority", Value: req.Authority},
	)
	headers = append(headers, req.Headers...)

	endStream := len(req.Body) == 0
	cc.mu.Lock()
	encoded := cc.hp.encodeHeaders(headers)
	// encodeHeaders returns bytes aliased to the encoder buffer; copy before
	// passing to the framer so a subsequent encode call cannot stomp them.
	block := make([]byte, len(encoded))
	copy(block, encoded)
	werr := cc.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      id,
		BlockFragment: block,
		EndStream:     endStream,
		EndHeaders:    true,
	})
	cc.mu.Unlock()
	if werr != nil {
		return H2Response{}, translateFramerErr(werr)
	}

	// Write body if present, chunked per ADR-0055 / I-1 + I-2.
	if !endStream {
		if err := cc.writeData(ctx, cs, req.Body, true); err != nil {
			return H2Response{}, err
		}
	}

	// Wait for response, ctx cancel, or conn-close.
	select {
	case err := <-cs.doneCh:
		if err != nil {
			return H2Response{}, err
		}
		return H2Response{
			Status:  cs.respStatus,
			Headers: cs.respHeaders,
			Body:    cs.respBody.Bytes(),
		}, nil
	case <-ctx.Done():
		// Emit RST_STREAM(CANCEL) on the upstream stream; return ctx error.
		cc.mu.Lock()
		_ = cc.fr.WriteRSTStream(id, http2.ErrCodeCancel)
		cc.mu.Unlock()
		return H2Response{}, ctx.Err()
	case <-cc.ctx.Done():
		return H2Response{}, fmt.Errorf("h2: client: conn closed mid-RoundTrip: %w", cc.ctx.Err())
	}
}

// writeData chunks b into one or more DATA frames respecting the same
// flow-control discipline as ServerConn.writeData (per ADR-0055 / I-1 + I-2).
// Symmetric implementation; the only difference is the per-stream window
// belongs to clientStream rather than serverStream.
//
// Each DATA frame is bounded by min(connSendWindow, streamSendWindow,
// peer.MaxFrameSize). Per-stream window is reserved FIRST (smaller bound
// reduces head-of-line blocking on the conn-level window); conn-level is
// reserved second for the streamTaken amount; on conn-level under-
// reservation we replenish the stream-level over-reservation so per-stream
// window state stays consistent.
func (cc *ClientConn) writeData(ctx context.Context, cs *clientStream, b []byte, endStream bool) error {
	// Peer SETTINGS_MAX_FRAME_SIZE cap. RFC 9113 §6.5.2 default is 16384 if
	// the peer has not yet sent SETTINGS (cc.serverS.MaxFrameSize == 0).
	maxFrame := int32(cc.serverS.MaxFrameSize)
	if maxFrame <= 0 {
		maxFrame = 16384
	}

	remaining := b
	for len(remaining) > 0 {
		want := int32(len(remaining))
		if want > maxFrame {
			want = maxFrame
		}

		// Reserve per-stream first.
		streamTaken, err := cs.sendW.reserveBlocking(ctx, want)
		if err != nil {
			return err
		}
		// Reserve conn-level for the streamTaken amount.
		connTaken, err := cc.sendW.reserveBlocking(ctx, streamTaken)
		if err != nil {
			// Replenish per-stream over-reservation on ctx cancel.
			cs.sendW.replenish(streamTaken)
			return err
		}
		if connTaken < streamTaken {
			cs.sendW.replenish(streamTaken - connTaken)
		}

		chunk := remaining[:connTaken]
		remaining = remaining[connTaken:]
		isLast := len(remaining) == 0 && endStream

		cc.mu.Lock()
		werr := cc.fr.WriteData(cs.id, isLast, chunk)
		cc.mu.Unlock()
		if werr != nil {
			return translateFramerErr(werr)
		}
	}

	// Empty body with endStream: emit a zero-length DATA frame with
	// END_STREAM. The chunking loop above is skipped when len(b) == 0.
	if len(b) == 0 && endStream {
		cc.mu.Lock()
		err := cc.fr.WriteData(cs.id, true, nil)
		cc.mu.Unlock()
		return translateFramerErr(err)
	}
	return nil
}

// Closed reports whether this connection's frame loop has torn down (the
// conn-lifetime ctx is canceled, e.g. after Close or a transport error). The
// h2 pool's admission scan skips a Closed conn; the pool evicts+Closes a
// Closed conn once its last in-flight stream drains. (phase 43.2a, ADR-0253)
func (cc *ClientConn) Closed() bool { return cc.ctx.Err() != nil }

// Close emits a graceful GOAWAY(NO_ERROR) with the highest allocated stream
// id as last-stream-id and closes the underlying conn. Idempotent — safe to
// call from multiple goroutines.
func (cc *ClientConn) Close() error {
	var closeErr error
	cc.closeOnce.Do(func() {
		lastID := atomic.LoadUint32(&cc.nextStreamID)
		cc.mu.Lock()
		if !cc.goawaySent {
			cc.goawaySent = true
			_ = cc.fr.WriteGoAway(lastID, http2.ErrCode(ErrNoError), []byte("client close"))
		}
		cc.mu.Unlock()
		cc.cancel()
		closeErr = cc.conn.Close()
	})
	return closeErr
}
