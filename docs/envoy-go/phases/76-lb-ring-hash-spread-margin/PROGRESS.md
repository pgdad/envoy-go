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

**STAGE: IMPL, 2026-07-26.** Worktree `/home/esa/git/envoy-go-wt-p76impl`, branch `phase-76-impl`, base master `921bc148`. Seven subagent dispatches over the T1-T9 spine, each committing locally only; controller squash-push at close. Every figure below was produced at THIS tip — the PLAN's probe-worktree transcripts were treated as predictions to be re-tested, never as evidence.

⚠️ **COMMIT ORDER IS NOT TASK ORDER.** T7 ran concurrently with T4 on a disjoint file set and landed between T3 and T4.

| task | sha | subject |
|---|---|---|
| T1 | `7d64cc2c` | seeded collapse-rate MEASUREMENT with a stacked K=4 non-vacuity control |
| T2 | `cb6dc8a0` | sourceIPs 4 -> 16 (totalConns 64 -> 256); driver_test.go goes RED, T3's input |
| T3 | `353e3457` | rescale driver_test.go 64->256 and re-prove all six legs fire their own assertion |
| T7 | `11426395` | BEHAVIOR_CONTRACT:1326 workload line 64 -> 256, IN PLACE, 5746 -> 5746 lines |
| T4 | `c19b011d` | go/parser LINKAGE gate closing the single-edit case break beta cannot |
| T5 | `92d76d43` | 0061 driver.go + expectations.yaml sweep, incl. the FALSE 'spread is DETERMINISTIC/EXACT' claim |
| T6 | `d9c6c0cf` | 0061 README sweep — strike the statistically invalid 20/20 certification and the FALSE 'fixed ring' claim |
| T9 | `8ffd248f` | ADR-0298 PROPOSED -> COMPLETE in place, row 76 -> done with its four stale-claim corrections |

---

## Task 1 — ACTUAL

### PLAN anchors — RE-DERIVED before use
`ringhash_test.go:3` is a single-line `import "testing"` ✓ (structural 1→N conversion, not an insertion). `_DistinctKeysSpread` ends `:64`, blank `:65`, `_WrapAround` begins `:66` ✓. Symbol preconditions re-derived independently rather than cited: `newRingHashWithRNG` `ringhash.go:78`, `Pick` `:133` (the 3-value destructuring is correct), `HashSourceIP` `hash.go:133`, `Endpoint.Addr()` `cluster.go:102`. Identifier-collision check over `internal/cluster/`: zero declared identifiers matching `collapse*`; `mathrand` alias already precedented at `leastrequest.go:7`.

### Step 3 — GREEN, verbatim
```
    ringhash_test.go:214: CONTROL  K=4: 74/2000 collapses, rate=0.03700 | analytic 3^(1-K)=3.704e-02 → expected ~74.07/2000 | band [0.015, 0.07]
    ringhash_test.go:218: MEASURED K=16: 0/2000 collapses, rate=0.00000 | analytic 3^(1-K)=6.969e-08 → expected ~0.00014/2000 | bar 0.001
--- PASS: TestRingHash_EphemeralPortRing_KeyCollapseRate (0.62s)
ok  	github.com/pgdad/envoy-go/internal/cluster	0.629s
```
Reproduces the PLAN digit-for-digit — the seed stream is portable across worktrees. The FULL package was also run, not only the `-run` selector (`reference_change_set_measure_not_build_measure`): `ok … 4.258s`.

### Break γ-SHARP — FIRED, and only the CONTROL leg
Frozen ports (`_ = collapseDrawPorts(rng)` + `ports := [3]uint32{40001,40002,40003}`):
```
    ringhash_test.go:224: CONTROL leg K=4: collapse rate 0.00000 (0/2000) OUTSIDE band [0.015, 0.07] … the MEASURED leg below is VACUOUS: it reports 0 collapses because nothing varies, NOT because K=16 makes collapse improbable.
--- FAIL (0.60s)   exit=1
```
**MEASURED leg STAYED GREEN at `0/2000`** — the declared MUST-STAY-GREEN outcome. Which leg fired was confirmed by line anchor AND message text, not by the mere presence of a FAIL (`reference_deliberate_break_wrong_assertion`). Both `t.Logf` lines survived the failure — the `t.Errorf`-not-`Fatalf` choice, demonstrated (`reference_fatalf_makes_assertions_unreachable`).

### Break γ-RESTORE — GREEN after an observed RED
`--- PASS (0.61s)`, exit 0 (`reference_liveness_break_needs_failing_baseline`).

### Task 1 — surprises
None material. The PLAN's Step-3 block omits the bare `PASS` line `go test -v` actually emits, and its γ-restore wall time (1.46 s) differs from ours (0.61 s) — wall time, not a semantic claim.

---

## Task 2 — ACTUAL

### PLAN anchors — RE-DERIVED before use
`driver.go:78` `sourceIPs  = 4  // 127.0.0.2 .. 127.0.0.5`, `:79` `burstPerIP = 16`, `:80` `totalConns = sourceIPs * burstPerIP // 64 — the conservation target` — zero drift. `:196` `net.IPv4(127, 0, 0, byte(2+s))` inside `for s := 0; s < sourceIPs; s++` confirmed self-scaling, NOT edited.

### Step 1 — pre-edit baseline
6/6 PASS, `ok … 0.001s`. **This baseline is what makes T3's finding findable.**

### Step 3 — the derived constant BY MEASUREMENT
`PROBE-CONSTS sourceIPs=16 burstPerIP=16 totalConns=256`

### Step 4 — RED, verbatim
```
    driver_test.go:14: expected pass for an affine subject + conserving reference, got: subject conservation: sum 64 != 256
--- FAIL: TestAssertDistribution_Affinity (0.00s)
--- PASS: TestAssertDistribution_ScatterBitesAffinity
--- PASS: TestAssertDistribution_CollapseBitesSpread
--- PASS: TestAssertDistribution_SubjectConservation
--- PASS: TestAssertDistribution_ReferenceConservation
--- PASS: TestAssertDistribution_WrongLength
FAIL	…/test/fixtures/0061-lb-ring-hash/driver	0.002s
```
⚠️ **Exactly ONE test failed and FIVE printed `--- PASS`. That is the trap — two of those five were VACUOUS.** Confirmed by running the **package**, never by `--stat`, a file-scoped grep, or the differential suite.

---

## Task 3 — ACTUAL

### PLAN anchors — RE-DERIVED before use
**15 sites confirmed** (SPEC's 8 refuted): lines 8, 11, 12, 13, 22, 23, 28, 33, 39, 42, **43**, 48, 51, **52**, 59. `:11` carries the file's only bare `4`; **`:43` and `:52` are executable `t.Fatal` string literals**, not comments.

### Step 1 — which leg each test ACTUALLY fired, BEFORE the rescale
```
PROBE-LEG Affinity                 ref=[64 0 0] subj=[32 16 16] -> subject conservation: sum 64 != 256
PROBE-LEG ScatterBitesAffinity     ref=[64 0 0] subj=[20 28 16] -> subject affinity: backend[0]=20 not a multiple of 16 …
PROBE-LEG CollapseBitesSpread      ref=[64 0 0] subj=[64 0 0]   -> subject conservation: sum 64 != 256
PROBE-LEG SubjectConservation      ref=[64 0 0] subj=[16 16 16] -> subject conservation: sum 48 != 256
PROBE-LEG ReferenceConservation    ref=[32 0 0] subj=[32 16 16] -> subject conservation: sum 64 != 256
PROBE-LEG WrongLength              ref=[64 0]  subj=[32 16 16]  -> expected 3 backend counts, got ref=2 subj=3
```
⇒ **`_CollapseBitesSpread` fired CONSERVATION, not spread. `_ReferenceConservation` fired SUBJECT conservation, not reference.** Both printed `--- PASS`. Exactly two vacuous, exactly the two the SPEC named — and the first of them is the unit test for **this row's own assertion**.

### Step 3 — each test fires its OWN leg, AFTER the rescale
```
PROBE-LEG Affinity                 ref=[256 0 0] subj=[128 64 64]  -> <nil>
PROBE-LEG ScatterBitesAffinity     ref=[256 0 0] subj=[20 108 128] -> subject affinity: backend[0]=20 not a multiple of 16 …
PROBE-LEG CollapseBitesSpread      ref=[256 0 0] subj=[256 0 0]    -> subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)
PROBE-LEG SubjectConservation      ref=[256 0 0] subj=[64 64 64]   -> subject conservation: sum 192 != 256
PROBE-LEG ReferenceConservation    ref=[128 0 0] subj=[128 64 64]  -> reference conservation: sum 128 != 256
PROBE-LEG WrongLength              ref=[256 0]   subj=[128 64 64]  -> expected 3 backend counts, got ref=2 subj=3
```
**Six for six, each on its own leg.** Scatter tuple is `{20, 108, 128}` — **the ×4 rescale `{80,112,64}` was NOT used** (all three multiples of 16; it would have manufactured a third vacuity). `zzprobe_test.go` deleted; `ls` shows only `driver.go`, `driver_test.go`.

### Breaks V1-V6 — ALL SIX FIRED, each producing exactly ONE red test
Anchor uniqueness (`grep -cF` ⇒ 1) asserted before each edit; `git restore` between; `-count=1` throughout.

| # | test | break to `driver.go` | verbatim FAIL line | isolation |
|---|---|---|---|---|
| 1 | `_Affinity` | `:284` `c%burstPerIP != 0` → `c%burstPerIP == 0 && c > 0` | `driver_test.go:14: expected pass …, got: subject affinity: backend[0]=128 not a multiple of 16 …` | 1 red / 5 green |
| 2 | `_ScatterBitesAffinity` | `:284` → `if c%1 != 0 {` | `driver_test.go:25: expected affinity failure on a scattered subject distribution` | 1 red / 5 green |
| 3 | `_CollapseBitesSpread` | `:294` `nonzero < 2` → `nonzero < 1` | `driver_test.go:35: expected spread failure on a collapsed (single-backend) subject distribution` | 1 red / 5 green |
| 4 | `_SubjectConservation` | `:291` `subjSum != totalConns` → `subjSum > 1000000` | `driver_test.go:44: expected conservation failure on a sub-256 subject sum (192)` | 1 red / 5 green |
| 5 | `_ReferenceConservation` | `:302` `refSum != totalConns` → `refSum > 1000000` | `driver_test.go:53: expected conservation failure on a sub-256 reference sum (128)` | 1 red / 5 green |
| 6 | `_WrongLength` | `:277` drop the `\|\| len(refCounts) != 3` term | `driver_test.go:61: expected error on wrong-length reference counts` | 1 red / 5 green |

**Break 1 is the strongest line in the task**: its message names the **affinity** leg explicitly, so `_Affinity` genuinely observes affinity's verdict rather than merely observing *"no error from anywhere"* (`reference_positive_arm_cannot_catch_overfiring`). Breaks 3 and 5 are the two that were vacuous before T3 and are now load-bearing.

---

## Task 4 — ACTUAL

### Zero exported symbols — proved by execution, not asserted
`go doc -all ./test/fixtures/0061-lb-ring-hash/driver` with the file present vs parked away: both `exit=0 bytes=2950`, `diff` exit 0 — **byte-identical exported surface**. `grep -c '^package driver'` ⇒ 1 proves the package was actually read rather than the invocation failing open. ⚠️ **ONE package per invocation** (`reference_gate_command_negative_control`, as narrowed at the PLAN).

The relative path `../../../../internal/cluster/ringhash_test.go` was confirmed **by execution**, not by counting slashes: the (16,16) arm passes, which is only reachable after `parser.ParseFile` succeeds.

### Step 2 — the FULL 2×2 cross-product
| `sourceIPs` | `collapseFixtureK` | result |
|---|---|---|
| 16 | 16 | **PASS**, exit 0 |
| 4 | 16 | **FAIL**, exit 1 |
| **4** | **4** | **PASS**, exit 0 |
| 16 | 4 | **FAIL**, exit 1 |

⚠️ **The (4,4) arm is the one that matters and it passed** — it proves the gate READS the test-side value rather than hardcoding 16. A gate that only ever saw (16,16) and (4,16) is indistinguishable from `if sourceIPs != 16 { fail }` (`reference_probe_must_discriminate`).

### Step 3 — the SPEC's gate, broken in BOTH directions, reproduced at THIS tip
**FAILS OPEN** (scratch copy, both constants renamed): `captured: a='' b=''` / `LINKED:` / **exit 0** — green on a tree where neither constant exists.
**FAILS CLOSED** (live, correctly-synced 16/16 tree): `DESYNC: sourceIPs=16 collapseFixtureK=16` / **exit 1**.
**The Go gate on the renamed tree:** `linkage_test.go:68: … found 0 const declarations of collapseFixtureK, want exactly 1`, exit 1.

Two bonus arms nobody specified: renaming **`sourceIPs`** is caught earlier still, at **COMPILE** time (`undefined: sourceIPs`); a **deleted/moved** file yields `parse …: no such file or directory`.

### Step 4 — the hardened shell fallback, three trees
```
synced   (16/16):  LINKAGE PASS: K=16                                              exit 0
renamed  (both):   LINKAGE RED: EMPTY capture (a='' b='') — renamed/moved/deleted  exit 1
desynced (16 vs 4):LINKAGE RED: DESYNC sourceIPs=16 collapseFixtureK=4             exit 1
```
The synced tree is the direct proof the `^\s*` anchor fixes the fail-CLOSED direction: **the same tree that made the SPEC form print `DESYNC` prints `LINKAGE PASS`.**

---

## Task 5 — ACTUAL

Roster re-derived at this tip: **zero drift** from the PLAN's table across all ten `driver.go` rows and all nine `expectations.yaml` sites. T2's `:78`/`:80` correctly drop out of the stale-numeral greps — that is T2's consequence, not drift.

8 `driver.go` comment sites + 6 `expectations.yaml` sites edited. **Diff proved comment-only**: every added/removed line in `driver.go` matches `^[+-]\s*//`; `expectations.yaml`'s diff is entirely inside the leading `#` header block, above `response-body:` — no YAML key changed.

### Step 3 — the OPPOSITE-DIRECTION claim, edited
OLD `driver.go:270-271`:
```
// SPREAD (>= 2 distinct backends nonzero). DETERMINISTIC/EXACT — not a σ-band
// (reference_differential_band_sigma_margin governs RNG bands; affinity is not one).
```
NEW splits the two legs: affinity (and conservation) stay DETERMINISTIC/EXACT; **SPREAD is named PROBABILISTIC**, with `P(collapse) <= 3^(1-sourceIPs) = 7.0e-8 at sourceIPs=16` stated as **ANALYTIC/EXTRAPOLATED, NOT measured**, `3^(1-K)` named a **CONSERVATIVE UPPER BOUND** (0.949 at K=4, 0.689 at K=8), and a pointer to the measurement and the linkage gate. The same split applied at `expectations.yaml:21-22`.

⚠️ These sites carry **none** of the stale numerals — a numeral-keyed sweep is structurally blind to them. An agent argued at the PLAN that they need no edit; **the controller ruled against it on the text**, and that ruling is what this task discharged.

### Step 5 — stale-numeral sweep
```
driver/driver.go:417:		v, err := strconv.ParseUint(valStr, 10, 64)
```
**One survivor, justified**: the `ParseUint` bitSize, executable code, explicitly NO EDIT. (Was `:405`; +12 from Step 3's expansion.) `D-S36-4` tokens intact at `driver.go:20` and `expectations.yaml:21`.

`0062/driver/driver.go:299` and `0063/driver/driver.go:299` confirmed **still carrying the false adjective** and deliberately NOT edited — `:299`, confirming the PLAN's correction of the SPEC's `:300`.

---

## Task 6 — ACTUAL

### PLAN §1.4 E1/E2/E3 — all three CONFIRMED, none refuted
`:57` carries the `64`; **`:58` is verbatim `across the three backends).` — no numeral at all**, so the SPEC's `:58` is wrong. `:114` and `:135` (two `64`s) are real sites absent from the SPEC roster. No further drift.

`README.md` **162 → 240 lines.**

### Step 5 — the REFUTED FLAKE CERTIFICATION, STRUCK not re-measured
The `### Flake check` section was replaced by `### Spread margin — a DERIVED margin, not a pass-count`, which quotes each of the three struck claims **as a mention and immediately labels it false**: *"fixed ring"* is FACTUALLY FALSE and is the root cause; *"overwhelmingly stable (4 source-IP keys)"* is precisely what the flake refuted at `3^(1−4) = 3.7e-2`; and `20/20 PASS` had **no statistical power**. The replacement points at the measurement, states `7.0e-8` as analytic/extrapolated, names `3^(1−K)` a conservative upper bound, and says explicitly that **a pass-count is not a margin**.

⚠️ **The fixture was NOT re-run N times.** That is the exact error being corrected.

### Step 4 — the scatter bound: the BOUND survives, the NUMERALS do not
Recomputed independently by exact rational multinomial: **0.0962% at n=64 → 0.3814% at n=256**, ratio **3.96×**. `< 1%` still holds. The trade is stated as deliberate: the spread flake is **OBSERVED** (three occurrences); the scatter adversary is **HYPOTHETICAL**.

### Step 6 — final shape sweep
Six survivors, each justified: `:57`/`:60` name the old blanket adjective **in order to disown it** and mark the affinity leg where DETERMINISTIC/EXACT is TRUE; `:180`/`:184`/`:189`/`:192` are the struck claims quoted inside the strike rationale, each immediately labelled FALSE. **Every occurrence is a mention, never a use.** `:76`'s *"overwhelmingly discriminating"* (affinity sense, correct) was NOT matched and NOT edited — the sweep pattern is `overwhelmingly stable`. `D-S36-4` at three sites proved byte-identical by extraction+diff.

---

## Task 7 — ACTUAL

`grep -n 'source IPs .127\|16 conns each'` ⇒ **exactly 1 hit, line 1326.** File **5746** lines; line 1326 **1791** chars; four `64`s, three `16`s, zero `256`s before the edit.

After: **5746 → 5746 lines** (unchanged), `diff` **exactly `1326c1326`** and nothing else (corroborated by `git diff -U0`'s `@@ -1326 +1326 @@` and `--numstat` `1 1`), chars **1791 → 1796 (+5)**, file bytes +5. Content checks on the real new line: `'64'` ×0 · `'256'` ×4 · `% 16 == 0` present · `16 conns each` present · `all-16-or-0` present · `127.0.0.2..17` present. ⇒ **every by-line citation into the section stays valid.**

Whole-file `grep -c '64'` went **101 → 100 lines** — the delta of exactly 1 is independent corroboration that only `:1326` lost its `64`s. `:1338` (which matches on **`xxHash64`**, a hash-function name) proved **BYTE-IDENTICAL** by `cmp` against the HEAD blob.

**Stat surface +0.** `grep -n '1205'` ⇒ 3 hits (`:831`, `:847`, `:5004`), **all three left at 1205, none edited**. ⚠️ 1205 is **DOCUMENTARY** — no mechanical counting command exists and the chain is discontinuous in two recorded places. **This row asserts the DELTA (zero), never the total.** No ledger line added: a +0 row has nothing to record.

---

## Task 8 — ACTUAL (the break roster)

### Anchors RE-DERIVED — including the ones this row's own edits moved
`ringhash.go:129-132` doc comment · `:133` `func … Pick(` · **`:140` `m := sort.Search(…)`** · `:142` `m = 0 // wrap`. Anchor confirmed exactly as the PLAN states. `sort` stays used at `:105`, so the α edit still compiles.

⚠️ **The five `return fmt.Errorf` legs in `AssertDistribution` MOVED** (T5's comment sweep pushed them ~+12): length `:290` · affinity `:297` · subject conservation `:304` · **spread `:307`** · reference conservation `:315`. The PLAN cites `:295` for spread; that is the **pre-T5** anchor.

### Break α (LIVENESS) — FIRED THE SPREAD LEG, and only it
`ringhash.go:140` `m := sort.Search(…)` → `m := 0`:
```
    runner_test.go:1293: distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)
--- FAIL: TestDifferential/0061-lb-ring-hash (3.77s)
```
Byte-identical to the expected string, from **`driver.go:307`, the FOURTH of five sequential legs.** Exactly one error line printed; no affinity-, conservation- or length-shaped text appeared. **Affinity SURVIVED** (counts collapse to `[256,0,0]`, `256 % 16 == 0`), **conservation SURVIVED** (sum unchanged at 256), **length SURVIVED** — exactly as predicted ⇒ **neither vacuous nor mis-targeted.** Restore ⇒ isolated `ok … 4.670s`, exit 0: **a green FOLLOWING an observed red.**

### Break β (TWO-EDIT) — FIRED the MEASURED leg
Both constants → 4:
```
    ringhash_test.go:233: MEASURED leg K=4: collapse rate 0.03700 (74/2000) >= bar 0.001 (analytic 3^(1-K)=3.704e-02)…
--- FAIL (0.63s)
```
**CONTROL leg stayed green** (74/2000 inside `[0.015, 0.07]`), so the failure is a real measurement, not a stalled harness. ⚠️ **A two-edit β proves only the UNIT TEST's constant load-bearing. It does NOT prove the FIXTURE's.**

### Break β-SINGLE-EDIT — DID NOT FIRE, as declared — paired with a RED linkage gate
Fixture at 4, `collapseFixtureK` at 16 ⇒ measurement `--- PASS`, **exit 0**. The fixture shrank to K=4 and the measurement reports a K=16 safety margin — **measuring a fixture that no longer exists.**

On that **same tree**, T4's gate:
```
    linkage_test.go:73: DESYNC: this fixture drives sourceIPs=4 distinct ring_hash keys, but internal/cluster's collapse-rate test pins collapseFixtureK=16 … Change both, or neither.
--- FAIL   exit 1
```
⇒ **measurement GREEN + linkage RED on one tree. That pairing is T4's entire justification, and it is now discharged by execution rather than argued.**

### Breaks γ / γ-SHARP — re-confirmed at the IMPL tip
γ-SHARP: CONTROL leg RED at rate 0.00000, **MEASURED leg green at `0/2000`**, no `:234` line. γ-restore: `CONTROL 74/2000 rate=0.03700` in band, `MEASURED 0/2000`, `--- PASS (0.62s)`, exit 0.

### Break-roster tally
**SIXTEEN breaks, ALL SIXTEEN executed at this tip** — α · β · β-single (declared MUST-NOT-FIRE, did not fire) · γ · γ-sharp · L1-L4 (the 2×2) · L5 (the empty-capture arm) · V1-V6. Declared MUST-STAY-GREEN (γ-sharp's MEASURED leg) held.

---

## ⚠️ TEN PLAN/ADR CLAIMS REFUTED BY EXECUTION AT THE IMPL TIP

1. **PLAN §1.1(b): `grep -c 'collapseFixtureK'` ⇒ 2.** At this tip it is **9** — T1's own test adds seven more mentions. The fail-CLOSED *conclusion* holds; the *number* does not. ⚠️ **The PLAN measured at `98c27fc9`, before T1 landed. A count taken before this row's own edits cannot describe the tree the gate runs on.**
2. **PLAN `:784` — "a rename, move or deletion produces `found == 0` and fails LOUD."** Only a rename of the **test-side** constant yields `found 0`. A move/deletion yields `t.Fatalf("parse …: no such file")`; a rename of `sourceIPs` is caught earlier still, as a **COMPILE error**. All three fail loud — through **three different mechanisms**, not one. The guarantee is intact; the stated mechanism is not.
3. **PLAN T5 Step 5's expected-survivor list over-predicts its own gate.** It says the `D-S36-4` tokens will survive the sweep — but **none** of the four alternations can match `D-S36-4`. Real expected output is exactly ONE line. An executor waiting to see the `D-S36-4` lines would have concluded the gate had not run. ⚠️ **Same defect class as the SPEC gate the PLAN itself caught: a documented expectation that was never executed against.**
4. **PLAN T6 Step 5 and ADR-0298 ¶5's `-count=81` for 95% power.** `(1−0.0355)^81 = 0.0535` ⇒ **94.6%**, short of the 95% it is offered as. The first N reaching ≥95% is **83** (`0.0498`). ⚠️ **The error runs in the direction that FLATTERS the ADR's own conclusion** — an under-stated N makes the rejected alternative look cheaper than it is.
5. **PLAN §1.7(c)/M21 — "the unit package catches break α by exactly ONE test out of 403."** At this tip **TWO of 404** fail: `TestRingHash_DistinctKeysSpread` **and T1's own new `TestRingHash_EphemeralPortRing_KeyCollapseRate`**, the latter firing **BOTH** legs at rate **1.00000**. ⇒ **α is a valid liveness break but is NOT isolating.** This row's own T1 moved the denominator the PLAN's finding was stated against.
6. **⚠️ A defect in the CONTROL leg's own diagnostic prose, surfaced by break α.** The message says *"A rate of 0 HERE means the ring is NO LONGER BEING REDRAWN"* — under α the control goes red at rate **1.00000**, the **opposite pole**. The leg still catches it; **the sentence that explains it mis-describes this failure mode.** A diagnostic string is exactly the kind of prose no test can falsify. Recorded, deliberately NOT fixed by this row.
7. **PLAN T8.2 — "the fixture's own package is completely blind to break β (`ok … 0.002s`)."** It now goes **RED**: `_Affinity` fails `subject conservation: sum 256 != 64`, because β's `sourceIPs=4` re-derives `totalConns` to 64 while `driver_test.go`'s tuples stay pinned at 256.
8. **PLAN T8.3 — `grep -rn '\bsourceIPs\b' --include='*.go' .` ⇒ 3 hits, all in the 0061 driver.** At this tip: **16 hits across 3 files** (`ringhash_test.go` 7 prose · `linkage_test.go` 4, three of them CODE · `driver.go` 5). The unexported-identifier **argument** survives; the cited **count** does not — T4 added the very consumer that changes it.
9. **PLAN §4 — break β's margin is "35×".** Measured **37.0×** (0.03700 / 0.001).
10. **PLAN T7 Step 4 — "the Stat surface section runs to `:5006`."** `:5006` is the **NEXT** heading (`### Forward-pointer note (26.3)`); the section's last content line is `:5004` with a blank `:5005`. A future row appending to the ledger must insert **after `:5005`**, or it lands under the wrong heading.

### A NEW latent defect, found only by break β at (4,4) — recorded, not chartered
At (4,4) only **ONE** of the six `AssertDistribution` tests goes red, not all six: the other five are **negative arms asserting only `err == nil`**, so they pass whichever leg fired. **Two of them pass VACUOUSLY** — `_CollapseBitesSpread` and `_ReferenceConservation` trip the earlier subject-conservation leg (`driver.go:304`) instead of the leg they are named for. **Not a defect at the real tip** (T3's probe proved all six fire their own leg at 256), but **the negative arms are not self-isolating and would survive a re-ordering of the five legs** (`reference_ordered_assertion_legs_vacuous_on_constant_change`). Deferred, named here so a future sweep finds it.

### ADR-0298's OWN §Context defects, CORRECTED IN PLACE at T9
Four, on the ADR-0297 ¶7/¶9 precedent, each **leading with what survives**: **¶1**'s `driver.go:295` → `:307` (drift caused by this row's own edits — a line cite drafted at the SPEC, for a file the IMPL then edits above that cite, is stale **by construction**) · **¶5**'s `-count ≈ 81` → **83** · **¶7**'s *"expecting ~71 collapses"* → measured **74/2000** · **¶11**'s *"`README.md:80`'s `< 1%` therefore still holds and **needs no edit**"* → the **bound** holds, the **numerals do not**; T6 rewrote the block. ⚠️ ***"the bound still holds"* and *"the text needs no edit"* are DIFFERENT claims**, and ¶11 inferred the second from the first — the same inference a numeral-keyed sweep makes when it skips a line whose numbers it did not have to change.

---

## Task 9 — ACTUAL (gates, suites, counts, ADR, row flip)

### The six-gate + envelope battery — ALL GREEN, EVERY NEGATIVE CONTROL OBSERVED RED

| # | gate | ACTUAL | NC — observed red |
|---|---|---|---|
| G0 | branch tripwire | `phase-76-impl`, `master..HEAD` = 8 | ⚠️ bare `pwd` printed `/home/esa/git/envoy-go` — **the cwd reset fired again** |
| G1 | `go build ./...` | silent, 0 | `hash.go:211:13: cannot use "nope" … as int value`, exit 1 |
| G2 | `go vet ./...` | silent, 0 | `format %d has arg "str" of wrong type string`, exit 1 |
| G3 | `[ "$(gofmt -l . \| wc -l)" -eq 0 ]` | `count=0`, 0 | file printed; ⚠️ **`gofmt -l` RAW exit=0**, and the `&&`-chained form printed `INERT-GATE-SAYS-PASS` |
| G4 | `golangci-lint run ./...` | silent, 0 | `declared and not used: unusedVar (typecheck)`, exit 1 |
| G5 | `go mod tidy -diff` + `git diff master -- go.mod go.sum` | both EMPTY; modules **67** | fake require ⇒ `bytes=343`, 67 → 68, exit 1 |
| G6 | `go test ./test/differential/ -count=1` | **119/119 PASS / 0 FAIL / 0 SKIP**, `ok … 409.759s` | break α (T8) |
| G7 | `-race` over `./test/differential/` | **119/119 PASS, ZERO data races** (2nd run) | — |
| G8 | +0 production `.go` vs master | **0 lines** | real history `9f5d667b..c57b98b8` ⇒ **2** |
| G9 | +0 exported symbols, ONE pkg per invocation | 3 packages, symbol-scoped diff clean | `> func ExportedProbeSymbol() int`, exit 1 |
| G10 | +0 PRODUCTION imports | `BASE=1952 HEAD=1952`, PASS | injected `"math"`/`"math/rand"` ⇒ 1952 → 1954, exit 1 |
| G11 | fixtures | **119** | `mkdir 0118-fake` ⇒ 120 |
| G12 | fuzzers | **55** | `func FuzzProbeOnly` ⇒ 56 |
| G13 | BackendKind tail **38** (⚠️ **39 constants declared, 0-38 — do NOT "fix" 38 to 39**) · 5× `TestNoNewStat*` PASS · linkage Go gate PASS · hardened shell `LINKAGE PASS: K=16` | | `ZZProbeKind = 39` ⇒ tail 39 |
| G14 | sha256 roster, **production-only, 375 files** | **0 mismatches, 0 missing**; `comm -12` vs the 10-file EDIT roster **EMPTY** | **TWO legs**: MISMATCH (`ringhash.go` sha differs) and **MISSING** (`rm hash.go` — ⚠️ mismatches stayed **0**, so the mismatch leg alone reads a deletion as clean) |

⚠️ **The phase-75 roster was NOT copied forward.** `internal/cluster/ringhash_test.go` is **absent** from phase 76's roster and **present** in its edit roster ⇒ the phase-73 SEVERE-1 is not reproduced.

**Test-side import delta is +8 and that is correct**: `ringhash_test.go` gains `fmt`, `math`, `mathrand "math/rand/v2"`; `linkage_test.go` adds `go/ast`, `go/parser`, `go/token`, `strconv`, `testing`. All stdlib, all test-side. **An import LINE is not a go.mod MODULE.**
⚠️ **§1.1(e) reproduced verbatim**: the phase-75 `^import \($`-only helper emits **ZERO** lines for both single-line-import files — structurally blind to 2 of this row's 3 `.go` files.

### ⚠️ TWO MORE BROKEN GATES — the SIXTH and SEVENTH in this lineage, both NEW

**G-A. PLAN `:1159-1163`'s G9 form is not symbol-scoped and reads RED on a correctly-implemented tree.** `./test/fixtures/0061-lb-ring-hash/driver` exports **zero** top-level symbols, so its entire `go doc -all` output is package-doc PROSE — and T2/T5's legitimate comment rescale (`127.0.0.2..5`→`..17`, `64 total`→`256 total`) surfaces as three diff lines, exit 1. **Not a fail-open — a FALSE POSITIVE**, which is worse in a specific way: it would either stop a close script or, over time, teach a future session to suppress the gate. Symbol-scoped (`^(func|type|var|const)` + indented exported members) all three packages diff clean: 59=59, 20=20, 0=0.

**G-B. PLAN `:1155`'s wrong-baseline caveat names the WRONG sha — refuted by execution.** It warns that *"baselining against the phase-75 tip returns 2 and reads RED on a clean tree."* Executed: HEAD vs **`c57b98b8`** (the phase-75 IMPL tip) ⇒ **0** — it is an **ANCESTOR** of master, so that baseline is **harmless**. HEAD vs **`9f5d667b`** (the phase-75 *pre*-IMPL router commit) ⇒ **2**. The `2` the PLAN quotes comes from the *range* `9f5d667b c57b98b8`, a different operation. ⚠️ **A session following the caveat literally would guard against the SAFE sha and walk into the unsafe one.**

### G6 — the full differential suite at the IMPL tip
```
ok  github.com/pgdad/envoy-go/test/differential  409.759s
subtest PASS: 119 | FAIL: 0 | SKIP: 0
    --- PASS: TestDifferential/0061-lb-ring-hash (3.41s)
```
`comm -3` fixture-directory set vs subtest set ⇒ **EMPTY**. First attempt, no flake, `0061` did **not** fire a spread failure.

⚠️ **NO cross-session runtime delta is claimed from this number.** There is no same-session full-suite master control at this tip, and *"the absolutes do not transfer across sessions; the deltas do."* The PLAN's 402.326 s belongs to its probe tip and is not comparable.

### G7 — `-race`: a REAL failure on the first attempt, classified on evidence
**First `-race` run FAILED, exit 1** — and it is worth recording exactly how, because the wrapper's exit code lied:

```
panic: driver: start ALS receiver on 0.0.0.0:38633: listen tcp 0.0.0.0:38633: bind: address already in use [recovered, repanicked]
  …/0082-grpc-access-log-buffering/driver/driver.go:185  (*alsDriver).ensureServer
  …/driver.go:203                                        (*alsDriver).ReferenceBootstrap
  …/test/differential/runner_test.go:200
--- FAIL: TestDifferential/0082-grpc-access-log-buffering (0.10s)
FAIL  …/test/differential  254.203s
```
⚠️ **The background-task notification reported "exit code 0" — that is the WRAPPER shell's status, not `go test`'s.** The logged `EXIT=1` is the real one. **A harness's exit code is not the command's exit code.**

**Classification — startup class, NOT a regression**, on this evidence:
1. It fired at `ReferenceBootstrap` (`runner_test.go:200`), **BEFORE any assertion executed** — a bootstrap site, not an assertion site.
2. Signature is the recorded `bind: address already in use` on an OS-ephemeral port — a TOCTOU race on port allocation.
3. **`0082` is byte-untouched by this row** — the 10-file EDIT roster contains no `0082` path.
4. **Isolate-re-run 3/3 GREEN under `-race`** (`ok … 5.192s / 5.274s / 5.145s`).
5. ⚠️ It is a **PANIC**, so it aborted the whole test binary at 84 of 119 subtests — which is why the first run's PASS tally was 83, not a partial-failure count.

**Clean re-run: 119/119 PASS / 0 FAIL / 0 SKIP, ZERO `WARNING: DATA RACE`, zero panics, `ok … 400.595s`, exit 0**, `0061` at 3.29 s.

⚠️ **`0082-grpc-access-log-buffering` was NOT on this stage's hazard roster by name — the FIFTH consecutive stage at which that has held.** Its sibling `0083-grpc-access-log-headers` fired the byte-identical signature at phase 75. *A stage brief's flake list is not the index.*
⚠️ **The PLAN's observation that the startup flake fires MORE readily under `-race` is REPRODUCED, a third data point**: this session ran one non-race full suite (0 hits) and two `-race` full suites (1 hit). Recorded, not chartered.

### The runtime question — measured with a SAME-SESSION control, 3 warm runs per arm
| arm | wall mean | `go`-reported mean | arm's own spread (wall) |
|---|---|---|---|
| K=16 (256 conns) | **4.627 s** | **3.860 s** | 4.56 – 4.75 = **0.19 s** |
| K=4 (64 conns, temp revert) | **4.523 s** | **3.772 s** | 4.46 – 4.58 = **0.12 s** |

Delta **+0.104 s (+2.3%)** wall, **+0.088 s (+2.3%)** reported.

⚠️ **The delta is SMALLER than the K=16 arm's own run-to-run spread, so n=3 does not resolve it.** And ⚠️ **the SIGN FLIPPED against the PLAN's same-session measurement** (which got −1.2% on the same comparison). **Two same-session controls disagreeing in SIGN is the strongest available evidence that the effect is below measurement resolution.** The honest statement: **the runtime cost of K=4 → K=16 is not resolvable at this sample size, and it is certainly not the SPEC's +0.158 s (+4.5%)** — that figure stays REFUTED. Do not carry a signed delta forward in either direction.

⇒ **PLAN §7's "unmeasured until T9: the differential-suite effect of a 4× larger workload" is now DISCHARGED** — measured, and the answer is "below resolution", not a number.

### Counts at exit — re-run MECHANICALLY in the worktree
fixtures **119** (`0061` EDITED, not added) · fuzzers **55** · stat surface **1205** (⚠️ **DOCUMENTARY — no mechanical command**; only the **+0 DELTA** is asserted, and that argument is **STRUCTURAL**: G8 = 0 production `.go` files changed and G14 = 0 byte mismatches over 375 production files, so no `NewCounter`/`NewGauge` call site can have moved) · BackendKind tail **38** (⚠️ a TAIL VALUE; **39** constants declared, 0-38) · go.mod modules **2** (⚠️ the phase-61.2 lineage figure — `quic-go` + `qpack` — **NOT a repo total**; the single `go.mod` requires **67** = 18 direct + 49 indirect) · DECISIONS tail **ADR-0298 COMPLETE**, next-free **ADR-0299** · next-free reference port **10450**.

### ADR-0298 and the row flip
ADR-0298 completed **IN PLACE**, no renumber. Controller-re-derived shape: `### Context` **1** · `### Decision` **1** · `### Consequences` **1** · retained italic footer **1** · `^## ADR-0299` **0** · tail `## ADR-0298` · last `^---$` still at **`:17020`** (no separator added). It carries **NO whole-file grep count** and claims **NO family ordinal** — the Load-balancing family closed at row 54, and *maintenance rows do not extend a charter*.

`ROADMAP.md:138` flipped `in-progress` → **`done`**, a **SINGLE FLAT ROW**, with all four BRAINSTORM-era stale claims corrected in place (`4 → 10` ⇒ **4 → 16** · `K=10 → 5.1e-5` ⇒ **K=16 → 7.0e-8, analytic** · β re-characterised as **TWO-EDIT** · *"THE EXECUTABLE DELTA IS ONE INTEGER"* marked **FALSE**).

⚠️ One refinement the row-flip made over the PLAN's own correction text: PLAN §9 Step 8 says `grep -c '\b64\b' driver.go` ⇒ 8, *"one of them CODE"*. The count is right, but the eighth hit is `strconv.ParseUint(valStr, 10, 64)` — a **bit-size argument, not a workload constant**. It is code, but it is not evidence that the workload constant appears in code, and the cell says so precisely rather than repeating the bare phrase.

### Sentinel — re-run MECHANICALLY by the controller AFTER the ROADMAP edit landed
- **(1) SILENT** — row 76 was the last non-`done` chartered row. **This stage silenced it.** ⚠️ Anti-false-negative guard: silence could also mean row 76 fell into the regex blind spot, so it was proved not to — the check-(1) regex applied to `:138` alone returns `| 76 | lb-ring-hash-spread-margin | 75 | done `. **The silence is genuine.**
- **(2) ⇒ 3** — HTTP/3 `:186`, xDS `:196`, Observability `:206`, unchanged. **This row narrowed nothing, as declared.**
- **(3) `NEVER OPENED: gRPC / Runtime / WASM`** — unchanged.

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS NOT CREATED and MUST NOT BE.**

**check-(1) blind spot, re-derived independently at this tip: 108 data rows (`:31`-`:138`) / 104 matched / FOUR misses** — `:31` `| 00 |` (em-dash in the after-column), `:35` `| 04 |` (DOT in the slug `http-1.1`), `:83` `| 28.1a |`, `:84` `| 28.1b |` (letter-suffixed row id). All four `done` ⇒ no current impact.
