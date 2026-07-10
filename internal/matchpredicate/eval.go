package matchpredicate

import "net/http"

// Evaluator carries one stream's evaluation state for a shared Program.
type Evaluator struct {
	p  *Program
	st state
}

// NewEvaluator mints per-stream state. The Program is not mutated.
func (p *Program) NewEvaluator() *Evaluator { return &Evaluator{p: p} }

// FeedRequestHeaders records the request headers. h MUST be lowercase-keyed.
func (e *Evaluator) FeedRequestHeaders(h http.Header) {
	e.st.reqHdrs, e.st.reqSeen = h, true
}

// FeedResponseHeaders records the response headers. h MUST be lowercase-keyed.
func (e *Evaluator) FeedResponseHeaders(h http.Header) {
	e.st.respHdrs, e.st.respSeen = h, true
}

// Value returns the current tri-state of the tree.
func (e *Evaluator) Value() Value { return e.p.root.eval(&e.st) }

// Resolve collapses the tri-state to a match decision. A still-Undetermined
// tree resolves to false: this is a total-function guard, near-unreachable for
// HTTP (envoy-go always synthesizes a response, even a local-reply 5xx, so
// EncodeHeaders always runs), NOT an observed reference behavior.
func (e *Evaluator) Resolve() bool { return e.Value() == True }
