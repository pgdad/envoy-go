// Package jwtauthn implements envoy.filters.http.jwt_authn — Envoy v1.37.2's
// canonical "JWT authentication" filter, decoder-only MVP, under the 07.1 HTTP
// filter framework. Phase 17. TWO framework primitives introduced
// (HTTP-outbound JWKS fetcher at internal/jwks/ per ADR-0150; JWT verifier at
// internal/jwt/ per ADR-0151) plus the filter itself.
//
// JwtAuthentication outer-proto 6-field surface (per §1.1 amendment 1; proto
// has 6 fields per `[#next-free-field: 7]`):
//   - providers (map<string, JwtProvider>; consumed)
//   - rules (repeated RequirementRule; listener-level first-match dispatch;
//     consumed)
//   - filter_state_rules (FilterStateRule; SILENT-IGNORED per amendment 1 +
//     §8 deferral 12; couples to future filter-state-family framework phase)
//   - bypass_cors_preflight (bool; consumed)
//   - requirement_map (map<string, JwtRequirement>; referenced by listener-
//     level rules AND per-route TPFC; consumed)
//   - strip_failure_response (bool; consumed)
//
// JwtProvider 21-field surface (per amendments 2 + 3; proto has 21 fields per
// `[#next-free-field: 22]`):
//   - 13 CONSUMED: issuer, audiences, remote_jwks, local_jwks, forward,
//     from_headers, from_params, from_cookies, forward_payload_header,
//     pad_forward_payload_header, claim_to_headers, clear_route_cache,
//     clock_skew_seconds.
//   - 8 SILENT-IGNORED: payload_in_metadata, header_in_metadata,
//     failed_status_in_metadata, normalize_payload_in_metadata (dynamic-
//     metadata-family-coupled per §8 deferrals 1-4); jwt_cache_config
//     (caching-family-coupled per §8 deferral 8; structurally unreachable in
//     MVP); subjects, require_expiration, max_lifetime (v1.37.x claim-coverage-
//     extension-family-coupled per §8 deferrals 15-17 + amendments 2 + 3).
//
// 6-variant JwtRequirement evaluator (per §11.P16 + ADR-0149):
//   - provider_name: validate JWT against named provider.
//   - provider_and_audiences: validate against named provider with per-rule
//     audience override.
//   - requires_any: OR-semantic; short-circuit on first success.
//   - requires_all: AND-semantic; short-circuit on first failure.
//   - allow_missing: JWT absent → OK; JWT present-and-invalid → FAIL.
//   - allow_missing_or_failed: any outcome → OK.
//
// RS+ES algorithm allow-list (per §11.P1 + ADR-0151): 6 algorithms —
// RS256/384/512 (RSASSA-PKCS1-v1_5 + SHA-256/384/512) + ES256/384/512 (ECDSA
// + P-256/P-384/P-521). HS family + EdDSA + `none` + PS family DEFERRED per §8
// deferrals 5-7 + SPEC §12.6.
//
// All 4 token extraction sources (per §11.P14 + §11.P15 + ADR-0152):
//   - default Authorization Bearer (when no explicit per-provider extraction-
//     sources set; case-insensitive header lookup).
//   - default access_token query param (case-sensitive name; URL-decoded value;
//     first-value-only on multi-value).
//   - configured from_headers (case-insensitive name; substring-search on
//     value_prefix).
//   - configured from_params (case-sensitive name; URL-decoded value).
//   - configured from_cookies (case-sensitive name; verbatim value; NO URL-
//     decode per RFC 6265).
//
// Side-effect emit-order (per §6.9 + §11.P10 + §11.P13 + ADR-0149) AFTER
// successful JWT validation:
//
//	(1) strip JWT from request header (when forward=false)
//	(2) emit forward_payload_header (base64url-encoded payload)
//	(3) emit claim_to_headers (dot-notation path extraction; array claims
//	    rejected per §11.P10)
//	(4) clear_route_cache (HCM-side primitive invocation when proto bool true).
//
// Per-route 8th canonical pattern (per ADR-0125 §(xiii) + ADR-0153 +
// PerRouteConfig at config.pb.go:1595-1679): wrapper proto with REQUIRED oneof
// `RequirementSpecifier` carrying two arms — `disabled` (bool; varint at
// field 1; NOT Empty as BRAINSTORM hypothesized) and `requirement_name`
// (string; bytes at field 2; PGV min_len=1). The defining feature is
// **string-reference-delegation** — per-route does NOT embed a filter config;
// it embeds a string name that resolves at REQUEST TIME against the listener-
// level `JwtAuthentication.requirement_map`. Per-route stats SHARED with
// listener-level per ADR-0154 (NO INDEPENDENT-stats discipline; pure
// delegation does not spawn new state; mirrors phase-12 csrf + phase-13 buffer
// + phase-14 compressor SHARED-stats discipline).
//
// Public surface (per §6.1):
//   - TypeURL const — the canonical filter typed_config type URL.
//   - New(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) —
//     the HTTPFilterFactory registered at boot per ADR-0072 + ADR-0148
//     (boot-registration ordering: alphabetical-after-header_mutation per
//     ADR-0100 §2.2; lands at Task 10).
//
// HTTPFilter value: DECODER-only (Decoder: f, Encoder: nil). Per ADR-0148 +
// §1 item 5; mirrors phase-12 csrf + phase-13 buffer + phase-16 rbac
// decoder-only precedent (4th §9 row to ship pure decode-side). jwt_authn is
// a pre-body request gate; the disposition (validate + forward / strip /
// claim-to-headers / deny) is computed at DecodeHeaders BEFORE the request
// body is forwarded.
//
// Iteration protocol (DECODER-ONLY per ADR-0148):
//   - DecodeHeaders: capture original URI for WWW-Authenticate realm; resolve
//     per-route TPFC; CORS-preflight bypass when configured; extract token(s);
//     evaluate requirement; apply side-effects on success OR emit SendLocalReply
//     on failure. Full body lands at Task 9 per ADR-0155; Task 2 ships a
//     minimal Continue stub.
//   - DecodeData / DecodeTrailers: pass-through (jwt_authn is pre-body).
//   - OnDestroy: no-op (no async per-stream resources; the jwks.Fetcher is
//     owned by the listener-level compiledProvider per ADR-0150 §Decision).
//   - SetDecoderCallbacks: stores f.dcb for later SendLocalReply +
//     RequestRouteConfig use.
//
// Deny-path wire shape (per §1.1 amendments 8 + 12 + §11.P1 + §11.P2 +
// ADR-0155):
//   - Status: 401 for most failure-reasons + 403 specifically for
//     JwtAudienceNotAllowed (mirrors Envoy filter.cc).
//   - Body: canonical jwt_verify_lib getStatusString(status) table (~70-entry
//     mapping; ~10 commonly-hit at runtime).
//   - WWW-Authenticate header: `Bearer realm="<original_uri>"` with
//     conditional `, error="invalid_token"` append for non-JwtMissed
//     statuses (RFC 6750 §3 challenge syntax).
//   - strip_failure_response: true strips both body AND WWW-Authenticate.
//   - Per-route runtime-resolve error: 403 + body `"Failed JWT authentication:
//     Wrong requirement_name: <name>"` + NO WWW-Authenticate per ADR-0153 +
//     ADR-0155.
//
// Stat surface (per §1.1 amendments 9 + 10 + §11.P6 + §11.P7 + ADR-0154):
// 7 base counters allocated unconditionally at New() time (NO lazy-allocation;
// SHARED-stats discipline; mirrors phase-12 csrf + phase-13 buffer precedent):
//   - allowed (request validated successfully + forwarded)
//   - denied (validation failed; SendLocalReply emitted)
//   - cors_preflight_bypassed (CORS preflight matched bypass predicate; canonical
//     name per amendment 10 — REFUTES BRAINSTORM `bypassed_cors_preflight`)
//   - jwks_fetch_success / jwks_fetch_failed (per JWKS refresh cycle)
//   - jwt_cache_hit / jwt_cache_miss (STRUCTURALLY UNREACHABLE under MVP per §8
//     deferral 8; counters registered for scrape-stability — operators see 0;
//     wired fully when jwt_cache_config is honored in a future phase).
//
// Namespace shape SN2-reuse hypothesis per §11.P7 (RATIFIED-PENDING-IMPL-TIME-
// EMPIRICAL-SCRAPE at Task 8 per ADR-0154 + planner-time decision 10):
// `http.<HCM_stat_prefix>.jwt_authn.<counter>`. Impl-time empirical scrape
// RATIFIES or AMENDS at Task 8.
//
// Per-route stats discipline: SHARED with listener-level per ADR-0154
// (DIVERGES from phase-11/15/16 INDEPENDENT-stats discipline). Pure
// delegation-by-name into the listener-level requirement_map does NOT spawn
// new policy-evaluation state, so a shared stat namespace is operationally
// correct.
//
// Wire-shape divergence-windows vs reference Envoy v1.37.2 (per §1.1
// amendments 1 + 2 + 3 + 11 + §8 deferrals 1-4 + 8 + 12 + 13 + 15-17):
//   - 4 dynamic-metadata-family-coupled fields (payload_in_metadata +
//     header_in_metadata + failed_status_in_metadata + normalize_payload_in_metadata):
//     envoy-go silent-ignored; Envoy emits dynamic metadata. Couples to future
//     dynamic-metadata family framework phase.
//   - jwt_cache_config (default 100-entry LRU): envoy-go silent-ignored;
//     Envoy caches verified JWT bytes for repeat-request optimization. Couples
//     to future caching-family framework phase.
//   - filter_state_rules: envoy-go silent-ignored; Envoy supports
//     stream-info-driven dynamic provider selection. Couples to future
//     filter-state-family framework phase.
//   - 3 v1.37.x claim-coverage extensions (subjects + require_expiration +
//     max_lifetime): envoy-go silent-ignored at parse + claim-validation;
//     Envoy enforces these strict checks. Future jwt-claim-coverage phase.
//   - response_code_details on DENY: envoy-go MVP no emission (SendLocalReply
//     has no response-code-details slot); Envoy emits
//     `jwt_authn_access_denied{<reason>}`. Couples to future HCM
//     response-code-details framework phase (joint with phase-16 ADR-0146
//     forward-pointer).
//
// Cross-cutting ADR anchors (per ADR-0044 ADR-on-impl convention; authored at
// impl-time per phase-13 + phase-15 + phase-16 precedent):
//   - ADR-0148 (this package's shape + decoder-only HTTPFilter + 7-base-counter
//     filterStats + unconditional counter allocation + deny-path wire shape +
//     boot-registration ordering) — lands at Task 2.
//   - ADR-0149 (compiledConfig shape + 5-of-6 outer + 13-of-21 JwtProvider +
//     6-variant JwtRequirement + RS+ES allow-list + side-effect emit-order +
//     defensive PGV-mirror) — lands at Task 2.
//   - ADR-0150 (HTTP-outbound JWKS fetcher framework primitive at NEW top-level
//     internal/jwks/ package) — lands at Task 3.
//   - ADR-0151 (JWT verifier framework primitive at NEW top-level internal/jwt/
//     package) — lands at Task 4.
//   - ADR-0152 (token extraction across all 4 sources) — lands at Task 5.
//   - ADR-0153 (per-route 8th canonical pattern with string-reference-delegation
//   - runtime-resolve at request time + ADR-0125 §(xiii) cross-reference) —
//     lands at Task 7.
//   - ADR-0154 (stat surface 7 base counters + canonical naming +
//     RATIFIED-PENDING-IMPL-TIME closure for §11.P7) — lands at Task 8.
//   - ADR-0155 (deny-path wire shape — 401-or-403 + getStatusString body +
//     WWW-Authenticate Bearer realm) — lands at Task 9.
//   - ADR-0125 §(xiii) amendment paragraph (8th canonical with
//     string-reference-delegation) — ALREADY landed at SPEC commit per
//     phase-13/14/15/16 precedent.
package jwtauthn
