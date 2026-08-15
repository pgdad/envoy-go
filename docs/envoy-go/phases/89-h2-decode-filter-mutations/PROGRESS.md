# PROGRESS — phase 89 (`h2-decode-filter-mutations`)

## BRAINSTORM — done 2026-08-15

## What landed

`BRAINSTORM.md` + this `PROGRESS.md` under `docs/envoy-go/phases/89-h2-decode-filter-mutations/`; **row 89 registered `in-progress`** at `ROADMAP.md:151` with the sentinel `want` bumped **120 -> 121 in the SAME commit** (the phase-84/85/86/87/88 precedent); `STATE.md` rolled IN PLACE (lifecycle-state **DONE -> 1**) with the oldest §Recent entry evicted to `STATE_HISTORY.md`; `next-prompt.txt` rolled. **ZERO production `.go`, ZERO test `.go`.** `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED** (verified by EMPTY DIFF against master) — **a BRAINSTORM adds no ADR: next-free stays `ADR-0311` and the strict `^> **STATUS: PROPOSED` guard stays 0.** The SPEC re-arms it.

## Method

**SELF-PICKED** per the 2026-07-12 standing directive. **No banked mid-lifecycle work existed and this was PROVEN, not assumed:** phase 88 CLOSED, all 120 rows `done`, check (1) SILENT at `want=120` ⇒ no row `in-progress`. (The incomplete artifact sets under `docs/envoy-go/phases/` are historical parent/child SPLIT structure on CLOSED rows.)

**Subagent-driven per `feedback_execution_style`:** three read-only sizing agents on ONE shared detached worktree (`wt-probe`), one reproduction agent on its own detached worktree (`wt-repro`), the stage branch in a fourth (`wt-89`). **None committed.** The controller re-derived every load-bearing claim by execution — **and that re-derivation earned its keep twice**: it corrected the sizing agent's value-vs-pointer framing to the true two-container mechanism (BRAINSTORM §3), and it caught a malformed-row defect in the controller's OWN row-89 text before it landed (see §Traps below).

## The stage's headline

**Decode-side filter header mutations never reach the upstream request on envoy-go's HTTP/2 downstream leg — additions never arrive, removals never take effect — while the SAME chain on HTTP/1 and the SAME chain's ENCODE direction on HTTP/2 both work, and real Envoy applies both directions.** `internal/filter/hcm/h2dispatch.go:457` snapshots `h2req` before the decode chain runs at `:518`; the mechanism is **two independent containers with no write-back** (`buildH2Request` `h2/stream.go:331` builds the ordered `[]hpack.HeaderField` the upstream is emitted from; `buildRequest` `:389` independently builds the `http.Header` map the chain mutates). **Two of three codecs are already correct** — H1 `connection.go:468`/`:571` and H3 `h3dispatch.go:217`/`:263` both hand the chain the same object. ⚠️ **The H3 fact is NEW at this stage.**

## Reproduced by execution at `125f0714`

Seven arms, subject built from tip into scratch, reference `envoyproxy/envoy:contrib-v1.37.2`, filter fragment copied VERBATIM from `0012-http-header-mutation`. Full quotes in BRAINSTORM §2.

| Arm | Shape | Observed | vs hypothesis |
|---|---|---|---|
| 1 | subject H2, decode ADD | backend got **NO `X-Probe`**; `X-Client` SURVIVED | PASS |
| 2 | subject H1, decode ADD (positive control) | `X-Probe: seen`, `X-Remove-Me` removed | PASS |
| 3 | subject H2, ENCODE side (positive control) | `x-resp-probe: resp-seen` present | PASS |
| 4 | subject H2, decode REMOVE | removal **ignored** | PASS |
| 5 | reference H2-in / H1-out | both applied (delivery verified 1 -> 2) | PASS |
| 5b | reference H2-in / H2-out | both applied | PASS |
| 6 | subject H2, `[header_mutation, rbac(iff x-probe), router]` | **200 — RBAC SAW it — backend did NOT** | PASS |
| 7 | Arm-6 shape, negative control | **403 RBAC: access denied** | PASS |

**Arm 6 + Arm 7 are the isolating pair:** they establish that the mutation exists, is visible to later filters, and is dropped at the emit boundary — and that Arm 6's 200 is not vacuous.

## Sentinel (measured, BOTH sides of the ROADMAP edit, ACTUAL output)

`ROADMAP.md` numstat **`1 0`** — strictly additive, one line, zero removed.

| Check | BEFORE (`want=120`) | AFTER (`want=121`) |
|---|---|---|
| input | **238** lines / **120** data rows | **239** / **121** |
| (1) | **SILENT** (the SEVENTH silent reading in project history) | **`NOT DONE: row 89`** ALONE, denominator silent |
| (2) | **SIX** at `:198 :204 :210 :220 :226 :234` | **SIX** at `:199 :205 :211 :221 :227 :235` |
| (3) | **SILENT** | **SILENT** |

⇒ **the conjunction FAILS on both sides; the sentinel does NOT fire; `stop` was NOT created** (verified absent at the git root AND in the stage worktree).

⚠️ **THE +1 ANCHOR SHIFT WAS PREDICTED IN §5 AND THEN MEASURED, NOT FORECAST.** The count stays SIX and the CONTENT is untouched; only the line anchors move. **Any future stage citing `:198 …` must re-derive — the current six are `:199 :205 :211 :221 :227 :235`.**

**ALL FOUR NCs FIRED ON BOTH SIDES.**

- **row-62 doctoring** — `NC LANDED? [ in-progress ]` INSPECTED FIRST. BEFORE: `NOT DONE: row 62` ALONE. AFTER: `NOT DONE: row 62` **AND** `NOT DONE: row 89` — the correct two-line output once a real row is in-progress.
- **denominator** — BEFORE `want=119` ⇒ `GATE FAIL: examined 120 data rows, expected 119` (its ONLY output, which is what proves the assertion live against a silent check). AFTER `want=120` ⇒ `NOT DONE: row 89` **plus** `GATE FAIL: examined 121 data rows, expected 120`.
- **check-(3) doctoring** — residual `gRPC-family row` **2 -> 0** confirmed on the doctored copy FIRST, then `NEVER OPENED: gRPC` fired ALONE with WASM correctly silent. Both sides.
- **check-(2) one-arm** — long **5** / short **1** / union **6**, both sides. A one-arm strip is NOT an NC for the union.

**Every leak axis INVARIANT across the edit:** `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.

**`want` bump:** the **single executable `want=` site is `next-prompt.txt:17`** — measured, not assumed. Every other `want=120` occurrence is historical prose in append-only phase docs plus `STATE.md`'s narrative line 18; `phases/77-runtime-static-layer/PLAN.md:69` carries a historical `want=109` and was NOT touched.

## ⚠️ Traps that fired ON THE CONTROLLER at this stage

1. ⚠️ **A MARKDOWN TABLE ROW CANNOT CARRY AN UNESCAPED `|`, AND THE CONTROLLER ALMOST LANDED ONE.** The drafted row-89 summary contained a grep alternation `` `h2dispatch|SetH2Request|decode-side filter mutation` ``. Markdown reads those pipes as CELL DELIMITERS: the row measured **`fields=10`** against a well-formed **8** (row 88 as the control), i.e. it would have minted a THIRD malformed row alongside the documented `{119, 131}` pair. **Caught by counting fields BEFORE installing, not after.** The text was rephrased to commas. ⚠️ Note the failure was SILENT for check (1) — `$5` was still `in-progress` because the stray pipes fell AFTER field 5, so the gate would have passed while the row was malformed.
2. ⚠️ **AND THE CENSUS THAT CAUGHT IT IS NOT THE ARM-A FORM — DO NOT "CORRECT" `{119, 131}` WITH IT.** The ad-hoc `NF!=8` form reads **17** malformed rows, because rows legitimately carry ESCAPED pipes. The router's warning is exact: the ARM-A figure binds ONLY to its escape-aware pipe-split command. **What was asserted here is the INVARIANT — the same 17 members with the same field counts on both sides, and row 89 NOT among them — never the absolute.**
3. ⚠️ **THREE ROUTER FIGURES REFUTED** (controller-measured; `125f0714` touched ONLY `next-prompt.txt`, numstat `1 0`, so none is tip drift): `DECISIONS.md` is **18208** lines, not 18206 · the contested stat-surface reads **406 occurrences / 404 lines** by the stated command and the phase-88 IMPL's **405/403 does not reproduce on either axis** · the blank-import gate `grep -cP '^\t_ "'` reads **123**, not 121 — only the narrowed `^\t_ "[^"]*test/fixtures/` reads **121**, matching the 121 fixture dirs 1:1.
4. ⚠️ **THE REPRODUCTION'S FIRST BACKEND WAS THE WRONG PROTOCOL AND THE FAILURE LOOKED LIKE THE DEFECT** — a plain `net/http` backend produced **502 / `hcm: h2: EOF` with ZERO backend requests**, trivially mistakable for "the mutation was dropped". **Assert the backend RECEIVED something before interpreting what it received.**

## Anticipated counts (each +0 unless noted)

stat-surface DELTA **0** · fuzzers **55 / 48** +0 · BackendKind tail **38** +0 · `go.mod` +0 · config fields **+0** · differential fixtures **121 +0 ANTICIPATED BUT NOT DECIDED** (extend `0004` in place vs mint `0120` is a SPEC decision; `0120` stays UNCONSUMED) · next-free **`ADR-0311`** (TAIL-derived; headings+1 COLLIDES at the ADR-0209 gap, headings = **309**) · strict `PROPOSED` guard **0**, re-armed by the SPEC.

## Cost

**ESTIMATED ~+50-90 net production `.go`**, concentrated in `h2dispatch.go` plus one helper. ⚠️ **AN ESTIMATE, NOT A MEASUREMENT** — `reference_measured_prototype_is_a_lower_bound` fired on BOTH axes at the phase-88 IMPL (+284 production against a ~+190-240 band). **THE SPEC MUST ENUMERATE BY COMPILING PROTOTYPE.** The named under-enumeration risk is BRAINSTORM §7 Q3: whether a filter may legitimately mutate `:path`/`:authority`, which would route mutations to the H2Request SCALAR fields rather than to `.Headers`.

## NEXT

**SPEC** — dispose the nine BRAINSTORM §7 questions BY EXECUTION: **Q1** the reconciler shape decided by COMPILING PROTOTYPE (delta-against-snapshot vs full rebuild vs slice-native chain), pricing all three and settling whether `filter_http.ReconcileOrderedHeaders` (`h2dispatch.go:612`) is reusable verbatim · **Q2** where `SetH2Request` moves, re-checking the two `RunAction`-bypassing early exits at `:522` and `:530` · **Q3** pseudo-header safety and whether `:path`/`:authority` mutations must route to the SCALAR fields — **the row's most likely hidden cost** · **Q4** the proof shape, with the corpus facts already measured (fixture `codec_type` census **HTTP1 270 / AUTO 6 / HTTP3 3 / HTTP2 ZERO**; `header_mutation` only in `0012`, which is HTTP1 at both listeners) · **Q5** re-confirm the encode side is already correct so the charter cannot silently widen · **Q6** the contract — **CLOSE the honest "NOT asserted: … H2 differential coverage" carve-out; there is NO false sentence to correct**, and decide in writing whether the reconciler makes the mirror sentence at `BEHAVIOR_CONTRACT.md:829` stale · **Q7** whether decode-side BODY mutation is also dropped (`:552` passes the same frozen `h2req.Body`) · **Q8** keep H3 explicitly OUT of charter · **Q9** h2spec is NOT available as a red anchor (ADR-0307) — cite only from the SPEC's own run.
