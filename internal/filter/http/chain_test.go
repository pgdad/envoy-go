package http

import (
	"bytes"
	"context"
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
	headers http.Header
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
		headers:         http.Header{"content-type": []string{"application/json"}},
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
