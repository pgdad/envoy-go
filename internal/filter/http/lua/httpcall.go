package lua

// httpcall.go — Task 11 (phase 22.2 IMPL) httpCall bridge per SPEC §3.4 +
// §11.7 D6 closure + AMEND-22.2-3 (PURE FIRE-AND-FORGET on
// asynchronous=true) + PLAN Task 11 + ADR-0192 §Decision body anticipation.
//
// # Surface (request_handle + response_handle parity)
//
//   - :httpCall(cluster, headers, body, timeout_ms, asynchronous?) —
//     cluster-based outbound HTTP dispatch. The named cluster is
//     resolved via the per-stream *cluster.Manager (threaded via
//     FactoryCtx.ClusterManager); the request is dispatched via
//     httpclient.Client.ClusterDispatch — the FIRST CO-CONSUMER call site
//     of Task 4's ADR-0177 IN-PLACE AMENDMENT (R5 RATIFIED at Task 4
//     + actualized here).
//
// # Sync path (asynchronous=nil or false; the default)
//
// Per SPEC §11.7.3 + §11.1 D2 closure:
//
//  1. The bridge LGFunction validates args + builds *http.Request from
//     the Lua headers table + body string + per-call timeout context.
//  2. The bridge stashes the bridge LState on f.pendingHTTPCallResume,
//     spawns a dispatch goroutine, increments
//     httpcall_total + coroutine_yields_total, and returns
//     YieldFromBridge(L, lua.LNil) — gopher-lua's callGFunction
//     unwinds via switchToParentThread per §11.1 D2.
//  3. The dispatch goroutine calls f.httpClient.ClusterDispatch(ctx,
//     cluster, req, f.clusterMgr). On success: marshals response headers
//     into a Lua table + response body into a Lua string, then calls
//     f.vm.Resume(child, nil, headersTable, bodyString). On failure:
//     calls f.vm.Resume(child, nil, lua.LNil, lua.LString(err.Error())).
//     The goroutine ALSO closes f.httpCallDone to signal completion to
//     test code (production HCM dispatch coordination lands at a future
//     phase — the HCM dispatcher schedules the response-ready callback).
//  4. Counter increments per SPEC §7.1 + §11.7 D6: httpcall_failures on
//     transport-failure (any non-nil err from ClusterDispatch); the
//     timeout sub-case is detected via errors.Is(err, context.DeadlineExceeded)
//     and increments httpcall_timeouts (in ADDITION to httpcall_failures
//     so operators can scrape both rates). Per upstream synthetic-503
//     parity discipline, ALSO increments httpcall_failures on a 5xx
//     response from a successful dispatch (this matches the upstream
//     onFailure synthetic-503 path observation that 5xx-from-upstream is
//     treated as a transport failure for stat-counter purposes).
//
// # Async path (asynchronous=true; PURE FIRE-AND-FORGET per AMEND-22.2-3)
//
// Per SPEC §11.7.2 + AMEND-22.2-3 + upstream lua_filter.cc:400-416
// noopCallbacks parity:
//
//   - The bridge LGFunction spawns a fire-and-forget goroutine that
//     calls ClusterDispatch; response/error are FULLY DISCARDED.
//   - NO yield occurs (the script keeps running synchronously).
//   - The LGFunction returns 0 values to Lua.
//   - httpcall_total increments (every dispatch counted).
//   - httpcall_failures + httpcall_timeouts are NOT incremented (async
//     failures invisible at filter-stats per upstream parity).
//
// # Arm-20 runtime-reject (cluster name required)
//
// Per SPEC §6 arm-20 + W2 byte-stable wording: if cluster == "", the
// bridge raises a Lua runtime-error with the exact wording
// `"lua: httpCall: cluster name must not be empty"` (matches the parent
// SPEC §6.2 arm-20 row verbatim). This is a RUNTIME-REJECT (via Lua
// luaL_error), NOT a PARSE-REJECT.
//
// # Coroutine goroutine-safety discipline (§11.1 D2 closure)
//
// gopher-lua's parent.Resume(child, ...) is NOT goroutine-safe with
// itself or with concurrent child-state access. The dispatch goroutine's
// Resume call MUST NOT race with the outer parent.Resume call site
// (decode_headers.go / encode_headers.go). At Task 11 the test goroutine
// waits on f.httpCallDone before asserting on script results; production
// HCM dispatch coordination lands at a future phase (the HCM dispatcher
// schedules the response-ready callback that then drives Resume from the
// dispatch goroutine on the chain dispatch's serialized thread).
//
// # Cross-references
//
//   - SPEC §3.4 (httpCall surface signature)
//   - SPEC §11.7 D6 closure + AMEND-22.2-3 (PURE FIRE-AND-FORGET semantics)
//   - SPEC §6 arm-20 + W2 (byte-stable cluster-required wording)
//   - SPEC §11.4 (in-place AMEND on ADR-0177 for ClusterDispatch — Task 4)
//   - SPEC §11.1 D2 (coroutine yield/resume via gopher-lua native)
//   - PLAN Task 11 (R5 RATIFIED at Task 4 + actualized at this Task)
//   - ADR-0192 §Decision body anticipation (lands at Task 19 atomic
//     landing)
//   - ADR-0177 §Decision AMENDMENT body (lands at Task 19 atomic landing
//     per ADR-0044 in-place edit discipline; no new ADR number consumed)

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// httpCallClusterRequiredMsg is the SPEC §6 arm-20 + W2 byte-stable
// runtime-reject wording raised when :httpCall("", ...) is called with
// an empty cluster name. Pinned via a package-level const so a
// regression on the byte-stable contract surfaces at the
// Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording assertion.
// Matches parent SPEC §6.2 arm-20 row verbatim.
const httpCallClusterRequiredMsg = "lua: httpCall: cluster name must not be empty"

// defaultHTTPCallTimeout is the per-call timeout applied when the
// script-supplied timeout_ms is zero or negative. Mirrors a conservative
// default; production scripts should supply an explicit timeout.
const defaultHTTPCallTimeout = 5 * time.Second

// HTTP pseudo-headers extracted from the Lua headers table for request
// composition. Per SPEC §11.7.1 + upstream lua_filter.cc:180-184, the
// :method / :path / :authority headers are required-shape per Envoy's
// HTTP/2 framing convention. Per-pseudo-header handling in
// buildHTTPCallRequest below.
const (
	pseudoHeaderMethod    = ":method"
	pseudoHeaderPath      = ":path"
	pseudoHeaderScheme    = ":scheme"
	pseudoHeaderAuthority = ":authority"
)

// requestHandleHttpCall implements request_handle:httpCall(cluster,
// headers, body, timeout_ms, asynchronous?) per SPEC §3.4 + §11.7 D6
// closure + AMEND-22.2-3.
func requestHandleHttpCall(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	return runHTTPCall(L, ctx.filterRef, 2)
}

// responseHandleHttpCall implements response_handle:httpCall(...) per
// SPEC §3.4 — symmetric to requestHandleHttpCall for the encode side.
func responseHandleHttpCall(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	return runHTTPCall(L, ctx.filterRef, 2)
}

// runHTTPCall is the shared sync/async dispatch core consumed by both
// request_handle:httpCall and response_handle:httpCall. argStart is the
// stack index of the first positional argument AFTER the receiver
// userdata (always 2 — Lua method-call convention).
//
// Argument shape per SPEC §11.7.1:
//
//  1. cluster — string (required; empty raises arm-20)
//  2. headers — table (may include :method / :path / :scheme / :authority
//     pseudo-headers + regular HTTP headers)
//  3. body — string (may be empty; nil treated as empty)
//  4. timeout_ms — number (zero or negative uses defaultHTTPCallTimeout)
//  5. asynchronous — boolean (optional; default = false = sync)
func runHTTPCall(L *lua.LState, f *filter, argStart int) int {
	// Arg 1: cluster (required).
	cluster := L.CheckString(argStart)
	if cluster == "" {
		L.RaiseError("%s", httpCallClusterRequiredMsg)
		return 0
	}

	// Arg 2: headers table. Defensive: tolerate nil/absent → empty.
	var headersTbl *lua.LTable
	if hv := L.Get(argStart + 1); hv != lua.LNil {
		if t, isTable := hv.(*lua.LTable); isTable {
			headersTbl = t
		}
	}

	// Arg 3: body string. nil/absent → "".
	var body string
	if bv := L.Get(argStart + 2); bv != lua.LNil {
		body = bv.String()
	}

	// Arg 4: timeout_ms.
	timeoutMs := L.OptInt(argStart+3, 0)

	// Arg 5: asynchronous (optional bool).
	async := false
	if av := L.Get(argStart + 4); av != lua.LNil {
		if b, isBool := av.(lua.LBool); isBool {
			async = bool(b)
		}
	}

	// Compute per-call timeout. Zero or negative uses default.
	timeout := defaultHTTPCallTimeout
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	// Build *http.Request from Lua headers table + body string. The
	// resulting request has URL.Host = cluster (placeholder; ClusterDispatch
	// rewrites to the picked endpoint's addr per httpclient.go:296).
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	req, buildErr := buildHTTPCallRequest(ctx, cluster, headersTbl, body)
	if buildErr != nil {
		cancel()
		// Surface as a Lua runtime error — malformed-request is an
		// operator-error analog to arm-20. The exact wording carries the
		// underlying error verbatim (test code asserts substring not
		// byte-exact for this surface — only arm-20 is byte-stable per W2).
		L.RaiseError("lua: httpCall: %s", buildErr.Error())
		return 0
	}

	// Increment httpcall_total on every dispatch (sync + async) per SPEC
	// §7.1 + §11.7 D6 R6.1.
	if f != nil && f.cc != nil && f.cc.stats != nil && f.cc.stats.httpcallTotal != nil {
		f.cc.stats.httpcallTotal.Inc()
	}

	if async {
		// Async path — PURE FIRE-AND-FORGET per AMEND-22.2-3 + upstream
		// lua_filter.cc:400-416 noopCallbacks parity. Spawn goroutine that
		// calls ClusterDispatch + DISCARDS response/error. Returns 0 values
		// immediately; NO yield. httpcall_failures + httpcall_timeouts are
		// NOT incremented per upstream parity (async failures invisible at
		// filter-stats layer).
		go func() {
			defer cancel()
			if f == nil || f.httpClient == nil || f.clusterMgr == nil {
				return
			}
			resp, err := f.httpClient.ClusterDispatch(ctx, cluster, req, f.clusterMgr)
			if err == nil && resp != nil {
				// Drain + close to release the connection back to the pool
				// (mirrors upstream noopCallbacks "discard fully").
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
		return 0
	}

	// Sync path. If the filter has no httpClient / clusterMgr (synthetic
	// test path with HTTP-call exercise but no plumbing), surface a Lua
	// runtime error rather than yield forever. This is a defensive
	// guard; production wiring always supplies non-nil pointers per
	// FactoryCtx.HTTPClient + FactoryCtx.ClusterManager.
	if f == nil || f.httpClient == nil || f.clusterMgr == nil {
		cancel()
		if f != nil && f.cc != nil && f.cc.stats != nil && f.cc.stats.httpcallFailures != nil {
			f.cc.stats.httpcallFailures.Inc()
		}
		L.Push(lua.LNil)
		L.Push(lua.LString("httpCall: no http client / cluster manager bound to filter"))
		return 2
	}

	// Increment coroutine_yields_total ONCE per yield event per SPEC §7.1.
	if f.cc != nil && f.cc.stats != nil && f.cc.stats.coroutineYieldsTotal != nil {
		f.cc.stats.coroutineYieldsTotal.Inc()
	}

	// Stash the bridge LState on the per-filter pending slot + allocate
	// the gate + completion signal channels. The dispatch goroutine waits
	// on f.httpCallReady BEFORE its Resume call — the caller (test code
	// at 22.2; production HCM dispatch coordination at a future phase)
	// closes this AFTER observing the outer parent.Resume return
	// ResumeYield. The dispatch goroutine signals on f.httpCallDone AFTER
	// its Resume call completes — test code waits on this channel to
	// ensure ordering with parent.Resume.
	//
	// Per gopher-lua's non-goroutine-safe Resume + child-state-touching
	// discipline (§11.1 D2 closure), this gating is REQUIRED to satisfy
	// the Go race detector: without httpCallReady the dispatch goroutine
	// might call Resume while the outer parent.Resume's switchToParent-
	// Thread Push is still touching the registry.
	f.pendingHTTPCallResume = L
	f.httpCallReady = make(chan struct{})
	f.httpCallDone = make(chan struct{})

	// Dispatch goroutine — calls ClusterDispatch + drives parent.Resume
	// on completion with the marshaled (headers, body) tuple OR (nil, err).
	// Waits on readyCh before touching the child *LState so the outer
	// parent.Resume call has fully unwound.
	go func(child *lua.LState, readyCh, doneCh chan struct{}, cancelFn context.CancelFunc) {
		defer cancelFn()
		defer close(doneCh)

		resp, err := f.httpClient.ClusterDispatch(ctx, cluster, req, f.clusterMgr)

		// Stats discipline (sync-only per SPEC §11.7 D6 + AMEND-22.2-3):
		//   - transport-failure (err != nil) → httpcall_failures++.
		//   - timeout sub-case (errors.Is(err, context.DeadlineExceeded))
		//     → httpcall_timeouts++ IN ADDITION (operators want both
		//     scrape rates).
		//   - 5xx response → httpcall_failures++ (mirrors upstream
		//     synthetic-503 disposition; 5xx is treated as a transport-
		//     failure analog for stat-counter purposes).
		if err != nil {
			if f.cc != nil && f.cc.stats != nil && f.cc.stats.httpcallFailures != nil {
				f.cc.stats.httpcallFailures.Inc()
			}
			if isTimeoutError(err) {
				if f.cc != nil && f.cc.stats != nil && f.cc.stats.httpcallTimeouts != nil {
					f.cc.stats.httpcallTimeouts.Inc()
				}
			}
		} else if resp != nil && resp.StatusCode >= 500 && resp.StatusCode < 600 {
			if f.cc != nil && f.cc.stats != nil && f.cc.stats.httpcallFailures != nil {
				f.cc.stats.httpcallFailures.Inc()
			}
		}

		// Wait for the caller to signal that the outer parent.Resume call
		// has unwound — gopher-lua's Resume is not goroutine-safe with
		// itself or with concurrent child-state access, so this goroutine
		// MUST NOT touch the child *LState until readyCh is closed.
		// Test code closes readyCh after observing ResumeYield; production
		// HCM dispatch coordination at a future phase does the same.
		<-readyCh

		// Build resume values + drive parent.Resume on the child *LState.
		// Per §11.1 D2 goroutine-safety: this goroutine MUST NOT race with
		// the outer parent.Resume call site; coordination at 22.2 is via
		// the readyCh gate above + doneCh signal below.
		f.pendingHTTPCallResume = nil // clear before Resume to prevent double-dispatch
		if err != nil {
			if f.vm != nil {
				_, _, _ = f.vm.Resume(child, nil, lua.LNil, lua.LString(err.Error()))
			}
			return
		}
		hdrsTbl, bodyStr := marshalHTTPCallResponse(child, resp)
		if f.vm != nil {
			_, _, _ = f.vm.Resume(child, nil, hdrsTbl, lua.LString(bodyStr))
		}
	}(L, f.httpCallReady, f.httpCallDone, cancel)

	// Yield to gopher-lua per §11.1 D2 closure. The bridge LGFunction MUST
	// return YieldFromBridge's result directly to gopher-lua so
	// callGFunction sees gfnret < 0 and unwinds via switchToParentThread.
	return luaprim.YieldFromBridge(L, lua.LNil)
}

// buildHTTPCallRequest constructs an *http.Request from the cluster name +
// Lua headers table + body string + bound context. Per SPEC §11.7.1:
//
//   - :method pseudo-header → request method (default "GET").
//   - :path pseudo-header → request URL path (default "/").
//   - :scheme pseudo-header → request URL scheme (default "http"; note
//     ClusterDispatch overrides this based on cluster TLS config at
//     httpclient.go:296-301).
//   - :authority pseudo-header → request URL host (informational only —
//     ClusterDispatch rewrites URL.Host to the picked endpoint's addr at
//     httpclient.go:296).
//   - All other entries → http.Header (case-preserved per Go convention).
//
// The cluster name is used as a placeholder URL.Host so the URL parses
// validly; ClusterDispatch rewrites it before transport-layer dial.
func buildHTTPCallRequest(ctx context.Context, cluster string, headers *lua.LTable, body string) (*http.Request, error) {
	method := http.MethodGet
	path := "/"
	scheme := "http"
	authority := cluster

	if headers != nil {
		if v := headers.RawGetString(pseudoHeaderMethod); v != lua.LNil {
			method = strings.ToUpper(v.String())
		}
		if v := headers.RawGetString(pseudoHeaderPath); v != lua.LNil {
			path = v.String()
		}
		if v := headers.RawGetString(pseudoHeaderScheme); v != lua.LNil {
			scheme = v.String()
		}
		if v := headers.RawGetString(pseudoHeaderAuthority); v != lua.LNil {
			authority = v.String()
		}
	}

	// Compose URL string. cluster goes in URL.Host; ClusterDispatch
	// rewrites to the picked endpoint's addr per httpclient.go:296.
	url := fmt.Sprintf("%s://%s%s", scheme, authority, path)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader([]byte(body)))
	if err != nil {
		return nil, err
	}

	// Copy non-pseudo headers from the Lua table into req.Header.
	if headers != nil {
		headers.ForEach(func(k, v lua.LValue) {
			keyStr, ok := k.(lua.LString)
			if !ok {
				return
			}
			name := string(keyStr)
			if strings.HasPrefix(name, pseudoHeaderPrefix) {
				return // pseudo-header; consumed above for URL composition
			}
			req.Header.Add(name, v.String())
		})
	}

	return req, nil
}

// marshalHTTPCallResponse converts an *http.Response into a Lua headers
// table + body string per SPEC §11.7.3 sync-response wire shape. The
// response headers table is keyed by lower-case header name (mirrors
// upstream Lua filter HeaderMapWrapper which lower-cases all header
// names per HTTP/2 framing convention) with the :status pseudo-header
// included.
//
// The body is read fully into memory + the response is closed; the
// returned bodyStr is a Go string (immutable; safe to push as
// lua.LString into the resumed coroutine).
func marshalHTTPCallResponse(L *lua.LState, resp *http.Response) (*lua.LTable, string) {
	tbl := L.NewTable()
	if resp == nil {
		return tbl, ""
	}
	// :status pseudo-header per upstream Lua filter convention.
	tbl.RawSetString(":status", lua.LString(fmt.Sprintf("%d", resp.StatusCode)))
	// Regular headers — lower-case key + first value (matches upstream's
	// header map flattening — multiple values collapse to last-wins or
	// concatenated; we adopt last-wins to keep the Lua-side table shape
	// flat per the upstream wire-shape convention).
	for name, vals := range resp.Header {
		if len(vals) == 0 {
			continue
		}
		tbl.RawSetString(strings.ToLower(name), lua.LString(vals[len(vals)-1]))
	}
	// Read body fully; close on exit. Defensive-copy via Go string
	// conversion — Lua's LString IS an immutable Go string, detached from
	// the response body's underlying buffer lifetime.
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return tbl, string(bodyBytes)
}

// isTimeoutError reports whether err represents a request timeout
// (context.DeadlineExceeded or net-package timeout-implementer). Used by
// the dispatch goroutine's stat-counter discipline to distinguish
// timeout-failure from other transport-failure for the httpcall_timeouts
// SYNC-ONLY counter per SPEC §11.7 D6 + AMEND-22.2-3.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// stdlib net/url.Error wraps a timeout error; the underlying error's
	// Timeout() method reports true.
	type timeouter interface{ Timeout() bool }
	var t timeouter
	if errors.As(err, &t) {
		return t.Timeout()
	}
	return false
}
