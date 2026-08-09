# BRAINSTORM 85 — h2spec-selector-repair

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Date:** 2026-08-08.
**Base master:** `84c16c650fafcd9853e34960fe9356d105fa6abc` (from `git rev-parse master`), branch `phase-85-brainstorm`.
**Method:** SELF-PICKED per the 2026-07-12 standing directive; no banked mid-lifecycle work existed at this tip (phase 84 CLOSED, row 84 `done`, every chartered row `done` — sentinel check (1) SILENT at `want=116` before this stage). **THREE investigation agents** on disjoint remits (h2spec candidate re-derivation BY EXECUTION; the six family backlog paragraphs; a sweep for every other written-down deferral). Every load-bearing claim was controller-re-derived; the h2spec agent's probe worktree was removed and verified gone.

---

## 1. THE HEADLINE

### 1.1 ⚠️ THE PROJECT'S H2 CONFORMANCE GATE HAS RUN 44% SHORT OF ITS DECLARED SCOPE FOR ~80 PHASES, AND THE TRUE SCOPE WAS EXECUTED AT THIS STAGE — 95 CASES, FOUR RED

The phase-84 SPEC discovered the defect statically; **this stage ran the gate both ways** (pinned image `summerwind/h2spec@sha256:5f4a65…`, subject built at `84c16c65`, detached probe worktree, port band 46500-46599, worktree removed and verified afterward):

| selector set | result |
|---|---|
| **as committed** (nine slash-form section-6 strings) | `53 tests, 53 passed, 0 skipped, 0 failed` — `--- PASS: TestH2Spec (3.77s)`; byte-for-byte the 18 suites of `CONFORMANCE_PINS.md`'s table, zero 6.x suites |
| **nine one-character edits, slash -> dot** | `95 tests, 90 passed, 1 skipped, 4 failed`, gate exits 1 — section 6 alone: **42 / 37 / 1 skipped / 4 FAILED** |

`docker run --rm <pin> --dryrun -S http2/6/10` prints the banner and **zero case lines, exit 0** — an unmatched positional argument is a silent no-op; `-S http2/6.10` lists 6 CONTINUATION cases. The selector list is `test/conformance/h2spec/h2spec.go:22-37`: **NINE declared slash-form section-6 strings** (`http2/6/1`…`6/5`, `6/7`…`6/10`) — line 30 is the `http2/6/6` PUSH_PROMISE exclusion **as a comment**, which is why the SPEC-84/`SPEC.md:457` figure "ten" is wrong and ADR-0306 §Context ¶4's "nine" is right (re-verified by direct read here).

### 1.2 ⚠️ THE FOUR HIDDEN FAILURES ARE GENUINE PRODUCTION RFC 9113 MUST VIOLATIONS — NOT PIN OR EXPECTATION ISSUES

Verbatim from the run, with the code sites read at this tip:

| case | expected | actual | production cause |
|---|---|---|---|
| 6.5.2/1 — ENABLE_PUSH value other than 0/1 | GOAWAY(PROTOCOL_ERROR) + close | SETTINGS ACK | `onSettings` (`internal/filter/hcm/h2/conn.go:507-538`) applies every SETTINGS value with **zero validation** |
| 6.5.2/3 — MAX_FRAME_SIZE below 16384 | GOAWAY(PROTOCOL_ERROR) + close | SETTINGS ACK | same — no bounds check |
| 6.5.2/4 — MAX_FRAME_SIZE above 2^24-1 | GOAWAY(PROTOCOL_ERROR) + close | SETTINGS ACK | same |
| 6.9.2/1 — INITIAL_WINDOW_SIZE changed after HEADERS | DATA (length:1, flags:0x00) | DATA (length:3, flags:0x01) | live streams' send windows never adjusted on a SETTINGS_INITIAL_WINDOW_SIZE change |

⚠️ **The gap is PARTIAL, not total** — 6.5.2/2 (INITIAL_WINDOW_SIZE above 2^31-1) and 6.9.2/3 **PASS**; only the three listed SETTINGS identifiers lack checks. The skipped case is 6.9.2/2. These are MUSTs — closing them as "accepted departures" would require a departure ADR, not an expectation tweak, and this row does not propose that.

### 1.3 ⚠️ TWO GATE DEFECTS BEYOND THE SELECTORS, BOTH NEW AT THIS STAGE

1. **The JUnit failure-element parse is broken too:** in the corrected run, `assertThreshold`'s per-case `FAILED: <name>` lines **never printed** — `tc.Failure` parsed nil for all four failing cases; only suite-level `Failures` counts fired. Failing-case identity is currently recoverable **only from container logs**. A repair that fixes the selectors but not this ships a gate that says "4 failed" without saying *which*.
2. **ADR-0051's PUSH_PROMISE exclusion is VACUOUS in the pinned image:** `--dryrun -S http2/6` lists **no 6.6 suite at all** (grep exit 1) — there are zero PUSH_PROMISE cases to exclude; the client-sends-PUSH_PROMISE probe lives at 8.2 and already runs green. The comment scaffolding at `h2spec.go:20,30` defends nothing.

Standing context re-verified: `assertThreshold`'s `if s.Tests == 0 { continue }` at **`h2spec_test.go:310`** (a zero-case suite is invisible); **the gate is not in CI at all** (`command grep -rni 'h2spec\|conformance' .github/` exits 1; `TestH2Spec` skips under `-short`, and CI's test job runs `-short`); ADR-0051 §Decision item 2 (`DECISIONS.md:1747-1750`) still asserts the gate runs `http2/6/1–6/5`, `http2/6/7–6/10` — false on scope, recorded-not-fixed; `CONFORMANCE_PINS.md:37-57` still shows 18 suites / total 53 / zero 6.x rows, first-run record 2026-04-25.

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

**PICKED — `h2spec-selector-repair`.** Registered as an **Operational-tooling-family MAINTENANCE row claiming NO family ordinal** (the ADR-0298/ADR-0300 precedent — a maintenance row does not extend a charter, and the Operational-tooling backlog sentence at `ROADMAP.md:231` is deliberately untouched: this candidate came from SPEC-84 §13, not from any family window, so nothing rolls out of any live sentence at row-done either).

Why it is the smallest **defensible** candidate:

1. **It is the only candidate a predecessor SPEC packaged as a row** — SPEC-84 §13 item 1 and §9.3 both: *"the repair, the four failures, and the missing `tests == 0` guard are one coherent future row."* The router shortlisted exactly two candidates; the other (the six backlog paragraphs) is **not a row at all** (§2.1).
2. **It repairs a broken evidence gate in a project whose discipline IS evidence.** "NO ROW MAY CITE h2spec 53/53" has been a standing caveat since the phase-84 SPEC; this row is the only way that caveat ever retires. Every figure was re-measured here by execution, not inherited (`reference_deferred_candidate_cost_restale`).
3. **It has a deterministic failing-first anchor already observed RED at this tip** — the four cases above, plus the gate's own exit 1 under the corrected selectors.
4. **Its re-derived cost is the smallest among candidates that change anything real:** ~9 one-character selector edits, ~15-25 harness lines (guard + failure-parse fix), **~40-80 net production lines** confined to `internal/filter/hcm/h2/conn.go`, ~100-200 unit-test lines, ~25 doc-table lines, one ADR. No new fixture, no new BackendKind, no new module, no new port. Stated WITH the lineage caveat: `reference_measured_prototype_is_a_lower_bound` fired on six consecutive rows; every figure here is a floor, and the SPEC's job is to ENUMERATE (§4 names the likeliest blowout lines).

### 2.1 Rejected: candidate (i), "the six family backlog paragraphs" — NOT A ROW

Mechanically classified at this tip: all six check-(2) phrases live in standalone family **prose** paragraphs (each line begins `*`, not `|`), naming **~43 candidates (~42 distinct** — RTDS is windowed by two families). Doctrine forecloses every "retire the paragraphs" shape: emptying or rewording live sentences while candidates remain is the `reference_sentinel_matcher_string_self_clears` defect committed deliberately (the phase-57 Task-11 incident is the recorded instance); pre-populating `planned` rows violates `ROADMAP.md:21`; and the router itself says check (2) "has NEVER gone down across ~40 phases — the candidates sentences are a WINDOW onto a larger deferred backlog, not an inventory of it." **The only legitimate move candidate (i) admits is the standard loop move — charter ONE real item — which is what this row does.** Check (2) stays SIX by design.

### 2.2 The other rejected alternatives, with re-derived cost

| rejected alternative | re-derived cost at this tip | why rejected |
|---|---|---|
| **CONTINUATION conformance row** (decode discard `conn.go:255-259` + the encode hole + ⚠️ **NEW: `h2/client.go`'s response read-loop has NO `*http2.ContinuationFrame` arm at all** (:376-:620 switch) and `:428` decodes only the first fragment — the defect is TWO-SIDED on the path row 84 just made load-bearing) | ~100-250 net production + 400-800 test; needs its own split-offset gates — **h2spec 6.10 MEASURED 6/6 GREEN over the live discard**, so no conformance selector gates it | The strongest real-product-defect candidate on the board and the natural NEXT row — but 2-4x this row's size, two-sided (`reference_one_sided_gate_for_a_two_sided_fix`), on the hottest decode path in the proxy, and the gate it needs does not exist yet. Deferring it does not orphan it: it is already recorded in FOUR documents and in the gRPC family window. |
| **Stat-surface mechanical recount** (STATE.md:29 named maintenance deferral + ADR-0301's carried quartet) | ~0 production; one read-only test-side counter ~100-200 lines + doc reconciliation of the live **1205-vs-1207** contradiction (`STATE.md:33` vs ADR-0300/0301 headers) | The cheapest candidate on the board — but it repairs no gate, changes no behavior, and its deliverable can ride any future +0 row, whereas the h2spec repair cannot ride anything. Remains available. |
| **`ssl.connection_error`** (Observability window; the sweep agent's nominee from the six paragraphs) | floor **+444 net `.go` VERIFIED** at BRAINSTORM-84 §2.2 (production-only ~+30; the rest tests + fixture driver); NOT re-measured here | The agent's "inline add" framing counts only the production bucket — the whole-row floor is the largest sub-500 figure on the board, and the phase-75 category error (`production-only vs whole-.go`) is exactly the trap. |
| **`test/conformance/grpc/`** | test-only ~400-1100 lines; **9 of 26** interop cases reachable, 8 structurally un-runnable behind the response-buffering seam | 65% vacuous at birth; SPEC-84 §4 deferred it in writing on two grounds that still hold. A later gRPC-family row's job. |
| **`validate` nil-`sdsProvider` bug** (carried from BRAINSTORM-84 §2.2) | ~30-40 prod + 60-120 test; not re-measured here | Still the strongest sub-row-sized production bug, still available — but it repairs one CLI mode; the h2spec row repairs the gate every future H2-touching row cites. |
| **REVIEW.md restoration** (37 of 125 phase dirs, none since 25.3) | n/a | Process-not-product: retro-writing ~88 reviews would fabricate review acts that never happened; resuming the practice is per-phase gate posture, not a chartered row. |
| **harness_test.go:208 port inventory + xDS cycle-guard automation** | ~10 lines / ~50-100 test lines | Hygiene fold-ins, too thin standalone; natural second legs of a future maintenance row. |
| **Everything blocked or subsystem-sized** (gRPC streaming, `grpc_http1_bridge`/`grpc_web` behind the H1->H2 502, ADS/CDS/EDS/LDS/RDS, hot restart, QUIC robustness…) | multi-row each | Categorically not "smallest"; each stays in its family window. |

---

## 3. SCOPE

### 3.1 IN

1. **Selector repair:** nine slash -> dot one-character edits in `thresholdSections` (`h2spec.go:22-37`), MEASURED sufficient — the corrected set ran the full 95-case sweep at this stage.
2. **Zero-case guard:** replace `if s.Tests == 0 { continue }` (`h2spec_test.go:310`) with an assertion that every declared selector produced >= 1 case — the gate defect class of broken-gate shape 32 (a declared-but-unmatched selector), closed at the gate itself.
3. **JUnit failure-parse repair** (§1.3.1), so a future failure names its case.
4. **The four production fixes** in `internal/filter/hcm/h2/conn.go`: a SETTINGS validation block (ENABLE_PUSH in {0,1}; MAX_FRAME_SIZE in [16384, 2^24-1]) emitting GOAWAY(PROTOCOL_ERROR) + close, and a per-stream send-window delta walk on SETTINGS_INITIAL_WINDOW_SIZE change (RFC 9113 §6.9.2). Unit tests per arm; `-race` on the package.
5. **Docs:** `CONFORMANCE_PINS.md` 53 -> 95 with section-6 audit rows; a NEW ADR correcting ADR-0051 §2's false scope claim and recording the vacuous 6.6 exclusion (append-only — ADR-0051 stays recorded-not-fixed; ADR-0052 `:1803` repeats the claim and is reconciled by the same vehicle; any `BEHAVIOR_CONTRACT.md` statement change rides ADR-0052 `:1821`).

### 3.2 OUT — each with its measured basis

| excluded | basis |
|---|---|
| **CONTINUATION decode/encode/client-side** | h2spec 6.10 MEASURED 6/6 GREEN over the live discard — the repaired gate does not cover it; two-sided; 2-4x this row (§2.2) |
| **`test/conformance/grpc/`** | SPEC-84 §4's two grounds re-verified; 9/26 reachable |
| **Pinning the four failures as accepted departures** | they are RFC 9113 MUSTs; a pin would be a departure ADR, and the fixes are ~40-80 lines |
| **CI enrollment of the gate** | genuinely open — docker is available in the differential job, but cost/flake posture is UNMEASURED (§4 Q1); deciding it here would be a commitment without a measurement |
| **The reference-side failure set** | SPEC-84 recorded "a disjoint set of three" on the reference; **UNVERIFIED at this tip** (needs the reference container); §4 Q2 |

### 3.3 COST POSTURE

Floor, measured or read at this tip: 9 chars + ~15-25 harness + ~40-80 production + ~100-200 unit-test + ~25 docs + one ADR. No fixture leg — the differential surface is untouched, so gates (a)/(b) are owed only as posture statements unless the SPEC finds otherwise. ⚠️ **Every figure is a lower bound** (`reference_measured_prototype_is_a_lower_bound`, six consecutive firings; 84.1 realized 1.50x/1.37x). The likeliest under-enumerated lines are named in §4, not hidden.

---

## 4. OPEN QUESTIONS FOR THE SPEC

1. **CI enrollment:** should the repaired gate run in CI (docker already present in the differential job), and at what flake budget? The h2spec container ran 3.77 s locally — cheap — but testcontainers + docker-in-CI posture is unmeasured.
2. **The reference's own section-6 failures:** run the pinned `contrib-v1.37.2` on the corrected selectors — if the reference fails a disjoint set (SPEC-84 said three), the pins doc should record both sides, and any case the reference ALSO fails needs a decision (fix-to-spec vs fix-to-reference; the project's differential doctrine says spec-first for conformance gates, but say so explicitly).
3. **Ordering inside the row:** selectors + guard land RED unless the four production fixes land in the same leg. One leg (fixes first, then flip the selectors in the same commit as the gate re-run) or two legs (a deliberately-RED intermediate is against the discipline)? The SPEC must sequence this so no intermediate tip has a red gate.
4. **6.9.2/1's blast radius:** the send-window delta walk touches live-stream flow control — enumerate which existing fixtures/tests exercise INITIAL_WINDOW_SIZE changes (suspected: near zero, which is itself the finding), and whether a differential fixture is warranted or the unit + h2spec surface suffices.
5. **`assertThreshold` semantics after repair:** per-suite failures==0 across 31 suites — should the threshold structure change (e.g., named-suite enumeration) so a future selector typo fails loudly instead of vanishing? The zero-case guard covers matching-nothing; it does not cover a suite silently dropping out of the declared list.
6. **The vacuous 6.6 exclusion:** delete the comment scaffolding or annotate it as vacuous? (The ADR records the fact either way.)
7. **The "53/53" citation sweep:** enumerate every document citing h2spec 53/53 (CONFORMANCE_PINS, ADR-0051, ADR-0052, BEHAVIOR_CONTRACT, STATE, ROADMAP row summaries) and state which the row's ADR reconciles vs records.

---

## 5. REFUTATION LEDGER — WHAT THIS STAGE ESTABLISHED

### 5.1 Load-bearing

1. **The broken and corrected gates were both EXECUTED** — 53/53 green as committed; 95/90/1/4 corrected; the four failing cases identified by ID with production causes read at the code sites (§1.1-1.2). The candidate's cost is no longer inherited from SPEC-84's static analysis.
2. **The four failures are production defects, not pins** — `onSettings` applies SETTINGS with zero validation; the gap is partial (6.5.2/2 and 6.9.2/3 pass), so the fix is three identifier checks + one window walk, not a rewrite.
3. **h2spec 6.10 does NOT gate the CONTINUATION discard** — 6/6 PASS with the defect live at `conn.go:255-259` (byte-identical to the BRAINSTORM-84 quote, re-read here). Kills any "the selector repair subsumes the CONTINUATION fix" argument in either direction.
4. **Candidate (i) is not a row** (§2.1) — mechanically classified (prose paragraphs, not rows) and doctrinally foreclosed.
5. ⚠️ **NEW: the upstream client response path is ALSO CONTINUATION-blind** — `h2/client.go`'s frame switch has no `ContinuationFrame` arm and `:428` decodes only the first fragment. Recorded here so the deferred CONTINUATION row prices BOTH sides (`reference_one_sided_gate_for_a_two_sided_fix`).
6. **SPEC-84's "ten malformed strings" is wrong — NINE** (line 30 is a comment); ADR-0306 ¶4 already corrected it; re-verified by direct read. Do not re-inherit "ten" from `SPEC.md:52/:457`.

### 5.2 New facts with no prior record

- The JUnit `<failure>` parse gap (§1.3.1).
- The vacuous 6.6 exclusion (§1.3.2).
- The SETTINGS validation gap is partial, not total (§1.2).
- The stat-surface contradiction has widened: `STATE.md` §Project says 1205, ADR-0300/0301 headers say 1207 — both live, mutually inconsistent, both DOC-SOURCED (carried to the maintenance candidate, not fixed here).

### 5.3 Agent claims the controller did NOT accept as stated

- **"`ssl.connection_error` is the smallest defensible standalone row"** (backlog agent) — rejected: the claim counts only the production bucket; the whole-row floor is **+444 net `.go` VERIFIED** (BRAINSTORM-84 §2.2), and the agent did not re-measure. The phase-75 category error, one row later.
- **The reference "disjoint set of three" section-6 failures** — the h2spec agent explicitly did NOT verify it (needs the reference container); it is carried as §4 Q2, not as a fact.

---

## 6. SENTINEL — RE-RUN MECHANICALLY AT THIS STAGE, BEFORE AND AFTER THE EDIT. IT DOES **NOT** FIRE

Input measured **234 lines / 116 data rows** BEFORE anything was written.

| check | BEFORE edits (`want=116`) | AFTER edits (`want=117`) |
|---|---|---|
| **(1)** | **SILENT** | **`NOT DONE: row 85`** — correct while the phase is open; goes silent again at this row's IMPL flip |
| **(2)** | **SIX** — `:194 :200 :206 :216 :222 :230` | **SIX** — anchors shifted by the row insert to **`:195 :201 :207 :217 :223 :231`** (re-derived, not predicted) |
| **(3)** | **SILENT** | **SILENT** |

⇒ the condition is a CONJUNCTION, checks (1) and (2) print, so **the sentinel does NOT fire**. `stop` was **NOT** created (`ls stop` => `No such file or directory`, before and after).

**NCs, ALL FIRED, both before and after the edit:** row-62 doctoring => `NOT DONE: row 62` (before; after: `row 62` AND `row 85`), with `NC LANDED? [ in-progress ]` inspected first · `want` off-by-one => `GATE FAIL: examined 116 data rows, expected 115` before / `examined 117 data rows, expected 116` after · check-(3) doctoring (residual confirmed **0** first) => `NEVER OPENED: gRPC`, WASM control correctly silent · check-(2) one-arm strips => **6 -> 5** (short) / **6 -> 1** (long), never 6 -> 0, before and after.

**Leak check, whole-file before/after counts (never a diff grep):** lines **234 -> 235** · data rows **116 -> 117** · check-(2) union **6 -> 6** · `-family row` **95 occurrences / 67 lines -> 95 / 67** (the row's "Operational-tooling-family MAINTENANCE row" phrasing deliberately does not match the check-(3) pattern — rows 76/78 precedent) · `gRPC-family row` **2 -> 2** · `Operational-tooling-family row` **3 -> 3** · ARM-A well-formedness flags **119 and 131 ONLY** (row 85 at `:147` is NOT flagged) · new slug occurrences **2**, both inside row 85's own line. **No sentinel matcher string was written into `ROADMAP.md`.**

---

## 7. COUNTS RE-DERIVED AT THIS TIP

- fixtures **121** (`^[0-9]{4}[a-z]?-` predicate) · phase dirs **125 -> 126** (this stage adds `85-h2spec-selector-repair/`) · fuzzers not re-counted (no fuzz-adjacent claim made here)
- `DECISIONS.md` **17990** lines · **305** `^## ADR-` headings · tail **ADR-0306** · `^## ADR-0307` = **0** ⇒ next-free **ADR-0307 from the tail** · BYTE-UNTOUCHED this stage
- `BEHAVIOR_CONTRACT.md` **5955** · `STATE.md` **64** · `STATE_HISTORY.md` **466** (pre-eviction) · `BOOTSTRAP_PROMPT.md` **522** at the repo root
- `ROADMAP.md` **234 -> 235** lines / **116 -> 117** data rows; row 85 at `:147`
- h2spec: **9** declared slash-form section-6 selectors + 1 comment · broken-set cases **53** · corrected-set **95** · section-6 **42/37/1/4** — all BY EXECUTION at this tip

---

## 8. BROKEN-GATE LEDGER

No NEW shape — this row is the CHARTERED REPAIR of shape 32 (a declared-but-unmatched gate selector, minted at the phase-84 SPEC) and closes its class at the gate itself (§3.1 item 2). The JUnit failure-parse gap (§1.3.1) is an instance of an existing class (a gate that fires without naming what fired); recorded against this row's own IMPL rather than minted as new.

---

## 9. HYGIENE

Three agents: two read-only in the stage worktree, one (h2spec) in its own DETACHED worktree at `84c16c65` with port band 46500-46599 — its only edits were the probe (port banding + the nine selector chars), the worktree was removed, `git worktree list` verified showing only the canonical entries, and the stage worktree's `git status` verified clean of any agent edit. No docker container outlived the probe. `go.mod`/`go.sum` untouched. This stage lands **ZERO production `.go`, ZERO test `.go`** — docs only.

---

## 10. NEXT

**SPEC.** It owes: the seven §4 open questions (Q2 needs the reference container run; Q3 is the sequencing constraint that keeps every tip's gate green); the new ADR's §Context drafted STATUS `PROPOSED` (next-free **ADR-0307** from the tail); an ENUMERATION of the row's cost (the §3.3 floor is a floor); and the "53/53" citation sweep (§4 Q7).
