package matchpredicate

import (
	"errors"
	"fmt"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"

	"github.com/pgdad/envoy-go/internal/headermatch"
)

// MaxDepth caps MatchPredicate nesting (root = depth 1). The compiler is
// recursive over an attacker-influenceable proto; without a cap a deeply
// nested not_match chain would exhaust the stack. Exercised by
// FuzzMatchPredicateCompile.
const MaxDepth = 32

var (
	// ErrDepthExceeded is returned when nesting exceeds MaxDepth.
	ErrDepthExceeded = errors.New("matchpredicate: nesting depth exceeds MaxDepth")
	// ErrTrailersUnsupported rejects the two trailer arms (f6/f8): envoy-go's
	// HTTP filters cannot observe trailers (the never-done HCM "Task 18").
	ErrTrailersUnsupported = errors.New("matchpredicate: http_{request,response}_trailers_match is not supported")
	// ErrGenericBodyUnsupported rejects the two generic_body arms (f9/f10).
	ErrGenericBodyUnsupported = errors.New("matchpredicate: http_{request,response}_generic_body_match is not supported")
)

// Program is an immutable compiled predicate tree, safe to share across streams.
type Program struct{ root node }

// Compile builds a Program from a MatchPredicate proto, rejecting every
// unsupported arm explicitly.
func Compile(mp *cmatcherv3.MatchPredicate) (*Program, error) {
	root, err := compileNode(mp, 1)
	if err != nil {
		return nil, err
	}
	return &Program{root: root}, nil
}

func compileNode(mp *cmatcherv3.MatchPredicate, depth int) (node, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("%w (max %d)", ErrDepthExceeded, MaxDepth)
	}
	if mp == nil {
		return nil, errors.New("matchpredicate: nil MatchPredicate")
	}
	switch r := mp.GetRule().(type) {
	case *cmatcherv3.MatchPredicate_AnyMatch: // f4
		return anyNode{}, nil
	case *cmatcherv3.MatchPredicate_NotMatch: // f3
		child, err := compileNode(r.NotMatch, depth+1)
		if err != nil {
			return nil, err
		}
		return notNode{child: child}, nil
	case *cmatcherv3.MatchPredicate_OrMatch: // f1
		kids, err := compileSet(r.OrMatch, depth)
		if err != nil {
			return nil, err
		}
		return orNode{children: kids}, nil
	case *cmatcherv3.MatchPredicate_AndMatch: // f2
		kids, err := compileSet(r.AndMatch, depth)
		if err != nil {
			return nil, err
		}
		return andNode{children: kids}, nil
	case *cmatcherv3.MatchPredicate_HttpRequestHeadersMatch: // f5
		return compileHeaders(r.HttpRequestHeadersMatch, false)
	case *cmatcherv3.MatchPredicate_HttpResponseHeadersMatch: // f7
		return compileHeaders(r.HttpResponseHeadersMatch, true)

	// ---- EXPLICIT rejects (never a fall-through default) ----
	case *cmatcherv3.MatchPredicate_HttpRequestTrailersMatch: // f6
		return nil, ErrTrailersUnsupported
	case *cmatcherv3.MatchPredicate_HttpResponseTrailersMatch: // f8
		return nil, ErrTrailersUnsupported
	case *cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch: // f9
		return nil, ErrGenericBodyUnsupported
	case *cmatcherv3.MatchPredicate_HttpResponseGenericBodyMatch: // f10
		return nil, ErrGenericBodyUnsupported

	case nil:
		return nil, errors.New("matchpredicate: MatchPredicate has no rule set")
	default:
		return nil, fmt.Errorf("matchpredicate: unsupported rule arm %T", r)
	}
}

func compileSet(ms *cmatcherv3.MatchPredicate_MatchSet, depth int) ([]node, error) {
	if ms == nil || len(ms.GetRules()) == 0 {
		// An empty conjunction/disjunction has no unambiguous meaning; reject it.
		// NOTE: Envoy's PGV declares min_items:2 on MatchSet.rules. That bound was
		// NOT probed, so we reject only the unambiguous 0 case and accept 1.
		return nil, errors.New("matchpredicate: {or,and}_match.rules must be non-empty")
	}
	kids := make([]node, 0, len(ms.GetRules()))
	for i, r := range ms.GetRules() {
		k, err := compileNode(r, depth+1)
		if err != nil {
			return nil, fmt.Errorf("rules[%d]: %w", i, err)
		}
		kids = append(kids, k)
	}
	return kids, nil
}

func compileHeaders(hm *cmatcherv3.HttpHeadersMatch, response bool) (node, error) {
	if hm == nil || len(hm.GetHeaders()) == 0 {
		return nil, errors.New("matchpredicate: http_*_headers_match.headers must be non-empty")
	}
	ms := make([]headerMatcher, 0, len(hm.GetHeaders()))
	for i, h := range hm.GetHeaders() {
		m, err := headermatch.New(h)
		if err != nil {
			return nil, fmt.Errorf("headers[%d]: %w", i, err)
		}
		ms = append(ms, m)
	}
	return headersNode{matchers: ms, response: response}, nil
}
