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
