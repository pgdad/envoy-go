package fault

import (
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
