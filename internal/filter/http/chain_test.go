package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingFilter logs each callback for assertion.
type recordingFilter struct {
	name string
	// Decode-side return statuses.
	headersStatus  FilterHeadersStatus
	dataStatus     FilterDataStatus
	trailersStatus FilterTrailersStatus
	// Encode-side return statuses (separate so encode/decode can diverge in
	// tests without coupling — added for Task 6).
	encHeadersStatus  FilterHeadersStatus
	encDataStatus     FilterDataStatus
	encTrailersStatus FilterTrailersStatus
	decodeHeaders     atomic.Int32
	decodeData        atomic.Int32
	decodeTrailers    atomic.Int32
	encodeHeaders     atomic.Int32
	encodeData        atomic.Int32
	encodeTrailers    atomic.Int32
	destroyed         atomic.Int32
	dcb               DecoderFilterCallbacks
	ecb               EncoderFilterCallbacks
}

func (f *recordingFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.decodeHeaders.Add(1)
	return f.headersStatus
}
func (f *recordingFilter) DecodeData([]byte, bool) FilterDataStatus {
	f.decodeData.Add(1)
	return f.dataStatus
}
func (f *recordingFilter) DecodeTrailers(http.Header) FilterTrailersStatus {
	f.decodeTrailers.Add(1)
	return f.trailersStatus
}
func (f *recordingFilter) SetDecoderCallbacks(cb DecoderFilterCallbacks) { f.dcb = cb }
func (f *recordingFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus {
	f.encodeHeaders.Add(1)
	return f.encHeadersStatus
}
func (f *recordingFilter) EncodeData([]byte, bool) FilterDataStatus {
	f.encodeData.Add(1)
	return f.encDataStatus
}
func (f *recordingFilter) EncodeTrailers(http.Header) FilterTrailersStatus {
	f.encodeTrailers.Add(1)
	return f.encTrailersStatus
}
func (f *recordingFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks) { f.ecb = cb }
func (f *recordingFilter) OnDestroy()                                    { f.destroyed.Add(1) }

func newChainOf(filters ...*recordingFilter) (*FilterChain, []*recordingFilter) {
	hf := make([]HTTPFilter, len(filters))
	for i, f := range filters {
		hf[i] = HTTPFilter{Name: f.name, Decoder: f, Encoder: f}
	}
	return NewFilterChain(hf, nil), filters
}

func TestChain_Decode_AllContinue(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected RunDecodeHeaders to report iteration-complete (terminated=true)")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's DecodeHeaders called once; got a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_ResumeAdvances(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.dcb.ContinueDecoding()
	}()
	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after async resume")
	}
	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected b's DecodeHeaders to run after resume; a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

func TestChain_Decode_StopIteration_CtxCancelAborts(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration}
	b := &recordingFilter{name: "b", headersStatus: Continue}
	chain, _ := newChainOf(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	terminated, err := chain.RunDecodeHeaders(ctx, http.Header{}, true)
	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if terminated {
		t.Fatalf("expected aborted iteration; got terminated=true")
	}
	chain.Destroy()
	if a.destroyed.Load() == 0 || b.destroyed.Load() == 0 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy after ctx-cancel; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}
}

// encodeRecorder wraps a recordingFilter as a StreamEncoderFilter so that
// per-filter EncodeHeaders invocation order can be observed independent of
// the underlying recording counters. Used by TestChain_Encode_ReverseOrder
// to assert the SPEC §11.1 empirical pin.
type encodeRecorder struct {
	f     *recordingFilter
	order *[]string
	mu    *sync.Mutex
}

func (e encodeRecorder) EncodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	e.mu.Lock()
	*e.order = append(*e.order, e.f.name)
	e.mu.Unlock()
	return e.f.EncodeHeaders(h, end)
}
func (e encodeRecorder) EncodeData(d []byte, end bool) FilterDataStatus      { return e.f.EncodeData(d, end) }
func (e encodeRecorder) EncodeTrailers(t http.Header) FilterTrailersStatus   { return e.f.EncodeTrailers(t) }
func (e encodeRecorder) SetEncoderCallbacks(cb EncoderFilterCallbacks)       { e.f.SetEncoderCallbacks(cb) }
func (e encodeRecorder) OnDestroy()                                          { e.f.OnDestroy() }

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChain_Encode_ReverseOrder(t *testing.T) {
	order := make([]string, 0, 3)
	var orderMu sync.Mutex
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	b := mk("b")
	c := mk("c")
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: encodeRecorder{f: a, order: &order, mu: &orderMu}},
		{Name: "b", Decoder: b, Encoder: encodeRecorder{f: b, order: &order, mu: &orderMu}},
		{Name: "c", Decoder: c, Encoder: encodeRecorder{f: c, order: &order, mu: &orderMu}},
	}
	chain := NewFilterChain(hf, nil)
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration complete")
	}
	want := []string{"c", "b", "a"}
	if !equalSlice(order, want) {
		t.Fatalf("expected encode order %v; got %v", want, order)
	}
}

func TestChain_Encode_StopIteration_ResumeAdvances(t *testing.T) {
	// b returns StopIteration on encode; iteration starts at index len-1=1 (b),
	// parks, then async ContinueEncoding unparks; cursor advances to a (index 0)
	// which returns Continue and completes.
	a := &recordingFilter{
		name:              "a",
		encHeadersStatus:  Continue,
		encDataStatus:     DataContinue,
		encTrailersStatus: TrailersContinue,
	}
	b := &recordingFilter{
		name:              "b",
		encHeadersStatus:  StopIteration,
		encDataStatus:     DataContinue,
		encTrailersStatus: TrailersContinue,
	}
	chain, _ := newChainOf(a, b)
	go func() {
		time.Sleep(20 * time.Millisecond)
		b.ecb.ContinueEncoding()
	}()
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after async resume")
	}
	if a.encodeHeaders.Load() != 1 || b.encodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's EncodeHeaders called once; got a=%d b=%d", a.encodeHeaders.Load(), b.encodeHeaders.Load())
	}
}

func TestChain_Encode_StopIteration_CtxCancelAborts(t *testing.T) {
	// Encode-side analogue of TestChain_Decode_StopIteration_CtxCancelAborts.
	// b returns StopIteration on encode; ctx cancellation during park yields
	// ctx.Err and terminated=false.
	a := &recordingFilter{name: "a", encHeadersStatus: Continue}
	b := &recordingFilter{name: "b", encHeadersStatus: StopIteration}
	chain, _ := newChainOf(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	terminated, err := chain.RunEncodeHeaders(ctx, http.Header{}, true)
	if err == nil {
		t.Fatalf("expected ctx-cancel error")
	}
	if terminated {
		t.Fatalf("expected aborted iteration; got terminated=true")
	}
	chain.Destroy()
	if a.destroyed.Load() == 0 || b.destroyed.Load() == 0 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy after ctx-cancel; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}
}

// TestChain_Encode_UnknownTrailersStatusErrs guards the spec-review fix to
// RunEncodeTrailers' switch. Before the fix, an unknown FilterTrailersStatus
// value fell through the switch, encodeIdx was not decremented, and the for-
// loop re-tested the same cursor → infinite loop on the dispatch goroutine.
// The default clause (mirroring RunEncodeHeaders / RunEncodeData / RunDecode*)
// must abort iteration with a descriptive error and terminated=false. Run with
// a -timeout shorter than the default 10m so a regression hangs the test
// rather than the whole suite, but t.Fatalf on timeout would still fire.
func TestChain_Encode_UnknownTrailersStatusErrs(t *testing.T) {
	// Cast 99 (an unknown FilterTrailersStatus value) to force the default
	// branch in RunEncodeTrailers' switch.
	b := &recordingFilter{
		name:              "b",
		encHeadersStatus:  Continue,
		encDataStatus:     DataContinue,
		encTrailersStatus: FilterTrailersStatus(99),
	}
	chain, _ := newChainOf(b)
	done := make(chan struct{})
	var (
		terminated bool
		err        error
	)
	go func() {
		terminated, err = chain.RunEncodeTrailers(context.Background(), http.Header{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("RunEncodeTrailers hung on unknown FilterTrailersStatus — missing default clause regressed")
	}
	if err == nil {
		t.Fatalf("expected unknown-status error from RunEncodeTrailers; got nil")
	}
	if terminated {
		t.Fatalf("expected terminated=false on unknown-status err; got true")
	}
	if b.encodeTrailers.Load() != 1 {
		t.Fatalf("expected EncodeTrailers called exactly once; got %d", b.encodeTrailers.Load())
	}
}

// localReplyFilter wraps a recordingFilter and triggers SendLocalReply on its
// DecodeHeaders call. Used by the SendLocalReply tests to inject the
// encode-chain-entry path. Returns StopIteration so the chain stops further
// decode-side iteration after the trigger.
type localReplyFilter struct {
	*recordingFilter
	status  int
	body    string
	headers OrderedHeaders
	// triggerCount permits TestChain_SendLocalReply_FirstCallWins to call
	// SendLocalReply twice in a row from a single DecodeHeaders. Default 1.
	triggerCount int
}

func (lf *localReplyFilter) DecodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	lf.recordingFilter.DecodeHeaders(h, end)
	n := lf.triggerCount
	if n == 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		lf.dcb.SendLocalReply(lf.status, lf.body, lf.headers)
	}
	return StopIteration
}

// TestChain_SendLocalReply_EntersAtLenMinus1 asserts SPEC §11 #4 empirical
// pin: on a synthetic 4-filter chain [a, b, c, router] where b's DecodeHeaders
// calls SendLocalReply, the FULL encode chain runs in reverse declaration
// order — router → c → b → a — and NO decode side past b runs.
func TestChain_SendLocalReply_EntersAtLenMinus1(t *testing.T) {
	order := make([]string, 0, 4)
	var orderMu sync.Mutex
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	bRec := mk("b")
	c := mk("c")
	router := mk("router")
	bTrigger := &localReplyFilter{
		recordingFilter: bRec,
		status:          418,
		body:            "i am teapot",
		headers:         nil,
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: encodeRecorder{f: a, order: &order, mu: &orderMu}},
		{Name: "b", Decoder: bTrigger, Encoder: encodeRecorder{f: bRec, order: &order, mu: &orderMu}},
		{Name: "c", Decoder: c, Encoder: encodeRecorder{f: c, order: &order, mu: &orderMu}},
		{Name: "router", Decoder: router, Encoder: encodeRecorder{f: router, order: &order, mu: &orderMu}},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)

	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if terminated {
		// SendLocalReply aborts decode-side iteration; expect terminated=false.
		t.Fatalf("expected decode iteration aborted by SendLocalReply (terminated=false); got true")
	}
	// The FULL encode chain must have run in reverse order including b's own
	// encode side (per ADR-0075 (d)).
	want := []string{"router", "c", "b", "a"}
	if !equalSlice(order, want) {
		t.Fatalf("expected encode order %v (entry at filter[len-1], reverse iteration, calling filter's encode side INCLUDED); got %v", want, order)
	}
	// Decode side past b must NOT have run (a + b were called; c + router were not).
	if a.decodeHeaders.Load() != 1 {
		t.Fatalf("expected a.DecodeHeaders called once; got %d", a.decodeHeaders.Load())
	}
	if bRec.decodeHeaders.Load() != 1 {
		t.Fatalf("expected b.DecodeHeaders called once; got %d", bRec.decodeHeaders.Load())
	}
	if c.decodeHeaders.Load() != 0 {
		t.Fatalf("expected c.DecodeHeaders NOT called; got %d", c.decodeHeaders.Load())
	}
	if router.decodeHeaders.Load() != 0 {
		t.Fatalf("expected router.DecodeHeaders NOT called; got %d", router.decodeHeaders.Load())
	}
}

// TestChain_SendLocalReply_FirstCallWins asserts that two back-to-back
// SendLocalReply calls from the same DecodeHeaders invocation result in
// exactly one synthesized response. The dedup mechanism is layered: the
// `c.encodeStarted.Load()` early-return at the top of beginLocalReply
// short-circuits the second call BEFORE Once.Do is reached (RunEncodeHeaders
// flips encodeStarted on the first call's encode pass, before this filter's
// DecodeHeaders even returns to issue the second SendLocalReply). Once.Do is
// defense-in-depth for a hypothetical pre-RunEncodeHeaders concurrent call
// from another goroutine — which the ADR-0071 single-driver invariant rules
// out in production. The user-observable behavior (one synthesized response)
// is correct either way.
func TestChain_SendLocalReply_FirstCallWins(t *testing.T) {
	order := make([]string, 0, 2)
	var orderMu sync.Mutex
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	bRec := mk("b")
	bTrigger := &localReplyFilter{
		recordingFilter: bRec,
		status:          500,
		body:            "boom",
		headers:         nil,
		triggerCount:    2, // call SendLocalReply twice in a row
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: encodeRecorder{f: a, order: &order, mu: &orderMu}},
		{Name: "b", Decoder: bTrigger, Encoder: encodeRecorder{f: bRec, order: &order, mu: &orderMu}},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)
	_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	// Each encode-side filter should run exactly once. The encodeStarted gate
	// short-circuits the second SendLocalReply at the top of beginLocalReply
	// (the encode chain has already started + flipped the flag by the time
	// the calling filter's DecodeHeaders issues call #2); Once.Do is
	// defense-in-depth and is not exercised here.
	if a.encodeHeaders.Load() != 1 {
		t.Fatalf("expected a.EncodeHeaders called once (first-call-wins); got %d", a.encodeHeaders.Load())
	}
	if bRec.encodeHeaders.Load() != 1 {
		t.Fatalf("expected b.EncodeHeaders called once (first-call-wins); got %d", bRec.encodeHeaders.Load())
	}
}

// TestChain_SendLocalReply_CallingFilterEncodeRuns asserts ADR-0075 (d): the
// FULL encode chain runs INCLUDING the calling filter's own encode side.
// (Distinct from EntersAtLenMinus1 in framing — that test asserts ordering;
// this one asserts inclusion of the calling filter.)
func TestChain_SendLocalReply_CallingFilterEncodeRuns(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	bRec := mk("b")
	c := mk("c")
	bTrigger := &localReplyFilter{
		recordingFilter: bRec,
		status:          403,
		body:            "denied",
		headers:         nil,
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: a},
		{Name: "b", Decoder: bTrigger, Encoder: bRec},
		{Name: "c", Decoder: c, Encoder: c},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)
	_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	// b is the calling filter — its EncodeHeaders MUST run per ADR-0075 (d).
	if bRec.encodeHeaders.Load() != 1 {
		t.Fatalf("expected calling filter b's EncodeHeaders to run on SendLocalReply (ADR-0075 (d)); got %d", bRec.encodeHeaders.Load())
	}
	// All other encode sides also run (full chain in reverse).
	if a.encodeHeaders.Load() != 1 {
		t.Fatalf("expected a.EncodeHeaders called; got %d", a.encodeHeaders.Load())
	}
	if c.encodeHeaders.Load() != 1 {
		t.Fatalf("expected c.EncodeHeaders called; got %d", c.encodeHeaders.Load())
	}
	// Body present → EncodeData must also run on each encoder.
	if bRec.encodeData.Load() != 1 || a.encodeData.Load() != 1 || c.encodeData.Load() != 1 {
		t.Fatalf("expected each filter's EncodeData called once for non-empty body; got a=%d b=%d c=%d",
			a.encodeData.Load(), bRec.encodeData.Load(), c.encodeData.Load())
	}
}

// TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs asserts that a
// second SendLocalReply invoked AFTER the encode chain has begun is a no-op
// AND emits the diagnostic log line (per ADR-0075 (e)).
func TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs(t *testing.T) {
	var buf bytes.Buffer

	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	// Encoder for a: when it runs (during the synthesized-reply encode pass),
	// it issues a SECOND SendLocalReply via a's decoder callbacks. encodeStarted
	// is true at this point so the call should be a no-op + log.
	a2 := &reentrantEncoderFilter{recordingFilter: a}

	bRec := mk("b")
	bTrigger := &localReplyFilter{
		recordingFilter: bRec,
		status:          418,
		body:            "i am teapot",
		headers:         nil,
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: a2},
		{Name: "b", Decoder: bTrigger, Encoder: bRec},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)
	chain.SetDiagLogWriter(&buf)
	// a2 needs a decoder callback handle to issue the second SendLocalReply
	// from the encode-side path; the chain wired callbacks at NewFilterChain.
	a2.dcb = a.dcb

	_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)

	logged := buf.String()
	wantSubstr := `hcm: filter "a" called SendLocalReply after encode-side started; ignoring`
	if !strings.Contains(logged, wantSubstr) {
		t.Fatalf("expected log to contain %q; got %q", wantSubstr, logged)
	}
	// First SendLocalReply still won; encode chain ran exactly once per filter.
	if a.encodeHeaders.Load() != 1 {
		t.Fatalf("expected a.EncodeHeaders called once; got %d", a.encodeHeaders.Load())
	}
	if bRec.encodeHeaders.Load() != 1 {
		t.Fatalf("expected b.EncodeHeaders called once; got %d", bRec.encodeHeaders.Load())
	}
}

// reentrantEncoderFilter is an encoder that issues a SendLocalReply from its
// EncodeHeaders. Used by TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
// to drive the "second call after encode-started" path.
type reentrantEncoderFilter struct {
	*recordingFilter
	dcb DecoderFilterCallbacks
}

func (r *reentrantEncoderFilter) EncodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	st := r.recordingFilter.EncodeHeaders(h, end)
	if r.dcb != nil {
		r.dcb.SendLocalReply(500, "second-call-ignored", nil)
	}
	return st
}

// headerCaptureRecorder is an Encoder wrapper that captures the encoded
// http.Header map exactly as beginLocalReply passes it down the encode chain.
// Used by TestChain_SendLocalReply_UserContentTypeNonCanonicalKey to assert
// that user-supplied non-canonical keys (e.g. "content-type") are merged into
// the canonical "Content-Type" key — i.e. NO duplicate header pair on the wire.
type headerCaptureRecorder struct {
	f        *recordingFilter
	captured *http.Header
	mu       *sync.Mutex
}

func (h headerCaptureRecorder) EncodeHeaders(hd http.Header, end bool) FilterHeadersStatus {
	h.mu.Lock()
	// Snapshot the header map at the moment this encoder runs.
	cp := make(http.Header, len(hd))
	for k, vs := range hd {
		cp[k] = append([]string(nil), vs...)
	}
	*h.captured = cp
	h.mu.Unlock()
	return h.f.EncodeHeaders(hd, end)
}
func (h headerCaptureRecorder) EncodeData(d []byte, end bool) FilterDataStatus {
	return h.f.EncodeData(d, end)
}
func (h headerCaptureRecorder) EncodeTrailers(t http.Header) FilterTrailersStatus {
	return h.f.EncodeTrailers(t)
}
func (h headerCaptureRecorder) SetEncoderCallbacks(cb EncoderFilterCallbacks) {
	h.f.SetEncoderCallbacks(cb)
}
func (h headerCaptureRecorder) OnDestroy() { h.f.OnDestroy() }

// TestChain_SendLocalReply_UserContentTypeNonCanonicalKey is the regression
// guard for I-1 (code-quality review on commit a03a1d3): when a filter passes
// a user-supplied header map with a non-canonical key like "content-type",
// beginLocalReply's merge step must canonicalize the key so the wire shape
// has EXACTLY ONE Content-Type header (the user's value), not a duplicate
// pair (`content-type: application/json` AND `Content-Type: text/plain`).
//
// Pre-fix: `for k, v := range headers { merged[k] = v }` copies the lowercase
// key verbatim; merged.Get("Content-Type") (which canonicalizes its arg) then
// misses the user value, and the framework injects the default text/plain
// under the canonical key — duplicate Content-Type on the wire.
//
// Post-fix: `merged.Add(k, v)` calls textproto.CanonicalMIMEHeaderKey
// internally, ensuring all keys land canonical and the user's value wins.
func TestChain_SendLocalReply_UserContentTypeNonCanonicalKey(t *testing.T) {
	var captured http.Header
	var capMu sync.Mutex

	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	aRec := mk("a")
	router := mk("router")
	aTrigger := &localReplyFilter{
		recordingFilter: aRec,
		status:          200,
		body:            "hi",
		// Non-canonical key "content-type" is the regression probe — beginLocalReply
		// must canonicalize it via http.Header.Add when building its mutation view.
		headers: OrderedHeaders{{Name: "content-type", Value: "application/json"}},
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: aTrigger, Encoder: headerCaptureRecorder{f: aRec, captured: &captured, mu: &capMu}},
		{Name: "router", Decoder: router, Encoder: headerCaptureRecorder{f: router, captured: &captured, mu: &capMu}},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)
	_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)

	// Snapshot the captured map (router's encoder runs first per reverse
	// iteration; a's overwrites with the same canonical map).
	capMu.Lock()
	got := captured
	capMu.Unlock()
	if got == nil {
		t.Fatalf("expected encode chain to have run and captured headers; got nil")
	}

	// The canonical "Content-Type" key MUST exist with the user's value.
	ct := got["Content-Type"]
	if len(ct) != 1 || ct[0] != "application/json" {
		t.Fatalf("expected exactly one canonical Content-Type=application/json; got %v", ct)
	}
	// The non-canonical "content-type" key MUST NOT exist (would manifest
	// as a duplicate header on the wire). Probing for the non-canonical key
	// is the whole point of this regression test — staticcheck SA1008 flags
	// the literal as suspicious in a normal Header use, but here it is the
	// negative assertion we need.
	//nolint:staticcheck // SA1008: deliberate probe for non-canonical key absence
	if v, present := got["content-type"]; present {
		t.Fatalf("expected non-canonical \"content-type\" to be merged into canonical key; saw duplicate: %v", v)
	}
	// Defense-in-depth: total Content-Type values across the entire map (any
	// case) is exactly 1 — no other casing variant either.
	totalCT := 0
	for k, vs := range got {
		if strings.EqualFold(k, "Content-Type") {
			totalCT += len(vs)
		}
	}
	if totalCT != 1 {
		t.Fatalf("expected exactly one Content-Type value across all casings; got %d in %v", totalCT, got)
	}
}

// TestChain_SendLocalReply_DefaultsAmbientCtxToBackground is the regression
// guard for I-2 (code-quality review on commit a03a1d3): if a filter calls
// SendLocalReply BEFORE HCM dispatch has called SetRequestCtx (e.g. tests, or
// pre-Task-13 callers), c.ambientCtx is nil → decoderCB.SendLocalReply
// propagates nil to beginLocalReply → RunEncode* → parkEncode(nil) where
// `<-ctx.Done()` on a nil channel blocks forever, masking cancellation.
//
// The hang only manifests when an encoder filter returns StopIteration, since
// that is when parkEncode is reached. We construct a chain where router's
// encode side returns StopIteration once, then ContinueEncoding is fired
// async — under the fix (ambientCtx defaults to context.Background()) the
// resume channel unparks the encode chain. Without the fix, parkEncode would
// be `select { case <-resumeCh; case <-nil-ctx-Done }` — fortuitously the
// resumeCh path still wins, BUT a hostile race between a hypothetical
// ctx-cancel and the resume signal would blackhole. The strong assertion is
// nil-safety in the select itself: a nil context.Done() channel just blocks
// (does not panic), which means the bug is silent — we therefore assert the
// observable invariant that parkEncode does not block forever and the encode
// chain completes.
//
// Fix: NewFilterChain sets ambientCtx = context.Background() by default.
func TestChain_SendLocalReply_DefaultsAmbientCtxToBackground(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	aRec := mk("a")
	// router's encode side returns StopIteration — forces parkEncode under the
	// (potentially-nil) ambient ctx. Resume is fired async so the test has a
	// happy-path completion.
	router := mk("router")
	router.encHeadersStatus = StopIteration

	aTrigger := &localReplyFilter{
		recordingFilter: aRec,
		status:          200,
		body:            "ok",
		headers:         nil,
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: aTrigger, Encoder: aRec},
		{Name: "router", Decoder: router, Encoder: router},
	}
	chain := NewFilterChain(hf, nil)
	// NOTE: deliberately NOT calling chain.SetRequestCtx — exercises the
	// ambientCtx == nil path. Without the I-2 fix, parkEncode's
	// `case <-ctx.Done()` arm operates on a nil channel.

	// Async resume so the StopIteration park returns. We allow up to 1s
	// before the resume signal fires; under the fix this completes quickly.
	go func() {
		time.Sleep(20 * time.Millisecond)
		router.ecb.ContinueEncoding()
	}()

	done := make(chan struct{})
	go func() {
		defer func() {
			// Defensive: a nil-ctx panic would also fail this test.
			_ = recover()
			close(done)
		}()
		_, _ = chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("RunDecodeHeaders hung — ambientCtx default-init regressed (parkEncode reached on nil ctx.Done channel)")
	}

	// Encode chain ran on both filters (full reverse iteration).
	if aRec.encodeHeaders.Load() != 1 {
		t.Fatalf("expected a.EncodeHeaders called once; got %d", aRec.encodeHeaders.Load())
	}
	if router.encodeHeaders.Load() != 1 {
		t.Fatalf("expected router.EncodeHeaders called once; got %d", router.encodeHeaders.Load())
	}
}

// TestChain_ConcurrentContinueDecoding_Coalesced asserts SPEC §5.7 + §14.10
// bullet 1: N goroutines concurrently calling ContinueDecoding on the same
// chain are silently coalesced by the buffered-1 + non-blocking-send pattern.
// Concretely: filter[0] returns StopIteration → dispatch goroutine parks; 64
// goroutines each call dcb.ContinueDecoding; the buffered-1 channel absorbs
// exactly one send (the rest hit the default arm of the select and silently
// drop); dispatch unparks once and iteration completes; no panic, no race
// (assert under -race), no goroutine leak (sync.WaitGroup guards return of
// every spawned goroutine before the test exits).
//
// The discipline under test lives in decoderCB.ContinueDecoding:
//
//	select { case d.c.decodeResumeCh <- struct{}{}: default: }
//
// Without the `default:` arm, 63 of the 64 goroutines would block forever on
// the channel send (capacity 1 is full), which would manifest under -race as
// a goroutine leak detected by the test's WaitGroup timeout.
func TestChain_ConcurrentContinueDecoding_Coalesced(t *testing.T) {
	a := &recordingFilter{name: "a", headersStatus: StopIteration, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	b := &recordingFilter{name: "b", headersStatus: Continue, dataStatus: DataContinue, trailersStatus: TrailersContinue}
	chain, _ := newChainOf(a, b)

	const N = 64
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	// Spawn N goroutines that all unblock simultaneously on close(startGate)
	// and then hammer ContinueDecoding. Use a small delay before close() to
	// give the dispatch goroutine time to enter parkDecode, so all 64 sends
	// race against an actually-parked receiver (vs. hitting it before park).
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-startGate
			a.dcb.ContinueDecoding()
		}()
	}
	go func() {
		// Let dispatch reach parkDecode before flooding the channel.
		time.Sleep(20 * time.Millisecond)
		close(startGate)
	}()

	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected iteration to complete after concurrent resume hammer")
	}

	// Every spawned goroutine MUST have returned — a leak would mean a sender
	// blocked on a full channel (i.e., the `default:` arm regressed). Bound
	// the wait so a regression fails fast rather than hanging the test suite.
	doneAll := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneAll)
	}()
	select {
	case <-doneAll:
	case <-time.After(2 * time.Second):
		t.Fatalf("ContinueDecoding goroutines leaked — non-blocking-send default arm regressed (some sends blocked on full channel)")
	}

	if a.decodeHeaders.Load() != 1 || b.decodeHeaders.Load() != 1 {
		t.Fatalf("expected each filter's DecodeHeaders called once; got a=%d b=%d", a.decodeHeaders.Load(), b.decodeHeaders.Load())
	}
}

// timerSendLocalReplyFilter is filter[1] of TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply.
// Its DecodeHeaders spawns a goroutine that — after a short delay — calls
// cb.SendLocalReply from OUTSIDE the dispatch goroutine, then calls
// ContinueDecoding to unpark the dispatch goroutine (which will see
// localReplyDone=true and return cleanly). DecodeHeaders itself returns
// StopIteration, which parks the dispatch goroutine while the timer races.
type timerSendLocalReplyFilter struct {
	*recordingFilter
	delay  time.Duration
	status int
	body   string
}

func (f *timerSendLocalReplyFilter) DecodeHeaders(h http.Header, end bool) FilterHeadersStatus {
	f.recordingFilter.DecodeHeaders(h, end)
	go func() {
		time.Sleep(f.delay)
		// Race: SendLocalReply is called from this goroutine while the
		// dispatch goroutine is parked in parkDecode. The encode chain runs
		// on THIS goroutine via beginLocalReply. After encode completes we
		// signal ContinueDecoding to unpark dispatch, which will observe
		// localReplyDone=true at the top of the RunDecodeHeaders loop and
		// return (false, nil).
		f.dcb.SendLocalReply(f.status, f.body, nil)
		f.dcb.ContinueDecoding()
	}()
	return StopIteration
}

// TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply asserts SPEC §5.6
// + §14.10 bullet 2: a filter's timer goroutine racing with the dispatch
// goroutine on cb.SendLocalReply is safe — first-call-wins via sync.Once +
// encodeStarted gate, no race-detector hit, encode chain runs to completion.
//
// Setup: 3-filter chain [a, b, c]. a returns Continue (decode iteration
// advances). b's DecodeHeaders spawns a timer goroutine that 5ms later calls
// SendLocalReply(403, ...) from off-dispatch; b returns StopIteration so the
// dispatch goroutine parks at index 1 (b) while the timer races. The encode
// chain — running on the timer goroutine via beginLocalReply — must run all
// three encoders in reverse order (c → b → a). After the encode chain
// completes the timer fires ContinueDecoding to unpark dispatch, which sees
// localReplyDone=true and returns (false, nil).
//
// The discipline under test:
//   - chain.localReplyOnce + chain.encodeStarted are concurrency-safe across
//     the dispatch + timer goroutines (no racy read of localReplyDone);
//   - decodeResumeCh's buffered-1 + non-blocking-send tolerates the timer
//     calling ContinueDecoding even though no one is currently parked
//     (would happen if dispatch races ahead and exits first — buffered-1
//     absorbs the stale signal harmlessly).
func TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	bRec := mk("b")
	c := mk("c")
	bTimer := &timerSendLocalReplyFilter{
		recordingFilter: bRec,
		delay:           5 * time.Millisecond,
		status:          403,
		body:            "",
	}
	hf := []HTTPFilter{
		{Name: "a", Decoder: a, Encoder: a},
		{Name: "b", Decoder: bTimer, Encoder: bRec},
		{Name: "c", Decoder: c, Encoder: c},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)

	// NewFilterChain calls bTimer.SetDecoderCallbacks via the embedded
	// *recordingFilter's promoted method, which writes bRec.dcb. The timer
	// goroutine accesses the same memory through embedding promotion as
	// bTimer.dcb — no explicit copy is needed.

	terminated, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	if terminated {
		t.Fatalf("expected decode iteration aborted by SendLocalReply (terminated=false); got true")
	}

	// Encode chain must have run on every filter exactly once (full reverse
	// iteration including b — the calling filter — per ADR-0075 (d)). The
	// race is between the timer goroutine driving the encode chain and any
	// hypothetical concurrent dispatch-side encode entry; the encodeStarted
	// gate + sync.Once make first-call-wins safe across goroutines.
	if a.encodeHeaders.Load() != 1 {
		t.Fatalf("expected a.EncodeHeaders called once; got %d", a.encodeHeaders.Load())
	}
	if bRec.encodeHeaders.Load() != 1 {
		t.Fatalf("expected b.EncodeHeaders called once; got %d", bRec.encodeHeaders.Load())
	}
	if c.encodeHeaders.Load() != 1 {
		t.Fatalf("expected c.EncodeHeaders called once; got %d", c.encodeHeaders.Load())
	}
	// Decode side: a + b ran; c did NOT (decode aborted at b's SendLocalReply).
	if a.decodeHeaders.Load() != 1 {
		t.Fatalf("expected a.DecodeHeaders called once; got %d", a.decodeHeaders.Load())
	}
	if bRec.decodeHeaders.Load() != 1 {
		t.Fatalf("expected b.DecodeHeaders called once; got %d", bRec.decodeHeaders.Load())
	}
	if c.decodeHeaders.Load() != 0 {
		t.Fatalf("expected c.DecodeHeaders NOT called (decode aborted at b); got %d", c.decodeHeaders.Load())
	}
}

// TestChain_DestroyVsInFlightContinueEncoding asserts SPEC §14.10 bullet 4 +
// the OnDestroy semantics: a filter that races against chain teardown — i.e.,
// calls cb.ContinueEncoding AFTER chain.Destroy has fired — must NOT panic.
// The buffered-1 + non-blocking-send pattern on encodeResumeCh ensures the
// stale send is silently absorbed (first call fills the buffer; subsequent
// calls hit the default arm and drop). The channel is intentionally NEVER
// closed by Destroy (closing would panic any in-flight sender), so the
// observable invariant is: no panic + the dispatch goroutine has already
// returned (chain teardown happens after iteration completes per §5.7).
//
// We exercise both the once-after-Destroy case (single send into the
// post-iteration channel) AND the multiple-after-Destroy case (subsequent
// sends drop via the default arm). The race-tested concurrency model is
// asserted by N=8 goroutines each calling ContinueEncoding from off-dispatch
// after Destroy has fired — under -race no synchronization issue surfaces
// (the channel itself + the recordingFilter's atomic counters are the only
// shared state, both lock-free safe).
func TestChain_DestroyVsInFlightContinueEncoding(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	b := mk("b")
	chain, _ := newChainOf(a, b)
	chain.SetRequestCtx(context.Background(), 0)

	// Drive an encode pass so encoderCB callbacks are wired and the chain has
	// completed iteration. After this returns, the dispatch goroutine is gone.
	terminated, err := chain.RunEncodeHeaders(context.Background(), http.Header{}, true)
	if err != nil {
		t.Fatalf("RunEncodeHeaders: %v", err)
	}
	if !terminated {
		t.Fatalf("expected encode iteration to complete")
	}

	// Tear down the chain. After Destroy returns, OnDestroy has fired on every
	// filter. A slow filter that lost the race against teardown (e.g., a timer
	// goroutine that wakes up post-OnDestroy) might still call ContinueEncoding;
	// this MUST be a no-op, not a panic.
	chain.Destroy()
	if a.destroyed.Load() != 1 || b.destroyed.Load() != 1 {
		t.Fatalf("expected OnDestroy to fire on chain.Destroy; a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}

	// Now race N goroutines all calling ContinueEncoding from off-dispatch
	// after Destroy. Each call must:
	//   (a) not panic (channel is not closed by Destroy);
	//   (b) be silently absorbed by the buffered-1 + non-blocking-send pattern
	//       (first send fills the unread buffer; rest hit the default arm).
	const N = 8
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ContinueEncoding after Destroy panicked: %v", r)
		}
	}()
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			// Defensive recover inside the goroutine too — a panic here would
			// crash the test process before the outer recover fires.
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ContinueEncoding goroutine panicked: %v", r)
				}
			}()
			b.ecb.ContinueEncoding()
		}()
	}
	doneAll := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneAll)
	}()
	select {
	case <-doneAll:
	case <-time.After(2 * time.Second):
		t.Fatalf("ContinueEncoding goroutines leaked after Destroy — non-blocking-send default arm regressed")
	}

	// Calling Destroy again must be idempotent (sync.Once guard).
	chain.Destroy()
	if a.destroyed.Load() != 1 || b.destroyed.Load() != 1 {
		t.Fatalf("expected Destroy to be idempotent (each filter's OnDestroy called exactly once); a=%d b=%d", a.destroyed.Load(), b.destroyed.Load())
	}

	// Final ContinueEncoding after the second Destroy — still must not panic.
	b.ecb.ContinueEncoding()
}

// bufferOnceFilter is a decode-side filter that returns DataStopIterationAndBuffer
// the first time DecodeData is called. Used to drive the buffer-overflow path on
// RunDecodeData: a chunk that exceeds filterBufferLimitBytes synthesizes a 413
// local reply per ADR-0076 + SPEC §11 #3.
type bufferOnceFilter struct {
	*recordingFilter
}

func (b *bufferOnceFilter) DecodeData(d []byte, end bool) FilterDataStatus {
	b.recordingFilter.DecodeData(d, end)
	return DataStopIterationAndBuffer
}

// captureRecorder is an Encoder wrapper that captures both the encoded
// http.Header map and the encoded body bytes as beginLocalReply passes them
// down the encode chain. Used by TestChain_DecodeData_OverflowSynthesizes413
// to assert the verbatim 413 wire shape (headers + body).
type captureRecorder struct {
	f             *recordingFilter
	capturedHdr   *http.Header
	capturedBody  *[]byte
	mu            *sync.Mutex
}

func (c captureRecorder) EncodeHeaders(hd http.Header, end bool) FilterHeadersStatus {
	c.mu.Lock()
	cp := make(http.Header, len(hd))
	for k, vs := range hd {
		cp[k] = append([]string(nil), vs...)
	}
	*c.capturedHdr = cp
	c.mu.Unlock()
	return c.f.EncodeHeaders(hd, end)
}
func (c captureRecorder) EncodeData(d []byte, end bool) FilterDataStatus {
	c.mu.Lock()
	body := make([]byte, len(d))
	copy(body, d)
	*c.capturedBody = body
	c.mu.Unlock()
	return c.f.EncodeData(d, end)
}
func (c captureRecorder) EncodeTrailers(t http.Header) FilterTrailersStatus {
	return c.f.EncodeTrailers(t)
}
func (c captureRecorder) SetEncoderCallbacks(cb EncoderFilterCallbacks) {
	c.f.SetEncoderCallbacks(cb)
}
func (c captureRecorder) OnDestroy() { c.f.OnDestroy() }

// TestChain_DecodeData_OverflowSynthesizes413 asserts ADR-0076 + SPEC §11 #3
// empirical pin: when a decode-side filter returns DataStopIterationAndBuffer
// and the per-stream body buffer would exceed filterBufferLimitBytes, the
// chain synthesizes a 413 local reply with the verbatim wire shape:
//   - body == "Payload Too Large" (17 bytes ASCII; no trailing newline);
//   - Content-Length: 17;
//   - Content-Type: text/plain (framework default — no user override);
//   - Connection: close (framework-injected per the 413 path).
//
// Status code is consumed by the HCM wire-write layer (Task 13/15) and is not
// observable at the chain layer; the assertion proxies it via
// `chain.localReplyDone == true` (the SendLocalReply guard fired).
//
// The Date and Server headers are NOT framework-injected (per ADR-0075 (b) +
// the empirical pin's footnote: those land on the wire-write path) — only the
// three observable headers above are asserted on the encode-chain capture.
func TestChain_DecodeData_OverflowSynthesizes413(t *testing.T) {
	var capturedHdr http.Header
	var capturedBody []byte
	var capMu sync.Mutex

	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	bufRec := mk("buf")
	router := mk("router")
	bufferer := &bufferOnceFilter{recordingFilter: bufRec}
	hf := []HTTPFilter{
		{Name: "buf", Decoder: bufferer, Encoder: captureRecorder{f: bufRec, capturedHdr: &capturedHdr, capturedBody: &capturedBody, mu: &capMu}},
		{Name: "router", Decoder: router, Encoder: captureRecorder{f: router, capturedHdr: &capturedHdr, capturedBody: &capturedBody, mu: &capMu}},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)

	// Drive RunDecodeHeaders first (decode iteration cursor must advance to the
	// data phase) then RunDecodeData with a chunk larger than filterBufferLimitBytes.
	if _, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, false); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	// The cursor sits at len(filters) after RunDecodeHeaders completes (both
	// filters returned Continue on headers). For RunDecodeData the cursor must
	// re-enter at index 0; the production code resets the cursor at the top of
	// RunDecodeData (mirrors the encode-side reset in RunEncodeData).
	chunk := make([]byte, filterBufferLimitBytes+1)
	for i := range chunk {
		chunk[i] = 'A'
	}
	terminated, err := chain.RunDecodeData(context.Background(), chunk, true)
	if err != nil {
		t.Fatalf("RunDecodeData: %v", err)
	}
	if terminated {
		t.Fatalf("expected RunDecodeData to abort iteration via 413 synthesis (terminated=false); got true")
	}

	// localReplyDone is the framework's signal that a SendLocalReply (synthesized
	// or otherwise) fired.
	if !chain.localReplyDone.Load() {
		t.Fatalf("expected localReplyDone=true after overflow-triggered 413 synthesis")
	}

	// Verify the encode chain ran with the verbatim 413 wire shape.
	capMu.Lock()
	gotHdr := capturedHdr
	gotBody := capturedBody
	capMu.Unlock()
	if gotHdr == nil {
		t.Fatalf("expected encode chain to have run + captured headers; got nil")
	}

	// Body: 17 bytes, exact ASCII "Payload Too Large", no trailing newline.
	wantBody := "Payload Too Large"
	if len(gotBody) != 17 {
		t.Fatalf("expected body length == 17; got %d", len(gotBody))
	}
	if string(gotBody) != wantBody {
		t.Fatalf("expected body == %q; got %q", wantBody, string(gotBody))
	}
	if bytes.HasSuffix(gotBody, []byte{'\n'}) {
		t.Fatalf("expected body to have NO trailing newline; got %q", string(gotBody))
	}

	// Content-Length: 17.
	if cl := gotHdr.Get("Content-Length"); cl != "17" {
		t.Fatalf("expected Content-Length: 17; got %q", cl)
	}
	// Content-Type: text/plain (framework default).
	if ct := gotHdr.Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("expected Content-Type: text/plain; got %q", ct)
	}
	// Connection: close (framework-injected on the 413 path).
	if cn := gotHdr.Get("Connection"); cn != "close" {
		t.Fatalf("expected Connection: close on 413 synthesis; got %q", cn)
	}
}

// TestChain_DecodeData_BelowCapDoesNotSynthesize asserts the body-cap-respected-
// on-non-overflow path: a chunk that does NOT exceed filterBufferLimitBytes is
// buffered (decodeBuf accumulates) and parkDecode is invoked; once
// ContinueDecoding fires the iteration advances and NO 413 is synthesized.
func TestChain_DecodeData_BelowCapDoesNotSynthesize(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			headersStatus:     Continue,
			dataStatus:        DataContinue,
			trailersStatus:    TrailersContinue,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	bufRec := mk("buf")
	router := mk("router")
	bufferer := &bufferOnceFilter{recordingFilter: bufRec}
	hf := []HTTPFilter{
		{Name: "buf", Decoder: bufferer, Encoder: bufRec},
		{Name: "router", Decoder: router, Encoder: router},
	}
	chain := NewFilterChain(hf, nil)
	chain.SetRequestCtx(context.Background(), 0)

	if _, err := chain.RunDecodeHeaders(context.Background(), http.Header{}, false); err != nil {
		t.Fatalf("RunDecodeHeaders: %v", err)
	}
	// Async resume: park happens because bufferer returns DataStopIterationAndBuffer;
	// 20ms later we fire ContinueDecoding to unblock the dispatch goroutine.
	go func() {
		time.Sleep(20 * time.Millisecond)
		bufferer.dcb.ContinueDecoding()
	}()
	// Chunk well under the cap (1024 bytes).
	chunk := make([]byte, 1024)
	for i := range chunk {
		chunk[i] = 'B'
	}
	terminated, err := chain.RunDecodeData(context.Background(), chunk, true)
	if err != nil {
		t.Fatalf("RunDecodeData: %v", err)
	}
	if !terminated {
		t.Fatalf("expected RunDecodeData to complete after async resume (terminated=true); got false")
	}
	// No 413 synthesis: localReplyDone must NOT be set.
	if chain.localReplyDone.Load() {
		t.Fatalf("expected localReplyDone=false on under-cap chunk; got true")
	}
	// decodeBuf must have accumulated the chunk (1024 bytes).
	if len(chain.decodeBuf) != 1024 {
		t.Fatalf("expected decodeBuf length 1024; got %d", len(chain.decodeBuf))
	}
	// router's decode side must have run (iteration advanced past the buffering filter).
	if router.decodeData.Load() != 1 {
		t.Fatalf("expected router.DecodeData called once after resume; got %d", router.decodeData.Load())
	}
}

// TestChain_EncodeData_OverflowReturnsSentinel asserts ADR-0076 encode-side
// overflow path: RunEncodeData accumulates encodeBufLen across calls; when a
// chunk would push the total above filterBufferLimitBytes the chain returns
// errEncodeBufferOverflow without iterating further encode-side filters on
// that chunk. The HCM dispatch path resets the connection (H1 close, H2
// RST_STREAM) — handled in Tasks 15 + 16.
func TestChain_EncodeData_OverflowReturnsSentinel(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	chain, _ := newChainOf(a)

	// First call: a chunk exactly at the cap. This should NOT overflow yet
	// (encodeBufLen+len(data) == filterBufferLimitBytes; the check is "> cap",
	// not ">= cap" per the PLAN scaffold).
	first := make([]byte, filterBufferLimitBytes)
	terminated, err := chain.RunEncodeData(context.Background(), first, false)
	if err != nil {
		t.Fatalf("RunEncodeData(first): %v", err)
	}
	if !terminated {
		t.Fatalf("expected first chunk to complete iteration (terminated=true); got false")
	}

	// Second call: ONE more byte would push us over the cap. Sentinel must be
	// returned at exactly this boundary.
	overflow := make([]byte, 1)
	terminated, err = chain.RunEncodeData(context.Background(), overflow, true)
	if err == nil {
		t.Fatalf("expected errEncodeBufferOverflow on encode-side overflow; got nil")
	}
	if !errors.Is(err, errEncodeBufferOverflow) {
		t.Fatalf("expected errEncodeBufferOverflow sentinel; got %v", err)
	}
	if terminated {
		t.Fatalf("expected terminated=false on overflow sentinel; got true")
	}
}

// TestChain_EncodeData_BelowCapNoSentinel asserts the symmetric-with-decode
// case: encode-side chunks that stay within the cap iterate normally and do
// NOT return the overflow sentinel.
func TestChain_EncodeData_BelowCapNoSentinel(t *testing.T) {
	mk := func(name string) *recordingFilter {
		return &recordingFilter{
			name:              name,
			encHeadersStatus:  Continue,
			encDataStatus:     DataContinue,
			encTrailersStatus: TrailersContinue,
		}
	}
	a := mk("a")
	chain, _ := newChainOf(a)
	// Three small chunks well under the cap.
	for i := 0; i < 3; i++ {
		terminated, err := chain.RunEncodeData(context.Background(), make([]byte, 1024), i == 2)
		if err != nil {
			t.Fatalf("RunEncodeData(i=%d): %v", i, err)
		}
		if !terminated {
			t.Fatalf("expected each under-cap chunk to complete (terminated=true); got false on i=%d", i)
		}
	}
	if a.encodeData.Load() != 3 {
		t.Fatalf("expected a.EncodeData called 3 times; got %d", a.encodeData.Load())
	}
}
