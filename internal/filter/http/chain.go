package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
)

// filterBufferLimitBytes is the per-stream body-buffer cap matching Envoy's
// default. Per ADR-0076 (lands in Task 9). Defined here at chain.go's head so
// later tasks can reference it without re-declaration.
const filterBufferLimitBytes = 1 << 20 // 1 MiB

// localReply413Body is the verbatim 17-byte ASCII body of the synthesized
// 413 Payload Too Large response. Per SPEC §11 #3 empirical pin: no trailing
// newline. Pinned here to ensure the body shape can never drift relative to
// the wire-shape contract asserted in TestChain_DecodeData_OverflowSynthesizes413.
const localReply413Body = "Payload Too Large" // 17 bytes

// errEncodeBufferOverflow is the sentinel returned by RunEncodeData when the
// per-stream encode-side buffer cap (filterBufferLimitBytes) is exceeded. The
// HCM dispatch path resets the connection (H1 close, H2 RST_STREAM) on this
// sentinel — wired in Tasks 15 + 16. Per ADR-0076.
var errEncodeBufferOverflow = errors.New("chain: encode-side buffer overflow; resetting connection")

// FilterChain is the per-stream state machine that drives iteration of HTTP
// filters. Allocated by HCM dispatch (connection.go for H1, h2dispatch.go for
// H2) at the start of each request via NewFilterChain.
//
// Concurrency invariant (per ADR-0071): the HCM dispatch goroutine is the
// only goroutine that drives RunDecode* / RunEncode* methods. Filter callbacks
// (ContinueDecoding / ContinueEncoding) are signal-only — they unblock the
// dispatch goroutine via channel send.
type FilterChain struct {
	filters  []HTTPFilter
	perRoute *PerRouteConfig // optional; nil if no per-route config

	// Iteration cursors (per Decision §3.5: two int cursors).
	decodeIdx int
	encodeIdx int

	// Async-resume signal channels (capacity 1; non-blocking sends; idempotent
	// coalesce). Written from any goroutine via callback methods; read only by
	// the dispatch goroutine.
	decodeResumeCh chan struct{}
	encodeResumeCh chan struct{}

	// Body buffers. Per ADR-0076: decode-side overflow synthesizes a 413 local
	// reply (verbatim shape per SPEC §11 #3); encode-side overflow returns the
	// errEncodeBufferOverflow sentinel from RunEncodeData (HCM dispatch resets
	// the connection — Tasks 15 + 16). decodeBufOver is set when the decode-side
	// cap is crossed and gates the overflow path; the framework's behavior is
	// driven by the local-reply path / sentinel return. Encode-side tracks both
	// the running byte count (encodeBufLen — cap-check) AND the chain-level
	// accumulated buffer (encodeBuf — the ADR-0175 buffer-and-hold primitive
	// landed at phase-19.2 Task 2). encodeBuf accumulates incoming EncodeData
	// chunks whenever an encode-side filter returns DataStopIterationAndBuffer;
	// on resume at end_stream the chain releases the (possibly-mutated)
	// accumulated buffer downstream as the new payload for the remaining
	// encoders + clears encodeBuf. Pre-ADR-0175 the encode-side StopIterationAndBuffer
	// disposition was park-only (no replay buffer existed); ADR-0175 closes
	// the encode-side gap symmetrically to ADR-0128 decode-side.
	decodeBuf     []byte
	decodeBufOver bool
	encodeBuf     []byte
	encodeBufLen  int

	// SendLocalReply guard (Task 7).
	localReplyOnce sync.Once
	localReplyDone atomic.Bool

	// localReplyResponse carries the synthesized response shape from
	// SendLocalReply so HCM dispatch's wire-write path can emit the bytes
	// after the chain's beginLocalReply runs the encode side. Phase 07.1
	// Task 18 prereq P2: pre-Task-18 the chain ran the encode chain but did
	// not surface the synthesized response to dispatch — wire-write was the
	// Action's responsibility and the SendLocalReply path bypassed Action.
	// Now dispatch reads this via LocalReplyResponse() when localReplyDone
	// is set.
	localReplyStatus  int
	localReplyHeaders OrderedHeaders
	localReplyBody    []byte

	// Encode-side started flag (Task 7) — second SendLocalReply after this is
	// a no-op + log line.
	encodeStarted atomic.Bool

	// Per-request state set by HCM dispatch via SetRequestCtx (Task 7; HCM wire-up at Task 13).
	// routeIdx is the matched-route index used by RequestRouteConfig's perRoute lookup.
	// ambientCtx is the request context propagated to beginLocalReply when a filter
	// triggers SendLocalReply (since SendLocalReply itself does not take a ctx).
	routeIdx   int
	ambientCtx context.Context

	// tlsPrincipals carries the priority-ordered TLS principal-name candidates
	// (URI SAN → DNS SAN → Subject DN CN) extracted by HCM dispatch from the
	// downstream *tls.Conn's ConnectionState at chain build time. nil for
	// plaintext / non-mTLS / no-client-cert connections per ADR-0144
	// §Decision (iii) + ADR-0143 §Decision (vi) case (c). Set once via
	// SetTLSPrincipals before RunDecodeHeaders dispatch; read by per-stream
	// decoderCB.DownstreamPrincipal() callbacks during filter iteration.
	//
	// Phase-16 ADR-0144 framework primitive (cross-phase reusable; future
	// filters jwt_authn / ext_authz / oauth2 / ext_proc consume the same
	// accessor surface). Per-stream-stateless after set: no mutation across
	// filter dispatch — the slice is read-only per the single-dispatch-
	// goroutine invariant (ADR-0071).
	tlsPrincipals []string

	// ADR-0165 callback-surface extension (phase-18.2 Task 4 — the ADR-0044
	// escape-valve firing per planner-time decision D3 + D12). The 6 fields
	// below carry per-stream socket / TLS / listener-cert state needed by
	// ext_authz gRPC-mode's AttributeContext builder (and cross-phase reusable
	// by ext_proc + global_ratelimit + future ext_authz extensions). Each
	// field is set ONCE at chain build time by HCM dispatch (connection.go H1
	// + h2dispatch.go H2) BEFORE RunDecodeHeaders dispatch; per-stream
	// decoderCB callback methods read the seeded value verbatim — no copy,
	// no transformation — per the same ownership-invariant ADR-0071 codifies
	// for tlsPrincipals (single-dispatch-goroutine read; signal-only
	// concurrent writes are NOT permitted on these fields).
	//
	// Zero-value semantics (no SetX call by HCM dispatch):
	//   - downstreamRemoteAddr / downstreamLocalAddr: nil (synthetic stream)
	//   - downstreamTLSServerName: "" (plaintext or SNI-absent handshake)
	//   - downstreamTLSPeerCertDER: nil (plaintext or no-client-cert)
	//   - downstreamProtocol: "" (synthetic stream; H1/H2 dispatch always seeds)
	//   - listenerPrincipal: "" (plaintext listener — no transport_socket)
	downstreamRemoteAddr     net.Addr
	downstreamLocalAddr      net.Addr
	downstreamTLSServerName  string
	downstreamTLSPeerCertDER []byte
	downstreamProtocol       string
	listenerPrincipal        string

	// Phase 24.1 Task 5 (ADR-0198 — the route-table rate_limits exposure
	// framework primitive). routeRateLimits + virtualHostRateLimits carry the
	// matched route's + parent virtual_host's RAW
	// []*routev3.RateLimit policy slices seeded by HCM dispatch at chain build
	// time (BEFORE RunDecodeHeaders). Filter callbacks read via
	// decoderCB.RouteRateLimits() / VirtualHostRateLimits(); the
	// descriptor-action INTERPRETATION stays filter-owned (the ratelimit
	// filter's engine consumes the policies — the framework surfaces RAW
	// proto slices).
	//
	// Zero-value semantics (no SetX call by HCM dispatch):
	//   - routeRateLimits: nil (route has no rate_limits[], or the
	//     no-match-route 404 path where HCM elides the seed; or a synthetic
	//     stream test that bypasses HCM dispatch entirely)
	//   - virtualHostRateLimits: nil (vhost has no rate_limits[], or the
	//     same no-match / synthetic paths above)
	//
	// Set-once invariant per ADR-0071: the single dispatch goroutine writes
	// these fields ONCE before filter iteration; filter callbacks are
	// read-only. No mutex required. Mirrors the ADR-0165 DownstreamRemoteAddr
	// plumbing template — the D-RL1 byte-confirmation outcome at phase-24.1
	// Task 5 IMPL: the raw-proto seed fits the existing chain primitive
	// without divergence (ADR-0202 UNCONSUMED).
	//
	// Narrow-exposure / YAGNI: ONLY rate_limits is exposed here; other
	// route-level data (cors, retry, etc.) stays on the
	// RequestRouteConfig() / typed_per_filter_config path per ADR-0073.
	routeRateLimits       []*routev3.RateLimit
	virtualHostRateLimits []*routev3.RateLimit

	// Phase 24.2 Task 1 (D-RL8 — RouteMetadata accessor extension; ADR-0165
	// set-once-by-dispatch + ADR-0198 DELTA-2 chain-field plumbing template).
	// routeMetadata carries the matched route's RAW *corev3.Metadata as parsed
	// from `Route.metadata` at HCM build time. Consumed by the `ratelimit`
	// filter's `metadata` descriptor action under MetadataSource_ROUTE_ENTRY=1
	// per parent SPEC §4.1 row 8. nil for synthetic streams, for routes with
	// no metadata, or for the no-match-route 404 path (HCM elides the seed).
	//
	// Zero-value semantics (no SetX call by HCM dispatch):
	//   - routeMetadata: nil (route has no metadata, or the no-match-route 404
	//     path where HCM elides the seed; or a synthetic stream test that
	//     bypasses HCM dispatch entirely)
	//
	// Set-once invariant per ADR-0071: HCM dispatch writes the field ONCE
	// before filter iteration; filter callbacks are read-only. No mutex
	// required. Mirrors the 24.1 DELTA-2 RouteRateLimits plumbing template.
	//
	// Narrow-exposure / YAGNI: ONLY the ratelimit filter's `metadata` action
	// (MetadataSource_ROUTE_ENTRY=1) consumes this at 24.2; other route-level
	// metadata access (cors, retry, etc.) stays on the RequestRouteConfig() /
	// typed_per_filter_config path per ADR-0073.
	routeMetadata *corev3.Metadata

	// Phase 24.2 Task 4 (D-RL11 — RouteIncludeVhRateLimits legacy-bool accessor
	// extension; ADR-0165 set-once-by-dispatch + ADR-0198 DELTA-2 chain-field
	// plumbing template). routeIncludeVhRateLimits carries the matched route's
	// legacy `RouteAction.include_vh_rate_limits` bool as parsed at HCM build
	// time. Consumed by the `ratelimit` filter's §4.3 Axis-B vhost-walk
	// composition table per parent SPEC §4.3 + AMEND-5: when true, the vhost-
	// level RateLimit policy slice is ALWAYS walked regardless of
	// `RateLimitPerRoute.vh_rate_limits` (the legacy force-include override
	// trumps the enum). Default false ⇒ honor the enum (OVERRIDE / INCLUDE /
	// IGNORE per §4.3).
	//
	// Zero-value semantics (no SetX call by HCM dispatch):
	//   - routeIncludeVhRateLimits: false (route has no
	//     include_vh_rate_limits, or the no-match-route 404 path where HCM
	//     elides the seed; or a synthetic stream test that bypasses HCM
	//     dispatch entirely)
	//
	// Set-once invariant per ADR-0071: HCM dispatch writes the field ONCE
	// before filter iteration; filter callbacks are read-only. No mutex
	// required. Mirrors the 24.1 DELTA-2 RouteRateLimits + 24.2 Task 1
	// RouteMetadata plumbing template.
	//
	// Narrow-exposure / YAGNI: ONLY the ratelimit filter's §4.3 Axis-B
	// composition table consumes this at 24.2 — the legacy bool is a
	// route-level RouteAction field with no other consumer.
	routeIncludeVhRateLimits bool

	// encodeResponseStatus carries the HTTP response status code (e.g. 200,
	// 503) for the stream being encoded. Set ONCE by HCM dispatch via
	// SetEncodeResponseStatus (connection.go H1 + h2dispatch.go H2 from
	// resp.Status, and the beginLocalReply path from the local-reply status)
	// BEFORE RunEncodeHeaders dispatch; read by per-stream
	// encoderCB.ResponseStatus() callbacks during encode-filter iteration.
	// Zero-value 0 = unset (synthetic stream with no status). Mirrors the
	// set-once-by-dispatch / read-via-accessor discipline of downstreamProtocol
	// et al. (ADR-0165); the single-dispatch-goroutine invariant (ADR-0071)
	// applies — no mutation across filter dispatch. Added per ADR-0196.
	encodeResponseStatus int

	// tlsConnectionState carries the FULL *tls.ConnectionState extracted by
	// HCM dispatch from the downstream *tls.Conn at chain build time. Nil
	// for plaintext / non-mTLS connections (HCM dispatch elides
	// SetTLSConnectionState on those paths). Set ONCE BEFORE RunDecodeHeaders
	// per ADR-0071 set-once discipline; read concurrently by per-stream
	// decoderCB.DownstreamTLSConnectionState() / encoderCB.DownstreamTLS
	// ConnectionState() callbacks during filter iteration.
	//
	// Distinct from the ADR-0165 derived projections (downstreamTLSServerName /
	// downstreamTLSPeerCertDER) — those carry pre-extracted scalar fields for
	// the ext_authz/ext_proc AttributeContext envelope. The full state is
	// needed by the phase-22.2 lua bridge's :connection():ssl() surface (12
	// ssl methods consume PEM-encoded certs + cert chain + cipher suite +
	// tls version + verification details directly from the state per SPEC
	// §11.5.4 wire-format conventions).
	//
	// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
	// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD — no separate ADR
	// for chain-side extension). Cross-phase reusable framework primitive.
	tlsConnectionState *tls.ConnectionState

	// dynamicMetadata is the per-stream cross-filter dynamic-metadata Bucket
	// (per ADR-0190 + SPEC §3 + §3.1). Initialized at chain construction via
	// dynamicmetadata.NewBucket(); Reset + nil-out at Destroy. Per-stream
	// sequential per ADR-0033 (no cross-filter concurrency within a stream;
	// no mutex required). All Bucket methods are nil-receiver tolerant per
	// ADR-0085 — post-Destroy reads through the callback accessor return
	// nil-tolerantly.
	//
	// Per ADR-0192 §Decision body anticipation. Cross-phase reusable
	// framework primitive (consumed by the phase-22.2 lua
	// :dynamicMetadata() / :dynamicTypedMetadata() / :setDynamicMetadata()
	// surface; future filters may consume the same accessor for cross-filter
	// state propagation per ADR-0190 §Consequences).
	dynamicMetadata *dynamicmetadata.Bucket

	// encodeBodyOverride / encodeBodyOverridden carry the encode-side body
	// replacement bytes registered via EncoderFilterCallbacks.OverwriteBody.
	// The sentinel discriminates (override is nil bytes + set) from (no
	// override registered). HCM dispatch (connection.go H1 / h2dispatch.go H2)
	// reads via EncodeBodyOverride() after RunEncodeData returns and
	// substitutes resp.Body before the wire-write path consumes it.
	// Per ADR-0131 §Decision (vi).
	encodeBodyOverride   []byte
	encodeBodyOverridden bool

	// diagLogW overrides the destination of the framework's diagnostic log
	// lines. Default nil → stderr. Test-only setter SetDiagLogWriter swaps in
	// a buffer to capture the log line in TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs.
	diagLogW io.Writer

	// Per-stream destroyed-once guard.
	destroyOnce sync.Once
}

// NewFilterChain allocates a per-stream chain. filters is the chain config
// expanded by per-request factory invocation (caller supplies fresh instances).
// perRoute may be nil.
func NewFilterChain(filters []HTTPFilter, perRoute *PerRouteConfig) *FilterChain {
	c := &FilterChain{
		filters:        filters,
		perRoute:       perRoute,
		decodeResumeCh: make(chan struct{}, 1),
		encodeResumeCh: make(chan struct{}, 1),
		// Default-init ambientCtx so any code path that calls SendLocalReply
		// before HCM dispatch invokes SetRequestCtx (tests, or pre-Task-13
		// callers) does not propagate a nil ctx into parkEncode/parkDecode
		// where `<-ctx.Done()` on a nil interface would panic. SetRequestCtx
		// overwrites this in production. Per code-quality review on a03a1d3.
		ambientCtx: context.Background(),
		// Initialize the per-stream dynamic-metadata Bucket at chain
		// construction per ADR-0190 + ADR-0192 §Decision body anticipation.
		// The chain owns the bucket lifecycle: created here, Reset + nil-out
		// at Destroy. Per-stream sequential per ADR-0033 — no mutex required.
		dynamicMetadata: dynamicmetadata.NewBucket(),
	}
	// Wire per-filter callback structs (concrete impl tied to this chain).
	for i := range filters {
		idx := i
		if d := filters[i].Decoder; d != nil {
			d.SetDecoderCallbacks(&decoderCB{c: c, idx: idx})
		}
		if e := filters[i].Encoder; e != nil {
			e.SetEncoderCallbacks(&encoderCB{c: c, idx: idx})
		}
	}
	return c
}

// RunDecodeHeaders iterates the decode-side filters in declaration order.
// Returns (terminated=true) if iteration completed; (terminated=false, err) if
// aborted by ctx-cancel or SendLocalReply.
func (c *FilterChain) RunDecodeHeaders(ctx context.Context, headers http.Header, endStream bool) (bool, error) {
	for c.decodeIdx < len(c.filters) {
		if c.localReplyDone.Load() {
			// SendLocalReply called from a previous filter; encode chain runs in Task 7.
			return false, nil
		}
		f := c.filters[c.decodeIdx].Decoder
		if f == nil {
			c.decodeIdx++
			continue
		}
		status := f.DecodeHeaders(headers, endStream)
		// If the filter triggered SendLocalReply during DecodeHeaders, the
		// encode chain has already run synchronously inside beginLocalReply;
		// abort decode iteration immediately regardless of returned status
		// (do NOT park even if StopIteration was returned, since no
		// ContinueDecoding will arrive — the request is terminated).
		if c.localReplyDone.Load() {
			return false, nil
		}
		switch status {
		case Continue:
			c.decodeIdx++
		case StopIteration:
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterHeadersStatus %d", c.filters[c.decodeIdx].Name, status)
		}
	}
	return true, nil
}

// parkDecode waits on decodeResumeCh, ctx.Done, or returns the appropriate
// error. Single-goroutine invariant — only the dispatch goroutine calls this.
func (c *FilterChain) parkDecode(ctx context.Context) error {
	select {
	case <-c.decodeResumeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunDecodeData iterates decode-side body chunks in declaration order. Per
// ADR-0076 + SPEC §11 #3 empirical pin: a filter returning
// DataStopIterationAndBuffer accumulates body bytes into c.decodeBuf up to
// filterBufferLimitBytes; an overflowing chunk synthesizes a verbatim 413
// Payload Too Large local reply (entered through beginLocalReply, which runs
// the FULL encode chain in reverse declaration order per ADR-0075). After the
// 413 fires, the chain state machine has transitioned to encode mode and
// RunDecodeData returns (false, nil).
//
// The decode iteration cursor resets to 0 at the top of every RunDecodeData
// call (mirrors the encode-side cursor reset in RunEncodeData) — RunDecodeHeaders
// has already advanced the cursor past len(filters) by the time we get here.
func (c *FilterChain) RunDecodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
	c.decodeIdx = 0
	for c.decodeIdx < len(c.filters) {
		if c.localReplyDone.Load() {
			return false, nil
		}
		f := c.filters[c.decodeIdx].Decoder
		if f == nil {
			c.decodeIdx++
			continue
		}
		status := f.DecodeData(data, endStream)
		// If the filter triggered SendLocalReply during DecodeData, the encode
		// chain has already run synchronously inside beginLocalReply; abort
		// decode-data iteration immediately regardless of returned status.
		if c.localReplyDone.Load() {
			return false, nil
		}
		switch status {
		case DataContinue:
			c.decodeIdx++
		case DataStopIterationAndBuffer:
			// Buffer cap enforcement per ADR-0076 + SPEC §6.5: if appending the
			// current chunk would exceed filterBufferLimitBytes, synthesize the
			// verbatim 413 wire shape (per SPEC §11 #3 empirical pin) — the
			// chain transitions to encode mode via beginLocalReply.
			if len(c.decodeBuf)+len(data) > filterBufferLimitBytes {
				c.decodeBufOver = true
				// Connection: close per the §11 #3 empirical pin — forces the
				// H1 conn to terminate after the local reply emits. The HCM
				// dispatch path reads this header on the synthesized response
				// and closes the conn after writing (Task 15).
				headers := OrderedHeaders{
					{Name: "Connection", Value: "close"},
				}
				c.beginLocalReply(c.ambientCtx, c.decodeIdx, 413, localReply413Body, headers)
				return false, nil
			}
			c.decodeBuf = append(c.decodeBuf, data...)
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		case DataStopIterationNoBuffer:
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterDataStatus %d on decode", c.filters[c.decodeIdx].Name, status)
		}
	}
	return true, nil
}

// RunDecodeTrailers iterates decode-side trailers in declaration order.
// Phase 07.1 Task 19: added to support the envoygotest probe filter's
// stop-trailers mode (DecodeTrailers returns TrailersStopIteration; resume
// via dcb.ContinueDecoding). HCM dispatch (connection.go / h2dispatch.go)
// does not yet drive trailers — H1 trailers are gated on chunked transfer-
// encoding which the phase-04..07.1 fixture set does not exercise; H2
// trailers are observed-and-discarded in the codec layer per ADR-0058.
// envoygotest's chain-direct test exercises this method without HCM
// dispatch.
func (c *FilterChain) RunDecodeTrailers(ctx context.Context, trailers http.Header) (bool, error) {
	c.decodeIdx = 0
	for c.decodeIdx < len(c.filters) {
		if c.localReplyDone.Load() {
			return false, nil
		}
		f := c.filters[c.decodeIdx].Decoder
		if f == nil {
			c.decodeIdx++
			continue
		}
		status := f.DecodeTrailers(trailers)
		if c.localReplyDone.Load() {
			return false, nil
		}
		switch status {
		case TrailersContinue:
			c.decodeIdx++
		case TrailersStopIteration:
			if err := c.parkDecode(ctx); err != nil {
				return false, err
			}
			c.decodeIdx++
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterTrailersStatus %d on decode", c.filters[c.decodeIdx].Name, status)
		}
	}
	return true, nil
}

// RunEncodeHeaders iterates the encode-side filters in REVERSE declaration
// order per SPEC §5.5 + §11.1 empirical pin. Returns (terminated=true) when
// iteration completes; (terminated=false, err) if aborted by ctx-cancel.
// Mirror of RunDecodeHeaders with the cursor traversing len-1 → 0.
func (c *FilterChain) RunEncodeHeaders(ctx context.Context, headers http.Header, endStream bool) (bool, error) {
	c.encodeStarted.Store(true)
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeHeaders(headers, endStream)
		switch status {
		case Continue:
			c.encodeIdx--
		case StopIteration:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterHeadersStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	return true, nil
}

// RunEncodeData iterates encode-side body chunks in reverse declaration order.
// Per ADR-0076: tracks per-stream encode-side buffer accumulation against
// filterBufferLimitBytes; if a chunk would push the cumulative size above the
// cap, returns errEncodeBufferOverflow without iterating any filter on that
// chunk. The HCM dispatch path resets the connection (H1 close, H2 RST_STREAM)
// on this sentinel — wired in Tasks 15 + 16.
//
// Per ADR-0175 (phase-19.2 Task 2 — envoy-go's FIRST encode-side body-
// buffering framework primitive): when an encoder filter returns
// DataStopIterationAndBuffer, the chain ACCUMULATES the current chunk into
// c.encodeBuf (analogous to the decode-side c.decodeBuf at RunDecodeData)
// AND parks the dispatch goroutine on encodeResumeCh. The buffering filter
// inspects the accumulated bytes via EncoderFilterCallbacks.BufferEncodedBody()
// and unparks via ContinueEncoding(). On resume at end_stream, the chain
// RELEASES the (possibly-mutated) accumulated buffer downstream as the new
// `data` payload for the remaining encoders (i.e., the next filters in the
// reverse-iteration order see the buffered bytes rather than the original
// chunk) AND CLEARS c.encodeBuf — closing the buffering window per ADR-0175
// §Decision. Mid-stream chunks (endStream=false on the buffering call)
// accumulate-and-park without releasing; the buffer persists across
// RunEncodeData invocations until end_stream closes the window. This
// diverges DataStopIterationAndBuffer from DataStopIterationNoBuffer (the
// latter stays park-only / continue-iteration on this same chunk — no
// accumulation, no release).
func (c *FilterChain) RunEncodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
	// Buffer-cap check up front: the sentinel is returned BEFORE iterating any
	// filter, so the connection-reset wire path never observes a partially-
	// emitted overflowing chunk. Per ADR-0076 + SPEC §15 acceptance bullet 2.
	// Per ADR-0175 D7 (planner-time): overflow handling is SYMMETRIC to the
	// decode-side errDecodeBufferOverflow path — the same filterBufferLimitBytes
	// cap governs encode-side accumulation; once exceeded, the chain returns
	// errEncodeBufferOverflow + HCM dispatch resets the connection.
	if c.encodeBufLen+len(data) > filterBufferLimitBytes {
		return false, errEncodeBufferOverflow
	}
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeData(data, endStream)
		switch status {
		case DataContinue:
			c.encodeIdx--
		case DataStopIterationAndBuffer:
			// ADR-0175 §Decision accumulation: append the current chunk into
			// the chain-level encodeBuf BEFORE parking — so the filter can
			// inspect via BufferEncodedBody() from inside its EncodeData
			// callback OR from a concurrent goroutine that wakes the dispatch
			// via ContinueEncoding. The accumulation is in-place via append;
			// the chain owns the backing array, callers MUST treat the slice
			// returned from BufferEncodedBody as read-aliased.
			c.encodeBuf = append(c.encodeBuf, data...)
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			// ADR-0175 §Decision release-and-clear at end_stream: the buffering
			// window closes on the last EncodeData call (endStream=true). The
			// chain releases the (possibly-mutated by the filter via the
			// reader-aliased slice OR via OverwriteBody) accumulated buffer
			// downstream as the new `data` payload for the remaining filters
			// in this reverse-iteration. Mid-stream chunks (endStream=false)
			// leave the buffer intact + advance to the next filter with the
			// ORIGINAL chunk — preserving the cross-call accumulation
			// invariant (the filter MAY return DataStopIterationAndBuffer
			// across multiple RunEncodeData invocations and the buffer
			// accumulates across all of them until end_stream closes the
			// window).
			//
			// Cap-counter discipline (phase-19.2 Task 2 rework): on release we
			// reassign `data` to the UNION buffer (c.encodeBuf — already counts
			// the prior-chunk per-call bumps the post-loop encodeBufLen += len(data)
			// recorded on previous RunEncodeData calls). Subtract that prior
			// accumulation here so the post-loop bump below counts each byte
			// exactly once. See the cap-counter comment at the post-loop site.
			if endStream {
				priorAccum := len(c.encodeBuf) - len(data)
				data = c.encodeBuf
				c.encodeBuf = nil
				c.encodeBufLen -= priorAccum
			}
			c.encodeIdx--
		case DataStopIterationNoBuffer:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterDataStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	// Iteration completed; record this call's contribution to the encode-side
	// cap. Per ADR-0175 the cap tracks CUMULATIVE DISTINCT BODY BYTES seen by
	// the chain — each byte counted EXACTLY ONCE whether it transited as a
	// per-call `data` argument or as part of an accumulated-then-released
	// buffer. The release-at-end_stream branch above subtracts the prior
	// accumulation from encodeBufLen BEFORE we reassign `data = c.encodeBuf`,
	// so the bump below does not double-count the previously-buffered chunks
	// (those bytes were already counted on their original RunEncodeData calls).
	// The running total is the source of truth for the cap-vs-overflow check
	// at the top of the next RunEncodeData call.
	c.encodeBufLen += len(data)
	return true, nil
}

// RunEncodeTrailers iterates encode-side trailers in reverse declaration order.
func (c *FilterChain) RunEncodeTrailers(ctx context.Context, trailers http.Header) (bool, error) {
	c.encodeIdx = len(c.filters) - 1
	for c.encodeIdx >= 0 {
		f := c.filters[c.encodeIdx].Encoder
		if f == nil {
			c.encodeIdx--
			continue
		}
		status := f.EncodeTrailers(trailers)
		switch status {
		case TrailersContinue:
			c.encodeIdx--
		case TrailersStopIteration:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterTrailersStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	return true, nil
}

// parkEncode waits on encodeResumeCh, ctx.Done, or returns the appropriate
// error. Single-goroutine invariant — only the dispatch goroutine calls this.
func (c *FilterChain) parkEncode(ctx context.Context) error {
	select {
	case <-c.encodeResumeCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Destroy fires OnDestroy on every filter exactly once. Safe to call multiple
// times; idempotent.
//
// Per ADR-0192 §Decision body anticipation: Destroy ALSO Reset+nil-outs the
// per-stream dynamicMetadata bucket — Reset clears the bucket's entries (per
// ADR-0190 Reset discipline; nil-tolerant per ADR-0085) and the nil-out
// invalidates subsequent DynamicMetadata() accessor calls on cached callback
// structs so post-Destroy reads return nil (the bucket consumer surface is
// nil-tolerant per ADR-0085, making this safe).
func (c *FilterChain) Destroy() {
	c.destroyOnce.Do(func() {
		for _, f := range c.filters {
			if f.Decoder != nil {
				f.Decoder.OnDestroy()
			} else if f.Encoder != nil {
				f.Encoder.OnDestroy()
			}
		}
		// Bucket Reset + nil-out per ADR-0192. Reset is nil-tolerant per
		// ADR-0085 (safe even though destroyOnce guards re-entry; defensive).
		c.dynamicMetadata.Reset()
		c.dynamicMetadata = nil
	})
}

// decoderCB is the framework's concrete impl of DecoderFilterCallbacks for a
// specific filter index. ContinueDecoding is non-blocking; channel send is
// coalesced via the buffered-1 channel.
type decoderCB struct {
	c   *FilterChain
	idx int
}

func (d *decoderCB) ContinueDecoding() {
	select {
	case d.c.decodeResumeCh <- struct{}{}:
	default:
	}
}

func (d *decoderCB) SendLocalReply(status int, body string, headers OrderedHeaders) {
	d.c.beginLocalReply(d.c.ambientCtx, d.idx, status, body, headers)
}

// RequestRouteConfig returns the merged proto.Message for the calling filter's
// name via the chain's perRoute lookup at the route-index supplied by HCM
// dispatch (chain.routeIdx, set by SetRequestCtx). Returns nil if the chain
// has no perRoute config or no scope carries an entry for this filter.
func (d *decoderCB) RequestRouteConfig() proto.Message {
	if d.c.perRoute == nil {
		return nil
	}
	return d.c.perRoute.Resolve(d.c.filters[d.idx].Name, d.c.routeIdx)
}

// RequestRouteConfigsAllTiers returns the unmerged per-tier configs for the
// calling filter's name via the chain's perRoute lookup at the route-index
// supplied by HCM dispatch (chain.routeIdx, set by SetRequestCtx). Returns
// (nil, nil, nil) if the chain has no perRoute config or no scope carries
// an entry for this filter at any tier.
func (d *decoderCB) RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message) {
	if d.c.perRoute == nil {
		return nil, nil, nil
	}
	return d.c.perRoute.ResolveAllTiers(d.c.filters[d.idx].Name, d.c.routeIdx)
}

func (d *decoderCB) EncodeHeaders(http.Header, bool) {}
func (d *decoderCB) EncodeData([]byte, bool)         {}
func (d *decoderCB) EncodeTrailers(http.Header)      {}

// DownstreamPrincipal returns the priority-ordered TLS principal-name
// candidates from the chain's HCM-seeded tlsPrincipals field. Returns nil
// for plaintext / non-mTLS / no-client-cert connections (HCM dispatch does
// NOT call SetTLSPrincipals on such connections per ADR-0144 §Decision (iii)).
// Per ADR-0144 §Decision (i)+(ii) framework primitive; cross-phase reusable.
func (d *decoderCB) DownstreamPrincipal() []string {
	return d.c.tlsPrincipals
}

// The 6 reader methods below are the per-stream filter-facing accessors for
// the ADR-0165 callback-surface extension (phase-18.2 Task 4 — the ADR-0044
// escape-valve firing per planner-time decision D3 + D12). Each method
// returns the chain field verbatim; HCM dispatch seeds the field via the
// matching SetX primitive BEFORE RunDecodeHeaders. Zero-value returns
// (nil / "") mirror the chain field's documented zero-value semantics —
// see chain.FilterChain struct field comments.
//
// Per ADR-0165 §Decision. Cross-phase reusable; ext_proc / global_ratelimit /
// future ext_authz extensions consume the same accessor surface.

func (d *decoderCB) DownstreamRemoteAddr() net.Addr {
	return d.c.downstreamRemoteAddr
}

func (d *decoderCB) DownstreamLocalAddr() net.Addr {
	return d.c.downstreamLocalAddr
}

func (d *decoderCB) DownstreamTLSServerName() string {
	return d.c.downstreamTLSServerName
}

func (d *decoderCB) DownstreamTLSPeerCertDER() []byte {
	return d.c.downstreamTLSPeerCertDER
}

func (d *decoderCB) DownstreamProtocol() string {
	return d.c.downstreamProtocol
}

func (d *decoderCB) ListenerPrincipal() string {
	return d.c.listenerPrincipal
}

// RouteRateLimits returns the matched route's HCM-seeded raw
// []*routev3.RateLimit policy slice (set via SetRouteRateLimits before
// RunDecodeHeaders). Returns nil for synthetic streams, for routes with no
// rate_limits[], or for the no-match-route 404 path where HCM elides the
// seed. Per ADR-0198 §Decision (phase-24.1 Task 5 / DELTA-2).
func (d *decoderCB) RouteRateLimits() []*routev3.RateLimit {
	return d.c.routeRateLimits
}

// VirtualHostRateLimits returns the matched route's parent virtual_host's
// HCM-seeded raw []*routev3.RateLimit policy slice (set via
// SetVirtualHostRateLimits before RunDecodeHeaders). Same zero-value
// semantics as RouteRateLimits. Per ADR-0198 §Decision.
func (d *decoderCB) VirtualHostRateLimits() []*routev3.RateLimit {
	return d.c.virtualHostRateLimits
}

// RouteMetadata returns the matched route's HCM-seeded RAW *corev3.Metadata
// (set via SetRouteMetadata before RunDecodeHeaders). Returns nil for
// synthetic streams, for routes with no metadata, or for the no-match-route
// 404 path where HCM elides the seed. Per phase 24.2 Task 1 (D-RL8) —
// extension of the ADR-0165 set-once-by-dispatch + ADR-0198 DELTA-2 plumbing
// template. Consumed by the ratelimit filter's `metadata` descriptor action
// under MetadataSource_ROUTE_ENTRY=1 per parent SPEC §4.1 row 8.
func (d *decoderCB) RouteMetadata() *corev3.Metadata {
	return d.c.routeMetadata
}

// RouteIncludeVhRateLimits returns the matched route's HCM-seeded legacy
// `RouteAction.include_vh_rate_limits` bool (set via SetRouteIncludeVhRateLimits
// before RunDecodeHeaders). Returns false for synthetic streams, for routes
// without the field set, or for the no-match-route 404 path where HCM elides
// the seed. Per phase 24.2 Task 4 (D-RL11) — extension of the ADR-0165
// set-once-by-dispatch + ADR-0198 DELTA-2 plumbing template. Consumed by the
// ratelimit filter's §4.3 Axis-B vhost-walk composition table per parent SPEC
// §4.3 + AMEND-5: when true, the vhost-level RateLimit policy slice is ALWAYS
// walked regardless of `RateLimitPerRoute.vh_rate_limits` (the legacy force-
// include override trumps the enum).
func (d *decoderCB) RouteIncludeVhRateLimits() bool {
	return d.c.routeIncludeVhRateLimits
}

// DownstreamTLSConnectionState returns the chain's HCM-seeded FULL
// *tls.ConnectionState (set once via SetTLSConnectionState before
// RunDecodeHeaders). Returns nil for plaintext / non-mTLS / no-client-cert
// connections — HCM dispatch elides SetTLSConnectionState on those paths per
// the same discipline as ADR-0144 §Decision (iii). Per ADR-0192 §Decision
// body anticipation + SPEC §11.5.3.
//
// Consumed by the phase-22.2 lua bridge's :connection():ssl() metatable
// (12 ssl methods extract PEM-encoded certs, cipher suite, TLS version, etc.
// from the state per SPEC §11.5.4). Cross-phase reusable for any future
// filter needing access to the full TLS handshake state.
func (d *decoderCB) DownstreamTLSConnectionState() *tls.ConnectionState {
	return d.c.tlsConnectionState
}

// DynamicMetadata returns the chain's per-stream dynamic-metadata Bucket.
// Initialized non-nil at chain construction via dynamicmetadata.NewBucket();
// Reset+nil-out at Destroy. Post-Destroy returns nil — consumers are
// nil-tolerant per ADR-0085 Bucket discipline. Per ADR-0192 §Decision body
// anticipation + ADR-0190 §Context.
//
// Consumed by the phase-22.2 lua bridge's :dynamicMetadata() / :dynamic
// TypedMetadata() / :setDynamicMetadata() surface. Cross-phase reusable for
// cross-filter state propagation per ADR-0190 §Consequences.
func (d *decoderCB) DynamicMetadata() *dynamicmetadata.Bucket {
	return d.c.dynamicMetadata
}

// encoderCB is the framework's concrete impl of EncoderFilterCallbacks. Same
// non-blocking send discipline.
type encoderCB struct {
	c   *FilterChain
	idx int
}

func (e *encoderCB) ContinueEncoding() {
	select {
	case e.c.encodeResumeCh <- struct{}{}:
	default:
	}
}

func (e *encoderCB) EncodeHeaders(http.Header, bool) {}
func (e *encoderCB) EncodeData([]byte, bool)         {}
func (e *encoderCB) EncodeTrailers(http.Header)      {}

// OverwriteBody registers a replacement encode-side body on the chain.
// Filters call this from inside their EncodeData(data, endStream)
// implementation; HCM dispatch reads the registered bytes via
// FilterChain.EncodeBodyOverride() after RunEncodeData returns and
// substitutes resp.Body before the wire-write path consumes it.
// Not goroutine-safe — the encode chain runs synchronously in the dispatch
// goroutine (per ADR-0071). Per ADR-0131 §Decision (vi).
func (e *encoderCB) OverwriteBody(b []byte) {
	e.c.encodeBodyOverride = b
	e.c.encodeBodyOverridden = true
}

// BufferEncodedBody returns the chain-accumulated encode-side body bytes the
// chain has appended on prior EncodeData calls where this (or another encode-
// side) filter returned DataStopIterationAndBuffer. Returns nil before any
// buffering has occurred AND nil after the chain has released the buffer
// downstream at end_stream (see ADR-0175 §Decision release-and-clear
// discipline in RunEncodeData). The returned slice ALIASES the chain's
// internal encodeBuf — callers MUST treat the underlying bytes as read-aliased;
// in-place mutation through the slice header is permitted only while the
// dispatch goroutine is parked on encodeResumeCh AND only when the calling
// filter is the (presumed-unique) buffering filter holding the resume signal
// (ADR-0175 §Decision discipline; per-stream single-goroutine invariant per
// ADR-0071). Per ADR-0175 (envoy-go's FIRST encode-side body-buffering
// framework primitive; phase-19.2 Task 2 anchor).
func (e *encoderCB) BufferEncodedBody() []byte {
	return e.c.encodeBuf
}

// The 6 reader methods below are the per-stream filter-facing accessors for
// the ADR-0174 symmetric encoder-side extension of ADR-0165's callback-
// surface family (phase-19.1 Task 5 — the ADR-0044 escape-valve firing at
// SPEC time per BRAINSTORM §11 lesson (h); planner-time D10 settles 6
// methods, NOT 7, DownstreamPrincipal staying decode-side-specific per
// ADR-0144's framing). Each method returns the SAME chain field the
// decoderCB readers at chain.go:524-546 return — verbatim, no copy, no
// transformation. NO new chain fields, NO new seeding primitives, NO new
// HCM dispatch wiring; the 6 chain fields are SET-once at HCM dispatch
// BEFORE either decode or encode dispatch (the existing SetDownstreamRemoteAddr
// / SetDownstreamLocalAddr / SetDownstreamTLSServerName /
// SetDownstreamTLSPeerCertDER / SetDownstreamProtocol / SetListenerPrincipal
// setters at chain.go:633+ are reused). Zero-value returns (nil / "")
// mirror the chain field's documented zero-value semantics — see
// FilterChain struct field comments.
//
// Per ADR-0174 §Decision. PRE-REQUISITE for phase-19.1 attributes.go (Task 9)
// encoder-side response_attributes envelope population. Cross-phase reusable
// for any future encode-side filter needing socket / TLS / listener-cert
// state.

func (e *encoderCB) DownstreamRemoteAddr() net.Addr {
	return e.c.downstreamRemoteAddr
}

func (e *encoderCB) DownstreamLocalAddr() net.Addr {
	return e.c.downstreamLocalAddr
}

func (e *encoderCB) DownstreamTLSServerName() string {
	return e.c.downstreamTLSServerName
}

func (e *encoderCB) DownstreamTLSPeerCertDER() []byte {
	return e.c.downstreamTLSPeerCertDER
}

func (e *encoderCB) DownstreamProtocol() string {
	return e.c.downstreamProtocol
}

func (e *encoderCB) ListenerPrincipal() string {
	return e.c.listenerPrincipal
}

// ResponseStatus reads the chain's encodeResponseStatus field per ADR-0196.
func (e *encoderCB) ResponseStatus() int {
	return e.c.encodeResponseStatus
}

// DownstreamTLSConnectionState (encoder side) reads the SAME chain field the
// decoder side reads — no separate chain field, no separate seeding. HCM
// dispatch sets via SetTLSConnectionState BEFORE either RunDecodeHeaders OR
// RunEncodeHeaders dispatch; both callback sides observe the same value.
// Per ADR-0192 §Decision body anticipation; symmetric to the encoder-side
// mirror of ADR-0165's DecoderFilterCallbacks accessors landed at phase-19.1
// per ADR-0174.
//
// Cross-phase reusable for any encode-side filter needing access to the full
// TLS handshake state (response-mutating filters; response-stage attribute
// emission against TLS-session metadata).
func (e *encoderCB) DownstreamTLSConnectionState() *tls.ConnectionState {
	return e.c.tlsConnectionState
}

// DynamicMetadata (encoder side) returns the chain's per-stream dynamic-
// metadata Bucket — the SAME pointer the decoder side observes (per-stream
// sequential per ADR-0033; no cross-side concurrency). Post-Destroy returns
// nil; consumers are nil-tolerant per ADR-0085. Per ADR-0192 §Decision body
// anticipation.
//
// Consumed by the phase-22.2 encode-side lua bridge's :dynamicMetadata() /
// :setDynamicMetadata() surface (the same Bucket accessed via the decode side
// surface — bucket entries persist across decode → encode iteration within a
// stream per ADR-0190 §Consequences).
func (e *encoderCB) DynamicMetadata() *dynamicmetadata.Bucket {
	return e.c.dynamicMetadata
}

// EncodeBodyOverride returns the registered encode-side body override (if any).
// Returns (override, true) if a filter called cb.OverwriteBody during the
// encode chain; (nil, false) otherwise. Callers (HCM dispatch) use this to
// substitute resp.Body before the wire-write path. Per ADR-0131 §Decision (vi).
func (c *FilterChain) EncodeBodyOverride() ([]byte, bool) {
	return c.encodeBodyOverride, c.encodeBodyOverridden
}

// SetRequestCtx wires the request context + matched-route index into the
// chain. Called by HCM dispatch at request start (HCM wire-up lands in
// Task 13). The ambient ctx is used by beginLocalReply since the
// DecoderFilterCallbacks.SendLocalReply API does not accept a ctx.
func (c *FilterChain) SetRequestCtx(ctx context.Context, routeIdx int) {
	c.ambientCtx = ctx
	c.routeIdx = routeIdx
}

// SetTLSConnectionState seeds the chain's per-stream FULL *tls.ConnectionState.
// Called by HCM dispatch (connection.go H1 / h2dispatch.go H2) at chain build
// time BEFORE RunDecodeHeaders dispatch when the downstream conn is a
// *tls.Conn with HandshakeComplete. Nil-passthrough for plaintext / non-mTLS
// connections — HCM dispatch elides the SetTLSConnectionState call on those
// paths.
//
// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD; no separate ADR for
// chain-side extension). Mirrors the ADR-0144 SetTLSPrincipals plumbing
// pattern. The seeded state is read by per-stream decoderCB.DownstreamTLS
// ConnectionState() / encoderCB.DownstreamTLSConnectionState() callbacks.
//
// Concurrency: called once per request before filter iteration starts (single
// dispatch-goroutine invariant per ADR-0071); no synchronization required.
func (c *FilterChain) SetTLSConnectionState(state *tls.ConnectionState) {
	c.tlsConnectionState = state
}

// SetDynamicMetadata replaces the chain's per-stream dynamic-metadata Bucket
// with the supplied bucket. Per ADR-0192 §Decision body anticipation.
//
// The chain auto-initializes a non-nil Bucket at NewFilterChain construction
// (via dynamicmetadata.NewBucket()), so production HCM dispatch typically
// does NOT call this setter; the setter is provided for tests + advanced
// scenarios that want to inject a pre-populated Bucket OR share a Bucket
// reference across chains (e.g., shadowing/replay test fixtures).
//
// Concurrency: called once per request before filter iteration starts (single
// dispatch-goroutine invariant per ADR-0071); no synchronization required.
func (c *FilterChain) SetDynamicMetadata(b *dynamicmetadata.Bucket) {
	c.dynamicMetadata = b
}

// SetTLSPrincipals seeds the chain's per-stream TLS principal-name candidate
// slice. Called by HCM dispatch (connection.go H1 / h2dispatch.go H2) at chain
// build time BEFORE RunDecodeHeaders dispatch when the downstream conn is a
// *tls.Conn with HandshakeComplete && len(PeerCertificates) > 0. The slice is
// priority-ordered: URI SANs first (from PeerCertificates[0].URIs), then
// DNS SANs (PeerCertificates[0].DNSNames), then the Subject DN Common Name
// (PeerCertificates[0].Subject.CommonName) — mirrors Envoy v1.37.2's
// Principal_Authenticated extraction semantics per rbac.pb.go:1432-1438.
//
// Per ADR-0144 §Decision (ii)+(iii) (phase-16 framework primitive). Per-stream
// callbacks read the seeded slice via decoderCB.DownstreamPrincipal(). HCM
// dispatch does NOT call this method on plaintext / non-mTLS / no-client-cert
// connections — the per-stream field stays nil, which prinAuthenticated's
// three-case algorithm interprets as case (c) FALSE per ADR-0143 §Decision (vi).
//
// Concurrency: called once per request before filter iteration starts (single
// dispatch-goroutine invariant per ADR-0071); no synchronization required.
func (c *FilterChain) SetTLSPrincipals(p []string) {
	c.tlsPrincipals = p
}

// SetDownstreamRemoteAddr / SetDownstreamLocalAddr / SetDownstreamTLSServerName /
// SetDownstreamTLSPeerCertDER / SetDownstreamProtocol / SetListenerPrincipal are
// the 6 chain-seeding primitives anchored by ADR-0165 (phase-18.2 Task 4 — the
// ADR-0044 escape-valve firing per planner-time decision D3 + D12). All 6
// mirror the SetTLSPrincipals discipline (set ONCE at chain build time by HCM
// dispatch BEFORE RunDecodeHeaders; read concurrently by per-stream
// callbacks). HCM dispatch sites: connection.go's dispatchRequest (H1) +
// h2dispatch.go's chainDispatchAction.WriteH2 via chainDispatchAction fields
// (H2). Zero-value semantics on the chain side are documented inline at the
// field declarations; HCM dispatch elides the SetX call when the source value
// is the natural zero (e.g. plaintext listener → no SetListenerPrincipal call,
// chain field stays "").
//
// Per ADR-0165 §Decision. Cross-phase reusable framework primitives (ext_proc
// / global_ratelimit / future ext_authz extensions).

// SetDownstreamRemoteAddr seeds the downstream remote-address field. Called
// by HCM dispatch with net.Conn.RemoteAddr() of the downstream conn.
func (c *FilterChain) SetDownstreamRemoteAddr(a net.Addr) {
	c.downstreamRemoteAddr = a
}

// SetDownstreamLocalAddr seeds the downstream local-address field. Called by
// HCM dispatch with net.Conn.LocalAddr() of the downstream conn.
func (c *FilterChain) SetDownstreamLocalAddr(a net.Addr) {
	c.downstreamLocalAddr = a
}

// SetDownstreamTLSServerName seeds the downstream TLS SNI field. Called by
// HCM dispatch with tls.ConnectionState.ServerName when the downstream conn
// is a *tls.Conn with HandshakeComplete; empty string passthrough otherwise.
func (c *FilterChain) SetDownstreamTLSServerName(s string) {
	c.downstreamTLSServerName = s
}

// SetDownstreamTLSPeerCertDER seeds the downstream client-cert leaf DER bytes
// field. Called by HCM dispatch with tls.ConnectionState.PeerCertificates[0].Raw
// when len(PeerCertificates) > 0 (mTLS); nil-passthrough otherwise. The
// callee retains a reference to the supplied slice — callers MUST treat the
// underlying bytes as immutable for the request lifetime.
func (c *FilterChain) SetDownstreamTLSPeerCertDER(b []byte) {
	c.downstreamTLSPeerCertDER = b
}

// SetDownstreamProtocol seeds the downstream-protocol canonical string field.
// HCM dispatch passes "HTTP/1.1" (H1 path) or "HTTP/2" (H2 path).
func (c *FilterChain) SetDownstreamProtocol(p string) {
	c.downstreamProtocol = p
}

// SetListenerPrincipal seeds the listener-principal string field. HCM
// dispatch passes the listener TLS-leaf-cert SAN[0]/CN extracted at listener-
// build time (plumbed through ListenerCtx → *Filter → here). The empty string
// passes through unchanged for plaintext listeners.
func (c *FilterChain) SetListenerPrincipal(p string) {
	c.listenerPrincipal = p
}

// SetRouteRateLimits seeds the matched route's raw []*routev3.RateLimit policy
// slice. Called by HCM dispatch (connection.go H1 / h2dispatch.go H2) at chain
// build time BEFORE RunDecodeHeaders when the matched routeEntry carries a
// non-empty rateLimits slice. nil-passthrough for routes with no rate_limits[]
// (the zero-regression path) — the chain field stays nil, the accessor
// returns nil. Per ADR-0198 §Decision (phase-24.1 Task 5 / DELTA-2). Mirrors
// the ADR-0165 SetDownstreamRemoteAddr set-once-by-dispatch discipline; the
// single-dispatch-goroutine invariant per ADR-0071 applies.
func (c *FilterChain) SetRouteRateLimits(rls []*routev3.RateLimit) {
	c.routeRateLimits = rls
}

// SetVirtualHostRateLimits seeds the matched route's parent virtual_host's
// raw []*routev3.RateLimit policy slice. Same seeding discipline as
// SetRouteRateLimits — called by HCM dispatch at chain build time BEFORE
// RunDecodeHeaders when the parent vhost carries a non-empty rate_limits
// slice. nil-passthrough for vhosts with no rate_limits[]. Per ADR-0198
// §Decision.
func (c *FilterChain) SetVirtualHostRateLimits(rls []*routev3.RateLimit) {
	c.virtualHostRateLimits = rls
}

// RouteRateLimits returns the chain's seeded matched-route raw
// []*routev3.RateLimit policy slice. Test-readable companion to the
// per-filter decoderCB.RouteRateLimits accessor — production filters read via
// the callback surface; tests use this chain-level accessor when the filter-
// instance machinery is not in play (e.g., assertions on chain state after a
// dispatch-time seed). Per ADR-0198 §Decision.
func (c *FilterChain) RouteRateLimits() []*routev3.RateLimit {
	return c.routeRateLimits
}

// VirtualHostRateLimits is the chain-level test-readable companion to
// decoderCB.VirtualHostRateLimits. Per ADR-0198 §Decision.
func (c *FilterChain) VirtualHostRateLimits() []*routev3.RateLimit {
	return c.virtualHostRateLimits
}

// SetRouteMetadata seeds the matched route's RAW *corev3.Metadata. Called by
// HCM dispatch (connection.go H1 / h2dispatch.go H2) at chain build time
// BEFORE RunDecodeHeaders when the matched routeEntry carries a non-nil
// metadata. nil-passthrough when the route has no metadata (the chain field
// stays nil; the accessor returns nil). Per phase 24.2 Task 1 (D-RL8) —
// extension of the ADR-0165 set-once-by-dispatch + ADR-0198 DELTA-2 plumbing
// template; the single-dispatch-goroutine invariant per ADR-0071 applies.
func (c *FilterChain) SetRouteMetadata(md *corev3.Metadata) {
	c.routeMetadata = md
}

// RouteMetadata is the chain-level test-readable companion to
// decoderCB.RouteMetadata. Per phase 24.2 Task 1 (D-RL8).
func (c *FilterChain) RouteMetadata() *corev3.Metadata {
	return c.routeMetadata
}

// SetRouteIncludeVhRateLimits seeds the matched route's legacy
// `RouteAction.include_vh_rate_limits` bool. Called by HCM dispatch
// (connection.go H1 / h2dispatch.go H2) at chain build time BEFORE
// RunDecodeHeaders when the matched routeEntry carries the legacy bool.
// false-passthrough when the route has no include_vh_rate_limits (the chain
// field stays false; the accessor returns false). Per phase 24.2 Task 4
// (D-RL11) — extension of the ADR-0165 set-once-by-dispatch + ADR-0198
// DELTA-2 plumbing template; the single-dispatch-goroutine invariant per
// ADR-0071 applies.
func (c *FilterChain) SetRouteIncludeVhRateLimits(v bool) {
	c.routeIncludeVhRateLimits = v
}

// RouteIncludeVhRateLimits is the chain-level test-readable companion to
// decoderCB.RouteIncludeVhRateLimits. Per phase 24.2 Task 4 (D-RL11).
func (c *FilterChain) RouteIncludeVhRateLimits() bool {
	return c.routeIncludeVhRateLimits
}

// SetEncodeResponseStatus seeds the chain's per-stream HTTP response status
// code field. Called by HCM dispatch (connection.go H1 / h2dispatch.go H2 from
// resp.Status, and the beginLocalReply path from the local-reply status) ONCE
// BEFORE RunEncodeHeaders dispatch; read by per-stream encoderCB.ResponseStatus()
// callbacks during encode-filter iteration. Mirrors the SetDownstreamProtocol
// set-once discipline (ADR-0165): single dispatch-goroutine invariant per
// ADR-0071, no synchronization required. Added per ADR-0196.
func (c *FilterChain) SetEncodeResponseStatus(status int) {
	c.encodeResponseStatus = status
}

// SetDiagLogWriter overrides the destination for framework diagnostic log
// lines. Test-only helper: production callers leave this unset (the default
// destination is os.Stderr). The TestChain_SendLocalReply_SecondCallAfterEncodeStartedLogs
// test uses this to capture the "second SendLocalReply ignored" log line.
//
// Codified deviation from PLAN.md scaffold (which framed SetDiagLogWriter as
// "not in this task") — Task 7's last test asserts the log line, so the
// helper must land here. Mirrors the phase-04..06.2 PLAN-deviation precedent.
func (c *FilterChain) SetDiagLogWriter(w io.Writer) { c.diagLogW = w }

// diagLogWriter returns the destination for log messages. Default is stderr;
// tests override via SetDiagLogWriter.
func (c *FilterChain) diagLogWriter() io.Writer {
	if c.diagLogW != nil {
		return c.diagLogW
	}
	return os.Stderr
}

// beginLocalReply synthesizes a response from a filter's SendLocalReply call.
// Per ADR-0075 + SPEC §11 #4 empirical pin:
//   - enters the encode chain at filter[len-1] (NOT at the calling filter's
//     index, NOT at index 0);
//   - runs the FULL encode chain in reverse declaration order, INCLUDING the
//     calling filter's own encode side;
//   - first-call-wins via sync.Once;
//   - second-call-after-encode-started is a no-op + diagnostic log.
//
// Date and Server response headers are NOT set here — those are filled by the
// HCM wire-write path (per ADR-0075 (b)). The framework injects only the
// minimal Content-Length + default Content-Type headers needed for a valid
// HTTP/1.x response shape.
func (c *FilterChain) beginLocalReply(ctx context.Context, callerIdx int, status int, body string, headers OrderedHeaders) {
	if c.encodeStarted.Load() {
		// Encode chain already started; second SendLocalReply is a no-op + log.
		_, _ = fmt.Fprintf(c.diagLogWriter(), "hcm: filter %q called SendLocalReply after encode-side started; ignoring\n", c.filters[callerIdx].Name)
		return
	}
	c.localReplyOnce.Do(func() {
		c.localReplyDone.Store(true)
		// Build an http.Header view of the ordered carrier so encode-side
		// filters (which still operate on the http.Header API per ADR-0071)
		// can mutate values via Set/Add. Mutations are reconciled back onto
		// the OrderedHeaders carrier after RunEncodeHeaders returns to
		// preserve caller-supplied wire-emission order (per Task 18 review:
		// SPEC §11.2 verbatim 6-header order must survive on the wire).
		//
		// Use http.Header.Add (which canonicalizes via textproto.CanonicalMIMEHeaderKey)
		// rather than a raw map copy — a user-supplied non-canonical key like
		// "content-type" would otherwise survive verbatim and the subsequent
		// merged.Get("Content-Type") miss would cause the framework to inject
		// a duplicate default Content-Type pair on the wire. Per code-quality
		// review on a03a1d3 (I-1).
		merged := make(http.Header, len(headers)+4)
		for _, hf := range headers {
			merged.Add(hf.Name, hf.Value)
		}
		merged.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		if merged.Get("Content-Type") == "" {
			merged.Set("Content-Type", "text/plain")
		}
		// The status int is consumed by the HCM wire-write layer (Task 13) on
		// the response status-line; the framework's beginLocalReply does not
		// emit a status code itself, only the response headers + body.
		// Seed the encode-side response-status accessor (ADR-0196) from the
		// local-reply status so encode-side classifying filters (e.g.
		// admission_control) observe the same status the wire will carry —
		// e.g. a 503 SendLocalReply from another filter classifies as failure.
		c.SetEncodeResponseStatus(status)
		// Run the encode chain. Errors here propagate to logs only — at this
		// point the request has already reached SendLocalReply and there is
		// no upstream to report failure to.
		_, _ = c.RunEncodeHeaders(ctx, merged, len(body) == 0)
		if len(body) > 0 {
			_, _ = c.RunEncodeData(ctx, []byte(body), true)
		}
		// no trailers

		// Phase 07.1 Task 18 prereq P2 + Task 18 review fix: surface the
		// synthesized response (post-encode-chain mutation) to HCM dispatch
		// so the wire-write path can emit the bytes IN CALLER-SUPPLIED ORDER.
		// Reconcile post-encode mutations back onto the OrderedHeaders carrier:
		//   1. Walk the original ordered list; for each name take the (possibly-
		//      mutated) value from `merged` (encode-side filters mutate via Set,
		//      preserving order at the framework level for known names).
		//   2. Append any net-new keys from `merged` that were not in the
		//      original list (framework-injected Content-Length / Content-Type,
		//      plus any encode-side filter Add()s for new header names).
		// This preserves the §11.2 6-header verbatim order from cors.go while
		// honoring encode-side mutations + framework defaults.
		c.localReplyStatus = status
		c.localReplyHeaders = reconcileOrderedHeaders(headers, merged)
		c.localReplyBody = []byte(body)
	})
}

// reconcileOrderedHeaders is a thin wrapper around the package-level
// ReconcileOrderedHeaders helper. Phase 07.1 Task 19 (I-3 prereq) exported
// the implementation so HCM dispatch's action-driven path can use the same
// machinery; this wrapper preserves the call site's lowercase form for chain
// internals.
func reconcileOrderedHeaders(original OrderedHeaders, merged http.Header) OrderedHeaders {
	return ReconcileOrderedHeaders(original, merged)
}

// LocalReplyDone reports whether SendLocalReply was invoked on this chain.
// HCM dispatch reads this post-RunDecodeHeaders to discriminate the local-
// reply path (chain owns the response shape) from the action-driven path
// (router filter owns the response shape).
func (c *FilterChain) LocalReplyDone() bool { return c.localReplyDone.Load() }

// LocalReplyResponse returns the synthesized local-reply response (status +
// headers + body) for the wire-write path. Only meaningful when
// LocalReplyDone() returns true. Phase 07.1 Task 18 prereq P2: the chain's
// beginLocalReply ran the encode chain over (status, headers, body); this
// getter surfaces the (post-mutation) shape so HCM dispatch can write wire
// bytes via writeH1Reply / writeH2Reply.
//
// Per Task 18 review (SPEC §11.2 ordered-headers compliance): headers is the
// ordered carrier that preserves caller-supplied insertion order through the
// chain's encode iteration. writeH1Reply / writeH2Reply iterate this slice
// in order rather than walking an http.Header map (which would lose order).
func (c *FilterChain) LocalReplyResponse() (int, OrderedHeaders, []byte) {
	return c.localReplyStatus, c.localReplyHeaders, c.localReplyBody
}
