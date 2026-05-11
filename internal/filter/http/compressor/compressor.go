package compressor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	gzipv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/compression/gzip/compressor/v3"
	compressorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/compressor/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// strongEtagPattern matches RFC 7232 §2.3 "strong validator" ETag values:
// a double-quote, zero or more non-quote chars, a double-quote. Per
// §11.7 + §1.1 amendment 6 mode-a: strong-ETag is STRIPPED on compress path
// because the strong-validator semantic ("entity bytes have not changed")
// cannot hold for a gzip-encoded entity whose bytes differ from the
// uncompressed entity.
//
// weakEtagPattern matches RFC 7232 §2.3 "weak validator" ETag values: the
// literal W/ prefix followed by a quoted string. Per §11.7 mode-a: weak-ETag
// is PRESERVED on compress path — the weak-validator semantic ("entity is
// semantically equivalent") DOES survive gzip re-encoding.
//
// Both patterns compile once at init per ADR-0129 §Decision (iv) helper-
// regex discipline.
var (
	strongEtagPattern = regexp.MustCompile(`^"[^"]*"$`)
	weakEtagPattern   = regexp.MustCompile(`^W/"[^"]*"$`)
)

// TypeURL is the canonical envoy.filters.http.compressor typed_config type URL.
// Boot wiring in cmd/envoy-go/main.go (Task 10) registers New under this key
// in the HTTPRegistry per ADR-0072.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.compressor.v3.Compressor"

// filterName is the canonical http_filters[].name string for compressor.
const filterName = "envoy.filters.http.compressor"

// gzipLibraryTypeURL is the only compressor_library.typed_config TypeURL
// the gzip-only MVP accepts. Per ADR-0130 §Decision (iii): envoy-go-only
// parse-rejects any other TypeURL with explicit error wording.
const gzipLibraryTypeURL = "type.googleapis.com/envoy.extensions.compression.gzip.compressor.v3.Gzip"

// defaultMinContentLength is Envoy's verbatim default for response bodies
// per SPEC §11.9 + ADR-0130 §Decision (i): below this byte length no
// compression is attempted.
const defaultMinContentLength uint32 = 30

// defaultContentTypes is the 8-entry default content_type list per SPEC §11.1
// (compressor.proto v1.37.2 line 49-65 verbatim). When
// response_direction_config.common_config.content_type is unset OR empty,
// envoy-go's compiledConfig.contentTypes initializes to this list.
var defaultContentTypes = []string{
	"application/javascript",
	"application/json",
	"application/xhtml+xml",
	"image/svg+xml",
	"text/css",
	"text/html",
	"text/plain",
	"text/xml",
}

// compiledConfig captures the listener-level Compressor proto's consumed
// fields per SPEC §6.2 + ADR-0130 §Decision (i). All fields are immutable
// post-New; the per-stream *filter holds a *compiledConfig pointer.
//
//   - libraryName: embedded in stat namespace (compressor.<libraryName>.gzip.*);
//     empty allowed per D5.
//   - gzip: gzip-only MVP per ADR-0130 §Decision (ii); non-nil.
//   - minContentLength: default 30 if proto unset per SPEC §11.9.
//   - contentTypes: default 8-entry list if empty per SPEC §11.1; lowercase
//     prefix-matched per §11.6 (matching algorithm lands at Task 6).
//   - disableOnEtagHeader: dual-mode per §1.1 amendment 6 (Task 6).
//   - removeAcceptEncodingHeader: per-route override allowed (Task 5).
//   - uncompressibleResponseCodes: SPEC §11.2 documents the v1.37.2 proto
//     field; the go-control-plane v1.32.4 proto envoy-go consumes does NOT
//     expose it, so this field stays empty by construction and operators
//     cannot exercise the skip path. Documented at §13.4 forward-pointer.
type compiledConfig struct {
	libraryName                 string
	gzip                        *compiledGzipConfig
	minContentLength            uint32
	contentTypes                []string
	disableOnEtagHeader         bool
	removeAcceptEncodingHeader  bool
	uncompressibleResponseCodes map[uint32]struct{}
}

// compiledGzipConfig captures the codec-library Gzip proto's consumed fields
// per ADR-0130 §Decision (i) + (iv) + (v).
//
//   - level: mapped from Envoy's CompressionLevel enum per ADR-0130 §Decision
//     (iv) table; range [-1, 9]; passed to gzip.NewWriterLevel at Task 7.
//   - huffmanOnly: true iff compression_strategy == HUFFMAN_ONLY; all other
//     strategies collapse to default (Go compress/gzip exposes only this
//     strategy knob per §Decision (v)).
//
// Silent-ignored Gzip fields (memory_level, window_bits, chunk_size) are NOT
// stored — Go compress/gzip does not expose libz-equivalent knobs.
type compiledGzipConfig struct {
	level       int
	huffmanOnly bool
}

// compiledPerRoute captures the per-route CompressorPerRoute proto's parsed
// shape per SPEC §6.2 + ADR-0125 amendment §(viii). The per-route surface is
// NARROWER than listener-level (§1.1 amendment 4): only
// remove_accept_encoding_header is overridable; min_content_length / content_type
// / disable_on_etag_header / etc. CANNOT be overridden because the proto's
// ResponseDirectionOverrides type carries only that one field.
//
//   - disabled: exclusive with override; true → filter wholly inactive on this
//     route (no header mutation, no body inspection, no stat increments).
//   - removeAcceptEncodingHeaderOverride: pointer to distinguish "unset"
//     (inherit listener-level) from "explicitly false" (override the
//     listener-level true setting with false).
//
// No *filterStats field — per-route stats SHARED with listener-level per
// ADR-0132 §Decision (iv) + ADR-0125 amendment §(viii).
type compiledPerRoute struct {
	disabled                           bool
	removeAcceptEncodingHeaderOverride *bool
}

// filterStats holds the 17 counter pointers per HCM stat_prefix per ADR-0132
// §Decision (i). Field names use Go PascalCase matching the counter-name
// suffix per planner-time decision D8 (bijective mapping with the dotted
// counter names in the stat namespace).
//
// The 17 NewCounter registration calls land at Task 8 per ADR-0132's
// Lands-in-task field — see newFilterStats below. Production paths through
// New supply a non-nil *stats.Registry from FactoryCtx so all 17 fields are
// populated with non-nil *stats.Counter values; nil-tolerant test paths per
// ADR-0085 receive a nil *filterStats from newFilterStats(nil, ...).
type filterStats struct {
	// 6 Accept-Encoding-cluster counters (NOT split per direction; shared
	// across both per SPEC §11.5 empirical scrape verbatim).
	HeaderCompressorOvershadowed *stats.Counter
	HeaderCompressorUsed         *stats.Counter
	HeaderIdentity               *stats.Counter
	HeaderNotValid               *stats.Counter
	HeaderWildcard               *stats.Counter
	NoAcceptHeader               *stats.Counter

	// 1 ETag-skip counter.
	NotCompressedEtag *stats.Counter

	// 5 response-side counters (active in MVP).
	ResponseCompressed             *stats.Counter
	ResponseContentLengthTooSmall  *stats.Counter
	ResponseNotCompressed          *stats.Counter
	ResponseTotalCompressedBytes   *stats.Counter
	ResponseTotalUncompressedBytes *stats.Counter

	// 5 request-side counters (always-zero in MVP since request_direction_config
	// silent-ignored; registered for byte-equivalent stat scrape with reference
	// Envoy per ADR-0129 §Decision (vi)).
	RequestCompressed             *stats.Counter
	RequestContentLengthTooSmall  *stats.Counter
	RequestNotCompressed          *stats.Counter
	RequestTotalCompressedBytes   *stats.Counter
	RequestTotalUncompressedBytes *stats.Counter
}

// filter is the per-instance per-stream filter state. Per ADR-0071's single-
// goroutine-per-stream invariant, the per-instance state is race-free without
// synchronization. config + stats pointers are closure-captured at New time
// and are immutable post-construction.
//
// Per ADR-0129 §Decision (iv): the SAME *filter instance implements both
// StreamDecoderFilter and StreamEncoderFilter; per-stream state set in
// DecodeHeaders is consumed in EncodeHeaders / EncodeData.
//
// Per-stream state fields land as later tasks consume them (mirrors phase-13
// buffer Task 2 PROGRESS precedent — the `unused` linter flags fields not yet
// referenced; they are added when first-referenced):
//   - acceptedEncoding + acceptHeaderClassification + perRoute + passthrough:
//     Task 5 (DecodeHeaders body per SPEC §6.4).
//   - willCompress: Task 6 (EncodeHeaders body per SPEC §6.6); consumed by
//     EncodeData at Task 7.
//
// Task 2 ships STUB iteration-protocol method bodies (Continue / DataContinue /
// TrailersContinue); Tasks 5-7 land the real bodies and the per-stream fields.
type filter struct {
	config *compiledConfig
	stats  *filterStats

	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	// Per-stream state populated by DecodeHeaders (Task 5) per SPEC §6.4.
	//
	//   - acceptedEncoding: the codec token the AE parser selected ("gzip" /
	//     "" / "identity" / etc.); consumed by EncodeHeaders skip-decision (Task 6).
	//   - acceptHeaderClassification: one of the 6 §6.4 classification tokens
	//     {"compressor_used", "overshadowed", "identity", "wildcard",
	//     "no_accept_header", "not_valid"}; drives the 6 header_* counter
	//     increments at EncodeHeaders time (Task 8).
	//   - perRoute: the parsed per-route TPFC (or nil); cached once at
	//     DecodeHeaders to avoid re-parsing on each callsite.
	//   - passthrough: set to true on per-route disabled; EncodeHeaders
	//     short-circuits on this flag per ADR-0125 amendment §(viii) wholly-
	//     inactive semantic.
	acceptedEncoding           string
	acceptHeaderClassification string
	perRoute                   *compiledPerRoute
	passthrough                bool

	// willCompress is set to true by EncodeHeaders on the compress path (Task
	// 6) and consumed by EncodeData (Task 7) to decide whether to gzip-encode
	// the body via OverwriteBody. Per SPEC §6.6 + §6.7: only true when the
	// 11-bucket skip-decision returned the empty skipReason ("" — compress
	// path); all skip paths leave it false.
	willCompress bool
}

// Statically assert the both-sides interface conformance per ADR-0129
// §Decision (iv).
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// New is the HTTPFilterFactory exposed at boot per SPEC §6.1. New PGV-mirrors
// the Compressor proto's required-fields per ADR-0130 §Decision (ii)-(vi):
// non-nil compressor_library; gzip-only TypeURL; well-formed Gzip codec
// payload. The factory closure returns a both-sides HTTPFilter with the
// SAME *filter instance servicing both Decoder and Encoder per ADR-0129
// §Decision (iv).
//
// Stats registration happens once per factory call (HCM-build-time, per
// stat_prefix) via newFilterStats — the 17-counter wiring per ADR-0132
// §Decision (i)+(ii)+(v).
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, fmt.Errorf("compressor: invalid typed_config: nil")
	}
	cfg := &compressorv3.Compressor{}
	if err := tc.UnmarshalTo(cfg); err != nil {
		return nil, fmt.Errorf("compressor: invalid typed_config: %w", err)
	}
	gzipCfg, libraryName, err := unmarshalCompressorLibrary(cfg.GetCompressorLibrary())
	if err != nil {
		return nil, err
	}
	compiled, err := buildCompiledConfig(cfg, gzipCfg, libraryName)
	if err != nil {
		return nil, err
	}
	fStats := newFilterStats(ctx.Stats, ctx.StatPrefix, libraryName)
	return func() envoyhttp.HTTPFilter {
		f := &filter{config: compiled, stats: fStats}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // SAME *filter per ADR-0129 §Decision (iv).
		}
	}, nil
}

// unmarshalCompressorLibrary validates the compressor_library TypedExtensionConfig
// and dispatches to the codec-library compiler (gzip-only MVP). Per ADR-0130
// §Decision (ii)-(iii): non-nil library required; TypeURL must be Gzip;
// envoy-go-only parse-rejection for non-Gzip TypeURLs with explicit error
// wording.
//
// Returns (*compiledGzipConfig, library.Name, error). The library.Name is
// embedded in the 17-counter stat namespace per ADR-0132 §Decision (i);
// empty allowed per D5.
func unmarshalCompressorLibrary(library *corev3.TypedExtensionConfig) (*compiledGzipConfig, string, error) {
	if library == nil {
		return nil, "", fmt.Errorf("compressor: compressor_library is required")
	}
	tc := library.GetTypedConfig()
	if tc == nil {
		return nil, "", fmt.Errorf("compressor: compressor_library.typed_config is required")
	}
	if tc.GetTypeUrl() != gzipLibraryTypeURL {
		return nil, "", fmt.Errorf(
			"compressor: unsupported compressor_library TypeURL %q; phase-14 MVP supports only %s",
			tc.GetTypeUrl(), gzipLibraryTypeURL,
		)
	}
	gzipPB := &gzipv3.Gzip{}
	if err := tc.UnmarshalTo(gzipPB); err != nil {
		return nil, "", fmt.Errorf("compressor: invalid Gzip typed_config: %w", err)
	}
	compiled, err := buildCompiledGzipConfig(gzipPB)
	if err != nil {
		return nil, "", err
	}
	return compiled, library.GetName(), nil
}

// buildCompiledConfig projects the parsed Compressor proto onto the internal
// compiledConfig shape per SPEC §6.2 + ADR-0130 §Decision (i). Defaults per
// SPEC §11.1 (8-entry content_type list when unset/empty) + SPEC §11.9
// (min_content_length=30 when unset) + SPEC §11.2 (empty
// uncompressible_response_codes set when unset).
//
// Top-level deprecated mirrors (content_length, content_type,
// disable_on_etag_header, remove_accept_encoding_header, runtime_enabled) +
// choose_first + request_direction_config + response_direction_config.
// common_config.enabled + status_header_enabled are SILENT-IGNORED per
// ADR-0040 + §1.1 amendments 1-3.
func buildCompiledConfig(
	cfg *compressorv3.Compressor,
	gzipCfg *compiledGzipConfig,
	libraryName string,
) (*compiledConfig, error) {
	cc := &compiledConfig{
		libraryName:                 libraryName,
		gzip:                        gzipCfg,
		minContentLength:            defaultMinContentLength,
		contentTypes:                defaultContentTypes,
		uncompressibleResponseCodes: map[uint32]struct{}{},
	}
	rdc := cfg.GetResponseDirectionConfig()
	if rdc == nil {
		return cc, nil
	}
	// disable_on_etag_header + remove_accept_encoding_header (direct fields on RDC).
	cc.disableOnEtagHeader = rdc.GetDisableOnEtagHeader()
	cc.removeAcceptEncodingHeader = rdc.GetRemoveAcceptEncodingHeader()
	// common_config: min_content_length + content_type (enabled is silent-
	// ignored at parse + runtime per §1.1 amendment 2).
	cc2 := rdc.GetCommonConfig()
	if cc2 == nil {
		return cc, nil
	}
	if mcl := cc2.GetMinContentLength(); mcl != nil {
		cc.minContentLength = mcl.GetValue()
	}
	if ct := cc2.GetContentType(); len(ct) > 0 {
		// Non-empty list replaces the default wholesale; per SPEC §11.6 the
		// matching algorithm is lowercase prefix-match (lands at Task 6).
		cc.contentTypes = append([]string(nil), ct...)
	}
	return cc, nil
}

// buildCompiledGzipConfig maps the Gzip proto onto compiledGzipConfig per
// ADR-0130 §Decision (iv) (compression_level enum → numeric int) + §Decision
// (v) (compression_strategy: HUFFMAN_ONLY honored, others collapse to default).
//
// Silent-ignored Gzip fields (memory_level, window_bits, chunk_size) are
// neither read nor stored — Go compress/gzip does not expose libz-equivalent
// knobs per ADR-0130 §Consequences.
func buildCompiledGzipConfig(g *gzipv3.Gzip) (*compiledGzipConfig, error) {
	out := &compiledGzipConfig{}
	switch g.GetCompressionLevel() {
	case gzipv3.Gzip_DEFAULT_COMPRESSION:
		out.level = gzip.DefaultCompression // -1
	case gzipv3.Gzip_BEST_SPEED: // aliases COMPRESSION_LEVEL_1 == 1
		out.level = gzip.BestSpeed
	case gzipv3.Gzip_COMPRESSION_LEVEL_2:
		out.level = 2
	case gzipv3.Gzip_COMPRESSION_LEVEL_3:
		out.level = 3
	case gzipv3.Gzip_COMPRESSION_LEVEL_4:
		out.level = 4
	case gzipv3.Gzip_COMPRESSION_LEVEL_5:
		out.level = 5
	case gzipv3.Gzip_COMPRESSION_LEVEL_6:
		out.level = 6
	case gzipv3.Gzip_COMPRESSION_LEVEL_7:
		out.level = 7
	case gzipv3.Gzip_COMPRESSION_LEVEL_8:
		out.level = 8
	case gzipv3.Gzip_BEST_COMPRESSION: // aliases COMPRESSION_LEVEL_9 == 9
		out.level = gzip.BestCompression
	default:
		// Defensive: enum value outside [0, 9]. proto3 enum default is 0
		// (DEFAULT_COMPRESSION); unknown values would have been rejected at
		// proto-decode time. Fall back to DEFAULT_COMPRESSION rather than
		// erroring — silent-tolerant per ADR-0040.
		out.level = gzip.DefaultCompression
	}
	out.huffmanOnly = g.GetCompressionStrategy() == gzipv3.Gzip_HUFFMAN_ONLY
	return out, nil
}

// parsePerRoute validates and compiles a CompressorPerRoute proto.Message per
// SPEC §6.3 + ADR-0125 amendment §(viii). Called at HCM-build time from
// registry.go's per-tier-TPFC parse pass for each CompressorPerRoute entry on
// Route / VirtualHost / RouteConfiguration scopes.
//
// PGV-mirror discipline per ADR-0130 §Decision (vi):
//   - oneof empty (neither disabled nor overrides set) → defensive rejection
//     (PGV validate.required catches at proto-decode time; defensive re-check
//     for unit-test coverage).
//   - disabled: false → defensive rejection (PGV bool.const = true catches;
//     defensive re-check).
//   - both fields set → rejected by JSON→proto decoder BEFORE this function
//     fires (mirrors phase-13 buffer P3 + §11.13 empirical pin).
//
// Per-route surface is NARROWER than listener-level per §1.1 amendment 4: only
// remove_accept_encoding_header is overridable on the response_direction_config
// path. The v1.32.4 proto's CompressorOverrides exposes ONLY
// response_direction_config (no compressor_library field); per-route library
// swap is structurally impossible at this version and even on v1.37.2 the
// library swap is silent-ignored at parse + runtime per §1.1 amendment 4 +
// D7.
func parsePerRoute(perRoute proto.Message) (*compiledPerRoute, error) {
	cfg, ok := perRoute.(*compressorv3.CompressorPerRoute)
	if !ok {
		return nil, fmt.Errorf("compressor per-route: expected *CompressorPerRoute, got %T", perRoute)
	}
	switch override := cfg.GetOverride().(type) {
	case *compressorv3.CompressorPerRoute_Disabled:
		if !override.Disabled {
			return nil, fmt.Errorf("compressor per-route: disabled must be true (PGV bool.const violation)")
		}
		return &compiledPerRoute{disabled: true}, nil
	case *compressorv3.CompressorPerRoute_Overrides:
		ov := override.Overrides
		if ov == nil {
			// Empty overrides — wholly equivalent to no per-route entry.
			return &compiledPerRoute{}, nil
		}
		var rmae *bool
		if rdc := ov.GetResponseDirectionConfig(); rdc != nil {
			if v := rdc.GetRemoveAcceptEncodingHeader(); v != nil {
				b := v.GetValue()
				rmae = &b
			}
		}
		return &compiledPerRoute{removeAcceptEncodingHeaderOverride: rmae}, nil
	case nil:
		return nil, fmt.Errorf("compressor per-route: override oneof is required (neither disabled nor overrides set)")
	default:
		return nil, fmt.Errorf("compressor per-route: unknown override case %T", override)
	}
}

// newFilterStats registers the 17 compressor counters on the supplied
// *stats.Registry at the namespace path
//
//	http.<hcmStatPrefix>.compressor.<libraryName>.gzip.[response.|request.]<counter>
//
// per ADR-0132 §Decision (i)+(ii)+(v). The 17 counters split:
//
//   - 6 Accept-Encoding-cluster counters + 1 not_compressed_etag counter
//     registered with NO direction infix (`compressor.<lib>.gzip.<counter>`
//     directly per SPEC §11.5 verbatim probeA evidence).
//   - 5 response_* counters registered with the `response.` infix (active in
//     MVP).
//   - 5 request_* counters registered with the `request.` infix (always-zero
//     in MVP per ADR-0132 §Decision (vii) twin-series discipline; registered
//     for byte-equivalent stat scrape with reference Envoy).
//
// Empty libraryName produces a consecutive-dots path
// `compressor..gzip.<counter>` per planner-time decision 5 (D5 settlement);
// the existing Rule SN2 flatten (ADR-0061) renders this as
// `envoy_http_compressor__gzip_<counter>` double-underscore Prometheus form
// per ADR-0132 §Decision (v). This function MUST NOT collapse the empty
// segment — phase-14 mirrors reference Envoy's namespace shape verbatim.
//
// Nil-tolerant: returns nil if reg == nil (per ADR-0085 nil-tolerance pattern;
// non-stat-bearing test code paths). Production paths through New supply a
// non-nil *stats.Registry from FactoryCtx; the returned *filterStats has all
// 17 fields populated with non-nil *stats.Counter values.
//
// Called exactly once per HCM-build-time per stat_prefix per HCM. Mirrors the
// phase-11 localratelimit newFilterStats + phase-12 csrf newFilterStats
// precedents at a wider counter surface (17 vs 4 vs 3).
func newFilterStats(reg *stats.Registry, hcmStatPrefix string, libraryName string) *filterStats {
	if reg == nil {
		return nil
	}
	// Build the per-instance namespace prefix once. The fmt.Sprintf surfaces
	// the consecutive-dots D5 shape when libraryName == "" — DO NOT collapse.
	prefix := fmt.Sprintf("http.%s.compressor.%s.gzip.", hcmStatPrefix, libraryName)
	return &filterStats{
		// 6 Accept-Encoding-cluster counters (NO direction infix).
		HeaderCompressorOvershadowed: reg.NewCounter(prefix + "header_compressor_overshadowed"),
		HeaderCompressorUsed:         reg.NewCounter(prefix + "header_compressor_used"),
		HeaderIdentity:               reg.NewCounter(prefix + "header_identity"),
		HeaderNotValid:               reg.NewCounter(prefix + "header_not_valid"),
		HeaderWildcard:               reg.NewCounter(prefix + "header_wildcard"),
		NoAcceptHeader:               reg.NewCounter(prefix + "no_accept_header"),
		// 1 ETag-skip counter (NO direction infix).
		NotCompressedEtag: reg.NewCounter(prefix + "not_compressed_etag"),
		// 5 response-side counters (response. infix; active in MVP).
		ResponseCompressed:             reg.NewCounter(prefix + "response.compressed"),
		ResponseContentLengthTooSmall:  reg.NewCounter(prefix + "response.content_length_too_small"),
		ResponseNotCompressed:          reg.NewCounter(prefix + "response.not_compressed"),
		ResponseTotalCompressedBytes:   reg.NewCounter(prefix + "response.total_compressed_bytes"),
		ResponseTotalUncompressedBytes: reg.NewCounter(prefix + "response.total_uncompressed_bytes"),
		// 5 request-side counters (request. infix; always-zero in MVP per
		// ADR-0132 §Decision (vii) twin-series discipline).
		RequestCompressed:             reg.NewCounter(prefix + "request.compressed"),
		RequestContentLengthTooSmall:  reg.NewCounter(prefix + "request.content_length_too_small"),
		RequestNotCompressed:          reg.NewCounter(prefix + "request.not_compressed"),
		RequestTotalCompressedBytes:   reg.NewCounter(prefix + "request.total_compressed_bytes"),
		RequestTotalUncompressedBytes: reg.NewCounter(prefix + "request.total_uncompressed_bytes"),
	}
}

// --- Filter method skeletons per ADR-0129 §Decision (iv) ---
// Bodies are STUBS at Task 2; Tasks 5-7 land the real iteration-protocol
// behavior per SPEC §6.4-§6.8.

// SetDecoderCallbacks stores the framework-supplied decoder callbacks for
// later use by DecodeHeaders (per-route resolution via RequestRouteConfig).
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) {
	f.dcb = cb
}

// SetEncoderCallbacks stores the framework-supplied encoder callbacks for
// later use by EncodeData (body replacement via OverwriteBody — lands at
// Task 4 framework primitive + Task 7 consumer).
func (f *filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) {
	f.ecb = cb
}

// DecodeHeaders parses Accept-Encoding, resolves per-route TPFC, and (when the
// effective remove_accept_encoding_header is true) strips the AE header from
// the upstream request per SPEC §6.4 + §11.8 + §1.1 amendment 4. Step order:
//
//  1. Parse AE via parseAcceptEncoding; cache (acceptedEncoding, classification).
//     EncodeHeaders (Task 6) consumes acceptedEncoding to drive the skip-decision;
//     filterStats (Task 8) consumes classification to drive the 6 header_*
//     counters per SPEC §11.5.
//
//  2. Resolve per-route TPFC via f.dcb.RequestRouteConfig() → parsePerRoute →
//     *compiledPerRoute; cache on f.perRoute. The framework returns
//     proto.Message (a *compressorv3.CompressorPerRoute); we parse it here on
//     a per-stream basis matching the buffer / csrf / fault precedent. On
//     parse error we fall through to listener-level config (per-route invalid
//     entries are caught by reference Envoy at PGV time — differential
//     equivalence preserved for well-formed configs).
//
//  3. Per-route disabled bypass: set f.passthrough=true and return Continue
//     WITHOUT applying any AE strip — per ADR-0125 amendment §(viii) +
//     SPEC §5.5, a disabled route is wholly inactive: no header mutation, no
//     body inspection, no stat increments. EncodeHeaders (Task 6) short-
//     circuits on this flag.
//
//  4. Compute effective rmAE: per-route override (when non-nil) wins over the
//     listener-level setting; otherwise listener-level applies.
//
//  5. When effective rmAE is true, delete the Accept-Encoding header from the
//     request so the upstream cannot vary on it. The AE parser already cached
//     the negotiated encoding token at step 1, so EncodeHeaders still selects
//     gzip on the response path even though the header is gone upstream.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Parse AE and cache.
	f.acceptedEncoding, f.acceptHeaderClassification = parseAcceptEncoding(headers.Get("Accept-Encoding"))

	// Step 2: Resolve per-route TPFC. The framework returns a proto.Message;
	// we parse it through parsePerRoute (mirrors buffer.resolveEffective + csrf
	// route-config dispatch). Unparseable per-route → fall through to listener-
	// level config; defensive PGV-mirror discipline.
	if f.dcb != nil {
		if pr := f.dcb.RequestRouteConfig(); pr != nil {
			if cpr, err := parsePerRoute(pr); err == nil {
				f.perRoute = cpr
			}
		}
	}

	// Step 3: Per-route disabled bypass — wholly inactive per ADR-0125 amendment §(viii).
	if f.perRoute != nil && f.perRoute.disabled {
		f.passthrough = true
		return envoyhttp.Continue
	}

	// Step 4: Effective rmAE — per-route override wins over listener-level.
	effectiveRmAE := f.config.removeAcceptEncodingHeader
	if f.perRoute != nil && f.perRoute.removeAcceptEncodingHeaderOverride != nil {
		effectiveRmAE = *f.perRoute.removeAcceptEncodingHeaderOverride
	}

	// Step 5: Maybe-strip Accept-Encoding.
	if effectiveRmAE {
		headers.Del("Accept-Encoding")
	}

	return envoyhttp.Continue
}

// DecodeData passes through unconditionally — the compressor filter never
// inspects request body bytes (gzip-only response-side MVP per SPEC §6.5).
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers passes through unconditionally per SPEC §6.5.
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders runs the 11-bucket skip-decision sequence + (on compress path)
// the Content-Encoding/Vary/ETag/Content-Length header mutations per SPEC
// §6.6 + §11.5 + §11.10 + §11.15 + §1.1 amendments 5-6. Step order:
//
//  1. Bucket 0 — per-route disabled passthrough: no header mutation, no
//     counter, return Continue immediately (wholly inactive per ADR-0125
//     amendment §(viii)).
//  2. Compute effective config (per-route override of rmAE applied; other
//     fields inherit listener-level per §1.1 amendment 4).
//  3. Compute skipReason via computeSkipReason (11-bucket sequence in Envoy
//     compressor_filter.cc order: AE-side {no_accept_header / identity /
//     wildcard_uncompressed / not_valid / overshadowed} → server-side
//     {uncompressible_status / already_encoded / etag_disabled / no_transform
//     / content_type_mismatch / content_length_too_small_known}).
//  4. Increment the header_* counter per acceptHeaderClassification (mapping
//     verbatim per SPEC §6.6 step 4 + §11.5 6-counter Accept-Encoding cluster).
//  5. On skip → increment response_not_compressed (+ secondary counter for
//     etag_disabled / content_length_too_small_known); inject Vary on AE-side
//     skip buckets per §11.15 trichotomy; return Continue.
//  6. On compress path → set Content-Encoding=gzip; appendVaryAcceptEncoding
//     (always-append even on Vary: * per §11.10); mode-a maybeStripStrongEtag
//     when !disableOnEtagHeader (preserves weak-ETag per §11.7); strip
//     Content-Length (writeH1Reply rewrites unconditionally at wire time);
//     set willCompress=true (consumed by EncodeData at Task 7).
func (f *filter) EncodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Step 1: Per-route disabled bypass — wholly inactive per ADR-0125 amendment §(viii).
	if f.passthrough {
		return envoyhttp.Continue
	}

	// Step 2: Effective config (per-route override of rmAE; other fields inherit).
	effective := f.effectiveConfig()

	// Step 3: 11-bucket skip-decision sequence (first-hit wins per §6.6 + §11.15).
	skipReason := computeSkipReason(headers, effective, f.acceptHeaderClassification, f.acceptedEncoding, endStream)

	// Step 4: header_* counter dispatch per §11.5 6-counter cluster (incremented
	// on EVERY non-passthrough request — both compress + skip paths).
	switch f.acceptHeaderClassification {
	case "no_accept_header":
		incCounter(f.stats.NoAcceptHeader)
	case "wildcard":
		incCounter(f.stats.HeaderWildcard)
	case "identity":
		incCounter(f.stats.HeaderIdentity)
	case "not_valid":
		incCounter(f.stats.HeaderNotValid)
	case "compressor_used":
		incCounter(f.stats.HeaderCompressorUsed)
	case "overshadowed":
		incCounter(f.stats.HeaderCompressorOvershadowed)
	}

	if skipReason != "" {
		// Step 5a: skip-path counter increments.
		incCounter(f.stats.ResponseNotCompressed)
		switch skipReason {
		case "etag_disabled":
			incCounter(f.stats.NotCompressedEtag)
		case "content_length_too_small_known":
			incCounter(f.stats.ResponseContentLengthTooSmall)
		}
		// Step 5b: §11.15 trichotomy — Vary INJECTED on AE-side skip paths
		// (no_accept_header / identity / wildcard_uncompressed / not_valid /
		// overshadowed); NOT injected on server-side skip paths
		// (uncompressible_status / already_encoded / etag_disabled /
		// no_transform / content_type_mismatch / content_length_too_small_known).
		switch skipReason {
		case "no_accept_header", "identity", "wildcard_uncompressed", "not_valid", "overshadowed":
			appendVaryAcceptEncoding(headers)
		}
		return envoyhttp.Continue
	}

	// Step 6: Compress path header mutations.
	headers.Set("Content-Encoding", "gzip")
	appendVaryAcceptEncoding(headers) // ALWAYS append, even on existing Vary: * per §11.10
	if !effective.disableOnEtagHeader {
		// Mode-a per §1.1 amendment 6 + §11.7: strip strong-ETag; preserve weak-ETag.
		maybeStripStrongEtag(headers)
	}
	// Content-Length strip on compress path per SPEC §6.6: the framework's
	// writeH1Reply (codec.go:87-89) rewrites Content-Length unconditionally to
	// len(body) at wire time, so this strip is defensive but observable in
	// unit tests (and mirrors Envoy's compressor_filter.cc behavior on the
	// compress path).
	headers.Del("Content-Length")

	f.willCompress = true
	return envoyhttp.Continue
}

// effectiveConfig returns the *compiledConfig that applies to this stream per
// SPEC §6.6 + §1.1 amendment 4. When no per-route override is present (or the
// per-route override does not set removeAcceptEncodingHeaderOverride), the
// listener-level config is returned pointer-equal (immutable). When the
// per-route override is set, the listener-level config is shallow-cloned with
// removeAcceptEncodingHeader overridden, so the listener-level *compiledConfig
// stays immutable across streams (ADR-0125 amendment §(viii) wholesale-not-merge
// semantic within the override-field envelope).
//
// Per §1.1 amendment 4: the per-route surface is NARROWER than listener-level —
// only removeAcceptEncodingHeader is overridable. All other fields
// (minContentLength, contentTypes, disableOnEtagHeader,
// uncompressibleResponseCodes, gzip) inherit listener-level by structural
// impossibility of override.
func (f *filter) effectiveConfig() *compiledConfig {
	if f.perRoute == nil || f.perRoute.removeAcceptEncodingHeaderOverride == nil {
		return f.config
	}
	cloned := *f.config
	cloned.removeAcceptEncodingHeader = *f.perRoute.removeAcceptEncodingHeaderOverride
	return &cloned
}

// computeSkipReason runs the 11-bucket skip-decision sequence per SPEC §6.6 +
// §11.15 + Envoy compressor_filter.cc bucket order. First-hit wins; returns
// the empty string on the compress path.
//
// AE-side buckets (consult the cached acceptHeaderClassification — Vary
// INJECTED on these per §11.15 trichotomy):
//  1. no_accept_header        — client had no AE header
//  2. identity                — AE preferred identity (no compression desired)
//  3. wildcard_uncompressed   — AE wildcard, no configured codec selectable (vacuous in gzip-only MVP)
//  4. not_valid               — malformed AE / out-of-range q-value
//  5. overshadowed            — AE present but no configured codec selectable (e.g. AE: br with gzip-only)
//
// Server-side buckets (consult response headers — NO Vary on these):
//  6. uncompressible_status   — response status in uncompressible_response_codes set
//  7. already_encoded         — response Content-Encoding non-empty (incl. identity per §11.11)
//  8. etag_disabled           — effective.disableOnEtagHeader && ETag present (mode-b)
//  9. no_transform            — Cache-Control contains no-transform token (per §11.12)
//  10. content_type_mismatch  — response Content-Type prefix not in effective.contentTypes (per §11.6)
//  11. content_length_too_small_known — Content-Length present and < effective.minContentLength (per §11.9)
//
// endStream is currently unused; future EncodeData-side late-content-length
// gating (per §6.7) keys off it.
func computeSkipReason(
	headers http.Header,
	effective *compiledConfig,
	cls string,
	acceptedEncoding string,
	_ bool,
) string {
	// AE-side buckets — derived from acceptHeaderClassification cached at
	// DecodeHeaders time per SPEC §6.4.
	switch cls {
	case "no_accept_header":
		return "no_accept_header"
	case "identity":
		return "identity"
	case "not_valid":
		return "not_valid"
	case "overshadowed":
		return "overshadowed"
	case "wildcard":
		// In gzip-only MVP the wildcard always maps to gzip (parseAcceptEncoding
		// returns ("gzip", "wildcard") for Accept-Encoding: *). The
		// wildcard_uncompressed bucket only fires if wildcard would prefer
		// identity over configured codecs — vacuous in MVP.
		if acceptedEncoding == "" {
			return "wildcard_uncompressed"
		}
	}

	// Defensive: if parseAcceptEncoding produced empty acceptedEncoding under
	// a classification not covered above, treat as "no codec selectable" =
	// not_valid AE-side skip.
	if acceptedEncoding == "" {
		return "not_valid"
	}

	// Server-side buckets — consult response headers.

	// Bucket 6: uncompressible status. The :status pseudo-header is not
	// normally in the http.Header for the encode chain in envoy-go's framework
	// (the codec sets it at wire-write time), but tests can drive this path
	// by injecting :status. In production MVP the v1.32.4 proto cannot
	// populate uncompressibleResponseCodes, so this bucket is unreachable.
	if len(effective.uncompressibleResponseCodes) > 0 {
		if s := headers.Get(":status"); s != "" {
			if code, err := strconv.ParseUint(s, 10, 32); err == nil {
				if _, ok := effective.uncompressibleResponseCodes[uint32(code)]; ok {
					return "uncompressible_status"
				}
			}
		}
	}

	// Bucket 7: already encoded. Per §11.11: ANY non-empty Content-Encoding
	// (including identity) skips compression.
	if headers.Get("Content-Encoding") != "" {
		return "already_encoded"
	}

	// Bucket 8: etag_disabled (mode-b). Per §11.7 + §1.1 amendment 6: when
	// disable_on_etag_header is true, ANY ETag presence (strong or weak)
	// triggers skip.
	if effective.disableOnEtagHeader && headers.Get("Etag") != "" {
		return "etag_disabled"
	}

	// Bucket 9: Cache-Control: no-transform. Per §11.12: tokenize by `,`;
	// case-insensitive token match per RFC 7234 §5.2.2.4.
	if cc := headers.Get("Cache-Control"); cc != "" {
		for _, tok := range strings.Split(cc, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "no-transform") {
				return "no_transform"
			}
		}
	}

	// Bucket 10: content-type mismatch. Per §11.6: case-insensitive prefix
	// match on the media-type/subtype prefix (strip parameters after `;`).
	if !contentTypeMatches(headers.Get("Content-Type"), effective.contentTypes) {
		return "content_type_mismatch"
	}

	// Bucket 11: content-length too small (known). Per §11.9: when
	// Content-Length is present (parseable uint) and below minContentLength,
	// skip. Unset/unparseable Content-Length defers the gate to EncodeData
	// (Task 7 late-gate per §6.7).
	if cl := headers.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseUint(cl, 10, 32); err == nil {
			if uint32(n) < effective.minContentLength {
				return "content_length_too_small_known"
			}
		}
	}

	// Compress path.
	return ""
}

// contentTypeMatches returns true iff the response Content-Type matches any
// entry in the configured content_type list per SPEC §11.6 case-insensitive
// prefix-match semantics. The media-type/subtype prefix is extracted by
// trimming whitespace then truncating at the first `;` or whitespace; the
// matcher compares case-insensitively against the configured list.
func contentTypeMatches(responseCT string, list []string) bool {
	if responseCT == "" || len(list) == 0 {
		return false
	}
	// Extract media-type/subtype prefix (strip parameters after `;` or whitespace).
	prefix := strings.TrimSpace(responseCT)
	if i := strings.IndexAny(prefix, "; \t"); i >= 0 {
		prefix = prefix[:i]
	}
	for _, allowed := range list {
		if strings.EqualFold(prefix, allowed) {
			return true
		}
	}
	return false
}

// appendVaryAcceptEncoding appends `Accept-Encoding` to the Vary header per
// SPEC §11.10 + §1.1 amendment 5:
//   - No existing Vary → set Vary: Accept-Encoding.
//   - Existing Vary contains Accept-Encoding (case-insensitive token-match) →
//     no-op (idempotent on re-traversal).
//   - Existing Vary does NOT contain Accept-Encoding → append with
//     comma-space, regardless of contents (APPEND ALWAYS, even on existing
//     Vary: *; per §11.10 REFUTES BRAINSTORM §2.8).
func appendVaryAcceptEncoding(headers http.Header) {
	existing := headers.Get("Vary")
	if existing == "" {
		headers.Set("Vary", "Accept-Encoding")
		return
	}
	for _, tok := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "Accept-Encoding") {
			return
		}
	}
	headers.Set("Vary", existing+", Accept-Encoding")
}

// maybeStripStrongEtag implements ETag mode-a per SPEC §11.7 + §1.1 amendment 6:
// on the compress path with disable_on_etag_header=false (default), the strong
// ETag (`^"[^"]*"$`) is STRIPPED because the strong-validator semantic cannot
// survive gzip re-encoding (RFC 7232 §2.3: strong validators must change when
// entity bytes change). Weak ETag (`^W/"[^"]*"$`) is PRESERVED — the
// weak-validator "semantically equivalent" semantic survives.
//
// Defensive: malformed ETag values that match neither pattern are PRESERVED
// verbatim (mirrors Envoy's behavior under unexpected ETag formats per
// compressor_filter.cc).
func maybeStripStrongEtag(headers http.Header) {
	val := headers.Get("Etag")
	if val == "" {
		return
	}
	if strongEtagPattern.MatchString(val) {
		headers.Del("Etag")
		return
	}
	if weakEtagPattern.MatchString(val) {
		// Weak validator preserved verbatim per SPEC §6.6 line 918 + §11.7.
		return
	}
	// Malformed ETag → preserved verbatim (defensive; mirrors compressor_filter.cc).
}

// incCounter is a nil-tolerant Inc wrapper per ADR-0085. Production paths
// through New supply a non-nil *stats.Registry to newFilterStats; the
// resulting *filterStats has all 17 fields populated with non-nil
// *stats.Counter values, so the nil-guard is structurally vacuous on the
// production path. The wrapper is retained for two reasons:
//
//  1. Defensive: future per-route SHARED-stats discipline (ADR-0132
//     §Decision (iv) + ADR-0125 amendment §(viii)) could in principle widen
//     to lazy-registration paths analogous to phase-11 localratelimit's
//     NewCounterIfAbsent; the nil-guard keeps the call sites uniform.
//  2. Test ergonomics: callers may construct *filter directly (without going
//     through New) with stats: nil; the wrapper avoids a nil-deref panic
//     before the structural-guard `f.stats != nil` check at the EncodeData
//     byte-counter .Add call sites.
//
// Per-call cost is one predictable nil-check; negligible vs the
// atomic.Uint64.Add in Inc.
func incCounter(c *stats.Counter) {
	if c != nil {
		c.Inc()
	}
}

// EncodeData runs the Path B body algorithm per SPEC §6.7 + §11.9 + §11.14 +
// ADR-0131 §Decision (i)+(vi)+(vii). On the compress path (willCompress=true,
// !passthrough), it gzip-encodes the full body in one shot via
// gzip.NewWriterLevel(buf, level) and emits the compressed bytes via the
// framework primitive f.ecb.OverwriteBody (landed at Task 4). Step order:
//
//  1. Passthrough / willCompress=false short-circuit → DataContinue with no
//     mutation, no counter (per-route disabled OR EncodeHeaders skip-path).
//  2. Defensive endStream=false guard → DataContinue. The current encode-chain
//     framework always invokes EncodeData once with endStream=true (per
//     connection.go RunEncodeData call sites + h2dispatch.go); this branch is
//     defensive for any future streaming-encode framework change per ADR-0131
//     §Decision (vii) forward-pointer.
//  3. Late min_content_length gate per planner-time decision 4 (D4 settlement).
//     When len(data) < effective.minContentLength at EncodeData time, the
//     compression decision must revert — EncodeHeaders' Content-Encoding/Vary
//     mutations are already on the wire (the late-revert anomaly per SPEC
//     §13.4 + ADR-0131 §Decision (vii); structurally rare since fixture 0016
//     direct_response routes carry CL on action headers so EncodeHeaders'
//     bucket-10 catches them). Per D4: BOTH response_content_length_too_small
//     AND response_not_compressed increment.
//  4. Compress in one shot. gzip.NewWriterLevel cannot fail under the level
//     range [-1, 9] guaranteed by ADR-0130 §Decision (iv) — the err returns are
//     defensive (level out-of-range would have been rejected at config-parse
//     time). On any defensive error, response_not_compressed +1 + DataContinue.
//  5. Emit via f.ecb.OverwriteBody(compressed) — the framework primitive per
//     ADR-0131 §Decision (vi). The chain dispatch substitutes resp.Body before
//     the wire-write path consumes it.
//  6. Increment 3 success counters: response_compressed +1,
//     response_total_uncompressed_bytes += len(data),
//     response_total_compressed_bytes += len(compressed).
//
// Per §1.1 amendment 4: per-route override does NOT carry minContentLength
// (proto's ResponseDirectionOverrides exposes only remove_accept_encoding_header),
// so effectiveConfig() returns the listener-level minContentLength here. The
// effectiveConfig() call is symmetric with EncodeHeaders' use for future-
// proofing in case the per-route surface widens.
//
// Per ADR-0130 §Decision (v): HUFFMAN_ONLY compression_strategy is silent-
// ignored at runtime — Go compress/gzip exposes only level via NewWriterLevel,
// not a strategy knob. The huffmanOnly bit on compiledGzipConfig is parsed but
// never read by this function.
func (f *filter) EncodeData(data []byte, endStream bool) envoyhttp.FilterDataStatus {
	// Step 1: Passthrough / willCompress=false short-circuit.
	if f.passthrough || !f.willCompress {
		return envoyhttp.DataContinue
	}
	// Step 2: Defensive endStream=false guard.
	if !endStream {
		return envoyhttp.DataContinue
	}
	// Step 3: Late min_content_length gate per D4 settlement — BOTH counters
	// increment on below-threshold late-revert anomaly.
	effective := f.effectiveConfig()
	if uint32(len(data)) < effective.minContentLength {
		incCounter(f.stats.ResponseContentLengthTooSmall)
		incCounter(f.stats.ResponseNotCompressed)
		return envoyhttp.DataContinue
	}
	// Step 4: Compress body in one shot. Defensive error branches mirror SPEC
	// §6.7 verbatim — gzip.NewWriterLevel can only fail on out-of-range level,
	// which ADR-0130 §Decision (iv) prevents at config-parse time.
	var buf bytes.Buffer
	gzWriter, err := gzip.NewWriterLevel(&buf, f.config.gzip.level)
	if err != nil {
		incCounter(f.stats.ResponseNotCompressed)
		return envoyhttp.DataContinue
	}
	if _, err := gzWriter.Write(data); err != nil {
		_ = gzWriter.Close()
		incCounter(f.stats.ResponseNotCompressed)
		return envoyhttp.DataContinue
	}
	if err := gzWriter.Close(); err != nil {
		incCounter(f.stats.ResponseNotCompressed)
		return envoyhttp.DataContinue
	}
	compressed := buf.Bytes()
	// Step 5: Emit via framework primitive (ADR-0131 §Decision (vi)).
	f.ecb.OverwriteBody(compressed)
	// Step 6: Counter increments — 3 response_* counters.
	incCounter(f.stats.ResponseCompressed)
	if f.stats != nil && f.stats.ResponseTotalUncompressedBytes != nil {
		f.stats.ResponseTotalUncompressedBytes.Add(uint64(len(data)))
	}
	if f.stats != nil && f.stats.ResponseTotalCompressedBytes != nil {
		f.stats.ResponseTotalCompressedBytes.Add(uint64(len(compressed)))
	}
	return envoyhttp.DataContinue
}

// EncodeTrailers passes through unconditionally per SPEC §6.8: no trailer-side
// state, no counter increments — compressor's wire-shape divergence-window is
// confined to headers + body per ADR-0131 §Decision (i).
func (f *filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy is a no-op per SPEC §6.8: no per-stream resources to clean up
// (no timers, no goroutines, no buffer references held beyond the *filter
// lifetime). The OverwriteBody-side body-override field on *FilterChain dies
// with the chain at Task 4.
func (f *filter) OnDestroy() {}
