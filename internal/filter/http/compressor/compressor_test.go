package compressor

import (
	"bytes"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// --- Group 1: New factory + buildCompiledConfig (PGV-mirror discipline) ---

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, freshFactoryCtx())
	if err == nil {
		t.Fatal("expected error on nil typed_config")
	}
	if !strings.Contains(err.Error(), "compressor:") {
		t.Errorf("error wording missing 'compressor:' prefix: %v", err)
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte("not-a-valid-proto")}
	_, err := New(bad, freshFactoryCtx())
	if err == nil {
		t.Fatal("expected error on malformed typed_config")
	}
	if !strings.Contains(err.Error(), "invalid typed_config") {
		t.Errorf("error wording missing 'invalid typed_config': %v", err)
	}
}

func TestNew_MissingCompressorLibrary_Rejects(t *testing.T) {
	// PGV-mirror: compressor_library is required (Envoy proto required constraint).
	cfg := &compressorv3.Compressor{} // CompressorLibrary nil
	any := mustAny(t, cfg)
	_, err := New(any, freshFactoryCtx())
	if err == nil || !strings.Contains(err.Error(), "compressor_library is required") {
		t.Errorf("expected 'compressor_library is required' error, got: %v", err)
	}
}

func TestNew_NonGzipLibrary_Rejects(t *testing.T) {
	// Per ADR-0130 §Decision (iii): envoy-go-only parse-rejection of non-Gzip TypeURL.
	bogusAny := &anypb.Any{
		TypeUrl: "type.googleapis.com/envoy.extensions.compression.brotli.compressor.v3.Brotli",
		Value:   nil,
	}
	cfg := &compressorv3.Compressor{
		CompressorLibrary: &corev3.TypedExtensionConfig{
			Name:        "brotli",
			TypedConfig: bogusAny,
		},
	}
	any := mustAny(t, cfg)
	_, err := New(any, freshFactoryCtx())
	if err == nil || !strings.Contains(err.Error(), "unsupported compressor_library TypeURL") {
		t.Errorf("expected 'unsupported compressor_library TypeURL' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "envoy.extensions.compression.gzip.compressor.v3.Gzip") {
		t.Errorf("error should name the Gzip TypeURL as the supported one, got: %v", err)
	}
}

func TestNew_LibraryTypedConfigNil_Rejects(t *testing.T) {
	// A library entry with no typed_config carries no information; reject.
	cfg := &compressorv3.Compressor{
		CompressorLibrary: &corev3.TypedExtensionConfig{
			Name:        "text_optimized",
			TypedConfig: nil,
		},
	}
	any := mustAny(t, cfg)
	_, err := New(any, freshFactoryCtx())
	if err == nil {
		t.Fatal("expected error on missing typed_config on compressor_library")
	}
}

func TestNew_GzipDefault_HappyPath(t *testing.T) {
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, nil, "text_optimized")
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	hf := factory()
	if hf.Decoder == nil || hf.Encoder == nil {
		t.Errorf("expected both-sides HTTPFilter (Decoder!=nil && Encoder!=nil), got %+v", hf)
	}
	// Per ADR-0129 §Decision (iv): SAME *filter instance on both sides.
	// (Compare via interface{} since Decoder/Encoder have different static
	// interface types but should hold the same concrete pointer.)
	if any(hf.Decoder) != any(hf.Encoder) {
		t.Errorf("expected Decoder == Encoder (same *filter instance), got distinct values")
	}
	if hf.Name != filterName {
		t.Errorf("expected filter name %q; got %q", filterName, hf.Name)
	}
}

func TestNew_DefaultContentTypes_Populated(t *testing.T) {
	// Per SPEC §11.1 / ADR-0130: empty/unset content_type → 8-entry default list.
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, nil, "text_optimized")
	any := mustAny(t, cfg)
	cc, _, err := buildFromAny(any)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cc.contentTypes) != 8 {
		t.Fatalf("expected 8 default content-types; got %d (%v)", len(cc.contentTypes), cc.contentTypes)
	}
	want := []string{
		"application/javascript",
		"application/json",
		"application/xhtml+xml",
		"image/svg+xml",
		"text/css",
		"text/html",
		"text/plain",
		"text/xml",
	}
	for i, w := range want {
		if cc.contentTypes[i] != w {
			t.Errorf("default contentTypes[%d] = %q; want %q", i, cc.contentTypes[i], w)
		}
	}
}

func TestNew_CustomContentType_Replaces(t *testing.T) {
	// Per SPEC §11.1: non-empty list replaces the default wholesale.
	rdc := &compressorv3.Compressor_ResponseDirectionConfig{
		CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
			ContentType: []string{"text/markdown", "application/xml"},
		},
	}
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cc.contentTypes) != 2 {
		t.Fatalf("expected 2 content-types; got %d (%v)", len(cc.contentTypes), cc.contentTypes)
	}
	if cc.contentTypes[0] != "text/markdown" || cc.contentTypes[1] != "application/xml" {
		t.Errorf("custom contentTypes mismatch: %v", cc.contentTypes)
	}
}

func TestNew_DefaultMinContentLength_30(t *testing.T) {
	// Per SPEC §11.9 / ADR-0130: min_content_length defaults to 30 when unset.
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, nil, "text_optimized")
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.minContentLength != 30 {
		t.Errorf("expected default minContentLength=30; got %d", cc.minContentLength)
	}
}

func TestNew_CustomMinContentLength_Honored(t *testing.T) {
	rdc := &compressorv3.Compressor_ResponseDirectionConfig{
		CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
			MinContentLength: wrapperspb.UInt32(128),
		},
	}
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.minContentLength != 128 {
		t.Errorf("expected minContentLength=128; got %d", cc.minContentLength)
	}
}

func TestNew_DisableOnEtag_Honored(t *testing.T) {
	rdc := &compressorv3.Compressor_ResponseDirectionConfig{
		DisableOnEtagHeader: true,
	}
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cc.disableOnEtagHeader {
		t.Error("expected disableOnEtagHeader=true")
	}
}

func TestNew_RemoveAcceptEncodingHeader_Honored(t *testing.T) {
	rdc := &compressorv3.Compressor_ResponseDirectionConfig{
		RemoveAcceptEncodingHeader: true,
	}
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cc.removeAcceptEncodingHeader {
		t.Error("expected removeAcceptEncodingHeader=true")
	}
}

func TestNew_EnabledBoolValue_OptionalAtParse(t *testing.T) {
	// Per SPEC §11.3 / §1.1 amendment 2: response_direction_config.common_config.enabled
	// is OPTIONAL at parse-time (BoolValue default; SILENT-IGNORED at runtime).
	// Both true and false should accept without error.
	for _, v := range []bool{true, false} {
		rdc := &compressorv3.Compressor_ResponseDirectionConfig{
			CommonConfig: &compressorv3.Compressor_CommonDirectionConfig{
				Enabled: &corev3.RuntimeFeatureFlag{
					DefaultValue: wrapperspb.Bool(v),
					RuntimeKey:   "compressor.enabled",
				},
			},
		}
		cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
		_, err := New(mustAny(t, cfg), freshFactoryCtx())
		if err != nil {
			t.Errorf("expected New to accept enabled.default_value=%v; got error: %v", v, err)
		}
	}
}

func TestNew_DeprecatedTopLevelMirrors_SilentIgnored(t *testing.T) {
	// Per SPEC §4.2 / D6: top-level mirrors (content_length, content_type,
	// disable_on_etag_header, remove_accept_encoding_header) are SILENT-IGNORED
	// when only response_direction_config carries the active values.
	cfg := &compressorv3.Compressor{
		// All four deprecated top-level mirrors set; should be ignored.
		ContentLength:              wrapperspb.UInt32(9999),
		ContentType:                []string{"deprecated/bogus"},
		DisableOnEtagHeader:        true,
		RemoveAcceptEncodingHeader: true,
		CompressorLibrary: &corev3.TypedExtensionConfig{
			Name:        "text_optimized",
			TypedConfig: mustAny(t, &gzipv3.Gzip{}),
		},
		// No response_direction_config — listener-level defaults apply.
	}
	cc, _, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Defaults survive — top-level mirrors NOT projected onto compiledConfig.
	if cc.minContentLength != 30 {
		t.Errorf("expected default minContentLength=30; got %d (top-level mirror leaked)", cc.minContentLength)
	}
	if len(cc.contentTypes) != 8 {
		t.Errorf("expected default 8-entry contentTypes; got %d (top-level mirror leaked)", len(cc.contentTypes))
	}
	if cc.disableOnEtagHeader {
		t.Error("expected disableOnEtagHeader=false; top-level mirror leaked")
	}
	if cc.removeAcceptEncodingHeader {
		t.Error("expected removeAcceptEncodingHeader=false; top-level mirror leaked")
	}
}

func TestNew_LibraryName_PreservedFromTypedExtensionConfig(t *testing.T) {
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, nil, "text_optimized")
	cc, libName, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if libName != "text_optimized" {
		t.Errorf("expected libraryName='text_optimized'; got %q", libName)
	}
	if cc.libraryName != "text_optimized" {
		t.Errorf("expected compiledConfig.libraryName='text_optimized'; got %q", cc.libraryName)
	}
}

func TestNew_LibraryName_EmptyAllowed(t *testing.T) {
	// Per SPEC §6.2 + D5: empty library name is permitted (Envoy mirror;
	// produces consecutive-dots in stat namespace which SN2 flattens).
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, nil, "")
	cc, libName, err := buildFromAny(mustAny(t, cfg))
	if err != nil {
		t.Fatalf("unexpected error on empty library name: %v", err)
	}
	if libName != "" || cc.libraryName != "" {
		t.Errorf("expected empty libraryName; got %q / %q", libName, cc.libraryName)
	}
}

// --- Group 2: parsePerRoute (TPFC oneof discipline + per-route narrow surface) ---

func TestParsePerRoute_Disabled_Parses(t *testing.T) {
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Disabled{Disabled: true},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cpr.disabled {
		t.Error("expected disabled=true")
	}
	if cpr.removeAcceptEncodingHeaderOverride != nil {
		t.Errorf("expected removeAcceptEncodingHeaderOverride=nil on disabled path; got %v", *cpr.removeAcceptEncodingHeaderOverride)
	}
}

func TestParsePerRoute_DisabledFalse_Rejects(t *testing.T) {
	// PGV bool.const = true rejects disabled: false at proto-decode time;
	// envoy-go defensively re-checks per ADR-0125 5th canonical discipline.
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Disabled{Disabled: false},
	}
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "disabled must be true") {
		t.Errorf("expected 'disabled must be true' rejection, got: %v", err)
	}
}

func TestParsePerRoute_OverridesRmAE_True_Parses(t *testing.T) {
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{
				ResponseDirectionConfig: &compressorv3.ResponseDirectionOverrides{
					RemoveAcceptEncodingHeader: wrapperspb.Bool(true),
				},
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpr.disabled {
		t.Error("expected disabled=false on overrides path")
	}
	if cpr.removeAcceptEncodingHeaderOverride == nil {
		t.Fatal("expected non-nil removeAcceptEncodingHeaderOverride")
	}
	if !*cpr.removeAcceptEncodingHeaderOverride {
		t.Error("expected removeAcceptEncodingHeaderOverride=true")
	}
}

func TestParsePerRoute_OverridesRmAE_False_Parses(t *testing.T) {
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{
				ResponseDirectionConfig: &compressorv3.ResponseDirectionOverrides{
					RemoveAcceptEncodingHeader: wrapperspb.Bool(false),
				},
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpr.removeAcceptEncodingHeaderOverride == nil || *cpr.removeAcceptEncodingHeaderOverride {
		t.Errorf("expected removeAcceptEncodingHeaderOverride=&false; got %+v", cpr)
	}
}

func TestParsePerRoute_OverridesEmpty_NoopCompiledPerRoute(t *testing.T) {
	// Per SPEC §6.3 / §11.13: empty overrides (no fields set) is a no-op
	// (equivalent to no per-route entry).
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{}, // no response_direction_config
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpr.disabled {
		t.Error("expected disabled=false on empty overrides")
	}
	if cpr.removeAcceptEncodingHeaderOverride != nil {
		t.Errorf("expected nil removeAcceptEncodingHeaderOverride on empty overrides; got %v", *cpr.removeAcceptEncodingHeaderOverride)
	}
}

func TestParsePerRoute_OverridesRDC_Empty_NoopCompiledPerRoute(t *testing.T) {
	// SPEC §11.13: probeB attempted overrides.response_direction_config: {} and
	// Envoy booted successfully — empty RDC override compiles to no-op.
	pr := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{
				ResponseDirectionConfig: &compressorv3.ResponseDirectionOverrides{}, // no rmAE BoolValue
			},
		},
	}
	cpr, err := parsePerRoute(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpr.removeAcceptEncodingHeaderOverride != nil {
		t.Errorf("expected nil removeAcceptEncodingHeaderOverride; got %v", *cpr.removeAcceptEncodingHeaderOverride)
	}
}

func TestParsePerRoute_OneofUnset_Rejects(t *testing.T) {
	pr := &compressorv3.CompressorPerRoute{} // Override nil
	_, err := parsePerRoute(pr)
	if err == nil || !strings.Contains(err.Error(), "override oneof is required") {
		t.Errorf("expected 'override oneof is required' rejection, got: %v", err)
	}
}

func TestParsePerRoute_WrongMessageType_Rejects(t *testing.T) {
	// Defensive: parsePerRoute receives proto.Message; non-CompressorPerRoute → error.
	_, err := parsePerRoute(&compressorv3.Compressor{}) // wrong type
	if err == nil || !strings.Contains(err.Error(), "expected *CompressorPerRoute") {
		t.Errorf("expected type-assert rejection, got: %v", err)
	}
}

// --- Group 3: buildCompiledGzipConfig (compression-level + strategy + silent-ignored fields) ---

func TestBuildGzipConfig_NilGzip_Defaults(t *testing.T) {
	cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DEFAULT_COMPRESSION (enum 0) maps to Go gzip.DefaultCompression = -1.
	if cfg.level != -1 {
		t.Errorf("expected level=-1 (default); got %d", cfg.level)
	}
	if cfg.huffmanOnly {
		t.Error("expected huffmanOnly=false on default strategy")
	}
}

func TestBuildGzipConfig_LevelMapping(t *testing.T) {
	// Per ADR-0130 §Decision (iv) mapping table verbatim.
	cases := []struct {
		name        string
		enumLevel   gzipv3.Gzip_CompressionLevel
		wantNumeric int
	}{
		{"DEFAULT_COMPRESSION", gzipv3.Gzip_DEFAULT_COMPRESSION, -1},
		{"BEST_SPEED", gzipv3.Gzip_BEST_SPEED, 1}, // aliases COMPRESSION_LEVEL_1
		{"COMPRESSION_LEVEL_2", gzipv3.Gzip_COMPRESSION_LEVEL_2, 2},
		{"COMPRESSION_LEVEL_3", gzipv3.Gzip_COMPRESSION_LEVEL_3, 3},
		{"COMPRESSION_LEVEL_4", gzipv3.Gzip_COMPRESSION_LEVEL_4, 4},
		{"COMPRESSION_LEVEL_5", gzipv3.Gzip_COMPRESSION_LEVEL_5, 5},
		{"COMPRESSION_LEVEL_6", gzipv3.Gzip_COMPRESSION_LEVEL_6, 6},
		{"COMPRESSION_LEVEL_7", gzipv3.Gzip_COMPRESSION_LEVEL_7, 7},
		{"COMPRESSION_LEVEL_8", gzipv3.Gzip_COMPRESSION_LEVEL_8, 8},
		{"BEST_COMPRESSION", gzipv3.Gzip_BEST_COMPRESSION, 9}, // aliases COMPRESSION_LEVEL_9
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{CompressionLevel: tc.enumLevel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.level != tc.wantNumeric {
				t.Errorf("for %s: expected level=%d; got %d", tc.name, tc.wantNumeric, cfg.level)
			}
		})
	}
}

func TestBuildGzipConfig_StrategyMapping(t *testing.T) {
	// Per ADR-0130 §Decision (v): only HUFFMAN_ONLY is honored; others collapse to default.
	cases := []struct {
		name        string
		strategy    gzipv3.Gzip_CompressionStrategy
		wantHuffman bool
	}{
		{"DEFAULT_STRATEGY", gzipv3.Gzip_DEFAULT_STRATEGY, false},
		{"FILTERED", gzipv3.Gzip_FILTERED, false},
		{"HUFFMAN_ONLY", gzipv3.Gzip_HUFFMAN_ONLY, true},
		{"RLE", gzipv3.Gzip_RLE, false},
		{"FIXED", gzipv3.Gzip_FIXED, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{CompressionStrategy: tc.strategy})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.huffmanOnly != tc.wantHuffman {
				t.Errorf("for %s: expected huffmanOnly=%v; got %v", tc.name, tc.wantHuffman, cfg.huffmanOnly)
			}
		})
	}
}

func TestBuildGzipConfig_SilentIgnored_MemoryLevel(t *testing.T) {
	// Per ADR-0130 §Consequences + Decision (i): memory_level NOT stored on compiledGzipConfig;
	// parse accepts without error.
	cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{
		MemoryLevel: wrapperspb.UInt32(9),
	})
	if err != nil {
		t.Fatalf("unexpected error on memory_level=9: %v", err)
	}
	if cfg.level != -1 || cfg.huffmanOnly {
		t.Errorf("expected defaults preserved; got level=%d huffmanOnly=%v", cfg.level, cfg.huffmanOnly)
	}
}

func TestBuildGzipConfig_SilentIgnored_WindowBits(t *testing.T) {
	cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{
		WindowBits: wrapperspb.UInt32(15),
	})
	if err != nil {
		t.Fatalf("unexpected error on window_bits=15: %v", err)
	}
	if cfg.level != -1 || cfg.huffmanOnly {
		t.Errorf("expected defaults preserved; got level=%d huffmanOnly=%v", cfg.level, cfg.huffmanOnly)
	}
}

func TestBuildGzipConfig_SilentIgnored_ChunkSize(t *testing.T) {
	cfg, err := buildCompiledGzipConfig(&gzipv3.Gzip{
		ChunkSize: wrapperspb.UInt32(8192),
	})
	if err != nil {
		t.Fatalf("unexpected error on chunk_size=8192: %v", err)
	}
	if cfg.level != -1 || cfg.huffmanOnly {
		t.Errorf("expected defaults preserved; got level=%d huffmanOnly=%v", cfg.level, cfg.huffmanOnly)
	}
}

func TestUnmarshalCompressorLibrary_NilLibrary_Rejects(t *testing.T) {
	_, _, err := unmarshalCompressorLibrary(nil)
	if err == nil || !strings.Contains(err.Error(), "compressor_library is required") {
		t.Errorf("expected 'compressor_library is required'; got %v", err)
	}
}

func TestUnmarshalCompressorLibrary_GzipTypeURL_Dispatches(t *testing.T) {
	lib := &corev3.TypedExtensionConfig{
		Name:        "text_optimized",
		TypedConfig: mustAny(t, &gzipv3.Gzip{CompressionLevel: gzipv3.Gzip_BEST_COMPRESSION}),
	}
	gzipCfg, name, err := unmarshalCompressorLibrary(lib)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "text_optimized" {
		t.Errorf("expected name=text_optimized; got %q", name)
	}
	if gzipCfg.level != 9 {
		t.Errorf("expected level=9 (BEST_COMPRESSION); got %d", gzipCfg.level)
	}
}

// --- Group 4: Accept-Encoding parser (per SPEC §6.4 + §11.4 + §11.5 verbatim
// probeA evidence + RFC 7231 §5.3.4). The 6 classification tokens map verbatim
// to the 6 header_* counter names (compressor_used, overshadowed, identity,
// wildcard, no_accept_header, not_valid). ---

func TestParseAcceptEncoding_Empty(t *testing.T) {
	enc, cls := parseAcceptEncoding("")
	if enc != "" || cls != "no_accept_header" {
		t.Errorf("expected (\"\", \"no_accept_header\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_WhitespaceOnly(t *testing.T) {
	// Per §6.4 absent + per RFC 7231 §5.3.4 "no field" semantics.
	enc, cls := parseAcceptEncoding("   ")
	if enc != "" || cls != "no_accept_header" {
		t.Errorf("expected (\"\", \"no_accept_header\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipExplicit(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipExplicitWithQ1(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=1.0")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipExplicitWithQ05(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=0.5")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_Identity(t *testing.T) {
	// Per §6.4: "Only identity → ('', 'identity')".
	enc, cls := parseAcceptEncoding("identity")
	if enc != "" || cls != "identity" {
		t.Errorf("expected (\"\", \"identity\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_Wildcard(t *testing.T) {
	// Per §6.4: "Only `*` → ('gzip', 'wildcard') if gzip is configured codec".
	enc, cls := parseAcceptEncoding("*")
	if enc != "gzip" || cls != "wildcard" {
		t.Errorf("expected (\"gzip\", \"wildcard\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_WildcardWithQ(t *testing.T) {
	enc, cls := parseAcceptEncoding("*;q=0.5")
	if enc != "gzip" || cls != "wildcard" {
		t.Errorf("expected (\"gzip\", \"wildcard\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_MultiCodingSortedByQValue(t *testing.T) {
	// Per §11.4 verbatim probeA: "gzip;q=0.5, br;q=1.0" — br has higher q but
	// is NOT configured (gzip-only MVP); gzip wins via fall-through.
	enc, cls := parseAcceptEncoding("gzip;q=0.5, br;q=1.0")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipQ0_Blocks(t *testing.T) {
	// Per §11.4(b): "gzip;q=0 → no gzip selected; identity selected (default
	// fallback)". `enc == ""` is load-bearing; cls is "identity" per the
	// default-fallback semantics.
	enc, cls := parseAcceptEncoding("gzip;q=0")
	if enc != "" {
		t.Errorf("expected blocked gzip on q=0; got %q", enc)
	}
	if cls != "identity" {
		t.Errorf("expected cls=identity (default fallback per §11.4(b)); got %q", cls)
	}
}

func TestParseAcceptEncoding_MalformedQValue_NotValid(t *testing.T) {
	// Per §11.4(d) verbatim: "gzip;q=blah → AE parse error; no coding selected;
	// identity fallback. Increments header_not_valid +1".
	enc, cls := parseAcceptEncoding("gzip;q=blah")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_OutOfRangeQValue_NotValid(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=2.0")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\") on q>1; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_NegativeQValue_NotValid(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=-0.1")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\") on q<0; got (%q, %q)", enc, cls)
	}
}

// q=NaN and q=Inf slip past strconv.ParseFloat but are outside RFC 7231 §5.3.1
// strict-decimal grammar; the parser MUST reject them as malformed per the
// classification taxonomy.
func TestParseAcceptEncoding_NaNQValue_NotValid(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=NaN")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\") on q=NaN; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_InfQValue_NotValid(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;q=+Inf")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\") on q=+Inf; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_EmptyTokenInList_NotValid(t *testing.T) {
	// "gzip,,br" → middle token is empty.
	enc, cls := parseAcceptEncoding("gzip,,br")
	if enc != "" || cls != "not_valid" {
		t.Errorf("expected (\"\", \"not_valid\") on empty token; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_BrOnly_Overshadowed(t *testing.T) {
	// Per §6.4: "Only known codings unconfigured (e.g., br with gzip-only MVP) →
	// ('', 'overshadowed') — codings present but not selectable".
	enc, cls := parseAcceptEncoding("br")
	if enc != "" || cls != "overshadowed" {
		t.Errorf("expected (\"\", \"overshadowed\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_DeflateOnly_Overshadowed(t *testing.T) {
	enc, cls := parseAcceptEncoding("deflate")
	if enc != "" || cls != "overshadowed" {
		t.Errorf("expected (\"\", \"overshadowed\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_BrAndDeflate_Overshadowed(t *testing.T) {
	// Multiple unconfigured codings; no gzip / wildcard / identity.
	enc, cls := parseAcceptEncoding("br;q=0.8, deflate;q=0.5")
	if enc != "" || cls != "overshadowed" {
		t.Errorf("expected (\"\", \"overshadowed\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_CaseInsensitiveToken(t *testing.T) {
	enc, cls := parseAcceptEncoding("GZIP")
	if enc != "gzip" {
		t.Errorf("expected case-insensitive token match (gzip); got %q", enc)
	}
	if cls != "compressor_used" {
		t.Errorf("expected cls=compressor_used; got %q", cls)
	}
}

func TestParseAcceptEncoding_CaseInsensitiveMixedCase(t *testing.T) {
	enc, _ := parseAcceptEncoding("GzIp;Q=0.5")
	if enc != "gzip" {
		t.Errorf("expected case-insensitive token+param match; got %q", enc)
	}
}

func TestParseAcceptEncoding_WhitespaceTolerance(t *testing.T) {
	enc, cls := parseAcceptEncoding("  gzip  ; q=0.5  ,  br  ; q=1.0  ")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected whitespace-tolerant parsing; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_TrailingSemicolon(t *testing.T) {
	enc, cls := parseAcceptEncoding("gzip;")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected trailing-semicolon tolerance; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_MultipleEntriesSameCoding(t *testing.T) {
	// "gzip, gzip;q=0.5" — first entry has implicit q=1.0, wins by q-value desc.
	enc, cls := parseAcceptEncoding("gzip, gzip;q=0.5")
	if enc != "gzip" {
		t.Errorf("expected gzip selection; got %q", enc)
	}
	if cls != "compressor_used" {
		t.Errorf("expected cls=compressor_used; got %q", cls)
	}
}

func TestParseAcceptEncoding_GzipAndWildcard_GzipWins(t *testing.T) {
	// Explicit gzip outranks wildcard at the same q-value (declared order
	// tie-break for ties; explicit-gzip preference for selectability).
	enc, cls := parseAcceptEncoding("gzip, *")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected explicit gzip wins over wildcard; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_IdentityHigherQThanGzip_IdentityWins(t *testing.T) {
	// "identity;q=1.0, gzip;q=0.5" — identity outranks gzip by q-value desc.
	enc, cls := parseAcceptEncoding("identity;q=1.0, gzip;q=0.5")
	if enc != "" || cls != "identity" {
		t.Errorf("expected identity wins; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipHigherQThanIdentity_GzipWins(t *testing.T) {
	// "gzip;q=1.0, identity;q=0.5" — gzip outranks identity.
	enc, cls := parseAcceptEncoding("gzip;q=1.0, identity;q=0.5")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected gzip wins; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_GzipQ0_AndBr_Overshadowed(t *testing.T) {
	// "gzip;q=0, br;q=0.5" — gzip blocked; br unconfigured; no selectable
	// configured codec; classification overshadowed (recognized non-configured
	// codec present with q>0).
	enc, cls := parseAcceptEncoding("gzip;q=0, br;q=0.5")
	if enc != "" || cls != "overshadowed" {
		t.Errorf("expected (\"\", \"overshadowed\"); got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_QValueWithThreeDecimals(t *testing.T) {
	// RFC 7231 §5.3.4 admits up to 3 decimal places.
	enc, cls := parseAcceptEncoding("gzip;q=0.123")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected (\"gzip\", \"compressor_used\") on q=0.123; got (%q, %q)", enc, cls)
	}
}

func TestParseAcceptEncoding_NonQParameterIgnored(t *testing.T) {
	// Per RFC 7231, non-q parameters on Accept-Encoding are not standardized;
	// the parser ignores unknown parameters (only q= matters).
	enc, cls := parseAcceptEncoding("gzip;foo=bar")
	if enc != "gzip" || cls != "compressor_used" {
		t.Errorf("expected non-q param ignored; got (%q, %q)", enc, cls)
	}
}

// --- Group 5a: DecodeHeaders + DecodeData + DecodeTrailers (per SPEC §6.4 + §6.5).
// Decode-side surface lands at Task 5: AE-parse + per-route resolve + maybe-strip-AE +
// pass-through DataContinue/TrailersContinue. ---

func TestDecodeHeaders_NoAE_StoresEmptyEncoding_ContinueNoAEStrip(t *testing.T) {
	// Empty AE → classification "no_accept_header"; rmAE off by default → no strip.
	f := freshDecodeFilter(t, false, nil)
	h := http.Header{}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if f.acceptedEncoding != "" {
		t.Errorf("expected empty acceptedEncoding; got %q", f.acceptedEncoding)
	}
	if f.acceptHeaderClassification != "no_accept_header" {
		t.Errorf("expected classification=no_accept_header; got %q", f.acceptHeaderClassification)
	}
	if f.passthrough {
		t.Error("expected passthrough=false on no per-route disabled")
	}
	if v := h.Get("Accept-Encoding"); v != "" {
		t.Errorf("expected AE absent (none to strip); got %q", v)
	}
}

func TestDecodeHeaders_GzipAE_StoresGzip_Continue(t *testing.T) {
	// AE: gzip; classification "compressor_used"; listener rmAE default false → AE NOT stripped.
	f := freshDecodeFilter(t, false, nil)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if f.acceptedEncoding != "gzip" {
		t.Errorf("expected acceptedEncoding=gzip; got %q", f.acceptedEncoding)
	}
	if f.acceptHeaderClassification != "compressor_used" {
		t.Errorf("expected classification=compressor_used; got %q", f.acceptHeaderClassification)
	}
	if v := h.Get("Accept-Encoding"); v != "gzip" {
		t.Errorf("expected AE preserved (rmAE=false default); got %q", v)
	}
}

func TestDecodeHeaders_PerRouteDisabled_PassthroughTrue_NoAEStrip_Continue(t *testing.T) {
	// Disabled-route bypass is wholly inactive per SPEC §5.5 / ADR-0125 amendment §(viii):
	// no AE strip even when listener-level rmAE=true.
	perRoute := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Disabled{Disabled: true},
	}
	f := freshDecodeFilter(t, true /* listener rmAE=true */, perRoute)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if !f.passthrough {
		t.Error("expected passthrough=true on per-route disabled")
	}
	if f.perRoute == nil || !f.perRoute.disabled {
		t.Errorf("expected f.perRoute.disabled=true; got %+v", f.perRoute)
	}
	if v := h.Get("Accept-Encoding"); v != "gzip" {
		t.Errorf("expected AE preserved on disabled-route (wholly inactive); got %q", v)
	}
}

func TestDecodeHeaders_ListenerLevelRmAE_True_StripsAE(t *testing.T) {
	// Listener rmAE=true; no per-route override → strip AE from request.
	f := freshDecodeFilter(t, true /* listener rmAE=true */, nil)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := h.Get("Accept-Encoding"); v != "" {
		t.Errorf("expected AE stripped (listener rmAE=true); got %q", v)
	}
	if f.acceptedEncoding != "gzip" {
		t.Errorf("expected acceptedEncoding=gzip (cached BEFORE strip); got %q", f.acceptedEncoding)
	}
}

func TestDecodeHeaders_PerRouteRmAEOverride_True_StripsAE_EvenWhenListenerFalse(t *testing.T) {
	// Listener rmAE=false; per-route override rmAE=true → AE stripped (override wins).
	perRoute := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{
				ResponseDirectionConfig: &compressorv3.ResponseDirectionOverrides{
					RemoveAcceptEncodingHeader: wrapperspb.Bool(true),
				},
			},
		},
	}
	f := freshDecodeFilter(t, false /* listener rmAE=false */, perRoute)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := h.Get("Accept-Encoding"); v != "" {
		t.Errorf("expected AE stripped (per-route override rmAE=true); got %q", v)
	}
}

func TestDecodeHeaders_PerRouteRmAEOverride_False_DoesNotStrip_EvenWhenListenerTrue(t *testing.T) {
	// Listener rmAE=true; per-route override rmAE=false → AE NOT stripped (override wins).
	perRoute := &compressorv3.CompressorPerRoute{
		Override: &compressorv3.CompressorPerRoute_Overrides{
			Overrides: &compressorv3.CompressorOverrides{
				ResponseDirectionConfig: &compressorv3.ResponseDirectionOverrides{
					RemoveAcceptEncodingHeader: wrapperspb.Bool(false),
				},
			},
		},
	}
	f := freshDecodeFilter(t, true /* listener rmAE=true */, perRoute)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := h.Get("Accept-Encoding"); v != "gzip" {
		t.Errorf("expected AE preserved (per-route override rmAE=false wins); got %q", v)
	}
}

func TestDecodeHeaders_NilDcb_NoPerRouteAccess(t *testing.T) {
	// Defensive: when dcb is nil (no callbacks wired yet), DecodeHeaders must not panic;
	// per-route resolution is skipped; listener-level rmAE applies.
	f := freshDecodeFilterNoCB(t, true /* listener rmAE=true */)
	h := http.Header{"Accept-Encoding": []string{"gzip"}}
	status := f.DecodeHeaders(h, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := h.Get("Accept-Encoding"); v != "" {
		t.Errorf("expected AE stripped (listener rmAE=true, no per-route); got %q", v)
	}
	if f.perRoute != nil {
		t.Errorf("expected f.perRoute=nil when dcb is nil; got %+v", f.perRoute)
	}
}

func TestDecodeData_PassThrough_DataContinue(t *testing.T) {
	// DecodeData pass-through unconditionally per SPEC §6.5; compressor never inspects request body.
	f := freshDecodeFilter(t, false, nil)
	for _, endStream := range []bool{false, true} {
		status := f.DecodeData([]byte("anything"), endStream)
		if status != envoyhttp.DataContinue {
			t.Errorf("expected DataContinue (endStream=%v); got %v", endStream, status)
		}
	}
}

func TestDecodeTrailers_PassThrough_TrailersContinue(t *testing.T) {
	// DecodeTrailers pass-through per SPEC §6.5.
	f := freshDecodeFilter(t, false, nil)
	status := f.DecodeTrailers(http.Header{"X-Foo": []string{"bar"}})
	if status != envoyhttp.TrailersContinue {
		t.Errorf("expected TrailersContinue; got %v", status)
	}
}

func TestSetDecoderCallbacks_StoresOnSameFilter(t *testing.T) {
	// Per PROGRESS planner-time decision 10: SetDecoderCallbacks stores on the
	// SAME *filter that services the encode side.
	f := freshDecodeFilterNoCB(t, false)
	cb := &fakeCallbacks{}
	f.SetDecoderCallbacks(cb)
	if f.dcb != cb {
		t.Errorf("expected f.dcb == cb; got %v", f.dcb)
	}
}

func TestSetEncoderCallbacks_StoresOnSameFilter(t *testing.T) {
	// Per PROGRESS planner-time decision 10: both SetDecoderCallbacks and
	// SetEncoderCallbacks store on the SAME *filter instance.
	f := freshDecodeFilterNoCB(t, false)
	cb := &fakeCallbacks{}
	f.SetEncoderCallbacks(cb)
	if f.ecb != cb {
		t.Errorf("expected f.ecb == cb; got %v", f.ecb)
	}
}

func TestOnDestroy_NoOp(t *testing.T) {
	// OnDestroy is a no-op (no per-stream resources beyond the *filter itself);
	// safety property: must not panic on freshly-constructed or stripped filter.
	f := freshDecodeFilterNoCB(t, false)
	f.OnDestroy() // no panic == pass
}

// --- Helpers ---

// mustAny wraps proto.Message into anypb.Any or fails the test.
func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// freshFactoryCtx builds a per-test FactoryCtx with a fresh stats registry +
// non-empty stat_prefix so the 17-counter registration in New doesn't fail
// when other tests share state.
func freshFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{
		Stats:      stats.NewRegistry(),
		StatPrefix: "test_p14",
	}
}

// newGzipCompressor builds a Compressor proto with the given Gzip codec config
// wrapped in a compressor_library TypedExtensionConfig under `libName`, plus
// the supplied response_direction_config (nil for "use listener defaults").
func newGzipCompressor(
	t *testing.T,
	gzipCfg *gzipv3.Gzip,
	rdc *compressorv3.Compressor_ResponseDirectionConfig,
	libName string,
) *compressorv3.Compressor {
	t.Helper()
	return &compressorv3.Compressor{
		CompressorLibrary: &corev3.TypedExtensionConfig{
			Name:        libName,
			TypedConfig: mustAny(t, gzipCfg),
		},
		ResponseDirectionConfig: rdc,
	}
}

// buildFromAny is a test convenience that unmarshals Compressor from Any and
// runs the full N.ew-style pipeline (unmarshalCompressorLibrary +
// buildCompiledConfig) without going through the per-request factory closure,
// so Group 1 tests can inspect compiledConfig directly.
func buildFromAny(any *anypb.Any) (*compiledConfig, string, error) {
	var cfg compressorv3.Compressor
	if err := any.UnmarshalTo(&cfg); err != nil {
		return nil, "", err
	}
	gzipCfg, libraryName, err := unmarshalCompressorLibrary(cfg.GetCompressorLibrary())
	if err != nil {
		return nil, "", err
	}
	cc, err := buildCompiledConfig(&cfg, gzipCfg, libraryName)
	if err != nil {
		return nil, "", err
	}
	return cc, libraryName, nil
}

// fakeCallbacks is a minimal test stub for both DecoderFilterCallbacks and
// EncoderFilterCallbacks. perRoute is the raw proto.Message that
// RequestRouteConfig() returns; tests set it to a *compressorv3.CompressorPerRoute
// (or nil for "no per-route entry on this route").
//
// overwriteBodyCalls captures every OverwriteBody invocation (Group 6 tests
// inspect the captured bytes to assert the gzip-encoded payload). Tests that
// only need a no-op stub may ignore this field. overwriteBodyCallCount counts
// invocations (== len(overwriteBodyCalls); kept separate for cheap "called
// exactly N times" assertions without slice introspection).
type fakeCallbacks struct {
	perRoute               proto.Message
	overwriteBodyCalls     [][]byte
	overwriteBodyCallCount int
}

func (c *fakeCallbacks) ContinueDecoding()                                    {}
func (c *fakeCallbacks) ContinueEncoding()                                    {}
func (c *fakeCallbacks) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {}
func (c *fakeCallbacks) RequestRouteConfig() proto.Message                    { return c.perRoute }
func (c *fakeCallbacks) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (c *fakeCallbacks) EncodeHeaders(http.Header, bool) {}
func (c *fakeCallbacks) EncodeData([]byte, bool)         {}
func (c *fakeCallbacks) EncodeTrailers(http.Header)      {}
func (c *fakeCallbacks) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (c *fakeCallbacks) DownstreamRemoteAddr() net.Addr   { return nil }
func (c *fakeCallbacks) DownstreamLocalAddr() net.Addr    { return nil }
func (c *fakeCallbacks) DownstreamTLSServerName() string  { return "" }
func (c *fakeCallbacks) DownstreamTLSPeerCertDER() []byte { return nil }
func (c *fakeCallbacks) DownstreamProtocol() string       { return "" }
func (c *fakeCallbacks) ListenerPrincipal() string        { return "" }

// ADR-0175 callback-surface extension stub (phase-19.2 Task 2 — encode-side
// body-buffering framework primitive). Zero-value return; compressor does
// NOT consume BufferEncodedBody (Path B per ADR-0131 uses OverwriteBody
// for per-call replacement, not the buffer-and-hold primitive).
func (c *fakeCallbacks) BufferEncodedBody() []byte { return nil }

// OverwriteBody captures b (defensive copy — the EncodeData implementation
// passes buf.Bytes() which aliases its bytes.Buffer's internal slice; capturing
// a copy is the safer assertion surface). Increments overwriteBodyCallCount.
func (c *fakeCallbacks) OverwriteBody(b []byte) {
	captured := append([]byte(nil), b...)
	c.overwriteBodyCalls = append(c.overwriteBodyCalls, captured)
	c.overwriteBodyCallCount++
}

// Compile-time check that fakeCallbacks implements both callback interfaces.
var (
	_ envoyhttp.DecoderFilterCallbacks = (*fakeCallbacks)(nil)
	_ envoyhttp.EncoderFilterCallbacks = (*fakeCallbacks)(nil)
)

// freshDecodeFilter constructs a *filter via the real factory pipeline (so
// compiledConfig is initialized realistically), wires it with a fakeCallbacks
// that returns `perRoute` from RequestRouteConfig(), and sets listener-level
// remove_accept_encoding_header per the rmAE flag.
func freshDecodeFilter(t *testing.T, listenerRmAE bool, perRoute proto.Message) *filter {
	t.Helper()
	f := freshDecodeFilterNoCB(t, listenerRmAE)
	cb := &fakeCallbacks{perRoute: perRoute}
	f.SetDecoderCallbacks(cb)
	f.SetEncoderCallbacks(cb)
	return f
}

// freshDecodeFilterNoCB constructs a *filter via the real factory pipeline
// WITHOUT wiring callbacks. Used by tests that want to exercise pre-callback
// paths (nil-dcb safety) and the Set*Callbacks setters directly.
func freshDecodeFilterNoCB(t *testing.T, listenerRmAE bool) *filter {
	t.Helper()
	rdc := &compressorv3.Compressor_ResponseDirectionConfig{
		RemoveAcceptEncodingHeader: listenerRmAE,
	}
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, rdc, "text_optimized")
	factory, err := New(mustAny(t, cfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	f, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("expected hf.Decoder to be *filter; got %T", hf.Decoder)
	}
	return f
}

// --- Group 5: EncodeHeaders skip-decision matrix (per SPEC §6.6 + §11.5 +
// §11.10 + §11.15 + §1.1 amendments 5-6) ---
//
// Group 5 tests drive the 11-bucket skip-decision sequence + Vary trichotomy +
// counter increments + compress-path header mutations. Each test pre-seeds
// f.acceptHeaderClassification + f.acceptedEncoding to bypass the AE-parser
// (Task 3's parseAcceptEncoding is exercised in Group 4); the focus here is on
// the EncodeHeaders dispatch table + skip predicates.

// freshEncodeFilter builds a *filter with allocated stat counters (so tests
// can observe counter increments) + a compiledConfig matching the test
// scenario. listenerCfg is the listener-level config (use newGzipCompressor
// helper); perRouteCfg may be nil. Counters are allocated directly on
// filterStats so tests can call .Load() on each.
func freshEncodeFilter(t *testing.T, cc *compiledConfig, cpr *compiledPerRoute) *filter {
	t.Helper()
	return &filter{
		config:   cc,
		stats:    newTestFilterStats(),
		perRoute: cpr,
	}
}

// newTestFilterStats allocates all 17 counters on a fresh stats.Registry so
// EncodeHeaders / EncodeData increments are observable via .Load(). Mirrors
// the 17-counter shape that Task 8 will land via newFilterStats; this helper
// stays test-local until then.
func newTestFilterStats() *filterStats {
	r := stats.NewRegistry()
	return &filterStats{
		HeaderCompressorOvershadowed:   r.NewCounter("header_compressor_overshadowed"),
		HeaderCompressorUsed:           r.NewCounter("header_compressor_used"),
		HeaderIdentity:                 r.NewCounter("header_identity"),
		HeaderNotValid:                 r.NewCounter("header_not_valid"),
		HeaderWildcard:                 r.NewCounter("header_wildcard"),
		NoAcceptHeader:                 r.NewCounter("no_accept_header"),
		NotCompressedEtag:              r.NewCounter("not_compressed_etag"),
		ResponseCompressed:             r.NewCounter("response_compressed"),
		ResponseContentLengthTooSmall:  r.NewCounter("response_content_length_too_small"),
		ResponseNotCompressed:          r.NewCounter("response_not_compressed"),
		ResponseTotalCompressedBytes:   r.NewCounter("response_total_compressed_bytes"),
		ResponseTotalUncompressedBytes: r.NewCounter("response_total_uncompressed_bytes"),
		RequestCompressed:              r.NewCounter("request_compressed"),
		RequestContentLengthTooSmall:   r.NewCounter("request_content_length_too_small"),
		RequestNotCompressed:           r.NewCounter("request_not_compressed"),
		RequestTotalCompressedBytes:    r.NewCounter("request_total_compressed_bytes"),
		RequestTotalUncompressedBytes:  r.NewCounter("request_total_uncompressed_bytes"),
	}
}

// defaultCompiledConfig builds a minimal compiledConfig with sensible defaults
// for Group 5 tests (gzip default level, default 8-entry content_types,
// min_content_length=30).
func defaultCompiledConfig() *compiledConfig {
	return &compiledConfig{
		libraryName:                 "text_optimized",
		gzip:                        &compiledGzipConfig{level: -1},
		minContentLength:            defaultMinContentLength,
		contentTypes:                defaultContentTypes,
		uncompressibleResponseCodes: map[uint32]struct{}{},
	}
}

// Bucket 0 — passthrough bypass: per-route disabled → no header mutation, no counter.
func TestEncodeHeaders_Bucket0_Passthrough_NoMutation_NoCounter(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.passthrough = true
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":   []string{"text/html"},
		"Content-Length": []string{"1024"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Content-Encoding"); v != "" {
		t.Errorf("expected no Content-Encoding on passthrough; got %q", v)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected no Vary on passthrough; got %q", v)
	}
	if v := headers.Get("Content-Length"); v != "1024" {
		t.Errorf("expected Content-Length preserved on passthrough; got %q", v)
	}
	if f.stats.HeaderCompressorUsed.Load() != 0 {
		t.Errorf("expected no header_compressor_used increment on passthrough; got %d", f.stats.HeaderCompressorUsed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 0 {
		t.Errorf("expected no response_not_compressed increment on passthrough; got %d", f.stats.ResponseNotCompressed.Load())
	}
	if f.willCompress {
		t.Error("expected willCompress=false on passthrough")
	}
}

// Bucket 1 — no_accept_header AE-side skip: Vary INJECTED.
func TestEncodeHeaders_Bucket1_NoAcceptHeader_Skip_VarySet(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "no_accept_header"
	f.acceptedEncoding = ""
	headers := http.Header{
		"Content-Type":   []string{"text/html"},
		"Content-Length": []string{"1024"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding on AE-side skip; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "" {
		t.Errorf("expected no Content-Encoding on skip; got %q", v)
	}
	if f.stats.NoAcceptHeader.Load() != 1 {
		t.Errorf("expected no_accept_header=1; got %d", f.stats.NoAcceptHeader.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
	if f.willCompress {
		t.Error("expected willCompress=false on skip")
	}
}

// Bucket 2 — identity AE-side skip: Vary INJECTED + header_identity counter.
func TestEncodeHeaders_Bucket2_Identity_Skip_VarySet(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "identity"
	f.acceptedEncoding = ""
	headers := http.Header{"Content-Type": []string{"text/html"}}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding; got %q", v)
	}
	if f.stats.HeaderIdentity.Load() != 1 {
		t.Errorf("expected header_identity=1; got %d", f.stats.HeaderIdentity.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 3 — not_valid AE-side skip: Vary INJECTED + header_not_valid counter.
func TestEncodeHeaders_Bucket3_NotValid_Skip_VarySet(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "not_valid"
	f.acceptedEncoding = ""
	headers := http.Header{"Content-Type": []string{"text/html"}}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding; got %q", v)
	}
	if f.stats.HeaderNotValid.Load() != 1 {
		t.Errorf("expected header_not_valid=1; got %d", f.stats.HeaderNotValid.Load())
	}
}

// Bucket 4 — overshadowed AE-side skip (e.g. AE: br with gzip-only): Vary INJECTED +
// header_compressor_overshadowed counter per §11.15 row "AE: br".
func TestEncodeHeaders_Bucket4_Overshadowed_Skip_VarySet(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "overshadowed"
	f.acceptedEncoding = ""
	headers := http.Header{"Content-Type": []string{"text/html"}}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding (AE-side skip per §11.15); got %q", v)
	}
	if f.stats.HeaderCompressorOvershadowed.Load() != 1 {
		t.Errorf("expected header_compressor_overshadowed=1; got %d", f.stats.HeaderCompressorOvershadowed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 5 — uncompressible_status server-side skip: NO Vary.
// (Driven via a status code present in cc.uncompressibleResponseCodes;
// MVP cannot populate this map via proto but the predicate is observable.)
func TestEncodeHeaders_Bucket5_UncompressibleStatus_Skip_NoVary(t *testing.T) {
	cc := defaultCompiledConfig()
	cc.uncompressibleResponseCodes = map[uint32]struct{}{204: {}}
	f := freshEncodeFilter(t, cc, nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		":status":      []string{"204"},
		"Content-Type": []string{"text/html"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on server-side skip; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "" {
		t.Errorf("expected no Content-Encoding; got %q", v)
	}
	if f.stats.HeaderCompressorUsed.Load() != 1 {
		t.Errorf("expected header_compressor_used=1; got %d", f.stats.HeaderCompressorUsed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 6 — already_encoded server-side skip: NO Vary (per §11.11 + §11.15).
func TestEncodeHeaders_Bucket6_AlreadyEncodedGzip_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":     []string{"text/html"},
		"Content-Encoding": []string{"gzip"},
		"Content-Length":   []string{"1024"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on already-encoded server-side skip; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "gzip" {
		t.Errorf("expected Content-Encoding preserved as 'gzip'; got %q", v)
	}
	if v := headers.Get("Content-Length"); v != "1024" {
		t.Errorf("expected Content-Length preserved on skip; got %q", v)
	}
	if f.stats.HeaderCompressorUsed.Load() != 1 {
		t.Errorf("expected header_compressor_used=1; got %d", f.stats.HeaderCompressorUsed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 6 variant — already_encoded deflate: same skip path.
func TestEncodeHeaders_Bucket6_AlreadyEncodedDeflate_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":     []string{"text/html"},
		"Content-Encoding": []string{"deflate"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "deflate" {
		t.Errorf("expected Content-Encoding=deflate preserved; got %q", v)
	}
}

// Bucket 6 variant — already_encoded identity (per §11.11: identity treated as
// "already encoded"; NOT replaced with gzip).
func TestEncodeHeaders_Bucket6_AlreadyEncodedIdentity_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":     []string{"text/html"},
		"Content-Encoding": []string{"identity"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary; got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "identity" {
		t.Errorf("expected Content-Encoding=identity preserved per §11.11; got %q", v)
	}
}

// Bucket 7 — etag_disabled server-side skip (mode-b): NO Vary + not_compressed_etag.
func TestEncodeHeaders_Bucket7_EtagDisabled_Skip_NoVary_NotCompressedEtag(t *testing.T) {
	cc := defaultCompiledConfig()
	cc.disableOnEtagHeader = true
	f := freshEncodeFilter(t, cc, nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type": []string{"text/html"},
		"Etag":         []string{`"abc123"`},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on server-side etag-disabled skip; got %q", v)
	}
	if v := headers.Get("Etag"); v != `"abc123"` {
		t.Errorf("expected ETag preserved verbatim on mode-b skip; got %q", v)
	}
	if f.stats.NotCompressedEtag.Load() != 1 {
		t.Errorf("expected not_compressed_etag=1; got %d", f.stats.NotCompressedEtag.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 7 mode-b with WEAK etag: same skip (any ETag presence).
func TestEncodeHeaders_Bucket7_EtagDisabled_WeakEtag_Skip_NoVary(t *testing.T) {
	cc := defaultCompiledConfig()
	cc.disableOnEtagHeader = true
	f := freshEncodeFilter(t, cc, nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type": []string{"text/html"},
		"Etag":         []string{`W/"abc123"`},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on mode-b weak-etag skip; got %q", v)
	}
	if v := headers.Get("Etag"); v != `W/"abc123"` {
		t.Errorf("expected weak ETag preserved verbatim on mode-b skip; got %q", v)
	}
}

// Bucket 8 — no_transform Cache-Control server-side skip: NO Vary.
func TestEncodeHeaders_Bucket8_NoTransform_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":  []string{"text/html"},
		"Cache-Control": []string{"no-transform"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on no-transform server-side skip; got %q", v)
	}
	if v := headers.Get("Cache-Control"); v != "no-transform" {
		t.Errorf("expected Cache-Control preserved; got %q", v)
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 8 variant — multi-directive Cache-Control: same skip per §11.12.
func TestEncodeHeaders_Bucket8_NoTransformMultiDirective_Skip(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":  []string{"text/html"},
		"Cache-Control": []string{"max-age=3600, no-transform, public"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary; got %q", v)
	}
}

// Bucket 9 — content_type_mismatch server-side skip: NO Vary.
func TestEncodeHeaders_Bucket9_ContentTypeMismatch_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type": []string{"image/png"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on content-type mismatch (server-side); got %q", v)
	}
	if v := headers.Get("Content-Encoding"); v != "" {
		t.Errorf("expected no Content-Encoding on skip; got %q", v)
	}
	if f.stats.HeaderCompressorUsed.Load() != 1 {
		t.Errorf("expected header_compressor_used=1; got %d", f.stats.HeaderCompressorUsed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 9 — content-type prefix match (text/html;charset=utf-8): COMPRESS path.
func TestEncodeHeaders_Bucket9_ContentTypePrefixMatch_Compress(t *testing.T) {
	// Per §11.6: prefix-match is case-insensitive on the media-type/subtype
	// prefix. "text/html; charset=utf-8" matches "text/html".
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":   []string{"text/html; charset=utf-8"},
		"Content-Length": []string{"1024"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Content-Encoding"); v != "gzip" {
		t.Errorf("expected Content-Encoding=gzip; got %q", v)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding; got %q", v)
	}
	if !f.willCompress {
		t.Error("expected willCompress=true on compress path")
	}
}

// Bucket 10 — content_length_too_small_known server-side skip: NO Vary +
// response_content_length_too_small counter.
func TestEncodeHeaders_Bucket10_ContentLengthTooSmall_Skip_NoVary(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":   []string{"text/html"},
		"Content-Length": []string{"10"}, // below default 30
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Vary"); v != "" {
		t.Errorf("expected NO Vary on content-length-too-small (server-side); got %q", v)
	}
	if v := headers.Get("Content-Length"); v != "10" {
		t.Errorf("expected Content-Length preserved on skip; got %q", v)
	}
	if f.stats.ResponseContentLengthTooSmall.Load() != 1 {
		t.Errorf("expected response_content_length_too_small=1; got %d", f.stats.ResponseContentLengthTooSmall.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 1 {
		t.Errorf("expected response_not_compressed=1; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Bucket 10 — Content-Length above threshold: COMPRESS path.
func TestEncodeHeaders_Bucket10_ContentLengthAtThreshold_Compress(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":   []string{"text/html"},
		"Content-Length": []string{"30"}, // == default threshold; passes (>=).
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Content-Encoding"); v != "gzip" {
		t.Errorf("expected Content-Encoding=gzip at threshold; got %q", v)
	}
	if !f.willCompress {
		t.Error("expected willCompress=true at threshold")
	}
}

// Compress path — happy path: Content-Encoding=gzip, Vary=Accept-Encoding,
// Content-Length stripped, willCompress=true, response_compressed NOT yet
// incremented at EncodeHeaders (Task 7 EncodeData increments).
func TestEncodeHeaders_AllowCompress_HappyPath(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type":   []string{"text/html"},
		"Content-Length": []string{"1024"},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Content-Encoding"); v != "gzip" {
		t.Errorf("expected Content-Encoding=gzip on compress path; got %q", v)
	}
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding on compress path; got %q", v)
	}
	if v := headers.Get("Content-Length"); v != "" {
		t.Errorf("expected Content-Length stripped on compress path; got %q", v)
	}
	if !f.willCompress {
		t.Error("expected willCompress=true on compress path")
	}
	if f.stats.HeaderCompressorUsed.Load() != 1 {
		t.Errorf("expected header_compressor_used=1; got %d", f.stats.HeaderCompressorUsed.Load())
	}
	if f.stats.ResponseNotCompressed.Load() != 0 {
		t.Errorf("expected response_not_compressed=0 on compress path; got %d", f.stats.ResponseNotCompressed.Load())
	}
}

// Compress path — mode-a (default disable_on_etag_header=false): strong ETag STRIPPED.
func TestEncodeHeaders_AllowCompress_ModeA_StrongEtagStripped(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type": []string{"text/html"},
		"Etag":         []string{`"abc123"`},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Content-Encoding"); v != "gzip" {
		t.Errorf("expected Content-Encoding=gzip; got %q", v)
	}
	if v := headers.Get("Etag"); v != "" {
		t.Errorf("expected strong ETag STRIPPED on mode-a compress path (per §11.7); got %q", v)
	}
	if !f.willCompress {
		t.Error("expected willCompress=true")
	}
}

// Compress path — mode-a: WEAK ETag PRESERVED.
func TestEncodeHeaders_AllowCompress_ModeA_WeakEtagPreserved(t *testing.T) {
	f := freshEncodeFilter(t, defaultCompiledConfig(), nil)
	f.acceptHeaderClassification = "compressor_used"
	f.acceptedEncoding = "gzip"
	headers := http.Header{
		"Content-Type": []string{"text/html"},
		"Etag":         []string{`W/"abc123"`},
	}
	status := f.EncodeHeaders(headers, false)
	if status != envoyhttp.Continue {
		t.Errorf("expected Continue; got %v", status)
	}
	if v := headers.Get("Etag"); v != `W/"abc123"` {
		t.Errorf("expected weak ETag PRESERVED on mode-a compress path (per §11.7); got %q", v)
	}
}

// effectiveConfig — listener-level when no per-route.
func TestEffectiveConfig_NoPerRoute_ReturnsListenerLevel(t *testing.T) {
	cc := defaultCompiledConfig()
	cc.removeAcceptEncodingHeader = true
	f := freshEncodeFilter(t, cc, nil)
	eff := f.effectiveConfig()
	if eff != cc {
		t.Error("expected effectiveConfig to return listener-level *compiledConfig pointer-equal when no per-route")
	}
	if !eff.removeAcceptEncodingHeader {
		t.Error("expected removeAcceptEncodingHeader=true from listener-level")
	}
}

// effectiveConfig — per-route override wins.
func TestEffectiveConfig_PerRouteOverride_ClonesAndOverrides(t *testing.T) {
	cc := defaultCompiledConfig()
	cc.removeAcceptEncodingHeader = false
	override := true
	cpr := &compiledPerRoute{removeAcceptEncodingHeaderOverride: &override}
	f := freshEncodeFilter(t, cc, cpr)
	eff := f.effectiveConfig()
	if eff == cc {
		t.Error("expected effectiveConfig to return CLONED *compiledConfig when per-route override present")
	}
	if !eff.removeAcceptEncodingHeader {
		t.Errorf("expected per-route override true to win over listener false; got %v", eff.removeAcceptEncodingHeader)
	}
	// Listener-level config remains unchanged.
	if cc.removeAcceptEncodingHeader {
		t.Error("listener-level removeAcceptEncodingHeader was mutated; expected immutable")
	}
}

// --- Group 7: Vary / ETag / Content-Encoding mutators
// (appendVaryAcceptEncoding + maybeStripStrongEtag helpers per §1.1
// amendments 5-6 + §11.10 + §11.7). ---

func TestAppendVaryAcceptEncoding_NoExisting_SetsAccept(t *testing.T) {
	headers := http.Header{}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary=Accept-Encoding; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_ExistingOrigin_Appends(t *testing.T) {
	// Per §11.10 verbatim: "vary: Origin, Accept-Encoding".
	headers := http.Header{"Vary": []string{"Origin"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Origin, Accept-Encoding" {
		t.Errorf("expected Vary=\"Origin, Accept-Encoding\"; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_ExistingWildcard_AppendsCommaSpaceAccept(t *testing.T) {
	// Per §11.10: APPEND EVEN ON WILDCARD; NOT short-circuited.
	headers := http.Header{"Vary": []string{"*"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "*, Accept-Encoding" {
		t.Errorf("expected Vary=\"*, Accept-Encoding\"; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_ExistingAcceptEncoding_NoOp(t *testing.T) {
	// Per §11.10(b): token-match dedup is case-insensitive idempotent.
	headers := http.Header{"Vary": []string{"Accept-Encoding"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Accept-Encoding" {
		t.Errorf("expected Vary unchanged on dedup; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_ExistingAcceptEncodingMixedCase_NoOp(t *testing.T) {
	// Case-insensitive dedup per §11.10(b).
	headers := http.Header{"Vary": []string{"accept-encoding"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "accept-encoding" {
		t.Errorf("expected Vary unchanged on case-insensitive dedup; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_MultiToken_WithAcceptEncoding_NoOp(t *testing.T) {
	// Existing Vary already contains Accept-Encoding among multi tokens.
	headers := http.Header{"Vary": []string{"Origin, Accept-Encoding, User-Agent"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Origin, Accept-Encoding, User-Agent" {
		t.Errorf("expected Vary unchanged on multi-token dedup; got %q", v)
	}
}

func TestAppendVaryAcceptEncoding_MultiToken_WithoutAcceptEncoding_Appends(t *testing.T) {
	headers := http.Header{"Vary": []string{"Origin, User-Agent"}}
	appendVaryAcceptEncoding(headers)
	if v := headers.Get("Vary"); v != "Origin, User-Agent, Accept-Encoding" {
		t.Errorf("expected Vary appended; got %q", v)
	}
}

func TestMaybeStripStrongEtag_StrongEtag_Stripped(t *testing.T) {
	headers := http.Header{"Etag": []string{`"abc123"`}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("Etag"); v != "" {
		t.Errorf("expected strong ETag stripped per §11.7 mode-a; got %q", v)
	}
}

func TestMaybeStripStrongEtag_WeakEtag_Preserved(t *testing.T) {
	headers := http.Header{"Etag": []string{`W/"abc123"`}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("Etag"); v != `W/"abc123"` {
		t.Errorf("expected weak ETag preserved per §11.7 mode-a; got %q", v)
	}
}

func TestMaybeStripStrongEtag_NoEtag_NoOp(t *testing.T) {
	headers := http.Header{"Content-Type": []string{"text/html"}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("Etag"); v != "" {
		t.Errorf("expected no-op on missing ETag; got %q", v)
	}
	if v := headers.Get("Content-Type"); v != "text/html" {
		t.Errorf("expected unrelated headers untouched; got %q", v)
	}
}

func TestMaybeStripStrongEtag_MalformedEtag_Preserved(t *testing.T) {
	// Defensive: malformed ETag (no quotes) — preserve verbatim per §6.6 comment.
	headers := http.Header{"Etag": []string{"not-a-valid-etag"}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("Etag"); v != "not-a-valid-etag" {
		t.Errorf("expected malformed ETag preserved verbatim; got %q", v)
	}
}

func TestMaybeStripStrongEtag_EmptyQuotedEtag_Stripped(t *testing.T) {
	// `""` matches the strong-etag regex `^"[^"]*"$` (zero-length tag).
	headers := http.Header{"Etag": []string{`""`}}
	maybeStripStrongEtag(headers)
	if v := headers.Get("Etag"); v != "" {
		t.Errorf("expected empty-quoted ETag stripped (matches strong regex); got %q", v)
	}
}

// --- Group 6: EncodeData body — gzip-encode + OverwriteBody primitive call +
// counter increments + late-MCL revert anomaly (per SPEC §6.7 + §11.9 + §11.14
// + ADR-0131 §Decision (i)-(ii)+(vi)+(vii) + D4 settlement) ---
//
// Group 6 tests drive the EncodeData body algorithm: Path B one-shot
// gzip-encode + emit via the framework primitive f.ecb.OverwriteBody. The
// fakeCallbacks helper captures OverwriteBody invocations so tests can assert
// (a) the primitive was called exactly once on the compress path, (b) the
// captured bytes round-trip through gzip.NewReader back to the original input,
// (c) the 3 response_* counter increments fired correctly. The late-MCL
// revert anomaly (per D4 settlement + ADR-0131 §Decision (vii)) increments
// BOTH response_content_length_too_small AND response_not_compressed.

// freshEncodeDataFilter builds a *filter ready for EncodeData testing: real
// counters allocated (so .Load() works), config defaults, and a fakeCallbacks
// wired through SetEncoderCallbacks so OverwriteBody captures are observable.
// willCompress / passthrough must be set explicitly by each test.
func freshEncodeDataFilter(t *testing.T, cc *compiledConfig) (*filter, *fakeCallbacks) {
	t.Helper()
	cb := &fakeCallbacks{}
	f := &filter{
		config: cc,
		stats:  newTestFilterStats(),
	}
	f.SetEncoderCallbacks(cb)
	return f, cb
}

func TestEncodeData_Passthrough_DataContinue_NoOverwrite(t *testing.T) {
	// f.passthrough=true (per-route disabled) → pass-through; no compression,
	// no counter increments, no OverwriteBody call. Per SPEC §6.7 line 948.
	f, cb := freshEncodeDataFilter(t, defaultCompiledConfig())
	f.passthrough = true
	f.willCompress = false
	body := []byte(strings.Repeat("A", 1024))
	status := f.EncodeData(body, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 0 {
		t.Errorf("OverwriteBody calls = %d; want 0 (passthrough)", cb.overwriteBodyCallCount)
	}
	if v := f.stats.ResponseCompressed.Load(); v != 0 {
		t.Errorf("ResponseCompressed = %d; want 0 on passthrough", v)
	}
	if v := f.stats.ResponseNotCompressed.Load(); v != 0 {
		t.Errorf("ResponseNotCompressed = %d; want 0 on passthrough", v)
	}
}

func TestEncodeData_WillCompressFalse_DataContinue_NoOverwrite(t *testing.T) {
	// f.willCompress=false (EncodeHeaders skip-path) → pass-through; no
	// compression, no counter increments, no OverwriteBody call. Per SPEC §6.7
	// line 948.
	f, cb := freshEncodeDataFilter(t, defaultCompiledConfig())
	f.passthrough = false
	f.willCompress = false
	body := []byte(strings.Repeat("B", 1024))
	status := f.EncodeData(body, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 0 {
		t.Errorf("OverwriteBody calls = %d; want 0 (willCompress=false)", cb.overwriteBodyCallCount)
	}
	if v := f.stats.ResponseCompressed.Load(); v != 0 {
		t.Errorf("ResponseCompressed = %d; want 0 on skip path", v)
	}
}

func TestEncodeData_NotEndStream_DataContinue_NoOverwrite_Defensive(t *testing.T) {
	// endStream=false → defensive pass-through (current framework always invokes
	// once with endStream=true; defensive for future). No compression, no
	// counters, no OverwriteBody. Per SPEC §6.7 lines 954-958.
	f, cb := freshEncodeDataFilter(t, defaultCompiledConfig())
	f.willCompress = true
	body := []byte(strings.Repeat("C", 1024))
	status := f.EncodeData(body, false /* endStream */)
	if status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 0 {
		t.Errorf("OverwriteBody calls = %d; want 0 on !endStream defensive", cb.overwriteBodyCallCount)
	}
	if v := f.stats.ResponseCompressed.Load(); v != 0 {
		t.Errorf("ResponseCompressed = %d; want 0 on !endStream", v)
	}
	if v := f.stats.ResponseNotCompressed.Load(); v != 0 {
		t.Errorf("ResponseNotCompressed = %d; want 0 on !endStream (defensive — no skip count either)", v)
	}
}

func TestEncodeData_LateMinContentLength_RevertSkip_DataContinue_CountersIncremented(t *testing.T) {
	// Late min_content_length gate per D4 settlement (per ADR-0131 §Decision
	// (vii)): body length < minContentLength at EncodeData time → revert
	// compression decision. Per SPEC §6.7 lines 964-973: BOTH
	// response_content_length_too_small AND response_not_compressed increment.
	// No OverwriteBody call.
	cc := defaultCompiledConfig()
	cc.minContentLength = 100
	f, cb := freshEncodeDataFilter(t, cc)
	f.willCompress = true
	body := []byte(strings.Repeat("D", 50)) // 50 < 100
	status := f.EncodeData(body, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 0 {
		t.Errorf("OverwriteBody calls = %d; want 0 on late-revert", cb.overwriteBodyCallCount)
	}
	if v := f.stats.ResponseContentLengthTooSmall.Load(); v != 1 {
		t.Errorf("ResponseContentLengthTooSmall = %d; want 1 (D4 settlement: both counters)", v)
	}
	if v := f.stats.ResponseNotCompressed.Load(); v != 1 {
		t.Errorf("ResponseNotCompressed = %d; want 1 (D4 settlement: both counters)", v)
	}
	if v := f.stats.ResponseCompressed.Load(); v != 0 {
		t.Errorf("ResponseCompressed = %d; want 0 on late-revert", v)
	}
	if v := f.stats.ResponseTotalUncompressedBytes.Load(); v != 0 {
		t.Errorf("ResponseTotalUncompressedBytes = %d; want 0 on late-revert (no compression occurred)", v)
	}
	if v := f.stats.ResponseTotalCompressedBytes.Load(); v != 0 {
		t.Errorf("ResponseTotalCompressedBytes = %d; want 0 on late-revert", v)
	}
}

func TestEncodeData_LateMinContentLength_AtThreshold_Compresses(t *testing.T) {
	// Boundary: len(data) == minContentLength → compress (the late-gate
	// predicate is strict `<`, matching computeSkipReason's bucket-10 boundary
	// semantic). Asserts symmetry with EncodeHeaders' bucket-10 test
	// (TestEncodeHeaders_Bucket10_ContentLengthAtThreshold_Compress).
	cc := defaultCompiledConfig()
	cc.minContentLength = 100
	f, cb := freshEncodeDataFilter(t, cc)
	f.willCompress = true
	body := []byte(strings.Repeat("E", 100)) // exactly at threshold
	status := f.EncodeData(body, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 1 {
		t.Errorf("OverwriteBody calls = %d; want 1 at threshold", cb.overwriteBodyCallCount)
	}
	if v := f.stats.ResponseCompressed.Load(); v != 1 {
		t.Errorf("ResponseCompressed = %d; want 1 at threshold", v)
	}
	if v := f.stats.ResponseContentLengthTooSmall.Load(); v != 0 {
		t.Errorf("ResponseContentLengthTooSmall = %d; want 0 at threshold (strict < semantic)", v)
	}
}

func TestEncodeData_AllowPath_GzipEncodes_OverwriteBodyCalled_CountersIncremented(t *testing.T) {
	// Parametrized over small/medium/large bodies. Each case verifies:
	//   (1) status = DataContinue;
	//   (2) OverwriteBody called exactly once;
	//   (3) captured compressed bytes are non-empty;
	//   (4) gzip.NewReader on captured bytes round-trips to the original input
	//       (decompressed-byte equivalence per SPEC §11.14 + ADR-0133);
	//   (5) 3 counter increments fire correctly:
	//       response_compressed +1, response_total_uncompressed_bytes += len(data),
	//       response_total_compressed_bytes += len(captured).
	cases := []struct {
		name string
		size int
	}{
		{"Small_64B", 64},       // small but above default min_content_length=30
		{"Medium_1024B", 1024},  // canonical 1 KiB
		{"Large_10240B", 10240}, // canonical 10 KiB
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, cb := freshEncodeDataFilter(t, defaultCompiledConfig())
			f.willCompress = true
			body := []byte(strings.Repeat("A", tc.size))
			status := f.EncodeData(body, true)
			if status != envoyhttp.DataContinue {
				t.Errorf("status = %v; want DataContinue", status)
			}
			if cb.overwriteBodyCallCount != 1 {
				t.Fatalf("OverwriteBody calls = %d; want exactly 1", cb.overwriteBodyCallCount)
			}
			compressed := cb.overwriteBodyCalls[0]
			if len(compressed) == 0 {
				t.Fatal("captured compressed bytes are empty")
			}
			// Round-trip: gzip.NewReader → original bytes.
			gr, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatalf("gzip.NewReader on captured bytes: %v", err)
			}
			decompressed, err := io.ReadAll(gr)
			if err != nil {
				t.Fatalf("io.ReadAll(gzip-reader): %v", err)
			}
			if err := gr.Close(); err != nil {
				t.Fatalf("gzip-reader Close: %v", err)
			}
			if !bytes.Equal(decompressed, body) {
				t.Errorf("decompressed bytes do not equal original input "+
					"(len got=%d, want=%d; first-mismatch-at = %d)",
					len(decompressed), len(body), firstMismatchIndex(decompressed, body))
			}
			// Counter assertions.
			if v := f.stats.ResponseCompressed.Load(); v != 1 {
				t.Errorf("ResponseCompressed = %d; want 1", v)
			}
			if v := f.stats.ResponseTotalUncompressedBytes.Load(); v != uint64(len(body)) {
				t.Errorf("ResponseTotalUncompressedBytes = %d; want %d", v, len(body))
			}
			if v := f.stats.ResponseTotalCompressedBytes.Load(); v != uint64(len(compressed)) {
				t.Errorf("ResponseTotalCompressedBytes = %d; want %d", v, len(compressed))
			}
			if v := f.stats.ResponseNotCompressed.Load(); v != 0 {
				t.Errorf("ResponseNotCompressed = %d; want 0 on compress path", v)
			}
			if v := f.stats.ResponseContentLengthTooSmall.Load(); v != 0 {
				t.Errorf("ResponseContentLengthTooSmall = %d; want 0 on compress path", v)
			}
		})
	}
}

func TestEncodeData_LevelMapping_DifferentGzippedSizes(t *testing.T) {
	// Sanity check that f.config.gzip.level threads through to gzip.NewWriterLevel:
	// BestSpeed (1) vs BestCompression (9) on a compressible body produce
	// different compressed-byte sizes. Both round-trip correctly. Per
	// ADR-0130 §Decision (iv): the compression_level enum maps to int that
	// passes through to compress/gzip verbatim.
	body := make([]byte, 4096)
	for i := range body {
		// Non-repetitive enough that level choice meaningfully affects output.
		body[i] = byte((i * 17) ^ (i >> 3))
	}

	encodeWithLevel := func(level int) []byte {
		t.Helper()
		cc := defaultCompiledConfig()
		cc.gzip = &compiledGzipConfig{level: level}
		f, cb := freshEncodeDataFilter(t, cc)
		f.willCompress = true
		if status := f.EncodeData(body, true); status != envoyhttp.DataContinue {
			t.Fatalf("status @ level=%d = %v; want DataContinue", level, status)
		}
		if cb.overwriteBodyCallCount != 1 {
			t.Fatalf("OverwriteBody calls @ level=%d = %d; want 1", level, cb.overwriteBodyCallCount)
		}
		return cb.overwriteBodyCalls[0]
	}

	bestSpeed := encodeWithLevel(gzip.BestSpeed)
	bestComp := encodeWithLevel(gzip.BestCompression)

	// Different levels should produce different compressed sizes on this input.
	if len(bestSpeed) == len(bestComp) {
		t.Errorf("expected different compressed sizes for BestSpeed vs BestCompression on a non-repetitive input; both = %d", len(bestSpeed))
	}

	// Both must round-trip correctly.
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"BestSpeed", bestSpeed},
		{"BestCompression", bestComp},
	} {
		gr, err := gzip.NewReader(bytes.NewReader(tc.data))
		if err != nil {
			t.Errorf("gzip.NewReader @ %s: %v", tc.name, err)
			continue
		}
		decompressed, err := io.ReadAll(gr)
		if err != nil {
			t.Errorf("io.ReadAll @ %s: %v", tc.name, err)
			continue
		}
		_ = gr.Close()
		if !bytes.Equal(decompressed, body) {
			t.Errorf("round-trip mismatch @ %s", tc.name)
		}
	}
}

func TestEncodeData_HuffmanOnlyStrategy_SilentIgnored_Compresses(t *testing.T) {
	// Per ADR-0130 §Decision (v): HUFFMAN_ONLY compression_strategy is
	// silent-ignored at runtime — Go's compress/gzip does not expose a
	// strategy knob via NewWriterLevel, only level. The huffmanOnly bit on
	// compiledGzipConfig is parsed but unused at runtime; EncodeData calls
	// gzip.NewWriterLevel with level only. This test asserts that the
	// compress path runs successfully when huffmanOnly=true and produces
	// gzip-readable bytes (i.e. huffmanOnly does NOT alter the output
	// codec-format; the gzip reader can decode either way).
	cc := defaultCompiledConfig()
	cc.gzip = &compiledGzipConfig{level: gzip.DefaultCompression, huffmanOnly: true}
	f, cb := freshEncodeDataFilter(t, cc)
	f.willCompress = true
	body := []byte(strings.Repeat("Z", 1024))
	if status := f.EncodeData(body, true); status != envoyhttp.DataContinue {
		t.Errorf("status = %v; want DataContinue", status)
	}
	if cb.overwriteBodyCallCount != 1 {
		t.Fatalf("OverwriteBody calls = %d; want 1", cb.overwriteBodyCallCount)
	}
	compressed := cb.overwriteBodyCalls[0]
	gr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader (huffmanOnly silent-ignored): %v", err)
	}
	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	_ = gr.Close()
	if !bytes.Equal(decompressed, body) {
		t.Error("decompressed body mismatch under huffmanOnly=true")
	}
	if v := f.stats.ResponseCompressed.Load(); v != 1 {
		t.Errorf("ResponseCompressed = %d; want 1", v)
	}
}

func TestEncodeTrailers_PassThrough_TrailersContinue(t *testing.T) {
	// EncodeTrailers passes through unconditionally per SPEC §6.8. No body
	// inspection, no counter increments. Mirror of DecodeTrailers test.
	f, _ := freshEncodeDataFilter(t, defaultCompiledConfig())
	trailers := http.Header{"X-Trailer-Foo": []string{"bar"}}
	status := f.EncodeTrailers(trailers)
	if status != envoyhttp.TrailersContinue {
		t.Errorf("status = %v; want TrailersContinue", status)
	}
	// Verify trailers were not mutated.
	if v := trailers.Get("X-Trailer-Foo"); v != "bar" {
		t.Errorf("trailer mutated; got %q want %q", v, "bar")
	}
}

// firstMismatchIndex returns the first index at which a and b differ, or
// min(len(a), len(b)) if one is a prefix of the other. Test-helper for the
// large-body round-trip error message.
func firstMismatchIndex(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// --- Group 8: stats namespace + 17-counter registration (per SPEC §6.9 +
// §11.5 + §14.1 Group 8 + ADR-0132 §Decision (i)+(ii)+(v)) ---
//
// Group 8 tests verify the production newFilterStats helper registers all 17
// counters at the namespace path
//   http.<HCM_stat_prefix>.compressor.<libraryName>.gzip.[response.|request.]<counter>
// per ADR-0132 §Decision (i)+(v). The library-name-empty consecutive-dots edge
// case per planner-time decision 5 (D5 settlement) is verified verbatim — when
// libraryName == "", the path contains ".." which SN2 flatten produces
// `envoy_http_compressor__gzip_<counter>` double-underscore Prometheus form
// per ADR-0132 §Decision (v).

// stats17CounterSuffixes enumerates the 17 counter-name suffixes per ADR-0132
// §Decision (i). Order arbitrary but stable for test deltas. Split into 7
// no-direction-infix + 5 response-infix + 5 request-infix groups (matches
// SPEC §11.5 verbatim probeA scrape grouping).
var stats17CounterSuffixes = struct {
	noInfix  []string
	response []string
	request  []string
}{
	noInfix: []string{
		"header_compressor_overshadowed",
		"header_compressor_used",
		"header_identity",
		"header_not_valid",
		"header_wildcard",
		"no_accept_header",
		"not_compressed_etag",
	},
	response: []string{
		"compressed",
		"content_length_too_small",
		"not_compressed",
		"total_compressed_bytes",
		"total_uncompressed_bytes",
	},
	request: []string{
		"compressed",
		"content_length_too_small",
		"not_compressed",
		"total_compressed_bytes",
		"total_uncompressed_bytes",
	},
}

// stats17ExpectedNames builds the 17 internal stat names that newFilterStats
// MUST register at (statPrefix, libraryName). Format per ADR-0132 §Decision
// (i)+(v):
//
//	http.<statPrefix>.compressor.<libraryName>.gzip.[<infix>.]<counter>
//
// 7 no-infix + 5 response. + 5 request. = 17 names total. The function is
// the test-side mirror of newFilterStats's prefix-building loop; if either
// drifts, Group 8 catches the divergence.
func stats17ExpectedNames(statPrefix, libraryName string) []string {
	base := "http." + statPrefix + ".compressor." + libraryName + ".gzip."
	out := make([]string, 0, 17)
	for _, c := range stats17CounterSuffixes.noInfix {
		out = append(out, base+c)
	}
	for _, c := range stats17CounterSuffixes.response {
		out = append(out, base+"response."+c)
	}
	for _, c := range stats17CounterSuffixes.request {
		out = append(out, base+"request."+c)
	}
	return out
}

// registryHasCounter returns true iff the registry has a counter registered
// under `name`. Uses Walk to enumerate registered metrics so the test does not
// depend on private byName field access.
func registryHasCounter(reg *stats.Registry, name string) bool {
	found := false
	reg.Walk(func(m stats.Metric) {
		if m.Type() == stats.MetricCounter && m.Name() == name {
			found = true
		}
	})
	return found
}

// registryCounter returns the *stats.Counter registered under `name`, or nil.
// Allows tests to read counter values via .Load() to verify always-zero
// request_* counters per ADR-0132 §Decision (vii) twin-series discipline.
func registryCounter(reg *stats.Registry, name string) *stats.Counter {
	var got *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Type() != stats.MetricCounter || m.Name() != name {
			return
		}
		if c, ok := m.(*stats.Counter); ok {
			got = c
		}
	})
	return got
}

// TestStatsNamespace_LibraryNameSet_StatPathCorrect verifies the canonical
// namespace shape per ADR-0132 §Decision (i) with a non-empty libraryName.
// Verbatim Prometheus rendering per SPEC §11.5 probeA evidence:
//
//	envoy_http_compressor_text_optimized_gzip_response_compressed{envoy_http_conn_manager_prefix="ingress_p14"}
//
// is produced from the internal stat name
// `http.ingress_p14.compressor.text_optimized.gzip.response.compressed` via
// the existing Rule SN2 (ADR-0061; ADR-0132 §Decision (iii) reuses SN2; NO new
// SN10). This test asserts the internal name is correct; the SN2 flatten is
// covered by internal/stats/name_test.go.
func TestStatsNamespace_LibraryNameSet_StatPathCorrect(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_p14", "text_optimized")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; expected non-nil with non-nil registry")
	}
	want := "http.ingress_p14.compressor.text_optimized.gzip.response.compressed"
	if !registryHasCounter(reg, want) {
		t.Errorf("counter %q NOT registered", want)
	}
	// Sanity check: the field pointer matches the registered counter.
	if got := registryCounter(reg, want); got != fs.ResponseCompressed {
		t.Errorf("fs.ResponseCompressed (%p) does not match registry-resolved counter (%p)", fs.ResponseCompressed, got)
	}
}

// TestStatsNamespace_LibraryNameEmpty_DoubleDotPath verifies the D5 settlement:
// when libraryName == "", the namespace contains consecutive dots
// (`compressor..gzip.<counter>`); SN2 flatten produces double-underscore
// Prometheus form `envoy_http_compressor__gzip_<counter>` per ADR-0132
// §Decision (v) + SPEC §1.1 amendment 3 + §11.5 probeC evidence.
//
// This is the LOAD-BEARING D5 behavioral pin. The internal name must contain
// the verbatim `..gzip.` substring; the stat-name builder MUST NOT collapse
// the empty segment.
func TestStatsNamespace_LibraryNameEmpty_DoubleDotPath(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_p14", "")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; expected non-nil with non-nil registry")
	}
	// Spot-check the canonical D5 verbatim path: response.compressed.
	wantD5 := "http.ingress_p14.compressor..gzip.response.compressed"
	if !registryHasCounter(reg, wantD5) {
		t.Errorf("counter %q NOT registered (D5 double-dot path)", wantD5)
	}
	// All 17 counters MUST embed `..gzip.` (per D5 + ADR-0132 §Decision (v)).
	for _, name := range stats17ExpectedNames("ingress_p14", "") {
		if !strings.Contains(name, "..gzip.") {
			t.Errorf("expected name %q to contain `..gzip.` substring (D5)", name)
		}
		if !registryHasCounter(reg, name) {
			t.Errorf("counter %q NOT registered (D5 double-dot path)", name)
		}
	}
}

// TestStatsNamespace_AllSeventeenCountersRegistered verifies all 17 counters
// are registered under the expected names + assigned to non-nil struct fields
// (per ADR-0132 §Decision (i) full registration).
func TestStatsNamespace_AllSeventeenCountersRegistered(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_p14", "text_optimized")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; expected non-nil")
	}

	// Verify every expected name is registered as a counter.
	for _, name := range stats17ExpectedNames("ingress_p14", "text_optimized") {
		if !registryHasCounter(reg, name) {
			t.Errorf("counter %q NOT registered", name)
		}
	}

	// Verify each struct field is non-nil (the 17 NewCounter return values).
	fields := []struct {
		label string
		ptr   *stats.Counter
	}{
		{"HeaderCompressorOvershadowed", fs.HeaderCompressorOvershadowed},
		{"HeaderCompressorUsed", fs.HeaderCompressorUsed},
		{"HeaderIdentity", fs.HeaderIdentity},
		{"HeaderNotValid", fs.HeaderNotValid},
		{"HeaderWildcard", fs.HeaderWildcard},
		{"NoAcceptHeader", fs.NoAcceptHeader},
		{"NotCompressedEtag", fs.NotCompressedEtag},
		{"ResponseCompressed", fs.ResponseCompressed},
		{"ResponseContentLengthTooSmall", fs.ResponseContentLengthTooSmall},
		{"ResponseNotCompressed", fs.ResponseNotCompressed},
		{"ResponseTotalCompressedBytes", fs.ResponseTotalCompressedBytes},
		{"ResponseTotalUncompressedBytes", fs.ResponseTotalUncompressedBytes},
		{"RequestCompressed", fs.RequestCompressed},
		{"RequestContentLengthTooSmall", fs.RequestContentLengthTooSmall},
		{"RequestNotCompressed", fs.RequestNotCompressed},
		{"RequestTotalCompressedBytes", fs.RequestTotalCompressedBytes},
		{"RequestTotalUncompressedBytes", fs.RequestTotalUncompressedBytes},
	}
	if len(fields) != 17 {
		t.Fatalf("test bug: expected 17 fields, got %d", len(fields))
	}
	for _, f := range fields {
		if f.ptr == nil {
			t.Errorf("filterStats.%s is nil; expected non-nil after newFilterStats", f.label)
		}
	}

	// Verify the registry contains EXACTLY 17 counters (no extras, no shortfall).
	count := 0
	reg.Walk(func(m stats.Metric) {
		if m.Type() == stats.MetricCounter {
			count++
		}
	})
	if count != 17 {
		t.Errorf("registry has %d counters; want 17", count)
	}
}

// TestStatsNamespace_ResponseInfixPresent_WhenResponseDirectionConfigSet
// verifies the direction-infix discipline per ADR-0132 §Decision (i)+(ii) +
// SPEC §11.5 verbatim probeA evidence:
//
//   - 6 header_* + 1 not_compressed_etag carry NO direction infix (registered
//     at `compressor.<lib>.gzip.<counter>` directly).
//   - 5 response_* carry the `response.` infix.
//   - 5 request_* carry the `request.` infix (always-zero in MVP per ADR-0132
//     §Decision (vii) twin-series discipline).
//
// Phase-14 always sets response_direction_config (per ADR-0132 §Decision (ii)
// + planner-time settlement); the differential fixture's envoy.yaml +
// envoy-go.yaml MUST agree on this namespace shape for byte-equivalent scrape.
func TestStatsNamespace_ResponseInfixPresent_WhenResponseDirectionConfigSet(t *testing.T) {
	reg := stats.NewRegistry()
	_ = newFilterStats(reg, "ingress_p14", "text_optimized")

	base := "http.ingress_p14.compressor.text_optimized.gzip."
	// No-infix counters: 6 header_* + 1 not_compressed_etag.
	for _, c := range stats17CounterSuffixes.noInfix {
		name := base + c
		if !registryHasCounter(reg, name) {
			t.Errorf("no-infix counter %q NOT registered", name)
		}
		// Sanity: no-infix counters MUST NOT collide with response./request. shape.
		if strings.HasPrefix(c, "response.") || strings.HasPrefix(c, "request.") {
			t.Errorf("test bug: no-infix suffix %q must not begin with response./request.", c)
		}
	}
	// Response-infix counters: 5 response_*.
	for _, c := range stats17CounterSuffixes.response {
		name := base + "response." + c
		if !registryHasCounter(reg, name) {
			t.Errorf("response-infix counter %q NOT registered", name)
		}
	}
	// Request-infix counters: 5 request_* (always-zero in MVP).
	for _, c := range stats17CounterSuffixes.request {
		name := base + "request." + c
		if !registryHasCounter(reg, name) {
			t.Errorf("request-infix counter %q NOT registered", name)
		}
	}
}

// TestStatsNamespace_RequestCountersRegisteredAtZero verifies the 5 always-zero
// request_* counters per ADR-0132 §Decision (vii) twin-series discipline: the
// counters are registered + observable as zero on both sides (no twin-series
// filtering required). MVP silent-ignores request_direction_config (per SPEC
// §1.1 amendment 1 + ADR-0132 §Decision (vii)); the request_* counters never
// fire, but their registration is load-bearing for byte-equivalent stat
// scrape with reference Envoy per fixture 0016.
func TestStatsNamespace_RequestCountersRegisteredAtZero(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_p14", "text_optimized")
	if fs == nil {
		t.Fatalf("newFilterStats returned nil; expected non-nil")
	}

	// Pair each field with the expected internal stat name + assert .Load() == 0.
	pairs := []struct {
		label string
		ptr   *stats.Counter
		name  string
	}{
		{"RequestCompressed", fs.RequestCompressed, "http.ingress_p14.compressor.text_optimized.gzip.request.compressed"},
		{"RequestContentLengthTooSmall", fs.RequestContentLengthTooSmall, "http.ingress_p14.compressor.text_optimized.gzip.request.content_length_too_small"},
		{"RequestNotCompressed", fs.RequestNotCompressed, "http.ingress_p14.compressor.text_optimized.gzip.request.not_compressed"},
		{"RequestTotalCompressedBytes", fs.RequestTotalCompressedBytes, "http.ingress_p14.compressor.text_optimized.gzip.request.total_compressed_bytes"},
		{"RequestTotalUncompressedBytes", fs.RequestTotalUncompressedBytes, "http.ingress_p14.compressor.text_optimized.gzip.request.total_uncompressed_bytes"},
	}
	for _, p := range pairs {
		if p.ptr == nil {
			t.Errorf("filterStats.%s is nil; expected non-nil registered counter", p.label)
			continue
		}
		if v := p.ptr.Load(); v != 0 {
			t.Errorf("filterStats.%s = %d; want 0 (always-zero MVP)", p.label, v)
		}
		// Cross-check: registry-resolved counter pointer-equals the struct field.
		if got := registryCounter(reg, p.name); got != p.ptr {
			t.Errorf("registry counter for %q (%p) != filterStats.%s (%p)", p.name, got, p.label, p.ptr)
		}
	}
}

// TestStatsNamespace_NilRegistry_ReturnsNil verifies the nil-tolerance contract
// per ADR-0085 nil-tolerance pattern (referenced from newFilterStats doc): when
// reg == nil, newFilterStats returns nil (caller's responsibility to guard).
// Documents the test-code path for non-stat-bearing test scenarios that may
// still want to exercise the production newFilterStats without a real registry.
func TestStatsNamespace_NilRegistry_ReturnsNil(t *testing.T) {
	if fs := newFilterStats(nil, "ingress_p14", "text_optimized"); fs != nil {
		t.Errorf("newFilterStats(nil, ...) = %+v; want nil", fs)
	}
}

// TestStatsNamespace_NewFactoryRegistersAllSeventeen verifies end-to-end that
// the `New` factory threads ctx.Stats + ctx.StatPrefix into newFilterStats so
// the 17 counters are registered exactly once per HCM stat_prefix per HCM-build
// time. Mirrors the production path from boot through HCM-build-time factory
// invocation; the localratelimit `TestNew_FactoryRegistersFourCounters`
// precedent (phase-11) shaped this test.
func TestStatsNamespace_NewFactoryRegistersAllSeventeen(t *testing.T) {
	reg := stats.NewRegistry()
	cfg := newGzipCompressor(t, &gzipv3.Gzip{}, &compressorv3.Compressor_ResponseDirectionConfig{}, "text_optimized")
	_, err := New(mustAny(t, cfg), envoyhttp.FactoryCtx{
		Stats:      reg,
		StatPrefix: "ingress_p14",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, name := range stats17ExpectedNames("ingress_p14", "text_optimized") {
		if !registryHasCounter(reg, name) {
			t.Errorf("counter %q NOT registered after New", name)
		}
	}
}
