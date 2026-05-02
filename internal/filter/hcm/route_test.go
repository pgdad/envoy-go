package hcm

import (
	"net/http"
	"net/url"
	"testing"
)

func reqWithPath(p string) *http.Request {
	return &http.Request{URL: &url.URL{Path: p}}
}

func TestMatchPath(t *testing.T) {
	m := matchPath("/health")
	if !m.matches("/health") {
		t.Error("matchPath should match exact path")
	}
	if m.matches("/HEALTH") {
		t.Error("matchPath is case-sensitive (per Envoy default)")
	}
	if m.matches("/health/") {
		t.Error("matchPath should NOT match trailing slash")
	}
	if m.matches("/api") {
		t.Error("matchPath should NOT match a different path")
	}
}

func TestMatchPrefix(t *testing.T) {
	m := matchPrefix("/api")
	for _, p := range []string{"/api", "/api/", "/api/v1", "/api/v1/users"} {
		if !m.matches(p) {
			t.Errorf("matchPrefix(/api) should match %q", p)
		}
	}
	if !m.matches("/apifoo") {
		t.Error("phase-04 matchPrefix is bytewise; expected to match /apifoo (documented divergence per ADR-0038)")
	}
	if m.matches("/v1/api") {
		t.Error("matchPrefix should not match a path that does not begin with the prefix")
	}
}

func TestRouteTableMatch_FirstMatchWins(t *testing.T) {
	t1 := &routeTable{routes: []routeEntry{
		{match: matchPath("/health")},
		{match: matchPrefix("/")},
	}}
	if e, idx, ok := t1.match(reqWithPath("/health")); !ok || e != &t1.routes[0] || idx != 0 {
		t.Errorf("first-match-wins should resolve /health to routes[0]; got idx=%d ok=%v", idx, ok)
	}
	if e, idx, ok := t1.match(reqWithPath("/anything-else")); !ok || e != &t1.routes[1] || idx != 1 {
		t.Errorf("first-match-wins should resolve /anything-else to routes[1] (catch-all); got idx=%d ok=%v", idx, ok)
	}
}

func TestRouteTableMatch_QueryStringExcluded(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPrefix("/api")},
	}}
	r := &http.Request{URL: &url.URL{Path: "/api", RawQuery: "q=1"}}
	if _, _, ok := tt.match(r); !ok {
		t.Error("match should evaluate URL.Path only (query excluded)")
	}
}

func TestRouteTableMatch_NoMatch(t *testing.T) {
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health")},
		{match: matchPrefix("/api")},
	}}
	if _, _, ok := tt.match(reqWithPath("/missing")); ok {
		t.Error("expected no-match for unrouted path")
	}
}

func TestRouteTableMatch_EmptyTable(t *testing.T) {
	tt := &routeTable{}
	if _, _, ok := tt.match(reqWithPath("/anything")); ok {
		t.Error("empty route table should never match")
	}
}
