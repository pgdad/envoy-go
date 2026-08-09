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

## SPEC — done 2026-08-09

Docs-only: **ZERO production `.go`, ZERO test `.go`; `ROADMAP.md` BYTE-UNTOUCHED, `want` stays 117.** Landed: `SPEC.md` (all seven BRAINSTORM-§4 questions DISPOSED plus one new scope decision) · **ADR-0307 §Context drafted STATUS `PROPOSED`** (`DECISIONS.md` 17990 -> 18010, strictly append-only `20 0`, base a byte-exact prefix; headings 305 -> 306, tail moves to ADR-0307, next-free ADR-0308 FROM THE NEW TAIL; strict `PROPOSED` guard ARMED 0 -> 1 — a live pointer until the IMPL) · `STATE.md` rolled in place · `next-prompt.txt` rolled to the PLAN.

**Method:** THREE investigation agents — A1 EXECUTED (detached worktree at `cbaf5010`, band 46600-46699, containers `a1p85-*` torn down by name, worktree removed and verified): the subject gate x3 corrected (95/90/1/4 all three, ZERO variance, 6.58 s inner), the failing-run JUnit XML captured, and the PINNED REFERENCE run twice under the corrected strict set; A2 read-only (Q4/Q5/CI enumeration); A3 read-only (the Q7 sweep). Controller re-derived every load-bearing claim from the saved artifacts; three agent/predecessor claims did not survive (SPEC §13).

**The dispositions:** **D-85-SEQ** — ONE IMPL leg, one atomic commit (fixes + selectors + guards + parse fix + docs + CI); no red tip, no split. **D-85-CI** — ENROLL in the `differential` job (deterministic n=3; ~9 s warm vs 23 min headroom). **D-85-REF** — the reference fails FOUR section-6 cases {6.3/1, 6.7/2, 6.9.1/2, 6.9.1/3}, fully DISJOINT from the subject's four; SPEC-84's "three" REFUTED on count, CONFIRMED on disjointness; reference 95/82/1/12 with a FLAKY twelfth slot; fix-to-SPEC rule stated. **D-85-WALK** — unit + h2spec suffice, NO differential fixture; coverage hole measured at exactly zero; walk surface fully enumerated (streams map under `s.mu`, `window.adjust` single-critical-section, overflow -> connection FLOW_CONTROL_ERROR). **D-85-GUARD** — THREE layers keyed on the `package` attr (the captured XML shows `hpack/*` `id` values COLLIDE with http2 ones). **D-85-66** — the exclusion comment REWRITTEN to state the measured vacuity, not deleted. **D-85-SWEEP** — reconcile set is FIVE files (h2spec.go, CONFORMANCE_PINS append-style, BEHAVIOR_CONTRACT `:2054`+`:2056` riding ADR-0307 per `:1821`, STATE.md `:38`, ci.yml); everything else RECORD. **D-85-CLIENT** (new) — the symmetric client-side SETTINGS gap (`client.go:376-407`) is OUT: h2spec cannot gate it; named in ADR-0307 beside the CONTINUATION backlog. **JUnit root cause MEASURED:** h2spec emits `<error>` never `<failure>` (0 vs 4 whole-file), and `<testcase>` has NO `name` attr — fix is `Error` + `ClassName` fields. **Cost: enumerated ~420-750 net `.go` central band, every figure a lower bound.**

**Sentinel (measured at this tip, `ROADMAP.md` untouched):** (1) `NOT DONE: row 85` — the single expected line at `want=117` · (2) SIX at `:195 :201 :207 :217 :223 :231` · (3) SILENT ⇒ does NOT fire; `stop` NOT created. All four NCs fired.

## NEXT

**PLAN** — task decomposition of the single leg in TDD order; refute the §10 enumeration by execution; the 6.5.2/2 accidental-pass probe; the IWS=0 seeding-quirk probe; guard NCs (a doctored selector MUST redden the harness); CI enrollment mechanics.

## PLAN — done 2026-08-09

Docs-only: **ZERO production `.go`, ZERO test `.go`; `ROADMAP.md` BYTE-UNTOUCHED** (verified by empty diff), **`DECISIONS.md` and `BEHAVIOR_CONTRACT.md` BYTE-UNTOUCHED**, `want` stays 117, strict `PROPOSED` guard stays **1**. Landed: `PLAN.md` (the nine-finding re-derivation ledger, the two probe verdicts, the seven-task single-leg decomposition with measured code, the executed break roster, the measured cost table) · `STATE.md` rolled in place · `next-prompt.txt` rolled to the IMPL.

**Method:** TWO probe agents in DETACHED worktrees off `be018027` (bands 46700-46799 / 46800-46899; `p2p85-*` containers removed by name; both worktrees destroyed with byte-exact sha256 proof; nothing committed or pushed by either). **P1 built and ran the ENTIRE change set** — 11 unit arms RED on the unfixed tree, fixes green, harness repaired, the corrected gate **95/94/1/0 FIVE times**, three guard NCs fired. **P2 measured the two open questions at the frame level** (standalone raw-framer probe + h2spec `--verbose` + an instrumented-subject IWS census).

**The four moved claims:** (1) the 6.5.2/2 pass is **GENUINE** — x/net's `parseSettingsFrame` rejects IWS>2^31-1 at parse time (SPEC's conditional validator arm dissolves into 3-line defense-in-depth; a REAL wrong-code handshake defect surfaces instead: `readClientSettings` blanket-wraps parse errors as PROTOCOL_ERROR where RFC wants FLOW_CONTROL_ERROR — new RED anchor + plumbing fix). (2) the IWS=0 quirk is REAL but **unit-only-discriminable** — the two probe agents CONFLICTED (P2: "co-requisite for 95-green"; P1: 95/94/1/0 three times WITHOUT it) and execution resolved it: a consistent clamp in seeding + effective-old compensates, `pendingDispatch` closes the timing hole; the announced-flag fix is IN, gated by its unit arm. (3) the SPEC's layer-2 guard is **deletable one roster entry at a time** — the roster-drop NC did NOT fire as SPEC'd; a REVERSE check (every running http2/* suite must be rostered) repairs it. (4) the roster is **31 http2/* suites, not 24** (49 total incl. 13 generic + 5 hpack, id collision re-confirmed). **Cost: measured net +757 `.go`** (production +127, harness +126, tests +504) vs the SPEC's ~420-750 band — the SEVENTH consecutive lower-bound firing, cause under-enumeration; IMPL budget ~760-900.

**Sentinel (measured at this tip, `ROADMAP.md` untouched):** (1) `NOT DONE: row 85` — the single expected line at `want=117` · (2) SIX at `:195 :201 :207 :217 :223 :231` · (3) SILENT ⇒ does NOT fire; `stop` NOT created. All four NCs fired; every leak axis invariant (235/117, union 6, `-family row` 95/67, ARM-A 119+131 only).

## NEXT

**IMPL — the single leg** per PLAN §5: unit arms RED (11 measured reds re-proven at the IMPL tip), production fixes (validator + plumbing + announced flag + `window.adjust` + walk), harness repair (selectors, JUnit parse, three-layer guard WITH the reverse direction, 31-entry roster), guard NCs re-fired, ci.yml enrollment, the D-85-SWEEP doc set, ADR-0307 completed in place (guard 1 -> 0), row 85 flipped `done`, gate evidence `95 tests, 94 passed, 1 skipped, 0 failed` run LAST and quoted in the commit.
