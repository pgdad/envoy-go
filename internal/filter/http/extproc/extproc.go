package extproc

// extproc.go — main filter file for envoy.filters.http.ext_proc per phase 19.1.
//
// Public surface (per SPEC §6.1):
//   - TypeURL: canonical filter typed_config type URL.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error):
//     the HTTPFilterFactory registered at boot per ADR-0072 + ADR-0167.
//
// See doc.go for the full package overview including the dual-mode envelope,
// the 19.1-consumed field set, filter shape, per-route canonical, stat
// surface, async-resume discipline, and ADR anchors.
//
// THIS FILE LANDS THE SKELETON AT TASK 2 — the type shapes, the factory stub
// (returns "ext_proc: factory under construction; lands at Task 11"), the
// pass-through filter methods, the three compile-time interface assertions,
// and the newFilterStats 9-counter registration helper. The real
// buildCompiledConfig body lands at Task 11; the real DecodeHeaders /
// EncodeHeaders bodies land at Tasks 7-11 (via processor.go / check.go /
// attributes.go split per SPEC §6.9).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/grpcclient"
	"github.com/esalaine/envoy-go/internal/stats"
)

// TypeURL is the canonical Envoy type-URL for the ext_proc HTTP filter config.
// Boot wiring at cmd/envoy-go/main.go registers New under this key in the
// HTTPRegistry per ADR-0072 + ADR-0167.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"

// filterName is the canonical http_filters[].name string for ext_proc (matches
// the listener config typed_per_filter_config map keys; underscore preserved
// per the filter's proto canonical name).
const filterName = "envoy.filters.http.ext_proc"

// ---------------------------------------------------------------------------
// Core types per SPEC §6.1 + §6.2 + ADR-0167 + ADR-0168.
// ---------------------------------------------------------------------------

// processorClient is the mode-agnostic outbound-processor interface. The two
// concrete implementations (one per transport arm) are produced at
// config-load time inside buildCompiledConfig (Task 11):
//
//   - gRPC mode: *grpcclient.ProcessorClient (lands at Task 4 per ADR-0169).
//     Wraps a bidi-streaming Process RPC; one *ClientConn per (cluster,
//     compiledConfig) pair; per-stream Process(ctx) opens a fresh bidi-stream
//     for each HTTP transaction.
//
//   - HTTP mode: *httpProcessorClient (filter-local; declared below). Wraps
//     an *http.Client with the parsed base URL + path_prefix; each stage
//     does a one-shot POST with a protojson-encoded ProcessingRequest.
//
// The interface itself is intentionally narrow at Task 2 (lands as a marker
// type). The per-stage dispatcher at processor.go (Task 7) + check.go (Task 8)
// settles the exact method set once both arms are wired.
//
// Both arms are mutually exclusive per ADR-0168 §Decision — exactly one of
// compiledConfig.grpcClient / compiledConfig.httpClient is non-nil.
type processorClient interface {
	// Close releases any underlying transport resource (gRPC ClientConn /
	// HTTP idle connections). Idempotent + concurrent-safe (mirrors
	// grpcclient.AuthClient.Close discipline).
	Close() error
}

// httpProcessorClient is the filter-local http_service-mode transport per
// SPEC §6.5 buildHTTPProcessorClient (real body lands at Task 8 in check.go).
// Wraps an *http.Client configured with the proto's HttpService.http_uri.timeout
// + the parsed base URL from http_uri.uri. Per-stage POST construction at
// the dispatcher (Task 11 wires the call site).
//
// **Field-shape rationale**: the proto's ExtProcHttpService wraps a
// corev3.HttpService which exposes http_uri (NOT server_uri as the SPEC §6.5
// sketch suggested at planner-time — the SPEC's "server_uri" wording was a
// drafting carry-over from the auth-filter terminology). The 19.1 IMPL
// captures only the base URL from http_uri.uri; the per-stage POST URL is the
// bare base URL with the per-stage path-suffix derived from the stage
// discriminator at dispatch time. No path_prefix field exists on the
// ExtProcHttpService proto at v1.32.4.
type httpProcessorClient struct {
	client  *http.Client
	baseURL string
}

// Compile-time interface-satisfaction assertion — anchors the
// `processorClient` marker interface against its HTTP-mode implementation.
// Per Carryforward P (Task 14), this replaces the now-retired
// TestSkeletonReachability anchor; the gRPC-mode arm uses the concrete
// `*grpcclient.ProcessorClient` type directly (per Task 11 retirement of the
// Task-2 `grpcProcessorClient` placeholder), so the interface is
// documentation-only at 19.1 — the assertion keeps it referenced so the
// unused linter does not flag it.
var _ processorClient = (*httpProcessorClient)(nil)

// Close implements processorClient. Closes idle connections via
// Transport.CloseIdleConnections (best-effort; mirrors phase-18.1 extauthz
// httpAuthClient.Close shape — though that one was a noop placeholder; at
// 19.1 the explicit CloseIdleConnections call is preserved for the
// hot-reload phase's transport-recycle discipline).
//
// Idempotent: subsequent calls are safe (nil-tolerant client; net/http's
// CloseIdleConnections is itself idempotent per the package doc).
func (c *httpProcessorClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	if t, ok := c.client.Transport.(interface{ CloseIdleConnections() }); ok {
		t.CloseIdleConnections()
	}
	return nil
}

// resolvedProcessingMode is the parsed-and-validated form of
// *extprocv3.ProcessingMode (per SPEC §6.2 + parent §5.P9). DEFAULT
// translates to SEND for header-modes; trailer-modes are restricted to SKIP
// (PARSE-REJECT permanently); body-modes are restricted to NONE in 19.1
// (PARSE-REJECT lifts at 19.2 per ADR-0168 + ADR-0171 §Decision AMENDMENTS).
//
// The real validation + translation logic lives in resolveProcessingMode
// (lands at Task 11 per SPEC §6.5 buildCompiledConfig step 3).
type resolvedProcessingMode struct {
	// Header-modes — consumed in 19.1. SEND fires the stage; SKIP bypasses.
	// DEFAULT is translated to SEND at parse time per parent §5.P9.
	RequestHeaderMode  extprocv3.ProcessingMode_HeaderSendMode
	ResponseHeaderMode extprocv3.ProcessingMode_HeaderSendMode

	// Body-modes — must equal NONE in 19.1 (PARSE-REJECT otherwise). The
	// fields are retained on the struct for 19.2 forward-compat (the struct
	// shape stays UNCHANGED at 19.2; body-mode validation lifts).
	RequestBodyMode  extprocv3.ProcessingMode_BodySendMode
	ResponseBodyMode extprocv3.ProcessingMode_BodySendMode

	// Trailer-modes — must equal SKIP permanently (PARSE-REJECT otherwise).
	// Retained for forward-compat against any future scope expansion.
	RequestTrailerMode  extprocv3.ProcessingMode_HeaderSendMode
	ResponseTrailerMode extprocv3.ProcessingMode_HeaderSendMode
}

// resolvedMutationRules is the pre-compiled form of
// *v31.HeaderMutationRules per parent §5.P3. Holds the four protected-set
// boolean flags ready to apply at per-stage CommonResponse.header_mutation
// time (Task 8); the regex-matcher fields (allow_expression /
// disallow_expression) are reserved for an IMPL-time AMENDMENT path at 19.2.
//
// **Field-promotion at Task 8 per ADR-0172 §Decision (header-mode portion)**:
// the Task 2 placeholder shape (an empty struct) is PROMOTED to carry the
// four boolean wrappers + the protected-default-set semantics. The
// promotion is race-safe because compiledConfig (which holds the
// *resolvedMutationRules pointer) is immutable post-buildCompiledConfig
// per ADR-0101 — the rules struct is read-only after that point + safe to
// share across all per-stream filter goroutines + per-stage dispatch
// invocations.
type resolvedMutationRules struct {
	// AllowAllRouting: when true, the routing-influencing headers (host /
	// :authority / :scheme / :method) may be mutated. Default false →
	// protected per parent §5.P3.
	AllowAllRouting bool
	// AllowEnvoy: when true, x-envoy-* headers may be mutated. Default false
	// → protected per parent §5.P3.
	AllowEnvoy bool
	// DisallowSystem: when true, ANY :-prefixed pseudo-header is protected
	// regardless of AllowAllRouting. Default false.
	DisallowSystem bool
	// DisallowAll: when true, NO mutations are allowed (overrides all other
	// flags). Default false.
	DisallowAll bool
}

// resolvedForwardRules is the pre-compiled form of *HeaderForwardingRules
// per the proto's allowed_headers / disallowed_headers matcher lists.
// Filters which request-side headers are forwarded into the
// ProcessingRequest's `headers` field on each stage (Task 8 / Task 9).
//
// Real fields land at Task 11 (compilation) and are consumed at Task 9
// (buildAttributeEnvelope + the header-population call sites).
type resolvedForwardRules struct {
	// Placeholder — populated at Task 11.
}

// resolvedPerRoute is the parsed *ExtProcPerRoute per-route override per
// ADR-0173. Two arms per the PGV-required `override` oneof:
//
//   - disabled (bool, PGV `const: true`): wholly deactivates the filter on
//     this route. envoy-go PARSE-REJECTs disabled:false (PGV const:true
//     enforcement).
//
//   - overrides (*ExtProcOverrides): a narrower per-route override carrying
//     processing_mode (#1; consumed) + grpc_service (#5; consumed). The
//     other 5 fields (#2 async_mode + #3 request_attributes + #4
//     response_attributes + #6 metadata_options + #7 grpc_initial_metadata)
//     are SILENT-IGNORED per the proto's [#not-implemented-hide:] convention.
//
// **SHARED-stats per ADR-0173 §Decision**: the per-route override adjusts
// processing_mode/grpc_service but spawns no new stateful policy-evaluation
// surface — there is NO per-route *filterStats allocation. The 9 counters are
// shared with the listener-level via compiledConfig.stats (mirrors
// phase-12/13/14/17/18 SHARED-stats discipline).
//
// **grpcService captures the raw proto pointer**: the per-route grpc_service
// validation + per-route *grpcclient.ProcessorClient construction live at
// Task 11 buildCompiledConfig integration (the buildGRPCProcessorClient
// helper from check.go is reused with the per-route GrpcService + the
// listener-level message_timeout — the per-route arm does NOT carry its own
// timeout fields). At Task 10 the raw pointer is captured + the construction
// is deferred — same staging as the listener-level grpc_service which captures
// at parse time + constructs at integration time.
//
// **effectiveProcessingMode**: nil when no per-route processing_mode override
// is present (the listener-level config's processing_mode is the effective
// mode at DecodeHeaders time per parent §5.P1). Non-nil when the per-route
// overrides arm carries a processing_mode field — the resolved struct holds
// the full validated set (header-modes + body-mode-NONE + trailer-mode-SKIP
// per 19.1's PARSE-REJECT discipline). The DecodeHeaders integration at
// Task 11 picks per-route OR listener-level on a per-stream basis.
type resolvedPerRoute struct {
	// disabled is true when the per-route override arm is disabled:true.
	// Per parent §5.P6 + ADR-0173 §Decision: disabled:true wholly deactivates
	// the filter for this route (DecodeHeaders returns Continue immediately;
	// no processor call; no counter increments).
	disabled bool

	// effectiveProcessingMode is the per-route ProcessingMode override (#1)
	// validated through resolveProcessingMode (same 19.1 PARSE-REJECT
	// discipline as the listener-level processing_mode: body-mode != NONE
	// PARSE-REJECT; trailer-mode != SKIP PARSE-REJECT; DEFAULT translates to
	// SEND for header-modes). nil when the overrides arm does NOT carry a
	// processing_mode field — the listener-level processing_mode is the
	// effective mode in that case.
	effectiveProcessingMode *resolvedProcessingMode

	// grpcService is the per-route grpc_service override (#5) raw proto
	// pointer. nil when the overrides arm does NOT carry a grpc_service field.
	// Captured at parseExtProcPerRoute time + consumed at
	// (*factoryState).resolvePerRouteConfig time to construct processorClient
	// via the listener-level buildGRPCProcessorClient (Task 8) helper.
	grpcService *corev3.GrpcService

	// processorClient is the per-route *grpcclient.ProcessorClient constructed
	// from grpcService via the listener-level buildGRPCProcessorClient helper
	// (check.go) — Carryforward G of Task 11 rework per SPEC §5 line 216
	// MVP-CONSUMED classification + PLAN Task 10 line 602 acceptance gate
	// ("grpc_service consumed"). nil when (a) the overrides arm has no
	// grpc_service field, (b) the listener is HTTP-mode (cross-mode
	// PARSE-REJECT fires earlier; the per-route falls back to listener-level),
	// or (c) the per-route TPFC was the disabled:true arm.
	//
	// **Cache key**: (*factoryState).perRouteProcessorClients sync.Map keyed
	// by the raw *corev3.GrpcService pointer per ADR-0117 pointer-identity
	// (mirrors the s.perRoute sync.Map keyed by *ExtProcPerRoute). Two
	// per-route entries that share the same *GrpcService pointer share the
	// same *ProcessorClient (no duplicate dial).
	//
	// At DecodeHeaders entry, (*filter).resolvePerRoute selects
	// processorClient when non-nil; else falls back to f.cc.grpcClient
	// (listener-level). The selection is captured on f.activeProcessorClient
	// for the entire filter lifetime per parent §5.P7 cache-on-first-use.
	processorClient *grpcclient.ProcessorClient
}

// filterStats is the 9-counter set per parent SPEC §6 amendment 4 + ADR-0167.
// ALL 9 counters allocated unconditionally at New() time (predeclared empty
// counters for scrape stability — operators get a consistent counter
// surface; mirrors phase-17 jwt_authn + phase-18.x ext_authz unconditional-
// allocation discipline).
//
// Namespace shape per SN2-reuse: `http.<HCM_stat_prefix>.ext_proc.<counter>`.
// Closure of the exact roster + stat namespace is RATIFIED-PENDING-IMPL-TIME
// per parent §5.P4 — closes at Task 13 fixture-harness scrape per the
// phase-16 §10 lesson (c) + phase-18.2 §11.P4 precedent.
//
//   - streamsStarted: a bidi-stream was opened (gRPC mode) OR the first
//     per-stage POST was dispatched (HTTP mode). One per HTTP transaction.
//
//   - streamMsgsSent: each ProcessingRequest sent on the stream (gRPC mode)
//     OR per-stage POST issued (HTTP mode). Per stage.
//
//   - streamMsgsReceived: each ProcessingResponse received on the stream
//     (gRPC mode) OR per-stage POST response parsed (HTTP mode). Per stage.
//
//   - spuriousMsgsReceived: protocol violations classified as spurious:
//     stage-mismatch ProcessingResponses, CONTINUE_AND_REPLACE in 19.1 (per
//     ADR-0172 §Decision header-mode portion + D7 — lifts at 19.2),
//     ImmediateResponse when disable_immediate_response=true, AND
//     mutation_rules-rejected header-mutations (one increment per stage with
//     any rejection — per parent §5.P3).
//
//   - streamsFailed: transport-error / message_timeout exceeded / unmarshal
//     failure (per D8 fail-loud) / protocol violations. Pairs with
//     failureModeAllowed on the failure_mode_allow=true allow-through path.
//
//   - streamsClosed: normal closure (CloseSend after final stage, OR
//     OnDestroy cancellation, OR ImmediateResponse arrival, OR HTTP-mode
//     transaction completion).
//
//   - failureModeAllowed: error disposition + failure_mode_allow=true; BOTH
//     streamsFailed AND failureModeAllowed increment on the allow-through
//     path (mirrors phase-18.x ext_authz failure_mode_allowed discipline).
//
//   - overrideMessageTimeoutReceived: ProcessingResponse.override_message_timeout
//     arrived AND was honored (range OK + max_message_timeout>=1ms AND
//     once-per-stage). Per parent §5.P10 + ADR-0171.
//
//   - overrideMessageTimeoutIgnored: override_message_timeout out-of-range
//     OR more than once per stage OR max_message_timeout=0 (override
//     disabled). Per parent §5.P10 + ADR-0171.
type filterStats struct {
	streamsStarted                 *stats.Counter
	streamMsgsSent                 *stats.Counter
	streamMsgsReceived             *stats.Counter
	spuriousMsgsReceived           *stats.Counter
	streamsFailed                  *stats.Counter
	streamsClosed                  *stats.Counter
	failureModeAllowed             *stats.Counter
	overrideMessageTimeoutReceived *stats.Counter
	overrideMessageTimeoutIgnored  *stats.Counter
}

// compiledConfig is the immutable post-parse listener-level config per SPEC
// §6.2 + ADR-0168. Mode-agnostic across grpc_service / http_service arms;
// FIELD-FINAL at 19.1 — 19.2 amends only to lift body-mode PARSE-REJECT (the
// struct gains NO new fields; body-mode-specific config lives in the closure
// captures inside processFn). Allocated once at New() time; read-only shared
// after that per ADR-0101.
//
// Mutual-exclusion invariant per ADR-0168 §Decision: exactly one of
// grpcClient / httpClient is non-nil. Enforced at buildCompiledConfig time
// (Task 11) via the parent §5.P1 both-set / neither-set PARSE-REJECT.
type compiledConfig struct {
	// Transport — mutually exclusive: exactly one of these is non-nil.
	// At Task 2 the gRPC arm was a placeholder type (grpcProcessorClient);
	// at Task 7 the field type is PROMOTED to *grpcclient.ProcessorClient
	// (the real wrapper from internal/grpcclient/processor_client.go landed
	// at Task 4 per ADR-0169). Task 11 buildCompiledConfig constructs it.
	// The httpProcessorClient real fields land at Task 8.
	grpcClient             *grpcclient.ProcessorClient // when grpc_service is set (Task 11 constructs)
	httpClient             *httpProcessorClient        // when http_service is set (Task 8 + Task 11)
	httpServiceHeadersOnly bool                        // true when http_service mode (body PARSE-REJECT per proto constraint)

	// Processing mode + mode-override per parent §5.P1 + §5.P9.
	processingMode       *resolvedProcessingMode   // header-modes only in 19.1; body-modes activate at 19.2
	allowModeOverride    bool                      // governs whether mode_override responses are honored
	allowedOverrideModes []*resolvedProcessingMode // empty list = no restriction

	// Error posture per parent §5.P11 + ADR-0168 §Decision.
	failureModeAllow         bool          // default false; allow through on error
	messageTimeout           time.Duration // default 200ms (PGV gte=0s lte=1h)
	maxMessageTimeout        time.Duration // 0 = override disabled (PGV gte=0s lte=1h)
	disableImmediateResponse bool          // when true, ImmediateResponse → spurious_msgs_received++ + drop

	// Mutation + forward discipline per parent §5.P3.
	mutationRules *resolvedMutationRules // pre-compiled allow/deny matchers for header-mutation safety
	forwardRules  *resolvedForwardRules  // pre-compiled allowed/disallowed header matchers for forwarding

	// Attribute envelopes per parent §5.P1 + SPEC §6.6.
	requestAttributes  []string // CEL attribute names for request_headers stage
	responseAttributes []string // CEL attribute names for response_headers stage

	// Route-cache discipline per parent §5.P5.
	// disable_clear_route_cache:true translates to RETAIN; otherwise the
	// raw enum value (DEFAULT / RETAIN / CLEAR) flows through.
	routeCacheAction extprocv3.ExternalProcessor_RouteCacheAction

	// Stat surface — 9 counters per parent §5.P4 hypothesis. SHARED with
	// per-route per ADR-0173 (per-route override adjusts processing_mode /
	// grpc_service but spawns no new stateful policy-evaluation surface).
	// nil when ctx.Stats is nil (test path per ADR-0085 nil-tolerance) —
	// the hot-path counter increments are guarded `if cc.stats != nil` at
	// each call site (Task 8 / Task 9 / Task 10 wiring).
	stats *filterStats

	// Stat-prefix segment if set (ExternalProcessor.stat_prefix #8).
	// Combined with the HCM stat_prefix to form the per-listener namespace.
	statPrefix string
}

// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-18.1 ext_authz factoryState shape (SHARED-stats per ADR-0173
// — no per-route *filterStats allocation).
//
// The perRoute sync.Map lazy-caches parsed *resolvedPerRoute keyed by
// *extprocv3.ExtProcPerRoute pointer-identity per ADR-0117 + ADR-0125 §(v).
// Race-safe via sync.Map.LoadOrStore.
//
// Real wiring lands at Task 10 (parsePerRoute + resolvePerRouteConfig per
// ADR-0173).
type factoryState struct {
	listenerRC *compiledConfig
	perRoute   sync.Map // map[*extprocv3.ExtProcPerRoute]*resolvedPerRoute — populated at Task 10

	// factoryCtx is the listener-level FactoryCtx captured at New() time —
	// retained on factoryState so (*factoryState).resolvePerRouteConfig can
	// invoke buildGRPCProcessorClient(perRouteGS, factoryCtx,
	// listenerRC.messageTimeout) to construct per-route ProcessorClients
	// lazily on first per-route resolution (Task 11 rework Carryforward G).
	// Mirrors phase-18.1 ext_authz factoryState's ctx-retention discipline.
	factoryCtx envoyhttp.FactoryCtx

	// perRouteProcessorClients caches per-route *grpcclient.ProcessorClient
	// keyed by raw *corev3.GrpcService pointer-identity per ADR-0117. Mirrors
	// the s.perRoute sync.Map pattern (different key — *GrpcService rather
	// than *ExtProcPerRoute — so two per-routes sharing the same *GrpcService
	// pointer share the same *ProcessorClient and the underlying
	// *grpc.ClientConn is dialed ONCE per (per-route *GrpcService pointer,
	// listener *compiledConfig) pair). Populated lazily at resolvePerRouteConfig
	// cache-miss; survives until factoryState GC at listener-exit. The MVP
	// leaks-on-exit posture per ADR-0158 §Decision (vi) applies — envoy-go has
	// no xDS-CDS hot-reload yet; per-route ProcessorClients are reclaimed at
	// process exit.
	perRouteProcessorClients sync.Map // map[*corev3.GrpcService]*grpcclient.ProcessorClient
}

// filter is the per-stream filter instance allocated by the factory closure.
// BOTH-DECODE-AND-ENCODE per ADR-0167 §Decision — Decoder: f AND Encoder: f
// (SAME *filter instance servicing both sides; mirrors phase-14 compressor's
// both-sides shape per ADR-0129 §Decision (iv) but with both sides
// structurally active in 19.1).
//
// The field set captures the per-stream state per SPEC §6.1 + §6.2 + §6.3 +
// §6.4 sketches. The actual reads/writes are wired across:
//   - Task 7 (processor.go): activeProcessingMode + stream/streamCtx +
//     dispatchStage + completeStage + the resume-channel discipline.
//   - Task 8 (check.go): applyProcessingResponse + applyHeaderMutation +
//     emitImmediateResponse + the per-stage SendLocalReply emission.
//   - Task 9 (attributes.go): buildAttributeEnvelope reads from the
//     ADR-0165 decoder-side callbacks + ADR-0174 encoder-side callbacks.
//   - Task 10 (per-route resolution): activePerRoute caching.
//   - Task 11 (buildCompiledConfig integration): cc wiring + factory init.
//   - Task 12 (race tests): mu / done OnDestroy-race-guard exercised under
//     -race.
type filter struct {
	state *factoryState
	dcb   envoyhttp.DecoderFilterCallbacks
	ecb   envoyhttp.EncoderFilterCallbacks

	// Per-stream effective config (resolved at DecodeHeaders entry; cached
	// for encode-side reuse). cc is always state.listenerRC at 19.1
	// (SHARED-stats per ADR-0173 — no per-route *compiledConfig allocation).
	cc             *compiledConfig
	activePerRoute *resolvedPerRoute // nil when no per-route TPFC applies

	// activeProcessorClient is the per-stream gRPC ProcessorClient — selected
	// at resolvePerRoute time (Task 11 rework Carryforward G): the per-route
	// override's processorClient when non-nil, else the listener-level
	// cc.grpcClient. The selection is captured once at DecodeHeaders entry +
	// stays in effect for the entire filter lifetime per parent §5.P7
	// cache-on-first-use discipline (one stream per HTTP transaction; the
	// per-route choice fixes the client choice for that stream's lifetime).
	//
	// openProcessorStream reads activeProcessorClient (NOT cc.grpcClient
	// directly) so per-route gRPC routing is honored end-to-end. nil in
	// HTTP-mode listeners (the http_service arm uses per-stage POSTs, not
	// the bidi Process stream).
	activeProcessorClient *grpcclient.ProcessorClient

	// activeProcessingMode is the EFFECTIVE per-direction ProcessingMode
	// for this stream — initialized from cc.processingMode (or the per-route
	// override at DecodeHeaders entry) and mutated mid-stream when a
	// ProcessingResponse carries a validated mode_override (parent §5.P1 +
	// ADR-0171 §Decision). Read on both decode and encode side; the
	// framework's sequential decode→encode dispatch provides happens-before
	// ordering per D9 — no atomic load/store needed.
	activeProcessingMode *resolvedProcessingMode

	// Per-stream gRPC bidi-stream state per parent §5.P10 + ADR-0171.
	// streamCtx is the per-stream context.Context (derived from parentCtx);
	// streamCancel propagates OnDestroy / final-stage cancellation to any
	// in-flight Send/Recv. closeOnce guards the CloseSend + streamCancel
	// pair for OnDestroy idempotency per D9. stream is the bidi-stream
	// wrapper opened lazily at first dispatchStage in gRPC mode (NOT used
	// in http_service mode — that arm uses per-stage POSTs).
	//
	// Populated at Task 7 (openProcessorStream); consumed at Task 8
	// (check.dispatchStage / applyProcessingResponse) + the OnDestroy cancel
	// path.
	parentCtx    context.Context
	streamCtx    context.Context
	streamCancel context.CancelFunc
	stream       grpcclient.ProcessStream
	closeOnce    sync.Once

	// Per-stage override-timeout bookkeeping per parent §5.P10 + ADR-0171.
	// The override_message_timeout ProcessingResponse field is at most ONCE
	// per stage — overrideApplied tracks which stages have already consumed
	// an override (so a second override at the same stage classifies as
	// spurious_msgs_received++). activeMsgTimeout is the effective per-
	// message timeout currently in force (starts at cc.messageTimeout; reset
	// by handleOverrideMessageTimeout to the override value per D6 cancel-
	// and-rebuild discipline).
	overrideApplied  [numStages]bool
	activeMsgTimeout time.Duration

	// streamStartTime is the wall-clock instant of DecodeHeaders entry —
	// the canonical anchor for ProcessingRequest.attributes.request.time
	// per SPEC §6.6 hypothesis-table. Captured at DecodeHeaders entry per
	// the phase-18.2 ADR-0144 capture-site convention.
	streamStartTime time.Time

	// requestContentType is the downstream request's content-type at
	// DecodeHeaders entry — anchored for the parent §5.P2
	// gRPC-downstream-detection sniff (`application/grpc` →
	// ImmediateResponse body routes into grpc-message header + grpc_status
	// → grpc-status trailer). Cached at DecodeHeaders so the encode-side
	// emitImmediateResponse can read it without re-inspecting headers.
	requestContentType string

	// mu + done guard the resume-after-OnDestroy race per D9 + planner-time
	// decision pattern (mirrors phase-18.x ext_authz mu/done pattern). The
	// decode-side and encode-side dispatch goroutines (one at a time per
	// the framework's sequential decode→encode dispatch invariant) check
	// `done` under `mu` before touching dcb/ecb; OnDestroy sets `done=true`
	// under `mu` + invokes streamCancel + the bidi-stream CloseSend. Full
	// wiring at Task 7 + Task 12 race-tests.
	mu   sync.Mutex
	done bool

	// httpStreamStarted is the lazy first-POST flag for HTTP-service mode
	// per ADR-0167 §Decision — HTTP-mode has no explicit stream-open point
	// (each stage is a fresh POST), so streamsStarted is incremented on the
	// first dispatchHTTPStage invocation rather than at openProcessorStream
	// time. Reset is per-filter-lifetime (one transaction). Wired at
	// Task 13 alongside the dispatchHTTPStage rollout.
	httpStreamStarted bool

	// immediateResponseEmitted is set when emitImmediateResponse fires (at
	// EITHER decode or encode stage). After this flag is set, subsequent
	// stage entry points (EncodeHeaders most notably) short-circuit to
	// Continue without re-dispatching to the processor — the framework's
	// SendLocalReply has already terminated the upstream chain; the
	// encode-chain entry at filter[len-1] re-enters this filter's
	// EncodeHeaders but the processor must NOT be called again per parent
	// §5.P2 (one-shot deny). Wired at Task 13 alongside the differential
	// fixture's scenario 2 (immediate_response at request_headers) ROOT
	// CAUSE FIX.
	immediateResponseEmitted bool

	// decodeHeaders / encodeHeaders stash the in-flight request / response
	// http.Header references at DecodeHeaders / EncodeHeaders entry. The
	// async dispatch goroutine's applyHeaderMutation reads these references
	// to apply the per-stage CommonResponse.header_mutation.set_headers /
	// remove_headers entries against the live upstream / downstream header
	// map. WITHOUT the stash the dispatch goroutine has no path back to the
	// in-flight headers (DecodeHeaders / EncodeHeaders return before the
	// dispatch goroutine completes). Mirrors the phase-18.x ext_authz
	// `cachedHeaders` stash pattern (extauthz.go:1232) — one stash per
	// direction per stream-lifetime; read at apply-time under the same
	// per-stream sequencing invariant the HCM dispatch provides.
	//
	// Task 13 rework: fixture-0022 scenario 1+3+8 byte-equivalence
	// regression root-cause. The Task 8 applyHeaderMutation body left the
	// `headers.Set/Add/Del` call site as a "deferred to Task 11" TODO; the
	// Task 11 wire-up landed only the parse-time integration, leaving the
	// runtime apply as an UNFINISHED gap. The fixture-harness scrape at
	// Task 13 surfaced this as missing-upstream-injection for scenario 1
	// (request-side) + missing-downstream-injection for scenarios 3 + 8
	// (response-side).
	decodeHeaders http.Header
	encodeHeaders http.Header
}

// Compile-time assertions per SPEC §6.10 — the filter satisfies BOTH sides
// of the iteration protocol + the HTTPFilter tagged-union value. Per ADR-0167
// §Decision — ext_proc is the FIRST §9 row to ship BOTH decoder + encoder
// participation in a single filter package (phase-14 compressor is encode-
// only; all prior §9 rows decode-only).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// ---------------------------------------------------------------------------
// SetDecoderCallbacks / SetEncoderCallbacks — framework-supplied per-stream
// callback wiring per ADR-0071 + ADR-0167.
// ---------------------------------------------------------------------------

// SetDecoderCallbacks stores the per-stream decoder callbacks for later
// RequestRouteConfig + SendLocalReply + ContinueDecoding + the ADR-0165
// 6 accessor reads (DownstreamRemoteAddr / DownstreamLocalAddr /
// DownstreamTLSServerName / DownstreamTLSPeerCertDER / DownstreamProtocol /
// ListenerPrincipal) consumed at Task 9 attributes.go.
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks stores the per-stream encoder callbacks for later
// SendLocalReply (response_headers-stage ImmediateResponse emission per
// ADR-0167) + ContinueEncoding + the NEW ADR-0174 6 encode-side accessor
// reads (lands at Task 5; consumed at Task 9 attributes.go for
// response_attributes envelope population).
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// ---------------------------------------------------------------------------
// New — HTTPFilterFactory per SPEC §6.1 + ADR-0072 + ADR-0167.
// ---------------------------------------------------------------------------

// New is the HTTPFilterFactory exposed at boot per ADR-0072 + ADR-0167.
// Lands FULLY-WIRED at Task 11 (the Task 2 skeleton stub is replaced).
//
// Per ADR-0167 + ADR-0168:
//
//  1. tc must be non-nil (PARSE-REJECT).
//  2. Unmarshal tc → *extprocv3.ExternalProcessor; PARSE-REJECT on malformed Any.
//  3. Invoke buildCompiledConfig (parses + validates the proto per ADR-0168
//     §Decision — grpc_service/http_service mutual-exclusion, processing_mode
//     restrictions, STREAMED-only flag PARSE-REJECT, error-posture fields,
//     mutation_rules/forward_rules compilation, route_cache_action mutual-
//     exclusion, filterStats allocation).
//  4. Capture *factoryState{listenerRC} for per-route lazy-cache (Task 10).
//  5. Return FilterInstanceFactory closure that allocates a fresh *filter per
//     stream. HTTPFilter value: Decoder: f, Encoder: f (BOTH per ADR-0167).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("ext_proc: typed_config required")
	}
	var raw extprocv3.ExternalProcessor
	if err := tc.UnmarshalTo(&raw); err != nil {
		return nil, fmt.Errorf("ext_proc: unmarshal: %w", err)
	}
	cc, err := buildCompiledConfig(&raw, ctx)
	if err != nil {
		return nil, err
	}
	// Carryforward G of Task 11 rework: retain FactoryCtx on factoryState so
	// resolvePerRouteConfig can invoke buildGRPCProcessorClient for per-route
	// grpc_service overrides lazily (the per-route ClusterManager lookup +
	// dial happens at first per-route resolution time, NOT at New time).
	state := &factoryState{listenerRC: cc, factoryCtx: ctx}
	return func() envoyhttp.HTTPFilter {
		f := &filter{
			state:                state,
			cc:                   cc,
			parentCtx:            context.Background(),
			activeProcessingMode: cc.processingMode,
			activeMsgTimeout:     cc.messageTimeout,
		}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// buildCompiledConfig parses + validates an extprocv3.ExternalProcessor
// envelope per ADR-0168 §Decision. LANDS FULLY-WIRED at Task 11 (the Task 2
// skeleton stub is replaced).
//
// Parse-order per SPEC §6.5 + ADR-0168 §Decision (10-step pipeline):
//
//  1. Mutual-exclusion check on grpc_service (#1) vs http_service (#20):
//     both-set OR neither-set → PARSE-REJECT (parent §5.P1).
//  2. STREAMED-only flag PARSE-REJECT (parent §5.P10) — fires early so
//     operators with a STREAMED-only config get the rejection deterministically
//     before the transport-build dispatch.
//  3. Error-posture fields with defaults (#2 + #7 + #10 + #15).
//  4. route_cache_action (#18) mutual-exclusion with disable_clear_route_cache
//     (#11) per parent §5.P5; disable_clear_route_cache=true → RETAIN.
//  5. Build the transport client:
//     - gRPC arm → buildGRPCProcessorClient (Task 8 helper) — needs
//     ctx.ClusterManager; GoogleGrpc PARSE-REJECT per ADR-0157 inherited.
//     - HTTP arm → buildHTTPProcessorClient (Task 8 helper); sets
//     cc.httpServiceHeadersOnly so step 6 PARSE-REJECTs body-mode != NONE.
//  6. Validate + resolve processing_mode (#3) per parent §5.P9 via Task 7's
//     resolveProcessingMode (body-mode != NONE PARSE-REJECT; trailer-mode !=
//     SKIP PARSE-REJECT; http_service + body-mode != NONE PARSE-REJECT;
//     DEFAULT → SEND for header-modes).
//  7. Resolve mutation_rules (#9) via Task 8's resolveMutationRules + capture
//     forward_rules (#12) raw pointer (compile is a 19.1 no-op — the
//     resolvedForwardRules struct is a placeholder).
//  8. allow_mode_override (#14) + allowed_override_modes (#22): resolve the
//     per-mode allowlist (each entry validated through resolveProcessingMode).
//  9. Attribute envelopes: request_attributes (#5) + response_attributes (#6)
//     captured as []string allowlists.
//  10. stat_prefix (#8) + filterStats allocation via newFilterStats (guarded
//     `if ctx.Stats != nil` per ADR-0085 nil-tolerance).
func buildCompiledConfig(raw *extprocv3.ExternalProcessor, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
	if raw == nil {
		return nil, errors.New("ext_proc: typed_config (ExternalProcessor) is required")
	}
	cc := &compiledConfig{}

	// Step 1: grpc_service vs http_service mutual-exclusion (parent §5.P1).
	gs := raw.GetGrpcService()
	hs := raw.GetHttpService()
	switch {
	case gs == nil && hs == nil:
		return nil, errors.New("ext_proc: exactly one of grpc_service or http_service must be set (neither set)")
	case gs != nil && hs != nil:
		return nil, errors.New("ext_proc: exactly one of grpc_service or http_service must be set (both set)")
	}

	// Step 2: STREAMED-only flag PARSE-REJECT (parent §5.P10).
	if raw.GetObservabilityMode() {
		return nil, errors.New("ext_proc: observability_mode not supported (STREAMED-only flag, out of envelope per parent §5.P10)")
	}
	if raw.GetSendBodyWithoutWaitingForHeaderResponse() {
		return nil, errors.New("ext_proc: send_body_without_waiting_for_header_response not supported (STREAMED-only flag, out of envelope per parent §5.P10)")
	}
	if dt := raw.GetDeferredCloseTimeout(); dt != nil && dt.AsDuration() > 0 {
		return nil, errors.New("ext_proc: deferred_close_timeout not supported (observability_mode-coupled STREAMED-only flag, out of envelope per parent §5.P10)")
	}

	// Step 3: error-posture defaults per parent §5.P11 + ADR-0168 §Decision (viii).
	cc.failureModeAllow = raw.GetFailureModeAllow()
	cc.messageTimeout = durationOrDefault(raw.GetMessageTimeout(), 200*time.Millisecond)
	cc.maxMessageTimeout = durationOrDefault(raw.GetMaxMessageTimeout(), 0)
	cc.disableImmediateResponse = raw.GetDisableImmediateResponse()

	// Step 4: route_cache_action vs disable_clear_route_cache mutual-exclusion
	// (parent §5.P5 + ADR-0168 §Decision (xi)).
	rca := raw.GetRouteCacheAction()
	dcrc := raw.GetDisableClearRouteCache()
	if rca != extprocv3.ExternalProcessor_DEFAULT && dcrc {
		return nil, errors.New("ext_proc: only one of route_cache_action or disable_clear_route_cache can be set")
	}
	switch {
	case dcrc:
		cc.routeCacheAction = extprocv3.ExternalProcessor_RETAIN
	default:
		cc.routeCacheAction = rca
	}

	// Step 5: build the transport client (per-arm dispatch).
	if gs != nil {
		pc, err := buildGRPCProcessorClient(gs, ctx, cc.messageTimeout)
		if err != nil {
			return nil, err
		}
		cc.grpcClient = pc
	} else {
		hc, err := buildHTTPProcessorClient(hs)
		if err != nil {
			return nil, err
		}
		cc.httpClient = hc
		cc.httpServiceHeadersOnly = true
	}

	// Step 6: processing_mode validation per parent §5.P9 + ADR-0171 (header-
	// mode portion). Body-mode != NONE PARSE-REJECT in 19.1; trailer-mode !=
	// SKIP PARSE-REJECT permanently; DEFAULT translates to SEND for header-modes.
	pm, err := resolveProcessingMode(raw.GetProcessingMode(), cc.httpServiceHeadersOnly)
	if err != nil {
		return nil, fmt.Errorf("ext_proc: processing_mode: %w", err)
	}
	cc.processingMode = pm

	// Step 7: mutation_rules (Task 8 helper) + forward_rules (placeholder at
	// 19.1; the resolvedForwardRules struct is a forward-compat anchor —
	// allowed_headers / disallowed_headers compilation deferred to a future
	// regex-matcher promotion path per parent §5.P3).
	cc.mutationRules = resolveMutationRules(raw.GetMutationRules())
	cc.forwardRules = &resolvedForwardRules{}

	// Step 8: allow_mode_override + allowed_override_modes (per parent §5.P1
	// + ADR-0171). Each entry validated through resolveProcessingMode (same
	// 19.1 PARSE-REJECT discipline as the listener-level processing_mode).
	cc.allowModeOverride = raw.GetAllowModeOverride()
	for i, am := range raw.GetAllowedOverrideModes() {
		resolved, err := resolveProcessingMode(am, cc.httpServiceHeadersOnly)
		if err != nil {
			return nil, fmt.Errorf("ext_proc: allowed_override_modes[%d]: %w", i, err)
		}
		cc.allowedOverrideModes = append(cc.allowedOverrideModes, resolved)
	}

	// Step 9: attribute envelopes (per parent §5.P1 + SPEC §6.6). The
	// allowlists flow verbatim into the compiledConfig; the buildAttribute-
	// Envelope dispatch consumes them at DecodeHeaders / EncodeHeaders entry.
	cc.requestAttributes = raw.GetRequestAttributes()
	cc.responseAttributes = raw.GetResponseAttributes()

	// Step 10: stat_prefix + filterStats allocation per ADR-0167. Guarded
	// `if ctx.Stats != nil` per ADR-0085 nil-tolerance (test paths may not
	// supply a registry).
	cc.statPrefix = raw.GetStatPrefix()
	if ctx.Stats != nil {
		cc.stats = newFilterStats(ctx.Stats, ctx.StatPrefix)
	}

	return cc, nil
}

// durationOrDefault returns d.AsDuration() when d is non-nil, else fallback.
// Per ADR-0168 §Decision (viii) — the proto's PGV constraint guarantees
// `gte=0s, lte=1h0m0s` for both message_timeout + max_message_timeout fields,
// so the helper does no range validation (the PGV envelope is honored at the
// proto unmarshal layer).
func durationOrDefault(d *durationpb.Duration, fallback time.Duration) time.Duration {
	if d == nil {
		return fallback
	}
	return d.AsDuration()
}

// ---------------------------------------------------------------------------
// Filter method skeletons — pass-through Continue / noop until Tasks 7-11
// land the real bodies per SPEC §6.3 + §6.4 + §6.7 + §6.8.
// ---------------------------------------------------------------------------

// DecodeHeaders is the request_headers-stage entry point per SPEC §6.3.
// LANDS FULLY-WIRED at Task 11 (replaces the Task 2 pass-through stub).
//
//  1. Resolve per-route → cache on f.activePerRoute (Task 10).
//  2. If activePerRoute.disabled → return Continue immediately (no processor
//     call, no counter increments — per parent §5.P6).
//  3. Compute the EFFECTIVE processing_mode (per-route override wins over
//     listener-level) → cache on f.activeProcessingMode.
//  4. If request_header_mode == SKIP → return Continue.
//  5. Capture per-stream entry state (streamStartTime + requestContentType
//     per parent §5.P2 for the gRPC-downstream-detection sniff at the encode-
//     side ImmediateResponse emitter).
//  6. Build the request_headers ProcessingRequest via Task 9
//     buildRequestHeadersProcessingRequest.
//  7. Open the bidi-stream (gRPC mode) OR no-op (HTTP mode — per-stage POST
//     at dispatchStage).
//  8. Fire async send + recv goroutine; park the decode goroutine on the
//     resume channel via StopIteration — Task 7 dispatchStage.
//
// Note on HTTP-mode dispatch at 19.1: the dispatchStage path assumes a gRPC
// bidi-stream (f.stream non-nil). The HTTP-mode per-stage POST dispatcher
// lands at Task 13 fixture integration when the http_service test scenario
// activates. At 19.1 unit-test time the HTTP-mode arm is exercised only
// through the buildHTTPProcessorClient construction path; the per-stage
// POST dispatcher is structurally reserved.
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: per-route resolution (cache-on-first-use per ADR-0173).
	pr := f.resolvePerRoute()

	// Step 2: disabled-per-route short-circuit per parent §5.P6.
	if pr.disabled {
		return envoyhttp.Continue
	}

	// Step 3: effective processing_mode (per-route override wins over listener-level).
	if pr.effectiveProcessingMode != nil {
		f.activeProcessingMode = pr.effectiveProcessingMode
	}
	mode := f.activeProcessingMode
	if mode == nil {
		// Defensive: no processing_mode configured → fall back to listener cc.
		mode = f.cc.processingMode
		f.activeProcessingMode = mode
	}

	// Step 4: SKIP discipline.
	if mode == nil || mode.RequestHeaderMode == extprocv3.ProcessingMode_SKIP {
		return envoyhttp.Continue
	}

	// Step 5: per-stream capture for the encode-side ImmediateResponse +
	// the SPEC §6.6 attribute envelope's `request.time` anchor (per the
	// phase-18.2 ADR-0144 capture-site convention).
	f.streamStartTime = time.Now()
	f.requestContentType = headers.Get("content-type")
	// Stash the in-flight request header map so the async dispatch
	// goroutine's applyHeaderMutation can apply set_headers / remove_headers
	// entries against the live upstream header set per ADR-0172 §Decision +
	// parent §5.P3 (Task 13 rework: fixture-0022 scenario-1 byte-
	// equivalence root-cause fix).
	f.decodeHeaders = headers

	// Step 6: build the request_headers ProcessingRequest envelope.
	req := buildRequestHeadersProcessingRequest(f, headers, endStream, f.cc.requestAttributes)

	// Step 7: open the per-stream bidi-stream (gRPC mode) — no-op for HTTP mode.
	if f.cc.grpcClient != nil {
		if err := f.openProcessorStream(); err != nil {
			// Transport-open failure: classify per failure_mode_allow. The
			// dispatch goroutine never starts; the resume signal fires
			// synchronously via mapTransportError's classifier.
			act := mapTransportError(f, stageRequestHeaders, err)
			if act == actContinue {
				return envoyhttp.Continue
			}
			// actImmediate: SendLocalReply already fired; treat as Continue
			// so the chain unwinds (the local-reply enters at filter[len-1]
			// per ADR-0075).
			return envoyhttp.Continue
		}
	}

	// Step 8: async dispatch + park on StopIteration (the dispatch goroutine
	// signals ContinueDecoding via completeStage's signalResume on completion).
	f.dispatchStage(stageRequestHeaders, req)
	return envoyhttp.StopIteration
}

// DecodeData is a pass-through in 19.1: body-mode arms PARSE-REJECT at
// parse-time per ADR-0168 §Decision (body-mode != NONE → PARSE-REJECT),
// so any well-formed 19.1 config never reaches a state where DecodeData
// has substantive work. The DataContinue return is kept for
// code-completeness (the encoder/decoder interfaces require the method to
// exist). 19.2 replaces this body with the BUFFERED-arm body-stage logic
// per ADR-0168 + ADR-0171 + ADR-0172 §Decision AMENDMENTS.
func (f *filter) DecodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	_ = data
	_ = endStream
	return envoyhttp.DataContinue
}

// DecodeTrailers is a pass-through per the trailer-mode != SKIP PARSE-REJECT
// at parse-time per ADR-0168 §Decision (permanent — trailers stay out of
// envelope). The TrailersContinue return is kept for interface-completeness.
func (f *filter) DecodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	_ = trailers
	return envoyhttp.TrailersContinue
}

// EncodeHeaders is the response_headers-stage entry point per SPEC §6.4.
// LANDS FULLY-WIRED at Task 11 (replaces the Task 2 pass-through stub).
//
//  1. If f.activePerRoute.disabled → return Continue (cached from DecodeHeaders).
//  2. If f.activeProcessingMode.ResponseHeaderMode == SKIP → return Continue
//     (the mode may have been mutated mid-stream by a validated mode_override
//     in the request_headers ProcessingResponse).
//  3. Build the response_headers ProcessingRequest via Task 9
//     buildResponseHeadersProcessingRequest.
//  4. Fire async send + recv on the EXISTING bidi-stream (gRPC mode — same
//     Process stream that decode-side opened) OR a fresh HTTP POST (HTTP-mode
//     — per-stage POST dispatcher reserved for Task 13 fixture integration).
//  5. Park the encode goroutine on the resume channel via StopIteration —
//     phase-09 async-resume primitive reuse on the ENCODE side (a 19.1 FIRST).
//
// ImmediateResponse at the response_headers stage triggers
// f.ecb.SendLocalReply(...) — the framework's SendLocalReply enters the
// encode chain at filter[len-1] per ADR-0075, which is consistent with
// emitting the immediate-response from the encode side per ADR-0167.
func (f *filter) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 0: immediate-response one-shot guard per parent §5.P2 (Task 13
	// scenario-2 root-cause fix). When the request_headers stage emitted an
	// ImmediateResponse, the framework's SendLocalReply re-enters this
	// filter's encode chain at filter[len-1] per ADR-0075; the processor
	// MUST NOT be called again. Short-circuit before any per-route /
	// processing_mode evaluation.
	if f.immediateResponseEmitted {
		return envoyhttp.Continue
	}

	// Step 1: per-route disabled short-circuit (uses cached value from
	// DecodeHeaders entry per ADR-0173 cache-on-first-use). If the encode
	// side fires WITHOUT DecodeHeaders having run first (synthetic streams
	// / test paths), resolvePerRoute resolves to a fresh non-disabled
	// zero-value per its nil-tolerance contract.
	pr := f.resolvePerRoute()
	if pr.disabled {
		return envoyhttp.Continue
	}

	// Step 2: SKIP discipline + mid-stream mode_override honor. The
	// activeProcessingMode may have been mutated by a validated mode_override
	// arriving on the request_headers ProcessingResponse (per parent §5.P1 +
	// ADR-0171; see applyProcessingResponse step 3).
	mode := f.activeProcessingMode
	if mode == nil {
		mode = f.cc.processingMode
	}
	if mode == nil || mode.ResponseHeaderMode == extprocv3.ProcessingMode_SKIP {
		return envoyhttp.Continue
	}

	// Stash the in-flight response header map so the async dispatch
	// goroutine's applyHeaderMutation can apply set_headers / remove_headers
	// entries against the live downstream response header set per ADR-0172
	// §Decision + parent §5.P3 (Task 13 rework: fixture-0022 scenarios 3+8
	// byte-equivalence root-cause fix).
	f.encodeHeaders = headers

	// Step 3: build the response_headers ProcessingRequest envelope.
	req := buildResponseHeadersProcessingRequest(f, headers, endStream, f.cc.responseAttributes)

	// Step 4: dispatch on the EXISTING bidi-stream (opened at DecodeHeaders).
	// When the decode side was SKIPPED (decode-side request_header_mode==SKIP
	// + encode-side response_header_mode==SEND), the stream has NOT been opened
	// yet — open it here.
	if f.cc.grpcClient != nil && f.stream == nil {
		if err := f.openProcessorStream(); err != nil {
			act := mapTransportError(f, stageResponseHeaders, err)
			if act == actContinue {
				return envoyhttp.Continue
			}
			return envoyhttp.Continue
		}
	}

	// Step 5: async dispatch + park on StopIteration. The dispatch goroutine
	// signals ContinueEncoding via completeStage's signalResume on completion.
	f.dispatchStage(stageResponseHeaders, req)
	return envoyhttp.StopIteration
}

// EncodeData is a pass-through in 19.1 per the body-mode != NONE PARSE-REJECT
// at parse-time (mirrors DecodeData rationale). 19.2 activates the
// response_body BUFFERED arm via ADR-0175 (encode-side body-buffering
// primitive) — at that point the body bytes are buffered before the
// response_body ProcessingRequest fires.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	_ = data
	_ = endStream
	return envoyhttp.DataContinue
}

// EncodeTrailers is a pass-through per the trailer-mode != SKIP PARSE-REJECT
// (mirrors DecodeTrailers rationale).
func (f *filter) EncodeTrailers(trailers http.Header) envoyhttp.FilterTrailersStatus {
	_ = trailers
	return envoyhttp.TrailersContinue
}

// OnDestroy is the filter-lifetime cancel hook per ADR-0071 + ADR-0167.
// Idempotently cancels the per-stream context (f.streamCancel via
// f.closeOnce.Do — covers OnDestroy + final-stage-cleanup races) + invokes
// the bidi-stream CloseSend; the race-guard (f.mu + f.done) ensures any
// in-flight dispatch goroutine observes the done flag before touching
// dcb/ecb on resume.
//
// LANDS AT TASK 7 (header-mode portion + bidi-stream lifecycle) via the
// onDestroyImpl body in processor.go. Task 12 race-tests exercise the
// concurrent-OnDestroy + dispatch-completion race surface end-to-end.
func (f *filter) OnDestroy() {
	f.onDestroyImpl()
}

// ---------------------------------------------------------------------------
// newFilterStats — 9-counter registration per SPEC §6.2 + ADR-0167.
// ---------------------------------------------------------------------------

// newFilterStats registers the 9 base counters per ADR-0167 + parent SPEC §6
// amendment 4. All 9 registered unconditionally (predeclared empty counters
// for scrape stability per Prometheus best practice; mirrors phase-17
// jwt_authn + phase-18.x ext_authz unconditional-allocation discipline).
//
// Internal stat path per SN2-reuse: `http.<HCM_stat_prefix>.ext_proc.<counter>`.
// Empty HCM stat_prefix (test code paths) folds to `ext_proc.<counter>` to
// satisfy the Registry's nameRE which forbids leading dots / double dots.
// RATIFIED-PENDING-IMPL-TIME closure at Task 13 fixture-harness scrape per
// the phase-16 §10 lesson (c).
//
// Uses NewCounterIfAbsent (rather than NewCounter) to avoid the multi-listener-
// same-prefix panic footgun per the phase-16 rbac Task 8 lesson (CF-Task2-I3)
// — idempotent registration is safe and consistent with the existing
// discipline.
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences reg. The nil-tolerance contract lives at buildCompiledConfig's
// `if ctx.Stats != nil` gate per ADR-0085 (Task 11 wiring).
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := baseStatPrefix(hcmStatPrefix)
	return &filterStats{
		streamsStarted:                 reg.NewCounterIfAbsent(prefix + "streams_started"),
		streamMsgsSent:                 reg.NewCounterIfAbsent(prefix + "stream_msgs_sent"),
		streamMsgsReceived:             reg.NewCounterIfAbsent(prefix + "stream_msgs_received"),
		spuriousMsgsReceived:           reg.NewCounterIfAbsent(prefix + "spurious_msgs_received"),
		streamsFailed:                  reg.NewCounterIfAbsent(prefix + "streams_failed"),
		streamsClosed:                  reg.NewCounterIfAbsent(prefix + "streams_closed"),
		failureModeAllowed:             reg.NewCounterIfAbsent(prefix + "failure_mode_allowed"),
		overrideMessageTimeoutReceived: reg.NewCounterIfAbsent(prefix + "override_message_timeout_received"),
		overrideMessageTimeoutIgnored:  reg.NewCounterIfAbsent(prefix + "override_message_timeout_ignored"),
	}
}

// baseStatPrefix returns the canonical base-prefix segment for the 9 base
// counters per ADR-0167 + parent §5.P4 SN2-reuse hypothesis. Shape:
//
//	http.<HCM_stat_prefix>.ext_proc.
//
// Empty HCM stat_prefix (test code paths) folds to the bare `ext_proc.` form
// to satisfy the Registry's nameRE (forbids leading dots / double dots).
// RATIFIED-PENDING-IMPL-TIME closure at Task 13 via the reference Envoy
// v1.37.2 empirical scrape.
func baseStatPrefix(hcmStatPrefix string) string {
	if hcmStatPrefix == "" {
		return "ext_proc."
	}
	return "http." + hcmStatPrefix + ".ext_proc."
}

// ---------------------------------------------------------------------------
// Per-route 5th-canonical helpers per ADR-0173 + parent SPEC §5.P6 + §5.P7.
//
// parseExtProcPerRoute parses the *ExtProcPerRoute proto per the PGV-required
// `override` oneof discipline. (*factoryState).resolvePerRouteConfig is the
// sync.Map lazy-cache + listener-level fallback path. (*filter).resolvePerRoute
// is the per-stream cache-on-first-use entry point — the per-route resolved at
// the first call (DecodeHeaders time at Task 11) stays in effect for the
// entire filter's lifetime even across hypothetical ClearRouteCache
// invocations per parent §5.P7.
// ---------------------------------------------------------------------------

// parseExtProcPerRoute parses one *ExtProcPerRoute TPFC entry per parent SPEC
// §5.P6 + ADR-0173 §Decision. Envoy-go-side defensive PGV-mirror:
//
//   - override oneof PGV-required → PARSE-REJECT when nil (empty
//     ExtProcPerRoute → parse error).
//   - disabled arm: PGV const:true → PARSE-REJECT when disabled=false.
//     Returns &resolvedPerRoute{disabled: true} on the valid disabled:true
//     case.
//   - overrides arm: validate the narrower override sub-message:
//   - processing_mode (#1, *ProcessingMode): MVP-CONSUMED. Validated
//     through resolveProcessingMode (same 19.1 PARSE-REJECT discipline as
//     listener-level: body-mode != NONE → PARSE-REJECT; trailer-mode != SKIP
//     → PARSE-REJECT; DEFAULT → SEND for headers). Captures into
//     .effectiveProcessingMode.
//   - grpc_service (#5, *GrpcService): MVP-CONSUMED. Raw pointer captured
//     into .grpcService for downstream consumption by
//     (*factoryState).resolvePerRouteConfig — that helper invokes
//     buildGRPCProcessorClient with the listener-level FactoryCtx +
//     messageTimeout to construct the per-route *grpcclient.ProcessorClient
//     (cached on .processorClient per Task 11 rework Carryforward G). The
//     ClusterManager lookup + UseH2 gate + dial happen at first per-route
//     resolution time, NOT at parse time — this helper stays
//     mode-agnostic + side-effect-free (no I/O).
//   - async_mode (#2), request_attributes (#3), response_attributes (#4),
//     metadata_options (#6), grpc_initial_metadata (#7): SILENT-IGNORED
//     per the proto's [#not-implemented-hide:] convention + the DEFERRED
//     classifications per ADR-0173 §Decision. The fields are not read +
//     not surfaced on *resolvedPerRoute. Note #3/#4 are distinct from the
//     TOP-LEVEL ExternalProcessor #5/#6 attribute envelopes which ARE
//     MVP-consumed.
//
// `httpServiceMode` (true when the listener's transport is http_service) gates
// two cross-mode PARSE-REJECTs per Task 11 rework Carryforward H:
//
//   - per-route grpc_service override under HTTP-mode listener → PARSE-REJECT
//     (proto-mode-mismatch; the ExtProcPerRoute.overrides arm is the ONLY
//     per-route override field — there is no per-route http_service override
//     per the proto — so a per-route grpc_service in an http_service listener
//     is structurally meaningless; the operator is asking for a per-route
//     transport switch which envoy-go does not support).
//   - per-route processing_mode body-mode != NONE under HTTP-mode listener →
//     PARSE-REJECT via the resolveProcessingMode helper's existing
//     httpServiceMode-gated check (the proto's ExtProcHttpService constraint:
//     body-mode != NONE is incompatible with http_service).
//
// The initial Task 11 commit hard-coded `httpServiceMode=false` at this call
// site (silently fallthrough-to-listener for cross-mode overrides) — the
// rework lifts the hard-code so the caller — (*factoryState).resolvePerRouteConfig —
// passes the listener's httpServiceHeadersOnly flag verbatim.
//
// Returns *resolvedPerRoute on success; nil + non-nil error on PARSE-REJECT.
// The caller handles the parse failure by falling back to the listener-level
// config + emitting a log line (mirrors phase-18.1 extauthz parsePerRoute →
// resolvePerRouteConfig contract).
func parseExtProcPerRoute(raw *extprocv3.ExtProcPerRoute, httpServiceMode bool) (*resolvedPerRoute, error) {
	if raw == nil {
		return nil, errors.New("ext_proc: per-route: ExtProcPerRoute is nil")
	}

	// override oneof PGV-required.
	if raw.GetOverride() == nil {
		return nil, errors.New("ext_proc: per-route: override oneof is required (set either disabled:true or overrides)")
	}

	switch arm := raw.GetOverride().(type) {
	case *extprocv3.ExtProcPerRoute_Disabled:
		// PGV const:true — disabled must be true. disabled:false is not
		// meaningful (the absence of a per-route entry is the way to express
		// "use listener-level"; disabled:false would be a no-op override).
		if !arm.Disabled {
			return nil, errors.New("ext_proc: per-route: disabled must be true (PGV const:true violation; disabled:false is not meaningful)")
		}
		return &resolvedPerRoute{disabled: true}, nil

	case *extprocv3.ExtProcPerRoute_Overrides:
		ov := arm.Overrides
		if ov == nil {
			return nil, errors.New("ext_proc: per-route: overrides is required when arm is set")
		}
		rp := &resolvedPerRoute{}

		// Cross-mode PARSE-REJECT (Carryforward H): per-route grpc_service in
		// HTTP-mode listener is structurally meaningless — the ExtProcPerRoute
		// proto has no per-route http_service override field, so a per-route
		// grpc_service in an http_service listener is asking for a per-route
		// transport switch which envoy-go does not support. The opposite
		// direction (per-route http_service in a gRPC-mode listener) is NOT
		// POSSIBLE per the proto.
		if httpServiceMode && ov.GetGrpcService() != nil {
			return nil, errors.New("ext_proc: per-route: overrides.grpc_service incompatible with http_service listener (cross-mode per-route override not supported per proto)")
		}

		// #1 processing_mode — validate through resolveProcessingMode (Task 7
		// helper). Pass the listener-level httpServiceMode through so the
		// http_service body-mode != NONE check fires for per-route overrides
		// as well (per Task 11 rework Carryforward H — the initial Task 11
		// commit hard-coded false here, masking the cross-mode check). At
		// 19.1 the body-mode != NONE check fires uniformly across both
		// transport arms; the httpServiceMode-specific error wording is
		// preserved for the 19.2 body-mode lift where the gRPC arm allows
		// BUFFERED but the http_service arm permanently rejects.
		if pm := ov.GetProcessingMode(); pm != nil {
			resolved, err := resolveProcessingMode(pm, httpServiceMode)
			if err != nil {
				return nil, fmt.Errorf("ext_proc: per-route: overrides.processing_mode: %w", err)
			}
			rp.effectiveProcessingMode = resolved
		}

		// #5 grpc_service — capture the raw pointer. The per-route
		// *grpcclient.ProcessorClient construction happens at
		// (*factoryState).resolvePerRouteConfig time via the listener-level
		// buildGRPCProcessorClient helper (check.go) with the per-route
		// service + the listener-level FactoryCtx + message_timeout.
		rp.grpcService = ov.GetGrpcService()

		// #2 async_mode, #3 request_attributes, #4 response_attributes,
		// #6 metadata_options, #7 grpc_initial_metadata — SILENT-IGNORED per
		// ADR-0173 §Decision. The proto's [#not-implemented-hide:] convention
		// means upstream reference Envoy does not implement these per-route
		// arms either; envoy-go matches the upstream behavior verbatim. No
		// per-field read here — the values are intentionally dropped on the
		// floor.
		_ = ov.GetAsyncMode()
		_ = ov.GetRequestAttributes()
		_ = ov.GetResponseAttributes()
		_ = ov.GetMetadataOptions()
		_ = ov.GetGrpcInitialMetadata()

		return rp, nil

	default:
		return nil, fmt.Errorf("ext_proc: per-route: unknown override arm type %T", arm)
	}
}

// resolvePerRouteConfig returns the effective *resolvedPerRoute for the given
// TPFC message (which may be an *extprocv3.ExtProcPerRoute returned by
// dcb.RequestRouteConfig()). Per ADR-0117 + ADR-0125 §(v): keyed by proto
// pointer-identity via sync.Map lazy-cache. Mirrors phase-18.1 extauthz
// resolvePerRouteConfig discipline:
//
//   - msg == nil: inherit listener-level config (no per-route TPFC). Returns
//     a fresh non-disabled *resolvedPerRoute zero value.
//   - type assertion failure: inherit listener-level config (defensive
//     fallback for non-*ExtProcPerRoute proto.Message inputs).
//   - cache hit: return cached *resolvedPerRoute.
//   - cache miss: parseExtProcPerRoute → LoadOrStore. On parse error: log +
//     return listener-level fallback. DO NOT cache the error sentinel
//     (mirrors phase-18.1 ext_authz + phase-16 rbac pattern — parse-error
//     entries are never persisted; the next request gets a fresh attempt
//     which is fine because the proto is the same parse-error producer +
//     fails again identically).
//
// **SHARED-stats per ADR-0173 §Decision**: there is NO per-route
// *compiledConfig allocation; the listener-level cc.stats is the sole stat
// surface. This contrasts with phase-11/15/16 INDEPENDENT-stats which allocate
// a fresh per-route *filterStats. The 9-counter discipline + the SHARED
// posture together yield a stable Prometheus scrape regardless of per-route
// resolution activity.
func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *resolvedPerRoute {
	if msg == nil {
		return &resolvedPerRoute{}
	}

	// Type-assert to *ExtProcPerRoute; fall back on mismatch.
	raw, ok := msg.(*extprocv3.ExtProcPerRoute)
	if !ok {
		return &resolvedPerRoute{}
	}

	// sync.Map lookup by pointer identity. The framework returns the same
	// *ExtProcPerRoute instance from RequestRouteConfig() for the same route
	// per ADR-0117 (proto-pointer-identity cache key); this lets sync.Map
	// dedupe parses + memoize the resolved *resolvedPerRoute.
	if cached, found := s.perRoute.Load(raw); found {
		return cached.(*resolvedPerRoute)
	}

	// Cache miss: parse + LoadOrStore. The listener's httpServiceHeadersOnly
	// flag is passed through so parseExtProcPerRoute can enforce the
	// cross-mode PARSE-REJECT (Task 11 rework Carryforward H) — HTTP-mode
	// listener + per-route grpc_service override → reject; HTTP-mode listener
	// + per-route processing_mode body-mode != NONE → reject (the proto's
	// ExtProcHttpService body-only-NONE constraint extended to per-route).
	httpServiceMode := false
	if s.listenerRC != nil {
		httpServiceMode = s.listenerRC.httpServiceHeadersOnly
	}
	fresh, err := parseExtProcPerRoute(raw, httpServiceMode)
	if err != nil {
		// Per-route parse error: log + fall back to listener-level. DO NOT
		// cache (mirrors phase-18.1 ext_authz + phase-16 rbac pattern).
		log.Printf("ext_proc: per-route resolve failed (inherit-listener): %v", err)
		return &resolvedPerRoute{}
	}

	// Carryforward G (Task 11 rework): when the per-route carries a
	// grpc_service override AND the listener is in gRPC-mode, construct the
	// per-route *grpcclient.ProcessorClient via the listener-level
	// buildGRPCProcessorClient helper (check.go) — cache by raw
	// *corev3.GrpcService pointer-identity per ADR-0117 so two per-routes
	// sharing the same *GrpcService pointer share the same ProcessorClient
	// (no duplicate dial). On construction failure: log + fall back to
	// listener-level (DO NOT cache the parse error — same discipline as the
	// parseExtProcPerRoute error path above).
	if fresh.grpcService != nil && !httpServiceMode {
		fresh.processorClient = s.resolvePerRouteProcessorClient(fresh.grpcService)
		if fresh.processorClient == nil {
			// Construction failed — fall back to listener-level (the error
			// was already logged inside resolvePerRouteProcessorClient).
			return &resolvedPerRoute{}
		}
	}

	actual, _ := s.perRoute.LoadOrStore(raw, fresh)
	return actual.(*resolvedPerRoute)
}

// resolvePerRouteProcessorClient returns the cached or freshly-constructed
// per-route *grpcclient.ProcessorClient for the given raw *corev3.GrpcService.
// Carryforward G (Task 11 rework) cache discipline:
//
//   - Cache hit: return the cached *ProcessorClient (no duplicate dial; the
//     underlying *grpc.ClientConn is shared across all per-routes pinning the
//     same *GrpcService pointer).
//   - Cache miss: invoke buildGRPCProcessorClient(gs, s.factoryCtx,
//     s.listenerRC.messageTimeout) → LoadOrStore. On error: log + return nil
//     (caller falls back to listener-level per-route).
//
// The cache survives until factoryState GC at listener-exit; per-route
// ProcessorClient connections are reclaimed at process exit per the
// MVP leaks-on-exit posture (ADR-0158 §Decision (vi)).
func (s *factoryState) resolvePerRouteProcessorClient(gs *corev3.GrpcService) *grpcclient.ProcessorClient {
	if gs == nil || s.listenerRC == nil {
		return nil
	}
	if cached, found := s.perRouteProcessorClients.Load(gs); found {
		return cached.(*grpcclient.ProcessorClient)
	}
	pc, err := buildGRPCProcessorClient(gs, s.factoryCtx, s.listenerRC.messageTimeout)
	if err != nil {
		log.Printf("ext_proc: per-route grpc_service ProcessorClient construction failed (inherit-listener): %v", err)
		return nil
	}
	actual, loaded := s.perRouteProcessorClients.LoadOrStore(gs, pc)
	if loaded {
		// Lost the race — close the duplicate we just dialed; return the
		// winner.
		_ = pc.Close()
	}
	return actual.(*grpcclient.ProcessorClient)
}

// resolvePerRoute is the per-stream cache-on-first-use entry point per parent
// SPEC §5.P7 + ADR-0173 §Decision. The first invocation resolves the
// per-route through f.dcb.RequestRouteConfig() + the factoryState's
// resolvePerRouteConfig lazy-cache + STORES the result on f.activePerRoute;
// subsequent invocations return f.activePerRoute verbatim — even after a
// hypothetical mid-stream ClearRouteCache (per parent §5.P7: "the per-route
// resolved at DecodeHeaders time stays in effect for the entire bidi-stream's
// lifetime, even across ClearRouteCache invocations — mirrors phase-10/17
// precedents").
//
// Defensive nil-tolerance:
//
//   - f.state == nil: returns a fresh zero-value *resolvedPerRoute (no
//     listener context; the test path may not set state). Cached for
//     subsequent calls.
//   - f.dcb == nil: returns the listener-level fallback via state's
//     resolvePerRouteConfig(nil). Cached for subsequent calls.
//
// The cache discipline is a single-goroutine read/write — the framework's
// sequential decode→encode dispatch provides happens-before ordering per ADR
// for the encode-side reads of f.activePerRoute (the encode-side call site
// at Task 11 EncodeHeaders reads the cached value without re-resolving). No
// mutex needed because the resolvePerRoute call site is single-threaded
// per-stream.
func (f *filter) resolvePerRoute() *resolvedPerRoute {
	if f.activePerRoute != nil {
		return f.activePerRoute
	}
	// First-call resolution path.
	var pr *resolvedPerRoute
	switch {
	case f.state == nil:
		pr = &resolvedPerRoute{}
	case f.dcb == nil:
		pr = f.state.resolvePerRouteConfig(nil)
	default:
		pr = f.state.resolvePerRouteConfig(f.dcb.RequestRouteConfig())
	}
	f.activePerRoute = pr

	// Carryforward G (Task 11 rework): cache the active ProcessorClient at
	// resolve time so openProcessorStream + dispatchStage read a single
	// resolved pointer (NOT cc.grpcClient directly) — per-route grpc_service
	// override wins over listener-level when present. The selection is
	// captured ONCE per filter lifetime per parent §5.P7 cache-on-first-use
	// discipline (one stream per HTTP transaction; the per-route choice fixes
	// the client for that stream's lifetime even if the per-route is mutated
	// mid-stream by a hypothetical ClearRouteCache).
	if f.cc != nil {
		f.activeProcessorClient = pickActiveProcessorClient(f.cc, pr)
	}
	return pr
}

// pickActiveProcessorClient returns the per-stream effective
// *grpcclient.ProcessorClient: per-route override's processorClient when
// non-nil, else the listener-level cc.grpcClient. Returns nil in HTTP-mode
// listeners (cc.grpcClient is nil — the http_service arm uses per-stage POSTs
// instead of bidi-stream Process). Factored out so the resolvePerRoute call
// site + the Group 8 rework tests share the same selection rule.
func pickActiveProcessorClient(cc *compiledConfig, pr *resolvedPerRoute) *grpcclient.ProcessorClient {
	if cc == nil {
		return nil
	}
	if pr != nil && pr.processorClient != nil {
		return pr.processorClient
	}
	return cc.grpcClient
}

// ---------------------------------------------------------------------------
// Skeleton-reachability anchor lives in extproc_test.go's
// TestSkeletonReachability — references every Task 2 type + field + helper
// so the `unused` linter does not flag the scaffolding. Each Tasks 4-11
// consumer (processor.go, check.go, attributes.go, the Task 11
// buildCompiledConfig integration) retires one or more references at its
// landing commit. By Task 11 every field has a real read site and the
// anchor test is removable.
// ---------------------------------------------------------------------------
