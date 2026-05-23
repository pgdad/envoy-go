package ratelimit

// headers_test.go — TDD coverage + D-RL9 byte-pin for the X-RateLimit
// DRAFT_VERSION_03 byte construction in headers.go per phase-24.2 PLAN Task 5
// (parent SPEC §4.7 + AMEND-8 + ADR-0197 X-RateLimit slice). The byte format
// is sourced directly from upstream Envoy
// `source/extensions/filters/http/ratelimit/ratelimit_headers.cc:13-65` at
// the v1.37.2 tag (ADR-0008 pin via ENVOY_TARGET.md).
//
// # D-RL9 outcome (recorded at this Task's PROGRESS.md entry)
//
// **CONFIRMED — byte-match.** The upstream source is unambiguous on
// tie-breakers (strict `<` ⇒ first-equal-minimum wins per insertion order),
// rounding (no `nanos` participation — `.seconds()` emitted verbatim as a
// decimal int64), and `name=` quoting (bare `"%s"` substitution via
// absl::Substitute; no escaping of embedded chars). ADR-0202 escape-valve
// (target #2 per the PLAN) UNCONSUMED.
//
// # Tests
//
//   - TestHeaders_DRAFT_VERSION_03_ByteShape: 9 sub-rows covering:
//   - single descriptor (UNKNOWN unit ⇒ no quota-policy segment)
//   - single descriptor (SECOND unit ⇒ `;w=1` segment)
//   - single descriptor (SECOND unit + name ⇒ `;w=1;name="<n>"` segment)
//   - multi-descriptor: MIN selection by limit_remaining
//   - multi-descriptor: MIN selection + quota-policy iterates ALL
//   - unit→seconds: SECOND/MINUTE/HOUR/DAY/WEEK/MONTH/YEAR
//   - UNKNOWN unit in mixed list ⇒ no segment for that descriptor
//   - statuses without current_limit are SKIPPED entirely
//   - empty / nil input ⇒ ok=false
//   - TestHeaders_MIN_Selection_TieBreaker: equal limit_remaining ⇒ FIRST
//     equal-minimum wins (strict `<` semantic per upstream).
//   - TestHeaders_UnitToSeconds_Table: exhaustive table covering all 8 enum
//     values (UNKNOWN + 7 named) byte-pinned per ratelimit_headers.cc:65-85.
//   - TestHeaders_NameQuoting_BareNoEscape: documentary — the bare
//     absl::Substitute discipline means embedded `"` in `name` would
//     produce malformed output (envoy-go preserves byte parity verbatim).

import (
	"testing"

	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ----------------------------------------------------------------------------
// Helper: makeStatus builds a DescriptorStatus with the given fields.
// ----------------------------------------------------------------------------

func makeStatus(rpu uint32, unit ratelimitservicev3.RateLimitResponse_RateLimit_Unit, name string, limitRemaining uint32, resetSec int64) *ratelimitservicev3.RateLimitResponse_DescriptorStatus {
	return &ratelimitservicev3.RateLimitResponse_DescriptorStatus{
		CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
			RequestsPerUnit: rpu,
			Unit:            unit,
			Name:            name,
		},
		LimitRemaining:     limitRemaining,
		DurationUntilReset: &durationpb.Duration{Seconds: resetSec},
	}
}

// ----------------------------------------------------------------------------
// Test: TestHeaders_DRAFT_VERSION_03_ByteShape
// ----------------------------------------------------------------------------

// TestHeaders_DRAFT_VERSION_03_ByteShape pins the byte-exact upstream X-RateLimit
// header values per ratelimit_headers.cc:13-65 (v1.37.2). This is the D-RL9
// byte-pin per PLAN Task 5; ADR-0202 fires if any row fails.
func TestHeaders_DRAFT_VERSION_03_ByteShape(t *testing.T) {
	tests := []struct {
		name          string
		statuses      []*ratelimitservicev3.RateLimitResponse_DescriptorStatus
		wantLimit     string
		wantRemaining string
		wantReset     string
		wantOK        bool
	}{
		{
			name: "single_UNKNOWN_unit_no_quota_segment",
			// rpu=10, unit=UNKNOWN ⇒ window=0 ⇒ NO quota-policy segment.
			// Limit header is just the bare MIN.rpu without any suffix.
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_UNKNOWN, "", 5, 30),
			},
			wantLimit:     "10",
			wantRemaining: "5",
			wantReset:     "30",
			wantOK:        true,
		},
		{
			name: "single_SECOND_unit_no_name",
			// rpu=10, unit=SECOND ⇒ window=1 ⇒ segment `, 10;w=1` (no name).
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "", 5, 1),
			},
			wantLimit:     "10, 10;w=1",
			wantRemaining: "5",
			wantReset:     "1",
			wantOK:        true,
		},
		{
			name: "single_SECOND_unit_with_name",
			// rpu=10, unit=SECOND, name="per-ip" ⇒ `, 10;w=1;name="per-ip"`.
			// Matches the upstream comment example at ratelimit_headers.cc:32:
			//   `, 10;w=1;name="per-ip", 1000;w=3600`
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "per-ip", 5, 1),
			},
			wantLimit:     `10, 10;w=1;name="per-ip"`,
			wantRemaining: "5",
			wantReset:     "1",
			wantOK:        true,
		},
		{
			name: "multi_descriptor_MIN_selection_by_limit_remaining",
			// First descriptor: rpu=100, unit=SECOND, remaining=50
			// Second descriptor: rpu=10, unit=SECOND, remaining=3 (MIN)
			// MIN selection ⇒ second descriptor wins; limit prefix = 10.
			// Quota-policy iterates ALL: `, 100;w=1, 10;w=1`.
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(100, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "", 50, 1),
				makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "", 3, 7),
			},
			wantLimit:     "10, 100;w=1, 10;w=1",
			wantRemaining: "3",
			wantReset:     "7",
			wantOK:        true,
		},
		{
			name: "multi_descriptor_upstream_comment_example",
			// Mirrors the comment-block example at ratelimit_headers.cc:32:
			//   `, 10;w=1;name="per-ip", 1000;w=3600`
			// Two descriptors: (rpu=10, SECOND, name=per-ip, remaining=2)
			//                  (rpu=1000, HOUR, no-name, remaining=999)
			// MIN selection: first wins (remaining=2 < 999).
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "per-ip", 2, 1),
				makeStatus(1000, ratelimitservicev3.RateLimitResponse_RateLimit_HOUR, "", 999, 3600),
			},
			wantLimit:     `10, 10;w=1;name="per-ip", 1000;w=3600`,
			wantRemaining: "2",
			wantReset:     "1",
			wantOK:        true,
		},
		{
			name: "mixed_UNKNOWN_unit_skips_segment_but_participates_in_MIN",
			// First: rpu=20, UNKNOWN unit, remaining=1 (MIN, NO quota segment)
			// Second: rpu=5, SECOND unit, remaining=10 (segment ", 5;w=1")
			// MIN wins by first; quota-policy iterates both but only second
			// contributes (UNKNOWN ⇒ no segment).
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				makeStatus(20, ratelimitservicev3.RateLimitResponse_RateLimit_UNKNOWN, "", 1, 0),
				makeStatus(5, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "", 10, 1),
			},
			wantLimit:     "20, 5;w=1",
			wantRemaining: "1",
			wantReset:     "0",
			wantOK:        true,
		},
		{
			name: "status_without_current_limit_is_skipped",
			// Status with nil CurrentLimit is fully skipped (does not
			// participate in MIN AND contributes no quota-policy segment).
			// Verifies upstream "if (!status.has_current_limit()) continue;".
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				// nil CurrentLimit AND nil DurationUntilReset — skipped.
				{LimitRemaining: 0},
				makeStatus(7, ratelimitservicev3.RateLimitResponse_RateLimit_MINUTE, "", 3, 60),
			},
			wantLimit:     "7, 7;w=60",
			wantRemaining: "3",
			wantReset:     "60",
			wantOK:        true,
		},
		{
			name:          "empty_statuses_no_emission",
			statuses:      nil,
			wantLimit:     "",
			wantRemaining: "",
			wantReset:     "",
			wantOK:        false,
		},
		{
			name: "all_statuses_lack_current_limit_no_emission",
			// All-skipped ⇒ min_remaining_limit_status absl::optional stays
			// unset upstream; envoy-go returns ok=false (no headers emitted).
			statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
				{LimitRemaining: 0},
				{LimitRemaining: 5},
			},
			wantLimit:     "",
			wantRemaining: "",
			wantReset:     "",
			wantOK:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, remaining, reset, ok := buildXRateLimitHeaders(tt.statuses)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if limit != tt.wantLimit {
				t.Errorf("x-ratelimit-limit:\n  got  %q\n  want %q", limit, tt.wantLimit)
			}
			if remaining != tt.wantRemaining {
				t.Errorf("x-ratelimit-remaining:\n  got  %q\n  want %q", remaining, tt.wantRemaining)
			}
			if reset != tt.wantReset {
				t.Errorf("x-ratelimit-reset:\n  got  %q\n  want %q", reset, tt.wantReset)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test: TestHeaders_MIN_Selection_TieBreaker
// ----------------------------------------------------------------------------

// TestHeaders_MIN_Selection_TieBreaker pins the strict `<` MIN selection
// per upstream ratelimit_headers.cc:27-29: equal `limit_remaining` values
// ⇒ the FIRST equal-minimum status wins per insertion order. Insertion order
// = descriptor-list order = AMEND-6 action-list order.
func TestHeaders_MIN_Selection_TieBreaker(t *testing.T) {
	// Three descriptors all with limit_remaining=5; rpu differs so we can
	// detect which one wins as MIN. First in order has rpu=10; second rpu=20;
	// third rpu=30. Strict `<` ⇒ first wins.
	statuses := []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
		makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "first", 5, 1),
		makeStatus(20, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "second", 5, 2),
		makeStatus(30, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, "third", 5, 3),
	}

	limit, remaining, reset, ok := buildXRateLimitHeaders(statuses)

	if !ok {
		t.Fatal("buildXRateLimitHeaders: got ok=false, want true (non-empty statuses with current_limit)")
	}
	// MIN is the FIRST descriptor: rpu=10, remaining=5, reset=1 (from "first").
	// Quota-policy iterates all 3: "10;w=1;name=first" + "20;w=1;name=second" + "30;w=1;name=third".
	wantLimit := `10, 10;w=1;name="first", 20;w=1;name="second", 30;w=1;name="third"`
	if limit != wantLimit {
		t.Errorf("x-ratelimit-limit:\n  got  %q\n  want %q (first equal-minimum wins per strict `<`)", limit, wantLimit)
	}
	if remaining != "5" {
		t.Errorf("x-ratelimit-remaining: got %q, want %q", remaining, "5")
	}
	if reset != "1" {
		t.Errorf("x-ratelimit-reset: got %q, want %q (from FIRST equal-minimum status)", reset, "1")
	}
}

// ----------------------------------------------------------------------------
// Test: TestHeaders_UnitToSeconds_Table
// ----------------------------------------------------------------------------

// TestHeaders_UnitToSeconds_Table pins the unit→seconds map per upstream
// `XRateLimitHeaderUtils::convertRateLimitUnit` (ratelimit_headers.cc:65-85).
// Covers ALL 8 enum values (UNKNOWN=0 + the 7 named units).
func TestHeaders_UnitToSeconds_Table(t *testing.T) {
	tests := []struct {
		unit ratelimitservicev3.RateLimitResponse_RateLimit_Unit
		want uint32
	}{
		{ratelimitservicev3.RateLimitResponse_RateLimit_UNKNOWN, 0},
		{ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, 1},
		{ratelimitservicev3.RateLimitResponse_RateLimit_MINUTE, 60},
		{ratelimitservicev3.RateLimitResponse_RateLimit_HOUR, 3600},
		{ratelimitservicev3.RateLimitResponse_RateLimit_DAY, 86400},
		{ratelimitservicev3.RateLimitResponse_RateLimit_WEEK, 604800},
		{ratelimitservicev3.RateLimitResponse_RateLimit_MONTH, 2592000},
		{ratelimitservicev3.RateLimitResponse_RateLimit_YEAR, 31536000},
	}
	for _, tt := range tests {
		t.Run(tt.unit.String(), func(t *testing.T) {
			if got := unitToSeconds(tt.unit); got != tt.want {
				t.Errorf("unitToSeconds(%v): got %d, want %d", tt.unit, got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Test: TestHeaders_NameQuoting_BareNoEscape
// ----------------------------------------------------------------------------

// TestHeaders_NameQuoting_BareNoEscape documents the bare absl::Substitute
// quoting discipline (no escaping of embedded `"` chars). Operator-authored
// config text with an embedded `"` produces malformed output upstream too —
// envoy-go preserves byte parity. The test pins the byte-format for a
// `name` containing a `"` character so any future "add escaping" code-change
// would surface as a test failure (a documented behavior-shift that would
// require an ADR amendment).
func TestHeaders_NameQuoting_BareNoEscape(t *testing.T) {
	statuses := []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
		makeStatus(10, ratelimitservicev3.RateLimitResponse_RateLimit_SECOND, `nm"with"quotes`, 5, 1),
	}
	limit, _, _, ok := buildXRateLimitHeaders(statuses)
	if !ok {
		t.Fatal("buildXRateLimitHeaders: ok=false; want true")
	}
	// Verbatim bare substitution: `name="<nm-with-embedded-double-quotes>"`.
	// No backslash escaping. Matches upstream `, $0;w=$1;name="$2"`.
	want := `10, 10;w=1;name="nm"with"quotes"`
	if limit != want {
		t.Errorf("x-ratelimit-limit:\n  got  %q\n  want %q (bare substitution — no escaping)", limit, want)
	}
}

// ----------------------------------------------------------------------------
// Test: TestHeaders_ConstantsByteStable
// ----------------------------------------------------------------------------

// TestHeaders_ConstantsByteStable pins the three X-RateLimit header NAME
// strings byte-stable per upstream ratelimit_headers.h:15-17 (a
// `Http::LowerCaseString`). HTTP headers are case-insensitive but envoy-go
// preserves the upstream lowercase form for byte-stable differential output.
func TestHeaders_ConstantsByteStable(t *testing.T) {
	if headerXRateLimitLimit != "x-ratelimit-limit" {
		t.Errorf("headerXRateLimitLimit: got %q, want %q", headerXRateLimitLimit, "x-ratelimit-limit")
	}
	if headerXRateLimitRemaining != "x-ratelimit-remaining" {
		t.Errorf("headerXRateLimitRemaining: got %q, want %q", headerXRateLimitRemaining, "x-ratelimit-remaining")
	}
	if headerXRateLimitReset != "x-ratelimit-reset" {
		t.Errorf("headerXRateLimitReset: got %q, want %q", headerXRateLimitReset, "x-ratelimit-reset")
	}
}
