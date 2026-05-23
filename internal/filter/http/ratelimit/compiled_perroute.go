package ratelimit

// compiled_perroute.go — the `RateLimitPerRoute` TPFC compile per parent
// SPEC §5.3 + §6.7 + ADR-0199 (the NEW 10th canonical per-route shape,
// "data-only-with-vh-inclusion-enum") + ADR-0125 §(xv) AMENDMENT 9 → 10.
//
// # Per-route shape (AMEND-4 / AMEND-5 — empirical pin §11 D3)
//
// `envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute` carries
// FOUR fields per v1.32.4 `rate_limit.pb.go`:
//
//  1. `vh_rate_limits` (`VhRateLimitsOptions{OVERRIDE=0[default],INCLUDE=1,
//     IGNORE=2}`, field 1) — drives cross-tier descriptor composition.
//     Honored at Task 4 (Axis-B walk). The compiled output preserves it
//     verbatim.
//  2. `override_option` (`OverrideOptions{DEFAULT, OVERRIDE_POLICY,
//     INCLUDE_POLICY, IGNORE_POLICY}`, field 2) — marked
//     `[#not-implemented-hide:]` at `rate_limit.pb.go:410`, NEVER read by
//     the C++ filter (AMEND-4 REFUTES the BRAINSTORM framing). envoy-go
//     PARSE-ACCEPTS-but-IGNORES per upstream parity: the validator does
//     not error; the compiled output exposes NO override-option field
//     (the projection drops it).
//  3. `rate_limits[]` (`[]config.route.v3.RateLimit`, field 3) — the
//     Axis-A embedded-policy list. When non-empty, the request-time
//     engine walks ONLY this list and the route-table policy + the
//     `vh_rate_limits` switch are bypassed (unconditional early-return
//     per `ratelimit.cc` `populateRateLimitDescriptors`). REUSES the
//     existing 24.1 `ValidateRouteRateLimits` for §5.2 PARSE-REJECT arms
//     (disable_key / extension / dynamic_metadata + per-policy stage > 10).
//  4. `domain` (string, field 4) — per-route domain override. Empty ⇒
//     use the filter-config domain (Task 4 wires the wins-discipline at
//     request time). Empty is PARSE-ACCEPTED here.
//
// # TPFC registration (ADR-0110 single-chokepoint mirror)
//
// Mirrors header_mutation + oauth2 + lua's precedent (header_mutation.go:
// 184-193; lua.go:226-241): the filter-package exports
// `RegisterPerRouteValidator(reg)` which `cmd/envoy-go/main.go` calls
// BEFORE `httpReg.Freeze()`. The framework's `BuildPerRouteConfig`
// (perroute.go:108-133) dispatches the registered `validatePerRouteRateLimit`
// against each parsed `proto.Message` at each tier
// (Route, VirtualHost, RouteConfiguration); a non-nil return surfaces as
// a boot-time HCM-parse-time error per ADR-0072 boot-time-fail-fast.
//
// # Decode-time projection (`compilePerRouteForRequest`)
//
// The framework's `dcb.RequestRouteConfig()` returns a parsed
// `proto.Message` (or nil). The filter's decode-side (Task 4) calls
// `compilePerRouteForRequest(msg)` to project the validated proto into a
// `*compiledPerRoute` opaque carrying ONLY the request-time-relevant
// fields {vhRateLimits, rateLimits, domain} — NO override-option.
// ADR-0085 nil-tolerant (nil + wrong-type both return nil).
//
// # Cross-references
//
//   - ADR-0199 (the 10th canonical per-route shape — anchored at this Task)
//   - ADR-0125 §(xv) AMENDMENT 9 → 10 (anchored at this Task)
//   - ADR-0080 (byte-stable wording — the §5.2 PARSE-REJECT arms reused via
//     `ValidateRouteRateLimits`)
//   - ADR-0085 (nil-tolerance — `compilePerRouteForRequest` returns nil on
//     nil + wrong-type input)
//   - ADR-0110 (single-chokepoint per-route validator — `RegisterPerRouteValidator`
//     registers with the boot-time `*HTTPRegistry`)
//   - ADR-0143 SN1 (byte-stable TypeURL pin)
//   - ADR-0198 (the BASE route-table `rate_limits` surface — Axis-A is
//     route-additional and bypasses the BASE policy when non-empty)
//   - parent SPEC §5.3 + §6.7 + §1.1 AMEND-4/5

import (
	"errors"
	"fmt"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"google.golang.org/protobuf/proto"
)

// PerRouteTypeURL is the canonical envoy.extensions.filters.http.ratelimit.v3
// .RateLimitPerRoute typed_config TypeURL per ADR-0143 SN1. Byte-exact;
// regression-pinned at TestPerRouteTypeURL_ByteStable. Consumed by the
// HCM's BuildPerRouteConfig dispatch (perroute.go:78-83 unmarshals the
// `*anypb.Any` to the proto.Message type indicated by the TypeURL).
const PerRouteTypeURL = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"

// errPerRouteVhRateLimitsOutOfRange is the defensive PARSE-REJECT for a
// `vh_rate_limits` enum varint that escapes the 3 in-proto names
// (OVERRIDE=0, INCLUDE=1, IGNORE=2). This is impossible against the
// canonical proto bindings (a future upstream extension to the enum would
// require the local proto-binding version to bump and a new arm to land
// in `walkPerRouteVhRateLimits`), but the validator must never panic
// per ADR-0018 — a 4th value would be a defensive PARSE-REJECT here
// rather than a silent honor-as-zero.
var errPerRouteVhRateLimitsOutOfRange = errors.New("ratelimit: vh_rate_limits enum value out of range (expected OVERRIDE|INCLUDE|IGNORE)")

// compiledPerRoute is the per-route opaque type produced by the
// `compilePerRouteForRequest` projection. Holds ONLY the request-time-
// relevant fields per AMEND-4:
//
//   - vhRateLimits — the Axis-B inclusion enum (Task 4 consumer).
//   - rateLimits   — the Axis-A embedded-policy list (Task 4 consumer;
//     early-return wins per AMEND-4).
//   - domain       — the per-route domain override (Task 4 consumer;
//     empty ⇒ use filter-config domain).
//
// NO override-option field per AMEND-4 INERT discipline (upstream-parity).
type compiledPerRoute struct {
	vhRateLimits ratelimitfilterv3.RateLimitPerRoute_VhRateLimitsOptions
	rateLimits   []*routev3.RateLimit
	domain       string
}

// validatePerRouteRateLimit is the ADR-0110 single-chokepoint per-route
// validator for the ratelimit filter. Registered by
// `RegisterPerRouteValidator` against the *HTTPRegistry; invoked by
// `BuildPerRouteConfig` at HCM-build time against the per-tier parsed
// `proto.Message` (Route, VirtualHost, RouteConfiguration).
//
// Validation arms:
//
//  1. Type-assert to *ratelimitfilterv3.RateLimitPerRoute. Wrong type is
//     a defensive silent no-op (mirrors header_mutation precedent at
//     header_mutation.go:204 — the framework's unmarshaler at
//     perroute.go:78-83 already type-asserts; reaching this validator
//     with the wrong type is a framework bug, not a config error).
//  2. `vh_rate_limits` enum bounds (must be one of OVERRIDE=0,
//     INCLUDE=1, IGNORE=2 — the 3 in-proto values).
//  3. `rate_limits[]` slice REUSES `ValidateRouteRateLimits` (the 24.1
//     Task-3 §5.2 PARSE-REJECT validator — disable_key / extension /
//     dynamic_metadata + per-policy stage > 10). Byte-stable wording
//     preserved verbatim per ADR-0080.
//  4. `override_option` PARSE-ACCEPTS (AMEND-4 INERT — never validated,
//     never rejected, never compiled into the per-route opaque).
//  5. `domain` PARSE-ACCEPTS (empty + non-empty — empty defers to the
//     filter-config domain at request time per AMEND-4).
//
// Returns the FIRST violation encountered; nil + valid input both return nil.
func validatePerRouteRateLimit(msg proto.Message) error {
	pr, ok := msg.(*ratelimitfilterv3.RateLimitPerRoute)
	if !ok {
		// Defensive: framework type-asserts upstream of this callback;
		// a wrong-type reach is unreachable in production. Silent no-op
		// matches header_mutation.go:204 precedent — surfacing here as
		// an error would obscure the framework bug.
		return nil
	}

	// Arm 2: vh_rate_limits enum bounds. The proto enum carries exactly 3
	// values (OVERRIDE=0, INCLUDE=1, IGNORE=2 per AMEND-5 + §11 D3); a
	// 4th value is impossible against the canonical proto bindings but
	// defensively rejected for ADR-0018 never-panic posture.
	switch pr.GetVhRateLimits() {
	case ratelimitfilterv3.RateLimitPerRoute_OVERRIDE,
		ratelimitfilterv3.RateLimitPerRoute_INCLUDE,
		ratelimitfilterv3.RateLimitPerRoute_IGNORE:
		// In-range — proceed.
	default:
		return fmt.Errorf("%w: got %d", errPerRouteVhRateLimitsOutOfRange, pr.GetVhRateLimits())
	}

	// Arm 3: embedded rate_limits[] slice — REUSE the 24.1 Task-3 validator.
	// Same byte-stable §5.2 PARSE-REJECT arms apply (disable_key / extension /
	// dynamic_metadata + per-policy stage > 10). The proto comment at
	// rate_limit.pb.go:420-429 notes that the per-route embedded list is
	// "Not all configuration fields ... supported" — the upstream filter
	// ignores disable_key + dynamic_metadata + stage + override here too;
	// envoy-go REJECTS them at parse time per ADR-0200 + ADR-0080.
	if err := ValidateRouteRateLimits(pr.GetRateLimits()); err != nil {
		return err
	}

	// Arm 4: override_option PARSE-ACCEPTS unconditionally (AMEND-4 INERT —
	// the field is `[#not-implemented-hide:]` at rate_limit.pb.go:410 and
	// NEVER read by the C++ filter; envoy-go matches upstream parity).
	// Arm 5: domain PARSE-ACCEPTS unconditionally (empty + non-empty both
	// honored at request time per AMEND-4 — empty defers to filter-config).
	return nil
}

// RegisterPerRouteValidator registers the ratelimit per-route validator
// with the supplied HTTPRegistry. MUST be called BEFORE `httpReg.Freeze()`
// in `cmd/envoy-go/main.go` — the registry rejects registrations after
// Freeze, and `New` is called during listener construction (after Freeze).
// Mirrors the header_mutation + oauth2 + lua precedent.
//
// The validator is the 10th-canonical-per-route shape validator landed at
// phase 24.2 Task 3; see `validatePerRouteRateLimit` above.
//
// Registration ordering: wire `RegisterPerRouteValidator` before
// `httpReg.Register(ratelimit.TypeURL, ratelimit.New)` — the registry
// rejects registrations after Freeze, and `New` runs post-Freeze. The
// header_mutation + lua precedent in main.go places both calls in the same
// block, validators after Register, before Freeze.
func RegisterPerRouteValidator(reg interface {
	RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)
}) {
	reg.RegisterPerRouteValidator(filterName, validatePerRouteRateLimit)
}

// compilePerRouteForRequest projects a per-route `RateLimitPerRoute`
// `proto.Message` into a `*compiledPerRoute` opaque consumed by the
// filter's decode-side at Task 4. Used by the request-time engine when
// `dcb.RequestRouteConfig()` returns a non-nil per-route proto for the
// matched route's tier (Route most-specific override; per ADR-0073
// most-specific wins).
//
// Per-stream compile is sub-microsecond on typical inputs (per AMEND-4 the
// per-route `rate_limits[]` is typically empty or tiny); per-request
// re-compile aligns with the header_mutation precedent at header_mutation.go:
// 258-285 (no caching — the proto.Message is already parsed + validated).
//
// Per ADR-0085 nil-tolerant: nil input returns nil; wrong-type input
// (defensive — should not happen post-HCM-validation per ADR-0110)
// returns nil. Both cases trip the engine's "no per-route override at
// this tier" branch at Task 4.
func compilePerRouteForRequest(msg proto.Message) *compiledPerRoute {
	if msg == nil {
		return nil
	}
	pr, ok := msg.(*ratelimitfilterv3.RateLimitPerRoute)
	if !ok {
		// Defensive: framework guarantees the proto type matches the
		// registered TypeURL, but defensively return nil rather than panic
		// per ADR-0018.
		return nil
	}
	return &compiledPerRoute{
		vhRateLimits: pr.GetVhRateLimits(),
		rateLimits:   pr.GetRateLimits(),
		domain:       pr.GetDomain(),
	}
}
