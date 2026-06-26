package bootstrap

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzParseHttpGrpcAccessLogConfig exercises the grpc-ALS parse arm for
// arbitrary byte inputs. It MUST NOT panic; any input must return either nil
// or a "bootstrap:"-prefixed error (D-ALS-FUZZER, ADR-0255).
//
// Phase 44.2 (D-BUF-FUZZER-CORPUS): buffer_size_bytes (field 4 of
// CommonGrpcAccessLogConfig, a UInt32Value wrapper) and buffer_flush_interval
// (field 3, a Duration) are now CONSUMED by parseGrpcAccessLog. Seeds below
// explicitly exercise the nil-wrapper→16384 default, explicit-0, huge, sub-ms
// interval, very-long interval, and the absent-interval→1s coercion paths.
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

	// Phase 44.2 buffer seeds — raw proto wire bytes for HttpGrpcAccessLogConfig.
	// Outer tag \x0a = field 1 (common_config, length-delimited).
	// Inner \x22 = field 4 (buffer_size_bytes UInt32Value, length-delimited).
	// Inner \x1a = field 3 (buffer_flush_interval Duration, length-delimited).
	// UInt32Value.value = field 1 varint (\x08).
	// Duration.seconds = field 1 varint (\x08); .nanos = field 2 varint (\x10).

	// buffer_size_bytes = 1 (tiny; wrapper present, non-zero)
	f.Add([]byte("\x0a\x04\x22\x02\x08\x01"))
	// buffer_size_bytes = 0 (explicit zero; wrapper present, value=default→not emitted;
	// hits the flush-every-entry path since size threshold always fires)
	f.Add([]byte("\x0a\x02\x22\x00"))
	// buffer_size_bytes = 0xffffffff (huge uint32 max; varint \xff\xff\xff\xff\x0f)
	f.Add([]byte("\x0a\x08\x22\x06\x08\xff\xff\xff\xff\x0f"))

	// buffer_flush_interval = 1 ns (sub-millisecond; Duration.nanos=1 hits the
	// <=0 coercion guard since AsDuration() > 0 — but the interval is positive,
	// so it passes through verbatim and exercises the non-coercion arm)
	f.Add([]byte("\x0a\x04\x1a\x02\x10\x01"))
	// buffer_flush_interval = 86400 s (24 h, very long; seconds varint \x80\xa3\x05)
	f.Add([]byte("\x0a\x06\x1a\x04\x08\x80\xa3\x05"))

	// Combined: buffer_size_bytes = 16384 (default), buffer_flush_interval = 1 s
	// (default) both explicitly set — exercises the "explicit defaults" code path.
	// 16384 as varint = \x80\x80\x01; 1s Duration.seconds = 1 → varint \x01.
	f.Add([]byte("\x0a\x0a\x1a\x02\x08\x01\x22\x04\x08\x80\x80\x01"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Synthesize a *anypb.Any with the gRPC-ALS TypeURL so the arm is reached.
		a := &anypb.Any{TypeUrl: httpGrpcAccessLogTypeURL, Value: data}
		result := &Bootstrap{}
		// parseGrpcAccessLog MUST NOT panic regardless of data content.
		_ = parseGrpcAccessLog(a, 0, result)
	})
}
