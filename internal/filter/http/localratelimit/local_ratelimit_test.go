package localratelimit

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
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
	if inst.rc.statusCode != 429 {
		t.Errorf("statusCode default: got %d, want 429", inst.rc.statusCode)
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
	if inst.rc.statPrefix != "myprefix" {
		t.Errorf("statPrefix: got %q, want %q", inst.rc.statPrefix, "myprefix")
	}
	if inst.rc.statusCode != 503 {
		t.Errorf("statusCode: got %d, want 503", inst.rc.statusCode)
	}
	if inst.rc.body != "local_rate_limited" {
		t.Errorf("body: got %q, want %q", inst.rc.body, "local_rate_limited")
	}
	if inst.rc.bucket == nil {
		t.Errorf("bucket: got nil, want non-nil")
	}
	if inst.rc.stats == nil {
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
}

func (f *fakeDecoderCB) ContinueDecoding() {}
func (f *fakeDecoderCB) SendLocalReply(status int, body string, headers envoyhttp.OrderedHeaders) {
	f.sendCalled = true
	f.sendStatus = status
	f.sendBody = body
	f.sendHdrs = headers
}
func (f *fakeDecoderCB) RequestRouteConfig() proto.Message { return nil }
func (f *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (f *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (f *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (f *fakeDecoderCB) EncodeTrailers(http.Header)      {}

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
	if got := f.rc.stats.enabled.Load(); got != 1 {
		t.Errorf("enabled: got %d, want 1", got)
	}
	if got := f.rc.stats.ok.Load(); got != 1 {
		t.Errorf("ok: got %d, want 1", got)
	}
	if got := f.rc.stats.rateLimited.Load(); got != 0 {
		t.Errorf("rateLimited: got %d, want 0", got)
	}
	if got := f.rc.stats.enforced.Load(); got != 0 {
		t.Errorf("enforced: got %d, want 0", got)
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

	if got := f.rc.stats.enabled.Load(); got != 2 {
		t.Errorf("enabled: got %d, want 2", got)
	}
	if got := f.rc.stats.ok.Load(); got != 1 {
		t.Errorf("ok: got %d, want 1", got)
	}
	if got := f.rc.stats.rateLimited.Load(); got != 1 {
		t.Errorf("rateLimited: got %d, want 1", got)
	}
	if got := f.rc.stats.enforced.Load(); got != 1 {
		t.Errorf("enforced: got %d, want 1", got)
	}
	// MVP invariant per ADR-0118: rateLimited == enforced (lockstep).
	if f.rc.stats.rateLimited.Load() != f.rc.stats.enforced.Load() {
		t.Errorf("MVP lockstep violated: rateLimited=%d enforced=%d",
			f.rc.stats.rateLimited.Load(), f.rc.stats.enforced.Load())
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
