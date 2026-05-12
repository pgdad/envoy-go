# Phase 16 — HTTP filter `envoy.filters.http.rbac` (`internal/filter/http/rbac/`, NEW top-level `internal/matcher/` framework primitive, NEW `DecoderFilterCallbacks.DownstreamPrincipal() []string` framework primitive, differential fixture `0018-http-rbac` with 8 scenarios including mTLS, `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.rbac` extension + `## Stat-name mapping` 60→64 extension + `## HTTPFilterCallbacks ### DownstreamPrincipal accessor` extension + NEW `## Matcher engine framework primitive` section, **TWO framework deltas** — FIRST §9 row since phase 14 to introduce non-zero deltas + FIRST single phase to introduce TWO simultaneously) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory user preference) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.rbac` — Envoy v1.37.2's canonical role-based-access-control filter, BOTH-engine MVP (rules-engine + matcher-engine, proto-faithful per SPEC Q2 + §1.1 amendment 2), ALLOW + DENY + LOG-partial action enum (per §1.1 amendment 5 LOG-partial = always-allow + match-evaluated + `access_log_hint` metadata silent + `allowed` counter increments per amendment 8 — no separate `logged` counter exists in Envoy v1.37.2), decode-side policy gate — as the NINTH production HTTP filter in envoy-go, with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis EXCEPT (a) the LOG-action `access_log_hint` dynamic-metadata emission (envoy-go MVP: silent; Envoy: emits — divergence-window per §1.1 amendment 5 + §8.6 deferral coupling to dynamic-metadata family) AND (b) the `response_code_details` field emission on DENY (envoy-go MVP: no emission; Envoy: emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"` per §1.1 amendment 11 + §8.12 deferral coupling to HCM response-code-details framework phase) AND (c) the CEL three-field condition evaluation (envoy-go MVP: silent-ignored per Q7 + §1.1 amendment 6; Envoy: full evaluation via CEL runtime — divergence-window per §8.1 deferral coupling to future CEL framework phase) AND (d) the shadow-rules access-log integration (envoy-go MVP: counter-only; Envoy: counter-only confirmed at SPEC §11.P13 — no current divergence but documented as forward-pointer per §8.7) AND (e) the sourced-metadata + filter-state runtime always-no-match (envoy-go MVP: always FALSE; Envoy: actual evaluation against dynamic-metadata + filter-state subsystems — divergence-window per §2.5 + §8.10 deferrals; in practice no divergence under fixture-baseline default-empty conditions per §11.P15), under the 07.1 framework with **TWO new framework deltas** (FIRST §9 row since phase 14 to introduce non-zero deltas + FIRST single phase to introduce TWO simultaneously per SPEC §3): (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor returning the priority-ordered TLS principal-name candidates (URI SANs → DNS SANs → Subject DN CN per §1.1 amendment 12 + §11.P14) — anchored at ADR-0144; (ii) matcher-engine evaluator framework primitive at NEW top-level `internal/matcher/` package implementing `xds.type.matcher.v3.Matcher` generic match-tree evaluator with `New(tree, supportedActionTypes []string)` + `Evaluate(MatchContext)` API + parse-time PARSE-REJECT for unknown terminal TypeURLs (canonical RBAC `Action` only in MVP per §11.P3 + §2.6) — anchored at ADR-0142; both primitives cross-phase reusable by future filters (ext_authz, jwt_authn, ext_proc, oauth2, lua, wasm, adaptive_concurrency, admission_control, global_ratelimit).

**Architecture:** New `internal/filter/http/rbac/` package owning the filter implementation; DECODER-only `HTTPFilter` value (mirrors phase-12 csrf ADR-0120 + phase-13 buffer ADR-0125 decoder-only precedent — rbac is a pre-body request gate; `Encoder: nil`); `compiledConfig` shape with primary engine (`rules` xor `matcher`; rules wins when both set per SPEC §1.1 amendment 2 + §6.2) + shadow engine (`shadowRules` xor `shadowMatcher`) + stat namespacing (`rules_stat_prefix` + `shadow_rules_stat_prefix` empty-allowed per §1.1 amendment 3 + `track_per_rule_stats` bool) + 4-base-counter `filterStats` (`allowed`, `denied`, `shadow_allowed`, `shadow_denied` per §1.1 amendment 8 + §11.P6 — REFUTES BRAINSTORM 5-counter hypothesis; LOG-partial folds into `allowed` since LOG always-allows) + lazy per-policy counter sub-cache (allocated on first match when `track_per_rule_stats: true`; `sync.Map` keyed by `<policy_name>.<suffix>` via `NewCounterIfAbsent` post-Freeze idempotent registration per ADR-0117 + ADR-0139 precedent); Permission + Principal Large 11+11 evaluator surface (per SPEC §1 item 3 + §6.5 + ADR-0143) — 11 Permission variants (`any`, `header`, `url_path`, `destination_ip`, `destination_port`, `destination_port_range`, `requested_server_name`, `and_rules`, `or_rules`, `not_rule`, `sourced_metadata` with always-no-match runtime per §2.5) + 11 Principal variants (`any`, `authenticated` with three-case algorithm per §1.1 amendment 12 + ADR-0144 — case (a) nil principal_name matches ANY authenticated user via `len(DownstreamPrincipal()) > 0`; case (b) non-nil StringMatcher iteration over URI SAN/DNS SAN/Subject DN candidates; case (c) plaintext connection → FALSE; `direct_remote_ip`, `remote_ip`, `header`, `url_path`, `and_ids`, `or_ids`, `not_id`, `sourced_metadata` always-no-match, `filter_state` always-no-match) — 3 Permission deferred (`metadata` deprecated; `matcher` extension; `uri_template` extension — PARSE-REJECT envoy-go-only) + 3 Principal deferred (`source_ip` deprecated; `metadata` deprecated; `custom` extension — the 14th Principal variant per §1.1 amendment 7 NEW finding; PARSE-REJECT envoy-go-only); rules-engine path walks `policies` map in lexicographic order, OR-semantic permissions + OR-semantic principals + ALLOW/DENY/LOG action apply (per SPEC §6.9 `evaluateRulesEngine`); matcher-engine path delegates to `internal/matcher.Matcher.Evaluate()` returning the matched terminal `Any` (canonical `envoy.config.rbac.v3.Action` per §11.P3) — match-tree-no-match → DENY per `rbac.pb.go:43-46` proto comment; shadow path runs the parallel engine walk on `shadowRules`/`shadowMatcher` (same algorithm; never affects disposition; emits `shadow_allowed` / `shadow_denied` counters per ADR-0146); `DecodeHeaders` resolves per-route TPFC via `dcb.RequestRouteConfig()` → caches effective `*compiledConfig` on `f.activeRC` + sets `f.passthrough` flag if per-route disabled OR both engines unset → builds `evalContext` from headers + connection accessors → runs primary engine `evaluateEngine()` → runs shadow engine if configured → emits primary counters + applies disposition (`HeaderContinue` on ALLOWED; `cb.SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain})` + `HeaderStopIteration` on DENIED — wire shape per SPEC §4 + §1.1 amendment 10 + §11.P5 — 19 bytes ASCII body verbatim from `source/extensions/filters/http/rbac/rbac_filter.cc::sendLocalReply` invocation, 4-header set lowercase wire-form `content-length: 19, content-type: text/plain, date: <RFC1123>, server: envoy`, keep-alive); `DecodeData` + `DecodeTrailers` pass-through; `OnDestroy` no-op (decode-side gate with no timers; mirrors phase-12 csrf precedent); per-route `RBACPerRoute` wrapper proto with reserved field 1 + single optional `rbac` field at field 2 — phase-16 introduces the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override per ADR-0125 §(xii) amendment paragraph anchored at impl-time Task 10 per phase-15 SPEC-deferred-amendment convention applied to PLAN-time — see Task 10 + planner-time decision 14): case (a) `RBACPerRoute{rbac: nil}` or wrapper-itself-absent → `compiledPerRoute{disabled: true, overrideConfig: nil}` (filter wholly inactive on route; no counter increments); case (b) `RBACPerRoute{rbac: <RBAC>}` → wholesale-override per ADR-0073 (recursive `buildCompiledConfig` produces independent `*compiledConfig` with own `*filterStats` keyed by per-route `rules_stat_prefix` — INDEPENDENT per-route stats per ADR-0145 mirrors phase-11 ADR-0117 + phase-15 ADR-0139 stateful-override-implies-INDEPENDENT precedent; phase-16 rbac is THIRD row using this discipline after phase-11 local_ratelimit + phase-15 bandwidth_limit); 4-base-counter stat surface 60→64 names (4 new active rows; per-policy template-form documented separately per ADR-0145 + SPEC §13.2) under stat namespace SN2-reuse hypothesis `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` per §1.1 amendment 9 + §11.P7 (impl-time empirical scrape confirms or amends; no new SN10 rule pending) + lazy per-policy counter family `<base>.<policy_name>.<suffix>` (4 suffixes per policy per active prefix; operator-config-driven surface growth foot-gun documented at BEHAVIOR_CONTRACT §13.4); differential fixture `0018-http-rbac` THREE-listener topology (`l_test_a` plaintext + `l_test_b` echo-backend + `l_test_a_tls` mTLS-required) with 8 scenarios per SPEC §7 covering: scenario 1 allow-by-header-match (ALLOW + match), scenario 2 deny-no-match (ALLOW + no-match) producing 403 + 19-byte body, scenario 3 allow-by-url-path, scenario 4 allow-by-destination-port, scenario 5 allow-by-direct-remote-ip, **scenario 6 mTLS allow-by-TLS-principal** (exercises ADR-0144 framework primitive; client cert URI SAN `spiffe://example.com/admin` matched via Principal_Authenticated), **scenario 7 per-route 7th-canonical disabled** (`RBACPerRoute{}` empty → bypass), **scenario 8 per-route wholesale-override with INDEPENDENT stat namespace + shadow** (per-route DENIES guests with own `rules_stat_prefix: override` + own `shadow_rules_stat_prefix: override_shadow`; listener-level `default.*` counters UNCHANGED), byte-exact body assertion on ALL scenarios (allow paths passthrough verbatim; deny paths 19-byte `RBAC: access denied`), per-counter delta byte-equivalence on the 4 base counters per active namespace.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module + v1.37.0 cross-check (proto pin per ADR-0008; phase-16 SPEC §11 used both for cross-version sanity verification); `protojson.Unmarshal` for proto decoding; `crypto/x509` + `crypto/tls` Go stdlib for the mTLS scenario 6 PKI generation + client cert handshake; `cncf/xds/go` module at `v0.0.0-20251110193048-8bfbf64dc13e` for the `xds.type.matcher.v3.Matcher` proto bindings (per SPEC §11 verification); `sync.Map` for per-route lazy-cache (reuse from phase-11 ADR-0117 + phase-15 ADR-0139 precedent); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (per ADR-0008 + ENVOY_TARGET.md); golangci-lint 1.64.8 (ADR-0009 pin); Docker for differential harness; HTTP/1.1 plaintext fixture for scenarios 1-5, 7, 8 + HTTP/1.1-over-mTLS for scenario 6 (no H2 differential coverage per SPEC §7.4); existing PathMatcher / HeaderMatcher / StringMatcher / CidrRange evaluators from phase-07.1 cors (shared infrastructure for Permission/Principal leaf matchers per SPEC §3 reuse table).

---

## Scope check — why phase 16 ships as one row (not split)

Net change estimate (mirroring the 06.1 / 06.2 / 07.1 / 07.2 / 08.1 / 08.2 / 09 / 10 / 11 / 12 / 13 / 14 / 15 PLAN's component-table convention):

- `internal/filter/http/rbac/doc.go` ~30
- `internal/filter/http/rbac/rbac.go` ~350–450 (filter + factory + types + DecodeHeaders body + DecodeData + DecodeTrailers + OnDestroy + SetDecoderCallbacks + compiledConfig + compiledRulesEngine + compiledMatcherEngine + compiledPolicy + compiledPerRoute + filterStats + parsePerRoute + resolvePerRouteConfig + buildCompiledConfig + buildCompiledRulesEngine + buildCompiledMatcherEngine + buildCompiledConfigPerRoute + newFilterStats + newFilterStatsIfAbsent + emitPrimaryCounters + emitShadowCounters + evalContext implementation on `*filter` + matcherCtxAdapter)
- `internal/filter/http/rbac/evaluator.go` ~650–850 (Permission Large 11 evaluator types + buildPermissionEvaluators + buildOnePermission switch + Principal Large 11 evaluator types + buildPrincipalEvaluators + buildOnePrincipal switch + AND/OR/NOT combinators with recursive descent + prinAuthenticated three-case algorithm per §1.1 amendment 12 + permission/principal evaluator interfaces + StringMatcher / CidrRange / HeaderMatcher / PathMatcher shared-infrastructure adapters from phase-07.1 + matchString helper + matchCidr helper + evaluateEngine + evaluateRulesEngine + evaluateMatcherEngine + policyMatches)
- `internal/filter/http/rbac/rbac_test.go` ~900–1100 (8 unit-test groups per SPEC §14.1 + 1 planner-time-emerging Group 9 stats-namespace integration sub-group; ~70-90 test cases total)
- `internal/filter/http/rbac/fuzz_test.go` ~80 (20th fuzzer in repo: `FuzzRBACConfigParse`; mirrors phase-15 + phase-14 fuzzer shape extended to the 7-field outer RBAC + nested rules-engine config + nested matcher-engine tree)
- `internal/matcher/doc.go` ~25 (NEW top-level package; cross-phase-reusable per ADR-0142 §Decision (i))
- `internal/matcher/matcher.go` ~150–250 (NEW top-level package; `Matcher` type + `New(tree, supportedActionTypes)` constructor + `Evaluate(MatchContext)` walker + `MatchContext` interface + parse-time PARSE-REJECT for unknown terminal TypeURLs + initial predicate evaluator subset scoped to RBAC's canonical surface — header predicate + path predicate + IP predicate + AND/OR predicate combinators per match-tree-of-match shape; cross-phase reuse intent codified at ADR-0142 §Decision (i)+(ii); future filters extend `supportedActionTypes` + widen `MatchContext` additively)
- `internal/matcher/matcher_test.go` ~150–250 (canonical RBAC Action terminal parsing + unknown TypeURL rejection + match-tree walk + no-match disposition + MatchContext adapter; ~12-18 test cases)
- `internal/filter/http/callbacks.go` +1 LoC (`DownstreamPrincipal() []string` interface method on `DecoderFilterCallbacks`)
- `internal/filter/http/chain.go` ~+30 LoC (`decoderCB.DownstreamPrincipal` impl plumbing from `chain.tlsPrincipals` per-stream field set by HCM dispatch at chain build time; new `c.tlsPrincipals []string` per-stream field; new `chain.SetTLSPrincipals(p []string)` accessor for HCM-side wiring)
- `internal/filter/http/chain_test.go` ~+60 LoC (probe-filter-driven DownstreamPrincipal integration tests covering: probe reads accessor → returns the seeded principals; no seed → returns nil; cross-phase probe integration)
- `internal/filter/hcm/connection.go` ~+25 LoC (H1 dispatch: extract `tls.ConnectionState` from connection's `*tls.Conn` if `conn.ConnectionState().HandshakeComplete && len(conn.ConnectionState().PeerCertificates) > 0`; build priority-ordered principal list URI SAN → DNS SAN → Subject DN CN; thread into `chain.SetTLSPrincipals(principals)` at chain-build time before `RunDecodeHeaders` dispatch)
- `internal/filter/hcm/h2dispatch.go` ~+15 LoC (H2 dispatch: symmetric to H1 path; TLS state extraction at per-stream chain construction)
- `cmd/envoy-go/main.go` +1 import line + 1 register line ~+3 (`httpReg.Register(rbac.TypeURL, rbac.New)` inserted alphabetical-after-localratelimit per ADR-0100 §2.2 + ADR-0140 §Decision; resulting block reads `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → localratelimit → rbac → Freeze`)
- `test/fixtures/0018-http-rbac/` (NEW DIRECTORY) — `envoy.yaml` ~165 + `envoy-go.yaml` ~165 + `expectations.yaml` ~75 + `README.md` ~110 + `inputs/driver.go` ~290 + `pki/gen.go` ~120 (mTLS PKI generation: fixture-CA cert + server cert + client cert with URI SAN `spiffe://example.com/admin`; per planner-time decision 11) = ~925
- `test/differential/fixture/fixture.go` new `BackendKind` enum value (`HTTPRbac BackendKind = 15`) + doc-comment ~+15
- `test/differential/runner_test.go` blank-import addition + new switch-case for `HTTPRbac` reuses the existing `startEchoBackend` spawn helper from phase-14 Task 10 ~+10
- `docs/envoy-go/DECISIONS.md` 7 ADRs (ADR-0140..ADR-0146) authored at impl-time per ADR-0044 ADR-on-impl convention; ADR-0125 amendment §(xii) authored at Task 10 per planner-time decision 14 (NOT pre-landed at SPEC commit despite SPEC §5.4 prose claim — see planner-time decision 14 for the disposition); ~+700 LoC across the 7 new ADRs + ~+30 LoC for the §(xii) amendment paragraph
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` per SPEC §13 patches — §13.1 `### envoy.filters.http.rbac` subsection ~230 + §13.2 stat-table 60→64 names extension (4 new active rows + per-policy template-form documentation) ~30 + §13.3 equivalence-matrix row ~3 + §13.4 `### Phase 16 forward-pointer notes` subsection ~100 + §13.5 `## HTTPFilterCallbacks` extension with `### DownstreamPrincipal accessor` subsection ~25 + §13.6 NEW `## Matcher engine framework primitive` section ~20 = ~+410 LoC
- `docs/envoy-go/ROADMAP.md` row `16` status flip `in-progress → done` + summary sharpening (post-PLAN counts; 16 tasks + ~1240-1640 LoC production estimate; final ADR roster 7 anchored + 1 amendment) ~+1 net
- `docs/envoy-go/STATE.md` advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place
- `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (NEW; lifecycle artefact) ~800
- `docs/envoy-go/phases/16-http-filter-rbac/REVIEW.md` (NEW; lifecycle artefact) ~250

**Production code: ~1030–1330 LoC (filter impl in `rbac.go` + `evaluator.go` + `doc.go`) + ~175–275 LoC framework deltas across 6 files (`internal/matcher/{doc.go,matcher.go}` + `callbacks.go` + `chain.go` + `connection.go` + `h2dispatch.go`) + ~3 LoC main.go + 0 LoC echobackend helper (reused from phase-14) = ~1208–1608 LoC production + ~1200–1430 LoC tests (~900-1100 unit + 80 fuzzer + ~150-250 matcher tests) + ~925 LoC fixture YAML/Go/PKI + ~1450 LoC docs (7 ADRs + ADR-0125 amendment + BEHAVIOR_CONTRACT 6 patches + ROADMAP + STATE + PROGRESS + REVIEW) ≈ ~4780–5410 LoC total** (production-only ~1208–1608 LoC; **the high-end estimate borderline crosses the ADR-0045 ~1500 LoC threshold**; task count below is **16**, comfortably under the 25-task threshold). Per the BOOTSTRAP_PROMPT.md §6.1 OR-trigger ("if PLAN.md > ~25 tasks OR > ~1500 LoC estimated → split") strict reading, the LoC leg borderline trips at the high-end estimate. **PLAN's disposition: SINGLE-ROW** per the phase 13/14/15 PLAN precedent (each of those phases stayed single-row despite their respective LoC + task-count estimates being at-or-near the thresholds; phase-14 PLAN explicitly framed the gate as "both legs are well under" with the production-only LoC at ~588-678 + 16 tasks; phase-15 at ~413-503 + 16 tasks). Rationale:

1. **Task count is the load-bearing trigger** for the ADR-0045 split-gate — the 25-task limit reflects mental-model + commit-cadence + review-grain considerations that LoC alone does not capture. 16 tasks is well within the comfortable single-session range.

2. **Splitting the phase would fragment a single coherent filter** — the natural 16.1+16.2 split per BRAINSTORM §1.4 (16.1 = rules-engine + Permission/Principal evaluators + per-route 7th canonical + 4-counter aggregate + TLS-principal framework primitive; 16.2 = matcher-engine + shadow + LOG-partial + track_per_rule_stats + matcher-engine framework primitive) would mean 16.1 ships an incomplete MVP missing 3 of 7 features (matcher-engine, shadow, LOG-partial), with the differential fixture's 8 scenarios artificially carved across two phases. The dual-engine dispatch table is a single switch in `DecodeHeaders` body — splitting it artificially seams the code.

3. **Both framework primitives belong in the same phase** — ADR-0142 (matcher-engine at `internal/matcher/`) is consumed by the matcher-engine path; ADR-0144 (TLS-principal accessor) is consumed by `Principal_Authenticated` in the rules-engine path. Splitting would either ship matcher-engine framework primitive WITHOUT its consumer in 16.1, or ship it alongside the rules-engine in 16.2 — neither shape is structurally honest.

4. **Phase-14/15 LoC-borderline precedent governs** — phase-14 production was ~588-678 LoC at the high-end (just below 1500); phase-15 was ~413-503 LoC. Phase-16 at ~1208-1608 LoC at the high-end is the largest single-row §9-family production-LoC count to date, but the increase tracks the wider proto surface (7 outer fields + dual-engine + 22 Permission/Principal evaluator variants vs phase-15's 4 outer fields + single-direction-throttle algorithmic surface). The 1500 LoC threshold isn't a hard line — phase-15's PLAN explicitly framed it as "well under" + phase-14's similarly; phase-16's PLAN frames the high-end estimate as "borderline" and ships single-row anyway.

5. **ADR-0045 PRECEDENT was phase-05's surface-split** of a much larger phase (~33-40 tasks + ~3450 LoC across H2 codec + cluster integration + conformance suite + fixture) where BOTH legs of the gate tripped firmly. Phase-16 trips only the LoC leg at the borderline + only at the high-end estimate; task count is firmly under. Phase-05's split was structural; phase-16's single-row is structural.

The 7 anticipated ADRs (ADR-0140..ADR-0146) all have their `Lands-in-task` anchors set in the table at `## ADRs introduced by this plan` below per ADR-0044 ADR-on-impl convention (phase-13/15 precedent; phase-14's SPEC-time-pre-landing of ADR-0129..ADR-0133 is the divergent precedent). **ADR-0125 amendment §(xii) authored at Task 10 per planner-time decision 14** — SPEC §5.4 carried the verbatim §(xii) paragraph but the SPEC commit `3159811` did NOT patch DECISIONS.md (verified via `grep -n 'xii\|phase 16' docs/envoy-go/DECISIONS.md` returning 0 matches at master tip `cedf29a`); PLAN re-anchors §(xii) authoring at impl-time Task 10 mirroring the phase-13 ADR-0127-v2 in-place-update precedent + phase-15 ADR-0125 §(xi) SPEC-time-amendment precedent generalized: phase-16's §(xii) lands at impl-time per the ADR-0044 ADR-on-impl convention (the phase-16 SPEC's prose claim "authored at this SPEC commit" was not executed at SPEC commit time — disposition documented at Task 10).

The natural ADR-0045 release-valve split per BRAINSTORM §1.4 / SPEC §1 (deferred-to-PLAN-time per SPEC §1.1 amendments + §11.Q1 resolution) would be `16.1 = rules-engine + Large 11+11 + per-route 7th canonical + 4-counter aggregate + TLS-principal framework delta` (~600-800 LoC production) and `16.2 = matcher-engine + shadow + LOG-partial + track_per_rule_stats + matcher-engine framework primitive` (~400-700 LoC production); PLAN explicitly rejects the split per rationale points 2-3 above (fragmenting a single filter + framework-primitive-consumer pairing) and ships single-row.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/rbac/doc.go` | NEW | Package doc enumerating: (a) typed_config surface (7 outer fields per §1.1 amendment 1 — `rules`/`matcher` UDPA-field-alias-grouped per amendment 2, `shadow_rules`/`shadow_matcher` analogous, `rules_stat_prefix`/`shadow_rules_stat_prefix` empty-allowed per amendment 3, `track_per_rule_stats` bool); (b) inner config.rbac.v3.RBAC consumed (`action` enum ALLOW/DENY/LOG with PGV defined_only + LOG-partial semantic per amendment 5 / `policies` lexicographic-walk / `audit_logging_options` silent-ignored per amendment 6 + §8.2 / 3 CEL fields silent-ignored per amendment 6 + §8.1); (c) Permission Large 11 + 3 deferred (parse-reject); (d) Principal Large 11 + 3 deferred including the NEW `custom` variant per amendment 7; (e) Per-route TPFC `RBACPerRoute` wrapper with reserved field 1 + single `rbac` optional field — **7th canonical absent-implies-disabled-OR-wholesale-override per ADR-0125 §(xii) amendment** (anchored at Task 10); per-route stats INDEPENDENT per ADR-0145 mirrors phase-11 + phase-15; (f) public API surface (`TypeURL` const, `New` HTTPFilterFactory); (g) iteration protocol (DECODER-only: DecodeHeaders dual-engine dispatch + SendLocalReply 403; DecodeData/Trailers pass-through; OnDestroy no-op); (h) wire-shape divergence-windows from reference Envoy — LOG-action `access_log_hint` metadata silent (envoy-go MVP; Envoy emits per amendment 5) / `response_code_details` field-emission absent (envoy-go MVP; Envoy emits per amendment 11) / CEL three-field silent-ignored (envoy-go MVP; Envoy evaluates per Q7) / shadow-rules access-log counter-only (envoy-go MVP matches Envoy v1.37.2 counter-only confirmation per §11.P13); (i) cross-cutting ADR anchors (ADR-0140 / ADR-0141 / ADR-0142 / ADR-0143 / ADR-0144 / ADR-0145 / ADR-0146 + ADR-0125 §(xii)). Mirrors phase-15 `bandwidthlimit/doc.go` shape extended for the wider proto surface. ~30 LoC. Per SPEC §1 + §6.1. |
| `internal/filter/http/rbac/rbac.go` | NEW | Main filter file — public surface + factory + types + DecodeHeaders body + filterStats. **Public surface** (per SPEC §6.1): `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBAC"`; `New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)` factory. **Internal package consts:** `filterName = "envoy.filters.http.rbac"`; `actionTypeURL = "type.googleapis.com/envoy.config.rbac.v3.Action"` (the canonical matcher-engine terminal per §11.P3 + §2.6). **Unexported types** (per SPEC §6.2 + §6.3): `compiledConfig` (8 fields per §6.2: `rules *compiledRulesEngine` + `matcher *compiledMatcherEngine` + `shadowRules *compiledRulesEngine` + `shadowMatcher *compiledMatcherEngine` + `rulesStatPrefix string` + `shadowRulesStatPrefix string` + `trackPerRuleStats bool` + `stats *filterStats`); `compiledRulesEngine` (2 fields: `action rbacconfigv3.RBAC_Action` + `policies []*compiledPolicy` lexicographic-sorted); `compiledMatcherEngine` (1 field: `tree *matcher.Matcher` wrapping the framework primitive); `compiledPolicy` (3 fields: `name string` + `permissions []permissionEvaluator` + `principals []principalEvaluator`); `compiledPerRoute` (2 fields: `disabled bool` + `overrideConfig *compiledConfig`); `factoryState` (3 fields per IMPL-1 pattern: `listenerRC *compiledConfig` + `perRoute sync.Map` + `reg *stats.Registry`); `filter` (4 fields: `state *factoryState` + `dcb envoyhttp.DecoderFilterCallbacks` + `activeRC *compiledConfig` + `passthrough bool`); `filterStats` (6 fields per §1.1 amendment 8 + ADR-0145 — **4 base counters** `allowed`/`denied`/`shadowAllowed`/`shadowDenied` + 2 management fields `perPolicy *sync.Map` (lazy per-policy counter cache keyed by `<policy_name>.<suffix>`) + `reg *stats.Registry`). **Helpers** (per SPEC §6.5 + §6.10 + §6.11): `buildCompiledConfig(c, ctx, isPerRoute)` (engine selection rules-wins per amendment 2; defensive PGV-mirror validation for action enum + policies.permissions/principals min_items=1 + destination_port lte=65535 per amendment 4); `buildCompiledRulesEngine(r, role)` (sorts policy names lexicographically per `rbac.pb.go:268-269`; recursively builds permission + principal evaluators via the evaluator.go entry points; silent-ignores audit_logging_options + 3 CEL fields per §2.1.1 + §2.1.2); `buildCompiledMatcherEngine(m)` (wraps via `matcher.New(m, []string{actionTypeURL})` per ADR-0142; PARSE-REJECT propagates as envoy-go-only error); `parsePerRoute(any)` (unmarshals to `*rbacv3.RBACPerRoute`; returns proto-message for the registry's per-route map); `(s *factoryState) resolvePerRouteConfig(msg)` (mirrors phase-11/15 lazy-cache + `LoadOrStore` pattern; returns `*compiledConfig` or nil-for-disabled per §5.1); `buildCompiledPerRoute(p, reg)` (case (a) rbac nil → `compiledPerRoute{disabled: true}`; case (b) rbac non-nil → recursive `buildCompiledConfig(..., isPerRoute=true)` → independent `*filterStats` via `newFilterStatsIfAbsent`); `newFilterStats(reg, primaryPrefix, shadowPrefix)` (registers 4 base counters under SN2-reuse namespace; nil-tolerance per ADR-0085); `newFilterStatsIfAbsent(reg, primaryPrefix, shadowPrefix)` (post-Freeze idempotent via `NewCounterIfAbsent` per ADR-0117 + ADR-0139); `(s *filterStats) incPolicyAllowed/Denied/ShadowAllowed/ShadowDenied(policyName)` (lazy-allocation via `sync.Map` keyed by `<policy_name>.<suffix>`); `emitPrimaryCounters(cc, result, policyName)`; `emitShadowCounters(cc, result, policyName)`; `(f *filter) buildEvalContext(headers)` (constructs `evalContext` interface impl on `*filter` exposing header/path/method/IP/SNI/principal/sourced-metadata/filter-state accessors per SPEC §6.2). **DecodeHeaders body** (per SPEC §6.7): resolve per-route via `f.dcb.RequestRouteConfig()` → cache on `f.activeRC` (or sets `f.passthrough` for disabled-route + returns `HeaderContinue`); if `f.activeRC` is nil OR both engines unset → passthrough; build evalContext; run primary `evaluateEngine(cc, ctx, /*shadow=*/false)` → emit primary counters; if shadow engines configured run shadow walk + emit shadow counters; on ALLOWED → `HeaderContinue`; on DENIED → `f.dcb.SendLocalReply(403, "RBAC: access denied", envoyhttp.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}})` + `HeaderStopIteration`. **DecodeData / DecodeTrailers / OnDestroy** — pass-through / no-op per SPEC §6.8 + §1 item 5. **SetDecoderCallbacks** stores `f.dcb = cb`. ~350-450 LoC. |
| `internal/filter/http/rbac/evaluator.go` | NEW | Permission + Principal evaluator surface — the algorithmic core per SPEC §6.5 + §6.6 + §6.9. **Interfaces**: `permissionEvaluator { evaluatePermission(ctx evalContext) bool }`; `principalEvaluator { evaluatePrincipal(ctx evalContext) bool }`; `evalContext` (the per-stream accessor abstraction implemented by `*filter` at `rbac.go`; per SPEC §6.2 — Header/Path/Method/DirectRemoteIP/RemoteIP/DestinationIP/DestinationPort/RequestedServerName/DownstreamPrincipal/SourcedMetadata/FilterState). **Permission Large 11** (per SPEC §1 item 3 + ADR-0143 + §6.5): `permAny` (val bool; matches anything when val=true; PGV `const=true` enforced at parse) / `permHeader` (HeaderMatcher; reuses phase-07.1 cors matcher infrastructure) / `permURLPath` (PathMatcher) / `permDestIP` (CidrRange) / `permDestPort` (uint32 exact match against `ctx.DestinationPort()`) / `permDestPortRange` (Int32Range; start ≤ port < end) / `permSNI` (StringMatcher against `ctx.RequestedServerName()`) / `permAnd` (children []permissionEvaluator; AND-semantic short-circuit) / `permOr` (children; OR-semantic short-circuit) / `permNot` (child; logical negate) / `permSourcedMetadata` (parse-supported; ALWAYS returns FALSE at runtime per §2.5). **Permission DEFERRED 3** (per §2.3 + ADR-0143): `Permission_Metadata` → PARSE-REJECT `"rbac: permission.metadata deprecated; use sourced_metadata"`; `Permission_Matcher` → PARSE-REJECT `"rbac: permission.matcher extension types unsupported in this build"`; `Permission_UriTemplate` → PARSE-REJECT `"rbac: permission.uri_template extension types unsupported in this build"`. **Principal Large 11** (per SPEC §1 item 3 + §1.1 amendment 7 + ADR-0143 + §6.5): `prinAny` / `prinAuthenticated` (per §1.1 amendment 12 + §6.6 three-case algorithm: (a) `nameMatcher == nil` + `len(ctx.DownstreamPrincipal()) > 0` → TRUE / (b) non-nil StringMatcher iterates over candidates in priority order → TRUE on first match / (c) plaintext or no client cert → FALSE) / `prinDirectRemoteIP` (CidrRange against peer connection's source IP) / `prinRemoteIP` (CidrRange against XFF-resolved IP via phase-04/05 XFF resolver per §11.P18) / `prinHeader` / `prinURLPath` / `prinAnd` / `prinOr` / `prinNot` / `prinSourcedMetadata` (always FALSE runtime) / `prinFilterState` (FilterStateMatcher; always FALSE runtime per §2.5). **Principal DEFERRED 3** (per §2.4 + §1.1 amendment 7 + ADR-0143): `Principal_SourceIp` → PARSE-REJECT `"rbac: principal.source_ip deprecated; use direct_remote_ip or remote_ip"`; `Principal_Metadata` → PARSE-REJECT `"rbac: principal.metadata deprecated; use sourced_metadata"`; `Principal_Custom` → PARSE-REJECT `"rbac: principal.custom extension types unsupported in this build"`. **Builders** (per SPEC §6.5): `buildPermissionEvaluators(perms []*Permission) ([]permissionEvaluator, error)` (iterates calling `buildOnePermission`; wraps errors with `permission[%d]:` prefix); `buildOnePermission(p *Permission)` (switch on `p.GetRule()` 14 cases — 11 accepted + 3 PARSE-REJECT + nil-rule defensive); analogous `buildPrincipalEvaluators` + `buildOnePrincipal` (switch on 14 cases). **Engine evaluators** (per SPEC §6.9): `evaluateEngine(cc, ctx, shadow bool) (engineResult, string)` (engine result enum {Allowed, Denied} + matched policy name; chooses rules-engine OR matcher-engine via cc fields; defensive ALLOWED for shadow-unset case); `evaluateRulesEngine(re, ctx)` (walks policies; first match wins per SPEC §6.9; applies action: ALLOW match → Allowed; DENY match → Denied; LOG → Allowed always per §1.1 amendment 5 with matched policy captured); `policyMatches(p, ctx)` (OR-semantic permissions + OR-semantic principals short-circuit); `evaluateMatcherEngine(me, ctx)` (delegates to `me.tree.Evaluate(matcherCtxAdapter{ctx})` → unmarshal terminal Any as `rbacconfigv3.Action` → switch on Action.GetAction(): ALLOW/LOG → Allowed + matchedName=Action.GetName(); DENY → Denied; no-match per `rbac.pb.go:43-46` → Denied). **Shared infrastructure adapters**: `matchString(matcher *envoytypev3.StringMatcher, candidate string) bool` (reuses existing phase-07.1 StringMatcher impl from cors); `matchCidr(cidr *envoycorev3.CidrRange, ip net.IP) bool` (CidrRange evaluation; reuses existing phase-07.1 infrastructure). ~650-850 LoC. |
| `internal/filter/http/rbac/rbac_test.go` | NEW | Unit tests per SPEC §14.1 (8 SPEC-named groups + 1 stats-namespace integration sub-group surfaced at PLAN-time per planner-time decision 16). **Group 1 — Config parse + buildCompiledConfig (per SPEC §14.1 #1 + §6.5 + §1.1 amendments 1-6):** `TestNew_NilTC`, `TestNew_MalformedTC`, `TestBuildCompiledConfig_AllSevenOuterFieldsAccepted` (proto-faithful per amendment 1), `TestBuildCompiledConfig_BothRulesAndMatcherSet_RulesWins` (per amendment 2 + §11.P1), `TestBuildCompiledConfig_BothShadowSet_ShadowRulesWins`, `TestBuildCompiledConfig_NeitherEngineSet_WhollyInactive` (per `rbac.pb.go:33`), `TestBuildCompiledConfig_EmptyRulesStatPrefix_Accepted` (per amendment 3), `TestBuildCompiledConfig_EmptyShadowRulesStatPrefix_Accepted`, `TestBuildCompiledConfig_AllThreeActionEnumValues_Accepted` (ALLOW/DENY/LOG), `TestBuildCompiledConfig_InvalidActionEnum_Rejected` (per amendment 4 PGV defined_only), `TestBuildCompiledRulesEngine_EmptyPermissions_Rejected` (per amendment 4 min_items=1), `TestBuildCompiledRulesEngine_EmptyPrincipals_Rejected`, `TestBuildCompiledRulesEngine_LexicographicPolicyOrder_Preserved` (per `rbac.pb.go:268-269`), `TestBuildCompiledRulesEngine_AuditLoggingOptions_SilentIgnored` (per §2.1.1), `TestBuildCompiledRulesEngine_ConditionField_SilentIgnored` (per amendment 6 + Q7), `TestBuildCompiledRulesEngine_CheckedConditionField_SilentIgnored`, `TestBuildCompiledRulesEngine_CelConfigField_SilentIgnored` (per amendment 6 NEW — third CEL field). **Group 2 — buildCompiledConfigPerRoute + parsePerRoute (per SPEC §14.1 #2 + §5 + §11.P1):** `TestParsePerRoute_EmptyWrapper_DisabledOnRoute` (case (a) 7th canonical absent-implies-disabled), `TestParsePerRoute_RbacFieldNil_DisabledOnRoute` (same), `TestParsePerRoute_RbacFieldSet_WholesaleOverride` (case (b) per §5.1), `TestBuildCompiledPerRoute_OverrideCarriesOwnStatPrefix_INDEPENDENT` (per ADR-0145 INDEPENDENT-stats), `TestParsePerRoute_MalformedAny_Rejected`, `TestResolvePerRouteConfig_NilMsg_FallsBackToListener`, `TestResolvePerRouteConfig_LazyCacheSyncMap_PointerIdentityKey` (multi-request same per-route entry → single allocation). **Group 3 — Permission evaluators (per SPEC §14.1 #3 + §6.5):** `TestPermAny_True_Matches`, `TestPermAny_FalseValue_Rejected` (PGV const=true mirror per amendment 4), `TestPermHeader_Match` (header exact/prefix/safe-regex), `TestPermURLPath_PathMatcher`, `TestPermDestIP_CIDR`, `TestPermDestPort_Exact`, `TestPermDestPortRange_StartLEPortLTEnd`, `TestPermSNI_StringMatcher` (server-name match against `ctx.RequestedServerName()`), `TestPermAndRules_Recursive_AllMatch` (3-4 level depth), `TestPermOrRules_Recursive_AnyMatch`, `TestPermNotRule_Recursive_Negate`, `TestPermSourcedMetadata_ParseSupported_RuntimeFalse` (per §2.5 + §8.10 always-no-match), `TestPermMetadata_PARSE_REJECT` (per §2.3 + §11.P12 deprecated), `TestPermMatcher_PARSE_REJECT` (per §8.8 extension), `TestPermUriTemplate_PARSE_REJECT`. **Group 4 — Principal evaluators (per SPEC §14.1 #4 + §6.5 + §1.1 amendment 7):** `TestPrinAny_True_Matches`, `TestPrinDirectRemoteIP_CIDR_PeerSource`, `TestPrinRemoteIP_CIDR_XFFResolved` (per §11.P18; reuses phase-04/05 XFF resolver), `TestPrinHeader_HeaderMatcher`, `TestPrinURLPath_PathMatcher`, `TestPrinAndIds_Recursive_AllMatch`, `TestPrinOrIds_Recursive_AnyMatch`, `TestPrinNotId_Recursive_Negate`, `TestPrinSourcedMetadata_RuntimeFalse`, `TestPrinFilterState_RuntimeFalse`, `TestPrinAuthenticated_ThreeCaseAlgorithm` (per amendment 12 + §6.6 — case (a) nil principal_name + len(DownstreamPrincipal)>0 → TRUE; case (b) StringMatcher iteration over URI SAN → DNS SAN → Subject DN; case (c) plaintext → FALSE), `TestPrinSourceIp_PARSE_REJECT` (per §2.4 deprecated), `TestPrinMetadata_PARSE_REJECT` (per §2.4 deprecated), `TestPrinCustom_PARSE_REJECT` (per amendment 7 + §8.11 NEW — the 14th Principal variant). **Group 5 — Dual-engine dispatch (per SPEC §14.1 #5 + §6.9):** `TestEvaluateRulesEngine_AllowMatch_Allowed`, `TestEvaluateRulesEngine_AllowNoMatch_Denied`, `TestEvaluateRulesEngine_DenyMatch_Denied`, `TestEvaluateRulesEngine_DenyNoMatch_Allowed`, `TestEvaluateRulesEngine_LogMatch_AllowedWithPolicyName` (per amendment 5 — LOG always-allow + matched-policy captured for per-policy emission), `TestEvaluateRulesEngine_LogNoMatch_Allowed` (LOG always-allows regardless), `TestEvaluateRulesEngine_LexicographicOrderShortCircuit` (multi-policy; first match wins), `TestEvaluateMatcherEngine_CanonicalActionTerminal_Honored` (canonical RBAC `Action` TypedExtensionConfig), `TestEvaluateMatcherEngine_NoMatch_Denied` (per `rbac.pb.go:43-46`), `TestEvaluateMatcherEngine_UnknownTerminalTypeURL_ParseRejected` (PARSE-time at `buildCompiledMatcherEngine`), `TestEvaluateEngine_BothPrimaryAndShadowConfigured_PrimaryDispositionWinsShadowEmitsCounter` (shadow never affects disposition), `TestEvaluateEngine_BothEnginesUnset_DefensiveAllowed`. **Group 6 — DecodeHeaders gating + SendLocalReply (per SPEC §14.1 #6 + §6.7):** `TestDecodeHeaders_ListenerLevelAllowMatch_HeaderContinue`, `TestDecodeHeaders_ListenerLevelDenyMatch_HeaderStopIteration_SendLocalReply403`, `TestDecodeHeaders_SendLocalReply_Body19Bytes_RBACAccessDenied` (per amendment 10 + §11.P5; byte-exact `RBAC: access denied`), `TestDecodeHeaders_SendLocalReply_4HeaderSet_LowercaseWireForm` (content-length: 19, content-type: text/plain, date, server: envoy), `TestDecodeHeaders_SendLocalReply_KeepAliveDisposition_NoConnectionClose`, `TestDecodeHeaders_LOGMatch_HeaderContinue_AllowedCounterIncremented` (per amendment 5 — LOG folds into allowed; no separate logged counter per amendment 8), `TestDecodeHeaders_PerRouteDisabled_PassthroughNoCounters` (case (a) 7th canonical), `TestDecodeHeaders_PerRouteOverride_INDEPENDENTCounterNamespace` (per ADR-0145), `TestDecodeHeaders_BothEnginesUnset_PassthroughNoCounters`. **Group 7 — DownstreamPrincipal accessor + Principal_Authenticated three-case (per SPEC §14.1 #7 + §1.1 amendment 12 + ADR-0144):** `TestDownstreamPrincipal_PlaintextConnection_NilSlice`, `TestDownstreamPrincipal_mTLSConnection_URISANs_FirstPriority`, `TestDownstreamPrincipal_mTLSConnection_DNSSANs_SecondPriority`, `TestDownstreamPrincipal_mTLSConnection_SubjectDNCommonName_ThirdPriority`, `TestDownstreamPrincipal_OrderingPreserved` (slice contains URI SAN first, then DNS SAN, then Subject DN CN in that order). **Group 8 — Matcher-engine framework primitive (per SPEC §14.1 #8 + ADR-0142):** `TestMatcherNew_CanonicalRBACActionTerminal_Accepted`, `TestMatcherNew_UnknownTypeURL_PARSE_REJECT`, `TestMatcherEvaluate_FirstMatchingPredicate_ReturnsTerminalAny`, `TestMatcherEvaluate_NoMatchingPredicate_ReturnsNilNil`, `TestMatcherEvaluate_HeaderPredicate_Match`, `TestMatcherEvaluate_PathPredicate_Match`, `TestMatcherEvaluate_AndPredicate_AllMatch`, `TestMatcherEvaluate_OrPredicate_AnyMatch`, `TestMatchContext_AccessorAdapter_DelegatesToFilter`. **Group 9 — Stats namespace integration (per planner-time decision 16 + ADR-0145):** `TestStatsNamespace_AllFourBaseCountersRegistered` (allowed/denied/shadow_allowed/shadow_denied per active prefix), `TestStatsNamespace_SN2Reuse_NoNewSN10Rule` (per amendment 9 + §11.P7; existing default-branch flatten), `TestStatsNamespace_HCMRootedPath_HttpHCMRbacPrefixCounter`, `TestStatsNamespace_PerPolicyLazyAllocation_OnFirstMatch` (per amendment 9 — `track_per_rule_stats: true`; cache hit on subsequent matches), `TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent` (post-Freeze per ADR-0117 + ADR-0139). Test helpers `mustAny(t, msg)` + `freshFactoryCtx()` + `freshFactoryCtxWithRegistry()` + `freshMTLSEvalContext(t, principals)` (mTLS-equipped evalContext mock for Group 7) mirror phase-13/14/15 precedents. ~900-1100 LoC. |
| `internal/filter/http/rbac/fuzz_test.go` | NEW | `FuzzRBACConfigParse` — fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`. Asserts: `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. **Fuzzer axes**: random bytes / partial proto-shaped bytes / valid proto with random rules-engine policy variants (including the deprecated + extension PARSE-REJECT cases for coverage) / valid proto with random matcher-engine tree shapes / recursion-depth AND/OR/NOT combinator inputs. Per ADR-0018 "every parser/codec/filter ships a fuzzer". ~80 LoC; 30s budget per ADR-0018; **20th fuzzer overall** (post-phase-15's 19th `FuzzBandwidthLimitConfigParse`). Seed corpus: 8 valid seeds (rules-engine ALLOW; rules-engine DENY; rules-engine LOG; matcher-engine canonical-Action terminal; rules+shadow_rules combo; matcher+shadow_matcher combo; track_per_rule_stats=true; per-route TPFC wholesale-override) + 5 invalid seeds (nil tc; empty rules policies map; nil permissions array; Principal_Custom variant; non-canonical matcher terminal TypeURL). |
| `internal/matcher/doc.go` | NEW | Package doc for the NEW top-level package per ADR-0142: `// Package matcher implements a generic xds.type.matcher.v3.Matcher match-tree evaluator usable by HTTP filters that consume the matcher-engine surface (e.g., envoy.filters.http.rbac, future ext_authz/jwt_authn/oauth2/ext_proc). The package is cross-phase reusable: callers extend supportedActionTypes for their own terminal action TypeURLs and widen MatchContext additively as new predicate accessors are needed.` ~25 LoC. |
| `internal/matcher/matcher.go` | NEW | `Matcher` type wrapping a parsed `xds.type.matcher.v3.Matcher` tree per ADR-0142 + SPEC §3.2 + §6.2. **Public API:** `Matcher` opaque type; `New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher, error)` parses the proto tree + validates terminal `Any.TypeUrl` against the allow-list at config-load time + PARSE-REJECT for unknown TypeURLs with envoy-go-only error `"matcher: terminal action type %q unsupported by this caller"`; `(m *Matcher) Evaluate(ctx MatchContext) (*anypb.Any, error)` walks the tree at request time + returns matched terminal `Any` OR `(nil, nil)` on no-match; `MatchContext` interface (request-side accessor abstraction) exposing the predicate accessors RBAC needs (Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName — initial set scoped to RBAC's canonical surface; extended additively by future callers per ADR-0142 §Decision (iii)). **Internal structure:** parsed `*compiledNode` tree mirroring the proto tree's `OnMatch_OnMatchTree`/`OnMatch_OnMatchAction` walk semantics; each leaf carries the validated terminal `Any` pointer; predicate evaluators (`headerPredicate`, `pathPredicate`, etc.) reuse phase-07.1 HeaderMatcher/PathMatcher/StringMatcher infrastructure. **No state-mutation across Evaluate calls** (tree is read-only post-New). ~150-250 LoC. Per ADR-0142 + SPEC §3.2 + §11.P3 (canonical terminal allow-list). |
| `internal/matcher/matcher_test.go` | NEW | Unit tests per ADR-0142 acceptance. `TestNew_CanonicalActionTerminal_Accepted` (RBAC's canonical Action; supportedActionTypes allow-list); `TestNew_UnknownTerminalTypeURL_PARSE_REJECT` (envoy-go-only error verbatim); `TestNew_EmptyAllowList_AllTerminalsRejected`; `TestEvaluate_FirstMatchingPredicate_ReturnsTerminalAny`; `TestEvaluate_NoMatchingPredicate_NilNil`; `TestEvaluate_HeaderPredicate_ExactMatch`; `TestEvaluate_HeaderPredicate_PresentMatch`; `TestEvaluate_PathPredicate_PrefixMatch`; `TestEvaluate_AndPredicate_AllChildrenMatch`; `TestEvaluate_OrPredicate_FirstChildMatch`; `TestEvaluate_NestedTree_DepthThree` (matcher within matcher within matcher); `TestMatchContext_AdapterPattern` (mock MatchContext returning canned values); `TestMatcher_StatelessAcrossEvaluations` (concurrent Evaluate calls safe). ~150-250 LoC. |
| `internal/filter/http/callbacks.go` | MODIFIED | NEW one-line addition to `DecoderFilterCallbacks` interface: `DownstreamPrincipal() []string` per ADR-0144 §Decision (i). GoDoc note: `// DownstreamPrincipal returns the priority-ordered TLS principal-name candidates from the downstream client connection: URI SANs first, then DNS SANs, then the Subject DN Common Name as fallback. Returns nil (or empty) for plaintext / non-mTLS connections or connections where no client cert was presented. The slice mirrors Envoy v1.37.2's Principal_Authenticated extraction semantics per rbac.pb.go:1432-1438.` +1 LoC. Per SPEC §3.1 + ADR-0144. |
| `internal/filter/http/chain.go` | MODIFIED | Three additions per ADR-0144 §Decision (ii): (a) new per-stream field `tlsPrincipals []string` on `*FilterChain`; (b) new method on `decoderCB` struct: `func (c *decoderCB) DownstreamPrincipal() []string { return c.chain.tlsPrincipals }`; (c) new accessor on `*FilterChain`: `func (c *FilterChain) SetTLSPrincipals(p []string) { c.tlsPrincipals = p }` (called by HCM's connection.go + h2dispatch.go at chain-build time before `RunDecodeHeaders` dispatch). Total ~+30 LoC delta. Per SPEC §3.1 + ADR-0144. |
| `internal/filter/http/chain_test.go` | MODIFIED | NEW probe-filter-driven DownstreamPrincipal integration tests per Task 6 TDD discipline. Probe filter `downstreamPrincipalProbe` implements `StreamDecoderFilter` capturing `f.cb.DownstreamPrincipal()` at DecodeHeaders into a slot for test assertion. Test cases: `TestDecoderCB_DownstreamPrincipal_NoSeed_NilSlice`; `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed`; `TestDecoderCB_DownstreamPrincipal_OrderingPreservedAcrossCalls` (multiple DownstreamPrincipal() invocations return the same slice in the same order). ~+60 LoC delta. |
| `internal/filter/hcm/connection.go` | MODIFIED | NEW pre-`RunDecodeHeaders` TLS principal extraction per ADR-0144 §Decision (iii): if the connection's underlying transport is `*tls.Conn` AND `conn.ConnectionState().HandshakeComplete` AND `len(state.PeerCertificates) > 0` → extract URI SAN strings from `state.PeerCertificates[0].URIs[]` + DNS SAN strings from `state.PeerCertificates[0].DNSNames` + Subject DN CN string from `state.PeerCertificates[0].Subject.CommonName`; concatenate in priority order; call `chain.SetTLSPrincipals(principals)` before `RunDecodeHeaders` dispatch. Inserted at H1 dispatch path between chain construction and `RunDecodeHeaders` invocation. ~+25 LoC delta. Per SPEC §3.1 + ADR-0144. |
| `internal/filter/hcm/h2dispatch.go` | MODIFIED | NEW pre-`RunDecodeHeaders` TLS principal extraction symmetric to `connection.go` H1 path per ADR-0144 §Decision (iii). Inserted at H2 dispatch path at per-stream chain construction. ~+15 LoC delta. Per SPEC §3.1 + ADR-0144. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW one-line `httpReg.Register(rbac.TypeURL, rbac.New)` registration inserted IMMEDIATELY AFTER the existing `httpReg.Register(localratelimit.TypeURL, localratelimit.New)` line and BEFORE `header_mutation.RegisterPerRouteValidator(httpReg)` + `httpReg.Freeze()` per ADR-0100 §2.2 + ADR-0140 §Decision (v) router-first-then-alphabetical stylistic discipline. Plus the matching `import "github.com/esalaine/envoy-go/internal/filter/http/rbac"` alphabetically among the existing filter-package imports. The resulting block reads: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → localratelimit → rbac → header_mutation.RegisterPerRouteValidator → httpReg.Freeze()`. ~+3 LoC delta (1 import + 1 register). Per SPEC §1 item 2 + ADR-0140. |
| `test/differential/fixture/fixture.go` | MODIFIED | NEW `BackendKind` enum value `HTTPRbac BackendKind = 15` after the existing `HTTPBandwidthLimit BackendKind = 14`. Doc-comment: "HTTPRbac reuses the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go` for scenarios that exercise upstream routes (scenarios 5 + 6 + 8). Three-listener fixture (l_test_a plaintext + l_test_b echo-backend + l_test_a_tls mTLS-required for scenario 6). No new helper authored at phase 16 — phase-14's echobackend remains the shared helper." ~+15 LoC delta. |
| `test/differential/runner_test.go` | MODIFIED | NEW blank-import addition `_ "github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/inputs"` (alphabetical-after `0017`). NEW switch-case in BackendKind dispatch for `HTTPRbac` reusing the existing `startEchoBackend` helper introduced at phase-14 Task 10. ~+10 LoC delta. |
| `test/fixtures/0018-http-rbac/` | NEW DIRECTORY | Differential fixture with 8 scenarios per SPEC §7. THREE-listener topology (`l_test_a` plaintext + `l_test_b` upstream echo-backend cluster + `l_test_a_tls` mTLS-required for scenario 6). |
| `test/fixtures/0018-http-rbac/envoy.yaml` | NEW | Reference Envoy bootstrap. Listener `l_test_a` (TCP plaintext; HCM chain `rbac → router`) with listener-level RBAC config per SPEC §7.2 — rules_stat_prefix=default; action=ALLOW; 4 policies (admin_users via X-User=admin header; public_paths via /public url_path; listener_port_match via destination_port; local_clients via /protected url_path + 127.0.0.0/8 direct_remote_ip). Routes: `/` direct_response 200 + 32-byte body (scenarios 1, 2, 4); `/public` direct_response 200 (scenario 3); `/protected` cluster c_backend_b (scenario 5); `/per-route-disabled` direct_response with per-route TPFC `RBACPerRoute{}` (empty wrapper → disabled per §5.1 case (a); scenario 7); `/per-route-override` direct_response with per-route TPFC `RBACPerRoute{rbac: <RBAC rules_stat_prefix:"override", shadow_rules_stat_prefix:"override_shadow", action:DENY, policies:{guests:{permissions:[any], principals:[header X-User=guest]}}, shadow_rules: mirror>}` (scenario 8). Listener `l_test_a_tls` (TCP + mTLS-required; transport_socket DownstreamTlsContext with fixture-CA validation + require_client_certificate=true; HCM chain identical to l_test_a + one extra policy `authenticated_admin` on `/admin` url_path requiring Principal_Authenticated with principal_name=spiffe://example.com/admin StringMatcher; scenario 6). Cluster `c_backend_b` STRICT_DNS to the echobackend subprocess. ~165 LoC. Per SPEC §7.2. |
| `test/fixtures/0018-http-rbac/envoy-go.yaml` | NEW | Equivalent envoy-go bootstrap. Same three-listener topology (cluster type STATIC instead of STRICT_DNS); same route+per-route map; same TLS context. Both sides set explicit `rules_stat_prefix` + `shadow_rules_stat_prefix` per amendment 9 to drive SN2-reuse counter namespace identically. ~165 LoC. Per SPEC §7.2. |
| `test/fixtures/0018-http-rbac/pki/gen.go` | NEW | mTLS PKI generation for scenario 6 per planner-time decision 11 + SPEC §7.2. **Algorithm:** `init()` generates at fixture-load time (NOT pre-baked) a fresh fixture-CA cert/key pair (`ecdsa.P256`) + a server cert/key pair signed by fixture-CA carrying `DNSNames: [l_test_a_tls.fixture.test]` + a client cert/key pair signed by fixture-CA carrying `URIs: [spiffe://example.com/admin]` + Subject CN `client.fixture.test`. **Output:** PEM-encoded `caCert`, `serverCert`, `serverKey`, `clientCert`, `clientKey` files written to a fixture-managed temp directory; paths plumbed into the Envoy/envoy-go YAMLs via runner-substitution placeholders at boot. **Library:** `crypto/x509` + `crypto/ecdsa` + `crypto/elliptic` + `encoding/pem` Go stdlib. Mirrors phase-03 mTLS PKI generation discipline (see `test/fixtures/0002-tcp-tls-passthrough/pki/gen.go` precedent — to-be-confirmed at impl-time path; otherwise new per-fixture pattern). ~120 LoC. Per SPEC §7.2 + ADR-0144 §Consequences. |
| `test/fixtures/0018-http-rbac/inputs/driver.go` | NEW | Go driver issuing the 8 scenarios per SPEC §7.4 mirroring phase-14/15 driver shape. Functions `runScenario1..runScenario8(ctx, baseURL_plaintext, baseURL_mTLS) error`. Per-scenario assertion helper for byte-exact body (allow paths verbatim; deny paths `RBAC: access denied` 19-byte) + per-counter delta byte-equivalence on the 4 base counters per active namespace. **Scenario 6 helper** `runTLSScenario6(ctx, baseURL_mTLS, clientCertPath, clientKeyPath, caCertPath)` uses a fresh `http.Client` with `Transport: &http.Transport{TLSClientConfig: &tls.Config{Certificates: [clientCertKeyPair], RootCAs: caCertPool}}` to present the URI SAN `spiffe://example.com/admin` client cert during handshake; `/admin` route allowed via `Principal_Authenticated` match. **Scenario 8 INDEPENDENT-stats assertion**: scrape `/stats/prometheus`; assert `<HCM>.rbac.override.denied += 1` AND `<HCM>.rbac.override_shadow.shadow_denied += 1` (per-route prefixes) AND `<HCM>.rbac.default.*` UNCHANGED (listener-level prefix). Total estimated driver size: ~290 LoC. Per SPEC §7.4. |
| `test/fixtures/0018-http-rbac/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7.3. Documents the 8-scenario equivalence claim including the per-route INDEPENDENT-stats scenarios 7 + 8 + the mTLS scenario 6 + the divergence-window allow-list (LOG-action `access_log_hint` metadata + `response_code_details` field both ABSENT on envoy-go side). ~75 LoC. Per SPEC §7.3 + planner-time decision 18 (counter-delta byte-equivalence convention from phase-13/14/15). |
| `test/fixtures/0018-http-rbac/README.md` | NEW | Fixture overview + 8-scenario list + reference config citations + mTLS PKI generation notes (init()-time fresh-cert pattern; CA trust scoped to fixture only) + per-route 7th canonical absent-implies-disabled discipline note (ADR-0125 §(xii)) + INDEPENDENT-stats discipline note (ADR-0145) + counter-delta assertion discipline + divergence-window note (LOG metadata + response_code_details + CEL silent-ignored). ~110 LoC. Per SPEC §7.2. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | **7 NEW ADRs** (ADR-0140 + ADR-0141 + ADR-0142 + ADR-0143 + ADR-0144 + ADR-0145 + ADR-0146) authored at impl-time per ADR-0044 ADR-on-impl convention — Lands-in-task: ADR-0140 + ADR-0141 at Task 2; ADR-0142 at Task 3; ADR-0143 across Tasks 4 (Permission) + 5 (Principal finalized); ADR-0144 at Task 6; ADR-0145 at Task 8; ADR-0146 at Task 9. Plus **ADR-0125 amendment paragraph §(xii)** authored at Task 10 per planner-time decision 14 (NOT pre-landed at SPEC commit despite SPEC §5.4 prose claim — see Task 10 + planner-time decision 14). ~+700 LoC across the 7 ADRs + ~+30 LoC for the §(xii) amendment paragraph. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 patches — §13.1 `### envoy.filters.http.rbac` subsection inserted **AFTER `### envoy.filters.http.bandwidth_limit` at line 1416** (landing-chronological per phase-13/14/15 precedent per planner-time decision 19; SPEC §13.1 stub-text's "alphabetical" claim is inaccurate against observed file state) ~230 LoC; §13.2 stat-table 60→64 names extension (4 new active rows: `<HCM>.rbac.<prefix>.allowed`/`.denied`/`<shadow>.shadow_allowed`/`.shadow_denied`; per-policy template-form documented separately) ~30 LoC; §13.3 NEW equivalence-matrix row pointing at fixture 0018 ~3 LoC; §13.4 NEW `### Phase 16 forward-pointer notes` subsection ~100 LoC covering the 12-item deferral list + LOG-action divergence-window + response_code_details divergence-window + CEL three-field silent-ignore + shadow access-log forward-pointer + TWO new framework primitives note + track_per_rule_stats foot-gun + Principal_Set recursion-depth foot-gun + no-new-SN10-rule note; §13.5 `## HTTPFilterCallbacks` extension with NEW `### DownstreamPrincipal accessor` subsection ~25 LoC per ADR-0144; §13.6 NEW `## Matcher engine framework primitive` top-level section ~20 LoC per ADR-0142. Total ~+410 LoC. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `16` status `in-progress → done` flip + summary sharpening (post-impl counts; PLAN-confirmed 16-task + ~1208-1608 LoC production estimate + final ADR roster 7 anchored + 1 amendment) ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state: lifecycle-state `phase 16 done; awaiting next planning`; next-skill (none — phase complete); next-active-phase TBD by ROADMAP. |
| `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror phase 04-15 PROGRESS.md structure. ~800 LoC across 16 task entries. |
| `docs/envoy-go/phases/16-http-filter-rbac/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~250 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + this PLAN's planner-time-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's twelve deferred decisions before implementation; this PLAN settles all twelve plus seven that emerged at PLAN-drafting time (items 13–19 below). The nineteen resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced in summary form here so the implementer at each task can act without re-deriving them:

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

These nineteen decisions are reproduced verbatim in `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` Preamble (Task 1) so any subsequent reader has the full context without re-reading this PLAN.

---

## ADRs introduced by this plan

The seven ADRs anticipated by SPEC §8 (ADR-0140..ADR-0146). **AUTHORED AT IMPL-TIME per ADR-0044 ADR-on-impl convention** (phase-13 buffer + phase-15 bandwidth_limit pattern; UNLIKE phase-14 compressor's SPEC-time-pre-landing — phase-14 was the divergent precedent). Per-ADR Lands-in-task anchors:

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

The implementer at each impl-anchor task AUTHORS the ADR §Context/Decision/Consequences body in DECISIONS.md (in the slot immediately after the prior ADR; ADR-0140 inserts after ADR-0139; ADR-0141 inserts after ADR-0140; etc.) AND includes the ADR in the commit message AND verifies via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning 1 match (the canonical authoring-check) AND verifies `Lands-in-task: Task N` field via `grep -nE '^Lands-in-task: Task N' docs/envoy-go/DECISIONS.md`.

**Inline supersessions / amendments anticipated** (cross-references only; **NO in-place ADR edits required by phase 16 except for ADR-0125 §(xii) amendment paragraph at Task 10**):

- **ADR-0073** (typed_per_filter_config 3-tier merge — most-specific override) — UNCHANGED in phase 16. Phase-16 rbac's per-route discipline (7th canonical absent-implies-disabled-OR-wholesale-override) inherits ADR-0073's wholesale-not-merge semantic for case (b). ADR-0125 §(xii) amendment (anchored at Task 10) extends ADR-0125's canonical-shape catalog to 7 entries without amending ADR-0073. NO in-place edit.
- **ADR-0040** (out-of-scope deferrals format) — UNCHANGED in phase 16. The 12-item deferral list (per SPEC §12) is captured INLINE at BEHAVIOR_CONTRACT §13.4 (the `### Phase 16 forward-pointer notes` subsection). NO new deferral ADRs (mirrors phase 10-15 SPEC §8.1 collapse precedent).
- **ADR-0044** (ADR-on-impl convention) — UNCHANGED. The 7 ADRs (ADR-0140..ADR-0146) each carry a `Lands-in-task` field anchored at the first-use impl-task; the ADR body is authored at impl-time per the phase-13/15 convention.
- **ADR-0061** (stats Registry + SN1–SN9 rules) — UNCHANGED in phase 16. NO new SN flattening rule per §1.1 amendment 9 + §11.P7 (SN2-reuse hypothesis at PLAN time; impl-time empirical scrape ratifies or amends). RBAC reuses the existing `internal/stats/name.go` default-branch flatten with HCM-rooted `http.*` segment routing. Cross-reference recorded in ADR-0145 §Decision. NO in-place edit.
- **ADR-0072** (HTTPRegistry threaded constructor map) — UNCHANGED. Cross-reference recorded in ADR-0140 §Consequences. NO in-place edit.
- **ADR-0074** (filter set) — purely additive expansion recorded in ADR-0140 §Consequences. Filter set extends from {bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, router} to {bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, **rbac**, router}. NO in-place edit.
- **ADR-0075 + ADR-0076** (HCM dispatch + framework body-buffer cap) — UNCHANGED. Phase-16 rbac is decoder-only + pre-body; no body-buffering interaction. NO in-place edit.
- **ADR-0100 + ADR-0101 + ADR-0102** (FactoryCtx framework extension + runtimeConfig shape + StopIteration localReplyDone gate) — UNCHANGED. Phase-16 rbac CONSUMES ADR-0100's `ctx.Stats` for filterStats registration + CONSUMES ADR-0102's StopIteration on DENY via `SendLocalReply` mechanism + mirrors ADR-0101's closure-capture-at-New + read-only-shared-after-New discipline. Cross-references recorded in ADR-0140 + ADR-0141 + ADR-0145. NO in-place edits.
- **ADR-0117** (per-route bucket isolation as ADR-0073 wholesale-override consequence; phase-11 local_ratelimit) — UNCHANGED §Decision sections. ADR-0145 directly inherits ADR-0117's machinery (lazy-cache `sync.Map`, `NewCounterIfAbsent` post-Freeze, `resolvePerRouteConfig` accessor); phase-16 rbac is THIRD row using stateful-override-with-INDEPENDENT-stats (phase-11 FIRST + phase-15 SECOND + phase-16 THIRD). NO in-place edit.
- **ADR-0125** (5+6 canonical per-route patterns + §(viii)-(x) + §(xi) in-place amendments) — **§(xii) AMENDMENT PARAGRAPH AUTHORED AT TASK 10** per planner-time decision 14. The amendment paragraph documents phase-16's 7th canonical pattern + the canonical-roster growth from 6 to 7 entries. The §(xi) amendment from phase-15 is unchanged; the §(viii)-(x) amendments from phase-14 are unchanged.
- **ADR-0128** (HCM framework primitives — synthetic empty-terminal RunDecodeData + post-body CL reconciliation) — UNCHANGED in phase 16. Phase-16 rbac is pre-body decoder-only; the synthetic empty-terminal RunDecodeData primitive is structurally unused (rbac evaluates at DecodeHeaders before body bytes flow). Cross-reference recorded in ADR-0140 §Decision (vi). NO in-place edit.
- **ADR-0131** (Path B body algorithm + OverwriteBody encode-side primitive) — UNCHANGED in phase 16. Phase-16 rbac is encoder-nil; OverwriteBody is structurally unused. NO in-place edit.
- **ADR-0139** (per-route INDEPENDENT-stats wiring for bandwidth_limit) — UNCHANGED. ADR-0145 cross-references ADR-0139 as the closest precedent (phase-15 bandwidth_limit was SECOND row using INDEPENDENT-stats; phase-16 rbac is THIRD); the underlying machinery (`buildCompiledConfigPerRoute` + `newFilterStatsIfAbsent` + `sync.Map` lazy-cache) reuses ADR-0139's pattern verbatim modulo the 4-counter-no-gauges surface (vs phase-15's 8-counters + 6-gauges). NO in-place edit.

These fifteen cross-references land at the tasks that anchor each affected ADR (ADR-0140 + ADR-0141 at Task 2; ADR-0142 at Task 3; ADR-0143 at Tasks 4 + 5; ADR-0144 at Task 6; ADR-0145 at Task 8; ADR-0146 at Task 9; ADR-0125 §(xii) at Task 10). **The ONLY in-place ADR edit required by phase 16 is the §(xii) amendment paragraph on ADR-0125 at Task 10** — this is consistent with the phase-13 ADR-0127-v2 in-place-update precedent (also one in-place edit at impl-time).

---

## Execution preconditions

Before Task 1, the implementer cold-starts and verifies. **Worktree spawn discipline:** the impl session is expected to run on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (per the user's persistent preference for git worktrees recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`). The expected sequence (executed by the orchestrating session BEFORE invoking the impl session, OR by the impl session itself at cold-start if it's running standalone) is:

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-16-http-filter-rbac-impl \
                 -b phase-16-http-filter-rbac-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-16-http-filter-rbac-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md commit + its SHA-fill follow-up (filled by the orchestrating session that landed the PLAN).

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-16-http-filter-rbac-impl` (the impl-stage worktree). If a SPEC-stage or PLAN-stage worktree is the only branch present, branch a fresh impl worktree from master HEAD per ADR-0003: `git worktree add .worktrees/phase-16-http-filter-rbac-impl -b phase-16-http-filter-rbac-impl master` then `cd` into it.
2. **Master tail.** `git log --oneline master | head -10` shows the PLAN.md commit (this plan) and its SHA-fill follow-up at the head, with the SPEC.md squash commit `3159811` and its SHA-fill follow-up `cedf29a` immediately before, then the BRAINSTORM.md commits `38749ba` (squash) + `b45c1eb` (SHA-fill) + earlier phase 15 commits. If not, the cold-start environment is stale; resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer. `golangci-lint version` reports `1.64.8` (ADR-0009 pin). `docker version` reports both client + server (the differential harness needs Docker).
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `139`. If it returns a higher number, another phase has landed concurrently; re-verify the next-free numbers (phase-16 anticipated ADR-0140..ADR-0146). If it returns `138` or lower, ADR-0139 from phase-15 is missing — re-verify phase-15 phase-done state.
5. **ADR-0125 amendment status.** `grep -nE '\(xi\)' docs/envoy-go/DECISIONS.md | head -3` returns at least one match (the phase-15 §(xi) amendment paragraph). `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md` returns 0 matches at master tip — phase-16's §(xii) amendment is NOT yet landed (anchored at Task 10 per planner-time decision 14). If `(xii)` returns matches, the amendment has been landed concurrently — investigate.
6. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/SPEC.md` returns `3159811` (or descendant). If different, re-read SPEC + re-verify §11 empirical pins.
7. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/PLAN.md` returns the PLAN commit's SHA (filled at PLAN-session end). If a different SHA OR earlier than the SPEC, PLAN has been amended — re-read PLAN.
8. **Pristine tree.** `git status --porcelain` returns empty.
9. **Pre-existing fixtures green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
10. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|...|Test.*0017'` returns every fixture PASS. The 18 pre-existing fixtures (0000–0017) are the regression baseline.
11. **Pre-existing fuzzers run clean at 30s.** The 19 fuzzers from phases 02–15 run clean. Phase 16 adds the twentieth (`FuzzRBACConfigParse` per Task 11).
12. **Reference Envoy image present.** `docker pull envoyproxy/envoy:v1.37.2` returns success; `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin).
13. **`envoy.extensions.filters.http.rbac.v3` + `xds.type.matcher.v3` proto packages present in module closure.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3 RBAC | head -5` returns the `RBAC` proto type's exported fields without an `import path failed` error; `go doc github.com/cncf/xds/go/xds/type/matcher/v3 Matcher | head -5` returns the Matcher proto type's exported fields. If either fails, `go mod download` (or `go mod tidy` if a version bump is needed).
14. **Pre-existing `internal/filter/http/rbac/` directory does NOT exist.** `test ! -d internal/filter/http/rbac && echo "ok: rbac absent"` returns success.
15. **Pre-existing `internal/matcher/` directory does NOT exist.** `test ! -d internal/matcher && echo "ok: matcher absent"` returns success.
16. **Pre-existing `fixture.HTTPRbac` does NOT exist.** `grep -nE 'HTTPRbac' test/differential/fixture/fixture.go` returns 0 matches.
17. **Pre-existing `cmd/envoy-go/main.go` registers exactly the TEN filters expected at master `cedf29a`** — `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `10` matches: `router`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`. If 11+, another filter has been added concurrently; re-verify the registration ordering before adding the rbac line.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044 ADR-on-impl convention, the 7 ADRs (ADR-0140..ADR-0146) + the ADR-0125 §(xii) amendment paragraph are NOT pre-landed at SPEC commit; each ADR is authored AT its impl-time anchor task. The PROGRESS preamble ANTICIPATES the 7 ADRs (with each ADR's Lands-in-task anchor reproduced from this PLAN's per-ADR table) and records the planner-time decisions resolution.

**Precondition:** worktree exists at `phase-16-http-filter-rbac-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions above report green.
**Artifact:** `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble entry committed; `git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` § above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` with: (a) Preamble section summarizing the 17-precondition verification (verbatim command outputs captured); (b) ADR anticipation summary (the 7-ADR table from `## ADRs introduced by this plan` reproduced verbatim); (c) 19 planner-time decisions reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) Task 1 entry slot for the commit-SHA fill-in at this task's commit time.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 1: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: `internal/filter/http/rbac/` package — `doc.go` + `rbac.go` skeleton (TypeURL, types, compiledConfig + compiledRulesEngine + compiledMatcherEngine + compiledPolicy + compiledPerRoute + factoryState + filter + filterStats + parsePerRoute + resolvePerRouteConfig + buildCompiledConfig + buildCompiledRulesEngine + buildCompiledMatcherEngine STUB + buildCompiledPerRoute + newFilterStats + newFilterStatsIfAbsent + New factory) + `rbac_test.go` Group 1 + Group 2 tests [ADR-0140, ADR-0141]

**Files:**
- Create: `internal/filter/http/rbac/doc.go`
- Create: `internal/filter/http/rbac/rbac.go`
- Create: `internal/filter/http/rbac/rbac_test.go`
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0140 + ADR-0141 in slots after ADR-0139)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 2 entry)

Establishes the rbac package skeleton + the compiledConfig/compiledRulesEngine/compiledMatcherEngine shapes + the filterStats 4-base-counter struct + the New factory + the per-route resolver scaffolding. Task 2 lands two ADRs: ADR-0140 (package shape + decoder-only HTTPFilter + filterStats) + ADR-0141 (compiledConfig + 7-consumed-proto-faithful + dual-engine dispatch + UDPA-field-alias + PGV-mirror validation + LOG-partial divergence-window + CEL three-field silent-ignore). The matcher-engine evaluator at `internal/matcher/` is STUBBED in Task 2 (buildCompiledMatcherEngine returns a sentinel error if invoked); the real implementation lands at Task 3. The Permission/Principal evaluator surface is similarly stubbed (buildPermissionEvaluators + buildPrincipalEvaluators return placeholder errors); the real impl lands at Tasks 4 + 5.

**Precondition:** Task 1 acceptance green.
**Artifact:** new package directory with `doc.go` + `rbac.go` + `rbac_test.go`; ADR-0140 + ADR-0141 in DECISIONS.md with `Lands-in-task: Task 2` fields.
**Acceptance:** Group 1 + Group 2 unit tests PASS; `go test -race -count=1 ./internal/filter/http/rbac/...` exit 0; `go vet ./internal/filter/http/rbac/...` exit 0; `grep -nE '^## ADR-0140|^## ADR-0141' docs/envoy-go/DECISIONS.md` returns 2 matches; `grep -nE '^Lands-in-task: Task 2' docs/envoy-go/DECISIONS.md | wc -l` returns 2.

- [ ] **Step 1: Write the Group 1 + Group 2 failing tests first** (per `superpowers:test-driven-development`). Implement the test cases enumerated in the File structure table for `rbac_test.go` Group 1 (Config parse + buildCompiledConfig — 17 test cases) + Group 2 (buildCompiledConfigPerRoute + parsePerRoute — 7 test cases). Use `mustAny`, `freshFactoryCtx`, `freshFactoryCtxWithRegistry` helpers mirroring phase-15 `bandwidthlimit_test.go` precedent.

- [ ] **Step 2: Run tests to verify they FAIL** (`go test ./internal/filter/http/rbac/ -run 'TestNew_|TestBuildCompiled' -v` — expect BUILD FAIL, package does not exist).

- [ ] **Step 3: Author `doc.go`** — package overview per the File structure table responsibility for `doc.go`.

- [ ] **Step 4: Author `rbac.go`** — types + factory + helpers per the File structure table responsibility for `rbac.go`. Pay specific attention to: (a) the dual-engine dispatch table at `buildCompiledConfig` (rules-wins-when-both-set); (b) the defensive PGV-mirror validation per §1.1 amendment 4; (c) the lexicographic policy-name sort per `rbac.pb.go:268-269`; (d) the silent-ignore of audit_logging_options + 3 CEL fields at `buildCompiledRulesEngine`; (e) the `buildCompiledMatcherEngine` STUB returning a sentinel error (the real impl lands at Task 3); (f) the `buildPermissionEvaluators` + `buildPrincipalEvaluators` STUBS (returning placeholder errors; real impl lands at Tasks 4 + 5).

- [ ] **Step 5: Run tests to verify they PASS** (`go test ./internal/filter/http/rbac/ -run 'TestNew_|TestBuildCompiled' -v` — expect Group 1 + Group 2 tests PASS).

- [ ] **Step 6: Author ADR-0140 + ADR-0141 in DECISIONS.md** — insert two new `## ADR-XXXX:` blocks immediately after ADR-0139 with `Status: Accepted`, `Date: 2026-MM-DD` (impl-time date), `Doctrine: ...`, `Lands-in-task: Task 2` fields + §Context/Decision/Consequences bodies per ADR-0044 ADR-on-impl convention. Body content per the ADR table at `## ADRs introduced by this plan` above.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 2: rbac package skeleton + compiledConfig + filterStats + Group 1+2 tests [ADR-0140, ADR-0141]"
grep -nE '^## ADR-0140|^## ADR-0141' docs/envoy-go/DECISIONS.md
# expect: 2 matches with verbatim titles per the table
```

---

## Task 3: `internal/matcher/` NEW top-level package — `doc.go` + `matcher.go` (Matcher type + New + Evaluate + MatchContext interface + parse-time PARSE-REJECT for unknown terminal TypeURLs + initial predicate evaluators) + `matcher_test.go` [ADR-0142]

**Files:**
- Create: `internal/matcher/doc.go`
- Create: `internal/matcher/matcher.go`
- Create: `internal/matcher/matcher_test.go`
- Modify: `internal/filter/http/rbac/rbac.go` (replace `buildCompiledMatcherEngine` STUB with real `matcher.New(m, []string{actionTypeURL})` call)
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0142 in slot after ADR-0141)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 3 entry)

Lands the NEW top-level `internal/matcher/` framework primitive per ADR-0142. Package exports `Matcher` opaque type + `New(tree, supportedActionTypes)` constructor + `Evaluate(MatchContext)` walker + `MatchContext` interface. Cross-phase reusable. Initial `MatchContext` accessor subset scoped to RBAC's canonical surface per planner-time decision 17 (Header / Path / Method / SourceIP / DestinationIP / DestinationPort / RequestedServerName). Parse-time PARSE-REJECT for unknown terminal `Any.TypeUrl` per §11.P3 + §2.6.

**Precondition:** Task 2 acceptance green.
**Artifact:** new package `internal/matcher/` directory with `doc.go` + `matcher.go` + `matcher_test.go`; rbac.go `buildCompiledMatcherEngine` updated to call `matcher.New`; ADR-0142 in DECISIONS.md with `Lands-in-task: Task 3`.
**Acceptance:** matcher unit tests PASS; rbac package's matcher-engine path tests (subset of Group 5) PASS; `go test -race -count=1 ./internal/matcher/... ./internal/filter/http/rbac/...` exit 0; `grep -nE '^## ADR-0142' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write the matcher_test.go failing tests** per the File structure table responsibility (~12-18 test cases).

- [ ] **Step 2: Run tests to verify they FAIL** (BUILD FAIL — package does not exist).

- [ ] **Step 3: Author `internal/matcher/doc.go` + `matcher.go`** per the File structure table.

- [ ] **Step 4: Run tests to verify they PASS**.

- [ ] **Step 5: Replace `buildCompiledMatcherEngine` STUB in `internal/filter/http/rbac/rbac.go`** with the real `matcher.New(m, []string{actionTypeURL})` call. Update Group 5's matcher-engine subset tests to verify the canonical RBAC `Action` terminal acceptance + unknown-TypeURL PARSE-REJECT propagation through `buildCompiledMatcherEngine`.

- [ ] **Step 6: Author ADR-0142 in DECISIONS.md** — `Status: Accepted`, `Lands-in-task: Task 3`, §Context/Decision/Consequences per the ADR table.

- [ ] **Step 7: Commit**

```bash
git add internal/matcher/ internal/filter/http/rbac/rbac.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 3: internal/matcher framework primitive + rbac matcher-engine wiring [ADR-0142]"
```

---

## Task 4: `evaluator.go` — Permission Large 11 evaluators + AND/OR/NOT recursive combinators + deferred PARSE-REJECT (3 Permission variants) + Group 3 tests [ADR-0143 partial]

**Files:**
- Create: `internal/filter/http/rbac/evaluator.go` (partial — Permission surface only at this task; Principal added at Task 5)
- Modify: `internal/filter/http/rbac/rbac.go` (replace `buildPermissionEvaluators` STUB with real impl using evaluator.go's entry point)
- Modify: `internal/filter/http/rbac/rbac_test.go` (Group 3 tests — 15 test cases)
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0143 in slot after ADR-0142; the ADR's body covers BOTH Permission + Principal surfaces — Principal-related sections marked TODO-at-Task-5)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 4 entry)

Lands the Permission evaluator subset of ADR-0143. Permission Large 11 (`permAny`, `permHeader`, `permURLPath`, `permDestIP`, `permDestPort`, `permDestPortRange`, `permSNI`, `permAnd`, `permOr`, `permNot`, `permSourcedMetadata`) + AND/OR/NOT recursive combinator implementations + the 3 deferred Permission variants PARSE-REJECT (`Permission_Metadata` + `Permission_Matcher` + `Permission_UriTemplate` per §2.3 + §11.P12). Reuses phase-07.1 cors-precedent PathMatcher / HeaderMatcher / StringMatcher / CidrRange evaluator infrastructure via shared adapters at evaluator.go.

**Precondition:** Task 3 acceptance green.
**Artifact:** new `evaluator.go` with Permission surface; rbac.go `buildPermissionEvaluators` updated; ADR-0143 in DECISIONS.md with `Lands-in-task: Tasks 4 + 5` (per the ADR's split-anchor disposition).
**Acceptance:** Group 3 tests PASS; `go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPerm' -v` exit 0.

- [ ] **Step 1: Write Group 3 failing tests** (15 Permission test cases per the File structure table).

- [ ] **Step 2: Run tests to verify they FAIL**.

- [ ] **Step 3: Author `evaluator.go` Permission surface** — 11 evaluator types + AND/OR/NOT recursion + `buildOnePermission` switch + PARSE-REJECT for 3 deferred + shared-infrastructure adapters (`matchString`, `matchCidr`).

- [ ] **Step 4: Replace `buildPermissionEvaluators` STUB in rbac.go** with real impl.

- [ ] **Step 5: Run tests to verify they PASS**.

- [ ] **Step 6: Author ADR-0143 in DECISIONS.md** — body covers BOTH Permission + Principal surfaces; Principal-related sections marked `<!-- TODO at Task 5 -->` placeholders that Task 5 fills in.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 4: evaluator.go Permission Large 11 + AND/OR/NOT + Group 3 tests [ADR-0143 partial]"
```

---

## Task 5: `evaluator.go` — Principal Large 11 evaluators + `prinAuthenticated` three-case algorithm + deferred PARSE-REJECT (3 Principal variants including the NEW `Principal_Custom` per §1.1 amendment 7) + Group 4 tests [ADR-0143 finalized]

**Files:**
- Modify: `internal/filter/http/rbac/evaluator.go` (add Principal surface)
- Modify: `internal/filter/http/rbac/rbac.go` (replace `buildPrincipalEvaluators` STUB with real impl)
- Modify: `internal/filter/http/rbac/rbac_test.go` (Group 4 tests — 14 test cases)
- Modify: `docs/envoy-go/DECISIONS.md` (finalize ADR-0143 — fill in Principal-related sections; remove TODO placeholders)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 5 entry)

Lands the Principal evaluator subset of ADR-0143. Principal Large 11 (`prinAny`, `prinAuthenticated` with three-case algorithm per §1.1 amendment 12, `prinDirectRemoteIP`, `prinRemoteIP`, `prinHeader`, `prinURLPath`, `prinAnd`, `prinOr`, `prinNot`, `prinSourcedMetadata`, `prinFilterState`) + AND/OR/NOT recursive combinator implementations + the 3 deferred Principal variants PARSE-REJECT (`Principal_SourceIp` + `Principal_Metadata` + `Principal_Custom` per §2.4 + §1.1 amendment 7 NEW). The `prinAuthenticated` three-case algorithm consumes the `ctx.DownstreamPrincipal() []string` accessor (which returns nil at this task; the real accessor wires up at Task 6).

**Precondition:** Task 4 acceptance green.
**Artifact:** evaluator.go extended with Principal surface; rbac.go `buildPrincipalEvaluators` updated; ADR-0143 finalized (no TODO placeholders).
**Acceptance:** Group 4 tests PASS; `go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestPrin' -v` exit 0; `grep -nE 'TODO at Task 5' docs/envoy-go/DECISIONS.md` returns 0 matches.

- [ ] **Step 1: Write Group 4 failing tests** (14 Principal test cases per the File structure table; the `TestPrinAuthenticated_ThreeCaseAlgorithm` test uses a mock evalContext with a configurable principals slice).

- [ ] **Step 2: Run tests to verify they FAIL**.

- [ ] **Step 3: Extend `evaluator.go` with Principal surface** — 11 evaluator types + AND/OR/NOT recursion + `buildOnePrincipal` switch + PARSE-REJECT for 3 deferred + the `prinAuthenticated` three-case algorithm per §6.6 + §1.1 amendment 12.

- [ ] **Step 4: Replace `buildPrincipalEvaluators` STUB in rbac.go**.

- [ ] **Step 5: Run tests to verify they PASS**.

- [ ] **Step 6: Finalize ADR-0143** — fill in Principal-related sections in DECISIONS.md; remove `<!-- TODO at Task 5 -->` placeholders.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 5: evaluator.go Principal Large 11 + prinAuthenticated three-case + Group 4 tests [ADR-0143 finalized]"
```

---

## Task 6: TLS-principal accessor framework primitive — `DecoderFilterCallbacks.DownstreamPrincipal() []string` + plumbing from connection-level TLS state through HCM dispatch to per-stream filter-callback + chain_test.go probe-filter integration tests + Group 7 tests [ADR-0144]

**Files:**
- Modify: `internal/filter/http/callbacks.go` (add `DownstreamPrincipal() []string` to `DecoderFilterCallbacks` interface; +1 LoC)
- Modify: `internal/filter/http/chain.go` (`tlsPrincipals` per-stream field + `decoderCB.DownstreamPrincipal` impl + `chain.SetTLSPrincipals` accessor; ~+30 LoC)
- Modify: `internal/filter/http/chain_test.go` (3 probe-filter-driven integration tests; ~+60 LoC)
- Modify: `internal/filter/hcm/connection.go` (H1 dispatch: TLS principal extraction + `chain.SetTLSPrincipals` call; ~+25 LoC)
- Modify: `internal/filter/hcm/h2dispatch.go` (H2 dispatch: symmetric to H1; ~+15 LoC)
- Modify: `internal/filter/http/rbac/rbac_test.go` (Group 7 tests — 5 DownstreamPrincipal + 3-case algorithm integration cases)
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0144 in slot after ADR-0143)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 6 entry)

Lands the SECOND of two framework primitives per ADR-0144. The accessor returns priority-ordered TLS principal-name candidates (URI SAN → DNS SAN → Subject DN CN per `rbac.pb.go:1432-1438` + §1.1 amendment 12 + §11.P14). HCM dispatch (both H1 + H2) extracts `tls.ConnectionState` from the connection's underlying `*tls.Conn`, concatenates principal candidates in priority order, and threads into `chain.SetTLSPrincipals(principals)` before `RunDecodeHeaders` dispatch. The `prinAuthenticated` three-case algorithm from Task 5 now reads real principals via the accessor.

**Precondition:** Task 5 acceptance green.
**Artifact:** callbacks.go interface extended; chain.go + chain_test.go updated; hcm/connection.go + hcm/h2dispatch.go updated; Group 7 tests added; ADR-0144 in DECISIONS.md.
**Acceptance:** Group 7 + chain_test.go DownstreamPrincipal tests PASS; `go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...` exit 0; `grep -nE '^## ADR-0144' docs/envoy-go/DECISIONS.md` returns 1 match; race-test on the existing 38+ packages still clean.

- [ ] **Step 1: Write chain_test.go probe-filter integration tests + Group 7 tests failing** (per the File structure table).

- [ ] **Step 2: Run tests to verify they FAIL** (BUILD FAIL — `DownstreamPrincipal()` method on `DecoderFilterCallbacks` does not exist).

- [ ] **Step 3: Add `DownstreamPrincipal() []string` interface method** to `DecoderFilterCallbacks` in callbacks.go.

- [ ] **Step 4: Implement `chain.go` plumbing** — `tlsPrincipals` per-stream field + `decoderCB.DownstreamPrincipal` impl + `chain.SetTLSPrincipals` accessor.

- [ ] **Step 5: Run chain_test.go tests** to verify probe-filter-driven plumbing passes.

- [ ] **Step 6: Wire HCM dispatch (H1 + H2)** — extract `tls.ConnectionState`, build priority-ordered principal list, call `chain.SetTLSPrincipals` before `RunDecodeHeaders`.

- [ ] **Step 7: Run Group 7 tests** to verify the prinAuthenticated three-case algorithm now consumes real principals from end-to-end mTLS path.

- [ ] **Step 8: Author ADR-0144** — `Status: Accepted`, `Lands-in-task: Task 6`, §Context/Decision/Consequences per the ADR table.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 6: DownstreamPrincipal framework primitive + HCM plumbing + Group 7 + chain integration tests [ADR-0144]"
```

---

## Task 7: `DecodeHeaders` body — dual-engine dispatch + SendLocalReply 403 wire shape + per-route resolution + Group 5 (dispatch) + Group 6 (DecodeHeaders gating) tests

**Files:**
- Modify: `internal/filter/http/rbac/rbac.go` (fill in `DecodeHeaders` body + `evaluateEngine` + `evaluateRulesEngine` + `evaluateMatcherEngine` + `policyMatches` + `emitPrimaryCounters` STUB; the real counter emission lands at Task 8)
- Modify: `internal/filter/http/rbac/rbac_test.go` (Group 5 dispatch tests — 12 cases; Group 6 DecodeHeaders gating tests — 9 cases including 19-byte body + 4-header set + LOG-allowed-counter)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 7 entry)

Lands the request-time dispatch surface. DecodeHeaders resolves per-route TPFC, builds evalContext, runs primary engine (rules-or-matcher), runs shadow engine (if configured), applies disposition. ALLOWED → HeaderContinue. DENIED → SendLocalReply(403, "RBAC: access denied", {Content-Type: text/plain}) + HeaderStopIteration. LOG-partial → HeaderContinue + `allowed` counter increments per §1.1 amendments 5 + 8. Counter emission STUB at Task 7 (the real `newFilterStats` registration + post-Freeze idempotent allocation lands at Task 8 with ADR-0145).

**Precondition:** Task 6 acceptance green.
**Artifact:** rbac.go DecodeHeaders body finalized; Groups 5 + 6 tests passing.
**Acceptance:** Groups 5 + 6 tests PASS; `go test -race -count=1 ./internal/filter/http/rbac/ -run 'TestEvaluate|TestDecodeHeaders' -v` exit 0.

- [ ] **Step 1: Write Group 5 + Group 6 failing tests** (21 test cases per the File structure table).

- [ ] **Step 2: Run tests to verify they FAIL**.

- [ ] **Step 3: Implement DecodeHeaders body + dispatch helpers** — `evaluateEngine` / `evaluateRulesEngine` / `evaluateMatcherEngine` / `policyMatches` per SPEC §6.7 + §6.9.

- [ ] **Step 4: Run tests to verify they PASS**.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 7: DecodeHeaders dual-engine dispatch + SendLocalReply 403 + Groups 5+6 tests"
```

---

## Task 8: Stat surface finalization — `newFilterStats` registration helper (4 base counters under SN2-reuse namespace) + per-route INDEPENDENT-stats wiring (`newFilterStatsIfAbsent` + `resolvePerRouteConfig` lazy-cache via `LoadOrStore`) + Group 9 stats integration tests [ADR-0145]

**Files:**
- Modify: `internal/filter/http/rbac/rbac.go` (replace `newFilterStats` + `newFilterStatsIfAbsent` STUBS with real impl; finalize `emitPrimaryCounters` + `emitShadowCounters` + lazy per-policy counter family via `sync.Map` per ADR-0145)
- Modify: `internal/filter/http/rbac/rbac_test.go` (Group 9 stats-namespace integration tests — 5 cases)
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0145 in slot after ADR-0144)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 8 entry)

Lands the stat surface per ADR-0145. 4 base counters (allowed/denied/shadow_allowed/shadow_denied) per active stat-prefix namespace combination. Internal stat path `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for primary; analogous for shadow. SN2-reuse hypothesis — NO new SN10 rule pending impl-time empirical scrape (if scrape ratifies, ADR-0145 amends in-place per planner-time discipline; if refutes, ADR amends to introduce SN10). Per-policy counter family lazy-allocated when `track_per_rule_stats: true` via `sync.Map` keyed by `<policy_name>.<suffix>` + `NewCounterIfAbsent` post-Freeze idempotent. Per-route INDEPENDENT-stats via `newFilterStatsIfAbsent`.

**Precondition:** Task 7 acceptance green.
**Artifact:** rbac.go newFilterStats + newFilterStatsIfAbsent + emitPrimaryCounters + emitShadowCounters + per-policy lazy-allocation finalized; Group 9 tests passing; ADR-0145 in DECISIONS.md.
**Acceptance:** Group 9 tests PASS; full Group 1-9 test run clean; counter-emission tests in Groups 5 + 6 + 8 + 9 all green; `grep -nE '^## ADR-0145' docs/envoy-go/DECISIONS.md` returns 1 match. **Impl-time empirical scrape** of reference Envoy v1.37.2 stats output for the 0018 fixture's listener config — verify the actual stat namespace shape against the SN2-reuse hypothesis; if divergent, amend ADR-0145 in-place at this task (per planner-time discipline + phase-13 ADR-0127-v2 in-place-amendment precedent).

- [ ] **Step 1: Write Group 9 failing tests** (5 stats-namespace integration cases per the File structure table).

- [ ] **Step 2: Run tests to verify they FAIL**.

- [ ] **Step 3: Implement newFilterStats + newFilterStatsIfAbsent** — register 4 base counters under SN2-reuse namespace; lazy per-policy counter cache via `sync.Map`.

- [ ] **Step 4: Finalize emitPrimaryCounters + emitShadowCounters + per-policy increment helpers**.

- [ ] **Step 5: Run tests to verify they PASS**.

- [ ] **Step 6: Impl-time empirical scrape** — start a reference Envoy with the 0018 fixture's listener config; scrape `/stats/prometheus`; verify the actual stat namespace shape matches `http.<HCM>.rbac.<prefix>.<counter>`. If divergent (e.g., flat `<prefix>.rbac.<counter>` mirroring phase-15 bandwidth_limit's shape per §1.1 amendment 9 release-valve), amend ADR-0145 §Decision in-place to introduce SN10 rule.

- [ ] **Step 7: Author ADR-0145** — `Status: Accepted`, `Lands-in-task: Task 8`, §Context/Decision/Consequences per the ADR table.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 8: stat surface finalization + per-route INDEPENDENT-stats + Group 9 tests [ADR-0145]"
```

---

## Task 9: Shadow + LOG-partial + `track_per_rule_stats` per-policy emission discipline finalization [ADR-0146]

**Files:**
- Modify: `internal/filter/http/rbac/rbac.go` (finalize shadow-path orchestration in DecodeHeaders + LOG-partial divergence-window codification + per-policy counter family lazy-allocation post-Freeze idempotent)
- Modify: `internal/filter/http/rbac/rbac_test.go` (additional shadow-path test cases extending Groups 5 + 9 + targeted LOG-partial + track_per_rule_stats cases)
- Modify: `docs/envoy-go/DECISIONS.md` (insert ADR-0146 in slot after ADR-0145)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 9 entry)

Codifies the shadow-evaluation discipline (parallel-to-primary; never affects disposition; emits shadow_* counters) + LOG-partial divergence-window (always-allow + match-evaluated + `access_log_hint` metadata silent; `allowed` counter increments — no separate `logged` counter per amendment 8) + per-policy emission discipline (lazy `NewCounterIfAbsent` per matched policy) + `response_code_details` divergence-window. The shadow + LOG-partial codification per ADR-0146 finalizes the runtime behavior; the BEHAVIOR_CONTRACT §13.4 `### Phase 16 forward-pointer notes` subsection content is anchored here (the actual BEHAVIOR_CONTRACT edit lands at Task 15).

**Precondition:** Task 8 acceptance green.
**Artifact:** rbac.go shadow-path orchestration finalized; LOG-partial + track_per_rule_stats tests passing; ADR-0146 in DECISIONS.md.
**Acceptance:** all unit tests PASS; full `go test -race -count=1 ./internal/filter/http/rbac/... ./internal/matcher/...` exit 0; `grep -nE '^## ADR-0146' docs/envoy-go/DECISIONS.md` returns 1 match.

- [ ] **Step 1: Write additional shadow + LOG-partial + track_per_rule_stats failing tests** extending Groups 5 + 9.

- [ ] **Step 2: Run tests to verify they FAIL**.

- [ ] **Step 3: Finalize rbac.go shadow-path orchestration + LOG-partial + per-policy lazy-allocation**.

- [ ] **Step 4: Run tests to verify they PASS**.

- [ ] **Step 5: Author ADR-0146** — `Status: Accepted`, `Lands-in-task: Task 9`, §Context/Decision/Consequences per the ADR table (covers shadow + LOG-partial + track_per_rule_stats + response_code_details divergence-window + BEHAVIOR_CONTRACT §13.4 forward-pointer notes anchoring).

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/rbac/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 9: shadow + LOG-partial + track_per_rule_stats per-policy emission [ADR-0146]"
```

---

## Task 10: ADR-0125 in-place amendment paragraph §(xii) — phase-16 introduces the 7th canonical per-route pattern

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (insert §(xii) amendment paragraph in the ADR-0125 amendment-block, after the existing §(xi) paragraph from phase-15)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 10 entry)

Lands the ADR-0125 in-place amendment paragraph §(xii) per planner-time decision 14 (NOT pre-landed at SPEC commit despite SPEC §5.4 prose claim). The amendment paragraph documents phase-16 rbac as the FIRST row to use the **7th canonical per-route pattern**: a wrapper proto (`RBACPerRoute`) with reserved field 1 + a single optional sub-message field (`rbac` at field 2); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment (per Envoy v1.37.2 proto comment `"If absent, RBAC policy will be disabled for this route."`); PRESENCE-of-the-sub-message-field implies wholesale-override of the listener-level config (mirrors ADR-0073 wholesale-not-merge). Structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof (phase-13 buffer + phase-14 compressor) AND 6th canonical's bare-message-via-TPFC + code-level-required-field (phase-15 bandwidth_limit). The 7th canonical's stat-discipline is INDEPENDENT (per ADR-0145; mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT). ADR-0125's canonical-pattern roster grows from 6 to 7.

The verbatim amendment paragraph text is captured at SPEC §5.4 — Task 10 reproduces it verbatim into DECISIONS.md within the ADR-0125 amendment block at the slot immediately after the §(xi) amendment paragraph from phase-15.

**Precondition:** Task 9 acceptance green.
**Artifact:** DECISIONS.md ADR-0125 amendment block extended with §(xii) paragraph.
**Acceptance:** `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md` returns at least 1 match; `grep -nE '7th canonical' docs/envoy-go/DECISIONS.md` returns at least 1 match referencing phase 16; ADR-0125 canonical-pattern catalog mentions both 6 (phase-15 6th canonical) AND 7 (phase-16 7th canonical) entries.

- [ ] **Step 1: Locate ADR-0125 amendment block** — `grep -nE 'Amendment .per phase 15' docs/envoy-go/DECISIONS.md` returns the line of the §(xi) amendment paragraph; the §(xii) amendment paragraph inserts immediately after.

- [ ] **Step 2: Author the §(xii) amendment paragraph** in DECISIONS.md — reproduce verbatim from SPEC §5.4 (`Phase 16 rbac is the FIRST row to use the **7th canonical per-route pattern**: a wrapper proto (RBACPerRoute) with reserved field 1 + a single optional sub-message field (rbac at field 2); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment...`) per planner-time decision 14.

- [ ] **Step 3: Verify** `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md` returns at least 1 match.

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 10: ADR-0125 §(xii) amendment paragraph — 7th canonical per-route pattern"
```

---

## Task 11: `cmd/envoy-go/main.go` register `rbac.New` under `rbac.TypeURL` + fixture infrastructure (`BackendKind=HTTPRbac` enum + runner spawn helper switch-case) + `FuzzRBACConfigParse` 20th fuzzer

**Files:**
- Modify: `cmd/envoy-go/main.go` (add import + register line)
- Modify: `test/differential/fixture/fixture.go` (add `HTTPRbac BackendKind = 15` enum value)
- Modify: `test/differential/runner_test.go` (blank-import + switch-case for HTTPRbac reusing `startEchoBackend` helper)
- Create: `internal/filter/http/rbac/fuzz_test.go` (FuzzRBACConfigParse — 20th fuzzer)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 11 entry)

Wires the rbac filter into the boot registry + adds the fixture-runner infrastructure for the 0018 fixture + adds the 20th fuzzer. The fuzzer mirrors phase-14/15 fuzzer shape extended for the 7-field outer RBAC + nested rules-engine + nested matcher-engine + AND/OR/NOT recursion.

**Precondition:** Task 10 acceptance green.
**Artifact:** main.go + fixture.go + runner_test.go + fuzz_test.go updated; 20 fuzzers green at 30s.
**Acceptance:** `go build ./cmd/envoy-go/` exit 0; `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 11; `grep -nE 'HTTPRbac' test/differential/fixture/fixture.go` returns 1; `go test -fuzz=FuzzRBACConfigParse -fuzztime=30s ./internal/filter/http/rbac/` clean exit (no failure messages); seed corpus runs as regression tests via `go test ./internal/filter/http/rbac/ -run FuzzRBACConfigParse -v`.

- [ ] **Step 1: Add rbac import + register line** to main.go (alphabetical-after-localratelimit per ADR-0140 §Decision (v)).

- [ ] **Step 2: Add HTTPRbac BackendKind enum value** to fixture.go.

- [ ] **Step 3: Add blank-import + switch-case** to runner_test.go (reuses existing `startEchoBackend` helper from phase-14 Task 10).

- [ ] **Step 4: Author FuzzRBACConfigParse fuzzer** in `internal/filter/http/rbac/fuzz_test.go` per the File structure table responsibility (~80 LoC; 13-seed corpus).

- [ ] **Step 5: Run fuzzer at 30s budget** — `go test -fuzz=FuzzRBACConfigParse -fuzztime=30s ./internal/filter/http/rbac/`. Expect clean exit.

- [ ] **Step 6: Run all 20 fuzzers regression** at 30s each — phase 02-14 fuzzers + phase-15 FuzzBandwidthLimitConfigParse + phase-16 FuzzRBACConfigParse. All clean.

- [ ] **Step 7: Commit**

```bash
git add cmd/envoy-go/main.go test/differential/fixture/fixture.go test/differential/runner_test.go internal/filter/http/rbac/fuzz_test.go docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 11: main.go register + fixture infra + FuzzRBACConfigParse (20th fuzzer)"
```

---

## Task 12: Fixture 0018 — `inputs/driver.go` (8-scenario driver including mTLS scenario 6; byte-exact body comparison + per-counter delta scrape + INDEPENDENT-stats assertion)

**Files:**
- Create: `test/fixtures/0018-http-rbac/inputs/driver.go`
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 12 entry)

Lands the 8-scenario fixture driver mirroring phase-15 driver shape. Scenarios 1-5, 7, 8 use HTTP/1.1 plaintext; scenario 6 uses HTTP/1.1-over-mTLS via a fresh `http.Client` with client cert presented. Per-scenario assertions: byte-exact body (allow paths verbatim; deny paths 19-byte `RBAC: access denied`) + per-counter delta byte-equivalence on the 4 base counters per active namespace + INDEPENDENT-stats discipline verification for scenarios 7 + 8.

**Precondition:** Task 11 acceptance green.
**Artifact:** new driver.go file; integrates with the existing differential runner via `fixture.RegisterFixture("0018-http-rbac", &rbacDriver{})` per phase-15 convention.
**Acceptance:** driver compiles; the fixture is registered (verified via `grep -nE 'RegisterFixture\("0018-http-rbac"' test/fixtures/0018-http-rbac/inputs/driver.go` returning 1). Note: driver doesn't run end-to-end until Task 14 (YAMLs + expectations + PKI generation needed first).

- [ ] **Step 1: Author driver.go** per the File structure table responsibility — `runScenario1..runScenario8` functions + `runTLSScenario6` helper using mTLS-capable http.Client + counter-delta scrape helper + 4-base-counter assertion logic. ~290 LoC.

- [ ] **Step 2: Verify compilation** — `go build ./test/fixtures/0018-http-rbac/inputs/...`.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0018-http-rbac/inputs/ docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 12: Fixture 0018 driver — 8 scenarios incl. mTLS"
```

---

## Task 13: Fixture 0018 — `envoy.yaml` + `envoy-go.yaml` bootstraps (three-listener topology + cluster `c_backend_b`) + `pki/gen.go` mTLS PKI generation

**Files:**
- Create: `test/fixtures/0018-http-rbac/envoy.yaml`
- Create: `test/fixtures/0018-http-rbac/envoy-go.yaml`
- Create: `test/fixtures/0018-http-rbac/pki/gen.go`
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 13 entry)

Lands the three-listener fixture bootstraps + the mTLS PKI generation. Reference + subject bootstraps both with `l_test_a` (plaintext HCM filter chain `rbac → router` with the 4-policy listener-level RBAC config) + `l_test_b` (echo-backend listener) + `l_test_a_tls` (mTLS-required HCM filter chain with 5-policy RBAC config including `authenticated_admin`). The PKI generator produces fresh fixture-CA + server cert + client cert (URI SAN `spiffe://example.com/admin`) at fixture-load time per planner-time decision 11.

**Precondition:** Task 12 acceptance green.
**Artifact:** YAMLs + pki/gen.go.
**Acceptance:** YAMLs lint clean; PKI generator produces valid x509 certs (verified via standalone Go test of `gen.go`).

- [ ] **Step 1: Author pki/gen.go** — fresh-cert generation per the File structure table responsibility (~120 LoC).

- [ ] **Step 2: Author envoy.yaml** — three-listener bootstrap per the File structure table (~165 LoC).

- [ ] **Step 3: Author envoy-go.yaml** — equivalent envoy-go bootstrap (~165 LoC).

- [ ] **Step 4: Verify YAML lint** — `docker run --rm -v $(pwd):/data envoyproxy/envoy:v1.37.2 -c /data/test/fixtures/0018-http-rbac/envoy.yaml --mode validate` exit 0 (reference Envoy validates the YAML structure). Repeat for envoy-go.yaml via `go run ./cmd/envoy-go -c test/fixtures/0018-http-rbac/envoy-go.yaml --mode validate` (or analogous validation invocation).

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0018-http-rbac/ docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 13: Fixture 0018 YAMLs + mTLS PKI gen"
```

---

## Task 14: Fixture 0018 — `expectations.yaml` + `README.md` + driver counter-assertion fleshing + end-to-end differential pass (all 8 scenarios + all 19 fixtures)

**Files:**
- Create: `test/fixtures/0018-http-rbac/expectations.yaml`
- Create: `test/fixtures/0018-http-rbac/README.md`
- Modify: `test/fixtures/0018-http-rbac/inputs/driver.go` (finalize counter-delta + INDEPENDENT-stats assertion logic with concrete expected values from runs against reference Envoy)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 14 entry)

Lands the expectations.yaml + README documentation + finalizes the driver's counter-delta assertions with concrete expected values from running the fixture against reference Envoy. Validates the full 8-scenario differential pass. ALL 19 differential fixtures (0000-0018) green at this task's end.

**Precondition:** Task 13 acceptance green.
**Artifact:** expectations.yaml + README.md + finalized driver.go.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|...|Test.*0018'` returns every fixture PASS including 0018; total runtime ~60-90s wallclock per SPEC §14.6.

- [ ] **Step 1: Run fixture 0018 dry-run against reference Envoy** — capture actual per-scenario counter deltas + body byte counts + header sets.

- [ ] **Step 2: Author expectations.yaml** — per-scenario allow-list + counter-delta map per the captured reference data.

- [ ] **Step 3: Author README.md** — fixture overview + 8-scenario narrative + mTLS PKI notes + 7th canonical per-route notes + INDEPENDENT-stats notes + divergence-window notes.

- [ ] **Step 4: Finalize driver counter-assertion logic** — replace placeholder expected values with concrete reference-side values.

- [ ] **Step 5: Run end-to-end differential** — `go test -count=1 -v ./test/differential/ -run 'Test.*0018'` PASS.

- [ ] **Step 6: Run full regression** — `go test -count=1 ./test/differential/ -run 'Test.*'` ALL 19 fixtures PASS.

- [ ] **Step 7: Commit**

```bash
git add test/fixtures/0018-http-rbac/ docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 14: Fixture 0018 expectations + README + end-to-end differential pass (19 fixtures green)"
```

---

## Task 15: BEHAVIOR_CONTRACT.md 6-edit bundle + ROADMAP row 16 in-progress→done + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§13.1 + §13.2 + §13.3 + §13.4 + §13.5 + §13.6 — 6 patches per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (row 16 `in-progress → done`)
- Modify: `docs/envoy-go/STATE.md` (advance to `phase 16 done; awaiting next planning`)
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 15 entry + 6-gate report)

Lands the BEHAVIOR_CONTRACT.md 6 patches per SPEC §13 + flips ROADMAP row 16 + advances STATE.md + runs the 6 phase-done gates per BOOTSTRAP_PROMPT.md §7.5.

**Precondition:** Task 14 acceptance green.
**Artifact:** BEHAVIOR_CONTRACT.md + ROADMAP.md + STATE.md updated; 6-gate report appended to PROGRESS.md.
**Acceptance:** All 6 phase-done gates green per the table at SPEC §14.7:
- Gate A: `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0.
- Gate B: `go test -race -count=1 ./...` exit 0 across all packages including the new `internal/matcher/` + `internal/filter/http/rbac/` packages.
- Gate C: h2spec 53/53 PASS at ADR-0051 pin.
- Gate D: 20 fuzzers green at 30s/each.
- Gate E: 19 differential fixtures (0000-0018) PASS.
- Gate F: BEHAVIOR_CONTRACT.md §13.1-§13.6 populated per SPEC §13 patches.

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md §13.1 patch** — insert `### envoy.filters.http.rbac` subsection AFTER `### envoy.filters.http.bandwidth_limit` at line 1416 per planner-time decision 19 (~230 LoC).

- [ ] **Step 2: Apply §13.2 patch** — stat-table 60→64 names extension (4 new active rows + per-policy template-form documentation) (~30 LoC).

- [ ] **Step 3: Apply §13.3 patch** — Equivalence Matrix new row for fixture 0018 (~3 LoC).

- [ ] **Step 4: Apply §13.4 patch** — `### Phase 16 forward-pointer notes` subsection appended to existing `## Forward-pointer notes` section (~100 LoC).

- [ ] **Step 5: Apply §13.5 patch** — `## HTTPFilterCallbacks` extension with `### DownstreamPrincipal accessor` subsection (~25 LoC).

- [ ] **Step 6: Apply §13.6 patch** — NEW `## Matcher engine framework primitive` top-level section (~20 LoC).

- [ ] **Step 7: Flip ROADMAP row 16** to `in-progress → done` + sharpen summary with post-impl counts.

- [ ] **Step 8: Advance STATE.md** — `lifecycle-state: phase 16 done; awaiting next planning`; `last-commit: <Task 15 squash>`; `next-skill: (none — phase complete)`; `last-updated: <impl date>`.

- [ ] **Step 9: Run 6-gate phase-done verification** — execute all 6 gate commands per SPEC §14.7; capture verbatim outputs into PROGRESS.md Task 15 entry; all green.

- [ ] **Step 10: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 15: BEHAVIOR_CONTRACT 6-edit bundle + STATE advance + 6-gate phase-done verification"
```

---

## Task 16: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/16-http-filter-rbac/REVIEW.md`
- Modify: `docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md` (append Task 16 entry)

Lands the end-of-phase review document per `superpowers:requesting-code-review` skill. Document covers: phase-16 deliverables + ADR roster final state (7 anchored + 1 amendment) + SPEC §15 13-item acceptance checklist verification + the 18 §11 empirical-pin dispositions (10 RATIFIED + 2 REFUTED + 3 PARTIAL/REFINED + 3 RATIFIED-PENDING-IMPL-TIME-CONFIRMED-AT-TASK-8 + 1 DEFERRED) + the 12 §1.1 amendment dispositions + the framework-delta impact assessment (TWO new primitives + cross-phase reuse intent) + the divergence-window enumeration (LOG-action metadata + response_code_details + CEL three-field + shadow access-log + sourced-metadata always-no-match + Principal_Authenticated canonical 3 cert fields).

**Precondition:** Task 15 acceptance green.
**Artifact:** new REVIEW.md file.
**Acceptance:** REVIEW.md committed; phase-16 end-state captured.

- [ ] **Step 1: Author REVIEW.md** — structure per `superpowers:requesting-code-review` skill output template + phase-13/14/15 REVIEW.md precedent. ~250 LoC.

- [ ] **Step 2: Commit**

```bash
git add docs/envoy-go/phases/16-http-filter-rbac/REVIEW.md docs/envoy-go/phases/16-http-filter-rbac/PROGRESS.md
git commit -m "phase 16 Task 16: REVIEW.md — end-of-phase review"
```

---

## End of phase 16 implementation plan
