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

---

## SPEC — done 2026-08-16

## What landed

`SPEC.md` (new) + this `PROGRESS.md` entry under `docs/envoy-go/phases/89-h2-decode-filter-mutations/`; `DECISIONS.md` gains **ADR-0311 §Context** with the strict `^> **STATUS: PROPOSED` guard **RE-ARMED 0 -> 1** (`numstat 20 0`, 18208 -> 18228, headings 309 -> 310, STATUS census 23 -> 24, `^---$` **STAYS 216** — a new ADR takes no separator); `STATE.md` rolled IN PLACE (lifecycle-state **1 -> 2**); `STATE_HISTORY.md` appended; `next-prompt.txt` rolled. **ZERO production `.go`, ZERO test `.go`.** `ROADMAP.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED** (verified by EMPTY DIFF against master) — row 89 stays `in-progress` at `:151`, sentinel `want` stays **121**, and the contract edit lands at the IMPL.

## Method

**Subagent-driven per `feedback_execution_style`:** four probe agents on four disjoint detached worktrees, disjoint port bands (47600-47689), private scratch each. **None committed.** The controller ran the sentinel battery, re-derived every load-bearing agent claim by execution, and ran two probes of its own (a real-types aliasing probe through the router's own action closure, in a disposable worktree removed with proof; and a reference header-order probe that FAILED on container networking and is recorded as NOT MEASURED rather than inferred).

**Three claims were corrected or refuted by that re-derivation — one of them the controller's own.**

| # | Claim as received | Verdict |
|---|---|---|
| 1 | ADR-0071 forecloses a slice-native decode chain | **REFUTED** — the ADR body has ZERO occurrences of `http.Header`/`OrderedHeaders`/`signature`/`stability`, with `DecodeHeaders` = 1 as the live positive control. It is a CODE COMMENT with eleven citations. |
| 2 | `SetH2Request` has zero coverage, unlike `SetRequest` | **HALF-REFUTED** — both read 0. The 4 apparent `SetRequest` test files are all `SetRequestCtx`, a different symbol. |
| 3 | (controller's own) `0004` is the only downstream-H2 fixture | **REFUTED BY THE CONTROLLER'S OWN SECOND PROBE** — three more exist with driver-inline configs and no `envoy-go.yaml`; the first scan was yaml-only and therefore blind. |

## The stage's headline

**THE BRAINSTORM'S TWO-CONTAINER MODEL IS INCOMPLETE, AND THE MISSING PIECE SELECTS THE FIX.** There is a **THIRD writer**: the phase-46.1a tracing seam writes `x-request-id`/`traceparent`/`tracestate`/`X-B3-*` onto the ordered carrier **only**, never onto the decode map (`grep -c 'c\.req'` over that block = **0**). **Any whole-map projection deletes them from the upstream request** — measured three independent ways (an OTLP wire probe across three binaries; a **4-of-4** `TestWriteH2_Tracing*` flip with a green baseline and the `=== RUN` denominator asserted at 4; and the grep). That single fact eliminates two of the three candidate shapes, and it falls on an axis the BRAINSTORM had not flagged at all.

## Decisions frozen

| id | decision |
|---|---|
| **D-89-SHAPE** | **DELTA against a pre-decode snapshot.** Verbatim `ReconcileOrderedHeaders` reuse REFUTED (compiles clean, then emits an illegal HEADERS block -> **502 with ZERO backend requests** at a conformant peer). Full rebuild REFUTED (4/4 tracing tests RED). Slice-native REJECTED on **measured** blast radius (33 prod + 38 test files, ~101 mutation sites, a mutation API that does not exist) — **NOT** on ADR-0071. |
| **D-89-SITE** | Keep the existing Set; **RE-ISSUE after the reconcile, after `RunDecodeData`**, over a **FRESH** slice. Do NOT move it — `RunAction`'s H2 arm has no guard while the H1 arm panics. |
| **D-89-PSEUDO** | **Skip every `:`-prefixed key.** Do NOT route to the scalar fields. `Host`/`host` left OPEN for the PLAN, deliberately. |
| **D-89-PROOF** | **Extend `0004-h2-routing` IN PLACE.** Every count axis +0; `0120` stays UNCONSUMED; `0012` is the H1 control at zero cost. Option C (extend `0012`) is **structurally impossible**. |
| **D-89-DOC** | Close the `, H2 differential coverage.` carve-out **IN PLACE, zero line delta**. The mirror sentence under `### Does not yet apply to` **stays — EXACTLY TRUE**, with the mechanism written down. |
| **D-89-BODY** | Decode-side body mutation is **OUT of charter — no reachable defect** (no `DecodeBodyOverride` exists; the repo already documents this in `extproc.go`). H1 and H2 do not diverge. |
| **D-89-STAT** | No new stat. DELTA **0**, measured on the prototype (406 both sides by the same command). |

## Sentinel (measured, ACTUAL output)

⚠️ **A SPEC edits no ROADMAP row — the binding proof is an EMPTY `ROADMAP.md` diff against master**, asserted at stage start (0 bytes) and again at close.

239 lines / 121 data rows. **(1)** `NOT DONE: row 89` ALONE, denominator silent · **(2)** SIX at `:199 :205 :211 :221 :227 :235` · **(3)** SILENT. ⇒ the conjunction FAILS; **the sentinel does NOT fire; `stop` NOT created** (verified absent at the git root).

**ALL FOUR NCs FIRED.** row-62 doctoring — `NC LANDED? [ in-progress ]` inspected FIRST, then `NOT DONE: row 62` **AND** `NOT DONE: row 89` · denominator — `want=120` gave `NOT DONE: row 89` **plus** `GATE FAIL: examined 121 data rows, expected 120` · check-(3) doctoring — residual `gRPC-family row` **2 -> 0** on the doctored copy FIRST, then `NEVER OPENED: gRPC` ALONE with WASM correctly silent · check-(2) one-arm — long **5** / short **1** / union **6**.

**Leak axes:** `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**. **Row 89 well-formed: `fields=8`, control row 88 also `fields=8`.**

## Cost

**MEASURED +162 / -0 net production `.go`, ONE file, ZERO tests** — controller-re-measured independently. 69 comment lines (42.6%), 5 blank, 24 brace-only, **64 substantive executable lines**. ⚠️ **1.8-3.2x over the BRAINSTORM's ~+50-90**, and **still a FLOOR** with the remainder ENUMERATED (outbound RFC 9113 §8.2.2 re-validation — measured leaking `connection`/`transfer-encoding` and reachable through `header_mutation`; the `Host` decision; duplicate-name collapse; four unit files; the differential arms; the unexercised `RunDecodeHeaders`-error exit). ⚠️ **The validation item is the phase-88 shape: the hazard does not exist at the tip, so THIS ROW'S FIX INTRODUCES IT.** Bands — production **~+165-230**, unit **~+150-350**, differential **~+145-200** (anchored on phase 88's MEASURED `+172/-7` into the same fixture).

## Traps that fired ON THE CONTROLLER at this stage

1. ⚠️ **A `\b`-anchored coverage grep and a bare-substring one disagreed, and BOTH readings were wrong in turn.** The `\b` form read 0 test files for `SetRequest`; the substring form read 4; the 4 are all **`SetRequestCtx`**, a different symbol on a different type. Resolved only by printing the matched forms and adding a live positive control (`SetAction` = 2). `reference_symbol_assertion_needs_qualified_name`.
2. ⚠️ **The controller's own first Q4 probe had a BLIND AXIS.** Scanning `test/fixtures/*/envoy-go.yaml` for downstream TLS+ALPN found ONE fixture and made `0004` look unique; three more configure themselves from driver-inline Go and have no yaml at all. Corrected by a second probe before it reached the SPEC.
3. ⚠️ **`--network host` did NOT share the host netns here.** The reference bound its ports inside its own namespace (visible in the container's `/proc/net/tcp`, absent from the host's `ss -ltn`); published ports were unreachable too. The order probe is recorded **NOT MEASURED** rather than inferred. Sibling agents reached the reference fine with `--add-host=host.docker.internal:host-gateway`.
4. ⚠️ **The Bash cwd reset fired repeatedly and the harness announced it** (`Shell cwd was reset to /home/esa/git/envoy-go`). Every git command used `git -C <abs-path>`.

## Probe hygiene

Five worktrees this stage — `wt-89-spec` (the stage branch), `wt-89-a`/`wt-89-b`/`wt-89-c`/`wt-89-d` (probes), plus prototype worktrees `wt-89-a-proto`, `wt-89-a-verbatim`, `wt-89-d2` and a disposable controller worktree `wt-89-ctl`. **No agent committed; the controller squashes.** Ports banded **47600-47689** for agents, **47700-47719** for the controller — clear of the static fixture ports (10000-19172), the subject block (20000-31007), the backend band (11000-14999) and the receiver-race ports 35097/35323/42039. Containers created BY NAME with `--rm` and torn down BY NAME; `docker ps` empty before and after. ⚠️ **A sibling container appeared mid-run and was RECORDED, NOT TORN DOWN.** No tracked file was patched by any probe; the controller's own probe file was deleted and its worktree proven clean by EMPTY `status --porcelain` and `diff --stat`.

## NEXT

**PLAN** — the nine deliverables of SPEC §17: TDD task decomposition with the tracing pin in the RED set · the **slice-only-writer inventory** as a named deliverable · the outbound RFC 9113 §8.2.2 validation decision (⚠️ the fix INTRODUCES this hazard) · the `Host`/`host` decision · the `0004` arm roster with a break protocol per arm and the injection site named · the unit roster including the currently-zero `SetH2Request` coverage · a duplicate-name-collapse reference measurement or an explicit written deferral · the contract edit's exact substring and its zero line delta · **cost re-measured at the PUBLISHING commit** (`reference_cost_figure_measured_at_publishing_commit` went stale TWICE inside the phase-88 IMPL, the second time in the sentence correcting the first).

---

## PLAN — done 2026-08-16

## What landed

`PLAN.md` (new, 578 lines) under `docs/envoy-go/phases/89-h2-decode-filter-mutations/`, this `PROGRESS.md` entry, `STATE.md` rolled IN PLACE (lifecycle-state **2 -> 3**) with the oldest §Recent entry evicted to `STATE_HISTORY.md`, and `next-prompt.txt` rolled. **ZERO production `.go`, ZERO test `.go`.** `ROADMAP.md`, `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **ALL BYTE-UNTOUCHED** (verified by EMPTY DIFF against master, 0 bytes, at stage start AND at close). **A PLAN adds no ADR: next-free stays `ADR-0312` and the strict `^> **STATUS: PROPOSED` guard STAYS ARMED at 1** — the IMPL disarms it. Row 89 stays `in-progress` at `ROADMAP.md:151`; `want` stays **121**. Base master `733f9830`, branch `phase-89-plan`.

## Method

**SUBAGENT-DRIVEN** per `feedback_execution_style`: five probe agents on five disjoint detached worktrees (`wt-89-a`..`wt-89-e`), disjoint port bands **47800-47879**, private scratch each; **none committed and each proved its tree clean** (EMPTY `status --porcelain` AND `diff --stat`). The controller ran the sentinel battery, the count battery, and **re-derived every load-bearing agent claim by execution** — which produced two further refutations no agent made and confirmed one that **corrects a standing method note**.

## The stage's headline

**THIS PLAN REFUTES ITS OWN SPEC ON THREE DECISIONS, AND ONE OF THEM CHANGES THE ALGORITHM.**

1. ⚠️ **DUPLICATE-NAME COLLAPSE: MEASURED AND REFUTED.** SPEC §13.2 item 3 proposed collapsing a duplicated name to its FIRST wire position and asked for a reference measurement. Measured on `contrib-v1.37.2` with a raw-framer client AND backend: reference Envoy is an **ORDERED MULTIMAP** — a *replaced* name is **removed everywhere and ONE copy appended at the TAIL** (`x-dup` at `[04]`/`[06]` -> a single `REPL` at `[07]`); *untouched* duplicates stay at their original non-adjacent positions; nothing is ever comma-joined or collapsed to position 0. Same preservation on H2->H1, H1->H2 and H1->H1. **All THREE prototypes built this stage rewrite at the first occurrence and therefore diverge.** ⇒ **D-89-DUP: adopt the reference rule.** Two corroborations: the subject's H2 **passthrough** ordering already matches the reference exactly, and **the repo's own `upsertH2Header` already implements drop-everywhere-then-append-at-tail** — the reconciler differs only in building a FRESH slice instead of the `fields[:0]` in-place compaction. ⚠️ **AND the H1 leg is NOT a usable ordering anchor** — it groups duplicates adjacently and emits names SORTED (Go `net/http` map semantics), already lossy on exactly this input.
2. ⚠️ **THE CONTRACT EDIT'S "EXACT SUBSTRING" IS NOT A UNIQUE ANCHOR, AND THE STATED GATE IS BLIND TO THE FAILURE.** `, H2 differential coverage.` occurs **THREE** times in `BEHAVIOR_CONTRACT.md` — `:32` `header_mutation` (this row's), `:33` `local_ratelimit`, `:34` `csrf`. Executed on scratch copies: a naive global `sed` leaves residual **0** and silently closes two carve-outs this row does not own; the scoped edit leaves residual **2**. **BOTH have a line delta of 0**, so the SPEC's zero-line-delta gate cannot tell them apart (`reference_compensating_defects_cancel_in_the_gate_metric`). ⇒ anchor on the ROW (`^| HTTP filter \`envoy.filters.http.header_mutation\` |` reads **1**) and **gate on the RESIDUAL COUNT reading 2, not 0**.
3. ⚠️ **SPEC §4.4's TWO NUMBERS ARE BOTH WRONG.** `emitAccessLogH2` has **SIX** production call sites (`h2dispatch.go:318 :401 :542 :609 :616 :645`), not five — the `:609`/`:616` pair straddles the `ReconcileOrderedHeaders` branch and reads as one by eye. And there are **THREE** `req.Headers` scans, not two: the third is `reqHeaderLookupH2` (`accesslog_emit.go:230`), feeding tracing `custom_tags` (`:118`) and **`request_headers_to_log`** (`:135`). ⇒ the fix changes those outputs too; stated in the PLAN rather than discovered at the IMPL.

## The row got LARGER, not smaller

**D-89-VALIDATE: the outbound RFC 9113 §8.2.2 validation is IN, and it must be VALUE-AWARE.** The SPEC flagged one hazard; there are **four**, three new:
- the leak SET is incomplete — **`te` with a non-`trailers` value also leaks**, and a name-only guard **structurally cannot** catch it (`te` is deliberately excluded from `isConnectionSpecificField` because it is value-conditional);
- ⚠️ **a FOURTH hazard the SPEC never named: header-name CASE.** `header_mutation` canonicalizes config keys (`http.CanonicalHeaderKey`, `:145`/`:158`) and **nothing below the reconciler lowercases** — controller-confirmed at the emit site, `h2/client.go`: `headers = append(headers, req.Headers...)`, verbatim. A counterfactual `nolower` binary put `"X-Mixed-Case"` and `"X-Upper"` **on the wire**;
- ⚠️ **"reachable" UNDERSTATES it** — against a conformant `h2c.NewHandler` peer the leak is **`400`, body `request header "Connection" is not valid in HTTP/2`, backend request count ZERO**, against a control of 200/count 1. A production outage shape, not a cosmetic divergence;
- the tip is clean on all of it precisely BECAUSE nothing reaches upstream ⇒ **THIS ROW'S FIX INTRODUCES THE HAZARD** (the phase-88 shape).
**Cost MEASURED: +22 lines across ONE extra file.** ⇒ **EXPORT `h2.IsIllegalH2RequestHeader(name, value)`, do NOT duplicate** (the predicate's own doc calls itself *"the ONE source of truth for the RFC list"*), and **DROP rather than reject**. A **stacked over-firing control** was executed: illegal dropped, **legal `te: trailers` KEPT**, benign kept, removal applied, order preserved, no duplication.

**D-89-HOST: SKIP `host` alongside the `:` prefix.** Measured: the reference **NORMALIZES** — `host` and `:authority` are one entry; a regular `host` appeared on the H/2 upstream in **0 of 15** reference readings, and `add("host")` **comma-joins into `:authority`**. envoy-go's own **H1** leg silently ignores a Lua `host` replace, proven with an `x-lua-ran` positive control. Projecting would put a `host` next to a contradictory frozen-scalar `:authority`. ⚠️ **The residual divergence is written into the contract, or the row's claim ships over-broad.**

## Sentinel (measured, ACTUAL output)

⚠️ **A PLAN edits no ROADMAP row — the binding proof is an EMPTY `ROADMAP.md` diff against master**, asserted at stage start (**0 bytes**) and again at close.

239 lines / 121 data rows. **(1)** `NOT DONE: row 89` ALONE, denominator silent · **(2)** SIX at `:199 :205 :211 :221 :227 :235` · **(3)** SILENT. ⇒ the conjunction FAILS; **the sentinel does NOT fire; `stop` NOT created** (verified absent at the git root).

**ALL FOUR NCs FIRED.** row-62 doctoring — `NC LANDED? [ in-progress ]` inspected FIRST, then `NOT DONE: row 62` **AND** `NOT DONE: row 89` · denominator — `want=120` gave `NOT DONE: row 89` **plus** `GATE FAIL: examined 121 data rows, expected 120` · check-(3) doctoring — residual `gRPC-family row` **2 -> 0** on the doctored copy FIRST, then `NEVER OPENED: gRPC` ALONE with WASM correctly silent · check-(2) one-arm — long **5** / short **1** / union **6**.

**Leak axes:** `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**. **Row 89 well-formed: `fields=8`, control row 88 also `fields=8`.**

## Cost

**THREE independent implementations of one instruction were built this stage, and the spread IS the finding:** SPEC **+162** @ `c1284a03` (69 comment / 5 blank / 24 brace / **64 substantive**) · agent D **+158** @ `733f9830` (84 comment / 6 blank / 19 brace / **49 substantive**) · agent B reconciler-only **+121**, reconciler+validation **+127 / +16**. ⚠️ **The near-agreement of 162 and 158 is PARTLY COINCIDENCE — the totals differ by 4 while the SUBSTANTIVE counts differ by 15**, a heavier comment ratio absorbing a leaner algorithm. **Quote +162 (the highest) as the floor.** ⚠️ **AND IT IS STILL A FLOOR, because NONE of the three implements the §2.3 tail-append rule** — all three rewrite at the first occurrence, which is the measured reference divergence. Remainder enumerated in PLAN §8. Bands: production **~+185-250** · unit **~+250-450** (revised UP from the SPEC's ~+150-350 — the roster grew) · differential **~+145-200** (anchored on phase 88's MEASURED `+172/-7` into the same fixture). Every count axis anticipated **+0**; stat surface **406/406, DELTA 0** measured on the prototypes.

## Traps that fired ON THE CONTROLLER at this stage

1. ⚠️ **A STANDING METHOD NOTE WAS WRONG AND IS CORRECTED BY EXECUTION.** The carried note said `-p` publishing was unreachable here. Controller-executed: `docker run -d --rm -p 47899:80 nginx:alpine` then `curl 127.0.0.1:47899` ⇒ **HTTP=200**, and host `ss` sees the port. **`-p` WORKS.** The real mechanism is narrower: **`--network host` is broken here AND it silently IGNORES `-p`**, so a probe combining them reads as "publishing is unreachable". Host-gateway is **`192.168.65.2`** — a Docker-Desktop-style VM daemon, NOT a `172.17.x` bridge gateway, which is exactly why host networking fails.
2. ⚠️ **A PHANTOM SYMBOL IS CITED THREE TIMES INSIDE THE VERY BLOCK THIS ROW EDITS.** `h2dispatch.go:462`, `:479`, `:491` cite `h2.parseHeadersForRequest`, one with a hard line number. `git grep -c '^func.*parseHeadersForRequest'` ⇒ **0**, against a positive control of **1** for `buildH2Request`. Recorded, NOT fixed; flagged so the IMPL does not read those comments as a map of the code.
3. ⚠️ **THE TRACING PIN IS SPLIT ACROSS TWO FILES.** SPEC §6.4 names `tracing_zipkin_dispatch_test.go`; it holds **2 of the 4** `TestWriteH2_Tracing*` rows — the other two are `connection_test.go:855`/`:883`. A PLAN editing only the zipkin file touches half the pin.
4. ⚠️ **SPEC §6.4's `SetAction = 2` POSITIVE CONTROL IS VACUOUS — both hits are COMMENT PROSE.** Repo-wide there are **ZERO non-comment test call sites** for `SetAction`, `SetRequest`, `SetH2Action`, `SetH2Request` AND `RunAction`. The SPEC's conclusion is right for the wrong reason. And **`router_test.go` never constructs a `router.Filter` at all** (the token appears **once**, in a comment at `:140`) — the template the SPEC assumes does not exist, so the `SetH2Request` arm is placed in `hcm` instead.
5. ⚠️ **`buildH2Request` HAS ZERO TEST COVERAGE** (`git grep -l -w` ⇒ **0** test files), so the pseudo-header EXCLUSION contract the whole `:`-skip rests on is pinned by nothing. ⚠️ Even the positive control collided: `buildRequest` reads **6** test files, but **three are `internal/filter/network/kafkabroker/*`** — a different package's function; the h2-side control is **3**.
6. ⚠️ **AN INSTRUMENT DEFECT ONE PROBE FOUND IN ITSELF, carried forward:** a raw-framer backend that builds a **fresh `hpack.Decoder` per request** decodes only the FIRST request on a connection — **the HPACK dynamic table is CONNECTION-scoped** — and every later request yields `invalid indexed representation index NN` with a truncated field list, **which reads exactly like "headers were lost"**. Caught, fixed, everything re-run, `HPACK-ERR` count **0** across all reported logs, with a client->backend DIRECT negative control reproducing `a=1,b=2,a=3` at indices 4/5/6 verbatim.
7. ⚠️ **`0004`'s two YAMLs say "documentation only" (line 3 of each) BUT ARE THE LIVE TEMPLATES** (`readYAML` strips the comment block; `renderBootstrap` substitutes). And `renderBootstrap` does **POSITIONAL, FIRST-OCCURRENCE** `port_value: 0` replacement — **inserting anything containing `port_value: 0` above an existing occurrence silently reassigns ports.** `driver_test.go`'s `TestRenderBootstrap_*` pin the ordering and redden first.
8. ⚠️ **A CASE-VARIANT HAZARD BOTH PROTOTYPES CARRY IS UNREACHABLE TODAY, AND THE CONTROLLER PROVED IT RATHER THAN ASSUMING IT.** `git grep -nE '[Hh]eaders?\[[^]]+\][[:space:]]*=[^=]' -- 'internal/filter/http/**/*.go'` reads **ZERO**, against an NC on the same selector firing **3x** in `h2dispatch.go`. Every filter writer (`lua` 8, `wasm` 9, `extauthz` 13) goes through `Set`/`Add`/`Del`, which canonicalize. **Latent, not live.**

## Two pre-existing subject bugs found in passing (NOT chartered)

Both measured against the reference with no filter at all, so both are independent of phase 89: inbound `:authority` **and** `host` ⇒ envoy-go projects **BOTH** onto the H/2 upstream (`host` verbatim at `[04]`) while the reference **drops** the regular `host`; inbound `host` **only** ⇒ envoy-go emits **`:authority = ""` (EMPTY)** plus `host`, while the reference **PROMOTES** `host` into `:authority`. **Named, not fixed.**

## Probe hygiene

Six worktrees this stage — `wt-89-plan` (the stage branch) plus `wt-89-a`..`wt-89-e` (probes). **No agent committed; the controller squashes.** Ports banded **47800-47879** for agents and **47899** for the controller's single container probe — clear of the static fixture ports (10000-19172), the subject block (20000-31007), the backend band (11000-14999) and the receiver-race ports 35097/35323/42039. **ZERO containers were running at stage start**; every container created was named (`b89c-*`, `b89ctl-*`), `--rm`, torn down BY NAME, and `docker ps` verified back to **0** at close. A pre-existing `--rm` container from other work exited on its own and was never touched. No tracked file was patched by any probe; all doctoring ran on scratch copies.

## NEXT

**IMPL** — ONE atomic commit delivering PLAN §4's T1-T8: the RED census observed FIRST with denominators (the fifth tracing row in **BOTH** files inside it) · the reconciler with the **tail-append** rule, the `host` skip, explicit lowercasing and the exported value-aware §8.2.2 guard · the 13-row table plus the three early-exit arms (⚠️ the `RunDecodeData` arm is **silently vacuous unless `hasBody` is true**) and the `SetH2Request` arm in `hcm` · the `0004` extension with eight arms and **seven break arms at seven DISTINCT injection sites** · **the scoped contract edit whose gate is a residual count of 2, not 0** · the two added contract sentences · ADR-0311 completed with the strict guard **1 -> 0** · **row 89 flipped `done`** with `want` **121 -> 121** · the slice-only-writer gate re-read at **6** and stated · **cost re-measured at the IMPL's OWN publishing commit** · the sentinel and all four NCs run on **BOTH sides** of the row flip.
