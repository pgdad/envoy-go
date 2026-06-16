package router

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.router type URL. Boot wiring
// in cmd/envoy-go/main.go registers New under this key in the HTTPRegistry
// (per ADR-0072) at Task 20.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"

// bad502Body is the local-reply body for upstream-failure 502 emissions per
// SPEC §11.9. The constant is the single source of truth so the H2 router's
// 502 prose cannot drift from any future H1 router 502 prose; phase-04's H1
// 502 currently uses an empty body via writeStatusReply(bw, 502, "") and is
// NOT touched by 05.2 (no regression). The shared-constant pattern future-
// proofs the migration. Migrated verbatim from internal/filter/hcm/actions.go
// at phase 07.1 Task 11; the duplication with the hcm-package constant is
// intentional per the PLAN's Task 11 → Task 12 split.
const bad502Body = "bad gateway\n"

// errCloseAfterAction is returned by routerAction.do when the action's
// response carried Connection: close (or the equivalent semantic on the
// upstream-routed response). The connection loop checks for this sentinel
// via errors.Is and closes the downstream after the current iteration.
//
// SPEC §10 #3 settled to the sentinel-error mechanism (option (a)). Other
// non-nil errors from do trigger downstream close + log (the connection
// loop handles).
var errCloseAfterAction = errors.New("router: action requested connection close")

// serverHeader returns the canonical Server header value for HCM-locally-
// generated responses. ADR-0014 (admin /ready precedent) reaffirmed for HCM
// per ADR-0044 / SPEC §10 #12 settled. Duplicated from internal/filter/hcm/codec.go
// at phase 07.1 Task 11 so the router package's locally-synthesized error
// responses do not require a cross-package call.
func serverHeader() string { return "envoy" }

// dateHeader returns the current Date header value formatted as RFC 7231
// IMF-fixdate (e.g. "Sun, 06 Nov 2024 08:49:37 GMT"). Per-response computation
// (no caching) per SPEC §10 #8 settled.
func dateHeader() string { return time.Now().UTC().Format(http.TimeFormat) }

// writeStatusReply writes a complete HTTP/1.1 local-reply response to w. The
// status line uses http.StatusText for the reason phrase; if the status is
// unknown to stdlib, the reason phrase is empty. Headers in fixed order:
// Content-Type, Content-Length, Server, Date. A CRLF blank line then the body.
func writeStatusReply(w io.Writer, status int, body string) error {
	reason := http.StatusText(status)
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Content-Type: text/plain\r\nContent-Length: %d\r\nServer: %s\r\nDate: %s\r\n\r\n",
		len(body), serverHeader(), dateHeader()); err != nil {
		return err
	}
	if body != "" {
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
	}
	return nil
}

// upstreamHostString renders cluster.Endpoint as `host:port` for the access-log
// UPSTREAM_HOST operator. Zero-valued Endpoint (host == "") yields empty
// string; the formatter then emits the literal `-` per accesslog Decision A.
// Duplicated from internal/filter/hcm/accesslog_emit.go.
func upstreamHostString(ep cluster.Endpoint) string {
	if ep.Host == "" {
		return ""
	}
	return ep.Host + ":" + strconv.Itoa(int(ep.Port))
}

// ActionResponse is the logical response shape produced by an Action. It is
// the unit fed to the encode chain (Phase 07.1 Task 18 prereq P2): HCM
// dispatch invokes the chain's RunEncodeHeaders/RunEncodeData over the
// response BEFORE writing wire bytes, so encode-side filters (e.g. cors) can
// inject/modify headers on the actual upstream response.
//
// Status is the finalized HTTP response code; Headers is the response header
// set (mutable — filters mutate this in place via the chain); Body is the
// response body bytes (for direct_response: the inline_string; for cluster
// proxying: the upstream response body bytes, fully buffered before encode).
//
// On the H1 path, the wire-bytes serialization is the chain-completion
// dispatch's responsibility (status line + headers + body via writeStatusReply
// or http.Response.Write). On the H2 path, dispatch maps Status into the
// :status pseudo-header and writes HEADERS+DATA frames via h2.StreamWriter.
//
// Phase 07.1 Task 19 (I-3 prereq): Headers is the OrderedHeaders carrier (was
// http.Header pre-Task-19). The unification with the SendLocalReply path
// collapses ~80 LoC of helper duplication (writeH1Reply{,Ordered} +
// writeH2Reply{,Ordered}) into a single ordered helper per codec. Encode-
// chain integration: HCM dispatch projects Headers.ToHTTPHeader() into a
// mutable map for RunEncodeHeaders, then reconciles via
// envoyhttp.ReconcileOrderedHeaders so caller-supplied insertion order is
// preserved across filter mutations. See PROGRESS Task 19 close-out for
// trade-offs around upstream-response wire-order (lost via Go's net/http
// resp.Header map iteration; alphabetical-by-canonical-name as substitute).
type ActionResponse struct {
	Status  int
	Headers envoyhttp.OrderedHeaders
	Body    []byte
	// Close is true if the action requested the connection be closed after
	// the response (H1 only; H2 ignores). Surfaced via this field rather than
	// errCloseAfterAction so the wire-write path can observe it without
	// shadowing real errors. Phase 07.1 Task 18 prereq P1 surfaces this
	// alongside the response struct.
	Close bool
}

// Action is the per-request executor injected by HCM dispatch into the
// terminal router filter. HCM dispatch resolves the matched route into one
// of these closures (direct_response synthesize OR upstream cluster dial)
// and calls *Filter.SetAction before iteration begins; the router invokes
// it from RunAction after decode chain iteration returns.
//
// Phase 07.1 Task 18 prereq P1: the signature is reduced to (resp, picked, err).
// bytesSent is removed from the Action's surface — the chain owns wire-byte
// accounting via len(resp.Body) + the chain's encode-side iteration. The
// Action no longer takes a *bufio.Writer: wire-bytes serialization moved out
// of Action and into HCM dispatch's chain-completion path so the encode chain
// can mutate the response BEFORE the wire-write fires.
//
// Returning (resp, picked, err): resp.Status is the finalized HTTP response
// code (used by HCM to Inc the downstream_rq_<Nxx> bucket and to populate
// the access-log record); resp.Body provides bytesSent via len(); picked is
// the cluster.Endpoint actually dialed (zero-value for direct_response); err
// is the action error (e.g. a writer failure on cluster proxying).
type Action func(ctx context.Context, req *http.Request) (resp ActionResponse, picked cluster.Endpoint, err error)

// H2Action is the H2-flavored counterpart to Action — the per-request executor
// injected by HCM dispatch into the terminal router filter on the H2 path.
// HCM dispatch (h2dispatch.go) resolves the matched route into one of these
// closures (direct_response synthesize OR upstream H2 RoundTrip) and calls
// *Filter.SetH2Action before chain iteration begins; the router invokes it
// from RunAction after the decode chain returns.
//
// Phase 07.1 Task 18 prereq P1: signature reduced to (resp, picked, err).
// status==0 in resp.Status is the H2 ctx-cancel sentinel per SPEC §2.1 last
// bullet — HCM dispatch's chain-completion access-log emit hook skips
// submission on status==0. err carrying an *h2.Error propagates upward;
// serverStream.dispatch then emits the matching RST_STREAM(<code>) per the
// dispatch carry-error contract.
//
// Note: H2 actions populate ActionResponse.Headers with HTTP-style header
// fields (regular header keys, no colon-prefixed pseudo-headers). The
// chain-completion dispatch path injects the :status pseudo-header at
// wire-write time from resp.Status.
type H2Action func(ctx context.Context, req h2.H2Request) (resp ActionResponse, picked cluster.Endpoint, err error)

// Filter is the terminal HTTP filter (envoy.filters.http.router) implementing
// envoyhttp.StreamDecoderFilter + envoyhttp.StreamEncoderFilter per ADR-0071.
// It dispatches the resolved route action — cluster dial OR direct_response
// synthesize — driven by the chain's iteration callbacks (Tasks 13–16 wire
// HCM dispatch through to the chain).
//
// At Task 15 the iteration-protocol surface lands as a working H1 endpoint:
// HCM dispatch (connection.go) populates the per-request action + request
// via SetAction / SetRequest BEFORE chain.RunDecodeHeaders; the router's
// RunAction invokes the action after the decode chain returns and captures
// (response, picked, actionErr) for HCM dispatch to read via
// Status/Response/Picked/ActionErr. Phase 07.1 Task 18 prereq P1 removed the
// SetWriter setter and the (status, bytesSent) tuple — wire-bytes
// serialization is now HCM dispatch's responsibility post-encode-chain.
//
// The accessLog field carries the access-log sink slice plumbed in via the
// per-request factory (Task 14 wires the HCM Filter's accessLog through to
// the router instance). Tests (`router_test.go`) exercise the access-log
// emit directly by constructing &Filter{accessLog: []accesslog.Sink{...}}
// — this matches the byte-preserved shape from internal/filter/hcm.
//
// Per-request lifecycle (single-goroutine-per-stream — see ADR-0071's
// "single dispatch goroutine drives the chain" invariant):
//
//  1. HCM allocates a fresh `*Filter` from the per-instance factory (router.New's
//     returned FilterInstanceFactory closure runs once per request).
//  2. HCM calls `SetRequest`, `SetAction` in any order, both BEFORE
//     chain iteration begins (i.e. BEFORE chain.RunDecodeHeaders). On the H2
//     path, SetH2Action + SetH2Request are called instead.
//  3. The chain's iteration callbacks (`SetDecoderCallbacks`, `SetEncoderCallbacks`,
//     `DecodeHeaders/Data/Trailers`, `EncodeHeaders/Data/Trailers`) run during
//     chain dispatch on the chain's single dispatch goroutine.
//  4. HCM calls `RunAction(ctx)` exactly once after `chain.RunDecodeHeaders`
//     (and `RunDecodeData` if body) returns.
//  5. HCM reads result fields via `Status / Response / Picked / ActionErr /
//     ActionRan` AFTER `RunAction` returns; runs the response through the
//     encode chain (`RunEncodeHeaders` / `RunEncodeData`); finally writes
//     wire bytes via codec-specific writeH1Reply / writeH2Reply.
//  6. The `*Filter` is then discarded; `OnDestroy` callback fires.
//
// The single-goroutine-per-stream invariant means the result-capture fields
// (`actionRan`, `actionResponse`, `actionPicked`, `actionErr`) require no
// synchronization: only one goroutine ever reads or writes them per request.
// Future filters that schedule encode-side work on a separate goroutine MUST
// synchronize externally before invoking RunAction or reading the getters;
// the chain framework's parkEncode/resume machinery preserves the invariant
// by routing the resume back through the same dispatch goroutine.
type Filter struct {
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	// accessLog holds the configured access-log sinks plumbed through from
	// the HCM Filter (Task 14). Nil when no sinks are configured.
	accessLog []accesslog.Sink

	// Per-request injection (Task 15). HCM dispatch sets these before
	// chain.RunDecodeHeaders begins iteration.
	//
	// Phase 07.1 Task 18 prereq P1: the *bufio.Writer (bw) is REMOVED from the
	// per-request injection set. Wire-bytes serialization moved from the
	// Action into HCM dispatch's chain-completion path so the encode chain
	// can mutate the response before wire-write. SetWriter is preserved as a
	// no-op tombstone so existing test scaffolding compiles; will be deleted
	// in a follow-up cleanup.
	action Action
	req    *http.Request

	// Per-request H2 injection (Task 16). HCM dispatch sets these on the H2
	// path before chain.RunDecodeHeaders begins iteration. Mutually exclusive
	// with the H1 trio above: HCM populates ONE set per request based on the
	// listener's negotiated codec. RunAction routes to the populated path.
	//
	// Phase 07.1 Task 18 prereq P1: h2Sw REMOVED from the per-request
	// injection set; wire-bytes serialization moved out of Action.
	h2Action H2Action
	h2Req    h2.H2Request

	// Per-request action result (populated when action runs in RunAction).
	// HCM dispatch reads these via the public getters after RunAction returns.
	//
	// Phase 07.1 Task 18 prereq P1: actionBytesSent removed (chain owns
	// wire-byte accounting; HCM dispatch reads len(actionResponse.Body)
	// directly when populating the access-log record). actionResponse holds
	// the logical response surfaced by the action; HCM dispatch feeds it to
	// the encode chain BEFORE wire-write so encode-side filters (cors etc.)
	// can mutate headers/body.
	actionRan      bool
	actionResponse ActionResponse
	actionPicked   cluster.Endpoint
	actionErr      error
}

// SetAction wires the per-request action closure resolved by HCM dispatch
// from the matched route. Called once per request, BEFORE chain iteration.
func (f *Filter) SetAction(a Action) { f.action = a }

// SetRequest wires the *http.Request into the router for the action's H1
// upstream call. Called once per request, BEFORE chain iteration.
func (f *Filter) SetRequest(r *http.Request) { f.req = r }

// SetH2Action is the H2-side mirror of SetAction (Task 16). HCM h2dispatch.go
// injects the per-request H2 action closure resolved from the matched route.
// Called once per request, BEFORE chain iteration. Mutually exclusive with
// SetAction — the same *Filter instance must NOT receive both an H1 and an
// H2 action; HCM dispatch picks ONE based on the listener's negotiated codec.
func (f *Filter) SetH2Action(a H2Action) { f.h2Action = a }

// SetH2Request wires the codec-internal H2Request into the router for the
// action's upstream H2 RoundTrip. Called once per request, BEFORE chain
// iteration.
func (f *Filter) SetH2Request(r h2.H2Request) { f.h2Req = r }

// Status / Response / Picked / ActionErr expose the action's terminal
// outcome so HCM dispatch can Inc the downstream_rq_<Nxx> bucket, populate
// the access-log record, run the encode chain over the response, and write
// wire bytes after chain return. Zero values when the action did not run
// (e.g. SendLocalReply pre-empted the terminal filter).
//
// Phase 07.1 Task 18 prereq P1: BytesSent() getter REMOVED — HCM dispatch
// reads len(rf.Response().Body) directly when populating the access-log
// record. Response() returns the logical response struct surfaced by the
// action; HCM dispatch feeds it through the encode chain BEFORE wire-write.
// Status returns the action response status code (0 if action did not run).
func (f *Filter) Status() int { return f.actionResponse.Status }

// Response returns the logical action response captured at RunAction time.
func (f *Filter) Response() ActionResponse { return f.actionResponse }

// Picked returns the cluster endpoint chosen by the action (zero value if none).
func (f *Filter) Picked() cluster.Endpoint { return f.actionPicked }

// ActionErr returns any error captured during RunAction (nil on success).
func (f *Filter) ActionErr() error { return f.actionErr }

// ActionRan reports whether the per-request Action has already executed.
func (f *Filter) ActionRan() bool { return f.actionRan }

// RunAction invokes the per-request Action and captures the outcome. Idempotent
// once-per-request via the `actionRan` boolean (a check-then-set, NOT a sync.Once
// — see lifecycle doc on `Filter`).
//
// Routes to either the H1 path (action+req+bw populated via Set*) or the H2
// path (h2Action+h2Req+h2Sw populated via SetH2*) based on which setter trio
// HCM dispatch invoked. The two trios are mutually exclusive: HCM populates
// exactly ONE per request based on the listener's negotiated codec.
//
// Concurrency contract: this method assumes the single-goroutine-per-stream
// invariant codified by ADR-0071's chain-dispatch model. The HCM dispatch path
// (connection.go's `dispatchRequest` for H1; h2dispatch.go's per-stream
// dispatch for H2) is the sole caller per request, runs on a single goroutine,
// and reads the result fields only AFTER this method returns. The result-
// capture fields (actionRan / actionStatus / actionBytesSent / actionPicked /
// actionErr) are written here without synchronization — that is safe ONLY
// under the single-goroutine invariant. Concurrent invocations from multiple
// goroutines are NOT supported and would race on `actionRan`. Future filters
// that schedule encode-side work on a separate goroutine MUST synchronize
// externally before calling RunAction.
//
// Called by HCM dispatch AFTER chain.RunDecodeHeaders + RunDecodeData +
// RunDecodeTrailers complete (the terminal-action invocation logically sits
// "after" the decode chain).
//
// Per Decision §3.1, the access-log emit is deferred to HCM dispatch's
// chain-completion hook (which reads Status/BytesSent/Picked from this
// filter); the router filter does NOT emit the access-log directly post-Task-15.
//
// Panics with a clear message if neither trio was populated before RunAction.
// This is programmer-error early-detection per the per-request lifecycle
// doc on `*Filter` (step 2 must precede step 4). The HCM dispatch path always
// calls one trio of setters before chain iteration; a panic here means a
// future caller has violated the contract.
func (f *Filter) RunAction(ctx context.Context) {
	if f.actionRan {
		return
	}
	switch {
	case f.h2Action != nil:
		// H2 path (Task 16). h2Req is a value type so the zero value is allowed.
		f.actionRan = true
		resp, picked, err := f.h2Action(ctx, f.h2Req)
		f.actionResponse = resp
		f.actionPicked = picked
		f.actionErr = err
	case f.action != nil:
		// H1 path (Task 15). req must be set.
		if f.req == nil {
			panic("router.Filter.RunAction: SetAction set without SetRequest (per *Filter lifecycle doc; ADR-0071 single-goroutine-per-stream invariant)")
		}
		f.actionRan = true
		resp, picked, err := f.action(ctx, f.req)
		f.actionResponse = resp
		f.actionPicked = picked
		f.actionErr = err
	default:
		panic("router.Filter.RunAction: neither SetAction nor SetH2Action was called before RunAction (per *Filter lifecycle doc; ADR-0071 single-goroutine-per-stream invariant)")
	}
}

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode in normal flow, but the guard is uniform across H1+H2 callers.
// Migrated verbatim from internal/filter/hcm/accesslog_emit.go.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       r.Method,
		Path:         r.URL.Path,
		Protocol:     r.Proto,
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    r.Host,
		UserAgent:    r.Header.Get("User-Agent"),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// SetDecoderCallbacks stores the framework-supplied decoder callbacks. The
// terminal filter uses dcb.EncodeHeaders / dcb.EncodeData to dispatch the
// upstream response back through the encode chain (Task 13–16 wire-up).
func (f *Filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks stores the framework-supplied encoder callbacks.
func (f *Filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders is the entry point for the terminal-filter dispatch. The
// full route-action driving (cluster.Dial → req.Write → ReadResponse → encode)
// is wired by HCM dispatch in Tasks 15+16 (which pass the resolved route
// action + cluster handle through the chain). At Task 11, the skeleton
// returns Continue so the framework's chain.RunDecodeHeaders advances past
// the router as the terminal filter; the byte-preserved tests below
// exercise routerAction.do / routerActionH2.doH2 directly.
func (f *Filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// DecodeData is the body-side counterpart to DecodeHeaders. Routing dispatch
// runs once end_stream is observed (Tasks 15+16).
func (f *Filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is the trailer-side counterpart to DecodeData.
func (f *Filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders is the encode-side pass-through; the terminal filter does
// not modify upstream response headers.
func (f *Filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// EncodeData is the encode-side body pass-through.
func (f *Filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is the encode-side trailer pass-through.
func (f *Filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy releases per-request state. The router's per-instance state is
// callback-pointer + access-log slice; both are GC-tracked references with
// no cleanup required.
func (f *Filter) OnDestroy() {}

// New is the HTTPFilterFactory exposed at boot via filter.HTTPRegistry.Register
// (Task 20 wires the boot registration). The router proto carries no fields
// envoy-go consumes at Task 11 — every field in envoy.extensions.filters.http.router.v3.Router
// is in the silent-ignore set inherited from ADR-0040. tc may be nil (default
// router config); a non-nil tc is unmarshaled-and-discarded only to honor the
// ADR-0072 "factories validate typed_config shape" contract; payload fields
// remain silently ignored at this phase.
func New(_ *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	return func() envoyhttp.HTTPFilter {
		f := &Filter{}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.router",
			Decoder: f,
			Encoder: f,
		}
	}, nil
}

// HashKind enumerates the supported RouteAction.hash_policy specifiers (36.2).
type HashKind uint8

// HashKind values for the supported RouteAction.hash_policy specifiers.
const (
	HashKindHeader   HashKind = iota // xxHash64 over the matched request-header value
	HashKindSourceIP                 // cluster.HashSourceIP over the downstream client IP
)

// HashPolicy is a lowered, proto-free RouteAction.hash_policy entry. The proto
// read + DEPARTURE-reject happens at the hcm config-build boundary
// (parseRouteHashPolicies); this descriptor is router-owned because the
// per-request fold (applyHashKey) lives here. ADR-0237.
type HashPolicy struct {
	Kind       HashKind
	HeaderName string // lowercased; set for HashKindHeader only
	Terminal   bool   // RouteAction_HashPolicy.terminal — short-circuit once acc is non-empty
}

// downstreamRemoteAddrCtxKey is the private ctx key carrying the downstream
// client's remote address for the source_ip hash_policy producer.
type downstreamRemoteAddrCtxKey struct{}

// WithDownstreamRemoteAddr carries the downstream client's remote address
// ("host:port") for the source_ip hash_policy producer. HCM dispatch sets it
// before invoking the action (H1: connection.go; H2: h2dispatch.go) since
// neither *http.Request (HCM codec path) nor h2.H2Request carries a remote
// addr. ADR-0237.
func WithDownstreamRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, downstreamRemoteAddrCtxKey{}, addr)
}

func downstreamRemoteAddrFrom(ctx context.Context) string {
	v, _ := ctx.Value(downstreamRemoteAddrCtxKey{}).(string)
	return v
}

// applyHashKey folds the route's hash_policy list into a ring_hash key and
// returns ctx carrying it (cluster.WithHashKey). If nothing contributes, ctx is
// returned unchanged (the LB's no-hash fallback). Mirrors Envoy
// HashPolicyImpl::generateHash (SPEC §3.2 / hash_policy.cc:259-270):
// rotl64(prev,1)^new fold, first contributor verbatim, nullopt policies skipped
// entirely, a terminal policy short-circuiting once the accumulator is
// non-empty. headerVal is a codec-agnostic accessor; remoteAddr is the
// ctx-carried downstream addr (empty when unset). Returns (ctx, key, has) for
// testability; the dial sites use only ctx.
//
// D-S362-4: the H1/H2 headerVal shims return the FIRST header value
// (single-value producer path); the full multi-value byte-sorted fold lives in
// cluster.HashHeaderValues (Task 2, unit-tested).
func applyHashKey(ctx context.Context, hps []HashPolicy, headerVal func(name string) (string, bool), remoteAddr string) (context.Context, uint64, bool) {
	var acc uint64
	var has bool
	for _, hp := range hps {
		var nh uint64
		var ok bool
		switch hp.Kind {
		case HashKindHeader:
			if v, present := headerVal(hp.HeaderName); present {
				nh, ok = cluster.HashHeaderValues([]string{v}), true
			}
		case HashKindSourceIP:
			if remoteAddr != "" {
				nh, ok = cluster.HashSourceIP(remoteAddr), true
			}
		}
		if ok {
			if has {
				acc = bits.RotateLeft64(acc, 1) ^ nh
			} else {
				acc, has = nh, true
			}
		}
		if hp.Terminal && has {
			break // hash_policy.cc:270 — checks the accumulator
		}
	}
	if !has {
		return ctx, 0, false
	}
	return cluster.WithHashKey(ctx, acc), acc, true
}

// H1ClusterAction returns an Action closure that proxies the per-request H1
// upstream call to the supplied cluster's selected endpoint. The closure
// delegates to the package-private routerAction.do (the byte-preserved H1
// upstream-driving logic from phase 04 / 06.1 — preserved verbatim under
// router_test.go regression tests). HCM dispatch (connection.go) builds one
// of these per matched route at filter-build time and injects it via
// *Filter.SetAction at request-start.
//
// ErrCloseAfterAction returned by the closure signals the HCM connection
// loop to close the downstream after the response writes (per SPEC §5.3 +
// §10 #3 settled).
func H1ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) Action {
	a := &routerAction{cluster: c, hashPolicies: hps, subsetMatch: subsetMatch}
	return func(ctx context.Context, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
		// Phase 07.1 Task 18 prereq P1: doH1ClusterAction returns an
		// ActionResponse logical shape (no wire-bytes serialization here).
		// HCM dispatch's chain-completion path runs the encode chain over
		// the response THEN writes wire bytes — see internal/filter/hcm/
		// connection.go's dispatchRequest at the chain-mediated dispatch path.
		return doH1ClusterAction(ctx, a, req)
	}
}

// ErrCloseAfterAction is the exported sentinel signaling "downstream close
// after this response writes" per SPEC §10 #3 settled. HCM dispatch checks
// errors.Is(err, router.ErrCloseAfterAction) to drive the connection-loop
// close-after-iteration semantics.
var ErrCloseAfterAction = errCloseAfterAction

// doH1ClusterAction runs the per-request H1 upstream-dial dispatch and surfaces
// the logical ActionResponse for the Action closure. Phase 07.1 Task 18 prereq
// P1: returns an ActionResponse (status + headers + body + close) instead of
// writing wire bytes directly. The HCM dispatch chain-completion path runs
// the response through the encode chain THEN writes wire bytes via writeH1Reply.
//
// On dial / req.Write / ReadResponse failures the function synthesizes a 502
// or 503 ActionResponse shape (status + minimal headers + empty body); the
// HCM dispatch path treats these uniformly with proxied responses (encode
// chain runs; wire-write happens in dispatch). This collapses the previous
// "writeStatusReply directly into bw on failure path" with the chain-mediated
// path into a single shape.
func doH1ClusterAction(ctx context.Context, a *routerAction, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
	picked := cluster.Endpoint{}

	a.cluster.IncUpstreamRqTotal()

	// 36.2: fold the route's hash_policy list into a ring_hash key carried on
	// ctx (cluster.WithHashKey) → ringHashLB.Pick reads it in AcquireH1. The H1
	// codec path carries no remote addr on *http.Request, so source_ip uses the
	// ctx-carried downstream addr set by HCM dispatch (connection.go). ADR-0237.
	ctx, _, _ = applyHashKey(ctx, a.hashPolicies, func(n string) (string, bool) {
		v := req.Header.Get(n)
		return v, v != ""
	}, downstreamRemoteAddrFrom(ctx))

	// 38.1: thread the route-static metadata_match onto ctx → subsetLB.Pick
	// reads it in AcquireH1. Route-static (resolved once at config-build, NOT a
	// per-request fold like applyHashKey); threaded verbatim only when non-empty
	// (an empty match leaves ctx untouched → the subsetLB fallback path). ADR-0239.
	if !a.subsetMatch.Empty() {
		ctx = cluster.WithSubsetMatch(ctx, a.subsetMatch)
	}

	pooled, err := a.cluster.AcquireH1(ctx)
	if err != nil {
		a.cluster.IncStatusClass(503)
		return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}, picked, nil
	}
	upstream := pooled.Conn
	// reusable is flipped to true on the happy path right before return; on
	// every error/early-exit path the deferred drop closes the conn instead
	// of returning it to the keep-alive pool.
	reusable := false
	defer func() {
		if reusable {
			a.cluster.PutIdleH1(pooled)
		} else {
			_ = upstream.Close()
		}
	}()
	picked = pooled.Endpoint()

	// Propagate the downstream ctx deadline (if any) to the upstream socket
	// so a stalled upstream cannot hold the action past the ctx's deadline
	// during req.Write or http.ReadResponse — both of which are otherwise
	// ctx-unaware (REVIEW.md I-3 from REVIEW.md 04527eb).
	if dl, ok := ctx.Deadline(); ok {
		_ = upstream.SetDeadline(dl)
	}

	if err := req.Write(upstream); err != nil {
		a.cluster.IncStatusClass(502)
		return ActionResponse{Status: 502, Headers: localReplyHeaders(0), Body: nil}, picked, nil
	}

	// Reuse the pooled bufio.Reader so that any read-ahead buffered bytes
	// (e.g. start of the next response on a pipelined-but-not-here connection)
	// are not silently discarded; for the simple sequential-keep-alive case
	// the reader's internal buffer just persists with empty state after each
	// drain.
	br := pooled.BufioReader()
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		a.cluster.IncStatusClass(502)
		return ActionResponse{Status: 502, Headers: localReplyHeaders(0), Body: nil}, picked, nil
	}
	defer func() { _ = resp.Body.Close() }()

	a.cluster.IncStatusClass(resp.StatusCode)
	a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: resp.StatusCode})

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ActionResponse{Status: resp.StatusCode}, picked, err
	}

	// Decide pool reuse eligibility. Keep-alive applies when:
	//   - the upstream did NOT signal Close (resp.Close is set when either the
	//     response uses HTTP/1.0 without keep-alive, or carries
	//     "Connection: close")
	//   - the request itself did not request close (req.Close)
	//   - the protocol is HTTP/1.1+ (resp.ProtoMajor==1 && ProtoMinor>=1)
	//   - the body was successfully drained above (no read error)
	if !resp.Close && !req.Close && resp.ProtoMajor == 1 && resp.ProtoMinor >= 1 {
		// Clear the per-request deadline before parking the conn in the pool
		// so a stale deadline doesn't fire on the next caller's I/O.
		_ = upstream.SetDeadline(time.Time{})
		reusable = true
	}

	// Build the response headers from the upstream response. Phase 07.1 Task 19
	// (I-3 prereq): project resp.Header (Go map; iteration non-deterministic)
	// into an OrderedHeaders carrier with deterministic alphabetical key
	// ordering. The original wire-order from the upstream is LOST inside Go's
	// net/http response parser (resp.Header is a map); alphabetical-by-canonical-
	// name is the deterministic substitute. Content-Length is recomputed at
	// wire-write time from len(Body).
	return ActionResponse{
		Status:  resp.StatusCode,
		Headers: envoyhttp.OrderedHeadersFromHTTPHeader(resp.Header),
		Body:    bodyBytes,
		Close:   resp.Close,
	}, picked, nil
}

// localReplyHeaders builds the standard local-reply header set for synthesized
// 5xx responses on the cluster-dial / write / read failure paths. Mirrors
// writeStatusReply's wire shape.
//
// Phase 07.1 Task 19 (I-3 prereq): returns OrderedHeaders so the action-
// driven wire-emit path (writeH1Reply / writeH2Reply on ActionResponse)
// preserves the four-header insertion order (Content-Type, Content-Length,
// Server, Date) — matching writeStatusReply's verbatim shape from phase-04.
func localReplyHeaders(bodyLen int) envoyhttp.OrderedHeaders {
	return envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Content-Length", Value: strconv.Itoa(bodyLen)},
		{Name: "Server", Value: serverHeader()},
		{Name: "Date", Value: dateHeader()},
	}
}

// routerAction proxies the request to the named cluster's selected endpoint.
// Per ADR-0039, every routed request opens a fresh upstream connection via
// Cluster.Dial(ctx); no pooling at phase 04. Per-failure-class mapping:
//
//	Cluster.Dial error      → 503 local reply, do returns nil
//	Request.Write error     → 502 local reply, do returns nil
//	http.ReadResponse error → 502 local reply, do returns nil
//	resp.Write error        → propagated up (downstream is broken)
//
// The router does NOT inject x-envoy-*, x-forwarded-*, or x-request-id
// headers (SPEC §2). The upstream sees the unmodified downstream request
// (modulo stdlib's textproto canonicalization on header names).
//
// Migrated from internal/filter/hcm/actions.go at phase 07.1 Task 11 with
// signatures preserved byte-for-byte so the byte-preserved tests in
// router_test.go exercise the same shape (`a.do(ctx, req, bw)`).
type routerAction struct {
	cluster      *cluster.Cluster
	filter       *Filter             // set post-build by routeTable.bindFilter; nil when no sinks configured.
	hashPolicies []HashPolicy        // 36.2: stored at H1ClusterAction; per-request fold lands in Task 4 (applyHashKey).
	subsetMatch  cluster.SubsetMatch // 38.1: route-static metadata_match threaded onto ctx at dispatch (ADR-0239).
}

// do drives one upstream H1 round-trip. Phase 06.1 Task 11 wires the cluster-
// scope upstream_rq_total + upstream_rq_<Nxx> counters per SPEC §5.5: total
// Inc's at dispatch entry (once per attempt, BEFORE Dial); the status-class
// counter Inc's after the response code is finalized. On the dial-failure /
// write-failure / read-failure local-reply paths, the synthesized 5xx status
// (503 for dial, 502 for write/read) is the bucket the cluster-scope counter
// reflects per the "5xx Inc lands on the dial-failure local-reply path too"
// annotation in PLAN Task 11.
//
// Returns (statusCode, error). statusCode is the finalized HTTP response code
// (503/502 on local-reply paths; resp.StatusCode on a successful proxy);
// err is the routeAction error (errCloseAfterAction sentinel or a real
// write-side failure).
func (a *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	start := time.Now()

	// bytesSent counts only the response body bytes for BYTES_SENT per SPEC
	// §12 #3 option (a). We do NOT wrap bw in a byteCounterWriter because
	// resp.Write writes the full HTTP/1.1 wire bytes (status line + headers +
	// body), which would inflate the count. Instead, we read the body into a
	// buffer, record its length, replace resp.Body with the buffered reader, and
	// let resp.Write drain the buffer through bw directly.
	bytesSent := int64(0)

	// statusCode and picked are captured by the deferred access-log emit so
	// the closure always reads the final values after all writes have completed.
	statusCode := 0
	picked := cluster.Endpoint{}
	if a.filter != nil {
		defer func() { a.filter.emitAccessLog(req, statusCode, bytesSent, picked, start) }()
	}

	a.cluster.IncUpstreamRqTotal()

	// 36.2: fold the route's hash_policy list into a ring_hash key carried on
	// ctx (cluster.WithHashKey) → ringHashLB.Pick reads it in Dial. See
	// doH1ClusterAction for the source_ip ctx-carry rationale. ADR-0237.
	ctx, _, _ = applyHashKey(ctx, a.hashPolicies, func(n string) (string, bool) {
		v := req.Header.Get(n)
		return v, v != ""
	}, downstreamRemoteAddrFrom(ctx))

	upstream, ep, err := a.cluster.Dial(ctx)
	if err != nil {
		a.cluster.IncStatusClass(503)
		statusCode = 503
		return 503, writeStatusReply(bw, 503, "")
	}
	defer func() { _ = upstream.Close() }()
	picked = ep

	// Propagate the downstream ctx deadline (if any) to the upstream socket
	// so a stalled upstream cannot hold the action past the ctx's deadline
	// during req.Write or http.ReadResponse — both of which are otherwise
	// ctx-unaware (REVIEW.md I-3 from REVIEW.md 04527eb).
	if dl, ok := ctx.Deadline(); ok {
		_ = upstream.SetDeadline(dl)
	}

	if err := req.Write(upstream); err != nil {
		a.cluster.IncStatusClass(502)
		statusCode = 502
		return 502, writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		a.cluster.IncStatusClass(502)
		statusCode = 502
		return 502, writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	a.cluster.IncStatusClass(resp.StatusCode)
	statusCode = resp.StatusCode

	// Read the entire response body to count body bytes for BYTES_SENT.
	// Replace resp.Body with a bytes.Reader so resp.Write drains the same
	// bytes downstream without a second upstream read.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	bytesSent = int64(len(bodyBytes))
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	if err := resp.Write(bw); err != nil {
		return resp.StatusCode, err
	}
	// Honor the upstream's Connection: close (and HTTP/1.0 close-by-default)
	// by signaling the connection loop via errCloseAfterAction. http.ReadResponse
	// populates resp.Close from the wire-level signals (Connection: close on
	// HTTP/1.1, default-close on HTTP/1.0). SPEC §5.3 / SPEC §10 #3 settled.
	// REVIEW.md I-1 from REVIEW.md 04527eb.
	if resp.Close {
		return resp.StatusCode, errCloseAfterAction
	}
	return resp.StatusCode, nil
}

// Compile-time interface conformance assertions. *Filter must satisfy both
// StreamDecoderFilter + StreamEncoderFilter so the chain framework can drive
// it as a both-sides filter (terminal: decode dispatches; encode forwards).
var (
	_ envoyhttp.StreamDecoderFilter = (*Filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*Filter)(nil)
	_ envoyhttp.HTTPFilterFactory   = New
)
