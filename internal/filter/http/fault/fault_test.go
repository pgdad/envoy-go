package fault

import (
	"net/http"
	"strconv"
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
type recordingDCB struct {
	sentStatus  int
	sentBody    string
	sentHeaders envoyhttp.OrderedHeaders
	continued   atomic.Int32
	routeCfg    proto.Message
}

func (r *recordingDCB) SendLocalReply(s int, b string, h envoyhttp.OrderedHeaders) {
	r.sentStatus = s
	r.sentBody = b
	r.sentHeaders = h
}
func (r *recordingDCB) ContinueDecoding()                 { r.continued.Add(1) }
func (r *recordingDCB) RequestRouteConfig() proto.Message { return r.routeCfg }
func (r *recordingDCB) EncodeHeaders(http.Header, bool)   {}
func (r *recordingDCB) EncodeData([]byte, bool)           {}
func (r *recordingDCB) EncodeTrailers(http.Header)        {}

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
	if dcb.sentStatus != 503 {
		t.Errorf("sentStatus: got %d, want 503", dcb.sentStatus)
	}
	if dcb.sentBody != "fault filter abort" {
		t.Errorf("sentBody: got %q, want %q", dcb.sentBody, "fault filter abort")
	}
	if got, want := len(dcb.sentBody), 18; got != want {
		t.Errorf("body length: got %d, want %d (no trailing newline)", got, want)
	}
	if len(dcb.sentHeaders) != 1 || dcb.sentHeaders[0].Name != "Content-Type" || dcb.sentHeaders[0].Value != "text/plain" {
		t.Errorf("sentHeaders: got %+v, want OrderedHeaders{Content-Type: text/plain}", dcb.sentHeaders)
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
	if dcb.sentStatus != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no SendLocalReply at 0%%)", dcb.sentStatus)
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
	if dcb.sentStatus != 503 {
		t.Errorf("sentStatus: got %d, want 503", dcb.sentStatus)
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
	if dcb.sentStatus != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no fault on value mismatch)", dcb.sentStatus)
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
	if dcb.sentStatus != 0 {
		t.Errorf("sentStatus: got %d, want 0 (no fault when headers absent)", dcb.sentStatus)
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
