package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
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
	// driven by the local-reply path / sentinel return. Encode-side tracks only
	// the running byte count (encodeBufLen) because the chunk has already been
	// forwarded down the encode chain — no replay buffer is needed.
	decodeBuf     []byte
	decodeBufOver bool
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
func (c *FilterChain) RunEncodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
	// Buffer-cap check up front: the sentinel is returned BEFORE iterating any
	// filter, so the connection-reset wire path never observes a partially-
	// emitted overflowing chunk. Per ADR-0076 + SPEC §15 acceptance bullet 2.
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
		case DataStopIterationAndBuffer, DataStopIterationNoBuffer:
			if err := c.parkEncode(ctx); err != nil {
				return false, err
			}
			c.encodeIdx--
		default:
			return false, fmt.Errorf("chain: filter %q returned unknown FilterDataStatus %d on encode", c.filters[c.encodeIdx].Name, status)
		}
	}
	// Iteration completed; record the chunk's size for the encode-side cap.
	// Only the running total is tracked — the bytes have already been
	// forwarded down the encode chain on this call, so no replay buffer is
	// needed (encode-side StopIterationAndBuffer is park-only per Task 6).
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
func (c *FilterChain) Destroy() {
	c.destroyOnce.Do(func() {
		for _, f := range c.filters {
			if f.Decoder != nil {
				f.Decoder.OnDestroy()
			} else if f.Encoder != nil {
				f.Encoder.OnDestroy()
			}
		}
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
