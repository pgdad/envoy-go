# Phase 26.3 Implementation Plan — `rbac_network` at full upstream parity + shared `internal/rbac/` engine extraction + connection-scoped dynamic-metadata writes

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended, per the `feedback_execution_style` project memory) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task is TDD-first (write-the-failing-test → run-it-fails → minimal-impl → run-it-passes → commit) per superpowers:test-driven-development. Subagents commit LOCAL-ONLY (they do NOT push — `feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close.

**Goal:** Land `rbac_network` (`envoy.extensions.filters.network.rbac.v3.RBAC`) at FULL upstream parity (enforced + shadow rules + connection-level dynamic-metadata shadow-pair + the four `<stat_prefix>.rbac.*` counters) by EXTRACTING the phase-16 RBAC principal/permission evaluation engine from `internal/filter/http/rbac/` into a NEW shared `internal/rbac/` primitive (HTTP rbac migrated as consumer #1, re-verified byte-exact by its phase-16 differential fixtures — R4; `rbac_network` as consumer #2), wiring the connection-scoped `*dynamicmetadata.Bucket` writes the 26.1 framework already exposes, and landing the real `NoFlush` close distinction (F3). Phase 26.3 phase-done flips parent row 26 `in-progress → done` ATOMICALLY (the 18/19/22/24/25 ROLLUP).

**Architecture:** The phase-16 engine ALREADY evaluates against an abstract `evalContext` interface (`internal/filter/http/rbac/evaluator.go:60-121`) whose L4 accessors are present but STUBBED to nil/zero in the HTTP filter (`rbac.go:961-984`). The extraction is therefore mechanical (AMEND-A11): the engine internals (the `evalContext`/`permissionEvaluator`/`principalEvaluator` interfaces + the ~23 evaluators + the `matchString`/`matchHeader`/`matchPath`/`matchCidr` adapters + the rules-path + matcher-path compilers + the `matcherCtxAdapter` bridge + the per-policy lazy-allocation machinery) MOVE to `internal/rbac/` (package `rbac`) and gain exported names; the HTTP-consumer surface (`New`/`buildCompiledConfig`/`compiledConfig`/`factoryState`/`filter`/per-route TPFC/the HTTP-derived `EvalContext` impl/the four named base counters + their registration/`DecodeHeaders`+`SendLocalReply` deny path) STAYS at `internal/filter/http/rbac/` and imports the shared engine (aliased — package-name collision). The engine's builders gain an input-capability `Profile` (`ProfileHTTP`/`ProfileL4`) so the L4 consumer PARSE-REJECTs HTTP-only matcher arms (`header`/`url_path`/`uri_template`) at COMPILE without changing consumer #1's behavior (R4). `rbac_network` (`internal/filter/network/rbac/`) supplies a NON-stub L4 `EvalContext` built from the `network.Connection` accessors (`LocalAddr`/`RemoteAddr`/`RequestedServerName`/`DownstreamPrincipals`), decides allow/deny in `OnData` (AMEND-A8; `OnNewConnection` is a `Continue` no-op per the sticky-halt constraint), writes the shadow pair to the per-connection `*dynamicmetadata.Bucket` via `ReadFilterCallbacks.DynamicMetadata()`, and on enforced deny sets `rbac_deny_close` via the existing `SetResponseCodeDetails` sink + `Close(NoFlush)`. Allow → `Continue` → terminal (`tcp_proxy`/HCM) via the 26.2 `prefixConn` handover — the FIRST production mixed read→terminal chain (R-M-LIVE).

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); golangci-lint 1.64.8 (ADR-0009). `internal/matcher/` (phase-16 matcher path); `internal/dynamicmetadata/` (phase-22.2, REUSED at connection scope, NO code change); `internal/stats/` (the counter roster). ZERO new third-party `go.mod` dependencies (the engine + filter + sink are entirely in-house). REUSES the 26.1/26.2 `internal/filter/network/` framework + the phase-16 RBAC engine (extracted, not rewritten).

**Module path:** `github.com/esalaine/envoy-go`.

**Source of truth:** the phase-26.3 SPEC (`docs/envoy-go/phases/26.3-network-filter-rbac/SPEC.md`), especially §3 (engine extraction + the `Profile` capability + the consumer-#1/#2 split), §4 (`rbac_network` + the connection-metadata sink + the L4 `EvalContext` + decision-in-OnData + enforced-deny `NoFlush`+rcd + shadow writes + the 5th-built-in registration), §5 (proto roster — 8 fields, NO `track_per_rule_stats`), §6 (PARSE-REJECT — F1 CEL silent-ignore, `ProfileL4` HTTP-only rejects, `delay_deny` reject), §7 (stat surface 132→136; F2 no per-policy for network), §8 (the +2 differential fixtures), §10 (the ~15-task spine this plan decomposes), §11.1 D-S2 (as-built baselines + line anchors = the Task-1 gate), §12 (D-26.3-1..7), §13 (R4/R5/R2/R-M-LIVE/R-E/R-S/R-N), §15 (acceptance).

---

## ADR-0045 split-gate check (PERFORMED at PLAN time)

Per SKILL_ROUTING state-2 GATE: split if PLAN > ~25 tasks OR > ~1500 estimated **net-new** LoC. This plan is **15 tasks**. SPEC §3.0 (D-P1 RESOLVED) measures the basis as **net-new** (the ~790 moved LoC is mechanical relocation re-verified byte-exact by the phase-16 fixtures — R4 — NOT counted as churn): the input-capability `Profile` (~80-150) + the `rbac_network` filter + L4 `EvalContext` (~250-400) + the connection-metadata writes (~40-80) + `NoFlush` (~30) + stats wiring (~80) + the fuzzer + 2 fixtures (~150) ≈ **~630-890 net-new LoC**. Both axes (15 tasks / ~630-890 net-new LoC) sit comfortably within the gate (~25 tasks / ~1500 LoC). **NO split.** Proceed as a single 26.3 IMPL. (Re-confirms the parent §11.8 estimate + the §11.1 D-S2 ~790-move/~400-stay measurement.)

## PLAN-time D-question resolutions (SPEC §12)

The SPEC marks **D-26.3-1 / D-26.3-3 / D-26.3-7** for PLAN-time resolution; resolved here so IMPL has no open design choices. D-26.3-2 (NoFlush scope), D-26.3-4 (mTLS/SNI fixture coverage), D-26.3-5 (import aliasing), D-26.3-6 (import-cycle audit) remain IMPL-time empirical pins (scoped into Tasks 7 / 14 / 5+8 / 5 below).

- **D-26.3-1 (input-capability profile mechanism) → an enum `Profile` (`ProfileHTTP`/`ProfileL4`) + a single `profile.permits(arm armKind) bool` check at the HTTP-only arms in `buildOnePermission`/`buildOnePrincipal`.** This keeps the engine's arm `switch` the single owner of arm-classification + reject wording (R-E). The builders + their recursive helpers (`BuildRulesEngine`/`BuildMatcherEngine`/`buildPermissionEvaluators`/`buildPrincipalEvaluators`/`buildOnePermission`/`buildOnePrincipal`) gain a `profile Profile` parameter threaded through the recursion. `ProfileHTTP.permits(*) == true` for every arm → byte-identical compile + evaluation for consumer #1 (R4). `ProfileL4.permits(armHeader|armURLPath) == false` → those arms reject at compile, in BOTH the `rules` and `matcher` leaf-matcher paths (AMEND-A10). The `uri_template`/`matcher`-extension arms ALREADY reject unconditionally in the engine (`evaluator.go:355/358`) — `ProfileL4` adds only the `header`/`url_path` rejects (perm + prin). Rejected alternatives: a `map[reflect.Type]bool` predicate (heavier, reflection at compile-time) and a per-consumer injected `errArmUnsupported` set (scatters arm-classification across consumers, violates R-E).

- **D-26.3-3 (rbac_network's `*stats.Registry` wiring) → closure-capture the `*stats.Registry` in `RegisterBuiltins`, NOT a `FactoryCtx` field.** `network.FactoryCtx` is primitives-only by ADR-0215 (the heavy boot singletons are closure-captured in `internal/filter/network/builtins`; `Deps.StatsRegistry` already carries the `*stats.Registry` — verified `builtins.go:29`). `rbac_network` exposes `networkrbac.NewFactory(reg *stats.Registry) network.NetworkFilterFactory`; `RegisterBuiltins` calls `reg.Register(networkrbac.TypeURL, networkrbac.NewFactory(deps.StatsRegistry))` — exactly the closure-capture shape `tcpproxy.NewNetworkFactory`/`hcm.NewNetworkFactory` already use (`builtins.go:43-51`). The per-chain `stat_prefix` comes from the parsed proto field, threaded at parse time INSIDE the factory closure (each chain's `New(tc, ctx)` reads `cfg.GetStatPrefix()` and registers `<stat_prefix>.rbac.*` against the captured `reg`). The ADR-0215 import-light `FactoryCtx` invariant is preserved (no `*stats.Registry` field added).

- **D-26.3-7 (per-policy machinery placement — engine vs HTTP-consumer) → the `incPolicy` `sync.Map`-backed lazy-allocation moves to the engine as a thin reusable helper `rbac.PerPolicyCounters` (an exported type wrapping a `sync.Map` with `Inc(reg *stats.Registry, base, policyName, suffix string)`); the four NAMED base counters + their registration + the `trackPerRuleStats` decision STAY per-consumer.** This honors SPEC §3.1 ("the per-policy machinery moves with the engine") + R-E (the lazy-allocation logic, including the load-bearing `.policy.` segment, has a single home) while keeping consumer-specific concerns (counter NAMES, `*stats.Registry` lifetime, the `track_per_rule_stats` gate) in each consumer. The HTTP `filterStats` replaces its inline `perPolicy *sync.Map` field + `incPolicy` method with a held `*rbac.PerPolicyCounters`; `emitPrimaryCounters`/`emitShadowCounters` (HTTP consumer) call `cc.stats.perPolicy.Inc(cc.stats.reg, base, policyName, suffix)`. Network passes `trackPerRuleStats=false` (F2 — the network proto has NO `track_per_rule_stats`), so the network consumer NEVER constructs or calls `PerPolicyCounters` — the machinery is dormant for consumer #2.

## Engine-extraction design pin (Tasks 2–4 — what MOVES, what STAYS, the exported boundary)

The extraction splits the two phase-16 files (`evaluator.go` ~854 LoC, `rbac.go` ~1191 LoC) along the engine/consumer seam. The package-name collision (`internal/rbac` is package `rbac`; the HTTP consumer at `internal/filter/http/rbac/` is ALSO package `rbac`) is resolved by the consumer aliasing the engine import (anticipated `rbacengine "github.com/esalaine/envoy-go/internal/rbac"` — D-26.3-5, finalized at IMPL).

### MOVES to `internal/rbac/` (gains exported names) — Task 2 (evaluator.go body) + Task 3 (rbac.go engine body)

| Today (unexported, `internal/filter/http/rbac/`) | Exported in `internal/rbac/` | Anchor |
|---|---|---|
| `evalContext` interface (11 accessors) | `EvalContext` | evaluator.go:60-121 |
| `permissionEvaluator` / `principalEvaluator` ifaces | unexported (package-internal) | evaluator.go:24-38 |
| the ~23 `perm*`/`prin*` evaluator types | unexported (package-internal) | evaluator.go:143-531 |
| `buildPermissionEvaluators` / `buildOnePermission` | unexported (gain `profile` param — Task 4) | evaluator.go:257/275 |
| `buildPrincipalEvaluators` / `buildOnePrincipal` | unexported (gain `profile` param — Task 4) | evaluator.go:541/565 |
| `matchString` / `matchHeader` / `matchPath` / `matchCidr` | unexported (package-internal) | evaluator.go:680-854 |
| `compiledRulesEngine` / `compiledMatcherEngine` / `compiledPolicy` | `CompiledRulesEngine` / `CompiledMatcherEngine` / `compiledPolicy` (the policy stays internal) | rbac.go:76/86/95 |
| `engineResult` + `engineResultAllowed`/`engineResultDenied` | `EngineResult` + `Allowed`/`Denied` | rbac.go:702-713 |
| `buildCompiledRulesEngine` / `buildCompiledMatcherEngine` | `BuildRulesEngine(r, profile)` / `BuildMatcherEngine(m, profile)` | rbac.go:373/445 |
| `evaluateEngine`/`evaluateRulesEngine`/`evaluateMatcherEngine`/`policyMatches` | `(*CompiledRulesEngine).Evaluate(ctx) (EngineResult,string)` + `(*CompiledMatcherEngine).Evaluate(ctx) (EngineResult,string)` (the per-engine entries; `policyMatches` stays internal) | rbac.go:726-845 |
| `matcherCtxAdapter` (EvalContext→matcher.MatchContext bridge) | unexported (package-internal) | rbac.go:866-897 |
| `incPolicy` + the `perPolicy *sync.Map` lazy-allocation | `PerPolicyCounters` + `(*PerPolicyCounters).Inc(reg, base, policyName, suffix)` (D-26.3-7) | rbac.go:210-222 |
| `actionTypeURL` const (matcher terminal allow-list) | `actionTypeURL` (package-internal) | rbac.go:36 |

The leaf-level PARSE-REJECT arms (the `errors.New("rbac: permission.X is nil")` etc. — evaluator.go's 14 permission + 12 principal arms + rbac.go's action/min-items/unmarshal rejects) MOVE with the engine and emit byte-stable (the `rbac:` prefix RETAINED — R-S; both consumers wrap with their own prefix).

### STAYS at `internal/filter/http/rbac/` (imports the engine, aliased) — Task 5

`TypeURL` / `filterName` / `denyBody` consts; `compiledConfig` (now holding `*rbacengine.CompiledRulesEngine`/`*rbacengine.CompiledMatcherEngine`); `factoryState`; `filter` (+ its HTTP-derived `EvalContext` impl: `Header`/`URLPath`/`Method` real, the six L4 accessors STUBBED nil/zero — UNCHANGED, since HTTP rbac has no L4 connection accessor); `New`/`buildCompiledConfig` (now calling `rbacengine.BuildRulesEngine(r, rbacengine.ProfileHTTP)` / `rbacengine.BuildMatcherEngine(m, rbacengine.ProfileHTTP)`); `parsePerRoute`/`resolvePerRouteConfig`/`buildCompiledPerRoute`; `filterStats` (the four NAMED base counters + `newFilterStats`/`newFilterStatsIfAbsent`/`namespacePrefix`/`baseStatPrefix` + a held `*rbacengine.PerPolicyCounters`); `evaluateEngine(cc, ctx, shadow)` (the consumer-side rules-vs-matcher + primary-vs-shadow dispatcher, now calling the engine's per-engine `Evaluate`); `emitPrimaryCounters`/`emitShadowCounters`; `DecodeHeaders` + the `SendLocalReply(403, denyBody)` deny path. NO operator-visible HTTP-rbac change (R4).

### The engine import graph (D-26.3-6, verified at Task 5 by `go build ./...`)

`internal/rbac` imports: `config/rbac/v3` (`configrbacv3`), `config/core/v3`, `config/route/v3`, `type/matcher/v3` (`matcherv3`), `cncf/xds .../matcher/v3` (`matchv3`), `internal/matcher`, `internal/stats`, `net`, stdlib. It does NOT import `internal/filter/http` or `internal/filter/network` (one-directional). Both consumers import `internal/rbac`.

---

## File Structure

### Created — the shared engine `internal/rbac/`

| File | Responsibility |
|---|---|
| `internal/rbac/evaluator.go` | the `EvalContext` interface + the `permissionEvaluator`/`principalEvaluator` ifaces + the ~23 evaluators + `matchString`/`matchHeader`/`matchPath`/`matchCidr` + `buildPermissionEvaluators`/`buildPrincipalEvaluators`/`buildOnePermission`/`buildOnePrincipal` (gaining the `profile` param) |
| `internal/rbac/rbac.go` | `CompiledRulesEngine`/`CompiledMatcherEngine`/`compiledPolicy` + `EngineResult` + `BuildRulesEngine`/`BuildMatcherEngine` + the `Evaluate` entries + `matcherCtxAdapter` + `actionTypeURL` |
| `internal/rbac/profile.go` | the `Profile` enum (`ProfileHTTP`/`ProfileL4`) + `armKind` + `(Profile).permits(armKind) bool` (Task 4) |
| `internal/rbac/perpolicy.go` | `PerPolicyCounters` + `Inc` (the moved per-policy lazy-allocation; D-26.3-7) |
| `internal/rbac/doc.go` | package doc (scope-agnostic shared RBAC engine; the two consumers; the `Profile` capability) |

Tests: `internal/rbac/evaluator_test.go`, `rbac_test.go`, `profile_test.go`, `perpolicy_test.go` — the phase-16 evaluator/engine tests MOVE here (renamed to the exported surface); the `Profile`/`PerPolicyCounters` tests are net-new.

### Created — `rbac_network` consumer #2 `internal/filter/network/rbac/`

| File | Responsibility |
|---|---|
| `internal/filter/network/rbac/rbac.go` | the `New`/`NewFactory(reg)` factory; parse (`stat_prefix` required, `delay_deny` reject, `ProfileL4` engines); the per-connection `filter` (OnNewConnection no-op, OnData decision, enforced-deny rcd+NoFlush-close, shadow writes); the four static counters |
| `internal/filter/network/rbac/evalctx.go` | the L4 `EvalContext` impl (the §4.2 accessor mapping from `network.Connection`) |
| `internal/filter/network/rbac/fuzz_test.go` | the 36th fuzzer `FuzzNetworkRBACConfigParse` (Task 13) |

Tests: `internal/filter/network/rbac/rbac_test.go`, `evalctx_test.go`.

### Modified — framework + HTTP consumer + registration

| File | Change |
|---|---|
| `internal/filter/http/rbac/{evaluator,rbac}.go` | DELETE the moved engine body; import `rbacengine`; `compiledConfig` holds engine types; `*filter` implements `rbacengine.EvalContext`; builders pass `rbacengine.ProfileHTTP`; `filterStats` holds `*rbacengine.PerPolicyCounters`; `evaluateEngine` calls the engine's `Evaluate` (Task 5) |
| `internal/filter/http/rbac/{evaluator,rbac}_test.go` | engine-internal tests MOVE to `internal/rbac/`; the consumer-level tests (DecodeHeaders/per-route/stat-registration/deny path) STAY + green against the imported engine |
| `internal/filter/network/callbacks.go` | doc-comment on `Close`/`CloseType` updated (NoFlush now distinguished — F3) |
| `internal/filter/network/chain.go` | `connection.Close(ct)` records the `CloseType`; `chainRuntime.closeType` field + accessor (Task 7) |
| `internal/listener/manager.go` | `serveNetworkChain` honors the recorded `CloseType` on the pure-read close (Task 7) |
| `internal/filter/network/builtins/builtins.go` | register the 5th built-in: `reg.Register(networkrbac.TypeURL, networkrbac.NewFactory(deps.StatsRegistry))` (Task 12) |
| `internal/dynamicmetadata/doc.go` | generalize the package doc to scope-agnostic (per-stream OR per-connection; owner-determined lifetime) — ADR-0044 (Task 11) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | the 26.3 bundle (Task 15) |
| `docs/envoy-go/DECISIONS.md` | ADR-0216/0217/0218 §Decision/§Consequences bodies (tail STAYS ADR-0218; Task 15) |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | phase-done advance + parent-row-26 ROLLUP (Task 15) |

---

## Task 1: First-action baselines + proto/anchor re-confirm (HARD GATE)

The master tip may have advanced since the SPEC commit (`ef22eb7`; tip at PLAN authoring `dbc460b` — next-prompt repoints only, no Go-code drift expected). Re-pin every baseline + every engine/framework line anchor BEFORE asserting any delta or editing (SPEC §11.1 D-S2; R-S). No production code in this task.

**Files:** none (verification only). Record results in `docs/envoy-go/phases/26.3-network-filter-rbac/PROGRESS.md` (create it).

- [ ] **Step 1: Re-grep the baselines — git-tracked enumeration (deterministic).**

Use `git ls-files`, NOT `find .`: the repo root has dozens of nested worktrees under `.worktrees/` + `.claude/worktrees/` whose `fuzz_test.go` files inflate a naive `find`/`grep -r` count (the 26.1/26.2 PLAN reviews flagged exactly this artifact).

```bash
cd "$(git rev-parse --show-toplevel)"
echo "fuzzers:";      git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # expect 35
echo "fixture dirs:"; ls test/fixtures/ | grep -E '^[0-9]' | wc -l                          # expect 44
echo "fixture tail:"; ls test/fixtures/ | grep -E '^[0-9]' | sort | tail -1                 # expect 0042-...
echo "ADR tail:";     grep -nE '^#+ +ADR-0[0-9]{3}' docs/envoy-go/DECISIONS.md | tail -1   # expect ADR-0218 (this SPEC drafted 0216/0217/0218 §Context). Grep HEADINGS, not prose — a naive 'grep -oE ADR-0[0-9]{3}' over-reports forward-refs (0219) in provisional-span text.
```

Expected: `35`, `44`, `0042-…`, `ADR-0218`. **If any differ**, STOP and reconcile (the SPEC's deltas assume these) — note the drift in PROGRESS.md before proceeding.

- [ ] **Step 2: Re-confirm the stat surface = 132.** The whole-project surface is pinned in `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the stat table — `grep -n "stat surface" docs/envoy-go/BEHAVIOR_CONTRACT.md`); the per-package count-tests (e.g. `TestProjectStatCount_Wasm25_3`, `internal/filter/http/wasm/wasm_test.go:466`) are the per-filter pins. Expected: BEHAVIOR_CONTRACT says **132**. 26.3 adds +4 → 136 (landed at Task 12 via the new `rbac_network` per-package count-test + at Task 15 via the BEHAVIOR_CONTRACT table edit).

- [ ] **Step 3: Re-confirm the network-rbac TypeURL via `proto.MessageName` (R-S; memory `reference_network_filter_typeurl_extensions` — the `extensions.` segment bit 26.1 echo; do NOT hand-type it).**

```bash
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3.RBAC | head -5
```

Confirm the network RBAC proto has the 8 fields (`Rules`/`ShadowRules`/`StatPrefix`/`EnforcementType`/`ShadowRulesStatPrefix`/`Matcher`/`ShadowMatcher`/`DelayDeny`) and **NO `track_per_rule_stats`** (F2). The IMPL derives the TypeURL via `proto.MessageName(&networkrbacv3.RBAC{})` at Task 8 (NOT a string literal); the empirical full string is `type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC`.

- [ ] **Step 4: Re-pin the engine + framework line anchors against the IMPL-session tip** (the SPEC §11.1 anchors were against `24f9b13`; re-pin and record in PROGRESS.md — subsequent tasks cite ranges, drift moves them):
  - `internal/filter/http/rbac/evaluator.go`: `evalContext` interface (SPEC says @60-121); `permissionEvaluator`/`principalEvaluator` (@24-38); `buildOnePermission` (@275); `buildOnePrincipal` (@565); the `matchString`/`matchHeader`/`matchPath`/`matchCidr` adapters (@680-854).
  - `internal/filter/http/rbac/rbac.go`: `compiledRulesEngine`/`compiledMatcherEngine` (@76/86); `incPolicy` (SPEC says @210-222); `buildCompiledRulesEngine` (@373); `buildCompiledMatcherEngine` (@445); CEL silent-ignore (@420-424); the matcher bridge `matcherCtxAdapter` (@866-897); the L4 stubs (`DestinationIP`@961.. through `FilterState`@1001); `evaluateEngine`/`evaluateRulesEngine`/`evaluateMatcherEngine`/`policyMatches` (@726-845).
  - `internal/filter/network/`: `Connection` accessors (`callbacks.go:50-67`); `ReadFilterCallbacks` (`callbacks.go:16-30`); `CloseType` (`callbacks.go:37-45`); `connection.Close` ignoring the type (`chain.go:369-371`); the `SetResponseCodeDetails` sink (`chain.go:336-340`); the bucket `NewBucket`/`Reset` (`chain.go:159`/`:301`); `TerminalReady`/`HandleTerminal`/`prefixConn` (`chain.go:179-203`); `builtins.RegisterBuiltins` + `Deps.StatsRegistry` (`builtins/builtins.go:24-52`).
  - `internal/listener/manager.go`: `serveNetworkChain` (@1025); the pure-read close `dispatchConn.Close()` (@1080).

- [ ] **Step 5: Re-confirm AMEND-A6 — HTTP rbac emits NO dynamic metadata** (the connection-metadata sink is net-new for `rbac_network` only):

```bash
git grep -nE 'dynamicmetadata|shadow_engine_result|shadow_effective_policy_id' internal/filter/http/rbac/   # expect 0 matches
```

- [ ] **Step 6: Confirm six gates green at the tip** (baseline must be clean before new code):

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
```

Expected: all pass.

- [ ] **Step 7: Commit** (PROGRESS.md only):

```bash
git add docs/envoy-go/phases/26.3-network-filter-rbac/PROGRESS.md
git commit -m "phase 26.3 Task 1: re-pin D-S2 baselines (fuzzers 35, fixtures 44/tail 0042, stats 132, ADR-0218) + engine/framework anchors"
```

---

## Task 2: NEW `internal/rbac/` package — MOVE the evaluator surface (`EvalContext` + evaluators + adapters + builders)

MOVE the `evaluator.go` body (the `evalContext` interface, the `permissionEvaluator`/`principalEvaluator` interfaces, the ~23 evaluators, the `matchString`/`matchHeader`/`matchPath`/`matchCidr` adapters, `buildPermissionEvaluators`/`buildOnePermission`/`buildPrincipalEvaluators`/`buildOnePrincipal`) to `internal/rbac/evaluator.go`, package `rbac`, exporting only `EvalContext` (the evaluators + adapters + builders stay package-internal — both consumers reach them only through the compilers/`Evaluate`, landing at Task 3). Move the evaluator unit tests with them. The `profile` parameter is NOT added yet (Task 4) — this task is a pure mechanical move + the `evalContext`→`EvalContext` rename. The HTTP package is left in a TEMPORARILY-broken intermediate state (it still references the now-moved symbols) — that is expected; the consumer flip lands at Task 5. To keep the tree building between tasks, this task ALSO leaves thin local copies? **NO** — instead Tasks 2+3 move the whole engine and Task 5 flips the consumer; the intervening build-break is contained to `internal/filter/http/rbac/` and is repaired at Task 5. (Verification at Steps 4/5 is scoped to `./internal/rbac/...`, not `./...`.)

> **Decomposition note (granularity):** Tasks 2+3 are MOVE tasks — large but mechanical (relocate + export-rename). Each is ONE logical TDD unit: the moved unit tests are the failing→passing test (they fail to compile in the old location's absence and pass in the new package). Do NOT hand-retype the ~850 LoC; use `git mv`-style relocation + a mechanical rename pass, then verify by the moved tests.

**Files:**
- Create: `internal/rbac/evaluator.go` (moved body, package `rbac`), `internal/rbac/evaluator_test.go` (moved tests), `internal/rbac/doc.go`
- Modify (later, Task 5): `internal/filter/http/rbac/evaluator.go` (emptied / repointed)

- [ ] **Step 1: Move the evaluator body + tests; rename `evalContext` → `EvalContext`.**
  - Create `internal/rbac/evaluator.go`: copy `internal/filter/http/rbac/evaluator.go` verbatim, change `package rbac` (same token, new path), rename the interface `evalContext` → `EvalContext` (and every reference within the moved file). The evaluators, the two evaluator interfaces, `matchString`/`matchHeader`/`matchPath`/`matchCidr`, and the four `build*` funcs stay UNEXPORTED. Keep the `rbac:` error prefix on every leaf reject byte-for-byte (R-S).
  - Identify the evaluator-only tests in `internal/filter/http/rbac/evaluator_test.go` (or `rbac_test.go`) that exercise ONLY the moved surface (the per-arm evaluator tests, the `matchString`/`matchHeader`/`matchPath`/`matchCidr` tests, the `buildOnePermission`/`buildOnePrincipal` reject-wording tests). Move them to `internal/rbac/evaluator_test.go`, package `rbac`, updating `evalContext` → `EvalContext` in test fakes. Find them:
    ```bash
    git grep -nE 'func Test(Match|Build(One)?(Permission|Principal)|Perm|Prin|Eval)' internal/filter/http/rbac/*_test.go
    ```
  - Create `internal/rbac/doc.go` (package doc: the shared RBAC principal/permission evaluation engine extracted from phase-16 at its second consumer; consumer #1 = HTTP rbac, consumer #2 = `rbac_network`; evaluates against the abstract `EvalContext`; the `Profile` input-capability arrives at Task 4; per ADR-0216).

- [ ] **Step 2: Run the moved tests → fail to compile only if a symbol was missed.**
Run: `go test ./internal/rbac/ -run 'Match|Build|Perm|Prin' -v 2>&1 | head -40`
Expected at first: compile errors naming any still-`evalContext` reference or any unmoved helper. Fix until it compiles.

- [ ] **Step 3: (No separate impl step — the move IS the impl.)** Ensure the moved `internal/rbac/evaluator.go` compiles standalone (it imports only `config/{core,rbac,route}/v3`, `type/matcher/v3`, `errors`/`fmt`/`net`/`regexp`/`strconv`/`strings` — NO internal/filter imports).

- [ ] **Step 4: Run the moved tests → pass.**
Run: `go test ./internal/rbac/... -v`
Expected: PASS (every moved evaluator/adapter/build test green against the new package).

- [ ] **Step 5: Vet the new package** (NOT `go build ./...` — the HTTP consumer is intentionally broken until Task 5):
Run: `go vet ./internal/rbac/...`
Expected: clean.

- [ ] **Step 6: Commit.**

```bash
git add internal/rbac/evaluator.go internal/rbac/evaluator_test.go internal/rbac/doc.go
git commit -m "phase 26.3 Task 2: MOVE the RBAC evaluator surface to internal/rbac/ (EvalContext + evaluators + matchers + builders) [SPEC 3.1; ADR-0216]"
```

---

## Task 3: MOVE the compilers + `Evaluate` entries + matcher bridge + per-policy machinery to `internal/rbac/`

MOVE the engine body of `rbac.go` (the compiled-engine types, the builders, the dual `Evaluate` entries, the `matcherCtxAdapter` bridge, the `engineResult` enum, the `actionTypeURL` const, and the per-policy lazy-allocation) to `internal/rbac/rbac.go` + `internal/rbac/perpolicy.go`, exporting the cross-boundary surface. The per-policy `incPolicy`/`sync.Map` becomes `PerPolicyCounters` (D-26.3-7).

**Files:**
- Create: `internal/rbac/rbac.go`, `internal/rbac/perpolicy.go`, and move the engine/per-policy tests into `internal/rbac/rbac_test.go` + `internal/rbac/perpolicy_test.go`

- [ ] **Step 1: Write/relocate the engine tests** (the `BuildRulesEngine`/`BuildMatcherEngine`/`Evaluate`/policy-walk tests + the per-policy `Inc` test). Move the engine-level tests from `internal/filter/http/rbac/rbac_test.go` that exercise the compiled engine + evaluation walk (rename to the exported names). Add a NEW `perpolicy_test.go`:

```go
package rbac

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestPerPolicyCounters_IncLazyAllocatesPolicySegment(t *testing.T) {
	reg := stats.NewRegistry()
	pc := &PerPolicyCounters{}
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed")
	pc.Inc(reg, "http.hcm.rbac.p", "policy_a", "allowed") // idempotent registration, 2 increments
	// the byte-exact name shape carries the inserted ".policy." segment (rbac.go:214).
	got := findCounter(reg, "http.hcm.rbac.p.policy.policy_a.allowed")
	if got == nil || got.Value() != 2 {
		t.Fatalf("per-policy counter name/value wrong: %v", got)
	}
}

func TestPerPolicyCounters_NilRegOrEmptyPolicyIsNoOp(t *testing.T) {
	pc := &PerPolicyCounters{}
	pc.Inc(nil, "b", "p", "allowed")   // nil reg → no-op (no panic)
	pc.Inc(stats.NewRegistry(), "b", "", "allowed") // empty policy name → no-op
}
```

(Use the project's existing stat-counter lookup helper — `git grep -n 'func findStatCounter\|findCounter\|NewCounterIfAbsent' internal/stats/ internal/filter/http/rbac/` — reuse it; do NOT invent one.)

- [ ] **Step 2: Run → fail** (`undefined: PerPolicyCounters` / `BuildRulesEngine` / etc.).
Run: `go test ./internal/rbac/ -run 'PerPolicy|Build|Evaluate' -v`

- [ ] **Step 3: Move + export the engine body.**
  - `internal/rbac/rbac.go` (package `rbac`): move `compiledRulesEngine`→`CompiledRulesEngine`, `compiledMatcherEngine`→`CompiledMatcherEngine` (keep `compiledPolicy` internal); `engineResult`→`EngineResult` with `engineResultAllowed`→`Allowed`, `engineResultDenied`→`Denied`; `buildCompiledRulesEngine`→`BuildRulesEngine` + `buildCompiledMatcherEngine`→`BuildMatcherEngine` (the `profile` param arrives at Task 4 — for now keep the single-arg signature so the move is pure); convert the free `evaluateRulesEngine`/`evaluateMatcherEngine` into methods `(*CompiledRulesEngine).Evaluate(ctx EvalContext) (EngineResult, string)` + `(*CompiledMatcherEngine).Evaluate(ctx EvalContext) (EngineResult, string)` (keep `policyMatches` internal; the consumer-side `evaluateEngine` rules-vs-matcher dispatch does NOT move — it is `compiledConfig`-shaped, stays at Task 5). Move `matcherCtxAdapter` (internal) + `actionTypeURL` (internal). The CEL silent-ignore (the structural no-slot on `compiledPolicy`) moves intact (F1).
  - `internal/rbac/perpolicy.go`: 
    ```go
    package rbac

    import (
        "sync"

        "github.com/esalaine/envoy-go/internal/stats"
    )

    // PerPolicyCounters is the engine-side per-policy lazy-allocation cache
    // (moved from the phase-16 HTTP filterStats.incPolicy per D-26.3-7). The
    // sync.Map LoadOrStore + NewCounterIfAbsent first-emission path is race-safe.
    // The four NAMED base counters + the trackPerRuleStats gate stay per-consumer;
    // consumer #2 (rbac_network) never constructs this (F2 — no track_per_rule_stats).
    type PerPolicyCounters struct {
        m sync.Map // map[string]*stats.Counter keyed by the full counter name
    }

    // Inc lazy-allocates + increments <base>.policy.<policyName>.<suffix>. The
    // inserted ".policy." segment is the empirically-RATIFIED Envoy v1.37.2 shape
    // (phase-16 ADR-0145). No-op on nil reg or empty policyName.
    func (s *PerPolicyCounters) Inc(reg *stats.Registry, base, policyName, suffix string) {
        if s == nil || reg == nil || policyName == "" {
            return
        }
        key := base + ".policy." + policyName + "." + suffix
        if cached, ok := s.m.Load(key); ok {
            cached.(*stats.Counter).Inc()
            return
        }
        c := reg.NewCounterIfAbsent(key)
        actual, _ := s.m.LoadOrStore(key, c)
        actual.(*stats.Counter).Inc()
    }
    ```

- [ ] **Step 4: Run → pass; the engine package builds + tests green.**
Run: `go test ./internal/rbac/... && go vet ./internal/rbac/...`
Expected: PASS. (`go build ./...` still fails — HTTP consumer repaired at Task 5.)

- [ ] **Step 5: Commit.**

```bash
git add internal/rbac/rbac.go internal/rbac/perpolicy.go internal/rbac/rbac_test.go internal/rbac/perpolicy_test.go
git commit -m "phase 26.3 Task 3: MOVE compilers + Evaluate entries + matcher bridge + PerPolicyCounters to internal/rbac/ [SPEC 3.1; D-26.3-7; ADR-0216]"
```

---

## Task 4: ADD the input-capability `Profile` (`ProfileHTTP`/`ProfileL4`) + the HTTP-only-arm reject (D-26.3-1; SPEC §3.4 / §6.2)

The engine's builders gain a `profile Profile` parameter threaded through the recursion; `ProfileL4` rejects the `header`/`url_path` permission + principal arms at COMPILE (in both `rules` and `matcher` leaf paths); `ProfileHTTP` permits all (byte-identical to today — R4). The `uri_template`/`matcher`-extension arms already reject unconditionally (unchanged).

**Files:**
- Create: `internal/rbac/profile.go`, `internal/rbac/profile_test.go`
- Modify: `internal/rbac/evaluator.go` (`buildPermissionEvaluators`/`buildOnePermission`/`buildPrincipalEvaluators`/`buildOnePrincipal` gain `profile`), `internal/rbac/rbac.go` (`BuildRulesEngine`/`BuildMatcherEngine` gain `profile`; the matcher leaf path threads it)

- [ ] **Step 1: Write the failing test** (`profile_test.go`):

```go
package rbac

import (
	"testing"

	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// minimal ALLOW engine with one policy carrying a single permission/principal.
func ruleWith(perm *configrbacv3.Permission, prin *configrbacv3.Principal) *configrbacv3.RBAC {
	return &configrbacv3.RBAC{
		Action: configrbacv3.RBAC_ALLOW,
		Policies: map[string]*configrbacv3.Policy{
			"p": {Permissions: []*configrbacv3.Permission{perm}, Principals: []*configrbacv3.Principal{prin}},
		},
	}
}

func permHeaderRule() *configrbacv3.Permission {
	return &configrbacv3.Permission{Rule: &configrbacv3.Permission_Header{Header: &routev3.HeaderMatcher{Name: "x"}}}
}
func prinAnyId() *configrbacv3.Principal {
	return &configrbacv3.Principal{Identifier: &configrbacv3.Principal_Any{Any: true}}
}
func permAnyRule() *configrbacv3.Permission {
	return &configrbacv3.Permission{Rule: &configrbacv3.Permission_Any{Any: true}}
}
func prinHeaderId() *configrbacv3.Principal {
	return &configrbacv3.Principal{Identifier: &configrbacv3.Principal_Header{Header: &routev3.HeaderMatcher{Name: "x"}}}
}

func TestProfileHTTP_PermitsHTTPOnlyArms(t *testing.T) {
	if _, err := BuildRulesEngine(ruleWith(permHeaderRule(), prinAnyId()), ProfileHTTP); err != nil {
		t.Fatalf("ProfileHTTP must permit permission.header: %v", err)
	}
	if _, err := BuildRulesEngine(ruleWith(permAnyRule(), prinHeaderId()), ProfileHTTP); err != nil {
		t.Fatalf("ProfileHTTP must permit principal.header: %v", err)
	}
}

func TestProfileL4_RejectsHTTPOnlyPermissionHeader(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permHeaderRule(), prinAnyId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject permission.header at compile")
	}
}

func TestProfileL4_RejectsHTTPOnlyPrincipalHeader(t *testing.T) {
	_, err := BuildRulesEngine(ruleWith(permAnyRule(), prinHeaderId()), ProfileL4)
	if err == nil {
		t.Fatal("ProfileL4 must reject principal.header at compile")
	}
}

func TestProfileL4_PermitsL4Arms(t *testing.T) {
	// destination_port permission + any principal — both L4-evaluable.
	perm := &configrbacv3.Permission{Rule: &configrbacv3.Permission_DestinationPort{DestinationPort: 8080}}
	if _, err := BuildRulesEngine(ruleWith(perm, prinAnyId()), ProfileL4); err != nil {
		t.Fatalf("ProfileL4 must permit destination_port + any: %v", err)
	}
}

func TestProfileL4_RejectsHTTPOnlyArmInMatcherLeaf(t *testing.T) {
	// A matcher tree whose leaf predicate is a permission.url_path must reject
	// under ProfileL4 too (AMEND-A10 — both rules + matcher paths). Build a
	// minimal matchv3.Matcher with an rbac-permission leaf; assert BuildMatcherEngine
	// rejects under ProfileL4 and accepts under ProfileHTTP.
	m := matcherWithPermissionLeaf(t, &configrbacv3.Permission{Rule: &configrbacv3.Permission_UrlPath{UrlPath: &matcherv3.PathMatcher{}}})
	if _, err := BuildMatcherEngine(m, ProfileHTTP); err != nil {
		t.Fatalf("ProfileHTTP must accept url_path leaf in matcher path: %v", err)
	}
	if _, err := BuildMatcherEngine(m, ProfileL4); err == nil {
		t.Fatal("ProfileL4 must reject url_path leaf in matcher path")
	}
}
```

> NOTE: `matcherWithPermissionLeaf` builds a minimal `matchv3.Matcher` whose terminal/predicate references an rbac permission via the matcher framework's leaf shape — model it on the existing phase-16 matcher-path test fixtures (`git grep -n 'matchv3.Matcher\|buildCompiledMatcherEngine' internal/filter/http/rbac/*_test.go` for the construction helper to copy). If the matcher framework does not surface permission/principal leaves through a path the `profile` can reach in v1.32.4, DROP this single test and instead assert the rules-path rejects only, recording the matcher-leaf-reachability finding in PROGRESS.md (the SPEC §3.4 claim "applies to both paths" is then verified by code-reading the leaf-build call site rather than a live test). Decide at IMPL.

- [ ] **Step 2: Run → fail** (`undefined: Profile`/`ProfileHTTP`/`ProfileL4`; `BuildRulesEngine` arity).
Run: `go test ./internal/rbac/ -run 'Profile' -v`

- [ ] **Step 3: Minimal implementation.**
  - `internal/rbac/profile.go`:
    ```go
    package rbac

    // Profile is an input-capability declaration: which Permission/Principal arms
    // a consumer's input surface can evaluate. The engine compiles ALL arms by
    // default (the phase-16 HTTP superset); a Profile lets the L4 consumer
    // PARSE-REJECT the HTTP-only arms at compile WITHOUT changing consumer #1
    // (R4). It keeps the engine the single owner of arm-classification (R-E).
    type Profile int

    const (
        // ProfileHTTP permits every arm (the phase-16 superset). HTTP rbac passes
        // this → byte-identical compile + evaluation to pre-extraction.
        ProfileHTTP Profile = iota
        // ProfileL4 permits only the L4-evaluable arms; the HTTP-only arms
        // (header / url_path permissions + principals) reject at compile.
        // (uri_template + matcher-extension arms already reject unconditionally.)
        ProfileL4
    )

    // armKind enumerates the HTTP-only arms a Profile gates. The L4-evaluable
    // arms (any / destination_ip / destination_port / destination_port_range /
    // requested_server_name permissions; any / authenticated / direct_remote_ip /
    // remote_ip / source_ip principals; and / or / not combinators) are never
    // gated — permits returns true for them under either profile.
    type armKind int

    const (
        armHeader  armKind = iota // permission.header / principal.header
        armURLPath                // permission.url_path / principal.url_path
    )

    // permits reports whether the profile allows the given HTTP-only arm.
    func (p Profile) permits(a armKind) bool {
        if p == ProfileHTTP {
            return true
        }
        // ProfileL4: header + url_path are HTTP-only → rejected.
        return false
    }
    ```
  - `internal/rbac/evaluator.go`: thread `profile Profile` through `buildPermissionEvaluators(perms, profile)` → `buildOnePermission(p, profile)` and `buildPrincipalEvaluators(prins, profile)` → `buildOnePrincipal(p, profile)` (recursion through `and`/`or`/`not` passes `profile` down). In `buildOnePermission`, in the `Permission_Header` + `Permission_UrlPath` cases, gate at entry:
    ```go
    case *rbacconfigv3.Permission_Header:
        if !profile.permits(armHeader) {
            return nil, errors.New("rbac: permission.header is HTTP-only (unsupported for L4 network RBAC)")
        }
        if r.Header == nil { ... } // existing nil-guard unchanged
        return &permHeader{matcher: r.Header}, nil
    case *rbacconfigv3.Permission_UrlPath:
        if !profile.permits(armURLPath) {
            return nil, errors.New("rbac: permission.url_path is HTTP-only (unsupported for L4 network RBAC)")
        }
        ...
    ```
    and the mirror in `buildOnePrincipal` for `Principal_Header` (`armHeader`) + `Principal_UrlPath` (`armURLPath`). **Wording is byte-stable-pending (D-P6 — finalized at the Task-15 `TestParseRejectConstants_ByteStable` table); the `rbac:` prefix + the engine-as-owner discipline (R-E) is the load-bearing invariant.**
  - `internal/rbac/rbac.go`: `BuildRulesEngine(r *rbacconfigv3.RBAC, profile Profile)` threads `profile` into `buildPermissionEvaluators`/`buildPrincipalEvaluators` — this is the GUARANTEED reject target (the rules-path is the only path that calls `buildOnePermission`/`buildOnePrincipal`). `BuildMatcherEngine(m *matchv3.Matcher, profile Profile)` takes the `profile` for signature symmetry, BUT note: the matcher-path (`matcher.New`) does NOT call `buildOnePermission`/`buildOnePrincipal` — its leaves are generic `internal/matcher` predicates (e.g. an HTTP-header *match-input* type) and its terminal is a `config.rbac.v3.Action`, so an `rbac` `Permission_Header` arm never appears there. **The `profile.permits(arm)` check therefore has no rbac-permission/principal leaf to gate in the matcher path.** IMPL DECISION (from Task 4 Step 1's NOTE + SPEC §3.4): if the matcher framework surfaces no profile-reachable HTTP-only leaf, the matcher-path HTTP-only rejection is satisfied vacuously (there is no HTTP-only rbac arm to reach) — pass `profile` to `BuildMatcherEngine` for symmetry/future-proofing but expect the rules-path test to carry the live reject; record the reachability finding in PROGRESS.md. Do NOT block on wiring a matcher-leaf reject that the framework cannot route through.

- [ ] **Step 4: Run → pass; engine package green.**
Run: `go test ./internal/rbac/... -v && go vet ./internal/rbac/...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/rbac/profile.go internal/rbac/profile_test.go internal/rbac/evaluator.go internal/rbac/rbac.go
git commit -m "phase 26.3 Task 4: input-capability Profile (ProfileHTTP/ProfileL4) + HTTP-only-arm reject in rules+matcher paths [SPEC 3.4; D-26.3-1; ADR-0216]"
```

---

## Task 5: Consumer #1 migration — flip HTTP rbac onto `internal/rbac/` (R4; D-26.3-6)

Repair `internal/filter/http/rbac/` to import the extracted engine (aliased), pass `ProfileHTTP`, and delete the moved bodies. The HTTP-derived `EvalContext` impl + the L4 stubs + the four named counters + per-route + the deny path STAY. After this task `go build ./...` is green again.

**Files:**
- Modify: `internal/filter/http/rbac/evaluator.go` (DELETE the moved evaluator body; keep nothing engine-internal here), `internal/filter/http/rbac/rbac.go` (import `rbacengine`; `compiledConfig` holds engine types; `*filter` implements `rbacengine.EvalContext`; builders pass `ProfileHTTP`; `filterStats` holds `*rbacengine.PerPolicyCounters`; `evaluateEngine` calls the engine `Evaluate`)
- Modify: `internal/filter/http/rbac/{evaluator,rbac}_test.go` (the consumer-level tests stay; the engine-internal tests already moved at Tasks 2/3 — delete their now-duplicate originals)

- [ ] **Step 1: (Tests already exist.)** The consumer-level phase-16 tests (DecodeHeaders / per-route TPFC / stat registration / SendLocalReply deny path) are the regression net. Confirm the engine-internal tests were removed from the HTTP package at Tasks 2/3 (no duplicate-symbol). The failing state right now is the BUILD (the HTTP package references moved symbols).

- [ ] **Step 2: Run → the HTTP package fails to build.**
Run: `go build ./internal/filter/http/rbac/ 2>&1 | head`
Expected: undefined `evalContext`/`buildCompiledRulesEngine`/`compiledRulesEngine`/`engineResultAllowed`/etc.

- [ ] **Step 3: Repair the consumer.**
  - `internal/filter/http/rbac/evaluator.go`: DELETE the moved body. If nothing consumer-specific remains in this file, delete the file (and drop it from the package) — or leave a thin file only if the consumer keeps local helpers (it does not; all evaluator logic moved).
  - `internal/filter/http/rbac/rbac.go`:
    - Add the aliased import: `rbacengine "github.com/esalaine/envoy-go/internal/rbac"` (D-26.3-5; the consumer is itself package `rbac`, so the alias is mandatory).
    - `compiledConfig`: `rules *rbacengine.CompiledRulesEngine` / `matcher *rbacengine.CompiledMatcherEngine` / `shadowRules` / `shadowMatcher` (same field names; engine types).
    - `buildCompiledConfig`: call `rbacengine.BuildRulesEngine(c.GetRules(), rbacengine.ProfileHTTP)` / `rbacengine.BuildMatcherEngine(c.GetMatcher(), rbacengine.ProfileHTTP)` (and the shadow pair). Wrap returned errors verbatim (the engine already prefixes `rbac:`; the HTTP consumer adds no new wrapping → byte-identical wording, R4).
    - `*filter` implements `rbacengine.EvalContext`: replace `var _ evalContext = (*filter)(nil)` with `var _ rbacengine.EvalContext = (*filter)(nil)`. The accessor method bodies (Header/URLPath/Method real; the six L4 accessors returning nil/0/""; SourcedMetadata/FilterState nil) are UNCHANGED.
    - `evaluateEngine(cc, ctx, shadow)`: keep the consumer-side rules-vs-matcher + primary-vs-shadow dispatch, calling `cc.rules.Evaluate(ctx)` / `cc.matcher.Evaluate(ctx)` (the engine methods returning `(rbacengine.EngineResult, string)`). Map `rbacengine.Allowed`/`rbacengine.Denied` to the consumer's disposition switch.
    - `filterStats`: replace the inline `perPolicy *sync.Map` + the `incPolicy` method with `perPolicy *rbacengine.PerPolicyCounters` (constructed in `newFilterStatsIfAbsent`). `emitPrimaryCounters`/`emitShadowCounters` call `cc.stats.perPolicy.Inc(cc.stats.reg, base, policyName, suffix)` (gated on `cc.trackPerRuleStats && policyName != ""` — unchanged). Delete the now-moved `incPolicy` method.
    - The `denyBody`/`filterName`/`TypeURL` consts + `DecodeHeaders` + `SendLocalReply` path: UNCHANGED.

- [ ] **Step 4: Run → build green + the phase-16 HTTP-rbac unit tests pass + import-cycle clean (D-26.3-6).**
Run: `go build ./... && go vet ./... && go test ./internal/filter/http/rbac/... ./internal/rbac/... -v`
Expected: PASS. (`go build ./...` proves no import cycle — `internal/rbac` imports neither consumer.)

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/http/rbac/
git commit -m "phase 26.3 Task 5: migrate HTTP rbac onto internal/rbac/ (ProfileHTTP; engine types; PerPolicyCounters) — consumer #1 [SPEC 3.2; R4; D-26.3-6; ADR-0216]"
```

---

## Task 6: R4 re-verification gate — phase-16 HTTP-rbac differential fixtures byte-exact green LIVE

The LOAD-BEARING extraction proof (SPEC §13 R4; AMEND-A6 — engine-correctness, NOT metadata). The HTTP-rbac dispatch genuinely changed (engine import flip), so this is run LIVE — NOT asserted-unaffected.

**Files:** none (verification only; record results in PROGRESS.md).

- [ ] **Step 1: Identify the phase-16 HTTP-rbac differential fixtures.**

```bash
ls test/fixtures/ | grep -iE 'rbac'   # find the phase-16 rbac cross-side fixture dir(s)
```

- [ ] **Step 2: Run them cross-side byte-exact vs reference Envoy v1.37.2** (use the project's differential harness invocation — `git grep -n 'func TestDifferential\|differential' test/ | head`; mirror how the 26.2 PROGRESS.md ran the suite). Run the rbac fixtures specifically + the full suite.
Expected: every phase-16 rbac fixture byte-exact green (R4). Record the exact fixture names + the green output in PROGRESS.md.

- [ ] **Step 3: Run the engine + consumer race gate.**
Run: `go test -race -short ./internal/rbac/... ./internal/filter/http/rbac/...`
Expected: PASS (race-clean; the per-policy `sync.Map` + the registry are the only shared state).

- [ ] **Step 4: Commit** (PROGRESS.md only):

```bash
git add docs/envoy-go/phases/26.3-network-filter-rbac/PROGRESS.md
git commit -m "phase 26.3 Task 6: R4 gate — phase-16 HTTP-rbac differential fixtures byte-exact green after engine extraction [SPEC 3.2; R4]"
```

---

## Task 7: `NoFlush` close semantics in `connection.Close` (F3; D-26.3-2)

26.1/26.2 collapsed `FlushWrite ≡ NoFlush` (`chain.go:369-371` ignores the `CloseType`). `rbac_network` enforced-deny uses `NoFlush`; 26.3 records + honors the distinction. The decision is the FRAMEWORK touchpoint (a `connection.Close` impl change + a `serveNetworkChain` honoring), NOT a callbacks-interface change (R2).

**Files:**
- Modify: `internal/filter/network/chain.go` (`connection.Close(ct)` records `ct`; `chainRuntime.closeType` + an accessor on `ChainRuntime`)
- Modify: `internal/filter/network/callbacks.go` (doc-comment: NoFlush now distinguished)
- Modify: `internal/listener/manager.go` (`serveNetworkChain` honors the recorded `CloseType`)
- Test: `internal/filter/network/chain_test.go` (NoFlush vs FlushWrite recorded distinctly)

- [ ] **Step 1: Write the failing test** (`chain_test.go`): a filter that closes `NoFlush` is recorded distinctly from one that closes `FlushWrite`.

```go
func TestConnectionCloseRecordsCloseType(t *testing.T) {
	// FlushWrite (default) path.
	rtF := NewChainRuntime([]NetworkFilter{&closeFilter{ct: FlushWrite}}, scriptedConn(nil), ConnFacts{})
	rtF.OnNewConnection()
	if !rtF.CloseRequested() {
		t.Fatal("FlushWrite close not requested")
	}
	if rtF.CloseType() != FlushWrite {
		t.Fatalf("CloseType = %v, want FlushWrite", rtF.CloseType())
	}
	// NoFlush path.
	rtN := NewChainRuntime([]NetworkFilter{&closeFilter{ct: NoFlush}}, scriptedConn(nil), ConnFacts{})
	rtN.OnNewConnection()
	if rtN.CloseType() != NoFlush {
		t.Fatalf("CloseType = %v, want NoFlush", rtN.CloseType())
	}
}
```

```go
// closeFilter closes the connection with a configured CloseType from
// OnNewConnection then halts.
type closeFilter struct {
	Marker
	cb ReadFilterCallbacks
	ct CloseType
}

func (f *closeFilter) OnNewConnection() Status {
	f.cb.Connection().Close(f.ct)
	return StopIteration
}
func (f *closeFilter) OnData(*Buffer, bool) Status               { return StopIteration }
func (f *closeFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *closeFilter) OnDestroy()                                {}
```

- [ ] **Step 2: Run → fail** (`undefined: (*ChainRuntime).CloseType`).
Run: `go test ./internal/filter/network/ -run 'CloseRecordsCloseType' -v`

- [ ] **Step 3: Minimal implementation.**
  - `chain.go`: add `closeType CloseType` to `chainRuntime` (defaults to `FlushWrite` = 0). `connection.Close(ct)` sets `c.rt.closeReq = true; c.rt.closeType = ct`. Add `func (rt *chainRuntime) closeTypeRequested() CloseType { return rt.closeType }` + the exported `func (c *ChainRuntime) CloseType() CloseType { return c.rt.closeTypeRequested() }`.
  - `callbacks.go`: update the `Close`/`CloseType` doc-comments — NoFlush is now honored (drop the "when 26.3 lands" deferral language; F3).
  - `manager.go` `serveNetworkChain`: at the pure-read close (`@1080`), honor the type:
    ```go
    // Honor the close semantics the filter requested (F3 — NoFlush drops any
    // pending downstream write; FlushWrite drains then closes). For a plain
    // net.Conn whose writes are already flushed synchronously the two are
    // operationally equivalent today, but the distinction is now byte-faithful
    // + future-proof (rbac_network enforced-deny uses NoFlush).
    if rtChain.CloseType() == network.NoFlush {
        // best-effort drop-and-close: set a zero linger if the conn is a
        // *net.TCPConn, then close. (Minimal correct NoFlush — D-26.3-2.)
        if tc, ok := dispatchConn.(interface{ SetLinger(int) error }); ok {
            _ = tc.SetLinger(0)
        }
    }
    _ = dispatchConn.Close()
    ```
    > **D-26.3-2 (NoFlush scope — IMPL decision):** the SetLinger(0) shape is the anticipated minimal-correct NoFlush. If the IMPL finds the differential deny fixture (Task 14) requires a graceful FIN rather than RST for byte-exact parity with upstream's `NoFlush` (upstream `NoFlush` drops the write buffer but still half-closes gracefully), DROP the SetLinger and instead just `Close()` without flushing a pending write (there is none) — record the empirical parity finding in PROGRESS.md. The load-bearing pin: the `CloseType` is RECORDED + reaches the close path distinctly; the exact socket-level behavior is parity-verified at Task 14.

- [ ] **Step 4: Run → pass; framework + manager green.**
Run: `go test ./internal/filter/network/... && go build ./... && go test -short ./internal/listener/...`
Expected: PASS (existing direct_response FlushWrite path unchanged; echo never closes).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/chain.go internal/filter/network/callbacks.go internal/filter/network/chain_test.go internal/listener/manager.go
git commit -m "phase 26.3 Task 7: distinguish NoFlush from FlushWrite in connection.Close + serveNetworkChain (F3) [SPEC 4.3; D-26.3-2; ADR-0218]"
```

---

## Task 8: `internal/filter/network/rbac/` package — parse + factory + the four static counters

The `rbac_network` package: `NewFactory(reg)` (closure-captures `*stats.Registry` — D-26.3-3) → `New(tc, ctx)` parses `networkrbacv3.RBAC` (TypeURL via `proto.MessageName`; `stat_prefix` PGV-required; `delay_deny` PARSE-REJECT; builds enforced+shadow engines via `rbacengine.ProfileL4`; registers the four `<stat_prefix>.rbac.*` counters). NO OnData decision yet (Tasks 9-10).

**Files:**
- Create: `internal/filter/network/rbac/rbac.go`, `internal/filter/network/rbac/rbac_test.go`

- [ ] **Step 1: Write the failing tests.**

```go
package rbac

import (
	"testing"

	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

func TestTypeURLViaProtoMessageName(t *testing.T) {
	// memory reference_network_filter_typeurl_extensions: derive, do not hand-type.
	if TypeURL != "type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC" {
		t.Fatalf("TypeURL = %q", TypeURL)
	}
}

func mustAny(t *testing.T, m *networkrbacv3.RBAC) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNew_StatPrefixRequired(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	_, err := factory(mustAny(t, &networkrbacv3.RBAC{ /* StatPrefix empty */ }), network.FactoryCtx{})
	if err == nil {
		t.Fatal("empty stat_prefix must PARSE-REJECT")
	}
}

func TestNew_DelayDenyRejected(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{StatPrefix: "p", DelayDeny: durationpb.New(0)}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err == nil {
		t.Fatal("delay_deny must PARSE-REJECT (AMEND-A9)")
	}
}

func TestNew_HTTPOnlyArmRejectedViaProfileL4(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{
		StatPrefix: "p",
		Rules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"x": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Header{}}},
					Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
				},
			},
		},
	}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err == nil {
		t.Fatal("permission.header must reject for L4 (ProfileL4)")
	}
}

func TestNew_FourStaticCountersRegistered(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	cfg := &networkrbacv3.RBAC{StatPrefix: "lis"}
	if _, err := factory(mustAny(t, cfg), network.FactoryCtx{}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, name := range []string{"lis.rbac.allowed", "lis.rbac.denied", "lis.rbac.shadow_allowed", "lis.rbac.shadow_denied"} {
		if findCounter(reg, name) == nil {
			t.Errorf("static counter %q not registered (predeclared-empty for scrape stability)", name)
		}
	}
}
```

> NOTE on `shadow_rules_stat_prefix`: when set, it inserts a segment between `rbac.` and the two `shadow_*` counters ONLY (enforced counters unaffected — SPEC §7.1). Add a `TestNew_ShadowRulesStatPrefixSegment` asserting `lis.rbac.<shadow_prefix>.shadow_allowed` when `ShadowRulesStatPrefix` is set. Reuse the HTTP filter's `baseStatPrefix`/`namespacePrefix` composition logic conceptually but with the NETWORK shape `<stat_prefix>.rbac.<...>` (NOT the HCM-rooted `http.<HCM>.rbac.<...>` — the network filter is chain-scoped, `stat_prefix` is the root).

- [ ] **Step 2: Run → fail** (`undefined: TypeURL`/`NewFactory`).
Run: `go test ./internal/filter/network/rbac/ -v`

- [ ] **Step 3: Minimal implementation** (`internal/filter/network/rbac/rbac.go`, package `rbac`):
  - Imports (aliased — D-26.3-5): `networkrbacv3` (extensions/filters/network/rbac/v3), `configrbacv3` (config/rbac/v3), `rbacengine` (internal/rbac), `network` (internal/filter/network), `stats`, `proto`, `anypb`, `structpb` (Task 11).
  - `TypeURL`: `var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&networkrbacv3.RBAC{}))` (derived, not hand-typed — memory).
  - `filterName` const `"envoy.filters.network.rbac"` (the dynamic-metadata namespace + the `rbac.` stat segment root).
  - PARSE-REJECT wording consts (byte-stable-pending, D-P6; pinned in the Task-15 `TestParseRejectConstants_ByteStable`): `parseRejectStatPrefixRequired = "rbac_network: stat_prefix is required"`; `parseRejectDelayDeny = "rbac_network: delay_deny is unsupported"`.
  - `compiledConfig` struct: `enforcedRules *rbacengine.CompiledRulesEngine` / `enforcedMatcher *rbacengine.CompiledMatcherEngine` / `shadowRules` / `shadowMatcher` / `enforcementContinuous bool` / `stats *filterStats`.
  - `filterStats`: the four `*stats.Counter` (allowed/denied/shadowAllowed/shadowDenied). NO `PerPolicyCounters` (F2 — network passes trackPerRuleStats=false → never constructs it).
  - `NewFactory(reg *stats.Registry) network.NetworkFilterFactory` returns `func(tc, ctx) (network.FilterInstanceFactory, error)`: parse `networkrbacv3.RBAC`; reject empty `StatPrefix` + non-nil `DelayDeny`; build enforced engine via `rbacengine.BuildRulesEngine(cfg.GetRules(), rbacengine.ProfileL4)` / `BuildMatcherEngine(cfg.GetMatcher(), rbacengine.ProfileL4)` (rules wins if both — mirror the HTTP consumer's switch); same for shadow from `GetShadowRules()`/`GetShadowMatcher()`; register the four counters against the captured `reg` under `<stat_prefix>.rbac.*` (with the `shadow_rules_stat_prefix` segment for the two shadow counters); set `enforcementContinuous = cfg.GetEnforcementType() == networkrbacv3.RBAC_CONTINUOUS`; return a `FilterInstanceFactory` closure (`func() network.NetworkFilter { return &filter{cc: cc} }` — the per-connection filter lands at Task 10).
  - For now the `filter` is a minimal read filter (OnNewConnection→Continue, OnData→Continue, OnDestroy no-op) so the package builds; the real decision lands at Task 10.

(Reuse the project counter-lookup helper for `findCounter` in the test — `git grep -n 'findCounter\|findStatCounterValue' internal/`.)

- [ ] **Step 4: Run → pass; package + build green.**
Run: `go test ./internal/filter/network/rbac/... -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/rbac/rbac.go internal/filter/network/rbac/rbac_test.go
git commit -m "phase 26.3 Task 8: rbac_network parse + NewFactory(reg) + 4 static counters (ProfileL4; stat_prefix req; delay_deny reject) [SPEC 4.1/7; D-26.3-3; ADR-0218]"
```

---

## Task 9: The L4 `EvalContext` impl (the §4.2 accessor mapping from `network.Connection`)

A non-stub `rbacengine.EvalContext` built from `network.Connection`. The HTTP-only accessors (`Header`/`URLPath`/`Method`) return ""/absent — they are UNREACHABLE (`ProfileL4` rejected those arms at compile, Task 4/8), but must be present to satisfy the interface.

**Files:**
- Create: `internal/filter/network/rbac/evalctx.go`, `internal/filter/network/rbac/evalctx_test.go`

- [ ] **Step 1: Write the failing test** (build a fake `network.Connection`, assert the accessor mapping):

```go
package rbac

import (
	"net"
	"testing"
)

// fakeConn implements network.Connection with scriptable L4 facts.
type fakeConn struct {
	local, remote net.Addr
	sni           string
	principals    []string
}

func (c *fakeConn) Write([]byte, bool)            {}
func (c *fakeConn) Close(network.CloseType)       {}
func (c *fakeConn) LocalAddr() net.Addr           { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr          { return c.remote }
func (c *fakeConn) RequestedServerName() string   { return c.sni }
func (c *fakeConn) DownstreamPrincipals() []string { return c.principals }

func TestL4EvalContext_MapsConnectionFacts(t *testing.T) {
	conn := &fakeConn{
		local:      &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 8443},
		remote:     &net.TCPAddr{IP: net.ParseIP("192.168.1.5"), Port: 51000},
		sni:        "svc.internal",
		principals: []string{"spiffe://td/a"},
	}
	ec := newL4EvalContext(conn)
	if got := ec.DestinationIP(); !got.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("DestinationIP = %v", got)
	}
	if ec.DestinationPort() != 8443 {
		t.Errorf("DestinationPort = %d", ec.DestinationPort())
	}
	if !ec.DirectRemoteIP().Equal(net.ParseIP("192.168.1.5")) {
		t.Errorf("DirectRemoteIP = %v", ec.DirectRemoteIP())
	}
	if !ec.RemoteIP().Equal(net.ParseIP("192.168.1.5")) { // == DirectRemoteIP at L4 (no XFF)
		t.Errorf("RemoteIP = %v", ec.RemoteIP())
	}
	if ec.RequestedServerName() != "svc.internal" {
		t.Errorf("SNI = %q", ec.RequestedServerName())
	}
	if got := ec.DownstreamPrincipal(); len(got) != 1 || got[0] != "spiffe://td/a" {
		t.Errorf("DownstreamPrincipal = %v", got)
	}
	// HTTP-only accessors are present-but-empty (unreachable under ProfileL4).
	if _, present := ec.Header("x"); present {
		t.Error("Header must be absent at L4")
	}
	if ec.URLPath() != "" || ec.Method() != "" {
		t.Error("URLPath/Method must be empty at L4")
	}
}
```

- [ ] **Step 2: Run → fail** (`undefined: newL4EvalContext`).
Run: `go test ./internal/filter/network/rbac/ -run 'L4EvalContext' -v`

- [ ] **Step 3: Minimal implementation** (`evalctx.go`): a `l4EvalContext` struct wrapping `network.Connection`, with a helper to split `net.Addr` → `(net.IP, uint32)`:
  ```go
  // var _ rbacengine.EvalContext = (*l4EvalContext)(nil)
  type l4EvalContext struct{ conn network.Connection }

  func newL4EvalContext(c network.Connection) *l4EvalContext { return &l4EvalContext{conn: c} }

  func (e *l4EvalContext) DestinationIP() net.IP        { ip, _ := addrParts(e.conn.LocalAddr()); return ip }
  func (e *l4EvalContext) DestinationPort() uint32      { _, p := addrParts(e.conn.LocalAddr()); return p }
  func (e *l4EvalContext) DirectRemoteIP() net.IP       { ip, _ := addrParts(e.conn.RemoteAddr()); return ip }
  func (e *l4EvalContext) RemoteIP() net.IP             { return e.DirectRemoteIP() } // no XFF at L4 (documented)
  func (e *l4EvalContext) RequestedServerName() string  { return e.conn.RequestedServerName() }
  func (e *l4EvalContext) DownstreamPrincipal() []string { return e.conn.DownstreamPrincipals() }
  // HTTP-only — unreachable under ProfileL4 (compile-rejected); present for the interface.
  func (e *l4EvalContext) Header(string) (string, bool) { return "", false }
  func (e *l4EvalContext) URLPath() string              { return "" }
  func (e *l4EvalContext) Method() string               { return "" }
  func (e *l4EvalContext) SourcedMetadata() any         { return nil }
  func (e *l4EvalContext) FilterState() any             { return nil }
  ```
  `addrParts(net.Addr) (net.IP, uint32)`: type-switch `*net.TCPAddr` (→ `a.IP`, `uint32(a.Port)`); fall back to `net.SplitHostPort(addr.String())` + `net.ParseIP` for other addr types; nil/0 on failure.

- [ ] **Step 4: Run → pass.**
Run: `go test ./internal/filter/network/rbac/... -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/rbac/evalctx.go internal/filter/network/rbac/evalctx_test.go
git commit -m "phase 26.3 Task 9: L4 EvalContext from network.Connection (IP/port/SNI/principals; HTTP accessors empty) [SPEC 4.2; ADR-0218]"
```

---

## Task 10: The OnData decision (allow→Continue / enforced-deny→rcd+NoFlush-close / shadow) + the sticky-halt regression

The per-connection `filter` decision logic: `OnNewConnection` returns `Continue` (CRITICAL — a `StopIteration` here sets the sticky `connHalted` flag blocking ALL `OnData`, per memory `reference_network_read_filter_onnewconnection_halts`); `OnData` makes the RBAC decision (once on first data for `ONE_TIME_ON_FIRST_BYTE`, every `OnData` for `CONTINUOUS`). Shadow metadata writes land at Task 11.

**Files:**
- Modify: `internal/filter/network/rbac/rbac.go` (the `filter` decision body)
- Test: `internal/filter/network/rbac/rbac_test.go`

- [ ] **Step 1: Write the failing tests.** (Use a fake `network.ReadFilterCallbacks` exposing a `*fakeConn`, a close-recorder, and a `SetResponseCodeDetails` recorder.)

```go
func TestOnNewConnection_NeverStopIteration_StickyHaltRegression(t *testing.T) {
	// memory reference_network_read_filter_onnewconnection_halts: a StopIteration
	// from OnNewConnection sets sticky connHalted blocking all OnData. rbac_network
	// MUST Continue from OnNewConnection.
	f := newTestFilter(t, allowAllConfig(t))
	if got := f.OnNewConnection(); got != network.Continue {
		t.Fatalf("OnNewConnection = %v, want Continue (sticky-halt-safe)", got)
	}
}

func TestOnData_AllowContinues_IncrementsAllowed(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f := newFilterWith(allowAllConfig(t), cb)
	f.SetReadFilterCallbacks(cb)
	if got := f.OnData(network.BufferOf("hello"), false); got != network.Continue {
		t.Fatalf("allow OnData = %v, want Continue", got)
	}
	if cb.conn.closed {
		t.Error("allow must not close the connection")
	}
	assertCounter(t, f, "allowed", 1)
}

func TestOnData_EnforcedDeny_ClosesNoFlushWithRCD_IncrementsDenied(t *testing.T) {
	cb := newFakeCallbacks(denyingConn()) // a connection no policy matches → default-deny
	f := newFilterWith(denyAllConfig(t), cb)
	f.SetReadFilterCallbacks(cb)
	if got := f.OnData(network.BufferOf("x"), false); got != network.StopIteration {
		t.Fatalf("deny OnData = %v, want StopIteration", got)
	}
	if !cb.conn.closed || cb.conn.closeType != network.NoFlush {
		t.Errorf("enforced deny must Close(NoFlush); closed=%v type=%v", cb.conn.closed, cb.conn.closeType)
	}
	if cb.rcd != "rbac_deny_close" {
		t.Errorf("response-code-details = %q, want rbac_deny_close", cb.rcd)
	}
	assertCounter(t, f, "denied", 1)
}

func TestOnData_OneTimeDecidesOnce(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f := newFilterWith(allowAllConfig(t), cb) // ONE_TIME_ON_FIRST_BYTE (default)
	f.SetReadFilterCallbacks(cb)
	f.OnData(network.BufferOf("a"), false)
	f.OnData(network.BufferOf("b"), false) // second OnData is pass-through, no re-decide
	assertCounter(t, f, "allowed", 1) // decided once
}
```

> NOTE: the helpers (`newFakeCallbacks`, the fake conn with `closed`/`closeType`, the `rcd` recorder via `SetResponseCodeDetails`, `network.BufferOf`/`assertCounter`) — model the fake callbacks on `chain_test.go`'s in-package fakes, but here they live in the network/rbac test package (cross-package), so they implement the EXPORTED `network.ReadFilterCallbacks`/`network.Connection` interfaces. `network.BufferOf` may not exist — construct a `*network.Buffer` via the exported constructor or append (check `git grep -n 'func.*Buffer' internal/filter/network/types.go`); if no exported Buffer constructor exists, the OnData signature takes `*network.Buffer` — build it the way `echo`/`directresponse` tests do.

- [ ] **Step 2: Run → fail.**
Run: `go test ./internal/filter/network/rbac/ -run 'OnData|OnNewConnection' -v`

- [ ] **Step 3: Minimal implementation.** The `filter` struct gains `cb network.ReadFilterCallbacks` + `decided bool`. Methods:
  ```go
  func (f *filter) OnNewConnection() network.Status { return network.Continue } // sticky-halt-safe

  func (f *filter) OnData(_ *network.Buffer, _ bool) network.Status {
      if f.decided && !f.cc.enforcementContinuous {
          return network.Continue // ONE_TIME: decided already → pass-through
      }
      f.decided = true
      ctx := newL4EvalContext(f.cb.Connection())

      // Shadow walk (never affects the enforced disposition) — Task 11 adds the
      // metadata write; here just the counters.
      if f.cc.shadowRules != nil || f.cc.shadowMatcher != nil {
          sres, sname := f.cc.evalShadow(ctx)
          f.cc.emitShadow(sres)         // shadow_allowed / shadow_denied
          _ = sname                      // effective policy id → metadata at Task 11
      }

      // Enforced walk.
      res, _ := f.cc.evalEnforced(ctx)
      switch res {
      case rbacengine.Allowed:
          f.cc.stats.allowed.Inc()
          return network.Continue // advance to the terminal (prefixConn handover)
      default: // rbacengine.Denied
          f.cc.stats.denied.Inc()
          if s, ok := f.cb.(interface{ SetResponseCodeDetails(string) }); ok {
              s.SetResponseCodeDetails("rbac_deny_close")
          }
          f.cb.Connection().Close(network.NoFlush)
          return network.StopIteration
      }
  }
  ```
  `evalEnforced`/`evalShadow` on `compiledConfig`: rules-wins-over-matcher dispatch (mirror the HTTP consumer) calling `engine.Evaluate(ctx)`. `emitShadow(res)` ticks `shadowAllowed`/`shadowDenied`. The `rbac_deny_close` string is byte-stable-pending (D-P6) but matches upstream's `setConnectionTerminationDetails("rbac_deny_close")`.

- [ ] **Step 4: Run → pass; race-clean.**
Run: `go test -race ./internal/filter/network/rbac/... -v`
Expected: PASS (incl. the sticky-halt regression).

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/rbac/rbac.go internal/filter/network/rbac/rbac_test.go
git commit -m "phase 26.3 Task 10: rbac_network OnData decision (allow→Continue / deny→rcd+NoFlush-close; OnNewConnection Continue no-op) [SPEC 4.2; AMEND-A8; ADR-0218]"
```

---

## Task 11: Connection-scoped shadow-metadata writes + the `internal/dynamicmetadata/` doc generalization (ADR-0217; R5)

When `shadow_rules`/`shadow_matcher` is configured, write the shadow pair to the per-connection bucket via `callbacks.DynamicMetadata()`. Generalize the package doc to scope-agnostic in place (ADR-0044). NO code change to `internal/dynamicmetadata/` (R5).

**Files:**
- Modify: `internal/filter/network/rbac/rbac.go` (the shadow walk writes the pair)
- Modify: `internal/dynamicmetadata/doc.go` (scope-agnostic doc)
- Test: `internal/filter/network/rbac/rbac_test.go` (the Bucket round-trip — R5)

- [ ] **Step 1: Write the failing test.**

```go
func TestOnData_ShadowWritesMetadataPair(t *testing.T) {
	cb := newFakeCallbacks(allowingConn()) // enforced allow + shadow deny configured
	f := newFilterWith(shadowDenyConfig(t), cb)
	f.SetReadFilterCallbacks(cb)
	f.OnData(network.BufferOf("x"), false)

	bucket := cb.DynamicMetadata()
	res, ok := bucket.Get("envoy.filters.network.rbac", "shadow_engine_result")
	if !ok || res.GetStringValue() != "denied" {
		t.Errorf("shadow_engine_result = %v ok=%v, want denied", res, ok)
	}
	// shadow_effective_policy_id written only when the matched policy id is non-empty.
	if id, ok := bucket.Get("envoy.filters.network.rbac", "shadow_effective_policy_id"); ok {
		if id.GetStringValue() == "" {
			t.Error("shadow_effective_policy_id written but empty")
		}
	}
	assertCounter(t, f, "shadow_denied", 1)
}

func TestOnData_NoShadowConfigured_NoMetadata(t *testing.T) {
	cb := newFakeCallbacks(allowingConn())
	f := newFilterWith(allowAllConfig(t), cb) // no shadow
	f.SetReadFilterCallbacks(cb)
	f.OnData(network.BufferOf("x"), false)
	if _, ok := cb.DynamicMetadata().Get("envoy.filters.network.rbac", "shadow_engine_result"); ok {
		t.Error("no shadow configured → no metadata write")
	}
}
```

(The `newFakeCallbacks` must expose a real `*dynamicmetadata.NewBucket()` via `DynamicMetadata()`.)

- [ ] **Step 2: Run → fail.**
Run: `go test ./internal/filter/network/rbac/ -run 'Shadow' -v`

- [ ] **Step 3: Minimal implementation.** In the shadow walk of `OnData` (Task 10), after `evalShadow`:
  ```go
  if f.cc.shadowRules != nil || f.cc.shadowMatcher != nil {
      sres, sname := f.cc.evalShadow(ctx)
      f.cc.emitShadow(sres)
      bucket := f.cb.DynamicMetadata()
      result := "allowed"
      if sres == rbacengine.Denied {
          result = "denied"
      }
      bucket.Set(filterName, "shadow_engine_result", structpb.NewStringValue(result))
      if sname != "" { // parity with upstream: write the id only when non-empty
          bucket.Set(filterName, "shadow_effective_policy_id", structpb.NewStringValue(sname))
      }
  }
  ```
  (`filterName == "envoy.filters.network.rbac"`; the keys are byte-faithful to upstream — §11.4 D4.)
  - `internal/dynamicmetadata/doc.go`: generalize the package doc — change "per-stream" framing to "scope-agnostic; owner-determined lifetime — per-stream (HTTP filter chain) OR per-connection (network filter chain)". Add the 26.3 note: "first per-connection production consumer = `rbac_network`'s shadow-pair writes (ADR-0217)." Do NOT touch `dynamicmetadata.go` (R5 — no code change).

- [ ] **Step 4: Run → pass; the dynamicmetadata package tests still green (doc-only change).**
Run: `go test ./internal/filter/network/rbac/... ./internal/dynamicmetadata/... -v`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/rbac/rbac.go internal/dynamicmetadata/doc.go internal/filter/network/rbac/rbac_test.go
git commit -m "phase 26.3 Task 11: connection-scoped shadow-metadata writes (shadow_engine_result/shadow_effective_policy_id) + dynamicmetadata doc generalized [SPEC 4.4; ADR-0217; R5]"
```

---

## Task 12: Register the 5th built-in + boot smoke (the `[rbac_network, tcp_proxy]` mixed chain) + the per-package stat-count pin

Wire `rbac_network` into `builtins.RegisterBuiltins` (the closure-captured `*stats.Registry` — D-26.3-3), and prove a `[rbac_network, tcp_proxy]` chain builds + classifies through the unified dispatch (the first production mixed read→terminal chain — pre-differential smoke).

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go` (register the 5th)
- Test: `internal/filter/network/builtins/builtins_test.go`; `internal/filter/network/rbac/rbac_test.go` (the per-package stat-count pin)

- [ ] **Step 1: Write the failing tests.**

```go
// builtins_test.go: rbac_network is registered + the chain classifies.
func TestRegisterBuiltins_RegistersRBACNetwork(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{StatsRegistry: stats.NewRegistry(), ClusterManager: testCM(t)})
	reg.Freeze()
	if _, ok := reg.Lookup(networkrbac.TypeURL); !ok { // network.Registry exposes Lookup(typeURL)(factory,bool) — registry.go:46
		t.Fatal("rbac_network not registered as the 5th built-in")
	}
}
```

```go
// rbac_test.go: the per-package surface is exactly 4 static counters per chain.
func TestProjectStatCount_RBACNetwork26_3(t *testing.T) {
	reg := stats.NewRegistry()
	factory := NewFactory(reg)
	if _, err := factory(mustAny(t, &networkrbacv3.RBAC{StatPrefix: "lis"}), network.FactoryCtx{}); err != nil {
		t.Fatal(err)
	}
	const want = 4 // allowed + denied + shadow_allowed + shadow_denied (no per-policy — F2)
	got := 0
	reg.Walk(func(stats.Metric) { got++ })
	if got != want {
		t.Errorf("rbac_network stat-count = %d; want %d (4 static; NO per-policy — F2)", got, want)
	}
}
```

- [ ] **Step 2: Run → fail** (rbac_network not registered).
Run: `go test ./internal/filter/network/builtins/ ./internal/filter/network/rbac/ -run 'RegistersRBACNetwork|StatCount' -v`

- [ ] **Step 3: Minimal implementation** (`builtins.go`): add the import `networkrbac "github.com/esalaine/envoy-go/internal/filter/network/rbac"` and the registration:
  ```go
  reg.Register(networkrbac.TypeURL, networkrbac.NewFactory(deps.StatsRegistry))
  ```
  (No `Deps` field change — `Deps.StatsRegistry` already exists.) The boot-wiring in `cmd/envoy-go/main.go` is unchanged structurally (the `RegisterBuiltins` call already runs there).

- [ ] **Step 4: Run → pass + the boot-smoke build-path test** (a `[rbac_network, tcp_proxy]` chain builds + classifies as read-prefix + terminal — reuse the manager build-path test harness from 26.2's `manager_test.go`; assert no `terminal-not-last`/`multiple-terminals` reject, and that `serveNetworkChain` would dispatch it as mixed). Run the full build + race gate:
Run: `go test -race ./internal/filter/network/... ./internal/listener/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit.**

```bash
git add internal/filter/network/builtins/ internal/filter/network/rbac/rbac_test.go
git commit -m "phase 26.3 Task 12: register rbac_network (5th built-in) + boot smoke [rbac_network,tcp_proxy] + per-package 4-counter stat pin [SPEC 4.6; D-26.3-3; ADR-0218]"
```

---

## Task 13: The 36th fuzzer `FuzzNetworkRBACConfigParse`

A config-parse fuzzer over the `rbac_network` `New` path (mirroring `internal/filter/network/directresponse/fuzz_test.go`).

**Files:**
- Create: `internal/filter/network/rbac/fuzz_test.go`

- [ ] **Step 1: Write the fuzzer** (model on the existing network direct_response fuzzer — `cat internal/filter/network/directresponse/fuzz_test.go` for the seed/harness shape):

```go
package rbac

import (
	"testing"

	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzNetworkRBACConfigParse drives the rbac_network config-parse path with
// arbitrary typed_config bytes — it must never panic; parse errors are fine.
func FuzzNetworkRBACConfigParse(f *testing.F) {
	// Seeds: empty, valid stat_prefix, delay_deny, http-only arm.
	seed := func(m *networkrbacv3.RBAC) []byte {
		a, _ := anypb.New(m)
		return a.GetValue()
	}
	f.Add(seed(&networkrbacv3.RBAC{}))
	f.Add(seed(&networkrbacv3.RBAC{StatPrefix: "p"}))

	factory := NewFactory(stats.NewRegistry())
	f.Fuzz(func(t *testing.T, body []byte) {
		tc := &anypb.Any{TypeUrl: TypeURL, Value: body}
		// Must not panic regardless of bytes.
		_, _ = factory(tc, network.FactoryCtx{})
	})
}
```

- [ ] **Step 2: Run the fuzzer briefly** (seed corpus + a short fuzz to confirm no panic):
Run: `go test ./internal/filter/network/rbac/ -run 'FuzzNetworkRBACConfigParse$' -v` then `go test ./internal/filter/network/rbac/ -fuzz 'FuzzNetworkRBACConfigParse$' -fuzztime 20s`
Expected: PASS; no crash. (Note: the per-fuzz-call `factory` reuses one registry — `NewCounterIfAbsent` is idempotent, so duplicate `stat_prefix` across fuzz iterations does not panic. Confirm; if the registry panics on re-registration, construct a fresh `NewFactory(stats.NewRegistry())` inside the `f.Fuzz` closure.)

- [ ] **Step 3: Re-confirm the fuzzer count = 36.**
```bash
cd "$(git rev-parse --show-toplevel)"; git ls-files '*fuzz_test.go' | xargs grep -h "^func Fuzz" | wc -l   # expect 36
```

- [ ] **Step 4: Commit.**

```bash
git add internal/filter/network/rbac/fuzz_test.go
git commit -m "phase 26.3 Task 13: 36th fuzzer FuzzNetworkRBACConfigParse [SPEC 15.1 Layer C]"
```

---

## Task 14: Differential fixtures — `rbac_network` cross-side (R-M-LIVE) + boot-reject

+2 fixture dirs (44 → 46). Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject are SEPARATE dirs (one dir = one runner branch). Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `StatsAsserter` (NOT `SubjectAsserter`) + MUST be proven live. Per `reference_network_filter_typeurl_extensions`: the differential bootstraps need a cluster (zero-cluster boot is rejected). Numbering continues from `0042`; the exact numbers (`0043`/`0044`) are pinned at IMPL.

**Files:**
- Create: `test/fixtures/0043-network-rbac/` (cross-side allow/deny/shadow), `test/fixtures/0044-network-rbac-boot-reject/` (stat_prefix-missing PGV-mirror) — exact names/numbers pinned at IMPL against the live tail.

- [ ] **Step 1: Study the existing network fixture shape.** Mirror the 26.1/26.2 network fixtures (`0040-network-echo`/`0041-network-direct-response`/`0042-network-direct-response-boot-reject`) for the bootstrap + asserter wiring. Confirm: (a) the cross-side dir uses `StatsAsserter` for the `<stat_prefix>.rbac.*` counters (memory — `SubjectAsserter` only runs on the reference-less path, would be a dead vacuous assertion); (b) the bootstrap has a cluster (zero-cluster boot rejected); (c) the boot-reject dir is its OWN runner branch.

- [ ] **Step 2: Author the cross-side `0043-network-rbac` fixture** — a listener with `[rbac_network, tcp_proxy]` (the FIRST production mixed read→terminal chain — R-M-LIVE). Bootstrap arms (may be one dir with multiple sub-cases or split per the harness — pin at IMPL):
  - **allow** — `direct_remote_ip` principal matching the test client + `destination_port` matching the listener → `Continue` → `tcp_proxy` passthrough → byte-exact echo/round-robin upstream response. `<stat_prefix>.rbac.allowed` increments (StatsAsserter).
  - **deny (enforced)** — a config the connection does NOT satisfy (default-deny or explicit DENY) → connection close, no upstream bytes; both upstream + subject close byte-exactly (R-N — the `NoFlush` close is byte-faithful). `<stat_prefix>.rbac.denied` increments (StatsAsserter).
  - **shadow** — `shadow_rules` (deny) + `rules` (allow) → enforced passthrough + `<stat_prefix>.rbac.shadow_denied` increments (StatsAsserter); the shadow metadata is emitted but unread (asserted indirectly via the stat + directly by the Task-11 unit test).
  - L4 inputs at minimum: `direct_remote_ip` principal + `destination_port` permission. **D-26.3-4 (IMPL):** add an SNI (`requested_server_name`) + mTLS-`authenticated` scenario IF the differential harness supports client certs (it does for the TLS-TCP/mTLS fixtures — confirm at IMPL); else unit-test the SNI/cert accessor mapping (Task 9) + note the differential gap in PROGRESS.md.

- [ ] **Step 3: Author the boot-reject `0044-network-rbac-boot-reject` fixture** — a `stat_prefix`-missing config (PGV-mirror parity — both upstream + envoy-go reject at boot; boot-stderr substring parity). The envoy-go-strict-only arms (HTTP-only matcher, `delay_deny` — upstream ACCEPTS them) are SUBJECT-SIDE-only rejects → covered by the Task-8 `manager.go`/filter build-path UNIT tests, NOT a cross-side fixture (dispatch-constraint memory: a subject-side-only reject is not cross-side parity).

- [ ] **Step 4: Prove the assertions are LIVE.** Run the two fixtures cross-side vs reference Envoy v1.37.2; confirm the `StatsAsserter` counter assertions actually execute (not vacuous — flip a counter expectation to a wrong value and confirm the test FAILS, then restore — the deliberate-break discipline per the asserter-dispatch memory). Confirm the allow path is the first LIVE mixed read→terminal differential (R-M-LIVE).
Run: the differential harness for `0043`/`0044` + the full suite (46 dirs).
Expected: byte-exact green; fixture count 46.

- [ ] **Step 5: Re-confirm the fixture count = 46.**
```bash
cd "$(git rev-parse --show-toplevel)"; ls test/fixtures/ | grep -E '^[0-9]' | wc -l   # expect 46
```

- [ ] **Step 6: Commit.**

```bash
git add test/fixtures/0043-network-rbac/ test/fixtures/0044-network-rbac-boot-reject/
git commit -m "phase 26.3 Task 14: +2 differential fixtures — rbac_network cross-side (allow/deny/shadow, StatsAsserter, R-M-LIVE) + boot-reject [SPEC 8]"
```

---

## Task 15: BEHAVIOR_CONTRACT 26.3 bundle + ADR §Decision/§Consequences bodies + ROADMAP rollup + six-gate

The atomic final task (ADR-0052): the behavior-contract bundle, the ADR-0216/0217/0218 §Decision/§Consequences bodies (in place; tail STAYS ADR-0218), the byte-stable reject-wording pin, the STATE/ROADMAP phase-done advance + the parent-row-26 ROLLUP, and the six-gate verification.

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Create/Modify: a `TestParseRejectConstants_ByteStable` table for the rbac_network reject wording (D-P6) — in `internal/filter/network/rbac/rbac_test.go` (mirror the direct_response byte-stable test, `directresponse.go:24-30`)

- [ ] **Step 1: Pin the byte-stable reject wording (D-P6).** Add/extend `TestParseRejectConstants_ByteStable` in `internal/filter/network/rbac/rbac_test.go` asserting the exact bytes of `parseRejectStatPrefixRequired` / `parseRejectDelayDeny`; and (in `internal/rbac/`) a test pinning the `ProfileL4` HTTP-only-arm reject wording. Finalize the exact strings here (R-S).
Run: `go test ./internal/filter/network/rbac/ ./internal/rbac/ -run 'ByteStable' -v`

- [ ] **Step 2: BEHAVIOR_CONTRACT.md 26.3 bundle** (§9 / §14): NEW `### envoy.filters.network.rbac` subsection under `## Network filters` (full enforced + shadow parity; the L4 `ProfileL4` principal/permission input surface; decision-in-`OnData` + OnNewConnection `Continue` no-op per the sticky-halt constraint; enforced-deny = `NoFlush` close + `rbac_deny_close` termination-detail; shadow = dynamic-metadata shadow-pair + `shadow_*` stats; the `rules`/`matcher` dual-path; CEL/audit silent-ignore inherited from the engine). UPDATE the stat table **132 → 136** (the four `<stat_prefix>.rbac.*` counters). Add the envoy-go-strict departure records: HTTP-only-matcher PARSE-REJECT (AMEND-A4); `delay_deny` PARSE-REJECT (AMEND-A9); xDS dynamic-policy PARSE-REJECT. Add the connection-metadata-emitted-but-unread note (namespace/keys; AMEND-A5/A6). Add the `internal/rbac/` engine-extraction structural note (HTTP rbac = consumer #1, re-verified byte-exact — no HTTP-rbac behavior change). Add the `NoFlush`-now-distinguished note (F3).

- [ ] **Step 3: DECISIONS.md ADR bodies (in place; ADR-0044; tail STAYS ADR-0218).** Fill the §Decision + §Consequences bodies for ADR-0216 (engine extraction + `Profile` + consumer-#1/#2 + R-E + the LIVE-first-consumer re-verification discipline), ADR-0217 (connection-scoped dynamic-metadata writes + the in-place doc generalization + no-reader), ADR-0218 (`rbac_network` — L4 input surface; OnData decision + sticky-halt; `NoFlush`+rcd deny; shadow metadata+stats; the 4-counter roster, NO per-policy F2; `delay_deny` reject; CEL silent-ignore F1; the `rules`/`matcher` dual-path; R-M-LIVE). NO new ADR number consumed (next-free STAYS ADR-0219).

- [ ] **Step 4: STATE.md + ROADMAP.md phase-done advance + ROLLUP.** STATE.md: `active-phase` → phase 26.3 IMPL DONE; `lifecycle-state` → phase-done; `last-commit` → (the squash, filled by the controller post-merge); counts fuzzers 36 / fixtures 46 / stat surface 136 / DECISIONS.md tail ADR-0218. ROADMAP: sub-row 26.3 `in-progress → done` + **parent row 26 `in-progress → done` ATOMICALLY** (the 18/19/22/24/25 ROLLUP); note the §9 Network-filters family stays OPEN (6 candidates remain: redis/mongo/kafka_broker/thrift/zookeeper/sni_cluster).

- [ ] **Step 5: Six-gate verification (run LIVE; quote outputs in PROGRESS.md).**

```bash
go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...
```
Then the FULL differential suite (46 fixtures — incl. the R4 phase-16 HTTP-rbac fixtures + the +2 new `rbac` dirs, run LIVE) + conformance 10/10 + h2spec 53/53 (asserted-unaffected — 26.3 touches no HTTP/h2/proxy-wasm path internals; the HTTP-rbac engine move is differential-gated by R4). Counts at phase-done: fuzzers **36**; fixtures **46**; stat surface **136**; DECISIONS.md tail **ADR-0218**.

Expected: all green. Record the honest outputs in PROGRESS.md.

- [ ] **Step 6: Commit.**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md internal/filter/network/rbac/rbac_test.go internal/rbac/ docs/envoy-go/phases/26.3-network-filter-rbac/PROGRESS.md
git commit -m "phase 26.3 Task 15: BEHAVIOR_CONTRACT 26.3 bundle + ADR-0216/0217/0218 bodies + ROADMAP rollup (parent 26 → done) + six-gate [SPEC 9/14/15]"
```

---

## Acceptance (SPEC §15.3 — verified at Task 15)

1. Shared `internal/rbac/` engine extracted (`EvalContext`/evaluators/builders/matcher-bridge/`PerPolicyCounters` MOVED + re-exported; `Profile` capability added); import-cycle-clean (D-26.3-6).
2. Consumer #1 (HTTP rbac) migrated onto `internal/rbac/` with `ProfileHTTP`; R4 byte-exact (phase-16 fixtures green LIVE); NO operator-visible HTTP-rbac change.
3. `rbac_network` lands: parse (`stat_prefix` required; `delay_deny` reject; `ProfileL4` HTTP-only-arm reject; CEL/audit silent-ignore inherited); L4 `EvalContext`; OnData decision (ONE_TIME + CONTINUOUS; OnNewConnection `Continue` no-op — sticky-halt-safe); allow→`Continue`→terminal; enforced-deny→`rbac_deny_close`+`NoFlush`-close+`StopIteration`; shadow→metadata+stats.
4. Connection-scoped shadow-metadata writes land (namespace/keys byte-faithful — R5); `internal/dynamicmetadata/` doc generalized (ADR-0044).
5. `NoFlush` close distinction lands (F3); the 5th built-in registered; boot smoke green.
6. Four static counters (`<stat_prefix>.rbac.*`) land (132 → 136); NO per-policy for network (F2); per-policy machinery dormant for consumer #2.
7. +2 differential fixtures (first LIVE mixed read→terminal `rbac` cross-side with StatsAsserter; boot-reject); 36th fuzzer; R-M-LIVE proven.
8. ADR-0216/0217/0218 §Decision/§Consequences bodies land (tail STAYS ADR-0218); BEHAVIOR_CONTRACT.md 26.3 bundle lands.
9. Six gates green; STATE.md advanced; ROADMAP sub-row 26.3 + parent row 26 `in-progress → done` (ROLLUP); §9 family stays OPEN (6 candidates remain).
