# PLAN 78 — boot-panic **visibility**: release the wait, don't just move it

> **For agentic workers:** execute this plan task-by-task, red-first. Steps use checkbox (`- [ ]`) syntax. Each task re-derives its own anchors at the tip it is editing — **anchors drift within a phase's own tasks**, and *a drift CORRECTION is itself a claim* (`reference_a_drift_correction_is_itself_a_claim`).

**Goal:** make every panic that unwinds through `main()` during boot VISIBLE — it must kill the process, print the panic, and print a goroutine dump — by relocating the `<-flusherDone` shutdown defer past both `close(flusherDone)` branches **and** making `cancel()` the first statement of its body, so its wait cannot outlive an unwinding `main`.

**Architecture:** the production change is **one moved 6-line defer plus one inserted `cancel()` line** in `cmd/envoy-go/main.go`; everything else is prose and tests. Three guard arms ship: one **behavioural** (black-box, boots the binary on the single config-reachable in-window panic trigger and asserts a four-way conjunction), and two **structural** (`go/parser` over `main.go` — an ordering arm and a `cancel()`-precedence arm). The structural arms exist because the behavioural one **passes on a tree that still hangs**, which was executed, not reasoned.

**Tech Stack:** Go 1.26.5 · `cmd/envoy-go` only · `go/ast`/`go/parser`/`go/token` (stdlib, test-side) · `os/exec` + `context.WithTimeout` · the 120-fixture differential harness against `envoyproxy/envoy:contrib-v1.37.2`.

**STAGE:** PLAN (lifecycle-state **2 → 3**). **ROW 78 STAYS `in-progress`.** `ROADMAP.md` **BYTE-UNTOUCHED**; `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; `DECISIONS.md` **BYTE-UNTOUCHED** (ADR-0300 is PROPOSED and completes at the IMPL). File set: this `PLAN.md` + `PROGRESS.md` + `STATE.md` + `next-prompt.txt`.

**ADR-0045 split gate APPLIED and NOT TRIPPED.** The gate is *"> ~25 tasks OR > ~1500 LoC"*. This plan derives **10** tasks and ~330 production+test LoC (production: **7 lines added / 6 removed** of code, plus ~60 lines of comment; test: ~270). A single flat row stands. The escape valve at the *(production + prose)* / *(guards + gates)* seam stays **UNARMED**.

---

## 1. PLAN re-derivation ledger — what this stage RE-DERIVED, REFUTED, and newly EXECUTED

Every figure below was produced at this PLAN's own tip (`6905c9ed`) by execution — **four investigation agents on disjoint remits**, each with a private detached worktree, private scratch and a banded port range (23000/23100/23200/23300), plus **controller re-derivation of every load-bearing claim including an independent cross-product run** (controller band 22000). **None is carried from the SPEC.**

**The SPEC contradicts itself in one place, and the contradiction is load-bearing** (§1.1). **Three further SPEC claims are refuted** (§1.3–§1.5). **The task cost is re-derived UPWARD, 7–9 → 10** (§1.8).

### 1.1 ⚠️ THE HEADLINE — SPEC §3.1 PERMITS A FORM THAT SPEC §3.7 FORBIDS, AND THE PLAN RESOLVES IT BY RETRACTION

SPEC §3.1 records a second alternative and explicitly permits it:

> *"registering an additional `defer cancel()` **after** the relocated defer, so LIFO runs it first — is behaviourally equivalent to the pinned form and is a matter of taste; **the IMPL may take it if it reads better in situ**, and must say so."*

SPEC §3.7 specifies the shipped coverage arm as:

> *"Assert on `main.go`'s AST that the deferred `<-flusherDone` receive is **preceded, inside the same function literal**, by a call to `cancel`."*

**On alternative (ii) the `cancel` call is NOT inside that function literal** — it is a sibling `defer cancel()` statement. Two agents built it independently and both executed the consequence:

| form | behaviour (post-anchor panic + sink, SIGKILL deadline) | ordering arm | coverage arm |
|---|---|---|---|
| pinned — `cancel()` first inside the body | **EXIT=2**, 0.009–0.011 s, panic PRESENT | **PASS** | **PASS** |
| **alt (ii)** — trailing sibling `defer cancel()` | **EXIT=2**, 0.0094 s, panic PRESENT (behaviourally correct) | **PASS** | ⚠️ **FAIL** |
| alt (i) — bounded `select` + `time.After(5s)`, no `cancel()` | EXIT=2 but **burns the full 5.012 s timer on every panic path** | PASS | **FAIL** |

⇒ **the SPEC's own new guard goes RED on the SPEC's own permitted alternative.**

**RESOLUTION — the PLAN RETRACTS §3.1's permission.** The single-literal `cancel()`-first form is the **only** accepted shape, and T5's arm is what enforces it. The reasoning is not stylistic:

- Widening the arm to also accept "a `defer cancel()` registered later in the same block" makes it reason about **registration order across sibling statements** — which is precisely the reasoning that failed in SPEC §1.2 and produced the shipped-hang scenario. A guard with two accepted shapes has two ways to be satisfied wrongly.
- Alternative (ii) makes the fix a **distance** relationship: a reader must reconstruct LIFO across a ten-line gap to see why the receive is safe, and a later editor inserting any defer between them silently re-breaks it with **no local signal**. The pinned form puts the release and the wait on adjacent lines under one comment, where the invariant is locally checkable.
- Alternative (i) is refuted on its own terms by measurement, not by taste: **5.012 s of dead wall-clock on every panic path**, which is exactly the *"silent truncation of the final sink drain on a timer the SPEC would have to justify"* that §3.1 itself warns about.

⚠️ **The IMPL must NOT take alternative (ii) or (i).** If a future row deliberately switches, it must **edit T5's arm** — a visible, reviewable act — rather than have the arm silently accept a second shape. T5's doc comment says so in those words.

### 1.2 ⚠️ THE SECOND HEADLINE — A **stderr-NON-EMPTY** ASSERTION PASSES ON THE HANGING TREE. This is a THIRD counter-example in the R5 family and no phase-78 document carries it.

SPEC §1.1's cross-product table leaves stderr as `—` for the hanging cell. **Measured by the controller, and independently by agent A:** the naive-relocation tree with a post-anchor panic and a stats sink emits **564 / 705 / 846 / 987 / 1122 / 1269 bytes of stderr** while hanging — growing with wall-clock, and with the panic text **ABSENT**. Composition, executed:

```
$ sed 's/[0-9]//g' hang.err | sort | uniq -c
      4 // :: statssink: statsd udp write failed, dropping line: write udp ...:->...:: write: connection refused
```

It is the still-running flush ticker, not a diagnostic. ⇒ **a harness asserting *"stderr is non-empty ⇒ the failure was visible"* is GREEN on a tree that hangs forever having reported nothing.** SPEC R5 proves an *output-only* assertion is satisfied by a print-then-hang build and an *exit-status-only* assertion is satisfied by success; this adds a third: an *output-VOLUME* assertion is satisfied by unrelated log noise. **Only asserting the panic TEXT discriminates** — which is why T3 leg 3 names `stats: duplicate metric registration` rather than measuring bytes.

### 1.3 ⚠️ THE §3.8 PROSE DENOMINATOR IS **NINE CANDIDATE SITES, NOT SEVEN** — the SPEC misses two, and both sit in the blast zone

SPEC §3.8 states *"Seven candidate `defer`/LIFO comment sites in `main()` (one further match, `:93` … is excluded)"* ⇒ a claimed denominator of eight matches. **Re-derived by the controller** (`grep -niE 'defer|lifo' cmd/envoy-go/main.go`, comment lines only, contiguous lines grouped into sites):

| # | site | in SPEC's seven? |
|---|---|---|
| 0 | `:93` (`operator-knob deferred per ADR-0095`) | excluded — **verified unrelated, AGREE** |
| 1 | `:112-113` | yes (SURVIVE) |
| 2 | `:198` | yes (MISLEADING) — ⚠️ **reclassified, §1.4** |
| 3 | `:289-297` | yes (FALSE) |
| 4 | `:304-308` | yes (SURVIVE) |
| 5 | `:326-329` | yes (MISLEADING) |
| 6 | **`:365-367`** | ❌ **MISSED** |
| 7 | `:370` | yes (FALSE) |
| 8 | `:393` | yes (FALSE) |
| 9 | **`:396`** | ❌ **MISSED** |

**`:365-367`** is the important miss. Every clause in it stays literally true, but its causal story omits the `cancel()` — which post-move is *what makes `Start` return during an unwind*. ⚠️ **It sits PHYSICALLY ADJACENT to the relocation anchor** (the defer is inserted immediately after `:371`), so a reader who lands at the new code gets the pre-move mechanism three lines above it. After `:289-297` is rewritten it becomes `flusherDone`'s **only remaining stale comment site**. Classified **INCOMPLETE**; T2 edits it.

**`:396`** (`// Existing deferred-stop chain runs as the function unwinds.`) **SURVIVES**; T2 records it as checked so a later sweep does not "correct" a true comment — the R11 failure mode.

**Re-derived audit: 3 FALSE · 3 INCOMPLETE · 3 SURVIVE**, plus the pre-existing `:201` drift. (SPEC: 3 / 2 / 2.)

### 1.4 `:198` is RECLASSIFIED from MISLEADING to a clarification — the SPEC's stated reason does not hold

SPEC §3.8 says of `:198` — *"collected in their OWN statsSinks slice + closed via a dedicated defer"* — that *"the pointer should be re-aimed ~70 lines down."* **The sentence carries no location claim at all**, and it stays true post-move. Editing it is a *clarification*, not a correction. T2 still edits it (the pointer genuinely helps once the defer is 70 lines away) but must **not** describe it as fixing a falsity — a break test written against a "false" comment that was never false is a vacuous break.

### 1.5 ⚠️ THE BLAST RADIUS HAS A **THIRD** BUILD SITE THE SPEC NEVER NAMES

SPEC §9 (and `next-prompt.txt` §5, and `STATE.md`) present the blast radius as *"`test/differential/harness.go:240` and `:594` build and spawn `./cmd/envoy-go` for EVERY fixture."* Both cites HOLD (`:240` = `StartSubjectProxy`, the normal path; `:594` = `tryStartSubjectProxy`, the `BootRejectFixture` path). **But the set is incomplete** — controller-executed:

```
$ grep -rn '"\./cmd/envoy-go"' --include='*.go' .
test/conformance/h2spec/h2spec_test.go:210:	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, "./cmd/envoy-go")
test/differential/harness.go:240:	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")
test/differential/harness.go:594:	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")
```

**`test/conformance/h2spec/h2spec_test.go:210` (`TestH2Spec`, entry at `:30`) also compiles and spawns the changed binary**, and the project's conformance claim is *"h2spec 53/53"* (`STATE.md:39`). A shutdown-ordering change to `main()` reaches it. **T9 adds it to the gate set** rather than leaving a suite that spawns the edited binary un-run.

### 1.6 ⚠️ A GATE HAZARD NOT IN THE LINEAGE'S THIRTEEN: `golangci-lint` RUNS `misspell` IN **US LOCALE**, AND THE SPEC'S PROSE FAILS IT

Controller-read, `.golangci.yml`:

```yaml
    - misspell
linters-settings:
  misspell:
    locale: US
```

Agent C's first draft was flagged **3×** — `behavioural` ×2 → `behavioral`, `CANCELLED` → `CANCELED`. **The phase-78 SPEC uses "behavioural" throughout.** ⇒ ⚠️ **the IMPL must not paste SPEC prose verbatim into `.go` comments.** This is a new entry: **broken/hazardous-gate count 13 → 14.**

### 1.7 Other corrections this PLAN owes forward

| # | claim | source | verdict |
|---|---|---|---|
| a | *"`cmd/envoy-go` currently declares 8 test functions plus **4 helpers**"* | SPEC §5 | **REFUTED — FIVE helpers**: `waitForReadySentinels` :127, `acceptEcho` :167, `freeTCPPort` :177, `buildBinaryOrSkip` :191, `pkiFixture0002` :206 |
| b | SPEC §13 exit table: *"DECISIONS \| **17531** \| value at this close"* | SPEC §13 | **DRIFTED — the value at that close is 17560.** `17531` is the *pre*-commit value; `STATE.md:22` and `next-prompt.txt` §14 both say `17531 -> 17560` correctly. Only the SPEC's own exit table is wrong |
| c | *"`internal/filter/hcm/config.go:352-358`"* as the registration site | SPEC §3.6 | **TRUNCATES, not wrong.** `:352` is `prefix := "http." + statPrefix + "."`; the registration block is `:358-362` (five `NewCounter` calls); `:358` (`downstream_rq_total`) is the one that actually collides, confirmed by the live stack trace. T3's doc comment anchors on the **message string and the counter name** as well as the lines (`reference_stale_cite_recurs_fix_by_pattern`) |
| d | *"under a bounded-timeout harness the **healthy** negative control also exits 124, so exit-status alone is satisfied by success"* | SPEC R5 | **CORRECT but for a sharper reason than stated.** Measured through the actual harness shape (`exec.CommandContext` + `context.WithTimeout`), a healthy boot and a hung boot are **byte-identical on every status observable**: `ctx.Err()` = `context deadline exceeded`, `runErr` = `signal: killed`, `ExitCode()` = **-1**, on BOTH. They differ **only** in stdout (109 B of ready sentinels vs **0 B**). So leg 2 is insufficient through **blindness**, not through satisfaction-by-success — and `ctx.Err()` alone cannot separate them either. **T3 encodes the discrimination into leg 1's failure message** (it prints `stdout.Len()` and the first line), so the two red modes are distinguishable by a maintainer |
| e | *"the pre-existing `internal/cluster` `-race` outlier flake"* — unnamed | SPEC §9 hazards | **UNDER-SPECIFIED.** The index names the test: `TestOutlierDetector_ConcurrentEjectExactlyOnce`. Name it, so an *unrelated* `internal/cluster` `-race` failure cannot be laundered through this exemption |
| f | `reference_sds_init_fetch_timeout_dial_budget_flake` cited by slug only | SPEC §9 hazards | **UNDER-SPECIFIED, and it matters here.** The index records **TWO** packages: `internal/xds TestProvider_FetchInitialCertificate_Timeout` **and** `internal/boot TestSDSEndToEnd_FetchFailure_BootFailsClosed`. ⚠️ **`internal/boot` is the direct callee of the line this row edits** (`main.go:316`). A red `internal/boot` at T9 is far likelier to be mis-classified as a regression than a red `internal/xds` |
| g | `STATE.md` §Project counts | — | **STALE ON THREE AXES**, of which `next-prompt.txt` §16 flags only one. It says fixtures **119** (real: 120 — flagged), stat surface **1205** (lineage: **1207** — NOT flagged), DECISIONS tail **ADR-0298** (real: **ADR-0300** — NOT flagged). A session anchoring there gets the wrong tail ADR. Recorded, **not fixed** — §7 |

### 1.8 ⚠️ COST RE-DERIVED **7–9 → 10**, and the SPEC's own calibration sentence is imprecise

SPEC §9 argues: *"phase 76 … also anticipated ~5–7, and its PLAN shipped NINE tasks. The project's own most recent 5–7 anticipation under-counted by two."* **The ~5–7 was phase 76's BRAINSTORM.** Its **SPEC already re-derived to 7–9** (`SPEC.md:355`), and the PLAN shipped **9** — the *ceiling* of the SPEC band, not a miss of it.

**The load-bearing calibration is therefore the opposite shape, and it is two-for-two:**

| phase | SPEC band | PLAN shipped |
|---|---|---|
| 76 | 7–9 | **9** (ceiling) |
| 77 | 11–13 | **12** |
| **78** | **7–9** | **10 — one ABOVE the ceiling** |

Two executed findings push it past the ceiling:

1. **The floor of 7 is refuted.** It requires folding T2 into T1 and T10 into T9. §1.3 makes T2 *larger* than the SPEC scoped (nine candidate sites, not seven, across five separate regions of the file) while T1's code diff is **7 added / 6 removed lines**. Folding a five-region prose sweep into a nine-line code change means a prose regression rides in on a green *build* gate.
2. **T4 splits into T4 + T5.** The SPEC bundles both structural arms into one task. They have **different anti-vacuity fatals** and **different negative controls**, and — decisively — the ordering arm is **GREEN on the tree that still hangs** while the coverage arm is the one that catches it. Shipping the arm that closes SPEC §1.2's gap inside the same reviewer gate as the arm that *demonstrates* the gap is the failure mode this row exists to fix.

Agent A (having actually made the production change) independently estimated **8–10**; agent D (having re-derived the prose denominator and the phase-76/77 calibration) independently estimated **9–10**. **⚠️ Their agreement is not evidence** — both read the same SPEC. The band is stated as **10, with 9 defensible if T4/T5 re-merge**, and the IMPL must **re-derive it again at its own tip** (`reference_deferred_candidate_cost_restale`).

### 1.9 What this PLAN EXECUTED — the row is now largely BUILT

Not designed on paper. At this PLAN's tip the following were built and run:

- **The production change**, three times independently (agents A, B, C), each `gofmt`-clean, `go build`/`go vet`/`golangci-lint` clean, whole-package `go test ./cmd/envoy-go/ -count=1` green (**9/9** with the behavioural arm, **10/10** with both structural arms, 7.9–9.2 s).
- **A 24-cell cross-product** by agent A (`{pre, mid, post}` panic position × `{sink, no sink}` × `{baseline, naive, pinned, alt-ii}`) — **the pinned form holds on 6/6 discriminating cells, zero contradicting cells.** ⚠️ Agent A added a **mid-window** position the SPEC never tested; the baseline hangs there too (`137 / 0 B / 0 B`, sink present **and** absent), extending SPEC C5 past the `boot.Construct` trigger.
- **An independent controller cross-product** (band 22000, SIGKILL deadline, inputs validated first at `configuration OK` and byte-counted): naive+post+sink ⇒ **EXIT=137, panic ABSENT, 3/3 deterministic**; naive+post+**no sink** ⇒ EXIT=2 (defect structurally impossible — the `else` pre-closes); pinned ⇒ **EXIT=2 in 0.011 s, panic PRESENT, both sink states**. **The SIGTERM rescue reproduced**: same binary, same config ⇒ `EXIT=124` at 8.006 s **with the panic text PRESENT**.
- **The behavioural guard**, red-then-green: RED on the unmodified tree (leg 1, `stdout=0 bytes, stderr=0 bytes, signal: killed`, 31.15 s), GREEN on the fixed tree (0.97 s).
- **Both structural arms**, plus a **six-cell negative-control matrix**, plus the residual demonstration (§4).
- **All three sentinel checks**, twice (controller at session open, agent D independently), with negative controls.
- **Every gate recipe in §5**, each with an executed negative control.

**⚠️ NOT EXECUTED, and the IMPL owes each a run:** the full 120-fixture differential suite (~400–420 s per attempt) · `TestH2Spec` · the `-race` variants · the IMPL's own red-then-green (a prior stage's red is not this stage's red).

---

## 2. Global constraints

Every task's requirements implicitly include this section.

1. **The fix is NOT "move the defer."** It is *move the defer **and** make its wait unable to outlive an unwinding `main`*. `cancel()` is the **first** statement of the relocated body. ADR-0300 §Context ¶4 says so; §1.1 pins the single accepted shape.
2. **`cancel()` must come BEFORE the receive.** Placed after, it is unreachable during the hang (T5 cell 5 proves the arm catches this).
3. **US spelling in all `.go` comments** — `misspell` locale US (§1.6). Do not paste SPEC prose into code.
4. **`t.Errorf` per property; `t.Fatalf` only where later assertions would be meaningless** (`reference_fatalf_makes_assertions_unreachable`). Every structural arm carries a `found == 0` ⇒ `t.Fatalf`.
5. **A hang is detected from the DEADLINE STATE (`ctx.Err() == context.DeadlineExceeded`), never from an exit code.** A deadline-killed process reports only `signal: killed` / `ExitCode() == -1`.
6. **SIGKILL, never SIGTERM, in any harness that must observe the hang.** `exec.CommandContext`'s default cancel is `Process.Kill` — do not set `cmd.Cancel`. A SIGTERM cancels the server `ctx`, releasing the very wait that is hanging.
7. **Never assert stderr VOLUME** (§1.2). Assert the panic TEXT.
8. **`ROADMAP.md` row 78 stays `in-progress` until T10.** `want` **STAYS 110** — this row adds no ROADMAP row.
9. **`ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, `DECISIONS.md` are BYTE-UNTOUCHED at this PLAN.** They are edited only at the IMPL, by T7/T8/T10.
10. **`main.go:304-308` and `:112-113` and `:396` stay BYTE-UNTOUCHED** — they are true (R11, §1.3). **`internal/stats/**` stays BYTE-UNTOUCHED** — this row makes panics visible, it does not re-litigate which should exist.
11. **`ADR-0089` stays BYTE-UNTOUCHED** — this row lands neither `/runtime` nor `POST /runtime_modify`.
12. **The new `BEHAVIOR_CONTRACT` subsection must not claim boot failures are never silent** (ADR-0300 §Context ¶11) and **must not inherit the non-existent import path** `github.com/esalaine/envoy-go/validate`.
13. **`-count=1` on every `go test`.** Confirm every `-run` selector matched — a no-match prints `[no tests to run]` and **exits 0**.
14. Per-task `gofmt` + `golangci-lint` on touched packages; **gate `gofmt` on OUTPUT, never exit code** — it never exits non-zero.
15. Fresh worktree off master; subagent-driven; subagents commit **locally only**, never push; controller squash-pushes at close. Use `git -C <abs-path>` for every git command.

---

## 3. File structure

**Production — ONE file:**

| file | change |
|---|---|
| `cmd/envoy-go/main.go` | the `:298-303` defer relocated below both `close(flusherDone)` branches **with `cancel()` first in the body**; the `:289-297` comment block rewritten and moved with it; five further prose sites |

⚠️ **Blast radius is mechanically one file** — controller-confirmed: `grep -rn 'flusherDone' --include='*.go'` ⇒ **9 hits, all in `cmd/envoy-go/main.go`** (4 code: `:205` declare, `:299` receive, `:368`/`:370` close; 5 comment: `:290 :292 :294 :365 :366`). The tree's other 16 `package main` files are fixture backends and PKI generators; none carries the pattern. ⚠️ `cmd/envoy-go/main_test.go` is **`package main`**, not `package main_test`.

**Test — ONE file:**

| file | change |
|---|---|
| `cmd/envoy-go/main_test.go` | 3 new tests + 3 new helpers + 1 config renderer; imports gain `go/ast`, `go/parser`, `go/token` (stdlib) |

Identifiers, **all collision-checked at this tip** (`git grep -c` ⇒ 0 for each; negative control `TestEnvoyGoBinary_ModeValidate` ⇒ 2):
`TestMain_BootPanicIsVisible` · `TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose` · `TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel` · `bootPanicVisibleDeadline` · `bootPanicTriggerYAML` · `firstLineOf` · `bootPanicVisibilityMainGo` · `isFlusherDoneReceive` · `isFlusherDoneClose`.
⚠️ **Re-run the collision check at the IMPL tip** (`reference_spec_drafted_identifier_collision_check`) — the package currently declares **8 test functions and FIVE helpers** (§1.7a).

**Docs (IMPL only):** `BEHAVIOR_CONTRACT.md` (T7) · `DECISIONS.md` ADR-0300 completion (T8) · `ROADMAP.md` row 78 (T10) · `STATE.md` · `next-prompt.txt`.

---

## Task 1 — the production change: relocate **and release**

**Files:** Modify `cmd/envoy-go/main.go` (delete `:289-303`; insert a 26-line block after the `close(flusherDone)` if/else, currently closing at `:371`).

**Interfaces:**
- Consumes: `cancel` (declared `main.go:339`, `ctx, cancel := signal.NotifyContext(...)`) — **in scope at the relocation anchor**, confirmed by build.
- Produces: a `main.go` whose deferred `<-flusherDone` receive sits below every `close(flusherDone)` and is preceded inside its own function literal by `cancel()`. T4 and T5 assert exactly this.

- [ ] **Step 1: Re-derive the anchors at YOUR tip.** All 23 SPEC-cited line numbers were CONFIRMED at `6905c9ed` with zero drift, but this task edits the file, so later tasks' anchors move. Record:

```sh
grep -n 'flusherDone\|defer \|Stats.Freeze()\|signal.NotifyContext\|boot.Construct' cmd/envoy-go/main.go
```
Expected at `6905c9ed`: declare `:205` · receive `:299` · closes `:368`/`:370` · `defer cancel()` `:340` · block `:298-303` · comment `:289-297`.

- [ ] **Step 2: Delete the old block.** Remove lines `289-303` inclusive — the 9-line comment **and** the 6-line defer. Nothing replaces them in situ.

- [ ] **Step 3: Insert the new block** immediately after the `close(flusherDone)` if/else closing brace (`:371` pre-edit; the line after `\t}` following `close(flusherDone) // no flusher: …`), before the blank line preceding `// Per-listener ready sentinels`:

```go
	// LIFO: registered LAST in main(), so this runs FIRST in the shutdown drain —
	// ahead of lm.Stop(), cancel(), admSrv.Close() and tracingProvider.CloseAll(). It
	// is registered HERE, below BOTH close(flusherDone) branches, so that a panic
	// unwinding through main() earlier in boot can never arm a receive on a channel
	// that nothing has yet been started to close (phase 78 / ADR-0300).
	//
	// Because it now runs BEFORE the deferred cancel() rather than after it, the body
	// must cancel the server ctx ITSELF — that is what the leading cancel() is for.
	// context.CancelFunc is idempotent, so on the normal <-ctx.Done() shutdown path it
	// is a no-op; on the panic path it is the ONLY thing that can release the wait,
	// and without it the process hangs forever having printed nothing.
	//
	// The sink contract (no Submit after Close) is unchanged and the mechanism is the
	// same, only self-driven: cancel() fires ctx.Done(), the Flusher's Start loop
	// finishes any in-progress flushOnce (its last Submit goes to the still-OPEN
	// channel) then returns and closes flusherDone, and only THEN do we Close() the
	// sinks (close their channels). This enforces the sink contract and prevents a
	// flush-tick / Close race from sending on a closed channel. Each Close() drains
	// the in-flight stream (CloseAndRecv) + closes the gRPC conn.
	defer func() {
		cancel()      // release the wait: a panic must never leave this blocked on a flusher
		<-flusherDone // wait for the Flusher goroutine to stop Submitting before closing the sink channels
		for _, s := range statsSinks {
			_ = s.Close()
		}
	}()
```

⚠️ **The comment deliberately carries NO `main.go:NNN` cite** — it anchors on symbols (`lm.Stop()`, `close(flusherDone)`) so it cannot go stale by line drift (`reference_stale_cite_recurs_fix_by_pattern`).
⚠️ **US spelling only** (§1.6).
⚠️ **Do NOT take SPEC §3.1's alternative (ii) or (i)** — §1.1.

- [ ] **Step 4: Gate the edit.**

```sh
[ -z "$(gofmt -l cmd/envoy-go/main.go)" ] || { echo "GOFMT FAIL"; gofmt -l cmd/envoy-go/main.go; exit 1; }
go build ./cmd/envoy-go/ && go vet ./cmd/envoy-go/ && golangci-lint run ./cmd/envoy-go/...
```
Expected: all silent, exit 0. ⚠️ `gofmt -l` **never** exits non-zero — the `[ -z … ]` form is the gate.

- [ ] **Step 5: Prove the fix BY EXECUTION, on the cell that refuted the SPEC's predecessor.** Do not rely on this PLAN's run.

Build a probe config with **≥1 cluster** (a `clusters: []` bootstrap dies at `cluster: zero clusters in bootstrap` — a broken arm, not a result), **a stats sink** (so `statsFlusher != nil`), and one listener; validate the input FIRST (`--mode validate` ⇒ `configuration OK`) and record its byte count. Then inject `panic("t1: post-anchor probe")` immediately after the relocated defer and run:

```sh
timeout -s KILL 8 ./envoy-go -c probe.yaml >o.out 2>o.err; echo "EXIT=$?"
echo "stdout=$(stat -c%s o.out)B stderr=$(stat -c%s o.err)B"
grep -qF 't1: post-anchor probe' o.err && echo PANIC_PRESENT || echo PANIC_ABSENT
```

Expected on the fixed tree: **`EXIT=2`, ≲0.02 s, `PANIC_PRESENT`.**
⚠️ **SIGKILL, not SIGTERM** — measured on the *broken* tree, `timeout -s TERM 8` gives `EXIT=124` at 8.006 s **with the panic text PRESENT** (the signal cancels `ctx`, releases the wait, and lets it finally print). It reads as "printed, just slow."
⚠️ Also run the **control that must NOT be mistaken for a result**: the same probe with **no** stats sink. It exits 2 on *every* tree including the broken one — the `else` branch closes `flusherDone` before the defer is registered, so the defect is **structurally impossible** in that shape (`reference_probe_input_is_a_claim`). Remove the injected panic before Step 7.

- [ ] **Step 6: Prove the NORMAL shutdown path is byte-identical.** Boot on a config with a **live** UDP statsd receiver, wait for `envoy-go ready`, run ≥2 s (≈8 flush ticks at `0.25s`), `kill -TERM`, and compare exit code / stdout / stderr / datagram count against the same arms on `git stash`-ed baseline. Executed at this PLAN across four trees × 3 reps: **12/12 identical**, `EXIT=0`, **152 datagrams / 5888 bytes** on every arm, stdout and stderr byte-identical by `diff`. ⚠️ **Measure the receiver counts** — a green with zero datagrams is an empty-input artifact.

- [ ] **Step 7: Commit.**

```sh
git add cmd/envoy-go/main.go
git commit -m "phase 78 T1: relocate the flusherDone shutdown defer below both close() branches AND cancel() first in its body — relocation alone MOVES the hang rather than removing it"
```

---

## Task 2 — the prose sweep: **nine** candidate sites, three of them SURVIVE untouched

**Files:** Modify `cmd/envoy-go/main.go` (five edits; four sites recorded as checked-and-unchanged).

**Interfaces:** Consumes T1's tree. Produces no symbol.

⚠️ **This task is separate from T1 deliberately** (§1.8). T1 gates on *build + execution*; T2 gates on *per-site grep*. Folded together, a prose regression rides in on a green build gate.

- [ ] **Step 1: Re-derive the denominator at YOUR tip** (T1 moved lines):

```sh
grep -nE '^[[:space:]]*//' cmd/envoy-go/main.go | grep -iE 'defer|lifo'
```
At `6905c9ed` this returns 13 comment lines grouping into **9 candidate sites + `:93`** (§1.3). Verify you find the same set post-T1, with shifted numbers.

- [ ] **Step 2: Edit site `:198`** — a **clarification, not a correction** (§1.4). The sentence was never false; it simply now points 70 lines away.

```go
	// collected in their OWN statsSinks slice + closed via a dedicated defer that
	// is registered BELOW, after both close(flusherDone) branches (phase 78).
```

- [ ] **Step 3: Edit the pre-existing `:201` drift.** Current text says *"when **both** config slices are empty"*; the guard immediately below tests **five** (`StatsSinkConfigs`, `StatsdSinkConfigs`, `DogStatsdSinkConfigs`, `GraphiteStatsdSinkConfigs`, `OTLPSinkConfigs`) — controller-confirmed by `grep -o 'len(bs\.[A-Za-z]*)'`.

```go
	// (D-MS-FLUSH-INERT: when ALL FIVE stats-sink config slices tested below are
	// empty, statsFlusher stays nil and NO flush goroutine starts — byte-stability).
```
⚠️ It says *"tested below"* rather than naming a line, so a future sixth sink kind drifts only the word "FIVE" — visible against the adjacent `if`.

- [ ] **Step 4: Edit site `:326-329`** — MISLEADING. It gains a fourth entry ahead of all three, and item 3's *"shuts listeners after admin"* is already backwards in **execution** order.

```go
	// §12 #3). Defers are LIFO, so registration order is the REVERSE of execution
	// order. Post-phase-78 the full main() defer chain EXECUTES in this order:
	//   1. the stats-sink close — registered LAST, below the close(flusherDone)
	//      branches; cancels ctx, waits for the Flusher, closes the stats sinks
	//   2. lm.Stop()                  — stops the listeners
	//   3. cancel()                   — idempotent; already called by (1)
	//   4. admSrv.Close()             — closes the admin server
	//   5. tracingProvider.CloseAll() — flushes and stops the trace exporters
	//   6. the access-log sinks close — flushes access logs LAST
```

- [ ] **Step 5: Edit site `:365-367` — THE SITE THE SPEC MISSED** (§1.3), and the one physically adjacent to T1's new code:

```go
		// close(flusherDone) when Start returns so the shutdown sink-close defer
		// (which waits on <-flusherDone) closes the sink channels only AFTER the
		// flush loop has stopped Submitting (no send-on-closed-channel race).
		// Start returns when ctx is canceled — on the normal path by the signal,
		// and on a panic-unwind path by that defer's OWN leading cancel().
```

- [ ] **Step 6: Edit site `:370`** — FALSE post-move (the defer is registered *after* this line; nothing is blocked yet):

```go
		close(flusherDone) // no flusher: pre-closed so the defer below never blocks
```

- [ ] **Step 7: Edit site `:393`** — FALSE. ⚠️ **State which reading you are correcting.** The *relative* order of the three named elements is unchanged; what breaks is that the chain no longer *starts* with `lm.Stop`.

```go
	// before the deferred-stop chain runs (LIFO EXECUTION order: stats-sink close,
	// lm.Stop, cancel, admSrv.Close, tracing close, access-log sinks close).
```

- [ ] **Step 8: Record the four sites deliberately NOT touched**, in `PROGRESS.md`, with the reason each survives — `:93` (unrelated use of "deferred"), `:112-113` (access-log defer is rank 6, `lm.Stop` rank 2 — both clauses hold), `:304-308` (**R11**: tracing still runs after `lm.Stop()` and before the access-log sinks close — both clauses hold), `:396` (generic and true). ⚠️ **A prose sweep driven by a brief would have "corrected" `:304-308`, which is true** — that is why the SURVIVE list is written down rather than left implicit.

- [ ] **Step 9: Verify per site, and verify the untouched ones byte-for-byte.**

```sh
gofmt -l cmd/envoy-go/main.go            # must print NOTHING
go build ./cmd/envoy-go/
git diff -U0 cmd/envoy-go/main.go | grep -cE '^[-+].*(Phase 46\.1b|no new spans|sinks per ADR-0069|Existing deferred-stop)'
```
Expected: `0` — the SURVIVE sites appear in no diff hunk.

- [ ] **Step 10: Commit.**

```sh
git add cmd/envoy-go/main.go
git commit -m "phase 78 T2: the prose sweep — 3 FALSE + 3 INCOMPLETE sites rewritten (including :365-367, which the SPEC's denominator missed), 3 SURVIVE recorded untouched"
```

---

## Task 3 — the behavioural guard, and the conjunction that three counter-examples force

**Files:** Modify `cmd/envoy-go/main_test.go` (append; no import changes needed — `bytes`, `context`, `errors`, `fmt`, `os`, `os/exec`, `path/filepath`, `strings`, `testing`, `time` are all already imported).

**Interfaces:**
- Consumes: `buildBinaryOrSkip(t) string` (`:191`), `freeTCPPort(t) int` (`:177`).
- Produces: `bootPanicVisibleDeadline`, `bootPanicTriggerYAML(...) string`, `firstLineOf(string) string`, `TestMain_BootPanicIsVisible`.

⚠️ **KNOW WHAT THIS ARM DOES NOT COVER, BEFORE WRITING IT.** EXECUTED at this PLAN: `TestMain_BootPanicIsVisible` **PASSES on the naive-relocation tree — the tree that still hangs** (`--- PASS … (1.02s)`). Its trigger panics **pre**-anchor, which the naive relocation genuinely fixes, and per SPEC R3/R8 **there is no config-reachable post-anchor panic** to drive a behavioural probe from. ⇒ **T5's structural arm is load-bearing, not belt-and-braces**, and this task's red-then-green is **not** evidence that the fix is complete.

- [ ] **Step 1: Collision-check, then write the failing test.** `git grep -c` each of `TestMain_BootPanicIsVisible`, `bootPanicVisibleDeadline`, `bootPanicTriggerYAML`, `firstLineOf` ⇒ expect **0**. Confirm `grep -c DeadlineExceeded cmd/envoy-go/main_test.go` ⇒ **0** (the idiom is introduced here; negative control: `grep -c buildBinaryOrSkip` ⇒ 7).

Append:

```go
// bootPanicVisibleDeadline bounds the trigger process. MEASURED: on a tree
// carrying the phase-78 fix the binary panics and dies in 0.009-0.011 s wall
// (5 consecutive runs), so this is ~2700x headroom and exists only to bound a
// REGRESSION, never a healthy run. On the pre-fix tree the same process runs
// until the deadline kills it, having printed ZERO bytes.
const bootPanicVisibleDeadline = 30 * time.Second

// bootPanicTriggerYAML renders the ONLY config-reachable in-window boot panic:
// two HTTP connection managers on DISTINCT listener addresses sharing one
// stat_prefix, so the second chain's `http.<prefix>.downstream_rq_total`
// counter collides in the stats registry.
//
// Every element is load-bearing and a probe's INPUT is a claim:
//   - DISTINCT addresses: two listeners on the SAME address die at
//     `bind: address already in use` (exit 1) BEFORE registration runs, because
//     registerListenerMetrics runs post-bind.
//   - >=1 cluster: a `clusters: []` bootstrap dies at
//     `cluster: zero clusters in bootstrap`, which is a BROKEN ARM, not a result.
//   - a stats_sinks[] entry: it makes statsFlusher non-nil so the flusher
//     goroutine and the `flusherDone` wait actually exist. The statsd sink is UDP
//     and needs no live receiver.
//   - an http_filters[] router: the HCM rejects a chain with zero http filters
//     before it ever registers a counter.
func bootPanicTriggerYAML(adminPort, listenerAPort, listenerBPort, statsdPort, backendPort int, statPrefixA, statPrefixB string) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.StatsdSink
      address:
        socket_address: { protocol: UDP, address: 127.0.0.1, port_value: %d }
      prefix: p78
stats_flush_interval: 0.5s
static_resources:
  listeners:
    - name: l_a
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  name: r_a
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response: { status: 200, body: { inline_string: "a" } }
    - name: l_b
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: %s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  name: r_b
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          direct_response: { status: 200, body: { inline_string: "b" } }
  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, adminPort, statsdPort, listenerAPort, statPrefixA, listenerBPort, statPrefixB, backendPort)
}

// TestMain_BootPanicIsVisible is the phase-78 black-box guard (ADR-0300): a
// panic unwinding through main() during boot must be VISIBLE — it must kill the
// process, print the panic and print a goroutine dump. Before phase 78 the
// shutdown defer that waits on `<-flusherDone` was registered at the TOP of a
// 68-line boot window while the channel was only closed at the BOTTOM, so any
// in-window panic unwound into a permanent block and the process HUNG with ZERO
// bytes on both streams.
//
// THE ASSERTION IS A CONJUNCTION, and each half has an EXECUTED counter-example
// (SPEC 78 R5, PLAN 78 SS1.2) — do not weaken any of it:
//   - exit status alone is BLIND: through this exact harness shape a HEALTHY
//     boot and a HUNG boot are byte-identical on every status observable —
//     ctx.Err() is DeadlineExceeded, the run error is `signal: killed` and
//     ExitCode() is -1 on BOTH. They differ only in OUTPUT.
//   - output alone is satisfied by a PRINT-THEN-HANG build: recover(), print the
//     exact panic text, then block forever. Every string assertion passes while
//     the process never dies.
//   - output VOLUME is satisfied by NOISE: on the broken tree the still-running
//     flush ticker writes 500-1300 bytes of `statsd udp write failed` lines to
//     stderr while hanging. Assert the panic TEXT, never a byte count.
//
// So the hang is detected from the DEADLINE STATE (ctx.Err() ==
// context.DeadlineExceeded), NOT from an exit code. exec.CommandContext's default
// cancel action is Process.Kill (SIGKILL) and MUST NOT be changed to SIGTERM: a
// SIGTERM cancels the server ctx, which releases the very wait that is hanging,
// so a SIGTERM-based harness cannot falsify this contract at all — it reads a
// hang as "printed, just slow" (measured: exit 124 at 8.006 s WITH the panic
// text present).
//
// TRIGGER DEPENDENCY — this test's trigger is the ONLY config-reachable
// in-window panic in the tree, and there is NO fallback. It reaches the panic at
// internal/stats/registry.go:107 ("stats: duplicate metric registration")
// through the HCM per-filter counter registration in internal/filter/hcm/config.go
// (prefix derivation `prefix := "http." + statPrefix + "."` at :352; the five
// `registry.NewCounter(prefix + ...)` calls at :358-362, of which
// `downstream_rq_total` at :358 is the one that collides).
// If a future row makes duplicate registration a get-or-create (reference
// parity) or a clean config reject, this test goes RED — that is intended.
// RE-POINT THE TRIGGER; DO NOT RELAX THE ASSERTION. A guard that stops
// triggering must fail loudly, never pass vacuously.
func TestMain_BootPanicIsVisible(t *testing.T) {
	bin := buildBinaryOrSkip(t)

	cfgPath := filepath.Join(t.TempDir(), "boot-panic-trigger.yaml")
	// The SAME stat_prefix on both chains is the trigger. Distinct addresses.
	cfg := bootPanicTriggerYAML(
		freeTCPPort(t), freeTCPPort(t), freeTCPPort(t), freeTCPPort(t), freeTCPPort(t),
		"dup_prefix", "dup_prefix",
	)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), bootPanicVisibleDeadline)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-c", cfgPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	// LEG 1 — THE PROCESS DID NOT HANG. Sole t.Fatalf in this test: after a hang
	// there is no output left to assert on, and every later leg would report a
	// second, derived failure for one defect.
	if ctx.Err() == context.DeadlineExceeded {
		// stdout is the discriminator between the two ways this leg goes red:
		// ZERO bytes means the phase-78 defect is back (a panic swallowed by a
		// blocking defer in main()'s boot window — see the <-flusherDone wait in
		// cmd/envoy-go/main.go); the `envoy-go ... ready` sentinels mean the
		// process booted HEALTHILY, i.e. the trigger stopped triggering and must
		// be RE-POINTED (see this test's doc comment).
		t.Fatalf("BOOT DID NOT TERMINATE within %v: stdout=%d bytes %q, stderr=%d bytes, run error=%v; "+
			"zero stdout => a boot-window panic is being swallowed by a blocking defer; "+
			"a ready sentinel on stdout => the trigger no longer panics and must be re-pointed",
			bootPanicVisibleDeadline, stdout.Len(), firstLineOf(stdout.String()), stderr.Len(), runErr)
	}

	// LEG 2 — Go's unrecovered-panic exit status is exactly 2. t.Errorf, not
	// Fatalf, so legs 3 and 4 are not dead code.
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		t.Errorf("exit status: got 0 (process exited cleanly), want 2 (unrecovered panic); stderr=%q", stderr.String())
	case errors.As(runErr, &exitErr):
		if got := exitErr.ExitCode(); got != 2 {
			t.Errorf("exit status: got %d, want 2 (unrecovered panic); run error=%v; stderr=%q", got, runErr, stderr.String())
		}
	default:
		t.Errorf("exit status: run failed without an *exec.ExitError: %v", runErr)
	}

	// LEG 3 — stderr NAMES the panic. This is what pins the guard to a real
	// defect rather than to "something went wrong", and it is what a byte-count
	// assertion cannot do: the hanging tree emits hundreds of bytes of unrelated
	// statsd write-failure noise.
	const wantPanic = `stats: duplicate metric registration`
	if !strings.Contains(stderr.String(), wantPanic) {
		t.Errorf("stderr does not name the panic: want a line containing %q; got %d bytes:\n%s",
			wantPanic, stderr.Len(), stderr.String())
	}

	// LEG 4 — stderr carries Go's panic dump: the `panic:` header AND at least
	// one goroutine stack. A recover-and-log build satisfies leg 3 but not this.
	if !strings.Contains(stderr.String(), "panic: ") {
		t.Errorf("stderr lacks the %q header (a recovered-and-reprinted panic is not a panic dump); got %d bytes:\n%s",
			"panic: ", stderr.Len(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "goroutine ") {
		t.Errorf("stderr lacks a goroutine dump (want a %q frame header); got %d bytes:\n%s",
			"goroutine ", stderr.Len(), stderr.String())
	}
}

// firstLineOf returns s up to (not including) the first newline. Used to keep
// the boot-hang failure message short while still showing whether the subject
// printed a ready sentinel.
func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
```

- [ ] **Step 2: Prove the INPUT before trusting the RED.** Render the config and run the SAME file under `--mode validate` on the pre-T1 binary:

```sh
./envoy-go -c trigger.yaml --mode validate; echo "EXIT=$?"
```
Expected: `EXIT=2`, ~2.4–2.5 kB on stderr, containing `stats: duplicate metric registration: "http.dup_prefix.downstream_rq_total"` and a trace through `internal/stats/registry.go:107 ← internal/filter/hcm/config.go:358 ← … ← validate/validate.go:49`. ⚠️ **This isolates the trigger from the defer**: same config, same binary, one entry path prints ~2.5 kB and exits 2, the other prints **zero bytes** and never dies.

- [ ] **Step 3: Run it RED**, against `git stash`-ed (pre-T1) `main.go`:

```sh
go test -run 'TestMain_BootPanicIsVisible' -count=1 -v ./cmd/envoy-go/ 2>&1 | tee red.log
grep -c 'no tests to run' red.log     # MUST be 0 — the -run selector footgun
```
Expected: **FAIL** at ~31 s, **leg 1**, message containing `stdout=0 bytes ""`, `stderr=0 bytes`, `run error=signal: killed`. ⚠️ **A liveness break needs a FAILING baseline** — if this is green, the test is not running.

- [ ] **Step 4: Restore T1 and run it GREEN.**

```sh
go test -run 'TestMain_BootPanicIsVisible' -count=1 -v ./cmd/envoy-go/
```
Expected: **PASS** in ~0.95–1.05 s.

- [ ] **Step 5: Gate and commit.**

```sh
[ -z "$(gofmt -l cmd/envoy-go/main_test.go)" ] && golangci-lint run ./cmd/envoy-go/...
go test ./cmd/envoy-go/ -count=1        # WHOLE package — a file-scoped measure is not a build measure
git add cmd/envoy-go/main_test.go
git commit -m "phase 78 T3: the black-box boot-panic guard — a four-leg conjunction, hang detected from ctx.Err() rather than an exit code"
```
Expected whole-package: **9/9 PASS**, ~8.9–9.2 s (the guard adds ~0.54 s, ~+6 %; on a regression it costs the full 30 s deadline).

---

## Task 4 — structural arm 2: the receive follows **every** close

**Files:** Modify `cmd/envoy-go/main_test.go` (add `go/ast`, `go/parser`, `go/token` to the import block in sorted position after `"fmt"`; append).

**Interfaces:**
- Produces: `bootPanicVisibilityMainGo(t) (*token.FileSet, *ast.File, string)`, `isFlusherDoneReceive(ast.Node) bool`, `isFlusherDoneClose(ast.Node) bool`, `TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose`. **T5 consumes all three helpers.**

⚠️ **Parse, do not grep.** `go/parser` in mode `0` sees declarations, not comments — so `main.go`'s own prose about `<-flusherDone` and `close(flusherDone)` (a **majority** of the file's nine `flusherDone` occurrences are comment text, five of nine, and T2 rewrites several) is invisible here and cannot spoof the assertion in either direction. Precedent: `test/fixtures/0061-lb-ring-hash/driver/linkage_test.go`.

- [ ] **Step 1: Write the failing test.**

```go
// ---------------------------------------------------------------------------
// Phase 78 (ADR-0300) — the two STRUCTURAL arms of the boot-panic-visibility
// guard. They are structural because there is no config-reachable POST-anchor
// panic to drive a behavioral one: the only config-reachable boot-window panic
// trigger (duplicate stat_prefix -> internal/stats/registry.go:107, registered
// at internal/filter/hcm/config.go:352-362) fires inside boot.Construct, i.e.
// PRE-anchor, and a pre-anchor panic is fixed by the relocation alone. The
// behavioral guard therefore PASSES on a tree that still hangs post-anchor
// (EXECUTED, PLAN 78 T3); these two arms are what closes that gap.
// ---------------------------------------------------------------------------

// bootPanicVisibilityMainGo parses cmd/envoy-go/main.go and returns its AST plus
// the FileSet needed to turn token.Pos into line numbers. The path is resolved
// from THIS source file via runtime.Caller (the same technique as pkiFixture0002)
// rather than from the process working directory, so the arms are correct no
// matter how the test binary is invoked.
func bootPanicVisibilityMainGo(t *testing.T) (*token.FileSet, *ast.File, string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate main.go")
	}
	path := filepath.Join(filepath.Dir(thisFile), "main.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, f, path
}

// isFlusherDoneReceive reports whether n is the expression `<-flusherDone`.
func isFlusherDoneReceive(n ast.Node) bool {
	u, ok := n.(*ast.UnaryExpr)
	if !ok || u.Op != token.ARROW {
		return false
	}
	id, ok := u.X.(*ast.Ident)
	return ok && id.Name == "flusherDone"
}

// isFlusherDoneClose reports whether n is the call `close(flusherDone)`.
func isFlusherDoneClose(n ast.Node) bool {
	c, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := c.Fun.(*ast.Ident)
	if !ok || fn.Name != "close" || len(c.Args) != 1 {
		return false
	}
	arg, ok := c.Args[0].(*ast.Ident)
	return ok && arg.Name == "flusherDone"
}

// TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose is phase-78 structural
// arm 2: every `<-flusherDone` receive in main.go must sit at a line strictly
// GREATER than every `close(flusherDone)` call.
//
// Why it is red-on-regression: the phase-78 defect is a `defer func(){ <-flusherDone
// ... }()` registered ~70 lines BEFORE anything closes flusherDone. Every panic
// unwinding through main() in that window blocks forever on a channel that nothing
// has closed, producing a zero-byte silent hang. Moving the receive after both
// close sites is exactly what makes the boot window panic-visible, and that is a
// pure source-ORDER property, which is why this arm is structural.
//
// ⚠️ This arm is NOT sufficient on its own — it is GREEN on the naively-relocated
// tree, which still hangs. See the companion arm
// TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel.
//
// Anti-vacuity: a structural test that finds nothing and says nothing is GREEN for
// the wrong reason. Both counts are asserted non-zero BEFORE the ordering leg, and
// the two zero cases are reported separately so a rename of flusherDone can never
// masquerade as a pass.
func TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose(t *testing.T) {
	fset, f, path := bootPanicVisibilityMainGo(t)

	var receiveLines, closeLines []int
	ast.Inspect(f, func(n ast.Node) bool {
		switch {
		case isFlusherDoneReceive(n):
			receiveLines = append(receiveLines, fset.Position(n.Pos()).Line)
		case isFlusherDoneClose(n):
			closeLines = append(closeLines, fset.Position(n.Pos()).Line)
		}
		return true
	})

	// Anti-vacuity legs — never a silent pass.
	if len(receiveLines) == 0 {
		t.Fatalf("%s: found ZERO `<-flusherDone` receive expressions (closes found: %v) — "+
			"the phase-78 boot-panic-visibility guard is VACUOUS; if flusherDone was "+
			"renamed, re-point this arm at the new name, do NOT delete it", path, closeLines)
	}
	if len(closeLines) == 0 {
		t.Fatalf("%s: found ZERO `close(flusherDone)` calls (receives found: %v) — "+
			"the phase-78 boot-panic-visibility guard is VACUOUS; if flusherDone was "+
			"renamed, re-point this arm at the new name, do NOT delete it", path, receiveLines)
	}

	// Ordering leg.
	maxClose := closeLines[0]
	for _, l := range closeLines {
		if l > maxClose {
			maxClose = l
		}
	}
	for _, r := range receiveLines {
		if r <= maxClose {
			t.Errorf("%s:%d: `<-flusherDone` occurs at or before the last "+
				"`close(flusherDone)` (line %d; all closes: %v). A defer that waits on "+
				"flusherDone before anything can close it turns EVERY panic in the boot "+
				"window into a zero-byte silent hang (phase 78 / ADR-0300). Move the "+
				"waiting defer below the close(flusherDone) if/else.",
				path, r, maxClose, closeLines)
		}
	}
	t.Logf("%s: `<-flusherDone` receives at %v; `close(flusherDone)` calls at %v", path, receiveLines, closeLines)
}
```

- [ ] **Step 2: Run it RED** against pre-T1 `main.go`:

```sh
go test -run 'TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose' -count=1 -v ./cmd/envoy-go/
```
Expected: **FAIL** on the ordering leg at `main.go:299`, reporting `closes [368 370]`.

- [ ] **Step 3: Restore T1/T2 and run it GREEN.** Expected: **PASS**, `t.Logf` reporting receives at `[373]` and closes at `[353 355]` (exact numbers depend on your T1/T2 diff — record what YOU see).

- [ ] **Step 4: Prove the anti-vacuity leg fires.** Rename `flusherDone` → `flushDoneCh` throughout `main.go`, re-run, and confirm the **`found ZERO … receive expressions`** `t.Fatalf` fires — not a silent pass. Restore.

- [ ] **Step 5: Gate and commit.**

```sh
[ -z "$(gofmt -l cmd/envoy-go/main_test.go)" ] && golangci-lint run ./cmd/envoy-go/...
git diff --stat go.mod go.sum      # MUST be EMPTY — an import line is not a module
git add cmd/envoy-go/main_test.go
git commit -m "phase 78 T4: structural arm 2 — the flusherDone receive must follow every close, with both anti-vacuity fatals"
```

---

## Task 5 — structural arm 3 (**D-BPV-GUARD-COVERAGE**): the wait is released on the unwind path

**Files:** Modify `cmd/envoy-go/main_test.go` (append).

**Interfaces:** Consumes T4's `bootPanicVisibilityMainGo` and `isFlusherDoneReceive`. Produces `TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel`.

⚠️ **THIS IS THE TASK THAT CLOSES SPEC §1.2.** It is separated from T4 because T4's arm is **GREEN on the tree that still hangs**, and shipping the arm that catches the defect inside the same reviewer gate as the arm that demonstrates the gap defeats the point.

⚠️ **Scope the assertion to the DEFERRED BODY, not to `main()` generally** — a `cancel()` call elsewhere in `main` does not release this wait on the panic path. And find the receive in **that literal's own `Body.List`**, not in a nested literal: a nested literal is a different frame with different unwind rules.

- [ ] **Step 1: Write the failing test.**

```go
// TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel is phase-78 structural
// arm 3 (D-BPV-GUARD-COVERAGE): inside the deferred function literal that waits on
// flusherDone, a call to cancel() must appear BEFORE the receive.
//
// Why arm 2 is not enough. Relocating the waiting defer past the close sites makes
// it the LAST-registered defer in main(), hence the FIRST to run in LIFO — ahead of
// `defer cancel()`. Its wait is released only when statsFlusher.Start(ctx) returns,
// which requires ctx to be CANCELED. So on the naively-relocated tree a panic in
// the new post-anchor window still hangs (EXECUTED: exit 137 under a SIGKILL
// deadline, zero panic bytes) while arm 2 is GREEN, because the receive line is
// genuinely below every close line. The cancel() first in the body is what makes
// the wait unable to outlive an unwinding main(); context.CancelFunc is idempotent,
// so the normal signal path is byte-identical (measured: 12/12 arms, 152 datagrams
// / 5888 bytes on a live receiver, identical across trees).
//
// THIS ARM PINS ONE SHAPE, DELIBERATELY. A bounded `select { case <-flusherDone:
// case <-time.After(d): }` and a trailing sibling `defer cancel()` both also work
// behaviorally, and both go RED here. That is intended: accepting a second shape
// would make this arm reason about registration order across sibling statements,
// which is exactly the reasoning that failed and produced the moved hang. If a
// later row deliberately switches, it must EDIT this arm — a visible, reviewable
// act — rather than have it silently accept a second shape.
//
// Anti-vacuity: not finding the deferred wait at all is a t.Fatalf, never a pass.
func TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel(t *testing.T) {
	fset, f, path := bootPanicVisibilityMainGo(t)

	deferredWaits := 0
	guarded := 0
	ast.Inspect(f, func(n ast.Node) bool {
		d, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}
		lit, ok := d.Call.Fun.(*ast.FuncLit)
		if !ok || lit.Body == nil {
			return true
		}
		// The receive must be in THIS literal's own body — not in a nested
		// literal, which would be a different frame with different unwind rules.
		var recvPos token.Pos = token.NoPos
		for _, stmt := range lit.Body.List {
			ast.Inspect(stmt, func(m ast.Node) bool {
				if recvPos == token.NoPos && isFlusherDoneReceive(m) {
					recvPos = m.Pos()
				}
				return recvPos == token.NoPos
			})
			if recvPos != token.NoPos {
				break
			}
		}
		if recvPos == token.NoPos {
			return true
		}
		deferredWaits++

		// Two positions are tracked, not one, so the "no cancel() at all" and the
		// "cancel() present but AFTER the receive" regressions fire DISTINCT
		// messages. Both are red, but they are different mistakes and a shared
		// message would let one masquerade as the other in a break test.
		beforePos, afterPos := token.NoPos, token.NoPos
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			c, ok := m.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := c.Fun.(*ast.Ident)
			if !ok || id.Name != "cancel" || len(c.Args) != 0 {
				return true
			}
			if c.Pos() < recvPos {
				if beforePos == token.NoPos {
					beforePos = c.Pos()
				}
			} else if afterPos == token.NoPos {
				afterPos = c.Pos()
			}
			return true
		})
		if beforePos == token.NoPos {
			where := "there is NO cancel() call in this literal at all"
			if afterPos != token.NoPos {
				where = fmt.Sprintf("the only cancel() call is at line %d, AFTER the receive, "+
					"where it is unreachable during the hang", fset.Position(afterPos).Line)
			}
			t.Errorf("%s:%d: the deferred `<-flusherDone` wait is NOT preceded by a "+
				"`cancel()` call inside the same function literal — %s. Relocating this "+
				"defer below the close(flusherDone) sites makes it LAST-registered hence "+
				"FIRST in LIFO — ahead of `defer cancel()` — so on a panic path nothing "+
				"ever fires ctx.Done(), statsFlusher.Start(ctx) never returns, flusherDone "+
				"is never closed, and the panic is swallowed into a zero-byte hang exactly "+
				"as before (phase 78 / ADR-0300). Put cancel() FIRST in this body.",
				path, fset.Position(recvPos).Line, where)
			return true
		}
		guarded++
		t.Logf("%s: deferred `<-flusherDone` at line %d is preceded by `cancel()` at line %d",
			path, fset.Position(recvPos).Line, fset.Position(beforePos).Line)
		return true
	})

	// Anti-vacuity leg — never a silent pass.
	if deferredWaits == 0 {
		t.Fatalf("%s: found ZERO deferred function literals containing a `<-flusherDone` "+
			"receive — the phase-78 coverage arm is VACUOUS. If the wait moved or "+
			"flusherDone was renamed, re-point this arm; do NOT delete it.", path)
	}
	if guarded != deferredWaits {
		t.Errorf("%s: %d of %d deferred flusherDone waits are cancel()-guarded", path, guarded, deferredWaits)
	}
}
```

- [ ] **Step 2: Run it RED on the NAIVE relocation** — not on the pre-T1 tree. Temporarily delete the `cancel()` line from T1's body:

```sh
go test -run 'TestBootPanicVisibility' -count=1 -v ./cmd/envoy-go/
```
Expected — **this is the load-bearing pairing**: the **coverage arm FAILS** on the *"there is NO cancel() call in this literal at all"* leg while the **ordering arm stays GREEN**. If both go red, or the ordering arm goes red, stop: the arms are not independent and the split is not doing its job.

- [ ] **Step 3: Prove the tree it flags really does hang.** With the `cancel()` still deleted, inject a post-anchor panic, boot with a stats sink, and run under a **SIGKILL** deadline:

```sh
timeout -s KILL 12 ./envoy-go -c probe.yaml >o.out 2>o.err; echo "EXIT=$?"
echo "stdout=$(stat -c%s o.out)B stderr=$(stat -c%s o.err)B"
grep -c 'post-anchor probe' o.err
```
Expected: **`EXIT=137`, ~12 s, stdout 0 B, panic count 0.** ⚠️ stderr will be **non-zero** (500–1500 B of `statsd/dog_statsd udp write failed` noise) — §1.2. **That is why the guard asserts panic TEXT, not byte counts.** Restore the `cancel()`.

- [ ] **Step 4: Prove the distinct-message leg.** Move `cancel()` to AFTER the receive inside the body; re-run. Expected: coverage arm RED on the *"the only cancel() call is at line N, AFTER the receive"* leg — a **different** message from Step 2's. ⚠️ Without this, cells 2 and 4 of the break map produce byte-identical output and one can pass for the other's reason (`reference_deliberate_break_wrong_assertion`). Restore.

- [ ] **Step 5: Run GREEN** and confirm both arms pass, with the `t.Logf` naming the receive line and the `cancel()` line.

- [ ] **Step 6: Gate and commit.**

```sh
[ -z "$(gofmt -l cmd/envoy-go/main_test.go)" ] && golangci-lint run ./cmd/envoy-go/...
go test ./cmd/envoy-go/ -count=1 -v     # expect 10/10 PASS, ~7.9-8.2 s
git add cmd/envoy-go/main_test.go
git commit -m "phase 78 T5: structural arm 3 (D-BPV-GUARD-COVERAGE) — the deferred wait must be preceded by cancel() in its own literal; RED on the naive relocation while arm 2 stays GREEN"
```

---

## Task 6 — the break roster: **eight arms**, each proven to fire its OWN assertion

**Files:** no shipped change. Every arm is applied, run, recorded in `PROGRESS.md`, and **reverted**.

⚠️ **Use `-count=1` on every arm** (`reference_differential_break_protocol_count1`) and **confirm which leg fired** (`reference_deliberate_break_wrong_assertion`). ⚠️ **Commit before breaking** (`reference_break_protocol_commit_first`) — `git restore` wipes uncommitted work, and an unscoped `git restore` must never be run.

The SPEC requires *"at minimum seven"*. This roster is **eight**: the SPEC's seven, minus the healthy-boot arm (demoted to a recorded demonstration, see below), plus the `cancel()`-after-receive arm (§T5 Step 4) and the residual demonstration.

| # | arm | edit | expected RED leg | proves |
|---|---|---|---|---|
| **α** | revert-the-relocation | `main.go` ← pre-T1 | **T3 leg 1** (`stdout=0 bytes ""`, `stderr=0 bytes`) **and** T4 ordering leg | the whole row |
| **β** | delete-the-`cancel()` | drop `cancel()` from the body | **T5 only** — *"NO cancel() call in this literal at all"*; ⚠️ **T3 and T4 stay GREEN** | §1.2 / SPEC §3.7 — the arm adds coverage rather than duplicating |
| **γ** | `cancel()` after the receive | move it below `<-flusherDone` | **T5**, *"the only cancel() call is at line N, AFTER the receive"* — a **distinct** message from β | the two mistakes are distinguishable |
| **δ** | **print-then-hang** | prepend to `main()`: `defer func(){ if r:=recover(); r!=nil { fmt.Fprintf(os.Stderr, "panic: %v\n\ngoroutine 1 [running]:\nmain.main()\n\t/fake/main.go:1 +0x0\n", r); select{} } }()` | **T3 leg 1** | ⚠️ **R5's output half.** Show BOTH halves: the full test goes red on leg 1, **and** legs 3/4a/4b each evaluate TRUE against this build's 141-byte stderr. An output-only assertion PASSES here |
| **ε** | trigger → get-or-create | test args `"dup_prefix","dup_prefix"` → `"uniq_a","uniq_b"` | **T3 leg 1**, with `stdout=109 bytes "envoy-go listener l_a ready on …"` | the guard is not vacuous when the trigger stops triggering; and leg 1's message discriminates its two red modes |
| **ζ** | trigger → clean reject | test args → `"bad-prefix","bad-prefix"` (hyphen fails `stats.IsValidName`) | **legs 2, 3, 4a, 4b — all four, in one run, at ~1.4 s** | ⚠️ **the `t.Errorf` discipline itself.** Had leg 2 been `Fatalf`, legs 3/4 would be dead code and the maintainer would see only `got 1, want 2`, losing the information that identifies *which* future the trigger fell into |
| **η** | structural rename | `flusherDone` → `flushDoneCh` throughout | **both** anti-vacuity `t.Fatalf`s (T4 *"found ZERO … receive expressions"*, T5 *"found ZERO deferred function literals …"*) | neither structural arm silently passes on a rename |
| **θ** | **the residual** — a DIFFERENT blocking defer | on the FIXED tree insert `defer func() { <-make(chan struct{}) }()` before `boot.Construct` | ⚠️ **NOTHING goes red** | see below |

### ⚠️ Arm θ is the most valuable line in this task, and it is a NEGATIVE result

EXECUTED at this PLAN. On the fully-fixed tree, with all three arms shipped:

```
structural: AfterEveryClose=PASS  PrecededByCancel=PASS
behavioural: PASS      whole package: 10/10 PASS
binary:     EXIT=137  wall=12.001s  stdout=0B  stderr=0B  panic-present=0
```

**Two green structural arms, a green behavioural arm, a green package suite — and a binary that boots to a totally silent 12-second wedge with ZERO bytes on both streams.** Quieter, in fact, than the original defect, because the relocated defer's `cancel()` runs first and silences the flusher's error log before the process wedges on the sibling defer.

⇒ **all three arms are blind to a different blocking defer introduced elsewhere in the window.** SPEC §3.4's detector (inject a blocking defer and see whether the hang reproduces) is a **METHOD, not a shipped test**, and the PLAN says so in those words. Record arm θ's output verbatim in `PROGRESS.md`; **do not** attempt to fix it in this row.

### The demoted arm — recorded, not shipped

The SPEC's **healthy-boot** arm is a *demonstration*, not a test. A test asserting "leg 1 fires on a healthy server" is a tautology (a healthy server never exits) and would add a 30-second sleep to the suite. Its content is the §1.7d table, which belongs in `PROGRESS.md`:

| arm | `ctx.Err()` | `runErr` | `ExitCode()` | stdout | stderr |
|---|---|---|---|---|---|
| healthy boot, fixed binary | `context deadline exceeded` | `signal: killed` | **-1** | **109 B** | 1410 B |
| baseline (hung), trigger cfg | `context deadline exceeded` | `signal: killed` | **-1** | **0 B** | **0 B** |
| fixed binary, trigger cfg | `<nil>` | `exit status 2` | **2** | 0 B | 2302 B |

**Rows 1 and 2 are byte-identical on every status observable.** Arm ε already covers "the trigger stopped triggering" with real red-on-change value.

- [ ] **Step 1–8:** run arms α…θ in order. For each: record the exact edit, the exact leg(s) that fired with their message text, and confirm the tree is restored GREEN (`go test ./cmd/envoy-go/ -count=1`) before the next arm.
- [ ] **Step 9:** write all eight into `PROGRESS.md` with verbatim output. Commit (`docs` only — no code change survives this task).

---

## Task 7 — `BEHAVIOR_CONTRACT.md`: the boot-failure-visibility subsection

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md` (insert after `:872`, before `:874`).

**Anchors, controller-verified at this tip:**

```
860  ## Bootstrap config validation (per phase 51 ADR-0268)
862  **What triggers it.** … github.com/esalaine/envoy-go/validate …   ← the WRONG path, live
864  **What it does — validation depth.** …
866  **Public API shape.** …
868  **The `--mode validate` CLI flag.** …
870  **Byte-stable / zero differential surface.** …
872  **No new fuzzer (D-VALIDATE-FUZZER).** …
873  (blank)                                                          ← INSERT HERE
874  ### Does not yet apply to (bootstrap config validation)
875-877  three existing bullets
879  ---
```

⚠️ **Anchor on the PARENTHESISED heading.** There are **15** `### Does not yet apply to` headings in the file; the bare string is ambiguous. The exact string is `### Does not yet apply to (bootstrap config validation)`.

- [ ] **Step 1: Re-derive both anchors** with `grep -n '^## Bootstrap config validation'` and `grep -n '^### Does not yet apply to (bootstrap config validation)'`. Record what YOU see.

- [ ] **Step 2: Insert the subsection.** It must say what it actually covers. Content, four elements:
  1. **A panic unwinding through `main()` during boot is always visible** — it kills the process with exit status **2** and prints Go's panic dump (header + goroutine stacks) to stderr. It never hangs.
  2. **The two boot-failure mechanisms and their exit codes** — `log.Fatalf` → `os.Exit(1)` with a one-line message (config, bind, construction errors), **which skips every deferred call**, versus an unrecovered panic with exit status 2 and a goroutine dump. The fix changes **nothing** for the `os.Exit` class.
  3. **Entry-path symmetry**: `--mode validate` and normal boot report the same panic the same way. Before this row they diverged — the same config, the same panic, the same line, printed on one path and swallowed on the other.
  4. **Two new `### Does not yet apply to` bullets**, appended after `:877`:
     - a boot-path block that occurs **before any defer is registered** — `os.OpenFile` on an `access_log` `path` that is a FIFO with no reader (`internal/accesslog/writer.go:56`, reached from `main.go:117`) still hangs silently;
     - a **shutdown**-class unbounded wait — `AsyncFileSink.Close` (`internal/accesslog/writer.go:91-98`) is the only sink `Close` with no grace timeout.

⚠️ **DO NOT write "a boot failure is never silent."** ADR-0300 §Context ¶11 forecloses it and bullet (a) is the live counter-example. Scope the statement to *panics unwinding through `main`*.
⚠️ **No stat-ledger line** (the surface is unchanged) and **no whole-file grep count anywhere** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297.
⚠️ **Do NOT inherit `github.com/esalaine/envoy-go/validate`** from the neighbouring `:862`. The real module path is `github.com/pgdad/envoy-go` (`head -1 go.mod`). This row does **not** fix the 36 existing occurrences (§7).

- [ ] **Step 3: Verify.**

```sh
# the new subsection must not carry the wrong path
sed -n '<new-range>p' docs/envoy-go/BEHAVIOR_CONTRACT.md | grep -c 'esalaine'      # MUST be 0
# the forbidden absolute claim
sed -n '<new-range>p' docs/envoy-go/BEHAVIOR_CONTRACT.md | grep -ci 'never silent' # MUST be 0
grep -c '^### Does not yet apply to (bootstrap config validation)' docs/envoy-go/BEHAVIOR_CONTRACT.md  # 1
```
⚠️ **Negative-control each grep** — run it against a scratch copy that DOES contain the token and confirm it reports non-zero, so an empty output is a measured zero and not a broken command (`reference_empty_output_is_not_a_zero_result`).

- [ ] **Step 4: Commit.**

---

## Task 8 — ADR-0300 §Decision + §Consequences, appended IN PLACE

**Files:** Modify `docs/envoy-go/DECISIONS.md` (append at EOF).

**Anchors, controller-verified:** heading `^## ADR-0300` at `:17532`; block runs to **EOF at `:17560`** (29 lines); status line `:17534` = **PROPOSED**; **`^---$` count within the block = 0**; italic footer `*(§Decision + §Consequences land at the phase-78 IMPL.)*` is the **last line of the file** (`tail -c 90 | od -c` ends `IMPL.)*\n`); `grep -c '^## ADR-0301'` ⇒ **0**; §Context has **11** `**§Context ¶` paragraphs.

- [ ] **Step 1: Locate by CONTENT anchor, not a line range** (`reference_stale_cite_recurs_fix_by_pattern` — a RANGE gate cannot detect anchor drift):

```sh
awk '/^## ADR-0300/{f=1} f' docs/envoy-go/DECISIONS.md | wc -l
awk '/^## ADR-0300/{f=1} f' docs/envoy-go/DECISIONS.md | grep -c '^### Context\|^### Decision\|^### Consequences\|^---$'
```

- [ ] **Step 2: Append §Decision and §Consequences after the retained italic footer.** ⚠️ **No renumber. NO `---` separator** — the convention was abandoned, controller-verified per block: ADR-0295/0296/0297/0298/0299/0300 all carry **0**. ⚠️ **Retain the footer**; flip the status line PROPOSED → the completed form, mirroring ADR-0295–0299.

**§Decision must state, at minimum:**
- the shipped shape — relocation **plus** `cancel()` as the **first** statement of the deferred body — and that **relocation alone is not the fix** (¶4);
- that the **two alternatives are recorded and NOT permitted**, with the reason being the guard (§1.1), not taste;
- the three-arm guard structure, and that **the behavioural arm passes on a tree that still hangs**, which is why the structural coverage arm exists;
- that the hang is detected from the **deadline state**, never an exit code, and that **SIGTERM cannot falsify the contract**.

**§Consequences must state:**
- what the row does **not** cover (¶11's FIFO `os.OpenFile` hang; the shutdown-class `AsyncFileSink.Close`);
- that all three arms are blind to a **different** blocking defer in the window (arm θ, EXECUTED);
- that the `/stats/prometheus` projection-completeness gap (`ROADMAP.md:208`) is **UNBLOCKED** by this row.

⚠️ **NO whole-file grep count anywhere in the ADR.** Every count line-scoped or stated with no numeral.
⚠️ **US spelling is not required here** (this is Markdown, not Go) — but do not copy ADR prose into `.go` comments (§1.6).

- [ ] **Step 3: Verify block shape** — expect **1** `### Context` / **1** `### Decision` / **1** `### Consequences` / **1** retained footer / **0** new `^---$`, and `grep -c '^## ADR-0301'` ⇒ **0**. ⚠️ Run the same `awk` range gate over **ADR-0299** as a negative control, confirming it reports that block's own shape rather than ADR-0300's.

- [ ] **Step 4: Commit.**

---

## Task 9 — the gates, including the **three** suites that build this binary

**Files:** no change. Every command run in the stage worktree with `git -C <abs-path>` discipline.

- [ ] **Step 1: Package gates.**

```sh
[ -z "$(gofmt -l ./cmd/envoy-go/)" ] || { echo "GOFMT FAIL"; gofmt -l ./cmd/envoy-go/; exit 1; }
golangci-lint run ./cmd/envoy-go/...
go vet ./cmd/envoy-go/
go test ./cmd/envoy-go/ -count=1 -v ; echo "INNER_EXIT=$?"
go test ./cmd/envoy-go/ -count=1 -race
git diff --stat go.mod go.sum          # MUST be EMPTY
```
Expected: **11/11 top-level PASS** (8 pre-existing + 3 new). Baseline at this tip is **8 tests / 12 `--- PASS` lines** (some carry subtests) in ~10 s wall.

- [ ] **Step 2: THE FULL 120-FIXTURE DIFFERENTIAL — MANDATORY, NOT OPTIONAL.** `test/differential/harness.go:240` (`StartSubjectProxy`) and `:594` (`tryStartSubjectProxy`, the `BootRejectFixture` path) build and spawn `./cmd/envoy-go` for **every** fixture, so a shutdown-ordering change to `main()` has a **120-fixture blast radius**.

```sh
go test ./test/differential/ -run TestDifferential -count=1 -v > full.log 2>&1
INNER_EXIT=$?          # capture INSIDE — a harness's exit code is not the command's
echo "INNER_EXIT=$INNER_EXIT"
grep -c -- '--- PASS' full.log ; grep -c -- '--- FAIL' full.log
grep -c -- '--- SKIP' full.log ; grep -c '^panic:'    full.log
# scope the tally to the fixture denominator, then CROSS-CHECK it:
grep -oE -- '--- (PASS|FAIL|SKIP): TestDifferential/[0-9]{4}[a-z]?-[^ ]+' full.log \
  | sed 's#.*TestDifferential/##' | sort > subtests.txt
ls -1 test/fixtures | grep -E '^[0-9]{4}[a-z]?-' | sort > fixdirs.txt
wc -l fixdirs.txt subtests.txt        # MEASURE THE INPUT — expect 120 each
comm -3 fixdirs.txt subtests.txt      # MUST be EMPTY
```

⚠️ **`-v` is mandatory.** Executed negative control at this PLAN: a green package without `-v` yields a **53-byte** log where `grep -c -- '--- PASS'` **and** `--- FAIL` both return **0**. With `-v`: 1090 bytes, 13 PASS. **Refinement the SPEC does not carry** — `--- FAIL` **IS** printed without `-v` on a red run (executed: `'--- FAIL' = 1`). The precise statement: *without `-v`, a green full-suite log is indistinguishable from a suite that ran nothing — there is no pass tally and no per-fixture record. Failures remain visible.*
⚠️ **The raw `--- PASS` count EXCEEDS 120** — `test/differential` declares 15 other top-level tests (`TestReferenceH3_ServesGET`, `TestParseEnvoyTarget_*`, `TestCompareBytes_*`, the responder-backend tests, …). Phase 77 measured **134** for **120** subtests. This is why the tally is scoped to `TestDifferential/` and cross-checked with `comm -3`.
⚠️ **Budget ~400–420 s per green attempt** (phase-77 record: 120 fixtures / 403 s; phase-76: 119 / 409.8 s). A `-race` attempt is a **second** run, not a substitute, and phase 76 recorded one dying on the bind flake after 254 s.

**The ten fixtures whose assertions ride the reordered chain** — enumerate their results individually in `PROGRESS.md`, do not merely report a total:

| index | fixture | sink kind |
|---|---|---|
| 0089 | `0089-stats-sink-metrics-service` | metrics_service |
| 0090 | `0090-stats-sink-metrics-service-deltas` | metrics_service (deltas) |
| 0091 | `0091-stats-sink-metrics-service-labels` | metrics_service (labels) |
| 0092 | `0092-stats-sink-statsd` | statsd UDP |
| 0093 | `0093-stats-sink-dogstatsd` | dog_statsd |
| 0094 | `0094-stats-sink-dogstatsd-batching` | dog_statsd (batching) |
| **0098** | `0098-stats-sink-statsd-tcp` | ⚠️ **statsd TCP — the tree's ONLY stats-sink background mutator**, and the send-on-closed-channel exposure |
| 0101 | `0101-stats-sink-graphite` | graphite_statsd |
| 0112 | `0112-stats-sink-otlp` | OTLP metrics |
| 0113 | `0113-stats-sink-otlp-knobs` | OTLP metrics (knobs) |

- [ ] **Step 3: `TestH2Spec` — THE THIRD BUILD SITE** (§1.5). `test/conformance/h2spec/h2spec_test.go:210` also compiles and spawns `./cmd/envoy-go`, and the project's conformance claim is **h2spec 53/53**.

```sh
go test ./test/conformance/h2spec/ -run TestH2Spec -count=1 -v > h2spec.log 2>&1
INNER_EXIT=$?; echo "INNER_EXIT=$INNER_EXIT"
grep -c 'no tests to run' h2spec.log     # MUST be 0
```
Record the pass count. ⚠️ **No phase-78 document names this suite** — if it is excluded, the exclusion must be *stated with a reason*, not silent.

- [ ] **Step 4: The counts, each with its negative control.** Every one **+0** for this row:

| axis | expected | command | negative control |
|---|---|---|---|
| fixtures | **120** | `ls -d test/fixtures/[0-9]*/ \| wc -l` | faithful predicate `ls -1 test/fixtures \| grep -cE '^[0-9]{4}[a-z]?-'` ⇒ also 120; complement `grep -vE` ⇒ empty; `^ZZZZ` ⇒ 0 |
| fuzzers | **55** | `grep -rn '^func Fuzz' --include='*.go' internal/ \| wc -l` | ⚠️ the repo-wide form also returns 55 (all fuzzers live under `internal/`), so it is **not** discriminating — append a scratch `func FuzzProbeOnly` ⇒ 56 |
| internal packages | **73** | `go list ./internal/... \| wc -l` | — |
| blank imports | **120** | `grep -cE '^\t_ "github.com/pgdad/envoy-go/test/fixtures/' test/differential/runner_test.go` | ⚠️ naive `^\t_ ` ⇒ **126** (2 internal filter pkgs + 4 `_ =` assignments) |
| BackendKind | **tail 38** | tail of `BackendKind = N` | ⚠️ a TAIL VALUE — 39 constants, `TCPEcho = 0`; do **NOT** "fix" 38 to 39 |
| go.mod | clean | `git diff master -- go.mod go.sum \| wc -c` ⇒ **0** | single `go.mod` requires **67** (18 direct + 49 indirect); the lineage figure **2** is phase-61.2's quic-go count, **not** a repo total |
| stat surface | **1207**, +0 | ⚠️ **no mechanical command; DOCUMENTARY, two unaudited ledger gaps — assert the DELTA** | see below |

⚠️ **The `+0` stat-surface claim CANNOT be discharged by a `TestNoNewStat*` run.** Controller-verified: all **five** such guards live in `internal/statssink/registration_test.go` (`:26 :53 :81 :109 :137`) and guard **that package's** registration sites. **None applies to `cmd/envoy-go`.** The available argument is **structural** and must be stated as such: `main()` contains no `NewCounter`/`NewGauge` call, and the production diff is confined to `main()`. Phase 76 used exactly this structural form.

- [ ] **Step 5: The byte-untouched roster, spanning BOTH trees.**

```sh
# roster.sh <baseline-ref> <worktree-root> <path...>
for p in "$@"; do
  git -C "$CANON" cat-file -e "$BASE:$p" || { echo "ROSTER FAIL: $p ABSENT at baseline"; rc=1; continue; }
  [ -f "$WT/$p" ]                        || { echo "ROSTER FAIL: $p DELETED in worktree"; rc=1; continue; }
  b=$(git -C "$CANON" show "$BASE:$p" | sha256sum | cut -d' ' -f1)
  w=$(sha256sum "$WT/$p" | cut -d' ' -f1)
  [ "$b" = "$w" ] || { echo "ROSTER FAIL: $p MODIFIED ($b -> $w)"; rc=1; }
done; exit $rc
```
Roster for this row: `internal/stats/**` · `docs/envoy-go/ADR-0089`'s block · `cmd/envoy-go/main.go`'s SURVIVE comment sites (by content grep, not sha).
⚠️ **It must span BOTH trees** — a roster built from the tip's `git ls-files` alone is desynced **by construction** against a DELETED file, and a naive `[ -f ] || continue` **exits 0** on one. All three negative controls were executed at this PLAN: MODIFIED ⇒ RED; file `mv`'d away ⇒ `DELETED in worktree`, RED; roster naming a nonexistent path ⇒ `ABSENT at baseline`, RED.

- [ ] **Step 6: The three sentinel checks, re-run AFTER the `ROADMAP.md` edit lands in T10** — and never before. Record ACTUAL output. Expected after T10: check (1) **silent** (row 78 `done`), **no `GATE FAIL`**; check (2) **FIVE, unchanged** (`:188 :198 :208 :214 :222`); check (3) `NEVER OPENED: gRPC` / `NEVER OPENED: WASM`. ⚠️ **`stop` fires only if ALL THREE print nothing** — checks (2) and (3) will not, so **`stop` must NOT be created.**

- [ ] **Step 7: Flake triage, if anything is red.** Isolate-re-run, then state the classification **and its evidence** (§8). ⚠️ **A stage brief's flake list is not the index.**

---

## Task 10 — row 78 → `done`, `STATE.md`, and the stage close

**Files:** Modify `docs/envoy-go/ROADMAP.md` (row 78 at `:140`), `docs/envoy-go/STATE.md`, `next-prompt.txt`.

- [ ] **Step 1: Flip row 78 `in-progress` → `done`** and write its IMPL summary in the row's cell.

⚠️ **NEVER WRITE A SENTINEL MATCHER STRING INTO `ROADMAP.md`.** The sentinel greps that file. Forbidden substrings in any new cell text: `deferred candidates:`, `remaining deferred (not-yet-chartered) candidates:`, and any `<Family>-family row` slug you do not intend to register. This fired **live** at the phase-76 BRAINSTORM, twice in one commit — **`grep` cannot tell a mention from a use.**

- [ ] **Step 2: `want` STAYS 110.** This row adds no ROADMAP row. Verify: `awk … -v want=110` prints no `GATE FAIL`.

- [ ] **Step 3: Re-run ALL THREE sentinel checks** (T9 Step 6) and record the actual output.

- [ ] **Step 4: Roll `STATE.md` §Current pointer IN PLACE** (ADR-0288 — **never prepend a new block above it**). Update all seven singleton fields; verify each with `grep -c '^- \*\*<field>'` ⇒ **1**.
⚠️ **Record, do not silently fix,** the pre-existing bookkeeping gaps: there has never been a `phase-77 PLAN` lineage entry; the §Recent-lineage heading says FIVE while the list is six; and **§Project counts is stale on THREE axes** — fixtures 119 (real 120), stat surface 1205 (lineage 1207), DECISIONS tail ADR-0298 (real **ADR-0300**) — of which `next-prompt.txt` flags only the first (§1.7g). **Repairing a count by editing the sentence that states it is how the ADR-0296/0297 species starts.**

- [ ] **Step 5: Roll `next-prompt.txt`** to the next stage. Carry forward: the §1.1 retraction, arm θ's negative result, the third build site, the `misspell` gate, and the re-derived cost.

- [ ] **Step 6: The six-gate.** `BOOTSTRAP_PROMPT.md` §7.5, gates (a)–(f) at `:357-366`. ⚠️ **Cite it correctly: *"the six-gate (`BOOTSTRAP_PROMPT` §7.5) … the SOLE leg (ADR-0106)"*.** **ADR-0106 does NOT define the six-gate** — its block (`DECISIONS.md:4788-4857`) contains **zero** occurrences of "six"; its subject is the §9 family-expansion shape and the SOLE-leg property. The gates live at the **repo root**, not under `docs/envoy-go/`.

- [ ] **Step 7: Commit, squash, push.** Controller squashes and pushes at close (`feedback_push_to_origin`); subagents commit locally only.

---

## 4. Break map — **eight** arms, **seven already RUN at this PLAN**

| arm | run at this PLAN? | outcome observed |
|---|---|---|
| α revert-the-relocation | ✅ | T3 leg 1 FAIL 31.00 s (`stdout=0 bytes ""`); T4 ordering leg FAIL at `main.go:299` |
| β delete-the-`cancel()` | ✅ | **T5 RED, T4 GREEN, T3 GREEN** — the load-bearing pairing |
| γ `cancel()` after receive | ✅ | T5 RED with the **distinct** *"AFTER the receive"* message |
| δ print-then-hang | ✅ | T3 leg 1 FAIL 30.98 s; **legs 3/4a/4b each TRUE** against its 141-byte stderr |
| ε trigger → get-or-create | ✅ | T3 leg 1 FAIL, `stdout=109 bytes "envoy-go listener l_a ready on 127.0.0.1:32803"` |
| ζ trigger → clean reject | ✅ | T3 **legs 2, 3, 4a, 4b all four** in one 1.36 s run |
| η structural rename | ✅ | **both** anti-vacuity `t.Fatalf`s fired |
| θ the residual | ✅ | ⚠️ **all arms GREEN, binary wedged 12 s at 0 B / 0 B** |

⚠️ **The IMPL must RE-RUN every arm at its own tip.** A prior stage's red is not this stage's red, and `main.go`'s line numbers move under T1/T2.

⚠️ **Two arms are AST-IDENTICAL.** "Delete the `cancel()`" (β) and "the naive relocation as the BRAINSTORM specified" differ only in **comment text**, which `go/parser` in mode `0` does not see. They are one structural case, not two independent ones — which is itself the anti-spoofing property that makes the AST technique better than grep. Do not count them as two arms.

---

## 5. Gates — every one NEGATIVE-CONTROLLED at this PLAN

| # | gate | status |
|---|---|---|
| **G1** | `gofmt`, gated on **OUTPUT** | **[RUN]** ⚠️ **PROVEN: `gofmt -l` exits 0 while listing a file.** Clean pkg ⇒ exit 0, empty. Scratch copy with `func   badlyFormatted( ) {}` ⇒ **exit 0** and a filename on stdout. The `&&`-chained form is **inert**; the `[ -z "$(…)" ]` form is the gate |
| **G2** | `golangci-lint run ./cmd/envoy-go/...` | **[RUN]** exit 0 · ⚠️ **`misspell` locale US** — British spellings in `.go` comments FAIL (§1.6) |
| **G3** | sha256 roster spanning BOTH trees | **[RUN]** 3 negative controls fired: MODIFIED · DELETED-in-worktree · ABSENT-at-baseline |
| **G4** | the three sentinel checks | **[RUN]** twice independently; NCs: `want=109` ⇒ `GATE FAIL: examined 110 … expected 109`; scratch row-77 flip ⇒ names row 77; invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`; input measured (994 044 B / 5 hits vs an empty control 0 B / 0 hits) |
| **G5** | the `-run` selector | **[RUN]** `-run 'TestNoSuchThingZZZ'` ⇒ `ok … [no tests to run]`, **exit 0**. Every task greps its log for `no tests to run` |
| **G6** | whole-package `go test ./cmd/envoy-go/ -count=1` | **[RUN]** baseline **8 tests / 12 `--- PASS`** green at this tip, ~10 s; with all three arms **10/10–11/11**, 7.9–9.2 s |
| **G7** | `go.mod` untouched | **[RUN]** `git diff --stat go.mod go.sum` empty with `go/ast`+`go/parser`+`go/token` added — an import line is not a module |
| **G8** | the full 120-fixture differential | **[IMPL]** — T9 Step 2. **Capture the inner exit status; derive the tally from the log; cross-check the denominator with `comm -3`** |
| **G9** | `TestH2Spec` | **[IMPL]** — T9 Step 3. The third build site (§1.5) |
| **G10** | the `-v` vacuity property | **[RUN]** green-without-`-v` ⇒ 53 B log, 0 PASS **and** 0 FAIL; green-with-`-v` ⇒ 1090 B, 13 PASS; **red-without-`-v` ⇒ `--- FAIL` = 1** (the refinement, §1.5 note) |
| **G11** | ADR-0300 block shape, content-anchored | **[IMPL]** — T8 Step 3, with the ADR-0299 range as negative control. ⚠️ **A RANGE gate cannot detect anchor drift** |
| **G12** | counts, each with its NC | **[RUN]** all eight; ⚠️ the fuzzer count's `internal/`-vs-repo-wide pair is **NOT** discriminating (both 55) — use the append-a-`func Fuzz` NC |

⚠️ **Broken/hazardous-gate count: 13 → 14** (§1.6 adds the `misspell` locale). The standing thirteen: full-suite recipe without `-v` is vacuous · a roster from one tree is desynced against a DELETED file · `gofmt -l` never exits non-zero · `go doc -all <A> <B>` silently swallows arg2 · a `+0 exported symbols` gate over an EMPTY package goes RED on a correct tree · a RANGE gate cannot detect anchor drift · a naive `[ -f ] || continue` exits 0 on a deleted file · a count-only stat guard passes a build with BOTH names wrong · **a harness's exit code is not the command's** · a `-run` no-match exits 0 · a `--- PASS` tally over a package with sibling tests exceeds the fixture denominator · a stat-delta claim cannot be discharged by guards scoped to another package (T9 Step 4) · a stderr-VOLUME assertion passes on the hang (§1.2).

---

## 6. Self-review against the SPEC

| SPEC item | covered by | note |
|---|---|---|
| §3.1 D-BPV-FIX (pinned form) | **T1** | ⚠️ §3.1's alternative (ii) **RETRACTED** — §1.1 |
| §3.2 D-BPV-ORDER | **T1 Step 6** | the SIGTERM path proven byte-identical with a **live receiver** and measured datagram counts |
| §3.3 D-BPV-WINDOW | **T1 Step 5** | ⚠️ agent A added a **mid-window** position the SPEC never tested; the baseline hangs there too, sink present **and** absent |
| §3.4 D-BPV-OTHER-DEFERS | **T6 arm θ** | the SPEC's detector is a **METHOD, not a shipped test** — arm θ proves the residual by execution |
| §3.5 D-BPV-LOGFATAL | **T7 element 2** | the contract states the `os.Exit` class is unchanged |
| §3.6 D-BPV-GUARD-TRIGGER | **T3** | all three pinning mechanisms present: the conjunction, the doc comment (with the `:352-362` correction, §1.7c), and T5 as the trigger-independent companion |
| §3.7 D-BPV-GUARD-COVERAGE | **T5** | ⚠️ **split into its own task** — §1.8 |
| §3.8 D-BPV-PROSE | **T2** | ⚠️ denominator **9 sites, not 7** (§1.3); `:198` reclassified (§1.4) |
| §5 identifier hygiene | **§3 + T3 Step 1** | ⚠️ SPEC's "4 helpers" refuted — **5** (§1.7a) |
| §6 stat/fuzz/fixture +0 | **T9 Step 4** | ⚠️ the `TestNoNewStat*` guards do **not** apply — the argument is structural |
| §7 behavior contract | **T7** | ⚠️ must not claim "never silent"; must not inherit the wrong import path |
| §8 sentinel | **T9 Step 6 + T10** | ⚠️ this row narrows **NOTHING**; check (2) stays at five. **Stated, not forecast** |
| §9 task surface 7–9 | **§1.8** | ⚠️ re-derived to **10** |
| §9 full differential | **T9 Step 2** | ⚠️ plus **`TestH2Spec`**, the third build site the SPEC never names (§1.5) |
| §11 deferrals | **§7** | all five carried forward, none fixed |
| §12 ADR-0300 | **T8** | no renumber, no `---`, footer retained |

**Gaps found and closed by this review:** the §3.1/§3.7 contradiction (§1.1) · the two missed prose sites (§1.3) · the third build site (§1.5) · the `misspell` gate (§1.6) · the stat-delta guard scope (T9 Step 4).

**Nothing in the SPEC is left unaddressed.** Two items are addressed by *retraction* rather than implementation (§3.1's alternatives), and that is stated rather than silent.

---

## 7. Deferred — named so a future sweep finds them

1. **`AsyncFileSink.Close`** (`internal/accesslog/writer.go:91-98`) — the only sink `Close` with no grace timeout. **Shutdown-class**; this row is boot-class.
2. ⚠️ **A SECOND silent zero-byte boot hang that is not a defer at all** — `os.OpenFile` at `internal/accesslog/writer.go:56`, reached from `main.go:117`, blocks forever on a FIFO `access_log` path with no reader, **before any defer is registered**. EXECUTED with a discriminating control (reader attached ⇒ boots to `envoy-go ready`). **This row's fix cannot reach it**, and T7's contract wording must say so.
3. ⚠️ **The residual all three guards are blind to** — a *different* blocking defer introduced elsewhere in the boot window (T6 arm θ, EXECUTED: three green arms, a green package suite, and a 12-second silent wedge). The detector is a **method**, not a shipped test.
4. **The `/stats/prometheus` projection-completeness gap** (`ROADMAP.md:208`) — 30 names / 6 families / 28 pre-dating phase 77. **UNBLOCKED by this row.**
5. **Whether duplicate `stat_prefix` should be rejected or aggregated** (SPEC §2.1), and the `Listener.stat_prefix` row at `ROADMAP.md:139` that would settle it. ⚠️ **There is exactly ONE config-reachable in-window panic trigger and NO fallback** — a future row that legislates it away must re-point T3's trigger, not relax its assertion.
6. **The documentary defects, unchanged:**
   - ⚠️ **The public import path `github.com/esalaine/envoy-go` does not exist** (`head -1 go.mod` ⇒ `github.com/pgdad/envoy-go`). **36 occurrences** of `esalaine/envoy-go/validate` repo-wide, including `BEHAVIOR_CONTRACT.md:862` (inside the very section T7 extends) and four in `DECISIONS.md`. ⚠️ **`DECISIONS.md:142` is `## ADR-0006: module path \`github.com/esalaine/envoy-go\`` — an ADR that DECIDES the wrong path, never superseded.** ⚠️ **Fix by PATTERN `esalaine/envoy-go` (bare), not the `/validate` form** — ADR-0006 has no `/validate` suffix and is invisible to the narrower grep (`reference_stale_cite_recurs_fix_by_pattern`).
   - A mechanical stat-surface count to replace the documentary **1207** — 8–11 tasks, two unaudited ledger gaps.
   - The unresolved *"plus two more in the same file"* half of the `BEHAVIOR_CONTRACT.md` stale-cite claim (`internal/tls/config.go:272`, `internal/filter/http/chain.go:19` flagged, **not audited**).
   - `BEHAVIOR_CONTRACT.md:501`'s `wasm.` *"NEW SN9 flattening rule"* colliding with ADR-0118's actual SN9; and `internal/stats/name.go:350`'s already-wrong error enumeration.
   - ⚠️ **`STATE.md` §Project counts is stale on THREE axes** (§1.7g), of which the router flags one.
   - ⚠️ **A live port-convention contradiction**: `next-prompt.txt` §16 says next-free reference port **10119** (family-banded `10<index>`); `STATE.md:32` says *"`10450`, NOT `10118` — ports are NOT fixture-index aligned."* Executed: `0117/envoy.yaml:56` ⇒ `10117`, `0118/envoy.yaml:90` ⇒ `10118` — **the index-aligned convention is what actually landed.** Row 78 adds no fixture, so this is informational only.
7. ⚠️ **Two load flakes UNINDEXED for the NINTH consecutive stage** — `internal/httpclient TestOptions_ZeroValue_NoOpDefaults` and `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery`. Both symbols verified present at this tip; both carried as prose across four phases. The SDS memory's own argument — *two instances in two packages justifies a fix rather than indexing a third* — applies.

---

## 8. Known live hazards — never reflex-classify any of these as a regression

⚠️ **A stage brief's flake list is not the index.** The index is `/home/esa/.claude/projects/-home-esa-git-envoy-go/memory/`; agent D re-derived it (`ls | grep -i flake` ⇒ exactly four files) and verified every named symbol exists at this tip.

| # | flake | evidence · what to do |
|---|---|---|
| 1 | **Full-suite startup flake** — `subject ready: EOF` **and** `bind: address already in use` | Both fail **before any assertion**; the latter as a **PANIC that can abort the whole binary**. Fires more readily under `-race` and as the fixture count grows — now **120**. Root-caused and hardened at `0e9cc680` (`freeTCPPortBlock` 20000..31007 + `startSubjectWithRetry`); ⚠️ **a post-hardening recurrence warrants suspicion, not acceptance.** Observed live at phase-76 G7, aborting at `0082` after 254 s |
| 2 | **SDS `init_fetch_timeout` dial-budget** | ⚠️ **TWO packages, which the SPEC does not say**: `internal/xds TestProvider_FetchInitialCertificate_Timeout` **and `internal/boot TestSDSEndToEnd_FetchFailure_BootFailsClosed`**. Reproducible on master: `-race -count=20` under load ⇒ **3/20**. ⚠️ **`internal/boot` is the direct callee of the line this row edits (`main.go:316`)** — a red there is far likelier to be mis-classified as a regression |
| 3 | **`internal/cluster` `-race` outlier** | ⚠️ **Name it: `TestOutlierDetector_ConcurrentEjectExactlyOnce`** (§1.7e). PRE-EXISTING; isolate-re-run. An *unrelated* `internal/cluster` `-race` failure must **not** be laundered through this exemption |
| 4 | `internal/httpclient TestOptions_ZeroValue_NoOpDefaults` | ⚠️ **STILL UNINDEXED** — no memory file; prose-only across four phases |
| 5 | `internal/filter/hcm/h2 TestServerConn_TinyWindowDelivery` | ⚠️ **STILL UNINDEXED** |
| — | `0061-lb-ring-hash` spread | ✅ **RESOLVED** at phase 76 (`sourceIPs` 4→16, P≈7.0e-8). ⚠️ **A spread failure there is now a FINDING, not a flake** |

**Protocol:** isolate-re-run, then state the classification **and its evidence**. `-count=1` always.

---

## 9. Operative memories

`reference_probe_must_discriminate` (necessary but **not sufficient** — every probe in the SPEC's headline discriminated perfectly along the axis it varied while holding the decisive axis fixed) · `reference_independent_probes_can_share_a_blind_axis` · `reference_probe_input_is_a_claim` (T1 Step 5's sink-less control **cannot fail** — it is not a negative result) · `reference_empty_output_is_not_a_zero_result` (measure the config bytes) · `reference_timeout_exit_124_shared_by_healthy_and_hung` (only OUTPUT discriminates; §1.7d) · `reference_liveness_break_needs_failing_baseline` · `reference_fatalf_makes_assertions_unreachable` (T3 leg 1 is the sole `Fatalf`; arm ζ proves why) · `reference_deliberate_break_wrong_assertion` (T5 Step 4's distinct messages) · `reference_ordered_assertion_legs_vacuous_on_constant_change` · `reference_gate_command_negative_control` · `reference_harness_exit_code_is_not_command_exit_code` (capture `INNER_EXIT`) · `reference_differential_break_protocol_count1` · `reference_differential_run_selector` · `reference_stale_cite_recurs_fix_by_pattern` (the `esalaine` sweep needs the **bare** pattern) · `reference_a_drift_correction_is_itself_a_claim` · `reference_verification_table_launders_wrong_cites` · `reference_deferred_candidate_cost_restale` (re-derive 10 at the IMPL tip) · `feedback_brief_citations_not_evidence` · `reference_sentinel_matcher_string_self_clears` (T10 Step 1) · `reference_next_prompt_tracked_despite_gitignore` (locate commits by SUBJECT) · `reference_bash_cwd_reset_commits_to_main` (⚠️ **fired again this session — the FOURTEENTH consecutive**) · `feedback_git_worktrees` · `feedback_execution_style` · `feedback_subagents_no_push` · `reference_parallel_subagents_private_scratch` · `reference_parallel_agents_shared_machine_namespaces` · `feedback_pertask_gofmt_lint` · `reference_break_protocol_commit_first` · `reference_full_suite_race_after_background_mutator` (⚠️ `0098-stats-sink-statsd-tcp` is the tree's only stats-sink background mutator).
