package h2

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// respHeaderValidateStreamID is an arbitrary NON-ZERO stream id threaded through
// the direct table so the returned *Error's Stream field can be asserted. A
// stream-scoped error MUST carry the id; a connection-scoped one carries 0 and
// would tear the pooled conn down instead of failing the one stream.
const respHeaderValidateStreamID = uint32(9)

// The four leg-message prefixes validateResponseHeaders emits, spelled out here
// rather than imported so an ORDER arm can assert WHICH leg fired. Asserting
// only the quoted field name cannot discriminate two legs that both name the
// same field (`Content-Length` is named by BOTH the uppercase leg and — were it
// lowercase — the duplicate-content-length leg).
const (
	msgLegUppercase  = "uppercase character in header field name not permitted in a response header block: "
	msgLegConnSpec   = "connection-specific header field not permitted in a response header block: "
	msgLegTE         = "te header field value not 'trailers': te="
	msgLegDupContLen = "duplicate content-length header field in a response header block: "
)

// TestValidateResponseHeaders_Table is TABLE A: the direct table over
// validateResponseHeaders. It exercises the validator IN ISOLATION — it does
// NOT go through the capture call site — so a neutered call site leaves this
// table fully green while Table B (the wire table below) reddens. That split is
// deliberate and load-bearing: Table B is the liveness gate for the call site,
// Table A is the correctness gate for the rule set. A row shipping only Table A
// would pass with the validator never wired in.
//
// ⚠️ PLAN-92 §10.2 MEASURED that the validator ships COMPLETELY UNGATED by the
// pre-existing suite: adding it moved ZERO of 655 tests, and deleting its whole
// body left the suite green. This table is the only thing in the repository
// that can fail on a wrong rule set.
//
// The enforced set is MEASURED against the ADR-0008 reference pin
// (envoyproxy/envoy contrib-v1.37.2), not derived from RFC text:
// RFC 9113 §8.2.2 connection-specific fields, `te` with any value other than
// "trailers" (INCLUDING the empty value), RFC 9113 §8.2.1 uppercase field
// names, and a DUPLICATE `content-length`.
//
// ⚠️ `empty_block` is deliberately NOT an arm: a leading block with no fields is
// not a legal H/2 response at all, and this validator does not enforce
// `:status` presence. Such an arm would document a non-decision as coverage.
func TestValidateResponseHeaders_Table(t *testing.T) {
	// status200 is prepended to every arm's field list: a leading block always
	// carries :status, and its presence also proves the pseudo-header leg
	// really INVERTED relative to the trailer validator (which BARS it).
	status200 := hpack.HeaderField{Name: ":status", Value: "200"}

	tests := []struct {
		name string
		// fields are appended AFTER `:status: 200`.
		fields           []hpack.HeaderField
		wantReject       bool
		wantMsgSubstr    string
		wantMsgNotSubstr string // "" = unchecked; used by the ORDER arms
	}{
		// --- REJECT: RFC 9113 §8.2.2 connection-specific set, ONE ARM PER
		// MEMBER. A single "some connection-specific field" arm cannot catch a
		// member dropped from the shared set.
		{
			name:          "connection",
			fields:        []hpack.HeaderField{{Name: "connection", Value: "keep-alive"}},
			wantReject:    true,
			wantMsgSubstr: `"connection"`,
		},
		{
			name:          "transfer_encoding",
			fields:        []hpack.HeaderField{{Name: "transfer-encoding", Value: "chunked"}},
			wantReject:    true,
			wantMsgSubstr: `"transfer-encoding"`,
		},
		{
			name:          "keep_alive",
			fields:        []hpack.HeaderField{{Name: "keep-alive", Value: "timeout=5"}},
			wantReject:    true,
			wantMsgSubstr: `"keep-alive"`,
		},
		{
			name:          "upgrade",
			fields:        []hpack.HeaderField{{Name: "upgrade", Value: "websocket"}},
			wantReject:    true,
			wantMsgSubstr: `"upgrade"`,
		},
		{
			name:          "proxy_connection",
			fields:        []hpack.HeaderField{{Name: "proxy-connection", Value: "keep-alive"}},
			wantReject:    true,
			wantMsgSubstr: `"proxy-connection"`,
		},

		// --- REJECT: RFC 9113 §8.2.1 uppercase field name. The name is not
		// otherwise barred, so this arm is about CASE, not membership.
		{
			name:          "uppercase_name",
			fields:        []hpack.HeaderField{{Name: "X-Upper-Case", Value: "yes"}},
			wantReject:    true,
			wantMsgSubstr: `"X-Upper-Case"`,
		},

		// --- REJECT: duplicate content-length.
		{
			// ⚠️ THE TWO VALUES ARE IDENTICAL DELIBERATELY. The reference was
			// MEASURED to answer 502 for an identical duplicate, byte-identical
			// to its answer for a differing-value duplicate: the rule is ANY
			// SECOND `content-length`, not "differing values". Do not "improve"
			// this arm to differing values — that would stop pinning the rule.
			name:          "duplicate_content_length",
			fields:        []hpack.HeaderField{{Name: "content-length", Value: "5"}, {Name: "content-length", Value: "5"}},
			wantReject:    true,
			wantMsgSubstr: "duplicate content-length",
		},

		// --- REJECT: the `te` value rule. TWO shapes, because the empty value
		// is the one an implementer gets wrong.
		{
			name:          "te_gzip",
			fields:        []hpack.HeaderField{{Name: "te", Value: "gzip"}},
			wantReject:    true,
			wantMsgSubstr: "not 'trailers'",
		},
		{
			// ⚠️ THE ARM AN IMPLEMENTER GETS WRONG. Writing the leg as
			// `value != "" && value != teTrailersValue` looks like defensive
			// hygiene and is MEASURABLY WRONG: the reference answers 502 for a
			// present-but-empty `te`.
			name:          "te_empty",
			fields:        []hpack.HeaderField{{Name: "te", Value: ""}},
			wantReject:    true,
			wantMsgSubstr: "not 'trailers'",
		},

		// --- ORDER arms: a leg can be SHADOWED rather than absent, and a
		// presence-only arm cannot tell the two apart. Each asserts WHICH leg
		// fired, by naming the losing leg's message in wantMsgNotSubstr.
		//
		// ⚠️ ONLY TWO OF THESE FOUR ARE ACTUALLY ORDER GUARDS, AND THAT WAS
		// MEASURED AT THE PHASE-92 IMPL RATHER THAN ASSUMED. Moving
		// hasUppercaseHeaderChar to LAST in the switch is a behavioral NO-OP:
		// every sibling leg compares the name against an all-lowercase literal,
		// so a name containing an uppercase character matches NONE of them at
		// ANY position. The NC that permutes the legs leaves the table FULLY
		// GREEN (measured: TEST_RC=0, empty reddened set).
		//
		// => uppercase_beats_connection and uppercase_beats_content_length
		// cannot be reddened by any leg permutation. They are LEG-PRESENCE
		// guards, and against leg presence they add nothing beyond the
		// uppercase_name reject arm. They are RETAINED as executable
		// documentation of the intended precedence, NOT as order guards.
		//
		// The genuine order guards are connection_beats_te and te_beats_dup_cl,
		// which redden when the FIELD loop is reversed (the NC that does work).
		// Do not read this block's green as evidence that leg order is pinned;
		// a test that passes is not thereby a guard.
		{
			name:             "uppercase_beats_connection",
			fields:           []hpack.HeaderField{{Name: "Connection", Value: "keep-alive"}},
			wantReject:       true,
			wantMsgSubstr:    msgLegUppercase + `"Connection"`,
			wantMsgNotSubstr: msgLegConnSpec,
		},
		{
			// Proves the case-sensitive legs below the uppercase leg are
			// unreachable for this input.
			name:             "uppercase_beats_content_length",
			fields:           []hpack.HeaderField{{Name: "Content-Length", Value: "5"}},
			wantReject:       true,
			wantMsgSubstr:    msgLegUppercase + `"Content-Length"`,
			wantMsgNotSubstr: msgLegDupContLen,
		},
		{
			// First offending FIELD in the block wins, not the "worst" one.
			name:             "connection_beats_te",
			fields:           []hpack.HeaderField{{Name: "connection", Value: "x"}, {Name: "te", Value: "gzip"}},
			wantReject:       true,
			wantMsgSubstr:    msgLegConnSpec + `"connection"`,
			wantMsgNotSubstr: msgLegTE,
		},
		{
			name:             "te_beats_dup_cl",
			fields:           []hpack.HeaderField{{Name: "te", Value: "gzip"}, {Name: "content-length", Value: "1"}, {Name: "content-length", Value: "1"}},
			wantReject:       true,
			wantMsgSubstr:    msgLegTE + `"gzip"`,
			wantMsgNotSubstr: msgLegDupContLen,
		},

		// --- PARITY / POSITIVE CONTROLS (must PASS, i.e. return nil).
		{
			// Baseline: a validator rejecting everything reddens here.
			name:   "plain_200",
			fields: []hpack.HeaderField{{Name: "content-type", Value: "text/plain"}},
		},
		{
			// ⚠️ THE SINGLE ARM PROVING THE INVERTED PSEUDO-HEADER LEG REALLY
			// INVERTED. validateResponseTrailers BARS every pseudo-header;
			// `:status` is REQUIRED in a leading block, so this one must not.
			name:   "status_pseudo_passes",
			fields: nil,
		},
		{
			name:   "single_content_length",
			fields: []hpack.HeaderField{{Name: "content-length", Value: "5"}},
		},
		{
			// ⚠️ PINS THAT "arm E" IS OUT OF SCOPE: a content-length whose value
			// disagrees with the body length is NOT rejected here. It is
			// DUPLICATE-NESS, not value-CORRECTNESS, that trips the leg.
			name:   "single_content_length_wrong_value",
			fields: []hpack.HeaderField{{Name: "content-length", Value: "99"}},
		},
		{
			// ⚠️ MEASURED PARITY: the reference FORWARDS `te: trailers`
			// VERBATIM. This arm is what stops a later broadening of the `te`
			// leg to an unconditional reject, which would redden the
			// differential against a CORRECT reference.
			name:   "te_trailers_passes",
			fields: []hpack.HeaderField{{Name: "te", Value: "trailers"}},
		},
		{
			// Reference-parity control: barred by RFC 9110 §6.5.1, FORWARDED by
			// the reference. Rejecting it makes the differential RED on a
			// correct implementation. Do not "complete" the list against RFC 9110.
			name:   "host_passes",
			fields: []hpack.HeaderField{{Name: "host", Value: "x"}},
		},
		{
			// Reference-parity control, same as host_passes.
			name:   "trailer_passes",
			fields: []hpack.HeaderField{{Name: "trailer", Value: "y"}},
		},
		{
			// Optional whitespace around a value, and a present-but-empty value:
			// measured PARITY on BOTH sides.
			name:   "ows_and_empty_value",
			fields: []hpack.HeaderField{{Name: "x-a", Value: " v "}, {Name: "x-b", Value: ""}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := append([]hpack.HeaderField{status200}, tc.fields...)
			got := validateResponseHeaders(respHeaderValidateStreamID, fields)
			if !tc.wantReject {
				if got != nil {
					t.Errorf("validateResponseHeaders = %v, want nil (legal leading block)", got)
				}
				return
			}
			if got == nil {
				t.Errorf("validateResponseHeaders = nil, want a rejection")
				return
			}
			if got.Code != ErrInternalError {
				t.Errorf("Code = %v, want INTERNAL_ERROR", got.Code)
			}
			// A stream-scoped error MUST carry the id. A connection-scoped one
			// carries 0 and would tear the pooled conn down.
			if got.Stream != respHeaderValidateStreamID {
				t.Errorf("Stream = %d, want %d (stream-scoped, not connection-scoped)", got.Stream, respHeaderValidateStreamID)
			}
			// The offending field is asserted in its QUOTED form. An unquoted
			// assertion is unfalsifiable wherever the name also occurs in the
			// message's own fixed prefix.
			if !strings.Contains(got.Error(), tc.wantMsgSubstr) {
				t.Errorf("Error() = %q, want it to name the offending field (substring %q)", got.Error(), tc.wantMsgSubstr)
			}
			if tc.wantMsgNotSubstr != "" && strings.Contains(got.Error(), tc.wantMsgNotSubstr) {
				t.Errorf("Error() = %q, want the SHADOWED leg's message (%q) NOT to appear", got.Error(), tc.wantMsgNotSubstr)
			}
			// The SENTINEL — not the code — is what a caller must discriminate
			// on; the code is shared with a peer RST_STREAM(INTERNAL_ERROR) and
			// with a malformed-TRAILERS reject. See
			// TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject.
			if !errors.Is(got, ErrMalformedResponseHeaders) {
				t.Errorf("errors.Is(got, ErrMalformedResponseHeaders) = false for %v, want true", got)
			}
			// ...and it must NOT be the trailer sentinel: router_h2.go selects a
			// DIFFERENT arm (Status 0, a downstream reset) on that one.
			if errors.Is(got, ErrMalformedTrailers) {
				t.Errorf("errors.Is(got, ErrMalformedTrailers) = true for %v, want false", got)
			}
		})
	}
}

// TestClientConn_RoundTrip_ResponseHeaderValidation_Wire is TABLE B: cases
// driven through a real ClientConn/peer conn pair. THIS IS THE LIVENESS GATE —
// it is the only table that exercises the capture-site call into
// validateResponseHeaders, so neutering the call site (validator left intact)
// reddens this table while Table A stays FULLY GREEN. That asymmetry is the
// whole point of the split.
//
// Each reject arm asserts five independent properties:
//  1. RoundTrip returns a non-nil error (never a "cleaned" 200),
//  2. carried as a stream-scoped *Error with code INTERNAL_ERROR wrapping
//     ErrMalformedResponseHeaders (and NOT ErrMalformedTrailers),
//  3. the rejection is NOT a hang-to-timeout,
//  4. the message names the offending field, and
//  5. the CONN SURVIVES — the reference resets the STREAM, not the connection,
//     so nothing here may tear down the pooled upstream conn.
//
// ⚠️ dialClientConnTCP, NOT dialClientConn. The leading-block reject writes
// RST_STREAM from the readLoop goroutine while the peer may be mid-write of its
// DATA frame. net.Pipe is synchronous and unbuffered, so that is a GUARANTEED
// deadlock (a SPEC-stage probe over net.Pipe deadlocked and was killed at
// 120 s). Kernel socket buffers absorb both writes.
func TestClientConn_RoundTrip_ResponseHeaderValidation_Wire(t *testing.T) {
	tests := []struct {
		name string
		// extra fields follow `:status: 200` in the LEADING block.
		extra         []hpack.HeaderField
		wantErrSubstr string // "" = expect success
	}{
		{
			name:  "success_capture",
			extra: []hpack.HeaderField{{Name: "content-type", Value: "text/plain"}},
		},
		{
			name:          "connection_specific",
			extra:         []hpack.HeaderField{{Name: "connection", Value: "keep-alive"}},
			wantErrSubstr: `"connection"`,
		},
		{
			name:          "transfer_encoding",
			extra:         []hpack.HeaderField{{Name: "transfer-encoding", Value: "chunked"}},
			wantErrSubstr: `"transfer-encoding"`,
		},
		{
			name:          "uppercase_field_name",
			extra:         []hpack.HeaderField{{Name: "X-Upper-Case", Value: "yes"}},
			wantErrSubstr: `"X-Upper-Case"`,
		},
		{
			name:          "duplicate_content_length",
			extra:         []hpack.HeaderField{{Name: "content-length", Value: "5"}, {Name: "content-length", Value: "5"}},
			wantErrSubstr: "duplicate content-length",
		},
		{
			name:          "te_gzip",
			extra:         []hpack.HeaderField{{Name: "te", Value: "gzip"}},
			wantErrSubstr: "not 'trailers'",
		},
		{
			name:          "te_empty",
			extra:         []hpack.HeaderField{{Name: "te", Value: ""}},
			wantErrSubstr: "not 'trailers'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc, peer, cleanup := dialClientConnTCP(t)
			defer cleanup()

			peerDone := make(chan error, 1)
			// ⚠️ THE PEER RUNS IN ITS OWN GOROUTINE: a synchronous peer blocks
			// the read loop.
			go func() {
				hf, _, err := peer.readRequestHeaders()
				if err != nil {
					peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
					return
				}
				// A body is written so END_STREAM lands on DATA, not on the
				// leading HEADERS: the reject therefore fires while the peer
				// still believes the stream is open, which is the state in
				// which a missing markReset takes the CONNECTION down.
				if werr := peer.writeResponse(hf.StreamID, 200, tc.extra, []byte("hello")); werr != nil {
					peerDone <- fmt.Errorf("writeResponse: %w", werr)
					return
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

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, err := cc.RoundTrip(ctx, H2Request{
				Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
			})

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
					t.Errorf("RoundTrip = %v, want nil (legal leading block)", err)
					return
				}
				if resp.Status != 200 {
					t.Errorf("Status = %d, want 200", resp.Status)
				}
				if string(resp.Body) != "hello" {
					t.Errorf("Body = %q, want %q", resp.Body, "hello")
				}
				return
			}

			// (1) a rejection, never a cleaned 200.
			if err == nil {
				t.Errorf("RoundTrip = nil error, want a rejection (status=%d headers=%+v)", resp.Status, resp.Headers)
				return
			}
			if resp.Status != 0 || len(resp.Headers) != 0 {
				t.Errorf("rejected RoundTrip returned a response: status=%d headers=%+v, want the zero H2Response", resp.Status, resp.Headers)
			}
			// (3) not a hang-to-timeout. Checked BEFORE the code/message
			// assertions so a hang reports as a hang, not as a wrong code.
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
			if !errors.Is(err, ErrMalformedResponseHeaders) {
				t.Errorf("errors.Is(err, ErrMalformedResponseHeaders) = false for %v, want true", err)
			}
			if errors.Is(err, ErrMalformedTrailers) {
				t.Errorf("errors.Is(err, ErrMalformedTrailers) = true for %v, want false — router_h2.go selects a DIFFERENT arm on that sentinel", err)
			}
			// (4) the message names the offending field.
			if !strings.Contains(err.Error(), tc.wantErrSubstr) {
				t.Errorf("error = %q, want it to name the offending field (substring %q)", err.Error(), tc.wantErrSubstr)
			}
			// (5) the conn survives: reset the stream, not the connection.
			if cc.Closed() {
				t.Errorf("cc.Closed() = true after a rejected leading block; the reference resets the STREAM, not the conn")
			}
		})
	}
}

// TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject is the
// NEGATIVE CONTROL for ErrMalformedResponseHeaders, and the reason a SECOND
// sentinel exists at all.
//
// THREE outcomes all finish the stream with an *Error whose Code is
// INTERNAL_ERROR: a peer RST_STREAM(INTERNAL_ERROR), a malformed LEADING block,
// and a malformed TRAILING block. router_h2.go routes them to three DIFFERENT
// arms (peer reset -> 502 via the generic path, leading -> 502 via the new arm,
// trailers -> Status 0, a downstream stream reset), and the only thing it can
// route on is the sentinel.
//
// ⚠️ A TWO-WAY VERSION OF THIS TEST IS VACUOUS. Sub-test 3 is what makes the
// pair non-vacuous: without it, a validator returning ErrMalformedResponseHeaders
// for TRAILERS too would pass. Every arm is driven over the wire so neither side
// is a hand-built claim about what the codec produces.
func TestMalformedResponseHeaders_SentinelDiscriminatesTrailerReject(t *testing.T) {
	t.Run("peer_reset_is_NEITHER", func(t *testing.T) {
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
			t.Errorf("peer: %v", perr)
			return
		}
		if err == nil {
			t.Errorf("RoundTrip = nil, want the peer reset surfaced as an error")
			return
		}
		var h2err *Error
		if !errors.As(err, &h2err) {
			t.Errorf("error is %T (%v), want *h2.Error", err, err)
		} else if h2err.Code != ErrInternalError {
			// ⚠️ ASSERTING THE CODE EXPLICITLY IS WHAT PROVES THE CODE IS A
			// NON-DISCRIMINATOR: all three outcomes carry INTERNAL_ERROR.
			// Without this the sentinels' whole rationale is unproven.
			t.Errorf("Code = %v, want INTERNAL_ERROR (the code BOTH malformed-block rejects also carry)", h2err.Code)
		}
		if errors.Is(err, ErrMalformedResponseHeaders) {
			t.Errorf("errors.Is(peerResetErr, ErrMalformedResponseHeaders) = true for %v, want false", err)
		}
		if errors.Is(err, ErrMalformedTrailers) {
			t.Errorf("errors.Is(peerResetErr, ErrMalformedTrailers) = true for %v, want false", err)
		}
	})

	t.Run("malformed_leading_IS_response_headers_NOT_trailers", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
				return
			}
			if werr := peer.writeResponse(hf.StreamID, 200,
				[]hpack.HeaderField{{Name: "connection", Value: "keep-alive"}}, []byte("hello")); werr != nil {
				peerDone <- fmt.Errorf("writeResponse: %w", werr)
				return
			}
			peerDone <- nil
			for {
				if _, rerr := peer.readNextFrame(); rerr != nil {
					return
				}
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if perr := <-peerDone; perr != nil {
			t.Errorf("peer: %v", perr)
			return
		}
		if err == nil {
			t.Errorf("RoundTrip = nil, want a malformed-response-headers rejection")
			return
		}
		if !errors.Is(err, ErrMalformedResponseHeaders) {
			t.Errorf("errors.Is(err, ErrMalformedResponseHeaders) = false for %v, want true", err)
		}
		if errors.Is(err, ErrMalformedTrailers) {
			t.Errorf("errors.Is(err, ErrMalformedTrailers) = true for %v, want false", err)
		}
	})

	// ⚠️ THIS THIRD SUB-TEST IS WHAT MAKES THE PAIR NON-VACUOUS. Without it, a
	// validator handing ErrMalformedResponseHeaders to the TRAILER path too
	// would pass every other assertion in this file.
	t.Run("malformed_trailers_IS_trailers_NOT_response_headers", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- fmt.Errorf("readRequestHeaders: %w", err)
				return
			}
			// A LEGAL leading block, then an ILLEGAL trailing one.
			if werr := peer.writeScriptedTrailers(hf.StreamID, 200, []byte("hello"),
				[]hpack.HeaderField{{Name: "content-length", Value: "5"}}, true); werr != nil {
				peerDone <- fmt.Errorf("writeScriptedTrailers: %w", werr)
				return
			}
			peerDone <- nil
			for {
				if _, rerr := peer.readNextFrame(); rerr != nil {
					return
				}
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if perr := <-peerDone; perr != nil {
			t.Errorf("peer: %v", perr)
			return
		}
		if err == nil {
			t.Errorf("RoundTrip = nil, want a malformed-trailers rejection")
			return
		}
		if !errors.Is(err, ErrMalformedTrailers) {
			t.Errorf("errors.Is(err, ErrMalformedTrailers) = false for %v, want true", err)
		}
		if errors.Is(err, ErrMalformedResponseHeaders) {
			t.Errorf("errors.Is(err, ErrMalformedResponseHeaders) = true for %v, want false — the LEADING-block sentinel must not leak onto the trailer path", err)
		}
	})
}
