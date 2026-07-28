# PROGRESS 78 — boot-panic-visibility

Companion to `PLAN.md`. The PLAN half below is written at the PLAN stage; the `# IMPL record` half is appended at the phase-78 IMPL.

---

# PLAN record (2026-07-27)

**Stage:** PLAN. Lifecycle-state **2 → 3**. **ROW 78 STAYS `in-progress`.**
**Base:** master `6905c9ed`. Worktree `/home/esa/git/envoy-go-wt-p78plan`, branch `phase-78-plan`.
**File set:** `PLAN.md` (NEW) + `PROGRESS.md` (NEW) + `STATE.md` + `next-prompt.txt`.
**BYTE-UNTOUCHED at this stage:** `ROADMAP.md` · `BEHAVIOR_CONTRACT.md` · `DECISIONS.md` · **ZERO `.go` files.**

**Dispatch:** four investigation agents on disjoint remits, each with a **private detached worktree**, **private scratch** and a **banded port range** (A 23000-23099, B 23100-23199, C 23200-23299, D 23300-23399; controller 22000-22099) — plus controller re-derivation of every load-bearing claim **including an independent cross-product execution**. No docker. Nothing was torn down by image or ancestor filter. All four agent worktrees removed at close; `git worktree list` verified.

## What was EXECUTED at this stage

| # | thing | outcome |
|---|---|---|
| 1 | The production change, built **three times independently** (A, B, C) | `gofmt` output empty · `go build` · `go vet` · `golangci-lint` all clean |
| 2 | Agent A's **24-cell cross-product** `{pre, mid, post} × {sink, no sink} × {baseline, naive, pinned, alt-ii}` | pinned holds **6/6 discriminating cells, zero contradictions** |
| 3 | **Controller's independent cross-product** (band 22000, SIGKILL, inputs validated + byte-counted first) | naive+post+sink ⇒ **137 / panic ABSENT, 3/3**; naive+post+**no sink** ⇒ 2 (defect impossible); pinned ⇒ **2 in 0.011 s, panic PRESENT**, both sink states |
| 4 | The **SIGTERM rescue**, same binary + config as the hanging cell | `EXIT=124` at 8.006 s **with the panic text PRESENT** — reads as "printed, just slow" |
| 5 | The **mid-window** panic position the SPEC never tested (agent A) | baseline hangs there too: `137 / 0 B / 0 B`, sink present **AND** absent |
| 6 | The behavioural guard, **red-then-green** | RED leg 1 at 31.15 s (`stdout=0 bytes ""`, `stderr=0 bytes`); GREEN at 0.97 s |
| 7 | Both structural arms + a **six-cell negative-control matrix** | cells 3 and 6 (the load-bearing pair) behaved exactly as specified |
| 8 | The **eight-arm break roster**, seven of eight run | see §Break roster below |
| 9 | The **residual demonstration** (arm θ) | ⚠️ three green arms, green package suite, **12-second silent wedge at 0 B / 0 B** |
| 10 | The SIGTERM shutdown path with a **live UDP receiver**, 4 trees × 3 reps | **12/12 identical**, `EXIT=0`, **152 datagrams / 5888 bytes** every arm, stdout+stderr byte-identical by `diff` |
| 11 | Twelve gates, **each with its negative control** | G1-G12; one new broken-gate shape found (§1.6) |
| 12 | All three sentinel checks, **twice independently** | does NOT fire; `stop` NOT created |
| 13 | Every documentary anchor in the SPEC's read-first list | **one SPEC self-contradiction, four refuted claims, two missed prose sites, one missed build site** |

## THE HEADLINE: THE SPEC CONTRADICTS ITSELF, AND THE PLAN RESOLVES IT BY RETRACTION

SPEC §3.1 **permits** a trailing sibling `defer cancel()` as a taste alternative. SPEC §3.7 **mandates** a guard asserting the `cancel` call is *inside the same function literal* as the receive. Two agents built the alternative independently; it is behaviourally correct (`EXIT=2` in 0.0094 s) and the SPEC's own mandated guard goes **RED** on it.

⇒ **the PLAN RETRACTS §3.1's permission.** The single-literal `cancel()`-first form is the only accepted shape, and T5's arm is what enforces it. The bounded-`select` alternative is also refuted on its own terms by measurement: **5.012 s of dead wall-clock on every panic path.**

## FOUR FURTHER SPEC CLAIMS REFUTED

1. **The §3.8 prose denominator is NINE candidate sites, not seven.** The SPEC misses `:365-367` — which sits **physically adjacent to the relocation anchor** and becomes `flusherDone`'s only remaining stale comment after `:289-297` is rewritten — and `:396`, which SURVIVES and must be recorded as checked so a later sweep does not "correct" a true comment.
2. **The blast radius has a THIRD build site.** `test/conformance/h2spec/h2spec_test.go:210` also compiles and spawns `./cmd/envoy-go`. Named in **no** phase-78 document. The project's conformance claim is h2spec 53/53.
3. **`cmd/envoy-go` declares FIVE helpers, not four** (`waitForReadySentinels`, `acceptEcho`, `freeTCPPort`, `buildBinaryOrSkip`, `pkiFixture0002`).
4. **SPEC §13's exit table says DECISIONS `17531` "at this close".** The value at that close is **17560**; 17531 is the pre-commit figure. `STATE.md` and `next-prompt.txt` both have it right — only the SPEC's own table is wrong.

## A THIRD COUNTER-EXAMPLE IN THE R5 FAMILY, WHICH NO PHASE-78 DOCUMENT CARRIES

SPEC §1.1 leaves stderr as `—` for the hanging cell. Measured — by the controller and independently by agent A — the naive tree with a post-anchor panic and a stats sink emits **564 / 705 / 846 / 987 / 1122 / 1269 bytes of stderr** while hanging, growing with wall-clock, panic text **ABSENT**. Composition:

```
$ sed 's/[0-9]//g' hang.err | sort | uniq -c
      4 // :: statssink: statsd udp write failed, dropping line: write udp ...:->...:: write: connection refused
```

⇒ **an assertion of the form "stderr is non-empty ⇒ the failure was visible" is GREEN on a tree that hangs forever having reported nothing.** R5 proves output-only and exit-status-only each insufficient; this adds output-VOLUME. **Only the panic TEXT discriminates.**

## A NEW GATE HAZARD — 13 → 14

`.golangci.yml` enables **`misspell` with `locale: US`**. Agent C's first draft was flagged 3× (`behavioural` ×2, `CANCELLED`). **The phase-78 SPEC uses "behavioural" throughout.** ⇒ the IMPL must not paste SPEC prose into `.go` comments.

## THE COST, RE-DERIVED UPWARD: 7–9 → **10**

The SPEC's calibration sentence is imprecise: phase 76's ~5–7 was its **BRAINSTORM**; its SPEC already said 7–9 and its PLAN shipped **9** — the *ceiling*, not a miss. The load-bearing pattern is the opposite: **76: SPEC 7–9 → PLAN 9. 77: SPEC 11–13 → PLAN 12.** Two for two at the ceiling.

Phase 78 lands **one above** it, for two executed reasons: the floor of 7 requires folding a now-larger five-region prose sweep into a nine-line code change (a prose regression would ride in on a green *build* gate), and T4 splits into T4+T5 because **the ordering arm is GREEN on the tree that still hangs** while the coverage arm is the one that catches it.

⚠️ Agents A (8–10) and D (9–10) agreed independently — **but both read the same SPEC, so their agreement is not evidence.** Re-derive at the IMPL tip.

## Break roster — eight arms, seven RUN at this stage

| arm | edit | leg that fired | verdict |
|---|---|---|---|
| α | revert-the-relocation | T3 leg 1 (31.00 s, `stdout=0 bytes ""`); T4 ordering leg at `main.go:299` | ✅ |
| β | delete-the-`cancel()` | **T5 only** — *"NO cancel() call in this literal at all"* | ✅ **T3 and T4 stay GREEN — the load-bearing pairing** |
| γ | `cancel()` after the receive | T5, *"the only cancel() call is at line N, AFTER the receive"* | ✅ a **distinct** message from β |
| δ | print-then-hang | T3 leg 1 (30.98 s) — **and legs 3/4a/4b each TRUE** against its 141-byte stderr | ✅ R5's output half, both halves shown |
| ε | trigger → get-or-create | T3 leg 1, `stdout=109 bytes "envoy-go listener l_a ready on 127.0.0.1:32803"` | ✅ not vacuous when the trigger stops triggering |
| ζ | trigger → clean reject | **legs 2, 3, 4a, 4b — all four, one 1.36 s run** | ✅ proves the `t.Errorf` discipline itself |
| η | structural rename | **both** anti-vacuity `t.Fatalf`s | ✅ neither arm silently passes |
| θ | a DIFFERENT blocking defer | ⚠️ **NOTHING went red** | see below |

⚠️ **Two arms are AST-IDENTICAL**: β and "the naive relocation as the BRAINSTORM specified" differ only in **comment text**, invisible to `go/parser` in mode `0`. One structural case, not two.

### Arm θ — the most valuable line in the roster, and it is a NEGATIVE result

On the fully-fixed tree, `defer func() { <-make(chan struct{}) }()` inserted before `boot.Construct`:

```
structural: AfterEveryClose=PASS  PrecededByCancel=PASS
behavioural: PASS      whole package: 10/10 PASS
binary:     EXIT=137  wall=12.001s  stdout=0B  stderr=0B  panic-present=0
```

**Three green guards, a green suite, and a totally silent 12-second wedge** — quieter than the original defect, because the relocated defer's `cancel()` runs first and silences the flusher's error log before the process wedges on the sibling defer. ⇒ SPEC §3.4's detector is a **METHOD, not a shipped test.**

### The demoted arm

The SPEC's **healthy-boot** arm is a demonstration, not a test — asserting "leg 1 fires on a healthy server" is a tautology and adds a 30-second sleep. Its content, measured through the exact harness shape:

| arm | `ctx.Err()` | `runErr` | `ExitCode()` | stdout | stderr |
|---|---|---|---|---|---|
| healthy boot, fixed binary | `context deadline exceeded` | `signal: killed` | **-1** | **109 B** | 1410 B |
| baseline (hung), trigger cfg | `context deadline exceeded` | `signal: killed` | **-1** | **0 B** | **0 B** |
| fixed binary, trigger cfg | `<nil>` | `exit status 2` | **2** | 0 B | 2302 B |

**Rows 1 and 2 are byte-identical on every status observable.** Leg 2 is insufficient through **blindness**, not satisfaction-by-success — a sharpening of SPEC R5. `ctx.Err()` alone cannot separate them either; only stdout can, which is why T3 leg 1's failure message prints `stdout.Len()` and the first line.

## Counts re-derived at this tip, each with a negative control

fixtures **120** (next-free `0119`; `discoverFixtures` predicate `^[0-9]{4}[a-z]?-` verified against `runner_test.go:1461-1497`, **not drifted**) · fuzzers **55** (⚠️ the `internal/`-vs-repo-wide pair is **not** discriminating — both 55) · internal packages **73** · blank imports **120** (naive `^\t_ ` ⇒ **126**; the six extras enumerated) · BackendKind **tail 38** (39 constants, `TCPEcho = 0`) · go.mod clean, single `go.mod` requires **67** · ROADMAP **226 lines / 110 data rows**, `want=110` **STAYS** · BEHAVIOR_CONTRACT **5750** · DECISIONS **17560** · tail **ADR-0300 PROPOSED**, next-free **ADR-0301**, block shape **1 Context / 0 Decision / 0 Consequences / 1 footer / 0 `---`**, footer is the **last line of the file** · stat surface **1207** DOCUMENTARY.

⚠️ **The `+0` stat-surface claim cannot be discharged by a `TestNoNewStat*` run** — all five live in `internal/statssink/registration_test.go` and guard **that package**. The argument is **structural**: `main()` registers no stats and the diff is confined to it.

## Sentinel — re-run MECHANICALLY, twice, at this stage. It does NOT fire; `stop` was NOT created

- **(1)** `NOT DONE: row 78` — correct and intended until the IMPL. **No `GATE FAIL`** ⇒ `want=110` holds and **STAYS**.
- **(2)** **FIVE, UNCHANGED** — `:188 :198 :208 :214 :222`. **This row narrows NOTHING**, and that is **stated, not forecast**.
- **(3)** `NEVER OPENED: gRPC` and `NEVER OPENED: WASM`.

Negative controls: `want=109` ⇒ `GATE FAIL: examined 110 data rows, expected 109`; a scratch copy with row 77 flipped ⇒ names row 77; an invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`; input measured (994 044 B / 5 hits vs an empty control at 0 B / 0 hits).

⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage**, so no matcher string can have leaked.

## Hygiene

- Every git command used `git -C <abs-worktree-path>`. ⚠️ **The Bash cwd reset fired AGAIN — the FOURTEENTH consecutive session** (`Shell cwd was reset to /home/esa/git/envoy-go`), observed live twice.
- Four agents ran concurrently with private scratch, private detached worktrees and banded ports; no collision; all worktrees removed; canonical repo verified at `6905c9ed` with only `?? .claude/`.
- Zero pushes by any agent. Zero commits outside throwaway worktrees. No unscoped `git restore` anywhere.

---

# IMPL record (2026-07-27)

**Lifecycle-state 3 → 4. Row 78 flips `in-progress` → `done`.** Ten tasks T1–T10 executed subagent-driven across four worktrees (one stage worktree on `phase78-impl`, two docs worktrees merged in, one detached gates worktree at the T5 tip so the break arms could not corrupt the differential run). Every figure below was measured at this stage's own tip. **A prior stage's red is not this stage's red** — all eight break arms were re-run here.

## Shipped

| task | file | commit |
|---|---|---|
| T1 | `cmd/envoy-go/main.go` — the relocated defer, `cancel()` first in its body | `54f410a2` |
| T2 | `cmd/envoy-go/main.go` — the prose sweep, six edits / four SURVIVE | `0d34aa23` |
| T3 | `cmd/envoy-go/main_test.go` — `TestMain_BootPanicIsVisible` | `3ce917c6` |
| T4 | `cmd/envoy-go/main_test.go` — `…_FlusherDoneWaitIsAfterEveryClose` | `be01ce8a` |
| T5 | `cmd/envoy-go/main_test.go` — `…_FlusherDoneWaitIsPrecededByCancel` | `7436c0b3` |
| T7 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the boot-failure-visibility subsection | `8e71359f` |
| T8 | `docs/envoy-go/DECISIONS.md` — ADR-0300 §Decision + §Consequences | `5add562a` |

Production diff vs master: **`cmd/envoy-go/main.go` only, 44 insertions / 24 deletions** (T1 26/15, T2 18/9). `go.mod`/`go.sum` **byte-clean**. The shipped literal:

```go
	defer func() {
		cancel()      // release the wait: a panic must never leave this blocked on a flusher
		<-flusherDone // wait for the Flusher goroutine to stop Submitting before closing the sink channels
		for _, s := range statsSinks {
			_ = s.Close()
		}
	}()
```

Final line geometry, from the shipped guards' own `t.Logf` (**not** the PLAN's guesses of `[373]` / `[353 355]`): `<-flusherDone` receive at **`:386`**, `close(flusherDone)` at **`:361`** and **`:363`**, `cancel()` at **`:385`**.

## T1 — the fix proven by execution, with the control that makes it non-vacuous

Probe configs validated FIRST (`--mode validate` ⇒ `configuration OK`, exit 0) and byte-counted — **1628 B** with a statsd sink, **1351 B** without — so the greens are measured over real input.

| tree | arm | EXIT | wall | stdout | stderr | panic text |
|---|---|---|---|---|---|---|
| **fixed** | sink | **2** | 0.0139 s | 0 B | 129 B | **PRESENT** |
| **fixed** | no sink | 2 | 0.0123 s | 0 B | 129 B | PRESENT |
| naive (control) | sink | **137** | **8.002 s** | 0 B | **987 B** | **ABSENT** |
| naive (control) | no sink | 2 | 0.0115 s | 0 B | 129 B | PRESENT |

⚠️ **The naive control was added beyond the PLAN's step list, and it is what makes the result mean anything.** Step 5 as literally written exercises only the fixed tree plus the no-sink shape — and the no-sink shape exits 2 on *every* tree, because the `else` branch pre-closes `flusherDone` before the defer is registered, so the defect is **structurally impossible** there (`reference_probe_input_is_a_claim`). Anyone re-running Step 5 as written gets a green that does not discriminate.

**Normal shutdown is byte-identical, with a LIVE receiver: 6/6 arms** (3 reps × {baseline, fixed}) — `EXIT=0`, stdout **62 B**, stderr **121 B**, **190 datagrams / 7260 UDP bytes** on every single arm, stdout/stderr identical by `diff`. The datagram count is non-zero, so this is not an empty-input artifact. (The PLAN's 152/5888 is a different harness window, not a contradiction.)

## T2 — the nine-site prose sweep

Six edits landed (`:198`, the pre-existing `:201` "both"-vs-five drift, `:326-329`, `:365-367`, `:370`, `:393`). **The four SURVIVE sites, recorded with the reason each survives** — this list exists because a brief-driven sweep would have "corrected" a true comment (the R11 failure mode):

- **`:93`** — "operator-knob deferred per ADR-0095": an unrelated English use of "deferred"; no `defer` statement, no LIFO claim. Never a candidate.
- **`:112-113`** — the access-log defer is execution rank **6**, `lm.Stop` rank **2**; both clauses hold post-move.
- **`:290-294`** (was `:304-308`) — **R11**: tracing `CloseAll` still runs after `lm.Stop()` (rank 5 vs 2) and before the access-log sinks close (rank 5 vs 6). Both clauses hold.
- **`:416`** (was `:396`) — generic and true; names no order and no participant.

Verification: `git diff -U0 … | grep -cE '^[-+].*(Phase 46\.1b|no new spans|sinks per ADR-0069|Existing deferred-stop)'` ⇒ **0**, against both the T1 tip and master; a `:93` byte-check ⇒ **0**. All four appear in no diff hunk.

⚠️ **PLAN §1.3's "13 comment lines" is off by one — the grep returns 14.** The *grouping* is unchanged (9 candidate sites + `:93`), so the denominator and every classification hold.

## The guards — red-then-green at THIS tip

**The input was proven before the red was trusted.** The same trigger config (2565 B) run under `--mode validate` on a pre-T1 binary: `EXIT=2`, **2438 B** on stderr, first line `panic: stats: duplicate metric registration: "http.dup_prefix.downstream_rq_total"`, stack `internal/stats/registry.go:107` ← `registry.go:86` ← `internal/filter/hcm/config.go:358` ← … ← `validate/validate.go:49`. **This isolates the trigger from the defer**: same config, same binary, one entry path prints 2.4 kB and exits 2, the other prints zero bytes and never dies.

| arm | RED | GREEN |
|---|---|---|
| T3 behavioral | leg 1, **31.10 s**, `stdout=0 bytes ""`, `stderr=0 bytes`, `run error=signal: killed` | **1.02 s** |
| T4 ordering | ordering leg at `main.go:299`, `closes [368 370]` | PASS, receives `[386]`, closes `[361 363]` |
| T5 coverage | *"there is NO cancel() call in this literal at all"* while **T4 and T3 stayed GREEN** | PASS, receive `:386` preceded by `cancel()` at `:385` |

`no tests to run` = **0** on every one of the fifteen `-run` invocations across T3–T6.

## Break roster — **all eight arms RE-RUN at this tip**

Baseline before arm α: `ok … 9.203s`. Tree confirmed green between every arm.

| arm | fired | evidence |
|---|---|---|
| **α** revert-the-relocation | T3 leg 1 (31.09 s, `stdout=0 bytes ""`) **+ T4 ordering leg + ⚠️ T5, both legs** | ⚠️ **three arms red, not the map's two** |
| **β** delete the `cancel()` | **T5 only** — *"NO cancel() call in this literal at all"*; T3 **PASS (1.07 s)**, T4 **PASS** | the load-bearing pairing HOLDS |
| **γ** `cancel()` after the receive | T5, *"the only cancel() call is at line 386, AFTER the receive"* — textually **distinct** from β | the two mistakes are distinguishable |
| **δ** print-then-hang | T3 leg 1 (31.17 s); **legs 3, 4a and 4b each evaluate TRUE** against its **141 B** stderr | an output-only assertion **PASSES** here |
| **ε** trigger → get-or-create | T3 leg 1, `stdout=109 bytes "envoy-go listener l_a ready on 127.0.0.1:33497"` | leg 1 discriminates its two red modes |
| **ζ** trigger → clean reject | **legs 2, 3, 4a, 4b — all four in ONE 1.02 s run**, count verified = 4 | ⚠️ the `t.Errorf` discipline itself: a `Fatalf` on leg 2 would have made legs 3/4 dead code |
| **η** structural rename | **both** anti-vacuity `t.Fatalf`s (T4 *"found ZERO … receive expressions"*, T5 *"found ZERO deferred function literals …"*); T3 PASS | neither structural arm passes silently on a rename |
| **θ** the residual | ⚠️ **REFUTED AS SPECIFIED — see below** | |

### ⚠️ ARM θ: THE PLAN'S DEMONSTRATION IS REFUTED, AND THE RESIDUAL IS **POSITION-DEPENDENT**

PLAN §4 recorded θ as *"insert `defer func() { <-make(chan struct{}) }()` before `boot.Construct` ⇒ NOTHING goes red."* Executed here, **that is false**, and the correction is worth more than the original claim.

**θ-1, the PLAN's stated placement (before `boot.Construct`) — SOMETHING DOES GO RED:**
```
--- FAIL: TestMain_BootPanicIsVisible (31.08s)
    BOOT DID NOT TERMINATE within 30s: stdout=0 bytes "", stderr=0 bytes, run error=signal: killed
--- PASS: TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose (0.00s)
--- PASS: TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel (0.00s)
whole package: PKG_EXIT=1, FAIL … 218.894s
```
**Mechanism — coincidence of position, not coverage.** T3's trigger panics *inside* `boot.Construct`, so a defer registered above that call sits on that panic's own unwind path and swallows it. The arm catches this case by accident of where the single trigger lives.

**θ′, a POST-anchor placement (after `defer lm.Stop()`) — NOTHING goes red, and this is where the residual actually lives:**
```
--- PASS: TestMain_BootPanicIsVisible (1.09s)
--- PASS: TestBootPanicVisibility_FlusherDoneWaitIsAfterEveryClose (0.00s)
--- PASS: TestBootPanicVisibility_FlusherDoneWaitIsPrecededByCancel (0.00s)
whole package: PKG_EXIT=0, ok … 188.688s     (baseline 8.7 s)
```
Three green arms, a green package — and **every graceful-shutdown path in that package silently wedging to its own deadline**, reported as `ok`. Per §Context there is **no config-reachable post-anchor panic**, so nothing can drive a behavioral probe there.

**Two further PLAN claims refuted along the way:**
1. **The stated CAUSE of the zero-byte silence is wrong.** It is not *"the relocated `cancel()` runs first and silences the flusher's error log"* — at a pre-anchor panic the flusher was never started and the relocated defer was never registered. `cancel()` plays no part.
2. **"Boot it on a VALID config and expect 0 B / 0 B" is self-contradictory.** On a valid config a healthy `main()` blocks at `<-ctx.Done()` and never unwinds, so the injected defer never runs: measured `EXIT=137`, **109 B stdout / 1269 B stderr** — a healthy server being killed, not a wedge. The 0 B / 0 B figure is reproducible **byte-for-byte**, but only on the **trigger** config, which is the RED case. Also measured: the trigger config does **not** pass `--mode validate` (exit 2, 2391 B, panic printed) — only a distinct-prefix config does.

⇒ **SPEC §3.4's detector remains a METHOD, not a shipped test**, and ⚠️ **a future row applying it must VARY THE INSERTION POINT across the pre- and post-anchor windows** — a single pre-anchor injection reports "caught" and conceals the case that is genuinely uncovered. ADR-0300 §Consequences (ii) carries the corrected form. **Not fixed in this row.**

### The demoted arm — measured here, not inherited

Harness: `exec.CommandContext` + `context.WithTimeout(30s)`, `cmd.Cancel` **not set** ⇒ SIGKILL.

| arm | `ctx.Err()` | run error | `ExitCode()` | stdout | stderr | panic text |
|---|---|---|---|---|---|---|
| 1 healthy boot, fixed binary, valid cfg | `context deadline exceeded` | `signal: killed` | **-1** | **109 B** | 3102 B | false |
| 2 baseline (pre-T1, hung), trigger cfg | `context deadline exceeded` | `signal: killed` | **-1** | **0 B** | **0 B** | false |
| 3 fixed binary, trigger cfg | `<nil>` | `exit status 2` | **2** | 0 B | 2243 B | **true** |

**Rows 1 and 2 are byte-identical on every STATUS observable** — that is the load-bearing claim and it holds. ⚠️ **Two corrections to how the PLAN phrased it:** the rows do **not** differ "only in stdout" — they differ in stdout (109 B vs 0 B) **and** stderr (3102 B vs 0 B); the claim is true only when scoped to *status* observables. And row 1's stderr measured **3102 B** here against the PLAN's 1410 B — pure wall-clock-dependent noise volume, which is precisely why §1.2 forbids asserting stderr VOLUME.

Row 2's 0 B stderr is consistent with §1.2's 564–1269 B figure rather than contradicting it: §1.2 measures the **naive-relocation** tree with a **post-anchor** panic (flusher running); the pre-T1 baseline hangs **pre-anchor**, so no flusher ever starts.

## Gates — every one run at the T5 tip

| gate | result |
|---|---|
| `gofmt -l ./cmd/envoy-go/` | empty output ⇒ CLEAN (gated on OUTPUT — it never exits non-zero) |
| `golangci-lint run ./cmd/envoy-go/...` | exit 0, no diagnostics — the `misspell` locale-US hazard **not tripped** |
| `go vet ./cmd/envoy-go/` | exit 0 |
| `go test ./cmd/envoy-go/ -count=1 -v` | **INNER_EXIT=0**, `ok … 9.434s`, **11 top-level `--- PASS`**, 0 FAIL, 0 SKIP |
| `go test ./cmd/envoy-go/ -count=1 -race` | **INNER_EXIT=0**, `ok … 10.072s` |
| **full 120-fixture differential** | **INNER_EXIT=0**, **414 s**, `--- FAIL` 0, `--- SKIP` 0, `^panic:` 0 |
| ↳ denominator cross-check | `fixdirs.txt` **120** / `subtests.txt` **120**, **`comm -3` EMPTY**, 120/120 PASS |
| **`TestH2Spec`** (the third build site) | **INNER_EXIT=0**, `--- PASS (2.70s)`, **53 tests, 53 passed, 0 failed** — h2spec **53/53** holds |
| `git diff --stat master -- go.mod go.sum` | empty |

⚠️ **The eleven top-level tests were enumerated BY NAME**, not tallied: the 8 pre-existing (`TwoListenerCutover`, `HCMSmoke`, `StatsPrometheusEndpointResponds`, `TLSInspectorBootWiring`, `AccessLogSmoke`, `H2Smoke`, `FourNewAdminEndpointsRespond200`, `ModeValidate`) plus the 3 new arms. Including subtests the log carries 15 `--- PASS` lines — reported separately so the two denominators are not conflated.

**The ten stats-sink fixtures whose assertions ride the reordered chain, enumerated individually rather than totalled:**

| fixture | verdict | | fixture | verdict |
|---|---|---|---|---|
| `0089-stats-sink-metrics-service` | **PASS** 2.69 s | | `0094-stats-sink-dogstatsd-batching` | **PASS** 5.10 s |
| `0090-stats-sink-metrics-service-deltas` | **PASS** 4.74 s | | **`0098-stats-sink-statsd-tcp`** | **PASS** 5.12 s |
| `0091-stats-sink-metrics-service-labels` | **PASS** 2.72 s | | `0101-stats-sink-graphite` | **PASS** 5.15 s |
| `0092-stats-sink-statsd` | **PASS** 5.09 s | | `0112-stats-sink-otlp` | **PASS** 2.59 s |
| `0093-stats-sink-dogstatsd` | **PASS** 4.76 s | | `0113-stats-sink-otlp-knobs` | **PASS** 2.38 s |

`0098-stats-sink-statsd-tcp` is the tree's **only** stats-sink background mutator and the send-on-closed-channel exposure, so a `-race` pass over the ten was run as a **second** run, not a substitute (`reference_full_suite_race_after_background_mutator`).

### The one red, and its classification

**`-race` over the ten, attempt 1: `INNER_EXIT=1`.** Sole failure `TestDifferential/0089-stats-sink-metrics-service`; the other nine passed. `WARNING: DATA RACE` = **0**, `^panic:` = **0**.

```
listen: listen tcp 0.0.0.0:46501: bind: address already in use
runner_test.go:343: backend[0] not ready: waitTCPDial: 127.0.0.1:46501 did not become reachable within 5s
```

**Isolate-re-run:** the fixture alone under `-race -count=1` ⇒ **PASS** (3.43 s, 0 races); the full ten re-run under `-race -count=1` ⇒ **all ten PASS**, 0 races, **0** occurrences of `bind: address already in use`; and the same fixture PASSed in the 414 s full suite.

**Classification: PLAN §8 hazard 1 (bind-collision startup flake) — accepted, with the evidence that stops it being laundered:**
1. It failed **before any assertion** — the spawned backend died at its own `net.Listen`; no comparison logic ran.
2. It is **outside the `0e9cc680` hardened surface**, so it is *not* a post-hardening recurrence of the hardened path. Port 46501 is a kernel ephemeral from `freeTCPPort` (`harness_test.go:203`), used for **backends**; the hardening added `freeTCPPortBlock` (bases 20000..31007) and `startSubjectWithRetry`, which cover the **subject proxy only**. Backends have **no retry loop**, so a single collision is fatal.
3. It is **causally unreachable from this row's change**: the process that failed to bind is a fixture backend, spawned independently of and upstream from `./cmd/envoy-go`. A shutdown-ordering change inside `main()` cannot affect it.
4. A sibling agent was running concurrently, raising ephemeral-range contention.

Not hazards 2–5 (no SDS / `internal/boot` / `internal/xds` / `internal/cluster` / `internal/httpclient` / `h2` involvement) and not `0061` — which passed in the full suite, and where a spread failure would be a **FINDING**, not a flake.

## Counts, each with its negative control — all **+0** for this row

| axis | measured | control |
|---|---|---|
| fixtures | **120** | faithful predicate ⇒ 120 · complement `grep -vE` ⇒ empty · `^ZZZZ` ⇒ 0 |
| fuzzers | **55** | ⚠️ the `internal/`-vs-repo-wide pair is **NOT discriminating** (both 55) — the discriminating NC ran on a scratch **copy**: 55 → append `func FuzzProbeOnly` → **56** → delete → 55 |
| internal packages | **73** | — |
| blank imports | **120** (full `^\t_ "github.com/pgdad/envoy-go/test/fixtures/` prefix) | naive `^\t_ ` ⇒ **126** |
| BackendKind | **tail 38** — `H2GoawayResponder BackendKind = 38` at `fixture.go:614`, 39 constants, `TCPEcho = 0` | a TAIL VALUE; **not** "fixed" to 39 |
| go.mod | `git diff master -- go.mod go.sum \| wc -c` ⇒ **0** | the single `go.mod` requires **67** (18 direct + 49 indirect); the lineage figure **2** is phase-61.2's quic-go count, not a repo total |
| stat surface | **1207, +0** | ⚠️ **STRUCTURAL argument — see below** |

⚠️ **The `+0` stat-surface claim CANNOT be discharged by a `TestNoNewStat*` run, and the scoping was verified rather than assumed.** All **five** such guards live in `internal/statssink/registration_test.go` (`:26 :53 :81 :109 :137`), file header `package statssink`; their bodies assert `countMetrics(reg) == 0` against a fresh registry for **that package's** sinks. **None can reach `cmd/envoy-go`.** The available argument is **structural** and is stated as such: `git diff --name-status master` ⇒ exactly two files, both under `cmd/envoy-go/`; `main.go` declares exactly **one** function (`func main()` at `:38`), so all five diff hunks are confined to `main()` by construction; and `grep -rnE 'New(Counter|Gauge|Histogram)' cmd/envoy-go/` returns **only comment lines** — **no call site**. ⇒ delta **+0**.

## Byte-untouched roster — spanning BOTH trees

`internal/stats/` enumerated from **both** trees (19 paths each, identical): **`ROSTER: examined 19 path(s), rc=0`** — all byte-identical. The **ADR-0089 block**, extracted by content anchor: **89 lines / 10 679 B**, sha `733c1ce8…` on both sides ⇒ byte-identical. Extractor NC: an invented `^## ADR-9999` anchor yields **0 bytes**, proving the 10 679-byte match is a real match and not a silent empty (`reference_empty_output_is_not_a_zero_result`).

⚠️ **All three roster negative controls fired** — a roster built from one tree's `ls-files` is desynced **by construction** against a deleted file, and a naive `[ -f ] || continue` exits 0 on one:
- MODIFIED — `cmd/envoy-go/main.go` ⇒ `ROSTER FAIL: … MODIFIED (f6546383… -> 7c7ecfcf…)`, exit 1
- DELETED — a scratch **copy** with `internal/stats/name.go` `mv`'d aside ⇒ `ROSTER FAIL: … DELETED in worktree`, exit 1
- ABSENT — a nonexistent path ⇒ `ROSTER FAIL: … ABSENT at baseline`, exit 1

## Corrections this stage owes forward

1. ⚠️ **ARM θ AS THE PLAN SPECIFIED IT IS REFUTED** — the residual is position-dependent; the pre-anchor injection is *caught*, the post-anchor one is not, and the PLAN's stated cause and its "valid config ⇒ 0 B / 0 B" recipe are both wrong. Above, and in ADR-0300 §Consequences (ii).
2. ⚠️ **The package test count is ELEVEN, not ten.** PLAN §1.9 and T5 Step 6 say "10/10"; §1.7a's own figure of 8 pre-existing + 3 new arms gives **11**, and 11 is what was counted by name. T9 Step 1 has it right; the other two are off by one.
3. ⚠️ **`α` is a THREE-arm red, not two.** PLAN §4 lists T3 + T4; **T5 fires as well**, on both legs — consistent with §4's own note that β and the naive relocation are AST-identical, but the map understates it.
4. ⚠️ **The raw differential `--- PASS` tally is 121, not ~134.** The PLAN warns the raw count "exceeds 120 … phase 77 measured 134", but that describes an *unfiltered* package run; the prescribed command carries `-run TestDifferential`, which excludes the 15 siblings, leaving 120 subtests + 1 parent. The `comm -3` cross-check is the load-bearing gate and it is clean.
5. ⚠️ **PLAN §1.3's "13 comment lines" is 14** — grouping and denominator unaffected.
6. ⚠️ **T4's predicted line numbers were wrong**, as the PLAN itself flagged they might be: `[386]` / `[361 363]`, not `[373]` / `[353 355]`.
7. ⚠️ **`BEHAVIOR_CONTRACT.md` carries 14 `### Does not yet apply to` headings, not the PLAN's 15.** Immaterial — the parenthesised anchor is unique (1) and was used — but the numeral must not be inherited. It is the same whole-file-count species the PLAN itself warns about, appearing inside the PLAN.
8. ⚠️ **ADR-0299's STATUS line still reads `PROPOSED`** although its §Decision and §Consequences landed at the phase-77 IMPL, and its headings lack the `(landed at the phase-N IMPL)` suffix ADR-0295–0298 carry. ADR-0300 therefore mirrors **ADR-0298** (the most recent genuinely-COMPLETE row, and also a maintenance row claiming no family ordinal). **Recorded, not fixed** — that block is not this row's to touch.
9. ⚠️ **PLAN Task 9 Step 5's third roster element** — `main.go`'s SURVIVE comment sites by content grep — belongs to T2's sweep and was discharged there (Step 9, ⇒ 0 against both baselines), not by the T9 roster. Flagged so it is not assumed covered twice.
10. `go build ./cmd/envoy-go/` drops an untracked `envoy-go` binary in the worktree root; explicit-path staging kept it out of every commit, but the gate recipe should use `-o`.

## Deferrals — carried forward UNCHANGED, none fixed

All seven of PLAN §7 stand, and **this row narrows NOTHING**: `AsyncFileSink.Close` (shutdown-class) · the FIFO `os.OpenFile` boot block at `internal/accesslog/writer.go:56` (pre-defer, unreachable by this fix) · **the position-dependent residual** (now better characterised, still unfixed) · the `/stats/prometheus` projection-completeness gap, **UNBLOCKED by this row** · whether duplicate `stat_prefix` should be rejected or aggregated — ⚠️ **there is exactly ONE config-reachable in-window panic trigger and NO fallback**, so a future row that legislates it away must **RE-POINT T3's trigger, not relax its assertion** · the documentary defects (the non-existent `esalaine/envoy-go` import path — fix by the **bare** pattern, since ADR-0006 at `DECISIONS.md:142` has no `/validate` suffix; the documentary 1207 stat surface; the unaudited stale-cite half; `BEHAVIOR_CONTRACT.md:501`'s SN9 collision; `STATE.md` §Project counts stale on three axes) · the two **still-unindexed** load flakes, now a **tenth** consecutive stage as prose only.

## Sentinel — re-run MECHANICALLY at this stage, before and after the ROADMAP edit

Recorded in full at the stage close below. **It does NOT fire; `stop` was NOT created and must not be.**

## Hygiene

Fresh worktrees off master (`feedback_git_worktrees`); subagent-driven (`feedback_execution_style`); every subagent committed **locally only** (`feedback_subagents_no_push`); controller squash-pushes at close (`feedback_push_to_origin`). Parallel agents held **private scratch** and **disjoint port bands** (19000-19099 for the break arms, deliberately **below** the differential harness's 20000-31007 allocation floor so a manual bind could not manufacture a false red in the sibling's suite), staged by **explicit path**, and ran no unscoped `git restore`. ⚠️ **`reference_bash_cwd_reset_commits_to_main` fired again — the FIFTEENTH consecutive session** — on a plain read-only command block; every git command used `git -C <abs-path>`.
