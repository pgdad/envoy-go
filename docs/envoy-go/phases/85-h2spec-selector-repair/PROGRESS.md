# PROGRESS — phase 85 (h2spec-selector-repair)

## BRAINSTORM — done 2026-08-08

## What landed

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `docs/envoy-go/phases/85-h2spec-selector-repair/BRAINSTORM.md` (new) · `ROADMAP.md` **row 85 registered `in-progress`** (234 -> 235 lines, 116 -> 117 data rows; the row sits at `:147`) · `next-prompt.txt` sentinel **`want` 116 -> 117 in the SAME commit as the row** · `STATE.md` rolled **IN PLACE** · `STATE_HISTORY.md` appended (BRAINSTORM-84 evicted at the five-entry cap), verified STRICTLY APPEND-ONLY. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; this stage adds **NO ADR** (next-free stays ADR-0307).

## Method

**SELF-PICKED** per the 2026-07-12 standing directive; no banked mid-lifecycle work existed (phase 84 CLOSED, row 84 `done` — check (1) was SILENT at `want=116` for the first time before this stage re-opened it at row 85). **THREE investigation agents** on disjoint remits: h2spec candidate re-derivation **BY EXECUTION** (own detached worktree at `84c16c65`, port band 46500-46599, removed and verified afterward); the six family backlog paragraphs (read-only); a sweep over every other written-down deferral (read-only). Every load-bearing claim controller-re-derived; two agent claims did NOT survive as stated (BRAINSTORM §5.3).

## The stage's headline

**THE H2 CONFORMANCE GATE'S TRUE SCOPE WAS EXECUTED FOR THE FIRST TIME: 95 cases, FOUR RED.** As committed, the gate runs 53/53 green; with the nine slash-form section-6 selectors dot-corrected it runs **95 tests / 90 passed / 1 skipped / 4 FAILED** — 42 strict RFC 9113 section-6 cases (44% of declared scope) silently unrun since 2026-04-25. The four failures are **production RFC 9113 MUST violations**, causes read at the code sites: 6.5.2/1,3,4 — `onSettings` (`internal/filter/hcm/h2/conn.go:507-538`) applies SETTINGS with zero validation; 6.9.2/1 — live-stream send windows not adjusted on INITIAL_WINDOW_SIZE change. Two NEW gate defects beyond the selectors: the JUnit `<failure>` parse gap (failing-case identity invisible to the Go report) and ADR-0051's **vacuous** 6.6 PUSH_PROMISE exclusion (the pinned image ships no 6.6 suite). Also new: `h2/client.go`'s response path is CONTINUATION-blind too (no `ContinuationFrame` arm) — recorded for the deferred CONTINUATION row, which h2spec 6.10 is **measured NOT to gate** (6/6 green over the live discard).

## The pick

**Row 85 `h2spec-selector-repair`, an Operational-tooling-family MAINTENANCE row claiming NO family ordinal** (ADR-0298/ADR-0300 precedent). Rejected with re-derived costs (BRAINSTORM §2): candidate (i) "the six backlog paragraphs" — **not a row** (prose windows; every retire-shape is doctrinally foreclosed); the CONTINUATION two-sided repair (strongest product defect, 2-4x this row, ungated by any conformance selector — the natural NEXT row); the stat-surface recount (cheapest but repairs no gate; the 1205-vs-1207 contradiction is recorded as widened); `ssl.connection_error` (+444 whole-`.go` floor — the agent's "smallest" framing repeated the phase-75 category error and was rejected); `test/conformance/grpc/` (9/26 reachable); the `validate` nil-`sdsProvider` bug (still the best sub-row bug, still available); REVIEW.md restoration (process-not-product); hygiene fold-ins (too thin).

## Sentinel (measured, before AND after the ROADMAP edit)

BEFORE at `want=116`: (1) SILENT · (2) SIX `:194 :200 :206 :216 :222 :230` · (3) SILENT. AFTER at `want=117`: (1) **`NOT DONE: row 85`** (correct while open) · (2) SIX, anchors shifted to **`:195 :201 :207 :217 :223 :231`** · (3) SILENT. Conjunction fails ⇒ **does NOT fire**; `stop` NOT created (checked before and after). All doctoring NCs fired both sides (row-62; want off-by-one; check-(3) doctoring with residual 0 confirmed first; check-(2) one-arm strips 6->5 / 6->1). Leak axes: check-(2) union **6 -> 6**, `-family row` **95/67 -> 95/67**, `gRPC-family row` **2 -> 2**, `Operational-tooling-family row` **3 -> 3** (the row's MAINTENANCE phrasing deliberately does not match check (3)'s pattern — rows 76/78 precedent); ARM-A flags 119/131 only.

## NEXT

**SPEC** — the seven open questions of BRAINSTORM §4, chiefly: CI enrollment (Q1), the reference container's own section-6 failure set (Q2, UNVERIFIED here), and the green-at-every-tip sequencing of selector flip vs production fixes (Q3). ADR-0307 §Context drafted STATUS `PROPOSED` at the SPEC per convention.
