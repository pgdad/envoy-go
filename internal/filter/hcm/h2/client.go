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
	"errors"
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
	Status   int
	Headers  []hpack.HeaderField // includes :status as the first element
	Body     []byte
	Trailers []hpack.HeaderField // populated from a trailing HEADERS block, if any
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
	// hdrAccum holds the in-progress response header block when a HEADERS frame
	// arrived without END_HEADERS (RFC 9113 §6.10). nil whenever no block is
	// open. Mutated only from the readLoop goroutine (dispatchFrame) — the
	// mirror of ServerConn.hdrAccum.
	hdrAccum   *headerBlockAccumulator
	goawaySent bool // true after we've emitted our own GOAWAY
	// onRxReset / onTxReset are nil-guarded behavioral hooks fired when this conn
	// RECEIVES (dispatchFrame RSTStreamFrame) / SENDS (RoundTrip ctx.Done CANCEL)
	// a RST_STREAM. The cluster pool wires them to its http2.rx_reset / tx_reset
	// counters at dial (WithResetHooks); nil for non-pooled conns. The codec stays
	// stats-agnostic (no internal/stats import). (phase 43.2b, ADR-0254)
	onRxReset func()
	onTxReset func()
	// resetStreams tracks stream ids this conn has locally reset (RST_STREAM
	// (CANCEL) sent from RoundTrip's ctx-cancel branch). Response HEADERS/DATA
	// frames already in flight from the server for such a stream race our
	// RST_STREAM and would otherwise miss lookupStream; RFC 9113 §5.1 requires
	// tolerating frames on a stream "for a short period" after sending
	// RST_STREAM, so dispatchFrame silently discards frames for these ids
	// instead of escalating to a connection error (which would tear down every
	// other in-flight stream on this pooled multiplexed conn). Guarded by
	// resetMu; entries are pruned on observed stream completion (END_STREAM /
	// RST_STREAM) and by the resetRetainWindow monotonic-id horizon in markReset.
	resetMu      sync.Mutex
	resetStreams map[uint32]struct{}
}

// resetRetainWindow bounds resetStreams growth: when a new id is marked reset,
// entries more than this many stream-id units behind it are pruned. Stream ids
// are monotonic (odd, +2 per RoundTrip), so 256 units ≈ 128 RoundTrips — far
// past RFC 9113 §5.1's "short period" tolerance for post-reset frames.
const resetRetainWindow = 256

// markReset records id as locally reset, pruning entries beyond the retain
// window. Called from RoundTrip's ctx-cancel branch BEFORE the stream-map
// delete so the readLoop can never observe the stream as neither live nor reset.
func (cc *ClientConn) markReset(id uint32) {
	cc.resetMu.Lock()
	if cc.resetStreams == nil {
		cc.resetStreams = make(map[uint32]struct{})
	}
	cc.resetStreams[id] = struct{}{}
	for old := range cc.resetStreams {
		if old+resetRetainWindow < id {
			delete(cc.resetStreams, old)
		}
	}
	cc.resetMu.Unlock()
}

// wasReset reports whether id was locally reset and is still within the
// tolerance window. done=true additionally clears the entry (the peer's frame
// carried END_STREAM or was an RST_STREAM — the stream is finished on both
// sides and no further frames need tolerating).
func (cc *ClientConn) wasReset(id uint32, done bool) bool {
	cc.resetMu.Lock()
	_, ok := cc.resetStreams[id]
	if ok && done {
		delete(cc.resetStreams, id)
	}
	cc.resetMu.Unlock()
	return ok
}

// ClientConnOption configures a ClientConn at construction, applied BEFORE the
// readLoop goroutine starts (so hook fields are race-free). (phase 43.2b)
type ClientConnOption func(*ClientConn)

// WithResetHooks installs the RST_STREAM rx/tx behavioral hooks. Either may be
// nil. The pool passes cluster-counter increments; the codec fires them
// nil-guarded at its existing RST sites. (phase 43.2b, ADR-0254)
func WithResetHooks(onRx, onTx func()) ClientConnOption {
	return func(cc *ClientConn) { cc.onRxReset = onRx; cc.onTxReset = onTx }
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
	respTrailers    []hpack.HeaderField // populated on a trailing HEADERS block, if any (capture-only; ADR-0058)
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
func NewClientConn(ctx context.Context, upstream net.Conn, opts ...ClientConnOption) (*ClientConn, error) {
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
	// Apply options BEFORE any framer write / readLoop spawn so the hook fields
	// are written-once-at-construction and never race the readLoop goroutine.
	for _, o := range opts {
		o(cc)
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
	// Starts here: Step 3 above read the peer's SETTINGS off the socket
	// DIRECTLY, so the reader must not exist until that has completed.
	cc.fr.startReader()
	go cc.readLoop()
	// Step 5: wait synchronously for the peer's SETTINGS_ACK for our SETTINGS.
	select {
	case <-cc.settingsAckCh:
		return cc, nil
	case <-ctx.Done():
		cc.cancel()
		// The caller receives nil here and therefore can never call Close(),
		// so this is the only place the reader can be reclaimed on this path.
		cc.fr.closeReader()
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
	defer cc.fr.closeReader()
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

// debitConnRecvWindow accounts dataLen inbound DATA bytes against the
// connection-level recv window and emits the half-window conn-level
// WINDOW_UPDATE when the running debit crosses the threshold (ADR-0055 I-3).
// Called from the readLoop for every inbound DATA frame — including frames
// discarded for locally-reset streams, whose bytes still consumed the shared
// conn-level window on the peer's side. No-op for dataLen <= 0.
func (cc *ClientConn) debitConnRecvWindow(dataLen int32) error {
	if dataLen <= 0 {
		return nil
	}
	cc.recvW.replenish(-dataLen)
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
	return nil
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
		// A HEADERS frame without END_HEADERS opens a header-block accumulator and
		// is neither decoded nor acted on here; the remainder of the block arrives
		// as CONTINUATION frames (RFC 9113 §6.10). Before phase 88 this arm decoded
		// the partial fragment, so a split upstream response block silently lost
		// every field after the split point AND left the connection-scoped HPACK
		// decoder diverged from the peer's encoder — cumulative damage on a POOLED
		// upstream connection, where the next request on the same conn hangs and
		// the one after it takes the GOAWAY (ADR-0310 §Decision).
		if !fr.HeadersEnded() {
			cc.hdrAccum = startHeaderBlock(fr)
			return nil
		}
		return cc.onResponseHeaderBlock(fr.StreamID, fr.HeaderBlockFragment(), fr.StreamEnded())

	case *http2.ContinuationFrame:
		// Mirror of ServerConn.onContinuation. Before phase 88 the client had no
		// CONTINUATION arm at all: the frame fell through to the "unknown frame
		// types are silently ignored" default and its bytes were dropped.
		if err := appendContinuation(cc.hdrAccum, fr); err != nil {
			cc.hdrAccum = nil
			// appendContinuation returns only connError(...) values, so the
			// errors.As arm always succeeds and the fallback below is
			// UNREACHABLE BY CONSTRUCTION — kept, and labeled rather than
			// deleted, so that a future appendContinuation returning a plain
			// error still tears the connection down instead of silently
			// leaving a half-read header block on a POOLED conn.
			var hErr *Error
			if errors.As(err, &hErr) {
				cc.emitGoaway(hErr.Code)
			} else {
				cc.emitGoaway(ErrProtocolError)
			}
			return err
		}
		if !fr.HeadersEnded() {
			return nil
		}
		a := cc.hdrAccum
		cc.hdrAccum = nil
		return cc.onResponseHeaderBlock(a.streamID, a.buf, a.endStream)

	case *http2.DataFrame:
		cs, ok := cc.lookupStream(fr.StreamID)
		if !ok {
			if cc.wasReset(fr.StreamID, fr.StreamEnded()) {
				// Late response DATA racing our RST_STREAM(CANCEL) — tolerated
				// per RFC 9113 §5.1 and discarded. The bytes still counted
				// against the shared conn-level recv window on the peer's side,
				// so account them (keeping the conn-level WINDOW_UPDATE
				// replenishment flowing for the other in-flight streams).
				return cc.debitConnRecvWindow(int32(len(fr.Data())))
			}
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
			cs.recvW.replenish(-dataLen)
			// Conn-level debit + WINDOW_UPDATE(0, delta). Mutates cc.recvDebitSinceLastUpdate.
			if err := cc.debitConnRecvWindow(dataLen); err != nil {
				return err
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
			// Stream gone; ignore. A peer RST_STREAM also terminates the reset-
			// tolerance window for a locally-reset stream (both sides now agree
			// the stream is dead), so clear the entry if present.
			cc.wasReset(fr.StreamID, true)
			return nil
		}
		cs.finish(streamError(ErrCode(fr.ErrCode), fr.StreamID, "client: peer RST_STREAM"))
		// Fired per RST_STREAM frame, not per stream-lifetime; exactly-once holds
		// for a conformant peer (<=1 RST/stream) since finish is idempotent and
		// RoundTrip's defer removes the stream-map entry once it returns.
		if cc.onRxReset != nil {
			cc.onRxReset()
		}
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

// onResponseHeaderBlock processes one COMPLETE response header block — the
// whole of it, whether it arrived in a single HEADERS frame or was reassembled
// from a HEADERS plus CONTINUATION frames. endStream comes from the HEADERS
// frame that opened the block.
func (cc *ClientConn) onResponseHeaderBlock(streamID uint32, block []byte, endStream bool) error {
	cs, ok := cc.lookupStream(streamID)
	if !ok {
		if cc.wasReset(streamID, endStream) {
			// Late response HEADERS racing our RST_STREAM(CANCEL) — tolerated
			// per RFC 9113 §5.1. The block MUST still be HPACK-decoded (the
			// decoder's dynamic table is connection-scoped state shared with
			// every other stream's HEADERS); the decoded fields are discarded.
			if _, err := cc.hp.decodeBlock(block, true); err != nil {
				cc.emitGoaway(ErrCompressionError)
				return err
			}
			return nil
		}
		// Server-initiated HEADERS (PUSH_PROMISE-class) — we advertise
		// ENABLE_PUSH=0 so this is a peer protocol violation.
		cc.emitGoaway(ErrProtocolError)
		return connError(ErrProtocolError, "client: server-initiated HEADERS with ENABLE_PUSH=0")
	}
	decoded, err := cc.hp.decodeBlock(block, true)
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
	} else {
		// Trailing HEADERS (RFC 9113 §8.1 trailer section). VALIDATE BEFORE
		// RETAINING: the reference books this reject in the upstream codec
		// (measured: rx_messaging_error / upstream_rq_tx_reset) and the
		// downstream consequence is a stream reset, never a "cleaned" 200.
		//
		// A violation fails THIS STREAM ONLY. cs.finish carries the
		// stream-scoped *Error out through RoundTrip, and
		// serverStream.dispatch reads err.Code to emit RST_STREAM with the
		// carried code downstream. Nothing here touches the CONNECTION: no
		// GOAWAY, no connection-scoped error return (which would cancel
		// cc.ctx and tear the pooled conn down) — the reference resets the
		// stream, not the conn.
		//
		// ⚠️ THERE IS DELIBERATELY NO SECOND-TRAILING-BLOCK RULE, and none
		// may be re-added. The END_STREAM leg inside validateResponseTrailers
		// makes one unreachable by its PRESENCE (not by being checked first):
		// a first trailing block that does not terminate the stream is
		// rejected here whatever else is wrong with it, and after a LEGAL
		// END_STREAM block cs.finish has already returned RoundTrip — so no
		// second block can ever reach this branch. Leg ORDER only selects
		// which message a doubly-malformed block reports. The reference
		// behaves identically. (PLAN-84.1 §1.8, broken-gate shape 35: a
		// validation leg no wire sequence can reach still reads as coverage
		// in a direct table.)
		//
		// ⚠️ NAMED NON-GOAL — 1xx INTERIM RESPONSES ARE NOT SUPPORTED BY THIS
		// CODEC AND NOW FAIL THE STREAM. A leading 1xx HEADERS block sets
		// respHeadersSeen, so the REAL final HEADERS lands in this branch and
		// is rejected as a pseudo-header violation (it carries :status). That
		// is a change from the pre-validation tip, which silently mis-captured
		// the final block as trailers; the reference FORWARDS 1xx. Failing
		// loudly is preferred to mis-capturing, but this is a known divergence
		// — do not read it as accidental.
		if verr := validateResponseTrailers(streamID, decoded, endStream); verr != nil {
			// Reset the UPSTREAM stream too, copying RoundTrip's ctx-cancel
			// pattern exactly. markReset FIRST (before the RST write and
			// before cs.finish releases RoundTrip, whose deferred
			// cc.streams.Delete removes the map entry) so any further peer
			// frames on this id — guaranteed when the rejected block did not
			// carry END_STREAM, since the peer never terminated the stream —
			// are discarded per RFC 9113 §5.1 instead of falling through to
			// the "stream gone" arms, which return CONNECTION-level errors
			// and would tear the pooled conn down. onTxReset is what books
			// the reference's upstream_rq_tx_reset; it fires OUTSIDE cc.mu so
			// the codec mutex is never held across a cluster callback.
			cc.markReset(streamID)
			cc.mu.Lock()
			_ = cc.fr.WriteRSTStream(streamID, http2.ErrCodeInternal)
			cc.mu.Unlock()
			if cc.onTxReset != nil {
				cc.onTxReset()
			}
			cs.finish(verr)
			return nil
		}
		cs.respTrailers = decoded
	}
	if endStream {
		cs.finish(nil)
	}
	return nil
}

// ErrMalformedTrailers is the sentinel every response-trailer rejection carries
// in the Underlying field of its stream-scoped *Error, so callers can
// discriminate with errors.Is.
//
// ⚠️ THE ERROR CODE ALONE IS NOT A DISCRIMINATOR. A peer RST_STREAM whose code
// happens to be INTERNAL_ERROR finishes the stream with an identically-coded
// *Error (see the RSTStreamFrame arm above), and that case must keep its
// existing handling. The router layer needs errors.Is(err, ErrMalformedTrailers)
// — not err.Code == ErrInternalError — to tell a locally-detected malformed
// trailer block from a peer-originated reset.
var ErrMalformedTrailers = errors.New("malformed response trailers")

// malformedTrailersError builds the stream-scoped rejection: INTERNAL_ERROR (so
// serverStream.dispatch emits RST_STREAM(INTERNAL_ERROR) downstream), carrying
// the stream id (never 0 — a connection-scoped error would reset the CONN) and
// the ErrMalformedTrailers sentinel. NewStreamError takes no Underlying, hence
// the struct literal.
func malformedTrailersError(streamID uint32, msg string) *Error {
	return &Error{
		Code:       ErrInternalError,
		Stream:     streamID,
		Msg:        msg,
		Underlying: ErrMalformedTrailers,
	}
}

// validateResponseTrailers enforces the HTTP/2 trailer rules on an inbound
// trailing HEADERS block, returning nil when the block is legal and a
// STREAM-SCOPED *Error (INTERNAL_ERROR, carrying streamID) when it is not.
//
// The enforced set is MEASURED against the ADR-0008 reference pin
// (envoyproxy/envoy contrib-v1.37.2), not derived from the RFC text, because
// the two disagree:
//
//   - RFC 9113 §8.2.2 connection-specific fields (`connection`, `keep-alive`,
//     `proxy-connection`, `transfer-encoding`, `upgrade`, and `te` with any
//     value other than teTrailersValue) — REJECT. The name set comes from
//     isConnectionSpecificField in stream.go, shared with the request path so
//     the RFC list has exactly one source of truth.
//   - `content-length` — REJECT. A trailer section carries no framing.
//   - any pseudo-header (name starting ':') — REJECT. Pseudo-headers are
//     defined for the leading block only.
//   - RFC 9113 §8.2.1: any header field name containing an uppercase ASCII
//     letter — REJECT. This leg runs BEFORE the member/content-length checks
//     below, which are case-sensitive string comparisons: without it, an
//     uppercase spelling of a barred name (e.g. "Content-Length") falls
//     through every other leg untouched. Task 13's fix round: pre-Task-3 the
//     trailer block was discarded outright (no forward path); this
//     function's capture+emit creates the conduit, so an unvalidated
//     uppercase name reaching H2Response.Trailers is capture-without-
//     validation — the exact smuggling conduit this row's charter forbids.
//     Mirrors the request-side guard in buildRequest (stream.go ~:437-442).
//   - a trailing block that does not carry END_STREAM — REJECT. Its PRESENCE
//     in the rule set (not its position in the checks) is what makes a
//     second-trailing-block rule unreachable — see the capture site's comment.
//     It is evaluated first only so that a doubly-malformed block reports the
//     framing violation rather than a field violation.
//
// ⚠️ `host` and `trailer` are barred by RFC 9110 §6.5.1 but are FORWARDED
// VERBATIM by the reference (measured, 16 paths). They deliberately PASS here:
// filtering them would diverge from the reference on a correct implementation.
// Do not "complete" this list against RFC 9110.
//
// The message names the offending field, QUOTED and in trailing position. The
// quoting is load-bearing for the tests: an unquoted name is unfalsifiable when
// it also appears inside the message's own fixed prefix (dropping "+name" from
// "connection-specific header field: connection" leaves a string that still
// contains ": connection" via the prefix), so the assertion could not tell a
// message that names the field from one that does not.
// hasUppercaseHeaderChar reports whether name contains an uppercase ASCII
// letter. RFC 9113 §8.2.1 requires header field names to be lowercase on the
// wire; this mirrors the inline uppercase-rejection loop in buildRequest
// (stream.go ~:437-442) on the response-trailer side.
func hasUppercaseHeaderChar(name string) bool {
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func validateResponseTrailers(streamID uint32, fields []hpack.HeaderField, endStream bool) *Error {
	// Framing first: a trailing block MUST terminate the stream.
	if !endStream {
		return malformedTrailersError(streamID, "trailing HEADERS block without END_STREAM")
	}
	for _, hf := range fields {
		name := hf.Name
		switch {
		case len(name) > 0 && name[0] == ':':
			return malformedTrailersError(streamID,
				"pseudo-header field not permitted in a trailer section: "+strconv.Quote(name))
		case hasUppercaseHeaderChar(name):
			return malformedTrailersError(streamID,
				"uppercase character in header field name not permitted in a trailer section: "+strconv.Quote(name))
		case isConnectionSpecificField(name):
			return malformedTrailersError(streamID,
				"connection-specific header field not permitted in a trailer section: "+strconv.Quote(name))
		case name == "te" && hf.Value != teTrailersValue:
			return malformedTrailersError(streamID,
				"te header field value not 'trailers': te="+strconv.Quote(hf.Value))
		case name == "content-length":
			return malformedTrailersError(streamID,
				"content-length not permitted in a trailer section: "+strconv.Quote(name))
		}
	}
	return nil
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
	// Split into HEADERS + CONTINUATION frames bounded by the peer's advertised
	// SETTINGS_MAX_FRAME_SIZE (RFC 9113 §6.10). Emitting the whole block as one
	// frame — what this call site did before phase 88 — is illegal as soon as
	// the encoded block exceeds the peer's limit; it went unnoticed only because
	// x/net's Server reads up to 1 MiB regardless of what it advertised, while
	// its Transport enforces the 16 KiB spec default.
	werr := cc.fr.writeHeaderBlock(http2.HeadersFrameParam{
		StreamID:      id,
		BlockFragment: block,
		EndStream:     endStream,
	}, int32(cc.serverS.MaxFrameSize))
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
			Status:   cs.respStatus,
			Headers:  cs.respHeaders,
			Body:     cs.respBody.Bytes(),
			Trailers: cs.respTrailers,
		}, nil
	case <-ctx.Done():
		// Emit RST_STREAM(CANCEL) on the upstream stream; return ctx error.
		// Mark the stream reset FIRST (before the RST write and before the
		// deferred cc.streams.Delete runs at return) so late response frames
		// racing our reset are discarded by the readLoop per RFC 9113 §5.1
		// instead of tearing down the whole conn.
		cc.markReset(id)
		cc.mu.Lock()
		_ = cc.fr.WriteRSTStream(id, http2.ErrCodeCancel)
		cc.mu.Unlock()
		// Fire the tx hook OUTSIDE cc.mu (after unlock) so we never hold the
		// codec mutex across an arbitrary cluster callback. (phase 43.2b)
		if cc.onTxReset != nil {
			cc.onTxReset()
		}
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
		maxFrame = defaultPeerMaxFrameSize
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

// GoneAway reports whether this connection has observed a peer GOAWAY (its
// goawayCh has been closed by dispatchFrame's GoAwayFrame case). It is a
// non-blocking read of the closed state — distinct from Closed(): a GOAWAY'd
// conn keeps serving its in-flight streams (the ctx is NOT canceled) until
// they drain, so Closed() stays false while GoneAway() is true. The h2 pool's
// admission scan skips a GoneAway conn (takes no new streams) and evicts+Closes
// it once its last in-flight stream drains. (phase 43.2b, ADR-0254)
func (cc *ClientConn) GoneAway() bool {
	select {
	case <-cc.goawayCh:
		return true
	default:
		return false
	}
}

// GoneAwayCh returns the channel closed when a peer GOAWAY is observed, for the
// pool's per-conn drain-watcher to select on. (phase 43.2b, ADR-0254)
func (cc *ClientConn) GoneAwayCh() <-chan struct{} { return cc.goawayCh }

// Done returns the conn-lifetime ctx's Done channel, closed when the conn is
// torn down (Close or a transport error cancels cc.ctx). The drain-watcher
// selects on it as the "evicted by another path → exit" signal (so the watcher
// never leaks when a conn is evicted before any GOAWAY arrives). (phase 43.2b)
func (cc *ClientConn) Done() <-chan struct{} { return cc.ctx.Done() }

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
		// Join the reader BEFORE closing the conn, so Close carries a
		// deterministic contract: Close returned => the reader is gone.
		// cancel() above runs first so a readLoop re-entering readFrameCtx
		// takes its ctx path rather than waiting on a channel nobody closes.
		cc.fr.closeReader()
		closeErr = cc.conn.Close()
	})
	return closeErr
}
