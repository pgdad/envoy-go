package http

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

// filterBufferLimitBytes is the per-stream body-buffer cap matching Envoy's
// default. Per ADR-0076 (lands in Task 9). Defined here at chain.go's head so
// later tasks can reference it without re-declaration.
const filterBufferLimitBytes = 1 << 20 // 1 MiB

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

	// Body buffers (decode-side; encode-side added in Task 6/Task 9 with the
	// 413/reset overflow paths).
	decodeBuf     []byte
	decodeBufOver bool

	// SendLocalReply guard (Task 7).
	localReplyOnce sync.Once
	localReplyDone atomic.Bool

	// Encode-side started flag (Task 7) — second SendLocalReply after this is
	// a no-op + log line.
	encodeStarted atomic.Bool

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
// Buffer overflow / 413-on-encode handling lands in Task 9.
func (c *FilterChain) RunEncodeData(ctx context.Context, data []byte, endStream bool) (bool, error) {
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
	// Implementation lands in Task 7 (beginLocalReply + first-call-wins).
	// Stubbed for Task 5 to avoid linker-error cascade; Task 7 replaces.
	_ = d.c.localReplyOnce
	d.c.localReplyDone.Store(true)
	_ = status
	_ = body
	_ = headers
}

// RequestRouteConfig returns the merged proto.Message for the calling filter's
// name. Returns nil until Task 7 wires in the perRoute lookup (the lookup
// needs the route-match index from the HCM dispatch path, which connects in
// Task 13). Note: PLAN scaffold used `any` as the return type but that does
// not satisfy DecoderFilterCallbacks (declared in callbacks.go) which requires
// proto.Message — corrected here.
func (d *decoderCB) RequestRouteConfig() proto.Message { return nil } // wired to perroute lookup in Task 7

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
