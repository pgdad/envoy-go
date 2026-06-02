# Phase 28.1b PLAN — the read-side seam + the `zookeeper_proxy` cross-side proof + the 28.1 completion bundle

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`).

**Goal:** Land the symmetric **read-side seam** — a `readChainConn` conn-wrap re-feeding every post-terminal-handoff socket read through the read-filter chain (`chainRuntime.replayRead`), re-based on a monotonic `Buffer.TotalAppended()` counter so the zookeeperproxy decoder feed survives runtime drains — then prove it cross-side (`0046-zookeeper-requests` re-enabled + GREEN on all 7 arms + R4), land `0047-zookeeper-boot-reject`, and close phase 28.1 with the completion bundle (ADR-0221 both-halves + ADR-0222 bodies; BEHAVIOR_CONTRACT 136 → 337; ROADMAP sub-row 28.1b → done).

**Architecture:** The existing `internal/filter/network/` package gains `readconn.go` (the `readChainConn`, mirroring `writeconn.go`/`prefixconn.go`'s embed-and-override-one-method shape), a `chainRuntime.replayRead` post-handoff replay path (append → re-iterate read filters → drain; bounded memory; observational), and a `Buffer.TotalAppended() int64` monotonic counter that the zookeeperproxy decoder's high-water mark re-bases onto. `handleTerminal` installs the `readChainConn` INNERMOST (`writeChainConn(prefixConn(readChainConn(conn)))`) under the SAME `len(writeFilters) > 0` predicate as the writeChainConn — both seams wrap together; every existing production chain (zero write filters) gets NEITHER wrap (R1 byte-identical back-compat). `internal/listener/manager.go`, `tcp_proxy`, HCM, and the zookeeperproxy config/stats/dispatch code are UNTOUCHED.

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses the as-built 28.1a `internal/filter/network/` + `internal/filter/network/zookeeperproxy/` packages, the differential harness (`TCPSink` BackendKind + `fixture.StatsAsserter` + `differential.BootRejectFixture`), and the committed-but-DISABLED `0046` driver. ZERO new third-party dependencies.

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per 28.1b SPEC §10.1)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **10 tasks** / **~80–100 production LoC** (the 26.x accounting basis — production code; fixture drivers + tests excluded):

| Unit | Production LoC | Tasks |
|---|---|---|
| `buffer.go` `TotalAppended` (`total int64` + accessor) | ~8 | 2 |
| zookeeperproxy decoder/filter feed re-base (`decodeOnData` signature + mark widening + `OnData` pass-through) | ~10 | 2 |
| `chain.go` `replayRead` | ~12 | 3 |
| `readconn.go` (the `readChainConn`) | ~35 | 4 |
| `chain.go` `handleTerminal` read-wrap insertion | ~8 | 5 |
| **Total (production basis)** | **~75–100** | **10** |

Both axes trivially under the gate (10 ≤ ~25 tasks; ~100 ≤ ~1500 LoC) → **NO further split.** The `0047` driver (~220 LoC, the `0044` template precedent) and the test surface are excluded per the established 26.x/27/28.1 accounting.

## PLAN-time D-question dispositions (SPEC §8.2)

- **D-S28.1b-1 (the `Buffer.total` field representation) — RESOLVED at PLAN: `int64`.** `Buffer.TotalAppended() int64`; `Append` does `b.total += int64(len(p))`. A very-long-lived connection can exceed 2³¹ bytes on 32-bit platforms; `int64` is costless. **Consequence honored per the spec-reviewer advisory:** the decoder's `chainConsumed` mark widens to `int64` IN LOCKSTEP, and `decodeOnData`'s new parameter is `totalAppended int64` — both widenings land in ONE task (Task 2 below MERGES the SPEC §10 spine's tasks 2 + 3) so no type-mismatch is ever split across commits.
- **D-S28.1b-2 (the race-test shape) — RESOLVED at PLAN: a `chain_test.go` unit test with synthetic filters + `net.Pipe` + live concurrent pump goroutines (Task 6).** NOT a re-use of the 0046 fixture under `-race`: the unit shape keeps the race gate docker-independent and runs on every `go test -race -short ./internal/filter/network/` invocation (per-task + six-gate), not only when the differential harness is available.
- **D-S28.1b-3 (0046 driver banner replacement wording) — stays IMPL-owned (Task 7 step 2):** a one-paragraph "re-enabled at 28.1b (the read seam)" note citing the 28.1b SPEC, replacing the `driver.go:5-24` DISABLED banner.
- **Task ordering vs the SPEC §10 spine** — the spine's task 4 (`readconn.go`) and task 5 (`replayRead` + wrap) are REORDERED + re-cut here into Tasks 3/4/5: `replayRead` lands FIRST (Task 3) because `readChainConn.Read` calls `rt.replayRead` — the spine order would leave Task 4 uncompilable; the `handleTerminal` wrap insertion is its own task (Task 5) because that is where the composition/R1/soundness test surface lives. The SPEC §10 lead-in explicitly permits this merge/split.

---

## File Structure

**Created:**
- `internal/filter/network/readconn.go` — the `readChainConn` (embeds `net.Conn`, overrides `Read` only; mirrors `prefixconn.go:12-28` / `writeconn.go:13-48`).
- `internal/filter/network/readconn_test.go` — passthrough / replay-delivery / replay-before-return / EOF-endStream / non-EOF-error / zero-byte-read unit tests.
- `test/fixtures/0046-zookeeper-requests/README.md` — the fixture README (deferred from 28.1a Task 16 so it documents the as-shipped GREEN result).
- `test/fixtures/0047-zookeeper-boot-reject/driver/driver.go` + `README.md` — the symmetric boot-reject fixture (the `0044` 220-LoC template).
- `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md` — created at Task 1; appended per task.

**Modified:**
- `internal/filter/network/buffer.go` — + `total int64` field + `TotalAppended()` (after `Len`, `buffer.go:21`).
- `internal/filter/network/buffer_test.go` — + TotalAppended monotonicity-under-Drain test.
- `internal/filter/network/chain.go` — + `replayRead` (after `runData`, `chain.go:356`); `handleTerminal` read-wrap insertion (`chain.go:239-267`).
- `internal/filter/network/chain_test.go` — + replay-path tests + composition/R1/soundness-invariant tests + the §3.6 race test.
- `internal/filter/network/zookeeperproxy/decoder.go` — `chainConsumed int → int64` (`decoder.go:39`); `decodeOnData(chainBytes []byte, totalAppended int64)` re-base (`decoder.go:69-73`).
- `internal/filter/network/zookeeperproxy/decoder_test.go` — mechanical signature updates (~30 call sites) + NEW drain-regime tests.
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` — `OnData` passes `buf.TotalAppended()` (`zookeeperproxy.go:66`).
- `internal/filter/network/zookeeperproxy/fuzz_test.go` — mechanical signature updates (2 call sites, `fuzz_test.go:54/:57`). **NOT in the SPEC §3.7 file table — discovered at PLAN authoring; the fuzzer calls the production `decodeOnData` entry point directly.**
- `test/fixtures/0046-zookeeper-requests/driver/driver.go` — the DISABLED banner (`:5-24`) replaced with the re-enabled note; R4 break protocol recorded in comments.
- `test/differential/runner_test.go` — the `0046` blank-import uncommented (`:72-77`); the `0047` blank-import added.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `DECISIONS.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle (Tasks 9–10).

**Untouched (pinned — SPEC §2.4/§2.5/§4):** `internal/listener/manager.go` (28.1b's production diff to `internal/listener/` is ZERO files); `internal/filter/tcpproxy/`; `internal/filter/hcm/`; `internal/filter/network/` `types.go` / `terminal.go` / `callbacks.go` / `registry.go` / `prefixconn.go` / `writeconn.go` / `upstreamcluster.go`; `internal/filter/network/zookeeperproxy/` `config.go` / `stats.go` / `doc.go`; `internal/filter/network/builtins/`; `internal/bootstrap/bootstrap.go`; `internal/stats/name.go` (except the Task-7 R4 TEMPORARY break, fully reverted); `test/differential/fixture/fixture.go`; `test/differential/harness.go`.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:**
- Create: `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md`

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
# fixture dirs on disk (48 expected: 0000..0046; 47 ACTIVE + 0046 disabled):
ls -d test/fixtures/[0-9]* | wc -l            # expect 48
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0046-zookeeper-requests
# the 0046 blank-import is COMMENTED (disabled):
grep -n "0046-zookeeper-requests/driver" test/differential/runner_test.go   # expect ONE commented line (~:77)
# fuzzers (canonical recipe):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l         # expect 37
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3       # tail = ADR-0223 → next-free ADR-0224
```
Expected: fixture dirs **48 on disk** (tail `0046-zookeeper-requests`), of which **47 active** + `0046` committed-but-DISABLED; fuzzers **37**; DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**). 28.1b lands `0046` re-enabled + `0047` → **49 active**, fuzzers STAY 37 (no new fuzzer — SPEC §2.6), and the ADR-0221/0222 BODIES in place (no new ADR number).

- [ ] **Step 2: Re-confirm the stat surface = 136**

Canonical recipe = the BEHAVIOR_CONTRACT.md cumulative "internal names" narrative accounting (the 28.1a Task-1 precedent — do NOT invent a new recipe). The last delta is the phase-26.3 block at `docs/envoy-go/BEHAVIOR_CONTRACT.md:462`: "**Phase 26.3 extension — 132 → 136 internal names**". Expected: **136**. 28.1b rolls 136 → **337** at Task 9 (the +201 zookeeper roster), gated on the Task-7 0046 cross-side proof.

- [ ] **Step 3: Re-confirm the as-built line anchors (drift here re-points later tasks)**

Confirm each SPEC §7.1 anchor still holds at the live IMPL tip (all verified at SPEC tip `4bc9790` / PLAN tip `dc1a89e`; only docs-only commits land between sessions, but the gate catches drift):

| # | File:line | Construct |
|---|-----------|-----------|
| 1 | `internal/filter/network/chain.go:230-232` | `terminalReady` |
| 2 | `internal/filter/network/chain.go:239-267` | `handleTerminal` (prefix drain `:241-245`; write-wrap `:256-262`) — the Task-5 modification site |
| 3 | `internal/filter/network/chain.go:303-310` | `onData` |
| 4 | `internal/filter/network/chain.go:323-356` | `runData` (replayRead inserts AFTER it) |
| 5 | `internal/filter/network/chain.go:366-381` | `onDestroy` |
| 6 | `internal/filter/network/chain.go:146-192` | `chainRuntime` struct (`filters`/`writeFilters`/`buf` fields) |
| 7 | `internal/filter/network/buffer.go:9-31` | `Buffer` (`Append` `:14` / `Bytes` `:18` / `Len` `:21` / `Drain` `:25-31`) — the Task-2 extension site |
| 8 | `internal/filter/network/writeconn.go:13-48` | `writeChainConn` (the symmetric precedent readconn.go mirrors) |
| 9 | `internal/filter/network/prefixconn.go:12-28` | `prefixConn` (`Read` serves prefix WITHOUT delegating — `:21-28`; the §3.1 prefix-not-re-fed basis) |
| 10 | `internal/filter/network/zookeeperproxy/decoder.go:30-53` | `requestDecoder` (`chainConsumed` mark `:34-39`) |
| 11 | `internal/filter/network/zookeeperproxy/decoder.go:69-84` | `decodeOnData` (the Task-2 modification site; the frames loop `:74-84` UNCHANGED) |
| 12 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:65-68` | `OnData` (the Task-2 pass-through site); `:76` the no-op `OnWrite`; `:48-54` the both-directions `filter` struct |
| 13 | `internal/filter/network/zookeeperproxy/fuzz_test.go:54/:57` | the fuzzer's 2 `decodeOnData` call sites (Task-2 mechanical update) |
| 14 | `internal/filter/tcpproxy/filter.go:101-139` | `Handle` (pump A `:136`, pump B `:137`, `wg.Wait()` `:138` — the §3.6 goroutine anchors; READ-ONLY context, file untouched) |
| 15 | `internal/listener/manager.go:1025-1091` | `serveNetworkChain` (handoff-return `:1066-1069`; EOF delivery `:1073-1077`; READ-ONLY context, file untouched) |
| 16 | `test/differential/runner_test.go:72-77` | the commented `0046` blank-import (Task-7 re-enable site) |
| 17 | `test/differential/runner_test.go:832/:845/:1263-1269` | the `TCPSink` backend arm / `acceptSinkCounting` |
| 18 | `test/differential/runner_test.go:1069-1070` | the `StatsAsserter` cross-side dispatch |
| 19 | `test/differential/fixture/fixture.go:70-77/:493-502/:505-508` | `StatsAsserter` / `TCPSink BackendKind = 28` / `BackendKindAware` |
| 20 | `test/differential/harness.go:340-352` | `BootRejectFixture` (the Task-8 interface) |
| 21 | `test/fixtures/0046-zookeeper-requests/driver/driver.go:5-24` | the DISABLED banner (Task-7 replacement site); driver = 881 LoC |
| 22 | `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` | the `0047` template (220 LoC; `BootRejectScript` `:159`; `ExpectedBootErrorSubstring` `:163`) |
| 23 | `internal/stats/name.go:243-255` | the `.zookeeper.` INLINE-PREFIX arm (`const zkSegment = ".zookeeper."` at `:255`) — the Task-7 R4 break-(b) site |
| 24 | `docs/envoy-go/DECISIONS.md:14228/:14235/:14249` | ADR-0221 heading / its 28.1b §AMEND / ADR-0222 heading — the Task-9 body-landing sites |
| 25 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:462/:3642/:3656` | the 26.3 stat-table block / `### Stat surface` / `### Does not yet apply to` ("L4 write filters … deferred") — the Task-9 edit sites |

- [ ] **Step 4: Re-confirm the Task-16 divergence reference column (the Task-7 expected-green table)**

From `docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/PROGRESS.md` (Task 16, "The blocking divergence"): the reference column the seam must reproduce is `connect_rq=2 ping_rq=1 getdata_rq=2 create_rq=1 close_rq=1 create2_rq=1 getchildren2_rq=1 setwatches2_rq=1 decoder_error=1 request_bytes=307`. Record it in PROGRESS.md as the Task-7 acceptance reference.

- [ ] **Step 5: Commit the gate record**

```bash
git add docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 1: baselines/anchors gate — 48 dirs (47 active, 0046 disabled), fuzzers 37, stats 136, DECISIONS tail ADR-0223; as-built anchors re-pinned"
```

---

## Task 2: `Buffer.TotalAppended` (int64) + the decoder-feed re-base (SPEC §3.3; D-S28.1b-1)

> Merges the SPEC §10 spine's tasks 2 + 3 per the spec-reviewer advisory: the `int64` widening lands atomically across `buffer.go` AND the decoder so no type mismatch is split across commits.

**Files:**
- Modify: `internal/filter/network/buffer.go` (+ `total` field + `TotalAppended()`)
- Modify: `internal/filter/network/buffer_test.go`
- Modify: `internal/filter/network/zookeeperproxy/decoder.go:34-39` (mark type), `:64-84` (`decodeOnData`)
- Modify: `internal/filter/network/zookeeperproxy/zookeeperproxy.go:61-68` (`OnData`)
- Modify: `internal/filter/network/zookeeperproxy/decoder_test.go` (mechanical + new drain-regime tests)
- Modify: `internal/filter/network/zookeeperproxy/fuzz_test.go:54/:57` (mechanical)

- [ ] **Step 1: Write the failing Buffer test**

Append to `internal/filter/network/buffer_test.go`:
```go
// TotalAppended is the monotonic count of bytes ever Appended: unlike Len(),
// it is unaffected by Drain — it only grows (the 28.1b §3.3 decoder-feed
// re-base basis: filters that track novelty against TotalAppended are immune
// to WHO drains the buffer and WHEN).
func TestBufferTotalAppendedMonotonicUnderDrain(t *testing.T) {
	b := &Buffer{}
	if b.TotalAppended() != 0 {
		t.Fatalf("zero-value TotalAppended = %d, want 0", b.TotalAppended())
	}
	b.Append([]byte("hello"))
	if b.TotalAppended() != 5 {
		t.Fatalf("after Append(5): TotalAppended = %d, want 5", b.TotalAppended())
	}
	b.Drain(b.Len()) // full drain
	if b.Len() != 0 || b.TotalAppended() != 5 {
		t.Fatalf("after Drain: Len=%d (want 0), TotalAppended=%d (want 5 — Drain must NOT shrink it)", b.Len(), b.TotalAppended())
	}
	b.Append([]byte("-world"))
	if b.TotalAppended() != 11 {
		t.Fatalf("after second Append: TotalAppended = %d, want 11 (5+6, monotonic across the drain)", b.TotalAppended())
	}
	if b.Len() != 6 {
		t.Fatalf("Len = %d, want 6 (only the undrained bytes)", b.Len())
	}
	b.Append(nil) // nil/empty append is a no-op on both counters
	if b.TotalAppended() != 11 || b.Len() != 6 {
		t.Fatalf("after Append(nil): TotalAppended=%d Len=%d, want 11/6", b.TotalAppended(), b.Len())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestBufferTotalAppendedMonotonicUnderDrain -v`
Expected: FAIL — compile error: `b.TotalAppended undefined`.

- [ ] **Step 3: Implement the Buffer extension**

In `internal/filter/network/buffer.go`: add the field to the struct and the accessor; extend `Append`:
```go
type Buffer struct {
	data []byte
	// total is the monotonic count of bytes ever Appended to this Buffer.
	// Unlike Len(), it is unaffected by Drain — it only grows. Filters that
	// need to distinguish never-before-seen bytes from re-delivered bytes (the
	// zookeeperproxy request decoder) track novelty against TotalAppended
	// instead of Len, which makes their tracking immune to WHO drains the
	// buffer and WHEN (the filter never drains — R3; the runtime drains at
	// terminal handoff and after each post-handoff replay pass — 28.1b §3.2/§3.3).
	total int64
}

// Append copies p onto the tail of the buffer.
func (b *Buffer) Append(p []byte) {
	b.data = append(b.data, p...)
	b.total += int64(len(p))
}

// TotalAppended returns the monotonic count of bytes ever Appended (int64 —
// D-S28.1b-1: a very-long-lived connection can exceed 2^31 bytes).
func (b *Buffer) TotalAppended() int64 { return b.total }
```
(`Bytes`/`Len`/`Drain` UNCHANGED.)

- [ ] **Step 4: Run the Buffer tests**

Run: `go test ./internal/filter/network/ -run TestBuffer -v`
Expected: PASS — both `TestBufferDrainSemantics` (existing, unchanged) and the new test.

- [ ] **Step 5: Write the failing decoder drain-regime tests**

Append to `internal/filter/network/zookeeperproxy/decoder_test.go`, reusing the existing helpers IN that file: `newTestDecoder(t)` (returns `(*requestDecoder, *rosterStats, *compiledConfig)` — `decoder_test.go:53`), `counterValue(t, rs, suffix)` (`:64-66`), and the `be32`/`zkFrame`/`dataFrame`/`padTo` frame builders:
```go
// TestDecodeFeedAfterRuntimeDrain proves the §3.3 re-base: after the runtime
// drains the chain buffer (terminal handoff / post-handoff replay), the
// physical chainBytes slice RESTARTS while totalAppended keeps growing — the
// decoder must keep decoding the new tail with no drop and no double-count.
func TestDecodeFeedAfterRuntimeDrain(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ping := zkFrame(be32(pingXid), be32(opPing))
	getdata := dataFrame(1, opGetData, padTo(opGetData))

	// Pre-drain feed: cumulative regime (chainBytes == all bytes ever appended).
	d.decodeOnData(ping, int64(len(ping)))
	// The runtime now drains the chain buffer (handoff or replay-pass drain).
	// Post-drain feed: chainBytes holds ONLY the new bytes; totalAppended is
	// cumulative across the drain.
	d.decodeOnData(getdata, int64(len(ping)+len(getdata)))

	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1 (no double-count across the drain)", got)
	}
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (no drop across the drain)", got)
	}
}

// TestDecodeHandoffBoundarySequence proves the exact handoff regime: cumulative
// feeds pre-handoff, then a drain, then per-replay delta feeds — every frame
// decoded exactly once.
func TestDecodeHandoffBoundarySequence(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	f1 := zkFrame(be32(pingXid), be32(opPing))
	f2 := dataFrame(1, opGetData, padTo(opGetData))
	f3 := dataFrame(2, opGetData, padTo(opGetData))

	// Pre-handoff: two cumulative feeds (the chain buffer accumulates).
	d.decodeOnData(f1, int64(len(f1)))
	cum := append(append([]byte{}, f1...), f2...)
	d.decodeOnData(cum, int64(len(cum)))
	// Handoff: the runtime drains the buffer. Post-handoff replay feeds are
	// per-pass deltas (the replay drains after each pass).
	d.decodeOnData(f3, int64(len(cum)+len(f3)))

	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "getdata_rq"); got != 2 {
		t.Fatalf("getdata_rq = %d, want 2 (f2 pre-handoff + f3 post-handoff, each exactly once)", got)
	}
}

// TestDecodePartialFrameAcrossDrainBoundary: a frame whose bytes arrive split
// across the drain boundary (first half pre-drain cumulative, second half
// post-drain delta) must still reassemble in the decoder-internal readBuf.
func TestDecodePartialFrameAcrossDrainBoundary(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, padTo(opGetData))
	cut := len(frame) / 2

	d.decodeOnData(frame[:cut], int64(cut))        // pre-drain: first half
	d.decodeOnData(frame[cut:], int64(len(frame))) // post-drain: second half only
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (reassembled across the drain boundary)", got)
	}
}
```
(The helper names/signatures above were verified against the as-built `decoder_test.go`; if they have drifted by IMPL time, the Task-1 anchor gate catches it — adapt the helpers, keep the assertions.)

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestDecodeFeedAfterRuntimeDrain|TestDecodeHandoffBoundary|TestDecodePartialFrameAcrossDrain' -v`
Expected: FAIL — compile error: too many arguments to `d.decodeOnData`.

- [ ] **Step 7: Implement the decoder re-base**

In `internal/filter/network/zookeeperproxy/decoder.go`:

1. Widen the mark (`decoder.go:34-39`):
```go
	// chainConsumed is the high-water mark of bytes already fed into readBuf,
	// kept against Buffer.TotalAppended() — NOT against the physical chain-buffer
	// length (28.1b §3.3 re-base; redesigns the 28.1 SPEC §4.5 basis). The
	// never-before-seen bytes are always the trailing (totalAppended −
	// chainConsumed) bytes of chainBytes: bytes are only ever appended at the
	// tail and only ever drained at the head, and the runtime never drains
	// bytes the filters have not yet been shown (the §3.3 soundness invariant).
	// int64 in lockstep with TotalAppended (D-S28.1b-1).
	chainConsumed int64
```

2. Re-base the feed (`decoder.go:64-73`; the frames loop below it UNCHANGED):
```go
// decodeOnData feeds the current chain-buffer contents into the decoder.
// totalAppended is the buffer's monotonic Buffer.TotalAppended() value; the
// high-water mark (chainConsumed) is kept against IT, not against the physical
// buffer length, so the feed is correct regardless of runtime drains (the
// 28.1b read seam drains the buffer at handoff and after every post-handoff
// replay pass). On any never-drained execution TotalAppended() == Len(), so
// this selects byte-for-byte the same slice the 28.1a feed selected (the §3.3
// equivalence — existing assertions unchanged).
func (d *requestDecoder) decodeOnData(chainBytes []byte, totalAppended int64) {
	if newCount := totalAppended - d.chainConsumed; newCount > 0 {
		d.readBuf = append(d.readBuf, chainBytes[int64(len(chainBytes))-newCount:]...)
		d.chainConsumed = totalAppended
	}
	for {
		frame, ok := d.nextFrame()
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeFrame(frame) {
			// decoder_error path already counted + readBuf abandoned.
			return
		}
	}
}
```

3. In `internal/filter/network/zookeeperproxy/zookeeperproxy.go:65-68`, pass the counter through (signature UNCHANGED — only the call body):
```go
// OnData feeds the decoder with the chain-buffer contents + the monotonic
// TotalAppended counter (the 28.1b §3.3 re-based feed — correct under runtime
// drains) and ALWAYS returns Continue. It NEVER drains the chain buffer, never
// closes, never halts (AMEND-A8 unconditional passthrough; R3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnData(buf.Bytes(), buf.TotalAppended())
	return network.Continue
}
```

- [ ] **Step 8: Mechanical test updates (assertions UNCHANGED — the §3.3 equivalence)**

Update every existing `decodeOnData(X)` call site to `decodeOnData(X, int64(len(X)))`:
- `decoder_test.go` — ~30 call sites (lines 97–502). **Cumulative-feed tests** (`TestDecodePartialFrameReassembly:187-193`, `TestDecodeHighWaterMarkNoDoubleCount:208-210`, `TestDecodeOversizedThenRecovers:334-341`, `TestDecodeCorrelationStructuresPopulated:486-502`) already pass cumulative slices — `int64(len(cumulativeSlice))` IS the correct totalAppended for the never-drained regime, so the transformation is uniform. NO assertion changes anywhere (any assertion change indicates a re-base bug — STOP and re-check Step 7).
- `fuzz_test.go:54/:57` — the 2 fuzzer call sites: `d.decodeOnData(data, int64(len(data)))` and `d.decodeOnData(doubled, int64(len(doubled)))` (where `doubled` is the existing append-twice expression, hoisted to a named variable).

- [ ] **Step 9: Run the full package suites**

Run: `go test ./internal/filter/network/... -race -short`
Expected: PASS — all existing decoder/filter/chain tests green with assertions unchanged + the 3 new drain-regime tests green.

Run the fuzzer's seed corpus: `go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperRequestDecode -v`
Expected: PASS (seed-corpus mode; no live fuzzing needed).

- [ ] **Step 10: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/buffer.go internal/filter/network/buffer_test.go \
  internal/filter/network/zookeeperproxy/decoder.go internal/filter/network/zookeeperproxy/decoder_test.go \
  internal/filter/network/zookeeperproxy/zookeeperproxy.go internal/filter/network/zookeeperproxy/fuzz_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 2: Buffer.TotalAppended (int64) + zookeeperproxy decoder feed re-base onto the monotonic counter (SPEC §3.3; D-S28.1b-1=int64)"
```

---

## Task 3: `chainRuntime.replayRead` — the post-handoff replay path (SPEC §3.2)

**Files:**
- Modify: `internal/filter/network/chain.go` (new method after `runData`, `chain.go:356`)
- Test: `internal/filter/network/chain_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `chain_test.go` (white-box, `package network`). First the recording double, then the tests:
```go
// recordingReadFilter records every OnData delivery: the bytes it saw (novel
// tail, tracked via the TotalAppended discipline — the same discipline the
// zookeeperproxy decoder uses) + the endStream flags + a call order tag.
// status controls the per-call return (Status is IGNORED on the replay path —
// the StopIteration variant proves it).
type recordingReadFilter struct {
	Marker
	name      string
	status    Status
	seen      []byte // novel bytes, accumulated via the high-water-mark discipline
	mark      int64  // high-water mark against buf.TotalAppended()
	ends      []bool // endStream flag per OnData call
	order     *[]string
	destroyed int
}

func (f *recordingReadFilter) OnNewConnection() Status { return Continue }
func (f *recordingReadFilter) OnData(buf *Buffer, endStream bool) Status {
	if newCount := buf.TotalAppended() - f.mark; newCount > 0 {
		bs := buf.Bytes()
		f.seen = append(f.seen, bs[int64(len(bs))-newCount:]...)
		f.mark = buf.TotalAppended()
	}
	f.ends = append(f.ends, endStream)
	if f.order != nil {
		*f.order = append(*f.order, f.name)
	}
	return f.status
}
func (f *recordingReadFilter) SetReadFilterCallbacks(ReadFilterCallbacks) {}
func (f *recordingReadFilter) OnDestroy()                                { f.destroyed++ }

var _ ReadFilter = (*recordingReadFilter)(nil)

// replayRead delivers to ALL read filters in CHAIN order (SPEC §3.2 item 1 /
// D-28.1b-5: upstream re-iteration parity).
func TestReplayReadDeliversToAllFiltersInChainOrder(t *testing.T) {
	var order []string
	a := &recordingReadFilter{name: "A", status: Continue, order: &order}
	b := &recordingReadFilter{name: "B", status: Continue, order: &order}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("xyz"), false)
	if string(a.seen) != "xyz" || string(b.seen) != "xyz" {
		t.Fatalf("replay delivery: A saw %q, B saw %q, want xyz/xyz", a.seen, b.seen)
	}
	if len(order) != 2 || order[0] != "A" || order[1] != "B" {
		t.Fatalf("replay order = %v, want [A B] (chain order)", order)
	}
}

// Status is IGNORED on the replay path (SPEC §3.5 row 1 — observational): a
// mid-chain StopIteration does NOT halt delivery to later filters. This is the
// LIVE divergence-from-pre-handoff assertion (§11.1).
func TestReplayReadStatusIgnoredMidChainStop(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: StopIteration}
	b := &recordingReadFilter{name: "B", status: Continue}
	rt := newChainRuntime([]ReadFilter{a, b}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("xyz"), false)
	if string(b.seen) != "xyz" {
		t.Fatalf("filter B saw %q, want xyz (A's StopIteration must be IGNORED on the replay path)", b.seen)
	}
	// And the chain's pre-handoff park state is untouched (no resumeIdx/connHalted side effects).
	if rt.connHalted {
		t.Fatal("replayRead must not set connHalted")
	}
}

// The runtime drains the chain buffer fully after each replay pass (SPEC §3.2
// item 3 — bounded memory).
func TestReplayReadDrainsAfterPass(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{a}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("hello"), false)
	if rt.buf.Len() != 0 {
		t.Fatalf("chain buffer Len = %d after replay pass, want 0 (drain-after-pass)", rt.buf.Len())
	}
	// TotalAppended is monotonic across the drain (the §3.2 item 2 continuity).
	if rt.buf.TotalAppended() != 5 {
		t.Fatalf("TotalAppended = %d, want 5", rt.buf.TotalAppended())
	}
	rt.replayRead([]byte("-more"), false)
	if string(a.seen) != "hello-more" {
		t.Fatalf("filter saw %q across two replay passes, want hello-more (every byte exactly once)", a.seen)
	}
}

// endStream is delivered through the replay (SPEC §3.2 item 4 — the EOF replay).
func TestReplayReadEndStream(t *testing.T) {
	a := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{a}, &fakeConn{}, connFacts{})
	rt.replayRead([]byte("x"), false)
	rt.replayRead(nil, true) // the EOF endStream replay (zero bytes)
	if len(a.ends) != 2 || a.ends[0] != false || a.ends[1] != true {
		t.Fatalf("endStream flags = %v, want [false true]", a.ends)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestReplayRead -v`
Expected: FAIL — compile error: `rt.replayRead undefined`.

- [ ] **Step 3: Implement `replayRead`**

Append to `chain.go` after `runData` (`:356`), the SPEC §3.2 production code VERBATIM:
```go
// replayRead re-iterates the read-filter chain over post-handoff downstream
// bytes (the read-side seam, ADR-0221 §AMEND). It restores upstream
// FilterManagerImpl::onRead parity for wrapped chains: every read filter's
// OnData runs, in chain order, on every socket read for the connection's
// lifetime. The replay is OBSERVATIONAL (28.1b SPEC §3.5): Status is ignored
// (the bytes are already committed to the terminal via readChainConn.Read's
// return), and the buffer is fully drained after the pass (bounded memory —
// the bytes' forward path is the terminal's conn, not the chain buffer).
//
// Called from readChainConn.Read on the terminal's downstream-pump goroutine
// (§3.6 concurrency pin) — never concurrently with the pre-handoff read loop
// (handoff is a happens-before edge: the loop returns before Handle spawns the
// pumps).
func (rt *chainRuntime) replayRead(p []byte, endStream bool) {
	rt.buf.Append(p)
	for _, f := range rt.filters {
		_ = f.OnData(rt.buf, endStream) // Status ignored — observational (§3.5)
	}
	rt.buf.Drain(rt.buf.Len())
}
```

- [ ] **Step 4: Run the tests + the package suite**

Run: `go test ./internal/filter/network/ -run TestReplayRead -v` → PASS (all 4).
Run: `go test ./internal/filter/network/... -race -short` → PASS (replayRead is additive; nothing calls it yet — zero behavioral delta to existing tests).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/chain.go internal/filter/network/chain_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 3: chainRuntime.replayRead — post-handoff observational replay (all filters, chain order, Status ignored, drain-after-pass) (SPEC §3.2)"
```

---

## Task 4: `readconn.go` — the `readChainConn` (SPEC §3.1)

**Files:**
- Create: `internal/filter/network/readconn.go`
- Create: `internal/filter/network/readconn_test.go`

- [ ] **Step 1: Write the failing tests**

> **Test-double note (load-bearing):** the existing `scriptConn` (`chain_test.go:18-41`) is SINGLE-read — it has a `live []byte` field, serves it on the first `Read`, then returns `(0, io.EOF)` forever. The readconn/composition tests need MULTIPLE distinct live reads, so this task defines a new `multiReadConn` double (one payload per Read, then `io.EOF`), embedding `scriptConn` for the no-op `net.Conn` methods + Write capture. It is reused by Tasks 5–6.

Create `internal/filter/network/readconn_test.go` (white-box, `package network`; `recordingReadFilter` comes from Task 3):
```go
package network

import (
	"errors"
	"io"
	"testing"
)

// multiReadConn yields one scripted payload per Read call, then io.EOF forever.
// (The chain_test.go scriptConn is single-read; the readconn tests need multiple
// distinct live reads.) Embeds scriptConn for the no-op net.Conn methods + the
// Write capture; only Read is overridden.
type multiReadConn struct {
	scriptConn
	payloads [][]byte
}

func (c *multiReadConn) Read(b []byte) (int, error) {
	if len(c.payloads) == 0 {
		return 0, io.EOF
	}
	n := copy(b, c.payloads[0])
	c.payloads = c.payloads[1:]
	return n, nil
}

// errConn returns a fixed non-EOF error on every Read.
type errConn struct {
	scriptConn
	err error
}

func (c *errConn) Read(_ []byte) (int, error) { return 0, c.err }

// Live reads pass through unchanged AND are replayed to the read chain BEFORE
// Read returns (replay-before-return — the §5.1 deterministic-scrape ordering).
func TestReadChainConnPassthroughAndReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{[]byte("hello")}}
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	n, err := rc.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("Read = (%q, %v), want (hello, nil) — passthrough", buf[:n], err)
	}
	// Replay-before-return: by the time Read returned, the filter saw the bytes.
	if string(f.seen) != "hello" {
		t.Fatalf("filter saw %q, want hello (replay-before-return)", f.seen)
	}
}

// io.EOF triggers ONE final endStream replay (pre-handoff read-loop symmetry,
// manager.go:1073-1077), in addition to replaying any bytes delivered before it.
func TestReadChainConnEOFEndStreamReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{[]byte("x")}} // one read, then EOF
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	_, _ = rc.Read(buf) // "x"
	n, err := rc.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("second Read = (%d, %v), want (0, io.EOF)", n, err)
	}
	// The filter saw the live bytes (endStream=false) AND a final endStream pass.
	if string(f.seen) != "x" {
		t.Fatalf("filter saw %q, want x", f.seen)
	}
	last := f.ends[len(f.ends)-1]
	if !last {
		t.Fatalf("final replay endStream = %v, want true (the EOF endStream replay)", last)
	}
}

// A non-EOF read error propagates verbatim and does NOT trigger an endStream replay.
func TestReadChainConnNonEOFErrorNoEndStreamReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	myErr := errors.New("reset by peer")
	inner := &errConn{err: myErr}
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	_, err := rc.Read(buf)
	if !errors.Is(err, myErr) {
		t.Fatalf("Read err = %v, want %v (verbatim propagation)", err, myErr)
	}
	for _, e := range f.ends {
		if e {
			t.Fatal("non-EOF error must NOT trigger an endStream replay")
		}
	}
}

// A zero-byte read (n==0, err==nil) is NOT replayed (no empty OnData passes).
func TestReadChainConnZeroByteReadNoReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	rt := newChainRuntime([]ReadFilter{f}, &fakeConn{}, connFacts{})
	inner := &multiReadConn{payloads: [][]byte{{}}} // one zero-byte "read", then EOF
	rc := newReadChainConn(inner, rt)

	buf := make([]byte, 16)
	n, err := rc.Read(buf)
	if n != 0 || err != nil {
		t.Fatalf("zero-byte Read = (%d, %v), want (0, nil)", n, err)
	}
	if len(f.ends) != 0 {
		t.Fatalf("zero-byte read produced %d replay passes, want 0", len(f.ends))
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestReadChainConn -v`
Expected: FAIL — compile error: `newReadChainConn undefined`.

- [ ] **Step 3: Implement `readconn.go`**

Create `internal/filter/network/readconn.go`, the SPEC §3.1 production code VERBATIM:
```go
// internal/filter/network/readconn.go — read-side seam conn for terminal
// handoff (ADR-0221 §AMEND — the read-direction half of the terminal-handoff
// conn-wrap seam). Mirrors prefixconn.go / writeconn.go's embed-and-override-
// one-method shape.

package network

import (
	"errors"
	"io"
	"net"
)

// readChainConn re-feeds every post-handoff downstream socket read through the
// read-filter chain (rt.replayRead) BEFORE returning the bytes to the terminal,
// restoring upstream FilterManagerImpl::onRead re-iteration parity for chains
// that need it (the §3.4 predicate). All non-Read methods promote from the
// embedded net.Conn.
type readChainConn struct {
	net.Conn               // the RAW downstream conn (innermost wrap — §3.1 composition)
	rt       *chainRuntime // the per-connection runtime whose read filters get the replay
}

func newReadChainConn(c net.Conn, rt *chainRuntime) *readChainConn {
	return &readChainConn{Conn: c, rt: rt}
}

// Read reads from the wrapped conn, replays any received bytes through the
// read-filter chain (observational — §3.5), then returns them to the terminal.
// Replay-before-return makes stat increments visible BEFORE the bytes are
// forwarded upstream (deterministic ordering for the 0046 scrape — §5.1).
// io.EOF additionally delivers a final endStream replay (pre-handoff read-loop
// symmetry, manager.go:1073-1077).
func (r *readChainConn) Read(b []byte) (int, error) {
	n, err := r.Conn.Read(b)
	if n > 0 {
		r.rt.replayRead(b[:n], false)
	}
	if err != nil && errors.Is(err, io.EOF) {
		r.rt.replayRead(nil, true)
	}
	return n, err
}
```

- [ ] **Step 4: Run the tests + the package suite**

Run: `go test ./internal/filter/network/ -run TestReadChainConn -v` → PASS (all 4).
Run: `go test ./internal/filter/network/... -race -short` → PASS (nothing constructs a readChainConn in production yet — additive).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/readconn.go internal/filter/network/readconn_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 4: readconn.go — readChainConn (replay-before-return, EOF endStream replay, error passthrough) (SPEC §3.1)"
```

---

## Task 5: `handleTerminal` read-wrap insertion + composition / R1 / soundness-invariant tests (SPEC §3.4 / §3.3)

**Files:**
- Modify: `internal/filter/network/chain.go:239-267` (`handleTerminal`)
- Test: `internal/filter/network/chain_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `chain_test.go`:
```go
// --- Task 5 (28.1b): handleTerminal read-wrap insertion ---

// R1 (BOTH wraps): zero-write-filter chains get NEITHER conn-wrap — the
// terminal receives the raw conn (or prefixConn over the raw conn), never a
// readChainConn and never a writeChainConn. This is the structural back-compat
// guarantee for every existing production chain (SPEC §3.4 item 1).
func TestHandleTerminalZeroWriteFiltersNeitherWrap(t *testing.T) {
	rec := &recordingTerminal{}
	rt := newChainRuntime(nil, &fakeConn{}, connFacts{})
	rt.terminal = rec
	rt.handleTerminal(context.Background())
	if _, isWrap := rec.gotConn.(*writeChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT get a writeChainConn (R1)")
	}
	if _, isWrap := rec.gotConn.(*readChainConn); isWrap {
		t.Fatal("zero-write-filter chain must NOT get a readChainConn (R1 — the SHARED predicate)")
	}
}

// Zero write filters + buffered prefix: prefixConn wraps the RAW conn directly
// (byte-identical to the 26.2/27/28.1a composition — no readChainConn in between).
func TestHandleTerminalZeroWriteFiltersPrefixOverRawConn(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.buf.Append([]byte("prefix"))
	rt.handleTerminal(context.Background())
	pc, ok := rec.gotConn.(*prefixConn)
	if !ok {
		t.Fatalf("terminal conn = %T, want *prefixConn", rec.gotConn)
	}
	if pc.Conn != net.Conn(raw) {
		t.Fatalf("prefixConn wraps %T, want the raw conn (no intermediate wrap for R1 chains)", pc.Conn)
	}
}

// ≥1 write filter + buffered prefix: the FULL composition, innermost → outermost:
// readChainConn(raw) ← prefixConn ← writeChainConn (SPEC §3.1).
func TestHandleTerminalFullCompositionOrder(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.buf.Append([]byte("prefix"))
	rt.handleTerminal(context.Background())

	wc, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("outermost = %T, want *writeChainConn", rec.gotConn)
	}
	pc, ok := wc.Conn.(*prefixConn)
	if !ok {
		t.Fatalf("middle = %T, want *prefixConn", wc.Conn)
	}
	rc, ok := pc.Conn.(*readChainConn)
	if !ok {
		t.Fatalf("inner = %T, want *readChainConn (INNERMOST — prefix bytes bypass the replay)", pc.Conn)
	}
	if rc.Conn != net.Conn(raw) {
		t.Fatalf("readChainConn wraps %T, want the raw conn", rc.Conn)
	}
}

// ≥1 write filter, NO prefix: writeChainConn(readChainConn(raw)).
func TestHandleTerminalCompositionNoPrefix(t *testing.T) {
	rec := &recordingTerminal{}
	raw := &fakeConn{}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	rt := newChainRuntime(nil, raw, connFacts{})
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}
	rt.handleTerminal(context.Background())

	wc, ok := rec.gotConn.(*writeChainConn)
	if !ok {
		t.Fatalf("outermost = %T, want *writeChainConn", rec.gotConn)
	}
	rc, ok := wc.Conn.(*readChainConn)
	if !ok {
		t.Fatalf("inner = %T, want *readChainConn", wc.Conn)
	}
	if rc.Conn != net.Conn(raw) {
		t.Fatalf("readChainConn wraps %T, want the raw conn", rc.Conn)
	}
}

// The prefixConn's buffered prefix is NOT re-fed through the replay: the read
// filters already saw those bytes pre-handoff. Only LIVE post-handoff socket
// reads replay (SPEC §3.1 composition item 1 — the innermost-is-load-bearing test).
func TestHandleTerminalPrefixNotReFedThroughReplay(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	raw := &multiReadConn{payloads: [][]byte{[]byte("LIVE")}} // the Task-4 multi-read double
	rt := newChainRuntime([]ReadFilter{f}, raw, connFacts{})
	rec := &recordingTerminal{}
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}

	// Pre-handoff: the filter sees "pre" via the normal onData path; the bytes
	// stay undrained (passthrough filter) → become the handoff prefix.
	rt.onData([]byte("pre"), false)
	if string(f.seen) != "pre" {
		t.Fatalf("pre-handoff: filter saw %q, want pre", f.seen)
	}

	rt.handleTerminal(context.Background())

	// The terminal reads through the full stack: first the prefix, then live bytes.
	buf := make([]byte, 16)
	n, _ := rec.gotConn.Read(buf)
	if string(buf[:n]) != "pre" {
		t.Fatalf("first terminal read = %q, want the prefix replay", buf[:n])
	}
	// The prefix read must NOT have re-fed the filter (it saw "pre" exactly once).
	if string(f.seen) != "pre" {
		t.Fatalf("after prefix read: filter saw %q, want pre (prefix NOT re-fed)", f.seen)
	}
	n, _ = rec.gotConn.Read(buf)
	if string(buf[:n]) != "LIVE" {
		t.Fatalf("second terminal read = %q, want LIVE", buf[:n])
	}
	// The live read DID replay: the filter has now seen pre + LIVE, each exactly once.
	if string(f.seen) != "preLIVE" {
		t.Fatalf("after live read: filter saw %q, want preLIVE (live bytes replayed exactly once)", f.seen)
	}
}

// The §3.3 soundness invariant, end-to-end: a tracking filter (the TotalAppended
// high-water-mark discipline — exactly the zookeeperproxy decoder's discipline)
// sees EVERY appended byte EXACTLY ONCE across the pre-handoff regime, the
// handoff drain, and the post-handoff replay regime.
func TestChainSoundnessInvariantEveryByteSeenExactlyOnce(t *testing.T) {
	f := &recordingReadFilter{name: "A", status: Continue}
	wf := &fakeWriteFilter{name: "w", status: Continue}
	raw := &multiReadConn{payloads: [][]byte{[]byte("ghi"), []byte("jkl")}} // the Task-4 multi-read double
	rt := newChainRuntime([]ReadFilter{f}, raw, connFacts{})
	rec := &recordingTerminal{}
	rt.terminal = rec
	rt.writeFilters = []WriteFilter{wf}

	// Pre-handoff: two socket reads via the normal onData path.
	rt.onData([]byte("abc"), false)
	rt.onData([]byte("def"), false)
	// Handoff (drains the buffer into the prefix).
	rt.handleTerminal(context.Background())
	// Post-handoff: the terminal drains the prefix, then two live reads (replayed).
	buf := make([]byte, 16)
	for i := 0; i < 3; i++ { // prefix ("abcdef"), "ghi", "jkl"
		_, _ = rec.gotConn.Read(buf)
	}

	if string(f.seen) != "abcdefghijkl" {
		t.Fatalf("tracking filter saw %q, want abcdefghijkl (every appended byte exactly once — no drop, no double-feed)", f.seen)
	}
}
```
Also UPDATE the two existing 28.1a composition tests in place:
- `TestHandleTerminalZeroWriteFiltersUnwrapped` (`chain_test.go:592-600`) — superseded by `TestHandleTerminalZeroWriteFiltersNeitherWrap` above; either DELETE it (the new test is a strict superset) or extend it with the readChainConn assertion. Prefer DELETE + the new name (one test, both wraps).
- `TestHandleTerminalWrapComposition` (`chain_test.go:605-626`) — its `wrap.Conn.(*prefixConn)` assertion still holds; ADD an inner assertion that `prefixConn.Conn` is now a `*readChainConn` (it was the raw conn at 28.1a). The existing "Read through writeChainConn = prefix" assertion stays.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run 'TestHandleTerminal|TestChainSoundness' -v`
Expected: FAIL — `TestHandleTerminalFullCompositionOrder` (and friends) fail: `pc.Conn` is the raw `*fakeConn`, not a `*readChainConn` (the wrap is not yet installed). The R1 tests PASS already (nothing to install for zero-write-filter chains) — that is expected; note it in PROGRESS.

- [ ] **Step 3: Implement the `handleTerminal` read-wrap insertion**

Replace the body of `handleTerminal` (`chain.go:239-267`) with the SPEC §3.4 composition. The diff has TWO load-bearing changes: (1) the new innermost read-wrap; (2) `newPrefixConn(conn, prefix)` now wraps the running `conn` variable (which may be the readChainConn), NOT `rt.conn` directly:
```go
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
	conn := rt.conn
	// Read-side seam (ADR-0221 §AMEND): wrap the RAW conn in a readChainConn —
	// INNERMOST — under the SAME predicate as the writeChainConn below (R1: the
	// two seams wrap together; zero-write-filter chains get NEITHER wrap).
	// Innermost is load-bearing: the prefixConn's buffered prefix (bytes the
	// read filters ALREADY saw pre-handoff) is served by prefixConn.Read WITHOUT
	// delegating inward, so prefix bytes bypass the replay; only LIVE post-handoff
	// socket reads pass through readChainConn.Read (28.1b SPEC §3.1).
	if len(rt.writeFilters) > 0 {
		conn = newReadChainConn(conn, rt)
	}
	if rt.buf.Len() > 0 {
		prefix := make([]byte, rt.buf.Len())
		copy(prefix, rt.buf.Bytes())
		rt.buf.Drain(rt.buf.Len())
		conn = newPrefixConn(conn, prefix)
	}
	// WriteFilter seam (ADR-0221): wrap the conn handed to the terminal in a
	// writeChainConn IFF the chain has ≥1 write filter, so terminal-originated
	// downstream writes run the write chain BEFORE reaching the socket.
	// Composition: writeChainConn OUTER, prefixConn MIDDLE, readChainConn INNER
	// (reads promote: write → prefix replay → live read + replay → raw conn;
	// writes promote: write chain → embedded conns → raw conn). Zero-write-filter
	// chains get NO wrap → byte-identical to the pre-28.1 path (R1 back-compat
	// over all 47 existing fixtures).
	// The dispatch slice is a REVERSED COPY of the chain-order writeFilters
	// (AMEND-A11 LIFO parity: config [A, B, C] ⇒ write dispatch C → B → A).
	if len(rt.writeFilters) > 0 {
		dispatch := make([]WriteFilter, len(rt.writeFilters))
		for i, wf := range rt.writeFilters {
			dispatch[len(rt.writeFilters)-1-i] = wf
		}
		conn = newWriteChainConn(conn, dispatch)
	}
	if rt.upstreamClusterOverride != "" {
		ctx = withUpstreamClusterOverride(ctx, rt.upstreamClusterOverride)
	}
	rt.terminal.Handle(ctx, conn)
}
```
(The doc comment above `handleTerminal` — `chain.go:234-238` — gains one sentence noting the read-side wrap + the §3.1 composition; keep the existing R-M prefix sentence.)

- [ ] **Step 4: Run the full package suite**

Run: `go test ./internal/filter/network/... -race -short -v 2>&1 | tail -40`
Expected: PASS — all new Task-5 tests green AND every pre-existing chain/writeconn/prefixconn/upstreamcluster test green (the updated `TestHandleTerminalWrapComposition` included). Any pre-existing test failure here is an R1 violation — STOP and fix before committing.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/chain.go internal/filter/network/chain_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 5: handleTerminal read-wrap insertion under the shared >=1-write-filter predicate — composition writeChainConn(prefixConn(readChainConn(conn))), R1 + soundness invariant proven (SPEC §3.4/§3.3)"
```

---

## Task 6: The §3.6 concurrency race test (D-S28.1b-2)

**Files:**
- Test: `internal/filter/network/chain_test.go`

- [ ] **Step 1: Write the race test**

Append to `chain_test.go`. The test reproduces the EXACT post-handoff goroutine topology (`tcp_proxy.Handle`, `filter.go:134-138`): goroutine A pumps downstream→upstream through the wrapped conn's `Read` (→ replayRead → OnData), goroutine B pumps upstream→downstream through the wrapped conn's `Write` (→ the write chain's OnWrite), concurrently, over real `net.Pipe` ends:
```go
// pumpingTerminal mirrors tcp_proxy.Handle's goroutine topology (filter.go:134-138):
// two concurrent io.Copy pumps + wg.Wait. It is the §3.6 race-surface double.
type pumpingTerminal struct {
	Marker
	upstream net.Conn
}

func (p *pumpingTerminal) Handle(_ context.Context, downstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(p.upstream, downstream) }() // goroutine A: reads → replay
	go func() { defer wg.Done(); _, _ = io.Copy(downstream, p.upstream) }() // goroutine B: writes → write chain
	wg.Wait()
}

// TestWrappedChainConcurrentPumpsRace drives a wrapped chain (a both-directions
// synthetic filter, the zookeeperproxy shape) under live concurrent pumps over
// net.Pipe, with traffic flowing BOTH directions simultaneously. The assertion
// is `go test -race` itself: the 28.1b race surface must be EMPTY (SPEC §3.6
// item 2 — goroutine A touches rt.buf + the read path; goroutine B's OnWrite
// path shares NO mutable state with it). The test also asserts the filter saw
// every downstream byte (the replay is live under concurrency).
func TestWrappedChainConcurrentPumpsRace(t *testing.T) {
	// Both-directions filter: records read-path bytes via the TotalAppended
	// discipline; OnWrite is a pure no-op Continue (the 28.1 §4.7 pin).
	f := &raceBothFilter{}
	term := &pumpingTerminal{}

	// downstream pipe: testEnd <-> chainEnd (rt.conn); upstream pipe: term <-> backendEnd.
	testEnd, chainEnd := net.Pipe()
	upstreamEnd, backendEnd := net.Pipe()
	term.upstream = upstreamEnd

	crt := NewChainRuntime([]NetworkFilter{f, term}, chainEnd, ConnFacts{})
	crt.OnNewConnection()
	if !crt.TerminalReady() {
		t.Fatal("chain must be terminal-ready after the eager pass (both filter Continues)")
	}

	done := make(chan struct{})
	go func() { crt.HandleTerminal(context.Background()); close(done) }()

	const writes = 50
	var wg sync.WaitGroup
	wg.Add(3)
	// Downstream client: 50 writes toward the chain (goroutine A's input).
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			if _, err := testEnd.Write([]byte("downstream-frame-")); err != nil {
				return
			}
		}
		_ = testEnd.Close() // EOF → pump A exits (+ the EOF endStream replay)
	}()
	// Backend: 50 writes back toward the client (goroutine B's input) + drain reads.
	go func() {
		defer wg.Done()
		for i := 0; i < writes; i++ {
			if _, err := backendEnd.Write([]byte("upstream-frame-")); err != nil {
				return
			}
		}
		_ = backendEnd.Close() // → pump B exits
	}()
	// Client-side reader: drain what the backend pushed through pump B.
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, testEnd)
	}()
	wg.Wait()
	<-done

	// All downstream bytes were replayed to the filter exactly once.
	want := writes * len("downstream-frame-")
	if got := f.readBytes.Load(); got != int64(want) {
		t.Fatalf("filter saw %d downstream bytes via the replay, want %d", got, want)
	}
}

// raceBothFilter is the minimal both-directions double for the race test: the
// read path counts bytes ATOMICALLY (it runs on pump goroutine A); OnWrite is
// a no-op Continue (it runs on pump goroutine B). The two paths share no other
// state — exactly the 28.1b production posture (§3.6 item 2).
type raceBothFilter struct {
	Marker
	readBytes atomic.Int64
	mark      int64 // guarded by being touched ONLY on goroutine A (the §3.6 pin)
}

func (f *raceBothFilter) OnNewConnection() Status { return Continue }
func (f *raceBothFilter) OnData(buf *Buffer, _ bool) Status {
	if newCount := buf.TotalAppended() - f.mark; newCount > 0 {
		f.readBytes.Add(newCount)
		f.mark = buf.TotalAppended()
	}
	return Continue
}
func (f *raceBothFilter) SetReadFilterCallbacks(ReadFilterCallbacks)   {}
func (f *raceBothFilter) OnWrite(*Buffer, bool) Status                 { return Continue }
func (f *raceBothFilter) SetWriteFilterCallbacks(WriteFilterCallbacks) {}
func (f *raceBothFilter) OnDestroy()                                   {}
```
(Add the `sync` / `sync/atomic` / `io` imports to `chain_test.go` if not already present. `net.Pipe` writes block until read — the three driver goroutines + the two pumps ensure no deadlock: every written byte has a reader.)

- [ ] **Step 2: Run under `-race` (the gate IS the assertion)**

Run: `go test ./internal/filter/network/ -run TestWrappedChainConcurrentPumpsRace -race -count=5 -v`
Expected: PASS, ZERO race reports, 5/5 runs. (`-count=5` shakes scheduling nondeterminism.) A race report here is a REAL 28.1b design violation — STOP, do not suppress; re-check that goroutine B's path (writeChainConn.Write → OnWrite no-op) touches neither `rt.buf` nor any decoder state.

- [ ] **Step 3: Run the full package suite under -race**

Run: `go test ./internal/filter/network/... -race -short`
Expected: PASS.

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/chain_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 6: concurrent-pumps race test over net.Pipe — the 28.1b race surface is empty (SPEC §3.6; D-S28.1b-2=unit-shape)"
```

---

## Task 7: `0046-zookeeper-requests` — RE-ENABLE + cross-side GREEN + R4 deliberate-break + fixture README (SPEC §5.1; R4/R8)

> Requires docker (the differential harness boots reference Envoy v1.37.2). The Task-16 reference column (PROGRESS.md Task 1 Step 4) is the expected-green table.

**Files:**
- Modify: `test/differential/runner_test.go:72-77` (uncomment the blank-import)
- Modify: `test/fixtures/0046-zookeeper-requests/driver/driver.go:5-24` (the banner)
- Create: `test/fixtures/0046-zookeeper-requests/README.md`

- [ ] **Step 1: Re-enable the fixture**

In `test/differential/runner_test.go:72-77`: delete the 6-line DISABLED comment block and uncomment the blank-import, restoring the plain import line in the driver import group:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0045-sni-cluster/driver"
	_ "github.com/esalaine/envoy-go/test/fixtures/0046-zookeeper-requests/driver"
	"github.com/esalaine/envoy-go/test/helpers"
```
Run `gofmt -w` + `goimports -w test/differential/runner_test.go` to restore the canonical import-group format (the 28.1a closure introduced the comment gap; re-enabling removes it).

- [ ] **Step 2: Replace the driver's DISABLED banner (D-S28.1b-3)**

In `test/fixtures/0046-zookeeper-requests/driver/driver.go:5-24`: delete the `DISABLED at 28.1a` banner block and replace with a one-paragraph note (final wording IMPL-owned; this is the anticipated shape):
```go
// ============================================================================
// RE-ENABLED at 28.1b — the read-side seam
// ============================================================================
//
// This driver was committed-but-DISABLED at the 28.1a closure (the ADR-0045
// 28.1a/28.1b split): its multi-frame arms (2, 3, 4) require every socket
// read of a connection to reach zookeeper_proxy.OnData, which the 28.1a chain
// runtime did not do post-terminal-handoff. The 28.1b read-side seam
// (readChainConn + chainRuntime.replayRead — 28.1b SPEC §3, ADR-0221 §AMEND)
// re-feeds post-handoff reads through the read chain, restoring reference
// Envoy's forever-re-iteration parity; this fixture is its cross-side proof
// (R8). Re-enabled (runner blank-import restored) at 28.1b Task 7.
```

- [ ] **Step 3: Run the fixture — expect GREEN on all 7 arms**

```bash
go test ./test/differential/ -run 'TestDifferential/0046-zookeeper-requests' -v 2>&1 | tail -40
```
Expected: **PASS.** The subject now matches the Task-16 reference column on all 10 `l_plain` counters: `connect_rq=2 ping_rq=1 getdata_rq=2 create_rq=1 close_rq=1 create2_rq=1 getchildren2_rq=1 setwatches2_rq=1 decoder_error=1 request_bytes=307` + the `l_flags` arm-5 parity + the arm-6 exists-at-zero assertions. Arm-by-arm seam dependency (record in PROGRESS.md): arms 1/5/6 were already green at 28.1a; arms 2/3/4 (multi-socket-read connections) are the seam's proof (R8).

If RED: this is a production bug in Tasks 2–5, NOT a fixture problem (the driver was proven correct + the PoC proven green at Task 16). Use `superpowers:systematic-debugging`; do NOT modify the driver's arms or assertions to make it pass.

- [ ] **Step 4: R4 deliberate-break protocol (on the now-green baseline)**

Per `reference_differential_asserter_dispatch` (prove the assertions are LIVE):

1. **Break (a) — wrong expected value:** temporarily edit the driver's `AssertStats` expectation `{"zk_plain.zookeeper.getdata_rq", 2}` → `3`. Run the fixture. Expected: **FAIL** on both ref + subj sides (`getdata_rq = 2, want 3`). Revert.
2. **Break (b) — name-shape liveness:** temporarily comment out the `.zookeeper.` arm in `internal/stats/name.go:243-255` (the `zkSegment` case). Run the fixture. Expected: **FAIL** — every subject-side lookup reports ABSENT (the subject's Prometheus scrape no longer renders zookeeper counters; envoy-go's `name.go` default branch errors on them). Revert (verify `git diff internal/stats/` is empty after).

Record both break outputs + the reverts honestly in PROGRESS.md and summarize the protocol in the driver's arm-7 comment + the README.

- [ ] **Step 5: Author `test/fixtures/0046-zookeeper-requests/README.md`**

Document (deferred from 28.1a Task 16 so it records the as-shipped GREEN result): the `[zookeeper_proxy, tcp_proxy]` → TCPSink topology + the TCPSink-not-echo rationale; the two listeners / two stat_prefixes (`zk_plain` flag-off / `zk_flags` flag-on); the 7-arm taxonomy with the per-arm expected counters; **the seam dependency** (arms 2/3/4 require the 28.1b read seam — the fixture is the R8 re-iteration proof; cite the 28.1b SPEC §3 + the Task-16 divergence table); the cross-side `request_bytes=307` equality proof; the StatsAsserter mechanics (both-sides `/stats/prometheus`, flat names per AMEND-A4); the R4 deliberate-break record (both breaks + outputs).

- [ ] **Step 6: No-regression spot check**

```bash
go test ./test/differential/ -run 'TestDifferential/(0001-tcp-proxy-rr|0043-network-rbac|0045-sni-cluster)' -v 2>&1 | tail -12
```
Expected: 3/3 PASS (zero-write-filter chains — the R1 spot check before the Task-10 full gate).

- [ ] **Step 7: gofmt + lint + commit**

```bash
gofmt -l test/ ; golangci-lint run ./test/...
git add test/differential/runner_test.go test/fixtures/0046-zookeeper-requests/ \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 7: 0046-zookeeper-requests re-enabled + cross-side GREEN on all 7 arms + R4 deliberate-break recorded + fixture README (SPEC §5.1; R4/R8)"
```

---

## Task 8: `0047-zookeeper-boot-reject` fixture (SPEC §5.2 — the 28.1 SPEC §8.2 design, verbatim)

**Files:**
- Create: `test/fixtures/0047-zookeeper-boot-reject/driver/driver.go`
- Create: `test/fixtures/0047-zookeeper-boot-reject/README.md`
- Modify: `test/differential/runner_test.go` (the `0047` blank-import)

- [ ] **Step 1: Author the driver**

Mirror `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` (220 LoC — the symmetric `BootRejectFixture` template) with the zookeeper substitutions:

- Package doc: the missing-`stat_prefix` PGV-mirror both-sides boot-reject; the `0044` precedent; per `reference_differential_fixture_dispatch_constraint` this dir is the BOOT-REJECT branch only (one dir = one runner branch; the cross-side arms live in `0046`).
- Constants: `fixtureName = "0047-zookeeper-boot-reject"`; `refAdminPort = 9901`; `refZKPort = 15049` (next free: `0044`=15046, `0046`=15047/15048); `expectedBootErrorSubstr = "stat_prefix"`.
- `init()` → `fixture.RegisterFixture(fixtureName, &zkBootRejectDriver{})`.
- `fixture.Driver` methods: `BackendCount()=1`, `SubjectListenerName()="l_zk"`, `ReferenceListenerPort()=refZKPort`, `ReferenceBootstrap`/`SubjectConfig` → `renderBootRejectBootstrap(...)`, no-op `DriveReference`/`DriveSubject`, `ProbeAdmin` via `helpers.HTTPGetReadyRaw` (the `0044:143-153` shape).
- `harness.BootRejectFixture`: `BootRejectScript() = ""`; `ExpectedBootErrorSubstring() = expectedBootErrorSubstr`.
- The bootstrap (same chain shape as `0046`'s `l_plain` MINUS the `stat_prefix` field):
```go
func renderBootRejectBootstrap(adminPort, listenerPort int) string {
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }

static_resources:
  listeners:
    - name: l_zk
      address: { socket_address: { address: 0.0.0.0, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.zookeeper_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy
                # stat_prefix INTENTIONALLY OMITTED — PGV min_len=1 violation
                # triggers the stat_prefix-required PARSE-REJECT on both sides.
                max_packet_bytes: 1048576
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_unused

  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1 }
`, adminPort, listenerPort)
}
```
(The minimal `c_unused` cluster satisfies the zero-cluster boot reject — `reference_network_filter_typeurl_extensions`; the `@type` carries the `extensions.` segment. envoy-go's reject wording is the 28.1a Task-7 byte-stable `zookeeper_proxy: stat_prefix is required` → contains `stat_prefix`; the reference's PGV error names the field + echoes the bootstrap → also contains `stat_prefix` — the `0044` substring precedent, same PGV violation class. IMPL note: verify the reference's actual stderr on the first run; if `stat_prefix` is unexpectedly absent, choose the longest common case-sensitive substring and record the finding honestly, per the `0044` package-doc precedent.)
- Compile-time assertion: `var _ fixture.Driver = (*zkBootRejectDriver)(nil)`.

- [ ] **Step 2: Add the runner blank-import**

In `test/differential/runner_test.go`, after the `0046` line:
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0047-zookeeper-boot-reject/driver"
```

- [ ] **Step 3: Run the fixture**

```bash
go test ./test/differential/ -run 'TestDifferential/0047-zookeeper-boot-reject' -v 2>&1 | tail -20
```
Expected: PASS — BOTH sides reject at boot; both stderrs contain `stat_prefix`.

- [ ] **Step 4: Author the README**

Document: the missing-`stat_prefix` PGV-mirror arm (the load-bearing `0047` arm); the symmetric-reject discipline (`BootRejectFixture`, not `SubjectOnlyBootRejectFixture`); the common-substring choice + the per-side load-bearing analysis (the `0044` README shape); the boot-reject-vs-cross-side dir split (`reference_differential_fixture_dispatch_constraint`); the latency PGV-mirror arms' disposition (unit-test-only at 28.1; fixture disposition is the 28.2 SPEC's).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/ ; golangci-lint run ./test/...
git add test/fixtures/0047-zookeeper-boot-reject/ test/differential/runner_test.go \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 8: 0047-zookeeper-boot-reject symmetric PGV-mirror fixture (SPEC §5.2; the 0044 template)"
```

---

## Task 9: Completion bundle part 1 — ADR-0221 (both seams) + ADR-0222 bodies + the BEHAVIOR_CONTRACT 28.1 bundle (SPEC §6.1/§6.2)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0221 + ADR-0222 §Decision/§Consequences bodies — IN PLACE per ADR-0044; tail STAYS ADR-0223)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: ADR-0221 §Decision + §Consequences body (after its §Context, `DECISIONS.md:~14237-14246`)**

Land IN PLACE (ADR-0044 discipline; NO new ADR number — D-28.1b-3). The body covers **BOTH halves of the terminal-handoff conn-wrap seam**:

- **§Decision — the write half (28.1a-landed facts):** the `WriteFilter`/`WriteFilterCallbacks` interfaces (`OnWrite(buf, endStream) Status`; `OnDestroy` on the interface; minimal callbacks = `Connection()` only); the independent-type-asserts classification (read / write / both / terminal; a both-directions filter is the SAME instance in both sets; dual callback injection; once-per-instance OnDestroy dedupe); the REVERSE-chain-order write dispatch (AMEND-A11 LIFO); the `writeChainConn` + D-P7 return semantics (StopIteration → `(len(p), nil)`); the write-only-filter boot boundary (manager.go untouched — write-only filters stay boot-rejected); the terminal-originated-writes-only boundary (a ReadFilter's `Connection().Write` bypasses the write chain).
- **§Decision — the read half (28.1b; this SPEC §3):** the `readChainConn` (innermost wrap; replay-before-return; EOF endStream replay); `chainRuntime.replayRead` (all read filters, chain order, Status ignored, drain-after-pass); the `Buffer.TotalAppended` (int64) decoder-feed re-base (the §3.3 redesign of the 28.1 SPEC §4.5 mark basis + the soundness invariant); the SHARED `len(writeFilters) > 0` wrap predicate (R1: both wraps install together; zero-write-filter chains get neither); the composition `writeChainConn(prefixConn(readChainConn(conn)))`.
- **§Consequences:** CONSUMES the ADR-0213 §Decision item 8 API-revision allowance (consumer #1 `zookeeper_proxy`; anticipated #2 `mongo_proxy`); the three observational post-handoff boundaries (§3.5: OnData Status ignored / Close not acted on / ContinueReading meaningless — each deferred to the first consumer needing it under the same allowance); the §3.4 future-generalization note (a read-decoding non-write filter would need an opt-in marker interface — no such filter exists or is planned); the hot-path cost note (wrapped chains only; zero cost for the R1 population); **the §3.6 concurrency pins + THE 28.2 FORWARD-POINTER** (goroutine A = read replay path, goroutine B = OnWrite path; the 28.1b race surface is empty; **the 28.2 response decoder in OnWrite WILL race the replay-path request decoder on the correlation structures → ADR-0223 / the 28.2 SPEC MUST add synchronization — anticipated shape: a per-connection `sync.Mutex` on the decoder guarding the correlation maps**).

- [ ] **Step 2: ADR-0222 §Decision + §Consequences body (after its §Context, `DECISIONS.md:~14258-14266`)**

Land IN PLACE per its existing §Context draft (the request package — unchanged scope), with the §3.3 re-base note: TypeURL via `proto.MessageName`; `NewFactory(reg)` closure-capture; the 9-field parse + PGV-mirror PARSE-REJECT arms + proto→wire opcode mapping; the 201-counter eager roster + creation parity (D-P5) + the dynamic `auth.<scheme>_rq` counters; the shallow decoder (framing + xid sniffing + D-S28.1-1 min-length table + AMEND-A8 no-resync) **+ the 28.1b §3.3 TotalAppended feed re-base** (the high-water mark is kept against the monotonic appended-bytes counter, not the physical buffer length — D-S28.1-3 ownership unchanged); the correlation structures (written 28.1, consumed 28.2 — R5); the `.zookeeper.` name.go arm (AMEND-A4 flat Prometheus); the both-directions filter glue + pure no-op OnWrite; the fixtures (`0046` cross-side StatsAsserter + `0047` boot-reject) + the TCPSink pin + the 37th fuzzer; the AMEND-A9 dynamic-metadata deferral + the shallow-decode leniency departure.

Verify after both edits: `grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1` → still `ADR-0223` (no new number).

- [ ] **Step 3: The BEHAVIOR_CONTRACT 28.1 bundle (ONE atomic edit per ADR-0052)**

Four edits to `docs/envoy-go/BEHAVIOR_CONTRACT.md`:

1. **The stat-table roll 136 → 337** (in `## Stat-name mapping`, after the phase-26.3 block at `:462`): a new block `**Phase 28.1 extension — 136 → 337 internal names:**` — the 201 `zookeeper_proxy` counters (the `<stat_prefix>.zookeeper.` eager roster; the upstream `ALL_ZOOKEEPER_PROXY_STATS` macro mirror; AMEND-A1/A2/A3 asymmetries: `connect_readonly_rq*` rq-side-only, NO static `auth_rq` [dynamic per-scheme `auth.<scheme>_rq` instead], `auth_resp*` present; the four `enable_*` flags gate INCREMENTS never creation — flag-false counters exist at 0 forever; response-side counters exist-at-zero until 28.2). Note explicitly: the roll lands at 28.1b (not 28.1a) because the BEHAVIOR_CONTRACT records cross-side-PROVEN surface and the proof is the now-green `0046` (the deliberate 28.1a deferral). Per-row enumeration of 201 rows is NOT required — the roster is normatively defined by `internal/filter/network/zookeeperproxy/stats.go` `rosterSuffixes()` + the R2 golden-list unit test; the block records the families + the count (the 25.x large-roster precedent).
2. **NEW `### Network filter chain framework — terminal-handoff conn-wrap seam (28.1 amendment)` block** (in the network-filter framework section, after the 27-amendment block at `:3625`): BOTH directions — the write-side items (REVERSE dispatch, StopIteration-no-forward documented-unsupported, terminal-originated-writes-only, write-only-filter boot boundary, the `writeChainConn`) PLUS the read-side items (the replay semantics §3.2; the shared wrap predicate + R1 §3.4; **the three observational boundaries as a table** — post-handoff OnData-Status ignored / Close not acted on / ContinueReading meaningless, each with its "why acceptable" rationale per SPEC §3.5; the goroutine-placement note + the 28.2 synchronization forward-pointer §3.6; the TotalAppended soundness invariant §3.3).
3. **NEW `### envoy.filters.network.zookeeper_proxy` subsection** (after the `### envoy.filters.network.sni_cluster` subsection at `:3611`): request-side semantics; the 201-counter roster + creation parity; the `<stat_prefix>.zookeeper.` scope; the Prometheus flat rendering (AMEND-A4 — no labels); the dynamic auth counters; the shallow-decode leniency departure (payload-malformed → `<op>_rq` not `decoder_error`); the dynamic-metadata coverage boundary (AMEND-A9); the `access_log` parse-accept-ignore note; the parsed-not-consumed latency-field note; **the re-iteration guarantee (R8): every frame of every connection is decoded regardless of how many socket reads deliver it** — now writable as cross-side-PROVEN facts (0046 green).
4. **Update `### Stat surface` (`:3642`) + `### Does not yet apply to` (`:3656`):** stat surface narrative gains the 28.1 sentence (136 → 337; fixtures 47 → 49; fuzzers 37); the "L4 write filters (`WriteFilter` / `onWrite`) — deferred with API-revision allowance (ADR-0213)" bullet is REPLACED by "Response-side decode + latency counters (`zookeeper_proxy` OnWrite) — 28.2 / ADR-0223" + a new bullet for the §3.5 post-handoff observational boundaries; the `### Applies to` list gains "Phase 28.1 onward (the terminal-handoff conn-wrap seam, both directions; `zookeeper_proxy` request side)".

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 9: completion bundle — ADR-0221 (both-seams body) + ADR-0222 bodies in place; BEHAVIOR_CONTRACT conn-wrap-seam block + zookeeper_proxy subsection + stat table 136->337 [ADR-0221,ADR-0222]"
```

---

## Task 10: Six-gate + STATE.md + ROADMAP sub-row advance + next-prompt.txt (SPEC §6.3/§11.2)

**Files:**
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`
- Modify: `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md`

- [ ] **Step 1: The six-gate (SPEC §11.2) — run LIVE, quote into PROGRESS.md**

```bash
go build ./...                       # gate 1: clean
go vet ./...                         # gate 2: clean
golangci-lint run                    # gate 3: clean (whole repo)
go test ./... -race -short           # gate 4: green (all packages)
# gate 5: FULL differential suite — 49 active dirs (incl. the 47-dir R1 back-compat gate)
go test ./test/differential/ -run TestDifferential -v 2>&1 | tail -80
# gate 6: h2spec 53/53 + proxy-wasm 10/10 (asserted-unaffected — the seam is network-chain-only,
#   HCM's chain has zero write filters so it is never wrapped — re-run LIVE since the harness is available)
go test ./test/conformance/h2spec/ -run TestH2Spec -v
go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v
```
Expected: gates 1–4 clean/green; gate 5 **49/49 PASS** (47 pre-existing R1 + `0046` + `0047`); gate 6 **53/53** + **all families PASS**. All outputs quoted honestly into PROGRESS.md (per `superpowers:verification-before-completion`), including any `freeTCPPort` TOCTOU bind flakes — re-run flaked dirs in isolation and record both runs (the 28.1a-closure precedent; a flake is NOT a regression, but it must be recorded, not hidden).

Confirm + quote the counts: fixture dirs **49 active** (tail `0047-zookeeper-boot-reject`); fuzzers **37** (unchanged); stat table **337**; DECISIONS tail **ADR-0223** / next-free **ADR-0224** (both unchanged).

- [ ] **Step 2: ROADMAP.md sub-row advance (D-28.1b-4 — sub-row ONLY)**

- Sub-row **28.1b**: `in-progress → done` + append the IMPL-DONE note (the read seam landed [readChainConn + replayRead + TotalAppended re-base + shared R1 predicate]; 0046 re-enabled + green + R4; 0047 landed; counts 47→49 active fixtures / 37 fuzzers / 136→337 stats; ADR-0221 both-halves + ADR-0222 bodies landed; the 28.2 synchronization forward-pointer pinned).
- **Parent row 28 STAYS `in-progress`** (the rollup is 28.2's — the 18/19/22/24/25/26 final-sub-phase precedent).
- Sub-row 28.2 STAYS `planned`.

- [ ] **Step 3: STATE.md advance**

- `active-phase`: → `phase 28.1b IMPL done (next = 28.2 SPEC)` + the summary paragraph (what 28.1b landed; the 28.1 family is now complete except the 28.2 response side).
- `phase-directory`: 28.1b dir now holds README/SPEC/PLAN/PROGRESS (+ REVIEW.md if the review stage produced one); the 28.2 sub-phase directory is created at the 28.2 SPEC session.
- `lifecycle-state`: SKILL_ROUTING state 1-for-28.2 (sub-phase directory does not exist → `superpowers:brainstorming` scoped to the 28.2 SPEC, per the 25.x/26.x per-sub-phase precedent).
- `next-skill`: `superpowers:brainstorming` (the 28.2 SPEC — response decoder in OnWrite + xid correlation consumption + latency-threshold counters + `0048-zookeeper-responses` + the parent-row-28 rollup; **MUST carry the 28.1b SPEC §3.6 correlation-structure synchronization obligation into ADR-0223 / the 28.2 SPEC**).
- `last-commit`: filled at squash (the controller fills the squash SHA post-merge).
- Counts: fixtures **49** (tail `0047`), fuzzers **37**, stats **337**, DECISIONS tail **ADR-0223**, next-free **ADR-0224**.

- [ ] **Step 4: next-prompt.txt rewrite (the 28.2-SPEC cold-start)**

Rewrite for the next session: phase 28.1b DONE + squash-merged + pushed; active phase 28 (parent in-progress; 28.1a done, 28.1b done, 28.2 planned); the session authors the 28.2 SPEC per SKILL_ROUTING state 1 (sub-phase directory creation + `superpowers:brainstorming`); the read-first list (STATE.md, the parent 28 SPEC §11 + BRAINSTORM, the 28.1/28.1b SPECs + PROGRESS for the as-built surface, DECISIONS ADR-0221/0222/0223, the as-built zookeeperproxy package); the load-bearing pins the 28.2 SPEC must carry (the §3.6 synchronization obligation [a per-connection mutex on the correlation structures — goroutine B's response decoder vs goroutine A's request decoder]; the deterministic-threshold differential discipline; the parent-row-28 rollup at 28.2 phase-done; the correlation-consumption R5 ratification; counts at 28.2 open: 49 fixtures / 37 fuzzers / 337 stats / tail ADR-0223 / next-free ADR-0224).

- [ ] **Step 5: Commit + the stage-close handoff**

```bash
git add docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt \
  docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PROGRESS.md
git commit -m "phase 28.1b Task 10: six gates GREEN LIVE (49/49 differential incl. 47-dir R1 gate; h2spec 53/53; proxy-wasm 10/10); ROADMAP 28.1b in-progress->done (parent 28 stays); STATE + next-prompt for the 28.2-SPEC cold-start [ADR-0221,ADR-0222]"
```

**Controller stage-close (NOT a subagent step):** per `feedback_push_to_origin` + the project squash discipline — squash-merge the worktree branch to master with the phase commit message format `phase 28.1b: <title> [ADR-0221,ADR-0222]`, push to origin, fill the squash SHA into STATE.md `last-commit` (+1 docs commit), repoint next-prompt.txt's master-tip reference (+1 docs commit), push again.

---

## Test surface summary (SPEC §11.1)

- **Layer A — framework unit** (`internal/filter/network/`): `Buffer.TotalAppended` monotonicity under Append/Drain (Task 2); `replayRead` all-filters chain-order / Status-ignored-mid-chain-stop LIVE / drain-after-pass + TotalAppended continuity / endStream (Task 3); `readChainConn` passthrough+replay-before-return / EOF endStream replay / non-EOF error propagation / zero-byte no-replay (Task 4); `handleTerminal` R1 NEITHER-wrap / prefix-over-raw-conn (R1+prefix) / full composition order / no-prefix composition / prefix-not-re-fed / the §3.3 soundness invariant end-to-end (Task 5); the §3.6 concurrent-pumps race test over `net.Pipe` (Task 6).
- **Layer A — zookeeperproxy unit**: the mechanical `decodeOnData` signature updates with assertions UNCHANGED (the §3.3 equivalence — Task 2); NEW drain-regime tests (feed-after-drain / handoff-boundary sequence / partial-frame-across-drain — Task 2); the existing multi-read/partial-frame/garbage/correlation tests re-pass (Task 2).
- **Layer C — fuzz**: `FuzzZookeeperRequestDecode` re-passes with the re-based signature (Task 2; count STAYS 37 — no new fuzzer, SPEC §2.6).
- **Layer D — differential**: `0046` re-enabled, 7 arms cross-side GREEN + R4 deliberate-break (Task 7); `0047` symmetric boot-reject (Task 8); the FULL 47-dir back-compat gate (R1) → **49/49** (Task 10).
- **Layer E — race**: `go test -race -short ./internal/filter/network/...` per task + the dedicated Task-6 race test + the repo-wide gate-4 run (Task 10).

## Acceptance checklist (SPEC §11.3 — verified at Task 10)

- [ ] The read-side seam lands per SPEC §3 (`readconn.go` + `replayRead` + the TotalAppended re-base + the shared wrap predicate); `manager.go`/`tcp_proxy`/HCM untouched (zero `internal/listener/` diff); all 47 pre-existing fixtures byte-exact green (R1).
- [ ] `0046-zookeeper-requests` re-enabled + GREEN on all 7 arms + the R4 deliberate-break recorded + the fixture README authored (R4/R8).
- [ ] `0047-zookeeper-boot-reject` lands and is green; counts: **49 active** fixture dirs (tail `0047`), **37** fuzzers, stat table **337** (R6).
- [ ] ADR-0221 (both-seams body, incl. the 28.2 synchronization forward-pointer) + ADR-0222 §Decision/§Consequences bodies in place; DECISIONS.md tail STAYS **ADR-0223** (no new number); the BEHAVIOR_CONTRACT 28.1 bundle lands (§6.2) incl. the §3.5 observational boundaries documented (NOT silent).
- [ ] Six gates green LIVE + quoted into PROGRESS.md; STATE.md advanced; ROADMAP sub-row 28.1b `in-progress → done`; parent row 28 STAYS `in-progress`; 28.2 STAYS `planned`; next-prompt.txt rewritten for the 28.2-SPEC cold-start.
