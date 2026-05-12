package matcher

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	matchv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ----------------------------------------------------------------------------
// Test helpers + canonical TypeURL constants.
// ----------------------------------------------------------------------------

const (
	// canonical RBAC terminal action TypeURL per SPEC §11.P3 + §2.6 + ADR-0142.
	rbacActionTypeURL = "type.googleapis.com/envoy.config.rbac.v3.Action"
)

// mustAny packages a proto into an *anypb.Any.
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// rbacActionAny returns an *anypb.Any wrapping a canonical RBAC Action proto.
// This is the terminal-action shape the matcher engine returns on a leaf match
// per ADR-0142 + SPEC §11.P3.
func rbacActionAny(t *testing.T, name string, act rbacconfigv3.RBAC_Action) *anypb.Any {
	t.Helper()
	return mustAny(t, &rbacconfigv3.Action{Name: name, Action: act})
}

// terminalActionConfig wraps an *anypb.Any terminal action as the
// TypedExtensionConfig the matcher engine carries on OnMatch.Action leaves.
func terminalActionConfig(t *testing.T, a *anypb.Any) *xdscorev3.TypedExtensionConfig {
	t.Helper()
	return &xdscorev3.TypedExtensionConfig{Name: "action", TypedConfig: a}
}

// headerInputAny builds a TypedExtensionConfig wrapping
// envoy.type.matcher.v3.HttpRequestHeaderMatchInput{HeaderName: name}. Used as
// the SinglePredicate.Input on FieldMatcher entries that pull a request header.
func headerInputAny(t *testing.T, headerName string) *xdscorev3.TypedExtensionConfig {
	t.Helper()
	return &xdscorev3.TypedExtensionConfig{
		Name:        "header-input-" + headerName,
		TypedConfig: mustAny(t, &matcherv3.HttpRequestHeaderMatchInput{HeaderName: headerName}),
	}
}

// fieldMatcher returns a Matcher_MatcherList_FieldMatcher with a single
// SinglePredicate of (header == exact-value) + the supplied on_match leaf.
func fieldMatcherHeaderExact(t *testing.T, headerName, exact string, onMatch *matchv3.Matcher_OnMatch) *matchv3.Matcher_MatcherList_FieldMatcher {
	t.Helper()
	return &matchv3.Matcher_MatcherList_FieldMatcher{
		Predicate: &matchv3.Matcher_MatcherList_Predicate{
			MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputAny(t, headerName),
					Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &matchv3.StringMatcher{
							MatchPattern: &matchv3.StringMatcher_Exact{Exact: exact},
						},
					},
				},
			},
		},
		OnMatch: onMatch,
	}
}

// fieldMatcherHeaderPresent returns a FieldMatcher with a present-only
// SinglePredicate (no Matcher oneof set) — semantically "header is present".
//
// The matcher proto's SinglePredicate carries a `matcher` oneof (value_match
// XOR custom_match). For present-match semantics the initial evaluator
// interprets a value_match with empty Exact + non-empty present-check.
// We model present via a StringMatcher_SafeRegex of `.+` (any non-empty value).
func fieldMatcherHeaderPresent(t *testing.T, headerName string, onMatch *matchv3.Matcher_OnMatch) *matchv3.Matcher_MatcherList_FieldMatcher {
	t.Helper()
	return &matchv3.Matcher_MatcherList_FieldMatcher{
		Predicate: &matchv3.Matcher_MatcherList_Predicate{
			MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputAny(t, headerName),
					Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &matchv3.StringMatcher{
							MatchPattern: &matchv3.StringMatcher_Prefix{Prefix: ""},
						},
					},
				},
			},
		},
		OnMatch: onMatch,
	}
}

// fieldMatcherPathPrefix builds a FieldMatcher with a SinglePredicate matching
// the :path pseudo-header against a StringMatcher_Prefix.
func fieldMatcherPathPrefix(t *testing.T, prefix string, onMatch *matchv3.Matcher_OnMatch) *matchv3.Matcher_MatcherList_FieldMatcher {
	t.Helper()
	return &matchv3.Matcher_MatcherList_FieldMatcher{
		Predicate: &matchv3.Matcher_MatcherList_Predicate{
			MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
				SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
					Input: headerInputAny(t, ":path"),
					Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
						ValueMatch: &matchv3.StringMatcher{
							MatchPattern: &matchv3.StringMatcher_Prefix{Prefix: prefix},
						},
					},
				},
			},
		},
		OnMatch: onMatch,
	}
}

// onMatchAction wraps a terminal *anypb.Any in the OnMatch.Action leaf shape.
func onMatchAction(t *testing.T, a *anypb.Any) *matchv3.Matcher_OnMatch {
	t.Helper()
	return &matchv3.Matcher_OnMatch{
		OnMatch: &matchv3.Matcher_OnMatch_Action{Action: terminalActionConfig(t, a)},
	}
}

// onMatchNestedMatcher wraps a nested *Matcher as the OnMatch.Matcher branch.
func onMatchNestedMatcher(nested *matchv3.Matcher) *matchv3.Matcher_OnMatch {
	return &matchv3.Matcher_OnMatch{
		OnMatch: &matchv3.Matcher_OnMatch_Matcher{Matcher: nested},
	}
}

// stubCtx is a canned MatchContext used by the evaluator tests. Header lookups
// consult `headers`; the path/method/IP/SNI accessors return their slot value.
type stubCtx struct {
	headers     map[string]string
	path        string
	method      string
	srcIP       net.IP
	dstIP       net.IP
	dstPort     uint32
	sni         string
	headerCalls int // mutation under sync.Mutex; for stateless-concurrent test
	mu          sync.Mutex
}

func (s *stubCtx) Header(name string) (string, bool) {
	s.mu.Lock()
	s.headerCalls++
	s.mu.Unlock()
	// HTTP/2 pseudo-header convention: :path / :method route through
	// the dedicated accessor; otherwise the headers map.
	switch name {
	case ":path":
		return s.path, s.path != ""
	case ":method":
		return s.method, s.method != ""
	}
	v, ok := s.headers[name]
	return v, ok
}
func (s *stubCtx) Path() string                { return s.path }
func (s *stubCtx) Method() string              { return s.method }
func (s *stubCtx) SourceIP() net.IP            { return s.srcIP }
func (s *stubCtx) DestinationIP() net.IP       { return s.dstIP }
func (s *stubCtx) DestinationPort() uint32     { return s.dstPort }
func (s *stubCtx) RequestedServerName() string { return s.sni }

// allowedActionTypes is the canonical RBAC-only supportedActionTypes
// allow-list used by the majority of tests.
var allowedActionTypes = []string{rbacActionTypeURL}

// ----------------------------------------------------------------------------
// Tests — `New` parse-time disposition.
// ----------------------------------------------------------------------------

func TestNew_CanonicalActionTerminal_Accepted(t *testing.T) {
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice",
						onMatchAction(t, rbacActionAny(t, "p1", rbacconfigv3.RBAC_ALLOW))),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: unexpected error for canonical RBAC Action terminal: %v", err)
	}
	if m == nil {
		t.Fatal("New: returned nil Matcher with nil error")
	}
}

func TestNew_UnknownTerminalTypeURL_PARSE_REJECT(t *testing.T) {
	// Wrap a non-canonical terminal (a StringMatcher proto) — its TypeURL
	// is NOT in supportedActionTypes; New must PARSE-REJECT with verbatim
	// envoy-go-only error per SPEC §2.6 + ADR-0142.
	bogus := mustAny(t, &matcherv3.StringMatcher{
		MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "x"},
	})
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice",
						onMatchAction(t, bogus)),
				},
			},
		},
	}
	_, err := New(tree, allowedActionTypes)
	if err == nil {
		t.Fatal("New: want PARSE-REJECT for unknown terminal TypeURL, got nil error")
	}
	// Verbatim error wording per ADR-0142 + PLAN.md line 69.
	want := `matcher: terminal action type "type.googleapis.com/envoy.type.matcher.v3.StringMatcher" unsupported by this caller`
	if got := err.Error(); !strings.Contains(got, want) {
		t.Errorf("New: error wording want substring %q; got %q", want, got)
	}
}

func TestNew_EmptyAllowList_AllTerminalsRejected(t *testing.T) {
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice",
						onMatchAction(t, rbacActionAny(t, "p1", rbacconfigv3.RBAC_ALLOW))),
				},
			},
		},
	}
	_, err := New(tree, nil)
	if err == nil {
		t.Fatal("New(nil allow-list): want error rejecting every terminal, got nil")
	}
	_, err = New(tree, []string{})
	if err == nil {
		t.Fatal("New([]string{}): want error rejecting every terminal, got nil")
	}
}

// ----------------------------------------------------------------------------
// Tests — `Evaluate` walker dispatch.
// ----------------------------------------------------------------------------

func TestEvaluate_FirstMatchingPredicate_ReturnsTerminalAny(t *testing.T) {
	wantAny := rbacActionAny(t, "p1", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := &stubCtx{headers: map[string]string{"x-user": "alice"}}
	got, err := m.Evaluate(ctx)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got == nil {
		t.Fatal("Evaluate: want non-nil terminal Any on match, got nil")
	}
	if got.GetTypeUrl() != wantAny.GetTypeUrl() {
		t.Errorf("Evaluate: type URL want %q, got %q", wantAny.GetTypeUrl(), got.GetTypeUrl())
	}
}

func TestEvaluate_NoMatchingPredicate_NilNil(t *testing.T) {
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice",
						onMatchAction(t, rbacActionAny(t, "p1", rbacconfigv3.RBAC_ALLOW))),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := &stubCtx{headers: map[string]string{"x-user": "bob"}}
	got, gotErr := m.Evaluate(ctx)
	if gotErr != nil {
		t.Fatalf("Evaluate (no match): want nil error, got %v", gotErr)
	}
	if got != nil {
		t.Fatalf("Evaluate (no match): want nil Any per rbac.pb.go:43-46 proto comment, got %v", got)
	}
}

func TestEvaluate_HeaderPredicate_ExactMatch(t *testing.T) {
	wantAny := rbacActionAny(t, "exact", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-tenant", "acme", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Match path.
	matchCtx := &stubCtx{headers: map[string]string{"x-tenant": "acme"}}
	if got, _ := m.Evaluate(matchCtx); got == nil {
		t.Error("Evaluate (exact match): want non-nil terminal Any on match, got nil")
	}
	// Non-match path.
	noMatchCtx := &stubCtx{headers: map[string]string{"x-tenant": "other"}}
	if got, _ := m.Evaluate(noMatchCtx); got != nil {
		t.Error("Evaluate (exact mismatch): want nil terminal Any on no-match, got non-nil")
	}
}

func TestEvaluate_HeaderPredicate_PresentMatch(t *testing.T) {
	wantAny := rbacActionAny(t, "present", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderPresent(t, "x-debug", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Header present (any non-empty value).
	matchCtx := &stubCtx{headers: map[string]string{"x-debug": "yes"}}
	if got, _ := m.Evaluate(matchCtx); got == nil {
		t.Error("Evaluate (present): want non-nil terminal Any when header present")
	}
	// Header absent → no match.
	noMatchCtx := &stubCtx{headers: map[string]string{}}
	if got, _ := m.Evaluate(noMatchCtx); got != nil {
		t.Error("Evaluate (absent): want nil terminal Any when header absent, got non-nil")
	}
}

func TestEvaluate_PathPredicate_PrefixMatch(t *testing.T) {
	wantAny := rbacActionAny(t, "path-pfx", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherPathPrefix(t, "/admin/", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, _ := m.Evaluate(&stubCtx{path: "/admin/users"}); got == nil {
		t.Error("Evaluate (path prefix match): want non-nil")
	}
	if got, _ := m.Evaluate(&stubCtx{path: "/public"}); got != nil {
		t.Error("Evaluate (path prefix no-match): want nil")
	}
}

func TestEvaluate_AndPredicate_AllChildrenMatch(t *testing.T) {
	wantAny := rbacActionAny(t, "and", rbacconfigv3.RBAC_ALLOW)
	// AND ( header(x-user)==alice , header(x-tenant)==acme ).
	andPred := &matchv3.Matcher_MatcherList_Predicate{
		MatchType: &matchv3.Matcher_MatcherList_Predicate_AndMatcher{
			AndMatcher: &matchv3.Matcher_MatcherList_Predicate_PredicateList{
				Predicate: []*matchv3.Matcher_MatcherList_Predicate{
					{
						MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
							SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
								Input: headerInputAny(t, "x-user"),
								Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
									ValueMatch: &matchv3.StringMatcher{MatchPattern: &matchv3.StringMatcher_Exact{Exact: "alice"}},
								},
							},
						},
					},
					{
						MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
							SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
								Input: headerInputAny(t, "x-tenant"),
								Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
									ValueMatch: &matchv3.StringMatcher{MatchPattern: &matchv3.StringMatcher_Exact{Exact: "acme"}},
								},
							},
						},
					},
				},
			},
		},
	}
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					{Predicate: andPred, OnMatch: onMatchAction(t, wantAny)},
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, _ := m.Evaluate(&stubCtx{headers: map[string]string{"x-user": "alice", "x-tenant": "acme"}}); got == nil {
		t.Error("Evaluate (AND all match): want non-nil")
	}
	if got, _ := m.Evaluate(&stubCtx{headers: map[string]string{"x-user": "alice", "x-tenant": "other"}}); got != nil {
		t.Error("Evaluate (AND partial match): want nil")
	}
}

func TestEvaluate_OrPredicate_FirstChildMatch(t *testing.T) {
	wantAny := rbacActionAny(t, "or", rbacconfigv3.RBAC_ALLOW)
	orPred := &matchv3.Matcher_MatcherList_Predicate{
		MatchType: &matchv3.Matcher_MatcherList_Predicate_OrMatcher{
			OrMatcher: &matchv3.Matcher_MatcherList_Predicate_PredicateList{
				Predicate: []*matchv3.Matcher_MatcherList_Predicate{
					{
						MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
							SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
								Input: headerInputAny(t, "x-user"),
								Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
									ValueMatch: &matchv3.StringMatcher{MatchPattern: &matchv3.StringMatcher_Exact{Exact: "alice"}},
								},
							},
						},
					},
					{
						MatchType: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_{
							SinglePredicate: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate{
								Input: headerInputAny(t, "x-user"),
								Matcher: &matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch{
									ValueMatch: &matchv3.StringMatcher{MatchPattern: &matchv3.StringMatcher_Exact{Exact: "bob"}},
								},
							},
						},
					},
				},
			},
		},
	}
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					{Predicate: orPred, OnMatch: onMatchAction(t, wantAny)},
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// First child match.
	if got, _ := m.Evaluate(&stubCtx{headers: map[string]string{"x-user": "alice"}}); got == nil {
		t.Error("Evaluate (OR first child match): want non-nil")
	}
	// Second child match.
	if got, _ := m.Evaluate(&stubCtx{headers: map[string]string{"x-user": "bob"}}); got == nil {
		t.Error("Evaluate (OR second child match): want non-nil")
	}
	// No child match.
	if got, _ := m.Evaluate(&stubCtx{headers: map[string]string{"x-user": "eve"}}); got != nil {
		t.Error("Evaluate (OR no child match): want nil")
	}
}

func TestEvaluate_NestedTree_DepthThree(t *testing.T) {
	// Depth-3 nesting: outer matcher → inner matcher (via OnMatch.Matcher
	// branch) → leaf header predicate → action terminal.
	wantAny := rbacActionAny(t, "deep", rbacconfigv3.RBAC_ALLOW)
	leaf := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-tenant", "acme", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	mid := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice", onMatchNestedMatcher(leaf)),
				},
			},
		},
	}
	outer := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherPathPrefix(t, "/api/", onMatchNestedMatcher(mid)),
				},
			},
		},
	}
	m, err := New(outer, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// All three levels match.
	full := &stubCtx{path: "/api/v1", headers: map[string]string{"x-user": "alice", "x-tenant": "acme"}}
	got, err := m.Evaluate(full)
	if err != nil {
		t.Fatalf("Evaluate (depth-3 all match): %v", err)
	}
	if got == nil {
		t.Fatal("Evaluate (depth-3 all match): want non-nil terminal Any")
	}
	// Outer matches but mid doesn't.
	partial := &stubCtx{path: "/api/v1", headers: map[string]string{"x-user": "eve", "x-tenant": "acme"}}
	if got, _ := m.Evaluate(partial); got != nil {
		t.Error("Evaluate (depth-3 mid mismatch): want nil")
	}
	// Outer doesn't match → entire tree short-circuits to nil.
	none := &stubCtx{path: "/public", headers: map[string]string{"x-user": "alice", "x-tenant": "acme"}}
	if got, _ := m.Evaluate(none); got != nil {
		t.Error("Evaluate (depth-3 outer mismatch): want nil")
	}
}

func TestMatchContext_AdapterPattern(t *testing.T) {
	// Verify MatchContext is implemented as a Go interface and that arbitrary
	// types implementing it interoperate. We use the test's stubCtx directly +
	// also a second adapter to assert the interface boundary.
	var _ MatchContext = (*stubCtx)(nil)

	wantAny := rbacActionAny(t, "adapter", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// A second adapter implementing MatchContext via a different concrete type.
	adapter := &canned{header: "alice", path: "/anything", method: "GET"}
	var _ MatchContext = adapter
	got, err := m.Evaluate(adapter)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got == nil {
		t.Fatal("Evaluate: want non-nil terminal Any via adapter")
	}
}

// canned is a second MatchContext implementer used by the adapter test to
// assert the interface boundary (NOT just the stubCtx concrete type).
type canned struct {
	header string
	path   string
	method string
}

func (c *canned) Header(name string) (string, bool) {
	if name == "x-user" {
		return c.header, c.header != ""
	}
	return "", false
}
func (c *canned) Path() string                { return c.path }
func (c *canned) Method() string              { return c.method }
func (c *canned) SourceIP() net.IP            { return nil }
func (c *canned) DestinationIP() net.IP       { return nil }
func (c *canned) DestinationPort() uint32     { return 0 }
func (c *canned) RequestedServerName() string { return "" }

func TestMatcher_StatelessAcrossEvaluations(t *testing.T) {
	// Concurrent Evaluate calls must be safe (the parsed tree is read-only
	// post-New per ADR-0142 §Decision). Verifies the matcher does not mutate
	// shared state across Evaluate invocations. Race-detector will fail the
	// test on accidental sharing.
	wantAny := rbacActionAny(t, "concurrent", rbacconfigv3.RBAC_ALLOW)
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice", onMatchAction(t, wantAny)),
				},
			},
		},
	}
	m, err := New(tree, allowedActionTypes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var wg sync.WaitGroup
	const N = 64
	wg.Add(N)
	var failures int32
	var mu sync.Mutex
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			user := "alice"
			if i%2 == 1 {
				user = "bob"
			}
			ctx := &stubCtx{headers: map[string]string{"x-user": user}}
			got, err := m.Evaluate(ctx)
			if err != nil {
				mu.Lock()
				failures++
				mu.Unlock()
				return
			}
			if (i%2 == 0 && got == nil) || (i%2 == 1 && got != nil) {
				mu.Lock()
				failures++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if failures != 0 {
		t.Errorf("concurrent Evaluate: %d failures across %d goroutines", failures, N)
	}
}

// Sanity assertion that errors.New(...) shape is reused (no fmt.Errorf-only
// wrappers swallowing the verbatim wording).
func TestNew_ErrorWordingIsUnwrappable(t *testing.T) {
	bogus := mustAny(t, &matcherv3.StringMatcher{MatchPattern: &matcherv3.StringMatcher_Exact{Exact: "x"}})
	tree := &matchv3.Matcher{
		MatcherType: &matchv3.Matcher_MatcherList_{
			MatcherList: &matchv3.Matcher_MatcherList{
				Matchers: []*matchv3.Matcher_MatcherList_FieldMatcher{
					fieldMatcherHeaderExact(t, "x-user", "alice", onMatchAction(t, bogus)),
				},
			},
		},
	}
	_, err := New(tree, allowedActionTypes)
	if err == nil {
		t.Fatal("New: want error")
	}
	// Either the top-level error or a wrapped chain must include the verbatim
	// phrase. We accept both flat-error and wrapped-error shapes.
	for unwrapped := err; unwrapped != nil; unwrapped = errors.Unwrap(unwrapped) {
		if strings.Contains(unwrapped.Error(), "unsupported by this caller") {
			return
		}
	}
	t.Errorf("New: error chain does not contain the verbatim PARSE-REJECT phrase; got %q", err)
}
