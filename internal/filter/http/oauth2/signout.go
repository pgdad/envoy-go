package oauth2

// signout.go — category (c) 302 sign-out emitter per phase-20 SPEC §4.1 +
// §4.5 + §6.9 + ADR-0184.
//
// # Surface
//
// This file owns the sign-out flow's terminal-emission body invoked from
// the dispatcher's priority-1 step (`decode_headers.go::DecodeHeaders`
// step 1 per SPEC §6.3). On `signoutPath` hit, the dispatcher calls
// `*filter.handleSignout(headers)`; this file's body emits:
//
//   - Status: 302 (category (c) per SPEC §4.1)
//   - Body:   empty
//   - Headers:
//   - Location: per `denyRedirectMatcher` integration (matched URL when
//     a (HeaderMatcher, redirect_uri) pair hits), else `cc.redirectURI`
//     verbatim, else empty (browser-default per ADR-0184 §Context)
//   - Set-Cookie: Max-Age=0 for ALL 5 envelope cookies (BearerToken /
//     OauthHMAC / OauthExpires / IdToken / RefreshToken) per SPEC §4.5
//     category (c) — full envelope clearing
//
// # Counter discipline per AMEND-4 + S5 + ADR-0181
//
// NO counter increments on the sign-out path. The 6-counter wire-exact
// upstream roster (ADR-0181 + AMEND-4 — `oauth_unauthorized_rq` /
// `oauth_failure` / `oauth_passthrough` / `oauth_success` /
// `oauth_refreshtoken_success` / `oauth_refreshtoken_failure`) does NOT
// include a `signout_completed` counter per §20.P8 REFUTED + §20.P11
// RATIFIED-AS-ABSENT. The sign-out-completed event IS the 302 emission:
// operator observability arrives via downstream access-logs recording the
// 302 emission with the signout_path-match URL; no per-filter counter
// needed. Phase 20 mirrors upstream wire-compat fully.
//
// # Location-header construction per ADR-0184 + SPEC §4.4 category (c)
//
// The Location header value cascades through three tiers (highest-
// priority first):
//
//  1. `denyRedirectMatcher(headerView)` returns `(url, true)` — the
//     matched URL becomes the Location. This is the operator-intended
//     post-sign-out landing per ADR-0184 §Context (the operator
//     configures one or more `(HeaderMatcher, redirect_uri)` pairs to
//     route sign-out requests to a per-tenant / per-locale landing page).
//  2. `cc.redirectURI` non-empty — the operator-registered callback URI
//     doubles as the sign-out fall-back destination. This is the common
//     single-page-app pattern: the same URI serves as the post-sign-in
//     landing AND the post-sign-out landing (browsers landing on the URI
//     post-sign-out arrive without auth cookies and re-trigger the
//     category (a) 302 challenge naturally).
//  3. Empty Location — neither the matcher matched nor cc.redirectURI
//     was configured. The 302 emission proceeds with an empty Location;
//     the browser typically renders the current page / no-op. This is
//     the "browser-default" arm per ADR-0184 §Context.
//
// # OnDestroy guard discipline per ADR-0159 D4 precedent
//
// handleSignout is synchronous — there is no async-resume goroutine that
// could race with OnDestroy in the sign-out flow (the callback +
// refresh-token paths are the only async legs per phase-20 SPEC §6.8).
// The guard nonetheless mirrors the other handlers' no-touch-dcb-after-
// OnDestroy discipline (handleBadState / handleUnauthenticated do the
// same `if f.dcb != nil` check; this file adds the `f.done` short-circuit
// for symmetry with applyTokenEndpointResponse + applyRefreshTokenResponse
// per ADR-0159 D4 precedent). The guard cost is one mutex acquisition on
// the synchronous deny path — negligible.
//
// # Dispatcher signature
//
// handleSignout takes the inbound request headers as an argument so the
// denyRedirectMatcher can read them via the headerView adapter. The Task
// 5 SKELETON's no-headers entry shape (`f.handleSignout()`) is replaced
// by this Task 9 signature (`f.handleSignout(headers)`); the dispatcher
// in `decode_headers.go::DecodeHeaders` is updated to pass the headers
// through. The change is forward-stable — Task 11 + Task 12 do not need
// to revisit the signature.
//
// # Cross-references
//
//   - SPEC §4.1 category (c) wire shape; §4.4 Location construction
//     category (c); §4.5 Set-Cookie envelope discipline category (c);
//     §6.3 dispatch priority 1; §6.9 handleSignout
//   - SPEC §1.1 AMEND-4 (no `signout_completed` counter); §11 §20.P8
//     REFUTED + §20.P11 RATIFIED-AS-ABSENT
//   - ADR-0044 (in-place edit discipline; this file lands the §Decision
//     + §Consequences body of ADR-0184 §Context drafted at the SPEC
//     commit); ADR-0085 (nil-tolerance); ADR-0159 D4 (OnDestroy guard
//     precedent); ADR-0181 (6-counter wire-exact roster; sign-out
//     absence); ADR-0184 (sign-out flow ADR — Decision body lands at
//     this file's Task)

import (
	"net/http"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// handleSignout emits the category (c) 302 sign-out response per phase-20
// SPEC §4.1 + §4.5 + §6.9 + ADR-0184. The Location header is sourced
// (highest-priority first) from `cc.denyRedirectMatcher` match against
// `headers`, else `cc.redirectURI`, else empty (browser-default per
// ADR-0184 §Context). The 5 envelope cookies are emitted as Max-Age=0
// Set-Cookie headers for full envelope clearing per §4.5 category (c).
//
// NO counter increments per AMEND-4 + S5 + ADR-0181 — the 6-counter wire-
// exact upstream roster does NOT include a `signout_completed` counter
// (the 302 emission IS the sign-out completion event; operator
// observability via downstream access-logs).
//
// Returns StopIteration unconditionally — the sign-out path is a
// synchronous deny-emission (no upstream proxy).
//
// Per ADR-0159 D4 precedent the OnDestroy guard short-circuits without
// touching dcb when f.done has been set (the synchronous nature of the
// sign-out path makes a post-OnDestroy invocation unlikely in practice;
// the guard is for symmetry with the async-resume handlers).
//
// The `headers` argument is the inbound request header carrier. nil is
// tolerated per ADR-0085 (the denyRedirectMatcher receives an empty
// header view; the matcher's miss flow falls through to cc.redirectURI).
func (f *filter) handleSignout(headers http.Header) envoyhttp.FilterHeadersStatus {
	cc := f.cc

	// OnDestroy guard per ADR-0159 D4 precedent. Acquire f.mu briefly
	// to read f.done; if the stream has been torn down, short-circuit
	// without touching dcb. Returns StopIteration so the calling
	// dispatcher's contract (StopIteration on deny-emission) is honored
	// even on the guarded path.
	f.mu.Lock()
	done := f.done
	f.mu.Unlock()
	if done {
		return envoyhttp.StopIteration
	}

	// Compose the Location header per ADR-0184 + SPEC §4.4 category (c)
	// three-tier cascade.
	location := composeSignoutLocation(cc, headers)

	hdrs := envoyhttp.OrderedHeaders{
		{Name: "Location", Value: location},
	}

	// Append the full-envelope Max-Age=0 clearing per SPEC §4.5
	// category (c). emitCategoryC_Signout (cookies.go) emits all 5
	// envelope cookies (BearerToken / OauthHMAC / OauthExpires /
	// IdToken / RefreshToken) with Max-Age=0; the IdToken cookie is
	// emitted here even though it's `(n/a)` for category (a) + (b) per
	// the §4.5 table — sign-out clears the FULL envelope including the
	// deferred-write IdToken slot.
	clearing := make(http.Header)
	emitCategoryC_Signout(clearing, cc.cookieNames, cc.cookieAttrs)
	for _, v := range clearing["Set-Cookie"] {
		hdrs = append(hdrs, envoyhttp.HeaderField{Name: "Set-Cookie", Value: v})
	}

	if f.dcb != nil {
		f.dcb.SendLocalReply(302, "", hdrs)
	}
	return envoyhttp.StopIteration
}

// composeSignoutLocation builds the category (c) 302 Location header value
// per ADR-0184 + SPEC §4.4 category (c) three-tier cascade.
//
// Tier 1 (highest priority): cc.denyRedirectMatcher returns (url, true) —
// the matched URL is the operator-intended post-sign-out destination per
// ADR-0184 §Context. Per ADR-0085 nil-tolerance, a nil matcher closure is
// treated as a guaranteed miss (no panic). A nil `headers` carrier is
// also tolerated — the matcher receives a no-op headerLookup that
// returns "" for every key, ensuring the miss fall-through.
//
// Tier 2: cc.redirectURI non-empty — the operator-registered callback URI
// doubles as the sign-out fall-back destination (single-page-app pattern).
//
// Tier 3: empty Location — neither tier 1 nor tier 2 yielded a URL. The
// browser-default behavior (typically: no navigation; render the current
// page) per ADR-0184 §Context.
func composeSignoutLocation(cc *compiledConfig, headers http.Header) string {
	if cc.denyRedirectMatcher != nil {
		var view headerLookup
		if headers != nil {
			view = headerView(headers)
		} else {
			view = emptyHeaderView{}
		}
		if url, ok := cc.denyRedirectMatcher(view); ok {
			return url
		}
	}
	return cc.redirectURI
}

// emptyHeaderView is the no-op headerLookup adapter used when
// composeSignoutLocation receives a nil headers carrier. Every Get
// returns the empty string, guaranteeing the denyRedirectMatcher miss
// fall-through. Mirrors the ADR-0085 nil-tolerance convention.
type emptyHeaderView struct{}

// Get returns "" for every key per the no-op contract.
func (emptyHeaderView) Get(string) string { return "" }
