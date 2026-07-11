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

- [x] T1  PROGRESS baselines + ADR-0045 split re-check (docs)
- [x] T2  `config.go`: store `format` (bodyAsString) + resolve both caps (nil → 1024; present incl. 0) [TDD]
- [x] T3  `tap.go`: `DecodeData`/`EncodeData` accumulation — append up to cap, strict-`>` truncation, saw-body flag, always `DataContinue` [TDD]
- [x] T4  `trace.go`: `bodyProto` (oneof from format, AS_STRING `ToValidUTF8` sanitize) + `buildTrace` wiring (Body iff hook fired) [TDD]
- [x] T5  Fixture `0100-http-tap-bodies`: YAMLs + driver (three POSTs vs `HTTPEchoBody`); fixtures 101 → 102
- [x] T6  `0100` `AssertStats`: cross-side body payload + truncation + the 4 deliberate breaks
- [x] T7  Docs bundle: ADR-0274 body + `BEHAVIOR_CONTRACT` + `ROADMAP` (row 56 `done`) + `STATE`/`README`/`PROGRESS` + count reconcile

---

## Baseline Counts

Re-recorded at the **IMPL** stage against its own cold-start HEAD `8d6ef087` (the
56.2 PLAN squash) — NOT the PLAN-time `1b344286`. Literal output:

```
$ git log --oneline -1
8d6ef087 phase 56.2 (http-tap-filter, bodies leg) PLAN: a 7-task TDD spine ...

$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
101
$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | tail -1
test/fixtures/0099-http-tap-headers

$ grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l
53

$ grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1
16542:## ADR-0273 — the HTTP tap filter, headers leg ...

$ grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1
606:	H2GoawayResponder BackendKind = 38
```

Baseline summary (confirmed, unchanged from the PLAN-time expectation):
- stat surface: **1201** (a live reference figure; the tree enforces a *delta* — 56.2 adds **+0**, no delta-guard change)
- fixtures: **101** (tail `0099-http-tap-headers`)
- fuzzers: **53**
- BackendKind tail: **38** (`H2GoawayResponder`) — **stays 38** (the `0100` backend reuses `HTTPEchoBody = 6`)
- DECISIONS tail: **ADR-0273** (next-free **ADR-0274**)

Anticipated at 56.2 IMPL exit: stat surface **1201** (+0) · fixtures **102** (`0100-http-tap-bodies`) · fuzzers **53** (+0) · BackendKind **38** (+0) · DECISIONS tail **ADR-0274** (next-free ADR-0275) · new Go packages **0** · new go.mod modules **0** (`go mod tidy -diff` EMPTY) · **ROADMAP row 56 → `done`** (the final leg — `reference_roadmap_split_phase_row_done` + ADR-0106).

### T7 count reconcile (re-run against the worktree HEAD, literal output)

```
$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
102

$ grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l
53

$ grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1
16571:## ADR-0274 — the HTTP tap filter, bodies leg (`envoy.filters.http.tap`) (the NINTH Observability-family row's SECOND and FINAL leg ...

$ grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1
606:	H2GoawayResponder BackendKind = 38

$ go build ./... && echo BUILD_OK
BUILD_OK

$ go mod tidy -diff && echo TIDY_EMPTY
TIDY_EMPTY
```

All five counts match the anticipated exit values above exactly (fixtures 101 → **102**;
fuzzers **53** +0; DECISIONS tail **ADR-0274**; BackendKind tail **38** +0; build clean;
`go mod tidy -diff` empty). Per the T7 brief, the full `go test ./...` suite is NOT run
here — the controller runs it at stage-close on the frozen HEAD.

### Seam re-derivation at IMPL (`feedback_brief_citations_not_evidence`) — ALL HOLD

Re-derived from source in the worktree before dispatching Task 2 (NOT copied from
the PLAN/SPEC — the PLAN itself caught two stale SPEC citations, so citations are
never evidence):

- `config.go`: `config` struct `:24-29`, `parseConfig` `:51`, `oc := sc.GetOutputConfig()` `:100`, format switch `:113-122`, `cfg := &config{}` `:149`.
- `tap.go`: `tapFilter` struct `:16-27`, inert `DecodeData`/`EncodeData` `:60-65`.
- `trace.go`: `buildTrace` `:68-88`, Request/Response Message assembly `:70-78`; `marshalOpts` in `sink.go:26`.
- proto (`go-control-plane/envoy@v1.32.4`): `OutputConfig.GetMaxBufferedRxBytes()`/`GetMaxBufferedTxBytes()` → `*wrapperspb.UInt32Value` (`config/tap/v3/common.pb.go:598/605`); `Body_AsBytes`/`Body_AsString`/`GetTruncated` (`data/tap/v3/common.pb.go:109/115/98`); `OutputSink_JSON_BODY_AS_BYTES=0`, `AS_STRING=1` (`config/tap/v3/common.pb.go:43/51`).
- test files: `config_test.go` (`validTap`/`validTapReqAndResp` helpers), `tap_test.go` (`newTapFilter`), `emit_test.go` (buildTrace/marshal tests land here), `sink_test.go`.

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
| 1 | T2 | `resolveCap`: treat present-0 as unset (→1024) | `TestParseConfig_CapResolution/present_ZERO` (`maxRx=1024, want 0`) | `config_test.go:296: maxRx = 1024, want 0` (+`:299` maxTx) on subtest `present_ZERO_->_0_(NOT_1024)` |
| 2 | T3(a) | `accumulate`: strict `>` → `>=` | `TestDecodeData_AtCapNotTruncated` (`reqTrunc=true, want FALSE`) | `tap_test.go:164: reqTrunc = true, want FALSE` |
| 3 | T3(b) | gate saw-body flag behind `len>0 && cap>0` | `TestDecodeData_CapZeroNonEmpty` (`sawReqBody=false, want true`) | `tap_test.go:201: sawReqBody = false, want true` |
| 4 | T3(c) | return `DataStopIterationAndBuffer` | `TestDecodeData_SingleChunkUnderCap` (status `!= DataContinue`) | `tap_test.go:118: DecodeData status = 1, want DataContinue` |
| 5 | T4(a) | swap the AS_STRING oneof → `Body_AsBytes` | `TestBodyProto_AsString` (as_string missing / as_bytes present) | `sink_test.go:184: AS_STRING render missing as_string` + `sink_test.go:187: AS_STRING must NOT render as_bytes` (payload showed `as_bytes:MDEyMzQ1Njc4OQ==`) |
| 6 | T4(b) | drop the `ToValidUTF8` sanitize | `TestBodyProto_AsStringSanitizesNonUTF8` (marshal Fatalf on invalid UTF-8) | `sink_test.go:216: marshal: proto: field envoy.data.tap.v3.Body.as_string contains invalid UTF-8` |
| 7 | T4(c) | gate `Body` on `len(buf)>0` | `TestBuildTrace_BodyPresentWhenHookFired` (empty-but-truncated body absent) | `emit_test.go:205: empty-but-truncated body must be PRESENT (hook fired)` (+`:208` truncated:true) |
| 8 | T4(d) | set `Body` unconditionally | `TestBuildTrace_BodyOmittedWhenHookNeverFired` (RAW-BYTES `"body"` present) | `emit_test.go:223: body must be OMITTED when the hook never fired` (RAW-BYTES) |
| 9 | T6(a) | leave `DecodeData`/`EncodeData` inert | `0100`: `request.body`/`response.body` PRESENT (all arms) | `runner_test.go:1317: subject/out_1.json: request.body must be PRESENT (non-empty POST); got nil` (+response, +out_2/3; P2/P3 cascade empty; ref unaffected) |
| 10 | T6(b) | ignore the cap (always append full) | `0100`: truncated arm `truncated==true` + 20-byte-prefix `as_string` | `subject request.body.as_string multiset = [...ABCDEFGHIJKLMNOPQRSTUVWXYZabcd], want [...ABCDEFGHIJ]` + `subject request.body.truncated multiset = [false false false], want [false false true]` (+response; P1 unaffected) |
| 11 | T6(c) | strict `>` → `>=` | `0100`: **BOUNDARY arm `truncated`-flag ONLY** (payload UNCHANGED) | `subject request.body.truncated multiset = [false true true], want [false false true]` (+response) — **P3 (flag) ONLY; P2 (payload) did NOT fire** (the strict-`>` proof, confirmed per `reference_deliberate_break_wrong_assertion`) |
| 12 | T6(d) | wrong oneof for AS_STRING (emit `as_bytes`) | `0100`: `request.body.as_string`/`response.body.as_string` decode empty | `subject request.body.as_string multiset = ["" "" ""], want [...]` (+response); P3 truncated unaffected |

> **Break (c) is the strict-`>` proof and is subtle** (`reference_deliberate_break_wrong_assertion`):
> only the BOUNDARY arm's `truncated`-flag assertion may fire; the captured PAYLOAD is
> UNCHANGED (the prefix that fits is `cap-capturedLen` = the whole 20-byte body regardless
> of `>`-vs-`>=`). CONFIRM the flag check fired, not the payload check. This is why the
> `0100` driver drives a body EXACTLY at length == C.

---

## Findings log

**FINDING IMPL-1 (controller-caught during `-race` verification, FIXED).**
`TestBuildTrace_BodyPresentWhenHookFired_EmptyButTruncated` (`emit_test.go`, from T4)
matched the `truncated` flag via raw bytes `"truncated": true`/`"truncated":true` (the
0/1-space `EmitDefaultValues` spellings). Under `-race`, `internal/detrand` emits the
2-space form `"truncated":  true` — NEITHER pattern matched — so `emit_test.go:208`
fired. This re-introduced the exact protojson-raw-byte fragility ADR-0273 §Consequences
forbids; the sibling `TestBodyProto_*` tests were already robust (they use `canonJSON`).
Fixed by compacting via `canonJSON` before the `truncated` match (commit `e5fe29e3`,
test-only). Verified: `tap` package `-race` + normal both green, gates clean. The T4
reviewer + normal gates missed it because it only surfaces under a detrand-alternate
build (`-race`) — a coverage note for future byte-bearing protojson assertions in this
package: route through `canonJSON`, never a raw substring match.
