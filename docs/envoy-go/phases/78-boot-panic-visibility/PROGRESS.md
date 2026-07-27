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

# IMPL record

*(appended at the phase-78 IMPL)*
