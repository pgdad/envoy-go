// Package rbac implements envoy.filters.http.rbac — Envoy v1.37.2's canonical
// "role-based access control" filter, decoder-only MVP, under the 07.1 HTTP
// filter framework. Phase 16. TWO framework primitives introduced
// (matcher-engine evaluator at internal/matcher/ per ADR-0142; TLS-principal
// accessor on DecoderFilterCallbacks per ADR-0144) plus the filter itself.
//
// Listener-level RBAC outer-proto fields envoy-go consumes (7 — proto-faithful
// per §1.1 amendment 1; outer envelope has NO silent-ignored set):
//   - rules (config.rbac.v3.RBAC; primary engine; UDPA-field_alias-grouped
//     with `matcher` per amendment 2 — rules wins when both set per
//     rbac.pb.go:38 + §11.P1)
//   - matcher (xds.type.matcher.v3.Matcher; alternative primary engine per
//     amendment 2)
//   - shadow_rules (config.rbac.v3.RBAC; parallel shadow walk; never affects
//     disposition; emits shadow_allowed / shadow_denied counters; UDPA-
//     field_alias-grouped with `shadow_matcher`)
//   - shadow_matcher (xds.type.matcher.v3.Matcher)
//   - rules_stat_prefix (string; empty permitted per amendment 3; namespaces
//     the primary engine's stat surface)
//   - shadow_rules_stat_prefix (string; empty permitted; namespaces shadow)
//   - track_per_rule_stats (bool; when true, per-policy counters lazily
//     allocate on first match)
//
// Inner config.rbac.v3.RBAC sub-message consumed (per §6.5):
//   - action (PGV defined_only enum: ALLOW=0 / DENY=1 / LOG=2; LOG always
//     allows + records matched-policy per amendment 5)
//   - policies (map keyed by policy name; walked in lexicographic order per
//     rbac.pb.go:268-269)
//
// Silent-ignored inside config.rbac.v3.RBAC (per ADR-0040 silent-ignore
// discipline):
//   - audit_logging_options ([#not-implemented-hide:] upstream per §2.1.1)
//   - Policy.condition (CEL field; amendment 6 + Q7 silent-ignored)
//   - Policy.checked_condition (CEL field; amendment 6 silent-ignored)
//   - Policy.cel_config (CEL field NEW per amendment 6; structurally absent
//     in go-control-plane v1.32.4 binding — silent-ignore is doctrinal)
//
// Per-route discipline — 7th canonical absent-implies-disabled-OR-wholesale-
// override per ADR-0125 §(xii) amendment (anchored at Task 10): RBACPerRoute
// wrapper has reserved field 1 + single `rbac` optional field at field 2.
// (a) rbac absent → wholly disabled on this route (passthrough; no counters);
// (b) rbac present → wholesale-override of listener-level config (per ADR-0073
// most-specific override semantic). Per-route stats are INDEPENDENT per
// ADR-0145 (lands at Task 8; phase-16 is the THIRD row using INDEPENDENT-
// stats after phase-11 + phase-15).
//
// Permission Large 11 + 3 deferred (per §2.3 + ADR-0143; the deferred 3
// PARSE-REJECT envoy-go-only at parse time): any / header / url_path /
// destination_ip / destination_port / destination_port_range /
// requested_server_name / and_rules / or_rules / not_rule / sourced_metadata
// (parse-supported; always-FALSE runtime per §2.5). DEFERRED 3: metadata
// (deprecated per §11.P12); matcher (extension); uri_template (extension).
//
// Principal Large 11 + 3 deferred (per §2.4 + amendment 7 + ADR-0143):
// any / authenticated (three-case algorithm per §6.6 + amendment 12; consumes
// DownstreamPrincipal() per ADR-0144 lands at Task 6) / direct_remote_ip /
// remote_ip (XFF-resolved) / header / url_path / and_ids / or_ids / not_id /
// sourced_metadata (always-FALSE) / filter_state (always-FALSE). DEFERRED 3:
// source_ip (deprecated); metadata (deprecated); custom (NEW per amendment 7;
// extension TypedExtensionConfig — the 14th Principal variant per §8.11).
//
// Public surface (per §6.1):
//   - TypeURL const — the canonical filter typed_config type URL.
//   - New(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) —
//     the HTTPFilterFactory registered at boot per ADR-0072 + ADR-0140
//     (boot-registration ordering: alphabetical-after-localratelimit; lands
//     at Task 11).
//
// HTTPFilter value: DECODER-only (Decoder: f, Encoder: nil). Per ADR-0140 +
// §1 item 5; mirrors phase-12 csrf + phase-13 buffer decoder-only precedent.
// RBAC is a pre-body request gate; the disposition (allow / deny / log) is
// computed at DecodeHeaders BEFORE the request body is forwarded.
//
// Iteration protocol:
//   - DecodeHeaders: resolve per-route TPFC; build evalContext from headers
//   - connection state; run primary engine + (if configured) shadow engine;
//     dispatch ALLOWED → HeaderContinue; DENIED → SendLocalReply(403, "RBAC:
//     access denied", text/plain) + HeaderStopIteration; LOG-partial folds
//     into ALLOWED + matched-policy captured for per-policy emission per
//     amendment 5.
//   - DecodeData / DecodeTrailers: pass-through (RBAC is pre-body).
//   - OnDestroy: no-op (no async resources).
//   - SetDecoderCallbacks: stores f.dcb for later SendLocalReply +
//     RequestRouteConfig + DownstreamPrincipal use.
//   - The full DecodeHeaders body lands at Task 7 per ADR-0141 + ADR-0146;
//     the body at Task 2 is a stub returning HeaderContinue.
//
// Deny-path wire shape (per §1.1 amendment 10 + §11.P5 + ADR-0140):
// SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain}).
// Body is a 19-byte ASCII payload byte-exact; 4-header set in lowercase
// wire-form (content-length: 19, content-type: text/plain, date, server:
// envoy); keep-alive disposition (no Connection: close).
//
// Stat surface (per ADR-0140 + ADR-0145; lands at Task 8):
//   - 4 base counters per active stat-prefix combination: allowed / denied /
//     shadow_allowed / shadow_denied (per §1.1 amendment 8 — NO `logged`
//     counter per §11.P6 REFUTES BRAINSTORM 5-counter hypothesis; LOG folds
//     into `allowed`).
//   - Lazy per-policy counters when track_per_rule_stats=true: each matched
//     policy allocates <policy_name>.<suffix> counters via NewCounterIfAbsent
//     post-Freeze idempotent (mirrors phase-11 ADR-0117 + phase-15 ADR-0139).
//   - Namespace shape SN2-reuse hypothesis per §11.P7 + ADR-0145
//     (http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>); NO new
//     SN-numbered rule. Impl-time empirical scrape RATIFIES or AMENDS at
//     Task 8.
//
// Wire-shape divergence-windows vs reference Envoy v1.37.2 (documented at
// BEHAVIOR_CONTRACT.md §13.4):
//   - LOG-action `access_log_hint` dynamic metadata: envoy-go silent (no
//     metadata emission); Envoy emits per amendment 5 + §8.6. Couples to a
//     future dynamic-metadata family phase.
//   - `response_code_details` field: envoy-go absent on DENY; Envoy emits
//     "rbac_access_denied_matched_policy[<sanitized_policy_id>]" per §11.P5
//   - amendment 11 + §8.12. Couples to a future HCM framework phase.
//   - CEL three fields (condition / checked_condition / cel_config):
//     envoy-go silent-ignored; Envoy evaluates per Q7. Couples to a future
//     CEL evaluator family phase.
//   - Shadow-rules access-log integration: envoy-go counter-only matches
//     Envoy v1.37.2 counter-only confirmation per §11.P13 — no current
//     divergence; documented as forward-pointer.
//
// Cross-cutting ADR anchors (per ADR-0044 ADR-on-impl convention; authored
// at impl-time per phase-13 + phase-15 precedent):
//   - ADR-0140 (this package's shape + decoder-only HTTPFilter value + 4-base-
//     counter filterStats + lazy per-policy counter allocation + deny-path
//     wire shape + boot-registration ordering) — lands at Task 2.
//   - ADR-0141 (compiledConfig 8-field shape + 7-consumed proto-faithful field
//     decomposition + dual-engine dispatch rules-wins-when-both-set +
//     UDPA-field_alias-annotation framing + envoy-go-side defensive PGV-mirror
//     validation + ALLOW/DENY/LOG-partial action enum + CEL three-field
//     silent-ignore + audit_logging_options silent-ignore) — lands at Task 2.
//   - ADR-0142 (matcher-engine evaluator framework primitive at NEW top-level
//     internal/matcher package) — lands at Task 3.
//   - ADR-0143 (Permission + Principal Large 11+11 evaluators + AND/OR/NOT
//     combinators + deprecated-field PARSE-REJECT + extension-coupling
//     PARSE-REJECT + Principal_Authenticated three-case algorithm) — lands
//     across Tasks 4 + 5.
//   - ADR-0144 (TLS-principal accessor on DecoderFilterCallbacks framework
//     primitive + HCM plumbing) — lands at Task 6.
//   - ADR-0145 (stat surface 4-base + variable per-policy + namespace SN2-
//     reuse + INDEPENDENT per-route stats) — lands at Task 8.
//   - ADR-0146 (shadow-evaluation discipline + LOG-partial divergence-window +
//     track_per_rule_stats per-policy emission + response_code_details
//     divergence-window + BEHAVIOR_CONTRACT §13.4 forward-pointer notes) —
//     lands at Task 9.
//   - ADR-0125 §(xii) amendment paragraph (7th canonical absent-implies-
//     disabled-OR-wholesale-override per-route pattern) — lands at Task 10.
package rbac
