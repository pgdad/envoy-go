package http

import (
	"context"
	"net"
	"net/http"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type fakeDecoderCB struct {
	continueCalls int
	localReplies  int
	routeCfgCalls int
}

func (c *fakeDecoderCB) ContinueDecoding()                          { c.continueCalls++ }
func (c *fakeDecoderCB) SendLocalReply(int, string, OrderedHeaders) { c.localReplies++ }
func (c *fakeDecoderCB) RequestRouteConfig() proto.Message          { c.routeCfgCalls++; return nil }
func (c *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (c *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (c *fakeDecoderCB) EncodeTrailers(http.Header)      {}
func (c *fakeDecoderCB) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4). Zero-value
// returns keep the existing TestDecoderFilterCallbacks_Compile compile-time
// conformance assertion green.
func (c *fakeDecoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *fakeDecoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *fakeDecoderCB) DownstreamTLSServerName() string  { return "" }
func (c *fakeDecoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *fakeDecoderCB) DownstreamProtocol() string       { return "" }
func (c *fakeDecoderCB) ListenerPrincipal() string        { return "" }

func TestDecoderFilterCallbacks_Compile(t *testing.T) {
	var _ DecoderFilterCallbacks = (*fakeDecoderCB)(nil)
}

type fakeEncoderCB struct{}

func (c *fakeEncoderCB) ContinueEncoding()               {}
func (c *fakeEncoderCB) EncodeHeaders(http.Header, bool) {}
func (c *fakeEncoderCB) EncodeData([]byte, bool)         {}
func (c *fakeEncoderCB) EncodeTrailers(http.Header)      {}
func (c *fakeEncoderCB) OverwriteBody([]byte)            {}

// ADR-0174 callback-surface extension stubs (phase-19.1 Task 5 — the
// symmetric encoder-side mirror of ADR-0165's 6 decoder-side accessors).
// Zero-value returns keep the existing TestEncoderFilterCallbacks_Compile
// compile-time conformance assertion green.
func (c *fakeEncoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *fakeEncoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (c *fakeEncoderCB) DownstreamTLSServerName() string  { return "" }
func (c *fakeEncoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *fakeEncoderCB) DownstreamProtocol() string       { return "" }
func (c *fakeEncoderCB) ListenerPrincipal() string        { return "" }

// ADR-0175 callback-surface extension stub (phase-19.2 Task 2 — encode-side
// body-buffering framework primitive). Zero-value return preserves the
// existing TestEncoderFilterCallbacks_Compile compile-time conformance
// assertion green.
func (c *fakeEncoderCB) BufferEncodedBody() []byte { return nil }

func TestEncoderFilterCallbacks_Compile(t *testing.T) {
	var _ EncoderFilterCallbacks = (*fakeEncoderCB)(nil)
}

func TestDecoderCB_RequestRouteConfigsAllTiers(t *testing.T) {
	chainNames := []string{"envoy.filters.http.header_mutation"}
	rcCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("rc"))}
	vhCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("vh"))}
	rtCfg := map[string]*anypb.Any{"envoy.filters.http.header_mutation": mustAny(t, wrapperspb.String("route"))}
	pr, err := BuildPerRouteConfig(rcCfg, []routeScope{{VHost: vhCfg, Route: rtCfg}}, chainNames, nil)
	if err != nil {
		t.Fatalf("BuildPerRouteConfig: %v", err)
	}
	// Build a chain with a single filter named header_mutation.
	fakeFilter := &fakeBothSidesFilter{}
	chain := NewFilterChain([]HTTPFilter{{Name: "envoy.filters.http.header_mutation", Decoder: fakeFilter, Encoder: fakeFilter}}, pr)
	chain.SetRequestCtx(context.Background(), 0)

	// Reach into the chain to grab the decoderCB the framework wired into the filter.
	cb := fakeFilter.dcb
	if cb == nil {
		t.Fatal("fakeFilter.dcb not set")
	}
	route, vhost, rc := cb.RequestRouteConfigsAllTiers()
	if r, ok := route.(*wrapperspb.StringValue); !ok || r.GetValue() != "route" {
		t.Errorf("route: got %v, want route", route)
	}
	if v, ok := vhost.(*wrapperspb.StringValue); !ok || v.GetValue() != "vh" {
		t.Errorf("vhost: got %v, want vh", vhost)
	}
	if c, ok := rc.(*wrapperspb.StringValue); !ok || c.GetValue() != "rc" {
		t.Errorf("rc: got %v, want rc", rc)
	}
}

func TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute(t *testing.T) {
	fakeFilter := &fakeBothSidesFilter{}
	chain := NewFilterChain([]HTTPFilter{{Name: "envoy.filters.http.header_mutation", Decoder: fakeFilter, Encoder: fakeFilter}}, nil)
	chain.SetRequestCtx(context.Background(), 0)
	cb := fakeFilter.dcb
	route, vhost, rc := cb.RequestRouteConfigsAllTiers()
	if route != nil || vhost != nil || rc != nil {
		t.Errorf("nil-perRoute should return all-nil; got route=%v vhost=%v rc=%v", route, vhost, rc)
	}
}

// fakeBothSidesFilter is a test helper implementing both StreamDecoderFilter and StreamEncoderFilter
// to capture the dcb wiring. (If a similar helper exists in the test package, reuse it.)
type fakeBothSidesFilter struct {
	dcb DecoderFilterCallbacks
	ecb EncoderFilterCallbacks
}

func (f *fakeBothSidesFilter) SetDecoderCallbacks(cb DecoderFilterCallbacks)       { f.dcb = cb }
func (f *fakeBothSidesFilter) SetEncoderCallbacks(cb EncoderFilterCallbacks)       { f.ecb = cb }
func (f *fakeBothSidesFilter) DecodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (f *fakeBothSidesFilter) EncodeHeaders(http.Header, bool) FilterHeadersStatus { return Continue }
func (f *fakeBothSidesFilter) DecodeData([]byte, bool) FilterDataStatus            { return DataContinue }
func (f *fakeBothSidesFilter) EncodeData([]byte, bool) FilterDataStatus            { return DataContinue }
func (f *fakeBothSidesFilter) DecodeTrailers(http.Header) FilterTrailersStatus {
	return TrailersContinue
}
func (f *fakeBothSidesFilter) EncodeTrailers(http.Header) FilterTrailersStatus {
	return TrailersContinue
}
func (f *fakeBothSidesFilter) OnDestroy() {}
