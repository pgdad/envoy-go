# Phase 16 Brainstorm — `envoy.filters.http.rbac`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 16 (`http-filter-rbac`), the NINTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, `compressor` at phase 14, and `bandwidth_limit` at phase 15). The next session (lifecycle-state 1 → 2 for phase 16, skill `superpowers:writing-plans` per ADR-0005, routed through the SPEC-authoring step first per the phase 09/10/11/12/13/14/15 precedent) authors `docs/envoy-go/phases/16-http-filter-rbac/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-16-http-filter-rbac-brainstorm`, branch `phase-16-http-filter-rbac-brainstorm`, branched from master tip `98a8ca6` (the phase 15 impl follow-up STATE.md SHA-fill + lifecycle-state-6 narrative commit). The phase 15 squash-merge commit `c1361d3` and its earlier SHA-fill follow-up `98a8ca6` are the immediate predecessors on master; phase 15's PLAN squash `a5c5ec9` + PLAN SHA-fill `36c91c9` are earlier still. `98a8ca6` is the most recent master tip.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via 7-question dialogue (Q1 §9 family-row pick — `rbac` chosen from the 10-candidate remaining list `jwt_authn / rbac / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control / global_ratelimit`; Q2 engine MVP — `BOTH engines proto-faithful (rules + matcher)` chosen from `rules-only-silent-ignore-matcher / rules-only-PARSE-REJECT-matcher / matcher-only / BOTH`; Q3 Permission/Principal subset — `Large 11+11` chosen from `Small 8+7 / Tiny 5+4 / Large 11+11`; Q4 action enum — `ALLOW+DENY+LOG-partial` chosen from `ALLOW+DENY-only / +LOG-partial / +LOG-PARSE-REJECT`; Q5 shadow + track_per_rule_stats — `Both proto-faithful` chosen from `shadow-only / both-proto-faithful / both-silent-ignored / both-bounded`; Q6 per-route canonical pattern — `NEW 7th canonical (absent-implies-disabled)` chosen from `7th-canonical / degenerate-5th / wrap-as-5th-no-amendment`; Q7 CEL conditions — `Silent-ignored` chosen from `silent-ignored / PARSE-REJECT / support-via-cel-go`). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0139, where ADR-0135-ADR-0139 landed in phase 15), and the just-shipped phase 15 + phase 14 + phase 13 + phase 12 + phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 + 12 + 13 + 14 + 15 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/15-http-filter-bandwidth-limit/BRAINSTORM.md` and `docs/envoy-go/phases/14-http-filter-compressor/BRAINSTORM.md` section-for-section, reframed for the rbac scope and adapted for its specific surface area. Phase 16 sits in a structurally important position relative to the §9 family: it is the FIRST §9 family-row whose proto surface is genuinely a **policy-engine subsystem** rather than a single-config-knob filter — `Permission` has 14 oneof variants, `Principal` has 13, `action` is a tri-valued enum, and the filter consumes TWO PARALLEL evaluation engines (`rules` and `matcher`) plus a parallel shadow track. Per the Q2 + Q3 user picks (proto-faithful + Large 11+11), phase 16 commits to **TWO new framework primitives** — the first non-zero-framework-delta §9 row since phase 14: (i) a TLS-principal accessor on `DecoderFilterCallbacks` required by `Principal_Authenticated`; (ii) a matcher-engine evaluator for `xds.type.matcher.v3.Matcher`. Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 16 — this brainstorm cold-started fresh from the §9 heading + the phase 15 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-12.

---

## 1. Mission and scope confirmation (16 only)

ROADMAP row `16 | http-filter-rbac | 15 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 16 is the NINTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 62 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 15 squash-merge commit `c1361d3` (with SHA-fill at `98a8ca6`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 64: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114); `csrf` shipped in phase 12 (`internal/filter/http/csrf/` per ADR-0120); `buffer` shipped in phase 13 (`internal/filter/http/buffer/` per ADR-0125); `compressor` shipped in phase 14 (`internal/filter/http/compressor/` per ADR-0129–ADR-0134); `bandwidth_limit` shipped in phase 15 (`internal/filter/http/bandwidthlimit/` per ADR-0135–ADR-0139). Phase 16 ships **role-based access control as a decode-side policy gate** as the NINTH real filter — the canonical Envoy-style "evaluate policies against the request; allow / deny / log" filter. The chosen branch + directory + Go-package identifier are all `rbac` (single token; matching the Envoy filter type-URL `envoy.filters.http.rbac` exactly; no underscore-stripping per ADR-0114 — `rbac` has no underscore in either side).

Phase 16 is also: (i) the FIRST §9 family-row whose proto surface is a **genuine policy-engine subsystem** — Permission has 14 oneof variants, Principal has 13, the filter consumes TWO PARALLEL evaluation engines (`rules` and `matcher`), each with optional parallel `shadow_*` tracks; (ii) the FIRST §9 family-row to introduce **TWO new framework primitives in a single phase** — the TLS-principal accessor on `DecoderFilterCallbacks` (required by `Principal_Authenticated`) AND the matcher-engine evaluator for `xds.type.matcher.v3.Matcher` (reusable by future filters like ext_authz, jwt_authn); (iii) the FIRST §9 family-row to introduce a **NEW canonical per-route pattern** since phase 15 codified the 6th — RBAC's `RBACPerRoute{rbac: RBAC|nil}` where nil-means-disabled is structurally distinct from the 5th (oneof-with-disabled-bool; phase-13/14) and 6th (bare-message-via-TPFC with code-level-required field; phase-15), warranting a 7th-canonical entry in ADR-0125 amendment §(xii); (iv) the FIRST §9 family-row whose stat surface **grows variably with operator config** — `track_per_rule_stats: true` adds 2N per-policy counters per `rules_stat_prefix` namespace (foot-gun for misconfigured large-N policies); (v) the SECOND §9 family-row using INDEPENDENT-per-route stats discipline (mirrors phase-11 local_ratelimit + phase-15 bandwidth_limit per ADR-0117 + ADR-0139; per-route override carries its own `rules_stat_prefix` namespace driven by policy-state-per-route distinction).

### 1.1 What 16 delivers as a self-contained whole

Phase 16 lands `envoy.filters.http.rbac` (the canonical Envoy role-based-access-control filter, ALLOW + DENY + LOG-partial, dual-engine, shadow-mode, per-rule-stats-capable) under the 07.1 framework. **Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):**

1. **New `internal/filter/http/rbac/` package** owning the filter implementation. Package directory + Go package identifier are both `rbac` (single token; matches the Envoy filter type-URL `envoy.filters.http.rbac` exactly). Files mirror the multi-file structure of phase 14 + phase 15 (the precedent for larger filters): `rbac.go` (filter type + factory + decode methods + filterStats struct + compiledConfig + per-route helper), `evaluator.go` (Permission + Principal evaluators + AND/OR/NOT combinators + the dual-engine dispatch — calls into the rules-engine policy-map walk OR the matcher-engine match-tree walk), `rbac_test.go` (unit tests; anticipated 600-900 LoC given the evaluator subsurface), `fuzz_test.go` (the 20th fuzzer in the repo — `FuzzRBACConfigParse`), `doc.go` (package overview + 7-decision summary + Large-11+11 + dual-engine + shadow + LOG-partial + per-route 7th-canonical summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor / bandwidth_limit precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 10 entries after phase 15: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New` before the `httpReg.Freeze()` invocation) gains an eleventh `httpReg.Register(rbac.TypeURL, rbac.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoy_go_test → fault → header_mutation → local_ratelimit → rbac → Freeze`. `rbac` inserts between `local_ratelimit` and `Freeze` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing of `envoy.extensions.filters.http.rbac.v3.RBAC`,** the canonical filter-level config message. Per `go-control-plane/envoy v1.32.4` (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 7 top-level fields; phase 16 **consumes ALL 7** in the proto-faithful posture per Q2 + Q5 user picks. **Consumed at runtime (7 fields):**

   - `rules` (`config.rbac.v3.RBAC`) — primary policy engine; oneof rules_specifier with `matcher`. When BOTH set: `rules` WINS per proto semantic; envoy-go mirrors.
   - `matcher` (`xds.type.matcher.v3.Matcher`) — alternative match-tree engine; oneof rules_specifier with `rules`. Consumed when `rules` is unset.
   - `rules_stat_prefix` (string) — stat namespace tag for primary counters; empty default permitted.
   - `shadow_rules` (`config.rbac.v3.RBAC`) — parallel non-enforcing policy engine; oneof shadow_rules_specifier with `shadow_matcher`. When BOTH set: `shadow_rules` WINS per proto semantic.
   - `shadow_matcher` (`xds.type.matcher.v3.Matcher`) — alternative shadow match-tree; oneof shadow_rules_specifier with `shadow_rules`.
   - `shadow_rules_stat_prefix` (string) — stat namespace tag for shadow counters; empty default permitted.
   - `track_per_rule_stats` (bool) — when true, emit per-policy-name counters (2N counters per active `rules_stat_prefix` + 2N per active `shadow_rules_stat_prefix`; foot-gun for large-N configs; §11 pin §16.P10 measures resource-budget impact).

   **Inside `config.rbac.v3.RBAC` (the rules engine; consumed when `rules` is set):**

   - `action` enum: ALLOW / DENY / LOG-partial honored per §2.4 + Q4.
   - `policies` map<string, Policy>: each Policy has `permissions` []Permission (≥1; OR-semantic), `principals` []Principal (≥1; OR-semantic), `condition` (CEL; SILENT-IGNORED per Q7 + §2.7), `checked_condition` (CEL; SILENT-IGNORED). Policies evaluated in lexicographic order of policy name.
   - `audit_logging_options` — SILENT-IGNORED (marked `[#not-implemented-hide:]` upstream; never emitted by Envoy v1.37.2 regardless of config).

   **Inside `xds.type.matcher.v3.Matcher` (the matcher engine; consumed when `matcher` is set):**

   - The match-tree structure with predicates → on_match actions evaluating to RBAC `Action` terminal types. §11 pin §16.P3 confirms the exact terminal-extension-config TypeURL set Envoy v1.37.2 uses for the on_match action; envoy-go MVP supports the canonical terminal types only (further extension TypeURLs PARSE-REJECTED with envoy-go-only error).

   **Permission MVP subset (11 of 14; Q3 Large pick):** `any`, `header` (HeaderMatcher), `url_path` (PathMatcher), `destination_ip` (CidrRange), `destination_port` (uint32), `destination_port_range` (Int32Range), `requested_server_name` (StringMatcher; SNI), `and_rules` (Permission_Set recursive), `or_rules` (Permission_Set recursive), `not_rule` (Permission recursive), `sourced_metadata` (SourcedMetadata; parse-supported with always-no-metadata-match runtime divergence-window per coupling-to-dynamic-metadata-family deferral). DEFERRED: `metadata` (DEPRECATED upstream; superseded by `sourced_metadata`), `matcher` (extension TypedExtensionConfig; couples to plugin framework), `uri_template` (extension TypedExtensionConfig; couples to plugin framework).

   **Principal MVP subset (11 of 13; Q3 Large pick):** `any`, `authenticated` (Principal_Authenticated; TLS principal-name match — requires NEW framework primitive per §3.1 + ADR-0144), `direct_remote_ip` (CidrRange; peer-address match), `remote_ip` (CidrRange; XFF-resolved match), `header` (HeaderMatcher), `url_path` (PathMatcher), `and_ids` (Principal_Set recursive), `or_ids` (Principal_Set recursive), `not_id` (Principal recursive), `sourced_metadata` (SourcedMetadata; parse-supported with always-no-metadata-match runtime divergence-window), `filter_state` (FilterStateMatcher; parse-supported with always-no-filter-state-match runtime divergence-window per coupling-to-filter-state-framework deferral). DEFERRED: `source_ip` (DEPRECATED upstream; superseded by `direct_remote_ip` + `remote_ip`), `metadata` (DEPRECATED upstream).

4. **Per-route TPFC: NEW 7th canonical pattern (absent-implies-disabled OR wholesale-override; ADR-0125 amendment §(xii)).** Per the proto message `RBACPerRoute`, per-route entries carry a SINGLE field `rbac` (RBAC; field number 2 — field 1 is reserved per proto, was likely a removed `disabled` boolean in an earlier proto version). The proto comment explicitly states: "If absent, RBAC policy will be disabled for this route." Two cases:
   - (a) `RBACPerRoute{rbac: nil}` → the filter is wholly inactive on this route, no policy enforcement, no counter increments, request forwards as-is past the gate.
   - (b) `RBACPerRoute{rbac: <RBAC>}` → a WHOLESALE override of the listener-level RBAC message (NOT a merge; mirrors phase-13/14/15's wholesale-not-merge per ADR-0125 §(xi) + per ADR-0073). The per-route override carries its own `rules`, `matcher`, `rules_stat_prefix`, `shadow_rules`, `shadow_matcher`, `shadow_rules_stat_prefix`, `track_per_rule_stats`; if it owns its own `rules_stat_prefix` it emits to an INDEPENDENT counter namespace per §2.6 + §5 INDEPENDENT-stats discipline (mirrors phase-11 ADR-0117 + phase-15 ADR-0139). Both shapes honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (nil-rbac OR non-nil-RBAC) drives the disposition.

   **Phase 16 is the FIRST row to use the absent-implies-disabled discipline** — distinct from the 5th canonical (explicit `disabled` boolean oneof; phase-13/14) and the 6th (bare-message-via-TPFC + code-level-required field; phase-15). ADR-0125 gains an in-place amendment paragraph §(xii) codifying the 7th canonical pattern + noting the WHOLESALE-not-merge override semantic for the entire RBAC message. This is a NEW canonical pattern, NOT an extension of the 5th: the 5th canonical's defining feature is the explicit-disabled-bool-in-oneof which makes "disabled" a first-class config knob; the 7th canonical's defining feature is the absence-implies-disabled-via-proto-comment which makes "disabled" a side-effect of the message being absent. Both signaling-by-absence and signaling-by-explicit-bool are valid; future filters whose per-route proto uses one or the other compose against the matching canonical.

5. **Filter-callback shape: `StreamDecoderFilter` ONLY on the `*filter` instance.** Phase 16 is decode-side only — `RBAC` is a request-gate filter, evaluated at `DecodeHeaders` time, with the disposition (allow / deny / log) computed BEFORE the request body is forwarded. The filter does NOT implement `StreamEncoderFilter`. Static blank-identifier compile-time check for `StreamDecoderFilter` only. The decode-side surface: `DecodeHeaders(headers, endStream)` resolves per-route → caches `*compiledPerRoute` on filter state → runs the dual-engine evaluation (primary rules-or-matcher, shadow rules-or-matcher) → emits the appropriate counter delta → returns `HeaderContinue` (allow) OR invokes `cb.SendLocalReply(403, ...)` and returns `HeaderStopIteration` (deny). `DecodeData` + `DecodeTrailers` pass-through. `OnDestroy` no-op (no timers; no state to release; mirrors phase-12 csrf precedent).

6. **Dual-engine evaluation: rules-engine path + matcher-engine path (Decision #2 → ADR-0141 + ADR-0142).** Per Q2 = "BOTH engines proto-faithful", the algorithm:

   **Rules-engine path** (when `rules` is set in the consumed config):
   1. Walk `policies` map in lexicographic key order. For each Policy:
      - Evaluate `permissions[]` (OR-semantic; short-circuit on first match) via Permission evaluators (the 11-variant set).
      - Evaluate `principals[]` (OR-semantic; short-circuit on first match) via Principal evaluators (the 11-variant set).
      - Skip `condition` evaluation (CEL silent-ignored per Q7); treat condition as always-true.
      - Policy MATCHES if both permissions-OR and principals-OR are TRUE.
   2. After walking all policies, the aggregate match decision is "any policy matched" or "no policy matched".
   3. Apply `action`:
      - `ALLOW`: allow iff any-policy-matched; deny iff no-policy-matched.
      - `DENY`: allow iff no-policy-matched; deny iff any-policy-matched.
      - `LOG-partial`: ALWAYS allow; metadata not emitted per Q4-partial divergence-window.

   **Matcher-engine path** (when `matcher` is set in the consumed config):
   1. Walk the match-tree per the `xds.type.matcher.v3.Matcher` semantics: predicates evaluate against the request; the first matching predicate's `on_match` returns the action.
   2. The terminal action is a `TypedExtensionConfig` referencing the RBAC `Action` enum (or one of the canonical terminal types §11 pin §16.P3 enumerates).
   3. Per proto semantic ("requests not matching any matcher will be denied"): if the match-tree walk completes without a matching predicate, the disposition is DENY.

   **Shadow path** (when `shadow_rules` or `shadow_matcher` is set, parallel to primary):
   1. Run the parallel shadow-engine walk (same algorithm as primary, on the shadow-engine config).
   2. Emit `shadow_allowed` / `shadow_denied` counters per §2.8.
   3. Shadow path NEVER affects the actual disposition — it is purely an observability emission.

   **Counter emission** (per §2.8 + ADR-0145): on each request, emit ONE primary-path counter (`allowed` or `denied`) per active `rules_stat_prefix`, ONE shadow-path counter (`shadow_allowed` or `shadow_denied`) per active `shadow_rules_stat_prefix` (if shadow configured), and (if `track_per_rule_stats: true`) per-policy-name counters for each matched policy per active prefix.

   **NEW framework primitives** (per §3): (i) TLS-principal accessor on `DecoderFilterCallbacks` exposing the downstream client cert URI SAN / DNS SAN / Subject DN — required by `Principal_Authenticated`. (ii) Matcher-engine evaluator — `xds.type.matcher.v3.Matcher` generic match-tree evaluator. Both are first-time framework introductions in phase 16; ADR-0144 + ADR-0142 anchor.

   **Wire-shape conformance with reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-16 forward-pointer notes): allow-decisions pass through with byte-equivalent headers + body (NO mutation). Deny-decisions emit `SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` mirroring the phase-09 / phase-11 / phase-12 wire-shape discipline; §11 empirical pin §16.P5 confirms exact byte-content of the 403 body + 4-header set.

7. **Stat surface — 60→~66-name minimum extension; variably larger (Decision implicit in stat-surface hypothesis → ADR-0145).** **5 base counters + 2N per-rule counters (variable)** under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`, extending the phase-15 60-name table:

   Base counters (5 per active `rules_stat_prefix` + `shadow_rules_stat_prefix` namespace combination):
   - `allowed` — counter; increments per request the primary engine ALLOWED.
   - `denied` — counter; increments per request the primary engine DENIED.
   - `logged` — counter; increments per request when `action: LOG` per Q4-partial; the request always allows but the counter distinguishes the action mode.
   - `shadow_allowed` — counter; increments per request the shadow engine WOULD HAVE allowed (configured when shadow_rules or shadow_matcher set).
   - `shadow_denied` — counter; increments per request the shadow engine WOULD HAVE denied.

   Per-rule counters (when `track_per_rule_stats: true`; 2N per active namespace):
   - `policy.<name>.allowed` — counter; increments per request where Policy `<name>` matched primary.
   - `policy.<name>.denied` — counter; increments per request where Policy `<name>` matched primary AND action was DENY.
   - Plus shadow variants under `shadow_rules_stat_prefix`.

   §11 pin §16.P6 confirms exact stat names + scope + counter-vs-gauge disposition (e.g., `logged` may or may not be a separate counter — Envoy might emit only `allowed` regardless of action mode and rely on access-log distinction). §11 pin §16.P7 confirms exact Prometheus tag-extractor name + namespace flattening rule (hypothesis: SN2 reuse — the existing HCM-stat-prefix tag-extractor handles this without amendment; SN10 introduced only if pin demands).

   **Stat surface count summary:**
   - Phase 15 (bandwidth_limit): 46 → 60 names (14 new active counter/gauge; SN2 reuse; +2 deferred-histogram via twin-series-filter divergence-window per ADR-0138).
   - Phase 16 (rbac): 60 → **65+ names** (5 new base counters minimum; variable upward with `track_per_rule_stats: true` per operator config; SN2 reuse hypothesis with SN10 introduced only if §11 pin §16.P7 demands).

   **Per-route stats discipline: INDEPENDENT hypothesis** (§11 pin §16.P9 confirms). Rationale: per-route owns its own policy-set state + its own `rules_stat_prefix` as own emission scope (mirrors phase-11 local_ratelimit per ADR-0117 + phase-15 bandwidth_limit per ADR-0139; DIVERGES from phase-12/13/14 SHARED-stats per ADR-0124/ADR-0125/ADR-0132). The stateful-policy-set-per-route precedent is the load-bearing motivator: each per-route override OWNS its own policy-match-evaluation state (different policy-map / different matcher-tree / different per-policy-counter-set), so its stats MUST be tagged with its own `rules_stat_prefix` to be observably distinct.

8. **TWO new framework primitives** — the FIRST §9 row since phase 14 to introduce non-zero framework deltas: (i) TLS-principal accessor on `DecoderFilterCallbacks` (ADR-0144); (ii) matcher-engine evaluator framework primitive (ADR-0142). See §3 for details. This is a deliberate trade-off — the Q3 Large 11+11 + Q2 BOTH-engines picks require these primitives. The framework-delta accretion across §9 family-rows:

   - Phase 07.1 cors: NEW framework (the entire HTTP-filter framework). N/A baseline.
   - Phase 09 fault: introduced `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume primitives.
   - Phase 10 header_mutation: ZERO framework deltas.
   - Phase 11 local_ratelimit: ZERO framework deltas.
   - Phase 12 csrf: ZERO framework deltas.
   - Phase 13 buffer: TWO framework deltas (decode-side per ADR-0128).
   - Phase 14 compressor: ONE framework delta (`OverwriteBody` per ADR-0131).
   - Phase 15 bandwidth_limit: ZERO framework deltas (load-bearing reusability demonstration of phase-13/14 primitives).
   - **Phase 16 rbac: TWO framework deltas (TLS-principal accessor + matcher-engine evaluator).** The first non-zero §9 row since phase 14 — driven by the Q3 Large 11+11 + Q2 BOTH-engines picks rather than by inherent filter shape (a Q3 Tiny 5+4 pick would have been ZERO-delta; the user explicitly traded framework surface for proto fidelity).

**Plus three artifact-level deliverables:**

9. **Differential fixture `0018-http-rbac`** under `test/fixtures/0018-http-rbac/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising 8 scenarios per §6 below. The fixture reuses `test/helpers/echobackend/` from phase 14/15 for the allow-disposition scenarios (the request passes through to backend; backend echoes; equivalence asserted on backend-arrival-time + response status + body). The fixture asserts response status, **body byte-equivalent on allow paths AND on deny-403 paths** (rbac does NOT transform bytes), counter deltas via `/stats/prometheus` scrape equivalence on the 5 base counters + per-policy counters when `track_per_rule_stats: true`, per-route-tier-disposition (both `rbac: nil` disabled-via-absence and `rbac: <RBAC>` wholesale-override shapes exercised), shadow-mode counter equivalence (shadow allowed/denied increment without affecting the actual disposition), AND TLS-principal-match scenario (requires TLS client cert; uses the existing phase-03 mTLS infrastructure).

10. **`BEHAVIOR_CONTRACT.md` 5-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing 8 filter subsections): a NEW `### envoy.filters.http.rbac` subsection covering the 7-consumed field map, the dual-engine semantics (rules + matcher), the Large-11+11 Permission/Principal subset, the ALLOW + DENY + LOG-partial action semantics, the shadow-mode semantics + counter emissions, the per-route 7th-canonical (absent-implies-disabled) semantics, the INDEPENDENT-stats hypothesis for per-route, the deny-path SendLocalReply wire shape. Plus the 60→65+-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0018 with per-scenario tolerance discipline (allow/deny ±0; counter-delta exact; shadow counter-delta exact; TLS-principal scenario byte-equivalent). Plus a NEW `### Phase 16 forward-pointer notes` subsection under `## Forward-pointer notes` covering the ~10-item deferral list (per §8 below). Plus an extension to `## HTTPFilterCallbacks` section documenting the new TLS-principal accessor (per §3.1 + ADR-0144).

11. **Anticipated 7 ADRs (ADR-0140 through ADR-0146) plus ADR-0125 §(xii) amendment paragraph** per §7 below. ADR-0139 is the highest-numbered ADR landed in phase 15; ADR-0140 is the next-free.

### 1.2 What 16 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12/13/14/15 precedent; deferrals are 10 items grouped by family-coupling, larger than phase 15's 8-item list given rbac's much wider proto surface). Summary: CEL `condition` + `checked_condition` policy fields (couples to a future CEL framework phase); `audit_logging_options` (not-implemented upstream); deprecated `metadata` Permission + `metadata` Principal + `source_ip` Principal variants; extension-coupling `matcher` + `uri_template` Permission variants (couples to plugin framework); `sourced_metadata` + `filter_state` runtime always-no-match semantic (parse-supported but the underlying metadata/filter-state subsystems are not yet shipped); `track_per_rule_stats: true` envoy-go-only large-N parse-rejection (optional foot-gun guard; §8.5); LOG-action dynamic-metadata emission (couples to dynamic-metadata family); shadow-rules access-log integration (currently emit shadow counters only; access-log shadow-decision row not emitted); some matcher-engine terminal-action TypedExtensionConfig types beyond the canonical RBAC `Action` types (parse-rejected with envoy-go-only error per §11 pin §16.P3); `Principal_Authenticated` outside the URI SAN / DNS SAN / Subject DN canonical fields (full X.509 cert introspection beyond the canonical principal-name fields). None are blockers for closing row 16 phase-done.

### 1.3 Phase-done as a §9 family-row landing

Phase 16's phase-done commit closes ROADMAP row `16` (single-row hypothesis at brainstorm time — but with ADR-0045 split highly-anticipated per §1.4 below). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships. Phase 16 is the NINTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, 12-csrf, 13-buffer, 14-compressor, 15-bandwidth_limit). The next §9 family-row will be numbered `17` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 62 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness — HIGHLY anticipated for phase 16

The brainstorm's POSITION is that phase 16 **likely exceeds ADR-0045's 1500-LoC / 25-task split-trigger** and that the SPEC author should evaluate the split early in SPEC drafting. LoC estimate: ~2200-3000+ LoC (anticipated):

- `rbac.go` (filter + factory + compiledConfig + decode methods + filterStats + per-route helper): ~500-700 LoC.
- `evaluator.go` (11 Permission variants + 11 Principal variants + AND/OR/NOT combinators + dual-engine dispatch): ~700-900 LoC.
- `rbac_test.go` (unit tests for evaluator subsurface): ~700-1000 LoC.
- `fuzz_test.go`: ~80 LoC.
- `doc.go`: ~50-100 LoC.
- Framework-delta plumbing (TLS-principal accessor at `internal/filter/http/{callbacks.go, chain.go}` + matcher-engine evaluator location TBD): ~200-400 LoC.
- Fixture (driver + yaml + README): ~250-400 LoC.

Total: ~2480-3580 LoC. This is **well above** the ADR-0045 1500-LoC trigger; the split-by-surface release valve is RECOMMENDED at SPEC time. Two anticipated splits:

- **16.1 = rules-engine + Large 11+11 + per-route 7th canonical + 4-counter aggregate stats + TLS-principal framework delta**: the foundational filter — the rules-engine evaluator path, the 11+11 Permission/Principal subset (including the Principal_Authenticated TLS-principal accessor framework delta), the ALLOW + DENY action enum (LOG deferred to 16.2), the per-route 7th canonical pattern + ADR-0125 §(xii) amendment, the 2-counter base stats (`allowed`, `denied`). Differential fixture covers ALLOW + DENY scenarios with header/path/IP/SNI/TLS-principal matchers. Excludes: matcher engine, shadow mode, LOG action, track_per_rule_stats, per-rule counters. LoC estimate: ~1200-1600 LoC.
- **16.2 = matcher-engine + shadow + LOG-partial + track_per_rule_stats + per-rule counters + matcher-engine framework primitive**: the secondary-engine landing — the matcher-engine evaluator path (with the framework primitive landing), the shadow-rules/shadow-matcher parallel evaluation path, the LOG-partial action semantic, the `track_per_rule_stats: true` per-policy-name counter emission. Fixture extends with shadow + LOG + track scenarios. Adds: ADR-0142 (matcher-engine framework primitive), ADR-0146 (shadow + LOG + track discipline). LoC estimate: ~1000-1400 LoC.

This split mirrors phase 05 (downstream-h2 → upstream-h2) + phase 06 (stats-prometheus → access-log) + phase 07 (http-filter-framework → listener-chain-completion) + phase 08 (admin-endpoints → graceful-drain) precedents for surface-split-with-framework-primitive-on-first-half. The brainstorm does NOT pre-commit to the split per Q approval pick — that decision is the SPEC author's call (mirrors phase-13 single-row position that ended up single-row in practice). If the SPEC author concludes single-row is feasible (e.g., the framework-delta scope is narrower than anticipated, or the Permission/Principal evaluator surface compresses), single-row is acceptable too. The split-by-direction-split-by-engine option above is the structurally clean cut if the surface comes in heavy.

### 1.5 Seed-stub alignment

Like phases 09, 10, 11, 12, 13, 14, and 15, phase 16 has NO sibling SPEC stub — phase 16 enters fresh after the phase 15 close. The §9 family-children list at ROADMAP line 64 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`jwt_authn`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 16 has NO such branch. The brainstorm dialogue (Q1-Q7 over the user-Claude exchange) was sufficient to settle filter pick + engine choice + Permission/Principal subset + action enum + shadow+track posture + per-route classification + CEL stance without preliminary scoping notes. This matches the phase 09 / 10 / 12 / 13 / 14 / 15 cold-start precedent.

### 1.7 Phase 16's relationship to prior framework deltas + framework-delta accretion shape

Phase 16's TWO new framework deltas (TLS-principal accessor on `DecoderFilterCallbacks` + matcher-engine evaluator) are the FIRST framework introductions since phase 14's `OverwriteBody` per ADR-0131. The accretion shape across §9 rows:

- Phase 13 introduced two decode-side framework primitives (ADR-0128).
- Phase 14 introduced one encode-side framework primitive (ADR-0131).
- Phase 15 introduced ZERO and demonstrated ADR-0128 + ADR-0131 reusability.
- **Phase 16 introduces TWO** — one is genuinely cross-cutting (TLS-principal accessor; reusable by any filter wanting downstream cert info, e.g., future `jwt_authn`, `ext_authz`); the other is a new evaluator subsystem (matcher-engine; reusable by future filters wanting xds.type.matcher.v3.Matcher generic match-tree semantics, e.g., future `ext_authz` matcher-based gating). Both primitives are landed-in-phase-16 but CROSS-PHASE-REUSABLE. This is the §9 family-row's first dual-introduction phase.

The framework-delta budget across phases 13-16 is now: ADR-0128 (decode-side; phase 13) + ADR-0131 (encode-side OverwriteBody; phase 14) + ADR-0144 (TLS-principal accessor; phase 16) + ADR-0142 (matcher-engine evaluator; phase 16) = 4 framework deltas across 4 phases. The HTTP filter framework primitive surface has accreted ~5 net new APIs over the phase-13-to-16 arc.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 7 decisions below are the phase-16-specific design choices reached during the Q-dialogue. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 Family-child selection: `rbac` *(Decision #0 → ADR-0140 rationale)*

Per Q1 = "rbac chosen from §9 remaining unbrainstormed list", the selection criteria per ADR-0106(e):

- **Coherence ★★★★★ with §9 family rotation.** Phase 15 bandwidth_limit (body throttle) was the seventh-last §9 unbrainstormed candidate ranked by complexity; phase 16 rbac (decode-side policy gate) is the natural next-medium-complexity candidate. RBAC sits in the SAME complexity band as csrf @ 12 (same SendLocalReply discipline; same decode-side-only scope), but with a MUCH larger config surface (7 fields vs csrf's 3 active fields; Permission/Principal evaluator subsystem).
- **Framework-delta budget consideration.** Phase 16 was the OPTION among the 10 remaining candidates with the cleanest framework-delta story under a "small MVP" envelope (Q3 Tiny 5+4 would have been ZERO-delta). The user's Q3 Large 11+11 + Q2 BOTH-engines picks expanded the framework-delta scope from ZERO to TWO — a deliberate trade-off to maximize proto fidelity. The remaining candidates (jwt_authn, ext_authz, ext_proc, oauth2, lua, wasm, global_ratelimit) require SIGNIFICANTLY more framework infrastructure (outbound HTTP for JWKS / gRPC client for ext_authz / Lua/WASM VM for scripting); rbac is the FAR-SMALLER framework-delta path even under proto-faithful Q3 Large + Q2 BOTH picks.
- **Upstream-dependency readiness.** Pure-Go implementation. No new SDK dependencies required (CEL silent-ignored per Q7; would have added `cel-go`). Permission/Principal evaluators are config-driven; AND/OR/NOT combinators are standard tree-walks.
- **Scope-fit.** Estimated ~2200-3000+ LoC; ABOVE ADR-0045's 1500-LoC split-trigger. Split-readiness recommended (§1.4); brainstorm position is single-row at brainstorm time but ADR-0045 release-valve flagged.
- **Family-child rotation.** The remaining 10 §9 family-children (`jwt_authn / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control / global_ratelimit`) span widely-varying surface complexities. `rbac` sits in the medium-large complexity band — large enough to require TWO new framework primitives but small enough to fit a phase-or-split-pair release. The MUCH-larger remaining candidates (auth filters with side-channel HTTP/gRPC; scripting filters with VM dependencies) are still ahead.

ADR-0140 (the anticipated layout ADR for phase 16) documents the selection rationale + the FIRST §9 row to introduce TWO framework primitives in one phase.

### 2.2 Engine MVP: BOTH engines proto-faithful *(Decision #2 → ADR-0141)*

Per Q2 = "BOTH engines proto-faithful (rules + matcher)", the engine scope:

- `rules` engine: the canonical policy-map evaluation. Walk `policies` map lexicographically; for each Policy, evaluate `permissions[]` OR `principals[]` AND optional `condition` (CEL silent-ignored); aggregate to ANY-MATCH; apply `action`. Mirrors documented Envoy semantics verbatim.
- `matcher` engine: the generic match-tree evaluation. Walk the `xds.type.matcher.v3.Matcher` tree; first matching predicate's `on_match` action drives the disposition. If walk completes without match: DENY (per proto semantic).
- Oneof discipline: `rules_specifier` oneof of `rules` + `matcher`; only one consumed at a time; when both unset, no RBAC enforcement (returns Continue without evaluation per proto comment "If absent, no RBAC enforcement occurs"). Symmetric for `shadow_rules_specifier`.
- When both `rules` and `matcher` are set in the SAME config (proto admits this via the oneof being optional): `rules` WINS, `matcher` IS IGNORED per the proto comment ("When both `rules` and `matcher` are configured, `rules` will be ignored"). envoy-go mirrors this priority semantic verbatim.

The matcher-engine path introduces NEW framework infrastructure (ADR-0142): the `xds.type.matcher.v3.Matcher` evaluator. This is a generic match-tree evaluator REUSABLE by future filters (ext_authz, jwt_authn, oauth2 all use the same `xds.type.matcher.v3.Matcher` primitive for some of their config surface). Phase 16 lands this primitive scoped to RBAC's needs initially; future filters extending the primitive's API is permissible per the cross-phase-reuse discipline.

**Location decision** (open for §11 SPEC author): the matcher-engine evaluator could live at (a) `internal/filter/http/rbac/matcher.go` (scoped to rbac; future filters depend on rbac package — bad coupling); (b) `internal/matcher/` new top-level package (cleanest cross-phase reuse; commits to a new framework subsystem boundary at phase 16); (c) `internal/filter/http/matcher/` scoped to HTTP filters (compromise). Brainstorm position: option (b) `internal/matcher/` for cleanest reuse — but SPEC author can override based on framework-survey findings.

### 2.3 Permission + Principal MVP subset: Large 11+11 *(Decision #3 → ADR-0143)*

Per Q3 = "Large 11+11 Permission/Principal variant subset", the subset is:

**Permission MVP (11 of 14):** `any`, `header`, `url_path`, `destination_ip`, `destination_port`, `destination_port_range`, `requested_server_name`, `and_rules`, `or_rules`, `not_rule`, `sourced_metadata`.
- `and_rules` + `or_rules` + `not_rule` are recursive combinators (a Permission_Set inside a Permission; recursion depth bounded by parse-time depth check — §11 pin §16.P11 confirms Envoy's exact depth bound).
- `any` is a degenerate true-match (always-true).
- `header` + `url_path` are leaf matchers against request data (HeaderMatcher + PathMatcher; both well-defined by the envoy.type.matcher.v3 subsystem already shipped via Permission consumption pathway).
- `destination_ip` + `destination_port` + `destination_port_range` are listener-level matchers (envoy-go-side accessor must surface the downstream listener's bound port + bound IP; phase-07.2's listener-chain-completion shipped this via FilterChainMatch evaluation — phase 16's RBAC reuses the same accessor pattern).
- `requested_server_name` is the SNI value from the downstream TLS context (similar TLS-context accessor as `Principal_Authenticated` but a different field — SNI vs principal-name).
- `sourced_metadata` parse-supported with always-no-metadata-match runtime divergence-window (the underlying SourcedMetadata's MetadataMatcher requires a dynamic-metadata subsystem not yet shipped; envoy-go MVP evaluates SourcedMetadata to always-FALSE; documented divergence-window).

**DEFERRED Permission variants (3 of 14):**
- `metadata` (DEPRECATED upstream; superseded by `sourced_metadata`).
- `matcher` (TypedExtensionConfig; couples to plugin framework — Envoy's extension registry mechanism for matcher-action plugins). PARSE-REJECT with envoy-go-only error `rbac: permission.matcher extension types unsupported in this build`.
- `uri_template` (TypedExtensionConfig; same extension-framework coupling). PARSE-REJECT.

**Principal MVP (11 of 13):** `any`, `authenticated` (Principal_Authenticated), `direct_remote_ip`, `remote_ip`, `header`, `url_path`, `and_ids`, `or_ids`, `not_id`, `sourced_metadata`, `filter_state`.
- `authenticated` requires a TLS-principal accessor on `DecoderFilterCallbacks` (NEW framework primitive; ADR-0144) — exposes downstream client cert URI SAN / DNS SAN / Subject DN per `Principal_Authenticated.principal_name` StringMatcher semantics.
- `direct_remote_ip` matches against the peer connection's source IP; `remote_ip` matches against the XFF-resolved IP (depending on listener's `use_remote_address` + `xff_num_trusted_hops` settings — phase 16 brainstorm-time hypothesis is that the existing listener-chain config exposes the resolved-remote-IP value via an accessor pattern; if not, ADR-0144 amendment scope expands to cover both TLS-principal and XFF-resolved-IP accessors).
- `header` + `url_path` mirror the Permission variants.
- `and_ids` + `or_ids` + `not_id` are recursive combinators (Principal_Set inside Principal).
- `sourced_metadata` + `filter_state` parse-supported with always-no-match runtime divergence-windows (sourced_metadata couples to dynamic-metadata; filter_state couples to a filter-state subsystem not yet shipped — both documented).

**DEFERRED Principal variants (2 of 13):**
- `metadata` (DEPRECATED upstream).
- `source_ip` (DEPRECATED upstream; superseded by `direct_remote_ip` + `remote_ip`). PARSE-REJECT with envoy-go-only error `rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip` (mirrors deprecation-rejection pattern; SPEC author may revise to silent-ignore if Envoy's parser accepts source_ip without warning under v1.37.2).

The Large 11+11 subset commits phase 16 to TLS-principal accessor framework delta (ADR-0144) but defers the extension-framework coupling. Future phase-16-or-17 amendments can add `matcher` + `uri_template` + `sourced_metadata` full-functionality once the dependent subsystems land.

### 2.4 Action enum: ALLOW + DENY + LOG-partial *(Decision #4 → ADR-0141 consequence + ADR-0146)*

Per Q4 = "ALLOW + DENY + LOG-partial", the action scope:

- `ALLOW` → allow if any-policy-matched; deny if no-policy-matched. The canonical safe-list mode.
- `DENY` → allow if no-policy-matched; deny if any-policy-matched. The canonical block-list mode.
- `LOG-partial` → ALWAYS allow regardless of match; envoy-go-MVP DIVERGES from Envoy in that the `envoy.common.access_log_hint` dynamic metadata key is NOT emitted (always-no-metadata divergence-window). The match evaluator still RUNS to determine whether any policy would have matched (for stats emission purposes — the `logged` counter increments unconditionally; some downstream-observability variants might still want the match-evaluated counter even without the metadata signal). §11 pin §16.P4 measures Envoy's exact behavior under `action: LOG` — specifically whether `allowed`/`denied` counters increment in addition to or instead of `logged`.

This is the third §9 row to introduce a per-action-mode divergence-window (phase-15 bandwidth_limit's `enable_mode` enum was the second; phase-14 compressor's compression-skip decision matrix was the first). LOG-partial divergence couples to a future dynamic-metadata family phase that lands the `EncoderFilterCallbacks.SetDynamicMetadata(key string, value structpb.Value)` primitive (or equivalent). Re-activation completes the LOG-action wire-shape equivalence.

### 2.5 Shadow + track_per_rule_stats: BOTH proto-faithful *(Decision #5 → ADR-0146)*

Per Q5 = "Both proto-faithful (shadow + track_per_rule_stats)", the observability scope:

- **Shadow evaluation:** run the parallel `shadow_rules`/`shadow_matcher` engine alongside primary. Emit `shadow_allowed`/`shadow_denied` counters under `shadow_rules_stat_prefix`. Shadow NEVER affects disposition (mirrors Envoy semantic).
- **`track_per_rule_stats: true`:** emit per-policy-name counters `policy.<name>.allowed` + `policy.<name>.denied` (and shadow variants) for each policy that matched the request. Per-policy counter cost: 2N counters per active `rules_stat_prefix` (worst case where every config policy matches at least once during the proxy's lifetime); foot-gun for misconfigured large-N policies.

**Foot-gun discipline.** The stat-surface explosion (2N + 2M counters under primary + shadow + track) is documented at BEHAVIOR_CONTRACT phase-16 forward-pointer notes. The `track_per_rule_stats` field is parse-supported but recommended-against in operator-facing docs for large-N configs; envoy-go does NOT impose a parse-time N-cap (mirrors Envoy's permissive policy-map size — no upper bound documented). §8.5 deferral notes that an envoy-go-only N-cap could be introduced via ADR-0126-style filter-internal validation if operator-observability shows a real-world foot-gun.

### 2.6 Per-route discipline: NEW 7th canonical (absent-implies-disabled) *(Decision #6 → ADR-0125 amendment §(xii))*

Per Q6 = "NEW 7th canonical (absent-implies-disabled)", the per-route TPFC shape:

```proto
message RBACPerRoute {
  reserved 1;  // formerly a removed disabled boolean per proto evolution
  RBAC rbac = 2;  // If absent, RBAC policy will be disabled for this route.
}
```

(§11 pin §16.P1 confirms exact proto shape including the reserved field's prior semantic.)

Two cases:
- **(a) `RBACPerRoute{rbac: nil}` (or absent message entirely)** — the filter is wholly inactive on this route. No policy enforcement. No counter increments (the listener-level filter's counter scope is NOT affected by per-route-absent streams). Request passes through to the next filter without RBAC evaluation.
- **(b) `RBACPerRoute{rbac: <RBAC>}`** — WHOLESALE override of the listener-level RBAC message (NOT a merge; the override's `rules`, `matcher`, `shadow_rules`, `shadow_matcher`, all 7 top-level fields REPLACE the listener-level values entirely). Mirrors phase-13/14/15 WHOLESALE-not-merge per ADR-0125 + ADR-0073.

`parsePerRoute` flow:
1. If `rbac` field is nil (or `RBACPerRoute` message itself is nil) → produce `*compiledPerRoute{disabled: true, overrideConfig: nil}`.
2. If `rbac: { … }` → recursively run `buildCompiledConfig(rbac)` → produce `*compiledPerRoute{disabled: false, overrideConfig: <built>}`.
3. The `disabled` boolean inside `*compiledPerRoute` is an envoy-go-internal artifact derived from `RBACPerRoute.rbac == nil`; it is NOT a proto-level field. Documented at ADR-0140.

Resolution flow at request time (mirrors phase-13/14/15):
1. `PerRouteConfig.Resolve(ctx)` → most-specific `*compiledPerRoute` for this route.
2. If `compiledPerRoute.disabled=true` → set `f.passthrough=true`; `DecodeHeaders` short-circuits to `HeaderContinue`.
3. If `compiledPerRoute.disabled=false` AND `overrideConfig != nil` → use `overrideConfig` for the policy evaluation (including its own `rules_stat_prefix`, which drives INDEPENDENT stat namespace per §2.7 + §5); listener-level config NOT consulted on this route.

**ADR-0125 in-place amendment paragraph §(xii)** (NOT a new ADR; in-place per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) precedent): noting phase 16 rbac as the FIRST row to use the absent-implies-disabled-OR-override **7th canonical per-route pattern** + the WHOLESALE-not-merge semantic for the entire RBAC message. ADR-0125's canonical-pattern roster grows from 6 to 7. Future filters whose per-route proto follows the same "single message field; absent-means-disabled; presence-means-override" shape compose against this canonical. Authored at phase 16 SPEC drafting time per the ADR-0125 in-place-update precedent.

### 2.7 CEL conditions: silent-ignored *(Decision #7 → ADR-0141 consequence)*

Per Q7 = "Silent-ignore conditions", policies with `condition` or `checked_condition` set parse cleanly but the condition is silent-ignored at runtime — the evaluator skips the condition and treats it as always-true. This means:

- Policies that rely on CEL conditions for fine-grained control see envoy-go-vs-Envoy decision DIVERGENCE — envoy-go allows requests Envoy would deny (and vice versa) for the condition-driven slice of the policy graph.
- The divergence-window is documented at BEHAVIOR_CONTRACT phase-16 forward-pointer notes.
- Couples to a future CEL framework phase that lands `internal/cel/` evaluator + `github.com/google/cel-go` dependency. Re-activation enables full condition evaluation.

The silent-ignore choice avoids pulling in a ~3000-LoC dependency (cel-go) plus the framework-level CEL evaluation context this phase. The decision is reversible at a future phase.

### 2.8 Stat surface hypothesis *(Decision implicit → ADR-0145)*

Per Q4 + Q5 picks, the stat surface is:

**Base counters (5; constant per active rules_stat_prefix + shadow_rules_stat_prefix combination):**
1. `allowed` — counter; increments when primary engine ALLOWED the request.
2. `denied` — counter; increments when primary engine DENIED the request.
3. `logged` — counter; increments when `action: LOG` per Q4-partial (always-allow but tracked separately).
4. `shadow_allowed` — counter; increments when shadow engine WOULD HAVE allowed.
5. `shadow_denied` — counter; increments when shadow engine WOULD HAVE denied.

**Per-rule counters (variable; emitted when `track_per_rule_stats: true`):**
- `policy.<name>.allowed` and `policy.<name>.denied` per matched policy per active prefix.
- Shadow variants under `shadow_rules_stat_prefix`.

**Stat-name surface count summary:**
- Phase 15 (bandwidth_limit): 60-name table (14 new active + 2 deferred-histogram per ADR-0138).
- Phase 16 (rbac): **60 → 65+ names** (5 new active counters minimum; variable upward with `track_per_rule_stats: true` per operator config; the 2N per-policy-name counter surface is OPERATOR-CONFIG-DRIVEN, NOT a fixed +N in the table — documented at BEHAVIOR_CONTRACT per a NEW emission-discipline subsection).

**Stat namespace + Prometheus tag-extractor:** §11 empirical pin §16.P7 confirms exact stat path + tag pattern. Hypothesis: `http.<HCM stat_prefix>.<rules_stat_prefix>.<counter>` for primary, `http.<HCM stat_prefix>.<shadow_rules_stat_prefix>.<counter>` for shadow (mirrors phase-11/15 SN9-or-SN2-reuse pattern). The SN-rule hypothesis is **SN2 reuse**; SN10 introduced only if §11 pin §16.P7 demands.

**Per-route stats discipline: INDEPENDENT hypothesis** (§11 pin §16.P9 confirms). Rationale: per-route owns its own policy-set state + its own `rules_stat_prefix` as own emission scope (mirrors phase-11 ADR-0117 + phase-15 ADR-0139; DIVERGES from phase-12/13/14 SHARED per ADR-0124/0125/0132). Per-route override emits its own `allowed`/`denied`/`logged`/`shadow_*` counter namespace driven by its own `rules_stat_prefix`.

---

## 3. Framework survey — TWO framework deltas anticipated

Phase 16 is the FIRST §9 row since phase 14 to introduce framework deltas — and the FIRST single phase to introduce TWO simultaneously. Both are CROSS-PHASE-REUSABLE per the §1.7 accretion-shape rationale.

### 3.1 TLS-principal accessor on `DecoderFilterCallbacks` (ADR-0144)

`Principal_Authenticated` requires access to the downstream client TLS certificate's principal name(s). Per Envoy v1.37.2 semantics, `Principal_Authenticated.principal_name` (StringMatcher) matches against (in order of priority): (a) URI SAN, (b) DNS SAN, (c) Subject DN (Common Name).

**Current envoy-go state:** Phase 03 ships downstream TLS termination. The TLS context (cert chain, principal info) is held by the listener/connection layer. It is NOT currently surfaced through `DecoderFilterCallbacks`. Phase 16 must introduce an accessor.

**Proposed API (subject to SPEC author refinement):**

```go
// DecoderFilterCallbacks extension (phase-16 framework delta):
type DecoderFilterCallbacks interface {
    // ... existing methods ...

    // DownstreamPrincipal returns the TLS principal name(s) of the downstream client connection,
    // in priority order (URI SAN, DNS SAN, Subject DN). Returns nil if the downstream is
    // plaintext or if no client cert was presented. Multi-valued slice mirrors Envoy's
    // PRINCIPAL_NAME ordering.
    DownstreamPrincipal() []string
}
```

Alternative API shape (StringMatcher-compatible accessor):

```go
// DownstreamPrincipalMatches evaluates the StringMatcher against all principal candidates
// (URI SAN, DNS SAN, Subject DN) in priority order and returns true if any candidate matches.
DownstreamPrincipalMatches(matcher *typev3.StringMatcher) bool
```

SPEC author picks. Brainstorm position: the matcher-compatible variant is more efficient (no slice allocation; single-shot match) but couples the framework primitive to the StringMatcher type. The principal-list variant is more flexible (filters can do their own matching) but allocates. Mirrors the `internal/listener/match.go` precedent where phase-07.2 chose the multi-valued accessor pattern.

**Plumbing:** the TLS context is held at the listener/connection layer; the HCM filter-chain dispatch context is held at the request layer. The accessor must thread the TLS info through the HCM-to-filter dispatch. Mirrors how phase-07.2's FilterChainMatch surfaces SNI/cert info to listener-side filters — but for the HCM HTTP filters, a new accessor on `DecoderFilterCallbacks` is the cleanest cut.

**Scope:** the accessor is HTTP-filter-only in phase 16 (not surfaced to network-filters; that's a future cross-cut). Future filters needing TLS info (jwt_authn, ext_authz, oauth2) consume the same accessor.

### 3.2 Matcher-engine evaluator framework primitive (ADR-0142)

The `xds.type.matcher.v3.Matcher` generic match-tree primitive. Reusable across filters; phase 16 lands the evaluator scoped initially to RBAC's needs.

**Location decision** (§2.2 noted; SPEC author confirms):
- (a) `internal/filter/http/rbac/matcher.go` — scoped to rbac package; future filters depend on rbac → bad coupling.
- (b) `internal/matcher/` new top-level package — cleanest cross-phase reuse; commits to a new framework subsystem boundary in phase 16.
- (c) `internal/filter/http/matcher/` — scoped to HTTP filters; compromise.

Brainstorm preference: option (b). It commits to the eventual cross-phase reuse upfront and avoids a future refactor when ext_authz/jwt_authn need the same primitive. SPEC author refines.

**API shape (subject to SPEC author refinement):**

```go
// internal/matcher/matcher.go (NEW package):

// Matcher wraps a parsed xds.type.matcher.v3.Matcher tree.
type Matcher struct { /* opaque */ }

// New parses the proto match tree at config-load time + returns a Matcher.
// Errors if the tree contains unsupported terminal types or predicates beyond
// what the evaluator implements. The supported set is per-call: the caller
// (a filter) passes in a set of supported terminal-action TypeURLs that the
// evaluator validates against.
func New(tree *xdsmatchv3.Matcher, supportedActionTypes []string) (*Matcher, error)

// Evaluate walks the match tree against the request data + returns the matched
// action TypedExtensionConfig (or nil for no-match). The req parameter is a
// match-context abstraction (header accessors, IP accessors, principal accessors)
// that the request-side filter provides.
func (m *Matcher) Evaluate(req MatchContext) (*anypb.Any, error)
```

**Match-context abstraction:** the evaluator needs to look up request data (headers, IP, principal, etc.) — same data the Permission/Principal evaluators look at. A common `MatchContext` interface threads through both subsystems; the filter implements `MatchContext` and passes its `*filter` to both Permission/Principal walkers AND the matcher-engine walker.

**Terminal action types:** the matcher-engine's `on_match` returns a TypedExtensionConfig wrapping a typed action proto. For RBAC, the canonical terminal actions are RBAC `Action` enum (ALLOW/DENY/LOG). §11 pin §16.P3 confirms the exact TypeURL Envoy v1.37.2 uses for RBAC terminal actions. envoy-go MVP supports the canonical RBAC terminal actions; non-RBAC terminal types PARSE-REJECTED with envoy-go-only error.

### 3.3 What is reused (already-on-disk primitives the filter composes against)

- `cb.SendLocalReply(status, body, headers)` — used by phase-09 fault for abort + phase-11 local_ratelimit for 429 + phase-12 csrf for 403 + phase-13 buffer for 413; phase-16 rbac uses for 403-deny.
- 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) per phase 07.1.
- `internal/filter/http/extension/HTTPRegistry` per ADR-0072 + boot-registration discipline.
- `*stats.Registry` + counters per phase-06.1; per-route INDEPENDENT-stats helpers per phase-11 ADR-0117 + phase-15 ADR-0139 (`NewCounterIfAbsent`-style accessors).
- `internal/listener/match.go` listener-port accessor (for `Permission_DestinationPort` / `Permission_DestinationPortRange`).
- SNI accessor from phase-07.2 listener-chain-completion (for `Permission_RequestedServerName`).
- XFF resolver from phase-04 HTTP-1.1 + phase-05 HTTP/2 (for `Principal_RemoteIp`).
- Existing PathMatcher + HeaderMatcher + StringMatcher + CidrRange evaluators (shared with cors / csrf / header_mutation per their precedent).

### 3.4 No filter-chain ordering surgery

Phase 16 rbac's filter-chain position is up to the operator. Recommended ordering: rbac EARLY in the chain (immediately after listener filters) so denied requests don't incur downstream filter cost (header_mutation, buffer, compressor, bandwidth_limit can all be skipped if RBAC denies). Phase 16 fixture pins rbac as the first filter in the HCM chain.

---

## 4. Rejection-path wire shape (deny disposition)

Phase 16's deny-path mirrors phase-09 fault.abort + phase-11 local_ratelimit + phase-12 csrf + phase-13 buffer (413) wire-shape discipline:

- **Status code:** 403 (Forbidden). §11 pin §16.P5 confirms (Envoy v1.37.2's RBAC deny status is 403 per documentation; envoy-go mirrors).
- **Body:** byte-exact `RBAC: access denied` hypothesis (19 bytes ASCII, no trailing newline). §11 pin §16.P5 confirms exact body bytes. Alternative hypothesis (per Envoy source proximity): some Envoy versions emit `403 — access denied`, others emit `RBAC: access denied`, others emit empty body. §11 pin resolves.
- **4-header set (lowercase wire-form):** `content-length: <len>`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`. Mirrors phase-09/11/12/13 4-header discipline; `server: envoy` lowercase value (NOT `envoy-go`) per phase-11 ADR-0118 + phase-12 ADR-0123 precedent.
- **Connection disposition:** keep-alive (NO `connection: close`). Unlike phase-13 buffer 413 which closes the connection due to potential partial-body-consumption ambiguity, rbac's 403 is a pre-body-decision (the body has not started yet at deny time), so keep-alive is safe. §11 pin §16.P5 confirms.

`cb.SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` mechanism (mirrors fault.abort, local_ratelimit, csrf, buffer-413). NO new framework primitive; reuses existing.

---

## 5. Per-route discipline — 7th canonical + INDEPENDENT-stats

Per §2.6 + §2.7, per-route is the **NEW 7th canonical (absent-implies-disabled)** pattern with INDEPENDENT-per-route-stats. Rationale:

- **Stateful per-route policy set.** Each per-route override OWNS its own policy-evaluation state — a fresh effective `rules`/`matcher` (different policy-map / different matcher-tree), a fresh `shadow_rules`/`shadow_matcher`, a fresh `rules_stat_prefix`. The listener-level policy set and the per-route policy set are LITERALLY DIFFERENT policy-evaluator objects (different lexicographic-walk-order, different terminal actions). Mirrors phase-11 ADR-0117 + phase-15 ADR-0139 (each per-route override = fresh policy set = own stat namespace).
- **Own rules_stat_prefix.** The override's `rules_stat_prefix` field is a first-class config knob — the operator can choose to set it to anything (including the same string as the listener-level for SHARED-emission). Brainstorm hypothesis: the override `rules_stat_prefix` is honored as a NEW emission-scope tag.
- **Counter-arithmetic observability.** A 100-stream load against a route with a per-route override should produce 100 increments on the override's `allowed` counter (assuming all allow), NOT 100 increments on the listener-level's counter. This is what an observability dashboard operator expects.

**Divergence from phase-12/13/14:** Those filters' per-route stats SHARED with listener-level because their per-route overrides are STATELESS in the policy-set sense (their per-route override changes effective config knobs; no fresh per-route policy state). Phase-16 rbac's stateful-override matches phase-11 local_ratelimit + phase-15 bandwidth_limit per the stateful-override-implies-INDEPENDENT-stats discipline.

**§11 empirical pin §16.P9 confirms or refutes** the INDEPENDENT hypothesis. If Envoy v1.37.2 emits SHARED stats (per-route routed into listener-level counter namespace), envoy-go SPEC author either (a) matches Envoy (SHARED) and amends ADR-0145 accordingly + amends BEHAVIOR_CONTRACT to note the per-route `rules_stat_prefix` is honored at parse but ignored at emission; or (b) elects to DIVERGE from Envoy on this axis (INDEPENDENT — the more useful behavior) and documents the divergence-window per phase-12/14 style. Brainstorm position: INDEPENDENT is the operationally-correct shape regardless of Envoy's choice; the SPEC author's pin is to ratify or document the divergence.

**ADR-0145** (anticipated): codifies the INDEPENDENT-vs-SHARED resolution per the empirical pin.

---

## 6. Differential fixture (`0018-http-rbac`)

### 6.1 Topology

Two listeners + two clusters (matches phase 11/12/13/14/15 fixture topology):

- **Listener `l_test_a`** (TCP plaintext on port `<envoy-go-test-port>` for envoy-go side; matching port on Envoy side per the `0017` template — and a parallel `l_test_a_tls` TCP+TLS listener on a separate port for the TLS-principal scenario per §6.2 scenario 6). Hosts an HCM with one filter-chain; the chain has filters: `rbac → router`. Listener-level config is an `RBAC` message with `rules_stat_prefix: "default"`, `action: ALLOW`, `policies: {<canonical-policies>}` per §6.2 scenarios. Routes:
  - Route `/`: `direct_response` 200 OK with body `<canonical 32-byte ASCII payload>`. Default route; allow-by-default + allow-by-policy-match scenarios.
  - Route `/admin`: `direct_response` 200. Allow-only-when-authenticated-as-admin scenario.
  - Route `/protected`: routes to backend cluster `c_backend_b`. Allow-by-IP-match scenario.
  - Route `/per-route-disabled`: per-route TPFC `RBACPerRoute{}` (empty; rbac field nil → disabled). Exercises 7th canonical absent-implies-disabled in scenario 7.
  - Route `/per-route-override`: per-route TPFC `RBACPerRoute{rbac: <RBAC with own rules_stat_prefix: "override", action: DENY>}`. Exercises wholesale-override in scenario 8.

- **Listener `l_test_b`** + cluster `c_backend_b`: echo-backend cluster pair (real upstream-echo backend from phase 14/15's `test/helpers/echobackend/`). Used as the routing target for `/protected`.

- **Optional `l_test_a_tls`** for scenario 6 (TLS-principal scenario): mTLS-required listener with a phase-03 mTLS config + a fixture-bundled client cert with URI SAN `spiffe://example.com/admin`.

### 6.2 8 scenarios

Per 5 routes + 3 additional traversal modes:

1. **Allow-by-header-match (basic ALLOW):** request `GET /` with `X-User: admin`. Policy `admin_users` permissions: `header.X-User: admin`; principals: `any`. Expected: status 200, body byte-equivalent, `/stats/prometheus` shows `allowed` counter +1.

2. **Deny-by-no-match (basic DENY-via-ALLOW-no-match):** request `GET /` with `X-User: guest`. No policy matches. Expected: status 403, body byte-exact `RBAC: access denied` (19 bytes), 4-header set lowercase wire-form, `denied` counter +1.

3. **Allow-by-URL-path:** request `GET /` (no special header). Policy `public_paths` permissions: `url_path.exact: /`; principals: `any`. Expected: status 200, `allowed` +1.

4. **Allow-by-DestinationPort:** request `GET /` arriving on listener port `<l_test_a's port>`. Policy `listener_ports` permissions: `destination_port: <l_test_a's port>`; principals: `any`. Expected: status 200, `allowed` +1.

5. **Allow-by-IP (Principal_DirectRemoteIp):** request `GET /protected` from a peer IP within `127.0.0.0/8`. Policy `local_clients` permissions: `url_path.prefix: /protected`; principals: `direct_remote_ip: 127.0.0.0/8`. Expected: status 200, body byte-equivalent (echoed back from backend), `allowed` +1.

6. **Allow-by-TLS-principal (Principal_Authenticated):** request `GET /admin` over mTLS to `l_test_a_tls` listener with the fixture client cert (URI SAN `spiffe://example.com/admin`). Policy `admin_users` permissions: `any`; principals: `authenticated.principal_name.exact: spiffe://example.com/admin`. Expected: status 200, `allowed` +1. Exercises the new TLS-principal accessor framework primitive (ADR-0144).

7. **Per-route disabled (7th canonical absent-implies-disabled):** request `GET /per-route-disabled` with `X-User: guest` (would deny at listener-level). Per-route TPFC `RBACPerRoute{}` empty. Expected: status 200 (passthrough), `allowed`/`denied` counters NOT incremented at listener namespace, `/per-route-disabled` route observably bypasses RBAC.

8. **Per-route override with own stat_prefix + shadow:** request `GET /per-route-override` with `X-User: guest`. Per-route TPFC `RBACPerRoute{rbac: <RBAC with rules_stat_prefix: "override", shadow_rules_stat_prefix: "override_shadow", action: DENY, policies: {<guests_policy: permissions: any, principals: header.X-User: guest>}>}`. Listener-level policy would have ALLOWED (different rule set). Per-route override DENIES guests. Expected: status 403, `override.denied` counter +1, `override_shadow.shadow_allowed`/`shadow_denied` counter +1 (shadow run alongside; shadow result mirrors primary in this simple config). The `default.allowed`/`default.denied` counters at listener namespace stay UNCHANGED on this route (INDEPENDENT-stats per §5).

### 6.3 Asserted equivalence

**Per-scenario assertions** (mirrors phase 11/12/13/14/15 scenario-by-scenario equivalence):

- **Status code:** byte-exact.
- **Headers:** lowercase wire-form byte-exact on ALL response headers (no header mutation; rbac is a pure decision filter).
- **Body:** byte-exact on allow (passthrough; backend bytes) AND on deny (`RBAC: access denied` 19 bytes per §4).
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the 5 base counters + the per-rule counters for scenarios with `track_per_rule_stats: true` (scenarios TBD: include 1-2 scenarios with the flag set).
- **Per-route fixture-config disposition:** scenarios 7 + 8 exercise BOTH per-route shapes (`rbac: nil` 7th-canonical-disabled + wholesale-override); scenario 8 ALSO exercises INDEPENDENT-vs-SHARED stat namespace per §11 pin §16.P9.
- **TLS-principal scenario:** scenario 6 exercises the new ADR-0144 framework primitive; the fixture's client cert is generated at test-time + bundled into the fixture directory; mTLS connection from the driver presents the cert.

### 6.4 Driver shape

`inputs/driver.go` mirrors the `0017` driver shape:
- 8 scenarios, each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Per-scenario assertion helper (status + body + counter-delta).
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.
- For scenario 6, a separate `runTLSScenario` helper using the mTLS-capable HTTP client (similar to phase-03's TLS test infrastructure).

---

## 7. Anticipated ADRs (ADR-0140 through ADR-0146)

7 anticipated ADRs (the largest §9-row ADR roster to date — phase 14 had 6 ADRs including the impl-time ADR-0134; phase 15 had 5). Phase 16 next-free is ADR-0140 (ADR-0139 was the highest landed in phase 15).

- **ADR-0140: `rbac` package shape + boot registration + 4-file split + filterStats struct + DECODER-only `HTTPFilter`.** Mirrors phase-15 ADR-0135 same-package-shape precedent + phase-14 ADR-0129 + phase-13 ADR-0125 + phase-12 ADR-0120 + phase-11 ADR-0114 + phase-10 ADR-0108 layout ADRs. Documents the package directory + extension-registry registration position (`rbac` after `local_ratelimit` alphabetical) + the DECODER-only nature (no encode-side path; rbac is a decode-side request gate). Codifies the FIRST §9 row to introduce TWO framework primitives in one phase. Documents the 4-file split (rbac.go + evaluator.go + tests + fuzz_test + doc.go).

- **ADR-0141: `compiledConfig` shape + 7-consumed field decomposition + dual-engine dispatch + envoy-go-side parse validation.** Documents the 7 consumed top-level fields (the full proto-faithful surface per Q2 + Q5 picks) + the dual-engine dispatch table (`rules`/`matcher` priority; `shadow_rules`/`shadow_matcher` priority; CEL `condition` silent-ignored per Q7; `audit_logging_options` silent-ignored per upstream-not-implemented; deprecated `metadata` Permission/Principal variants parse-rejected with envoy-go-only error per §2.3). Cross-references phase-15 ADR-0136's envoy-go-only-check precedent + phase-14 ADR-0130's typed-Any-dispatch precedent. Includes the ALLOW + DENY + LOG-partial action semantics + the LOG-partial divergence-window.

- **ADR-0142: Matcher-engine evaluator framework primitive.** Documents the new `internal/matcher/` package (or whatever location SPEC author picks) implementing `xds.type.matcher.v3.Matcher` generic match-tree evaluator. Documents the API shape (Matcher type + New constructor + Evaluate method + MatchContext interface) + the cross-phase reuse intent (future filters like ext_authz, jwt_authn, oauth2 will consume the same primitive). Includes the terminal-action TypedExtensionConfig dispatch + the canonical-RBAC-action-types supported-set + the unknown-TypeURL parse-rejection discipline.

- **ADR-0143: Permission + Principal Large 11+11 evaluators + AND/OR/NOT combinators + deprecation parse-rejection discipline.** Documents the 11 Permission variants + 11 Principal variants implemented in MVP + the 3 Permission + 2 Principal variants deferred. Cross-references phase-07.1's existing PathMatcher/HeaderMatcher/StringMatcher/CidrRange evaluators (shared infrastructure). Documents the recursive AND/OR/NOT combinator depth-bound (parse-time hard cap; §11 pin §16.P11 confirms Envoy's exact bound — hypothesis: 32 or 64 levels of nesting before envoy-go rejects with depth-exceeded error). Documents the `metadata`-deprecated + `source_ip`-deprecated parse-rejection discipline (envoy-go-only error mirrors phase-14 ADR-0130 unknown-codec-Any-reject precedent).

- **ADR-0144: TLS-principal accessor on `DecoderFilterCallbacks` framework primitive.** Documents the new accessor (API shape per §3.1) + the plumbing from TLS-context-at-listener through HCM-dispatch to filter-callback + the cross-phase reuse intent (jwt_authn, ext_authz, oauth2). Documents the URI-SAN-then-DNS-SAN-then-Subject-DN priority order per Envoy v1.37.2 semantic. Cross-references phase-03 downstream-TLS-termination ADR for the TLS-context source + phase-07.2 listener-chain-completion ADR for the listener-to-filter dispatch precedent.

- **ADR-0145: Stat surface (5 base + 2N per-rule) + namespace + SN-rule (SN2 reuse hypothesis; SN10 introduced only if §11 pin §16.P7 demands) + INDEPENDENT-vs-SHARED per-route resolution + variable-stat-surface-with-track_per_rule_stats foot-gun.** Documents the 5 base counters (allowed/denied/logged/shadow_allowed/shadow_denied) + the 2N per-rule counter emission discipline + their hypothesized scope (`http.<HCM>.<rules_stat_prefix>.<counter>` + analogous shadow-prefix) + SN2-reuse flattening rule (no new tag-extractor required under the brainstorm hypothesis; SN10 introduced if pin amends). Documents the per-route INDEPENDENT-stats discipline (mirrors phase-11 + phase-15 INDEPENDENT precedent; DIVERGES from phase-12/13/14 SHARED). Documents the variable-stat-surface foot-gun (operator-config-driven surface growth; documented at BEHAVIOR_CONTRACT). Cross-references phase-11 ADR-0118 SN9 + phase-15 ADR-0138 SN-rule-disposition precedent.

- **ADR-0146: Shadow-evaluation discipline + LOG-partial divergence-window + track_per_rule_stats per-rule-emission discipline + stat-surface variability foot-gun.** Documents the shadow-engine evaluation path (parallel to primary; never affects disposition; emits shadow_* counters) + the LOG-partial action semantic (always-allow + match-evaluated + no-metadata-emit divergence-window) + the track_per_rule_stats per-policy-name counter emission. Documents the BEHAVIOR_CONTRACT phase-16 forward-pointer notes subsection summarizing all 3 divergences for operator awareness.

**Plus an ADR-0125 in-place amendment paragraph §(xii)** (NOT a new ADR): noting phase 16 rbac as the FIRST row to use the absent-implies-disabled-OR-override **7th canonical per-route pattern** + the WHOLESALE-not-merge semantic for the entire RBAC message. ADR-0125 grows its canonical-pattern roster from 6 to 7. Authored at phase 16 SPEC drafting time per the ADR-0125 in-place-update precedent (mirrors phase-13 ADR-0127 v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi)).

SPEC-time may revise the 7-ADR count per ADR-0044 SPEC-time-anticipation discipline. Phase 14's 6-ADR roster (5 anticipated + 1 impl-time-added) is the precedent for impl-time ADR additions; phase 16 may see similar additions at Task-14-style follow-up. If the matcher-engine framework primitive lands in a separate file or with a separately-named API surface than anticipated, an additional ADR may anchor that surface; SPEC author judgment.

---

## 8. Deferral list

Per phase 11/12/13/14/15 inline-deferral discipline (no omnibus ADR), the deferrals are 10 family-coupled items (larger than phase 15's 8-item list given rbac's much wider proto surface):

### 8.1 CEL `condition` + `checked_condition` Policy fields

Per Q7 silent-ignore. Couples to a future CEL framework phase that lands `internal/cel/` evaluator + `github.com/google/cel-go` dependency. Re-activation enables fine-grained condition evaluation per policy. Operator divergence-window: policies relying on CEL conditions see envoy-go-vs-Envoy decision divergence (envoy-go treats condition as always-true; matches OR fails depending on the rest of the policy permissions/principals).

### 8.2 `audit_logging_options` (not-implemented upstream)

Marked `[#not-implemented-hide:]` in the proto comment; Envoy v1.37.2 does NOT emit audit logging regardless of config. envoy-go silent-ignores. Couples to a future audit-logging family phase.

### 8.3 Deprecated `metadata` Permission + Principal variants

PARSE-REJECT with envoy-go-only error `rbac: permission.metadata deprecated; use sourced_metadata` (and analogous for Principal). If Envoy v1.37.2 still accepts these (deprecated-but-functional), envoy-go's parse-rejection is a divergence-window. §11 pin §16.P12 confirms Envoy's parse-time disposition for deprecated fields. SPEC author may revise to silent-ignore if Envoy is lenient.

### 8.4 Deprecated `Principal_SourceIp`

Same disposition as 8.3. PARSE-REJECT with `rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip`. Pin confirms Envoy v1.37.2 disposition.

### 8.5 `track_per_rule_stats: true` envoy-go-only large-N parse-rejection (optional)

If operator-observability shows real-world large-N policy-map foot-guns (e.g., 1000+ policies × 2 counters each = 2000+ counters per filter instance under track), an envoy-go-only N-cap could be introduced at parse-time. Brainstorm position: NO cap in phase-16 MVP (mirrors Envoy's permissive disposition); document the foot-gun at BEHAVIOR_CONTRACT phase-16 forward-pointer notes; revisit if real-world deployments hit problems. §11 pin §16.P10 measures the per-counter cost under Envoy at large N.

### 8.6 LOG-action dynamic-metadata emission

Per Q4-partial. envoy-go silent-emits the `envoy.common.access_log_hint` dynamic metadata key. Couples to a future dynamic-metadata family phase that lands the `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive (or analogous filter-state accessor for setting metadata visible to downstream filters/access log). Re-activation enables LOG-action wire-shape equivalence. Operator divergence-window: configs setting `action: LOG` with downstream access-log integration expecting the access_log_hint hint see envoy-go's access-log lacking the hint.

### 8.7 Shadow-rules access-log integration

The shadow-engine emits counters per §2.5 + §6.2 scenario 8 but does NOT emit access-log entries marking shadow decisions. Envoy v1.37.2 may emit an access-log entry annotated with the shadow disposition; envoy-go MVP emits only the counter. Couples to access-log subsystem feature (post-phase-06.2). Re-activation enables shadow-decision-annotated access-log entries. §11 pin §16.P13 confirms Envoy's shadow access-log emission shape.

### 8.8 Matcher-engine terminal action TypedExtensionConfig types beyond canonical RBAC `Action`

Envoy's `xds.type.matcher.v3.Matcher` admits arbitrary TypedExtensionConfig as the terminal `on_match` action. envoy-go MVP supports only the canonical RBAC `Action` enum types (or whatever TypeURL §11 pin §16.P3 confirms Envoy v1.37.2 uses for RBAC matcher-engine terminal-actions). Non-RBAC TypeURLs PARSE-REJECT with envoy-go-only error `rbac: matcher action type %q unsupported in this build`. Couples to the matcher-engine framework primitive's extension pluggability — a future matcher-extension family phase could open the TypeURL set.

### 8.9 `Principal_Authenticated` outside URI SAN / DNS SAN / Subject DN canonical fields

`Principal_Authenticated.principal_name` StringMatcher matches against URI SAN, DNS SAN, then Subject DN. Envoy v1.37.2 might also expose additional cert fields (issuer DN, serial number, fingerprint) via additional Principal sub-fields; envoy-go MVP covers only the canonical three. §11 pin §16.P14 confirms Envoy's full principal-name extraction algorithm including any additional fields. Couples to a future TLS-context-extension phase.

### 8.10 `Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState` runtime always-no-match

Parse-supported per Q3 Large + §2.3, but the underlying sourced-metadata + filter-state subsystems are not yet shipped. envoy-go MVP evaluates these to always-FALSE at runtime. Documented divergence-window. Couples to dynamic-metadata family (sourced_metadata) and filter-state family (filter_state). Re-activation enables full evaluation. §11 pin §16.P15 measures Envoy v1.37.2's default dynamic-metadata + filter-state population under fixture conditions (without operator-set metadata/filter-state, Envoy's matcher likely also returns no-match — minimizing the divergence in real-world configs).

---

## 9. Empirical pins for SPEC §11

The SPEC author (lifecycle-state 1 → 2) executes these pins IN-SESSION against reference Envoy v1.37.2 per ADR-0004. Each pin either RATIFIES the brainstorm hypothesis (→ no SPEC §11 amendment) or AMENDS it (→ SPEC §11 amendment-block + possibly a §12 brainstorm-amendment cycle if the empirical re-frame is too large for the §11 amendment-block channel — phase 13 precedent).

**P1 — Exact `RBACPerRoute` proto shape.** CRITICAL pin. Confirm: single `rbac` field at field 2 + reserved field 1 (brainstorm hypothesis from .pb.go inspection)? Or alternate shape? Determines the 7th-canonical ADR-0125 §(xii) amendment paragraph wording + §2.6 parsePerRoute flow.

**P2 — PGV requirements on each consumed field.** Are `rules`/`matcher` (oneof) PGV-required (at-least-one-set)? Is `shadow_rules_specifier` truly optional? Is `rules_stat_prefix` REQUIRED or OPTIONAL? Confirm exact PGV constraints on all 7 top-level fields + the rules-engine and matcher-engine sub-fields + Permission/Principal sub-fields.

**P3 — Matcher-engine terminal action TypeURL set.** CRITICAL pin. What TypedExtensionConfig TypeURL(s) does Envoy v1.37.2 use for RBAC matcher-engine terminal `on_match` actions? Brainstorm hypothesis: a single canonical "RBAC Action" TypeURL pointing at the `RBAC_Action` enum type. Confirm the exact TypeURL + the matcher-engine parsing/dispatch flow at Envoy source. Determines §8.8 deferral scope + ADR-0142 supported-set wording.

**P4 — `action: LOG` exact behavior.** CRITICAL pin. Does Envoy v1.37.2 always-allow under LOG (regardless of match)? Does it run the match evaluator to determine the metadata key value? Does it emit counters during LOG mode (allowed/denied separate from logged, or instead-of)? Brainstorm hypothesis: always-allow + match-runs + `logged` counter increments + `allowed`/`denied` may also increment depending on match. Confirm exact counter-emission disposition for LOG.

**P5 — Exact 403 wire shape.** CRITICAL pin. Confirm: body byte-exact `RBAC: access denied` (brainstorm hypothesis, 19 bytes ASCII)? Or alternate text? 4-header set lowercase wire-form? Status 403? Connection keep-alive (no `connection: close`)? Determines §4 wire-shape discipline.

**P6 — Exact stat names + counter/gauge disposition.** Does Envoy v1.37.2 emit `allowed`/`denied`/`shadow_allowed`/`shadow_denied` as counters (brainstorm hypothesis)? Does it emit `logged` as a separate counter or inline within `allowed`? Does it emit any gauges (e.g., active-shadow-evaluations gauge)? Confirm exact name list, scope, and counter-vs-gauge disposition. Documents at ADR-0145.

**P7 — Prometheus tag-extractor name + namespace flattening rule.** SN2 reuse vs new SN10 rule. Hypothesis: SN2 reuse (HCM-stat-prefix tag is sufficient; rules_stat_prefix + counter-name is enough). Pin confirms; if amendment needed, SN10 lands.

**P8 — Per-route override `rules_stat_prefix` emission scope.** Sub-pin of P9. If P9 is INDEPENDENT, does the per-route's `rules_stat_prefix` field drive a wholly-own counter namespace, OR does it share-with-listener-level? Brainstorm hypothesis: wholly-own counter namespace.

**P9 — Per-route stat SHARED-vs-INDEPENDENT.** CRITICAL pin (mirrors phase-11/13/14/15 question). Does Envoy emit per-route counters into the listener-level counter namespace (SHARED) or into a per-route namespace tagged by the override's `rules_stat_prefix` (INDEPENDENT)? Brainstorm hypothesis: INDEPENDENT. ADR-0145 codifies the resolution.

**P10 — `track_per_rule_stats: true` per-policy counter emission cost + format.** What is the exact counter-name format Envoy v1.37.2 emits for per-rule counters? Hypothesis: `policy.<name>.allowed` + `policy.<name>.denied`. Confirm exact format + measure per-counter cost (memory) under large-N policy maps (1000 policies × 2 counters = 2000 counters per filter instance) — if Envoy hits practical limits beyond a certain N, document at §8.5 deferral.

**P11 — Permission_Set + Principal_Set recursion depth bound.** What is Envoy v1.37.2's exact recursion depth bound for `and_rules`/`or_rules`/`not_rule` + `and_ids`/`or_ids`/`not_id`? Hypothesis: 32 or 64 levels of nesting before parse-time rejection. envoy-go mirrors at parse time (depth-exceeded error).

**P12 — Deprecated `metadata` Permission + Principal disposition.** Does Envoy v1.37.2 parse-reject configs setting `permission.metadata` or `principal.metadata`? Or does it accept them with deprecated-but-functional semantic? Hypothesis: accepts but logs deprecation warning. envoy-go-side disposition: PARSE-REJECT (envoy-go-only validation; cleaner than silent-ignore for deprecated fields).

**P13 — Shadow access-log integration.** Does Envoy v1.37.2 emit an access-log entry annotated with shadow disposition (e.g., a new access-log field `RBAC_SHADOW_ALLOW_OR_DENY`)? Or are shadow decisions counter-only? Hypothesis: counter-only in current Envoy. Pin confirms + amends §8.7 deferral if Envoy emits.

**P14 — `Principal_Authenticated` full algorithm.** CRITICAL pin (drives ADR-0144 framework primitive scope). What is the exact principal-name extraction algorithm Envoy v1.37.2 uses? Hypothesis: URI SAN (first), then DNS SAN, then Subject DN. Confirm exact ordering + cert-field set (does Envoy also consider Issuer DN, Serial Number, fingerprints?). Determines the ADR-0144 accessor API surface.

**P15 — SourcedMetadata + FilterState default values under fixture conditions.** Under fixture-baseline single-stream load, what is Envoy v1.37.2's default dynamic-metadata + filter-state population? Hypothesis: empty (no metadata set by upstream filters); SourcedMetadata and FilterState matchers all return no-match. Confirms the §8.10 always-no-match envoy-go MVP behavior is the default-case-equivalent (minimizing real-world divergence).

**P16 — Listener-level config field types accessed by Permission variants.** Confirm: `destination_ip` (CidrRange) compares against what? Envoy's listener-bound IP? Or the connection's local-side IP at connection-receive time? Same question for `destination_port` and `destination_port_range`. Determines the envoy-go-side accessor pattern for these variants (already shipped via phase-07.2 listener-chain-completion for the SNI variant; `destination_*` are analogous accessor pulls). Brainstorm hypothesis: connection's local-side IP+port at receive time.

**P17 — Listener-level vs per-stream access path for SNI.** `Permission_RequestedServerName` matches the SNI. Phase-07.2's listener-chain-completion exposes SNI to listener filters; phase 16's RBAC needs SNI in the HCM filter chain. Confirm the access path — is the SNI value cached on the connection accessor and surfaced to HCM filters? Or does HCM-side need an explicit accessor extension?

**P18 — XFF resolution algorithm for `Principal_RemoteIp`.** What is Envoy v1.37.2's exact XFF resolution algorithm under various `xff_num_trusted_hops` settings? Brainstorm hypothesis: rightmost-trusted-hop algorithm per Envoy documentation. Determines the envoy-go-side accessor for `Principal_RemoteIp` (versus `Principal_DirectRemoteIp` which is always the peer's source-IP).

---

## 10. ROADMAP delta

### 10.1 New row added by this brainstorm

A single ROW is added to `ROADMAP.md` (the table after row 15 per phase-09-onward flat-row convention; per ADR-0106 the §9 family-rows are flat top-level rows):

| id | title | depends-on | status | sub-phases | summary |
|---|---|---|---|---|---|
| 16 | http-filter-rbac | 15 | planned |  | New `internal/filter/http/rbac/` package implementing `envoy.filters.http.rbac` (Envoy v1.37.2 canonical role-based-access-control filter; decode-side policy gate) under the 07.1 framework. NINTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15). MVP envelope per BRAINSTORM Q1-Q7: 7 fields consumed proto-faithful (Q2) — `rules` + `matcher` (rules_specifier oneof; rules wins when both set), `shadow_rules` + `shadow_matcher` (shadow_rules_specifier oneof), `rules_stat_prefix`, `shadow_rules_stat_prefix`, `track_per_rule_stats`. Inside `config.rbac.v3.RBAC`: `action` enum ALLOW + DENY + LOG-partial (Q4); `policies` map<name, Policy{permissions OR principals AND condition-silent-ignored}>; `audit_logging_options` silent-ignored. Permission MVP Large 11+11 (Q3): `any`, `header`, `url_path`, `destination_ip`, `destination_port`, `destination_port_range`, `requested_server_name`, `and_rules`, `or_rules`, `not_rule`, `sourced_metadata` (with runtime always-no-metadata-match divergence). Principal MVP Large 11+11: `any`, `authenticated` (TLS principal name; NEW framework primitive ADR-0144), `direct_remote_ip`, `remote_ip`, `header`, `url_path`, `and_ids`, `or_ids`, `not_id`, `sourced_metadata`, `filter_state` (with runtime always-no-match divergences). DEFERRED: deprecated `metadata` Permission/Principal + `source_ip` Principal (PARSE-REJECT envoy-go-only); extension-coupling `matcher`/`uri_template` Permission (PARSE-REJECT); CEL `condition`/`checked_condition` policy fields (silent-ignore per Q7); `audit_logging_options` (silent-ignore — not-implemented upstream). Per-route TPFC `RBACPerRoute{rbac: <RBAC>|nil}` — NEW 7th canonical absent-implies-disabled-OR-wholesale-override per ADR-0125 §(xii) amendment (Q6; DISTINCT from 5th canonical's explicit-disabled-bool-in-oneof). Stat surface 60→65+ names (5 base counters minimum: `allowed`, `denied`, `logged`, `shadow_allowed`, `shadow_denied`; variable upward with `track_per_rule_stats: true` 2N per-policy-name counters per active prefix — Q5; foot-gun documented). Per-route stats INDEPENDENT per ADR-0145 (mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT precedent). **TWO new framework deltas** — FIRST §9 row since phase 14 to introduce non-zero deltas + FIRST single phase to introduce TWO: (i) ADR-0144 — TLS-principal accessor on `DecoderFilterCallbacks` (required by `Principal_Authenticated`; exposes downstream client cert URI SAN / DNS SAN / Subject DN); (ii) ADR-0142 — matcher-engine evaluator at new `internal/matcher/` package (or SPEC-determined location) implementing `xds.type.matcher.v3.Matcher` generic match-tree primitive (cross-phase-reusable by future filters ext_authz/jwt_authn/oauth2). Deny-path wire shape `SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` — body byte-exact 19 bytes hypothesis (§11 pin §16.P5 confirms); 4-header set lowercase wire-form; connection keep-alive. Differential fixture `0018-http-rbac` (8 scenarios: allow-by-header, deny-no-match, allow-by-url-path, allow-by-destination-port, allow-by-direct-remote-ip, allow-by-tls-principal-via-mTLS-listener, per-route 7th-canonical disabled-via-absent, per-route wholesale-override with INDEPENDENT stat namespace + shadow). 20th fuzzer `FuzzRBACConfigParse`. **Anticipated 7 ADRs (ADR-0140 through ADR-0146)** plus ADR-0125 §(xii) amendment paragraph; LARGEST §9-row ADR roster to date. Per ADR-0106, §9 family-rows are flat top-level rows; phase 16 lands as row `16`. **ADR-0045 surface-split release valve HIGHLY anticipated** at SPEC time — estimated ~2200-3000+ LoC well above the 1500-LoC trigger; recommended split is 16.1 (rules-engine + Large 11+11 + per-route 7th canonical + 4-counter aggregate + TLS-principal framework delta; ~1200-1600 LoC) + 16.2 (matcher-engine + shadow + LOG-partial + track_per_rule_stats + per-rule counters + matcher-engine framework primitive; ~1000-1400 LoC). SPEC author confirms split-or-single-row decision at SPEC drafting time.

### 10.2 §9 family heading at ROADMAP line 62 stays unchanged

Per ADR-0106(c). The line `### HTTP filters family` and the family-children enumeration at ROADMAP line 64 are unchanged across this brainstorm + the eventual phase-done landing.

### 10.3 No-sibling-stub discipline (per ADR-0106(b))

This brainstorm authors NO sibling stubs in ROADMAP for the 9 not-yet-brainstormed §9 family-children (`jwt_authn`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

---

## 11. Open structural questions for SPEC author (carry-forwards)

Items that the SPEC author resolves at lifecycle-state 1 → 2 transition. Each is decision-bearing — neither the brainstorm hypothesis nor the empirical pin alone resolves it; both inputs + the SPEC author's judgment are required.

### 11.1 Should phase 16 PRE-COMMIT to the 16.1 + 16.2 split, or land as a single row?

Brainstorm position: ADR-0045 split HIGHLY anticipated (per §1.4) but NOT pre-committed at brainstorm time per Q approval pick "Approved — author the doc" (not "approved with split-at-brainstorm-time"). LoC estimate ~2200-3000+ LoC, task estimate ~22-30 tasks; ABOVE ADR-0045 1500-LoC / 25-task split-trigger. The split-by-engine-and-observability (rules-engine + framework deltas + 4-counter base @ 16.1; matcher-engine + shadow + LOG-partial + track + per-rule @ 16.2) is mechanically clean. SPEC author re-evaluates at SPEC time with the empirical pin resolutions in hand (some pins might surface significantly more surface — e.g., if §16.P14 reveals a richer-than-expected TLS-principal extraction algorithm, or §16.P11 reveals a smaller-than-expected recursion depth bound requiring complex parse-time validation).

### 11.2 Matcher-engine evaluator location: `internal/matcher/` vs scoped

Per §3.2 + ADR-0142. Brainstorm preference: `internal/matcher/` new top-level package (cleanest cross-phase reuse). SPEC author confirms — if the framework-survey at SPEC time reveals only marginal cross-phase reuse opportunity (e.g., ext_authz/jwt_authn use only specific terminal-action types not generalizable), a scoped location at `internal/filter/http/rbac/matcher.go` is acceptable. Decision affects ADR-0142 scope.

### 11.3 TLS-principal accessor API shape: list-returning vs matcher-applying

Per §3.1 + ADR-0144. Brainstorm preference: list-returning (`DownstreamPrincipal() []string`) for flexibility. SPEC author may prefer matcher-applying (`DownstreamPrincipalMatches(matcher) bool`) for efficiency. Decision affects ADR-0144 scope.

### 11.4 INDEPENDENT-vs-SHARED stats — divergence-window or match Envoy?

Per §5 + §11.P9. If Envoy emits SHARED stats, envoy-go has two paths: (a) match Envoy (SHARED) or (b) DIVERGE (INDEPENDENT; divergence-window documented). The operationally-correct shape is INDEPENDENT; the byte-equivalent shape is whatever Envoy emits. SPEC author + user decide.

### 11.5 Should phase 16 carry forward a phase-13-style §12 amendment cycle?

Phase 13 buffer had a post-landing §12 amendment cycle. Phase 14 compressor + phase 15 bandwidth_limit did not (the empirical pins resolved cleanly at SPEC time). Phase 16's amendment-cycle posture: NOT anticipated at brainstorm time — the proto-faithful design admits direct §11 pin resolution; the framework-delta scope is well-known. The SPEC author retains the §12 channel as a release valve if a pin resolution unexpectedly reframes a brainstorm decision (e.g., if §16.P3 reveals matcher-engine terminal-action types far more complex than anticipated).

### 11.6 Filter-chain ordering with respect to other filters

Phase 16 RBAC is recommended FIRST in the HCM filter chain (immediately after listener filters; before header_mutation/buffer/compressor/bandwidth_limit). This avoids cost on denied requests. Fixture pins this ordering; SPEC author may revise if framework-survey reveals an issue with rbac-first placement (e.g., a header_mutation filter that mutates `X-User` BEFORE rbac evaluates it would require ordering header_mutation BEFORE rbac — a real-world operator case that must be documented).

### 11.7 Deprecated-field disposition: parse-reject vs silent-ignore

Per §8.3 + §8.4 + §11.P12. Brainstorm preference: PARSE-REJECT deprecated Permission_Metadata + Principal_Metadata + Principal_SourceIp (envoy-go-only validation; explicit foot-gun). If Envoy v1.37.2 is lenient (accepts deprecated fields with warning), envoy-go's strict rejection is a divergence-window. SPEC author + pin resolution decide whether to match Envoy's lenience or maintain strict rejection.

### 11.8 LOG-action counter semantics

Per §2.4 + §2.8 + §11.P4. Does `action: LOG` increment `logged` counter ONLY, or also `allowed`/`denied` based on whether any policy matched? Brainstorm hypothesis: `logged` always increments under LOG; `allowed` and `denied` may also increment depending on match result (Envoy may emit both for full observability). SPEC pin confirms.

### 11.9 `track_per_rule_stats` envoy-go-only N-cap

Per §8.5 + §11.P10. If §11 pin §16.P10 reveals Envoy v1.37.2 hits a practical N-cap or performance degradation at high N, envoy-go could impose a parse-time N-cap (e.g., max 256 policies if track_per_rule_stats: true). Brainstorm position: NO cap in MVP; revisit if real-world observability shows a foot-gun.

### 11.10 Matcher-engine terminal-action type set

Per §3.2 + §8.8 + §11.P3. The matcher-engine's `on_match` returns TypedExtensionConfig. Phase 16 MVP supports only the canonical RBAC Action terminal type(s). Pin confirms exact TypeURL set. If Envoy v1.37.2 admits additional terminal types (e.g., for matcher-extension plugins), envoy-go's PARSE-REJECT for unknown types is a divergence-window. SPEC author + pin resolution decide.

---

## End of phase 16 brainstorm
