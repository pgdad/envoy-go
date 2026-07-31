# PLAN 81 — the nine charset guards: **the bare-vs-assembled question is decided by SEGMENT POSITION, and the SPEC measured it at the one position where the answer is degenerate** — plus D-81-EMPTY's premise collapses, the shared table is not constructible, and the row is SMALLER than its own band floor

**Stage:** PLAN (lifecycle-state `2` → `3`). **Row 81 STAYS `in-progress`**; `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` are **BYTE-UNTOUCHED**; sentinel `want` STAYS **113**. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master **`82e425ac`** — taken from `git rev-parse master` at session start. ⚠️ **At this tip the SPEC squash IS the master tip**, so the usual "the router's quoted SHA sits below the tip" hazard did not bite this session; it was still re-derived rather than assumed. Worktree `/home/esa/git/envoy-go-wt/phase-81-plan`, branch `phase-81-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (`# PLAN record` appended) + `STATE.md` + `STATE_HISTORY.md` + `next-prompt.txt` — **five files**, matching the phase-79 and phase-80 PLAN precedent for the same reason (§Recent at its five-entry cap with an unarchived evictee).

## What was EXECUTED at this stage

**Five investigation agents on disjoint remits**, each in its own **DETACHED** worktree with private scratch and a private port band inside `42000-42499`, plus controller re-derivation of every load-bearing claim (`feedback_brief_citations_not_evidence`).

| agent | remit | headline |
|---|---|---|
| **A1** | sources A, B, H + the incumbent-template census | the assembled template makes **mongo strictly weaker than its three bare-guarded siblings** |
| **A2** | **the keystone** — F1 + F2 | **§2.8's "OVER-REJECTS = 1" does not transfer to F**; the SPEC's pasted panic name is the **reference's** |
| **A3** | sources C, D, E, G | **D-81-EMPTY's premise collapses**; four of §8's seven cited tests never reach the production entry point |
| **A4** | gates, blast radius, the full differential | a **SECOND corpus red**; the differential **aborts** and the notification said success |
| **A5** | counts, ADR state, bookkeeping | **ADR-0045's "mis-location" is REFUTED**; `84 of 121` is stale |

**Zero commits, zero pushes, zero branches by any agent.** Every experimental edit reverted **by explicit path**; all five reported `git status --porcelain` = **0 lines**, controller-re-confirmed. No docker container was created by any agent; teardown, where used, was **BY NAME** (`reference_parallel_agents_shared_machine_namespaces`).

---

## 1. PLAN re-derivation ledger — what this stage REFUTED

*A PLAN that refutes nothing has not looked.* Load-bearing first.

### 1.1 ⚠️ HEADLINE — THE BARE-VS-ASSEMBLED QUESTION IS DECIDED BY **SEGMENT POSITION**, AND §2.8 MEASURED IT AT THE DEGENERATE POSITION

SPEC §2.8 measures the incumbent `network/rbac` template and concludes **"MIS-ACCEPTS = 0, OVER-REJECTS = 1"**, then §3 promotes that template row-wide. **Three agents hit the same wall independently, on three different sources.** Controller re-derivation, executed inside `internal/stats`:

| token | bare | **LEADING** segment (`<tok>.zookeeper.decoder_error`) | **INTERIOR** segment (`http.myhcm.rbac.rbac.policy.<tok>.allowed`) |
|---|---|---|---|
| `allow-admins` | false | false | false |
| `0policy` | false | false | **true** |
| `policy.` | false | **true** | **true** |
| `.policy` | false | false | **true** |
| `9` | false | false | **true** |
| `""` | false | false | **true** |
| `a..b` / `a.b.c` / `ok` | true | true | true |
| **disagreements / 9** | — | **1** | **5** |

**`network/rbac`'s variable segment LEADS the assembled name.** At that position bare and assembled differ on exactly one shape — which is precisely §2.8's "OVER-REJECTS = 1". **At an INTERIOR position they differ on five**, because a fixed prefix to the left legalises the whole leading-digit / leading-dot / trailing-dot / empty class.

⚠️ **EIGHT OF THE NINE SOURCES ARE INTERIOR.** Only **B** (zookeeper, `<sp>.zookeeper.<leaf>`) is leading. A/C/D/E/F1/F2/G/H all carry a fixed prefix to the left. **The SPEC generalised from the single unrepresentative incumbent.**

Per-source measurement, each by a different agent:

- **A2, source F1** — 19-token cross-product, base `http.myhcm.rbac.rbac`: **5/19 disagreements** (`0policy`, `policy.`, `.policy`, `9`, `""`). A bare F1 would **boot-reject 4 non-empty config shapes that today boot, serve, and register a perfectly valid counter**, and would make **F1 and F2 disagree** — the same policy name rejected via the rules engine and silently accepted via the matcher engine.
- **A1, source A** — `IsValidName("1abc") = false` but `IsValidName("mongo.1abc.op_query") = true`. Mongo, thrift, kafka and redis share the identical `"<literal>." + sp + "."` shape; **thrift/kafka/redis guard the BARE prefix and therefore reject `1abc`, while an assembled mongo accepts it.**
- **A3, sources C/D/E/G** — its first-draft reject arms for `trailing.`, `text.`, `row81_bad.` **all went RED**: the assembled probe accepts them.

**DECISION — ASSEMBLED AT ALL NINE SOURCES**, per ADR-0065 §Consequences (b) (`DECISIONS.md:2379`, re-verified verbatim by A5). Rationale, in order:

1. **Panic-safety is the row's purpose**, and the assembled name is exactly what `checkName` inspects. A bare probe at an interior position rejects configs that **cannot panic** — a regression in an availability row.
2. **F1/F2 must agree.** F2 has no boot-time enumeration, so its backstop can only probe the assembled key; a bare F1 would diverge from it.
3. ADR-0065 §Consequences (b) states the assembled rule *with its argument*; §Consequences (e) (`:2382`) is the standing obligation on filter authors.

⚠️ **TWO CONSEQUENCES, RECORDED AS DELIBERATE RATHER THAN DISCOVERED AT THE IMPL:**

- **The nine guards inherit the interior-empty-segment hole BY CONSTRUCTION** (`IsValidName("a..b") = true`, controller-executed). SPEC §13.1 predicted this; it is now **measured on four sources**. A3 converted its failing reject-arms into **ACCEPT-pins naming §13.1**, so the successor row inherits a failing-first anchor instead of a silent hole. **The PLAN adopts that shape — see T9.**
- **Source A ends up more permissive than its three bare-guarded siblings** (thrift/kafka/redis) on a leading-digit prefix. **Named, not fixed** — closing it means changing three landed guards, which is out of row 81's remit.

### 1.2 ⚠️ D-81-EMPTY'S PREMISE COLLAPSES — SKIP-IF-EMPTY IS A SEMANTIC NO-OP FOR C, D, E AND G

SPEC §8 argues an unconditional guard reds **seven** tests. **Measured by A3, three-arm cross-product, all executed:**

| arm | wasm | rbac | compressor |
|---|---|---|---|
| unconditional **BARE** | `ok` (0 red) | **FAIL — 26 top-level** | `ok` (0 red) |
| unconditional **ASSEMBLED** | `ok` | `ok` | `ok` |
| **skip-if-empty ASSEMBLED** (§8) | `ok` | `ok` | `ok` |

The reason is §1.1's: under the assembled template an empty token yields a **valid** name at every one of these sources —
`IsValidName("wasm..http_call_dispatch_unknown_cluster") = true`, `IsValidName("compressor..gzip.<leaf>") = true` (controller-executed), and **D/E never see an empty segment at all** because `namespacePrefix("")` substitutes the literal `"rbac"` (controller-verified at `rbac.go:507-512`, whose own comment ties the fallback to the reference's `generateStats`).

⇒ **§8 reasoned about a BARE token and applied the conclusion to an ASSEMBLED template.**

⚠️ **AND FOUR OF §8's SEVEN CITED TESTS CANNOT BE REDDENED BY ANY GUARD AT A PRODUCTION ENTRY POINT** — they bypass it:

| §8 row | what it actually calls |
|---|---|
| `TestNew_LibraryName_EmptyAllowed` `compressor_test.go:291` | **`buildFromAny` (`:1061`)** — a test-local re-implementation of `New`'s body. Never calls `New`. |
| `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` `:2376` | **`newFilterStats(reg,"ingress_p14","")` directly.** Never calls `New`. |
| `TestBuildCompiledConfig_Arm26_EmptyName_SkipsRegistry` `:1239` | **`registerPluginConfigName("")`.** Never calls `buildCompiledConfig`, despite its name. |
| `TestUnregisterPluginConfigName_EmptyName_NoOp` `:1273` | **`unregisterPluginConfigName("")`.** Same. |

⚠️ **AND §8's H row is MISCITED.** It claims *"2 `ratelimitfilterv3.RateLimit{}` literals"* in `ratelimit/{compiled_config,fuzz}_test.go`. Controller-verified: **exactly ONE** bare `RateLimit{}` exists, at **`fuzz_test.go:380`**; `compiled_config_test.go` has two `RateLimit{` literals but **both are populated**. The real empty-`StatPrefix` carrier is `validRateLimitConfig()` at `compiled_config_test.go:53`, called **40** times. **The property is real; the citation is wrong.**

**DISPOSITION: KEEP skip-if-empty**, because it is the landed incumbent shape (`lua/compiled_config.go:363`, `network/rbac/rbac.go:114`), it is free, and it makes intent legible. ⚠️ **But the PLAN does NOT carry it as load-bearing, and the IMPL must NOT repeat "an unconditional guard reds seven tests."**

⚠️ **SEPARATELY, §8's universal quantifier is wrong for A and B.** Both already `return errors.New(errStatPrefixRequired)` on empty *before* any guard can run (`mongoproxy/config.go:81`, `zookeeperproxy/config.go:169`), so the `tok != ""` arm there is **dead code** — and the redis/thrift/kafka incumbents omit it for the same reason. **A1 shipped A and B without it.** T1/T2 pin the empty arm to `errStatPrefixRequired`, **not** to the new charset error.

### 1.3 ⚠️ THE SPEC'S PASTED PANIC NAME IS THE **REFERENCE'S**, NOT envoy-go's

SPEC §2.1 pastes `"http.myhcm.rbac.policy.allow-admins.allowed"` — **one** `rbac` segment. A2 drove the real path (`New()` → `factory()` → `DecodeHeaders()`) and measured:

```
*** REQUEST-TIME PANIC: stats: invalid metric name:
    "http.myhcm.rbac.rbac.policy.allow-admins.allowed"
```

**Two** `rbac` segments, because `namespacePrefix("")` substitutes the literal after `baseStatPrefix` already emitted one. **The severity claim survives intact; the quoted string does not** — and this **independently re-proves SPEC §13 item 3** (the empty-prefix fallback divergence) **by execution**, upgrading it from a reference-probe inference to a measured subject-side fact.

⚠️ **Any IMPL test that pins the panic string must use the DOUBLE form.** Pinning the SPEC's string would fail.

### 1.4 ⚠️ THE HEADLINE CRASH RE-PROVEN END-TO-END, WITH A FOUR-ARM CROSS-PRODUCT

A2 did not re-run the SPEC's direct-call probe; it drove the **real** boot-then-request path and varied **two** axes (`reference_probe_must_discriminate`):

```
F2 matcher allow-admins track=true   BOOT ACCEPTED -> *** REQUEST-TIME PANIC
F2 matcher allow_admins track=true   BOOT ACCEPTED -> no panic; stats=[...allow_admins.allowed]
F2 matcher allow-admins track=false  BOOT ACCEPTED -> no panic; stats=[]
F2 matcher allow_admins track=false  BOOT ACCEPTED -> no panic; stats=[]
F1 rules   allow-admins track=true   BOOT ACCEPTED -> *** REQUEST-TIME PANIC
F1 rules   allow_admins track=true   BOOT ACCEPTED -> no panic
F1 rules   allow-admins track=false  BOOT ACCEPTED -> no panic
F1 rules   allow_admins track=false  BOOT ACCEPTED -> no panic
```

**Both NC axes fire independently** (charset AND the `track_per_rule_stats` gate), so the probe discriminates rather than merely agreeing. **Un-recovered confirmed:** 20 `recover()` hits tree-wide, **all** in `internal/lua`, `internal/wasm` VM wrappers or comments — **none on the HCM/HTTP-filter dispatch path.**

### 1.5 ⚠️ "TABLE-DRIVEN-SHARED, WRITTEN ONCE OVER ALL NINE SOURCES" IS **NOT CONSTRUCTIBLE**

SPEC §7 mandates that *"the byte-sweep and longest-suffix audits are written **once**, table-driven over all nine sources."* **It cannot be.** Controller-verified: five of the six guard targets are **package-private**, spread across **five distinct packages** —

```
buildCompiledConfigImpl   internal/filter/http/wasm            (private)
buildCompiledConfig       internal/filter/http/rbac            (private)
buildCompiledConfig       internal/filter/http/ratelimit       (private)
parseConfig               internal/filter/network/mongoproxy   (private)
parseConfig               internal/filter/network/zookeeperproxy (private)
New                       internal/filter/http/compressor      (exported)
```

A Go test can share a table only **within** a package. A2 adds the same finding for the keystone: **F1's table lives in `internal/filter/http/rbac` and F2's in `internal/rbac`** — different packages, so even the keystone pays for the charset sweep **twice**.

⇒ **The shared audit is ~7 near-identical per-package drivers, not one table.** The *saving* D-81-DEPTH buys is the driver loop and the shared arm roster, **not** the config construction. **The disposition still stands** (it is still far cheaper than the phase-80 per-source-deep shape) — but **T9 budgets it as seven drivers**, and the IMPL must not go looking for a single-file table that cannot exist.

### 1.6 ⚠️ THE COST IS **MEASURED, NOT MODELLED — AND THE ROW IS SMALLER THAN ITS OWN BAND FLOOR**

Every figure below was produced by **building the guard and its tests, running them, and reading `git diff --numstat`** — not estimated.

| src | guard `.go` | test `.go` | **net** | agent |
|---|---|---|---|---|
| **A** mongo | 12 | 47 | **59** | A1 |
| **B** zookeeper | 12 | 47 | **59** | A1 |
| **H** ratelimit | 18 | 54 (+3/−3 corpus repair) | **72** | A1 |
| **C** wasm | 12 | 87 | **99** | A3 |
| **D** rbac rules-prefix | 9 | 29 | **38** | A3 |
| **E** rbac shadow-prefix | 9 | 29 | **38** | A3 |
| **G** compressor | 11 | 75 | **86** | A3 |
| **F1** rbac policy name | 60 | 118 | **178** | A2 |
| **F2** `Inc` backstop | 22 | 74 | **96** | A2 |
| **MEASURED TOTAL** | **165** | **560** | **725** | — |

**Reconciliation against SPEC §12, which said `F1 174 + F2 50 + 7×75 + shared 140 ≈ 890`, band 850-1200:**

- **F1 = 178. SPEC's 174 CONFIRMED** (+2.3%).
- **F2 = 96. SPEC's "~50" REFUTED at 1.92×** — the miss is the aggregated-log-dedupe test (needs a log-sink capture harness) plus the second charset table §1.5 forces.
- **The "~75 per non-F source" model is refuted in BOTH directions**: D and E cost **38 each** (they genuinely share one driver), while C costs **99** and G **86**. Sum over the seven non-F sources = **451** vs the model's 525. **Right as an average, wrong as a per-source constant.**
- **The measured total is 725 — BELOW the SPEC's band FLOOR of 850.**

**BAND: 750-1000, BUDGET ~850.** The budget adds ~100 over the measured 725 for T9's seven drivers beyond what each source's own arms already carry, plus slack. ⚠️ **The measured figures are REALIZED-BASIS and must NOT be re-multiplied** — SPEC §12 says this explicitly, and the phase-80 2.56× overrun is not applicable: its fixture bucket was 631 of 1636 at 4.10×, and **phase 81 adds +0 fixtures**.

**§6.1 TRIGGERS — NEITHER FIRES:**

| trigger | value | margin | fires? |
|---|---|---|---|
| `~25 numbered tasks` (`BOOTSTRAP_PROMPT.md:289`) | **12** | **2.1×** | **no** |
| `~1500 lines of code` net (`:290`) | **~850** (band 750-1000) | **1.5-2.0×** | **no** |
| mid-execution >~10 sub-steps (`:292`) | — | — | ⚠️ **RECORD if it fires; never absorb it** (it fired at phase 80: 6 enumerated vs 17 executed) |

⇒ **NO SPLIT.** The SPEC's contingent axis (81.1 `rbac-stat-name-guards` = F1+F2+D+E · 81.2 `filter-stat-prefix-guards` = A+B+C+G+H) is **not invoked** and is carried forward unused. The router's *"81.1 sources / 81.2 the empty-segment retrofit"* axis stays **refuted** for the reason SPEC §12 gives.

### 1.7 ⚠️ ADR-0045 DOES **NOT** MIS-LOCATE THE SPLIT GATE — A LANDED CLAIM, REVERSED

Both the router (item 9) and SPEC §11 assert *"ADR-0045 QUOTES rather than states the split gate AND mis-locates it to §5 state 2."* **The mis-location half is REFUTED.** Controller-verified at this tip:

```
BOOTSTRAP_PROMPT.md:209  ## 5. Phase Lifecycle State Machine
BOOTSTRAP_PROMPT.md:221  2. SPEC.md exists, PLAN.md does not
                    :225     → GATE: if PLAN.md > ~25 tasks OR > ~1500 LoC estimated
                    :226             → split into NN.1, NN.2, …; update ROADMAP + STATE; stop
```

**§5 state 2 genuinely carries the gate**, verbatim. And ADR-0045 cites **both** locations, **both accurately** — §6.1 at its `**Settles:**` line and again in its cost paragraph, §5 state 2 in §Context and §Cross-references.

**What survives:** ADR-0045 *quotes* rather than *states*, so a reader inherits a paraphrase; **quoting `BOOTSTRAP_PROMPT.md:289-290` directly is still the right habit.** But the PLAN and the IMPL must **not** repeat the mis-location charge. ⚠️ **This is the second time this lineage has carried a "defect" that measurement dissolves** (the first was D-81-SANITIZE's "internal inconsistency", partially refuted at the SPEC). `reference_a_drift_correction_is_itself_a_claim`.

### 1.8 ⚠️ `84 of 121 PHASE DIRS` IS STALE — IT IS **85 of 122**

Controller-verified: **122** phase dirs, **37** carry `REVIEW.md`, **85** do not. Newest carrying one: `docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/` — so *"none since 25.3"* **holds**. The delta is exactly the `81-stats-name-charset-guards` dir the phase-81 BRAINSTORM created; **the `37 with` figure is unchanged.** Gate (f) remains a **STANDING LINEAGE DEPARTURE**, now correctly denominated.

### 1.9 ⚠️ A **SECOND** CORPUS RED — PLACEMENT-CONTINGENT, AND D-81-DEPTH IS WHAT WOULD TRIP IT

SPEC §2.4 found one hard red (`tenant-foo`). **A4 found a second, in the same hiding place — a `_test.go`:**

```
internal/filter/http/wasm/abi_callbacks_test.go:1453
    f.cfg = &compiledConfig{pluginName: "my-plugin"}
```

Controller-verified: `pluginName` **is** the source-C field — `compiled_config.go:444-447` documents it as *"the `PluginConfig.name` discriminator … Threads into stat-name registration (`wasm.<pluginName>.executions`)"* — and `IsValidName("my-plugin") = false`.

**Verdict: AMBER, not a hard red — but only because of where guard C is placed.** The test sets the field by **struct literal**, bypassing `buildCompiledConfigImpl` and `newFilterStats(factoryCtx.Stats, pc.GetName())` entirely. The T4 boot-site placement does not reach it.

⚠️ **IT BECOMES A HARD RED THE MOMENT THE GUARD MOVES** — into `compiledConfig` construction, into a `pluginName` accessor, or into `newFilterStats` as an unconditional precondition. **§7's table-driven-shared disposition makes exactly that relocation tempting.** ⇒ **T4 PINS guard C at the `pc.GetName()` boundary and records this site.** Controller-enumerated: the wasm package has exactly **two** struct-literal `pluginName:` assignments — `fuzz_hostcall_test.go:269` (`fuzz_plugin`, valid) and this one.

### 1.10 THE GATE POSTURE, THE STAT-SURFACE +0 METHOD, AND THE MEASURED BASELINES

⚠️ **The differential is a REGRESSION ANCHOR, NOT EVIDENCE for this row.** SPEC §10's zero-coverage finding was re-derived and **HOLDS**: `track_per_rule_stats` occurs **exactly once in all of `test/`** (`0018-http-rbac/README.md:293`; NC `rules_stat_prefix` ⇒ **43**), and neither `0032` nor `0033` sets a `stat_prefix` inside a `RateLimit` block. **F1/F2 and H — the sources carrying the headline crash and the proven red — have ZERO fixture coverage. Unit tests are the sole venue** (`0044-network-rbac-boot-reject/driver/driver.go:22-26`).

⚠️ **The `TestNoNewStat*` blindness is now proven BY EXECUTION, not by reading.** All five live in `internal/statssink/registration_test.go` (`:26 :53 :81 :109 :137`). `go list -deps ./internal/statssink` = **328** packages containing `internal/stats` but **zero of the seven target packages**. A4's cross-product:

| tree state | result |
|---|---|
| clean | **5 PASS** |
| registration injected in `internal/statssink/flusher.go` (what they are *for*) | **5 FAIL** |
| registration injected in `internal/filter/http/wasm/stats.go` (a target package) | **5 PASS — BLIND** |

The negative arm is therefore not vacuous: the guards do catch what they are meant to.

⚠️ **THE DENOMINATOR PAIRING IS WRONG IN EVERY PHASE-81 DOCUMENT.** *"208-code-site / 84-file"* **welds two denominators together**. Re-derived: **210** production grep-hits → minus **2** comment lines (`internal/stats/doc.go:20,21`) → **208 code sites**, living in **36 production files**. **84** is the production+**test** file count, whose hit total is **508**, not 210. **Cite 208 / 36. Never 208 / 84.**

**MEASURED BASELINES at this tip** (A4, clean tree, recipe in T11):

| gate | result | wall-clock |
|---|---|---|
| full 120-fixture differential | **120/120**, `INNER_EXIT=0`, all four arms green, 0 panics/races | **406 s** |
| h2spec | **53/53**, `INNER_EXIT=0` | **4 s** |
| `go test ./cmd/envoy-go/` | `INNER_EXIT=0` | **9 s** |
| seven target packages + `internal/stats/...` | all `ok` | **1 s** |

**406 s sits inside SPEC §3's ~400-430 s band — CONFIRMED.**

⚠️ **BUT h2spec IS NOT A CO-EQUAL GATE COST.** SPEC §3 prices the pair as *"~400-430 s per green attempt"* plus *"an explicit h2spec run"*. Measured, h2spec is **4 s — about 1% of the differential**, overstated ~100× by being named alongside it. **The gate cost IS the differential.**

⚠️ **AND THE `./cmd/envoy-go` CONSUMER SET IS THREE PACKAGES, NOT TWO.** Beyond `harness.go:240`/`:594` and `h2spec_test.go:210` (all three byte-exact at this tip), **`cmd/envoy-go/main_test.go` builds and boots the same binary at FOUR sites** (`:51 :198 :230 :943`) and **boots real configs, so it can red on a new boot reject**. It costs **9 s**. **T11 adds it.**

### 1.11 ANCHOR DRIFT — TWO REAL, THE REST HELD

Every `.go` anchor in SPEC §1/§2/§4/§6/§8/§10 was re-verified at `82e425ac`. **Two drifted, both by a symbol-vs-comment-range confusion:**

| SPEC cite | ACTUAL | note |
|---|---|---|
| `namespacePrefix :500-506` | doc comment `:500-506`, **func decl `:507`** | cite the SYMBOL |
| `mongoproxy/codec_test.go:518,539` | func decls at **`:517`, `:538`** | 1-line drift |

⚠️ **`DECISIONS.md:6291` (ADR-0132 §Decision (v)) did NOT drift** — SPEC §14 hazard 7 warned it would, because the SPEC's own commit moved the file +42 lines. **Those 42 lines were an append at the tail (`:17726+`), which cannot shift `:6291`.** The warning was sound in principle and wrong in fact; **verify, do not assume, in either direction.**

Everything else held exactly, including all four RBAC engine builds (`:235 :241 :254 :260`), both `perPolicy.Inc` sites (`:732 :760`), and A/B/H's sites.

### 1.12 ⚠️ THE DIFFERENTIAL **ABORTS**, IT DOES NOT MERELY FAIL — AND THE NOTIFICATION SAID SUCCESS

**A4's first full run died at fixture 84 of 119** and **35 fixtures never ran**:

```
panic: driver: start ALS receiver on 0.0.0.0:35843: … bind: address already in use
  test/fixtures/0083-grpc-access-log-headers/driver/driver.go:183 (ensureServer)
INNER_EXIT=1 · --- PASS: 84 (want 120) · comm -3 listed 35 unrun fixtures
```

⚠️ **The background-task notification reported "completed (exit code 0)". The inner status was 1.** `reference_harness_exit_code_is_not_command_exit_code`, **FIRED AND OBSERVED** — and at the *same* 84-of-119 abort point as the phase-76 instance on the sibling fixture `0082`.

**Classified: KNOWN-LIVE FLAKE, a THIRD species. Not a FINDING.** Evidence, four independent strands:
1. **The port is OUT-OF-BAND** — `35843` is kernel-ephemeral, outside `20000-31007` (subject) **and** `11000-14999` (backends) and every static fixture range. It is therefore **neither** hardened half, and an in-band recurrence of either would still be a FINDING.
2. **The bind site is the fixture driver's OWN receiver**, not the harness: `allocateALSPort` binds `:0`, reads the port, **closes**, then `ensureServer` **re-binds** — a TOCTOU window the `0e9cc680`/`f2dd994a` hardenings never covered, because **driver-owned gRPC receivers are not `BackendKind`s** (`reference_differential_grpc_receiver_driver_owned`).
3. **Isolate re-run PASSED** — `--- PASS: TestDifferential/0083-grpc-access-log-headers (4.09s)`, selector match asserted positively (no `[no tests to run]`).
4. **Full re-run CLEAN** — 120/120, `INNER_EXIT=0`, zero panics.

**None of the "recurrence is a FINDING" species fired:** no `hcm/h2 TestServerConn_TinyWindowDelivery`, no in-band subject/backend bind failure, no `0061-lb-ring-hash` spread failure (`0061` passed in run 2).

⚠️ **BUDGET CONSEQUENCE:** observed **1-in-2** abort rate ⇒ budget **~2 differential launches per green pass** (~14 min wall clock, ~28 min with `-race`). ⚠️ **A naive `--- PASS` tally of 84 reads as "a few failures", not as an abort that skipped 35 fixtures.** **Always capture the INNER exit code, always run `grep -cE '^panic:'`, and always run the `comm -3` arm.**

### 1.13 FINDINGS NO PHASE-81 DOCUMENT CARRIES

1. **`internal/filter/network/rbac/rbac.go:50` already contains the token `F2`** as an unrelated **phase-26 fork label** (`NO PerPolicyCounters (F2 — …)`). ⚠️ **Do not use `F2` as a grep anchor in that package** — `reference_sentinel_matcher_string_self_clears`, in a new carrier.
2. **`/BOOTSTRAP_PROMPT.md` DOES NOT EXIST at the filesystem root.** Every phase-81 document writes it with a leading slash as though absolute; only `<repo>/BOOTSTRAP_PROMPT.md` exists. ⚠️ **And the second copy is a LIVE stale-cite hazard**: `docs/superpowers/plans/2026-04-21-envoy-go-bootstrap-prompt.md` is **1024** lines vs the repo root's **522**, and **all nine cited anchors differ** — offset **+197** for §6.x but **+228** for §7.5, so **no constant mental correction works**. Always open the repo-root file.
3. **A THIRD extractor trap in `STATE_HISTORY.md`.** The naive anchor `^- \*\*prior active-phase:\*\*` yields **161**; the parenthetical-tolerant `^- \*\*prior active-phase[^*]*:\*\*` yields **167**. The **6** missed bullets use the eviction form `- **prior active-phase (evicted at the phase-NN … close, …):**` at lines **420, 424, 428, 430, 432, 434**. ⚠️ **With the naive anchor, phase 79 reads as ENTIRELY ABSENT from the archive when all four of its bullets are present.** Any archive-gap gate must use the tolerant anchor. *(A5 reported the tolerant figure as 165, which contradicts its own "6 missed"; the controller re-derived 161/167, delta exactly 6, self-consistent.)*
4. **`misspell` locale US fired LIVE on first-draft guard prose** — A1's `behaviour` in a new comment, A2's in `perpolicy.go`. **All nine guards ship new comment prose.** This is an IMPL hazard, not a hypothetical (`reference_golangci_misspell_locale_us`).
5. **Guard C needs a NEW import.** `internal/filter/http/wasm/compiled_config.go` does not import `internal/stats` today, and the target function already declares a local `stats := newFilterStats(...)` at `:1048`. A guard placed **before** that `:=` compiles (Go scoping starts the shadow at the assignment) — **but the fragility is real and T4 pins it.**
6. **The per-route wasm reject is FAIL-OPEN, not a boot failure.** `decode_headers.go:134-140` catches it, increments `envoyGoFailures`, logs, and returns `Continue`. SPEC §10's claim that a boot-site guard covers both tiers is **CONFIRMED and strengthened** by call-graph walk (`decode_headers.go:133` → `resolveEffective` `:1312` → `parsePerRouteWasm` `:70` → `buildCompiledConfig` `:97`).
7. **`RegisterPerRouteValidator`: the SPEC's "21 files repo-wide" is a FILE-MENTION count.** The real number of **filters** with a registered per-route validator is **FIVE**, wired at `builtins.go:67-71` (verified by execution against the live registry after `RegisterBuiltins`, with rbac `false` and an invented filter `false`). **Say "5 of 12 registered HTTP filters", not "21 files".**
8. **F2's aggregated-log-dedupe assertion has NO in-tree precedent** — all four landed GUARD-SKIP sites are **silent**. It is the row's only genuinely new test shape and is budgeted as such in T8.
9. **The archive-gap total of 57 could NOT be reproduced and is NOT carried.** What reproduces exactly: **phases 67-75 missing all four bullets = 36**, and **`phase 77 PLAN done` absent from BOTH files** (archive-bullet form ⇒ 0/0, with FIRING NCs on the phase-77 BRAINSTORM/SPEC/IMPL siblings ⇒ 1 each). ⚠️ **A5 additionally found phase 76 missing THREE bullets** (BRAINSTORM, SPEC, PLAN) — the same species, never called out. **57 has no denominator anyone can state.** `reference_a_drift_correction_is_itself_a_claim`: **contested counts get no number.**

10. ⚠️ **SPEC §9's CORPUS FIGURES ARE NOT REPRODUCIBLE, THOUGH THE PROPERTY HOLDS.** *"1830 extracted values"* matches **none** of four natural extractions (all YAML scalars **5336** · distinct **987** · distinct key:value **1202** · `envoy.yaml`-only **2241**). And *"the only dotted value anywhere is `http.test`"* is **REFUTED twice over**: controller-verified, `http.test` has **ZERO** occurrences in `test/` — it lives only in unit tests of `internal/filter/hcm`, `http/admission_control` and `http/adaptive_concurrency`, **none of them among the nine sources** — while the fixture corpus carries **170** distinct dotted values. ⚠️ **What DOES hold, on a stated denominator: of the 108 actual nine-source token values, ZERO carry an interior `..`, leading dot or trailing dot.** **The IMPL must state its own extraction method and denominator, never inherit 1830.**
11. **Fixture coverage for A and B counts INSTANTIATIONS, not token values.** `0050-mongo-boot-reject` and `0047-zookeeper-boot-reject` **deliberately omit** `stat_prefix` (their drivers say so verbatim) — they *are* the missing-prefix boot-reject fixtures. **Value-supplying coverage is A = 3 (not 4) and B = 2 (not 3).**
12. **The `0018/README.md:293` "deferred" adjective is invented.** The line is a `D6 … per ADR-0146` **decisions-honoured recital**, and the next sentence says scenario 8's shadow-rules *do* emit. **Location and the zero-coverage conclusion are exact; the adjective is not.**

### 1.14 CONFIRMED, SO THE IMPL CAN RELY ON IT

D-81-F-SITE's two-part shape and its atomicity · the `PerPolicyCounters.Inc` **2** non-test call sites and `BuildRulesEngine` **3** · **network rbac never constructs `PerPolicyCounters`** (2 hits, both comments; sole production site `http/rbac/rbac.go:493`) · the §2.9 **11 REJECT / 4 SKIP** split, SKIP set = `mongoproxy/codec.go:477,:503,:517` + `xds/stats.go:29` · the per-route **request-time** compile · `guest-side-config` is a **genuine false positive** (`IsValidName` = **false**, yet guard C passes the test — the exemption rests on the call graph, **not** on the string being harmless) · the `tenant-foo` RED, fired at exactly `:579`'s `t.Fatalf` · ADR-0132 §Decision (v) at `:6291` and the D5 shape surviving guard G · all four SPEC §6 wordings collision-free in their target packages · ADR-0065 (b)/(e) verbatim at `:2379`/`:2382`.

---

## 2. Global constraints

1. **ONE STAGE PER SESSION.** This PLAN writes no `.go`. The IMPL is a separate session.
2. **`ROADMAP.md` BYTE-UNTOUCHED at this stage.** Row 81 stays `in-progress`; it flips at the IMPL six-gate. Sentinel `want` STAYS **113**.
3. **`BEHAVIOR_CONTRACT.md` and `DECISIONS.md` BYTE-UNTOUCHED at this stage.** ADR-0303 is `PROPOSED`; its §Decision + §Consequences land at the IMPL, and **the STATUS word must flip `PROPOSED` → `COMPLETE` in that same commit** (ADR-0299 shipped stale at `PROPOSED` for two full rows). Mechanical recurrence guard: *"a block whose STATUS reads `PROPOSED` must carry no `### Decision` heading"* — A5 verified it fires on both doctored arms.
4. **F1 and F2 land ATOMICALLY** (T7 + T8 in one commit). Shipping the boot reject without the `Inc` backstop leaves the process crashable by the very config class the row exists to close.
5. **The `tenant-foo` repair lands in the SAME commit as guard H** (T3). Split across commits, the tree is red in between and no differential run reports it.
6. **ASSEMBLED probe at all nine sources** (§1.1). **Skip-if-empty everywhere except A and B**, where the empty case is already rejected upstream (§1.2).
7. **Per-task `gofmt` + `golangci-lint` on touched packages.** ⚠️ `gofmt -l` **never exits non-zero — gate on OUTPUT.** ⚠️ `misspell` runs `locale: US` and **fired live twice this stage**.
8. **Guards register NO stat names.** Stat-surface delta **+0**, discharged by call-site enumeration against **208 code sites / 36 production files** (§1.10).
9. **No new fixtures, no new fuzzers, no new packages, no new modules, no new `BackendKind`.**

---

## 3. File structure — the IMPL's edit surface, RE-DERIVED at `82e425ac`

**Production (9 files, ~165 added lines):**

```
internal/filter/network/mongoproxy/config.go          A   +12
internal/filter/network/zookeeperproxy/config.go      B   +12
internal/filter/http/ratelimit/compiled_config.go     H   +18
internal/filter/http/wasm/compiled_config.go          C   +12   (NEW import of internal/stats)
internal/filter/http/rbac/rbac.go                     D,E +18 · F1 +60   (two DISJOINT hunk sets)
internal/filter/http/compressor/compressor.go         G   +11
internal/rbac/perpolicy.go                            F2  +22
```

⚠️ **`internal/filter/http/rbac/rbac.go` is edited by TWO tasks.** A3's D/E hunks are the `const (…)` block after `New` and the two `if p := …` guards after the `cc := &compiledConfig{…}` literal. A2's F1 hunks are inside `buildCompiledConfig`'s `trackPerRuleStats` gating. **They are disjoint** — verified by both agents — but T5 and T7 must not be executed by two agents in one worktree without staging **by explicit path**.

**Test (7 files, ~560 added lines):** the `_test.go` sibling of each of the above, plus the shared per-package charset drivers (T9).

**Corpus repair (1 file, +3/−3):** `internal/filter/http/ratelimit/compiled_config_test.go:574`, `:616`, `:617` — `tenant-foo` → `tenant_foo`.

**Docs at the IMPL, not now:** `DECISIONS.md` (ADR-0303 §Decision + §Consequences + STATUS flip), `BEHAVIOR_CONTRACT.md` (the departure paragraph), `ROADMAP.md` (row 81 → `done`).

---

## Task 1 — source **A**, mongo `stat_prefix`

**Site:** `internal/filter/network/mongoproxy/config.go` :: `parseConfig`, after the existing empty-check at `:81-82`.
**Probe:** `"mongo." + sp + ".cx_destroy_remote_with_active_rq"` — the **longest** assembled name over 23 fixed leaves (A1 measured the roster).
**Shape:** **no** `tok != ""` arm (dead code — `errStatPrefixRequired` fires first).
**Wording:** `errStatPrefixInvalid = "mongo_proxy: stat_prefix contains characters invalid for a metric name"` → `errors.New(…)`. Collision-checked **0** in the target package (NC: `errStatPrefixRequired` ⇒ 5).

**Tests (~47):** the `http/lua/compiled_config_test.go` 3-arm shape — invalid / valid-passthrough / empty-passthrough — as a 6-row table, plus a byte-stable const pin.
⚠️ **The `empty` arm asserts `errStatPrefixRequired`, NOT the new charset error.**
⚠️ **One arm is an ACCEPT-pin, not a reject:** `invalid-trailing-dot-segment` (`foo.` → `mongo.foo..op_query` → accepted), naming SPEC §13.1. **Do not "fix" it.**

**Gate:** package `go test -count=1` green; the NC (neuter the guard to `if false && …`) must red the invalid arms — **A1 verified the NC landed by grepping the neutered line before running it.**

**Record, do not fix:** guard A makes mongo **accept** a leading-digit `stat_prefix` that its bare-guarded thrift/kafka/redis siblings reject (§1.1).

## Task 2 — source **B**, zookeeper `stat_prefix`

**Site:** `internal/filter/network/zookeeperproxy/config.go` :: `parseConfig`, after `:169-170`.
**Probe:** `sp + ".zookeeper.getallchildrennumber_decoder_error"` — longest over **201** suffixes (A1 measured by execution).
**Shape:** no `tok != ""` arm (as T1).
**Wording:** `errStatPrefixInvalid = "zookeeper_proxy: stat_prefix contains characters invalid for a metric name"`. Collision-checked **0**.

⚠️ **B is the ONLY LEADING-position source** (§1.1), so bare and assembled agree except on a trailing dot. **Use assembled anyway** — uniformity across the row, and it is the ADR-0065 (b) rule. **T9's position table pins B as the leading-position control.**

**Tests (~47):** as T1, 6-row table + const pin. Includes the `foo.` accept-pin.

## Task 3 — source **H**, ratelimit `stat_prefix` — **AND the corpus repair, in the SAME commit**

**Site:** `internal/filter/http/ratelimit/compiled_config.go` :: `buildCompiledConfig` (decl `:408`), at the `statPrefix:` assignment `:467`.
**Probe:** `"cluster." + clusterName + ".ratelimit." + sp + "." + statNameFailureModeAllowed`.
**Shape:** **keeps** the `tok != ""` arm — H's empty case is live and landed (`stats.go:198-200` elides the segment wholesale when empty; A1 verified `newFilterStats(rls,"")` registers cleanly).
**Wording:** `parseRejectStatPrefixInvalid = "ratelimit: stat_prefix contains characters invalid for a metric name"`. Collision-checked **0** in the target package (NC: `parseRejectDomainRequired` ⇒ 5).

⚠️ **ORDERING IS A DECISION, NOT A DEFAULT.** The assembled name needs `clusterName`, available only after `validateGrpcServiceAndResolveCluster` (`:449`). The charset reject therefore fires **after** arms 6-12: an invalid `stat_prefix` on a config with an unknown RLS cluster reports the **cluster** error. **Accepted** — `clusterName` is pre-guaranteed valid (SPEC §2.9's `ratelimit/stats.go` finding) — **but the IMPL must not silently reposition it.**

**⚠️ THE CORPUS REPAIR — MANDATORY, SAME COMMIT:**
```
internal/filter/http/ratelimit/compiled_config_test.go:574   StatPrefix: "tenant-foo"  ->  "tenant_foo"
internal/filter/http/ratelimit/compiled_config_test.go:616-617   assertion, both lines
```
**A1 executed the full before/after.** Without the repair: `--- FAIL: TestBuildCompiledConfig/HappyPath … compiled_config_test.go:579: buildCompiledConfig: ratelimit: stat_prefix contains characters invalid for a metric name` — firing at **exactly** the line SPEC §2.4 forecast. With it: green.
⚠️ **No differential run can find this** — `0032`/`0033` set only the HCM `stat_prefix`.

**Also owed:** a new row in the existing `TestParseRejectConstants_ByteStable` case table (~5 lines; additive, so it does not red on arrival).

## Task 4 — source **C**, wasm `PluginConfig.name` — **covers BOTH tiers**

**Site:** `internal/filter/http/wasm/compiled_config.go` :: `buildCompiledConfigImpl` (decl `:847`), **before arm 26 (`:977`)** so it also fires on the boot-time `validatePerRouteWasm` shape-check (the validate-only short-circuit is at `:1033`, after arm 26).
**Probe:** `"wasm." + pc.GetName() + ".http_call_dispatch_unknown_cluster"` — longest of 16 per-plugin counters. ⚠️ **A second registration path exists** at `:1064` (`dynamic.NewRegistry(…, "wasm."+pc.GetName(), …)`); the same guard covers both.
**Wording:** `parseRejectPluginNameInvalidFmt` — collision-checked **0** tree-wide.

⚠️ **NEW IMPORT** of `internal/stats` (§1.13 item 5); `goimports` orders it before `internal/stats/dynamic`. The function declares a local `stats :=` at `:1048`; the guard sits **before** it and compiles, but **pin the ordering in a comment.**

**Tests (~87):** the 3-arm table (~47) + a **per-route tier pin** (~17) asserting `parsePerRouteWasm` returns the byte-identical listener-tier wording, + a **`guest-side-config` non-regression pin** (~5) + header.
⚠️ **The per-route reject is FAIL-OPEN** (`decode_headers.go:134-140` → `Continue`), not a boot failure. Assert that disposition, not an error return.
⚠️ **`TestBuildCompiledConfig_Arm26_EmptyName_SkipsRegistry` and `TestUnregisterPluginConfigName_EmptyName_NoOp` are IRRELEVANT to this guard** — they call `registerPluginConfigName`/`unregisterPluginConfigName` directly (§1.2). Do not cite them as blast radius.

**⚠️ PIN THE GUARD AT THE `pc.GetName()` BOUNDARY — THIS IS LOAD-BEARING, NOT STYLISTIC.** `internal/filter/http/wasm/abi_callbacks_test.go:1453` sets `&compiledConfig{pluginName: "my-plugin"}` by **struct literal**, and `IsValidName("my-plugin") = false` (§1.9). At the `pc.GetName()` boundary the guard never sees it and the test stays green. **Relocated** — into `compiledConfig` construction, a `pluginName` accessor, or `newFilterStats` as an unconditional precondition — **it becomes a hard red.** T9's shared-driver work is exactly what makes that relocation tempting. **Add a comment at the guard site naming this constraint**, and record the two struct-literal sites (`fuzz_hostcall_test.go:269` valid, `abi_callbacks_test.go:1453` invalid) so a later sweep does not "tidy" the placement.

## Task 5 — sources **D** + **E**, HTTP-rbac `rules_stat_prefix` / `shadow_rules_stat_prefix`

**Site:** `internal/filter/http/rbac/rbac.go` — const block after `New`, guards after the `cc := &compiledConfig{…}` literal (`:224`/`:225` are the assignments).
**Probes:** `http.<hcm>.rbac.<namespacePrefix(rules)>.allowed` and `…<namespacePrefix(shadow)>.shadow_allowed`.
**Wordings:** `rejectRulesStatPrefixInvalidFmt`, `rejectShadowRulesStatPrefixInvalidFmt` — both collision-checked **0**.

⚠️ **`namespacePrefix("")` returns the literal `"rbac"`** (`:507`, controller-verified), so **the empty token never produces an empty segment** and the two `_Accepted` tests at `rbac_test.go:197`/`:213` are untouched by an assembled guard. **Keep skip-if-empty for uniformity; do not claim it is what saves those tests** (§1.2).
⚠️ **A BARE guard here reds 26 top-level rbac tests** (A3, measured). That is the one place §8's fear was real — and it is an argument for assembled, not for skip-if-empty.

**Tests (~58 total, ~29 each):** D and E genuinely share one table-driven driver — the **only** pair in the row that does (§1.5).

**⚠️ COORDINATION:** disjoint from T7's F1 hunks in the same file. Stage by explicit path.

## Task 6 — source **G**, `compressor_library.name`

**Site:** `internal/filter/http/compressor/compressor.go` :: `New` (`:283`), `libraryName` bound at `:291`, stats at `:299`.
**Probe:** `"http." + hcm + ".compressor." + lib + ".gzip.response.total_uncompressed_bytes"`.
**Wording:** `rejectLibraryNameInvalidFmt` — collision-checked **0**.

**⚠️ THE D5 NON-REGRESSION ARM IS OWED, AND IT IS NOT THE INCUMBENT TEST.** `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` (`:2376`) calls `newFilterStats` **directly** and is **structurally blind to any guard in `New`** (§1.2). The new arm must drive the **production `New` path** with `libraryName == ""` and assert the registry contains `http.<hcm>.compressor..gzip.response.compressed` and that all 17 names carry `..gzip.`. **A3 built and ran it — `TestRow81_CompressorD5NonRegression` PASS.**

**The pin it protects:** `compressor.go:525-526` *"DO NOT collapse"* · `compressor_test.go:2373` *"LOAD-BEARING D5 behavioral pin"* · **ADR-0132 §Decision (v)** at `DECISIONS.md:6291` (**verified exact; it did not drift**). Controller-executed: `IsValidName("compressor..gzip.total") = true`, so guard G leaves D5 intact.

**Tests (~75):** 3-arm table (~40) + D5 non-regression (~19) + header (~16).

## Task 7 — **F1**: the boot reject at `buildCompiledConfig`, GATED on `trackPerRuleStats`

**Site:** `internal/filter/http/rbac/rbac.go` :: `buildCompiledConfig` (`:222`).
**⚠️ NOT `internal/rbac BuildRulesEngine`** — it is shared (3 non-test call sites: `:235`, `:254`, `network/rbac/rbac.go:162`) and blind to `track_per_rule_stats`; the network consumer **never constructs `PerPolicyCounters`**, so a guard there would boot-reject L4 configs that **cannot panic**. A2 re-verified the L4 non-regression: `New err=<nil>` with `allow-admins`, both before and after.

**⚠️ PROBE THE ASSEMBLED NAME** — `base + ".policy." + name + "." + suffix` — **not the bare token** (§1.1). A bare F1 over-rejects 4 non-empty shapes and desynchronises F1 from F2.
**Wording:** SPEC §6's F1 string is **byte-usable as written** — `%q` carries the policy name and *"the policy name cannot form a valid metric name"* stays true under the assembled probe.

**Two tier-dependent dispositions for free:** boot failure at the listener tier; **log + inherit-listener** at the per-route tier, because `resolvePerRouteConfig` (`:346`, from `DecodeHeaders` `:798`) already implements that policy per ADR-0072. A2 measured the per-route arm: `rbac: per-route resolve failed (inherit-listener): …` → **Continue, 4 base counters only.**

**Tests (~118):** the charset table + the `trackPerRuleStats` gate arms + both tier dispositions.
⚠️ **Two of A2's four deliberate breaks live here** — gutting the guard reds 7 charset subtests; dropping the gate reds the gate test on 7 names. **Both fired.**

## Task 8 — **F2**: the skip-and-log backstop at `PerPolicyCounters.Inc`

**Site:** `internal/rbac/perpolicy.go` :: `Inc` (`:21`), extending the `policyName == ""` early return (`:22`) to also skip when the assembled `key` (`:25`) is not `IsValidName`.
**Wording:** `rbac: per-policy stat skipped: policy name %q cannot form a valid metric name` — **one aggregated line per call site**, not per request.

**⚠️ LANDS ATOMICALLY WITH T7.** A2's break 3 — disabling the skip — produced `panic: stats: invalid metric name: "http.hcm.rbac.p.policy.allow-admins.allowed"`. That is the row's reason to exist.

**Test shape (~74)**, following the `mongoproxy/codec_test.go:517`/`:538` precedent (**decl lines, not the SPEC's `:518,539`**):
1. **no panic** on the guarded path, no `recover`;
2. **no dynamic counter** — assert the specific name absent **AND** the registry total unchanged (a name-only assertion misses a mis-assembled registration);
3. **the fixed counters still fire** — the 4 base counters present;
4. **the enforcement disposition is unchanged** — Continue/403 identical with and without a nameable policy;
5. ⚠️ **NEW SHAPE, no in-tree precedent:** the aggregated-diagnostic assertion — capture `log` output, assert **exactly 2 lines over 150 `Inc` calls at 2 call sites**. A2's break 4 (drop the dedupe) produced `got 150 lines over 150 Inc calls at 2 call sites, want 2`. **All four landed GUARD-SKIP precedents are silent, so this is budgeted, not copied.**

⚠️ **F2's table cannot share F1's** — different packages (§1.5).

## Task 9 — the cross-package charset audit: **SEVEN per-package drivers, not one table**

**⚠️ THE SINGLE SHARED TABLE IS NOT CONSTRUCTIBLE** (§1.5). Budget seven near-identical in-package drivers over a **shared arm roster** (the saving D-81-DEPTH actually buys).

**Each driver asserts, per source:**
- the reject arms (hyphen, bang, and — **for interior sources only** — leading digit / leading dot);
- the accept arms: valid token, and the **`trailing.` ACCEPT-pin naming SPEC §13.1**;
- the byte-stable wording const.

**⚠️ PLUS the row's one genuinely new invariant — the SEGMENT-POSITION table** (§1.1), written **once** in `internal/stats` where it costs nothing:

| position | sources | bare≡assembled? |
|---|---|---|
| **LEADING** | B | agree except trailing dot (1/9) |
| **INTERIOR** | A, C, D, E, F1, F2, G, H | **disagree on 5/9** |

This is the pin that stops a later sweep "simplifying" the nine guards to bare probes and silently boot-rejecting working configs.

## Task 10 — the break roster, each arm proven to fire its OWN assertion

**Ten arms, each restored with `sha256sum` byte-identity.** ⚠️ **Breaks run AFTER committing** (`reference_break_protocol_commit_first`) and **need `-count=1`** (`reference_differential_break_protocol_count1`).

Four are already executed and confirmed (A2): gut F1's guard → 7 charset subtests · drop the `trackPerRuleStats` gate → gate test on 7 names · disable F2's skip → **process panic** · drop the log dedupe → `150 lines … want 2`.
Six remain, one per non-F source: neuter each guard to `if false && …` (**A1's and A3's shape — the NC's landedness verified by grep BEFORE running**, `reference_probe_input_is_a_claim`).

⚠️ **A3 recorded a broken-gate near-miss the IMPL must avoid:** its first NC used a bare `if true {`, which orphaned the `stats` import → the package **failed to build**, printing `FAIL … [build failed]` with **zero `--- FAIL:` lines**, so a `grep -c '^ *--- FAIL:'` read **0** and looked like *"the NC did not fire."* Re-run compile-clean (`|| true`) it read 41. ⚠️ **A build failure is not a zero result** (`reference_empty_output_is_not_a_zero_result`). **Every break arm must be compile-clean and must assert WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`).

## Task 11 — the gates

1. **Per-package** `go test -count=1` over the seven target packages **plus `internal/rbac` and `internal/stats`** (baseline **1 s**); then `-race`.
2. **`gofmt -l`** on all touched packages — ⚠️ **gate on OUTPUT, it never exits non-zero.**
3. **`golangci-lint run`** (v1.64.8, 9 linters, `disable-all: true`) — ⚠️ **`misspell` locale US fired live TWICE this stage.**
4. **`go test ./cmd/envoy-go/ -count=1`** — ⚠️ **the THIRD consumer of the `./cmd/envoy-go` binary** (`main_test.go:51 :198 :230 :943`), which **boots real configs and can red on a new boot reject**. Baseline **9 s**. Neither the SPEC nor the router names it.
5. **The FULL 120-fixture differential**, `-count=1 -v`. Baseline **120/120 in 406 s**, inside SPEC §3's band.
   ```sh
   ( go test ./test/differential/ -count=1 -v > "$SCRATCH/full.log" 2>&1; echo "INNER_EXIT=$?" )
   grep -cE '^    --- PASS: TestDifferential/' "$SCRATCH/full.log"          # want 120
   grep -E  '^    --- (FAIL|SKIP): TestDifferential/' "$SCRATCH/full.log"   # want EMPTY
   grep -c  'no driver registered for fixture' "$SCRATCH/full.log"          # want 0
   grep -cE '^panic:|DATA RACE|SIGSEGV' "$SCRATCH/full.log"                 # want 0
   grep -o  'TestDifferential/[^ ]*' "$SCRATCH/full.log" | sed 's|TestDifferential/||' | sort -u \
     | comm -3 - <(ls -1 test/fixtures/ | grep -E '^[0-9]{4}[a-z]?-' | sort)  # want EMPTY
   ```
   ⚠️ **Use `./test/differential/` WITHOUT `...`** — with `...` it matches two packages and buffers `-v`, so a 7-minute silent log is normal.
   ⚠️ **CAPTURE `INNER_EXIT` AND THE PANIC ARM.** The run **aborts** rather than failing (§1.12): a 1-in-2 observed ALS-driver port race kills the binary at fixture 84 and **35 fixtures silently never run**, while the harness notification reports success. **Budget ~2 launches per green pass.**
   ⚠️ The faithful dir predicate is `^[0-9]{4}[a-z]?-`; a bare `^[0-9]{4}-` gives **118**.
6. **h2spec explicitly** — a consumer **not** covered by `./test/differential/`. Baseline **53/53 in 4 s**. ⚠️ **It is ~1% of the differential's cost, not a co-equal gate** (§1.10) — run it because it is a distinct consumer, not because it is expensive.
7. **Stat-surface +0 by CALL-SITE ENUMERATION**, not by `TestNoNewStat*` (blindness proven by execution, §1.10):
   ```sh
   # ARM 1 — added PRODUCTION registration sites in the row's diff. MUST print NOTHING.
   git diff --unified=0 "$BASE"..HEAD -- '*.go' ':!*_test.go' \
     | grep -E '^\+[^+]' | grep -E '\.New(Counter|Gauge)(IfAbsent)?\('
   git diff --unified=0 "$BASE"..HEAD -- '*.go' ':!*_test.go' | wc -l   # input measure, MUST be > 0
   # ARM 2 — production census invariant. MUST print 208 then 36.
   git grep -nE '\.New(Counter|Gauge)(IfAbsent)?\(' -- '*.go' ':!*_test.go' \
     | grep -vE ':[0-9]+:[[:space:]]*//' | wc -l
   git grep -lE '\.New(Counter|Gauge)(IfAbsent)?\(' -- '*.go' ':!*_test.go' | wc -l
   ```
   ⚠️ **Denominator is 208 code sites / 36 production files — NEVER 208/84** (§1.10).
   ⚠️ **ARM 1 on a clean tree is VACUOUS** (empty in, empty out) — A4's discriminating NC was a **comment-only production edit** giving a 6-line input diff and still an empty ARM 1. **Assert the input measure, or the arm proves nothing** (`reference_empty_output_is_not_a_zero_result`).
   Matcher NC'd: `\.NewHistogram\(` ⇒ 0; the union's arms are disjoint and sum (143 + 67 = 210), so neither may be dropped.

⚠️ **The differential is a REGRESSION ANCHOR ONLY.** F1/F2 and H have **zero** fixture coverage; the differential can neither cover nor regress on the headline crash or the proven red.

## Task 12 — ADR-0303, `BEHAVIOR_CONTRACT.md`, row 81 → `done`, the sentinel, and the stage close

1. **ADR-0303 §Decision + §Consequences appended IN PLACE after the retained italic footer** — no renumber, **no `---` separator** (`^---$` stays **216**). **STATUS flips `PROPOSED` → `COMPLETE` in the SAME commit.** ⚠️ **Carry NO whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and again in ADR-0302 ¶11. **Enumerate by site.**
   **What ADR-0303 must record:** the segment-position rule (§1.1) and why assembled won; the two-part F1/F2 remedy and **why the asymmetry forces it**; the two-limb ADR-0065 discriminator; and — ⚠️ **new** — that **D-81-EMPTY is retained for legibility, not for load-bearing effect** (§1.2).
2. **`BEHAVIOR_CONTRACT.md`** — the envoy-go-strict **DEPARTURE** paragraph, inline in the HTTP-filter-chain section (the file has no departure-ledger heading). ⚠️ **The reference ACCEPTS the hyphen and hoists the policy name into `envoy_rbac_policy_name`; envoy-go flattens.** Write it as a departure, not a fix.
3. **`ROADMAP.md` row 81 → `done`.** ⚠️ **THE LEAK CHECK RE-ARMS HERE** — never write a sentinel matcher string into `ROADMAP.md`. Row 81's cell must carry **0** deferred-candidate phrases and only the already-registered `Observability-family row` slug.
4. **Sentinel re-run MECHANICALLY after the flip**, with firing NCs. `want` stays **113**. Check (1) goes silent for row 81 — **which is the one time the doctored-copy NC is mandatory**, because silence is then indistinguishable from a broken check.
5. **Six-gate**, honestly labelled: (a)-(e) green/vacuous as measured; **(f) a STANDING LINEAGE DEPARTURE — `REVIEW.md` absent, 85 of 122 dirs carry none, none since 25.3** (§1.8).

---

## 4. Band — **750-1000, budget ~850**, and the estimate is MEASURED rather than modelled

**12 tasks.** Neither §6.1 trigger fires (§1.6): **2.1×** margin on tasks, **1.5-2.0×** on LoC.

⚠️ **Read the budget as "expect the ceiling"**: 76 `~7-9 → 9` · 77 `11-13 → 12` · 78 `7-9 → 10` (**above**) · 79 `10-12 → 12` · 80 `11-14 → 13`. **Four of five landed at or above ceiling** ⇒ read 12-in-a-12-task plan as *"expect 13-14."*

⚠️ **The LoC figure is REALIZED-BASIS and must NOT be re-multiplied.** Phase 80's 2.56× overrun does not transfer: its fixture bucket was **631 of 1636 at 4.10×**, and **phase 81 adds +0 fixtures** (fixture-excluded multiplier **1.97×**, and even that applies to *estimates*, not to code that has been built and measured).

⚠️ **If the IMPL's realized figure crosses ~1200, do NOT split on the router's refuted axis.** The coherent axis, carried forward unused, is **81.1 `rbac-stat-name-guards` (F1+F2+D+E) / 81.2 `filter-stat-prefix-guards` (A+B+C+G+H)** — §6.2 item 3's coherent-slice obligation.

---

## 5. Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

`ls stop` ⇒ `No such file or directory`. **It must not be created.**

- **(1)** `NOT DONE: row 81` at `want=113`. NCs, all fired: `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`; row 81 doctored `done` on a scratch copy ⇒ **SILENT**; row **62** doctored on that same copy ⇒ `NOT DONE: row 62`, so the check still discriminates when otherwise silent.
  ⚠️ **THE ROW-62 NC DID NOT LAND ON FIRST ATTEMPT.** A `sed` targeting `done      ` missed the actual `| done |` spacing and printed nothing — which reads exactly like *"the check is blind."* Caught by inspecting the doctored field before trusting the result, then redone with `awk`. `reference_gate_command_negative_control`, **firing on the controller in the first gate of the session.**
- **(2)** **FIVE — `:191 :201 :211 :217 :225`** — **UNCHANGED. The THIRTIETH consecutive phase at which it did not go down. STATED, not forecast.** ⚠️ One-arm strip moves the union **5 → 4, not 5 → 0** (re-executed: **4**).
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NCs: invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`; the registered slug `Observability` correctly printed **nothing**.
- Input measured **229 lines / 113 data rows**, so an empty result could not read as a zero result.

⚠️ **`want` STAYS 113 — this PLAN adds no row.**

⚠️ **THE LEAK CHECK IS INAPPLICABLE, NOT "PASSED."** The sentinel greps `ROADMAP.md`; **this PLAN writes no ROADMAP cell**, so the check has no input. **It re-arms at the IMPL** (T12.3).

⚠️ **ROW WELL-FORMEDNESS: the gate must remain a DISJUNCTION.** ARM-A (escape-aware `NF!=8`) catches 57 and 69 only; ARM-B (trailing-piece) catches 78 only; row 81 is flagged by neither. Naive `NF==8` is wrong in **both** directions (15 FP + 1 FN).

---

## 6. Counts at this tip — re-derived, each with a negative control

fixtures **120** (bare predicate **118**; next-free **0119**, no gaps) · fuzzers **55** · internal packages **73** (cross-checked two ways) · blank imports **120** on the FULL prefix (naive `^\t_ ` ⇒ **126**) · `BackendKind` **tail 38** over **39** declarations, no gap, no dup, no `iota` · `ROADMAP.md` **229 / 113** · `DECISIONS.md` **17766**, **302** headings, ids 0001-0303, **one gap at ADR-0209**, zero duplicates, tail **ADR-0303 PROPOSED**, next-free **ADR-0304**, STATUS census **16 = 15 COMPLETE + 1 PROPOSED**, retained italic footers **NINE** (`ADR-0294…0300, 0302, 0303`; **0301 carries none**), `^---$` **216** · `BEHAVIOR_CONTRACT.md` **5868** · `STATE.md` **64** · `STATE_HISTORY.md` **434** · `stats.IsValidName` guard sites **15** (chain: 40 → −15 `_test.go` → −2 `test/helpers/` → −8 comments) · stat-registration **508 tree-wide → 210 production grep-hits → 208 code sites in 36 production files**.

⚠️ **`\t` IN GNU ERE IS A LITERAL `t`. Use `-P`.** A5 found the trap fires in the **opposite** direction from the documented warning: the harness `grep` **shell function** normalizes `\t` and returns **126**, while `command grep -cE '^\t_ '` returns **0**. **The trap therefore bites exactly where gates run** (CI, scripts, `git grep`) and is invisible interactively. **Use `-P` unconditionally.**

**Gate baselines, measured at this tip:** full differential **120/120 in 406 s** · h2spec **53/53 in 4 s** · `go test ./cmd/envoy-go/` **9 s** · seven target packages + `internal/stats/...` **1 s** · `go list -deps ./cmd/envoy-go` **560** packages containing **8 of 8** targets (NCs: `test/differential`, `test/conformance/h2spec`, an invented package — all **0**).

**Corpus denominators, stated rather than inherited:** nine-source token values **108** (rejects: `tenant-foo`, `my-plugin`, `guest-side-config`) · Go string literals in the seven target packages **7969** across **93** files · fixture-YAML scalars **5336** (distinct **987**). ⚠️ **Interior `..` / leading dot / trailing dot among the 108: ZERO** — the SPEC's property holds even though its figures do not.

⚠️ **CONTESTED OR UNREPRODUCIBLE, SO NO NUMBER IS CARRIED:** the next-free REFERENCE port (routers say `10119`, `STATE.md` §Project says `10450`) · the `STATE_HISTORY.md` archive-gap total (documented **57**, not reproducible — §1.13 item 9) · SPEC §9's *"1830 extracted values"* (matches none of four extractions — §1.13 item 10). **Row 81 needs none of them.**

⚠️ **`STATE.md` §Project counts SELF-CONTRADICTS §Current and is FROZEN at the phase-76 IMPL close. Anchor on §Current. Do NOT "fix" §Project** — repairing a count by editing the sentence that states it is how the ADR-0296/0297 self-falsifying species starts.

**Eviction target for this close, RE-DERIVED at the tip:** `phase 80 (stats-sds-projection) BRAINSTORM done` (`STATE.md:54`) — the oldest of the five §Recent bullets. **Verified ABSENT from `STATE_HISTORY.md`** (`grep -cF` ⇒ 0, exit 1) with **FIRING NCs on three phase-79 siblings that ARE present** (⇒ 1 each). ⚠️ **Do not inherit an evictee name from any document** — the phase-81 SPEC already moved the one its own router named.

---

## 7. Deferred — named so no later stage re-derives them

Carried unchanged from SPEC §13, plus what this stage added:

1. **The interior empty-segment hole** — `IsValidName("a..b") = true`, **INTERIOR-ONLY** (`a.`, `.a`, `..`, `""` all already rejected — controller-executed). Row 81's nine guards **inherit it by construction** (§1.1), now **measured on four sources**, with **ACCEPT-pins** left in the corpus so the successor gets a failing-first anchor. ⚠️ **BLOCKED: contradicts ADR-0132 §Decision (v)** (`DECISIONS.md:6291`) — the successor must carve out the compressor or supersede that ADR. ⚠️ **Its target is GENERATED, not authored** — a config sweep for `..` finds zero and is structurally blind. Slug: `stats-name-empty-segment-guards`.
2. **The RBAC policy-name PROJECTION divergence** — the reference hoists to `envoy_rbac_policy_name`; envoy-go flattens.
3. **The HTTP-rbac empty-prefix fallback divergence** — ⚠️ **UPGRADED from inference to MEASURED FACT this stage** (§1.3): envoy-go emits the **doubled** `http.myhcm.rbac.rbac.policy.…` where the reference emits a single `rbac` segment.
4. **The matcher-tree `Action`-name boot walk**, which would let F2 be boot-rejected rather than skipped.
5. **The three bare-prefix incumbents** (redis/thrift/kafka) — ⚠️ **now a two-way divergence**: after row 81, mongo (assembled) and its three siblings (bare) disagree on a leading-digit prefix (§1.1).
6. **The two-leg `invalidSecretNameErrFmt` wording** (`boot.go:244`).
7. **The `STATE_HISTORY.md` archive gap** — phases 67-75 (36 bullets) + `phase 77 PLAN done` + ⚠️ **phase 76's THREE missing bullets, newly surfaced**. **No total is asserted.**
8. ⚠️ **NEW — the ALS-family driver port race.** `0081`/`0082`/`0083` (and any driver-owned gRPC receiver) `allocateALSPort` bind `:0` → read → **close** → **re-bind**, a TOCTOU window that **aborts the whole test binary**, not one subtest (§1.12). Observed **1-in-2** this session; the identical shape hit `0082` at the phase-76 IMPL. **Out of row 81's remit** — it touches no stat name — but it is a standing tax on every future row's differential gate, and nothing tracks it.
9. ⚠️ **NEW — `my-plugin` at `abi_callbacks_test.go:1453`** (§1.9). Not owed while guard C sits at the `pc.GetName()` boundary; **owed the moment any later row relocates the guard.** Recorded so the relocation is a decision rather than a surprise.

---

## 8. Gate hygiene — the lineage's broken-gate count is **EIGHTEEN**, and FIVE priors fired live at this stage

**No nineteenth shape.** Fired live:

1. ⚠️ **A negative control that did not land** — the controller's row-62 `sed` missed the real column spacing and printed nothing, reading as *"the check is blind"* (§5). `reference_gate_command_negative_control`.
2. ⚠️ **A build failure read as a zero result** — A3's first break arm orphaned an import, so `FAIL … [build failed]` produced **zero `--- FAIL:` lines** and a count-based gate read **0**. `reference_empty_output_is_not_a_zero_result`.
3. ⚠️ **A harness's exit code is not the command's** — the differential's abort at 84/119 reported `INNER_EXIT=1` while the background-task notification said *"completed (exit code 0)"* (§1.12). `reference_harness_exit_code_is_not_command_exit_code`, at the **same abort point** as the phase-76 instance.
4. ⚠️ **A vacuous gate arm on a clean tree** — the stat-surface ARM 1 is empty-in/empty-out and proves nothing without an input measure; A4's real discriminator was a comment-only production edit (T11.7).
5. ⚠️ **`\t` in GNU ERE** — and in the **opposite** direction from the documented warning (§6).

**The eighteen carried forward, unchanged:** two defects that CANCEL in the gate metric · an inert gate cell · a full-suite recipe without `-v` is VACUOUS · a sha256 roster desynced against a DELETED file · `gofmt -l` NEVER exits non-zero · `go doc -all <A> <B>` swallows arg2 · a `+0 exported symbols` gate over an EMPTY package reds on a CORRECT tree · a RANGE gate cannot detect anchor drift · a roster's naive `[ -f ] || continue` exits 0 on a DELETED file · a count-only stat guard PASSES a build with BOTH names wrong · a `-run` no-match exits 0 with `[no tests to run]` · a `--- PASS` tally over a package with sibling tests exceeds the fixture denominator · a stat-delta claim cannot be discharged by guards scoped to another package · a stderr-VOLUME assertion passes on the hang · `golangci-lint` runs `misspell` with `locale: US` · a harness's exit code is not the command's · a GOLDEN ROSTER that omits the family under test · a NEGATIVE CONTROL POINTED AT A TARGET THAT DOES NOT EXIST.

---

## 9. Self-review against the SPEC

**Held:** the two-part F1/F2 remedy and its atomicity · D-81-HELPER = INLINE · D-81-SANITIZE = reject · the eight §6 wordings, all collision-free · the zero-fixture posture and its `0044` citation · the zero-coverage finding for F1/F2 and H · the `tenant-foo` RED · the ordinal TWENTY-FOURTH · the ADR block.

**Refuted or materially refined by execution — fourteen:**
1. §2.8's "OVER-REJECTS = 1" does not generalise; the answer is set by **segment position** (§1.1). **Load-bearing — it decides all nine guards.**
2. §8's D-81-EMPTY premise collapses under the assembled template (§1.2). **Load-bearing.**
3. §8's seven-test roster: **four tests cannot be reddened by any production guard**, the H row is miscited (**one** bare literal, in `fuzz_test.go` only), and a bare guard on D/E reds **26**, not 2.
4. §7's single shared table is **not constructible** across seven packages (§1.5). **Load-bearing — it re-prices D-81-DEPTH.**
5. §2.1's pasted panic name is the **reference's**; envoy-go doubles the `rbac` segment (§1.3).
6. §12's F2 "~50" is **96**; the "~75/source" model is wrong in both directions; **the measured total 725 is below the band floor 850** (§1.6).
7. §11's ADR-0045 **mis-location charge is REFUTED** (§1.7).
8. §4.1's "21 files register a per-route validator" is a file-mention count; the real figure is **5 filters**.
9. Two anchors drifted (`namespacePrefix`, `codec_test.go`), while the one the SPEC **warned** would drift (`DECISIONS.md:6291`) **did not** (§1.11).
10. **§2.4's corpus survey missed a SECOND red** — `my-plugin` in the source-C field, placement-contingent (§1.9).
11. **§9's `208/84` is a mixed denominator**; the production pairing is **208 / 36** (§1.10).
12. **§9's `1830 extracted values` is not reproducible, and `http.test` has ZERO occurrences in `test/`** (§1.13 item 10). The *property* holds on a stated denominator: 0 of 108.
13. **§3's h2spec pricing overstates it ~100×** — 4 s against the differential's 406 s — and **the `./cmd/envoy-go` consumer set is three packages, not two** (§1.10).
14. **§10's fixture counts for A and B are instantiations, not token values** — `0050`/`0047` deliberately omit `stat_prefix`, so value-supplying coverage is **A = 3, B = 2** (§1.13 item 11).

---

## 10. Operative memories

`feedback_brief_citations_not_evidence` (**five briefs re-derived, four corrected**) · `reference_probe_must_discriminate` (**the four-arm F1/F2 cross-product**) · `reference_probe_input_is_a_claim` (**NC landedness grepped before every break**) · `reference_gate_command_negative_control` (**fired on the controller, §8.1**) · `reference_empty_output_is_not_a_zero_result` (**fired on A3, §8.2**) · `reference_a_drift_correction_is_itself_a_claim` (**ADR-0045 reversed; the archive total refused a number**) · `reference_code_comment_not_evidence` (**`namespacePrefix`'s comment vs the measured double segment**) · `reference_sample_is_not_an_audit` · `reference_grep_c_zero_is_a_broken_gate` · `reference_stale_cite_recurs_fix_by_pattern` (**symbol anchors beat the two that drifted**) · `reference_spec_drafted_identifier_collision_check` (**8/8 free**) · `reference_golangci_misspell_locale_us` (**fired twice**) · `reference_sentinel_matcher_string_self_clears` (**the phase-26 `F2` label**) · `reference_dynamic_stat_name_charset_guard` · `reference_nil_stats_counter_inc_crashes_goroutine` (⚠️ **the WRONG mechanism — `checkName` PANICS**) · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `reference_bash_cwd_reset_commits_to_main` (⚠️ **FIRED, OBSERVED — the twenty-seventh consecutive session**) · `reference_parallel_subagents_private_scratch` · `reference_parallel_agents_shared_machine_namespaces`.
