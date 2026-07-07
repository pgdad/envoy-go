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
// implementations cover Permission Large 11 + AND/OR/NOT combinators + 3
// PARSE-REJECT cases per ADR-0143.
//
// evaluatePermission returns true iff the permission matches the request.
// ctx provides per-stream accessors (headers / path / IP / destination port /
// SNI / sourced-metadata) per SPEC §6.2.
type permissionEvaluator interface {
	evaluatePermission(ctx EvalContext) bool
}

// principalEvaluator is the per-Principal evaluator interface. Concrete
// implementations cover Principal Large 11 + prinAuthenticated three-case +
// 3 PARSE-REJECT cases per ADR-0143.
//
// evaluatePrincipal returns true iff the principal matches the request. ctx
// provides per-stream accessors (TLS principal candidates / headers / IP /
// sourced-metadata / filter-state).
type principalEvaluator interface {
	evaluatePrincipal(ctx EvalContext) bool
}

// EvalContext is the per-request/connection accessor abstraction the evaluators
// consume. It is implemented by BOTH consumers of this shared engine: the HTTP
// consumer (the *filter in internal/filter/http/rbac, sourcing per-stream facts
// from DecoderFilterCallbacks + the request headers) AND the L4 consumer (the
// l4EvalContext in internal/filter/network/rbac, sourcing connection facts from
// a network.Connection). The full surface (Header / URLPath / Method /
// DirectRemoteIP / RemoteIP / DestinationIP / DestinationPort /
// RequestedServerName / DownstreamPrincipal / SourcedMetadata / FilterState)
// per SPEC §6.2; the SOURCE of each production accessor is consumer-specific.
//
// The interface covers Permission-relevant accessors per SPEC §6.5 + ADR-0143
// §Decision (Permission section), Principal-side accessors (DirectRemoteIP /
// RemoteIP / DownstreamPrincipal / SourcedMetadata / FilterState) per
// ADR-0143 §Decision (iii), and Method() to feed the matcherCtxAdapter's
// MatchContext.Method() bridge (the matcher framework primitive ships a Method
// accessor per ADR-0142 §Decision (iii) initial-surface). None of the
// Permission/Principal Large 11+11 evaluators consume Method directly; it
// surfaces only through the matcher-engine bridge.
//
// SourcedMetadata() + FilterState() accessor shapes are FORWARD-COMPAT-ONLY
// placeholders (both runtimes are always-FALSE per §2.5 + §8.10). The
// `any`-typed returns let future dynamic-metadata + filter-state phases
// finalize the shape without breaking the interface contract.
type EvalContext interface {
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
	// consumer does not expose a callable XFF resolver, the production accessor
	// returns the peer addr verbatim (the HTTP consumer at MVP; the L4 consumer
	// always, since a raw connection carries no XFF layer); the distinction with
	// DirectRemoteIP is then interface-level forward-compat. Consumed by
	// prinRemoteIP.
	RemoteIP() net.IP

	// DownstreamPrincipal returns the TLS principal candidates in priority
	// order: URI SANs first, DNS SANs second, Subject DN Common Name third
	// per §1.1 amendment 12 + ADR-0144. Returns nil/empty for plaintext or
	// non-mTLS connections. Consumed by prinAuthenticated.
	//
	// The production-side source is consumer-specific: the HTTP consumer reads
	// it from DecoderFilterCallbacks (the ADR-0144 framework primitive), the L4
	// consumer reads it from network.Connection.DownstreamPrincipals().
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

func (e *permAny) evaluatePermission(_ EvalContext) bool { return e.val }

// permHeader wraps a build-time-compiled HeaderMatcher. Per SPEC §6.5; uses
// the compiledHeaderMatcher local adapter (see Shared infrastructure adapters
// below).
type permHeader struct{ matcher compiledHeaderMatcher }

func (e *permHeader) evaluatePermission(ctx EvalContext) bool {
	return e.matcher.matches(ctx)
}

// permURLPath wraps a parsed PathMatcher lowered onto its inner compiled
// StringMatcher via compilePathMatcher. Per SPEC §6.5.
type permURLPath struct{ matcher compiledStringMatcher }

func (e *permURLPath) evaluatePermission(ctx EvalContext) bool {
	return e.matcher.matches(ctx.URLPath())
}

// permDestIP wraps a build-time-compiled CidrRange. CIDR-range match against
// ctx.DestinationIP() per SPEC §6.5.
type permDestIP struct{ cidr compiledCidr }

func (e *permDestIP) evaluatePermission(ctx EvalContext) bool {
	return e.cidr.matches(ctx.DestinationIP())
}

// permDestPort exact-matches a single uint32 port. Per SPEC §6.5.
type permDestPort struct{ port uint32 }

func (e *permDestPort) evaluatePermission(ctx EvalContext) bool {
	return ctx.DestinationPort() == e.port
}

// permDestPortRange implements [start, end) half-open semantics per
// typev3.Int32Range + SPEC §6.5. Note: int32 source widened to int64 for
// the safe comparison; uint32 port is in [0, 65535] so it always fits.
type permDestPortRange struct {
	start int32
	end   int32
}

func (e *permDestPortRange) evaluatePermission(ctx EvalContext) bool {
	p := int64(ctx.DestinationPort())
	return p >= int64(e.start) && p < int64(e.end)
}

// permSNI matches a build-time-compiled StringMatcher against
// ctx.RequestedServerName(). Per SPEC §6.5.
type permSNI struct{ matcher compiledStringMatcher }

func (e *permSNI) evaluatePermission(ctx EvalContext) bool {
	return e.matcher.matches(ctx.RequestedServerName())
}

// permAnd is the AND-semantic short-circuit recursive combinator. Returns
// FALSE on the first child returning FALSE; TRUE only if all children TRUE.
// Empty children → TRUE (consistent with conjunction over empty set).
type permAnd struct{ children []permissionEvaluator }

func (e *permAnd) evaluatePermission(ctx EvalContext) bool {
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

func (e *permOr) evaluatePermission(ctx EvalContext) bool {
	for _, c := range e.children {
		if c.evaluatePermission(ctx) {
			return true
		}
	}
	return false
}

// permNot is the recursive logical-negate combinator.
type permNot struct{ child permissionEvaluator }

func (e *permNot) evaluatePermission(ctx EvalContext) bool {
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

func (e *permSourcedMetadata) evaluatePermission(_ EvalContext) bool {
	// MVP always-no-match per §2.5. Future phase reads e.matcher.
	return false
}

// ----------------------------------------------------------------------------
// buildPermissionEvaluators + buildOnePermission entry points per SPEC §6.5.
// ----------------------------------------------------------------------------

// buildPermissionEvaluators iterates calling buildOnePermission, wrapping
// errors with `permission[%d]:` prefix per SPEC §6.5 + ADR-0143.
func buildPermissionEvaluators(perms []*rbacconfigv3.Permission, profile Profile) ([]permissionEvaluator, error) {
	out := make([]permissionEvaluator, 0, len(perms))
	for i, perm := range perms {
		ev, err := buildOnePermission(perm, profile)
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
//
// ProfileL4 gates the HTTP-only arms (header + url_path) at entry per
// D-26.3-1: an L4 consumer PARSE-REJECTs these arms at compile time.
// ProfileHTTP permits all arms (byte-identical to pre-Profile behavior, R4).
func buildOnePermission(p *rbacconfigv3.Permission, profile Profile) (permissionEvaluator, error) {
	switch r := p.GetRule().(type) {
	case *rbacconfigv3.Permission_Any:
		// PGV `const=true` mirror per §1.1 amendment 4 + planner-time D7 (defensive
		// envoy-go check; Envoy would PGV-validate at proto-decode boundary).
		if !r.Any {
			return nil, errors.New("rbac: permission.any must be true (PGV const=true mirror)")
		}
		return &permAny{val: r.Any}, nil
	case *rbacconfigv3.Permission_Header:
		if !profile.permits(armHeader) {
			return nil, errors.New("rbac: permission.header is HTTP-only (unsupported for L4 network RBAC)")
		}
		if r.Header == nil {
			return nil, errors.New("rbac: permission.header is nil")
		}
		return &permHeader{matcher: compileHeaderMatcher(r.Header)}, nil
	case *rbacconfigv3.Permission_UrlPath:
		if !profile.permits(armURLPath) {
			return nil, errors.New("rbac: permission.url_path is HTTP-only (unsupported for L4 network RBAC)")
		}
		if r.UrlPath == nil {
			return nil, errors.New("rbac: permission.url_path is nil")
		}
		return &permURLPath{matcher: compilePathMatcher(r.UrlPath)}, nil
	case *rbacconfigv3.Permission_DestinationIp:
		if r.DestinationIp == nil {
			return nil, errors.New("rbac: permission.destination_ip is nil")
		}
		return &permDestIP{cidr: compileCidr(r.DestinationIp)}, nil
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
		return &permSNI{matcher: compileStringMatcher(r.RequestedServerName)}, nil
	case *rbacconfigv3.Permission_AndRules:
		if r.AndRules == nil {
			return nil, errors.New("rbac: permission.and_rules is nil")
		}
		children, err := buildPermissionEvaluators(r.AndRules.GetRules(), profile)
		if err != nil {
			return nil, err
		}
		return &permAnd{children: children}, nil
	case *rbacconfigv3.Permission_OrRules:
		if r.OrRules == nil {
			return nil, errors.New("rbac: permission.or_rules is nil")
		}
		children, err := buildPermissionEvaluators(r.OrRules.GetRules(), profile)
		if err != nil {
			return nil, err
		}
		return &permOr{children: children}, nil
	case *rbacconfigv3.Permission_NotRule:
		if r.NotRule == nil {
			return nil, errors.New("rbac: permission.not_rule is nil")
		}
		child, err := buildOnePermission(r.NotRule, profile)
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

func (e *prinAny) evaluatePrincipal(_ EvalContext) bool { return e.val }

// prinAuthenticated implements the three-case algorithm per §1.1 amendment 12
// + SPEC §6.6 + ADR-0143 §Decision (vi). The nameMatcher field is the
// build-time-compiled form of the (possibly-nil) StringMatcher carried by
// Principal_Authenticated.principal_name; a nil pointer records the
// proto-nil case.
//
//   - Case (a): nameMatcher == nil + len(ctx.DownstreamPrincipal()) > 0 → TRUE
//     (match any authenticated user per amendment 12).
//   - Case (b): non-nil compiled StringMatcher iterates over
//     ctx.DownstreamPrincipal() candidates in priority order (URI SAN first,
//     then DNS SAN, then Subject DN Common Name per ADR-0144); TRUE on first
//     match.
//   - Case (c): len(ctx.DownstreamPrincipal()) == 0 → FALSE (plaintext /
//     no client cert).
type prinAuthenticated struct {
	nameMatcher *compiledStringMatcher // nil for case (a)
}

func (e *prinAuthenticated) evaluatePrincipal(ctx EvalContext) bool {
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
		if e.nameMatcher.matches(candidate) {
			return true
		}
	}
	return false
}

// prinDirectRemoteIP CIDR-matches the peer connection source IP per §11.P18.
// Distinct from prinRemoteIP: NO XFF resolution.
type prinDirectRemoteIP struct{ cidr compiledCidr }

func (e *prinDirectRemoteIP) evaluatePrincipal(ctx EvalContext) bool {
	return e.cidr.matches(ctx.DirectRemoteIP())
}

// prinRemoteIP CIDR-matches the XFF-resolved remote IP per §11.P18. The
// production *filter's RemoteIP() accessor uses the framework's XFF resolver
// when one is exposed; phase-16 MVP MAY return the peer addr verbatim when
// the framework primitive is not yet surfaced (documented at the EvalContext
// interface comment).
type prinRemoteIP struct{ cidr compiledCidr }

func (e *prinRemoteIP) evaluatePrincipal(ctx EvalContext) bool {
	return e.cidr.matches(ctx.RemoteIP())
}

// prinHeader wraps a build-time-compiled routev3.HeaderMatcher; reuses the
// compiledHeaderMatcher local adapter (the matcher type is the same as
// Permission.Header per the proto binding).
type prinHeader struct{ matcher compiledHeaderMatcher }

func (e *prinHeader) evaluatePrincipal(ctx EvalContext) bool {
	return e.matcher.matches(ctx)
}

// prinURLPath wraps a matcherv3.PathMatcher lowered via compilePathMatcher;
// mirrors permURLPath.
type prinURLPath struct{ matcher compiledStringMatcher }

func (e *prinURLPath) evaluatePrincipal(ctx EvalContext) bool {
	return e.matcher.matches(ctx.URLPath())
}

// prinAnd is the AND-semantic short-circuit recursive combinator. Mirrors
// permAnd semantics: FALSE on first child returning FALSE; empty children →
// TRUE (conjunction-over-empty-set).
type prinAnd struct{ children []principalEvaluator }

func (e *prinAnd) evaluatePrincipal(ctx EvalContext) bool {
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

func (e *prinOr) evaluatePrincipal(ctx EvalContext) bool {
	for _, c := range e.children {
		if c.evaluatePrincipal(ctx) {
			return true
		}
	}
	return false
}

// prinNot is the recursive logical-negate combinator.
type prinNot struct{ child principalEvaluator }

func (e *prinNot) evaluatePrincipal(ctx EvalContext) bool {
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

func (e *prinSourcedMetadata) evaluatePrincipal(_ EvalContext) bool {
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

func (e *prinFilterState) evaluatePrincipal(_ EvalContext) bool {
	// MVP always-no-match per §2.5. Future phase reads e.matcher.
	return false
}

// ----------------------------------------------------------------------------
// buildPrincipalEvaluators + buildOnePrincipal entry points per SPEC §6.5.
// ----------------------------------------------------------------------------

// buildPrincipalEvaluators iterates calling buildOnePrincipal, wrapping errors
// with `principal[%d]:` prefix per SPEC §6.5 + ADR-0143 §Decision (iii).
func buildPrincipalEvaluators(prins []*rbacconfigv3.Principal, profile Profile) ([]principalEvaluator, error) {
	out := make([]principalEvaluator, 0, len(prins))
	for i, prin := range prins {
		ev, err := buildOnePrincipal(prin, profile)
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
//
// ProfileL4 gates the HTTP-only arms (header + url_path) at entry per
// D-26.3-1: an L4 consumer PARSE-REJECTs these arms at compile time.
// ProfileHTTP permits all arms (byte-identical to pre-Profile behavior, R4).
func buildOnePrincipal(p *rbacconfigv3.Principal, profile Profile) (principalEvaluator, error) {
	switch id := p.GetIdentifier().(type) {
	case *rbacconfigv3.Principal_Any:
		// PGV const=true mirror per §1.1 amendment 4 + planner-time discipline.
		// Mirrors permAny's defensive reject on val=false.
		if !id.Any {
			return nil, errors.New("rbac: principal.any must be true (PGV const=true mirror)")
		}
		return &prinAny{val: id.Any}, nil
	case *rbacconfigv3.Principal_Authenticated_:
		// nameMatcher MAY be nil — case (a) of the three-case algorithm. A
		// non-nil principal_name is compiled at build time (case (b)).
		var nm *compiledStringMatcher
		if id.Authenticated != nil {
			if pn := id.Authenticated.GetPrincipalName(); pn != nil {
				compiled := compileStringMatcher(pn)
				nm = &compiled
			}
		}
		return &prinAuthenticated{nameMatcher: nm}, nil
	case *rbacconfigv3.Principal_DirectRemoteIp:
		if id.DirectRemoteIp == nil {
			return nil, errors.New("rbac: principal.direct_remote_ip is nil")
		}
		return &prinDirectRemoteIP{cidr: compileCidr(id.DirectRemoteIp)}, nil
	case *rbacconfigv3.Principal_RemoteIp:
		if id.RemoteIp == nil {
			return nil, errors.New("rbac: principal.remote_ip is nil")
		}
		return &prinRemoteIP{cidr: compileCidr(id.RemoteIp)}, nil
	case *rbacconfigv3.Principal_Header:
		if !profile.permits(armHeader) {
			return nil, errors.New("rbac: principal.header is HTTP-only (unsupported for L4 network RBAC)")
		}
		if id.Header == nil {
			return nil, errors.New("rbac: principal.header is nil")
		}
		return &prinHeader{matcher: compileHeaderMatcher(id.Header)}, nil
	case *rbacconfigv3.Principal_UrlPath:
		if !profile.permits(armURLPath) {
			return nil, errors.New("rbac: principal.url_path is HTTP-only (unsupported for L4 network RBAC)")
		}
		if id.UrlPath == nil {
			return nil, errors.New("rbac: principal.url_path is nil")
		}
		return &prinURLPath{matcher: compilePathMatcher(id.UrlPath)}, nil
	case *rbacconfigv3.Principal_AndIds:
		if id.AndIds == nil {
			return nil, errors.New("rbac: principal.and_ids is nil")
		}
		children, err := buildPrincipalEvaluators(id.AndIds.GetIds(), profile)
		if err != nil {
			return nil, err
		}
		return &prinAnd{children: children}, nil
	case *rbacconfigv3.Principal_OrIds:
		if id.OrIds == nil {
			return nil, errors.New("rbac: principal.or_ids is nil")
		}
		children, err := buildPrincipalEvaluators(id.OrIds.GetIds(), profile)
		if err != nil {
			return nil, err
		}
		return &prinOr{children: children}, nil
	case *rbacconfigv3.Principal_NotId:
		if id.NotId == nil {
			return nil, errors.New("rbac: principal.not_id is nil")
		}
		child, err := buildOnePrincipal(id.NotId, profile)
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
// + PLAN.md line 65. The cors filter has NO extractable matcher helpers;
// internal/matcher's StringMatcher impl uses the cncf/xds variant
// (matchv3.StringMatcher) NOT the envoy variant (matcherv3.StringMatcher),
// so reuse is not type-compatible. Local impl mirrors the canonical
// StringMatcher / HeaderMatcher / PathMatcher / CidrRange semantics.
//
// **TECH-DEBT** — future operator-ergonomics phase MAY extract these adapters
// to a top-level `internal/stringmatcher/` (or analogous) package for
// cross-filter reuse.
// ----------------------------------------------------------------------------

// compiledStringMatcher pairs a matcherv3.StringMatcher with its pre-compiled
// SafeRegex program. The proto matcher is retained for the non-regex arms
// (their per-call cost is a plain string compare); the regex is compiled ONCE
// at buildOnePermission/buildOnePrincipal time so request evaluation never
// invokes regexp.Compile (closing the former per-call-compile TECH-DEBT).
//
// A nil `re` alongside a SafeRegex arm means "nil SafeRegex OR compile
// failure" — matches() returns false, byte-identical to the historical
// per-call disposition (compile failure → runtime false).
type compiledStringMatcher struct {
	sm *matcherv3.StringMatcher
	re *regexp.Regexp // non-nil iff the SafeRegex arm compiled successfully
}

// compileStringMatcher lowers a StringMatcher at parse time. nil sm is
// tolerated (matches() → false), mirroring the historical nil handling.
func compileStringMatcher(sm *matcherv3.StringMatcher) compiledStringMatcher {
	c := compiledStringMatcher{sm: sm}
	if sm == nil {
		return c
	}
	if mp, ok := sm.GetMatchPattern().(*matcherv3.StringMatcher_SafeRegex); ok && mp.SafeRegex != nil {
		if re, err := regexp.Compile(mp.SafeRegex.GetRegex()); err == nil {
			c.re = re
		}
	}
	return c
}

// matches evaluates the StringMatcher against a candidate string.
// Subset honored: Exact / Prefix / Suffix / Contains / SafeRegex with
// `ignore_case` for non-regex variants. Custom + nil pattern → false (no
// PARSE-REJECT here; the build-side helpers reject earlier if applicable).
func (c compiledStringMatcher) matches(candidate string) bool {
	sm := c.sm
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
		// Pre-compiled at build time; a nil re covers BOTH the nil-SafeRegex
		// and the compile-failure dispositions (runtime false, byte-identical
		// to the former per-call compile).
		return c.re != nil && c.re.MatchString(candidate)
	case *matcherv3.StringMatcher_Custom, nil:
		return false
	default:
		return false
	}
}

// compilePathMatcher lowers a matcherv3.PathMatcher onto its single inner
// StringMatcher (`Path` field per matcherv3.PathMatcher_Path) at parse time.
// The config-shape failure modes (nil PathMatcher / non-Path rule / nil inner
// Path) all collapse onto the zero-value compiledStringMatcher, whose
// matches() returns false — byte-identical to the historical disposition.
func compilePathMatcher(pm *matcherv3.PathMatcher) compiledStringMatcher {
	if pm == nil {
		return compiledStringMatcher{}
	}
	rule, ok := pm.GetRule().(*matcherv3.PathMatcher_Path)
	if !ok || rule.Path == nil {
		return compiledStringMatcher{}
	}
	return compileStringMatcher(rule.Path)
}

// compiledHeaderMatcher pairs a routev3.HeaderMatcher with the pre-compiled
// forms of its regex-bearing arms: the canonical StringMatch arm's inner
// StringMatcher and the deprecated SafeRegexMatch arm's regex program. All
// other arms evaluate off the retained proto per call (plain compares).
//
//nolint:staticcheck // proto-faithful support for deprecated HeaderMatchSpecifier variants per ADR-0143 §Decision (vii) — operators may still configure them
type compiledHeaderMatcher struct {
	hm          *routev3.HeaderMatcher
	stringMatch compiledStringMatcher // compiled StringMatch arm (zero-value for other arms)
	re          *regexp.Regexp        // compiled SafeRegexMatch arm; nil = absent-or-failed
}

// compileHeaderMatcher lowers a HeaderMatcher at parse time, pre-compiling
// the StringMatch + SafeRegexMatch arms. nil hm is tolerated (matches() →
// false). A SafeRegexMatch that is nil or fails to compile leaves re nil —
// runtime false, byte-identical to the historical per-call disposition.
//
//nolint:staticcheck // proto-faithful support for deprecated HeaderMatchSpecifier variants per ADR-0143 §Decision (vii) — operators may still configure them
func compileHeaderMatcher(hm *routev3.HeaderMatcher) compiledHeaderMatcher {
	c := compiledHeaderMatcher{hm: hm}
	if hm == nil {
		return c
	}
	switch spec := hm.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_StringMatch:
		c.stringMatch = compileStringMatcher(spec.StringMatch)
	case *routev3.HeaderMatcher_SafeRegexMatch:
		if spec.SafeRegexMatch != nil {
			if re, err := regexp.Compile(spec.SafeRegexMatch.GetRegex()); err == nil {
				c.re = re
			}
		}
	}
	return c
}

// matches evaluates the routev3.HeaderMatcher against the per-stream header
// state exposed via EvalContext.Header(name).
//
// HeaderMatcher subset honored:
//   - PresentMatch     — match when header present (treat_missing_header_as_empty respected).
//   - ExactMatch       — DEPRECATED upstream but still in proto; exact-equal.
//   - PrefixMatch      — DEPRECATED upstream; HasPrefix.
//   - SuffixMatch      — DEPRECATED upstream; HasSuffix.
//   - ContainsMatch    — DEPRECATED upstream; Contains.
//   - SafeRegexMatch   — DEPRECATED upstream; pre-compiled regexp MatchString.
//   - StringMatch      — canonical Envoy-recommended replacement; delegates
//     to the compiled inner StringMatcher.
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
func (c compiledHeaderMatcher) matches(ctx EvalContext) bool {
	hm := c.hm
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
			matched = c.stringMatch.matches(value)
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
		// Pre-compiled at build time; a nil re covers BOTH the nil-
		// SafeRegexMatch and the compile-failure dispositions (runtime false,
		// byte-identical to the former per-call compile).
		matched = present && c.re != nil && c.re.MatchString(value)
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

// compiledCidr is the parse-time-lowered form of a corev3.CidrRange: the
// *net.IPNet is computed once in buildOnePermission/buildOnePrincipal instead
// of per evaluation (the former per-call fmt.Sprintf + ParseIP + ParseCIDR
// cost). PrefixLen defaults to 0 when unset (wrapperspb.UInt32Value nil) —
// meaning the prefix matches all addresses (the canonical Envoy semantic).
//
// A nil ipNet means "nil CidrRange OR unparseable address prefix OR prefix
// length exceeding the address family's bit-width" — matches() returns
// false, byte-identical to the historical per-call parse-failure disposition.
type compiledCidr struct {
	ipNet *net.IPNet
}

// compileCidr lowers a CidrRange at parse time per the compiledCidr contract.
func compileCidr(cidr *corev3.CidrRange) compiledCidr {
	if cidr == nil {
		return compiledCidr{}
	}
	prefixIP := net.ParseIP(cidr.GetAddressPrefix())
	if prefixIP == nil {
		return compiledCidr{}
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
		return compiledCidr{}
	}
	_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", cidr.GetAddressPrefix(), prefixLen))
	if err != nil {
		return compiledCidr{}
	}
	return compiledCidr{ipNet: ipNet}
}

// matches reports whether ip falls inside the compiled range. nil ip → false.
func (c compiledCidr) matches(ip net.IP) bool {
	if c.ipNet == nil || ip == nil {
		return false
	}
	return c.ipNet.Contains(ip)
}
