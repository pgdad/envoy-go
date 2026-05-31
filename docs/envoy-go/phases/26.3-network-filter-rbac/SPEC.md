# Phase 26.3 SPEC — `rbac_network` at full upstream parity + shared `internal/rbac/` engine extraction + connection-scoped dynamic-metadata writes

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 26.3** (`network-filter-rbac`), the THIRD and FINAL sub-phase of the phase-26 BRAINSTORM-time 3-way pre-split (26.1 / 26.2 / 26.3). It is authored per the phase-22.1 / 25.1 / 26.1 / 26.2 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/26-network-filter-chain-and-rbac/SPEC.md`) resolved the BRAINSTORM §10 D1–D8 empirical pins (parent §11), formalized the 3-way split surface-mapping (parent §3), and anchored the ADR-0213..0218 §Context drafts; the **26.1 + 26.2 SPEC/PLAN/IMPL** landed the NEW `internal/filter/network/` read-filter chain framework (`ReadFilter`/`ReadFilterCallbacks`/`Connection`/`*dynamicmetadata.Bucket` reuse/`*network.Registry`), the `network.TerminalFilter` connection-takeover seam, the unified `[read-filter*, terminal-filter?]` dispatch + the buffered-prefix `prefixConn` handover, and the `internal/filter/network/builtins.RegisterBuiltins` registration seam. This 26.3 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §12 D-questions + §13 RATIFIED-PENDING items, and **refines per-Task-level surface only**. It does NOT re-execute the parent empirical pins (it adds the §11.1 D-S2 as-built code-surface findings the parent could not pin before 26.1/26.2 landed). It **drafts ADR-0216 + ADR-0217 + ADR-0218 §Context** into DECISIONS.md (the engine-extraction + connection-metadata-sink + `rbac_network` ADRs; §Decision/§Consequences land at 26.3 IMPL per ADR-0044). The next session, per BOOTSTRAP §5, authors the **26.3 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Land `rbac_network` (`envoy.extensions.filters.network.rbac.v3.RBAC`) at FULL upstream parity — enforced rules + shadow rules + the connection-level dynamic-metadata shadow-pair emission + the `allowed`/`denied`/`shadow_allowed`/`shadow_denied` stat roster — by EXTRACTING the phase-16 RBAC principal/permission evaluation engine from `internal/filter/http/rbac/` into a NEW shared `internal/rbac/` primitive (HTTP rbac migrated as consumer #1, re-verified byte-exact by its phase-16 differential fixtures; `rbac_network` as consumer #2), and wiring the connection-scoped `*dynamicmetadata.Bucket` writes the 26.1 framework already exposes. `rbac_network` is the FIRST production consumer of the mixed `[read-filter*, terminal-filter]` chain (allow → `Continue` → `tcp_proxy`/HCM via the `prefixConn` handover). Phase 26.3 phase-done flips parent row 26 `in-progress → done` ATOMICALLY (the 18/19/22/24/25 ROLLUP precedent).

**Architecture:** The phase-16 engine ALREADY evaluates against an abstract `evalContext` interface (`internal/filter/http/rbac/evaluator.go:60-121`) whose L4 accessors (`DestinationIP`/`DestinationPort`/`RequestedServerName`/`DirectRemoteIP`/`RemoteIP`/`DownstreamPrincipal`) are present but STUBBED to nil/zero in the HTTP filter (`rbac.go:965-988`). The extraction is therefore mechanical (AMEND-A11): the engine (the `evalContext`/`permissionEvaluator`/`principalEvaluator` interfaces + the ~23 permission/principal evaluators + the rules-path + matcher-path compilers + the matcher-bridge + the per-policy-counter machinery) MOVES to `internal/rbac/` (~790 LoC), re-exported; the HTTP filter (`New`/`buildCompiledConfig`/per-route/the HTTP-derived `evalContext` impl/the stat-registration wiring) STAYS (~400 LoC) and imports the shared engine. `rbac_network` (`internal/filter/network/rbac/`) supplies a NON-stub L4 `evalContext` built from the `network.Connection` accessors (`LocalAddr`/`RemoteAddr`/`RequestedServerName`/`DownstreamPrincipals`), makes its allow/deny decision in `OnData` (AMEND-A8; OnNewConnection is a `Continue` no-op per the sticky-halt constraint), writes the shadow pair to the per-connection `*dynamicmetadata.Bucket` via `ReadFilterCallbacks.DynamicMetadata()`, and on enforced deny sets the `rbac_deny_close` response-code-details (via the existing `SetResponseCodeDetails` sink) + `Close(NoFlush)`. Two framework touchpoints the 26.1/26.2 framework deferred to "when 26.3 lands" are filled: the real `NoFlush` close semantics (`chain.go:362-368`) and an input-capability profile on the engine builder so the L4 consumer PARSE-REJECTs HTTP-only matcher arms (AMEND-A4) without changing consumer #1's behavior.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/matcher/` (phase-16, the `matcher`-path); `internal/dynamicmetadata/` (phase-22.2, REUSED at connection scope); `internal/stats/` (the counter roster). ZERO new third-party `go.mod` dependencies (the engine + filter + sink are entirely in-house). REUSES the 26.1/26.2 `internal/filter/network/` framework + the phase-16 RBAC engine (extracted, not rewritten).

**Authored:** 2026-05-31. **Empirical-pin probe date (inherited):** 2026-05-30 (parent SPEC §11). **As-built code-surface probe date:** 2026-05-31 (§11.1 D-S2).

---

## 1. Purpose / Mission

Phase 26.3 is the capstone of the phase-26 Network-filters-family opening: it lands the family's first SECURITY filter at full upstream parity and pays down the project's only duplicated-policy-engine risk by extracting the phase-16 RBAC engine into a shared primitive. It is the THIRD §9 row to extract a framework primitive at its Nth consumer (after phase-22.1 `internal/lua` + phase-25.1 `internal/wasm` first-consumer extractions) — and the project's FIRST extraction with a LIVE first consumer (HTTP rbac) that must be migrated + re-verified in the SAME sub-phase. The risk profile is migration-regression (bounded by the phase-16 HTTP-rbac differential fixtures staying byte-exact green — R4) rather than the speculative-future-consumer risk of the 22.1/25.1 extractions.

Three deliverables land atomically (each independently testable; all under the 26.3 sub-phase):

1. **The shared `internal/rbac/` engine** (ADR-0216) — the phase-16 principal/permission evaluation engine MOVED from `internal/filter/http/rbac/` and refactored to evaluate against the abstract `evalContext` it ALREADY uses, gaining an input-capability profile so a consumer can declare which `Permission`/`Principal` arms its input surface supports. HTTP rbac migrates onto it as consumer #1 (byte-exact re-verification — R4).

2. **The connection-scoped dynamic-metadata writes** (ADR-0217) — the 26.1 framework already exposes `ReadFilterCallbacks.DynamicMetadata() *dynamicmetadata.Bucket` (owned by the per-connection `chainRuntime`, reset at `OnDestroy`); 26.3 lands the FIRST production WRITE through it (the rbac shadow pair) + generalizes the `internal/dynamicmetadata/` package doc to scope-agnostic (per-stream OR per-connection) in place (ADR-0044).

3. **The `rbac_network` filter** (ADR-0218) — `envoy.extensions.filters.network.rbac.v3.RBAC` consumer #2: a read filter that decides in `OnData`, allows by `Continue`-ing to the terminal (the first production mixed read→terminal chain), denies by `NoFlush`-closing with the `rbac_deny_close` termination-detail, and emits the shadow pair + the four `<stat_prefix>.rbac.*` counters.

After 26.3 the project has: one RBAC engine (shared by HTTP + network); five network filters (`echo`/`direct_response`/`tcp_proxy`/HCM/`rbac_network`); the first production write through connection-level dynamic metadata; and the first cross-side differential fixture exercising a real `Continue`-to-terminal chain. Parent row 26 `in-progress → done`; the §9 Network-filters family stays OPEN with 6 candidates remaining (`redis`, `mongo`, `kafka_broker`, `thrift`, `zookeeper`, `sni_cluster`).

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 11 AMENDs (A1–A11) in the parent SPEC §1.1 are the empirical-finding-driven scope revisions for phase 26. The amendments load-bearing for **26.3**, plus the NEW as-built code-surface findings this SPEC pins (the parent could not, because 26.1/26.2 had not yet landed):

- **AMEND-A4** (HTTP-only matchers SILENTLY never-match upstream; PARSE-REJECT is an envoy-go-strict DEPARTURE) — `header`/`url_path`/`uri_template` arms. **26.3-SPEC refinement (§3.4 + §6.2):** the extracted engine ALREADY compiles these arms (the phase-16 HTTP filter supports them — `permHeader`/`permURLPath`/`prinHeader`/`prinURLPath` + the `uri_template`/`matcher` extension rejects). To PARSE-REJECT them for the L4 consumer WITHOUT changing consumer #1, the engine builder gains an **input-capability profile** (an allowed-arm predicate); the HTTP profile permits all arms (byte-identical to today → R4), the L4 profile rejects the HTTP-only arms at compile.
- **AMEND-A5** (connection-metadata sink = REUSE `internal/dynamicmetadata/`, NOT a new package) — RATIFIED as-built: the 26.1 `chainRuntime` constructs `dynamicmetadata.NewBucket()` per connection (`chain.go:159`), resets it at `onDestroy` (`chain.go:294-302`), and exposes it via `callbacks.DynamicMetadata()` (`chain.go:334`). 26.3 lands the WRITES; no callbacks-API revision (R2/R5).
- **AMEND-A6** (HTTP rbac emits NO dynamic metadata today — RE-CONFIRMED as-built: grep of `internal/filter/http/rbac/` for `dynamicmetadata`/`shadow_engine_result`/`shadow_effective_policy_id` = ZERO matches). Consumer-#1 re-verification (R4) is engine-EVALUATION correctness (the phase-16 fixtures byte-exact after the engine moves), NOT metadata preservation. The connection-metadata sink is net-new for `rbac_network` only.
- **AMEND-A7** (enforced denial = connection-close + termination-detail, NOT dynamic metadata). **26.3-SPEC refinement (§4.3 + §11.1):** the framework already carries a response-code-details sink — `callbacks.SetResponseCodeDetails(string)` (`chain.go:336-340`, reached via a `interface{ SetResponseCodeDetails(string) }` type-assertion, mirroring direct_response's `DirectResponse` rcd). `rbac_network` enforced-deny sets `rbac_deny_close` via it, then `Close(NoFlush)`. The termination-detail string has no in-repo consumer yet (like the shadow metadata — observable in unit tests + future-consumer-ready).
- **AMEND-A8** (decision made in `OnData`, not `OnNewConnection`). **26.3-SPEC constraint (§4.2; project memory `reference_network_read_filter_onnewconnection_halts`):** `OnNewConnection` MUST return `Continue` (a `StopIteration` there sets the sticky `connHalted` flag that blocks ALL `OnData`). The decision is made in `OnData` — once on first data for `ONE_TIME_ON_FIRST_BYTE` (default), every `OnData` for `CONTINUOUS`.
- **AMEND-A9** (`delay_deny` field → PARSE-REJECT, envoy-go-strict departure) — the synchronous single-goroutine-per-connection read-filter model has no clean timer/`readDisable` seam. PARSE-REJECT stands (D-P3).
- **AMEND-A10** (`matcher`/`shadow_matcher` xDS-unified-matcher fields). The phase-16 engine ALREADY supports the matcher-path (`buildCompiledMatcherEngine` at `rbac.go:445`, `evaluateMatcherEngine` at `:824`, the `matcherCtxAdapter` at `:860-897` bridging `evalContext` → `matcher.MatchContext`). 26.3 supports BOTH `rules` and `matcher` paths via the extracted engine, with the input-capability profile rejecting HTTP-input leaf matchers in either path (§3.4).
- **AMEND-A11** (engine extraction is mechanical; LoC fits) — RATIFIED as-built (§11.1 D-S2): the `evalContext` interface + the L4 stubs are exactly where the parent claimed; the extraction is move + re-export. The MOVE is ~790 LoC; STAY ~400 LoC; net-new ~350-650 LoC. Net-new basis fits one sub-phase (D-P1 resolved at §3.0).

**NEW 26.3-SPEC findings (no parent counterpart — pinned at §11.1 D-S2):**

- **F1 — CEL `condition`/`checked_condition`/`cel_config` are SILENT-IGNORED by the phase-16 engine** (`rbac.go:419-424` + doc.go:33-36, per ADR-0040 silent-ignore discipline) — NOT parse-rejected. **This REFINES the parent §6.3 anticipated `rbac-network-condition-cel-unsupported` arm: there is NO CEL reject. `rbac_network` MIRRORS the silent-ignore** (the extracted engine carries the structural ignore — `compiledPolicy` has no CEL slot). D-P4 RESOLVED.
- **F2 — the network RBAC proto has NO `track_per_rule_stats` field** (8 fields confirmed: `rules`/`shadow_rules`/`stat_prefix`/`enforcement_type`/`shadow_rules_stat_prefix`/`matcher`/`shadow_matcher`/`delay_deny` — §5.3; the HTTP rbac.v3.RBAC's `track_per_rule_stats` has no network analogue). **So `rbac_network` emits ONLY the four static counters; the per-policy dynamic counters (the engine's `incPolicy`/`sync.Map` machinery, `rbac.go:165-222`) stay DORMANT for consumer #2.** The extracted engine retains the machinery for consumer #1 (HTTP, which has the field). D-P5 RESOLVED (network: no per-policy counters; static surface 132 → 136 only).
- **F3 — the `NoFlush` close semantics must be properly implemented at 26.3** — `chain.go:362-368` collapsed `FlushWrite ≡ NoFlush` at 26.1/26.2 (read filters drain writes synchronously before close, so there was no pending write to distinguish) and explicitly defers the distinction to "when 26.3 lands". `rbac_network` enforced-deny uses `NoFlush` (drop-buffered-writes-then-close, upstream `ConnectionCloseType::NoFlush`). 26.3 lands the real `NoFlush` path (a small framework touchpoint folded into ADR-0218).

This 26.3 SPEC makes the F1 refinement (CEL silent-ignore, not reject) as its only SUBSTANTIVE departure from the parent's anticipation; everything else REFINES the parent's surface against the as-built 26.1/26.2 code.

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0215** (the 26.2 migration ADR; §Decision/§Consequences bodies landed in place at 26.2 IMPL); next-free **ADR-0216**. This SPEC DRAFTS the §Context for the three 26.3 ADRs — **ADR-0216** (`internal/rbac/` extraction), **ADR-0217** (connection-scoped dynamic-metadata writes), **ADR-0218** (`rbac_network` filter) — into DECISIONS.md (DECISIONS.md tail advances ADR-0215 → **ADR-0218**; next-free **ADR-0219**). No ADR §Decision/§Consequences body is consumed at SPEC time (a SPEC drafts §Context only; the bodies land at 26.3 IMPL per ADR-0044). The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. The parent §12 D-questions in 26.3 territory are resolved at this SPEC (D-P1 §3.0; D-P4 §6.2/F1; D-P5 §7.2/F2) or carried to 26.3 IMPL (D-P3 `delay_deny` reconsideration STANDS-REJECT; D-P6/D-P7 IMPL-time wording). The NEW 26.3-additive D-questions are §12 (D-26.3-1..7).

---

## 2. Non-purposes

Phase 26.3 lands `rbac_network` + the engine extraction + the connection-metadata writes, under three NEW ADRs (ADR-0216/0217/0218). It does NOT:

- **2.1 No write-filter chain.** Read-filter-only + the 26.2 terminal seam remain the scope (parent §2.1 / BRAINSTORM Q4 / ADR-0213 API-revision allowance). `rbac_network` is a read filter. The write-filter allowance is NOT consumed.
- **2.2 No new network filter beyond `rbac_network`.** The five network filters after 26.3 are `echo`/`direct_response` (26.1) + `tcp_proxy`/HCM (26.2) + `rbac_network` (26.3). `sni_cluster`, the protocol proxies, and `network-filter-wasm` stay deferred (parent §2.1).
- **2.3 No per-route surface.** Network filters are chain-scoped, not route-scoped (parent §2.2 / §11.6 D6 — ZERO `*PerRoute` messages across `extensions/filters/network/rbac/`). ADR-0125 untouched. (HTTP rbac's per-route TPFC handling STAYS in the HTTP filter — §3.3 — and is unaffected by the engine extraction.)
- **2.4 No `delay_deny` support.** PARSE-REJECT (AMEND-A9; the synchronous read-filter model has no timer/`readDisable` seam). The `rbac-network-delay-deny-unsupported` arm is an envoy-go-strict departure (BEHAVIOR_CONTRACT). D-P3 reconsideration STANDS-REJECT for 26.3.
- **2.5 No CEL evaluation.** `condition`/`checked_condition`/`cel_config` are SILENT-IGNORED (F1; mirrors the phase-16 engine's existing ADR-0040 silent-ignore — NOT a new reject, NOT new evaluation). No CEL engine is added.
- **2.6 No `audit_logging_options`.** `config.rbac.v3.RBAC.audit_logging_options` is `[#not-implemented-hide:]` upstream; SILENT-IGNORED at 26.3 per the phase-16 engine's existing disposition (parent §2.1).
- **2.7 No xDS / RTDS dynamic policy.** `rbac_network` accepts only static inline policy at config-load (parent §2.1). No hot-fetch.
- **2.8 No per-policy dynamic counters for `rbac_network`.** The network RBAC proto has no `track_per_rule_stats` (F2); the per-policy machinery stays dormant for consumer #2 (retained in the engine for consumer #1). Only the four static counters land (§7).
- **2.9 No HTTP-rbac behavior change.** The HTTP rbac filter's operator-visible behavior is UNCHANGED by the extraction (R4 byte-exact via the phase-16 fixtures). The HTTP-derived `evalContext` impl + the per-route TPFC + the stat-registration + the `DecodeHeaders`/`SendLocalReply` deny path STAY in `internal/filter/http/rbac/`; only the engine internals move + the import flips to `internal/rbac/`.
- **2.10 No connection-metadata READER.** 26.3 lands the WRITE of the shadow pair; no in-repo consumer reads connection-level dynamic metadata (no access-log sink at L4 — grep-confirmed AMEND-A5/§11.1). The emission is for upstream parity + future-consumer readiness; it is observed in fixtures only indirectly (via the `shadow_*` stats) + directly by unit tests on the `Bucket`.
- **2.11 No new conformance/h2spec harness.** `rbac_network` is validated by the differential + unit + fuzz layers (parent §6.5 — no L4 conformance harness). HTTP rbac's re-verification reuses its existing phase-16 differential fixtures.

---

## 3. The shared `internal/rbac/` engine extraction (ADR-0216)

Per parent §4.4 + §10.3 + AMEND-A11. The extraction is mechanical (the engine already evaluates against an abstract `evalContext`); the 26.3-additive design points are the extracted package's exported boundary (§3.1), the input-capability profile (§3.4), and the consumer-#1/#2 split (§3.2/§3.3).

### 3.0 Split disposition — D-P1 RESOLVED (one sub-phase; net-new basis)

Parent D-P1 asked whether the ADR-0045 split-gate counts the ~790 MOVED LoC as 26.3 churn. **RESOLVED: net-new basis — 26.3 stays ONE sub-phase.** The moved code is mechanical relocation (move + re-export), re-verified byte-exact by the phase-16 HTTP-rbac differential fixtures (R4); the net-new surface (the input-capability profile ~80-150 + the `rbac_network` filter + L4 `evalContext` ~250-400 + the connection-metadata writes ~40-80 + `NoFlush` ~30 + stats wiring ~80 + fixtures/fuzzer ~150) ≈ **~630-890 net-new LoC, ~16-22 tasks** — within the ADR-0045 gate (~25 tasks / ~1500 LoC). No 26.3a/26.3b split. The §11.1 D-S2 as-built LoC measurement (~790 move / ~400 stay) confirms the parent §11.8 estimate.

### 3.1 The extracted package boundary (what MOVES; the exported API)

`internal/rbac/` is a NEW package (Go package `rbac` at `internal/rbac/`, matching the `internal/matcher`/`internal/jwt`/`internal/lua`/`internal/wasm` single-token convention). The phase-16 engine internals MOVE there and gain exported names (today they are unexported within `internal/filter/http/rbac/`). Anticipated exported surface (the 26.3 PLAN/IMPL finalizes exact names):

- **`EvalContext` interface** — the abstract matchable-context (today `evalContext`, `evaluator.go:60-121`): the 11 accessors `Header(name) (string,bool)` / `URLPath() string` / `Method() string` / `DestinationIP() net.IP` / `DestinationPort() uint32` / `RequestedServerName() string` / `DirectRemoteIP() net.IP` / `RemoteIP() net.IP` / `DownstreamPrincipal() []string` / `SourcedMetadata() any` / `FilterState() any`. UNCHANGED in shape — both consumers implement it.
- **The compiled-engine types** — `CompiledRulesEngine` (today `compiledRulesEngine`) + `CompiledMatcherEngine` (today `compiledMatcherEngine`) + `CompiledPolicy` + the `engineResult` enum (exported as `EngineResult` / `Allowed` / `Denied`).
- **The builders** — `BuildRulesEngine(r *configrbacv3.RBAC, profile Profile) (*CompiledRulesEngine, error)` (today `buildCompiledRulesEngine`, `rbac.go:373`) + `BuildMatcherEngine(m *matchv3.Matcher, profile Profile) (*CompiledMatcherEngine, error)` (today `buildCompiledMatcherEngine`, `rbac.go:445`), both gaining the `Profile` parameter (§3.4). The permission/principal builders (`buildPermissionEvaluators`/`buildOnePermission`/`buildPrincipalEvaluators`/`buildOnePrincipal`, `evaluator.go:257/275/541/565`) move as package-internal helpers.
- **The evaluators** — the `permissionEvaluator`/`principalEvaluator` interfaces + the ~23 implementations (`permAny`/`permHeader`/`permURLPath`/`permDestIP`/`permDestPort`/`permDestPortRange`/`permSNI`/`permAnd`/`permOr`/`permNot`/`permSourcedMetadata` + `prinAny`/`prinAuthenticated`/`prinDirectRemoteIP`/`prinRemoteIP`/`prinHeader`/`prinURLPath`/`prinAnd`/`prinOr`/`prinNot`/`prinSourcedMetadata`/`prinFilterState`) — package-internal.
- **The evaluation entry** — `Evaluate(engine, ctx EvalContext) (EngineResult, policyName string)` (today `evaluateEngine`/`evaluateRulesEngine`/`evaluateMatcherEngine`/`policyMatches`).
- **The matcher bridge** — `matcherCtxAdapter` (today `rbac.go:860-897`) GENERALIZED to take an `EvalContext` (it already does — it adapts `evalContext` to `matcher.MatchContext`; its `SourceIP() → DirectRemoteIP()` bridge stays). Moves with the engine.
- **The per-policy counter machinery** — `incPolicy` + the `sync.Map`-backed per-policy lazy allocation (`rbac.go:165-222`) move with the engine, gated by a caller-supplied `trackPerRuleStats bool` (consumer #1 sets it from the HTTP proto field; consumer #2 always false — F2/§7.2). The static-counter registration STAYS in each consumer (§3.2/§3.3) because the counter NAMES + the `*stats.Registry` lifetime are consumer-specific (HTTP: `http.<HCM>.rbac.<prefix>.*`; network: `<stat_prefix>.rbac.*`).
- **The leaf-level PARSE-REJECT roster** — the engine's existing per-arm rejects (`evaluator.go` 14 permission + 12 principal arms; `rbac.go` action/policy-min-items/unmarshal rejects — §11.1 D-S2 / parent §6) move with the engine and are emitted byte-stable. The `rbac:` error prefix is RETAINED (both consumers wrap with their own filter prefix).

The package imports: `config/rbac/v3` (the policy proto), `cncf/xds .../matcher/v3` (the matcher proto), `internal/matcher`, `internal/stats`, `net`, stdlib. It does NOT import `internal/filter/http` or `internal/filter/network` (one-directional; both consumers import `internal/rbac`).

### 3.2 Consumer #1 — HTTP rbac (`internal/filter/http/rbac/`; migrated + re-verified)

The HTTP filter STAYS at its path and KEEPS: `New` (the `HTTPFilterFactory`); `buildCompiledConfig` (the HTTP boot envelope, now calling `rbac.BuildRulesEngine(r, rbac.ProfileHTTP)` / `rbac.BuildMatcherEngine(m, rbac.ProfileHTTP)`); the per-route TPFC handling (`parsePerRoute`/`buildCompiledPerRoute`/`resolvePerRouteConfig`); the HTTP-request-derived `EvalContext` impl (the `*filter` receiver methods — `Header`/`URLPath`/`Method` real; the L4 accessors STAY stubbed to nil/zero, `rbac.go:965-988`, because HTTP rbac has no L4 connection accessor — UNCHANGED); the stat registration (`newFilterStatsIfAbsent`, the four `http.<HCM>.rbac.<prefix>.*` counters + the per-policy machinery driven by the HTTP proto's `track_per_rule_stats`); the `DecodeHeaders`/`SendLocalReply(403, denyBody)` enforced-deny path. The migration FLIPS the engine imports to `internal/rbac/` and passes `ProfileHTTP` (which permits ALL arms → byte-identical compile + evaluation). **R4: the phase-16 HTTP-rbac differential fixtures stay byte-exact green** (engine-correctness re-verification — AMEND-A6). NO operator-visible HTTP-rbac change.

### 3.3 Consumer #2 — `rbac_network` (`internal/filter/network/rbac/`; new)

A new filter package supplying a NON-stub L4 `EvalContext`. See §4.

### 3.4 The input-capability profile (resolving AMEND-A4 HTTP-only PARSE-REJECT)

The engine compiles ALL `Permission`/`Principal` arms today (HTTP rbac supports `header`/`url_path`). To PARSE-REJECT the HTTP-only arms for the L4 consumer (AMEND-A4) WITHOUT changing consumer #1 (R4), `BuildRulesEngine`/`BuildMatcherEngine` gain a **`Profile`** parameter — an allowed-arm capability declaration. Two profiles ship at 26.3:

- **`ProfileHTTP`** — permits ALL arms (the phase-16 superset: `header`/`url_path`/`uri_template`/all L4 arms/`and`/`or`/`not`/etc.). HTTP rbac passes this → byte-identical to today.
- **`ProfileL4`** — permits only the L4-evaluable arms (`any`/`destination_ip`/`destination_port`/`destination_port_range`/`requested_server_name` permissions; `any`/`authenticated`/`source_ip`/`direct_remote_ip`/`remote_ip` principals; + the recursive `and`/`or`/`not` combinators). PARSE-REJECTs `header`/`url_path`/`uri_template` permissions + `header`/`url_path` principals at COMPILE (the `buildOnePermission`/`buildOnePrincipal` arm switch consults the profile and returns a `rbac-network-http-only-*` error before constructing the HTTP-only evaluator). Applies to BOTH the `rules`-path and the `matcher`-path leaf matchers (AMEND-A10).

> **D-26.3-1 (profile mechanism — predicate vs enum vs builder-method):** the SPEC anchors a `Profile` value consulted in `buildOnePermission`/`buildOnePrincipal`. The PLAN/IMPL MAY model it as (a) an enum (`ProfileHTTP`/`ProfileL4`) switched at each HTTP-only arm; (b) an allowed-arm `map[reflect.Type]bool`/predicate; (c) a per-consumer set of `errArmUnsupported` injected into the builder. **Resolution at:** 26.3 PLAN. **Anticipated:** an enum (`Profile`) + a single `profile.permits(arm) bool` check at the HTTP-only arms — minimal, grep-discoverable, and keeps the engine's arm switch the single owner of arm-classification. The `uri_template`/`matcher`-extension arms ALREADY reject in the engine (`evaluator.go:355/358`); `ProfileL4` adds the `header`/`url_path` rejects.

This keeps the engine the single owner of arm classification + reject wording (R-E); the HTTP-only-reject is additive, not a network-side re-implementation. The byte-stable arm wording is finalized at 26.3 IMPL (D-P6).

---

## 4. The `rbac_network` filter (ADR-0218) + the connection-metadata sink (ADR-0217)

### 4.1 Package + parse + registration

`internal/filter/network/rbac/` (Go package `rbac`; NOTE the import collision — `internal/rbac` + this package + the two go-control-plane `rbacv3` bindings all want `rbac`/`rbacv3`; import aliasing required — §11.1 D-S2). TypeURL `type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC` — **the IMPL MUST derive/verify it via `proto.MessageName(&networkrbacv3.RBAC{})`, NOT a hand-typed string** (project memory `reference_network_filter_typeurl_extensions`; the `extensions.` segment bit 26.1 echo). The `New(tc, ctx) (network.FilterInstanceFactory, error)` factory (mirroring `echo.New`/`directresponse.New`) parses the `networkrbacv3.RBAC`, validates `stat_prefix` (PGV-required min 1 rune — §5.3), PARSE-REJECTs `delay_deny` (AMEND-A9), builds the enforced + shadow engines via `rbac.BuildRulesEngine(rules, rbac.ProfileL4)` / `rbac.BuildMatcherEngine(matcher, rbac.ProfileL4)` (the `rules`/`matcher` mutually-preferred per §5.3; shadow from `shadow_rules`/`shadow_matcher`), registers the four static counters, and returns a `FilterInstanceFactory` yielding a fresh per-connection filter. Registered as the 5th built-in via `builtins.RegisterBuiltins` (§4.6).

### 4.2 The L4 `EvalContext` + decision-in-OnData

The per-connection filter builds a NON-stub `rbac.EvalContext` from its `network.Connection` accessor (obtained via `callbacks.Connection()`):

| `rbac.EvalContext` method | Source (network.Connection) | Note |
|---|---|---|
| `DestinationIP()` | `LocalAddr()` → IP | listener-bound local IP |
| `DestinationPort()` | `LocalAddr()` → port | |
| `RequestedServerName()` | `RequestedServerName()` | SNI from tls_inspector |
| `DirectRemoteIP()` | `RemoteAddr()` → IP | peer source IP (no proxy-protocol resolution at 26.3) |
| `RemoteIP()` | `RemoteAddr()` → IP | == DirectRemoteIP at L4 (no XFF at the network layer; documented) |
| `DownstreamPrincipal()` | `DownstreamPrincipals()` | mTLS URI/DNS-SAN + CN principals |
| `Header()` / `URLPath()` / `Method()` | `""` / absent | HTTP-only; UNREACHABLE — `ProfileL4` rejected these arms at compile (§3.4), so they never evaluate |
| `SourcedMetadata()` / `FilterState()` | `nil` | MVP, same as HTTP (always-FALSE) |

**Decision timing (AMEND-A8 + the sticky-halt constraint, memory `reference_network_read_filter_onnewconnection_halts`):**

- `OnNewConnection()` → `Continue` (no-op; a `StopIteration` here sets the sticky `connHalted` flag blocking all `OnData`).
- `OnData(buf, endStream)` → make the RBAC decision (enforced engine; + shadow engine if configured):
  - **`ONE_TIME_ON_FIRST_BYTE`** (default, enum 0): decide ONCE on the first `OnData`; subsequent `OnData` (if any before handoff) are pass-through. The decision needs only connection facts (IP/port/SNI/cert), not payload bytes — the filter does NOT drain `buf`.
  - **`CONTINUOUS`** (enum 1): re-decide on every `OnData` (parity with upstream's continuous re-check, e.g. if dynamic metadata an upstream filter set changes — though no L4 filter precedes rbac_network at 26.3).
  - **Allow** → `Continue` (advance the chain to the trailing terminal — `tcp_proxy`/HCM — via the buffered-prefix `prefixConn` handover; the bytes rbac inspected-but-did-not-drain replay to the terminal). Increment `allowed`.
  - **Deny (enforced)** → `SetResponseCodeDetails("rbac_deny_close")` + `Connection().Close(NoFlush)` + `StopIteration`. Increment `denied`. (The terminal never runs.)
  - **Shadow** → evaluate the shadow engine, emit the shadow pair to dynamic metadata (§4.4) + increment `shadow_allowed`/`shadow_denied`; the shadow result NEVER affects the enforced disposition (parity with the HTTP filter's shadow walk).

### 4.3 Enforced deny — `NoFlush` close + the `rbac_deny_close` termination-detail (F3 + AMEND-A7)

Upstream enforced denial: `setConnectionTerminationDetails("rbac_deny_close")` + `connection().close(ConnectionCloseType::NoFlush)`. envoy-go:

- The `rbac_deny_close` string is written via the existing `callbacks.SetResponseCodeDetails("rbac_deny_close")` sink (`chain.go:336-340`, reached by the `interface{ SetResponseCodeDetails(string) }` type-assertion direct_response uses for `DirectResponse`). The chainRuntime owns the `rcd` field; no callbacks-interface change is required (the concrete `*callbacks` already has the method). The string has no in-repo consumer (like the shadow metadata) — observable in unit tests; future-consumer-ready.
- **`NoFlush` close (F3):** 26.1/26.2 collapsed `FlushWrite ≡ NoFlush` (`chain.go:362-368`) because read filters drained writes synchronously before close. `rbac_network` enforced-deny needs the real `NoFlush` (drop any buffered downstream writes, close immediately — upstream `NoFlush`). 26.3 lands the distinguished `NoFlush` path in `connection.Close` (a small framework touchpoint folded into ADR-0218; the `CloseType` enum already exists — `callbacks.go:37-44`). For rbac_network the deny path has no pending write to flush anyway (it writes nothing before closing), so `NoFlush` ≡ `FlushWrite` operationally here, but the SPEC pins the real distinction so the close-semantics are byte-faithful + future-proof.

> **D-26.3-2 (NoFlush close scope):** does 26.3 fully implement the `FlushWrite`/`NoFlush` distinction in `connection.Close`, or only the minimal `NoFlush`-immediate-close the rbac deny path needs? **Resolution at:** 26.3 IMPL. **Anticipated:** implement the `NoFlush` immediate-close path (drop pending write buffer if any) + keep `FlushWrite` as the drain-then-close default; the distinction is observable only when a filter buffers a write then closes `NoFlush` — rbac_network does not, so a minimal correct `NoFlush` (close without an explicit flush attempt) suffices for parity. The IMPL confirms against a unit test.

### 4.4 The connection-scoped shadow-metadata writes (ADR-0217)

When `shadow_rules`/`shadow_matcher` is configured, after evaluating the shadow engine `rbac_network` writes the shadow pair to the per-connection bucket via `callbacks.DynamicMetadata()`:

- `Set("envoy.filters.network.rbac", "shadow_engine_result", structpb.NewStringValue("allowed"|"denied"))`
- `Set("envoy.filters.network.rbac", "shadow_effective_policy_id", structpb.NewStringValue(id))` — only when the effective policy id is non-empty (parity with upstream's `setDynamicMetadata`, which writes the id only when non-empty).

The namespace + key strings are byte-faithful to upstream (`§11.4 D4`). The `Evaluate` entry returns the matched `policyName` (the effective policy id for the shadow pair). The bucket is owned by the `chainRuntime` (constructed at connection entry, reset at `OnDestroy`); `internal/dynamicmetadata/` is REUSED at connection scope with NO code change — only its package doc is generalized in place to "scope-agnostic; owner-determined lifetime — per-stream OR per-connection" (ADR-0044; the doc currently says "per-stream"). No in-repo reader (AMEND-A5/§2.10).

### 4.5 The first production mixed read→terminal chain

`[rbac_network, tcp_proxy]` (or `[rbac_network, hcm]`): on allow, `rbac_network` returns `Continue` from `OnData` without draining the buffer; the 26.2 chain runner's `TerminalReady()` fires; `HandleTerminal` hands the conn (wrapped in `prefixConn` replaying the inspected-but-undrained bytes) to the terminal's `Handle`. This is the FIRST production exercise of the 26.2 `prefixConn` handover (26.2 unit-tested it with a synthetic always-`Continue` filter; 26.3 lands the cross-side differential fixture — §8). The R-M capability becomes a R-M-LIVE differential proof.

### 4.6 Registration (the 5th built-in)

`internal/filter/network/builtins.RegisterBuiltins` (the seam ADR-0215 landed) gains a fifth `reg.Register(networkrbac.TypeURL, networkrbac.New)`. `rbac_network` is a READ filter (no terminal-build singletons) — its `New` needs no `Deps` fields beyond what the `FactoryCtx` carries (it needs the `*stats.Registry` for the counter roster, though: see D-26.3-3). The boot-wiring in `cmd/envoy-go/main.go` is unchanged structurally (the `RegisterBuiltins` call already runs there).

> **D-26.3-3 (rbac_network's stats-registry dependency):** `rbac_network` registers four counters from a `*stats.Registry`, but `network.FactoryCtx` is primitives-only (the heavy singletons are closure-captured in `RegisterBuiltins`, ADR-0215 §3). The `*stats.Registry` is in `builtins.Deps.StatsRegistry`. **Resolution at:** 26.3 PLAN. **Anticipated:** `rbac_network`'s factory is closure-captured WITH the `*stats.Registry` in `RegisterBuiltins` (like the HCM adapter captures its singletons) — `networkrbac.NewFactory(reg *stats.Registry) network.NetworkFilterFactory` — rather than adding a `StatsRegistry` field to the primitives-only `FactoryCtx` (preserving the ADR-0215 import-light invariant). The stat prefix comes from the parsed `stat_prefix` proto field (per-chain), threaded at parse time inside the factory.

---

## 5. Proto-field roster (INHERITS parent §5.3 + §5.4)

INHERITED from the parent SPEC; re-confirmed as-built at §11.1 D-S2. Summary:

### 5.1 `envoy.extensions.filters.network.rbac.v3.RBAC` (8 fields; parent §5.3)

`Rules`(1, `*config.rbac.v3.RBAC`) / `ShadowRules`(2) / `StatPrefix`(3, **PGV-required min 1 rune**) / `EnforcementType`(4, enum `ONE_TIME_ON_FIRST_BYTE=0`/`CONTINUOUS=1`) / `ShadowRulesStatPrefix`(5) / `Matcher`(6, `*xds.type.matcher.v3.Matcher`) / `ShadowMatcher`(7) / `DelayDeny`(8, `*durationpb.Duration` — **PARSE-REJECT**). `rules`/`matcher` mutually-preferred (matcher wins if both set, parity with upstream `rbac.pb.go:90-99`). **NO `track_per_rule_stats` field** (F2). Verified live this session (§11.1 D-S2).

### 5.2 Shared `envoy.config.rbac.v3` policy proto (parent §5.4)

`RBAC{action ALLOW=0/DENY=1/LOG=2, policies map, audit_logging_options [not-impl→ignore]}`; `Policy{permissions[] min1, principals[] min1, condition CEL→silent-ignore (F1), checked_condition [not-impl]}`; `Permission` 14-variant oneof; `Principal` 13-variant oneof. L4-evaluable vs HTTP-only classification per parent §5.4 + the §3.4 `ProfileL4` allow-set. The engine ALREADY compiles all arms (consumer #1); `ProfileL4` rejects the HTTP-only ones (§3.4/§6.2).

### 5.3 `echo`/`direct_response`/`tcp_proxy`/HCM rosters — UNCHANGED

Inherited from 26.1/26.2 (parent §5.1/§5.2). 26.3 adds no new fields to them.

---

## 6. PARSE-REJECT roster (INHERITS parent §6; REFINES per F1)

Per ADR-0080 byte-stable discipline. The exact wording + `TestParseRejectConstants_ByteStable` tables finalized at 26.3 IMPL (D-P6). The 26.3 reject surface:

### 6.1 Boot-reject parity arms (mirror an upstream PGV failure)

- `rbac-network-stat-prefix-required` — `stat_prefix` PGV min-1-rune (§5.3). The load-bearing 26.3 boot-reject parity fixture arm (§8.3).
- Framework-level: unknown network-filter `type_url` not in the frozen `*network.Registry` → the unified `"%s: unknown filter type_url %q"` reject (ADR-0215, byte-stable — UNCHANGED).
- The engine's existing leaf-level rejects (the ~26 arms — `evaluator.go`/`rbac.go`, §11.1 D-S2) move with the engine and emit byte-stable (the `rbac:` prefix retained; `rbac_network` wraps them with its filter+chain prefix).

### 6.2 envoy-go-strict departure arms (reject where upstream silently accepts/no-ops)

- `rbac-network-http-only-permission-<header|url-path|uri-template>` + `rbac-network-http-only-principal-<header|url-path>` — AMEND-A4; emitted by the engine when `ProfileL4` is passed and an HTTP-only arm appears (in either the `rules` or `matcher` path — §3.4). Upstream silently never-matches these at L4.
- `rbac-network-delay-deny-unsupported` — AMEND-A9 (the synchronous read-filter model has no timer seam).

### 6.3 CEL disposition — REFINED to SILENT-IGNORE (F1; D-P4 RESOLVED)

The parent §6.3 anticipated a `rbac-network-condition-cel-unsupported` arm "if the phase-16 engine does not already support CEL." **The phase-16 engine SILENT-IGNORES `condition`/`checked_condition`/`cel_config`** (`rbac.go:419-424`, ADR-0040 discipline; `compiledPolicy` has no CEL slot). **`rbac_network` MIRRORS the silent-ignore — there is NO CEL reject arm.** The extracted engine carries the structural ignore unchanged; both consumers inherit it. (This is the SPEC's one substantive refinement of the parent anticipation; recorded in BEHAVIOR_CONTRACT as the inherited silent-ignore, NOT a new departure.)

### 6.4 `audit_logging_options` — SILENT-IGNORE

`config.rbac.v3.RBAC.audit_logging_options` is `[#not-implemented-hide:]`; the engine silent-ignores it (no slot) — inherited unchanged.

---

## 7. Stat surface (INHERITS parent §7.2; F2 resolves D-P5)

### 7.1 The four static counters (132 → 136)

| Counter | Type | Name |
|---|---|---|
| `allowed` | counter | `<stat_prefix>.rbac.allowed` |
| `denied` | counter | `<stat_prefix>.rbac.denied` |
| `shadow_allowed` | counter | `<stat_prefix>.rbac.[shadow_rules_stat_prefix]shadow_allowed` |
| `shadow_denied` | counter | `<stat_prefix>.rbac.[shadow_rules_stat_prefix]shadow_denied` |

Prefix composition (parent §11.2): `stat_prefix` (PGV-required) dot-joined before the literal `rbac.` segment; `shadow_rules_stat_prefix` (if set) inserts a segment between `rbac.` and the two `shadow_*` counters ONLY (enforced counters unaffected). All four registered UNCONDITIONALLY at boot (predeclared-empty for scrape stability — mirrors the HTTP filter's `newFilterStatsIfAbsent`, §11.1 D-S2). **Project stat surface 132 → 136** at 26.3 phase-done. NO envoy-go-strict extension counter (the upstream roster is complete — parent §7.3).

### 7.2 NO per-policy dynamic counters for `rbac_network` (F2; D-P5 RESOLVED)

The HTTP rbac filter emits per-policy counters (`…rbac.<prefix>.policy.<name>.<suffix>`) gated on its `track_per_rule_stats` proto field (the engine's `incPolicy`/`sync.Map`, `rbac.go:165-222`). **The network RBAC proto has NO `track_per_rule_stats` field (F2/§5.3)** — so `rbac_network` does NOT emit per-policy counters; it passes `trackPerRuleStats=false` to the engine. The per-policy machinery stays in the engine (DORMANT for consumer #2; LIVE for consumer #1). Per-policy counters are config-dependent/lazily-created and not counted in the static 132→136 surface either way.

### 7.3 R7 — stat baseline re-confirm

The 132 master-tip baseline is re-confirmed live at 26.3 IMPL Task 1 (parallel to the fuzzer/fixture/ADR baselines) before asserting the 132 → 136 delta.

---

## 8. Differential fixture taxonomy (+2; INHERITS parent §8.3)

Full cross-side byte-exact vs reference Envoy v1.37.2. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject are SEPARATE dirs (one fixture dir = one runner branch). Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `StatsAsserter` (NOT `SubjectAsserter`) and MUST be proven live. Per `reference_network_filter_typeurl_extensions`: the differential bootstraps need a cluster (zero-cluster boot is rejected). Numbering continues from `0042` (the 26.2 tail); exact numbers pinned at IMPL (next free `0043`/`0044`).

### 8.1 `rbac_network` cross-side (`00XX-network-rbac`)

A listener with `[rbac_network, tcp_proxy]` (the first production mixed read→terminal chain — §4.5). Scenarios (may share one dir with multiple bootstrap arms, or split — 26.3 IMPL pins):

- **allow** — a principal/permission the connection satisfies (e.g. `direct_remote_ip` matching the test client + `destination_port` matching the listener) → `Continue` → `tcp_proxy` passthrough → byte-exact echo/round-robin response. `allowed` increments (StatsAsserter).
- **deny (enforced)** — a policy the connection does NOT satisfy (default-deny or an explicit DENY action) → connection close, no upstream bytes. `denied` increments (StatsAsserter). Byte-exact (both upstream + subject close).
- **shadow** — `shadow_rules` configured (deny) + `rules` allow → passthrough (enforced allow) + `shadow_denied` increments (StatsAsserter); the shadow metadata is emitted but unread (asserted indirectly via the stat + directly by unit test).

L4 inputs exercised at minimum: `direct_remote_ip` principal + `destination_port` permission. An SNI (`requested_server_name`) + mTLS-`authenticated` scenario is added IF the differential harness supports client certs (D-26.3-4).

### 8.2 `rbac_network` boot-reject (`00XX-network-rbac-boot-reject`)

A `stat_prefix`-missing config (PGV-mirror parity — both upstream + envoy-go reject at boot; boot-stderr substring parity). The envoy-go-strict-only arms (HTTP-only matcher, `delay_deny`) are subject-side-only boot-rejects (upstream ACCEPTS them) → covered by `manager.go`/filter build-path UNIT tests, NOT a cross-side fixture (the dispatch-constraint memory: a subject-side-only reject is not cross-side parity).

### 8.3 Total fixture-dir count

44 → **46** at 26.3 phase-done (+2: the cross-side `rbac` dir + the boot-reject dir). The HTTP-rbac re-verification (R4) reuses the EXISTING phase-16 fixtures (no new dir). No new conformance harness (§2.11).

---

## 9. Behavior-contract delta (the 26.3 bundle; parent §9)

ONE atomic bundle at the 26.3 IMPL final task (ADR-0052). Anticipated edits:

- NEW `### envoy.filters.network.rbac` subsection under `## Network filters`: full enforced + shadow parity; the L4 principal/permission input surface (the `ProfileL4` allow-set); decision-in-`OnData` (OnNewConnection `Continue` no-op per the sticky-halt constraint); enforced-deny = `NoFlush` close + `rbac_deny_close` termination-detail; shadow = dynamic-metadata shadow-pair + `shadow_*` stats; the `rules`/`matcher` dual-path; CEL/audit silent-ignore (inherited from the engine).
- UPDATE the stat table 132 → 136 (the four `<stat_prefix>.rbac.*` counters).
- envoy-go-strict departure records: HTTP-only-matcher PARSE-REJECT (AMEND-A4); `delay_deny` PARSE-REJECT (AMEND-A9); xDS dynamic-policy PARSE-REJECT.
- A note that connection-level dynamic metadata is emitted (namespace `envoy.filters.network.rbac`, keys `shadow_engine_result`/`shadow_effective_policy_id`) but has no in-repo downstream consumer yet (AMEND-A5/A6).
- A note on the `internal/rbac/` engine extraction (HTTP rbac = consumer #1, re-verified byte-exact; the engine is now shared) — structural, no HTTP-rbac behavior change.
- A note that `NoFlush` close semantics are now distinguished (F3).
- ROLLUP: parent row 26 `in-progress → done` ATOMICALLY with sub-row 26.3 at phase-done (18/19/22/24/25 precedent).

---

## 10. Per-task structure (~16-22 tasks; parent §11.8 + §15)

The 26.3 PLAN authors the exact bite-sized TDD tasks; the SPEC-anticipated spine (the PLAN may merge/split):

| # | Task | Lands |
|---|---|---|
| 1 | First-task baselines: re-confirm fuzzers **35** + fixtures **44** (tail 0042) + stat surface **132** + DECISIONS.md tail **ADR-0218** (this SPEC drafts 0216/0217/0218) via grep; re-confirm the network-rbac TypeURL via `proto.MessageName` + the `internal/filter/http/rbac/` engine line-anchors (`evalContext`@60-121, L4 stubs@965-988, `incPolicy`@210-222, matcher@445/824) against the IMPL-session tip | §11 / §3 gates |
| 2 | NEW `internal/rbac/` package skeleton + MOVE the `EvalContext` interface + the `permissionEvaluator`/`principalEvaluator` interfaces + the ~23 evaluators (re-exported) + the leaf-level PARSE-REJECT arms; unit tests move with them | §3.1 |
| 3 | MOVE the rules-path + matcher-path compilers (`BuildRulesEngine`/`BuildMatcherEngine`) + the matcher bridge + the `Evaluate` entry + the per-policy `incPolicy` machinery; unit tests move | §3.1 |
| 4 | ADD the input-capability `Profile` (`ProfileHTTP`/`ProfileL4`) + the HTTP-only-arm reject in `buildOnePermission`/`buildOnePrincipal` (D-26.3-1) + tests (ProfileHTTP permits all; ProfileL4 rejects header/url_path/uri_template in rules + matcher paths) | §3.4 / §6.2 |
| 5 | Consumer #1 migration: flip `internal/filter/http/rbac/` to import `internal/rbac/`, pass `ProfileHTTP`; the HTTP-derived `EvalContext` impl + L4 stubs + stat registration + per-route + deny path STAY; build + the existing phase-16 unit tests green | §3.2 |
| 6 | R4 re-verification gate: the phase-16 HTTP-rbac differential fixtures byte-exact green LIVE after the engine move (consumer-#1 correctness) | §3.2 / R4 |
| 7 | `NoFlush` close semantics in `connection.Close` (F3; D-26.3-2) + tests (NoFlush immediate-close vs FlushWrite drain-then-close) | §4.3 / F3 |
| 8 | `internal/filter/network/rbac/` package: parse (`networkrbacv3.RBAC`; TypeURL via `proto.MessageName`; `stat_prefix` required; `delay_deny` reject), build enforced+shadow engines via `ProfileL4`, the static 4-counter registration (D-26.3-3 closure-captured `*stats.Registry`) | §4.1 / §7 |
| 9 | The L4 `EvalContext` impl (the §4.2 accessor mapping from `network.Connection`) + tests | §4.2 |
| 10 | The OnData decision (ONE_TIME_ON_FIRST_BYTE + CONTINUOUS; allow→Continue, deny→rcd+NoFlush-close+StopIteration, shadow→metadata+stats; OnNewConnection Continue no-op) + tests (incl. the sticky-halt regression: OnNewConnection must NOT StopIteration) | §4.2 |
| 11 | The shadow-metadata writes (`Set("envoy.filters.network.rbac", "shadow_engine_result"/"shadow_effective_policy_id", …)`) + the `internal/dynamicmetadata/` doc generalization to scope-agnostic (ADR-0044) + Bucket round-trip unit tests (R5) | §4.4 / ADR-0217 |
| 12 | Register the 5th built-in in `builtins.RegisterBuiltins` + boot smoke (rbac_network in a `[rbac_network, tcp_proxy]` chain through the unified dispatch) | §4.6 |
| 13 | The 36th fuzzer `FuzzNetworkRBACConfigParse` (config-parse) | §15 |
| 14 | Differential fixtures: `00XX-network-rbac` cross-side (allow/deny/shadow; StatsAsserter for the counters; the first live mixed read→terminal chain — R-M-LIVE) + `00XX-network-rbac-boot-reject` (stat_prefix missing, PGV-mirror) | §8 |
| 15 | BEHAVIOR_CONTRACT 26.3 bundle (§9) + ADR-0216/0217/0218 §Decision/§Consequences body landing (DECISIONS.md tail STAYS ADR-0218; no new number consumed at IMPL) + STATE.md re-advance + ROADMAP sub-row 26.3 `in-progress → done` + parent row 26 `in-progress → done` (ROLLUP) + six-gate verification | §9 / §15 |

---

## 11. SPEC-time empirical-pin block (INHERITS parent §11; adds D-S2 as-built code-surface)

The 26.3 SPEC does NOT re-execute the parent §11 D1–D8 pins (resolved once at the parent SPEC; inherited). The 26.3-additive pins:

### 11.1 D-S2 — master-tip baselines + as-built code surface VERIFIED at this SPEC session

Verified live this session (the source of the §10 Task-1 first-action gate) against master tip (the substantive 26.2 IMPL squash `24f9b13`; the tip trails it by next-prompt repoint commits — no Go code changed since, so no code-surface drift):

- **Fuzzer count = 35** (`git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l` = 35). 26.3 adds the 36th (`FuzzNetworkRBACConfigParse`) → **36** at phase-done.
- **Differential fixture-dir count = 44**; numbering tail = **0042** (`test/fixtures/0040-network-echo`/`0041-network-direct-response`/`0042-network-direct-response-boot-reject`). 26.3 adds +2 → **46**.
- **Stat surface = 132**. 26.3 adds +4 → **136**.
- **DECISIONS.md tail = ADR-0215** at master tip; **next-free = ADR-0216**. THIS 26.3 SPEC commit DRAFTS the **ADR-0216 + ADR-0217 + ADR-0218 §Context** → DECISIONS.md tail advances to **ADR-0218**; next-free becomes **ADR-0219**. The §Decision/§Consequences bodies land at 26.3 IMPL (Task 15) per ADR-0044 (no new number consumed at IMPL).
- **As-built network framework surface** (verified — the rbac_network consumes it without revision; R2):
  - `network.Connection` exposes `Write`/`Close(CloseType)`/`LocalAddr`/`RemoteAddr`/`RequestedServerName`/`DownstreamPrincipals` (`callbacks.go:47-67`) — the exact L4 inputs §4.2 maps.
  - `ReadFilterCallbacks` exposes `Connection()`/`ContinueReading()`/`DynamicMetadata() *dynamicmetadata.Bucket` (`callbacks.go:11-30`). The `*callbacks` concrete also has `SetResponseCodeDetails(string)` (`chain.go:336-340`, type-assertion-reachable) — the `rbac_deny_close` sink.
  - `CloseType`: `FlushWrite=0`/`NoFlush=1` (`callbacks.go:37-44`); `connection.Close` ignores the type at 26.1/26.2 (`chain.go:362-368`) with an explicit "distinguished when 26.3 lands" note (F3).
  - The `chainRuntime` owns the per-connection `*dynamicmetadata.Bucket` (`chain.go:159` `NewBucket()`; `:294-302` `Reset()` at onDestroy; `:334` exposed via `DynamicMetadata()`).
  - The mixed read→terminal handover: `TerminalReady()`/`HandleTerminal()` + `prefixConn` (`chain.go:189-203` + `prefixconn.go`) — §4.5's first production consumer.
  - `builtins.RegisterBuiltins(reg, deps Deps)` (`builtins/builtins.go:24-52`) — the 5th-built-in seam; `Deps.StatsRegistry` carries the `*stats.Registry` (D-26.3-3).
- **As-built phase-16 engine surface** (the extraction target; AMEND-A11 RATIFIED):
  - `evalContext` interface @`evaluator.go:60-121` (11 accessors; the 6 L4-ish ones present); L4 stubs @`rbac.go:965-988` (return nil/zero).
  - `permissionEvaluator`/`principalEvaluator` @`evaluator.go:24-38`; builders @`evaluator.go:257/275/541/565`; rules-path @`rbac.go:373`; matcher-path @`rbac.go:445/824`; matcher bridge `matcherCtxAdapter` @`rbac.go:860-897` (`SourceIP()→DirectRemoteIP()`).
  - **F1 CEL silent-ignore** @`rbac.go:419-424` + `doc.go:33-36` (NOT reject).
  - **F2 no `track_per_rule_stats`** in the network RBAC proto (8 fields; go-control-plane v1.32.4 verified).
  - Per-policy `incPolicy`/`sync.Map` @`rbac.go:165-222` (shape `<base>.policy.<name>.<suffix>`); static 4-counter registration @`rbac.go:644-657`.
  - HTTP rbac emits NO dynamic metadata (grep `dynamicmetadata`/`shadow_engine_result`/`shadow_effective_policy_id` in `internal/filter/http/rbac/` = 0 — AMEND-A6).
  - Extraction LoC: ~790 move / ~400 stay (AMEND-A11 confirmed).
- **Proto + import:** network RBAC FQN `envoy.extensions.filters.network.rbac.v3.RBAC` (go-control-plane v1.32.4; TypeURL via `proto.MessageName`); the `rbacv3` Go-package-name collision (network binding + config binding both `rbacv3`) requires import aliasing (parent §11.1; the new `internal/rbac` package + the network filter package add more `rbac`/`rbacv3` aliasing pressure — D-26.3-5).
- **No in-repo connection-metadata reader** (grep `DynamicMetadata`/`dynamicmetadata` in `internal/`/`cmd/` minus tests: only HTTP-chain + network-chain producers; no L4 reader — AMEND-A5/§2.10).

The 26.3 IMPL Task-1 RE-RUNS these greps + line-anchor checks as a hard first-action gate (the master tip may advance between this SPEC commit and the IMPL session; the gate catches drift before the deltas are asserted).

---

## 12. SPEC-time D-questions for PLAN / IMPL resolution

Inherits the parent §12 D-questions; the 26.3-territory ones are resolved here (D-P1 §3.0; D-P4 §6.3/F1; D-P5 §7.2/F2) or carried (D-P3 `delay_deny` STANDS-REJECT; D-P6/D-P7 IMPL wording). The 26.3-additive D-questions (each cross-referenced inline above):

- **D-26.3-1 (input-capability profile mechanism).** §3.4. **Resolution at:** 26.3 PLAN Task 4. **Anticipated:** an enum `Profile` (`ProfileHTTP`/`ProfileL4`) + a `profile.permits(arm)` check at the HTTP-only arms in `buildOnePermission`/`buildOnePrincipal`.
- **D-26.3-2 (NoFlush close scope).** §4.3. **Resolution at:** 26.3 IMPL Task 7. **Anticipated:** implement `NoFlush` immediate-close (drop pending writes) + keep `FlushWrite` drain-then-close; a minimal correct `NoFlush` suffices for the rbac deny path (no pending write).
- **D-26.3-3 (rbac_network's `*stats.Registry` wiring).** §4.6. **Resolution at:** 26.3 PLAN Task 8. **Anticipated:** closure-capture the `*stats.Registry` in `RegisterBuiltins` (`networkrbac.NewFactory(reg)`), NOT a `FactoryCtx` field — preserving the ADR-0215 import-light `FactoryCtx` invariant.
- **D-26.3-4 (mTLS/SNI differential fixture coverage).** §8.1. **Resolution at:** 26.3 IMPL Task 14. **Anticipated:** the cross-side fixture exercises `direct_remote_ip` + `destination_port` at minimum; add an SNI/`authenticated` scenario IF the differential harness already supports client certs (it does for the TLS-TCP/mTLS fixtures — confirm at IMPL); else unit-test the SNI/cert accessor mapping and note the differential gap.
- **D-26.3-5 (import-aliasing for the rbac/rbacv3 collision).** §11.1. **Resolution at:** 26.3 IMPL. **Anticipated:** alias the go-control-plane bindings (`networkrbacv3` for `extensions/filters/network/rbac/v3`, `configrbacv3` for `config/rbac/v3`) + keep `internal/rbac` as package `rbac`; the network filter package is `rbac` at `internal/filter/network/rbac/` (path-disambiguated, imported as `networkrbac` by `builtins`). The `go build ./...` import-cycle audit (D-26.3-6) confirms clean.
- **D-26.3-6 (engine import-cycle audit).** §3.1. **Resolution at:** 26.3 IMPL Task 5. **Anticipated:** `internal/rbac` imports `internal/matcher`/`internal/stats`/proto/stdlib only (NOT `internal/filter/http` or `internal/filter/network`); both consumers import `internal/rbac` (one-directional). Verified by `go build ./...`.
- **D-26.3-7 (per-policy machinery placement — engine vs HTTP-consumer).** §3.1/§7.2. **Resolution at:** 26.3 PLAN Task 3. **Anticipated:** the `incPolicy`/`sync.Map` evaluation-side machinery moves to the engine (it is evaluation logic the engine drives via the matched-policy-name return); the static-counter REGISTRATION + the `trackPerRuleStats` decision stay per-consumer (counter names + `*stats.Registry` lifetime are consumer-specific). Network passes `trackPerRuleStats=false` (F2).

---

## 13. RATIFIED-PENDING items (parent §13 + sub-phase-specific)

- **R4 (consumer-#1 re-verification — the extraction proof; parent §13 R4).** The phase-16 HTTP-rbac differential fixtures stay byte-exact green LIVE after the engine moves to `internal/rbac/` (engine-correctness, NOT metadata — AMEND-A6). The LOAD-BEARING 26.3 extraction gate (the deliberate-break discipline applied to the engine move). Run LIVE at the six-gate (the HTTP-rbac dispatch genuinely changed — engine import flip — so NOT asserted-unaffected).
- **R5 (connection-metadata sink reuse — parent §13 R5).** `internal/dynamicmetadata/.Bucket` reused at connection scope with NO code change; the doc generalized to scope-agnostic in place (ADR-0044). 26.3 IMPL verifies the `Set("envoy.filters.network.rbac", "shadow_engine_result", …)` round-trips on the per-connection bucket + the namespace/key strings match §11.4 byte-for-byte.
- **R2 (26.1 callbacks API completeness — parent §13 R2; CONFIRMED as-built §11.1).** `ReadFilterCallbacks` + `Connection` + `SetResponseCodeDetails` supply every L4 input + the deny-close sink `rbac_network` needs WITHOUT a callbacks revision. The ONLY framework touchpoint is the `NoFlush` close distinction (F3) — a `connection.Close` impl change, not a callbacks-interface change.
- **R-M-LIVE (the first production mixed read→terminal chain).** The 26.2 `prefixConn` handover (R-M, unit-tested with a synthetic always-`Continue` filter) gets its FIRST production consumer + its first cross-side differential proof: `[rbac_network, tcp_proxy]` allow → `Continue` → terminal (§4.5 / §8.1).
- **R-E (engine single-ownership of arm classification + reject wording).** The HTTP-only PARSE-REJECT is additive via the `ProfileL4` capability check INSIDE the engine's arm switch (NOT a network-side re-implementation) — the engine stays the single owner of arm classification + byte-stable reject wording.
- **R-S (baselines + wording stability).** 26.3 IMPL Task-1 re-confirms fuzzers 35→36, fixtures 44→46, stat surface 132→136, DECISIONS.md tail ADR-0218; the engine's moved leaf-reject wording is preserved byte-for-byte (the `rbac:` prefix); the unified unknown-type wording is unchanged.
- **R-N (NoFlush byte-faithfulness).** The enforced-deny `NoFlush` close (F3) is byte-faithful to upstream `ConnectionCloseType::NoFlush`; 26.3 IMPL verifies via a unit test + the deny-scenario differential fixture (the subject closes byte-exactly with upstream).

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (parent §9 + §9 above)

ONE atomic bundle at 26.3 IMPL final task (§10 Task 15), per ADR-0052. The edits enumerated at §9: the NEW `### envoy.filters.network.rbac` subsection; the stat-table 132 → 136 extension; the envoy-go-strict departure records (HTTP-only-matcher / `delay_deny` / xDS PARSE-REJECT); the shadow-metadata-emitted-but-unread note; the engine-extraction structural note; the `NoFlush`-distinguished note; the parent-row-26 ROLLUP to `done`.

---

## 15. Test surface + 26.3 IMPL acceptance checklist

### 15.1 Test surface (parent §14)

- **Layer A — unit tests** at `internal/rbac/` (the MOVED engine: principal/permission evaluation against an abstract `EvalContext`; the `ProfileHTTP`/`ProfileL4` arm gating; the rules + matcher paths; the per-policy machinery; the phase-16 evaluator tests move with the engine) + `internal/filter/http/rbac/` (the migrated consumer #1: the existing tests green against the imported engine) + `internal/filter/network/rbac/` (parse + `stat_prefix`/`delay_deny` reject; the L4 `EvalContext` mapping; the OnData decision incl. the sticky-halt regression; enforced-deny rcd+NoFlush-close; shadow metadata + stats) + `internal/filter/network/` (the `NoFlush` close distinction — F3).
- **Layer B — `manager.go`/build-path** unit tests: the `[rbac_network, tcp_proxy]` chain builds + classifies (read prefix + terminal); the subject-side-only boot-rejects (HTTP-only matcher, `delay_deny`).
- **Layer C — fuzz**: the 36th fuzzer `FuzzNetworkRBACConfigParse`.
- **Layer D — differential**: +2 (§8 — the `rbac` cross-side allow/deny/shadow with `StatsAsserter`; the boot-reject) + the EXISTING phase-16 HTTP-rbac fixtures as the R4 consumer-#1 re-verification gate (run LIVE). The cross-side `rbac` dir is the first LIVE mixed read→terminal differential (R-M-LIVE).
- **Layer E — race**: `go test -race -short ./internal/rbac/... ./internal/filter/network/... ./internal/filter/http/rbac/...` (the engine is evaluated single-goroutine-per-connection/stream; the per-policy `sync.Map` + the registry are the only shared state — `-race` proves no data race).

### 15.2 Six-gate checklist (phase-22/24/25/26.1/26.2 precedent)

`go build ./...` + `go vet ./...` + `golangci-lint run` + `go test -race -short ./...` + the FULL differential suite (46 fixtures — incl. the R4 phase-16 HTTP-rbac fixtures + the +2 new `rbac` dirs, run LIVE) + conformance 10/10 + h2spec 53/53 (asserted-unaffected — 26.3 touches no HTTP/h2/proxy-wasm path internals; the HTTP-rbac engine move is differential-gated by R4, not conformance). Counts at phase-done: fuzzers 36; fixtures 46; stat surface 136; DECISIONS.md tail ADR-0218.

### 15.3 26.3 IMPL acceptance checklist (parent §15 + sub-phase-specific)

1. The shared `internal/rbac/` engine extracted (the `EvalContext`/evaluators/builders/matcher-bridge/per-policy machinery MOVED + re-exported; the `Profile` capability added); import-cycle-clean (D-26.3-6).
2. Consumer #1 (HTTP rbac) migrated onto `internal/rbac/` with `ProfileHTTP`; R4 byte-exact (the phase-16 differential fixtures green LIVE); NO operator-visible HTTP-rbac change.
3. `rbac_network` lands: parse (`stat_prefix` required; `delay_deny` reject; `ProfileL4` HTTP-only-arm reject; CEL/audit silent-ignore inherited); the L4 `EvalContext` mapping; OnData decision (ONE_TIME + CONTINUOUS; OnNewConnection `Continue` no-op — sticky-halt-safe); allow→`Continue`→terminal; enforced-deny→`rbac_deny_close`+`NoFlush`-close+`StopIteration`; shadow→metadata+stats.
4. The connection-scoped shadow-metadata writes land (namespace/keys byte-faithful — R5); `internal/dynamicmetadata/` doc generalized to scope-agnostic (ADR-0044).
5. The `NoFlush` close distinction lands (F3); the 5th built-in registered (`builtins.RegisterBuiltins`); boot smoke green.
6. The four static counters (`<stat_prefix>.rbac.*`) land (132 → 136); NO per-policy counters for network (F2); the per-policy machinery dormant for consumer #2.
7. +2 differential fixtures (the first LIVE mixed read→terminal `rbac` cross-side with StatsAsserter; the boot-reject); the 36th fuzzer; R-M-LIVE proven.
8. ADR-0216/0217/0218 §Decision/§Consequences bodies land (DECISIONS.md tail STAYS ADR-0218 — drafted at THIS SPEC; no new number consumed at IMPL); BEHAVIOR_CONTRACT.md 26.3 bundle lands (§14).
9. Six gates green (§15.2); STATE.md advanced; ROADMAP sub-row 26.3 `in-progress → done` + parent row 26 `in-progress → done` (ROLLUP); the §9 Network-filters family stays OPEN (6 candidates remain).

---

## 16. Stage-close handoff

Per ADR-0004/0005 (autonomous adaptation): this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, STATE.md advances to lifecycle-state 2 with `next-skill = superpowers:writing-plans` scoped to the **26.3 PLAN** (per the per-sub-phase precedent — the next session authors the 26.3 PLAN, the FINAL sub-phase's bite-sized TDD task spine from this SPEC's §10). ROADMAP sub-row 26.3 flips `planned → in-progress` at THIS SPEC commit per ADR-0106; parent row 26 STAYS `in-progress` (flips at 26.3 phase-done per the ROLLUP precedent). The SPEC is squash-merged to master + pushed (controller pushes at stage-close per `feedback_subagents_no_push`); next-prompt.txt is rewritten for the 26.3-PLAN cold-start.

---

## Appendix A — Cross-references to parent SPEC

| 26.3 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (26.3 scope detail) | refines |
| §1.1 AMENDs + F1-F3 | parent §1.1 (A4-A11) + §11.3-11.5 | inherits the 26.3-relevant amendments; adds F1-F3 as-built findings |
| §2 Non-purposes | parent §2 + §3.2 | refines (26.3-scoped) |
| §3 Engine extraction | parent §4.4 + §10.3 (ADR-0216) + AMEND-A11 | refines + resolves D-P1 |
| §4 rbac_network + sink | parent §3.2 + §4.3 + §4.4 + §10.3 (ADR-0217/0218) | refines |
| §5 Proto roster | parent §5.3 + §5.4 | INHERITS (re-confirmed as-built) |
| §6 PARSE-REJECT | parent §6.3 | refines (F1 CEL silent-ignore; ProfileL4 HTTP-only rejects) |
| §7 Stat surface | parent §7.2 | INHERITS + resolves D-P5 (F2; no per-policy for network) |
| §8 Fixtures | parent §8.3 | refines (+2; first LIVE mixed read→terminal) |
| §9 Behavior contract | parent §9 (26.3 bundle) | refines |
| §10 Tasks | parent §11.8 + §15 (26.3 row) | NEW (task spine) |
| §11 Empirical pins | parent §11 (D-S2 sub-pin) | inherits; adds D-S2 baseline + as-built code-surface |
| §12 D-questions | parent §12 (D-P1/P4/P5 resolved here) | adds D-26.3-1..7 |
| §13 RATIFIED-PENDING | parent §13 (R4/R5/R2) | refines (R4/R5/R2/R-M-LIVE/R-E/R-S/R-N scoped to 26.3) |

## Appendix B — Phase 26.3 ADR landing summary

- **ADR-0216** (`internal/rbac/` shared engine extraction) — §Context DRAFTED at THIS 26.3 SPEC commit (DECISIONS.md tail ADR-0215 → ADR-0216). Covers: the MOVE of the phase-16 engine to `internal/rbac/` (the abstract `EvalContext` + evaluators + rules/matcher builders + matcher bridge + per-policy machinery); the input-capability `Profile` (`ProfileHTTP`/`ProfileL4`) resolving the AMEND-A4 HTTP-only PARSE-REJECT without changing consumer #1; consumer #1 (HTTP rbac) migration + byte-exact re-verification (R4, engine-correctness — AMEND-A6); consumer #2 (`rbac_network`); the API-revision-allowance clause (phase-22.1 ADR-0188 / phase-25.1 ADR-0202 pattern, with the added LIVE-first-consumer re-verification discipline); the engine single-ownership of arm classification + reject wording (R-E). §Decision/§Consequences at 26.3 IMPL (Task 15).
- **ADR-0217** (connection-scoped dynamic-metadata writes) — §Context DRAFTED (tail → ADR-0217). Covers: the FIRST production WRITE through the 26.1-shaped `ReadFilterCallbacks.DynamicMetadata()` (the rbac shadow pair, namespace `envoy.filters.network.rbac`, keys `shadow_engine_result`/`shadow_effective_policy_id`); the connection-scoped REUSE of `internal/dynamicmetadata/` (NO new package — AMEND-A5; the `chainRuntime` already owns the per-connection bucket); the in-place doc generalization to scope-agnostic (ADR-0044); the no-in-repo-reader note (AMEND-A5/A6/§2.10). §Decision/§Consequences at 26.3 IMPL.
- **ADR-0218** (`rbac_network` filter) — §Context DRAFTED (tail → ADR-0218; next-free → ADR-0219). Covers: the L4 principal/permission input surface (the `ProfileL4` allow-set, the `network.Connection`→`EvalContext` mapping); decision-in-`OnData` (AMEND-A8; OnNewConnection `Continue` no-op per the sticky-halt constraint, memory `reference_network_read_filter_onnewconnection_halts`); enforced-deny = `NoFlush` close + the `rbac_deny_close` response-code-details sink (AMEND-A7 + F3 — the NoFlush framework touchpoint); shadow = metadata + `shadow_*` stats; the `<stat_prefix>.rbac.{allowed,denied,shadow_allowed,shadow_denied}` roster (132 → 136); NO per-policy counters (F2/no `track_per_rule_stats`); `delay_deny` PARSE-REJECT (AMEND-A9); CEL silent-ignore (F1, inherited); the `rules`/`matcher` dual-path (AMEND-A10); the FIRST production mixed read→terminal chain (R-M-LIVE). §Decision/§Consequences at 26.3 IMPL.
- DECISIONS.md tail = **ADR-0218** at 26.3 SPEC commit (this SPEC drafts the three §Context blocks); STAYS ADR-0218 at 26.3 phase-done (IMPL fills the §Decision/§Consequences bodies in place — no new number consumed). Next-free **ADR-0219** (anticipated next-free after phase-26 phase-done, per the parent §10.4). The ADR-0209 escape-valve reserve STANDS-UNCONSUMED.
