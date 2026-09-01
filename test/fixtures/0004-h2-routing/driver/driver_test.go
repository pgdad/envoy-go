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

	"golang.org/x/net/http2/hpack"
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

// p93Obs builds a per-arm content-length observation slice for the unit table:
// one entry per p92 arm, IN ROSTER ORDER AND NAMED FROM THE LIVE ROSTER, every
// entry carrying the same arity / declared / declaredOK / bodyLen.
//
// ⚠️ DERIVED FROM p92Arms(), NEVER A LITERAL. Adding, removing or renaming a
// p92 arm must not silently make these rows vacuous again — which is exactly
// what happened once already (see the note on wantErrSubstrs below). The arm
// NAMES matter too now: p93AssertRoster checks per-index identity, so a
// literal roster would also have to be kept in sync by hand.
func p93Obs(arity, declared int, declaredOK bool, bodyLen int) []p93CLObs {
	arms := p92Arms()
	obs := make([]p93CLObs, len(arms))
	for i, a := range arms {
		obs[i] = p93CLObs{
			arm:        a.name,
			arity:      arity,
			declared:   declared,
			declaredOK: declaredOK,
			bodyLen:    bodyLen,
		}
	}
	return obs
}

// p93RefOK / p93SubjOK are the HEALTHY per-side observations — each side's own
// MEASURED pins, arity 1, declaring exactly what it delivers. Every row below
// starts from these so that a row which perturbs nothing is green and each
// failing row is SINGLE-CAUSE.
func p93RefOK() []p93CLObs  { return p93Obs(1, p93WantRefBodyLen, true, p93WantRefBodyLen) }
func p93SubjOK() []p93CLObs { return p93Obs(1, p93WantSubjBodyLen, true, p93WantSubjBodyLen) }

// p93Perturb copies obs and replaces arm index i, so exactly ONE arm on ONE
// side deviates and the row keeps naming exactly one violated property.
func p93Perturb(obs []p93CLObs, i int, f func(p93CLObs) p93CLObs) []p93CLObs {
	out := append([]p93CLObs(nil), obs...)
	out[i] = f(out[i])
	return out
}

// TestH2Driver_AssertDistribution covers the AssertDistribution branches:
// happy [3,3,3] (both sides), subject skew, reference skew, length mismatch,
// the per-side content-length arity pins, the per-side declared==delivered and
// body-length pins, and the roster barrier that keeps the per-arm pins from
// passing vacuously.
//
// Per the driver's design note: AssertDistribution consults the BODY-derived
// counts the driver populated during Drive (because subprocess HTTPSH2
// backends don't increment the runner's accept counter). The runner-supplied
// refCounts/subjCounts arguments are length-checked but their values are
// ignored — the test seeds d.refBodyCnt / d.subjBodyCnt directly to exercise
// the assertion branches.
//
// ⚠️ EVERY ROW ASSERTS *WHICH* PROPERTY FAILED, VIA wantErrSubstrs, AND THAT
// IS NOT DECORATION. When phase 92 added the two arity pins to
// AssertDistribution it did NOT seed the new d.refP92CL / d.subjP92CL fields
// here, so the bare &h2Driver{} carried zero observations against five arms.
// The non-vacuity guard inside p92AssertCLFields then fired on EVERY row: the
// happy row went RED, and — worse — the five wantErr:true rows kept PASSING
// for the WRONG REASON, reporting the arity guard instead of the distribution
// branch each one exists to exercise. A bare `err != nil` check could not tell
// the two apart. Asserting the message is what makes each row a guard rather
// than a test that merely passes.
//
// ⚠️ wantErrAbsent IS THE OTHER HALF OF THAT DOCTRINE. Asserting that a pin
// DID fire says nothing about which pins did NOT. The no-observations rows use
// it to pin the roster barrier's whole reason for existing: with an empty
// slice the roster must be what reports the problem, and the per-arm pins it
// gates must NOT be — they would otherwise have passed vacuously, which is the
// exact failure mode measured before the barrier existed.
//
// ⚠️ SINGLE-CAUSE, EXCEPT WHERE THE INPUT MAKES IT IMPOSSIBLE. Every row below
// perturbs one property on one side. The two no-observations rows are the one
// place two guards necessarily fire together — an empty slice is both "the
// arity pin has nothing to check" and "the roster is empty" — so those rows
// assert BOTH messages explicitly rather than pretending one of them is the
// only cause. absent / duplicated `content-length` are likewise inseparable
// from arity here (they ARE arity 0 and arity 2), so the three-state observer
// is pinned directly in TestP93ContentLength instead, where each state is a
// single cause by construction.
func TestH2Driver_AssertDistribution(t *testing.T) {
	// ok3 / zero3 are the healthy distribution shape and the runner-supplied
	// counter shape, hoisted so each row reads as its own perturbation.
	ok3 := [3]uint64{3, 3, 3}
	zero3 := []uint64{0, 0, 0}
	cases := []struct {
		name           string
		refBody        [3]uint64
		subjBody       [3]uint64
		refCounts      []uint64
		subjCounts     []uint64
		refCL          []p93CLObs
		subjCL         []p93CLObs
		wantErr        bool
		wantErrSubstrs []string
		wantErrAbsent  []string
	}{
		// --- the distribution branches. All carry HEALTHY observations, so a
		// failure here can only come from the distribution rule itself.
		{"both [3,3,3]", ok3, ok3, zero3, zero3, p93RefOK(), p93SubjOK(), false, nil, nil},
		{"subj [4,3,2]", ok3, [3]uint64{4, 3, 2}, zero3, zero3, p93RefOK(), p93SubjOK(), true, []string{"subj distribution"}, nil},
		{"ref [4,3,2]", [3]uint64{4, 3, 2}, ok3, zero3, zero3, p93RefOK(), p93SubjOK(), true, []string{"ref distribution"}, nil},
		{"subj count length mismatch", ok3, ok3, zero3, []uint64{3, 3}, p93RefOK(), p93SubjOK(), true, []string{"subj backend count"}, nil},
		{"ref count length mismatch", ok3, ok3, []uint64{3, 3, 3, 3}, zero3, p93RefOK(), p93SubjOK(), true, []string{"ref backend count"}, nil},
		{"both [9,0,0] (full skew)", [3]uint64{9, 0, 0}, [3]uint64{9, 0, 0}, zero3, zero3, p93RefOK(), p93SubjOK(), true, []string{"distribution"}, nil},

		// --- the arity pins. The distribution is [3,3,3] on both sides in every
		// row below, so a failure can only come from a content-length pin.
		//
		// The two no-observations rows are ALSO the roster barrier's negative
		// control: they assert that an empty slice is reported by the arity
		// guard AND the roster barrier, and is NOT reported by the two per-arm
		// pins the barrier gates — which would have passed vacuously.
		{"no observations: ref", ok3, ok3, zero3, zero3, nil, p93SubjOK(), true,
			[]string{"p92 ref content-length arity", "p93 ref observation roster: got 0 observations, want 5"},
			[]string{"p93 ref declared != delivered", "p93 ref body length"}},
		{"no observations: subj", ok3, ok3, zero3, zero3, p93RefOK(), nil, true,
			[]string{"p92 subj content-length arity", "p93 subj observation roster: got 0 observations, want 5"},
			[]string{"p93 subj declared != delivered", "p93 subj body length"}},
		{"p92 ref arity: 0 where 1 expected", ok3, ok3, zero3, zero3, p93Obs(0, 0, false, p93WantRefBodyLen), p93SubjOK(), true, []string{"p92 ref content-length fields"}, nil},
		{"p92 subj arity: 0 where 1 expected", ok3, ok3, zero3, zero3, p93RefOK(), p93Obs(0, 0, false, p93WantSubjBodyLen), true, []string{"p92 subj content-length fields"}, nil},

		// --- the declared==delivered pin, RFC 9110 §8.6. Arity is 1 on every
		// arm in these rows, so the arity pins stay green and only this pin can
		// fire. One arm deviates per row, so the "1 of 5 arms" tail also pins
		// that the pin is not over-firing.
		{"p93 ref declared > delivered (one arm)", ok3, ok3, zero3, zero3,
			p93Perturb(p93RefOK(), 0, func(o p93CLObs) p93CLObs { o.declared = p93WantRefBodyLen + 1; return o }),
			p93SubjOK(), true,
			[]string{"p93 ref declared != delivered: keepalive=declared 88/delivered 87 (1 of 5 arms)"}, nil},
		{"p93 subj declared < delivered (one arm)", ok3, ok3, zero3, zero3, p93RefOK(),
			p93Perturb(p93SubjOK(), 2, func(o p93CLObs) p93CLObs { o.declared = p93WantSubjBodyLen - 1; return o }),
			true,
			[]string{"p93 subj declared != delivered: proxyconn=declared 11/delivered 12 (1 of 5 arms)"}, nil},
		{"p93 subj declared unparsable (arity 1, no usable value)", ok3, ok3, zero3, zero3, p93RefOK(),
			p93Perturb(p93SubjOK(), 3, func(o p93CLObs) p93CLObs { o.declaredOK = false; o.declared = 0; return o }),
			true,
			[]string{"te-gzip=declared unparsable/delivered 12"}, nil},

		// --- the per-side body-length pins. Each row moves ONE arm's bodyLen
		// and moves its declared value with it, so declared==delivered still
		// holds and only the length pin can fire.
		{"p93 ref body length: one arm empty", ok3, ok3, zero3, zero3,
			p93Perturb(p93RefOK(), 1, func(o p93CLObs) p93CLObs { o.declared = 0; o.bodyLen = 0; return o }),
			p93SubjOK(), true,
			[]string{"p93 ref body length: want 87 on every arm, got upgrade=0 (1 of 5 arms)"}, nil},
		{"p93 subj body length: one arm at the ref length", ok3, ok3, zero3, zero3, p93RefOK(),
			p93Perturb(p93SubjOK(), 4, func(o p93CLObs) p93CLObs { o.declared = 87; o.bodyLen = 87; return o }),
			true,
			[]string{"p93 subj body length: want 12 on every arm, got te-empty=87 (1 of 5 arms)"}, nil},
		{"p93 subj body length: ALL arms named (not fail-fast)", ok3, ok3, zero3, zero3, p93RefOK(),
			p93Obs(1, 0, true, 0), true,
			[]string{"p93 subj body length: want 12 on every arm, got keepalive=0,upgrade=0,proxyconn=0,te-gzip=0,te-empty=0 (5 of 5 arms)"}, nil},

		// --- the roster barrier's identity half. A full-length roster carrying
		// the WRONG arm names passes every count check, so only per-index
		// identity can catch it. Arity/declared/delivered are all healthy here,
		// so the gated pins never run and the barrier is the sole cause.
		{"p93 roster: ref arm identity", ok3, ok3, zero3, zero3,
			p93Perturb(p93RefOK(), 3, func(o p93CLObs) p93CLObs { o.arm = "not-an-arm"; return o }),
			p93SubjOK(), true,
			[]string{`p93 ref observation roster: arm identity mismatch: [3]="not-an-arm" want "te-gzip" (1 of 5 arms)`},
			[]string{"p93 ref declared != delivered", "p93 ref body length"}},
		{"p93 roster: subj arm identity", ok3, ok3, zero3, zero3, p93RefOK(),
			p93Perturb(p93SubjOK(), 0, func(o p93CLObs) p93CLObs { o.arm = "not-an-arm"; return o }),
			true,
			[]string{`p93 subj observation roster: arm identity mismatch: [0]="not-an-arm" want "keepalive" (1 of 5 arms)`},
			[]string{"p93 subj declared != delivered", "p93 subj body length"}},
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
			for _, want := range tc.wantErrSubstrs {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("AssertDistribution: err=%v, want it to mention %q", err, want)
				}
			}
			for _, unwanted := range tc.wantErrAbsent {
				if err != nil && strings.Contains(err.Error(), unwanted) {
					t.Errorf("AssertDistribution: err=%v, must NOT mention %q (it would be reporting a vacuous pass)", err, unwanted)
				}
			}
		})
	}
}

// TestP93ContentLength pins the THREE-STATE declared observation at its source,
// where each state is a single cause by construction. It cannot be pinned in
// the AssertDistribution table: absent and duplicated ARE arity 0 and arity 2,
// so they necessarily move the arity pin too.
//
// ⚠️ A DUPLICATED `content-length` MUST NOT YIELD "THE FIRST VALUE". A
// duplicate is malformed per RFC 9113 §8.1.1 and its value is meaningless;
// returning the first would launder a real defect into a plausible number that
// the declared==delivered pin might then accept. The duplicate rows below make
// the first value a *correct-looking* one on purpose, so a "take the first"
// implementation would pass declared==delivered and only this test can fail.
//
// ⚠️ THE ZERO RETURNED WITH ok=false IS NOT AN OBSERVATION. The absent row and
// the `content-length: 0` row read the same int and are distinguished ONLY by
// the bool — which is the whole reason the observation is not a bare int.
func TestP93ContentLength(t *testing.T) {
	cl := func(vals ...string) []hpack.HeaderField {
		f := []hpack.HeaderField{{Name: ":status", Value: "502"}}
		for _, v := range vals {
			f = append(f, hpack.HeaderField{Name: "content-length", Value: v})
		}
		return f
	}
	cases := []struct {
		name         string
		fields       []hpack.HeaderField
		wantDeclared int
		wantOK       bool
		wantRendered string
	}{
		{"parsed", cl("87"), 87, true, "87"},
		{"parsed zero is a real observation", cl("0"), 0, true, "0"},
		{"absent", cl(), 0, false, "absent"},
		{"duplicated, identical values", cl("12", "12"), 0, false, "duplicated(x2)"},
		{"duplicated, first value plausible", cl("12", "99"), 0, false, "duplicated(x2)"},
		{"duplicated x3", cl("12", "12", "12"), 0, false, "duplicated(x3)"},
		{"unparsable", cl("twelve"), 0, false, "unparsable"},
		{"empty value", cl(""), 0, false, "unparsable"},
		{"negative", cl("-1"), 0, false, "unparsable"},
		{"uppercase wire name still counts", []hpack.HeaderField{{Name: "Content-Length", Value: "12"}}, 12, true, "12"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotDeclared, gotOK := p93ContentLength(tc.fields)
			if gotDeclared != tc.wantDeclared || gotOK != tc.wantOK {
				t.Errorf("p93ContentLength = (%d, %v), want (%d, %v)", gotDeclared, gotOK, tc.wantDeclared, tc.wantOK)
			}
			obs := p93Observe("keepalive", tc.fields, 12)
			if got := obs.declaredString(); got != tc.wantRendered {
				t.Errorf("declaredString = %q, want %q", got, tc.wantRendered)
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
