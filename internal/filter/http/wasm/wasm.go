package wasm

// wasm.go — Task 8 skeleton for the `envoy.filters.http.wasm` HTTP filter
// per 25.1 SPEC §4.1 + §4.3. Lands:
//
//   - `TypeURL` (canonical wire URL per parent §6.2)
//   - `filterName` (canonical Envoy filter name per ADR-0070)
//   - `filter` struct + `capturedLocalResponse` (per-stream state shape;
//     no behavioral methods at Task 8 — DecodeHeaders/EncodeHeaders/
//     OnDestroy land at Task 12)
//   - Static interface assertions (StreamDecoderFilter + StreamEncoderFilter
//     per 25.1 SPEC §4.3 both-sides shape)
//   - `New` factory STUB returning sentinel `errFactorySkeleton` (Task 9
//     lands the real parse via buildCompiledConfig; Task 11+12 wire the
//     per-stream dispatch)
//   - `validatePerRouteWasm` arm-18 PARSE-REJECT one-liner (25.3 IMPL
//     replaces with the real per-route shape validator)
//   - `RegisterPerRouteValidator` exported boot-time hook for
//     cmd/envoy-go/main.go (Task 13)
//
// Mirrors the phase-22.1 `internal/filter/http/lua` precedent at Task 8 of
// that phase (`lua.go` skeleton). The interface choices, factory shape,
// per-route validator chokepoint, and stat-registration discipline all
// follow lua's Task-8 shape exactly.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	wasmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filterstate"
	"github.com/pgdad/envoy-go/internal/wasm"
)

// TypeURL is the proto typed_config TypeURL for `envoy.filters.http.wasm`
// per 25.1 SPEC §4.1 + ADR-0143 SN1 + the v1.32.4 / v1.37.x proto package
// `envoy.extensions.filters.http.wasm.v3.Wasm`. Byte-exact constant;
// regression-pinned at TestTypeURL (wasm_test.go).
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm"

// filterName is the canonical Envoy filter name per ADR-0070 +
// 25.1 SPEC §4.1 boot-registration. Matches the listener config
// `http_filters[].name` string + the HCM-chain dispatch identifier.
// Identical to upstream `envoy.filters.http.wasm`.
const filterName = "envoy.filters.http.wasm"

// parseRejectTypedConfigRequired re-exports compiled_config.go's arm-1
// constant for the New factory body. Arm 1 (typed_config required) fires
// at the factory entry BEFORE buildCompiledConfig is called — preserving
// the byte-stable wording at the boot-time-fail-fast surface per ADR-0072.
// The compiled_config.go declaration is the canonical source-of-truth;
// no separate constant here.

// NOTE: The Task 8 forward-stub `compiledConfig` type was REMOVED at Task 9
// when the canonical declaration landed in compiled_config.go (Go forbids
// duplicate type declarations in a single package). The Task 9 IMPL holds
// the full struct: module + compileCache + sandbox + pluginName +
// rootContextID + vmConfig + pluginConfig + stats. The *filter.cfg field
// below continues to reference *compiledConfig — that type now resolves
// to the canonical declaration in compiled_config.go.

// Arm 18 LIFTED (phase-25.3 Task 7): the 25.1/25.2 arm-18 deferral constant
// parseRejectPerRouteUnsupported is RETIRED. validatePerRouteWasm now validates
// the per-route Wasm shape (see below); a valid per-route config ACCEPTS, an
// invalid one rejects with the SAME byte-stable buildCompiledConfig wording
// (single source of truth). There is no longer a standalone "per-route not yet
// supported" wording.

// -----------------------------------------------------------------------------
// filter struct + capturedLocalResponse — per-stream state shape per §4.3.
// Task 8 lands the SHAPE only; DecodeHeaders/EncodeHeaders/OnDestroy bodies
// land at Task 12.
// -----------------------------------------------------------------------------

// filter is the per-stream filter instance per 25.1 SPEC §4.3. NOT goroutine-
// safe (per-stream-goroutine isolation per ADR-0071's single-goroutine-per-
// stream invariant).
//
// Both-sides filter: implements StreamDecoderFilter (Task 12 DecodeHeaders
// dispatches `proxy_on_request_headers`) AND StreamEncoderFilter (Task 12
// EncodeHeaders dispatches `proxy_on_response_headers`). Static interface
// assertions at the var-block below mirror phase-22.1 lua's Task-8 shape.
//
// Per-stream lifecycle (Task 12):
//   - DecodeHeaders: NewVM → RegisterABICallbacks → Run (module-init) →
//     CallProxyOnContextCreate → executions++ → CallProxyOnRequestHeaders →
//     dispatch on captured local-response / ProxyAction.
//   - EncodeHeaders: CallProxyOnResponseHeaders → same disposition.
//   - OnDestroy: CallProxyOnDone → CallProxyOnLog → CallProxyOnDelete → Close.
type filter struct {
	// cfg is the SHARED per-listener compiledConfig (module + sandbox +
	// pluginName + stats). Closure-captured at Task 9 New's FilterInstanceFactory.
	// Type added at Task 9 (compiled_config.go); at Task 8 the field is typed
	// `*compiledConfig` and points to nil until the type lands.
	cfg *compiledConfig //nolint:unused // closure-captured at Task 9 New's FilterInstanceFactory

	// eff is the EFFECTIVE per-stream compiledConfig resolved ONCE at
	// DecodeHeaders entry (per-route override > listener default) per phase-25.3
	// Task 9 + AMEND-C1. The whole per-stream lifecycle (NewStreamContext /
	// rootCB.register / the guest dispatch / stats / failurePolicy / the reload
	// machine / EncodeHeaders / OnDestroy) binds to eff.rootVM — NOT f.cfg —
	// so a per-route override swaps the ENTIRE VM consistently across decode +
	// encode + destroy. Resolved once + reused (the framework's per-route proto
	// pointer is stable per route; resolving per-callback would risk a split
	// decode/encode VM). nil until DecodeHeaders runs; EncodeHeaders / OnDestroy
	// fall back to f.cfg on the encode-only / never-decoded edge.
	eff *compiledConfig //nolint:unused // resolved at Task 9 DecodeHeaders; consumed by EncodeHeaders + OnDestroy

	// NOTE: the 25.1 `vm *wasm.VM` field was REMOVED at 25.2 IMPL Task 18
	// alongside the deletion of internal/wasm/vm.go (Task 1 per D-P-PLAN-6).
	// The replacement is the `streamCtx *wasm.StreamContext` field below
	// (added at Task 16) which is bound to the shared per-compiledConfig
	// *RootVM. The per-stream wazero.Runtime construction path is GONE;
	// the per-stream lifecycle now goes through cfg.rootVM.NewStreamContext
	// (microseconds per stream) → streamCtx.Close (cancel-at-destruction
	// per AMEND-B3).

	// streamContextID is the per-stream context discriminator passed to the
	// proxy-wasm callbacks. Allocated at DecodeHeaders entry (Task 12) as a
	// monotonically-increasing per-VM u32 counter; the parent context is
	// cfg.rootContextID.
	streamContextID uint32 //nolint:unused // allocated at Task 12 DecodeHeaders entry

	// sentLocalResponse holds the captured `proxy_send_local_response` payload
	// until the host-side filter dispatch handoffs to `FilterCallbacks.SendLocalReply`.
	// Populated by abi_callbacks.SendLocalResponse (Task 11); consumed by
	// DecodeHeaders/EncodeHeaders (Task 12) — non-nil signals the dispatcher
	// should return StopIteration + call decoderCb/encoderCb.SendLocalReply.
	sentLocalResponse *capturedLocalResponse //nolint:unused // populated at Task 11 SendLocalResponse abi_callbacks; consumed at Task 12

	// requestHeaders + responseHeaders hold per-side header maps captured by
	// DecodeHeaders / EncodeHeaders entry (Task 12) so the per-stream
	// abi_callbacks (Task 11) can route guest header-map hostcalls (proxy_get_
	// header_map_* / proxy_add_* / proxy_replace_* / proxy_remove_* / proxy_set_
	// header_map_pairs) to the right side. Nil on the side that has not yet
	// dispatched (e.g. encode side during decode hooks). The maps ALIAS the
	// http.Header passed into DecodeHeaders/EncodeHeaders — mutations by the
	// guest propagate to the chain through the same map. Per ADR-0071 single-
	// goroutine-per-stream invariant: no cross-goroutine mutation surface.
	requestHeaders  http.Header //nolint:unused // populated at Task 12 DecodeHeaders entry; consumed by Task 11 abi_callbacks
	responseHeaders http.Header //nolint:unused // populated at Task 12 EncodeHeaders entry; consumed by Task 11 abi_callbacks

	// decoderCb + encoderCb store the framework-supplied per-stream callbacks
	// from SetDecoderCallbacks / SetEncoderCallbacks (Task 12). Consumed by the
	// per-stream abi_callbacks (Task 11): GetStatus reads encoderCb.ResponseStatus()
	// per ADR-0196 D-P3 first co-consumer; future 25.2 surfaces (DynamicMetadata
	// property tree, TLS-state property surface, etc.) read both sides. Nil
	// before SetX dispatch + on the side that does not exist for this stream
	// (encode-only / decode-only filter shapes).
	decoderCb envoyhttp.DecoderFilterCallbacks //nolint:unused // populated at Task 12 SetDecoderCallbacks; consumed by Task 11 abi_callbacks
	encoderCb envoyhttp.EncoderFilterCallbacks //nolint:unused // populated at Task 12 SetEncoderCallbacks; consumed by Task 11 abi_callbacks GetStatus (ADR-0196 D-P3 first co-consumer)

	// downstreamFilterState + upstreamFilterState hold the per-stream
	// filterstate.Bucket references per ADR-0207 + 25.2 SPEC §3.2 + AMEND-B4.
	// CONSUMED at 25.2 IMPL Task 15 by abi_callbacks.GetProperty's filterPropertyResolver
	// for the filter_state.<key> + upstream_filter_state.<key> + wasm.<key>
	// property-tree branches (consumer #2 of NEW internal/filterstate/ —
	// phase-22.2 lua is consumer #1 via Task 10 MIGRATION). Allocated at
	// per-stream construction at Task 18 decode_headers.go (one Bucket per
	// stream per root — downstream is the chain-side bucket; upstream is the
	// upstream-binding-side bucket). May be nil at Task 15 in test-double
	// paths AND in pre-Task-18 wiring — the abi_callbacks resolver is
	// nil-tolerant per ADR-0085 (nil bucket → NotFound for every
	// filter_state.<key> probe).
	downstreamFilterState *filterstate.Bucket //nolint:unused // populated at Task 18 per-stream construction; consumed by Task 15 abi_callbacks filterPropertyResolver
	upstreamFilterState   *filterstate.Bucket //nolint:unused // populated at Task 18 per-stream construction; consumed by Task 15 abi_callbacks filterPropertyResolver

	// -------------------------------------------------------------------------
	// 25.2 IMPL Task 16 EXTENSIONS (per 25.2 SPEC §4.3 + Q1 + Q2).
	// -------------------------------------------------------------------------

	// streamCtx is the per-stream context handle bound to the SHARED *RootVM
	// owned by *compiledConfig (per ADR-0205 root-VM lifecycle evolution +
	// 25.2 SPEC §4.3). Allocated at Task 18 decode_headers.go via
	// `cfg.rootVM.NewStreamContext(ctx)`; RELEASED at OnDestroy via
	// `streamCtx.Close(ctx)` (which fires proxy_on_done + proxy_on_log +
	// proxy_on_delete + cancels outstanding httpCalls per AMEND-B3).
	//
	// REPLACES the 25.1 `vm *wasm.VM` field above (Task 18 deletes the obsolete
	// vm field + the per-stream wasm.NewVM construction; the package compile-
	// break since Task 1 closes at Task 18 per D-P-PLAN-6). At Task 16 the
	// field is added IN ADDITION TO the obsolete vm field — body.go +
	// trailers.go consume `streamCtx.CallProxyOn{Request,Response}{Body,
	// Trailers}` via the NEW *StreamContext methods landed at Task 16
	// stream_context.go EXTEND. Production callers populate the field at
	// Task 18; test callers populate it directly in body_test.go +
	// trailers_test.go.
	streamCtx *wasm.StreamContext //nolint:unused // populated at Task 18 decode_headers.go; consumed by Task 16 body.go + trailers.go

	// decodeBody + encodeBody hold the per-stream accumulated body bytes per
	// 25.2 SPEC §4.3 + Q1. Grow on each DecodeData / EncodeData call BEFORE
	// the cap-enforcement check + the proxy_on_*_body dispatch. The guest
	// reads via proxy_get_buffer_bytes(HttpRequestBody | HttpResponseBody,
	// start, max_size) which dispatches through abi_callbacks.GetBuffer
	// (Task 15 stub ACTIVATED at Task 16 to read from these fields).
	//
	// Per Q1: body_size passed to proxy_on_*_body is the ACCUMULATED total
	// (uint32(len(decodeBody)) / uint32(len(encodeBody))), NOT the just-new-
	// chunk delta. The proxy-wasm v0.2.1 contract is per-chunk-invoke +
	// guest reads the accumulated buffer; envoy-go follows this verbatim.
	decodeBody []byte //nolint:unused // populated at Task 16 body.go DecodeData; consumed at Task 16 abi_callbacks.GetBuffer
	encodeBody []byte //nolint:unused // populated at Task 16 body.go EncodeData; consumed at Task 16 abi_callbacks.GetBuffer

	// decodeBodyCapExceeded + encodeBodyCapExceeded are sticky flags set to
	// true on the FIRST cap-exceeded event per Q2 + 25.2 SPEC §4.3. Once set,
	// subsequent DecodeData / EncodeData calls on the same stream do NOT
	// re-invoke SendLocalReply (decoderCb.SendLocalReply is idempotent at the
	// chain level but re-invoking would re-bump the body_buffer_cap_exceeded
	// counter on every chunk past the cap — the sticky flag ensures the
	// counter increments exactly once per stream). On the encode side
	// SendLocalReply is unavailable (EncoderFilterCallbacks does not expose
	// it); the sticky flag short-circuits to StopAllIteration so the chain
	// terminates the response early.
	decodeBodyCapExceeded bool //nolint:unused // populated at Task 16 body.go DecodeData on first cap-exceeded
	encodeBodyCapExceeded bool //nolint:unused // populated at Task 16 body.go EncodeData on first cap-exceeded
}

// capturedLocalResponse holds the proxy_send_local_response payload until the
// host-side filter dispatch handoffs to FilterCallbacks.SendLocalReply.
// Populated by abi_callbacks.SendLocalResponse (Task 11); consumed by
// DecodeHeaders/EncodeHeaders (Task 12).
//
// The 8-argument `proxy_send_local_response(status_code, status_msg_ptr,
// status_msg_size, body_ptr, body_size, additional_headers_ptr,
// additional_headers_size, grpc_status)` per parent SPEC §4.2 + 25.1 SPEC §5.1
// row 3 marshals into this struct at Task 11; the dispatcher at Task 12
// consumes REUSE 5 (`SendLocalReply` per parent §3.3) to emit the local reply.
type capturedLocalResponse struct {
	// statusCode is the HTTP status code from the guest (e.g. 403, 503).
	statusCode uint32 //nolint:unused // populated at Task 11 SendLocalResponse abi_callbacks
	// statusMsg is the HTTP status reason-phrase from the guest
	// (typically empty; the framework re-derives from statusCode).
	statusMsg string //nolint:unused // populated at Task 11
	// body is the response body bytes from the guest (may be empty).
	body string //nolint:unused // populated at Task 11
	// additionalHeaders is the guest-supplied additional header set, decoded
	// via internal/wasm pairs.DecodePairs at Task 11.
	additionalHeaders []wasm.HeaderPair //nolint:unused // populated at Task 11
	// grpcStatus is the gRPC status code (-1 if not gRPC; per proxy-wasm
	// v0.2.1 spec the guest passes a signed i32 here).
	grpcStatus int32 //nolint:unused // populated at Task 11
}

// Static interface conformance assertions per 25.1 SPEC §4.3 both-sides
// shape (mirrors phase-22.1 lua's both-sides shape). The *filter implements
// BOTH StreamDecoderFilter AND StreamEncoderFilter — the SAME struct
// participates on the decode side (proxy_on_request_headers dispatch per
// Task 12 decode_headers.go) and the encode side (proxy_on_response_headers
// dispatch per Task 12 encode_headers.go).
//
// Task 12 IMPL has landed DecodeHeaders/DecodeData/DecodeTrailers/
// SetDecoderCallbacks + EncodeHeaders/EncodeData/EncodeTrailers/
// SetEncoderCallbacks + OnDestroy; the static interface conformance
// assertions are now live (replaces the Task 8 placeholder blank-var
// references).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// -----------------------------------------------------------------------------
// New — HTTPFilterFactory skeleton per ADR-0072 + ADR-0071.
// -----------------------------------------------------------------------------

// New is the HTTPFilterFactory registered at boot per ADR-0072 / cmd/envoy-go/
// main.go Task 13. Full body landed at Task 12 (cross-references Task 9
// buildCompiledConfig + Task 10 datasource resolution + Task 11
// abiCallbacks).
//
// Step sequence:
//
//  1. ADR-0072 boot-time-fail-fast: tc != nil check. Arm-1 PARSE-REJECT
//     wording matches the parent §6.2 arm-1 byte-stable text. Without
//     this guard the buildCompiledConfig call below would also reject
//     with the same wording — duplicated here as a defensive fast-path
//     so the typed_config-less listing surfaces at the factory entry.
//  2. buildCompiledConfig(tc) (Task 9 + 10): parses 18-arm PARSE-REJECT
//     roster, resolves the 4-arm AsyncDataSource.Local, compiles via
//     internal/wasm/CompileModule (gates ABI version per AMEND-A6), and
//     constructs the per-config SandboxConfig from
//     PluginConfig.capability_restriction_config. The Task 9 body
//     ALREADY threads ctx.Stats through newFilterStats internally.
//  3. Returns the per-stream FilterInstanceFactory closure constructing
//     a fresh *filter bound to the SHARED *compiledConfig. The returned
//     HTTPFilter has Decoder=f AND Encoder=f per 25.1 SPEC §4.3 both-sides
//     shape — the same *filter struct participates on BOTH sides.
//
// Per-route validator registration is NOT performed inside New: registration
// lands at boot via the exported RegisterPerRouteValidator function (called
// from cmd/envoy-go/main.go BEFORE httpReg.Freeze() at Task 13). New is
// called by the listener constructor AFTER Freeze; any
// RegisterPerRouteValidator call from inside New would panic. Mirrors
// phase-22.1 lua + phase-20 oauth2 + phase-10 header_mutation precedent.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	// Step 1: ADR-0072 boot-time-fail-fast. The same arm-1 wording is
	// re-asserted by buildCompiledConfig; duplicated here to fast-path
	// the typed_config-less listing at the factory entry.
	if tc == nil {
		return nil, errors.New(parseRejectTypedConfigRequired)
	}

	// Step 2: parse + validate + compile per Tasks 9 + 10.
	// buildCompiledConfig threads ctx.Stats through newFilterStats per
	// the Task 9 body — no separate stat-allocation call at this site.
	cfg, err := buildCompiledConfig(context.Background(), tc, ctx)
	if err != nil {
		return nil, err
	}

	// Step 3: per-stream FilterInstanceFactory closure. The *filter is
	// fresh per stream (per-stream-goroutine isolation per ADR-0071);
	// cfg is closure-captured + SHARED across streams (immutable post-
	// buildCompiledConfig).
	return func() envoyhttp.HTTPFilter {
		f := &filter{cfg: cfg}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f, // proxy_on_request_headers fires at DecodeHeaders
			Encoder: f, // proxy_on_response_headers fires at EncodeHeaders
		}
	}, nil
}

// -----------------------------------------------------------------------------
// Per-route validator (Task 8 stub; 25.3 IMPL replaces).
// -----------------------------------------------------------------------------

// RegisterPerRouteValidator registers the wasm per-route validator with the
// supplied HTTPRegistry. MUST be called BEFORE httpReg.Freeze() in
// cmd/envoy-go/main.go — the registry rejects registrations after Freeze, and
// New is called during listener construction (after Freeze). Mirrors the
// header_mutation + oauth2 + lua precedent.
//
// At phase-25.3 (arm 18 LIFTED) the validator validates the per-route
// wholesale Wasm TPFC override shape via validateWasmConfigShape (AMEND-C1 +
// ADR-0210 5th-canonical wholesale-override; ADR-0125 STAYS at 10 canonicals;
// NO §(xvi) amendment per AMEND-A3). A valid per-route config ACCEPTS; an
// invalid one rejects with the SAME byte-stable buildCompiledConfig wording.
//
// The exported function takes the same interface shape as phase-22.1 lua's
// RegisterPerRouteValidator (a structural-typed interface accepting any
// registry that exposes RegisterPerRouteValidator — decouples this package
// from a hard import of the framework's *HTTPRegistry type at the
// registration call site).
//
// Registration ordering: wire RegisterPerRouteValidator before
// httpReg.Register(wasm.TypeURL, wasm.New) — the registry rejects
// registrations after Freeze, and New runs post-Freeze.
func RegisterPerRouteValidator(reg interface {
	RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)
}) {
	reg.RegisterPerRouteValidator(filterName, validatePerRouteWasm)
}

// validatePerRouteWasm is the ADR-0110 single-chokepoint per-route validator
// for the envoy.filters.http.wasm filter. At phase-25.3 (arm 18 LIFTED) it
// validates the per-route wholesale Wasm TPFC override shape per AMEND-C1 +
// ADR-0210 (the per-route override is a full *wasmv3.Wasm message — there is
// NO WasmPerRoute type; the entire compiledConfig, hence its *RootVM, swaps
// per-route).
//
// The validator is invoked by BuildPerRouteConfig at HCM-build time for each
// parsed proto.Message at each tier (Route, VirtualHost, RouteConfiguration,
// listener-typed_per_filter_config). It type-asserts to *wasmv3.Wasm, round-
// trips into an *anypb.Any, and runs validateWasmConfigShape — a validate-only
// pass of buildCompiledConfig that fires every PARSE-REJECT arm WITHOUT
// claiming the process-wide plugin name (arm 26) NOR acquiring a registry
// *RootVM refcount (no VM instantiation / goroutine / leak). A valid per-route
// config returns nil; an invalid one returns the SAME byte-stable wording as
// the listener build path (single source of truth = buildCompiledConfig).
//
// Per-route validation design decision (recorded in PROGRESS.md): validate-
// only mode, NOT build-then-discard. A build-then-discard would (a) leak the
// registry *RootVM refcount + goroutine and (b) permanently claim the plugin
// name via arm-26's append-only registry — so a legitimate second use of the
// same override on another route would FALSE-reject. validate-only avoids both
// while still fail-fast-rejecting every malformed per-route config at HCM-build.
//
// The real per-route compiledConfig + *RootVM build happens at Task 9 dispatch
// resolution (parsePerRouteWasm + resolvePerRoute), not here.
func validatePerRouteWasm(m proto.Message) error {
	w, ok := m.(*wasmv3.Wasm)
	if !ok {
		return fmt.Errorf("wasm: per-route: expected *wasmv3.Wasm, got %T", m)
	}
	tc, err := anypb.New(w)
	if err != nil {
		return err
	}
	return validateWasmConfigShape(tc)
}
