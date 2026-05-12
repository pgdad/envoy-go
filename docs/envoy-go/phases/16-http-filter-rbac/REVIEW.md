# Phase 16 — Code review (REVIEW.md)

**Phase id:** `16` (NINTH §9 HTTP-filters family-row to land per ADR-0106; FIRST §9 row since phase 14 to introduce non-zero framework deltas; FIRST single phase to introduce **TWO** new framework primitives simultaneously per ADR-0142 (matcher-engine at NEW top-level package `internal/matcher/`) + ADR-0144 (`DecoderFilterCallbacks.DownstreamPrincipal() []string` TLS-principal accessor); FIRST row to introduce a NEW canonical per-route discipline (**7th canonical** — absent-implies-disabled-OR-wholesale-override) per the ADR-0125 §(xii) in-place amendment; THIRD row to ship stateful-INDEPENDENT-stats per ADR-0117 precedent — phase-11 local_ratelimit FIRST, phase-15 bandwidth_limit SECOND, phase-16 rbac THIRD; LARGEST anticipated-ADR roster of any §9 row to date (**7** ADRs ADR-0140..ADR-0146); FIRST row since phase-14 to require an impl-time-unanticipated ADR (ADR-0147 TLS-layer mTLS-lift; lifted at Task 13 follow-up per ADR-0044 escape-valve))
**Slug:** `16-http-filter-rbac`
**Branch under review:** `phase-16-http-filter-rbac-impl`
**Range:** branch tip `6ab026f` (Task 15 phase-done at this SHA; this REVIEW.md is the final Task 16 commit). 15 task-landing commits across 15 tasks + 1 Task 14 follow-up `gofmt` lint fix at `570ce50`; phase-done at `6ab026f`. Lifecycle-state advance state-5 → state-6 deferred to the post-`wt-merge` SHA-fill follow-up commit per phase-13/14/15 close pattern.
**Parent ROADMAP row:** `16 http-filter-rbac` flipped `in-progress → done` at Task 15 commit `6ab026f` (already landed prior to this REVIEW; row 16's status field reads `done` on the impl branch at HEAD with date `2026-05-12`).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 16 explicit allowance (the controller IS the agent per the cold-start prompt — no sub-subagent needed); inputs: SPEC §15 13-claim acceptance checklist + PLAN's 16-task structure + the branch diff + phase-13 + phase-14 + phase-15 REVIEW.md structural templates + PROGRESS.md per-task entries (Tasks 1-15 with verbatim outputs) + DECISIONS.md ADR-0140..ADR-0147 + ADR-0125 §(xii) amendment paragraph + BEHAVIOR_CONTRACT §13.1-§13.6 6-edit bundle.
**Six-gate state at HEAD:** all green per Task 15's verification sweep — outputs reproduced verbatim in §4 below.

This review covers the full phase 16 surface: `internal/filter/http/rbac/` package (`doc.go` 148 LoC + `rbac.go` 1191 LoC + `evaluator.go` 854 LoC + `rbac_test.go` 3601 LoC + `fuzz_test.go` 357 LoC); the NEW top-level package `internal/matcher/` (`doc.go` 75 LoC + `matcher.go` 510 LoC + `matcher_test.go` 689 LoC) — the FIRST cross-phase-reusable framework primitive in envoy-go to live outside `internal/filter/`; the ADR-0144 framework delta at `internal/filter/http/callbacks.go` (+18 LoC `DownstreamPrincipal()` interface method) + `internal/filter/http/chain.go` (+45 LoC TLS-principal threading) + `internal/filter/hcm/connection.go` (+99 LoC TLS-state extraction + threading) + `internal/filter/hcm/h2dispatch.go` (+55 LoC dispatch-time wiring) + `internal/filter/hcm/filter.go` (+10 LoC) + new `internal/filter/hcm/tls_test.go` (+201 LoC); the ADR-0147 TLS-layer mTLS-lift at `internal/tls/config.go` (+31 LoC `RequireAndVerifyClientCert` path lifted SCOPED to well-formed mTLS configs); boot registration at `cmd/envoy-go/main.go` (alphabetical-after-localratelimit per ADR-0140); differential fixture `0018-http-rbac` (8 scenarios incl. mTLS scenario 6; envoy.yaml 296 LoC + envoy-go.yaml 242 LoC + expectations.yaml 255 LoC + driver.go 1053 LoC + README.md 304 LoC + `pki/` mTLS PKI generator); fixture-driver wiring at `test/differential/runner_test.go` (+30 LoC blank-import + dispatch); `FuzzRBACConfigParse` (twentieth fuzzer in repo at `internal/filter/http/rbac/fuzz_test.go`); BEHAVIOR_CONTRACT 6-edit bundle (NEW rbac subsection §13.1 at line 1487 + 60→64-name table extension §13.2 + Equivalence Matrix row §13.3 + Phase 16 forward-pointer notes §13.4 at line 2010 enumerating the 6 divergence-windows per ADR-0146 §Context + NEW top-level `## HTTPFilterCallbacks` umbrella §13.5 with `### DownstreamPrincipal accessor` subsection + NEW top-level `## Matcher engine framework primitive` section §13.6); the seven anticipated ADRs ADR-0140..ADR-0146 + the impl-time-unanticipated ADR-0147 + ADR-0125 §(xii) amendment paragraph; the ROADMAP row 16 status flip + STATE.md advance to lifecycle-state-5.

This REVIEW closes phase 16's lifecycle (state 5 → 6 at the post-merge SHA-fill follow-up commit) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED with two explicit DONE_WITH_CONCERNS carry-overs from Task-14 fixture-integration time, both documented + auditable; neither is a regression.**

All six phase-done gates are GREEN at HEAD `6ab026f` (Task 15 phase-done at this SHA; this REVIEW is the only commit past `6ab026f`) per the Task 15 verification sweep (§4 below). The implementation faithfully realizes the SPEC across all 16 PLAN tasks. The rbac filter is the NINTH §9 HTTP-filters family-row to ship under ADR-0106 and the FIRST since phase 14 to introduce non-zero framework deltas. It is the FIRST single phase to introduce TWO new framework primitives simultaneously: (i) the matcher-engine evaluator at the NEW top-level package `internal/matcher/` per ADR-0142 — explicitly designed cross-phase-reusable for future filters (ext_authz / jwt_authn / oauth2 / ext_proc all consume the same `xds.type.matcher.v3.Matcher` proto for parts of their config surface) with `supportedActionTypes` extension + additive `MatchContext` widening hooks; and (ii) the TLS-principal accessor `DecoderFilterCallbacks.DownstreamPrincipal() []string` per ADR-0144 — explicitly designed for cross-phase reuse by future TLS-aware filters. It is the THIRD §9 row to use stateful-INDEPENDENT-stats per ADR-0117 precedent (phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND; phase-16 rbac THIRD). It is the FIRST §9 row to introduce a NEW canonical per-route discipline — the 7th canonical per-route shape (absent-implies-disabled-OR-wholesale-override) — landed as the ADR-0125 §(xii) in-place amendment at Task 10 per phase-13 ADR-0127-v2 + phase-14 §(viii)-(x) + phase-15 §(xi) in-place-update precedent. It is the FIRST §9 row since phase 14 to require an impl-time-unanticipated ADR via ADR-0044's escape-valve: ADR-0147 lifts the phase-03 TLS-layer blanket rejection of `require_client_certificate=true` SCOPED to well-formed mTLS configurations (validation_context.trusted_ca PEM provided); the unanticipation was surfaced at Task 13 when fixture 0018 scenario 6 required mTLS-server-verify at the listener level to thread the cert URI SAN through the ADR-0144 accessor.

The architectural centerpiece is the **dual-engine proto-faithful dispatch** (ADR-0141) — `rules` (RBAC policy map) and `matcher` (xds matcher-tree) are independently-settable optional fields at the proto level (per §1.1 amendment 2 — they share a UDPA `field_alias` annotation but generate as TWO SEPARATE pointer fields, NOT a Go-level oneof); the filter-source-enforced "rules wins when both set" semantic is mirrored in envoy-go's `buildCompiledConfig`. The matcher-engine path delegates to the framework primitive at `internal/matcher/`; the rules-engine path walks the policy map lexicographically. Both paths emit the SAME 4-base-counter shape per ADR-0145 (`allowed` / `denied` / `shadow_allowed` / `shadow_denied`) — REFUTES BRAINSTORM's 5-counter hypothesis; NO `logged` counter exists in Envoy v1.37.2 (`utility.h::ENFORCE_RBAC_FILTER_STATS` macro defines exactly 2 counters, plus 2 shadow counters via `SHADOW_RBAC_FILTER_STATS`). LOG-action folds into `allowed` since LOG always-allows. The per-policy counter family (when `track_per_rule_stats: true`) is lazily allocated via `sync.Map.LoadOrStore` + `NewCounterIfAbsent` post-Freeze per ADR-0145 §Decision (iii); the counter name shape `<base_prefix>.policy.<policy_name>.<suffix>` was **empirically refined at Task 8** — the SPEC §13.2 line 1842 hypothesis omitted the `.policy.` segment infix; the impl-time scrape against reference Envoy v1.37.2 revealed the literal segment + the ADR-0145 + ADR-0146 trail codified the refinement.

The differential fixture `0018-http-rbac` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 8 scenarios — scenario 1 (ALLOW + match → 200 + `allowed=1`); scenario 2 (ALLOW + no-match → 403 + `denied=1`); scenario 3 (DENY + match → 403 + `denied=1`); scenario 4 (DENY + no-match → 200 + `allowed=1`); scenario 5 (LOG + match → 200 + `allowed=1` + per-policy `.allowed=1`); scenario 6 (mTLS + Principal_Authenticated three-case — exercises ADR-0144 framework primitive + ADR-0147 TLS-layer mTLS-lift); scenario 7 (shadow primary-ALLOW + shadow-DENY → 200 + `allowed=1` + `shadow_denied=1`); scenario 8 (matcher-engine + `track_per_rule_stats` per-policy emission). All 8 PASS at the 4-base-counter delta-equal + per-policy delta-equal + 19-byte byte-exact deny-body assertion + 4-header lowercase wire-form.

Seven anticipated ADRs landed (ADR-0140..ADR-0146) all SPEC-anticipated per ADR-0044's standard discipline; ONE impl-time-added ADR (ADR-0147) via ADR-0044's escape-valve clause (mirrors phase-14 ADR-0134 + phase-13 ADR-0127-v2 precedent). Plus ADR-0125 §(xii) in-place amendment paragraph at Task 10 — phase-16 ADR roster ends at ADR-0147 inclusive; next-free ADR advances to ADR-0148.

---

## 2. ADR roster

Each of the seven anticipated ADRs ADR-0140..ADR-0146 + the impl-time-unanticipated ADR-0147 + the ADR-0125 §(xii) in-place amendment, evaluated for whether the §Decision body held up under implementation + fixture exercise:

**ADR-0125 §(xii) in-place amendment** (NEW **7th canonical** per-route pattern — wrapper proto `RBACPerRoute` with reserved field 1 + single optional sub-message field `rbac` at field 2; ABSENCE → disabled-on-route per proto comment; PRESENCE → wholesale-override of listener config per ADR-0073 wholesale-not-merge inheritance; structurally distinct from the 5th canonical (explicit-`disabled`-bool-in-oneof; phase-13 buffer + phase-14 compressor) AND the 6th canonical (bare-message-via-TPFC + code-level-required-field; phase-15 bandwidth_limit); Lands-at: Task 10): **VALIDATED.** Fixture 0018 scenario 7 + scenario 8 exercise the per-route-presence path (wholesale override + INDEPENDENT-stats); the per-route-absence path is structurally exercised by all scenarios that do not set TPFC for the route. The ADR-0125 canonical-pattern roster grows from 6 to 7; future §9 rows whose per-route proto follows the same "wrapper-with-reserved-field-and-single-optional-sub-message" shape compose against this canonical.

**ADR-0140** (`internal/filter/http/rbac/` package shape — single-token directory matching cors / fault / csrf / buffer / compressor / localratelimit / bandwidthlimit precedent + DECODER-only `HTTPFilter` value (`Encoder: nil`; 3rd §9 row to ship pure decode-side per phase-12 csrf + phase-13 buffer precedent) + 4-base-counter `filterStats` per §1.1 amendment 8 (REFUTES BRAINSTORM 5-counter hypothesis) + lazy per-policy counter allocation via `NewCounterIfAbsent` post-Freeze + deny-path wire shape `SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` per §1.1 amendment 10 + §11.P5 + boot-registration alphabetical-after-localratelimit; Lands-in: Task 2): **VALIDATED.** Single-token directory `rbac/` aligns with precedent. DECODER-only shape: `SetEncoderCallbacks` never invoked; `EncodeHeaders/EncodeData/EncodeTrailers` never called; the filter's full responsibility is the decode-side policy gate evaluation in `DecodeHeaders`. 4-counter `filterStats` shape verified at unit-test Group 9 + fixture 0018 counter assertions. SendLocalReply 4-header lowercase wire-form (`content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`) byte-equivalent to Envoy per fixture-time scrape. Boot-registration ordering alphabetically correct.

**ADR-0141** (`compiledConfig` shape + 7-consumed proto-faithful field decomposition per §1.1 amendment 1 + dual-engine dispatch table (rules xor matcher; rules-wins-when-both-set per §1.1 amendment 2) + UDPA-`field_alias`-annotation framing (NOT a Go-level oneof) + envoy-go-side defensive PGV-mirror validation per amendment 4 + ALLOW + DENY + LOG-partial action enum per amendment 5 + CEL three-field silent-ignore per amendment 6 + `audit_logging_options` silent-ignore; Lands-in: Task 2): **VALIDATED.** Outer envelope's 7 fields all consumed proto-faithful — the silent-ignore set lives ONE LEVEL DEEPER inside `config.rbac.v3.RBAC` (audit_logging_options + 3 CEL fields per amendment 6 refinement of BRAINSTORM's 2-CEL claim). Dual-engine dispatch verified at unit-test Group 5 + fixture 0018 scenario 8 (matcher-engine path). Both-set precedence rule confirmed at `TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins` + analogous shadow test. envoy-go-own error wording per phase-11 ADR-0115 + phase-15 ADR-0136 precedent.

**ADR-0142** (Matcher-engine evaluator framework primitive at NEW top-level package `internal/matcher/` — cross-phase reusable; `Matcher` opaque type + `New(tree, supportedActionTypes)` constructor with PARSE-REJECT for unknown terminal `Any.TypeUrl` + `Evaluate(MatchContext) (*anypb.Any, error)` walker returning `(nil, nil)` for no-match + `MatchContext` interface with initial accessor subset (Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName); Lands-in: Task 3): **VALIDATED.** 585 LoC across `doc.go` (75) + `matcher.go` (510); test surface 689 LoC at `matcher_test.go`. Cross-phase reuse intent codified at the package-level doc.go + ADR-0142 §Decision (iii). The package lives at `internal/matcher/` (NOT under `internal/filter/`) explicitly to anchor the cross-phase reusability — future filters consume `matcher.New(tree, supportedActionTypes)` + extend `supportedActionTypes` for their own terminal action TypeURLs without re-rolling the match-tree walker. RBAC's `supportedActionTypes = ["type.googleapis.com/envoy.config.rbac.v3.Action"]` per ADR-0142 §Decision + §11.P3 + §2.6.

**ADR-0143** (Permission + Principal Large 11+11 evaluators + AND/OR/NOT recursive combinators + Permission/Principal evaluator interface design + deprecated-field PARSE-REJECT discipline + extension-coupling PARSE-REJECT including the NEW `Principal_Custom` 14th variant per §1.1 amendment 7 + SourcedMetadata + FilterState always-no-match runtime semantic + `Principal_Authenticated` three-case algorithm per §1.1 amendment 12; Lands-in: Tasks 4 + 5): **VALIDATED.** Permission Large 11: `any` / `header` / `url_path` / `destination_ip` / `destination_port` / `destination_port_range` / `requested_server_name` / `and_rules` / `or_rules` / `not_rule` / `sourced_metadata` — all 11 evaluators verified at Group 3 unit tests. Principal Large 11: `any` / `authenticated` / `header` / `url_path` / `direct_remote_ip` / `remote_ip` / `and_ids` / `or_ids` / `not_id` / `sourced_metadata` / `filter_state` — all 11 evaluators verified at Group 4 unit tests. The 3+3 DEFERRED variants PARSE-REJECT with envoy-go-only error wording (envoy-go-strict divergence from Envoy v1.37.2 lenient-accept-with-deprecation-warning): Permission `metadata` / `matcher` / `uri_template`; Principal `source_ip` / `metadata` / `custom` (the LATTER is NEW per amendment 7; Principal has 14 variants not 13 as BRAINSTORM hypothesized). The `Principal_Authenticated` three-case algorithm landed verbatim per amendment 12 — case (a) nil principal_name + `len(DownstreamPrincipal)>0` → TRUE; case (b) non-nil StringMatcher iteration over URI SAN/DNS SAN/Subject DN candidates; case (c) plaintext → FALSE.

**ADR-0144** (TLS-principal accessor on `DecoderFilterCallbacks` framework primitive — `DownstreamPrincipal() []string` returning priority-ordered candidates from `tls.ConnectionState.PeerCertificates[0]` (URI SAN values from `.URIs[]` first, DNS SAN values from `.DNSNames` second, Subject DN Common Name third) + plumbing from connection-level TLS state through HCM dispatch to per-stream filter-callback + three-case algorithm consumption + cross-phase reuse intent + LIST-returning API + canonical 3-cert-field scope; Lands-in: Task 6): **VALIDATED.** Plumbing: `internal/filter/hcm/connection.go` (+99 LoC) extracts `*tls.Conn.ConnectionState()` at HCM dispatch time; `internal/filter/hcm/h2dispatch.go` (+55 LoC) + `internal/filter/hcm/filter.go` (+10 LoC) thread principals into chain via `chain.SetTLSPrincipals(principals)` before `RunDecodeHeaders`; `internal/filter/http/chain.go` (+45 LoC) stores per-stream; `internal/filter/http/callbacks.go` (+18 LoC) surfaces the interface method. h2spec 53/53 PASS at Gate C confirms the plumbing preserves h2 conformance. Cross-phase reuse intent codified at ADR-0144 §Decision + the BEHAVIOR_CONTRACT §13.5 `## HTTPFilterCallbacks` umbrella section (NEW; future filters jwt_authn / ext_authz / oauth2 / ext_proc extend the umbrella additively).

**ADR-0145** (rbac stat surface — 4 base counters registered UNCONDITIONALLY under HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` + lazy per-policy counter family via `sync.Map` LoadOrStore + `NewCounterIfAbsent` post-Freeze + per-route INDEPENDENT-stats per `newFilterStatsIfAbsent` (third row using INDEPENDENT-stats after phase-11 ADR-0117 + phase-15 ADR-0139) + per-policy counter name shape `<base_prefix>.policy.<policy_name>.<suffix>` empirically refined at Task 8; Lands-in: Task 8): **VALIDATED with Task-8 empirical refinement.** SN2-reuse hypothesis from §1.1 amendment 9 + §11.P7 was carried forward to impl-time empirical confirmation; Task 8's reference Envoy v1.37.2 `/stats` scrape RATIFIED the SN2-reuse shape — NO new SN10 rule needed (existing `internal/stats/name.go::flattenToProm` default-branch handles `http.<HCM>.rbac.<rules>.<counter>` verbatim via the `http.` segment routing). The per-policy counter shape was REFINED at the same scrape: the SPEC §13.2 line 1842 hypothesis `<rules_stat_prefix>.<policy_name>.<suffix>` omitted the `.policy.` segment infix; the empirical shape is `<base_prefix>.policy.<policy_name>.<suffix>` per `utility.h::incPolicyAllowed/Denied/ShadowAllowed/ShadowDenied` calling convention. ADR-0145 §Decision (iii) codifies the refined shape; the SPEC document itself retains the original (refuted) framing per phase-15 SPEC-immutable-post-commit precedent (the ADR-trail is the authoritative reference; the SPEC §13.2 row-template is operator-orienting and remains useful in its hypothesis form).

**ADR-0146** (rbac shadow-evaluation discipline parallel-to-primary engine walk + LOG-partial divergence-window + `track_per_rule_stats` per-policy emission discipline + `response_code_details` field-emission divergence-window + shadow-rules access-log integration deferred + BEHAVIOR_CONTRACT §13.4 phase-16 forward-pointer notes anchoring; Lands-in: Task 9): **VALIDATED.** The 6 divergence-windows enumerated at §Context anchor the BEHAVIOR_CONTRACT §13.4 phase-16 forward-pointer notes subsection (the BEHAVIOR_CONTRACT edit lands at Task 15). Shadow walk invocation gate `cc.shadowRules != nil || cc.shadowMatcher != nil` exercised at fixture 0018 scenario 7 (primary-ALLOW + shadow-DENY → 200 + `allowed=1` + `shadow_denied=1`; shadow disposition NEVER affects primary dispatch). LOG-partial precise discipline (LOG → always `engineResultAllowed`; matched policy captured for per-policy `.allowed`) exercised at scenario 5. Per-policy emission gating `cc.trackPerRuleStats && policyName != "" && suffix != ""` exercised at scenario 8. `response_code_details` non-emission verified at scenario 3 fixture-time byte-exact 4-header set match (envoy-go matches Envoy's wire-bytes on the SendLocalReply path; the divergence-window is non-asserted under fixture-baseline conditions per §11.P15).

**ADR-0147** (Phase-03 TLS-layer `require_client_certificate=true` blanket-rejection lifted SCOPED to well-formed mTLS configurations — maps onto stdlib `crypto/tls.RequireAndVerifyClientCert` mode + `ClientCAs` pool populated from parsed trusted_ca; **IMPL-TIME-UNANTICIPATED** via ADR-0044 escape-valve; Lands-in: Task 13 follow-up): **VALIDATED.** Phase-03's `internal/tls/config.go.NewDownstreamConfig` blanket-rejected `require_client_certificate=true` per ADR-0032 §Decision (7); the rejection was structurally appropriate at phase 03 (TCP+TLS bootstrap; no listener-level filter required mTLS-server-verify), but blocks fixture 0018 scenario 6 — the listener `l_test_a_tls` MUST require a verified client cert to thread the cert URI SAN to the rbac filter's `authenticated_admin` policy via the ADR-0144 `DownstreamPrincipal()` framework primitive. The lift is SCOPED — only well-formed mTLS configurations (validation_context.trusted_ca PEM provided) pass; malformed configs (require_client_certificate=true without trusted_ca) still parse-reject. ADR-0147 mirrors phase-14 ADR-0134 + phase-13 ADR-0127-v2 in-task-lift precedent under ADR-0044 §Decision (b) impl-time-anchoring.

---

## 3. Empirical pins outcome

All 18 SPEC §11 pins were resolved at SPEC drafting: **10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME-CONFIRMED-AT-TASK-8 + 1 DEFERRED** (per SPEC §11 summary disposition table at line 1358). The structural design (TWO new framework primitives, dual-engine proto-faithful, 11+11 Large MVP, INDEPENDENT-stats hypothesis, 7th canonical per-route, ALLOW + DENY + LOG-partial action enum, 4-counter base, response_code_details deferral) survived intact through all refutations.

Final per-pin dispositions:

| Pin | Topic | Final disposition | Impl-time confirmation |
|---|---|---|---|
| **§11.P1** | `RBACPerRoute` proto shape | **RATIFIED** | Task 10 ADR-0125 §(xii) landing |
| **§11.P2** | PGV requirements per consumed field | **PARTIAL/REFRAMED → RATIFIED** | Task 2 buildCompiledConfig PGV mirror landed |
| **§11.P3** | Matcher-engine terminal action TypeURL set | **RATIFIED** | Task 3 `internal/matcher/` constructor PARSE-REJECT verified |
| **§11.P4** | `action: LOG` exact behavior | **REFINED → RATIFIED** | Task 9 LOG-partial emission gate + amendment 5 |
| **§11.P5** | Exact 403 wire shape | **RATIFIED + NEW finding** | Task 7 SendLocalReply 4-header verified at fixture 0018 scenario 3 |
| **§11.P6** | Stat names + counter disposition | **REFUTED (4 not 5 counters)** | Task 8 stat surface 4-counter shape verified |
| **§11.P7** | Prometheus tag-extractor + namespace | **PARTIAL → RATIFIED at Task 8** | Task 8 empirical scrape — SN2-reuse confirmed; NO new SN10 rule |
| **§11.P8** | Per-route override `rules_stat_prefix` emission scope | **RATIFIED-PENDING → RATIFIED at Task 8** | Task 8 empirical scrape — per-route INDEPENDENT-stats wiring verified |
| **§11.P9** | Per-route stat SHARED-vs-INDEPENDENT | **RATIFIED-PENDING → RATIFIED at Task 8** | Task 8 + Task 14 fixture 0018 scenario 7+8 INDEPENDENT-stats confirmed |
| **§11.P10** | `track_per_rule_stats` per-policy counter format | **PARTIAL → REFINED at Task 8** | Task 8 empirical scrape REFINED SPEC line 1842 — `.policy.` segment infix added (ADR-0145 §Decision (iii) authoritative; SPEC retains hypothesis form per phase-15 precedent) |
| **§11.P11** | Permission_Set + Principal_Set recursion depth bound | **RATIFIED-VIA-ABSENCE** | Task 4 + Task 5 AND/OR/NOT combinators no parse-time depth-cap |
| **§11.P12** | Deprecated `metadata` Permission + Principal disposition | **RATIFIED** | Task 4 + Task 5 PARSE-REJECT envoy-go-only error wording verified |
| **§11.P13** | Shadow access-log integration | **DEFERRED** | Task 9 ADR-0146 §Decision (v) forward-pointer (Envoy also counter-only; no current divergence) |
| **§11.P14** | `Principal_Authenticated` full algorithm | **RATIFIED-AND-EXTENDED** | Task 5 + Task 6 three-case algorithm + nil-principal-name = any-authenticated-user verified |
| **§11.P15** | SourcedMetadata + FilterState default values | **RATIFIED** | Task 4 + Task 5 always-no-match runtime semantic verified |
| **§11.P16** | Listener-level config field types for Permission variants | **RATIFIED** | Task 4 listener-port + listener-IP accessors verified via phase-07.2 primitives |
| **§11.P17** | Listener-level vs per-stream access path for SNI | **RATIFIED** | Task 4 SNI accessor via phase-07.2 listener-chain-completion verified |
| **§11.P18** | XFF resolution algorithm for `Principal_RemoteIp` | **RATIFIED** | Task 5 phase-04 + phase-05 XFF resolver reused verbatim |

**Summary:** 10 RATIFIED (P1, P3, P5, P11, P12, P14, P15, P16, P17, P18) + 2 REFUTED (P6 4-not-5 counters + P2 in its PGV-mirror reframing, although both ultimately RATIFIED post-refinement) + 3 PARTIAL/REFINED (P2 → final RATIFIED; P4 → final RATIFIED via amendment 5 + 8; P10 → REFINED via Task 8 `.policy.` segment empirical scrape) + 3 RATIFIED-PENDING-IMPL-TIME-CONFIRMED-AT-TASK-8 (P7, P8, P9 — all RATIFIED at Task 8 empirical scrape against reference Envoy) + 1 DEFERRED (P13 shadow access-log; envoy-go matches Envoy counter-only; no current divergence; documented as forward-pointer for future Envoy releases).

**Task-8 empirical scrape against reference Envoy v1.37.2 was load-bearing for FOUR pins** (P7, P8, P9, P10) — the SPEC carried these forward as RATIFIED-PENDING + PARTIAL with the explicit understanding that the impl-time scrape would either ratify or refine. The scrape RATIFIED P7-P9 verbatim (SN2-reuse + per-route INDEPENDENT shape both confirmed) and REFINED P10 (the `.policy.` segment infix was empirically established + the ADR-0145 §Decision (iii) trail codifies the refined shape).

**Lesson:** SPEC §11 empirical pins are guidance subject to impl-time refinement; the Task-8 stat-surface scrape is the canonical place for RATIFIED-PENDING pin closure. The ADR-trail (ADR-0145 + ADR-0146) carries the refined shape; the SPEC document remains in its hypothesis form per the immutability discipline (phase-15 precedent). This is the same lesson phase-14 + phase-15 surfaced (4 per-side counter divergences; per-side wall-clock + per-side counter discipline); phase-16 generalizes it to "stat-surface namespace + per-policy counter format claims are subject to impl-time refinement; the impl-time scrape is the authoritative truth-source."

---

## 4. Gate-by-gate evidence

Verbatim from PROGRESS.md Task 15 outputs (`docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` lines 1082-1326). All 6 gates green at HEAD `6ab026f`:

**Gate A — build + vet + lint clean:**
```
$ go build ./...           (BUILD_RC=0; no output)
$ go vet ./...             (VET_RC=0; no output)
$ golangci-lint run ./...  (LINT_RC=0; no output)
```

**Gate B — race-test pass across all packages:**
```
$ go test -race -count=1 ./...
[Initial run flaked on 0017 scenario 2 — phase-15-known timing-divergence; not a regression]
Re-run: 43 `ok` package verdicts + 0 FAIL
ok  github.com/esalaine/envoy-go/internal/filter/http/rbac           1.069s
ok  github.com/esalaine/envoy-go/internal/matcher                    1.068s
[... 41 other internal + fixture packages PASS ...]
```

**Gate C — h2spec 53/53 PASS at ADR-0051 pin:**
```
$ go test -v -count=1 -run TestH2Spec ./test/conformance/h2spec/
53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (6.93s)
```
(Confirms phase-16 HCM-side changes — `DownstreamPrincipal` accessor plumbing + TLS-state extraction — preserved h2spec conformance.)

**Gate D — 20 fuzzers green at 30s/each (631s total wallclock):**
```
$ /tmp/run-fuzzers.sh
RESULT: FuzzRBACConfigParse PASS                      (./internal/filter/http/rbac)    [20th fuzzer; phase 16]
[... 19 prior fuzzers PASS at 30s/each ...]
PASS=20 FAIL=0 TOTAL=20 ELAPSED=631s
```

**Gate E — 19 differential fixtures 0000-0018 PASS:**
```
$ go test -v -count=1 -run='TestDifferential' ./test/differential/
--- PASS: TestDifferential/0018-http-rbac (1.54s)
[... 18 prior fixtures 0000-0017 all PASS ...]
--- PASS: TestDifferential (52.99s)
```

**Gate F — BEHAVIOR_CONTRACT.md §13.1-§13.6 6-edit bundle landed:**
```
$ grep -n '^### envoy.filters.http.rbac' docs/envoy-go/BEHAVIOR_CONTRACT.md
1487:### envoy.filters.http.rbac
$ grep -nE '0018-http-rbac' docs/envoy-go/BEHAVIOR_CONTRACT.md
38:| 0018-http-rbac | envoy.filters.http.rbac (decode-side dual-engine policy gate ...
$ grep -n '^### Phase 16 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
2010:### Phase 16 forward-pointer notes
$ grep -n '^### DownstreamPrincipal accessor' docs/envoy-go/BEHAVIOR_CONTRACT.md
1860:### DownstreamPrincipal accessor (per phase 16 ADR-0144)
$ grep -n '^## Matcher engine framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md
1876:## Matcher engine framework primitive (per phase 16 ADR-0142)
All 6 §13 patches landed at expected line positions.
```

---

## 5. Acceptance checklist — SPEC §15 13-claim verification

Per SPEC §15 lines 1986-2000. All 13 claims verified PASS with citations to specific commits + gate outputs + file paths; two explicit DONE_WITH_CONCERNS carry-overs noted inline (claim 6 stat surface refined at Task 8 per ADR-0145 §Decision (iii); claim 11 response_code_details DEFERRED per amendment 11 — both auditable + non-regression).

- [x] **Claim 1.** Phase 16 SPEC.md authored with **12 §1.1 amendment blocks** (5 structural + 7 field-bookkeeping refinements); each amendment cross-referenced to §11 empirical evidence. — **PASS.** Evidence: SPEC §1.1 lines 114-232 (12 amendment blocks landed at SPEC commit `3159811`); cross-references to §11.P1..P14 at each amendment's inline citation.

- [x] **Claim 2.** §3 framework-survey result locked: **TWO new framework deltas**: (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor per ADR-0144; (ii) matcher-engine evaluator at new top-level `internal/matcher/` package per ADR-0142. — **PASS.** Evidence: SPEC §3 lines 309-352 + ADR-0142 + ADR-0144 verbatim; framework-delta files: `internal/filter/http/callbacks.go` (+18) + `internal/filter/http/chain.go` (+45) + `internal/filter/hcm/connection.go` (+99) + `internal/filter/hcm/h2dispatch.go` (+55) + `internal/filter/hcm/filter.go` (+10) + new `internal/filter/hcm/tls_test.go` (+201) + NEW `internal/matcher/` package (585 LoC production + 689 LoC tests). FIRST §9 row since phase 14 to introduce non-zero framework deltas + FIRST single phase to introduce TWO simultaneously.

- [x] **Claim 3.** §11 empirical-pin block: **18 pins** resolved IN-SESSION against reference Envoy v1.37.2 per ADR-0004; 10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME + 1 DEFERRED tally captured. — **PASS.** Evidence: SPEC §11 summary disposition table at line 1358; per-pin transcripts at SPEC §§11.1-11.18 lines 1385-1697. Task 8 RATIFIED-PENDING closure for P7+P8+P9; Task 8 REFINEMENT for P10 (`.policy.` segment infix) per ADR-0145 §Decision (iii). See §3 above for the full disposition table.

- [x] **Claim 4.** Differential fixture: 8 scenarios; byte-exact body assertion (allow paths verbatim + deny paths 19-byte `"RBAC: access denied"`); per-counter delta byte-equivalence on the 4 active base counters per active namespace; per-route INDEPENDENT-stats per scenarios 7 + 8; mTLS scenario 6 exercises ADR-0144 framework primitive. — **PASS.** Evidence: fixture at `test/fixtures/0018-http-rbac/` (envoy.yaml 296 + envoy-go.yaml 242 + expectations.yaml 255 + driver.go 1053 + README.md 304 + pki/gen.go mTLS PKI generator); Gate E pass at `6ab026f`. Counter delta-equal on 4 base counters + per-policy lazy counters verified at scenarios 5 + 7 + 8.

- [x] **Claim 5.** ADR roster: 7 ADRs anticipated (ADR-0140..ADR-0146; LARGEST §9-row roster to date) + ADR-0125 in-place §(xii) amendment paragraph documenting the NEW 7th canonical per-route pattern. — **PASS.** Evidence: DECISIONS.md ADR-0140 line 6765; ADR-0141 line 6815; ADR-0142 line 6905; ADR-0143 line 6976; ADR-0144 line 7113; ADR-0145 line 7210; ADR-0146 line 7285; ADR-0125 §(xii) amendment paragraph at line 5883. Plus ONE impl-time-unanticipated ADR (ADR-0147 line 7364) via ADR-0044 escape-valve (Task 13 follow-up TLS-layer mTLS-lift) — total 8 ADRs landed; next-free advances to ADR-0148.

- [~] **Claim 6.** Stat surface: **4 base counters** + lazy per-policy counter family when `track_per_rule_stats: true`; namespace SN2-reuse hypothesis pending impl-time empirical scrape confirmation; NO new SN10 rule. — **PASS with DONE_WITH_CONCERNS carry-over (Task-8 empirical refinement).** Evidence: ADR-0145 §Decision (i)-(vi) verbatim; 4-counter struct verified at `internal/filter/http/rbac/rbac.go` `filterStats` definition; Group 9 unit tests + fixture 0018 counter assertions PASS. **Concern:** the per-policy counter shape was REFINED at Task 8 — the SPEC §13.2 line 1842 hypothesis omitted the `.policy.` segment infix; the empirical shape per reference Envoy v1.37.2 is `<base_prefix>.policy.<policy_name>.<suffix>`. ADR-0145 §Decision (iii) + ADR-0146 §Decision (iii) codify the refined shape; the SPEC document retains the hypothesis form per phase-15 SPEC-immutable-post-commit precedent. Operator-facing impact: BEHAVIOR_CONTRACT §13.2 60→64-name table extension (Task 15) carries the corrected shape; operators querying per-policy counters by `grep policy` see the correct names regardless of SPEC's intermediate framing.

- [x] **Claim 7.** Per-route surface: **NEW 7th canonical per-route pattern** documented at ADR-0125 §(xii) amendment; structurally distinct from 5th canonical (explicit-disabled-bool-in-oneof) AND 6th canonical (bare-message-via-TPFC + code-level-required-field); INDEPENDENT-stats per ADR-0145 (THIRD row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent). — **PASS.** Evidence: SPEC §5 lines 450-491 + ADR-0125 §(xii) amendment paragraph (DECISIONS.md line 5883) + ADR-0145 §Decision verbatim; `parsePerRoute` + `resolvePerRouteConfig` impl at `internal/filter/http/rbac/rbac.go`; fixture 0018 scenarios 7+8 (per-route override + INDEPENDENT-stats) Gate E pass at `6ab026f`.

- [x] **Claim 8.** Dual-engine dispatch: rules-engine + matcher-engine BOTH proto-faithful per Q2; rules wins when both set; shadow path parallel-but-never-affects-disposition; ALLOW + DENY + LOG-partial action enum honored; LOG-partial silent-no-metadata-emit divergence-window documented. — **PASS.** Evidence: ADR-0141 §Decision (dual-engine dispatch table) + ADR-0146 §Decision (i) (shadow parallel-walk discipline); rules-wins precedence verified at `TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins`; matcher-engine integration verified at fixture 0018 scenario 8; LOG-partial precise discipline verified at scenario 5; shadow primary-ALLOW + shadow-DENY verified at scenario 7.

- [x] **Claim 9.** Permission + Principal Large 11+11 MVP per Q3; Permission has 14 variants (3 deferred); Principal has 14 variants per §1.1 amendment 7 (3 deferred — adds `custom` to the deferred list compared to BRAINSTORM); all deferred variants PARSE-REJECT with envoy-go-only error wording. — **PASS.** Evidence: ADR-0143 §Decision verbatim; Permission Large 11 + 3 PARSE-REJECT verified at Group 3 unit tests (Task 4); Principal Large 11 + 3 PARSE-REJECT verified at Group 4 unit tests (Task 5); `Principal_Custom` test skipped under v1.32.4 binding (the 14th variant lands at v1.37.2; the default-arm PARSE-REJECT structurally handles); ADR-0143 §Decision (iv) verbatim error wording locked.

- [x] **Claim 10.** TWO new framework primitives: ADR-0142 matcher-engine evaluator at new `internal/matcher/` package; ADR-0144 TLS-principal accessor on `DecoderFilterCallbacks` with three-case algorithm. — **PASS.** Evidence: ADR-0142 + ADR-0144 verbatim; framework-delta file diffs cited at claim 2. The `internal/matcher/` package is the FIRST framework primitive in envoy-go to live outside `internal/filter/` — explicitly cross-phase-reusable per ADR-0142 §Decision (iii) + the BEHAVIOR_CONTRACT §13.6 NEW top-level section. The `DownstreamPrincipal()` accessor lives in the NEW BEHAVIOR_CONTRACT §13.5 `## HTTPFilterCallbacks` umbrella section — future filters extend the umbrella additively.

- [~] **Claim 11.** Deny-path wire shape: 403 + body byte-exact `"RBAC: access denied"` (19 bytes); 4-header set lowercase; keep-alive; `response_code_details` field-emission divergence-window per §1.1 amendment 11 + §8.12. — **PASS with DONE_WITH_CONCERNS carry-over (response_code_details DEFERRED).** Evidence: ADR-0140 §Decision (deny-path wire-shape) + ADR-0146 §Decision (iv) (response_code_details divergence-window); fixture 0018 scenarios 2+3 byte-exact wire-bytes match at Gate E. **Concern:** envoy-go MVP does NOT emit `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` on DENY responses; reference Envoy emits via `utility.cc::responseDetail`. The deferral couples to a future HCM response-code-details framework phase. Under fixture-baseline conditions (no access log; no trace; just wire bytes + `/stats`) NO operator-visible divergence (per §11.P15); the divergence-window is observable only at access-log formatters consuming `%RESPONSE_CODE_DETAILS%` or trace span tags.

- [x] **Claim 12.** Twelve §1.1 amendment blocks document the SPEC-time refutations/refinements cleanly via the §1.1 amendment-block channel (NOT §12 BRAINSTORM-amendment cycle). — **PASS.** Evidence: SPEC §1.1 lines 114-232 (12 amendment blocks landed at SPEC commit `3159811`); BRAINSTORM.md §12 NOT amended in-place; mirrors phase-12 csrf (4 amendments) + phase-14 compressor (6 amendments) + phase-15 bandwidth_limit (10 amendments) precedent at extended-scale volume.

- [x] **Claim 13.** STATE.md updated post-SPEC: lifecycle-state-2 → 3 transition (SPEC-done, awaiting PLAN); next-skill `superpowers:writing-plans`; ROADMAP.md row 16 `planned → in-progress`. — **PASS.** Evidence: STATE.md SPEC-done flip at commit `3159811`; STATE.md PLAN-done flip at `40f030b`; STATE.md phase-done flip at `6ab026f` (lifecycle-state-4 → 5; this REVIEW.md closes lifecycle-state-5 → 6 at the post-merge SHA-fill follow-up per phase-13/14/15 close pattern); ROADMAP.md row 16 `planned → in-progress → done` at the same commits.

**Summary:** 11 claims PASS; 2 claims PASS-with-DONE_WITH_CONCERNS-carry-over (claim 6 per-policy counter shape refined at Task 8 per ADR-0145 §Decision (iii); claim 11 response_code_details DEFERRED per amendment 11 + ADR-0146 §Decision (iv)). Both concerns are documented in ADR-trail + BEHAVIOR_CONTRACT §13.2 + §13.4; not regressions; auditable. NO claims BLOCKED.

---

## 6. Divergence-window roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 16 forward-pointer notes" at line 2010 + ADR-0146 §Context enumeration:

**(i) LOG-action `access_log_hint` dynamic-metadata emission DEFERRED** per §1.1 amendment 5 + §8.6 + ADR-0146 §Decision (ii). envoy-go MVP: silent. Envoy: emits `envoy.common.access_log_hint` dynamic metadata key (true on match; false on no-match). Couples to future dynamic-metadata family framework phase landing `(EncoderFilterCallbacks).SetDynamicMetadata(key, value)` primitive. Operator divergence-window: configs setting `action: LOG` with downstream access-log integration expecting the access_log_hint hint see envoy-go's access-log lacking the hint.

**(ii) `response_code_details` field-emission on DENY DEFERRED** per §1.1 amendment 11 + §8.12 + ADR-0146 §Decision (iv). envoy-go MVP: no emission (`SendLocalReply` 3-arg signature has no response-code-details slot). Envoy: emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"` via `utility.cc::responseDetail`. Couples to future HCM response-code-details framework phase. Under fixture-baseline conditions NO operator-visible divergence (wire bytes byte-identical); divergence observable only at access-log formatters consuming `%RESPONSE_CODE_DETAILS%` or trace span tags.

**(iii) CEL three-field condition evaluation DEFERRED** per §1.1 amendment 6 + Q7 + §8.1 + ADR-0146 §Decision (iii). envoy-go MVP: `Policy.condition` + `Policy.checked_condition` + `Policy.cel_config` all silent-ignored at parse + runtime (`policyMatches` skips condition evaluation entirely; treats condition as always-true). Refines BRAINSTORM §2.7's 2-CEL-field claim — the proto has THREE CEL fields per amendment 6 + §11.P2. Couples to future CEL runtime framework phase landing `internal/cel/` evaluator + `github.com/google/cel-go` dependency. Operator divergence-window: policies relying on CEL conditions for fine-grained control see envoy-go-vs-Envoy decision DIVERGENCE on the condition-driven policy slice.

**(iv) Shadow-rules access-log integration NO CURRENT DIVERGENCE** per §8.7 + §11.P13 + ADR-0146 §Decision (v). envoy-go MVP: counter-only. Reference Envoy v1.37.2: ALSO counter-only (the §11.P13 empirical confirmation closed the BRAINSTORM §6 hypothesis — no shadow-decision-annotated access-log entries in Envoy either). Documented as forward-pointer because future Envoy releases MAY introduce shadow access-log integration; envoy-go's deferral is preemptive.

**(v) `sourced_metadata` + `filter_state` runtime always-no-match** per §2.5 + §8.10 + ADR-0143 §Decision (v) + ADR-0146 §Context. envoy-go MVP: parse-supported but evaluator returns FALSE for `Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState`. Envoy: actual evaluation against dynamic-metadata + filter-state subsystems. In practice no operator-visible divergence under fixture-baseline default-empty conditions per §11.P15; the divergence-window is structural (couples to dynamic-metadata + filter-state families).

**(vi) `Principal_Authenticated` canonical 3 cert fields only** per §1.1 amendment 12 + §8.9 + D11 + ADR-0144 §Decision (canonical 3-cert-field scope) + ADR-0146 §Context. envoy-go MVP: URI SAN + DNS SAN + Subject DN Common Name (in priority order). Additional cert fields (Issuer DN, Serial Number, fingerprints) NOT exposed. Couples to future TLS-context-extension phase.

---

## 7. Framework-delta impact + cross-phase reuse intent

Phase 16 introduces TWO new framework primitives — the LARGEST framework-delta in a §9 row since phase 14 (which introduced ONE new primitive at ADR-0131). Both deltas are explicitly designed for cross-phase reuse:

**Matcher-engine primitive at `internal/matcher/` (ADR-0142).** Lives outside `internal/filter/` — the FIRST envoy-go framework primitive to live in its own top-level package. The location anchors the cross-phase reusability intent: future filters consuming `xds.type.matcher.v3.Matcher` for parts of their config surface (ext_authz / jwt_authn / oauth2 / ext_proc all use the same matcher proto for portions of their routing / authorization / authentication policy graphs) call `matcher.New(tree, supportedActionTypes)` + extend `supportedActionTypes` for their own terminal action TypeURLs. The `MatchContext` interface is widened additively per ADR-0142 §Decision (iii) — RBAC's initial accessor subset (Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName) is the seed; future filters add accessors without breaking RBAC.

**TLS-principal accessor `DownstreamPrincipal() []string` (ADR-0144).** Lives on `DecoderFilterCallbacks` per the existing phase-07.1 callbacks surface. Future TLS-aware filters (jwt_authn extracts the bearer-token JWT's subject claim; ext_authz forwards the TLS principal to an external auth service; oauth2 binds the OAuth identity to the TLS cert; ext_proc threads the TLS principal to an external processor) consume the same accessor without re-rolling TLS-context plumbing. The BEHAVIOR_CONTRACT §13.5 NEW `## HTTPFilterCallbacks` umbrella section is the operator-facing anchor; future filters extend the umbrella additively with their own callback subsection.

**Cross-phase reuse intent is load-bearing for the framework-survey discipline.** The phase-16 ADR roster (ADR-0142 + ADR-0144) is explicit about cross-phase reuse in the §Decision body of each — NOT a deferred consideration. Future §9 family-row brainstorms should evaluate whether the matcher-engine primitive or the TLS-principal accessor can be reused before proposing new framework deltas. This is the same lesson phase-13/14/15 surfaced (the cumulative framework-primitive accumulation enables cleaner downstream phases); phase-16 generalizes it to "framework primitives that are explicitly cross-phase-reusable at introduction time enable faster downstream filter onboarding."

---

## 8. Test counts + verification surface

**Unit tests at phase-done (HEAD `6ab026f`):**
- `internal/filter/http/rbac/`: **100 PASS + 2 SKIP** (rbac_test.go 3601 LoC across 9 test groups + fuzz_test.go 357 LoC). The 2 SKIP are go-control-plane v1.32.4 binding gaps: (a) `cel_config` field not present at v1.32.4 (silent-ignore disposition is structurally encoded at `buildCompiledRulesEngine`'s NO-CEL-fields read); (b) `Principal_Custom` field not present at v1.32.4 (PARSE-REJECT disposition is structurally encoded in `buildOnePrincipal`'s default arm). Both re-activate when the module bumps past v1.32.x to expose the fields.
- `internal/matcher/`: matcher_test.go 689 LoC test surface (matcher framework primitive).
- `internal/filter/hcm/`: tls_test.go 201 LoC NEW (TLS-principal accessor plumbing).
- `internal/tls/`: config_test.go +45 LoC (ADR-0147 mTLS-lift coverage).
- `internal/filter/http/`: callbacks_test.go + chain_test.go +116 LoC (DownstreamPrincipal interface + per-stream threading).

**Differential fixtures:** 19/19 PASS (0000-0018) at Gate E (52.99s wallclock; under SPEC §14.6 60-90s budget).

**Fuzzers:** 20/20 PASS at Gate D @ 30s each (631s total). `FuzzRBACConfigParse` is the 20th fuzzer; corpus seeds 0-12 verified at PROGRESS.md Task 11.

**h2spec:** 53/53 at ADR-0051 pin (Gate C; 6.93s wallclock).

**Production LoC:** ~2778 — rbac.go 1191 + evaluator.go 854 + doc.go 148 = 2193 LoC rbac package; matcher.go 510 + doc.go 75 = 585 LoC matcher package. Plus framework-delta deltas: ~228 LoC (callbacks.go +18 + chain.go +45 + connection.go +99 + h2dispatch.go +55 + filter.go +10 + tls/config.go +31 − 30 for the trim of the previous blanket-reject path).

**Fixture LoC:** 2150 — envoy.yaml 296 + envoy-go.yaml 242 + expectations.yaml 255 + driver.go 1053 + README.md 304.

**Documentation deltas:** BEHAVIOR_CONTRACT.md +162 LoC (6-edit bundle); DECISIONS.md +672 LoC (7 anticipated ADRs + 1 unanticipated ADR + ADR-0125 §(xii) amendment paragraph); PROGRESS.md 1330 LoC (the 16-task ledger).

---

## 9. Deferred items + open follow-ups

For future phase consideration (none are regressions; all are auditable in the trail):

1. **`:path` injection has no targeted unit test (Task 14 self-flagged concern 2).** The `:path` pseudo-header injection (used in fixture 0018 scenarios where `url_path` matchers fire) is exercised end-to-end via the differential fixture but lacks a dedicated unit test in `rbac_test.go`. Test gap; tractable refactor.

2. **Scenarios 4+5 destination_port / direct_remote_ip canonical end-to-end deferral (Task 14 reshape).** The fixture exercises `Permission_DestinationPort` + `Principal_DirectRemoteIp` evaluators via unit tests; the end-to-end canonical fixture scenario covering both was deferred at the Task 14 reshape to keep the fixture footprint at 8 scenarios. Future fixture-extension phase could add a 9th scenario or extend an existing one.

3. **Prometheus tag-extractor structural close deferred (Task 14 driver-normalization absorbs).** The Task 14 reshape moved the Prometheus tag-extractor verification into the driver-normalization pass rather than a standalone tag-extractor assertion. Functionally equivalent; the tag-extractor surface is verified end-to-end at the `/stats/prometheus` scrape comparison; structural close is deferred.

4. **dead-code `perSideScrape` + mutex in driver.go (Task 14 code-quality I-1).** The 0018 driver carries a `perSideScrape` helper + mutex that were superseded during the Task 14 reshape but not removed. Could clean at REVIEW; left in place to avoid scope creep at the phase-done commit. Cleanable in a future cleanup pass.

5. **SafeRegex per-call compilation tech-debt (Task 4 / 5 code-quality note).** The Permission/Principal evaluators compile SafeRegex patterns on every match call rather than at parse time. Mirror approach to existing project shared-infrastructure pattern matchers; tractable to extract a shared SafeRegex cache in a future refactor.

6. **`rbac_test.go` file split deferred (Task 14/16 cleanup per Task 5 CF-5).** The 3601-LoC test file is the largest single test file in the project; splitting along the 9 test groups (Group 1 config + Group 2 factory + Group 3 Permission + Group 4 Principal + Group 5 DecodeHeaders dispatch + Group 6 SendLocalReply + Group 7 per-route + Group 8 shadow + Group 9 stats) is a future refactor. Mirrors phase-15's analogous split-deferral.

7. **Shadow access-log integration DEFERRED per ADR-0146 §Decision (v).** envoy-go matches Envoy v1.37.2 counter-only; preemptive forward-pointer for future Envoy releases that MAY introduce shadow access-log integration.

8. **`response_code_details` framework primitive DEFERRED per ADR-0146 §Decision (iv).** Couples to future HCM response-code-details framework phase.

9. **LOG-action `access_log_hint` dynamic-metadata primitive DEFERRED per ADR-0146 §Decision (ii).** Couples to future dynamic-metadata family framework phase.

10. **CEL three-field condition evaluation DEFERRED per amendment 6 + ADR-0146 §Decision (iii).** Couples to future CEL runtime framework phase landing `internal/cel/` + `cel-go` dependency.

11. **`Principal_Custom` v1.32.4 binding-absent workaround.** The Principal `custom` 14th variant (per amendment 7) is not present in the v1.32.4 go-control-plane binding; PARSE-REJECT is structurally encoded in `buildOnePrincipal`'s default arm. The explicit `case *rbac.Principal_Custom` arm + the dedicated unit test re-activate when the module bumps past v1.32.x. Same for the `cel_config` field at amendment 6.

12. **`log.Printf` in `resolvePerRouteConfig` parse-fail path.** Uses stdlib `log.Printf` for parse-failure logging; should use the project logger when one exists. Tech-debt note; low priority (parse-failure is a startup-time error, not a hot path).

13. **`track_per_rule_stats` operator-config-driven N-cap foot-gun** per §2.10 + ADR-0146 §Decision (iii). No envoy-go-only parse-time N-cap on policy-map size; mirrors Envoy permissive disposition. Documented at BEHAVIOR_CONTRACT phase-16 forward-pointer notes. Future operator-ergonomics phase MAY add an envoy-go-only N-cap.

14. **Principal_Set / Permission_Set recursion depth foot-gun** per §11.P11. No parse-time depth-cap (RATIFIED-VIA-ABSENCE — Envoy also no cap). Documented as forward-pointer; future operator-ergonomics phase MAY add an envoy-go-only depth-cap.

15. **SPEC §13.2 line 1842 per-policy counter shape retains hypothesis form.** The Task 8 empirical scrape refined the shape to `<base_prefix>.policy.<policy_name>.<suffix>` (adding the `.policy.` segment infix); the ADR-trail (ADR-0145 §Decision (iii) + ADR-0146 §Decision (iii)) is the authoritative reference. The SPEC document retains the original framing per phase-15 SPEC-immutable-post-commit precedent. Future SPEC-housekeeping pass could in-place-amend §13.2 line 1842 for historical clarity (not a regression — the operator-facing BEHAVIOR_CONTRACT §13.2 carries the corrected shape).

---

## 10. Phase-done lessons learned

**TWO new framework primitives in a single phase as a structural data point.** Phase 16 is the FIRST §9 row to ship TWO framework deltas simultaneously (matcher-engine + TLS-principal accessor). The previous record was phase-14's ONE delta (ADR-0131 encode-side `OverwriteBody`). The deltas are explicitly cross-phase-reusable at introduction time — NOT deferred-consideration. **Lesson:** when SPEC §3 framework-survey identifies multiple deltas, they can ship together in a single phase without operational friction IF the cross-phase reuse intent is explicitly anchored in each ADR's §Decision body. Future §9 rows requiring multiple deltas should explicitly evaluate whether they can ship together vs splitting across phases.

**Impl-time-unanticipated ADR-0147 lift surfaced cleanly at Task 13 + landed at Task 13 follow-up.** The phase-03 TLS-layer blanket-rejection of `require_client_certificate=true` was structurally appropriate at phase-03 (no listener-level filter required mTLS-server-verify at that time) but blocks fixture 0018 scenario 6 at Task 13 (the mTLS path through `DownstreamPrincipal`). The lift is SCOPED — only well-formed mTLS configs pass; malformed configs (require_client_certificate=true without trusted_ca) still parse-reject. **Lesson:** the ADR-0044 escape-valve clause is the right release for impl-time-unanticipated structural lifts (phase-13 ADR-0127-v2 + phase-14 ADR-0134 + phase-16 ADR-0147 precedent). Future phases should expect ONE such lift per phase as a working estimate.

**Task-8 empirical scrape as the canonical RATIFIED-PENDING pin closure mechanism.** 4 of 18 §11 pins (P7, P8, P9, P10) carried forward as RATIFIED-PENDING or PARTIAL with explicit understanding that the Task-8 stat-surface scrape against reference Envoy v1.37.2 would either ratify or refine. The scrape RATIFIED P7-P9 verbatim + REFINED P10 (the `.policy.` segment infix discovered). The ADR-trail (ADR-0145 §Decision (iii)) codified the refinement; the SPEC document retained its hypothesis form per phase-15 immutability precedent. **Lesson:** SPEC § 11 pins with PARTIAL or RATIFIED-PENDING dispositions are a SIGNAL that the impl-time scrape is load-bearing for closure. Future phases should explicitly identify which Task in the PLAN owns the empirical-scrape closure for each PARTIAL / RATIFIED-PENDING pin.

**ADR-0125 canonical-pattern roster grows linearly per phase.** Phase 13 contributed the 5th canonical (explicit-`disabled`-bool-in-oneof). Phase 15 contributed the 6th canonical (bare-message-via-TPFC + code-level-required-field). Phase 16 contributes the 7th canonical (absent-implies-disabled-OR-wholesale-override). The §(xii) amendment paragraph mirrors phase-13 §(ix) + phase-14 §(x) + phase-15 §(xi) in-place-update precedent. **Lesson:** the per-route discipline taxonomy at ADR-0125 is a generative framework — each new §9 row tends to introduce a new canonical shape OR reuse an existing one, and the in-place amendment channel scales. Future §9 row brainstorms should check the per-route proto shape against the 7 canonical patterns before proposing a NEW one.

**12-amendment §1.1 channel scales from phase-14's 6 to phase-15's 10 to phase-16's 12.** Phase 16's amendment count grows the SPEC-time §11-pin-driven refinement channel to its largest volume. ALL 12 amendments fit within the §1.1 self-contained-prose-block channel without requiring a BRAINSTORM §12 amendment cycle. **Lesson:** the §1.1 amendment-block channel scales linearly with §11 empirical-pin depth; investing in §11 pin breadth at SPEC time is the right move to maximize amendment-channel volume without operational friction.

**100 PASS + 2 SKIP unit-test surface is the largest single-filter unit-test surface to date.** Phase 16's `rbac_test.go` is 3601 LoC across 9 test groups — the largest single test file in the project. The 2 SKIPs are v1.32.4 binding-gap structurally-redundant tests that re-activate when the module bumps past v1.32.x. **Lesson:** unit-test depth scales with the filter's surface complexity (Permission + Principal Large 11+11 + dual-engine + shadow + LOG-partial + per-route + per-policy counters); future filters of comparable complexity (jwt_authn / ext_authz / oauth2) should plan for similar unit-test surfaces. Future cleanup pass could split `rbac_test.go` along the 9 test groups.

---

## 11. Sign-off

Phase 16 is **ready for master squash-merge via `wt-merge`** per the project memory `feedback_git_worktrees.md` + ADR-0003 worktree-isolation discipline. All 6 phase-done gates green at HEAD `6ab026f`; all 13 SPEC §15 acceptance claims verified PASS (with 2 claims carrying explicit DONE_WITH_CONCERNS notes — claim 6 per-policy counter shape refined at Task 8 per ADR-0145 §Decision (iii); claim 11 response_code_details DEFERRED per ADR-0146 §Decision (iv) — neither is a regression); 7 SPEC-anticipated ADRs landed cleanly + 1 impl-time-unanticipated ADR-0147 via ADR-0044 escape-valve + ADR-0125 §(xii) amendment paragraph; 19 differential fixtures + 20 fuzzers green; h2spec 53/53 at ADR-0051 pin; BEHAVIOR_CONTRACT §13.1-§13.6 6-edit bundle landed at expected line positions; ROADMAP row 16 `done` (`2026-05-12`); STATE.md at lifecycle-state-5 transitioning to lifecycle-state-6 at the post-`wt-merge` SHA-fill follow-up commit (per phase-13/14/15 close pattern; the SHA-fill follow-up is OUT OF SCOPE for this Task 16 worktree commit — it lands on master after squash-merge).

Phase-16 task chain summary: **16 tasks × 17 commits total at worktree HEAD** (15 task-landing commits + 1 Task-14 follow-up gofmt lint fix at `570ce50` + this Task 16 main commit). Phase-done at `6ab026f`; phase-closed at the Task 16 main commit (this REVIEW). The phase did NOT use per-task SHA-fill follow-up commits at the worktree level — the SHA-fill discipline is deferred to the post-`wt-merge` master-side follow-up per the phase-16 PLAN's task allocation.

**End of phase 16 review.**
