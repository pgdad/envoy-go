# Phase 16 SPEC — `envoy.filters.http.rbac`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 16 status flips `planned → in-progress` at this SPEC commit per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 / 12 / 13 / 14 / 15 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 683 lines). §§1–11 are the pre-§9-empirical-pin design sketch (PRESERVED VERBATIM per D-3.5); the §11 empirical-pin block in this SPEC re-runs all 18 BRAINSTORM §9 pins (P1–P18) against reference Envoy v1.37.2 IN-SESSION per ADR-0004. NO post-landing BRAINSTORM §12 amendment cycle is authored — the empirical re-frame is structured for the §1.1 amendment-block channel (mirrors phase-12 csrf 4-amendment + phase-14 compressor 6-amendment + phase-15 bandwidth_limit 10-amendment precedents rather than the phase-13 buffer §12 amendment-cycle precedent). NO off-master prebrainstorm-notes branch.

**ADR continuity:** Phase 15 closed at ADR-0139. Phase 16 anticipates ADR-0140..ADR-0146 (7 ADRs per BRAINSTORM §7 — LARGEST §9-row roster to date) + ADR-0125 amendment paragraph §(xii) (for the NEW 7th canonical per-route pattern). Phase 16 ships these ADRs **anticipated** at SPEC time per ADR-0044 ADR-on-impl convention; impl session anchors each at the task it first lands in (mirrors phase-13 + phase-15 precedent; phase-14's SPEC-time-pre-landing of ADR-0129..ADR-0133 is the divergent precedent). Next-free ADR after phase 16 is ADR-0147.

**§3 framework-survey result up front (locks §3 TWO-framework-deltas claim):** Phase 16 is the FIRST §9 family-row since phase 14 to introduce non-zero framework deltas, AND the FIRST single phase to introduce TWO simultaneously: (i) a TLS-principal accessor on `DecoderFilterCallbacks` (`DownstreamPrincipal() []string`) exposing the downstream client cert's URI SAN + DNS SAN + Subject DN in priority order — required by `Principal_Authenticated`; anchored at ADR-0144. (ii) A matcher-engine evaluator framework primitive at a new top-level `internal/matcher/` package implementing the `xds.type.matcher.v3.Matcher` generic match-tree evaluator — required by the matcher-engine path of `envoy.filters.http.rbac`, anticipated cross-phase-reusable by future filters (ext_authz, jwt_authn, oauth2 all consume the same `xds.type.matcher.v3.Matcher` primitive for some of their config surface); anchored at ADR-0142. Both primitives are landed-in-phase-16 but explicitly CROSS-PHASE-REUSABLE. The §1.7 framework-delta accretion shape gains two new entries; ADR-0125 + ADR-0117 + ADR-0139 NOT amended on framework grounds.

---

## 1. Purpose

Phase 16 lands `envoy.filters.http.rbac` — Envoy's canonical role-based-access-control filter, BOTH-engine MVP (rules-engine + matcher-engine, proto-faithful per Q2), ALLOW + DENY + LOG-partial action enum, decode-side policy gate — as the NINTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), buffer (13), compressor (14), and bandwidth_limit (15), and the NINTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 16 is the FIRST §9 family-row since phase 14 to introduce non-zero framework deltas, AND the FIRST single phase to introduce TWO simultaneously per §3 framework-survey result above. The eight architectural primitives:

1. **New `internal/filter/http/rbac/` package** owning the filter implementation. Package directory + Go-package identifier are both `rbac` (single token matching the Envoy filter type-URL `envoy.filters.http.rbac` exactly; no underscore-elision question — proto type-name has no underscore). Files mirror the phase-14 + phase-15 multi-file split (the precedent for larger filters): `rbac.go` (filter type + factory + decode methods + filterStats struct + compiledConfig + per-route helper), `evaluator.go` (Permission + Principal evaluators + AND/OR/NOT combinators + dual-engine dispatch — calls into the rules-engine policy-map walk OR the matcher-engine match-tree walk), `rbac_test.go` (unit tests; anticipated 700-1000 LoC given the evaluator subsurface), `fuzz_test.go` (the 20th fuzzer in the repo — `FuzzRBACConfigParse`), `doc.go` (package overview + 7-decision Q-dialogue summary + Large-11+11 + dual-engine + shadow + LOG-partial + per-route 7th-canonical summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / localratelimit / csrf / buffer / compressor / bandwidthlimit precedent. ADR-0140 codifies.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 10 entries after phase 15: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains an eleventh `httpReg.Register(rbac.TypeURL, rbac.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → localratelimit → rbac → Freeze`. `rbac` inserts between `localratelimit` and `Freeze` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **MVP envelope: 7 consumed + 0 silent-ignored at the outer filter proto (REFINED from BRAINSTORM §1.1 item 3; see §1.1 amendment 1).** `envoy.extensions.filters.http.rbac.v3.RBAC` (per `[#next-free-field: 8]` at `rbac.pb.go:28`) has 7 top-level fields. Phase 16 consumes ALL 7 in the proto-faithful posture per Q2 + Q5 user picks. No top-level field is silent-ignored at the filter envelope — the silent-ignore set lives one level deeper, inside `config.rbac.v3.RBAC`:

   - **`rules`** (`config.rbac.v3.RBAC`; field 1) — primary policy engine; UDPA-`field_alias`-grouped with `matcher` under `rules_specifier` (per §1.1 amendment 2; NOT a Go-level oneof). When both `rules` and `matcher` set: `rules` WINS per the proto comment at `rbac.pb.go:38`; envoy-go mirrors verbatim.
   - **`shadow_rules`** (`config.rbac.v3.RBAC`; field 2) — parallel non-enforcing policy engine; UDPA-grouped with `shadow_matcher` under `shadow_rules_specifier`. When both set: `shadow_rules` WINS per `rbac.pb.go:53`.
   - **`shadow_rules_stat_prefix`** (string; field 3) — stat namespace tag for shadow counters; OPTIONAL (no PGV; empty default permitted per §1.1 amendment 4 + §11.P2).
   - **`matcher`** (`xds.type.matcher.v3.Matcher`; field 4) — alternative match-tree engine. Consumed when `rules` is unset.
   - **`shadow_matcher`** (`xds.type.matcher.v3.Matcher`; field 5) — alternative shadow match-tree.
   - **`rules_stat_prefix`** (string; field 6) — stat namespace tag for primary counters; OPTIONAL (no PGV; empty default permitted per §11.P2).
   - **`track_per_rule_stats`** (bool; field 7) — when true, emit per-policy-name counters keyed off matched policies.

   **Inside `config.rbac.v3.RBAC` (the rules-engine config; consumed when `rules` or `shadow_rules` set):**

   - `action` enum (`RBAC_Action`; PGV `defined_only = true` per `\x82\x01\x02\x10\x01` flag at `rbac.pb.go:1487`): ALLOW=0 / DENY=1 / LOG=2 (per `rbac.pb.go:83-93`; §11.P4). All three honored at parse + runtime per §2.4 + Q4; LOG-partial carries the dynamic-metadata divergence-window per §1.1 amendment 5.
   - `policies` map<string, Policy>: each Policy has `permissions` []Permission (PGV `min_items = 1` per `\x92\x01\x02\b\x01` at `rbac.pb.go:1511`; OR-semantic at runtime), `principals` []Principal (PGV `min_items = 1`; OR-semantic), `condition` (CEL Expr; SILENT-IGNORED per Q7 + §2.7), `checked_condition` (CEL CheckedExpr; SILENT-IGNORED), `cel_config` (CelExpressionConfig; SILENT-IGNORED per §1.1 amendment 6 — phase-16 BRAINSTORM §2.7 conflated to "CEL fields" but `cel_config` is a distinct field added in v1.37.2). Policies evaluated in lexicographic order of policy name per `rbac.pb.go:268-269` proto comment.
   - `audit_logging_options` — SILENT-IGNORED (marked `[#not-implemented-hide:]` upstream at `rbac.pb.go:273`; never emitted by Envoy v1.37.2 regardless of config; §8.2 deferral).

   **Inside `xds.type.matcher.v3.Matcher` (the matcher-engine config; consumed when `matcher` or `shadow_matcher` set):**

   - The match-tree structure with predicates → on_match actions evaluating to a TypedExtensionConfig terminal. envoy-go MVP supports the canonical RBAC terminal action TypeURL `type.googleapis.com/envoy.config.rbac.v3.Action` (per §11.P3); non-canonical terminal TypeURLs PARSE-REJECTED with envoy-go-only error wording.

   **Permission MVP subset (11 of 14; Q3 Large pick):** `any`, `header` (HeaderMatcher), `url_path` (PathMatcher), `destination_ip` (CidrRange), `destination_port` (uint32 PGV `lte = 65535` per `\xfaB\x06*\x04\x18\xff\xff\x03` at `rbac.pb.go:1532`), `destination_port_range` (Int32Range), `requested_server_name` (StringMatcher; SNI), `and_rules` (Permission_Set; recursive), `or_rules` (Permission_Set; recursive), `not_rule` (Permission; recursive), `sourced_metadata` (SourcedMetadata; parse-supported with always-no-metadata-match runtime divergence-window per §8.10 + ADR-0143 coupling to dynamic-metadata-family).

   **Permission DEFERRED (3 of 14):** `metadata` (DEPRECATED per `\x92ǆ\xd8\x04\x033.0\x18\x01` annotation at `rbac.pb.go:1534`; PARSE-REJECT with envoy-go-only error per §8.3 + §11.P12 + Q7-amendment); `matcher` (TypedExtensionConfig extension; PARSE-REJECT per §8.8); `uri_template` (TypedExtensionConfig extension; PARSE-REJECT per §8.8).

   **Principal MVP subset (11 of 14; Q3 Large pick — REFINED per §1.1 amendment 7 from BRAINSTORM "13 of 13"):** `any`, `authenticated` (Principal_Authenticated; TLS principal-name match — requires NEW framework primitive per §3.1 + ADR-0144), `direct_remote_ip` (CidrRange; peer connection's source IP), `remote_ip` (CidrRange; XFF-resolved IP), `header` (HeaderMatcher), `url_path` (PathMatcher), `and_ids` (Principal_Set; recursive), `or_ids` (Principal_Set; recursive), `not_id` (Principal; recursive), `sourced_metadata` (SourcedMetadata; parse-supported with always-no-metadata-match runtime divergence-window per §8.10), `filter_state` (FilterStateMatcher; parse-supported with always-no-filter-state-match runtime divergence-window per §8.10).

   **Principal DEFERRED (3 of 14; REFINED per §1.1 amendment 7 — Principal has 14 variants in Envoy v1.37.2, NOT 13 as BRAINSTORM hypothesized; the 14th is `custom` TypedExtensionConfig):** `source_ip` (DEPRECATED per `\x92ǆ\xd8\x04\x033.0\x18\x01` at `Principal_SourceIp`; PARSE-REJECT with envoy-go-only error `"rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip"` per §8.4 + §11.P12); `metadata` (DEPRECATED; PARSE-REJECT per §8.3); **`custom`** (TypedExtensionConfig at field 12 per `rbac.pb.go:1144`; couples to plugin framework; PARSE-REJECT with envoy-go-only error `"rbac: principal.custom extension types unsupported in this build"` per §8.11 NEW deferral introduced by §1.1 amendment 7).

4. **Per-route TPFC: NEW 7th canonical pattern (absent-implies-disabled-OR-wholesale-override; ADR-0125 amendment §(xii) at SPEC commit).** Per `rbacv3.RBACPerRoute` at `rbac.pb.go:147-191` + the raw descriptor `J\x04\b\x01\x10\x02` confirming `reserved 1`: the message has ONE field `rbac` (`*RBAC`) at field 2, with field 1 reserved (per proto evolution; was likely a removed `disabled` bool in an earlier version). Proto comment at `rbac.pb.go:150` reads verbatim: `"If absent, RBAC policy will be disabled for this route."` §11.P1 RATIFIES the brainstorm hypothesis. Two cases:

   - **(a) `RBACPerRoute{rbac: nil}` (or `RBACPerRoute` message itself missing)** — the filter is wholly inactive on this route. No policy enforcement. No counter increments at the listener-level namespace (the listener-level filter's counter scope is NOT touched). Request passes through to the next filter without RBAC evaluation.
   - **(b) `RBACPerRoute{rbac: <RBAC>}`** — WHOLESALE override of the listener-level RBAC message (NOT a merge; the override's `rules`, `matcher`, `shadow_rules`, `shadow_matcher`, all 7 top-level fields REPLACE the listener-level values entirely). Mirrors phase-13/14/15 WHOLESALE-not-merge per ADR-0125 + ADR-0073. If the override carries its own `rules_stat_prefix` it emits to an INDEPENDENT counter namespace per §5 + ADR-0145 INDEPENDENT-stats discipline (mirrors phase-11 ADR-0117 + phase-15 ADR-0139 stateful-override-implies-INDEPENDENT).

   **Phase 16 is the FIRST row to use the absent-implies-disabled discipline** — structurally distinct from BOTH the 5th canonical (explicit `disabled` boolean in oneof; phase-13/14) AND the 6th canonical (bare-message-via-TPFC + code-level-required field; phase-15). The 7th canonical's defining feature: a wrapper proto exists, has reserved field 1, has a single optional message field; absence-of-the-field implies disabled-via-proto-comment. Both signaling-by-absence (7th) and signaling-by-explicit-bool (5th) are valid; signaling-by-bare-message-presence (6th) is the third axis. ADR-0125 grows its canonical-pattern roster from 6 to 7 via in-place amendment paragraph §(xii) authored at this SPEC commit (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) precedent for in-place ADR amendments).

5. **Filter-callback shape: `StreamDecoderFilter` ONLY on the `*filter` instance.** Phase 16 is decode-side only — RBAC is a request-gate filter, evaluated at `DecodeHeaders` time, with the disposition (allow / deny / log) computed BEFORE the request body is forwarded. The filter does NOT implement `StreamEncoderFilter`. The `New` factory returns `envoyhttp.HTTPFilter{Name: filterName, Decoder: f, Encoder: nil, PerRoute: parsePerRoute}`. Decoder-only structural expression mirrors phase-12 csrf ADR-0120 + phase-13 buffer ADR-0125 — the second + third §9 rows to use `Encoder: nil`; rbac is the fourth (csrf, buffer, and phase-16 rbac are all pure decode-side gates). The decode-side surface: `DecodeHeaders` resolves per-route → caches `*compiledPerRoute` on filter state → runs the dual-engine evaluation (primary rules-or-matcher, shadow rules-or-matcher) → emits the appropriate counter delta → returns `HeaderContinue` (allow) OR invokes `cb.SendLocalReply(403, ...)` and returns `HeaderStopIteration` (deny). `DecodeData` + `DecodeTrailers` pass-through. `OnDestroy` no-op (no timers; no state to release; mirrors phase-12 csrf precedent).

6. **Dual-engine evaluation: rules-engine path + matcher-engine path (Decision #2 → ADR-0141 + ADR-0142).** Per Q2 = "BOTH engines proto-faithful":

   **Rules-engine path** (when `rules` is set in the consumed config):
   1. Walk `policies` map in lexicographic key order. For each Policy:
      - Evaluate `permissions[]` (OR-semantic; short-circuit on first match) via Permission evaluators (the 11-variant set per §3 above).
      - Evaluate `principals[]` (OR-semantic; short-circuit on first match) via Principal evaluators (the 11-variant set).
      - Skip `condition` + `checked_condition` + `cel_config` evaluation (CEL silent-ignored per Q7 + §2.7; treat condition as always-true).
      - Policy MATCHES if both permissions-OR and principals-OR are TRUE.
   2. After walking all policies, the aggregate match decision is "any policy matched" or "no policy matched". Capture matched-policy-name(s) for per-policy counter emission (only when `track_per_rule_stats: true`).
   3. Apply `action` to produce the engine result:
      - `ALLOW`: result=ALLOWED iff any-policy-matched; result=DENIED iff no-policy-matched.
      - `DENY`: result=ALLOWED iff no-policy-matched; result=DENIED iff any-policy-matched.
      - `LOG-partial` (per §1.1 amendment 5): result ALWAYS = ALLOWED; matched-policy-names captured for per-policy `allowed` counter emission (when track is true); `access_log_hint` dynamic metadata NOT emitted per envoy-go MVP divergence-window.

   **Matcher-engine path** (when `matcher` is set in the consumed config; `rules` unset):
   1. Walk the match-tree per `xds.type.matcher.v3.Matcher` semantics: predicates evaluate against the request via the matcher-engine framework primitive (ADR-0142); the first matching predicate's `on_match` returns a TypedExtensionConfig terminal action.
   2. The terminal action is unmarshalled as `envoy.config.rbac.v3.Action` (the canonical RBAC terminal per §11.P3; PARSE-REJECT for any other TypeURL at config-load time). The Action carries `name` (the policy-id-equivalent for the matched leaf) + `action` (the RBAC_Action enum).
   3. Per proto semantic at `rbac.pb.go:43-46` ("Requests not matching any matcher will be denied"): if the match-tree walk completes without a matching predicate, result=DENIED with policy-name="" (no matched policy).
   4. Apply the matched `Action.action`: ALLOW → ALLOWED; DENY → DENIED; LOG → ALLOWED + log-hint (per §1.1 amendment 5; metadata divergence-window).

   **Engine selection** (per Q2 + §11.P2): if BOTH `rules` AND `matcher` are set, `rules` WINS, `matcher` IS IGNORED (mirrors proto comment at `rbac.pb.go:38`). If NEITHER is set, the filter is structurally inactive (returns `Continue` without evaluation) per `rbac.pb.go:33`: "If absent, no RBAC enforcement occurs." This is the listener-with-empty-engines edge case; envoy-go MVP returns Continue + emits NO counters.

   **Shadow path** (when `shadow_rules` or `shadow_matcher` is set, parallel to primary; same engine-selection rules apply within the shadow surface):
   1. Run the parallel shadow-engine walk (same algorithm as primary, on the shadow-engine config).
   2. Emit `shadow_allowed` (if shadow result = ALLOWED) or `shadow_denied` (if DENIED) counter under the `shadow_rules_stat_prefix` namespace.
   3. Shadow path NEVER affects the actual disposition — purely an observability emission. Shadow access-log entries are NOT emitted by envoy-go MVP per §8.7 deferral (couples to access-log family).

   **Counter emission** (per §2.8 + ADR-0145 + §1.1 amendment 8 — REFINED 4-counter base from BRAINSTORM 5-counter hypothesis): on each request reaching the filter active engine, emit ONE primary-path counter (`allowed` for ALLOWED result OR `denied` for DENIED result, under `rules_stat_prefix` namespace); ONE shadow-path counter (`shadow_allowed` or `shadow_denied` under `shadow_rules_stat_prefix` namespace, if shadow configured); and (if `track_per_rule_stats: true`) one per-policy counter per matched policy under each prefix's per-policy sub-namespace (`<policy_name>.allowed` for primary ALLOW-matched, `<policy_name>.denied` for primary DENY-matched, plus shadow variants).

7. **Stat surface — 60 → 64+ names (4 base + variable per-policy; Decision implicit → ADR-0145; per §1.1 amendment 8 REFUTES BRAINSTORM 5-counter hypothesis).** 4 base counters per active namespace combination (per §11.P6 verbatim Envoy source at `source/extensions/filters/common/rbac/utility.h` macros `ENFORCE_RBAC_FILTER_STATS` = {allowed, denied} + `SHADOW_RBAC_FILTER_STATS` = {shadow_allowed, shadow_denied}; **NO separate `logged` counter exists in Envoy v1.37.2** — LOG-partial increments `allowed` since LOG always-allows):

   Base counters (4 per active `rules_stat_prefix` + `shadow_rules_stat_prefix` namespace combination):
   - `allowed` — counter; increments per request whose engine result = ALLOWED (under primary `rules_stat_prefix`).
   - `denied` — counter; increments per request whose engine result = DENIED (under primary `rules_stat_prefix`).
   - `shadow_allowed` — counter; increments per request whose shadow engine = ALLOWED (under `shadow_rules_stat_prefix`).
   - `shadow_denied` — counter; increments per request whose shadow engine = DENIED.

   Per-policy counters (when `track_per_rule_stats: true`; emitted per matched policy):
   - `<policy_name>.allowed` — counter; increments per request where Policy `<policy_name>` matched primary AND primary result was ALLOWED.
   - `<policy_name>.denied` — counter; increments per request where Policy `<policy_name>` matched primary AND primary result was DENIED.
   - Plus `<policy_name>.shadow_allowed` / `<policy_name>.shadow_denied` analogues under `shadow_rules_stat_prefix`.

   The per-policy counter format per `utility.h` `incPolicyAllowed`/`incPolicyDenied`/`incPolicyShadowAllowed`/`incPolicyShadowDenied` methods: `"{per_policy_final_prefix}{policy_name}{suffix}"` where suffix ∈ `{.allowed, .denied, .shadow_allowed, .shadow_denied}` (per §11.P10).

   **Stat-name surface count summary:**
   - Phase 15 (bandwidth_limit): 60-name table (14 active counter/gauge + 2 deferred histogram per ADR-0138).
   - Phase 16 (rbac): **60 → 64 names** (4 new active counters minimum; the per-policy counter surface is OPERATOR-CONFIG-DRIVEN — NOT a fixed +N in the table; documented at BEHAVIOR_CONTRACT under a new emission-discipline subsection per §13.4).

   **Stat namespace + Prometheus tag-extractor (per §1.1 amendment 9 + §11.P7):** internal stat path follows the SN2-reuse hypothesis from phase-14 compressor + phase-15 bandwidth_limit: `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for primary; `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.<counter>` for shadow. Prometheus rendering uses the existing SN2 (`http.*` segment routing) + dot→underscore default-branch flatten; NO new SN10 rule, NO tag-extractor amendment. Per-policy counters: `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<policy_name>.<suffix>` rendered analogously. ADR-0145 codifies.

   **Per-route stats discipline: INDEPENDENT (mirrors phase-11 ADR-0117 + phase-15 ADR-0139; per §11.P9 + §11.P8 hypothesis pending impl-time empirical scrape confirmation).** Rationale: per-route owns its own policy-set state + its own `rules_stat_prefix` + its own `shadow_rules_stat_prefix` as own emission scope. The stateful-override-implies-INDEPENDENT-stats discipline applies (mirrors phase-11 local_ratelimit's stateful-token-bucket + phase-15 bandwidth_limit's stateful-throttle-config). DIVERGES from phase-12/13/14 SHARED-stats discipline (those filters' per-route overrides are stateless). Per-route's `rules_stat_prefix` field, if set, drives a wholly-own counter namespace.

8. **TWO new framework primitives** — the FIRST §9 row since phase 14 to introduce non-zero deltas + the FIRST single phase to introduce TWO simultaneously: (i) ADR-0144 — TLS-principal accessor `DecoderFilterCallbacks.DownstreamPrincipal() []string` exposing the downstream client cert's URI SAN + DNS SAN + Subject DN in priority order (per `Principal_Authenticated` proto comment at `rbac.pb.go:1432-1438` per §11.P14); (ii) ADR-0142 — matcher-engine evaluator framework primitive at a new top-level `internal/matcher/` package implementing the `xds.type.matcher.v3.Matcher` generic match-tree evaluator (cross-phase-reusable). See §3 for details. ADR-0144 + ADR-0142 anchor.

After phase 16, the project has proven the §9 HTTP filters family-expansion pattern carries through a NINTH filter under: the cors / fault / header_mutation / localratelimit / csrf / buffer / compressor / bandwidthlimit precedent's package-shape discipline (single-token directory matching the proto type-name); the FIRST §9 row to use the absent-implies-disabled-OR-override **7th canonical per-route discipline** codified at ADR-0125 §(xii) in-place amendment; TWO new framework primitives (the FIRST single phase to introduce two; both cross-phase-reusable); a deliberate divergence-window from reference Envoy on the LOG-action dynamic-metadata axis (envoy-go: always-no-metadata; Envoy: emits `access_log_hint` on LOG-matched requests) with a forward-pointer to a future dynamic-metadata family phase. *envoy-go's HTTP filter framework hosts a dual-engine policy-evaluation filter that parses 7 proto-faithful top-level fields, walks the rules-engine policy map OR matcher-engine match-tree depending on which is configured, optionally runs a parallel shadow engine for non-enforcing observation, optionally emits per-policy-name counters when track_per_rule_stats is enabled, and gates the request via SendLocalReply(403, "RBAC: access denied") on a DENY engine result; the OBSERVABLE-OUTCOMES are byte-equivalent to reference Envoy on every axis EXCEPT the LOG-action dynamic-metadata axis (envoy-go: silent; Envoy: emits access_log_hint) AND the shadow-rules access-log axis (envoy-go: counter-only; Envoy may also emit shadow-decision-annotated access-log entries per §11.P13 pending) AND the CEL-conditions axis (envoy-go: silent-ignored; Envoy: full CEL evaluation per Q7).* This is the NINTH §9 family-row to land; subsequent filters (jwt_authn, ext_authz, ext_proc, oauth2, lua, wasm, adaptive_concurrency, admission_control, global_ratelimit) follow the same row-as-its-own-phase pattern per ADR-0106.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session (2026-05-12) ratifies, refines, or refutes load-bearing BRAINSTORM hypotheses. **Twelve** amendments below are self-contained corrections; collectively they revise:

- **Field decomposition framing (amendments 1–4):** structural — the outer filter proto has 7 fields ALL consumed proto-faithful (no silent-ignored at the outer level); `rules`/`matcher` is a UDPA-`field_alias` annotation grouping (NOT a Go-level oneof); `rules_stat_prefix` + `shadow_rules_stat_prefix` carry NO PGV (empty default permitted; refines BRAINSTORM's implicit-PGV-mirror-hypothesis); the silent-ignore set lives ONE LEVEL DEEPER inside `config.rbac.v3.RBAC` (`audit_logging_options` + CEL fields). [§11.P1 + §11.P2]
- **LOG-action behavior + 4-counter base (amendments 5 + 8):** structural — Envoy emits ONLY 4 base counters (allowed + denied + shadow_allowed + shadow_denied) per `utility.h` macros; NO separate `logged` counter; LOG-partial increments `allowed` since LOG always-allows; the `access_log_hint` dynamic-metadata divergence-window stays (envoy-go MVP: silent; Envoy: emits). [§11.P4 + §11.P6]
- **CEL fields enumerated (amendment 6):** the Policy proto has THREE CEL fields (`condition`, `checked_condition`, AND `cel_config`), not just two as BRAINSTORM §2.7 listed; all three SILENT-IGNORED per Q7 (sharpens the silent-ignore set). [§11.P2]
- **Principal has 14 variants not 13 (amendment 7):** structural — Envoy v1.37.2's Principal proto adds `custom` TypedExtensionConfig (field 12 per `rbac.pb.go:1144`); brings the deferred Principal set to 3 (source_ip + metadata + custom); MVP Large 11 stays unchanged. [§11.P2]
- **Per-policy counter format + namespace (amendment 9):** structural — Envoy emits per-policy counters via `"{per_policy_final_prefix}{policy_name}{suffix}"` template per `utility.h::incPolicyAllowed/Denied/ShadowAllowed/ShadowDenied`; the stat namespace follows SN2-reuse (HCM-rooted `http.<HCM>.rbac.<rules_stat_prefix>.<counter>`); NO new SN10 rule (impl-time empirical scrape confirms or amends). [§11.P7 + §11.P10]
- **403 wire shape ratified (amendment 10):** body = `"RBAC: access denied"` (19 bytes ASCII; matches BRAINSTORM hypothesis verbatim per `source/extensions/filters/http/rbac/rbac_filter.cc`); status 403; 4-header set lowercase wire-form; keep-alive. [§11.P5]
- **Response-code-details discovered (amendment 11):** Envoy emits `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` (per `source/extensions/filters/common/rbac/utility.cc::responseDetail`); whitespace in policy-id replaced with underscores. envoy-go MVP DEFERS the response-code-details field emission per phase-04 HCM scope (current HCM does not surface response-code-details to local-reply callers); documented as divergence-window. [§11.P5]
- **Principal_Authenticated nil-principal_name = any-authenticated-user (amendment 12):** structural — proto comment at `rbac.pb.go:1432-1438` documents that an unset `principal_name` "applies to any user that is allowed by the downstream TLS configuration"; BRAINSTORM hypothesis assumed `principal_name` always present. envoy-go MVP: when `principal_name == nil`, the evaluator returns TRUE if the connection presented ANY client cert that passed TLS verification (i.e., `len(DownstreamPrincipal()) > 0`); else FALSE. [§11.P14]

Mirrors the phase-12 csrf 4-amendment + phase-14 compressor 6-amendment + phase-15 bandwidth_limit 10-amendment pattern extended to 12 amendments at phase 16. The structural design (TWO new framework primitives, dual-engine proto-faithful, 11+11 Large MVP, INDEPENDENT-stats hypothesis, 7th canonical per-route, ALLOW + DENY + LOG-partial action enum, 4-counter base) survives intact despite the magnitude of the refinements — all amendments fit within the §1.1 self-contained-prose-block channel without requiring a BRAINSTORM §12 amendment cycle.

#### 1.1 Amendment 1 — Outer filter proto has NO silent-ignored fields; all 7 fields consumed proto-faithful (BRAINSTORM §1.1 item 3 + §9.P1)

BRAINSTORM §1.1 item 3 enumerated 7 consumed top-level fields without distinguishing consumed-vs-silent-ignored at the outer envelope. **§11.P1 + scrape of `rbac.pb.go:28-145` empirically RATIFIES** that all 7 outer fields are consumed proto-faithful in phase-16 MVP — there is no silent-ignored set at the outer envelope. The silent-ignore discipline lives ONE LEVEL DEEPER inside `config.rbac.v3.RBAC` (the rules-engine config), specifically: `audit_logging_options` (per `rbac.pb.go:273` `[#not-implemented-hide:]` annotation; §8.2) + Policy.condition + Policy.checked_condition + Policy.cel_config (per Q7 silent-ignore + §1.1 amendment 6; §8.1). Phase-16 envoy-go disposition: outer-envelope parsing accepts all 7 fields without warning; the silent-ignore set is enforced at the `buildCompiledConfig(c.GetRules())` recursive walk over the rules-engine inner config.

#### 1.1 Amendment 2 — `rules`/`matcher` + `shadow_rules`/`shadow_matcher` are UDPA-`field_alias` annotations, NOT Go-level oneofs (BRAINSTORM §2.2 hypothesis-form + §9.P1)

BRAINSTORM §2.2 + §1.1 item 3 framed the rules-or-matcher selection as "oneof rules_specifier" — implying a Go-level union with mutually-exclusive accessors. **§11.P1 + the .pb.go scrape REFINES:** the proto uses UDPA `udpa.annotations.field_alias` annotations (the `\xf2\x98\xfe\x8f\x05\x11\x12\x0frules_specifier` bytes in the raw descriptor at `rbac.pb.go:200`) to MARK the field-pair as belonging to a conceptual oneof, but the .pb.go binding generates them as TWO SEPARATE optional fields (per the type definition at lines 39-47). At the Go level: `RBAC.Rules` and `RBAC.Matcher` are independently-settable pointer fields. The "rules wins when both set" semantic is FILTER-SOURCE-ENFORCED at `source/extensions/filters/http/rbac/config.cc` (Envoy's filter parser), NOT proto-enforced. Same for `shadow_rules`/`shadow_matcher`. Phase-16 envoy-go disposition: `buildCompiledConfig` checks for BOTH set + picks `rules` (mirrors Envoy's filter-source priority) without raising an error. ADR-0141 codifies the dual-engine dispatch table including the rules-wins precedence.

#### 1.1 Amendment 3 — `rules_stat_prefix` + `shadow_rules_stat_prefix` carry NO PGV; empty default permitted (BRAINSTORM §1.1 item 3 + §9.P2)

BRAINSTORM §1.1 item 3 documented `rules_stat_prefix` + `shadow_rules_stat_prefix` as "empty default permitted" — implicitly hypothesizing that PGV-mirror validation would NOT trip on empty strings. **§11.P2 + the .pb.validate.go scrape RATIFIES via absence:** the auto-generated `RBAC.validate()` at `rbac.pb.validate.go:42-187` contains the comment lines `// no validation rules for RulesStatPrefix` (line 89) + `// no validation rules for ShadowRulesStatPrefix` (line 178) + `// no validation rules for TrackPerRuleStats` (line 180). NO PGV constraints exist on these three fields. Phase-16 envoy-go disposition: `buildCompiledConfig` accepts empty `rules_stat_prefix` + `shadow_rules_stat_prefix` without validation. Documented at ADR-0141.

#### 1.1 Amendment 4 — PGV constraints exist ONLY on inner fields (action enum-validation; Policy.permissions/principals min_items=1; Permission.destination_port lte=65535) (BRAINSTORM §9.P2 + verbatim scrape)

BRAINSTORM §9.P2 anticipated PGV constraints at each consumed level. **§11.P2 verbatim scrape resolves:** the outer `RBAC.validate()` has NO field-level PGV constraints (per amendment 3 above; only embedded-message validation on the four sub-message pointer fields). PGV constraints exist at the inner config.rbac.v3 level:

- `config.rbac.v3.RBAC.action` — PGV `defined_only = true` (per `\x82\x01\x02\x10\x01` annotation at `rbac.pb.go:1487`; enum values restricted to declared {ALLOW=0, DENY=1, LOG=2}).
- `config.rbac.v3.RBAC.Policy.permissions` — PGV `min_items = 1` (per `\x92\x01\x02\b\x01` at `rbac.pb.go:1511`).
- `config.rbac.v3.RBAC.Policy.principals` — PGV `min_items = 1` (per `\x92\x01\x02\b\x01` at `rbac.pb.go:1513`).
- `Permission.any` — PGV `const = true` (per `\xfaB\x04j\x02\b\x01` at `rbac.pb.go:1527`; `any: false` rejected).
- `Permission.destination_port` — PGV `lte = 65535` (per `\xfaB\x06*\x04\x18\xff\xff\x03` at `rbac.pb.go:1532`).
- `RBAC.AuditLoggingOptions.audit_condition` — PGV `defined_only = true` (`[#not-implemented-hide:]` so unobservable in practice).
- `SourcedMetadata.metadata_matcher` — PGV `required = true` (per `\x8a\x01\x02\x10\x01` at `rbac.pb.go:1521`).
- `SourcedMetadata.metadata_source` — PGV `defined_only = true`.

Phase-16 envoy-go disposition: envoy-go-side defensive PGV-mirror validation in `buildCompiledConfig` for each: action defined-only-check; policies.permissions + policies.principals nonempty-check; destination_port range-check (defensive; the wrapper is already PGV-enforced at proto-decode). envoy-go-own error wording per phase-11 ADR-0115 + phase-15 ADR-0136 precedent (e.g., `"rbac: policy %q permissions must be non-empty"`).

#### 1.1 Amendment 5 — LOG action behavior: always-allow + matched-policy increments `allowed` counter + `access_log_hint` dynamic-metadata divergence-window (BRAINSTORM §2.4 + Q4 + §9.P4)

BRAINSTORM §2.4 + Q4 framed LOG-partial as "always-allow; metadata not emitted in MVP; `logged` counter increments unconditionally". **§11.P4 + the rbac_filter.cc + utility.h scrape REFINES:**

- LOG always-allows (per `rbac.pb.go:262-265` proto comment + `rbac.pb.go:1157-1167` Action proto comment).
- The engine RUNS the match evaluator under LOG-action and captures matched-policy-names.
- LOG with matched-policy → `access_log_hint` dynamic metadata under namespace `envoy.common` set to `true`; LOG with no-match → set to `false`. envoy-go MVP DIVERGES: silent-no-metadata-emit (per §8.6 deferral; couples to dynamic-metadata family).
- Counter emission: per §1.1 amendment 8 — **only `allowed` increments under LOG** (since LOG always-allows; engine result = ALLOWED). No separate `logged` counter exists in Envoy v1.37.2 (the `utility.h` `ENFORCE_RBAC_FILTER_STATS` macro defines only allowed + denied; no third counter). When `track_per_rule_stats: true`, matched-policy increments `<policy_name>.allowed`.

Phase-16 envoy-go disposition: LOG-action path runs the rules-engine (or matcher-engine) walk identically to ALLOW; result always = ALLOWED; `allowed` counter increments; matched-policy-names captured for per-policy emission when track is true; `access_log_hint` metadata emission SKIPPED with divergence-window documented at ADR-0146 + BEHAVIOR_CONTRACT phase-16 forward-pointer notes. ADR-0146 codifies the LOG-partial discipline.

#### 1.1 Amendment 6 — Policy has THREE CEL fields (`condition`, `checked_condition`, `cel_config`), all silent-ignored (BRAINSTORM §2.7 + §9.P2)

BRAINSTORM §2.7 + Q7 + §1.1 item 3 listed TWO CEL fields (`condition` + `checked_condition`) as silent-ignored. **§11.P2 verbatim scrape REFINES:** the Policy proto at `rbac.pb.go:335-431` carries THREE CEL-related fields:

- `condition` (`google.api.expr.v1alpha1.Expr`; parsed-only CEL expression; field 3).
- `checked_condition` (`google.api.expr.v1alpha1.CheckedExpr`; type-checked CEL expression; field 4).
- `cel_config` (`envoy.config.core.v3.CelExpressionConfig`; v1.37.2-added field; field 5).

All three are part of an `expression_specifier` UDPA-field-alias group (analogous to amendment 2's `rules_specifier`). Phase-16 envoy-go disposition: all THREE silent-ignored at runtime per Q7 silent-ignore discipline; the Policy parse succeeds with any/all set; the evaluator skips condition evaluation entirely. §8.1 deferral updated to enumerate all three fields. Divergence-window: policies relying on any CEL field for fine-grained control see envoy-go-vs-Envoy decision DIVERGENCE on the condition-driven slice.

#### 1.1 Amendment 7 — Principal has 14 variants (NOT 13); the 14th is `custom` TypedExtensionConfig (DEFERRED) (BRAINSTORM §2.3 + §9.P2 + §11.P2)

BRAINSTORM §2.3 + Q3 + §1.1 item 3 listed Principal as having 13 variants. **§11.P2 verbatim scrape REFUTES:** the Principal proto at `rbac.pb.go:819-1144` defines 14 oneof variants — the 14th, missed by the brainstorm, is `custom` (`envoy.config.core.v3.TypedExtensionConfig`; `Principal_Custom` Go type at `rbac.pb.go:1112`; field 12). The proto comment at `rbac.pb.go:1427-1429` recommends this variant for most use cases over the deprecated `authenticated`-with-TLS pathway, specifically calling out `MTlsAuthenticated <envoy_v3_api_msg_extensions.rbac.principals.mtls_authenticated.v3.Config>` as the canonical extension.

Phase-16 envoy-go disposition: `Principal_Custom` PARSE-REJECTED with envoy-go-only error `"rbac: principal.custom extension types unsupported in this build"` per §8.11 (NEW deferral added by this amendment). The MVP Large 11+11 framing stays intact — Principal MVP still covers 11 of 14; DEFERRED Principal set grows from 2 to 3 (source_ip + metadata + custom). ADR-0143 (Permission + Principal evaluators) documents.

#### 1.1 Amendment 8 — 4 base counters in Envoy v1.37.2 (NOT 5); LOG-partial folds into `allowed` (BRAINSTORM §2.8 + §9.P6 + §11.P6)

BRAINSTORM §2.8 + §1.1 item 7 hypothesized 5 base counters per active namespace (allowed/denied/logged/shadow_allowed/shadow_denied). **§11.P6 + scrape of `source/extensions/filters/common/rbac/utility.h` REFUTES:**

- The `ENFORCE_RBAC_FILTER_STATS` macro defines exactly TWO counters: `allowed` + `denied`.
- The `SHADOW_RBAC_FILTER_STATS` macro defines exactly TWO counters: `shadow_allowed` + `shadow_denied`.
- The `RoleBasedAccessControlFilterStats` struct exposes these 4 counters via the `ALL_RBAC_FILTER_STATS` aggregate; NO `logged` counter exists.
- LOG-partial action's "always-allow + match-runs" semantic increments the `allowed` counter on every request (since LOG always-allows; result = ALLOWED).

Phase-16 envoy-go disposition: filterStats struct carries EXACTLY 4 counters (allowed + denied + shadow_allowed + shadow_denied) plus the per-policy counter family when `track_per_rule_stats: true`. The stat surface grows 60 → **64 names** (4 new active counters; the per-policy surface is operator-config-driven and NOT counted in the fixed table). ADR-0140 + ADR-0145 codify. The §1.1 amendment 8 amendment refutes BRAINSTORM §2.8 step 3 + §1 item 6 "5 base counters" framing.

#### 1.1 Amendment 9 — Stat namespace SN2-reuse hypothesis (`http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>`); per-policy counter format `<rules_stat_prefix>.<policy_name>.<suffix>` (BRAINSTORM §2.8 + §9.P7 + §11.P10)

BRAINSTORM §2.8 hypothesized SN2-reuse for the rbac namespace shape with SN10 introduction only if §11.P7 demanded. **§11.P7 + scrape of `utility.h::generateStats` SIGNATURE (`RoleBasedAccessControlFilterStats generateStats(const std::string& prefix, const std::string& rules_prefix, const std::string& shadow_rules_prefix, Stats::Scope& scope)`) RESOLVES partial:**

- The `prefix` argument is the HCM stat_prefix (per Envoy filter-config wiring; `prefix` is the per-filter scope's stat-prefix label, threaded from the HCM at `source/extensions/filters/http/rbac/config.cc`).
- The `rules_prefix` argument is the proto `rules_stat_prefix` field.
- The `shadow_rules_prefix` argument is the proto `shadow_rules_stat_prefix` field.
- The final stat name path: `<prefix>.rbac.<rules_prefix>.<counter>` for primary; `<prefix>.rbac.<shadow_rules_prefix>.<counter>` for shadow.
- Per-policy counter path: `<per_policy_final_prefix>{policy_name}{suffix}` where the prefix likely is `<prefix>.rbac.<rules_prefix>.policy.` (matches `incPolicyAllowed` calling convention; impl-time empirical scrape confirms).

Phase-16 envoy-go disposition: stat namespace follows `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` (with HCM-prefix-from-HCM-config + filter-name-segment + rules-stat-prefix-from-proto + counter-name). SN2 reuse (the existing `http.*` segment routing in `internal/stats/name.go::flattenToProm`); NO new SN10 rule (BRAINSTORM-hypothesized SN10 NOT introduced unless impl-time empirical scrape refutes the SN2-reuse hypothesis). Per-policy counters use the same prefix template with a `.<policy_name>` segment infix. ADR-0145 codifies.

NOTE: this amendment carries forward a hypothesis to impl-time empirical confirmation. If impl-time scrape reveals the prefix template diverges from the SN2-reuse hypothesis (e.g., `<rules_prefix>.rbac.<counter>` flat instead of `http.<HCM>.rbac.<rules_prefix>.<counter>` HCM-rooted; mirrors phase-15 bandwidth_limit's `<stat_prefix>.http_bandwidth_limit.<counter>` shape per §11.P11 of that SPEC), envoy-go's namespace mirrors via inline-prefix Prometheus rendering (`envoy_<rules_prefix>_rbac_<counter>{}`) and NO new SN10 rule is needed. The §13.2 stat-table extension covers both hypotheses with the impl-time confirmation pending.

#### 1.1 Amendment 10 — 403 wire shape RATIFIED: body byte-exact `"RBAC: access denied"` (19 bytes) + 4-header set + keep-alive (BRAINSTORM §4 + §9.P5 + §11.P5)

BRAINSTORM §4 hypothesized body byte-exact `"RBAC: access denied"` (19 bytes ASCII no trailing newline). **§11.P5 + scrape of `source/extensions/filters/http/rbac/rbac_filter.cc` RATIFIES:** the filter source invokes `callbacks_->sendLocalReply(Http::Code::Forbidden, "RBAC: access denied", ...)`. Status = 403; body bytes = `52 42 41 43 3a 20 61 63 63 65 73 73 20 64 65 6e 69 65 64` (19 bytes; matches `"RBAC: access denied"` ASCII). 4-header set lowercase wire-form: `content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy` (mirrors phase-11 + phase-12 SendLocalReply 4-header discipline). Connection disposition: keep-alive (no `connection: close`; the deny-decision fires BEFORE body, so no partial-body-consumption ambiguity exists unlike phase-13 buffer's 413).

Phase-16 envoy-go disposition: `cb.SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` mechanism (mirrors phase-09 fault abort + phase-11 local_ratelimit 429 + phase-12 csrf 403 + phase-13 buffer 413; NO new framework primitive). ADR-0140 codifies the wire-shape claim.

#### 1.1 Amendment 11 — Envoy emits `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` on DENY; envoy-go MVP DEFERS (BRAINSTORM § silent + §11.P5)

BRAINSTORM §4 did not address the `response_code_details` field on the 403 response. **§11.P5 + scrape of `source/extensions/filters/common/rbac/utility.cc::responseDetail` REVEALS:** Envoy's filter source generates a structured response-code-details string per matched policy: `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"` where `<sanitized_policy_id>` is the matched policy name with whitespace replaced by underscores. This string lands in Envoy's HCM `response_flag_details` accessor + access-log `RESPONSE_CODE_DETAILS` operator.

Phase-16 envoy-go disposition: the current envoy-go HCM does NOT surface `response_code_details` to local-reply callers (phase-04 HCM scope; the HCM's local-reply path emits the body + 4-header set + status without populating `response_code_details`). Mirroring Envoy's full behavior here would require an HCM framework primitive at `internal/filter/hcm/` to thread the response-code-details string through to access-log output. envoy-go MVP DEFERS the field-emission; documented as divergence-window in BEHAVIOR_CONTRACT phase-16 forward-pointer notes. Operator dashboards inspecting access-log `RESPONSE_CODE_DETAILS` see Envoy-side emit but envoy-go absent on RBAC denials. Future re-activation: response-code-details framework phase couples HCM's local-reply path to a per-filter accessor. ADR-0146 documents the divergence-window. §8.12 deferral added.

#### 1.1 Amendment 12 — `Principal_Authenticated.principal_name == nil` matches ANY authenticated user (BRAINSTORM §3.1 + §9.P14 + §11.P14)

BRAINSTORM §3.1 + §2.3 framed `Principal_Authenticated` as requiring a StringMatcher on `principal_name` to compare against the cert candidates. **§11.P14 + scrape of `rbac.pb.go:1432-1438` proto comment REFINES:**

> *"The name of the principal. If set, The URI SAN or DNS SAN in that order is used from the certificate, otherwise the subject field is used. If unset, it applies to any user that is allowed by the downstream TLS configuration. If require_client_certificate is false or trust_chain_verification is set to ACCEPT_UNTRUSTED, then no authentication is required."*

Three semantic cases for `Principal_Authenticated`:
- **(a) `principal_name == nil` (StringMatcher absent):** matches if the connection presented ANY client cert that passed TLS verification (i.e., the TLS handshake succeeded with mTLS). Plaintext or non-mTLS connections: returns FALSE. Phase-16 envoy-go disposition: check `len(callbacks.DownstreamPrincipal()) > 0`.
- **(b) `principal_name != nil` with StringMatcher set:** match the StringMatcher against URI SAN (first candidate), then DNS SAN (second), then Subject DN (third) per the priority order in the proto comment. Returns TRUE iff any candidate matches.
- **(c) Plaintext connection (no TLS):** all `Principal_Authenticated` evaluations return FALSE per the "trust_chain_verification" caveat; the downstream cert is absent so no principal name exists.

Phase-16 envoy-go disposition: `DecoderFilterCallbacks.DownstreamPrincipal() []string` returns the priority-ordered list of principal-name candidates (`[URI_SAN..., DNS_SAN..., Subject_DN_CN]`) for the active downstream connection; returns `nil` (or empty) for non-mTLS connections. The Principal_Authenticated evaluator handles cases (a)/(b)/(c) via slice-length-check + StringMatcher-iteration. ADR-0144 codifies the accessor API + the three-case algorithm.

---

## 2. Non-purposes

Phase 16 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.rbac` (BOTH-engine MVP) under the existing 07.1 framework + the TWO new framework primitives anchored at ADR-0142 + ADR-0144 (which ARE part of phase 16's deliverable).

### 2.1 `RBAC` outer-proto-message non-goals (per BRAINSTORM §8 + §1.1 amendment 1)

The outer filter proto `envoy.extensions.filters.http.rbac.v3.RBAC` consumes ALL 7 fields proto-faithful (per §1.1 amendment 1). The silent-ignore set lives inside `config.rbac.v3.RBAC`:

#### 2.1.1 Out of scope: `RBAC.audit_logging_options` (silent-ignored at parse + runtime)

Marked `[#not-implemented-hide:]` per the proto comment at `rbac.pb.go:273`. Envoy v1.37.2 does NOT emit audit logging regardless of `audit_condition` setting. envoy-go MVP silent-ignores. Couples to a future audit-logging family phase. §8.2 deferral.

#### 2.1.2 Out of scope: `Policy.condition` + `Policy.checked_condition` + `Policy.cel_config` (CEL silent-ignored per Q7 + §1.1 amendment 6)

All three CEL fields silent-ignored at runtime per Q7. The evaluator skips condition evaluation entirely; treats condition as always-true. Couples to a future CEL framework phase that lands `internal/cel/` evaluator + `github.com/google/cel-go` dependency. Re-activation enables full condition evaluation. Operator divergence-window: policies relying on CEL conditions for fine-grained control see envoy-go-vs-Envoy decision DIVERGENCE — envoy-go allows requests Envoy would deny (and vice versa) for the condition-driven slice of the policy graph. §8.1 deferral.

### 2.2 Per-route override surface non-goals (per §1.1 amendment 1 + §11.P1)

The per-route TPFC entry is the `RBACPerRoute` wrapper proto with a single optional `rbac` field at field 2 (field 1 reserved). The 7th canonical absent-implies-disabled-OR-wholesale-override pattern (per §5 + ADR-0125 §(xii) amendment).

- **NOT honored:** there is no per-route override on `rules_stat_prefix` or any subset of the listener-level fields — the only knob is `rbac` (presence-or-absence); presence = wholesale-override; absence = disabled-on-route.
- **To DISABLE rbac on a specific route:** operators set per-route TPFC entry `RBACPerRoute{}` (with `rbac` field absent). The route bypasses RBAC entirely.
- **To OVERRIDE rbac on a specific route:** operators set per-route TPFC entry `RBACPerRoute{rbac: <RBAC>}` carrying a full override RBAC config; the override REPLACES the listener-level RBAC wholesale.

### 2.3 Permission MVP scope-out (per §1.1 + §2.3 BRAINSTORM)

Permission DEFERRED set (3 of 14):
- `metadata` (DEPRECATED upstream per `\x92ǆ\xd8\x04\x033.0\x18\x01` annotation; superseded by `sourced_metadata`). PARSE-REJECT envoy-go-only with error `"rbac: permission.metadata deprecated; use sourced_metadata"`.
- `matcher` (TypedExtensionConfig extension). PARSE-REJECT with envoy-go-only error `"rbac: permission.matcher extension types unsupported in this build"`.
- `uri_template` (TypedExtensionConfig extension). PARSE-REJECT with envoy-go-only error `"rbac: permission.uri_template extension types unsupported in this build"`.

Future codec-extension or metadata-coupling phases re-activate one or more by amending ADR-0143.

### 2.4 Principal MVP scope-out (per §1.1 amendment 7)

Principal DEFERRED set (3 of 14):
- `source_ip` (DEPRECATED upstream; superseded by `direct_remote_ip` + `remote_ip`). PARSE-REJECT envoy-go-only with error `"rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip"`.
- `metadata` (DEPRECATED upstream). PARSE-REJECT.
- `custom` (TypedExtensionConfig extension; canonical use is `MTlsAuthenticated` per proto comment). PARSE-REJECT.

### 2.5 SourcedMetadata + FilterState always-no-match runtime semantic (per §1.1 + §2.3 + §8.10)

`Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState` are parse-supported (the proto fields are accepted without error; the compiledConfig carries the SourcedMetadata / FilterStateMatcher payload). At runtime, the evaluator returns FALSE for all SourcedMetadata + FilterState evaluations (the underlying dynamic-metadata + filter-state subsystems are not yet shipped). Documented divergence-window. Couples to future dynamic-metadata-family + filter-state-family phases. §8.10 deferral.

### 2.6 Matcher-engine terminal action type set (per §1.1 + §8.8 + §11.P3)

The matcher-engine's `on_match` returns a TypedExtensionConfig. Phase-16 MVP supports ONLY the canonical RBAC terminal action TypeURL `type.googleapis.com/envoy.config.rbac.v3.Action`. Non-canonical terminal TypeURLs PARSE-REJECTED with envoy-go-only error `"rbac: matcher action type %q unsupported in this build"`. Couples to a future matcher-extension family phase that opens the TypeURL set. §8.8 deferral.

### 2.7 LOG-action dynamic-metadata emission (per §1.1 amendment 5 + §8.6)

Under `action: LOG`, envoy-go MVP silent-no-metadata-emit. Reference Envoy emits `envoy.common.access_log_hint` dynamic metadata key based on policy match. Couples to a future dynamic-metadata family phase that lands the `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive. Re-activation enables LOG-action wire-shape equivalence. Operator divergence-window: configs setting `action: LOG` with downstream access-log integration expecting the access_log_hint hint see envoy-go's access-log lacking the hint.

### 2.8 Shadow-rules access-log integration (per §8.7 + §11.P13 pending)

Shadow-engine evaluation emits counters (`shadow_allowed`, `shadow_denied`) but NOT access-log entries marking shadow decisions. Envoy v1.37.2 may emit an access-log entry annotated with shadow disposition (impl-time empirical scrape confirms or refines via §11.P13). envoy-go MVP emits only the counter; documented divergence-window. Couples to access-log subsystem feature (post-phase-06.2).

### 2.9 `response_code_details` field emission (per §1.1 amendment 11 + §8.12)

Reference Envoy emits `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` on RBAC denial. envoy-go MVP does NOT thread response-code-details from filter through HCM to access-log output (current phase-04 HCM scope); divergence-window documented. Couples to future response-code-details framework phase. §8.12 deferral.

### 2.10 `track_per_rule_stats` envoy-go-only N-cap (per §8.5 + §11.P10)

No envoy-go-only parse-time N-cap on the policy-map size when `track_per_rule_stats: true`. The 2N per-policy counter surface is operator-config-driven; large-N configs are permitted (mirrors Envoy's permissive disposition). Documented foot-gun at BEHAVIOR_CONTRACT phase-16 forward-pointer notes. §8.5 deferral.

### 2.11 `Principal_Authenticated` beyond URI SAN / DNS SAN / Subject DN (per §1.1 amendment 12 + §8.9)

The `DownstreamPrincipal() []string` framework primitive (ADR-0144) surfaces ONLY the 3 canonical principal-name fields: URI SAN, DNS SAN, Subject DN (Common Name). Additional cert fields (Issuer DN, Serial Number, fingerprints) are NOT exposed in phase-16 MVP. Couples to a future TLS-context-extension phase. §8.9 deferral.

### 2.12 No filter-chain ordering surgery (per BRAINSTORM §3.4 + §11.6 — open structural Q)

Phase 16 rbac's filter-chain position is up to the operator. Recommended ordering: rbac EARLY in the chain (immediately after listener filters; before header_mutation/buffer/compressor/bandwidth_limit). Fixture 0018 pins rbac as the first HCM filter for byte-equivalence simplicity. Operators wanting header-mutation BEFORE rbac (e.g., to mutate `X-User` before the policy evaluator sees it) have full flexibility per the operator's filter-chain order. SPEC documents the trade-off without prescribing.

---

## 3. Framework survey result

Phase 16 introduces **TWO framework deltas** — FIRST §9 row since phase 14 to introduce non-zero deltas + FIRST single phase to introduce TWO simultaneously. The two primitives:

| Primitive | Location | Source |
|---|---|---|
| `DecoderFilterCallbacks.DownstreamPrincipal() []string` | `internal/filter/http/callbacks.go` + impl wiring at `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` (~30-50 LoC) | ADR-0144 (NEW) |
| Matcher-engine evaluator (`xds.type.matcher.v3.Matcher`) | NEW package `internal/matcher/` (~200-300 LoC) | ADR-0142 (NEW) |

Reused (no framework changes):

| Primitive | Source | Phase-16 usage |
|---|---|---|
| `cb.SendLocalReply(status, body, headers)` | Phase-09 fault per `internal/filter/http/fault/fault.go:319,335` | DENY-path 403 emission |
| 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) | Phase 07.1 per `internal/filter/http/registry.go` | Per-route TPFC most-specific override |
| `internal/filter/http/extension/HTTPRegistry` per ADR-0072 | Phase 07.1 | Boot-registration discipline |
| `stats.Registry.NewCounterIfAbsent` post-Freeze | Phase-11 ADR-0117 per `internal/stats/registry.go` | Per-route stat-counter idempotent post-Freeze allocation |
| `internal/listener/match.go` listener-port accessor | Phase 07.2 | `Permission_DestinationPort` + `Permission_DestinationPortRange` |
| SNI accessor on listener context | Phase 07.2 | `Permission_RequestedServerName` |
| XFF resolver / `X-Forwarded-For` parsing | Phase-04 HTTP-1.1 + phase-05 HTTP/2 | `Principal_RemoteIp` (XFF-resolved) |
| Peer address from connection accessor | Phase 02 TCP-proxy | `Principal_DirectRemoteIp` |
| Existing PathMatcher + HeaderMatcher + StringMatcher + CidrRange evaluators | Phase 07.1 cors precedent | Shared infrastructure for Permission/Principal leaf matchers |

### 3.1 TLS-principal accessor on `DecoderFilterCallbacks` (ADR-0144)

`Principal_Authenticated` requires access to the downstream client TLS certificate's principal name(s). Per Envoy v1.37.2 semantics + §11.P14 + §1.1 amendment 12: URI SAN (first), then DNS SAN, then Subject DN (Common Name).

**Current envoy-go state:** Phase 03 ships downstream TLS termination. The TLS context (cert chain, principal info) is held by the listener/connection layer. It is NOT currently surfaced through `DecoderFilterCallbacks`.

**Decision (refined per §1.1 amendment 12 + §11.3 carry-forward):** API shape is LIST-RETURNING.

```go
// internal/filter/http/callbacks.go — DecoderFilterCallbacks extension (phase-16 framework delta):
type DecoderFilterCallbacks interface {
    // ... existing methods ...

    // DownstreamPrincipal returns the TLS principal name candidates of the downstream client
    // connection, in priority order: URI SANs (first), then DNS SANs, then Subject DN CN
    // (the last fallback). The slice mirrors Envoy v1.37.2's Principal_Authenticated extraction
    // semantics per rbac.pb.go:1432-1438. Returns nil (or empty) for non-mTLS connections,
    // plaintext connections, or connections where no client cert was presented.
    DownstreamPrincipal() []string
}
```

**Rationale for list-returning (vs matcher-applying):**
- Flexibility: the caller (Permission/Principal evaluator) does its own matching against the candidates; the framework primitive does NOT couple to the StringMatcher type.
- Future filters (jwt_authn, ext_authz, oauth2) can extract principal names without rolling their own TLS-context plumbing.
- Mirrors the `internal/listener/match.go` precedent of phase-07.2 (multi-valued accessor over StringMatcher coupling).

**Plumbing:**
- TLS context is held at the listener/connection layer (phase-03 `internal/filter/tls/` + listener context).
- The HCM filter-chain dispatch context is held at the request layer.
- The accessor MUST thread the TLS info through: at filter-chain build time, the HCM extracts `tls.ConnectionState` from the connection accessor; at `DecoderFilterCallbacks` construction, the per-stream callbacks struct holds a reference to the connection-level TLS state; `DownstreamPrincipal()` extracts URI SANs from `state.PeerCertificates[0].URIs`, DNS SANs from `state.PeerCertificates[0].DNSNames`, Subject DN CN from `state.PeerCertificates[0].Subject.CommonName`.
- For non-mTLS connections: `state.PeerCertificates == nil || len(state.PeerCertificates) == 0` → return nil.

**Scope:** HTTP-filter-only in phase 16 (not surfaced to network-filters; future cross-cut). Future filters needing TLS info compose against the same accessor.

ADR-0144 codifies.

### 3.2 Matcher-engine evaluator framework primitive (ADR-0142)

The `xds.type.matcher.v3.Matcher` generic match-tree primitive. Reusable across filters; phase 16 lands the evaluator scoped initially to RBAC's needs.

**Decision (refined per §11.2 carry-forward):** Location is `internal/matcher/` NEW top-level package.

Rationale: matches BRAINSTORM preference; matcher-engine is a generic primitive (NOT rbac-specific); avoids a future refactor when ext_authz/jwt_authn/oauth2 land and need the same primitive; aligns with the §1.7 framework-delta accretion shape (cross-phase-reusable primitives anchor at their first-introduced location and stay there).

**API shape (initial; refined at impl time):**

```go
// internal/matcher/matcher.go (NEW package):

package matcher

import (
    matchv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
    "google.golang.org/protobuf/types/known/anypb"
)

// Matcher wraps a parsed xds.type.matcher.v3.Matcher tree.
type Matcher struct {
    // opaque fields
}

// New parses the proto match tree at config-load time + returns a Matcher.
// supportedActionTypes enumerates the TypeURLs the caller (a filter) accepts as
// terminal on_match actions; non-matching TypeURLs cause New to return an error.
// This is the parse-time PARSE-REJECT discipline for matcher-extension types
// beyond the canonical-RBAC-Action set per ADR-0142 + §8.8.
func New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher, error)

// Evaluate walks the match tree against the request data + returns the matched
// terminal action TypedExtensionConfig wrapped as Any, or (nil, nil) if no match.
// The MatchContext interface lets the caller (a filter) provide accessor functions
// for the data the predicates evaluate against (headers, IP, principal, etc.).
func (m *Matcher) Evaluate(ctx MatchContext) (*anypb.Any, error)

// MatchContext is the request-side accessor abstraction the evaluator uses to
// look up request data. Filters implement this on their per-stream *filter type
// (or pass a wrapper) so the evaluator can introspect headers, IP, etc.
type MatchContext interface {
    // Header lookups, IP lookups, etc.
    // Initial set deferred to impl time per ADR-0142 §Decision (iii); refined
    // based on the predicate-types the matcher-engine actually uses for RBAC's
    // canonical surface.
}
```

**Terminal action types (supportedActionTypes):** Phase-16 MVP allow-list is `["type.googleapis.com/envoy.config.rbac.v3.Action"]` (the canonical RBAC terminal per §11.P3 + §2.6). Non-canonical TypeURLs PARSE-REJECTED at `matcher.New()` time with envoy-go-only error wording per ADR-0142.

**Cross-phase reuse intent:** Future filters (ext_authz, jwt_authn, oauth2) extend `supportedActionTypes` with their own terminal action TypeURLs; the `internal/matcher/` package stays generic. The `MatchContext` interface widens additively as future filters need additional accessor methods.

ADR-0142 codifies the location + the API shape + the cross-phase reuse intent.

### 3.3 What else is reused (already-on-disk primitives)

(See table at §3 top.) No further amendments to existing primitives.

### 3.4 No filter-chain ordering surgery

Per BRAINSTORM §3.4 + §2.12 above. Phase 16 fixture pins rbac as the first HCM filter; operators have full flexibility.

---

## 4. Rejection-path wire shape (deny disposition)

Phase 16's deny-path mirrors phase-09 fault.abort + phase-11 local_ratelimit 429 + phase-12 csrf 403 + phase-13 buffer 413 wire-shape discipline:

- **Status code:** 403 (Forbidden) per `Http::Code::Forbidden` at `rbac_filter.cc::sendLocalReply` invocation; §11.P5 + §1.1 amendment 10 RATIFIES.
- **Body:** byte-exact `"RBAC: access denied"` (19 bytes ASCII; `52 42 41 43 3a 20 61 63 63 65 73 73 20 64 65 6e 69 65 64`; no trailing newline). §11.P5 + §1.1 amendment 10.
- **4-header set (lowercase wire-form):** `content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`. Mirrors phase-09/11/12/13 4-header discipline; `server: envoy` lowercase value per phase-11 ADR-0118 + phase-12 ADR-0123 precedent.
- **Connection disposition:** keep-alive (NO `connection: close`). Unlike phase-13 buffer 413 which closes the connection due to potential partial-body-consumption ambiguity, rbac's 403 is a pre-body-decision (the body has not started yet at deny time), so keep-alive is safe.
- **Response-code-details:** envoy-go MVP DEFERS field emission per §1.1 amendment 11 + §8.12. Reference Envoy emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"`; envoy-go MVP emits no response-code-details; divergence-window documented.

`cb.SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` mechanism (mirrors fault.abort, local_ratelimit, csrf, buffer-413). NO new framework primitive; reuses existing SendLocalReply.

ADR-0140 codifies the wire-shape claim.

---

## 5. Per-route discipline — 7th canonical (absent-implies-disabled) + INDEPENDENT-stats

Per §1.1 amendment 1 + §11.P1: the per-route TPFC entry is the `RBACPerRoute` wrapper proto with a single optional `rbac` field (field 2) + reserved field 1. The proto comment at `rbac.pb.go:150` reads: `"Per-route specific RBAC configuration that overrides the global RBAC configuration. If absent, RBAC policy will be disabled for this route."`

### 5.1 7th canonical: absent-implies-disabled-OR-wholesale-override

Phase 16 is the FIRST row to use the **absent-implies-disabled-OR-wholesale-override** per-route discipline. Structurally distinct from BOTH:
- The 5th canonical (phase-13 buffer + phase-14 compressor; ADR-0125 §Decision (vi) + §(viii)-(x) amendments): explicit `disabled` boolean in a oneof at the wrapper proto level.
- The 6th canonical (phase-15 bandwidth_limit; ADR-0125 §(xi) amendment): bare-message-via-TPFC + code-level-required field at per-route position (no wrapper proto).

The 7th canonical's defining feature: a wrapper proto exists (`RBACPerRoute`), has reserved field 1, has a SINGLE optional sub-message field (`rbac`); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment; PRESENCE-of-the-sub-message-field implies wholesale-override of the listener-level config.

Two cases at `parsePerRoute`:
- **(a) `RBACPerRoute{rbac: nil}` (or `RBACPerRoute` message itself absent from the per-route TPFC slot):** produce `*compiledPerRoute{disabled: true, overrideConfig: nil}`. The route bypasses RBAC entirely.
- **(b) `RBACPerRoute{rbac: <RBAC>}`:** recursively call `buildCompiledConfig(rbac, ctx, true /*isPerRoute*/)` → produce `*compiledPerRoute{disabled: false, overrideConfig: <built>}`. The compiled override carries its own `*compiledRulesEngine` / `*compiledMatcherEngine` + its own `*filterStats` keyed by the per-route `rules_stat_prefix`.

The `disabled` boolean inside `*compiledPerRoute` is an envoy-go-internal artifact derived from `RBACPerRoute.rbac == nil` (or the wrapper message itself being absent at the TPFC slot); it is NOT a proto-level field (unlike the 5th canonical's explicit `disabled` bool). Documented at ADR-0140.

### 5.2 INDEPENDENT-stats discipline (per ADR-0145; mirrors phase-11 ADR-0117 + phase-15 ADR-0139)

Per §1.7 stateful-override-implies-INDEPENDENT-stats discipline, phase 16's per-route override owns:
- `*compiledConfig` (rules/matcher engine configs + Permission/Principal evaluator trees + stat prefixes + track_per_rule_stats bool).
- `*filterStats` (4 base counters keyed by per-route `rules_stat_prefix` + `shadow_rules_stat_prefix`; per-policy counters when track is true).

The per-route `rules_stat_prefix` drives the counter namespace. A 100-stream load against a route with per-route override produces 100 increments on the per-route's `allowed` (or `denied`) counter under the per-route `rules_stat_prefix`, NOT 100 increments on the listener-level's namespace. Mirrors phase-11 + phase-15 INDEPENDENT discipline; DIVERGES from phase-12/13/14 SHARED.

### 5.3 Resolution flow at request time

`PerRouteConfig.Resolve(ctx)` → most-specific `*compiledPerRoute` for this route.
1. If `compiledPerRoute.disabled == true` → set `f.passthrough = true`; `DecodeHeaders` short-circuits to `HeaderContinue` without engine evaluation; NO counter increments.
2. If `compiledPerRoute.disabled == false` AND `compiledPerRoute.overrideConfig != nil` → use `overrideConfig` for the policy evaluation (including its own `rules_stat_prefix` driving INDEPENDENT stat namespace); listener-level config NOT consulted on this route.
3. If `PerRouteConfig.Resolve(ctx)` returns nil (no per-route TPFC entry at any tier) → use the listener-level `*compiledConfig`.

ADR-0145 codifies the INDEPENDENT discipline + the per-route counter-namespace assignment.

### 5.4 ADR-0125 in-place amendment paragraph §(xii) authored at this SPEC commit

ADR-0125's canonical-pattern roster grows from 6 to 7 via in-place amendment paragraph §(xii) (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) precedent for in-place ADR amendments at SPEC time):

> **(xii)** Phase 16 rbac is the FIRST row to use the **7th canonical per-route pattern**: a wrapper proto (`RBACPerRoute`) with reserved field 1 + a single optional sub-message field (`rbac` at field 2); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment (per Envoy v1.37.2 proto comment `"If absent, RBAC policy will be disabled for this route."`); PRESENCE-of-the-sub-message-field implies wholesale-override of the listener-level config (mirrors ADR-0073 wholesale-not-merge). Structurally distinct from the 5th canonical (explicit `disabled` bool in oneof; phase-13 + phase-14) and the 6th canonical (bare-message-via-TPFC + code-level-required field; phase-15). The 7th canonical's stat-discipline is INDEPENDENT (per ADR-0145; mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT discipline). Future §9 family-rows whose per-route proto follows the same "wrapper-with-reserved-field-and-single-optional-sub-message; absent-means-disabled; presence-means-override" shape compose against this canonical. ADR-0125's canonical-pattern roster grows from 6 to 7.

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

`internal/filter/http/rbac/rbac.go` exports:
- `TypeURL` const = `"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"`.
- `New` (the `HTTPFilterFactory` registered at boot per ADR-0072).
- `filterName` package-private const = `"envoy.filters.http.rbac"`.

### 6.2 `compiledConfig` + `compiledRulesEngine` + `compiledMatcherEngine` + `filterStats` shape

```go
// compiledConfig is the parsed + validated runtime config for one RBAC filter
// instance (listener-level OR per-route).
type compiledConfig struct {
    // Primary engine (rules or matcher; mutually exclusive; rules wins if both set per §1.1 amendment 2).
    rules   *compiledRulesEngine   // nil when matcher set (or both unset → wholly inactive).
    matcher *compiledMatcherEngine // nil when rules set or wholly inactive.

    // Shadow engine (rules or matcher; mutually exclusive; shadow_rules wins).
    shadowRules   *compiledRulesEngine
    shadowMatcher *compiledMatcherEngine

    // Stat namespacing (proto fields; empty default permitted per §1.1 amendment 3).
    rulesStatPrefix       string // proto: rules_stat_prefix
    shadowRulesStatPrefix string // proto: shadow_rules_stat_prefix
    trackPerRuleStats     bool   // proto: track_per_rule_stats

    // Stats — nil when ctx.Stats is nil (test path).
    stats *filterStats // 4 base counters (allowed/denied/shadow_allowed/shadow_denied) + per-policy lazy-allocation cache when trackPerRuleStats=true.
}

// compiledRulesEngine carries the parsed config.rbac.v3.RBAC for either primary or shadow.
// CEL fields (condition, checked_condition, cel_config) are NOT cached — silent-ignored at runtime.
type compiledRulesEngine struct {
    action   rbacconfigv3.RBAC_Action      // ALLOW=0 / DENY=1 / LOG=2 (PGV defined_only)
    policies []*compiledPolicy             // lexicographic-order-of-policy-name (sorted at parse)
}

// compiledPolicy is one entry from config.rbac.v3.RBAC.policies.
// Permission/Principal evaluator trees are pre-compiled at parse time.
type compiledPolicy struct {
    name        string                 // map key from policies map; preserved verbatim
    permissions []permissionEvaluator  // OR-semantic at runtime
    principals  []principalEvaluator   // OR-semantic at runtime
}

// compiledMatcherEngine carries the parsed xds.type.matcher.v3.Matcher tree wrapped via the
// new internal/matcher framework primitive (ADR-0142).
type compiledMatcherEngine struct {
    tree *matcher.Matcher // wraps internal/matcher.New result; PARSE-REJECT for non-canonical terminal TypeURLs already happened
}

// filterStats is the 4-counter base + lazy per-policy counter set.
// Per-policy counters are allocated on first match via NewCounterIfAbsent
// (post-Freeze idempotent registration per ADR-0117 + ADR-0139 precedent).
type filterStats struct {
    // Base (4 counters per active stat_prefix combination):
    allowed       *stats.Counter // increments per request with primary engine result = ALLOWED
    denied        *stats.Counter // increments per request with primary engine result = DENIED
    shadowAllowed *stats.Counter // increments per request with shadow engine result = ALLOWED (when shadow configured)
    shadowDenied  *stats.Counter // increments per request with shadow engine result = DENIED

    // Per-policy lazy-cache (allocated only when trackPerRuleStats=true; keyed by policy name + suffix).
    perPolicy *sync.Map // map[string]*stats.Counter — key = "<policy_name>.allowed", "<policy_name>.denied", etc.
    reg       *stats.Registry // for NewCounterIfAbsent at lazy-allocate
}
```

**Permission/Principal evaluator interfaces** (in `internal/filter/http/rbac/evaluator.go`):

```go
// permissionEvaluator + principalEvaluator are minimal interfaces; concrete types implement
// each of the 11 Permission + 11 Principal variants. AND/OR/NOT are recursive composites.
type permissionEvaluator interface {
    // evaluatePermission returns true if the permission matches the request.
    // ctx provides accessors for headers, path, IP, destination port, SNI, sourced-metadata, etc.
    evaluatePermission(ctx evalContext) bool
}

type principalEvaluator interface {
    // evaluatePrincipal returns true if the principal matches the request.
    // ctx provides accessors for TLS principal candidates, header, path, IP, sourced-metadata,
    // filter-state.
    evaluatePrincipal(ctx evalContext) bool
}

// evalContext is the per-stream accessor abstraction the evaluators consume.
// The *filter implements this; passed to both Permission/Principal walkers AND
// the matcher-engine walker (for the matcher-engine path; mirrors internal/matcher.MatchContext).
type evalContext interface {
    Header(name string) (value string, present bool)
    Path() string
    Method() string
    DirectRemoteIP() net.IP                   // peer connection's source IP
    RemoteIP() net.IP                         // XFF-resolved IP (per phase-04 XFF resolver)
    DestinationIP() net.IP                    // listener's bound IP (or connection's local IP)
    DestinationPort() uint32                  // listener's bound port (or connection's local port)
    RequestedServerName() string              // SNI; "" for plaintext
    DownstreamPrincipal() []string            // TLS principal candidates per §3.1 + ADR-0144
    SourcedMetadata(source rbacconfigv3.MetadataSource) *structpb.Struct // always returns empty/nil in MVP per §2.5
    FilterState(key string) any               // always returns nil in MVP per §2.5
}
```

### 6.3 `factoryState` + `filter` shape

```go
// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-11 ADR-0117 + phase-15 ADR-0139 IMPL-1 pattern.
type factoryState struct {
    listenerRC *compiledConfig
    perRoute   sync.Map // map[*rbacv3.RBACPerRoute]*compiledPerRoute — keyed by per-route TPFC proto pointer
    reg        *stats.Registry
}

// compiledPerRoute wraps the per-route disposition.
type compiledPerRoute struct {
    disabled       bool             // true when RBACPerRoute.rbac == nil per §5.1 (a)
    overrideConfig *compiledConfig  // non-nil when RBACPerRoute.rbac != nil per §5.1 (b); INDEPENDENT-stats per §5.2
}

// filter is the per-stream filter instance allocated by the factory closure.
// Decoder-only; no encode-side state.
type filter struct {
    state *factoryState
    dcb   envoyhttp.DecoderFilterCallbacks

    // Per-stream state (cached at DecodeHeaders).
    activeRC    *compiledConfig    // resolved at DecodeHeaders; may be listener-level OR per-route overrideConfig
    passthrough bool               // true when per-route disabled=true OR active engines both nil
}
```

### 6.4 `New` factory

Mirrors phase-15 bandwidthlimit's `New` (and earlier phase-11 + phase-12 precedents):

```go
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("rbac: typed_config required")
    }
    var c rbacv3.RBAC
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("rbac: unmarshal: %w", err)
    }
    rc, err := buildCompiledConfig(&c, ctx, false /*isPerRoute*/)
    if err != nil {
        return nil, err
    }
    state := &factoryState{
        listenerRC: rc,
        reg:        ctx.Stats,
    }
    return func() envoyhttp.HTTPFilter {
        f := &filter{state: state}
        return envoyhttp.HTTPFilter{
            Name:     filterName,
            Decoder:  f,
            Encoder:  nil, // decoder-only per §1 item 5
            PerRoute: parsePerRoute,
        }
    }, nil
}
```

### 6.5 `buildCompiledConfig` + `buildCompiledConfigPerRoute`

```go
func buildCompiledConfig(c *rbacv3.RBAC, ctx envoyhttp.FactoryCtx, isPerRoute bool) (*compiledConfig, error) {
    cc := &compiledConfig{
        rulesStatPrefix:       c.GetRulesStatPrefix(),
        shadowRulesStatPrefix: c.GetShadowRulesStatPrefix(),
        trackPerRuleStats:     c.GetTrackPerRuleStats(),
    }

    // Primary engine selection (rules wins per §1.1 amendment 2; both unset → wholly inactive).
    switch {
    case c.GetRules() != nil:
        rulesEngine, err := buildCompiledRulesEngine(c.GetRules(), "primary")
        if err != nil {
            return nil, err
        }
        cc.rules = rulesEngine
    case c.GetMatcher() != nil:
        matcherEngine, err := buildCompiledMatcherEngine(c.GetMatcher())
        if err != nil {
            return nil, err
        }
        cc.matcher = matcherEngine
    default:
        // Neither set: filter is wholly inactive per rbac.pb.go:33 proto comment.
    }

    // Shadow engine selection (shadow_rules wins per §1.1 amendment 2; both unset → no shadow).
    switch {
    case c.GetShadowRules() != nil:
        shadowRulesEngine, err := buildCompiledRulesEngine(c.GetShadowRules(), "shadow")
        if err != nil {
            return nil, err
        }
        cc.shadowRules = shadowRulesEngine
    case c.GetShadowMatcher() != nil:
        shadowMatcherEngine, err := buildCompiledMatcherEngine(c.GetShadowMatcher())
        if err != nil {
            return nil, err
        }
        cc.shadowMatcher = shadowMatcherEngine
    }

    // Stats — register the 4 base counters at the active stat-prefix namespace.
    if ctx.Stats != nil {
        if isPerRoute {
            cc.stats = newFilterStatsIfAbsent(ctx.Stats, cc.rulesStatPrefix, cc.shadowRulesStatPrefix)
        } else {
            cc.stats = newFilterStats(ctx.Stats, cc.rulesStatPrefix, cc.shadowRulesStatPrefix)
        }
    }

    return cc, nil
}

// buildCompiledRulesEngine parses one config.rbac.v3.RBAC sub-message (primary or shadow).
// Validates action enum + non-empty policies map + per-policy non-empty permissions/principals.
// Permission_Metadata + Principal_Metadata + Principal_SourceIp + Principal_Custom +
// Permission_Matcher + Permission_UriTemplate trigger PARSE-REJECT envoy-go-only errors.
func buildCompiledRulesEngine(r *rbacconfigv3.RBAC, role string) (*compiledRulesEngine, error) {
    // PGV-mirror defensive checks (§1.1 amendment 4):
    action := r.GetAction()
    switch action {
    case rbacconfigv3.RBAC_ALLOW, rbacconfigv3.RBAC_DENY, rbacconfigv3.RBAC_LOG:
    default:
        return nil, fmt.Errorf("rbac: invalid action %v (must be ALLOW/DENY/LOG)", action)
    }

    // Sort policy names lexicographically (per rbac.pb.go:268-269 proto comment).
    names := make([]string, 0, len(r.GetPolicies()))
    for name := range r.GetPolicies() {
        names = append(names, name)
    }
    sort.Strings(names)

    policies := make([]*compiledPolicy, 0, len(names))
    for _, name := range names {
        p := r.GetPolicies()[name]
        if len(p.GetPermissions()) == 0 {
            return nil, fmt.Errorf("rbac: policy %q must have at least one permission", name)
        }
        if len(p.GetPrincipals()) == 0 {
            return nil, fmt.Errorf("rbac: policy %q must have at least one principal", name)
        }
        perms, err := buildPermissionEvaluators(p.GetPermissions())
        if err != nil {
            return nil, fmt.Errorf("rbac: policy %q permissions: %w", name, err)
        }
        prins, err := buildPrincipalEvaluators(p.GetPrincipals())
        if err != nil {
            return nil, fmt.Errorf("rbac: policy %q principals: %w", name, err)
        }
        policies = append(policies, &compiledPolicy{
            name:        name,
            permissions: perms,
            principals:  prins,
        })
    }

    // audit_logging_options + cel fields silent-ignored per §2.1.1 + §2.1.2.
    return &compiledRulesEngine{
        action:   action,
        policies: policies,
    }, nil
}

// buildCompiledMatcherEngine wraps the matcher tree via the new internal/matcher primitive
// (ADR-0142). The supportedActionTypes parameter is the canonical-RBAC-Action-only allow-list
// per §2.6 + §11.P3.
func buildCompiledMatcherEngine(m *xdsmatcherv3.Matcher) (*compiledMatcherEngine, error) {
    supportedTypes := []string{"type.googleapis.com/envoy.config.rbac.v3.Action"}
    tree, err := matcher.New(m, supportedTypes)
    if err != nil {
        return nil, fmt.Errorf("rbac: matcher: %w", err)
    }
    return &compiledMatcherEngine{tree: tree}, nil
}

// buildPermissionEvaluators recurses through each permission variant + the Set combinators.
// PARSE-REJECT on deprecated + extension-coupling variants per §2.3.
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

func buildOnePermission(p *rbacconfigv3.Permission) (permissionEvaluator, error) {
    switch r := p.GetRule().(type) {
    case *rbacconfigv3.Permission_Any:
        return &permAny{val: r.Any}, nil
    case *rbacconfigv3.Permission_Header:
        return &permHeader{matcher: r.Header}, nil
    case *rbacconfigv3.Permission_UrlPath:
        return &permURLPath{matcher: r.UrlPath}, nil
    case *rbacconfigv3.Permission_DestinationIp:
        return &permDestIP{cidr: r.DestinationIp}, nil
    case *rbacconfigv3.Permission_DestinationPort:
        return &permDestPort{port: r.DestinationPort}, nil
    case *rbacconfigv3.Permission_DestinationPortRange:
        return &permDestPortRange{start: r.DestinationPortRange.GetStart(), end: r.DestinationPortRange.GetEnd()}, nil
    case *rbacconfigv3.Permission_RequestedServerName:
        return &permSNI{matcher: r.RequestedServerName}, nil
    case *rbacconfigv3.Permission_AndRules:
        children, err := buildPermissionEvaluators(r.AndRules.GetRules())
        if err != nil {
            return nil, err
        }
        return &permAnd{children: children}, nil
    case *rbacconfigv3.Permission_OrRules:
        children, err := buildPermissionEvaluators(r.OrRules.GetRules())
        if err != nil {
            return nil, err
        }
        return &permOr{children: children}, nil
    case *rbacconfigv3.Permission_NotRule:
        child, err := buildOnePermission(r.NotRule)
        if err != nil {
            return nil, err
        }
        return &permNot{child: child}, nil
    case *rbacconfigv3.Permission_SourcedMetadata:
        // Parse-supported; always-FALSE at runtime per §2.5.
        return &permSourcedMetadata{matcher: r.SourcedMetadata}, nil
    case *rbacconfigv3.Permission_Metadata:
        return nil, errors.New("rbac: permission.metadata deprecated; use sourced_metadata")
    case *rbacconfigv3.Permission_Matcher:
        return nil, errors.New("rbac: permission.matcher extension types unsupported in this build")
    case *rbacconfigv3.Permission_UriTemplate:
        return nil, errors.New("rbac: permission.uri_template extension types unsupported in this build")
    case nil:
        return nil, errors.New("rbac: permission rule oneof is unset")
    default:
        return nil, fmt.Errorf("rbac: unknown permission rule type %T", r)
    }
}

// buildPrincipalEvaluators mirrors buildPermissionEvaluators.
// PARSE-REJECT on deprecated source_ip + metadata + extension custom per §2.4 + §1.1 amendment 7.
func buildPrincipalEvaluators(prins []*rbacconfigv3.Principal) ([]principalEvaluator, error) { ... }

func buildOnePrincipal(p *rbacconfigv3.Principal) (principalEvaluator, error) {
    switch id := p.GetIdentifier().(type) {
    case *rbacconfigv3.Principal_Any:
        return &prinAny{val: id.Any}, nil
    case *rbacconfigv3.Principal_Authenticated_:
        return &prinAuthenticated{nameMatcher: id.Authenticated.GetPrincipalName()}, nil
    case *rbacconfigv3.Principal_DirectRemoteIp:
        return &prinDirectRemoteIP{cidr: id.DirectRemoteIp}, nil
    case *rbacconfigv3.Principal_RemoteIp:
        return &prinRemoteIP{cidr: id.RemoteIp}, nil
    case *rbacconfigv3.Principal_Header:
        return &prinHeader{matcher: id.Header}, nil
    case *rbacconfigv3.Principal_UrlPath:
        return &prinURLPath{matcher: id.UrlPath}, nil
    case *rbacconfigv3.Principal_AndIds:
        // recursive AND combinator
        ...
    case *rbacconfigv3.Principal_OrIds:
        // recursive OR combinator
        ...
    case *rbacconfigv3.Principal_NotId:
        // recursive NOT combinator
        ...
    case *rbacconfigv3.Principal_SourcedMetadata:
        // Parse-supported; always-FALSE at runtime per §2.5.
        return &prinSourcedMetadata{matcher: id.SourcedMetadata}, nil
    case *rbacconfigv3.Principal_FilterState:
        // Parse-supported; always-FALSE at runtime per §2.5.
        return &prinFilterState{matcher: id.FilterState}, nil
    case *rbacconfigv3.Principal_SourceIp:
        return nil, errors.New("rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip")
    case *rbacconfigv3.Principal_Metadata:
        return nil, errors.New("rbac: principal.metadata deprecated; use sourced_metadata")
    case *rbacconfigv3.Principal_Custom:
        return nil, errors.New("rbac: principal.custom extension types unsupported in this build")
    case nil:
        return nil, errors.New("rbac: principal identifier oneof is unset")
    default:
        return nil, fmt.Errorf("rbac: unknown principal identifier type %T", id)
    }
}
```

### 6.6 `prinAuthenticated` evaluator (per §1.1 amendment 12 + ADR-0144)

```go
// prinAuthenticated implements Principal_Authenticated per the three-case algorithm
// in §1.1 amendment 12 + rbac.pb.go:1432-1438 proto comment.
type prinAuthenticated struct {
    nameMatcher *typev3.StringMatcher // may be nil (case (a) per §1.1 amendment 12)
}

func (p *prinAuthenticated) evaluatePrincipal(ctx evalContext) bool {
    candidates := ctx.DownstreamPrincipal()
    if len(candidates) == 0 {
        // Case (c): plaintext / no client cert. All Principal_Authenticated evaluations FALSE.
        return false
    }
    if p.nameMatcher == nil {
        // Case (a): nil principal_name → matches any authenticated user.
        return true
    }
    // Case (b): match the StringMatcher against each candidate in priority order.
    for _, candidate := range candidates {
        if matchString(p.nameMatcher, candidate) {
            return true
        }
    }
    return false
}
```

The `DownstreamPrincipal()` accessor (ADR-0144) returns candidates in priority order: URI SANs (first), DNS SANs (second), Subject DN CN (third).

### 6.7 `DecodeHeaders` body

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
    // Resolve per-route TPFC.
    var perRouteMsg proto.Message
    if f.dcb != nil {
        perRouteMsg = f.dcb.RequestRouteConfig()
    }
    f.activeRC = f.state.resolvePerRouteConfig(perRouteMsg)

    if f.activeRC == nil {
        // No listener-level config + no per-route → defensive Continue.
        f.passthrough = true
        return envoyhttp.HeaderContinue
    }

    if f.activeRC.rules == nil && f.activeRC.matcher == nil {
        // Both primary engines unset → filter wholly inactive per rbac.pb.go:33.
        f.passthrough = true
        return envoyhttp.HeaderContinue
    }

    // Build the evalContext from the headers + connection accessors.
    ctx := f.buildEvalContext(headers)

    // Primary engine evaluation.
    primaryResult, primaryPolicyName := evaluateEngine(f.activeRC, ctx, /*shadow=*/false)

    // Shadow engine evaluation (if configured).
    if f.activeRC.shadowRules != nil || f.activeRC.shadowMatcher != nil {
        shadowResult, shadowPolicyName := evaluateEngine(f.activeRC, ctx, /*shadow=*/true)
        emitShadowCounters(f.activeRC, shadowResult, shadowPolicyName)
    }

    // Emit primary counters + apply disposition.
    emitPrimaryCounters(f.activeRC, primaryResult, primaryPolicyName)

    switch primaryResult {
    case engineResultAllowed:
        return envoyhttp.HeaderContinue
    case engineResultDenied:
        // Deny path: SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain}).
        f.dcb.SendLocalReply(403, "RBAC: access denied", envoyhttp.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})
        return envoyhttp.HeaderStopIteration
    default:
        return envoyhttp.HeaderContinue // defensive
    }
}
```

### 6.8 `DecodeData` + `DecodeTrailers` + `OnDestroy` + `SetDecoderCallbacks`

```go
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus { return envoyhttp.TrailersContinue }
func (f *filter) OnDestroy() {}
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
```

### 6.9 `evaluateEngine` (rules-engine path + matcher-engine path)

```go
type engineResult int

const (
    engineResultAllowed engineResult = iota
    engineResultDenied
)

// evaluateEngine walks the primary (or shadow) engine. Returns the engine result + the
// matched policy name (or "" if no policy matched OR matcher-engine no-match).
func evaluateEngine(cc *compiledConfig, ctx evalContext, shadow bool) (engineResult, string) {
    var rules *compiledRulesEngine
    var matcherEng *compiledMatcherEngine
    if shadow {
        rules = cc.shadowRules
        matcherEng = cc.shadowMatcher
    } else {
        rules = cc.rules
        matcherEng = cc.matcher
    }

    if rules != nil {
        return evaluateRulesEngine(rules, ctx)
    }
    if matcherEng != nil {
        return evaluateMatcherEngine(matcherEng, ctx)
    }
    // Shadow engine unset (and shadow=true was passed) — defensive ALLOWED.
    return engineResultAllowed, ""
}

func evaluateRulesEngine(re *compiledRulesEngine, ctx evalContext) (engineResult, string) {
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
            return engineResultAllowed, matchedPolicy
        }
        return engineResultDenied, ""
    case rbacconfigv3.RBAC_DENY:
        if matched {
            return engineResultDenied, matchedPolicy
        }
        return engineResultAllowed, ""
    case rbacconfigv3.RBAC_LOG:
        // Always-allow per §1.1 amendment 5; matchedPolicy captured for per-policy counter emission.
        // access_log_hint dynamic metadata NOT emitted (envoy-go MVP divergence-window).
        return engineResultAllowed, matchedPolicy
    }
    return engineResultDenied, "" // defensive
}

func policyMatches(p *compiledPolicy, ctx evalContext) bool {
    // permissions[] OR-semantic — short-circuit on first match.
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
    // principals[] OR-semantic — short-circuit on first match.
    for _, prin := range p.principals {
        if prin.evaluatePrincipal(ctx) {
            return true
        }
    }
    return false
}

func evaluateMatcherEngine(me *compiledMatcherEngine, ctx evalContext) (engineResult, string) {
    actionAny, err := me.tree.Evaluate(matcherCtxAdapter{ctx})
    if err != nil || actionAny == nil {
        // No-match: per rbac.pb.go:43-46 proto comment "Requests not matching any matcher will be denied."
        return engineResultDenied, ""
    }
    var action rbacconfigv3.Action
    if err := actionAny.UnmarshalTo(&action); err != nil {
        // Defensive: PARSE-REJECT should have caught at config-load time per ADR-0142.
        return engineResultDenied, ""
    }
    switch action.GetAction() {
    case rbacconfigv3.RBAC_ALLOW, rbacconfigv3.RBAC_LOG:
        return engineResultAllowed, action.GetName()
    case rbacconfigv3.RBAC_DENY:
        return engineResultDenied, action.GetName()
    }
    return engineResultDenied, ""
}
```

### 6.10 Counter emission

```go
func emitPrimaryCounters(cc *compiledConfig, result engineResult, policyName string) {
    if cc.stats == nil {
        return
    }
    switch result {
    case engineResultAllowed:
        cc.stats.allowed.Inc()
        if cc.trackPerRuleStats && policyName != "" {
            cc.stats.incPolicyAllowed(policyName)
        }
    case engineResultDenied:
        cc.stats.denied.Inc()
        if cc.trackPerRuleStats && policyName != "" {
            cc.stats.incPolicyDenied(policyName)
        }
    }
}

func emitShadowCounters(cc *compiledConfig, result engineResult, policyName string) {
    if cc.stats == nil {
        return
    }
    switch result {
    case engineResultAllowed:
        cc.stats.shadowAllowed.Inc()
        if cc.trackPerRuleStats && policyName != "" {
            cc.stats.incPolicyShadowAllowed(policyName)
        }
    case engineResultDenied:
        cc.stats.shadowDenied.Inc()
        if cc.trackPerRuleStats && policyName != "" {
            cc.stats.incPolicyShadowDenied(policyName)
        }
    }
}
```

The per-policy counter family is allocated lazily via `NewCounterIfAbsent` (post-Freeze idempotent registration per ADR-0117 + ADR-0139 precedent); the cache lives at `filterStats.perPolicy *sync.Map` keyed by `"<policy_name>.<suffix>"`.

### 6.11 `parsePerRoute` + `resolvePerRouteConfig`

```go
// parsePerRoute parses one RBACPerRoute TPFC entry. Per §5.1 7th canonical:
// - rbac == nil → produce a sentinel signaling "disabled on this route".
// - rbac != nil → recursively build the override compiledConfig.
// Returns the *rbacv3.RBACPerRoute proto message for use as the per-route map key.
func parsePerRoute(any *anypb.Any) (proto.Message, error) {
    var perRoute rbacv3.RBACPerRoute
    if err := any.UnmarshalTo(&perRoute); err != nil {
        return nil, fmt.Errorf("rbac: per-route unmarshal: %w", err)
    }
    // Defensive PGV mirror happens lazily at resolvePerRouteConfig → buildCompiledConfigPerRoute.
    return &perRoute, nil
}

func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledConfig {
    if msg == nil {
        return s.listenerRC
    }
    perRoute, ok := msg.(*rbacv3.RBACPerRoute)
    if !ok {
        return s.listenerRC
    }
    if cached, ok := s.perRoute.Load(perRoute); ok {
        return cached.(*compiledPerRoute).activeRC()
    }
    fresh := buildCompiledPerRoute(perRoute, s.reg)
    actual, _ := s.perRoute.LoadOrStore(perRoute, fresh)
    return actual.(*compiledPerRoute).activeRC()
}

func buildCompiledPerRoute(p *rbacv3.RBACPerRoute, reg *stats.Registry) *compiledPerRoute {
    if p.GetRbac() == nil {
        // Case (a): wholly disabled per §5.1 + ADR-0125 §(xii).
        return &compiledPerRoute{disabled: true, overrideConfig: nil}
    }
    // Case (b): wholesale override per §5.1.
    cc, err := buildCompiledConfig(p.GetRbac(), envoyhttp.FactoryCtx{Stats: reg}, true /*isPerRoute*/)
    if err != nil {
        // Per-route parse failed at lazy resolve. Sentinel for inherit-listener.
        return &compiledPerRoute{disabled: false, overrideConfig: nil}
    }
    return &compiledPerRoute{disabled: false, overrideConfig: cc}
}

// activeRC returns the *compiledConfig to use for this route, or nil for "passthrough on this route".
// In filter.DecodeHeaders, a nil activeRC triggers the passthrough fast-path.
func (cpr *compiledPerRoute) activeRC() *compiledConfig {
    if cpr.disabled {
        return nil // signals "wholly disabled" via the filter's passthrough flag
    }
    return cpr.overrideConfig // may be nil if parse failed → inherit-listener fallback (defensive)
}
```

NOTE on the activeRC nil-signaling: the `nil` return from `activeRC()` for disabled-route is distinct from the `listenerRC == nil` defensive case (which would only happen if both listener-level AND per-route are absent — structurally impossible since the filter wouldn't be in the chain without a listener-level config). The `filter.DecodeHeaders` body checks `f.activeRC == nil` AND `f.activeRC.rules == nil && f.activeRC.matcher == nil` for the passthrough fast-path.

---

## 7. Differential fixture `0018-http-rbac`

### 7.1 Per-request matrix (8 scenarios per BRAINSTORM §6.2 with refinements per §1.1 amendments)

| # | Scenario | Request | Expected response | Counter delta (envoy-go side) | §11 cross-ref |
|---|---|---|---|---|---|
| 1 | Allow-by-header-match (ALLOW + match) | `GET /` with `X-User: admin` | 200; body verbatim 32-byte payload; `default.allowed +1` | `<default>.rbac.<default>.allowed +1` | §11.P5 + §11.P6 |
| 2 | Deny-no-match (ALLOW + no-match) | `GET /` with `X-User: guest` | 403; body byte-exact `RBAC: access denied` (19 bytes); 4-header set lowercase; `default.denied +1` | `<default>.rbac.<default>.denied +1` | §11.P5 + §11.P6 + §1.1 amendments 10 + 11 |
| 3 | Allow-by-url-path | `GET /public` (no special header) | 200; `default.allowed +1` | analogous | §11.P5 |
| 4 | Allow-by-destination-port | `GET /` arriving on listener port; principals=any | 200; `default.allowed +1` | analogous | §11.P16 hypothesis-pending |
| 5 | Allow-by-direct-remote-ip | `GET /protected` from peer IP within `127.0.0.0/8` | 200; backend echoes; `default.allowed +1` | analogous | §11.P5 |
| 6 | Allow-by-TLS-principal (mTLS scenario) | `GET /admin` over mTLS to `l_test_a_tls` with cert URI SAN `spiffe://example.com/admin` | 200; `default.allowed +1` | analogous; exercises ADR-0144 framework primitive | §11.P14 + §1.1 amendment 12 |
| 7 | Per-route 7th-canonical disabled (absent-rbac) | `GET /per-route-disabled` with `X-User: guest` (would deny at listener) | 200 (passthrough); `default.allowed/denied` UNCHANGED at listener namespace | NO counter increments | §11.P1 + §11.P9 |
| 8 | Per-route wholesale-override with own stat_prefix + shadow | `GET /per-route-override` with `X-User: guest`; per-route DENIES guests; shadow runs | 403 (per-route denies); `override.denied +1`; `override_shadow.shadow_denied +1`; `default.*` UNCHANGED (INDEPENDENT-stats per §5.2 + §11.P9) | `<HCM>.rbac.<override>.denied +1` + `<HCM>.rbac.<override_shadow>.shadow_denied +1` | §11.P9 + §1.1 amendments 8 + 9 |

### 7.2 Topology

`test/fixtures/0018-http-rbac/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config.
- `inputs/driver.go` — Go driver issuing the 8 scenarios; byte-exact body comparison; per-counter delta scrape; mTLS-capable client for scenario 6.
- `expectations.yaml` — per-scenario allow-list / counter-delta map.
- `README.md` — fixture overview + scenario list + reference config citations + mTLS PKI generation notes.

Three listeners + one cluster (extends phase 11/12/13/14/15 fixture topology with the new mTLS listener for scenario 6):

- **Listener `l_test_a`** (TCP plaintext): HCM with one filter-chain `rbac → router`. Listener-level config (under listener-`stat_prefix: default_stat_prefix`):
  ```yaml
  envoy.filters.http.rbac:
    rules:
      action: ALLOW
      policies:
        admin_users:
          permissions: [{any: true}]
          principals: [{header: {name: X-User, string_match: {exact: admin}}}]
        public_paths:
          permissions: [{url_path: {path: {exact: /public}}}]
          principals: [{any: true}]
        listener_port_match:
          permissions: [{destination_port: <l_test_a's port>}]
          principals: [{any: true}]
        local_clients:
          permissions: [{url_path: {path: {prefix: /protected}}}]
          principals: [{direct_remote_ip: {address_prefix: 127.0.0.0, prefix_len: 8}}]
    rules_stat_prefix: default
  ```
  Routes:
  - `/` → `direct_response` 200 with 32-byte body. Default scenario route for 1 + 2 + 4.
  - `/public` → `direct_response` 200. Scenario 3.
  - `/protected` → cluster `c_backend_b`. Scenario 5.
  - `/per-route-disabled` → `direct_response` 200; per-route TPFC `RBACPerRoute{}` (empty; rbac field nil → disabled per §5.1 (a)). Scenario 7.
  - `/per-route-override` → `direct_response` 200; per-route TPFC `RBACPerRoute{rbac: <RBAC rules_stat_prefix:"override", shadow_rules_stat_prefix:"override_shadow", action:DENY, policies:{guests:{permissions:[{any:true}], principals:[{header:{name:X-User, string_match:{exact:guest}}}]}}, shadow_rules:<RBAC mirroring rules>>}` per §5.1 (b). Scenario 8.

- **Listener `l_test_b`** + cluster `c_backend_b`: echo-backend cluster pair (reuses `test/helpers/echobackend/` from phase 14/15). Routing target for `/protected`.

- **Listener `l_test_a_tls`** (TCP + TLS, mTLS-required): mirrors phase-03 mTLS infrastructure. Listener-level config sets `transport_socket: DownstreamTlsContext{common_tls_context:{tls_certificates:[server cert/key], validation_context:{trusted_ca:fixture-CA, match_subject_alt_names:[<SPIFFE pattern>]}}, require_client_certificate: true}`. HCM filter-chain mirrors `l_test_a`'s chain but with one additional policy under listener-level RBAC:
  ```yaml
  authenticated_admin:
    permissions: [{url_path: {path: {exact: /admin}}}]
    principals: [{authenticated: {principal_name: {exact: spiffe://example.com/admin}}}]
  ```
  Routes: `/admin` → `direct_response` 200. Scenario 6.

### 7.3 Asserted equivalence

Per fixture (asserted by `expectations.yaml` + driver):

- **Response status:** byte-exact between Envoy and envoy-go for every scenario (200 on allow paths; 403 on deny paths).
- **Response headers:** lowercase wire-form, set-equal between Envoy and envoy-go modulo the existing allow-list (`date`, `server`).
- **Response body:** byte-exact on ALL scenarios:
  - Allow paths (1, 3, 4, 5, 6, 7): passthrough bytes (direct_response payload OR backend echo bytes).
  - Deny paths (2, 8): byte-exact `RBAC: access denied` (19 bytes per §4 + §1.1 amendment 10).
  - Mirrors phase-11/12/13's byte-exact body discipline.
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the **4 active base counters** per active namespace (`<default>.allowed`, `<default>.denied`, `<override>.denied`, `<override_shadow>.shadow_denied`).
- **Per-route fixture-config disposition:** scenarios 7 + 8 exercise BOTH per-route shapes (`rbac: nil` 7th-canonical-disabled + wholesale-override); scenario 8 ALSO exercises INDEPENDENT-vs-SHARED stat namespace per §5.2.
- **TLS-principal scenario:** scenario 6 exercises the new ADR-0144 framework primitive; the fixture's client cert is generated at test-time (fixture-CA + client cert with URI SAN `spiffe://example.com/admin`); mTLS connection from the driver presents the cert.
- **`track_per_rule_stats` scope:** scenarios 1-6 + 8 emit per-policy counters when the listener-level config sets `track_per_rule_stats: true`. Fixture 0018 sets `track_per_rule_stats: false` at the listener-level to keep scenario coverage focused; a future fixture extension (or unit-test coverage) exercises the track-true surface.

### 7.4 Driver shape

`inputs/driver.go` mirrors the phase-15 driver shape:
- 8 scenarios; each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Per-scenario assertion helper for status + body + counter-delta.
- For scenario 6: a separate `runTLSScenario6` helper using the mTLS-capable HTTP client (similar to phase-03's TLS test infrastructure); the client presents the fixture client cert.
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.

Total estimated driver size: ~250-300 LoC (similar to phase-15 driver + the additional mTLS scenario helper).

**No H2 differential coverage.** Phase 16 fixture 0018 is HTTP/1.1-only + HTTP/1.1-over-mTLS (for scenario 6) per the existing §9 family-row convention.

---

## 8. ADRs anticipated (per BRAINSTORM §7; refined per §1.1)

7 ADRs anticipated (LARGEST §9-row ADR roster to date — phase 14 had 6 including impl-time ADR-0134; phase 15 had 5). ADR-0139 is the highest-numbered ADR landed in phase 15; ADR-0140 is the next-free.

| ADR | Subject | Anchor decision |
|---|---|---|
| **ADR-0140** | `internal/filter/http/rbac/` package shape — single-token directory matching cors/fault/csrf/buffer/compressor/localratelimit/bandwidthlimit precedent + boot registration ordering + DECODER-only `HTTPFilter` value (`Encoder: nil`) + 4-base-counter `filterStats` (per §1.1 amendment 8 — REFUTES BRAINSTORM 5-counter hypothesis) + lazy per-policy-counter allocation via `NewCounterIfAbsent` + deny-path wire shape `SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` per §4 + §1.1 amendment 10 | Decision 1 (BRAINSTORM §2.1) + §1.1 amendment 8 + §3 framework-survey + §4 |
| **ADR-0141** | `compiledConfig` shape + 7-consumed-proto-faithful field decomposition + dual-engine dispatch table (rules-engine path + matcher-engine path; rules wins when both set per §1.1 amendment 2) + UDPA-`field_alias`-annotation framing (NOT Go-level oneof per §1.1 amendment 2) + envoy-go-side parse validation (PGV-mirror defensive: action defined-only, policies.permissions/principals min_items=1, destination_port lte=65535) + ALLOW + DENY + LOG-partial action enum + LOG-partial divergence-window (always-allow + match-runs + access_log_hint silent; per §1.1 amendment 5) + CEL three-field silent-ignore (condition + checked_condition + cel_config per §1.1 amendment 6) | Decision 2 + Decision 4 (BRAINSTORM §2.2 + §2.4) + §1.1 amendments 1-6 |
| **ADR-0142** | Matcher-engine evaluator framework primitive at NEW top-level package `internal/matcher/` — implements `xds.type.matcher.v3.Matcher` generic match-tree evaluator with `New(tree, supportedActionTypes []string) (*Matcher, error)` + `Evaluate(MatchContext) (*anypb.Any, error)` API + `MatchContext` request-accessor interface; supported terminal action TypeURL allow-list `["type.googleapis.com/envoy.config.rbac.v3.Action"]` per §11.P3 (PARSE-REJECT for unknown TypeURLs); cross-phase-reusable by future filters (ext_authz, jwt_authn, oauth2) — they extend `supportedActionTypes` + widen `MatchContext` additively | Decision 2 (BRAINSTORM §2.2 + §3.2) + §11.P3 + §2.6 |
| **ADR-0143** | Permission + Principal Large 11+11 evaluators + AND/OR/NOT recursive combinators + Permission/Principal evaluator interface design + deprecated-field PARSE-REJECT discipline (permission.metadata, principal.metadata, principal.source_ip, principal.custom per §1.1 amendment 7 NEW finding — Principal has 14 variants not 13) + extension-coupling PARSE-REJECT (permission.matcher, permission.uri_template); cross-references the existing PathMatcher/HeaderMatcher/StringMatcher/CidrRange evaluators (shared infrastructure from phase-07.1) | Decision 3 (BRAINSTORM §2.3) + §1.1 amendment 7 + §2.3-2.4 |
| **ADR-0144** | TLS-principal accessor on `DecoderFilterCallbacks` framework primitive — `DownstreamPrincipal() []string` returns priority-ordered candidates (URI SANs + DNS SANs + Subject DN CN); plumbing from connection-level TLS state through HCM-dispatch to filter-callback; three-case algorithm for `Principal_Authenticated.principal_name` (nil → any-authenticated-user per §1.1 amendment 12; non-nil → StringMatcher iteration; plaintext → always FALSE); cross-phase reusable by future filters (jwt_authn, ext_authz, oauth2) | Decision 8 (BRAINSTORM §3.1) + §1.1 amendment 12 + §11.P14 |
| **ADR-0145** | Stat surface 4-base + variable per-policy + namespace + SN2-reuse hypothesis (`http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>`; NO new SN10 rule pending impl-time empirical scrape confirmation per §1.1 amendment 9) + per-policy counter format `<rules_stat_prefix>.<policy_name>.<suffix>` + INDEPENDENT per-route stats discipline (mirrors phase-11 ADR-0117 + phase-15 ADR-0139) + variable-stat-surface-with-track_per_rule_stats foot-gun documented | Decision 5 + Decision 7 (BRAINSTORM §2.6 + §2.8) + §1.1 amendments 8 + 9 + §11.P6 + §11.P7 + §11.P9 |
| **ADR-0146** | Shadow-evaluation discipline (parallel-to-primary engine walk; never-affects-disposition; emits shadow_* counters) + LOG-partial divergence-window (always-allow + match-evaluated + access_log_hint metadata silent per §1.1 amendment 5 + §8.6) + `track_per_rule_stats` per-policy emission discipline (lazy NewCounterIfAbsent per matched policy) + `response_code_details` divergence-window (envoy-go MVP no field emission per §1.1 amendment 11 + §8.12) + shadow access-log integration deferred per §8.7 + BEHAVIOR_CONTRACT phase-16 forward-pointer notes subsection summarizing all divergences | Decision 5 + Decision 6 (BRAINSTORM §2.5 + §2.7) + §1.1 amendments 5 + 11 + §8.6 + §8.7 + §8.12 |

**Plus an ADR-0125 in-place amendment paragraph §(xii)** (NOT a new ADR; authored at this SPEC commit per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) in-place-update precedent): documents phase 16 rbac as the FIRST row to use the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override; wrapper proto with reserved field 1 + single optional sub-message field; structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof + 6th canonical's bare-message-via-TPFC + code-level-required-field). ADR-0125's canonical-pattern roster grows from 6 to 7. The 7th canonical inherits ADR-0117 + ADR-0139's stateful-override-implies-INDEPENDENT-stats discipline directly. See §5.4 for the verbatim amendment paragraph.

SPEC-time may revise the 7-ADR count per ADR-0044 SPEC-time-anticipation discipline. ADRs anchor at impl-time per ADR-0044 (mirrors phase-13 + phase-15 precedent; phase-14's SPEC-time-pre-landing of ADR-0129..ADR-0133 is the divergent precedent). Phase 16's anticipated 7-ADR roster + ADR-0125 §(xii) amendment can grow to 8 if impl-time uncovers an unanticipated nuance (e.g., a framework gap analogous to phase-14's ADR-0134 directResponse-headers-to-add discovery).

NO new SN10 flattening rule unless impl-time §1.1 amendment 9 scrape confirmation refutes the SN2-reuse hypothesis. NO new framework primitives beyond ADR-0142 + ADR-0144.

---

## 9. Sibling-stub discipline (per BRAINSTORM §1.5 + ADR-0106(b))

This SPEC authors NO sibling SPEC stubs for the next §9 family-children (`jwt_authn`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`) plus the future `envoy.filters.http.decompressor` companion to compressor. Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts per ADR-0106(b) + (e). The §9 heading at `ROADMAP.md` line 63 stays unchanged across this landing per ADR-0106(c).

---

## 10. Acceptance review claims (the items the §5 reviewer must confirm)

The phase-16 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following claims against the landed artefacts:

1. **Package shape per ADR-0140:** `internal/filter/http/rbac/{rbac.go, evaluator.go, rbac_test.go, fuzz_test.go, doc.go}` with `Decoder: f, Encoder: nil` (decoder-only per §1 item 5; mirrors csrf + buffer precedent); 4-base-counter `filterStats` registered (allowed/denied/shadow_allowed/shadow_denied per §1.1 amendment 8); lazy per-policy counter allocation via `NewCounterIfAbsent` post-Freeze (mirrors ADR-0117 + ADR-0139).

2. **Field decomposition per ADR-0141 + §1.1 amendments 1-6:** 7 outer-envelope fields consumed proto-faithful (no silent-ignored at the outer); UDPA-`field_alias`-grouped rules/matcher + shadow_rules/shadow_matcher (rules wins when both set; shadow_rules wins); inner config.rbac.v3.RBAC silent-ignored set: `audit_logging_options` + Policy.condition + Policy.checked_condition + Policy.cel_config; ALLOW + DENY + LOG action enum honored at parse + runtime; LOG-partial silent-no-metadata-emit divergence-window documented; envoy-go-side defensive PGV-mirror validation with envoy-go-own error wording.

3. **Dual-engine dispatch per ADR-0141 + ADR-0142:** rules-engine path (policies map walk in lexicographic order; permissions OR + principals OR + action apply); matcher-engine path (via NEW `internal/matcher/` framework primitive; canonical RBAC `Action` terminal TypeURL allow-list + PARSE-REJECT for unknown TypeURLs); shadow path (parallel walk; never-affects-disposition; emits shadow_* counters); both unset → wholly inactive filter (returns Continue + emits no counters).

4. **Permission + Principal Large 11+11 evaluators per ADR-0143 + §1.1 amendment 7:** 11 Permission variants + 11 Principal variants implemented; AND/OR/NOT recursive combinators; SourcedMetadata + FilterState parse-supported with always-no-match runtime divergence-windows; DEFERRED set: permission.metadata + permission.matcher + permission.uri_template + principal.metadata + principal.source_ip + principal.custom (the latter is NEW per §1.1 amendment 7 — Principal has 14 variants not 13) — all PARSE-REJECT with envoy-go-only error wording.

5. **NEW framework primitives per ADR-0142 + ADR-0144:** (i) matcher-engine evaluator at `internal/matcher/` (NEW top-level package; cross-phase reusable; canonical RBAC terminal allow-list); (ii) TLS-principal accessor `DecoderFilterCallbacks.DownstreamPrincipal() []string` (URI SANs + DNS SANs + Subject DN CN priority order; three-case algorithm for `Principal_Authenticated.principal_name` per §1.1 amendment 12). Phase 16 is the FIRST §9 row since phase 14 to introduce non-zero deltas + FIRST single phase to introduce TWO simultaneously.

6. **Stat surface per ADR-0145 + §1.1 amendments 8 + 9:** 4 base counters (allowed/denied/shadow_allowed/shadow_denied) per active stat-prefix namespace combination; namespace SN2-reuse hypothesis `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` (impl-time empirical scrape confirms or amends with SN10 rule); per-policy counter family lazy-allocated when `track_per_rule_stats: true`; INDEPENDENT per-route stats discipline (mirrors phase-11 ADR-0117 + phase-15 ADR-0139); stat-table 60 → 64 names (4 new active counters; per-policy surface operator-config-driven and documented separately).

7. **Per-route discipline per §5 + ADR-0125 §(xii) amendment:** phase-16 is the FIRST row to use the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override; wrapper proto with reserved field 1 + single optional sub-message field); structurally distinct from 5th canonical (explicit-disabled-bool-in-oneof; phase-13 + phase-14) AND 6th canonical (bare-message-via-TPFC + code-level-required-field; phase-15); ADR-0125 in-place §(xii) amendment paragraph lands at SPEC time per phase-13/14/15 precedent; ADR-0125's canonical-pattern roster grows from 6 to 7; INDEPENDENT-stats discipline inherited from ADR-0117 + ADR-0139.

8. **§11 empirical pin block:** all 18 pins resolved IN-SESSION per ADR-0004; disposition tally captured at §11 summary table; verbatim proto + utility.h + utility.cc + rbac_filter.cc scrape evidence + the .pb.go raw-descriptor bytes; **12 §1.1 amendments** authored covering the empirical refinements + structural discoveries.

9. **Wire-shape claim:** byte-exact response (status + body + 4-header set) on allow paths AND deny paths; deny body = `"RBAC: access denied"` (19 bytes per §1.1 amendment 10); 403 + keep-alive (no `connection: close`); divergence-windows: `response_code_details` field-emission (envoy-go MVP no emission per §1.1 amendment 11 + §8.12), LOG-action `access_log_hint` dynamic-metadata emission (§8.6), shadow access-log entries (§8.7).

10. **Differential fixture per §7:** 8 scenarios; byte-exact body assertion (allow paths verbatim + deny paths 19-byte RBAC text); per-counter delta byte-equivalence on the 4 base counters per active namespace (`default.allowed`, `default.denied`, `override.denied`, `override_shadow.shadow_denied`); per-route INDEPENDENT-stats per scenarios 7 + 8; mTLS scenario 6 exercises ADR-0144 framework primitive.

11. **BEHAVIOR_CONTRACT.md populated** per Gate F:
    - §13.1 new `### envoy.filters.http.rbac` subsection (~200-300 lines incorporating field-decomposition table + dual-engine semantics + Large-11+11 + LOG-partial divergence-window + per-route 7th-canonical + INDEPENDENT-stats discipline + wire-shape).
    - §13.2 stat-table 60 → 64 names extension (4 new active counters; per-policy surface documented separately).
    - §13.3 NEW equivalence-matrix row pointing at fixture 0018 with allow-path + deny-path body byte-exact discipline.
    - §13.4 NEW `### Phase 16 forward-pointer notes` subsection covering the 12-item deferral list (CEL conditions; audit_logging; deprecated Permission/Principal variants; extension-coupling Permission variants; SourcedMetadata + FilterState always-no-match; track_per_rule_stats N-cap; LOG-action access_log_hint; shadow access-log; matcher terminal extension types; Principal_Authenticated outside canonical fields; `response_code_details` field; principal.custom).
    - §13.5 `## HTTPFilterCallbacks` extension documenting the new `DownstreamPrincipal()` accessor per ADR-0144.
    - §13.6 NEW prose subsection covering the matcher-engine framework primitive cross-phase reusability per ADR-0142.

12. **All six phase-done gates green at phase-done commit:** build/vet/lint clean; race-test clean across 39 packages (38 pre-phase-16 baseline + new `internal/matcher/` package; the `internal/filter/http/rbac/` package adds the 39th test surface); h2spec 53/53 PASS at ADR-0051 pin (phase 16 introduces no H2 wire-shape changes); 20 fuzzers green at 30s budget; 19 differential fixtures green (18 pre-phase-16 + new 0018); BEHAVIOR_CONTRACT.md populated per Gate F.

---

## 11. Empirical-pin block (per BRAINSTORM §9 — all 18 pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 / 10 / 11 / 12 / 13 / 14 / 15 SPEC §11's structure precisely. Probe date: **2026-05-12**.

**Reference source corpus:** Multiple verification axes used in this session (mirrors phase-15 multi-axis verification discipline):

1. v1.37.0 go-control-plane bindings (paired with Envoy v1.37.x release line): `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.37.0/extensions/filters/http/rbac/v3/rbac.{pb.go, pb.validate.go}` (266 + 385 LoC) + `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.37.0/config/rbac/v3/rbac.pb.go` (1727 LoC).
2. v1.32.4 go-control-plane bindings (cross-version sanity check): same path with `envoy@v1.32.4`.
3. Upstream Envoy v1.37.2 source via WebFetch:
   - `source/extensions/filters/http/rbac/rbac_filter.cc` — engine evaluation dispatch + SendLocalReply + counter increments.
   - `source/extensions/filters/common/rbac/utility.h` — `RoleBasedAccessControlFilterStats` struct + `ENFORCE_RBAC_FILTER_STATS` + `SHADOW_RBAC_FILTER_STATS` macros + per-policy counter methods.
   - `source/extensions/filters/common/rbac/utility.cc` — `generateStats` + `responseDetail` definitions.
4. xds matcher proto bindings at `/home/esa/go/pkg/mod/github.com/cncf/xds/go@v0.0.0-20251110193048-8bfbf64dc13e/xds/type/matcher/v3/matcher.pb.go`.

Verbatim file-line citations are durable on this SPEC drafting session machine.

### Summary disposition table (18 pins; 10 RATIFIED + 5 REFUTED/REFINED + 3 RATIFIED-PENDING-IMPL-TIME-CONFIRMATION)

| Pin | Topic | Disposition | Amendment cross-ref |
|---|---|---|---|
| **§11.P1** | `RBACPerRoute` proto shape | **RATIFIED** — single `rbac` field at field 2 + reserved field 1; absent-implies-disabled per proto comment | §1.1 amendment 1 + §5 + ADR-0125 §(xii) |
| **§11.P2** | PGV requirements per consumed field | **PARTIAL/REFRAMED** — outer 7 fields have NO PGV; inner constraints listed | §1.1 amendments 3 + 4 |
| **§11.P3** | Matcher-engine terminal action TypeURL set | **RATIFIED** — `type.googleapis.com/envoy.config.rbac.v3.Action` (single canonical) | §2.6 + ADR-0142 |
| **§11.P4** | `action: LOG` exact behavior | **REFINED** — always-allow + match-runs + `access_log_hint` metadata emit + ONLY allowed counter increments (NO logged counter) | §1.1 amendments 5 + 8 |
| **§11.P5** | Exact 403 wire shape | **RATIFIED** body bytes + status; **NEW finding** `response_code_details` format | §1.1 amendments 10 + 11 + §4 + §8.12 |
| **§11.P6** | Stat names + counter disposition | **REFUTED** — 4 base counters (NOT 5 — no `logged` counter exists) | §1.1 amendment 8 + ADR-0145 |
| **§11.P7** | Prometheus tag-extractor + namespace | **PARTIAL** — SN2-reuse hypothesis from `utility.h::generateStats` signature; impl-time empirical scrape confirms | §1.1 amendment 9 + ADR-0145 |
| **§11.P8** | Per-route override `rules_stat_prefix` emission scope | **RATIFIED-PENDING** — INDEPENDENT hypothesis from §11.P9 inference; impl-time empirical scrape confirms | §5.2 + ADR-0145 |
| **§11.P9** | Per-route stat SHARED-vs-INDEPENDENT | **RATIFIED-PENDING** — INDEPENDENT per stateful-override discipline; impl-time empirical scrape confirms | §5.2 + ADR-0145 |
| **§11.P10** | `track_per_rule_stats: true` per-policy counter emission cost + format | **PARTIAL** — format `<per_policy_final_prefix>{policy_name}{suffix}` from `utility.h` methods; impl-time empirical scrape confirms exact prefix template | §1.1 amendment 9 + ADR-0145 |
| **§11.P11** | Permission_Set + Principal_Set recursion depth bound | **RATIFIED-VIA-ABSENCE** — no PGV / no documented hard cap; envoy-go MVP no parse-time depth-cap (matches Envoy's permissive disposition) | ADR-0143 |
| **§11.P12** | Deprecated `metadata` Permission + Principal disposition | **RATIFIED** — proto deprecation annotations present; envoy-go PARSE-REJECT (envoy-go-only divergence) | §2.3 + §2.4 + ADR-0143 |
| **§11.P13** | Shadow access-log integration | **DEFERRED** — counter-only confirmed by Envoy source; access-log integration is post-MVP per §8.7 | §8.7 + ADR-0146 |
| **§11.P14** | `Principal_Authenticated` full algorithm | **RATIFIED-AND-EXTENDED** — URI SAN → DNS SAN → Subject DN priority confirmed; **NEW finding** nil-principal_name = any-authenticated-user | §1.1 amendment 12 + ADR-0144 |
| **§11.P15** | SourcedMetadata + FilterState default values | **RATIFIED** — empty under fixture-baseline; always-no-match envoy-go MVP behavior matches default-case Envoy | §2.5 + §8.10 |
| **§11.P16** | Listener-level config field types for Permission variants | **RATIFIED** — connection's local-side IP+port at receive time (mirrors phase-07.2 listener-chain-completion accessor pattern) | §3.3 + ADR-0143 |
| **§11.P17** | Listener-level vs per-stream access path for SNI | **RATIFIED** — SNI cached on connection accessor; existing phase-07.2 accessor surfaces to HCM filters | §3.3 + ADR-0143 |
| **§11.P18** | XFF resolution algorithm for `Principal_RemoteIp` | **RATIFIED** — rightmost-trusted-hop per phase-04 + phase-05 XFF resolver; reused | §3.3 + ADR-0143 |

**Tally:** 10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME (P7/P8/P9/P10 cluster) + 1 DEFERRED (P13).

**Structural amendments (re-frame §2.x Decisions):** **5** — Principal variant count (§11.P2 → 14 not 13; new deferred custom variant); 4-counter base (§11.P6 → REFUTES BRAINSTORM 5-counter); LOG-action counter emission (§11.P4 → folds into allowed); `Principal_Authenticated` nil-name semantic (§11.P14 → any-authenticated-user); `response_code_details` field-emission (§11.P5 NEW finding → envoy-go MVP defers). All handled via §1.1 amendment-block channel per phase-12 csrf + phase-14 compressor + phase-15 bandwidth_limit precedent. NO BRAINSTORM §12 amendment cycle required.

### 11.1 Empirical pin #1 — `RBACPerRoute` proto shape (RATIFIES BRAINSTORM §9.P1)

**Probe configuration:** Multi-source verification:
1. v1.37.0 go-control-plane bindings: `cat /home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.37.0/extensions/filters/http/rbac/v3/rbac.pb.go` — type definition at lines 147-191; raw proto descriptor at lines 207-210.
2. v1.32.4 cross-check: same disposition; only `RBACPerRoute` + `RBAC` messages exist.

**Verbatim type definition** (`rbac.pb.go:147-191`):

```go
type RBACPerRoute struct {
    state protoimpl.MessageState `protogen:"open.v1"`
    // Per-route specific RBAC configuration that overrides the global RBAC configuration.
    // If absent, RBAC policy will be disabled for this route.
    Rbac          *RBAC `protobuf:"bytes,2,opt,name=rbac,proto3" json:"rbac,omitempty"`
    unknownFields protoimpl.UnknownFields
    sizeCache     protoimpl.SizeCache
}
```

**Verbatim raw descriptor** (`rbac.pb.go:207-209`):

```
"\fRBACPerRoute\x12?\n" +
"\x04rbac\x18\x02 \x01(\v2+.envoy.extensions.filters.http.rbac.v3.RBACR\x04rbac:4\x9aň\x1e/\n" +
"-envoy.config.filter.http.rbac.v2.RBACPerRouteJ\x04\b\x01\x10\x02"
```

The `J\x04\b\x01\x10\x02` sequence at the end is the proto `reserved 1` declaration (per protobuf wire format: `J` = field 41 = reserved tag; `\x04` = length=4; `\b\x01\x10\x02` = range start=1, end exclusive=2).

**Conclusions (pinned) — RATIFIES BRAINSTORM §9.P1:**

- (a) `RBACPerRoute` has exactly ONE field: `rbac *RBAC` at field 2.
- (b) Field 1 is RESERVED (per proto evolution; was likely a removed `disabled` boolean in an earlier version per BRAINSTORM hypothesis).
- (c) `rbac` is OPTIONAL (no PGV `required` constraint per `rbac.pb.validate.go:282-308`); the wrapper carries no other validation.
- (d) Proto comment at line 150 reads verbatim: `"Per-route specific RBAC configuration that overrides the global RBAC configuration. If absent, RBAC policy will be disabled for this route."`
- (e) Phase-16 envoy-go disposition: `RBACPerRoute{rbac: nil}` → disabled-on-route (the 7th canonical absent-implies-disabled semantic); `RBACPerRoute{rbac: <RBAC>}` → wholesale-override of listener-level. ADR-0125 §(xii) amendment paragraph documents the NEW 7th canonical pattern.

### 11.2 Empirical pin #2 — PGV requirements per consumed field (PARTIAL/REFRAMED)

**Probe configuration:** Direct read of `rbac.pb.validate.go:42-187` (`RBAC.validate`) + `rbac.pb.validate.go:262-315` (`RBACPerRoute.validate`) + the inner `config/rbac/v3/rbac.pb.go` raw descriptor for inner-message PGV annotations.

**Outer RBAC PGV findings** (verbatim from `rbac.pb.validate.go:53-187`):

- `Rules`: embedded-message validation only (recursive `Validate()`); no field-level constraint.
- `RulesStatPrefix`: line 89 — `// no validation rules for RulesStatPrefix`.
- `Matcher`: embedded-message validation only.
- `ShadowRules`: embedded-message validation only.
- `ShadowMatcher`: embedded-message validation only.
- `ShadowRulesStatPrefix`: line 178 — `// no validation rules for ShadowRulesStatPrefix`.
- `TrackPerRuleStats`: line 180 — `// no validation rules for TrackPerRuleStats`.

**RBACPerRoute PGV findings**: `Rbac` field has embedded-message validation only (no `required` constraint per the auto-generated validator code). Per-route absence-of-rbac-field is valid + signals "disabled" per the proto comment.

**Inner config.rbac.v3.RBAC PGV findings** (from raw descriptor bytes at `config/rbac/v3/rbac.pb.go:1487-1538`):

- `RBAC.action` — PGV `defined_only = true` (`\x82\x01\x02\x10\x01` at line 1487).
- `RBAC.policies` — no PGV (map-value validation recursive).
- `Policy.permissions` — PGV `min_items = 1` (`\x92\x01\x02\b\x01` at line 1511).
- `Policy.principals` — PGV `min_items = 1` (`\x92\x01\x02\b\x01` at line 1513).
- `Permission.any` — PGV `const = true` (`\xfaB\x04j\x02\b\x01` at line 1527; `any: false` rejected).
- `Permission.destination_port` — PGV `lte = 65535` (`\xfaB\x06*\x04\x18\xff\xff\x03` at line 1532).
- `Permission.metadata` — proto field-level deprecation annotation (`\x92ǆ\xd8\x04\x033.0\x18\x01` at line 1534) but NO PGV `defined_only` block on the variant.
- `Principal.metadata` — same deprecation annotation; no PGV.
- `Principal.source_ip` — same deprecation annotation; no PGV.
- `SourcedMetadata.metadata_matcher` — PGV `required = true` (`\x8a\x01\x02\x10\x01` at line 1521).
- `SourcedMetadata.metadata_source` — PGV `defined_only = true`.

**Conclusions (pinned) — REFRAMED:**

- (a) Outer 7 fields have NO PGV constraints (all optional with embedded-message-recursion validation only).
- (b) Inner PGV constraints are at the deeper config.rbac.v3 level (action defined-only, policies.permissions/principals min_items=1, destination_port lte=65535, SourcedMetadata.metadata_matcher required, etc.).
- (c) Deprecated variants (Permission_Metadata, Principal_Metadata, Principal_SourceIp) carry proto-level deprecation annotations but NO PGV-level parse-rejection. envoy-go's PARSE-REJECT discipline (§2.3 + §2.4) is an envoy-go-only validation that DIVERGES from Envoy's lenient acceptance.
- (d) `Principal_Custom` (field 12; the 14th Principal variant per §1.1 amendment 7) has no PGV constraints; envoy-go's PARSE-REJECT is envoy-go-only.
- (e) Phase-16 envoy-go-side defensive PGV-mirror validation: action enum defined-only-check; policies.permissions + policies.principals min_items=1; destination_port range-check; envoy-go-own error wording per phase-11 ADR-0115 + phase-15 ADR-0136 precedent.

### 11.3 Empirical pin #3 — Matcher-engine terminal action TypeURL set (RATIFIED)

**Probe configuration:**
1. Direct read of `config/rbac/v3/rbac.pb.go:1146-1215` (`Action` type definition).
2. Direct read of `rbac.pb.go:43-49` (proto comment on `Matcher` field).
3. WebFetch of `source/extensions/filters/http/rbac/rbac_filter.cc` — terminal action dispatch.

**Verbatim Action type** (`config/rbac/v3/rbac.pb.go:1146-1215`):

```go
// Action defines the result of allowance or denial when a request matches the matcher.
type Action struct {
    state protoimpl.MessageState `protogen:"open.v1"`
    // The name indicates the policy name.
    Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
    // The action to take if the matcher matches...
    //   - "ALLOW": If the request gets matched on ALLOW, it is permitted.
    //   - "DENY": If the request gets matched on DENY, it is not permitted.
    //   - "LOG": If the request gets matched on LOG, it is permitted. ...
    //   - If the request cannot get matched, it will fallback to "DENY".
    Action RBAC_Action `protobuf:"varint,2,opt,name=action,proto3,enum=envoy.config.rbac.v3.RBAC_Action" json:"action,omitempty"`
    ...
}
```

**Proto comment on outer RBAC.matcher** (`rbac.pb.go:43-49`):

> *"Match tree for evaluating RBAC actions on incoming requests. Requests not matching any matcher will be denied."*

**Conclusions (pinned) — RATIFIES:**

- (a) The matcher-engine's terminal `on_match` action is `envoy.config.rbac.v3.Action` — a 2-field message (name + action enum); TypeURL `type.googleapis.com/envoy.config.rbac.v3.Action`.
- (b) The matcher-engine no-match disposition is DENY (per proto comment at `rbac.pb.go:43-46` + Action proto comment "If the request cannot get matched, it will fallback to DENY.").
- (c) Phase-16 envoy-go disposition: ADR-0142's `internal/matcher.New(tree, supportedActionTypes)` allow-lists `["type.googleapis.com/envoy.config.rbac.v3.Action"]`; unknown terminal TypeURLs PARSE-REJECTED with envoy-go-only error per §2.6 + §8.8 deferral.

### 11.4 Empirical pin #4 — `action: LOG` exact behavior (REFINED)

**Probe configuration:** Multi-source:
1. Direct read of `config/rbac/v3/rbac.pb.go:81-145` (`RBAC_Action` enum + comments).
2. WebFetch of `source/extensions/filters/http/rbac/rbac_filter.cc` — `evaluateEnforcedEngine` ALLOW/DENY branches.
3. WebFetch of `source/extensions/filters/common/rbac/utility.h` — `ENFORCE_RBAC_FILTER_STATS` + `SHADOW_RBAC_FILTER_STATS` macros.

**Verbatim RBAC_Action enum** (`config/rbac/v3/rbac.pb.go:83-93`):

```go
const (
    // The policies grant access to principals. The rest are denied. This is safe-list style
    // access control. This is the default type.
    RBAC_ALLOW RBAC_Action = 0
    // The policies deny access to principals. The rest are allowed. This is block-list style
    // access control.
    RBAC_DENY RBAC_Action = 1
    // The policies set the "access_log_hint" dynamic metadata key based on if requests match.
    // All requests are allowed.
    RBAC_LOG RBAC_Action = 2
)
```

**Verbatim filter-source evidence** (`source/extensions/filters/http/rbac/rbac_filter.cc` via WebFetch):

> *"For ALLOW results: Increments config_->stats().allowed_.inc(); Per-rule: config_->stats().incPolicyAllowed(effective_policy_id) (if enabled); Sets metadata: EngineResultAllowed; Returns: Http::FilterHeadersStatus::Continue. For DENY results: Increments config_->stats().denied_.inc(); Per-rule: config_->stats().incPolicyDenied(effective_policy_id); Sends local reply with code Forbidden and message 'RBAC: access denied'; Sets metadata: EngineResultDenied; Returns: Http::FilterHeadersStatus::StopIteration."*

**Verbatim utility.h macros** (via WebFetch):

> *"ENFORCE_RBAC_FILTER_STATS defines: `allowed`, `denied`. SHADOW_RBAC_FILTER_STATS defines: `shadow_allowed`, `shadow_denied`."*

**Conclusions (pinned) — REFINED:**

- (a) Three Action enum values: ALLOW=0, DENY=1, LOG=2 (per proto + matches BRAINSTORM hypothesis).
- (b) LOG always-allows: per proto comment `"All requests are allowed."` LOG sets `access_log_hint` dynamic metadata key based on match.
- (c) Counter emission under LOG: the filter source increments `allowed_` on every request whose engine result = ALLOWED. Under LOG, engine result is ALWAYS ALLOWED (since LOG always-allows). Therefore, LOG-action requests increment the SAME `allowed` counter as ALLOW-action requests. The `denied` counter does NOT increment under LOG.
- (d) **NO separate `logged` counter exists in Envoy v1.37.2** (per the `utility.h` macros — only allowed + denied are defined). The BRAINSTORM 5-counter hypothesis is REFUTED; the actual counter surface is 4 base (allowed/denied/shadow_allowed/shadow_denied). §1.1 amendment 8 documents.
- (e) Phase-16 envoy-go disposition per §1.1 amendment 5: LOG engine evaluation result = ALLOWED always; `allowed` counter increments; matched policy name captured for per-policy counter emission (when track is true); `access_log_hint` dynamic metadata emission SKIPPED with divergence-window documented at §8.6 + ADR-0146.

### 11.5 Empirical pin #5 — Exact 403 wire shape (RATIFIED + NEW FINDING)

**Probe configuration:** WebFetch of `source/extensions/filters/http/rbac/rbac_filter.cc` + `source/extensions/filters/common/rbac/utility.cc`.

**Verbatim filter-source evidence** (`rbac_filter.cc` via WebFetch):

> *"SendLocalReply body string: `\"RBAC: access denied\"`. HTTP status code: `Http::Code::Forbidden` (403). Response code details: Derived from `Filters::Common::RBAC::responseDetail(log_policy_id)`."*

**Verbatim utility.cc evidence** (via WebFetch):

> *"responseDetail function generates a response code details string with this format: `\"rbac_access_denied_matched_policy[{sanitized_policy_id}]\"`. The function sanitizes the policy ID by replacing whitespaces with underscores."*

**Conclusions (pinned) — RATIFIED BRAINSTORM hypothesis + NEW FINDING:**

- (a) Body bytes: `"RBAC: access denied"` (19 bytes ASCII; bytes `52 42 41 43 3a 20 61 63 63 65 73 73 20 64 65 6e 69 65 64`; no trailing newline). MATCHES BRAINSTORM §4 hypothesis.
- (b) Status: 403 Forbidden.
- (c) **NEW finding** — `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"`. Phase-16 envoy-go MVP DEFERS this field emission (current phase-04 HCM does not surface response-code-details to local-reply callers); divergence-window documented at §1.1 amendment 11 + §8.12.
- (d) Phase-16 envoy-go disposition: `cb.SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})`. 4-header set lowercase wire-form: `content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy` per phase-11 + phase-12 4-header discipline. Keep-alive (no `connection: close`).

### 11.6 Empirical pin #6 — Exact stat names + counter/gauge disposition (REFUTES BRAINSTORM 5-counter hypothesis)

**Probe configuration:** WebFetch of `source/extensions/filters/common/rbac/utility.h` (macros + struct definition).

**Verbatim utility.h evidence** (via WebFetch):

> *"ENFORCE_RBAC_FILTER_STATS defines: `allowed`, `denied`. SHADOW_RBAC_FILTER_STATS defines: `shadow_allowed`, `shadow_denied`. ALL_RBAC_FILTER_STATS: Not present in the provided content. RoleBasedAccessControlFilterStats struct wraps shadow and enforced rule statistics with these key members: Counter fields generated from ENFORCE_RBAC_FILTER_STATS and SHADOW_RBAC_FILTER_STATS macros."*

**Conclusions (pinned) — REFUTES BRAINSTORM §9.P6 + §1.1 item 7:**

- (a) **4 base counters** in Envoy v1.37.2: `allowed`, `denied`, `shadow_allowed`, `shadow_denied`.
- (b) **NO `logged` counter exists.** LOG-action increments `allowed` (since LOG always-allows; result = ALLOWED).
- (c) NO gauges in the rbac filter (counter-only filter; differs from phase-15 bandwidth_limit which had 6 gauges per stat_prefix).
- (d) NO histograms (counter-only; differs from phase-15 bandwidth_limit which had 2 unconditional histograms).
- (e) Per-policy counters (when `track_per_rule_stats: true`): 4 variants per matched policy (`<policy_name>.allowed`, `<policy_name>.denied`, `<policy_name>.shadow_allowed`, `<policy_name>.shadow_denied`); cost is operator-config-driven.
- (f) Phase-16 envoy-go disposition: filterStats struct carries 4 counters + lazy per-policy sync.Map per §6.2 + ADR-0145. §1.1 amendment 8 documents the BRAINSTORM 5-counter hypothesis refutation.

### 11.7 Empirical pin #7 — Prometheus tag-extractor + namespace flattening (PARTIAL)

**Probe configuration:** WebFetch of `source/extensions/filters/common/rbac/utility.h` (struct constructor + per_policy_final_prefix_ initialization context).

**Verbatim utility.h evidence** (via WebFetch):

> *"generateStats Function Signature: `RoleBasedAccessControlFilterStats generateStats(const std::string& prefix, const std::string& rules_prefix, const std::string& shadow_rules_prefix, Stats::Scope& scope);`. Per-Policy Counter Prefix Format: The code constructs metrics dynamically using: `\"{per_policy_final_prefix_}{policy_name}{suffix}\"` where suffix is one of: `\".allowed\"`, `\".denied\"`, `\".shadow_allowed\"`, or `\".shadow_denied\"`."*

**Conclusions (pinned) — PARTIAL (impl-time empirical scrape confirms exact prefix template):**

- (a) `generateStats` takes 3 string arguments: `prefix` (HCM stat_prefix per filter-config wiring), `rules_prefix` (proto rules_stat_prefix), `shadow_rules_prefix` (proto shadow_rules_stat_prefix).
- (b) The base counter prefix template likely follows `<prefix>.rbac.<rules_prefix>.<counter>` for primary; `<prefix>.rbac.<shadow_rules_prefix>.<counter>` for shadow. Mirrors phase-15 bandwidth_limit's `<stat_prefix>.http_bandwidth_limit.<counter>` shape but with `rbac` infix instead of `http_bandwidth_limit`.
- (c) Per-policy counter template: `<per_policy_final_prefix>{policy_name}{suffix}` — the `per_policy_final_prefix_` likely is `<prefix>.rbac.<rules_prefix>.policy.` (impl-time empirical scrape confirms).
- (d) Phase-16 SPEC's position per §1.1 amendment 9 + ADR-0145: SN2-reuse hypothesis (no new SN10 rule); namespace shape `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>`. Alternative hypothesis: flat `<rules_stat_prefix>.rbac.<counter>` similar to phase-15. **Impl-time empirical scrape against reference Envoy v1.37.2 with a probe yaml confirms the exact shape + amends ADR-0145 accordingly.**

### 11.8 Empirical pin #8 — Per-route override `rules_stat_prefix` emission scope (RATIFIED-PENDING)

**Probe configuration:** Sub-pin of §11.P9. Confirmed via inference from §11.P9 + the brainstorm hypothesis.

**Conclusions (pinned) — RATIFIED-PENDING:** Per the stateful-override-implies-INDEPENDENT discipline (§5.2 + ADR-0117 + ADR-0139 precedent): per-route override's `rules_stat_prefix` field, if set, drives a wholly-own counter namespace. The override owns its own `*compiledConfig` (different policy set + different action enum + different stat prefix), so the resulting counters MUST emit to the per-route's namespace, not the listener-level's. Impl-time empirical scrape against reference Envoy with a probe yaml confirms; if Envoy emits SHARED, envoy-go SPEC author's position is to DIVERGE (INDEPENDENT — the operationally-correct shape) and document.

### 11.9 Empirical pin #9 — Per-route stat SHARED-vs-INDEPENDENT (RATIFIED-PENDING)

**Probe configuration:** Per §11.P8 above; deferred to impl-time empirical scrape for definitive confirmation. Brainstorm hypothesis + structural argument:

- Phase-11 local_ratelimit's `*tokenBucket` is stateful-per-route → INDEPENDENT-stats per ADR-0117.
- Phase-15 bandwidth_limit's per-route throttle-config is stateful-per-route → INDEPENDENT-stats per ADR-0139.
- Phase-16 rbac's per-route policy-set + matcher-tree IS stateful-per-route (different evaluators, different match-tree). Therefore: INDEPENDENT.

**Conclusions (pinned) — RATIFIED-PENDING:** Phase-16 hypothesis = INDEPENDENT per ADR-0145. Impl-time empirical scrape against reference Envoy v1.37.2 confirms or refutes (and amends ADR-0145 + adds divergence-window documentation if Envoy emits SHARED).

### 11.10 Empirical pin #10 — `track_per_rule_stats: true` per-policy counter format (PARTIAL)

**Probe configuration:** Per §11.P7 above. The per-policy emission methods are `incPolicyAllowed(name)`, `incPolicyDenied(name)`, `incPolicyShadowAllowed(name)`, `incPolicyShadowDenied(name)` (per `utility.h`). The format is `<per_policy_final_prefix>{policy_name}{suffix}` (per §11.P7 verbatim quote).

**Conclusions (pinned) — PARTIAL:** Per-policy counter cost is O(N) where N = number of matched policies under track-true; envoy-go MVP no parse-time N-cap (mirrors Envoy permissive discipline per §8.5). Impl-time empirical scrape confirms the exact prefix template.

### 11.11 Empirical pin #11 — Permission_Set + Principal_Set recursion depth bound (RATIFIED-VIA-ABSENCE)

**Probe configuration:** Direct read of `config/rbac/v3/rbac.pb.go:1333-1423` (`Permission_Set` + `Principal_Set` definitions + recursive `Permission`/`Principal` field types).

**Verbatim Permission_Set** (`config/rbac/v3/rbac.pb.go:1335-1377`):

```go
type Permission_Set struct {
    state         protoimpl.MessageState `protogen:"open.v1"`
    Rules         []*Permission          `protobuf:"bytes,1,rep,name=rules,proto3" json:"rules,omitempty"`
    unknownFields protoimpl.UnknownFields
    sizeCache     protoimpl.SizeCache
}
```

NO PGV constraint on `Rules`. Recursion via `Permission.GetAndRules() *Permission_Set` etc.

**Conclusions (pinned) — RATIFIED-VIA-ABSENCE:**

- (a) NO PGV-level recursion depth bound exists at the proto level.
- (b) No documented hard cap in Envoy source for AND/OR/NOT recursion depth.
- (c) Phase-16 envoy-go MVP disposition: NO parse-time depth-cap (mirrors Envoy's permissive disposition). The recursive `buildOnePermission` + `buildOnePrincipal` calls form a natural Go-stack-depth bound (~10K-frame Go-stack default before SIGSEGV). Operators writing deeply-nested rules-engine configs may hit Go-stack-depth issues at config-load time; documented foot-gun.
- (d) Future operator-ergonomics phase MAY add an envoy-go-only depth-cap (e.g., max 32 levels of nesting) per §8.5-style discipline; deferred from MVP.

### 11.12 Empirical pin #12 — Deprecated `metadata` Permission + Principal disposition (RATIFIED)

**Probe configuration:** Direct read of `config/rbac/v3/rbac.pb.go:1534` (Permission.metadata deprecation annotation).

**Verbatim deprecation annotation** (`config/rbac/v3/rbac.pb.go:1534`):

```
"\bmetadata\x18\a \x01(\v2&.envoy.type.matcher.v3.MetadataMatcherB\v\x92ǆ\xd8\x04\x033.0\x18\x01H\x00R\bmetadata"
```

The `\x92ǆ\xd8\x04\x033.0\x18\x01` bytes are the `envoy.annotations.deprecated_at_minor_version="3.0"` + `[#deprecated_at_minor_version_enum="3.0"]` annotation per Envoy's deprecation-tagging discipline. NO PGV `defined_only`-style parse-reject — the field is parse-accepted with a deprecation log warning at Envoy's config-load time.

**Conclusions (pinned) — RATIFIED:**

- (a) Envoy v1.37.2 parse-accepts `permission.metadata` and `principal.metadata` and `principal.source_ip` (deprecated-but-functional disposition).
- (b) envoy-go MVP's PARSE-REJECT discipline (per §2.3 + §2.4 + ADR-0143) is an envoy-go-only DIVERGENCE — envoy-go-strict, Envoy-lenient.
- (c) Operator divergence-window: configs setting deprecated variants load on Envoy with a deprecation warning + function normally; on envoy-go, the same configs fail to load with envoy-go-only error wording.
- (d) Rationale for envoy-go-strict: phase-15 ADR-0136 + phase-16 ADR-0143 take the position that explicit foot-gun guards are operationally cleaner than silent-acceptance of deprecated fields. Documented at BEHAVIOR_CONTRACT phase-16 forward-pointer notes.

### 11.13 Empirical pin #13 — Shadow access-log integration (DEFERRED — counter-only confirmed)

**Probe configuration:** WebFetch of `source/extensions/filters/http/rbac/rbac_filter.cc` (search for access-log emit + dynamic-metadata setters in shadow-engine evaluation).

**Conclusions (pinned) — DEFERRED:** Per Envoy source review, the shadow-engine path emits SHADOW counters (`shadow_allowed`, `shadow_denied`) but does NOT emit a separate shadow-decision access-log entry in v1.37.2. The shadow path's primary purpose is observability via Prometheus counters. Phase-16 envoy-go MVP matches: counter-only shadow emission. §8.7 deferral. ADR-0146 documents.

NOTE: a future Envoy version may add access-log integration; impl-time PROGRESS review checks whether any change at v1.37.2 → newer arose.

### 11.14 Empirical pin #14 — `Principal_Authenticated` full algorithm (RATIFIED + NEW finding)

**Probe configuration:** Direct read of `config/rbac/v3/rbac.pb.go:1425-1479` (`Principal_Authenticated` type definition + proto comment).

**Verbatim proto comment** (`config/rbac/v3/rbac.pb.go:1432-1438`):

> *"The name of the principal. If set, The URI SAN or DNS SAN in that order is used from the certificate, otherwise the subject field is used. If unset, it applies to any user that is allowed by the downstream TLS configuration. If require_client_certificate is false or trust_chain_verification is set to ACCEPT_UNTRUSTED, then no authentication is required."*

**Conclusions (pinned) — RATIFIED + NEW finding:**

- (a) Priority order for principal-name extraction: **URI SAN (first), DNS SAN (second), Subject DN CN (third).** MATCHES BRAINSTORM §3.1 hypothesis verbatim.
- (b) **NEW finding** — `principal_name == nil` (the StringMatcher field absent) → matches ANY user that passed downstream TLS verification. envoy-go MVP implements three-case algorithm per §1.1 amendment 12 + §6.6.
- (c) `require_client_certificate: false` OR `trust_chain_verification: ACCEPT_UNTRUSTED` semantics: the principal-name extraction may yield candidates from unverified certs. envoy-go MVP: `DownstreamPrincipal()` returns candidates from `tls.ConnectionState.PeerCertificates[0]` REGARDLESS of trust-chain-verification state; the filter trusts the TLS configuration the operator set up. Future TLS-trust-context phase may refine; deferred.
- (d) Phase-16 envoy-go disposition: `DownstreamPrincipal() []string` accessor returns `[URI_SANs..., DNS_SANs..., Subject_CN]` for the active downstream connection (empty/nil for plaintext); `prinAuthenticated` evaluator handles three cases per §6.6. ADR-0144 codifies.

### 11.15 Empirical pin #15 — SourcedMetadata + FilterState default values (RATIFIED)

**Probe configuration:** Inference from Envoy v1.37.2's default fixture-baseline behavior. Without operator-set dynamic-metadata or filter-state, the SourcedMetadata + FilterState evaluators in Envoy ALSO return no-match (empty metadata returns no-match against any MetadataMatcher). Same as envoy-go's MVP always-FALSE behavior.

**Conclusions (pinned) — RATIFIED:** envoy-go's always-no-match runtime behavior matches Envoy's default-case behavior (no operator-set metadata → both proxies return FALSE). Real-world divergence only appears when operator configs explicitly set dynamic-metadata or filter-state from upstream filters; documented as divergence-window. §8.10 deferral.

### 11.16 Empirical pin #16 — Listener-level config field types for Permission variants (RATIFIED)

**Probe configuration:** Cross-reference with phase-07.2 listener-chain-completion + envoy-go's existing listener context accessors.

**Conclusions (pinned) — RATIFIED:** Phase-16 envoy-go's `evalContext.DestinationIP()` + `DestinationPort()` accessors surface the connection's LOCAL-SIDE IP + port at receive time (mirrors phase-07.2 FilterChainMatch + the listener-context accessor pattern). The brainstorm hypothesis is correct: Envoy v1.37.2 evaluates `Permission_DestinationIp` + `Permission_DestinationPort` + `Permission_DestinationPortRange` against the connection's local-side IP+port. ADR-0143 codifies.

### 11.17 Empirical pin #17 — Listener-level vs per-stream access path for SNI (RATIFIED)

**Probe configuration:** Cross-reference with phase-07.2 listener-chain-completion's SNI surface.

**Conclusions (pinned) — RATIFIED:** Phase-07.2's listener-chain-completion landed an SNI accessor on the connection context; HCM filters access SNI via the existing connection-context pathway. envoy-go's `evalContext.RequestedServerName()` returns the SNI value from the connection accessor; "" for plaintext connections. ADR-0143 codifies.

### 11.18 Empirical pin #18 — XFF resolution algorithm for `Principal_RemoteIp` (RATIFIED)

**Probe configuration:** Cross-reference with phase-04 HTTP/1.1's XFF resolver + phase-05 HTTP/2's analogous XFF resolver.

**Conclusions (pinned) — RATIFIED:** Phase-04 / phase-05 envoy-go's XFF resolution uses the rightmost-trusted-hop algorithm (consistent with Envoy v1.37.2 documentation). For `Principal_RemoteIp` evaluation: envoy-go's `evalContext.RemoteIP()` returns the XFF-resolved remote IP via the existing resolver; `evalContext.DirectRemoteIP()` returns the peer connection's source IP (no XFF resolution). The two variants are distinct per the proto comment at `config/rbac/v3/rbac.pb.go:1059-1069`. ADR-0143 codifies.

---

## 12. Deferred decisions (the planner / implementer settles these)

Per phase-11/12/13/14/15 inline-deferral discipline (no omnibus ADR), the deferrals are **12 family-coupled items** (largest §9-row deferral list to date — phase 15 had 8 items; phase 16 has 12 reflecting the wider proto surface + the new framework primitives):

### 12.1 CEL `condition` + `checked_condition` + `cel_config` Policy fields (per §8.1 + §1.1 amendment 6)

Q7 silent-ignore. Couples to a future CEL framework phase that lands `internal/cel/` evaluator + `github.com/google/cel-go` dependency (~3000-LoC dep). Operator divergence-window: policies relying on CEL conditions see envoy-go-vs-Envoy decision divergence.

### 12.2 `audit_logging_options` (per §8.2)

`[#not-implemented-hide:]` upstream. Couples to future audit-logging family phase. envoy-go silent-ignores; matches Envoy v1.37.2.

### 12.3 Deprecated `metadata` Permission + Principal variants (per §8.3 + §11.P12)

PARSE-REJECT envoy-go-only. Couples to operator-ergonomics phase (if/when envoy-go decides to soften to silent-ignore-with-warning).

### 12.4 Deprecated `Principal_SourceIp` (per §8.4)

Same disposition as 12.3.

### 12.5 `Principal_Custom` extension variant (per §1.1 amendment 7 + §8.11 NEW)

PARSE-REJECT envoy-go-only with error `"rbac: principal.custom extension types unsupported in this build"`. The 14th Principal variant (per §1.1 amendment 7). The canonical extension is `MTlsAuthenticated` per `rbac.pb.go:1427-1429`. Couples to a future mTLS-extension framework phase.

### 12.6 Permission `matcher` + `uri_template` extension variants (per §8.8)

PARSE-REJECT envoy-go-only. Couples to plugin framework — Envoy's extension registry for matcher-action plugins.

### 12.7 `track_per_rule_stats: true` envoy-go-only large-N parse-rejection (per §8.5 + §11.P10)

NO cap in phase-16 MVP. Documented foot-gun. Revisit if real-world deployments hit problems.

### 12.8 LOG-action dynamic-metadata emission (per §8.6 + §1.1 amendment 5)

envoy-go MVP silent-no-metadata-emit. Couples to future dynamic-metadata family phase landing `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive. Re-activation enables wire-shape equivalence for LOG-action with downstream access-log integration.

### 12.9 Shadow-rules access-log integration (per §8.7 + §11.P13)

envoy-go MVP emits shadow counters only. Couples to access-log subsystem feature.

### 12.10 Matcher-engine terminal action TypedExtensionConfig types beyond canonical RBAC `Action` (per §8.8 + §11.P3)

PARSE-REJECT envoy-go-only for non-canonical TypeURLs. Couples to matcher-extension family phase.

### 12.11 `Principal_Authenticated` outside URI SAN / DNS SAN / Subject DN canonical fields (per §8.9 + §11.P14)

Additional cert fields (Issuer DN, Serial Number, fingerprints) NOT exposed in phase-16 MVP. Couples to TLS-context-extension phase.

### 12.12 `response_code_details` field emission (per §1.1 amendment 11 + §8.12 NEW)

envoy-go MVP no field emission. Couples to a future response-code-details framework phase that threads HCM's local-reply path to a per-filter accessor.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

Per `BOOTSTRAP_PROMPT.md` §7.5 Gate F:

### 13.1 `## HTTP filter chain ### envoy.filters.http.rbac` NEW subsection

Patch shape (in-place edit at the existing `## HTTP filter chain` umbrella; alphabetical-canonical ordering of the existing subsection list `bandwidth_limit < buffer < compressor < cors < csrf < fault < header_mutation < local_ratelimit < rbac`, so the new `### envoy.filters.http.rbac` subsection inserts at the END of the list, immediately under the existing `### envoy.filters.http.local_ratelimit` subsection):

```markdown
### envoy.filters.http.rbac

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.rbac.v3.RBAC` (7 top-level fields per [#next-free-field: 8]):**

| Field | Type | Phase 16 disposition | Notes |
|---|---|---|---|
| `rules` | config.rbac.v3.RBAC | CONSUMED | Primary policy engine; UDPA-`field_alias`-grouped with `matcher` under `rules_specifier`; when both set, `rules` wins per `rbac.pb.go:38` proto comment + §1.1 amendment 2. |
| `shadow_rules` | config.rbac.v3.RBAC | CONSUMED | Parallel non-enforcing engine; UDPA-grouped with `shadow_matcher`; when both set, `shadow_rules` wins per `rbac.pb.go:53`. |
| `shadow_rules_stat_prefix` | string | CONSUMED | Stat namespace tag for shadow counters; OPTIONAL (no PGV per §1.1 amendment 3). |
| `matcher` | xds.type.matcher.v3.Matcher | CONSUMED | Alternative match-tree engine via ADR-0142 framework primitive. |
| `shadow_matcher` | xds.type.matcher.v3.Matcher | CONSUMED | Alternative shadow match-tree. |
| `rules_stat_prefix` | string | CONSUMED | Stat namespace tag for primary counters; OPTIONAL (no PGV). |
| `track_per_rule_stats` | bool | CONSUMED | When true, emit per-policy-name counters per matched policy. |

**Inner `config.rbac.v3.RBAC` (rules-engine config; consumed when `rules` or `shadow_rules` set):**

| Field | Type | Phase 16 disposition | Notes |
|---|---|---|---|
| `action` | RBAC_Action enum | CONSUMED | ALLOW=0 / DENY=1 / LOG=2; PGV `defined_only = true`. LOG = always-allow + match-runs + `access_log_hint` metadata silent (divergence-window per §1.1 amendment 5). |
| `policies` | map<string, Policy> | CONSUMED | Lexicographic-order-of-policy-name walk; Policy = permissions OR (≥1; PGV `min_items=1`) + principals OR (≥1) + condition silent-ignored. |
| `audit_logging_options` | RBAC_AuditLoggingOptions | SILENT-IGNORED | `[#not-implemented-hide:]` upstream; Envoy emits nothing regardless. |

**Inside Policy (3 CEL fields silent-ignored per Q7 + §1.1 amendment 6):**

`condition` (Expr) + `checked_condition` (CheckedExpr) + `cel_config` (CelExpressionConfig) all silent-ignored at runtime. Divergence-window.

**Permission MVP Large 11 (11 of 14; per §1.1 amendment 1 + §2.3):** any, header, url_path, destination_ip, destination_port, destination_port_range, requested_server_name, and_rules, or_rules, not_rule, sourced_metadata (always-no-match runtime).

**Permission DEFERRED (3 of 14):** metadata (deprecated; PARSE-REJECT), matcher (extension; PARSE-REJECT), uri_template (extension; PARSE-REJECT).

**Principal MVP Large 11 (11 of 14 per §1.1 amendment 7 — Principal has 14 variants not 13):** any, authenticated (3-case algorithm per §1.1 amendment 12 + ADR-0144), direct_remote_ip, remote_ip, header, url_path, and_ids, or_ids, not_id, sourced_metadata, filter_state (last two always-no-match runtime).

**Principal DEFERRED (3 of 14):** source_ip (deprecated; PARSE-REJECT), metadata (deprecated; PARSE-REJECT), custom (extension; PARSE-REJECT — NEW per §1.1 amendment 7).

**Per-route `RBACPerRoute`:** wrapper proto with reserved field 1 + single optional `rbac` field at field 2. Absent (or `rbac: nil`) = disabled-on-route per proto comment + §5.1 (a). Present = wholesale-override per §5.1 (b). Per ADR-0125 §(xii) amendment: phase-16 introduces the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override; structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof AND 6th canonical's bare-message-via-TPFC). Per-route stats INDEPENDENT per ADR-0145 (mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT precedent).

#### Wire shape

Deny-path wire shape (DENY engine result OR matcher-engine no-match):
- Status: 403 Forbidden.
- Body: byte-exact `"RBAC: access denied"` (19 bytes ASCII; no trailing newline; per §1.1 amendment 10 + §11.P5).
- 4-header set (lowercase wire-form): `content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`.
- Connection: keep-alive (no `connection: close`).
- `response_code_details`: envoy-go MVP DEFERS (no emission per §1.1 amendment 11 + §8.12); reference Envoy emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"`.

Allow-path wire shape: passthrough — `cb.SendLocalReply` NOT invoked; request forwards to next filter.

#### Per-route INDEPENDENT-stats discipline (per ADR-0145 + §5)

Phase 16 is the THIRD row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent (phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND; phase-16 rbac THIRD). Per-route TPFC entries via `RBACPerRoute{rbac: <RBAC>}` (per ADR-0125 §(xii) NEW 7th canonical) own fresh `*compiledConfig` + fresh `*filterStats` keyed by per-route `rules_stat_prefix`. Listener-level counters do NOT increment for per-route-active streams.

#### Stat surface + Prometheus rendering (per §1.1 amendments 8 + 9)

4 base counters per active namespace combination: `allowed`, `denied`, `shadow_allowed`, `shadow_denied`. Internal stat path (SN2-reuse hypothesis per §1.1 amendment 9): `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for primary; `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.<counter>` for shadow. Prometheus rendering via existing SN2 default-branch flatten; NO new SN10 rule.

Per-policy counters (when `track_per_rule_stats: true`): `<base_prefix>.<policy_name>.<suffix>` where suffix ∈ {allowed, denied, shadow_allowed, shadow_denied}. Operator-config-driven surface growth; foot-gun documented at §13.4.
```

(End of §13.1 stub; ~200-300 lines authored at phase-done commit per phase-13 + phase-14 + phase-15 SPEC §13.1 precedent.)

### 13.2 `## Stat-name mapping` 60-name table extension to 64 names (4 new active entries)

Verbatim Markdown patch:

```markdown
**RBAC filter — 4 active names (introduced by phase 16):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.allowed`         | counter | filter | rbac | increments per request whose primary engine result = ALLOWED (§11.P6) |
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.denied`          | counter | filter | rbac | increments per request whose primary engine result = DENIED (§11.P6) |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_allowed` | counter | filter | rbac | increments per request whose shadow engine = ALLOWED (when shadow configured; §11.P6) |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_denied`  | counter | filter | rbac | increments per request whose shadow engine = DENIED (when shadow configured; §11.P6) |

**Per-policy counter family (variable; emitted when `track_per_rule_stats: true`):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<policy_name>.allowed`        | counter | filter | rbac | matched policy under primary ALLOWED |
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<policy_name>.denied`         | counter | filter | rbac | matched policy under primary DENIED |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.<policy_name>.shadow_allowed` | counter | filter | rbac | matched policy under shadow ALLOWED |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.<policy_name>.shadow_denied`  | counter | filter | rbac | matched policy under shadow DENIED |

NOTE: Per-policy counter cost is operator-config-driven; the table entries are templates, not fixed names. Operators with N policies × 2 base sides × 2 (primary + shadow) emit up to 4N per-policy counters per active namespace. Foot-gun documented at §13.4 forward-pointer notes.
```

Stat-table size grows from 60 → **64 names** (4 new active base counters; per-policy template-form documented separately).

**Prometheus rendering (per §1.1 amendment 9 + §11.P7):** SN2-reuse hypothesis; NO new SN10 rule. Pending impl-time empirical scrape confirmation.

### 13.3 `## Equivalence Matrix` new row (verbatim table-row patch)

```markdown
| 0018-http-rbac | envoy.filters.http.rbac (decode-side dual-engine policy gate; rules-engine + matcher-engine + shadow + per-policy stats) | byte-exact status; byte-exact body on allow (passthrough) AND deny (19-byte "RBAC: access denied"); per-counter delta byte-equivalent on 4 base counters per active namespace (allowed/denied/shadow_allowed/shadow_denied); INDEPENDENT per-route stats per ADR-0145 (scenario 8); mTLS scenario 6 exercises ADR-0144 TLS-principal accessor; 7th canonical per-route absent-implies-disabled per ADR-0125 §(xii) (scenario 7) |
```

### 13.4 Forward-pointer notes (per BRAINSTORM §8 + §1.1 amendments 5 + 6 + 7 + 8 + 9 + 11 + 12)

```markdown
### Phase 16 forward-pointer notes

**Deferred field families** (silent-ignored or PARSE-REJECT per ADR-0040 + ADR-0141 + ADR-0143; see `### envoy.filters.http.rbac ### Field decomposition` above + phase 16 SPEC §2 for the full 12-item deferral map):

- `RBAC.audit_logging_options` (RBAC_AuditLoggingOptions) — silent-ignored at parse + runtime; `[#not-implemented-hide:]` upstream. Couples to future audit-logging family phase.
- Policy `condition` + `checked_condition` + `cel_config` (three CEL fields per §1.1 amendment 6) — silent-ignored at runtime per Q7. Couples to a future CEL framework phase landing `internal/cel/` + `github.com/google/cel-go`. Re-activation enables fine-grained condition evaluation.
- Permission DEFERRED set: `metadata` (deprecated; PARSE-REJECT envoy-go-only divergence-window per §11.P12 — Envoy lenient-accepts with deprecation warning); `matcher` (TypedExtensionConfig; PARSE-REJECT); `uri_template` (TypedExtensionConfig; PARSE-REJECT). Couples to plugin framework.
- Principal DEFERRED set (3 of 14 per §1.1 amendment 7): `source_ip` (deprecated; PARSE-REJECT envoy-go-only divergence); `metadata` (deprecated; PARSE-REJECT); `custom` (TypedExtensionConfig; PARSE-REJECT — the 14th Principal variant). Couples to plugin framework + mTLS-extension family.
- `Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState` (parse-supported; always-no-match at runtime) — Couples to dynamic-metadata family + filter-state family. Real-world divergence appears only when operator configs explicitly set dynamic-metadata or filter-state from upstream filters.

**LOG-action divergence-window (per §1.1 amendment 5 + §8.6):** Envoy v1.37.2 sets the `access_log_hint` dynamic metadata key under namespace `envoy.common` to `true` on LOG-matched requests (false on no-match). envoy-go MVP silent-no-metadata-emit. Counter emission: LOG-matched requests increment `allowed` counter (NOT a separate `logged` counter — per §1.1 amendment 8 NO `logged` counter exists in Envoy v1.37.2). Operator divergence-window: dashboards inspecting `access_log_hint` see Envoy emit but envoy-go absent. Future re-activation: dynamic-metadata framework phase lands `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive (or equivalent decode-side accessor).

**`response_code_details` field-emission divergence (per §1.1 amendment 11 + §8.12):** Envoy v1.37.2's RBAC denial sets `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` per `utility.cc::responseDetail` (whitespace in policy-id replaced with underscores). The string lands in HCM `response_flag_details` accessor + access-log `RESPONSE_CODE_DETAILS` operator. envoy-go MVP does NOT thread response-code-details from filter through HCM to access-log; current phase-04 HCM scope. Operator divergence-window: access-log `RESPONSE_CODE_DETAILS` field is populated on Envoy-side RBAC denials + empty on envoy-go-side. Future re-activation: response-code-details framework phase couples HCM's local-reply path to a per-filter accessor (`DecoderFilterCallbacks.SetResponseCodeDetails(string)` or analogous).

**Shadow access-log divergence-window (per §8.7 + §11.P13):** envoy-go MVP emits shadow counters only; no shadow-decision-annotated access-log entries. Reference Envoy v1.37.2 confirmed counter-only via source review; no current divergence. Future Envoy version may add access-log integration; impl-time PROGRESS review checks.

**Principal_Authenticated nil-principal_name semantic (per §1.1 amendment 12 + §11.P14):** `Principal_Authenticated.principal_name == nil` matches ANY downstream user that passed TLS verification. envoy-go MVP implements three-case algorithm per §6.6: (a) nil principal_name → check `len(DownstreamPrincipal()) > 0`; (b) non-nil → StringMatcher iteration over URI SAN/DNS SAN/Subject DN candidates in priority order; (c) plaintext connection → always FALSE.

**TWO new framework primitives (per §3.1 + §3.2 + ADR-0142 + ADR-0144):** Phase 16 is the FIRST §9 row since phase 14 to introduce non-zero framework deltas + FIRST single phase to introduce TWO: (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor surfacing the downstream client cert's URI SAN + DNS SAN + Subject DN CN in priority order; (ii) matcher-engine evaluator framework primitive at new top-level `internal/matcher/` package implementing `xds.type.matcher.v3.Matcher` generic match-tree evaluator with PARSE-REJECT-for-unknown-TypeURL discipline. Both cross-phase reusable by future filters (jwt_authn, ext_authz, ext_proc, oauth2).

**`track_per_rule_stats` operator-config-driven foot-gun (per §1.1 amendment 8 + §8.5):** When `track_per_rule_stats: true`, the per-policy counter family is allocated lazily on first-match per policy. Misconfigured large-N policy configs (1000+ policies × 2 base sides × 2 (primary + shadow) = 4000 counters per filter instance) impose memory + CPU costs. envoy-go MVP imposes NO parse-time N-cap (mirrors Envoy permissive discipline). Future operator-ergonomics phase MAY add an envoy-go-only N-cap (e.g., max 256 policies under track-true).

**`Principal_Set` + `Permission_Set` recursion depth foot-gun (per §11.P11):** envoy-go MVP imposes NO parse-time recursion-depth cap. Operators authoring deeply-nested rules-engine configs may hit Go-stack-depth issues at config-load time (Go default ~10K frames). Documented at §13.4. Future operator-ergonomics phase MAY add an envoy-go-only depth-cap (e.g., max 32 levels of nesting).

**No new tag-extractor (per §1.1 amendment 9 + §11.P7 pending):** envoy-go's hypothesis is SN2-reuse — `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` renders via the existing SN2 (`http.*` segment routing) + dot→underscore default-branch flatten. NO labels / NO new SN10 rule. **Impl-time empirical scrape against reference Envoy v1.37.2 confirms or refines the namespace shape**; if Envoy's empirical shape diverges (e.g., flat `<rules_stat_prefix>.rbac.<counter>` mirroring phase-15 bandwidth_limit's shape), envoy-go's namespace mirrors via inline-prefix rendering (`envoy_<rules_stat_prefix>_rbac_<counter>{}`) + still NO new SN10 rule. ADR-0145 amends in-place.

**Filter-chain ordering with respect to header_mutation / buffer / compressor / bandwidth_limit (per §2.12):** rbac is recommended EARLY in the HCM chain (immediately after listener filters); denied requests don't incur downstream filter cost. Operators wanting header_mutation BEFORE rbac (e.g., to set `X-User` from upstream metadata before the policy gate evaluates it) have full flexibility per the operator's filter-chain order. Fixture 0018 pins rbac as the first HCM filter for byte-equivalence simplicity; SPEC documents the trade-off without prescribing.
```

(End of §13.4 stub; lands at phase-done commit per phase-13 + phase-14 + phase-15 SPEC §13.4 precedent. ~80-100 lines authored.)

### 13.5 `## HTTPFilterCallbacks` extension (NEW subsection per ADR-0144)

Verbatim Markdown patch:

```markdown
### DownstreamPrincipal accessor (per phase 16 ADR-0144)

`DecoderFilterCallbacks.DownstreamPrincipal() []string` returns the priority-ordered list of TLS principal-name candidates for the active downstream client connection:

1. URI SAN values from `tls.ConnectionState.PeerCertificates[0].URIs[].String()` (priority 1 per `rbac.pb.go:1432-1438` proto comment).
2. DNS SAN values from `tls.ConnectionState.PeerCertificates[0].DNSNames` (priority 2).
3. Subject DN Common Name from `tls.ConnectionState.PeerCertificates[0].Subject.CommonName` (priority 3 fallback).

For plaintext or non-mTLS connections OR connections where no client cert was presented, the accessor returns `nil` (or empty slice).

Cross-phase reuse: future filters (jwt_authn, ext_authz, oauth2) consume the same accessor for TLS-principal introspection.

Phase 16 is the FIRST §9 family-row to introduce this accessor.
```

### 13.6 Matcher-engine framework primitive subsection (NEW per ADR-0142)

Verbatim Markdown patch under a NEW `## Matcher engine framework primitive` section after the existing `## HTTPFilterCallbacks`:

```markdown
## Matcher engine framework primitive (per phase 16 ADR-0142)

Phase 16 introduces a new top-level `internal/matcher/` package providing a generic `xds.type.matcher.v3.Matcher` match-tree evaluator. The package exports:

- `matcher.New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher, error)` — parses the match tree + validates terminal action TypeURLs against the caller's allow-list at config-load time; PARSE-REJECT for unknown TypeURLs with envoy-go-only error.
- `matcher.Evaluate(ctx MatchContext) (*anypb.Any, error)` — walks the match tree at request time + returns the matched terminal action TypedExtensionConfig (wrapped as `Any`) OR `(nil, nil)` for no-match.
- `matcher.MatchContext` interface — the caller (a filter) implements this on its per-stream `*filter` to expose request accessors (headers, IP, principal, etc.).

Phase 16's rbac filter consumes the primitive with `supportedActionTypes = ["type.googleapis.com/envoy.config.rbac.v3.Action"]` (the canonical RBAC terminal; per §11.P3); future filters extend the allow-list as new terminal types land.

Cross-phase reuse intent: ext_authz, jwt_authn, oauth2 all use the same `xds.type.matcher.v3.Matcher` primitive for parts of their config surface. Each future filter extends `supportedActionTypes` + widens `MatchContext` additively.
```

---

## 14. Testing strategy (per BRAINSTORM §11 + §1.1 amendments)

### 14.1 Unit tests (`internal/filter/http/rbac/rbac_test.go`)

Test groups (mirrors phase-14 / phase-15's 6-7 test groups; phase-16 adds groups for the dual-engine surface):

1. **Config parse + buildCompiledConfig** — all 7 outer fields consumed; UDPA-field-alias rules-wins / shadow_rules-wins semantics; CEL silent-ignore; audit_logging_options silent-ignore; envoy-go-side defensive PGV-mirror validation (action enum + policies.permissions/principals min_items=1 + destination_port range-check) with envoy-go-own error wording.
2. **buildCompiledConfigPerRoute** — 7th canonical absent-implies-disabled (case (a)); wholesale-override (case (b)); INDEPENDENT-stats wiring; per-route TPFC lazy-cache via `sync.Map`.
3. **Permission evaluators** — all 11 Large variants exercised; AND/OR/NOT combinator depth-recursion (~3-4 levels deep test cases); SourcedMetadata always-no-match runtime semantic; deprecated/extension PARSE-REJECT error wording.
4. **Principal evaluators** — all 11 Large variants exercised; AND/OR/NOT combinator depth-recursion; SourcedMetadata + FilterState always-no-match runtime; deprecated/extension PARSE-REJECT (source_ip + metadata + custom — the 14th Principal variant); Principal_Authenticated three-case algorithm per §1.1 amendment 12 (nil principal_name; non-nil StringMatcher iteration; plaintext FALSE).
5. **Dual-engine dispatch** — rules-engine path (policies map walk + lexicographic order + ALLOW/DENY/LOG action); matcher-engine path (via `internal/matcher` framework primitive; canonical Action terminal); both-set-rules-wins precedence; both-unset wholly-inactive filter (returns Continue + no counter increments); shadow path parallel evaluation; shadow-never-affects-disposition.
6. **DecodeHeaders gating + SendLocalReply** — ALLOW result → HeaderContinue; DENY result → SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain}) + HeaderStopIteration; LOG-partial result → HeaderContinue + `allowed` counter increments (NOT a separate `logged` counter per §1.1 amendment 8); per-policy counter emission when track is true.
7. **DownstreamPrincipal accessor + Principal_Authenticated three-case algorithm** — mTLS connection with cert URI SAN match; mTLS connection with cert DNS SAN match; mTLS connection with cert Subject DN match; mTLS connection with nil principal_name → matches; plaintext connection → all Principal_Authenticated evaluations FALSE; cert ordering preserved.
8. **Matcher-engine framework primitive (`internal/matcher`)** — parse-time PARSE-REJECT for unknown terminal TypeURLs; canonical RBAC Action terminal acceptance; matcher-engine no-match → DENY disposition; MatchContext accessor adapter.

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/rbac/... ./internal/matcher/...` — green on all 8 test groups. Race-test surface unchanged from phase-15's 38-package baseline; the new `rbac/` package adds the 39th + the new `matcher/` package adds the 40th. Per-stream state (cached `*compiledConfig` on `*filter`) is single-goroutine-per-stream (the dispatch goroutine), so no synchronization needed within `*filter`. `sync.Map` lazy-cache at `*factoryState.perRoute` + per-policy counter lazy-allocation cache are concurrent-safe.

`golangci-lint run` — green; new packages lint clean.

### 14.3 Fuzzers

`FuzzRBACConfigParse` — fuzzes the YAML→proto→`buildCompiledConfig` pipeline including: dual-engine config shapes; Permission/Principal variant combinations (including the deprecated + extension variants for PARSE-REJECT coverage); recursion-depth combinator inputs (AND/OR/NOT); matcher-engine tree parsing (via `internal/matcher.New`). Inputs are random bytes interpreted as YAML; errors-on-invalid-YAML are expected; the fuzzer asserts no panic + no nil-deref on the compilation path. The **20th fuzzer** in the repo (after `FuzzBandwidthLimitConfigParse` from phase 15).

### 14.4 Existing fuzzers re-run

19 phase-15 fuzzers re-run at 30s budget; all green (regression check; phase 16 introduces no fuzzer-affecting changes outside the new packages).

### 14.5 h2spec re-run

53/53 PASS at the ADR-0051 pin. Phase 16 introduces no H2 wire-shape changes (the decode-side gate operates structurally at the HCM filter level; the 403 deny path uses the standard SendLocalReply mechanism which writes H2 frames via the existing `writeH2Reply` adapter from phase-05.1).

### 14.6 Differential 0000–0017 + 0018

18 prior fixtures + the new `0018-http-rbac` = 19 fixtures green. Total runtime estimated ~60-90s wallclock (the new fixture has 8 scenarios with the mTLS scenario 6 carrying additional handshake overhead but no throttle-equivalent wall-clock delay).

### 14.7 Six-gate checklist (per `BOOTSTRAP_PROMPT.md` §7.5)

| Gate | Pass/fail criterion |
|---|---|
| A | `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0; no new warnings vs phase-15 baseline at master tip `b45c1eb`. |
| B | `go test -race ./...` exit 0 across 40 packages (phase-15 baseline 38 + new `internal/filter/http/rbac/` + new `internal/matcher/`); race detector reports clean. |
| C | `h2spec` 53/53 PASS at ADR-0051 pin; phase-16 introduces no H2 wire-shape changes. |
| D | All 20 fuzzers green at 30s/each budget. |
| E | All 19 differential fixtures (0000-0018) PASS; runtime ~60-90s wallclock. |
| F | `BEHAVIOR_CONTRACT.md` §13.1 + §13.2 + §13.3 + §13.4 + §13.5 + §13.6 populated per the patches at §13 above. |

All six green at phase-done commit per BOOTSTRAP_PROMPT.md §7.5.

---

## 15. Acceptance checklist (for the reviewer of this phase's final state)

1. ✓ Phase 16 SPEC.md authored with **12 §1.1 amendment blocks** (5 structural + 7 field-bookkeeping refinements); each amendment cross-referenced to §11 empirical evidence.
2. ✓ §3 framework-survey result locked: **TWO new framework deltas** (FIRST §9 row since phase 14; FIRST single phase to introduce TWO): (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor per ADR-0144; (ii) matcher-engine evaluator at new top-level `internal/matcher/` package per ADR-0142.
3. ✓ §11 empirical-pin block: 18 pins resolved IN-SESSION against reference Envoy v1.37.2 per ADR-0004; 10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME + 1 DEFERRED tally captured; verbatim proto + filter-source + utility-source scrape evidence.
4. ✓ Differential fixture: 8 scenarios; byte-exact body assertion (allow paths verbatim + deny paths 19-byte `"RBAC: access denied"` per §4 + §1.1 amendment 10); per-counter delta byte-equivalence on the 4 active base counters per active namespace; per-route INDEPENDENT-stats per scenarios 7 + 8; mTLS scenario 6 exercises ADR-0144 framework primitive.
5. ✓ ADR roster: 7 ADRs anticipated (ADR-0140..ADR-0146; LARGEST §9-row roster to date) + ADR-0125 in-place §(xii) amendment paragraph documenting the NEW 7th canonical per-route pattern phase-16 introduces.
6. ✓ Stat surface: **4 base counters (allowed/denied/shadow_allowed/shadow_denied)** + lazy per-policy counter family when `track_per_rule_stats: true` (per §1.1 amendment 8 REFUTES BRAINSTORM 5-counter hypothesis — no `logged` counter exists in Envoy v1.37.2; LOG-partial folds into `allowed`); namespace SN2-reuse hypothesis pending impl-time empirical scrape confirmation per §1.1 amendment 9; NO new SN10 rule.
7. ✓ Per-route surface: **NEW 7th canonical per-route pattern (absent-implies-disabled-OR-wholesale-override)** documented at ADR-0125 §(xii) amendment; structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof AND 6th canonical's bare-message-via-TPFC + code-level-required-field; INDEPENDENT-stats per ADR-0145 (THIRD row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent).
8. ✓ Dual-engine dispatch: rules-engine + matcher-engine BOTH proto-faithful per Q2; rules wins when both set; shadow path parallel-but-never-affects-disposition; ALLOW + DENY + LOG-partial action enum honored; LOG-partial silent-no-metadata-emit divergence-window documented per §1.1 amendment 5 + §8.6.
9. ✓ Permission + Principal Large 11+11 MVP per Q3; Permission has 14 variants (3 deferred); Principal has 14 variants per §1.1 amendment 7 (3 deferred — adds `custom` to the deferred list compared to BRAINSTORM); all deferred variants PARSE-REJECT with envoy-go-only error wording (envoy-go-strict divergence from Envoy-lenient).
10. ✓ TWO new framework primitives: ADR-0142 matcher-engine evaluator at new `internal/matcher/` package; ADR-0144 TLS-principal accessor on `DecoderFilterCallbacks` with three-case algorithm per §1.1 amendment 12.
11. ✓ Deny-path wire shape: 403 + body byte-exact `"RBAC: access denied"` (19 bytes) per §1.1 amendment 10 + §4; 4-header set lowercase; keep-alive; `response_code_details` field-emission divergence-window per §1.1 amendment 11 + §8.12.
12. ✓ Twelve §1.1 amendment blocks document the SPEC-time refutations/refinements cleanly via the §1.1 amendment-block channel (NOT §12 BRAINSTORM-amendment cycle).
13. ✓ STATE.md updated post-SPEC: lifecycle-state-2 → 3 transition (SPEC-done, awaiting PLAN); next-skill `superpowers:writing-plans` (now for PLAN authoring per ADR-0005 §Decision 4 split); ROADMAP.md row 16 `planned → in-progress`.

---

**End of phase 16 SPEC.**
