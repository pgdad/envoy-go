# Phase 22.3 IMPL — REVIEW.md

**Lifecycle stage:** IMPL phase-done (Task 6 atomic landing); awaiting squash-merge to master. **This commit CLOSES parent row 22** (22.3 is the FINAL sub-phase of the phase-22 3-way pre-split).

**Scope under review:** the 7-task IMPL execution of phase 22.3 (`22.3-http-filter-lua-multi-script-and-per-route`) — the multi-script `Lua.SourceCodes` content-hash registry consume + the `LuaPerRoute` 3-arm per-route override validator + the per-route 3-tier dispatch + the NEW 9th canonical per-route shape (ADR-0125 §(xiv) AMENDMENT, roster 8 → 9) + the upstream-parity dangling-name silent no-op (AMEND-22.3-1) + the no-reserved-name disposition + `FuzzLuaPerRouteConfig` (30 → 31) + the two-directory differential fixtures (29 → 31) + ADR-0193 §Decision + §Consequences body + the BEHAVIOR_CONTRACT.md edit bundle + doc.go + STATE/ROADMAP parent-row-22 closure.

**Review skill:** authored per `superpowers:requesting-code-review` per the phase-21 + phase-22.1 + phase-22.2 IMPL precedent.

**CONSUME + DISPATCH delta:** 0 new framework primitives; 0 net-new stats (count STAYS 107); 0 net-new bridge methods; **0 net-new BEHAVIOR_CONTRACT departure records (count STAYS 14)** — all 22.3 dispositions are upstream-parity. No new ADR number consumed (ADR-0194 STAYS next-free; R6 WEAK-default STANDS).

---

## 1. Green-gate phase-done verification (verbatim outputs)

### Gate A — build + vet + lint

```
$ go build ./... && go vet ./... && golangci-lint run
BUILD_EXIT=0
VET_EXIT=0
LINT_EXIT=0
```

(Empty stdout/stderr on all three; clean build + vet + lint across all packages.)

### Gate B — non-differential test suite

```
$ go test $(go list ./... | grep -v /test/differential) -count=1
... (all packages ok or [no test files])
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	(ok)
ok  	github.com/esalaine/envoy-go/internal/lua	(ok)
ok  	github.com/esalaine/envoy-go/internal/filter/http	(ok)
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.993s
... [fixture inputs packages: no test files] ...
ok  	github.com/esalaine/envoy-go/test/helpers	(ok)
TEST_EXIT=0
```

All non-differential packages PASS (exit 0). The h2spec conformance gate passes UNCHANGED (22.3 does not touch the H2 stack).

### Gate C — race (lua package)

```
$ go test -race ./internal/filter/http/lua/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.940s
RACE_EXIT=0
```

Race-clean. The race-detection-meaningful surface (SourceCodes registry build + per-route `perRouteChunks` memo guarded by `perRouteMu` + per-stream filter isolation + the shared content-hash CompileCache RWMutex discipline) is fully race-clean.

### Gate D — lua differential fixtures (0026 / 0027 / 0028 / 0029)

The combined `-run 'Differential/(0026|0027|0028|0029)'` run surfaced TWO transient failures (0027 + 0028) — both the documented pre-existing `freeTCPPort` multi-listener port-allocation flake (per 22.2 REVIEW §7.4 item 2), NOT a byte-divergence or compile defect:

```
$ go test ./test/differential/ -run 'Differential/0028' -count=1 -v 2>&1 | grep -i bind
2026/.. listener start: listener: "l_test_b": bind 0.0.0.0:44100: listen tcp 0.0.0.0:44100: bind: address already in use
    runner_test.go:719: subj start: subject ready: EOF
```

**Disposition: GREEN with documented transient.** Each fixture passes clean in isolation:

```
$ go test ./test/differential/ -run 'Differential/0026' -count=1   →  ok   (2.000s)
$ go test ./test/differential/ -run 'Differential/0029' -count=1   →  ok   (1.771s)

# 0028 — 4 isolated retries:
attempt 1: ok (2.197s)   attempt 2: ok (2.369s)
attempt 3: FAIL (bind 0.0.0.0:46870: address already in use)   attempt 4: ok (2.193s)

# 0027 — 3 isolated retries:
attempt 1: ok (2.794s)   attempt 2: ok (2.774s)   attempt 3: ok (2.732s)
```

The failure cause is ALWAYS `bind: address already in use` on a multi-listener fixture (0027 = 13 listeners; 0028 = 6 listeners) — another process grabs a picked-free port between `freeTCPPort` pick and bind. Never a byte-divergence, compile-reject mismatch, or scenario-selection error. Matches the 22.2 REVIEW §7.4 item 2 known limitation + the 22.3 PROGRESS Task 5 disposition. The unchanged-regression fixtures 0026 + 0027 (single-run) + the NEW 0028 (cross-side, 6 scenarios) + 0029 (boot-reject) all pass byte-exact / substring-match in isolation.

Fixture 0029 boot-reject confirmation (both proxies fail closed at config-load on the `source_codes{bad}` compile-error; common stderr substring `near '-'`):
```
script load error: [string "function envoy_on_request(handle) end this-is..."]:1: '=' expected near '-'   (reference Envoy)
listener manager: ... lua: source_codes["bad"]: lua compile: lua_filter_chunk line:1(column:43) near '-':   parse error   (envoy-go)
```

### Gate E — main.go + fixture.go ZERO delta

```
$ git diff --stat 04eba88 HEAD -- cmd/envoy-go/main.go test/differential/fixture/fixture.go
(no output — empty = zero delta = PASS)
```

`cmd/envoy-go/main.go` + `test/differential/fixture/fixture.go` have ZERO delta vs the 22.3 base `04eba88` — 22.3 consumes already-parsed proto surfaces + reuses the existing differential framework + the existing boot-registration; no main.go or framework edit. (The two new fixtures registered via +2 blank-import lines in `runner_test.go` — the established per-fixture discovery discipline, not a framework change.)

---

## 2. SPEC §15 ~16-item acceptance-checklist closure

| # | Item | Disposition + evidence |
|---|---|---|
| 1 | CONSUME `Lua.SourceCodes` (per-name DataSource resolve + content-hash compile into the shared `CompileCache` + `name → *Chunk` registry) | **GREEN** — Task 1: `cc.sourceCodes map[string]*internallua.Chunk`; sorted-key iteration; `resolveDataSource` + `CompileScript` into the SHARED per-listener cache; byte-identical-content dedup to one `*Chunk`. Arm-4 deferred reject RETIRED. PROGRESS Task 1. |
| 2 | CONSUME `LuaPerRoute` 3-arm oneof — REPLACE `validatePerRouteLua` with the real validator | **GREEN** — Task 2: NEW `perroute.go::parsePerRouteLua` (oneof-required / disabled-must-be-true / name-min-1 / source_code gauntlet + defensive default); `validatePerRouteLua` delegates. Arm-18 one-liner RETIRED. PROGRESS Task 2. |
| 3 | Per-route 3-tier dispatch (`disabled` skip / `name` registry-with-silent-no-op-on-miss / `source_code` override / fall through to default / else no-op) | **GREEN** — Task 3: `(*filter).resolveDecodeScript` + the D-P1(b') no-re-read memo + the encode-guard fix. PROGRESS Task 3. See §4.1 divergence. |
| 4 | NEW `perroute.go` + `perroute_test.go` | **GREEN** — Tasks 2-3: `perroute.go` (validator + dispatch + memo helper) + `perroute_test.go` (3-arm validator + 9 dispatch-tuple tests + 2 integration tests). |
| 5 | Config-load PARSE-REJECT arms (6 arm-groups per D-P3; arms 3 + 7 NOT present) | **GREEN** — Tasks 1-2: source-codes-key-empty + source-codes-value-gauntlet + 3 PGV-mirror per-route arms + per-route-source_code-gauntlet. Arms 3 (reserved-name) + 7 (dangling-name) DROPPED per BRAINSTORM decision #2 + AMEND-22.3-1. Byte-exact wording pinned per ADR-0080. |
| 6 | 0 net-new stats (SHARED-vacuous); stat count STAYS 107 | **GREEN** — Task 6: `stats.go` UNCHANGED; per-route errors charge to listener-level `lua.<prefix>.errors` (SHARED per ADR-0154; `LuaPerRoute` has no `stat_prefix`). |
| 7 | ADR-0193 §Decision + §Consequences body landed | **GREEN** — Task 6: DECISIONS.md ADR-0193 §Decision (10 sub-items i-x) + §Consequences; Status `Proposed → Accepted`; §Context UNCHANGED from SPEC anchor `e72af4c`. |
| 8 | ADR-0125 §(xiv) IN-PLACE AMENDMENT body landed (roster 8 → 9; no new ADR number) | **GREEN** — Task 6: the `**(xiv)**` clause + the 9-shape roster table + the lua-row first-use citation (referencing ADR-0193). |
| 9 | CONDITIONAL ADR-0194 — ONLY if R6 benchmark gate fires (> 1ms) | **NOT FIRED** — Task 4: R6 WEAK-default STANDS (resolution-only 10.46 ns/op + per-stream 31.47 ns/op, both ~5 orders of magnitude under the 1ms gate). ADR-0194 UNCONSUMED; STAYS next-free. |
| 10 | NEW `FuzzLuaPerRouteConfig` + `FuzzLuaConfigParse` corpus extension; project count 30 → 31 | **GREEN** — Task 4: `FuzzLuaPerRouteConfig` (recover-trap must-never-panic per ADR-0018; 15 branch-mapped seeds + 1 raw-garbage); `FuzzLuaConfigParse` +6 source_codes seeds; `grep '^func Fuzz' \| sort -u \| wc -l = 31`. 30s fuzz: NO crasher. |
| 11 | Differential fixture GREEN (5 cross-side + 3 boot-reject; 29 → 30 single-fixture) | **GREEN as the AUTHORIZED two-directory amendment (29 → 31)** — Task 5: realized as `0028` cross-side (5 selection scenarios + dangling-name no-op) + `0029` boot-reject. The framework's one-branch-per-directory constraint forced the split; (f) key-empty + (h) per-route source_code-failure covered at the unit layer. See §4.2 divergence. |
| 12 | BEHAVIOR_CONTRACT.md edit bundle (0 new departure records; count UNCHANGED at 14) | **GREEN** — Task 6: NEW `#### Phase 22.3 multi-script + per-route surface delta` + `#### Phase 22.3 forward-pointer notes` subsections; the `#### Phase 22.2 forward-pointer notes` 22.3-bullets converted to LANDED; 2 upstream-parity NOTES (dangling-name + no-reserved-name); ADR-0125 §(xiv) cross-reference. Departure count verified 14 before + 14 after (0 new markers added — see §3). |
| 13 | R6 *LState-pool gate disposition recorded (anticipated WEAK-default STANDS) | **GREEN** — Tasks 4 + 6: numbers in ADR-0193 §Decision (viii) + §Consequences + BEHAVIOR_CONTRACT 22.3 forward-pointer notes + STATE.md + ROADMAP. |
| 14 | Parent row 22 flips `in-progress → done` + STATE.md re-advance + ROADMAP row 22.3 flip | **GREEN** — Task 6: ROADMAP row 22.3 `in-progress → done` (date 2026-05-21) + parent row 22 `in-progress → done` (§9 family 4 → 3 remaining) + STATE.md rewrite-in-place (`phase 22.3 IMPL done; phase 22 parent done; awaiting next-phase BRAINSTORM`; next-skill `superpowers:brainstorming`). |
| 15 | Per-task PROGRESS.md entries quoting command outputs per verification-before-completion | **GREEN** — Tasks 0-6 entries in PROGRESS.md, each with verification command outputs quoted verbatim. |
| 16 | REVIEW.md authored at phase-done per requesting-code-review | **GREEN** — THIS file. |

**16/16 items GREEN** (item 9 = correctly NOT-fired; item 11 = the authorized two-directory amendment).

---

## 3. BEHAVIOR_CONTRACT departure-record count — UNCHANGED at 14

The `### envoy.filters.http.lua` departure-record count is **14 before + 14 after** the 22.3 edits (3 from 22.1: stdlib-sandbox-strict + `respond_calls` + runtime-error-log-wording; 11 from 22.2: 5 counters + 2 `:filterState()` + `:dynamicMetadata()` flat + `:body()` return-shape + 4 D8 crypto/fileBytes). The 22.3 edit bundle added ZERO departure-record markers — verified:

```
$ git diff docs/envoy-go/BEHAVIOR_CONTRACT.md | grep '^+' | grep -cF '##### envoy-go-strict departure'   →  0
$ git diff docs/envoy-go/BEHAVIOR_CONTRACT.md | grep '^+' | grep -cE '^\+\*\*envoy-go-strict departure —'  →  0
```

Both 22.3 NOTEs (dangling-name silent no-op + no-reserved-name) are phrased as **"Upstream-parity NOTE"** and each explicitly states "**0 departure records**" — they are upstream-parity observations, NOT envoy-go-strict divergence records.

---

## 4. The TWO PLAN divergences (explicitly documented)

### 4.1 Task 3 — D-P1(b') dispatch correction (no-re-read guarantee)

**Divergence:** The PLAN's File-structure table described `resolveDecodeScript` as "non-nil → `parsePerRouteLua` → …". A literal reading (call the full validator on every per-stream dispatch) re-runs `resolveDataSource` for a per-route `source_code` `Filename` arm — re-reading the file from disk EVERY stream purely to re-validate, discarding the chunk, BEFORE the `perRouteChunks` memo is consulted. This **DEFEATS the D-P1(b') "read+compile the Filename DataSource ONCE per route, never re-read per stream" guarantee.** The initial implementation followed the literal wording; the code-review-mandated read-counting `Filename` test (PLAN Task 3 Step 1: "the file is read once via a read-counting temp file") surfaced it.

**Resolution (owner-confirmed binding intent):** the D-P1(b') no-re-read guarantee is the binding intent; the literal "call `parsePerRouteLua` at dispatch" wording is the error. `resolveDecodeScript` was rewritten to type-assert to `*luav3.LuaPerRoute` and switch on `GetOverride()` DIRECTLY (no per-stream re-validation), letting the memo own the single read. `parsePerRouteLua` is UNCHANGED and stays the HCM-build validator (per-route config is already validated at HCM-build via the ADR-0110 single-chokepoint, so dispatch-time re-validation was redundant). The direct switch is semantically equivalent to the discarded path for the disabled-false (→ listener default) and dangling-name (→ `(nil,false)`) edge cases. The no-re-read guarantee is pinned by `perroute_test.go`'s memo-hit-Filename-not-reread test. Recorded at ADR-0193 §Decision (iv) + doc.go + PROGRESS Task 3.

### 4.2 Task 5 — two-directory fixture amendment (29 → 31, NOT 29 → 30)

**Divergence:** The PLAN's Task 5 specified a SINGLE fixture-0028 hosting "5 cross-side + 3 boot-reject" scenarios (29 → 30). Implementation surfaced a hard framework constraint: the differential runner (`runFixture`) dispatches **exactly ONE branch per fixture directory** — a fixture is EITHER a cross-side wire fixture (`MultiListenerDriver` + `CompareBytes`) OR a boot-reject fixture (`BootRejectFixture`, both-proxies-fail), never both (the boot-reject branch is a top-level early `return` before the cross-side path). Neither the 22.1 (0026) nor 22.2 (0027) precedent hosts both in one directory. Additionally, `runBootRejectFixture` is hardcoded cross-side (asserts BOTH proxies fail), so the PLAN's subject-only (f) key-empty scenario (upstream ACCEPTS an empty key) cannot be a `BootRejectFixture` at all.

**Resolution (owner-authorized):** TWO directories — `0028-http-lua-multi-script-and-per-route` (CROSS-SIDE multi-listener; 5 per-route selection scenarios + the dangling-name no-op) + `0029-http-lua-source-codes-boot-reject` (BOOT-REJECT; `source_codes{bad}` compile-error; common stderr substring `near '-'`). **Fixture count 29 → 31** (NOT the pre-IMPL "30" anticipation). The subject-only (f) `source_codes` key-empty + (h) per-route `source_code`-DataSource-failure arms are covered at the unit layer (Task 1's key-empty byte-exact reject test + Task 2's per-route source_code gauntlet test). SPEC §15 item 11's "5 cross-side + 3 boot-reject single fixture; 29 → 30" is realized as the amended two-directory 29 → 31. Recorded at ADR-0193 §Decision (vii) + §Consequences + PROGRESS Task 5.

---

## 5. R6 disposition — WEAK-default STANDS; ADR-0194 UNCONSUMED

`BenchmarkPerStream_PerRoute_Resolution` (Task 4):

```
BenchmarkPerStream_PerRoute_Resolution/resolution-only   10.46 ns/op    0 B/op   0 allocs/op
BenchmarkPerStream_PerRoute_Resolution/per-stream        31.47 ns/op   48 B/op   2 allocs/op
```

Both ~5 orders of magnitude under the 1 ms (1,000,000 ns) R6 gate. Per-route resolution is an O(1) `Resolve` + content-hash + proto-pointer-memo lookup (warm-memo hot path zero-alloc), NOT a new per-stream VM construction — confirming the SPEC §1.2 + §11.2 + §11.4 WEAK-HOLD prediction. **Conditional ADR-0194 does NOT fire; next-free ADR STAYS ADR-0194.** Parent row 22 is CLOSED — no successor sub-phase to carry the escape-valve buffer.

---

## 6. Per-task review summary (Tasks 0-6)

| Task | Tier | Summary | Two-stage review outcome |
|---|---|---|---|
| 0 | — | PROGRESS preamble + precondition verification (fuzzer 30 + fixtures 29 baselines confirmed; build/vet/lint/tests green). | n/a (verification task). |
| 1 | A | `Lua.SourceCodes` consume → `cc.sourceCodes` content-hash registry + `perRouteChunks`/`perRouteMu` (pre-declared) + `source-codes-key-empty` arm; arm-4 deferred reject DROPPED. | spec-compliance ✅; code-quality 1 Important (missing wording-pin row per ADR-0080) + 2 Minors — all fixed + re-verified green. |
| 2 | A | NEW `perroute.go` `parsePerRouteLua` real 3-arm validator; arm-18 one-liner DROPPED; 4 byte-exact wording consts. | spec-compliance ✅; code-quality 2 Important (stale doc-comments) + 3 Minors (const-name; `errors.New`; defensive default arm) — all fixed + re-verified green. |
| 3 | B | Per-route 3-tier dispatch `resolveDecodeScript` + decode/encode wiring + encode-guard fix. **D-P1(b') no-re-read correction (§4.1).** | spec-compliance ✅; code-quality 0 Critical/Important + 2 Minors (stale nolint; under-pinned no-re-read test → strengthened, which surfaced the §4.1 bug). Focused re-review confirmed all 9 dispatch tuples + no-re-read guaranteed. |
| 4 | C | `FuzzLuaPerRouteConfig` (30 → 31) + corpus extension + `BenchmarkPerStream_PerRoute_Resolution`. **R6 WEAK-default STANDS.** | spec-compliance ✅; code-quality APPROVED (0 Critical/Important; 1 cosmetic stale docstring → fixed). 30s fuzz NO crasher. |
| 5 | D | TWO differential fixtures (0028 cross-side + 0029 boot-reject). **Two-directory amendment 29 → 31 (§4.2).** | independent spec-review confirmed NON-trivial passes (hit-vs-no-op distinguishable; dangling-name distinct from default; disabled distinct from default). main.go + fixture.go ZERO delta verified. |
| 6 | E | Atomic doc landing: ADR-0193 §Decision + §Consequences + ADR-0125 §(xiv) AMENDMENT + BEHAVIOR_CONTRACT bundle + doc.go + STATE/ROADMAP parent-row-22 closure + REVIEW.md + green gate. | THIS review. All green gates GREEN; departure count 14; next-free ADR-0194; main.go + fixture.go zero delta. |

---

## 7. Reviewer notes — cross-cutting observations

**Test discipline.** TDD-first per task per `superpowers:test-driven-development`; every NEW config-load arm has byte-exact wording test coverage per ADR-0080; the D-P1(b') no-re-read guarantee is pinned by a read-counting `Filename` test (which surfaced the §4.1 divergence). Race-clean under `-race` on the lua package. Every PROGRESS task entry quotes verification command outputs verbatim per `superpowers:verification-before-completion`.

**ADR discipline.** ADR-0193 §Decision + §Consequences body REPLACES the SPEC-commit-anchored §Context placeholder posture at THIS Task 6 commit (Status `Proposed → Accepted`; §Context UNCHANGED from `e72af4c`) per ADR-0044 in-place edit discipline. The ADR-0125 §(xiv) IN-PLACE AMENDMENT body lands as the `**(xiv)**` clause + the 9-shape table (no new ADR number) per the phase-13/14/15/16/17 in-place-amend-at-IMPL precedent. NO new ADR consumption (ADR-0194 STAYS next-free — R6 WEAK-default STANDS). The ADR-0125 roster 8 → 9 ENDS the FIVE-CONSECUTIVE ADR-0125-skip streak (phases 18/19/20/21/22.1/22.2).

**Atomic-landing discipline.** The BEHAVIOR_CONTRACT.md edit bundle + STATE.md re-advance + ROADMAP row 22.3 + parent row 22 flip + REVIEW.md authoring + ADR-0193 §Decision + §Consequences body + ADR-0125 §(xiv) AMENDMENT body + doc.go + PROGRESS final entry — all in the same atomic Task 6 commit per ADR-0052.

**Known limitation (not a defect).** The multi-listener fixtures 0027 + 0028 inherit the pre-existing `freeTCPPort` port-allocation race (22.2 REVIEW §7.4 item 2): a fixture can transiently fail with `bind: address already in use` when another process grabs a picked-free port between pick and bind. Both pass clean in isolation. A contiguous-port reservation helper would close the gap (future test-helper work).

**No open issues at phase-done.** All 16 acceptance items GREEN (item 9 correctly NOT-fired; item 11 the authorized two-directory amendment). All green gates GREEN. Both PLAN divergences disposition-recorded. Phase 22.3 IMPL is READY FOR SQUASH-MERGE TO MASTER per project memory `feedback_git_worktrees.md` + ADR-0005 + `feedback_push_to_origin.md` (push to origin after the clean squash-merge with tests green). **This commit CLOSES parent row 22.**

---

## 8. Squash-merge handoff

**Branch:** `phase-22.3-http-filter-lua-multi-script-and-per-route-impl`
**Worktree:** `/home/esa/git/envoy-go/.worktrees/phase-22.3-http-filter-lua-multi-script-and-per-route-impl`
**Predecessor master tip:** `04eba88` (`phase 22.3 PLAN follow-up: STATE.md SHA-fill (TBD → 1efd4de post-squash)`)
**Squash-merge target:** `master`
**Post-squash SHA-fill follow-up:** `phase 22.3 IMPL follow-up: STATE.md SHA-fill (TBD → <squash-SHA> post-squash)` per the phase-09..22.2 convention.

**Squash-merge commit message** (per the project's phase-09..22.2 squash convention):

```
Squash merge phase-22.3-http-filter-lua-multi-script-and-per-route-impl
```

All 7 Tasks (Task 0 + Tasks 1-6) landed per the worktree branch's sequential commit history. Post-squash, the branch can be deleted + the worktree removed per `superpowers:finishing-a-development-branch`. **This squash-merge CLOSES parent row 22 + lands ADR-0193 full body + the ADR-0125 §(xiv) AMENDMENT.**
