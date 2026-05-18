package adaptive_concurrency

// adaptive_concurrency.go — the boot-time HTTPFilterFactory entry per
// phase-21 SPEC §3.4 + ADR-0072 (boot-time-fail-fast). Wires Tasks 2-8 into
// a fully-functional api.HTTPFilterFactory consumed by the HCM filter-chain
// builder at cmd/envoy-go/main.go.
//
// # Two-step factory pattern (per ADR-0071)
//
// `New(tc, ctx)` runs ONCE at HCM-build time:
//   - Validates tc != nil per ADR-0072.
//   - Builds the *compiledConfig (Task 2 buildCompiledConfig — 12 reachable
//     PARSE-REJECT arms + default-application per AMEND-4).
//   - Constructs the per-HCM-instance *filterStats (Task 4 newFilterStats —
//     7-name HCM-rooted stat surface per SPEC §6.6 + AMEND-3 C2).
//   - Constructs the per-HCM-instance *gradientController (Task 3
//     newGradientController — ONE per HCM filter chain mounting an
//     adaptive_concurrency filter).
//   - Returns the per-stream FilterInstanceFactory closure that allocates
//     a fresh *filter bound to the SHARED *compiledConfig + *gradientController.
//
// # HTTPFilter{Decoder: f, Encoder: f} per SPEC §3.4
//
// adaptive_concurrency participates on BOTH decode + encode sides — the
// SAME *filter struct implements StreamDecoderFilter (DecodeHeaders +
// forwardingDecision + 503 emission per SPEC §6.4 + AMEND-6) AND
// StreamEncoderFilter (EncodeHeaders + recordLatencySample +
// releaseInFlight per SPEC §6.5 + planner-time D11). The chain dispatches
// per-side as appropriate.
//
// # NO RegisterPerRouteValidator (REUSE-by-absence per SPEC §5.4)
//
// The v1.32.4 + v1.37.x proto bindings have NO AdaptiveConcurrencyPerRoute
// message at all — any TPFC entry at route or virtualHost level
// PARSE-REJECTs at proto-deserialization-time via the existing HCM
// framework (no per-route validator function required). FOURTH CONSECUTIVE
// §9 row to skip ADR-0125 amendment (per oauth2 / jwt_authn / extauthz /
// extproc THIRD-CONSECUTIVE precedent recorded at oauth2.go header).
//
// # Cross-references
//
//   - ADR-0070 (filter-registration convention; filterName below).
//   - ADR-0071 (two-step factory; HTTPFilterFactory return signature).
//   - ADR-0072 (boot-time-fail-fast).
//   - ADR-0143 SN1 (byte-exact TypeURL pin).
//   - SPEC §3.4 (boot-registration position — alphabetical between router
//     and bandwidthlimit at cmd/envoy-go/main.go line 125).
//   - SPEC §5.4 (REUSE-by-absence per-route discipline).
//   - SPEC §6.6 + AMEND-3 (7-name HCM-rooted stat surface).
//   - bandwidthlimit precedent (BOTH-sides filter; same Decoder + Encoder
//     wired to the same *filter struct).
//   - oauth2 precedent (most-recent two-step factory + boot-time-fail-fast
//     discipline).

import (
	"errors"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// filterName is the registered name for the adaptive_concurrency filter
// per ADR-0070 + SPEC §3.4 boot-registration. Matches the listener config
// `http_filters[].name` string + the HCM-chain dispatch identifier.
const filterName = "envoy.filters.http.adaptive_concurrency"

// TypeURL is the proto typed_config TypeURL for adaptive_concurrency per
// ADR-0143 SN1 + the v1.32.4 / v1.37.x proto package
// envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency.
// Byte-exact constant; regression-pinned at
// TestTypeURL_ByteExact (adaptive_concurrency_test.go).
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"

// New is the HTTPFilterFactory exposed at boot per ADR-0071 + ADR-0072.
//
// Steps (per the file-header comment):
//
//  1. tc != nil per ADR-0072 boot-time-fail-fast.
//  2. buildCompiledConfig(tc) — Task 2 PARSE-REJECT + default-application.
//  3. ctx.Stats != nil — required for the 7-name stat-surface registration
//     (the controller's constructor writes to filterStats.concurrencyLimit
//     unconditionally per SPEC §4.6 initial-state pin; a nil-stats path
//     would NPE at controller construction). Production callers per
//     `internal/filter/hcm/config.go` always pass a non-nil registry per
//     ADR-0061 LBP-1; tests that exercise New() supply a fresh
//     `stats.NewRegistry()` cheaply.
//  4. newFilterStats(ctx.Stats, ctx.StatPrefix) — Task 4 7-name registration.
//  5. newGradientController(cc, stats, defaultClock{}) — Task 3 controller
//     construction. ONE controller per HCM filter chain mounting an
//     adaptive_concurrency filter (captured into the closure).
//  6. Return the per-stream FilterInstanceFactory closure that allocates a
//     fresh *filter per stream bound to the SHARED *compiledConfig +
//     *gradientController + defaultClock{}.
//
// Per SPEC §3.4 the returned HTTPFilter has Decoder=f AND Encoder=f — the
// adaptive_concurrency filter participates on BOTH decode (forwardingDecision
// + 503 emission per SPEC §6.4 + AMEND-6) AND encode (RTT recording +
// releaseInFlight per SPEC §6.5 + planner-time D11). The SAME *filter
// instance implements both StreamDecoderFilter + StreamEncoderFilter per
// the var-block conformance assertions at filter.go.
//
// NO RegisterPerRouteValidator function — REUSE-by-absence per SPEC §5.4
// (the v1.32.4 / v1.37.x proto has NO AdaptiveConcurrencyPerRoute message;
// any TPFC placement at route/virtualHost level PARSE-REJECTs at proto-
// deserialization-time via the existing HCM framework). FOURTH CONSECUTIVE
// §9 row to skip ADR-0125 amendment.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	if tc == nil {
		return nil, errors.New("adaptive_concurrency: typed_config required")
	}
	cc, err := buildCompiledConfig(tc)
	if err != nil {
		return nil, err
	}
	if ctx.Stats == nil {
		// Boot-time-fail-fast per ADR-0072. The controller's
		// newGradientController writes to filterStats.concurrencyLimit at
		// construction (SPEC §4.6 initial-state pin); a nil-stats path
		// would NPE. Production callers per internal/filter/hcm/config.go
		// always supply a non-nil registry per ADR-0061 LBP-1.
		return nil, errors.New("adaptive_concurrency: ctx.Stats required (HCM-build-time non-nil per ADR-0061 LBP-1)")
	}
	// `http.` prepend per Rule SN2 (internal/stats/name.go) — the HCM
	// passes the bare stat_prefix (e.g. "ingress_http"); the filter
	// prepends "http." so the resulting internal stat names
	// (`http.<stat_prefix>.adaptive_concurrency.gradient_controller.<stat>`)
	// match SN2's prefix discriminator and surface at
	// /stats/prometheus. Mirrors the phase-20 oauth2
	// baseStatPrefix("http." + hcmStatPrefix + ".oauth2.") + phase-09
	// fault registerFaultStats("http." + prefix + ".fault.")
	// convention; surfaced at phase-21 Task 10 (fixture 0025
	// /stats/prometheus scrape exposed the empty
	// envoy_http_adaptive_concurrency_* family — the Task 4 stats.go
	// doc-comment expected the caller to pass `http.<stat_prefix>`
	// but the Task 9 factory was passing the bare ctx.StatPrefix per
	// the doc-comment's literal reading; the Task 10 IMPL closes the
	// loop by prepending here at the factory boundary, preserving the
	// Task 4 stats.go newFilterStats body byte-for-byte).
	stats := newFilterStats(ctx.Stats, "http."+ctx.StatPrefix)
	clock := defaultClock{}
	controller := newGradientController(cc, stats, clock)
	return func() envoyhttp.HTTPFilter {
		f := &filter{
			cc:         cc,
			controller: controller,
			clock:      clock,
		}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // adaptive_concurrency participates on both decode + encode
		}
	}, nil
}
