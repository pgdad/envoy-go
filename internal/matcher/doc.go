// Package matcher implements a generic xds.type.matcher.v3.Matcher match-tree
// evaluator usable by HTTP filters that consume the matcher-engine surface
// (e.g., envoy.filters.http.rbac, and future ext_authz / jwt_authn / oauth2 /
// ext_proc filters that adopt the same primitive).
//
// The package is cross-phase reusable: callers extend `supportedActionTypes`
// for their own terminal action TypeURLs at `New`-time + widen MatchContext
// additively as new predicate-accessor methods are needed by future callers.
// Phase 16's rbac filter is the FIRST consumer; the initial `MatchContext`
// accessor subset (Header / Path / Method / SourceIP / DestinationIP /
// DestinationPort / RequestedServerName) is scoped to RBAC's canonical
// predicate surface per ADR-0142 §Decision (iii) + PLAN.md planner-time
// decision 17. Future filters widen the interface; existing implementations
// extend the new methods additively.
//
// API surface:
//
//   - `Matcher` opaque type wrapping a parsed read-only match tree.
//   - `New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher,
//     error)` — parses the proto match tree at config-load time + validates
//     every terminal action `Any.TypeUrl` against the caller-supplied
//     allow-list. Unknown TypeURLs surface as a PARSE-REJECT with verbatim
//     envoy-go-only error wording `"matcher: terminal action type %q
//     unsupported by this caller"` per ADR-0142 §Decision (ii).
//   - `(m *Matcher) Evaluate(ctx MatchContext) (*anypb.Any, error)` — walks
//     the parsed tree at request time + returns the first matching terminal
//     action `*anypb.Any` OR `(nil, nil)` when no field matcher matches (per
//     the proto comment on `envoy.extensions.filters.http.rbac.v3.RBAC.matcher`
//     at `rbac.pb.go:43-46`: "Requests not matching any matcher will be
//     denied" — i.e. the matcher engine signals no-match to its caller; the
//     caller interprets the no-match disposition filter-specifically).
//   - `MatchContext` request-side accessor interface implemented by each
//     consuming filter on its per-stream `*filter` (or a thin adapter
//     wrapping it). The matcher package itself imports NO filter-specific
//     types — the cross-phase reuse rests on this clean interface boundary.
//
// Predicate evaluator scope (initial subset per PLAN.md line 69 + planner-
// time decision 17):
//
//   - SinglePredicate.ValueMatch with input
//     `envoy.type.matcher.v3.HttpRequestHeaderMatchInput` — routes through
//     `MatchContext.Header(name)` (HTTP/2 pseudo-headers `:path` + `:method`
//     route via `Path()` + `Method()` respectively).
//   - AND-predicate combinator over predicate children.
//   - OR-predicate combinator over predicate children.
//   - NOT-predicate combinator.
//
// Additional input TypeURLs (DestinationIP / DestinationPort / SourceIP /
// ServerName via the network common-inputs package) land additively as future
// callers require them; the dispatch is a TypeURL-keyed switch in
// `predicateFromSinglePredicate` such that adding a new input is a localized
// patch.
//
// Cross-phase reuse intent (per ADR-0142 §Decision (i) + §Consequences):
// ext_authz, jwt_authn, oauth2, ext_proc all consume the same
// `xds.type.matcher.v3.Matcher` primitive for parts of their config surface.
// Each future filter extends `supportedActionTypes` to include its own
// terminal action TypeURLs + widens `MatchContext` additively. No in-place
// ADR-0142 amendments are anticipated for the surface defined here; the
// additive-widening discipline preserves the framework primitive boundary.
//
// Forward-declared MatchContext accessors (Task 3 carry-forward I-2; lighter-
// touch option (b) per phase-16 Task 6 BOOTSTRAP_PROMPT): the MatchContext
// methods DestinationIP / DestinationPort / SourceIP / RequestedServerName
// are declared on the interface but the matcher engine's
// `predicateFromSinglePredicate` switch does not yet dispatch on their
// corresponding input TypeURLs (only HttpRequestHeaderMatchInput is wired at
// phase-16 MVP per planner-time decision 17). The accessor methods are
// forward-declared for Task 7+ consumers and for future filters
// (ext_authz / jwt_authn / oauth2 / ext_proc) that will register additional
// input TypeURLs via the dispatch switch. The disposition is intentional:
// adding the input-TypeURL handlers without consumers would be dead code;
// declaring the accessors now preserves the interface stability when those
// handlers land additively.
package matcher
