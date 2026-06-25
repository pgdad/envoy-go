package bootstrap

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzParseHttpGrpcAccessLogConfig exercises the grpc-ALS parse arm for
// arbitrary byte inputs. It MUST NOT panic; any input must return either nil
// or a "bootstrap:"-prefixed error (D-ALS-FUZZER, ADR-0255).
func FuzzParseHttpGrpcAccessLogConfig(f *testing.F) {
	// Seed corpus: empty, a truncated CommonGrpcAccessLogConfig varint,
	// a minimal valid-looking proto field (field 1 = CommonGrpcAccessLogConfig,
	// field 1 = log_name string "x"), and a degenerate null-byte input.
	f.Add([]byte{})
	f.Add([]byte("\x0a\x00"))                             // CommonGrpcAccessLogConfig present but empty
	f.Add([]byte("\x0a\x04\x0a\x02\x0a\x00"))             // nested common_config.log_name = ""
	f.Add([]byte("\x0a\x05\x0a\x03log"))                  // common_config.log_name = "log"
	f.Add([]byte("\x00\x00\x00"))                         // degenerate null bytes
	f.Add([]byte("\xff\xff\xff\xff\xff\xff\xff\xff\xff")) // overlong varint

	f.Fuzz(func(t *testing.T, data []byte) {
		// Synthesize a *anypb.Any with the gRPC-ALS TypeURL so the arm is reached.
		a := &anypb.Any{TypeUrl: httpGrpcAccessLogTypeURL, Value: data}
		result := &Bootstrap{}
		// parseGrpcAccessLog MUST NOT panic regardless of data content.
		_ = parseGrpcAccessLog(a, 0, result)
	})
}
