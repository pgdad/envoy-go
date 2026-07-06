package compressor

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzCompressorConfigParse fuzzes arbitrary byte sequences as the typed_config
// payload to New. Asserts: New returns either (factory, nil) OR (nil, error);
// never panics; never returns (nil, nil); on success the factory invocation
// also does not panic and yields a both-sides HTTPFilter. Per ADR-0018's
// "every parser/codec/filter ships a fuzzer" + SPEC §14.3.
//
// 18th fuzzer in the repo (post phase-13's 17th FuzzBufferConfigParse).
// Seed corpus: 8 valid-config seeds (gzip default; explicit min_content_length;
// explicit content_type list; disable_on_etag_header=true;
// remove_accept_encoding_header=true; BEST_SPEED level; HUFFMAN_ONLY strategy;
// listener-default with empty ResponseDirectionConfig) + 4 invalid-config
// seeds (empty bytes; single garbage byte; printable-string garbage;
// short proto-wire-format bytes). The fuzz engine derives further inputs
// from these seeds at the 30s budget per ADR-0018 short-mode CI policy.
func FuzzCompressorConfigParse(f *testing.F) {
	// Helper: wrap a Gzip codec proto in a compressor_library TypedExtensionConfig.
	gzipLibrary := func(g *gzipv3.Gzip, name string) *corev3.TypedExtensionConfig {
		any, err := anypb.New(g)
		if err != nil {
			f.Fatalf("seed gzipLibrary anypb.New: %v", err)
		}
		return &corev3.TypedExtensionConfig{Name: name, TypedConfig: any}
	}

	// 8 valid-config seeds — each is a well-formed Compressor proto that New
	// must accept (factory!=nil, err==nil).
	validSeeds := []*compressorv3.Compressor{
		// (a) default-everything: gzip library, no ResponseDirectionConfig.
		{CompressorLibrary: gzipLibrary(&gzipv3.Gzip{}, "lib_a")},
		// (b) explicit min_content_length=128.
		{
			CompressorLibrary: gzipLibrary(&gzipv3.Gzip{}, "lib_b"),
			ResponseDirectionConfig: &compressorv3.Compressor_ResponseDirectionConfig{
				CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
					MinContentLength: wrapperspb.UInt32(128),
				},
			},
		},
		// (c) explicit content_type list.
		{
			CompressorLibrary: gzipLibrary(&gzipv3.Gzip{}, "lib_c"),
			ResponseDirectionConfig: &compressorv3.Compressor_ResponseDirectionConfig{
				CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
					ContentType: []string{"text/markdown", "application/xml"},
				},
			},
		},
		// (d) disable_on_etag_header=true.
		{
			CompressorLibrary: gzipLibrary(&gzipv3.Gzip{}, "lib_d"),
			ResponseDirectionConfig: &compressorv3.Compressor_ResponseDirectionConfig{
				DisableOnEtagHeader: true,
			},
		},
		// (e) remove_accept_encoding_header=true.
		{
			CompressorLibrary: gzipLibrary(&gzipv3.Gzip{}, "lib_e"),
			ResponseDirectionConfig: &compressorv3.Compressor_ResponseDirectionConfig{
				RemoveAcceptEncodingHeader: true,
			},
		},
		// (f) BEST_SPEED compression level.
		{CompressorLibrary: gzipLibrary(&gzipv3.Gzip{CompressionLevel: gzipv3.Gzip_BEST_SPEED}, "lib_f")},
		// (g) HUFFMAN_ONLY compression strategy.
		{CompressorLibrary: gzipLibrary(&gzipv3.Gzip{CompressionStrategy: gzipv3.Gzip_HUFFMAN_ONLY}, "lib_g")},
		// (h) listener-default with empty ResponseDirectionConfig (all defaults).
		{
			CompressorLibrary:       gzipLibrary(&gzipv3.Gzip{}, "lib_h"),
			ResponseDirectionConfig: &compressorv3.Compressor_ResponseDirectionConfig{},
		},
	}
	for i, s := range validSeeds {
		raw, err := proto.Marshal(s)
		if err != nil {
			f.Fatalf("valid seed[%d] marshal: %v", i, err)
		}
		f.Add(raw)
	}

	// 4 invalid-config seeds — malformed proto bytes that must yield (nil, err)
	// without panic. Each exercises a different parse-rejection path.
	f.Add([]byte{})                    // empty bytes: Unmarshal succeeds with empty Compressor → compressor_library required.
	f.Add([]byte{0xff})                // single garbage byte: proto-wire decode failure.
	f.Add([]byte("not-a-valid-proto")) // printable-string garbage: proto-wire decode failure.
	f.Add([]byte{0x08, 0x01})          // short proto-wire-format bytes: decode may succeed with no compressor_library.

	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry) per PLAN Task 9 Step 1 + the
		// phase-13 buffer / phase-12 csrf precedent: this fuzzer targets the
		// typed_config Any-unmarshal pipeline (parse-rejection contract), not
		// the 17-counter stats-registration path. newFilterStats short-circuits
		// on reg==nil per ADR-0085 nil-tolerance, so the stats path is bypassed
		// regardless of the fuzzed libraryName value.
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(anyMsg, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation; len(raw)=%d", len(raw))
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, err) — invariant violation: %v", err)
		}
		if factory != nil {
			// Successful parse → factory invocation must not panic and must
			// produce a both-sides HTTPFilter per ADR-0129 §Decision (iv).
			hf := factory()
			if hf.Decoder == nil || hf.Encoder == nil {
				t.Fatalf("expected both-sides HTTPFilter (Decoder!=nil && Encoder!=nil), got %+v", hf)
			}
		}
	})
}
