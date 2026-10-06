# Phase 101 (listener-filters-timeout-envelope-lift) — IMPL progress

Base SHA: `09b612f95479d2061590c97e99ac418d32839d7e` (master). Worktree
`/home/esa/git/envoy-go-wt-p101impl`, branch `wt-phase-101-impl`.

## Task 1: Baseline, a proven-live panic gate, and PROGRESS.md

**Step 1** — worktree pre-existing (skipped `worktree add`). `git -C $W rev-parse --abbrev-ref HEAD` = `wt-phase-101-impl`; status clean at start.

**Step 2** — `go list ./internal/listener/... | wc -l` -> **3** (non-zero, no `[setup failed]`).

**Step 3** — bare-tip roster:
```
out=$(go test -count=1 -v ./internal/listener/... 2>&1); rc=$?
rc=0 RUN=274 FAIL=0
```
Matches the expected `rc=0 RUN=274 FAIL=0`. Roster saved as `base-listener.txt`.
Selector `./internal/listener/... ./internal/stats/...` (`go test -count=1 -v`): `rc=0`, `=== RUN` count **470**, FAIL=0 (equal to phase 100's figure; measured, not inherited).

**Step 4** — panic gate live: a throwaway `internal/listener/p101gate_test.go` with `panic("p101-gate")`;
`go test -count=1 -run TestP101Gate ./internal/listener/` -> rc=1, `/usr/bin/grep -cE '^panic:|DATA RACE|SIGSEGV'` = **1**
(`panic: p101-gate [recovered, repanicked]`). File deleted; `git status --short` prints nothing.

## Task 2: The parse and pipeline-hold arms — 13 of 15 RED at the un-fixed tip

Appendix C applied (`ext C $P`, `git apply --check` first). `git diff --numstat` -> `206	0	internal/listener/listener_filters_timeout_test.go`; `wc -l` -> **544**. `go vet ./internal/listener/` rc 0.
Selector `TestParseListenerFiltersTimeout(AboveUint32|Negative|AboveMax|ProtoMax|MaxAccepted|SubMillisecond|TruncatesNot|HalfSecond|Ninety|SixtyOne|ZeroParses|NilParses)|TestListenerFilterTimeoutPipelineHolds`: `=== RUN` count **15**.
At the tip: 13 `--- FAIL`, 2 `--- PASS` (`…ZeroParsesToZero`, `…NilParsesToDefault`). Every failure line (all envelope message / expected reasons):
```
    listener_filters_timeout_test.go:388: 4294968s: accept: got error listener: "l_lft": listener_filters_timeout 1193h2m48s is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:394: -1s: reject message: "listener: \"l_lft\": listener_filters_timeout -1s is outside the supported [1s, 60s] envelope" does not contain "expected a positive duration"
    listener_filters_timeout_test.go:400: -0.5s: reject message: "listener: \"l_lft\": listener_filters_timeout -500ms is outside the supported [1s, 60s] envelope" does not contain "expected a positive duration"
    listener_filters_timeout_test.go:406: 9223372036s: reject message: "listener: \"l_lft\": listener_filters_timeout 2562047h47m16s is outside the supported [1s, 60s] envelope" does not contain "duration out of range"
    listener_filters_timeout_test.go:412: 315576000000s: reject message: "listener: \"l_lft\": listener_filters_timeout 2562047h47m16.854775807s is outside the supported [1s, 60s] envelope" does not contain "duration out of range"
    listener_filters_timeout_test.go:418: 9223372035.999999999s: accept: got error listener: "l_lft": listener_filters_timeout 2562047h47m15.999999999s is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:424: 0.0005s: accept: got error listener: "l_lft": listener_filters_timeout 500µs is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:429: 0.0019s: accept: got error listener: "l_lft": listener_filters_timeout 1.9ms is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:434: 0.5s: accept: got error listener: "l_lft": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:439: 90s: accept: got error listener: "l_lft": listener_filters_timeout 1m30s is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:445: 61s: accept: got error listener: "l_lft": listener_filters_timeout 1m1s is outside the supported [1s, 60s] envelope, want accepted
    listener_filters_timeout_test.go:536: 9223372035.999999999s: pipeline: parse rejected: listener: "l_lft": listener_filters_timeout 2562047h47m15.999999999s is outside the supported [1s, 60s] envelope
    listener_filters_timeout_test.go:543: 4294968s: pipeline: parse rejected: listener: "l_lft": listener_filters_timeout 1193h2m48s is outside the supported [1s, 60s] envelope
```
Four reject arms: `reject message … does not contain …`; seven accept arms: `accept: got error … outside the supported [1s, 60s] envelope`; two pipeline arms: `parse rejected`. `gofmt -l internal/listener/` prints nothing.

## Task 3: Envelope pins re-pointed and renamed, the :4053 comment, the QUIC build arm

Step 1: the old pins' inputs, read from `manager_test.go` before applying: `ListenerFiltersTimeout: durationpb.New(500 * time.Millisecond)` (:3353) and `durationpb.New(90 * time.Second)` (:3380) — both valid Durations.
Appendices D and E applied (`git apply --check` first). `git diff --numstat` -> `19	19	…manager_test.go`, `35	0	…quic_test.go`.
Step 3: `git grep -c 'BelowFloorErrors\|AboveCapErrors' -- internal/listener/` printed one line, `internal/listener/listenerfilter/tls_inspector/proto_test.go:1` (rc 0). That is `TestParseConfigBufferBelowFloorErrors`, a tls_inspector buffer test unrelated to this row (the brief's pathspec is wider than the package); the two OLD listener-timeout names are gone (0 hits in `internal/listener/*.go`). `git grep -c 'ADR-0082 floor' -- internal/listener/manager_test.go` -> 0 (rc 1).
Step 4: `go test -count=1 -v -run 'SubSecondAccepted|AboveOldCapAccepted|TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds' ./internal/listener/` -> `=== RUN` **5**, all 5 FAIL (rc 1):
```
    manager_test.go:3359: NewManager: listener_filters_timeout=500ms rejected: listener: "name": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope
    manager_test.go:3386: NewManager: listener_filters_timeout=90s rejected: listener: "name": listener_filters_timeout 1m30s is outside the supported [1s, 60s] envelope
    quic_test.go:1875: build: QUIC listener with listener_filters_timeout 0.5s rejected: listener: "quic_listener": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope
    quic_test.go:1875: build: QUIC listener with listener_filters_timeout 120s rejected: listener: "quic_listener": listener_filters_timeout 2m0s is outside the supported [1s, 60s] envelope
```
Step 5: `gofmt -l internal/listener/` prints nothing; `go vet ./internal/listener/` rc 0.

## Task 4: Fixture 0126 — driver, README, expectations, and the four registration gates

Step 1 port re-census at the IMPL tip (`git grep -lw -- <port> -- test/ internal/ cmd/` and `ss -tanH; ss -uanH` all states) for 15126 15232 15233 15234 15235 15236: zero files, zero sockets each.
Step 2: Appendices F.1-F.3 extracted; `wc -l` = **673 / 92 / 29** (driver.go / README.md / expectations.yaml).
Step 3: F.4 applied (`git apply --check` first); `git diff --numstat -- test/differential/runner_test.go` -> `1	0`.
Step 4: `go vet ./test/fixtures/0126-listener-filters-timeout-envelope/... ./test/differential/` rc 0; `gofmt -l` on the fixture dir prints nothing.
Step 5 (by the SET, order (imports, dirs)): imports **128**, dirs **128**, `comm -23` empty, `comm -13` empty. `grep -c '^0126-listener-filters-timeout-envelope$'` = 1 in imports.txt and 1 in dirs.txt. Gate 1: `func init() { fixture.RegisterFixture(fixtureName, &lfeDriver{}) }` (driver.go:128) with `fixtureName = "0126-listener-filters-timeout-envelope"` (:59).
Step 6 (NC of the extractor, scratch copies only): renaming the import's directory token to `…envelopeX` fires BOTH directions (`comm -23` prints `…envelopeX`, `comm -13` prints `…envelope`); deleting the import line fires `comm -13` only (prints `…envelope`), `comm -23` empty.

## Task 5: The un-fixed tip recorded (every falsifier RED for its named reason)

Tip = Task 4 commit; no production file touched (`manager.go`, `pipeline.go` unchanged).

**Step 1 — unit suite:** `out=$(go test -count=1 -v ./internal/listener/... 2>&1); rc=$?` -> `rc=1 RUN=292 topFAIL=16`, panic gate (`^panic:|DATA RACE|SIGSEGV`) = 0. (Total matcher `^(FAIL|--- FAIL)|^ *--- FAIL` counts 21 lines = 16 top-level + 2 QUIC subtests + 3 package-level `FAIL` lines; informational.) The 16 top-level FAILs:
```
--- FAIL: TestListenerFilterTimeoutPipelineHoldsAtMax
--- FAIL: TestListenerFilterTimeoutPipelineHoldsPastUint32Wrap
--- FAIL: TestParseListenerFiltersTimeoutAboveMaxSecondsRejected
--- FAIL: TestParseListenerFiltersTimeoutAboveOldCapAccepted
--- FAIL: TestParseListenerFiltersTimeoutAboveUint32MsAccepted
--- FAIL: TestParseListenerFiltersTimeoutHalfSecond
--- FAIL: TestParseListenerFiltersTimeoutMaxAccepted
--- FAIL: TestParseListenerFiltersTimeoutNegativeNanosRejected
--- FAIL: TestParseListenerFiltersTimeoutNegativeSecondsRejected
--- FAIL: TestParseListenerFiltersTimeoutNinetySeconds
--- FAIL: TestParseListenerFiltersTimeoutProtoMaxRejected
--- FAIL: TestParseListenerFiltersTimeoutSixtyOneSeconds
--- FAIL: TestParseListenerFiltersTimeoutSubMillisecondTruncatesToZero
--- FAIL: TestParseListenerFiltersTimeoutSubSecondAccepted
--- FAIL: TestParseListenerFiltersTimeoutTruncatesNotRounds
--- FAIL: TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds
```
Failure reasons are those recorded in Tasks 2 and 3 (envelope message on every arm).
Roster diff vs Task 1's `base-listener.txt` (sorted `=== RUN`): 2 removals (`TestParseListenerFiltersTimeoutAboveCapErrors`, `…BelowFloorErrors`), 20 additions = 15 new top-level tests (13 `TestParseListenerFiltersTimeout…` + 2 `TestListenerFilterTimeoutPipelineHolds…`) + the QUIC parent + its 2 subtests (`/0.5s`, `/120s`) + the two renamed pins (`…SubSecondAccepted`, `…AboveOldCapAccepted`). 274 - 2 + 20 = 292.

**Step 2 — fixture 0126 ALONE** (`go test -count=1 -v ./test/differential/ -run '^TestDifferential$/^0126-listener-filters-timeout-envelope$' -timeout 30m`): `=== RUN   TestDifferential/0126-listener-filters-timeout-envelope` present (1), SKIP lines 0. rc=1, FAIL at subject BOOT; reference never driven:
```
listener manager: listener: "l_half": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope   (x3 start attempts)
runner_test.go:1222: subj start attempt 1 failed (subject ready: EOF); retrying with fresh ports
runner_test.go:1222: subj start attempt 2 failed (subject ready: EOF); retrying with fresh ports
runner_test.go:1222: subj start (attempt 3): subject ready: EOF
--- FAIL: TestDifferential/0126-listener-filters-timeout-envelope (5.67s)
```

**Step 3 — fixture 0125 ALONE:** `=== RUN` present, SKIP 0, rc=0: `--- PASS: TestDifferential/0125-listener-filters-timeout (35.21s)`.
(A `reaper_*` container left by the harness was not touched.)

## Task 6 — production edit (shape A)

`ext A $P` (67 lines) -> `git apply --check` ok -> applied. `git diff --numstat HEAD`: `8 9 manager.go`, `1 1 pipeline.go`, `1 1 pipeline_deadline_test.go` (as briefed).
Symbols: `timeoutMs uint64` (pipeline.go) 1; `lfTimeoutMs             uint64` 1; `s > 9223372035` 1; `expected a positive duration` 1; `AsDuration` in manager.go 0 (git grep -c prints nothing on zero).
Layout gate (Appendix G, base master): PASS a-f, `0 failed sub-gate(s)`.
`gofmt -l internal/listener/`: empty. `go vet ./internal/listener/...`: rc 0.

## Task 7 — comment reconciliation (Appendix B)

`ext B` (38 lines) -> `--check` ok -> applied. `git diff --numstat HEAD`: `7 6 manager.go`.
KIND gate (changed lines not starting `//`): **0**. Layout gate: `0 failed sub-gate(s)`.
`git grep -n 'p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs)'`: `manager.go:1356` (unmoved).
KIND gate fires: in a throwaway detached worktree (removed after) `const defaultMs = 15000` -> `15001` => gate reads **2** (>=1). `git status --short` in W showed only ` M manager.go` (the Task 7 edit itself) and no plant.

## Task 8 — all arms green

**Step 1** `go test -count=1 -v ./internal/listener/...`: rc 0, RUN 292, top-level FAIL 0, panic gate 0. Sorted `=== RUN` roster `diff`s EQUAL to `.superpowers/sdd/PLAN/tip-listener.txt` (only outcomes changed).
**Step 2** `go test -count=1 -race ./internal/listener/...`: rc 0, `DATA RACE` 0 (3 packages ok).
**Step 3** lint control: appended `// behaviour plant` to `internal/listener/integration_test.go`; `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/...` rc 1, fired `behaviour is a misspelling of behavior (misspell)` (it also fired gofmt, the file has no trailing newline - incidental). `git checkout --` the file; rerun rc 0; `git status --short` clean.
**Step 4** fixture 0126 ALONE x3, each rc 0, `=== RUN` line 1, SKIP 0, PASS (5.67s / 5.45s / 5.44s). H1 `l_half` close (ms, 0 bytes, FIN), all inside [350, 900]:
| run | ref | subj |
|---|---|---|
| 1 | 502 | 501 |
| 2 | 501 | 500 |
| 3 | 501 | 501 |
T1/T2 bodies, every run, both sides: `DEFAULT l_half_true\n` / `DEFAULT l_one_true\n`, status 200.
**Step 5** fixture 0125 ALONE x1: `=== RUN` 1, SKIP 0, rc 0, `--- PASS: TestDifferential/0125-listener-filters-timeout (35.38s)`.
No measurement differed from the brief.

## Task 9 — the NC roster (mechanisms written BEFORE any run)

Throwaway detached worktree `/home/esa/git/envoy-go-wt-p101-nc` at dd83474b (Task 7 tree; HEAD 01c5a232 has identical code). Patches: `ext H.1`..`ext H.5` (13/13/13/13/17 lines), each `patch -p1 --dry-run` clean. W never holds a mutant.

| NC | mutation | mechanism to a failure (stated before running) |
|---|---|---|
| NC1 | the return narrowed through `uint32` | 4294968000 and 9223372035999 truncate mod 2^32 (-> 704 and 2077251487) |
| NC2 | V2 neutralised (`&& false`) | a negative wraps to a huge `uint64` and is accepted |
| NC3 | V3 neutralised (`&& false`) | `9223372036s` / `315576000000s` are accepted |
| NC4 | round instead of truncate (`+ 500000`) | 0.0005s -> 1, 0.0019s -> 2, and the max -> 9223372036000 |
| NC5 | compute from `AsDuration()` | **VACUOUS by construction, NOT run**: `AsDuration()` saturates only above ~292 years (~9.2e9 s), which V3 (`s > 9223372035`) already rejects; no accepted input reaches the saturating range, so no input can distinguish it from the fix. |
| NC6 | the old envelope `[1000, 60000]` re-inserted for any non-zero value after V2/V3 | the old check rejects every accepted value outside it |

### Task 9 results (unit selector `-run 'TestParseListenerFiltersTimeout|TestListenerFilterTimeoutPipelineHolds|TestQUICListenerFiltersTimeout' ./internal/listener/`, `-count=1 -v`)

Every NC: `grep -c 'NC<n>'` on manager.go read 0 before, **1 after apply (asserted before reading any result)**, 0 after reverse; `=== RUN` count **23** each; rc 1 each; the throwaway tree was clean after each reverse. Per-arm outcome (P/F; 23 RUN includes the QUIC parent and its two subtests; per-NC total P/F: NC1 20/3, NC2 21/2, NC3 21/2, NC4 20/3, NC6 9/14 counting the QUIC parent and its two subtests):

| arm | NC1 | NC2 | NC3 | NC4 | NC6 |
|---|---|---|---|---|---|
| U-wrap `AboveUint32MsAccepted` | **F** | P | P | P | **F** |
| U-neg `NegativeSecondsRejected` | P | **F** | P | P | P |
| U-negnanos `NegativeNanosRejected` | P | **F** | P | P | P |
| U-oor `AboveMaxSecondsRejected` | P | P | **F** | P | P |
| U-pmax `ProtoMaxRejected` | P | P | **F** | P | P |
| U-max `MaxAccepted` | **F** | P | P | **F** | **F** |
| U-subms `SubMillisecondTruncatesToZero` | P | P | P | **F** | **F** |
| U-1ms9 `TruncatesNotRounds` | P | P | P | **F** | **F** |
| U-500ms `HalfSecond` | P | P | P | P | **F** |
| U-90s `NinetySeconds` | P | P | P | P | **F** |
| U-61s `SixtyOneSeconds` | P | P | P | P | **F** |
| U-zero `ZeroParsesToZero` / U-nil `NilParsesToDefault` | P | P | P | P | P |
| Pipe-max `PipelineHoldsAtMax` | **P** | P | P | P | **F** |
| Pipe-wrap `PipelineHoldsPastUint32Wrap` | **F** | P | P | P | **F** |
| re-pointed `SubSecondAccepted` / `AboveOldCapAccepted` | P | P | P | P | **F** |
| QUIC `…OutsideOldEnvelopeBuilds` (+ `/0.5s`, `/120s`) | P | P | P | P | **F** |
| `InRange` / `Default` / `ZeroDisables` | P | P | P | P | P |

Every cell equals brief section 4.1 (NC4 also reddens `MaxAccepted`; NC1 leaves `PipelineHoldsAtMax` green). **No cell differs.** NC5: VACUOUS, not run (mechanism above). NC2/NC3 have no fixture arm; NC6 at fixture level is the tip's boot reject (not run).

Fixture 0126, each NC ALONE from the throwaway worktree, `=== RUN` line 1, SKIP 0, rc 1, FAIL (reference / subject):

| row | NC1 | NC4 |
|---|---|---|
| H1 `l_half` | g/g (ref 502 ms, subj 501 ms FIN) | g/g (501/501) |
| W1 `l_wrap` open at 1500 | g / **R: subj FIN at 704 ms** | g/g (open/open) |
| M1 `l_subms` open at 1500 | g/g | g / **R: subj FIN at 1 ms** |
| T1, T2 (DEFAULT bodies), N1 | g/g | g/g |
| S series | `l_wrap` subj = 1 (**R**), all others as expected | `l_subms` subj = 1 (**R**), all others as expected; ref all present |
| CompareBytes | **R**, first divergence offset 70 at `M1 l_subms ...` boundary, i.e. the end of `W1 ... open_at_1500ms=false` | **R**, first divergence offset 101 at the end of `M1 ... open_at_1500ms=false` |
| verdict | FAIL | FAIL |

No cell differs from section 4.3. Containers: only the harness's `reaper_*` appeared/was left; none torn down by hand. Throwaway worktree removed (`git worktree list` shows no `-nc`). W never held a mutant.

## Task 10 — occurrence set re-derived at the IMPL tip (GROWTH only)

Binary named: `git` (`git -C $W grep -niE`, case-insensitive), `/usr/bin/grep` for post-filtering. Matchers over `internal/ cmd/ docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md` at HEAD b6d45c23 (code identical to dd83474b): matcher 1 (`60000|\[1s, ?60s\]|1s lower bound|envelope|uint32`) 2788 lines; matcher 2 (`listener_filters_timeout|lfTimeoutMs|timeoutMs`) 119; the `:4053` class (`ADR-0082 (floor|envelope)|ADR-0082\)` in `internal/`) 4; union 2890 lines (474 in the two docs). The bulk is generic `uint32`/`envelope` noise (ports, other ADRs); the table is INHERITED, not rebuilt.

**Inherited rows: none lost.** Every SPEC section 5 / PLAN 0.7 doc line (BEHAVIOR_CONTRACT `:4338 :4359 :5150`; DECISIONS `:3040 :3044 :3048 :3052 :3054 :3056 :3066 :3068 :3070 :3074 :3171 :3175 :3221 :19355 :19491 :19507 :19552 :19567 :19568 :19642 :19644`) still hits at the same line (the only docs change since 762c8411 is ADR-0323 APPENDED at `:19646+`, so no line shifted). The `:4053` class: the comment now reads `// 1s pipeline timeout; slowListenerFilter blocks 2s.` (`manager_test.go:4053`; "floor" gone, P reconciled by Appendix D); the 4 remaining `ADR-0082)` hits (`doc.go:18`, `manager_test.go:3324 :4033 :4101`) cite ADR-0082 for the 15 s DEFAULT / `continue`-semantics / shared per-pipeline budget, the clauses that SURVIVE (H, true as written).

**GROWTH (hits not in the inherited table), each adjudicated:**
| hit | what | verdict |
|---|---|---|
| `DECISIONS.md:19646-19656` (ADR-0323, appended at the SPEC; §Context ¶1/¶3 say envelope / `uint32`) | the new ADR's own Context, narrating the OLD behavior | H (its own history); §Decision + §Consequences are owed to **Task 11** |
| `DECISIONS.md:19495 :19513 :19527 :19544 :19590 :19639` (ADR-0322) | phase-100 record, e.g. "RANGE-CHECKED" | H (an ACCEPTED ADR at its time; same class as the table's ADR-0322 rows); superseded in part by ADR-0323 (**Task 11** names it) |
| `DECISIONS.md:19233 :19263 :19340 :19341 :19349` (ADR-0320) | `continue_on_listener_filters_timeout`, "NOT ENFORCED" | H (phase-99 record; the table's `:19355` class), not an envelope claim |
| `DECISIONS.md:13319 :13337` (ADR-0205) | proxy-wasm `httpCall timeoutMs uint32` | NOT A HIT (unrelated field) |
| `listener_filters_timeout_test.go:386 :410 :443 :533 :540`, `quic_test.go:1854-1859` | "a uint32 carrier wraps it", "the old `[1s, 60s]` envelope", "clamped to 60000", `…OutsideOldEnvelopeBuilds` | H (the new tests' narration of the OLD behavior, as briefed) |

**Docs hits still carrying a present-tense envelope claim, owed to a LATER task (not edited here):** BEHAVIOR_CONTRACT `:4338` gloss and `:4359` `Per-pipeline timeout` bullet = **Task 12**; ADR-0082 `:3040-:3074` (superseded, never edited), ADR-0081 `:3175` (noted), ADR-0078 `:3221` (named), ADR-0322 `:19642/:19644` (g) (superseded) = **Task 11** via ADR-0323.

**Step 3 assertion:** non-test code under `internal/listener/` (excluding `_test.go`): `git grep -niE 'envelope|60000|\[1s, ?60s\]|1s lower|uint32 (carrier|of ms|wrap)|old (cap|envelope)'` returns **nothing** (the `tls_inspector.go:18` `[256, 65536] envelope` is another field and was excluded by name). Remaining `uint32` there are ports (`chainmatch.go`, `types.go`, `quic.go`, `manager.go:1021 :1145 :1582-1598`); the timeout width is `uint64` at `pipeline.go:32` and `manager.go:169`, `:950-966`; the comments at `manager.go:166-167 :831-832 :950-953`, `pipeline.go:19-27` and `doc.go:18` carry no envelope or uint32 claim. **No new present-tense code/comment claim found; zero edits forced.**

## Task 11 — ADR-0323 completed IN PLACE, ACCEPTED

Before: `wc -l DECISIONS.md` 19662; the italic footer `*§Decision and §Consequences follow at the phase-101 IMPL.*` was the last line; the house guard `/usr/bin/grep -n '^> \*\*STATUS: PROPOSED'` hit `:19648` only (ADR-0323's status line).
**Steps 1-2** — appended, AFTER the RETAINED footer, `### Decision (landed at the phase-101 IMPL)` (rule V1-V5 read from the fields; `uint64` end to end, B and C rejected on mechanism; the layout of `pipeline.go:43`, `:33-37` unmoved and `:32` widened only; the ADR-0082 supersessions and its three KEPT clauses `:3048 :3054 :3056`; NOTES on ADR-0081 (c) `:3175` and ADR-0078 `:3221`; SUPERSESSION of ADR-0322 (g)) and `### Consequences (landed at the phase-101 IMPL)` ((a) behaviour changes incl. QUIC REJECTED -> ACCEPTED-AND-IGNORED; (b) evidence; (c) the two arms that moved, PLAN §0.2 / §0.3; (d) NC roster with NC5 vacuous; (e) +0 names; (f) not bought, SPEC §1; (g) `stats_flush_interval` next). Figures cited from Tasks 5-9 above. Heading wording follows ADR-0322's house shape `(landed at the phase-10x IMPL)`; the brief wrote `(phase-101 IMPL)` — both end in `phase-101 IMPL)`.
**Step 3** — `:19648` flipped to `> **STATUS: ACCEPTED — §Context drafted at the phase-101 SPEC; §Decision + §Consequences APPENDED IN PLACE at the phase-101 IMPL, …`, the ADR-0322 `:19491` shape copied (no count of either matcher written).
**Step 4** — measured after the flip:
- `/usr/bin/grep -n '^> \*\*STATUS: PROPOSED' DECISIONS.md` -> no output, rc 1; READ `sed -n 19648p` -> `> **STATUS: ACCEPTED — §Context drafted at the phase-101 SPEC; …` (the matcher is not broken: it hit `:19648` before the flip).
- decoy `/usr/bin/grep -n '^\*\*Status:\*\* PROPOSED'` -> `14866:**Status:** PROPOSED. §Context anchored at the **phase-33 SPEC commit** …`; backward `^## ADR-` from 14866 -> `14864:## ADR-0231`; `sed -n 14866p | md5sum` -> `929719b67c87…`.
- `^---$` 216; `^## ADR-` 322; bare `^## ` 330; tail `## ADR-0323` (`grep -oE '^## ADR-[0-9]+' | tail -1`), so next-free ADR-0324; backward heading from 19648 -> `19646:## ADR-0323`.
- `git diff --numstat` -> `146 1 docs/envoy-go/DECISIONS.md`; `wc -l` **19662 -> 19807**; `tail -c1` is `\n`; no trailing whitespace in the appended block.

## Task 12 — BEHAVIOR_CONTRACT.md: the bullet, the gloss, the `+0` ledger entry

Before: `wc -l BEHAVIOR_CONTRACT.md` **6000**; anchors by LITERAL: the `- Per-pipeline timeout (` bullet at `:4359`, the gloss `ADR-0082 (listener_filters_timeout [1s,60s] envelope)` at `:4338`, `**Phase 100 — +1 (` at `:5150` (followed by a blank and `### Forward-pointer note (26.3)`).
**Step 1** — bullet rewritten in place (one line); md5 of the installed line equals the brief's verbatim text (`99a24e0f…` both).
**Step 2** — gloss rewritten in place. `git grep -cF 'ADR-0082 (listener_filters_timeout [1s,60s] envelope)'` -> no output, rc 1 (0); `git grep -cF 'ADR-0082 (listener_filters_timeout; its [1s,60s] envelope superseded by ADR-0323)'` -> `…BEHAVIOR_CONTRACT.md:1`.
**Step 3** — ledger entry inserted as its own paragraph directly after the phase-100 entry (`:5152`, blank `:5151`); md5 equals the brief's verbatim text (`ad7c4d2e…` both).
**Step 4** — `/usr/bin/grep '^\*\*Phase 101 — ' | /usr/bin/grep -nE '→|->'` -> no output, rc 1 (no `A → B` absolute). `git diff --numstat` -> `4 2`; `wc -l` measured **6002** (6000 -> 6002, as predicted); `tail -c1` is `\n`.

## Task 13 — ROADMAP.md row 101 -> `done`, under the field-count gate

**Step 1** — new cell built in a scratch copy (`cp ROADMAP.md roadmap-new.md`; a script split ONLY the line starting `| 101 | listener-filters-timeout-envelope-lift |` on `|`, asserted 8 fields and field 5 == ` in-progress `, set field 5 to ` done ` and APPENDED the SPEC / PLAN / IMPL record to field 7, asserting the appended text holds no `|`). The BRAINSTORM text already in the cell is kept verbatim. The cell names ADR-0323 ACCEPTED, shape A's numstat (`manager.go` 15/15, `pipeline.go` 1/1, `pipeline_deadline_test.go` 1/1), fixture `0126` `+794` plus the one-line import, the 16 tip-RED tests and the NC outcome (NC5 vacuous).
**Step 2** — on the scratch copy: naive `awk -F'|' '/^\| *101 /{print NF}'` -> **8**; escape-aware `sed 's/\\|//g' F | awk -F'|' '/^\| *101 /{print NF}'` -> **8**; pipes inside field 7 -> 0.
**Step 3** — on the cell: both sentinel phrases -> 0; `grep -oi deferred` -> 0; `grep -o -- '-family row'` -> 0.
**Step 4** — installed (`git diff --numstat` -> `1 1 docs/envoy-go/ROADMAP.md`). Full sentinel, `/usr/bin/grep`, run on BOTH sides:

| check | before the flip | after the flip |
|---|---|---|
| (1) want=133 | `NOT DONE: row 101` | silent |
| (2) | six: `211 217 223 233 239` (`remaining deferred (not-yet-chartered) candidates:`), `247` (`deferred candidates:`) | same six |
| (3) | silent | silent |
| NC-A (row 62 forced in-progress; `NC LANDED? [ in-progress ]` both sides) | `NOT DONE: row 62`, `NOT DONE: row 101` | `NOT DONE: row 62` |
| NC-B want=132 | `NOT DONE: row 101`, `GATE FAIL: examined 133 data rows, expected 132` | `GATE FAIL: examined 133 data rows, expected 132` |
| NC-C (gRPC slug blanked) | residual 0, `NEVER OPENED: gRPC` | residual 0, `NEVER OPENED: gRPC` |
| NC-D (`-o` / `-c`) | 96 / 68 | 96 / 68 |
| c2pc (residual / `candidatesXX`) | 0 / 6 | 0 / 6 |
| malformed set (escape-aware NF != 8) | `119 57 9`, `131 69 10` | `119 57 9`, `131 69 10` |
| md5 of lines 211 217 223 233 239 247 | `10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 242e53c6f7a3 b2680e6f4fbf 6caa1c3ce0e7` | identical |
| `wc -l ROADMAP.md` | 251 | 251 |

`diff` of the two full sentinel outputs is exactly the three `NOT DONE: row 101` lines (in (1), NC-A and NC-B), nothing else. `want` stays 133; the six termination windows are byte-untouched. Every figure matched the brief.

## Task 14: The byte-untouched roster, the ARM roster, and the SIX-GATE sweep

Preconditions: branch `wt-phase-101-impl`, HEAD `209a2f3d` at task start, `git status --short` clean at start and after the lint-control probe.

### Step 1 - byte-untouched roster and the edit roster's path set

`git diff master --numstat` was EMPTY for `internal/listener/quic.go`, `internal/stats/`, `cmd/`, `REVIEW_FINDINGS.md`, `go.mod`, `go.sum`, `.github/`, every `internal/listener/listenerfilter/` path except `pipeline.go` and `pipeline_deadline_test.go`, and every `test/fixtures/*` directory except `0126` (0125 included).

Full branch diff (14 files): BEHAVIOR_CONTRACT.md `4 2`, DECISIONS.md `146 1`, ROADMAP.md `1 1`, this PROGRESS.md `248 0` (before this task's append), listener_filters_timeout_test.go `206 0`, pipeline.go `1 1`, pipeline_deadline_test.go `1 1`, manager.go `15 15`, manager_test.go `19 19`, quic_test.go `35 0`, runner_test.go `1 0`, 0126 README `92 0`, driver `673 0`, expectations `29 0`. The code rows match PLAN 1.2 exactly (the 0126 trio sums to `794 0`). The only path on neither roster is this phase's PROGRESS.md (expected; 1.2 names it as off-roster by design).

### Step 2 - ARM roster

Base `base-listener.txt` is 274 sorted `=== RUN` lines (`./internal/listener/...`). New roster, `go test -count=1 -v ./internal/listener/...` rc=0: **292** lines. Gained (`comm -13`): exactly **20** - the 15 new top-level names, `TestQUICListenerFiltersTimeoutOutsideOldEnvelopeBuilds` with `/0.5s` and `/120s`, `TestParseListenerFiltersTimeoutSubSecondAccepted`, `TestParseListenerFiltersTimeoutAboveOldCapAccepted`. Lost (`comm -23`): exactly **2**, `TestParseListenerFiltersTimeoutBelowFloorErrors` and `TestParseListenerFiltersTimeoutAboveCapErrors`. 274 + 20 - 2 = 292.

(A first run scoped to `./internal/listener/` alone gave 202 lines; the base roster spans the sub-packages, so the comparison was redone over `/...`.)

### Step 3 - Gate (a): full differential

`go test -count=1 -v ./test/differential/ -timeout 60m`, foreground, single run (not split): rc=0, `--- PASS: TestDifferential (443.78s)`, package `ok 447.685s`. Fixture set by name vs `ls -d test/fixtures/*/`: `comm -23` empty, `comm -13` empty, **128 = 128**. **PASS 128 / FAIL 0 / SKIP 0.** No port-race abort.

### Step 4 - Gate (b): non-Docker sweep

`go list ./...` minus differential and h2spec: **243** packages (242 + `0126/driver`, confirmed present). `go test -count=1`: rc=0. `ok` 125 + `FAIL` 0 + `[no test files]` 118 = **243**. No known flake fired.

### Step 5 - Gate (c): h2spec

`go test -count=1 -v ./test/conformance/h2spec/`: rc=0, `95 tests, 94 passed, 1 skipped, 0 failed`; 0 `[FAIL]` lines.

### Step 6 - Gate (d): fuzzers

`git grep -c '^func Fuzz' -- '*.go'`: **56 targets across 48 files**. The row adds none.

### Step 7 - Gate (e): panic gate, lint, planted control

Anchored `^panic:|DATA RACE|SIGSEGV` over gates a, b, c: **0** each. `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...`: rc=0, no output. Planted `internal/listener/zz_lintplant.go` (compiles, `go build` rc=0): lint rc=1 with `errcheck`, `revive` and `ineffassign` each named in the output. Deleted; `git status --short` empty.

### Step 8 - Gate (f)

No `REVIEW.md` under phases 93-101. This is the one standing departure; compliance is not claimed.

### Summary - all six gates

| gate | result |
|---|---|
| (a) differential | rc=0, 128 PASS / 0 FAIL / 0 SKIP, set 128 = 128 both `comm` directions, no abort |
| (b) sweep | rc=0, 243 pkgs = 125 ok + 0 FAIL + 118 no-test-files |
| (c) h2spec | `95 tests, 94 passed, 1 skipped, 0 failed` |
| (d) fuzzers | 56 targets / 48 files |
| (e) panic + lint | 0 hits; lint rc=0; control fired errcheck+revive+ineffassign, removed, tree clean |
| (f) REVIEW.md | absent (standing departure) |

## Final-review fix (after Task 14)

The whole-branch final review of `09b612f9..dba9cb5b` found one Important defect, reproduced by the reviewer;
the controller ruled to fix it, plus Minor 1 (test helper) and Minor 3 (documented, not fixed). Commits:
`65f4c482` (RED arm + Minor 1), `87e4232e` (fix), `a6c4848c` (docs), and this record.

- **Important — V3 tested seconds only.** `parseListenerFiltersTimeout` rejected `s<0||n<0` and
  `s>9223372035` but accepted `nanos > 999999999`, which only a Duration built in Go can carry (protojson and
  YAML cannot). `{9223372035, 2147483647}` parsed to 9223372037147 ms; `time.Duration(ms)*time.Millisecond`
  in `Pipeline.Run` (`pipeline.go:43`) overflows `int64` to a NEGATIVE duration, so the pipeline timed out
  after about 12 µs. `{1, 1.5e9}` was accepted as 2500 ms. ADR-0323 V3 and SPEC §2 claimed the overflow
  could not happen.
- **Ruling, as built.** `manager.go` V3 is now `if s > 9223372035 || n > 999999999 {` with the message
  `listener: %q: listener_filters_timeout: duration out of range: %ds %dns` (phrase kept); the doc comment's
  fourth line now names the nanos bound. LINE-COUNT-NEUTRAL: `manager.go` 1662 lines before and after, diff
  `3 3`; `git grep -n 'p.Run(ctx, filters, peeker, &inputs, rt.lfTimeoutMs)' -- internal/listener/manager.go`
  reads `manager.go:1356` (unmoved). `pipeline.go` untouched. Against master `manager.go` still reads `15 15`
  (the three changed lines were already inside the row's diff), so ADR-0323 §Decision's numstat stays true.
- **Minor 1 — `lftLoopbackPair` ignored the Accept error.** The accept goroutine now sends `{conn, err}` and
  the test goroutine closes the peer and `t.Fatalf("accept: %v", …)` on an error.
- **Minor 3 — recorded, not fixed.** Exported `Pipeline.Run` accepts any `uint64` and overflows above
  9223372036854 ms; only the parser bounds it. Its doc comment sits on layout-gated lines, so the residual is
  written into ADR-0323 §Consequences (h) instead.

### RED / GREEN

New arm `TestParseListenerFiltersTimeoutNanosOutOfRangeRejected` (two `lftReject` calls, each message naming
its input), run on the pre-fix tree (test committed first, `65f4c482`), rc 1:

```
=== RUN   TestParseListenerFiltersTimeoutNanosOutOfRangeRejected
    listener_filters_timeout_test.go:419: {0s, 1000000000ns}: reject: accepted with 1000 ms, want an error
    listener_filters_timeout_test.go:420: {9223372035s, 2147483647ns}: reject: accepted with 9223372037147 ms, want an error
--- FAIL: TestParseListenerFiltersTimeoutNanosOutOfRangeRejected (0.00s)
```

After `87e4232e`: `--- PASS: TestParseListenerFiltersTimeoutNanosOutOfRangeRejected`.

### NC7 — the nanos clause neutralised

Throwaway worktree `git -C /home/esa/git/envoy-go worktree add --detach /home/esa/git/envoy-go-wt-p101-nc7
87e4232e`, removed after (`worktree list` shows no `nc7`; the directory is gone). Mechanism, written before
running: with `(n > 999999999 && false)` the nanos half never fires, so both new inputs fall through to V4
and are accepted; no other arm feeds nanos above 999999999, so no other arm can move. Mutant line:
`if s > 9223372035 || (n > 999999999 && false) { // NC7`; `grep -c NC7 manager.go` = **1** before reading.
§4.1 selector `-run 'TestParseListenerFiltersTimeout|TestListenerFilterTimeoutPipelineHolds|TestQUICListenerFiltersTimeout'`:
**RUN 24, 23 PASS, 1 FAIL**, the only FAIL the new arm, with the two raw lines identical to the RED run above.
Reversed: marker **0**, same selector RUN 24, 0 FAIL, rc 0.

### Appendix H.3 applicability

`ext H.3 PLAN.md | patch -p1 --dry-run` on `87e4232e`: **`Hunk #1 FAILED at 960`** — its `-` line
(`if s > 9223372035 {`) and its context message line no longer exist. **H.3 no longer applies**; NC3's score
(two reject arms red) is historical evidence at `dd83474b` and was NOT re-scored. The other four appendices
still dry-run apply, but only with fuzz (H.1, H.4, H.5 fuzz 2; H.2 fuzz 1), because the V3 lines are their
context.

### Optional — one raw failing line per NC, regenerated on the final tree

Applied with fuzz in the same throwaway worktree, marker `grep -c NC<n>` = 1 before and 0 after each, §4.1
selector RUN 24 each. Line numbers are the final tree's, not Task 9's. NC3 is not regenerated (H.3 does not
apply; see above).

| NC | `--- FAIL` lines | one raw line |
|---|---|---|
| NC1 (H.1) | 3 | `listener_filters_timeout_test.go:388: 4294968s: value: got 704 ms, want 4294968000 ms` |
| NC2 (H.2) | 2 | `listener_filters_timeout_test.go:394: -1s: reject: accepted with 18446744073709550616 ms, want an error` |
| NC4 (H.4) | 3 | `listener_filters_timeout_test.go:432: 0.0005s: value: got 1 ms, want 0 ms` |
| NC6 (H.5) | 14 (12 top-level + 2 QUIC subtests) | `listener_filters_timeout_test.go:388: 4294968s: accept: got error listener: "l_lft": listener_filters_timeout is outside the supported [1s, 60s] envelope, want accepted` |

The FAIL sets match Task 9's per-arm scores (NC1: `…AboveUint32MsAccepted`, `…MaxAccepted`,
`…PipelineHoldsPastUint32Wrap`; NC2: the two negative arms; NC4: `…MaxAccepted` and the two truncation arms; NC6: the same 14 `--- FAIL` lines as Task 9's 9/14, now 10/14 with the new arm passing).

### Re-verification on the final tree (`87e4232e` code)

- `go test -count=1 -v ./internal/listener/...`: rc 0, **293** `=== RUN`, 293 `--- PASS`, **0** FAIL, 0 SKIP
  (292 + the one new top-level arm); three packages `ok`.
- `go test -count=1 -race ./internal/listener/...`: rc 0, three `ok`, **0** `DATA RACE`.
- `gofmt -l internal/listener/`: empty. `go vet ./internal/listener/...`: rc 0.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/listener/...`: rc 0.
- Fixture `0126` NOT re-run, by reasoning: its inputs arrive through YAML/protojson, which cannot carry nanos
  ≥ 1e9 (protojson rejects a fractional part past nine digits and normalises the rest into seconds), so no
  `0126` input reaches the new clause, and its accepted values parse to the same milliseconds as before.

### Doc edits (`a6c4848c`)

- `DECISIONS.md`, ADR-0323 only (from `## ADR-0323` to EOF): §Context ¶4 gains a sentence that the
  overflow guarantee needs both halves of V3; §Decision V3 bullet rewritten (seconds OR nanos, new message
  format, both halves load-bearing); §Consequences (a)'s reject bullet names the nanos bound; (b)'s final
  figure reads "292 `=== RUN` and 0 FAIL (293 and 0 on the final tree …)", the un-fixed-tip 292/16 left as
  historically true; new item **(h)** records the finding, the RED/GREEN/NC7 result, source-read (NOT
  measured) parity, and the Minor 3 residual. STATUS line still ACCEPTED. Guards: `^> \*\*STATUS: PROPOSED`
  **0**, `^---$` **216**, `^## ADR-` **322**, bare `^## ` **330**. `wc -l` **19821** (was 19807), `22 8`
  against `dba9cb5b`.
- `BEHAVIOR_CONTRACT.md`: the `Per-pipeline timeout (` bullet now reads "seconds above `9223372035` or nanos
  above `999999999` (`duration out of range`)", in place, `1 1`; **6002** lines.
- `ROADMAP.md` row 101 (line 163) only: "after the fix 293 and 0 FAIL on the final tree (the final review
  added a nanos guard …)". Built in a scratch copy: NF **8** under both the naive and the escape-aware forms,
  no `deferred` / `-family row`, 2 diff lines; installed; window md5s at 211 217 223 233 239 247 unchanged
  (`10d7807bf02d 4a92f7e62fc6 2a7eb298b9fd 242e53c6f7a3 b2680e6f4fbf 6caa1c3ce0e7`); **251** lines; check (1)
  printed nothing.

### Gates re-run on the final tree (after the fix)

Task 14's six gates ran at `dba9cb5b`, BEFORE the final-review fix. The controller then re-ran gates (a) and
(b) on the final tree, in this worktree, with code identical to `7daa568c` (only docs differ):
- **Gate (a):** `go test -count=1 -v ./test/differential/ -timeout 30m`: rc 0 in `443.201s`;
  `--- PASS: TestDifferential/` **128**, FAIL 0, SKIP 0. The set was checked BY NAME: 128
  `=== RUN TestDifferential/<name>` names against 128 fixture dirs, both `comm` directions (ran, dirs) EMPTY.
  Panic gate 0.
- **Gate (b):** `go list ./... | /usr/bin/grep -vE '/test/differential$|/test/conformance/h2spec$'` gives **243**
  packages; `go test -count=1` over them: rc 0, 125 `ok` + 0 FAIL + 118 `[no test files]` = 243. Panic gate 0.
- **NOT re-run after the fix:** gate (c) h2spec, gate (d) fuzzers (the fix adds no `func Fuzz`), and the
  full-repo lint. The fix touched only `internal/listener/` plus docs, and the fix wave re-ran lint and `-race`
  on `./internal/listener/...` (above).

### Numstat against master after the fix

```
4	2	docs/envoy-go/BEHAVIOR_CONTRACT.md
161	2	docs/envoy-go/DECISIONS.md
1	1	docs/envoy-go/ROADMAP.md
420	0	docs/envoy-go/phases/101-listener-filters-timeout-envelope-lift/PROGRESS.md
223	0	internal/listener/listener_filters_timeout_test.go
1	1	internal/listener/listenerfilter/pipeline.go
1	1	internal/listener/listenerfilter/pipeline_deadline_test.go
15	15	internal/listener/manager.go
19	19	internal/listener/manager_test.go
35	0	internal/listener/quic_test.go
1	0	test/differential/runner_test.go
92	0	test/fixtures/0126-listener-filters-timeout-envelope/README.md
673	0	test/fixtures/0126-listener-filters-timeout-envelope/driver/driver.go
29	0	test/fixtures/0126-listener-filters-timeout-envelope/expectations.yaml
```

## Close-out (Task 15)

### Per-task summary

| task | commit | result |
|---|---|---|
| 1 | `2cb360d0` | baseline: `./internal/listener/...` 274 `=== RUN` rc 0; the listener + stats selector 470; the anchored panic gate proven live |
| 2 | `ea4083aa` | Appendix C, 15 parse / pipeline-hold arms; 13 RED at the tip on the envelope message, 2 GREEN (zero, nil) |
| 3 | `f1ccd4af` | Appendices D + E: the two envelope pins re-pointed and renamed, the `:4053` comment, the QUIC build arm; all 5 RUN RED at the tip |
| 4 | `b55e7e50` | fixture `0126` (673 / 92 / 29) + the import; ports censused zero; fixture set 128 = 128; extractor NCs fired |
| 5 | `3e31eaa8` | the un-fixed tip recorded: 292 RUN / 16 top-level FAIL; `0126` RED at subject boot by its message; `0125` GREEN |
| 6 | `597de0d3` | shape A (Appendix A): `manager.go 8 9`, `pipeline.go 1 1`, `pipeline_deadline_test.go 1 1`; symbols asserted, `AsDuration` absent, layout gate 0 |
| 7 | `dd83474b` | comment pass (Appendix B, `7 6`); KIND gate 0 (fired by a plant), `manager.go:1356` unmoved |
| 8 | `01c5a232` | 292 / 0, `-race` clean, lint control fired; `0126` 3/3 (H1 ref 501-502, subj 500-501 ms); `0125` PASS |
| 9 | `b6d45c23` | NC1-NC4, NC6 scored per arm with markers asserted; every cell equal to §4.1 / §4.3; NC5 vacuous, not run |
| 10 | `74bc8231` | occurrence set re-derived, GROWTH only; no inherited row lost; zero edits forced |
| 11 | `c01b73fe` | ADR-0323 §Decision + §Consequences, `ACCEPTED`; house guard disarmed (proven by reading `:19648`) |
| 12 | `c4a7e904` | contract bullet + gloss rewritten, delta-only `+0` ledger entry; 6000 -> 6002 |
| 13 | `209a2f3d` | row 101 `done` under the field-count gate; sentinel measured on both sides of the flip |
| 14 | `dba9cb5b` | byte-untouched roster empty; arm roster 274 + 20 - 2 = 292; six gates (a 128/0/0, b 243 rc 0, c 95/94/1/0, d 56/48, e 0 + lint rc 0 with a live plant, f no `REVIEW.md`) |
| final-review fix | `65f4c482`, `87e4232e`, `a6c4848c`, `7daa568c` | V3 rejects nanos above `999999999` (RED arm first); `lftLoopbackPair` checks Accept; ADR-0323 (h), contract bullet, row-101 cell; NC7; 293 / 0 |
| 15 | this commit | close-out: `STATE.md` rolled, the archive +1, the router rolled to a phase-102 self-pick BRAINSTORM |
| 16 | pending | squash, merge, push and worktree removal — the controller's, not recorded here |

### Every refutation this IMPL made (method note 2) — ELEVEN

Against the SPEC, the PLAN and the stage's briefs:
1. 🔴 **`SPEC.md` §2's overflow guarantee is FALSE for a Duration built in Go.** The rule rejected seconds
   above `9223372035` and claimed that kept `Pipeline.Run`'s `time.Duration(ms)*time.Millisecond` inside
   `int64`. It tested SECONDS only: `{9223372035, 2147483647}` parsed to 9223372037147 ms, the product
   overflowed to a NEGATIVE duration and the pipeline timed out at once (about 12 µs); `{1, 1.5e9}` was
   accepted as 2500 ms. `PLAN.md`'s Appendix A and ADR-0323 V3 as drafted at Task 11 carried the same rule.
   Found by this IMPL's own final review (reproduced with `-overlay`); fixed in `87e4232e` with
   `s > 9223372035 || n > 999999999`, line-count-neutral, after a RED arm (`65f4c482`). YAML and protojson
   cannot carry nanos ≥ 1e9, so no reference arm exists: the parity claim (the reference rejects nanos outside
   `[0, 999999999]`) is from source reading, NOT measured. `SPEC.md` is left unedited as evidence.
2. ⚠️ **The exported `Pipeline.Run` still overflows above 9223372036854 ms.** Only the parser bounds the value;
   `Run` takes any `uint64`. Recorded in ADR-0323 §Consequences (h), not fixed (its doc comment sits on
   layout-gated lines).
3. ⚠️ **Appendix C's `lftLoopbackPair` ignored the Accept error** (a PLAN-built, byte-identical appendix).
   Fixed test-only in `65f4c482`.
4. ⚠️ **Appendix H.3 does not apply to the final tree** (`Hunk #1 FAILED at 960`): its `-` line and context
   message no longer exist. NC3's score is historical evidence at `dd83474b`. H.1, H.2, H.4 and H.5 still apply,
   but only with fuzz.
5. ⚠️ **`PLAN.md` §1.2's final figures moved:** `listener_filters_timeout_test.go` `206 0` -> **`223 0`**, and
   the final `./internal/listener/...` roster 292 / 0 -> **293 / 0**. Both were true until the final-review fix.
6. ⚠️ **`PLAN.md` §1.2's `DECISIONS.md ~+100 / −1` measured `146 1` at Task 11 and `161 2` against master after
   the fix** (19662 -> 19807 -> 19821). The estimate was low, as the phase-100 PLAN's was.
7. ⚠️ **`PLAN.md` §1.2's `BEHAVIOR_CONTRACT.md ~+1 / 0` net disagreed with its own Task 12** (which predicted
   6000 -> 6002). Measured `4 2`, **+2** net, 6002. Task 12 was right and §1.2 was wrong.
8. ⚠️ **Task 3 Step 3's "reads nothing" read one hit**: `git grep -c 'BelowFloorErrors\|AboveCapErrors' --
   internal/listener/` matches `listenerfilter/tls_inspector/proto_test.go` (`TestParseConfigBufferBelowFloorErrors`,
   a buffer test). The pathspec is wider than the claim; the two old listener-timeout names are gone.
9. ⚠️ **The Task 11 brief's heading form `(phase-101 IMPL)` departed from the house shape** that the same
   brief told the implementer to copy (ADR-0322's `(landed at the phase-10x IMPL)`). The house shape was used.

Against the IMPL's own work:
10. ⚠️ **Task 14's first ARM-roster run used the wrong selector**: `./internal/listener/` alone (202 lines)
    against a base roster taken over `./internal/listener/...`. It was redone over `/...` (292) before the
    record was written.
11. ⚠️ **Method note 97 fired a second row running.** ADR-0323, the contract bullet and the row-101 cell were
    written at Tasks 11-13 with a seconds-only bound and a final figure of 292. The final-review fix falsified
    both, and `a6c4848c` corrected them. Every figure this close quotes was re-derived at its last commit:
    `./internal/listener/...` **293 / 0** (rc 0, panic gate 0), and `./internal/listener/... ./internal/stats/...`
    **489** `=== RUN` / 0 FAIL (470 - 2 + 21).

Confirmed, not refuted: every PLAN-built artifact applied as built (F.1-F.3 `cmp`-identical; A, B, C, D, E
diff lines identical); the tip shape (274 bare, 292 / 16 top-level FAIL, 13 of 15 new arms RED, `0126`'s
boot-reject message, `0125` green); every NC cell in `PLAN.md` §4.1 and §4.3; the H1 window (`[350, 900]`,
measured 500-502 ms); gate (b) **243** (242 + `0126/driver`, as predicted); fixtures **128 = 128**;
`BEHAVIOR_CONTRACT.md` 6002; the sentinel shapes the router predicted for the flip; and the §Recent tie the
router projected (below). Items 1-4 came from the stage's own final review. Method note 2's "expect to be
refuted by your own review seam" held for a fifth row.

Left as is, by ruling (not refutations): ADR-0323's heading does not name the nanos clause (incomplete, not
false; headings are anchors other documents cite); `SPEC.md` §2 keeps its stale "cannot overflow" as evidence;
the Task 5 total matcher's "3 package-level FAIL lines" is recorded and not re-run; T2's 200 ms margin in `0126`
is banked with no action.

### Sentinel at this close

Commands verbatim from `next-prompt.txt` lines 12-72 as that file stood at `7daa568c` (pre-roll; in the rolled file the same commands sit at lines 12-73), `/usr/bin/grep` (a shell function forwarding to it), in
the worktree at `7daa568c`. Task 15 edits none of the files the checks read. ACTUAL output:
```
== (1)
== (2)
211:remaining deferred (not-yet-chartered) candidates:
217:remaining deferred (not-yet-chartered) candidates:
223:remaining deferred (not-yet-chartered) candidates:
233:remaining deferred (not-yet-chartered) candidates:
239:remaining deferred (not-yet-chartered) candidates:
247:deferred candidates:
== (3)
== NC-A
NC LANDED? [ in-progress ]
NOT DONE: row 62
== NC-B want=132
GATE FAIL: examined 133 data rows, expected 132
== NC-C
0
NEVER OPENED: gRPC   <- NC FIRED
== NC-D
96
68
== PC
0
6
== malformed (escape-aware)
119: row 57  NF=9
131: row 69  NF=10
== row101 NF naive/esc
163 8
163 8
== md5 (trailing newline included)
211 10d7807bf02d
217 4a92f7e62fc6
223 2a7eb298b9fd
233 242e53c6f7a3
239 b2680e6f4fbf
247 6caa1c3ce0e7
== wc
251
== stop
ls: cannot access '/home/esa/git/envoy-go/stop': No such file or directory
ls: cannot access '/home/esa/git/envoy-go-wt-p101impl/stop': No such file or directory
```
⇒ the sentinel does NOT fire (check (2) prints six); `stop` NOT created. Fixture set by the method-note-3e
extractor: imports **128**, dirs **128**, both `comm` directions empty, split **104 `driver/` + 24 `inputs/`**.

### Eviction and archive

Pre-roll histogram of §Current + §Recent (`grep -oE` over the `active-phase` / `prior active-phase` labels,
`uniq -c` on the dates), INCLUDING the entry this close promoted (the phase-101 PLAN): **1 x `2026-10-06`,
1 x `2026-09-30`, 1 x `2026-09-23`, 3 x `2026-09-22`**. That is a THREE-WIDE `09-22` tie at the tail, as the
router projected. LIST POSITION broke it: the evictee is the phase-100 SPEC entry. LABEL-BOUND PAIR
(`/usr/bin/grep -cF -- '<backticked label>'`):

| label | `STATE.md` before -> after | `STATE_HISTORY.md` before -> after |
|---|---|---|
| evictee (phase-100 SPEC) | 1 -> 0 | 0 -> 1 |
| fabricated (`… SPECTRE done`) NC | 0 -> 0 | 0 -> 0 |
| positive control (phase-100 BRAINSTORM, archived) | 0 -> 0 | present -> present |

Archive guard (house anchored forms): strict `163 -> 163` (DELTA 0), parenthetical `87 -> 88`, loose
`250 -> 251`; `STATE_HISTORY.md` `600 -> 602` (`wc -l`), `2 0`. `STATE.md` `66 -> 66`, `10 10`; `next-skill:`,
`lifecycle-state:` and `next-free ADR:` each read 2 (the line-7 prose, as on master, plus the one live bullet).

### Router roll

`next-prompt.txt` now points at a phase-102 self-pick BRAINSTORM (state DONE -> 1), with `stats_flush_interval`
as the banked first candidate (both sides measured at the phase-101 BRAINSTORM). Figures moved: check (1)
SILENT, NC-A and NC-B ONE, fixtures 128 (104 + 24), gate (b) 243, the house guard DISARMED, the ledger chain
ending in the phase-101 `+0` entry, the archive triple, the §Recent projection, the port census (`0126` now
holds `15126` and `15232`-`15236`). Method note 2 reads `101 IMPL eleven`, and one note was added (106, for
item 1). The `YOUR STAGE` / `IMPL` / figure greps and the memory-slug audit are in the task report.

**IMPL file scope**: `git diff --numstat master | wc -l` reads **14** before this commit and **17** with it. The
squashed precedents read **20** each (`1ae29a1b` phase 100, `d5bc9153` phase 99, `c7bd2880` phase 96, by
`git show --numstat --format= <sha> | wc -l`). They already carry `STATE.md`, `STATE_HISTORY.md` and
`next-prompt.txt`, so 17 vs 20 is like for like.

**Line counts at this commit** (`wc -l`): `ROADMAP.md` 251, `DECISIONS.md` 19821, `BEHAVIOR_CONTRACT.md` 6002,
`STATE.md` 66, `STATE_HISTORY.md` 602, `101/BRAINSTORM.md` 535, `SPEC.md` 791, `PLAN.md` 2524, this file
601. Phase dirs 142; fixtures 128 = 128.

## Task 16: squash, merge, push, worktree removal — done by the controller, not recorded here.
