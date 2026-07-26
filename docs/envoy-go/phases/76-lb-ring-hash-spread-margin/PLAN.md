# PLAN 76 — the `0061-lb-ring-hash` spread-assertion statistical margin

> **For agentic workers:** execute this plan task-by-task, red-first. Steps use checkbox (`- [ ]`) syntax. Each task re-derives its own anchors at the tip it is editing — **anchors drift within a phase's own tasks** (phase 75's ran +1 to +168, NON-MONOTONIC), and *a drift CORRECTION is itself a claim* (`reference_a_drift_correction_is_itself_a_claim`).

**Goal:** give `test/fixtures/0061-lb-ring-hash`'s spread assertion a DERIVED statistical margin (`sourceIPs` **4 → 16**), replace the pass-count that certified it with a seeded MEASURED RATE, rescue two silently-vacuous unit tests, and sweep every prose site the fix falsifies.

**Architecture:** `sourceIPs` is the number of DISTINCT ring-hash keys (`HashSourceIP` strips the port, so `burstPerIP` connections from one source IP are ONE key), while the ring is keyed on the backend's ephemeral-port-bearing address — a fresh random 3-way partition every run. Per-run collapse probability is `3^(1−K)`: **3.7e-2 at K=4, 7.0e-8 at K=16**. The fix is one integer plus everything that integer falsifies.

**Tech Stack:** Go 1.26.5 · `internal/cluster` (ring builder, `xxHash64`, `HashSourceIP`) · the differential harness (`test/differential`) against `envoyproxy/envoy:contrib-v1.37.2`.

**STAGE:** PLAN (lifecycle-state **2 → 3**). **ROW 76 STAYS `in-progress`.** ROADMAP **BYTE-UNTOUCHED** at this stage (the phase-75 precedent); `DECISIONS.md` **BYTE-UNTOUCHED** (ADR-0298 completes at the IMPL). File set: this `PLAN.md` + `PROGRESS.md` + `STATE.md` + `next-prompt.txt`.

---

## 1. PLAN re-derivation ledger — what this stage RE-DERIVED, REFUTED, and newly EXECUTED

Every figure below was produced at this PLAN's own tip (`98c27fc9`) by execution, in four isolated worktrees with private scratch, plus controller re-derivation of every load-bearing claim. **None is carried from the SPEC.**

### 1.1 ⚠️ THE HEADLINE — THE SPEC'S OWN LINKAGE GATE FAILS OPEN, AND SO DO TWO OF THE THREE GATES IT PRESCRIBES

SPEC §9.2 warns, correctly, that *"a green gate is evidence only if you have seen it go red"* — and then ships a gate whose red arm was never fired.

**(a) SPEC §3.3.1's linkage gate (`SPEC.md:225-227`) exits 0 on a tree where NEITHER constant exists.** `[ "$a" = "$b" ]` with both captures empty is TRUE in `test(1)`. Controller-executed:

```
SPEC FORM: exit=0 GREEN with a='' b=''   <-- FAILS OPEN
```

A rename, a file move or a deletion — the three things most likely to happen to this pair over the next year — all read GREEN. The SPEC's own control transcript (`:231-232`) covers only 4-vs-10 and 10-vs-10, and **still quotes the superseded `10`**, not 16.

**(b) The SAME gate ALSO fails CLOSED on a correctly-synced tree.** `grep -c 'collapseFixtureK'` returns **2**, not 1 — the second match is the measurement test's **own doc comment** (`K=collapseFixtureK=16`). Executed on a genuinely synced tree (both 16):

```
DESYNC: sourceIPs=16 collapseFixtureK=16
16
exit=1
```

⇒ `reference_sentinel_matcher_string_self_clears`, live in a gate rather than a sentinel. **The naive gate is broken in BOTH directions at once.** T4 replaces it with a `go/parser` Go test that is structurally immune (the parser sees declarations, not comments) plus a hardened shell fallback.

**(c) ⚠️ `go doc -all <pkgA> <pkgB>`'s fail-open is specific to the `./` PREFIX — this NARROWS the recorded finding.** `reference_gate_command_negative_control` and SPEC §9.2 both state the two-package form fails open flatly. Controller-executed:

| form | exit | stdout | stderr |
|---|---|---|---|
| `go doc -all internal/cluster internal/stats` | **1** | 0 B | `doc: no symbol internal/stats in package …/internal/cluster` |
| `go doc -all ./internal/cluster ./internal/stats` | **0** | 30471 B (== cluster-only) | *(empty)* |
| `go doc -all ./internal/cluster ./nope/nothing` | **0** | 30471 B | *(empty)* |

`grep -c '^package stats'` over the `./`-prefixed output ⇒ **0**. **A bare import path fails LOUD; a `./`-prefixed path is silently discarded — even when it names a directory that does not exist.** Phase 75 pinned the `./` form, which is why it went green without reading the package. **Audit ONE package per invocation, in either spelling.**

**(d) ⚠️ `gofmt -l` NEVER exits non-zero.** Controller-executed on a deliberately unformatted tree:

```
gofmt -l exit=0
gofmt -l output: [internal/cluster/zz_bad.go]
```

Any gate written `gofmt -l . && echo PASS` is **inert**. Gate on OUTPUT: `[ "$(gofmt -l . | wc -l)" -eq 0 ]`.

**(e) ⚠️ A THIRD defect in phase 75's `impblock`, unrecorded anywhere in the lineage.** Its awk matches only `^import \($`. **Both `driver_test.go:3` and `internal/cluster/ringhash_test.go:3` are single-line `import "testing"`** — the helper emits **ZERO lines** for each, so it is structurally blind to **2 of phase 76's 3 `.go` files**, and its `BASE=n` figure cannot distinguish *"no imports"* from *"not parsed"*. Basename normalisation alone does NOT fix this row's gate. T9 carries a helper that handles both import forms.

### 1.2 ⚠️ THE SECOND HEADLINE — THE NAIVE RESCALE MANUFACTURES A **THIRD** VACUITY, AND NO DOCUMENT SAW IT

SPEC §3.4 gives final tuples and says each test was re-proved to fire its own leg. Re-executed here from scratch, and one of the SPEC's tuples is a trap that the SPEC's own final answer happens to avoid without saying why.

The obvious rescale of `_ScatterBitesAffinity`'s `{20, 28, 16}` is ×4 = **`{80, 112, 64}`** — and **80 = 16×5, 112 = 16×7, 64 = 16×4, all three multiples of `burstPerIP`.** That tuple PASSES affinity, falls through to the spread leg, still satisfies `err == nil` → `t.Fatal` is not reached, and **prints `--- PASS` while testing a completely different leg.** The one test that was *healthy* before the rescale would have been silently converted into a broken one.

⇒ **`{20, 108, 128}`** is used instead: sums to 256, `20 mod 16 = 4` and `108 mod 16 = 12` (one source IP's 16 conns split 4/12), `backend[0]` stays the first-failing element so the error string is structurally identical. **Measured, not reasoned** (§1.5 table).

### 1.3 ⚠️ THE SPEC'S VACUITY MECHANISM IS ONLY TWO-THIRDS RIGHT

SPEC §1.1 states: *"a tuple that no longer sums to `totalConns` trips conservation before reaching its own leg."* **Executed: false for affinity.** Re-read from the code at `driver.go:276-306`, the leg order is:

1. `:277` length · 2. **`:284` affinity — INSIDE the per-element loop** · 3. `:291` subject conservation — after the loop · 4. `:294` spread · 5. `:302` reference conservation

**Affinity is evaluated BEFORE conservation**, so `_ScatterBitesAffinity` fired its own leg at the raised constant (`subject affinity: backend[0]=20 not a multiple of 16`) and was **never vacuous**. `_SubjectConservation` also fired its own leg (`sum 48 != 256`) — its narrative was stale, not its assertion. **The SPEC's COUNT is right (exactly two vacuous tests, exactly the two it named); its stated REASON over-generalises.** The correct rule is leg-order-relative: *conservation shadows only the legs that FOLLOW it.*

### 1.4 SPEC §10's edit roster — CONFIRMED anchors, and FOUR errors

Re-derived first-hand against `98c27fc9`; `git diff 6ef436ac 98c27fc9` over every target path is **EMPTY**, so each error below is an error at the SPEC's own stated tip, not drift.

| # | SPEC §10 says | ACTUAL | settled by |
|---|---|---|---|
| **E1** | `README.md:58` carries a `64` | **`:57`** does; **`:58` is `across the three backends).` — no numeral at all** | `grep -n '\b64\b' …/README.md` |
| **E2** | — | **`README.md:114` MISSING** — `rq-per-cx) → 64` | same grep |
| **E3** | — | **`README.md:135` MISSING — and it carries TWO `64`s** — `(all 64 land on ONE backend, 64 % 16 == 0)` | same grep |
| **E4** | `driver_test.go` has 8 edit sites | **15.** Missing: `:8`, `:11`, `:28`, `:39`, **`:43`**, `:48`, **`:52`**. ⚠️ **`:43` and `:52` are executable `t.Fatal` STRING LITERALS**, and `:11` carries the file's only bare `4` | `grep -n '\b64\b'` (12 hits) + `grep -n '\b4\b'` (1 hit) |
| **E5** | `0062`/`0063` σ-band comment at `:300` | **`:299`** in both | `sed -n '297,302p'`, controller-run |
| **E6** | identifiers `ringCollapseHosts`/`allSame`/`pow3` "collision-checked repo-wide — unique" | **`allSame` has 4 hits, 3 of them live Go** in `internal/filter/http/router/hedge_test.go:494,498,502` | `grep -rn 'allSame' . --exclude-dir=.git` |
| **E7** | imports "grow by `math/rand` and `strconv`" | **`ringhash_test.go:3` is a single-line `import "testing"`** — a 1→N *structural conversion*, not an insertion. Actual delta is **`fmt`, `math`, `math/rand/v2`** | `sed -n '1,5p'` |
| **E8** | no insertion line given for the measurement | **after `:64`** (`_DistinctKeysSpread` ends `:64`; blank `:65`; `_WrapAround` begins `:66`) | file read |

E6's *conclusion* survives — the new test is `package cluster`, `grep -rn 'allSame' internal/cluster/` is empty — but its *stated evidence* is false, and an executor re-running the check will see 4 hits. **T1 renames the helper to `collapseAllSame` so the check is clean rather than merely explained.**

### 1.5 What this PLAN EXECUTED — the row is now largely BUILT

| item | status | evidence |
|---|---|---|
| the measurement test, compiled + run | **BUILT, GREEN** | `CONTROL K=4: 74/2000 rate=0.03700` · `MEASURED K=16: 0/2000` · `--- PASS (0.61s)` |
| break γ-SHARP (frozen ring) | **FIRED** | CONTROL RED, MEASURED **still green at 0/2000** — the null-result proof |
| break γ-restore | **GREEN after an observed RED** | `--- PASS (1.46s)` |
| break β (TWO-EDIT) | **FIRED** | `MEASURED leg K=4: collapse rate 0.03700 (74/2000) >= bar 0.001` |
| break β (SINGLE-EDIT) | **DID NOT FIRE — refutation re-confirmed** | fixture at 4, test at 16 ⇒ `--- PASS`, **exit 0** |
| the linkage gate, Go form | **BUILT + 2×2 cross-product** | (16,16)✅ (4,16)❌ (4,4)✅ (16,4)❌ — the (4,4)-passes arm proves it READS rather than hardcodes |
| `sourceIPs` 4→16 + `driver_test.go` rescale | **BUILT, 6/6 PASS** | `ok …/driver 0.002s` |
| each of 6 tests fires its OWN leg | **MEASURED per test** | §3 T3 before/after table |
| six anti-vacuity breaks, one per test | **ALL SIX FIRED, each in isolation** | §3 T3 Step 6 table |
| **`127.0.0.12`-`.17` bind probe — OWED, now DISCHARGED** | **16/16 PASS** | every requested `LocalAddr` == observed `LocalAddr`, full round-trip per arm |
| the collapse law over 4×10⁵ real ring draws | **MEASURED** | §1.6 |
| full differential at MASTER (pre-fix baseline) | **119/119 PASS, 408.8 s** | `0061` subtest 3.42 s, first attempt, no flake |
| `BEHAVIOR_CONTRACT.md:1326` rewrite | **VERIFIED IN SCRATCH** | 5746 → 5746 lines, `diff` = exactly `1326c1326` |
| **break α** | ✅ **EXECUTED — FIRED THE SPREAD LEG** | `runner_test.go:1293: distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` — byte-identical to the expected string; **affinity and conservation both SURVIVED**, as predicted |
| **full differential at K=16** | ✅ **119/119 PASS, 402.3 s**, `0061` subtest **3.30 s** | **−6.47 s (−1.58%)** vs the same-day pre-fix master baseline; **no hazard fired at all** |
| **`-race` over `./test/differential/`** | ✅ **ZERO data races** | two UNRELATED startup-flake failures, both isolate-green |
| **runtime delta, both arms same-session** | ✅ **NO MEASURABLE COST** | ⚠️ **this REFUTES SPEC §3.6's +0.158 s (+4.5%)** — see §1.7 |

### 1.6 ⚠️ `3^(1−K)` is a CONSERVATIVE UPPER BOUND — and the PLAN must not call it "the" probability

Measured over 200 000 real ring draws per K, using the real `newRingHashWithRNG` + `xxHash64` + `HashSourceIP`:

| K | **fixture's own keys** p̂ | **random keys per trial** p̂ | analytic `3^(1−K)` | ratio (fixture) |
|---|---|---|---|---|
| 4 | 3.515e-2 | 3.741e-2 | 3.704e-2 | **0.949** |
| 8 | 3.150e-4 | 5.050e-4 | 4.572e-4 | **0.689** |
| 16 | 0/200000 | 0/200000 | 6.969e-8 | unresolvable |

The discriminating arm (keys redrawn uniformly every trial) recovers **1.010 / 1.104** — so the deficit is a property of the *specific* `xxHash64` key set for `127.0.0.2…`, not of the ring builder. Direction is **safe**: the real fixture is *better* than the bound.

⚠️ **K=16 is NOT measured and cannot be at this scale.** Expected count at 2×10⁵ draws is 0.014; observing one collapse needs ~1.4×10⁷ draws (~1.2 h). `0/200000` bounds p̂ ≲ 1.5e-5 only. **The 7.0e-8 figure is analytic/extrapolated — state it as such.**

⚠️ Control count is **74/2000**, not the SPEC's **71/2000** (different RNG construction). The band `[0.015, 0.070]` holds either way, and the run is deterministic under `collapseSeed`, so this is a documentation correction, not a flake.

### 1.7 ⚠️ SPEC §3.6's RUNTIME COST IS REFUTED — there is no measurable cost, and break α found a SECOND thing

**The row is now fully executed end-to-end.** All four items SPEC §13 carried as NOT-EXECUTED were run at this PLAN.

**(a) The full differential suite at K=16 — 119/119 PASS.**

```
ok  github.com/pgdad/envoy-go/test/differential  402.326s
subtest PASS: 119 | FAIL: 0 | SKIP: 0
    --- PASS: TestDifferential/0061-lb-ring-hash (3.30s)
```

Against the **same-day, pre-fix master baseline** (119/119, 408.8 s, `0061` 3.30→3.42 s): **−6.47 s (−1.58%)** at package scale, **−0.12 s (−3.5%)** on the subtest. **No hazard fired at all** — no `subject ready: EOF`, no `bind: address already in use`, no SDS dial-budget failure, and **`0061` did not fire a spread failure.**

**(b) The runtime delta, both arms measured in ONE session (3 warm runs each).**

| arm | wall mean | `go`-reported mean |
|---|---|---|
| K=16 (256 conns) | **4.410 s** | **3.6827 s** |
| K=4 (64 conns, temp revert) | **4.463 s** | **3.6997 s** |

Delta **−0.053 s (−1.2%)** wall, **−0.017 s (−0.46%)** reported. ⚠️ **The delta is NEGATIVE and smaller than the K=4 arm's own spread** (one 4.59 s outlier drives the sign single-handedly).

⇒ ⚠️ **SPEC §3.6's *"the honest cost is +0.158 s (+4.5%)"* is REFUTED at this tip.** The honest reading is **no measurable runtime cost from K=4 → K=16 — and NOT a speedup either.** The SPEC's methodological rule survives its own number: *quote the delta against a same-session control*. This measurement did exactly that and got a different answer, which is what a same-session control is for. **Do not carry +4.5% forward.**

**(c) Break α fired the SPREAD leg — and found a second thing nobody asked about.**

```
runner_test.go:1293: distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)
--- FAIL: TestDifferential/0061-lb-ring-hash (4.45s)
```

Byte-identical to the expected string. The five legs are sequential `return fmt.Errorf` sites at `:278`/`:285`/`:292`/**`:295`**/`:303`; the text that fired is **`:295`, the spread leg** — affinity and conservation are upstream in the same function and would have short-circuited first had they fired. They did not: counts collapse to `[256,0,0]`, `256 % 16 == 0` so affinity survives, and the sum is unchanged so conservation survives. **The break is neither vacuous nor mis-targeted.** Restore ⇒ isolated re-run `--- PASS (3.67s)`: a green **following an observed red**.

⚠️ **NEW FINDING — the unit package DOES catch break α, but by a single test out of 403.** `TestRingHash_DistinctKeysSpread` fires (`ringhash_test.go:62: 200 distinct keys covered only 1 endpoints (degenerate ring?)`) while **8 of the 9 `TestRingHash_*` tests PASS under a total ring collapse** — including `SameKeySameEndpoint` and `WrapAround`, which one might expect to be sensitive. ⇒ **the differential fixture is a genuine SECOND guard, not redundant coverage, and the unit-level guard is a single point of failure.** Neither the SPEC nor this PLAN's first draft asked the question.

**(d) `-race` over `./test/differential/` — ZERO data races**, `grep -c 'WARNING: DATA RACE'` ⇒ **0**. Two failures, **both on fixtures unrelated to this row** (`0009-admin-config-dump`, `0084-otlp-access-log`), both at `waitTCPDial` backend-readiness **before any assertion executed** (`runner_test.go:342`/`:360` are pre-drive readiness gates, not assertion sites) — the recorded startup-flake signature. **Both isolate-green under `-race`.** `0061` appears in **zero** FAIL lines.

⚠️ **NEITHER was on this PLAN's own hazard roster by name — the fourth consecutive stage at which that has been true.** *A stage brief's flake list is not the index.*
⚠️ **NEW OBSERVATION: the startup flake fires MORE readily under `-race`** (2 hits with, 0 without, same session) — plausibly the detector's slowdown pushing container boot past the 5 s `waitTCPDial` budget. Recorded, not chartered.

### 1.8 Other corrections this PLAN owes forward

- **`driver.go:80`'s trailing comment `// 64 — the conservation target` sits ON the line that now evaluates to 256.** A landmine directly on the edited const. T2 fixes it in the same task.
- **`README.md:81`'s `multinomial(64, 1/3)` and `{0,16,32,48,64}` MUST change.** SPEC §7 says the `< 1%` claim *"SURVIVES — do not delete"*, which invites leaving `:81` byte-untouched. The **bound** survives (0.096% → 0.381%, still `< 1%`); the **numerals** do not.
- **`README.md:145`'s `-count=20 → 20/20 PASS (66 s)` is stale in TWO ways** — it was measured at a superseded constant, and the workload is now 4× larger, so the wall-clock is wrong too. It must be **struck**, not re-measured: a pass-count is what this row exists to abolish (`reference_0061_ring_hash_spread_flake`).
- **`ROADMAP.md:138` (row 76) is BRAINSTORM-era and carries FOUR stale claims** the SPEC refuted: `sourceIPs 4 → 10`, `K=10 → 5.1e-5`, β as a single-edit *"THE PROOF"*, and *"THE EXECUTABLE DELTA IS ONE INTEGER … 7 hits, EVERY ONE A COMMENT"*. The IMPL's row-flip owes these corrections (phase-75 precedent: that row's cell carried five).
- ⚠️ **The phase-75 sha256 byte-untouched roster CANNOT be copied forward.** `internal/cluster/**` was byte-gated at phase 75 (`PLAN.md:243`, `:322`) and **phase 76 EDITS `internal/cluster/ringhash_test.go`.** `comm -12` over the two rosters returns exactly that file. Copying the glob reproduces the phase-73 SEVERE-1 (`reference_plan_schedules_edits_to_a_byte_gated_file`). T9's roster is **production-only, 375 files**, and its intersection with the edit roster is **EMPTY**.
- **`STATE.md` singleton greps — the router's *"each returns 2"* is right for only ONE of three.** Executed: `lifecycle-state` ⇒ **5** (`:7` rule · `:17` live · `:46`/`:48`/`:50` §Recent recaps) · `next-skill` ⇒ **2** ✓ · `next-free ADR` ⇒ **3**. The invariant that actually holds is `:7`'s own wording — exactly one **live** instance, in §Current. **Never "fix" any of these counts down.**

---

## 2. Global constraints

Every task's requirements implicitly include this section.

- **+0 PRODUCTION `.go` FILES.** No file outside `_test.go` and `test/` may change. This is the row's defining envelope and T9 gates it.
- **+0 stats (1205 → 1205) · +0 fixtures (119) · +0 fuzzers (55) · +0 BackendKinds (tail 38) · +0 go.mod modules (2) · +0 new exported symbols · +0 production imports · ZERO new packages.**
- **`burstPerIP` MUST NOT MOVE.** It is the affinity leg's discriminating modulus (`driver.go:284`). Every `% 16 == 0` and `all-16-or-0` in prose STAYS 16.
- **`totalConns` is DERIVED** (`sourceIPs * burstPerIP`) — it tracks to 256 with no second code edit. Its *trailing comment* does not.
- **`D-S36-4` is a token ID, not a numeral.** It occurs at `driver.go:20`, `expectations.yaml:21`, `README.md:52`/`:74`/`:153`. A careless `s/4/16/` corrupts these to `D-S36-16`.
- **`driver.go:405`'s `64` is `strconv.ParseUint`'s bitSize — CODE. Do not touch.**
- **Break protocol:** `-count=1` on every run (`reference_differential_break_protocol_count1`); run breaks **AFTER committing** (`reference_break_protocol_commit_first`); revert with `git restore`, **never** `git checkout <sha>` (detaches HEAD); **confirm WHICH assertion fired**, never just that something went red (`reference_deliberate_break_wrong_assertion`).
- **Worktree discipline:** `git -C <abs-worktree-path>` for EVERY git command. The Bash cwd silently resets to the repo root — **it fired again during this PLAN**. Tripwire `pwd` + `git rev-parse --abbrev-ref HEAD` (must be the stage branch, NEVER `master`) + `git rev-list --count master..HEAD` before any commit or gate run.
- **Per-task hygiene:** `gofmt -l <pkg>` (gate on OUTPUT — §1.1(d)), `go vet ./<pkg>/...`, `golangci-lint run ./<pkg>/...`.
- **Identifier roster (collision-checked at this PLAN, all 0 Go hits):** `collapseTrials`, `collapseSeed`, `collapseBar`, `collapseFixtureK`, `collapseControlK`, `collapseControlLo`, `collapseControlHi`, `collapseEphemeralLo`, `collapseEphemeralHi`, `collapseDrawPorts`, `collapseSourceKeys`, `collapseAllSame`, `collapseFixtureKSourcePath`, `TestRingHash_EphemeralPortRing_KeyCollapseRate`, `TestSourceIPsLinkedToCollapseFixtureK`.

---

## 3. File structure

```
internal/cluster/ringhash_test.go                    [EDIT] T1  import block 1→N lines; +the measurement after :64
test/fixtures/0061-lb-ring-hash/driver/driver.go     [EDIT] T2  the ONE code line (:78) + :80's comment; T5 ~11 comment sites
test/fixtures/0061-lb-ring-hash/driver/driver_test.go[EDIT] T3  15 sites: 6 tuples + 7 comments + 2 t.Fatal strings
test/fixtures/0061-lb-ring-hash/driver/linkage_test.go [NEW] T4  the go/parser linkage gate
test/fixtures/0061-lb-ring-hash/expectations.yaml    [EDIT] T5  :5 :6 :21-22 :25 :26 :27 :28 :32 :40
test/fixtures/0061-lb-ring-hash/README.md            [EDIT] T6  ~22 sites incl. :57 :81 :114 :135 :143-148
docs/envoy-go/BEHAVIOR_CONTRACT.md                   [EDIT] T7  line 1326 ONLY, in place, no line-count change
docs/envoy-go/DECISIONS.md                           [EDIT] T9  ADR-0298 PROPOSED → COMPLETE
docs/envoy-go/ROADMAP.md                             [EDIT] T9  row 76 → done + FOUR stale-claim corrections
docs/envoy-go/STATE.md, next-prompt.txt              [EDIT] T9  stage close

BYTE-UNTOUCHED (sha256 roster verified at T9 — production-only, 375 files):
  every .go outside test/ and outside *_test.go, plus go.mod, go.sum
  ⚠️ NOT the phase-75 roster: internal/cluster/** is NOT byte-gated this row.
```

---

## Task 1 — the collapse-rate MEASUREMENT, with its stacked non-vacuity CONTROL

**Files:** Modify `internal/cluster/ringhash_test.go` (import block at `:3`; new test after `:64`)

**Interfaces:** Consumes `newRingHashWithRNG`, `ringHashCfg`, `hashXX`, `Endpoint`, `HashSourceIP` — all reachable from `package cluster` with **nothing exported**. Produces `collapseFixtureK` (read by T4's linkage gate via `go/parser`).

⚠️ **THE MEASURED LEG IS A NULL RESULT STANDING ALONE.** At M=2000, K=16 the expected collapse count is **0.00014** — so `0/2000` is *also* exactly what a frozen ring, a stubbed builder or a broken detector reports. **The stacked K=4 control leg over the SAME M ring draws is what converts it into evidence**, and Step 4 MEASURES that rather than arguing it. Any wording of the form *"the unit test proves the ring randomizes"* is true only of the CONTROL leg.

⚠️ **`eps(n)` (`internal/cluster/leastrequest_test.go:19`) MUST NOT be used** — it builds FIXED hosts `a,b,c` on FIXED ports 1000-1002, which is precisely the frozen-ring posture this test must avoid. It holds fixed the exact variable the fixture randomizes.

⚠️ **`:3` is `import "testing"`, a SINGLE LINE** (SPEC §4 error E7). This is a structural 1→N conversion.

- [ ] **Step 1 — convert the import block**

Replace line 3 `import "testing"` with:

```go
import (
	"fmt"
	"math"
	mathrand "math/rand/v2"
	"testing"
)
```

- [ ] **Step 2 — insert the measurement after `TestRingHash_DistinctKeysSpread`**

Re-derive the insertion point first: `_DistinctKeysSpread` must end at `:64` and `_WrapAround` begin at `:66` **before** the import edit; after it, +5 lines. Insert between them.

```go
// Pinned parameters for TestRingHash_EphemeralPortRing_KeyCollapseRate (phase 76,
// the 0061-lb-ring-hash spread flake). collapseFixtureK MUST track sourceIPs in
// test/fixtures/0061-lb-ring-hash/driver/driver.go: nothing in the build links the
// two, so the linkage is a REVIEW gate, not a compile-time one.
const (
	collapseTrials   = 2000     // M independent ring draws, shared by BOTH legs
	collapseSeed     = 20260725 // deterministic pseudo-ephemeral-port stream
	collapseBar      = 1e-3     // MEASURED-leg ceiling on the observed collapse rate
	collapseFixtureK = 16       // MUST equal sourceIPs in the 0061 driver
	collapseControlK = 4        // the PRE-phase-76 sourceIPs value

	// CONTROL-leg acceptance band around the analytic 3^(1-4) = 3.70e-2
	// (expected ~74/2000).
	collapseControlLo = 0.015
	collapseControlHi = 0.070

	// Pseudo-ephemeral port draw range: the Linux default
	// net.ipv4.ip_local_port_range, i.e. the space the differential harness's
	// "0.0.0.0:0" backend binds land in (test/differential/runner_test.go).
	collapseEphemeralLo = 32768
	collapseEphemeralHi = 60999
)

// collapseDrawPorts draws 3 DISTINCT pseudo-ephemeral ports from rng, modeling
// one fresh run of the 0061 harness (three TCPEcho backends bound to "0.0.0.0:0").
func collapseDrawPorts(rng *mathrand.Rand) [3]uint32 {
	var ports [3]uint32
	for i := range ports {
		for {
			p := uint32(collapseEphemeralLo + rng.IntN(collapseEphemeralHi-collapseEphemeralLo+1))
			dup := false
			for j := 0; j < i; j++ {
				if ports[j] == p {
					dup = true
					break
				}
			}
			if !dup {
				ports[i] = p
				break
			}
		}
	}
	return ports
}

// collapseSourceKeys returns the ring_hash keys the 0061 driver produces for its
// first k source IPs (127.0.0.2 .. 127.0.0.(1+k)), via the REAL exported producer
// HashSourceIP — which STRIPS THE PORT, so the number of DISTINCT keys is exactly
// the driver's sourceIPs constant however many connections each source IP opens.
func collapseSourceKeys(k int) []uint64 {
	keys := make([]uint64, k)
	for i := range keys {
		keys[i] = HashSourceIP(fmt.Sprintf("127.0.0.%d:40000", 2+i))
	}
	return keys
}

// collapseAllSame reports whether every entry of addrs is the same string — the
// 0061 spread failure mode: all K distinct source-IP keys landing on ONE backend.
func collapseAllSame(addrs []string) bool {
	if len(addrs) == 0 {
		return true
	}
	for _, a := range addrs[1:] {
		if a != addrs[0] {
			return false
		}
	}
	return true
}

// TestRingHash_EphemeralPortRing_KeyCollapseRate MEASURES the per-run probability
// that the 0061-lb-ring-hash fixture's spread assertion (">= 2 backends nonzero")
// collapses.
//
// The 0061 ring is keyed on the backend's "IP:PORT" string (newRingHashWithRNG →
// endpoints[j].Addr()) and the harness binds every backend to "0.0.0.0:0", so the
// ring is a FRESH RANDOM 3-way partition of the hash space on every run. The number
// of DISTINCT hash keys is the driver's sourceIPs constant, because HashSourceIP
// strips the port — burstPerIP connections per source IP all reduce to ONE key. A
// run "collapses" when all K keys fall into one backend's arc: analytically
// 3*(1/3)^K = 3^(1-K), i.e. 3.7e-2 at K=4 and 7.0e-8 at K=16.
//
// TWO legs over the SAME collapseTrials ring draws:
//
//   - CONTROL (K=collapseControlK=4) — an ANTI-VACUITY leg. It asserts the observed
//     rate lands inside a band that only a genuinely re-randomized ring can hit. If
//     the ring stops being redrawn per trial, this leg reports 0 and goes RED, which
//     is the ONLY thing distinguishing "K=16 is safe" from "the harness measured
//     nothing".
//   - MEASURED (K=collapseFixtureK=16) — the result: the rate must sit below
//     collapseBar.
//
// Both legs use t.Errorf (never t.Fatalf) so a CONTROL failure still prints the
// MEASURED number.
func TestRingHash_EphemeralPortRing_KeyCollapseRate(t *testing.T) {
	maxK := collapseFixtureK
	if collapseControlK > maxK {
		maxK = collapseControlK
	}
	keys := collapseSourceKeys(maxK)

	rng := mathrand.New(mathrand.NewPCG(collapseSeed, collapseSeed))
	addrs := make([]string, maxK)
	var controlCollapses, measuredCollapses int

	for trial := 0; trial < collapseTrials; trial++ {
		ports := collapseDrawPorts(rng)
		endpoints := make([]Endpoint, len(ports))
		for i, p := range ports {
			endpoints[i] = Endpoint{Host: "127.0.0.1", Port: p}
		}
		// The REAL ring builder with the 0061 fixture's `ring_hash_lb_config: {}`
		// defaults (1024 / 8388608 / XX_HASH → 342 points per host, 1026 total —
		// the exact gauges the fixture asserts). The injected rng is never
		// consulted: every Pick below passes hasHash=true.
		rh := newRingHashWithRNG(endpoints,
			ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX},
			func() uint64 { return 0 })

		for i, key := range keys {
			ep, _, err := rh.Pick(key, true, SubsetMatch{}, false)
			if err != nil {
				// Harness precondition, not one of the two legs: a Pick error means
				// the ring was built empty and no rate is measurable at all.
				t.Fatalf("trial %d: Pick(keys[%d]): %v", trial, i, err)
			}
			addrs[i] = ep.Addr()
		}
		if collapseAllSame(addrs[:collapseControlK]) {
			controlCollapses++
		}
		if collapseAllSame(addrs[:collapseFixtureK]) {
			measuredCollapses++
		}
	}

	controlRate := float64(controlCollapses) / float64(collapseTrials)
	measuredRate := float64(measuredCollapses) / float64(collapseTrials)
	controlExpect := 3 * math.Pow(1.0/3.0, collapseControlK)
	measuredExpect := 3 * math.Pow(1.0/3.0, collapseFixtureK)

	t.Logf("CONTROL  K=%d: %d/%d collapses, rate=%.5f | analytic 3^(1-K)=%.3e → expected ~%.2f/%d | band [%g, %g]",
		collapseControlK, controlCollapses, collapseTrials, controlRate,
		controlExpect, controlExpect*collapseTrials, collapseTrials,
		collapseControlLo, collapseControlHi)
	t.Logf("MEASURED K=%d: %d/%d collapses, rate=%.5f | analytic 3^(1-K)=%.3e → expected ~%.5f/%d | bar %g",
		collapseFixtureK, measuredCollapses, collapseTrials, measuredRate,
		measuredExpect, measuredExpect*collapseTrials, collapseTrials, collapseBar)

	if controlRate < collapseControlLo || controlRate > collapseControlHi {
		t.Errorf("CONTROL leg K=%d: collapse rate %.5f (%d/%d) OUTSIDE band [%g, %g] "+
			"(analytic 3^(1-K)=%.3e). A rate of 0 HERE means the ring is NO LONGER BEING "+
			"REDRAWN PER TRIAL — the pseudo-ephemeral ports were frozen or the builder was "+
			"stubbed — and in that case the MEASURED leg below is VACUOUS: it reports 0 "+
			"collapses because nothing varies, NOT because K=%d makes collapse improbable.",
			collapseControlK, controlRate, controlCollapses, collapseTrials,
			collapseControlLo, collapseControlHi, controlExpect, collapseFixtureK)
	}

	if measuredRate >= collapseBar {
		t.Errorf("MEASURED leg K=%d: collapse rate %.5f (%d/%d) >= bar %g "+
			"(analytic 3^(1-K)=%.3e). K is the number of DISTINCT ring_hash keys the 0061 "+
			"fixture drives, i.e. the sourceIPs constant in "+
			"test/fixtures/0061-lb-ring-hash/driver/driver.go — if this leg fires, sourceIPs "+
			"SHRANK and the fixture's spread assertion (>= 2 backends nonzero) is flaky again "+
			"at this rate.",
			collapseFixtureK, measuredRate, measuredCollapses, collapseTrials, collapseBar,
			measuredExpect)
	}
}
```

- [ ] **Step 3 — run GREEN**

```bash
go test ./internal/cluster/ -run 'TestRingHash_EphemeralPortRing_KeyCollapseRate' -count=1 -v
```

**[RUN at this PLAN]** Observed verbatim:

```
=== RUN   TestRingHash_EphemeralPortRing_KeyCollapseRate
    ringhash_test.go:214: CONTROL  K=4: 74/2000 collapses, rate=0.03700 | analytic 3^(1-K)=3.704e-02 → expected ~74.07/2000 | band [0.015, 0.07]
    ringhash_test.go:218: MEASURED K=16: 0/2000 collapses, rate=0.00000 | analytic 3^(1-K)=6.969e-08 → expected ~0.00014/2000 | bar 0.001
--- PASS: TestRingHash_EphemeralPortRing_KeyCollapseRate (0.61s)
ok  	github.com/pgdad/envoy-go/internal/cluster	0.613s
```

- [ ] **Step 4 — hygiene + commit (BEFORE any break)**

```bash
[ "$(gofmt -l internal/cluster/ | wc -l)" -eq 0 ] && echo "gofmt clean"
go vet ./internal/cluster/ && golangci-lint run ./internal/cluster/...
git -C <abs-worktree-path> commit -am "phase 76 T1: seeded collapse-rate MEASUREMENT with a stacked K=4 non-vacuity control"
```

⚠️ `golangci-lint`'s `misspell` linter fired at this PLAN on `modelling`/`randomised`. Use US spellings.

- [ ] **Step 5 — BREAK γ-SHARP (ANTI-VACUITY — the load-bearing break of this task)**

Edit, inside the trial loop:

```go
		_ = collapseDrawPorts(rng)                 // BREAK γ-SHARP: draw, then DISCARD
		ports := [3]uint32{40001, 40002, 40003}    // FROZEN ring — identical every trial
```

Run: `go test ./internal/cluster/ -run 'TestRingHash_EphemeralPortRing_KeyCollapseRate' -count=1 -v`

**MUST fire: the CONTROL leg, and ONLY it. The MEASURED leg MUST STAY GREEN at `0/2000`** — that is the entire point. **[RUN at this PLAN — fired exactly so]**:

```
    ringhash_test.go:215: CONTROL  K=4: 0/2000 collapses, rate=0.00000 | …
    ringhash_test.go:219: MEASURED K=16: 0/2000 collapses, rate=0.00000 | …
    ringhash_test.go:224: CONTROL leg K=4: collapse rate 0.00000 (0/2000) OUTSIDE band [0.015, 0.07] … the MEASURED leg below is VACUOUS: it reports 0 collapses because nothing varies, NOT because K=16 makes collapse improbable.
--- FAIL: TestRingHash_EphemeralPortRing_KeyCollapseRate (0.60s)
exit=1
```

Both `Logf` lines and the measured number survived the control failure — the `t.Errorf`-not-`Fatalf` choice, demonstrated (`reference_fatalf_makes_assertions_unreachable`).

- [ ] **Step 6 — BREAK γ-RESTORE**

`git restore internal/cluster/ringhash_test.go`; re-run. **[RUN — `--- PASS (1.46s)`, exit 0.]** A green that FOLLOWS an observed red means *"ran and passed"*; a green with no red baseline means nothing (`reference_liveness_break_needs_failing_baseline`).

---

## Task 2 — `sourceIPs` 4 → 16, and the comment sitting on the line it falsifies

**Files:** Modify `test/fixtures/0061-lb-ring-hash/driver/driver.go` (`:78`, `:80`)

**Interfaces:** Consumes nothing. Produces `totalConns == 256` (derived) and `sourceIPs == 16`, read by T3's tuples and T4's linkage gate.

⚠️ **This task is EXPECTED to leave the tree RED. That red is T3's input, not a failure.**

- [ ] **Step 1 — record the pre-edit baseline (this is what makes T3's finding findable)**

```bash
go test ./test/fixtures/0061-lb-ring-hash/driver/ -count=1 -v
```
**[RUN at this PLAN]** — 6/6 PASS, `ok … 0.002s`. All six green *before* the constant moves.

- [ ] **Step 2 — the one code edit, plus the comment on the next-but-one line**

```diff
-	sourceIPs  = 4                      // 127.0.0.2 .. 127.0.0.5
+	sourceIPs  = 16                     // 127.0.0.2 .. 127.0.0.17
 	burstPerIP = 16                     // connections per source IP
-	totalConns = sourceIPs * burstPerIP // 64 — the conservation target
+	totalConns = sourceIPs * burstPerIP // 256 — the conservation target
```

⚠️ **`burstPerIP` MUST NOT MOVE.** ⚠️ `:196`'s `net.IPv4(127, 0, 0, byte(2+s))` inside `for s := 0; s < sourceIPs; s++` is **self-scaling — NO EDIT** (this is the mechanical reason the code delta really is one integer).

- [ ] **Step 3 — confirm the derived constant BY MEASUREMENT, not arithmetic**

A throwaway probe printing the three consts. **[RUN at this PLAN]**: `PROBE-CONSTS sourceIPs=16 burstPerIP=16 totalConns=256`.

- [ ] **Step 4 — run the fixture's own package and record the RED**

```bash
go test ./test/fixtures/0061-lb-ring-hash/driver/ -count=1 -v
```

**[RUN at this PLAN]** — verbatim:

```
=== RUN   TestAssertDistribution_Affinity
    driver_test.go:14: expected pass for an affine subject + conserving reference, got: subject conservation: sum 64 != 256
--- FAIL: TestAssertDistribution_Affinity (0.00s)
=== RUN   TestAssertDistribution_ScatterBitesAffinity
--- PASS ...   (and four more --- PASS)
FAIL	github.com/pgdad/envoy-go/test/fixtures/0061-lb-ring-hash/driver	0.001s
```

⚠️ **ONLY ONE test fails, and five print `--- PASS`. That is the trap.** Two of those five are now VACUOUS. **Do not proceed to a green suite from here — go to T3.**

⚠️ **DO NOT confirm this task with `git diff --stat`, a file-scoped `grep`, or `go test ./test/differential/`.** All three were run in this lineage and all three were blind in the same direction: `--stat` measures what was *edited*, a file-scoped grep cannot see a sibling file, and the differential suite **never compiles the driver package's `_test.go`**. **A change-set measure is not a build measure — run the PACKAGE** (`reference_change_set_measure_not_build_measure`).

- [ ] **Step 5 — commit (red is expected and recorded)**

```bash
git -C <abs-worktree-path> commit -am "phase 76 T2: sourceIPs 4 -> 16 (totalConns 64 -> 256); driver_test.go goes RED, T3's input"
```

---

## Task 3 — rescale `driver_test.go`, and re-prove each of six tests fires its OWN leg

**Files:** Modify `test/fixtures/0061-lb-ring-hash/driver/driver_test.go` — **15 sites, not the SPEC's 8** (§1.4 E4)

**Interfaces:** Consumes T2's `totalConns == 256` and `burstPerIP == 16`. Produces nothing downstream.

⚠️ **`_CollapseBitesSpread` is the unit test for THIS ROW'S OWN ASSERTION, and it had silently stopped testing spread.** A rescale is not complete when the suite goes green — it is complete when each test has been **re-proved BY MEASUREMENT to fire its own leg**.

⚠️ **DO NOT rescale the scatter tuple by ×4** — §1.2. `{20,28,16}×4 = {80,112,64}` are all multiples of 16 and would manufacture a **third** vacuity.

⚠️ **`:43` and `:52` are executable `t.Fatal` string literals**, not comments.

- [ ] **Step 1 — MEASURE which leg each test actually fires, BEFORE the rescale**

Write a temporary `zzprobe_test.go` in the driver package calling `AssertDistribution` with each test's exact tuples and printing the returned error. **[RUN at this PLAN]** — verbatim:

```
PROBE-CONSTS sourceIPs=16 burstPerIP=16 totalConns=256
PROBE-LEG Affinity               ref=[64 0 0] subj=[32 16 16] -> subject conservation: sum 64 != 256
PROBE-LEG ScatterBitesAffinity   ref=[64 0 0] subj=[20 28 16] -> subject affinity: backend[0]=20 not a multiple of 16 (key scattered? a source IP split across backends)
PROBE-LEG CollapseBitesSpread    ref=[64 0 0] subj=[64 0 0] -> subject conservation: sum 64 != 256
PROBE-LEG SubjectConservation    ref=[64 0 0] subj=[16 16 16] -> subject conservation: sum 48 != 256
PROBE-LEG ReferenceConservation  ref=[32 0 0] subj=[32 16 16] -> subject conservation: sum 64 != 256
PROBE-LEG WrongLength            ref=[64 0] subj=[32 16 16] -> expected 3 backend counts, got ref=2 subj=3
```

⇒ **`_CollapseBitesSpread` fires CONSERVATION, not spread. `_ReferenceConservation` fires SUBJECT conservation, not reference.** Both printed `--- PASS`. **Exactly two vacuous, exactly the two the SPEC named** — but for a leg-order reason the SPEC states too broadly (§1.3).

- [ ] **Step 2 — rewrite the file**

Final content, verbatim (compiled and run at this PLAN):

```go
package driver

import "testing"

// TestAssertDistribution_Affinity: a representative subject distribution where each
// per-backend count is a multiple of burstPerIP (16) and >= 2 backends are nonzero
// (the consistent-hash affinity+spread invariant) passes, with the reference held to
// conservation only (single-key pin → all 256 on one backend).
func TestAssertDistribution_Affinity(t *testing.T) {
	d := ringHashDriver{}
	// subject: 16 source IPs over 3 backends, e.g. eight IPs → backend[0] (128), four
	// each → backend[1]/backend[2] (64/64). reference: single-key pin {256,0,0}.
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{128, 64, 64}); err != nil {
		t.Fatalf("expected pass for an affine subject + conserving reference, got: %v", err)
	}
}

// TestAssertDistribution_ScatterBitesAffinity: a subject count NOT a multiple of 16
// (a source IP split across backends — the scatter break) FAILS the affinity leg.
func TestAssertDistribution_ScatterBitesAffinity(t *testing.T) {
	d := ringHashDriver{}
	// {20, 108, 128}: sum 256 (conserves) but 20 and 108 are not multiples of 16 —
	// one source IP's 16 conns split 4/12 across backend[0] and backend[1].
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{20, 108, 128}); err == nil {
		t.Fatal("expected affinity failure on a scattered subject distribution")
	}
}

// TestAssertDistribution_CollapseBitesSpread: a subject distribution with all 256 on
// ONE backend (a collapsed ring) conserves AND is a multiple of 16, but only ONE
// backend is nonzero → FAILS the spread leg.
func TestAssertDistribution_CollapseBitesSpread(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{256, 0, 0}); err == nil {
		t.Fatal("expected spread failure on a collapsed (single-backend) subject distribution")
	}
}

// TestAssertDistribution_SubjectConservation: a subject distribution of all-multiples
// of 16 that does NOT sum to 256 fails the conservation leg.
func TestAssertDistribution_SubjectConservation(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0, 0}, []uint64{64, 64, 64}); err == nil {
		t.Fatal("expected conservation failure on a sub-256 subject sum (192)")
	}
}

// TestAssertDistribution_ReferenceConservation: a reference distribution that does
// not sum to 256 fails the reference conservation leg (the only reference check).
func TestAssertDistribution_ReferenceConservation(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{128, 0, 0}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected conservation failure on a sub-256 reference sum (128)")
	}
}

// TestAssertDistribution_WrongLength: a non-3 count slice fails.
func TestAssertDistribution_WrongLength(t *testing.T) {
	d := ringHashDriver{}
	if err := d.AssertDistribution([]uint64{256, 0}, []uint64{128, 64, 64}); err == nil {
		t.Fatal("expected error on wrong-length reference counts")
	}
}
```

- [ ] **Step 3 — re-run the probe and prove EACH test now fires its own leg**

**[RUN at this PLAN]** — verbatim:

```
PROBE-LEG Affinity               ref=[256 0 0] subj=[128 64 64] -> <nil>
PROBE-LEG ScatterBitesAffinity   ref=[256 0 0] subj=[20 108 128] -> subject affinity: backend[0]=20 not a multiple of 16 (key scattered? a source IP split across backends)
PROBE-LEG CollapseBitesSpread    ref=[256 0 0] subj=[256 0 0] -> subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)
PROBE-LEG SubjectConservation    ref=[256 0 0] subj=[64 64 64] -> subject conservation: sum 192 != 256
PROBE-LEG ReferenceConservation  ref=[128 0 0] subj=[128 64 64] -> reference conservation: sum 128 != 256
PROBE-LEG WrongLength            ref=[256 0] subj=[128 64 64] -> expected 3 backend counts, got ref=2 subj=3
```

**Six for six, each on its own leg.** Then **DELETE `zzprobe_test.go`** and confirm `ls` shows only `driver.go`, `driver_test.go`.

- [ ] **Step 4 — full-package green**

```bash
go test ./test/fixtures/0061-lb-ring-hash/driver/ -count=1 -v
```
**[RUN at this PLAN]** — 6/6 PASS, `ok … 0.002s`.

- [ ] **Step 5 — hygiene + commit (before the breaks)**

```bash
[ "$(gofmt -l test/fixtures/0061-lb-ring-hash/driver/ | wc -l)" -eq 0 ] && echo clean
go vet ./test/fixtures/0061-lb-ring-hash/... && golangci-lint run ./test/fixtures/0061-lb-ring-hash/...
git -C <abs-worktree-path> commit -am "phase 76 T3: rescale driver_test.go 64->256 and re-prove all six legs fire their own assertion"
```

- [ ] **Step 6 — SIX ANTI-VACUITY BREAKS, one per test**

One at a time, `git restore` between, `-count=1`, anchor uniqueness asserted before each edit. **Each MUST produce exactly ONE red test — its own — with the other five green.** **[ALL SIX RUN at this PLAN]**:

| # | test | break to `driver.go` | FAIL line, verbatim |
|---|---|---|---|
| 1 | `_Affinity` | `if c%burstPerIP != 0 {` → `if c%burstPerIP == 0 && c > 0 {` | `driver_test.go:14: expected pass …, got: subject affinity: backend[0]=128 not a multiple of 16 (key scattered? …)` |
| 2 | `_ScatterBitesAffinity` | `if c%burstPerIP != 0 {` → `if c%1 != 0 {` | `driver_test.go:25: expected affinity failure on a scattered subject distribution` |
| 3 | `_CollapseBitesSpread` | `if nonzero < 2 {` → `if nonzero < 1 {` | `driver_test.go:35: expected spread failure on a collapsed (single-backend) subject distribution` |
| 4 | `_SubjectConservation` | `if subjSum != totalConns {` → `if subjSum > 1000000 {` | `driver_test.go:44: expected conservation failure on a sub-256 subject sum (192)` |
| 5 | `_ReferenceConservation` | `if refSum != totalConns {` → `if refSum > 1000000 {` | `driver_test.go:53: expected conservation failure on a sub-256 reference sum (128)` |
| 6 | `_WrongLength` | `if len(subjCounts) != 3 \|\| len(refCounts) != 3 {` → `if len(subjCounts) != 3 {` | `driver_test.go:61: expected error on wrong-length reference counts` |

**Break 1 is the strongest line in this task:** it is the *positive* test's anti-vacuity proof, and its message names the **affinity** leg explicitly — i.e. `_Affinity` genuinely observes affinity's verdict rather than merely observing "no error from anywhere". After all six, re-verify baseline green and `git status --short` empty.

---

## Task 4 — the LINKAGE gate: a `go/parser` Go test, plus a hardened shell fallback

**Files:** Create `test/fixtures/0061-lb-ring-hash/driver/linkage_test.go`

**Interfaces:** Consumes `sourceIPs` (own package, unexported) and parses `collapseFixtureK` out of `internal/cluster/ringhash_test.go`. Produces nothing. **Adds ZERO exported symbols on either side.**

⚠️ **This is what break β CANNOT close.** β is a **TWO-EDIT** break (revert both constants); executed at this PLAN it fires correctly, but it proves only that the *unit test's* constant is load-bearing. **The single-edit case — fixture reverted, unit test not — is the drift a future session actually commits, and nothing in the tree detects it.** Re-confirmed by execution at this tip: fixture at 4, `collapseFixtureK` at 16 ⇒ `--- PASS`, **exit 0**.

⚠️ **The SPEC's shell gate is broken in BOTH directions** (§1.1 a+b). Do not land it.

⚠️ **A comment is not a mechanism** (`reference_code_comment_not_evidence`). `// collapseFixtureK MUST equal sourceIPs` is prose; this task is the mechanism.

- [ ] **Step 1 — write the gate**

```go
package driver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// collapseFixtureKSourcePath is the file declaring internal/cluster's
// collapseFixtureK. It is relative to THIS package's directory
// (test/fixtures/0061-lb-ring-hash/driver — `go test` runs each test binary with
// its own package dir as the working directory); four levels up is the repo root.
const collapseFixtureKSourcePath = "../../../../internal/cluster/ringhash_test.go"

// TestSourceIPsLinkedToCollapseFixtureK is the phase-76 LINKAGE gate in pure Go.
//
// internal/cluster's TestRingHash_EphemeralPortRing_KeyCollapseRate measures the
// 0061 ring-collapse probability at K = collapseFixtureK; that measurement is only
// about THIS fixture if collapseFixtureK equals sourceIPs. Nothing in the Go build
// links them: internal/cluster cannot import a test fixture (and must not), both
// constants are unexported, and a const in a _test.go file is invisible even to its
// own package's non-test build. So the link is recovered by PARSING the other file's
// SOURCE with go/parser — which requires NO new exported symbol on either side, and
// which (unlike a grep) cannot be spoofed by prose: the parser sees declarations,
// not comments, so the other file's own doc comment "K=collapseFixtureK=16" is
// invisible here.
func TestSourceIPsLinkedToCollapseFixtureK(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, collapseFixtureKSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", collapseFixtureKSourcePath, err)
	}

	found := 0
	got := -1
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name != "collapseFixtureK" || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					t.Fatalf("%s: collapseFixtureK is not an integer literal (%T)",
						collapseFixtureKSourcePath, vs.Values[i])
				}
				v, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatalf("%s: collapseFixtureK = %q: %v", collapseFixtureKSourcePath, lit.Value, err)
				}
				found++
				got = v
			}
		}
	}

	if found != 1 {
		t.Fatalf("%s: found %d const declarations of collapseFixtureK, want exactly 1 "+
			"(the gate cannot resolve an ambiguous or missing declaration)",
			collapseFixtureKSourcePath, found)
	}
	if got != sourceIPs {
		t.Errorf("DESYNC: this fixture drives sourceIPs=%d distinct ring_hash keys, but "+
			"internal/cluster's collapse-rate test pins collapseFixtureK=%d (%s). The "+
			"MEASURED leg of TestRingHash_EphemeralPortRing_KeyCollapseRate is therefore "+
			"reporting the collapse probability of a DIFFERENT fixture than this one. "+
			"Change both, or neither.",
			sourceIPs, got, collapseFixtureKSourcePath)
	}
}
```

The `found != 1` arm is what closes the SPEC gate's fail-open hole: a rename, move or deletion produces `found == 0` and **fails LOUD**.

- [ ] **Step 2 — the FULL 2×2 CROSS-PRODUCT, not a single negative control**

`reference_probe_must_discriminate`. **[ALL FOUR RUN at this PLAN]**:

| `sourceIPs` | `collapseFixtureK` | Go gate | hardened shell gate |
|---|---|---|---|
| 16 | 16 | **PASS**, exit 0 | `LINKED: sourceIPs = collapseFixtureK = 16`, exit 0 |
| 4 | 16 | **FAIL**, exit 1 | `DESYNC: sourceIPs=4 collapseFixtureK=16`, exit 1 |
| 4 | 4 | **PASS**, exit 0 | `LINKED: … = 4`, exit 0 |
| 16 | 4 | **FAIL**, exit 1 | `DESYNC: sourceIPs=16 collapseFixtureK=4`, exit 1 |

⚠️ **The (4,4) arm is the one that matters** — it proves the gate READS the test-side value rather than hardcoding 16. A gate that only ever saw (16,16) and (4,16) is indistinguishable from `if sourceIPs != 16 { fail }`.

Verbatim (arm 2):
```
    linkage_test.go:73: DESYNC: this fixture drives sourceIPs=4 distinct ring_hash keys, but internal/cluster's collapse-rate test pins collapseFixtureK=16 (../../../../internal/cluster/ringhash_test.go). The MEASURED leg … is therefore reporting the collapse probability of a DIFFERENT fixture than this one. Change both, or neither.
--- FAIL: TestSourceIPsLinkedToCollapseFixtureK (0.00s)
```

- [ ] **Step 3 — add the EMPTY-CAPTURE arm the SPEC never ran**

Rename BOTH constants in a scratch copy and run the SPEC's literal gate. **[RUN at this PLAN]**: `SPEC-FORM EXIT=0, a='' b=''` — **GREEN on a tree where neither constant exists.** Then run the Go gate on the same tree: it must `t.Fatalf` with `found 0 const declarations`. Record both.

- [ ] **Step 4 — the hardened SHELL fallback (for the six-gate, since CI may not run the fixture package)**

```bash
a=$(grep -oP '^\s*sourceIPs\s*=\s*\K[0-9]+'        test/fixtures/0061-lb-ring-hash/driver/driver.go)
b=$(grep -oP '^\s*collapseFixtureK\s*=\s*\K[0-9]+' internal/cluster/ringhash_test.go)
if   [ -z "$a" ] || [ -z "$b" ]; then echo "LINKAGE RED: EMPTY capture (a='$a' b='$b') — renamed/moved/deleted"; exit 1
elif [ "$a" != "$b" ];           then echo "LINKAGE RED: DESYNC sourceIPs=$a collapseFixtureK=$b"; exit 1
else echo "LINKAGE PASS: K=$a"; fi
```

Two fixes over the SPEC form: the **`^\s*` anchor** (without it the measurement test's own doc comment is a second match and the gate fails CLOSED — §1.1(b); it also immunises against a future `maxSourceIPs = 8`), and the **empty-capture arm** (without it a rename reads green — §1.1(a)).

- [ ] **Step 5 — hygiene + commit**

```bash
[ "$(gofmt -l test/fixtures/0061-lb-ring-hash/driver/ | wc -l)" -eq 0 ] && echo clean
go vet ./test/fixtures/0061-lb-ring-hash/... && golangci-lint run ./test/fixtures/0061-lb-ring-hash/...
git -C <abs-worktree-path> commit -am "phase 76 T4: go/parser LINKAGE gate closing the single-edit case break beta cannot"
```

⚠️ **Land T4 atomically with, or after, T2** — at `sourceIPs=4` it correctly reports RED, which is a true verdict about a desynced tree but would leave the branch red mid-spine.

---

## Task 5 — `driver.go` + `expectations.yaml` prose sweep

**Files:** Modify `test/fixtures/0061-lb-ring-hash/driver/driver.go` (~11 comment sites), `test/fixtures/0061-lb-ring-hash/expectations.yaml` (9 sites)

**Interfaces:** none.

⚠️ **Sweep for the SHAPE of the old claim, not only its numerals.** A numeral-keyed sweep is structurally blind to a site falsified in the OPPOSITE direction — the phase-75 `0110/expectations.yaml` lesson.

- [ ] **Step 1 — re-derive the roster at this tip (do NOT trust the table below)**

```bash
cd test/fixtures/0061-lb-ring-hash
grep -n '\b64\b' driver/driver.go expectations.yaml
grep -n '\b4\b'  driver/driver.go expectations.yaml
grep -n '127\.0\.0\.' driver/driver.go expectations.yaml
grep -n 'DETERMINISTIC\|EXACT' driver/driver.go expectations.yaml
```

- [ ] **Step 2 — `driver.go` comment edits**

| line | current | → |
|---|---|---|
| `:11` | `source IPs 127.0.0.2..5 (via` | `127.0.0.2..17` |
| `:13` | `per source IP = 64 total` | `= 256 total` |
| `:23` | `single-key pin → all 64` | `all 256` |
| `:182` | `each of the 4 source IPs` | `16 source IPs` |
| `:183` | `net.Dialer.LocalAddr 127.0.0.2..5` | `127.0.0.2..17` |
| **`:270-271`** | **`DETERMINISTIC/EXACT — not a σ-band`** | **see Step 3** |
| `:272` | `all 64 on ONE backend` | `all 256` |
| `:308` | `upstream_cx_total==64` | `==256` |
| `:312` | `ref=64` | `ref=256` |
| `:364` | `rq-per-cx) → 64` | `→ 256` |

**NO EDIT:** `:20` (`D-S36-4` token) · `:78`/`:80` (done at T2) · `:196` (self-scaling) · `:405` (`ParseUint` bitSize) · `:7`/`:145`/`:151`/`:156`/`:176-178` (`127.0.0.1` listener/endpoint addrs).

- [ ] **Step 3 — the OPPOSITE-DIRECTION claim (`driver.go:270-271`, `expectations.yaml:21-22`)**

Current, verbatim:

```
// SPREAD (>= 2 distinct backends nonzero). DETERMINISTIC/EXACT — not a σ-band
// (reference_differential_band_sigma_margin governs RNG bands; affinity is not one).
```

⚠️ **The adjective governs the compound "affinity + SPREAD" while the parenthetical defends only affinity.** It is **TRUE of affinity and FALSE of spread** — spread is exactly the probabilistic leg this row exists to fix. These sites contain **none** of the stale numerals (`4`, `64`, `20/20`, "fixed ring"), so a numeral-keyed sweep **cannot reach them**.

Replace with a statement that splits the two legs and names the derived margin, e.g.:

```
// SPREAD (>= 2 distinct backends nonzero). AFFINITY is DETERMINISTIC/EXACT — not a
// σ-band. SPREAD is PROBABILISTIC: the ring is keyed on the backend's ephemeral-port
// address, so it is a fresh random 3-way partition per run and P(collapse) =
// 3^(1-sourceIPs) = 7.0e-8 at sourceIPs=16 (a 5.27σ-equivalent margin; ADR-0298).
// Measured by TestRingHash_EphemeralPortRing_KeyCollapseRate in internal/cluster.
```

Apply the same split to `expectations.yaml:21-22`.

⚠️ **`0062/driver/driver.go:299` and `0063/driver/driver.go:299` carry the SAME false adjective and are DELIBERATELY OUT OF SCOPE** — SPEC §3.1 records the widening it considered and refused; ADR-0298 ¶11 records the contradiction instead. ⚠️ **The SPEC says `:300`; it is `:299`** (§1.4 E5) — so a future session widening the scope has the right anchor.

- [ ] **Step 4 — `expectations.yaml`**

`:5` `4` → `16` · `:6` `127.0.0.2..5` → `..17`, `64 total` → `256 total` · `:21-22` per Step 3 · `:25` `4 keys` → `16 keys` · `:26` `== 64` → `== 256` · `:27` `sum == 64` → `256` · `:28` `all 64` → `all 256` · `:32` `upstream_cx_total == 64` → `== 256` · `:40` `ref 64` → `ref 256`. **NO EDIT:** `:21`'s `D-S36-4`, `:23`'s `≡ 0 mod 16`, `:43-45`.

- [ ] **Step 5 — verify no stale numeral survives, then commit**

```bash
grep -n '\b64\b\|127\.0\.0\.2\.\.5\|\b4 source\|4 keys' driver/driver.go expectations.yaml
```
Expected remaining: `driver.go:405` (`ParseUint`) and the `D-S36-4` tokens **only**. Anything else is a miss.

```bash
git -C <abs-worktree-path> commit -am "phase 76 T5: 0061 driver.go + expectations.yaml sweep, incl. the FALSE 'spread is DETERMINISTIC/EXACT' claim"
```

---

## Task 6 — `README.md`: the numerals, the refuted flake certification, and the two sites the SPEC missed

**Files:** Modify `test/fixtures/0061-lb-ring-hash/README.md` (162 lines)

**Interfaces:** none.

⚠️ **THE SPEC'S ROSTER IS WRONG HERE IN THREE PLACES** (§1.4 E1/E2/E3): `:58` should be **`:57`**, and **`:114`** and **`:135`** (two `64`s) are missing entirely. Re-derive before editing.

- [ ] **Step 1 — re-derive**

```bash
grep -n '\b64\b\|\b4\b\|127\.0\.0\.\|20/20\|count=20\|DETERMINISTIC\|EXACT\|fixed ring\|overwhelmingly' \
  test/fixtures/0061-lb-ring-hash/README.md
```

- [ ] **Step 2 — the numeral sites**

`:20`/`:21` `4 source IPs — 127.0.0.2, .3, .4, .5` → 16 source IPs, `127.0.0.2`…`127.0.0.17` (**use a RANGE form**; enumerating 16 inline bloats the line) · `:23` `127.0.0.2..5` → `..17` · `:25` `16 × 4 = 64` → `16 × 16 = 256` · `:40` `4 keys over 3 backends` → `16 keys` · `:45` · **`:57`** (NOT `:58`) `the 64 conns sum` → `256` · `:66` `:68` `:70` `:71` · `:96` `:102` stats-table cells · **`:114`** `rq-per-cx → 64` → `256` · `:131` break-table `64 → 99` · **`:135`** `(all 64 land on ONE backend, 64 % 16 == 0)` → `(all 256 land on ONE backend, 256 % 16 == 0)`.

**NO EDIT:** `:52`/`:74`/`:153` (`D-S36-4`) · `:6`/`:37`/`:43` (`127.0.0.1`, generic `127.0.0.x`) · `:76`'s *"necessary and overwhelmingly discriminating"* — that is about **affinity** and is correct. ⚠️ A blind `overwhelmingly` sweep hits it; **do not over-edit**.

- [ ] **Step 3 — `:54`, the σ-band claim** — same split as T5 Step 3.

- [ ] **Step 4 — `:80-82`, the scatter probability. The BOUND survives; the NUMERALS do not.**

⚠️ SPEC §7 says this claim *"SURVIVES — do not claim improvement"*, which invites leaving `:81` byte-untouched. **`:81` reads `multinomial(64, 1/3)` landing on `{0,16,32,48,64}`-only counts** — both stale.

The `< 1%` bound holds, but the row **WEAKENS this leg ~4×** and must say so plainly. Controller-computed exactly (multinomial(n, ⅓,⅓,⅓), all three counts ≡ 0 mod 16):

| n | P(scatter survives affinity) |
|---|---|
| 64 (before) | **0.0962%** |
| 256 (after) | **0.3814%** |

Rewrite `:80-82` to state `multinomial(256, 1/3)`, the step-16 support set over 0..256, the new **0.38%**, and that it is a deliberate trade: **the spread flake is OBSERVED (three occurrences); the scatter adversary is HYPOTHETICAL, and `:82` already concedes the invariant would not catch it.**

- [ ] **Step 5 — `:143-148`, THE REFUTED FLAKE CERTIFICATION. STRIKE IT; do not re-measure it.**

Current, verbatim:

```
### Flake check
`go test ./test/differential/ -run 'TestDifferential/0061' -count=20` → **20/20
PASS** (66 s; 20 fresh reference containers). The affinity leg is DETERMINISTIC
(fixed ring + fixed source-IP keys → never flakes); the spread leg (`>= 2`) is
overwhelmingly stable (4 source-IP keys over 3 backends). No assertion loosened.
```

**Three things here are false, not merely stale:**
1. **`fixed ring` (`:147`) is FACTUALLY FALSE** — the ring is keyed on `Endpoint.Addr()` *including* the OS-ephemeral port. The README certifies stability on the exact property that does not hold. This one phrase is the root cause.
2. **`overwhelmingly stable (4 source-IP keys)` (`:148`)** is the claim the observed flake refuted: `P = 3^(1−4) = 3.7%`.
3. **`20/20 PASS` (`:145`) has no power.** `(1−0.0355)^20 ≈ 0.48` — the check was **more likely than not to pass even if the assertion were exactly as broken as it in fact is.** 95% power would have needed `-count=81`. The 66 s wall-clock is also stale (4× workload).

⇒ **Replace the whole section with a pointer to the MEASUREMENT, not a bigger `-count=N`.** The replacement must state: the ring is re-randomized per run; `P = 3^(1−sourceIPs)`; at 16 that is 7.0e-8, **analytic/extrapolated, not measured** (§1.6); the rate is verified by `TestRingHash_EphemeralPortRing_KeyCollapseRate`; and — explicitly — **a pass-count is not a margin** (ADR-0298).

⚠️ **Do NOT verify this row by re-running the fixture N times.** That is the exact error being corrected.

- [ ] **Step 6 — final shape sweep + commit**

```bash
grep -n '20/20\|count=20\|fixed ring\|overwhelmingly stable\|DETERMINISTIC' test/fixtures/0061-lb-ring-hash/README.md
```
Every surviving hit must be justified in `PROGRESS.md` (`:76`'s affinity sense is the expected survivor).

```bash
git -C <abs-worktree-path> commit -am "phase 76 T6: 0061 README sweep — strike the statistically invalid 20/20 certification and the FALSE 'fixed ring' claim"
```

---

## Task 7 — `BEHAVIOR_CONTRACT.md:1326`, in place, no line-count change

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md` (line **1326 ONLY**)

**Interfaces:** none.

⚠️ **This task exists because the BRAINSTORM said it would not** — its default posture was *"no behavior changes ⇒ no contract edit."* **The contract documents the fixture's WORKLOAD as well as the proxies' behavior.**

- [ ] **Step 1 — confirm it is the ONLY such line**

```bash
grep -n 'source IPs .127\|16 conns each' docs/envoy-go/BEHAVIOR_CONTRACT.md
```
**[RUN at this PLAN]** ⇒ **1 hit, line 1326.** File total **5746** lines; the line is **1791 chars**.

⚠️ `grep -c '64'` over the file ⇒ **101 LINES** (162 occurrences) — but only `:1326` is about this fixture. The other `0061`-mentioning line, `:1338`, matches on **`xxHash64`**, a hash-function name. Bulk categories: `uint64|int64|float64` 37 · `base64` 13 · `xxHash64|Hash64` 9.

- [ ] **Step 2 — the edit**

The line has exactly **four `64`s and three `16`s**. The three `16`s — `16 conns each`, `% 16 == 0`, `all-16-or-0` — **ALL STAY** (`burstPerIP` unmoved). ⚠️ `256` contains no `16` substring, so a line-scoped `s/64/256/g` cannot collide with them.

```bash
sed -i '1326{s/`127\.0\.0\.2\.\.5`/`127.0.0.2..17`/; s/64/256/g}' docs/envoy-go/BEHAVIOR_CONTRACT.md
```

Changed span, OLD → NEW:

> OLD: ``the driver binds outgoing conns to source IPs `127.0.0.2..5` (16 conns each = 64 total) … conservation `sum == 64`. The reference (Docker-NAT'd to one source IP → all 64 on ONE backend) is asserted on `sum == 64` + …``
>
> NEW: ``the driver binds outgoing conns to source IPs `127.0.0.2..17` (16 conns each = 256 total) … conservation `sum == 256`. The reference (Docker-NAT'd to one source IP → all 256 on ONE backend) is asserted on `sum == 256` + …``

- [ ] **Step 3 — prove no line was added or removed**

**[VERIFIED IN SCRATCH at this PLAN]**:

```
ORIG lines 5746  →  NEW lines 5746          (unchanged)
diff: exactly "1326c1326"  — 1 line removed, 1 line added, NOTHING else
NEW 1326: '64'×0 · '256'×4 · '% 16 == 0' present · '16 conns each' present ·
          'all-16-or-0' present · '127.0.0.2..17' present
chars 1791 → 1796 (+5) ; file bytes +5
```

⇒ **every by-line citation into the section stays valid.**

- [ ] **Step 4 — the stat surface: +0, and do NOT re-derive the total**

`grep -n '1205'` ⇒ **3 hits** (`:831` graphite, `:847` OTLP, `:5004` the ledger tail). **All three STAY at 1205; none needs editing.** ⚠️ **1205 is DOCUMENTARY** — there is no mechanical counting command, and the chain is discontinuous in **TWO** recorded places (`1200 → 1201`, and Phase 46.1b closing at 1198 while 47.1 opens at 1200). **This row asserts the DELTA (zero, which IS checkable), never the total. Do not present 1205 as re-derived.**

The ledger heading is `### Stat surface` at **:4950**; the section runs to **:5006**; the tail entry is **:5004** (Phase 75). **Phase 76 adds NO ledger line** — a +0 row has nothing to record there.

- [ ] **Step 5 — commit**

```bash
git -C <abs-worktree-path> commit -am "phase 76 T7: BEHAVIOR_CONTRACT:1326 workload line 64 -> 256, IN PLACE, 5746 -> 5746 lines"
```

---

## Task 8 — the break roster: α, the TWO-EDIT β, and both γs

**Files:** temporary edits to `internal/cluster/ringhash.go` (α) and the two constants (β/γ), all reverted.

**Interfaces:** none. This task produces evidence, not code.

⚠️ **COMMIT FIRST.** `git restore` wipes uncommitted work (`reference_break_protocol_commit_first`). ⚠️ `-count=1` on every run — caching serves a stale PASS. ⚠️ **Revert with `git restore`, NEVER `git checkout <sha>`** (detaches HEAD).

- [ ] **Step 1 — BREAK α (LIVENESS), the one break that needs a PRODUCTION edit**

Re-derive the anchor first — it must be the `sort.Search` call, and this lineage has already mis-anchored it once:

```bash
sed -n '128,143p' internal/cluster/ringhash.go
```

Expected: `:129-132` `Pick`'s **doc comment** · `:133` the `func … Pick(` **declaration** · **`:140` `m := sort.Search(len(rh.ring), func(i int) bool { return rh.ring[i].hash >= hashKey })`** · `:142` `m = 0 // wrap`.

Edit **`:140`** → `m := 0`. ⚠️ **NOT `:129`** (a doc comment is not a code site — the router corrected all four documents for exactly this) and **NOT `:142`** (a legitimate wrap branch).

```bash
go test ./test/differential/ -count=1 -v -run 'TestDifferential/0061-lb-ring-hash'
```

**MUST fire, verbatim:** `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)`

⚠️ **CONFIRM WHICH LEG FIRED — this break is UNUSUALLY easy to mis-attribute.** Under a total collapse:
- **affinity SURVIVES** — 256 % 16 == 0, so the single nonzero count is still a multiple of `burstPerIP`;
- **conservation SURVIVES** — the sum is unchanged at 256;
- **length SURVIVES.**

⇒ **only the spread leg can fire.** An affinity- or conservation-shaped line means the break tested something else entirely (`reference_deliberate_break_wrong_assertion`).

⚠️ **Also record what this break does to `go test ./internal/cluster/ -count=1`** — if the unit package catches it too, α is not isolating the fixture's assertion.

Then `git restore internal/cluster/ringhash.go`, re-run isolated, confirm GREEN. **A green after an observed red means "ran and passed".**

- [ ] **Step 2 — BREAK β (THE ASYMMETRY) — TWO EDITS, and record why**

Set **both** `sourceIPs` and `collapseFixtureK` to 4.

```bash
go test ./internal/cluster/ -run 'TestRingHash_EphemeralPortRing_KeyCollapseRate' -count=1 -v
```

**[RUN at this PLAN]** — verbatim:

```
    ringhash_test.go:233: MEASURED leg K=4: collapse rate 0.03700 (74/2000) >= bar 0.001 (analytic 3^(1-K)=3.704e-02). K is the number of DISTINCT ring_hash keys the 0061 fixture drives, i.e. the sourceIPs constant in test/fixtures/0061-lb-ring-hash/driver/driver.go — …
--- FAIL: TestRingHash_EphemeralPortRing_KeyCollapseRate (0.64s)
```

A **35× margin** over the bar. ⚠️ **A two-edit β proves the UNIT TEST's constant is load-bearing. It does NOT, by itself, prove the FIXTURE's is. Do not describe β as proving more than it does.** Also record: on that same tree `go test ./test/fixtures/0061-lb-ring-hash/driver/` returns **`ok … 0.002s`** — **the fixture's own package is completely blind to the break.**

- [ ] **Step 3 — BREAK β-SINGLE-EDIT (the refutation; it MUST NOT fire)**

Revert **only** the fixture's `sourceIPs` to 4, leaving `collapseFixtureK` at 16.

**[RUN at this PLAN]** — `--- PASS`, **exit 0**. **DECLARED MUST-NOT-FIRE.** The two constants are mechanically decoupled: `collapseFixtureK` is an independent literal in a different package, and `sourceIPs` is unexported (`grep -rn '\bsourceIPs\b' --include='*.go' .` ⇒ **3 hits, all in `0061/driver/driver.go`**). **This green is a FINDING, not a pass** — it is exactly what T4's linkage gate exists to catch, and T4 must be shown RED on this same tree.

- [ ] **Step 4 — BREAK γ + γ-SHARP** — executed at T1 Steps 5-6. Re-confirm both at the IMPL tip and record.

- [ ] **Step 5 — restore, verify clean, record every outcome in `PROGRESS.md`**

```bash
git -C <abs-worktree-path> status --short          # must be EMPTY
git -C <abs-worktree-path> rev-parse --abbrev-ref HEAD   # must be the stage branch, NEVER master or "HEAD"
```

---

## Task 9 — gates, full suite, counts, ADR-0298, and row 76 → `done`

**Files:** Modify `docs/envoy-go/DECISIONS.md` (ADR-0298), `docs/envoy-go/ROADMAP.md` (row 76), `docs/envoy-go/STATE.md`, `next-prompt.txt`

⚠️ **PHASE 76 IS A +0-PRODUCTION-`.go` ROW, WHICH IS EXACTLY WHEN A BROKEN GATE GOES UNNOTICED.** Every production-envelope gate is trivially green here. **A green gate is evidence only if you have seen it go red.** §1.1 records **five** gates in this lineage broken across both directions — two from phase 75, three found at this PLAN, one of them in the phase-76 SPEC itself.

- [ ] **Step 1 — G0, the branch tripwire (run BEFORE anything else, and again before the commit)**

```bash
W=<abs-worktree-path>
pwd; git -C $W rev-parse --abbrev-ref HEAD; git -C $W rev-list --count master..HEAD
```
**NC:** if the branch prints `master`, STOP. This hazard has now fired in **six consecutive sessions**, including this one.

- [ ] **Step 2 — the six-gate, each with its observed negative control**

| # | gate | expected | negative control — **observed** |
|---|---|---|---|
| G1 | `go build ./...` | silent, 0 | `var _ int = "nope"` in hash.go ⇒ `cannot use "nope" … as int value`, **exit 1** |
| G2 | `go vet ./...` | silent, 0 | bad Printf verb ⇒ `format %d has arg … of wrong type string`, **exit 1** |
| G3 | `[ "$(gofmt -l . \| wc -l)" -eq 0 ]` | 0 files | mis-indent ⇒ prints the file. ⚠️ **`gofmt -l` itself EXITS 0 on an unformatted tree — gate on OUTPUT** (§1.1 d) |
| G4 | `golangci-lint run ./...` | silent, 0 | `unusedVar := 42` ⇒ `declared and not used (typecheck)`, **exit 1** |
| G5 | `go mod tidy -diff` + `git diff master -- go.mod go.sum` | both EMPTY | insert a fake `require` ⇒ count 67 → 68 |
| G6 | `go test ./test/differential/ -count=1` | **119/119 PASS, 0 FAIL, 0 SKIP** | break α (Step 5) |

Derive the tally rather than eyeballing it, and prove the fixture-directory set equals the subtest set:

```bash
go test ./test/differential/ -count=1 -v 2>&1 | tee /tmp/p76.diff.log | tail -40
grep -c '^    --- PASS: TestDifferential/' /tmp/p76.diff.log
comm -3 <(ls -d test/fixtures/[0-9]*/ | xargs -n1 basename | sort) \
        <(grep -oP '^    --- (PASS|FAIL): TestDifferential/\K[^ ]+' /tmp/p76.diff.log | sort)
```
`comm -3` must be **EMPTY**.

**Pre-fix baseline measured at master TODAY, same session: 119/119 PASS, 408.8 s, `0061` subtest 3.42 s, first attempt, no flake.** Quote the K=16 run as a **DELTA against that**, never as an absolute against a recorded figure — the absolutes do not transfer across sessions; the deltas do.

- [ ] **Step 3 — G7, `-race` over `./test/differential/`** — never run at K=16. Report wall-clock and any race verbatim.

- [ ] **Step 4 — G8/G9/G10, the envelope gates. AUDIT ONE PACKAGE PER INVOCATION.**

```bash
# G8 — +0 production .go files. Baseline MUST be master (98c27fc9).
git -C $W diff master --name-only | grep '\.go$' | grep -v '_test\.go$' | grep -v '^test/'
```
Expected **0 lines**. **NC from real history** (no perturbation needed): `git diff 9f5d667b c57b98b8 --name-only | …` ⇒ **2** (`internal/listener/manager.go`, `internal/stats/name.go`). ⚠️ **Baselining against the phase-75 tip returns 2 and reads RED on a clean tree.**

```bash
# G9 — +0 new exported symbols, ONE package per invocation
for p in ./internal/cluster ./test/differential/fixture ./test/fixtures/0061-lb-ring-hash/driver; do
  (cd $W_master && go doc -all "$p") > "/tmp/base.$(basename $p)"
  (cd $W        && go doc -all "$p") > "/tmp/head.$(basename $p)"
  diff "/tmp/base.$(basename $p)" "/tmp/head.$(basename $p)" || echo "RED: $p"
done
```
⚠️ **NEVER `go doc -all ./pkgA ./pkgB`** — §1.1(c): the `./`-prefixed second argument is silently discarded, exit 0, even when it names a directory that does not exist. **NC (observed):** appending `func ExportedProbeSymbol() int` ⇒ `> func ExportedProbeSymbol() int`, exit 1.

```bash
# G10 — +0 new imports. The phase-75 helper is blind to single-line imports (§1.1 e).
impblock() { awk -v n="$2" '
  /^import[ \t]*\($/ {b=1; next}
  b && /^\)/         {b=0; next}
  b && NF            {gsub(/^[ \t]+/,""); print n"\t"$0; next}
  /^import[ \t]+[^(]/{s=$0; sub(/^import[ \t]+/,"",s); print n"\t"s}
' "$1"; }
```
⚠️ **Basename-normalised (`-v n=`), not `FILENAME`** — the phase-75 form prints the path, so a `/tmp` baseline vs an in-tree HEAD differs on every line and exits 1 on a **+0-import** tree. **Both arms observed at this PLAN:** GREEN `BASE=13 HEAD=13, GATE PASS`; RED on an injected `"math/rand"` / `"math"`, exit 1.

⚠️ **The test-side import delta is NOT zero and that is fine:** `internal/cluster/ringhash_test.go` gains **`fmt`, `math`, `math/rand/v2`** and `linkage_test.go` adds `go/ast`, `go/parser`, `go/token`, `strconv`, `testing`. All stdlib, all test-side. **An import LINE is not a go.mod MODULE**, and there is no new sub-package, so `reference_new_subpackage_pulls_transitive_module` does not bite. ⚠️ SPEC §4 predicted `math/rand` + `strconv` — **refuted** (§1.4 E7).

- [ ] **Step 5 — G11/G12/G13, the count and linkage gates**

| gate | command | expected | NC observed |
|---|---|---|---|
| fixtures | `ls -d test/fixtures/[0-9]*/ \| wc -l` | **119** | `mkdir …/0118-fake` ⇒ 120 |
| fuzzers | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | **55** | append `func FuzzProbeOnly` ⇒ 56 |
| BackendKind | `grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go` | tail **38** at `:614`; **39 constants declared (0-38)** | append `= 39` ⇒ tail 39 |
| go.mod | `grep -cE '^\s+[a-z]' go.mod` | **67** (18 direct + 49 indirect) | fake require ⇒ 68 |
| stat surface | +0; `TestNoNewStat*` (`internal/statssink/registration_test.go:26,53,81,109,137`, 5 guards) | 5× PASS | — |
| **linkage** | T4's Go gate + hardened shell form | PASS | the full 2×2 **plus** the empty-capture arm |

⚠️ **BackendKind 38 is a TAIL VALUE, not a count.** The file declares **39** constants (`TCPEcho = 0`). **Do not "fix" 38 to 39.**

⚠️ **The +0-stat argument for this row is STRUCTURAL, not a `statssink` result:** zero production `.go` bytes change (G8 + G14), so no `NewCounter`/`NewGauge` call site can move. The ring gauges' own value pins live at `internal/cluster/manager_test.go:1343-1345` (`ring_hash_lb.size 1026 / min 342 / max 342`) — **a function of the HOST count, not the key count**, so K=16 leaves them untouched (SPEC §3.6 observed this positively).

- [ ] **Step 6 — G14, the sha256 byte-untouched roster**

⚠️ **DO NOT COPY THE PHASE-75 ROSTER.** `internal/cluster/**` was byte-gated at phase 75 and **this row EDITS `internal/cluster/ringhash_test.go`** — `comm -12` over the two rosters returns exactly that file. Inheriting the glob reproduces the phase-73 SEVERE-1 (`reference_plan_schedules_edits_to_a_byte_gated_file`).

Phase 76's roster is **production-only: every `.go` outside `test/` and outside `*_test.go`, plus `go.mod`/`go.sum` — 375 files.** Its intersection with the EDIT roster is **EMPTY** (verified at this PLAN).

**NC — TWO distinct legs, both observed:** MISMATCH (`// probe` appended to `ringhash.go`) and **MISSING** (`rm internal/cluster/hash.go`). A deletion otherwise reads as "no mismatch".

- [ ] **Step 7 — ADR-0298 PROPOSED → COMPLETE, IN PLACE, no renumber**

Current shape, verified at this PLAN: `DECISIONS.md:17394-17422` (the file's **last** line; **no `---` follows** — the last `^---$` is at `:17020`). `### Context` ⇒ **1** · `### Decision` ⇒ **0** · `### Consequences` ⇒ **0** · `*(§Decision` footer ⇒ **1**. Eleven §Context paragraphs at `:17400`-`:17420`.

Mirror **ADR-0297** (`:17324-17392`), the immediately-prior completed form:
1. Rewrite the `> **STATUS: PROPOSED …**` blockquote to `COMPLETE`, adding the *"§Decision + §Consequences APPENDED IN PLACE at the phase-76 IMPL"* clause.
2. **RETAIN the italic footer verbatim** — ADR-0297 keeps its `*(§Decision + §Consequences land at the phase-75 IMPL.)*` at `:17352` and appends **after** it.
3. Append `### Decision (landed at the phase-76 IMPL)` — a narrative opener, then a **numbered task list** with each task's commit sha, then **lettered rulings (a)…(d)**.
4. Append `### Consequences (landed at the phase-76 IMPL)` — **bulleted**.
5. **No `---` separator.** No renumber. Tail stays ADR-0298; next-free stays **ADR-0299**.

⚠️ **ADR-0298 carries NO whole-file grep count.** That species self-falsified in ADR-0296 ¶3, ADR-0297 ¶7 **and** ¶9, and at the phase-76 BRAINSTORM it escalated from a wrong *number* to a **flipped termination-sentinel check**. Every count must be line-scoped or stated with no number.

⚠️ **ADR-0298 claims NO family ordinal.** The Load-balancing family was declared **CLOSED at row 54** (`ROADMAP.md:116`, *"EIGHTH and FINAL Load-balancing-family row"*; `grep -c 'Load-balancing-family row'` ⇒ **8**, the chain 34/35/36/37/38/52/53/54 complete). A maintenance row does not extend a charter — that, not the BRAINSTORM's *"the family is already open"*, is the reason the heading paragraph stays unamended.

Verify after the edit:
```bash
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Context'       # 1
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'      # 1
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^### Consequences'  # 1
awk '/^## ADR-0298/,0' docs/envoy-go/DECISIONS.md | grep -c '^\*(§Decision'      # 1  (RETAINED)
grep -c '^## ADR-0299' docs/envoy-go/DECISIONS.md                                # 0
```

- [ ] **Step 8 — row 76 → `done`, and CORRECT ITS FOUR STALE CLAIMS**

`ROADMAP.md:138` is **BRAINSTORM-era** (the ROADMAP was byte-untouched at the SPEC) and carries claims the SPEC refuted:

| stale claim in the cell | correction |
|---|---|
| `sourceIPs` **4 → 10** | **4 → 16** — K=10 is 3.89σ against a recorded 4-5σ bar; `0062`/`0063` fixed the byte-identical defect at 16 (5.27σ) |
| `K=10 → 5.1e-5` | **K=16 → 7.0e-8** (analytic; §1.6) |
| β as a single-edit *"THE PROOF"* | **β is a TWO-EDIT break**, and it proves only the unit test's constant load-bearing |
| *"THE EXECUTABLE DELTA IS ONE INTEGER … 7 hits, EVERY ONE A COMMENT"* | **FALSE** — `driver_test.go` (15 sites) + `BEHAVIOR_CONTRACT:1326`; and `grep -c '\b64\b' driver.go` ⇒ **8**, one of them CODE |

Flip `in-progress` → `done`. **A SINGLE FLAT ROW** — no split legs (`reference_roadmap_split_phase_row_done`).

⚠️ **NEVER WRITE A SENTINEL'S OWN MATCHER STRING INTO A FILE THE SENTINEL GREPS.** This fired **LIVE, twice, in one commit** at the phase-76 BRAINSTORM: row 76's first draft quoted check (3)'s marker phrase for the gRPC family — *inside the sentence explaining why quoting it would be gaming the check* — and check (3) came back with **gRPC silently GONE**. `grep` cannot tell a mention from a use.

- [ ] **Step 9 — re-run all three sentinel checks AFTER the edits land**

⚠️ **A pre-edit run is clean and meaningless.** T9 is the only phase-76 task that touches `ROADMAP.md`, so it is the only one with real exposure.

Expected at IMPL-done: **(1) SILENT** (row 76 was the last non-`done` chartered row) · **(2) still 3** — ⚠️ **THIS ROW NARROWS NOTHING, EVER**: none of the three `candidates:` sentences names a load-balancing candidate; the only `ring`/`Load-balanc` substrings on those lines are `buffering`, `Load-bearing`, and a historical *"the Load-balancing family closed at phase 54"* · **(3) `NEVER OPENED: gRPC / Runtime / WASM`.**

⇒ **the sentinel does NOT fire; `stop` MUST NOT be created.**

**Check (1)'s blind spot — RE-DERIVE IT, NEVER COPY IT.** At this PLAN's tip, derived independently: **108 data rows (`:31`-`:138`) / 104 matched / FOUR misses** — `| 00 |` (em-dash in the "after" column), `| 04 |` (DOT in the slug `http-1.1`), `| 28.1a |`, `| 28.1b |` (LETTER suffix). All four `done` ⇒ no current impact. ⚠️ This figure was recorded **wrong in two consecutive lineages** before being re-derived correctly three times running.

- [ ] **Step 10 — counts at exit, re-run MECHANICALLY in the worktree**

fixtures **119** (`0061` EDITED, not added) · fuzzers **55** · stat surface **1205** (⚠️ documentary) · BackendKind tail **38** · go.mod modules **2** (lineage figure; the single `go.mod` requires **67**) · DECISIONS tail **ADR-0298 COMPLETE**, next-free **ADR-0299** · next-free reference port **10450** (⚠️ **not** `10118` — ports are not fixture-index aligned; this row needs none).

- [ ] **Step 11 — STATE.md + next-prompt.txt, then squash-push**

Roll STATE §Current **IN PLACE** (lifecycle **3 → DONE**; §Recent re-capped at FIVE **with its preamble updated** — the ADR-0288 rule). ⚠️ **Verify the singleton greps with `grep -n` and NEVER "fix" a count down** — the rule statement at `:7` is the FIRST hit and the live value the second. Executed at this PLAN: `lifecycle-state` ⇒ **5**, `next-skill` ⇒ **2**, `next-free ADR` ⇒ **3** (§1.8; these counts are file-scoped to `STATE.md`, so naming the tokens HERE is safe — naming them IN `STATE.md` is not, and doing so cost this stage a self-inflicted +1 on each). A close script taking `head -1` reads the RULE, not the value.

`next-prompt.txt` is **TRACKED despite `.gitignore`** — edit it in the stage worktree; locate its commits by **SUBJECT**, never by position.

Subagents commit **LOCALLY only**; the controller squashes and pushes at close.

---

## 4. Break map — SIXTEEN breaks; THIRTEEN already RAN at this PLAN

**Legend:** **[RUN]** = executed at this PLAN, outcome recorded verbatim. **[IMPL]** = to be executed at the phase-76 IMPL.

| # | Task | Edit | MUST fire | Status |
|---|---|---|---|---|
| **α** | T8.1 | `ringhash.go:140` `m := sort.Search(…)` → `m := 0` | `subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` — **and ONLY spread**; affinity and conservation both SURVIVE | **[IMPL]** — dispatched at this PLAN; see PROGRESS.md |
| **β** | T8.2 | **TWO EDITS** — both constants → 4 | `MEASURED leg K=4: collapse rate 0.03700 (74/2000) >= bar 0.001` | **[RUN — fired]** |
| **β-single** | T8.3 | fixture only → 4 | ⚠️ **DECLARED MUST-NOT-FIRE** | **[RUN — did not fire, exit 0]** |
| **γ** | T1.6 | restore both to 16 | measurement PASSES *after an observed red* | **[RUN — PASS 1.46s]** |
| **γ-sharp** | T1.5 | freeze the ephemeral ports | **CONTROL leg RED, MEASURED leg STAYS GREEN at 0/2000** | **[RUN — fired exactly so]** |
| **L1-L4** | T4.2 | the 2×2 linkage cross-product | (16,16)✅ (4,16)❌ (4,4)✅ (16,4)❌ | **[ALL FOUR RUN]** |
| **L5** | T4.3 | rename both constants | Go gate `found 0 const declarations`; **SPEC's shell form exits 0** | **[RUN — SPEC form FAILED OPEN]** |
| **V1-V6** | T3.6 | one per unit test | each fires **its own** leg, other five green | **[ALL SIX RUN]** |

**Declared MUST-NOT-FIRE: β-single.** Its green is the finding — it is what T4 exists to close.
**Declared MUST-STAY-GREEN: γ-sharp's MEASURED leg.** If it went red the test would be measuring something other than what it claims.
**Beyond the SPEC's α/β/γ this adds THIRTEEN** — every one exists because a naive version of its parent was vacuous or a gate was broken.

---

## 5. Self-review against the SPEC

| SPEC section | covered by | note |
|---|---|---|
| §3.1 scope (5 files IN) | T2, T3, T5, T6, T7 | + `linkage_test.go` (T4), a 6th file the SPEC's §3.1 list does not carry |
| §3.2 `sourceIPs = 16` | T2 | |
| §3.3 the measurement | T1 | parameters as pinned; **control count is 74/2000, not 71** |
| §3.3.1 β + the linkage gate | T4, T8.2-8.3 | ⚠️ **the SPEC's gate is replaced, not adopted** — it fails open AND closed |
| §3.4 the rescale | T3 | ⚠️ **the ×4 scatter trap the SPEC's tuples avoid without saying why** |
| §3.5 loopback | discharged at this PLAN | **16/16, `.2`-`.17`** |
| §3.6 CXTOTAL / RUNTIME | T9.2 | self-updating via `totalConns`; runtime quoted as a same-session delta |
| §3.7 sweep | §6 deferred | report-only, as scoped |
| §6 BEHAVIOR_CONTRACT | T7 | |
| §8 sentinel | T9.9 | **narrows nothing; check (2) stays 3** |
| §10 edit roster | T5, T6 | ⚠️ **four errors corrected** (§1.4) |
| §11 ADR-0298 | T9.7 | |
| §13 NOT-executed list | §1.5 | `.12`-`.17` discharged; α / full-suite-at-16 / `-race` dispatched |

---

## 6. Deferred — named so a future sweep finds them

`0059-lb-least-request`'s **empirical-only** margins (`driver.go:72-73`, *"observed over ≥20 runs"* — ⚠️ **the same evidentiary shape as `0061`'s refuted 20/20**; it has not flaked, so it is not owed, but it is the next candidate if one appears) · `0013-http-local-ratelimit`'s **10 ms** band, the highest timing-flake risk in the tree · **`0062/driver/driver.go:299` and `0063/driver/driver.go:299`** — the same false *"DETERMINISTIC/EXACT — not a σ-band"* adjective, ⚠️ **`:299`, not the SPEC's `:300`** · a **mechanical COUNT** of the stat surface to replace the documentary 1205 (a +0 row is a cheap place, though not this one) · `README.md:80`'s scatter figure (**survives as a bound; do NOT claim improvement** — it weakens 0.096% → 0.381%).

**The non-maintenance rivals — re-costed at the BRAINSTORM, NOT re-costed here. `reference_deferred_candidate_cost_restale`: re-derive before adopting.** The **Runtime family opening ~10-14** — the only evaluated candidate that genuinely clears a check-(3) blocker, with six landed ADR-anchored `runtime_key` parse-reject arms forward-pointing at it (ADR-0187 `DECISIONS.md:11280`, ADR-0195 `:12413`) · `fault.abort.grpc_status` ~10-13 (blocker retired by execution; ⚠️ it does **NOT** open the gRPC family) · `ssl.connection_error` ~10-13 (incomplete predicate; needs `syscall.ECONNRESET` ⇒ **+1 production import**) · `upstream_cluster` ~7-9 but ~85-100 lines / ~18 files, central premise **UNVERIFIED** · `Listener.stat_prefix` ~7-10.

⚠️ **AGREEMENT ACROSS DOCUMENTS IS NOT EVIDENCE.** `fault.abort.grpc_status` rode three stages and two documents on one unexecuted assertion, refuted by a single `grep -c`. **A claim that has survived several stages is MORE suspect than one written yesterday, not less.**

---

## 7. Known live hazards — never reflex-classify any of these as a regression

The PRE-EXISTING `internal/cluster` `-race` flake (`TestOutlierDetector_ConcurrentEjectExactlyOnce` — ⚠️ it did **NOT** surface in **five** green `-race` runs across this PLAN) · the full-suite startup flake (`subject ready: EOF`; at phase 75 also `bind: address already in use` on an UNRELATED fixture, failing **BEFORE any assertion**) · `reference_sds_init_fetch_timeout_dial_budget_flake` · two still-**UNINDEXED** load flakes (`internal/httpclient TestOptions_ZeroValue_NoOpDefaults`, `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`).

⚠️ **`0061-lb-ring-hash` IS THIS ROW'S OWN SUBJECT.** Before the fix a spread failure is EXPECTED at ~3.6%/run. **AFTER the fix it is a FINDING**, not a flake — investigate, do not absorb.

⚠️ **A stage brief's flake list is not the index.** **NEITHER** flake that fired at phase 75 was on that PLAN's roster of six. Isolate-re-run, then state the classification **and its evidence**.

⚠️ **Unmeasured until T9:** the differential-suite effect of a **4× larger workload** (256 connections, 16 bound source IPs per side) on runtime and on the dial budget.

---

## 8. Operative memories

`reference_0061_ring_hash_spread_flake` · `reference_differential_band_sigma_margin` · `reference_grep_for_sibling_derived_constant` · `reference_fixture_workload_constant_desync` · `reference_ordered_assertion_legs_vacuous_on_constant_change` · `reference_change_set_measure_not_build_measure` · `reference_differential_break_protocol_count1` · `reference_break_protocol_commit_first` · `reference_deliberate_break_wrong_assertion` · `reference_liveness_break_needs_failing_baseline` · `reference_positive_arm_cannot_catch_overfiring` · `reference_fatalf_makes_assertions_unreachable` · `reference_gate_command_negative_control` · `reference_sentinel_matcher_string_self_clears` · `reference_probe_must_discriminate` · `reference_code_comment_not_evidence` · `reference_quoting_is_not_executing` · `feedback_brief_citations_not_evidence` · `reference_a_drift_correction_is_itself_a_claim` · `reference_document_hygiene_claim_not_evidence` · `reference_plan_schedules_edits_to_a_byte_gated_file` · `reference_spec_drafted_identifier_collision_check` · `reference_iocopy_self_splice_echo_backend` · `reference_next_prompt_tracked_despite_gitignore` · `reference_roadmap_split_phase_row_done` · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `feedback_subagent_worktree_detach` · `feedback_subagent_worktree_path_targeting` · `reference_parallel_subagents_private_scratch` · `feedback_pertask_gofmt_lint` · **`reference_bash_cwd_reset_commits_to_main`**.
