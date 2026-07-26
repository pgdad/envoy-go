# PROGRESS — phase 76 (`lb-ring-hash-spread-margin`)

> **This file records what ACTUALLY happened per task** — red-first verbatims, WHICH break assertion fired (and any that did NOT), every PLAN/SPEC claim refuted by execution, and every gate's observed negative control. It is written FROM execution, never in advance over a placeholder. A prediction recorded as an observation is the failure mode this file exists to prevent.

⚠️ **Scope note.** The phase-76 PLAN executed a large fraction of the row in isolated probe worktrees (`phase-76-plan-v1`, `phase-76-plan-v2`, local commits only, never pushed). Those commits are **NOT** in the PLAN's own commit — the PLAN's file set is `PLAN.md` + `PROGRESS.md` + `STATE.md` + `next-prompt.txt`. **The IMPL re-derives and re-lands the work at its own tip; it does not cherry-pick the probes.** Anchors drift within a phase's own tasks.

---

## Stage pointer

- **PLAN done** (2026-07-26) — lifecycle-state **2 → 3**. ROW 76 STAYS `in-progress`. ROADMAP and DECISIONS **BYTE-UNTOUCHED**.
- **IMPL** — *(not started)*

---

## Adversarial-pass record (PLAN §1)

*Written AFTER the pass, from what the agents and the controller actually found.*

**STATUS: COMPLETE.** Five agents on disjoint remits, private scratch, separate worktrees, plus controller re-derivation of every load-bearing claim.

| agent | remit | worktree | result |
|---|---|---|---|
| A | re-derive the whole edit-site roster read-only | `p76plan` (RO) | 8 disagreements with SPEC §10 |
| B | build + adversarially verify the measurement test | `p76plan-v1` | test BUILT/RUN/broken 3 ways; the linkage gate rebuilt |
| C | `sourceIPs` 4→16 + `driver_test.go` rescale + leg proofs | `p76plan-v2` | 6/6 legs proved by measurement; 16/16 binds |
| D | gates, ADR, contract line, format precedent | `p76plan` (RO) + scratch | 3 broken gates found, 2 of them NEW |
| E | break α + full suite at K=16 + `-race` + runtime delta | `p76plan-v2` | **all four discharged**; SPEC §3.6's runtime figure REFUTED |

### Findings that changed the PLAN

| # | source | what it caught |
|---|---|---|
| **S1** | D + controller | ⚠️ **The phase-76 SPEC's OWN linkage gate FAILS OPEN.** `[ "$a" = "$b" ]` with both captures empty is TRUE ⇒ **exit 0 on a tree where neither constant exists** (rename / move / delete). Controller-reproduced: `SPEC FORM: exit=0 GREEN with a='' b=''`. The document that warns *"a green gate is evidence only if you have seen it go red"* shipped a gate whose red arm was never fired. |
| **S2** | B | ⚠️ **The SAME gate ALSO fails CLOSED.** `grep -c 'collapseFixtureK'` ⇒ **2** — the second match is the measurement test's **own doc comment**. On a genuinely synced tree (16 vs 16) it printed `DESYNC: sourceIPs=16 collapseFixtureK=16` and exited 1. **Broken in both directions at once.** `reference_sentinel_matcher_string_self_clears`, live in a gate. |
| **S3** | C | ⚠️ **The naive ×4 rescale of the scatter tuple manufactures a THIRD vacuity.** `{20,28,16}×4 = {80,112,64}` — **all three multiples of 16** ⇒ `_ScatterBitesAffinity` would have silently migrated from the affinity leg to the spread leg while still printing `--- PASS`, converting the one test that was healthy into a broken one. No document flagged this. `{20,108,128}` used instead. |
| **S4** | D + controller | ⚠️ **`go doc -all <pkgA> <pkgB>`'s fail-open is specific to the `./` PREFIX** — this NARROWS the recorded finding. Bare paths exit **1** with `doc: no symbol …`; `./`-prefixed exit **0** silently dropping the second arg, **even when it names a directory that does not exist**. Phase 75 pinned the `./` form. |
| **S5** | D + controller | ⚠️ **`gofmt -l` NEVER exits non-zero** — observed `exit=0` while printing `internal/cluster/zz_bad.go`. Any six-gate chaining it with `&&` is **inert**. Gate on OUTPUT. |
| **S6** | D | ⚠️ **A THIRD defect in phase 75's `impblock`, unrecorded in the lineage.** Its awk matches only `^import \($`, so it emits ZERO lines for `driver_test.go:3` and `ringhash_test.go:3` (both single-line `import "testing"`) — **structurally blind to 2 of phase 76's 3 `.go` files**. Basename normalisation alone does not fix it. |
| **S7** | D | ⚠️ **The phase-75 sha256 roster CANNOT be copied forward.** `internal/cluster/**` was byte-gated at phase 75 and phase 76 **edits `internal/cluster/ringhash_test.go`**; `comm -12` returns exactly that file. Would reproduce the phase-73 SEVERE-1. |
| **M8** | A | **SPEC §10 README anchor `:58` is WRONG — it is `:57`**; `:58` contains no numeral at all. `:114` and `:135` (two `64`s) are MISSING from the roster entirely. |
| **M9** | A | **`driver_test.go`'s roster is short by SEVEN sites** (15, not 8) — and **`:43`/`:52` are executable `t.Fatal` STRING LITERALS**, not comments. |
| **M10** | C + controller | **The SPEC's vacuity MECHANISM is two-thirds right.** Affinity is evaluated INSIDE the per-element loop (`:284`), *before* conservation (`:291`) — so `_ScatterBitesAffinity` and `_SubjectConservation` were **never vacuous**. The SPEC's COUNT (exactly two, exactly the two it named) is right; its stated REASON over-generalises. |
| **M11** | B | **`3^(1−K)` OVERSTATES the real rate for this key set** — p̂/analytic **0.949** at K=4, **0.689** at K=8 over 2×10⁵ real ring draws; a random-key discriminator arm recovers 1.010/1.104, so the deficit belongs to the specific `xxHash64` key set, not the builder. Direction is **conservative**. ⚠️ **K=16 is NOT measured and cannot be at this scale** (expected count 0.014); `0/200000` bounds p̂ ≲ 1.5e-5 only. **7.0e-8 is analytic, not measured.** |
| **M12** | A | **SPEC §4's collision claim is FALSE for `allSame`** — 4 hits, 3 of them live Go in `internal/filter/http/router/hedge_test.go`. Safe by package scoping, but the stated evidence is wrong. Helper renamed `collapseAllSame` so the check is clean rather than explained. |
| **M13** | A | **`ringhash_test.go:3` is a single-line `import "testing"`** — a 1→N structural conversion, not an insertion. And the real import delta is `fmt`/`math`/`math/rand/v2`, **not** SPEC §4's `math/rand`+`strconv`. |
| **M14** | C | **`driver.go:80`'s trailing `// 64 — the conservation target` sits ON the line that now evaluates to 256** — a landmine directly on the edited const. |
| **M15** | C | **`README.md:81`'s `multinomial(64, 1/3)` and `{0,16,32,48,64}` MUST change.** SPEC §7's *"survives — do not delete"* invites leaving it byte-untouched. The **bound** survives (0.096% → 0.381%, still `< 1%`); the **numerals** do not. |
| **M16** | D | **`ROADMAP.md:138` (row 76) is BRAINSTORM-era and carries FOUR stale claims** the SPEC refuted (`4 → 10`, `K=10 → 5.1e-5`, β-as-single-edit *"THE PROOF"*, *"THE EXECUTABLE DELTA IS ONE INTEGER"*). |
| **M17** | D | **The router's *"the ADR-0288 singleton greps return 2"* holds for only ONE of three tokens.** Executed: `lifecycle-state` ⇒ **5**, `next-skill` ⇒ **2**, `next-free ADR` ⇒ **3**. The invariant that does hold is `STATE.md:7`'s own wording — exactly one **live** instance, in §Current. **Never "fix" these counts down.** |
| **M18** | A + controller | **`0062`/`0063`'s σ-band comment is at `:299`, not the SPEC's `:300`** — matters for the deferred widening. |
| **M19** | B | **The CONTROL count is 74/2000, not the SPEC's 71/2000** (different RNG construction). Deterministic under the seed; a documentation correction, not a flake. |
| **S20** | E | ⚠️ **SPEC §3.6's runtime cost is REFUTED.** Both arms measured in ONE session, 3 warm runs each: K=16 **4.410 s** wall / 3.6827 s reported vs K=4 **4.463 s** / 3.6997 s ⇒ delta **−0.053 s (−1.2%)**, **negative and smaller than the K=4 arm's own spread** (a single 4.59 s outlier drives the sign). ⇒ **no measurable cost from K=4 → K=16, and NOT a speedup either.** The SPEC's *"+0.158 s (+4.5%)"* must not be carried forward. ⚠️ **The SPEC's methodological RULE survives its own NUMBER** — *quote the delta against a same-session control* is exactly what produced this different answer. |
| **M21** | E | ⚠️ **NEW: the unit package DOES catch break α — by ONE test out of 403.** `TestRingHash_DistinctKeysSpread` fires (`ringhash_test.go:62: 200 distinct keys covered only 1 endpoints`) while **8 of the 9 `TestRingHash_*` tests PASS under a TOTAL ring collapse**, including `SameKeySameEndpoint` and `WrapAround`. ⇒ **the differential fixture is a genuine SECOND guard, not redundant coverage**, and the unit-level guard is a single point of failure. Neither the SPEC nor the PLAN's first draft asked the question. |
| **M22** | E | ⚠️ **The startup flake fires MORE readily under `-race`** — 2 hits with, 0 without, same session; plausibly the detector's slowdown pushing container boot past the 5 s `waitTCPDial` budget. Recorded, not chartered. ⚠️ **And NEITHER of the two was on this PLAN's own hazard roster by name — the FOURTH consecutive stage at which that has held.** |

### An agent disagreement the controller ruled on, recorded rather than dropped

Agent C reported that the *"DETERMINISTIC/EXACT — not a σ-band"* sites need **no** edit, reasoning that the claim is about affinity and is unaffected by K. Agent A and SPEC §1.1 R4 say the opposite. **The controller read the text and ruled for A/the SPEC.** `driver.go:270-271` verbatim:

```
// SPREAD (>= 2 distinct backends nonzero). DETERMINISTIC/EXACT — not a σ-band
// (reference_differential_band_sigma_margin governs RNG bands; affinity is not one).
```

**The adjective governs the compound "affinity + SPREAD"; only the parenthetical is affinity-scoped.** Spread is exactly the probabilistic leg this row exists to fix. Same shape at `expectations.yaml:21-22` and `README.md:54`. ⇒ **the sites ARE owed an edit** (PLAN T5 Step 3), and — the reason they were nearly missed — they contain **none** of the stale numerals, so a numeral-keyed sweep cannot reach them.

---

## What the PLAN EXECUTED

| item | outcome |
|---|---|
| the measurement test | **BUILT, GREEN** — `CONTROL K=4: 74/2000 rate=0.03700` · `MEASURED K=16: 0/2000` · `PASS (0.61s)` |
| break γ-SHARP (frozen ring) | **FIRED** — CONTROL RED, **MEASURED still green at 0/2000**. The null-result proof, measured not argued. |
| break γ-restore | **GREEN after an observed RED** (`PASS 1.46s`) |
| break β (two-edit) | **FIRED** — `collapse rate 0.03700 (74/2000) >= bar 0.001`, a 35× margin |
| break β (single-edit) | ⚠️ **DID NOT FIRE — exit 0.** Refutation re-confirmed at this tip. `grep -rn '\bsourceIPs\b' --include='*.go' .` ⇒ **3 hits, all in the 0061 driver** — mechanically decoupled. |
| the linkage gate | **REBUILT in `go/parser`** (structurally immune to the doc-comment spoof) + a hardened shell fallback. **2×2 cross-product all four arms RUN**, plus the empty-capture arm the SPEC never ran. ⚠️ The **(4,4)-passes** arm is the one that matters — it proves the gate READS rather than hardcodes. |
| `sourceIPs` 4→16 + rescale | **6/6 PASS**, each leg proved by measurement |
| six anti-vacuity breaks | **ALL SIX FIRED, each in isolation** (exactly one red test per break) |
| **`127.0.0.12`-`.17` bind probe — OWED since the SPEC** | ✅ **DISCHARGED: 16/16 PASS**, every requested `LocalAddr` == observed, full round-trip per arm. No `ip addr add` needed. |
| the collapse law | **MEASURED over 2×10⁵ real ring draws per K**, with a random-key discriminator arm |
| full differential at MASTER (pre-fix baseline) | **119/119 PASS, 408.8 s**, `0061` subtest 3.42 s, first attempt, **no flake**; `comm -3` fixture-set vs subtest-set **EMPTY** |
| `BEHAVIOR_CONTRACT:1326` | **VERIFIED IN SCRATCH** — 5746 → 5746 lines, `diff` exactly `1326c1326`, `% 16 == 0` intact |
| every six-gate negative control | **OBSERVED RED**, one at a time |

⚠️ **A probe-harness lesson that did NOT bite this time because it was pre-empted:** the SPEC's first loopback probe reported `i/o timeout` on all ten arms; the binds had succeeded and the fault was its own `io.Copy(c, c)` echo backend self-splicing (`reference_iocopy_self_splice_echo_backend`). This PLAN's 16-arm probe used a read-then-write backend from the start and hit no such artifact.

---

## The four items SPEC §13 carried as NOT-EXECUTED — ALL FOUR DISCHARGED at this PLAN

| item | outcome, verbatim |
|---|---|
| **Break α** | ✅ **FIRED THE SPREAD LEG.** `runner_test.go:1293: distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` — byte-identical to the expected string. The five legs are sequential `return fmt.Errorf` sites at `:278`/`:285`/`:292`/**`:295`**/`:303`; **`:295` fired.** Affinity and conservation are upstream in the same function and did NOT fire — counts collapse to `[256,0,0]`, `256 % 16 == 0` so affinity survives and the sum is unchanged so conservation survives, **exactly as predicted.** Restore ⇒ isolated `--- PASS (3.67s)`: **green FOLLOWING an observed red.** |
| **Full differential at K=16** | ✅ **119/119 PASS / 0 FAIL / 0 SKIP, `ok … 402.326s`**, `0061` subtest **3.30 s**. Against the same-day pre-fix master baseline (119/119, 408.8 s): **−6.47 s (−1.58%)**. ⚠️ **NO hazard fired at all** — no `subject ready: EOF`, no `bind: address already in use`, no SDS dial-budget failure, **and `0061` did not fire a spread failure.** |
| **`-race` over `./test/differential/`** | ✅ **ZERO data races** (`grep -c 'WARNING: DATA RACE'` ⇒ **0**). Two failures, **both on fixtures unrelated to this row** (`0009-admin-config-dump`, `0084-otlp-access-log`), both at `waitTCPDial` backend-readiness **BEFORE any assertion executed** (`runner_test.go:342`/`:360` are pre-drive readiness gates, not assertion sites) — the recorded startup-flake signature. **Both isolate-green under `-race`.** `0061` in **zero** FAIL lines. |
| **Runtime delta** | ✅ **NO MEASURABLE COST** — see S20. ⚠️ **This REFUTES SPEC §3.6's +0.158 s (+4.5%).** |

## Still OWED at the IMPL

- The **prose sweeps** (T5, T6), the **contract line** (T7), **ADR-0298 completion**, and the **row-76 flip with its four stale-claim corrections** (T9).
- Re-running the thirteen already-fired breaks **at the IMPL's own tip** — a prior stage's transcript is not evidence for another tree.

⚠️ **A stale-comment defect confirmed live at the probe tip, which PLAN T2 fixes:** `driver.go:80` still reads `// 64 — the conservation target` on the line that now evaluates to **256**, and the package doc at `:11-13`/`:23` still says `127.0.0.2..5` / `= 64 total` / `all 64`. The probe commit's own subject says *"totalConns 64 -> 256"* while `git diff master` touches **only line 78**. **A commit message is not an edit.**

---

## Cross-cutting hazards carried into the IMPL

⚠️ **`0061-lb-ring-hash` is THIS ROW'S OWN SUBJECT.** Before the fix a spread failure is expected at ~3.6%/run; **after the fix it is a FINDING.**

⚠️ **A stage brief's flake list is not the index** — neither flake that fired at phase 75 was on that PLAN's roster of six. Isolate-re-run, then state the classification **and its evidence**.

⚠️ **The Bash cwd reset fired again during this PLAN** (`Shell cwd was reset to /home/esa/git/envoy-go`, on multiple calls) — **six consecutive sessions**. Use `git -C <abs-worktree-path>` for every git command; tripwire `pwd` + branch + commit count before any commit or gate run.

⚠️ **Unmeasured:** the differential-suite effect of a **4× larger workload** (256 connections, 16 bound source IPs per side) on runtime and dial budget.

---

# IMPL record

*(per-task `## Task N — ACTUAL` blocks land here, each with `### PLAN anchors — RE-DERIVED before use`, `### Step k — RED/GREEN, verbatim`, `### Break X — FIRED / DID NOT FIRE`, `### Task N — surprises`.)*
