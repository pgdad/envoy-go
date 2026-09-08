# Phase 96 — `listener-default-chain-tlsmode` — PLAN

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal.** Widen one production predicate so that a listener whose only TLS is its `default_filter_chain`
registers its five `ssl.*` counters — closing a remotely-triggerable process SIGSEGV and a QUIC
stat-registration divergence with the same line — and make the repair falsifiable by unit pins on both
crashing shapes and by a new cross-side differential fixture.

**Architecture.** `listenerRuntime.tlsMode` is currently written as `anyTLS`, an accumulator populated
ONLY inside the `filter_chains[]` loop. `registerListenerMetrics` gates the five counter registrations on
`rt.tlsMode`; `serveConnection` gates the five `.Inc()` calls on `selected.tlsCfg != nil`. Those are
DIFFERENT predicates, and a TLS `default_filter_chain` separates them: registration is skipped, the Inc
guard still fires, and `(*stats.Counter).Inc` dereferences nil. The repair makes the registration gate
consider the default slot, which is what the pinned reference does — measured, not assumed.

**Tech Stack.** Go; `internal/listener`, `internal/stats`; the differential harness under
`test/differential` + `test/fixtures`; the pinned reference `envoyproxy/envoy:contrib-v1.37.2`.

**Spec.** `docs/envoy-go/phases/96-listener-default-chain-tlsmode/SPEC.md` (668 lines). Its §14 is the
seven-item list this PLAN discharges; §11 is the pinned edit map. The PLAN argues FROM the SPEC and the
SPEC travels with it — executors read both.

## Global Constraints

- **Reference pin:** `envoyproxy/envoy:contrib-v1.37.2`, digest `7edd5b0fd763…`, verified against
  `docs/envoy-go/ENVOY_TARGET.md:3-4` before any arm is trusted. ⚠️ `ENVOY_TARGET.md` is under
  `docs/envoy-go/`, NOT the repo root.
- **envoy-go boots with `-c`, NOT `--config-path`.** Its validate flag is `-mode validate`; the
  reference's is `--mode validate`.
- ⚠️ **An omitted `clusters:` key BOOT-REJECTS envoy-go.** Every probe and fixture config carries a
  placeholder STATIC cluster. `BackendCount()` must be ≥ 1 — the runner `t.Fatalf`s on 0.
- ⚠️ **The reference container cannot read `filename:` cert paths** — deliver PEMs as `inline_string:`,
  indented ONE level deeper than the `inline_string:` key, and keep PEM substitutions OUT of YAML
  comments (`text/template` expands actions inside `#` comments too).
- ⚠️ **`-count=1` is not optional** on any differential run; the suite's failure mode is a SILENT PASS.
- ⚠️ **`gofmt -l` never exits non-zero — gate on OUTPUT.** `golangci-lint`'s misspell runs in locale US:
  sweep British spellings out of `.go` comments before the gate; Markdown prose may use them freely.
- ⚠️ **`go test` without `-v` prints zero `=== RUN`** — `RUN=0` beside `RC=0` is a vacuous green. A
  `-run` selector matching nothing prints `[no tests to run]` and EXITS 0; a selector naming a package
  that does not exist prints `FAIL … [setup failed]` and EXITS 1. Confirm every selector resolves.
- ⚠️ **`grep -c` counts LINES, not occurrences, and on zero matches prints `0` AND exits 1** — capture
  with `v=$(cmd || true)`, never `$(cmd || echo 0)`.
- ⚠️ **There is no `recover()` in non-test `internal/listener` or `internal/tls`** — a nil-counter `Inc`
  aborts the PROCESS. Gate every panic check on the anchored form `^panic:|DATA RACE|SIGSEGV`, and prove
  that gate live before believing a zero.
- **Port band for ad-hoc probes: `15000-19000` minus `18080-19000`** (a sibling `curl-world` session
  holds that range). The differential reserves `20000..31007` and `11000..14999`;
  `net.ipv4.ip_local_port_range` starts at 32768. Check with `ss -tan` AND `ss -uan` (ALL states), never
  `ss -ltn`. ⚠️ **For a unit table, use port 0.** ⚠️ **`net.Pipe` DEADLOCKS a client-cert handshake** —
  use a loopback TCP pair.
- **Every task ends with a commit.** Subagents commit LOCALLY on their own stage branch with EXPLICIT
  PATHSPECS; the controller merges and squashes. ⚠️ **Subagents do not push.** ⚠️ Use
  `git -C <abs-worktree-path>` for every git command — the Bash tool's cwd silently resets to the repo
  root, and commits then land on `master`.

---

## 0. What this PLAN refuted, by execution — FIFTEEN claims

Method note 2: every stage's job is to refute its predecessor by execution. The phase-96 SPEC refuted
TEN claims, two of them in the router itself and three in governing documents, and told this stage
(§14 item 7) to do the same to it. Every figure below was produced by running its command at this
PLAN's own tip, `da6ea191`.

### ⚠️ 0.1 — `SPEC.md` §10 SHIPS **EIGHT FALSE FIGURES AT ITS OWN PUBLISHING COMMIT**

§10 is headed *"Counts, re-derived at THIS stage's own tip"* and says every figure *"was produced by
running its command at `f6b50462`."* **`f6b50462` is not that stage's tip.** The SPEC ships at
`da6ea191`, and `da6ea191`'s own diff adds `ADR-0318` §Context to `DECISIONS.md` (**+30**) and an
archive row to `STATE_HISTORY.md` (**+2**). Measured on both sides with `git show <sha>:<path>`:

| figure | §10 (`f6b50462`) | at the publishing commit `da6ea191` |
|---|---|---|
| `DECISIONS.md` lines | 18942 | **18972** |
| `grep -c '^## ADR-'` | 316 | **317** |
| `grep -c '^## '` | 324 | **325** |
| tail ADR | ADR-0317 | **ADR-0318** |
| next-free ADR | ADR-0318 | **ADR-0319** |
| `STATE_HISTORY.md` lines | 556 | **558** |
| archive guard, parenthetical | 65 | **66** |
| archive guard, loose | 228 | **229** |

**Eight of §10's thirty-two figures are wrong the moment the SPEC lands** — and they are wrong in one
direction, for one reason: a stage that measures itself BEFORE its own edits publishes a snapshot of a
tree that no longer exists. This is [[reference_row_can_ship_false_figure_at_birth]] and the
self-incrementing-control rule firing together. ⚠️ **A PLAN that copied §10 forward would inherit all
eight.** The remaining twenty-four are CONFIRMED exact at `da6ea191` (§7), including the two the SPEC
itself flags — `internal/tls/config.go` **666**, not the 642 row 95 asserts twice.

⚠️ **The archive-guard TRIPLE reconciles at BOTH tips** (163 + 65 = 228; 163 + 66 = 229), so the
invariant is sound and only the operands moved. **Re-derive the operands; keep the invariant.**

### ⚠️ 0.2 — `SPEC.md` §11's PLAN SCOPE LINE IS **UNDER-ENUMERATED**: A PLAN LANDS **FOUR** FILES, NOT `PLAN.md` "ONLY"

§11 states *"The PLAN lands: `PLAN.md` only."* Measured with `git show --numstat` across the four
modern PLAN commits — located by the anchored `^phase NN (` subject form:

| precedent | files touched (add/del) |
|---|---|
| `e69f1c58` (92 PLAN) | `STATE.md` 9/9 · `STATE_HISTORY.md` **2/0** · `phases/92-…/PLAN.md` 1482/0 · `next-prompt.txt` 88/96 |
| `90010c4c` (93 PLAN) | `STATE.md` 9/8 · `STATE_HISTORY.md` **2/0** · `phases/93-…/PLAN.md` 1031/0 · `next-prompt.txt` 76/65 |
| `db539e7d` (94 PLAN) | `STATE.md` 9/9 · `STATE_HISTORY.md` **2/0** · `phases/94-…/PLAN.md` 1662/0 · `next-prompt.txt` 93/106 |
| `f647dd72` (95 PLAN) | `STATE.md` 10/10 · `STATE_HISTORY.md` **2/0** · `phases/95-…/PLAN.md` 865/0 · `next-prompt.txt` 88/78 |

**FOUR files, identical in all four**, with `STATE_HISTORY.md` at exactly **+2 / -0** every time — the
parenthetical-append shape, blank line PLUS entry line.

⚠️ **§11's four NEGATIVES are CORRECT and are what the line exists to enforce**: no `.go`, no
`ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no `DECISIONS.md` — **zero of the four (and zero of all seven
PLAN-stage commits examined) touches any of them.** The refutation is of the word *only*, not of the
prohibition. ⇒ `ADR-0318` STAYS `PROPOSED`, the house guard STAYS **ARMED**, tail STAYS `ADR-0318`,
next-free STAYS `ADR-0319`, and §11's edit map STAYS PINNED for the IMPL.

### ⚠️ 0.3 — THE PROJECT'S RECORDED FOUR-PRECEDENT PLAN SET IS **{94, 93, 75, 77}**, AND TWO OF ITS MEMBERS PREDATE THE ARCHIVE

`STATE.md` and `next-prompt.txt` both cite `db539e7d` `90010c4c` `bae5e24d` `acedfd2b` as *"FOUR
precedent PLAN commits."* All four ARE genuine PLAN commits — but they are phases **94, 93, 75 and 77**,
not the recent four. `bae5e24d` (75) and `acedfd2b` (77) **predate ADR-0288**, so:

- **neither touches `STATE_HISTORY.md`** — the archive did not exist yet; and
- **both CREATE `PROGRESS.md` at the PLAN** (75: +115, 77: +97), which the modern posture does not.

Inheriting that set would produce the wrong verdict on exactly the two CONDITIONAL files. The set
measured above (92/93/94/95) is the operative one. **`PROGRESS.md` is assigned to the IMPL by §11 row
15, consistent with 92-95; this PLAN creates none, and that is a STANDING DEPARTURE, named rather than
claimed.**

### ⚠️ 0.4 — A **SLUGLESS PLAN-STAGE CORRECTION COMMIT EXISTS**, AND THE ANCHORED LOCATE FORM IS BLIND TO IT

`next-prompt.txt` warns that a slugless CORRECTION is invisible to `--grep '^phase NN (slug)'` and tells
every stage to run BOTH forms. **This PLAN found the live witness, and it is in the PLAN stage
specifically:** `963d472d` — *"phase 92 PLAN CORRECTION: the published `PLAN.md` line count was STALE
INSIDE ITS OWN ROW"* — has no `(slug)` in its subject and does not appear in
`git log --grep '^phase 92 ('`. It touches `STATE.md` 2/2 and `next-prompt.txt` 2/2, so it does not move
the §0.2 verdict; it moves the *method*. Both forms were run for phase 96 at this tip and **reconcile
exactly** (§2): three commits, identical sets, no slugless member yet.

⚠️ **AND NOTE WHAT IT WAS CORRECTING** — a `PLAN.md` line count that was stale inside the very row that
published it. This PLAN re-derives its own line count in the same commit as the edit that moves it.

### 0.5 — THE SPLIT GATE HAS A **MEASURED STRUCTURAL PRECEDENT THE SPEC NEVER NAMES**, AND IT IS BIGGER THAN THIS ROW

`SPEC.md` §14 item 2 calls the split gate *"a real question rather than a formality"* because §6 charters
a fixture — but it offers no measurement. There is one, and it is the closest possible analogue: the
**phase-94 IMPL, `0a985a35`**, which created fixture `0120` from nothing, added `manager_test.go` arms
for the same counter family, and landed a production predicate in the same function of the same file.

`git show --numstat 0a985a35`, additions only, excluding `.md` and `next-prompt.txt`:

```
internal/listener/manager.go              +73    test/fixtures/0120/driver/driver.go   +618
internal/listener/manager_test.go        +419    test/fixtures/0120/envoy.yaml         +103
internal/listener/quic_test.go            +14    test/fixtures/0120/envoy-go.yaml       +89
internal/stats/name.go                     +8    test/fixtures/0120/expectations.yaml  +242
internal/stats/name_test.go                +9    test/fixtures/0120/pki/*.pem           +45
internal/stats/helptext_test.go            +1    test/differential/runner_test.go        +1
                                                 0110 + 0111 expectations.yaml          +13
```

**`.go` only: +1143. Code + YAML + PEM: +1635.** Phase 94 was **NOT split**, and phase 96's production
edit is **one line** where phase 94's was seventy-three. §1.3 uses this as the anchor rather than an
unanchored guess.

### 🔴 0.6 — THE OCCURRENCE SET IS **TEN**, NOT NINE: `ROADMAP.md:136` CARRIES THE SAME FALSE COROLLARY

`SPEC.md` §0.3 enumerates **nine** live carriers and excludes historical copies with the rule
*"historical copies **under `docs/envoy-go/phases/74-*, 92-*, 95-*`** are records, not live contract."*
**That exclusion does not reach `ROADMAP.md`**, which is not under that path and is a live governing
document. `ROADMAP.md:136` — row 74's notes cell — reads, verbatim:

> The landed gate is `rt.tlsMode` ALONE, with no kind check; `startQUIC` hard-errors without a TLS
> config, so **every QUIC listener that boots has `tlsMode == true`**. See ADR-0296 §Context…

That is site 2's claim word for word, and §0.4 of the SPEC refutes it by execution. **All nine of the
SPEC's cites are EXACT at this tip — zero drift on all nine** — so this is an addition, not a
correction. ⚠️ **Fixing nine of ten would be the phase-95 F-C failure at a tenth site**, and the
marginal cost is near zero: `SPEC.md` §11 row 14 already has the IMPL editing `ROADMAP.md`.

**DECISION: the tenth site RIDES.** Task 16 corrects it in the same edit that flips row 96.

⚠️ **AND THE SWEEP PHRASE VARIES, WHICH IS WHY A ONE-PHRASE GREP IS NOT AN AUDIT.** A tree-wide
case-insensitive search for the exact string *"every QUIC listener that boots has"* finds only
`DECISIONS.md`, `ROADMAP.md` and `manager.go` — sites 3 and 9 spell it *"every booting QUIC listener
has `tlsMode == true`"* and *"`startQUIC` hard-errors without a TLS config so `rt.tlsMode` is
necessarily true."* **Sweep the CLAIM, not one of its spellings.**
⚠️ **AND CASE MATTERS**: site 4's head at `manager_test.go:2327` spells it `EQUIVALENT`, invisible to a
lowercase-only grep. A case-sensitive auditor finds two of the three `equivalent` sites and concludes
the head of the class does not exist.

### ⚠️ 0.7 — `BEHAVIOR_CONTRACT.md:1973` IS STALE ON **TWO** AXES, AND §11 ROW 10 NAMES ONLY ONE

The SPEC lists `:1973` as a carrier of the false invariant. It is — and it also says **"all four
counters"**. There have been **FIVE** since phase 94 (`ssl.connection_error`), and **`:1967`, six lines
above it in the same subsection, already says "All five are registered."** The file contradicts itself.

⚠️ **A single-axis edit re-mints a half-correction.** Task 14 fixes both axes in one edit and says so.

### ⚠️ 0.8 — `TestListenerMetrics_GateMatchesInc` **HAS NO TABLE AND NO SHARED ASSERTION BODY**

`SPEC.md` §5.1 says *"ADD arm (d)"*, *"ADD arm (e)"*, *"ADD arm (f)"* and requires *"`t.Helper()` on any
shared assertion body"* — a framing that presumes a table-driven test. Read at this tip, `:2341-2493` is
**three literal `t.Run(name, func(t *testing.T){…})` blocks with fully inlined, duplicated bodies**.
`grep` for `[]struct{` and `for _, tc := range` inside that range: **zero**. `grep` for `t.Helper()`
inside that range: **zero**.

⇒ §5.1's `t.Helper()` clause is **an instruction to CREATE a shared body that does not exist**, and it
is **VACUOUS** if (d)/(e)/(f) are written in the landed inline style. Task 5 and Task 4 resolve this
explicitly: the arms are inlined to match the file, and the per-property `t.Errorf` discipline — which
IS landed practice here (**18** `t.Error*` for properties against **10** `t.Fatal*` for setup only,
counted in that range) — is what gives each failure its distinct call-site line.

### 🔴 0.9 — ARM (f) MUST CALL `mgr.Start(ctx)`, OR IT IS **RED REGARDLESS OF THE FIX**

`registerListenerMetrics` is defined at `manager.go:394` and has exactly **two** call sites:
`manager.go:1179` (inside `Manager.Start`) and **`quic.go:45` (inside `startQUIC`)**. Both run at
**Start** time, never at `NewManager` time.

`SPEC.md` §5.1 arm (f) says to build the QUIC shape *"with the existing `mkQUICListenerDefaultChain` +
`mkQUICDownstreamTS`"*, and the landed test that uses them —
`TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos` at `manager_test.go:1062` —
**calls only `NewManager` and never `Start`.** An arm (f) copied from that template reads all five
pointers as nil **whether or not the predicate is widened**: a guaranteed-red arm that proves nothing,
and whose NC (roster row 3) would "fire" for the wrong reason.

⇒ **Arm (f) MUST call `mgr.Start(ctx)` and `defer mgr.Stop()`.** Arms (b) and (c) already do; arm (a)
does not, because it asserts a build-time reject.

### 🔴 0.10 — `manager.go:766` PUTS THE DEFAULT CHAIN INTO `chainByName`, WHICH BREAKS THE ARM (c) TEMPLATE

```go
766:		chainByName[defaultSpec.Name] = defaultChain
```

Arm (c)'s body at `:2463-2467` ranges `rt.chainByName` and `t.Errorf`s on any entry with
`tlsCfg != nil`. **An arm (d)/(e) copied from arm (c) would fire on its own TLS default chain.** And arm
(c)'s non-vacuity guard `if len(rt.chainByName) == 0 { t.Fatal(…) }` **passes for the wrong reason on
shape A**, where `len(chainSpecs) == 0` but `len(chainByName) == 1`.

⚠️ **A SECOND DEAD ASSERTION THE SPEC DID NOT NAME.** Arm (b) at `:2401-2403` is the mirror of the one
§0.5 found:

```go
if rt.defaultChain != nil && rt.defaultChain.tlsCfg == nil {
    t.Error("TLS listener: defaultChain has tlsCfg == nil")
}
```

It is equally dead today (arm (b) builds no default chain), and if it ever went live it would be a
**FALSE POSITIVE** — ADR-0080 §Decision 3 explicitly permits a plaintext default chain beside a TLS
`filter_chains[]` entry, and `SPEC.md` §2 records that §Consequences (c) illustrates exactly that
arrangement as legal. **`SPEC.md` §11 row 3 scopes the inversion to `:2468-2470` only.** Task 5 handles
both: `:2468-2470` is inverted, and `:2401-2403` is **DELETED**, because there is no correct assertion
to replace it with — the condition it forbids is authorised.

### ⚠️ 0.11 — SPEC §0.5's CODE-BLOCK QUOTE IS MISLEADING, AND `// THE LOAD-BEARING HALF.` MUST NOT MOVE

§0.5 renders the site as the `if` block followed immediately by `// THE LOAD-BEARING HALF.`, which reads
as though that comment annotates the assertion being inverted. It does not. `:2471` is the **header for
the five pointer-nil assertions at `:2472-2491`**, and the identical comment appears in arm (b) at
`:2404` for the same purpose. ⚠️ **Do not delete or relocate `:2471` when inverting `:2468-2470`.**

### ⚠️ 0.12 — `counterValue` vs `pollCounter` IS A **DISCRIMINATION REQUIREMENT**, NOT A STYLE CHOICE

`SPEC.md` §5.2 step 3 says *"Assert the other three are `0` explicitly."* Two landed helpers can do
that and only one of them means anything:

- **`pollCounter` (`quic_test.go:34`) returns a silent `0` for BOTH "registered and reading zero" AND
  "never registered at all."** Using it for the three non-movers makes the whole assertion **pass
  vacuously at the un-fixed tip**, where nothing is registered.
- **`counterValue` (`quic_test.go:66`) `t.Errorf`s on an ABSENT name**, which is exactly the failure the
  row exists to catch.

⚠️ **`assertSSLCrossProduct` (`manager_test.go:4755`) already gets this right** — it polls the movers and
`counterValue`s the non-movers, against the landed `sslLeafRoster` (`:4725`). `manager_test.go:4990`
already calls it for the one-way TLS arm. **§5.2 should call
`assertSSLCrossProduct(t, reg, addr, "handshake", "no_certificate")` rather than hand-roll a map** — and
§3.1's measurement says those are exactly the two movers.

### ⚠️ 0.13 — THE BYTE-UNTOUCHED ROSTER AND THE EDIT ROSTER **INTERSECT AT `manager.go`, AND BOTH OBVIOUS GUARDS FAIL AGAINST CORRECT CODE**

`SPEC.md` §14 item 6 requires the D4 comment block be proven on the byte-untouched roster and that roster
set-differenced against §11's edit roster. Done:

| roster | members |
|---|---|
| §11 EDIT roster, deduplicated | `BEHAVIOR_CONTRACT.md` · `DECISIONS.md` · `ROADMAP.md` · `phases/96-…/PROGRESS.md` · **`internal/listener/manager.go`** (rows 1, 2) · `internal/listener/manager_test.go` (rows 3-6) · `internal/listener/quic_test.go` (row 7) · `test/differential/runner_test.go` (row 13) · `test/fixtures/0121-…/**` (row 12) |
| §11 BYTE-UNTOUCHED set | `internal/listener/quic.go` (1 file) · `internal/tls/**` (**8** files) · `internal/stats/**` (**23** files) · **the D4 block at `manager.go:746-747`** |

**INTERSECTION = `internal/listener/manager.go`, and it alone.** Consequences, both measured:

1. ⚠️ **A whole-file `sha256sum internal/listener/manager.go` CANNOT be the guard** — edit-roster rows 1
   and 2 modify that file by construction, so the digest is *guaranteed* to change. Asserting it would
   fail against correct code ([[reference_pin_can_fail_against_correct_code]]).
2. ⚠️ **A LINE-SCOPED digest is ALSO unsafe.** Row 2 rewrites `manager.go:384-393`, which is **ABOVE**
   line 746, so any net line delta there shifts the D4 block and
   `sed -n '746,747p' … | sha256sum` reddens for the wrong reason
   ([[reference_line_shift_after_insert_is_banded]]).

**THE GUARD THAT WORKS — literal-anchored, line-number-free, in TWO parts:**

```sh
# (i) uniqueness — must print exactly 1, or (ii) digests a concatenation
grep -c -F 'Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture' internal/listener/manager.go
# (ii) content digest — invariant under any line shift rows 1-2 cause
grep -A1 -F 'Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture' internal/listener/manager.go | sha256sum
```

Measured at `da6ea191`: uniqueness **1**; digest
**`9154e453c99515d96a5d4bd9aea17fcd9b70c39682bf4418f40e0136a8ec0817`**. The block, byte-exact under
`cat -A` (two leading TABs, a U+2014 em dash):

```
^I^I// Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture$
^I^I// from filter_chains[] M-bM-^@M-^T no mixed-TLS-rule cross-check here.$
```

⚠️ **AND PROVE THE GUARD LIVE** before believing a match — mutate one character in a scratch copy and
show the digest moves (method note 7i). For `quic.go`, `internal/tls/**` and `internal/stats/**` a
whole-file `sha256sum` IS correct and sufficient: **no edit-roster row names any of them** (verified by
grepping the roster block; the single hit is the byte-untouched sentence itself).

### ⚠️ 0.14 — `SPEC.md` §11 ROW 8 MIS-ATTRIBUTES ONE EDIT: `DECISIONS.md:17359` IS IN **ADR-0297**, NOT ADR-0296

Row 8 files four edits under the heading *"ADR-0296 §Decision (a) and §Context ¶8(ii) amended in place
… stale cite `manager_test.go:2137` at `:17359` corrected."* Resolved by backward heading search,
`:17359` lands in **`## ADR-0297` at `:17324`**, not ADR-0296 (`:17256`). The edit is correct; its
FILING is wrong, and an IMPL auditor scoping row 8 to ADR-0296's body would not find it. Task 8 states
the true enclosing ADR for each of the four.

### ⚠️ 0.15 — TWO SMALLER CORRECTIONS, RECORDED SO THE IMPL DOES NOT RE-DERIVE THEM

- **`buildListenerRuntime` (without `WithCtx`) DOES NOT EXIST.** The sole definition is
  `buildListenerRuntimeWithCtx` at `manager.go:562`, with one call site at `:330`. **No test in
  `internal/listener` calls either directly** — every test goes through `NewManager`. Any task written
  against the shorter name would not compile.
- **Site 4's comment block runs `:2325-2340`, not `:2325-2332`** as §0.3 and §11 row 6 both scope it.
  The extra eight lines carry the stale cites *"`manager.go:692 and :562`"* at `:2337` that §0.6 of the
  SPEC separately flags — **so the stale-cite fix falls OUTSIDE the edit range as §11 row 6 states it.**
  Task 7 widens the range to `:2325-2340` and says why.

---

## 1. Stage scope, MEASURED

### 1.1 What this PLAN commit touches — FOUR files

`docs/envoy-go/phases/96-listener-default-chain-tlsmode/PLAN.md` (new) ·
`docs/envoy-go/STATE.md` (rolled IN PLACE) ·
`docs/envoy-go/STATE_HISTORY.md` (**+2 / -0**, parenthetical append) ·
`next-prompt.txt` (rolled, `git add -f` — it is TRACKED but gitignored). **Nothing else.**

⚠️ **NOT `PLAN.md` "only"** — §0.2 measures the real scope across four precedents. ⚠️ **And nothing
under `internal/` or `test/`, no `ROADMAP.md`, no `BEHAVIOR_CONTRACT.md`, no `DECISIONS.md`** — those
four negatives hold in 4/4 modern precedents and 7/7 PLAN-stage commits examined. ⇒ **every figure in
§7 that belongs to those files must STAY unchanged across this stage**, and §7 records the pre-edit
baseline so the close can prove it.

**No `PROGRESS.md` at this PLAN** — the 92/93/94/95 posture; `SPEC.md` §11 row 15 assigns it to the
IMPL. **A STANDING DEPARTURE, named rather than claimed** (§0.3).

### 1.2 The split gate — EVALUATED, NOT SPLIT, AND THE REASONING IS STATED SO A REVIEWER CAN OVERTURN IT

`BOOTSTRAP_PROMPT.md` §6.1 (read at the repo root at this tip, `:285-292`) triggers a split if `PLAN.md`
exceeds **~25 numbered tasks** OR estimates exceed **~1500 lines of code** of net change. ⚠️ **§5 appears
VERBATIM TWICE** — §5 and the §11 Skill Routing Appendix; the second copy's offset is NON-constant, so it
was located by HEADING, never by arithmetic.

**Task count: 19** (§5). **DERIVED here** — `SPEC.md` §14 item 1 deliberately quotes none. Under the gate.

**LoC — anchored on the MEASURED structural precedent of §0.5 rather than on an unanchored guess:**

| component | basis | estimate |
|---|---|---|
| `internal/listener/manager.go` — the predicate | **MEASURED**: the exact line, applied and reverted by this stage's own agent | **+1 / -1** |
| `internal/listener/manager.go` — `registerListenerMetrics` doc (site 8) | one comment block | **+12 / -10** |
| `internal/listener/manager_test.go` — the two live-handshake crash pins + their shared dial helper | two shapes, pointers-then-drive, five value assertions each | **+260** |
| `internal/listener/manager_test.go` — arms (d) (e) (f) | three table arms on an existing table | **+130** |
| `internal/listener/manager_test.go` — INVERT `:2468-2470` | one assertion, inverted | **+8 / -6** |
| `internal/listener/manager_test.go` — prose sites 4, 5, 6, 7 | four comment blocks | **+35 / -28** |
| `internal/listener/quic_test.go` — prose site 9 | one comment block | **+12 / -9** |
| `test/fixtures/0121-…/driver/driver.go` | `0120`'s driver is **618**; `0121` is simpler — three arms not seven, no client-cert machinery, `direct_response` not `tcp_proxy` | **+450** |
| `test/fixtures/0121-…/envoy.yaml` + `envoy-go.yaml` | `0120` measured **103 + 89** | **+180** |
| `test/fixtures/0121-…/pki/*.pem` | three PEMs, no client leaf (`0120` shipped five at **45**) | **+28** |
| `test/differential/runner_test.go` | the blank import | **+1** |
| `test/fixtures/0121-…/expectations.yaml` | `0120` measured **242**; fewer arms | **+200** |
| `test/fixtures/0121-…/README.md` | `0120` measured **262**; fewer arms | **+250** |
| `DECISIONS.md` (ADR-0296 amendments, two stale cites, ADR-0318 §Decision + §Consequences) | precedent: `0a985a35` **+39** | **+70 / -12** |
| `BEHAVIOR_CONTRACT.md` (`:1973`, ledger chain entry) | precedent: `0a985a35` **+7 / -5** | **+8 / -3** |
| `ROADMAP.md` row 96 | flip + the `+0 fixtures` cell | **+3 / -3** |

**Totals, each labelled with its accounting — a figure without its measure is meaningless (method note 32/40):**

| accounting | phase 96 estimate | **phase 94 MEASURED (`0a985a35`)** | verdict |
|---|---|---|---|
| `.go` only | ≈ **+942 / -54** | **+1143** | under, and under the precedent |
| code + YAML + PEM | ≈ **+1150** | — | under |
| code + YAML + PEM + `expectations.yaml` (the accounting §0.5 used) | ≈ **+1350** | **+1635** | **under the precedent that was NOT split** |

`PROGRESS.md` (≈ +600) and `next-prompt.txt` are excluded from every column: a transcript and a router
are not lines of code, and `0a985a35` is measured on the same exclusion.

**DECISION: DO NOT SPLIT.** Four grounds, in order of weight:

1. ⚠️ **On the SAME accounting as the measured structural precedent, this row is SMALLER than one that
   was not split.** Phase 94's IMPL created fixture `0120` from nothing, added `manager_test.go` arms
   for the same counter family, and edited a predicate in the same function of the same file: **+1635**
   against this row's **≈ +1350**. The comparison is like-for-like, not analogical.
2. **The production edit is ONE LINE**, measured, `+1 / -1`. Phase 94's was seventy-three. The entire
   remaining cost is the surface that makes that one line falsifiable — and **splitting the pins away
   from the code is the one split that defeats the row.**
3. ⚠️ **The only clean seam — {unit layer} / {fixture `0121`} — would ship a remotely-triggerable
   process-crash fix with NO cross-side evidence.** `SPEC.md` §6 measured the consequence directly:
   deleting the fix leaves all 122 existing fixtures GREEN. A `96.1` shipping the predicate alone would
   have no differential gate at all, and a `96.2` shipping the fixture alone would gate nothing.
4. **19 tasks against ~25**, with no task carrying more than nine sub-steps.

⚠️ **`BOOTSTRAP_PROMPT.md` §6.1's MID-EXECUTION trigger still applies** — if any single task's sub-steps
blow past ~10 once contact with reality reveals complexity, the split happens THEN. The likeliest
candidates are **Task 10** (the `0121` driver) and **Task 2** (the first crash pin, which must also
introduce the shared live-handshake helper).

⚠️ **AND THE ESTIMATE ABOVE IS A LOWER BOUND** ([[reference_measured_prototype_is_a_lower_bound]] has
fired FOURTEEN consecutive rows, the phase-96 SPEC making it fourteen before any code was written). The
gate is evaluated against the lower bound because that is the only figure that exists; **if the IMPL's
actual net change crosses ~1500 on the `.go` reading, §6.1's mid-execution clause is the remedy, not a
retroactive re-reading of this verdict.**

---

## 2. Sentinel — RUN MECHANICALLY AT `da6ea191`, ACTUAL OUTPUT

Not inherited. All three checks, all four NCs and the check-(2) positive control were run at this
stage's own tip, before any edit.

### 2.1 The three checks

```
(1) want=128            NOT DONE: row 96                      <- ONE line, no GATE FAIL
(2)                     206 212 218 228 234 242               <- SIX
(3)                     (silent)
```

Verbatim check-(2) anchors: `206:` `212:` `218:` `228:` `234:` all
`remaining deferred (not-yet-chartered) candidates:` · `242:deferred candidates:`.

Per-line md5 of the six windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — ⚠️ **the
digest is METHOD-SENSITIVE and the method must be stated whenever it is quoted:**

`206 10d7807bf02d` · `212 4a92f7e62fc6` · `218 2a7eb298b9fd` · `228 242e53c6f7a3` · `234 b2680e6f4fbf`
· `242 6caa1c3ce0e7`

**All six BYTE-IDENTICAL to the phase-96 SPEC close.** ⇒ **THE SENTINEL DOES NOT FIRE, for TWO
independent reasons** — check (1) is non-silent AND check (2) reads SIX. **`stop` WAS EVALUATED AND
DELIBERATELY NOT CREATED**; verified absent at the git root and in the stage worktree.

⚠️ **DO NOT "FIX" THE SIX AND DO NOT FORECAST A DECREASE** — the history is `0 -> 1 -> 3 -> 4 -> 5 -> 6`
across ~40 phases, and **no gate may rest on a ` + ` split** (wrong on 3 of the 6 windows).
⚠️ **DELETING THE LAST DEFERRED-CANDIDATE LINE STILL ENDS THE PROJECT. DO NOT "TIDY" ONE.**

### 2.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

| control | result |
|---|---|
| **NC-A** — doctor row 62 to `in-progress`, then check (1) at `want=128` | **TWO lines**: `NOT DONE: row 62`, `NOT DONE: row 96`. The doctoring was INSPECTED before the result was trusted (`NC LANDED? [ in-progress ]`). |
| **NC-B** — `want=127` on the real file | **TWO lines**: `NOT DONE: row 96`, then `GATE FAIL: examined 128 data rows, expected 127` |
| **NC-C** — the mandatory check-(3) NC (`gRPC-family row` → `gRPC-XXXXXX row` in a scratch copy) | residual **0**; `NEVER OPENED: gRPC <- NC FIRED` |
| **NC-D** — `-family row` (⚠️ **pass `--` before the pattern** or the leading `-` reads as an option, rc=2, and the arithmetic prints `0`, which looks exactly like "no change") | **96 occurrences / 68 lines** |
| **check-(2) positive control** — substitute **BOTH** phrases (the longer does NOT contain the shorter, so a one-phrase control reports a residual of 5 and reads like a finding) | residual **0**, substitutions asserted **6** |

⚠️ **NC SHAPES CHANGE ACROSS A ROW CHANGE — NEVER INHERIT ONE.** NC-A and NC-B each read TWO at this
tip because row 96 is genuinely `in-progress`. **When the IMPL flips row 96 to `done`, check (1) goes
SILENT and NC-A and NC-B each drop from TWO lines to ONE.**

### 2.3 Row-shape baseline — measured BEFORE this stage's edits

`ROADMAP.md` row 96 reads **NF=8 under BOTH the naive and the escape-aware form**. Malformed rows,
escape-aware, are exactly IDs **57** (NF=9) and **69** (NF=10) — two, matching the SPEC's baseline.
⚠️ **The escape-aware command is `sed 's/\\|//g' F | awk -F'|' '…'` with NO file argument to awk** —
passing the file makes awk ignore stdin and print the NAIVE 17 under the escape-aware label.
⚠️ **DORMANT this stage — a PLAN does not touch `ROADMAP.md` — and LIVE again at the IMPL.**

### 2.4 The eviction instruments — a PAIR, run on BOTH files

⚠️ **The bare forms answer nothing:** on `STATE.md` the strict form and the naive substring form BOTH
read **5**, and both are invariant under which entry is evicted (five in, five out).

The §Recent list, read by DATE **and** by POSITION:

```
1  phase 96 (listener-default-chain-tlsmode) BRAINSTORM done  (2026-09-07)
2  phase 95 (tls-alpn-mismatch-fallback) IMPL done            (2026-09-07)
3  phase 95 (tls-alpn-mismatch-fallback) PLAN done            (2026-09-07)
4  phase 95 (tls-alpn-mismatch-fallback) SPEC done            (2026-09-07)
5  phase 95 (tls-alpn-mismatch-fallback) BRAINSTORM done      (2026-09-06)   <- unique oldest, AND the tail
```

**The FOUR-WAY tie is at the HEAD**, where it cannot affect which entry is oldest; date and position
AGREE on the evictee. ⚠️ **That agreement is a coincidence of this tip, not a rule** — the tie has now
sat in the MIDDLE, at the OLDEST POSITION, ABSENT, and at the HEAD across six consecutive closes.

**LABEL-BOUND PAIR, measured pre-roll on BOTH files** (the only discriminating form):

| probe | `STATE.md` | `STATE_HISTORY.md` |
|---|---|---|
| the evictee's own label | **1** | **0** |
| fabricated-label NC | 0 | 0 |
| positive control on a sibling label that IS present in the archive | 0 | **1** |

**Archive guard triple, pre-roll:** strict **163** · parenthetical **66** · loose **229** — and
**163 + 66 = 229 exactly**. ⚠️ **The strict form must move by ZERO across a correct close**; the raw
line delta is **+2**, not +1.

---

## 3. `0121`'s expectation map — **MEASURED ON BOTH SIDES**, not forecast

`SPEC.md` §14 item 5 requires the leaf set be measured on each side before the map is written, because a
pin can fail against correct code. It was, on the pinned digest (verified against `ENVOY_TARGET.md:3-4`
before any arm), with the reference in a container launched **by digest** and the subject built from
`da6ea191` with the one-line predicate applied temporarily and then reverted under `sha256sum -c`.

**Shape:** one listener, **no `filter_chains[]`**, a `default_filter_chain` carrying an
`envoy.transport_sockets.tls` `DownstreamTlsContext` (inline server PEM, `alpn_protocols`, **no**
`require_client_certificate`) + an HCM whose route is a `direct_response: {status: 200}`, plus a
placeholder STATIC cluster. `direct_response` is chosen deliberately: it makes
[[reference_ssl_stats_suppressed_by_fast_failing_upstream]] structurally impossible. Every arm returned
**HTTP 200** and the codes were confirmed BEFORE any counter was read.

### 3.1 The map that PASSES — named subset over `/stats/prometheus`, keyed on NAME, address label IGNORED

```
envoy_listener_ssl_handshake           = N
envoy_listener_ssl_no_certificate      = N     <- ⚠️ N, NOT 0
envoy_listener_ssl_fail_verify_error   = 0
envoy_listener_ssl_fail_verify_no_cert = 0
envoy_listener_ssl_connection_error    = 0
```

Measured at **N=1 and N=3**, on **both** sides, all five values identical cross-side:

| stat | ref N=1 | subj N=1 | ref N=3 | subj N=3 |
|---|---|---|---|---|
| `ssl.handshake` | 1 | 1 | **3** | **3** |
| `ssl.no_certificate` | 1 | 1 | **3** | **3** |
| `ssl.fail_verify_error` | 0 | 0 | 0 | 0 |
| `ssl.fail_verify_no_cert` | 0 | 0 | 0 | 0 |
| `ssl.connection_error` | 0 | 0 | 0 | 0 |

**`SPEC.md` §6's pin instruction is CONFIRMED, not refuted** — `handshake` and `no_certificate` at the
handshake count, the other three at `0`. The `{handshake: N, rest: 0}` map the SPEC warns against would
indeed fail against correct code, on BOTH sides, at both drive counts.

**DRIVE COUNT: N = 3.** N=1 is viable and strictly weaker — the value `1` is consistent both with a
per-connection counter and with a fire-once one, so N=1 cannot discriminate them. N=3 can.

The metric NAMES are byte-identical cross-side; only the labels differ —
`envoy_listener_address="0.0.0.0_10127"` on the reference against the subject's IPv6-wildcard form.
⚠️ **Key on the NAME and strip the label set entirely**, exactly as `0120`'s `scrapeProm` does.

### ⚠️ 3.2 — NEW FINDING: THE REFERENCE'S SEVENTEEN-NAME SET IS **POST-DRIVE**. AT BOOT IT IS **FOURTEEN**

`SPEC.md` §0.1 and §6 both quote **seventeen** listener-scope `ssl.*` names under the anchored form,
and every arm in §0.1's table drove three handshakes — so seventeen is correct *for that measurement*.
It is not correct at boot. Measured:

| reference, anchored `^listener\.[^:]*\.ssl\.` | names |
|---|---|
| N=0 (booted, never driven) | **14** |
| N=1 | **17** |
| N=3 | **17** |

The three that appear only after a handshake are the **dynamic families** — `ssl.ciphers.<suite>`,
`ssl.curves.<curve>`, `ssl.versions.<version>` (measured: `TLS_AES_256_GCM_SHA384`, `X25519`, `TLSv1.3`).
No `sigalgs.*` family appeared at all.

⚠️ **CONSEQUENCE FOR `0121`: any presence assertion over the seventeen-name set taken BEFORE the drive
fails against correct code.** The fixture asserts only the five-name SUBSET, and only at runner step 10,
strictly after both Drives — which is where `AssertStats` already runs. **The finding constrains the
design rather than changing it; it is recorded so a future arm is not written pre-drive.**

### ⚠️ 3.3 — REFUTATION/SHARPENING OF `SPEC.md` §0.9: `no_filter_chain_match` IS NOT MERELY `0` ON THE SUBJECT, **THE NAME DOES NOT EXIST THERE**

§0.9 records that the reference books `no_filter_chain_match: 0` on the ineligible-chain shape and warns
that a pin asserting `> 0` *"would fail against correct code on both sides."* Measured, the divergence is
one level up — it is a **NAME-level** divergence, not a value one:

- reference: `listener.0.0.0.0_10127.no_filter_chain_match: 0` present, plus `listener.admin.…`;
- subject: **absent** — `grep -c no_filter_chain_match` over the FULL `/stats` and the FULL
  `/stats/prometheus` both read **0**.

Confirmed independently from the source at this tip, which is decisive and needs no container: the string
`no_filter_chain_match` occurs **exactly once in the entire repository outside `docs/`**, and that one
occurrence is a comment in `internal/listener/tls_handshake_negative_test.go:25`. There is **no
production site**. `BEHAVIOR_CONTRACT.md` reads **0** hits.

⇒ ⚠️ **`0121` MUST NOT ASSERT `no_filter_chain_match` CROSS-SIDE AT ALL — not even `== 0`.** A
`== 0` pin fails on the subject side because the map has no such key, and `0120`'s `scrapeProm` returns
the zero value for a missing key, which would make the pin **silently vacuous** instead of red. Use
`downstream_cx_total` as the liveness co-assertion: it is present on both sides and reads **3** at N=3.

### 3.4 — `SPEC.md` §0.8's BARE-`ssl` TRAP IS CONFIRMED, AND ITS WITNESS IS **SIDE-DEPENDENT AS WELL AS CONFIG-DEPENDENT**

On the plaintext negative control the **anchored** matcher reads **0 on both sides** — the control the
SPEC requires. The **unanchored** `grep -c ssl` over the same output reads **4 on the reference**
(`http.admin.downstream_cx_ssl_active`/`_total`, `http.ingress_dfc.downstream_cx_ssl_active`/`_total`)
and **1 on the subject** (`server.acce`**`ssl`**`og_dropped`). §0.8 attributes these two witnesses to two
different rigs; measured here they appear **simultaneously, one per side, on the same config pair.**
⚠️ **Anchor on `^listener\.[^:]*\.ssl\.`, and NC the anchored form against the plaintext arm.**

### ⚠️ 3.5 — THE CRASH SITE IS `manager.go:1393`, THE **SUCCESS-PATH** Inc — NOT A FAILURE-CLASSIFIER SITE

At the un-fixed tip, one handshake against the shape-A listener kills the process. Anchored panic gate
`^panic:|DATA RACE|SIGSEGV` reads **2**, and the trace names the line:

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x10 pc=0xeb8cfb]
sync/atomic.(*Uint64).Add(...)
internal/stats.(*Counter).Inc(...)                        internal/stats/counter.go:22
internal/listener.(*listenerRuntime).serveConnection(...)  internal/listener/manager.go:1393
created by ...acceptLoop in goroutine 82                   internal/listener/manager.go:1268
```

`manager.go:1393` is `rt.sslHandshake.Inc()` — verified by direct read at this tip. It is the FIRST Inc
on the **success** path, reached by every completing handshake. ⚠️ **A crash pin that expected the fault
at `:1376` or `:1378` (the `fail_verify_*` classifier sites) would be looking at the wrong line** — those
are reachable only on a certificate failure, which this shape never produces. **Patched, the same drive
reads panic gate 0, three × 200, process alive.**

⚠️ **The un-fixed tip also registers ZERO of the five at N=0**, where the reference registers fourteen.
That is the QUIC-shaped half of the defect visible on the TCP shape: the hole exists before any traffic.

---

## 4. File structure

**Production (1 file, 2 edits):**

| file | change |
|---|---|
| `internal/listener/manager.go` | the one-line `tlsMode` predicate (§11 row 1) · the `registerListenerMetrics` doc correction, site 8 (§11 row 2) |

**Tests (3 files):** `internal/listener/manager_test.go` (the inversion, the deletion of the second dead
assertion, arms (d)(e)(f), the two live-handshake crash pins, prose sites 4-7) ·
`internal/listener/quic_test.go` (prose site 9) · `test/differential/runner_test.go` (the blank import —
⚠️ **the gate that is SILENTLY GREEN if missed**).

**New fixture `test/fixtures/0121-listener-default-chain-tls/` (7 files):** `driver/driver.go` ·
`envoy.yaml` · `envoy-go.yaml` · `expectations.yaml` · `README.md` · `pki/ca.pem` · `pki/server.pem` ·
`pki/server.key.pem`. ⚠️ **No client leaf** — the listener sends no `CertificateRequest`, which is what
makes `ssl.no_certificate` the second mover (§3.1).

**Docs at the IMPL:** `DECISIONS.md` (ADR-0296 §Decision (a) + §Context ¶8(ii); the two stale cites,
one of which is in ADR-0297 not ADR-0296 per §0.14; ADR-0318 §Decision + §Consequences appended in place
after the RETAINED italic footer — **no renumber, NO `---` separator**) · `BEHAVIOR_CONTRACT.md`
(`:1973` on BOTH axes per §0.7; the stat-surface ledger `+0, UNCHANGED` chain entry) · `ROADMAP.md` (row
96 → `done`, the `+0 fixtures` cell → `+1`, **and site 10 at `:136` per §0.6**) ·
`phases/96-…/PROGRESS.md` (new) · `STATE.md` · `STATE_HISTORY.md` · `next-prompt.txt`.

**Byte-untouched, asserted at Task 17:** `internal/listener/quic.go` · `internal/tls/**` (8 files) ·
`internal/stats/**` (23 files) — whole-file `sha256sum`, correct because no edit-roster row names them
— **and the D4 comment block at `manager.go:746-747`, under the two-part literal-anchored guard of
§0.13, because a whole-file digest there fails against correct code.**

### 4.1 STABLE ANCHORS — use these, never line numbers

Line anchors drift; literals and enclosing symbols do not. Every task anchors on one of these. All were
re-located by `grep -nF` at `da6ea191` and **every one is exact** — but the IMPL lands multi-line inserts
above several of them, so **a multi-insert shift is BANDED, never a scalar**
([[reference_line_shift_after_insert_is_banded]]).

| target | STABLE anchor | at `da6ea191` |
|---|---|---|
| the `tlsMode` write (the ONLY one in the file) | `tlsMode:                 anyTLS,` | `:825` |
| the accumulator's single `true` write | `anyTLS = true` (single occurrence) | `:643` |
| the mixed-TLS cross-check (NOT `:575`, NOT `:516-525`) | `if anyTLS && anyPlaintext` | `:683` |
| the registration gate | `if rt.tlsMode {` inside `func registerListenerMetrics(` | `:398` (func at `:394`) |
| the Inc guard | `if selected.tlsCfg != nil {` inside `func (rt *listenerRuntime) serveConnection(` | `:1363` |
| ⚠️ the CRASH site | `rt.sslHandshake.Inc()` — the SUCCESS-path Inc (§3.5) | `:1393` |
| the other four Inc sites | `rt.sslFailVerifyError.Inc()` · `rt.sslFailVerifyNoCert.Inc()` · `rt.sslConnectionError.Inc()` · `rt.sslNoCertificate.Inc()` | `:1376 :1378 :1385 :1398` |
| the default-chain build | `defaultChain = &chainInfo{serverNames: nil, tlsCfg: dfcTLS,` | `:765` |
| ⚠️ the default chain entering the name map | `chainByName[defaultSpec.Name] = defaultChain` (§0.10) | `:766` |
| the guard that is NOT the boundary | `if len(chains) == 0 && l.GetDefaultFilterChain() == nil {` | `:573` |
| ⚠️ **D4 — MUST NOT CHANGE** | `Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture` | `:746-747` |
| the five counter FIELDS on `listenerRuntime` | `sslHandshake` `sslFailVerifyError` `sslFailVerifyNoCert` `sslNoCertificate` `sslConnectionError` | `:187 :188 :189 :194 :200` |
| the vacuous guard | `func TestListenerMetrics_GateMatchesInc(` | `:2341`, body to `:2493` |
| the assertion to INVERT | `t.Error("plaintext listener: defaultChain has tlsCfg != nil")` | `:2469` |
| the assertion to DELETE (§0.10) | `t.Error("TLS listener: defaultChain has tlsCfg == nil")` | `:2402` |
| ⚠️ **NOT the assertion — the header for the five pointer checks; do not move** | `// THE LOAD-BEARING HALF.` — occurs TWICE, `:2404` and `:2471` | — |
| the one-way TLS listener starter (the §5.2 template) | `func startOneWayTLSListener(` | `:4877` |
| its ALPN sibling | `func startOneWayTLSListenerALPN(` | `:4915` |
| the mTLS starter | `func startMutualTLSListener(` | `:4681` |
| the PKI helper | `func mkTestPKI(` | `:4375` |
| the echo backend | `func startEchoBackend(` | `:4641` |
| ⚠️ the REAL loopback pair (**never `net.Pipe`**) | `func connPair(` | `:4407` |
| the round-trip driver | `func driveALPNAuthProbe(` → `type alpnAuthProbe` | `:5704` / `:5677` |
| ⚠️ the correct cross-product assertion | `func assertSSLCrossProduct(` + `var sslLeafRoster` (§0.12) | `:4755` / `:4725` |
| the ABSENT-name-detecting reader | `func counterValue(` (`quic_test.go`) — **NOT `pollCounter`** | `quic_test.go:66` |
| the QUIC default-chain builders | `func mkQUICListenerDefaultChain(` · `func mkQUICDownstreamTS(` | `:979` / `:803` |
| the QUIC template that must NOT be copied wholesale (§0.9) | `func TestBuildListenerRuntime_QUICDefaultFilterChain_QUICWrappedKeepsNextProtos(` | `:1062` |
| the shape-B skeleton | `func TestUnifiedDispatchDefaultFilterChainFallback(` | `:3219`, literal at `:3240-3257` |
| the TCP listener builders | `func mkListener(` · `func mkTLSListener(` · `func mkTLSChain(` · `func mkDownstreamTSInline(` | `:94 :775 :762 :629` |
| ⚠️ the ONLY QUIC registration path | `registerListenerMetrics(reg, rt)` inside `func startQUIC(` | `quic.go:45` |
| the house ADR guard | `^> \*\*STATUS: PROPOSED` resolved BACKWARD to `## ADR-0318` | `DECISIONS.md:18946` → `:18944` |
| the decoy (byte-untouched) | `^\*\*Status:\*\* PROPOSED` resolved BACKWARD to `## ADR-0231` | `:14866` → `:14864` |

⚠️ **PATHSPEC-SCOPE every symbol assertion and every `sed`.** ⚠️ **`internal/filter/hcm/chain.go` does
not exist — it is `internal/filter/http/chain.go`.**

---

## 5. Tasks

**19 tasks.** The count is DERIVED here (§1.2); `SPEC.md` §14 item 1 deliberately carries no figure.

⚠️ **THE ORDERING IS THE POINT, AND IT DISCHARGES `SPEC.md` §14 ITEM 4.** The un-fixed tip **is** the
negative control for roster rows 1, 2 and 3, and it is **consumed the moment the predicate is widened**.
Tasks 1-5 therefore all land and run **BEFORE** Task 6. Every one of them is written to be RED (or, for
two of them, to ABORT THE BINARY) at the tip and green after.

⚠️ **Every task ends with a commit.** Subagents commit LOCALLY on their own stage branch with EXPLICIT
PATHSPECS; the controller merges and squashes. **Subagents do not push.**

---

### Task 1: Prove the panic gate LIVE and record the un-fixed baseline

**Files:** none committed — this task produces evidence, not code.
**Interfaces:** Produces the panic-gate command and the tip baseline that Tasks 2, 3 and 18 compare against.

- [ ] **Step 1.** Record the tip baseline for the package under test:
      `go test ./internal/listener/ -count=1 -v 2>&1 | tee $SCRATCH/t1-base.log`. Assert BOTH
      `RC` and a nonzero `=== RUN` count — ⚠️ **`RUN=0` beside `RC=0` is a vacuous green**, and without
      `-v` there are no `=== RUN` lines at all. Count failures with the anchored form
      `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`; ⚠️ an unanchored `grep -c FAIL` reads nonzero on a
      fully green tree.
- [ ] **Step 2. PROVE THE PANIC GATE LIVE — roster row 8.** A gate that reads 0 has not been shown to
      work. In a scratch copy of a trivial test, insert `panic("t1 gate liveness probe")`, run it, and
      confirm `grep -cE '^panic:|DATA RACE|SIGSEGV'` reads **non-zero**. Then remove it and confirm the
      same command reads **0** on the clean run. Record both numbers.
- [ ] **Step 3.** Record, from `$SCRATCH/t1-base.log`, that `TestListenerMetrics_GateMatchesInc` is
      currently **GREEN** — it must be, because it is vacuous on the only shape that matters (§0.8).
      This green is what Tasks 4 and 5 turn red.
- [ ] **Step 4.** Confirm the selector resolves before believing any colour:
      `go list ./internal/listener/...` must print the package. ⚠️ A selector naming a package that does
      not exist prints `FAIL … [setup failed]` and EXITS 1, which reads exactly like a real red run.
- [ ] **Step 5.** Prove the tree is clean: `git -C <wt> status --porcelain --untracked-files=all` EMPTY.
      ⚠️ **If you wrote a probe file, delete it and `sha256sum -c` a pre-capture** — and COMMIT anything
      else pending FIRST, because `git checkout --` restores from HEAD.

**No commit** (this task lands no bytes). Record the four figures in the task log.

---

### Task 2: The live-handshake crash pin — **SHAPE A**, RED-AT-TIP BY PROCESS ABORT

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Produces `startOneWayTLSListenerDefaultChain(t *testing.T, pki handshakeTestPKI) (*stats.Registry, string)` — the shape-A starter Task 3 does **not** reuse (it needs its own) but Task 5's arm (d) mirrors. **This is `SPEC.md` §12 roster row 1.**

- [ ] **Step 1. Write the starter by MOVING the transport socket, not by inventing a listener.** Copy
      `startOneWayTLSListener` (`:4877`) and move its `mkDownstreamTSInline(...)` transport socket from
      the `filter_chains[]` entry to `DefaultFilterChain`, leaving `FilterChains` **empty**. Its doc at
      `:4911-4914` already states the phase-95 convention this row depends on — *"it sends NO
      CertificateRequest, so EVERY handshake that COMPLETES on this listener books `ssl.no_certificate`
      as well as `ssl.handshake`."* Keep the `NewManager` + `mgr.Start(ctx)` + `t.Cleanup(cancel)` +
      `t.Cleanup(mgr.Stop)` + `mgr.Listeners()[0].Addr` shape verbatim; it binds `127.0.0.1:0`.
      ⚠️ **Build the listener with the package's OWN helpers** — a hand-rolled `*stdtls.Config` is an
      invented input no production path produces.
- [ ] **Step 2. ASSERT THE POINTERS BEFORE DIALLING** ([[reference_nil_stats_counter_inc_crashes_goroutine]],
      method note 7k). `sslHandshake == nil` is the CAUSE; the SIGSEGV is the EFFECT, and a probe that
      observes only the crash proves something happened, not what. In order, each with its own
      `t.Errorf`: `rt.tlsMode == true` · `rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil` ·
      `len(rt.chainSpecs) == 0` · then all five pointers non-nil.
      ⚠️ **`t.Errorf`, never `t.Fatalf`** — a `Fatalf` makes every later assertion dead code
      ([[reference_fatalf_makes_assertions_unreachable]]). Setup failures (`NewManager`, `Start`) stay
      fatal; that is the landed discipline in this file (18 `t.Error*` for properties, 10 `t.Fatal*` for
      setup).
- [ ] **Step 3. COMPLETE A REAL HANDSHAKE *AND DRIVE A REQUEST THROUGH IT*.** ⚠️ *"Did the handshake
      complete"* is not *"was the connection served"*. Reuse `driveALPNAuthProbe` (`:5704`), which takes
      `(t, addr, pki, offer, sendCert, minVer, maxVer)` and returns
      `alpnAuthProbe{hsCompleted, served, negotiated, certSent, clientErr}` — its round trip at
      `:5753-5771` writes a payload and `io.ReadFull`s it back through `startEchoBackend`. Assert
      `hsCompleted` **and** `served`. ⚠️ **NEVER `net.Pipe`** — it deadlocks a client-cert handshake and
      the package already documents this at `:4402-4406`, `:5066`, `:5182`, `:5623`; use the loopback
      TCP path these starters already take.
- [ ] **Step 4. ASSERT THE LEAF SET WITH `assertSSLCrossProduct`, NOT A HAND-ROLLED MAP** (§0.12):
      `assertSSLCrossProduct(t, reg, addr, "handshake", "no_certificate")`. It polls the two movers and
      reads the three non-movers with `counterValue`, which **`t.Errorf`s on an ABSENT name**.
      ⚠️ **`pollCounter` returns a silent `0` for both "registered and zero" and "never registered", so
      using it for the non-movers makes the assertion pass vacuously at the un-fixed tip.**
      ⚠️ **Poll the gauge; NO SLEEPS.** ⚠️ **Never register a stat inside `Registry.Walk`** — the
      callback runs under `RLock` and registering re-enters the write lock, DEADLOCKING the process.
- [ ] **Step 5. RUN IT AT THE UN-FIXED TIP AND RECORD THE ABORT.** Expected, measured by this stage:
      the process dies, `curl`-equivalent gets nothing, and the anchored gate
      `grep -cE '^panic:|DATA RACE|SIGSEGV'` reads **non-zero**, with the trace naming
      `internal/stats/counter.go:22` and **`internal/listener/manager.go:1393`** — `rt.sslHandshake.Inc()`,
      the SUCCESS-path Inc. ⚠️ **A pin expecting the fault at `:1376`/`:1378` is looking at the wrong
      line**: those are the `fail_verify_*` classifier sites and this shape never reaches them.
      ⚠️ **The Step-2 pointer assertions must be observed to fire FIRST**, before the abort — that is
      what distinguishes "the cause" from "something happened".
- [ ] **Step 6. Also record the N=0 half:** at the un-fixed tip, before any dial, the listener boots and
      registers **ZERO** of the five, where the reference registers fourteen (§3.2, §3.5). Assert it.
- [ ] **Step 7.** `gofmt -l` (gate on OUTPUT, it never exits non-zero) and the US-locale misspell sweep
      on the new comments.

**Commit.**

---

### Task 3: The live-handshake crash pin — **SHAPE B**, and the proof that `len(chains) == 0` is NOT the boundary

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Consumes Task 2's assertion shape. **This is `SPEC.md` §12 roster row 2, and it is the arm that makes the class a class.**

- [ ] **Step 1. Build shape B from the landed skeleton, not from scratch.**
      `TestUnifiedDispatchDefaultFilterChainFallback` (`:3219`, listener literal at `:3240-3257`) already
      constructs exactly this arrangement: an ineligible `filter_chains[0]` carrying
      `FilterChainMatch{DestinationPort: wrapperspb.UInt32(resolvedPort + 1)}` beside a
      `DefaultFilterChain`, with a real port resolved via a probe listener and a live dial. **Swap its
      default chain's plaintext filters for `TransportSocket: mkDownstreamTSInline(t, certPEM, keyPEM)`**
      and keep everything else.
- [ ] **Step 2.** Same ordered assertions as Task 2 Step 2, with **`len(rt.chainSpecs) == 1`** instead of
      `== 0`. ⚠️ **This is the whole point of the task**: shape B has a filter chain and crashes
      identically, so the `len(chains) == 0` guard at `manager.go:573` is not the boundary and must NOT
      be widened instead of the predicate (`SPEC.md` §3).
- [ ] **Step 3.** Handshake + round trip + `assertSSLCrossProduct(t, reg, addr, "handshake", "no_certificate")`,
      exactly as Task 2 Step 4.
- [ ] **Step 4. RUN IT AT THE UN-FIXED TIP.** Expected: the binary ABORTS, anchored panic gate non-zero,
      same `manager.go:1393` frame. **Record the trace.**
- [ ] **Step 5. ⚠️ PIN BOTH SHAPES, NOT ONE.** Pinning only shape A would re-mint the §0.3/§0.6 error at
      a smaller scale — narrowing a class into a claim about one member. State in the test's doc comment
      that these are two members of one class and that the class is defined by *"the default slot carries
      TLS and `filter_chains[]` does not"*, never by chain count.
- [ ] **Step 6.** `gofmt -l` on OUTPUT; misspell sweep.

**Commit.**

---

### Task 4: Arm (f) — the QUIC shape, a REGISTRATION pin that must **Start**

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Consumes `mkQUICListenerDefaultChain` (`:979`) and `mkQUICDownstreamTS` (`:803`). **This is `SPEC.md` §12 roster row 3.**

- [ ] **Step 1.** Add a fourth `t.Run` block to `TestListenerMetrics_GateMatchesInc`, named
      `"quic_default_chain_tls"`. ⚠️ **INLINE the body in the landed style** — the test has **no table
      and no shared assertion body** (§0.8), so "add an arm" means "add a `t.Run` block", and §5.1's
      `t.Helper()` clause is vacuous unless a shared body is created, which this task deliberately does
      not do.
- [ ] **Step 2.** Build with `mkQUICDownstreamTS(t, testAlphaCertPEM, testAlphaKeyPEM, []string{"h3"})`
      → `mkQUICListenerDefaultChain(t, "c_echo", ts)` → `mkBoot` → `NewManager`. That helper already
      builds shape A-QUIC: `UdpListenerConfig.QuicOptions`, **zero `FilterChains`**, only
      `DefaultFilterChain{TransportSocket: ts, Filters: [tcp_proxy]}`.
- [ ] **Step 3. 🔴 CALL `mgr.Start(ctx)` AND `defer mgr.Stop()`.** ⚠️ **Without it this arm is red
      regardless of the fix and proves nothing** (§0.9): `registerListenerMetrics` runs only at Start —
      `manager.go:1179` for TCP and **`quic.go:45` for QUIC** — and the template this arm is modelled on
      (`manager_test.go:1062`) calls only `NewManager`. Arms (b) and (c) already call Start; copy them,
      not `:1062`.
- [ ] **Step 4.** Assert, each with its own `t.Errorf`: `rt.tlsMode == true` · `rt.kind == kindQUIC` ·
      the five pointers non-nil.
- [ ] **Step 5. ⚠️ DO NOT COPY ARM (c)'s `for n, ci := range rt.chainByName` LOOP** (§0.10):
      `manager.go:766` inserts the default chain into `chainByName`, so that loop would fire on this
      arm's own TLS default chain. And arm (c)'s `if len(rt.chainByName) == 0 { t.Fatal("vacuous") }`
      guard passes for the WRONG REASON here — `len(chainSpecs) == 0` while `len(chainByName) == 1`.
      Assert non-vacuity on `len(rt.chainSpecs)` and `rt.defaultChain != nil` instead.
- [ ] **Step 6.** Doc-comment the arm: **this is a REGISTRATION pin, not a traffic pin.** `Manager.Start`
      launches no accept loop for `kindQUIC`, so `serveConnection`'s Inc sites are structurally
      unreachable, the counters stay permanently zero, and **that is PARITY, unchanged by this row.**
- [ ] **Step 7. RUN IT AT THE UN-FIXED TIP.** Expected: **the five pointers are nil — a REGISTRATION
      failure with NO crash.** ⚠️ That is roster row 3's distinguishing signature: shape A and shape B
      abort the binary, the QUIC shape does not. **Record which of the two outcomes occurred**; if it
      crashes, the arm is wrong.
- [ ] **Step 8.** `gofmt -l` on OUTPUT.

**Commit.**

---

### Task 5: INVERT the dead assertion, DELETE its mirror, and add arms (d) and (e)

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Consumes Task 2's shape-A construction and Task 3's shape-B construction. **This is a real green→red→green transition on a landed test, not a fresh green.**

- [ ] **Step 1. INVERT `:2468-2470`** (anchored on `t.Error("plaintext listener: defaultChain has tlsCfg != nil")`).
      Today it treats **the crashing configuration** as a failure on a `tlsMode == false` listener. Under
      the fix that combination **cannot arise** — a TLS default chain now sets `tlsMode` — so the correct
      replacement asserts the post-fix invariant: *the five pointers are non-nil **iff** any chain
      reachable on this listener, INCLUDING the default slot, carries TLS.* Express it as
      `rt.tlsMode == (anyChainCarriesTLS || defaultChainCarriesTLS)` computed from `rt`, and keep the
      arm-(c) polarity (this arm is still the plaintext listener, so both disjuncts are false and
      `tlsMode` must be false).
- [ ] **Step 2. ⚠️ DO NOT MOVE OR DELETE `// THE LOAD-BEARING HALF.` at `:2471`** (§0.11). It is the
      header for the five pointer-nil assertions at `:2472-2491`, not an annotation on the `if` above it;
      the identical comment sits at `:2404` for the same purpose in arm (b).
- [ ] **Step 3. 🔴 DELETE `:2401-2403`, arm (b)'s mirror** (§0.10) —
      `t.Error("TLS listener: defaultChain has tlsCfg == nil")`. It is equally dead today, and if it ever
      went live it would be a **FALSE POSITIVE**: ADR-0080 §Decision 3 explicitly authorises a plaintext
      `default_filter_chain` beside a TLS `filter_chains[]` entry, which is precisely the arrangement
      §Consequences (c) illustrates as legal. **There is no correct assertion to replace it with, so it
      is deleted rather than inverted.** State that in the commit message. ⚠️ Leave `:2404`'s
      `// THE LOAD-BEARING HALF.` in place.
- [ ] **Step 4. ADD arm (d) `"default_chain_tls_zero_filter_chains"`** — shape A. Assert in order:
      `rt.tlsMode == true` · `rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil` ·
      `len(rt.chainSpecs) == 0` · all five pointers non-nil, **each with its own `t.Errorf`**.
- [ ] **Step 5. ADD arm (e) `"default_chain_tls_ineligible_plaintext_chain"`** — shape B. Same five
      assertions plus `len(rt.chainSpecs) == 1`. ⚠️ **BOTH ARMS ARE REQUIRED.**
- [ ] **Step 6. ⚠️ NEITHER NEW ARM MAY COPY ARM (c)'s `chainByName` LOOP** (§0.10) — the default chain is
      in that map.
- [ ] **Step 7. Correct the test's own doc comment, `:2325-2340`** — ⚠️ **the block runs to `:2340`, not
      `:2332` as `SPEC.md` §0.3 and §11 row 6 both scope it** (§0.15). Its head asserts the two gates are
      `EQUIVALENT` *"because a listener is all-TLS or all-plaintext and never both"*; that is the head of
      the class this row refutes. **And `:2337` carries the stale cites `manager.go:692` and `:562`,
      which fall outside the `:2325-2332` range as written** — correct them to `:825` (the `tlsMode`
      write) and `:675`/`:765` (the two `tlsCfg` writes) by LITERAL text.
- [ ] **Step 8. RUN AT THE UN-FIXED TIP.** Expected: arms (d) and (e) are **RED on the pointer
      assertions** (build-time only — these arms do not dial, so they do not abort), and the inverted
      `:2468-2470` stays green because arm (c) is still the plaintext listener.
      ⚠️ **Record WHICH assertion fired, per arm** — a red run is not evidence until you know which
      property failed.
- [ ] **Step 9.** `gofmt -l` on OUTPUT; misspell sweep on the rewritten comments.

**Commit.**

---

### Task 6: The production edit — ONE LINE — and PROVE it landed

**Files:** Modify `internal/listener/manager.go`
**Interfaces:** Consumes nothing. Turns Tasks 2, 3, 4 and 5 green. **This is the row.**

- [ ] **Step 1.** Anchor on the literal `tlsMode:                 anyTLS,` — the **only** `tlsMode:` in
      the file (`:825` at this tip) — and replace it with, verbatim:

```go
			tlsMode:                 anyTLS || (defaultChain != nil && defaultChain.tlsCfg != nil),
```

      `defaultChain` is built at `:765` and the composite literal opens at `:822` and closes at `:835`,
      so it is in scope with no reordering. **`+1 / -1`, one file.**
- [ ] **Step 2. ⚠️ DO NOT WIDEN `manager.go:573`'s `len(chains) == 0 && l.GetDefaultFilterChain() == nil`
      GUARD INSTEAD.** It is not the boundary — Task 3 proves shape B has `len(chainSpecs) == 1` and
      crashes identically.
- [ ] **Step 3. ⚠️ DO NOT TOUCH THE D4 COMMENT BLOCK AT `:746-747`.** ADR-0080 §Decision 3 authorises
      that exemption verbatim, and "fixing" D4 would break ADR-0080 parity. Verify with the two-part
      guard of §0.13 immediately after the edit: uniqueness must read **1** and the digest must still be
      `9154e453c99515d96a5d4bd9aea17fcd9b70c39682bf4418f40e0136a8ec0817`.
- [ ] **Step 4. ASSERT THE SYMBOL, THEN DRIVE THE ARM.** A build is not evidence the edit landed.
      (a) `grep -nF 'anyTLS || (defaultChain != nil && defaultChain.tlsCfg != nil)' internal/listener/manager.go`
      must return exactly one line, and it must be inside `func buildListenerRuntimeWithCtx(`
      — ⚠️ **`buildListenerRuntime` without `WithCtx` DOES NOT EXIST** (§0.15). (b) Tasks 2, 3, 4 and 5
      must all flip green in one run.
- [ ] **Step 5. RE-RUN EVERY NC BY REVERTING, ONE AT A TIME** — roster rows 1, 2, 3. Revert the
      predicate, run Task 2 (binary aborts), Task 3 (binary aborts), Task 4 (five pointers nil, no
      crash). ⚠️ **Gate each on the ANCHORED form `^panic:|DATA RACE|SIGSEGV`, and remember Task 1
      proved that gate live** — a zero there is otherwise indistinguishable from a broken command.
      Restore the predicate and re-confirm green before committing.
- [ ] **Step 6. Correct site 8, the `registerListenerMetrics` doc at `:384-393`.** Its last two sentences
      are false: *"This is sufficient for QUIC by construction: `startQUIC` hard-errors when the chain
      carries no TLS config (`quic.go:33-36`), so every QUIC listener that boots has `tlsMode == true`."*
      ⚠️ **LEAD WITH WHAT SURVIVES**: the gate IS `rt.tlsMode` alone and there IS deliberately no kind
      check — that decision is correct and unchanged. What is repealed is the justification. State the
      measured replacement: `quicTLSConfig()` (`quic.go:56-58`) returns `rt.defaultChain.tlsCfg` FIRST,
      before consulting `chainByName`, so a QUIC listener with zero `filter_chains[]` and a QUIC-wrapped
      default chain boots with `tlsMode == false` — **which is why this row widens the write site rather
      than adding a kind check.** ⚠️ **Keep `:392-393`'s "do NOT re-express this as *has a TCP-style TLS
      transport socket*" clause** — it is TRUE and still load-bearing.
- [ ] **Step 7.** `gofmt -l` on OUTPUT; misspell sweep; `go vet ./internal/listener/`.

**Commit.**

---

### Task 7: The `.go` prose reconciliation — sites 4, 5, 6, 7 and 9

**Files:** Modify `internal/listener/manager_test.go`, `internal/listener/quic_test.go`
**Interfaces:** Consumes Task 6. (Site 8 landed in Task 6 Step 6 because it sits beside the code it describes; sites 1-3 and 10 are documents and land in Tasks 8, 14 and 16.)

- [ ] **Step 1. Site 4 is already done** by Task 5 Step 7 (`manager_test.go:2325-2340`). Confirm, do not
      redo.
- [ ] **Step 2. Site 5, `manager_test.go:2342-2344`** — arm (a)'s comment. Its *"and hence what makes the
      two gates equivalent at all"* is a non-sequitur: the mixed-TLS reject constrains `filter_chains[]`
      only, and ADR-0080 §Decision 3 exempts the default slot. Keep the true half (arm (a) DOES pin a
      build-time reject); strike the inference.
- [ ] **Step 3. 🔴 Site 6, `manager_test.go:2433-2436`** — arm (c)'s *"The nil fields are BY DESIGN … do
      not add nil guards."* **FALSE AND ACTIVELY HARMFUL**: it is the instruction that would have
      prevented the crash from being caught. Replace with the measured statement — the Inc sites stay
      inside `if selected.tlsCfg != nil` (that is still right), but **the guard is not what makes the
      plaintext listener safe**; the registration gate and the Inc guard *agreeing on that shape* is.
      Cite ADR-0318.
- [ ] **Step 4. Site 7, `manager_test.go:4993-5000`** — the doc on
      `TestServeConnection_PlaintextListenerIncrementsNoSSL`. ⚠️ **The test's BEHAVIOUR is correct and
      stays; only its rationale is wrong.** It endorses `tlsCfg != nil` as the *sufficient* guard — the
      exact predicate that fails on a TLS default chain. Rewrite to say the guard is *necessary* and that
      sufficiency comes from the registration gate covering every shape the guard admits. ⚠️ **The block
      runs to `:5000` including its `⚠️ This test is GREEN ON ARRIVAL` line; keep that line.**
- [ ] **Step 5. Site 9, `internal/listener/quic_test.go:225-232`** — the doc on
      `TestQUICListener_RegistersSSLNamesAtZero`. Its *"`startQUIC` hard-errors without a TLS config so
      `rt.tlsMode` is necessarily true"* is the §0.4 corollary, refuted. **Lead with what survives**: the
      test's assertion (five names registered, permanently zero across a completed HTTP/3 handshake) is
      CORRECT and unchanged. Replace only the reason, and note that after this row the shape that used to
      register zero now registers five.
- [ ] **Step 6. RECONCILE THE WHOLE OCCURRENCE SET, AND SAY WHAT YOU LEFT.** ⚠️ **Sweep the CLAIM, not
      one spelling** (§0.6) — the phrase appears as *"every QUIC listener that boots has"*, *"every
      booting QUIC listener has"*, and *"`startQUIC` hard-errors without a TLS config so `rt.tlsMode` is
      necessarily true"*. ⚠️ **Sweep CASE-INSENSITIVELY** — `manager_test.go:2327` spells it `EQUIVALENT`
      and is invisible to a lowercase grep. Then restate the SPEC's deliberately-LEFT list and confirm it
      is still accurate at your tip: `manager.go:179-184` and `manager_test.go:2189-2194` (misleading but
      defensible, mechanism stated correctly); `manager_test.go:2281-2282`, `:2414-2419`, `:2423-2427`,
      `quic_test.go:274-282`, `BEHAVIOR_CONTRACT.md:1042` and `:1967`, `DECISIONS.md:18842`
      (ADR-0316 `D-TLSCE-NILGATE`) — **all TRUE, all left.** Historical copies under
      `docs/envoy-go/phases/74-*, 92-*, 95-*` are records and stay byte-untouched.
      ⚠️ `DECISIONS.md:18950/:18954/:18964` resolve backward to **`## ADR-0318`** — this row's own
      §Context, which quotes the false claim in order to repeal it. **Correctly excluded; do not "fix"
      them.**
- [ ] **Step 7.** `gofmt -l` on OUTPUT; misspell sweep (US locale) on every rewritten comment.

**Commit.**

---

### Task 8: `DECISIONS.md` — sites 1 and 2, plus TWO stale cites (one of them in a different ADR)

**Files:** Modify `docs/envoy-go/DECISIONS.md`
**Interfaces:** Consumes Task 6.

- [ ] **Step 1. ⚠️ ADR-0296 IS AMENDED, NOT SUPERSEDED, AND THE AMENDMENT LEADS WITH WHAT SURVIVES.** Its
      **decision** — *the registration gate is `rt.tlsMode` alone, no kind check* — is **CORRECT and
      stands.** Only its **justification** is repealed. This is the ADR-0296/0297 in-place-correction
      precedent; do not renumber and do not add an ADR.
- [ ] **Step 2. Site 1, `:17296` (ADR-0296 §Decision (a))** — resolved by backward heading search to
      `## ADR-0296` at `:17256`. Strike *"every QUIC listener that boots has `tlsMode == true`"* and
      *"envoy-go rejects mixed TLS+plaintext chains on one listener … so a listener is wholly TLS or
      wholly not."* Replace with the ADR-0080 §Decision 3 narrowing: the cross-chain rule applies WITHIN
      `filter_chains[]` only, the default slot is structurally separate, and the gate is correct for a
      different reason — it is now written from a predicate that covers both slots.
- [ ] **Step 3. Site 2, `:17276` (ADR-0296 §Context ¶8(ii))** — the *"provably sufficient"* clause. Same
      narrowing, same lead-with-what-survives shape.
- [ ] **Step 4. Stale cite A: `manager.go:516-525` at `:17296`.** That range is `validateQUICOptions`'
      doc comment and its `proof_source_config` reject. **The true site is `manager.go:683-690`** — anchor
      on the literal `if anyTLS && anyPlaintext`. Correct by literal, not by arithmetic.
- [ ] **Step 5. 🔴 Stale cite B: `manager_test.go:2137` at `:17359` — AND `:17359` IS IN ADR-0297, NOT
      ADR-0296** (§0.14). `SPEC.md` §11 row 8 files this edit under the ADR-0296 row, which is a
      mis-attribution: backward heading search resolves `:17359` to `## ADR-0297` at `:17324`. The cite
      itself is wrong too — `:2137` is mid-comment in an unrelated helper; the true site is
      `manager_test.go:2341`, `func TestListenerMetrics_GateMatchesInc(`. **Correct the cite AND record
      the true enclosing ADR**, so the IMPL's auditor can find it.
- [ ] **Step 6. Resolve EVERY edited hit to its enclosing ADR by BACKWARD heading search and record the
      pair**, never by eye:
      `awk 'NR<=<HITLINE> && /^## ADR-/ {n=NR; h=$0} END {print n, h}' docs/envoy-go/DECISIONS.md`.
- [ ] **Step 7. ⚠️ THE STRUCTURAL FIGURES MUST NOT MOVE.** This task adds no ADR and no separator:
      `grep -c '^---$'` STAYS **216**, `grep -c '^## ADR-'` STAYS **317**, bare `grep -c '^## '` STAYS
      **325**, tail STAYS `ADR-0318`, and `grep -c '^## ADR-0319'` STAYS **0**. Measure before and after.
- [ ] **Step 8. The house `PROPOSED` guard STAYS ARMED through this task** — it is disarmed in Task 15,
      not here. Verify **BY LINE AND BY ADR**, never by the count alone: the house-form hit must still
      resolve backward to `## ADR-0318`, and the `^\*\*Status:\*\* PROPOSED` decoy must still resolve to
      `## ADR-0231` at `:14864`, byte-untouched. ⚠️ **NEVER gate on the unanchored middle-ground form
      `^\*\*Status:\*\*.*PROPOSED`.**

**Commit.**

---

### Task 9: Fixture `0121` — directory, PKI, and both bootstraps

**Files:** Create `test/fixtures/0121-listener-default-chain-tls/{envoy.yaml,envoy-go.yaml,pki/ca.pem,pki/server.pem,pki/server.key.pem}`
**Interfaces:** Produces the two rendered templates Task 10's `ReferenceBootstrap` / `SubjectConfig` consume, with substitution keys `{{.AdminPort}}`, `{{.ListenerPort}}`, `{{.BackendPort}}` and the pre-indented PEM keys `ServerCertIndented`, `ServerKeyIndented`.

- [ ] **Step 1. Index and port. `0121` is FREE** — `ls -d test/fixtures/*/ | wc -l` reads **122**, tail
      `0120-tls-connection-error`. ⚠️ **Use the `ls -d` form**: `grep -cE '^[0-9]{4}-'` reads **120**
      because it drops `0007a-cors` and `0007b-iteration-probe`, and a bash glob
      `test/fixtures/[0-9]{4}-*` is not a repetition at all — it errors. **Reference in-container
      listener port `10127`** — ⚠️ **NOT `10121`** (`0028` holds `10120`-`10125` as a contiguous
      six-listener run at `inputs/driver.go:65-70`) and **NOT `10126`** (`0120` holds it). Admin `9901`.
      Re-census `10127` in CODE SCOPE at your tip and **NC the census against `10126`**, which must read
      as taken.
- [ ] **Step 2. Generate three PEMs** — CA, server leaf, server key. ⚠️ **NO client leaf**: the listener
      carries **no `require_client_certificate`** and sends no `CertificateRequest`, which is exactly what
      makes `ssl.no_certificate` the second mover (§3.1). Follow the `0002/0004/0045` pattern
      (`pki/gen/main.go`) or `0120`'s committed shape. The leaf MUST carry a DNS SAN matching the
      `serverName` the driver dials, or the positive arm fails verification CLIENT-side and zeroes the
      counters with no server-side fault.
- [ ] **Step 3. `envoy.yaml` (reference).** ONE listener, **no `filter_chains[]` key at all**, a
      `default_filter_chain` carrying an `envoy.transport_sockets.tls` `DownstreamTlsContext` and an HCM
      whose single route is `direct_response: {status: 200, body: {inline_string: …}}`.
      ⚠️ **`direct_response` is chosen deliberately** — it makes
      [[reference_ssl_stats_suppressed_by_fast_failing_upstream]] structurally impossible, and §3's
      measurement was taken on exactly this shape. ⚠️ **PEMs as `inline_string:`, indented ONE level
      deeper than the key**; the container cannot read `filename:` paths and this fixture implements no
      `ReferenceLogMounter`. ⚠️ **Keep the PEM substitution keys OUT of YAML comments** — `text/template`
      expands actions inside `#` lines and splatters the continuation lines outside the comment.
      Cluster: **`STRICT_DNS` + `host.docker.internal`** with `dns_lookup_family: V4_ONLY` (ADR-0010);
      launch flag `--add-host=host.docker.internal:host-gateway`.
- [ ] **Step 4. `envoy-go.yaml` (subject).** Byte-symmetric with the reference apart from the two
      documented divergences: cluster type **`STATIC`** at `127.0.0.1`, and runner-allocated ports.
      ⚠️ **An omitted `clusters:` key BOOT-REJECTS envoy-go** — the placeholder cluster is required even
      though `direct_response` never reaches it. ⚠️ envoy-go **rejects `match.headers`**, **boot-rejects
      `access_log[].log_format`**, and **boot-rejects `TLSv1_0`/`TLSv1_1` in `tls_params`** — none of
      which this fixture needs; do not add them.
- [ ] **Step 5. Boot BOTH sides before writing a driver.** Reference: `--mode validate` then a real boot,
      reading the OUTPUT (`starting main dispatch loop`) for the verdict — ⚠️ **`timeout` rc=124 is
      shared by a healthy server and a hung boot; never read the exit code.** Subject: `-mode validate`
      then `-c`. Both must accept.
- [ ] **Step 6.** Tear down every container **BY NAME**, and only ones you created. ⚠️ A sibling session
      owns other containers; a `reaper_*` testcontainers Ryuk container is created by the differential
      itself and is REUSED — **leave it alone.**

**Commit.**

---

### Task 10: Fixture `0121` — `driver/driver.go`, with the MEASURED expectation map

**Files:** Create `test/fixtures/0121-listener-default-chain-tls/driver/driver.go`
**Interfaces:** Produces `func init() { fixture.RegisterFixture(fixtureName, &defaultChainTLSDriver{}) }` and the compile-time assertions `_ fixture.Driver` / `_ fixture.StatsAsserter`. Consumes Task 9's templates. Modelled on `0120`'s driver (**618** lines), which is the only precedent for a cross-side `ssl.*` assertion.

- [ ] **Step 1. The interface surface**, exactly `0120`'s shape:
      `BackendCount() int` → **1** (⚠️ the runner `t.Fatalf`s on `< 1`, even though `direct_response`
      never reaches the backend) · `SubjectListenerName() string` → `"l_dfc"` ·
      `ReferenceListenerPort() int` → **10127** · `ReferenceBootstrap([]int) string` ·
      `SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string` ·
      `DriveReference` / `DriveSubject` (⚠️ **ONE DIRECTORY = ONE RUNNER BRANCH**, so both delegate to a
      single `driveSide`) · `ProbeAdmin` · `AssertStats`.
      ⚠️ **The `StatsAsserter` compile-time assertion is MANDATORY** — the runner dispatches step 10 via a
      SILENT type assertion with no `else` branch, so a signature typo makes `ok == false` and the whole
      assertion **never runs while every tool stays quiet.**
- [ ] **Step 2. `driveSide`: THREE arms, N = 3.** Each arm completes a TLS handshake **and drives a full
      HTTP round trip**, asserting **HTTP 200** before any counter is believed.
      ⚠️ **Confirm the drive returned 200 first** — §3's measurement did, on every arm, on both sides.
      **N = 3 and not 1**, decided in §3.1: the value `1` is consistent both with a per-connection counter
      and with a fire-once one, so N=1 cannot discriminate them; N=3 can.
- [ ] **Step 3. `scrapeProm`** — copy `0120`'s (`:645`-ish), which fetches `/stats/prometheus` and returns
      `map[string]uint64` keyed on the metric NAME **with the label set stripped entirely**. ⚠️ Stripping
      is REQUIRED, not a convenience: the two sides render `envoy_listener_address="0.0.0.0_10127"` and
      the subject's IPv6-wildcard form, so a label-preserving key is cross-side incomparable by
      construction. `ParseFloat` not `ParseUint`; skip non-finite and negative values.
- [ ] **Step 4. `AssertStats` — the MEASURED map (§3.1), as a NAMED SUBSET**, per side:

```go
envoy_listener_ssl_handshake           == 3
envoy_listener_ssl_no_certificate      == 3   // ⚠️ 3, NOT 0 — a completing handshake on a listener
                                              //    that sends no CertificateRequest books BOTH
envoy_listener_ssl_fail_verify_error   == 0
envoy_listener_ssl_fail_verify_no_cert == 0
envoy_listener_ssl_connection_error    == 0
```

      ⚠️ **NEVER a name-SET equality**: the reference emits **seventeen** listener-scope `ssl.*` names
      post-drive and the subject **five**; the subject's five are a strict subset (`comm -23` measured
      EMPTY). ⚠️ **EXACT equality, not a floor** — a floor cannot tell a per-connection counter from an
      over-firing one. ⚠️ **`t.Errorf` per violation, never `t.Fatalf`** — a `Fatalf` on the reference
      side makes every subject-side assertion dead code. The only `Fatalf` is the scrape itself.
- [ ] **Step 5. 🔴 DO NOT ASSERT `no_filter_chain_match` — NOT EVEN `== 0`** (§3.3). The reference emits
      it at 0; **the subject does not emit the name at all** — the string occurs exactly once in the whole
      repository outside `docs/`, as a comment in
      `internal/listener/tls_handshake_negative_test.go:25`, with **no production site**. Because
      `scrapeProm` returns the zero value for a missing key, a `== 0` pin would be **silently vacuous**
      rather than red. **Use `envoy_listener_downstream_cx_total == 3` as the liveness co-assertion** —
      present on both sides, measured **3** at N=3.
- [ ] **Step 6. ⚠️ DO NOT ASSERT ANY PRESENCE SET TAKEN BEFORE THE DRIVE** (§3.2). The reference registers
      **fourteen** listener-scope `ssl.*` names at boot and **seventeen** only after the first handshake —
      the three latecomers are the dynamic families `ssl.ciphers.<suite>`, `ssl.curves.<curve>`,
      `ssl.versions.<version>`. `AssertStats` runs at runner step 10, strictly after both Drives, so the
      landed design is safe; **this step exists so a later arm is not written pre-drive.**
- [ ] **Step 7. ⚠️ ANCHOR EVERY `ssl` MATCH** on `^listener\.[^:]*\.ssl\.` (or `^envoy_listener_ssl` on
      the prometheus side). An unanchored `grep ssl` over a plaintext listener's `/stats` reads **4 on
      the reference** (`downstream_cx_ssl_active`/`_total` pairs) and **1 on the subject**
      (`server.acce`**`ssl`**`og_dropped`) — **both witnesses appear simultaneously, one per side**
      (§3.4).
- [ ] **Step 8.** `gofmt -l` on OUTPUT; misspell sweep.

**Commit.**

---

### Task 11: Fixture `0121` — the THREE registration gates, proven and NC'd

**Files:** Modify `test/differential/runner_test.go`
**Interfaces:** Consumes Task 10's `init()`. **This is `SPEC.md` §12 roster rows 4 and 5.**

- [ ] **Step 1.** Add the blank import
      `_ "github.com/pgdad/envoy-go/test/fixtures/0121-listener-default-chain-tls/driver"`.
      ⚠️ **A MISSING BLANK IMPORT IS SILENTLY GREEN** — this is the gate that fails without a symptom.
- [ ] **Step 2. Prove all three gates with the anchored extractor**, not a hand-written grep — ⚠️ **a gate
      command can match PROSE instead of code**, so anchor on the blank-import LINE:

```sh
extract () { grep -oE '^[[:space:]]*_ "github\.com/pgdad/envoy-go/test/fixtures/[^/]+/(driver|inputs)"$' "$1" \
  | sed -E 's#.*/test/fixtures/##; s#/(driver|inputs)"$##' | sort; }
```

      Compare against `ls -d test/fixtures/*/`. **Expected after this task: dirs 123 = imports 123, both
      `comm` directions EMPTY**, split **99 `driver/` + 24 `inputs/`** (measured at this tip: 122 = 122,
      98 + 24).
- [ ] **Step 3. NC roster row 5 — NC THE EXTRACTOR ITSELF.** In a **scratch copy** of `runner_test.go`,
      rename one fixture import. **Both `comm` directions must fire while the count stays constant.**
      ⚠️ **A COUNT-ONLY CHECK IS VACUOUS** — re-proven at this tip: under a rename the import count stayed
      **122**, `comm -23` read **1** and `comm -13` read **1**.
- [ ] **Step 4. NC roster row 4 — DELETE the new blank import in a scratch copy.** The extractor's `comm`
      must fire in BOTH directions while the raw count merely drops by one. Restore.
- [ ] **Step 5.** Confirm the fixture actually runs:
      `go test ./test/differential/ -count=1 -v -run 'TestDifferential/0121-listener-default-chain-tls'`.
      ⚠️ **`-count=1` IS NOT OPTIONAL.** ⚠️ **A `-run` selector matching nothing prints
      `[no tests to run]` and EXITS 0** — assert a nonzero `=== RUN` count beside RC=0.

**Commit.**

---

### Task 12: Fixture `0121` — `expectations.yaml` and `README.md`

**Files:** Create `test/fixtures/0121-listener-default-chain-tls/{expectations.yaml,README.md}`
**Interfaces:** Consumes Tasks 9-11. Documentation only (ADR-0019) — the enforcers are the driver's per-side arm checks, the runner's `CompareBytes`, and `AssertStats`.

- [ ] **Step 1. `expectations.yaml`** — state THE PROPOSITION in one paragraph: *both sides register the
      five `ssl.*` names on a listener whose only TLS is its `default_filter_chain`, and both book
      `handshake` and `no_certificate` once per completed handshake.* Then the three-arm table with the
      measured per-side values of §3.1.
- [ ] **Step 2. Record the FIRSTS**, in this tree: **the first fixture whose listener carries NO
      `filter_chains[]` at all**; the first to assert cross-side `ssl.*` on a `default_filter_chain`; and
      the `direct_response` choice, with the fast-failing-upstream reason.
- [ ] **Step 3. Record what is deliberately NOT asserted, and why** — ⚠️ **a name in neither list reads as
      ASSERTED**, so both lists must be CLOSED enumerations. Name: `no_filter_chain_match` (§3.3 — a
      NAME-level divergence, the subject has no production site); the twelve reference-only names
      (`certificate.…expiration_unix_time_seconds`, `ciphers.*`, `curves.*`, `fail_verify_cert_hash`,
      `fail_verify_san`, the four `ocsp_staple_*`, `session_reused`, `versions.*`,
      `was_key_usage_invalid`); and the address label, which differs by construction.
- [ ] **Step 4. `README.md`** — `0120`'s structure (Why this fixture exists · What is new · Topology ·
      The arms · Design decisions and the traps behind them · What `AssertStats` pins · Cross-side
      divergences deliberately NOT asserted · Running it · Files). ⚠️ **Record the port reasoning
      explicitly** — `10127`, not `10121`, not `10126` — so the next fixture does not re-derive it.
- [ ] **Step 5. ⚠️ Do not copy `0120`'s first line.** `0120/expectations.yaml` opens *"Phase 94 fixture"*
      and is a known stale-provenance line; write `0121`'s own.

**Commit.**

---

### Task 13: Fixture `0121` — the two fixture NCs

**Files:** none committed — scratch mutations, reverted under `sha256sum -c`.
**Interfaces:** Consumes Tasks 10-12. **This is `SPEC.md` §12 roster rows 6 and 7.**

- [ ] **Step 1. Roster row 6 — DELETE `AssertStats`.** ⚠️ **NEUTRALISE, NEVER REVERT** — the package must
      still compile, so remove the method (and its `_ fixture.StatsAsserter` assertion) rather than
      breaking the file. The fixture must go **RED**, not silently green. ⚠️ **If it stays green, the
      runner's silent type assertion is not dispatching and the whole stats leg is dead** — that is a
      finding, not a nuisance.
- [ ] **Step 2. Roster row 7 — DROP ONE OF THE FIVE NAMES** from the expectation subset. Must go RED.
      This is what proves the subset is **asserted**, not merely scraped.
- [ ] **Step 3. A THIRD NC THIS PLAN ADDS: change `envoy_listener_ssl_no_certificate` from `3` to `0`.**
      Must go RED **on both sides**. ⚠️ This is the `{handshake: N, rest: 0}` map `SPEC.md` §14 item 5
      warns about, and §3.1 measured that it fails against CORRECT code. **Running it deliberately is
      what turns that warning into evidence.**
- [ ] **Step 4. ⚠️ CONFIRM *WHICH* ASSERTION FIRED** in each NC, and that the others did not. A red run is
      not evidence until the failing property is named. ⚠️ **First-divergence and fail-fast MASK later
      arms** — read the whole output.
- [ ] **Step 5. Revert every mutation and prove it:** `sha256sum` before, `git checkout -- <path>` after,
      `sha256sum -c` the capture. ⚠️ **Commit anything else pending FIRST** — `git checkout --` restores
      from HEAD. Then `git status --porcelain --untracked-files=all` EMPTY, in the worktree **and** the
      canonical root.

**No commit** (nothing lands). Record the four outcomes.

---

### Task 14: `BEHAVIOR_CONTRACT.md` — site 3 on BOTH axes, and the stat-surface ledger

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md`
**Interfaces:** Consumes Task 6.

- [ ] **Step 1. 🔴 Site 3, `:1973`, is stale on TWO axes** (§0.7). Fix both in one edit or the correction
      is a half-correction:
      **(a)** *"registers **all four counters**"* — there have been **FIVE** since phase 94
      (`ssl.connection_error`), and **`:1967`, in the same subsection, already says "All five are
      registered."** The file contradicts itself today.
      **(b)** the parenthetical *"`startQUIC` hard-errors when the chain carries no TLS config, so every
      booting QUIC listener has `tlsMode == true`"* — refuted by §0.4 of the SPEC.
      ⚠️ **THE CONCLUSION SURVIVES**: a QUIC listener's `ssl.*` counters are permanently zero and that is
      PARITY. It survives for a different reason — `Manager.Start` launches no accept loop for
      `kindQUIC`, so the Inc sites are structurally unreachable, **name-independently.** Lead with that.
- [ ] **Step 2. The stat-surface ledger — a `+0, UNCHANGED` chain entry.** The row adds **no stat NAME**;
      what it changes is **which listener shapes register the existing five**. ⚠️ **The router's claim
      that *"by its own convention at `:5130` a `+0` row earns NO chain entry"* is WRONG TWICE**: `:5130`
      is a **BLANK LINE**, and phases **44.2, 44.3, 45.2, 47.1 and 51** each carry a `+0, UNCHANGED`
      entry. A `+0` row earning an entry is established practice. The ledger is at `### Stat surface`,
      `:5071`. **Add the entry.**
- [ ] **Step 3. ⚠️ QUOTE THE STAT SURFACE AS A DELTA, NEVER AS AN ABSOLUTE.** Three different absolutes
      are live in this tree at one tip; the contract warns of itself that a mechanical re-derivation
      *"should expect the re-derived figure to disagree."* **On a contested count: NO NUMBER.**
      Enforcement is the per-phase `TestNoNewStat*` delta guards, never an absolute total.
      ⚠️ **`406` / `406 -> 407` must not be restated anywhere.**
- [ ] **Step 4.** Confirm the deliberately-LEFT siblings are untouched: `:1042` and `:1967` are **TRUE**
      and stay byte-identical.

**Commit.**

---

### Task 15: `ADR-0318` — §Decision + §Consequences, and DISARM the house guard

**Files:** Modify `docs/envoy-go/DECISIONS.md`
**Interfaces:** Consumes Tasks 6-14. **This is `SPEC.md` §12 roster row 9.**

- [ ] **Step 1. APPEND IN PLACE, after the RETAINED italic footer** — **no renumber, NO `---`
      separator.** `ADR-0318` §Context is already drafted (11 paragraphs, landed at the SPEC); this task
      adds §Decision and §Consequences only, on the ADR-0294-0317 shared-block form.
      ⚠️ **`grep -c '^---$'` must STAY 216** across this task.
- [ ] **Step 2. §Decision.** The gate is widened at the WRITE SITE, not at the registration site: the
      registration gate stays `rt.tlsMode` alone with no kind check (**ADR-0296's decision survives**),
      and `tlsMode` is now written from a predicate that covers both structural slots. Record that shape
      (B) — nil-guarding the Inc sites — is **rejected as AFFIRMATIVELY WRONG**, not merely larger: it
      would leave a TLS-serving listener reporting **zero** `ssl.*` names where the reference reports its
      full set. ⚠️ **Argue from the measurement, not from diff size.**
- [ ] **Step 3. §Consequences.** (a) the five names now register on the default-chain-only shape, TCP and
      QUIC alike; (b) the QUIC member is a REGISTRATION change with no traffic effect — `Manager.Start`
      launches no accept loop for `kindQUIC` — and that stays PARITY; (c) fixture `0121` pins the
      cross-side surface; (d) **ADR-0296 is AMENDED, not superseded**; (e) D2-QUICTS and D10-QUICSEL are
      banked with their measurements and are NOT resolved here.
- [ ] **Step 4. DISARM the house guard** — flip `ADR-0318`'s status line out of the house `PROPOSED`
      form, in place. ⚠️ **VERIFY BY LINE AND BY ADR, never by the count alone**: after the edit, resolve
      every remaining hit of `^> \*\*STATUS: PROPOSED` backward to its enclosing `## ADR-` heading, and
      confirm the `^\*\*Status:\*\* PROPOSED` decoy still resolves to `## ADR-0231` at `:14864`,
      byte-untouched. ⚠️ **NEVER gate on the unanchored `^\*\*Status:\*\*.*PROPOSED` middle-ground
      form.** ⚠️ **DO NOT WRITE A COUNT FOR EITHER GUARD FORM IN PROSE THE GREP MATCHES** — the phase-93
      SPEC falsified itself doing exactly that.
- [ ] **Step 5. NC roster row 9 — PROVE THE GUARD LIVE.** On a scratch copy of `DECISIONS.md`, append a
      house-form line; the count must move by one. ⚠️ **A ZERO ON THAT GUARD IS THE RESTING STATE, NOT
      EVIDENCE IT WORKS.**
- [ ] **Step 6. ⚠️ NO NEW ADR.** Tail STAYS `ADR-0318`; next-free STAYS `ADR-0319`;
      `grep -c '^## ADR-0319'` STAYS **0**. ⚠️ **Derive next-free from the TAIL, never from the heading
      count** — the id space is sparse at the `0209` gap, so headings+1 collides with a TAKEN id.

**Commit.**

---

### Task 16: `ROADMAP.md` — row 96 → `done`, the fixtures cell, and **SITE 10**

**Files:** Modify `docs/envoy-go/ROADMAP.md`
**Interfaces:** Consumes every prior task.

- [ ] **Step 1. ⚠️ COUNT ROW FIELDS BEFORE AND AFTER, UNDER BOTH FORMS.** Want **8 / 8** on row 96.
      Baseline at this tip: rows 94, 95, 96 all read **8 / 8**; the only malformed rows escape-aware are
      IDs **57** (NF=9) and **69** (NF=10). ⚠️ **An unescaped `|` PASSES check (1) and silently breaks
      the field count** — **reword a pipe away rather than escaping it.** ⚠️ **The escape-aware command
      is `sed 's/\\|//g' F | awk -F'|' '…'` with NO file argument to awk** — passing the file makes awk
      ignore stdin and print the NAIVE 17 under the escape-aware label.
- [ ] **Step 2.** Flip row 96 `in-progress` → `done`. **This is a FLIP, not an ADD**: `want` STAYS
      **128** and `ROADMAP.md` STAYS **246** lines.
- [ ] **Step 3. 🔴 CORRECT THE FIXTURES CELL.** Row 96 currently reads *"Row lands **+0 fixtures**, +0
      stat names, +0 BackendKinds, +0 fuzzers, +0 modules."* `SPEC.md` §6 charters `0121`, so **+0
      fixtures → +1 fixtures**. The other four stay **+0** and are correct: **+0 stat NAMES** (the five
      already exist; what changed is which shapes register them), **+0 BackendKinds** (the in-process
      `TCPEcho` branch), **+0 fuzzers**, **+0 modules**.
- [ ] **Step 4. 🔴 SITE 10 — `ROADMAP.md:136`** (§0.6). Row 74's notes cell carries the false corollary
      verbatim: *"`startQUIC` hard-errors without a TLS config, so every QUIC listener that boots has
      `tlsMode == true`."* Correct it in the same edit, **leading with what survives** (the landed gate IS
      `rt.tlsMode` alone with no kind check, and that is still right) and citing ADR-0318. ⚠️ **Fixing
      nine of ten is the phase-95 F-C failure at a tenth site.** ⚠️ **Re-count row 74's fields after the
      edit too** — it must stay NF=8 under both forms.
- [ ] **Step 5. ⚠️ NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW.** The phase-94 rule:
      the sentence asserting the sentinel could not move MOVED IT, six → seven, inside `:226`. **This rule
      is LIVE at this task.** Re-run check (2) before and after and confirm it still reads **SIX** at
      `:206 :212 :218 :228 :234 :242`, with the six per-line md5s unchanged (trailing newline INCLUDED).
- [ ] **Step 6. RE-RUN THE FULL SENTINEL + ALL FOUR NCs + THE CHECK-(2) POSITIVE CONTROL.** ⚠️ **The
      shapes CHANGE at this task**: with row 96 `done`, **check (1) goes SILENT**, and **NC-A and NC-B
      each drop from TWO lines to ONE**. Record the actual output. ⚠️ **A silent check is
      indistinguishable from a broken one — the NCs are the only evidence.**
- [ ] **Step 7.** Evaluate the termination sentinel mechanically. Check (2) will still read **SIX**, so it
      does not fire. ⚠️ **Do NOT create `stop`**, and ⚠️ **do NOT "tidy" a deferred-candidate line** —
      deleting the last one ends the project.

**Commit.**

---

### Task 17: The byte-untouched roster — asserted, set-differenced, and PROVEN LIVE

**Files:** none committed.
**Interfaces:** Consumes every prior task. **This discharges `SPEC.md` §14 item 6.**

- [ ] **Step 1. Whole-file `sha256sum` for the three safe members** — `internal/listener/quic.go` (1
      file), `internal/tls/**` (8 files), `internal/stats/**` (23 files). Correct and sufficient **because
      no §11 edit-roster row names any of them** — verified by grepping the roster block, whose single hit
      is the byte-untouched sentence itself. Baseline at this tip:
      `071cb7a536872da1605f3ef213187141b7aa19029ffd001569e4bda7845b0edb  internal/listener/quic.go`;
      reproduce the rest with `find internal/tls internal/stats -type f | sort | xargs sha256sum`.
- [ ] **Step 2. 🔴 `internal/listener/manager.go` IS ON BOTH ROSTERS — AND BOTH OBVIOUS GUARDS FAIL
      AGAINST CORRECT CODE** (§0.13). A whole-file digest is *guaranteed* to change (edit-roster rows 1
      and 2 modify the file). A line-scoped `sed -n '746,747p' | sha256sum` is ALSO unsafe, because row 2
      rewrites `:384-393` **above** line 746 and any net delta shifts the block. **Use the two-part
      literal-anchored guard:**

```sh
grep -c -F 'Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture' internal/listener/manager.go
# want 1 — a second copy would make the digest below silently wrong
grep -A1 -F 'Per ADR-0080: default_filter_chain has an INDEPENDENT TLS posture' internal/listener/manager.go | sha256sum
# want 9154e453c99515d96a5d4bd9aea17fcd9b70c39682bf4418f40e0136a8ec0817
```

- [ ] **Step 3. PROVE THE GUARD LIVE** (method note 7i) — mutate one character of the D4 block in a
      **scratch copy** and show the digest moves. ⚠️ **A guard that reads its expected value has not been
      shown to work.**
- [ ] **Step 4. SET-DIFFERENCE the two rosters and record the result:** the intersection is
      **`internal/listener/manager.go`, and it alone.** State that explicitly in `PROGRESS.md` so the
      reviewer does not have to re-derive it.
- [ ] **Step 5. ⚠️ DIFF THE ARM ROSTER, NOT THE COUNTERS** — a `+0/+0` negative-control arm can be
      deleted with every gate staying green. Compare the arm names of
      `TestListenerMetrics_GateMatchesInc` before and after: **(a)(b)(c) → (a)(b)(c)(d)(e)(f)**, six, none
      removed. Treat a shrinking roster as a finding even when every pin passes.

**No commit** (nothing lands). Record the digests and the set difference.

---

### Task 18: The six-gate verification sweep — NAME DEPARTURES, DO NOT CLAIM COMPLIANCE

**Files:** none committed.
**Interfaces:** Consumes every prior task.

- [ ] **Step 1. (a) DIFFERENTIAL — `123/123`** (122 today, `+1` from `0121`).
      ⚠️ **`-count=1` IS NOT OPTIONAL**; the suite's failure mode is a SILENT PASS. ⚠️ **ASSERT THE
      FIXTURE SET BY NAME, IN BOTH DIRECTIONS** with the Task 11 extractor. ⚠️ **`-race` on the
      differential suite is VACUOUS** — the subject is an unraced subprocess. The full suite takes ~400s.
      ⚠️ **Check for sibling sessions before blaming this row for a port flake**, and **never tear down a
      container this session did not create** — BY NAME only, and leave any `reaper_*` Ryuk alone.
      ⚠️ **The known startup flake has TWO reserved bands; an in-band recurrence is a FINDING.**
- [ ] **Step 2. (b) NON-DOCKER SWEEP — 235 packages**, gated on **`PIPESTATUS[0]`** plus a **SET
      RECONCILIATION**, not a count: `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`
      (`go list ./...` reads **237**). ⚠️ **`rc=$?` after a pipe returns the LAST command's status.**
      ⚠️ **`INNER_EXIT` does not exist in this repo.** ⚠️ **Run the FULL `internal/listener` package with
      `-race`** — a background mutator is only caught by the full package.
- [ ] **Step 3. (c) h2spec — `95 tests, 94 passed, 1 skipped, 0 failed`** (the skip is 6.9.2/2,
      invariant). ⚠️ **Use dotted selectors**; nine slash-form selectors are silently no-ops.
      ⚠️ **The REFERENCE h2spec section-8 flip is a registered flake.**
- [ ] **Step 4. (d) FUZZERS — `56 / 48`.** Reconcile against `^func Fuzz` before quoting; ⚠️ an `f.Add`
      seed is not a fuzzer, and the count went stale INSIDE row 92's own commit.
- [ ] **Step 5. (e) THE ANCHORED PANIC GATE — `^panic:|DATA RACE|SIGSEGV`, `0`, AND PROVEN LIVE.**
      ⚠️ **A gate that reads 0 has not been shown to work** — Task 1 Step 2 proved it live; re-prove it
      here at the post-fix tip, because the fix is precisely what removes the abort.
- [ ] **Step 6. (f) NO `REVIEW.md` — the STANDING DEPARTURE.** Name it; do not claim compliance.
- [ ] **Step 7. Re-run the flake register's live members before blaming this row for anything red**:
      the two SDS dial-budget flakes plus `TestSDSEndToEnd_FetchFailure_BootFailsClosed` · the
      driver-owned receiver port race · `internal/httpclient` zero-value · the two 84.2-era flakes ·
      `TestOutlierDetector_ConcurrentEjectExactlyOnce` · `0061-lb-ring-hash`'s σ-margin ·
      `TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure` ·
      `internal/filter/hcm/h2 TestFramer_ReaderGoroutineDoesNotLeak`.
      ⚠️ **`TestServerConn_TinyWindowDelivery` IS NOT A FLAKE** — a recurrence is a REGRESSION of row 91.
      ⚠️ **A GREEN RERUN CLEARS NOTHING.**

**No commit** (nothing lands). Quote every command's OUTPUT into `PROGRESS.md` at Task 19.

---

### Task 19: `PROGRESS.md`, `STATE.md`, `STATE_HISTORY.md`, `next-prompt.txt` — the close

**Files:** Create `docs/envoy-go/phases/96-listener-default-chain-tlsmode/PROGRESS.md`; modify `docs/envoy-go/STATE.md`, `docs/envoy-go/STATE_HISTORY.md`, `next-prompt.txt`
**Interfaces:** Consumes every prior task.

- [ ] **Step 1. `PROGRESS.md`** — quote all command OUTPUTS from Tasks 17 and 18 verbatim (⚠️ **quoting
      is not executing; these must be real captures**), the NC roster outcomes from Tasks 6, 11, 13 and
      15, and the byte-untouched set difference from Task 17 Step 4.
- [ ] **Step 2. `STATE.md` rolled IN PLACE** — ⚠️ **EDIT §Current pointer IN PLACE; never prepend a new
      block above it.** Advance `lifecycle-state` to **DONE**, set `next-skill:`, and evict the oldest
      §Recent entry. ⚠️ **READ THE DATES *AND* THE POSITIONS** — ⚠️ **and do NOT inherit this PLAN's
      eviction.** At the PRE-roll tip the four-way tie sat at the HEAD and the unique oldest was the tail,
      so date and position AGREED. ⚠️ **AFTER THIS PLAN'S OWN ROLL THEY NO LONGER CAN: all FIVE §Recent
      entries share the date `2026-09-07` — a TOTAL tie — because the PLAN's own entry is dated
      `2026-09-08` and lives in §Current, not §Recent.** MEASURED at this close, not predicted.
      ⚠️ **A direct date read cannot pick an evictee at the IMPL's tip at all; fall back to LIST POSITION
      — the tail — never to a guess** (method note 26). Re-measure anyway: the shape has now been the
      MIDDLE, the OLDEST POSITION, ABSENT, a THREE-WAY head tie, a FOUR-WAY head tie, and a FIVE-WAY
      TOTAL tie across seven consecutive closes.
      ⚠️ **THE BARE FORMS ANSWER NOTHING** — strict and naive both read FIVE on `STATE.md` and both are
      invariant under which entry is evicted. **Use the LABEL-BOUND PAIR across BOTH files**: the
      evictee's own label moves `STATE.md` **1 → 0** and `STATE_HISTORY.md` **0 → 1**, with a
      fabricated-label NC (0 in both) and a positive control on a sibling label that IS present.
- [ ] **Step 3. ROLL THE §Recent PREAMBLE SENTENCE TOO, without spelling the evictee's label** — a
      preamble that names the evictee makes the eviction check match its own prose.
- [ ] **Step 4. `STATE_HISTORY.md`** — ONE INLINE LINE in the **PARENTHETICAL** form. ⚠️ **Strict guard
      DELTA 0** (it reads **163** at this tip and a correctly-shaped parenthetical append does not move
      it); raw line delta **+2**, not +1 — a blank line PLUS the entry line.
      ⚠️ **NAME NO POSITIVE-CONTROL FIGURE IN THE ARCHIVE LINE** — the archive's positive controls are
      self-incrementing, and a control figure recorded in the file it measures is invalidated by the act
      of recording it.
- [ ] **Step 5. ⚠️ RE-DERIVE EVERY LINE COUNT YOU QUOTE IN `STATE.md` IN THE SAME COMMIT AS THE EDIT THAT
      MOVES IT** — §0.1 is this stage's demonstration of what happens otherwise, and §0.4 records that a
      prior PLAN needed a CORRECTION commit for exactly this.
- [ ] **Step 6. `next-prompt.txt` rolled** — ⚠️ **`git add -f`**; it is TRACKED but gitignored.
      ⚠️ **In the CONTROLLER shell `grep` is a function resolving to ugrep, which honours `.gitignore` and
      is therefore BLIND to `next-prompt.txt`**; in a SUBAGENT shell it is GNU `/usr/bin/grep` and sees it
      fine. **Name the shell as well as the tool.** ⚠️ `git check-ignore` reassures you wrongly — git
      exempts TRACKED files.
- [ ] **Step 7. ONE SQUASHED COMMIT, merged and pushed.** ⚠️ **If a late measurement invalidates something
      already pushed, land a CLEARLY-LABELLED correction commit WITH THE FULL SLUG IN ITS SUBJECT** —
      never force-push, never leave it standing. ⚠️ **The commit email must be the REPO's** — never
      `-c user.email`.
- [ ] **Step 8.** `git worktree list` must show only `master` — ⚠️ **no `phase-*`, `wt-*` or stage
      worktree may outlive its stage close.**

**Commit, merge, push.**

---

## 6. The negative-control roster, CORRECTED

`SPEC.md` §12 carries nine rows. All nine survive; **three are re-specified and three are added.** Every
control must be shown to FIRE — ⚠️ **a control that leaves its target green is not evidence that control
does any work.**

| # | control | must do | task | status vs `SPEC.md` §12 |
|---|---|---|---|---|
| 1 | revert the predicate, run the §5.2 shape-A pin | binary ABORTS; anchored gate non-zero; trace names **`manager.go:1393`** | T2, re-run T6 | ⚠️ **re-specified** — §3.5 pins the crash LINE, and it is the SUCCESS-path Inc, not `:1376`/`:1378` |
| 2 | revert the predicate, run the shape-B pin | binary ABORTS — **proves `len(chains) == 0` is not the boundary** | T3, re-run T6 | unchanged |
| 3 | revert the predicate, run arm (f) | five pointers nil on the QUIC shape, **REGISTRATION failure, NO crash** | T4, re-run T6 | ⚠️ **re-specified** — the arm MUST call `mgr.Start(ctx)` or it is red regardless of the fix and the control fires for the wrong reason (§0.9) |
| 4 | delete the blank import from `runner_test.go` | extractor `comm` fires in BOTH directions while the count merely drops | T11 | unchanged |
| 5 | rename one fixture import in a scratch copy | both `comm` directions fire; **the count-only form stays vacuous** | T11 | unchanged — **re-proven at this tip**: count 122 → 122, `comm -23` 1, `comm -13` 1 |
| 6 | delete `0121`'s `AssertStats` | the fixture goes **RED**, not silently green | T13 | ⚠️ **re-specified** — NEUTRALISE (remove the method AND its `_ fixture.StatsAsserter` assertion), never break the build; a green here means the runner's silent type assertion is not dispatching, which is a FINDING |
| 7 | drop one of the five names from the expectation subset | RED — proves the subset is asserted, not scraped | T13 | unchanged |
| 8 | run the anchored panic gate over a known-panicking input | non-zero, **before any zero is believed** | T1, re-run T18 | unchanged |
| 9 | the house `PROPOSED` guard on a scratch copy of `DECISIONS.md` | moves by one on an appended house-form line | T15 | unchanged — **proven live at this tip** |
| **10** | **set `envoy_listener_ssl_no_certificate` to `0` in `0121`'s map** | **RED on BOTH sides** | T13 | 🆕 **ADDED** — this is the `{handshake: N, rest: 0}` map §14 item 5 warns about, and §3.1 MEASURED that it fails against correct code. Running it deliberately turns the warning into evidence. |
| **11** | **prove the D4 literal-anchored digest live** — mutate one character in a scratch copy | the digest MOVES | T17 | 🆕 **ADDED** — §0.13: both obvious guards (whole-file, line-scoped) fail against correct code, so the replacement guard needs its own liveness proof |
| **12** | **diff the ARM ROSTER of `TestListenerMetrics_GateMatchesInc`, not its counters** | (a)(b)(c) → (a)(b)(c)(d)(e)(f), **six, none removed** | T17 | 🆕 **ADDED** — a `+0/+0` control arm can be deleted with every gate staying green ([[reference_deleted_zero_delta_control_is_invisible]]) |

⚠️ **NEUTRALISE, NEVER REVERT, FOR THE TEST-SIDE NCs** — the package must still compile, and the NC
itself must be shown to be EXECUTABLE. ⚠️ **A NC THAT LEAVES YOUR NEGATIVE CONTROL GREEN IS NOT EVIDENCE
THAT CONTROL DOES ANY WORK.** ⚠️ **CONFIRM *WHICH* ASSERTION FIRED, AND ALSO WHICH DID NOT.**

---

## 7. Counts — RE-DERIVED AT THIS PLAN'S OWN TIP (`da6ea191`)

Every figure below was produced by running its command at `da6ea191`, **the commit this PLAN is written
against and lands on top of** — not copied from `SPEC.md` §10, which is stale in eight places (§0.1).
⚠️ **A COUNT WITHOUT ITS MATCHER IS MEANINGLESS**, so each names its command or its form.

**Governing documents.** `ROADMAP.md` **246** lines / **128** data rows / row 96 at file line **158**,
`in-progress` · `DECISIONS.md` **18972**, `^---$` **216**, `^## ADR-` **317**, bare `^## ` **325**, tail
**ADR-0318**, next-free **ADR-0319** (`^## ADR-0319` reads **0**, TAIL-derived — ⚠️ **never from the
heading count**, which collides at the `0209` gap) · `BEHAVIOR_CONTRACT.md` **5989** · `STATE.md` **65** ·
`STATE_HISTORY.md` **558**, archive triple strict **163** / parenthetical **66** / loose **229**
(163 + 66 = 229 exactly, under the named anchored-occurrence forms).

**Guards.** House `PROPOSED` guard **ARMED**, its hit resolved BY BACKWARD HEADING SEARCH to
`## ADR-0318` at `:18944`; the `^\*\*Status:\*\* PROPOSED` decoy resolves to `## ADR-0231` at `:14864`,
byte-untouched; the middle-ground `^\*\*Status:\*\*.*PROPOSED` is **NEVER a gate**. Guard proven LIVE on
a scratch copy at this tip.

**Code.** `internal/listener/manager.go` **1636** · `internal/tls/config.go` **666** (⚠️ **NOT the 642
row 95 asserts twice**) · `go list ./...` **237**, **235** excluding the two Docker drivers · `go.mod`
**67** require entries under a **structural `awk`** extractor (⚠️ the character class
`^\s+[a-z0-9./-]+ v[0-9]` reads **62** — it drops exactly `AdaLogics/go-fuzz-headers`,
`Azure/go-ansiterm`, `Microsoft/go-winio`, `Microsoft/hcsshim`, `prometheus/client_model`, and the
reverse difference is EMPTY) · fuzzers **56 targets / 48 files**.

**Fixtures.** **122** via `ls -d test/fixtures/*/ | wc -l`, tail `0120-tls-connection-error`,
**`0121` FREE** (⚠️ `grep -cE '^[0-9]{4}-'` reads **120**, dropping `0007a-cors` and
`0007b-iteration-probe`; and the bash glob `test/fixtures/[0-9]{4}-*` is **not a repetition** — it
errors) · extractor **122 = 122** against `test/differential/runner_test.go`, both `comm` directions
EMPTY, split **98 `driver/` + 24 `inputs/`**, and **NC'd** (a rename fires BOTH `comm` arms while the
count stays 122) · BackendKind tail **38** (`H2GoawayResponder`, `test/differential/fixture/fixture.go:614`)
· phase dirs **137** (a PLAN adds a FILE, not a directory).

**Row shape.** `-family row` **96 occurrences / 68 lines** (⚠️ pass `--` before the pattern). Row 96
NF **8 / 8**; malformed rows escape-aware are IDs **57** (NF=9) and **69** (NF=10) only.

**Digests measured here, for the IMPL to pin:**
`internal/listener/quic.go` whole-file
`071cb7a536872da1605f3ef213187141b7aa19029ffd001569e4bda7845b0edb` · the D4 block, literal-anchored,
`9154e453c99515d96a5d4bd9aea17fcd9b70c39682bf4418f40e0136a8ec0817` with uniqueness **1**.

⚠️ **EVERY FIGURE IN THIS SECTION THAT BELONGS TO A FILE THIS PLAN DOES NOT TOUCH MUST BE UNCHANGED AT
THE CLOSE** (§1.1). The close re-measures them rather than asserting they held.

---

## 8. Cost — MEASURED and ESTIMATED, labelled separately

**MEASURED at this stage:** the production edit is exactly the SPEC's one line, **`+1 / -1`**, applied
and reverted under `sha256sum -c` by this stage's own reference-measurement agent, with the patched
binary serving three × HTTP 200 where the un-fixed one aborts.

**ESTIMATED:** §1.2's table — ≈ **+942 / -54** `.go`, ≈ **+1150** code + YAML + PEM, ≈ **+1350** on the
accounting that includes `expectations.yaml`. Against the measured structural precedent `0a985a35`
(**+1143** `.go`, **+1635** code + YAML + PEM), this row is smaller on both readings.

⚠️ **THE ESTIMATE IS A LOWER BOUND** — [[reference_measured_prototype_is_a_lower_bound]] has fired
FOURTEEN consecutive rows. §1.2 states the remedy (BOOTSTRAP §6.1's mid-execution trigger), not a
retroactive re-reading.

---

## 9. Deferred — newly surfaced by THIS PLAN, none chartered

- ⚠️ **`ROADMAP.md:136` cites `registry.go:107` for the stat-charset panic where `BEHAVIOR_CONTRACT.md:1042`
  says the correct site is `:117`.** A second, pre-existing defect in the same cell Task 16 Step 4
  edits. **Recorded, NOT fixed** — it is a different claim from this row's, and correcting it silently
  inside a `tlsMode` row would blur both. It is a one-line candidate for the next maintenance row.
- **`0120/expectations.yaml`'s *"Phase 94 fixture"* first line** — still stale, still not this row's.
  Task 12 Step 5 only ensures `0121` does not copy it.
- **D2-QUICTS** and **D10-QUICSEL** — banked with their measurements, decided OUT by `SPEC.md` §4 and §9.
- ⚠️ **The `stat_prefix` duplicate-registration panic** — two listeners sharing an HCM `stat_prefix`, a
  config the reference ACCEPTS, make envoy-go **panic rc=2** including under `-mode validate`. **The
  strongest banked candidate; it deserves its own row.**
- **D8-FCM's PGV-strictness nit** — UNMEASURED against a live reference; recorded, not chartered.
- **The other TEN fixed `ssl.*` names and FOUR dynamic families** — still blocked on NAMING (the stat
  name charset bans the hyphen, and §3.2 now names the three dynamic families the reference actually
  emits: `ssl.ciphers.*`, `ssl.curves.*`, `ssl.versions.*`).

---

## 10. `SPEC.md` §14 coverage — every owed item

| § | owed | discharged by |
|---|---|---|
| 1 | a TDD spine discharging §5, §6 and §11, task count DERIVED | §5, **19 tasks**, count derived at §1.2 |
| 2 | **EVALUATE THE §6.1 SPLIT GATE EXPLICITLY**, verdict either way | §1.2 — **EVALUATED, NOT SPLIT**, on four grounds, anchored on the MEASURED precedent `0a985a35` that §0.5 surfaced |
| 3 | re-derive every §10 count at the PLAN's own tip; re-locate anchors by LITERAL | §7 (all re-derived; **eight of §10's figures refuted**, §0.1) and §4.1 (every anchor re-located by `grep -nF`; all exact, and the banded-shift hazard named) |
| 4 | **order the tasks so §12 row 1's NC is available BEFORE the fix lands** | §5 — Tasks 1-5 land and RUN at the un-fixed tip; Task 6 is the fix; Task 6 Step 5 re-runs all three reverting NCs |
| 5 | decide `0121`'s drive count and exact expectation map, **MEASURING the leaf set on each side first** | §3.1 — **MEASURED on both sides at N=1 and N=3**; **N = 3** decided, with the reason; the passing map and the failing map both written out |
| 6 | prove the D4 block is on the byte-untouched roster; set-difference that roster against §11's edit roster | §0.13 + Task 17 — intersection is **`manager.go` alone**, and **BOTH obvious guards are shown to fail against correct code**; a working two-part guard is specified and its digest measured |
| 7 | **refute this SPEC by execution** and record it | §0 — **FIFTEEN claims**, including a **TENTH** occurrence-set carrier the SPEC missed, **eight stale figures in its own §10**, a test structure §5.1 presumes but that does not exist, an arm that would prove nothing, and a mis-attributed ADR |

---

## 11. Self-review — run against the SPEC with fresh eyes

**Spec coverage.** §1 scope → §5 Tasks 6, 4, 5 (both members of the root cause). §2 mechanism → §4.1
anchors. §3 the production edit → Task 6. §4 D2-QUICTS OUT → §9, no task. §5.1 invert + extend → Tasks 5
and 4. §5.2 the live-handshake pin → Tasks 2 and 3. §5.3 prose only → Task 7 Step 4. §5.4 what is NOT
added → honoured; no D2-QUICTS test, no D10-QUICSEL test, no `len(helpText)` guard. §6 fixture → Tasks
9-13. §7 gates → Task 18. §8 ADR-0318 → Task 15. §9 what the SPEC does not decide → §9, untouched. §10
counts → §7. §11 edit map → Tasks 5-16, **with row 6's range widened (§0.15), row 8's filing corrected
(§0.14), and row 14 gaining site 10 (§0.6)**. §12 NC roster → §6, three re-specified and three added.
§13 sentinel → §2. **No spec section is unclaimed.**

**Placeholder scan.** Run mechanically over the whole file for the `writing-plans` red-flag set.
⚠️ **THE SCAN SPELLS ITS OWN PATTERNS, SO A NAIVE RUN CONVICTS THIS PARAGRAPH** — the phase-93 SPEC
falsified itself doing exactly that, so the tokens are deliberately NOT restated here. Scoped to
`:1-1620`, i.e. everything above this section, **every pattern reads 0**; the only whole-file hits are
the ones this paragraph would itself create if it named them. Every code step carries the actual
literal, command or assertion — no step defers work to a later description.

**FOUR tasks land no bytes** — Tasks **1, 13, 17 and 18**. They are evidence tasks, they say so in
their own headers (*"none committed"*), and their commands are written out in full. ⚠️ **That is a
deliberate shape, not an omission**: the negative controls of §6 rows 1-3, 6, 7, 10 and 11 are
mutations that must be REVERTED, so a task that committed them would defeat itself.

**Type consistency.** The five counter fields are spelled `sslHandshake`, `sslFailVerifyError`,
`sslFailVerifyNoCert`, `sslNoCertificate`, `sslConnectionError` throughout (`manager.go:187-200`), and
the leaf names `handshake`, `fail_verify_error`, `fail_verify_no_cert`, `no_certificate`,
`connection_error` match `sslLeafRoster` at `manager_test.go:4725`. `buildListenerRuntimeWithCtx` is
used everywhere — **the short form does not exist** (§0.15). Helper signatures in §4.1 are copied from
the file, not paraphrased.

**One deliberate deviation, stated rather than made silently.** `SPEC.md` §5.1 asks for `t.Helper()` on
a shared assertion body. **There is no shared assertion body and no table** (§0.8), and this PLAN does
not create one: arms (d)(e)(f) are inlined to match the landed style of arms (a)(b)(c). The property
`t.Helper()` exists to protect — a distinct call-site line per failure — is delivered instead by the
landed per-property `t.Errorf` discipline, which is already what this test does (**18** `t.Error*` for
properties against **10** `t.Fatal*` for setup, counted in `:2341-2493`). **If the IMPL prefers to
refactor the three landed arms into a table, that is a bigger change than this row needs and it should
be its own decision, taken visibly.**
