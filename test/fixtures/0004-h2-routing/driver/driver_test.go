// Unit tests for the 0004-h2-routing driver. The differential gate
// (TestDifferential/0004-h2-routing under test/differential/) covers the e2e
// behavior; these tests cover the driver-internal AssertDistribution + body
// parsing + bootstrap rendering so the e2e gate's diagnostics are easier to
// read when something drifts.
package driver

import (
	"regexp"
	"strings"
	"testing"
)

// residualPortPlaceholder matches a `port_value: 0` that renderBootstrap did
// NOT substitute. Phase 89: the previous form was `strings.Contains(out,
// "port_value: 0\n")`, which is STRUCTURALLY VACUOUS — every `port_value: 0`
// in both YAMLs sits inside a flow mapping and is followed by ` }`, never by a
// newline, so `grep -c 'port_value: 0$'` reads 0 while the unanchored form
// reads 3 (envoy.yaml) and 5 (envoy-go.yaml). The old check could not fire on
// any input, including a deliberately injected extra placeholder. The trailing
// non-digit class is what keeps this from matching a substituted `port_value:
// 30000`; no port the driver assigns begins with `0`.
var residualPortPlaceholder = regexp.MustCompile(`port_value: 0(?:[^0-9]|$)`)

// p92CLObs builds a VALID per-arm content-length arity observation slice for
// the unit table: one entry per p92 arm, every entry equal to v. It is derived
// from p92Arms() rather than written as a literal, so adding a p92 arm cannot
// silently make these rows vacuous again — which is exactly what happened once
// already (see the note on wantErrSubstr below).
func p92CLObs(v int) []int {
	obs := make([]int, len(p92Arms()))
	for i := range obs {
		obs[i] = v
	}
	return obs
}

// TestH2Driver_AssertDistribution covers the AssertDistribution branches:
// happy [3,3,3] (both sides), subject skew, reference skew, length mismatch,
// and the phase-92 per-side content-length arity pins.
//
// Per the driver's design note: AssertDistribution consults the BODY-derived
// counts the driver populated during Drive (because subprocess HTTPSH2
// backends don't increment the runner's accept counter). The runner-supplied
// refCounts/subjCounts arguments are length-checked but their values are
// ignored — the test seeds d.refBodyCnt / d.subjBodyCnt directly to exercise
// the assertion branches.
//
// ⚠️ EVERY ROW ASSERTS *WHICH* PROPERTY FAILED, VIA wantErrSubstr, AND THAT IS
// NOT DECORATION. When phase 92 added the two arity pins to AssertDistribution
// it did NOT seed the new d.refP92CL / d.subjP92CL fields here, so the bare
// &h2Driver{} carried zero observations against five arms. The non-vacuity
// guard inside p92AssertCLFields then fired on EVERY row: the happy row went
// RED, and — worse — the five wantErr:true rows kept PASSING for the WRONG
// REASON, reporting the arity guard instead of the distribution branch each
// one exists to exercise. A bare `err != nil` check could not tell the two
// apart. Asserting the message is what makes each row a guard rather than a
// test that merely passes.
func TestH2Driver_AssertDistribution(t *testing.T) {
	cases := []struct {
		name          string
		refBody       [3]uint64
		subjBody      [3]uint64
		refCounts     []uint64
		subjCounts    []uint64
		refCL         []int
		subjCL        []int
		wantErr       bool
		wantErrSubstr string
	}{
		// --- the distribution branches. All carry VALID arity observations, so
		// a failure here can only come from the distribution rule itself.
		{"both [3,3,3]", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), false, ""},
		{"subj [4,3,2]", [3]uint64{3, 3, 3}, [3]uint64{4, 3, 2}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), true, "subj distribution"},
		{"ref [4,3,2]", [3]uint64{4, 3, 2}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), true, "ref distribution"},
		{"subj count length mismatch", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{3, 3}, p92CLObs(1), p92CLObs(0), true, "subj backend count"},
		{"ref count length mismatch", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{3, 3, 3, 3}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), true, "ref backend count"},
		{"both [9,0,0] (full skew)", [3]uint64{9, 0, 0}, [3]uint64{9, 0, 0}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(0), true, "distribution"},

		// --- the phase-92 arity pins. The distribution is [3,3,3] on both sides
		// in every row below, so a failure can only come from an arity pin.
		{"p92 ref arity: no observations", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, nil, p92CLObs(0), true, "p92 ref content-length arity"},
		{"p92 subj arity: no observations", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), nil, true, "p92 subj content-length arity"},
		{"p92 ref value: 0 where 1 expected", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(0), p92CLObs(0), true, "p92 ref content-length fields"},
		{"p92 subj value: 1 where 0 expected", [3]uint64{3, 3, 3}, [3]uint64{3, 3, 3}, []uint64{0, 0, 0}, []uint64{0, 0, 0}, p92CLObs(1), p92CLObs(1), true, "p92 subj content-length fields"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			d := &h2Driver{}
			d.refBodyCnt = tc.refBody
			d.subjBodyCnt = tc.subjBody
			d.refP92CL = tc.refCL
			d.subjP92CL = tc.subjCL
			err := d.AssertDistribution(tc.refCounts, tc.subjCounts)
			if (err != nil) != tc.wantErr {
				t.Errorf("AssertDistribution: err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErrSubstr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr)) {
				t.Errorf("AssertDistribution: err=%v, want it to mention %q", err, tc.wantErrSubstr)
			}
		})
	}
}

// TestRenderBootstrap_Subject and TestRenderBootstrap_Reference verify the
// driver-side YAML rendering: every {{...}} placeholder is replaced; every
// `port_value: 0` placeholder is replaced; the substituted ports appear in
// the expected order. A regression here breaks the differential gate
// silently (the subject would fail to parse the bootstrap), so guarding the
// rendering with a unit test makes diagnostics fast.
func TestRenderBootstrap_Subject(t *testing.T) {
	d := &h2Driver{}
	out := d.SubjectConfig(15004, 12345, []int{30000, 30001, 30002}, 19999)
	if strings.Contains(out, "{{") {
		t.Fatalf("subject contains leftover placeholder:\n%s", out)
	}
	if residualPortPlaceholder.MatchString(out) {
		t.Errorf("subject still has an unsubstituted port_value: 0:\n%s", out)
	}
	for _, want := range []string{
		"port_value: 19999",
		"port_value: 12345",
		"port_value: 30000",
		"port_value: 30001",
		"port_value: 30002",
		"-----BEGIN CERTIFICATE-----",
		"-----BEGIN PRIVATE KEY-----",
		"alpn_protocols: [\"h2\", \"http/1.1\"]",
		"alpn_protocols: [\"h2\"]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("subject missing %q", want)
		}
	}
}

func TestRenderBootstrap_Reference(t *testing.T) {
	d := &h2Driver{}
	out := d.ReferenceBootstrap([]int{30000, 30001, 30002})
	if strings.Contains(out, "{{") {
		t.Fatalf("reference contains leftover placeholder:\n%s", out)
	}
	if residualPortPlaceholder.MatchString(out) {
		t.Errorf("reference still has an unsubstituted port_value: 0:\n%s", out)
	}
	for _, want := range []string{
		"port_value: 9901",  // fixed admin
		"port_value: 15004", // fixed listener
		"port_value: 30000",
		"port_value: 30001",
		"port_value: 30002",
		"host.docker.internal",
		"dns_lookup_family: V4_ONLY",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("reference missing %q", want)
		}
	}
}

// TestParseBackendIdx covers the response-body parsing helper that drives
// the per-side distribution counters.
func TestParseBackendIdx(t *testing.T) {
	cases := []struct {
		name    string
		body    []byte
		wantIdx int
		wantErr bool
	}{
		{"backend-0", []byte("backend-0:v1/0"), 0, false},
		{"backend-1", []byte("backend-1:v1/3"), 1, false},
		{"backend-2", []byte("backend-2:v1/8"), 2, false},
		{"missing prefix", []byte("0:v1/0"), 0, true},
		{"missing colon", []byte("backend-0v1/0"), 0, true},
		{"non-numeric idx", []byte("backend-x:v1/0"), 0, true},
		{"empty", []byte(""), 0, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			idx, err := parseBackendIdx(tc.body)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseBackendIdx(%q): err=%v, wantErr=%v", string(tc.body), err, tc.wantErr)
				return
			}
			if !tc.wantErr && idx != tc.wantIdx {
				t.Errorf("parseBackendIdx(%q): got idx=%d, want %d", string(tc.body), idx, tc.wantIdx)
			}
		})
	}
}
