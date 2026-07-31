package stats

import "testing"

// This file exists SEPARATELY from registry_test.go (which tests Registry
// behavior) and from name_test.go (which tests the tag-extraction table in
// name.go) because what it pins is neither: it is the phase-81 row's central
// doctrinal invariant about IsValidName itself, cited BY PATH from nine guard
// sites across nine packages. Giving it its own self-describing file name keeps
// that citation stable and makes the pin findable by a later sweep.
//
// THE INVARIANT. NamePattern is WHOLE-STRING anchored:
//
//	^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$
//
// so its first-character and last-character classes are strictly narrower than
// its interior class. A token's verdict therefore depends on WHERE the token
// sits in the assembled metric name, not on the token alone:
//
//   - At a LEADING position the token supplies the assembled name's first
//     character, so the narrow first-character class still applies to it, and
//     bare ≡ assembled except at the token's own tail.
//   - At an INTERIOR position a fixed prefix already satisfied the narrow
//     first-character class and a fixed suffix already satisfied the narrow
//     last-character class, so the token is judged ONLY by the wide interior
//     class — strictly more permissive than the bare check.
//
// WHY THIS IS PINNED AS EXECUTABLE FACT: it is the reason every phase-81 guard
// probes the ASSEMBLED name rather than the bare token. A sweep that
// "simplifies" the nine guards to bare probes would, at the seven interior
// sources, boot-REJECT configs that today boot, serve, and register perfectly
// valid counters. That regression is invisible to any test that only feeds
// valid prefixes. This table makes it red.
//
// The counts below are MEASURED, not asserted from doctrine. If a future change
// to NamePattern moves them, this test must be re-measured and the guard shapes
// re-derived — do NOT edit the expectations to match.
func TestIsValidName_SegmentPositionDeterminesVerdict(t *testing.T) {
	// The row's nine-token roster. "allow-admins" is the live RBAC policy name
	// from fixture 0018; the rest span the boundary shapes the pattern
	// discriminates on (leading digit, leading dot, trailing dot, bare digit,
	// empty, interior double dot, dotted, plain).
	cases := []struct {
		token string
		// bare = IsValidName(token) with no surrounding segments.
		bare bool
		// leading = IsValidName(token + ".zookeeper.decoder_error"), the
		// zookeeperproxy shape: the token STARTS the assembled name.
		leading bool
		// interior = IsValidName("http.myhcm.rbac.rbac.policy." + token +
		// ".allowed"), the http/rbac per-policy shape: fixed segments on BOTH
		// sides of the token.
		interior bool
	}{
		{"allow-admins", false, false, false}, // hyphen is outside every class
		{"0policy", false, false, true},       // leading digit: interior-only accept
		{"policy.", false, true, true},        // trailing dot: BOTH assembled positions accept
		{".policy", false, false, true},       // leading dot: interior-only accept
		{"9", false, false, true},             // bare digit: interior-only accept
		{"", false, false, true},              // empty: interior-only accept
		{"a..b", true, true, true},            // interior empty segment: valid everywhere
		{"a.b.c", true, true, true},
		{"ok", true, true, true},
	}

	leadingDisagreements, interiorDisagreements := 0, 0
	for _, tc := range cases {
		leadingProbe := tc.token + ".zookeeper.decoder_error"
		interiorProbe := "http.myhcm.rbac.rbac.policy." + tc.token + ".allowed"

		if got := IsValidName(tc.token); got != tc.bare {
			t.Errorf("bare IsValidName(%q) = %v, want %v", tc.token, got, tc.bare)
		}
		gotLeading := IsValidName(leadingProbe)
		if gotLeading != tc.leading {
			t.Errorf("LEADING IsValidName(%q) = %v, want %v", leadingProbe, gotLeading, tc.leading)
		}
		gotInterior := IsValidName(interiorProbe)
		if gotInterior != tc.interior {
			t.Errorf("INTERIOR IsValidName(%q) = %v, want %v", interiorProbe, gotInterior, tc.interior)
		}
		if tc.bare != tc.leading {
			leadingDisagreements++
		}
		if tc.bare != tc.interior {
			interiorDisagreements++
		}
	}

	// The aggregate the guard-site comments cite. A bare probe is a NEAR
	// substitute at a leading position and a WRONG one at an interior position.
	if leadingDisagreements != 1 {
		t.Errorf("LEADING bare-vs-assembled disagreements = %d, want 1 (the trailing-dot token)",
			leadingDisagreements)
	}
	if interiorDisagreements != 5 {
		t.Errorf("INTERIOR bare-vs-assembled disagreements = %d, want 5 (0policy, policy., .policy, 9, \"\")",
			interiorDisagreements)
	}
	if len(cases) != 9 {
		t.Errorf("token roster = %d, want 9 (the denominator the 1-of-9 / 5-of-9 counts are stated over)",
			len(cases))
	}
}
