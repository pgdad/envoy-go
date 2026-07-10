// Package matchpredicate compiles a config/common/matcher/v3.MatchPredicate
// proto tree into an immutable, evaluable node tree, then evaluates it against
// a stream's request and response headers.
//
// Six of the ten oneof arms are supported. The four unsupported arms
// (request/response trailers, request/response generic_body) are EXPLICIT
// compile-time rejects, never a silent fall-through (ADR-0080).
//
// A compiled *Program holds NO mutable state and is safe to share across
// concurrent streams. Per-stream state lives in an *Evaluator.
package matchpredicate

import "net/http"

// Value is the tri-state of a predicate node.
type Value uint8

const (
	// Undetermined means the inputs this node depends on have not arrived yet.
	Undetermined Value = iota
	// True means the node matched.
	True
	// False means the node did not match.
	False
)

// state carries the per-stream inputs a node evaluates against.
type state struct {
	reqHdrs  http.Header // lowercase-keyed
	respHdrs http.Header // lowercase-keyed
	reqSeen  bool
	respSeen bool
}

type node interface{ eval(st *state) Value }

// anyNode is `any_match`: always True. Envoy constructs an always-true matcher
// whenever the arm is SET, regardless of the bool's value; we mirror that.
type anyNode struct{}

func (anyNode) eval(*state) Value { return True }

type notNode struct{ child node }

func (n notNode) eval(st *state) Value {
	switch n.child.eval(st) {
	case True:
		return False
	case False:
		return True
	}
	return Undetermined
}

type andNode struct{ children []node }

func (n andNode) eval(st *state) Value {
	sawUndetermined := false
	for _, c := range n.children {
		switch c.eval(st) {
		case False:
			return False // short-circuit: one False decides the conjunction
		case Undetermined:
			sawUndetermined = true
		}
	}
	if sawUndetermined {
		return Undetermined
	}
	return True
}

type orNode struct{ children []node }

func (n orNode) eval(st *state) Value {
	sawUndetermined := false
	for _, c := range n.children {
		switch c.eval(st) {
		case True:
			return True // short-circuit: one True decides the disjunction
		case Undetermined:
			sawUndetermined = true
		}
	}
	if sawUndetermined {
		return Undetermined
	}
	return False
}

// headerMatcher is an interface alias so the package does not leak its
// dependency into node signatures. *headermatch.Matcher satisfies it.
type headerMatcher interface{ Match(http.Header) bool }

// headersNode matches ALL of its matchers against one direction's headers
// (Envoy's HttpHeadersMatch is a conjunction).
type headersNode struct {
	matchers []headerMatcher
	response bool // false => request headers, true => response headers
}

func (n headersNode) eval(st *state) Value {
	h, seen := st.reqHdrs, st.reqSeen
	if n.response {
		h, seen = st.respHdrs, st.respSeen
	}
	if !seen {
		return Undetermined
	}
	for _, m := range n.matchers {
		if !m.Match(h) {
			return False
		}
	}
	return True
}
