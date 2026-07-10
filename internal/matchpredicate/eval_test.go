package matchpredicate

import (
	"net/http"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

func respHdr(name, exact string) *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseHeadersMatch{
		HttpResponseHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: name, HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: exact}}}},
	}}
}

// The 0099 predicate.
func andReqResp(t *testing.T) *Program {
	t.Helper()
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_AndMatch{
		AndMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{
			reqHdr("x-tap", "yes"), respHdr(":status", "204"),
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return p
}

func TestEvaluator_TriState_RequestOnlyIsUndetermined(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	if got := e.Value(); got != Undetermined {
		t.Errorf("before any feed: Value() = %v, want Undetermined", got)
	}
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	if got := e.Value(); got != Undetermined {
		t.Errorf("after request feed only: Value() = %v, want Undetermined", got)
	}
	// A never-arriving response resolves the whole tree to false.
	if e.Resolve() {
		t.Errorf("Resolve() with no response = true, want false")
	}
}

func TestEvaluator_AndMatch_BothArms(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	e.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !e.Resolve() {
		t.Errorf("Resolve() = false, want true")
	}
}

func TestEvaluator_AndMatch_RequestArmFalse_ShortCircuitsToFalseEarly(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"no"}})
	// One False decides a conjunction even while the response arm is Undetermined.
	if got := e.Value(); got != False {
		t.Errorf("Value() = %v, want False", got)
	}
	if e.Resolve() {
		t.Errorf("Resolve() = true, want false")
	}
}

// The `orshort` probe, in unit form: a TRUE request arm resolves an or_match to
// True even though the response arm never becomes true. This pins that the
// tri-state governs RESOLUTION only; it must NOT be read as a license to emit
// early (emission is unconditionally at stream end -- see the tap filter).
func TestEvaluator_OrMatch_RequestArmTrue(t *testing.T) {
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_OrMatch{
		OrMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{
			reqHdr("x-tap", "yes"), respHdr(":status", "999"),
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	e := p.NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	if got := e.Value(); got != True {
		t.Errorf("Value() = %v, want True", got)
	}
	e.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !e.Resolve() {
		t.Errorf("Resolve() = false, want true")
	}
}

func TestEvaluator_NotMatch_UndeterminedPassesThrough(t *testing.T) {
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_NotMatch{
		NotMatch: respHdr(":status", "204")}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	e := p.NewEvaluator()
	if got := e.Value(); got != Undetermined {
		t.Errorf("Value() = %v, want Undetermined", got)
	}
	e.FeedResponseHeaders(http.Header{":status": {"500"}})
	if !e.Resolve() {
		t.Errorf("not(:status==204) on a 500 = false, want true")
	}
}

// A Program must be reusable across streams with no cross-talk.
func TestProgram_IsImmutableAcrossEvaluators(t *testing.T) {
	p := andReqResp(t)
	a := p.NewEvaluator()
	a.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	a.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !a.Resolve() {
		t.Fatalf("evaluator a should match")
	}
	b := p.NewEvaluator()
	b.FeedRequestHeaders(http.Header{"x-tap": {"no"}})
	b.FeedResponseHeaders(http.Header{":status": {"204"}})
	if b.Resolve() {
		t.Errorf("evaluator b must NOT match; Program leaked state")
	}
}
