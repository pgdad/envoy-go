package mongoproxy

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	commonfaultv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func TestParseConfig_StatPrefixStored(t *testing.T) {
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "mongo_a"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.statPrefix != "mongo_a" {
		t.Errorf("statPrefix = %q, want mongo_a", cfg.statPrefix)
	}
}

func TestParseConfig_CommandsDefault(t *testing.T) {
	// Empty list → the default {delete, insert, update} (AMEND-B7).
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	for _, c := range []string{"delete", "insert", "update"} {
		if !cfg.commands[c] {
			t.Errorf("default commands missing %q", c)
		}
	}
	if cfg.commands["isMaster"] {
		t.Errorf("default commands must NOT contain isMaster")
	}
}

func TestParseConfig_CommandsExplicitReplacesDefault(t *testing.T) {
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Commands: []string{"isMaster"}})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.commands["isMaster"] {
		t.Errorf("explicit commands missing isMaster")
	}
	if cfg.commands["delete"] {
		t.Errorf("explicit list must REPLACE the default (delete leaked in)")
	}
}

func TestNormalizeCommand_Aliases(t *testing.T) {
	cases := map[string]string{
		"collstats":     "collStats",
		"dbstats":       "dbStats",
		"findandmodify": "findAndModify",
		"getlasterror":  "getLastError",
		"ismaster":      "isMaster",
		"find":          "",         // find clears → routed to the query path, never a cmd.* stat
		"isMaster":      "isMaster", // already canonical → unchanged
		"insert":        "insert",   // not an alias → unchanged
	}
	for in, want := range cases {
		if got := normalizeCommand(in); got != want {
			t.Errorf("normalizeCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseConfig_DelaySpecifierRequired(t *testing.T) {
	// delay: {} — the oneof absent → reject (AMEND-B9, PGV `required` mirror).
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: &commonfaultv3.FaultDelay{}})
	if err == nil || err.Error() != errDelaySpecifierRequired {
		t.Fatalf("err = %v, want %q", err, errDelaySpecifierRequired)
	}
}

func TestParseConfig_FixedDelayTooSmall(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(0)},
	}
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err == nil || err.Error() != errDelayFixedDelayTooSmall {
		t.Fatalf("err = %v, want %q", err, errDelayFixedDelayTooSmall)
	}
}

func TestParseConfig_FixedDelayValid(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(5 * time.Millisecond)},
		Percentage:         &typev3.FractionalPercent{Numerator: 50, Denominator: typev3.FractionalPercent_HUNDRED},
	}
	cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !cfg.delayConfigured || cfg.fixedDelay != 5*time.Millisecond {
		t.Errorf("delay not stored: configured=%v fixed=%v", cfg.delayConfigured, cfg.fixedDelay)
	}
}

func TestParseConfig_PercentageDenominatorInvalid(t *testing.T) {
	d := &commonfaultv3.FaultDelay{
		FaultDelaySecifier: &commonfaultv3.FaultDelay_FixedDelay{FixedDelay: durationpb.New(time.Millisecond)},
		Percentage:         &typev3.FractionalPercent{Numerator: 1, Denominator: 99}, // out-of-range enum
	}
	_, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: "p", Delay: d})
	if err == nil || err.Error() != errDelayDenominatorInvalid {
		t.Fatalf("err = %v, want %q", err, errDelayDenominatorInvalid)
	}
}

func TestParseRejectConstants_ByteStable(t *testing.T) {
	// ADR-0080 byte-stable wording guard (D-S29.1-1). DO NOT update these to
	// match a code change — a mismatch means the production wording regressed.
	want := map[string]string{
		"stat_prefix":         "mongo_proxy: stat_prefix is required",
		"stat_prefix_invalid": "mongo_proxy: stat_prefix contains characters invalid for a metric name",
		"specifier":           "mongo_proxy: delay: a delay type must be specified",
		"fixed_delay":         "mongo_proxy: delay: fixed_delay must be greater than 0s",
		"denominator":         "mongo_proxy: delay: percentage denominator is not a defined value",
	}
	got := map[string]string{
		"stat_prefix": errStatPrefixRequired, "specifier": errDelaySpecifierRequired,
		"fixed_delay": errDelayFixedDelayTooSmall, "denominator": errDelayDenominatorInvalid,
		"stat_prefix_invalid": errStatPrefixInvalid,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s arm = %q, want %q", k, got[k], w)
		}
	}
}

// TestParseConfig_StatPrefixCharsetGuard covers the phase-81 stat_prefix
// character-class guard in parseConfig. Without it, newMongoStats assembles
// "mongo." + stat_prefix + "." + <leaf> and Registry.checkName PANICS —
// a config-triggered process crash (ADR-0065 §Consequences (e)).
//
// The guard probes the ASSEMBLED name at an INTERIOR segment position, so the
// accepted set is WIDER than a bare-token check would give. Two rows below are
// deliberate ACCEPT-pins rather than reject arms; both are load-bearing.
func TestParseConfig_StatPrefixCharsetGuard(t *testing.T) {
	cases := []struct {
		name    string
		prefix  string
		wantErr string // "" ⇒ accepted
	}{
		{"valid underscore prefix", "mongo_a", ""},
		{"hyphen rejected", "my-mongo", errStatPrefixInvalid},
		{"space rejected (the ADR-0065 fuzz seed shape)", "0000000000 0", errStatPrefixInvalid},
		// The empty prefix is caught by the EARLIER required-check; it must NOT
		// surface as the charset error even though "mongo..<leaf>" is valid.
		{"empty yields REQUIRED, not INVALID", "", errStatPrefixRequired},
		// ACCEPT-PIN (SPEC §13.1): the interior empty-segment hole is INHERITED
		// DELIBERATELY. stats.IsValidName("a..b") is true, so "foo." assembles
		// to "mongo.foo..cx_destroy_remote_with_active_rq" and is ACCEPTED.
		// Deferred whole to successor row stats-name-empty-segment-guards.
		// DO NOT "fix" this arm into a reject.
		{"ACCEPT-PIN trailing dot (SPEC §13.1 inherited hole)", "foo.", ""},
		// ACCEPT-PIN: the interior probe accepts a leading-digit token that the
		// bare-guarded thriftproxy / kafkabroker / redisproxy siblings REJECT.
		// Recorded at the guard site; the divergence is in the error surface,
		// not in what can crash.
		{"ACCEPT-PIN leading digit (interior-position divergence)", "1abc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(&mongo_proxyv3.MongoProxy{StatPrefix: tc.prefix})
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("parseConfig(%q) err = %v, want nil", tc.prefix, err)
				}
				if cfg == nil {
					t.Errorf("parseConfig(%q) cfg = nil, want non-nil", tc.prefix)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseConfig(%q) err = nil, want %q", tc.prefix, tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Errorf("parseConfig(%q) err = %q, want %q", tc.prefix, err.Error(), tc.wantErr)
			}
		})
	}
}
