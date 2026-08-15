package h2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// ---------------------------------------------------------------------------
// phase-88 (h2-continuation-frames): CONTINUATION reassembly arms.
//
// These arms are written FIRST, TDD-style, against the UNFIXED tree. At the
// tip ServerConn.dispatchFrame DISCARDS every *http2.ContinuationFrame
// (conn.go, the `case *http2.ContinuationFrame:` arm) and ClientConn.
// dispatchFrame has no CONTINUATION case at all, so every field encoded at or
// after the frame split point is SILENTLY LOST and the connection HPACK
// decoder is left mid-block (desyncing every LATER request on the same
// connection).
//
// ⚠️ THE CONTROL VARIABLE IS THE HPACK-ENCODED BLOCK SIZE, NOT A HEADER BYTE
// COUNT (PLAN-88 §1.1). Every arm below therefore splits the block at an
// EXPLICIT, MEASURED byte offset produced by the encoder itself
// (contClientEnc.split) rather than by choosing a payload size and hoping
// Huffman coding lands the split where the arm needs it.
//
// House style (PLAN-88 §5.4): errors are asserted by Code / Stream /
// strings.Contains, never by exact-message equality.
// ---------------------------------------------------------------------------

// contDefaultBound mirrors the production `maxHeaderBlockSize` constant the
// phase-88 IMPL lands in conn.go (16 MiB). It is DELIBERATELY a separate
// test-local constant: this file must COMPILE against the unfixed tree so the
// behavioral RED census is observable, and referencing a symbol that does not
// exist yet would turn every arm here into a build failure instead.
const contDefaultBound = 16 << 20

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// contClientEnc is a CONNECTION-SCOPED HPACK encoder for the test client side.
//
// ⚠️ ONE ENCODER PER CONNECTION IS LOAD-BEARING, NOT TIDINESS. The dynamic
// table is connection state shared by every header block on that connection;
// arm D (split request, then an ordinary request) only exposes the tip's
// decoder desync because the SECOND request's block is encoded against the
// table state the FIRST request's block established.
type contClientEnc struct {
	buf bytes.Buffer
	enc *hpack.Encoder
}

func newContClientEnc() *contClientEnc {
	c := &contClientEnc{}
	c.enc = hpack.NewEncoder(&c.buf)
	return c
}

// split encodes head then tail through the SAME encoder and returns the whole
// block plus the byte offset that separates them. Splitting at this offset
// guarantees the CONTINUATION carries exactly the tail fields — the arm's
// question ("does a field encoded after the split arrive?") is then asked
// directly instead of being inferred from a payload size.
func (c *contClientEnc) split(head, tail []hpack.HeaderField) ([]byte, int) {
	c.buf.Reset()
	for _, h := range head {
		_ = c.enc.WriteField(h)
	}
	splitAt := c.buf.Len()
	for _, h := range tail {
		_ = c.enc.WriteField(h)
	}
	out := make([]byte, c.buf.Len())
	copy(out, c.buf.Bytes())
	return out, splitAt
}

// encode encodes fields through the same connection encoder (no split).
func (c *contClientEnc) encode(fields []hpack.HeaderField) []byte {
	block, _ := c.split(fields, nil)
	return block
}

// contWriteSplit writes block as HEADERS(block[:splitAt], no END_HEADERS)
// followed by exactly one CONTINUATION(block[splitAt:], END_HEADERS).
// prio is written only when non-zero (x/net sets the PRIORITY flag iff the
// PriorityParam is non-zero).
func contWriteSplit(t *testing.T, fr *http2.Framer, streamID uint32, block []byte, splitAt int, endStream bool, prio http2.PriorityParam) {
	t.Helper()
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block[:splitAt],
		EndStream:     endStream,
		EndHeaders:    false,
		Priority:      prio,
	}); err != nil {
		t.Fatalf("write split HEADERS: %v", err)
	}
	if err := fr.WriteContinuation(streamID, true, block[splitAt:]); err != nil {
		t.Fatalf("write CONTINUATION: %v", err)
	}
}

// contWriteHopped writes block as HEADERS + CONTINUATION frames each at most
// hop bytes, i.e. the way a conforming peer sends a block larger than
// SETTINGS_MAX_FRAME_SIZE. Distinct from contWriteSplit, which deliberately
// makes exactly two frames to exercise the minimal split.
func contWriteHopped(t *testing.T, fr *http2.Framer, streamID uint32, block []byte, hop int, endStream bool) {
	t.Helper()
	head := block
	if len(head) > hop {
		head = block[:hop]
	}
	rest := block[len(head):]
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: head,
		EndStream:     endStream,
		EndHeaders:    len(rest) == 0,
	}); err != nil {
		t.Fatalf("write hopped HEADERS: %v", err)
	}
	for len(rest) > 0 {
		n := len(rest)
		if n > hop {
			n = hop
		}
		chunk := rest[:n]
		rest = rest[n:]
		if err := fr.WriteContinuation(streamID, len(rest) == 0, chunk); err != nil {
			t.Fatalf("write hopped CONTINUATION: %v", err)
		}
	}
}

// contWriteWhole writes block as a single HEADERS frame with END_HEADERS.
func contWriteWhole(t *testing.T, fr *http2.Framer, streamID uint32, block []byte, endStream bool) {
	t.Helper()
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block,
		EndStream:     endStream,
		EndHeaders:    true,
	}); err != nil {
		t.Fatalf("write HEADERS: %v", err)
	}
}

// contGetHead is the pseudo-header prefix of a well-formed bodyless GET.
func contGetHead(path string) []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":method", Value: "GET"},
		{Name: ":path", Value: path},
		{Name: ":scheme", Value: "http"},
		{Name: ":authority", Value: "test.local"},
	}
}

// contObserved is what contReadFrames accumulates off the wire.
type contObserved struct {
	statuses     map[uint32]int
	rsts         map[uint32]http2.ErrCode
	goawayCode   http2.ErrCode
	goawayLastID uint32
	sawGoaway    bool
	nRespHeaders int
	nData        int
	readErr      error
}

// contReadFrames reads response frames until stop reports satisfied, the read
// budget expires, or the connection errors. Every HEADERS block seen is fed to
// a single decoder so the reader's HPACK state tracks the server's encoder.
func contReadFrames(t *testing.T, clientConn net.Conn, fr *http2.Framer, budget time.Duration, stop func(*contObserved) bool) *contObserved {
	t.Helper()
	obs := &contObserved{statuses: map[uint32]int{}, rsts: map[uint32]http2.ErrCode{}}
	var fields []hpack.HeaderField
	dec := hpack.NewDecoder(4096, func(hf hpack.HeaderField) { fields = append(fields, hf) })
	deadline := time.Now().Add(budget)
	for {
		if stop != nil && stop(obs) {
			return obs
		}
		if !time.Now().Before(deadline) {
			return obs
		}
		_ = clientConn.SetReadDeadline(deadline)
		f, err := fr.ReadFrame()
		if err != nil {
			obs.readErr = err
			return obs
		}
		switch tf := f.(type) {
		case *http2.HeadersFrame:
			obs.nRespHeaders++
			fields = fields[:0]
			if _, derr := dec.Write(tf.HeaderBlockFragment()); derr == nil && tf.HeadersEnded() {
				_ = dec.Close()
			}
			for _, hf := range fields {
				if hf.Name == ":status" {
					s, _ := strconv.Atoi(hf.Value)
					obs.statuses[tf.StreamID] = s
				}
			}
		case *http2.DataFrame:
			obs.nData++
		case *http2.RSTStreamFrame:
			obs.rsts[tf.StreamID] = tf.ErrCode
		case *http2.GoAwayFrame:
			if !obs.sawGoaway {
				obs.sawGoaway = true
				obs.goawayCode = tf.ErrCode
				obs.goawayLastID = tf.LastStreamID
			}
		}
	}
}

// contCaptureDispatcher is both the Dispatcher and the Action: it records every
// H2Request it is handed (so an arm can assert that a CONTINUATION-carried
// field ARRIVED, which is the property the tip loses silently) and answers 200.
type contCaptureDispatcher struct {
	mu   sync.Mutex
	reqs []H2Request
	ch   chan struct{}
}

func newContCaptureDispatcher() *contCaptureDispatcher {
	return &contCaptureDispatcher{ch: make(chan struct{}, 16)}
}

func (d *contCaptureDispatcher) Match(_ *http.Request) (Action, bool) { return d, true }

func (d *contCaptureDispatcher) WriteH2(_ context.Context, req H2Request, sw StreamWriter) error {
	d.mu.Lock()
	d.reqs = append(d.reqs, req)
	d.mu.Unlock()
	select {
	case d.ch <- struct{}{}:
	default:
	}
	body := []byte("OK")
	if err := sw.WriteHeaders([]hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
	}, false); err != nil {
		return err
	}
	return sw.WriteData(body, true)
}

// waitReqs blocks (bounded) until at least n requests have been dispatched.
func (d *contCaptureDispatcher) waitReqs(n int, budget time.Duration) []H2Request {
	deadline := time.Now().Add(budget)
	for {
		d.mu.Lock()
		got := len(d.reqs)
		snap := make([]H2Request, got)
		copy(snap, d.reqs)
		d.mu.Unlock()
		if got >= n || !time.Now().Before(deadline) {
			return snap
		}
		select {
		case <-d.ch:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// contHasHeader reports whether req carries name=value.
func contHasHeader(req H2Request, name, value string) bool {
	for _, h := range req.Headers {
		if h.Name == name && h.Value == value {
			return true
		}
	}
	return false
}

// contStartServer wires a capture dispatcher onto a fresh ServerConn and drives
// the handshake, returning the client-side conn, framer, encoder and captor.
func contStartServer(t *testing.T) (net.Conn, *http2.Framer, *contClientEnc, *contCaptureDispatcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d := newContCaptureDispatcher()
	clientConn, _ := startServerConn(t, ctx, d, DefaultServerSettings)
	fr := http2.NewFramer(clientConn, clientConn)
	fr.SetMaxReadFrameSize(1<<24 - 1)
	driveHandshake(t, clientConn, fr)
	return clientConn, fr, newContClientEnc(), d
}

// ---------------------------------------------------------------------------
// Arm C (+ its C0 control): a header carried in the CONTINUATION must ARRIVE.
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_HeaderInContinuationArrives is arm C with its C0
// control. The control is what makes the arm non-vacuous: it proves the very
// same field, in the very same encoder, DOES reach the dispatched request when
// the block is not split — so a red arm C is the SPLIT losing it, not the
// harness failing to observe headers at all.
//
// RED at the tip: arm C reads back zero `x-probe` fields (PLAN-88 §1.2 arm C:
// "NHDR 0 — SILENTLY GONE, status 200").
func TestServerConn_Continuation_HeaderInContinuationArrives(t *testing.T) {
	t.Run("C0_control_unsplit", func(t *testing.T) {
		clientConn, fr, enc, d := contStartServer(t)
		block := enc.encode(append(contGetHead("/probe"),
			hpack.HeaderField{Name: "x-probe", Value: "probe-value"}))
		contWriteWhole(t, fr, 1, block, true)

		reqs := d.waitReqs(1, 3*time.Second)
		if len(reqs) != 1 {
			t.Fatalf("dispatched requests = %d, want 1 (control must be green on BOTH sides of the fix)", len(reqs))
		}
		if !contHasHeader(reqs[0], "x-probe", "probe-value") {
			t.Errorf("control: dispatched request headers = %v, want x-probe=probe-value", reqs[0].Headers)
		}
		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool { return o.nData > 0 })
		if obs.statuses[1] != 200 {
			t.Errorf("control: status on stream 1 = %d, want 200", obs.statuses[1])
		}
	})

	t.Run("C_header_in_continuation", func(t *testing.T) {
		clientConn, fr, enc, d := contStartServer(t)
		block, splitAt := enc.split(
			contGetHead("/probe"),
			[]hpack.HeaderField{{Name: "x-probe", Value: "probe-value"}},
		)
		contWriteSplit(t, fr, 1, block, splitAt, true, http2.PriorityParam{})

		reqs := d.waitReqs(1, 3*time.Second)
		if len(reqs) != 1 {
			t.Fatalf("dispatched requests = %d, want 1", len(reqs))
		}
		if !contHasHeader(reqs[0], "x-probe", "probe-value") {
			t.Errorf("dispatched request headers = %v, want the CONTINUATION-carried x-probe=probe-value to have ARRIVED", reqs[0].Headers)
		}
		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool { return o.nData > 0 })
		if obs.statuses[1] != 200 {
			t.Errorf("status on stream 1 = %d, want 200", obs.statuses[1])
		}
		if code, ok := obs.rsts[1]; ok {
			t.Errorf("unexpected RST_STREAM(%v) on stream 1", code)
		}
	})
}

// ---------------------------------------------------------------------------
// Arm B: a REQUIRED pseudo-header carried in the CONTINUATION.
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_PathInContinuationDispatches is arm B. `:path`
// is encoded AFTER the split point, so at the tip the reassembled request is
// missing a required pseudo-header and buildRequest rejects it — measured at
// the tip as RST_STREAM PROTOCOL_ERROR(1) (PLAN-88 §1.2 arm B).
//
// This is the arm that distinguishes "a header went missing" from "the request
// was rejected outright": after the fix the request must dispatch normally with
// Path == "/marked".
func TestServerConn_Continuation_PathInContinuationDispatches(t *testing.T) {
	clientConn, fr, enc, d := contStartServer(t)
	// All four fields are pseudo-headers, so the RFC 9113 §8.3 ordering rule
	// (pseudo before regular) is satisfied whichever side of the split they
	// land on.
	block, splitAt := enc.split(
		[]hpack.HeaderField{
			{Name: ":method", Value: "GET"},
			{Name: ":scheme", Value: "http"},
			{Name: ":authority", Value: "test.local"},
		},
		[]hpack.HeaderField{{Name: ":path", Value: "/marked"}},
	)
	contWriteSplit(t, fr, 1, block, splitAt, true, http2.PriorityParam{})

	reqs := d.waitReqs(1, 3*time.Second)
	if len(reqs) != 1 {
		t.Errorf("dispatched requests = %d, want 1 (the CONTINUATION-carried :path must complete the request)", len(reqs))
	} else if reqs[0].Path != "/marked" {
		t.Errorf("dispatched Path = %q, want %q", reqs[0].Path, "/marked")
	}

	obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool {
		return o.nData > 0 || len(o.rsts) > 0 || o.sawGoaway
	})
	if code, ok := obs.rsts[1]; ok {
		t.Errorf("RST_STREAM(%v) on stream 1; want a normal 200-class dispatch", code)
	}
	if obs.sawGoaway {
		t.Errorf("GOAWAY(%v) emitted; want a normal 200-class dispatch", obs.goawayCode)
	}
	if obs.statuses[1] != 200 {
		t.Errorf("status on stream 1 = %d, want 200", obs.statuses[1])
	}
}

// ---------------------------------------------------------------------------
// Arm D: the HPACK dynamic-table desync pin.
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_SplitThenUnsplitOnSameConn is arm D — the pin
// that a discarded CONTINUATION does not merely lose ITS OWN fields but
// POISONS THE CONNECTION. The tip feeds the first fragment to the shared
// decoder and never closes the block, so the second (ordinary, un-split)
// request on the same connection decodes against a desynced dynamic table:
// measured at the tip as `GOAWAY LastStreamID=1 COMPRESSION_ERROR(9)` then EOF
// (PLAN-88 §1.2 arm D).
func TestServerConn_Continuation_SplitThenUnsplitOnSameConn(t *testing.T) {
	clientConn, fr, enc, d := contStartServer(t)

	// Request 1: split, with a regular header in the CONTINUATION.
	b1, splitAt := enc.split(
		contGetHead("/first"),
		[]hpack.HeaderField{{Name: "x-first", Value: "one"}},
	)
	contWriteSplit(t, fr, 1, b1, splitAt, true, http2.PriorityParam{})

	// Request 2: ORDINARY, un-split, encoded through the same connection
	// encoder (so it depends on the dynamic-table state request 1 established).
	b2 := enc.encode(append(contGetHead("/second"),
		hpack.HeaderField{Name: "x-second", Value: "two"}))
	contWriteWhole(t, fr, 3, b2, true)

	// Read the wire FIRST (it is also what gives the dispatch goroutines time to
	// run), so a connection torn down by the desync is reported as such rather
	// than only as a missing dispatch.
	obs := contReadFrames(t, clientConn, fr, 4*time.Second, func(o *contObserved) bool {
		return o.sawGoaway || o.nData >= 2 || len(o.rsts) > 0
	})
	reqs := d.waitReqs(2, time.Second)
	if len(reqs) != 2 {
		t.Errorf("dispatched requests = %d, want 2 (both requests on the SAME connection must succeed); rst=%v goaway=%v readErr=%v",
			len(reqs), obs.rsts, obs.sawGoaway, obs.readErr)
	}
	var sawFirst, sawSecond bool
	for _, r := range reqs {
		if r.Path == "/first" && contHasHeader(r, "x-first", "one") {
			sawFirst = true
		}
		if r.Path == "/second" && contHasHeader(r, "x-second", "two") {
			sawSecond = true
		}
	}
	if !sawFirst {
		t.Errorf("request 1 (split) did not arrive intact; dispatched = %+v", reqs)
	}
	if !sawSecond {
		t.Errorf("request 2 (ordinary) did not arrive intact; dispatched = %+v", reqs)
	}
	if obs.sawGoaway {
		t.Errorf("GOAWAY(%v) LastStreamID=%d emitted; a split block must not desync the connection HPACK decoder",
			obs.goawayCode, obs.goawayLastID)
	}
	if len(obs.rsts) != 0 {
		t.Errorf("RST_STREAM frames = %v, want none", obs.rsts)
	}
	if obs.statuses[1] != 200 {
		t.Errorf("status on stream 1 = %d, want 200", obs.statuses[1])
	}
	if obs.statuses[3] != 200 {
		t.Errorf("status on stream 3 = %d, want 200", obs.statuses[3])
	}
}

// ---------------------------------------------------------------------------
// Arm E: the GOAWAY pin (PLAN-88 §4.4 / D-88-LASTID).
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_ArmE_GoawayPin pins BOTH wire-visible properties
// of the arm-E sequence in ONE test, with t.Errorf per property so neither can
// hide behind the other:
//
//  1. the GOAWAY's LastStreamID is 0 — at the tip the HEADERS frame has ALREADY
//     been processed (s.lastInID = 1) by the time x/net's checkFrameOrder
//     rejects the interleaved frame, so the tip advertises stream 1 as
//     "processed" when nothing about it was ever completed. With the fix the
//     block is still being accumulated and no stream has been admitted, so the
//     correct advertisement is 0 (MEASURED, PLAN-88 §4.4: TIP 1 / PROTOTYPE 0);
//  2. NO response frames follow — at the tip the truncated request is still
//     proxied and answered 200 AFTER the GOAWAY (PLAN-88 §1.4(6)).
//
// The connection error itself originates INSIDE x/net (checkFrameOrder rejects
// a PING interleaved into an open header block); envoy-go never sees the frame.
// The arm is therefore about what envoy-go had already COMMITTED to before that
// rejection, which is exactly what the accumulator changes.
func TestServerConn_Continuation_ArmE_GoawayPin(t *testing.T) {
	clientConn, fr, enc, _ := contStartServer(t)

	// A COMPLETE, well-formed GET in the HEADERS fragment (so the tip has
	// everything it needs to dispatch), deliberately NOT terminated by
	// END_HEADERS.
	block := enc.encode(contGetHead("/marked"))
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: block,
		EndStream:     true,
		EndHeaders:    false,
	}); err != nil {
		t.Fatalf("write HEADERS: %v", err)
	}
	// Interleave a non-CONTINUATION frame — x/net rejects this with
	// ConnectionError(PROTOCOL_ERROR), which envoy-go surfaces as a GOAWAY.
	if err := fr.WritePing(false, [8]byte{'a', 'r', 'm', 'e', 'p', 'i', 'n', '!'}); err != nil {
		t.Fatalf("write PING: %v", err)
	}

	// Read to the end of the connection: the server drains for ~500ms after
	// the GOAWAY before closing, which is ample time for a (wrongly) dispatched
	// response to appear. Reading to EOF is what makes property 2 falsifiable —
	// stopping at the GOAWAY would make "no response frames follow" vacuous.
	obs := contReadFrames(t, clientConn, fr, 4*time.Second, nil)

	if !obs.sawGoaway {
		t.Fatalf("no GOAWAY observed (readErr=%v); arm E cannot be evaluated", obs.readErr)
	}
	if obs.goawayLastID != 0 {
		t.Errorf("GOAWAY LastStreamID = %d, want 0 (no stream was admitted: the header block never completed)", obs.goawayLastID)
	}
	if obs.nRespHeaders != 0 || obs.nData != 0 {
		t.Errorf("response frames after GOAWAY: HEADERS=%d DATA=%d, want 0/0 (a truncated request must never be proxied)",
			obs.nRespHeaders, obs.nData)
	}
	if len(obs.statuses) != 0 {
		t.Errorf("response statuses = %v, want none", obs.statuses)
	}
}

// ---------------------------------------------------------------------------
// The over-bound / flood arm — REAL coverage (PLAN-88 §4.1, D-88-BOUND).
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_FloodExceedsBound drives more CONTINUATION bytes
// than `maxHeaderBlockSize` (16 MiB) and requires the connection to be torn
// down with GOAWAY ENHANCE_YOUR_CALM(0xb).
//
// ⚠️ THIS IS NOT DEFENSE IN DEPTH — IT IS THE MITIGATION FOR A HAZARD THE FIX
// ITSELF CREATES (PLAN-88 §4.1). The TIP is not vulnerable precisely because it
// RETAINS NOTHING; the accumulator this row introduces is what makes an
// unbounded CONTINUATION flood a memory-growth vector. Nothing upstream binds:
// x/net's maxHeaderListSize lives inside readMetaFrame, which this codec never
// enables, so envoy-go does not inherit the CVE-2023-45288 mitigation, and
// SETTINGS_MAX_HEADER_LIST_SIZE is never advertised. checkFrameOrder caps the
// HOP (one frame) and imposes no limit on the NUMBER of CONTINUATION frames.
//
// MEASURED at the tip: the flood is absorbed silently and the request is still
// proxied — so this test hangs to its read budget and fails on the missing
// GOAWAY.
//
// The tail fragments are zero bytes on purpose: the bound must be enforced
// BEFORE (and therefore without) decoding, so their HPACK validity is
// irrelevant to a correct implementation, and generating 16 MiB of real HPACK
// would cost wall-clock the arm does not need.
func TestServerConn_Continuation_FloodExceedsBound(t *testing.T) {
	clientConn, fr, enc, _ := contStartServer(t)

	// The frame size the server ADVERTISED is what it will accept per hop.
	hop := int(DefaultServerSettings.MaxFrameSize) // 16384
	nCont := contDefaultBound/hop + 2              // strictly over 16 MiB
	pad := make([]byte, hop)

	head := enc.encode(contGetHead("/flood"))
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: head,
		EndStream:     true,
		EndHeaders:    false,
	}); err != nil {
		t.Fatalf("write HEADERS: %v", err)
	}
	start := time.Now()
	for i := 0; i < nCont; i++ {
		// Never END_HEADERS: the block stays open for the whole flood.
		if err := fr.WriteContinuation(1, false, pad); err != nil {
			// A write failure here is itself the fix working (the server closed
			// on us); fall through to the frame assertions rather than aborting.
			t.Logf("CONTINUATION %d/%d write stopped: %v", i, nCont, err)
			break
		}
	}
	wrote := time.Since(start)

	obs := contReadFrames(t, clientConn, fr, 5*time.Second, func(o *contObserved) bool { return o.sawGoaway })
	t.Logf("flood arm: %d CONTINUATION frames of %d bytes (%d bytes accumulated), write=%v total=%v",
		nCont, hop, len(head)+nCont*hop, wrote, time.Since(start))

	if !obs.sawGoaway {
		t.Errorf("no GOAWAY observed after %d accumulated header-block bytes (bound = %d); readErr=%v",
			len(head)+nCont*hop, contDefaultBound, obs.readErr)
		return
	}
	if obs.goawayCode != http2.ErrCode(ErrEnhanceYourCalm) {
		t.Errorf("GOAWAY code = %v (0x%x), want ENHANCE_YOUR_CALM (0xb)", obs.goawayCode, uint32(obs.goawayCode))
	}
}

// ---------------------------------------------------------------------------
// DEFENSE IN DEPTH — explicitly labeled, NOT counted as coverage.
// ---------------------------------------------------------------------------

// contIllegalFrameReader returns a Framer positioned over frames produced by
// write, with AllowIllegalReads set.
//
// ⚠️ AllowIllegalReads IS THE ISOLATING MECHANISM AND THE WHOLE POINT
// (PLAN-88 §4.1): with it set, the byte-identical inputs below are delivered as
// real *http2.ContinuationFrame values with no error, which PROVES
// Framer.checkFrameOrder is the sole gate that makes the two guards below
// unreachable in production. envoy-go NEVER sets this field; its only three
// frame producers are ReadFrame calls (settings.go, framer.go x2).
//
// ⚠️ Frames returned by ReadFrame alias an internal read buffer that the NEXT
// ReadFrame invalidates, so callers must dispatch each frame before reading the
// next — exactly as the production frame loop does.
func contIllegalFrameReader(t *testing.T, write func(fr *http2.Framer) error) *http2.Framer {
	t.Helper()
	var buf bytes.Buffer
	w := http2.NewFramer(&buf, nil)
	if err := write(w); err != nil {
		t.Fatalf("produce frames: %v", err)
	}
	r := http2.NewFramer(io.Discard, bytes.NewReader(buf.Bytes()))
	r.AllowIllegalReads = true
	r.SetMaxReadFrameSize(1<<24 - 1)
	return r
}

// contIdleServerConn builds a ServerConn that is NEVER Run: the arms below call
// dispatchFrame directly because the frames they need cannot be obtained
// through ReadFrame at all.
func contIdleServerConn(t *testing.T) *ServerConn {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewServerConn(ctx, a, newContCaptureDispatcher(), DefaultServerSettings)
}

// contAssertConnError asserts the house-style properties of a connection-scoped
// *Error: the code, Stream == 0 (a stream-scoped error would reset one stream
// instead of tearing the connection down), and a message substring. Exact
// message equality is deliberately never asserted.
func contAssertConnError(t *testing.T, err error, wantCode ErrCode, wantSubstr string) {
	t.Helper()
	if err == nil {
		t.Errorf("error = nil, want a connection-scoped %v naming %q", wantCode, wantSubstr)
		return
	}
	var h2err *Error
	if !errors.As(err, &h2err) {
		t.Errorf("error = %v (%T), want an *h2.Error", err, err)
		return
	}
	if h2err.Code != wantCode {
		t.Errorf("Code = %v, want %v", h2err.Code, wantCode)
	}
	if h2err.Stream != 0 {
		t.Errorf("Stream = %d, want 0 (connection-scoped)", h2err.Stream)
	}
	if !strings.Contains(h2err.Error(), wantSubstr) {
		t.Errorf("Error() = %q, want it to contain %q", h2err.Error(), wantSubstr)
	}
	if !strings.HasPrefix(h2err.Error(), "h2: ") {
		t.Errorf("Error() = %q, want the package-wide %q prefix", h2err.Error(), "h2: ")
	}
}

// TestServerConn_Continuation_UnexpectedContinuation_DefenseInDepth covers a
// CONTINUATION arriving with NO accumulator open.
//
// ⚠️ DEFENSE IN DEPTH — NOT COUNTED AS COVERAGE (PLAN-88 §4.1 / §5.2). This
// state is UNREACHABLE through ReadFrame: x/net's Framer.checkFrameOrder
// rejects the byte sequence first with the connection error
//
//	"unexpected CONTINUATION for stream 1"
//
// (frame.go, the `else if fh.Type == FrameContinuation` branch), which
// translateFramerErr turns into a framer-level connection error before any
// envoy-go code sees a frame. The ONLY way to reach the guard is to bypass that
// gate with AllowIllegalReads and hand the frame to dispatchFrame directly.
func TestServerConn_Continuation_UnexpectedContinuation_DefenseInDepth(t *testing.T) {
	rd := contIllegalFrameReader(t, func(fr *http2.Framer) error {
		return fr.WriteContinuation(1, true, []byte{0x82})
	})
	f, err := rd.ReadFrame()
	if err != nil {
		t.Fatalf("AllowIllegalReads should deliver the bare CONTINUATION: %v", err)
	}
	if _, ok := f.(*http2.ContinuationFrame); !ok {
		t.Fatalf("frame = %T, want *http2.ContinuationFrame", f)
	}

	sc := contIdleServerConn(t)
	contAssertConnError(t, sc.dispatchFrame(f), ErrProtocolError, "unexpected CONTINUATION for stream")
}

// TestServerConn_Continuation_WrongStreamContinuation_DefenseInDepth covers a
// CONTINUATION whose stream id differs from the open accumulator's.
//
// ⚠️ DEFENSE IN DEPTH — NOT COUNTED AS COVERAGE (PLAN-88 §4.1 / §5.2). Also
// UNREACHABLE through ReadFrame: checkFrameOrder rejects it first with
//
//	"got CONTINUATION for stream 3; expected stream 1"
//
// (frame.go, the `fh.StreamID != fr.lastHeaderStream` branch). AllowIllegalReads
// is again the only way past it.
func TestServerConn_Continuation_WrongStreamContinuation_DefenseInDepth(t *testing.T) {
	enc := newContClientEnc()
	block, splitAt := enc.split(
		[]hpack.HeaderField{{Name: ":method", Value: "GET"}, {Name: ":scheme", Value: "http"}},
		[]hpack.HeaderField{{Name: ":path", Value: "/x"}},
	)
	rd := contIllegalFrameReader(t, func(fr *http2.Framer) error {
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      1,
			BlockFragment: block[:splitAt],
			EndStream:     false,
			EndHeaders:    false,
		}); err != nil {
			return err
		}
		return fr.WriteContinuation(3, true, block[splitAt:])
	})

	sc := contIdleServerConn(t)

	// Dispatch the HEADERS frame FIRST (opening the accumulator) and only then
	// read the CONTINUATION: ReadFrame invalidates the previous frame's buffer.
	hf, err := rd.ReadFrame()
	if err != nil {
		t.Fatalf("read HEADERS: %v", err)
	}
	if herr := sc.dispatchFrame(hf); herr != nil {
		t.Fatalf("dispatch HEADERS (no END_HEADERS): %v", herr)
	}
	cf, err := rd.ReadFrame()
	if err != nil {
		t.Fatalf("AllowIllegalReads should deliver the cross-stream CONTINUATION: %v", err)
	}
	if _, ok := cf.(*http2.ContinuationFrame); !ok {
		t.Fatalf("frame = %T, want *http2.ContinuationFrame", cf)
	}
	contAssertConnError(t, sc.dispatchFrame(cf), ErrProtocolError, "expected stream")
}

// ---------------------------------------------------------------------------
// PLAN-88 §10 un-enumerated class 1: PRIORITY + CONTINUATION.
// ---------------------------------------------------------------------------

// TestServerConn_Continuation_PrioritySplitHeaders probes a HEADERS frame that
// carries BOTH the PRIORITY flag AND a CONTINUATION — a combination PLAN-88 §10
// records as ENTIRELY UNPROBED. The accumulator has to retain hasPriority and
// the PriorityParam across the split, and neither sub-arm alone proves that:
//
//   - dispatches: RED at the tip only because of the missing :path, so on its
//     own it would stay green against an accumulator that silently DROPPED the
//     priority data;
//   - self_dependency_rejected: the discriminating one. RFC 9113 §5.3.1 makes a
//     stream that depends on ITSELF a stream error, and that check reads
//     f.HasPriority()/f.Priority. It is GREEN AT THE TIP (which sees the
//     priority on the HEADERS frame it processes immediately) — it is here as
//     the pin that an accumulator which forgets the priority data turns it RED.
func TestServerConn_Continuation_PrioritySplitHeaders(t *testing.T) {
	t.Run("dispatches_with_priority", func(t *testing.T) {
		clientConn, fr, enc, d := contStartServer(t)
		block, splitAt := enc.split(
			[]hpack.HeaderField{
				{Name: ":method", Value: "GET"},
				{Name: ":scheme", Value: "http"},
				{Name: ":authority", Value: "test.local"},
			},
			[]hpack.HeaderField{{Name: ":path", Value: "/prio"}},
		)
		// StreamDep 0 = no dependency; Weight 15 makes the param non-zero so
		// x/net actually sets the PRIORITY flag.
		contWriteSplit(t, fr, 1, block, splitAt, true, http2.PriorityParam{StreamDep: 0, Weight: 15})

		reqs := d.waitReqs(1, 3*time.Second)
		if len(reqs) != 1 {
			t.Errorf("dispatched requests = %d, want 1 (PRIORITY must not block reassembly)", len(reqs))
		} else if reqs[0].Path != "/prio" {
			t.Errorf("dispatched Path = %q, want %q", reqs[0].Path, "/prio")
		}
		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool {
			return o.nData > 0 || len(o.rsts) > 0 || o.sawGoaway
		})
		if obs.statuses[1] != 200 {
			t.Errorf("status on stream 1 = %d, want 200", obs.statuses[1])
		}
		if code, ok := obs.rsts[1]; ok {
			t.Errorf("unexpected RST_STREAM(%v) on stream 1", code)
		}
	})

	t.Run("self_dependency_rejected", func(t *testing.T) {
		clientConn, fr, enc, _ := contStartServer(t)
		block, splitAt := enc.split(
			contGetHead("/selfdep"),
			[]hpack.HeaderField{{Name: "x-tail", Value: "t"}},
		)
		// StreamDep == the stream's own id: RFC 9113 §5.3.1 stream error.
		contWriteSplit(t, fr, 1, block, splitAt, true, http2.PriorityParam{StreamDep: 1, Weight: 15})

		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool {
			return len(o.rsts) > 0 || o.nData > 0 || o.sawGoaway
		})
		code, ok := obs.rsts[1]
		if !ok {
			t.Errorf("no RST_STREAM on stream 1 (statuses=%v goaway=%v); the split HEADERS' PRIORITY data must survive reassembly",
				obs.statuses, obs.sawGoaway)
			return
		}
		if code != http2.ErrCodeProtocol {
			t.Errorf("RST_STREAM code = %v, want PROTOCOL_ERROR", code)
		}
	})
}

// ---------------------------------------------------------------------------
// PLAN-88 §10 un-enumerated class 2: TRAILERS arriving split.
// ---------------------------------------------------------------------------

// contWriteRequestWithSplitTrailers writes HEADERS (whole, no END_STREAM),
// DATA (no END_STREAM), then a TRAILING header block split across
// HEADERS(END_STREAM, no END_HEADERS) + CONTINUATION(END_HEADERS).
//
// Note the flag placement: END_STREAM rides the trailing HEADERS frame; RFC
// 9113 §6.10 gives CONTINUATION only END_HEADERS.
func contWriteRequestWithSplitTrailers(t *testing.T, fr *http2.Framer, enc *contClientEnc, streamID uint32, path string, trailerHead, trailerTail []hpack.HeaderField) {
	t.Helper()
	reqBlock := enc.encode([]hpack.HeaderField{
		{Name: ":method", Value: "POST"},
		{Name: ":path", Value: path},
		{Name: ":scheme", Value: "http"},
		{Name: ":authority", Value: "test.local"},
	})
	contWriteWhole(t, fr, streamID, reqBlock, false)
	if err := fr.WriteData(streamID, false, []byte("hi")); err != nil {
		t.Fatalf("write DATA: %v", err)
	}
	tBlock, splitAt := enc.split(trailerHead, trailerTail)
	contWriteSplit(t, fr, streamID, tBlock, splitAt, true, http2.PriorityParam{})
}

// TestServerConn_Continuation_SplitTrailers probes the second class PLAN-88 §10
// records as ENTIRELY UNPROBED: a TRAILING header block split across a
// CONTINUATION after DATA.
//
// The discriminating sub-arm puts a PSEUDO-HEADER in the CONTINUATION half.
// RFC 9113 §8.1.2.1 bars pseudo-headers from a trailer section and
// serverStream.recvTrailingHeaders enforces it, so the reassembled block MUST
// be rejected with RST_STREAM(PROTOCOL_ERROR). At the tip the offending field
// rides the discarded CONTINUATION, the truncated trailer block looks clean,
// and the request is answered 200 — i.e. the tip's discard does not merely lose
// trailers, it DEFEATS TRAILER VALIDATION.
func TestServerConn_Continuation_SplitTrailers(t *testing.T) {
	t.Run("legal_split_trailers_control", func(t *testing.T) {
		clientConn, fr, enc, d := contStartServer(t)
		contWriteRequestWithSplitTrailers(t, fr, enc, 1, "/tr-ok",
			[]hpack.HeaderField{{Name: "x-trailer-a", Value: "a"}},
			[]hpack.HeaderField{{Name: "x-trailer-b", Value: "b"}},
		)
		reqs := d.waitReqs(1, 3*time.Second)
		if len(reqs) != 1 {
			t.Errorf("dispatched requests = %d, want 1", len(reqs))
		}
		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool {
			return o.nData > 0 || len(o.rsts) > 0 || o.sawGoaway
		})
		if obs.statuses[1] != 200 {
			t.Errorf("status on stream 1 = %d, want 200", obs.statuses[1])
		}
		if code, ok := obs.rsts[1]; ok {
			t.Errorf("unexpected RST_STREAM(%v) on a legal split trailer block", code)
		}
	})

	t.Run("pseudo_header_in_continuation_rejected", func(t *testing.T) {
		clientConn, fr, enc, _ := contStartServer(t)
		contWriteRequestWithSplitTrailers(t, fr, enc, 1, "/tr-bad",
			[]hpack.HeaderField{{Name: "x-trailer-a", Value: "a"}},
			[]hpack.HeaderField{{Name: ":method", Value: "GET"}},
		)
		obs := contReadFrames(t, clientConn, fr, 3*time.Second, func(o *contObserved) bool {
			return len(o.rsts) > 0 || o.nData > 0 || o.sawGoaway
		})
		code, ok := obs.rsts[1]
		if !ok {
			t.Errorf("no RST_STREAM on stream 1 (statuses=%v goaway=%v); a pseudo-header carried in the trailer CONTINUATION must still be rejected",
				obs.statuses, obs.sawGoaway)
			return
		}
		if code != http2.ErrCodeProtocol {
			t.Errorf("RST_STREAM code = %v, want PROTOCOL_ERROR", code)
		}
	})
}

// ---------------------------------------------------------------------------
// Client (upstream) read-leg mirror.
// ---------------------------------------------------------------------------

// writeSplitResponseHeaders writes a response header block as
// HEADERS(:status only, no END_HEADERS) + one CONTINUATION(the rest,
// END_HEADERS). END_STREAM, per RFC 9113 §6.10, rides the HEADERS frame.
//
// It is added ALONGSIDE the existing peer methods on the writeScriptedTrailers
// precedent (PLAN-88 §5.4): no existing helper can write a split header block,
// and a parallel harness would not exercise the same ClientConn wiring the rest
// of the client suite does.
//
// ⚠️ The split offset is taken from the encoder itself rather than from a
// payload size, so the CONTINUATION provably carries exactly the tail fields.
func (p *fakeH2ServerPeer) writeSplitResponseHeaders(streamID uint32, status int, tail []hpack.HeaderField, endStream bool) error {
	p.encBuf.Reset()
	if err := p.hpackEnc.WriteField(hpack.HeaderField{Name: ":status", Value: strconv.Itoa(status)}); err != nil {
		return err
	}
	splitAt := p.encBuf.Len()
	for _, h := range tail {
		if err := p.hpackEnc.WriteField(h); err != nil {
			return err
		}
	}
	block := make([]byte, p.encBuf.Len())
	copy(block, p.encBuf.Bytes())
	if err := p.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: block[:splitAt],
		EndStream:     endStream,
		EndHeaders:    false,
	}); err != nil {
		return err
	}
	return p.fr.WriteContinuation(streamID, true, block[splitAt:])
}

// TestClientConn_Continuation_SplitResponseHeaders is the client-leg mirror:
// a split RESPONSE header block from the peer must be reassembled and its
// fields must reach H2Response.
//
// The control is what makes the split arm non-vacuous — the SAME field written
// in ONE frame must arrive on both sides of the fix.
//
// Two independent ways to be wrong are pinned separately: the field can be lost
// (the tip's silent discard) or RoundTrip can return EARLY on the HEADERS
// frame's END_STREAM before the CONTINUATION has been consumed — which would
// leave the response "complete" but truncated. Both are asserted with
// t.Errorf.
func TestClientConn_Continuation_SplitResponseHeaders(t *testing.T) {
	t.Run("control_unsplit", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- err
				return
			}
			peerDone <- peer.writeResponse(hf.StreamID, 200,
				[]hpack.HeaderField{{Name: "x-cont", Value: "from-continuation"}}, nil)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if perr := <-peerDone; perr != nil {
			t.Fatalf("peer: %v", perr)
		}
		if resp.Status != 200 {
			t.Errorf("Status = %d, want 200", resp.Status)
		}
		if !contHasRespHeader(resp, "x-cont", "from-continuation") {
			t.Errorf("control: response headers = %v, want x-cont=from-continuation", resp.Headers)
		}
	})

	t.Run("split_response_block", func(t *testing.T) {
		cc, peer, cleanup := dialClientConnTCP(t)
		defer cleanup()

		peerDone := make(chan error, 1)
		go func() {
			hf, _, err := peer.readRequestHeaders()
			if err != nil {
				peerDone <- err
				return
			}
			peerDone <- peer.writeSplitResponseHeaders(hf.StreamID, 200,
				[]hpack.HeaderField{
					{Name: "content-type", Value: "text/plain"},
					{Name: "x-cont", Value: "from-continuation"},
				}, true)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := cc.RoundTrip(ctx, H2Request{
			Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
		})
		if perr := <-peerDone; perr != nil {
			t.Fatalf("peer: %v", perr)
		}
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		if resp.Status != 200 {
			t.Errorf("Status = %d, want 200", resp.Status)
		}
		if !contHasRespHeader(resp, "x-cont", "from-continuation") {
			t.Errorf("response headers = %v, want the CONTINUATION-carried x-cont=from-continuation to have ARRIVED", resp.Headers)
		}
		if !contHasRespHeader(resp, "content-type", "text/plain") {
			t.Errorf("response headers = %v, want the CONTINUATION-carried content-type=text/plain to have ARRIVED", resp.Headers)
		}
		if cc.Closed() {
			t.Errorf("ClientConn closed; a split response block must not tear down the (pooled) upstream connection")
		}
	})
}

// contHasRespHeader reports whether resp carries name=value.
func contHasRespHeader(resp H2Response, name, value string) bool {
	for _, h := range resp.Headers {
		if h.Name == name && h.Value == value {
			return true
		}
	}
	return false
}

// writeFloodedResponseHeaders opens a response header block with a HEADERS
// frame that does NOT carry END_HEADERS, then streams `frames` CONTINUATION
// frames of `size` bytes each, none terminating the block. It is the client-leg
// mirror of the server flood arm.
//
// The fragments after the first are junk: the accumulator is a BYTE buffer that
// decodes only at END_HEADERS, so the bound must trip before any HPACK decode
// is attempted. If a future change decodes incrementally this arm would report
// COMPRESSION_ERROR instead of ENHANCE_YOUR_CALM, which is itself the signal.
func (p *fakeH2ServerPeer) writeFloodedResponseHeaders(streamID uint32, frames, size int) error {
	p.encBuf.Reset()
	if err := p.hpackEnc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
		return err
	}
	head := make([]byte, p.encBuf.Len())
	copy(head, p.encBuf.Bytes())
	if err := p.fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: head,
		EndStream:     false,
		EndHeaders:    false,
	}); err != nil {
		return err
	}
	junk := make([]byte, size)
	for i := 0; i < frames; i++ {
		if err := p.fr.WriteContinuation(streamID, false, junk); err != nil {
			return err
		}
	}
	return nil
}

// TestClientConn_Continuation_FloodExceedsBound pins the CLIENT leg's
// CONTINUATION REJECT path — the arm the phase-88 IMPL's adversarial review
// proved was entirely unguarded: swallowing the whole error branch
// (clearing the accumulator and returning nil instead of emitting GOAWAY and
// failing the conn) left the unit suite GREEN.
//
// This matters more on the client than on the server, because the upstream
// connection is POOLED: an unbounded accumulator there is retained memory
// charged to a conn that unrelated concurrent requests are riding (ADR-0310
// §Consequences (i)).
//
// Two properties, t.Errorf each (reference_fatalf_makes_assertions_unreachable):
// the peer must observe GOAWAY(ENHANCE_YOUR_CALM), and the in-flight RoundTrip
// must FAIL rather than hang or return a truncated 200.
func TestClientConn_Continuation_FloodExceedsBound(t *testing.T) {
	cc, peer, cleanup := dialClientConnTCP(t)
	defer cleanup()

	peerDone := make(chan error, 1)
	go func() {
		hf, _, err := peer.readRequestHeaders()
		if err != nil {
			peerDone <- err
			return
		}
		// 16 MiB bound / 16384-byte frames => 1024 frames reaches it; 1026
		// crosses it with margin for the opening fragment.
		peerDone <- peer.writeFloodedResponseHeaders(hf.StreamID, 1026, 16384)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, rtErr := cc.RoundTrip(ctx, H2Request{
		Method: "GET", Path: "/", Scheme: "https", Authority: "example.test",
	})
	if rtErr == nil {
		t.Errorf("RoundTrip err = nil, want a failure: a flood past maxHeaderBlockSize must not complete as a response")
	}

	// Drain the peer side looking for our GOAWAY. The write side may error out
	// once we tear the conn down, which is expected and not asserted.
	<-peerDone
	var sawGoaway bool
	var code http2.ErrCode
	_ = peer.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		f, err := peer.readNextFrame()
		if err != nil {
			break
		}
		if ga, ok := f.(*http2.GoAwayFrame); ok {
			sawGoaway = true
			code = ga.ErrCode
			break
		}
	}
	if !sawGoaway {
		t.Errorf("no GOAWAY observed after a CONTINUATION flood past maxHeaderBlockSize (%d bytes)", maxHeaderBlockSize)
	} else if code != http2.ErrCodeEnhanceYourCalm {
		t.Errorf("GOAWAY ErrCode = %v, want ENHANCE_YOUR_CALM — the connection-scoped flood reject (ADR-0310 D-88-CODE)", code)
	}
}

// TestServerConn_Continuation_AcceptsPastReferenceLimit pins the LOWER side of
// maxHeaderBlockSize — the side the flood arm cannot see.
//
// The flood arm pins only "some bound at or below 16 MiB + 2 frames exists": a
// 16384x SHRINK of maxHeaderBlockSize leaves it GREEN (verified at the
// phase-88 IMPL's adversarial review). What actually needs pinning is the
// DIVERGENCE the behavior contract claims BY SYMBOL — that envoy-go ACCEPTS a
// header block the pinned reference rejects.
//
// 96 KiB of encoded block is chosen because it is measured to sit ABOVE both
// reference thresholds — past max_request_headers_kb (60 KiB) and past the
// ~64 KiB encoded point at which contrib-v1.37.2 switches from a stream-scoped
// RST_STREAM(INTERNAL_ERROR) to a connection-scoped GOAWAY(COMPRESSION_ERROR)
// — while sitting far under envoy-go's 16 MiB. A shrink of the constant to
// anything at or below 96 KiB reddens this row.
func TestServerConn_Continuation_AcceptsPastReferenceLimit(t *testing.T) {
	const padLen = 96 * 1024

	clientConn, fr, enc, d := contStartServer(t)
	block := enc.encode(append(contGetHead("/big"),
		hpack.HeaderField{Name: "x-big", Value: strings.Repeat("a", padLen)}))
	// ⚠️ HOP AT THE PEER'S LIMIT, NOT IN TWO PIECES. The block is far larger
	// than SETTINGS_MAX_FRAME_SIZE, so writing it as HEADERS + ONE CONTINUATION
	// produces two oversized frames and the server's own framer answers
	// GOAWAY(FRAME_SIZE_ERROR) before the accumulator is ever reached — which
	// would make this row test the framer's hop check rather than the bound.
	contWriteHopped(t, fr, 1, block, 16384, true)

	reqs := d.waitReqs(1, 5*time.Second)
	if len(reqs) != 1 {
		t.Errorf("dispatched requests = %d, want 1 — envoy-go must ACCEPT a block the reference rejects (maxHeaderBlockSize = %d)",
			len(reqs), maxHeaderBlockSize)
	} else {
		var got string
		for _, h := range reqs[0].Headers {
			if h.Name == "x-big" {
				got = h.Value
			}
		}
		if len(got) != padLen {
			t.Errorf("x-big len = %d, want %d (the whole reassembled block must arrive, not a prefix)", len(got), padLen)
		}
	}

	obs := contReadFrames(t, clientConn, fr, 5*time.Second, func(o *contObserved) bool {
		return o.nData > 0 || len(o.rsts) > 0 || o.sawGoaway
	})
	if obs.sawGoaway {
		t.Errorf("GOAWAY(%v) on a %d-byte block far under maxHeaderBlockSize %d", obs.goawayCode, padLen, maxHeaderBlockSize)
	}
	if code, ok := obs.rsts[1]; ok {
		t.Errorf("unexpected RST_STREAM(%v) — this is exactly the reference behavior envoy-go deliberately diverges from", code)
	}
}
