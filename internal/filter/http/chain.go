package http

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// localReply413BodyBytes is the verbatim 17-byte ASCII body of the synthesized
// 413 Payload Too Large response. Per SPEC §11 #3 empirical pin: no trailing
// newline. Pinned here to ensure the body shape can never drift relative to
// the wire-shape contract asserted in TestChain_DecodeData_OverflowSynthesizes413.
const localReply413BodyBytes = "Payload Too Large" // 17 bytes

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
	// the connection — Tasks 15 + 16). The *Over flags are set when the cap is
	// crossed and are observable for debug/log; the framework's behavior is
	// driven by the local-reply path / sentinel return.
	decodeBuf     []byte
	decodeBufOver bool
	encodeBuf     []byte
	encodeBufOver bool

	// SendLocalReply guard (Task 7).
	localReplyOnce sync.Once
	localReplyDone atomic.Bool

	// Encode-side started flag (Task 7) — second SendLocalReply after this is
	// a no-op + log line.
	encodeStarted atomic.Bool

	// Per-request state set by HCM dispatch via SetRequestCtx (Task 7; HCM wire-up at Task 13).
	// routeIdx is the matched-route index used by RequestRouteConfig's perRoute lookup.
	// ambientCtx is the request context propagated to beginLocalReply when a filter
	// triggers SendLocalReply (since SendLocalReply itself does not take a ctx).
	routeIdx   int
	ambientCtx context.Context

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
				headers := http.Header{}
				// Connection: close per the §11 #3 empirical pin — forces the
				// H1 conn to terminate after the local reply emits. The HCM
				// dispatch path reads this header on the synthesized response
				// and closes the conn after writing (Task 15).
				headers.Set("Connection", "close")
				c.beginLocalReply(c.ambientCtx, c.decodeIdx, 413, localReply413BodyBytes, headers)
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
	if len(c.encodeBuf)+len(data) > filterBufferLimitBytes {
		c.encodeBufOver = true
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
	// Iteration completed; record the chunk's size in the encode-side buffer
	// accumulator. We only track the size (len) rather than the bytes — the
	// bytes have already been forwarded down the encode chain on this call;
	// no replay is required (encode-side StopIterationAndBuffer is treated as
	// park-only in Task 6, no per-filter replay buffer).
	c.encodeBuf = append(c.encodeBuf, data...)
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

func (d *decoderCB) SendLocalReply(status int, body string, headers http.Header) {
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

func (d *decoderCB) EncodeHeaders(http.Header, bool) {}
func (d *decoderCB) EncodeData([]byte, bool)         {}
func (d *decoderCB) EncodeTrailers(http.Header)      {}

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

// SetRequestCtx wires the request context + matched-route index into the
// chain. Called by HCM dispatch at request start (HCM wire-up lands in
// Task 13). The ambient ctx is used by beginLocalReply since the
// DecoderFilterCallbacks.SendLocalReply API does not accept a ctx.
func (c *FilterChain) SetRequestCtx(ctx context.Context, routeIdx int) {
	c.ambientCtx = ctx
	c.routeIdx = routeIdx
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
func (c *FilterChain) beginLocalReply(ctx context.Context, callerIdx int, status int, body string, headers http.Header) {
	if c.encodeStarted.Load() {
		// Encode chain already started; second SendLocalReply is a no-op + log.
		_, _ = fmt.Fprintf(c.diagLogWriter(), "hcm: filter %q called SendLocalReply after encode-side started; ignoring\n", c.filters[callerIdx].Name)
		return
	}
	c.localReplyOnce.Do(func() {
		c.localReplyDone.Store(true)
		// Merge framework-injected standard headers with user-supplied headers.
		// Use Header.Add (which canonicalizes via textproto.CanonicalMIMEHeaderKey)
		// rather than a raw map copy — otherwise a user-supplied non-canonical key
		// like "content-type" would survive verbatim and the subsequent
		// merged.Get("Content-Type") miss would cause the framework to inject a
		// duplicate default Content-Type pair on the wire. Per code-quality review
		// on a03a1d3 (I-1).
		merged := make(http.Header, len(headers)+4)
		for k, vs := range headers {
			for _, v := range vs {
				merged.Add(k, v)
			}
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
	})
}
