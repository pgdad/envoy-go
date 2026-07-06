package oauth2

// stats.go — 6-counter wire-exact filterStats per phase-20 ADR-0181 + SPEC
// §6.11 + §6.12 + ADR-0143 HCM-rooted SN2-reuse convention.
//
// # Stat-name byte-exact roster per AMEND-4 + S5 + §20.P8 REFUTED
//
// The 6 counter wire-names are pinned upstream-byte-exact via per-counter
// `const statName*` declarations. The BRAINSTORM 8-counter hypothesis was
// REFUTED at the SPEC §20.P8 empirical scrape against reference Envoy
// v1.37.2 — the upstream stat-surface is unambiguously 6. The 2 absent
// counters per §20.P11 RATIFIED-AS-ABSENT:
//
//   - `signout_completed` ABSENT — sign-out completion IS the 302 emission
//     per SPEC §4.6 + AMEND-4 + §6.9; no separate counter.
//
//   - `cookie_decrypt_failure` ABSENT — decryption-failure fall-back returns
//     ciphertext-as-plaintext per AMEND-3 + §20.P11 + ADR-0182; downstream
//     HMAC validation rejects naturally; no separate counter. **envoy-go-strict
//     departure flag CLOSED-AS-ABSENT** per ADR-0181 §Decision.
//
// # HCM-rooted SN2-reuse per ADR-0143
//
// The stat-prefix shape mirrors the established §9 family-row convention
// (extauthz / extproc / jwt_authn precedent):
//
//	http.<HCM_stat_prefix>.oauth2.<counter>
//
// Empty HCM stat_prefix (test code paths) folds to the bare `oauth2.<counter>`
// form to satisfy the Registry's nameRE (forbids leading dots / double dots).
// Mirrors extauthz baseStatPrefix discipline per ADR-0156.
//
// # Compile-time stat-name guards per planner-time D6 + SPEC §6.12
//
// Two-layer guard:
//
//   1. Per-counter `const statName*` declarations pin the byte-exact wire
//      names at compile time. A regression on any wire name surfaces
//      immediately at build time (the constructor uses these constants).
//
//   2. The `TestStatNames_Equal_*` Group-9 assertion tests (in
//      stats_test.go) pin each constant to its string literal — preventing
//      a future refactor from silently renaming the constant alongside the
//      string literal.
//
// # NewCounterIfAbsent — multi-listener-same-prefix safety
//
// Uses `stats.Registry.NewCounterIfAbsent` (NOT `NewCounter`) to avoid the
// multi-listener-same-stat_prefix panic footgun per the rbac Task 8 lesson
// (recorded in ADR-0156 newFilterStats). Idempotent registration is safe AND
// consistent with the established §9 discipline.
//
// # Nil-tolerance contract per ADR-0085
//
// newFilterStats unconditionally dereferences `reg`. The caller
// (buildCompiledConfig at Task 11) guards `if ctx.Stats != nil` per ADR-0085
// nil-tolerance — test code paths that do not exercise stat-bearing flows
// supply a nil Registry.

import (
	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Stat-name compile-time constants per planner-time D6 + SPEC §6.12 + ADR-0181.
// -----------------------------------------------------------------------------

// statNameOauthUnauthorizedRq pins the byte-exact upstream wire name for the
// "bad state cookie at callback path" counter per AMEND-4 + §4.6. Incremented
// at category (d) bad-state-401 emission per SPEC §4.6 + ADR-0180.
const statNameOauthUnauthorizedRq = "oauth_unauthorized_rq"

// statNameOauthFailure pins the byte-exact upstream wire name for the
// "token_endpoint POST terminal-failure" counter per AMEND-4 + §4.6.
// Incremented at category (d) terminal-401 emission per SPEC §4.6 + ADR-0180.
// NOT also incremented on the refresh-failure path per ADR-0183.
const statNameOauthFailure = "oauth_failure"

// statNameOauthPassthrough pins the byte-exact upstream wire name for the
// pass-through-matcher hit counter per AMEND-4 + §4.6. Incremented on the
// `pass_through_matcher` hit (oauth2 bypass).
const statNameOauthPassthrough = "oauth_passthrough"

// statNameOauthSuccess pins the byte-exact upstream wire name for the
// successful sign-in completion counter per AMEND-4 + §4.6. Incremented at
// category (b) post-callback-success per SPEC §4.6 + ADR-0181.
const statNameOauthSuccess = "oauth_success"

// statNameOauthRefreshtokenSuccess pins the byte-exact upstream wire name
// for the silent refresh-token rotation success counter per AMEND-4 + §4.6.
// Incremented per CONTINUE-with-deferred-Set-Cookie completion on the
// refresh leg per ADR-0183.
//
// NOTE: the upstream wire name uses `refreshtoken` (no underscore between
// "refresh" and "token") — INTENTIONAL upstream byte-exact reuse, NOT a
// typo. A "refresh_token_success" variant would diverge wire-compat.
const statNameOauthRefreshtokenSuccess = "oauth_refreshtoken_success"

// statNameOauthRefreshtokenFailure pins the byte-exact upstream wire name
// for the silent refresh-token rotation failure counter per AMEND-4 + §4.6.
// Incremented on refresh-token POST non-2xx → 302 challenge fall-through per
// ADR-0183. NOT also `oauth_failure` per AMEND-3 + ADR-0183 counter-discipline.
const statNameOauthRefreshtokenFailure = "oauth_refreshtoken_failure"

// -----------------------------------------------------------------------------
// filterStats — 6-counter wire-exact stat-surface holder per ADR-0181.
// -----------------------------------------------------------------------------

// filterStats carries the 6-counter wire-exact stat surface per phase-20
// ADR-0181 + AMEND-4 + S5 + §20.P8 REFUTED. All 6 counters allocate
// unconditionally at buildCompiledConfig time (predeclared empty counters
// for scrape stability — operators get a consistent counter surface).
//
// Mirrors the extauthz `filterStats` shape (per ADR-0156); the discipline is
// shared across all §9 family-row filters per ADR-0143 SN2-reuse.
type filterStats struct {
	// oauthUnauthorizedRq — bad state cookie at callback path → 401 per
	// SPEC §4.6 category (d).
	oauthUnauthorizedRq *stats.Counter

	// oauthFailure — token_endpoint POST terminal-failure → 401 per SPEC
	// §4.6 category (d). NOT also incremented on refresh-failure per
	// ADR-0183.
	oauthFailure *stats.Counter

	// oauthPassthrough — pass_through_matcher hit per SPEC §4.6.
	oauthPassthrough *stats.Counter

	// oauthSuccess — successful sign-in completion per SPEC §4.6 category
	// (b).
	oauthSuccess *stats.Counter

	// oauthRefreshtokenSuccess — silent refresh-token rotation success per
	// SPEC §4.6 + ADR-0183.
	oauthRefreshtokenSuccess *stats.Counter

	// oauthRefreshtokenFailure — silent refresh-token rotation failure per
	// SPEC §4.6 + ADR-0183.
	oauthRefreshtokenFailure *stats.Counter
}

// -----------------------------------------------------------------------------
// newFilterStats — 6-counter registration per ADR-0181 + ADR-0143 SN2-reuse.
// -----------------------------------------------------------------------------

// newFilterStats registers the 6 oauth2 counters per ADR-0181 + AMEND-4 + S5
// with the HCM-rooted SN2-reuse prefix per ADR-0143. All 6 registered
// unconditionally (predeclared empty counters for scrape stability per
// Prometheus best practice; mirrors the extauthz / extproc / jwt_authn
// unconditional-allocation discipline).
//
// Internal stat path per SN2-reuse:
//
//	http.<HCM_stat_prefix>.oauth2.<counter>
//
// Empty HCM stat_prefix (test code paths) folds to the bare
// `oauth2.<counter>` form to satisfy the Registry's nameRE (forbids leading
// dots / double dots).
//
// Uses `NewCounterIfAbsent` (rather than `NewCounter`) to avoid the
// multi-listener-same-prefix panic footgun per the rbac Task 8 lesson —
// idempotent registration is safe and consistent with the existing §9
// discipline.
//
// **Caller MUST guard against nil registry**: this helper unconditionally
// dereferences `reg`. The nil-tolerance contract lives at buildCompiledConfig's
// `if ctx.Stats != nil` gate per ADR-0085. Test code paths that do not
// exercise stat-bearing flows supply a nil Registry; production code paths
// always carry a non-nil Registry at HCM-build time per ADR-0061.
func newFilterStats(reg *stats.Registry, hcmStatPrefix string) *filterStats {
	prefix := baseStatPrefix(hcmStatPrefix)
	return &filterStats{
		oauthUnauthorizedRq:      reg.NewCounterIfAbsent(prefix + statNameOauthUnauthorizedRq),
		oauthFailure:             reg.NewCounterIfAbsent(prefix + statNameOauthFailure),
		oauthPassthrough:         reg.NewCounterIfAbsent(prefix + statNameOauthPassthrough),
		oauthSuccess:             reg.NewCounterIfAbsent(prefix + statNameOauthSuccess),
		oauthRefreshtokenSuccess: reg.NewCounterIfAbsent(prefix + statNameOauthRefreshtokenSuccess),
		oauthRefreshtokenFailure: reg.NewCounterIfAbsent(prefix + statNameOauthRefreshtokenFailure),
	}
}

// baseStatPrefix returns the canonical base-prefix segment for the 6 oauth2
// counters per ADR-0143 + SPEC §5.3 HCM-rooted SN2-reuse. Shape:
//
//	http.<HCM_stat_prefix>.oauth2.
//
// Empty HCM stat_prefix (test code paths) folds to the bare `oauth2.` form
// to satisfy the Registry's nameRE (forbids leading dots / double dots).
// Mirrors the extauthz baseStatPrefix discipline per ADR-0156.
func baseStatPrefix(hcmStatPrefix string) string {
	if hcmStatPrefix == "" {
		return "oauth2."
	}
	return "http." + hcmStatPrefix + ".oauth2."
}
