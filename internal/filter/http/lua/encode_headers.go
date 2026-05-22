package lua

// encode_headers.go — EncodeHeaders dispatcher per 22.1 SPEC §4.3 +
// 22.2 production HCM coroutine orchestration per ADR-0192 §Decision
// body anticipation (Task 19a closure of Task 7 deferred production-
// HCM-orchestration gap; symmetric to decode_headers.go).
//
// The per-stream VM is REUSED from DecodeHeaders (the chunk was
// already vm.Run'd at decode time + the envoy_on_response global is
// already registered), so encode just constructs a fresh response_handle
// userdata and invokes the hook AS A COROUTINE.
//
// # Dispatch sequence (Task 19a coroutine orchestration)
//
//  1. Nil-prerequisites pass-through (unchanged from 22.1).
//
//  2. Build the response_handle context + LUserData (unchanged).
//
//  3. Hook-presence check (unchanged).
//
//  4. stats.executions++.
//
//  5. PRODUCTION COROUTINE DISPATCH (Task 19a NEW):
//
//     a. f.encodeChild, f.encodeChildCancel = vm.NewThread() — mints
//        a child *LState as a coroutine state for the encode-side.
//
//     b. fn = parent.GetGlobal("envoy_on_response").(*lua.LFunction).
//
//     c. state, _, _ = vm.Resume(child, fn, respUd) — drives the
//        coroutine. Three return shapes (symmetric to decode side):
//          - ResumeOK: script ran to completion; response headers
//            mutated; encode-side :respond() raised the AMEND-8
//            runtime-reject which surfaced as ResumeError.
//          - ResumeYield: encode-side bridge yielded — most common
//            cases: :body() pre-encode-endStream (resumes in
//            EncodeData); :httpCall sync (close+wait pattern).
//          - ResumeError: log + Continue (includes the AMEND-8
//            encode-side :respond() reject).
//
//  6. Continue. There is NO respond-state check on the encode side
//     (the encode-side :respond() raises a runtime error, never
//     captures state).
//
// Note: header mutations performed by envoy_on_response via
// response_handle:headers() mutate the response-side http.Header in
// place — the framework's encode chain consumes the mutated map after
// EncodeHeaders returns Continue (OR after the body-yield resume
// completes inside EncodeData).

import (
	"net/http"

	lua "github.com/yuin/gopher-lua"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// EncodeHeaders implements the encode-side dispatcher per 22.1 SPEC
// §4.3 + 22.2 production coroutine orchestration (Task 19a closure of
// Task 7 deferred production-HCM-orchestration gap).
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1: nil-prerequisites pass-through. Gated ONLY on f.vm == nil per
	// the phase 22.3 Task 3 encode-guard fix (load-bearing): a per-route
	// source_code (or name) override on a DEFAULT-LESS listener means
	// f.cc.chunk == nil but f.vm != nil (decode built + ran the override VM,
	// registering envoy_on_response). The old f.cc.chunk == nil guard would
	// WRONGLY skip envoy_on_response. The per-route SELECTION already happened
	// ONCE at decode (the matched route does not change mid-stream), so encode
	// reuses the VM and gates only on VM-presence — observably equivalent to
	// upstream's per-phase re-resolution.
	if f.cc == nil || f.vm == nil {
		return envoyhttp.Continue
	}

	// Step 2: build the response_handle context + LUserData.
	f.respCtx = &responseHandleContext{
		headers:   headers,
		cb:        newResponseHandleCallbacksAdapter(f.ecb, f.dcb, f),
		filterRef: f,
	}
	L := f.vm.State()
	respUd := L.NewUserData()
	respUd.Value = f.respCtx
	L.SetMetatable(respUd, L.GetTypeMetatable(responseHandleTypeName))

	// Step 3: hook-presence check.
	if !f.vm.HasGlobalFunc("envoy_on_response") {
		return envoyhttp.Continue
	}

	// Step 4: stats.executions++.
	if f.cc.stats != nil && f.cc.stats.executions != nil {
		f.cc.stats.executions.Inc()
	}

	// Step 5: PRODUCTION COROUTINE DISPATCH (Task 19a NEW).
	//
	// 5a. Mint a child *LState for the encode-side coroutine.
	child, cancel := f.vm.NewThread()
	f.encodeChild = child
	f.encodeChildCancel = cancel

	// 5b. Resolve envoy_on_response.
	fn := L.GetGlobal("envoy_on_response").(*lua.LFunction)

	// 5c. Drive the coroutine.
	state, rerr, _ := f.vm.Resume(child, fn, respUd)
	if rerr != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: envoy_on_response failed: %v", rerr)
		return envoyhttp.Continue
	}
	if state == lua.ResumeYield {
		if f.pendingHTTPCallResume != nil {
			// Sync httpCall yield on the encode side. Close+wait
			// pattern: release the dispatch goroutine, then wait for
			// its Resume to drive the script to completion.
			if f.httpCallReady != nil {
				close(f.httpCallReady)
				f.httpCallReady = nil
			}
			if f.httpCallDone != nil {
				<-f.httpCallDone
				f.httpCallDone = nil
			}
			// Fall through to Continue.
		} else if f.pendingRespBodyResume != nil {
			// Body yield on the encode side. Return Continue so
			// RunEncodeData fires and resumes the coroutine via
			// accumulateResponseBody at endStream. Header mutations
			// land inside the inner Resume driven from EncodeData.
			return envoyhttp.Continue
		} else {
			// Yielded with no pending slot — defensive.
			if f.cc.stats != nil && f.cc.stats.errors != nil {
				f.cc.stats.errors.Inc()
			}
			logf("ERROR lua: envoy_on_response yielded without pending resume slot")
		}
	}

	// Step 6: Continue. No respond-state check on encode side.
	return envoyhttp.Continue
}
