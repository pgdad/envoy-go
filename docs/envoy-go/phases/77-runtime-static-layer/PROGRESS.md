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
