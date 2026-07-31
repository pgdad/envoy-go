# SPEC 81 — stats-name-charset-guards

**Stage:** SPEC (lifecycle-state `1` → `2`). **ROW 81 STAYS `in-progress`**; `ROADMAP.md` is **BYTE-UNTOUCHED** and the sentinel `want` STAYS **113**. Base master **`aab596e4`** — taken from `git rev-parse master` at session start, **not** from a SHA quoted in `next-prompt.txt` (the BRAINSTORM squash `3816a17f` is two router-only commits below the tip; branching off it would have silently discarded them). Worktree `/home/esa/git/envoy-go-wt/phase-81-spec`, branch `phase-81-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go`**. `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; `DECISIONS.md` gains **ADR-0303 §Context** only.

**Five investigation agents** ran in parallel, each in its own DETACHED worktree with private scratch and a private port band inside `42000-42499`; the controller re-derived every load-bearing claim itself rather than adopting a brief (`feedback_brief_citations_not_evidence`). Zero commits, zero pushes, zero branches by any agent; every experimental edit reverted by explicit path with `git status --porcelain` = 0 verified 5/5.

---

## 0. SENTINEL — RE-RUN MECHANICALLY AT THIS TIP. IT DOES **NOT** FIRE; `stop` WAS **NOT** CREATED

Run by the controller **before** any edit. `ls stop` ⇒ `No such file or directory`.

| check | ACTUAL output | NC, observed FIRING |
|---|---|---|
| **(1)** `want=113` | **`NOT DONE: row 81`** — correct; the row is `in-progress` and stays so until the IMPL | `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`; row 81 doctored `done` on a scratch copy ⇒ **SILENT**; row **62** doctored on that same copy ⇒ `NOT DONE: row 62` (so the check still discriminates when otherwise silent) |
| **(2)** | **FIVE** — `:191 :201 :211 :217 :225`. **UNCHANGED** | one-arm strip moves the union **5 → 4, NOT 5 → 0** |
| **(3)** | **`NEVER OPENED: gRPC`**, **`NEVER OPENED: WASM`** | invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`; registered `Observability` correctly silent |

Input measured at **229 lines / 113 data rows**, so an empty result could not read as a zero result (`reference_empty_output_is_not_a_zero_result`).

⚠️ **CHECK (2) IS UNCHANGED AT FIVE. THIS ROW NARROWS NOTHING — STATED, NOT FORECAST.** The **twenty-ninth** consecutive phase at which it did not go down. The charset candidate sits on no family's deferred-candidate sentence.

⚠️ **`want` STAYS 113.** This SPEC adds no ROADMAP row.

### 0.1 The leak check is **INAPPLICABLE**, not "passed"

**NEVER WRITE A SENTINEL'S OWN MATCHER STRING INTO A FILE THE SENTINEL GREPS.** The sentinel greps `docs/envoy-go/ROADMAP.md`. **This SPEC writes no ROADMAP cell** — the file is byte-untouched — so the check has no input and is **inapplicable**. Recorded for reference only: row 81's cell is 3260 B, carries **0** deferred-candidate phrases and exactly one family slug (`Observability-family row`, a *use*).

### 0.2 Row well-formedness — the DISJUNCTION re-executed over all 113 rows

**ARM-A** (escape-aware `NF!=8`) flags rows **57 (NF=9)** and **69 (NF=10)** only. **ARM-B** (escape-aware trailing-piece) flags row **78** only. **Neither arm alone suffices**; row 81 is flagged by neither.

⚠️ **PRECISION CORRECTION ON THE ROUTER.** Naive `NF==8` flags **SEVENTEEN** rows — `16 17 18 18.1 19 20 22.1 24.2 25.1 43 48 49 55 57 62 69 74`. Of those, **FIFTEEN are FALSE POSITIVES and TWO (57, 69) are TRUE**. The router presents that list as the false-positive set; it is the **flag set**. Row **78 is absent from it** — the compensating-defect cancellation. So the naive form is wrong in **both** directions: 15 FP **and** 1 FN.

---

## 1. SCOPE — what row 81 does

Close the unguarded config- and wire-derived token sources feeding `stats.Registry` name registration, so no such token can reach registration unvalidated. `internal/stats/registry.go:48` defines `NamePattern = ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`; `checkName` at `:115-119` **PANICS** on a violation, and there is no `recover()` in the request path.

⚠️ **THE COUNT IS NINE, NOT EIGHT.** Source **F** is two structurally independent sources (§4.1). The BRAINSTORM's roster of eight is otherwise exact — **all eight anchors verified byte-for-byte at `aab596e4`, zero drift.**

| src | token | site (symbol anchor) | fires at |
|---|---|---|---|
| **F1** | RBAC policy name, **rules** engine | `internal/rbac/rbac.go` :: `for name := range r.GetPolicies()` | ⚠️ **REQUEST TIME** |
| **F2** | RBAC policy name, **matcher** engine | `internal/rbac/rbac.go` :: `Evaluate` → `action.GetName()` | ⚠️ **REQUEST TIME**, and **not enumerable at boot** |
| A | mongo `stat_prefix` | `network/mongoproxy/config.go` :: `parseConfig` empty-check | boot |
| B | zookeeper `stat_prefix` | `network/zookeeperproxy/config.go` :: `parseConfig` empty-check | boot |
| C | wasm `PluginConfig.name` | `http/wasm/compiled_config.go` :: `newFilterStats(factoryCtx.Stats, pc.GetName())` | boot **and** request (per-route) |
| D | HTTP rbac `rules_stat_prefix` | `http/rbac/rbac.go` :: `rulesStatPrefix: c.GetRulesStatPrefix()` | boot |
| E | HTTP rbac `shadow_rules_stat_prefix` | `http/rbac/rbac.go` :: `shadowRulesStatPrefix: …` | boot |
| G | `compressor_library.name` | `http/compressor/compressor.go` :: `fmt.Sprintf("http.%s.compressor.%s.gzip.", …)` | boot |
| H | ratelimit `stat_prefix` | `http/ratelimit/compiled_config.go` :: `statPrefix: raw.GetStatPrefix()` | boot |

**Explicitly OUT of scope**, deferred to a named successor (§13): the **interior empty-segment hole**.

---

## 2. WHAT THIS SPEC REFUTES OR REFINES IN THE BRAINSTORM AND THE ROUTER

*A SPEC that refutes nothing has not looked.* Load-bearing first.

### 2.1 ⚠️ THE NINTH SOURCE — `F` IS TWO SOURCES, AND D-81-F-SITE'S REMEDY COVERS ONLY ONE

`PerPolicyCounters.Inc` is fed from two independent origins:

- **F1 — rules engine.** `internal/rbac/rbac.go:93` `for name := range r.GetPolicies()` → `compiledPolicy.name`. This *is* the config-compile loop D-81-F-SITE names. **Enumerable at boot.**
- **F2 — matcher engine.** `BuildMatcherEngine` (`:147-153`) does **only** `matcher.New(m, []string{actionTypeURL})` — a **TypeURL allowlist**. `Evaluate` at `:249`/`:251` returns `action.GetName()`, read out of the matcher tree's terminal `envoy.config.rbac.v3.Action` proto **at request time**. **Nothing enumerates those names at boot** — `git grep -n 'GetName()' -- internal/rbac/ internal/matcher/ ':!*_test.go'` returns only those two returns, their doc comments, and an unrelated header matcher.

Both reach the same two `perPolicy.Inc` call sites (`http/rbac/rbac.go:732` primary, `:760` shadow). Both arms are first-class: the HTTP filter builds all four (`:235` rules, `:241` matcher, `:254` shadow-rules, `:260` shadow-matcher) and the network filter builds two (`:162`, `:168`).

**PROVED BY EXECUTION with a firing NC** (probe written, run, deleted; tree clean afterwards):

```
BOOT ACCEPTED matcher Action.name="allow_admins" (BuildMatcherEngine err=nil)
  PerPolicyCounters.Inc registered cleanly                        ← NC arm
BOOT ACCEPTED matcher Action.name="allow-admins" (BuildMatcherEngine err=nil)
  *** PerPolicyCounters.Inc PANICKED: stats: invalid metric name:
      "http.myhcm.rbac.policy.allow-admins.allowed"
```

⇒ **A matcher-based RBAC config with a hyphenated `Action.name` still panics the process** under any boot-only remedy. This is the single largest gap in the BRAINSTORM's writeup, and it forces §4's two-part remedy.

### 2.2 ⚠️ D-81-HELPER'S TRADEOFF DOES NOT EXIST

The BRAINSTORM frames inlining as *"keeps the gate per-package"*, implying only a shared `internal/stats` helper triggers the 120-fixture differential + h2spec. **Measured:** `go list -deps ./cmd/envoy-go` = **560** packages and contains **8 of 8** — all seven target packages *and* `internal/stats`. NCs fired: `test/differential`, `test/conformance/h2spec` and an invented package are all ABSENT. Both designs perturb the same binary that `test/differential/harness.go:240`, `:594` and `test/conformance/h2spec/h2spec_test.go:210` build with `go build -o … ./cmd/envoy-go` (all three cites verified verbatim). **The gate obligation is IDENTICAL.** The whole measured asymmetry is **11.09 s** of extra unit tests.

⚠️ **And the doctrine cite is refuted.** `internal/stats/dynamic/dynamic.go:40-42`'s *"one definition, no drift"* is about one definition of the **regex source** `stats.NamePattern` — already exported, already shared — and `dynamic` does not even call `IsValidName`. `internal/stats/registry.go:53-59` states the **opposite** intent verbatim: *"Exposed so callers that derive metric names from user-controlled inputs … can validate at the input boundary and return a hcm-/cluster-prefixed error."*

⚠️ **"six packages" is SEVEN** — `internal/rbac`, `network/mongoproxy`, `network/zookeeperproxy`, `http/wasm`, `http/rbac`, `http/compressor`, `http/ratelimit`. Only D/E share one.

### 2.3 ⚠️ THE REFERENCE ACCEPTS THE HYPHEN VERBATIM — AND THE REAL GAP IS **PROJECTION**, NOT CHARSET

Controller-run, `envoyproxy/envoy:contrib-v1.37.2`, three arms, torn down by name:

| arm | `/ready` | request | `/stats` | `/stats/prometheus` |
|---|---|---|---|---|
| `allow-admins`, track=true | 200 | 200 | `http.myhcm.rbac.policy.allow-admins.allowed: 1` | `envoy_http_rbac_policy_allowed{envoy_rbac_policy_name="allow-admins",…} 1` |
| `allow_admins`, track=true (NC) | 200 | 200 | `…policy.allow_admins.allowed: 1` | `…{envoy_rbac_policy_name="allow_admins",…} 1` |
| `allow-admins`, **track absent** (control) | 200 | 200 | **empty** | **empty** |

The control arm proves the scrape discriminates, so the hyphen arm is a result rather than an absence.

⚠️ **The reference's METRIC NAME is `envoy_http_rbac_policy_allowed` — policy-name-INDEPENDENT — with the name hoisted into the label `envoy_rbac_policy_name`.** Label values have no charset to violate. **That is why upstream never needed a guard.** envoy-go **flattens** the policy name into the metric name and has **no `rbac_policy_name` extraction at all** (`git grep rbac_policy_name` ⇒ 0; NC `envoy_http_conn_manager_prefix` ⇒ 2 in `internal/stats/name.go`).

⇒ **Any reject here is an envoy-go-strict DEPARTURE, not a fix**, and must be written up as one (the phase-80 SDS secret-name precedent, `internal/boot/boot.go:267-273`). Hoisting would close the *conformance* gap but **not the panic** — `checkName` inspects the registered dotted name, which must still carry the policy name for any extractor to hoist from. The projection gap is named as a deferred candidate in §13.

### 2.4 ⚠️ THE CORPUS DOES **NOT** SURVIVE — ONE PROVEN HARD RED, INVISIBLE TO THE DIFFERENTIAL

BRAINSTORM §4.2's *"No existing fixture should red"* is **narrowly true and materially wrong**. Fixture YAML *is* clean. The red is in a **unit test**, exactly where §4.2 did not look:

```
internal/filter/http/ratelimit/compiled_config_test.go:574   StatPrefix:    "tenant-foo",
internal/filter/http/ratelimit/compiled_config_test.go:616   if cc.statPrefix != "tenant-foo" {
```

`IsValidName("tenant-foo")` = **false**. The test is green today (`--- PASS: TestBuildCompiledConfig/HappyPath`); guard **H** turns `:579`'s `t.Fatalf` live. ⚠️ **No differential run will ever find it** — `0032`/`0033` set only the **HCM** `stat_prefix`, never the ratelimit filter's own (verified: neither `envoy.yaml` contains a `stat_prefix` inside the `filters.http.ratelimit` `typed_config`).

**The SPEC schedules the edit: `tenant-foo` → `tenant_foo` at `:574` and `:616-617`.**

### 2.5 ⚠️ THE FOUR ROSTERS IN §4.2 ARE WRONG AS ROSTERS, RIGHT AS PROPERTIES

Measured by three independent extractors over **251** config files (200 parsed, 3 parse failures separately closed by direct grep) and **998** `.go` files, with an 11-arm negative control that **fired only after two of its own defects were fixed** — a mis-targeted injection (`reference_probe_input_is_a_claim`) and a real extractor blind spot on single-line `map[string]*P{…}` literals, whose repair surfaced **5 additional** policy names:

| §4.2 claim | verdict |
|---|---|
| "**111** distinct fixture-YAML `stat_prefix` values" | **REFUTED as a count** — **77** distinct / 340 occurrences in `test/fixtures/**`, **166** tree-wide. 111 matches neither denominator. **CONFIRMED as a property**: all clean. |
| "wasm plugin names are `plugin_a`/`plugin_bootreject`/`plugin_listener_default`" | **REFUTED as a roster** — **16** distinct in fixture YAML, **59** Name-ish literals in the wasm package. All clean. |
| "`compressor_library.name` is `text_optimized`" | **CONFIRMED** — 1 distinct value, 2 occurrences. But `""` is also legal and tested (§8). |
| "RBAC policy keys are `admin_users`/`p_allow`/`public_paths`" | **REFUTED as a roster** — **22** distinct. All clean. |

### 2.6 ⚠️ `BEHAVIOR_CONTRACT.md:931` IS A SCOPE OVER-READ, A NON-UNIQUE ANCHOR, AND NOT HONOURED TREE-WIDE

The sentence *"All boot-rejects are SUBJECT unit tests (no fixture dirs)"* is verbatim-real, but:

1. It occurs **FIVE** times — `:841 :863 :877 :913 :931` — and **every occurrence is inside a stats-sink subsection's "Strict-rejects (boot)" paragraph** (`metrics_service`, `statsd`, `dog_statsd`, `graphite_statsd`, OTLP). It is a **stats-sink-family convention**, not a project-wide invariant. **Row 81 touches no stats sink.** `:931` is merely the last of five, so the line anchor is arbitrary.
2. It is **not honoured tree-wide**: **13 `*-boot-reject` FIXTURE DIRS** exist, including in six of the exact families row 81 touches — `0033-http-ratelimit`, `0035`/`0037`/`0039-http-wasm-*`, `0044-network-rbac`, `0047-zookeeper`, `0050-mongo`, `0054-kafka`, `0056-redis`, `0058-thrift`, plus `0029`, `0031`, `0042`.
3. **The CONCLUSION is nevertheless correct, and a far stronger, on-family citation exists.** Those 13 dirs are symmetric cross-side PGV-mirror both-rejects. `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go:22-26` names this exact case verbatim:

> *"…distinct from the envoy-go-strict-only rejects (HTTP-only matcher arm / delay_deny / **invalid stat_prefix**) which upstream silently ACCEPTS … and which are therefore **subject-side-only rejects covered by the Task-8/Task-13 unit tests, NOT cross-side fixtures** (`reference_differential_fixture_dispatch_constraint`)."*

**The SPEC and the ADR cite `0044`'s driver, not `BEHAVIOR_CONTRACT.md:931`.**

### 2.7 ⚠️ THE ROUTER'S SDS "MISLEADING MESSAGES" CITES POINT AT THE WRONG FILES

`next-prompt.txt` §5.3 and BRAINSTORM §4.1 attribute two actively-misleading SDS messages to `internal/xds/stats.go` and `internal/boot/boot.go`. **`internal/xds/stats.go` contains no error message whatsoever** — it is a silent skip. The `"is not supported in phase 03"` messages live in **`internal/tls/config.go:437`/`:454`** (plus `:114`, `:171`, `:388`); `internal/boot/boot.go:161-162` only **quotes** one in a comment (*"the misleading-but-safe … message"*) — the citation was mistaken for the definition. Those are `--mode validate` scope and **out of row 81's remit**.

⚠️ **A genuinely misleading CHARSET message does exist, and it is a different one.** `internal/boot/boot.go:244` `invalidSecretNameErrFmt` serves **two legs** — segment-emptiness and charset — with one charset-only wording. Proved by execution: `""`, `"trailing_dot."` and `"a..b"` are all rejected with *"must contain only ASCII letters, digits, underscore, or dot…"*, and **`"a..b"` is `IsValidName`-VALID**. The message asserts a cause that is provably not the cause. **Row 81's eight config sources are pure-charset (the segment leg is out of scope), so that wording is accurate for them** — but §6 must not inherit it for any guard that ever gains a segment leg.

### 2.8 ⚠️ THE INCUMBENT TEMPLATE'S OWN RATIONALE IS WRONG, AND IT IS THE **WEAKER** ARM ON THE DEFERRED DEFECT

`internal/filter/network/rbac/rbac.go:105` says *"nameRE is whole-string-anchored so a bare-prefix check would mis-accept."* Measured over 11 inputs with a pure-literal suffix: **MIS-ACCEPTS = 0, OVER-REJECTS = 1.** A bare-token check can never mis-accept when the suffix is a fixed literal — if the token is a valid whole name, prepending it to `.<literal>` is still valid. It is strictly **stronger**: it rejects `foo.`, which the assembled arm **accepts** as `foo..rbac.allowed`.

The mis-accept the comment fears is real **only when a SECOND variable segment sits inside the name**: `bare(outer="ok") = true` while `assembled("ok.rbac.bad-shadow.shadow_allowed") = false`. **The correct rule is "guard EVERY variable segment", not "probe the assembled name"** — assembling merely happens to do that when every segment is in hand.

⚠️ **Consequence the BRAINSTORM does not model: adopting the assembled template means row 81's new guards INHERIT the interior-empty-segment hole BY CONSTRUCTION.** That is stated deliberately here, not discovered at the IMPL (§13).

⚠️ **The canonical anchor is an ADR, not that comment.** ADR-0065 §Consequences **(b)** states the longest-assembled-name rule *with its argument*: *"Validating the longest assembled name suffices because the other four assembled names … differ only in the suffix's last 4 chars … which are all in the regex's permitted character class — they pass/fail together."* §Consequences **(e)** is the standing obligation on future filter authors. **Cite ADR-0065, not `rbac.go:105`.**

### 2.9 THE CENSUS ARITHMETIC — 15 AND 8 ARE BOTH RIGHT, `~19` IS NOT

**15 guarded sites CONFIRMED**, every line anchor intact at `aab596e4`. Auditable derivation: raw `git grep 'stats\.IsValidName' -- '*.go'` = **40** → −15 `_test.go` → −2 `test/helpers/` → −8 comment lines = **15**. NC `stats.IsValidNameZZZ` ⇒ 0; dropping mongoproxy ⇒ 12 (fires).

⚠️ **The BRAINSTORM's stated derivation contains a phantom exclusion** — *"minus `registry.go`'s own definition"*. `registry.go:60` is `func IsValidName(`, **unqualified**, so it never enters a `stats\.IsValidName` grep. The number is right; the arithmetic as written does not close.

**The 15/8/~19 contradiction reconciles because 15 counts SITES, not TOKENS:**
- 15 sites ⇒ **13 distinct guarded tokens** (tracing is a second guard on the same HCM `stat_prefix` as hcm; `xds/stats.go` and `boot.go` are two guards on the same SDS secret name). 13 + 2 = 15. ✅
- 13 guarded + 8 unguarded = **21 tokens needing an `IsValidName`-class guard**.
- **+2 protected by a different mechanism**, for which the BRAINSTORM's binary taxonomy has no cell: a **closed-set allowlist** (`zookeeperproxy/stats.go` `authSchemeBuiltins`, whose comment says *"makes charset sanitization unnecessary"*) and a **sanitizer** (`listener/manager.go` `normalizeAddr` — the only one in the tree).
- ⇒ **23 distinct non-literal token sources. `~19` is REFUTED.** The coincidence `15+8=23` is accidental and must not be quoted as the reconciliation.

⚠️ **The 15 split 11 REJECT / 4 SKIP**, a distinction the flat table erases. **All four SKIP sites sit on post-Freeze / request-time paths where no error can be returned** — exactly source F's situation. That is four landed precedents for §4's backstop.

**`210` production registration sites CONFIRMED as a grep count, REFINED to 208 code sites** — 2 are comment lines in `internal/stats/doc.go:20,21`. The `11 pure-literal / 199 non-literal` split holds against 210; against code sites it is 11 / 197.

⚠️ **`clusterName` in `ratelimit/stats.go` is NOT a tenth source** — proved, not assumed: `validateGrpcServiceAndResolveCluster` does `ctx.ClusterManager.Get(clusterName)` and rejects `unknown cluster`, so the name must already have passed `cluster/manager.go:419`.

### 2.10 THE RETAINED-ITALIC-FOOTER COUNT IS **EIGHT**, NOT SEVEN

Mechanical per-block scan: ADR-**0294**(72) 0295(73) 0296(74) 0297(75) 0298(76) 0299(77) 0300(78) **0302**(80) = **8**; ADR-0301 carries none (its own recorded departure); NC `phase-999` ⇒ 0. The router's *"ADR-0294 … ADR-0300 — SEVEN"* is true of the **contiguous run** but **stale as a total** — ADR-0302 added the eighth at the phase-80 IMPL. ADR-0302's own STATUS enumerates all eight correctly and never totals them.

⚠️ **ADR-0301's STATUS is miscounted in a second way**: its *"seven blocks"* is the correct count for `0294…0300`, but the range it then names — *"ADR-0295 through ADR-0300"* — is **SIX**, dropping ADR-0294. **Copy ADR-0302's rendering, not ADR-0301's.** The id↔phase map is exact: ADR-NNNN → phase NNNN − 222, so **ADR-0303 → phase 81**.

### 2.11 NO `# SPEC record` HAS EVER EXISTED IN THIS REPO

Census over all `docs/envoy-go/phases/*/PROGRESS.md` — **91** files across **122** phase dirs: `# IMPL record` **8** · `# PLAN record` **4** · `# BRAINSTORM record` **1** · **`# SPEC record` 0**. The single BRAINSTORM record is phase 81's own, minted 2026-07-31. The dominant shape is per-Task sections (**86** of 91). ⇒ This SPEC mints a **repo-first heading**. It is the consistent continuation of what phase 81's BRAINSTORM started and is recorded here **as a new convention, not as compliance with one**.

---

## 3. D-81-HELPER — DISPOSED: **INLINE**

**Decision: reuse the already-exported `stats.IsValidName` at each boundary, with per-package byte-stable reject wordings. Do NOT add a new exported symbol to `internal/stats`.**

The gate argument is gone (§2.2), so the fork decides on the remaining measured axes, and inlining wins all of them:

| axis | INLINE (chosen) | shared helper |
|---|---|---|
| net `.go`, source D/E, prototyped both ways | **+60** | +100 (53 marginal + 47 one-time) |
| gate obligation | differential + h2spec | differential + h2spec (**identical**) |
| extra unit-test wall-clock | — | **+11.09 s** |
| new exported API surface | **0** | 1 |
| D-81-WORDING freedom | per-package byte-stable wordings | one central wording, per-package prefix only via `%w` |

A leaf sub-package `internal/stats/nameguard` was built and deleted: `git diff go.mod go.sum` = **0 lines**, so `reference_new_subpackage_pulls_transitive_module` does not apply — but it changes **no** gate obligation, because the seven target packages must still be edited and all seven are in `cmd/envoy-go`'s dep set. **No placement of a helper can avoid the differential + h2spec.**

⚠️ **PRICE THE GATE ONCE, FOR THE ROW AS A WHOLE.** The full 120-fixture differential (~400-430 s per green attempt, `-race` a SECOND run) and an **explicit h2spec run** are owed by *any* of the nine guards, not by a helper choice. h2spec is a FOURTH consumer **not** covered by `./test/differential/` — run it explicitly.

**Row-wide template:** the `network/rbac/rbac.go` shape — longest-assembled-name probe per ADR-0065 §Consequences (b), `errors.New(const)` or `fmt.Errorf(constFmt, …)` per §6.

⚠️ **Flagged for a successor, not fixed here:** `redisproxy/config.go:50`, `thriftproxy/config.go:44` and `kafkabroker/config.go:61` validate the **bare** prefix rather than an assembled probe. Per §2.8 that makes them *stronger*, not weaker, so nothing is owed on the merits — but the divergence is real and the BRAINSTORM surfaced neither it nor its direction.

---

## 4. D-81-F-SITE — DISPOSED: **TWO PARTS, BECAUSE F IS TWO SOURCES**

The BRAINSTORM offers a binary — boot-reject **or** degrade-to-skip. §2.1 makes them non-exclusive and makes neither alone sufficient.

### 4.1 The decision

**(a) F1 — BOOT REJECT, at `internal/filter/http/rbac/rbac.go buildCompiledConfig`, GATED ON `trackPerRuleStats`.**

⚠️ **The BRAINSTORM's own site is REFUTED.** `internal/rbac/rbac.go BuildRulesEngine` is the wrong file: it is **shared** (3 non-test call sites — HTTP `:235`, HTTP-shadow `:254`, network `:162`) and is **blind to `track_per_rule_stats`**. The network consumer **never constructs `PerPolicyCounters`** (`network/rbac/rbac.go:50-51`, and `PerPolicyCounters.Inc` has exactly **2** non-test call sites, both `http/rbac/rbac.go:732` and `:760`), so a guard there would boot-reject L4 configs that **cannot panic**.

The gated guard at `buildCompiledConfig` is one site with **two tier-dependent dispositions, for free**: a boot failure at the listener tier, and **log + inherit-listener** at the per-route tier — because `resolvePerRouteConfig` (`:346`, called from `DecodeHeaders` at `:798`) already implements exactly that policy per ADR-0072. ⚠️ **The per-route tier compiles at REQUEST time** (`:346` → `buildCompiledPerRoute` `:413` → `buildCompiledConfig(…, isPerRoute=true)`), and rbac registers **no `RegisterPerRouteValidator`** while 21 files repo-wide do. A pure boot reject cannot close that tier; this shape does.

**(b) F2 — SKIP-AND-LOG BACKSTOP at `internal/rbac/perpolicy.go PerPolicyCounters.Inc`.**

F2's names live only inside the matcher tree's terminal `Action` Anys and **cannot be boot-rejected without new tree-traversal code that nothing in `internal/matcher` exposes today**. The `Inc` chokepoint is the **only single site covering F1, F2, primary and shadow simultaneously**, it needs no traversal, and it has **four landed in-tree precedents** — the 4 GUARD-SKIP sites of §2.9, all on paths that cannot return an error.

Extend the existing `policyName == ""` early return to `policyName == "" || !stats.IsValidName(key)`, with the phase-79 diagnostic posture: **one aggregated log line per call site**, not one per request.

### 4.2 Why both, and not either alone

- **(b) alone** would leave the enumerable case silent, and ADR-0065 §Consequences (e) obliges validation at the boundary *where the input enters process state*. It would also reverse row 80's landed doctrine, which converted a silent registration skip **into** a loud boot reject.
- **(a) alone** leaves F2 panicking the process — the exact defect the row exists to close.
- ⇒ **(a) closes what is enumerable; (b) guarantees no un-enumerated path can panic.** Row 80's remedy was available because SDS secret names are a closed boot-time list. F2's are not. **This asymmetry is the reason the row needs two parts, and it must be written into ADR-0303 so it is not re-litigated.**

⚠️ **(a) is an envoy-go-strict DEPARTURE** (§2.3): the reference boots, serves, and emits the hyphen verbatim. It must be documented as a departure — inline in the owning `BEHAVIOR_CONTRACT.md` feature section, since the file has **no** departure-ledger heading (departures are marked inline; **52** lines carry uppercase `DEPARTURE`, **215** carry `envoy-go-strict`).

⚠️ **Sanitizing is worst on this source specifically**: `allow-admins` and `allow_admins` would collapse to one counter — silent cross-policy loss in an **authorization** stat.

**Explicitly OUT of scope:** the matcher-tree `Action`-name walk that would let F2 be boot-rejected. Named in §13.

---

## 5. D-81-SANITIZE — DISPOSED: **THE CONTRADICTION DISSOLVES ON SCOPE; REJECT ANYWAY**

**ADR-0065 does not contain a general foreclosure of sanitizing.** It rejects **one candidate (A) for one token**, and the rejection is a **TWO-LIMB CONJUNCTION** (`DECISIONS.md:2364`, verbatim):

> *"`stat_prefix` is preserved verbatim as the Prometheus label `envoy_http_conn_manager_prefix=<stat_prefix>`. Sanitising would silently mutate that label vs upstream Envoy's emission. **Two stat_prefixes differing only in invalid chars would collapse to the same Prometheus label value — a silent data-loss bug.**"*

— limb (i) the token is hoisted verbatim into a pinned label; limb (ii) the map is **non-injective** over the token's domain.

⚠️ **And §Decision `:2374` cites `normalizeAddr` APPROVINGLY**: *"It is also the same shape the listener path already uses (the `normalizeAddr` pre-pass guarantees the assembled name is always valid)."*

`normalizeAddr` **fails limb (ii)**: `internal/listener/manager.go:1085` sets `rt.addr = ln.Addr().String() // capture resolved address` at Start, **post-bind**, before `registerListenerMetrics` at `:1094`. The operator's configured string never reaches it — a malformed address is boot-rejected by `net.Listen` first. The map is injective over that closed grammar (probed, including zone-scoped IPv6, which Go's `TCPAddr.String()` elides).

⇒ **THERE IS NO CONTRADICTION.** Phase 80's *"Sanitizing is FORECLOSED by ADR-0065"* is correct for the secret name and **over-general as a sentence**. **ADR-0302 §Consequences (vii) item 5's *"internally inconsistent"* finding is PARTIALLY REFUTED** — it drops limb (ii) and the post-bind fact, and cannot accommodate §Decision's approving citation.

**Measured doctrine ratio: 1 sanitizer : 15 rejecters.** `strings.Map` has 0 hits repo-wide; the only production stat-name sanitizer is `normalizeAddr`.

⇒ **All nine phase-81 sources are open-domain operator tokens satisfying BOTH limbs. REJECT (or skip), never sanitize.** Write the two-limb discriminator into ADR-0303 so it is not re-litigated.

---

## 6. D-81-WORDING — DISPOSED: EIGHT BYTE-STABLE WORDINGS

**Incumbent census.** 4 of the 15 sites emit **no message at all** (the GUARD-SKIP set). The 11 message-emitting sites use **6 distinct templates**. Quantified divergence: value echoed `%q` in **6 of 11**; pattern echoed in **1 of 11** (regex literal), described in prose in 3, absent in 7; `%w`-wrapped **1 of 11**; **4 distinct nouns** for the artifact ("stat name" / "metric name" / "metric-name segment" / "stats name"); construction `errors.New(const)` ×5, `fmt.Errorf(constFmt, …)` ×2, inline `fmt.Errorf` ×3, `%w` ×1.

⚠️ **The split is by FAMILY, not random**: all five *network-filter* `stat_prefix` guards use a no-echo bare const; all *HTTP-filter / core* guards echo. The proposal preserves that rather than imposing uniformity.

| src | package | wording |
|---|---|---|
| **A** | `network/mongoproxy/config.go` | `errStatPrefixInvalid = "mongo_proxy: stat_prefix contains characters invalid for a metric name"` → `errors.New(…)` |
| **B** | `network/zookeeperproxy/config.go` | `errStatPrefixInvalid = "zookeeper_proxy: stat_prefix contains characters invalid for a metric name"` → `errors.New(…)` |
| **C** | `http/wasm/compiled_config.go` | `parseRejectPluginNameInvalidFmt = "wasm: invalid config.name: %q (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)"` |
| **D** | `http/rbac/rbac.go` | `fmt.Errorf("rbac: invalid rules_stat_prefix: %q (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)", …)` |
| **E** | `http/rbac/rbac.go` | as D, with `shadow_rules_stat_prefix` |
| **F1** | `http/rbac/rbac.go` | `fmt.Errorf("rbac: policy %q: track_per_rule_stats is set but the policy name cannot form a valid metric name (ASCII letters, digits, underscore, or dot only)", name)` |
| **G** | `http/compressor/compressor.go` | `fmt.Errorf("compressor: invalid compressor_library.name: %q (must contain only …)", libraryName)` |
| **H** | `http/ratelimit/compiled_config.go` | `parseRejectStatPrefixInvalid = "ratelimit: stat_prefix contains characters invalid for a metric name"` → `errors.New(…)` |

**F2** emits no reject — it is a skip; its log line is `rbac: per-policy stat skipped: policy name %q cannot form a valid metric name` (one aggregated line per call site).

**Identifier collision check run** (`reference_spec_drafted_identifier_collision_check`): `parseRejectPluginNameInvalidFmt` ⇒ 0 tree-wide; `errStatPrefixInvalid` exists in 6 files but **not** in mongoproxy/zookeeperproxy; `parseRejectStatPrefixInvalid` exists only in `network/rbac`. **All free in their target packages.**

**Value-echo safety.** All nine tokens are **config-derived; none is wire-derived**. Every wire-derived token in the tree is already protected and emits no message, so the row introduces **zero new wire-derived echoes**. ⚠️ In an xDS/Gateway-API deployment the "config" is control-plane-supplied, so **`%q` — not `%s` — is load-bearing, not stylistic**: it escapes control bytes and non-ASCII, so a hostile prefix cannot inject a newline into the boot log.

**Lint constraints, VERIFIED BY RUNNING.** `.golangci.yml` is `disable-all: true` with **9** enabled (`govet errcheck staticcheck unused ineffassign gofmt goimports misspell revive`) — **BRAINSTORM CONFIRMED**; golangci-lint **v1.64.8**. `misspell` locale US **applies** — the eight strings pass at exit 0, and an injected `behaviour`/`analyser`/`colourful` NC **fired**. ⚠️ **`ST1005` is NOT enforced** — `stylecheck` is absent, v1.x `staticcheck` covers SA-checks only, and `revive` is restricted to `package-comments` + `exported`. Probed directly: `errors.New("Invalid stat prefix.")` ⇒ **exit 0**. ⇒ **No enabled linter constrains error wording beyond US spelling.** The lowercase/no-trailing-period habit is honoured for consistency; **the SPEC does not claim a gate that does not exist.**

---

## 7. D-81-DEPTH — A **NEW** FORK THE BRAINSTORM NEVER NAMED. DISPOSED: **TABLE-DRIVEN-SHARED**

The guards themselves are ~5 lines each. **The row's size is set almost entirely by per-source test depth**, and the in-tree spread for a single charset guard is **5 lines** (thriftproxy) to **388 lines** (phase-80's boot reject) — **78×**.

| posture | net `.go` | margin vs ~1500 |
|---|---|---|
| **table-driven-shared (CHOSEN)** | **~850-1000** | 1.5-1.8× clear |
| per-source-deep (the phase-80 shape, and the PLAN's default if unstated) | ~1390-1500 | **0.0-1.1× — AT the trigger** |

⇒ **The SPEC MANDATES table-driven-shared.** That single decision is worth ~450-500 net lines and is the difference between clearing §6.1 and crossing it. **Left undecided, the PLAN will default to the phase-80 shape because it is the nearest precedent, and the row will cross.**

**Per-source budget unit:** the `http/lua/compiled_config_test.go` **3-arm shape** — invalid / valid-passthrough / empty-passthrough — is the richest incumbent and the right unit. The byte-sweep and longest-suffix audits are written **once, table-driven over all nine sources**, not once per source.

---

## 8. D-81-EMPTY — A **SECOND** NEW FORK. DISPOSED: **SKIP-IF-EMPTY**

`IsValidName("")` = **false**. An unconditional guard therefore reds **seven** tests that assert empty is *accepted*:

| src | test | file:line |
|---|---|---|
| D | `TestBuildCompiledConfig_EmptyRulesStatPrefix_Accepted` | `http/rbac/rbac_test.go:197` |
| E | `TestBuildCompiledConfig_EmptyShadowRulesStatPrefix_Accepted` | `http/rbac/rbac_test.go:213` |
| G | `TestNew_LibraryName_EmptyAllowed` | `http/compressor/compressor_test.go:291` |
| G | `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` | `http/compressor/compressor_test.go:2376` |
| C | `TestBuildCompiledConfig_Arm26_EmptyName_SkipsRegistry` | `http/wasm/compiled_config_test.go:1239` |
| C | `TestUnregisterPluginConfigName_EmptyName_NoOp` | `http/wasm/compiled_config_test.go:1273` |
| H | 2 `ratelimitfilterv3.RateLimit{}` literals leave `StatPrefix` unset | `ratelimit/{compiled_config,fuzz}_test.go` |

⚠️ **G is the sharp one, and it is ADR-anchored.** `compressor.go:526` says verbatim *"the fmt.Sprintf surfaces the consecutive-dots D5 shape when libraryName == "" — **DO NOT collapse**"*, and `compressor_test.go:2373-2375` calls itself *"the **LOAD-BEARING D5 behavioral pin**"*. **ADR-0132 §Decision (v)** (`DECISIONS.md:6291`) pins it against the reference: *"Operators with bare-Gzip configs (no `name:` set) emit stats under `compressor..gzip.<counter>` … Phase-14 envoy-go MUST mirror this exact shape."* **Empty is landed, deliberate, reference-anchored behaviour.**

**A/B/F are immune** — mongo and zookeeper already `errStatPrefixRequired` on empty (`config.go:81`/`:169`), and `PerPolicyCounters.Inc` already early-returns on `policyName == ""`. **C/D/E/G/H are not.**

⇒ **Every guard is `if tok != "" && !stats.IsValidName(assembled) { reject }`.** This is a SPEC decision, not an IMPL detail: the alternative reds seven tests and contradicts a landed ADR.

**Row 81's guard on G is safe even with the empty name**, because the resulting interior `..` is **accepted** by `NamePattern` (`IsValidName("a..b") = true`). The PLAN still owes the D5 non-regression arm.

---

## 9. BLAST RADIUS

- **One proven hard RED**: `ratelimit/compiled_config_test.go:574`/`:616-617` (§2.4). Scheduled.
- **Seven empty-value tests** — neutralized by §8's skip-if-empty, not edited.
- **Zero fixture YAML values fail** — across 1830 extracted values, **0** contain an interior `..`, a leading dot or a trailing dot; the only dotted value anywhere is `http.test` (a single interior dot, not one of the nine sources).
- **`guest-side-config`** (`wasm/compiled_config_test.go:1147`) looks like a violation and is **not** one: it is a `PluginConfig{Name:…}` used only as an opaque `anypb` payload for `PluginConfig.configuration`; guard C reads the **outer** `pc.GetName()`. **Recorded so a later sweep does not "fix" it.**
- **`my-script-prefix` / `bad name` / `bad prefix!`** in lua/kafka/thrift/redis are the **existing reject arms** of already-guarded sources.
- **`StatPrefix: "rls"` / `"google_grpc"`** are `corev3.GrpcService_GoogleGrpc.StatPrefix` — a **different field**, not source H.

**Stat-surface delta +0.** Guards register no names. Assert it by **enumerating the diff's registration call sites and showing the set is EMPTY**, against the **208-code-site / 84-file** denominator — the `TestNoNewStat*` guards are **proven blind to `internal/stats`** (all five live in `internal/statssink/registration_test.go`).

---

## 10. DIFFERENTIAL AND FIXTURE POSTURE — **ZERO NEW FIXTURES**, FOR A STRONGER REASON THAN §4.2's

**Per-source fixture coverage, measured:**

| src | configured by N fixtures |
|---|---|
| **F1/F2** | ⚠️ **0** — `track_per_rule_stats` appears **once** in all of `test/`, as PROSE in `0018-http-rbac/README.md:293` listing it as *deferred*. No fixture YAML enables it. NC: `rules_stat_prefix` ⇒ 43 hits. |
| **H** | ⚠️ **0** — `0032`/`0033` set only the HCM `stat_prefix` |
| A | 4 — `0049 0050 0051 0052` |
| B | 3 — `0046 0047 0048` |
| C | 6 — `0034`-`0039` |
| D/E | 1 — `0018` |
| G | 1 — `0016` |

⚠️ **The two zero-coverage sources are the two that matter most.** The differential can neither regress on nor cover the §1 headline crash **or** the one proven red. ⇒ **Unit tests are the only venue**, per `0044-network-rbac-boot-reject/driver/driver.go:22-26` (§2.6). **Do not budget differential time as evidence for this row's correctness** — budget it as a regression anchor only.

⚠️ **The request-time test shape is a genuine gap.** Both `:931` and the `0044` precedent say *"boot-rejects"*; **no request-time reject exists in the tree** — the four request-time guards all skip silently, and their tests assert a **no-panic + no-counter** shape (`mongoproxy/codec_test.go:518,539`). That is the precedent F2's backstop test must follow. Source **C is not affected** — its per-route path re-enters `buildCompiledConfig`, so a boot-site guard covers both tiers.

**Targeted differential proof run** (`-count=1`, not the full suite):
```
--- PASS: TestDifferential/0016-http-compressor              (3.13s)
--- PASS: TestDifferential/0018-http-rbac                    (1.96s)
--- PASS: TestDifferential/0034-http-wasm-headers-bridge     (2.27s)
```
Selector match **asserted positively** by the three named `--- PASS:` lines (a missing blank import would show `--- SKIP`). ⚠️ **The `-run` no-match NC FIRED**: a deliberately-wrong selector printed `testing: warning: no tests to run` / `[no tests to run]` and **exited 0** — a green run can mean "did not run" (`reference_differential_run_selector`).

---

## 11. CONTRACT AND ADR EDITS (owed at the IMPL, specified here)

- **ADR-0303** — §Context lands at this SPEC; §Decision + §Consequences at the IMPL (ADR-0044-**as-used**; ⚠️ ADR-0044 itself is titled *"BEHAVIOR_CONTRACT HTTP/1.1 subsection"* and contains **zero** text about that discipline — the misattribution is real and already measured at ADR-0297 §Context ¶8). Retained italic footer per §2.10. **Carry no whole-file grep count** — enumerate by site instead.
- **`BEHAVIOR_CONTRACT.md`** — at the IMPL, a departure paragraph in the **HTTP filter chain** section (the file has no departure-ledger heading). **Byte-untouched at this SPEC.**
- **`ROADMAP.md`** — byte-untouched at this SPEC and at the PLAN; row 81 flips `done` at the IMPL six-gate.

⚠️ **Quote `/BOOTSTRAP_PROMPT.md:289-290` directly, never ADR-0045's rendering.** ADR-0045 **quotes rather than states** the split gate and **mis-locates it to "§5 state 2"**; its real home is §6.1 `:285-292`. ADR-0045's §Context is nonetheless the lineage's canonical worked example — per-bucket task **and** LoC counts, summed, both thresholds named — and **ADR-0303 §Context must reproduce that form**.

---

## 12. COST AND SPLIT

**Three independent measurements, reconciled:**

| source | figure | basis |
|---|---|---|
| A1 | ~30/source ⇒ ~240 for eight | D/E prototyped both ways, thin test |
| A2 | **174 for F1 alone** | fully built: two tiers + shadow + gate, 5 test arms |
| A4 | 850-1000 shared-depth · 1390-1500 per-source-deep | scaled off phase-80's **realized** 388-line reject |

The spread **is** D-81-DEPTH (§7). Reconciled on the chosen posture: **F1 174 (measured) + F2 backstop ~50 + 7 sources × ~75 + shared table-driven audit ~140 ≈ 890.**

**SPEC FIGURE: band ~850-1200 net `.go`, budget ~1000. Tasks ~14.**

| trigger | value | margin | fires? |
|---|---|---|---|
| `~25 numbered tasks` (`/BOOTSTRAP_PROMPT.md:289`) | ~14 | 1.8× | **no** |
| `~1500 lines of code` net (`:290`) | ~1000 (band 850-1200) | 1.25-1.75× | **no** |
| mid-execution >~10 sub-steps (`:292`) | — | — | ⚠️ **record if it fires; never absorb it** (it fired at phase 80: 6 enumerated vs 17 executed) |

⚠️ **Calibration, honestly.** Phase 80 estimated ~640 and realized **1636** (2.56×). But **the fixture bucket was 631 of that 1636 (38%) at 4.10×, and phase 81 has +0 fixtures** — so the applicable, fixture-excluded multiplier is **1.97×**, not 2.56×. And A1/A2's figures are **realized-basis prototypes**, not estimates, so they must **not** be re-multiplied. Read the budget as *"expect the ceiling"*: 76 `~7-9→9` · 77 `11-13→12` · 78 `7-9→10` · 79 `10-12→12` · 80 `11-14→13`.

### ⚠️ THE ROUTER'S SPLIT AXIS IS REFUTED

*"SPLIT 81.1 (the eight sources) / 81.2 (the empty-segment retrofit)"* **is not a split** — the retrofit is already out of scope. Re-badging it as 81.2 would, per `/BOOTSTRAP_PROMPT.md` §6.2 items 4-5, park **row 81 `in-progress` until the retrofit lands**, re-coupling the request-time availability fix to work it was deliberately decoupled from. **Strictly worse than the BRAINSTORM's own posture.**

**If the PLAN's own enumeration lands above ~1200, split on this axis instead** (§6.2 item 3, a coherent slice):
- **81.1 `rbac-stat-name-guards`** — F1 + F2 + D + E. Coherent: one family, one package, carries the entire severity case, honours §4's *"source F is the keystone and must land first."*
- **81.2 `filter-stat-prefix-guards`** — A, B, C, G, H. Five uniform boot-time config-boundary guards.

§6.3 forecloses any third option: *"Either the work is in this phase and gets tested, or it is in a split sub-phase with its own row in the roadmap. There is no third option."*

---

## 13. WHAT THIS ROW NAMES BUT DOES NOT FIX

1. **The interior empty-segment hole.** `IsValidName("a..b") = true`. ⚠️ **REFINEMENT: the hole is INTERIOR-ONLY** — `a.`, `.a`, `..` and `""` are all already rejected, so the retrofit needs an *interior* predicate, materially cheaper than "well-formedness" implies. Phase 80 closed it for `sds.` only (`boot.go:279-290`). Surface = **14 incumbents + row 81's 9 new = 23 sites**; cost ~700-900 net.
   ⚠️ **AND A BLOCKER THE BRAINSTORM DOES NOT KNOW ABOUT: the retrofit CONTRADICTS ADR-0132 §Decision (v)** (§8). The successor must carve out the compressor or supersede that ADR.
   ⚠️ **AND ITS TARGET IS GENERATED, NOT AUTHORED** — a config-token sweep for `..` finds **zero** and is structurally blind; the successor must sweep **assembled** names.
   **Proposed successor slug: `stats-name-empty-segment-guards`.**
2. **The RBAC policy-name PROJECTION divergence** (§2.3). The reference hoists to `envoy_rbac_policy_name`; envoy-go flattens. **New deferred candidate**, invisible to the differential.
3. **The HTTP-rbac empty-prefix fallback divergence.** `rbac.go:500-506` `namespacePrefix` returns the literal `"rbac"` on an empty `rules_stat_prefix`, justified by a code comment asserting the C++ filter does the same. **Refuted by the reference itself**: my control-free arm set no `rules_stat_prefix` and the reference emitted `http.myhcm.rbac.policy.allow-admins.allowed` — a **single** `rbac` segment, where `baseStatPrefix` would make envoy-go emit `http.myhcm.rbac.rbac.policy.…`. The differential cannot see it: `0018/envoy.yaml` sets an explicit prefix at `:107 :127 :247` and its header says so deliberately. `reference_code_comment_not_evidence`. **Deferred candidate.**
4. **The matcher-tree `Action`-name boot walk** (§4.2), which would let F2 be boot-rejected.
5. **The three bare-prefix incumbents** (§3) — stronger, not weaker; nothing owed on the merits.
6. **The two-leg `invalidSecretNameErrFmt` wording** (§2.7).

---

## 14. HAZARDS CARRIED INTO THE PLAN

1. ⚠️ **Do not re-litigate D-81-HELPER.** The gate cost is symmetric; price it once for the row.
2. ⚠️ **Do not put F's guard in `internal/rbac`.** It is shared with a consumer that cannot panic.
3. ⚠️ **Do not ship (a) without (b).** F2 panics otherwise.
4. ⚠️ **Every guard is skip-if-empty.** Otherwise seven tests red and a landed ADR is contradicted.
5. ⚠️ **`tenant-foo` must be edited** or the ratelimit package reds — and no differential run will tell you.
6. ⚠️ **The full 120-fixture differential AND an explicit h2spec run are owed** (~400-430 s per green attempt; `-race` a SECOND run). `./test/differential/...` with `...` matches TWO packages and buffers `-v` output — a 7-minute silent log is normal.
7. ⚠️ **Line anchors drift; phrase and symbol anchors did not.** Every anchor in this SPEC was verified at `aab596e4`, and **this row's own edits will move them.** Re-derive at the PLAN tip; prefer the symbol anchors in §1 and §2.9.
8. ⚠️ **Known-live flakes**: `reference_sds_init_fetch_timeout_dial_budget_flake` (TWO packages) · `internal/cluster` `-race` outlier `TestOutlierDetector_ConcurrentEjectExactlyOnce` · `internal/httpclient TestOptions_ZeroValue_NoOpDefaults`. **NOT flakes any more — a recurrence is a FINDING**: `hcm/h2 TestServerConn_TinyWindowDelivery`, the full-suite `bind: address already in use` **backend** half, and `0061-lb-ring-hash` spread failures. **None fired at this SPEC** across the targeted differential subset, the nine source packages and `internal/stats`.

---

## 15. HYGIENE

**Docs-only.** ZERO production `.go`, ZERO test `.go`. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; `DECISIONS.md` gains ADR-0303 §Context only. Per-task `gofmt`/`golangci-lint` **not owed** for a docs-only stage.

**Six-gate (§7.5, `/BOOTSTRAP_PROMPT.md` at the REPO ROOT — `:357`, `:360-365`, `:367`, all re-verified exact):** (a)/(b)/(c)/(e) **NOT OWED** — no fixture and no `.go` touched. (d) **VACUOUS**, said to be vacuous rather than green — **55** fuzzers repo-wide, **0** added; NC `^func FuzzZZ` ⇒ 0. (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`, and none since 25.3; 84 of 121 phase dirs carry none.** Recorded as a departure, not as compliance.

**Probes.** Five agents wrote experimental `_test.go` files and prototype edits into their own detached worktrees; every one was reverted **by explicit path** with `sha256sum` byte-identity verified, and **all five reported `git status --porcelain` = 0 lines**. Three docker containers (`p81ctl-hyphen`, `p81ctl-underscore`, `p81ctl-control`) plus A2's four were removed **BY NAME** — never by an `ancestor=`/image filter (`reference_parallel_agents_shared_machine_namespaces`); `docker ps -a --filter name=p81ctl` ⇒ **0**.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED, OBSERVED** — `Shell cwd was reset to /home/esa/git/envoy-go` appeared on the controller side this session. The **twenty-sixth consecutive session** at which it has been live. Every git command used `git -C <abs-worktree-path>`; the branch was tripwired as `phase-81-spec`, never `master`.

**Broken-gate count stays EIGHTEEN** — no nineteenth shape at this stage, but **three priors fired live**: the `-run` no-match exiting 0 (§10), a negative control whose own input was wrong (§2.5), and a `grep -c` on zero matches exiting 1 while printing `0`.

**Counts re-derived at this tip**, each with a negative control observed: fixtures **120** (naive `^[0-9]{4}-` ⇒ 118; next-free **0119**) · fuzzers **55** · internal packages **73** · blank imports **120** · `ROADMAP.md` **229 / 113** · `BEHAVIOR_CONTRACT.md` **5868** · production `IsValidName` guard sites **15** · production stat-registration sites **210** grep-hits / **208** code.

**`DECISIONS.md` at this SPEC's close: 17724 → 17766 (+42), a PURE APPEND.** `^## ADR-` headings **301 → 302**, tail **ADR-0302 → ADR-0303**, next-free **ADR-0304**; one gap at **ADR-0209**, zero duplicates. STATUS blockquotes **15 → 16**, of which exactly **ONE** reads `PROPOSED` (this block; the other 15 are COMPLETE). Retained italic footers **8 → 9** (§2.10). ⚠️ **`^---$` stays 216, byte-identical to master** — no horizontal-rule separator was added, per the convention abandoned after ADR-0295. ADR-0303 carries `### Context` **only**; a scan for `^### (Decision|Consequences)` inside the block returns nothing, as it must at a SPEC.

⚠️ **THE FAMILY ORDINAL IS THE TWENTY-FOURTH, AND THE CONTROLLER'S OWN DRAFTING BRIEF SAID TWENTY-FIFTH.** Re-derived from `ROADMAP.md`'s own ordinal cells at `aab596e4`, mapped row → ordinal: row 79 **TWENTY-SECOND**, row 80 **TWENTY-THIRD**, row 81 **TWENTY-FOURTH** — one ordinal per row, no gap, no duplicate, and agreeing with what ADR-0301 and ADR-0302 each claim for themselves. `grep -c 'TWENTY-FIFTH'` ⇒ **0** (NC `TWENTY-FOURTH` ⇒ 1). The brief's figure rested on the premise that row 80 held the twenty-fourth, which **ADR-0302 itself does not claim**. Recorded because a wrong ordinal asserted in an ADR is exactly the species that acquires authority by being landed.

---

## 16. NEXT

**→ the phase-81 PLAN.** It must enumerate against §12's ~14 tasks and ~1000 net `.go`, hold D-81-DEPTH to table-driven-shared, and land F1 and F2 **atomically** — shipping the boot reject without the `Inc` backstop leaves the process crashable by the very config class the row exists to protect.
