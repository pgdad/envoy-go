# Phase 28.2 PLAN — the `zookeeper_proxy` response decoder + latency-threshold counters + the phase-28 rollup

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`). Differential runs use `-count=1` (`reference_differential_break_protocol_count1`).

**Goal:** Complete `zookeeper_proxy`'s round-trip observability — the response decoder in `OnWrite` consuming the 28.1-laid correlation structures (R5), the per-opcode `*_resp`/`watch_event`/`response_bytes` counters, the per-connection mutex (§3.6 — discharging the ADR-0221 forward-pointer), the latency fast/slow counter surface — prove it cross-side (`0048-zookeeper-responses` over a NEW `TCPZKResponder` backend), and close phase 28 (ADR-0223 body; BEHAVIOR_CONTRACT 28.2 bundle; the ATOMIC parent-row-28 ROLLUP; the six-gate).

**Architecture:** The as-built `internal/filter/network/zookeeperproxy/` per-connection `requestDecoder` is renamed `decoder` (it now decodes BOTH directions, mirroring upstream's single `DecoderImpl`) and gains: a write-side reassembly buffer (`writeBuf`, fed by `OnWrite` — NO write-side `TotalAppended` machinery, since `writeChainConn.Write` allocates a fresh per-Write `Buffer`), the response dispatch (leading-int32 sniffing: connect / watch / control / data / unknown), correlation consumption (data-map erase-on-lookup + control FIFO pop, copied out under a **per-connection `sync.Mutex`** guarding EXACTLY the two correlation maps), and the latency fast/slow accounting (`<=` INCLUSIVE; wire-opcode-keyed overrides). `OnWrite` replaces its 28.1 no-op body and ALWAYS returns `Continue` (R3). The framework (`chain.go`/`readconn.go`/`writeconn.go`/`buffer.go`), `manager.go`, `tcp_proxy`, HCM, and the zookeeperproxy `config.go`/`stats.go` are UNTOUCHED (SPEC §2.4 — all config fields + all 201 counters already exist).

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Reuses the as-built 28.1a/28.1b `internal/filter/network/` + `zookeeperproxy/` packages and the differential harness (`fixture.StatsAsserter` + a NEW `TCPZKResponder` BackendKind). ZERO new third-party dependencies.

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per 28.2 SPEC §11.1)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **11 tasks** / **~360–490 production LoC** (the 26.x accounting basis — production code; fixture drivers + tests excluded):

| Unit | Production LoC | Tasks |
|---|---|---|
| `decoder.go` rename + `writeBuf`/`mu` fields + request-path locking | ~25–35 | 2 |
| `decoder.go` write-side reassembly + framing + uncorrelated dispatch (`decodeOnWrite`/`nextWriteFrame`/`responseError`/`onWatchEvent`) | ~70–90 | 3 |
| `decoder.go` correlated dispatch + correlation consumption + byte accounting (`popControl`/`takeData`/`onConnectResponse`/`onControlResponse`/`onDataResponse`/`countResponse`/`respOpname`) | ~110–150 | 4 |
| `decoder.go` latency-threshold counters (`recordLatency` + 3 call sites) | ~25–35 | 5 |
| `zookeeperproxy.go` `OnWrite` glue | ~10 | 6 |
| `fixture.go` `TCPZKResponder` BackendKind | ~15–20 | 8 |
| `runner_test.go` responder backend arm (`acceptZKResponder` + frame helpers) | ~100–130 | 8 |
| **Total (production basis)** | **~355–470** | **11** |

Both axes comfortably under the gate (11 ≤ ~25 tasks; ~470 ≤ ~1500 LoC) → **NO split.** The `0048` driver (~800–950 LoC, the `0046` 875-LoC template) and the test surface are excluded per the established 26.x/27/28.1 accounting. (The parent SPEC §11.9 estimated 28.2 at ~600–900 production LoC including driver-adjacent surfaces — fits either way.)

## PLAN-time D-question dispositions (SPEC §9.2)

- **D-S28.2-2 (responder trigger encoding + delay constant) — RESOLVED at PLAN.**
  - **Wrong-xid trigger opcode = `getacl` (wire op 6).** The responder answers a getacl request with `xid + 1000` instead of the echoed xid → uncorrelated on both sides → `decoder_error`. getacl appears in EXACTLY ONE arm (arm 3) — no other arm expects a correlated getacl response.
  - **Watch-event-push trigger opcode = `exists` (wire op 3).** The responder writes the standard correlated response, THEN an unsolicited watch-event frame (xid −1). exists appears in EXACTLY ONE arm (arm 2). (Semantically faithful: a real ZooKeeper `exists` call sets a watch.)
  - **Delay constant = `zkResponderDelay = 10 * time.Millisecond`,** applied before EVERY response write (triggers inherit it). Rationale: 10× the 1 ms slow-arm threshold (deterministic margin on both sides under scheduler jitter), and ~14 round-trips × 10 ms ≈ 140 ms of fixture wall-clock — negligible.
- **D-S28.2-1 (special-framing min-lengths) — stays IMPL-owned:** Task 3 Step 1 (watch event = 16) and Task 4 Step 1 (connect response = 20 + password) each verify their pinned minimum against upstream `decoder.cc` (`parseWatchEvent` / `parseConnectResponse` ensureMinLength calls, tag v1.37.2 via raw.githubusercontent.com) before writing tests; adjust + record in PROGRESS.md if upstream differs.
- **D-S28.2-3 (0048 ports) — anticipated 15050–15053;** re-pinned at Task 1 against the live fixture roster.
- **D-S28.2-4 (frame-scanner extraction) — RESOLVED at PLAN: parallel methods** (`nextFrame` / `nextWriteFrame`). The two differ in buffer field AND error path (`decoderError` abandons `readBuf`; `responseError` abandons `writeBuf`) — a shared helper parameterized by buffer pointer + error func reads worse than 18 duplicated lines. The IMPL may still extract if it disagrees (SPEC: either acceptable).
- **D-S28.2-5 (zxid/error disposition) — stays IMPL-owned (Task 4 Step 1):** confirm upstream emits no counter keyed on the response error value (parent §11.4: error only feeds dynamic metadata, which is deferred). The plan's design reads past zxid(8)+error(4) for min-length only.
- **PLAN-discovered refinement 1 (the write-side error path).** SPEC §8.1 says the response path "reuses" `decoderError` — the COUNTING pattern is reused, but the abandon target must be `writeBuf` (not `readBuf`). The plan lands a separate `responseError(opname)` method (Task 3). NOT a SPEC deviation; a disambiguation.
- **PLAN-discovered refinement 2 (correlate-then-validate order).** Upstream `decodeOnWrite` fetches/erases the pending request BEFORE validating the zxid+error tail — so a truncated-but-correlatable response still consumes the entry, and the flag-gated `<opname>_decoder_error` fires with the opname from the correlation hit (exactly the SPEC §3.3 "when the opname is known from a correlation hit" clause). Tasks 4's handlers correlate FIRST, validate SECOND.
- **PLAN-discovered refinement 3 (testable latency signature).** `recordLatency(respOpname string, wireOpcode int32, latency time.Duration)` takes the MEASURED latency as a parameter (the caller computes `time.Since(entry.start)`), so the unit boundary tests inject exact durations (`latency == threshold` → the inclusive edge) without clock manipulation.
- **Task-spine re-cut (permitted by SPEC §11 lead-in).** The SPEC spine's task 3 (response framing + dispatch, ALL rows) and task 4 (correlation + bytes) are re-cut along the correlation boundary: PLAN Task 3 = write-side reassembly + framing + the UNCORRELATED rows (watch / unknown / short / oversized); PLAN Task 4 = the CORRELATED rows (connect / control / data) + consumption + byte accounting. Reason: the correlated rows cannot be implemented or tested without the §3.4 consumption semantics — the SPEC's cut would force Task 3 to land untestable stubs.

---

## File Structure

**Created:**
- `test/fixtures/0048-zookeeper-responses/driver/driver.go` — the 4-listener / 8-arm cross-side driver (the `0046` 875-LoC template + round-trip reads).
- `test/fixtures/0048-zookeeper-responses/README.md` — the fixture README (topology, arm taxonomy, R4 record).
- `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md` — created at Task 1; appended per task.

**Modified:**
- `internal/filter/network/zookeeperproxy/decoder.go` — the rename (`requestDecoder` → `decoder`, `newRequestDecoder` → `newDecoder`); + `writeBuf`/`mu` fields; request-path lock acquisition; `decodeOnWrite`/`nextWriteFrame`/`responseError`/`onWatchEvent` (Task 3); `popControl`/`takeData`/`onConnectResponse`/`onControlResponse`/`onDataResponse`/`countResponse`/`respOpname` (Task 4); `recordLatency` (Task 5).
- `internal/filter/network/zookeeperproxy/decoder_test.go` — mechanical rename updates; response frame builders + response-path unit tests (Tasks 3–5); the §3.6 race test (Task 6).
- `internal/filter/network/zookeeperproxy/zookeeperproxy.go` — the `OnWrite` body (Task 6); doc-comment updates.
- `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go` — `TestFilterOnWritePureNoOp` replaced by the OnWrite-feeds-decoder tests (Task 6).
- `internal/filter/network/zookeeperproxy/fuzz_test.go` — mechanical rename (1 site); the 38th fuzzer (Task 7).
- `test/differential/fixture/fixture.go` — `TCPZKResponder BackendKind = 29` (Task 8).
- `test/differential/runner_test.go` — the `TCPZKResponder` backend arm + `acceptZKResponder` + responder unit test (Task 8); the `0048` blank-import (Task 9).
- `docs/envoy-go/DECISIONS.md` — ADR-0223 §Decision/§Consequences body IN PLACE (Task 10; tail STAYS ADR-0223).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the 28.2 bundle (Task 10).
- `docs/envoy-go/STATE.md` / `docs/envoy-go/ROADMAP.md` / `next-prompt.txt` — the rollup + handoff (Task 11).

**Untouched (pinned — SPEC §2.4/§3.7):** `internal/filter/network/` framework (`types.go`/`chain.go`/`readconn.go`/`writeconn.go`/`prefixconn.go`/`buffer.go`/`terminal.go`/`callbacks.go`/`registry.go`/`upstreamcluster.go`); `internal/filter/network/zookeeperproxy/` `config.go` / `stats.go` / `config_test.go` / `stats_test.go` / `doc.go`; `internal/filter/network/builtins/`; `internal/filter/tcpproxy/`; `internal/filter/hcm/`; `internal/listener/manager.go`; `internal/bootstrap/`; `internal/stats/name.go`; `test/differential/harness.go`; all 49 existing fixture dirs. Any diff to these files (outside a recorded+reverted R4 break) is a PLAN violation — STOP and re-check.

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:**
- Create: `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md`

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip (SPEC §8.1 / R6)**

Run (from repo root):
```bash
git log --oneline -1
# active fixture dirs (50 on disk expected? NO — 49 expected; 0048 lands at Task 9):
ls -d test/fixtures/[0-9]* | wc -l            # expect 49
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0047-zookeeper-boot-reject
# all 49 blank-imports active (none commented):
grep -c "test/fixtures/00.*/driver" test/differential/runner_test.go   # expect 49
# fuzzers (canonical ./internal-scoped recipe — parent §11.10):
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l    # expect 37
# DECISIONS.md tail + next-free:
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3  # tail = ADR-0223 → next-free ADR-0224
```
Expected: fixture dirs **49 active** (tail `0047-zookeeper-boot-reject`); fuzzers **37**; DECISIONS.md tail **ADR-0223** (next-free **ADR-0224**). 28.2 lands `0048` → **50 active**, the 38th fuzzer → **38**, and the ADR-0223 BODY in place (no new ADR number).

- [ ] **Step 2: Re-confirm the stat surface = 337**

Canonical recipe = the BEHAVIOR_CONTRACT.md cumulative "internal names" narrative accounting. The last delta is the phase-28.1 block at `docs/envoy-go/BEHAVIOR_CONTRACT.md:464`: "**Phase 28.1 extension — 136 → 337 internal names**". Expected: **337**. 28.2 keeps it **337** (increments only — zero creation delta; SPEC §7.2).

- [ ] **Step 3: Re-confirm the 0048 port assignments (D-S28.2-3)**

```bash
grep -rn "150[0-9][0-9]" test/fixtures/*/driver/driver.go | grep -oE "150[0-9][0-9]" | sort -u | tail -6
```
Expected: highest assigned reference port = **15049** (`0047`). Pin `0048` → `l_resp`=15050, `l_fast`=15051, `l_slow`=15052, `l_rflags`=15053. If any of 15050–15053 is unexpectedly taken, shift the block to the next four free ports and record in PROGRESS.md.

- [ ] **Step 4: Re-confirm the as-built line anchors (drift here re-points later tasks)**

Confirm each anchor still holds at the live IMPL tip (all verified at the PLAN session against master tip `54abf1b`; only docs-only commits land between sessions, but the gate catches drift):

| # | File:line | Construct | Used by |
|---|-----------|-----------|---------|
| 1 | `internal/filter/network/zookeeperproxy/decoder.go:27-56` | `requestDecoder` struct (the rename target; `requestsByXid` `:50`, `controlRequestsByXid` `:55`) | Task 2 |
| 2 | `internal/filter/network/zookeeperproxy/decoder.go:58-65` | `newRequestDecoder` (the rename target) | Task 2 |
| 3 | `internal/filter/network/zookeeperproxy/decoder.go:75-90` | `decodeOnData` (untouched body; the `decodeOnWrite` template) | Task 3 |
| 4 | `internal/filter/network/zookeeperproxy/decoder.go:96-112` | `nextFrame` (the `nextWriteFrame` template) | Task 3 |
| 5 | `internal/filter/network/zookeeperproxy/decoder.go:116-141` | `decodeFrame` (the request dispatch the response dispatch mirrors) | Task 3/4 |
| 6 | `internal/filter/network/zookeeperproxy/decoder.go:147-173` | `onConnect` (the `connect_readonly` entry-opname source; `recordControl(connectXid, opname, opConnect)` at `:171`) | Task 4 |
| 7 | `internal/filter/network/zookeeperproxy/decoder.go:216-219` | `recordControl` (a Task-2 lock site) | Task 2 |
| 8 | `internal/filter/network/zookeeperproxy/decoder.go:224` | `wireFootprint` (shared accounting basis) | Task 4 |
| 9 | `internal/filter/network/zookeeperproxy/decoder.go:239-245` | `decoderError` (the `responseError` counting template) | Task 3 |
| 10 | `internal/filter/network/zookeeperproxy/decoder.go:308-334` | `onDataRequest` (`requestsByXid` write at `:332` — a Task-2 lock site) | Task 2 |
| 11 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:70-76` | the no-op `OnWrite` (the Task-6 replacement site); `:40` the `newRequestDecoder` call; `:51` the `decoder *requestDecoder` field | Task 2/6 |
| 12 | `internal/filter/network/zookeeperproxy/zookeeperproxy.go:88` | `OnDestroy` (unchanged — runs strictly after both pumps join, needs NO lock; ADR-0221) | Task 6 (context) |
| 13 | `internal/filter/network/zookeeperproxy/config.go:118-137` | `compiledConfig` (the latency fields `:128-130` — parsed-not-consumed; the three flags `:133-136`) | Task 5 |
| 14 | `internal/filter/network/zookeeperproxy/stats.go:119-152` | `respOpNames` (28 names; NO `connect_readonly` — the §3.4-item-4 trap) | Task 4 |
| 15 | `internal/filter/network/zookeeperproxy/stats.go:204-220` | `inc`/`add` (panic-on-unknown-suffix — what the mapping protects against) | Task 4 |
| 16 | `internal/filter/network/zookeeperproxy/decoder_test.go:16-67` | the test helpers (`be32`/`be64`/`zkFrame`/`connectFrame`/`dataFrame`/`newTestDecoder`/`counterValue`); `padTo` at `:249` | Tasks 2–7 |
| 17 | `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go:135-150` | `TestFilterOnWritePureNoOp` (the Task-6 replacement site); `newTestFilter` at `:26` | Task 6 |
| 18 | `internal/filter/network/zookeeperproxy/fuzz_test.go:28-73` | `FuzzZookeeperRequestDecode` (the 38th fuzzer's sibling; `newRequestDecoder` call at `:49` — Task-2 mechanical site) | Task 2/7 |
| 19 | `internal/filter/network/writeconn.go:34-48` | `writeChainConn.Write` (fresh per-Write Buffer; `endStream=false` — the §3.2 item-1 basis; READ-ONLY context, file untouched) | Task 3/6 (context) |
| 20 | `internal/filter/tcpproxy/filter.go:134-138` | the two pumps + `wg.Wait()` (the §3.6 goroutine anchors; READ-ONLY context, file untouched) | Task 6 (context) |
| 21 | `test/differential/fixture/fixture.go:493-502` | `TCPSink BackendKind = 28` + the 0048-responder forward-pointer comment (the Task-8 insertion site); `:505-510` `BackendKindAware` | Task 8 |
| 22 | `test/differential/runner_test.go:827-841` | the `TCPSink` backend arm (the `TCPZKResponder` arm's sibling) | Task 8 |
| 23 | `test/differential/runner_test.go:1258-1276` | `acceptSinkCounting` (the `acceptZKResponder` template) | Task 8 |
| 24 | `test/differential/runner_test.go:70-72` | the `0045`/`0046`/`0047` blank-imports (the `0048` import lands after `:72`) | Task 9 |
| 25 | `test/fixtures/0046-zookeeper-requests/driver/driver.go` | the multi-listener + StatsAsserter + local-opcode-constants driver template (875 LoC; `driveFrames` `:394`; `AssertStats` `:605`; `renderBootstrap` `:805`) | Task 9 |
| 26 | `docs/envoy-go/DECISIONS.md:14324-14343` | ADR-0223 heading + §AMEND + §Context (the Task-10 body-landing site; the file currently ENDS at `:14343` — the body appends after §Context) | Task 10 |
| 27 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:464` | the 28.1 stat-mapping block (gains the 28.2 increments-only annotation) | Task 10 |
| 28 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3627-3654` | the `### envoy.filters.network.zookeeper_proxy` subsection (the Task-10 response-side extension site) | Task 10 |
| 29 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3656-3686` | the conn-wrap-seam block (its 28.2 forward-pointer is resolved at Task 10) | Task 10 |
| 30 | `docs/envoy-go/BEHAVIOR_CONTRACT.md:3688-3708` | `### Stat surface` / `### Applies to` / `### Does not yet apply to` (the Task-10 edit sites; the 28.2 forward bullet at `:3704`) | Task 10 |
| 31 | `docs/envoy-go/ROADMAP.md:82/:85` | parent row 28 / sub-row 28.2 (the Task-11 ATOMIC rollup sites) | Task 11 |

- [ ] **Step 5: Baseline build + test + race**

```bash
go build ./... && go vet ./...
go test ./internal/filter/network/... -race -short -count=1
go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1   # seed-corpus mode
```
Expected: all green. Record outputs in PROGRESS.md.

- [ ] **Step 6: Commit the gate record**

```bash
git add docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 1: baselines/anchors gate — 49 dirs (tail 0047), fuzzers 37, stats 337, DECISIONS tail ADR-0223, ports 15050-15053 free; as-built anchors re-pinned"
```

---

## Task 2: Decoder rename (`requestDecoder` → `decoder`) + `writeBuf`/`mu` fields + request-path locking (SPEC §3.1 / §3.6)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/decoder.go`
- Modify: `internal/filter/network/zookeeperproxy/decoder_test.go` (mechanical)
- Modify: `internal/filter/network/zookeeperproxy/zookeeperproxy.go:40,51` (mechanical)
- Modify: `internal/filter/network/zookeeperproxy/fuzz_test.go:49` (mechanical)

> This task is a behavior-preserving refactor + additive fields. The TDD discipline here is: every EXISTING test stays green with assertions UNCHANGED at every step. Any assertion change indicates a rename/locking bug — STOP.

- [ ] **Step 1: Mechanical rename**

From the repo root:
```bash
cd internal/filter/network/zookeeperproxy
# The type (lowercase r) and the constructor (capital R) do not overlap as substrings:
sed -i 's/\brequestDecoder\b/decoder/g; s/\bnewRequestDecoder\b/newDecoder/g' \
  decoder.go decoder_test.go zookeeperproxy.go fuzz_test.go
cd ../../../..
gofmt -l internal/filter/network/zookeeperproxy/   # expect: no output
```
Occurrence check (pre-verified at PLAN time): `decoder.go` 13 `requestDecoder` + 1 `newRequestDecoder` = 14 substitutions, `decoder_test.go` 7, `zookeeperproxy.go` 2, `fuzz_test.go` 1; `config.go`/`stats.go`/`config_test.go`/`stats_test.go`/`zookeeperproxy_test.go` have ZERO occurrences (untouched).

- [ ] **Step 2: Update the renamed struct's doc comment**

In `decoder.go`, replace the old `requestDecoder` doc comment (was `:27-29`) with the direction-neutral one:
```go
// decoder is the per-connection shallow decoder, BOTH directions (ADR-0222
// request side; ADR-0223 response side — upstream single-DecoderImpl parity).
// It owns its OWN reassembly buffers; the chain Buffer is read, NEVER drained
// (R3). The request path runs on goroutine A (pre-handoff: the chain read
// loop; post-handoff: the downstream→upstream pump via replayRead); the
// response path runs on goroutine B (the upstream→downstream pump via
// writeChainConn.Write → OnWrite). The two share ONLY the correlation maps,
// guarded by mu (§3.6).
type decoder struct {
```
Also update the constructor's doc line (`newDecoder returns the per-connection decoder...`) and the `zookeeperproxy.go:51` field comment (`decoder *decoder // per-connection (reassembly bufs + correlation structures + mu)`).

- [ ] **Step 3: Run the full package suite — assertions UNCHANGED**

```bash
go test ./internal/filter/network/... -race -short -count=1
```
Expected: PASS, every existing test green, zero assertion edits. (The rename is package-internal; nothing outside `zookeeperproxy/` references the type — pre-verified.)

- [ ] **Step 4: Add the `writeBuf` + `mu` fields (SPEC §3.1 verbatim)**

In `decoder.go`, inside the `decoder` struct, AFTER the `readBuf` field and BEFORE the correlation maps, add:
```go
	// writeBuf is the decoder-internal write-side reassembly buffer (the
	// upstream zk_filter_write_buffer_ mirror). Fed by OnWrite; complete
	// response frames are decoded + consumed; a trailing partial frame
	// survives until the next OnWrite; a decode failure ABANDONS it (no
	// resync — AMEND-A8 symmetry). Accessed ONLY by goroutine B (§3.6).
	writeBuf []byte

	// mu guards the two correlation maps below — the ONLY state shared
	// between goroutine A (request decode: replayRead → OnData) and
	// goroutine B (response decode: writeChainConn.Write → OnWrite).
	// The reassembly buffers (readBuf / writeBuf) and chainConsumed are
	// single-goroutine-owned and stay OUTSIDE the lock (§3.6). Entries are
	// COPIED OUT under the lock; counter increments + latency math happen
	// OUTSIDE it. The pre-handoff request path locks too (uniformity over
	// cleverness — pre-handoff the lock is uncontended, one atomic CAS).
	mu sync.Mutex
```
Add `"sync"` to the imports. The two correlation-map field declarations + their comments stay as-is, now sitting under `mu`.

- [ ] **Step 5: Lock the two request-path map writes (SPEC §3.6 item 3)**

`recordControl` (was `decoder.go:216-219`):
```go
// recordControl appends to the per-xid FIFO control queue (AMEND-A7), under the
// correlation-map lock (§3.6: the request path locks unconditionally — pre-handoff
// the lock is uncontended; post-handoff goroutine B's response decode contends).
func (d *decoder) recordControl(xid int32, opname string, wireOpcode int32) {
	entry := pendingRequest{opname: opname, wireOpcode: wireOpcode, start: time.Now()}
	d.mu.Lock()
	d.controlRequestsByXid[xid] = append(d.controlRequestsByXid[xid], entry)
	d.mu.Unlock()
}
```
`onDataRequest`'s map write (was `decoder.go:332`) — replace the single assignment line with:
```go
	entry := pendingRequest{opname: opname, wireOpcode: opcode, start: time.Now()}
	d.mu.Lock()
	d.requestsByXid[xid] = entry
	d.mu.Unlock()
```
(Note: the `pendingRequest` construction — including `time.Now()` — happens OUTSIDE the critical section in `recordControl` and outside-or-inside is equivalent for `onDataRequest`; keep the lock scope to the map mutation only, per §3.6 item 1.)

- [ ] **Step 6: Run the full package suite + race**

```bash
go test ./internal/filter/network/... -race -short -count=1
go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1
```
Expected: PASS — all existing tests green, assertions unchanged (the lock is invisible to single-goroutine tests). The race test that PROVES the lock is load-bearing lands at Task 6 (it needs the response path to exist).

- [ ] **Step 7: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/ \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 2: requestDecoder -> decoder rename (upstream single-DecoderImpl parity) + writeBuf/mu fields + request-path correlation-map locking (SPEC 3.1/3.6)"
```

---

## Task 3: Write-side reassembly + response framing + the UNCORRELATED dispatch rows (SPEC §3.2 / §3.3 rows 2 + 5 + failure paths)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/decoder.go`
- Test: `internal/filter/network/zookeeperproxy/decoder_test.go`

- [ ] **Step 1: D-S28.2-1 first action — verify the watch-event min-length against upstream**

Fetch `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/network/zookeeper_proxy/decoder.cc` and locate `parseWatchEvent`. Confirm its `ensureMinLength` requirement equals the SPEC pin: xid(4) + event_type(4) + client_state(4) + path-len(4) = **16** (the SPEC's shallow-decode minimum). If upstream differs, use upstream's value and record the finding in PROGRESS.md (the D-S28.1-1 transcription discipline). The constant goes into the Step-5 `onWatchEvent` implementation.

- [ ] **Step 2: Write the response frame builders + failing tests**

Append to `decoder_test.go` (after the existing frame builders ~`:51`):
```go
// --- response frame builders (28.2; big-endian; 4-byte length prefix EXCLUDES itself) ---

// stdRespFrame builds a standard response frame: xid(4) + zxid(8) + error(4)
// (SPEC §3.3 rows 3/4 framing).
func stdRespFrame(xid int32, zxid int64, errCode int32) []byte {
	return zkFrame(be32(xid), be64(zxid), be32(errCode))
}

// connectRespFrame builds a connect response: proto_version(4=0) + timeout(4) +
// session_id(8) + password(4-byte len + bytes) — NO zxid, NO error (SPEC §3.3
// row 1). The leading proto_version=0 doubles as the sniffed connectXid.
func connectRespFrame(pwLen int) []byte {
	return zkFrame(be32(0), be32(30000), be64(0x1234), be32(int32(pwLen)), make([]byte, pwLen))
}

// watchEventFrame builds a server-initiated watch event: xid(-1) + event_type(4)
// + client_state(4) + path(4-byte len + bytes) (SPEC §3.3 row 2).
func watchEventFrame(path string) []byte {
	return zkFrame(be32(watchXid), be32(1), be32(3), be32(int32(len(path))), []byte(path))
}

// feedRequest feeds one request frame through decodeOnData using the decoder's
// own high-water mark for the totalAppended bookkeeping (a per-call delta feed).
func feedRequest(d *decoder, frame []byte) {
	d.decodeOnData(frame, d.chainConsumed+int64(len(frame)))
}
```
Then the Task-3 tests:
```go
// --- Task 3 (28.2): write-side reassembly + framing + uncorrelated dispatch ---

// A watch event (xid −1) increments watch_event + response_bytes and NOTHING
// else: never correlated, no per-opcode counter, no latency (SPEC §3.3 row 2).
func TestDecodeWatchEvent(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := watchEventFrame("/zk-test")
	d.decodeOnWrite(frame)
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1", got)
	}
	// wireFootprint = 4-byte prefix + payload; watchEventFrame returns the
	// PREFIXED frame, so the footprint equals len(frame).
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(frame)) {
		t.Fatalf("response_bytes = %d, want %d", got, len(frame))
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A short watch event (< 16 bytes payload) → decoder_error + abandon.
func TestDecodeWatchEventTooShort(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(zkFrame(be32(watchXid), be32(1))) // 8-byte payload < 16
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := counterValue(t, rs, "watch_event"); got != 0 {
		t.Fatalf("watch_event = %d, want 0", got)
	}
}

// An unknown negative xid (not 0/−1/−2/−4/−8) → decoder_error + abandon
// (SPEC §3.3 row 5 — upstream unknown-xid onDecodeError parity).
func TestDecodeResponseUnknownNegativeXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(stdRespFrame(-3, 1, 0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// A response frame shorter than the universal 4-byte minimum → decoder_error.
func TestDecodeResponseTooShortForXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(zkFrame([]byte{0x00, 0x01})) // 2-byte payload < 4
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// An oversized response frame (length prefix > max_packet_bytes) → decoder_error
// + abandon ("packet is too big" — parent §11.5 symmetry).
func TestDecodeResponseOversized(t *testing.T) {
	d, rs, cfg := newTestDecoder(t)
	huge := append(be32(int32(cfg.maxPacketBytes)+1), make([]byte, 16)...)
	d.decodeOnWrite(huge)
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if d.writeBuf != nil {
		t.Fatal("oversized frame must ABANDON writeBuf (no resync)")
	}
}

// Partial-frame reassembly: a watch event split across three decodeOnWrite calls
// decodes exactly once when complete (the writeBuf reassembly — SPEC §3.2 item 2).
func TestDecodeResponsePartialFrameReassembly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := watchEventFrame("/zk-test")
	d.decodeOnWrite(frame[:3])
	d.decodeOnWrite(frame[3:10])
	if got := counterValue(t, rs, "watch_event"); got != 0 {
		t.Fatalf("watch_event = %d before the frame is complete, want 0", got)
	}
	d.decodeOnWrite(frame[10:])
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1 (reassembled across 3 OnWrite calls)", got)
	}
}

// Abandon-no-resync recovery: after a decode failure abandons writeBuf, a LATER
// decodeOnWrite (a fresh socket write) decodes normally (AMEND-A8 symmetry —
// the 0046 arm-4 request-side analogue).
func TestDecodeResponseAbandonThenRecover(t *testing.T) {
	d, rs, cfg := newTestDecoder(t)
	huge := append(be32(int32(cfg.maxPacketBytes)+1), make([]byte, 16)...)
	d.decodeOnWrite(huge) // decoder_error + abandon
	d.decodeOnWrite(watchEventFrame("/zk-test"))
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1 (the connection survives the abandon)", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (only the oversized frame)", got)
	}
}

// Multiple complete frames in ONE decodeOnWrite call all decode (the frames loop).
func TestDecodeResponseMultipleFramesOneWrite(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	two := append(watchEventFrame("/a"), watchEventFrame("/b")...)
	d.decodeOnWrite(two)
	if got := counterValue(t, rs, "watch_event"); got != 2 {
		t.Fatalf("watch_event = %d, want 2", got)
	}
}

// The correlated rows (connect / control / data) are NOT yet implemented at this
// task: they take the decoder_error path (the documented Task-3 temporary
// posture; Task 4 replaces it). This test pins the temporary behavior so Task 4's
// diff is observable.
func TestDecodeResponseCorrelatedRowsPendingTask4(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(stdRespFrame(1, 1, 0)) // data xid — Task 4 lands the real handler
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (Task-3 temporary posture)", got)
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestDecodeWatchEvent|TestDecodeResponse' -v`
Expected: FAIL — compile error: `d.decodeOnWrite undefined`.

- [ ] **Step 4: Implement `decodeOnWrite` + `nextWriteFrame` + `responseError`**

Append to `decoder.go` (after `onDataRequest`):
```go
// --- the response side (28.2; ADR-0223) ---

// decodeOnWrite feeds upstream→downstream bytes into the decoder's write-side
// reassembly buffer and decodes every complete response frame (SPEC §3.2).
// Each OnWrite call passes a fresh per-Write buffer's contents
// (writeChainConn.Write allocates one per call — writeconn.go:35), so p is
// appended directly: every byte arrives exactly once by construction; no
// write-side TotalAppended high-water mark exists (the §3.2 item-1 structural
// asymmetry vs the read side, recorded in ADR-0223). Runs ONLY on goroutine B.
func (d *decoder) decodeOnWrite(p []byte) {
	d.writeBuf = append(d.writeBuf, p...)
	for {
		frame, ok := d.nextWriteFrame()
		if !ok {
			return // no complete frame buffered (or buffer abandoned)
		}
		if !d.decodeResponseFrame(frame) {
			// decoder_error path already counted + writeBuf abandoned.
			return
		}
	}
}

// nextWriteFrame extracts one complete frame from writeBuf (the 4-byte BE length
// prefix EXCLUDES itself and is stripped from the returned frame). Returns
// ok=false when no complete frame is buffered. Oversized frames
// (len > max_packet_bytes) take the decoder_error path and abandon the buffer.
// (D-S28.2-4: the nextFrame sibling — parallel methods, distinct buffer + error path.)
func (d *decoder) nextWriteFrame() ([]byte, bool) {
	if len(d.writeBuf) < 4 {
		return nil, false
	}
	frameLen := int32(binary.BigEndian.Uint32(d.writeBuf[0:4]))
	if frameLen < 0 || uint32(frameLen) > d.cfg.maxPacketBytes {
		// "packet is too big" (parent §11.5) → decoder_error + abandon.
		d.responseError("")
		return nil, false
	}
	if len(d.writeBuf) < 4+int(frameLen) {
		return nil, false // partial frame — wait for more bytes
	}
	frame := d.writeBuf[4 : 4+frameLen]
	d.writeBuf = d.writeBuf[4+frameLen:]
	return frame, true
}

// responseError runs the decoder_error path for the response side (AMEND-A8
// symmetry; the decoderError counting pattern with the WRITE-side abandon
// target): increment decoder_error (always) + the flag-gated per-opcode counter
// (when opname is known — SPEC §3.3: "from a correlation hit"), then ABANDON
// writeBuf (no resync). The connection is never closed; later writes decode
// normally; the correlation maps persist.
func (d *decoder) responseError(opname string) {
	d.stats.inc("decoder_error")
	if opname != "" && d.cfg.enablePerOpcodeDecoderErrorMetrics {
		d.stats.inc(opname + "_decoder_error")
	}
	d.writeBuf = nil
}

// decodeResponseFrame dispatches one response frame by its leading int32
// (SPEC §3.3 xid sniffing). Returns false on a decode failure (the
// decoder_error path has already run).
func (d *decoder) decodeResponseFrame(frame []byte) bool {
	if len(frame) < 4 {
		// universal min: the leading int32 ("packet is too small").
		d.responseError("")
		return false
	}
	leading := int32(binary.BigEndian.Uint32(frame[0:4]))
	switch {
	case leading == watchXid:
		return d.onWatchEvent(frame)
	case leading == connectXid || leading == pingXid || leading == authXid ||
		leading == setWatchesXid || leading > 0:
		// Correlated rows (connect / control / data) — Task 4 (§3.4) replaces
		// this with onConnectResponse / onControlResponse / onDataResponse.
		d.responseError("")
		return false
	default:
		// Any other negative xid: unknown → decoder_error + abandon (upstream
		// unknown-xid onDecodeError parity — SPEC §3.3 row 5).
		d.responseError("")
		return false
	}
}

// onWatchEvent handles the server-initiated watch-event push (xid −1; SPEC §3.3
// row 2): never correlated, no per-opcode counter, no latency. Shallow
// validation: xid(4) + event_type(4) + client_state(4) + path-len(4) = 16
// minimum (D-S28.2-1, verified against upstream parseWatchEvent at this task).
// Byte accounting: response_bytes only (watch events have no per-opcode
// attribution — SPEC §3.3 byte-accounting note).
func (d *decoder) onWatchEvent(frame []byte) bool {
	if len(frame) < 16 {
		d.responseError("")
		return false
	}
	d.stats.inc("watch_event")
	d.stats.add("response_bytes", uint64(wireFootprint(frame)))
	return true
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestDecodeWatchEvent|TestDecodeResponse' -v`
Expected: PASS (all 9). Then the full suite: `go test ./internal/filter/network/... -race -short -count=1` → PASS (the new methods are additive; nothing calls decodeOnWrite in production yet).

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/ \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 3: write-side reassembly (decodeOnWrite/nextWriteFrame/responseError) + watch-event/unknown/short/oversized dispatch rows (SPEC 3.2/3.3)"
```

---

## Task 4: Correlated dispatch + correlation consumption + byte accounting (SPEC §3.3 rows 1/3/4 + §3.4)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/decoder.go`
- Test: `internal/filter/network/zookeeperproxy/decoder_test.go`

- [ ] **Step 1: D-S28.2-1/D-S28.2-5 first action — verify the connect-response min-length + the error-field disposition against upstream**

From the same upstream `decoder.cc` fetched at Task 3: locate `parseConnectResponse` — confirm the SPEC pin (proto_version(4) + timeout(4) + session_id(8) + password(4-byte len + bytes); fixed part = **20**; NO zxid, NO error). Also confirm (D-S28.2-5) that the response `error(4)` field feeds NO counter (upstream uses it only for dynamic metadata, which is deferred — parent §11.4); the decoder reads past it for min-length only. Record both findings in PROGRESS.md; adjust constants if upstream differs.

- [ ] **Step 2: Write the failing tests**

Append to `decoder_test.go`:
```go
// --- Task 4 (28.2): correlated dispatch + correlation consumption (§3.4) ---

// A data response correlates against requestsByXid, increments <opname>_resp +
// response_bytes, and ERASES the entry (erase-on-lookup — upstream parity).
func TestDecodeDataResponseCorrelates(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	resp := stdRespFrame(1, 100, 0)
	d.decodeOnWrite(resp)
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(resp)) {
		t.Fatalf("response_bytes = %d, want %d (wireFootprint)", got, len(resp))
	}
	if len(d.requestsByXid) != 0 {
		t.Fatal("erase-on-lookup: the entry must be ERASED by the correlation hit")
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A second response with the same data xid finds nothing → decoder_error
// (the erase-on-lookup consequence — SPEC §3.4 item 1).
func TestDecodeDataResponseDoubleResponse(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	d.decodeOnWrite(stdRespFrame(1, 100, 0))
	d.decodeOnWrite(stdRespFrame(1, 101, 0)) // same xid again
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (double response = missing xid)", got)
	}
}

// A data response whose xid has no pending request → decoder_error (upstream
// InvalidArgumentError parity — SPEC §3.3 row 4).
func TestDecodeDataResponseMissingXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(stdRespFrame(42, 100, 0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// Control responses FIFO-pop the per-xid queue: two pings answered in order;
// a third ping response with an empty queue → decoder_error (SPEC §3.4 item 2).
func TestDecodeControlResponseFIFOAndUnderflow(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	d.decodeOnWrite(stdRespFrame(pingXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(pingXid, 2, 0))
	if got := counterValue(t, rs, "ping_resp"); got != 2 {
		t.Fatalf("ping_resp = %d, want 2", got)
	}
	if len(d.controlRequestsByXid[pingXid]) != 0 {
		t.Fatal("FIFO pop must drain the control queue")
	}
	d.decodeOnWrite(stdRespFrame(pingXid, 3, 0)) // empty queue
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (empty control queue)", got)
	}
}

// A connect response (leading int32 == 0) uses the special framing and pops the
// connect control queue → connect_resp (SPEC §3.3 row 1).
func TestDecodeConnectResponse(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, connectFrame(nil))
	d.decodeOnWrite(connectRespFrame(16))
	if got := counterValue(t, rs, "connect_resp"); got != 1 {
		t.Fatalf("connect_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// THE §3.4-ITEM-4 PANIC TRAP: a READONLY connect request's queue entry carries
// opname "connect_readonly", and respOpNames has NO connect_readonly_resp — a
// naive inc(entry.opname + "_resp") PANICS on the closed roster. The response
// decoder must count connect_resp (upstream onConnectResponse parity).
func TestDecodeConnectReadonlyResponseMapsToConnect(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ro := true
	feedRequest(d, connectFrame(&ro)) // entry opname = "connect_readonly"
	// Must NOT panic; must count connect_resp.
	d.decodeOnWrite(connectRespFrame(16))
	if got := counterValue(t, rs, "connect_resp"); got != 1 {
		t.Fatalf("connect_resp = %d, want 1 (the connect_readonly→connect mapping)", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A connect response with NO pending connect request → decoder_error.
func TestDecodeConnectResponseEmptyQueue(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(connectRespFrame(0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_resp"); got != 0 {
		t.Fatalf("connect_resp = %d, want 0", got)
	}
}

// Correlate-then-validate (PLAN refinement 2 — upstream parity): a TRUNCATED
// data response with a valid, correlatable xid consumes the entry AND fires the
// flag-gated per-opcode decoder error (the opname IS known from the correlation
// hit — SPEC §3.3 decode-failure clause).
func TestDecodeDataResponseTruncatedAfterCorrelation(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                          "zk",
		EnablePerOpcodeDecoderErrorMetrics:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	d := newDecoder(cfg, rs)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	// 12-byte payload: xid(4) + 8 more — short of the 16-byte xid+zxid+error minimum.
	d.decodeOnWrite(zkFrame(be32(1), be64(100)))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := rs.counters["getdata_decoder_error"].Load(); got != 1 {
		t.Fatalf("getdata_decoder_error = %d, want 1 (flag-gated, opname from the correlation hit)", got)
	}
	if len(d.requestsByXid) != 0 {
		t.Fatal("correlate-then-validate: the entry is consumed even on a truncated frame")
	}
	if got := rs.counters["getdata_resp"].Load(); got != 0 {
		t.Fatalf("getdata_resp = %d, want 0", got)
	}
}

// Byte accounting flag-gating (SPEC §3.3): response_bytes is ALWAYS counted
// (ungated); <opname>_resp_bytes is counted ONLY when
// enable_per_opcode_response_bytes_metrics is true.
func TestDecodeResponseBytesFlagGating(t *testing.T) {
	// Flag OFF (the newTestDecoder default):
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	resp := stdRespFrame(1, 100, 0)
	d.decodeOnWrite(resp)
	if got := counterValue(t, rs, "getdata_resp_bytes"); got != 0 {
		t.Fatalf("flag OFF: getdata_resp_bytes = %d, want 0", got)
	}
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(resp)) {
		t.Fatalf("flag OFF: response_bytes = %d, want %d (ungated)", got, len(resp))
	}

	// Flag ON:
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                          "zk2",
		EnablePerOpcodeResponseBytesMetrics: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs2 := newRosterStats(reg, "zk2")
	d2 := newDecoder(cfg, rs2)
	feedRequest(d2, dataFrame(1, opGetData, padTo(opGetData)))
	d2.decodeOnWrite(resp)
	if got := rs2.counters["getdata_resp_bytes"].Load(); got != uint64(len(resp)) {
		t.Fatalf("flag ON: getdata_resp_bytes = %d, want %d", got, len(resp))
	}
}

// Control responses for auth and setwatches use their roster opnames
// (auth_resp / setwatches_resp both exist in respOpNames).
func TestDecodeControlResponseAuthAndSetwatches(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	// auth request (xid -4, scheme "digest"):
	authFrame := zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(6), []byte("digest"), be32(0))
	feedRequest(d, authFrame)
	// setwatches request (xid -8):
	feedRequest(d, zkFrame(be32(setWatchesXid), be32(opSetWatches)))
	d.decodeOnWrite(stdRespFrame(authXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(setWatchesXid, 2, 0))
	if got := counterValue(t, rs, "auth_resp"); got != 1 {
		t.Fatalf("auth_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "setwatches_resp"); got != 1 {
		t.Fatalf("setwatches_resp = %d, want 1", got)
	}
}

// The 28.1 "correlation maps grow unbounded" boundary CLOSES: responses drain
// both structures (SPEC §3.4 item 5).
func TestDecodeResponsesDrainCorrelationStructures(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	feedRequest(d, connectFrame(nil))
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	feedRequest(d, dataFrame(2, opSetData, padTo(opSetData)))
	d.decodeOnWrite(connectRespFrame(0))
	d.decodeOnWrite(stdRespFrame(pingXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(1, 2, 0))
	d.decodeOnWrite(stdRespFrame(2, 3, 0))
	if len(d.requestsByXid) != 0 {
		t.Fatalf("requestsByXid has %d entries after all responses, want 0", len(d.requestsByXid))
	}
	for xid, q := range d.controlRequestsByXid {
		if len(q) != 0 {
			t.Fatalf("controlRequestsByXid[%d] has %d entries, want 0", xid, len(q))
		}
	}
}
```
Also DELETE `TestDecodeResponseCorrelatedRowsPendingTask4` (the Task-3 temporary-posture pin — superseded by the real handlers).

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestDecodeDataResponse|TestDecodeControlResponse|TestDecodeConnectResponse|TestDecodeConnectReadonly|TestDecodeResponseBytes|TestDecodeResponsesDrain' -v`
Expected: FAIL — the correlated rows currently take the Task-3 `responseError("")` path (counters stay 0 / decoder_error fires instead).

- [ ] **Step 4: Implement the correlated rows (SPEC §3.3 rows 1/3/4 + §3.4)**

In `decoder.go`, replace `decodeResponseFrame`'s middle case with the three real handlers, and add the helpers:
```go
	switch {
	case leading == connectXid:
		return d.onConnectResponse(frame)
	case leading == watchXid:
		return d.onWatchEvent(frame)
	case leading == pingXid || leading == authXid || leading == setWatchesXid:
		return d.onControlResponse(leading, frame)
	case leading > 0:
		return d.onDataResponse(leading, frame)
	default:
		// Any other negative xid: unknown → decoder_error + abandon (upstream
		// unknown-xid onDecodeError parity — SPEC §3.3 row 5).
		d.responseError("")
		return false
	}
```
The handlers + helpers:
```go
// respOpname maps a popped entry's opname to its response-side roster opname
// (SPEC §3.4 item 4 — THE CLOSED-ROSTER PANIC TRAP): connect_readonly → connect
// (respOpNames has NO connect_readonly_resp; upstream onConnectResponse always
// increments connect_resp). Everything else passes through unchanged.
func respOpname(entryOpname string) string {
	if entryOpname == "connect_readonly" {
		return "connect"
	}
	return entryOpname
}

// popControl FIFO-pops the front entry of the control queue for xid under the
// correlation-map lock and returns a COPY (§3.6: entries copied out under the
// lock; counter increments + latency math happen OUTSIDE it). ok=false → empty
// queue (no correlation hit).
func (d *decoder) popControl(xid int32) (pendingRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	q := d.controlRequestsByXid[xid]
	if len(q) == 0 {
		return pendingRequest{}, false
	}
	entry := q[0]
	d.controlRequestsByXid[xid] = q[1:]
	return entry, true
}

// takeData looks up + ERASES the data-map entry for xid under the correlation-map
// lock (erase-on-lookup — upstream parity: a second response with the same xid
// finds nothing). ok=false → missing xid (no correlation hit).
func (d *decoder) takeData(xid int32) (pendingRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.requestsByXid[xid]
	if ok {
		delete(d.requestsByXid, xid)
	}
	return entry, ok
}

// countResponse runs the per-opcode counting + byte accounting for a
// successfully decoded, correlated response frame (SPEC §3.3 byte accounting):
// <opname>_resp (always) + response_bytes (always, ungated) + the flag-gated
// <opname>_resp_bytes. respOpname MUST already be roster-mapped (§3.4 item 4).
func (d *decoder) countResponse(respOpname string, frame []byte) {
	d.stats.inc(respOpname + "_resp")
	d.stats.add("response_bytes", uint64(wireFootprint(frame)))
	if d.cfg.enablePerOpcodeResponseBytesMetrics {
		d.stats.add(respOpname+"_resp_bytes", uint64(wireFootprint(frame)))
	}
}

// onConnectResponse handles the connect special framing (SPEC §3.3 row 1):
// proto_version(4) + timeout(4) + session_id(8) + password(4-byte len + bytes)
// — NO zxid, NO error (D-S28.2-1). Correlates by FIFO-popping
// controlRequestsByXid[connectXid]; counters ALWAYS use opname "connect"
// regardless of the popped entry's opname (§3.4 item 4).
// Correlate-then-validate order (upstream parity): the pop happens first, so a
// malformed connect response still consumes the pending entry and fires the
// flag-gated connect_decoder_error.
func (d *decoder) onConnectResponse(frame []byte) bool {
	entry, ok := d.popControl(connectXid)
	if !ok {
		d.responseError("") // empty queue — no correlation hit
		return false
	}
	_ = entry // latency consumption lands at Task 5 (recordLatency)
	const fixedLen = 4 + 4 + 8 + 4 // proto_version + timeout + session_id + password length
	if len(frame) < fixedLen {
		d.responseError("connect")
		return false
	}
	pwLen := int32(binary.BigEndian.Uint32(frame[16:20]))
	if pwLen < 0 || len(frame) < fixedLen+int(pwLen) {
		d.responseError("connect")
		return false
	}
	d.countResponse("connect", frame)
	return true
}

// onControlResponse handles control-xid responses (ping −2 / auth −4 /
// setwatches −8; SPEC §3.3 row 3): standard framing xid(4) + zxid(8) + error(4)
// = 16 minimum, correlated by FIFO pop (control xids repeat — AMEND-A7).
func (d *decoder) onControlResponse(xid int32, frame []byte) bool {
	entry, ok := d.popControl(xid)
	if !ok {
		d.responseError("") // empty queue — no correlation hit
		return false
	}
	op := respOpname(entry.opname)
	if len(frame) < 16 {
		// Correlate-then-validate: the entry is consumed; the per-opcode
		// counter fires (flag-gated) — the opname is known from the hit.
		d.responseError(op)
		return false
	}
	d.countResponse(op, frame)
	return true
}

// onDataResponse handles data-xid responses (xid > 0; SPEC §3.3 row 4): standard
// framing, correlated against requestsByXid with erase-on-lookup. The zxid(8) +
// error(4) fields are read past for min-length only — neither is extracted
// (shallow decode; D-S28.2-5).
func (d *decoder) onDataResponse(xid int32, frame []byte) bool {
	entry, ok := d.takeData(xid)
	if !ok {
		d.responseError("") // missing xid — upstream InvalidArgumentError parity
		return false
	}
	op := respOpname(entry.opname)
	if len(frame) < 16 {
		d.responseError(op)
		return false
	}
	d.countResponse(op, frame)
	return true
}
```

- [ ] **Step 5: Run the tests + the full package suite**

Run: `go test ./internal/filter/network/zookeeperproxy/ -v -count=1` → PASS (all Task-3/Task-4 tests + every pre-existing request-side test, assertions unchanged).
Run: `go test ./internal/filter/network/... -race -short -count=1` → PASS.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/ \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 4: correlated response dispatch (connect/control/data) + erase-on-lookup + FIFO pop + connect_readonly->connect mapping + byte accounting (SPEC 3.3/3.4)"
```

---

## Task 5: Latency-threshold counters (SPEC §4)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/decoder.go`
- Test: `internal/filter/network/zookeeperproxy/decoder_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `decoder_test.go`:
```go
// --- Task 5 (28.2): latency-threshold counters (§4) ---

// latencyTestDecoder builds a decoder with enable_latency_threshold_metrics +
// optional overrides. defaultThreshold uses the proto Duration field.
func latencyTestDecoder(t *testing.T, defaultThreshold time.Duration,
	overrides []*zookeeper_proxyv3.LatencyThresholdOverride) (*decoder, *rosterStats) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                    "zk",
		EnableLatencyThresholdMetrics: true,
		DefaultLatencyThreshold:       durationpb.New(defaultThreshold),
		LatencyThresholdOverrides:     overrides,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	return newDecoder(cfg, rs), rs
}

// The inclusive edge (AMEND-A10 / parent §11.7): latency == threshold → FAST.
// recordLatency takes the measured latency as a parameter (PLAN refinement 3),
// so the boundary is tested with exact injected durations.
func TestRecordLatencyInclusiveEdge(t *testing.T) {
	d, rs := latencyTestDecoder(t, 100*time.Millisecond, nil)
	d.recordLatency("getdata", opGetData, 100*time.Millisecond) // == threshold
	if got := counterValue(t, rs, "getdata_resp_fast"); got != 1 {
		t.Fatalf("getdata_resp_fast = %d, want 1 (latency == threshold is FAST — inclusive)", got)
	}
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 0 {
		t.Fatalf("getdata_resp_slow = %d, want 0", got)
	}
	d.recordLatency("getdata", opGetData, 100*time.Millisecond+time.Nanosecond) // > threshold
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 1 {
		t.Fatalf("getdata_resp_slow = %d, want 1 (latency > threshold is SLOW)", got)
	}
}

// A wire-opcode-keyed override beats the default (§4.1 item 3).
func TestRecordLatencyOverrideBeatsDefault(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Millisecond, []*zookeeper_proxyv3.LatencyThresholdOverride{
		{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_GetData, Threshold: durationpb.New(time.Hour)},
	})
	// 10 ms latency: getdata (override 1 h) → fast; setdata (default 1 ms) → slow.
	d.recordLatency("getdata", opGetData, 10*time.Millisecond)
	d.recordLatency("setdata", opSetData, 10*time.Millisecond)
	if got := counterValue(t, rs, "getdata_resp_fast"); got != 1 {
		t.Fatalf("getdata_resp_fast = %d, want 1 (the override wins)", got)
	}
	if got := counterValue(t, rs, "setdata_resp_slow"); got != 1 {
		t.Fatalf("setdata_resp_slow = %d, want 1 (no override → default)", got)
	}
}

// The flag gates INCREMENTS (AMEND-A2): flag off → neither fast nor slow moves.
func TestRecordLatencyFlagOff(t *testing.T) {
	d, rs, _ := newTestDecoder(t) // enable_latency_threshold_metrics defaults false
	d.recordLatency("getdata", opGetData, time.Nanosecond)
	if counterValue(t, rs, "getdata_resp_fast") != 0 || counterValue(t, rs, "getdata_resp_slow") != 0 {
		t.Fatal("flag off: neither fast nor slow may increment")
	}
}

// End-to-end injected-timestamp test: a pending request whose start is in the
// deep past → response decode → SLOW (the time.Since plumbing — §4.1).
func TestLatencyEndToEndInjectedStart(t *testing.T) {
	d, rs := latencyTestDecoder(t, 100*time.Millisecond, nil)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	// Inject: back-date the pending entry far past any threshold.
	d.mu.Lock()
	e := d.requestsByXid[1]
	e.start = time.Now().Add(-time.Hour)
	d.requestsByXid[1] = e
	d.mu.Unlock()
	d.decodeOnWrite(stdRespFrame(1, 100, 0))
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 1 {
		t.Fatalf("getdata_resp_slow = %d, want 1 (1 h latency >> 100 ms threshold)", got)
	}
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (the _resp counter increments alongside fast/slow)", got)
	}
}

// Connect responses participate in latency with opname "connect" + wire opcode
// opConnect (an override keyed on Connect applies — §4.1 item 3).
func TestLatencyConnectResponse(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Hour, nil)
	feedRequest(d, connectFrame(nil))
	d.decodeOnWrite(connectRespFrame(0))
	if got := counterValue(t, rs, "connect_resp_fast"); got != 1 {
		t.Fatalf("connect_resp_fast = %d, want 1 (1 h threshold → fast)", got)
	}
}

// Watch events NEVER get fast/slow (uncorrelated — no request timestamp; §4.1
// item 4); decoder_error responses never get fast/slow (§4.1 item 5).
func TestLatencyNeverForWatchEventsOrErrors(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Hour, nil)
	d.decodeOnWrite(watchEventFrame("/zk-test"))
	d.decodeOnWrite(stdRespFrame(42, 1, 0)) // missing xid → decoder_error
	suffixes := []string{"_resp_fast", "_resp_slow"}
	for _, op := range []string{"getdata", "connect", "exists"} {
		for _, s := range suffixes {
			if got := counterValue(t, rs, op+s); got != 0 {
				t.Fatalf("%s%s = %d, want 0", op, s, got)
			}
		}
	}
}
```
Add `"time"`, `durationpb "google.golang.org/protobuf/types/known/durationpb"` to `decoder_test.go`'s imports as needed.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestRecordLatency|TestLatency' -v`
Expected: FAIL — compile error: `d.recordLatency undefined`.

- [ ] **Step 3: Implement `recordLatency` + the three call sites (SPEC §4.1)**

Append to `decoder.go`:
```go
// recordLatency mirrors upstream errorBudgetDecision (filter.cc:134-154 —
// parent §11.7 / AMEND-A10): flag-gated; threshold = the wire-opcode-keyed
// override else the default; latency <= threshold → FAST (INCLUSIVE).
// respOpname must already be roster-mapped (§3.4 item 4). The measured latency
// is a parameter (not computed here) so unit tests inject exact boundary values.
// Runs OUTSIDE the correlation-map lock (§3.6 item 1).
func (d *decoder) recordLatency(respOpname string, wireOpcode int32, latency time.Duration) {
	if !d.cfg.enableLatencyThresholdMetrics {
		return // the flag gates INCREMENTS, not creation (AMEND-A2)
	}
	threshold, ok := d.cfg.latencyThresholdOverrides[wireOpcode]
	if !ok {
		threshold = d.cfg.defaultLatencyThreshold
	}
	if latency <= threshold {
		d.stats.inc(respOpname + "_resp_fast")
	} else {
		d.stats.inc(respOpname + "_resp_slow")
	}
}
```
Then wire the three call sites (each AFTER the `countResponse` call, replacing the Task-4 placeholder):
- `onConnectResponse`: replace `_ = entry // latency consumption lands at Task 5` with nothing, and after `d.countResponse("connect", frame)` add:
  ```go
  	d.recordLatency("connect", entry.wireOpcode, time.Since(entry.start))
  ```
- `onControlResponse`: after `d.countResponse(op, frame)` add:
  ```go
  	d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))
  ```
- `onDataResponse`: after `d.countResponse(op, frame)` add:
  ```go
  	d.recordLatency(op, entry.wireOpcode, time.Since(entry.start))
  ```

- [ ] **Step 4: Run the tests + the full package suite**

Run: `go test ./internal/filter/network/zookeeperproxy/ -v -count=1` → PASS.
Run: `go test ./internal/filter/network/... -race -short -count=1` → PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/ \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 5: latency-threshold counters — recordLatency (<= INCLUSIVE; wire-opcode-keyed overrides; flag-gated) + injected-duration boundary tests (SPEC 4)"
```

---

## Task 6: `OnWrite` glue + the §3.6 concurrent request/response race test (SPEC §3.2 / §3.6 / R9)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/zookeeperproxy.go:70-76` (the no-op `OnWrite`)
- Modify: `internal/filter/network/zookeeperproxy/zookeeperproxy_test.go:133-150` (`TestFilterOnWritePureNoOp` → replaced)
- Test: `internal/filter/network/zookeeperproxy/decoder_test.go` (the race test)

- [ ] **Step 1: Write the failing filter-level tests**

In `zookeeperproxy_test.go`, DELETE `TestFilterOnWritePureNoOp` (`:133-150` — the 28.1 posture it pins is exactly what this task removes) and add:
```go
// TestFilterOnWriteFeedsDecoder: OnWrite feeds the decoder's write side (the 28.2
// response decoder — ADR-0223) and ALWAYS returns Continue (R3 extended to the
// write side — SPEC §3.2 item 5).
func TestFilterOnWriteFeedsDecoder(t *testing.T) {
	f := newTestFilter(t)
	// Pre-load a pending request so the response correlates.
	reqBuf := &network.Buffer{}
	reqBuf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(reqBuf, false)

	respBuf := &network.Buffer{}
	resp := stdRespFrame(1, 100, 0)
	respBuf.Append(resp)
	before := respBuf.Len()
	if got := f.OnWrite(respBuf, false); got != network.Continue {
		t.Fatalf("OnWrite = %v, want Continue (always — R3)", got)
	}
	if respBuf.Len() != before {
		t.Fatalf("OnWrite drained/mutated the chain buffer (len %d -> %d) — FORBIDDEN (R3)", before, respBuf.Len())
	}
	rs := f.cfg.stats
	if got := rs.counters["getdata_resp"].Load(); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (OnWrite must feed the response decoder)", got)
	}
	if got := rs.counters["response_bytes"].Load(); got != uint64(len(resp)) {
		t.Fatalf("response_bytes = %d, want %d", got, len(resp))
	}
}

// TestFilterOnWritePartialFramesAcrossCalls: response bytes split across multiple
// OnWrite calls (each a FRESH per-Write Buffer — writeconn.go:35) reassemble in
// the decoder's writeBuf (SPEC §3.2 item 1: no write-side TotalAppended; each
// OnWrite call's bytes are appended directly).
func TestFilterOnWritePartialFramesAcrossCalls(t *testing.T) {
	f := newTestFilter(t)
	reqBuf := &network.Buffer{}
	reqBuf.Append(dataFrame(1, opGetData, padTo(opGetData)))
	f.OnData(reqBuf, false)

	resp := stdRespFrame(1, 100, 0)
	cut := len(resp) / 2
	for _, half := range [][]byte{resp[:cut], resp[cut:]} {
		b := &network.Buffer{} // fresh per-Write Buffer, exactly as writeChainConn.Write does
		b.Append(half)
		if got := f.OnWrite(b, false); got != network.Continue {
			t.Fatalf("OnWrite = %v, want Continue", got)
		}
	}
	if got := f.cfg.stats.counters["getdata_resp"].Load(); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (reassembled across OnWrite calls)", got)
	}
}
```

- [ ] **Step 2: Write the failing §3.6 race test**

Append to `decoder_test.go`:
```go
// --- Task 6 (28.2): the §3.6 concurrent request/response race test (R9) ---

// TestDecoderConcurrentRequestResponseRace drives goroutine A (request decode —
// the replayRead → OnData path) and goroutine B (response decode — the
// writeChainConn.Write → OnWrite path) CONCURRENTLY over one decoder. This is
// the production goroutine topology post-handoff (tcpproxy filter.go:134-138).
// The assertion is `go test -race` itself (the §3.6 mutex makes the correlation
// maps race-free) plus a conservation check: every response either correlated
// or counted decoder_error. Run with -race -count=5 (the zookeeperproxy-package
// analogue of the 28.1b framework-level concurrent-pumps test).
func TestDecoderConcurrentRequestResponseRace(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	const n = 200
	var wg sync.WaitGroup
	wg.Add(2)
	// Goroutine A: request decode (WRITES the correlation maps).
	go func() {
		defer wg.Done()
		var consumed int64
		for i := 1; i <= n; i++ {
			frame := dataFrame(int32(i), opGetData, padTo(opGetData))
			consumed += int64(len(frame))
			d.decodeOnData(frame, consumed)
		}
	}()
	// Goroutine B: response decode (READS + ERASES the correlation maps).
	go func() {
		defer wg.Done()
		for i := 1; i <= n; i++ {
			d.decodeOnWrite(stdRespFrame(int32(i), int64(i), 0))
		}
	}()
	wg.Wait()

	// Conservation: every response was either correlated (getdata_resp) or
	// arrived before its request was recorded (decoder_error). No response
	// is lost, none double-counted.
	resp := counterValue(t, rs, "getdata_resp")
	errs := counterValue(t, rs, "decoder_error")
	if resp+errs != n {
		t.Fatalf("getdata_resp(%d) + decoder_error(%d) = %d, want %d (conservation)", resp, errs, resp+errs, n)
	}
}
```
Add `"sync"` to `decoder_test.go`'s imports if not present.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/filter/network/zookeeperproxy/ -run 'TestFilterOnWrite' -v`
Expected: FAIL — `TestFilterOnWriteFeedsDecoder`: `getdata_resp = 0, want 1` (OnWrite is still the 28.1 no-op).
(The race test PASSES already at this step — `decodeOnWrite` exists since Task 3 and the maps are locked since Task 2/4. That is expected; the test's purpose is the permanent R9 regression gate. Note it in PROGRESS.)

- [ ] **Step 4: Replace the no-op `OnWrite` (SPEC §3.2 verbatim)**

In `zookeeperproxy.go`, replace the no-op `OnWrite` (`:70-76`) with:
```go
// OnWrite feeds the decoder's write-side reassembly buffer with the
// upstream→downstream bytes and ALWAYS returns Continue (AMEND-A8
// unconditional passthrough; R3 — the filter never mutates the chain Buffer,
// never halts the write, never closes). Each OnWrite call sees a FRESH
// per-Write *Buffer (writeChainConn.Write allocates one per call), so the
// bytes are appended directly — no TotalAppended high-water mark is needed on
// the write side (every byte arrives exactly once by construction; SPEC §3.2
// item 1 / ADR-0223). Runs on goroutine B (the upstream→downstream pump);
// the §3.6 decoder mutex makes the correlation-map accesses race-free against
// goroutine A's request decode.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.decoder.decodeOnWrite(buf.Bytes())
	return network.Continue
}
```

- [ ] **Step 5: Run the race gate + the full suites**

```bash
go test ./internal/filter/network/zookeeperproxy/ -run TestDecoderConcurrentRequestResponseRace -race -count=5 -v
go test ./internal/filter/network/... -race -short -count=1
go test ./internal/filter/network/zookeeperproxy/ -v -count=1
```
Expected: ALL PASS, ZERO race reports, 5/5 race runs. A race report here is a REAL §3.6 design violation — STOP, do not suppress; re-check that every correlation-map access (request inserts in `recordControl`/`onDataRequest`; response pops in `popControl`/`takeData`) holds `d.mu`, and that `writeBuf`/`readBuf`/`chainConsumed` are touched only by their owning goroutine.

Also verify the 28.1b framework-level concurrent-pumps test still passes unchanged (its synthetic filters are not zookeeperproxy — SPEC §3.6 item 4):
```bash
go test ./internal/filter/network/ -run TestWrappedChainConcurrentPumpsRace -race -count=5
```
Expected: PASS 5/5.

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/ \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 6: OnWrite feeds the response decoder (replaces the 28.1 no-op) + the 3.6 concurrent request/response race test -race -count=5 (SPEC 3.2/3.6; R9)"
```

---

## Task 7: The 38th fuzzer — `FuzzZookeeperResponseDecode` (SPEC §6 / R10)

**Files:**
- Modify: `internal/filter/network/zookeeperproxy/fuzz_test.go`

- [ ] **Step 1: Write the fuzzer**

Append to `fuzz_test.go` (the `FuzzZookeeperRequestDecode` sibling):
```go
// FuzzZookeeperResponseDecode is the 38th fuzzer (parent §11.10 / D-P6 /
// SPEC §6). It feeds arbitrary bytes through the production decodeOnWrite entry
// point on a decoder PRE-LOADED with pending requests (so the correlation paths
// are reachable) and with the latency + per-opcode-response-bytes flags ON (so
// those counter paths are fuzzed too), asserting:
//  1. no panic — in particular, the closed-roster rosterStats.inc can never
//     receive an unknown suffix (the §3.4-item-4 connect_readonly→connect
//     mapping is exactly what this guards);
//  2. writeBuf stays bounded by max_packet_bytes + slack (no unbounded growth
//     — R10, the 37th fuzzer's bounded-reassembly discipline);
//  3. the correlation maps never GROW from response input (responses only
//     erase/pop — R10).
func FuzzZookeeperResponseDecode(f *testing.F) {
	// Seed corpus (SPEC §6): a valid data response, a connect response, a watch
	// event, a control (ping) response, an unknown-xid response, a truncated
	// frame, an oversized frame.
	f.Add(stdRespFrame(1, 1, 0))
	f.Add(connectRespFrame(16))
	f.Add(watchEventFrame("/path"))
	f.Add(stdRespFrame(pingXid, 1, 0))
	f.Add(stdRespFrame(9999, 1, 0))
	f.Add(stdRespFrame(1, 1, 0)[:6])
	f.Add(append(be32(1<<20), make([]byte, 16)...))

	f.Fuzz(func(t *testing.T, data []byte) {
		const maxPkt = 1024 // small bound so the invariant is exercised by short inputs
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:                          "fuzzresp",
			MaxPacketBytes:                      wrapperspb.UInt32(maxPkt),
			EnableLatencyThresholdMetrics:       true,
			EnablePerOpcodeResponseBytesMetrics: true,
			EnablePerOpcodeDecoderErrorMetrics:  true,
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "fuzzresp")
		d := newDecoder(cfg, rs)

		// Pre-load pending requests so every correlation path is reachable:
		// a data request, a READONLY connect (the §3.4-item-4 trap), a ping.
		feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
		ro := true
		feedRequest(d, connectFrame(&ro))
		feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
		mapsBefore := correlationSize(d)

		orig := append([]byte(nil), data...)

		// Invariant 1: no panic (implicit — a panic fails the fuzz run).
		d.decodeOnWrite(data)
		// Feed a second time (reassembly accumulation across OnWrite calls).
		d.decodeOnWrite(data)

		// The input slice is never mutated (R3).
		if !bytes.Equal(data, orig) {
			t.Fatal("decodeOnWrite mutated the input bytes")
		}

		// Invariant 3: response input never GROWS the correlation maps.
		if got := correlationSize(d); got > mapsBefore {
			t.Fatalf("correlation maps grew from %d to %d entries on response input", mapsBefore, got)
		}

		// Invariant 2: the write-side reassembly buffer is bounded.
		if len(d.writeBuf) > maxPkt+8 {
			t.Fatalf("writeBuf grew to %d bytes, want <= max_packet_bytes(%d)+8", len(d.writeBuf), maxPkt)
		}
	})
}

// correlationSize returns the total entry count across both correlation
// structures (single-goroutine fuzz context — no lock needed for the read,
// but take it anyway for -race cleanliness under fuzz workers).
func correlationSize(d *decoder) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.requestsByXid)
	for _, q := range d.controlRequestsByXid {
		n += len(q)
	}
	return n
}
```

- [ ] **Step 2: Run the seed corpus + a short live fuzz**

```bash
go test ./internal/filter/network/zookeeperproxy/ -run FuzzZookeeperResponseDecode -count=1 -v
go test ./internal/filter/network/zookeeperproxy/ -fuzz FuzzZookeeperResponseDecode -fuzztime 30s
go test ./internal/filter/network/zookeeperproxy/ -run Fuzz -count=1   # both fuzzers' seed corpora
```
Expected: seed corpus PASS; 30 s live fuzz finds nothing (`execs` grows, zero crashers). If a crasher IS found: it is a REAL bug in Tasks 3–5 (most likely a missing roster-mapping or a bounds error) — fix the production code, add the crasher input as a regression seed (`f.Add(...)`), record in PROGRESS.md (the 28.1a 2-real-bugs precedent). Do NOT weaken the invariants.

- [ ] **Step 3: Verify the fuzzer count = 38**

```bash
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 38
```

- [ ] **Step 4: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/zookeeperproxy/ ; golangci-lint run ./internal/filter/network/...
git add internal/filter/network/zookeeperproxy/fuzz_test.go \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 7: FuzzZookeeperResponseDecode — the 38th fuzzer (no-panic / bounded writeBuf / maps-never-grow; pre-loaded correlation incl. the readonly-connect trap) (SPEC 6; R10)"
```

---

## Task 8: `TCPZKResponder` BackendKind = 29 + the runner backend arm (SPEC §5.1; resolves parent D-P9)

**Files:**
- Modify: `test/differential/fixture/fixture.go:493-503` (append the new kind to the const block)
- Modify: `test/differential/runner_test.go` (the backend-dispatch arm + `acceptZKResponder` + helpers + unit test)

- [ ] **Step 1: Add the BackendKind (failing reference)**

In `test/differential/fixture/fixture.go`, inside the BackendKind const block, AFTER `TCPSink BackendKind = 28` (`:502`), add:
```go
	// TCPZKResponder is a ZooKeeper-aware canned-response TCP backend: for every
	// complete ZK request frame it reads (4-byte BE length prefix + frame), it
	// waits a FIXED delay (zkResponderDelay, 10ms — D-S28.2-2), then writes a
	// correlated canned response frame. Added at 28.2 for 0048-zookeeper-responses
	// (28.2 SPEC §5). The fixed delay is the deterministic-threshold construction
	// (parent D-P9): every measured latency is ≥ the delay on BOTH sides, so a
	// 1ms threshold makes every response slow and a 3600s threshold makes every
	// response fast — no cross-side timing nondeterminism. Trigger behaviors
	// (D-S28.2-2): a getacl request (wire op 6) is answered with xid+1000
	// (→ decoder_error both sides); an exists request (wire op 3) is answered
	// normally THEN followed by an unsolicited watch-event push (xid −1).
	// NEW BackendKind per reference_differential_fixture_dispatch_constraint
	// (one fixture dir = one runner branch); TCPSink stays request-side-only
	// (the fixture.go:500 pin).
	TCPZKResponder BackendKind = 29
```
Also update the trailing sentence of `TCPSink`'s doc comment (`:500-501`): "(28.2's 0048 uses a driver-controlled responder — a separate kind; TCPSink stays request-side-only.)" → append " That responder is TCPZKResponder = 29 (landed at 28.2)."

- [ ] **Step 2: Write the failing responder unit test**

Append to `test/differential/runner_test.go` (near `acceptSinkCounting`'s tests, or at the end of the file):
```go
// TestZKResponderBackend unit-tests the TCPZKResponder accept loop against a raw
// TCP client (no proxies): canned connect response, xid-echoed standard
// responses with the fixed pre-response delay, the wrong-xid trigger (getacl),
// and the watch-event-push trigger (exists). This proves the backend BEFORE the
// docker-dependent 0048 fixture consumes it (Task 9).
func TestZKResponderBackend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Uint64
	go acceptZKResponder(ln, &accepts)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	readFrame := func() []byte {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var lenBuf [4]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			t.Fatalf("read frame length: %v", err)
		}
		frame := make([]byte, binary.BigEndian.Uint32(lenBuf[:]))
		if _, err := io.ReadFull(conn, frame); err != nil {
			t.Fatalf("read frame body: %v", err)
		}
		return frame
	}
	writeFrame := func(payload []byte) {
		t.Helper()
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
		if _, err := conn.Write(append(lenBuf[:], payload...)); err != nil {
			t.Fatal(err)
		}
	}
	be32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}

	// 1. Connect request (leading int32 == 0) → 20-byte connect response, after ≥ the fixed delay.
	start := time.Now()
	connectReq := append(append(append(append(be32(0), make([]byte, 8)...), be32(30000)...), make([]byte, 8)...), be32(0)...)
	writeFrame(connectReq)
	resp := readFrame()
	if elapsed := time.Since(start); elapsed < zkResponderDelay {
		t.Fatalf("connect response arrived after %v, want >= %v (the fixed-delay discipline)", elapsed, zkResponderDelay)
	}
	if len(resp) != 20 || int32(binary.BigEndian.Uint32(resp[0:4])) != 0 {
		t.Fatalf("connect response: len=%d leading=%d, want len=20 leading=0", len(resp), int32(binary.BigEndian.Uint32(resp[0:4])))
	}

	// 2. Data request (getdata, xid 7) → standard 16-byte response echoing xid 7.
	writeFrame(append(append(be32(7), be32(4)...), be32(0)...)) // xid 7, op getdata(4), path-len 0
	resp = readFrame()
	if len(resp) != 16 || int32(binary.BigEndian.Uint32(resp[0:4])) != 7 {
		t.Fatalf("data response: len=%d xid=%d, want len=16 xid=7", len(resp), int32(binary.BigEndian.Uint32(resp[0:4])))
	}

	// 3. Wrong-xid trigger: getacl (op 6, xid 9) → response carries xid 9+1000.
	writeFrame(append(append(be32(9), be32(6)...), be32(0)...))
	resp = readFrame()
	if got := int32(binary.BigEndian.Uint32(resp[0:4])); got != 1009 {
		t.Fatalf("wrong-xid trigger: response xid = %d, want 1009", got)
	}

	// 4. Watch-push trigger: exists (op 3, xid 10) → normal response THEN a watch event (xid −1).
	writeFrame(append(append(be32(10), be32(3)...), be32(0)...))
	resp = readFrame()
	if got := int32(binary.BigEndian.Uint32(resp[0:4])); got != 10 {
		t.Fatalf("watch-push trigger: first response xid = %d, want 10", got)
	}
	push := readFrame()
	if got := int32(binary.BigEndian.Uint32(push[0:4])); got != -1 {
		t.Fatalf("watch-push trigger: push frame xid = %d, want -1", got)
	}

	if accepts.Load() != 1 {
		t.Fatalf("accepts = %d, want 1", accepts.Load())
	}
}
```
(Add `"encoding/binary"` to `runner_test.go`'s imports if not already present; `io`/`net`/`time`/`sync/atomic` are already imported for the existing backend arms.)

- [ ] **Step 3: Run to verify failure**

Run: `go test ./test/differential/ -run TestZKResponderBackend -v -count=1`
Expected: FAIL — compile error: `acceptZKResponder undefined` / `zkResponderDelay undefined`.

- [ ] **Step 4: Implement the responder + the backend-dispatch arm**

In `test/differential/runner_test.go`:

(a) The responder constants + accept loop (after `acceptSinkCounting`, `:1276`):
```go
// zkResponderDelay is the TCPZKResponder fixed pre-response delay (D-S28.2-2:
// 10 ms — 10x the 0048 slow-arm 1ms threshold, so every measured latency is
// deterministically ≥ the delay on both sides; parent D-P9).
const zkResponderDelay = 10 * time.Millisecond

// TCPZKResponder trigger opcodes (D-S28.2-2). The responder peeks the request
// frame's opcode int (bytes 4-8) for data requests only.
const (
	zkTriggerWrongXid  int32 = 6 // getacl → respond with xid+1000 (uncorrelated → decoder_error)
	zkTriggerWatchPush int32 = 3 // exists → normal response + unsolicited watch-event push
)

// acceptZKResponder accepts connections, counts them, and runs the ZooKeeper-aware
// canned-response loop on each (the TCPZKResponder backend — 28.2 SPEC §5.1; the
// acceptSinkCounting sibling). The responder parses ONLY the request frame's
// length prefix + leading xid + (for data requests) the opcode int; it is NOT a
// ZooKeeper server (no session/watch semantics).
func acceptZKResponder(ln net.Listener, counter *atomic.Uint64) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		counter.Add(1)
		go zkRespondLoop(c)
	}
}

// zkRespondLoop reads request frames and writes canned responses until the
// client closes (read error / EOF). zxid is monotonic per connection.
func zkRespondLoop(c net.Conn) {
	defer func() { _ = c.Close() }()
	be32 := func(v int32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(v))
		return b
	}
	be64 := func(v int64) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(v))
		return b
	}
	writeFrame := func(payload []byte) bool {
		out := append(be32(int32(len(payload))), payload...)
		_, err := c.Write(out)
		return err == nil
	}
	var zxid int64
	for {
		var lenBuf [4]byte
		if _, err := io.ReadFull(c, lenBuf[:]); err != nil {
			return // client closed / EOF
		}
		frameLen := int32(binary.BigEndian.Uint32(lenBuf[:]))
		if frameLen < 4 || frameLen > 1<<20 {
			return // malformed / hostile — drop the connection
		}
		frame := make([]byte, frameLen)
		if _, err := io.ReadFull(c, frame); err != nil {
			return
		}
		xid := int32(binary.BigEndian.Uint32(frame[0:4]))

		// The fixed pre-response delay (every response, triggers included).
		time.Sleep(zkResponderDelay)

		if xid == 0 {
			// Connect request → canned connect response (20 bytes):
			// proto_version(0) + timeout(30000) + session_id + password(len 0).
			resp := append(append(append(be32(0), be32(30000)...), be64(0x5A5A)...), be32(0)...)
			if !writeFrame(resp) {
				return
			}
			continue
		}

		// Data/control request → standard response: xid(echoed) + zxid(8,
		// monotonic) + error(4, 0) = 16 bytes. Triggers adjust.
		opcode := int32(0)
		if len(frame) >= 8 {
			opcode = int32(binary.BigEndian.Uint32(frame[4:8]))
		}
		zxid++
		respXid := xid
		if opcode == zkTriggerWrongXid {
			respXid = xid + 1000 // the wrong-xid trigger (D-S28.2-2)
		}
		resp := append(append(be32(respXid), be64(zxid)...), be32(0)...)
		if !writeFrame(resp) {
			return
		}
		if opcode == zkTriggerWatchPush {
			// The watch-event push trigger (D-S28.2-2): an unsolicited
			// watch-event frame after the normal response.
			// xid(−1) + event_type(1=created) + client_state(3=connected) + path.
			path := []byte("/zk-watch")
			push := append(append(append(be32(-1), be32(1)...), be32(3)...), append(be32(int32(len(path))), path...)...)
			if !writeFrame(push) {
				return
			}
		}
	}
}
```

(b) The backend-dispatch arm — in the runner's backend-allocation switch, AFTER `case fixture.TCPSink:` (`:827-841`), add:
```go
		case fixture.TCPZKResponder:
			// ZooKeeper-aware canned responder (28.2 SPEC §5.1): for every request
			// frame, wait the fixed zkResponderDelay then write a correlated canned
			// response (+ the D-S28.2-2 trigger behaviors). The fixed delay is the
			// deterministic-threshold construction (parent D-P9).
			ln, err := net.Listen("tcp", "0.0.0.0:0")
			if err != nil {
				t.Fatalf("backend[%d] listen: %v", i, err)
			}
			defer func(ln net.Listener) { _ = ln.Close() }(ln)
			bo.ln = ln
			bo.port = ln.Addr().(*net.TCPAddr).Port
			go acceptZKResponder(ln, bo.accepts)
```

- [ ] **Step 5: Run the unit test + the existing differential compile**

```bash
go test ./test/differential/ -run TestZKResponderBackend -v -count=1
go vet ./test/...
```
Expected: PASS. (The full differential suite is NOT run here — no fixture uses the new kind yet; Task 9 + the Task-11 gate cover it.)

- [ ] **Step 6: gofmt + lint + commit**

```bash
gofmt -l test/ ; golangci-lint run ./test/...
git add test/differential/fixture/fixture.go test/differential/runner_test.go \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 8: TCPZKResponder BackendKind=29 + acceptZKResponder runner arm (fixed 10ms delay; getacl wrong-xid + exists watch-push triggers) + unit test (SPEC 5.1; D-P9/D-S28.2-2)"
```

---

## Task 9: `0048-zookeeper-responses` — driver + cross-side GREEN + R4 deliberate-break + README (SPEC §5.2; R4/R5)

> Requires docker (the differential harness boots reference Envoy v1.37.2). ALL differential runs in this task use `-count=1` (`reference_differential_break_protocol_count1`).

**Files:**
- Create: `test/fixtures/0048-zookeeper-responses/driver/driver.go`
- Create: `test/fixtures/0048-zookeeper-responses/README.md`
- Modify: `test/differential/runner_test.go` (the `0048` blank-import after `:72`)

- [ ] **Step 1: Author the driver**

Mirror `test/fixtures/0046-zookeeper-requests/driver/driver.go` (875 LoC — the multi-listener + StatsAsserter + local-opcode-constants template) with the 0048 substitutions. The structural deltas from 0046:

1. **Four listeners** (not two): `l_resp` / `l_fast` / `l_slow` / `l_rflags`, ref ports 15050/15051/15052/15053 (D-S28.2-3; subject ports = `subjListenerPort` + 0/1/2/3), stat prefixes `zk_resp` / `zk_fast` / `zk_slow` / `zk_rflags`.
2. **BackendKind() = fixture.TCPZKResponder** (not TCPSink) — ONE shared responder backend (`c_zk` cluster) serves all four listeners.
3. **Round-trip driving** (not write-only): each arm WRITES a request frame then READS the expected number of response frames before proceeding — this makes the cross-side decode ordering deterministic (every response is decoded by both proxies before the next request is sent) and applies natural backpressure.
4. **Cross-side dispatch constraints honored:** ONE runner branch (cross-side — `reference_differential_fixture_dispatch_constraint`); ALL stat assertions via `StatsAsserter.AssertStats` (`reference_differential_asserter_dispatch`); wire opcodes redeclared locally (no `internal/` import — the 0046 import-cycle precedent).

Key driver constants:
```go
const (
	fixtureName  = "0048-zookeeper-responses"
	refAdminPort = 9901
	// 150NN convention: 0047 took 15049 → 0048 takes 15050–15053 (D-S28.2-3).
	refLRespPort   = 15050
	refLFastPort   = 15051
	refLSlowPort   = 15052
	refLRflagsPort = 15053

	statPrefixResp   = "zk_resp"
	statPrefixFast   = "zk_fast"
	statPrefixSlow   = "zk_slow"
	statPrefixRflags = "zk_rflags"

	zkPath = "/zk-test"
	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0043/0046 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond
	// respReadTimeout bounds each response-frame read (the responder answers
	// after a fixed 10ms delay; 5s is generous for both proxies).
	respReadTimeout = 5 * time.Second
)

// Wire opcodes local to the driver (no internal/ import — 0046 precedent),
// values matching internal/filter/network/zookeeperproxy/config.go.
const (
	drvOpCreate  int32 = 1
	drvOpDelete  int32 = 2
	drvOpExists  int32 = 3 // the watch-event-push trigger (D-S28.2-2)
	drvOpGetData int32 = 4
	drvOpSetData int32 = 5
	drvOpGetACL  int32 = 6 // the wrong-xid trigger (D-S28.2-2)
	drvOpSync    int32 = 9
	drvOpPing    int32 = 11
	drvOpClose   int32 = -11
)
```

The round-trip helper (replaces 0046's write-only `driveFrames`):
```go
// driveRoundTrips opens a fresh connection to addr, then for each request frame
// frames[i]: writes it, reads back responses[i] complete response frames (the
// responder's correlated reply + any unsolicited push), then proceeds. Returns
// the number of request frames written and any error. Reading-before-next-write
// makes every response decode on both proxies before the next request is sent
// (deterministic cumulative counters for AssertStats).
func driveRoundTrips(ctx context.Context, addr string, frames [][]byte, responses []int) (int, error) {
	conn, err := dialZK(ctx, addr)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	for i, frame := range frames {
		if _, err := conn.Write(frame); err != nil {
			return i, fmt.Errorf("write frame %d: %w", i, err)
		}
		for r := 0; r < responses[i]; r++ {
			if err := readZKFrame(conn); err != nil {
				return i, fmt.Errorf("read response %d of frame %d: %w", r, i, err)
			}
		}
	}
	return len(frames), nil
}

// readZKFrame reads one complete length-prefixed frame (and discards it — the
// driver asserts via stats, not bytes).
func readZKFrame(conn net.Conn) error {
	if err := conn.SetReadDeadline(time.Now().Add(respReadTimeout)); err != nil {
		return err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n > 1<<20 {
		return fmt.Errorf("oversized response frame: %d", n)
	}
	_, err := io.CopyN(io.Discard, conn, int64(n))
	return err
}
```

The 8-arm workload (`driveProxy`, side-independent verdict lines — the 0046 shape). Frame payload builders: COPY 0046's `be32`/`be64`/`zkFrame`/`connectFrame`/`dataFrame`/`pingFrame`/`getdataPayload`/`createPayload`/`dialZK`/`emitArm`/`scrapeZKStats`/`parseZKPromBody`/`lookupZKCounter` into the new driver package (driver packages are self-contained — they cannot import each other or `internal/`; the 0046/0043 precedent), and ADD `existsPayload` (path-len + path + watch bool), `getaclPayload`/`syncPayload` (path-len + path), `setdataPayload` (path-len + path + data-len + version), `deletePayload` (path-len + path + version) — each meeting its request-side min-length:
```go
	// Arm 1 (round-trips, l_resp): connect + getdata(1) + create(2) + ping +
	// close(3), each answered with exactly 1 response frame.
	n, err := driveRoundTrips(ctx, lResp, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
		dataFrame(2, drvOpCreate, createPayload(zkPath)),
		pingFrame(),
		dataFrame(3, drvOpClose, nil),
	}, []int{1, 1, 1, 1, 1})
	emitArm(&b, side, "round-trips", n, err)

	// Arm 2 (watch event, l_resp): exists(4) [the watch-push trigger] → the
	// correlated response + the unsolicited watch-event push = 2 frames.
	n, err = driveRoundTrips(ctx, lResp, [][]byte{
		dataFrame(4, drvOpExists, existsPayload(zkPath)),
	}, []int{2})
	emitArm(&b, side, "watch-event", n, err)

	// Arm 3 (unknown xid + survival, l_resp): getacl(5) [the wrong-xid trigger →
	// the response carries xid 1005 → decoder_error on both sides], then sync(6)
	// on the SAME connection → sync_resp (the abandon-no-resync recovery proof).
	n, err = driveRoundTrips(ctx, lResp, [][]byte{
		dataFrame(5, drvOpGetACL, getaclPayload(zkPath)),
		dataFrame(6, drvOpSync, syncPayload(zkPath)),
	}, []int{1, 1})
	emitArm(&b, side, "unknown-xid-survival", n, err)

	// Arm 4 (all-fast, l_fast — 3600s default threshold): connect + getdata(1) +
	// setdata(2) → every response FAST on both sides (no round-trip takes an hour).
	n, err = driveRoundTrips(ctx, lFast, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
		dataFrame(2, drvOpSetData, setdataPayload(zkPath)),
	}, []int{1, 1, 1})
	emitArm(&b, side, "all-fast", n, err)

	// Arm 5 (all-slow, l_slow — 1ms default threshold + the responder's fixed
	// ≥10ms delay): connect + setdata(1) + delete(2) → every response SLOW on
	// both sides.
	n, err = driveRoundTrips(ctx, lSlow, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpSetData, setdataPayload(zkPath)),
		dataFrame(2, drvOpDelete, deletePayload(zkPath)),
	}, []int{1, 1, 1})
	emitArm(&b, side, "all-slow", n, err)

	// Arm 6 (override, l_slow): getdata(3) → getdata_resp_FAST (the GetData
	// override 3600s beats the 1ms default — proves wire-opcode-keyed override
	// consumption) while arm 5's setdata/delete were SLOW.
	n, err = driveRoundTrips(ctx, lSlow, [][]byte{
		dataFrame(3, drvOpGetData, getdataPayload(zkPath)),
	}, []int{1})
	emitArm(&b, side, "override-fast", n, err)

	// Arm 7 (flag-gated resp-bytes, l_rflags): getdata(1) round-trip →
	// getdata_resp_bytes > 0 and equal cross-side; on l_resp (flag false) it
	// stays 0 on both sides (asserted in AssertStats).
	n, err = driveRoundTrips(ctx, lRflags, [][]byte{
		connectFrame(false),
		dataFrame(1, drvOpGetData, getdataPayload(zkPath)),
	}, []int{1, 1})
	emitArm(&b, side, "flag-gated-resp-bytes", n, err)

	// Arm 8 (R4 deliberate-break): recorded procedure (no live traffic).
	fmt.Fprintf(&b, "arm deliberate-break sent=0 verdict=recorded\n")
```

The bootstrap rendering (4 listeners; the 0046 `renderBootstrap` shape extended). Per-listener zookeeper_proxy `typed_config`:
- `l_resp`: `stat_prefix: zk_resp` only (defaults — no latency metrics, no flags).
- `l_fast`: `stat_prefix: zk_fast` + `enable_latency_threshold_metrics: true` + `default_latency_threshold: 3600s`.
- `l_slow`: `stat_prefix: zk_slow` + `enable_latency_threshold_metrics: true` + `default_latency_threshold: 0.001s` + `latency_threshold_overrides: [{opcode: GetData, threshold: 3600s}]` (YAML list form; the proto enum value name is `GetData` — verify on the first reference `--mode validate` boot; if the reference rejects the spelling, consult the proto and record in PROGRESS.md).
- `l_rflags`: `stat_prefix: zk_rflags` + `enable_per_opcode_response_bytes_metrics: true`.
Every listener's chain is `[zookeeper_proxy, tcp_proxy]` → cluster `c_zk` (the shared TCPZKResponder backend).

The `AssertStats` expectation table (the load-bearing proof — every row asserted on BOTH sides):
```go
	expectations := []expect{
		// --- l_resp (latency + resp-bytes flags OFF) — arms 1–3 cumulative ---
		// Round-trips: request AND response counters (R5 — the 28.1 surface
		// re-proven through round-trips + the new _resp surface):
		{"zk_resp.zookeeper.connect_rq", 1}, {"zk_resp.zookeeper.connect_resp", 1},
		{"zk_resp.zookeeper.getdata_rq", 1}, {"zk_resp.zookeeper.getdata_resp", 1},
		{"zk_resp.zookeeper.create_rq", 1}, {"zk_resp.zookeeper.create_resp", 1},
		{"zk_resp.zookeeper.ping_rq", 1}, {"zk_resp.zookeeper.ping_resp", 1},
		{"zk_resp.zookeeper.close_rq", 1}, {"zk_resp.zookeeper.close_resp", 1},
		// Watch event (arm 2): the exists response correlates AND the push counts:
		{"zk_resp.zookeeper.exists_rq", 1}, {"zk_resp.zookeeper.exists_resp", 1},
		{"zk_resp.zookeeper.watch_event", 1},
		// Unknown xid + survival (arm 3):
		{"zk_resp.zookeeper.getacl_rq", 1}, {"zk_resp.zookeeper.getacl_resp", 0},
		{"zk_resp.zookeeper.decoder_error", 1},
		{"zk_resp.zookeeper.sync_rq", 1}, {"zk_resp.zookeeper.sync_resp", 1},
		// Flag-gating proof — latency metrics DISABLED on l_resp (arm 1 negative):
		{"zk_resp.zookeeper.connect_resp_fast", 0}, {"zk_resp.zookeeper.connect_resp_slow", 0},
		{"zk_resp.zookeeper.getdata_resp_fast", 0}, {"zk_resp.zookeeper.getdata_resp_slow", 0},
		// Flag-gating proof — resp-bytes flag OFF on l_resp (arm 7 negative):
		{"zk_resp.zookeeper.getdata_resp_bytes", 0},

		// --- l_fast (3600s default → ALL fast) — arm 4 ---
		{"zk_fast.zookeeper.connect_resp", 1}, {"zk_fast.zookeeper.connect_resp_fast", 1}, {"zk_fast.zookeeper.connect_resp_slow", 0},
		{"zk_fast.zookeeper.getdata_resp", 1}, {"zk_fast.zookeeper.getdata_resp_fast", 1}, {"zk_fast.zookeeper.getdata_resp_slow", 0},
		{"zk_fast.zookeeper.setdata_resp", 1}, {"zk_fast.zookeeper.setdata_resp_fast", 1}, {"zk_fast.zookeeper.setdata_resp_slow", 0},

		// --- l_slow (1ms default + ≥10ms responder delay → ALL slow; the
		//     GetData override 3600s → FAST) — arms 5–6 ---
		{"zk_slow.zookeeper.connect_resp_slow", 1}, {"zk_slow.zookeeper.connect_resp_fast", 0},
		{"zk_slow.zookeeper.setdata_resp_slow", 1}, {"zk_slow.zookeeper.setdata_resp_fast", 0},
		{"zk_slow.zookeeper.delete_resp_slow", 1}, {"zk_slow.zookeeper.delete_resp_fast", 0},
		{"zk_slow.zookeeper.getdata_resp_fast", 1}, {"zk_slow.zookeeper.getdata_resp_slow", 0}, // the override arm

		// --- l_rflags (resp-bytes flag ON) — arm 7 ---
		{"zk_rflags.zookeeper.getdata_resp", 1},
	}
```
Plus the cross-side EQUALITY assertions (present + equal + > 0 on both sides):
```go
	for _, metric := range []string{
		"zk_resp.zookeeper.request_bytes",
		"zk_resp.zookeeper.response_bytes",
		"zk_rflags.zookeeper.getdata_resp_bytes",
	} { /* the 0046 cross-side equality loop */ }
```

Package doc must record: the cross-side dispatch (ONE runner branch); the deterministic-threshold construction (D-P9: the fixed 10 ms responder delay + the extreme 3600s/1ms thresholds); the trigger-opcode encoding (D-S28.2-2); the R4 protocol (arm 8, `-count=1`).

- [ ] **Step 2: Add the runner blank-import**

In `test/differential/runner_test.go`, after the `0047` line (`:72`):
```go
	_ "github.com/esalaine/envoy-go/test/fixtures/0048-zookeeper-responses/driver"
```

- [ ] **Step 3: Run the fixture — expect GREEN on all arms (`-count=1`)**

```bash
go test ./test/differential/ -run 'TestDifferential/0048-zookeeper-responses' -v -count=1 2>&1 | tail -60
```
Expected: **PASS** — every fixed-value expectation + every cross-side equality holds on both sides. If RED, this is a production bug in Tasks 2–6 OR a driver/responder framing bug — use `superpowers:systematic-debugging`; the most likely divergence sources (check in order): (1) the reference's connect-response framing differs from the canned 20-byte form (probe the reference's stderr / counters with `FIXTURE_0048_DUMP_STATS=1`); (2) the `latency_threshold_overrides` YAML enum spelling; (3) the watch-event push arriving before the exists response on a slow scheduler (the responder writes them in order on one conn — TCP preserves it; both proxies see the same order). Do NOT modify assertion values to make it pass — every expectation above is SPEC §5.2-derived.

- [ ] **Step 4: R4 deliberate-break protocol (`-count=1` — `reference_differential_break_protocol_count1`)**

On the now-green baseline; per `reference_differential_asserter_dispatch` (prove the assertions are LIVE on the subject side):

1. **Break (a) — wrong expected value:** temporarily edit the driver's expectation `{"zk_resp.zookeeper.getdata_resp", 1}` → `2`. Run:
   ```bash
   go test ./test/differential/ -run 'TestDifferential/0048-zookeeper-responses' -v -count=1 2>&1 | tail -20
   ```
   Expected: **FAIL on BOTH the ref and subj sides** (`getdata_resp = 1, want 2`). Revert.
2. **Break (b) — production-side liveness:** temporarily comment out the `d.countResponse("connect", frame)` line in `onConnectResponse` (`decoder.go`). Run (same command, `-count=1` — a cached PASS here is exactly the trap the memory records). Expected: **FAIL** — the SUBJECT side reports `connect_resp = 0, want 1` on arm 1 while the reference still reports 1 (the cross-side divergence proves the subject-side assertion is live). Revert (verify `git diff internal/ test/` is empty after both reverts).

Record both break outputs + the reverts honestly in PROGRESS.md; summarize the protocol in the driver's arm-8 comment + the README.

- [ ] **Step 5: Author `test/fixtures/0048-zookeeper-responses/README.md`**

Document: the `[zookeeper_proxy, tcp_proxy]` → TCPZKResponder topology (and why TCPSink cannot serve this fixture — responses are the entire point); the four listeners / four stat_prefixes and which SPEC arm each carries; the deterministic-threshold construction (the fixed 10 ms responder delay + 3600s/1ms extreme thresholds + the GetData override — parent D-P9 / AMEND-A10); the trigger-opcode encoding (getacl wrong-xid / exists watch-push — D-S28.2-2); the 8-arm taxonomy with per-arm expected counters; the round-trip (read-before-next-write) driving discipline; the cross-side equality assertions (`request_bytes`/`response_bytes`/`getdata_resp_bytes`); the R4 deliberate-break record (both breaks + outputs + `-count=1`); R5 ratification (this fixture is the proof that the 28.1 correlation structures are consumed).

- [ ] **Step 6: No-regression spot check (`-count=1`)**

```bash
go test ./test/differential/ -run 'TestDifferential/(0001-tcp-proxy-rr|0046-zookeeper-requests|0047-zookeeper-boot-reject)' -v -count=1 2>&1 | tail -12
```
Expected: 3/3 PASS (`0046` is the request-side no-regression gate — the decoder rename + locking + the now-active OnWrite must not disturb it; the TCPSink backend never writes, so `0046`'s OnWrite path sees zero bytes).

- [ ] **Step 7: gofmt + lint + commit**

```bash
gofmt -l test/ ; golangci-lint run ./test/...
git add test/fixtures/0048-zookeeper-responses/ test/differential/runner_test.go \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 9: 0048-zookeeper-responses — 4-listener/8-arm cross-side fixture GREEN + R4 deliberate-break (-count=1) + README (SPEC 5.2; R4/R5)"
```

---

## Task 10: Completion bundle part 1 — ADR-0223 body + the BEHAVIOR_CONTRACT 28.2 bundle (SPEC §7.1/§7.2)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0223 §Decision/§Consequences body — IN PLACE per ADR-0044; tail STAYS ADR-0223)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: ADR-0223 §Decision + §Consequences body (after its §Context — the file currently ends at `:14343`)**

Land IN PLACE (ADR-0044 discipline; NO new ADR number — SPEC §2.7). The body covers:

- **§Decision — the unified decoder + the response dispatch (SPEC §3):** the `requestDecoder` → `decoder` rename (upstream single-DecoderImpl parity); the write-side `writeBuf` reassembly (fed by `OnWrite`; **NO write-side TotalAppended** — `writeChainConn.Write` allocates a fresh per-Write Buffer so every byte arrives exactly once; the asymmetry vs the read side's §3.3 re-base is STRUCTURAL, not an oversight); the leading-int32 dispatch table (connect 0 / watch −1 / control −2/−4/−8 / data >0 / unknown-negative → `decoder_error`); the correlate-then-validate order (upstream parity — a malformed-but-correlatable response consumes the entry and fires the flag-gated per-opcode decoder error); decode-failure = abandon-`writeBuf`-no-resync (AMEND-A8 symmetry); the shallow decode (zxid/error read past, never extracted — D-S28.2-5).
- **§Decision — correlation consumption (SPEC §3.4; R5 ratified):** data-map erase-on-lookup (double response → `decoder_error`); control FIFO pop (empty queue → `decoder_error`); **the connect_readonly → connect response-opname mapping** (the closed-roster panic trap defused; upstream onConnectResponse parity); the 28.1 "control queues grow unbounded" boundary REWRITTEN as upstream-parity behavior (responses now drain both structures; the residual unanswered-request growth is upstream's behavior too).
- **§Decision — THE PER-CONNECTION MUTEX (SPEC §3.6 — discharges the ADR-0221 §Consequences forward-pointer):** one `sync.Mutex` on the decoder guarding EXACTLY the two correlation maps; reassembly buffers + `chainConsumed` single-goroutine-owned and lock-free; entries copied out under the lock; counter increments + latency math outside the lock; the pre-handoff request path locks too (uniformity); `OnDestroy` needs no lock (runs strictly after both pumps join — the ADR-0221 happens-after edge); proven by the §3.6 race test (`-race -count=5`) + the live 0048 concurrent pumps. **State explicitly: the forward-pointer pinned in ADR-0221 §Consequences ("the 28.2 SPEC MUST add synchronization") is discharged EXACTLY as anticipated.**
- **§Decision — latency-threshold counters (SPEC §4):** `latency <= threshold` → fast (INCLUSIVE — AMEND-A10); wire-opcode-keyed overrides (consumed from the 28.1-parsed `latencyThresholdOverrides` map); flag gates increments; watch events + decoder errors never get fast/slow; **the deterministic-threshold differential discipline** (extreme 3600s/1ms thresholds + the TCPZKResponder fixed 10 ms delay — parent D-P9 resolved as the `TCPZKResponder BackendKind = 29`).
- **§Decision — the proof surface:** fixture `0048` (4 listeners / 8 arms; the trigger-opcode encoding getacl/exists — D-S28.2-2); the 38th fuzzer (D-P6 resolved: separate `FuzzZookeeperResponseDecode`); the latency PARSE-REJECT arms stay unit-test-only (D-P4 resolved).
- **§Consequences:** phase 28 closes at both-direction COUNTER parity (the BRAINSTORM Q1 envelope); the latency-HISTOGRAM family (`connect_response_latency`/`<opname>_latency`/`unknown_opcode_latency`) stays DEFERRED (ADR-0060 coverage boundary); the response-side shallow-decode leniency departure (a valid-header/malformed-payload response counts `<op>_resp` on envoy-go vs `decoder_error` upstream; the fixture corpus contains no such frames); counts at phase-28 close (fixtures 50 / fuzzers 38 / stat surface 337 / DECISIONS tail ADR-0223, next-free ADR-0224 — phase 28 consumed exactly its three BRAINSTORM-locked numbers); **the parent-row-28 ROLLUP** (the THIRD §9 Network-filters-family row closes; 4 candidates remain; `mongo_proxy` is the natural next + the anticipated consumer #2 of the ADR-0221 seam).
- **Cross-references:** ADR-0221 (the discharged forward-pointer), ADR-0222 (the consumed R5 structures + latency fields), ADR-0060, ADR-0044/0045/0052, the 28.2 SPEC §3–§7, the parent SPEC §11.4/§11.5/§11.7, project memories (`reference_differential_break_protocol_count1`, `reference_differential_asserter_dispatch`, `reference_differential_fixture_dispatch_constraint`).

Verify after the edit:
```bash
grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -1   # still ADR-0223
```

- [ ] **Step 2: The BEHAVIOR_CONTRACT 28.2 bundle (ONE atomic edit per ADR-0052)**

Six edits to `docs/envoy-go/BEHAVIOR_CONTRACT.md` (anchors from Task 1 Step 4 — re-pin if drifted):

1. **The `### envoy.filters.network.zookeeper_proxy` subsection (`:3627-3654`) EXTENDS with the response side:** the response dispatch table (§3.3 — connect/watch/control/data/unknown rows + min lengths); the correlation-consumption semantics (erase-on-lookup; FIFO pop; correlate-then-validate; empty/missing → `decoder_error`); **the connect_readonly→connect response mapping** (upstream parity; the closed-roster pin); the latency fast/slow semantics (`<=` inclusive; wire-opcode-keyed overrides; flag-gated; watch events/errors never); the response-side shallow-decode leniency departure (§2.2); the watch-event semantics (never correlated; `watch_event` + `response_bytes` only); the byte-accounting rules (response_bytes ungated; `*_resp_bytes` flag-gated). Mark all as cross-side-PROVEN by the green `0048`.
2. **The conn-wrap-seam block (`:3656-3686`): the 28.2 forward-pointer is RESOLVED** — the per-connection decoder mutex is recorded as the landed synchronization (goroutine A request decode vs goroutine B response decode; the maps-only lock scope; the race test + 0048 ratification). Replace the forward-pointer sentence with the landed fact.
3. **The latency-HISTOGRAM coverage-boundary record (ADR-0060):** `connect_response_latency` + `<opname>_latency` + `unknown_opcode_latency` unmirrored (the fast/slow counters are the deterministic stand-in).
4. **The 28.1 "control queues grow unbounded" boundary is REWRITTEN** as upstream-parity behavior (§3.4 item 5).
5. **The proto-vs-wire enum note (the SPEC §7.2 spec-reviewer advisory):** an explicit note distinguishing the **27-value proto `Opcode` enum** (keys `latency_threshold_overrides` config-side via `protoToWireOpcode`) from the **26-value gapped wire-opcode enum** (what the decoder dispatches on) — the two rosters differ by design (AMEND-A6).
6. **`### Stat surface` (`:3688`) + `### Applies to` (`:3697`) + `### Does not yet apply to` (`:3703`):** the stat-surface narrative gains the 28.2 sentence (**stays 337** — increments only, zero creation delta; fixtures 49 → 50; fuzzers 37 → 38); the stat-mapping block at `:464` gains a one-line "incremented at 28.2" annotation for the response-side families; the `### Does not yet apply to` bullet at `:3704` ("Response-side decode + latency counters … 28.2 / ADR-0223") MOVES to `### Applies to` (rewritten as the landed fact); the parent-row-28 family-close note (the THIRD §9 row done; 4 candidates remain — redis/mongo/kafka_broker/thrift; mongo_proxy the natural next).

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 10: completion bundle — ADR-0223 body in place (response decoder + per-connection mutex + latency + rollup); BEHAVIOR_CONTRACT 28.2 bundle (response side + seam forward-pointer resolved + histogram boundary) [ADR-0223]"
```

---

## Task 11: Six-gate + STATE.md + the ROADMAP ATOMIC rollup + next-prompt.txt (SPEC §7.3/§12.2)

**Files:**
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`
- Modify: `docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md`

- [ ] **Step 1: The six-gate (SPEC §12.2) — run LIVE, quote into PROGRESS.md**

```bash
go build ./...                                 # gate 1: clean
go vet ./...                                   # gate 2: clean
golangci-lint run                              # gate 3: clean (whole repo)
go test ./... -race -short -count=1            # gate 4: green (all packages; the locked correlation paths under -race)
# gate 5: FULL differential suite — 50 dirs, -count=1 (the 49-dir no-regression gate + 0048)
go test ./test/differential/ -run TestDifferential -v -count=1 2>&1 | tail -90
# gate 6: h2spec 53/53 + proxy-wasm 10/10 (asserted-unaffected — 28.2 touches no HTTP
#   path; re-run LIVE since the harness is available)
go test ./test/conformance/h2spec/ -run TestH2Spec -v -count=1
go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v -count=1
```
Expected: gates 1–4 clean/green; gate 5 **50/50 PASS** (49 pre-existing + `0048`); gate 6 **53/53** + **all 10 families PASS**. All outputs quoted honestly into PROGRESS.md (per `superpowers:verification-before-completion`), including any `freeTCPPort` TOCTOU bind flakes — re-run flaked dirs in isolation (`-count=1`) and record both runs (the 28.1a/28.1b closure precedent; a flake is NOT a regression, but it must be recorded, not hidden).

Confirm + quote the final counts (R6): fixture dirs **50 active** (tail `0048-zookeeper-responses`); fuzzers **38**; stat table **337** (unchanged); DECISIONS tail **ADR-0223** / next-free **ADR-0224** (both unchanged — phase 28 consumed exactly its three BRAINSTORM-locked numbers).

- [ ] **Step 2: The ROADMAP ATOMIC rollup (SPEC §7.3 — the 18/19/22/24/25/26 final-sub-phase precedent)**

In `docs/envoy-go/ROADMAP.md`, in ONE edit (both rows flip in the SAME commit):

- Sub-row **28.2** (`:85`): `in-progress → done` + append the IMPL-DONE note (the response decoder + the per-connection mutex [the ADR-0221 forward-pointer discharged] + latency fast/slow + `TCPZKResponder`/`0048` GREEN + R4 + the 38th fuzzer; counts 49→50 fixtures / 37→38 fuzzers / 337 stats unchanged; ADR-0223 body landed; six gates green).
- Parent row **28** (`:82`): `in-progress → done` + the final-counts summary appended to the row's notes (the 26-parent-row template): phase 28 complete at both-direction COUNTER parity — 28.1a (write seam + request package) + 28.1b (read seam + cross-side proof) + 28.2 (response decoder + latency + rollup); final counts fixtures 47 → 50, stat surface 136 → 337, fuzzers 36 → 38, ADRs 0221/0222/0223 all bodies landed (next-free ADR-0224); the §9 Network-filters family's THIRD row closes; **4 family candidates remain** (`redis`/`mongo`/`kafka_broker`/`thrift`); `mongo_proxy` is the natural next (consumer #2 of the conn-wrap seam).
- If ROADMAP's `### Network filters family` section (`:97`) carries a candidate list, update it to the 4 remaining candidates.

- [ ] **Step 3: STATE.md advance**

- `active-phase`: → `phase 28 DONE (28.1a + 28.1b + 28.2 all done; parent row rolled) — awaiting next phase brainstorm` + the summary paragraph (what 28.2 landed; what phase 28 delivered as a whole).
- `phase-directory`: the 28.2 dir now holds README/SPEC/PLAN/PROGRESS (+ REVIEW.md if the review stage produced one).
- `lifecycle-state`: **SKILL_ROUTING state 0** (no phase in flight → `superpowers:brainstorming` for the next phase) — unless the user directs otherwise at the closing session.
- `next-skill`: `superpowers:brainstorming` (the next §9 family candidate or any other ROADMAP row; `mongo_proxy` is the natural next but the choice is the brainstorm's).
- `last-commit`: filled at squash (the controller fills the squash SHA post-merge).
- Counts: fixtures **50** (tail `0048`), fuzzers **38**, stats **337**, DECISIONS tail **ADR-0223**, next-free **ADR-0224**.

- [ ] **Step 4: next-prompt.txt rewrite (the next-phase cold-start)**

Rewrite for the next session: phase 28 DONE (all three sub-phases + the parent row rolled) + squash-merged + pushed; no phase in flight; the session runs SKILL_ROUTING **state 0** (`superpowers:brainstorming` for the next phase — candidates: the 4 remaining §9 Network-filters rows [`mongo_proxy` natural next as conn-wrap-seam consumer #2] or any other ROADMAP family); the read-first list (STATE.md, ROADMAP.md §9 family sections, the phase-28 family of SPECs/PROGRESS for the as-built zookeeper/seam surface, DECISIONS ADR-0221/0222/0223); the carried coverage boundaries (histograms ADR-0060; dynamic metadata AMEND-A9; the tcp_proxy `downstream_cx_*` family; the post-handoff observational boundaries); the counts at open (50 fixtures / 38 fuzzers / 337 stats / tail ADR-0223 / next-free ADR-0224).

- [ ] **Step 5: Commit + the stage-close handoff**

```bash
git add docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt \
  docs/envoy-go/phases/28.2-network-filter-zookeeper-responses-and-latency/PROGRESS.md
git commit -m "phase 28.2 Task 11: six gates GREEN LIVE (50/50 differential -count=1; h2spec 53/53; proxy-wasm 10/10); ROADMAP ATOMIC rollup — sub-row 28.2 AND parent row 28 -> done; STATE + next-prompt for the next-phase cold-start [ADR-0223]"
```

**Controller stage-close (NOT a subagent step):** per `feedback_push_to_origin` + the project squash discipline — squash-merge the worktree branch to master with the phase commit message format `phase 28.2: <title> [ADR-0223]`, push to origin, fill the squash SHA into STATE.md `last-commit` (+1 docs commit), repoint next-prompt.txt's master-tip reference (+1 docs commit), push again.

---

## Test surface summary (SPEC §12.1)

- **Layer A — zookeeperproxy unit** (`decoder_test.go` + `zookeeperproxy_test.go`): the mechanical rename updates with assertions UNCHANGED (Task 2); write-side framing (watch event / short / oversized / unknown-negative / partial-frame reassembly across OnWrite calls / abandon-no-resync recovery / multi-frame single write — Task 3); correlation (erase-on-lookup / double response / missing xid / FIFO + underflow / connect response / **the connect_readonly→connect panic trap** / empty connect queue / correlate-then-validate truncation / structures-drain — Task 4); byte accounting (response_bytes ungated; `*_resp_bytes` flag-gated both ways — Task 4); latency (the `<=` inclusive edge with injected durations / override-beats-default / flag-off / end-to-end injected start / connect participation / never-for-watch-or-errors — Task 5); `OnWrite` feeds-decoder + Continue-always + never-mutates + fresh-Buffer-per-call reassembly (Task 6); **the §3.6 race test** (concurrent decodeOnData + decodeOnWrite, `-race -count=5`, conservation assertion — Task 6).
- **Layer C — fuzz**: the 38th fuzzer (`FuzzZookeeperResponseDecode` — no-panic incl. the closed-roster guard / bounded writeBuf / maps-never-grow; Task 7); the 37th re-runs unchanged (the rename is transparent — Task 2).
- **Layer D — differential**: the `TCPZKResponder` unit test (Task 8 — docker-free); `0048` all arms cross-side + R4 with `-count=1` (Task 9); the FULL 49-dir pre-existing suite (the `0046`/`0047` request-side no-regression gate) → **50/50** at the six-gate (Task 11).
- **Layer E — race**: `go test -race -short ./...` per task on touched packages + the dedicated Task-6 race test + the repo-wide gate-4 run (Task 11) — all now exercising the locked correlation paths.
- **Per-task hygiene**: `gofmt -l` + `golangci-lint run` on touched packages, every task (`feedback_pertask_gofmt_lint`).

## Acceptance checklist (SPEC §12.3 — verified at Task 11)

- [ ] The response decoder lands per SPEC §3 (dispatch + correlation + the connect-readonly mapping + decode-failure symmetry); `OnWrite` replaces the no-op; the chain `Buffer` is never mutated; `Continue` always returned (R3).
- [ ] The per-connection mutex lands per SPEC §3.6; the race test is green `-race -count=5`; ADR-0223's body records the synchronization (the ADR-0221 forward-pointer DISCHARGED).
- [ ] The latency fast/slow counters land per SPEC §4 (`<=` inclusive; wire-opcode-keyed overrides; flag-gated); the injected-duration boundary tests are green.
- [ ] `0048-zookeeper-responses` + `TCPZKResponder` land and are GREEN on all arms cross-side (`-count=1`); R4 recorded; R5 ratified; the fixture README authored.
- [ ] The 38th fuzzer lands; counts: **50** fixture dirs, **38** fuzzers, stat surface **337**, DECISIONS tail **ADR-0223** / next-free **ADR-0224** (R6).
- [ ] ADR-0223 §Decision/§Consequences body in place (no new ADR number); the BEHAVIOR_CONTRACT 28.2 bundle lands (SPEC §7.2) incl. the seam forward-pointer resolution + the histogram coverage boundary + the proto-vs-wire enum note.
- [ ] Six gates green LIVE (`-count=1`) + quoted into PROGRESS.md; STATE.md advanced to state 0; **ROADMAP sub-row 28.2 AND parent row 28 flip `→ done` ATOMICALLY**; next-prompt.txt rewritten for the next-phase cold-start.


