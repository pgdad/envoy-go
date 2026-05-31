package rbac

import (
	"fmt"
	"net"
	"sort"

	matchv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"

	"github.com/esalaine/envoy-go/internal/matcher"
)

// actionTypeURL is the canonical matcher-engine terminal action TypeURL
// per §11.P3 + §2.6 + ADR-0142. BuildMatcherEngine passes it as the
// sole entry of matcher.New's supportedActionTypes allow-list; non-canonical
// terminal TypeURLs PARSE-REJECT at config-load time.
const actionTypeURL = "type.googleapis.com/envoy.config.rbac.v3.Action"

// CompiledRulesEngine carries the parsed config.rbac.v3.RBAC sub-message for
// either primary or shadow. CEL fields (condition / checked_condition /
// cel_config) are NOT cached — silent-ignored at parse + runtime per §1.1
// amendment 6 + Q7. audit_logging_options similarly silent-ignored per §2.1.1.
type CompiledRulesEngine struct {
	action   rbacconfigv3.RBAC_Action // ALLOW=0 / DENY=1 / LOG=2 (PGV defined_only)
	policies []*compiledPolicy        // lexicographic-order-of-policy-name (sorted at parse per rbac.pb.go:268-269)
}

// CompiledMatcherEngine wraps the parsed xds.type.matcher.v3.Matcher tree via
// the internal/matcher framework primitive per ADR-0142. The tree is parsed +
// validated at config-load time inside matcher.New (PARSE-REJECT for unknown
// terminal Any.TypeUrl per §11.P3 + §2.6).
type CompiledMatcherEngine struct {
	// tree is the parsed match-tree wrapper. Read-only post-BuildMatcherEngine;
	// Evaluate calls are concurrent-safe.
	tree *matcher.Matcher
}

// compiledPolicy is one entry from config.rbac.v3.RBAC.policies. Permission +
// Principal evaluator trees are pre-compiled at parse time per ADR-0143.
type compiledPolicy struct {
	name        string                // map key from policies map; preserved verbatim
	permissions []permissionEvaluator // OR-semantic at runtime (per Policy proto comment)
	principals  []principalEvaluator  // OR-semantic at runtime
}

// EngineResult is the disposition produced by engine evaluation. Allowed and
// Denied are the only states per SPEC §6.9; LOG-partial folds into Allowed
// per §1.1 amendment 5.
type EngineResult int

// EngineResult enum values per SPEC §6.9.
const (
	// Allowed is the post-walk disposition that passes the request through.
	// Covers ALLOW-matched, DENY-no-match, and LOG-anything paths.
	Allowed EngineResult = iota
	// Denied is the post-walk disposition that triggers
	// SendLocalReply(403, ...) at the caller. Covers ALLOW-no-match,
	// DENY-matched, and matcher-engine no-match per rbac.pb.go:43-46.
	Denied
)

// BuildRulesEngine parses one config.rbac.v3.RBAC sub-message per
// SPEC §6.5 + ADR-0141. Validates the action enum (defensive PGV-mirror per
// amendment 4 — defined_only); sorts policy names lexicographically per
// rbac.pb.go:268-269; recursively builds per-policy permission + principal
// evaluators via the buildPermissionEvaluators / buildPrincipalEvaluators
// entry points. audit_logging_options + 3 CEL fields silent-ignored per
// §2.1.1 + §2.1.2 + amendment 6 + Q7.
//
// profile declares the input-capability surface: ProfileHTTP permits all arms
// (byte-identical to pre-Profile behavior, R4); ProfileL4 PARSE-REJECTs
// HTTP-only arms (header + url_path permissions + principals) at compile per
// D-26.3-1 + SPEC §3.4.
func BuildRulesEngine(r *rbacconfigv3.RBAC, profile Profile) (*CompiledRulesEngine, error) {
	// Defensive PGV-mirror per §1.1 amendment 4: action enum defined_only.
	// Although the proto-typed enum can hold only values in the .pb.go
	// generated set after a successful Unmarshal, the defensive check guards
	// against future enum extensions + makes the invariant explicit at the
	// envoy-go boundary.
	action := r.GetAction()
	switch action {
	case rbacconfigv3.RBAC_ALLOW, rbacconfigv3.RBAC_DENY, rbacconfigv3.RBAC_LOG:
		// OK.
	default:
		return nil, fmt.Errorf("rbac: invalid action %v (must be ALLOW/DENY/LOG)", action)
	}

	// Sort policy names lexicographically per rbac.pb.go:268-269 proto comment.
	// Determines per-stream walk order at request time (first-match wins per
	// SPEC §6.9).
	names := make([]string, 0, len(r.GetPolicies()))
	for name := range r.GetPolicies() {
		names = append(names, name)
	}
	sort.Strings(names)

	policies := make([]*compiledPolicy, 0, len(names))
	for _, name := range names {
		p := r.GetPolicies()[name]
		// Defensive PGV-mirror per amendment 4: permissions + principals
		// each min_items=1.
		if len(p.GetPermissions()) == 0 {
			return nil, fmt.Errorf("rbac: policy %q must have at least one permission", name)
		}
		if len(p.GetPrincipals()) == 0 {
			return nil, fmt.Errorf("rbac: policy %q must have at least one principal", name)
		}
		perms, err := buildPermissionEvaluators(p.GetPermissions(), profile)
		if err != nil {
			return nil, fmt.Errorf("rbac: policy %q permissions: %w", name, err)
		}
		prins, err := buildPrincipalEvaluators(p.GetPrincipals(), profile)
		if err != nil {
			return nil, fmt.Errorf("rbac: policy %q principals: %w", name, err)
		}
		policies = append(policies, &compiledPolicy{
			name:        name,
			permissions: perms,
			principals:  prins,
		})
		// audit_logging_options + Policy.condition + Policy.checked_condition
		// + Policy.cel_config are NOT extracted here — silent-ignored per
		// §2.1.1 + §2.1.2 + amendment 6 + Q7. The compiledPolicy struct has
		// NO slot for them (the silent-ignore is structural).
	}

	return &CompiledRulesEngine{
		action:   action,
		policies: policies,
	}, nil
}

// BuildMatcherEngine wraps a parsed xds.type.matcher.v3.Matcher tree
// via the internal/matcher framework primitive per ADR-0142. The
// supportedActionTypes allow-list is the canonical RBAC Action TypeURL only
// per §2.6 + §11.P3; non-canonical terminal TypeURLs PARSE-REJECT inside
// matcher.New with envoy-go-only error wording, which this helper wraps with
// the `rbac: matcher:` prefix for caller-side error chains.
//
// profile is accepted for signature symmetry with BuildRulesEngine and
// future-proofing (D-26.3-1 + SPEC §3.4). The matcher-path carries NO
// rbac permission/principal leaf that profile.permits() could gate (the
// leaves are generic internal/matcher predicates; the terminal is an
// rbac.v3.Action). Profile-gating on the matcher-path is therefore vacuous
// — the rules-path is the GUARANTEED reject target.
func BuildMatcherEngine(m *matchv3.Matcher, profile Profile) (*CompiledMatcherEngine, error) {
	tree, err := matcher.New(m, []string{actionTypeURL})
	if err != nil {
		return nil, fmt.Errorf("rbac: matcher: %w", err)
	}
	return &CompiledMatcherEngine{tree: tree}, nil
}

// Evaluate walks policies in lexicographic-sorted order; first match wins.
// Per SPEC §6.9 + rbac.pb.go:268-269.
//
// Action mapping (per SPEC §6.9 + §1.1 amendment 5):
//   - ALLOW + match     → Allowed + matchedPolicyName
//   - ALLOW + no-match  → Denied + ""
//   - DENY + match      → Denied + matchedPolicyName
//   - DENY + no-match   → Allowed + ""
//   - LOG + match       → Allowed + matchedPolicyName (LOG always-allows;
//     matched-policy captured for per-policy emission per amendment 5)
//   - LOG + no-match    → Allowed + ""
func (re *CompiledRulesEngine) Evaluate(ctx EvalContext) (EngineResult, string) {
	var matchedPolicy string
	for _, p := range re.policies {
		if policyMatches(p, ctx) {
			matchedPolicy = p.name
			break
		}
	}
	matched := matchedPolicy != ""
	switch re.action {
	case rbacconfigv3.RBAC_ALLOW:
		if matched {
			return Allowed, matchedPolicy
		}
		return Denied, ""
	case rbacconfigv3.RBAC_DENY:
		if matched {
			return Denied, matchedPolicy
		}
		return Allowed, ""
	case rbacconfigv3.RBAC_LOG:
		// Always-allow per §1.1 amendment 5; matchedPolicy captured for
		// per-policy counter emission. access_log_hint dynamic metadata NOT
		// emitted in envoy-go MVP per amendment 5 divergence-window.
		return Allowed, matchedPolicy
	}
	// Defensive: PGV-mirror at parse time ensures action ∈ {ALLOW, DENY, LOG};
	// this branch is structurally unreachable.
	return Denied, ""
}

// policyMatches evaluates one *compiledPolicy against ctx. Per SPEC §6.5 +
// §6.9: permissions[] OR-semantic AND principals[] OR-semantic. Short-circuit
// on first match on each side; the side-AND structure means a non-matching
// permissions-side short-circuits before iterating principals.
func policyMatches(p *compiledPolicy, ctx EvalContext) bool {
	permMatch := false
	for _, perm := range p.permissions {
		if perm.evaluatePermission(ctx) {
			permMatch = true
			break
		}
	}
	if !permMatch {
		return false
	}
	for _, prin := range p.principals {
		if prin.evaluatePrincipal(ctx) {
			return true
		}
	}
	return false
}

// Evaluate walks the matcher tree via the framework primitive (ADR-0142) +
// maps the terminal Any (canonical rbac.v3.Action) back to an EngineResult.
// Per SPEC §6.9:
//
//   - tree.Evaluate returns (nil, nil) on no-match per rbac.pb.go:43-46 →
//     caller interprets as DENY.
//   - tree.Evaluate returns a terminal Any wrapping rbacconfigv3.Action;
//     defensive-unmarshal (the PARSE-REJECT at BuildMatcherEngine guarantees
//     the TypeURL is canonical, but unmarshal can still fail theoretically on
//     malformed Any bytes).
//   - action.action ∈ {ALLOW, LOG} → Allowed + action.GetName().
//   - action.action == DENY → Denied + action.GetName().
//   - any other action enum (future variant; defensive) → Denied.
func (me *CompiledMatcherEngine) Evaluate(ctx EvalContext) (EngineResult, string) {
	actionAny, err := me.tree.Evaluate(&matcherCtxAdapter{ctx: ctx})
	if err != nil || actionAny == nil {
		// No-match per rbac.pb.go:43-46 proto comment "Requests not matching
		// any matcher will be denied."
		return Denied, ""
	}
	var action rbacconfigv3.Action
	if err := actionAny.UnmarshalTo(&action); err != nil {
		// Defensive: PARSE-REJECT at BuildMatcherEngine should have caught any
		// non-canonical TypeURL. Reaching this branch implies malformed Any bytes
		// inside a canonical TypeURL — treat as DENY.
		return Denied, ""
	}
	switch action.GetAction() {
	case rbacconfigv3.RBAC_ALLOW, rbacconfigv3.RBAC_LOG:
		return Allowed, action.GetName()
	case rbacconfigv3.RBAC_DENY:
		return Denied, action.GetName()
	}
	return Denied, ""
}

// ---------------------------------------------------------------------------
// matcherCtxAdapter — rbac↔matcher bridge per ADR-0142 §Decision (iii).
//
// Adapts the rbac-side EvalContext to the matcher-engine framework primitive's
// MatchContext. Each method delegates to the underlying EvalContext.
//
// CF-2: SourceIP() → DirectRemoteIP() mapping is load-bearing. The
// xds.type.matcher.v3.SourceIPInput proto semantics are
// peer-source-pre-XFF (NOT XFF-resolved); EvalContext's DirectRemoteIP() is
// the matching accessor. RemoteIP() (XFF-resolved) is NOT exposed via the
// MatchContext surface — matcher-engine predicates filtering on XFF-resolved
// IP would need a future MatchContext widening per ADR-0142 §Decision (iii)
// additive-extension contract.
// ---------------------------------------------------------------------------

// matcherCtxAdapter wraps an EvalContext for consumption by the internal/
// matcher framework primitive. Per CF-2: SourceIP() returns DirectRemoteIP()
// (peer-source-pre-XFF per xds.type.matcher.v3.SourceIPInput semantics), NOT
// RemoteIP() (XFF-resolved).
type matcherCtxAdapter struct {
	ctx EvalContext
}

// Compile-time assertion: matcherCtxAdapter implements matcher.MatchContext.
var _ matcher.MatchContext = (*matcherCtxAdapter)(nil)

// Header delegates to EvalContext.Header(name) verbatim.
func (a *matcherCtxAdapter) Header(name string) (string, bool) { return a.ctx.Header(name) }

// Path returns EvalContext.URLPath() (the `:path` H2 pseudo-header). The
// MatchContext API uses Path() naming; the rbac-side EvalContext uses
// URLPath() for naming-parity with the rbac.v3 proto field name (UrlPath).
func (a *matcherCtxAdapter) Path() string { return a.ctx.URLPath() }

// Method returns EvalContext.Method() (the `:method` H2 pseudo-header).
func (a *matcherCtxAdapter) Method() string { return a.ctx.Method() }

// SourceIP returns EvalContext.DirectRemoteIP() per CF-2: the matcher's
// SourceIP semantic is peer-source-pre-XFF per
// xds.type.matcher.v3.SourceIPInput; DirectRemoteIP() is the matching rbac-
// side accessor.
func (a *matcherCtxAdapter) SourceIP() net.IP { return a.ctx.DirectRemoteIP() }

// DestinationIP delegates verbatim.
func (a *matcherCtxAdapter) DestinationIP() net.IP { return a.ctx.DestinationIP() }

// DestinationPort delegates verbatim.
func (a *matcherCtxAdapter) DestinationPort() uint32 { return a.ctx.DestinationPort() }

// RequestedServerName delegates verbatim (TLS SNI).
func (a *matcherCtxAdapter) RequestedServerName() string { return a.ctx.RequestedServerName() }
