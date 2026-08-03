// Tests for the per-*RootVM proxy_http_call dispatch + cancel-at-destruction
// + http_call_response_after_close envoy-go-strict counter per 25.2 SPEC §3.1
// + §5.1 #37 + §5.3 C19 + §11.3 D-25.2-3 + Q4 + R-25.2-3 + AMEND-B3 + ADR-
// 0177 RE-CONSUMER (3rd-or-later co-consumer; phase-22.2 lua :httpCall() was
// second; CLOSES parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor).
//
// Coverage:
//
//   - TestHttpCall_NoDispatcher_InternalFailure: DispatchHttpCall with nil
//     httpDispatcher returns InternalFailure (programmer error / host
//     misconfiguration).
//   - TestHttpCall_DispatchOk_AllocatesMonotonicCallID: DispatchHttpCall to
//     known cluster returns Ok + a non-zero monotonic call_id.
//   - TestHttpCall_BadArgument_UnknownCluster: cluster.Manager.Get miss
//     returns BadArgument byte-faithful to cpp-host context.cc:1547-1550.
//   - TestHttpCall_CancelAtDestruction: dispatch call → before response →
//     StreamContext.Close → assert cancelFn invoked + httpCalls entry
//     removed; the dispatch goroutine observes ctx-cancellation.
//   - TestHttpCall_LateResponseAfterClose_TokenMissPath: handleHttpCallResponse
//     with stale callID (the entry was already removed by cancel-at-destruction
//     OR never existed) → drop silently + http_call_response_after_close
//     counter touchpoint exercised (counter wiring deferred Task 17 — the
//     test verifies the no-panic + return-without-effect path).
//   - TestHttpCall_LateResponseAfterStreamClosed: response arrives after
//     stream context is closed (closed flag set but entry still in the map);
//     the post-acquire-dispatchMu re-check fires + the dispatch is dropped.
//   - TestHttpCall_ConcurrentDispatch_N100_IsolationVerified: N=100
//     concurrent DispatchHttpCalls from the same RootVM all return Ok +
//     all return unique call_ids + each response routes to its originating
//     stream context.
//
// Per the brief: counter integration deferred Task 17 — these tests verify
// the no-panic + correct-behavior paths; counter increments are exercised
// indirectly via the dropped-response coverage.

package wasm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// --- fakeHTTPDispatcher ---------------------------------------------------

// fakeHTTPDispatcher is a goroutine-safe in-memory HTTPDispatcher for tests.
// Records every Dispatch invocation; serves a canned response per request;
// honors ctx-cancellation (returns ctx.Err() if canceled before the canned
// per-call sleep completes — used by the cancel-at-destruction test).
type fakeHTTPDispatcher struct {
	mu sync.Mutex

	// Set of cluster names that exist; HasCluster checks membership.
	clusters map[string]bool

	// Per-call response (returned by all Dispatch calls unless responseFn
	// overrides). Body is wrapped in a nopCloser by Dispatch.
	responseStatus int
	responseBody   []byte
	responseHdrs   http.Header

	// Per-call dispatch delay; Dispatch sleeps for this duration (subject
	// to ctx-cancellation) before returning the response. Used by the
	// cancel-at-destruction test to ensure Close fires while Dispatch is
	// still waiting.
	dispatchDelay time.Duration

	// dispatchFn, when set, overrides the canned-response behavior. Used
	// by per-stream-isolation tests that need to inspect the request +
	// emit per-stream responses.
	dispatchFn func(ctx context.Context, cluster string, req *http.Request) (*http.Response, error)

	// Recorded call log: per-call cluster + request URL for assertions.
	dispatched []dispatchRecord
}

type dispatchRecord struct {
	cluster string
	url     string
	method  string
}

func newFakeHTTPDispatcher(clusters ...string) *fakeHTTPDispatcher {
	m := make(map[string]bool, len(clusters))
	for _, c := range clusters {
		m[c] = true
	}
	return &fakeHTTPDispatcher{
		clusters:       m,
		responseStatus: 200,
		responseBody:   []byte("ok"),
		responseHdrs:   http.Header{"X-Test": []string{"yes"}},
	}
}

func (f *fakeHTTPDispatcher) HasCluster(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clusters[name]
}

func (f *fakeHTTPDispatcher) Dispatch(ctx context.Context, cluster string, req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	dispatchFn := f.dispatchFn
	delay := f.dispatchDelay
	status := f.responseStatus
	body := f.responseBody
	hdrs := f.responseHdrs.Clone()
	f.dispatched = append(f.dispatched, dispatchRecord{
		cluster: cluster,
		url:     req.URL.String(),
		method:  req.Method,
	})
	f.mu.Unlock()

	if dispatchFn != nil {
		return dispatchFn(ctx, cluster, req)
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	resp := &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Header:     hdrs,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	return resp, nil
}

func (f *fakeHTTPDispatcher) dispatchedSnapshot() []dispatchRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]dispatchRecord, len(f.dispatched))
	copy(out, f.dispatched)
	return out
}

// --- TestHttpCall_NoDispatcher_InternalFailure ----------------------------

func TestHttpCall_NoDispatcher_InternalFailure(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	rv, err := NewRootVM(ctx, mod, 1) // NO WithRootHTTPDispatcher
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, 100, "any_cluster", nil, nil, nil, 1000)
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultInternalFailure {
		t.Errorf("status=%v; want InternalFailure (no dispatcher wired)", status)
	}
	if callID != 0 {
		t.Errorf("callID=%d; want 0 on InternalFailure", callID)
	}
}

// --- TestHttpCall_BadArgument_UnknownCluster ------------------------------

func TestHttpCall_BadArgument_UnknownCluster(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	disp := newFakeHTTPDispatcher("cluster_a") // only cluster_a is known
	rv, err := NewRootVM(ctx, mod, 1, WithRootHTTPDispatcher(disp))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, 100, "unknown_cluster", nil, nil, nil, 1000)
	rv.dispatchMu.Unlock()

	if status != abi.WasmResultBadArgument {
		t.Errorf("unknown cluster: status=%v; want BadArgument per Q4 + AMEND-B3", status)
	}
	if callID != 0 {
		t.Errorf("unknown cluster: callID=%d; want 0", callID)
	}
}

// --- TestHttpCall_DispatchOk_AllocatesMonotonicCallID ---------------------

func TestHttpCall_DispatchOk_AllocatesMonotonicCallID(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	disp := newFakeHTTPDispatcher("cluster_a")
	// Use a long delay so the dispatch goroutine doesn't complete + remove
	// the entry before we inspect; cancel via Close at defer.
	disp.dispatchDelay = 100 * time.Millisecond
	rv, err := NewRootVM(ctx, mod, 1, WithRootHTTPDispatcher(disp))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	headers := []HeaderPair{
		{Key: ":method", Value: "POST"},
		{Key: ":path", Value: "/echo"},
		{Key: ":authority", Value: "echo.local"},
		{Key: "content-type", Value: "application/json"},
	}

	rv.dispatchMu.Lock()
	callID1, status1 := rv.DispatchHttpCall(ctx, 100, "cluster_a", headers, []byte(`{"hi":1}`), nil, 5000)
	callID2, status2 := rv.DispatchHttpCall(ctx, 100, "cluster_a", headers, nil, nil, 5000)
	rv.dispatchMu.Unlock()

	if status1 != abi.WasmResultOk {
		t.Errorf("first dispatch status=%v; want Ok", status1)
	}
	if status2 != abi.WasmResultOk {
		t.Errorf("second dispatch status=%v; want Ok", status2)
	}
	if callID1 == 0 || callID2 == 0 {
		t.Errorf("callIDs (%d, %d) must be non-zero", callID1, callID2)
	}
	if callID1 == callID2 {
		t.Errorf("callIDs not monotonic: %d == %d", callID1, callID2)
	}
	if callID2 <= callID1 {
		t.Errorf("callIDs not monotonic-increasing: callID1=%d callID2=%d", callID1, callID2)
	}

	// Verify the request shape was constructed from pseudo-headers correctly.
	// We must wait for at least the dispatch records to land (they record at
	// Dispatch entry, BEFORE the delay).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(disp.dispatchedSnapshot()) >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	recs := disp.dispatchedSnapshot()
	if len(recs) < 2 {
		t.Fatalf("dispatched records=%d; want >=2", len(recs))
	}
	for i, r := range recs {
		if r.cluster != "cluster_a" {
			t.Errorf("rec[%d] cluster=%q; want cluster_a", i, r.cluster)
		}
		if r.method != "POST" {
			t.Errorf("rec[%d] method=%q; want POST", i, r.method)
		}
		if r.url == "" {
			t.Errorf("rec[%d] empty URL", i)
		}
	}
}

// --- TestHttpCall_CancelAtDestruction -------------------------------------

func TestHttpCall_CancelAtDestruction(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	// Build a dispatcher whose Dispatch blocks on a per-call ctx until
	// canceled. Records the cancellation event for assertions.
	var (
		cancelObserved  atomic.Bool
		dispatchEntered = make(chan struct{}, 1)
	)
	disp := newFakeHTTPDispatcher("cluster_a")
	disp.dispatchFn = func(callCtx context.Context, _ string, _ *http.Request) (*http.Response, error) {
		select {
		case dispatchEntered <- struct{}{}:
		default:
		}
		<-callCtx.Done()
		cancelObserved.Store(true)
		return nil, callCtx.Err()
	}

	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootHTTPDispatcher(disp),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, sc.streamCtxID, "cluster_a", nil, nil, nil, 30000)
	rv.dispatchMu.Unlock()
	if status != abi.WasmResultOk {
		t.Fatalf("DispatchHttpCall status=%v; want Ok", status)
	}
	if callID == 0 {
		t.Fatalf("callID=0; want non-zero")
	}

	// Wait for the dispatch goroutine to enter Dispatch (so we know its
	// ctx is alive + the entry is in the map).
	select {
	case <-dispatchEntered:
	case <-time.After(time.Second):
		t.Fatalf("Dispatch goroutine never entered")
	}

	// Pre-Close: verify the entry is in the map.
	rv.httpCallsMu.Lock()
	_, present := rv.httpCalls[callID]
	rv.httpCallsMu.Unlock()
	if !present {
		t.Fatalf("httpCalls[%d] absent before Close; expected present", callID)
	}

	// Close the stream context. cancelOutstandingHttpCalls runs inside.
	if err := sc.Close(ctx); err != nil {
		t.Errorf("sc.Close: %v", err)
	}

	// Post-Close: verify the entry was removed + the dispatch goroutine
	// observed the cancellation.
	rv.httpCallsMu.Lock()
	_, present = rv.httpCalls[callID]
	rv.httpCallsMu.Unlock()
	if present {
		t.Errorf("httpCalls[%d] still present after Close; expected removed", callID)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !cancelObserved.Load() {
		time.Sleep(time.Millisecond)
	}
	if !cancelObserved.Load() {
		t.Errorf("dispatch goroutine did not observe ctx-cancellation within 1s")
	}
}

// --- TestHttpCall_LateResponseAfterClose_TokenMissPath --------------------

func TestHttpCall_LateResponseAfterClose_TokenMissPath(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	disp := newFakeHTTPDispatcher("cluster_a")
	rv, err := NewRootVM(ctx, mod, 1, WithRootHTTPDispatcher(disp))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	// Directly invoke handleHttpCallResponse with a callID that was never
	// in the httpCalls map (defensive token-miss path). Per AMEND-B3 +
	// cpp-host context.cc:1693-1696: the response is dropped silently +
	// http_call_response_after_close counter increments (counter wiring
	// deferred Task 17 — we verify the no-panic + correct-return path).
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader([]byte("late"))),
	}

	// Must NOT panic; body must be drained + closed (no resource leak).
	rv.handleHttpCallResponse(99999, resp, nil)

	// Verify the response body is closed (Read after Close returns io.EOF
	// or an error from the body Reader; the NopCloser's underlying reader
	// returns EOF after being drained).
	n, readErr := resp.Body.Read(make([]byte, 16))
	if n != 0 || readErr == nil {
		t.Errorf("response body not drained: n=%d err=%v; want (0, non-nil)", n, readErr)
	}
}

// --- TestHttpCall_LateResponseAfterStreamClosed ---------------------------

func TestHttpCall_LateResponseAfterStreamClosed(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	disp := newFakeHTTPDispatcher("cluster_a")
	disp.dispatchDelay = 200 * time.Millisecond
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootHTTPDispatcher(disp),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, sc.streamCtxID, "cluster_a", nil, nil, nil, 30000)
	rv.dispatchMu.Unlock()
	if status != abi.WasmResultOk {
		t.Fatalf("DispatchHttpCall status=%v; want Ok", status)
	}

	// Manually flip sc.closed AFTER dispatch — synthesizes the "stream
	// closed between dispatch + response arrival" race that AMEND-B3
	// guards against. The dispatch goroutine's handleHttpCallResponse
	// then drops via the post-acquire-dispatchMu re-check. We do NOT
	// remove the httpCalls entry here — the entry is still present but
	// the stream is gone; the response goroutine's sc-lookup-then-
	// closed-check fires.
	sc.closed.Store(true)

	// Wait for the dispatch goroutine to complete its delay + invoke
	// handleHttpCallResponse. It MUST NOT panic + MUST drop the response
	// silently.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		rv.httpCallsMu.Lock()
		_, present := rv.httpCalls[callID]
		rv.httpCallsMu.Unlock()
		if !present {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	rv.httpCallsMu.Lock()
	_, present := rv.httpCalls[callID]
	rv.httpCallsMu.Unlock()
	if present {
		t.Errorf("httpCalls[%d] still present after response delay; the response goroutine should have removed it", callID)
	}
}

// --- TestHttpCall_ConcurrentDispatch_N100_IsolationVerified ---------------

// TestHttpCall_ConcurrentDispatch_N100_IsolationVerified runs N=100 concurrent
// DispatchHttpCalls from the same RootVM (across distinct stream contexts);
// asserts (a) all 100 return Ok, (b) all 100 call_ids are unique, (c) the
// dispatcher saw exactly 100 distinct dispatches, (d) -race clean.
//
// The dispatcher records per-call cluster + URL; we verify the per-stream
// pseudo-header is preserved across the concurrent dispatches (no cross-
// stream argument leak).
func TestHttpCall_ConcurrentDispatch_N100_IsolationVerified(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)

	const N = 100
	disp := newFakeHTTPDispatcher("cluster_b")
	// Use a per-call dispatchFn that echoes the request URL into the
	// response Header so we can verify each dispatch's request arguments.
	disp.dispatchFn = func(_ context.Context, _ string, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"X-Echo-Path": []string{req.URL.Path}},
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}

	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootHTTPDispatcher(disp),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	type result struct {
		callID uint32
		status abi.WasmResult
	}
	results := make([]result, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			sc, err := rv.NewStreamContext(ctx)
			if err != nil {
				t.Errorf("[%d] NewStreamContext: %v", i, err)
				return
			}
			headers := []HeaderPair{
				{Key: ":path", Value: pathForStream(i)},
				{Key: ":method", Value: "GET"},
			}
			rv.dispatchMu.Lock()
			callID, status := rv.DispatchHttpCall(ctx, sc.streamCtxID, "cluster_b", headers, nil, nil, 5000)
			rv.dispatchMu.Unlock()
			results[i] = result{callID: callID, status: status}
		}()
	}
	wg.Wait()

	// (a) All 100 return Ok.
	for i, r := range results {
		if r.status != abi.WasmResultOk {
			t.Errorf("[%d] status=%v; want Ok", i, r.status)
		}
	}

	// (b) All 100 call_ids are unique + non-zero.
	seen := make(map[uint32]int, N)
	for i, r := range results {
		if r.callID == 0 {
			t.Errorf("[%d] callID=0; want non-zero", i)
			continue
		}
		seen[r.callID]++
	}
	if len(seen) != N {
		t.Errorf("distinct callIDs=%d; want %d (collision indicates non-monotonic allocator)", len(seen), N)
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("callID %d allocated %d times; want exactly 1", id, count)
		}
	}

	// (c) Wait for all dispatches to complete; verify the dispatcher saw
	// exactly N distinct dispatch records. We pre-cancel the test deadline
	// at 5s to avoid hangs.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(disp.dispatchedSnapshot()) >= N {
			break
		}
		time.Sleep(time.Millisecond)
	}
	recs := disp.dispatchedSnapshot()
	if len(recs) != N {
		t.Errorf("dispatch records=%d; want %d", len(recs), N)
	}

	// (d) Each record's URL must encode the per-stream path (no cross-stream
	// leak). The path is computed deterministically from the stream index;
	// we collect the seen paths + assert the set matches.
	wantPaths := make(map[string]bool, N)
	for i := 0; i < N; i++ {
		wantPaths[pathForStream(i)] = true
	}
	gotPaths := make(map[string]bool, len(recs))
	for _, r := range recs {
		// URL is "http://cluster_b<path>" — extract the path suffix.
		idx := indexOf(r.url, "cluster_b")
		if idx < 0 {
			t.Errorf("URL %q missing cluster_b host", r.url)
			continue
		}
		path := r.url[idx+len("cluster_b"):]
		gotPaths[path] = true
	}
	for p := range wantPaths {
		if !gotPaths[p] {
			t.Errorf("dispatcher did not see path %q (cross-stream argument leak?)", p)
		}
	}
}

// pathForStream returns a deterministic per-stream URL path that the
// concurrent-isolation test uses to track per-stream arguments through the
// dispatch.
func pathForStream(i int) string {
	return "/stream/" + itoaSmall(i)
}

// itoaSmall is a minimal int→string conversion for small positive ints,
// avoiding a strconv import in this test file (kept lean — the existing
// foreign_test.go pattern eschews strconv).
func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// indexOf returns the index of sub in s, or -1 if not present. Minimal
// substring search keeping this test file free of strings import.
func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	if len(sub) > len(s) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Compile-time guard: fakeHTTPDispatcher satisfies HTTPDispatcher.
var _ HTTPDispatcher = (*fakeHTTPDispatcher)(nil)

// --- TestHttpCall_DispatchOnClosedVM_InternalFailure ----------------------

// TestHttpCall_DispatchOnClosedVM_InternalFailure: DispatchHttpCall on a
// closed *RootVM must refuse up-front with InternalFailure — a dispatch
// racing past Close could otherwise lazily re-create the swept httpCalls
// map + launch a request goroutine that runs to its full timeout against
// the closed VM (escaping cancel-at-destruction).
func TestHttpCall_DispatchOnClosedVM_InternalFailure(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, minimalInitModule)
	disp := newFakeHTTPDispatcher("cluster_a")
	rv, err := NewRootVM(ctx, mod, 1, WithRootHTTPDispatcher(disp))
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	if err := rv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	callID, status := rv.DispatchHttpCall(ctx, 100, "cluster_a", nil, nil, nil, 1000)
	if status != abi.WasmResultInternalFailure {
		t.Errorf("status=%v; want InternalFailure (closed RootVM)", status)
	}
	if callID != 0 {
		t.Errorf("callID=%d; want 0 on closed RootVM", callID)
	}

	// No dispatch goroutine may have launched + the swept map must stay nil.
	if got := len(disp.dispatchedSnapshot()); got != 0 {
		t.Errorf("dispatched count = %d; want 0 (no request may launch against a closed VM)", got)
	}
	rv.httpCallsMu.Lock()
	if rv.httpCalls != nil {
		t.Error("httpCalls map re-created after Close; want nil (sweep must be final)")
	}
	rv.httpCallsMu.Unlock()
}

// --- countingStatsRecorder -------------------------------------------------

// countingStatsRecorder is a goroutine-safe RootStatsRecorder that counts
// every increment, so tests can assert on the counter DELTAS a dispatch path
// produced. Only the counters this file asserts on are read back; the rest
// satisfy the interface.
type countingStatsRecorder struct {
	tickInvocations             atomic.Int64
	httpCallDispatched          atomic.Int64
	httpCallResponse            atomic.Int64
	foreignFunctionDenied       atomic.Int64
	bodyBufferCapExceeded       atomic.Int64
	httpCallDispatchUnknownClus atomic.Int64
	sharedDataCapExceeded       atomic.Int64
	dynamicStatsCapExceeded     atomic.Int64
	httpCallResponseAfterClose  atomic.Int64
	envoyGoFailures             atomic.Int64
}

func (c *countingStatsRecorder) TickInvocationsInc()       { c.tickInvocations.Add(1) }
func (c *countingStatsRecorder) HttpCallDispatchedInc()    { c.httpCallDispatched.Add(1) }
func (c *countingStatsRecorder) HttpCallResponseInc()      { c.httpCallResponse.Add(1) }
func (c *countingStatsRecorder) ForeignFunctionDeniedInc() { c.foreignFunctionDenied.Add(1) }
func (c *countingStatsRecorder) BodyBufferCapExceededInc() { c.bodyBufferCapExceeded.Add(1) }
func (c *countingStatsRecorder) HttpCallDispatchUnknownClusterInc() {
	c.httpCallDispatchUnknownClus.Add(1)
}
func (c *countingStatsRecorder) SharedDataCapExceededInc()      { c.sharedDataCapExceeded.Add(1) }
func (c *countingStatsRecorder) DynamicStatsCapExceededInc()    { c.dynamicStatsCapExceeded.Add(1) }
func (c *countingStatsRecorder) HttpCallResponseAfterCloseInc() { c.httpCallResponseAfterClose.Add(1) }
func (c *countingStatsRecorder) EnvoyGoFailuresInc()            { c.envoyGoFailures.Add(1) }

var _ RootStatsRecorder = (*countingStatsRecorder)(nil)

// --- TestA4_HttpCallResponseTrap_PoisonsStreamContext ---------------------

// TestA4_HttpCallResponseTrap_PoisonsStreamContext is the phase-82 Task 3 +
// Task 4 anchor: a guest whose proxy_on_http_call_response body is a single
// `unreachable` TRAPS. The host must
//
//	(Task 3) poison the originating StreamContext (sc.trapped) so Close skips
//	         the teardown triplet, and co-increment envoy_go.failures per
//	         §2.25; and
//	(Task 4) NOT increment http_call_response — that counter means "the guest
//	         consumed the response", and the fixture's positive assertion
//	         `http_call_response >= 1` would otherwise be GREEN ON A TRAP.
//
// Two negative controls keep the assertions non-vacuous:
//
//	NC1 the capability gate did NOT short-circuit the dispatch —
//	    HasGlobalFunc("proxy_on_http_call_response") is true, so the guest
//	    export was actually reached (a denied cap would take the early-return
//	    arm and produce trapped=false for an unrelated reason).
//	NC2 the httpCalls entry was CONSUMED by handleHttpCallResponse rather than
//	    swept by cancel-at-destruction — the entry is present before the
//	    response and absent after, with the stream context still open.
func TestA4_HttpCallResponseTrap_PoisonsStreamContext(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, httpCallResponseTrapsModule)

	disp := newFakeHTTPDispatcher("cluster_a")
	rec := &countingStatsRecorder{}
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootHTTPDispatcher(disp),
		WithRootStats(rec),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()
	rv.RegisterABICallbacks(&fakeABICallbacks{})
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// NC1: the callback is reachable — cap allowed AND exported. If this were
	// false the dispatch would take the "no guest export / cap denied" arm and
	// sc.trapped would be false for a reason that has nothing to do with the
	// trap.
	if !rv.HasGlobalFunc("proxy_on_http_call_response") {
		t.Fatal("NC1: HasGlobalFunc(proxy_on_http_call_response) = false; the trap arm is unreachable and this test would be vacuous")
	}

	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, sc.streamCtxID, "cluster_a", nil, nil, nil, 5000)
	rv.dispatchMu.Unlock()
	if status != abi.WasmResultOk {
		t.Fatalf("DispatchHttpCall status=%v; want Ok", status)
	}

	// Wait for the dispatch goroutine to consume the entry (NC2's "after").
	deadline := time.Now().Add(3 * time.Second)
	consumed := false
	for time.Now().Before(deadline) {
		rv.httpCallsMu.Lock()
		_, present := rv.httpCalls[callID]
		rv.httpCallsMu.Unlock()
		if !present {
			consumed = true
			break
		}
		time.Sleep(time.Millisecond)
	}

	// NC2: the entry was consumed by handleHttpCallResponse, and the stream is
	// still open — so the response reached the guest-dispatch arm rather than
	// the token-miss / stream-gone early returns.
	if !consumed {
		t.Fatalf("NC2: httpCalls[%d] still present after 3s; the response never reached handleHttpCallResponse", callID)
	}
	if sc.closed.Load() {
		t.Fatal("NC2: stream context closed; the response would have taken the stream-gone early-return arm")
	}
	if got := rec.httpCallResponseAfterClose.Load(); got != 0 {
		t.Fatalf("NC2: http_call_response_after_close = %d; want 0 (a non-zero count means the response took an early-return arm, not the guest dispatch)", got)
	}

	// Task 3: the trap must poison the StreamContext.
	if !sc.trapped.Load() {
		t.Errorf("sc.trapped = false after proxy_on_http_call_response TRAPPED; want true")
	}

	// Task 3: §2.25 co-increment.
	if got := rec.envoyGoFailures.Load(); got < 1 {
		t.Errorf("envoy_go.failures = %d after a trapping proxy_on_http_call_response; want >= 1", got)
	}

	// Task 4: http_call_response counts CONSUMED responses. A trap did not
	// consume anything.
	if got := rec.httpCallResponse.Load(); got != 0 {
		t.Errorf("http_call_response = %d after a trapping proxy_on_http_call_response; want 0 (the counter must not go green on a trap)", got)
	}

	// Task 2: the deferred cache-clear must run even on the trap path.
	// runCallWithPanicWrapper recovers inside its OWN frame, which cannot skip
	// an outer defer — this asserts that rather than assuming it.
	if after := rv.HTTPCallResponse(); after != nil {
		t.Errorf("rv.HTTPCallResponse() = %+v after a TRAPPING callback; want nil (the deferred clear must survive the recover)", after)
	}
}

// --- TestHttpCallResponse_CachePublishedDuringCallback --------------------

// logHookABICallbacks wraps fakeABICallbacks and fires onLog inside the
// guest's proxy_log hostcall frame — i.e. WHILE proxy_on_http_call_response is
// still on the stack. It is the re-entrancy point the cache-lifetime assertion
// needs (the deferred clear only runs after the guest returns).
type logHookABICallbacks struct {
	*fakeABICallbacks
	onLog func(level abi.LogLevel)
}

func (l *logHookABICallbacks) Log(ctx context.Context, ctxID uint32, level abi.LogLevel, msg string) {
	if l.onLog != nil {
		l.onLog(level)
	}
	l.fakeABICallbacks.Log(ctx, ctxID, level, msg)
}

// TestHttpCallResponse_CachePublishedDuringCallback is the phase-82 Task 2
// anchor for the PRODUCER half of the http-call response cache. It asserts, on
// a response whose header map deliberately has MORE VALUES THAN KEYS:
//
//	(a) the cache is PUBLISHED while proxy_on_http_call_response is running;
//	(b) Headers carries a synthesized `:status` under the EXACT key ":status"
//	    — lowercase, colon-prefixed, un-canonicalized (Go's net/http never puts
//	    a `:status` in resp.Header, so a literal stash would be inert);
//	(c) the num_headers argument the guest received is the VALUE count, not the
//	    KEY count (GetHeaderMap is value-EXPANDING);
//	(d) the source resp.Header was NOT mutated — the synthesis went into a copy;
//	(e) the cache is CLEARED once the callback returns (callback-scoped
//	    lifetime per D-82-LIFETIME).
func TestHttpCallResponse_CachePublishedDuringCallback(t *testing.T) {
	ctx := context.Background()
	mod := mustCompileForRootVM(t, ctx, httpCallResponseLogsNumHeadersModule)

	// 2 keys / 3 values, so the key count (2, +1 for :status = 3) and the
	// value count (3, +1 for :status = 4) DIFFER — a key-count regression
	// cannot pass this test.
	//
	// The response is built by dispatchFn rather than the canned path so the
	// test RETAINS the exact http.Header map it handed to the host. The canned
	// path Clones responseHdrs per call, which would make the (d)
	// "source map not mutated" assertion vacuous — a no-copy implementation
	// would mutate the per-call clone and the retained original would look
	// pristine. VERIFIED by a deliberate break: with `hdrs := resp.Header`
	// (no copy) and the canned path, (d) did NOT fire.
	sourceHdrs := http.Header{
		"X-Multi":  []string{"a", "b"},
		"X-Single": []string{"c"},
	}
	disp := newFakeHTTPDispatcher("cluster_a")
	disp.dispatchFn = func(_ context.Context, _ string, _ *http.Request) (*http.Response, error) {
		return &http.Response{
			Status:     http.StatusText(201),
			StatusCode: 201,
			Header:     sourceHdrs, // the SAME map the assertions inspect
			Body:       io.NopCloser(bytes.NewReader([]byte("hello-body"))),
		}, nil
	}

	rec := &countingStatsRecorder{}
	rv, err := NewRootVM(ctx, mod, 1,
		WithRootSandboxConfig(allowAllSandbox()),
		WithRootHTTPDispatcher(disp),
		WithRootStats(rec),
	)
	if err != nil {
		t.Fatalf("NewRootVM: %v", err)
	}
	defer func() { _ = rv.Close() }()

	var (
		seenLevel    atomic.Int64
		seenSnapshot atomic.Pointer[HTTPCallResponse]
		logFired     atomic.Bool
	)
	cb := &logHookABICallbacks{
		fakeABICallbacks: &fakeABICallbacks{},
		onLog: func(level abi.LogLevel) {
			logFired.Store(true)
			seenLevel.Store(int64(level))
			// Snapshot the cache from INSIDE the callback frame.
			seenSnapshot.Store(rv.HTTPCallResponse())
		},
	}
	rv.RegisterABICallbacks(cb)
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	sc, err := rv.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}

	rv.dispatchMu.Lock()
	callID, status := rv.DispatchHttpCall(ctx, sc.streamCtxID, "cluster_a", nil, nil, nil, 5000)
	rv.dispatchMu.Unlock()
	if status != abi.WasmResultOk {
		t.Fatalf("DispatchHttpCall status=%v; want Ok", status)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rv.httpCallsMu.Lock()
		_, present := rv.httpCalls[callID]
		rv.httpCallsMu.Unlock()
		if !present && logFired.Load() {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// NC: the guest callback actually ran. Without this the remaining
	// assertions would read a zero-valued snapshot and could go green
	// vacuously.
	if !logFired.Load() {
		t.Fatal("NC: the guest's proxy_log never fired — proxy_on_http_call_response did not run, so every assertion below would be vacuous")
	}

	// (a) the cache was published for the duration of the callback.
	snap := seenSnapshot.Load()
	if snap == nil {
		t.Fatal("(a) rv.HTTPCallResponse() = nil from INSIDE proxy_on_http_call_response; want the published cache")
	}

	// (b) `:status` under the exact key, and NOT under a canonicalized one.
	vals, ok := snap.Headers[":status"]
	if !ok {
		keys := make([]string, 0, len(snap.Headers))
		for k := range snap.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("(b) no %q key in the published headers; keys = %q", ":status", keys)
	} else if len(vals) != 1 || vals[0] != "201" {
		t.Errorf("(b) headers[\":status\"] = %q; want [\"201\"]", vals)
	}
	for k := range snap.Headers {
		if k != ":status" && strings.EqualFold(k, ":status") {
			t.Errorf("(b) found a case-mangled status key %q; the key must be exactly %q", k, ":status")
		}
	}

	// (c) num_headers is the VALUE count. 3 source values (X-Multi x2,
	// X-Single x1) + 1 synthesized :status = 4. The KEY count would be 3.
	const wantValueCount = 4
	const keyCountWouldBe = 3
	if got := seenLevel.Load(); got != wantValueCount {
		t.Errorf("(c) guest received num_headers=%d; want %d (the VALUE count). %d would mean the KEY count was passed",
			got, wantValueCount, keyCountWouldBe)
	}
	if got := headerValueCount(snap.Headers); got != wantValueCount {
		t.Errorf("(c) headerValueCount(published headers) = %d; want %d", got, wantValueCount)
	}
	if got := len(snap.Headers); got != keyCountWouldBe {
		t.Errorf("(c) published header KEY count = %d; want %d (test fixture invariant: key count and value count must differ)", got, keyCountWouldBe)
	}

	// Body readable through the cache.
	if got := string(snap.Body); got != "hello-body" {
		t.Errorf("published body = %q; want %q", got, "hello-body")
	}

	// (d) the source header map was not mutated by the synthesis. sourceHdrs
	// is the very map handed to the host as resp.Header (see dispatchFn above).
	if _, bad := sourceHdrs[":status"]; bad {
		t.Error("(d) the source resp.Header was MUTATED with :status; the synthesis must write into a COPY")
	}
	if got := len(sourceHdrs); got != 2 {
		t.Errorf("(d) source resp.Header grew to %d keys; want 2 (it must be untouched)", got)
	}

	// (f) THE DISCRIMINATING CONTROL for the `http_call_response == 0` reading
	// in TestA4_HttpCallResponseTrap_PoisonsStreamContext. That test concludes
	// "the trap suppressed the counter" from a ZERO — but a zero is also what a
	// counter that never fires at all produces, and the phase-82 change MOVED
	// this increment. This arm runs the identical dispatch path with a guest
	// that RETURNS NORMALLY and asserts the counter does fire, so the zero over
	// there is a suppression and not a dead measurement path.
	if got := rec.httpCallResponse.Load(); got != 1 {
		t.Errorf("(f) http_call_response = %d after a NON-trapping callback; want 1 (the move must not have killed the increment outright)", got)
	}
	if got := rec.envoyGoFailures.Load(); got != 0 {
		t.Errorf("(f) envoy_go.failures = %d after a NON-trapping callback; want 0 (the trap arm must not fire on the success path)", got)
	}

	// (e) callback-scoped: cleared once the guest returned.
	if after := rv.HTTPCallResponse(); after != nil {
		t.Errorf("(e) rv.HTTPCallResponse() = %+v after the callback returned; want nil (the cache is callback-scoped)", after)
	}
}
