# PROGRESS 81 — stats-name-charset-guards

Stage-by-stage execution record. One section per lifecycle stage, appended at that stage's close.

---

# BRAINSTORM record (2026-07-31)

**Stage:** BRAINSTORM (lifecycle-state `DONE` → `1`). **Row 81 REGISTERED `in-progress`** at this commit. Sentinel `want` **112 → 113** in the same commit. Base master **`4f8e159c`** (phase-80 IMPL squash tip, located by SUBJECT). Worktree `/home/esa/git/envoy-go-wt-phase81`, branch `phase-81-brainstorm`. **SELF-PICKED** per the 2026-07-12 standing directive.

## What was EXECUTED at this stage

**Four costing agents in parallel**, all read-only in one shared worktree with private scratch dirs, none writing to the repo (`git status --porcelain` = 0 lines at every checkpoint). Controller re-derived every load-bearing claim itself rather than adopting a brief (`feedback_brief_citations_not_evidence`).

| agent | subject | outcome |
|---|---|---|
| A | dynamic-token charset exposure | **PICKED** — and found the request-time crash |
| B | `STATE_HISTORY.md` gap + `ROADMAP.md` malformed rows | rejected; 2 refutations + 2 new blockers |
| D | differential bind-retry + `--mode validate` | rejected; 5 refutations |
| E | gRPC + WASM family opens | rejected; 1 load-bearing refutation |

## ⚠️ THE HEADLINE: THE PICK RESTS ON A CRASH THIS STAGE PROVED, NOT INHERITED

`internal/rbac/perpolicy.go:25` lazily registers `<base>.policy.<policyName>.<suffix>` at **request time**, guarding `policyName` for emptiness only. `git grep -n 'IsValidName' -- internal/rbac/` returns **no output**. `internal/stats/registry.go:115-119` **panics** on an invalid name, and no `recover()` covers the request path.

Controller-run probe (written into `internal/rbac/`, executed, deleted; worktree verified clean afterwards):

```
NC OK: policy name "allow_admins" registered cleanly
CONFIRMED request-time PANIC: stats: invalid metric name:
  "http.myhcm.rbac.policy.allow-admins.allowed"
```

**The NC arm is what makes this a result** — the underscore spelling registers cleanly, so the probe discriminates on the hyphen alone.

## Refutation ledger — what EXECUTION found that `next-prompt.txt` got wrong

**Load-bearing (changed the pick or the scope):**

1. **`ROADMAP.md:76` does NOT decline a WASM row — a category error.** The quoted sentence is real (verified verbatim, and it occurs **exactly once** in the file) but says *"the FINAL §9 **HTTP-filters**-family row"*. `ROADMAP.md:218` is a **separate `### WASM host family` heading** (`:220` — *"Own multi-phase sub-project"*). A WASM-host row is not an HTTP-filters row. What `:76` correctly kills is the *rider*; a standalone row is untouched. ⇒ **WASM is live and defensible whenever it is smallest.**
2. **`--mode validate`'s defect runs the OPPOSITE way.** Not false-accept — **false-reject**, 3/3 against the pinned reference `envoyproxy/envoy:contrib-v1.37.2`. envoy-go `EXIT=1` on fixtures 0103/0108/0109; reference `configuration OK`, `EXIT=0`. Those three are live, passing differential fixtures.
3. **"The five existing rejects" is wrong by 3×** — there are **FIFTEEN** production `IsValidName` guard sites (controller-re-derived census in BRAINSTORM §2.2).
4. **The failure mode is a direct `panic`, not a nil counter.** `checkName` panics; registration never returns nil. `reference_nil_stats_counter_inc_crashes_goroutine` is the wrong mechanism for this surface — there is no nil to defend against.
5. **The bind-retry fix as prescribed cannot work.** All 26 backend arms spawn `go run`, so `cmd.Start()` returns `nil` on collision and the failure surfaces later at the child's `Listen`. A `startSubjectWithRetry` equivalent keys on a start error that never occurs.

**Bookkeeping / gate-shape (recorded, did not change the pick):**

6. **The archive gap is 57, not 58** (join key `(phase-id, STAGE)`, slug discarded). Two extractor traps cleared: slug drift (31 spurious misses) and **two distinct archive bullet shapes** (`STATE_HISTORY.md:428/:430` merge annotation and bullet on one line, inflating a naive count to 59). "36 for phases 67-75" **CONFIRMED exactly**. 57 is a **lower bound** — 87 pre-slug-convention subjects are invisible to the canonical extractor.
7. **The obvious archive guard SELF-CLEARS.** Without the `(slug)` parens, `phase 77 PLAN done` matches **`STATE.md`'s own confession prose** at `:18` and `:20`. `reference_sentinel_matcher_string_self_clears`, in a new file.
8. **CI is a shallow clone** — `actions/checkout@v6` with no `fetch-depth` ⇒ depth 1, killing any `git log`-derived expectation set. The git-free alternative was tested and **measurably fails** (380 stages from disk vs 209 from git).
9. **Naive `NF==8` also throws 15 FALSE POSITIVES**, not merely missing row 78. Unusable in both directions.
10. **`-count=6` is not a gate** — ~40 min to test for absence of a rare event; six clean runs occur with p ≈ 0.53 even if the bug is untouched.
11. **"close-then-rebind" is CURRENT, not stale** — the landed `f2dd994a` helper comment uses exactly that wording. What is stale is the ephemeral-range/loopback framing.
12. **`harness_test.go`'s "4 more" call sites contain ZERO backend arms.**
13. **A landed memory overgeneralized.** `reference_grep_c_zero_is_a_broken_gate` claimed GNU `grep -c` on a file list prints nothing. **False** — only `git grep -c` is silent; GNU/ugrep always print (`0`, or `path:0` per file). Both exit 1. **Memory corrected.**
14. **The router's `mongo collection names` hypothesis is REFUTED** (3 of 3 wire-derived tokens already guarded, denominator stated); its `wasm plugin names` hypothesis is **CONFIRMED**.

## Findings no phase-80 document carries

- **Production-dead trailer code** in four filters, incl. `admission_control`'s entire deferred gRPC-status state machine.
- **A vacuous differential arm** — `0036`'s `e_trailers_read` asserts `x-trailer-count=0` on a trailer-less GET.
- **Two stale non-test comments** claiming `RunDecodeTrailers` doesn't exist; it does, at `internal/filter/http/chain.go:455`.
- **`DECISIONS.md:13179`** enumerates 24 wasm module-function keys, **8 of which do not exist in `internal/`**.
- **Two sibling `freeTCPPort` helpers** still carry both pre-`f2dd994a` defects (`cmd/envoy-go/main_test.go:180`, `test/conformance/h2spec/h2spec_test.go:219`).
- **There is no doc-invariant test anywhere in the repo** — the 5 `_test.go` files naming these docs mention them in comments only.
- **A doctrinal contradiction to reconcile at the SPEC**: phase 80 says sanitizing is *"FORECLOSED by ADR-0065"*, yet `internal/listener/manager.go:352` `normalizeAddr` sanitizes.

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire

| check | ACTUAL output | NC, observed FIRING |
|---|---|---|
| **(1)** `want=112` | **NOTHING** | row **62** doctored ⇒ `NC NOT DONE: row 62` (`rows doctored: 1`); `want=111` ⇒ `NC GATE FAIL: examined 112 data rows, expected 111` |
| **(2)** | **FIVE** — `:190 :200 :210 :216 :224` (PRE-EDIT; ⚠️ **this row's own insertion shifted them to `:191 :201 :211 :217 :225` — the line-anchor-drift hazard firing on this stage's own document, caught only by re-running the gate on the FINAL tree**) | one-arm strip ⇒ **5 → 4, not 5 → 0** |
| **(3)** | `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` | invented slug fires; `Observability` correctly silent |

Input **228 lines / 112 data rows / 13** bare `candidates:` hits. **(2) and (3) print ⇒ the sentinel does NOT fire.** `ls stop` ⇒ `No such file or directory`; **NOT created**.

⚠️ **Check (1) is silent, so only a doctored-copy NC distinguishes it from a broken check. One was run and it fired.**
⚠️ **CHECK (2) UNCHANGED AT FIVE — THIS ROW NARROWS NOTHING, STATED NOT FORECAST.** Twenty-eighth consecutive phase.

**Leak check (row 81's cell only): PASSED.** 0 deferred-candidate phrases; one family slug, `Observability-family row`, already registered **52×** elsewhere ⇒ a use, not a mention.

**Row-81 well-formedness: the DISJUNCTION was required.** Over the live 112 rows, ARM-A flags **57, 69 only**; ARM-B flags **78 only** — independently reproduced by the controller. On a synthetic row 81 given **both** defects, **ARM-B fires, ARM-A stays silent, and naive `NF==8` PASSES** — `reference_compensating_defects_cancel_in_the_gate_metric`, on a row this stage authored.

## Counts re-derived at this tip

`ROADMAP.md` **228 → 229** lines / **112 → 113** data rows · `DECISIONS.md` **17724**, **301** ADR headings, tail **ADR-0302 COMPLETE**, next-free **ADR-0303** · `BEHAVIOR_CONTRACT.md` **5868** (untouched) · `STATE_HISTORY.md` **430 → 432** · fixtures **120** (naive predicate gives 118; next-free **0119**) · fuzzers **55** · internal packages **73** · blank imports **120** · **production `IsValidName` guard sites 15** · production stat-registration call sites **210 of 508** tree-wide.

**Normative cites re-verified EXACT**: `/BOOTSTRAP_PROMPT.md` §6.1 `:285` (`:289`/`:290` triggers, `:291` blank, `:292` third trigger); §7.5 `:357`, `(a)-(f)` `:360-365`, close `:367`.

## Six-gate (§7.5) — posture for a docs-only BRAINSTORM

(a)/(b)/(c)/(e) **not owed** — no `.go` and no fixture touched · (d) **VACUOUS**, said to be vacuous rather than green (55 fuzzers repo-wide, **0** added) · (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`; none since 25.3, 84 of 121 phase dirs carry none.** Recorded as a departure, not compliance.

## Hygiene

Docs-only: **ZERO production `.go`, ZERO test `.go`**. `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED** (`git diff --stat` shows neither). The §1 probe was deleted and the worktree verified clean before any commit.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED, OBSERVED THREE TIMES** (`Shell cwd was reset to /home/esa/git/envoy-go`). Twenty-fifth consecutive session. All git commands used `git -C <abs-path>`; branch tripwired as `phase-81-brainstorm`, never `master`.

**Broken-gate count stays EIGHTEEN** — no nineteenth shape. Two priors fired live: the compensating-defect cancellation (on this stage's own synthetic row) and the self-clearing matcher (in `STATE.md`, a new file for that shape).

## Next

**→ the phase-81 SPEC.** It must dispose **D-81-HELPER** first: a shared `internal/stats` helper makes the full 120-fixture differential **and** an explicit h2spec run mandatory for every later stage of this row; per-package inlining does not.

---

# SPEC record (2026-07-31)

**Stage:** SPEC (lifecycle-state `1` → `2`). **ROW 81 STAYS `in-progress`**; `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; sentinel `want` STAYS **113**. Base master **`aab596e4`** — from `git rev-parse master`, **not** the BRAINSTORM squash `3816a17f`, which sits two router-only commits below the tip. Worktree `/home/esa/git/envoy-go-wt/phase-81-spec`, branch `phase-81-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go`**.

⚠️ **THIS IS THE FIRST `# SPEC record` IN THE REPOSITORY.** Census over all 91 `PROGRESS.md` files across 122 phase dirs: `# IMPL record` **8** · `# PLAN record` **4** · `# BRAINSTORM record` **1** (phase 81's own, minted yesterday) · **`# SPEC record` 0**; the dominant shape is per-Task sections (**86** of 91). Recorded as a **new convention**, not as compliance with one.

## What was EXECUTED at this stage

**Five investigation agents in parallel**, each in its own DETACHED worktree with private scratch and a private port band inside `42000-42499`; the controller re-derived every load-bearing claim itself (`feedback_brief_citations_not_evidence`). Zero commits, zero pushes, zero branches; `git status --porcelain` = 0 lines reported 5/5.

| agent | remit | outcome |
|---|---|---|
| A1 | D-81-HELPER | **the fork's stated tradeoff does not exist** |
| A2 | D-81-F-SITE + the reference | **the BRAINSTORM's site is wrong; the reference accepts** |
| A3 | census + D-81-WORDING | ⚠️ **found the NINTH SOURCE** |
| A4 | D-81-SANITIZE + cost | **the contradiction dissolves; a THIRD fork named** |
| A5 | corpus survivability | ⚠️ **REFUTED — one proven hard RED** |

## ⚠️ THE HEADLINE: F IS TWO SOURCES, AND THE BRAINSTORM'S REMEDY COVERS ONE

`BuildMatcherEngine` (`internal/rbac/rbac.go:147-153`) does **only** `matcher.New(m, []string{actionTypeURL})` — a TypeURL allowlist. `Evaluate` at `:249`/`:251` returns `action.GetName()`, read out of the matcher tree's terminal `Action` proto **at request time**, and **nothing enumerates those names at boot**. Proved by execution with a firing NC:

```
BOOT ACCEPTED matcher Action.name="allow_admins"  → Inc registered cleanly     ← NC arm
BOOT ACCEPTED matcher Action.name="allow-admins"  → *** Inc PANICKED:
    stats: invalid metric name: "http.myhcm.rbac.policy.allow-admins.allowed"
```

⇒ **A matcher-based RBAC config still panics the process under any boot-only remedy.** The row's remedy is therefore two-part (§4 of the SPEC), and F1/F2 must land **atomically**.

## ⚠️ THE SECOND HEADLINE: THE REFERENCE ACCEPTS, AND THE REAL GAP IS **PROJECTION**

Controller-run, `envoyproxy/envoy:contrib-v1.37.2`, three arms torn down BY NAME:

| arm | `/stats` | `/stats/prometheus` |
|---|---|---|
| `allow-admins`, track=true | `http.myhcm.rbac.policy.allow-admins.allowed: 1` | `envoy_http_rbac_policy_allowed{envoy_rbac_policy_name="allow-admins",…} 1` |
| `allow_admins` (NC) | `…policy.allow_admins.allowed: 1` | `…{envoy_rbac_policy_name="allow_admins",…} 1` |
| track absent (control) | **empty** | **empty** |

The reference's metric NAME is **policy-name-independent**; the name is hoisted into a **label**, which has no charset to violate. envoy-go **flattens** it and has no `rbac_policy_name` extraction (0 hits; NC `envoy_http_conn_manager_prefix` ⇒ 2). ⇒ **any reject is an envoy-go-strict DEPARTURE, not a fix**, and the projection gap is a NEW deferred candidate.

## ⚠️ THE THIRD HEADLINE: THE CORPUS DOES NOT SURVIVE

`internal/filter/http/ratelimit/compiled_config_test.go:574` `StatPrefix: "tenant-foo"`, asserted at `:616-617`. `IsValidName("tenant-foo") = false`. Green today; guard **H** makes `:579`'s `t.Fatalf` live. ⚠️ **No differential run can find it** — `0032`/`0033` set only the HCM `stat_prefix`. BRAINSTORM §4.2's *"no existing fixture should red"* is narrowly true (fixture YAML **is** clean) and materially wrong as a corpus claim: the red is in a unit test, exactly where §4.2 did not look.

## The four D-questions disposed, plus TWO the BRAINSTORM never named

| question | disposition |
|---|---|
| **D-81-HELPER** | **INLINE** — the tradeoff does not exist: `go list -deps ./cmd/envoy-go` = 560 pkgs containing **8 of 8**; gate obligation identical; asymmetry **11.09 s** |
| **D-81-F-SITE** | **TWO PARTS** — a `trackPerRuleStats`-gated boot reject at `http/rbac buildCompiledConfig` (which also degrades correctly at the request-time per-route tier) **plus** a skip-and-log backstop at `PerPolicyCounters.Inc` |
| **D-81-SANITIZE** | **DISSOLVES ON SCOPE** — ADR-0065's rejection is a TWO-LIMB conjunction; `normalizeAddr` fails limb (ii); ADR-0065 §Decision cites it **approvingly**. All nine sources satisfy both limbs ⇒ reject |
| **D-81-WORDING** | eight byte-stable wordings, family-split preserved, identifier-collision-checked |
| **D-81-DEPTH** (NEW) | **table-driven-shared** — worth ~450-500 net lines and the entire split decision |
| **D-81-EMPTY** (NEW) | **skip-if-empty** — else 7 tests red and ADR-0132 §Decision (v) is contradicted |

## Refutation ledger — what EXECUTION found that the BRAINSTORM and router got wrong

**Load-bearing:**

1. **F is TWO sources; the count is NINE.** The boot-only remedy covers F1 only.
2. **The BRAINSTORM's F site is wrong.** `internal/rbac BuildRulesEngine` is shared (3 non-test call sites) and blind to `track_per_rule_stats`; network rbac never constructs `PerPolicyCounters` (`Inc` has exactly **2** non-test call sites, both in `http/rbac`). A guard there rejects configs that **cannot panic**.
3. **⚠️ The per-route tier compiles at REQUEST time** — `resolvePerRouteConfig` → `buildCompiledPerRoute` → `buildCompiledConfig`, and rbac registers **no `RegisterPerRouteValidator`** while 21 files repo-wide do. Recorded nowhere before.
4. **D-81-HELPER's tradeoff does not exist** (above), and the *"one definition, no drift"* doctrine cite is about the **regex source**, already shared. `registry.go:53-59` states the opposite intent verbatim.
5. **The corpus does not survive** — one hard RED, invisible to the differential.
6. **The reference accepts the hyphen verbatim**; the real gap is projection.
7. **D-81-SANITIZE dissolves.** ⇒ **ADR-0302 §Consequences (vii) item 5's *"internally inconsistent"* is PARTIALLY REFUTED** — it drops limb (ii) and the post-bind fact.
8. **The router's split axis is REFUTED.** 81.1-sources/81.2-retrofit is not a split; the retrofit is already out of scope, and re-badging it would park row 81 `in-progress` behind it (§6.2 items 4-5). The real axis is **F1/F2/D/E vs A/B/C/G/H**.

**Bookkeeping / gate-shape:**

9. **`BEHAVIOR_CONTRACT.md:931` is a scope over-read and a non-unique anchor** — **5** occurrences, all inside **stats-sink** subsections, and **13 `*-boot-reject` fixture dirs** exist, so it is not honoured tree-wide either. The conclusion survives on a far stronger, on-family citation: `0044-network-rbac-boot-reject/driver/driver.go:22-26`, which names *"invalid stat_prefix"* verbatim as a **subject-side-only reject covered by unit tests, NOT cross-side fixtures**.
10. **The retained-italic-footer count is EIGHT, not seven** — ADR-0294..0300 (the contiguous seven) **plus ADR-0302**. ADR-0301 carries none. And **ADR-0301's STATUS is miscounted a second way**: its *"seven blocks"* is right for 0294..0300 but the range it names, *"ADR-0295 through ADR-0300"*, is **SIX** — it drops ADR-0294.
11. **`~19` distinct token sources is REFUTED — there are 23.** 15 sites ⇒ **13** distinct guarded tokens (two duplicate guards); +8 unguarded = 21 `IsValidName`-class; **+2 protected by another mechanism** (a closed-set allowlist in `zookeeperproxy/stats.go`, and `normalizeAddr`, the tree's ONLY sanitizer) = 23. The coincidence `15+8=23` is accidental.
12. **The 15 split 11 REJECT / 4 SKIP**, and **all four SKIP sites sit on post-Freeze/request-time paths** — four landed precedents for the F2 backstop, a distinction the flat table erased.
13. **The BRAINSTORM's guard-census derivation carries a phantom exclusion** — *"minus `registry.go`'s own definition"*. `registry.go:60` is `func IsValidName(`, **unqualified**, so it never enters a `stats\.IsValidName` grep. Number right, arithmetic doesn't close.
14. **The router's SDS "misleading messages" cites point at the wrong files.** `internal/xds/stats.go` has **no error message at all**; the `"is not supported in phase 03"` strings live in `internal/tls/config.go:437`/`:454`, and `boot.go:161-162` only **quotes** one in a comment. ⚠️ A genuinely misleading charset message *does* exist elsewhere: `invalidSecretNameErrFmt` serves two legs with one charset-only wording — `"a..b"` is `IsValidName`-**VALID** yet rejected with *"must contain only ASCII letters…"*.
15. **The incumbent template's own rationale is wrong.** `network/rbac/rbac.go:105`'s *"a bare-prefix check would mis-accept"* measures **MIS-ACCEPTS = 0, OVER-REJECTS = 1** on a literal suffix. The correct rule is *"guard every variable segment"*. ⚠️ **And the assembled form is the WEAKER arm on the deferred defect** — it accepts `foo.` as `foo..rbac.allowed`, so row 81's guards inherit the interior-empty-segment hole **by construction**. The canonical anchor is **ADR-0065 §Consequences (b)**, not that comment.
16. **The four §4.2 rosters are wrong as rosters, right as properties** — **77** distinct fixture-YAML `stat_prefix` (not 111), **16**/**59** wasm plugin names (not 3), **22** RBAC policy keys (not 3). `text_optimized` stands.
17. **The empty-segment hole is INTERIOR-ONLY** — `a.`, `.a`, `..`, `""` are all already rejected. Cheaper than "well-formedness" implies. ⚠️ **And its target is GENERATED, not authored**: a config-token sweep for `..` finds **zero** across 1830 values and is structurally blind.
18. **`ST1005` is NOT enforced** (probed: `errors.New("Invalid stat prefix.")` ⇒ exit 0); `misspell` locale US **is** (NC fired). No enabled linter constrains error wording beyond spelling — **the SPEC does not claim a gate that does not exist**.
19. **`210` production registration sites is a grep count; the CODE-site count is 208** (2 comment lines in `internal/stats/doc.go:20,21`).
20. **`clusterName` is not a tenth source** — transitively guarded, proved via `ctx.ClusterManager.Get`'s `unknown cluster` reject.
21. **Phase 80's applicable multiplier is 1.97×, not 2.56×** — the 4.10× fixture bucket was **631 of 1636 (38%)** and phase 81 has **+0 fixtures**.
22. ⚠️ **THE CONTROLLER'S OWN DRAFTING BRIEF CARRIED A WRONG FAMILY ORDINAL, AND THE WRITER REFUTED IT.** The brief said this is the **TWENTY-FIFTH** §9 Observability-family row, on the premise that row 80 held the twenty-fourth. Re-derived from `ROADMAP.md`'s own ordinal cells at `aab596e4`, mapped row → ordinal: row 79 **TWENTY-SECOND**, row 80 **TWENTY-THIRD**, row 81 **TWENTY-FOURTH** — one ordinal per row, no gap, no duplicate, agreeing with what ADR-0301 and ADR-0302 each claim for themselves and with the ROADMAP row-81 cell. `grep -c 'TWENTY-FIFTH'` ⇒ **0** (NC `TWENTY-FOURTH` ⇒ 1). **ADR-0303 claims the TWENTY-FOURTH.** Recorded because a wrong ordinal asserted in an ADR is precisely the species that acquires authority by being landed (`reference_brainstorm_adjective_acquires_adr_authority`).
23. ⚠️ **AND THE SPEC'S OWN ROUTER PROSE WAS REFUTED BY ITS OWN COMMIT — CAUGHT POST-COMMIT, PRE-PUSH, BY RUNNING IT.** The rolled router asserted `git log --grep 'phase 81'` returns **2** at this tip. **It returns 4.** `git log` greps the **whole commit message**, and two router commits (`aab596e4`, `b66486a8`) discuss the matcher in their BODIES, so **the sentence warning about the grep satisfies the grep** — `reference_sentinel_matcher_string_self_clears` in a NEW carrier, a **commit body** rather than a document. The slug-anchored form `^phase 81 (stats-name-charset-guards)` returns exactly **2**, with a firing positive control on the completed phase 80 (**4** — IMPL/PLAN/SPEC/BRAINSTORM) and **0** on an invented slug. **The router now carries the anchored form.** This is the stage's twenty-third refutation and the only one whose target was the stage's own output.

## Findings no phase-81 document carried

- ⚠️ **A live cross-side stat-name divergence.** `http/rbac/rbac.go:500-506` `namespacePrefix` returns the literal `"rbac"` on an empty `rules_stat_prefix`, justified by a code comment asserting the C++ filter does the same. **Refuted by the reference itself** — an arm with no `rules_stat_prefix` emitted `http.myhcm.rbac.policy.allow-admins.allowed`, a **single** `rbac` segment. The differential cannot see it: `0018/envoy.yaml` sets an explicit prefix at `:107 :127 :247` and its header says so deliberately.
- ⚠️ **The compressor D5 collision.** ADR-0132 §Decision (v) + `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` pin a **deliberate, reference-probed** `compressor..gzip.` double dot. The deferred retrofit contradicts a landed ADR.
- **F and H have ZERO fixture coverage** — the differential can neither regress on nor cover the headline crash or the one proven red.
- **`guest-side-config`** (`wasm/compiled_config_test.go:1147`) looks like a violation and is not: an opaque `anypb` payload, not the outer `pc.GetName()`. Recorded so a sweep does not "fix" it.
- **Three incumbents validate the BARE prefix** (`redisproxy:50`, `thriftproxy:44`, `kafkabroker:61`) — **stronger**, not weaker, per refutation 15. Nothing owed on the merits.

## Cost

**Band ~850-1200 net `.go`, budget ~1000. Tasks ~14.** Three independent measurements reconciled: A1 ~30/source (~240), A2 **174 for F1 alone** (fully built), A4 850-1000 shared-depth / 1390-1500 per-source-deep. **The spread IS D-81-DEPTH.** On the chosen posture: F1 174 + F2 backstop ~50 + 7 × ~75 + shared audit ~140 ≈ **890**.

**Neither §6.1 trigger fires** — ~14 vs ~25 tasks (1.8×), ~1000 vs ~1500 net (1.25-1.75×). The third, mid-execution trigger must be **recorded if it fires, never absorbed** (it fired at phase 80).

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

| check | ACTUAL | NC, observed FIRING |
|---|---|---|
| **(1)** `want=113` | **`NOT DONE: row 81`** | `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`; row 81 doctored `done` ⇒ **SILENT**; row 62 doctored on that copy ⇒ `NOT DONE: row 62` |
| **(2)** | **FIVE** — `:191 :201 :211 :217 :225`, UNCHANGED | one-arm strip ⇒ **5 → 4, not 5 → 0** |
| **(3)** | `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` | invented slug fires; `Observability` silent |

Input **229 lines / 113 data rows**. `ls stop` ⇒ `No such file or directory`; **NOT created**. **`want` STAYS 113.**

⚠️ **THE LEAK CHECK IS INAPPLICABLE, NOT "PASSED"** — this SPEC writes no ROADMAP cell.
⚠️ **CHECK (2) UNCHANGED AT FIVE — THIS ROW NARROWS NOTHING, STATED NOT FORECAST.** Twenty-ninth consecutive phase.
⚠️ **Row well-formedness**: ARM-A flags **57, 69** only; ARM-B flags **78** only. Naive `NF==8` flags **SEVENTEEN**, of which **FIFTEEN are FALSE POSITIVES and TWO (57, 69) TRUE**, while row **78 is absent** — wrong in **both** directions, 15 FP + 1 FN. The router's parenthetical is the FLAG SET, not the FP set.

## Six-gate (§7.5, `/BOOTSTRAP_PROMPT.md` at the REPO ROOT — `:357`, `:360-365`, `:367` re-verified exact)

(a)/(b)/(c)/(e) **NOT OWED** — no fixture and no `.go` touched · (d) **VACUOUS**, said to be vacuous rather than green (**55** fuzzers repo-wide, **0** added; NC `^func FuzzZZ` ⇒ 0) · (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`; none since 25.3, 84 of 121 phase dirs carry none.** Recorded as a departure, not compliance.

## Hygiene

Docs-only: **ZERO production `.go`, ZERO test `.go`**. `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; `DECISIONS.md` gains **ADR-0303 §Context** only. Per-task `gofmt`/`golangci-lint` **not owed**.

All five agents' experimental edits reverted **by explicit path** with `sha256sum` byte-identity; **`git status --porcelain` = 0 lines reported 5/5**. Docker containers removed **BY NAME** (`p81ctl-hyphen`, `p81ctl-underscore`, `p81ctl-control`, plus A2's four) — never by an `ancestor=`/image filter; `docker ps -a --filter name=p81ctl` ⇒ **0**.

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED, OBSERVED** on the controller side. **Twenty-sixth consecutive session.** All git commands used `git -C <abs-path>`; branch tripwired `phase-81-spec`, never `master`.

**Broken-gate count stays EIGHTEEN** — no nineteenth shape, but **three priors fired live**: a `-run` no-match printing `[no tests to run]` and **exiting 0**; a negative control whose **own input was wrong** (a `.replace()` hit a comment, not the config — the arm read as "extractor blind" when the injection never landed); and `grep -c` on zero matches **printing `0` while exiting 1**.

## Next

**→ the phase-81 PLAN.** Enumerate against ~14 tasks / ~1000 net `.go`; hold **D-81-DEPTH** to table-driven-shared; schedule the `tenant-foo` edit; and **land F1 and F2 ATOMICALLY** — shipping the boot reject without the `Inc` backstop leaves the process crashable by the very config class the row exists to protect.

---

# PLAN record (2026-07-31)

**Stage:** PLAN (lifecycle-state `2` → 3). **Row 81 STAYS `in-progress`**; `ROADMAP.md`, `BEHAVIOR_CONTRACT.md` and `DECISIONS.md` **BYTE-UNTOUCHED**; sentinel `want` STAYS **113**. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.**

**Base:** master **`82e425ac`**, taken from `git rev-parse master` at session start. ⚠️ At this tip the SPEC squash **IS** the master tip, so the recurring "the router's quoted SHA sits below the tip" hazard did not bite — it was still re-derived rather than assumed. Worktree `/home/esa/git/envoy-go-wt/phase-81-plan`, branch `phase-81-plan`.

**File set:** `PLAN.md` (NEW) + `PROGRESS.md` + `STATE.md` + `STATE_HISTORY.md` + `next-prompt.txt` — **five files**, matching the phase-79/80 PLAN precedent (§Recent at its five-entry cap with an unarchived evictee).

## What was EXECUTED at this stage

**Five investigation agents on disjoint remits**, each in its own DETACHED worktree with private scratch and a private port band inside `42000-42499`, plus controller re-derivation of every load-bearing claim (`feedback_brief_citations_not_evidence` — **five briefs re-derived, four corrected**).

| agent | remit | headline |
|---|---|---|
| **A1** | sources A, B, H + the incumbent-template census | the assembled template makes **mongo strictly weaker than its three bare-guarded siblings** |
| **A2** | **the keystone** — F1 + F2 | **§2.8's "OVER-REJECTS = 1" does not transfer to F**; the SPEC's pasted panic name is the **reference's** |
| **A3** | sources C, D, E, G | **D-81-EMPTY's premise collapses**; four of §8's seven cited tests never reach the production entry point |
| **A4** | gates, blast radius, the full differential | a **SECOND corpus red**; the differential **ABORTS** and the notification reported success |
| **A5** | counts, ADR state, bookkeeping | **ADR-0045's mis-location charge is REFUTED**; `84 of 121` is stale |

**Zero commits, zero pushes, zero branches by any agent.** Every experimental edit reverted **by explicit path**; all five reported `git status --porcelain` = **0 lines**, controller-re-confirmed. No docker container created by any agent.

## ⚠️ THE HEADLINE: THE BARE-VS-ASSEMBLED QUESTION IS DECIDED BY **SEGMENT POSITION**, AND THE SPEC MEASURED IT AT THE DEGENERATE POSITION

SPEC §2.8 measures `network/rbac` and concludes **"MIS-ACCEPTS = 0, OVER-REJECTS = 1"**, then §3 promotes that template row-wide. **Three agents hit the wall independently, on three different sources.** Controller re-derivation, executed inside `internal/stats`:

| position | disagreements / 9 tokens |
|---|---|
| **LEADING** (`<tok>.zookeeper.decoder_error`) | **1** — only a trailing dot |
| **INTERIOR** (`http.myhcm.rbac.rbac.policy.<tok>.allowed`) | **5** — `0policy`, `policy.`, `.policy`, `9`, `""` |

**`network/rbac`'s token LEADS the assembled name — the one position where the answer is degenerate**, and that is exactly where "OVER-REJECTS = 1" comes from. ⚠️ **EIGHT OF THE NINE SOURCES ARE INTERIOR**; only B (zookeeper) is leading.

**DECISION: ASSEMBLED AT ALL NINE**, per ADR-0065 §Consequences (b) (`DECISIONS.md:2379`, verbatim-verified). A bare probe at an interior position would **boot-reject four config shapes that today boot, serve, and register a perfectly valid counter** — a regression in an availability row — and would make **F1 and F2 disagree**, since F2 can only probe the assembled key. **Two consequences recorded as deliberate**: the guards inherit the interior-empty-segment hole by construction (SPEC §13.1, now measured on four sources, with ACCEPT-pins left in the corpus as a failing-first anchor for the successor), and mongo ends up more permissive than its three bare-guarded siblings.

## ⚠️ THE SECOND HEADLINE: D-81-EMPTY'S PREMISE COLLAPSES

A3's three-arm cross-product, all executed: unconditional **BARE** ⇒ wasm `ok`, **rbac FAIL (26 top-level)**, compressor `ok`; unconditional **ASSEMBLED** ⇒ all `ok`; **skip-if-empty** ⇒ all `ok`. Under the assembled template an empty token yields a **valid** name at every one of C/D/E/G, and D/E never see an empty segment at all because `namespacePrefix("")` substitutes the literal `"rbac"` (controller-verified at `rbac.go:507`). ⇒ **§8 reasoned about a BARE token and applied the conclusion to an ASSEMBLED template.**

⚠️ **Four of §8's seven cited tests cannot be reddened by ANY production guard** — `TestNew_LibraryName_EmptyAllowed` calls a test-local `buildFromAny`, `TestStatsNamespace_LibraryNameEmpty_DoubleDotPath` calls `newFilterStats` directly, and both wasm rows call `registerPluginConfigName`/`unregisterPluginConfigName`. ⚠️ **And §8's H row is miscited** — controller-verified **ONE** bare `RateLimit{}` literal (`fuzz_test.go:380`), not two. **Skip-if-empty is KEPT for legibility, but is NOT load-bearing, and A/B omit it entirely** (their empty case is already rejected upstream).

## ⚠️ THE THIRD HEADLINE: A SECOND CORPUS RED, AND THE DIFFERENTIAL ABORTS

**`internal/filter/http/wasm/abi_callbacks_test.go:1453`** sets `&compiledConfig{pluginName: "my-plugin"}` — the **source-C field itself** (`compiled_config.go:444-447`), `IsValidName` = **false**. **AMBER, not hard red, and only because of guard placement**: the struct literal bypasses the `pc.GetName()` boundary. ⚠️ **It becomes a hard red the moment the guard relocates** — which §7's shared-driver disposition makes tempting. T4 pins the placement and records it.

**A4's first full differential run ABORTED at fixture 84 of 119** — `panic: … bind: address already in use` in `0083`'s own ALS receiver — and **35 fixtures silently never ran**, while the background-task notification reported *"completed (exit code 0)"* against an `INNER_EXIT=1`. Classified **KNOWN-LIVE FLAKE, third species** (out-of-band ephemeral port, driver-owned receiver, isolate re-run PASS, full re-run 120/120). ⚠️ **Budget ~2 launches per green pass.**

## Refutation ledger — what EXECUTION found that the SPEC and router got wrong

1. **§2.8's "OVER-REJECTS = 1" does not generalise** — decided by segment position. **Load-bearing: it decides all nine guards.**
2. **§8's D-81-EMPTY premise collapses** under the assembled template. **Load-bearing.**
3. **§8's seven-test roster** — four tests unreachable, the H row miscited, and a bare guard on D/E reds **26**, not 2.
4. **§7's single shared table is NOT CONSTRUCTIBLE.** Five of six guard targets are **package-private across five packages** (controller-verified); F1's and F2's tables are in different packages too. **The shared audit is ~7 per-package drivers. Load-bearing — it re-prices D-81-DEPTH.**
5. **§2.1's pasted panic name is the REFERENCE'S.** envoy-go emits `http.myhcm.rbac.**rbac**.policy.allow-admins.allowed` — **doubled**. Independently re-proves SPEC §13 item 3 **by execution**.
6. **§12's F2 "~50" is 96** (1.92×); F1's 174 confirmed at **178**; the "~75/source" model is wrong in both directions (D/E **38** each, C **99**, G **86**). **MEASURED TOTAL 725 — BELOW the SPEC's band floor of 850.**
7. **ADR-0045's "mis-location" charge is REFUTED** — `BOOTSTRAP_PROMPT.md:209` §5 state 2 genuinely carries the split gate at `:225-226`, and ADR-0045 cites **both** §6.1 and §5 state 2, **both accurately**.
8. **`84 of 121 phase dirs` is stale — it is 85 of 122** (37 with `REVIEW.md`, unchanged; newest 25.3).
9. **§9's `208-code-site / 84-file` is a MIXED DENOMINATOR** — 208 sites live in **36** production files; 84 is the production+test count (508 hits).
10. **§9's "1830 extracted values" is not reproducible** (four extractions: 5336 / 987 / 1202 / 2241), and **`http.test` has ZERO occurrences in `test/`** — it lives only in three packages outside the nine, while the fixture corpus carries **170** distinct dotted values. **The property holds on a stated denominator: 0 of 108.**
11. **§4.1's "21 files register a per-route validator" is a file-mention count** — the real figure is **FIVE filters** (`builtins.go:67-71`, verified against the live registry).
12. **§3's h2spec pricing overstates it ~100×** — **4 s** against the differential's **406 s** — and **the `./cmd/envoy-go` consumer set is THREE packages**, not two: `cmd/envoy-go/main_test.go` builds and boots the same binary at four sites and **can red on a new boot reject** (9 s; T11 adds it).
13. **§10's A=4 / B=3 count INSTANTIATIONS, not token values** — `0050` and `0047` **deliberately omit** `stat_prefix`. Value-supplying coverage is **A=3, B=2**.
14. **Two anchors drifted** (`namespacePrefix` decl `:507` not `:500-506`; `codec_test.go:517/:538` not `:518/:539`) — while the one SPEC §14 **warned** would drift, `DECISIONS.md:6291`, **did not**: the +42 lines were a tail append that cannot shift `:6291`.

## Findings no phase-81 document carries

`internal/filter/network/rbac/rbac.go:50` already contains the token **`F2`** as an unrelated phase-26 fork label — **do not use it as a grep anchor there** · **`/BOOTSTRAP_PROMPT.md` does not exist at the filesystem root**, and its second copy (`docs/superpowers/plans/2026-04-21-…`, **1024** lines vs **522**) shifts **all nine** cited anchors by **+197 for §6.x but +228 for §7.5** — no constant correction works · a **THIRD extractor trap** in `STATE_HISTORY.md`: naive `^- \*\*prior active-phase:\*\*` ⇒ **161**, parenthetical-tolerant ⇒ **167**, the **6** eviction-form bullets at `:420 :424 :428 :430 :432 :434` — with the naive anchor **phase 79 reads as entirely absent when all four of its bullets are present** · **`misspell` locale US fired LIVE twice** on first-draft guard prose · guard C needs a **new `internal/stats` import** and sits before a local `stats :=` shadow · the per-route wasm reject is **FAIL-OPEN**, not a boot failure · **F2's aggregated-log-dedupe assertion has NO in-tree precedent** — all four landed GUARD-SKIP sites are silent · **the archive-gap total of 57 is NOT reproducible** and **phase 76 is missing THREE bullets**, never called out — **no number is carried** · **the ALS-family driver port race** (`allocateALSPort` bind→close→re-bind) aborts the whole binary and is untracked.

## Cost

**MEASURED, not modelled** — every figure produced by building the guard and its tests, running them, and reading `git diff --numstat`:

| src | guard | test | net | | src | guard | test | net |
|---|---|---|---|---|---|---|---|---|
| A | 12 | 47 | **59** | | G | 11 | 75 | **86** |
| B | 12 | 47 | **59** | | F1 | 60 | 118 | **178** |
| H | 18 | 54 | **72** | | F2 | 22 | 74 | **96** |
| C | 12 | 87 | **99** | | **TOTAL** | **165** | **560** | **725** |
| D | 9 | 29 | **38** | | | | | |
| E | 9 | 29 | **38** | | | | | |

**BAND 750-1000, BUDGET ~850. TWELVE tasks. NEITHER §6.1 trigger fires** — **2.1×** margin on tasks, **1.5-2.0×** on LoC. ⚠️ **The figures are REALIZED-BASIS and must NOT be re-multiplied**; phase 80's 2.56× does not transfer (its fixture bucket was 631 of 1636 at 4.10×, and phase 81 adds **+0** fixtures). **NO SPLIT**; the SPEC's coherent axis (81.1 rbac / 81.2 filter-prefix) is carried forward unused.

## Sentinel — re-run MECHANICALLY at this stage. It does NOT fire; `stop` was NOT created

`ls stop` ⇒ `No such file or directory`. **It must not be created.**

- **(1)** `NOT DONE: row 81` at `want=113`. NCs all fired: `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`; row 81 doctored `done` ⇒ **SILENT**; row 62 doctored on that same copy ⇒ `NOT DONE: row 62`.
  ⚠️ **THE ROW-62 NC DID NOT LAND ON FIRST ATTEMPT** — a `sed` targeting `done      ` missed the real `| done |` spacing and printed nothing, reading exactly like *"the check is blind."* Caught by inspecting the doctored field before trusting it, then redone with `awk`. **`reference_gate_command_negative_control`, firing on the controller in the session's FIRST gate.**
- **(2)** **FIVE — `:191 :201 :211 :217 :225` — UNCHANGED. The THIRTIETH consecutive phase at which it did not go down. STATED, not forecast.** One-arm strip ⇒ **4**, not 0.
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NCs: invented slug fired; registered `Observability` correctly silent.
- Input measured **229 lines / 113 data rows**.

⚠️ **`want` STAYS 113.** ⚠️ **THE LEAK CHECK IS INAPPLICABLE, NOT "PASSED"** — this PLAN writes no ROADMAP cell, so the check has no input. **It re-arms at the IMPL.**

## Six-gate (§7.5, `BOOTSTRAP_PROMPT.md` at the REPO ROOT — `:357`, `:360-365`, `:367` re-verified exact)

⚠️ **`/BOOTSTRAP_PROMPT.md` with a leading slash resolves NOWHERE** — the file exists only at the repo root. For a docs-only PLAN the whole §7.5 set is **NOT OWED** (it is the *phase-done* gate; row 81 flips at the IMPL). Within it: (a)/(b)/(c)/(e) **not owed** — no fixture, no `.go`; (d) **VACUOUS**, said to be vacuous rather than green (**55** fuzzers tree-wide, **0** added; NC on a fuzzer-adding commit fires with 1); (f) ⚠️ **STANDING LINEAGE DEPARTURE — no `REVIEW.md`, 85 of 122 dirs carry none, none since 25.3.**

## Hygiene

⚠️ **`reference_bash_cwd_reset_commits_to_main` FIRED, OBSERVED** — `Shell cwd was reset to /home/esa/git/envoy-go`, repeatedly. **Twenty-seventh consecutive session.** Every git command used `git -C <abs-path>`; branch tripwired `phase-81-plan`, never `master`.

**Controller probes**: three `_test.go` files written into the PLAN worktree's `internal/stats`, run, and **deleted by explicit path**, with `git status --porcelain` ⇒ **0** verified after each.

**Broken-gate count stays EIGHTEEN** — no nineteenth shape, but **FIVE priors fired live**: the row-62 NC that did not land · a **build failure read as a zero result** (A3's first break arm orphaned an import, so `[build failed]` produced zero `--- FAIL:` lines and a count-based gate read **0**) · **a harness's exit code is not the command's** (the differential abort, notification said 0) · **a vacuous gate arm on a clean tree** (stat-surface ARM 1 is empty-in/empty-out; the real discriminator was a comment-only edit) · **`\t` in GNU ERE**, firing in the **opposite** direction from the documented warning — the harness `grep` **shell function** normalizes it and returns 126, while `command grep` returns 0, **so the trap bites exactly where gates run**.

**Flakes:** one fired — `0083-grpc-access-log-headers`, classified **known-live, third species**, with isolate-re-run PASS and full re-run 120/120. **None of the "recurrence is a FINDING" species fired.**

## Next

**→ the phase-81 IMPL.** Execute the twelve tasks against ~850 net `.go`. **Land T7 and T8 atomically**; **land T3's `tenant-foo` repair in the same commit as guard H**; **pin guard C at the `pc.GetName()` boundary**; and complete **ADR-0303 §Decision + §Consequences with the STATUS flip `PROPOSED` → `COMPLETE` in that same commit.**

---

# IMPL record (2026-07-31)

**Stage:** IMPL (lifecycle-state `3` → `DONE`). **ROW 81 FLIPPED `in-progress` → `done`; THE PHASE IS CLOSED.** Sentinel `want` STAYS **113** (this stage adds no row). Base master **`2a910ef2`** — taken from `git rev-parse master` at session start, not from any SHA quoted in the router. Worktree `/home/esa/git/envoy-go-wt/phase-81-impl`, branch `phase-81-impl`.

## What was EXECUTED at this stage

**Five implementation agents on disjoint package sets**, each in its own worktree off the same base, committing locally only and never pushing; the controller re-derived every load-bearing claim itself and integrated each branch by patch/cherry-pick with an `apply --check` gate first.

| agent | tasks | headline |
|---|---|---|
| **A1** | T1 mongo, T2 zookeeper, T9a the segment-position table | **at zookeeper the empty-check ORDER is load-bearing** — no phase-81 document says so |
| **A2** | T3 ratelimit + the corpus repair | **the `tok != ""` arm is a FIDELITY choice, not a safety gate**, proven by neutering it |
| **A3** | T4 wasm | **the `newFilterStats` relocation claim is FALSE**, and the struct-literal roster is **five** sites, not two |
| **A4** | T6 compressor | **the prescribed break arm is a VACUOUS BREAK MODE** — it cannot fire; substituted one that does |
| **A5** | T5 D+E, T7 F1, T8 F2 (the keystone) | **F2 caught live exactly what F1 dropped**, turning the atomicity argument into an observation |

All five reported `git status --porcelain` = **0 lines**, controller-re-confirmed. Zero pushes, zero branches beyond their own, zero docker containers created.

## ⚠️ THE HEADLINE: THE ROW'S CENTRAL DECISION WAS RE-DERIVED INDEPENDENTLY AND HELD EXACTLY, WHILE FIVE OF THE PLAN'S SUPPORTING CLAIMS DID NOT

The controller re-ran the segment-position cross-product inside `internal/stats` before dispatching anything: **LEADING disagrees on 1 of 9, INTERIOR on 5 of 9**, with the interior set exactly `0policy`, `policy.`, `.policy`, `9`, `""` — reproducing the PLAN's §1.1 measurement to the token. Every downstream `IsValidName` value the row depends on was confirmed in the same run: `a..b` **true**, `compressor..gzip.total` **true**, `tenant-foo` / `my-plugin` / `guest-side-config` all **false**, and mongo assembled `1abc` **true** against bare `1abc` **false**. **The decision to probe the assembled name at all nine sources is therefore executed fact at three independent levels** — controller, three agents at the PLAN, and five agents at the IMPL — and every guard shipped that way.

⚠️ **What did not survive is the PLAN's account of *why* several of the constraints hold.** The constraints' conclusions all stand; five of their stated reasons were wrong, and each was caught by executing the counterfactual rather than reading it. That is the stage's real yield.

## Refutation ledger — what EXECUTION found that the PLAN got wrong

**Load-bearing:**

1. ⚠️ **"Relocating guard C into `newFilterStats` turns `my-plugin` into a hard red" — FALSE.** `abi_callbacks_test.go:1453` sets `pluginName` by struct literal and reads it back through `GetProperty("plugin_name")`; it **never calls `newFilterStats`**. That relocation is the **vacuous** one. The two that genuinely redden are the other two the PLAN named. The pin at the `pc.GetName()` boundary stands; one of its three reasons is withdrawn.
2. ⚠️ **The wasm struct-literal `pluginName:` roster is FIVE sites, not two.** `body_test.go:73` and `trailers_test.go:38` are fed from a `pluginName string` **parameter** and are invisible to any literal-value grep, plus the production site. **The two omitted sites are precisely the ones that decide claim 1**, so the undercount was load-bearing.
3. ⚠️ **The compressor break arm the PLAN prescribed is a VACUOUS BREAK MODE.** "Drop the `tok != ""` skip so the guard rejects the empty name" cannot fire: with `tok == ""` the probe assembles to `…compressor..gzip.…`, an **interior** empty segment, which `NamePattern` accepts. Executed: `ok`, zero failures. Substituted a bare-token arm, which reds `TestRow81_CompressorD5NonRegression` specifically. `reference_vacuous_break_modes`.
4. ⚠️ **D-81-EMPTY is decorative at every INTERIOR source — confirmed three independent ways this stage** (ratelimit empty-arm neutered to `true` ⇒ package green; the compressor arm above; the rbac skip confirmed inert because `namespacePrefix("")` substitutes `"rbac"`). ⚠️ **But the one place it IS live is the opposite of where anyone looked: source B.** An empty zookeeper prefix assembles to `.zookeeper.<leaf>` — a **leading** dot, hence **invalid** — so B is safe without an empty arm **only because the pre-existing `errStatPrefixRequired` check runs first**. That ordering is now load-bearing, is pinned, and hoisting the charset guard above it would silently change `0047-zookeeper-boot-reject`'s error string. Source A has no such coupling.
5. ⚠️ **F2's aggregated-log-dedupe is NOT a precedent-free test shape, and the precedent the PLAN cited is not one.** `internal/stats/promskip_test.go` is an exact precedent — capture, flag/prefix reset, cleanup restore, non-empty-line count, must-not-parallelize note — with three more in the tree. Meanwhile the mongo `codec_test.go:517`/`:538` pair the PLAN called "the precedent for capture" are **guard-skip no-panic** precedents containing **no log capture at all**. A budget line and a test shape were both derived from that mis-citation.

**Structural and numeric:**

6. **The interior empty-segment hole covers LEADING dots and a bare `.`, not only trailing dots.** Every prior phase-81 document names only the trailing shape. Found twice independently (compressor, rbac). The prohibition bites only at a string boundary, which an interior token never occupies. Deferred candidate 1 is widened accordingly.
7. **The stat-surface arm split `143 + 67 = 210` is wrong.** The real disjoint split at this tip is **175 Counter + 35 Gauge = 210 raw**, → **174 + 34 = 208** code sites after the two `internal/stats/doc.go` comment lines (one in each arm). The **208 / 36** denominator itself is exact and unmoved by this row.
8. **The production edit surface is SEVEN files, not the nine PLAN §3 states.** §3 lists seven paths under a "9 files" heading.
9. **Break-arm magnitudes are understated ~5×.** F1 gutted reds **36 subtests + 5 top-level tests**, not "~7 charset subtests"; the gate arm reds **12** names, not "~7".
10. **`checkName` is at `registry.go:115-119`, not `113-118`** — `:113` is its doc comment. The same off-by-comment error the PLAN itself flagged for `namespacePrefix`, reproduced one paragraph later, and inherited into this stage's own briefs before an agent caught it.
11. **"Longest suffix" is irrelevant to correctness, and it is not always unique.** Every per-plugin and per-library suffix begins with a letter and ends alphanumeric, so all yield the identical verdict for any token; `NamePattern` has no length bound. `response.total_uncompressed_bytes` **ties at 33 bytes** with `response.content_length_too_small`. The wasm break arm proved it from the other side: the panic fired on `executions`, the **shortest** suffix and the first registered. Retained as a documentation convention, recorded as such at each site.
12. **The wasm "second registration path" was ALREADY fail-safe.** `dynamic.Registry.Register` pre-validates via `stats.IsValidName` and returns `ErrBadArgument`, with an in-code comment saying why. The guard covers it on a stronger argument than the PLAN's: both probes are **result-equivalent for every possible name**, not merely correlated.
13. **The mongo leaf roster is 22 suffixes, not 23** (the 32-byte longest is unchanged); the zookeeper roster is exactly **201** as claimed. Both re-derived by execution rather than eyeballed.
14. **Two incumbent guards carry stale cites** — `hcm/config.go:221`, `hcm/config.go:209`, `cluster/manager.go:205` are all wrong; the live sites are `internal/filter/hcm/config.go:267` and `internal/cluster/manager.go:419`, and `internal/filter/network/hcm/` does not exist. New comments were anchored on the probe **shape** instead.
15. **`stats.NamePattern` is exported**, so the hand-copied `statNameRegexLiteral` the lua package maintains (with its own drift test) is unnecessary; the new guards consume the exported constant, making drift structurally impossible rather than test-detected.
16. **The `STATE_HISTORY.md` tolerant-anchor count is 168, not the PLAN's 167** — that figure was measured at 434 lines, before the PLAN's own eviction append took the file to 436.
17. **`SPEC §13.1` has no literal heading.** §13 is at `SPEC.md:417` and the empty-segment hole is its **item 1**; the guards' comments cite it as such.

## ⚠️ THE COST: A MEASURED PROTOTYPE IS A LOWER BOUND, NOT AN ESTIMATE

| bucket | PLAN (measured prototypes) | REALIZED | ratio |
|---|---|---|---|
| production `.go` | 165 | **424** | 2.57× |
| test `.go` | 560 | **1812** | 3.24× |
| **net total** | **725** (budget ~850, band 750-1000) | **2229** | **3.07×** |

**16 files: 7 production, 9 test.** ⚠️ **THIS CROSSES BOOTSTRAP §6.1's `~1500` TRIGGER AND IS RECORDED AS A FINDING, NOT RETROACTIVELY SPLIT.** §6.1 gates the estimate the PLAN writes down; that estimate tripped neither threshold (12 tasks against ~25, ~850 against ~1500), so the split correctly did not fire at authoring time — the phase-80 precedent exactly. **This is the lineage's SECOND realized crossing, and the first in which the PLAN's estimate was itself measured-basis**: nine guards were built, run and read off `git diff --numstat` at the PLAN, and the row still ran 3× over them. The excess is almost entirely comment and regression-test volume — placement constraints, exemption rationales, accept-pin justifications the PLAN required at each site, plus regression arms for the assembled-probe decision that its task list did not enumerate. The coherent split axis (`81.1` = F1+F2+D+E, `81.2` = A+B+C+G+H) remains available and **unused**.

**Task count: TWELVE enumerated, TWELVE executed.** The §6.1 third trigger (mid-execution `>~10` sub-steps) did **not** fire.

## Sentinel — re-run MECHANICALLY after the ROADMAP flip. It does NOT fire; `stop` was NOT created

Input measured **229 lines / 113 data rows**, so an empty result cannot read as a zero result.

- **(1)** at `want=113`: **SILENT** — correct, every row is now `done`. ⚠️ **This is the one stage where silence is indistinguishable from a broken check, so the doctored-copy NC was MANDATORY and was run: row 62 flipped to `in-progress` on a scratch copy ⇒ `NOT DONE: row 62`.** The NC was **verified to have LANDED** by printing the doctored status field before trusting the result — the precise failure the PLAN hit when its `sed` missed the real column spacing. Denominator NC at `want=112` ⇒ `GATE FAIL: examined 113 data rows, expected 112`.
- **(2)** **FIVE — `:191 :201 :211 :217 :225` — UNCHANGED. The THIRTY-FIRST consecutive phase at which it did not go down. STATED, not forecast. This row narrows NOTHING.**
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. NC: an invented slug fires.
- ⇒ checks (2) and (3) both print, so **the sentinel does not fire**. `ls stop` ⇒ `No such file or directory`, and it was not created.

**⚠️ THE LEAK CHECK WAS ARMED AT THIS STAGE AND PASSED.** Row 81's cell alone carries **0** sentinel matcher phrases, and the only family slug it prints is `Observability-family row`, already registered elsewhere — a use, not a mention. The extractor was negative-controlled: injecting a second slug into a scratch copy made it print. Row well-formedness unchanged: **NF=8**, flagged by neither arm of the disjunction gate; `ROADMAP.md` stays **229 lines / 113 data rows**.

## Bookkeeping

`DECISIONS.md` **17766 → 17796**; **ADR-0303 `PROPOSED` → `COMPLETE` with its §Decision + §Consequences in the SAME commit**, appended in place after the **retained** italic footer, no renumber and no separator. Invariants held: `^---$` **216**, retained footers **NINE**, headings **302**, ids 0001-0303 with one gap at ADR-0209, next-free **ADR-0304**, STATUS census **15 COMPLETE + 1 PROPOSED → 16 COMPLETE**. The recurrence guard (*a `PROPOSED` STATUS must carry no `### Decision` heading*) is silent on the landed tree **and fires on a doctored copy of the POST-flip state**, so its silence is meaningful. The block carries **no whole-file grep count** — the ADR-0296/0302 self-falsifying species is avoided by enumerating per site.

`BEHAVIOR_CONTRACT.md` **5868 → 5870**. ⚠️ **The departure paragraph was RELOCATED before landing, for a reason worth recording.** Placed inline in the rbac stat subsection it would have shifted **nine** existing `BEHAVIOR_CONTRACT.md:<line>` citations by +2 — and every one of them lives in an **append-only or archived** document (`DECISIONS.md:17383`, `STATE_HISTORY.md`, historical phase records), so none could be repaired. All nine sit below line 5078; the file's own documented home for this material, `### Phase 16 forward-pointer notes`, sits at **5526** and is named by the rbac section itself. Landing there shifts **nothing** — verified mechanically against a stated denominator of **66** cites, zero at or above 5526, with `### Phase 16 forward-pointer notes` confirmed still at 5526 afterward. `reference_stale_cite_recurs_fix_by_pattern`, caught **before** landing rather than after.

`STATE_HISTORY.md` **436 → 438**: §Recent was at its five-entry cap, so **`phase 80 (stats-sds-projection) SPEC done`** — **RE-DERIVED as oldest at this tip, not inherited** — was moved **VERBATIM**, its absence verified beforehand (**0** hits) with **firing NCs on three phase-79 siblings** (1 each), the migrated body confirmed **byte-identical** by `sha256sum`, and **the identity check itself negative-controlled** (a deliberately doctored copy correctly mismatched).

## Gates — ACTUAL output

- **Full 120-fixture differential: `INNER_EXIT=0`, `--- PASS` = 120, FAIL/SKIP EMPTY, `no driver registered` = 0, `panic|DATA RACE|SIGSEGV` = 0, `comm -3` against the fixture directory EMPTY, 402 s** — inside SPEC §3's ~400-430 s band and within 4 s of the PLAN's 406 s baseline. ⚠️ **This was the THIRD launch; see the abort section below.**
- **h2spec**, run explicitly as a consumer **not** covered by `./test/differential/`: `INNER_EXIT=0`, **106 passed / 0 failed / 0 skipped** summed across the suite's reported groups, with the extractor negative-controlled (a synthetic `3 failed` line is seen). ⚠️ **The lineage's "53/53" figure is one half of that total** — the suite reports per-group counts and the run covers both arms; the method and denominator are stated here rather than the inherited number.
- **`go test ./cmd/envoy-go/ -count=1`**: `INNER_EXIT=0`, 8.7 s. **The THIRD consumer of the binary**, which boots real configs and can red on a new boot reject. Neither the SPEC nor the router names it.
- **Per-package** `go test -count=1` over the seven target packages plus `internal/rbac` and `internal/stats`: all `ok`. `-race`: all `ok` (nine packages, merged tree).
- **`gofmt -l`** over all 16 touched files: **EMPTY OUTPUT** (it never exits non-zero; the gate is on output).
- **`go vet`** over the eight packages: **0**.
- **`golangci-lint run`** (v1.64.8) over the eight packages: **exit 0**. ⚠️ **Negative-controlled on the MERGED tree** — a British spelling injected into a comment fired `misspell` at exit 1, the doctored line was grep-confirmed present before the run, and the file was restored **byte-identical to the committed blob**. An empty lint result is not evidence the linter looked.
- **`git diff go.mod go.sum`**: **0 lines**.
- **Stat surface +0, by CALL-SITE ENUMERATION** (the `TestNoNewStat*` guards were proven blind at the PLAN). ARM 1 prints nothing **against a 476-line production input diff with 410 added lines**, and the matcher was negative-controlled against a synthetic registration line (⇒ 1), so the empty result is a real zero rather than a broken matcher. ARM 2 unmoved at **208 code sites / 36 production files**. ⚠️ **Never 208/84.**

## ⚠️ THE DIFFERENTIAL ABORTED TWICE, AND THE NAMED DEFERRAL IS UNDERSTATED 4.7×

Both aborts were the **driver-owned receiver port race**, not a regression: the failing bind is the fixture driver's **own** receiver on an **out-of-band kernel-ephemeral** port (43109, then 45951 — outside `20000-31007`, `11000-14999`, and every static fixture range), reached through the driver's `allocate → read → close → re-bind` TOCTOU window. Neither hardened half of `reference_differential_fullsuite_startup_flake` fired; an in-band recurrence of either would still be a FINDING.

- **Run 1** — `INNER_EXIT=1`, aborted at **fixture 86 of 119** on `0085-otlp-access-log-operators`, **33 never ran**. ⚠️ **The harness notification reported "completed (exit code 0)".** `reference_harness_exit_code_is_not_command_exit_code`, **third consecutive instance**.
- **Isolate re-run of `0085`** — **PASS**, with the selector match asserted **positively** (a real `--- PASS:` line, `no tests to run` ⇒ 0).
- **Run 2** — `INNER_EXIT=1`, aborted at **fixture 84** on `0083-grpc-access-log-headers`, **35 never ran**.
- **Run 3 — CLEAN. 120/120, `INNER_EXIT=0`, zero panics, `comm -3` EMPTY, 402 s.** That is the run the gate rests on.

⚠️ **BUDGET CONSEQUENCE, CORRECTED UPWARD: the PLAN budgeted ~2 launches per green pass; this session needed THREE (~20 min of wall clock, plus a fourth short isolate run).** The observed abort rate was **2 in 3**, not 1 in 2.

⚠️ **The PLAN names this deferral as "the ALS-family driver port race (`0081`/`0082`/`0083`)". Enumerated at this tip, the shape spans FOURTEEN fixtures across THREE families** — gRPC access-log (`0081`-`0083`), OTLP access-log (`0084`, `0085`), and the tracing receivers (`0087`, `0102`, `0105`-`0107`, `0114`-`0117`) — all carrying a driver-owned `ensureServer`. **4.7× understated.** It aborts the whole test binary rather than failing one subtest, so a naive `--- PASS` tally reads as "a few failures" rather than as an abort.

⚠️ **A CONFOUND THIS SESSION HAD THAT THE PLAN DID NOT, RECORDED RATHER THAN ASSUMED AWAY:** two sibling Claude sessions were live on the same machine throughout, plus an unrelated long-running container, so ephemeral-port churn was higher than at the PLAN — a plausible reason the observed rate moved from 1-in-2 to 2-in-2. `reference_parallel_agents_shared_machine_namespaces`, at the level of **sessions** rather than agents. **No container was torn down**, by name or otherwise, because none was created by this session.

## Six-gate (§7.5, `BOOTSTRAP_PROMPT.md` at the REPO ROOT — `:357`, `:360-365`, `:367`)

- **(a) build + unit tests** — green, including `-race`.
- **(b) differential** — see above.
- **(c) conformance** — h2spec run explicitly as a distinct consumer; proxy-wasm **INHERITED, not re-run**, and said to be.
- **(d) fuzzers** — **VACUOUS**: this row adds none (**55** repo-wide, unchanged). Said to be vacuous, not green.
- **(e) lint/format/vet/modules** — green, with the lint gate negative-controlled.
- **(f) `REVIEW.md`** — **ABSENT. A STANDING LINEAGE DEPARTURE, recorded rather than claimed as compliance**: 85 of 122 phase directories carry none, and the last authored was 25.3. `PROGRESS.md` has discharged it for the whole recent lineage.

## Broken-gate count stays **EIGHTEEN** — no nineteenth shape, but FIVE priors fired live

1. **A vacuous break mode** — the compressor arm the PLAN prescribed cannot fire (ledger 3).
2. **A harness's exit code is not the command's** — reported success against `INNER_EXIT=1`, twice.
3. **An empty output is not a zero result** — driving both the stat-surface input measure and the lint negative control.
4. **A negative control must be verified to have LANDED** — the sentinel's doctored row-62 field was inspected before its result was trusted.
5. **`golangci-lint` runs `misspell` at `locale: US`** — fired on the merged tree under a deliberate control, and it covers `_test.go` files too.

## Carried forward, deliberately NOT fixed

The interior empty-segment hole (now widened to leading dots; **BLOCKED** on ADR-0132 §Decision (v); slug `stats-name-empty-segment-guards`) · the RBAC policy-name **PROJECTION** divergence · the HTTP-rbac empty-prefix fallback divergence, **upgraded to measured fact** by the doubled `rbac` segment in a live panic · the matcher-tree `Action`-name boot walk · the three bare-prefix incumbents, now a two-way divergence since mongo accepts a leading-digit prefix its siblings reject · the two-leg `invalidSecretNameErrFmt` wording · the `STATE_HISTORY.md` archive gap (**no total asserted**) · **the driver-owned receiver port race, now enumerated at fourteen fixtures** · `my-plugin` at `abi_callbacks_test.go:1453`, owed only if a later row relocates guard C · the lua package's hand-copied regex literal, unnecessary now that `NamePattern` is consumed directly · and the two stale incumbent guard cites (ledger 14).
