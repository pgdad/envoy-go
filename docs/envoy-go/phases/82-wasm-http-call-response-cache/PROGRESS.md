# PROGRESS 82 — wasm-http-call-response-cache

*(Per-stage records. The phase-82 BRAINSTORM authored no `PROGRESS.md`; this file is created at the SPEC and carries no retro-fitted BRAINSTORM record — writing one for a stage this session did not run would manufacture a record. The BRAINSTORM's own account is `BRAINSTORM.md`.)*

---

# SPEC record (2026-08-01)

**Stage:** SPEC (lifecycle-state `1` -> `2`). Base master **`61f4f5a3`** (`git rev-parse master`), branch `phase-82-spec`. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.** `ROADMAP.md` **BYTE-UNTOUCHED**, sentinel `want` **STAYS 114**, row 82 **STAYS `in-progress`**.

## What landed

`SPEC.md` (NEW) · `PROGRESS.md` (NEW, this file) · `DECISIONS.md` **17796 -> 17824** (ADR-0304 §Context, a pure tail append; **prefix proven byte-identical by `head -17796 | cmp`, so ZERO existing line cites shifted**) · `STATE.md` (§Current rolled IN PLACE, §Recent add + evict) · `STATE_HISTORY.md` · `next-prompt.txt`.

## Method

Five investigation agents on disjoint remits — four in **DETACHED** worktrees off `61f4f5a3` with private scratch and private port bands inside `42100-42499`, one read-only over the primary tree. **Every load-bearing claim was re-derived by the controller**, and one agent claim did **not** survive that re-derivation (SPEC §2.11). Zero commits, zero branches, zero pushes by any agent; all four worktrees ended at `git status --porcelain` = **0 lines**, controller-re-confirmed, and `docker ps -a --filter name=p82s` ended **empty**.

## The stage's headline

**The row's chartered premise is refuted in four independent ways, and the corrected shape is larger and order-constrained.**

1. **The reference emits nothing, not `200`** — it honors `Action::Pause` and the vendored guest never resumes. Two agents, two different discriminating designs.
2. **The subject's `0` is the guest's INITIAL value** — `proxy_on_http_call_response` is never invoked (`http_call_response 0` / `after_close 20` over 20 requests; a `777` sentinel control emits `777`).
3. **The mask hides a TRAP, not a wrong value.** The vendored `proxy-wasm-rust-sdk` v0.2.4 `get_map` has **no `NotFound` arm** — it `panic!`s. ⇒ honoring Pause without a correct cache converts a benign wrong header into a guest trap plus a BUG-3 poison cascade. **This inverts the row's risk profile and forbids one of the two split orders.**
4. **The header-map enum is swapped pairwise.** envoy-go has HttpCallResponse at 4/5 and Grpc metadata at 6/7; canonical is the reverse, and **the guest asks for 6**. Wiring cases at 4/5 exactly as the BRAINSTORM prescribes would ship the row **vacuous**. Pinned in place by a hand-written golden that shares the author's mistake.

## Refutation count: **TWENTY-THREE**, of which **TEN are load-bearing**

Load-bearing: the reference hang · the initial-value observable · the trap-not-wrong-value synthesis · the swapped enum · `resp.Header` carries no `:status` · key-count vs value-count · the callback-scoped (not stream-scoped) stash · the (l) stats arm being invariant under the row's own change · `0036`'s cross-side stream being 100% constants · `+0 new PUBLIC surface` not surviving.

Structural/numeric: `headerMapForType` handles 0 and **2** · four banked router counts wrong (phase dirs **123**, `STATE_HISTORY` tolerant **170**, ROADMAP cites **118**, plus the **ADR-0209 gap** no document records) · `:212` is a 45,959-char line with **six** occurrences · `:226` uses the SHORT form · the three cpp-host cites · cancel-at-destruction as a departure · the wrong `plugin_context_id` · the swallowed trap · no body cap · `docs/superpowers/plans/` has no file of that name · the `OnDestroy` lore cite stale by +1 · an agent refutation that did not survive re-derivation.

## Gates — a docs-only SPEC owes (a)-(f) only in the posture a docs-only stage can have

- **(a)/(b)** differential — **INAPPLICABLE**: zero `.go` committed. Measurement runs were made and reverted; `0036` as-is **PASSED in 37.91 s** and the flip **FAILED as designed**. Not claimed as a gate.
- **(c)** conformance — **INAPPLICABLE** for this row. ⚠️ When re-run, **state the denominator: 10 of the cpp-host's 16 files (62.5%), 6 deferred.**
- **(d)** fuzzers — **VACUOUS**: this row adds none (**55** repo-wide). Said to be vacuous, not green.
- **(e)** lint/format/vet/modules — **INAPPLICABLE**: zero `.go` bytes committed; `go.mod`/`go.sum` untouched.
- **(f)** `REVIEW.md` — **ABSENT. A STANDING LINEAGE DEPARTURE, recorded rather than claimed as compliance**: **86 of 123** phase directories carry none (**37** do); the last authored was 25.3.

## Sentinel

Re-run mechanically with firing NCs at the start and re-run after the writes. **Does NOT fire; `stop` NOT created.** (1) `NOT DONE: row 82` at `want=114`; row-62 doctored-copy NC fired with the doctored field printed first; `want=113` NC fired. (2) **FIVE**, unchanged — **this row narrows NOTHING, stated not forecast**; the one-arm strip moves 5 -> **4**, not 0. (3) **`NEVER OPENED: gRPC` ALONE**. **Leak check: `ROADMAP.md` BYTE-UNTOUCHED, verified by `git diff --stat`.**

## Handoff

**The PLAN must prototype S1 (the stream-control pause half) first** — it is the only unprototyped item of eight and it dominates the cost. Three items are **measured**: the cache half at **93 net** production `.go` (`-race` green, reverted), the enum fix at **~10**, the mechanical fixture flip at **+4**. Lower bound **~710-1300 net over 14-20 tasks**; the §6.1 `:290` ~1500-LoC trigger is live at ~1.5x. ⚠️ **If it splits, Leg B (enum + cache) MUST precede Leg A (stream control + blob + fixture) — never the reverse.**

---

# PLAN record (2026-08-02)

**Stage:** PLAN (lifecycle-state `2` -> `3`). Base master **`71fc86d7`** (`git rev-parse master`), branch `phase-82-plan`. Docs-only: **ZERO production `.go`, ZERO test `.go` committed.** `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**, sentinel `want` **STAYS 114**, row 82 **STAYS `in-progress`**.

## What landed

`PLAN.md` (NEW) · `PROGRESS.md` (this record) · `STATE.md` (§Current rolled IN PLACE, §Recent add + evict) · `STATE_HISTORY.md` · `next-prompt.txt` — **five files**, matching the phase-79/80/81 PLAN precedent.

## Method

**Five investigation agents on disjoint remits**, each in its own **DETACHED** worktree off `71fc86d7` with private scratch and a private port band inside `42100-42349`. **Every load-bearing claim was re-derived by the controller**; one agent figure did not survive that re-derivation (A1's guest-crate denominator of 25 — it is **35**), and one agent measurement could not be re-derived through the available channel and is **labelled as such rather than laundered** (A3's F1/F2 intermittency). Zero commits, zero branches, zero pushes by any agent; all five ended at `git status --porcelain` = **0 lines**, controller-re-confirmed. Docker was used by A2 only (`p82p-a2-ref-{old,new}`, network `p82p-a2-net`), removed **BY NAME**, never by an image or ancestor filter.

## The stage's headline

**The SPEC's dominant cost item was over-estimated by an order of magnitude, and the split axis it handed the PLAN dissolves.**

1. **S1 was priced at 300-600 net and MEASURES 30** — because **four of the six** production `ProxyActionPause` arms **already honor Pause** (`body.go:227`/`:314` -> `DataStopIterationAndBuffer`; `trailers.go:143`/`:200` -> `TrailersStopIteration`). Only the two **headers** arms are deferred. *"Stream control deferred since 25.2"* is false as a blanket claim.
2. **S1 alone HANGS and LEAKS** — `downstream_cx_active = 1` and `wasm_wazero_active = 1` persist 30+ s after client disconnect, with no `OnDestroy`. A **NEW scope item (S9)** no document names.
3. **The SPEC's own replacement gate goes GREEN ON A TRAP** — `HttpCallResponseInc()` fires at `http_call.go:423`, *before* the guest call at `:425`.
4. **S5 silently opens a guest WRITE surface** — `headerMapForType`'s `active` flag has **seven** consumers, four of them mutators; a measured probe had the guest mutating the stashed map.
5. **The blob is byte-reproducible only under rustc 1.94.0 + `log 0.4.30`, and neither pin is recorded** — `Cargo.lock` is gitignored, the documented recipe fails to link on the installed default 1.96.0, and **no checksum gate exists anywhere** (`git grep '4e630adf\|139655'` ⇒ zero hits).
6. **S0 is NET 0**, not the "~10 MEASURED" §12 records.

## Refutation count: **FOURTEEN**, of which **SIX are load-bearing**

Load-bearing: S1's cost · the parked-stream leak · the green-on-trap replacement gate · S5's mutator surface · the missing blob pins · the accessor fork having four options with the SPEC's preferred one dominated.

Structural/numeric: S0 net 0 · seven wrong cites (incl. §2.6's own `http_call.go:406`, and §2.23 "settling" two disputes the wrong way) · `ROADMAP.md:<line>` cites **117** not 118 · the `emitScenario` nolint claim · the cap-denied test population (**122 of 128** gate-sites across 423 tests, from only 4 tests) · `ABICallbacks` being **21** methods not 20 · the ADR-0071 single-goroutine invariant break · the chain-scoped `decodeResumeCh` latch.

## Decision: **NO SPLIT** — band ~800-1175, budget ~1050, **SIXTEEN** tasks

**NEITHER §6.1 trigger fires**: 16 tasks against `:289`'s ~25 (1.6x), ~1050 LoC against `:290`'s ~1500 (1.4x). The SPEC's Leg A / Leg B axis was priced on S1 dominating; it measures 30 net and is now the **fifth**-largest item. The ordering constraint the split existed to enforce is preserved as **task order within the row** (T1-T4 before T5), which SPEC §12 explicitly permits.

⚠️ **The composition inverted even though the total did not** — the SPEC put 300-600 on S1 and 200-400 on tests; measurement puts **30 on S1 and 600-900 on tests**. The residual risk has moved to **test scaffolding**, which is exactly what ran phase 81 to 3.07x. **A §6.1 crossing at the IMPL is a FINDING, not a retro-split.**

## Gates — a docs-only PLAN owes (a)-(f) only in the posture a docs-only stage can have

- **(a)/(b)** differential — **INAPPLICABLE**: zero `.go` committed. Measurement runs were made and reverted; the controller's own `0036` runs went **3/3 PASS** at 35.27 / 34.19 / 34.16 s, `INNER_EXIT=0` each, with `=== RUN` positively asserted and no `[no tests to run]`.
- **(c)** conformance — **INAPPLICABLE** for this row. ⚠️ When re-run, **state the denominator: 10 of the cpp-host's 16 files (62.5%), 6 deferred.**
- **(d)** fuzzers — **VACUOUS**: this row adds none (**55**). Said to be vacuous, not green.
- **(e)** lint/format/vet/modules — **INAPPLICABLE**: zero `.go` bytes committed; `go.mod`/`go.sum` untouched.
- **(f)** `REVIEW.md` — **ABSENT. A STANDING LINEAGE DEPARTURE, recorded rather than claimed as compliance**: **86 of 123** phase directories carry none (**37** do); the last authored was 25.3.

## Sentinel

Re-run mechanically with firing NCs at the start and again after the writes. **Does NOT fire; `stop` NOT created** (`ls stop` ⇒ `No such file or directory`). (1) `NOT DONE: row 82` at `want=114`, denominator printed; row-62 doctored-copy NC fired with the doctored field printed first; `want=113` NC fired. (2) **FIVE**, unchanged — **this row narrows NOTHING, stated not forecast**; the **thirty-fourth** consecutive phase without a decrease; the one-arm strip moves 5 -> **4**, not 0. (3) **`NEVER OPENED: gRPC` ALONE**. **Leak check: `ROADMAP.md` BYTE-UNTOUCHED, verified by `git diff --stat`.**

⚠️ **A gate-authoring trap the controller hit LIVE and is recording rather than hiding:** its first row-well-formedness ARM-A used the wrong pipe-split threshold and flagged **113 of 114** rows. **A gate that fires on almost everything reads as thorough and is worthless — the printed denominator is what exposed it.**

## Handoff

**The phase-82 IMPL, sixteen tasks, in the stated order.** T1 (S0+S5 atomic, **with the mutator read-only guard**) -> T2 (cache via the single-method C2 accessor) -> T3 (trap propagation, **reusing `EnvoyGoFailuresInc()` so stat surface `+0` survives**) -> T4 (`HttpCallResponseInc` ordering) -> T5/T6 (S1, **decode only — do NOT touch `body.go`/`trailers.go`**) -> T7 (**S9, the parked-stream leak**) -> T8 (**the capability-enabled test harness — 122 of 128 dispatch gate-sites are cap-denied today**) -> T9-T12 (tests) -> T13 (**S7 blob AND the two pins**) -> T14 (fixture, +26 net measured) -> T15 (breaks; **BREAK-B mandatory, BREAK-C already green as the negative control, BREAK-D a coin flip until T5**) -> T16 (gates, ADR-0304 completion **carrying the ADR-0071 invariant break**, row 82 -> `done`).

# IMPL record (2026-08-03)

**Stage:** IMPL (lifecycle-state `3` -> `DONE`). **ROW 82 FLIPPED `in-progress` -> `done`; THE PHASE IS CLOSED.** Base master **`c0be319b`** (from `git rev-parse master`), branch `phase-82-impl`. Sentinel `want` **STAYS 114** — the IMPL adds no row, it updates `status` in place per §Schema `:18`.

**Execution:** six agents, each in its own worktree with private scratch and disjoint port bands inside `42400-42799`. Three ran the sequential core chain in one worktree (the T1-T4-before-T5 ordering is a **correctness** constraint, not a preference); two ran the independent Rust/fixture streams in parallel detached worktrees and were cherry-picked in **clean, zero conflicts**; one ran the break roster in a detached worktree and **committed nothing** — its entire deliverable is evidence. All reported `git status --porcelain` = 0 lines.

⚠️ **One deliberate reordering, recorded:** **T8 ran FIRST, before T1.** T1 owes the repair of two vacuous tests, and that repair needs the capability-enabled config T8 builds. The correctness ordering (T1-T4 before T5) is untouched.

## Cost — a §6.1 CROSSING, recorded as a FINDING and NOT retro-split

**Budget ~1050 net `.go`. Realized `+2675` net `.go` (2809 added / 134 deleted) across 24 files; all-files `+2887`.** That is **2.55x**, and across `:290`'s ~1500-line trigger.

⚠️ **THE COMPOSITION INVERTED EXACTLY AS THE PLAN WARNED.** The PLAN priced the stream-control half at **30** measured lines and the tests at 600-900; the tests dominated. **A MEASURED PROTOTYPE IS A LOWER BOUND — this is the SECOND CONSECUTIVE ROW to demonstrate it** (phase 81: 3.07x over a nine-prototype measured basis). **No scope was reduced to conceal the crossing**, and the phase-81 precedent is followed: record, do not retroactively split.

## Gates — MEASURED, each with its denominator

| gate | result |
|---|---|
| full differential | **120/120 PASS in 388.961 s**, `INNER_EXIT=0`; FAIL/SKIP **empty**; `no driver registered` **0**; panic/DATA RACE/SIGSEGV **0**; fixture-roster `comm -3` **empty** |
| `0036` alone | **PASS 18.87 s** — ⚠️ **DOWN from 34.2-35.3 s**, unforecast: the rebuilt guest resumes and retires a 15 s dead reference client-timeout |
| h2spec | **53 tests, 53 passed, 0 skipped, 0 failed** — ⚠️ **ONE report, no reference arm; the PLAN's "106 passed" DOES NOT REPRODUCE at this tip** |
| proxy-wasm conformance | PASS |
| `go test ./cmd/envoy-go/` | ok 8.974 s |
| touched packages | ok, and a **second** `-race` run ok |
| `gofmt -l` | **empty OUTPUT** (never gated on exit code) |
| `golangci-lint` | 0 findings, **negative-controlled** — an injected probe FIRED, then was restored to a 0-line `git status` |
| **stat surface** | **+0 BY CALL-SITE ENUMERATION** — **145 registration code sites across 21 production files, IDENTICAL at base and at HEAD**, and `git diff --stat` over every stats source path returns **EMPTY**. ⚠️ Denominator is THIS session's matcher; it disagrees with the PLAN's 208/35. **Only the DELTA is asserted, and it is asserted two independent ways.** |

**Six-gate posture, named not claimed:** (a)/(b) **GREEN, for real** — this is the first phase-82 stage to commit `.go`. (c) proxy-wasm PASS. (d) **VACUOUS** — this row adds no fuzzer. (e) `go.mod`/`go.sum` byte-untouched. (f) `REVIEW.md` **ABSENT — the STANDING LINEAGE DEPARTURE**, named rather than papered over.

## The break roster — EVERY ARM LIVE, NONE VACUOUS

Each arm quotes the assertion that **actually** fired, not merely that one did.

| arm | injection | fired |
|---|---|---|
| **A** | side-asymmetric wrong header, ONE side only | byte divergence at offset 659 — **no stats leg fired** |
| **B** (mandatory) | revert the map-type-6 arm | divergence at 641 **AND** `http_call_response = 0; want >= 1`. ⚠️ **NOT RUNNABLE before this row** — there was no arm to revert |
| **C** (negative control) | flagrantly wrong value, **SYMMETRIC** | **PASSES**, as predicted — confirming a fixture-layer break MUST be side-asymmetric |
| **D1** | suppress the success-path counter | **only** leg 1 |
| **D2** | force the after-close path | **only** leg 2 |
| **D2'** | re-introduce the historical pre-82 defect | both legs — and the **retired disjunction would have summed to 1 and read GREEN** |
| **E** | re-introduce the swapped enum | 3 unit tests + the fixture; **the rest of the tree is BLIND** |
| **F** | remove the `:status` synthesis | divergence at 659 — **all three stats legs stay GREEN** |

⚠️ **THE TWO GATE-COVERAGE FINDINGS THAT MATTER MORE THAN THE GREENS.** (1) **BREAK-F is caught by the cross-side comparison ALONE.** The subject serves a wrong value while every counter reads success — so a counter proving a callback *ran* is structurally incapable of proving it returned the *right answer*. Had the cache landed without the fixture change, that defect would have shipped green. (2) **BREAK-E's prior explanation is REFUTED**: it is not "only numeric literals catch it" — a **real guest blob** catches it too, because its compiled SDK sends 6 regardless of what Go declares. The correct discriminator is *the wire value originates outside the Go declaration*.

## Refutations — SEVENTEEN, of which SIX are load-bearing

**Load-bearing:**
1. ⚠️ **`reference_differential_log_lacks_subject_stderr` is REFUTED for this harness.** `test/differential/harness.go:258` sets `cmd.Stderr = os.Stderr`, so under `go test … > log 2>&1` the subject's stderr **does** land in the log — full wazero trap traces were read verbatim. **The PLAN recorded this as a failed CHANNEL; the channel works. The memory is now corrected.**
2. ⚠️ **The PLAN's h2spec figure does not reproduce.** One report, **53/53/0/0**, no reference arm, and the per-group `X/X passed` figures sum to 53. **State your own denominator** — this stage's is 53.
3. ⚠️ **"Removing all five `//nolint:unused` leaves zero findings" is REFUTED BY EXECUTION** — at base it emits **five** findings. The stated reason (transitive reachability) is a non-sequitur: reachability from an **unused root** is still unused. The removals are valid **only after** the row's own flip supplies a live caller.
4. ⚠️ **`git grep '4e630adf\|139655' ⇒ zero hits repo-wide` was FALSE at the tip that asserted it** — it returns five, all of them the asserting stage's own prose. **The claim was self-invalidated by its own landing.** The substantive part (no roster, no size assertion, no golden, no Rust toolchain in CI) holds.
5. ⚠️ **The blob link-failure framing was misleading.** Not `proxy_continue_stream` x15+: **43** undefined-symbol lines across **32 distinct** host imports, reproducing on the **unmodified** source. It has no causal connection to the symbol this row adds a call to.
6. ⚠️ **The cap-denied population is 123/129, not 122/128** — stable across three runs; 13 distinct tests, 100 of the 123 from a single loop test.

**Also refuted:** `Header.Set` would **not** mangle `":status"` (the canonicalizer returns non-token input verbatim) — the fix is still right, the stated *reason* was wrong · the PLAN's per-site fixture numstat **disagrees at every site** (predicted ~+26, realized **+118**), and the stats split **grew** where a shrink was predicted · one propagating trap site was cited at the wrong line and propagates by *returning* the error, not via the trap helper · the ContinueStream test is at a different line than cited and was misnamed for a method live since 25.2 · `next-prompt.txt` and `ROADMAP.md` carry **zero** occurrences of the four enum symbols, contra the brief · a first break arm proved **vacuous** (the fixture Cloned the map, so the assertion inspected an object the implementation never touched) and was rebuilt · a stubbed producer **cannot test this surface at all** — the Rust SDK traps in its own token lookup long before any host wiring is reached.

**COULD NOT VERIFY, labelled rather than laundered:** the (l) counter's 3/5 intermittency (the row made the path deterministic, so the question is now moot rather than answered) · rustc 1.95.0 as a boundary (installed, but without the wasm target) · two prior figures reproduced at different coordinates after this row's own edits.

## Deferred BY NAME — added by this row

- ⚠️ **The body/trailers Pause arms set no paused flag**, so the resume gate will not release them. **No guest crate calls any resume hostcall today**, so nothing reaches it. Must land with a re-run of the body-cap scenario.
- ⚠️ **The row's own trap arm is SILENT.** The failure counter cannot discriminate *which* callback trapped (one break produced three increments; two were downstream shrapnel). A log line would introduce a logging surface into a package that has **none** — measured, with a firing NC on the sibling package that has one. **Deferred with the evidence attached rather than smuggling a convention change onto the last task of an over-budget row.**
- ⚠️ **A hardcoded test port collides** across parallel sessions and across back-to-back runs through `TIME-WAIT` — observed live.

Everything the PLAN §7 deferred is carried forward unchanged.

## Sentinel — re-run MECHANICALLY. It does **NOT** fire; `stop` was **NOT** created

Input measured **230 lines / 114 data rows** before anything was written.

- **(1) SILENT — and this row is what silenced it.** ⚠️ **Silence is otherwise indistinguishable from a broken check, so THREE negative controls were run and ALL THREE FIRED:** row 62 doctored on a scratch copy ⇒ `NOT DONE: row 62` with the doctored field printed first; **row 82 ITSELF doctored back ⇒ `NOT DONE: row 82`, proving the check sees THIS row**; `want=113` ⇒ `GATE FAIL: examined 114 data rows, expected 113`.
- **(2) FIVE — `:192 :202 :212 :218 :226`, UNCHANGED. THIS ROW NARROWS NOTHING, STATED AND NOT FORECAST.** The **thirty-fifth** consecutive phase without a decrease.
- **(3) `NEVER OPENED: gRPC` — ALONE.** This row retired the second of the two failures. NC: an invented slug fires.
- **Leak check:** the diff to `ROADMAP.md` is **exactly one line, the status cell**; check (2)'s five fail-strings are unchanged at five and check (3)'s literal `WASM-family row` still occurs **exactly once, at line 144, in field 7**.
- `ls stop` ⇒ `No such file or directory`. **`stop` MUST NOT be created — checks (2) and (3) are both still live.**

## Document deltas

`ROADMAP.md` **one cell** · `DECISIONS.md` **17824 -> 17858** (ADR-0304 PROPOSED -> COMPLETE; ⚠️ **the append shifted ZERO existing cites — PROVEN by `head -17824 | cmp -`, which reports exactly one differing line, the in-place STATUS flip at `:17800`**; 303 headings, `^---$` 216, STATUS census 17, retained footer intact, recurrence guard **disarmed to 0**).
