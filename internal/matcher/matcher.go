package matcher

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	matchv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

// Canonical input TypeURLs the initial predicate evaluator subset recognizes.
// The set is scoped to RBAC's canonical surface per planner-time decision 17;
// future callers extend additively per ADR-0142 §Decision (iii).
const (
	headerInputTypeURL = "type.googleapis.com/envoy.type.matcher.v3.HttpRequestHeaderMatchInput"
)

// MatchContext is the request-side accessor abstraction the matcher engine
// consumes at request time. Each consuming filter implements this on its
// per-stream `*filter` (or a thin adapter) so the evaluator can introspect
// request data without importing filter-specific types.
//
// The initial accessor set is scoped to RBAC's canonical predicate surface
// per ADR-0142 §Decision (iii) + PLAN.md planner-time decision 17. Future
// filters widen the interface additively as new predicates land; existing
// implementations extend the new methods accordingly.
//
// HTTP/2 pseudo-header convention: Header(":path") and Header(":method")
// MAY route through Path() + Method() respectively (the proto-level
// `HttpRequestHeaderMatchInput` carries a `:`-prefixed name for the H2
// pseudo-headers). The interface contract does not REQUIRE this routing —
// callers may also serve the pseudo-headers from a per-stream header map —
// but the matcher engine treats either disposition uniformly via Header().
type MatchContext interface {
	// Header returns the request header value for name + a presence flag.
	// Empty values with present=true are distinct from absent (present=false).
	Header(name string) (value string, present bool)

	// Path returns the request URI path (`:path` H2 pseudo-header).
	Path() string

	// Method returns the request HTTP method (`:method` H2 pseudo-header).
	Method() string

	// SourceIP returns the downstream connection's source IP (peer-source;
	// pre-XFF resolution). nil for non-IP transports.
	SourceIP() net.IP

	// DestinationIP returns the listener-bound (local) IP for the downstream
	// connection. nil for non-IP transports.
	DestinationIP() net.IP

	// DestinationPort returns the listener-bound (local) port. 0 for
	// non-TCP/UDP transports or when not available.
	DestinationPort() uint32

	// RequestedServerName returns the TLS SNI value. Empty string for
	// plaintext connections or when SNI was not provided.
	RequestedServerName() string
}

// Matcher wraps a parsed xds.type.matcher.v3.Matcher tree. It is read-only
// post-`New`; `Evaluate` walks the cached structure without mutating shared
// state, so concurrent invocations are safe across goroutines.
type Matcher struct {
	root *compiledNode
}

// compiledNode is the parsed shape of one `xds.type.matcher.v3.Matcher` node.
// A node is either a MatcherList (linear scan of FieldMatchers) or a
// MatcherTree (exact/prefix-trie dispatch on input); phase-16 supports
// MatcherList only — MatcherTree usage is PARSE-REJECTED at New-time
// because RBAC's canonical config surface uses the MatcherList form. The
// `onNoMatch` field carries the optional `on_no_match` fallback leaf walked
// when no field matcher matches.
type compiledNode struct {
	fields    []*compiledField
	onNoMatch *compiledOnMatch
}

// compiledField is one parsed FieldMatcher: a predicate + an on_match leaf.
type compiledField struct {
	predicate compiledPredicate
	onMatch   *compiledOnMatch
}

// compiledOnMatch is the parsed shape of `xds.type.matcher.v3.Matcher_OnMatch`:
// either a terminal action `*anypb.Any` (leaf), or a nested matcher tree
// (recursive node). Exactly one of the two fields is non-nil post-parse.
type compiledOnMatch struct {
	action *anypb.Any    // non-nil → terminal leaf; the Any wraps the caller's terminal action proto
	nested *compiledNode // non-nil → recurse into nested matcher
}

// compiledPredicate is the parsed Predicate. Implementations: single
// (header/path/method/etc. via the input dispatch), AND, OR, NOT.
type compiledPredicate interface {
	matches(ctx MatchContext) bool
}

// New parses + validates the supplied Matcher tree at config-load time. Every
// terminal action `Any.TypeUrl` must appear in `supportedActionTypes` or the
// function PARSE-REJECTs with the verbatim envoy-go-only error wording
// `"matcher: terminal action type %q unsupported by this caller"` per
// ADR-0142 §Decision (ii) + SPEC §11.P3 + §2.6.
//
// supportedActionTypes is the caller-supplied allow-list of accepted terminal
// `Any.TypeUrl` strings. Phase-16's rbac filter supplies
// `[]string{"type.googleapis.com/envoy.config.rbac.v3.Action"}`. Empty (or
// nil) allow-list rejects every terminal.
func New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher, error) {
	if tree == nil {
		return nil, errors.New("matcher: nil tree")
	}
	allowed := make(map[string]struct{}, len(supportedActionTypes))
	for _, t := range supportedActionTypes {
		allowed[t] = struct{}{}
	}
	root, err := compileNode(tree, allowed)
	if err != nil {
		return nil, err
	}
	return &Matcher{root: root}, nil
}

// Evaluate walks the parsed tree at request time. Returns the matched
// terminal action `*anypb.Any` on first match, or `(nil, nil)` on no-match
// per the proto comment at `rbac.pb.go:43-46`. The error return is always
// nil today — the walk itself has no failure path — but the exported
// signature keeps the error slot so future evaluator extensions can surface
// one without breaking callers.
//
// Concurrent invocations are safe: the parsed tree is read-only post-New;
// no shared state mutates on the evaluator side.
func (m *Matcher) Evaluate(ctx MatchContext) (*anypb.Any, error) {
	if m == nil || m.root == nil {
		return nil, nil
	}
	return evaluateNode(m.root, ctx), nil
}

// evaluateNode is the recursive tree walker. Returns the matched terminal
// action on match, nil on no-match (including when the node's `on_no_match`
// is itself absent or yields no-match).
func evaluateNode(node *compiledNode, ctx MatchContext) *anypb.Any {
	for _, f := range node.fields {
		if f.predicate.matches(ctx) {
			return evaluateOnMatch(f.onMatch, ctx)
		}
	}
	if node.onNoMatch != nil {
		return evaluateOnMatch(node.onNoMatch, ctx)
	}
	return nil
}

// evaluateOnMatch dispatches a leaf on_match: terminal action OR nested
// matcher recursion.
func evaluateOnMatch(om *compiledOnMatch, ctx MatchContext) *anypb.Any {
	if om == nil {
		return nil
	}
	if om.action != nil {
		return om.action
	}
	if om.nested != nil {
		return evaluateNode(om.nested, ctx)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Compile-time parse.
// ----------------------------------------------------------------------------

func compileNode(tree *matchv3.Matcher, allowed map[string]struct{}) (*compiledNode, error) {
	out := &compiledNode{}

	switch mt := tree.GetMatcherType().(type) {
	case *matchv3.Matcher_MatcherList_:
		list := mt.MatcherList
		if list == nil {
			return nil, errors.New("matcher: nil matcher_list")
		}
		for i, fm := range list.GetMatchers() {
			cf, err := compileFieldMatcher(fm, allowed)
			if err != nil {
				return nil, fmt.Errorf("matcher: matcher_list[%d]: %w", i, err)
			}
			out.fields = append(out.fields, cf)
		}
	case *matchv3.Matcher_MatcherTree_:
		// MatcherTree (exact/prefix-trie dispatch) deferred — RBAC's canonical
		// config surface uses MatcherList exclusively. Adding MatcherTree
		// support is an additive future extension per ADR-0142 §Consequences.
		return nil, errors.New("matcher: matcher_tree variant unsupported in this build (use matcher_list)")
	case nil:
		// Tree with no MatcherType set + no MatcherList/MatcherTree oneof —
		// still permitted (the on_no_match fallback may be the only branch).
	default:
		return nil, fmt.Errorf("matcher: unknown matcher_type variant %T", mt)
	}

	if onm := tree.GetOnNoMatch(); onm != nil {
		om, err := compileOnMatch(onm, allowed)
		if err != nil {
			return nil, fmt.Errorf("matcher: on_no_match: %w", err)
		}
		out.onNoMatch = om
	}

	return out, nil
}

func compileFieldMatcher(fm *matchv3.Matcher_MatcherList_FieldMatcher, allowed map[string]struct{}) (*compiledField, error) {
	if fm == nil {
		return nil, errors.New("nil field_matcher")
	}
	if fm.GetPredicate() == nil {
		return nil, errors.New("field_matcher missing predicate")
	}
	pred, err := compilePredicate(fm.GetPredicate())
	if err != nil {
		return nil, fmt.Errorf("predicate: %w", err)
	}
	if fm.GetOnMatch() == nil {
		return nil, errors.New("field_matcher missing on_match")
	}
	om, err := compileOnMatch(fm.GetOnMatch(), allowed)
	if err != nil {
		return nil, fmt.Errorf("on_match: %w", err)
	}
	return &compiledField{predicate: pred, onMatch: om}, nil
}

func compileOnMatch(om *matchv3.Matcher_OnMatch, allowed map[string]struct{}) (*compiledOnMatch, error) {
	switch v := om.GetOnMatch().(type) {
	case *matchv3.Matcher_OnMatch_Action:
		if v.Action == nil {
			return nil, errors.New("on_match.action: nil TypedExtensionConfig")
		}
		ta := v.Action.GetTypedConfig()
		if ta == nil {
			return nil, errors.New("on_match.action: nil typed_config")
		}
		typeURL := ta.GetTypeUrl()
		if _, ok := allowed[typeURL]; !ok {
			return nil, fmt.Errorf("matcher: terminal action type %q unsupported by this caller", typeURL)
		}
		return &compiledOnMatch{action: ta}, nil
	case *matchv3.Matcher_OnMatch_Matcher:
		if v.Matcher == nil {
			return nil, errors.New("on_match.matcher: nil nested matcher")
		}
		nested, err := compileNode(v.Matcher, allowed)
		if err != nil {
			return nil, err
		}
		return &compiledOnMatch{nested: nested}, nil
	case nil:
		return nil, errors.New("on_match oneof unset")
	default:
		return nil, fmt.Errorf("unknown on_match variant %T", v)
	}
}

func compilePredicate(p *matchv3.Matcher_MatcherList_Predicate) (compiledPredicate, error) {
	switch mt := p.GetMatchType().(type) {
	case *matchv3.Matcher_MatcherList_Predicate_SinglePredicate_:
		return compileSinglePredicate(mt.SinglePredicate)
	case *matchv3.Matcher_MatcherList_Predicate_AndMatcher:
		children, err := compilePredicateList(mt.AndMatcher)
		if err != nil {
			return nil, err
		}
		return &andPredicate{children: children}, nil
	case *matchv3.Matcher_MatcherList_Predicate_OrMatcher:
		children, err := compilePredicateList(mt.OrMatcher)
		if err != nil {
			return nil, err
		}
		return &orPredicate{children: children}, nil
	case *matchv3.Matcher_MatcherList_Predicate_NotMatcher:
		child, err := compilePredicate(mt.NotMatcher)
		if err != nil {
			return nil, err
		}
		return &notPredicate{child: child}, nil
	case nil:
		return nil, errors.New("predicate match_type oneof unset")
	default:
		return nil, fmt.Errorf("unknown predicate match_type %T", mt)
	}
}

func compilePredicateList(list *matchv3.Matcher_MatcherList_Predicate_PredicateList) ([]compiledPredicate, error) {
	if list == nil || len(list.GetPredicate()) == 0 {
		return nil, errors.New("predicate_list empty or nil")
	}
	out := make([]compiledPredicate, 0, len(list.GetPredicate()))
	for i, child := range list.GetPredicate() {
		cp, err := compilePredicate(child)
		if err != nil {
			return nil, fmt.Errorf("predicate_list[%d]: %w", i, err)
		}
		out = append(out, cp)
	}
	return out, nil
}

func compileSinglePredicate(sp *matchv3.Matcher_MatcherList_Predicate_SinglePredicate) (compiledPredicate, error) {
	if sp == nil {
		return nil, errors.New("nil single_predicate")
	}
	if sp.GetInput() == nil {
		return nil, errors.New("single_predicate missing input")
	}
	inputCfg := sp.GetInput().GetTypedConfig()
	if inputCfg == nil {
		return nil, errors.New("single_predicate input missing typed_config")
	}
	inputTypeURL := inputCfg.GetTypeUrl()

	// Compile the value-side matcher. SinglePredicate's `matcher` oneof
	// supports either `value_match` (StringMatcher) or `custom_match`
	// (extension TypedExtensionConfig). Phase-16 supports value_match only.
	var sm stringMatcherEval
	switch mm := sp.GetMatcher().(type) {
	case *matchv3.Matcher_MatcherList_Predicate_SinglePredicate_ValueMatch:
		var err error
		sm, err = compileStringMatcher(mm.ValueMatch)
		if err != nil {
			return nil, fmt.Errorf("value_match: %w", err)
		}
	case *matchv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch:
		return nil, errors.New("single_predicate.custom_match extension unsupported in this build")
	case nil:
		return nil, errors.New("single_predicate matcher oneof unset")
	default:
		return nil, fmt.Errorf("unknown single_predicate matcher variant %T", mm)
	}

	// Dispatch on the input TypeURL. Adding a new input type is a localized
	// patch on this switch + (optionally) a new MatchContext accessor.
	switch inputTypeURL {
	case headerInputTypeURL:
		var in matcherv3.HttpRequestHeaderMatchInput
		if err := inputCfg.UnmarshalTo(&in); err != nil {
			return nil, fmt.Errorf("unmarshal HttpRequestHeaderMatchInput: %w", err)
		}
		return &headerPredicate{headerName: in.GetHeaderName(), value: sm}, nil
	default:
		return nil, fmt.Errorf("matcher: input type %q unsupported by this caller", inputTypeURL)
	}
}

// ----------------------------------------------------------------------------
// Predicate evaluators.
// ----------------------------------------------------------------------------

type headerPredicate struct {
	headerName string
	value      stringMatcherEval
}

func (p *headerPredicate) matches(ctx MatchContext) bool {
	v, present := ctx.Header(p.headerName)
	if !present {
		return false
	}
	return p.value.matches(v)
}

type andPredicate struct {
	children []compiledPredicate
}

func (p *andPredicate) matches(ctx MatchContext) bool {
	for _, c := range p.children {
		if !c.matches(ctx) {
			return false
		}
	}
	return true
}

type orPredicate struct {
	children []compiledPredicate
}

func (p *orPredicate) matches(ctx MatchContext) bool {
	for _, c := range p.children {
		if c.matches(ctx) {
			return true
		}
	}
	return false
}

type notPredicate struct {
	child compiledPredicate
}

func (p *notPredicate) matches(ctx MatchContext) bool {
	return !p.child.matches(ctx)
}

// ----------------------------------------------------------------------------
// StringMatcher (cncf xds variant) — initial subset: Exact / Prefix / Suffix /
// Contains / SafeRegex. Reuses the well-known StringMatcher semantics
// (case-insensitive when IgnoreCase=true).
// ----------------------------------------------------------------------------

type stringMatcherEval interface {
	matches(candidate string) bool
}

func compileStringMatcher(sm *matchv3.StringMatcher) (stringMatcherEval, error) {
	if sm == nil {
		return nil, errors.New("nil StringMatcher")
	}
	ic := sm.GetIgnoreCase()
	switch mp := sm.GetMatchPattern().(type) {
	case *matchv3.StringMatcher_Exact:
		return &exactStringMatcher{want: mp.Exact, ignoreCase: ic}, nil
	case *matchv3.StringMatcher_Prefix:
		return &prefixStringMatcher{prefix: mp.Prefix, ignoreCase: ic}, nil
	case *matchv3.StringMatcher_Suffix:
		return &suffixStringMatcher{suffix: mp.Suffix, ignoreCase: ic}, nil
	case *matchv3.StringMatcher_Contains:
		needle := mp.Contains
		if ic {
			// Lowercase the constant needle ONCE at compile time; the per-call
			// matches() only lowercases the candidate. Results are identical
			// (strings.ToLower is idempotent).
			needle = strings.ToLower(needle)
		}
		return &containsStringMatcher{needle: needle, ignoreCase: ic}, nil
	case *matchv3.StringMatcher_SafeRegex:
		if mp.SafeRegex == nil {
			return nil, errors.New("safe_regex: nil")
		}
		re, err := regexp.Compile(mp.SafeRegex.GetRegex())
		if err != nil {
			return nil, fmt.Errorf("safe_regex compile: %w", err)
		}
		return &regexStringMatcher{re: re}, nil
	case *matchv3.StringMatcher_Custom:
		return nil, errors.New("custom string matcher extension unsupported in this build")
	case nil:
		return nil, errors.New("StringMatcher match_pattern oneof unset")
	default:
		return nil, fmt.Errorf("unknown StringMatcher pattern %T", mp)
	}
}

type exactStringMatcher struct {
	want       string
	ignoreCase bool
}

func (m *exactStringMatcher) matches(s string) bool {
	if m.ignoreCase {
		return strings.EqualFold(m.want, s)
	}
	return m.want == s
}

type prefixStringMatcher struct {
	prefix     string
	ignoreCase bool
}

func (m *prefixStringMatcher) matches(s string) bool {
	if m.ignoreCase {
		return len(s) >= len(m.prefix) && strings.EqualFold(s[:len(m.prefix)], m.prefix)
	}
	return strings.HasPrefix(s, m.prefix)
}

type suffixStringMatcher struct {
	suffix     string
	ignoreCase bool
}

func (m *suffixStringMatcher) matches(s string) bool {
	if m.ignoreCase {
		if len(s) < len(m.suffix) {
			return false
		}
		return strings.EqualFold(s[len(s)-len(m.suffix):], m.suffix)
	}
	return strings.HasSuffix(s, m.suffix)
}

type containsStringMatcher struct {
	// needle is pre-lowercased at compile time when ignoreCase=true (see
	// compileStringMatcher) so matches() lowercases only the candidate.
	needle     string
	ignoreCase bool
}

func (m *containsStringMatcher) matches(s string) bool {
	if m.ignoreCase {
		return strings.Contains(strings.ToLower(s), m.needle)
	}
	return strings.Contains(s, m.needle)
}

type regexStringMatcher struct {
	re *regexp.Regexp
}

func (m *regexStringMatcher) matches(s string) bool {
	return m.re.MatchString(s)
}
