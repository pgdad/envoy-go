# Phase 29.3 IMPL — PROGRESS

Worktree: `.worktrees/phase-29.3-network-filter-mongo-fault-delay-and-access-log-impl`
Branch: `phase-29.3-network-filter-mongo-fault-delay-and-access-log-impl` (off master `2b2a9d8`)
Execution: `superpowers:subagent-driven-development` (fresh subagent/task + two-stage review; subagents commit LOCAL-ONLY; controller squash-merges + pushes at stage-close).

## Task 1 — baselines/anchors gate (DONE)

Confirmed at the IMPL-session tip `2b2a9d8`:

| anchor | value | status |
|---|---|---|
| differential fixtures | **53** (tail `0051-mongo-responses`) | ✓ → 54 at T11 (`0052`) |
| fuzzers | **39** | ✓ (no new fuzzer; seam `-race` test instead) |
| stat surface | **360** | ✓ (+0 at 29.3 — increment-wiring only) |
| DECISIONS tail | **ADR-0226** (next-free ADR-0227) | ✓ (§Decision/§Consequences body lands T12) |
| BackendKind tail | **TCPMongoResponder = 30** | ✓ (reused; no new BackendKind) |

As-built anchors verified (the seam + consumers extend these):
- `chain.go`: `chainRuntime` (L146); `newChainRuntime([]ReadFilter,...)` (L198); `NewChainRuntime([]NetworkFilter,...)` (L60); `runData` per-pass-stop (L341); `replayRead` (L389, currently returns nothing — T4 adds `parked bool`); `ContinueReading` two-context (L439: connHalted + resumeRequested); `terminalReady` (L231).
- `readconn.go`: `readChainConn` (L19); `newReadChainConn` (L24); `Read` (L34).
- `prefixconn.go`/`writeconn.go`: embed `net.Conn` as INTERFACE (the non-promoting trap — T10 forwarding methods MANDATORY); `newPrefixConn`/`newWriteChainConn`.
- `mongoproxy/codec.go`: **5** as-built request decoders — `decodeQuery`(L224)/`decodeInsert`(L396)/`decodeGetMore`(L419)/`decodeKillCursors`(L439)/`decodeCommand`(L459); response-side `decodeReply`(L552)/`decodeCommandReply`(L598). D-S29.3-5: eval at the 5 request decoders only.

Clean baseline: `go build ./...` clean; `go test ./internal/... -count=1` all green.

## Task checklist
- [x] T1 baselines/anchors gate + PROGRESS.md
- [x] T2 chainRuntime halt-state sync + pre-handoff hold (seam-first) — the `MayHalt` construction-time `haltable` gate + halt `sync.Mutex`/`sync.Cond` + `held`/`resumeReady`; synthetic `haltingFilter`; never-halting byte-identity proof.
- [x] T3 ContinueReading async-active third context — off-dispatch resume releases the holder; pre-handoff async-resume reaches `TerminalReady`.
- [x] T4 replayRead + readChainConn post-handoff withhold + R-HALT `-race` + break (`-count=1`) — the `replayHeld` park-state + the publication-fence-under-`haltMu` fix (review I1); `TestSeamRaceReplayPublication` `-race -count=20`.
- [x] T5 fault-delay decision in decoder — `maybeInjectDelay` at the AS-BUILT five request decoders + `takePendingDelay` + `rollPercent` (deterministic 100%) + the `delayPending` re-entrancy guard.
- [x] T6 fault-delay timer in filter — `MayHalt`; `time.AfterFunc`+`onDelayTimer`+`ContinueReading`; `delays_injected` at ARM; StopIteration-while-pending; `OnDestroy` cancel + the timer↔destroy `-race`.
- [x] T7 accesslog pluggable Formatter seam + mongo formatter — `Formatter func(any)` (default `Default(*Record)` → HCM byte-identical) + `MongoRecord`/`MongoFormat` + per-opcode goldens.
- [x] T8 mongo access-log sink + per-message emission — gated on `cfg.accessLog`; both directions; the gated-off no-emit test.
- [x] T9 DrainState accessor + cx_drain_close — `Draining()`/`CloseDirection()` on `ReadFilterCallbacks`; the GUARDED `SetDrainState(rt.dm)` typed-nil fix; reply-completion list-empty + `Connection().Close(FlushWrite)`; the unit value proof (`TestFilter_DrainCloseOnEmptyListWhenDraining` ==1 / `…NoDrainClose…` ==0).
- [x] T10 close-direction seam D-P4 + cx_destroy_* value parity — `tcp_proxy` pump-EOF-first → `chainRuntime.closeDir`; the EXPLICIT `SetCloseDirection` forwarding methods on `prefixconn.go`+`writeconn.go` (`TestCloseDirectionThroughWrapStack`); the `OnDestroy` direction-keyed increment.
- [x] T11 fixture 0052-mongo-fault-delay cross-side + R4 break (`-count=1`)
- [x] T12 completion bundle + parent-row-29 ROLLUP + six-gate (this commit — see the gate evidence below)

## Deliberate-break records (per reference_differential_break_protocol_count1)
(recorded at T4 R-HALT and T11 R4 — broken-FAIL + reverted-PASS, all `-count=1`)

### T4 R-HALT — the halt mutex is NECESSARY (proven by deliberate break)

Break applied: `holdUntilResume`/`releaseResume` bodies stripped of `haltMu.Lock()/Unlock()`
+ the cond (`haltCond.Wait/Broadcast` removed; `holdUntilResume` busy-spins `for rt.held {}`),
leaving UNLOCKED reads/writes of `held`/`resumeReady`.

BROKEN — `go test ./internal/filter/network/ -race -count=1 -run 'TestSeamRace'`:
```
==================
WARNING: DATA RACE
Read at 0x00c0001d03a0 by goroutine 9:
  ...network.(*chainRuntime).releaseResume()  chain.go:454
  ...network.(*callbacks).ContinueReading()   chain.go:587
  ...network.(*haltingFilter).OnData.func1()  chain_test.go:1160
Previous write at 0x00c0001d03a0 by goroutine 8:
  ...network.(*chainRuntime).holdUntilResume()  chain.go:438
==================
--- FAIL: race detected during execution of test
```
→ `-race` reports a data race on `held`/`resumeReady` ⇒ the mutex is NECESSARY. (`-count=1`
used to defeat Go's test-result cache, per reference_differential_break_protocol_count1.)

REVERTED — same command after restoring the `haltMu`-guarded bodies: `ok` (clean). (Also
`-race -count=20` clean.)

### T4 R-HALT (review I1) — the replay-state publication fence is NECESSARY

`TestSeamRaceAsyncResume` was REPLACED by `TestSeamRaceReplayPublication`: the original drove
only the `held`/`resumeReady` coordination (already covered by T2/T3), not the Task-4 replay
state (`replayIdx`/`replayHeld`/`buf`). The new test drives the REAL post-handoff pump cycle
(`readChainConn.Read` → `replayRead` UNLOCKED writes → `holdUntilResume` → the async
`ContinueReading` haltMu-GUARDED reads via `finishParkedReplayLocked`) with a re-arming halting
filter so consecutive parked passes overlap.

Strengthening this test SURFACED a real defect: `replayRead` published `rt.replayHeld = true`
OUTSIDE `haltMu`, while `ContinueReading` reads it UNDER `haltMu`. The halting filter arms its
async resume DURING `OnData` (before that write lands), so the resume goroutine's guarded read
raced the unguarded write. Fix: publish `replayHeld` under `haltMu` in `replayRead` (the
publication fence). Also extracted `finishParkedReplayLocked` (caller-holds-haltMu helper, I2)
and annotated the combined advance+reset `replayIdx = 0` (I3).

Liveness check (deliberate break — guard around the `replayHeld` read in `ContinueReading`
removed): `go test ... -race -count=1 -run 'TestSeamRaceReplayPublication'` → DATA RACE on the
guarded `replayHeld` read (chain.go ContinueReading) vs the fenced write in `replayRead`; FAIL.
REVERTED → clean. Strengthened test passes `-race -count=20`.

### T11 — fixture 0052-mongo-fault-delay cross-side (all arms green; the R4 break)

The cross-side proof of the whole 29.3 phase. Two listeners on each side (reference
Envoy v1.37.2 docker + envoy-go subprocess), filter chain [mongo_proxy, tcp_proxy]
→ the shared TCPMongoResponder backend (BackendKind 30 REUSED — no new kind):
- `mongo_d`  (delay {fixed_delay: 0.100s, percentage 100% HUNDRED}).
- `mongo_nd` (no delay).

Arms (`go test ./test/differential/ -run 'TestDifferential/0052' -count=1`):
1. fault-delay round-trip (pre + post handoff): ONE conn on mongo_d, OP_QUERY reqID 1
   → reply, OP_QUERY reqID 2 → reply. delay 1 fires PRE-handoff (read loop), delay 2
   POST-handoff (replayRead). `delays_injected{mongo_d} == 2` BOTH sides; both replies
   received. Timing NEVER compared.
2. seam non-perturbation (no-delay): plain OP_QUERY → reply on mongo_nd;
   `delays_injected{mongo_nd} == 0` BOTH sides (R1 live equivalence); reply received.
3. cx_drain_close: PRESENCE-DOWNGRADED (D-S29.3-8) — asserted present + == 0 both sides.
   See the downgrade note below.
4. cx_destroy_* VALUE parity (D-P4 CLOSED) on mongo_nd:
   (i)   LOCAL: OP_QUERY(withhold 7777) → no reply, conn open → DRIVER closes →
         `cx_destroy_local_with_active_rq{mongo_nd} == 1` BOTH sides.
   (ii)  REMOTE: OP_QUERY(remoteClose 7006; NEW mongoMarkerRemoteClose added to
         mongoRespondLoop — reads then `return`s/closes WITHOUT replying) → upstream
         EOF first → `cx_destroy_remote_with_active_rq{mongo_nd} == 1` BOTH sides.
   (iii) all-answered: plain OP_QUERY → reply (list empties) → close → NEITHER increments.
5. all-quiesced roster: `op_query_active == 0` both prefixes, both sides (gauge TYPE
   line == gauge); the counters at their asserted values.

Live stat dump (FIXTURE_0052_DUMP_STATS=1) — ref and subj BYTE-IDENTICAL:
```
delays_injected{mongo_d}=2  delays_injected{mongo_nd}=0
cx_destroy_local_with_active_rq{mongo_nd}=1  cx_destroy_remote_with_active_rq{mongo_nd}=1
cx_destroy_local_with_active_rq{mongo_d}=0   cx_destroy_remote_with_active_rq{mongo_d}=0
cx_drain_close{mongo_d}=0  cx_drain_close{mongo_nd}=0  op_query_active{*}=0
```
Result: `--- PASS: TestDifferential/0052-mongo-fault-delay` on BOTH runner paths.

#### cx_drain_close differential disposition — PRESENCE-DOWNGRADED (D-S29.3-8)

The differential `cx_drain_close` arm is DOWNGRADED to PRESENCE + exists-at-zero on
both sides. Reason: the stat fires only when a correlated reply EMPTIES the active-
query list WHILE the connection's callbacks report `Draining()==true` (filter.go
OnWrite). Driving that cross-side requires (a) the admin `/drain_listeners` POST and
(b) a reply-completion landing in the narrow draining window — but the driver's
drive phase has NO admin addr (the runner passes admin addrs only to AssertStats,
not to DriveSubjectMulti), and the reply-vs-drain ordering is not deterministically
reproducible across the docker reference + the subprocess subject. Per D-S29.3-8 this
is a sanctioned downgrade; the LOAD-BEARING ratification is the Task-9 UNIT value
proof (deterministic): `TestFilter_DrainCloseOnEmptyListWhenDraining` (==1) +
`TestFilter_NoDrainCloseWhenNotDraining` (==0). The phase is NOT blocked on a flaky
differential drain arm.

#### T11 R4 deliberate-break liveness (per reference_differential_break_protocol_count1)

All breaks + reverts run with `-count=1` (Go caches test results — without it a stale
PASS would hide the break). Each new assertion proven LIVE:

(a) `delays_injected{mongo_d}` expected 3 (when 2 armed):
    BROKEN — `go test ./test/differential/ -run 'TestDifferential/0052' -count=1`:
      `ref ...delays_injected{...mongo_d} = 2, want 3`
      `subj ...delays_injected{...mongo_d} = 2, want 3`  → FAIL BOTH paths.
    REVERTED (expected 2) → `--- PASS: TestDifferential/0052-mongo-fault-delay`.

(b) Task-10 `cx_destroy_*` switch commented out in filter.go OnDestroy:
    BROKEN — same command:
      `subj ...cx_destroy_local_with_active_rq{...mongo_nd} = 0, want 1`
      `subj ...cx_destroy_remote_with_active_rq{...mongo_nd} = 0, want 1`  → FAIL
      (subject-side only; the reference side stays correct — a true cross-side proof).
    REVERTED (`git restore`) → PASS.

(c) Task-9 `cx_drain_close` increment commented out (PRESENCE-downgraded → the UNIT
    test is the proof per the protocol):
    BROKEN — `go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_DrainClose|TestFilter_NoDrainClose' -count=1`:
      `TestFilter_DrainCloseOnEmptyListWhenDraining: cx_drain_close = 0, want 1` → FAIL
      (`TestFilter_NoDrainCloseWhenNotDraining` correctly stays PASS — it asserts 0).
    REVERTED (`git restore`) → `ok`.

Files: created test/fixtures/0052-mongo-fault-delay/{driver/driver.go,README.md};
modified test/differential/runner_test.go (0052 blank-import; mongoMarkerRemoteClose
const + the REMOTE-close case in mongoRespondLoop). fixture.go NOT modified
(BackendKind TCPMongoResponder = 30 reused). gofmt -l + golangci-lint on ./test/...
clean; fixture count 54.

## Task 12 — completion bundle + parent-row-29 ROLLUP + the six-gate (DONE)

Docs landed (this commit): ADR-0226 §Decision/§Consequences body IN-PLACE in `DECISIONS.md`
(DECISIONS tail STAYS ADR-0226 — no new number; next-free ADR-0227); the BEHAVIOR_CONTRACT
29.3 bundle (the `### envoy.filters.network.mongo_proxy` subsection extension — fault-delay /
access-log / cx_drain_close / cx_destroy_* value-parity / runtime-key bullets + the 29.3
Differential bullet + the Stats 360→360 line; the NEW `### Network filter chain framework —
async halt/resume (29.3 amendment)` subsection; the `## Stat-name mapping` 360→360 [29.3]
block + the family-ROLLUP note; the `### Stat surface` 29.3 paragraph; the `### Applies to`
29.x line); `ROADMAP.md` sub-row 29.3 `in-progress → done` AND parent row 29
`in-progress → done` ATOMICALLY (the ROLLUP — same commit); `STATE.md` advanced (active-phase /
lifecycle-state / next-skill = NEXT-phase BRAINSTORM [redis/kafka_broker/thrift] / counts
54/39/360/30/ADR-0226); `next-prompt.txt` rewritten for the next-§9-family BRAINSTORM
cold-start; `internal/filter/network/mongoproxy/doc.go` 29.x forward-pointers flipped to LANDED
(phase-29 CLOSED).

### The six-gate (run LIVE from the worktree root; EXACT outputs)

1. `go build ./...` → clean (exit 0).
2. `go vet ./...` → clean (exit 0).
3. `golangci-lint run` → clean (exit 0).
4. `go test ./... -race -short` → **PASS** (exit 0; 80 packages `ok`, 0 FAIL).
   NOTE: a first invocation reported a single transient `FAIL` that did NOT reproduce on the
   immediate re-run (exit 0, 80 ok) — a known HTTP-fixture startup/port-bind flake (the same
   class recorded at 26.2/28.1b/28.2), NOT a 29.3 regression; 29.3 touches no HTTP filter path.
5. `ls -d test/fixtures/[0-9]* | wc -l` → **54** (tail `0052-mongo-fault-delay`).
6. `go test ./test/differential/ -count=1` → **PASS** (exit 0): `ok  …/test/differential  177.503s`
   (full 54-dir byte-exact suite incl. the 53-dir R1 back-compat / seam non-perturbation gate).
   A separate `-v` run for per-subtest visibility showed 54 subtests run, `0052-mongo-fault-delay`
   `--- PASS (10.52s)`, with TWO transient HTTP-fixture flakes (`0020-http-ext-authz-http`,
   `0022-http-ext-proc-grpc` — gRPC/HTTP-auth backend TOCTOU startup races) that **both PASS in
   isolation** (`go test … -run 'TestDifferential/(0020-…|0022-…)$'` → exit 0; `0020 PASS 2.08s`,
   `0022 PASS 1.77s`). These are unrelated to 29.3 (no HTTP filter touched); the authoritative
   non-verbose `-count=1` suite run passed all 54 byte-exact.

### Conformance (re-run LIVE — NOT asserted; the harness is runnable in this env)

- h2spec: `go test ./test/conformance/h2spec/ -count=1` → **53 tests, 53 passed, 0 skipped,
  0 failed** (`h2spec conformance report: 53 total tests, 0 failures`).
- proxy-wasm: `go test ./test/conformance/proxy-wasm/ -count=1` → **PASS** (all 10 families:
  exports / security {allowed,denied} / runtime / wasm_vm / bytecode_util / logging /
  stop_iteration {pause,continue} / shared_data / pairs_util / endianness — all `--- PASS`).
  Rationale: 29.3 touches no HTTP filter; the accesslog `Formatter func(any)` seam keeps the
  HTTP `Default` path byte-identical (`*Record` satisfies `any`) — re-run LIVE to confirm.

### Counts at phase-done

fixtures **54** (tail `0052-mongo-fault-delay`); fuzzers **39** (no new fuzzer — the seam
concurrency is `-race`-test-proven); stat surface **360** (+0 creation — increment-wiring only);
BackendKind tail **30** (`TCPMongoResponder` reused); DECISIONS tail **ADR-0226** (next-free
**ADR-0227**). The parent-row-29 ROLLUP is ATOMIC (parent row 29 + sub-row 29.3 both `done` in
THIS commit). The §9 Network-filters family's FOURTH row CLOSES; 3 candidates remain
(redis / kafka_broker / thrift).
