package localratelimit

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// mustAny packages a proto.Message into an *anypb.Any with the local_ratelimit TypeURL.
func mustAny(t *testing.T, msg *localratelimitv3.LocalRateLimit) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// happyConfig returns a minimum-viable LocalRateLimit proto for happy-path
// tests: stat_prefix + token_bucket{max_tokens=10, fill_interval=1s}.
func happyConfig() *localratelimitv3.LocalRateLimit {
	return &localratelimitv3.LocalRateLimit{
		StatPrefix: "test",
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    10,
			FillInterval: durationpb.New(1 * time.Second),
			// TokensPerFill omitted → defaults to 1 per §11.2b-i.
		},
	}
}

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("New(nil, _): want error, got nil")
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("New(nil, _): got error %q, want containing 'typed_config required'", err.Error())
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(bad, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("New(malformed, _): want error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("New(malformed, _): got error %q, want containing 'unmarshal'", err.Error())
	}
}

func TestNew_StatPrefixEmpty(t *testing.T) {
	cfg := happyConfig()
	cfg.StatPrefix = ""
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "stat_prefix required") {
		t.Errorf("New(stat_prefix=\"\", _): got %v, want error containing 'stat_prefix required'", err)
	}
}

func TestNew_MaxTokensZero(t *testing.T) {
	cfg := happyConfig()
	cfg.TokenBucket.MaxTokens = 0
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "max_tokens must be > 0") {
		t.Errorf("New(max_tokens=0, _): got %v, want error containing 'max_tokens must be > 0'", err)
	}
}

func TestNew_TokenBucketAbsent(t *testing.T) {
	cfg := happyConfig()
	cfg.TokenBucket = nil
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "token_bucket required") {
		t.Errorf("New(token_bucket=nil, _): got %v, want error containing 'token_bucket required'", err)
	}
}

func TestNew_TokensPerFillExplicitZero(t *testing.T) {
	cfg := happyConfig()
	cfg.TokenBucket.TokensPerFill = wrapperspb.UInt32(0)
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "tokens_per_fill must be > 0") {
		t.Errorf("New(tokens_per_fill=0, _): got %v, want error containing 'tokens_per_fill must be > 0'", err)
	}
}

func TestNew_TokensPerFillOmittedDefaultsToOne(t *testing.T) {
	cfg := happyConfig()
	cfg.TokenBucket.TokensPerFill = nil
	factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
	if err != nil {
		t.Fatalf("New(tokens_per_fill=absent, _): want success, got %v", err)
	}
	if factory == nil {
		t.Fatalf("New: returned nil factory")
	}
	// Verify happy-path acceptance; bucket primitive tested directly in bucket_test.go.
}

func TestNew_FillIntervalBelow50ms(t *testing.T) {
	for _, dt := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 49 * time.Millisecond} {
		t.Run(fmt.Sprintf("%dms", dt/time.Millisecond), func(t *testing.T) {
			cfg := happyConfig()
			cfg.TokenBucket.FillInterval = durationpb.New(dt)
			_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
			if err == nil {
				t.Fatalf("New(fill_interval=%v, _): want error, got nil", dt)
			}
			// Verbatim Envoy v1.37.2 error string per SPEC §11.2c + ADR-0115.
			wantString := "local rate limit token bucket fill timer must be >= 50ms"
			if err.Error() != wantString {
				t.Errorf("New(fill_interval=%v, _): got %q, want %q (verbatim Envoy)", dt, err.Error(), wantString)
			}
		})
	}
}

func TestNew_FillIntervalAtOrAbove50ms(t *testing.T) {
	for _, dt := range []time.Duration{50 * time.Millisecond, 51 * time.Millisecond, 100 * time.Millisecond, 1 * time.Second} {
		t.Run(fmt.Sprintf("%v", dt), func(t *testing.T) {
			cfg := happyConfig()
			cfg.TokenBucket.FillInterval = durationpb.New(dt)
			_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
			if err != nil {
				t.Fatalf("New(fill_interval=%v, _): want success, got %v", dt, err)
			}
		})
	}
}

func TestNew_StatusCodeBelow400(t *testing.T) {
	cfg := happyConfig()
	cfg.Status = &typev3.HttpStatus{Code: typev3.StatusCode(399)}
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
	if err == nil || !strings.Contains(err.Error(), "[400, 600)") {
		t.Errorf("New(status.code=399, _): got %v, want error containing '[400, 600)'", err)
	}
}

func TestNew_StatusCodeAtOrAbove600(t *testing.T) {
	for _, code := range []int{600, 700, 999} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			cfg := happyConfig()
			cfg.Status = &typev3.HttpStatus{Code: typev3.StatusCode(code)}
			_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{})
			if err == nil {
				t.Fatalf("New(status.code=%d, _): want error", code)
			}
		})
	}
}

func TestNew_StatusCodeOmittedDefaultsTo429(t *testing.T) {
	cfg := happyConfig()
	cfg.Status = nil // explicitly omit
	factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
	if err != nil {
		t.Fatalf("New(status omitted, _): want success, got %v", err)
	}
	inst := factory().Decoder.(*filter)
	if inst.state.listenerRC.statusCode != 429 {
		t.Errorf("statusCode default: got %d, want 429", inst.state.listenerRC.statusCode)
	}
}

func TestNew_HappyPath_AllConsumedFields(t *testing.T) {
	cfg := happyConfig()
	cfg.StatPrefix = "myprefix"
	cfg.TokenBucket.MaxTokens = 5
	cfg.TokenBucket.TokensPerFill = wrapperspb.UInt32(2)
	cfg.TokenBucket.FillInterval = durationpb.New(500 * time.Millisecond)
	cfg.Status = &typev3.HttpStatus{Code: typev3.StatusCode(503)}
	factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
	if err != nil {
		t.Fatalf("New(happy, _): want success, got %v", err)
	}
	inst := factory().Decoder.(*filter)
	if inst.state.listenerRC.statPrefix != "myprefix" {
		t.Errorf("statPrefix: got %q, want %q", inst.state.listenerRC.statPrefix, "myprefix")
	}
	if inst.state.listenerRC.statusCode != 503 {
		t.Errorf("statusCode: got %d, want 503", inst.state.listenerRC.statusCode)
	}
	if inst.state.listenerRC.body != "local_rate_limited" {
		t.Errorf("body: got %q, want %q", inst.state.listenerRC.body, "local_rate_limited")
	}
	if inst.state.listenerRC.bucket == nil {
		t.Errorf("bucket: got nil, want non-nil")
	}
	if inst.state.listenerRC.stats == nil {
		t.Errorf("stats: got nil, want non-nil (ctx.Stats provided)")
	}
}

// fakeDecoderCB is a test implementation of envoyhttp.DecoderFilterCallbacks that
// captures SendLocalReply arguments. The other methods are no-ops; unused proto
// methods satisfy the interface.
type fakeDecoderCB struct {
	sendCalled bool
	sendStatus int
	sendBody   string
	sendHdrs   envoyhttp.OrderedHeaders
	routeCfg   proto.Message // returned by RequestRouteConfig; nil by default
}

func (f *fakeDecoderCB) ContinueDecoding() {}
func (f *fakeDecoderCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	f.sendCalled = true
	f.sendStatus = status
	f.sendBody = body
	f.sendHdrs = headers
}
func (f *fakeDecoderCB) RequestRouteConfig() proto.Message { return f.routeCfg }
func (f *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (f *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (f *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (f *fakeDecoderCB) EncodeTrailers(http.Header)      {}
func (f *fakeDecoderCB) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (f *fakeDecoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (f *fakeDecoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (f *fakeDecoderCB) DownstreamTLSServerName() string  { return "" }
func (f *fakeDecoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (f *fakeDecoderCB) DownstreamProtocol() string       { return "" }
func (f *fakeDecoderCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (f *fakeDecoderCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeDecoderCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (f *fakeDecoderCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (f *fakeDecoderCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (f *fakeDecoderCB) RouteMetadata() *corev3.Metadata             { return nil }
func (f *fakeDecoderCB) RouteIncludeVhRateLimits() bool              { return false }

// TestDecodeHeaders_AllowPath_CountersIncremented verifies that on the allow path
// (bucket has tokens) DecodeHeaders returns Continue, increments enabled + ok,
// and does NOT call SendLocalReply.
func TestDecodeHeaders_AllowPath_CountersIncremented(t *testing.T) {
	reg := stats.NewRegistry()
	factory, err := New(mustAny(t, happyConfig()), envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	f := inst.Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	f.SetDecoderCallbacks(dcb)

	status := f.DecodeHeaders(nil, true)

	if status != envoyhttp.Continue {
		t.Errorf("DecodeHeaders allow: got %v, want Continue", status)
	}
	if dcb.sendCalled {
		t.Errorf("DecodeHeaders allow: SendLocalReply must NOT be called")
	}
	if got := f.state.listenerRC.stats.enabled.Load(); got != 1 {
		t.Errorf("enabled: got %d, want 1", got)
	}
	if got := f.state.listenerRC.stats.ok.Load(); got != 1 {
		t.Errorf("ok: got %d, want 1", got)
	}
	if got := f.state.listenerRC.stats.rateLimited.Load(); got != 0 {
		t.Errorf("rateLimited: got %d, want 0", got)
	}
	if got := f.state.listenerRC.stats.enforced.Load(); got != 0 {
		t.Errorf("enforced: got %d, want 0", got)
	}
}

// TestDecodeHeaders_NilStats_NoPanic verifies the request-path nil-tolerance
// contract: a filter built with a nil ctx.Stats (test path per the
// runtimeConfig doc + ADR-0085) must not panic in DecodeHeaders on either the
// allow arm or the rate-limited arm — every stat touch is nil-guarded
// (matching the bandwidthlimit sibling discipline).
func TestDecodeHeaders_NilStats_NoPanic(t *testing.T) {
	cfg := happyConfig()
	cfg.TokenBucket.MaxTokens = 1
	factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{}) // nil Stats
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	f := inst.Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	f.SetDecoderCallbacks(dcb)

	// Allow arm (first token) must not panic.
	if status := f.DecodeHeaders(nil, true); status != envoyhttp.Continue {
		t.Errorf("DecodeHeaders allow: got %v, want Continue", status)
	}
	// Rate-limited arm (bucket exhausted) must not panic either.
	if status := f.DecodeHeaders(nil, true); status != envoyhttp.StopIteration {
		t.Errorf("DecodeHeaders limited: got %v, want StopIteration", status)
	}
	if !dcb.sendCalled {
		t.Errorf("DecodeHeaders limited: SendLocalReply must be called")
	}
}

// TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep verifies that on
// the rate-limited path DecodeHeaders returns StopIteration, calls SendLocalReply
// with the canonical status/body/header, increments all 4 counters correctly,
// and upholds the MVP invariant rateLimited == enforced (ADR-0118 lockstep).
func TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep(t *testing.T) {
	cfg := &localratelimitv3.LocalRateLimit{
		StatPrefix: "rl_test",
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    1,
			FillInterval: durationpb.New(60 * time.Second),
			// TokensPerFill omitted → defaults to 1; fill interval very long
			// so the bucket is exhausted after the first consume.
		},
	}
	reg := stats.NewRegistry()
	factory, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	f := inst.Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	f.SetDecoderCallbacks(dcb)

	// First call: bucket has 1 token → allow.
	first := f.DecodeHeaders(nil, true)
	if first != envoyhttp.Continue {
		t.Errorf("first call: got %v, want Continue", first)
	}
	if dcb.sendCalled {
		t.Errorf("first call: SendLocalReply must NOT be called")
	}

	// Second call: bucket exhausted → rate-limited.
	second := f.DecodeHeaders(nil, true)
	if second != envoyhttp.StopIteration {
		t.Errorf("second call: got %v, want StopIteration", second)
	}
	if !dcb.sendCalled {
		t.Fatalf("second call: SendLocalReply must be called")
	}
	if dcb.sendStatus != 429 {
		t.Errorf("SendLocalReply status: got %d, want 429", dcb.sendStatus)
	}
	if dcb.sendBody != "local_rate_limited" {
		t.Errorf("SendLocalReply body: got %q, want %q", dcb.sendBody, "local_rate_limited")
	}
	if len(dcb.sendHdrs) != 1 || dcb.sendHdrs[0].Name != "Content-Type" || dcb.sendHdrs[0].Value != "text/plain" {
		t.Errorf("SendLocalReply headers: got %v, want [{Content-Type text/plain}]", dcb.sendHdrs)
	}

	if got := f.state.listenerRC.stats.enabled.Load(); got != 2 {
		t.Errorf("enabled: got %d, want 2", got)
	}
	if got := f.state.listenerRC.stats.ok.Load(); got != 1 {
		t.Errorf("ok: got %d, want 1", got)
	}
	if got := f.state.listenerRC.stats.rateLimited.Load(); got != 1 {
		t.Errorf("rateLimited: got %d, want 1", got)
	}
	if got := f.state.listenerRC.stats.enforced.Load(); got != 1 {
		t.Errorf("enforced: got %d, want 1", got)
	}
	// MVP invariant per ADR-0118: rateLimited == enforced (lockstep).
	if f.state.listenerRC.stats.rateLimited.Load() != f.state.listenerRC.stats.enforced.Load() {
		t.Errorf("MVP lockstep violated: rateLimited=%d enforced=%d",
			f.state.listenerRC.stats.rateLimited.Load(), f.state.listenerRC.stats.enforced.Load())
	}
}

// TestDecodeHeaders_PerRouteOverride_IndependentBuckets validates per-route
// TPFC bucket independence per SPEC §11.6 + ADR-0117.
//
// IMPL-1: per-route TPFC entries are *localratelimitv3.LocalRateLimit directly
// (no LocalRateLimitPerRoute wrapper — that proto does not exist in upstream
// Envoy v1.37.2; per PROGRESS.md preamble IMPL-1).
func TestDecodeHeaders_PerRouteOverride_IndependentBuckets(t *testing.T) {
	// Build a listener-level config with cap=10.
	listenerCfg := happyConfig()
	listenerCfg.StatPrefix = "listener_prefix"
	listenerCfg.TokenBucket.MaxTokens = 10
	listenerCfg.TokenBucket.FillInterval = durationpb.New(60 * time.Second)
	reg := stats.NewRegistry()
	factory, err := New(mustAny(t, listenerCfg), envoyhttp.FactoryCtx{Stats: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory().Decoder.(*filter)

	// Build TWO per-route LocalRateLimit proto messages directly (IMPL-1: no
	// LocalRateLimitPerRoute wrapper).
	perRouteA := &localratelimitv3.LocalRateLimit{
		StatPrefix: "perroute_a",
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    1,
			FillInterval: durationpb.New(60 * time.Second),
		},
	}
	perRouteB := &localratelimitv3.LocalRateLimit{
		StatPrefix: "perroute_b",
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    1,
			FillInterval: durationpb.New(60 * time.Second),
		},
	}

	// Resolve each per-route to its *runtimeConfig.
	rcA := inst.state.resolvePerRouteConfig(perRouteA)
	rcB := inst.state.resolvePerRouteConfig(perRouteB)
	rcListener := inst.state.resolvePerRouteConfig(nil)

	// Assert pointer-distinct.
	if rcA == rcB {
		t.Error("perRouteA and perRouteB should resolve to DIFFERENT *runtimeConfig instances")
	}
	if rcA == rcListener || rcB == rcListener {
		t.Error("per-route should NOT alias listener-level *runtimeConfig")
	}
	if rcA.bucket == rcB.bucket {
		t.Error("perRouteA and perRouteB should have INDEPENDENT *tokenBucket pointers")
	}
	if rcA.stats == rcB.stats {
		t.Error("perRouteA and perRouteB should have INDEPENDENT *filterStats pointers")
	}

	// Assert idempotent re-resolution (same pointer in → same *runtimeConfig out).
	rcAAgain := inst.state.resolvePerRouteConfig(perRouteA)
	if rcA != rcAAgain {
		t.Error("re-resolving perRouteA should return the SAME *runtimeConfig (idempotent)")
	}

	// Drain rcA's bucket; verify rcB unaffected.
	if !rcA.bucket.tryConsume() {
		t.Fatal("rcA initial tryConsume should succeed (cap=1)")
	}
	if rcA.bucket.tryConsume() {
		t.Error("rcA second tryConsume should fail (drained)")
	}
	if !rcB.bucket.tryConsume() {
		t.Error("rcB tryConsume should succeed independently (NOT affected by rcA drain)")
	}
}

// TestStatNames_FourCountersUnderStatPrefix verifies that newFilterStats registers
// exactly the 4 expected counter names under the given stat_prefix via
// stats.Registry.Walk.
func TestStatNames_FourCountersUnderStatPrefix(t *testing.T) {
	reg := stats.NewRegistry()
	prefix := "myns"
	newFilterStats(reg, prefix)

	want := []string{
		"myns.http_local_rate_limit.enabled",
		"myns.http_local_rate_limit.ok",
		"myns.http_local_rate_limit.rate_limited",
		"myns.http_local_rate_limit.enforced",
	}
	var got []string
	reg.Walk(func(m stats.Metric) {
		got = append(got, m.Name())
	})
	if len(got) != len(want) {
		t.Fatalf("stat count: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("stat[%d]: got %q, want %q", i, got[i], name)
		}
	}
}
