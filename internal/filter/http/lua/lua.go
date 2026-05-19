package lua

// lua.go — boot-time HTTPFilterFactory entry per 22.1 SPEC §4.1 +
// ADR-0072 (boot-time-fail-fast) + ADR-0071 (two-step factory). Wires
// Tasks 2-10 into a fully-functional api.HTTPFilterFactory consumed by
// the HCM filter-chain builder at cmd/envoy-go/main.go.
//
// TASK 10: New() full body wiring lands here. The factory body
// dispatches: (1) tc != nil ADR-0072 boot-time-fail-fast; (2)
// buildCompiledConfig(tc) parses + compiles via Tasks 2-4; (3) ADR-0085
// nil-tolerance guard `if ctx.Stats != nil` then newFilterStats(reg,
// ctx.StatPrefix, lua.StatPrefix) registers the 3-counter HCM-rooted
// stat surface per parent §7.2 + AMEND-2; (4) returns the per-stream
// FilterInstanceFactory closure constructing a fresh *filter bound to
// the SHARED *compiledConfig.
//
// # HTTPFilter{Decoder: f, Encoder: f} per 22.1 SPEC §3.1 #6
//
// lua participates on BOTH decode + encode sides — the SAME *filter
// struct implements StreamDecoderFilter (DecodeHeaders fires
// envoy_on_request hook + respond-state handling per Task 9) AND
// StreamEncoderFilter (EncodeHeaders fires envoy_on_response hook
// per Task 9; :respond() at encode raises AMEND-8 runtime-error).
// Static interface assertions at the var-block below.
//
// # RegisterPerRouteValidator per ADR-0110 single-chokepoint + PLAN D-P6
//
// Per-route validator registration happens BEFORE Freeze() in
// cmd/envoy-go/main.go via the exported RegisterPerRouteValidator
// function (mirrors header_mutation + oauth2 precedent). Do NOT call
// it inside New: New is invoked by the listener constructor which
// runs AFTER Freeze, so any call here would panic. The validator
// one-liner returns the arm-18 PARSE-REJECT "lua: per-route
// configuration is not yet supported (lands in phase 22.3)".

import (
	"context"
	"errors"
	"log"
	"net/http"

	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	lua "github.com/yuin/gopher-lua"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/cluster"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/httpclient"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
	"github.com/esalaine/envoy-go/internal/stats"
)

// defaultMaxBodyBufferedBytes is the 22.2 hardcoded body-buffer cap per
// PLAN Task 7: 16 MiB (16 * 1024 * 1024 = 16777216). Per-stream
// f.maxBodyBufferedBytes initializes from this constant unless test code
// overrides for cap-exceeded-arm testing. Per SPEC §6 arm-21:
// `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`
// (W2 byte-stable wording).
const defaultMaxBodyBufferedBytes = 16 * 1024 * 1024

// filterName is the registered name for the lua filter per ADR-0070 +
// 22.1 SPEC §4.1 boot-registration. Matches the listener config
// http_filters[].name string + the HCM-chain dispatch identifier.
const filterName = "envoy.filters.http.lua"

// TypeURL is the proto typed_config TypeURL for envoy.filters.http.lua
// per ADR-0143 SN1 + the v1.32.4 / v1.37.x proto package
// envoy.extensions.filters.http.lua.v3.Lua. Byte-exact constant;
// regression-pinned at TestTypeURL_Matches (lua_test.go).
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"

// filterStats holds the 3-counter HCM-rooted stat surface for one lua
// filter instance per 22.1 SPEC §4.2 + parent §7 + AMEND-3.
//
//   - errors — upstream-parity (ALL_LUA_FILTER_STATS macro arm 1).
//     Incremented on script run failure + envoy_on_request /
//     envoy_on_response runtime errors.
//   - executions — upstream-parity (ALL_LUA_FILTER_STATS macro arm 2).
//     Incremented per envoy_on_request / envoy_on_response invocation
//     (NOT per-success — matches upstream).
//   - respondCalls — envoy-go-strict extension per AMEND-3. Incremented
//     when :respond() fires from a Lua hook. Departure record at
//     BEHAVIOR_CONTRACT.md §13.6 row 2 at Task 16.
//
// Allocated unconditionally at lua.New time via newFilterStats(reg,
// ctx.StatPrefix, luaProto.StatPrefix) per ADR-0085 nil-tolerance
// (caller's `if ctx.Stats != nil` guard). See stats.go for the
// constructor + the 3 byte-exact statName* constants per ADR-0143
// SN2-reuse.
type filterStats struct {
	errors       *stats.Counter
	executions   *stats.Counter
	respondCalls *stats.Counter

	// Task 7 (phase 22.2 IMPL) — 2 of 5 NEW envoy-go-strict counters
	// land here (the remaining 3 — httpcall_total + httpcall_failures +
	// httpcall_timeouts — land at Task 14). Per SPEC §7.1 + the Task 7
	// dispatch outline: counters are added here at Task 7 rather than
	// deferred to Task 14 so the body-bridge tests can assert against
	// them at this Task. Task 14's stats.go extension appends the 3
	// remaining counters + the byte-stable wire-name constants.
	//
	//   - bodyBufferedBytesTotal — cumulative bytes accumulated across
	//     DecodeData + EncodeData chunks per stream. Incremented by
	//     DecodeData(data, _) / EncodeData(data, _) with delta=len(data).
	//   - coroutineYieldsTotal — per-yield event counter; ONCE per yield
	//     site, NOT per Resume site (semantic discipline per Task 7
	//     dispatch outline).
	bodyBufferedBytesTotal *stats.Counter
	coroutineYieldsTotal   *stats.Counter

	// Task 11 (phase 22.2 IMPL) — 3 of 5 NEW envoy-go-strict counters
	// for the httpCall bridge surface land here. Per SPEC §7.1 + §11.7 D6
	// closure + AMEND-22.2-3:
	//
	//   - httpcallTotal — incremented on EVERY :httpCall() dispatch
	//     (sync + async). Cumulative outbound-call observability per
	//     SPEC §7.1.
	//
	//   - httpcallFailures — SYNC-ONLY per AMEND-22.2-3. Incremented on
	//     sync transport-failure + sync 4xx/5xx (per upstream synthetic-
	//     503 disposition). Async fire-and-forget failures are
	//     UNOBSERVABLE at the filter-stats layer per upstream parity.
	//
	//   - httpcallTimeouts — SYNC-ONLY per AMEND-22.2-3. Incremented on
	//     sync timeout firings. Async timeout invisible per upstream
	//     parity.
	//
	// Counters are declared on the struct at this Task; production
	// registration is deferred to Task 14's stats.go extension where
	// the byte-stable wire-name constants land + the newFilterStats
	// constructor is extended to allocate them. Tests at this Task hand-
	// roll the *stats.Counter handles to exercise the increment surface.
	httpcallTotal    *stats.Counter
	httpcallFailures *stats.Counter
	httpcallTimeouts *stats.Counter
}

// New is the HTTPFilterFactory exposed at boot per ADR-0071 + ADR-0072.
//
// Step sequence (per 22.1 SPEC §4.2 + parent §7.2 + AMEND-2):
//
//  1. tc != nil per ADR-0072 boot-time-fail-fast. A typed_config-less
//     lua filter listing has no behavioral effect; surface configuration
//     mistakes at boot rather than per-stream.
//  2. buildCompiledConfig(tc) parses the proto envelope + walks the
//     18-arm PARSE-REJECT roster (Task 2) + resolves the 4-arm
//     DataSource (Task 3) + compiles the script via
//     internal/lua/CompileScript (Task 4). Returns a fresh
//     *compiledConfig holding the chunk + compileCache + sandbox.
//  3. Per ADR-0085 nil-tolerance: guard `if ctx.Stats != nil` before
//     allocating the 3-counter stat surface. The Lua proto's
//     `StatPrefix` field qualifies the `<config_stat_prefix>` slot in
//     the HCM-rooted template per parent §7.2 + AMEND-2:
//     `http.<ctx.StatPrefix>.lua.<lua.StatPrefix>.<stat>`.
//     The re-unmarshal here is cheap (one anypb decode at boot time);
//     the previous buildCompiledConfig call already validated the
//     wire-bytes parse cleanly, so the error path is unreachable.
//  4. Returns the per-stream FilterInstanceFactory closure that
//     allocates a fresh *filter bound to the SHARED *compiledConfig.
//     Per 22.1 SPEC §3.1 #6 the returned HTTPFilter has Decoder=f AND
//     Encoder=f — the lua filter participates on BOTH decode (Task 9
//     envoy_on_request dispatch + respond-state handling) AND encode
//     (Task 9 envoy_on_response dispatch).
//
// Per-route validator: NOT registered here. Registration lands at boot
// via the exported RegisterPerRouteValidator function (called from
// cmd/envoy-go/main.go BEFORE httpReg.Freeze() — see the comment block
// at RegisterPerRouteValidator below + the header_mutation + oauth2
// precedent). New is called by the listener constructor AFTER Freeze;
// any RegisterPerRouteValidator call from inside New would panic.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	// Step 1: ADR-0072 boot-time-fail-fast.
	if tc == nil {
		return nil, errors.New(parseRejectTypedConfigRequired)
	}

	// Step 2: parse + validate + compile.
	cc, err := buildCompiledConfig(tc)
	if err != nil {
		return nil, err
	}

	// Step 3: 3-counter stat-surface registration per parent §7.2 +
	// AMEND-2. Guarded by ADR-0085 nil-tolerance — test/synthetic
	// callers may pass FactoryCtx{} with Stats==nil; production callers
	// per ADR-0061 LBP-1 always supply a non-nil *stats.Registry pre-
	// Freeze. The re-unmarshal extracts `Lua.StatPrefix` for the
	// `<config_stat_prefix>` slot; buildCompiledConfig already
	// validated the Any decodes cleanly, so the error return path is
	// unreachable but defensively ignored (passing "" to newFilterStats
	// would land literal consecutive-dot names — still operationally
	// correct, just not the operator-intended naming).
	if ctx.Stats != nil {
		var luaCfg luav3.Lua
		_ = tc.UnmarshalTo(&luaCfg) // already validated by buildCompiledConfig
		cc.stats = newFilterStats(ctx.Stats, ctx.StatPrefix, luaCfg.GetStatPrefix())
	}

	// Task 11 (phase 22.2 IMPL) — capture the shared *httpclient.Client +
	// *cluster.Manager from FactoryCtx into closure for the :httpCall
	// bridge consumer (FIRST co-consumer of Task 4's ADR-0177 IN-PLACE
	// AMENDMENT ClusterDispatch — R5 RATIFIED at Task 4 + actualized at
	// Task 11). Both may be nil under ADR-0085 nil-tolerance at the
	// synthetic test path; the bridge LGFunction nil-checks before
	// invoking ClusterDispatch.
	httpClient := ctx.HTTPClient
	clusterMgr := ctx.ClusterManager

	// Step 4: per-stream FilterInstanceFactory closure. The *filter is
	// fresh per stream (per-stream-goroutine isolation per ADR-0071);
	// cc is closure-captured + shared across streams (immutable post-
	// New).
	return func() envoyhttp.HTTPFilter {
		f := &filter{cc: cc, httpClient: httpClient, clusterMgr: clusterMgr}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f, // envoy_on_request fires at DecodeHeaders per Task 9
			Encoder: f, // envoy_on_response fires at EncodeHeaders per Task 9
		}
	}, nil
}

// RegisterPerRouteValidator registers the lua per-route validator with
// the supplied HTTPRegistry. MUST be called BEFORE httpReg.Freeze() in
// cmd/envoy-go/main.go — the registry rejects registrations after
// Freeze, and New is called during listener construction (after Freeze).
// Mirrors the header_mutation + oauth2 precedent.
//
// TASK 1 SKELETON: declared so that the Task 10 boot-registration step
// can wire `lua.RegisterPerRouteValidator(httpReg)` before
// httpReg.Register(lua.TypeURL, lua.New). The validator body itself
// returns the arm-18 PARSE-REJECT at 22.1; 22.3 IMPL replaces the body
// with the 9th-canonical per-route shape validator.
func RegisterPerRouteValidator(reg interface {
	RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)
}) {
	reg.RegisterPerRouteValidator(filterName, validatePerRouteLua)
}

// validatePerRouteLua is the per-route arm-18 PARSE-REJECT one-liner
// per ADR-0110 single-chokepoint + parent §6.2 arm 18 + PLAN D-P6.
// Wording is byte-pinned for the regression-asserted error-string
// surface at Task 11 fuzzer + cross-side fixture coverage; 22.3 IMPL
// replaces the body with the 9th-canonical per-route shape validator.
func validatePerRouteLua(_ proto.Message) error {
	return errors.New("lua: per-route configuration is not yet supported (lands in phase 22.3)")
}

// filter is the per-stream lua filter instance per 22.1 SPEC §3.1 #6.
// Both-sides filter: implements StreamDecoderFilter (Task 9 envoy_on_request
// hook + respond-state capture) AND StreamEncoderFilter (Task 9
// envoy_on_response hook + AMEND-8 respond-on-encode runtime-reject).
//
// Per ADR-0071's single-goroutine-per-stream invariant, the per-instance
// state is race-free without synchronization. The cc pointer is closure-
// captured at New time and is immutable post-construction; the vm /
// reqCtx fields are decoded/encoded entirely from inside the chain
// dispatch goroutine (no concurrent access).
type filter struct {
	// cc is the SHARED per-listener compiledConfig (chunk + compileCache
	// + sandbox + stats). Closure-captured at New time per Task 10
	// FilterInstanceFactory. Nil-tolerant on the decode/encode hot path
	// (nil cc OR nil cc.chunk → pass through without VM construction).
	cc *compiledConfig

	// vm is the per-stream gopher-lua VM. Allocated at DecodeHeaders
	// entry per 22.1 SPEC §4.3 step 1; released at OnDestroy via
	// vm.Close. May be nil if DecodeHeaders short-circuited at the
	// nil-chunk pass-through (no VM construction) — OnDestroy guards.
	vm *luaprim.VM

	// reqCtx is the per-stream request_handle Go-side context, set up
	// in DecodeHeaders + read by encode_headers.go to consult the
	// respondCaptured field (defensive — encode-side dispatcher does
	// NOT consult respondCaptured at 22.1 since decode-side
	// SendLocalReply returns StopIteration before EncodeHeaders fires).
	reqCtx *requestHandleContext

	// respCtx is the per-stream response_handle Go-side context, set up
	// at EncodeHeaders entry per Task 7 (the 22.1 dispatcher constructed
	// a local respCtx but did not persist it on *filter; Task 7 stores
	// it on the filter so the body-bridge LGFunctions can resolve the
	// owning *filter via the userdata's filterRef back-pointer).
	respCtx *responseHandleContext

	// dcb / ecb are the framework-supplied per-stream callbacks. Stored
	// for SendLocalReply (decode-side respond capture path) + future
	// async-resume needs.
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	// --- Task 7 (phase 22.2 IMPL) — body-bridge per-stream state ---
	//
	// Per SPEC §4.1 + §11.3 D3 closure: the lua filter's per-stream
	// state accumulates body bytes across DecodeData / EncodeData calls
	// (mirrors ext_authz `f.body []byte` + ext_proc `f.decodeBodyBuf
	// []byte` per §11.3.2). At endStream the bridge's :body() defensive-
	// copies into a Lua-owned LString per §11.3.3 (gopher-lua's LString
	// IS an immutable Go string, safe across coroutine yield/resume +
	// HCM dispatch goroutine lifetimes).
	//
	// All fields are per-stream-goroutine-private per ADR-0071
	// single-goroutine-per-stream invariant — no synchronization
	// required.

	// decodedBodyBytes accumulates request-side body bytes across
	// DecodeData calls. Allocated lazily on first DecodeData; nil
	// before the first DecodeData fires. The 22.2 SPEC §11.3.2 prior-
	// patterns analog (ext_authz f.body, ext_proc f.decodeBodyBuf).
	decodedBodyBytes []byte

	// decodedBodyChunks holds per-DecodeData chunk slices (defensive
	// copies; each chunk is independently retained for the
	// :bodyChunks() iterator). Allocated lazily on first DecodeData.
	decodedBodyChunks [][]byte

	// encodedBodyBytes / encodedBodyChunks mirror the decode-side
	// accumulators for the response side per Task 7 SPEC §3.4 symmetric
	// response_handle:body() / :bodyChunks() bridge methods.
	encodedBodyBytes  []byte
	encodedBodyChunks [][]byte

	// bodyReady reports whether decode-side endStream has fired (the
	// terminal DecodeData(_, true) signal — synthetic empty-terminal
	// per ADR-0128 connection.go:505-509 + §11.3.1). Pre-endStream
	// :body() calls suspend via YieldFromBridge per §11.1 D2 closure.
	bodyReady bool

	// respBodyReady is the encode-side mirror of bodyReady. Reported
	// true once EncodeData(_, true) fires the terminal endStream signal.
	respBodyReady bool

	// pendingBodyResume holds the suspended request-side coroutine's
	// child *lua.LState awaiting endStream-resume. Set by the bridge's
	// requestHandleBody LGFunction immediately before YieldFromBridge;
	// consumed by DecodeData at endStream signal — calls
	// vm.Resume(child, nil, lua.LString(...)) to wake the coroutine
	// with the accumulated bytes. Cleared post-Resume to prevent double-
	// dispatch.
	pendingBodyResume *lua.LState

	// pendingRespBodyResume is the encode-side mirror. Set by
	// responseHandleBody; consumed by EncodeData at endStream.
	pendingRespBodyResume *lua.LState

	// --- Task 19a (phase 22.2 IMPL) — production HCM coroutine
	// orchestration per ADR-0192 §Decision body anticipation + §11.1 D2
	// closure (closes Task 7 deferred production-HCM-orchestration gap).
	//
	// The decode/encode dispatchers (decode_headers.go /
	// encode_headers.go) mint a child *LState via vm.NewThread() per
	// invocation + drive envoy_on_request / envoy_on_response via
	// vm.Resume(child, fn, ud). The child + its context.CancelFunc are
	// stashed here so OnDestroy can release any ctx-attached child loop
	// at stream destroy.
	//
	// Per gopher-lua state.go:1618 the cancel func is non-nil only when
	// the parent LState carries a context (via vm.State().SetContext);
	// at 22.2 the parent is constructed without an attached context, so
	// cancel will typically be nil — but we stash defensively for the
	// future-Task case where ctx-cancellation lands at the VM seam.

	// decodeChild is the per-stream decode-side coroutine *LState minted
	// at DecodeHeaders entry. Nil before DecodeHeaders fires + after
	// OnDestroy.
	decodeChild *lua.LState

	// decodeChildCancel is the context.CancelFunc paired with
	// decodeChild — invoked at OnDestroy to release any ctx-attached
	// child loop. May be nil per gopher-lua's NewThread() return contract.
	decodeChildCancel context.CancelFunc

	// encodeChild + encodeChildCancel are the encode-side mirrors,
	// minted at EncodeHeaders entry.
	encodeChild       *lua.LState
	encodeChildCancel context.CancelFunc

	// maxBodyBufferedBytes is the per-stream body-buffer cap per
	// SPEC §6 arm-21 + W2 byte-stable wording. Initialized to
	// defaultMaxBodyBufferedBytes (16 MiB) lazily on first DecodeData
	// (zero value means uninitialized). Test code may overwrite to a
	// smaller value to exercise the over-cap arm without allocating
	// 16 MiB worth of test data.
	maxBodyBufferedBytes int

	// --- Task 11 (phase 22.2 IMPL) — httpCall bridge per-stream state ---
	//
	// Per SPEC §3.4 + §11.7 D6 closure + AMEND-22.2-3 (FIRST co-consumer
	// call site of Task 4's ADR-0177 IN-PLACE AMENDMENT ClusterDispatch).
	// The lua filter's per-stream *filter holds references to the SHARED
	// *httpclient.Client + *cluster.Manager threaded via FactoryCtx (set
	// at New time + closure-captured for cross-stream sharing).
	//
	// Per-stream fields here are the httpCall yield/resume scaffolding +
	// the cluster manager / http client pointers. Per ADR-0071 single-
	// goroutine-per-stream invariant, the per-instance state is race-free
	// EXCEPT the httpCall dispatch goroutine (which marshals its
	// result back via the per-filter pending-resume slot — gopher-lua's
	// child *LState Resume from a non-owning goroutine is safe ONLY when
	// no other goroutine is concurrently running the same parent state,
	// see §11.1 D2 closure + state.go:2157).
	//
	// httpClient + clusterMgr may be nil under ADR-0085 nil-tolerance at
	// the synthetic test path (test code that constructs a *filter without
	// HTTP-call exercise).
	httpClient *httpclient.Client
	clusterMgr *cluster.Manager

	// pendingHTTPCallResume holds the suspended sync-httpCall coroutine's
	// child *lua.LState awaiting the dispatch goroutine's Resume call. Set
	// by requestHandleHttpCall / responseHandleHttpCall immediately before
	// YieldFromBridge; consumed by the dispatch goroutine on completion —
	// calls vm.Resume(child, nil, headersTable, bodyString) or
	// vm.Resume(child, nil, lua.LNil, lua.LString(errStr)). Cleared on
	// Resume to prevent double-dispatch.
	//
	// Per the goroutine-safety discipline at §11.1 D2: the dispatch
	// goroutine's Resume call MUST NOT race with the outer parent.Resume
	// call site (decode_headers.go / encode_headers.go). Coordination at
	// 22.2 is via the test goroutine waiting on the httpCallDone signal;
	// production HCM dispatch coordination lands at a future phase (the
	// HCM dispatcher schedules the response-ready callback that then
	// drives Resume from the dispatch goroutine).
	pendingHTTPCallResume *lua.LState

	// httpCallReady gates the dispatch goroutine's Resume call on the
	// outer parent.Resume call returning ResumeYield. Per gopher-lua's
	// non-goroutine-safe Resume + child-state-touching discipline, the
	// dispatch goroutine MUST NOT call parent.Resume until the outer
	// Resume call has unwound. The caller (test code at 22.2; production
	// HCM dispatch coordination at a future phase) closes this channel
	// AFTER observing ResumeYield from the outer parent.Resume — the
	// dispatch goroutine selects on this channel before calling Resume,
	// establishing the happens-before edge that satisfies the race
	// detector.
	//
	// Allocated lazily in runHTTPCall at sync-path setup; nil before any
	// sync httpCall fires.
	httpCallReady chan struct{}

	// httpCallDone is signaled by the dispatch goroutine when it has
	// finished its Resume call (sync httpCall path only). Tests wait on
	// this channel to ensure the resume has fired before asserting on
	// script results.
	//
	// Allocated lazily in runHTTPCall at sync-path setup; nil before any
	// sync httpCall fires.
	httpCallDone chan struct{}

	// --- Task 13 (phase 22.2 IMPL) — filterState bridge per-stream state ---
	//
	// Per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4: the lua filter's
	// per-stream `map[string]any` accessor consumed by the bridge surface
	// `streamInfo:filterState():get/set`. Lazily allocated on first :set
	// from the bridge LGFunction (filterstate.go); nil before any :set
	// fires.
	//
	// Per-stream lifecycle: created at filter struct allocation
	// (implicitly nil until lazy-init); released at OnDestroy (set to
	// nil for GC). Cross-stream isolation is via the per-stream *filter
	// — N parallel streams each have a separate map; no shared state.
	//
	// 2 envoy-go-strict departures from upstream are anchored at this
	// field's bridge surface per AMEND-22.2-4 (recorded at
	// BEHAVIOR_CONTRACT.md §13.6 at Task 19 atomic landing):
	//
	//   1. :set(name, value) exposed at Lua surface (upstream
	//      FilterStateWrapper is strictly read-only).
	//   2. :get(name) returns typed Lua values (LString / LNumber /
	//      LBool / LTable recursive); upstream returns
	//      serializeAsString strings always.
	filterState map[string]any
}

// Static interface conformance assertions. The *filter implements BOTH
// the decoder and encoder side per 22.1 SPEC §3.1 #6 + Task 10's
// `HTTPFilter{Decoder: f, Encoder: f}` wiring.
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// SetDecoderCallbacks stores the per-stream callbacks reference for
// the :respond() SendLocalReply path consumed by decode_headers.go.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks stores the per-stream callbacks reference for
// the encode-side dispatcher consumed by encode_headers.go.
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeData accumulates the decode-side body per Task 7 (phase 22.2
// IMPL) per SPEC §3.4 + §4.1 + §11.3 D3 closure. On terminal endStream
// the per-stream bodyReady flag fires + any suspended request-side
// coroutine (pending :body() awaiting endStream) is resumed with the
// accumulated bytes. Counter bodyBufferedBytesTotal increments by
// len(data) per call per SPEC §7.1.
//
// Body-bridge logic lives at body.go's accumulateRequestBody + the
// per-:body() yield/resume orchestration; the lua-filter callback simply
// dispatches to that helper.
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	accumulateRequestBody(f, data, endStream)
	return envoyhttp.DataContinue
}

// DecodeTrailers attaches the received request-side trailers map onto
// the per-stream request_handle context per Task 8 (phase 22.2 IMPL) +
// SPEC §3.4 + §2.2 + PLAN Q2 lazy-availability discipline. Lua scripts
// invoking `request_handle:trailers()` AFTER this callback fires
// observe a non-nil userdata wrapping the trailers map; pre-firing they
// observe nil.
//
// Nil-tolerant on reqCtx: if DecodeHeaders short-circuited at the nil-
// chunk pass-through (no per-stream context exists), the callback no-
// ops + returns Continue. Defensive against the synthetic test path +
// the framework's TrailersContinue contract.
//
// The trailers map is shared (not defensive-copied) — the bridge
// methods mutate the underlying http.Header in place, observable on
// subsequent filters in the chain (parallels headers-mutation semantics
// at 22.1).
func (f *filter) DecodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	if f.reqCtx != nil {
		f.reqCtx.trailers = trailers
	}
	return envoyhttp.TrailersContinue
}

// EncodeData accumulates the encode-side body per Task 7 (phase 22.2
// IMPL) — symmetric to DecodeData for the response_handle:body() bridge.
// Counter bodyBufferedBytesTotal increments by len(data) per call.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	accumulateResponseBody(f, data, endStream)
	return envoyhttp.DataContinue
}

// EncodeTrailers attaches the received response-side trailers map onto
// the per-stream response_handle context per Task 8 (phase 22.2 IMPL).
// Symmetric to DecodeTrailers for the encode side.
//
// Nil-tolerant on respCtx: if EncodeHeaders never fired (e.g.
// SendLocalReply short-circuit from a decode-side :respond()), the
// callback no-ops + returns Continue.
func (f *filter) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	if f.respCtx != nil {
		f.respCtx.trailers = trailers
	}
	return envoyhttp.TrailersContinue
}

// OnDestroy releases the per-stream *internal/lua.VM via vm.Close per
// 22.1 SPEC §3.4 per-stream lifecycle. Idempotent — guard against
// double-close + against nil vm (the latter occurs when DecodeHeaders
// short-circuited at the nil-chunk pass-through without ever
// constructing a VM).
//
// Task 13 (phase 22.2 IMPL) — additionally clears the per-stream
// filterState map for GC + cross-stream isolation discipline per SPEC
// §11.8 D4 closure. Defensive against unintended retention through the
// cb adapter's *filter back-pointer or any externally-held pointers
// (the *filter struct itself is held by the framework's per-stream
// reference; clearing the map releases the entries for GC even if the
// *filter outlives OnDestroy).
func (f *filter) OnDestroy() {
	// Task 19a (phase 22.2 IMPL) — release any ctx-attached child loops
	// minted at DecodeHeaders / EncodeHeaders per ADR-0191 §Context.
	// Each CancelFunc is non-nil only when the parent LState carries a
	// context (gopher-lua state.go:1618); defensive-nil at both sites.
	if f.decodeChildCancel != nil {
		f.decodeChildCancel()
		f.decodeChildCancel = nil
	}
	if f.encodeChildCancel != nil {
		f.encodeChildCancel()
		f.encodeChildCancel = nil
	}
	f.decodeChild = nil
	f.encodeChild = nil
	if f.vm != nil {
		f.vm.Close()
		f.vm = nil
	}
	f.filterState = nil
}

// logf is the package-level logger used by decode_headers.go +
// encode_headers.go for errors-channel diagnostics. Tagged via stdlib
// log.Printf with the canonical "lua:" prefix (mirrors the bridge-
// methods' "<LEVEL> lua:" pattern from the :logXxx methods). Indirected
// via a package var to make test-side capture trivial (override _ = log
// in the rare case a test wants to suppress noise; default goes to
// log.Default()).
var logf = log.Printf
