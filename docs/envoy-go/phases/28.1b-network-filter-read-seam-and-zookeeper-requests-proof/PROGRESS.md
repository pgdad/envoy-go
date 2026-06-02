# Phase 28.1b — network-filter read-seam and zookeeper-requests proof: IMPL PROGRESS

**Phase:** 28.1b — network-filter-read-seam-and-zookeeper-requests-proof
**IMPL session cold-start:** 2026-06-02
**Worktree tip at session open:** `6581558` (docs-only next-prompt repoint; substantive predecessor `eff30e8` = 28.1b-PLAN squash)

---

## Task 1: Baselines/anchors gate (no code change)

**Date:** 2026-06-02
**Status:** DONE — all counts match expectations; all 25 line anchors hold with zero drift.

### Step 1: Project counts at IMPL-session tip

```
$ git log --oneline -1
6581558 next-prompt.txt: repoint master-tip reference to fd93131 (actual HEAD; trails 28.1b-PLAN squash eff30e8 +1)

$ ls -d test/fixtures/[0-9]* | wc -l
48

$ ls -d test/fixtures/[0-9]* | tail -1
test/fixtures/0046-zookeeper-requests

$ grep -n "0046-zookeeper-requests/driver" test/differential/runner_test.go
77:	// _ "github.com/esalaine/envoy-go/test/fixtures/0046-zookeeper-requests/driver"

$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
37

$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3
ADR-0221
ADR-0222
ADR-0223
```

| Metric | Expected | Actual | Match? |
|---|---|---|---|
| Fixture dirs on disk | 48 | 48 | YES |
| Last fixture dir | `test/fixtures/0046-zookeeper-requests` | `test/fixtures/0046-zookeeper-requests` | YES |
| 0046 blank-import status | ONE commented line (~:77) | line 77, commented | YES |
| Fuzz functions | 37 | 37 | YES |
| DECISIONS.md tail | ADR-0223 (next-free ADR-0224) | ADR-0221/ADR-0222/ADR-0223 (tail = ADR-0223) | YES |

**Counts at 28.1b open:** 48 fixture dirs on disk (47 active + `0046` committed-but-DISABLED), fuzzers 37. DECISIONS.md tail ADR-0223 → next-free ADR-0224. 28.1b lands `0046` re-enabled + `0047` → **49 active**; fuzzers STAY 37 (no new fuzzer — SPEC §2.6); ADR-0221/0222 bodies IN PLACE (no new ADR number).

---

### Step 2: Stat surface

From `docs/envoy-go/BEHAVIOR_CONTRACT.md:462`:
> **Phase 26.3 extension — 132 → 136 internal names**

Stat surface = **136** (BEHAVIOR_CONTRACT.md cumulative narrative accounting — the stat-table doc count; last delta = the phase-26.3 block at BEHAVIOR_CONTRACT.md:462 '132 → 136 internal names'). 28.1b rolls the **BEHAVIOR_CONTRACT doc count** from 136 → **337** at Task 9, gated on the Task-7 0046 cross-side proof. NOTE: the 201 zookeeper counter objects themselves were created eagerly at 28.1a Task 8 (`internal/filter/network/zookeeperproxy/stats.go` `rosterSuffixes()` + `newRosterStats`) — Task 9 is a DOC-ONLY roll; no counter-creation code is involved.

---

### Step 3: As-built line anchors

All 25 anchors verified at the live IMPL tip. Zero drift — all line numbers match the PLAN exactly.

| # | File:line | Construct | Status |
|---|-----------|-----------|--------|
| 1 | `internal/filter/network/chain.go:230-232` | `terminalReady` | CONFIRMED |
| 2 | `internal/filter/network/chain.go:239-267` | `handleTerminal` (prefix drain `:241-245`; write-wrap `:256-262`) — Task-5 modification site | CONFIRMED |
| 3 | `internal/filter/network/chain.go:303-310` | `onData` | CONFIRMED |
| 4 | `internal/filter/network/chain.go:323-356` | `runData` (replayRead inserts AFTER it) | CONFIRMED |
| 5 | `internal/filter/network/chain.go:366-381` | `onDestroy` | CONFIRMED |
| 6 | `internal/filter/network/chain.go:146-192` | `chainRuntime` struct (`filters`/`writeFilters`/`buf` fields) | CONFIRMED |
| 7 | `internal/filter/network/buffer.go:9-31` | `Buffer` (`Append` `:14` / `Bytes` `:18` / `Len` `:21` / `Drain` `:25-31`) — Task-2 extension site | CONFIRMED |
| 8 | `internal/filter/network/writeconn.go:13-48` | `writeChainConn` (the symmetric precedent readconn.go mirrors) | CONFIRMED |
| 9 | `internal/filter/network/prefixconn.go:12-28` | `prefixConn` (`Read` serves prefix WITHOUT delegating — `:21-28`) | CONFIRMED |
| 10 | `internal/filter/network/zookeeperproxy/decoder.go:30-53` | `requestDecoder` (`chainConsumed` mark `:34-39`) | CONFIRMED |
| 11 | `internal/filter/network/zookeeperproxy/decoder.go:69-84` | `decodeOnData` (Task-2 modification site; frames loop `:74-84` UNCHANGED) | CONFIRMED |
| 12 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:65-68` | `OnData` (Task-2 pass-through site); `:76` no-op `OnWrite`; `:48-54` both-directions `filter` struct | CONFIRMED |
| 13 | `internal/filter/network/zookeeperproxy/fuzz_test.go:54/:57` | fuzzer's 2 `decodeOnData` call sites (Task-2 mechanical update) | CONFIRMED |
| 14 | `internal/filter/tcpproxy/filter.go:101-139` | `Handle` (pump A `:136`, pump B `:137`, `wg.Wait()` `:138`; READ-ONLY) | CONFIRMED |
| 15 | `internal/listener/manager.go:1025-1091` | `serveNetworkChain` (handoff-return `:1066-1069`; EOF delivery `:1073-1077`; READ-ONLY) | CONFIRMED |
| 16 | `test/differential/runner_test.go:72-77` | commented `0046` blank-import (Task-7 re-enable site) | CONFIRMED |
| 17 | `test/differential/runner_test.go:832/:845/:1263-1269` | `TCPSink` backend arm / `acceptSinkCounting` | CONFIRMED |
| 18 | `test/differential/runner_test.go:1069-1070` | `StatsAsserter` cross-side dispatch | CONFIRMED |
| 19 | `test/differential/fixture/fixture.go:70-77/:493-502/:505-508` | `StatsAsserter` / `TCPSink BackendKind = 28` / `BackendKindAware` | CONFIRMED |
| 20 | `test/differential/harness.go:340-352` | `BootRejectFixture` (Task-8 interface) | CONFIRMED |
| 21 | `test/fixtures/0046-zookeeper-requests/driver/driver.go:5-24` | DISABLED banner (Task-7 replacement site); driver = 881 LoC | CONFIRMED |
| 22 | `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` | `0047` template (220 LoC; `BootRejectScript` `:159`; `ExpectedBootErrorSubstring` `:163`) | CONFIRMED |
| 23 | `internal/stats/name.go:243-255` | `.zookeeper.` INLINE-PREFIX arm (`const zkSegment = ".zookeeper."` at `:255`) — Task-7 R4 break-(b) site | CONFIRMED |
| 24 | `docs/envoy-go/DECISIONS.md:14228/:14235/:14249` | ADR-0221 heading / its 28.1b §AMEND / ADR-0222 heading — Task-9 body-landing sites | CONFIRMED |
| 25 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:462/:3642/:3656` | 26.3 stat-table block / `### Stat surface` / `### Does not yet apply to` ("L4 write filters … deferred") — Task-9 edit sites | CONFIRMED |

**Notes on specific anchors verified:**

- Anchor 2: `handleTerminal` prefix-drain block is `:241-244` (4 lines: `if rt.buf.Len() > 0 {`, `prefix := make(...)`, `copy(...)`, `rt.buf.Drain(...)`); write-wrap block is `:256-262`. Both within `:239-267`. PLAN range `:241-245` includes the closing `conn = newPrefixConn(...)` line at 245; write-wrap `:256-262` matches exactly.
- Anchor 12: `filter` struct at `:48-54`; `OnData` at `:65-68`; `OnWrite` at `:76`. All match.
- Anchor 13: `decodeOnData` call sites at lines 54 and 57 in `fuzz_test.go`. Match.
- Anchor 24: ADR-0221 heading at line 14228; 28.1b AMEND at line 14235; ADR-0222 heading at line 14249. All match exactly. ADR-0223 heading is at line 14268 (not a Task-9 landing site).
- Anchor 25: line 3656 is `### Does not yet apply to`; line 3657 carries the "L4 write filters (`WriteFilter` / `onWrite`) — deferred with API-revision allowance (ADR-0213)" text (one line below the heading). PLAN says `:3656` → heading is indeed at 3656; the text about write filters is at 3657. No concern — Task-9 edits the section, not a single line.

---

### Step 4: Task-16 divergence reference column (Task-7 acceptance reference)

From `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md`, Task 16 "The blocking divergence (l_plain multi-frame connections)" table and the proof-of-cause green run:

**Reference column (what reference Envoy v1.37.2 counted on l_plain):**

| counter (`zk_plain.zookeeper.*`) | reference value |
|---|---|
| `connect_rq` | 2 |
| `ping_rq` | 1 |
| `getdata_rq` | 2 |
| `create_rq` | 1 |
| `close_rq` | 1 |
| `create2_rq` | 1 |
| `getchildren2_rq` | 1 |
| `setwatches2_rq` | 1 |
| `decoder_error` | 1 |
| `request_bytes` | 307 |

The proof-of-cause (Task 16, "Proof-of-cause" section) confirmed: with a temporary `readChainConn`, the subject matched the reference EXACTLY on all 10 l_plain counters:

```
envoy_zk_plain_zookeeper_connect_rq = 2
envoy_zk_plain_zookeeper_ping_rq = 1
envoy_zk_plain_zookeeper_getdata_rq = 2
envoy_zk_plain_zookeeper_create_rq = 1
envoy_zk_plain_zookeeper_close_rq = 1
envoy_zk_plain_zookeeper_create2_rq = 1
envoy_zk_plain_zookeeper_getchildren2_rq = 1
envoy_zk_plain_zookeeper_setwatches2_rq = 1
envoy_zk_plain_zookeeper_decoder_error = 1
envoy_zk_plain_zookeeper_request_bytes = 307
--- PASS: TestDifferential/0046-zookeeper-requests
```

**This is the Task-7 acceptance reference.** The 0046 fixture at Task-7 must achieve `--- PASS: TestDifferential/0046-zookeeper-requests` with subject counters matching this table exactly on l_plain.

---

### Summary

All baselines confirmed at expected values: fixtures **48** (47 active + `0046` committed-but-DISABLED), fuzzers **37**, stat surface **136**, DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**). All 25 as-built anchor rows HOLD with zero drift. Reference column pinned from 28.1 PROGRESS.md Task 16 "The blocking divergence (l_plain multi-frame connections)": 10 l_plain counters recorded with proof-of-cause green run. Gate GREEN. Ready to proceed to Task 2 (Buffer.TotalAppended + decoder feed re-base).

---

*Later tasks append their own sections below this line.*

## Task 2 — Buffer.TotalAppended (int64) + zookeeperproxy decoder feed re-base

**Goal:** make the zookeeperproxy request decoder drain-proof for the upcoming
read seam (Tasks 3–5) by re-basing its high-water mark from the physical
chain-buffer length onto a new monotonic `Buffer.TotalAppended()` counter. On
never-drained executions `TotalAppended() == Len()`, so the re-based feed is
byte-identical to the 28.1a feed — the §3.3 equivalence — which is why every
existing decoder assertion is unchanged.

### Files changed
- `internal/filter/network/buffer.go` — added `total int64` field;
  `Append` now does `b.total += int64(len(p))`; added accessor
  `TotalAppended() int64`. `Bytes`/`Len`/`Drain` unchanged.
- `internal/filter/network/buffer_test.go` — new
  `TestBufferTotalAppendedMonotonicUnderDrain` (monotonic across full Drain;
  nil-append no-op on both counters).
- `internal/filter/network/zookeeperproxy/decoder.go` — `chainConsumed` widened
  `int → int64` (lockstep with TotalAppended; D-S28.1b-1) with the §3.3
  soundness doc comment; `decodeOnData` signature `(chainBytes []byte)` →
  `(chainBytes []byte, totalAppended int64)`; the high-water-mark block now
  selects the trailing `totalAppended − chainConsumed` bytes of `chainBytes`.
  The frames loop is byte-for-byte unchanged.
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` — `OnData` now
  passes `buf.TotalAppended()` through; filter signature unchanged; still pure
  Continue, never drains (R3 / AMEND-A8).
- `internal/filter/network/zookeeperproxy/decoder_test.go` — 26 existing
  single-arg call sites mechanically widened to `decodeOnData(X, int64(len(X)))`
  (assertions UNCHANGED). `TestDecodeOversizedThenRecovers` got two local helper
  hoists (`oversized`, `cumulative`) because `d.chainConsumed` is now int64 and
  feeds `prior+int64(len(good))` as totalAppended — assertions unchanged.
  Three NEW §3.3 drain-regime tests added:
  `TestDecodeFeedAfterRuntimeDrain`, `TestDecodeHandoffBoundarySequence`,
  `TestDecodePartialFrameAcrossDrainBoundary`.
- `internal/filter/network/zookeeperproxy/fuzz_test.go` — 2 fuzzer call sites
  widened (`doubled` hoisted to a named variable).

### TDD evidence
- Buffer test verify-FAIL: `b.TotalAppended undefined (type *Buffer has no field
  or method TotalAppended)`.
- Decoder drain tests verify-FAIL: `too many arguments in call to
  d.decodeOnData — have ([]byte, int64) want ([]byte)`.
- After implementation: both pass.

### Final test results
- `go test ./internal/filter/network/ -run TestBuffer -v` → PASS
  (TestBufferDrainSemantics + TestBufferTotalAppendedMonotonicUnderDrain).
- `go test ./internal/filter/network/... -race -short` → all 7 packages ok
  (network, builtins, directresponse, echo, rbac, snicluster, zookeeperproxy).
- `go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperRequestDecode -v`
  → PASS (5 seeds + 1 corpus entry).
- 3 new drain-regime tests → PASS.
- `gofmt -l internal/filter/network/` → clean (nothing).
- `golangci-lint run ./internal/filter/network/...` → clean (exit 0).

### §3.3 equivalence audit
`git diff` of decoder_test.go shows ZERO removed/changed assertion lines; every
`+`-side `t.Fatalf`/`want`/comparison belongs to the three new tests. Only call
SIGNATURES + the two helper hoists changed in existing tests. Load-bearing
constraint satisfied.

---

## Task 3 — chainRuntime.replayRead — post-handoff observational replay (SPEC §3.2)

**Date:** 2026-06-02
**Status:** DONE — 4 new tests pass; zero behavioral delta to existing 7-package suite.

### What was implemented

Added `replayRead(p []byte, endStream bool)` method to `chainRuntime` in
`internal/filter/network/chain.go`, inserted after `runData` (ends at :356 in
the pre-Task-3 tree). The implementation is the SPEC §3.2 production code verbatim:

```go
func (rt *chainRuntime) replayRead(p []byte, endStream bool) {
    rt.buf.Append(p)
    for _, f := range rt.filters {
        _ = f.OnData(rt.buf, endStream) // Status ignored — observational (§3.5)
    }
    rt.buf.Drain(rt.buf.Len())
}
```

Field names (`rt.buf`, `rt.filters`) matched the as-built struct exactly — no
adaptation required.

### Test double

Added `recordingReadFilter` to `chain_test.go`. It uses the TotalAppended
high-water-mark discipline (same as the zookeeperproxy decoder re-based in
Task 2) to accumulate only novel bytes across drain-after-pass cycles. This
double is reused in Tasks 5-6 per the PLAN.

No name collision with any existing double in the test file.

### Four new tests

| Test | SPEC reference | What is proven |
|---|---|---|
| `TestReplayReadDeliversToAllFiltersInChainOrder` | §3.2 item 1 / D-28.1b-5 | Every filter in chain order receives every byte |
| `TestReplayReadStatusIgnoredMidChainStop` | §3.5 / §11.1 | A mid-chain StopIteration does NOT halt later filters; `connHalted` unchanged |
| `TestReplayReadDrainsAfterPass` | §3.2 items 2-3 | `Len()=0` after pass; `TotalAppended()` monotonic across drain; cross-pass byte continuity via high-water-mark |
| `TestReplayReadEndStream` | §3.2 item 4 | `endStream=true` is delivered even on zero-byte replay |

### Adaptations to PLAN code

None. Field names, constructor signature, and type names all matched exactly.

One gofmt fix: the `OnDestroy` method body on `recordingReadFilter` had manual
tab-alignment that gofmt normalised (single trailing tab before the brace block).
Applied `gofmt -w` before commit.

### TDD evidence

- **Verify-FAIL:** `rt.replayRead undefined (type *chainRuntime has no field or method replayRead)` — 6 compile errors across the 4 test functions.
- **Verify-PASS:** All 4 tests pass after implementation.

### Final test results

- `go test ./internal/filter/network/ -run TestReplayRead -v` → PASS (all 4).
- `go test ./internal/filter/network/... -race -short` → all 7 packages ok (network, builtins, directresponse, echo, rbac, snicluster, zookeeperproxy).
- `gofmt -l internal/filter/network/` → clean (nothing printed).
- `golangci-lint run ./internal/filter/network/...` → clean (exit 0).

### Additive delta verification

`replayRead` is a new method — no existing code path calls it yet (Task 4 wires
it via `readChainConn.Read`). Zero behavioral delta to the existing test corpus
confirmed by the unchanged race suite.

---

## Task 4 — readconn.go — readChainConn (SPEC §3.1)

**Date:** 2026-06-02
**Status:** DONE — 4 new tests pass; zero behavioral delta to existing 7-package suite.

### What was implemented

Created `internal/filter/network/readconn.go` with the `readChainConn` type: the
read-direction half of the terminal-handoff conn-wrap seam (ADR-0221 §AMEND).
Mirrors the `writeChainConn` / `prefixConn` embed-and-override-one-method shape.

Key behavioral properties:

- **Passthrough:** `Read` delegates to the embedded `net.Conn`, then returns the
  bytes unmodified to the terminal (the terminal is unaware of the intercept).
- **Replay-before-return:** `replayRead(b[:n], false)` is called BEFORE `return n, err`,
  so filters (e.g. the zookeeperproxy decoder) see every byte and increment their
  stats BEFORE the terminal processes the data (§5.1 deterministic-scrape ordering).
- **EOF endStream replay:** when `err == io.EOF`, a second `replayRead(nil, true)`
  delivers the final `endStream=true` pass, mirroring the pre-handoff read-loop
  `serveNetworkChain` EOF delivery at `manager.go:1073-1077`.
- **Non-EOF error passthrough:** any other error propagates verbatim; no
  `endStream` replay (the connection is not cleanly closed).
- **Zero-byte read guard:** `n > 0` check suppresses replay for zero-byte reads
  (avoids empty `OnData` passes that would misalign the high-water-mark accounting).

### Test double

Created `multiReadConn` in `readconn_test.go` — yields one scripted payload per
`Read` call, then `io.EOF` forever. Embeds `scriptConn` (from `chain_test.go`)
for the no-op `net.Conn` methods. Required because `chain_test.go`'s `scriptConn`
is single-read (one live payload then EOF) and the readconn tests need multiple
distinct live reads.

Also created `errConn` — returns a fixed non-EOF error on every `Read`.

`recordingReadFilter` (Task 3, `chain_test.go`) is reused as-is.

### Four new tests

| Test | What is proven |
|---|---|
| `TestReadChainConnPassthroughAndReplay` | Bytes pass through unchanged; filter sees them before Read returns (replay-before-return, §5.1) |
| `TestReadChainConnEOFEndStreamReplay` | EOF triggers a final `endStream=true` replay pass in addition to the live-bytes replay |
| `TestReadChainConnNonEOFErrorNoEndStreamReplay` | Non-EOF error propagates verbatim; no `endStream` replay |
| `TestReadChainConnZeroByteReadNoReplay` | Zero-byte read produces zero replay passes (no empty OnData calls) |

### Adaptations to PLAN code

None. All as-built names (`scriptConn`, `fakeConn`, `connFacts`, `newChainRuntime`,
`recordingReadFilter`) matched the PLAN exactly.

### TDD evidence

- **Verify-FAIL:** 4 compile errors (`undefined: newReadChainConn` at each test function).
- **Verify-PASS:** All 4 tests pass after implementation.

### Final test results

- `go test ./internal/filter/network/ -run TestReadChainConn -v` → PASS (all 4).
- `go test ./internal/filter/network/... -race -short` → all 7 packages ok (network, builtins, directresponse, echo, rbac, snicluster, zookeeperproxy).
- `gofmt -l internal/filter/network/` → clean (nothing printed).
- `golangci-lint run ./internal/filter/network/...` → clean (exit 0).

### Additive delta verification

`readChainConn` is a new type — no existing code path constructs one yet (Task 5
installs it in `handleTerminal`'s wrap composition). Zero behavioral delta to the
existing test corpus confirmed by the unchanged race suite.

### Code-review amendments (2026-06-02)

A post-commit review added two fixes: (1) `TestReadChainConnBytesAndEOFSameCall` — a new test using a `bytesAndEOFConn` double that returns bytes AND `io.EOF` in the same `Read` call, guarding the two-independent-`if` structure in `readChainConn.Read` against a future `else if` regression; (2) the `Read` method doc comment citation `manager.go:1073-1077` was qualified to `internal/listener/manager.go:1073-1077 (serveNetworkChain)` (both in `readconn.go` and the matching comment in `readconn_test.go`). Both fixes amended into the Task 4 commit; `TestReadChainConn` suite now counts 5 tests, all PASS.

## Task 5 — handleTerminal read-wrap insertion + composition / R1 / soundness-invariant tests (SPEC §3.4 / §3.3)

### What was done

Installed the read-side seam in `handleTerminal` (`internal/filter/network/chain.go`)
as the INNERMOST conn-wrap, under the SAME `len(rt.writeFilters) > 0` predicate as
the existing writeChainConn wrap. The two load-bearing changes:

1. `if len(rt.writeFilters) > 0 { conn = newReadChainConn(conn, rt) }` placed BEFORE
   the prefixConn block so readChainConn is innermost.
2. The prefixConn block now wraps the running `conn` variable (was `rt.conn`), so the
   composition is `writeChainConn(prefixConn(readChainConn(rawConn)))`. Innermost
   readChainConn is load-bearing: prefixConn serves its buffered prefix WITHOUT
   delegating inward, so prefix bytes (already seen pre-handoff) are NOT re-fed through
   the replay; only LIVE post-handoff socket reads pass through readChainConn.Read.

The `handleTerminal` doc comment gained a paragraph noting the read-side wrap + the
§3.1 composition; the writeChainConn block comment updated to OUTER/MIDDLE/INNER.

### Existing-test updates (the two specified)

- `TestHandleTerminalZeroWriteFiltersUnwrapped` (~chain_test.go:592-600) — DELETED.
  Superseded by the strict-superset `TestHandleTerminalZeroWriteFiltersNeitherWrap`
  (asserts NEITHER writeChainConn NOR readChainConn for zero-write-filter chains).
- `TestHandleTerminalWrapComposition` (~:605-626) — UPDATED in place: kept the
  `writeChainConn → prefixConn` and the "Read = prefix replay" assertions; ADDED an
  inner assertion that `prefixConn.Conn` is now a `*readChainConn` (was the raw conn
  at 28.1a).

### New Task-5 tests (appended to chain_test.go)

`TestHandleTerminalZeroWriteFiltersNeitherWrap`, `…PrefixOverRawConn`,
`…FullCompositionOrder`, `…CompositionNoPrefix`, `…PrefixNotReFedThroughReplay`,
`TestChainSoundnessInvariantEveryByteSeenExactlyOnce`. Reused the as-built doubles:
`recordingTerminal` (gotConn), `fakeWriteFilter`, `fakeConn`, `recordingReadFilter`
(Task 3), `multiReadConn` (Task 4, readconn_test.go).

### Verify-FAIL evidence (Step 2)

`go test ./internal/filter/network/ -run 'TestHandleTerminal|TestChainSoundness' -v`
before implementing the wrap:
- FAIL `TestHandleTerminalWrapComposition` — `prefixConn wraps *network.fakeConn, want *readChainConn`.
- FAIL `TestHandleTerminalFullCompositionOrder` — `inner = *network.fakeConn, want *readChainConn`.
- FAIL `TestHandleTerminalCompositionNoPrefix` — `inner = *network.fakeConn, want *readChainConn`.
- FAIL `TestHandleTerminalPrefixNotReFedThroughReplay` — `filter saw "pre", want preLIVE`.
- FAIL `TestChainSoundnessInvariantEveryByteSeenExactlyOnce` — `filter saw "abc", want abcdefghijkl`.
- PASS (already) `…NeitherWrap`, `…PrefixOverRawConn` — the R1 zero-write-filter tests
  need nothing installed (no wrap path for them), exactly as the PLAN predicted.

### Soundness-test adaptation (semantics preserved; mechanism made runtime-faithful)

The PLAN's `TestChainSoundnessInvariantEveryByteSeenExactlyOnce` drove TWO pre-handoff
`onData` calls (`"abc"` then `"def"`). That contradicts the as-built runtime: once every
read filter Continues, `terminalReady()` is true and serveNetworkChain's read loop hands
off and RETURNS at the first `TerminalReady` check (internal/listener/manager.go:1066-1068)
— a passthrough chain sees exactly ONE pre-handoff read; the rest are post-handoff replays.
A second pre-handoff `onData` appends undrained bytes the filter never saw (runData exits
immediately because `resumeIdx >= len(filters)`), so the prefix drain advances the buffer's
TotalAppended past the filter's high-water mark → the live replay computes a negative slice
bound and PANICS. That panic is the high-water-mark discipline (the exact discipline the
test asserts) being violated by an unfaithful pre-handoff feed, not a production bug.

Adaptation: feed the pre-handoff bytes in a SINGLE `onData("abcdef")` (the faithful
one-pre-handoff-read model) and assert the filter saw them pre-handoff. The exact invariant
assertion is unchanged — `f.seen == "abcdefghijkl"` after the prefix drain + two live replays
(every appended byte seen exactly once across pre-handoff, handoff drain, post-handoff replay).
The other soundness-shaped test `TestHandleTerminalPrefixNotReFedThroughReplay` already used a
single pre-handoff `onData` and needed no change.

### Final test results (Step 4)

- `go test ./internal/filter/network/ -run 'TestHandleTerminal|TestChainSoundness' -v` → all PASS.
- `go test ./internal/filter/network/... -race -short` → all 7 packages ok.
- `go test ./internal/... -short` → all green (no R1 violation in listener/manager, which calls handleTerminal).
- `gofmt -l internal/filter/network/` → clean. `golangci-lint run ./internal/filter/network/...` → clean.

### Self-review

- R1: zero-write-filter chains get NEITHER wrap; predicate is exactly `len(rt.writeFilters) > 0` in both places. ✓
- prefixConn wraps the running `conn` variable (the readChainConn when present), not `rt.conn`. ✓
- All pre-existing tests stayed green; only the two specified updates (one delete, one in-place assertion add). ✓
- gofmt/lint clean; race suite green. ✓

---

## Task 6 — §3.6 concurrent-pumps race test (D-S28.1b-2)

**Date:** 2026-06-02
**Status:** DONE — test passes with -race -count=5, zero race reports; full package suite clean.

### What was implemented

Appended to `internal/filter/network/chain_test.go`:

**New types:**
- `pumpingTerminal` — TerminalFilter that mirrors the tcp_proxy.Handle goroutine topology (filter.go:134-138): two concurrent `io.Copy` pumps (goroutine A: downstream→upstream; goroutine B: upstream→downstream) + wg.Wait. Pump A goroutine closes both `downstream` and `upstream` after it exits, ensuring clean termination in all scheduling orders (see topology analysis below).
- `raceBothFilter` — both-directions filter implementing ReadFilter + WriteFilter: `OnData` counts novel bytes via the TotalAppended high-water-mark discipline (atomic.Int64); `OnWrite` is a pure no-op Continue. Both compile-time assertions added.

**New test:** `TestWrappedChainConcurrentPumpsRace`

**Imports added:** `sync`, `sync/atomic` (were not present in chain_test.go).

### API adaptations

The PLAN's `NewChainRuntime` / `TerminalReady()` / `HandleTerminal()` / `OnNewConnection()` names matched the as-built exported API exactly.

The PLAN's `OnData` + `TerminalReady` sequence required a non-obvious fix: after `OnNewConnection()`, `resumeIdx` is reset to 0 by the loop's terminal condition, so `terminalReady()` (which checks `resumeIdx >= len(filters)`) returns false for a chain with 1 read filter. An `OnData(nil, true)` call (zero bytes, endStream=true) drives `runData()` past the filter — `raceBothFilter.OnData` sees TotalAppended=0 → adds nothing to `readBytes` — advancing `resumeIdx` to 1 = len(filters). No prefix bytes are buffered. `terminalReady()` returns true. ✓

### net.Pipe termination analysis

`net.Pipe()` writes block until the other end reads (no buffering). The test uses six goroutines (4 traffic + 2 pumps inside Handle) to ensure every written byte has a reader:

```
client writer  → testEnd.Write  → pump A chainEnd.Read  → replayRead → upstream A write
pump A         → upstreamEnd.Write → backend reader backendEnd.Read  (discarded)
backend sender → backendEnd.Write → pump B upstreamEnd.Read → chainEnd.Write
pump B         → chainEnd.Write → client reader testEnd.Read        (discarded)
```

Termination sequence:
1. Client writer finishes 50 writes (each blocks until pump A reads) → calls `testEnd.Close()` → pump A's chainEnd.Read returns error → pump A's io.Copy exits.
2. Pump A goroutine calls `downstream.Close()` (unblocks any stuck testEnd.Write or pump B's chainEnd.Write) and `upstream.Close()` (unblocks pump B if it is waiting on upstreamEnd.Read with no more backend data).
3. Pump B exits (either chainEnd.Write failed or upstreamEnd.Read failed). Handle's wg.Wait returns. HandleTerminal returns. `done` channel closes.
4. Test calls `upstreamEnd.Close()` (idempotent if pump A already closed it). Backend sender's backendEnd.Write and backend reader's backendEnd.Read both fail → both exit. All 4 traffic goroutines exit.

**Byte-count determinism:** All 50 `testEnd.Write` calls are synchronous (each blocks until pump A's `chainEnd.Read` succeeds). `readChainConn.Read` calls `replayRead` BEFORE returning the bytes to io.Copy (§5.1 replay-before-return). So all 850 bytes are counted before `testEnd.Close()` is even called — the assertion is unconditionally live.

The PLAN's backend sender used `backendEnd.Close()` to terminate pump B, which caused pump A's `upstreamEnd.Write` to fail mid-stream (ErrClosedPipe), leaving the client writer blocked. This was fixed by: (a) removing `backendEnd.Close()` from the backend sender, (b) having pump A's goroutine close both `downstream` and `upstream` after pump A's io.Copy exits, and (c) calling `upstreamEnd.Close()` after HandleTerminal returns.

### TDD evidence (verify-FAIL step)

Before adding the implementation:
- `crt.OnData(nil, true)` instead of no OnData call fixed: "chain must be terminal-ready" fatal → passes.
- The initial PLAN test code (missing backend reader goroutine) deadlocked: identified by running without `-race` (passes) vs with `-race` (30s timeout). Root cause: pump A's `upstreamEnd.Write` had no reader, blocking the client writer.

### Final test results

- `go test ./internal/filter/network/ -run TestWrappedChainConcurrentPumpsRace -race -count=5 -v` → 5/5 PASS, zero race reports.
- `go test ./internal/filter/network/... -race -short` → all 7 packages ok.
- `gofmt -l internal/filter/network/` → clean (nothing printed).
- `golangci-lint run ./internal/filter/network/...` → clean (exit 0).

### Self-review

- Two-pump topology with genuinely concurrent bidirectional traffic: goroutine A and goroutine B both run during the test (both pumps are alive simultaneously while client and backend data flows). ✓
- `-race -count=5` → 5/5 clean passes. ✓
- Byte-count assertion is live: if replayRead dropped bytes, `f.readBytes.Load()` would be < 850 and the test would fail. ✓
- Test-only change confirmed: `git diff` shows only `chain_test.go` + `PROGRESS.md`. ✓
- gofmt/lint clean. ✓

## Task 7 — 0046-zookeeper-requests RE-ENABLE + cross-side GREEN + R4 + README

The proof task. The read seam (Tasks 2–5) makes the previously-blocked
zookeeper fixture pass against reference Envoy v1.37.2.

### Step 1–2: Re-enable + banner

- `test/differential/runner_test.go`: deleted the 6-line DISABLED comment block
  and restored the plain blank-import line for
  `0046-zookeeper-requests/driver`. Ran `gofmt -w` + `goimports -w`; the import
  group is canonical (no comment gap). `gofmt -l` on the file → clean.
- `test/fixtures/0046-zookeeper-requests/driver/driver.go:5-24`: replaced the
  `DISABLED at 28.1a` banner with the `RE-ENABLED at 28.1b — the read-side seam`
  banner (D-S28.1b-3). ONLY the banner comment changed; `git diff` of the driver
  is comment-only (verified: the non-comment line diff is empty).

### Step 3: GREEN on all 7 arms

```
$ go test ./test/differential/ -run 'TestDifferential/0046-zookeeper-requests' -count=1 -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0046-zookeeper-requests
--- PASS: TestDifferential (4.77s)
    --- PASS: TestDifferential/0046-zookeeper-requests (4.77s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	4.853s
```

Arm-by-arm seam dependency:
- Arms 1 (single connect), 5 (single getdata on l_flags), 6 (exists-at-zero,
  no traffic) were ALREADY green at 28.1a — single-read or no-traffic.
- Arms 2 (5 paced frames), 3 (3 paced frames), 4 (oversized garbage → pause →
  recovery getdata, same connection) are MULTI-SOCKET-READ connections — RED at
  28.1a (the subject undercounted because the chain runtime stopped feeding
  filters after terminal handoff). These are the seam's R8 proof: the
  `readChainConn` + `chainRuntime.replayRead` re-feed of post-handoff reads now
  delivers every frame to `zookeeper_proxy.OnData`, matching reference Envoy's
  forever-re-iteration. Green confirms the seam closes the 28.1a gap.

No flakes on this run (no `freeTCPPort` bind error). First run (cached) and the
`-count=1` re-run both PASS.

### Step 4: R4 deliberate-break protocol (both breaks LIVE, both reverted)

**Break (a) — wrong expected value.** Edited the driver expectation
`{"zk_plain.zookeeper.getdata_rq", 2}` → `3`. Run output:

```
runner_test.go:1064: ref zk_plain.zookeeper.getdata_rq = 2, want 3
runner_test.go:1064: subj zk_plain.zookeeper.getdata_rq = 2, want 3
--- FAIL: TestDifferential (4.81s)
    --- FAIL: TestDifferential/0046-zookeeper-requests (4.81s)
FAIL
```

Both sides fail → the assertion runs against ref AND subj. Reverted the edit;
`git diff --stat test/fixtures/0046-zookeeper-requests/driver/` shows only the
banner change, and the non-comment line diff is empty.

**Break (b) — name-shape liveness.** Commented out the `.zookeeper.` `zkSegment`
arm in `internal/stats/name.go:255-262`. With this arm gone, `flattenToProm`
errors on every `<prefix>.zookeeper.<counter>` name (confirmed independently:
`go test ./internal/stats/ -run Zookeeper -count=1` → the 5 zookeeper name tests
FAIL with "no recognized top-level segment"), so `WriteProm` drops the lines and
the subject's `/stats/prometheus` no longer renders the zookeeper counters.

IMPORTANT CACHING NOTE / near-flake: the FIRST run of break (b) WITHOUT
`-count=1` reported `--- PASS` — a `go test` result-cache hit (the docker-driven
differential test had passed before name.go changed, and the cache key did not
invalidate for this package's run). Re-running with `-count=1` produced the
genuine LIVE FAIL:

```
runner_test.go:1064: subj: counter zk_plain.zookeeper.connect_rq ABSENT (creation parity / name-shape failure)
runner_test.go:1064: subj: counter zk_plain.zookeeper.ping_rq ABSENT (creation parity / name-shape failure)
runner_test.go:1064: subj: counter zk_plain.zookeeper.getdata_rq ABSENT (creation parity / name-shape failure)
... (all 15 fixed-value counters ABSENT on subj) ...
runner_test.go:1064: cross-side zk_plain.zookeeper.request_bytes: ref=(307,true) subj=(0,false), want present, equal, and > 0
runner_test.go:1064: cross-side zk_flags.zookeeper.getdata_rq_bytes: ref=(25,true) subj=(0,false), want present, equal, and > 0
--- FAIL: TestDifferential (5.01s)
    --- FAIL: TestDifferential/0046-zookeeper-requests (5.01s)
FAIL
```

The reference side retains all counters (`request_bytes ref=307`), the subject
drops them all → the subject-side name-shape assertion is LIVE. Reverted the
edit; `git diff --stat internal/stats/` is empty after revert.

LESSON RECORDED: run the differential fixture with `-count=1` for break/revert
verification — without it a stale cached PASS can mask a live production break.
The authoritative GREEN run in Step 3 above was also taken with `-count=1`.
NOTE FOR TASK 10: gate 5 (the full differential suite) and all other go test
gates should also use `-count=1` for the same reason — the warm cache from
earlier runs in this session could serve a stale PASS.

### Step 5: README

Authored `test/fixtures/0046-zookeeper-requests/README.md` (read 0043 + 0045
READMEs for format): topology + TCPSink-not-echo rationale, two listeners / two
stat_prefixes, the seam-dependency table (arms 2/3/4 = R8 proof; cites 28.1b
SPEC §3 + the 28.1a Task-16 divergence table), the 7-arm taxonomy with expected
counters, the `request_bytes=307` cross-side equality proof, the StatsAsserter
mechanics (both-sides `/stats/prometheus`, flat names per AMEND-A4), and the R4
deliberate-break record (both breaks + outputs + the `-count=1` caching note).

### Step 6: No-regression spot check

```
$ go test ./test/differential/ -run 'TestDifferential/(0001-tcp-proxy-rr|0043-network-rbac|0045-sni-cluster)' -count=1 -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0001-tcp-proxy-rr
=== RUN   TestDifferential/0043-network-rbac
=== RUN   TestDifferential/0045-sni-cluster
--- PASS: TestDifferential (9.56s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	9.650s
```

3/3 PASS (the R1 zero-write-filter spot check before the Task-10 full gate).

### gofmt + lint

- `gofmt -l test/` → clean (empty).
- `golangci-lint run ./test/...` → clean (exit 0).

---

## Task 8: 0047-zookeeper-boot-reject fixture (SPEC §5.2)

**Date:** 2026-06-02
**Status:** DONE — symmetric PGV-mirror boot-reject; fixture GREEN (-count=1).

### Step 1: Driver authoring

Mirrored `0044-network-rbac-boot-reject/driver/driver.go` (~220 LoC)
with ZooKeeperProxy substitutions:
- `fixtureName = "0047-zookeeper-boot-reject"`
- `refZKPort = 15049` (next free: 0044=15046, 0046=15047/15048)
- `expectedBootErrorSubstr = "stat_prefix"` (0044 precedent; see substring
  discipline below)
- Bootstrap: `[zookeeper_proxy (stat_prefix omitted), tcp_proxy] + c_unused
  cluster` — same chain shape as 0046's `l_plain` MINUS `stat_prefix`.
- Compile-time assertion: `var _ fixture.Driver = (*zkBootRejectDriver)(nil)`
- No harness import needed (BootRejectFixture is duck-typed by the runner;
  0044 sets the precedent of not importing the differential package).

Port conflict check: `grep -rn "15049"` returned only this new file — clean.

### Step 2: runner_test.go blank-import

Added after the `0046` line:
```go
_ "github.com/esalaine/envoy-go/test/fixtures/0047-zookeeper-boot-reject/driver"
```

### Step 3: Fixture run

```
$ go test ./test/differential/ -run 'TestDifferential/0047-zookeeper-boot-reject' -count=1 -v 2>&1 | tail -40
```

Output (tail):
```
...
goo.gle/debugproto
max_packet_bytes {
  value: 1048576
}
: Proto constraint validation failed (ZooKeeperProxyValidationError.StatPrefix: value length must be at least 1 characters)

2026/06/02 11:24:48 🐳 Terminating container: 47eaadf7c19c
2026/06/02 11:24:48 🚫 Container terminated: 47eaadf7c19c
2026/06/02 11:24:48 listener manager: listener: "l_zk": filter_chains[0]: filters[0]: zookeeper_proxy: stat_prefix is required
--- PASS: TestDifferential (1.71s)
    --- PASS: TestDifferential/0047-zookeeper-boot-reject (1.71s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.799s
```

**Both sides boot-reject. PASS.**

### Actual error messages (captured live, -count=1)

- **reference Envoy v1.37.2 stderr** (PGV violation; PascalCase field name):
  `Proto constraint validation failed (ZooKeeperProxyValidationError.StatPrefix: value length must be at least 1 characters)`
- **envoy-go stderr** (snake_case field name):
  `zookeeper_proxy: stat_prefix is required`

### Substring discipline

`stat_prefix` is the chosen substring — identical to the 0044 precedent.
The two error wordings share NO distinctive case-sensitive token (same
PascalCase vs snake_case asymmetry as 0044). The `stat_prefix` substring:
- **subject side**: matches the envoy-go error wording verbatim; fully
  load-bearing (subject stderr is just the error line, no YAML echo).
- **reference side**: reference Envoy echoes the offending bootstrap into
  its stderr, so `stat_prefix` (the tcp_proxy filter's `stat_prefix:
  ingress_tcp` field) appears there. The genuine reference-reject assertion
  is the runner's separate `refErr != nil` gate.

No IMPL-note adaptation needed — `stat_prefix` is present in both sides'
stderrs (reference via bootstrap echo; subject via its own error wording).

### Step 4: README

Authored `test/fixtures/0047-zookeeper-boot-reject/README.md` mirroring
0044's format: fixture type, boot-reject trigger, one-dir-one-branch
dispatch discipline, substring discipline with both actual error messages
quoted, bootstrap discipline, latency arms' deferred disposition.

### gofmt + lint

- `gofmt -l test/` → clean (empty).
- `golangci-lint run ./test/...` → clean (exit 0).

## Task 9: Completion bundle — ADR-0221 (both-seams body) + ADR-0222 bodies in place + BEHAVIOR_CONTRACT 28.1 bundle (SPEC §6.1/§6.2)

**Status:** DONE — docs-only (DECISIONS.md + BEHAVIOR_CONTRACT.md + this PROGRESS.md). No production or test code touched. DECISIONS.md tail STAYS **ADR-0223** (no new ADR number minted; ADR-0044 in-place body discipline).

### Step 1 — ADR-0221 §Decision + §Consequences (in place, after its §Context/§AMEND)

Landed the body covering BOTH halves of the terminal-handoff conn-wrap seam:

- **§Decision write half (28.1a):** the `WriteFilter`/`WriteFilterCallbacks` interfaces (`OnWrite(buf, endStream) Status`; `OnDestroy` on the interface; minimal `Connection()`-only callbacks); the independent-type-assert classification (read/write/both/terminal — a both-directions filter is the SAME instance in both sets; dual callback injection; once-per-instance `OnDestroy` dedupe via interface-identity map); the REVERSE-chain-order write dispatch (AMEND-A11 LIFO); the `writeChainConn` + D-P7 return semantics (StopIteration → `(len(p), nil)`); the write-only-filter boot boundary (manager.go untouched); the terminal-originated-writes-only boundary.
- **§Decision read half (28.1b):** the `readChainConn` (innermost wrap; replay-before-return; EOF endStream replay); `chainRuntime.replayRead` (all read filters, chain order, Status ignored, drain-after-pass); the `Buffer.TotalAppended` (int64) decoder-feed re-base + the soundness invariant; the SHARED `len(writeFilters) > 0` wrap predicate (R1); the composition `writeChainConn(prefixConn(readChainConn(conn)))`.
- **§Consequences:** CONSUMES the ADR-0213 §Decision item-8 API-revision allowance (consumer #1 zookeeper; anticipated #2 mongo); the three observational post-handoff boundaries (§3.5); the §3.4 future-generalization note; the wrapped-chains-only hot-path cost; **the §3.6 concurrency pins + the 28.2 FORWARD-POINTER** (the per-connection `sync.Mutex` the 28.2 SPEC MUST add to guard the correlation maps).

Each §Decision claim verified against the as-built code before writing: the interfaces (`internal/filter/network/types.go:57-84`); the independent-type-assert classification + dual injection (`chain.go:66-100`); the once-per-instance OnDestroy dedupe (`chain.go:405-419`); the reverse-order dispatch (`chain.go:274-279`); `writeChainConn` D-P7 (`writeconn.go:34-48`); the shared predicate + composition (`chain.go:255,258-263,274`); `readChainConn.Read` (`readconn.go:34-43`); `replayRead` (`chain.go:389-395`); the `Buffer.total int64` + `TotalAppended()` (`buffer.go:18,24,29`); the re-based feed (`decoder.go:75-79`).

### Step 2 — ADR-0222 §Decision + §Consequences (in place, after its §Context)

Landed the request-package body: TypeURL via `proto.MessageName` + `NewFactory(reg)` closure-capture (`zookeeperproxy.go:17,26-43`); the 9-field parse + PGV-mirror PARSE-REJECT arms + proto→wire opcode mapping (`config.go:47-75,148-211`); the **201**-counter eager roster + creation parity (D-P5) + the dynamic `auth.<scheme>_rq` counters (`stats.go`); the shallow decoder + D-S28.1-1 min-length table + AMEND-A8 no-resync + the 28.1b §3.3 TotalAppended feed re-base (`decoder.go`); the two correlation structures (written 28.1, consumed 28.2 — R5); the `.zookeeper.` name.go arm (AMEND-A4 flat Prometheus, `name.go:243-255`); the both-directions filter glue + pure no-op `OnWrite` (`zookeeperproxy.go:45-88`); the fixtures `0046`/`0047` + the TCPSink pin + the 37th fuzzer; the AMEND-A9 dynamic-metadata deferral + the shallow-decode leniency departure.

**Roster count VERIFIED = 201** against `stats.go`: 4 plain + 28 `_rq` + 29 `_rq_bytes` + 28 `_decoder_error` + 28×4 `_resp*` = 201 (counted from the opname lists; matches `rosterSuffixes()`'s `make([]string, 0, 201)` + `counters: make(..., 201)`).

### Step 3 — BEHAVIOR_CONTRACT 28.1 bundle (4 edits, ADR-0052 atomic)

1. **Stat-table roll 136 → 337** — new `**Phase 28.1 extension — 136 → 337 internal names:**` block in `## Stat-name mapping` (after the 26.3 block): the 201 families + asymmetries (connect_readonly rq-only; NO static auth_rq → dynamic `auth.<scheme>_rq`; auth_resp present; the 29-name `_rq_bytes` family; flags gate increments not creation; response-side exists-at-zero until 28.2). Records explicitly that the roll lands at 28.1b (cross-side-PROVEN by the green 0046), normatively defined by `rosterSuffixes()` + the R2 golden-list test (no per-row enumeration).
2. **NEW `### Network filter chain framework — terminal-handoff conn-wrap seam (28.1 amendment)`** — both directions: write-side items (reverse dispatch, StopIteration-no-forward documented-unsupported, terminal-originated-writes-only, write-only-filter boot boundary, writeChainConn) + read-side items (replay §3.2, shared predicate + R1 §3.4, the TotalAppended soundness invariant §3.3, the three observational boundaries **as a table** §3.5, the goroutine-placement note + the 28.2 sync forward-pointer §3.6).
3. **NEW `### envoy.filters.network.zookeeper_proxy` subsection** (after sni_cluster): request-side semantics; the 201-roster + creation parity; the `<stat_prefix>.zookeeper.` scope; flat Prometheus (AMEND-A4); the dynamic auth counters; the shallow-decode leniency departure; the dynamic-metadata coverage boundary (AMEND-A9); the access_log parse-accept-ignore + parsed-not-consumed latency note; the re-iteration guarantee R8 (cross-side-PROVEN, 0046 green).
4. **Stat surface / Applies to / Does not yet apply to updates** — the 28.1 stat-surface sentence (136 → 337; fixtures 47 → 49; fuzzers 36 → 37); the old "L4 write filters deferred (ADR-0213)" bullet REPLACED by "Response-side decode + latency counters (28.2 / ADR-0223)" + a new §3.5 observational-boundaries bullet; `### Applies to` gains the Phase-28.1-onward entry; the stale OPEN-family candidate list dropped `zookeeper`/`sni_cluster`.

### Self-review

- DECISIONS.md tail STAYS **ADR-0223** — verified `grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1` → `ADR-0223` (an earlier draft tripped this with a "next-free ADR-0224" prose reference; reworded to "no new ADR number is minted").
- The **337** count appears consistently across the four BEHAVIOR_CONTRACT edits.
- The **28.2 synchronization forward-pointer** (per-connection `sync.Mutex` on the decoder guarding the correlation maps) is present in BOTH ADR-0221 §Consequences AND the BEHAVIOR_CONTRACT seam block.
- Docs-only diff — `git status --short` shows only DECISIONS.md + BEHAVIOR_CONTRACT.md + this PROGRESS.md.

### Discrepancy resolved

The `_rq_bytes` family carries **29** names (vs 28 for `_rq`/`_decoder_error`/`_resp`) — the extra is `auth_rq_bytes` (the `_rq` family has NO `auth_rq`, but `_rq_bytes` carries `auth_rq_bytes` + `connect_readonly_rq_bytes` + `setauth_rq_bytes`). Verified against `stats.go`'s `rqBytesOpNames` (29 entries) vs `rqOpNames` (28). The 136 → 337 block records the 29-name `_rq_bytes` asymmetry explicitly so the families sum to 201.

---

## Task 10: Six-gate + STATE.md + ROADMAP sub-row advance + next-prompt.txt (SPEC §6.3/§11.2)

**Date:** 2026-06-02
**Status:** DONE — all six gates GREEN LIVE (`-count=1`); gate 5 one unrelated HTTP-fixture flake (`0020`) re-run GREEN in isolation; the two new zookeeper fixtures (`0046`/`0047`) PASS in the full suite. STATE/ROADMAP/next-prompt advanced for the 28.2-SPEC cold-start.

### Counts confirmed (LIVE)

```
$ ls -d test/fixtures/[0-9]* | wc -l
49
$ ls -d test/fixtures/[0-9]* | tail -1
test/fixtures/0047-zookeeper-boot-reject
$ grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l
37
$ grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1
ADR-0223
$ grep -n "136 → 337" docs/envoy-go/BEHAVIOR_CONTRACT.md | head -1
464:**Phase 28.1 extension — 136 → 337 internal names:** ...
```

| Metric | Expected | Actual | Match? |
|---|---|---|---|
| Active fixture dirs on disk | 49 | 49 | YES |
| Last fixture dir | `0047-zookeeper-boot-reject` | `0047-zookeeper-boot-reject` | YES |
| Fuzz functions | 37 | 37 | YES |
| DECISIONS.md tail | ADR-0223 | ADR-0223 | YES |
| BEHAVIOR_CONTRACT 136 → 337 block | present | present (`:464`, `:3640`) | YES |

### Gate 1 — `go build ./...`

```
$ go build ./...
EXIT=0
```
Clean.

### Gate 2 — `go vet ./...`

```
$ go vet ./...
EXIT=0
```
Clean.

### Gate 3 — `golangci-lint run` (whole repo)

```
$ golangci-lint run
LINT_EXIT=0
```
Clean (no findings).

### Gate 4 — `go test ./... -race -short -count=1`

All packages `ok`; ZERO `FAIL` lines across the whole repo (confirmed by
`go test ./... -race -short -count=1 2>&1 | grep -cE "^FAIL"` → `0`). Network
filter packages:

```
ok  	github.com/esalaine/envoy-go/internal/filter/network	1.009s
ok  	github.com/esalaine/envoy-go/internal/filter/network/builtins	1.025s
ok  	github.com/esalaine/envoy-go/internal/filter/network/directresponse	1.014s
ok  	github.com/esalaine/envoy-go/internal/filter/network/echo	1.009s
ok  	github.com/esalaine/envoy-go/internal/filter/network/rbac	1.021s
ok  	github.com/esalaine/envoy-go/internal/filter/network/snicluster	1.010s
ok  	github.com/esalaine/envoy-go/internal/filter/network/zookeeperproxy	1.096s
TEST_EXIT=0
```

### Gate 5 — FULL differential suite (`go test ./test/differential/ -run TestDifferential -count=1 -v`)

49 active fixtures (`0000`..`0047`, all active now that `0046` is re-enabled and
`0047` added). First full run: 48/49 PASS with ONE flake on the unrelated HTTP
fixture `0020-http-ext-authz-http` (the `freeTCPPort` TOCTOU bind precedent from
the 28.1a closure; NOT a zookeeper/seam fixture). The two NEW seam-dependent
fixtures both PASS in the full run:

```
    --- FAIL: TestDifferential/0020-http-ext-authz-http (1.81s)
    ...
    --- PASS: TestDifferential/0046-zookeeper-requests (4.30s)
    --- PASS: TestDifferential/0047-zookeeper-boot-reject (1.57s)
FAIL
FAIL	github.com/esalaine/envoy-go/test/differential	158.491s
```

(All other 47 dirs PASS — the 47-dir R1 back-compat gate holds: `0000`–`0019`,
`0021`–`0045` all `--- PASS`.)

**Flake re-run in isolation (`-count=1`):**

```
$ go test ./test/differential/ -run 'TestDifferential/0020-http-ext-authz-http' -count=1 -v
=== RUN   TestDifferential/0020-http-ext-authz-http
--- PASS: TestDifferential (2.44s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (2.44s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.535s
RERUN_EXIT=0
```

`0020` PASSES in isolation → confirmed flake (TOCTOU port-bind, the documented
28.1a-closure `freeTCPPort` precedent), NOT a real failure. Effective gate-5
result: **49/49 PASS** (the 47-dir R1 back-compat gate + `0046` + `0047`).

### Gate 6a — h2spec (`go test ./test/conformance/h2spec/ -run TestH2Spec -count=1 -v`)

```
        53 tests, 53 passed, 0 skipped, 0 failed
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
--- PASS: TestH2Spec (2.66s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.749s
H2_EXIT=0
```
**53/53.**

### Gate 6b — proxy-wasm (`go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -count=1 -v`)

```
--- PASS: TestProxyWasmConformance (0.24s)
    --- PASS: TestProxyWasmConformance/exports (0.02s)
    --- PASS: TestProxyWasmConformance/security (0.04s)
    --- PASS: TestProxyWasmConformance/runtime (0.02s)
    --- PASS: TestProxyWasmConformance/wasm_vm (0.02s)
    --- PASS: TestProxyWasmConformance/bytecode_util (0.00s)
    --- PASS: TestProxyWasmConformance/logging (0.02s)
    --- PASS: TestProxyWasmConformance/stop_iteration (0.04s)
    --- PASS: TestProxyWasmConformance/shared_data (0.02s)
    --- PASS: TestProxyWasmConformance/pairs_util (0.02s)
    --- PASS: TestProxyWasmConformance/endianness (0.02s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.241s
PW_EXIT=0
```
**All 10 families PASS** (exports, security, runtime, wasm_vm, bytecode_util,
logging, stop_iteration, shared_data, pairs_util, endianness).

### Six-gate summary

| Gate | Result |
|---|---|
| 1 build | clean (EXIT 0) |
| 2 vet | clean (EXIT 0) |
| 3 golangci-lint | clean (EXIT 0) |
| 4 race-short test | all packages ok; 0 FAIL |
| 5 differential | 49/49 PASS (47-dir R1 gate + 0046 + 0047; one `0020` flake re-run GREEN in isolation) |
| 6 h2spec / proxy-wasm | 53/53 + all 10 wasm families PASS |

### Step 2-4: docs advance

- `docs/envoy-go/ROADMAP.md`: sub-row **28.1b** `in-progress → done` + IMPL-DONE note; parent row **28** STAYS `in-progress` (rollup is 28.2's job, D-28.1b-4); sub-row **28.2** STAYS `planned`.
- `docs/envoy-go/STATE.md`: active-phase → `phase 28.1b IMPL done (next = 28.2 SPEC)`; next-skill → `superpowers:brainstorming` (SKILL_ROUTING state 1 — 28.2 sub-phase directory does not yet exist); counts 49/37/337/ADR-0223; last-commit placeholder for the controller squash SHA.
- `next-prompt.txt`: rewritten as the 28.2-SPEC cold-start (the §3.6 synchronization obligation carried as load-bearing pin #1).

### Self-review

- All six gates quoted from LIVE `-count=1` runs. ✓
- Counts 49 / 37 / 337 / ADR-0223 confirmed + quoted. ✓
- ROADMAP: 28.1b done + parent 28 in-progress + 28.2 planned. ✓
- STATE internally consistent (active-phase / next-skill state-1 / counts). ✓
- next-prompt carries the §3.6 synchronization obligation into the 28.2 SPEC pins. ✓
- Only concern: the `0020` differential flake (unrelated HTTP fixture, re-run GREEN in isolation) — recorded honestly, not hidden.
