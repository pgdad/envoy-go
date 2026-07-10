package matchpredicate

import (
	"errors"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

func reqHdr(name, exact string) *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestHeadersMatch{
		HttpRequestHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: name, HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: exact},
		}}},
	}}
}

func anyMatch() *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_AnyMatch{AnyMatch: true}}
}

// nest builds depth levels of not_match wrapping an any_match leaf.
// depth==1 returns the bare leaf.
func nest(depth int) *cmatcherv3.MatchPredicate {
	p := anyMatch()
	for i := 1; i < depth; i++ {
		p = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: p}}
	}
	return p
}

func TestCompile_AcceptsSixArms(t *testing.T) {
	accepts := map[string]*cmatcherv3.MatchPredicate{
		"any_match":                  anyMatch(),
		"not_match":                  {Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: anyMatch()}},
		"or_match":                   {Rule: &cmatcherv3.MatchPredicate_OrMatch{OrMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		"and_match":                  {Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		"http_request_headers_match": reqHdr("x-tap", "yes"),
		"http_response_headers_match": {Rule: &cmatcherv3.MatchPredicate_HttpResponseHeadersMatch{
			HttpResponseHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
				Name: ":status", HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "204"}}}}}},
	}
	for name, mp := range accepts {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(mp); err != nil {
				t.Errorf("Compile(%s) = %v, want nil", name, err)
			}
		})
	}
}

func TestCompile_RejectsFourArms(t *testing.T) {
	hh := &cmatcherv3.HttpHeadersMatch{}
	gb := &cmatcherv3.HttpGenericBodyMatch{}
	cases := map[string]struct {
		mp   *cmatcherv3.MatchPredicate
		want error
	}{
		"http_request_trailers_match":      {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestTrailersMatch{HttpRequestTrailersMatch: hh}}, ErrTrailersUnsupported},
		"http_response_trailers_match":     {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseTrailersMatch{HttpResponseTrailersMatch: hh}}, ErrTrailersUnsupported},
		"http_request_generic_body_match":  {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch{HttpRequestGenericBodyMatch: gb}}, ErrGenericBodyUnsupported},
		"http_response_generic_body_match": {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseGenericBodyMatch{HttpResponseGenericBodyMatch: gb}}, ErrGenericBodyUnsupported},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Compile(tc.mp)
			if !errors.Is(err, tc.want) {
				t.Errorf("Compile(%s) err = %v, want %v", name, err, tc.want)
			}
		})
	}
}

func TestCompile_StructuralRejects(t *testing.T) {
	for name, mp := range map[string]*cmatcherv3.MatchPredicate{
		"nil":         nil,
		"no_rule":     {},
		"empty_rules": {Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{}}},
		"nil_not":     {Rule: &cmatcherv3.MatchPredicate_NotMatch{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(mp); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestCompile_DepthCap(t *testing.T) {
	if _, err := Compile(nest(MaxDepth)); err != nil {
		t.Errorf("depth %d must compile, got %v", MaxDepth, err)
	}
	_, err := Compile(nest(MaxDepth + 1))
	if !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("depth %d: err = %v, want ErrDepthExceeded", MaxDepth+1, err)
	}
}
