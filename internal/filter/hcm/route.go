package hcm

import (
	"bufio"
	"context"
	"net/http"
	"strings"
)

// routeMatch is the predicate side of a routeEntry. Each route binds exactly
// one match implementation. Phase 04 supports two: matchPrefix (bytewise
// prefix on req.URL.Path) and matchPath (case-sensitive exact equality).
// Other Envoy match shapes (safe_regex, path_separated_prefix, headers, etc.)
// are rejected at config-parse time per ADR-0038.
type routeMatch interface {
	matches(path string) bool
}

// matchPath performs a case-sensitive exact comparison on the request URL's
// Path component. Envoy's default case_sensitive=true is the only mode
// supported in phase 04.
type matchPath string

func (m matchPath) matches(p string) bool { return string(m) == p }

// matchPrefix performs a bytewise prefix match on the request URL's Path
// component. Phase 04 documents a planned divergence from Envoy's
// segment-aware prefix semantics (ADR-0038): "/api" matches "/apifoo" under
// matchPrefix; under Envoy's segment-aware semantics it would not. The
// fixture driver issues only segment-boundary paths, so the divergence is
// not surfaced in the differential gate.
type matchPrefix string

func (m matchPrefix) matches(p string) bool { return strings.HasPrefix(p, string(m)) }

// routeAction is the action half of a routeEntry. Implementations live in
// actions.go: directResponseAction (synthesizes a local reply) and
// routerAction (proxies via Cluster.Dial). Returning errCloseAfterAction
// from do signals the connection loop to close after this iteration; other
// non-nil errors propagate and trigger downstream close.
type routeAction interface {
	do(ctx context.Context, req *http.Request, bw *bufio.Writer) error
}

// routeEntry pairs a match predicate with the action to invoke on a hit. The
// action interface is defined above; implementations live in actions.go to
// keep route.go free of any dependency on the cluster manager.
type routeEntry struct {
	match  routeMatch
	action routeAction
}

// routeTable is the resolved route_config. Routes are evaluated in
// declaration order; first match wins.
type routeTable struct {
	routes []routeEntry
}

// match walks the routes in declaration order, returning the first entry
// whose match predicate accepts req.URL.Path. Returns (nil, false) on no
// match. Query-string is excluded (URL.RawQuery is not considered).
func (t *routeTable) match(req *http.Request) (*routeEntry, bool) {
	p := req.URL.Path
	for i := range t.routes {
		if t.routes[i].match.matches(p) {
			return &t.routes[i], true
		}
	}
	return nil, false
}
