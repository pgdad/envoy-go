# Phase 56.2 (http-tap-filter, bodies leg) IMPL — PROGRESS

**SPEC:** `SPEC-56.2.md`
**PLAN:** `PLAN-56.2.md`
**Worktree branch:** `phase-56.2-http-tap-filter-impl` (to be created at the IMPL)

> Scaffolded at the **PLAN** stage (docs-only). Every checkbox below is UNCHECKED:
> no production code exists yet. The IMPL's Task 1 re-runs the baseline commands
> against its own cold-start HEAD and pastes the literal output into
> "Baseline Counts", then ticks tasks as they land.

---

## Task Checklist (7 tasks — WELL under the ADR-0045 `~15` gate, margin 8)

- [ ] T1  PROGRESS baselines + ADR-0045 split re-check (docs)
- [ ] T2  `config.go`: store `format` (bodyAsString) + resolve both caps (nil → 1024; present incl. 0) [TDD]
- [ ] T3  `tap.go`: `DecodeData`/`EncodeData` accumulation — append up to cap, strict-`>` truncation, saw-body flag, always `DataContinue` [TDD]
- [ ] T4  `trace.go`: `bodyProto` (oneof from format, AS_STRING `ToValidUTF8` sanitize) + `buildTrace` wiring (Body iff hook fired) [TDD]
- [ ] T5  Fixture `0100-http-tap-bodies`: YAMLs + driver (three POSTs vs `HTTPEchoBody`); fixtures 101 → 102
- [ ] T6  `0100` `AssertStats`: cross-side body payload + truncation + the 4 deliberate breaks
- [ ] T7  Docs bundle: ADR-0274 body + `BEHAVIOR_CONTRACT` + `ROADMAP` (row 56 `done`) + `STATE`/`README`/`PROGRESS` + count reconcile

---

## Baseline Counts

To be re-recorded at the **IMPL** stage against its own cold-start HEAD (the IMPL's T1
MUST re-run these and replace this block with its literal output — do not assume the
PLAN-time figures still hold). PLAN-time expectation (cold-start HEAD `1b344286`):

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
101

$ grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l
53

$ grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0273 — the HTTP tap filter, headers leg ...

$ grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1
H2GoawayResponder BackendKind = 38
```

Baseline summary:
- stat surface: **1201** (a live reference figure; the tree enforces a *delta* — 56.2 adds **+0**, no delta-guard change)
- fixtures: **101** (tail `0099-http-tap-headers`)
- fuzzers: **53**
- BackendKind tail: **38** (`H2GoawayResponder`) — **stays 38** (the `0100` backend reuses `HTTPEchoBody = 6`)
- DECISIONS tail: **ADR-0273** (next-free **ADR-0274**)

Anticipated at 56.2 IMPL exit: stat surface **1201** (+0) · fixtures **102** (`0100-http-tap-bodies`) · fuzzers **53** (+0) · BackendKind **38** (+0) · DECISIONS tail **ADR-0274** (next-free ADR-0275) · new Go packages **0** · new go.mod modules **0** (`go mod tidy -diff` EMPTY) · **ROADMAP row 56 → `done`** (the final leg — `reference_roadmap_split_phase_row_done` + ADR-0106).

---

## ADR-0045 split re-check (discharged at the PLAN)

56.2 is additive on the proven 56.1 spine (ADR-0045's split gate was CONSUMED at the
phase-56 BRAINSTORM Q6, re-affirmed at SPEC-56.1 §2 / SPEC-56.2 §10). The PLAN's honest
decomposition is **7 tasks** (T1 baselines + 6 substantive from SPEC §10) — ceiling `~15`,
margin **8**. No re-split; no absorbed 56.1 spillover.

**Standing instruction for the IMPL:** if any task grows a second independent deliverable,
split it into a NEW task and re-open the gate. Do NOT push spillover past the 56.2 IMPL —
56.2 is the FINAL leg of phase 56 (there is no 56.3).

---

## Deliberate-break ledger (filled in at the IMPL; record the LITERAL failing text)

The controller re-performs every break itself with `-count=1`. A break failing does NOT
prove the intended assertion is live — it can abort earlier and MASK it
(`reference_deliberate_break_wrong_assertion`). Confirm **which** assertion fired. All
`0100` breaks edit **subject-side** production code, so the **subject** trace must be the
one that violates — confirm the failure text names `subject/...`.

| # | Task | Break | Must fire | Fired? (literal text) |
|---|---|---|---|---|
| 1 | T2 | `resolveCap`: treat present-0 as unset (→1024) | `TestParseConfig_CapResolution/present_ZERO` (`maxRx=1024, want 0`) | _(IMPL)_ |
| 2 | T3(a) | `accumulate`: strict `>` → `>=` | `TestDecodeData_AtCapNotTruncated` (`reqTrunc=true, want FALSE`) | _(IMPL)_ |
| 3 | T3(b) | gate saw-body flag behind `len>0 && cap>0` | `TestDecodeData_CapZeroNonEmpty` (`sawReqBody=false, want true`) | _(IMPL)_ |
| 4 | T3(c) | return `DataStopIterationAndBuffer` | `TestDecodeData_SingleChunkUnderCap` (status `!= DataContinue`) | _(IMPL)_ |
| 5 | T4(a) | swap the AS_STRING oneof → `Body_AsBytes` | `TestBodyProto_AsString` (as_string missing / as_bytes present) | _(IMPL)_ |
| 6 | T4(b) | drop the `ToValidUTF8` sanitize | `TestBodyProto_AsStringSanitizesNonUTF8` (marshal Fatalf on invalid UTF-8) | _(IMPL)_ |
| 7 | T4(c) | gate `Body` on `len(buf)>0` | `TestBuildTrace_BodyPresentWhenHookFired` (empty-but-truncated body absent) | _(IMPL)_ |
| 8 | T4(d) | set `Body` unconditionally | `TestBuildTrace_BodyOmittedWhenHookNeverFired` (RAW-BYTES `"body"` present) | _(IMPL)_ |
| 9 | T6(a) | leave `DecodeData`/`EncodeData` inert | `0100`: `request.body`/`response.body` PRESENT (all arms) | _(IMPL)_ |
| 10 | T6(b) | ignore the cap (always append full) | `0100`: truncated arm `truncated==true` + 20-byte-prefix `as_string` | _(IMPL)_ |
| 11 | T6(c) | strict `>` → `>=` | `0100`: **BOUNDARY arm `truncated`-flag ONLY** (payload UNCHANGED) | _(IMPL)_ |
| 12 | T6(d) | wrong oneof for AS_STRING (emit `as_bytes`) | `0100`: `request.body.as_string`/`response.body.as_string` decode empty | _(IMPL)_ |

> **Break (c) is the strict-`>` proof and is subtle** (`reference_deliberate_break_wrong_assertion`):
> only the BOUNDARY arm's `truncated`-flag assertion may fire; the captured PAYLOAD is
> UNCHANGED (the prefix that fits is `cap-capturedLen` = the whole 20-byte body regardless
> of `>`-vs-`>=`). CONFIRM the flag check fired, not the payload check. This is why the
> `0100` driver drives a body EXACTLY at length == C.

---

## Findings log

_(IMPL — record any vacuous-proof classes caught at the IMPL, per the 56.1 precedent
where controller-run break verification caught TWO more vacuous-proof classes the PLAN's
review had missed. PROGRESS-56.1 §Findings IMPL-1/IMPL-2 are the template.)_
