# Phase 56.1 (http-tap-filter, headers leg) IMPL — PROGRESS

**SPEC:** `SPEC-56.1.md`
**PLAN:** `PLAN-56.1.md`
**Worktree branch:** `phase-56.1-http-tap-filter-impl` (to be created at the IMPL)

> Scaffolded at the **PLAN** stage (docs-only). Every checkbox below is UNCHECKED:
> no production code exists yet. The IMPL's Task 1 re-runs the baseline commands
> against its own cold-start HEAD and pastes the literal output into
> "Baseline Counts", then ticks tasks as they land.

---

## Task Checklist (15 tasks — AT the ADR-0045 `~15` gate, margin ZERO)

- [x] T1  PROGRESS baselines + FINAL ADR-0045 split re-check
- [x] T2  `internal/headermatch`: `stringMatcher` (5 arms + `ignore_case`; `custom` rejected) [TDD]
- [x] T3  `internal/headermatch`: `Matcher` (8 arms + `invert_match` + `treat_missing_header_as_empty`) + `Lowercase` [TDD]
- [x] T4  `internal/matchpredicate`: node types + `Compile` (6 accept / 4 explicit rejects / depth cap 32) [TDD]
- [x] T5  `internal/matchpredicate`: `Evaluator` (feed + tri-state `Resolve`; Undetermined ⇒ false) [TDD]
- [x] T6  `FuzzMatchPredicateCompile` (+depth-33/512 seeds exercising the cap); fuzzers 52 → 53 [fuzz]
- [x] T7  `internal/filter/http/tap`: config parse + FULL PARITY/DEPARTURE reject roster + `rq_tapped` (+1 delta guard) [TDD]
- [x] T8  Dual-sided capture: `:status` on a COPY, lowercase, sort + the wire-leak regression [TDD]
- [x] T9  `OnDestroy` emit + trace assembly + `record_downstream_connection` + ONE-SHARED-VALUE pins [TDD]
- [x] T10 `filePerTapSink`: per-stream file, monotonic trace-id, `MkdirAll` parent + byte-exact protojson golden [TDD]
- [x] T11 `doc.go` + `builtins.go` registration arm (20 → 21 registered; 19 → 20 production)
- [x] T12 Harness: `fixture.HostMount.Dir` + runner directory pre-create (+ the `0006` file-mount regression) [TDD]
- [x] T13 Fixture `0099-http-tap-headers`: YAMLs + driver (GET → 204, N=3 match + M=2 non-match); fixtures 100 → 101
- [x] T14 `0099` `AssertStats` glob-and-decode assertions + FIVE deliberate breaks + flake/race/full-suite gates
- [x] T15 Docs bundle: ADR-0273 + `BEHAVIOR_CONTRACT` + `STATE`/`ROADMAP`/`README` + fuzzer reconcile

---

## Baseline Counts

Re-recorded at the **IMPL** stage against cold-start HEAD `340964ef` (the IMPL's T1 MUST re-run
these against its own cold-start HEAD and replace this block with its literal output —
do not assume they still hold):

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l
100

$ grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l
52

$ grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1
16514:## ADR-0272 — the plain-statsd `tcp_cluster_name` transport ...

$ grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1
606:	H2GoawayResponder BackendKind = 38
```

Baseline summary:
- stat surface: **1200** (H2 cluster; non-H2 **1196**) — a live reference-Envoy probe figure, **not** an in-tree assertion. The tree enforces a *delta* (`internal/statssink/registration_test.go`'s `countMetrics` pattern); T7 adds the tap `+1` guard.
- fixtures: **100** (tail `0098-stats-sink-statsd-tcp`)
- fuzzers: **52**
- BackendKind tail: **38** (`H2GoawayResponder`) — **stays 38** this leg (the `0099` backend reuses `HTTPStatusHeader = 3`)
- DECISIONS tail: **ADR-0272** (next-free **ADR-0273**)

Anticipated at 56.1 IMPL exit: stat surface **1201** · fixtures **101** · fuzzers **53** · BackendKind **38** · DECISIONS tail **ADR-0273** · new Go packages **2** + the filter package · new go.mod modules **0**.

---

## FINAL ADR-0045 split re-check (D-TAP-SPLIT-FINAL) — discharged at the PLAN

The SPEC's sketch was ~14 tasks; its escape valve reads *"if the PLAN's decomposition
exceeds ~15 tasks, re-open ADR-0045 before writing code."*

The honest first decomposition came to **16**. Two **semantic** folds bring it to **15**:

1. `record_downstream_connection` folds into **T9** — populating `HttpBufferedTrace.DownstreamConnection` *is* part of assembling the trace; one deliverable, not two.
2. The reject-roster and byte-stability unit tests fold into **T7** and **T10** — the tests that prove a behavior belong in the task that introduces it (TDD).

Neither fold hides work. **15 ≤ ~15: the gate HOLDS**, with **zero margin**.

A further 56.1a/56.1b **by-layer** sub-split was CONSIDERED and REJECTED: the
BRAINSTORM's *(Q6)* already ruled it out because it would strand
`internal/matchpredicate` as dead library code with no differential surface, and the
phase-46.1a/46.1b precedent does not transfer (there, *both* halves had an observable
emit surface; here the library half has none).

**Standing instruction for the IMPL:** if any task grows a second deliverable, split it
into a NEW task and **re-open the gate**. Do **not** push spillover into 56.2.

---

## Deliberate-break ledger (filled in at the IMPL; record the LITERAL failing text)

The controller re-performs every break itself. A break failing does NOT prove the
intended assertion is live — it can abort earlier and MASK it
(`reference_deliberate_break_wrong_assertion`). Confirm **which** assertion fired.

All `0099` breaks edit **subject-side** production code, so the **subject** trace must be the one that violates. Confirm the failure text names `subject/...`.

All breaks were re-performed BY THE CONTROLLER with `-count=1`; the `0099` breaks (rows 8-14) all fired on the **subject** side.

| # | Task | Break | Must fire | Fired? (literal text) |
|---|---|---|---|---|
| 1 | T7 | delete the `tap_enabled` guard | `TestNew_RejectRoster/tap_enabled_set` | `config_test.go:191: expected reject, got nil error` (`TestNew_RejectRoster/tap_enabled_set`) |
| 2 | T7 | delete the `streaming_grpc` guard | `TestNew_RejectRoster/sink_streaming_grpc` | `config_test.go:197: sink_streaming_grpc: error "tap: unsupported output_sink_type *tapv3.OutputSink_StreamingGrpc" does not contain "streaming_grpc sink is not supported"` |
| 3 | T7 | delete the neither-match guard | `TestNew_RejectRoster/neither_match_nor_match_config` | `config_test.go:197: neither_match_nor_match_config: error "tap: match: matchpredicate: nil MatchPredicate" does not contain "neither match nor match_config is set"` |
| 4 | T8 | write `:status` into the wire-bound map | `TestEncodeHeaders_NeverMutatesTheWireBoundMap` | `tap_test.go:49: EncodeHeaders MUTATED the wire-bound map: got map[:status:[204] Content-Type:[text/plain]]` + `tap_test.go:52: :status leaked into the wire-bound header map` |
| 5 | T9 | split: `Decoder: f, Encoder: &tapFilter{…}` | `TestFactory_InstallsOneSharedValueInBothFields` | `emit_test.go:149: Decoder and Encoder must be the SAME *tapFilter value; a two-value split makes the encoder OnDestroy unreachable (chain.go:670)` (`TestFactory_InstallsOneSharedValueInBothFields`) |
| 6 | T9 | split: `Decoder: &tapFilter{…}, Encoder: f` | `TestChainDestroy_EmitsExactlyOnce` → 0 files, `rq_tapped` 0 | `emit_test.go:188: trace files = 0, want exactly 1` + `emit_test.go:191: rq_tapped = 0, want 1` (`TestChainDestroy_EmitsExactlyOnce`) |
| 7 | T10 | `EmitUnpopulated: true` | `TestMarshal_ByteExactGolden` — **canonJSON structure compare + the `"body"` substring pin** (updated from "raw bytes": Go's `protojson` `internal/detrand` build-seeded whitespace randomization makes a literal byte-exact golden incompatible with `-race`; see IMPL-3 below) | `sink_test.go:74: protojson structure drift` with `"body": null` appearing (canonJSON structure compare + the `"body"` substring pin both fire) |
| 8 | T14(a) | emit at `DecodeHeaders` | `0099` (1)+(2): 0 traces, `rq_tapped` 0 | `subject trace count = 0, want 3` + `subject http.tap_probe.tap.rq_tapped = 0, want 3` |
| 9 | T14(a′) | `bt.Response = nil` | `0099` (4): `response.headers` missing `:status` | `subject/out_1.json response.headers: missing key ":status" (have [])` (+ content-type; out_2, out_3) |
| 10 | T14(b) | `Resolve()` always true | `0099` (1)+(2): 5 traces, `rq_tapped` 5 | `subject trace count = 5, want 3` + `subject http.tap_probe.tap.rq_tapped = 5, want 3` |
| 11 | T14(c) | `EmitUnpopulated` in the sink | `0099` **(5b) ONLY** — the raw-bytes check. **(5) must NOT fire.** | `subject/out_1.json: raw trace must contain NO "body" key ...` — **(5b) ONLY; assertion (5) did NOT fire** (`"body": null` decodes to a nil Body) |
| 12 | T14(c′) | fabricate `Body{AsString:"x"}` | `0099` (5) *and* (5b) | `subject/out_1.json: request.body must be ABSENT (bodyless GET); got as_string:"x"` **(5)** + `raw trace must contain NO "body" key` **(5b)** |
| 13 | T14(d) | populate `Message.trailers` | `0099` (6): subject `request.trailers` non-empty | `subject/out_1.json: request.trailers must be empty; got 7` |
| 14 | T14(e) | populate `DownstreamConnection` unconditionally | `0099` (7): subject `downstream_connection` non-nil | `subject/out_1.json: downstream_connection must be ABSENT; got ` |

> **Do NOT add a T9 break that drops `Decoder` entirely.** Any test dereferencing `hf.Decoder` would panic on a nil interface conversion — a crash, not a proof. That framework fact is pinned by `chain_test.go`'s `TestDestroy_EncoderOnlyFilterWithNoDecoderFires`.

---

## Findings log

**PLAN stage — two defects caught by adversarial review, before any code was written.**
Both reviewers were dispatched with the instruction to re-derive every claim from
source and never to check a citation against the document that made it
(`feedback_brief_citations_not_evidence`). Both independently found defect (1); one
proved it by execution. The controller then re-verified it independently.

1. **The `body`-absence assertion had NO liveness proof (would have shipped vacuous).**
   The draft's deliberate break (c) set `EmitUnpopulated: true` and expected the
   decoded `request.body` to become non-nil. **False.** `protojson.Unmarshal` maps
   `"body": null` to a **nil** `Body`, exactly like an omitted field, and
   `EmitUnpopulated` is a superset of `EmitDefaultValues` (so `raw_value: ""` and
   `trailers: []` still render). Under break (c) the entire `0099` fixture stayed
   **GREEN — no assertion fired at all.** Since the fixture is deliberately bodyless
   on both sides, assertion (5) is a tautology on every normal run; it could only
   ever be shown live by a break that produces a genuinely *present* body.
   Measured:
   ```
   BAD file contains "body": null:                       true
   after decoding it: req.Body==nil? true  resp.Body==nil? true
   would assertion(5) `GetBody()!=nil` fire?             false
   ```
   **Fix:** split the property in two — assertion **(5)** stays a decoded
   `GetBody()==nil` check, and a new assertion **(5b)** greps the RAW file bytes for
   any `"body"` key. Break (c) now proves (5b); a new break **(c′)** (fabricate a
   real `Body{AsString:"x"}`) proves (5). A decode-based check structurally cannot
   distinguish "omitted" from "null".

2. **The ONE-SHARED-VALUE proof was self-defeating.** The draft's
   `TestChainDestroy_EmitsExactlyOnce` did `f := hf.Decoder.(*tapFilter)` and fed
   both header sets into `f`. Under the two-value split the break was meant to
   detect, `hf.Decoder` **is** the fresh value — so it received both header sets,
   matched, and emitted. The break **passed**, proving nothing about the leg's
   central constraint. **Fix:** drive the INTERFACE values (`hf.Decoder.DecodeHeaders`,
   `hf.Encoder.EncodeHeaders`) so a split lands the two header sets on two different
   values and `Destroy()`'s Decoder-only branch emits nothing.
   Relatedly, a proposed third break ("drop `Decoder` entirely; must PASS") would have
   **panicked** on `nil.(*tapFilter)`, not passed. It was removed.

3. **MINOR, corrected in place:** `recordingFilter.destroyed` is `atomic.Int32`
   (`chain_test.go:39`), not `Int64`; `emit_test.go`'s import block had an unused
   `os` and a missing `internal/stats`; `headermatch.Lowercase`'s intermediate slice
   copy is redundant (`append(out[lk], v...)` already copies).

4. **UNPROVEN, demoted from "verified":** `content-type: text/plain` surviving a
   **204** through *both* proxies. Only the backend's emission was probed. Fails safe
   (goes RED at T14 Step 2, not vacuous). Confirm live at the IMPL; if stripped, move
   it to the UNasserted list and accept a one-key `response.headers` assertion.
   **RESOLVED at the IMPL: content-type SURVIVED end-to-end on both proxies**
   (D-TAP-SUBSET is confirmed as originally stated — `response.headers ⊇
   {:status 204, content-type text/plain}` — NOT moved to the UNasserted list).

**IMPL findings**

**IMPL-1 (T7): the plan's breaks #2/#3 were VACUOUS as designed.** Deleting the
`streaming_grpc` case fell through to the defensive `default` arm, and deleting the
neither-match guard fell through to `matchpredicate.Compile(nil)`; both still
rejected, so the `err==nil`-only `TestNew_RejectRoster` stayed green. Hardened the
test to assert per-arm error SUBSTRINGS (ADR-0080 arm-distinctness) — commit
`95b1997a`. All 3 breaks now fire on their exact subtest.

**IMPL-2 (T9): the ONE-SHARED-VALUE dynamic proof was VACUOUS.**
`TestChainDestroy_EmitsExactlyOnce` used `validTap()`'s REQUEST-ONLY predicate;
under the two-value split the decoder value resolves True on the request arm alone
→ still 1 file/rq_tapped 1 → break did not bite (confirmed: the split ran `ok`).
The plan's interface-driven fix was insufficient for a request-only predicate.
Fix: new `validTapReqAndResp()` with `and_match{req x-tap=yes, resp :status=204}`
so a decoder-only split stays Undetermined → 0 files — commit `f99f77fa`. Break now
fires "trace files = 0, want exactly 1".

**IMPL-3 (T10): protojson detrand vs the byte-exact golden — USER-DECIDED.** Go's
`protojson` uses `internal/detrand`, which injects build-seeded whitespace
randomization; a literal byte-exact golden passes under plain `go test` but
deterministically FAILS under `-race` (a different binary → different detrand
seed). The plan mandated BOTH a byte-exact golden AND running the package under
`-race` — physically incompatible. Resolution (user-approved): the golden is
compared as CANONICAL JSON (`encoding/json.Compact` strips only insignificant
whitespace, preserves the token stream) plus detrand-robust substring pins; the
`want` golden string is kept verbatim as documentation. This still catches an
EmitUnpopulated regression (`"body":null` is a real token — verified live) and is
build-independent. The T10 break-ledger row 7 note is updated from "raw bytes" to
"canonJSON structure compare + the `"body"` substring pin" (reflected in the table
above).

**Minor findings (roll-up, not blocking):** stringmatch `ToLower` is Unicode vs
Envoy's ASCII (headers are ~always ASCII); `driver.go`'s
`bytes.HasPrefix([]byte(s),...)` could be `strings.HasPrefix`; `pollTraces` uses
`len>=want` (harmless; break (b) confirmed (1) fires with 5); the
`ReferenceLogMounter` doc comment doesn't mention the `Dir=true` branch.
