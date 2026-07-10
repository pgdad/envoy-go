package tap

import (
	"path/filepath"
	"strings"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	taptapv3 "github.com/envoyproxy/go-control-plane/envoy/config/tap/v3"
	commontapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/tap/v3"
	httptapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/tap/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func reqTapYes() *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestHeadersMatch{
		HttpRequestHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: "x-tap", HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}}},
	}}
}

func respStatus204() *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseHeadersMatch{
		HttpResponseHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: ":status", HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "204"}}}},
	}}
}

func fileSink(prefix string) *taptapv3.OutputSink {
	return &taptapv3.OutputSink{
		Format:         taptapv3.OutputSink_JSON_BODY_AS_STRING,
		OutputSinkType: &taptapv3.OutputSink_FilePerTap{FilePerTap: &taptapv3.FilePerTapSink{PathPrefix: prefix}},
	}
}

// validTap returns a minimal accepted Tap config writing under dir.
func validTap(dir string) *httptapv3.Tap {
	return &httptapv3.Tap{CommonConfig: &commontapv3.CommonExtensionConfig{
		ConfigType: &commontapv3.CommonExtensionConfig_StaticConfig{StaticConfig: &taptapv3.TapConfig{
			Match:        reqTapYes(),
			OutputConfig: &taptapv3.OutputConfig{Sinks: []*taptapv3.OutputSink{fileSink(filepath.Join(dir, "out"))}},
		}},
	}}
}

// validTapReqAndResp returns a Tap config whose match requires BOTH a
// request-header match and a response-header match (mirrors the 0099
// fixture). Only a value that observes BOTH the decode and encode legs can
// resolve this predicate True — a two-value decoder/encoder split leaves the
// response arm Undetermined on the decoder-side value.
func validTapReqAndResp(dir string) *httptapv3.Tap {
	tp := validTap(dir)
	tp.GetCommonConfig().GetStaticConfig().Match = &cmatcherv3.MatchPredicate{
		Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{
			Rules: []*cmatcherv3.MatchPredicate{reqTapYes(), respStatus204()},
		}},
	}
	return tp
}

func newCtx() (envoyhttp.FactoryCtx, *stats.Registry) {
	reg := stats.NewRegistry()
	return envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_probe"}, reg
}

func TestNew_AcceptsMinimalConfig(t *testing.T) {
	ctx, _ := newCtx()
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNew_RegistersExactlyOneCounter_ReadingZero(t *testing.T) {
	ctx, reg := newCtx()
	before := countMetrics(reg)
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := countMetrics(reg) - before; got != 1 {
		t.Errorf("registered %d metrics, want exactly 1", got)
	}
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == "http.hcm_probe.tap.rq_tapped" {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	if found == nil {
		t.Fatalf("counter http.hcm_probe.tap.rq_tapped not registered")
	}
	if got := found.Load(); got != 0 {
		t.Errorf("with no taps the counter must read 0; got %d", got)
	}
}

func countMetrics(reg *stats.Registry) int {
	n := 0
	reg.Walk(func(stats.Metric) { n++ })
	return n
}

// The `.tap.` segment is HARDCODED — it is NOT the http_filters[] entry name.
func TestNew_StatSegmentIsHardcodedNotFilterName(t *testing.T) {
	ctx, reg := newCtx()
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
	names := map[string]bool{}
	reg.Walk(func(m stats.Metric) { names[m.Name()] = true })
	if !names["http.hcm_probe.tap.rq_tapped"] {
		t.Errorf("want http.hcm_probe.tap.rq_tapped; got %v", names)
	}
}

func TestNew_RejectRoster(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "out")

	withStatic := func(mut func(*taptapv3.TapConfig)) *httptapv3.Tap {
		tp := validTap(dir)
		mut(tp.GetCommonConfig().GetStaticConfig())
		return tp
	}

	cases := map[string]struct {
		tp      *httptapv3.Tap
		wantErr string
	}{
		"record_headers_received_time": {tp: func() *httptapv3.Tap {
			tp := validTap(dir)
			tp.RecordHeadersReceivedTime = true
			return tp
		}(), wantErr: "record_headers_received_time is not supported"},
		"admin_config": {tp: &httptapv3.Tap{CommonConfig: &commontapv3.CommonExtensionConfig{
			ConfigType: &commontapv3.CommonExtensionConfig_AdminConfig{
				AdminConfig: &commontapv3.AdminConfig{ConfigId: "x"}}}},
			wantErr: "common_config.admin_config is not supported"},
		"common_config_unset": {tp: &httptapv3.Tap{}, wantErr: "common_config required"},
		"match_config_set": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.MatchConfig = &taptapv3.MatchPredicate{Rule: &taptapv3.MatchPredicate_AnyMatch{AnyMatch: true}} //nolint:staticcheck // SA1019: exercising PARSE-REJECT of this deprecated field.
		}), wantErr: "match_config (deprecated) is not supported"},
		"tap_enabled_set": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.TapEnabled = &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{
				Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}}
		}), wantErr: "tap_enabled is not supported"},
		"neither_match_nor_match_config": {tp: withStatic(func(sc *taptapv3.TapConfig) { sc.Match = nil }),
			wantErr: "neither match nor match_config is set"},
		"streaming_true": {tp: withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Streaming = true }),
			wantErr: "output_config.streaming=true is not supported"},
		"zero_sinks": {tp: withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Sinks = nil }),
			wantErr: "sinks must contain exactly 1 item(s)"},
		"two_sinks": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks = []*taptapv3.OutputSink{fileSink(prefix), fileSink(prefix + "2")}
		}), wantErr: "sinks must contain exactly 1 item(s)"},
		"format_proto_binary": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_BINARY
		}), wantErr: "format PROTO_BINARY is not supported"},
		"format_proto_binary_length_delimited": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_BINARY_LENGTH_DELIMITED
		}), wantErr: "format PROTO_BINARY_LENGTH_DELIMITED is not supported"},
		"format_proto_text": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_TEXT
		}), wantErr: "format PROTO_TEXT is not supported"},
		"sink_streaming_admin": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_StreamingAdmin{StreamingAdmin: &taptapv3.StreamingAdminSink{}}
		}), wantErr: "streaming_admin sink is not supported"},
		"sink_buffered_admin": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_BufferedAdmin{BufferedAdmin: &taptapv3.BufferedAdminSink{}}
		}), wantErr: "buffered_admin sink is not supported"},
		"sink_streaming_grpc": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_StreamingGrpc{StreamingGrpc: &taptapv3.StreamingGrpcSink{}}
		}), wantErr: "streaming_grpc sink is not supported"},
		"sink_custom": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_CustomSink{CustomSink: &corev3.TypedExtensionConfig{Name: "x"}}
		}), wantErr: "custom_sink is not supported"},
		"sink_no_arm": {tp: withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Sinks[0].OutputSinkType = nil }),
			wantErr: "no output_sink_type set"},
		"empty_path_prefix": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_FilePerTap{FilePerTap: &taptapv3.FilePerTapSink{}}
		}), wantErr: "file_per_tap.path_prefix required"},
		"match_trailer_arm": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.Match = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestTrailersMatch{
				HttpRequestTrailersMatch: &cmatcherv3.HttpHeadersMatch{}}}
		}), wantErr: "trailers_match is not supported"},
		"match_generic_body_arm": {tp: withStatic(func(sc *taptapv3.TapConfig) {
			sc.Match = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch{
				HttpRequestGenericBodyMatch: &cmatcherv3.HttpGenericBodyMatch{}}}
		}), wantErr: "generic_body_match is not supported"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := newCtx()
			f, err := New(mustAny(t, tc.tp), ctx)
			if err == nil {
				t.Errorf("expected reject, got nil error")
			}
			if f != nil {
				t.Errorf("expected nil factory on reject, got %v", f)
			}
			if err != nil && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: error %q does not contain %q", name, err, tc.wantErr)
			}
		})
	}
}

// Both JSON formats are accepted at 56.1 (indistinguishable without a body).
func TestNew_AcceptsBothJSONFormats(t *testing.T) {
	for _, f := range []taptapv3.OutputSink_Format{
		taptapv3.OutputSink_JSON_BODY_AS_BYTES, // the proto default (0)
		taptapv3.OutputSink_JSON_BODY_AS_STRING,
	} {
		tp := validTap(t.TempDir())
		tp.GetCommonConfig().GetStaticConfig().GetOutputConfig().GetSinks()[0].Format = f
		ctx, _ := newCtx()
		if _, err := New(mustAny(t, tp), ctx); err != nil {
			t.Errorf("format %v: New = %v, want nil", f, err)
		}
	}
}

func TestNew_NilAndGarbageTypedConfig(t *testing.T) {
	ctx, _ := newCtx()
	if _, err := New(nil, ctx); err == nil {
		t.Errorf("nil typed_config: want error")
	}
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	if _, err := New(bad, ctx); err == nil {
		t.Errorf("garbage typed_config: want error")
	}
}
