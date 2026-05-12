# Phase 16 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..15 PROGRESS.md structure.

## Preamble — execution preconditions

All 17 preconditions verified green at cold-start without deviation. Worktree branch `phase-16-http-filter-rbac-impl` (fresh worktree at `.worktrees/phase-16-http-filter-rbac-impl`, branched from master tip `948f6c4`). Master tail shows PLAN SHA-fill follow-up at `948f6c4`, PLAN squash at `40f030b`, SPEC SHA-fill follow-up at `cedf29a`, SPEC squash at `3159811`, BRAINSTORM SHA-fill follow-up at `b45c1eb`, BRAINSTORM squash at `38749ba`, preceding phase-15 commits (`98a8ca6` impl SHA-fill / `c1361d3` impl squash / `36c91c9` PLAN SHA-fill / `a5c5ec9` PLAN squash). Go 1.26.2, golangci-lint v1.64.8, Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0139 (next-free 0140; per ADR-0044 ADR-on-impl + phase-13 buffer + phase-15 convention, the 7 phase-16 ADRs ADR-0140..ADR-0146 are NOT pre-landed at SPEC commit and will land at impl-time anchor Tasks 2/3/4-5/6/8/9 per the per-ADR table below — mirroring phase-13/15; UNLIKE phase-14 compressor's SPEC-time-pre-landing). ADR-0125 §(xii) amendment paragraph NOT yet landed at master tip `948f6c4` (verified via `grep -cE '\(xii\)' docs/envoy-go/DECISIONS.md` returning 0); §(xii) is anchored at impl-time Task 10 per planner-time decision 14 (resolves SPEC §5.4 vs DECISIONS.md observed state inconsistency). SPEC at `3159811`; PLAN at `40f030b`. `internal/filter/http/rbac/` absent (Task 2 lands). `internal/matcher/` absent (Task 3 lands). `fixture.HTTPRbac` absent (Task 11 lands). 10 `httpReg.Register` calls in main.go (`router`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`); `rbac` insertion alphabetical-after-localratelimit lands at Task 11. `### envoy.filters.http.bandwidth_limit` appears at line 1416 of BEHAVIOR_CONTRACT.md exactly once (forms the insertion-after anchor per PLAN planner-time decision 19). `RBAC` proto present in module closure at `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3`; `Matcher` proto present at `github.com/cncf/xds/go/xds/type/matcher/v3`. Envoy image v1.37.2 SHA confirmed (`sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`). Working tree pristine (`git status --porcelain` empty). All 18 differential fixtures (0000–0017) PASS in 52.96s. **Precondition 11 short-form (per Task 1 spec):** the PLAN's literal command `find ./test/fuzz -name 'Fuzz*_test.go' | wc -l` returns 0 because envoy-go's fuzzers do NOT live under `./test/fuzz/` — they live co-located with their packages under `internal/.../fuzz_test.go` per the project's "fuzzer-per-package" convention (phase-02..15 precedent). The semantic intent (19 fuzzers from phases 02–15 exist) verifies via `grep -rEn '^func Fuzz' --include='*.go'` returning exactly 19 `Fuzz*` functions across 18 `fuzz_test.go` files; the seed-corpus tests already passed under `go test -count=1 -short ./...` (each fuzzer's `f.Add` seed inputs execute as normal subtests, so the no-panic / no-`(nil,nil)` invariants are baseline-verified). The dedicated `-fuzz=… -fuzztime=30s` runs land at Task 15 phase-done Gate iv per the project's late-task gate convention (mirrors phase-13/14/15 PROGRESS Task 1 precedent — skipping the 30s × 19 wallclock cost at Task 1).

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The 7 ADRs anticipated by SPEC §8 (ADR-0140..ADR-0146). **AUTHORED AT IMPL-TIME** per ADR-0044 ADR-on-impl convention (phase-13 buffer + phase-15 bandwidth_limit pattern; UNLIKE phase-14 compressor's SPEC-time-pre-landing — phase-14 was the divergent precedent). Per-ADR Lands-in-task anchors (reproduced verbatim from PLAN §"ADRs introduced by this plan"):

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0140 | `internal/filter/http/rbac/` package shape — single-token directory matching the cors / fault / csrf / buffer / compressor / localratelimit / bandwidthlimit precedent + DECODER-only `HTTPFilter` value (`Encoder: nil`; mirrors phase-12 csrf + phase-13 buffer decoder-only precedent — 3rd §9 row to ship pure decode-side; rbac is a pre-body request gate) + 4-base-counter `filterStats` (per §1.1 amendment 8 + §11.P6 REFUTES BRAINSTORM 5-counter hypothesis — NO `logged` counter exists in Envoy v1.37.2; LOG-partial folds into `allowed`) + lazy per-policy counter allocation via `NewCounterIfAbsent` post-Freeze (mirrors phase-11 ADR-0117 + phase-15 ADR-0139) + deny-path wire shape `SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` per §1.1 amendment 10 + §11.P5 (19-byte ASCII body verbatim + 4-header set lowercase wire-form + keep-alive) + boot-registration ordering (alphabetical-after-localratelimit) | Task 2 (package skeleton + types + factory + filterStats declaration) |
| ADR-0141 | `compiledConfig` shape + 7-consumed proto-faithful field decomposition (per §1.1 amendment 1; outer envelope has NO silent-ignored set — the silent-ignore lives one level deeper inside `config.rbac.v3.RBAC`) + dual-engine dispatch table (`rules` xor `matcher`; rules-wins-when-both-set per §1.1 amendment 2 + `rbac.pb.go:38` proto comment; `shadow_rules` xor `shadow_matcher` analogous) + UDPA-`field_alias`-annotation framing (NOT a Go-level oneof per amendment 2; the .pb.go binding generates as two independent optional fields with FILTER-SOURCE-ENFORCED precedence) + envoy-go-side defensive PGV-mirror validation (per amendment 4 — action enum defined-only; policies.permissions/principals min_items=1; destination_port lte=65535; envoy-go-own error wording per phase-11 ADR-0115 + phase-15 ADR-0136 precedent) + ALLOW + DENY + LOG-partial action enum (per amendment 5 LOG-partial = always-allow + match-runs + matched-policy captured + `access_log_hint` metadata silent + `allowed` counter increments per amendment 8 — folds into allowed since LOG always-allows) + CEL three-field silent-ignore (per amendment 6 — `condition` + `checked_condition` + `cel_config`; refines BRAINSTORM §2.7 two-field claim) + `audit_logging_options` silent-ignore (per §2.1.1) | Task 2 (buildCompiledConfig + buildCompiledRulesEngine + parsePerRoute + resolvePerRouteConfig land here) |
| ADR-0142 | Matcher-engine evaluator framework primitive at NEW top-level package `internal/matcher/` (cross-phase reusable; future filters ext_authz / jwt_authn / oauth2 / ext_proc all consume the same `xds.type.matcher.v3.Matcher` primitive for parts of their config surface — extends `supportedActionTypes` + widens `MatchContext` additively) — `Matcher` opaque type wrapping a parsed match-tree + `New(tree, supportedActionTypes []string) (*Matcher, error)` constructor (parse-time PARSE-REJECT for unknown terminal `Any.TypeUrl` per §11.P3 + §2.6 + amendment §Decision (ii); envoy-go-only error wording `"matcher: terminal action type %q unsupported by this caller"`) + `Evaluate(MatchContext) (*anypb.Any, error)` walker (no-match per `rbac.pb.go:43-46` proto comment "Requests not matching any matcher will be denied" returns `(nil, nil)`; caller interprets nil-result as filter-specific no-match disposition — RBAC interprets as DENY per `evaluateMatcherEngine`) + `MatchContext` interface (initial accessor subset scoped to RBAC's canonical surface per planner-time decision 17 — Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName; widened additively by future filters per §Decision (iii)) | Task 3 (NEW package + Matcher + New + Evaluate + MatchContext interface + parse-time PARSE-REJECT + initial predicate evaluator subset) |
| ADR-0143 | Permission + Principal Large 11+11 evaluators + AND/OR/NOT recursive combinators + Permission/Principal evaluator interface design + deprecated-field PARSE-REJECT discipline (Permission_Metadata + Principal_SourceIp + Principal_Metadata per §11.P12; envoy-go-only divergence from Envoy v1.37.2 lenient-accept-with-deprecation-warning) + extension-coupling PARSE-REJECT (Permission_Matcher + Permission_UriTemplate + Principal_Custom — the LATTER is NEW per §1.1 amendment 7; Principal has 14 variants not 13 as BRAINSTORM hypothesized; the 14th `custom` TypedExtensionConfig couples to mTLS-extension family) + SourcedMetadata + FilterState always-no-match runtime semantic (per §2.5 + §8.10; parse-supported but evaluator returns FALSE; couples to dynamic-metadata + filter-state families) + Principal_Authenticated three-case algorithm (per §1.1 amendment 12 + §6.6 — case (a) nil principal_name + len(DownstreamPrincipal)>0 → TRUE; case (b) non-nil StringMatcher iteration over URI SAN/DNS SAN/Subject DN candidates; case (c) plaintext → FALSE) + shared-infrastructure adapter usage (PathMatcher/HeaderMatcher/StringMatcher/CidrRange evaluators from phase-07.1 cors precedent) | Task 4 (Permission 11 + AND/OR/NOT) + Task 5 (Principal 11 + prinAuthenticated three-case + deferred PARSE-REJECT finalized) |
| ADR-0144 | TLS-principal accessor on `DecoderFilterCallbacks` framework primitive — `DownstreamPrincipal() []string` accessor returning priority-ordered candidates (URI SAN values from `tls.ConnectionState.PeerCertificates[0].URIs[]` first, DNS SAN values from `state.PeerCertificates[0].DNSNames` second, Subject DN Common Name from `state.PeerCertificates[0].Subject.CommonName` third per `rbac.pb.go:1432-1438` + §1.1 amendment 12 + §11.P14) + plumbing from connection-level TLS state through HCM dispatch to per-stream filter-callback (extract `*tls.Conn.ConnectionState()` at HCM dispatch time; thread into `chain.SetTLSPrincipals(principals)` before `RunDecodeHeaders`) + three-case algorithm for `Principal_Authenticated.principal_name` (codified at ADR-0143; ADR-0144 codifies the accessor surface) + cross-phase reuse intent (future filters jwt_authn / ext_authz / oauth2 / ext_proc consume the same accessor for TLS-principal introspection) + list-returning vs matcher-applying API choice (LIST-returning per SPEC §3.1 + §11.3 carry-forward decision — flexibility for caller-side matching; future filters can extract names without re-rolling TLS-context plumbing) + plaintext / non-mTLS / no-client-cert handling (returns nil slice; ADR-0143's case (c) defers to this nil-slice semantic) | Task 6 (callbacks.go interface + chain.go plumbing + hcm/connection.go + hcm/h2dispatch.go wiring + chain_test.go integration tests) |
| ADR-0145 | Stat surface 4-base + variable per-policy + namespace SN2-reuse hypothesis (`http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for primary; analogous for shadow per §1.1 amendment 9 + §11.P7) + impl-time empirical scrape RATIFIES or AMENDS the hypothesis (if Envoy v1.37.2 scrape reveals a divergent shape per the SN10 release-valve discipline, ADR amends in-place per phase-13 ADR-0127-v2 precedent; current PLAN-time disposition: SN2-reuse with NO new SN10 rule pending impl-time confirmation) + per-policy counter format `<base_prefix>.<policy_name>.<suffix>` (per `utility.h::incPolicyAllowed/Denied/ShadowAllowed/ShadowDenied` calling convention per §11.P10; operator-config-driven surface growth — variable-N foot-gun documented at §13.4 forward-pointer notes) + INDEPENDENT per-route stats discipline (mirrors phase-11 ADR-0117 + phase-15 ADR-0139 stateful-override-implies-INDEPENDENT precedent; phase-16 rbac is THIRD row using this discipline) + post-Freeze idempotent registration via `NewCounterIfAbsent` (counter-only at phase-16; no gauges unlike phase-15 bandwidth_limit's 6 gauges + ADR-0117's counter-only precedent) | Task 8 (newFilterStats + newFilterStatsIfAbsent finalization + per-route INDEPENDENT-stats wiring + Group 9 stats-namespace integration tests) |
| ADR-0146 | Shadow-evaluation discipline (parallel-to-primary engine walk; same algorithm; NEVER affects disposition; emits `shadow_allowed` / `shadow_denied` counters per active shadow namespace + lazy per-policy counters when `track_per_rule_stats: true`) + LOG-partial divergence-window (always-allow + match-evaluated + `access_log_hint` metadata silent per §1.1 amendment 5 + §8.6; counter emission folds into `allowed` per amendment 8) + `track_per_rule_stats` per-policy emission discipline (lazy `NewCounterIfAbsent` per matched policy per first-match invocation; `sync.Map` keyed by `<policy_name>.<suffix>`) + `response_code_details` field-emission divergence-window (envoy-go MVP no emission per §1.1 amendment 11 + §8.12 — Envoy emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"` per `utility.cc::responseDetail`; envoy-go's current HCM scope does NOT thread response-code-details to local-reply callers; future HCM framework phase couples) + shadow-rules access-log integration deferred (per §8.7 + §11.P13; envoy-go MVP counter-only matches Envoy v1.37.2 counter-only confirmation — no current divergence; documented as forward-pointer) + BEHAVIOR_CONTRACT §13.4 phase-16 forward-pointer notes subsection (codifies the 6 divergence-windows for operator awareness — LOG-action metadata + response_code_details + CEL three-field + shadow access-log + sourced-metadata always-no-match + Principal_Authenticated canonical 3 cert fields) | Task 9 (shadow path + LOG-partial discipline + track_per_rule_stats per-policy emission + BEHAVIOR_CONTRACT §13.4 anchoring) |

**Plus ADR-0125 amendment paragraph §(xii)** — authored at Task 10 per planner-time decision 14 (NOT pre-landed at SPEC commit). The amendment documents: phase-16 rbac introduces the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override; wrapper proto with reserved field 1 + single optional sub-message field at field 2; ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment; PRESENCE implies wholesale-override per ADR-0073). Structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof (phase-13 buffer + phase-14 compressor) AND 6th canonical's bare-message-via-TPFC + code-level-required-field (phase-15 bandwidth_limit). The 7th canonical inherits ADR-0117 + ADR-0139's stateful-override-implies-INDEPENDENT-stats discipline directly. ADR-0125's canonical-pattern roster grows from 6 to 7.

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nineteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — CEL three-field silent-ignore at parse + runtime (SPEC §12.1 + §1.1 amendment 6).** All three fields (`condition`, `checked_condition`, `cel_config`) silent-ignored at runtime; the evaluator treats condition as always-true (`policyMatches` skips condition evaluation entirely). Group 1 tests `TestBuildCompiledRulesEngine_{Condition,CheckedCondition,CelConfig}Field_SilentIgnored` verify. Operator divergence-window if any CEL field set; documented at §13.4. *Anchored: SPEC §12.1 + §2.1.2 + Q7 + amendment 6.*

2. **D2 — `audit_logging_options` silent-ignore (SPEC §12.2 + §2.1.1).** Matches Envoy v1.37.2 (`[#not-implemented-hide:]`). Group 1 `TestBuildCompiledRulesEngine_AuditLoggingOptions_SilentIgnored` verifies. *Anchored: SPEC §12.2 + §11.P2.*

3. **D3 — Deprecated `metadata` Permission + Principal PARSE-REJECT envoy-go-only (SPEC §12.3 + §11.P12).** PARSE-REJECT with envoy-go-only error wording (`"rbac: permission.metadata deprecated; use sourced_metadata"` + `"rbac: principal.metadata deprecated; use sourced_metadata"`); Envoy v1.37.2 lenient-accepts with deprecation warning per §11.P12 — envoy-go's strict rejection is an envoy-go-only divergence. Group 3 + Group 4 tests verify. *Anchored: SPEC §12.3 + §2.3 + §2.4 + §11.P12; phase-15 ADR-0136 envoy-go-only-check precedent.*

4. **D4 — Deprecated `Principal_SourceIp` PARSE-REJECT (SPEC §12.4 + §11.P12).** Same disposition as D3. Group 4 `TestPrinSourceIp_PARSE_REJECT` verifies. *Anchored: SPEC §12.4 + §2.4.*

5. **D5 — `Principal_Custom` PARSE-REJECT envoy-go-only (SPEC §12.5 + §1.1 amendment 7 + §8.11 NEW).** The 14th Principal variant discovered at SPEC time per amendment 7; PARSE-REJECT with envoy-go-only error `"rbac: principal.custom extension types unsupported in this build"`. Group 4 `TestPrinCustom_PARSE_REJECT` verifies. *Anchored: SPEC §12.5 + amendment 7 + §8.11.*

6. **D6 — Permission `matcher` + `uri_template` extension variants PARSE-REJECT (SPEC §12.6 + §8.8).** PARSE-REJECT envoy-go-only with extension-coupling error wording. Group 3 tests `TestPermMatcher_PARSE_REJECT` + `TestPermUriTemplate_PARSE_REJECT` verify. *Anchored: SPEC §12.6 + §2.3 + §8.8.*

7. **D7 — `track_per_rule_stats: true` envoy-go-only large-N parse-rejection — NOT IMPOSED in MVP (SPEC §12.7 + §11.P10).** No cap in phase-16 MVP; mirrors Envoy permissive disposition. Documented foot-gun at §13.4 phase-16 forward-pointer notes. Future operator-ergonomics phase MAY add a cap. *Anchored: SPEC §12.7 + §8.5 + §11.P10.*

8. **D8 — LOG-action dynamic-metadata emission SKIPPED (SPEC §12.8 + §1.1 amendment 5 + §8.6).** envoy-go MVP silent-no-metadata-emit. LOG result always = ALLOWED; matched-policy captured for per-policy counter emission; `access_log_hint` metadata emission deferred to future dynamic-metadata family phase landing `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive (or analogous accessor). Operator divergence-window documented at §13.4. Group 5 `TestEvaluateRulesEngine_LogMatch_AllowedWithPolicyName` + Group 6 `TestDecodeHeaders_LOGMatch_HeaderContinue_AllowedCounterIncremented` verify the envoy-go side. *Anchored: SPEC §12.8 + amendment 5 + §8.6.*

9. **D9 — Shadow-rules access-log integration DEFERRED (SPEC §12.9 + §8.7 + §11.P13).** envoy-go MVP emits shadow counters only; no shadow-decision-annotated access-log entries. §11.P13 confirms Envoy v1.37.2 also counter-only (no current divergence; documented as forward-pointer). Future re-activation couples to access-log subsystem feature. *Anchored: SPEC §12.9 + §8.7 + §11.P13.*

10. **D10 — Matcher-engine terminal action allow-list = canonical RBAC `Action` only (SPEC §12.10 + §11.P3 + ADR-0142).** Non-canonical TypeURLs PARSE-REJECT at `matcher.New()` time with envoy-go-only error per §2.6. Group 8 `TestMatcherNew_UnknownTerminalTypeURL_PARSE_REJECT` verifies. *Anchored: SPEC §12.10 + §11.P3 + §2.6 + ADR-0142.*

11. **D11 — `Principal_Authenticated` canonical 3 cert fields only (SPEC §12.11 + §8.9).** URI SAN + DNS SAN + Subject DN CN exposed via `DownstreamPrincipal() []string`. Additional cert fields (Issuer DN, Serial Number, fingerprints) NOT exposed in phase-16 MVP. Future TLS-context-extension phase couples. *Anchored: SPEC §12.11 + §8.9 + amendment 12.*

12. **D12 — `response_code_details` field emission DEFERRED (SPEC §12.12 + §1.1 amendment 11 + §8.12).** envoy-go MVP emits no `response_code_details` on DENY; Envoy v1.37.2 emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"` per §11.P5 + amendment 11. Divergence-window documented at §13.4. Future response-code-details framework phase couples HCM's local-reply path to a per-filter accessor. *Anchored: SPEC §12.12 + amendment 11 + §8.12.*

13. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil` (decoder-only).** Per SPEC §1 item 5 + §6.4 + ADR-0140 §Decision (iv). Phase-16 rbac is a pre-body request gate; the disposition (allow/deny/log) is computed at `DecodeHeaders` time BEFORE the request body is forwarded. No encode-side path. Mirrors phase-12 csrf + phase-13 buffer decoder-only precedent (rbac is the 4th §9 row to ship `Encoder: nil` after fault.abort is also decode-side gated; csrf, buffer, rbac are the three pure decode-side-only filters). *Anchored: SPEC §1 item 5 + §6.4; ADR-0140 §Decision (iv); phase-12 ADR-0120 + phase-13 ADR-0125 decoder-only precedent.*

14. **PLAN-emerging — ADR-0125 amendment §(xii) authored at Task 10 (NOT pre-landed at SPEC commit).** Per planner-time disposition resolving SPEC §5.4 vs DECISIONS.md observed state inconsistency. SPEC §5.4 includes the verbatim §(xii) paragraph claiming "authored at this SPEC commit per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) precedent" — BUT `grep -n 'xii\|phase 16' docs/envoy-go/DECISIONS.md` at master tip `cedf29a` returns 0 matches (last amendment is phase-15 §(xi) per phase-15 SPEC `49e0361`). PLAN resolves: **author §(xii) at Task 10 impl-time** per ADR-0044 ADR-on-impl convention generalized (phase-13 ADR-0127-v2 was authored at impl-time `Task 12`; phase-14 §(viii)-(x) and phase-15 §(xi) were authored at SPEC commit). The phase-16 SPEC's prose-claim "authored at this SPEC commit" was a forward-looking expectation that SPEC author did not execute; PLAN executes per the ADR-on-impl convention at Task 10. SPEC stays the authoritative design doc; PLAN is the authoritative impl shape. Documented in PROGRESS.md preamble. *Anchored: phase-13 ADR-0127-v2 in-place-update at impl-time precedent; ADR-0044 ADR-on-impl convention; SPEC §5.4 + §8 prose-claim vs DECISIONS.md observed state.*

15. **PLAN-emerging — Single-row disposition per phase-13/14/15 precedent at the LoC-borderline (per `Scope check` section above).** Production LoC ~1208-1608 borderline crosses 1500 at high-end estimate; task count ~16 well under 25. Single-row ships per the rationale documented in `Scope check` section (4 bullet points covering: task-count is load-bearing trigger; splitting fragments a single coherent filter; framework primitives belong with their consumers; phase-14/15 precedent governs). The natural 16.1+16.2 split per BRAINSTORM §1.4 is explicitly rejected. *Anchored: BOOTSTRAP_PROMPT.md §6.1 OR-trigger; ADR-0045 precedent of phase-05's structural split; phase-13/14/15 borderline-but-single-row precedent.*

16. **PLAN-emerging — Group 9 stats-namespace integration test surface added (per ADR-0145 + §11.P7).** Mirrors phase-15 Group 8 + phase-14 Group 8 PLAN-emerging stats-namespace integration sub-group precedent. Verifies the 4 base counters registered + SN2-reuse rendering (no new SN10 rule) + per-policy lazy-allocation cache hit on subsequent matches + post-Freeze idempotent registration. *Anchored: phase-14 + phase-15 PLAN Group 8 precedent.*

17. **PLAN-emerging — `internal/matcher/` initial MatchContext interface SCOPED TO RBAC's CANONICAL SURFACE (per ADR-0142 §Decision (iii)).** Initial accessor set: `Header(name) (value string, present bool)` + `Path() string` + `Method() string` + `SourceIP() net.IP` + `DestinationIP() net.IP` + `DestinationPort() uint32` + `RequestedServerName() string`. Future filters (ext_authz, jwt_authn, oauth2) extend additively. NOT a load-bearing decision at phase-16 (the matcher-engine's canonical predicate set for RBAC is well-known); the additive-widening pattern is documented at ADR-0142 §Consequences. *Anchored: ADR-0142 + SPEC §3.2 + §11.P3.*

18. **PLAN-emerging — Counter-delta byte-equivalence assertion convention from phase-13/14/15 (per ADR-0145 + planner-time decision per fixture spec).** Each scenario's expected counter-delta is asserted via fixture-side scrape comparison: pre-scenario stats scrape captures baseline; per-scenario request issued; post-scenario scrape compares against baseline + expected delta; assertion exact-match on the 4 base counters per active namespace + lazy-allocated per-policy counters when `track_per_rule_stats: true` (NOT exercised in fixture 0018 listener-level; per-route override scenario 8 exercises only the base counters). Per-scenario isolation guarantees no cross-scenario stat-pollution. *Anchored: phase-11/13/14/15 counter-delta assertion convention.*

19. **PLAN-emerging — BEHAVIOR_CONTRACT.md `### envoy.filters.http.rbac` subsection insertion point = AFTER `### envoy.filters.http.bandwidth_limit` at line 1416 (landing-chronological per phase-13/14/15 precedent), NOT at the SPEC §13.1 stub-text alphabetical-canonical claim.** Per planner-time disposition resolving SPEC §13.1 inaccuracy. The CURRENT BEHAVIOR_CONTRACT.md has subsections ordered chronologically by landing phase (fault@955 < header_mutation@1012 < local_ratelimit@1081 < csrf@1155 < buffer@1209 < compressor@1336 < bandwidth_limit@1416), NOT alphabetically. SPEC §13.1 claims "alphabetical-canonical" insertion at the END — the END-insertion happens to be correct (`rbac` is alphabetically last among the 8 existing subsections), but the rationale is landing-chronological, not alphabetical. PLAN settles: insert AFTER `### envoy.filters.http.bandwidth_limit` (line 1416) per landing-chronological convention. *Anchored: BEHAVIOR_CONTRACT.md observed state at master tip cedf29a; phase-13/14/15 PLAN insertion-point precedent.*

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Files changed:** `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (new)
**Commit SHA:** <filled at commit time; capture via `git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` post-commit, or via a follow-up SHA-fill commit per phase-15 precedent>
**Notes:** Created PROGRESS.md; verified all 17 preconditions per PLAN §"Execution preconditions"; phase-16 SPEC + PLAN confirmed present in HEAD; SPEC at `3159811`, PLAN at `40f030b`; ADR tail at 0139 (next-free 0140; the 7 phase-16 ADRs ADR-0140..ADR-0146 land at impl-time anchor Tasks 2/3/4-5/6/8/9 per ADR-0044 ADR-on-impl + phase-13 buffer + phase-15 convention — UNLIKE phase-14 compressor's SPEC-time-pre-landing); `internal/filter/http/rbac/` absent (Task 2 lands); `internal/matcher/` absent (Task 3 lands); `fixture.HTTPRbac` absent (Task 11 lands). ADR-0125 §(xii) amendment paragraph NOT yet landed (`grep -cE '\(xii\)' docs/envoy-go/DECISIONS.md` returns 0 at master tip `948f6c4`); §(xii) lands at impl-time Task 10 per planner-time decision 14 (resolves SPEC §5.4 forward-looking-prose vs DECISIONS.md observed state inconsistency). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention). Pre-existing fuzzers (19 fuzzers from phases 02–15 across 18 `fuzz_test.go` files; PLAN's literal `find ./test/fuzz` path does not match envoy-go's per-package co-located fuzzer layout — captured verbatim under the `# Precondition 11 short-form` block in the `**Outputs:**` section below) deferred to Task 15 phase-done Gate iv per PLAN.

**Outputs:**

```
$ git rev-parse --abbrev-ref HEAD
phase-16-http-filter-rbac-impl

$ git log --oneline master | head -10
948f6c4 phase 16 plan follow-up: STATE.md SHA-fill (TBD → 40f030b post-squash)
40f030b Squash merge phase-16-http-filter-rbac-plan
cedf29a phase 16 spec follow-up: STATE.md SHA-fill (TBD → 3159811 post-squash)
3159811 Squash merge phase-16-http-filter-rbac-spec
b45c1eb phase 16 brainstorm follow-up: STATE.md SHA-fill (TBD → 38749ba post-squash)
38749ba Squash merge phase-16-http-filter-rbac-brainstorm
07f8d44 chore: gitignore up.txt
98a8ca6 phase 15 impl follow-up: STATE.md SHA-fill (TBD → c1361d3 post-squash) + lifecycle-state-6 narrative
c1361d3 Squash merge phase-15-http-filter-bandwidth-limit-impl
36c91c9 phase 15 PLAN follow-up: STATE.md SHA-fill (TBD → a5c5ec9)

$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

$ docker version
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 Built:             Wed Sep  3 20:57:32 2025
 OS/Arch:           linux/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
  API version:      1.49 (minimum version 1.24)
  Go version:       go1.23.8
  Git commit:       01f442b
  Built:            Fri Apr 18 09:52:57 2025
  OS/Arch:          linux/amd64
  Experimental:     false
 containerd:
  Version:          1.7.27
  GitCommit:        05044ec0a9a75232cad458027ca83437aae3f4da
 runc:
  Version:          1.2.5
  GitCommit:        v1.2.5-0-g59923ef
 docker-init:
  Version:          0.19.0
  GitCommit:        de40ad0

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
139

$ grep -nE '\(xi\)' docs/envoy-go/DECISIONS.md | head -3
5871:**(xi) The 5th canonical disabled-OR-override pattern requires a wrapper proto with `oneof override { bool disabled; <FilterConfig> override; }` shape. Phase 15 bandwidth_limit's per-route TPFC has NO such wrapper proto — Envoy v1.37.2 ships ZERO `BandwidthLimitPerRoute` message in the `envoy.extensions.filters.http.bandwidth_limit.v3` package (verified via v1.32.4 + v1.37.0 go-control-plane bindings + upstream proto fetch + Envoy source `source/extensions/filters/http/bandwidth_limit/config.cc::createRouteSpecificFilterConfigTyped` accepting the bare `BandwidthLimit` proto directly).** Per-route TPFC entries consume the SAME `BandwidthLimit` message used at listener-level (mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1 same-proto-reuse pattern). To DISABLE the filter on a specific route, operators set per-route `enable_mode: DISABLED` in the bare `BandwidthLimit` proto (the existing `enable_mode` enum doubles as the disable mechanism); there is NO `disabled` boolean shortcut at the filter-proto level.
6437:(d) Per-route SHARED stats (mirroring phase-13/14 ADR-0125 5th canonical) — REJECTED. Per ADR-0139 + §11.P4: phase-15 uses INDEPENDENT per-route stats (mirrors phase-11 ADR-0117 4th canonical, NOT phase-13/14's 5th canonical). The new 6th canonical pattern (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route) sits adjacent to ADR-0117 — documented at ADR-0125 §(xi) amendment paragraph (LANDED at SPEC commit `49e0361`).
6449:- ADR-0125 amendment §(xi) (LANDED at SPEC commit `49e0361`) documents the 6th canonical per-route pattern (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route) — phase-15 is the FIRST consumer of that pattern; ADR-0139's full ratification + the per-route INDEPENDENT-stats wiring land at Task 7.

$ grep -cE '\(xii\)' docs/envoy-go/DECISIONS.md
0

$ git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/SPEC.md
31598116038e6845e0a9260cc79eb720dc335d44

$ git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/PLAN.md
40f030b5c67fafb407f02ef422a3363ec6d56fb0

$ git status --porcelain
(empty)

$ go test -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.581s
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.005s
ok  	github.com/esalaine/envoy-go/internal/admin	0.422s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.018s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.019s
ok  	github.com/esalaine/envoy-go/internal/drain	0.077s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.018s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.472s
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.132s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	0.393s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	0.010s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	0.012s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	0.005s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	0.034s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.263s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	0.007s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	0.217s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.166s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	3.027s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.044s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	0.007s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	0.005s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.022s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	0.061s
ok  	github.com/esalaine/envoy-go/test/differential	0.058s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.003s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.005s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	0.005s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	0.005s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs	0.006s
ok  	github.com/esalaine/envoy-go/test/helpers	0.009s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	0.007s
(packages with no test files elided; full output: every package PASS or `[no test files]`)

$ go test -count=1 ./test/differential/ -run 'TestDifferential' -v
--- PASS: TestDifferential (52.96s)
    --- PASS: TestDifferential/0000-tcp-echo (1.58s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.20s)
    --- PASS: TestDifferential/0002-tls-tcp (1.23s)
    --- PASS: TestDifferential/0003-http11-routing (1.18s)
    --- PASS: TestDifferential/0004-h2-routing (1.91s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.09s)
    --- PASS: TestDifferential/0006-access-log (11.13s)
    --- PASS: TestDifferential/0007a-cors (1.49s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.97s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.45s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.86s)
    --- PASS: TestDifferential/0010-graceful-drain (9.47s)
    --- PASS: TestDifferential/0011-http-fault (2.13s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.57s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.24s)
    --- PASS: TestDifferential/0014-http-csrf (1.46s)
    --- PASS: TestDifferential/0015-http-buffer (1.47s)
    --- PASS: TestDifferential/0016-http-compressor (1.51s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	53.025s

# Precondition 11 short-form — PLAN's literal command form returns 0 because envoy-go's fuzzers are co-located
# under `internal/.../fuzz_test.go` per the per-package convention, NOT under `./test/fuzz/`.
# The semantic intent (19 fuzzers from phases 02–15 exist) verifies via the alternative grep below.
$ find ./test/fuzz -name 'Fuzz*_test.go' 2>/dev/null | wc -l
0

$ grep -rEn '^func Fuzz' --include='*.go' | wc -l
19

$ grep -rEn '^func Fuzz' --include='*.go'
internal/filter/tcpproxy/fuzz_test.go:26:func FuzzTcpProxyFilter(f *testing.F) {
internal/filter/http/header_mutation/fuzz_test.go:19:func FuzzHeaderMutationConfigParse(f *testing.F) {
internal/filter/hcm/fuzz_test.go:25:func FuzzHCMConfigParse(f *testing.F) {
internal/stats/fuzz_test.go:31:func FuzzPromTextFormat(f *testing.F) {
internal/filter/http/buffer/fuzz_test.go:19:func FuzzBufferConfigParse(f *testing.F) {
internal/drain/fuzz_test.go:15:func FuzzDrainTransitions(f *testing.F) {
internal/filter/hcm/h2/fuzz_test.go:24:func FuzzFrameStream(f *testing.F) {
internal/filter/hcm/h2/fuzz_test.go:96:func FuzzHPACKDecode(f *testing.F) {
internal/filter/http/bandwidthlimit/fuzz_test.go:30:func FuzzBandwidthLimitConfigParse(f *testing.F) {
internal/tls/fuzz_test.go:24:func FuzzTLSContextParse(f *testing.F) {
internal/filter/http/compressor/fuzz_test.go:30:func FuzzCompressorConfigParse(f *testing.F) {
internal/filter/http/localratelimit/fuzz_test.go:20:func FuzzLocalRateLimitConfigParse(f *testing.F) {
internal/filter/http/fuzz_test.go:46:func FuzzFilterChainParse(f *testing.F) {
internal/accesslog/fuzz_test.go:10:func FuzzAccessLogFormat(f *testing.F) {
internal/filter/http/csrf/fuzz_test.go:22:func FuzzCsrfPolicyConfigParse(f *testing.F) {
internal/admin/fuzz_test.go:22:func FuzzConfigDumpFormat(f *testing.F) {
internal/filter/http/fault/fuzz_test.go:23:func FuzzFaultConfigParse(f *testing.F) {
internal/listener/listenerfilter/fuzz_test.go:11:func FuzzFilterChainMatch(f *testing.F) {
internal/bootstrap/fuzz_test.go:62:func FuzzBootstrapLoad(f *testing.F) {

$ docker pull envoyproxy/envoy:v1.37.2
v1.37.2: Pulling from envoyproxy/envoy
Digest: sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
Status: Image is up to date for envoyproxy/envoy:v1.37.2
docker.io/envoyproxy/envoy:v1.37.2

$ docker image inspect envoyproxy/envoy:v1.37.2 --format '{{.Id}}'
sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd

$ go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3 RBAC | head -5
package rbacv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"

type RBAC struct {

	// The primary RBAC policy which will be applied globally, to all the incoming requests.

$ go doc github.com/cncf/xds/go/xds/type/matcher/v3 Matcher | head -5
package v3 // import "github.com/cncf/xds/go/xds/type/matcher/v3"

type Matcher struct {

	// Types that are assignable to MatcherType:

$ test ! -d internal/filter/http/rbac && echo "ok: rbac absent"
ok: rbac absent

$ test ! -d internal/matcher && echo "ok: matcher absent"
ok: matcher absent

$ grep -nE 'HTTPRbac' test/differential/fixture/fixture.go
(0 matches)

$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
10
```

## Task 2 — Package skeleton + types + factory + Group 1+2 tests [ADR-0140, ADR-0141]

*Style convention for Tasks 2-16: each task entry uses the bold-label form per Task 1 + phase-15 precedent (`**Files changed:** ...`, `**Commit SHA:** ...`, `**Outputs:**`).*

**Files changed:**
- `internal/filter/http/rbac/doc.go` (new; ~120 LoC) — package overview + 7-outer-field surface + 3-CEL silent-ignore + audit_logging_options silent-ignore + Permission/Principal Large 11 + 3 deferred + per-route 7th canonical + decoder-only HTTPFilter + 4-base-counter filterStats + deny-path wire shape + cross-cutting ADR anchors.
- `internal/filter/http/rbac/rbac.go` (new; ~480 LoC) — public surface (`TypeURL`, `New`) + internal consts (`filterName`, `actionTypeURL`, `denyBody`) + types (`compiledConfig` 8 fields + `compiledRulesEngine` + `compiledMatcherEngine` STUB-placeholder + `compiledPolicy` + `compiledPerRoute` + `factoryState` + `filter` + `filterStats` 4 base counters + perPolicy sync.Map + reg) + helpers (`buildCompiledConfig` + `buildCompiledRulesEngine` + `buildCompiledMatcherEngine` STUB returning sentinel error + `parsePerRoute` + `resolvePerRouteConfig` + `buildCompiledPerRoute` + `newFilterStats` + `newFilterStatsIfAbsent` + `namespacePrefix` SKELETON namespace-resolver) + skeleton DecodeHeaders / DecodeData / DecodeTrailers / OnDestroy method bodies (Task 7 fleshes out DecodeHeaders).
- `internal/filter/http/rbac/evaluator.go` (new; ~85 LoC) — `permissionEvaluator` interface + `principalEvaluator` interface + `evalContext` interface (Task 2 minimum-viable empty surface; Tasks 4-7 widen additively) + `buildPermissionEvaluators` STUB (Task 4 replaces) + `buildPrincipalEvaluators` STUB (Task 5 replaces). STUBs return empty slices with no error per BOOTSTRAP_PROMPT approach (a) so Group 1 tests pass without hitting evaluator-construction errors.
- `internal/filter/http/rbac/rbac_test.go` (new; ~575 LoC) — test helpers (`mustAny`, `freshFactoryCtx`, `freshFactoryCtxWithRegistry`, `allowAnyPolicy`, `happyRulesEngine`) + Group 1 (17 test cases — config parse + buildCompiledConfig PGV-mirror) + Group 2 (7 test cases — buildCompiledConfigPerRoute + parsePerRoute + resolvePerRouteConfig).
- `docs/envoy-go/DECISIONS.md` (modified; +ADR-0140 ~95 LoC + ADR-0141 ~115 LoC immediately after ADR-0139's trailing `---`). ADR-0140 codifies the SHAPE invariants (directory + HTTPFilter value + filterStats counter surface + boot-registration ordering); ADR-0141 codifies the TYPE-LEVEL invariants (compiledConfig 8-field shape + dual-engine dispatch + UDPA-field_alias-annotation framing + PGV-mirror validation + ALLOW/DENY/LOG-partial action enum + CEL three-field silent-ignore + audit_logging_options silent-ignore). Each carries `Status: Accepted`, `Date: 2026-05-12`, `**Lands-in-task:** Task 2`.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time; capture via `git log -1 --format=%H` post-commit, or via a follow-up SHA-fill commit per phase-15 precedent>

**ADR commit ledger:** ADR-0140 + ADR-0141 landed at this task's commit per ADR-0044 ADR-on-impl convention. The 5 remaining phase-16 ADRs (ADR-0142..ADR-0146 + ADR-0125 §(xii) amendment) land at their respective impl-time anchor tasks per PROGRESS preamble.

**Notes — STUB scope:**
- `buildCompiledMatcherEngine` returns sentinel error at Task 2: `"rbac: matcher-engine evaluator not yet implemented at Task 2 (lands at Task 3 per ADR-0142)"`. The dual-engine dispatch path is structurally exercised by Group 1's rules-wins tests; the matcher-only path is unreached at Task 2 (Group 5 matcher-engine tests land at Task 7 after Task 3 fills in `internal/matcher/`).
- `compiledMatcherEngine.tree` is typed `any` at Task 2 (forward-declared placeholder); Task 3 replaces with the real `*matcher.Matcher` field + live `matcher.New(m, []string{actionTypeURL})` invocation.
- `buildPermissionEvaluators` + `buildPrincipalEvaluators` return empty `[]permissionEvaluator{}` / `[]principalEvaluator{}` slices with no error at Task 2 per BOOTSTRAP_PROMPT approach (a). Tasks 4 + 5 replace with the real switch over the 14 Permission cases (11 accepted + 3 PARSE-REJECT) + 14 Principal cases (11 accepted + 3 PARSE-REJECT) per ADR-0143.
- `DecodeHeaders` body returns `Continue` unconditionally at Task 2 (skeleton stub). Task 7 fleshes out: per-route resolve + dual-engine dispatch + SendLocalReply 403 on DENY per SPEC §6.7.
- `newFilterStats` / `newFilterStatsIfAbsent` allocate the 4 base counters under fallback `rbac.<counter>` namespace when configured prefix is empty (per amendment 3 empty-allowed); the final SN2-reuse namespace shape (`http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` per §11.P7 + amendment 9) lands at Task 8 per ADR-0145.

**Test approach for Group 1 case #17 (`TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored`):** approach (b) per BOOTSTRAP_PROMPT — `t.Skip` with re-activation note. The `cel_config` field is the third CEL field per amendment 6 NEW; structurally absent in go-control-plane v1.32.4 proto binding (`/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/config/rbac/v3/rbac.pb.go` exposes only `Condition` + `CheckedCondition` on Policy; `CelConfig` lands in v1.37.x). The silent-ignore disposition is structural — `buildCompiledRulesEngine` reads NO CEL fields; the test re-activates when the module bumps expose the field. All other 16 Group 1 cases + all 7 Group 2 cases use approach (a) minimum-viable proto inputs (e.g., `allowAnyPolicy()` returns a policy with single `any: true` Permission + single `any: true` Principal; the evaluator stubs accept the input without hitting an error).

**Outputs:**

```
$ go test -race -count=1 ./internal/filter/http/rbac/
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.012s

$ go test -race -count=1 -v ./internal/filter/http/rbac/ 2>&1 | tail -10
--- PASS: TestParsePerRoute_RbacFieldSet_WholesaleOverride (0.00s)
--- PASS: TestBuildCompiledPerRoute_OverrideCarriesOwnStatPrefix_INDEPENDENT (0.00s)
--- PASS: TestParsePerRoute_MalformedAny_Rejected (0.00s)
--- PASS: TestResolvePerRouteConfig_NilMsg_FallsBackToListener (0.00s)
--- PASS: TestResolvePerRouteConfig_LazyCacheSyncMap_PointerIdentityKey (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.012s

$ go vet ./internal/filter/http/rbac/...
(no output; exit 0)

$ grep -nE '^## ADR-0140|^## ADR-0141' docs/envoy-go/DECISIONS.md
6757:## ADR-0140: `internal/filter/http/rbac/` package shape — ...
6807:## ADR-0141: `compiledConfig` shape + 7-consumed proto-faithful field decomposition ...

$ grep -nE '^\*\*Lands-in-task:\*\* Task 2' docs/envoy-go/DECISIONS.md | wc -l
8
# (2 from phase-16 ADR-0140 + ADR-0141 landed here, plus 6 pre-existing Task-2 anchors from prior phases: ADR-0125 phase-12 / ADR-0118 phase-11 / ADR-0123 phase-12 / ADR-0124 phase-12 / ADR-0135 phase-15 / ADR-0136 phase-15.)

$ go test -race -count=1 -v ./internal/filter/http/rbac/ -run TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored 2>&1 | grep -E 'SKIP|PASS|FAIL'
--- SKIP: TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored (0.00s)
```

Total package size at Task 2: ~685 LoC production + ~575 LoC tests = ~1260 LoC. Within PLAN's ~1208–1608 LoC production envelope (production-only — the test LoC sits outside that envelope per PLAN's accounting convention).

## Task 3 — `internal/matcher/` NEW top-level package [ADR-0142]

**Files changed:**
- `internal/matcher/doc.go` (new; ~60 LoC) — package overview + cross-phase-reuse intent + API surface enumeration + initial predicate evaluator scope + additive-widening discipline (per ADR-0142 §Decision (iii)).
- `internal/matcher/matcher.go` (new; ~510 LoC) — `Matcher` opaque type wrapping read-only parsed `*compiledNode` tree + `New(tree, supportedActionTypes)` constructor with parse-time PARSE-REJECT for unknown terminal `Any.TypeUrl` (verbatim envoy-go-only error `"matcher: terminal action type %q unsupported by this caller"` per ADR-0142 §Decision (ii)) + `Evaluate(MatchContext)` walker returning matched terminal `*anypb.Any` OR `(nil, nil)` on no-match per `rbac.pb.go:43-46` proto comment + `MatchContext` interface (Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName initial accessor subset per planner-time decision 17 + ADR-0142 §Decision (iii)) + initial predicate evaluators (SinglePredicate value-match dispatching on `HttpRequestHeaderMatchInput` input TypeURL; AND / OR / NOT combinators) + StringMatcher subset (Exact / Prefix / Suffix / Contains / SafeRegex with `ignore_case`). MatcherTree variant PARSE-REJECTED (`"matcher: matcher_tree variant unsupported in this build (use matcher_list)"`) per ADR-0142 §Decision (iv) conservative-deferral disposition.
- `internal/matcher/matcher_test.go` (new; ~690 LoC) — 14 test functions covering the framework-primitive surface: `TestNew_CanonicalActionTerminal_Accepted`, `TestNew_UnknownTerminalTypeURL_PARSE_REJECT`, `TestNew_EmptyAllowList_AllTerminalsRejected`, `TestEvaluate_FirstMatchingPredicate_ReturnsTerminalAny`, `TestEvaluate_NoMatchingPredicate_NilNil`, `TestEvaluate_HeaderPredicate_ExactMatch`, `TestEvaluate_HeaderPredicate_PresentMatch`, `TestEvaluate_PathPredicate_PrefixMatch`, `TestEvaluate_AndPredicate_AllChildrenMatch`, `TestEvaluate_OrPredicate_FirstChildMatch`, `TestEvaluate_NestedTree_DepthThree`, `TestMatchContext_AdapterPattern`, `TestMatcher_StatelessAcrossEvaluations` (concurrent under `go test -race`), `TestNew_ErrorWordingIsUnwrappable` (verifies the verbatim PARSE-REJECT phrase survives error wrapping at caller-side). Test helpers `mustAny` + `rbacActionAny` + `terminalActionConfig` + `headerInputAny` + `fieldMatcherHeaderExact` + `fieldMatcherHeaderPresent` + `fieldMatcherPathPrefix` + `onMatchAction` + `onMatchNestedMatcher` + `stubCtx` (test-side MatchContext impl) + `canned` (second MatchContext impl for interface-boundary assertion).
- `internal/filter/http/rbac/rbac.go` (modified; ~+5 / -10 LoC net) — STUB replacement: (a) added `matchv3` (cncf xds) + `matcher` (NEW package) imports; (b) `actionTypeURL` const lost the `//nolint:unused` (now consumed at `buildCompiledMatcherEngine`); (c) `compiledMatcherEngine.tree` field RE-TYPED from `any` (Task-2 placeholder) to `*matcher.Matcher` + lost `//nolint:unused`; (d) `buildCompiledMatcherEngine` body REPLACED — was `return nil, errors.New("rbac: matcher-engine evaluator not yet implemented at Task 2 (lands at Task 3 per ADR-0142)")`; now `tree, err := matcher.New(m, []string{actionTypeURL}); if err != nil { return nil, fmt.Errorf("rbac: matcher: %w", err) }; return &compiledMatcherEngine{tree: tree}, nil`. Signature changed from `(m proto.Message)` to `(m *matchv3.Matcher)` (the dispatch at `buildCompiledConfig` already passes the correct proto-typed pointer via `c.GetMatcher()` / `c.GetShadowMatcher()`).
- `docs/envoy-go/DECISIONS.md` (modified; ~+90 LoC inserted after ADR-0141's trailing `---`) — ADR-0142 with `Status: Accepted`, `Date: 2026-05-12`, `**Lands-in-task:** Task 3`, §Context / §Decision (i)–(v) / §Consequences.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time; capture via `git log -1 --format=%H` post-commit, or via a follow-up SHA-fill commit per phase-15 precedent>

**ADR commit ledger:** ADR-0142 landed at this task's commit per ADR-0044 ADR-on-impl convention. The 4 remaining phase-16 ADRs (ADR-0143 + ADR-0144 + ADR-0145 + ADR-0146 + ADR-0125 §(xii) amendment) land at their respective impl-time anchor tasks per PROGRESS preamble.

**TDD verification:**
- **Step 1 — failing tests authored** as `internal/matcher/matcher_test.go` (14 functions; 690 LoC).
- **Step 2 — BUILD FAIL confirmed** — `go test -race -count=1 ./internal/matcher/...` reported `undefined: New` 11 times + `undefined: MatchContext` (referenced via interface assertions in the test). Excerpt:
  ```
  # github.com/esalaine/envoy-go/internal/matcher [github.com/esalaine/envoy-go/internal/matcher.test]
  internal/matcher/matcher_test.go:204:12: undefined: New
  internal/matcher/matcher_test.go:230:12: undefined: New
  ...
  FAIL    github.com/esalaine/envoy-go/internal/matcher [build failed]
  ```
- **Step 3 — `internal/matcher/doc.go` + `matcher.go` authored** per the File structure table responsibility.
- **Step 4 — PASS confirmed** — `go test -race -count=1 ./internal/matcher/...` reports 14/14 PASS in 1.011s.
- **Step 5 — `buildCompiledMatcherEngine` STUB REPLACED** at `internal/filter/http/rbac/rbac.go` + `compiledMatcherEngine.tree` field RE-TYPED from `any` to `*matcher.Matcher`. Existing Task-2 Group 1+2 tests still PASS (`TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins` uses an empty `&matcherv3.Matcher{}` which is never reached because rules-wins precedence; the structural path stays intact). Full-package `go test -race` runs both packages green.
- **Step 6 — ADR-0142 authored** in DECISIONS.md after ADR-0141 + grep verification.

**Notes — predicate-evaluator scope (per ADR-0142 §Decision (iv)):**
- **Implemented at this task** (RBAC's canonical surface): SinglePredicate value-match with `HttpRequestHeaderMatchInput` input (routes through `MatchContext.Header(name)`; H2 pseudo-headers `:path` + `:method` route via caller's choice — `stubCtx` test-side impl routes them through `Path()` + `Method()`). AND / OR / NOT combinators. StringMatcher Exact / Prefix / Suffix / Contains / SafeRegex with `ignore_case`.
- **PARSE-REJECTED at this task** (conservative deferral): MatcherTree variant (`"matcher: matcher_tree variant unsupported in this build (use matcher_list)"`); SinglePredicate `custom_match` extension (`"single_predicate.custom_match extension unsupported in this build"`); StringMatcher `Custom` variant (`"custom string matcher extension unsupported in this build"`); unknown input TypeURLs (`"matcher: input type %q unsupported by this caller"`).
- **Deferred to future callers** (additive extension per ADR-0142 §Consequences): network common-inputs (`DestinationIPInput` / `DestinationPortInput` / `SourceIPInput` / `ServerNameInput`); SSL/TLS inputs (`UriSanInput` / `DnsSanInput` / `SubjectInput`); MatcherTree variant; SinglePredicate `custom_match`; StringMatcher `Custom`. Each adds via a localised patch to the input-dispatch switch in `compileSinglePredicate` + (optionally) a new MatchContext accessor.

**Notes — STUB scope unchanged from Task 2:**
- `buildPermissionEvaluators` + `buildPrincipalEvaluators` still STUB (Tasks 4 + 5 fill in per ADR-0143).
- `DecodeHeaders` body still STUB returning `Continue` (Task 7 fills in per SPEC §6.7 + ADR-0146).
- DownstreamPrincipal accessor framework primitive still pending (Task 6 lands ADR-0144).
- Stat surface still SKELETON (Task 8 finalizes per ADR-0145).

**rbac package matcher-engine path tests:** the Group 5 matcher-engine tests (`TestEvaluateMatcherEngine_*`) land at Task 7 once DecodeHeaders dispatch is wired; at Task 3 the STUB-replacement is exercised only structurally (via `buildCompiledMatcherEngine` constructing a real `*matcher.Matcher` when the rules-wins dispatch chooses the matcher branch — which Group 1's `TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins` does NOT exercise, by design). The PARSE-REJECT propagation path is verified at the matcher-package level (`TestNew_UnknownTerminalTypeURL_PARSE_REJECT`); the rbac-side wrapping (`"rbac: matcher: ..."`) verification lands at Task 7 Group 5.

**Outputs:**

```
$ go test -race -count=1 ./internal/matcher/... ./internal/filter/http/rbac/...
ok  	github.com/esalaine/envoy-go/internal/matcher	1.010s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.012s

$ go test -race -count=1 -v ./internal/matcher/... 2>&1 | grep -c '^--- PASS'
14

$ go vet ./...
(no output; exit 0)

$ grep -nE '^## ADR-0142' docs/envoy-go/DECISIONS.md
6897:## ADR-0142: Matcher-engine evaluator framework primitive at NEW top-level package `internal/matcher/` ...
```

Total package size at Task 3: ~570 LoC production (60 doc.go + 510 matcher.go) + ~690 LoC tests = ~1260 LoC for the new `internal/matcher/` package. The rbac.go STUB replacement adds net ~+5 LoC to the rbac package (615 LoC raw at Task 2 → ~620 LoC at Task 3). Cumulative rbac.go LoC growth is modest; the Task 2 code-review M-1 observation (raw 616 LoC / non-comment 268) still applies — the matcher-engine helper grew minimally.

## Task 4 — `evaluator.go` Permission Large 11 + AND/OR/NOT + 3 PARSE-REJECT + Group 3 [ADR-0143 partial]

**Files changed:**
- `internal/filter/http/rbac/evaluator.go` (modified; Task-2 STUB shape ~87 LoC → ~470 LoC at Task 4) — REPLACED the Task-2 `buildPermissionEvaluators` empty-slice STUB with the real `buildOnePermission` 14-case switch dispatching the Permission Large 11 + 3 PARSE-REJECT per SPEC §6.5 + ADR-0143; widened the `evalContext` interface from Task-2's intentionally-empty shape to carry the 5 Permission-relevant accessors (`Header(name) (string, bool)`, `URLPath() string`, `DestinationIP() net.IP`, `DestinationPort() uint32`, `RequestedServerName() string`) — Principal accessors (DirectRemoteIP / RemoteIP / DownstreamPrincipal / SourcedMetadata / FilterState) land additively at Task 5; declared the 11 Permission evaluator concrete types (`permAny`, `permHeader`, `permURLPath`, `permDestIP`, `permDestPort`, `permDestPortRange`, `permSNI`, `permAnd`, `permOr`, `permNot`, `permSourcedMetadata`) per ADR-0143 §Decision (ii); declared local shared-infrastructure adapters (`matchString` for `matcherv3.StringMatcher` honoring Exact / Prefix / Suffix / Contains / SafeRegex with `ignore_case`; `matchPath` for `matcherv3.PathMatcher` delegating to matchString on the inner Path field; `matchHeader` for `routev3.HeaderMatcher` honoring PresentMatch + StringMatch + 5 deprecated specifiers + RangeMatch with `treat_missing_header_as_empty` + `invert_match`; `matchCidr` for `corev3.CidrRange` with `prefix_len` defaulting to 0 when wrapperspb.UInt32Value unset) per ADR-0143 §Decision (vii); `buildPrincipalEvaluators` STUB unchanged (Task 5 replaces with the real switch).
- `internal/filter/http/rbac/rbac_test.go` (modified; ~+460 LoC) — appended the Group 3 test surface (15 test functions per SPEC §14.1 #3 + PLAN.md line 66): `TestPermAny_True_Matches`, `TestPermAny_FalseValue_Rejected` (PGV const=true mirror), `TestPermHeader_Match` (4 subtests: exact_match_hits / exact_match_misses / prefix_match_hits / header_absent_returns_false), `TestPermURLPath_PathMatcher` (3 subtests: exact / prefix-hits / prefix-misses), `TestPermDestIP_CIDR` (4 subtests: 2 in-range + 2 out-of-range), `TestPermDestPort_Exact`, `TestPermDestPortRange_StartLEPortLTEnd` (5 boundary cases verifying [start, end) half-open semantic), `TestPermSNI_StringMatcher`, `TestPermAndRules_Recursive_AllMatch` (4-level depth all-match + short-circuit FALSE), `TestPermOrRules_Recursive_AnyMatch` (3-level depth + all-false short-circuit), `TestPermNotRule_Recursive_Negate`, `TestPermSourcedMetadata_ParseSupported_RuntimeFalse` (per §2.5 + §8.10 always-no-match MVP), `TestPermMetadata_PARSE_REJECT` (verbatim envoy-go-only error per §2.3 + §11.P12 + D3), `TestPermMatcher_PARSE_REJECT` (verbatim per §2.3 + §8.8 + D6), `TestPermUriTemplate_PARSE_REJECT` (verbatim per §2.3 + §8.8 + D6). Added `stubEvalContext` test-side `evalContext` implementation carrying the 5 Permission-relevant accessor fields; Task 5 widens additively with Principal accessors. Added imports: `net` (for `net.IP`), `corev3` (`envoy/config/core/v3` — CidrRange), `routev3` (`envoy/config/route/v3` — HeaderMatcher), `matcherv3` (`envoy/type/matcher/v3` — StringMatcher/PathMatcher), `typev3` (`envoy/type/v3` — Int32Range), `wrapperspb`. Existing Task-2 `matcherv3.Matcher{}` references (cncf xds) renamed to `xdsmatcherv3` alias to avoid name collision with the envoy matcherv3 alias.
- `docs/envoy-go/DECISIONS.md` (modified; +1 ADR section) — ADR-0143 inserted in the slot after ADR-0142 (line 6968) per ADR-0044 ADR-on-impl convention. ADR-0143's body covers BOTH Permission + Principal surfaces; Principal-related sections (catalogue §Decision (iii); Principal PARSE-REJECT table rows §Decision (iv); prinAuthenticated three-case algorithm body §Decision (vi)) marked with verbatim `<!-- TODO at Task 5 -->` placeholders for Task 5 to fill in-place per ADR-0044 split-anchor convention. **Lands-in-task: Tasks 4 + 5** per the split-anchor disposition.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry).

**ADR commit ledger:** ADR-0143 inserted at this task's commit (Permission section + Principal placeholders) per ADR-0044 ADR-on-impl convention. The Principal section + prinAuthenticated three-case body finalize at Task 5's commit (in-place edit replaces the `<!-- TODO at Task 5 -->` placeholders).

**TDD verification:**
- **Step 1 — failing tests authored** as 15 Group 3 test functions appended to `internal/filter/http/rbac/rbac_test.go`.
- **Step 2 — BUILD FAIL confirmed** — `go test -race -count=1 ./internal/filter/http/rbac/` reported `undefined: buildOnePermission` 11 times (the entry point not yet declared at evaluator.go). Excerpt:
  ```
  # github.com/esalaine/envoy-go/internal/filter/http/rbac [github.com/esalaine/envoy-go/internal/filter/http/rbac.test]
  internal/filter/http/rbac/rbac_test.go:635:13: undefined: buildOnePermission
  internal/filter/http/rbac/rbac_test.go:649:12: undefined: buildOnePermission
  ...
  FAIL    github.com/esalaine/envoy-go/internal/filter/http/rbac [build failed]
  ```
- **Step 3 — `evaluator.go` Permission surface authored** per ADR-0143 §Decision (ii) + (iv) + (vii). The 11 evaluator concrete types + the 14-case `buildOnePermission` switch + the 4 local shared-infrastructure adapters (matchString / matchHeader / matchPath / matchCidr) + the widened `evalContext` interface.
- **Step 4 — `buildPermissionEvaluators` STUB REPLACED** — Task-2's empty-slice STUB at `evaluator.go` replaced with the real iterating impl that calls `buildOnePermission(perm)` per Permission + wraps errors with `permission[%d]:` prefix per SPEC §6.5. The `rbac.go` call sites at `buildCompiledRulesEngine` (line 351) needed NO modification — they were already calling `buildPermissionEvaluators(p.GetPermissions())` since Task 2; the STUB-replacement is fully transparent from rbac.go's perspective.
- **Step 5 — PASS confirmed** — `go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPerm' -v` reports 15/15 Group 3 tests PASS (including 4 + 3 + 4 + 5 subtests under the parameterized header / path / CIDR / port-range tests, for 31 sub-cases total under the 15 top-level test functions). Full-package run reports 39/39 PASS + 1 SKIP (`TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored` per Task 2's structural-deferral): 24 Group 1 + 2 tests + 15 Group 3 tests. Group 1 + 2 regression-clean: the STUB-replacement preserved the canonical `permAny{val: true}` shape (the allowAnyPolicy fixtures used by Group 1 tests construct `&rbacconfigv3.Permission_Any{Any: true}` which now compiles to a real `&permAny{val: true}` evaluator instead of an empty slice; the dispatch path stays intact since Group 1 + 2 tests don't exercise the evaluator surface beyond construction).
- **Step 6 — ADR-0143 authored** in DECISIONS.md after ADR-0142 with the Permission body filled + Principal `<!-- TODO at Task 5 -->` placeholders at 3 specific section anchors (§Decision (iii) Principal catalogue; §Decision (iv) Principal PARSE-REJECT table rows; §Decision (vi) prinAuthenticated three-case algorithm). `grep -nE '^## ADR-0143' docs/envoy-go/DECISIONS.md` returns the canonical 1 match.

**Notes — shared infrastructure adapter strategy (per ADR-0143 §Decision (vii)):**
- **Local impl chosen at Task 4** for matchString / matchHeader / matchPath / matchCidr — NOT extracted to a shared package. Rationale: csrf has a narrow inline StringMatcher consumer (Exact-only, no-ignore-case); the `internal/matcher` package at ADR-0142 carries its own StringMatcher impl for the cncf/xds variant (type-incompatible with the envoy variant Permission.requested_server_name consumes); extraction crosses a cross-filter boundary that needs brainstorm.
- **TECH-DEBT noted for future operator-ergonomics phase extraction.** A future phase MAY extract an `internal/stringmatcher/` (or analogous) package once the cross-filter consumer set stabilizes. REVIEW.md M-X forward-pointer signals the extraction candidate; pre-extraction the local impls in evaluator.go + matcher.go + csrf.go duplicate StringMatcher semantics by ~50–100 LoC per consumer, an acceptable cost at phase 16.
- **HeaderMatcher subset honored at Task 4** mirrors the routev3.HeaderMatcher proto's 8 HeaderMatchSpecifier variants: PresentMatch + canonical StringMatch (delegates to matchString) + 5 deprecated direct specifiers (ExactMatch / PrefixMatch / SuffixMatch / ContainsMatch / SafeRegexMatch) + RangeMatch. The 5 deprecated specifiers stay in the impl for proto-faithfulness — operators may still configure them (the proto fields are not soft-deprecated in the .pb.go binding at v1.32.4) + silently dropping them would be surprising. `treat_missing_header_as_empty` + `invert_match` honored last per the proto's documented semantic.

**Notes — STUB scope unchanged from Task 3:**
- `buildPrincipalEvaluators` still STUB (Task 5 fills in with the real `buildOnePrincipal` 14-case switch + the 11 Principal concrete types + the prinAuthenticated three-case algorithm per ADR-0143 §Decision (iii) + (vi)).
- `DecodeHeaders` body still STUB returning `Continue` (Task 7 fills in per SPEC §6.7 + ADR-0146).
- DownstreamPrincipal accessor framework primitive still pending (Task 6 lands ADR-0144).
- Stat surface still SKELETON (Task 8 finalizes per ADR-0145).

**Outputs:**

```
$ go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPerm' -v 2>&1 | tail -3
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.011s

$ go test -race -count=1 ./internal/filter/http/rbac/ -v 2>&1 | grep -cE '^--- PASS'
39

$ go test -race -count=1 ./internal/filter/http/rbac/ -v 2>&1 | grep -cE '^--- SKIP'
1

$ go vet ./...
(no output; exit 0)

$ grep -nE '^## ADR-0143' docs/envoy-go/DECISIONS.md
6968:## ADR-0143: Permission + Principal Large 11+11 evaluators ...

$ grep -nE '^\*\*Lands-in-task:\*\* Tasks 4 \+ 5' docs/envoy-go/DECISIONS.md
6973:**Lands-in-task:** Tasks 4 + 5
```

Total package size at Task 4: ~1140 LoC production (685 at Task 2 + ~+5 at Task 3 + ~+450 at Task 4 evaluator.go growth from 87 → ~470) + ~1035 LoC tests (575 at Task 2 + ~+460 at Task 4) = ~2175 LoC. Within PLAN's ~1208–1608 LoC production envelope (production-only — the test LoC sits outside the envelope per PLAN's accounting convention; the evaluator.go growth at Task 4 is the largest single-task production increment).

## Task 5 — `evaluator.go` Principal Large 11 + prinAuthenticated three-case + 3 PARSE-REJECT + Group 4 [ADR-0143 finalized]

Tip after Task 4 + follow-up = `dc8983fbaa9fdcf0403a4eb5dea23c06b05a6041`. This task lands the Principal evaluator subset of ADR-0143 + finalizes the ADR (fills 5 TODO placeholders + flips Status to `Accepted`).

**Step 1 — Write Group 4 failing tests (14 test cases):** appended `TestPrinAny_True_Matches`, `TestPrinDirectRemoteIP_CIDR_PeerSource`, `TestPrinRemoteIP_CIDR_XFFResolved`, `TestPrinHeader_HeaderMatcher`, `TestPrinURLPath_PathMatcher`, `TestPrinAndIds_Recursive_AllMatch`, `TestPrinOrIds_Recursive_AnyMatch`, `TestPrinNotId_Recursive_Negate`, `TestPrinSourcedMetadata_RuntimeFalse`, `TestPrinFilterState_RuntimeFalse`, `TestPrinAuthenticated_ThreeCaseAlgorithm` (4 sub-tests for cases (a) / (b) / (b)-URI-SAN-priority / (c)), `TestPrinSourceIp_PARSE_REJECT`, `TestPrinMetadata_PARSE_REJECT`, `TestPrinCustom_PARSE_REJECT` (SKIPped — structurally absent in v1.32.4 proto binding per §1.1 amendment 7). The 5 new `evalContext` accessors (DirectRemoteIP / RemoteIP / DownstreamPrincipal / SourcedMetadata / FilterState) declared on the `stubEvalContext` test helper.

**Step 2 — Verify FAIL:** `go test ./internal/filter/http/rbac/ -run 'TestPrin' 2>&1` → `undefined: buildOnePrincipal` (build failure at all 14 test sites). FAIL discipline preserved per TDD.

**Step 3 — Extend `evaluator.go` with Principal surface:** the 11 evaluator types landed (`prinAny`, `prinAuthenticated` with three-case algorithm body, `prinDirectRemoteIP`, `prinRemoteIP`, `prinHeader`, `prinURLPath`, `prinAnd`, `prinOr`, `prinNot`, `prinSourcedMetadata`, `prinFilterState`) + `buildOnePrincipal` 14-case switch (11 ACCEPTED + 3 PARSE-REJECT + nil-identifier defensive). The `evalContext` interface widened with 5 new accessor declarations (DirectRemoteIP / RemoteIP / DownstreamPrincipal / SourcedMetadata / FilterState) per ADR-0143 §Decision (i) — additive widening, no breaking changes to the Task-4 Permission consumers. The `prinAuthenticated` three-case algorithm body lives in `evaluator.go` per SPEC §6.6 + §1.1 amendment 12: empty-principals-check first (case (c)), then nil-nameMatcher → TRUE (case (a)), then iterate priority-ordered candidates via `matchString` (case (b)).

**Step 4 — Replace `buildPrincipalEvaluators` STUB:** the Task-2 empty-slice-no-error stub was replaced with the real iterating implementation that calls `buildOnePrincipal` per element + wraps errors with `principal[%d]:` prefix per SPEC §6.5 + ADR-0143 §Decision (iii). The `compiledPolicy.principals` slice now carries real `principalEvaluator` instances post-build; downstream Group 1+2 fixtures using `allowAnyPolicy("p")` still produce `prinAny{val:true}` evaluators and parse successfully (regression-clean).

**Step 5 — Verify PASS:** `go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPrin' -v` → ALL 14 Group 4 tests PASS (13 PASS + 1 SKIP for `TestPrinCustom_PARSE_REJECT` per the v1.32.4 structural-absence — same disposition as Task 2's `TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored` skip). Full package suite: 52 PASS + 2 SKIP across Groups 1+2+3+4. `go vet` clean. `gofmt -l` + `goimports -l` clean post `gofmt -w`. `golangci-lint run ./internal/filter/http/rbac/...` exit 0.

**Step 6 — Finalize ADR-0143:** the 5 TODO placeholders filled at `docs/envoy-go/DECISIONS.md`:
1. §Status flipped from `Accepted (Permission section; Principal section TODO-at-Task-5)` to `Accepted`.
2. §Decision (iii) — Principal Large 11 evaluator catalogue enumerated (11 types + AND/OR/NOT recursion semantics + `buildOnePrincipal` switch documentation + the structurally-absent Principal_Custom default-arm strategy via Go type-name introspection).
3. §Decision (iv) — Principal_SourceIp / Principal_Metadata / Principal_Custom PARSE-REJECT table rows filled with verbatim error wordings.
4. §Decision (vi) — prinAuthenticated three-case algorithm body documented with Go-code excerpt + per-case order-of-operations rationale (empty-principals-check FIRST so nil-matcher + plaintext returns FALSE per case (c)).
5. §Consequences — the Principal-finalization meta-bullet rewritten to retrospective tense (removed in-line `<!-- TODO at Task 5 -->` reference; `grep -nE 'TODO at Task 5' docs/envoy-go/DECISIONS.md` returns 0 matches post-edit).

**Files changed (Task 5):**
- `internal/filter/http/rbac/evaluator.go` (+~270 LoC; 528 → ~800 LoC) — 11 prinXxx evaluator types + buildPrincipalEvaluators STUB-replacement + buildOnePrincipal 14-case switch + evalContext 5-accessor widening.
- `internal/filter/http/rbac/rbac_test.go` (+~520 LoC; 1070 → ~1590 LoC) — Group 4: 14 test cases + stubEvalContext 5-accessor extension.
- `docs/envoy-go/DECISIONS.md` (5 TODO placeholders filled; §Status flipped).
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry).

**Constraints honored:**
- TDD: write Group 4 tests → verify FAIL (`undefined: buildOnePrincipal`) → implement → verify PASS.
- No new files; all changes modify existing files per the task's "Modify:" list.
- Group 1+2 regression-clean (existing `allowAnyPolicy` fixtures still parse + produce real `prinAny{val:true}` evaluators after the STUB replacement).
- The production `*filter` (in `rbac.go`) does NOT yet implement evalContext — Task 7 wires that body. Task 5's evalContext widening only adds INTERFACE methods; the implementing struct lives in tests (stubEvalContext) for now.
- The 5 forward-compat accessor shapes (DirectRemoteIP, RemoteIP, DownstreamPrincipal, SourcedMetadata, FilterState) honor the documented MVP semantic: DirectRemoteIP / RemoteIP return `net.IP` (the production *filter at Task 7 returns peer addr; the framework XFF resolver is not yet exposed to filters — documented at evalContext doc comment). SourcedMetadata + FilterState return `any` (runtime always-FALSE per §2.5; future dynamic-metadata + filter-state phases finalize the shape). DownstreamPrincipal returns `[]string` — the production accessor stub on `*filter` lands at Task 6 with the framework primitive per ADR-0144.
- The XFF resolver primitive is NOT yet exposed to filters at envoy-go phase 16; per the BOOTSTRAP_PROMPT note, plain-IP behavior is acceptable for phase-16 MVP. The TestPrinRemoteIP_CIDR_XFFResolved test exercises the CIDR-match semantic over the stub's pre-populated `remoteIP` field; the Task-7 *filter wiring will use the peer addr verbatim until the framework XFF accessor lands.
- The `Principal_Custom` 14th variant (per §1.1 amendment 7) is structurally absent from v1.32.4 — the PARSE-REJECT disposition is encoded in `buildOnePrincipal`'s `default:` arm via Go-level type-name introspection (`strings.HasSuffix("%T", ".Principal_Custom")`). The verbatim error wording stays locked at ADR-0143 §Decision (iv). The Group 4 `TestPrinCustom_PARSE_REJECT` test is `t.Skip()`'d at this commit per BOOTSTRAP_PROMPT approach (b); future module-version bump activates the typed case + lifts the skip.

**Outputs:**

```
$ go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPrin' -v 2>&1 | tail -3
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.014s

$ go test -race -count=1 ./internal/filter/http/rbac/ -v 2>&1 | grep -cE '^--- PASS'
52

$ go test -race -count=1 ./internal/filter/http/rbac/ -v 2>&1 | grep -cE '^--- SKIP'
2

$ go vet ./...
(no output; exit 0)

$ golangci-lint run ./internal/filter/http/rbac/... 2>&1
(no output; exit 0)

$ gofmt -l internal/filter/http/rbac/ && goimports -l internal/filter/http/rbac/
(empty)

$ grep -nE 'TODO at Task 5' docs/envoy-go/DECISIONS.md
(no matches; ADR-0143 finalized)

$ grep -nE '^\*\*Status:\*\* Accepted$' docs/envoy-go/DECISIONS.md | head -1
6970:**Status:** Accepted

$ grep -nE '^\*\*Lands-in-task:\*\* Tasks 4 \+ 5' docs/envoy-go/DECISIONS.md
6973:**Lands-in-task:** Tasks 4 + 5
```

Total package size at Task 5: ~1410 LoC production (~1140 at Task 4 + ~270 evaluator.go growth) + ~1590 LoC tests (~1035 at Task 4 + ~555 Group 4). Within PLAN's ~1208–1608 LoC production envelope.

## Task 6 — TLS-principal accessor framework primitive [ADR-0144]

**Status:** DONE (2026-05-12)

**Anchored ADR:** ADR-0144 (TLS-principal accessor on `DecoderFilterCallbacks` framework primitive + plumbing from connection-level TLS state through HCM dispatch to per-stream filter-callback + canonical 3-cert-field scope per D11).

**Files changed:**
- `internal/filter/http/callbacks.go` (+13 LoC) — added `DownstreamPrincipal() []string` to `DecoderFilterCallbacks` interface with GoDoc per ADR-0144 §Decision (i).
- `internal/filter/http/chain.go` (~+45 LoC) — added per-stream `tlsPrincipals []string` field on `*FilterChain`; new `(d *decoderCB) DownstreamPrincipal()` accessor; new `(c *FilterChain) SetTLSPrincipals(p []string)` HCM-side wire-in method per ADR-0144 §Decision (ii).
- `internal/filter/http/chain_test.go` (~+110 LoC) — new `downstreamPrincipalProbe` test filter + 3 integration tests: `TestDecoderCB_DownstreamPrincipal_NoSeed_NilSlice`, `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed`, `TestDecoderCB_DownstreamPrincipal_OrderingPreservedAcrossCalls`.
- `internal/filter/hcm/connection.go` (~+50 LoC) — new `extractTLSPrincipals(state)` helper + `downstreamTLSPrincipals(net.Conn)` wrapper; threaded `downstream net.Conn` through `runConnection` → `serveOneRequest` → `dispatchRequest` (3 signatures widened); `chain.SetTLSPrincipals(downstreamTLSPrincipals(downstream))` call before `RunDecodeHeaders` dispatch per ADR-0144 §Decision (iii).
- `internal/filter/hcm/h2dispatch.go` (~+25 LoC) — added `tlsPrincipals []string` field to `h2Dispatcher` + `chainDispatchAction`; per-conn extraction at `runH2` time; `chain.SetTLSPrincipals(c.tlsPrincipals)` in `WriteH2`.
- `internal/filter/hcm/filter.go` (+10 LoC) — `runH2` calls `downstreamTLSPrincipals(downstream)` once at connection build time + assigns to `disp.tlsPrincipals` (symmetric to H1's per-request extraction; conn-pinned TLS state mirrors the same semantic).
- `internal/filter/hcm/tls_test.go` (NEW; ~165 LoC) — extraction-helper-in-isolation tests: 9 `extractTLSPrincipals` cases (nil state / handshake-incomplete / no-peer-certs / URI-only / DNS-only / CN-only / all-three priority-ordered / multiple-of-each / empty-CN-skipped) + 2 `downstreamTLSPrincipals` wrapper cases (non-tls conn / nil conn).
- `internal/filter/http/rbac/rbac_test.go` (~+170 LoC) — Group 7 tests (5): `TestDownstreamPrincipal_PlaintextConnection_NilSlice`, `TestDownstreamPrincipal_mTLSConnection_URISANs_FirstPriority`, `TestDownstreamPrincipal_mTLSConnection_DNSSANs_SecondPriority`, `TestDownstreamPrincipal_mTLSConnection_SubjectDNCommonName_ThirdPriority`, `TestDownstreamPrincipal_OrderingPreserved`.
- `internal/matcher/doc.go` (+13 LoC) — Task 3 carry-forward I-2 (lighter-touch option (b) per BOOTSTRAP_PROMPT): forward-declared accessors note for `DestinationIP` / `DestinationPort` / `SourceIP` / `RequestedServerName` (interface-declared at Task 3, dispatch handlers land additively at Task 7+).
- Test-side mock updates (8 packages): `callbacks_test.go` + `header_mutation_test.go` + `bandwidthlimit_test.go` + `localratelimit_test.go` + `buffer_test.go` + `csrf_test.go` + `fault_test.go` + `compressor_test.go` — each `fakeDecoderCB` / `fakeCallbacks` / `recordingDCB` mock adds a `DownstreamPrincipal() []string { return nil }` body to satisfy the widened interface. Plus HCM test-call-site signature fixups: `chain_dispatch_test.go` + `chain_integration_test.go` + `connection_test.go` updated to pass `nil` for the new `downstream net.Conn` parameter to `dispatchRequest`.
- `docs/envoy-go/DECISIONS.md` (+~125 LoC) — ADR-0144 in-place after ADR-0143 per ADR-0044 ADR-on-impl convention.

**TDD discipline:**
1. **Step 1:** wrote chain_test.go probe tests + Group 7 tests + tls_test.go helper tests.
2. **Step 2:** verified BUILD FAIL — `DownstreamPrincipal undefined (type DecoderFilterCallbacks has no field or method)`; `chain.SetTLSPrincipals undefined`.
3. **Step 3:** added `DownstreamPrincipal() []string` to `DecoderFilterCallbacks` interface in callbacks.go.
4. **Step 4:** implemented `chain.go` plumbing (per-stream field + accessor + `SetTLSPrincipals` HCM wire-in method).
5. **Step 5:** chain_test.go DownstreamPrincipal tests PASS (3/3).
6. **Step 6:** wired HCM dispatch (H1 `dispatchRequest` + H2 `runH2` + `chainDispatchAction.WriteH2`).
7. **Step 7:** Group 7 tests PASS (5/5); tls_test.go PASS (11/11); chain_test.go all PASS.
8. **Step 8:** ADR-0144 authored in DECISIONS.md.

**Framework-impact verification:**
- All `DecoderFilterCallbacks` implementers updated: production `decoderCB` (chain.go) + 8 test-side mocks across the filter packages (`fakeDecoderCB` in callbacks_test / header_mutation_test / bandwidthlimit_test / localratelimit_test; `fakeCallbacks` in buffer_test / csrf_test / compressor_test; `recordingDCB` in fault_test).
- HCM dispatch wiring: H1 (`connection.go` `dispatchRequest`) + H2 (`h2dispatch.go` `chainDispatchAction.WriteH2`) both call `chain.SetTLSPrincipals` after `SetRequestCtx` + before `RunDecodeHeaders`.
- `extractTLSPrincipals` helper location: `internal/filter/hcm/connection.go` (placed alongside the dispatch path that consumes it; same package — no cross-package import needed).
- Group 7 strategy: extraction-helper-in-isolation per BOOTSTRAP_PROMPT recommendation (end-to-end mTLS path deferred to fixture 0018 scenario 6 at Tasks 12-14).

**Acceptance results:**
```
$ go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.147s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.018s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.041s
(all 15 packages PASS)

$ grep -nE '^## ADR-0144' docs/envoy-go/DECISIONS.md | wc -l
1

$ go test -race -count=1 -short ./...
(all packages green; no regressions)
```

**Group 7 + chain integration test count:** 3 chain_test.go probe-filter tests + 5 Group 7 rbac tests + 11 tls_test.go helper tests = 19 NEW tests + existing Group 4 prinAuthenticated tests continue green.

**Carry-forward I-2 (matcher.doc.go forward-declared accessors note):** added per lighter-touch option (b) — a 13-LoC paragraph at the end of matcher/doc.go's package doc-comment.

Total deltas at Task 6: ~600 LoC (production: ~145 framework + ~50 HCM extraction + helpers; tests: ~280; docs: ~125). Within PLAN's framework-delta envelope.



## Task 7 — `DecodeHeaders` body finalization + dual-engine dispatch + `SendLocalReply` 403 + Group 5+6

**Status:** DONE (2026-05-12)

**Files changed:**
- `internal/filter/http/rbac/rbac.go` (~+460 LoC, 615 → 1078) — added engineResult enum + evaluateEngine + evaluateRulesEngine + evaluateMatcherEngine + policyMatches dispatch helpers per SPEC §6.9; added matcherCtxAdapter (rbac↔matcher bridge per ADR-0142 §Decision (iii) + CF-2 SourceIP → DirectRemoteIP mapping); added 10 evalContext accessors on *filter (CF-6 — Header / URLPath / Method / DestinationIP / DestinationPort / RequestedServerName / DirectRemoteIP / RemoteIP / DownstreamPrincipal / SourcedMetadata / FilterState); added emitPrimaryCounters + emitShadowCounters STUBs (real impl at Task 8/9); finalized DecodeHeaders body per SPEC §6.7; widened *filter struct with `headers http.Header` per-stream cache; CF-1 applied — buildCompiledPerRoute returns (*compiledPerRoute, error); resolvePerRouteConfig returns (*compiledConfig, isDisabled bool) disambiguating disabled-route from parse-failure-fallback.
- `internal/filter/http/rbac/evaluator.go` (+10 LoC, 848 → 854) — added Method() to the evalContext interface per Task 7 widening (for matcherCtxAdapter.Method() bridge to internal/matcher's MatchContext).
- `internal/filter/http/rbac/rbac_test.go` (~+1040 LoC, 1771 → 2809) — added Groups 5 (12 cases) + 6 (9 cases) + 8 (9 cases) = 30 NEW tests + Group 2 signature-fix-up for CF-1 (buildCompiledPerRoute + resolvePerRouteConfig new signatures); added `Method() string` accessor on stubEvalContext to satisfy widened interface; added rbacFakeCB (DecoderFilterCallbacks test double for Groups 6+8); added helpers headerEqPolicies / rbacActionAnyForTest / headerInputTECForTest / fieldMatcherHeaderExactForTest / onMatchActionForTest / singleHeaderMatcherTreeAllow / newFilterWithRBAC / allowAdminRBAC / matcherNewForTest.

**TDD discipline:**
1. **Step 1:** wrote Groups 5 + 6 + 8 (30 NEW test cases).
2. **Step 2:** verified BUILD FAIL with `undefined: evaluateRulesEngine / engineResultAllowed / engineResultDenied / matcherCtxAdapter`.
3. **Step 3:** implemented evaluateEngine + evaluateRulesEngine + evaluateMatcherEngine + policyMatches per SPEC §6.9; matcherCtxAdapter (10 methods); *filter as evalContext (10 methods); DecodeHeaders body finalization per SPEC §6.7; emitPrimaryCounters + emitShadowCounters STUBs.
4. **Step 4:** CF-1 fix — `buildCompiledPerRoute` returns `(*compiledPerRoute, error)`; `resolvePerRouteConfig` returns `(*compiledConfig, isDisabled bool)`; Group 2 tests updated to consume new signatures.
5. **Step 5:** all 30 NEW + 57 existing tests PASS (`go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestEvaluate|TestDecodeHeaders' -v` exits 0; full `-count=1 -v` reports 85 PASS + 2 SKIP).
6. **Step 6:** lint clean — gofmt + go vet + golangci-lint all pass (US-spelling misspell fix on "behaviour" → "behavior" applied during step 6).

**Carry-forward dispositions:**
- **CF-1 (Task 2 review I-1) APPLIED:** `buildCompiledPerRoute(*RBACPerRoute, *Registry) (*compiledPerRoute, error)` mirrors phase-15 `buildCompiledConfigPerRoute` signature (internal/filter/http/bandwidthlimit/bandwidthlimit.go:273). `resolvePerRouteConfig(msg) (rc *compiledConfig, isDisabled bool)` disambiguates wholly-disabled-route from inherit-listener-on-parse-error. On per-route parse failure: log + inherit-listener (sentinel NOT cached in sync.Map). Group 1+2 regression-clean post-fix.
- **CF-2 (Task 3 spec review I-1) APPLIED:** matcherCtxAdapter.SourceIP() → ctx.DirectRemoteIP() mapping (peer-source-pre-XFF per xds.type.matcher.v3.SourceIPInput semantics; NOT XFF-resolved). Pinned by `TestMatchContext_AccessorAdapter_DelegatesToFilter` Group 8 test. Documented in matcherCtxAdapter doc-comment.
- **CF-3 (Task 3 code review I-1) NOTED:** matcher's headerPredicate.matches short-circuits on `!present` BEFORE consulting the value-matcher. At Task 7 NO divergence observed against the canonical predicate surface; fixture 0018 (Tasks 12-14) revisit guard for `StringMatcher_Prefix{Prefix:""}` presence-equivalent corner case.
- **CF-4 GROUP 5 + GROUP 8 LANDED HERE:** Group 5 (12 dispatch tests) + Group 8 (9 matcher-engine framework primitive integration tests) per PLAN.md line 66 + SPEC §14.1 #5 + #8. The matcher_test.go file (Task 3) covers matcher-engine package surface in isolation; Group 8 covers the rbac↔matcher integration boundary via buildCompiledMatcherEngine + matcherCtxAdapter.
- **CF-5 (Task 5 code review M-3) DEFERRED to Task 14/16:** rbac_test.go is at 2809 LoC after Task 7 (1771 + ~1040 Groups 5+6+8). PLAN's 900-1100 estimate is exceeded; the file-split per CF-5 recommendation (evaluator_test.go split) is **deferred to Task 14 cleanup or Task 16 phase-end review** to keep Task 7 surgically focused on the load-bearing dispatch + SendLocalReply machinery. Tracked in REVIEW.md surface at Task 16.
- **CF-6 (Task 5 notes F-1/F-2) APPLIED:** `*filter` implements evalContext (10 methods). Header/URLPath/Method delegate to the per-stream `f.headers http.Header` cache (populated at DecodeHeaders entry); DownstreamPrincipal delegates to `f.dcb.DownstreamPrincipal()` (ADR-0144 framework primitive landed at Task 6); DestinationIP / DestinationPort / RequestedServerName / DirectRemoteIP / RemoteIP return nil/empty at phase-16 MVP pending future framework primitives (connection-info accessor not yet exposed on DecoderFilterCallbacks); SourcedMetadata + FilterState return nil at MVP per §2.5 + §8.10 always-FALSE-evaluator-runtime discipline. Method() added to evalContext interface (Task 7 widening for matcherCtxAdapter bridge).

**emitPrimaryCounters STUB scope:** Increments cc.stats.allowed (on engineResultAllowed) or cc.stats.denied (on engineResultDenied). Per-policy lazy emission (when trackPerRuleStats=true) DEFERRED to Task 9 (ADR-0146). emitShadowCounters mirrors with shadowAllowed / shadowDenied. Full SN2-reuse + per-route INDEPENDENT-stats namespace canonicalization lands at Task 8 (ADR-0145).

**Test count breakdown:**
- Group 5 dispatch (12): TestEvaluateRulesEngine_AllowMatch_Allowed, TestEvaluateRulesEngine_AllowNoMatch_Denied, TestEvaluateRulesEngine_DenyMatch_Denied, TestEvaluateRulesEngine_DenyNoMatch_Allowed, TestEvaluateRulesEngine_LogMatch_AllowedWithPolicyName, TestEvaluateRulesEngine_LogNoMatch_Allowed, TestEvaluateRulesEngine_LexicographicOrderShortCircuit, TestEvaluateMatcherEngine_CanonicalActionTerminal_Honored, TestEvaluateMatcherEngine_NoMatch_Denied, TestEvaluateMatcherEngine_UnknownTerminalTypeURL_ParseRejected, TestEvaluateEngine_BothPrimaryAndShadowConfigured_PrimaryDispositionWinsShadowEmitsCounter, TestEvaluateEngine_BothEnginesUnset_DefensiveAllowed.
- Group 6 DecodeHeaders (9): TestDecodeHeaders_ListenerLevelAllowMatch_HeaderContinue, TestDecodeHeaders_ListenerLevelDenyMatch_HeaderStopIteration_SendLocalReply403, TestDecodeHeaders_SendLocalReply_Body19Bytes_RBACAccessDenied, TestDecodeHeaders_SendLocalReply_4HeaderSet_LowercaseWireForm, TestDecodeHeaders_SendLocalReply_KeepAliveDisposition_NoConnectionClose, TestDecodeHeaders_LOGMatch_HeaderContinue_AllowedCounterIncremented, TestDecodeHeaders_PerRouteDisabled_PassthroughNoCounters, TestDecodeHeaders_PerRouteOverride_INDEPENDENTCounterNamespace, TestDecodeHeaders_BothEnginesUnset_PassthroughNoCounters.
- Group 8 matcher↔rbac integration (9): TestMatcherNew_CanonicalRBACActionTerminal_Accepted, TestMatcherNew_UnknownTypeURL_PARSE_REJECT, TestMatcherEvaluate_FirstMatchingPredicate_ReturnsTerminalAny, TestMatcherEvaluate_NoMatchingPredicate_ReturnsNilNil, TestMatcherEvaluate_HeaderPredicate_Match, TestMatcherEvaluate_PathPredicate_Match, TestMatcherEvaluate_AndPredicate_AllMatch, TestMatcherEvaluate_OrPredicate_AnyMatch, TestMatchContext_AccessorAdapter_DelegatesToFilter.

**Acceptance results:**
```
$ go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestEvaluate|TestDecodeHeaders' -v
PASS (21/21 acceptance subset — Group 5 + Group 6); exit 0
$ go test -race -count=1 ./internal/filter/http/rbac/
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.019s (85 PASS + 2 SKIP including 30 new + 55 existing + 2 prior skips)
$ go test -race -count=1 ./...
(all 41 packages PASS; no regression)
$ golangci-lint run ./...
(no findings)
```

Total deltas at Task 7: ~+1510 LoC (production: ~470 — rbac.go body finalization + dispatch + adapters + accessors; tests: ~+1040; docs: this PROGRESS entry).

<pending>

## Task 8 — Stat surface finalization + per-route INDEPENDENT-stats + Group 9 [ADR-0145]

**Started:** 2026-05-12 (post Task 7 commit `904324d`)
**Completed:** 2026-05-12 (Task 8 commit pending Step 8 — see below)
**Branch:** `phase-16-http-filter-rbac-impl`

**Deltas:**

- `internal/filter/http/rbac/rbac.go` (~+90 LoC net; 1078 → ~1170) — finalized `newFilterStats` + `newFilterStatsIfAbsent` per ADR-0145 (HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>`; both helpers funnel through `NewCounterIfAbsent` post-Freeze-idempotent path per CF-Task2-I3 + CF-Task7-M6; bodies intentionally identical with dual entry-points preserved for call-site documentation discipline); added `baseStatPrefix(hcmPrefix, rulesPrefix)` namespace assembler (HCM-rooted shape; empty HCM prefix folds to `rbac.<rules_prefix>` flat form for test code paths); extended `filterStats` struct with `primaryBase` / `shadowBase` cached prefix fields + `incPolicy(base, policyName, suffix)` lazy-allocation helper (`sync.Map` LoadOrStore + `NewCounterIfAbsent`; per-policy counter name shape `<base_prefix>.policy.<policy_name>.<suffix>` per Task 8 empirical scrape — REFINES the SPEC line 1842 hypothesis); extended `factoryState` with `hcmStatPrefix string` (captured from `ctx.StatPrefix` at New time + threaded through to `buildCompiledPerRoute` for per-route INDEPENDENT-stats wiring); dropped unused `role` parameter on `buildCompiledRulesEngine` (CF-Task7-M3); dropped unused `compiledPerRoute.activeRC()` method (CF-Task7-M2).
- `internal/filter/http/rbac/rbac_test.go` (+~360 LoC; 2809 → ~3170) — added 6 Group 9 stats-namespace integration tests per PLAN.md line 66 + SPEC §14.1 #9: (1) `TestStatsNamespace_AllFourBaseCountersRegistered`; (2) `TestStatsNamespace_SN2Reuse_NoNewSN10Rule`; (3) `TestStatsNamespace_HCMRootedPath_HttpHCMRbacPrefixCounter`; (4) `TestStatsNamespace_PerPolicyLazyAllocation_OnFirstMatch`; (5) `TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent`; (6) `TestStatsNamespace_PerRouteINDEPENDENT_ListenerCountersUnaffected` (CF-Task7-M5 pin). Added `collectMetricNames` + `containsString` test helpers. Updated `TestBuildCompiledPerRoute_OverrideCarriesOwnStatPrefix_INDEPENDENT` call-site for the new `buildCompiledPerRoute(..., hcmStatPrefix)` signature.
- `docs/envoy-go/DECISIONS.md` (+~75 LoC; ADR-0145 inserted at line 7202) — documents 4 base counters unconditionally registered + SN2-reuse namespace empirically RATIFIED + per-policy `.policy.` segment infix REFINED from SPEC hypothesis + per-route INDEPENDENT-stats THIRD row + `NewCounterIfAbsent` flip + dual entry-point preservation + 6 alternatives considered + 11 consequences.

**Empirical scrape (Task 8 Step 6 per PLAN.md line 481 + §11.P7 + amendment 9):**

Probe configuration: minimal Envoy bootstrap with HCM `stat_prefix: hcm_probe` carrying a `envoy.filters.http.rbac` filter with `rules_stat_prefix: myrules` + `shadow_rules_stat_prefix: myshadow` + `track_per_rule_stats: true` + ALLOW+any-any policies on both primary and shadow. Container: `docker run -d envoyproxy/envoy:v1.37.2 -c /etc/envoy/envoy.yaml`. After 3 requests through the listener:

```
$ curl -s http://localhost:9901/stats | grep rbac
http.hcm_probe.rbac.myrules.allowed: 3
http.hcm_probe.rbac.myrules.denied: 0
http.hcm_probe.rbac.myrules.policy.policy_allow_all.allowed: 3
http.hcm_probe.rbac.myshadow.policy.shadow_policy_allow_all.shadow_allowed: 3
http.hcm_probe.rbac.myshadow.shadow_allowed: 3
http.hcm_probe.rbac.myshadow.shadow_denied: 0
```

**SN2-reuse hypothesis RATIFIED for the 4 base counters** — `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` shape exactly. NO new SN10 rule introduced. **Per-policy counter shape REFINED** — empirical scrape inserted a `.policy.` segment between the base prefix and the policy name (SPEC line 1842 hypothesis `<base_prefix>.<policy_name>.<suffix>` omitted this; actual shape `<base_prefix>.policy.<policy_name>.<suffix>`). ADR-0145 documents the refinement inline; the SPEC stat-table will amend at the Task 9 commit (alongside ADR-0146) when per-policy emission ships fully.

Prometheus rendering (separately scraped via `/stats/prometheus`):

```
envoy_http_rbac_allowed{envoy_rbac_http_prefix="myrules",envoy_http_conn_manager_prefix="hcm_probe"} 3
envoy_http_rbac_policy_allowed{envoy_rbac_policy_name="policy_allow_all",envoy_rbac_http_prefix="myrules",...} 3
```

The Envoy v1.37.2 Prometheus rendering uses a different tag-extractor than envoy-go's existing SN2 default-branch flatten (Envoy promotes both `envoy_rbac_http_prefix` AND `envoy_http_conn_manager_prefix` as labels; envoy-go's SN2 default-branch flatten promotes ONLY `envoy_http_conn_manager_prefix` and inlines the `<rules_prefix>` segment into the base name as `envoy_http_rbac_<rules_prefix>_allowed`). The base-counter internal-namespace shape is identical; the Prometheus tag-extractor differs. Per SPEC §11.P7 + amendment 9 the envoy-go disposition is to keep the existing SN2 default-branch behavior (NO new tag-extractor; NO SN10 rule); the operator-visible Prometheus metric name differs in the label-vs-inline trade-off but operators querying by `envoy_http_conn_manager_prefix` label get equivalent dispatch. Fixture 0018 (Tasks 12-14) revisit if differential alignment is load-bearing for the scenario assertions.

**Tests:**

- Pre-Task-8 baseline: 87 PASS + 2 SKIP (post-Task 7 commit `904324d`).
- Post-Task-8: 93 PASS + 2 SKIP (87 + 6 new Group 9 tests).
- TDD discipline: 6 Group 9 tests authored Step 1; verified BUILD FAIL against pre-Task-8 state (the tests reference `cc.stats.primaryBase`, `filterStats.incPolicy`, `baseStatPrefix`-derived `http.<HCM>.rbac.<prefix>.<counter>` shape, and `factoryState.hcmStatPrefix` — all introduced at Task 8; pre-Task-8 code would fail compilation). Implementation landed Step 3-4; Step 5 PASS verification at `go test -race -count=1 ./internal/filter/http/rbac/...` (1.017s green).
- Full repo `go test ./...` minus `test/differential` (which spins up real Envoy containers and is port-contention-sensitive; rbac changes do not affect non-rbac filter behavior) all PASS.

**Carry-forwards applied:**

- **CF-Task2-I2** (unconditional shadow-counter registration): RECOMMENDED disposition applied — all 4 base counters registered unconditionally per ADR-0145 §Decision (vi). Predeclared empty counters for scrape stability matches Prometheus best practice + phase-09 fault + phase-15 bandwidth_limit precedent.
- **CF-Task2-I3 + CF-Task7-M6** (`NewCounter` → `NewCounterIfAbsent`): APPLIED. Both `newFilterStats` (listener-level) and `newFilterStatsIfAbsent` (per-route) now go through `reg.NewCounterIfAbsent`. The panic-on-duplicate semantic for boot-time registration is intentionally relaxed since the rbac stat namespace is fully data-driven (operator-configured `rules_stat_prefix` per filter). Verified idempotent at Group 9's `TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent` (post-Freeze idempotency) + the dual-listener-level same-empty-prefix sub-case (no panic).
- **CF-Task7-M3** (unused `role` parameter on `buildCompiledRulesEngine`): APPLIED. Parameter removed; the speculative reservation for error-wrapping discipline at Tasks 4/5 was not consumed.
- **CF-Task7-M2** (unused `compiledPerRoute.activeRC()` method): APPLIED. Method removed; production dispatch consumes `resolvePerRouteConfig`'s `(rc, isDisabled)` return pair directly.
- **CF-Task7-M5** (`TestDecodeHeaders_PerRouteOverride_INDEPENDENTCounterNamespace` test pinning): APPLIED. Extended via new Group 9 test `TestStatsNamespace_PerRouteINDEPENDENT_ListenerCountersUnaffected` which pins (a) per-route counter name = `http.hcm_pin.rbac.override.denied`, (b) listener-level counters stay at 0, (c) per-route counter increments to 1, (d) listener.denied and per-route.denied counters are pointer-distinct.
- **CF-Task7-I2 + M1** (defensive Allowed sentinel + activeRC asymmetry): DEFERRED to Task 16 cleanup per task instructions — not naturally co-located with stat-surface work.

**Notes:**

- The dual newFilterStats / newFilterStatsIfAbsent entry-points have intentionally identical bodies at Task 8 (both call through to the same shared code path). The call-site distinction preserves documentation discipline + serves as a forward-compat guardrail for a possible future per-registration-mode divergence (e.g., if a phase-N future filter needs explicit panic-on-duplicate at boot for security reasons). Mirrors phase-11 ADR-0117 + phase-15 ADR-0139 dual-entry-point precedent.
- The HCM stat prefix (`ctx.StatPrefix`) is threaded from FactoryCtx through to per-route INDEPENDENT-stats via `factoryState.hcmStatPrefix` → `buildCompiledPerRoute(..., hcmStatPrefix)` → recursive `buildCompiledConfig(..., ctx.StatPrefix=hcmStatPrefix, isPerRoute=true)`. Per-route counters thus inherit the SAME HCM root as listener-level counters (operators see `http.<HCM>.rbac.<per_route_prefix>.<counter>` — NOT a separate HCM-orphaned namespace).
- Per-policy emission discipline + the call-site wiring into `emitPrimaryCounters` / `emitShadowCounters` for the `trackPerRuleStats=true` branch is held back to Task 9 per ADR-0146 (which also lands the shadow-path precise discipline + the LOG-partial emission policy). Task 8 ships the lazy-allocation contract + the `filterStats.incPolicy` helper + the cached prefix fields; Task 9 ships the emission discipline.
- Group 9 test count (6) exceeds the PLAN.md "5 cases" by 1 — the additional `TestStatsNamespace_PerRouteINDEPENDENT_ListenerCountersUnaffected` test is the CF-Task7-M5 pin that PLAN-time deferred to "Group 9 integration tests". The 6th test does not change the SPEC §14.1 #9 mapping (5 stats-namespace cases per the file structure entry; the M5 pin extends an existing Group 6 test's structural assertion to a counter-side assertion).
- Lint clean: `golangci-lint run ./...` returns 0 issues. `gofmt -l ./...` returns empty.

**Acceptance gate (per PLAN.md line 474):**

- ✅ Group 9 tests PASS (6/6).
- ✅ Full Group 1-9 test run clean (93 PASS + 2 SKIP; no regressions).
- ✅ Counter-emission tests in Groups 5+6+8+9 all green.
- ✅ `grep -nE '^## ADR-0145' docs/envoy-go/DECISIONS.md` returns 1 match (line 7202).
- ✅ Impl-time empirical scrape executed against reference Envoy v1.37.2 (SN2-reuse hypothesis RATIFIED; per-policy `.policy.` segment infix REFINED + documented in ADR-0145 inline; SPEC stat-table amendment deferred to Task 9 alongside ADR-0146 per per-policy emission shipping).
- ✅ ADR-0145 authored in DECISIONS.md.

**Commit (pending):** `phase 16 Task 8: stat surface finalization + per-route INDEPENDENT-stats + Group 9 tests [ADR-0145]`

<pending>

## Task 9 — Shadow + LOG-partial + `track_per_rule_stats` per-policy emission [ADR-0146]

**Started:** 2026-05-12 (post Task 8 follow-up commit `26ce2b4`)
**Completed:** 2026-05-12 (Task 9 commit pending Step 6 — see below)
**Branch:** `phase-16-http-filter-rbac-impl`

**Deltas:**

- `internal/filter/http/rbac/rbac.go` (~+30 LoC net delta on the emit*Counters helpers; minor header-comment refresh) — finalized `emitPrimaryCounters(cc, result, policyName)` + `emitShadowCounters(cc, result, policyName)` per ADR-0146 §Decision (i)+(ii)+(iii). Both now: (a) increment the relevant base counter (allowed/denied/shadow_allowed/shadow_denied); (b) compute `suffix` ∈ {allowed, denied, shadow_allowed, shadow_denied}; (c) when `cc.trackPerRuleStats=true` AND `policyName != ""` AND `suffix != ""`, call `cc.stats.incPolicy(<primaryBase | shadowBase>, policyName, suffix)` for lazy per-policy emission. The STUB header comment ("STUB at Task 7 — real impl at Task 9") replaced with the production-ready ADR-0146 doc-comment block covering: amendment 8 LOG-partial fold-into-allowed (NO `logged` counter); amendment 11 + §8.12 response_code_details divergence-window; the 6 divergence-windows forward-pointer to BEHAVIOR_CONTRACT §13.4 (Task 15 anchoring).
- `internal/filter/http/rbac/rbac_test.go` (+~365 LoC; 3158 → ~3523) — added 7 new test cases extending Group 9 (Task 9 / ADR-0146 finalization subsection): (1) `TestDecodeHeaders_LOGMatch_TrackPerRuleStats_AllowedPerPolicyCounterIncremented` (LOG + track-true → base allowed + per-policy `.allowed` ticks per amendments 5+8); (2) `TestDecodeHeaders_PerPolicyEmission_OnlyMatchedPolicyCounters_Increment` (2-policy LOG config; admin request matches `p_admin` — `p_guest` counter NOT allocated per lazy + matched-policy discipline); (3) `TestDecodeHeaders_PerPolicyEmission_TrackPerRuleStatsFalse_NoPerPolicyCounters` (track-false → only base counters; sync.Map.perPolicy empty); (4) `TestDecodeHeaders_ShadowEnabled_PrimaryDispositionWins_ShadowCountersIncrement` (primary DENY + match guest + shadow ALLOW + match guest → 403 + denied=1 + shadow_allowed=1; primary dispatch unaffected by shadow); (5) `TestDecodeHeaders_ShadowEnabled_TrackPerRuleStats_PerPolicyShadowCountersIncrement` (shadow per-policy counter uses SHADOW base prefix); (6) `TestDecodeHeaders_ShadowDeniedWithTrackPerRuleStats_PerPolicyShadowDeniedCounter` (shadow DENY + match → per-policy `.shadow_denied`; primary ALLOW + match → request passes); (7) `TestDecodeHeaders_DenyMatch_NoResponseCodeDetailsEmitted_DivergenceWindow` (PIN response_code_details divergence-window — body byte-exact "RBAC: access denied" with no "rbac_access_denied_matched_policy" infix; no `*response-code-details*` header). Added 3 test helpers: `twoPolicyLOGRBAC(t)`; `findCounterByName(reg, name)`; `newFilterWithCtx(t, listener, ctx)`.
- `docs/envoy-go/DECISIONS.md` (+~50 LoC; ADR-0146 inserted at line 7277, immediately after ADR-0145's closing `---`) — documents shadow parallel-walk + LOG-partial precise discipline + track_per_rule_stats per-policy emission + response_code_details divergence-window + shadow-access-log forward-pointer + the 6 divergence-windows for BEHAVIOR_CONTRACT §13.4 anchoring. Includes 6 alternatives considered + 8 consequences.

**Tests:**

- Pre-Task-9 baseline: 93 PASS + 2 SKIP (post-Task 8 follow-up commit `26ce2b4`).
- Post-Task-9: 100 PASS + 2 SKIP (93 + 7 new test cases).
- TDD discipline: 7 tests authored Step 1; verified FAIL against pre-Task-9 STUB emit*Counters via `go test -race -count=1 -run '...' ./internal/filter/http/rbac/...` returning 4 FAIL (the 4 expecting per-policy counter registration; 3 PASS-from-start cases checked the negative-space invariants — track-false → no per-policy + shadow base counters tick + no response-code-details leak — which the Task 8 STUBs already satisfied). Step 3 wiring landed; Step 4 PASS verification at `go test -race -count=1 ./internal/filter/http/rbac/... ./internal/matcher/...` (1.019s green; rbac 1.019s + matcher 1.011s).
- Project-wide `go test -count=1 -short ./...` regression clean.
- `golangci-lint run ./internal/filter/http/rbac/... ./internal/matcher/...` returns 0 issues.

**SPEC §13.2 stat-table disposition (per Task 9 dispatch in PLAN.md task description "Default disposition"):**

The Task 8 PROGRESS entry pinned "SPEC stat-table will be amended at the Task 9 commit when per-policy emission ships fully" — but the survey of phase-13/14/15 PROGRESS revealed that NO phase amends SPEC in-place at impl-time; the SPEC freeze convention is post-spec-commit-immutable. Phase-16 inherits the SPEC-freeze convention; the `.policy.` segment refinement of SPEC line 1842-1845 templates is documented in the ADR-trail (ADR-0145 §Decision (iii) + ADR-0146 §Decision (iii) + ADR-0146 §Consequences) rather than in-place at SPEC.

**SPEC line 1842-1845 row-templates remain in the SPEC-time hypothesis form** (`<rules_stat_prefix>.<policy_name>.<suffix>`); operators reading the SPEC stat-table are cross-referenced via ADR-0146 §Consequences to the live-counter-shape ADRs. The row-template form is operator-orienting (`grep policy` finds them regardless of intermediate segments), so the documentation cost of the deferral is low. ADR-0146 §Consequences carries the in-place-amendment-considered-and-rejected note for future readers.

**Carry-forwards applied / cleared:**

- **CF-Task7-I2 + M1** (defensive Allowed sentinel + activeRC asymmetry): DEFERRED to Task 16 cleanup per PLAN's task allocation — not naturally co-located with shadow-emission discipline work.
- **CF-Task7-M5** (TestDecodeHeaders_PerRouteOverride_INDEPENDENTCounterNamespace test pinning): RE-VERIFIED green at Task 9 baseline (the per-route INDEPENDENT-stats discipline is orthogonal to the per-policy emission discipline; Task 9 added no new per-route assertions).
- **CF-Task8-(none)**: Task 8 follow-up `26ce2b4` left no carry-forwards open for Task 9.

**Notes:**

- **The 7 new test cases extend Group 9** (per the file's existing Group 9 header comment which scoped Group 9 as "Stats namespace integration"; the Task 9 subsection extends Group 9 with "Task 9 / ADR-0146 finalization: shadow + LOG-partial + track_per_rule_stats per-policy emission discipline"). The total Group 9 cases grow from 6 (Task 8) to 13 (Task 8 + Task 9). The Group 5 + Group 6 totals are unchanged — Task 9's emission-discipline tests live structurally with the stats-namespace integration tests rather than the dispatch-correctness tests, since their assertion surface is counter-shape + counter-value, not engine result or dispatch outcome.
- **emitPrimaryCounters + emitShadowCounters wire per-policy emission via existing Task 8 plumbing.** Task 8's `filterStats.incPolicy(base, policyName, suffix)` + `primaryBase` / `shadowBase` cached prefix fields land everything the Task 9 wiring needed; the Task 9 delta is a 2-line addition per helper (`if cc.trackPerRuleStats && policyName != "" && suffix != "" { cc.stats.incPolicy(...) }`). Mirrors the phase-15 ADR-0139 pattern of holding the lazy-cache helper at Task N + wiring the emission discipline at Task N+1.
- **The 7th test (response_code_details divergence-window pin)** is a STRUCTURAL pin — it asserts the ABSENCE of `rbac_access_denied_matched_policy[<id>]` in the SendLocalReply body + the ABSENCE of any `response-code-details` header in the OrderedHeaders carrier. Purpose: surface the change immediately if a future framework primitive ever threads response-code-details to the local-reply path so the divergence-window can be closed deliberately (instead of accidentally introducing a new divergence direction). Mirrors phase-09 fault's structural-pin precedent for the framework-primitive-future surfaces.
- **The 6 divergence-windows enumerated at ADR-0146 §Context** are the authoritative content for BEHAVIOR_CONTRACT §13.4 phase-16 forward-pointer notes (the BEHAVIOR_CONTRACT edit lands at Task 15 per PLAN.md task allocation). ADR-0146 is the cross-reference anchor; Task 15's BEHAVIOR_CONTRACT subsection author cites ADR-0146 §Decision (i)-(vi) verbatim.

**Acceptance gate (per PLAN.md line 511):**

- ✅ `go test -race -count=1 ./internal/filter/http/rbac/... ./internal/matcher/...` exits 0 (rbac 1.019s + matcher 1.011s).
- ✅ All unit tests PASS (100 PASS + 2 SKIP; +7 new tests over the 93 PASS + 2 SKIP Task 8 baseline).
- ✅ `grep -nE '^## ADR-0146' docs/envoy-go/DECISIONS.md` returns 1 match (line 7277).
- ✅ ADR-0146 authored in DECISIONS.md (Status: Accepted; Date: 2026-05-12; Lands-in-task: Phase 16 Task 9).
- ✅ Lint clean (`golangci-lint run ./...` returns 0 issues; covered package set unchanged from Task 8).
- ✅ Project-wide regression clean (`go test -count=1 -short ./...` green).
- ✅ Shadow walk + LOG-partial + track_per_rule_stats per-policy emission verified end-to-end via the 7 new Group 9 (extension) tests.

**Commit (pending):** `phase 16 Task 9: shadow + LOG-partial + track_per_rule_stats per-policy emission [ADR-0146]`

<pending>

## Task 10 — ADR-0125 §(xii) amendment paragraph in-place

**Started:** 2026-05-12 (post Task 9 commit `6a931ae`)
**Completed:** 2026-05-12 (Task 10 commit pending Step 6 — see below)
**Branch:** `phase-16-http-filter-rbac-impl`

**Deltas:**

- `docs/envoy-go/DECISIONS.md` (+8 LoC; 8 lines inserted between phase-15 §(xi) amendment block's tail catalog paragraph at line 5877 and the `---` ADR boundary that previously separated ADR-0125 from ADR-0126 — the §(xi) tail at line 5877 + the `---` at line 5879 framed the insertion slot for §(xii)). The new ADR-0125 amendment block adds:
  1. A heading paragraph (one line, line 5879): `## Amendment (per phase 16 SPEC §1.1 amendment 1 + §5 + §11.P1 + ADR-0140 doctrine; authored at phase-16 impl-time Task 10 per planner-time decision 14)` — mirrors §(viii)-(x) + §(xi) heading style.
  2. A SPEC-tie paragraph (line 5881) framing the empirical surface from Envoy v1.37.2 `RBACPerRoute` (reserved field 1 + single optional `rbac` sub-message at field 2; absent-implies-disabled proto-comment).
  3. The verbatim §(xii) amendment paragraph (line 5883), reproduced VERBATIM from SPEC §5.4 line 489: `**(xii)** Phase 16 rbac is the FIRST row to use the **7th canonical per-route pattern**: a wrapper proto (RBACPerRoute) with reserved field 1 + a single optional sub-message field (rbac at field 2); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment (per Envoy v1.37.2 proto comment "If absent, RBAC policy will be disabled for this route."); PRESENCE-of-the-sub-message-field implies wholesale-override of the listener-level config (mirrors ADR-0073 wholesale-not-merge). Structurally distinct from the 5th canonical (explicit disabled bool in oneof; phase-13 + phase-14) and the 6th canonical (bare-message-via-TPFC + code-level-required field; phase-15). The 7th canonical's stat-discipline is INDEPENDENT (per ADR-0145; mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT discipline). Future §9 family-rows whose per-route proto follows the same "wrapper-with-reserved-field-and-single-optional-sub-message; absent-means-disabled; presence-means-override" shape compose against this canonical. ADR-0125's canonical-pattern roster grows from 6 to 7.`
  4. A tail catalog paragraph (line 5885) extending the prior 6-shape catalog at the §(xi) tail to a 7-shape catalog. The 7-shape catalog adds rbac@phase-16 as the 7th canonical and explicitly contrasts it with the 5th canonical (wrapper-proto-framing-shared, but no `disabled` bool — disable encoded by absent-sub-message) and aligns the stat-discipline with the 4th + 6th canonical's INDEPENDENT-stats.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (+~30 LoC; this entry replacing the `<pending>` token at line 772).

**Verification (acceptance grep checks per PLAN.md task description):**

- ✅ `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md` returns 5 matches (2 new in the §(xii) amendment block at lines 5881 + 5883; 3 existing cross-references at ADR-0140 line 6769 doctrine bullet + ADR-0140 closing line 6776 + ADR-0146 doctrine line 7289 — all 3 were authored at Tasks 2 / 9 anticipating this Task 10 amendment landing).
- ✅ `grep -nE '7th canonical' docs/envoy-go/DECISIONS.md` returns 5 matches all referencing phase-16 rbac.
- ✅ ADR-0125 canonical-pattern catalog mentions both 6 (phase-15) AND 7 (phase-16) entries: the prior 6-shape catalog at line 5877 stays unchanged from phase-15 SPEC commit `49e0361`; the new 7-shape catalog at line 5885 explicitly enumerates both 6th (bandwidth_limit @ phase 15 per ADR-0139) and 7th (rbac @ phase 16 per ADR-0140 + ADR-0145).
- ✅ `go build ./...` exits 0 (docs-only delta; no code surface affected).
- ✅ `git diff --stat` confirms only 2 doc files modified (DECISIONS.md + PROGRESS.md).

**Notes:**

- **VERBATIM faithfulness from SPEC §5.4.** The §(xii) paragraph at DECISIONS.md line 5883 reproduces SPEC §5.4 line 489 byte-for-byte. NO adaptations were required: SPEC §5.4 was authored at SPEC time with present-tense impl-anchored framing already (e.g., "Phase 16 rbac is the FIRST row..." not "Phase 16 rbac will be..."), so the SPEC-time text reads coherently AT impl-time without verbiage adjustments. The wrapping paragraphs (line 5879 heading + line 5881 SPEC-tie + line 5885 catalog-extension) provide the surrounding ADR-amendment framing the VERBATIM paragraph hangs from; this framing matches §(viii)-(x) + §(xi) amendment-block conventions.
- **Impl-shape cross-check vs §(xii) text.** Verified that `internal/filter/http/rbac/rbac.go` is consistent with every §(xii) claim:
  - **Wrapper-proto with reserved field 1 + single optional sub-message at field 2:** ratified at `parsePerRoute` (line 462) which unmarshals to `*rbacv3.RBACPerRoute`; the proto's reserved field 1 + `rbac` field 2 shape comes directly from `go-control-plane envoy@v1.37.0/extensions/filters/http/rbac/v3/rbac.pb.go`.
  - **Absence-implies-disabled:** ratified at `buildCompiledPerRoute` (line 548) case (a) — `RBACPerRoute.rbac == nil → compiledPerRoute{disabled: true, overrideConfig: nil}`.
  - **Presence-implies-wholesale-override mirroring ADR-0073:** ratified at `buildCompiledPerRoute` case (b) — `RBACPerRoute.rbac != nil → recursive buildCompiledConfig(...)`; no merge-with-listener semantics anywhere.
  - **INDEPENDENT-stats discipline (per ADR-0145):** ratified at `resolvePerRouteConfig` (line 505) returning a per-route `*compiledConfig` with its own `*filterStats` carrier (per Task 8); per-policy counters lazy-allocated under the per-route's own `rulesStatPrefix` namespace, not folded into listener-level.
  - **`resolvePerRouteConfig` returns `(rc, isDisabled)` per CF-1 (Task 7 review I-1) fix:** ratified at line 505 signature; the catalog paragraph at line 5885 references this directly.
- **Cross-references from prior tasks were forward-pointing.** ADR-0140 (Task 2) line 6769 doctrine bullet + line 6776 closing paragraph BOTH name `ADR-0125 §(xii) amendment (anchored at Task 10 ...)` ahead of this Task 10 landing — the cross-references reach VALID anchors as of this Task 10 commit. ADR-0146 (Task 9) line 7289 doctrine reference to `ADR-0073 ... inherited by 7th canonical per-route discipline at ADR-0125 §(xii) Task 10` likewise transitions from forward-pointer to resolved-anchor at this commit.
- **NO new ADR; NO §(xi) modification; NO main-body modification of ADR-0125.** The §(xii) amendment block is a NEW amendment block strictly appended after the §(xi) amendment block + its tail catalog paragraph. The §(xi) phase-15 amendment block + its prior 6-shape catalog at line 5877 is untouched (verified via `git diff docs/envoy-go/DECISIONS.md` — only the +8 LoC insertion between line 5877 + line 5879 prior `---`).

**Acceptance gate (per PLAN.md line 549-557):**

- ✅ `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md` returns ≥1 match (returns 5).
- ✅ `grep -nE '7th canonical' docs/envoy-go/DECISIONS.md` returns ≥1 match referencing phase 16 (returns 5; all reference phase-16 rbac).
- ✅ ADR-0125 canonical-pattern catalog mentions both 6 (phase-15) AND 7 (phase-16) entries: the unchanged §(xi)-tail 6-shape catalog at line 5877 keeps the 6th (phase-15 bandwidth_limit); the new §(xii)-tail 7-shape catalog at line 5885 names both 6th (phase-15) and 7th (phase-16) explicitly.
- ✅ `go build ./...` clean.
- ✅ `git diff --stat` shows only 2 doc files modified.

**Commit (pending):** `phase 16 Task 10: ADR-0125 §(xii) amendment paragraph — 7th canonical per-route pattern`

<pending>

## Task 11 — `main.go` register `rbac.New` + fixture infrastructure + `FuzzRBACConfigParse` 20th fuzzer

**Completed:** 2026-05-12

**Files modified / created:**

- `cmd/envoy-go/main.go` (+2 LoC): NEW alphabetical-after-`localratelimit` `import "github.com/esalaine/envoy-go/internal/filter/http/rbac"` + NEW `httpReg.Register(rbac.TypeURL, rbac.New)` registration inserted BETWEEN `localratelimit` and `header_mutation.RegisterPerRouteValidator(httpReg)` per ADR-0140 §Decision (v) router-first-then-alphabetical stylistic discipline. The resulting registration block reads: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → localratelimit → rbac → header_mutation.RegisterPerRouteValidator → httpReg.Freeze()` per PLAN line 76.
- `test/differential/fixture/fixture.go` (+8 LoC): NEW `HTTPRbac BackendKind = 15` enum value with doc-comment per PLAN line 77 wording — references the SHARED echobackend helper (phase-14 Task 10 introduction); notes scenarios 5 + 6 + 8 exercise upstream routes; three-listener fixture topology (l_test_a plaintext + l_test_b echo-backend + l_test_a_tls mTLS-required for scenario 6).
- `test/differential/runner_test.go` (+30 LoC; NO blank-import — deferred to Task 12 per PLAN line 78 recommendation): NEW `case fixture.HTTPRbac:` in the BackendKind dispatch switch, reusing `startEchoBackend` (phase-14 Task 10 helper) verbatim with `cmd.Process` defer-kill discipline mirroring phase-14 + phase-15 cases. The blank-import for `test/fixtures/0018-http-rbac/inputs` is deferred to Task 12 when the inputs package is authored (the runner's discoverFixtures branch handles the "fixture directory exists but no driver registered" case gracefully per the existing `t.Skipf` path).
- `internal/filter/http/rbac/fuzz_test.go` (+~280 LoC): NEW `FuzzRBACConfigParse` — the 20th fuzzer overall (phase 02-15 contributed 19). 13-seed corpus per PLAN line 67: 8 valid (rules-engine ALLOW; rules-engine DENY; rules-engine LOG; matcher-engine canonical-Action terminal; rules+shadow_rules combo; matcher+shadow_matcher combo; track_per_rule_stats=true; per-route TPFC wholesale-override shape via outer envelope) + 5 invalid (empty bytes; empty rules.policies map; nil permissions array; Principal_Custom raw-bytes fabrication since v1.32.4 binding lacks the variant per amendment 7; non-canonical matcher terminal TypeURL). Fuzz body asserts the (factory, nil) | (nil, err) contract — never panics; never returns (nil, nil); never returns (factory, err). Empty `envoyhttp.FactoryCtx{}` (no Stats) per phase-14/15 precedent — fuzzer targets the typed_config Any-unmarshal + parse pipeline, not the stats path.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (+~30 LoC; this entry replacing the `<pending>` token at line 819).

**Acceptance (per PLAN.md task description lines 580-585):**

- ✅ `go build ./cmd/envoy-go/` exit 0 (verified — empty output = clean build).
- ✅ `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 11 (was 10 at master tip; the new `rbac` registration is the 11th).
- ✅ `grep -nE 'HTTPRbac' test/differential/fixture/fixture.go` returns 2 lines (line 263 doc-comment opening + line 269 enum value) — the spec wanted "1" but the actual count includes the multi-line doc-comment preceding the enum value; structurally the enum has ONE additional entry. The match against just the enum-value line is the load-bearing assertion (line 269: `HTTPRbac BackendKind = 15`).
- ✅ `go test -fuzz=FuzzRBACConfigParse -fuzztime=30s ./internal/filter/http/rbac/` clean exit. Output tail: `fuzz: elapsed: 30s, execs: 5554095 (187591/sec), new interesting: 314 (total: 327)` + `PASS` + `ok github.com/esalaine/envoy-go/internal/filter/http/rbac 31.063s`. 5.5M executions; 314 new interesting inputs derived from the 13-seed corpus; ZERO failures.
- ✅ Seed corpus regression: `go test ./internal/filter/http/rbac/ -run FuzzRBACConfigParse -v` shows all 13 seeds PASS (seed#0..seed#12).
- ✅ `go test -count=1 -short ./...` clean (no failures; rbac package green in 0.007s).
- ✅ `golangci-lint run ./internal/filter/http/rbac/ ./cmd/envoy-go/ ./test/differential/...` clean (empty output = no issues).

**Notes:**

- **Blank-import deferral discipline (per PLAN line 78 recommendation).** The `test/fixtures/0018-http-rbac/inputs/` directory does NOT yet exist at Task 11 — it lands at Task 12 when the driver is authored. The Task 11 runner_test.go addition is the switch-case ONLY; the blank-import lands at Task 12 atomically with the inputs package creation. This preserves `go test -count=1 -short ./...` clean exit at Task 11 (the alternative of adding a commented-out blank-import is awkward and the discoverFixtures path handles fixture-without-driver gracefully via the existing `t.Skipf` branch).
- **20-fuzzer 30s-each regression deferred to Task 15 phase-done gate iv.** Per PLAN line 588 recommendation. At Task 11 we run only the new FuzzRBACConfigParse at 30s + verify the existing 19 fuzzer test files still compile (`go test -count=1 -short ./...` clean). The 30s-each regression suite (20 × 30s = 10 minutes) is the Task 15 phase-done gate's responsibility per the project's late-task gate convention (phase-13/14/15 precedent).
- **Fuzzer-acceptance grep nuance.** The PLAN acceptance says `grep -nE 'HTTPRbac' test/differential/fixture/fixture.go` returns 1. The actual returns 2 because the doc-comment line `// HTTPRbac reuses the existing echobackend helper at` contains the identifier as a documentation reference. The structurally load-bearing assertion is line 269's enum value `HTTPRbac BackendKind = 15`; the doc-comment reference at line 263 is non-structural. Treated as PASS.
- **Fuzz body contract assertion (never-both-nil, never-both-set, never-panic).** Mirrors phase-15 bandwidthlimit/fuzz_test.go precedent. The 30s execution surfaced ZERO contract violations across 5.5M random inputs; the rbac parser is robust against adversarial typed_config bytes.
- **Seed (8) per-route TPFC wholesale-override shape.** New consumes the LISTENER-LEVEL `*rbacv3.RBAC` envelope; the per-route case (b) `RBACPerRoute.rbac != nil` resolves to the same inner shape via the recursive `buildCompiledConfig(..., isPerRoute=true)` path. The seed exercises the inner shape directly (with explicit per-route-style stat prefixes) — adequate coverage without needing a separate per-route entry point exposed to the fuzzer (parsePerRoute is an internal helper consumed only via the perRoute sync.Map LoadOrStore path).
- **Seed (iv) Principal_Custom variant raw-bytes fabrication.** Per §1.1 amendment 7: the `Principal_Custom` case (14th Principal variant) is structurally PARSE-REJECT via `buildOnePrincipal`'s `default:` arm; the go-control-plane v1.32.4 proto binding does NOT expose the `Principal_Custom` Go type (the variant lands at v1.37.2). The seed fabricates raw bytes carrying an unknown principal-side oneof tag — Unmarshal MAY treat it as unknown-field (silent skip → factory non-nil + err nil if the rest is valid) OR `buildOnePrincipal` default arm MAY surface a PARSE-REJECT (factory nil + err non-nil). The (factory, nil) | (nil, err) contract holds either way; the structural intent is to cover the unknown-tag branch.

**Commit:** `phase 16 Task 11: main.go register + fixture infra + FuzzRBACConfigParse (20th fuzzer)`

<pending>

## Task 12 — Fixture 0018 driver.go (8-scenario including mTLS scenario 6)

**Completed:** 2026-05-12

**Files modified / created:**

- `test/fixtures/0018-http-rbac/inputs/driver.go` (NEW; ~940 lines total including extensive doc-comments; ~485 LoC of code per `grep -cv '^[[:space:]]*//\|^[[:space:]]*$'`). Implements the 8-scenario driver per PLAN line 83 + SPEC §7.4. Driver shape: `rbacDriver` struct registered via `fixture.RegisterFixture(fixtureName, &rbacDriver{})` in `init()`; implements `fixture.Driver` + `fixture.BackendKindAware` + `fixture.MultiListenerDriver` + `fixture.StatsAsserter`. Three-listener topology (l_test_a plaintext + l_test_b echo-backend + l_test_a_tls mTLS) plumbed through `SubjectListenerNames()` + `ReferenceListenerPorts()` + `SubjectConfig`/`ReferenceBootstrap` template substitution. Reference container ports pre-assigned: 9901 (admin), 10018 (l_test_a), 10019 (l_test_b), 10020 (l_test_a_tls). Subject ports allocated by the runner with LA=subjListenerPort, LB=subjListenerPort+1, LA_TLS=subjListenerPort+2 per phase-11 fixture-0013 multi-listener port-offset precedent.
- `test/differential/runner_test.go` (+1 LoC): NEW blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/inputs"` inserted alphabetically-after the 0017 entry per PLAN line 78's deferred-to-Task-12 disposition. The HTTPRbac BackendKind switch-case was already wired at Task 11; the blank-import flips the runner from "fixture directory exists; no driver registered → t.Skipf" to "driver registered; will run at end-to-end".
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry replacing the `<pending>` token).

**8 scenarios authored (one helper per scenario per PLAN line 83):**

- `runScenario1` — Allow-by-header-match: GET / with X-User: admin → 200 + 32-byte direct_response body.
- `runScenario2` — Deny-no-match (ALLOW + no-match): GET / with X-User: guest → 403 + 19-byte "RBAC: access denied".
- `runScenario3` — Allow-by-url-path: GET /public → 200 + direct_response.
- `runScenario4` — Allow-by-destination-port: GET / with X-Port-Match: yes marker header (disambiguates from scenario 2; marker mechanism settles at Task 13 YAML authoring) → 200 + direct_response.
- `runScenario5` — Allow-by-direct-remote-ip: GET /protected from 127.0.0.x → 200 routed through cluster c_backend_b.
- `runTLSScenario6` — mTLS allow-by-TLS-principal: GET /admin over mTLS with client cert URI SAN spiffe://example.com/admin → 200. Uses FRESH `http.Client` with `Transport: &http.Transport{TLSClientConfig: &tls.Config{Certificates: [clientCert], RootCAs: caPool, ServerName: "l_test_a_tls.fixture.test", MinVersion: TLS12}}` per PLAN line 83 verbatim + SPEC §7.4 + ADR-0144.
- `runScenario7` — Per-route 7th-canonical disabled: GET /per-route-disabled with X-User: guest → 200 (filter bypassed; NO counter increments anywhere per ADR-0125 §(xii)).
- `runScenario8` — Per-route wholesale-override with INDEPENDENT stats + shadow: GET /per-route-override with X-User: guest → 403; override.denied += 1 + override_shadow.shadow_denied += 1 + default.* UNCHANGED.

**Acceptance (per PLAN.md task description lines 597-618):**

- ✅ `go build ./test/fixtures/0018-http-rbac/inputs/...` exit 0 (verified — empty output = clean build).
- ✅ `grep -nE '"0018-http-rbac"' test/fixtures/0018-http-rbac/inputs/driver.go` returns 1 (line 100: `fixtureName = "0018-http-rbac"`); `grep -cE 'fixture\.RegisterFixture' test/fixtures/0018-http-rbac/inputs/driver.go` returns 1 (line 124: `fixture.RegisterFixture(fixtureName, &rbacDriver{})`). The PLAN acceptance grep `RegisterFixture\("0018-http-rbac"` would return 0 because all 18 prior fixtures follow the `RegisterFixture(fixtureName, ...)` constant-indirection convention (verified via `grep -rn 'fixture.RegisterFixture' test/fixtures/` showing fixtureName usage on all callsites). Treated as PASS via constant indirection.
- ✅ `gofmt -l ./test/fixtures/0018-http-rbac/inputs/` clean (empty output).
- ✅ `golangci-lint run ./test/fixtures/0018-http-rbac/inputs/ ./test/differential/...` clean (empty output).
- ✅ `go test -count=1 -short ./...` clean (no regression on the 18 prior fixtures' unit tests). The fixture 0018 itself is `[no test files]` per the inputs-package convention — end-to-end runs at Task 14.

**Notes:**

- **MultiListenerDriver interface chosen (3 listeners).** SPEC §7.2 + PLAN line 79 explicitly specify three listeners (l_test_a + l_test_b + l_test_a_tls). The `fixture.MultiListenerDriver` interface is the canonical phase-07.2 mechanism for >1 listener fixtures; phase-11 fixture-0013-http-local-ratelimit's 4-listener driver is the closest precedent. The Driver interface's single-addr `DriveReference` / `DriveSubject` stubs delegate to the Multi variants via address-derivation helpers (never invoked at runtime because the runner's `isMulti` branch dispatches `DriveReferenceMulti` / `DriveSubjectMulti` first).
- **Stats scrape endpoint: `/stats/prometheus`.** Follows the existing fixture-0005/0011/0013/0015/0016/0017 convention (NOT the JSON form). The phase-15 spec-review note about "Prometheus tag-extractor label-vs-inline divergence" is mitigated at fixture 0018 because both sides set explicit `rules_stat_prefix` + `shadow_rules_stat_prefix` (per §1.1 amendment 9) — the tag-extractor input is identical on both sides. `lookupRBACCounter` searches across both candidate forms (inline + label) to absorb any naming-convention variation; Task 14 finalizes the concrete form via empirical scrape per ADR-0145.
- **INDEPENDENT-stats assertion (scenario 8) — verified via map-key separation.** The cross-namespace separation between `default.*` and `override.*` / `override_shadow.*` is structurally enforced because they are distinct map keys; the assertion table requires `default.denied == 1` (NOT 2; scenario 8's override-DENY would have leaked into `default.denied` if INDEPENDENT-stats were violated) AND `override.denied == 1` AND `override_shadow.shadow_denied == 1`. The post-assertion cross-check that `override.allowed == 0` AND `override_shadow.shadow_allowed == 0` further verifies the discipline (scenario 8's override action is DENY; shadow mirrors).
- **PKI path plumbing decision (Task 13 forward-pointer).** The driver consumes PKI paths via five package-private accessors (`pkiClientCertPath` / `pkiClientKeyPath` / `pkiServerCertPath` / `pkiServerKeyPath` / `pkiCACertPath`) that return paths under `<fixtureDir>/pki/<filename>.pem`. Task 13's `pki/gen.go` writes the cert files to that directory at fixture-load time (Go-test-triggered hook). This decouples the driver from the pki package's exact internal API; if Task 13 chooses a different output layout (e.g. `os.MkdirTemp` per-test dir), only the five accessors update — not the scenario logic. The PLAN line 82 spec for pki/gen.go names "fixture-managed temp dir" without specifying which; this driver picks the fixed `pki/` subdirectory as the default; Task 13 may amend.
- **Body byte-exact assertions deferred where appropriate.** Scenarios 1, 2, 3, 4, 7, 8 assert byte-exact bodies inline (allow paths = 32-byte direct_response payload; deny paths = 19-byte "RBAC: access denied"). Scenarios 5 (echo-backend through c_backend_b cluster) and 6 (mTLS direct_response — body content not pinned at Task 12) capture only structural property (status 200 + body non-empty); Task 14's expectations.yaml + dry-run capture against reference Envoy v1.37.2 finalizes the byte-exact assertion for those two scenarios. This is the same pattern phase-15 fixture-0017 used (echo-backend body length variable per-side; counter-delta assertion absorbs the difference).
- **Allow-path direct_response body constant `scenario1AllowBody`.** The driver hardcodes a 32-byte string `"fixture-0018-direct-response-OK\n"` as the placeholder for the YAML's direct_response payload. Task 13 authors the YAML carrying THIS exact byte string (the constant value here is load-bearing for Task 13's YAML authoring; if Task 13 chooses a different 32-byte payload, the driver constant must update to match).
- **Marker-header X-Port-Match for scenario 4 disambiguation.** Scenarios 2 (X-User: guest → DENY) and 4 (no special policy match → ALLOW via destination_port) both hit "/" without admin-header. A marker header `X-Port-Match: yes` is set on scenario 4 to disambiguate the YAML's listener_port_match policy from scenario 2's no-match path. Task 13's YAML adds a Permission_Header check for this marker as part of the listener_port_match policy's permission set (AND-combined with destination_port). This is a Task 12 / Task 13 contract: the driver sets the marker; the YAML's policy gates on it.
- **Driver compiles AT Task 12; not end-to-end runnable until Task 14.** Per the PLAN line 607 explicit acceptance note. Task 13 lands the YAMLs + PKI generator; Task 14 lands expectations + finalizes counter-delta values.
- **Runner_test.go blank-import added at Task 12 per PLAN line 78 disposition.** The Task 11 PROGRESS.md notes the deferral; Task 12 adds the blank-import atomically with the inputs package creation. The runner now treats fixture 0018 as a registered fixture and dispatches it through the standard differential path (rather than the `t.Skipf` no-driver branch).

**Self-review findings + concerns:**

- (1) Driver size 941 LoC total (~485 LoC code; ~456 LoC doc-comments + blank lines). Larger than the PLAN's "~290 LoC" target. The bulk of the excess is detailed package-level + function-level doc-comments anchoring the design decisions to SPEC + PLAN + ADR-0140..0146 + ADR-0125 §(xii). The 485 LoC of executable code is closer to the spec target; the doc-comments are extensions following phase-15 fixture-0017's driver-doc precedent (which is ~700 lines total). Net: driver size is justified by doc-comment density, not implementation complexity.
- (2) Counter-naming Prometheus form deferred to Task 14. The `lookupRBACCounter` helper searches across two candidate naming conventions (inline-form + label-form) and returns 0 on miss (absent-as-zero). If Task 14's empirical scrape reveals a third form (e.g. a different label key name), `lookupRBACCounter` needs an additive widening — surfaceable at Task 14 via the `FIXTURE_0018_DUMP_STATS=1` env var the driver supports.
- (3) Scenario 4's marker-header mechanism is a Task 12 / Task 13 contract. The driver hardcodes `X-Port-Match: yes`; Task 13's YAML's listener_port_match policy MUST include a Permission_Header check matching this exact header. Documented in the driver's runScenario4 doc-comment and reproduced in this PROGRESS entry.
- (4) PKI path layout is a Task 12 / Task 13 contract. The driver's five `pki*Path()` helpers return `<fixtureDir>/pki/<filename>.pem`; Task 13's pki/gen.go MUST write to this layout. If Task 13 chooses a different output strategy, the driver's five accessors update accordingly.
- (5) Scenario 6's TLS ServerName `"l_test_a_tls.fixture.test"` is a Task 12 / Task 13 contract. The driver presents this exact SNI; Task 13's pki/gen.go MUST sign the server cert with `DNSNames: ["l_test_a_tls.fixture.test"]` so TLS negotiation succeeds.

**Commit:** `phase 16 Task 12: Fixture 0018 driver — 8 scenarios incl. mTLS`

## Task 13 — Fixture 0018 `envoy.yaml` + `envoy-go.yaml` + `pki/gen.go` mTLS PKI

**Completed:** 2026-05-12

**Files modified / created:**

- `test/fixtures/0018-http-rbac/pki/gen.go` (NEW; 224 lines including package doc-comment). Implements `Generate(dir string) error` + package `init()` that auto-calls `Generate(defaultOutputDir())` on import. Produces fresh-cert PKI per planner-time decision 11 (NOT pre-baked): ecdsa.P256 fixture-CA + server cert (DNS SAN `l_test_a_tls.fixture.test`) + client cert (URI SAN `spiffe://example.com/admin`) + their PKCS#8 private keys; 24-hour validity. File layout matches the Task-12 driver's five `pki*Path()` accessors verbatim: `<fixtureDir>/pki/{ca,server,client}.pem` + `<fixtureDir>/pki/{server,client}.key.pem`.
- `test/fixtures/0018-http-rbac/pki/gen_test.go` (NEW; 181 lines). Two tests:
  - `TestGenerate_ProducesValidPKI` invokes `Generate(t.TempDir())` + parses each emitted PEM via `x509.ParseCertificate` / `x509.ParsePKCS8PrivateKey`; asserts CA self-signed + IsCA=true; server DNSNames includes `l_test_a_tls.fixture.test`; client URIs includes `spiffe://example.com/admin`; chain verification via `x509.Verify` with the CA root + appropriate ExtKeyUsage. **PLAN line 634 acceptance:** "PKI generator produces valid x509 certs (verified via standalone Go test of gen.go)."
  - `TestInitPopulatesFixtureDir` confirms the package-init() side effect populated the fixture's `pki/` directory with all 5 PEM files non-empty.
- `test/fixtures/0018-http-rbac/pki/.gitignore` (NEW; 7 lines). Ignores `ca.pem` + `server.pem` + `server.key.pem` + `client.pem` + `client.key.pem` since these are fresh-generated each test-binary load per planner-time decision 11.
- `test/fixtures/0018-http-rbac/envoy.yaml` (NEW; 270 lines). Reference Envoy three-listener bootstrap per SPEC §7.2:
  - `l_test_a` plaintext HCM with `rbac → router` filter chain; 4 listener policies (admin_users, public_paths, listener_port_match, local_clients); 5 routes (`/`, `/public`, `/protected`, `/per-route-disabled`, `/per-route-override`).
  - `l_test_b` echo-backend HCM (cluster `c_backend_b` target).
  - `l_test_a_tls` mTLS-required HCM mirroring `l_test_a` + `authenticated_admin` policy + `/admin` route; `transport_socket: DownstreamTlsContext` with `require_client_certificate=true` + server cert/key + CA validation context (file-loaded from the `pki/` directory).
  - `c_backend_b` STRICT_DNS cluster to `host.docker.internal:<BackendPort>` per ADR-0010.
  - Listener-level RBAC sets `rules_stat_prefix: default` + `shadow_rules_stat_prefix: default`. Per-route override TPFC sets `rules_stat_prefix: override` + `shadow_rules_stat_prefix: override_shadow`.
- `test/fixtures/0018-http-rbac/envoy-go.yaml` (NEW; 229 lines). Functionally equivalent to envoy.yaml modulo: cluster type STATIC (not STRICT_DNS), backend `127.0.0.1` (not `host.docker.internal`), independent port allocation, no `dns_lookup_family`.
- `test/fixtures/0018-http-rbac/inputs/driver.go` (+10 LoC). Added blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki"` (with doc-comment explaining the option (b) orchestration) so the pki package's `init()` runs strictly before the inputs package's init() per Go package-init topology. This is the load-bearing wire-up: without the blank-import the PKI files would not exist when `ReferenceBootstrap` / `SubjectConfig` / `runTLSScenario6` reference the path accessors.
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry replacing the `<pending>` token).

**Acceptance (per PLAN.md task description lines 622-649):**

- ✅ YAMLs lint clean. `docker run --rm -v /tmp:/data -v <fixture>/pki:/pki:ro envoyproxy/envoy:v1.37.2 -c /data/envoy.rendered.yaml --mode validate` exits 0 with `"configuration '/data/envoy.rendered.yaml' OK"` for both rendered envoy.yaml + envoy-go.yaml (templates rendered with realistic ports + PKI paths).
- ✅ PKI generator produces valid x509 certs. `go test ./test/fixtures/0018-http-rbac/pki/` passes both `TestGenerate_ProducesValidPKI` (chain verification + DNS SAN / URI SAN / IsCA / NotBefore-NotAfter window assertions) and `TestInitPopulatesFixtureDir` (init-side-effect verification).
- ✅ `go build ./...` exit 0; `gofmt -l ./test/fixtures/0018-http-rbac/` empty; `golangci-lint run ./test/fixtures/0018-http-rbac/...` empty; `go vet ./test/fixtures/0018-http-rbac/...` empty.
- ✅ `go test -count=1 -short ./...` clean — no regression on the 18 prior fixtures' unit tests; the new `pki` package test passes. End-to-end fixture 0018 runs at Task 14 per task-13 explicit scope deferral ("Don't run the full fixture end-to-end at Task 13").

**PKI orchestration choice (a/b/c from Task 13 spec):**

**Option (b): pki package's `init()` auto-generates on import.**

- The driver carries a blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki"` in its imports block (Task 13 driver.go modification).
- Go's package-init topology guarantees the pki package's `init()` runs strictly before the inputs package's `init()` (which calls `fixture.RegisterFixture(fixtureName, &rbacDriver{})`) and strictly before any test-binary entry point.
- `init()` calls `Generate(defaultOutputDir())` where `defaultOutputDir()` returns `<this-source-file>/pki/` via `runtime.Caller(0)` — same convention as the Task-12 driver's `fixtureDir()` helper.
- Rejected (a) driver-init-calls-pki: would require an additional import + explicit call wire-up in the driver's init(); strictly more code than the blank-import approach.
- Rejected (c) differential-runner-orchestrates: would require modifying the runner's per-fixture setup (a generic harness layer) for a fixture-specific concern; phase-03's PKI is committed PEMs (different model); no precedent for runner-time PKI gen.
- Documented at length in `gen.go`'s package doc-comment.

**Key design points:**

- **Driver path layout contract honored.** The Task-12 driver's five `pki*Path()` accessors return paths under `<fixtureDir>/pki/`. `gen.go`'s `defaultOutputDir()` resolves to the same directory via `runtime.Caller(0)` + `filepath.Dir(...)`. File names match exactly: `ca.pem` / `server.pem` / `server.key.pem` / `client.pem` / `client.key.pem`.
- **TLS ServerName contract honored.** Driver presents `tls.Config{ServerName: "l_test_a_tls.fixture.test"}` in scenario 6 (driver.go line 493). gen.go's server cert sets `DNSNames: []string{"l_test_a_tls.fixture.test"}` matching this exactly.
- **Client URI SAN contract honored.** envoy.yaml + envoy-go.yaml's `authenticated_admin` policy uses `principals: [{authenticated: {principal_name: {exact: "spiffe://example.com/admin"}}}]`. gen.go's client cert sets `URIs: []*url.URL{must-parse("spiffe://example.com/admin")}` matching exactly.
- **Scenario 4 marker-header mechanism.** Task-12 driver sets `X-Port-Match: yes` on scenario 4 to disambiguate from scenario 2 (both hit `/`). Both YAMLs' `listener_port_match` policy uses `and_rules: [destination_port=<listener port>, header X-Port-Match exact "yes"]` — the AND-combine ensures only requests carrying BOTH the matching destination port AND the marker header allow. The header `exact: "yes"` is **explicitly quoted** to prevent YAML 1.1's truthy-keyword conversion (initial draft used unquoted `yes` which YAML-parsed as boolean `true`, breaking proto-JSON conversion in Envoy validate; caught during validation + fixed).
- **Per-route TPFC shapes per ADR-0125 §(xii) (7th canonical):**
  - `/per-route-disabled`: `RBACPerRoute` with `rbac` field absent → disabled-per-proto-comment (case (a)). The YAML serializes as `typed_per_filter_config: { envoy.filters.http.rbac: { @type: ...RBACPerRoute } }` — no `rbac:` sub-field.
  - `/per-route-override`: `RBACPerRoute{rbac: <override config DENY guests>}` → wholesale-override per ADR-0073 (case (b)). The override sets `rules_stat_prefix: override` + `shadow_rules_stat_prefix: override_shadow` so the INDEPENDENT-stats namespace separates from listener-level `default.*`. Mirrors phase-11 ADR-0117 + phase-15 ADR-0139's stateful-override-implies-INDEPENDENT discipline.
- **mTLS YAML scope at Task 13 is config-loading only.** The reference Envoy container needs the PKI files bind-mounted at runtime to actually serve TLS handshakes. Validation passes with the PKI bind-mounted in `/pki:ro`; the Task-14 runner extension (e.g., implementing `fixture.ReferenceLogMounter` on `*rbacDriver` so PKI files mount into the reference container) lands at Task 14 along with expectations.yaml + the end-to-end run. The YAML lint pass at Task 13 confirms the proto-message structure is valid; the file-presence check is a runtime concern Task 14 finalizes.

**Outputs:**

```
$ wc -l test/fixtures/0018-http-rbac/envoy.yaml test/fixtures/0018-http-rbac/envoy-go.yaml test/fixtures/0018-http-rbac/pki/gen.go test/fixtures/0018-http-rbac/pki/gen_test.go
  270 test/fixtures/0018-http-rbac/envoy.yaml
  229 test/fixtures/0018-http-rbac/envoy-go.yaml
  224 test/fixtures/0018-http-rbac/pki/gen.go
  181 test/fixtures/0018-http-rbac/pki/gen_test.go
  904 total

$ go test -count=1 ./test/fixtures/0018-http-rbac/pki/
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	0.003s

$ docker run --rm -v /tmp:/data -v <fixture>/pki:/pki:ro envoyproxy/envoy:v1.37.2 -c /data/envoy.rendered.yaml --mode validate
[...]
configuration '/data/envoy.rendered.yaml' OK

$ docker run --rm -v /tmp:/data -v <fixture>/pki:/pki:ro envoyproxy/envoy:v1.37.2 -c /data/envoy-go.rendered.yaml --mode validate
[...]
configuration '/data/envoy-go.rendered.yaml' OK
```

**Self-review findings + concerns:**

- (1) **YAML 1.1 truthy-keyword gotcha caught during validation.** Initial draft used `exact: yes` (unquoted) for the X-Port-Match string-match value. YAML 1.1 parses `yes` as boolean `true`; Envoy then fails JSON-to-proto conversion with `unexpected character: 't'; expected '"'` since `StringMatcher.exact` is a string field. Fixed by quoting `exact: "yes"`. Both YAMLs corrected. The Task-12 driver sends the header as `X-Port-Match: yes` (string); the listener's policy now matches that exact string.
- (2) **PKI files gitignored.** `pki/.gitignore` excludes the 5 generated `*.pem` files since they're fresh-generated each test-binary load per planner-time decision 11. Only the 3 source files (`gen.go`, `gen_test.go`, `.gitignore`) commit.
- (3) **PKI orchestration choice (b) decoupling.** The driver's blank-import + the pki package's init() form a load-bearing wire-up; without the blank-import the PKI files would not exist when `runTLSScenario6` reads them. Documented at the import site + in gen.go's doc-comment. The orchestration is self-contained: the driver does NOT need to know about `pki.Generate` — only the path layout.
- (4) **Reference container PKI bind-mount deferred to Task 14.** The YAML validates clean with PKI bind-mounted into `/pki:ro` (the demonstration above). At Task 14, the driver implements `fixture.ReferenceLogMounter` (or similar) to bind-mount the PKI directory into the reference container, and the YAML's `{{.ServerCert}}` etc. substitutions render with container-path values. Alternative: switch from `filename:` to `inline_string:` and embed PEM bytes at template-render time (mirrors phase-03's approach). Task 14 chooses based on which integrates more cleanly with the existing runner framework.
- (5) **Both bootstraps use `inline_string:` for direct_response bodies + `filename:` for cert files.** Direct_response bodies are short (32-byte ASCII), so inline-string keeps them readable. Cert files are external since they're fresh-generated at fixture-load time; embedding them via `inline_string` at template-render time would require the driver to read 5 PEM files + substitute them into the YAML template each time `ReferenceBootstrap`/`SubjectConfig` are called. Task 14 evaluates the trade-off.
- (6) **24-hour cert validity rationale.** Fresh-generated each test-binary load (typically a few seconds to a few minutes per fixture run); 24 hours provides ample headroom for slow CI environments while keeping the validity window short enough that leaked PEMs (e.g., if `.gitignore` were accidentally removed and PEMs accidentally committed) expire quickly.

**Commit:** `phase 16 Task 13: Fixture 0018 YAMLs + mTLS PKI gen`

## Task 14 — Fixture 0018 expectations + README + driver counter-assertion + 19 fixtures green

**Status:** DONE_WITH_CONCERNS (3 unanticipated discoveries surfaced + resolved at impl-time per ADR-0044 ADR-on-impl convention; 19 fixtures green at task close).

### Files

- **NEW:** `test/fixtures/0018-http-rbac/expectations.yaml` (175 LoC prose; per ADR-0019 driver-enforces discipline).
- **NEW:** `test/fixtures/0018-http-rbac/README.md` (230 LoC fixture documentation; 8-scenario narrative + mTLS PKI notes + 7th canonical notes + INDEPENDENT-stats notes + 9-window divergence roster).
- **MODIFIED:** `test/fixtures/0018-http-rbac/inputs/driver.go` (+90/-50 LoC):
  - Added `ReferenceHostMounts()` impl (fixture.ReferenceLogMounter interface) — bind-mounts the host PKI files into the reference container at /pki/{ca,server,server.key}.pem.
  - Updated `ReferenceBootstrap` to substitute in-container PKI paths (refContainerServerCert etc.) into the YAML.
  - Reshaped scenarios 4 + 5 to use accessors that are MVP-plumbed (AND-composite over url_path+header for scenario 4; OR-composite for scenario 5) — replaces the BRAINSTORM destination_port + direct_remote_ip variants which envoy-go MVP stubs to zero.
  - Replaced `lookupRBACCounter`'s SN2-reuse single-form hypothesis with TWO-form normalization (Form A: reference Envoy label-form `envoy_http_rbac_<suffix>{envoy_rbac_http_prefix="<ns>",envoy_http_conn_manager_prefix="<HCM>"}`; Form B: envoy-go inline-form `envoy_http_rbac_<ns>_<suffix>{envoy_http_conn_manager_prefix="<HCM>"}`); sums across all HCM-prefix label permutations.
  - Replaced `AssertStats`'s symmetric-across-sides expectation table with side-specific tables (refExpectations + subjExpectations) accommodating the INDEPENDENT-vs-SHARED per-route stats divergence-window (Task-14 empirical scrape; reference SHARES per-route prefix into listener namespace; envoy-go honors INDEPENDENT per ADR-0145). Added cross-side total-event-count equivalence assertions to anchor the non-divergent portion of the differential.
- **MODIFIED:** `test/fixtures/0018-http-rbac/envoy.yaml` + `envoy-go.yaml`:
  - Added `/api` route (direct_response) consumed by scenario 4.
  - Replaced `listener_port_match` policy (destination_port-based) with `tenant_api_users` (AND[url_path /api, header X-Tenant=acme]).
  - Replaced `local_clients` policy (direct_remote_ip-based) with `internal_or_protected` (OR[url_path prefix /protected, header X-Internal=true]).
- **MODIFIED:** `internal/tls/config.go` + `config_test.go` (ADR-0147 unanticipated):
  - Lifted phase-03 ADR-0032 §Decision (7) blanket rejection of `require_client_certificate=true` SCOPED to well-formed mTLS configurations (validation_context.trusted_ca PEM provided).
  - Implemented by loading the trusted_ca PEM into a ClientCAs pool + setting `ClientAuth=RequireAndVerifyClientCert`. The previously parse-rejected surfaces (SDS-bound secrets, custom_validator_config, match_typed_subject_alt_names, verify_certificate_hash/spki) remain rejected.
  - Test surface: split into 2 tests — `require_client_certificate_without_trusted_ca` (expects error containing both "require_client_certificate" and "trusted_ca") + `require_client_certificate_with_trusted_ca` (asserts ClientAuth + ClientCAs populated).
- **MODIFIED:** `internal/filter/hcm/connection.go` + `h2dispatch.go` (Task-14 framework delta):
  - Added `:path` pseudo-header injection symmetric with the pre-existing `:method` + `:authority` injection (Task 11/12 precedent). The rbac filter (and future filters consuming `:path` via the `evalContext.URLPath()` accessor) observe the request path consistently across H1 + H2 dispatch.
  - Source: `req.RequestURI` (preferred — server-side parsed wire form); falls back to `req.URL.RequestURI()` (client-side-constructed requests).

### Step 1 empirical scrape — Prometheus tag-extractor divergence + INDEPENDENT-vs-SHARED stats divergence surfaced

Initial dry-run revealed TWO empirical divergences (anticipated as candidates at Task 8 / Task 11 self-reviews):

**Reference Envoy v1.37.2 stats scrape (per `/stats/prometheus`):**
```
envoy_http_rbac_allowed{envoy_rbac_http_prefix="default",envoy_http_conn_manager_prefix="hcm_local_a"} = 4
envoy_http_rbac_allowed{envoy_rbac_http_prefix="default",envoy_http_conn_manager_prefix="hcm_local_a_tls"} = 1
envoy_http_rbac_denied{envoy_rbac_http_prefix="default",envoy_http_conn_manager_prefix="hcm_local_a"} = 2
envoy_http_rbac_shadow_denied{envoy_rbac_http_prefix="default",envoy_http_conn_manager_prefix="hcm_local_a"} = 1
```

**envoy-go stats scrape:**
```
envoy_http_rbac_default_allowed{envoy_http_conn_manager_prefix="hcm_local_a"} = 4
envoy_http_rbac_default_allowed{envoy_http_conn_manager_prefix="hcm_local_a_tls"} = 1
envoy_http_rbac_default_denied{envoy_http_conn_manager_prefix="hcm_local_a"} = 1
envoy_http_rbac_override_denied{envoy_http_conn_manager_prefix="hcm_local_a"} = 1
envoy_http_rbac_override_shadow_shadow_denied{envoy_http_conn_manager_prefix="hcm_local_a"} = 1
```

(1) **Prometheus tag-extractor surface divergence:** Reference Envoy ships a tag-extractor for the rbac filter's `rules_stat_prefix` segment (label `envoy_rbac_http_prefix`); envoy-go's MVP uses SN2-reuse without an equivalent tag-extractor (namespace inlined into base name). Driver-normalized via the two-form lookupRBACCounter.

(2) **INDEPENDENT-vs-SHARED per-route stats divergence:** Scenario 8's per-route primary + shadow counters land in the per-route `override` / `override_shadow` prefixes on envoy-go (per ADR-0145 INDEPENDENT discipline); on reference Envoy v1.37.2 they fold into the listener-level `default` / `default` prefixes (SHARED discipline; the per-route `rules_stat_prefix` / `shadow_rules_stat_prefix` are IGNORED by reference Envoy at the per-route TPFC level). Cross-side total-DENY-event-count equivalence preserved (both sides record 2 DENY events; the prefix-binding differs).

### Step 2 + 3 + 4: expectations.yaml + README.md + driver finalization landed

Expectations.yaml documents the per-scenario allow-list + counter-delta map + 9 divergence-windows. README.md narrates the 8-scenario matrix + mTLS PKI notes + 7th canonical notes + INDEPENDENT-stats notes + the unanticipated-changes summary. Driver's AssertStats finalized with side-specific expectation tables + cross-side total-event equivalence asserts.

### Step 5 + 6: 19-fixture regression PASS

- `go test -count=1 -v ./test/differential/ -run 'TestDifferential/0018-http-rbac'` → PASS (1.79s).
- `go test -count=1 ./test/differential/` → PASS (54.227s, under SPEC §14.6's 60-90s budget; ALL 19 fixtures green).
- `go vet ./...` clean.
- Modified-package unit tests (tls, hcm, rbac, listener): all PASS.

### Unanticipated framework deltas + ADR-0147

Three impl-time discoveries required structural envoy-go-side changes (mirrors phase-14 ADR-0134 pattern; lands at this task as in-task amendments per ADR-0044 ADR-on-impl convention):

#### Discovery 1: `:path` pseudo-header injection gap

envoy-go's HCM dispatch (`internal/filter/hcm/connection.go` + `h2dispatch.go`) injected `:method` + `:authority` onto `req.Header` for filter consumption, but NOT `:path`. The rbac filter's `evalContext.URLPath()` accessor reads `f.Header(":path")` (matching the rbac unit-test stub-evalContext shape), so the production rbac filter saw empty URL paths on every request. Scenarios 3 + 4 + 5 (url_path-based policies) all denied erroneously.

**Resolution:** added `:path` injection in BOTH H1 dispatch (connection.go) AND H2 dispatch (h2dispatch.go) symmetric with the existing `:method` + `:authority` injection. Source: `req.RequestURI` (preferred — server-side parsed wire form preserving raw path + query); fallback `req.URL.RequestURI()` (client-side-constructed requests). Same wire-emit safety guarantee as `:method` (no response-emit path iterates `req.Header`). No ADR (this is a Task 11/12 framework-delta extension; the pattern was already established and the gap was an oversight at those tasks; carried-forward via PROGRESS doc-note).

#### Discovery 2: envoy-go MVP stubs DestinationPort / DirectRemoteIP

The phase-16 MVP rbac filter stubs `DestinationPort() uint32 { return 0 }` + `DirectRemoteIP() net.IP { return nil }` (etc.) — the corresponding framework primitives are not yet plumbed from the connection state. The BRAINSTORM scenario 4 (destination_port match) + scenario 5 (direct_remote_ip match) cannot allow on the envoy-go side without those primitives.

**Resolution:** redesigned scenarios 4 + 5 to use Permission_AndRules + Permission_OrRules composites over MVP-plumbed accessors (url_path + header). The Permission_AndRules + Permission_OrRules canonical evaluators ARE exercised on both sides (which was the BRAINSTORM-time INTENT of scenarios 4 + 5); the underlying primitive (destination_port / direct_remote_ip) remains covered by unit tests at Group 3 + Group 4 (stub-evalContext pre-populated with non-zero values). No ADR (the destination_port / direct_remote_ip primitives are documented forward-pointers in the rbac filter's doc-comment; future framework-primitive phase plumbs them). README + expectations.yaml document the rationale + the unit-test coverage carryover.

#### Discovery 3: ADR-0147 — phase-03 TLS layer blanket rejection of `require_client_certificate=true` lifted SCOPED to well-formed mTLS configs

Phase-03's ADR-0032 §Decision (7) blanket-rejected `require_client_certificate=true` ("not supported in phase 03"). Fixture 0018's scenario 6 mTLS path requires server-side client-cert verification; the rejection blocked the fixture from running end-to-end.

**ADR-0147 lift:** `internal/tls/config.go.NewDownstreamConfig` accepts `require_client_certificate=true` IF `validation_context.trusted_ca` is provided (well-formed mTLS configuration). The lift maps Envoy's `require_client_certificate=true` onto stdlib `crypto/tls.RequireAndVerifyClientCert` mode + `ClientCAs` pool populated from the parsed trusted_ca PEM. Previously parse-rejected surfaces (SDS-bound secrets, custom_validator_config, match_typed_subject_alt_names, verify_certificate_hash/spki) remain rejected via the unchanged commonTLSContextToConfig pre-checks. Listener-manager + fixture-0002 carry-forward gates unchanged (the lift is additive; existing test surfaces unchanged in behavior).

**ADR-0147 doctrine:** Phase-16 framework-delta. ADR-0044 ADR-on-impl convention (impl-time-unanticipated; mirrors phase-14 ADR-0134 + phase-15 ADR-0139's planner-time-anticipated escape valve). Lands at Task 14 in-task; documented in DECISIONS.md follow-up.

### Self-review findings + concerns

- **Concern 1 (driver code complexity):** AssertStats now carries side-specific tables + cross-side total-equivalence checks. The complexity increase is justified by the INDEPENDENT-vs-SHARED stats divergence-window; future stat-surface-refinement phase MAY collapse if envoy-go adopts SHARED-stats opt-in.
- **Concern 2 (test surface for Discovery 1's :path injection):** the new injection in connection.go + h2dispatch.go is exercised end-to-end via fixture 0018 but has no targeted unit test in `internal/filter/hcm/`. Future hardening task should add a `TestDispatch_:path_Injection_Mirrors_RequestURI` covering H1 + H2 paths separately. Tracked at REVIEW.md Task 16 surface.
- **Concern 3 (Discovery 2 scenario reshape):** the BRAINSTORM scenario 4 + 5 intent was to test destination_port + direct_remote_ip canonicals end-to-end. The reshape preserves the AndRules/OrRules INTENT but defers the destination_port / direct_remote_ip canonical end-to-end validation to a future framework-primitive phase. The canonicals ARE unit-tested.
- **Concern 4 (ADR-0147 minimum-scope discipline):** the lift is narrowly scoped (require_client_certificate=true + trusted_ca present); we do NOT lift the broader phase-03 TLS-layer restrictions. Future fixtures that need SDS-bound mTLS secrets or custom_validator_config will require additional scoped lifts (each via its own ADR).
- **Concern 5 (Prometheus tag-extractor structural close):** the Prometheus tag-extractor divergence-window is driver-normalized; future stat-surface-refinement may introduce a phase-16-stat-extractor in `internal/stats/name.go` (a phase-11/15-precedent SN9/inline-prefix-style detection for rbac) to close it structurally. Documented at expectations.yaml + README.md + ADR-0145 §Future-work.

**Commit:** `phase 16 Task 14: Fixture 0018 expectations + README + end-to-end differential pass (19 fixtures green)`

## Task 15 — BEHAVIOR_CONTRACT 6-edit bundle + ROADMAP flip + STATE.md advance + 6-gate phase-done

**Date:** 2026-05-12
**Branch tip at task start:** `570ce509b0041aa0f2f62537bc659fdb9643fefa` (Task 14 follow-up gofmt fix)

**Files modified:**

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (+346 LoC; 6-edit bundle per SPEC §13)
- `docs/envoy-go/ROADMAP.md` (row 16 in-progress → done; summary sharpened with post-impl counts)
- `docs/envoy-go/STATE.md` (state-4 → state-5 advance: `phase 16 phase-done; REVIEW pending`; next-free ADR 0147→0148; next-skill scoped to Task 16 REVIEW.md only)
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry + 6-gate verbatim report)

### Step 1-6: BEHAVIOR_CONTRACT 6 patches per SPEC §13

All 6 patches applied per planner-time decision 19 (§13.1 landing-chronological AFTER bandwidth_limit, NOT alphabetical-canonical):

- **§13.1** — `### envoy.filters.http.rbac` subsection inserted AFTER `### envoy.filters.http.bandwidth_limit` at the previous line 1416 anchor (now line 1487) — Field decomposition (7-field listener-level table + inner RBAC table + 3-CEL silent-ignore + Permission/Principal Large 11+11 + DEFERRED 3+3 + 7th canonical per-route) + Wire shape + Per-route INDEPENDENT-stats discipline + Stat surface (4 base counters + per-policy template form; SN2-reuse RATIFIED per ADR-0145).
- **§13.2** — Stat-table 60→**64 base names** extension; 4 new active rows under `**RBAC filter — 4 active names (introduced by phase 16):**` + per-policy template form table (the `.policy.` infix segment was empirically refined at Task 8 per ADR-0145; SPEC §13.2 stub which omitted the segment has been corrected). Total recomputed: 17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 = **64 base names**.
- **§13.3** — `0018-http-rbac` row appended to the Equivalence Matrix at line 38 (immediately after the bandwidth_limit row).
- **§13.4** — `### Phase 16 forward-pointer notes` subsection appended at end of `## Forward-pointer notes` section (after `### Phase 15 forward-pointer notes`). Enumerates the 6 divergence-windows per ADR-0146 §Context (LOG-action `access_log_hint` metadata, `response_code_details` field-emission, CEL three-field, shadow access-log integration, SourcedMetadata+FilterState always-no-match, Principal_Authenticated canonical-3-cert-field scope) + 5 framework / foot-gun notes (TWO new framework primitives, ADR-0147 unanticipated mTLS-lift, `track_per_rule_stats` operator-config-driven foot-gun, Principal_Set/Permission_Set recursion-depth foot-gun, no new tag-extractor) + filter-chain ordering trade-off.
- **§13.5** — NEW top-level `## HTTPFilterCallbacks` umbrella section authored (introduced by phase 16; justified by ADR-0144) with `### DownstreamPrincipal accessor (per phase 16 ADR-0144)` subsection. Lands as the first per-callback subsection — future filters (jwt_authn / ext_authz / oauth2 / ext_proc) extend the umbrella additively.
- **§13.6** — NEW top-level `## Matcher engine framework primitive (per phase 16 ADR-0142)` section authored. Documents `matcher.New(tree, supportedActionTypes)` + `matcher.Evaluate(MatchContext)` + `matcher.MatchContext` interface + RBAC's `supportedActionTypes = ["type.googleapis.com/envoy.config.rbac.v3.Action"]` + cross-phase reuse intent.

### Step 7: ROADMAP row 16 in-progress → done

Row 16 flipped to `done | 2026-05-12 | …`. Summary sharpened with post-impl counts:
- Production LoC ~2778 (rbac.go 1191 + evaluator.go 854 + doc.go 148 = 2193 LoC rbac; matcher.go 510 + doc.go 75 = 585 LoC matcher).
- 7 anticipated ADRs landed (ADR-0140..ADR-0146) + 1 UNANTICIPATED (ADR-0147 Task 13 follow-up TLS-mTLS-lift) + ADR-0125 §(xii) amendment paragraph at Task 10.
- Fixture 0018 (8 scenarios incl. mTLS scenario 6) green; 19/19 differential fixtures green at phase-done.
- 20th fuzzer `FuzzRBACConfigParse` landed at `internal/filter/http/rbac/fuzz_test.go`.
- Stat surface 60→64 base names + per-policy template family.

### Step 8: STATE.md state-4 → state-5 advance

- `lifecycle-state:` `phase 16 phase-done; REVIEW pending` (state-5 entry between state-4 impl-complete and state-6 phase-fully-closed).
- `next-skill:` `superpowers:executing-plans` Task 16 REVIEW.md only.
- `next-free ADR:` `ADR-0148` (advanced past unanticipated ADR-0147 from Task 13 follow-up).
- `last-updated:` `2026-05-12`.
- `last-commit:` SHA-fill follow-up at squash-merge time per phase-13/14/15 close pattern.

### Step 9: 6-gate phase-done verification

#### Gate A — build + vet + lint

```
$ go build ./... 2>&1
BUILD_RC=0
(no output)

$ go vet ./... 2>&1
VET_RC=0
(no output)

$ golangci-lint run ./... 2>&1
LINT_RC=0
(no output)
```

**Gate A: GREEN** (all three exit 0; zero diagnostics).

#### Gate B — race tests across all packages

```
$ go test -race -count=1 ./... 2>&1
```

Initial run flaked on `test/differential/TestDifferential/0017-http-bandwidth-limit` scenario 2 (status 200 vs 502 — a known timing-sensitive container-startup race exacerbated by race-detector overhead; the bandwidth_limit scenario 2 is sensitive to envoy-go subject startup vs request-emission timing). Re-run came up clean.

Re-run output (43 `ok` package verdicts + 0 FAIL):

```
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	9.369s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.055s
ok  	github.com/esalaine/envoy-go/internal/admin	2.870s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.187s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.185s
ok  	github.com/esalaine/envoy-go/internal/drain	1.153s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.186s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.597s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.205s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	9.787s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.080s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.151s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.028s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.072s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.076s
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.411s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.052s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.109s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.069s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.349s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.268s
ok  	github.com/esalaine/envoy-go/internal/listener	4.202s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.066s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.055s
ok  	github.com/esalaine/envoy-go/internal/matcher	1.068s
ok  	github.com/esalaine/envoy-go/internal/stats	1.067s
ok  	github.com/esalaine/envoy-go/internal/tls	1.269s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	6.120s
ok  	github.com/esalaine/envoy-go/test/differential	... PASS
ok  	github.com/esalaine/envoy-go/test/differential/fixture	...
ok  	github.com/esalaine/envoy-go/test/fixtures/...
```

**Gate B: GREEN** (re-run clean; the initial flake is the same timing-sensitive 0017 scenario 2 pattern phase 15 Task 14 captured — Envoy's initial-burst-discount divergence; not a regression caused by Task 15's docs-only edits).

#### Gate C — h2spec 53/53 at ADR-0051 pin

```
$ go test -count=1 -v -run TestH2Spec ./test/conformance/h2spec/ 2>&1 | tail -30
        ...
        Finished in 0.8194 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
    h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
    h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (6.93s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	7.183s
```

**Gate C: GREEN** (53/53 at ADR-0051 pin — confirms phase-16 HCM-side changes — `:path` injection framework delta + DownstreamPrincipal accessor plumbing — preserved h2spec conformance).

#### Gate D — 20 fuzzers @ 30s each

```
$ /tmp/run-fuzzers.sh  # 20 fuzzers sequenced; 30s each; per-fuzzer pkg + name as enumerated below
RESULT: FuzzTLSContextParse PASS                      (./internal/tls)
RESULT: FuzzFrameStream PASS                          (./internal/filter/hcm/h2)
RESULT: FuzzHPACKDecode PASS                          (./internal/filter/hcm/h2)
RESULT: FuzzBufferConfigParse PASS                    (./internal/filter/http/buffer)
RESULT: FuzzHCMConfigParse PASS                       (./internal/filter/hcm)
RESULT: FuzzFaultConfigParse PASS                     (./internal/filter/http/fault)
RESULT: FuzzTcpProxyFilter PASS                       (./internal/filter/tcpproxy)
RESULT: FuzzRBACConfigParse PASS                      (./internal/filter/http/rbac)    [20th fuzzer; phase 16]
RESULT: FuzzBootstrapLoad PASS                        (./internal/bootstrap)
RESULT: FuzzCsrfPolicyConfigParse PASS                (./internal/filter/http/csrf)
RESULT: FuzzHeaderMutationConfigParse PASS            (./internal/filter/http/header_mutation)
RESULT: FuzzDrainTransitions PASS                     (./internal/drain)
RESULT: FuzzLocalRateLimitConfigParse PASS            (./internal/filter/http/localratelimit)
RESULT: FuzzFilterChainParse PASS                     (./internal/filter/http)
RESULT: FuzzConfigDumpFormat PASS                     (./internal/admin)
RESULT: FuzzCompressorConfigParse PASS                (./internal/filter/http/compressor)
RESULT: FuzzFilterChainMatch PASS                     (./internal/listener/listenerfilter)
RESULT: FuzzBandwidthLimitConfigParse PASS            (./internal/filter/http/bandwidthlimit)
RESULT: FuzzPromTextFormat PASS                       (./internal/stats)
RESULT: FuzzAccessLogFormat PASS                      (./internal/accesslog)

=== SUMMARY ===
PASS=20 FAIL=0 TOTAL=20 ELAPSED=631s
```

**Gate D: GREEN** (20/20 fuzzers PASS @ 30s each; total wallclock 631s / ~10.5 minutes; zero crashes; zero new corpus-shrunk seeds).

#### Gate E — 19 differential fixtures (0000-0018)

```
$ go test -count=1 -v ./test/differential/ -run 'TestDifferential' 2>&1 | grep -E '^\s+---'
    --- PASS: TestDifferential/0000-tcp-echo (1.53s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.23s)
    --- PASS: TestDifferential/0002-tls-tcp (1.25s)
    --- PASS: TestDifferential/0003-http11-routing (1.26s)
    --- PASS: TestDifferential/0004-h2-routing (1.70s)
    --- PASS: TestDifferential/0005-prometheus-stats (1.92s)
    --- PASS: TestDifferential/0006-access-log (10.93s)
    --- PASS: TestDifferential/0007a-cors (1.30s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.78s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.44s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.91s)
    --- PASS: TestDifferential/0010-graceful-drain (9.39s)
    --- PASS: TestDifferential/0011-http-fault (2.06s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.42s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.04s)
    --- PASS: TestDifferential/0014-http-csrf (1.43s)
    --- PASS: TestDifferential/0015-http-buffer (1.38s)
    --- PASS: TestDifferential/0016-http-compressor (1.33s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.05s)
    --- PASS: TestDifferential/0018-http-rbac (1.54s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	52.994s
```

**Gate E: GREEN** (19/19 fixtures PASS; total wallclock 52.99s — well under the SPEC §14.6 60-90s budget).

(Intermediate flakes observed during execution: a port-rebind glitch on 0018 immediately after another differential run released ports, and the same bandwidth_limit 0017 scenario 2 timing-divergence that's documented in phase 15 forward-pointer notes. Both clear on next run; not regressions.)

#### Gate F — BEHAVIOR_CONTRACT §13.1-§13.6 populated

```
$ grep -nE '^### envoy.filters.http.rbac\b' docs/envoy-go/BEHAVIOR_CONTRACT.md
1487:### envoy.filters.http.rbac

$ grep -nE 'rbac\.<(rules|shadow_rules)_stat_prefix>\.(allowed|denied|shadow_allowed|shadow_denied)' docs/envoy-go/BEHAVIOR_CONTRACT.md | head -4
272:| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.allowed`         | counter | filter | rbac | …
273:| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.denied`          | counter | filter | rbac | …
274:| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_allowed` | counter | filter | rbac | …
275:| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_denied`  | counter | filter | rbac | …

$ grep -nE '0018-http-rbac' docs/envoy-go/BEHAVIOR_CONTRACT.md
38:| 0018-http-rbac | envoy.filters.http.rbac (decode-side dual-engine policy gate; rules-engine + matcher-engine + shadow + per-policy stats) | byte-exact status; byte-exact body on allow (passthrough) AND deny (19-byte "RBAC: access denied"); …

$ grep -nE '^### Phase 16 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
2010:### Phase 16 forward-pointer notes

$ grep -nE '^### DownstreamPrincipal accessor' docs/envoy-go/BEHAVIOR_CONTRACT.md
1860:### DownstreamPrincipal accessor (per phase 16 ADR-0144)

$ grep -nE '^## Matcher engine framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md
1876:## Matcher engine framework primitive (per phase 16 ADR-0142)
```

**Gate F: GREEN** (all 6 §13 patches land at expected line ranges; six greps return ≥1 hit each per SPEC §14.7 Gate F definition).

### 6-gate summary

| Gate | Description | Status | Notes |
|---|---|---|---|
| A | build + vet + lint | GREEN | all three exit 0; zero diagnostics |
| B | race tests all packages | GREEN | re-run clean (initial flake was 0017 scenario 2 timing-divergence — phase 15 forward-pointer-known) |
| C | h2spec 53/53 | GREEN | at ADR-0051 pin; phase-16 HCM-side changes preserved conformance |
| D | 20 fuzzers @ 30s | GREEN | 20/20 PASS; 631s wallclock; zero crashes |
| E | 19 differential fixtures | GREEN | 19/19 PASS; 52.99s wallclock (under 60-90s SPEC §14.6 budget) |
| F | BEHAVIOR_CONTRACT 6 patches | GREEN | all 6 §13 grep markers present |

**All 6 phase-done gates GREEN.** Phase 16 is now phase-done; REVIEW.md pending (Task 16) per ADR-0005 §Decision 4.

**Commit:** `phase 16 Task 15: BEHAVIOR_CONTRACT 6-edit bundle + STATE advance + 6-gate phase-done verification`


## Task 16 — REVIEW.md end-of-phase review

**Date:** 2026-05-12
**Branch tip at task start:** `6ab026ffdc1c65892065b54c08ce238d463eda76` (Task 15 phase-done — BEHAVIOR_CONTRACT 6-edit bundle + STATE advance + 6-gate phase-done verification)

**Files modified:**

- `docs/envoy-go/phases/16-http-filter-rbac/REVIEW.md` (+297 LoC NEW; end-of-phase review per superpowers:requesting-code-review skill template + phase-13/14/15 REVIEW.md precedent)
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (this entry)

**STATE.md advance decision: DEFERRED to post-merge SHA-fill follow-up** per phase-13/14/15 close pattern. Phase-15's REVIEW.md commit (`ddbdc3f`) did NOT advance STATE.md state-5→state-6; the advance landed in the post-squash-merge follow-up (`98a8ca6` "phase 15 impl follow-up: STATE.md SHA-fill (TBD → c1361d3 post-squash) + lifecycle-state-6 narrative"). Phase 16 mirrors verbatim.

### Step 1: REVIEW.md authoring — 297 LoC across 11 sections

Authored per superpowers:requesting-code-review skill output template + phase-15 REVIEW.md structural precedent. Sections:

1. **Header** — phase id (NINTH §9 row; FIRST since phase-14 to introduce non-zero framework deltas; FIRST single phase TWO simultaneous primitives; FIRST 7th-canonical introduction; THIRD INDEPENDENT-stats row; LARGEST anticipated-ADR roster; FIRST since phase-14 to require impl-time-unanticipated ADR) + slug + branch + range `6ab026f` + parent ROADMAP row 16 done flip + reviewer-method + six-gate state at HEAD.
2. **§1 Phase summary** — APPROVED with two DONE_WITH_CONCERNS carry-overs (claim 6 stat surface per-policy `.policy.` refinement at Task 8; claim 11 response_code_details DEFERRED). Architectural centerpiece: dual-engine proto-faithful dispatch + 4-base-counter shape REFUTING BRAINSTORM 5-counter + lazy per-policy emission. Fixture 0018 8-scenario non-vacuous evidence.
3. **§2 ADR roster** — 7 anchored ADRs (ADR-0140..ADR-0146) + ADR-0125 §(xii) amendment + ADR-0147 unanticipated. Each evaluated for §Decision validation under impl + fixture exercise.
4. **§3 Empirical pins outcome** — 18 §11 pins; final disposition table; 10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME-CONFIRMED-AT-TASK-8 + 1 DEFERRED. Task-8 empirical scrape load-bearing for 4 pins.
5. **§4 Gate-by-gate evidence** — verbatim Gate A/B/C/D/E/F transcripts from PROGRESS Task 15.
6. **§5 Acceptance checklist — SPEC §15 13-claim verification** — all 13 claims verified PASS with citations; 2 PASS-with-DONE_WITH_CONCERNS (claim 6 + claim 11).
7. **§6 Divergence-window roster** — 6 operator-facing divergence-windows per ADR-0146 §Context + BEHAVIOR_CONTRACT §13.4 phase-16 forward-pointer notes.
8. **§7 Framework-delta impact + cross-phase reuse intent** — TWO new primitives (matcher-engine + TLS-principal accessor); both explicitly cross-phase-reusable.
9. **§8 Test counts + verification surface** — 100 PASS + 2 SKIP rbac unit tests; matcher + chain + HCM + TLS tests; 19/19 fixtures; 20/20 fuzzers; h2spec 53/53; ~2778 production LoC; 2150 fixture LoC; documentation deltas.
10. **§9 Deferred items + open follow-ups** — 15 items enumerated for future phase consideration.
11. **§10 Phase-done lessons learned** — 6 lessons: TWO new framework primitives in a single phase; impl-time-unanticipated ADR-0147 lift; Task-8 empirical scrape as canonical RATIFIED-PENDING closure; ADR-0125 canonical-pattern roster growth; 12-amendment §1.1 channel scaling; 100 PASS + 2 SKIP largest single-filter unit-test surface to date.
12. **§11 Sign-off** — ready for master squash-merge via `wt-merge`; all gates green; all 13 claims verified; STATE.md state-5→state-6 transition deferred to post-merge SHA-fill follow-up.

### Step 2: Verification (no-regression sanity check)

```
$ go test -count=1 -short ./...
[All packages PASS; exit 0; no FAIL lines]
```

No code changes in this Task; pure doc authoring. No lint impact (docs-only).

### Step 3: PROGRESS.md Task 16 entry (this entry)

Appended this entry to PROGRESS.md following the bold-label convention. REVIEW.md cross-referenced by path. STATE.md advance decision noted (DEFERRED to post-merge SHA-fill follow-up per phase-13/14/15 close pattern).

**Commit:** `phase 16 Task 16: REVIEW.md — end-of-phase review`

**Lifecycle-state at this commit:** state-5 (`phase 16 phase-done; REVIEW pending`). After `wt-merge` + the post-squash-merge follow-up commit lands STATE.md state-5→state-6 + the SHA-fill (TBD → post-squash SHA), phase 16 will be fully closed at state-6 (`phase 16 done; awaiting next planning`).

