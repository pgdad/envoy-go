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
