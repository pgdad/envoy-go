package bootstrap

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	commonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	otlpalv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// FuzzParseOpenTelemetryAccessLogConfig exercises the OTLP access-log parse arm
// (phase 45.1 Task 2) for arbitrary byte inputs. It MUST NOT panic; any input
// must return either nil or a "bootstrap:"-prefixed error (D-OTLP-FUZZER).
//
// The seeds exercise: empty/degenerate wire, a truncated common_config, the
// reused CommonGrpcAccessLogConfig buffer/flush/log_name paths (shared with the
// 44.x grpc-ALS fuzzer), disable_builtin_labels (field 5), and the phase-45.1
// "not supported"/"inert" rejection arms (body, attributes, resource_attributes,
// formatters, stat_prefix).
func FuzzParseOpenTelemetryAccessLogConfig(f *testing.F) {
	f.Add([]byte{})                                       // empty
	f.Add([]byte("\x0a\x00"))                             // a truncated/empty common_config
	f.Add([]byte("\x00\x00\x00"))                         // degenerate null bytes
	f.Add([]byte("\xff\xff\xff\xff\xff\xff\xff\xff\xff")) // overlong varint

	// Marshal real OpenTelemetryAccessLogConfig messages so the wire is
	// well-formed and the various arms are reached deterministically.
	addSeed := func(cfg *otlpalv3.OpenTelemetryAccessLogConfig) {
		b, err := proto.Marshal(cfg)
		if err != nil {
			f.Fatalf("marshal otlp seed: %v", err)
		}
		f.Add(b)
	}

	// common_config with a log_name + cluster (the happy-ish path).
	addSeed(&otlpalv3.OpenTelemetryAccessLogConfig{
		CommonConfig: &commonv3.CommonGrpcAccessLogConfig{
			LogName: "otlp-log",
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: "otlp-cluster"},
				},
			},
		},
	})
	// disable_builtin_labels = true (field 5).
	addSeed(&otlpalv3.OpenTelemetryAccessLogConfig{
		CommonConfig:         &commonv3.CommonGrpcAccessLogConfig{LogName: "x"},
		DisableBuiltinLabels: true,
	})
	// buffer_size_bytes + buffer_flush_interval set (reused 44.2 machinery).
	addSeed(&otlpalv3.OpenTelemetryAccessLogConfig{
		CommonConfig: &commonv3.CommonGrpcAccessLogConfig{
			LogName:             "buf",
			BufferSizeBytes:     wrapperspb.UInt32(16384),
			BufferFlushInterval: durationpb.New(1),
		},
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Synthesize a *anypb.Any with the OTLP-ALS TypeURL so the arm is reached.
		a := &anypb.Any{TypeUrl: otlpAccessLogTypeURL, Value: data}
		result := &Bootstrap{}
		// parseOpenTelemetryAccessLog MUST NOT panic regardless of data content.
		_ = parseOpenTelemetryAccessLog(a, 0, result)
	})
}
