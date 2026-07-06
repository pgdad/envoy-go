package ratelimit

// compiled_perroute_test.go — Task 3 unit-test coverage for the
// `RateLimitPerRoute` 10th canonical TPFC compile per parent SPEC §5.3 +
// §6.7 + ADR-0199 (10th canonical) + ADR-0125 §(xv) AMENDMENT 9 → 10.
//
// Six test groups land here (mirrors the §14 testing taxonomy + §15 acceptance):
//
//   - TestPerRouteTypeURL_ByteStable — the per-route TypeURL constant
//     (byte-exact pin per ADR-0143 SN1).
//   - TestPerRoute_VhRateLimits_Honored — the `vh_rate_limits` enum is
//     compiled verbatim for the 3 in-proto values
//     (OVERRIDE=0[default], INCLUDE=1, IGNORE=2). Axis-B Task 4 consumes it.
//   - TestPerRoute_OverrideOption_AcceptedButIgnored — AMEND-4 INERT contract:
//     all 4 `override_option` enum values PARSE-ACCEPT; the compiled output
//     exposes NO override-option field (the projection drops it).
//   - TestPerRoute_Domain_Override — per-route `domain` field captured (empty
//     + non-empty); the request-time wins-discipline lands at Task 4.
//   - TestPerRoute_RateLimits_AxisA_Compile — Axis-A embedded `rate_limits[]`
//     slice compiles into the per-route opaque; `ValidateRouteRateLimits`
//     reuse (24.1 Task 3) — embedded slice with `disable_key` PARSE-REJECTs
//     byte-stable.
//   - TestPerRoute_TPFC_Registration — the HTTPRegistry per-route validator
//     resolves on the canonical filterName; PARSE-ACCEPT happy-path +
//     PARSE-REJECT wrong-proto + PARSE-REJECT embedded-disable_key.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"google.golang.org/protobuf/proto"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// -----------------------------------------------------------------------------
// TestPerRouteTypeURL_ByteStable
// -----------------------------------------------------------------------------

// TestPerRouteTypeURL_ByteStable pins the per-route TypeURL constant byte-exact
// per ADR-0143 SN1. Any change to the constant breaks the regression — this is
// the load-bearing TPFC dispatch key.
func TestPerRouteTypeURL_ByteStable(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"
	if PerRouteTypeURL != want {
		t.Fatalf("PerRouteTypeURL = %q, want %q", PerRouteTypeURL, want)
	}
}

// -----------------------------------------------------------------------------
// TestPerRoute_VhRateLimits_Honored
// -----------------------------------------------------------------------------

// TestPerRoute_VhRateLimits_Honored exercises the 3-value enum surface (the
// only 3 values v1.32.4 ships: OVERRIDE=0[default], INCLUDE=1, IGNORE=2). The
// compiled-per-route projection preserves the enum verbatim — Task 4's Axis-B
// composition consumes it via cc.vhRateLimits.
func TestPerRoute_VhRateLimits_Honored(t *testing.T) {
	rows := []struct {
		name  string
		input ratelimitfilterv3.RateLimitPerRoute_VhRateLimitsOptions
		want  ratelimitfilterv3.RateLimitPerRoute_VhRateLimitsOptions
	}{
		{"OVERRIDE_default", ratelimitfilterv3.RateLimitPerRoute_OVERRIDE, ratelimitfilterv3.RateLimitPerRoute_OVERRIDE},
		{"INCLUDE", ratelimitfilterv3.RateLimitPerRoute_INCLUDE, ratelimitfilterv3.RateLimitPerRoute_INCLUDE},
		{"IGNORE", ratelimitfilterv3.RateLimitPerRoute_IGNORE, ratelimitfilterv3.RateLimitPerRoute_IGNORE},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			pr := &ratelimitfilterv3.RateLimitPerRoute{VhRateLimits: row.input}
			if err := validatePerRouteRateLimit(pr); err != nil {
				t.Fatalf("validatePerRouteRateLimit unexpected err: %v", err)
			}
			got := compilePerRouteForRequest(pr)
			if got == nil {
				t.Fatal("compilePerRouteForRequest returned nil; expected non-nil for valid input")
			}
			if got.vhRateLimits != row.want {
				t.Errorf("vhRateLimits = %v, want %v", got.vhRateLimits, row.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestPerRoute_OverrideOption_AcceptedButIgnored
// -----------------------------------------------------------------------------

// TestPerRoute_OverrideOption_AcceptedButIgnored exercises the AMEND-4 INERT
// contract: each of the 4 in-proto `override_option` enum values PARSE-ACCEPTS
// (no error from validate), AND the compiled-per-route projection drops the
// field (compiledPerRoute carries no override-option field; we assert that by
// confirming the struct shape — see TestCompiledPerRoute_StructShape below for
// the field-roster pin — and by varying the input enum across all 4 values
// + asserting the other fields are unchanged across the variations).
func TestPerRoute_OverrideOption_AcceptedButIgnored(t *testing.T) {
	rows := []struct {
		name  string
		input ratelimitfilterv3.RateLimitPerRoute_OverrideOptions
	}{
		{"DEFAULT", ratelimitfilterv3.RateLimitPerRoute_DEFAULT},
		{"OVERRIDE_POLICY", ratelimitfilterv3.RateLimitPerRoute_OVERRIDE_POLICY},
		{"INCLUDE_POLICY", ratelimitfilterv3.RateLimitPerRoute_INCLUDE_POLICY},
		{"IGNORE_POLICY", ratelimitfilterv3.RateLimitPerRoute_IGNORE_POLICY},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			pr := &ratelimitfilterv3.RateLimitPerRoute{
				VhRateLimits:   ratelimitfilterv3.RateLimitPerRoute_INCLUDE,
				OverrideOption: row.input,
				Domain:         "fixed",
			}
			if err := validatePerRouteRateLimit(pr); err != nil {
				t.Fatalf("validatePerRouteRateLimit(%v) unexpected err: %v", row.input, err)
			}
			got := compilePerRouteForRequest(pr)
			if got == nil {
				t.Fatalf("compilePerRouteForRequest returned nil")
			}
			// The compiled output ignores override_option — the other fields
			// are preserved verbatim across all 4 enum values.
			if got.vhRateLimits != ratelimitfilterv3.RateLimitPerRoute_INCLUDE {
				t.Errorf("vhRateLimits = %v, want INCLUDE (override_option must not perturb it)", got.vhRateLimits)
			}
			if got.domain != "fixed" {
				t.Errorf("domain = %q, want %q", got.domain, "fixed")
			}
		})
	}
}

// TestCompiledPerRoute_StructShape pins the compiledPerRoute struct's
// field-roster: exactly {vhRateLimits, rateLimits, domain} — NO
// override-option field per AMEND-4. Uses reflect to assert NumField == 3
// AND the exact field-name set — a future contributor adding a 4th field
// (especially the AMEND-4-forbidden `overrideOption`) trips this test,
// forcing explicit review per the AMEND-4 INERT contract.
func TestCompiledPerRoute_StructShape(t *testing.T) {
	const wantNumFields = 3
	wantFields := []string{"vhRateLimits", "rateLimits", "domain"}

	tp := reflect.TypeOf(compiledPerRoute{})
	if got := tp.NumField(); got != wantNumFields {
		t.Fatalf("compiledPerRoute.NumField() = %d, want %d — a new field on this struct must be reviewed against AMEND-4 (overrideOption is FORBIDDEN — PARSE-ACCEPTED-but-IGNORED at validate/compile time)", got, wantNumFields)
	}

	gotFields := make([]string, tp.NumField())
	for i := 0; i < tp.NumField(); i++ {
		gotFields[i] = tp.Field(i).Name
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("compiledPerRoute field-roster = %v, want %v (exact order + names) — a field rename or addition must be reviewed against AMEND-4 INERT contract", gotFields, wantFields)
	}
}

// -----------------------------------------------------------------------------
// TestPerRoute_Domain_Override
// -----------------------------------------------------------------------------

// TestPerRoute_Domain_Override exercises the AMEND-4 `domain` field:
// empty + non-empty cases. The compile-time projection captures the field
// verbatim; the actual request-time wins-discipline (per-route overrides
// filter-config) lands at Task 4 — at Task 3 we only verify field capture.
func TestPerRoute_Domain_Override(t *testing.T) {
	rows := []struct {
		name   string
		domain string
	}{
		{"empty_uses_filter_config_domain", ""},
		{"non_empty_per_route_override", "per-route-override"},
		{"single_char", "x"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			pr := &ratelimitfilterv3.RateLimitPerRoute{Domain: row.domain}
			if err := validatePerRouteRateLimit(pr); err != nil {
				t.Fatalf("validatePerRouteRateLimit unexpected err: %v", err)
			}
			got := compilePerRouteForRequest(pr)
			if got == nil {
				t.Fatal("compilePerRouteForRequest returned nil")
			}
			if got.domain != row.domain {
				t.Errorf("domain = %q, want %q", got.domain, row.domain)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestPerRoute_RateLimits_AxisA_Compile
// -----------------------------------------------------------------------------

// TestPerRoute_RateLimits_AxisA_Compile exercises the embedded `rate_limits[]`
// slice (the Axis-A early-return list per AMEND-4 + parent §4.3). Three rows:
//
//  1. Happy-path: a single-action embedded RateLimit with `generic_key`
//     compiles into the per-route opaque verbatim.
//  2. PARSE-REJECT: an embedded RateLimit with non-empty `disable_key`
//     surfaces the §5.2 byte-stable wording (the 24.1 `ValidateRouteRateLimits`
//     reuse). Asserts the byte-stable wording matches `parseRejectRouteRateLimitDisableKey`.
//  3. PARSE-REJECT: an embedded RateLimit with the `extension` action arm
//     surfaces the §5.2 byte-stable wording.
func TestPerRoute_RateLimits_AxisA_Compile(t *testing.T) {
	t.Run("happy_path_generic_key", func(t *testing.T) {
		pr := &ratelimitfilterv3.RateLimitPerRoute{
			RateLimits: []*routev3.RateLimit{
				{Actions: []*routev3.RateLimit_Action{
					{ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{
							DescriptorKey:   "k",
							DescriptorValue: "v",
						},
					}},
				}},
			},
		}
		if err := validatePerRouteRateLimit(pr); err != nil {
			t.Fatalf("validatePerRouteRateLimit unexpected err: %v", err)
		}
		got := compilePerRouteForRequest(pr)
		if got == nil {
			t.Fatal("compilePerRouteForRequest returned nil")
		}
		if len(got.rateLimits) != 1 {
			t.Fatalf("rateLimits len = %d, want 1", len(got.rateLimits))
		}
		// Confirm the slice carries the embedded entry (action carried).
		if len(got.rateLimits[0].GetActions()) != 1 {
			t.Errorf("rateLimits[0].Actions len = %d, want 1", len(got.rateLimits[0].GetActions()))
		}
	})

	t.Run("disable_key_rejected", func(t *testing.T) {
		pr := &ratelimitfilterv3.RateLimitPerRoute{
			RateLimits: []*routev3.RateLimit{
				{DisableKey: "some-key", Actions: []*routev3.RateLimit_Action{
					{ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{
							DescriptorKey:   "k",
							DescriptorValue: "v",
						},
					}},
				}},
			},
		}
		err := validatePerRouteRateLimit(pr)
		if err == nil {
			t.Fatal("validatePerRouteRateLimit should reject embedded disable_key; got nil")
		}
		if err.Error() != parseRejectRouteRateLimitDisableKey {
			t.Errorf("err = %q, want %q (byte-stable ADR-0080)", err.Error(), parseRejectRouteRateLimitDisableKey)
		}
	})

	t.Run("extension_action_rejected", func(t *testing.T) {
		pr := &ratelimitfilterv3.RateLimitPerRoute{
			RateLimits: []*routev3.RateLimit{
				{Actions: []*routev3.RateLimit_Action{
					{ActionSpecifier: &routev3.RateLimit_Action_Extension{
						Extension: &corev3.TypedExtensionConfig{Name: "x"},
					}},
				}},
			},
		}
		err := validatePerRouteRateLimit(pr)
		if err == nil {
			t.Fatal("validatePerRouteRateLimit should reject extension action; got nil")
		}
		if err.Error() != parseRejectRouteRateLimitActionExtension {
			t.Errorf("err = %q, want %q (byte-stable ADR-0080)", err.Error(), parseRejectRouteRateLimitActionExtension)
		}
	})

	t.Run("dynamic_metadata_action_rejected", func(t *testing.T) {
		pr := &ratelimitfilterv3.RateLimitPerRoute{
			RateLimits: []*routev3.RateLimit{
				{Actions: []*routev3.RateLimit_Action{
					{ActionSpecifier: &routev3.RateLimit_Action_DynamicMetadata{
						DynamicMetadata: &routev3.RateLimit_Action_DynamicMetaData{},
					}},
				}},
			},
		}
		err := validatePerRouteRateLimit(pr)
		if err == nil {
			t.Fatal("validatePerRouteRateLimit should reject dynamic_metadata action; got nil")
		}
		if err.Error() != parseRejectRouteRateLimitActionDynamicMetadata {
			t.Errorf("err = %q, want %q (byte-stable ADR-0080)", err.Error(), parseRejectRouteRateLimitActionDynamicMetadata)
		}
	})
}

// -----------------------------------------------------------------------------
// TestPerRoute_TPFC_Registration
// -----------------------------------------------------------------------------

// TestPerRoute_TPFC_Registration exercises the boot-time TPFC validator
// registration path per ADR-0110 single-chokepoint. The exported
// `RegisterPerRouteValidator(reg)` populates the *HTTPRegistry's per-route
// validator map keyed on the canonical `filterName`; Task 5's
// BuildPerRouteConfig drives it at HCM-build time.
//
// Three sub-rows: (a) registration resolves the validator on filterName;
// (b) the validator PARSE-ACCEPTS a valid per-route config; (c) the validator
// PARSE-REJECTS a per-route config with embedded `disable_key` (byte-stable).
func TestPerRoute_TPFC_Registration(t *testing.T) {
	t.Run("validator_resolves_on_filterName", func(t *testing.T) {
		r := envoyhttp.NewHTTPRegistry()
		RegisterPerRouteValidator(r)
		v := r.PerRouteValidator(filterName)
		if v == nil {
			t.Fatal("expected per-route validator registered for filterName; got nil")
		}
	})

	t.Run("validator_accepts_valid", func(t *testing.T) {
		r := envoyhttp.NewHTTPRegistry()
		RegisterPerRouteValidator(r)
		v := r.PerRouteValidator(filterName)
		ok := &ratelimitfilterv3.RateLimitPerRoute{
			VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_INCLUDE,
			Domain:       "perroute",
			RateLimits: []*routev3.RateLimit{
				{Actions: []*routev3.RateLimit_Action{
					{ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{
							DescriptorKey:   "k",
							DescriptorValue: "v",
						},
					}},
				}},
			},
		}
		if err := v(ok); err != nil {
			t.Errorf("validator should accept valid per-route msg; got %v", err)
		}
	})

	t.Run("validator_rejects_embedded_disable_key", func(t *testing.T) {
		r := envoyhttp.NewHTTPRegistry()
		RegisterPerRouteValidator(r)
		v := r.PerRouteValidator(filterName)
		bad := &ratelimitfilterv3.RateLimitPerRoute{
			RateLimits: []*routev3.RateLimit{
				{DisableKey: "k"},
			},
		}
		err := v(bad)
		if err == nil {
			t.Fatal("validator should reject per-route msg with embedded disable_key; got nil")
		}
		if !strings.Contains(err.Error(), "disable_key") {
			t.Errorf("err = %q, want it to mention disable_key", err.Error())
		}
	})

	t.Run("validator_handles_wrong_message_type", func(t *testing.T) {
		// Defensive: the framework should never call the validator with the
		// wrong proto type, but per ADR-0085 we degrade gracefully (return
		// nil — defensive skip mirrors header_mutation's pattern at
		// header_mutation.go:204).
		r := envoyhttp.NewHTTPRegistry()
		RegisterPerRouteValidator(r)
		v := r.PerRouteValidator(filterName)
		var wrong proto.Message = &corev3.TypedExtensionConfig{Name: "wrong"}
		if err := v(wrong); err != nil {
			t.Errorf("validator should silently accept wrong proto type (defensive); got %v", err)
		}
	})
}

// -----------------------------------------------------------------------------
// TestCompilePerRouteForRequest_NilTolerance — ADR-0085 nil-tolerance
// -----------------------------------------------------------------------------

// TestCompilePerRouteForRequest_NilTolerance asserts the request-time projection
// returns nil on nil input + on wrong-type input (defensive — the framework
// type-asserts at BuildPerRouteConfig-time, so the wrong-type path is unreachable
// in production; tested here for ADR-0018 never-panic posture).
func TestCompilePerRouteForRequest_NilTolerance(t *testing.T) {
	if got := compilePerRouteForRequest(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := compilePerRouteForRequest(&corev3.TypedExtensionConfig{}); got != nil {
		t.Errorf("wrong-type input: got %v, want nil", got)
	}
}

// -----------------------------------------------------------------------------
// TestPerRoute_VhRateLimitsEnum_OutOfRange (defensive)
// -----------------------------------------------------------------------------

// TestPerRoute_VhRateLimitsEnum_OutOfRange exercises an out-of-range
// `vh_rate_limits` enum value (a hypothetical varint past the 3 in-proto names).
// The validator PARSE-REJECTS with a byte-stable wording — defensive against any
// future enum-extension that escapes our switch (mirrors ADR-0080 + the §5.2
// validator-arm wording style).
func TestPerRoute_VhRateLimitsEnum_OutOfRange(t *testing.T) {
	pr := &ratelimitfilterv3.RateLimitPerRoute{
		VhRateLimits: ratelimitfilterv3.RateLimitPerRoute_VhRateLimitsOptions(99),
	}
	err := validatePerRouteRateLimit(pr)
	if err == nil {
		t.Fatal("validatePerRouteRateLimit should reject out-of-range enum; got nil")
	}
	if !errors.Is(err, errPerRouteVhRateLimitsOutOfRange) {
		t.Errorf("err = %v, want errors.Is(err, errPerRouteVhRateLimitsOutOfRange)", err)
	}
}
