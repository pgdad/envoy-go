package tap

import (
	"net"
	"net/http"
	"path/filepath"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

type stubDecCB struct {
	envoyhttp.DecoderFilterCallbacks
	local, remote net.Addr
}

func (s stubDecCB) DownstreamLocalAddr() net.Addr  { return s.local }
func (s stubDecCB) DownstreamRemoteAddr() net.Addr { return s.remote }

func tcp(t *testing.T, s string) net.Addr {
	t.Helper()
	a, err := net.ResolveTCPAddr("tcp", s)
	if err != nil {
		t.Fatalf("ResolveTCPAddr: %v", err)
	}
	return a
}

func globTraces(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "out_*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return m
}

// driveStream runs one request/response pair through f and tears it down.
func driveStream(f *tapFilter, xtap string, status int) {
	f.SetEncoderCallbacks(stubEncCB{status: status})
	f.DecodeHeaders(http.Header{"X-Tap": {xtap}, ":method": {"GET"}}, true)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)
	f.OnDestroy()
}

func TestOnDestroy_EmitsOneTraceOnMatch(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, err := New(mustAny(t, validTap(dir)), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	driveStream(hf.Decoder.(*tapFilter), "yes", 204)

	if got := len(globTraces(t, dir)); got != 1 {
		t.Errorf("trace files = %d, want 1", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 1 {
		t.Errorf("rq_tapped = %d, want 1", got)
	}
}

func TestOnDestroy_EmitsNothingOnNoMatch(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, _ := New(mustAny(t, validTap(dir)), ctx)
	driveStream(factory().Decoder.(*tapFilter), "no", 204)

	if got := len(globTraces(t, dir)); got != 0 {
		t.Errorf("trace files = %d, want 0 (no file on no-match)", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 0 {
		t.Errorf("rq_tapped = %d, want 0", got)
	}
}

// The trace is the WHOLE stream: request headers are present even though the
// predicate only names a request arm, and the response is captured too.
func TestBuildTrace_CarriesBothDirections_NoBody_EmptyTrailers(t *testing.T) {
	dir := t.TempDir()
	ctx, _ := newCtx()
	factory, _ := New(mustAny(t, validTap(dir)), ctx)
	f := factory().Decoder.(*tapFilter)
	f.SetEncoderCallbacks(stubEncCB{status: 204})
	f.DecodeHeaders(http.Header{"X-Tap": {"yes"}, ":method": {"GET"}}, true)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)

	bt := f.buildTrace().GetHttpBufferedTrace()
	if bt.GetRequest() == nil || len(bt.GetRequest().GetHeaders()) == 0 {
		t.Errorf("request must be populated")
	}
	if bt.GetResponse() == nil || len(bt.GetResponse().GetHeaders()) == 0 {
		t.Errorf("response must be populated")
	}
	if bt.GetRequest().GetBody() != nil || bt.GetResponse().GetBody() != nil {
		t.Errorf("body must NEVER be populated at 56.1")
	}
	if len(bt.GetRequest().GetTrailers()) != 0 || len(bt.GetResponse().GetTrailers()) != 0 {
		t.Errorf("trailers must NEVER be populated (the framework coverage boundary)")
	}
	if bt.GetDownstreamConnection() != nil {
		t.Errorf("downstream_connection must be absent when record_downstream_connection is unset")
	}
}

func TestBuildTrace_RecordDownstreamConnection(t *testing.T) {
	dir := t.TempDir()
	tp := validTap(dir)
	tp.RecordDownstreamConnection = true
	ctx, _ := newCtx()
	factory, err := New(mustAny(t, tp), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := factory().Decoder.(*tapFilter)
	f.SetDecoderCallbacks(stubDecCB{local: tcp(t, "10.0.0.1:10000"), remote: tcp(t, "10.0.0.2:38216")})
	f.SetEncoderCallbacks(stubEncCB{status: 204})
	f.DecodeHeaders(http.Header{"X-Tap": {"yes"}}, true)
	f.EncodeHeaders(http.Header{}, true)

	conn := f.buildTrace().GetHttpBufferedTrace().GetDownstreamConnection()
	if conn == nil {
		t.Fatalf("downstream_connection must be populated when the flag is set")
	}
	if got := conn.GetLocalAddress().GetSocketAddress().GetAddress(); got != "10.0.0.1" {
		t.Errorf("local address = %q, want 10.0.0.1", got)
	}
	if got := conn.GetLocalAddress().GetSocketAddress().GetPortValue(); got != 10000 {
		t.Errorf("local port = %d, want 10000", got)
	}
	if got := conn.GetRemoteAddress().GetSocketAddress().GetAddress(); got != "10.0.0.2" {
		t.Errorf("remote address = %q, want 10.0.0.2", got)
	}
}

// D-TAP-EMITSITE (i): both HTTPFilter fields must hold the SAME pointer.
func TestFactory_InstallsOneSharedValueInBothFields(t *testing.T) {
	ctx, _ := newCtx()
	factory, _ := New(mustAny(t, validTap(t.TempDir())), ctx)
	hf := factory()
	d, okD := hf.Decoder.(*tapFilter)
	e, okE := hf.Encoder.(*tapFilter)
	if !okD || !okE {
		t.Fatalf("Decoder=%T Encoder=%T, want both *tapFilter", hf.Decoder, hf.Encoder)
	}
	if d != e {
		t.Errorf("Decoder and Encoder must be the SAME *tapFilter value; " +
			"a two-value split makes the encoder OnDestroy unreachable (chain.go:670)")
	}
}

// D-TAP-EMITSITE (ii): driving a real FilterChain to Destroy() emits once.
//
// The match predicate here is validTapReqAndResp: an and_match requiring BOTH
// a request-header match (x-tap: yes) AND a response-header match
// (:status: 204). This is deliberate — a predicate that resolves on the
// request alone would still pass under a two-value decoder/encoder split,
// since the decoder-side value sees the request headers and nothing else is
// needed to satisfy it. Requiring both arms means only a value that observes
// BOTH DecodeHeaders and EncodeHeaders can resolve the match True.
//
// CRITICAL: this test drives the INTERFACE VALUES hf.Decoder / hf.Encoder — it
// must NEVER downcast hf.Decoder to *tapFilter and poke that. With a downcast,
// a two-value split (Decoder: &tapFilter{}, Encoder: f) would still feed BOTH
// header sets into whatever hf.Decoder happens to be, the emit would fire, and
// the test would pass — proving nothing. Driving through the two interfaces is
// exactly what makes the split break bite: the decoder-only value never sees
// the response headers, so its response arm stays Undetermined, the and_match
// stays Undetermined, Resolve() is false, and Destroy() (which reaches only
// the Decoder branch) emits nothing.
func TestChainDestroy_EmitsExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, _ := New(mustAny(t, validTapReqAndResp(dir)), ctx)
	hf := factory()

	hf.Encoder.SetEncoderCallbacks(stubEncCB{status: 204})
	hf.Decoder.DecodeHeaders(http.Header{"X-Tap": {"yes"}}, true)
	hf.Encoder.EncodeHeaders(http.Header{}, true)

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{hf}, nil)
	chain.Destroy()
	chain.Destroy() // idempotent (destroyOnce)

	if got := len(globTraces(t, dir)); got != 1 {
		t.Errorf("trace files = %d, want exactly 1", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 1 {
		t.Errorf("rq_tapped = %d, want 1", got)
	}
}

func counterValue(t *testing.T, reg *stats.Registry, name string) uint64 {
	t.Helper()
	var v uint64
	found := false
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				v, found = c.Load(), true
			}
		}
	})
	if !found {
		t.Fatalf("counter %s not registered", name)
	}
	return v
}
