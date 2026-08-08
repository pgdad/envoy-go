package h2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// trailerValidateStreamID is an arbitrary non-zero stream id threaded through
// the direct table so the returned *Error's Stream field can be asserted (a
// stream-scoped error MUST carry the id; a connection-scoped one carries 0 and
// would tear the conn down instead of the stream).
const trailerValidateStreamID = uint32(7)

// TestValidateResponseTrailers_Table is TABLE A: the direct table over
// validateResponseTrailers. It exercises the validator in isolation — it does
// NOT go through the capture call site — so a neutered call site leaves this
// table fully green while Table B (the wire table below) reddens. That split is
// deliberate: Table B is the liveness gate for the call site, Table A is the
// correctness gate for the rule set.
//
// Enforced set per PLAN-84.1 §1.4 (MEASURED against envoyproxy/envoy
// contrib-v1.37.2, not inferred): RFC 9113 §8.2.2 connection-specific fields
// (incl. `te` with any value other than "trailers"), `content-length`, any
// pseudo-header, and a trailing block that does not carry END_STREAM.
//
// `host` and `trailer` are BARRED BY RFC 9110 §6.5.1 BUT FORWARDED VERBATIM BY
// THE REFERENCE — they are reference-parity controls, not omissions. Rejecting
// them would make the 84.2 differential RED on a correct implementation.
func TestValidateResponseTrailers_Table(t *testing.T) {
	tests := []struct {
		name          string
		fields        []hpack.HeaderField
		endStream     bool
		wantReject    bool
		wantMsgSubstr string
	}{
		// --- positive controls (must PASS) ---
		{
			name:      "legal_grpc_status",
			fields:    []hpack.HeaderField{{Name: "grpc-status", Value: "0"}},
			endStream: true,
		},
		{
			name:      "empty_block",
			fields:    nil,
			endStream: true,
		},
		{
			name:      "multi_legal_fields",
			fields:    []hpack.HeaderField{{Name: "grpc-status", Value: "0"}, {Name: "grpc-message", Value: "ok"}, {Name: "set-cookie", Value: "a=b"}},
			endStream: true,
		},
		{
			// Reference-parity control: RFC 9110 §6.5.1 bars `host` in trailers,
			// the reference FORWARDS it verbatim. Parity wins.
			name:      "host_passes",
			fields:    []hpack.HeaderField{{Name: "host", Value: "example.test"}},
			endStream: true,
		},
		{
			// Reference-parity control, same as host_passes.
			name:      "trailer_passes",
			fields:    []hpack.HeaderField{{Name: "trailer", Value: "grpc-status"}},
			endStream: true,
		},
		{
			// `te` is conditionally legal: only the value "trailers" is allowed
			// (RFC 9113 §8.2.2). Measured as forwarded by the reference.
			name:      "te_trailers_passes",
			fields:    []hpack.HeaderField{{Name: "te", Value: "trailers"}},
			endStream: true,
		},

		// --- framing (must REJECT) ---
		{
			name:          "no_end_stream",
			fields:        []hpack.HeaderField{{Name: "grpc-status", Value: "0"}},
			endStream:     false,
			wantReject:    true,
			wantMsgSubstr: "END_STREAM",
		},

		// --- pseudo-headers (must REJECT) ---
		{
			name:          "pseudo_status",
			fields:        []hpack.HeaderField{{Name: ":status", Value: "200"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `":status"`,
		},
		{
			name:          "pseudo_path",
			fields:        []hpack.HeaderField{{Name: ":path", Value: "/x"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `":path"`,
		},

		// --- content-length (must REJECT) ---
		{
			name:          "content_length",
			fields:        []hpack.HeaderField{{Name: "content-length", Value: "5"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"content-length"`,
		},
		{
			// The violating field is NOT at index 0: asserts the loop scans the
			// whole block rather than only inspecting fields[0].
			name:          "content_length_not_first",
			fields:        []hpack.HeaderField{{Name: "grpc-status", Value: "0"}, {Name: "x-foo", Value: "bar"}, {Name: "content-length", Value: "5"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"content-length"`,
		},

		// --- RFC 9113 §8.2.2 connection-specific set: ONE CASE PER MEMBER.
		// A single "some connection-specific field" case cannot catch a member
		// dropped from the shared set (break C measured exactly that on
		// "upgrade"), so the members are enumerated individually.
		{
			name:          "conn_specific_connection",
			fields:        []hpack.HeaderField{{Name: "connection", Value: "keep-alive"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"connection"`,
		},
		{
			name:          "conn_specific_keep_alive",
			fields:        []hpack.HeaderField{{Name: "keep-alive", Value: "timeout=5"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"keep-alive"`,
		},
		{
			name:          "conn_specific_proxy_connection",
			fields:        []hpack.HeaderField{{Name: "proxy-connection", Value: "keep-alive"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"proxy-connection"`,
		},
		{
			name:          "conn_specific_transfer_encoding",
			fields:        []hpack.HeaderField{{Name: "transfer-encoding", Value: "chunked"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"transfer-encoding"`,
		},
		{
			name:          "conn_specific_upgrade",
			fields:        []hpack.HeaderField{{Name: "upgrade", Value: "websocket"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"upgrade"`,
		},
		{
			name:          "te_gzip",
			fields:        []hpack.HeaderField{{Name: "te", Value: "gzip"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: "not 'trailers'",
		},

		// --- RFC 9113 §8.2.1 uppercase field names (must REJECT). Task 13
		// fix round: pre-Task-3 the trailer block was discarded outright (no
		// forward path); Task 2/3's capture+emit created the conduit, and
		// validateResponseTrailers's member/content-length checks are all
		// case-sensitive string comparisons, so an uppercase name bypassed
		// EVERY leg — capture-without-validation, the exact smuggling
		// conduit the charter forbids. buildRequest already carries the
		// mirror guard on the request side (stream.go ~:437-442); this rule
		// is about CASE, not the barred-name set, so it must also catch an
		// uppercase name that is not otherwise barred.
		{
			// The bypass case: barred lowercase, sent uppercase. Proves the
			// new leg fires BEFORE the (still case-sensitive)
			// isConnectionSpecificField/content-length legs, which would
			// silently pass "Content-Length" through untouched.
			name:          "uppercase_content_length",
			fields:        []hpack.HeaderField{{Name: "Content-Length", Value: "5"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"Content-Length"`,
		},
		{
			// Not otherwise barred at all — proves the rule is about case,
			// not membership in the RFC 9113 §8.2.2 set.
			name:          "uppercase_non_barred_name",
			fields:        []hpack.HeaderField{{Name: "X-Custom", Value: "v"}},
			endStream:     true,
			wantReject:    true,
			wantMsgSubstr: `"X-Custom"`,
		},
		{
			// PASS control: the all-lowercase form of the same non-barred
			// name must NOT be caught by the new leg.
			name:      "lowercase_non_barred_name_passes",
			fields:    []hpack.HeaderField{{Name: "x-custom", Value: "v"}},
			endStream: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateResponseTrailers(trailerValidateStreamID, tc.fields, tc.endStream)
			if !tc.wantReject {
				if got != nil {
					t.Errorf("validateResponseTrailers = %v, want nil (legal trailer block)", got)
				}
				return
			}
			if got == nil {
				t.Errorf("validateResponseTrailers = nil, want a rejection")
				return
			}
			if got.Code != ErrInternalError {
				t.Errorf("Code = %v, want INTERNAL_ERROR", got.Code)
			}
			if got.Stream != trailerValidateStreamID {
				t.Errorf("Stream = %d, want %d (stream-scoped, not connection-scoped)", got.Stream, trailerValidateStreamID)
			}
			// The offending field is asserted in its QUOTED form. An unquoted
			// assertion is unfalsifiable wherever the name also occurs in the
			// message's own fixed prefix: substring "connection" is satisfied by
			// "connection-specific header field" alone, so dropping the
			// interpolated name would stay green.
			if !strings.Contains(got.Error(), tc.wantMsgSubstr) {
				t.Errorf("Error() = %q, want it to name the offending field (substring %q)", got.Error(), tc.wantMsgSubstr)
			}
			// The sentinel — not the code — is what a caller must discriminate
			// on; see TestMalformedTrailers_SentinelDiscriminatesPeerReset.
			if !errors.Is(got, ErrMalformedTrailers) {
				t.Errorf("errors.Is(got, ErrMalformedTrailers) = false for %v, want true", got)
			}
		})
	}
}

// writeScriptedTrailers writes a leading HEADERS block (:status, never
// END_STREAM), an optional DATA frame (never END_STREAM), then a trailing
// HEADERS block whose END_STREAM flag is caller-controlled.
//
// It is the malformed-framing sibling of writeResponseWithTrailers, which
// always terminates the stream on the trailing block. Table B's no_end_stream
// case needs a trailing block that does NOT carry END_STREAM, which no existing
// peer helper can produce.
func (p *fakeH2ServerPeer) writeScriptedTrailers(streamID uint32, status int, body []byte, trailers []hpack.HeaderField, endStream bool) error {
	block := p.encodeHeaders([]hpack.HeaderField{{Name: ":status", Value: strconv.Itoa(status)}})
	if err := p.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return err
	}
	if len(body) > 0 {
		if err := p.fr.WriteData(streamID, false, body); err != nil {
			return err
		}
	}
	trailerBlock := p.encodeHeaders(trailers)
	return p.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: trailerBlock,
		EndStream:     endStream,
		EndHeaders:    true,
	})
}

// dialClientConnTCP mirrors dialClientConn but runs the pair over a LOOPBACK
// TCP socket instead of net.Pipe.
//
// ⚠️ THIS IS LOAD-BEARING, NOT A STYLE CHOICE. net.Pipe is synchronous and
// unbuffered: every write blocks until the far side reads. Table B's reject
// path now writes RST_STREAM from the readLoop goroutine while the peer may be
// mid-write of its next frame — over net.Pipe that is a guaranteed deadlock
// (client blocked writing the RST, peer blocked writing DATA, neither reading).
// The kernel socket buffers absorb both. Same family as the recorded
// net.Pipe-deadlocks-a-client-cert-handshake finding.
func dialClientConnTCP(t *testing.T) (*ClientConn, *fakeH2ServerPeer, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	type accepted struct {
		conn net.Conn
		err  error
	}
	accCh := make(chan accepted, 1)
	go func() {
		c, aerr := ln.Accept()
		accCh <- accepted{c, aerr}
	}()
	clientSide, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		_ = ln.Close()
		t.Fatalf("dial: %v", err)
	}
	acc := <-accCh
	_ = ln.Close()
	if acc.err != nil {
		_ = clientSide.Close()
		t.Fatalf("accept: %v", acc.err)
	}
	serverSide := acc.conn

	peer := newFakeH2ServerPeer(t, serverSide)
	hsErr := peer.runHandshakeAsync()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	cc, err := NewClientConn(ctx, clientSide)
	if err != nil {
		cancel()
		_ = clientSide.Close()
		_ = serverSide.Close()
		t.Fatalf("NewClientConn: %v", err)
	}
	if err := <-hsErr; err != nil {
		cancel()
		_ = cc.Close()
		_ = serverSide.Close()
		t.Fatalf("peer handshake: %v", err)
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cancel()
			_ = cc.Close()
			_ = serverSide.Close()
		})
	}
	return cc, peer, cleanup
}

// awaitPingAck writes a PING and reads frames until its ACK comes back. It is a
// LIVENESS BARRIER placed DOWNSTREAM of the frame whose handling is under test:
// HTTP/2 frames on one connection are processed in order, so an ACK for a PING
// written after frame F proves the client's readLoop consumed F, survived it,
// and is still serving the connection. A bare cc.Closed() poll cannot prove
// that — it is equally green when the readLoop simply has not got there yet.
func (p *fakeH2ServerPeer) awaitPingAck() error {
	if err := p.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	defer func() { _ = p.conn.SetReadDeadline(time.Time{}) }()
	payload := [8]byte{'l', 'i', 'v', 'e', 'n', 'e', 's', 's'}
	if err := p.fr.WritePing(false, payload); err != nil {
		return fmt.Errorf("write PING: %w", err)
	}
	for {
		f, err := p.readNextFrame()
		if err != nil {
			return fmt.Errorf("waiting for PING ACK: %w", err)
		}
		if pf, ok := f.(*http2.PingFrame); ok && pf.IsAck() {
			return nil
		}
	}
}

// TestClientConn_RoundTrip_TrailerValidation_Wire is TABLE B: six cases driven
// through a real ClientConn/peer conn pair. THIS IS THE LIVENESS GATE — it is
// the only table that exercises the capture-site call into the validator, so
// neutering the call site (validator left intact) reddens this table while
// Table A stays fully green.
//
// Each reject case asserts FIVE properties, because a rejection can be wrong in
// five independent ways:
//  1. RoundTrip returns a non-nil error (never a "cleaned" 200),
//  2. carried as a stream-scoped *Error with code INTERNAL_ERROR (so
//     serverStream.dispatch emits RST_STREAM(INTERNAL_ERROR) downstream) AND
//     wrapping ErrMalformedTrailers, the sentinel callers discriminate on,
//  3. the rejection is NOT a hang-to-timeout (context.DeadlineExceeded),
//  4. the message names the offending field, and
//  5. the CONN SURVIVES (cc.Closed() == false) — the reference resets the
//     stream, not the connection, so nothing here may tear down / evict the
//     pooled upstream conn.
//
// The no_end_stream case carries a deliberately SHORT (~500 ms) context: at the
// pre-fix tip that case fails by HANGING to the request timeout (PLAN-84.1
// §1.9 defect 1, measured at 1.5 s), so a 5 s context would turn a fast red
// into a slow one.
//
// data_after_reject is the DISCRIMINATING case for property 5: a rejected block
// that carried no END_STREAM leaves the peer believing the stream is open, so
// it keeps sending. Without the upstream RST_STREAM + markReset on the reject
// path, that late DATA misses lookupStream, falls into the "stream gone" arm,
// and returns a CONNECTION-level STREAM_CLOSED that cancels cc.ctx and kills
// the pooled conn. The other five cases cannot see that — their peers stop.
func TestClientConn_RoundTrip_TrailerValidation_Wire(t *testing.T) {
	tests := []struct {
		name           string
		trailers       []hpack.HeaderField
		endStream      bool
		postRejectData bool // peer keeps sending on the stream after the reject
		ctxTimeout     time.Duration
		wantErrSubstr  string // "" ⇒ expect success
	}{
		{
			name:       "success_capture",
			trailers:   []hpack.HeaderField{{Name: "grpc-status", Value: "0"}},
			endStream:  true,
			ctxTimeout: 5 * time.Second,
		},
		{
			name:          "no_end_stream",
			trailers:      []hpack.HeaderField{{Name: "grpc-status", Value: "0"}},
			endStream:     false,
			ctxTimeout:    500 * time.Millisecond,
			wantErrSubstr: "END_STREAM",
		},
		{
			name:           "data_after_reject",
			trailers:       []hpack.HeaderField{{Name: "grpc-status", Value: "0"}},
			endStream:      false,
			postRejectData: true,
			ctxTimeout:     500 * time.Millisecond,
			wantErrSubstr:  "END_STREAM",
		},
		{
			name:          "content_length",
			trailers:      []hpack.HeaderField{{Name: "grpc-status", Value: "0"}, {Name: "content-length", Value: "5"}},
			endStream:     true,
			ctxTimeout:    5 * time.Second,
			wantErrSubstr: `"content-length"`,
		},
		{
			name:          "connection_specific",
			trailers:      []hpack.HeaderField{{Name: "connection", Value: "keep-alive"}},
			endStream:     true,
			ctxTimeout:    5 * time.Second,
			wantErrSubstr: `"connection"`,
		},
		{
			name:          "pseudo_header",
			trailers:      []hpack.HeaderField{{Name: ":status", Value: "200"}},
			endStream:     true,
			ctxTimeout:    5 * time.Second,
			wantErrSubstr: `":status"`,
		},
		{
			// Task 13 fix round wire case: the uppercase leg must reject over
			// the real wire too, not merely in the direct table.
			name:          "uppercase_field_name",
			trailers:      []hpack.HeaderField{{Name: "Content-Length", Value: "5"}},
			endStream:     true,
			ctxTimeout:    5 * time.Second,
			wantErrSubstr: `"Content-Length"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc, peer, cleanup := dialClientConnTCP(t)
			defer cleanup()

			peerDone := make(chan error, 1)
			roundTripDone := make(chan struct{})
			go func() {
				hf, _, err := peer.readRequestHeaders()
				if err != nil {
					peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
					return
				}
				if werr := peer.writeScriptedTrailers(hf.StreamID, 200, []byte("hello"), tc.trailers, tc.endStream); werr != nil {
					peerDone <- fmt.Errorf("writeScriptedTrailers: %w", werr)
					return
				}
				if tc.postRejectData {
					// Sequenced AFTER RoundTrip returns so the stream-map entry
					// is already gone — that is the state in which a missing
					// markReset takes the connection down, and sending earlier
					// would race it into the harmless "stream still live" arm.
					<-roundTripDone
					if werr := peer.fr.WriteData(hf.StreamID, true, []byte("late")); werr != nil {
						peerDone <- fmt.Errorf("late DATA: %w", werr)
						return
					}
					if perr := peer.awaitPingAck(); perr != nil {
						peerDone <- fmt.Errorf("conn did not survive the late DATA: %w", perr)
						return
					}
				}
				peerDone <- nil
				// Keep reading so the client never blocks on a full socket
				// buffer; exits when cleanup() closes the conn.
				for {
					if _, rerr := peer.readNextFrame(); rerr != nil {
						return
					}
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), tc.ctxTimeout)
			defer cancel()
			resp, err := cc.RoundTrip(ctx, H2Request{
				Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
			})
			close(roundTripDone)

			select {
			case perr := <-peerDone:
				if perr != nil {
					t.Errorf("peer scripting: %v", perr)
				}
			case <-time.After(10 * time.Second):
				t.Errorf("peer goroutine did not finish")
			}

			if tc.wantErrSubstr == "" {
				if err != nil {
					t.Errorf("RoundTrip = %v, want nil (legal trailer block)", err)
					return
				}
				if resp.Status != 200 {
					t.Errorf("Status = %d, want 200", resp.Status)
				}
				if string(resp.Body) != "hello" {
					t.Errorf("Body = %q, want %q", resp.Body, "hello")
				}
				if len(resp.Trailers) != 1 {
					t.Errorf("len(Trailers) = %d, want 1: %+v", len(resp.Trailers), resp.Trailers)
				} else if resp.Trailers[0].Name != "grpc-status" || resp.Trailers[0].Value != "0" {
					t.Errorf("Trailers[0] = %+v, want {grpc-status 0}", resp.Trailers[0])
				}
				return
			}

			// (1) a rejection, never a cleaned 200.
			if err == nil {
				t.Errorf("RoundTrip = nil error, want a rejection (status=%d trailers=%+v)", resp.Status, resp.Trailers)
				return
			}
			if resp.Status != 0 || len(resp.Trailers) != 0 {
				t.Errorf("rejected RoundTrip returned a response: status=%d trailers=%+v, want the zero H2Response", resp.Status, resp.Trailers)
			}
			// (3) not a hang-to-timeout. Checked BEFORE the code/message
			// assertions so a hang reports as a hang rather than as a wrong code.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				t.Errorf("RoundTrip = %v: hung to the request timeout instead of failing the stream", err)
				return
			}
			// (2) stream-scoped *Error carrying INTERNAL_ERROR + the sentinel.
			var h2err *Error
			if !errors.As(err, &h2err) {
				t.Errorf("RoundTrip error is %T (%v), want *h2.Error", err, err)
			} else {
				if h2err.Code != ErrInternalError {
					t.Errorf("Code = %v, want INTERNAL_ERROR", h2err.Code)
				}
				if h2err.Stream == 0 {
					t.Errorf("Stream = 0 (connection-scoped), want the stream id — a conn-scoped error resets the CONN, not the stream")
				}
			}
			if !errors.Is(err, ErrMalformedTrailers) {
				t.Errorf("errors.Is(err, ErrMalformedTrailers) = false for %v, want true", err)
			}
			// (4) the message names the offending field.
			if !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("error = %q, want it to name the offending field (substring %q)", err.Error(), tc.wantErrSubstr)
			}
			// (5) the conn survives: reset the stream, not the connection.
			if cc.Closed() {
				t.Errorf("cc.Closed() = true after a rejected trailer block; the reference resets the STREAM, not the conn")
			}
		})
	}
}

// TestMalformedTrailers_SentinelDiscriminatesPeerReset is the NEGATIVE CONTROL
// for ErrMalformedTrailers, and the reason the sentinel exists at all.
//
// A peer RST_STREAM(INTERNAL_ERROR) finishes the stream with an *Error whose
// Code is INTERNAL_ERROR — IDENTICAL to a malformed-trailers rejection. A
// caller (the router, at Task 5) that discriminated on the code alone would
// treat a peer-originated reset as a locally-detected trailer violation. Both
// arms are driven over the wire so neither side is a hand-built claim about
// what the codec produces.
func TestMalformedTrailers_SentinelDiscriminatesPeerReset(t *testing.T) {
	t.Run("peer_reset_is_NOT_malformed_trailers", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
				return
			}
			peerDone <- peer.fr.WriteRSTStream(hf.StreamID, http2.ErrCodeInternal)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if perr := <-peerDone; perr != nil {
			t.Fatalf("peer: %v", perr)
		}
		if err == nil {
			t.Errorf("RoundTrip = nil, want the peer reset surfaced as an error")
			return
		}
		var h2err *Error
		if !errors.As(err, &h2err) {
			t.Errorf("error is %T (%v), want *h2.Error", err, err)
		} else if h2err.Code != ErrInternalError {
			// If this ever stops being INTERNAL_ERROR the sentinel's rationale
			// weakens — the test would no longer be proving that the code is a
			// non-discriminator. Assert it explicitly rather than assume it.
			t.Errorf("Code = %v, want INTERNAL_ERROR (the code a malformed-trailers reject also carries)", h2err.Code)
		}
		if errors.Is(err, ErrMalformedTrailers) {
			t.Errorf("errors.Is(peerResetErr, ErrMalformedTrailers) = true for %v, want false", err)
		}
	})

	t.Run("malformed_trailers_IS_malformed_trailers", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
				return
			}
			peerDone <- peer.writeScriptedTrailers(hf.StreamID, 200, []byte("hello"),
				[]hpack.HeaderField{{Name: "content-length", Value: "5"}}, true)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if perr := <-peerDone; perr != nil {
			t.Fatalf("peer: %v", perr)
		}
		if err == nil {
			t.Errorf("RoundTrip = nil, want a malformed-trailers rejection")
			return
		}
		if !errors.Is(err, ErrMalformedTrailers) {
			t.Errorf("errors.Is(err, ErrMalformedTrailers) = false for %v, want true", err)
		}
	})
}
