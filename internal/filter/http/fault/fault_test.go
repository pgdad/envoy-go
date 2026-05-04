package fault

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	faultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err == nil {
		t.Fatal("expected error for nil tc; got nil")
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: "type.googleapis.com/garbage", Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(bad, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err == nil {
		t.Fatal("expected error for malformed tc; got nil")
	}
}

func TestNew_AbortHTTPStatusOutOfRange(t *testing.T) {
	cases := []struct {
		name   string
		status uint32
	}{
		{"zero", 0},
		{"too_high", 9999},
		{"too_low", 100},
		{"upper_exclusive", 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &faultv3.HTTPFault{
				Abort: &faultv3.FaultAbort{
					Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
					ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: tc.status},
				},
			}
			_, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
			if err == nil {
				t.Fatalf("status=%d: expected error; got nil", tc.status)
			}
		})
	}
}

func TestNew_DelayPercentageWithoutFixedDelay(t *testing.T) {
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage: &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
		},
	}
	_, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err == nil {
		t.Fatal("expected error for delay.percentage > 0 without delay.fixed_delay; got nil")
	}
}

func TestNew_HappyPath(t *testing.T) {
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(100 * time.Millisecond)},
		},
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
		},
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}
	if factory == nil {
		t.Fatal("factory is nil")
	}
	inst := factory()
	if inst.Decoder == nil || inst.Encoder == nil {
		t.Errorf("expected both Decoder and Encoder set; got %+v", inst)
	}
	if inst.Name != "envoy.filters.http.fault" {
		t.Errorf("Name: got %q, want %q", inst.Name, "envoy.filters.http.fault")
	}
}

func TestNew_RegistersStats(t *testing.T) {
	reg := stats.NewRegistry()
	f := &faultv3.HTTPFault{
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
		},
	}
	_, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	expectedNames := []string{
		"http.ingress_http.fault.aborts_injected",
		"http.ingress_http.fault.delays_injected",
		"http.ingress_http.fault.faults_overflow",
		"http.ingress_http.fault.active_faults",
		"http.ingress_http.fault.response_rl_injected",
	}
	seen := map[string]bool{}
	reg.Walk(func(m stats.Metric) { seen[m.Name()] = true })
	for _, n := range expectedNames {
		if !seen[n] {
			t.Errorf("missing stat: %q", n)
		}
	}
}

func TestRuntimeConfig_FieldExtraction(t *testing.T) {
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 25, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(50 * time.Millisecond)},
		},
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 75, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 418},
		},
		MaxActiveFaults: wrapperspb.UInt32(5),
		Headers: []*routev3.HeaderMatcher{
			{
				Name: "x-fault-on",
				HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{
					StringMatch: &matcherv3.StringMatcher{
						MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"},
					},
				},
			},
		},
	}
	rc, err := parseRuntimeConfig(f)
	if err != nil {
		t.Fatalf("parseRuntimeConfig: %v", err)
	}
	if !rc.delayEnabled || rc.delayPercentage != 25.0 || rc.delayFixedDelay != 50*time.Millisecond {
		t.Errorf("delay fields: got enabled=%v p=%v d=%v", rc.delayEnabled, rc.delayPercentage, rc.delayFixedDelay)
	}
	if !rc.abortEnabled || rc.abortPercentage != 75.0 || rc.abortHTTPStatus != 418 {
		t.Errorf("abort fields: got enabled=%v p=%v s=%v", rc.abortEnabled, rc.abortPercentage, rc.abortHTTPStatus)
	}
	if rc.maxActiveFaults != 5 {
		t.Errorf("maxActiveFaults: got %v, want 5", rc.maxActiveFaults)
	}
	if len(rc.matchHeaders) != 1 || rc.matchHeaders[0].name != "X-Fault-On" || rc.matchHeaders[0].exactValue != "yes" {
		t.Errorf("matchHeaders: got %+v", rc.matchHeaders)
	}
}

// recordingDCB captures SendLocalReply + ContinueDecoding invocations from the
// fault filter's decode-side discipline. Pre-Task-4 the fault tests did not
// need a callback mock (Task-3 stub returns Continue without consulting f.dcb);
// Task 4's abort-only path calls f.dcb.SendLocalReply, so this stub records
// the (status, body, headers) triple for assertion.
//
// Task 5 introduces the timer-callback path: SendLocalReply may be invoked
// from the time.AfterFunc-spawned goroutine while the test goroutine is
// polling for completion. The (status, body, headers) triple is therefore
// guarded by a sync.Mutex; tests read via the Status/Body/Headers accessors
// rather than touching the unexported fields directly. ContinueDecoding's
// counter is already an atomic.Int32 and is goroutine-safe without the mutex.
type recordingDCB struct {
	mu          sync.Mutex
	sentStatus  int
	sentBody    string
	sentHeaders envoyhttp.OrderedHeaders
	continued   atomic.Int32
	routeCfg    proto.Message
}

func (r *recordingDCB) SendLocalReply(s int, b string, h envoyhttp.OrderedHeaders) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sentStatus = s
	r.sentBody = b
	r.sentHeaders = h
}
func (r *recordingDCB) Status() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sentStatus
}
func (r *recordingDCB) Body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sentBody
}
func (r *recordingDCB) Headers() envoyhttp.OrderedHeaders {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sentHeaders
}
func (r *recordingDCB) ContinueDecoding()                 { r.continued.Add(1) }
func (r *recordingDCB) RequestRouteConfig() proto.Message { return r.routeCfg }
func (r *recordingDCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (r *recordingDCB) EncodeHeaders(http.Header, bool) {}
func (r *recordingDCB) EncodeData([]byte, bool)         {}
func (r *recordingDCB) EncodeTrailers(http.Header)      {}

// makeFilter constructs a fault filter with the supplied abort.http_status,
// abort.percentage, and headers-field shape, returning the *filter and the
// attached *recordingDCB. Used by the Task-4 abort-only / headers / percentage
// tests.
func makeFilter(t *testing.T, abortStatus uint32, abortPercent uint32, headers []*routev3.HeaderMatcher) (*filter, *recordingDCB) {
	t.Helper()
	f := &faultv3.HTTPFault{
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: abortPercent, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: abortStatus},
		},
		Headers: headers,
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl := inst.Decoder.(*filter)
	dcb := &recordingDCB{}
	fl.SetDecoderCallbacks(dcb)
	return fl, dcb
}

// TestDecodeHeaders_AbortOnly_100Percent verifies the abort-only terminal-replace
// path per SPEC §6.4 + §11.3: abort 503 100% → StopIteration + sentStatus=503 +
// sentBody="fault filter abort" (18 bytes, no trailing newline) +
// sentHeaders=OrderedHeaders{Content-Type: text/plain}.
func TestDecodeHeaders_AbortOnly_100Percent(t *testing.T) {
	fl, dcb := makeFilter(t, 503, 100, nil)
	status := fl.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration", status)
	}
	if got := dcb.Status(); got != 503 {
		t.Errorf("sentStatus: got %d, want 503", got)
	}
	if got := dcb.Body(); got != "fault filter abort" {
		t.Errorf("sentBody: got %q, want %q", got, "fault filter abort")
	}
	if got, want := len(dcb.Body()), 18; got != want {
		t.Errorf("body length: got %d, want %d (no trailing newline)", got, want)
	}
	if h := dcb.Headers(); len(h) != 1 || h[0].Name != "Content-Type" || h[0].Value != "text/plain" {
		t.Errorf("sentHeaders: got %+v, want OrderedHeaders{Content-Type: text/plain}", h)
	}
}

// TestDecodeHeaders_AbortOnly_0Percent verifies the percentage gate at the
// short-circuit boundary: abort 503 0% → Continue + no SendLocalReply
// (rollPercent's p<=0 short-circuit returns false without consulting RNG).
func TestDecodeHeaders_AbortOnly_0Percent(t *testing.T) {
	fl, dcb := makeFilter(t, 503, 0, nil)
	status := fl.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (0%% should not fire)", status)
	}
	if got := dcb.Status(); got != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no SendLocalReply at 0%%)", got)
	}
}

// TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName verifies that
// the headers-field name comparison is case-insensitive (canonicalized via
// http.CanonicalHeaderKey at parse time + http.Header.Get at fault-eval time)
// per §11.8 conclusion (a). Header "X-FAULT-ON: yes" against matcher
// (name="x-fault-on", exact="yes") fires the fault.
func TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName(t *testing.T) {
	headers := []*routev3.HeaderMatcher{
		{Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
	}
	fl, dcb := makeFilter(t, 503, 100, headers)
	h := http.Header{}
	h.Set("X-FAULT-ON", "yes") // uppercase name
	status := fl.DecodeHeaders(h, true)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (case-insensitive name match)", status)
	}
	if got := dcb.Status(); got != 503 {
		t.Errorf("sentStatus: got %d, want 503", got)
	}
}

// TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue verifies that
// the headers-field value comparison is case-sensitive (byte-equal) per §11.8
// conclusion (b). Header "x-fault-on: YES" against matcher exact "yes" does
// NOT match → Continue.
func TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue(t *testing.T) {
	headers := []*routev3.HeaderMatcher{
		{Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
	}
	fl, dcb := makeFilter(t, 503, 100, headers)
	h := http.Header{}
	h.Set("x-fault-on", "YES") // uppercase value — should NOT match
	status := fl.DecodeHeaders(h, true)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (case-sensitive value mismatch)", status)
	}
	if got := dcb.Status(); got != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no fault on value mismatch)", got)
	}
}

// TestDecodeHeaders_NoFaultHeaderMismatch verifies that when the headers field
// is non-empty but the request carries none of the configured matchers, the
// fault does NOT fire (Continue). Per §11.8 conclusion: empty matchHeaders =
// match-all; non-empty requires ALL pairs to match.
func TestDecodeHeaders_NoFaultHeaderMismatch(t *testing.T) {
	headers := []*routev3.HeaderMatcher{
		{Name: "x-fault-on", HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "yes"}}}},
	}
	fl, dcb := makeFilter(t, 503, 100, headers)
	status := fl.DecodeHeaders(http.Header{}, true) // empty headers → no match
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if got := dcb.Status(); got != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no fault when headers absent)", got)
	}
}

// TestDecodeHeaders_AbortStatRecorded verifies the recordFaultEvent
// (eventAbortsInjected) Inc dispatch on the abort path. After a 100% abort
// fires, http.ingress_http.fault.aborts_injected counter == 1.
func TestDecodeHeaders_AbortStatRecorded(t *testing.T) {
	reg := stats.NewRegistry()
	f := &faultv3.HTTPFault{
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 503},
		},
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl := inst.Decoder.(*filter)
	fl.SetDecoderCallbacks(&recordingDCB{})
	fl.DecodeHeaders(http.Header{}, true)
	// Walk to find the aborts_injected counter.
	var got int64
	reg.Walk(func(m stats.Metric) {
		if m.Name() == "http.ingress_http.fault.aborts_injected" {
			got, _ = strconv.ParseInt(m.Format(), 10, 64)
		}
	})
	if got != 1 {
		t.Errorf("aborts_injected: got %d, want 1", got)
	}
}

// counterValue walks the Registry and returns the Load() of the counter named
// `name`, or -1 if no counter by that name is registered. Mirrors the helper
// at internal/filter/http/router/router_test.go:counterValue per the broader
// project precedent for stat-counter assertions across the package boundary.
// Used by Task-5 timer-callback Inc-from-timer-goroutine assertions.
func counterValue(t *testing.T, r *stats.Registry, name string) int64 {
	t.Helper()
	got := int64(-1)
	r.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				got = int64(c.Load())
			}
		}
	})
	return got
}

// makeDelayFilter constructs a fault filter with delay-only or combined
// delay+abort configuration. delayMs is the fixed-delay in ms. abortStatus=0
// disables the abort side (delay-only path); non-zero enables the combined
// path. The supplied registry is used so the caller can assert counter values
// after the timer callback fires.
func makeDelayFilter(t *testing.T, reg *stats.Registry, delayMs uint32, abortStatus uint32) (*filter, *recordingDCB) {
	t.Helper()
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{
				FixedDelay: durationpb.New(time.Duration(delayMs) * time.Millisecond),
			},
		},
	}
	if abortStatus != 0 {
		f.Abort = &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: abortStatus},
		}
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "ingress_http"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl := inst.Decoder.(*filter)
	dcb := &recordingDCB{}
	fl.SetDecoderCallbacks(dcb)
	return fl, dcb
}

// waitForCondition polls fn at 2ms intervals until it returns true or the
// deadline elapses. Returns true if fn became true within the deadline.
// Used by the Task-5 timer-callback tests to avoid CPU spin while waiting on
// the time.AfterFunc-driven async resume.
func waitForCondition(deadline time.Duration, fn func() bool) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if fn() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return fn()
}

// TestDecodeHeaders_DelayOnly verifies the delay-only async-resume path per
// SPEC §5.2 + §11.2 + ADR-0102: delay 100% 50ms → DecodeHeaders returns
// StopIteration synchronously; a time.AfterFunc-scheduled callback fires
// after ~50ms and calls cb.ContinueDecoding(); no SendLocalReply is invoked
// (delay-only does NOT abort the request). Timing tolerance per §11.2
// conclusion (c): elapsed ∈ [40ms, 200ms].
func TestDecodeHeaders_DelayOnly(t *testing.T) {
	fl, dcb := makeDelayFilter(t, stats.NewRegistry(), 50, 0)
	start := time.Now()
	status := fl.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (delay-only parks the chain)", status)
	}
	// Wait up to 500ms for the timer callback to fire ContinueDecoding.
	if !waitForCondition(500*time.Millisecond, func() bool {
		return dcb.continued.Load() > 0
	}) {
		t.Fatalf("ContinueDecoding never invoked within 500ms; continued=%d", dcb.continued.Load())
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("elapsed: got %v, want in [40ms, 200ms] per §11.2 conclusion (c)", elapsed)
	}
	if got := dcb.Status(); got != 0 {
		t.Errorf("sentStatus: got %d, want 0 (delay-only must NOT call SendLocalReply)", got)
	}
}

// TestDecodeHeaders_Combined verifies the combined delay+abort timer-callback
// path per SPEC §5.4 + §11.3 + ADR-0102: delay 100% 50ms + abort 100% 503 →
// DecodeHeaders returns StopIteration synchronously; the timer fires after
// ~50ms and the callback invokes SendLocalReply with the abort response shape.
// The wire response is the abort response shape (sentStatus=503,
// sentBody="fault filter abort") arriving delay.fixed_delay after the request.
//
// The callback also calls ContinueDecoding to wake the parked dispatch
// goroutine (which is blocked in parkDecode after StopIteration was returned
// synchronously). Without that wake-up, the chain's RunDecodeHeaders would
// stay parked indefinitely even though localReplyDone is set; on resume it
// observes localReplyDone=true and returns terminated=false, so the
// ContinueDecoding signal is purely a parkDecode wake-up — the chain does NOT
// re-iterate the decode side. This is the load-bearing fixture-0011 scenario-2
// integration claim verified at Task 14.
func TestDecodeHeaders_Combined(t *testing.T) {
	fl, dcb := makeDelayFilter(t, stats.NewRegistry(), 50, 503)
	start := time.Now()
	status := fl.DecodeHeaders(http.Header{}, true)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (combined parks the chain at the timer)", status)
	}
	// Wait up to 500ms for the timer callback to invoke SendLocalReply.
	if !waitForCondition(500*time.Millisecond, func() bool {
		return dcb.Status() != 0
	}) {
		t.Fatalf("SendLocalReply never invoked within 500ms; sentStatus=%d", dcb.Status())
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("elapsed: got %v, want >= 40ms per §11.2 conclusion (c) lower bound", elapsed)
	}
	if got := dcb.Status(); got != 503 {
		t.Errorf("sentStatus: got %d, want 503", got)
	}
	if got := dcb.Body(); got != "fault filter abort" {
		t.Errorf("sentBody: got %q, want %q", got, "fault filter abort")
	}
	// continued == 1: the callback signals ContinueDecoding to wake parkDecode.
	// The chain's localReplyDone gate makes the resumed iteration short-circuit
	// immediately; the signal is a wake-up, not a re-iteration request.
	if got := dcb.continued.Load(); got != 1 {
		t.Errorf("continued: got %d, want 1 (combined wakes parkDecode after SendLocalReply)", got)
	}
}

// TestDecodeHeaders_DelayStatRecorded verifies the recordFaultEvent
// (eventDelaysInjected) Inc dispatch on the delay path. After a 100% delay
// fires, http.ingress_http.fault.delays_injected counter == 1. The Inc
// happens BEFORE the timer is scheduled (on the dispatch goroutine), so this
// assertion does not need to wait for the callback to fire.
func TestDecodeHeaders_DelayStatRecorded(t *testing.T) {
	reg := stats.NewRegistry()
	fl, _ := makeDelayFilter(t, reg, 50, 0)
	fl.DecodeHeaders(http.Header{}, true)
	if got := counterValue(t, reg, "http.ingress_http.fault.delays_injected"); got != 1 {
		t.Errorf("delays_injected: got %d, want 1", got)
	}
}

// TestDecodeHeaders_CombinedStatsRecorded verifies stat-Inc ordering on the
// combined path: delays_injected Inc happens synchronously on the dispatch
// goroutine; aborts_injected Inc happens from the timer-callback goroutine
// after the delay elapses. Both end at 1 after the timer fires.
func TestDecodeHeaders_CombinedStatsRecorded(t *testing.T) {
	reg := stats.NewRegistry()
	fl, dcb := makeDelayFilter(t, reg, 50, 503)
	fl.DecodeHeaders(http.Header{}, true)
	// delays_injected fires synchronously before the timer is scheduled.
	if got := counterValue(t, reg, "http.ingress_http.fault.delays_injected"); got != 1 {
		t.Errorf("delays_injected (synchronous): got %d, want 1", got)
	}
	// Wait for the timer callback to fire SendLocalReply + aborts_injected Inc.
	if !waitForCondition(500*time.Millisecond, func() bool {
		return dcb.Status() != 0
	}) {
		t.Fatalf("SendLocalReply never invoked within 500ms")
	}
	if got := counterValue(t, reg, "http.ingress_http.fault.aborts_injected"); got != 1 {
		t.Errorf("aborts_injected (post-timer): got %d, want 1", got)
	}
}

// TestDecodeHeaders_MaxActiveFaultsCapOverflow verifies the LBP-1 sixth
// application + faults_overflow stat per ADR-0105 + SPEC §5.6. delay 100% 200ms,
// max_active_faults=1; allocate 2 instances from the same factory (sharing the
// closure-captured *atomic.Int64 activeFaults counter). First DecodeHeaders →
// StopIteration (fault fires, active=1). Second DecodeHeaders → Continue (cap
// hit, faults_overflow Inc, no fault injected). faults_overflow == 1.
func TestDecodeHeaders_MaxActiveFaultsCapOverflow(t *testing.T) {
	reg := stats.NewRegistry()
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(200 * time.Millisecond)},
		},
		MaxActiveFaults: wrapperspb.UInt32(1),
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Allocate two instances from the same factory (sharing the activeFaults counter).
	fl1 := factory().Decoder.(*filter)
	fl1.SetDecoderCallbacks(&recordingDCB{})
	fl2 := factory().Decoder.(*filter)
	fl2.SetDecoderCallbacks(&recordingDCB{})

	s1 := fl1.DecodeHeaders(http.Header{}, true)
	if s1 != envoyhttp.StopIteration {
		t.Errorf("first request: got %v, want StopIteration (fault should fire)", s1)
	}
	s2 := fl2.DecodeHeaders(http.Header{}, true)
	if s2 != envoyhttp.Continue {
		t.Errorf("second request: got %v, want Continue (cap should skip fault)", s2)
	}
	if got := counterValue(t, reg, "http.x.fault.faults_overflow"); got != 1 {
		t.Errorf("faults_overflow: got %d, want 1", got)
	}
}

// TestOnDestroy_TimerStopped verifies that OnDestroy cancels the pending delay
// timer + Dec's the activeFaults counter (idempotent via markedActive guard) per
// ADR-0105 + SPEC §6.5. delay 100% 500ms; DecodeHeaders schedules timer; OnDestroy
// cancels before fire; sleep 100ms; assert dcb.continued == 0; assert
// active.Load() == 0 (Inc balanced by Dec).
func TestOnDestroy_TimerStopped(t *testing.T) {
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(500 * time.Millisecond)},
		},
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fl := factory().Decoder.(*filter)
	dcb := &recordingDCB{}
	fl.SetDecoderCallbacks(dcb)
	fl.DecodeHeaders(http.Header{}, true)
	fl.OnDestroy()                     // should cancel the timer before it fires
	time.Sleep(100 * time.Millisecond) // briefly wait; timer should NOT have fired
	if got := dcb.continued.Load(); got != 0 {
		t.Errorf("timer fired despite OnDestroy; ContinueDecoding called %d times", got)
	}
	// active counter should be balanced (Inc'd on DecodeHeaders, Dec'd on OnDestroy).
	if got := fl.active.Load(); got != 0 {
		t.Errorf("active counter: got %d, want 0", got)
	}
}

// TestPerRouteWholesaleOverride verifies the per-route 3-tier merge
// wholesale-override discipline per ADR-0073 + SPEC §6.5 + §11.7. Listener-level
// config carries a 200ms delay only; per-route HTTPFault carries an abort 100%
// 418 with NO delay. When `cb.RequestRouteConfig()` returns the per-route
// HTTPFault proto, `routeConfigOrListener` projects it WHOLESALE (NOT
// field-merge) so the listener-level delay is dropped; the abort fires
// immediately (elapsed < 50ms) and `sentStatus == 418`. Confirms that a
// per-route HTTPFault that omits `delay` does NOT inherit the listener-level
// `delay` per the §11.7 empirical pin.
func TestPerRouteWholesaleOverride(t *testing.T) {
	listenerCfg := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(200 * time.Millisecond)},
		},
	}
	routeCfg := &faultv3.HTTPFault{
		Abort: &faultv3.FaultAbort{
			Percentage: &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			ErrorType:  &faultv3.FaultAbort_HttpStatus{HttpStatus: 418},
		},
	}
	factory, err := New(mustAny(t, listenerCfg), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fl := factory().Decoder.(*filter)
	dcb := &recordingDCB{routeCfg: routeCfg}
	fl.SetDecoderCallbacks(dcb)
	start := time.Now()
	status := fl.DecodeHeaders(http.Header{}, true)
	elapsed := time.Since(start)
	if status != envoyhttp.StopIteration {
		t.Errorf("status: got %v, want StopIteration (per-route abort should fire)", status)
	}
	if got := dcb.Status(); got != 418 {
		t.Errorf("sentStatus: got %d, want 418 (per-route override)", got)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("elapsed: got %v, want <50ms (no inherited delay — wholesale-override)", elapsed)
	}
}

// TestFault_DelayTimerRace is the race-detector cycle test per planner-time
// decision 10. Skip under -short. Loop 100 iterations with 1ms delay; each
// iteration: factory() → DecodeHeaders → sleep 0/1ms → OnDestroy. Forces
// OnDestroy to race the timer-callback. The markedActive guard in
// decrementActive guarantees exactly-one Inc/Dec balance under the race.
func TestFault_DelayTimerRace(t *testing.T) {
	if testing.Short() {
		t.Skip("race-cycle test skipped under -short")
	}
	f := &faultv3.HTTPFault{
		Delay: &commonfaultv3.FaultDelay{
			Percentage:         &typev3.FractionalPercent{Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED},
			FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(1 * time.Millisecond)},
		},
	}
	factory, err := New(mustAny(t, f), envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 100; i++ {
		fl := factory().Decoder.(*filter)
		fl.SetDecoderCallbacks(&recordingDCB{})
		fl.DecodeHeaders(http.Header{}, true)
		// Race OnDestroy against the timer firing.
		time.Sleep(time.Duration(i%2) * time.Millisecond) // 0 or 1ms — straddles the 1ms timer
		fl.OnDestroy()
	}
}
