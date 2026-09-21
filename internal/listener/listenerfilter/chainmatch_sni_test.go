package listenerfilter

import (
	"errors"
	"net"
	"testing"
)

// sniInputs builds the inputs a TLS connection carries after tls_inspector:
// SNI, transport_protocol "tls" and an ALPN offer. Loopback addressing mirrors
// the TCP serve path; none of the chains below set an address dimension.
func sniInputs(sni string) ChainMatchInputs {
	return ChainMatchInputs{
		DestinationIP:        net.ParseIP("127.0.0.1"),
		DestinationPort:      10000,
		SourceIP:             net.ParseIP("127.0.0.1"),
		SourcePort:           40000,
		ServerName:           sni,
		TransportProtocol:    "tls",
		ApplicationProtocols: []string{"h2", "http/1.1"},
	}
}

func sniChain(name string, patterns ...string) *ChainSpec {
	return &ChainSpec{Name: name, ServerNames: patterns}
}

// TestSelectChainSNILongestMatchedSuffix pins the server_names sub-ordering:
// among chains that tie on the specificity bitmask, the chain whose MATCHED
// server_names pattern is most specific wins — an exact name beats any
// wildcard, and between wildcards the LONGEST matching suffix wins, in either
// declaration order. Each row is one property, scored on its own.
//
// Rows with wantErr set pin the one slot-2 outcome that stays ambiguous: two
// chains whose best MATCHED patterns are the same string tie on rank AND
// suffix length, so SelectChain returns (nil, ErrAmbiguousChainMatch) and the
// connection closes (the reference refuses this config class at validate).
func TestSelectChainSNILongestMatchedSuffix(t *testing.T) {
	long := sniChain("LONG", "*.b.foo.test")
	short := sniChain("SHORT", "*.foo.test")
	l3 := sniChain("L3", "*.c.b.foo.test")
	l2 := sniChain("L2", "*.b.foo.test")
	l1 := sniChain("L1", "*.foo.test")
	x := sniChain("X", "x.test", "*.foo.test")
	y := sniChain("Y", "*.b.foo.test")
	x2 := sniChain("X2", "*.foo.test", "*.c.b.foo.test")
	e1 := sniChain("E1", "a.b.foo.test")
	w := sniChain("W", "*.b.foo.test")
	def := &ChainSpec{Name: "DEFAULT"}
	oa := sniChain("OA", "*.foo.test", "q.test")
	ob := sniChain("OB", "*.foo.test")

	cases := []struct {
		name    string
		chains  []*ChainSpec
		def     *ChainSpec
		sni     string
		want    string
		wantErr error
	}{
		{"C0_long_declared_first_longer_suffix_wins", []*ChainSpec{long, short}, nil, "a.b.foo.test", "LONG", nil},
		{"C0_short_declared_first_longer_suffix_wins", []*ChainSpec{short, long}, nil, "a.b.foo.test", "LONG", nil},
		{"C0_matched_negative_only_short_matches", []*ChainSpec{long, short}, nil, "x.foo.test", "SHORT", nil},
		{"B_desc_three_levels_deepest_suffix_wins", []*ChainSpec{l3, l2, l1}, nil, "z.c.b.foo.test", "L3", nil},
		{"B_desc_middle_suffix_beats_shortest", []*ChainSpec{l3, l2, l1}, nil, "y.b.foo.test", "L2", nil},
		{"B_asc_three_levels_deepest_suffix_wins", []*ChainSpec{l1, l2, l3}, nil, "z.c.b.foo.test", "L3", nil},
		{"B_asc_middle_suffix_beats_shortest", []*ChainSpec{l1, l2, l3}, nil, "y.b.foo.test", "L2", nil},
		{"M1_rank_of_matched_pattern_not_whole_set_X_first", []*ChainSpec{x, y}, nil, "a.b.foo.test", "Y", nil},
		{"M2_rank_of_matched_pattern_not_whole_set_Y_first", []*ChainSpec{y, x}, nil, "a.b.foo.test", "Y", nil},
		{"M1_exact_member_of_mixed_set_still_wins_its_name", []*ChainSpec{x, y}, nil, "x.test", "X", nil},
		{"M3_longest_matching_member_not_longest_member_X2_first", []*ChainSpec{x2, y}, nil, "a.b.foo.test", "Y", nil},
		{"M4_longest_matching_member_not_longest_member_Y_first", []*ChainSpec{y, x2}, nil, "a.b.foo.test", "Y", nil},
		{"M3_deeper_member_of_mixed_set_wins", []*ChainSpec{x2, y}, nil, "z.c.b.foo.test", "X2", nil},
		{"M4_deeper_member_of_mixed_set_wins", []*ChainSpec{y, x2}, nil, "z.c.b.foo.test", "X2", nil},
		{"E_default_present_two_wildcards_resolve_not_default", []*ChainSpec{long, short}, def, "a.b.foo.test", "LONG", nil},
		{"E_default_serves_only_no_match", []*ChainSpec{long, short}, def, "nomatch.example", "DEFAULT", nil},
		{"R_exact_beats_wildcard_exact_first", []*ChainSpec{e1, w}, nil, "a.b.foo.test", "E1", nil},
		{"R_exact_beats_wildcard_wildcard_first", []*ChainSpec{w, e1}, nil, "a.b.foo.test", "E1", nil},
		{"O1_shared_matched_wildcard_is_ambiguous", []*ChainSpec{oa, ob}, nil, "a.b.foo.test", "", ErrAmbiguousChainMatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SelectChain(sniInputs(tc.sni), tc.chains, tc.def)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("%s: SNI %q: SelectChain error %v, want %v", tc.name, tc.sni, err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("%s: SNI %q: got chain %v, want nil (ambiguous)", tc.name, tc.sni, got)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: SNI %q: SelectChain error %v, want chain %s", tc.name, tc.sni, err, tc.want)
				return
			}
			if got == nil || got.Name != tc.want {
				t.Errorf("%s: SNI %q: got chain %v, want %s", tc.name, tc.sni, got, tc.want)
			}
		})
	}
}
