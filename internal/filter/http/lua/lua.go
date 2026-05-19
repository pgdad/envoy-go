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
	"errors"
	"log"
	"net/http"

	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
	"github.com/esalaine/envoy-go/internal/stats"
)

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

	// Step 4: per-stream FilterInstanceFactory closure. The *filter is
	// fresh per stream (per-stream-goroutine isolation per ADR-0071);
	// cc is closure-captured + shared across streams (immutable post-
	// New).
	return func() envoyhttp.HTTPFilter {
		f := &filter{cc: cc}
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

	// dcb / ecb are the framework-supplied per-stream callbacks. Stored
	// for SendLocalReply (decode-side respond capture path) + future
	// async-resume needs.
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks
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

// DecodeData is pass-through. Body-access bridge methods are OUT OF
// SCOPE at 22.1 per 22.1 SPEC §2.1; 22.2 activates.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is pass-through. Trailer-access bridge methods are
// OUT OF SCOPE at 22.1 per 22.1 SPEC §2.2; 22.2 activates.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeData is pass-through.
func (f *filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is pass-through.
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy releases the per-stream *internal/lua.VM via vm.Close per
// 22.1 SPEC §3.4 per-stream lifecycle. Idempotent — guard against
// double-close + against nil vm (the latter occurs when DecodeHeaders
// short-circuited at the nil-chunk pass-through without ever
// constructing a VM).
func (f *filter) OnDestroy() {
	if f.vm != nil {
		f.vm.Close()
		f.vm = nil
	}
}

// logf is the package-level logger used by decode_headers.go +
// encode_headers.go for errors-channel diagnostics. Tagged via stdlib
// log.Printf with the canonical "lua:" prefix (mirrors the bridge-
// methods' "<LEVEL> lua:" pattern from the :logXxx methods). Indirected
// via a package var to make test-side capture trivial (override _ = log
// in the rare case a test wants to suppress noise; default goes to
// log.Default()).
var logf = log.Printf
