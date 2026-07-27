# PROGRESS 77 — runtime-static-layer

Companion to `PLAN.md`. The PLAN half below is written at the PLAN stage; the `# IMPL record` half is appended at the phase-77 IMPL.

---

# PLAN record (2026-07-27)

**Stage:** PLAN. Lifecycle-state **2 → 3**. **ROW 77 STAYS `in-progress`.**
**Base:** master `b2dba137`. Worktree `/home/esa/git/envoy-go-wt-p77plan`, branch `phase-77-plan`.
**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + `next-prompt.txt`.
**BYTE-UNTOUCHED at this stage:** `ROADMAP.md` · `BEHAVIOR_CONTRACT.md` · `DECISIONS.md` · **ZERO `.go` files.**

**Dispatch:** three agents on disjoint remits with **PRIVATE scratch** — one LIVE reference probe fleet (**43 fresh containers**, image id verified per boot), one Go-execution agent in a throwaway worktree (`/home/esa/git/envoy-go-wt-p77probe`, restored to an EMPTY porcelain), one read-only gate/anchor agent — plus four read-only surveys and controller re-derivation of every load-bearing claim.

## What was EXECUTED at this stage

| # | thing | outcome |
|---|---|---|
| 1 | The four-arm `layered_runtime` config on the pinned reference, **3 fresh boots** | `num_keys=6`, `num_layers=2`, identical 3/3 |
| 2 | Four **isolation** arms (A/B/C/D separately), 3 boots each | 1 / 2 / 1 / 2 — **they sum EXACTLY to 6** |
| 3 | Four **counterfactual** arms (no-terminator, capitalized, denominator-alone, top-level-empty) | 3 / 2 / 1 / 1 — every discriminator confirmed |
| 4 | Two **controls** (P-single, P-baseline) | 1/1 and 0/0 — gate is neither stuck-red nor stuck-constant |
| 5 | Go-side unmarshal of **nine** shapes with the pre-check lifted, + a vacuity control | control ERRORED ⇒ results valid; all nine recorded |
| 6 | A `flatten` **prototype** + its 14-row key-set roster | 6-key union CONFIRMED |
| 7 | **Four break-arm counterfactuals** on the prototype | **B and D COLLIDE at 4** — see the headline |
| 8 | `FuzzBootstrapLoad` prefix upgrade + `-fuzztime 30s` | **2,357,987 execs**, 369 corpus entries, ZERO violations |
| 9 | The fuzz assertion's **negative control** | two seeds RED on a `NEGCTRL:` prefix ⇒ the assertion is live |
| 10 | **Eleven gates**, each with its negative control | G1-G11; three new broken-gate shapes found |
| 11 | Identifier collisions by **AST**, not grep | `Snapshot` DECLARES ONCE |
| 12 | Sentinel check (1)'s blind spot, re-derived **and fired in the unsafe direction** | `stop` would be created with four not-done rows |
| 13 | Every documentary anchor in the SPEC's read-first list | one REFUTED, one stale cite found |

## THREE OF THIS CONTROLLER'S OWN PREDICTIONS WERE REFUTED

1. **The tuned per-arm key counts (`+1 / −1 / +2 / −2`) do not work.** They assume each break perturbs one arm. A broken recursion collapses the nested arm **and** the empty-struct arm at once, so B and D both land on 4. **No choice of counts separates them.**
2. **`Snapshot` is not collision-free.** `internal/dynamicmetadata/dynamicmetadata.go:74` declares it as a method. Benign by scope; the name is kept; the SPEC's `^type Snapshot` regex could not see it.
3. **The four counterfactuals do not give four distinct totals** — asked explicitly, answered NO.

## Findings that change what the IMPL must do

- **§1.1 — the break roster is SPLIT BY LAYER.** Six discriminating breaks move to the unit test (key sets); the fixture keeps three it alone can see (registration-absence, transposition, dispatch-vanish). **SPEC §8.3's instruction that a break be identifiable from the gauge is WITHDRAWN as impossible.** Two counterfactuals (case-insensitive, pair-match) do not move `num_keys` at all.
- **§1.2 — sentinel check (1) fails UNSAFE**, proved on five arms. A fixed field-parsed form with a denominator assertion was written and armed; **T12 installs it in `next-prompt.txt`.**
- **§1.5 — the oneof wrapper type names are asymmetric.** `RuntimeLayer_StaticLayer` has no trailing underscore; `_DiskLayer_`, `_AdminLayer_`, `_RtdsLayer_` do. The un-suffixed forms are the nested MESSAGE types, so a switch drafted from memory is an `impossible type switch case`.
- **§1.6 — an unset oneof takes `default`**, so `case nil` must be explicit; and `layered_runtime: {}` vs `{layers: []}` are **indistinguishable after unmarshal**, so arm 9 is one predicate covering two spellings.
- **§1.7 — the landed `countMetrics` stat-guard idiom is BLIND to a rename** (proved: two in, two out, both names wrong ⇒ PASS). **T5 ships a name-set guard.**
- **§1.9 — `BEHAVIOR_CONTRACT.md:1857`'s two stale cites are REAL**, both drifted **+20** (real sites `manager.go:1098` and `:1064`; the first cite lands on the **bind-failure unwind block**). ⚠️ **This stage's first pass wrongly REFUTED them and STRUCK the deferral; the strike is WITHDRAWN.** See the second self-correction below.
- **§1.10 — the `1200 → 1201` ledger step is documented nowhere in `BEHAVIOR_CONTRACT.md`.** Assert the delta; the absolute rides an unaudited gap.

## Broken-gate count: EIGHT → ELEVEN

| # | gate | defect |
|---|---|---|
| 9 | sha256 roster, naive `[ -f ] || continue` | a DELETED file exits 0 — invisible |
| 10 | `countMetrics` stat delta | blind to a rename at constant count |
| 11 | `+0 exported symbols` over an EMPTY package | a stray newline vs a 0-byte baseline ⇒ RED on a correct tree |

Plus one **sharpened**: the `go doc` fail-open is not *"`./` prefix"* (`./nonexistent` alone exits 1 — REFUTED); it is that **with a valid arg1, ANY arg2 is silently swallowed**.

## Counts at this close — re-derived MECHANICALLY, never copied

fixtures **119** (tail `0117`, next-free **0118**) · fuzzers **55** · internal packages **73** · blank imports in `runner_test.go` **119** · inline `fmt.Errorf("bootstrap: …")` arms **47**, all in `bootstrap.go` · byte-gate roster **977** (universe 990 − edit 13) · stat surface **1205** (DOCUMENTARY — no mechanical command, **two** ledger gaps) · BackendKind **tail 38 / 39 declared** · go.mod modules **2** (lineage figure; the single `go.mod` requires 67) · DECISIONS tail **ADR-0299 PROPOSED** (next-free **ADR-0300**) · reference port for `0118`: **10118**, verified free.

## Sentinel — re-run MECHANICALLY at session open. It does NOT fire; `stop` was NOT created.

- **(1)** `NOT DONE: row 77`
- **(2)** **5** broadened (`:187 :197 :207 :213` long form + **`:221`** short form) / **4** old matcher. ⚠️ **Both correct; do NOT "fix" either down.**
- **(3)** `NEVER OPENED: gRPC`, `NEVER OPENED: WASM`. `Runtime` legitimately cleared by row 77.

`ROADMAP.md` is byte-untouched at this stage, so one run at session open suffices. Blind spot re-derived independently: **109 data rows / 105 matched / four misses** at `:31`, `:35`, `:83`, `:84`, all `done`, row 77 among the MATCHED — **and fired in the unsafe direction for the first time (§1.2).**

## Task status

| task | status |
|---|---|
| T1 `internal/runtime` flatten, BOTH branches | **PLANNED** — prototype RAN, 14-row roster drafted |
| T2 `Snapshot` UNION | **PLANNED** — prototype RAN, 6/2 reference-measured |
| T3 nine reject constants + roster-size guard | **PLANNED** |
| T4 the LIFT + roster + both test sites, **ATOMIC** | **PLANNED** — all nine arms' Go-side behavior EXECUTED |
| T5 two gauges + NAME-SET delta guard | **PLANNED** — guard shape RAN on a scratch harness, 5 red arms |
| T6 fuzz seed + prefix invariant | **PLANNED** — the change itself RAN, 2.36M execs + neg control |
| T7 fixture `0118` on `10118` | **PLANNED** — reference side MEASURED |
| T8 break roster | **PLANNED** — U1-U4 RAN as prototypes; U5/U6, R*, F1-F5 at the IMPL |
| T9 `BEHAVIOR_CONTRACT.md` sweep | **PLANNED** — anchors re-derived; the `+20` step STRUCK |
| T10 `DECISIONS.md` prose | **PLANNED** — ADR-0268 `:16430`, ADR-0278 `:16672` |
| T11 gates | **PLANNED** — G1-G5, G10-partial RAN here |
| T12 ADR-0299 + row 77 → `done` + the fixed check (1) | **PLANNED** |

## TWO controller self-corrections, recorded rather than quietly amended

### (1) The four-arm delta design

This stage's first design tuned the four fixture arms to give breakage deltas of `+1 / −1 / +2 / −2` and asserted, in the PLAN's own headline, that the SPEC's discrimination requirement was thereby satisfied. **The prototype run refuted it in the same session.** The error was not arithmetic — the arithmetic was right for the model. The model was wrong: it treated the four arms as independently breakable when two of them are reached through the same recursive descent. **The correction is architectural, not numerical**, and it is why the roster moved to the unit layer. A design that survives only because nobody ran it is not a design.

### (2) A published refutation that was itself wrong — caught AFTER the commit landed

§1.9 originally reported `BEHAVIOR_CONTRACT.md:1857`'s two stale cites as **REFUTED — "no such anchor exists"** — and STRUCK the deferral. **That was wrong, and it shipped.** The probe was asked for *"the two stale `+20` cites"* and **grepped for the literal string `+20`**; but `+20` in the claim is **the SIZE OF A DRIFT, not a token in the document**. Resolving each cite against the code it names shows both have drifted **+20** — `manager.go:1078-1082` is cited as the accept-loop `kindQUIC` `continue` and is actually the **bind-failure unwind block** (real site `:1098`), and `:1044-1054` is a doc comment (real site `:1064`).

**Why it got through:** the refutation arrived with commands and outputs attached, which is what made it persuasive, and is not what makes it true. `reference_probe_must_discriminate` was satisfied — the probe discriminated perfectly between *"the string `+20` is present"* and *"it is absent"*. **It was the wrong pair of hypotheses.**

**The transferable rule: a REFUTATION MUST ANSWER THE CLAIM AS STATED.** Grepping a literal token does not test a claim about drift; a range check does not test a claim about content. And an agent's confident refutation is exactly as much a claim as the thing it refutes.

**Disposition:** the deferral is REINSTATED as §5 item 15, T9 Step 4 now fixes the cites with **symbol anchors**, and the *"plus two more in the same file"* half is recorded as **UNVERIFIED** rather than resolved in either direction.

## Worktree / discipline notes

⚠️ **The Bash cwd reset fired AGAIN — the TENTH consecutive session** (`Shell cwd was reset to /home/esa/git/envoy-go` immediately after a `cd` into scratch). Every git command in this session used `git -C <abs-worktree-path>`.

The Go-execution agent's throwaway worktree `/home/esa/git/envoy-go-wt-p77probe` was left at an **EMPTY porcelain** with the `bootstrap.go:568-570` pre-check restored verbatim, nothing committed, nothing pushed. Remove it before the IMPL opens.
# IMPL record (2026-07-27)

**Stage:** phase-77 IMPL. Lifecycle-state **3 → DONE**. Worktree `/home/esa/git/envoy-go-wt-p77impl`, branch `phase-77-impl`, off master `9003de01`. Five subagent dispatches over the T1-T12 spine; subagents committed LOCALLY only; controller squashed at close.

## Sentinel — re-run MECHANICALLY at session open. Does NOT fire; `stop` NOT created.
- **(1)** `NOT DONE: row 77`
- **(2)** **5** broadened (`:187 :197 :207 :213` long form + `:221` short form) / **4** old matcher — both correct.
- **(3)** `NEVER OPENED: gRPC` / `NEVER OPENED: WASM`

## Commits (pre-squash)
| # | SHA | subject |
|---|---|---|
| T1 | `5bf79d64` | internal/runtime flatten with BOTH termination branches |
| T2 | `ba7979b3` | Snapshot UNION across layers, reference-measured at 6 keys / 2 layers |
| T3 | `53c02817` | nine layered_runtime reject constants + the wasm-style roster-size guard |
| T4 | `697f93a3` | the LIFT + the nine-arm roster + both test sites, ATOMICALLY |
| T5 | `a98e48fd` | runtime.num_keys + runtime.num_layers gauges with a NAME-SET delta guard |
| T6 | `92814abb` | a layered_runtime fuzz seed, and the prefix invariant FuzzBootstrapLoad only claimed |
| T9 | `8320f2c9` | BEHAVIOR_CONTRACT sweep |
| T10 | `8dded410` | ADR-0268's falsified validate roster + ADR-0278's stale pre-check cite |

## ⚠️ THE HEADLINE — THE PLAN'S U6 "STAYS AT 6" CLAIM IS REFUTED BY EXECUTION

PLAN §1.1 asserts that two counterfactuals — a case-INSENSITIVE match and a PAIR match — *"both leave `num_keys` at 6 on the shipped config and change only the key SET"*, and Task 8 line 1966 escalates that into a **declared MUST-STAY-GREEN for the fixture under U5 and U6**.

**MEASURED at T1: U5 holds, U6 does not.** Under a pair-matching `flatten`, the shipped config yields `NumKeys() = 8`, key set `[emp.e1 emp.e2 frac.bar frac.foo frac.numerator nest.mid.leaf1 nest.mid.leaf2 ov.key]`.

**The arithmetic is on the PLAN's own config.** Shipped arm C is `frac: {numerator: 25, foo: 2, bar: 3}` — **`numerator` ALONE, no `denominator`** — so a pair-match rule does not terminate there and recurses into 3 keys: 1(A) + 2(B) + **3**(C) + 2(D) = **8**.

**The error is a category confusion between a CONFIG counterfactual and an IMPLEMENTATION counterfactual.** The reference probe measured config arms (`C-capitalized {Numerator,Denominator}` ⇒ 2, `C-denominator-alone {denominator,foo}` ⇒ 1) against a fixed correct implementation. U5/U6 are the opposite: a fixed config against a broken implementation. A probe over the first cannot answer a question about the second, and the two were treated as interchangeable.

⚠️ **§1.1's ARCHITECTURAL CONCLUSION SURVIVES INTACT** — U2 and U3 still both yield 4 with different key sets, so `num_keys` still cannot identify which arm broke and the roster still belongs at the unit layer. Only the specific "U6 is invisible to the gauge" premise falls. The fixture is a **stronger** detector than the PLAN credited it with being, not a weaker one.

## Task 1 — ACTUAL
Red: `undefined: flatten`, build failed, EXIT=1. Green: 14 subtests PASS. Step-5 non-vacuity fired (`empty root: got [], want exactly one empty-string key`) and restored green. `doc.go` deleted via `git rm`; `go list ./internal/... | wc -l` still **73**. `go list -deps ./internal/runtime | grep -c 'pgdad/envoy-go'` ⇒ **1** (itself) — the cycle guard holds. Exported surface **0 → 4** exactly as forecast.

**PLAN discrepancies:** (i) the PLAN's `snapshot_test.go` block **fails `golangci-lint`** — `misspell` rejects `behaviour` (`snapshot_test.go:106:21`); fixed to `behavior`. (ii) The PLAN's U1 predicted message names the per-index leg (`key[0] = "frac.bar"`); the actual failure is the **length-mismatch leg**, which runs first and `return`s — exactly as the PLAN's own Step-1 note describes, so the prediction contradicts the design one page earlier. (iii) Task 2 Step 3 says `import "sort" // add to the existing import block`; T1 ships a single-line import, so a 1→N conversion was needed.

## Task 2 — ACTUAL
Red: `undefined: NewSnapshot` at 5 sites. Green: all subtests. `TestSnapshot_KeysIsACopy` proved non-vacuous (aliased-slice red, then restored). Green under `-race`.

### Breaks U1-U6 — ALL SIX FIRED
| break | subtest that fired | combined total | matched? |
|---|---|---|---|
| U1 delete lexical branch | `ArmC_NumeratorTerminates` (+3) | 6 → **8** | y |
| U2 delete `len(fields)==0` | `ArmD_EmptyStructsAreLeaves` | 6 → **4** | y |
| U3 emit at depth 1 | `ArmB_NestedTwoLeaves` | 6 → **4** | y |
| U4 per-layer SUM | `TestSnapshot_OverlapCountsOnce` | 6 → **7** | y |
| U5 case-insensitive | `CaseSensitive_CapitalizedRecurses` only | **STAYS 6** ✅ | y |
| U6 pair match | `DenominatorAloneTerminates` (+2) | 6 → **8** ⚠️ | **REFUTED** |

**U2 vs U3 both total 4 with DIFFERENT key sets** — §1.1's central claim confirmed live, by the same run that refuted its U6 corollary.

## Task 3 — ACTUAL
**T3 landed SEPARATELY; the PLAN's unused-constant hazard did not fire.** `golangci-lint` on the T3-only tree (constants present, `parseLayeredRuntime` not yet written): `LINT_EXIT=0`, zero diagnostics — the `unused` checker is satisfied by same-package `_test.go` references. No suppression, no `_ =` sink. The PLAN's contingency was correct to include and went unused.

Step 5 (deleted the `Arm07` row): **confirmed the roster-size `Fatalf` fired, not a wording mismatch** — zero subtests ran. Step 6 (stripped the prefix from `parseRejectRtdsLayer`): **both** tests fired, as predicted.

## Task 4 — ACTUAL
### PLAN anchors — RE-DERIVED before use: **all seven MATCHED**
`// Load parses r as YAML` `:545` · `dynamic_resources` pre-check `:565-567` · `layered_runtime` pre-check `:568-570` · `ConfigPath string` `:542` · `parseStatsSinks` `:584` · `TestLoad_RejectsLayeredRuntime` `:82-96` · `TestBootstrap_ReusesLoad_RejectsLayeredRuntime` `:65-79`. No drift. The `dynamic_resources` pre-check is byte-untouched.

### Breaks R1-R10 + R-lift — ALL FIRED THEIR OWN ARM
Every neutralization reddened exactly its own row with the other nine green. **The arm-4 / arm-5 pair proved non-short-circuiting in BOTH directions** — neutralizing arm 5 leaves `Arm04` green and vice versa. R9/R10 red **together** with the other eight green, which is the correct outcome for a 10-row/9-arm roster and positively confirms the two spellings share one predicate. **R-lift fired**: restoring the pre-check reddens `TestLoad_AcceptsStaticLayer` with the old wholesale message — the one break that proves the row does what it says.

**PLAN discrepancy:** Task 4 Step 2 predicts ONE `validate` red; **both** new `validate` tests go red, because the old wholesale message does not contain the substring `rtds_layer`. A stronger red than advertised.

## Task 5 — ACTUAL
Red exactly as predicted (`runtime.* name set = [] (0), want [...] (2)` + `not registered` on all three value rows). All three value rows (0/0, 1/1, 6/2) passed on the **first** implementation. Implemented via a `setRuntimeStats(result)` helper at BOTH success returns — no `defer`.

### Breaks S1-S5 — ALL FIVE FIRED
S3 (`runtime.numLayers`) and S4 (both renamed to `runtime.keys`/`runtime.layers`) fire on **name mismatch with the count still 2** — the exact arm a `countMetrics` count guard PASSES. That is the discrimination the task exists for.

`NamePattern` negative control: `runtime.num_keys`/`runtime.num_layers` ⇒ true; `runtime.` and `runtime.num-keys` ⇒ false.

**PLAN discrepancies:** Task 5 Step 1 lists `internal/stats` among `bootstrap_test.go`'s existing imports — it was **not** imported at this tip; both it and `"sort"` were added. The PLAN gives no body for `containsName`/`gaugeValueByName`; written to spec (`Load()` is the correct Gauge read method).

## Task 6 — ACTUAL
Seed corpus **9 seeds, all PASS**. `-fuzztime 30s`: **661,639 execs, 240 new interesting (total 610), ZERO violations, NO crasher** — `internal/bootstrap/testdata` still does not exist. **The nine new reject arms all carry the `bootstrap: ` prefix under mutation.** NEGCTRL fired (seed#2, seed#3 red on `NEGCTRL: empty document`); restored, residue 0. `^func Fuzz` ⇒ **55**, control ⇒ 56, back to 55. **Fuzz delta +0.**

**PLAN discrepancy:** the PLAN cites the fuzz body at both `:78-81` and `:77-81` in adjacent paragraphs; the actual `f.Fuzz(` opens at `:77`.

### ⚠️ A PROCESS FINDING THAT EXTENDS `reference_break_protocol_commit_first`
While running the T6 **fuzzer-count negative control**, the agent appended a throwaway `func FuzzProbeOnly` and removed it with `git restore` — **which wiped the still-uncommitted T6 edits.** The memory frames the commit-first rule around *deliberate breaks*; this was a *negative control*, and it has the identical hazard. **Any `git restore`-based probe must run on a committed tree.** The edits were re-applied, re-verified and committed, and both negative controls were then re-run post-commit; the recorded results are the post-commit ones.

## Task 9 — ACTUAL
Three "rejects still stand" sites amended — `:906` reads `still stand` while `:926`/`:968` read `STILL stand`, so a case-sensitive grep finds **one of three**, as warned. Phase 77 ledger line added after Phase 75 at `:5004` (`grep -n 'Phase 76'` ⇒ ZERO hits, confirmed). **Both ledger gaps NAMED, NEITHER back-filled.** `1205 → 1207` at `:831`/`:847`; the stale `115-dir` at `:847` fixed to `120-dir`.

**LINE-COUNT DELTA: `BEHAVIOR_CONTRACT.md` 5746 → 5748 (+2).** Every by-line citation into this file below `:5004` shifts by 2.

### The two drifted QUIC cites — CONFIRMED by resolving them against the code
`manager.go:1078-1082` is the **bind-failure UNWIND block** (`for j := 0; j < i; j++ { m.runtimes[j].closeBind() }`); the real accept-loop site is `:1096-1101`. `:1044-1054` is a string-builder tail plus `Start`'s **doc comment**; the real bind-loop site is `:1063-1074`. **Both drifted +20**, and the first lands on code that contradicts the claim. Replaced with **symbol anchors** naming the loop and the condition, plus a retirement note recording the old numbers. `quic.go:84-85`/`:109` verified still correct and left byte-untouched. **The settled parity determination itself is untouched.**

⚠️ **`:1078-1082` occurred TWICE in that paragraph** (the phase-74 statement and the phase-75 inheritance statement), not once. Both fixed.

⚠️ **The *"plus two more in the same file"* half is STILL UNRESOLVED** and is reported as such rather than closed. The claim never names which two. A mechanical spot-resolution of the file's ten fully-qualified `internal/…go:N` cites flagged two as suspicious (`internal/tls/config.go:272` lands on a bare `}`; `internal/filter/http/chain.go:19` lands on a blank line) — **not audited, not edited, deferral stands open.**

**PLAN discrepancy:** the PLAN predicts `grep -c '1205'` ⇒ **1** post-edit. It is **2** — Phase 75's own ledger line legitimately *ends* at 1205 and must not be rewritten. Reported rather than forced to match.

## Task 10 — ACTUAL
ADR-0268's roster amended in place, leading with what survives (the other three cases are untouched). ADR-0278's `bootstrap.go:499` replaced with a symbol anchor.

⚠️ **A SECOND STALE COPY THE PLAN DID NOT NAME: `bootstrap.go:499` appears TWICE.** ADR-0280 §Context re-uses ADR-0278's frame and carries an identical copy of the cite. **Fixing only the named occurrence would have left the stale cite live** — the same class as the doubled `:1078-1082` in T9, found in the same session. Both re-anchored.

**`ADR-0089` byte-untouched, PROVEN** — block `3514-3602`, SHA-256 identical before and after: `733c1ce8928f2bd8febdef982cb6ed45e2a0e158f6ce0934a3decd67eae225b4`.

**`DECISIONS.md` LINE-COUNT DELTA: +0** (17494 → 17494); all three edits in-line, and ADR-0089/0268/0278/0280/0299 headings re-verified at their pre-edit line numbers.

Two self-checks caught before commit: the new ledger line initially claimed *"`grep -n '1201'` returns only the two ledger lines"* — **a self-clearing sentinel**, since the line itself now matches (`reference_sentinel_matcher_string_self_clears`); and the two stats were written up as counters when they are **gauges**, `Set` in `Load`.

## Task 7 — ACTUAL

Fixture `0118-runtime-static-layer` on port **10118**, five-file shape, `BackendCount() = 1` (default `TCPEcho`, +0 BackendKinds). All three registration gates satisfied; blank imports **120**, equal to the fixture count. `10118` appears nowhere else in any `.go`, `.yaml`, or under `test/`.

**The live reference AGREES with the PLAN's measured numbers — `num_keys=6`, `num_layers=2`. No `want` was adjusted to match anything.**

### ⚠️ A REAL DEFECT THIS ROW IS THE FIRST TO EXPOSE — and the fixture-as-specified was RED because of it

The PLAN specified `/stats/prometheus` as the assertion endpoint on both sides. The first run came back RED with `subj: envoy_runtime_num_keys ABSENT`. Root-caused by execution rather than by adjusting the assertion:

| side | endpoint | result |
|---|---|---|
| reference | `/stats` | `runtime.num_keys: 6` · `runtime.num_layers: 2` |
| reference | `/stats/prometheus` | `envoy_runtime_num_keys{} 6` · `envoy_runtime_num_layers{} 2` |
| subject | `/stats` | `runtime.num_keys: 6` · `runtime.num_layers: 2` |
| subject | `/stats/prometheus` | **BOTH NAMES ABSENT** |

**The gauges are registered with the correct values; the Prometheus RENDERER drops them.** `internal/stats.ExtractTags` (`internal/stats/name.go`) dispatches on a fixed set of recognized top-level segments — `cluster.` / `http.` / `listener.` / `server.` / `wasm.` plus the SN9 `local_ratelimit` default arm — and returns `stats: name "runtime.num_keys" has no recognized top-level segment`. `internal/stats.WriteProm` (`internal/stats/prom.go`) then **silently `return`s** on any `flattenToProm` error, under the comment *"skip malformed names (defense-in-depth; should not occur)"*. **Nothing logs and nothing errors** — which is why the gap passed registration, the whole unit suite, `go vet` and `golangci-lint` alike. Corroborated two ways: the live cross-side probe, and a direct read of both call sites.

⚠️ **`runtime.` was never added to that dispatch when the gauges landed at T5, and row 77 is the FIRST row to register a stat under a top-level segment the projection does not know.** ⚠️ **Whether any PRE-EXISTING stat name is also dropped this way is UNVERIFIED — it was not audited and is reported as neither confirmed nor refuted.**

**NOT FIXED HERE, deliberately.** `internal/stats` is on this row's byte-untouched roster, its projection rules are governed by ADR-0061, and it carries its own name-mapping tests — a new rule is owed an ADR amendment and its own row.

**But the deferral is ENFORCED, not prose.** The fixture (1) reads its VALUES from the flat `/stats` — sound cross-side here, because both names carry no address and no dynamic segment, so the internal name is byte-identical on both sides and `reference_listener_stat_scope_cross_side_divergence` does not arise; and (2) PINS the asymmetry symmetrically in `assertPrometheusExpositionDeparture`: the reference MUST still publish both Prometheus names and the subject MUST still be missing both. **The day `internal/stats` learns `runtime.`, fixture 0118 goes RED on purpose**, with an error naming the follow-up. **A prose-only deferral would have rotted.**

Confirmed against the live reference: exactly **9** `runtime.*` names (`admin_overrides_active`, `deprecated_feature_seen_since_process_start`, `deprecated_feature_use`, `load_error`, `load_success`, `num_keys`, `num_layers`, `override_dir_exists`, `override_dir_not_exists`) vs envoy-go's **2**.

### Breaks F1-F5 + U5/U6-FIXTURE — ALL FIRED

| break | outcome | assertion that fired | matched? |
|---|---|---|---|
| **F1** delete `num_keys` registration | RED | `subj: runtime.num_keys ABSENT` — the ABSENT branch, not a value mismatch. ⚠️ The log line read `subj num_keys=0`, so a single-value lookup WOULD have read `0 == 0` and passed. **This is the vacuity the separate branch exists to prevent, observed live.** | y (endpoint wording differs) |
| **F2** transpose the two `Set` calls | RED | both value legs + 2 cross-side mismatches + **`subj: the two gauges look TRANSPOSED (num_keys=2 num_layers=6)`** | y |
| **F3** remove the blank import | RED | **`GATE FAIL: AssertStats did NOT run`** — ⚠️ *not* the predicted `selector matched NOTHING`. **BARE `-run`: `ok … 0.073s`, EXIT 0 — silently green.** | ⚠️ partially |
| **F4** rename `AssertStats`→`AssertStatsX` | RED | `GATE FAIL: AssertStats did NOT run`, **and the suite otherwise stayed fully GREEN** (`--- PASS`). **F4b** with the compile-time tripwire kept: build fails loudly. | y |
| **F5** `wantNumKeys` 6→5 | RED | **both** `ref … = 6, want 5` and `subj … = 6, want 5` — live on both sides | y |
| **U5** case-INSENSITIVE match | **GREEN** | none; `subj num_keys=6` | y — the confirmed half |
| **U6-FIXTURE** pair match | **RED** | `subj runtime.num_keys = 8, want 6` + `cross-side mismatch: ref=6 subj=8` | ⚠️ **the CORRECTION is confirmed; the PLAN is REFUTED** |

⚠️ **F3's PREDICTED GUARD IS THE WRONG ONE, and the correction matters.** The PLAN predicted `GATE FAIL: selector matched NOTHING`. `[no tests to run]` appears only when the fixture **DIRECTORY** is also absent. With the directory present, `discoverFixtures` still creates the subtest, the registry lookup misses, and the runner `t.Skipf`s — so `-run` matches, a test "runs", and the bare form exits **0 with `ok`**. **Both guards stay load-bearing, but it is the `AssertStats` guard that catches a missing blank import in the realistic case** — the case where someone adds a fixture directory and forgets the import.

### Task 7 — other PLAN discrepancies
`cmd/envoy-go` takes `-c`, not `-config`. The PLAN's driver draft used `net`/`time` without listing them in its import block. The `0110` `scrapeProm` clone needed a sibling `scrapeFlat` for the `/stats` path.

## Tasks 9 & 10 — see above. Task 11 — ACTUAL

**Every gate run AND negative-controlled at the IMPL tip `dc705035`.** A control observed at the PLAN is a citation, not evidence; all of these are this tip's.

| gate | GREEN arm | RED control | verdict |
|---|---|---|---|
| **G1** gofmt on OUTPUT | empty list | misformatted file ⇒ output-form EXIT 1, while the naive `gofmt -l … && echo` printed **`CHAINED_RHS_RAN`** with gofmt exit **0** — the inert-`&&` idiom re-reproduced | PASS |
| **G2** exported symbols, ONE pkg/invocation | `internal/runtime` **5** · `internal/bootstrap` **11** · `validate` **2** | `+type` ⇒ RED · `+func` ⇒ RED · **unexported addition ⇒ GREEN** (no false positive) · bad dir ⇒ exit 2 | PASS |
| **G2-hazard** empty-package gate | file-based lister: 0 bytes, EXIT 0 | the `printf '%s\n' "$cur"` idiom: 1 byte ⇒ **EXIT 1 on a correct tree** — the eleventh broken gate reproduced live | PASS (fix confirmed) |
| **G3** sha256 roster, FOUR legs | `universe=992 editroster=17 byte_gated=976`, `ok=976 missing=0 mismatch=0 desync=0` | all four fired **alone** — see below | PASS |
| **G6** `internal/runtime` is a leaf | `grep -c 'pgdad/envoy-go'` = **1** | a stray `internal/stats` import ⇒ **2** | PASS |
| **G7** go.mod untouched | `tidy -diff` 0 bytes; `git diff master` 0 bytes | fake `require` ⇒ tidy EXIT 1, requires 67 → **68** | PASS |
| **G8** count envelope | below | each control observed | PASS |
| **G9** full differential | 120/120 subtests, `comm -3` EMPTY | organic RED + a 0118 liveness break | PASS |
| **G-extra** `./internal/...` + `-race` | 70 ok / 0 FAIL, INNER_EXIT 0; `-race` on the 3 touched pkgs **292 PASS / 0 FAIL / 0 DATA RACE** | `NumKeys()→0` ⇒ 2 FAIL in `internal/runtime` + 2 in `internal/bootstrap` | PASS |

### G3's four legs — each fires ALONE
OK ⇒ EXIT 0 · MISMATCH (append a comment) ⇒ `[MISMATCH]` with both hashes, EXIT 1 · **MISSING (delete the file) ⇒ `[MISSING]`, EXIT 1 — while the naive `[ -f ] || continue` idiom exited 0 on the SAME deletion** · ROSTER-DESYNC, both directions (a roster entry under an edited prefix; a ghost entry in neither tree) ⇒ EXIT 1.

### G8 envelope — all with controls
fixtures **120** (`mkdir 0119-fake` ⇒ 121) · `discoverFixtures` predicate **120** · fuzzers **55** (`+FuzzProbeOnly` ⇒ 56) · internal packages **73** · BackendKind **tail 38 / 39 declared constants (0-38)** — ⚠️ a TAIL VALUE, NOT reconciled to 39 · blank imports **120**, `comm -3` against the fixture dirs EMPTY · go.mod **67 requires (18 direct + 49 indirect) in TWO blocks**; the lineage figure **2** is the phase-61.2 figure and is left as-is.

### G9 — the tally DERIVED FROM THE LOG, not from an exit code
**Run B (`-v`):** `--- PASS` **134** / `--- FAIL` **2** / `--- SKIP` **0** / panics **0**; `TestDifferential` subtests **120 — 119 PASS, 1 FAIL**; wall 403 s, **INNER_EXIT=1**. `comm -3` fixture dirs vs subtest names **EMPTY**, red-controlled (dropping 0118 ⇒ 1 difference). **0118's own line: `--- PASS: TestDifferential/0118-runtime-static-layer (1.54s)`**, and its **liveness break** (`NumKeys()→0`) reddens it — the fixture is live, not vacuous.

### The one failure — classified as the INDEXED startup flake, with the evidence
`--- FAIL: TestDifferential/0084-otlp-access-log` — `listen tcp 0.0.0.0:44017: bind: address already in use`, then `backend[0] not ready`. **Evidence, not assertion:** (1) the signature is verbatim the indexed full-suite startup flake, failing **before any assertion**, on an **ephemeral** backend port (44017), not a banded fixture port; (2) **the same fixture PASSED in Run A at the same tip** seven minutes earlier; (3) **isolate-re-run 3×: PASS, PASS, PASS**, guarded selector confirming it matched; (4) `0084` is untouched by this row. ⚠️ **`0061-lb-ring-hash` PASSED** — no finding there.

## ⚠️ BROKEN-GATE COUNT: ELEVEN → THIRTEEN, both new ones found by execution at this IMPL

- **TWELFTH — the PLAN's OWN G9 recipe is vacuous.** `go test … > full.log` **without `-v`** produces a **2-line** log, so `grep -c -- '--- PASS'` returns **0** and `grep -c -- '--- FAIL'` returns **0** *on a fully green run*. Proven directly against Run A's log. **A "0 FAIL" reading from that recipe is vacuous**, and it is the exact shape of the phase-76 defect the same section warns about — a tally that cannot be derived is worse than an exit code that lies. **`-v` is mandatory.**
- **THIRTEENTH — G3's roster is desynced BY CONSTRUCTION against a DELETED file.** A roster built from the tip's `git ls-files` alone **cannot represent** `internal/runtime/doc.go`, which this row deleted, so the DESYNC leg fires on a **correct** tree. The roster must span **both** trees (`ls-files` ∪ `ls-tree master`). Caught because the leg fired; a roster without a desync leg would have silently under-gated.

### Other PLAN §4 corrections at this tip
- **G2's `internal/runtime` baseline of 4 is WRONG — it is 5.** §4 itself enumerates five identifiers (`Snapshot`, `NewSnapshot`, `Keys`, `NumKeys`, `NumLayers`); an AST lister counts methods individually. `internal/bootstrap` **11** holds; **`validate` stays 2, so "+0 new PUBLIC surface" holds.**
- **G3's 990/13/977 are PLAN-tip figures; at this tip they are 992/17/976.**
- **G6's prose *"the whole `-deps` output is one line"* is FALSE here** — it is **110** lines now that `internal/runtime` pulls `structpb`'s transitive tree. The **gated** figure (`grep -c 'pgdad/envoy-go'` ⇒ 1) is what matters and is unaffected; only the note is stale.
- **G8's blank-import count needs the FULL anchor.** A naive `grep -c '^\t_ '` gives **126** — it catches two non-fixture blank imports and four `_ = …` statement lines. Anchor on `^\t_ "github.com/pgdad/envoy-go/test/fixtures/` ⇒ **120**.

## Task 12 — ACTUAL

ADR-0299 **PROPOSED → COMPLETE in place**, appended after the RETAINED italic footer; **no renumber, NO `---` separator**. Block shape `### Context` 1 / `### Decision` 1 / `### Consequences` 1 / retained footer 1 / in-block `^---$` **0**, with the **`awk`-range negative control over ADR-0298** returning 1/1 — proving the range discriminates rather than spanning the file. ⚠️ **A closing italic footer was drafted and then REMOVED**: it would have made `grep -cF '*(§Decision'` read **2** and failed the PLAN's own G11, and ADR-0298 carries no such line. **ADR-0299 carries no whole-file grep count**; every count in it is line-scoped or stated with no numeral.

Row 77 (`ROADMAP.md:139`) **`in-progress` → `done`**, the ONLY ROADMAP edit. Data rows still **109**, so the sentinel's `want=109` stands unbumped.

### Sentinel — RE-RUN AFTER the ROADMAP edit
- **(1) SILENT** — ⚠️ **and guarded against a false negative**: flipping row 139 back to `in-progress` on a scratch copy prints `NOT DONE: row 77`, so **the silence is genuine**. Check (1) closes for the fourth time in project history, using the FIXED field-parsed form installed at the PLAN.
- **(2) 5 broadened (`:187 :197 :207 :213` + `:221`) / 4 old matcher** — UNCHANGED, exactly as predicted. This row narrows nothing.
- **(3) `NEVER OPENED: gRPC` / `NEVER OPENED: WASM`** — `Runtime` legitimately gone.
⇒ **checks (2) and (3) still print ⇒ the sentinel does NOT fire; `stop` was NOT created and MUST NOT be.**
⚠️ **No sentinel matcher string leaked into `ROADMAP.md`** — verified mechanically, not by eye: check (2) still returns exactly its five known sites (none on row 139) and check (3) still reports gRPC/WASM, which a leaked `-family row` token would have silenced.

### Document line deltas at this close
`BEHAVIOR_CONTRACT.md` **5746 → 5750 (+4)** — T9's +2 plus the +2 prometheus-departure paragraph. ⚠️ **Every by-line citation into that file below its Phase 75 ledger entry shifts by 4.** `DECISIONS.md` **17494 → 17531 (+37)**, all of it ADR-0299's tail, so every ADR heading keeps its line number. `ROADMAP.md` **225 → 225 (+0)**, an in-line cell edit.
