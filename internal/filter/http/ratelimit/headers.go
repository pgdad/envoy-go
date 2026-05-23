package ratelimit

// headers.go — X-RateLimit DRAFT_VERSION_03 byte construction per parent SPEC
// §4.7 + AMEND-8 + D-RL9. Byte-mirrors upstream Envoy
// `source/extensions/filters/http/ratelimit/ratelimit_headers.cc:13-65`
// (v1.37.2 per ENVOY_TARGET.md / ADR-0008).
//
// # The three X-RateLimit headers (DRAFT_VERSION_03)
//
//	x-ratelimit-limit:     <MIN.requests_per_unit>[, <rpu>;w=<sec>[;name="<n>"]]...
//	x-ratelimit-remaining: <MIN.limit_remaining>
//	x-ratelimit-reset:     <MIN.duration_until_reset.seconds>
//
// MIN selection: by `limit_remaining` smallest across all DescriptorStatus
// entries that carry `current_limit`. Strict-less-than comparison (upstream
// `status.limit_remaining() < min.value().limit_remaining()`) means the FIRST
// equal-minimum status wins per insertion order (= descriptor-list order =
// action-list order per AMEND-6).
//
// Quota-policy suffix: appended PER DescriptorStatus whose
// `current_limit.unit` maps to a NON-ZERO window via `unitToSeconds` (i.e.
// SECOND/MINUTE/HOUR/DAY/WEEK/MONTH/YEAR; UNKNOWN/0 produces NO segment for
// that descriptor). The MIN-status's OWN descriptor ALSO appears as a segment
// (upstream iterates ALL statuses for the quota-policy build, regardless of
// MIN selection). Empty `name` ⇒ omit the `;name="<n>"` clause.
//
// # Unit→seconds map (line-cited against ratelimit_headers.cc:65-85)
//
//	SECOND  = 1
//	MINUTE  = 60
//	HOUR    = 3600          (60 * 60)
//	DAY     = 86400         (24 * 60 * 60)
//	WEEK    = 604800        (7 * 24 * 60 * 60)
//	MONTH   = 2592000       (30 * 24 * 60 * 60)
//	YEAR    = 31536000      (365 * 24 * 60 * 60)
//	UNKNOWN = 0             (and default arm — no segment emitted)
//
// # D-RL9 byte-confirmation outcome
//
// The byte format is derived directly from the upstream source (Option B per
// the PLAN); no ambiguity in the source on tie-breakers, rounding, or
// `name=` quoting. Quoting is bare `\"%s\"` substitution via absl::Substitute
// — no escaping of embedded chars (the proto's `name` field is operator-
// authored config text; embedded `"` chars in upstream would produce malformed
// output too — envoy-go preserves byte parity). ADR-0202 UNCONSUMED.
//
// # Cross-references
//
//   - parent SPEC §4.7 + AMEND-8 (X-RateLimit byte shape)
//   - parent SPEC §6.6 (encode-side injection)
//   - ADR-0197 (X-RateLimit slice; §Decision amendment lands at Task 5)
//   - ADR-0202 (escape-valve target #2; UNCONSUMED — byte-match confirmed)
//   - upstream `source/extensions/filters/http/ratelimit/ratelimit_headers.cc`
//     at v1.37.2 (line-cited above)
//   - upstream `source/extensions/filters/http/common/ratelimit_headers.h`
//     (QuotaPolicyKeys.Window="w" + QuotaPolicyKeys.Name="name")

import (
	"strconv"
	"strings"

	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
)

// ----------------------------------------------------------------------------
// Header-name constants (line-cited against ratelimit_headers.h:15-18).
// ----------------------------------------------------------------------------

const (
	// headerXRateLimitLimit is the canonical lowercase wire name per upstream
	// `XRateLimitHeaderValues::XRateLimitLimit` (a `LowerCaseString` per
	// ratelimit_headers.h:15). HTTP headers are case-insensitive on the wire
	// but envoy-go preserves the upstream lowercase byte form for byte-stable
	// differential output.
	headerXRateLimitLimit = "x-ratelimit-limit"

	// headerXRateLimitRemaining mirrors ratelimit_headers.h:16.
	headerXRateLimitRemaining = "x-ratelimit-remaining"

	// headerXRateLimitReset mirrors ratelimit_headers.h:17.
	headerXRateLimitReset = "x-ratelimit-reset"
)

// ----------------------------------------------------------------------------
// unitToSeconds — the upstream convertRateLimitUnit mirror.
// ----------------------------------------------------------------------------

// unitToSeconds maps a proto `RateLimitResponse.RateLimit.Unit` enum value to
// the corresponding window-in-seconds per upstream
// `XRateLimitHeaderUtils::convertRateLimitUnit` (ratelimit_headers.cc:65-85).
// Returns 0 for UNKNOWN (the proto-zero default) AND for any future-added
// enum value not yet known here — the upstream `default: return 0;` arm.
func unitToSeconds(u ratelimitservicev3.RateLimitResponse_RateLimit_Unit) uint32 {
	switch u {
	case ratelimitservicev3.RateLimitResponse_RateLimit_SECOND:
		return 1
	case ratelimitservicev3.RateLimitResponse_RateLimit_MINUTE:
		return 60
	case ratelimitservicev3.RateLimitResponse_RateLimit_HOUR:
		return 60 * 60
	case ratelimitservicev3.RateLimitResponse_RateLimit_DAY:
		return 24 * 60 * 60
	case ratelimitservicev3.RateLimitResponse_RateLimit_WEEK:
		return 7 * 24 * 60 * 60
	case ratelimitservicev3.RateLimitResponse_RateLimit_MONTH:
		return 30 * 24 * 60 * 60
	case ratelimitservicev3.RateLimitResponse_RateLimit_YEAR:
		return 365 * 24 * 60 * 60
	default:
		// UNKNOWN / future-added enum values — no segment emitted.
		return 0
	}
}

// ----------------------------------------------------------------------------
// buildXRateLimitHeaders — the byte-construction primitive.
// ----------------------------------------------------------------------------

// buildXRateLimitHeaders constructs the three X-RateLimit DRAFT_VERSION_03
// header values from a slice of *RateLimitResponse_DescriptorStatus per
// upstream `XRateLimitHeaderUtils::create`
// (ratelimit_headers.cc:13-65).
//
// Returns (limit, remaining, reset, ok) where `ok` is false iff no status in
// the slice carries `current_limit` (the upstream `min_remaining_limit_status`
// absl::optional stays unset and the function returns an empty header map).
// Callers in that case MUST NOT inject the headers — empty values would mean
// "limit=0, remaining=0, reset=0" which is wire-meaningful and would diverge
// from upstream (which simply omits the headers).
//
// Byte-format precisely mirrors upstream (verified by direct source reading
// at ratelimit_headers.cc:33-63 + v1.37.2 commit-pin per ADR-0008):
//   - `limit`: <MIN.current_limit.requests_per_unit> [+ per-descriptor
//     quota-policy segments `, <rpu>;w=<sec>[;name="<n>"]`]
//   - `remaining`: decimal-string of MIN.limit_remaining (uint32)
//   - `reset`: decimal-string of MIN.duration_until_reset.seconds (int64)
//
// MIN selection: strict-less-than `<` comparison ⇒ first equal-minimum status
// wins per insertion order. Upstream code at ratelimit_headers.cc:27-29:
//
//	if (!min_remaining_limit_status ||
//	    status.limit_remaining() < min_remaining_limit_status.value().limit_remaining()) {
//	  min_remaining_limit_status.emplace(status);
//	}
//
// Quota-policy iteration: upstream iterates ALL statuses (including the MIN-
// status itself) and accumulates segments for each non-zero-unit descriptor.
// The MIN-status's segment thus appears in the suffix even if it is the only
// status.
func buildXRateLimitHeaders(statuses []*ratelimitservicev3.RateLimitResponse_DescriptorStatus) (limit, remaining, reset string, ok bool) {
	if len(statuses) == 0 {
		return "", "", "", false
	}

	var (
		minStatus    *ratelimitservicev3.RateLimitResponse_DescriptorStatus
		quotaPolicyB strings.Builder
	)

	for _, s := range statuses {
		if s == nil {
			continue
		}
		// Upstream: `if (!status.has_current_limit()) { continue; }` —
		// statuses without a current_limit are skipped entirely (do not
		// participate in MIN selection AND do not contribute a quota-policy
		// segment). The proto's CurrentLimit getter returns nil for absent
		// sub-message per Go-proto convention.
		cl := s.GetCurrentLimit()
		if cl == nil {
			continue
		}

		// MIN selection — strict `<` ⇒ first-equal-minimum wins.
		if minStatus == nil || s.GetLimitRemaining() < minStatus.GetLimitRemaining() {
			minStatus = s
		}

		// Quota-policy suffix: append `, <rpu>;w=<sec>[;name="<n>"]` for
		// every descriptor whose unit maps to a non-zero window.
		window := unitToSeconds(cl.GetUnit())
		if window == 0 {
			continue
		}
		// `, <rpu>;w=<sec>` — note the leading ", " (comma + space) per
		// upstream absl::Substitute("$0;$1=$2", rpu, "w", sec) preceded by
		// the literal ", " on every iteration (the upstream pattern is one
		// continuous absl::SubstituteAndAppend with `, $0;$1=$2`).
		quotaPolicyB.WriteString(", ")
		quotaPolicyB.WriteString(strconv.FormatUint(uint64(cl.GetRequestsPerUnit()), 10))
		quotaPolicyB.WriteString(";w=")
		quotaPolicyB.WriteString(strconv.FormatUint(uint64(window), 10))
		if name := cl.GetName(); name != "" {
			// `;name="<n>"` — upstream uses bare absl::Substitute("$0=\"$1\"",
			// "name", name) with no escaping of embedded chars. envoy-go
			// preserves the same byte form for differential parity.
			quotaPolicyB.WriteString(`;name="`)
			quotaPolicyB.WriteString(name)
			quotaPolicyB.WriteString(`"`)
		}
	}

	if minStatus == nil {
		// No status carried current_limit — upstream returns the empty
		// header map without adding any of the three headers.
		return "", "", "", false
	}

	limit = strconv.FormatUint(uint64(minStatus.GetCurrentLimit().GetRequestsPerUnit()), 10) + quotaPolicyB.String()
	remaining = strconv.FormatUint(uint64(minStatus.GetLimitRemaining()), 10)
	// duration_until_reset is a google.protobuf.Duration; upstream emits the
	// `.seconds()` int64 field directly (NO fractional rounding — `nanos` is
	// IGNORED by upstream per ratelimit_headers.cc:62-63). envoy-go mirrors
	// this byte-shape verbatim.
	reset = strconv.FormatInt(minStatus.GetDurationUntilReset().GetSeconds(), 10)
	return limit, remaining, reset, true
}
