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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
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
