// Package wasm — http_call.go: proxy_http_call dispatch via per-*RootVM
// HTTPDispatcher per 25.2 SPEC §3.1 + §5.1 #37 + §5.3 C19 + §11.3 D-25.2-3
// + Q4 + R-25.2-3 + AMEND-B3 + ADR-0177 RE-CONSUMER at 3rd-or-later co-
// consumer (phase-22.2 `:httpCall()` was second) — CLOSES parent SPEC
// §13-R6 RATIFIED-PENDING-IMPL anchor.
//
// # Design (per Q4 + AMEND-B3 + D-P-PLAN-9)
//
// Outbound HTTP dispatch from a guest `proxy_http_call` hostcall is a
// cluster-based dispatch via the phase-20 `internal/httpclient/` framework
// primitive (the THIRD co-consumer after phase-17 jwks/extauthz/oauth2 at
// introduction + phase-22.2 lua `:httpCall()` at the IN-PLACE AMENDMENT). The
// 25.2 dispatch is byte-faithful to upstream cpp-host `context.cc:1547+1693+
// 1900` semantics:
//
//   - Cluster lookup miss → return WasmResult::BadArgument (=2) + the
//     `wasm.<plugin>.http_call_dispatch_unknown_cluster` counter increments
//     (counter wiring deferred Task 17). Byte-faithful to v1.37.2
//     `context.cc:1547-1550`.
//
//   - Cluster lookup hit → allocate monotonic call_id; insert pendingHttpCall
//     entry into the per-`*RootVM` httpCalls map; spawn a dispatch goroutine
//     that invokes `httpclient.Client.ClusterDispatch` synchronously + on
//     completion routes the response through `handleHttpCallResponse`
//     (callback re-acquires dispatchMu + invokes proxy_on_http_call_response
//     on the originating stream context). The `wasm.<plugin>.http_call_-
//     dispatched` counter increments (deferred Task 17).
//
//   - Cancel-at-destruction at StreamContext.Close → iterate httpCalls + cancel
//     the entry's per-call context for each entry whose originating
//     streamCtxID matches the closing stream; delete from the map. The
//     dispatch goroutine's ClusterDispatch returns with the canceled-context
//     error; handleHttpCallResponse early-returns because the token is gone.
//     Byte-faithful to cpp-host `context.cc:1900-1905` destructor pattern.
//
//   - Defensive token-miss guard at response arrival → if httpCalls[callID]
//     is absent OR the originating stream context is gone, the
//     `wasm.<plugin>.http_call_response_after_close` envoy-go-strict counter
//     increments + the response is dropped (NO panic). Byte-faithful to
//     cpp-host `context.cc:1693-1696` defensive `find()` check. The counter
//     is upstream-superset (cpp-host has NO counter at this site) per
//     AMEND-B3 recommendation: defensive observability for the rare race
//     where envoy-go's cancellation has a bug + a stray response arrives.
//
// # Concurrency contract per D-P-PLAN-9 (LOAD-BEARING — DO NOT VIOLATE)
//
// DispatchHttpCall is invoked from inside a dispatchMu-held hostcall body
// frame (the registration.go envelope acquires dispatchMu via the per-
// `*StreamContext.CallProxyOnX` path; the HttpCallShim → DispatchHttpCall
// chain inherits the frame). DispatchHttpCall MUST NOT re-acquire dispatchMu
// — sync.Mutex in Go is non-reentrant and would deadlock. The httpCallsMu
// is a DIFFERENT mutex (guards only the httpCalls map + nextCallID
// counter); it is safe to acquire from within the dispatchMu-held frame
// (lock order: dispatchMu → httpCallsMu; the reverse order is forbidden).
//
// handleHttpCallResponse runs on the dispatch goroutine (a DIFFERENT
// goroutine from the per-stream caller). It acquires dispatchMu BEFORE
// invoking proxy_on_http_call_response — the per-RootVM dispatchMu
// serialization keeps the guest-side proxy_on_* invocations sequenced
// (mirrors the tick goroutine's lockAndDispatchTick at tick.go).
//
// # HTTPDispatcher interface (decouples from concrete httpclient + clusterMgr)
//
// To keep the wasm package focused on the dispatch orchestration + to enable
// straightforward mocking in unit tests, we declare a thin HTTPDispatcher
// interface that aggregates the cluster-lookup + cluster-dispatch surface.
// Production wires a concrete dispatcher at Task 14 (compiledConfig.New
// constructs a *clusterHTTPDispatcher from the configured httpclient.Client +
// cluster.Manager); tests construct their own mocks via the same interface.
// NO API extension on the httpclient package per the parent §13-R6 invariant
// (the 22.2 IMPL ClusterDispatch covers 25.2's proxy_http_call byte-for-byte).
package wasm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// pendingHttpCall is the per-call state tracked in the per-`*RootVM`
// httpCalls map. The map key is the monotonic call_id allocated at
// DispatchHttpCall time + returned to the guest.
//
// Lifecycle:
//
//	(1) DispatchHttpCall inserts into the map.
//	(2) Either StreamContext.Close cancels (cancel-at-destruction) OR the
//	    dispatch goroutine's handleHttpCallResponse looks up + deletes.
//	(3) Map removal is exactly-once: whichever path fires first deletes the
//	    entry; the other path's lookup observes the absence + treats it as
//	    a token-miss (drops + counter-increment per AMEND-B3).
type pendingHttpCall struct {
	streamCtxID uint32             // originating stream context
	deadline    time.Time          // per-call deadline (timeoutMs from guest)
	cancel      context.CancelFunc // cancel the dispatch goroutine's request context
}

// HTTPDispatcher is the per-`*RootVM` outbound-HTTP dispatch surface
// consumed by DispatchHttpCall. Aggregates the cluster-lookup + cluster-
// dispatch operations so the wasm package can be unit-tested against a
// mock without dragging the cluster.Manager + httpclient.Client concrete
// types into test setup.
//
// Production wiring at Task 14: compiledConfig.New constructs a thin
// adapter struct holding (*httpclient.Client, *cluster.Manager) and wires
// it via WithRootHTTPDispatcher. Tests construct in-package mocks (see
// http_call_test.go fakeHTTPDispatcher).
//
// NO API extension on the httpclient package per parent SPEC §13-R6
// invariant — the 22.2 IMPL `httpclient.Client.ClusterDispatch + cluster.
// Manager.Get` covers 25.2 proxy_http_call byte-for-byte. This interface
// is the wasm-package-private aggregation seam, NOT a new public httpclient
// surface.
type HTTPDispatcher interface {
	// HasCluster reports whether the named cluster is registered. Used by
	// DispatchHttpCall for the up-front BadArgument-on-unknown-cluster
	// check per Q4 + AMEND-B3. Returns true only when cluster.Manager.Get
	// succeeds.
	HasCluster(name string) bool

	// Dispatch performs the outbound HTTP request synchronously. The
	// caller controls the request lifetime via the supplied ctx — at
	// cancel-at-destruction the wasm package cancels the per-call ctx +
	// Dispatch returns with the ctx-cancellation error. On success
	// returns (response, nil); on cluster-lookup-miss, transport error,
	// or ctx-cancellation returns (nil, error). The implementation is
	// expected to drain + close the response body in the wasm-side
	// handleHttpCallResponse path AFTER reading.
	Dispatch(ctx context.Context, cluster string, req *http.Request) (*http.Response, error)
}

// WithRootHTTPDispatcher installs a per-*RootVM HTTPDispatcher. Default nil
// = proxy_http_call returns InternalFailure (no dispatcher wired). Production
// wiring at Task 14 supplies a concrete dispatcher; tests inject a fake.
//
// RE-CONSUMES phase-20 ADR-0177 internal/httpclient/ at 3rd-or-later co-
// consumer (phase-22.2 lua :httpCall() was second) — CLOSES parent SPEC
// §13-R6 RATIFIED-PENDING-IMPL anchor. NO API extension on httpclient
// (phase-22.2's cluster-based dispatch covers 25.2 byte-for-byte).
func WithRootHTTPDispatcher(d HTTPDispatcher) RootVMOption {
	return func(rv *RootVM) { rv.httpDispatcher = d }
}

// DispatchHttpCall starts an outbound HTTP request via the configured
// HTTPDispatcher and returns the allocated call_id. Response routes
// asynchronously to proxy_on_http_call_response on the originating stream
// context via handleHttpCallResponse.
//
// Per Q4 + AMEND-B3 + ADR-0177 RE-CONSUMER:
//
//   - dispatcher == nil ⇒ WasmResultInternalFailure (programmer error;
//     no dispatcher wired at RootVM construction).
//   - cluster unknown ⇒ WasmResultBadArgument + http_call_dispatch_unknown_
//     cluster counter increment (counter wiring deferred Task 17).
//   - cluster known ⇒ allocate monotonic call_id; spawn dispatch goroutine;
//     return (callID, Ok). http_call_dispatched counter increments
//     (counter wiring deferred Task 17).
//
// # Concurrency contract per D-P-PLAN-9 (LOAD-BEARING)
//
// MUST be called from inside a dispatchMu-held caller frame (the HttpCallShim
// hostcall body inherits dispatchMu from the per-StreamContext.CallProxyOnX
// caller). MUST NOT re-acquire dispatchMu — sync.Mutex is non-reentrant.
// httpCallsMu is a DIFFERENT mutex (guards only the httpCalls map +
// nextCallID); acquiring it from within dispatchMu is safe (lock order:
// dispatchMu → httpCallsMu).
func (rv *RootVM) DispatchHttpCall(ctx context.Context, streamCtxID uint32, cluster string, headers []HeaderPair, body []byte, trailers []HeaderPair, timeoutMs uint32) (uint32, abi.WasmResult) {
	if rv.httpDispatcher == nil {
		return 0, abi.WasmResultInternalFailure
	}

	// Cluster-lookup gate per Q4 + AMEND-B3 + cpp-host context.cc:1547-1550.
	// Unknown cluster → BadArgument + http_call_dispatch_unknown_cluster
	// counter increment (counter wiring deferred Task 17).
	if !rv.httpDispatcher.HasCluster(cluster) {
		// envoy-go-strict counter increment per Q4 + AMEND-B3 (counter 11 of
		// 14 in §7.1). Wire name: `wasm.<plugin>.http_call_dispatch_unknown_
		// cluster`. Wired via RootStatsRecorder per 25.2 IMPL Task 20
		// follow-up (Concern 2).
		rv.stats.HttpCallDispatchUnknownClusterInc()
		return 0, abi.WasmResultBadArgument
	}

	// Build the *http.Request from the cluster name + headers + body +
	// per-call timeout context. Headers carry the proxy-wasm pseudo-header
	// shape (:method / :path / :scheme / :authority); buildHttpCallRequest
	// extracts them into the URL + method.
	timeout := defaultHttpCallTimeout
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
	req, err := buildHttpCallRequest(reqCtx, cluster, headers, body, trailers)
	if err != nil {
		cancel()
		return 0, abi.WasmResultBadArgument
	}

	// Allocate monotonic call_id + insert pendingHttpCall under httpCallsMu.
	// Pre-allocate the map lazily on first use to keep zero-config RootVMs
	// allocation-free.
	rv.httpCallsMu.Lock()
	rv.nextCallID++
	callID := rv.nextCallID
	if rv.httpCalls == nil {
		rv.httpCalls = make(map[uint32]*pendingHttpCall)
	}
	rv.httpCalls[callID] = &pendingHttpCall{
		streamCtxID: streamCtxID,
		deadline:    time.Now().Add(timeout),
		cancel:      cancel,
	}
	rv.httpCallsMu.Unlock()

	// envoy-go-strict counter increment per Q9 + AMEND-B3 (counter 7 of 14
	// in §7.1). Wire name: `wasm.<plugin>.http_call_dispatched`. Wired via
	// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
	rv.stats.HttpCallDispatchedInc()

	// Spawn the dispatch goroutine. Uses a SEPARATE ctx (NOT the caller's
	// ctx — the caller's ctx is the per-host-call dispatch ctx which may be
	// canceled when the hostcall returns; our reqCtx is bound to the per-
	// call timeout + the cancel-at-destruction hook). The reqCtx is the
	// SAME ctx held in pendingHttpCall.cancel — closing this ctx from
	// cancelOutstandingHttpCalls terminates the in-flight Dispatch.
	//
	// We do NOT thread the caller's ctx — the dispatch survives the host-
	// call return + completes asynchronously. The hostcall layer captures
	// the bare context.Background() at dispatch time as the response-
	// invocation ctx (used when re-entering the wazero VM to invoke
	// proxy_on_http_call_response).
	go rv.dispatchHttpCallGoroutine(callID, cluster, req)
	_ = ctx // ctx is for the hostcall path; response goroutine uses background

	return callID, abi.WasmResultOk
}

// dispatchHttpCallGoroutine runs on a NEW goroutine spawned from
// DispatchHttpCall. Invokes httpDispatcher.Dispatch synchronously +
// routes the response through handleHttpCallResponse on completion.
//
// On error from Dispatch (transport-level + ctx-canceled inclusive), routes
// (nil resp, err) — handleHttpCallResponse treats both error + nil-resp as
// the "no body / no headers" path + dispatches proxy_on_http_call_response
// with (num_headers=0, body_size=0, num_trailers=0) IF the originating
// stream is still alive. (When the stream is gone OR the call was canceled
// at destruction, handleHttpCallResponse's token-miss guard fires + the
// http_call_response_after_close counter increments.)
func (rv *RootVM) dispatchHttpCallGoroutine(callID uint32, cluster string, req *http.Request) {
	resp, err := rv.httpDispatcher.Dispatch(req.Context(), cluster, req)
	rv.handleHttpCallResponse(callID, resp, err)
}

// handleHttpCallResponse is invoked from the dispatch goroutine after the
// HTTPDispatcher returns. Routes the response (or error) to the originating
// stream context's proxy_on_http_call_response callback per §5.3 C19.
//
// Token-miss guard per AMEND-B3 + cpp-host context.cc:1693-1696:
//
//   - if httpCalls[callID] is absent ⇒ the cancel-at-destruction path
//     already removed the entry (or the call was never registered — a
//     defensive impossibility for the in-package caller); the
//     http_call_response_after_close counter increments + the response is
//     dropped (no proxy_on_http_call_response invocation).
//
//   - if the originating stream context is gone (rv.streamCtxs miss OR the
//     looked-up sc has closed.Load() == true) ⇒ ditto.
//
//   - else: acquire dispatchMu + set currentCtxID = streamCtxID + invoke
//     proxy_on_http_call_response(streamCtxID, callID, num_headers, body_-
//     size, num_trailers) under the panic-wrapper. http_call_response
//     counter increments (Task 17).
//
// The response body is drained + closed regardless of the dispatch path
// — net/http connection-pool reuse requires this discipline.
func (rv *RootVM) handleHttpCallResponse(callID uint32, resp *http.Response, dispatchErr error) {
	// Always drain + close the response body before returning (connection-pool
	// reuse discipline). Defer-close is safe whether or not the body is
	// later consumed by the guest — the body bytes have already been read
	// + materialized into a host-side buffer by the time we drain.
	var bodyBytes []byte
	if resp != nil {
		// Read body fully into memory. The per-call timeout already gated
		// the read (req.Context() carries the deadline); we accept the
		// resulting bytes verbatim.
		bodyBytes, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	_ = dispatchErr // dispatchErr surfaces in the body_size=0 path; logged via PanicHandler indirectly

	// Token-lookup + delete-or-drop. Both branches release httpCallsMu
	// before doing any further work (we MUST NOT hold httpCallsMu across
	// the dispatchMu acquisition — lock order is dispatchMu → httpCallsMu,
	// reverse acquisition would deadlock).
	rv.httpCallsMu.Lock()
	entry, ok := rv.httpCalls[callID]
	if ok {
		delete(rv.httpCalls, callID)
	}
	rv.httpCallsMu.Unlock()

	if !ok {
		// Cancel-at-destruction or token-miss path per AMEND-B3 + cpp-host
		// context.cc:1693-1696. The entry was already removed (Close hit
		// first) or never existed (defensive). Increment the envoy-go-strict
		// observability counter + drop. Wire name:
		// `wasm.<plugin>.http_call_response_after_close`. Wired per 25.2
		// IMPL Task 20 follow-up (Concern 2).
		rv.stats.HttpCallResponseAfterCloseInc()
		return
	}

	// Release the per-call cancel func (no-op if already canceled at
	// destruction; idempotent per stdlib context.CancelFunc contract).
	entry.cancel()

	// Resolve the originating stream context. If the stream is gone (the
	// map was cleared OR the sc was marked closed), the response is
	// orphaned: increment the counter + drop.
	rv.streamCtxsMu.RLock()
	var sc *StreamContext
	if rv.streamCtxs != nil {
		sc = rv.streamCtxs[entry.streamCtxID]
	}
	rv.streamCtxsMu.RUnlock()
	if sc == nil || sc.closed.Load() {
		// The stream is gone AFTER we observed a live token; the response
		// is orphaned. Counter increments for defensive observability per
		// AMEND-B3. Wired per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.HttpCallResponseAfterCloseInc()
		return
	}

	// Acquire dispatchMu + invoke proxy_on_http_call_response on the
	// originating stream context. Mirrors tick.go lockAndDispatchTick.
	//
	// Use a fresh context.Background() — the original reqCtx is canceled
	// by the deferred entry.cancel() above; the guest invocation runs
	// synchronously under the dispatchMu-held frame and consumes its own
	// dispatch ctx, NOT the per-call ctx.
	ctx := context.Background()

	rv.dispatchMu.Lock()
	defer rv.dispatchMu.Unlock()

	// Re-check closed flag AFTER lock: RootVM.Close races with the response
	// goroutine. Close marks every live child closed BEFORE releasing
	// dispatchMu; an early-return here keeps us from re-entering a torn-
	// down wazero instance.
	if rv.closed.Load() || rv.instance == nil || sc.closed.Load() {
		// RootVM closed mid-flight. Defensive observability per AMEND-B3.
		// Wired per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.HttpCallResponseAfterCloseInc()
		return
	}

	// Set currentCtxID for the per-stream callback dispatch. Mirrors the
	// per-StreamContext.CallProxyOnX pattern at stream_context.go.
	rv.currentCtxID.Store(entry.streamCtxID)

	// gate-at-getFunction per AMEND-B5 — proxy_on_http_call_response is
	// one of the 7 NEW 25.2 callbacks gated via capProxyOnHttpCallResponse.
	if !rv.HasGlobalFunc("proxy_on_http_call_response") {
		// No guest export OR cap denied — the dispatch is a no-op success;
		// the http_call_response counter still increments since we
		// successfully routed (the guest just chose to not consume). Wired
		// per 25.2 IMPL Task 20 follow-up (Concern 2).
		rv.stats.HttpCallResponseInc()
		return
	}
	fn := rv.instance.ExportedFunction("proxy_on_http_call_response")
	if fn == nil {
		// HasGlobalFunc said it exists but ExportedFunction returned nil —
		// defensive (theoretically unreachable; concurrent module-close
		// would have set closed=true above).
		return
	}

	// Compute (num_headers, body_size, num_trailers) for the callback args.
	// On dispatchErr OR nil resp, all three are 0 (the guest receives a
	// degenerate response; it MUST inspect via proxy_get_buffer_bytes +
	// proxy_get_header_map_pairs which return empty/NotFound for the
	// HttpCallResponse* buffer + header-map types — Task 15 wires the
	// consumer-side response-body + headers cache via the OnHttpCallResponse
	// ABICallbacks method that lands at Task 15).
	var numHeaders, bodySize, numTrailers uint32
	if resp != nil {
		numHeaders = uint32(len(resp.Header))
		bodySize = uint32(len(bodyBytes))
		// Trailers are part of resp.Trailer once the body is fully read.
		numTrailers = uint32(len(resp.Trailer))
	}

	// TODO Task 15: stash (resp.Header, bodyBytes, resp.Trailer) on the
	// originating *StreamContext so the guest's proxy_get_buffer_bytes
	// (HttpCallResponseBody) + proxy_get_header_map_pairs
	// (HttpCallResponseHeaders / HttpCallResponseTrailers) hostcalls can
	// fetch the materialized response. At Task 8 we only invoke the
	// callback with the size/count tuple per §5.3 C19; consumer-side body+
	// headers cache lands at Task 15 (abi_callbacks.go EXTEND OnHttpCall-
	// Response + Task 16 body.go cache wiring).

	// envoy-go-strict counter increment per Q9 (counter 8 of 14 in §7.1).
	// Wire name: `wasm.<plugin>.http_call_response`. Wired via
	// RootStatsRecorder per 25.2 IMPL Task 20 follow-up (Concern 2).
	rv.stats.HttpCallResponseInc()

	_ = rv.runCallWithPanicWrapper(func() error {
		_, callErr := fn.Call(ctx,
			uint64(entry.streamCtxID), // plugin_context_id (= streamCtxID per §5.3 C19)
			uint64(callID),
			uint64(numHeaders),
			uint64(bodySize),
			uint64(numTrailers),
		)
		return callErr
	})
}

// cancelOutstandingHttpCalls is the StreamContext.Close-time path: walks
// the per-*RootVM httpCalls map + cancels every entry whose originating
// streamCtxID matches sc.streamCtxID. Byte-faithful to cpp-host
// `context.cc:1900-1905` destructor that iterates http_request_ + calls
// p.second.request_->cancel() on each.
//
// Two-phase: snapshot under httpCallsMu, then cancel OUTSIDE the lock to
// avoid holding httpCallsMu while waiting on context.CancelFunc + to keep
// the lock-held duration bounded.
func (rv *RootVM) cancelOutstandingHttpCalls(streamCtxID uint32) {
	rv.httpCallsMu.Lock()
	var cancels []context.CancelFunc
	for id, entry := range rv.httpCalls {
		if entry.streamCtxID == streamCtxID {
			cancels = append(cancels, entry.cancel)
			delete(rv.httpCalls, id)
		}
	}
	rv.httpCallsMu.Unlock()

	for _, c := range cancels {
		if c != nil {
			c()
		}
	}
}

// buildHttpCallRequest constructs an *http.Request from the cluster name +
// headers + body + bound context. Mirrors the phase-22.2 lua httpcall.go
// buildHTTPCallRequest pattern (cluster goes in URL.Host placeholder;
// HTTPDispatcher.Dispatch rewrites to the picked endpoint's addr).
//
// Proxy-wasm pseudo-headers extracted into URL composition:
//
//   - :method   → request method (default GET)
//   - :path     → request URL path (default /)
//   - :scheme   → request URL scheme (default http; HTTPDispatcher may
//     override based on cluster TLS config)
//   - :authority → request URL host (informational; HTTPDispatcher rewrites
//     URL.Host to the picked endpoint's addr)
//
// All other headers copy verbatim into req.Header. Trailers are NOT carried
// on the outbound request at 25.2 (proxy-wasm guests rarely populate
// outbound trailers; the trailers arg is accepted on the wire for spec
// parity but discarded at this level — future scope may wire stdlib
// http.Request.Trailer for HTTP/1.1 chunked + HTTP/2 trailers if a
// consumer demands it).
func buildHttpCallRequest(ctx context.Context, cluster string, headers []HeaderPair, body []byte, _ []HeaderPair) (*http.Request, error) {
	method := http.MethodGet
	path := "/"
	scheme := "http"
	authority := cluster

	for _, h := range headers {
		switch h.Key {
		case ":method":
			if h.Value != "" {
				method = h.Value
			}
		case ":path":
			if h.Value != "" {
				path = h.Value
			}
		case ":scheme":
			if h.Value != "" {
				scheme = h.Value
			}
		case ":authority":
			if h.Value != "" {
				authority = h.Value
			}
		}
	}

	url := fmt.Sprintf("%s://%s%s", scheme, authority, path)
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = newByteReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Copy non-pseudo headers into req.Header.
	for _, h := range headers {
		if len(h.Key) > 0 && h.Key[0] == ':' {
			continue
		}
		req.Header.Add(h.Key, h.Value)
	}
	return req, nil
}

// byteReader is a minimal bytes.Reader-equivalent that avoids importing the
// bytes package just for this single helper. Keeps the byte slice as-is +
// reads sequentially. The reader's Close is a no-op (http.NewRequest does
// not call Close on the body Reader; the stdlib re-wraps in nopCloser
// internally).
type byteReader struct {
	b   []byte
	pos int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

// defaultHttpCallTimeout is the per-call timeout applied when the guest-
// supplied timeoutMs is zero. Mirrors a conservative default; production
// guests should always supply an explicit timeout.
const defaultHttpCallTimeout = 5 * time.Second

// errHttpDispatchTimeout is a sentinel surfaced when the per-call deadline
// exceeds (future scope — currently unused; reserved for the Task 17 +
// counter integration).
//
//nolint:unused // reserved for Task 17 counter wiring
var errHttpDispatchTimeout = errors.New("wasm: http_call: per-call deadline exceeded")

// Compile-time guard: the byteReader satisfies io.Reader.
var _ io.Reader = (*byteReader)(nil)
