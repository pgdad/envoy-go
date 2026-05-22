package admission_control

// admission_control.go — the TypeURL constant + per-stream filter struct +
// compile-time interface assertions + New factory per phase-23 SPEC §6 +
// §6.3 + PD-1 + PD-4 + ADR-0071 + ADR-0143 SN1.
//
// # TypeURL (ADR-0143 SN1)
//
// The byte-exact TypeURL for AdmissionControl typed_config is consumed by the
// HCM filter-chain builder at boot time to resolve typed_config Any envelopes
// against the HTTPRegistry's frozen factory map. Any drift surfaces as a
// runtime "no factory registered for type URL" error; regression-pinned at
// TestTypeURL_ByteExact (admission_control_test.go).
//
// # Filter struct + both-sides wiring (PD-4)
//
// The per-stream *filter implements BOTH StreamDecoderFilter AND
// StreamEncoderFilter. The SAME *filter instance is wired as
// `HTTPFilter{Decoder: f, Encoder: f}` at the factory return (per PD-4 +
// adaptive_concurrency precedent). The shared *controller + *filterStats
// pointers are hoisted to the factory-closure level (one per HCM filter chain
// mounting an admission_control filter); each per-request *filter captures
// the shared pointers.
//
// # Encode-side methods
//
// The StreamEncoderFilter methods (SetEncoderCallbacks, EncodeHeaders,
// EncodeData, EncodeTrailers, OnDestroy) are implemented in encode.go per
// SPEC §6.5 + AMEND-10 + AMEND-11 + PD-5 (Task 6). The compile-time
// assertion `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` in this
// file holds because encode.go's methods are on the same *filter type.
//
// # Cross-references
//
//   - ADR-0070 (filter-registration convention; filterName below)
//   - ADR-0071 (two-step factory; HTTPFilterFactory return signature)
//   - ADR-0072 (boot-time-fail-fast)
//   - ADR-0143 SN1 (byte-exact TypeURL pin)
//   - PD-1 (New factory signature — real FactoryCtx shape)
//   - PD-3 (health-check gate arm NOT-MODELED at MVP; Task 11 BEHAVIOR_CONTRACT note)
//   - PD-4 (both-sides filter shape; *controller hoisted to closure level)
//   - SPEC §6.3 (filter struct fields; record + expectGRPCStatusInTrailer)
//   - SPEC §6.4 (DecodeHeaders gate — decode_headers.go)
//   - SPEC §6.5 (EncodeHeaders classification — encode.go)
//   - adaptive_concurrency/adaptive_concurrency.go (nearest precedent for New factory)
//   - adaptive_concurrency/filter.go (nearest precedent for both-sides filter struct)

import (
	"errors"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// filterName is the registered name for the admission_control filter per
// ADR-0070 + SPEC §3.4 boot-registration. Matches the listener config
// `http_filters[].name` string + the HCM-chain dispatch identifier.
const filterName = "envoy.filters.http.admission_control"

// TypeURL is the proto typed_config TypeURL for admission_control per
// ADR-0143 SN1 + the v1.32.4 / v1.37.x proto package
// envoy.extensions.filters.http.admission_control.v3.AdmissionControl.
// Byte-exact constant; regression-pinned at TestTypeURL_ByteExact.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"

// filter is the per-stream admission_control filter instance per SPEC §6.3 +
// PD-4. Owns per-request record + expectGRPCStatusInTrailer bookkeeping; shares
// the *controller + *filterStats pointers (one per HCM filter chain mounting an
// admission_control filter) captured at the New() factory level.
//
// Both-sides filter: implements StreamDecoderFilter (DecodeHeaders / DecodeData /
// DecodeTrailers / SetDecoderCallbacks / OnDestroy — decode_headers.go) AND
// StreamEncoderFilter (EncodeHeaders / EncodeData / EncodeTrailers /
// SetEncoderCallbacks — encode.go Task 6). New() returns
// `HTTPFilter{Decoder: f, Encoder: f}` (wired at Task 5 since Tasks 2-4 were
// already landed; boot-registration + doc.go complete at Task 8).
//
// Field grouping per SPEC §6.3 + PD-4 LOCKED contract:
type filter struct {
	cc         *compiledConfig
	controller *controller
	stats      *filterStats
	cb         envoyhttp.DecoderFilterCallbacks

	// ecb stores the encoder-side callbacks supplied via SetEncoderCallbacks.
	// The encode-path HTTP classification (encode.go) reads the response status
	// via ecb.ResponseStatus() (ADR-0196) — the encode header map does not carry
	// :status, so the framework accessor is the only source of the status code.
	ecb envoyhttp.EncoderFilterCallbacks

	// record is the per-stream classification gate per AMEND-11. Default TRUE
	// at construction; cleared to false on:
	//   - disabled pass-through (gate 1 short-circuit)
	//   - reject path (gate 3 short-circuit)
	// Consulted by the encode-side classification hook (Task 6): when false,
	// the encode side skips classify() (no rqSuccess/rqFailure increment, no
	// recordRequest window update). Prevents health-check, disabled-filter,
	// and rejected requests from polluting the success-rate window.
	record bool

	// expectGRPCStatusInTrailer is set when a gRPC response is detected at
	// EncodeHeaders but the grpc-status header is absent (deferred to trailers
	// per PD-5 + AMEND-10). The encode-side EncodeTrailers hook consults this
	// flag to complete the gRPC classification. Initialized + consumed in
	// encode.go per SPEC §6.5.
	expectGRPCStatusInTrailer bool
}

// Static interface conformance assertions. The *filter implements BOTH the
// decoder and encoder side per SPEC §6.3 + PD-4's
// `HTTPFilter{Decoder: f, Encoder: f}` wiring.
//
// The StreamEncoderFilter assertion is satisfied by encode.go's methods
// (SetEncoderCallbacks, EncodeHeaders, EncodeData, EncodeTrailers, OnDestroy)
// and the shared OnDestroy below — all on the same *filter type.
var (
	_ envoyhttp.StreamDecoderFilter = (*filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*filter)(nil)
)

// New is the HTTPFilterFactory exposed at boot per ADR-0071 + ADR-0072.
//
// Validates tc + calls buildCompiledConfig + registers stats + constructs the
// per-HCM-instance controller + returns the per-stream factory closure.
// Boot-registration lands at Task 8.
//
// Steps (per PD-1 + adaptive_concurrency.New precedent):
//
//  1. tc != nil per ADR-0072 boot-time-fail-fast.
//  2. buildCompiledConfig(tc) — Task 2 PARSE-REJECT + default-application.
//  3. ctx.Stats != nil — required for the 3-counter stat-surface registration.
//  4. newFilterStats(ctx.Stats, "http."+ctx.StatPrefix) — Task 3 3-name registration.
//  5. newController(cc, stats, defaultClock{}, defaultRand{}) — Task 4 controller.
//  6. Return the per-stream FilterInstanceFactory closure.
//
// The returned HTTPFilter has Decoder=f AND Encoder=f — admission_control
// participates on BOTH decode (gate + reject per SPEC §6.4) AND encode
// (classify per SPEC §6.5). The SAME *filter struct implements both
// StreamDecoderFilter + StreamEncoderFilter per the var-block assertions above.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("admission_control: typed_config required")
	}
	cc, err := buildCompiledConfig(tc)
	if err != nil {
		return nil, err
	}
	if ctx.Stats == nil {
		// Boot-time-fail-fast per ADR-0072. The 3-counter stat surface requires
		// a non-nil registry. Production callers per internal/filter/hcm/config.go
		// always supply a non-nil registry per ADR-0061 LBP-1.
		return nil, errors.New("admission_control: ctx.Stats required (HCM-build-time non-nil per ADR-0061 LBP-1)")
	}
	// "http." prepend per Rule SN2 (internal/stats/name.go) — mirrors the
	// adaptive_concurrency factory boundary convention (Task 10 fix) and the
	// phase-20 oauth2 baseStatPrefix precedent.
	st := newFilterStats(ctx.Stats, "http."+ctx.StatPrefix)
	ctrl := newController(cc, st, defaultClock{}, defaultRand{})
	return func() envoyhttp.HTTPFilter {
		f := &filter{
			cc:         cc,
			controller: ctrl,
			stats:      st,
			record:     true, // default per AMEND-11
		}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // admission_control participates on both decode + encode
		}
	}, nil
}

// The StreamEncoderFilter methods (SetEncoderCallbacks, EncodeHeaders,
// EncodeData, EncodeTrailers, OnDestroy) are implemented in encode.go per
// SPEC §6.5 + AMEND-10 + AMEND-11 + PD-5 (Task 6 landing).
