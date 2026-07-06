package hcm

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/pgdad/envoy-go/internal/accesslog"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/stats"
)

// orderRecordingFilter is a decode-only test filter that appends its name to
// a shared []string when DecodeHeaders fires, then returns Continue. Used by
// TestDispatchRequest_ChainInvocationOrder to assert the chain's decode-side
// iteration order.
//
// Decode-only is sufficient: the M-9 sentinel test asserts the order of the
// decode-side callbacks ahead of the router; the encode side is dormant on
// the H1 direct_response path post-Task-15 (PLAN deviation (vi); see
// PROGRESS Task 15). Encode-side coverage will land at Task 18 (cors).
type orderRecordingFilter struct {
	name  string
	order *[]string
	mu    *sync.Mutex
}

func (f *orderRecordingFilter) DecodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	f.mu.Lock()
	*f.order = append(*f.order, f.name)
	f.mu.Unlock()
	return filter_http.Continue
}
func (f *orderRecordingFilter) DecodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *orderRecordingFilter) DecodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *orderRecordingFilter) SetDecoderCallbacks(filter_http.DecoderFilterCallbacks) {}
func (f *orderRecordingFilter) OnDestroy()                                             {}

// orderRecordingEncoder is a no-op encoder side that satisfies the
// StreamEncoderFilter interface so the filter can be paired with the decoder
// via HTTPFilter{Decoder: f, Encoder: f}. The chain framework requires both
// sides on filters that participate in the both-sides envelope; for a decode-
// only assertion test we provide trivial pass-throughs on the encode side.
func (f *orderRecordingFilter) EncodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	return filter_http.Continue
}
func (f *orderRecordingFilter) EncodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *orderRecordingFilter) EncodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *orderRecordingFilter) SetEncoderCallbacks(filter_http.EncoderFilterCallbacks) {}

// TestDispatchRequest_ChainInvocationOrder is the M-9 sentinel test (Task 15
// review-loop). It proves the chain's decode-side iteration order: filters
// run in declaration order ahead of the terminal router. The chain config is
// [a, b, router] with a direct_response route so no upstream is needed; the
// shared order slice captures each filter's DecodeHeaders firing.
//
// Asserted: order == ["a", "b"]. The terminal router filter does NOT record
// because its DecodeHeaders is package-private internal (it returns Continue
// pass-through; the action driving runs from RunAction, not DecodeHeaders).
//
// This regression test guards against future refactors that might invert
// chain iteration, skip filters, or mis-route the chain build into a
// non-terminal-router shape.
func TestDispatchRequest_ChainInvocationOrder(t *testing.T) {
	order := make([]string, 0, 2)
	var orderMu sync.Mutex

	mkFilterFactory := func(name string) filter_http.FilterInstanceFactory {
		return func() filter_http.HTTPFilter {
			f := &orderRecordingFilter{name: name, order: &order, mu: &orderMu}
			return filter_http.HTTPFilter{
				Name:    name,
				Decoder: f,
				Encoder: f,
			}
		}
	}

	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}

	r := stats.NewRegistry()
	prefix := "http.test_chain_order."
	f := &Filter{
		table:             tt,
		statPrefix:        "test_chain_order",
		downstreamRqTotal: r.NewCounter(prefix + "downstream_rq_total"),
		downstreamRq2xx:   r.NewCounter(prefix + "downstream_rq_2xx"),
		downstreamRq3xx:   r.NewCounter(prefix + "downstream_rq_3xx"),
		downstreamRq4xx:   r.NewCounter(prefix + "downstream_rq_4xx"),
		downstreamRq5xx:   r.NewCounter(prefix + "downstream_rq_5xx"),
		// Chain layout: two recording filters ahead of the terminal router.
		// chainConfig validation enforces "non-empty; last is router"; the
		// router factory is the chain's terminal entry.
		chainConfig: []chainEntry{
			{name: "a", factory: mkFilterFactory("a")},
			{name: "b", factory: mkFilterFactory("b")},
			{name: "envoy.filters.http.router", factory: rfFactory},
		},
		accessLog: []accesslog.Sink{},
	}

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	// Phase 16 Task 6 (ADR-0144): dispatchRequest now takes a downstream
	// net.Conn so the H1 path can extract TLS principal candidates. The
	// test passes nil — TLS extraction returns nil for non-*tls.Conn types
	// (the type assertion fails) so the chain's tlsPrincipals stays nil.
	status, derr := f.dispatchRequest(context.Background(), nil, req, bw)
	if derr != nil {
		t.Fatalf("dispatchRequest: %v", derr)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}

	// Assert decode-side iteration order: a runs before b; both run before
	// the terminal router action. Router does NOT record (its DecodeHeaders
	// is a Continue pass-through; the action drive happens in RunAction).
	orderMu.Lock()
	got := append([]string(nil), order...)
	orderMu.Unlock()

	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("expected order length %d (%v); got %d (%v)", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decode iteration order: got %v, want %v", got, want)
		}
	}
}
