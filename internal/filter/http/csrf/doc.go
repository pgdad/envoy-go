// Package csrf implements envoy.filters.http.csrf — Envoy v1.37.2's canonical
// same-origin enforcement filter. The filter rejects modifying-method requests
// whose Origin/Referer-derived source-origin does not match the request's
// Host/:authority-derived target-origin (or any operator-supplied
// additional_origins[]).
//
// MVP envelope per phase 12 SPEC §1 + §1.1:
//   - 1 proto field actively consumed (additional_origins[].StringMatcher.exact
//     non-empty values; non-exact variants dropped at PARSE time per ADR-0101 §3).
//   - 1 proto field PGV-validated-not-honored at runtime (filter_enabled — REQUIRED
//     at parse-time per §11.11 amendment; runtime always-100%-active).
//   - 1 proto field deferred (shadow_enabled — optional at parse, never-shadow at
//     runtime; couples to Runtime + hot restart family).
//
// Comparison algorithm is host:port-only equality per §11.3 + §11.7 + §11.8
// amendments: scheme is computed only to make URLs parseable then stripped on
// both sides; NO case folding; NO default-port stripping; trailing slashes are
// stripped via the URL parser. additional_origins[].exact values are matched
// against the source's host[:port] form (NOT full URL with scheme — operator
// footgun documented at BEHAVIOR_CONTRACT §13.4).
//
// Origin-extraction trichotomy per §11.2 amendment: (i) Origin: null literal →
// empty source NO Referer fallback; (ii) Origin empty/absent → fall back to
// Referer's hostAndPort; (iii) Origin non-empty unparseable → verbatim string
// used as source.
//
// Method gate is canonical 4-method set {POST, PUT, DELETE, PATCH} per §11.1.
// Non-modifying methods short-circuit to Continue BEFORE any state touch.
//
// Per-route TPFC override is data-only with SHARED listener-level stats per
// §11.9 amendment — diverges from phase 11 local_ratelimit precedent (ADR-0117)
// which had INDEPENDENT per-route stats. csrf is the FIRST production filter
// to demonstrate the "wholesale data-only override + shared stats" pattern.
//
// Iteration-protocol coverage:
//   - DecodeHeaders runs the disposition table; on allow → Continue; on
//     missing/reject → SendLocalReply(403) + StopIteration (request-side
//     terminal-replace per ADR-0102; reused verbatim from phase 09 fault).
//   - DecodeData / DecodeTrailers / OnDestroy: pass-through / no-op.
//   - NO encode-side methods. The HTTPFilter value sets Decoder: f, Encoder: nil.
//
// Cross-cutting ADR anchors (ADR-0122/0123/0124 land in phase 12 Tasks 3-4):
//   - ADR-0120 (package shape + boot registration ordering)
//   - ADR-0121 (runtimeConfig + 1/1/1-field decomposition + PGV-mirror discipline)
//   - ADR-0122 (origin trichotomy + host:port-only equality + method gate)
//   - ADR-0123 (rejection wire shape + SendLocalReply reuse)
//   - ADR-0124 (3-counter stat surface + namespace anchor at HCM stat_prefix +
//     per-route stats SHARED with listener-level)
package csrf
