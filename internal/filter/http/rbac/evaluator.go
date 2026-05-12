package rbac

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// permissionEvaluator is the per-Permission evaluator interface. Concrete
// implementations land at Task 4 (Permission Large 11 + AND/OR/NOT + 3
// PARSE-REJECT per ADR-0143).
//
// evaluatePermission returns true iff the permission matches the request.
// ctx provides per-stream accessors (headers / path / IP / destination port /
// SNI / sourced-metadata) per SPEC §6.2.
type permissionEvaluator interface {
	evaluatePermission(ctx evalContext) bool
}

// principalEvaluator is the per-Principal evaluator interface. Concrete
// implementations land at Task 5 (Principal Large 11 + prinAuthenticated
// three-case + 3 PARSE-REJECT per ADR-0143). At Task 4 the interface is
// declared so compiledPolicy + buildPrincipalEvaluators can refer to it.
//
// evaluatePrincipal returns true iff the principal matches the request. ctx
// provides per-stream accessors (TLS principal candidates / headers / IP /
// sourced-metadata / filter-state).
type principalEvaluator interface {
	evaluatePrincipal(ctx evalContext) bool
}

// evalContext is the per-stream accessor abstraction the evaluators consume.
// The *filter implements this at runtime; the full surface (Header / URLPath /
// Method / DirectRemoteIP / RemoteIP / DestinationIP / DestinationPort /
// RequestedServerName / DownstreamPrincipal / SourcedMetadata / FilterState)
// lands across Tasks 4 + 5 + 6 + 7 per SPEC §6.2.
//
// At Task 4 the interface carried only the Permission-relevant accessors per
// SPEC §6.5 + ADR-0143 §Decision (Permission section). Task 5 widens with
// Principal-side accessors (DirectRemoteIP / RemoteIP / DownstreamPrincipal /
// SourcedMetadata / FilterState) per ADR-0143 §Decision (iii). Task 7 widens
// with Method() to feed the matcherCtxAdapter's MatchContext.Method() bridge
// (the matcher framework primitive ships a Method accessor per ADR-0142 §
// Decision (iii) initial-surface). None of the Permission/Principal Large 11+11
// evaluators consume Method directly; it surfaces only through the matcher-
// engine bridge.
//
// SourcedMetadata() + FilterState() accessor shapes are FORWARD-COMPAT-ONLY
// placeholders (both runtimes are always-FALSE per §2.5 + §8.10). The
// `any`-typed returns let future dynamic-metadata + filter-state phases
// finalize the shape without breaking the interface contract.
type evalContext interface {
	// Header returns the request header value for name + a presence flag.
	// Empty values with present=true are distinct from absent (present=false).
	// Consumed by permHeader + prinHeader.
	Header(name string) (value string, present bool)

	// URLPath returns the request URI path (the `:path` H2 pseudo-header).
	// Consumed by permURLPath + prinURLPath.
	URLPath() string

	// Method returns the request HTTP method (`:method` H2 pseudo-header).
	// Surfaced for matcherCtxAdapter.Method() bridging to internal/matcher's
	// MatchContext per ADR-0142 §Decision (iii). Not consumed by the Permission/
	// Principal Large 11+11 evaluators directly.
	Method() string

	// DestinationIP returns the listener-bound (local) IP for the downstream
	// connection. nil for non-IP transports. Consumed by permDestIP.
	DestinationIP() net.IP

	// DestinationPort returns the listener-bound (local) port. 0 for non-TCP
	// or when not available. Consumed by permDestPort + permDestPortRange.
	DestinationPort() uint32

	// RequestedServerName returns the TLS SNI value. Empty string for
	// plaintext connections or when SNI was not provided. Consumed by permSNI.
	RequestedServerName() string

	// DirectRemoteIP returns the peer connection's source IP (pre-XFF).
	// Consumed by prinDirectRemoteIP per §11.P18.
	DirectRemoteIP() net.IP

	// RemoteIP returns the XFF-resolved remote IP per §11.P18. When the
	// framework does not yet expose a callable XFF resolver to filters
	// (phase-16 MVP), the production *filter returns the peer addr verbatim;
	// the distinction with DirectRemoteIP is interface-level forward-compat.
	// Consumed by prinRemoteIP.
	RemoteIP() net.IP

	// DownstreamPrincipal returns the TLS principal candidates in priority
	// order: URI SANs first, DNS SANs second, Subject DN Common Name third
	// per §1.1 amendment 12 + ADR-0144. Returns nil/empty for plaintext or
	// non-mTLS connections. Consumed by prinAuthenticated.
	//
	// The production-side accessor on `DecoderFilterCallbacks` lands at
	// Task 6 per ADR-0144; at Task 5 the *filter STUBs return nil pending
	// the framework-primitive landing.
	DownstreamPrincipal() []string

	// SourcedMetadata is a forward-compat placeholder returning the dynamic-
	// metadata accessor shape for `Principal_SourcedMetadata` consumption.
	// Phase-16 MVP runtime is always-FALSE per §2.5 + §8.10, so the shape
	// is intentionally `any`-typed (returns nil at MVP). A future dynamic-
	// metadata-family phase finalizes the shape without breaking callers.
	SourcedMetadata() any

	// FilterState is a forward-compat placeholder returning the filter-state
	// accessor shape for `Principal_FilterState` consumption. Phase-16 MVP
	// runtime is always-FALSE per §2.5 + §8.10; future filter-state-family
	// phase finalizes the shape.
	FilterState() any
}

// ----------------------------------------------------------------------------
// Permission Large 11 evaluator implementations per SPEC §6.5 + ADR-0143.
//
// Permission Large 11 catalog:
//   - permAny             — matches anything when val=true (PGV const=true).
//   - permHeader          — routev3.HeaderMatcher dispatch.
//   - permURLPath         — matcherv3.PathMatcher dispatch.
//   - permDestIP          — corev3.CidrRange match.
//   - permDestPort        — uint32 exact match.
//   - permDestPortRange   — typev3.Int32Range [start, end) half-open.
//   - permSNI             — matcherv3.StringMatcher match against SNI.
//   - permAnd             — AND-semantic short-circuit recursive combinator.
//   - permOr              — OR-semantic short-circuit recursive combinator.
//   - permNot             — logical negate recursive combinator.
//   - permSourcedMetadata — parse-supported; ALWAYS FALSE at runtime per §2.5.
//
// PARSE-REJECT-only (no evaluator type) — Permission_Metadata +
// Permission_Matcher + Permission_UriTemplate handled in buildOnePermission.
// ----------------------------------------------------------------------------

// permAny matches every request when val=true. PGV `const=true` is enforced
// at parse time by buildOnePermission rejecting val=false. Per SPEC §6.5.
type permAny struct{ val bool }

func (e *permAny) evaluatePermission(_ evalContext) bool { return e.val }

// permHeader wraps a parsed HeaderMatcher. Per SPEC §6.5; uses the
// matchHeader local adapter (see Shared infrastructure adapters below).
type permHeader struct{ matcher *routev3.HeaderMatcher }

func (e *permHeader) evaluatePermission(ctx evalContext) bool {
	return matchHeader(e.matcher, ctx)
}

// permURLPath wraps a parsed PathMatcher. Per SPEC §6.5; uses matchPath.
type permURLPath struct{ matcher *matcherv3.PathMatcher }

func (e *permURLPath) evaluatePermission(ctx evalContext) bool {
	return matchPath(e.matcher, ctx.URLPath())
}

// permDestIP wraps a parsed CidrRange. CIDR-range match against
// ctx.DestinationIP() per SPEC §6.5.
type permDestIP struct{ cidr *corev3.CidrRange }

func (e *permDestIP) evaluatePermission(ctx evalContext) bool {
	return matchCidr(e.cidr, ctx.DestinationIP())
}

// permDestPort exact-matches a single uint32 port. Per SPEC §6.5.
type permDestPort struct{ port uint32 }

func (e *permDestPort) evaluatePermission(ctx evalContext) bool {
	return ctx.DestinationPort() == e.port
}

// permDestPortRange implements [start, end) half-open semantics per
// typev3.Int32Range + SPEC §6.5. Note: int32 source widened to int64 for
// the safe comparison; uint32 port is in [0, 65535] so it always fits.
type permDestPortRange struct {
	start int32
	end   int32
}

func (e *permDestPortRange) evaluatePermission(ctx evalContext) bool {
	p := int64(ctx.DestinationPort())
	return p >= int64(e.start) && p < int64(e.end)
}

// permSNI matches a StringMatcher against ctx.RequestedServerName(). Per
// SPEC §6.5.
type permSNI struct{ matcher *matcherv3.StringMatcher }

func (e *permSNI) evaluatePermission(ctx evalContext) bool {
	return matchString(e.matcher, ctx.RequestedServerName())
}

// permAnd is the AND-semantic short-circuit recursive combinator. Returns
// FALSE on the first child returning FALSE; TRUE only if all children TRUE.
// Empty children → TRUE (consistent with conjunction over empty set).
type permAnd struct{ children []permissionEvaluator }

func (e *permAnd) evaluatePermission(ctx evalContext) bool {
	for _, c := range e.children {
		if !c.evaluatePermission(ctx) {
			return false
		}
	}
	return true
}

// permOr is the OR-semantic short-circuit recursive combinator. Returns TRUE
// on the first child returning TRUE; FALSE only if all children FALSE. Empty
// children → FALSE (consistent with disjunction over empty set).
type permOr struct{ children []permissionEvaluator }

func (e *permOr) evaluatePermission(ctx evalContext) bool {
	for _, c := range e.children {
		if c.evaluatePermission(ctx) {
			return true
		}
	}
	return false
}

// permNot is the recursive logical-negate combinator.
type permNot struct{ child permissionEvaluator }

func (e *permNot) evaluatePermission(ctx evalContext) bool {
	return !e.child.evaluatePermission(ctx)
}

// permSourcedMetadata is parse-supported (the SourcedMetadata proto is
// accepted at parse time) but ALWAYS returns FALSE at runtime per SPEC §2.5
// + §8.10. The dynamic-metadata subsystem is not yet wired in envoy-go MVP;
// future dynamic-metadata-family phases will replace this stub with a real
// SourcedMetadata evaluator.
//
//nolint:unused // matcher field captured for future dynamic-metadata wiring.
type permSourcedMetadata struct {
	matcher *rbacconfigv3.SourcedMetadata
}

func (e *permSourcedMetadata) evaluatePermission(_ evalContext) bool {
	// MVP always-no-match per §2.5. Future phase reads e.matcher.
	return false
}

// ----------------------------------------------------------------------------
// buildPermissionEvaluators + buildOnePermission entry points per SPEC §6.5.
// ----------------------------------------------------------------------------

// buildPermissionEvaluators iterates calling buildOnePermission, wrapping
// errors with `permission[%d]:` prefix per SPEC §6.5 + ADR-0143.
func buildPermissionEvaluators(perms []*rbacconfigv3.Permission) ([]permissionEvaluator, error) {
	out := make([]permissionEvaluator, 0, len(perms))
	for i, perm := range perms {
		ev, err := buildOnePermission(perm)
		if err != nil {
			return nil, fmt.Errorf("permission[%d]: %w", i, err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// buildOnePermission is the per-Permission switch over the 14 cases per SPEC
// §6.5 + ADR-0143:
//   - 11 accepted variants (Permission Large 11).
//   - 3 PARSE-REJECT variants (Metadata + Matcher + UriTemplate per §2.3 +
//     §11.P12 + planner-time decisions D3 + D6).
//   - nil-rule defensive PARSE-REJECT.
func buildOnePermission(p *rbacconfigv3.Permission) (permissionEvaluator, error) {
	switch r := p.GetRule().(type) {
	case *rbacconfigv3.Permission_Any:
		// PGV `const=true` mirror per §1.1 amendment 4 + planner-time D7 (defensive
		// envoy-go check; Envoy would PGV-validate at proto-decode boundary).
		if !r.Any {
			return nil, errors.New("rbac: permission.any must be true (PGV const=true mirror)")
		}
		return &permAny{val: r.Any}, nil
	case *rbacconfigv3.Permission_Header:
		if r.Header == nil {
			return nil, errors.New("rbac: permission.header is nil")
		}
		return &permHeader{matcher: r.Header}, nil
	case *rbacconfigv3.Permission_UrlPath:
		if r.UrlPath == nil {
			return nil, errors.New("rbac: permission.url_path is nil")
		}
		return &permURLPath{matcher: r.UrlPath}, nil
	case *rbacconfigv3.Permission_DestinationIp:
		if r.DestinationIp == nil {
			return nil, errors.New("rbac: permission.destination_ip is nil")
		}
		return &permDestIP{cidr: r.DestinationIp}, nil
	case *rbacconfigv3.Permission_DestinationPort:
		// PGV lte=65535 mirror per §1.1 amendment 4. uint32 from proto carries
		// values up to 2^32-1; defensive reject out-of-port-range.
		if r.DestinationPort > 65535 {
			return nil, fmt.Errorf("rbac: permission.destination_port %d exceeds 65535", r.DestinationPort)
		}
		return &permDestPort{port: r.DestinationPort}, nil
	case *rbacconfigv3.Permission_DestinationPortRange:
		if r.DestinationPortRange == nil {
			return nil, errors.New("rbac: permission.destination_port_range is nil")
		}
		return &permDestPortRange{
			start: r.DestinationPortRange.GetStart(),
			end:   r.DestinationPortRange.GetEnd(),
		}, nil
	case *rbacconfigv3.Permission_RequestedServerName:
		if r.RequestedServerName == nil {
			return nil, errors.New("rbac: permission.requested_server_name is nil")
		}
		return &permSNI{matcher: r.RequestedServerName}, nil
	case *rbacconfigv3.Permission_AndRules:
		if r.AndRules == nil {
			return nil, errors.New("rbac: permission.and_rules is nil")
		}
		children, err := buildPermissionEvaluators(r.AndRules.GetRules())
		if err != nil {
			return nil, err
		}
		return &permAnd{children: children}, nil
	case *rbacconfigv3.Permission_OrRules:
		if r.OrRules == nil {
			return nil, errors.New("rbac: permission.or_rules is nil")
		}
		children, err := buildPermissionEvaluators(r.OrRules.GetRules())
		if err != nil {
			return nil, err
		}
		return &permOr{children: children}, nil
	case *rbacconfigv3.Permission_NotRule:
		if r.NotRule == nil {
			return nil, errors.New("rbac: permission.not_rule is nil")
		}
		child, err := buildOnePermission(r.NotRule)
		if err != nil {
			return nil, err
		}
		return &permNot{child: child}, nil
	case *rbacconfigv3.Permission_SourcedMetadata:
		// Parse-supported; ALWAYS FALSE at runtime per §2.5 + §8.10.
		return &permSourcedMetadata{matcher: r.SourcedMetadata}, nil
	case *rbacconfigv3.Permission_Metadata:
		// DEPRECATED upstream per `\x92ǆ\xd8\x04\x033.0\x18\x01` at
		// rbac.pb.go:1534; envoy-go-only PARSE-REJECT per §2.3 + §11.P12 + D3.
		return nil, errors.New("rbac: permission.metadata deprecated; use sourced_metadata")
	case *rbacconfigv3.Permission_Matcher:
		// Extension TypedExtensionConfig per §2.3 + §8.8 + D6.
		return nil, errors.New("rbac: permission.matcher extension types unsupported in this build")
	case *rbacconfigv3.Permission_UriTemplate:
		// Extension TypedExtensionConfig per §2.3 + §8.8 + D6.
		return nil, errors.New("rbac: permission.uri_template extension types unsupported in this build")
	case nil:
		// Defensive: oneof unset.
		return nil, errors.New("rbac: permission rule oneof is unset")
	default:
		return nil, fmt.Errorf("rbac: unknown permission rule type %T", r)
	}
}

// ----------------------------------------------------------------------------
// Principal Large 11 evaluator implementations per SPEC §6.5 + §6.6 +
// §1.1 amendments 7 + 12 + ADR-0143 §Decision (iii) + (vi).
//
// Principal Large 11 catalog:
//   - prinAny             — matches anything when val=true (PGV const=true mirror).
//   - prinAuthenticated   — three-case algorithm per §1.1 amendment 12 + §6.6.
//   - prinDirectRemoteIP  — corev3.CidrRange match against peer source IP.
//   - prinRemoteIP        — corev3.CidrRange match against XFF-resolved IP per §11.P18.
//   - prinHeader          — routev3.HeaderMatcher dispatch (reuses matchHeader).
//   - prinURLPath         — matcherv3.PathMatcher dispatch (reuses matchPath).
//   - prinAnd             — AND-semantic short-circuit recursive combinator.
//   - prinOr              — OR-semantic short-circuit recursive combinator.
//   - prinNot             — logical negate recursive combinator.
//   - prinSourcedMetadata — parse-supported; ALWAYS FALSE at runtime per §2.5.
//   - prinFilterState     — parse-supported; ALWAYS FALSE at runtime per §2.5.
//
// PARSE-REJECT-only (no evaluator type) — Principal_SourceIp + Principal_Metadata
// + Principal_Custom handled in buildOnePrincipal. Per §1.1 amendment 7, the
// Principal_Custom variant is the NEW 14th proto oneof discovered post-
// BRAINSTORM; the v1.32.4 go-control-plane binding does NOT expose it (the
// field landed in Envoy post-v1.32.x; visible in v1.37.2 per rbac.pb.go:1112).
// The PARSE-REJECT disposition for Principal_Custom is encoded in
// buildOnePrincipal's `default:` arm + the explicit case re-activates when
// the module bumps expose the variant; the verbatim error wording stays
// locked at ADR-0143 §Decision (iv).
// ----------------------------------------------------------------------------

// prinAny matches every request when val=true. Mirrors permAny semantics.
// Per ADR-0143 §Decision (iii).
type prinAny struct{ val bool }

func (e *prinAny) evaluatePrincipal(_ evalContext) bool { return e.val }

// prinAuthenticated implements the three-case algorithm per §1.1 amendment 12
// + SPEC §6.6 + ADR-0143 §Decision (vi). The nameMatcher field is the
// (possibly-nil) StringMatcher carried by Principal_Authenticated.principal_name.
//
//   - Case (a): nameMatcher == nil + len(ctx.DownstreamPrincipal()) > 0 → TRUE
//     (match any authenticated user per amendment 12).
//   - Case (b): non-nil StringMatcher iterates over ctx.DownstreamPrincipal()
//     candidates in priority order (URI SAN first, then DNS SAN, then Subject
//     DN Common Name per ADR-0144); TRUE on first match via matchString.
//   - Case (c): len(ctx.DownstreamPrincipal()) == 0 → FALSE (plaintext /
//     no client cert).
type prinAuthenticated struct {
	nameMatcher *matcherv3.StringMatcher // nil for case (a)
}

func (e *prinAuthenticated) evaluatePrincipal(ctx evalContext) bool {
	candidates := ctx.DownstreamPrincipal()
	if len(candidates) == 0 {
		// Case (c): plaintext / no client cert.
		return false
	}
	if e.nameMatcher == nil {
		// Case (a): nil principal_name → match any authenticated user.
		return true
	}
	// Case (b): iterate priority-ordered candidates; TRUE on first match.
	for _, candidate := range candidates {
		if matchString(e.nameMatcher, candidate) {
			return true
		}
	}
	return false
}

// prinDirectRemoteIP CIDR-matches the peer connection source IP per §11.P18.
// Distinct from prinRemoteIP: NO XFF resolution.
type prinDirectRemoteIP struct{ cidr *corev3.CidrRange }

func (e *prinDirectRemoteIP) evaluatePrincipal(ctx evalContext) bool {
	return matchCidr(e.cidr, ctx.DirectRemoteIP())
}

// prinRemoteIP CIDR-matches the XFF-resolved remote IP per §11.P18. The
// production *filter's RemoteIP() accessor uses the framework's XFF resolver
// when one is exposed; phase-16 MVP MAY return the peer addr verbatim when
// the framework primitive is not yet surfaced (documented at the evalContext
// interface comment).
type prinRemoteIP struct{ cidr *corev3.CidrRange }

func (e *prinRemoteIP) evaluatePrincipal(ctx evalContext) bool {
	return matchCidr(e.cidr, ctx.RemoteIP())
}

// prinHeader wraps a routev3.HeaderMatcher; reuses the matchHeader local
// adapter from Task 4 (the matcher type is the same as Permission.Header per
// the proto binding).
type prinHeader struct{ matcher *routev3.HeaderMatcher }

func (e *prinHeader) evaluatePrincipal(ctx evalContext) bool {
	return matchHeader(e.matcher, ctx)
}

// prinURLPath wraps a matcherv3.PathMatcher; reuses matchPath from Task 4.
type prinURLPath struct{ matcher *matcherv3.PathMatcher }

func (e *prinURLPath) evaluatePrincipal(ctx evalContext) bool {
	return matchPath(e.matcher, ctx.URLPath())
}

// prinAnd is the AND-semantic short-circuit recursive combinator. Mirrors
// permAnd semantics: FALSE on first child returning FALSE; empty children →
// TRUE (conjunction-over-empty-set).
type prinAnd struct{ children []principalEvaluator }

func (e *prinAnd) evaluatePrincipal(ctx evalContext) bool {
	for _, c := range e.children {
		if !c.evaluatePrincipal(ctx) {
			return false
		}
	}
	return true
}

// prinOr is the OR-semantic short-circuit recursive combinator. Mirrors permOr
// semantics: TRUE on first child returning TRUE; empty children → FALSE
// (disjunction-over-empty-set).
type prinOr struct{ children []principalEvaluator }

func (e *prinOr) evaluatePrincipal(ctx evalContext) bool {
	for _, c := range e.children {
		if c.evaluatePrincipal(ctx) {
			return true
		}
	}
	return false
}

// prinNot is the recursive logical-negate combinator.
type prinNot struct{ child principalEvaluator }

func (e *prinNot) evaluatePrincipal(ctx evalContext) bool {
	return !e.child.evaluatePrincipal(ctx)
}

// prinSourcedMetadata is parse-supported (the SourcedMetadata proto is accepted
// at parse time) but ALWAYS returns FALSE at runtime per SPEC §2.5 + §8.10.
// Mirrors permSourcedMetadata.
//
//nolint:unused // matcher field captured for future dynamic-metadata wiring.
type prinSourcedMetadata struct {
	matcher *rbacconfigv3.SourcedMetadata
}

func (e *prinSourcedMetadata) evaluatePrincipal(_ evalContext) bool {
	// MVP always-no-match per §2.5. Future phase reads e.matcher.
	return false
}

// prinFilterState is parse-supported (the FilterStateMatcher proto is accepted
// at parse time) but ALWAYS returns FALSE at runtime per SPEC §2.5 + §8.10.
// The filter-state subsystem is not yet wired in envoy-go MVP.
//
//nolint:unused // matcher field captured for future filter-state wiring.
type prinFilterState struct {
	matcher *matcherv3.FilterStateMatcher
}

func (e *prinFilterState) evaluatePrincipal(_ evalContext) bool {
	// MVP always-no-match per §2.5. Future phase reads e.matcher.
	return false
}

// ----------------------------------------------------------------------------
// buildPrincipalEvaluators + buildOnePrincipal entry points per SPEC §6.5.
// ----------------------------------------------------------------------------

// buildPrincipalEvaluators iterates calling buildOnePrincipal, wrapping errors
// with `principal[%d]:` prefix per SPEC §6.5 + ADR-0143 §Decision (iii). Real
// implementation lands at Task 5 (replacing the Task-2 STUB per
// ADR-0143 §Consequences).
func buildPrincipalEvaluators(prins []*rbacconfigv3.Principal) ([]principalEvaluator, error) {
	out := make([]principalEvaluator, 0, len(prins))
	for i, prin := range prins {
		ev, err := buildOnePrincipal(prin)
		if err != nil {
			return nil, fmt.Errorf("principal[%d]: %w", i, err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// buildOnePrincipal is the per-Principal switch over 14 cases per SPEC §6.5 +
// ADR-0143 §Decision (iii) + (iv):
//   - 11 accepted variants (Principal Large 11).
//   - 3 PARSE-REJECT variants: Principal_SourceIp + Principal_Metadata +
//     Principal_Custom per §2.4 + §11.P12 + planner-time decisions D4 + D5.
//   - nil-identifier defensive PARSE-REJECT.
//
// Principal_Custom (the NEW 14th variant per §1.1 amendment 7) is structurally
// absent from the go-control-plane v1.32.4 proto binding; the PARSE-REJECT
// disposition is encoded in the `default:` arm via the type-name-string check
// (the future v1.37.2-bump-time activation lifts the check to a typed case).
// The verbatim error wording is locked at ADR-0143 §Decision (iv).
func buildOnePrincipal(p *rbacconfigv3.Principal) (principalEvaluator, error) {
	switch id := p.GetIdentifier().(type) {
	case *rbacconfigv3.Principal_Any:
		// PGV const=true mirror per §1.1 amendment 4 + planner-time discipline.
		// Mirrors permAny's defensive reject on val=false.
		if !id.Any {
			return nil, errors.New("rbac: principal.any must be true (PGV const=true mirror)")
		}
		return &prinAny{val: id.Any}, nil
	case *rbacconfigv3.Principal_Authenticated_:
		// nameMatcher MAY be nil — case (a) of the three-case algorithm.
		var nm *matcherv3.StringMatcher
		if id.Authenticated != nil {
			nm = id.Authenticated.GetPrincipalName()
		}
		return &prinAuthenticated{nameMatcher: nm}, nil
	case *rbacconfigv3.Principal_DirectRemoteIp:
		if id.DirectRemoteIp == nil {
			return nil, errors.New("rbac: principal.direct_remote_ip is nil")
		}
		return &prinDirectRemoteIP{cidr: id.DirectRemoteIp}, nil
	case *rbacconfigv3.Principal_RemoteIp:
		if id.RemoteIp == nil {
			return nil, errors.New("rbac: principal.remote_ip is nil")
		}
		return &prinRemoteIP{cidr: id.RemoteIp}, nil
	case *rbacconfigv3.Principal_Header:
		if id.Header == nil {
			return nil, errors.New("rbac: principal.header is nil")
		}
		return &prinHeader{matcher: id.Header}, nil
	case *rbacconfigv3.Principal_UrlPath:
		if id.UrlPath == nil {
			return nil, errors.New("rbac: principal.url_path is nil")
		}
		return &prinURLPath{matcher: id.UrlPath}, nil
	case *rbacconfigv3.Principal_AndIds:
		if id.AndIds == nil {
			return nil, errors.New("rbac: principal.and_ids is nil")
		}
		children, err := buildPrincipalEvaluators(id.AndIds.GetIds())
		if err != nil {
			return nil, err
		}
		return &prinAnd{children: children}, nil
	case *rbacconfigv3.Principal_OrIds:
		if id.OrIds == nil {
			return nil, errors.New("rbac: principal.or_ids is nil")
		}
		children, err := buildPrincipalEvaluators(id.OrIds.GetIds())
		if err != nil {
			return nil, err
		}
		return &prinOr{children: children}, nil
	case *rbacconfigv3.Principal_NotId:
		if id.NotId == nil {
			return nil, errors.New("rbac: principal.not_id is nil")
		}
		child, err := buildOnePrincipal(id.NotId)
		if err != nil {
			return nil, err
		}
		return &prinNot{child: child}, nil
	case *rbacconfigv3.Principal_SourcedMetadata:
		// Parse-supported; ALWAYS FALSE at runtime per §2.5 + §8.10.
		return &prinSourcedMetadata{matcher: id.SourcedMetadata}, nil
	case *rbacconfigv3.Principal_FilterState:
		// Parse-supported; ALWAYS FALSE at runtime per §2.5 + §8.10.
		return &prinFilterState{matcher: id.FilterState}, nil
	case *rbacconfigv3.Principal_SourceIp:
		// DEPRECATED upstream per `rbac.pb.go` annotation; envoy-go-only
		// PARSE-REJECT per §2.4 + §11.P12 + planner-time D4.
		return nil, errors.New("rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip")
	case *rbacconfigv3.Principal_Metadata:
		// DEPRECATED upstream; envoy-go-only PARSE-REJECT per §2.4 + §11.P12 + D4.
		return nil, errors.New("rbac: principal.metadata deprecated; use sourced_metadata")
	case nil:
		// Defensive: oneof unset.
		return nil, errors.New("rbac: principal identifier oneof is unset")
	default:
		// Per §1.1 amendment 7 NEW: Principal_Custom (the 14th variant; field
		// 12 per rbac.pb.go:1112 in v1.37.2) is structurally absent from the
		// v1.32.4 proto binding. The default arm catches both (a) unknown
		// future variants AND (b) the v1.37.2-bump-time Principal_Custom
		// presentation. The verbatim error wording differs between the two
		// cases — use Go-level type-name introspection to pick the canonical
		// `principal.custom` wording when applicable, falling back to the
		// generic-unknown wording otherwise.
		typeName := fmt.Sprintf("%T", id)
		if strings.HasSuffix(typeName, ".Principal_Custom") {
			return nil, errors.New("rbac: principal.custom extension types unsupported in this build")
		}
		return nil, fmt.Errorf("rbac: unknown principal identifier type %T", id)
	}
}

// ----------------------------------------------------------------------------
// Shared infrastructure adapters — local impl in evaluator.go per ADR-0143
// + PLAN.md line 65. Cors precedent has NO extractable matcher helpers
// (verified at Task 3 spec review per REVIEW.md M-5 forward-pointer);
// internal/matcher's StringMatcher impl uses the cncf/xds variant
// (matchv3.StringMatcher) NOT the envoy variant (matcherv3.StringMatcher),
// so reuse is not type-compatible. Local impl mirrors the canonical
// StringMatcher / HeaderMatcher / PathMatcher / CidrRange semantics.
//
// **TECH-DEBT** — future operator-ergonomics phase MAY extract these adapters
// to a top-level `internal/stringmatcher/` (or analogous) package for
// cross-filter reuse. Noted at PROGRESS.md Task 4 entry per REVIEW.md
// forward-pointer convention.
// ----------------------------------------------------------------------------

// matchString evaluates a matcherv3.StringMatcher against a candidate string.
// Subset honored: Exact / Prefix / Suffix / Contains / SafeRegex with
// `ignore_case` for non-regex variants. Custom + nil pattern → false (no
// PARSE-REJECT here; the build-side helpers reject earlier if applicable).
func matchString(sm *matcherv3.StringMatcher, candidate string) bool {
	if sm == nil {
		return false
	}
	ic := sm.GetIgnoreCase()
	switch mp := sm.GetMatchPattern().(type) {
	case *matcherv3.StringMatcher_Exact:
		if ic {
			return strings.EqualFold(mp.Exact, candidate)
		}
		return mp.Exact == candidate
	case *matcherv3.StringMatcher_Prefix:
		if ic {
			return len(candidate) >= len(mp.Prefix) && strings.EqualFold(candidate[:len(mp.Prefix)], mp.Prefix)
		}
		return strings.HasPrefix(candidate, mp.Prefix)
	case *matcherv3.StringMatcher_Suffix:
		if ic {
			if len(candidate) < len(mp.Suffix) {
				return false
			}
			return strings.EqualFold(candidate[len(candidate)-len(mp.Suffix):], mp.Suffix)
		}
		return strings.HasSuffix(candidate, mp.Suffix)
	case *matcherv3.StringMatcher_Contains:
		if ic {
			return strings.Contains(strings.ToLower(candidate), strings.ToLower(mp.Contains))
		}
		return strings.Contains(candidate, mp.Contains)
	case *matcherv3.StringMatcher_SafeRegex:
		if mp.SafeRegex == nil {
			return false
		}
		// Re-compile per call: parse-time helpers (Task 5/7) may pre-cache;
		// at Task 4 the runtime cost is acceptable for the canonical subset.
		// TECH-DEBT: pre-compile at buildOnePermission time per future
		// optimization phase.
		re, err := regexp.Compile(mp.SafeRegex.GetRegex())
		if err != nil {
			return false
		}
		return re.MatchString(candidate)
	case *matcherv3.StringMatcher_Custom, nil:
		return false
	default:
		return false
	}
}

// matchPath evaluates a matcherv3.PathMatcher against a request URL path.
// PathMatcher carries a single inner StringMatcher (`Path` field per
// matcherv3.PathMatcher_Path). nil PathMatcher → false; nil inner Path →
// false.
func matchPath(pm *matcherv3.PathMatcher, candidate string) bool {
	if pm == nil {
		return false
	}
	rule, ok := pm.GetRule().(*matcherv3.PathMatcher_Path)
	if !ok || rule.Path == nil {
		return false
	}
	return matchString(rule.Path, candidate)
}

// matchHeader evaluates a routev3.HeaderMatcher against the per-stream header
// state exposed via evalContext.Header(name).
//
// HeaderMatcher subset honored at Task 4:
//   - PresentMatch     — match when header present (treat_missing_header_as_empty respected).
//   - ExactMatch       — DEPRECATED upstream but still in proto; exact-equal.
//   - PrefixMatch      — DEPRECATED upstream; HasPrefix.
//   - SuffixMatch      — DEPRECATED upstream; HasSuffix.
//   - ContainsMatch    — DEPRECATED upstream; Contains.
//   - SafeRegexMatch   — DEPRECATED upstream; regexp.MatchString.
//   - StringMatch      — canonical Envoy-recommended replacement; delegates
//     to matchString on the inner StringMatcher.
//   - RangeMatch       — Int64Range over numeric header value.
//
// InvertMatch is honored last (final XOR).
//
// HeaderMatcher_StringMatch is the canonical surface per the upstream
// recommendation; tests favor it. The 5 deprecated specifiers (Exact /
// Prefix / Suffix / Contains / SafeRegex) are retained for proto-faithfulness
// — operators may still configure them, and silently dropping them would be
// surprising.
//
//nolint:staticcheck // proto-faithful support for deprecated HeaderMatchSpecifier variants per ADR-0143 §Decision (vii) — operators may still configure them
func matchHeader(hm *routev3.HeaderMatcher, ctx evalContext) bool {
	if hm == nil {
		return false
	}
	value, present := ctx.Header(hm.GetName())
	if !present {
		if hm.GetTreatMissingHeaderAsEmpty() {
			value = ""
			present = true
		}
	}

	var matched bool
	switch spec := hm.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_PresentMatch:
		matched = present == spec.PresentMatch
	case *routev3.HeaderMatcher_StringMatch:
		if !present {
			matched = false
		} else {
			matched = matchString(spec.StringMatch, value)
		}
	case *routev3.HeaderMatcher_ExactMatch:
		matched = present && value == spec.ExactMatch
	case *routev3.HeaderMatcher_PrefixMatch:
		matched = present && strings.HasPrefix(value, spec.PrefixMatch)
	case *routev3.HeaderMatcher_SuffixMatch:
		matched = present && strings.HasSuffix(value, spec.SuffixMatch)
	case *routev3.HeaderMatcher_ContainsMatch:
		matched = present && strings.Contains(value, spec.ContainsMatch)
	case *routev3.HeaderMatcher_SafeRegexMatch:
		if !present || spec.SafeRegexMatch == nil {
			matched = false
		} else {
			re, err := regexp.Compile(spec.SafeRegexMatch.GetRegex())
			matched = err == nil && re.MatchString(value)
		}
	case *routev3.HeaderMatcher_RangeMatch:
		if !present || spec.RangeMatch == nil {
			matched = false
		} else {
			// Parse value as a strict signed int64 — Envoy uses absl::SimpleAtoi
			// which rejects leading/trailing whitespace and trailing garbage.
			// strconv.ParseInt matches that semantic.
			n, err := strconv.ParseInt(value, 10, 64)
			matched = err == nil && n >= spec.RangeMatch.GetStart() && n < spec.RangeMatch.GetEnd()
		}
	default:
		matched = false
	}
	if hm.GetInvertMatch() {
		matched = !matched
	}
	return matched
}

// matchCidr evaluates a corev3.CidrRange against an IP. PrefixLen defaults to
// 0 when unset (wrapperspb.UInt32Value nil) — meaning the prefix matches all
// addresses (the canonical Envoy semantic). nil ip → false.
func matchCidr(cidr *corev3.CidrRange, ip net.IP) bool {
	if cidr == nil || ip == nil {
		return false
	}
	prefixIP := net.ParseIP(cidr.GetAddressPrefix())
	if prefixIP == nil {
		return false
	}
	var prefixLen int
	if pl := cidr.GetPrefixLen(); pl != nil {
		prefixLen = int(pl.GetValue())
	}
	// Bit-width depends on the address family. ParseIP returns a 16-byte slice
	// for both v4 and v6 — but for v4 the IsIPv4 fast-path is `.To4() != nil`.
	var bits int
	if prefixIP.To4() != nil {
		bits = 32
	} else {
		bits = 128
	}
	if prefixLen > bits {
		return false
	}
	_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", cidr.GetAddressPrefix(), prefixLen))
	if err != nil {
		return false
	}
	return ipNet.Contains(ip)
}
