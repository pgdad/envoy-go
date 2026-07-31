# BRAINSTORM 81 — stats-name-charset-guards

**Stage:** BRAINSTORM (lifecycle-state `DONE` → `1`). **Row 81 REGISTERED `in-progress`** at this commit per the `ROADMAP.md` §Schema invariant. Sentinel `want` **112 → 113** in this same commit. Base master **`4f8e159c`** (the phase-80 IMPL squash tip, located by SUBJECT via `git log --grep 'phase 80'`, never by position). Worktree `/home/esa/git/envoy-go-wt-phase81`, branch `phase-81-brainstorm`. Docs-only: **ZERO production `.go`, ZERO test `.go`**; `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED**.

**SELF-PICKED** per the 2026-07-12 standing directive (no human pick). The pick and every rejected alternative are recorded in §3 with costs **re-derived at this tip**, never inherited.

---

## 1. ⚠️ THE HEADLINE: A CONFIG-TRIGGERED, REQUEST-TIME, UN-RECOVERED PROCESS CRASH

**envoy-go boots green on an idiomatic Envoy RBAC config and then panics the process on the first matching request.**

`internal/rbac/perpolicy.go:21-33` lazily registers `<base>.policy.<policyName>.<suffix>` on first emission:

```go
func (s *PerPolicyCounters) Inc(reg *stats.Registry, base, policyName, suffix string) {
	if s == nil || reg == nil || policyName == "" {
		return
	}
	key := base + ".policy." + policyName + "." + suffix
	...
	c := reg.NewCounterIfAbsent(key)
```

`policyName` is checked for **emptiness only**. There is **no charset check anywhere in `internal/rbac`** — `git grep -n 'IsValidName' -- internal/rbac/` returns **no output**. And registration does not degrade: `internal/stats/registry.go:115-119` **panics**.

**Proved by execution at this tip, with a firing negative control** (probe written into the worktree, run, deleted; `git status --porcelain` = 0 lines afterwards):

```
=== RUN   TestZZPhase81_PerPolicyHyphenPanics
    NC OK: policy name "allow_admins" registered cleanly
    CONFIRMED request-time PANIC: stats: invalid metric name:
      "http.myhcm.rbac.policy.allow-admins.allowed"
      (must match ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$)
--- PASS
```

The NC arm is what makes this a result rather than an assertion: the **underscore** spelling registers cleanly, so the probe discriminates on the hyphen and nothing else (`reference_probe_must_discriminate`).

**Why this is severe rather than merely wrong:**

- `allow-admins` is **idiomatic Envoy**. Hyphenated RBAC policy names are the upstream norm; the reference accepts them.
- The gate is `track_per_rule_stats: true` plus one matching request — **not** a malformed config.
- **Nothing recovers it.** Production `recover()` lives in `internal/boot/boot.go` and the lua/wasm guest-VM wrappers only (`internal/lua/{vm,coroutine,doc}.go`, `internal/wasm/{root_vm,foreign,doc}.go`, `internal/wasm/abi/foreign.go`). There is **no `recover()` in `internal/filter/hcm/`, `internal/listener/`, or any filter factory**.
- **Boot-time validation cannot catch it** — the name does not exist until a request matches the policy.

⚠️ **This is a data-path availability defect in the proxy itself**, which is what separates it from every other candidate on the menu (§3). It is also the reason the standing directive's *"smallest defensible candidate first"* does **not** select here: that clause orders candidates of comparable defensibility, and there is no tie on severity.

---

## 2. THE SHAPE OF THE GAP — EIGHT UNGUARDED SOURCES, FIFTEEN GUARDED ONES

### 2.1 The validator, brute-forced rather than read

`internal/stats/registry.go:48` — `NamePattern = ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`, applied by `checkName` (panic) and `IsValidName` (read-only). Byte sweep over all 256 values:

| position | accepted set |
|---|---|
| first | `A-Z` `_` `a-z` |
| middle | `.` `0-9` `A-Z` `_` `a-z` |
| last | `0-9` `A-Z` `_` `a-z` |

Hyphen, space, `:`, non-ASCII and newline are rejected in **every** position. The hyphen is the live trigger because it is the one character that is simultaneously **idiomatic in Envoy config** and **banned by this regex**.

### 2.2 ⚠️ THE ROUTER'S "FIVE EXISTING REJECTS" IS REFUTED — THERE ARE FIFTEEN

Re-derived by the controller, not inherited: `git grep -n 'stats\.IsValidName' -- '*.go' ':!*_test.go'`, minus comment lines, minus `registry.go`'s own definition, minus the `test/helpers/` occurrences:

| # | token | guard site |
|---|---|---|
| 1 | cluster name | `internal/cluster/manager.go:419` |
| 2 | HCM `stat_prefix` | `internal/filter/hcm/config.go:267` |
| 3 | lua `stat_prefix` | `internal/filter/http/lua/compiled_config.go:365` |
| 4 | kafka `stat_prefix` | `internal/filter/network/kafkabroker/config.go:61` |
| 5 | redis `stat_prefix` | `internal/filter/network/redisproxy/config.go:50` |
| 6 | thrift `stat_prefix` | `internal/filter/network/thriftproxy/config.go:44` |
| 7 | **network** rbac `stat_prefix` | `internal/filter/network/rbac/rbac.go:106` |
| 8 | **network** rbac `shadow_rules_stat_prefix` | `internal/filter/network/rbac/rbac.go:115` |
| 9-11 | mongo wire cmd / collection / callsite | `internal/filter/network/mongoproxy/codec.go:477, :503, :517` |
| 12 | tracing HCM prefix | `internal/tracing/stats.go:29` |
| 13 | SDS secret (registration) | `internal/xds/stats.go:29` |
| 14 | SDS secret (boot boundary, phase 80) | `internal/boot/boot.go:286` |
| 15 | wasm **guest**-defined metric name | `internal/stats/dynamic/dynamic.go:353` |

The row-80 lineage documents repeatedly say *"the five existing rejects"*. **That figure is wrong by a factor of three**, and it matters: a phase that sets out to "complete five guards" would mis-scope its own retrofit surface.

### 2.3 ⚠️ THE ROUTER'S FAILURE-MODE IS ALSO WRONG

`next-prompt.txt` §THE PICK inherits `reference_nil_stats_counter_inc_crashes_goroutine` — *"nil `*stats.Counter` `Inc` = PROCESS CRASH"*. **Registration never returns nil here.** `checkName` panics directly (`registry.go:115-119`). Both crash the process, but the mechanism differs, and the difference is load-bearing for the fix: there is **no nil to defend against**, so the remedy is a boundary guard, never a nil-check.

### 2.4 The eight unguarded sources

Enumerated by the costing agent and spot-checked by the controller on the load-bearing one (§1):

| # | token | site | fires at |
|---|---|---|---|
| **F** | **RBAC policy name** | `internal/rbac/perpolicy.go:25` | ⚠️ **REQUEST TIME** |
| A | mongo `stat_prefix` | `internal/filter/network/mongoproxy/config.go:81` | boot |
| B | zookeeper `stat_prefix` | `internal/filter/network/zookeeperproxy/config.go:169` | boot |
| C | wasm `PluginConfig.name` | `internal/filter/http/wasm/compiled_config.go:1048` | boot **and request** (per-route) |
| D | **HTTP** rbac `rules_stat_prefix` | `internal/filter/http/rbac/rbac.go:224` | boot |
| E | **HTTP** rbac `shadow_rules_stat_prefix` | `internal/filter/http/rbac/rbac.go:225` | boot |
| G | `compressor_library.name` | `internal/filter/http/compressor/compressor.go:527` | boot |
| H | ratelimit `stat_prefix` | `internal/filter/http/ratelimit/compiled_config.go:467` | boot |

**D/E are an asymmetry, not an oversight.** The *network* rbac sibling validates **both** prefixes with assembled probes and its comment says so explicitly — *"otherwise invalid prefix would pass the != "" check but panic later"* (`internal/filter/network/rbac/rbac.go:106,115`). The **HTTP** sibling validates **neither**. One family, two behaviours, and the safe one wrote down why.

⚠️ **The router's `mongo collection names` hypothesis is REFUTED.** All three wire-derived mongo tokens are already guarded (`codec.go:477/503/517`), deliberately, with a comment naming the panic. Denominator stated so the zero is a measurement: mongoproxy has exactly **three** dynamic-arg stat calls against 21 fixed-roster `inc()` calls. **3 of 3 covered.** The router's `wasm plugin names` hypothesis is **CONFIRMED** (source C).

---

## 3. THE PICK, AND THE REJECTED ALTERNATIVES

**Four costing agents ran in parallel, each read-only in this worktree with private scratch.** Every cost below was re-derived at `4f8e159c`; none was inherited from `next-prompt.txt`, per `reference_deferred_candidate_cost_restale`.

| candidate | tasks | net `.go` | verdict |
|---|---|---|---|
| **⇒ charset guards (this row, SCOPED — §4)** | **8-11** | **~900-1400** | **PICKED** |
| charset guards, FULL (incl. empty-segment retrofit) | 12-16 | 1800-2600 | **scope-split**; retrofit deferred to a named successor |
| `--mode validate` SDS companion | 4-5 | ~+320 | **runner-up** — see below |
| differential bind-retry | 3 (+1 opt) | **−190** | rejected: test-harness only, no user-visible defect |
| `ROADMAP.md` rows 57/69/78 guard | 3-4 | ~120-160 | rejected: cosmetic render defect |
| `STATE_HISTORY.md` archive gap | 6-9 | ~230-285 + `ci.yml` | rejected: doc bookkeeping, and blocked (below) |
| WASM host family open (`wasm-host-shared-queue`) | 11-15 | ~1400-2000 | rejected **this round**: larger, and carries unresolved substrate risk |
| gRPC family open | 20-30 | 2500-4000 | **HARD-BLOCKED** (below) |

**Why not the runner-up.** `--mode validate` is a genuine, user-visible, cross-side-proven defect (§5.3) and sits on an already-enumerated Operational-tooling deferred list, so chartering it would **narrow** that list 3 → 2 rather than mint anything. It loses on one axis only: it degrades an **auxiliary validation tool**, while row 81 fixes an **availability defect in the data path**. It is the strongest single remaining candidate and should be picked next unless something larger surfaces.

**Why not the two check-(3) blockers.** Opening a family is the only way that family's work becomes enumerable at all — a prerequisite, not a shortcut — but neither is the smallest defensible move now:

- **gRPC is HARD-BLOCKED, and the block is deeper than recorded.** `\.RunEncodeTrailers(` and `\.RunDecodeTrailers(` are each **0 non-test / 1 test** (NC: `\.RunEncodeHeaders(` = **4** non-test). Trailers are destroyed at **six** independent seams; `router.ActionResponse` has no `Trailers` field at all; and `internal/filter/hcm/codec.go:74` `writeH1Reply` rewrites `Content-Length` unconditionally, so **H1 physically cannot emit trailers**. The one trailers-free charter item — `grpc-timeout` → route timeout — is doubly blocked: envoy-go has **no route-level timeout mechanism whatsoever**.
- **WASM is genuinely available** (§5.1 refutes the recorded decline) but is 11-15 tasks with an unresolved cross-VM-substrate question that must be settled before, not during.

---

## 4. SCOPE — WHAT THIS ROW DOES AND DOES NOT DO

**In scope:** close the **eight** unguarded sources of §2.4 at their config/registration boundaries, so that no config-derived or wire-derived token can reach stat registration unvalidated. Source **F** is the keystone and must land first; it is the only one that fires after boot.

**Explicitly OUT of scope, and deferred as a named successor row:** the **empty-segment hole**. All fifteen incumbent guards accept an interior `..` — `IsValidName("a..b") = true` — because `NamePattern` permits it. Phase 80 closed this for `sds.` **only**, at `internal/boot/boot.go:281-288`. The downstream damage is real and two-flavoured (malformed `envoy_x__y` names with a silently truncated label value, or the metric vanishing from `/stats/prometheus` entirely), but it is a **different defect with a different remedy**, and folding it in is what pushes the estimate from ~900-1400 to 1800-2600 — across the `~1500` split trigger.

Keeping the retrofit out is what keeps this row atomic. **Recording it here is what stops it being lost.**

### 4.1 The open design questions for the SPEC

- **D-81-HELPER — the one load-bearing fork.** Shared helper in `internal/stats` vs. inlining the boundary check per package. The project's own *"one definition, no drift"* doctrine (`internal/stats/dynamic/dynamic.go:40-42`) pulls hard toward a shared helper — but **a shared helper touches `internal/stats`, which makes the full 120-fixture differential AND an explicit h2spec run mandatory** (~400-430 s per green attempt, `-race` a second run). Inlining keeps the gate per-package. **This must be decided at the SPEC, with the gate cost priced in, not discovered at the IMPL.**
- **D-81-WORDING.** Eight new reject messages across six packages. The incumbent wordings are not uniform, and two existing SDS messages are actively misleading (§5.3). Byte-stable wordings should be pinned at the SPEC.
- **D-81-F-SITE.** Source F is a **request-time** path, so a reject cannot be returned to a caller — the guard must move to **config-compile time** (`internal/rbac/rbac.go`, where policies are compiled) and reject the policy name at boot, or the counter emission must degrade to a skip. These are observably different behaviours; pick one deliberately.
- **D-81-SANITIZE.** ⚠️ **A doctrinal contradiction that will otherwise be re-litigated mid-row.** Phase 80 records *"Sanitizing is FORECLOSED by ADR-0065's own Context"* (`internal/boot/boot.go:264-266`) — yet `internal/listener/manager.go:352` `normalizeAddr` **sanitizes** the listener address (`:`→`_`, `.`→`_`). Both are landed. The SPEC must reconcile them.

### 4.2 Anticipated surface

**Zero new differential fixtures.** These are envoy-go-strict rejects where the **reference accepts** — a cross-side boot-reject dir is asymmetric by construction, and the in-tree precedent is explicit: `BEHAVIOR_CONTRACT.md:931` — *"All boot-rejects are SUBJECT unit tests (no fixture dirs)"*. Budget unit tests.

**No existing fixture should red.** All 111 distinct fixture-YAML `stat_prefix` values are `[A-Za-z0-9_.]`-clean with no `..`; wasm plugin names are `plugin_a`/`plugin_bootreject`/`plugin_listener_default`; `compressor_library.name` is `text_optimized`; RBAC policy keys are `admin_users`/`p_allow`/`public_paths`. ⚠️ **Anticipated, not proven** — the differential was not run at this stage.

**Stat-surface delta +0** (guards register no names). **+0 fuzzers, +0 packages, +0 go.mod modules, +0 BackendKind.** ADRs: **1 anticipated (ADR-0303)**, §Context at the SPEC, §Decision/§Consequences at the IMPL per ADR-0044-as-used.

### 4.3 Cost and split posture

**8-11 tasks, ~900-1400 net `.go`.** Both §6.1 triggers are clear (`~25` tasks, `~1500` net lines) — **but the margin is ~1.1-1.7×, which is thin.** Calibration says read a budget as *"expect the ceiling"*: 76 `~7-9 → 9` · 77 `11-13 → 12` · 78 `7-9 → 10` · 79 `10-12 → 12` · 80 `11-14 → 13`. And phase 80 estimated **~640 net** and realized **1636** — **2.6×**, every bucket over, errors compounding. ⚠️ **If the SPEC's own estimate lands above ~1200, the PLAN should split 81.1 (sources) / 81.2 (retrofit) rather than absorb it.** The third §6.1 trigger (mid-execution, >~10 sub-steps) fired at phase 80 and must be **recorded if it fires again, never absorbed**.

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE FOUND BY EXECUTION

*A BRAINSTORM that refutes nothing has not looked.* Load-bearing items first.

### 5.1 ⚠️ `ROADMAP.md:76` DOES NOT DECLINE A WASM ROW — THE DECLINE IS A CATEGORY ERROR

`next-prompt.txt` records the WASM rider as *"DECLINED ON THE MERITS"* because `ROADMAP.md:76` (row 25.3) says verbatim *"phase 25 is the FINAL §9 HTTP-filters-family row"*. **That sentence is real and quoted correctly — and it does not say what the decline needs it to say.** `ROADMAP.md:218` is a **separate heading in its own right**: `### WASM host family` — *"Own multi-phase sub-project: ABI, engine binding, proxy-wasm conformance."* A WASM-**host**-family row is not an HTTP-filters-family row. `:76` forecloses a 26th HTTP-filters row; it is silent on the WASM host family.

What `:76` **does** correctly kill is the *rider* — bolting a `WASM-family row` marker onto an existing HTTP-filters row, which would silence check (3) **by mention**. A standalone WASM-host row is untouched by it. The family also carries **five landed, ADR-anchored forward-pointers naming it by name** (`phases/25-http-filter-wasm/SPEC.md:183`, `:750`; `BEHAVIOR_CONTRACT.md:4255`; `phases/25.3-…/SPEC.md:25`; plus two `BEHAVIOR_CONTRACT.md` section headers) — the same accumulated-forward-pointer profile that justified opening Runtime at phase 77.

⇒ **WASM is a live, defensible candidate whenever it is the smallest one.** The recorded decline should not be re-inherited.

### 5.2 ⚠️ THE BIND-RETRY FIX AS PRESCRIBED CANNOT WORK

`next-prompt.txt` says *"the fix is a `startSubjectWithRetry` equivalent."* **A literal equivalent would not catch the failure.** All 26 backend arms spawn `exec.CommandContext(ctx, "go", "run", …)`, so `cmd.Start()` starts the **go toolchain** and returns `nil` on a port collision; the collision surfaces later, when the compiled child calls `Listen`. `startSubjectWithRetry` keys on the *start error* — of which there is none. A backend retry must key on **readiness failure or child exit**.

Three further corrections in the same area:

- **The window was measured, not assumed: ~150 ms warm, 3.3 s cold**, and the CI signature was reproduced byte-exact by squatting an in-band port inside the build window.
- ⚠️ **`-count=6` is not a gate.** ~40 minutes to test for the *absence* of a rare event; at the observed rate six clean runs occur with p ≈ 0.53 **even if the bug is untouched**. A deterministic squat-arm plus a firing NC costs ~7 minutes and actually discriminates.
- ⚠️ **"close-then-rebind" is the CURRENT characterization, not a stale one** — `next-prompt.txt` calls it stale, but the landed `f2dd994a` helper comment uses exactly that wording. What *is* stale is the ephemeral-range/loopback-probe framing.
- **`harness_test.go`'s "4 more" call sites contain ZERO backend arms** (`:143`/`:144` are a unit test, `:315` is the internal fallback, `:342` is the pinning test). True as a grep count, false as a statement of work.

### 5.3 ⚠️ THE `--mode validate` DEFECT RUNS IN THE OPPOSITE DIRECTION

`next-prompt.txt` frames it as a validate that green-lights a bootstrap that would fail at boot. **It is the exact inverse.** Measured against the pinned reference image `envoyproxy/envoy:contrib-v1.37.2`, no SDS server running:

| fixture | envoy-go | reference |
|---|---|---|
| `0103` `tls_certificate_sds_secret_configs` | `EXIT=1` — *"requires a live SDS provider"* | `configuration OK`, `EXIT=0` |
| `0108` `validation_context_sds_secret_config` | `EXIT=1` — *"is not supported in phase 03"* | `EXIT=0` |
| `0109` `combined_validation_context` | `EXIT=1` — *"is not supported in phase 03"* | `EXIT=0` |
| control: non-SDS `0005` | `configuration OK`, `EXIT=0` | — |

**3/3 cross-side divergence.** `validate` **false-rejects** configs that demonstrably boot — 0103/0108/0109 are live, passing differential fixtures. Two of the three messages say *"is not supported in phase 03"*, telling a Gateway-API integrator that envoy-go cannot do SDS validation contexts **at all**, when phases 65/66 landed exactly that support. `internal/boot/boot.go:165-167` already calls this wording *"misleading-but-safe"*. The premise is confirmed (`validate/validate.go:48-49` passes a literal `nil` provider) but the **consequence was recorded backwards**.

### 5.4 ⚠️ THE ARCHIVE-GAP FIGURE IS 57, NOT 58 — AND THE OBVIOUS GUARD SELF-CLEARS

- **57 missing bullets, not 58** (`comm -13`, join key `(phase-id, STAGE)` with the slug discarded). Two extractor traps had to be cleared first: **slug drift** (git says `phase 42.1 (retries)`, the archive says `(retries-and-hedging)` — 31 spurious misses) and **two distinct archive bullet shapes** (`STATE_HISTORY.md:428/:430` merge the eviction annotation and the bullet onto one line, which a naive extractor misses — inflating the count to 59). The **"36 for phases 67-75" figure is CONFIRMED exactly**. `phase 77 PLAN done` is confirmed absent from both files, NC-discriminating 9/9. **57 is a lower bound project-wide** — 87 older subjects use a pre-slug convention invisible to the canonical extractor.
- ⚠️ **`reference_sentinel_matcher_string_self_clears`, reproduced in a NEW file.** Drop the `(slug)` parens and `grep 'phase 77 PLAN done'` matches **`STATE.md`'s own confession prose** at `:18` and `:20` — the sentence documenting the gap satisfies the gate that checks for it. Any such guard **must** require the parens.
- ⚠️ **CI is a shallow clone.** `.github/workflows/ci.yml` uses `actions/checkout@v6` with no `fetch-depth` ⇒ depth 1, so a `git log`-derived expectation set returns **one commit** in CI. The git-free alternative was tested and **measurably fails** (deriving from `phases/*/` on disk yields **380** stages vs git's **209**, because split legs share a parent directory).

### 5.5 ⚠️ THE `NF==8` GATE IS WORSE THAN RECORDED

`next-prompt.txt` records that naive `NF==8` **passes** malformed row 78 (compensating defects). Re-derived here over all 112 rows — **it also throws 15 FALSE POSITIVES** (rows `16 17 18 18.1 19 20 22.1 24.2 25.1 43 48 49 55 57 62 69 74`, whose summaries legitimately carry escaped `\|`). It is not merely blind; it is unusable in both directions.

The disjunction was re-confirmed independently by the controller over the live file:

```
ARM-A: row 57 (NF-2=7)     ARM-A: row 69 (NF-2=8)     ARM-B: row 78
```

**ARM-A catches 57/69 only; ARM-B catches 78 only.** Reproduced again on a *synthetic* row 81 given both defects: **ARM-B fires, ARM-A stays silent, naive `NF==8` PASSES** — `reference_compensating_defects_cancel_in_the_gate_metric`, on a row this stage authored.

### 5.6 A LANDED MEMORY OVERGENERALIZED — `grep -c` IS TOOL-SPECIFIC

`reference_grep_c_zero_is_a_broken_gate` claimed `git grep -c` *"(and GNU `grep -c` on a file list)"* prints nothing on zero matches. **The parenthetical is FALSE.** Measured across six forms:

| form | zero-match stdout | exit |
|---|---|---|
| `git grep -c NEEDLE -- f1` / `-- f1 f2` | *(empty)* | 1 |
| GNU `grep -c NEEDLE f1` | `0` | 1 |
| GNU `grep -c NEEDLE f1 f2` | `f1:0`, `f2:0` | 1 |
| GNU `grep -rc NEEDLE dir/` | `path:0` for **every** file | 1 |
| ugrep-wrapped `grep -c NEEDLE f1` | `0` | 1 |

Only `git grep -c` is silent; GNU/ugrep always print a number. Both still exit **1**, so an exit-code gate is wrong for either. **The memory has been corrected.** A `git grep`-specific hazard asserted of GNU `grep` is itself a drift (`reference_a_drift_correction_is_itself_a_claim`).

### 5.7 SMALLER FINDINGS, RECORDED SO THEY ARE NOT RE-DERIVED

- **Production-dead trailer code.** Four filters ship non-trivial `EncodeTrailers`/`DecodeTrailers` bodies that can never run: `admission_control/encode.go:165` (its **entire deferred gRPC-status state machine** is unreachable), `extproc/extproc.go:1089,1398`, `lua/lua.go:548,570`, `wasm/trailers.go:84,160` (8.5 KB). Guest hooks `proxy_on_request_trailers`/`proxy_on_response_trailers` are registered but never dispatched.
- **A vacuous differential arm.** `test/fixtures/0036-http-wasm-body-and-advanced/expectations.yaml:39-45`, arm `e_trailers_read`, asserts `x-trailer-count=0` on a trailer-less GET. It cannot fail.
- **Two stale non-test comments.** `internal/filter/hcm/connection.go:565` and `h2dispatch.go:503` both say *"the FilterChain does not yet expose a RunDecodeTrailers method (Task 18 will add it)"*. It does — `internal/filter/http/chain.go:455`. `reference_code_comment_not_evidence`.
- **A DECISIONS roster that reads as a claim about landed code.** `DECISIONS.md:13179` enumerates 24 module-function getter keys including `proxy_validate_configuration`, `proxy_on_queue_ready`, the four gRPC receive callbacks and the two metadata callbacks — **none of which exist in `internal/`**. `internal/wasm/sandbox.go` carries only the 16 implemented `capProxyOn*` constants.
- **Two sibling `freeTCPPort` helpers still carry both original defects** — `cmd/envoy-go/main_test.go:180` and `test/conformance/h2spec/h2spec_test.go:219` are still verbatim `net.Listen("tcp","127.0.0.1:0")` (loopback probe, ephemeral range). Band-disjoint from the differential suite, so they cannot collide with it, but the defect is intact in their own scope.
- **There is no doc-invariant test anywhere in this repo.** The five `_test.go` files mentioning these documents mention them in **comments only**. The one reusable precedent is `test/differential/harness_test.go:123-137`'s `runtime.Caller(0)` → repoRoot idiom.

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE. IT DOES **NOT** FIRE

Run by the controller in this worktree at `4f8e159c`, **before** any edit. `stop` was **NOT** created; `ls stop` ⇒ `No such file or directory`.

| check | ACTUAL output | NC, observed FIRING |
|---|---|---|
| **(1)** `want=112` | **NOTHING** — every chartered row is `done` | row **62** doctored on a scratch copy ⇒ `NC NOT DONE: row 62` (script self-reported `rows doctored: 1`); `want=111` ⇒ `NC GATE FAIL: examined 112 data rows, expected 111` |
| **(2)** | **FIVE** — `:190 :200 :210 :216 :224` (PRE-EDIT; ⚠️ **this row's own insertion shifted them to `:191 :201 :211 :217 :225` — the line-anchor-drift hazard firing on this stage's own document, caught only by re-running the gate on the FINAL tree**) | one-arm strip moves the union **5 → 4, NOT 5 → 0** |
| **(3)** | **`NEVER OPENED: gRPC`**, **`NEVER OPENED: WASM`** | invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`; registered `Observability` correctly silent |

Input measured at **228 lines / 112 data rows / 13** bare `candidates:` hits, so an empty result could not read as a zero result (`reference_empty_output_is_not_a_zero_result`).

⚠️ **Check (1) is silent, so a doctored-copy NC is the only thing that distinguishes it from a broken check. One was run, and it fired.**

⚠️ **CHECK (2) IS UNCHANGED AT FIVE. THIS ROW NARROWS NOTHING — STATED, NOT FORECAST.** The **twenty-eighth** consecutive phase at which it did not go down. The charset candidate does not sit on any family's deferred-candidate sentence, so chartering it removes nothing from one. (Had the runner-up been picked, it would have narrowed the Operational-tooling list 3 → 2 — while still leaving the sentence, and therefore the count, at five.)

⚠️ **`want` MOVES 112 → 113 IN THIS COMMIT**, because this stage charters row 81. The only live occurrence is `next-prompt.txt:17`; every other `want=112` in the tree is historical phase-document prose.

### 6.1 The leak check — row 81's cell only

**NEVER WRITE A SENTINEL'S OWN MATCHER STRING INTO A FILE THE SENTINEL GREPS.** The sentinel greps `docs/envoy-go/ROADMAP.md`. Row 81's cell carries **0** occurrences of either deferred-candidate phrase, and exactly one family slug — **`Observability-family row`**, which is **already registered 52×** elsewhere in the file, so it is a *use*, not a *mention*. Both greps were run against the row's cell in isolation, and the well-formedness disjunction was run over the resulting 113-row file.

---

## 7. COUNTS RE-DERIVED AT THIS TIP

Re-run mechanically in this worktree; **not** copied from `next-prompt.txt`. GNU `command grep` / `git grep` used for anything repo-wide, per `reference_recursive_grep_blind_to_gitignored_tracked_file`.

| figure | value | note |
|---|---|---|
| `ROADMAP.md` | **228** lines / **112** data rows | → 229 / 113 at this commit |
| bare `candidates:` hits | **13** | vs the sentinel's narrower 5 |
| `DECISIONS.md` | **17724** lines, **301** `^## ADR-` headings | tail **ADR-0302 COMPLETE**, next-free **ADR-0303** (`^## ADR-0303` ⇒ **0**; NC `^## ADR-0302` ⇒ **1**) |
| `BEHAVIOR_CONTRACT.md` | **5868** lines | BYTE-UNTOUCHED this stage |
| `STATE_HISTORY.md` | **430** lines | → 432 at this commit (one eviction) |
| fixtures | **120** | faithful predicate `^[0-9]{4}[a-z]?-`; a bare `^[0-9]{4}-` gives **118**. Next-free **0119** (tail dir `0118-runtime-static-layer`) |
| fuzzers | **55** | `^func Fuzz`, repo-wide |
| internal packages | **73** | |
| blank imports in `runner_test.go` | **120** | anchored on the FULL `^\t_ "github.com/pgdad/envoy-go/test/fixtures/` prefix, via `grep -P` (`\t` in GNU ERE is a literal `t`) |
| production `IsValidName` guard sites | **15** | ⚠️ **not 5** — §2.2 |
| production stat-registration call sites | **210** of **508** tree-wide | 11 pure-literal, 199 non-literal, fanning out from ~19 distinct token sources |

**Normative cites re-verified EXACT** in `/BOOTSTRAP_PROMPT.md` **at the repo root**: `### 6.1 When to split` at **`:285`** (`:289` the ~25-task trigger, `:290` the ~1500-LoC trigger, **`:291` blank**, `:292` the third trigger); §7.5 heading **`:357`**, gates (a)-(f) **`:360-365`**, close **`:367`**.

---

## 8. HYGIENE

**Docs-only.** ZERO production `.go`, ZERO test `.go`. `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED** — a BRAINSTORM authors no ADR (ADR-0044-as-used: §Context lands at the SPEC, §Decision/§Consequences at the IMPL). Per-task `gofmt`/`golangci-lint` **not owed** for a docs-only stage.

The §1 probe was written into `internal/rbac/`, executed, and **deleted**; `git status --porcelain` reported **0 lines** afterwards, verified before any commit.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED AGAIN, OBSERVED** — `Shell cwd was reset to /home/esa/git/envoy-go` appeared **three times** this session. The **twenty-fifth consecutive session** at which it has been live. Every git command used `git -C <abs-worktree-path>`; the branch was tripwired as `phase-81-brainstorm`, never `master`.

**Six-gate (§7.5) posture for a docs-only BRAINSTORM:** (a)/(b) **not owed** — no fixture or code change; (c) **not owed**; (d) **VACUOUS** — 55 fuzzers repo-wide, **0** added (said to be vacuous, not green); (e) **not owed** — no `.go` touched; (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`, and none since 25.3; 84 of 121 phase dirs carry none.** Recorded as a departure, not as compliance.

---

## 9. NEXT

**→ the phase-81 SPEC.** It must dispose D-81-HELPER (§4.1) **first**, because that single fork decides whether the full 120-fixture differential plus an explicit h2spec run become mandatory gates for every later stage of this row.
