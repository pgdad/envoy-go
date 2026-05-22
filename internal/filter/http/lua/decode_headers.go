package lua

// decode_headers.go — DecodeHeaders dispatcher per 22.1 SPEC §4.3 +
// 22.2 production HCM coroutine orchestration per ADR-0192 §Decision
// body anticipation + §11.1 D2 closure (Task 19a closes Task 7 deferred
// production-HCM-orchestration gap).
//
// Constructs a per-stream gopher-lua VM at request-headers arrival,
// installs the bridge metatables + the request_handle userdata,
// executes the operator-supplied script top-level (to define
// envoy_on_request / envoy_on_response globals), then dispatches the
// `envoy_on_request` hook AS A COROUTINE (via vm.NewThread +
// vm.Resume) so that bridge LGFunctions calling YieldFromBridge (e.g.,
// :body() pre-endStream, :httpCall sync) can suspend the script per
// §11.1 D2 closure.
//
// # Dispatch sequence (Task 19a coroutine orchestration)
//
//  1. Nil-chunk pass-through (unchanged from 22.1).
//
//  2. Construct per-stream VM with the SHARED sandbox config.
//
//  3. Install bridge metatables (unchanged).
//
//  4. Build the request_handle context + LUserData (unchanged).
//
//  5. vm.Run(cc.chunk) — executes script top-level (unchanged).
//
//  6. Hook-presence check — if envoy_on_request absent, Continue.
//
//  7. stats.executions++.
//
//  8. PRODUCTION COROUTINE DISPATCH (Task 19a NEW):
//
//     a. f.decodeChild, f.decodeChildCancel = vm.NewThread() — mints a
//        child *LState as a coroutine state, sharing globals with the
//        parent. The cancel func is stashed on the filter; OnDestroy
//        invokes it to release any ctx-attached child loop.
//
//     b. fn = parent.GetGlobal("envoy_on_request").(*lua.LFunction) —
//        resolves the script-defined function.
//
//     c. state, _, _ = vm.Resume(child, fn, reqUd) — drives the
//        coroutine. Three return shapes:
//          - ResumeOK: script ran to completion; headers mutated;
//            respond may have been captured. Proceed to step 9.
//          - ResumeYield: script suspended on a bridge YieldFromBridge
//            site. Two sub-cases distinguished by which pending-resume
//            slot the bridge stashed L into:
//              - f.pendingBodyResume != nil: script suspended on :body()
//                awaiting endStream. Return Continue so DecodeData can
//                fire and resume the coroutine via accumulateRequestBody.
//                Headers mutated by the script's post-:body() code WILL
//                land on the request envelope BEFORE RunAction sends
//                upstream (the chain's RunDecodeData runs synchronously
//                ahead of RunAction).
//              - f.pendingHTTPCallResume != nil: script suspended on
//                sync :httpCall(). The dispatch goroutine waits on
//                f.httpCallReady — close it, then synchronously wait on
//                f.httpCallDone so the script can run to completion
//                before this DecodeHeaders call returns. After the
//                wait, the script HAS mutated headers + respond may
//                have been captured.
//          - ResumeError: log + Continue (the script error is captured
//            via stats.errors).
//
//  9. Respond-state check (unchanged from 22.1). If respondCaptured is
//     non-nil, SendLocalReply + StopIteration.
//
// 10. Otherwise Continue.
//
// # Why return Continue (not StopIteration) on body-yield
//
// envoy-go's chain serializes Headers→Data: RunDecodeHeaders must
// return before RunDecodeData fires. If we returned StopIteration here
// (parking the chain on decodeResumeCh), the HCM's body-read loop
// would never start and the body bytes would never flow into
// DecodeData — deadlock.
//
// The trade-off: returning Continue means subsequent decode-side
// filters see the request headers BEFORE Lua's post-:body() mutations.
// For fixture-0027's topology (lua → router), this is benign: the
// router's RunAction reads headers AFTER chain.RunDecodeData has
// completed (i.e. AFTER our DecodeData(endStream=true) has resumed the
// script and the script has mutated the headers). General-case multi-
// lua-chain topologies that depend on inter-filter header-ordering on
// the decode side need a future framework change (e.g. defer headers-
// processing until after-body OR cooperative ContinueDecoding for the
// chain's parkDecode site). Tracked at REVIEW.md for the next phase.

import (
	"net/http"

	lua "github.com/yuin/gopher-lua"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
)

// DecodeHeaders implements the decode-side dispatcher per 22.1 SPEC
// §4.3 + 22.2 production coroutine orchestration (Task 19a closure of
// Task 7 deferred production-HCM-orchestration gap).
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1: nil-cc pass-through (defensive) + per-route 3-tier dispatch
	// (phase 22.3 Task 3). resolveDecodeScript selects the chunk ONCE for this
	// stream per the load-bearing dispatch invariant: it resolves the matched
	// route's per-route override (disabled / name / source_code) or falls back
	// to the listener default (f.cc.chunk). A disabled route OR a nil chunk
	// (nil listener default, dangling name, or D1-REFUTED arm-5 silent-no-op)
	// short-circuits BEFORE VM construction → no VM is built → encode's
	// f.vm == nil guard naturally skips both hooks (upstream nullptr parity).
	if f.cc == nil {
		return envoyhttp.Continue
	}
	chunk, disabled := f.resolveDecodeScript()
	if disabled || chunk == nil {
		return envoyhttp.Continue
	}

	// Step 2: construct per-stream VM with the SHARED sandbox config.
	f.vm = luaprim.NewVM(
		luaprim.WithSandboxConfig(f.cc.sandbox),
	)

	// Step 3: install bridge metatables + pairs shim.
	L := f.vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installTrailersMetatable(L)
	installStreamInfoMetatable(L)
	installMetadataMetatable(L)
	installDynamicMetadataMetatable(L)
	installConnectionMetatable(L)
	installSSLMetatable(L)
	installPublicKeyWrapperMetatable(L)
	installFilterStateMetatable(L)
	installPairsShim(L)

	// Step 4: build the request_handle context + LUserData.
	f.reqCtx = &requestHandleContext{
		headers:   headers,
		cb:        newRequestHandleCallbacksAdapter(f.dcb, f),
		filterRef: f,
	}
	reqUd := L.NewUserData()
	reqUd.Value = f.reqCtx
	L.SetMetatable(reqUd, L.GetTypeMetatable(requestHandleTypeName))

	// Step 5: execute the per-route-RESOLVED script top-level. Errors →
	// stats.errors + log + Continue.
	if err := f.vm.Run(chunk); err != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: script run failed: %v", err)
		return envoyhttp.Continue
	}

	// Step 6: hook-presence check.
	if !f.vm.HasGlobalFunc("envoy_on_request") {
		return envoyhttp.Continue
	}

	// Step 7: stats.executions++ (upstream-parity).
	if f.cc.stats != nil && f.cc.stats.executions != nil {
		f.cc.stats.executions.Inc()
	}

	// Step 8: PRODUCTION COROUTINE DISPATCH (Task 19a NEW).
	//
	// 8a. Mint a child *LState as a coroutine state. The child shares
	// globals (`G`+`Env`) with the parent per gopher-lua state.go:1614.
	child, cancel := f.vm.NewThread()
	f.decodeChild = child
	f.decodeChildCancel = cancel

	// 8b. Resolve the script-defined envoy_on_request function. Safe
	// type-assertion: HasGlobalFunc already verified type==Function.
	fn := L.GetGlobal("envoy_on_request").(*lua.LFunction)

	// 8c. Drive the coroutine. ResumeYield branches on which bridge
	// stashed the pending-resume slot (body vs httpCall).
	state, rerr, _ := f.vm.Resume(child, fn, reqUd)
	if rerr != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: envoy_on_request failed: %v", rerr)
		// Fall through to respond-state check.
	}
	if state == lua.ResumeYield {
		if f.pendingHTTPCallResume != nil {
			// Sync httpCall yield. The dispatch goroutine is blocked on
			// f.httpCallReady; close it so the goroutine can call Resume
			// back. Then wait on httpCallDone — the dispatch goroutine's
			// Resume call runs the script's continuation to completion
			// (mutating headers + possibly capturing respond). After the
			// wait, the script HAS finished + state observable via
			// reqCtx.respondCaptured / mutated headers.
			if f.httpCallReady != nil {
				close(f.httpCallReady)
				f.httpCallReady = nil
			}
			if f.httpCallDone != nil {
				<-f.httpCallDone
				f.httpCallDone = nil
			}
			// Fall through to respond-state check.
		} else if f.pendingBodyResume != nil {
			// Body yield. Return Continue so RunDecodeData can fire and
			// resume the coroutine via accumulateRequestBody at
			// endStream. The script's post-:body() code (including any
			// header mutations + respond capture) lands inside the
			// inner Resume driven from DecodeData, BEFORE RunAction
			// sends upstream. No respond can be captured at this point
			// yet (the script hasn't reached the respond call site).
			return envoyhttp.Continue
		} else {
			// Yielded with no pending slot — defensive; treat as error.
			if f.cc.stats != nil && f.cc.stats.errors != nil {
				f.cc.stats.errors.Inc()
			}
			logf("ERROR lua: envoy_on_request yielded without pending resume slot")
		}
	}

	// Step 9: respond-state check.
	if f.reqCtx.respondCaptured != nil {
		if f.cc.stats != nil && f.cc.stats.respondCalls != nil {
			f.cc.stats.respondCalls.Inc()
		}
		rs := f.reqCtx.respondCaptured
		if f.dcb != nil {
			f.dcb.SendLocalReply(rs.status, rs.body, rs.headers)
		}
		return envoyhttp.StopIteration
	}

	// Step 10: no respond; pass through.
	return envoyhttp.Continue
}
