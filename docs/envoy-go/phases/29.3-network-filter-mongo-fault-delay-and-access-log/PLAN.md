# Phase 29.3 PLAN — the async halt/resume seam + `mongo_proxy` fault-delay + the mongo access log + `cx_drain_close` + the close-direction seam

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended, per `feedback_execution_style`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Subagents commit **LOCAL-ONLY** (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Work in the git worktree (`feedback_git_worktrees`). Each task runs `gofmt -l` + `golangci-lint run` on the touched packages before its commit (`feedback_pertask_gofmt_lint`) — NOT just `go vet`. After ANY temporary deliberate break (the Task 4 seam `-race` break + the Task 11 R4 fixture break), use `go test -count=1` to defeat Go's result cache (`reference_differential_break_protocol_count1`). Cross-side fixtures MUST use `fixture.StatsAsserter` (`reference_differential_asserter_dispatch`); the responder backend emits CORRELATED bytes whose `responseTo` echoes the request `requestID` (`reference_wire_format_both_sides_see_same_bytes`). Wire-derived dynamic stat segments stay guarded by `stats.IsValidName` (`reference_dynamic_stat_name_charset_guard`) — already in place from 29.1, untouched here.

**Goal:** Land the framework's FIFTH structural extension — the **async halt/resume seam** (ACTIVE asynchronous `ContinueReading` from a `time.AfterFunc` goroutine + cross-goroutine safety behind an atomic/construction fast-path + post-handoff withhold-until-resume) — SEAM-FIRST and GREEN with its `-race` test BEFORE any mongo consumer wires in; then its first consumer, mongo **fault-delay injection** (`delays_injected` at timer-arm; deterministic 100%-probability arms); the **mongo access log** (a pluggable formatter seam on `internal/accesslog`; unit goldens + coverage boundary, NO fixture dir); **`cx_drain_close`** (reply-completion drain close, FlushWrite); CLOSE the deferred **close-direction seam** (D-P4 → `cx_destroy_local/remote_with_active_rq` VALUE parity); prove it cross-side with fixture **`0052-mongo-fault-delay`**; and land the **parent-row-29 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 29.3). Never-halting chains stay byte-identical (R1).

**Architecture:** Unlike 29.1/29.2 (framework-ZERO-touch), 29.3 is the **framework-SURGERY** sub-phase — the ONE consolidated ripple (ADR-0219) where the deferred framework work converges, precisely because the async halt/resume seam already opens the `chain.go` / `readconn.go` / `tcp_proxy`-pump area. `internal/filter/network/` gains: (i) a per-`chainRuntime` halt mutex + `sync.Cond` + a construction-time `haltable` fast-path gate so never-halting chains pay no cost (R1); (ii) an `MayHalt()`-declared opt-in (mongo with a `delay` configured) that turns an `OnData`/replay `StopIteration` into a HOLD (block the dispatching goroutine until `ContinueReading`) instead of the back-compat per-pass stop; (iii) an async-active `ContinueReading` that advances past the halting filter, re-dispatches, and re-evaluates terminal readiness (pre-handoff) or releases the held pump (post-handoff); (iv) a minimal **close-direction** recording (`tcp_proxy` pump-EOF-first → `chainRuntime.closeDirection`) + a `CloseDirection()` callbacks accessor; (v) a minimal **drain-state** signal (`DrainState` structural interface → `chainRuntime` → a `Draining()` callbacks accessor). The `mongoproxy` package consumes all of it: fault delay (`codec.go` roll+duration / `filter.go` timer + `ContinueReading`), the access log (a formatter-seam'd `internal/accesslog` sink), `cx_drain_close` (`Draining()` + `Connection().Close(FlushWrite)`), and the close-direction-keyed `cx_destroy_*` increment. `internal/accesslog` gains a pluggable `Formatter` (HCM callers byte-identical). `TerminalFilter.Handle` signature UNCHANGED; HCM untouched; the `ReadFilter`/`WriteFilter`/`Status` iteration contracts unchanged (§2.1).

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy v1.37.2 (ADR-0008); go-control-plane v1.32.4 (ADR-0008). Extends the as-built `internal/filter/network/mongoproxy/` package (29.1 request + 29.2 response) IN PLACE; extends `internal/filter/network/` (`chain.go`/`readconn.go`/`callbacks.go`/`types.go`), `internal/filter/tcpproxy/filter.go`, `internal/listener/manager.go`, and `internal/accesslog/` (`writer.go` + a new mongo formatter); consumes `internal/drain/` (`IsDraining()` — read, not modified) + `internal/filter/http/fault/fault.go` (the `rollPercent`/`time.AfterFunc`/`markedActive` precedent) + the differential harness + `fixture.StatsAsserter` + the `0049`/`0051` scrape helpers + the `0051` `TCPMongoResponder` (BackendKind 30, reused). ZERO new third-party `go.mod` dependencies.

---

## ADR-0045 split-gate FINAL re-check (at PLAN time, per SPEC §12.1 + parent §3.0)

The gate fires at `> ~25 tasks OR > ~1500 net-new production LoC`. This PLAN decomposes to **12 tasks** / **~420–760 production LoC** (the SPEC §12.1 envelope, re-confirmed at PLAN time on the 26.x/28.2/29.2 accounting basis — fixture drivers + unit tests excluded):

| Deliverable | Production LoC | Tasks |
|---|---|---|
| The async halt/resume seam (`chain.go`/`readconn.go` — halt mutex/cond + `haltable` gate + hold/resume coordination + async-active `ContinueReading` + post-handoff withhold) | ~150–280 | 2–4 |
| The drain + close-direction callbacks accessors (`callbacks.go`/`types.go`/`manager.go`/`tcpproxy/filter.go`) | ~40–80 | 9–10 |
| Fault-delay injection (`codec.go` roll+duration + `filter.go` timer/resume/cancel) | ~90–150 | 5–6 |
| The access-log formatter seam (`internal/accesslog/`) + the mongo formatter + sink construction | ~110–190 | 7–8 |
| `cx_drain_close` (the list-empty check + the close) + the `cx_destroy_*` direction-keyed increment | ~30–60 | 9–10 |
| **Total (production basis)** | **~420–760** | **12** |

Both axes well under the gate (12 ≤ ~25 tasks; ~760 ≤ ~1500 LoC) → **NO split. 29.3 proceeds as ONE sub-phase** (the 28.2/29.2 single-sub-phase precedent; squarely in the parent §11.11/§15 estimate ~470–780). The `0052` driver (~450–600 LoC; the `0051` precedent) is excluded per the accounting precedent. The pre-authorized 29.3 split axis (parent §3.0) STANDS-UNCONSUMED.

---

## PLAN-time D-question dispositions (SPEC §10.2)

The SPEC pins the seam SEMANTICS and resolves D-P5/D-P7/D-P12; this PLAN pins the concrete DESIGN for D-S29.3-1..8. Where the SPEC marks a point "IMPL transcribes against `proxy.cc`/`codec_impl.cc` v1.37.2", the PLAN code blocks are the faithful default the IMPL verifies, NOT a re-derivation.

- **D-S29.3-1 (the halt-seam synchronization primitive) — RESOLVED at PLAN: a per-`chainRuntime` halt `sync.Mutex` + `sync.Cond` (block-the-dispatcher) behind a construction-time `haltable bool` fast-path gate.** A read filter opts into halting by implementing the new optional `network.HaltingReadFilter` interface (`MayHalt() bool`); `NewChainRuntime` sets `rt.haltable` true iff some read filter `MayHalt()` returns true (mongo with a `delay` configured). For a **non-haltable** chain (`rt.haltable == false` — zookeeper, every existing filter, mongo with NO `delay`) the halt/cond code is bypassed entirely → the byte-identical pre-29.3 path (R1; the 53-dir suite is the regression gate). For a **haltable** chain, an `OnData`/replay `StopIteration` becomes a HOLD: the dispatching goroutine blocks on the cond (`holdUntilResume`) until the timer's `ContinueReading` releases it. The hold↔resume race (the timer may fire before the holder blocks) is closed by a `resumeReady` flag under the halt mutex (classic cond-with-pending-flag — no missed wakeup). The exact primitive (mutex+cond vs a release channel) MAY be refined at IMPL if the `-race` test demands; the cond form below is the pinned default. `haltable` is set ONCE at construction (immutable thereafter → race-free without an atomic; the SPEC's "atomic fast-path" is satisfied by the construction-time immutable read).
- **D-S29.3-2 (async-resume terminal handoff) — RESOLVED at PLAN: the dispatcher (read loop pre-handoff / pump post-handoff) BLOCKS at the hold, so resume re-enters via that SAME goroutine; no handoff is driven from the timer goroutine.** Pre-handoff, `runData` blocks inside the read-loop goroutine at `holdUntilResume`; the timer's `ContinueReading` releases it; `runData` then advances `resumeIdx` past the halting filter and returns; `serveNetworkChain` (unchanged) observes `TerminalReady()` and performs the handoff. The held request bytes (mongo never drains — R3) replay to the terminal via the existing `prefixConn` in `handleTerminal`. Post-handoff, the pump goroutine blocks at `holdUntilResume` inside `readChainConn.Read`; resume releases it; the bytes flow upstream. The common `0052` first-message delay fires pre-handoff (the handoff is the read loop's, after resume); later-message delays fire post-handoff. **`serveNetworkChain` and `handleTerminal` are NOT modified** (the timer never touches the conn).
- **D-S29.3-3 (the callbacks accessor surface — drain + close-direction) — RESOLVED at PLAN: TWO minimal accessors on `ReadFilterCallbacks` (`Draining() bool` + `CloseDirection() CloseDirection`), both backed by the per-connection `chainRuntime`, threaded in ONE folded callbacks ripple.** The drain decider reaches the chain as a narrow structural `network.DrainState interface { IsDraining() bool }` (the `network` package does NOT import `internal/drain`; `*drain.Manager` satisfies it structurally) set via `ChainRuntime.SetDrainState(d)` from `serveNetworkChain` (which holds `rt.dm`) — NOT a `*drain.Manager` leak into the framework's public API, and NOT a `NewChainRuntime` signature change (existing test callers stay green). The close direction is recorded onto the chain by `tcp_proxy` (§3.5) and read via `CloseDirection()`. **(Rationale for NOT using the factory-capture precedent for drain:** the close-direction MUST traverse the chain callbacks regardless — it is per-connection runtime state recorded by the framework — so folding drain onto the same callbacks surface is the SPEC-stated one-ripple design; the mongo filter stays framework-decoupled, consuming `f.cb.Draining()`/`f.cb.CloseDirection()`.)
- **D-S29.3-4 (the access-log mongo carrier) — RESOLVED at PLAN: a typed `Formatter func(any) []byte` on `AsyncFileSink` (default `Default`-over-`*Record`) + a mongo-owned record type + a mongo formatter.** Add `type Formatter func(rec any) []byte` and an optional `formatter Formatter` field to `AsyncFileSink` (zero value → the existing `Default(*Record)` adapter → every existing HCM caller byte-identical). `Submit` accepts `any`. The mongo sink is constructed with the mongo formatter; the mongo record is a small `mongoproxy`-package struct. The HTTP `*Record` path is UNCHANGED (the regression gate: the existing accesslog + HCM tests stay byte-identical). This is option (a)/(b) hybrid (a typed-payload formatter over `any`), the least-invasive that keeps `Default` byte-stable.
- **D-S29.3-5 (the fault-eval callback set) — INHERITED verbatim: the parent §11.6 six callbacks `decodeQuery`/`decodeInsert`/`decodeGetMore`/`decodeKillCursors`/`decodeCommand`/`decodeCommandReply` (NOT `decodeReply`).** The PLAN places the `maybeInjectDelay()` eval at the entry of each of those six decode callbacks (Task 5). `0052` exercises OP_QUERY (the load-bearing arm); the IMPL transcribes the exact callback set against `proxy.cc` v1.37.2 rather than re-deriving (`reference_wire_format_both_sides_see_same_bytes`).
- **D-S29.3-6 (the fault filter↔decoder split + the guard) — RESOLVED at PLAN: the DECODER decides (roll + duration → `pendingDelay`/`delayDecided`); the FILTER owns the `time.AfterFunc` + `cb`; the cross-goroutine re-entrancy guard is `decoder.delayPending atomic.Bool` (set by the filter at arm, cleared by `onDelayTimer` at fire); the timer-cancel is best-effort `timer.Stop()` at `OnDestroy` (the phase-09 `markedActive` precedent — no double-count risk because `delays_injected` increments once at arm on the read goroutine).** `decodeOnData`/`replayRead` stops decoding further buffered messages once a delay is decided this pass (at-most-one delay per pass — the re-entrancy guard).
- **D-S29.3-7 (the omitted zero-ms drain timer) — RESOLVED at PLAN: yes, envoy-go's deferred close subsumes it.** `Connection.Close(FlushWrite)` records `closeReq`/`closeType` on the runtime; the close happens after the current pass (the existing deferred-close discipline), structurally subsuming upstream's zero-ms dispatcher timer. Confirmed no observable `0052`-arm difference at IMPL (Task 9 unit test + the `0052` drain arm).
- **D-S29.3-8 (the `0052` drain-trigger mechanism) — RESOLVED at PLAN: the fixture drives a listener drain via the admin `/drain_listeners` endpoint (the phase-08.2 vehicle), with a PRESENCE + unit-value FALLBACK if the cross-side drain timing is not deterministically controllable.** Treated as a real risk (the spec-reviewer flagged it): Task 9 lands the `cx_drain_close` UNIT value proof FIRST (deterministic, both the drain-active→increment and not-draining→no-close cases); Task 11 then attempts the differential `cx_drain_close` arm via admin `/drain_listeners` on both sides; if the arm is not reliably reproducible cross-side, it DOWNGRADES to a PRESENCE-only assertion (the stat exists at its value-proven-by-unit-test level) with the boundary recorded in the `0052` README + PROGRESS.md. The unit value proof is the load-bearing ratification (R-DRAIN); the differential arm is best-effort.

---

## The seam design in one picture (the load-bearing mental model for Tasks 2–6)

```
Production chain: [mongo_proxy (ReadFilter+WriteFilter, MayHalt iff delay configured), tcp_proxy (Terminal)]
                  rt.filters = [mongo]; rt.terminal = tcp_proxy; rt.writeFilters = [mongo]; rt.haltable = cfg.delayConfigured

PRE-HANDOFF (first message; handoff is BLOCKED by the delay until resume):
  serveNetworkChain → OnData → runData(i=0)
    mongo.OnData: decode → maybeInjectDelay rolls 100% → delayDecided; takePendingDelay → arm timer; return StopIteration
    runData: StopIteration && haltable && !resumeRequested → holdUntilResume()  ── BLOCKS the read-loop goroutine
       ...~100ms... timer fires (AfterFunc goroutine) → onDelayTimer → cb.ContinueReading() → release (held=false, Broadcast)
    runData wakes → resumeIdx = 1 → loop exits (1 >= len(filters)) → return
  serveNetworkChain: TerminalReady() (resumeIdx>=len) → HandleTerminal → held bytes replay via prefixConn → tcp_proxy pumps

POST-HANDOFF (later messages; steady state — mongo decode runs on the downstream-pump goroutine via replayRead):
  pump A: readChainConn.Read → r.Conn.Read(b) → replayRead(b) 
    mongo.OnData (inside replayRead): decode → roll → arm timer → StopIteration → replayRead PARKS (replayIdx=0), returns parked=true
  readChainConn.Read: parked → holdUntilResume()  ── BLOCKS the pump goroutine (withhold the bytes upstream)
       ...~100ms... timer fires → ContinueReading() → advance replayIdx past mongo + drain + release → pump wakes
  readChainConn.Read returns b[:n] → io.Copy forwards upstream (the delayed request completes)

NON-HALTABLE (zookeeper / every existing filter / mongo-no-delay): rt.haltable==false → runData/replayRead/Read take the
  byte-identical pre-29.3 path; the halt mutex/cond are never touched. (R1 — the 53-dir suite is the regression gate.)
```

---

## File Structure

**Created:**
- `internal/accesslog/mongo_format.go` — the mongo JSON access-log formatter (`{"time","message","upstream_host"}`) + the per-opcode `message.toString` shapes (transcribed against `codec_impl.cc` v1.37.2 at IMPL). (May instead live in `mongoproxy/accesslog.go` if the carrier ends up mongo-package-local — Task 7 decides; the PLAN places it in `internal/accesslog/` beside `format.go`.)
- `internal/accesslog/mongo_format_test.go` — the formatter unit goldens (request `full=true` / reply `full=false`; the `time` field asserted by shape, not value).
- `test/fixtures/0052-mongo-fault-delay/driver/driver.go` — the cross-side fault-delay `StatsAsserter` fixture (the delayed + no-delay listeners; the pre/post-handoff delay arms; the `cx_drain_close` arm; the `cx_destroy_*` VALUE-parity arms; the R4 break). The `0049` `MultiListenerDriver` + the `0051` `TCPMongoResponder` + the `scrapeMongoStats`/`scrapeTypeLine`/`canonicalize`/`httpGet` helpers are the structural template (reused verbatim).
- `test/fixtures/0052-mongo-fault-delay/README.md` — the fixture envelope + the R4 deliberate-break record + the access-log no-fixture note + the `cx_drain_close` drain-trigger disposition (D-S29.3-8).
- `PROGRESS.md` (worktree root, Task 1) — the per-task six-gate evidence log (run honestly; the seam `-race` break + the R4 break recorded with `-count=1` output).

**Modified:**
- `internal/filter/network/types.go` — `type HaltingReadFilter interface { ReadFilter; MayHalt() bool }`; `type DrainState interface { IsDraining() bool }`; `type CloseDirection int` + `CloseDirectionUnset/Local/Remote`.
- `internal/filter/network/callbacks.go` — `ReadFilterCallbacks` gains `Draining() bool` + `CloseDirection() CloseDirection` (minimal additions; the iteration protocol — `Status`/`OnData` — is UNCHANGED).
- `internal/filter/network/chain.go` — `chainRuntime` gains `haltable bool` + `haltMu sync.Mutex` + `haltCond *sync.Cond` + `held bool` + `resumeReady bool` + `replayIdx int` + `closeDir atomic.Int32` + `drain DrainState`; `NewChainRuntime` sets `haltable` (the `MayHalt()` scan); `runData` honors the haltable hold; `replayRead` honors the haltable hold (returns `parked bool`); `ContinueReading` gains the async-active third context; `holdUntilResume`/`releaseResume` helpers; `setCloseDirection`/`closeDirection` + the `callbacks.Draining()/CloseDirection()` impls; `ChainRuntime.SetDrainState`.
- `internal/filter/network/readconn.go` — `readChainConn.Read` post-handoff withhold-until-resume (behind the `rt.haltable` gate; byte-identical when not haltable) + the `SetCloseDirection` setter (the innermost wrap; relayed inward by the prefixConn/writeChainConn forwarding methods — Task 10).
- `internal/filter/network/prefixconn.go` + `internal/filter/network/writeconn.go` — FORWARDING `SetCloseDirection` methods (the embedded `net.Conn` interface does not auto-promote the custom method; Task 10).
- `internal/filter/tcpproxy/filter.go` — record which pump EOF'd first → `downstream.(interface{ SetCloseDirection(...) })` (additive; no behavior change for any chain; §3.5).
- `internal/listener/manager.go` — `serveNetworkChain` calls `rtChain.SetDrainState(rt.dm)` right after `NewChainRuntime` (the `dm` already on `rt`).
- `internal/accesslog/writer.go` — the pluggable `Formatter func(any) []byte` + the optional `formatter` field on `AsyncFileSink` (default → `Default(*Record)` adapter); `Submit(any)`; the `run()` call site uses `s.formatter`.
- `internal/filter/network/mongoproxy/config.go` — (no parse change — `delay`/`access_log` already parsed at 29.1; 29.3 consumes them) the `compiledConfig` comment forward-pointers flipped to LANDED.
- `internal/filter/network/mongoproxy/codec.go` — `decoder` gains `pendingDelay time.Duration` + `delayDecided bool` + `delayPending atomic.Bool` + (the access-log) the per-message record emission hook; `maybeInjectDelay()` at the six decode-callback entries; `takePendingDelay()`; `decodeOnData`/`replayRead` (via the filter) stop after a decided delay; `onDestroy()` returns the residual count.
- `internal/filter/network/mongoproxy/filter.go` — `MayHalt()`; the fault-delay timer (`delayTimer`/`onDelayTimer`/`ContinueReading`; `delays_injected` at arm; StopIteration-while-pending in `OnData`/`OnWrite`-no the response never halts/the replay path via `OnData`); the access-log sink construction (gated on `cfg.accessLog`) + per-message emission both directions; the `cx_drain_close` `Draining()` + `Connection().Close(FlushWrite)`; the `OnDestroy` close-direction-keyed `cx_destroy_*` increment + timer cancel; `dm`/sink fields on the filter (sink shared via the factory; `cb`/`wcb` already stored).
- `internal/filter/network/mongoproxy/stats.go` — (no roster change — `delays_injected`/`cx_drain_close`/`cx_destroy_*` already eager; 29.3 calls `ms.inc(...)`).
- `internal/filter/network/mongoproxy/doc.go` — the package-doc 29.3 forward-pointers flipped to LANDED (fault/log/drain/close-direction LANDED; phase-29 CLOSED).
- `internal/filter/network/mongoproxy/{codec,filter}_test.go` + `internal/filter/network/{chain,readconn}_test.go` + the accesslog tests — the seam unit + `-race` tests; the fault-delay + access-log + drain + close-direction unit tests.
- `test/differential/fixture/fixture.go` — (no new BackendKind — `TCPMongoResponder = 30` reused) possibly a withhold/close-on-demand marker for the `cx_destroy_remote` arm (Task 11; runner-side).
- `test/differential/runner_test.go` — the `0052` driver blank-import + (if needed) a `mongoRespondLoop` REMOTE-close marker arm (Task 11).
- `docs/envoy-go/DECISIONS.md` — ADR-0226 §Decision/§Consequences body IN PLACE (no new ADR number — Task 12).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` / `STATE.md` / `ROADMAP.md` / `next-prompt.txt` — the completion bundle + the parent-row-29 ROLLUP (Task 12).

**Untouched (pinned — a regression gate):** `terminal.go`, `registry.go`, `buffer.go` (`writeconn.go`/`prefixconn.go` gain ONLY the additive `SetCloseDirection` forwarding method — the `Write`/`Read` paths are byte-stable); `internal/listener/manager.go`'s `serveNetworkChain` body BEYOND the one guarded `SetDrainState` line (the read loop / handoff / close paths are UNCHANGED — D-S29.3-2); HCM / `internal/filter/hcm/` / `internal/http/` / the h2 path; `internal/drain/manager.go` (consumed via `IsDraining()`); `internal/stats/` (name.go's four-rule `mongo.` arm + the counter/gauge primitives are consumed, NOT modified); `internal/dynamicmetadata/`; `internal/bootstrap/`; `internal/filter/network/builtins/` (mongoproxy already registered; the registration line is UNCHANGED — drain reaches the chain via `serveNetworkChain`, not the factory); `accesslog/format.go`'s `Default` bytes (the HTTP path stays byte-identical — the formatter-seam regression gate).

---

## Task 1: First-action baselines/anchors gate (no code change)

**Files:** none modified — verification + re-pin gate at the IMPL-session tip; record in `PROGRESS.md` (created this task at the worktree root).

- [ ] **Step 1: Re-confirm the project counts at the IMPL-session tip**

Run (from repo root):
```bash
git log --oneline -1
ls -d test/fixtures/[0-9]* | wc -l            # expect 53; tail dir:
ls -d test/fixtures/[0-9]* | tail -1          # expect test/fixtures/0051-mongo-responses
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 39
grep -nE "^## ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | tail -1       # expect ## ADR-0226
grep -n "TCPMongoResponder BackendKind\|TCPZKResponder BackendKind" test/differential/fixture/fixture.go  # 30 / 29
```
Expected: fixtures **53** (tail `0051-mongo-responses`); fuzzers **39**; DECISIONS.md tail-ADR header **ADR-0226** (next-free **ADR-0227**); `TCPMongoResponder = 30` (reused — no new BackendKind at 29.3). 29.3 lands `0052` → **54**, fuzzers stay **39** (a seam `-race` test instead), stat surface stays **360** (+0 — increment-wiring only), BackendKind tail stays **30**, and the ADR-0226 §Decision/§Consequences body IN PLACE (no new ADR number).

- [ ] **Step 2: Re-confirm the stat surface = 360**

The count STATE.md / BEHAVIOR_CONTRACT.md report is **360** (the 29.1 `337 → 360` extension). Do NOT invent a new recipe. 29.3 lands **+0** (`delays_injected` / `cx_drain_close` / `cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq` all created eagerly at 29.1, presence-only in `0051`, NEVER incremented until now → they go increment-active; no new stats) → stays **360** at Task 12.

- [ ] **Step 3: Re-confirm the as-built anchors (§9.1) the seam + the consumers extend**

```bash
sed -n '146,192p;341,395p;439,450p' internal/filter/network/chain.go   # chainRuntime struct; runData (per-pass stop); replayRead; ContinueReading
sed -n '34,43p' internal/filter/network/readconn.go                    # readChainConn.Read
sed -n '16,38p;62,83p' internal/filter/network/callbacks.go            # ReadFilterCallbacks; Connection (Close/CloseType)
sed -n '105,116p' internal/filter/network/types.go                     # FactoryCtx; (Status/ReadFilter above)
sed -n '101,139p' internal/filter/tcpproxy/filter.go                   # the two pumps + wg.Wait (the §3.5 site)
grep -n "rt.dm\|func .*serveNetworkChain\|NewChainRuntime" internal/listener/manager.go
sed -n '20,53p;82,92p' internal/accesslog/writer.go                    # AsyncFileSink; Submit; run (the Default(r) call site)
grep -n "func (m \*Manager) IsDraining" internal/drain/manager.go
sed -n '53,74p' internal/filter/network/mongoproxy/config.go           # delayConfigured/fixedDelay/delayPercent*/accessLog (parsed 29.1)
sed -n '49,60p;126,155p' internal/filter/network/mongoproxy/codec.go   # decoder struct; decodeOnData loop
sed -n '62,119p' internal/filter/network/mongoproxy/filter.go          # OnData/OnWrite/OnDestroy + cb/wcb
sed -n '40,61p' internal/filter/network/mongoproxy/stats.go            # the eager roster + inc()
sed -n '374,382p' internal/filter/http/fault/fault.go                  # rollPercent precedent
```
Expected: the anchors are present as the §9.1 SPEC pins describe (the per-pass-stop `runData`; the observational `replayRead`; the two-context `ContinueReading`; `Connection.Close(CloseType)`; the two `tcp_proxy` pumps + `wg.Wait`; `rt.dm *drain.Manager`; `AsyncFileSink.run()`'s hard-wired `Default(r)`; the parsed `delayConfigured`/`accessLog`; the mongo `decoder`/`filter`/eager-roster; `IsDraining()`; `rollPercent`'s `p>=100→true` short-circuit).

- [ ] **Step 4: Confirm the clean baseline + create PROGRESS.md**

```bash
go build ./... && go test ./internal/... -count=1
```
Expected: build clean; all internal tests green. Create `PROGRESS.md` at the worktree root with the confirmed counts (53 / 39 / 360 / ADR-0226 / BackendKind 30) + a 12-task checklist. **No commit** (no code change) — or an optional `docs: 29.3 IMPL PROGRESS.md baseline gate` commit if the controller wants the log tracked.

---

## Task 2: The halt-state synchronization on `chainRuntime` (the pre-handoff hold) + a synthetic halting test filter + the never-halting byte-identity proof

**Files:**
- Modify: `internal/filter/network/types.go` (the `HaltingReadFilter` interface)
- Modify: `internal/filter/network/chain.go` (the `chainRuntime` halt fields; the `MayHalt()` scan in `NewChainRuntime`; the `haltable` hold in `runData`; `holdUntilResume`/`releaseResume`)
- Test: `internal/filter/network/chain_test.go`

This task lands the synchronization primitive + the PRE-handoff hold path, tested with a SYNTHETIC halting filter (no mongo dependency — the seam is independently testable per SPEC §12). The async-active `ContinueReading` advance is Task 3; the post-handoff withhold is Task 4. **The never-halting path must stay byte-identical** (the existing `chain_test.go` suite + the 53-dir differential suite are the regression gate).

- [ ] **Step 1: Write the failing tests (synthetic halting filter; hold-then-resume; never-halting byte-identity)**

Add to `chain_test.go` a synthetic halting filter and two tests:
```go
// haltingFilter is a synthetic MayHalt read filter for the seam tests: its OnData
// returns StopIteration exactly once per "armed" hold; an external goroutine calls
// cb.ContinueReading() to release. It models mongo's fault-delay shape WITHOUT the
// mongo dependency (the seam is independently testable — SPEC §12 / R-HALT).
type haltingFilter struct {
	cb        ReadFilterCallbacks
	mayHalt   bool
	holdOnce  bool // arm: StopIteration on the first OnData with bytes
	sawData   int
	released  chan struct{}
}

func (f *haltingFilter) OnNewConnection() Status { return Continue }
func (f *haltingFilter) OnData(b *Buffer, _ bool) Status {
	f.sawData++
	if f.holdOnce {
		f.holdOnce = false
		// Resume asynchronously from a separate goroutine (the time.AfterFunc shape).
		go func() { <-f.released; f.cb.ContinueReading() }()
		return StopIteration
	}
	return Continue
}
func (f *haltingFilter) SetReadFilterCallbacks(cb ReadFilterCallbacks) { f.cb = cb }
func (f *haltingFilter) OnDestroy()                                    {}
func (f *haltingFilter) MayHalt() bool                                 { return f.mayHalt }

// isNetworkFilter sealing: reuse the test Marker if chain_test.go has one; else the
// haltingFilter embeds network.Marker. (Check the existing fakes — filterA/B embed it.)

func TestChainHaltablePreHandoffHoldThenResume(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	if !rt.haltable {
		t.Fatal("chain with a MayHalt filter must be haltable")
	}
	rt.onNewConnection()
	// Drive OnData on a goroutine: it will HOLD (block in holdUntilResume) until released.
	done := make(chan struct{})
	go func() { rt.onData([]byte("PING"), false); close(done) }()
	// onData must NOT have returned yet (the hold blocks the dispatcher).
	select {
	case <-done:
		t.Fatal("onData returned before resume — the haltable hold did not block")
	case <-time.After(50 * time.Millisecond):
	}
	close(hf.released) // let the async ContinueReading fire
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("onData did not return after resume — the hold never released")
	}
	if rt.resumeIdx != 1 {
		t.Errorf("resumeIdx = %d, want 1 (advanced PAST the halting filter)", rt.resumeIdx)
	}
}

func TestChainNonHaltableByteIdentity(t *testing.T) {
	// A MayHalt filter whose MayHalt()==false (mongo-no-delay) takes the byte-identical
	// pre-29.3 path: an OnData StopIteration is a PER-PASS stop (next read re-delivers),
	// NOT a hold; the halt mutex is never engaged.
	hf := &haltingFilter{mayHalt: false, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	if rt.haltable {
		t.Fatal("MayHalt()==false must NOT make the chain haltable")
	}
	rt.onNewConnection()
	rt.onData([]byte("PING"), false) // must return immediately (per-pass stop, no block)
	if rt.resumeIdx != 0 {
		t.Errorf("resumeIdx = %d, want 0 (per-pass stop leaves resumeIdx at the filter)", rt.resumeIdx)
	}
}
```
Add `"time"` to `chain_test.go` imports if absent.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run 'TestChainHaltable|TestChainNonHaltable' -count=1`
Expected: compile error (`rt.haltable` undefined / `MayHalt` not in the classifier) then FAIL — the hold path does not exist yet.

- [ ] **Step 3: Add the `HaltingReadFilter` interface**

In `types.go`, after the `ReadFilter` interface:
```go
// HaltingReadFilter is an OPTIONAL extension a ReadFilter implements to opt into
// the async halt/resume seam (29.3, ADR-0226). A filter returning MayHalt()==true
// declares that its OnData (and the post-handoff replay path) may return
// StopIteration as a HOLD resolvable ONLY by a later cb.ContinueReading() (e.g.
// mongo_proxy fault-delay, where the resume rides a time.AfterFunc goroutine). A
// chain containing ≥1 such filter is "haltable": the dispatching goroutine BLOCKS
// at the hold until resume (block-the-dispatcher; D-S29.3-1). A filter that does
// NOT implement this interface (or returns false) keeps the byte-identical
// pre-29.3 per-pass-stop semantics (R1). CONTRACT: a MayHalt filter that returns
// StopIteration MUST eventually call ContinueReading (mongo's timer always fires),
// else the dispatcher blocks forever.
type HaltingReadFilter interface {
	ReadFilter
	MayHalt() bool
}
```

- [ ] **Step 4: Add the `chainRuntime` halt fields + `NewChainRuntime` `MayHalt()` scan**

In `chain.go`, add `"sync"` + `"sync/atomic"` to imports. Extend `chainRuntime` (keep existing fields; add):
```go
	// 29.3 async halt/resume seam (ADR-0226). haltable is set ONCE at construction
	// (immutable thereafter → race-free without an atomic; the construction-time
	// fast-path gate of SPEC §3.1): true iff ≥1 read filter is a HaltingReadFilter
	// with MayHalt()==true. When false, runData/replayRead/readChainConn.Read take
	// the byte-identical pre-29.3 path (R1). haltMu/haltCond + held/resumeReady are
	// the hold↔resume coordination (block-the-dispatcher; D-S29.3-1); replayIdx is
	// the post-handoff replay resume index (Task 4). closeDir/drain are §3.5/§3.4.
	haltable    bool
	haltMu      sync.Mutex
	haltCond    *sync.Cond
	held        bool // a delay-hold is in effect (guarded by haltMu)
	resumeReady bool // ContinueReading fired before the holder blocked (guarded by haltMu)
	replayIdx   int  // post-handoff replay resume index (Task 4)
	replayHeld  bool // post-handoff replay is parked at a halting filter (Task 4; the pinned park-state — do NOT infer from replayIdx)
	closeDir    atomic.Int32 // CloseDirection recorded by tcp_proxy (Task 10)
	drain       DrainState   // drain decider (Task 9; nil-tolerant)
```
In `newChainRuntime`, after building `rt.cxn`/`rt.cb`, initialize the cond:
```go
	rt.haltCond = sync.NewCond(&rt.haltMu)
```
In `NewChainRuntime`, after the classification loop builds `read`, set `haltable` (BEFORE `newChainRuntime`, or set `rt.haltable` after — set it after since `rt` is built inside `newChainRuntime`; cleanest: scan `read` and set on the returned `rt.rt`):
```go
	rt := newChainRuntime(read, conn, connFacts{ ... }) // (existing call)
	rt.terminal = terminal
	rt.writeFilters = write
	for _, rf := range read { // 29.3: a MayHalt read filter makes the chain haltable
		if hf, ok := rf.(HaltingReadFilter); ok && hf.MayHalt() {
			rt.haltable = true
			break
		}
	}
```
> Note: the internal-test path `newChainRuntime([]ReadFilter{...}, ...)` must ALSO set `haltable` so `chain_test.go` exercises it. Add the same scan to `newChainRuntime` (it receives `[]ReadFilter`), and drop the duplicate scan in `NewChainRuntime` (it flows through `newChainRuntime`). Concretely: put the scan inside `newChainRuntime` over its `filters` param.

So inside `newChainRuntime`, after the callbacks-injection loop:
```go
	for _, f := range rt.filters {
		if hf, ok := f.(HaltingReadFilter); ok && hf.MayHalt() {
			rt.haltable = true
			break
		}
	}
```
(and remove the scan from `NewChainRuntime` — it calls `newChainRuntime`).

- [ ] **Step 5: Add the hold/resume coordination helpers**

In `chain.go`:
```go
// holdUntilResume blocks the calling (dispatcher) goroutine until an async
// ContinueReading releases the hold (block-the-dispatcher; D-S29.3-1). Only called
// on a haltable chain after a filter returned a hold StopIteration. The resumeReady
// flag closes the missed-wakeup race: if ContinueReading fired between OnData's
// return and this acquiring haltMu, resumeReady is already set and we do not block.
func (rt *chainRuntime) holdUntilResume() {
	rt.haltMu.Lock()
	if rt.resumeReady {
		rt.resumeReady = false
	} else {
		rt.held = true
		for rt.held {
			rt.haltCond.Wait()
		}
	}
	rt.haltMu.Unlock()
}

// releaseResume is the resume side: if a holder is blocked, wake it; otherwise
// record that a resume arrived (resumeReady) so the next holdUntilResume does not
// block. Called by ContinueReading's async path (the timer goroutine).
func (rt *chainRuntime) releaseResume() {
	rt.haltMu.Lock()
	if rt.held {
		rt.held = false
		rt.haltCond.Broadcast()
	} else {
		rt.resumeReady = true
	}
	rt.haltMu.Unlock()
}
```

- [ ] **Step 6: Honor the haltable hold in `runData`**

In `runData`, replace the OnData-StopIteration block:
```go
		rt.resumeRequested = false
		status := rt.filters[i].OnData(rt.buf, rt.lastEndStream)
		if status == StopIteration {
			if rt.resumeRequested {
				rt.resumeRequested = false
				rt.resumeIdx = i + 1
				continue
			}
			if rt.haltable {
				// 29.3 delay-hold (SPEC §3.1): block this dispatcher goroutine until
				// the async ContinueReading releases, then advance PAST the halting
				// filter (upstream std::next(filter->entry()) parity) and continue.
				rt.holdUntilResume()
				rt.resumeIdx = i + 1
				continue
			}
			// Per-pass stop (pre-29.3 back-compat; non-haltable chains).
			return
		}
		rt.resumeIdx = i + 1
```

- [ ] **Step 7: Run the tests; verify pass + no regression**

Run:
```bash
go test ./internal/filter/network/ -run 'TestChainHaltable|TestChainNonHaltable' -count=1
go test ./internal/filter/network/ -count=1           # the full existing suite — byte-identity regression
go test ./internal/filter/network/ -race -count=1 -run 'TestChainHaltable'
```
Expected: the two new tests PASS; the full network suite PASS (never-halting byte-identity intact); `-race` clean. `gofmt -l internal/filter/network/ && golangci-lint run ./internal/filter/network/...` clean.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/network/types.go internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 29.3 Task 2: chainRuntime halt-state sync + haltable gate + pre-handoff hold (seam-first; never-halting byte-identical)"
```

---

## Task 3: `ContinueReading` async-active third context (pre-handoff) + the terminal-readiness-after-resume proof

**Files:**
- Modify: `internal/filter/network/chain.go` (`callbacks.ContinueReading` async third context)
- Test: `internal/filter/network/chain_test.go`

Task 2 added the hold + `releaseResume`, but `ContinueReading` does not yet route the async (not-`connHalted`, not-re-entrant) call to `releaseResume`. This task adds the THIRD context and proves a pre-handoff async resume makes `terminalReady()` reachable.

- [ ] **Step 1: Write the failing test (async resume drives terminal readiness)**

```go
func TestChainAsyncResumeReachesTerminalReady(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	term := &recordingTerminal{} // the existing TerminalFilter double in upstreamcluster_test.go (or recordTerminal in chain_test.go)
	rt := NewChainRuntime([]NetworkFilter{hf, term}, &fakeConn{}, ConnFacts{})
	rt.OnNewConnection()
	if rt.TerminalReady() {
		t.Fatal("not ready before the read filter Continues past")
	}
	done := make(chan struct{})
	go func() { rt.OnData([]byte("Q1"), false); close(done) }()
	time.Sleep(30 * time.Millisecond) // let it reach the hold
	close(hf.released)                 // async ContinueReading fires
	<-done
	if !rt.TerminalReady() {
		t.Fatal("after async resume the chain must be TerminalReady (resumeIdx past the last read filter)")
	}
}
```
> `recordingTerminal` is the real TerminalFilter double in `internal/filter/network/upstreamcluster_test.go` (a sibling `recordTerminal` lives in `chain_test.go`); either works — both implement `TerminalFilter.Handle`. No new double is needed.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run TestChainAsyncResumeReachesTerminalReady -count=1`
Expected: FAIL/HANG-then-timeout — `ContinueReading` sets `resumeRequested` (the re-entrant path) which nothing consumes off-dispatch → the holder never releases → `onData` never returns. (If it hangs, the test's 2s guards in Task 2's helper bound it; add a timeout guard here too.)

- [ ] **Step 3: Add the async third context to `ContinueReading`**

In `chain.go`, extend `callbacks.ContinueReading`:
```go
func (c *callbacks) ContinueReading() {
	rt := c.rt
	if rt.connHalted {
		rt.connHalted = false
		rt.resumeIdx++
		rt.runData()
		return
	}
	if rt.haltable {
		// 29.3 async-active third context (SPEC §3.1 extension 1): a ContinueReading
		// arriving off-dispatch (from a time.AfterFunc goroutine) while the dispatcher
		// is blocked at a hold. Release the holder; the blocked dispatcher (runData
		// pre-handoff / readChainConn.Read post-handoff) advances past the halting
		// filter itself on wake (D-S29.3-2 — no handoff is driven from this goroutine).
		rt.releaseResume()
		return
	}
	// Re-entrant from within the current filter's OnData (non-haltable chains).
	rt.resumeRequested = true
}
```
> Subtlety: the re-entrant case on a HALTABLE chain. Mongo's fault-delay resume is ALWAYS async (the timer goroutine), never re-entrant (mongo never calls ContinueReading from inside its own OnData). So routing haltable ContinueReading to `releaseResume` is correct for mongo. The re-entrant `resumeRequested` path is exercised only by non-haltable filters (the existing `TestChainContinueReadingResumesAtNextFilter`), which now also keep working because they are non-haltable. Verify the existing test still passes.

- [ ] **Step 4: Run; verify pass + no regression**

Run:
```bash
go test ./internal/filter/network/ -run 'TestChainAsyncResume|TestChainContinueReading|TestChainHaltable' -count=1
go test ./internal/filter/network/ -count=1
go test ./internal/filter/network/ -race -count=1 -run 'TestChainAsyncResume'
```
Expected: all PASS; `-race` clean; `gofmt`/`golangci-lint` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/network/chain.go internal/filter/network/chain_test.go
git commit -m "phase 29.3 Task 3: ContinueReading async-active third context (pre-handoff resume reaches TerminalReady)"
```

---

## Task 4: `replayRead` + `readChainConn.Read` post-handoff withhold-until-resume + the `-race -count=N` cross-goroutine seam test (R-HALT)

**Files:**
- Modify: `internal/filter/network/chain.go` (`replayRead` honors the hold; `ContinueReading` post-handoff replay advance)
- Modify: `internal/filter/network/readconn.go` (`readChainConn.Read` withhold-until-resume behind the `haltable` gate)
- Test: `internal/filter/network/chain_test.go` / `readconn_test.go`

This is the LOAD-BEARING path: steady-state mongo decode runs post-handoff via `replayRead`. A delay on a later message must be honored here. The `-race` test (R-HALT) proves the mutex is necessary AND sufficient (the deliberate-break with `-count=1`).

- [ ] **Step 1: Write the failing tests (post-handoff hold; never-halting replay byte-identity; the `-race` seam test)**

```go
func TestReplayReadPostHandoffHoldThenResume(t *testing.T) {
	hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
	rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
	// Simulate post-handoff: a replay parks at the hold; the pump must withhold.
	parked := rt.replayRead([]byte("Q2"), false)
	if !parked {
		t.Fatal("replayRead must report parked=true when a haltable filter holds")
	}
	done := make(chan struct{})
	go func() { rt.holdUntilResume(); close(done) }()
	select {
	case <-done:
		t.Fatal("holdUntilResume returned before resume")
	case <-time.After(40 * time.Millisecond):
	}
	close(hf.released)
	<-done // resume releases
}

func TestReplayReadNonHaltableObservational(t *testing.T) {
	// Non-haltable replay is byte-identical to pre-29.3: all filters observe, buffer
	// fully drained, parked=false always.
	b := &filterB{}
	rt := newChainRuntime([]ReadFilter{b}, &fakeConn{}, connFacts{})
	parked := rt.replayRead([]byte("hello"), false)
	if parked {
		t.Fatal("non-haltable replay never parks")
	}
	if rt.buf.Len() != 0 {
		t.Errorf("buffer not drained after observational replay: %d", rt.buf.Len())
	}
	if b.saw != "hello" {
		t.Errorf("filter did not observe the replayed bytes: %q", b.saw)
	}
}

// TestSeamRaceAsyncResume is the R-HALT cross-goroutine proof: an async ContinueReading
// racing the post-handoff replay path under -race. With the halt mutex it is clean;
// WITHOUT it (the Task-4 deliberate break) -race reports immediately (run -count=1).
func TestSeamRaceAsyncResume(t *testing.T) {
	for iter := 0; iter < 50; iter++ {
		hf := &haltingFilter{mayHalt: true, holdOnce: true, released: make(chan struct{})}
		rt := newChainRuntime([]ReadFilter{hf}, &fakeConn{}, connFacts{})
		parked := rt.replayRead([]byte("X"), false)
		if !parked {
			t.Fatal("expected park")
		}
		go func() { close(hf.released) }() // fire the async resume
		rt.holdUntilResume()               // racing the released ContinueReading
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ -run 'TestReplayRead|TestSeamRace' -count=1`
Expected: FAIL — `replayRead` currently returns nothing (signature change) and never parks; the observational path drains unconditionally.

- [ ] **Step 3: Make `replayRead` honor the hold (return `parked bool`)**

In `chain.go`, replace `replayRead`:
```go
// replayRead re-iterates the read-filter chain over post-handoff downstream bytes
// (the read-side seam, ADR-0221 §AMEND). For a NON-haltable chain it is the
// pre-29.3 OBSERVATIONAL pass (Status ignored; buffer drained) — byte-identical.
// For a HALTABLE chain (29.3, SPEC §3.1) it honors a hold StopIteration: it stops
// at the halting filter, leaves the buffer UNDRAINED, parks replayIdx, and returns
// parked=true so readChainConn.Read withholds the bytes until ContinueReading.
func (rt *chainRuntime) replayRead(p []byte, endStream bool) (parked bool) {
	rt.buf.Append(p)
	if !rt.haltable {
		for _, f := range rt.filters {
			_ = f.OnData(rt.buf, endStream) // Status ignored — observational (§3.5)
		}
		rt.buf.Drain(rt.buf.Len())
		return false
	}
	for rt.replayIdx < len(rt.filters) {
		i := rt.replayIdx
		if rt.filters[i].OnData(rt.buf, endStream) == StopIteration {
			// Hold: do NOT advance replayIdx, do NOT drain. The pump (readChainConn.Read)
			// holdUntilResume()s; ContinueReading advances + drains + releases.
			rt.replayHeld = true // the pinned park-state ContinueReading consumes (Step 4)
			return true
		}
		rt.replayIdx++
	}
	rt.replayIdx = 0
	rt.buf.Drain(rt.buf.Len())
	return false
}
```
Update the existing `readChainConn.Read` caller (Step 5) to consume the bool.

- [ ] **Step 4: Add the post-handoff advance to `ContinueReading`**

The async `releaseResume` releases the pump, but post-handoff the held bytes' replay must complete (advance past the halting filter, re-dispatch any remaining filters, drain). Pin a dedicated `rt.replayHeld bool` (set true by `replayRead` when it parks; do NOT infer park-state from `replayIdx==0`, which is ambiguous for the single-filter case). Add the field to `chainRuntime` (Task 2 added the others); `replayRead` (Step 3) sets `rt.replayHeld = true` when it parks. Fold the post-handoff advance into `ContinueReading`'s haltable path so it works for BOTH pre- and post-handoff:
```go
	if rt.haltable {
		rt.haltMu.Lock()
		if rt.replayHeld {
			// Post-handoff: advance the replay PAST the halting filter + finish the pass
			// (re-dispatch any remaining read filters; drain). For the production single
			// read-filter mongo chain there are no filters after mongo → just reset+drain.
			rt.replayHeld = false
			rt.replayIdx++
			for rt.replayIdx < len(rt.filters) {
				if rt.filters[rt.replayIdx].OnData(rt.buf, rt.lastEndStream) == StopIteration {
					rt.replayHeld = true // re-parked (not reached by single-filter mongo)
					rt.haltMu.Unlock()
					return
				}
				rt.replayIdx++
			}
			rt.replayIdx = 0
			rt.buf.Drain(rt.buf.Len())
		}
		// Pre-handoff: replayHeld is false; the blocked runData advances resumeIdx itself
		// on wake. Either way, release the holder (inline releaseResume under the lock).
		if rt.held {
			rt.held = false
			rt.haltCond.Broadcast()
		} else {
			rt.resumeReady = true
		}
		rt.haltMu.Unlock()
		return
	}
```
> **D-S29.3-2 simplification for mongo (single read filter):** `rt.filters = [mongo]`, so the inner re-dispatch loop never executes; the post-handoff resume is `replayHeld=false` + `replayIdx=0` + `buf.Drain` + release. The general (multi-read-filter) re-dispatch is included for correctness but is not exercised by any 29.3 fixture (recorded as a completeness path; the IMPL may simplify to the single-filter case + a `// general N-filter re-dispatch: future` note if the loop complicates the `-race` proof). The PLAN pins the SEMANTICS (advance past the halting filter, drain, release exactly once); the exact primitive is IMPL latitude (D-S29.3-1) but `replayHeld` is the pinned park-state representation. `replayRead` must set `rt.replayHeld = true` on the `return true` (park) branch.

- [ ] **Step 5: `readChainConn.Read` withhold-until-resume**

In `readconn.go`, update `Read`:
```go
func (r *readChainConn) Read(b []byte) (int, error) {
	n, err := r.Conn.Read(b)
	if n > 0 {
		parked := r.rt.replayRead(b[:n], false)
		if parked {
			// 29.3 post-handoff withhold (SPEC §3.1 extension 3): block the pump until
			// the async ContinueReading releases, so the delayed request's bytes are
			// withheld from the upstream pump until the fault delay elapses. Behind the
			// haltable gate inside replayRead — byte-identical when no hold (R1).
			r.rt.holdUntilResume()
		}
	}
	if err != nil && errors.Is(err, io.EOF) {
		r.rt.replayRead(nil, true)
	}
	return n, err
}
```

- [ ] **Step 6: Run the tests + the `-race` seam test**

Run:
```bash
go test ./internal/filter/network/ -run 'TestReplayRead|TestSeamRace|TestChain' -count=1
go test ./internal/filter/network/ -race -count=20 -run 'TestSeamRaceAsyncResume'
go test ./internal/filter/network/ -count=1     # full regression (readChainConn byte-identity)
```
Expected: all PASS; `-race -count=20` clean.

- [ ] **Step 7: Deliberate-break R-HALT (prove the mutex is necessary) — `-count=1`**

Temporarily replace `holdUntilResume`/`releaseResume`'s `haltMu`-guarded body with UNLOCKED `held`/`resumeReady` access (delete the `rt.haltMu.Lock()/Unlock()` pairs and the cond, busy-spin on `held`). Run:
```bash
go test ./internal/filter/network/ -race -count=1 -run 'TestSeamRaceAsyncResume'
```
Expected: `-race` reports a data race on `held`/`resumeReady` (the mutex is NECESSARY). **Revert** the break; re-run clean. Record both outputs (broken FAIL + reverted PASS) in `PROGRESS.md` per `reference_differential_break_protocol_count1`.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/network/chain.go internal/filter/network/readconn.go internal/filter/network/*_test.go
git commit -m "phase 29.3 Task 4: replayRead + readChainConn.Read post-handoff withhold-until-resume + R-HALT -race seam test"
```

---

## Task 5: The fault-delay roll+duration decision in the decoder (`maybeInjectDelay` + `takePendingDelay`)

**Files:**
- Modify: `internal/filter/network/mongoproxy/codec.go` (`decoder` fields; `maybeInjectDelay`; `takePendingDelay`; the six-callback eval points; the decode-loop stop-after-decide)
- Test: `internal/filter/network/mongoproxy/codec_test.go`

The decoder owns the roll+duration decision (D-S29.3-6). It evaluates at the entry of each of the six request decode callbacks (D-S29.3-5), at-most-once per pass (the `delayPending` re-entrancy guard). `rollPercent` is deterministic at 100% (no RNG — the phase-09 precedent). The FILTER arms the timer (Task 6).

- [ ] **Step 1: Write the failing tests**

```go
func TestDecoder_DelayDecidedAt100Percent(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 100 * time.Millisecond,
		delayPercentNum: 100, delayPercentDenom: 0 /*HUNDRED*/, commands: map[string]bool{}}
	d, ms := newTestDecoderCfg(t, cfg) // a helper that wires cfg + a fresh roster
	_ = ms
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q, int64(len(q)))
	dur, ok := d.takePendingDelay()
	if !ok || dur != 100*time.Millisecond {
		t.Fatalf("takePendingDelay = (%v,%v), want (100ms,true)", dur, ok)
	}
	if _, ok := d.takePendingDelay(); ok {
		t.Fatal("takePendingDelay must be consumed once per decide")
	}
}

func TestDecoder_NoDelayWhenUnconfigured(t *testing.T) {
	d, _ := newTestDecoder(t) // delayConfigured == false
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q, int64(len(q)))
	if _, ok := d.takePendingDelay(); ok {
		t.Fatal("no delay must be decided when delayConfigured is false")
	}
}

func TestDecoder_ReentrancyGuardAtMostOnePerPass(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 50 * time.Millisecond,
		delayPercentNum: 100, delayPercentDenom: 0, commands: map[string]bool{}}
	d, _ := newTestDecoderCfg(t, cfg)
	d.delayPending.Store(true) // simulate an armed (pending) timer
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	d.decodeOnData(q, int64(len(q)))
	if _, ok := d.takePendingDelay(); ok {
		t.Fatal("an armed (pending) delay must suppress re-decide (re-entrancy guard)")
	}
}
```
> Add `newTestDecoderCfg(t, cfg)` to the test helpers if absent (the 29.1/29.2 `newTestDecoder` builds a default cfg; add a cfg-taking variant).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecoder_Delay|TestDecoder_NoDelay|TestDecoder_Reentrancy' -count=1`
Expected: compile error (`takePendingDelay`/`delayPending` undefined) then FAIL.

- [ ] **Step 3: Add the decoder fields + `maybeInjectDelay` + `takePendingDelay`**

In `codec.go`, extend the `decoder` struct:
```go
	// 29.3 fault-delay (SPEC §3.2; D-S29.3-6). The decoder DECIDES (roll+duration);
	// the filter arms the timer. delayDecided/pendingDelay are read-direction-local
	// (the request decode is single-goroutine per direction); delayPending is the
	// cross-goroutine re-entrancy guard (filter sets at arm, onDelayTimer clears).
	pendingDelay time.Duration
	delayDecided bool
	delayPending atomic.Bool
```
Add the helpers:
```go
// maybeInjectDelay evaluates the fault delay at the entry of a request-direction
// decode callback (SPEC §3.2; the parent §11.6 six-callback set — D-S29.3-5). It
// decides AT MOST ONCE per pass: when a delay is configured, none is already
// pending (the re-entrancy guard), none was already decided this pass, and the
// percentage rolls true (deterministic at 100% — no RNG, the phase-09 precedent),
// it records pendingDelay + delayDecided. The FILTER consumes it via takePendingDelay
// after the decode pass and arms the time.AfterFunc. Mirrors upstream tryInjectDelay
// (proxy.cc:434-449) — the IMPL transcribes the callback set verbatim vs proxy.cc v1.37.2.
func (d *decoder) maybeInjectDelay() {
	if !d.cfg.delayConfigured || d.delayDecided || d.delayPending.Load() {
		return
	}
	if !rollPercent(d.cfg.delayPercentNum, d.cfg.delayPercentDenom) {
		return
	}
	d.pendingDelay = d.cfg.fixedDelay
	d.delayDecided = true
}

// takePendingDelay returns (duration, true) exactly once per decided delay; the
// filter calls it after decodeOnData/replayRead-via-OnData to arm the timer.
func (d *decoder) takePendingDelay() (time.Duration, bool) {
	if d.delayDecided {
		d.delayDecided = false
		return d.pendingDelay, true
	}
	return 0, false
}
```
Add the package-level `rollPercent` (the phase-09 deterministic-boundary form; mongo's `0052` arms are 100% so no RNG is needed — keep it RNG-free, the deterministic-differential discipline):
```go
// rollPercent returns true iff the configured FractionalPercent fires. Deterministic
// at the boundaries (>=100% always; 0% never) — the phase-09 rollPercent precedent.
// 29.3's differential arms are 100% (BOOTSTRAP §7.2: timing never compared, the
// roll deterministic), so NO RNG path is wired (an intermediate percentage would
// need one — out of scope; the parent §11.6 percentage gate at 100% is sufficient).
func rollPercent(num uint32, denom int32) bool {
	if num == 0 {
		return false
	}
	var scale uint64
	switch denom {
	case 0: // HUNDRED
		scale = 100
	case 1: // TEN_THOUSAND
		scale = 10000
	case 2: // MILLION
		scale = 1000000
	default:
		scale = 100
	}
	return uint64(num) >= scale // deterministic ≥100% fire; intermediate (RNG) out of scope
}
```
> **D-S29.3-5 transcription:** wire `maybeInjectDelay()` at the FIRST line of EACH of the six request decode callbacks: `decodeQuery`, `decodeInsert`, `decodeGetMore`, `decodeKillCursors`, `decodeCommand`, and the request-path `decodeCommandReply` (NOTE: the as-built codec has NO `decodeCommandReply` on the request path — `opCommandReply` is response-only via `decodeResponseMessage`; the parent §11.6 six-callback set lists `decodeCommandReply` because upstream's request decoder has it. Mongo-go decodes `opCommand` (2010) on the request side and `opCommandReply` (2011) on the response side. The IMPL transcribes the upstream set to the AS-BUILT callbacks: the five request decoders that exist — `decodeQuery`/`decodeInsert`/`decodeGetMore`/`decodeKillCursors`/`decodeCommand` — get the eval; `decodeReply`/`decodeCommandReply` (response-side) do NOT, matching upstream's "NOT decodeReply" exclusion. Record this five-vs-six reconciliation in the IMPL commit + the BEHAVIOR_CONTRACT). For `0052` only `decodeQuery` is load-bearing.

Add the eval call at each request decoder entry, e.g. in `decodeQuery`:
```go
func (d *decoder) decodeQuery(requestID int32, body []byte) bool {
	d.maybeInjectDelay() // 29.3 fault-delay eval (SPEC §3.2)
	r := &bsonReader{buf: body}
	...
```
(and likewise the first line of `decodeInsert`/`decodeGetMore`/`decodeKillCursors`/`decodeCommand`).

> **Stop-after-decide (at-most-one delay per pass):** in `decodeOnData`'s message loop, break once a delay is decided so the held bytes are the in-flight request (SPEC §3.2). After `d.decodeMessage(m)`:
```go
	for {
		m, ok := d.nextMessage()
		if !ok {
			break
		}
		if !d.decodeMessage(m) {
			break // decoding_error path
		}
		if d.delayDecided {
			break // 29.3: one delay armed this pass; remaining buffered messages wait (re-entrancy)
		}
	}
```
(The remaining undecoded messages stay in `d.readBuf` — the decoder's own buffer — and decode on the next pass after resume. For `0052`, the driver sends one message per round-trip so this is the common single-message case.)

- [ ] **Step 4: Run; verify pass**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestDecoder_Delay|TestDecoder_NoDelay|TestDecoder_Reentrancy' -count=1 && go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: new tests PASS; all existing mongoproxy tests PASS (no behavior change when `delayConfigured` is false). `gofmt`/`golangci-lint` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/codec_test.go
git commit -m "phase 29.3 Task 5: fault-delay roll+duration decision in the decoder (maybeInjectDelay/takePendingDelay; deterministic 100%; re-entrancy guard)"
```

---

## Task 6: The fault-delay timer in the filter (`time.AfterFunc` + `onDelayTimer` + `ContinueReading`; `delays_injected` at arm; StopIteration-while-pending; timer cancel on `OnDestroy`) + the timer↔destroy `-race` test

**Files:**
- Modify: `internal/filter/network/mongoproxy/filter.go` (`MayHalt`; `delayTimer`; arm-on-`OnData`/replay; `onDelayTimer`; `OnDestroy` cancel)
- Test: `internal/filter/network/mongoproxy/filter_test.go`

The filter owns the timer + the resume (D-S29.3-6). It increments `delays_injected` at ARM (upstream `stats_.delays_injected_.inc()` at arm). `OnData` returns StopIteration while the timer pends. The timer goroutine clears the guard + calls `ContinueReading`. `OnDestroy` cancels best-effort (the phase-09 precedent).

- [ ] **Step 1: Write the failing tests**

```go
func TestFilter_MayHaltReflectsDelayConfigured(t *testing.T) {
	fNo := newTestFilter(t, &compiledConfig{statPrefix: "m", commands: map[string]bool{}})
	if fNo.MayHalt() {
		t.Error("no delay configured → MayHalt() must be false (chain stays non-haltable)")
	}
	fYes := newTestFilter(t, &compiledConfig{statPrefix: "m", delayConfigured: true,
		fixedDelay: 10 * time.Millisecond, delayPercentNum: 100, commands: map[string]bool{}})
	if !fYes.MayHalt() {
		t.Error("delay configured → MayHalt() must be true")
	}
}

func TestFilter_OnDataArmsDelayAndStops(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 20 * time.Millisecond,
		delayPercentNum: 100, commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg) // a fake ReadFilterCallbacks recording ContinueReading
	buf := &network.Buffer{}
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	buf.Append(q)
	if got := f.OnData(buf, false); got != network.StopIteration {
		t.Fatalf("OnData = %v, want StopIteration (delay armed)", got)
	}
	if v := ms.counters["delays_injected"].Load(); v != 1 {
		t.Errorf("delays_injected = %d, want 1 (at ARM)", v)
	}
	// The timer fires after ~20ms → ContinueReading on the callbacks.
	select {
	case <-cb.continued:
	case <-time.After(2 * time.Second):
		t.Fatal("timer did not fire ContinueReading")
	}
	if f.dec.delayPending.Load() {
		t.Error("delayPending must be cleared after the timer fires")
	}
}

func TestFilter_OnDestroyCancelsPendingTimer(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", delayConfigured: true, fixedDelay: 10 * time.Second, // long
		delayPercentNum: 100, commands: map[string]bool{}}
	f, _, _ := newTestFilterWithCB(t, cfg)
	buf := &network.Buffer{}
	q := msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery()))
	buf.Append(q)
	_ = f.OnData(buf, false) // arms a 10s timer
	f.OnDestroy()            // must Stop it (no panic, no leak) — race-clean w/ a firing timer
}
```
> `newTestFilterWithCB` returns a fake `network.ReadFilterCallbacks` whose `ContinueReading()` closes/ sends on a `continued chan struct{}`, plus a `Connection()` stub (for later tasks) and `Draining()`/`CloseDirection()` returning defaults. Add it to the test helpers.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_MayHalt|TestFilter_OnDataArms|TestFilter_OnDestroyCancels' -count=1`
Expected: compile error (`MayHalt`/`delayTimer` undefined) then FAIL.

- [ ] **Step 3: Add `MayHalt`, the timer, and the arm/fire/cancel wiring**

In `filter.go`, add `"time"` to imports + a `delayTimer` field:
```go
type filter struct {
	network.Marker
	cfg        *compiledConfig
	dec        *decoder
	cb         network.ReadFilterCallbacks
	wcb        network.WriteFilterCallbacks
	delayTimer *time.Timer // 29.3 fault-delay async-resume timer (SPEC §3.2); OnDestroy cancels.
}

// MayHalt declares the filter haltable (the chain's async halt/resume seam, 29.3)
// iff a fault delay is configured. A no-delay mongo filter is non-haltable → the
// chain takes the byte-identical pre-29.3 path (R1).
func (f *filter) MayHalt() bool { return f.cfg.delayConfigured }
```
Extend `OnData` to arm a delay (the decoder already decided in `decodeOnData`):
```go
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	f.emitDynamicMetadata()
	f.emitAccessLogRequests() // Task 8 (no-op until then)
	if dur, ok := f.dec.takePendingDelay(); ok {
		return f.armDelay(dur)
	}
	return network.Continue
}

// armDelay increments delays_injected AT ARM (upstream stats_.delays_injected_.inc()),
// sets the cross-goroutine re-entrancy guard, schedules the resume timer, and returns
// StopIteration so the chain HOLDS (the haltable seam — runData/replayRead block the
// dispatcher until onDelayTimer's ContinueReading; SPEC §3.2 / §3.1).
func (f *filter) armDelay(dur time.Duration) network.Status {
	f.cfg.stats.inc("delays_injected")
	f.dec.delayPending.Store(true)
	f.delayTimer = time.AfterFunc(dur, f.onDelayTimer)
	return network.StopIteration
}

// onDelayTimer runs on the time.AfterFunc goroutine: clear the re-entrancy guard,
// then resume the chain (the §3.1 async-active ContinueReading — the load-bearing
// cross-goroutine resume). cb is set once at construction; safe to read here.
func (f *filter) onDelayTimer() {
	f.dec.delayPending.Store(false)
	if f.cb != nil {
		f.cb.ContinueReading()
	}
}
```
Update `OnDestroy` to cancel the timer (best-effort; the phase-09 precedent) BEFORE the residual drain. (The full `OnDestroy` — with the close-direction increment — lands at Task 10; here add only the timer cancel):
```go
func (f *filter) OnDestroy() {
	if f.delayTimer != nil {
		f.delayTimer.Stop() // best-effort; the timer always fires on a blocked pump, so this
		// races only a torn-down-mid-delay edge — no double-count (delays_injected Inc'd at arm).
	}
	if f.dec != nil {
		f.dec.onDestroy()
	}
	f.dec = nil
}
```
> **Post-handoff arm:** the filter's `OnData` is ALSO invoked by `replayRead` (the post-handoff path runs `f.OnData` via the chain). So the SAME `OnData` arm path handles both pre- and post-handoff (the chain's `replayRead` parks on the StopIteration; `readChainConn.Read` withholds). No separate replay arm code is needed in the filter. ✔

- [ ] **Step 4: Run the tests + the timer↔destroy `-race`**

Run:
```bash
go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_MayHalt|TestFilter_OnDataArms|TestFilter_OnDestroyCancels' -count=1
go test ./internal/filter/network/mongoproxy/ -race -count=10 -run 'TestFilter_OnDestroyCancels|TestFilter_OnDataArms'
go test ./internal/filter/network/mongoproxy/ -count=1
```
Expected: all PASS; `-race -count=10` clean (the `delayPending atomic.Bool` + the happens-after `OnDestroy` discipline are race-clean). `gofmt`/`golangci-lint` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/network/mongoproxy/filter.go internal/filter/network/mongoproxy/filter_test.go
git commit -m "phase 29.3 Task 6: fault-delay timer in the filter (MayHalt; arm+delays_injected; StopIteration-while-pending; OnDestroy cancel; timer<->destroy -race)"
```

---

## Task 7: The `internal/accesslog` pluggable formatter seam + the mongo JSON formatter + the per-opcode `message.toString` goldens

**Files:**
- Modify: `internal/accesslog/writer.go` (the `Formatter` type; the optional `formatter` field; `Submit(any)`; the `run()` call site)
- Create: `internal/accesslog/mongo_format.go` (the mongo record type + the mongo JSON formatter)
- Create: `internal/accesslog/mongo_format_test.go` (the goldens)
- Test: `internal/accesslog/writer_test.go` (the HTTP `Default` byte-identity regression)

D-P7 / D-S29.3-4: a pluggable formatter over `any`, default `Default(*Record)` (HCM byte-identical). The mongo formatter emits `{"time","message","upstream_host"}`; the `time` field is timing-bearing (asserted by shape, not value). NO fixture dir (AMEND-B10).

- [ ] **Step 1: Write the failing tests (HTTP default byte-identity; mongo formatter golden)**

In `writer_test.go`:
```go
func TestAsyncFileSinkDefaultFormatterByteIdentical(t *testing.T) {
	// The default (zero-value formatter) path stays byte-identical to Default(*Record).
	dir := t.TempDir()
	path := filepath.Join(dir, "al.log")
	s, err := NewAsyncFileSink(path, stats.NewRegistry().NewCounter("x"))
	if err != nil { t.Fatal(err) }
	rec := &Record{Method: "GET", Path: "/p", Protocol: "HTTP/1.1", ResponseCode: 200}
	s.Submit(rec)
	if err := s.Close(); err != nil { t.Fatal(err) }
	got, _ := os.ReadFile(path)
	if string(got) != string(Default(rec)) {
		t.Errorf("default-formatter bytes diverged:\n got=%q\nwant=%q", got, Default(rec))
	}
}
```
In `mongo_format_test.go`:
```go
func TestMongoFormatterGolden(t *testing.T) {
	rec := &MongoRecord{
		Time:         time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
		Message:      `{"opcode": "OP_QUERY", "id": 1, "collection": "db.collection1"}`,
		UpstreamHost: "127.0.0.1:27017",
	}
	line := MongoFormat(rec)
	// The time field is asserted by SHAPE (RFC3339-ish), the rest by value.
	if !strings.HasPrefix(string(line), `{"time":"2026-06-06T12:00:00`) {
		t.Errorf("time field shape wrong: %q", line)
	}
	if !strings.Contains(string(line), `"upstream_host":"127.0.0.1:27017"`) {
		t.Errorf("upstream_host missing: %q", line)
	}
	if !strings.HasSuffix(string(line), "}\n") {
		t.Errorf("line must be one JSON object + newline: %q", line)
	}
}

func TestMongoFormatterUpstreamHostDash(t *testing.T) {
	rec := &MongoRecord{Time: time.Unix(0, 0).UTC(), Message: "{}", UpstreamHost: ""}
	if !strings.Contains(string(MongoFormat(rec)), `"upstream_host":"-"`) {
		t.Error("empty upstream host must render as \"-\" (Envoy missing-value convention)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/accesslog/ -run 'TestAsyncFileSinkDefaultFormatter|TestMongoFormatter' -count=1`
Expected: compile error (`Formatter`/`Submit(any)`/`MongoRecord`/`MongoFormat` undefined) then FAIL.

- [ ] **Step 3: Add the `Formatter` seam to `writer.go`**

```go
// Formatter renders an access-log record into a line of bytes. The default
// formatter (when AsyncFileSink.formatter is nil) is the HTTP Default over *Record
// (HCM callers byte-identical — the 29.3 D-P7 regression gate). Non-HTTP sinks
// (mongo_proxy, 29.3) supply their own formatter over their own record type.
type Formatter func(rec any) []byte
```
Change `AsyncFileSink`'s channel + add the field:
```go
type AsyncFileSink struct {
	ch          chan any   // was chan *Record — now carries any record type
	f           *os.File
	done        chan struct{}
	dropped     *stats.Counter
	lastDropLog atomic.Int64
	closeOnce   sync.Once
	path        string
	formatter   Formatter // nil → the Default(*Record) adapter (byte-identical HCM path)
}
```
Add a formatter-taking constructor + keep the existing one byte-stable:
```go
// NewAsyncFileSinkWithFormatter is the 29.3 variant: a sink with a custom record
// formatter (mongo_proxy). The default NewAsyncFileSink keeps formatter nil → the
// Default(*Record) adapter, so every existing HCM caller is byte-identical.
func NewAsyncFileSinkWithFormatter(path string, dropped *stats.Counter, f Formatter) (*AsyncFileSink, error) {
	s, err := newAsyncFileSinkWithCapacity(path, dropped, defaultChannelCapacity)
	if err != nil {
		return nil, err
	}
	s.formatter = f
	return s, nil
}
```
> The `formatter` is set AFTER `newAsyncFileSinkWithCapacity` starts the goroutine — but `run()` reads `s.formatter` per record; set it before any `Submit`. Since boot constructs the sink before serving, this is safe. (Alternatively thread `f` into `newAsyncFileSinkWithCapacity`; the IMPL may prefer that to avoid the post-start set — pick whichever keeps the existing `NewAsyncFileSink` signature byte-stable.)

Change `Submit` + `run`:
```go
func (s *AsyncFileSink) Submit(r any) {
	select {
	case s.ch <- r:
	default:
		s.dropped.Inc()
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("accesslog: channel full, dropping record (path=%s)", s.path)
		}
	}
}

func (s *AsyncFileSink) run() {
	defer close(s.done)
	for r := range s.ch {
		var line []byte
		if s.formatter != nil {
			line = s.formatter(r)
		} else {
			line = Default(r.(*Record)) // the byte-identical HCM path
		}
		if _, err := s.f.Write(line); err != nil {
			log.Printf("accesslog: file write error (path=%s): %v", s.path, err)
		}
	}
}
```
> `ch` becomes `chan any` and `Submit(any)`. Existing HCM callers pass `*Record` (unchanged at the call site — `*Record` satisfies `any`). Verify the HCM access-log call sites compile (they call `Submit(rec)` where `rec` is `*Record` — still fine).

- [ ] **Step 4: Add `mongo_format.go`**

```go
package accesslog

import (
	"bytes"
	"strconv"
	"time"
)

// MongoRecord is the mongo_proxy access-log record (parent §11.8; 29.3). One JSON
// line per decoded message in BOTH directions: {"time","message","upstream_host"}.
// The time field is per-message wall clock → timing-bearing → the access log is
// differential-INVISIBLE (AMEND-B10): the proof is unit goldens (time by shape) +
// a BEHAVIOR_CONTRACT coverage boundary, NO fixture dir.
type MongoRecord struct {
	Time         time.Time
	Message      string // message.toString() per opcode (full=true requests / full=false replies)
	UpstreamHost string // "addr" or "-" (Envoy missing-value convention)
}

// MongoFormat renders a MongoRecord as one JSON line (newline-terminated),
// mirroring upstream AccessLog::logMessage (proxy.cc:37-57). rec MUST be a
// *MongoRecord (the sink is constructed with this formatter only).
func MongoFormat(rec any) []byte {
	r := rec.(*MongoRecord)
	var b bytes.Buffer
	b.Grow(128 + len(r.Message))
	b.WriteString(`{"time":`)
	b.WriteString(strconv.Quote(r.Time.UTC().Format("2006-01-02T15:04:05.000Z")))
	b.WriteString(`,"message":`)
	b.WriteString(strconv.Quote(r.Message))
	b.WriteString(`,"upstream_host":`)
	b.WriteString(strconv.Quote(orEmptyDash(r.UpstreamHost)))
	b.WriteString("}\n")
	return b.Bytes()
}
```
> **`message.toString()` per-opcode shapes (D-S29.3-4 / parent §11.8):** the `Message` string is built by the mongo CODEC per opcode (`codec_impl.cc:55-307`), NOT by `MongoFormat` (which just JSON-wraps it). Task 8 builds the per-opcode string in `mongoproxy` and passes it as `MongoRecord.Message`. The exact per-opcode format (request `full=true` dumps documents; reply `full=false` counts) is transcribed against `codec_impl.cc` v1.37.2 at Task 8 and pinned as goldens; since the access log is differential-invisible, the goldens are the authority (no cross-side comparison).

- [ ] **Step 5: Run; verify pass + the HTTP regression**

Run:
```bash
go test ./internal/accesslog/ -count=1
go test ./internal/filter/hcm/... -count=1   # HCM access-log callers still byte-identical
```
Expected: all PASS; `gofmt`/`golangci-lint` clean. The existing accesslog + HCM tests prove the `Default` path is byte-stable.

- [ ] **Step 6: Commit**

```bash
git add internal/accesslog/writer.go internal/accesslog/mongo_format.go internal/accesslog/mongo_format_test.go internal/accesslog/writer_test.go
git commit -m "phase 29.3 Task 7: accesslog pluggable Formatter seam (default Default byte-identical) + mongo JSON formatter + goldens (D-P7/D-S29.3-4)"
```

---

## Task 8: The mongo access-log sink construction + per-message emission both directions

**Files:**
- Modify: `internal/filter/network/mongoproxy/filter.go` (the sink construction gated on `cfg.accessLog`; the per-message emit on request + response decode)
- Modify: `internal/filter/network/mongoproxy/codec.go` (the `message.toString` per-opcode builder + the decode-time emit hook)
- Test: `internal/filter/network/mongoproxy/filter_test.go` / `codec_test.go`

The sink is constructed once (gated on `cfg.accessLog != ""`) at the factory; the per-connection filter submits a `MongoRecord` per decoded message in both directions. Timing-bearing → the proof is unit tests (the gated-off no-emit; the per-opcode string shape).

- [ ] **Step 1: Write the failing tests**

```go
func TestFilter_AccessLogGatedOffNoEmit(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", accessLog: "", commands: map[string]bool{}} // disabled
	f, _, _ := newTestFilterWithCB(t, cfg)
	buf := &network.Buffer{}
	buf.Append(msg(1, 2004, opQueryBody("db.collection1", 0, simpleQuery())))
	_ = f.OnData(buf, false)
	// No sink configured → no panic, no emit (the sink is nil; emit is a no-op).
}

func TestDecoder_MessageToStringQuery(t *testing.T) {
	// The per-opcode message string (full=true for requests). Exact format is the
	// IMPL transcription vs codec_impl.cc; this asserts the shape is non-empty +
	// names the opcode + collection (the golden is pinned at IMPL).
	s := messageToString(2004, 1, "db.collection1", true)
	if s == "" || !strings.Contains(s, "collection1") {
		t.Errorf("OP_QUERY message string wrong: %q", s)
	}
}
```
> A fuller test feeds a fake sink (a `Formatter`-backed in-memory `[]string`) and asserts one record per decoded message both directions. Add a sink-injection seam to the filter for testability (the factory sets `f.alSink`; tests set a fake).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_AccessLog|TestDecoder_MessageToString' -count=1`
Expected: compile error then FAIL.

- [ ] **Step 3: Add the sink construction (factory) + the filter field + the emit**

In `filter.go`, the factory constructs the sink ONCE when `cfg.accessLog != ""`:
```go
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		// ... existing parse ...
		cfg.stats = newMongoStats(reg, cfg.statPrefix)
		var alSink *accesslog.AsyncFileSink
		if cfg.accessLog != "" {
			// One sink per configured path (freeze-after-boot; the HCM precedent). The
			// dropped counter rides the existing accesslog ADR-0069 stat. Errors at boot
			// fail-fast (ADR-0072).
			s, err := accesslog.NewAsyncFileSinkWithFormatter(cfg.accessLog, accessLogDroppedCounter(reg), accesslog.MongoFormat)
			if err != nil {
				return nil, fmt.Errorf("mongo_proxy: access_log %q: %w", cfg.accessLog, err)
			}
			alSink = s
		}
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg, cfg.stats), alSink: alSink}
		}, nil
	}
}
```
> `accessLogDroppedCounter(reg)` returns the shared `server.accesslog_dropped` counter (ADR-0069 — reuse the existing accesslog dropped stat helper; if none is exported, the IMPL uses `reg.NewCounterIfAbsent("server.accesslog_dropped")`). The sink is SHARED across the listener's per-connection filter instances (constructed once at the factory, captured in the closure) — the AsyncFileSink is goroutine-safe (`Submit` is a non-blocking channel send).

Add the filter field + the emit helpers:
```go
type filter struct {
	network.Marker
	cfg        *compiledConfig
	dec        *decoder
	cb         network.ReadFilterCallbacks
	wcb        network.WriteFilterCallbacks
	delayTimer *time.Timer
	alSink     *accesslog.AsyncFileSink // 29.3 access log; nil when access_log is unset (no-op emit)
}

// emitAccessLog submits one MongoRecord per the decoder's pending log lines for
// THIS pass (both directions). A no-op when no sink is configured. The decoder
// accumulates per-message strings during decode (recordMessage); the filter drains
// + submits them with the wall-clock time + the upstream host. Timing-bearing →
// differential-invisible (AMEND-B10).
func (f *filter) emitAccessLog(lines []string) {
	if f.alSink == nil || len(lines) == 0 {
		return
	}
	host := "-"
	if f.cb != nil {
		if ra := f.cb.Connection().RemoteAddr(); ra != nil { // downstream host; upstream host is "-" at L4 (no upstream addr surfaced)
			host = ra.String()
		}
	}
	for _, msgStr := range lines {
		f.alSink.Submit(&accesslog.MongoRecord{Time: time.Now(), Message: msgStr, UpstreamHost: host})
	}
}
```
> **Upstream host:** upstream's mongo access log records `upstream_host`. At envoy-go's L4 seam the mongo filter sees no upstream address (tcp_proxy owns the upstream dial). Per the differential-invisibility (the access log is NEVER compared cross-side), `upstream_host` is recorded as the available address (downstream remote, or `-`); the IMPL pins the faithful value + records the boundary. This does NOT affect any gate (no fixture).

In `codec.go`, accumulate per-message log strings during decode (gated on `cfg.accessLog != ""`):
```go
// messageToString renders one decoded message per upstream codec_impl.cc:55-307
// (full=true for request-direction messages; full=false for replies). The exact
// per-opcode format is transcribed at IMPL vs v1.37.2; the access log is
// differential-invisible (AMEND-B10) so the unit goldens are the authority.
func messageToString(opCode int32, id int32, collection string, full bool) string {
	// ... IMPL: the per-opcode shapes (OP_QUERY/OP_INSERT/.../OP_REPLY/OP_COMMANDREPLY) ...
}
```
Add a `dec.logLines []string` accumulator (gated; appended at each successful decode, both directions) + a `takeLogLines()` the filter drains in `OnData`/`OnWrite`. Wire `f.emitAccessLog(f.dec.takeLogLines())` into `OnData` (replace the Task-6 `f.emitAccessLogRequests()` placeholder) and `OnWrite`:
```go
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	f.emitAccessLog(f.dec.takeLogLines())
	return network.Continue
}
```

- [ ] **Step 4: Run; verify pass**

Run: `go test ./internal/filter/network/mongoproxy/ -run 'TestFilter_AccessLog|TestDecoder_MessageToString' -count=1 && go test ./internal/filter/network/mongoproxy/ -count=1`
Expected: PASS; `gofmt`/`golangci-lint` clean. (The `import "github.com/esalaine/envoy-go/internal/accesslog"` is added to `filter.go`.)

- [ ] **Step 5: Commit**

```bash
git add internal/filter/network/mongoproxy/filter.go internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/*_test.go
git commit -m "phase 29.3 Task 8: mongo access-log sink (gated on access_log) + per-message emission both directions + message.toString goldens"
```

---

## Task 9: The drain-state accessor + `cx_drain_close` (reply-completion drain close, FlushWrite)

**Files:**
- Modify: `internal/filter/network/types.go` (`DrainState` interface)
- Modify: `internal/filter/network/callbacks.go` (`Draining()` on `ReadFilterCallbacks`)
- Modify: `internal/filter/network/chain.go` (`callbacks.Draining()`; `ChainRuntime.SetDrainState`; `rt.drain`)
- Modify: `internal/listener/manager.go` (`serveNetworkChain` calls `SetDrainState(rt.dm)`)
- Modify: `internal/filter/network/mongoproxy/{codec,filter}.go` (the list-empty drain-close on the reply path)
- Test: chain/manager/mongoproxy tests

`cx_drain_close`: on a correlated reply, when the active-query list becomes EMPTY and the drain signal is active → `cx_drain_close` +1 → `Connection().Close(FlushWrite)`. The drain decider threads as a narrow structural interface (no `*drain.Manager` leak; D-S29.3-3).

- [ ] **Step 1: Write the failing tests**

Chain (accessor):
```go
type fakeDrain struct{ draining bool }
func (f fakeDrain) IsDraining() bool { return f.draining }

func TestCallbacksDrainingAccessor(t *testing.T) {
	rt := NewChainRuntime(nil, &fakeConn{}, ConnFacts{})
	if rt.rt.cb.Draining() {
		t.Error("no drain state set → Draining() must be false")
	}
	rt.SetDrainState(fakeDrain{draining: true})
	if !rt.rt.cb.Draining() {
		t.Error("Draining() must reflect the set DrainState")
	}
}
```
Mongo (drain-close):
```go
func TestFilter_DrainCloseOnEmptyListWhenDraining(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg)
	cb.draining = true // the callbacks report Draining()==true
	// Send a query (appends to the active-query list), then a correlated reply (empties it).
	rbuf := &network.Buffer{}
	rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery())))
	_ = f.OnData(rbuf, false)
	wbuf := &network.Buffer{}
	wbuf.Append(respMsg(1 /*responseTo*/, 1 /*OP_REPLY*/, opReplyBody(0, 0, 0)))
	_ = f.OnWrite(wbuf, false)
	if v := ms.counters["cx_drain_close"].Load(); v != 1 {
		t.Errorf("cx_drain_close = %d, want 1 (list emptied while draining)", v)
	}
	if cb.closeType != network.FlushWrite || !cb.closed {
		t.Errorf("expected Connection().Close(FlushWrite), got closed=%v type=%v", cb.closed, cb.closeType)
	}
}

func TestFilter_NoDrainCloseWhenNotDraining(t *testing.T) {
	cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
	f, ms, cb := newTestFilterWithCB(t, cfg) // cb.draining == false
	rbuf := &network.Buffer{}; rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery()))); _ = f.OnData(rbuf, false)
	wbuf := &network.Buffer{}; wbuf.Append(respMsg(1, 1, opReplyBody(0, 0, 0))); _ = f.OnWrite(wbuf, false)
	if v := ms.counters["cx_drain_close"].Load(); v != 0 {
		t.Errorf("cx_drain_close = %d, want 0 (not draining)", v)
	}
	if cb.closed { t.Error("must not close when not draining") }
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ ./internal/filter/network/mongoproxy/ -run 'TestCallbacksDraining|TestFilter_DrainClose|TestFilter_NoDrainClose' -count=1`
Expected: compile errors then FAIL.

- [ ] **Step 3: Add `DrainState` + `Draining()`**

In `types.go`:
```go
// DrainState is the narrow drain-decider the network chain consumes (29.3, §3.4).
// *drain.Manager satisfies it structurally, so the network package does NOT import
// internal/drain (no manager leak into the framework's public API — D-S29.3-3).
type DrainState interface {
	IsDraining() bool
}
```
In `callbacks.go`, add to `ReadFilterCallbacks`:
```go
	// Draining reports whether the listener is draining (29.3, §3.4). mongo_proxy
	// gates cx_drain_close on it. False when no drain state is wired (nil-tolerant).
	Draining() bool
	// CloseDirection reports which side initiated the post-handoff close (29.3,
	// §3.5): Local (downstream) / Remote (upstream) / Unset. mongo_proxy keys
	// cx_destroy_local/remote_with_active_rq on it at OnDestroy.
	CloseDirection() CloseDirection
```
> Adding two methods to `ReadFilterCallbacks` ripples to EVERY implementor: the concrete `*callbacks` (chain.go) + every test fake that implements the interface. The TDD tasks update the fakes. The mongo `newTestFilterWithCB` fake gains `draining`/`closeDir` fields + the two methods.

In `chain.go`:
```go
func (c *callbacks) Draining() bool { return c.rt.drain != nil && c.rt.drain.IsDraining() }
func (c *callbacks) CloseDirection() CloseDirection { return CloseDirection(c.rt.closeDir.Load()) }

// SetDrainState wires the listener's drain decider into the per-connection chain
// (29.3, §3.4). Called by serveNetworkChain right after NewChainRuntime; nil-safe.
func (c *ChainRuntime) SetDrainState(d DrainState) { c.rt.drain = d }
```
(`CloseDirection` type lands in Task 10 / types.go; declare it here or in Task 10 — declare the type in types.go now to keep `callbacks.go` compiling.)

In `types.go`, add the `CloseDirection` type (needed by the accessor signature):
```go
// CloseDirection records which side initiated a post-handoff connection close
// (29.3, §3.5 — D-P4 CLOSED). The framework records close TYPE not DIRECTION
// pre-29.3 (reference_close_direction_framework_gap); tcp_proxy's pump-EOF-first
// recording (Task 10) supplies the direction for the terminal chain.
type CloseDirection int32

const (
	CloseDirectionUnset  CloseDirection = iota // no close recorded (or pre-handoff)
	CloseDirectionLocal                        // downstream-initiated (downstream EOF first)
	CloseDirectionRemote                       // upstream-initiated (upstream EOF first)
)
```

- [ ] **Step 4: Thread `rt.dm` in `serveNetworkChain`**

In `manager.go`, in `serveNetworkChain`, right after `rtChain := network.NewChainRuntime(...)`:
```go
	rtChain := network.NewChainRuntime(filters, dispatchConn, facts)
	if rt.dm != nil { // 29.3 (§3.4): the drain decider for cx_drain_close
		rtChain.SetDrainState(rt.dm)
	}
	defer rtChain.OnDestroy()
```
> **CRITICAL — the typed-nil-in-interface trap:** `rt.dm` is `*drain.Manager` and MAY be nil (legacy/test listener callers — `manager.go` "nil if drain is not wired"). Do NOT call `SetDrainState(rt.dm)` unconditionally: storing a typed-nil `*drain.Manager` into the `network.DrainState` interface yields a NON-nil interface value, so `callbacks.Draining()`'s `c.rt.drain != nil` guard would pass and `IsDraining()` would run on a nil `*Manager` receiver and PANIC (it dereferences `m.state`). Guard the call site with `if rt.dm != nil` (above) so `rt.drain` stays a true nil interface when drain is unwired → `Draining()` returns false. (Belt-and-suspenders: a unit test with a nil-dm listener proving no panic.) No `internal/drain` import is added to `manager.go` (it already imports it). The rest of `serveNetworkChain` is UNCHANGED (D-S29.3-2).

- [ ] **Step 5: Add the `cx_drain_close` on the reply path**

In `codec.go`'s `decodeReply`, the `takeQuery` hit already Decs the gauge; have it ALSO report whether the list is now empty, so the filter can decide the drain-close. Cleanest: the decoder signals "a correlated reply emptied the list" via a return/flag the filter reads. Add to `decodeReply` after the correlation:
```go
	if _, ok := d.takeQuery(responseTo); ok {
		d.stats.opQueryActive.Dec()
		d.mu.Lock()
		empty := len(d.queries) == 0
		d.mu.Unlock()
		if empty {
			d.replyEmptiedList = true // 29.3: a correlated reply drained the list (cx_drain_close candidate)
		}
	}
```
Add `replyEmptiedList bool` to the decoder + a `takeReplyEmptied() bool`. The FILTER (which holds `f.cb`) checks `Draining()` + closes — the increment + close are OUTSIDE `mu` (ADR-0223). In `filter.OnWrite`:
```go
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	f.emitAccessLog(f.dec.takeLogLines())
	if f.dec.takeReplyEmptied() && f.cb != nil && f.cb.Draining() {
		// cx_drain_close (SPEC §3.4): the active-query list emptied on a correlated
		// reply while draining → increment + close FlushWrite (the reply is flushed
		// first; the deferred close subsumes upstream's zero-ms timer — D-S29.3-7).
		f.cfg.stats.inc("cx_drain_close")
		f.cb.Connection().Close(network.FlushWrite)
	}
	return network.Continue
}
```
> `OnWrite` runs on the write pump goroutine (post-handoff). `f.cb` is the read callbacks (set at construction); `Draining()`/`Connection()` are read-only/atomic-safe. The `Connection().Close` records `closeReq`/`closeType` on the runtime (the existing deferred-close path honors it). No mid-query close (the list-empty check gates it — upstream's between-operations semantics).

- [ ] **Step 6: Run; verify pass + no regression**

Run:
```bash
go test ./internal/filter/network/ ./internal/filter/network/mongoproxy/ ./internal/listener/ -count=1
go test ./internal/filter/network/ -race -count=1
```
Expected: all PASS (the new fakes implement `Draining()`/`CloseDirection()`); `gofmt`/`golangci-lint` clean on the touched packages.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/network/types.go internal/filter/network/callbacks.go internal/filter/network/chain.go internal/listener/manager.go internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/filter.go internal/filter/network/*_test.go internal/filter/network/mongoproxy/*_test.go internal/listener/*_test.go
git commit -m "phase 29.3 Task 9: DrainState accessor (Draining/CloseDirection on callbacks) + cx_drain_close on reply-completion (FlushWrite)"
```

---

## Task 10: The close-direction recording (`tcp_proxy` pump-EOF-first → `chainRuntime.closeDirection`) + the `OnDestroy` direction-keyed `cx_destroy_*` increment (D-P4 CLOSED)

**Files:**
- Modify: `internal/filter/network/readconn.go` (the `SetCloseDirection` conn method → `rt.setCloseDirection`)
- Modify: `internal/filter/network/prefixconn.go` + `internal/filter/network/writeconn.go` (FORWARDING `SetCloseDirection` methods — the embedded `net.Conn` is an INTERFACE, so a custom method on `readChainConn` does NOT auto-promote through these wraps; see Step 3)
- Modify: `internal/filter/network/chain.go` (`setCloseDirection` first-wins CAS on `rt.closeDir`)
- Modify: `internal/filter/tcpproxy/filter.go` (record which pump EOF'd first)
- Modify: `internal/filter/network/mongoproxy/filter.go` (the `OnDestroy` direction-keyed increment)
- Test: tcpproxy / chain / readconn / mongoproxy tests (incl. a recording test THROUGH the actual wrap stack — not just a fake `cb`)

D-P4 CLOSED: `cx_destroy_local/remote_with_active_rq` become VALUE-parity. tcp_proxy records which pump EOF'd first (downstream→upstream = LOCAL; upstream→downstream = REMOTE) → `chainRuntime.closeDir` → the mongo `OnDestroy` increments the direction-keyed counter when the residual list was non-empty.

- [ ] **Step 1: Write the failing tests**

Chain/readconn (the recorder + accessor):
```go
func TestChainCloseDirectionRecording(t *testing.T) {
	rt := newChainRuntime([]ReadFilter{&filterB{}}, &fakeConn{}, connFacts{})
	rc := newReadChainConn(&fakeConn{}, rt)
	rc.SetCloseDirection(CloseDirectionLocal)
	if got := rt.cb.CloseDirection(); got != CloseDirectionLocal {
		t.Errorf("CloseDirection = %v, want Local", got)
	}
	rc.SetCloseDirection(CloseDirectionRemote) // first-wins: stays Local
	if got := rt.cb.CloseDirection(); got != CloseDirectionLocal {
		t.Errorf("first-wins violated: %v", got)
	}
}

// TestCloseDirectionThroughWrapStack proves the setter reaches the chainRuntime
// through the ACTUAL handoff composition writeChainConn(prefixConn(readChainConn))
// — NOT just a bare readChainConn. The embedded net.Conn is an INTERFACE, so the
// custom SetCloseDirection does not auto-promote; the forwarding methods on
// prefixConn + writeChainConn (Step 3) are what make this pass. WITHOUT them this
// test fails (the type-assert misses) — guarding against the silently-dead bug.
func TestCloseDirectionThroughWrapStack(t *testing.T) {
	rt := newChainRuntime([]ReadFilter{&filterB{}}, &fakeConn{}, connFacts{})
	inner := newReadChainConn(&fakeConn{}, rt)
	mid := newPrefixConn(inner, []byte("x"))                 // prefix present
	outer := newWriteChainConn(mid, []WriteFilter{})         // the conn tcp_proxy gets
	sd, ok := net.Conn(outer).(interface{ SetCloseDirection(CloseDirection) })
	if !ok {
		t.Fatal("the handed-off conn must expose SetCloseDirection through the wrap stack")
	}
	sd.SetCloseDirection(CloseDirectionRemote)
	if got := rt.cb.CloseDirection(); got != CloseDirectionRemote {
		t.Errorf("CloseDirection through wrap stack = %v, want Remote", got)
	}
}
```
> Constructor names (`newPrefixConn`/`newWriteChainConn`) per the as-built `prefixconn.go`/`writeconn.go`. Also add the no-prefix variant (`writeChainConn(readChainConn)` directly) — `handleTerminal` omits the prefixConn when the buffer is empty.
Mongo (`OnDestroy` direction-keyed):
```go
func TestFilter_OnDestroyCloseDirectionKeyed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		dir       network.CloseDirection
		answered  bool
		wantLocal, wantRemote int64
	}{
		{"local+active", network.CloseDirectionLocal, false, 1, 0},
		{"remote+active", network.CloseDirectionRemote, false, 0, 1},
		{"all-answered", network.CloseDirectionLocal, true, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &compiledConfig{statPrefix: "m", commands: map[string]bool{}}
			f, ms, cb := newTestFilterWithCB(t, cfg)
			cb.closeDir = tc.dir
			rbuf := &network.Buffer{}; rbuf.Append(msg(1, 2004, opQueryBody("db.c1", 0, simpleQuery()))); _ = f.OnData(rbuf, false)
			if tc.answered {
				wbuf := &network.Buffer{}; wbuf.Append(respMsg(1, 1, opReplyBody(0, 0, 0))); _ = f.OnWrite(wbuf, false)
			}
			f.OnDestroy()
			if got := ms.counters["cx_destroy_local_with_active_rq"].Load(); got != tc.wantLocal {
				t.Errorf("local = %d, want %d", got, tc.wantLocal)
			}
			if got := ms.counters["cx_destroy_remote_with_active_rq"].Load(); got != tc.wantRemote {
				t.Errorf("remote = %d, want %d", got, tc.wantRemote)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/filter/network/ ./internal/filter/network/mongoproxy/ -run 'TestChainCloseDirection|TestFilter_OnDestroyCloseDirection' -count=1`
Expected: compile errors (`SetCloseDirection`/`setCloseDirection`) then FAIL.

- [ ] **Step 3: Add the recorder (`chain.go` + `readconn.go`)**

In `chain.go`:
```go
// setCloseDirection records the post-handoff close direction first-wins (29.3,
// §3.5). Called by tcp_proxy via the readChainConn.SetCloseDirection forwarding on
// the pump goroutine; read by callbacks.CloseDirection() at OnDestroy (after the
// pumps join — a happens-after edge; atomic for race-cleanliness).
func (rt *chainRuntime) setCloseDirection(d CloseDirection) {
	rt.closeDir.CompareAndSwap(int32(CloseDirectionUnset), int32(d))
}
```
In `readconn.go`, add the actual setter on `readChainConn` (which holds `rt`):
```go
// SetCloseDirection lets the terminal (tcp_proxy) record which pump EOF'd first
// onto the chain runtime (29.3, §3.5). readChainConn is the INNERMOST wrap (it
// holds rt); the prefixConn/writeChainConn forwarding methods below relay to it.
func (r *readChainConn) SetCloseDirection(d CloseDirection) { r.rt.setCloseDirection(d) }
```
> **The promotion trap (the reviewer's catch):** `prefixConn` and `writeChainConn` embed `net.Conn` as an INTERFACE, not the concrete `*readChainConn`. Go promotes only the embedded INTERFACE's method set, so a custom `SetCloseDirection` on `*readChainConn` is NOT auto-surfaced on the outer `*writeChainConn` that `tcp_proxy` receives. Without explicit forwarding, `tcp_proxy`'s `downstream.(interface{ SetCloseDirection(...) })` type-assert misses → the recording is silently dead (and the fake-`cb` unit test would still pass, hiding it until the `0052` arm 4). Add EXPLICIT forwarding methods on BOTH wraps:

In `prefixconn.go`:
```go
// SetCloseDirection forwards to the embedded conn (the read-side seam, 29.3 §3.5):
// the embedded net.Conn is an interface so readChainConn.SetCloseDirection does not
// auto-promote — forward it explicitly so tcp_proxy reaches the runtime through the
// prefixConn wrap.
func (p *prefixConn) SetCloseDirection(d CloseDirection) {
	if sd, ok := p.Conn.(interface{ SetCloseDirection(CloseDirection) }); ok {
		sd.SetCloseDirection(d)
	}
}
```
In `writeconn.go`:
```go
// SetCloseDirection forwards to the embedded conn (the write-side seam, 29.3 §3.5):
// writeChainConn is the OUTERMOST wrap tcp_proxy receives; forward SetCloseDirection
// inward (→ prefixConn → readChainConn → runtime) since the embedded net.Conn
// interface does not promote the custom method.
func (w *writeChainConn) SetCloseDirection(d CloseDirection) {
	if sd, ok := w.Conn.(interface{ SetCloseDirection(CloseDirection) }); ok {
		sd.SetCloseDirection(d)
	}
}
```
(Field names `p.Conn`/`w.Conn` per the as-built `prefixconn.go`/`writeconn.go` embed. For a chain WITHOUT a write filter there is no `readChainConn` wrap at all — the forwarding terminates at a raw `net.Conn` with no `SetCloseDirection` → harmless no-op; non-mongo chains need no close-direction.)

- [ ] **Step 4: Record in `tcp_proxy` (which pump EOF'd first)**

In `tcpproxy/filter.go`'s `Handle`, replace the two pump goroutines + `wg.Wait`:
```go
	// 29.3 (§3.5): record which pump EOF'd first so a mongo_proxy chain can key
	// cx_destroy_local/remote_with_active_rq. The downstream→upstream pump returns
	// on downstream (LOCAL) EOF; the upstream→downstream pump on upstream (REMOTE)
	// EOF. Additive — chains without a readChainConn (no SetCloseDirection method)
	// are unaffected. The chain does first-wins CAS internally.
	recordDir := func(d network.CloseDirection) {
		if sd, ok := downstream.(interface{ SetCloseDirection(network.CloseDirection) }); ok {
			sd.SetCloseDirection(d)
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(netConn{upstream}, netConn{downstream})
		recordDir(network.CloseDirectionLocal) // downstream EOF first → local
		halfClose(upstream)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(netConn{downstream}, netConn{upstream})
		recordDir(network.CloseDirectionRemote) // upstream EOF first → remote
		halfClose(downstream)
	}()
	wg.Wait()
```
> Both pumps eventually finish (one EOFs, the `halfClose` triggers the other's EOF). The FIRST `recordDir` wins (the chain's CAS). For the all-answered/clean-close case the direction still records (local or remote) but the mongo `OnDestroy` only increments when the residual list was NON-empty → all-answered increments NEITHER (the test's third case). The recording is harmless for non-mongo chains (no `SetCloseDirection` method → no-op).

- [ ] **Step 5: The `OnDestroy` direction-keyed increment (mongo)**

In `mongoproxy/filter.go`, complete `OnDestroy` (folding the Task-6 timer cancel):
```go
func (f *filter) OnDestroy() {
	if f.delayTimer != nil {
		f.delayTimer.Stop() // best-effort (Task 6)
	}
	if f.dec != nil {
		n := f.dec.onDestroy() // drains residual + Decs the gauge; returns the residual count
		if n > 0 && f.cb != nil {
			// D-P4 CLOSED (SPEC §3.5): a non-empty active-query list at close keys the
			// direction-specific counter. The count is snapshotted under mu inside
			// onDestroy; the increment is OUTSIDE the lock (ADR-0223).
			switch f.cb.CloseDirection() {
			case network.CloseDirectionLocal:
				f.cfg.stats.inc("cx_destroy_local_with_active_rq")
			case network.CloseDirectionRemote:
				f.cfg.stats.inc("cx_destroy_remote_with_active_rq")
			}
		}
	}
	f.dec = nil
}
```
In `codec.go`, change `decoder.onDestroy()` to RETURN the residual count (update the 29.2 callers + tests):
```go
func (d *decoder) onDestroy() int {
	d.mu.Lock()
	n := len(d.queries)
	d.queries = nil
	d.mu.Unlock()
	if n > 0 {
		d.stats.opQueryActive.Add(int64(-n))
	}
	return n
}
```

- [ ] **Step 6: Run; verify pass + no regression**

Run:
```bash
go test ./internal/filter/network/ ./internal/filter/network/mongoproxy/ ./internal/filter/tcpproxy/ ./internal/listener/ -count=1
go test ./internal/filter/network/ ./internal/filter/tcpproxy/ -race -count=1
```
Expected: all PASS (the 29.2 `onDestroy()` callers updated for the new return; tcp_proxy unchanged for non-mongo chains — the existing tcp_proxy suite proves it); `gofmt`/`golangci-lint` clean.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/network/chain.go internal/filter/network/readconn.go internal/filter/tcpproxy/filter.go internal/filter/network/mongoproxy/codec.go internal/filter/network/mongoproxy/filter.go internal/filter/network/*_test.go internal/filter/tcpproxy/*_test.go internal/filter/network/mongoproxy/*_test.go
git commit -m "phase 29.3 Task 10: close-direction recording (tcp_proxy pump-EOF-first) + OnDestroy direction-keyed cx_destroy_* (D-P4 CLOSED)"
```

---

## Task 11: Fixture `0052-mongo-fault-delay` (cross-side; all arms) + README + the R4 break

**Files:**
- Create: `test/fixtures/0052-mongo-fault-delay/driver/driver.go`
- Create: `test/fixtures/0052-mongo-fault-delay/README.md`
- Modify: `test/differential/runner_test.go` (the `0052` driver blank-import; a REMOTE-close marker arm on `mongoRespondLoop` if needed for arm 4(ii))
- Possibly modify: `test/differential/fixture/fixture.go` (reuse `TCPMongoResponder = 30` — no new BackendKind)

The cross-side proof. The `0049` `MultiListenerDriver` (two listeners: delayed `mongo_d` + no-delay `mongo_nd`) + the `0051` `TCPMongoResponder` are the template. DETERMINISTIC 100%-probability delay; timing NEVER compared (only `delays_injected` value + the traffic-completes verdict).

- [ ] **Step 1: Author the driver (the SPEC §6.1 arms)**

Model on `test/fixtures/0049-mongo-requests/driver/driver.go` (`MultiListenerDriver`, the bootstrap render, `scrapeMongoStats`/`scrapeTypeLine`/`canonicalize`/`httpGet`) + `0051`'s `TCPMongoResponder` topology. Two listeners on each side:
- `mongo_d` (`stat_prefix: mongo_d`, `delay: {fixed_delay: 0.100s, percentage: {numerator: 100, denominator: HUNDRED}}`) → `TCPMongoResponder` backend.
- `mongo_nd` (`stat_prefix: mongo_nd`, no `delay`) → the same backend (the R1 non-perturbation arm).

Reference bootstrap delay block (both sides; HUNDRED is the default enum 0 → may be omitted, but spell it for clarity):
```yaml
            - name: envoy.filters.network.mongo_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy
                stat_prefix: mongo_d
                delay:
                  fixed_delay: 0.100s
                  percentage: { numerator: 100, denominator: HUNDRED }
```
Arms (the `driveProxy` body, both runner paths via `DriveReferenceMulti`/`DriveSubjectMulti`):
1. **fault-delay round-trip (pre + post handoff).** On `mongo_d`: send a query (requestID 1) → read the correlated OP_REPLY back (the responder replies after the request traverses; the ~100ms delay is invisible to correctness, only latency); send a SECOND query (requestID 2) on the SAME connection → read its reply. The first delay fires pre-handoff, the second post-handoff (via `replayRead`). Assert `mongo.mongo_d.delays_injected == 2` both sides; both replies received (passthrough-not-broken).
2. **seam non-perturbation (no-delay).** On `mongo_nd`: a query→reply round trip; assert `mongo.mongo_nd.delays_injected == 0` both sides; reply received (R1 live equivalence).
3. **`cx_drain_close`** (best-effort, D-S29.3-8). On a fresh `mongo_d` connection: send a query, read the reply (list empties), THEN trigger a listener drain via admin `POST /drain_listeners?graceful` (the phase-08.2 vehicle — confirm the admin route name against `internal/admin/`) on BOTH sides, then send a query→reply that empties the list while draining → assert `mongo.mongo_d.cx_drain_close >= 1` both sides + the connection closes. **If the cross-side drain timing is not deterministically reproducible**, DOWNGRADE this arm to PRESENCE-only (`cx_drain_close` line exists) + rely on the Task-9 unit value proof; record the downgrade in the README + PROGRESS.md.
4. **`cx_destroy_*` VALUE parity.** (i) LOCAL: open a connection, send a query with the `mongoMarkerWithhold` requestID (the responder withholds the reply → the query stays active), then CLOSE the connection from the driver (downstream/LOCAL close) → assert `cx_destroy_local_with_active_rq == 1` both sides. (ii) REMOTE: a query outstanding, then the RESPONDER closes its side (upstream/REMOTE close) — add a `mongoMarkerRemoteClose` requestID to `mongoRespondLoop` that reads the query then closes the backend conn → assert `cx_destroy_remote_with_active_rq == 1` both sides. (iii) all-answered: a query→reply (list empties), then close → assert NEITHER increments.
5. **all-quiesced roster.** After the arms: `delays_injected`/`cx_drain_close`/`cx_destroy_*` at their asserted values; `op_query_active == 0` both sides (the 29.2 gauge re-proven under fault load).

`AssertStats` scrapes both sides via `GET /stats/prometheus` (the `scrapeMongoStats` label-aware helper, reused) and compares the asserted counters; timing/duration is NEVER scraped or compared.

- [ ] **Step 2: Add the REMOTE-close marker to `mongoRespondLoop` (arm 4(ii))**

In `runner_test.go`, add `mongoMarkerRemoteClose int32 = 7006` and a case in `mongoRespondLoop`'s OP_QUERY switch: read the request, then `return` (closing the backend conn via the deferred `c.Close()`) WITHOUT replying → the upstream/REMOTE close while the query is active. (Distinct from `mongoMarkerWithhold`, which withholds but keeps the conn OPEN for the LOCAL-close arm.)

- [ ] **Step 3: Register + run the fixture cross-side**

Blank-import the `0052` driver in `runner_test.go` (the `0051` registration precedent). Run:
```bash
go test ./test/differential/ -run '0052|Mongo' -count=1 -v
```
Expected: `0052` PASS on BOTH runner paths (reference Envoy v1.37.2 docker + envoy-go subprocess). Use the docker-bridge-network topology (`reference_docker_probe_bridge_network`) if the reference side runs in docker.

- [ ] **Step 4: R4 deliberate-break liveness (`-count=1`)**

Per `reference_differential_break_protocol_count1`, prove each new assertion is LIVE. Temporarily, one at a time:
- (a) assert `delays_injected == 3` (when 2 are armed) → MUST FAIL both paths.
- (b) skip the `cx_destroy_*` direction-keyed increment (comment out the Task-10 `switch`) → arm 4 MUST FAIL (subject-side).
- (c) skip the `cx_drain_close` increment (comment out the Task-9 `f.cfg.stats.inc("cx_drain_close")`) → arm 3 MUST FAIL (or, if downgraded, the unit test MUST FAIL).
Each break + revert run with `-count=1`. Record the broken-FAIL + reverted-PASS outputs in the README + PROGRESS.md.

- [ ] **Step 5: Author the README + commit**

Write `test/fixtures/0052-mongo-fault-delay/README.md` (the `0051` README structure): the topology (two listeners), the arms, the deterministic-100%/timing-never-compared discipline, the access-log no-fixture note (AMEND-B10), the `cx_drain_close` drain-trigger disposition (D-S29.3-8 — driven or PRESENCE-downgraded), the R4 break record.
```bash
git add test/fixtures/0052-mongo-fault-delay/ test/differential/runner_test.go test/differential/fixture/fixture.go
git commit -m "phase 29.3 Task 11: 0052-mongo-fault-delay cross-side fixture (pre/post-handoff delay; cx_drain_close; cx_destroy_* value parity; R4 break) + README"
```

---

## Task 12: The completion bundle — ADR-0226 body + BEHAVIOR_CONTRACT + STATE/ROADMAP rollup + next-prompt + the six-gate

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0226 §Decision/§Consequences body IN PLACE; no new number)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 29.3 bundle)
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`
- Modify: `internal/filter/network/mongoproxy/doc.go` (the package-doc forward-pointers → LANDED)

ONE atomic bundle (ADR-0052). The parent-row-29 ROLLUP lands here.

- [ ] **Step 1: ADR-0226 §Decision/§Consequences body in place**

Fill the ADR-0226 §Decision + §Consequences bodies (the §Context already exists at `DECISIONS.md:14521`) per ADR-0044 (no new ADR number; DECISIONS tail STAYS ADR-0226; next-free ADR-0227). The body's blueprint is SPEC §3 (the three-extension async halt/resume seam §3.1; fault-delay §3.2; the access-log formatter seam §3.3; `cx_drain_close` §3.4; the close-direction seam D-P4 CLOSED §3.5) + the D-S29.3-1..8 PLAN resolutions above. Record the seam design (the `MayHalt` haltable gate + the mutex/cond hold + `resumeReady`), the drain factory-vs-callbacks decision (callbacks, one folded ripple), and the access-log differential-invisibility.

- [ ] **Step 2: BEHAVIOR_CONTRACT 29.3 bundle**

Add to the `### envoy.filters.network.mongo_proxy` subsection: the fault-delay semantics (per-request-message eval; the five-as-built request-callback set reconciled to the upstream six; re-entrancy guard; `delays_injected` at arm; StopIteration-while-pending; deterministic 100% arms); the access-log semantics (the JSON line format both directions; the timing-bearing differential-invisibility boundary; the upstream-host boundary); the `cx_drain_close` reply-completion drain semantics; the `cx_destroy_*` close-direction VALUE parity (D-P4 CLOSED). Add the NEW framework subsection `### Network filter chain framework — async halt/resume (29.3 amendment)`: the `MayHalt` haltable gate, the block-the-dispatcher hold (halt mutex + cond + `resumeReady`), the post-handoff withhold-until-resume (the 28.1b §3.5 boundary lifted for halt purposes only), the never-halting byte-identical equivalence (R1), the close-direction accessor, the drain-state accessor. Add the NEW coverage boundaries: the access-log timing-bearing differential boundary (AMEND-B10); the runtime-key-gating boundary (§2.6 — fault/drain/logging keys at defaults). Stat table: **360 → 360** (+0 creation; `delays_injected`/`cx_drain_close`/`cx_destroy_*` go increment-active — explicitly a no-creation increment-wiring delta). Add the parent-row-29 family ROLLUP note (the FOURTH §9 Network-filters-family row CLOSED; 3 candidates remain — `redis`/`kafka_broker`/`thrift`).

- [ ] **Step 3: STATE.md + ROADMAP.md (the ROLLUP) + next-prompt.txt + doc.go**

- ROADMAP: sub-row 29.3 `in-progress → done` **AND parent row 29 `in-progress → done` ATOMICALLY** (the ROLLUP — the 18/19/22/24/25/26/28 precedent).
- STATE.md: advance `active-phase`/`lifecycle-state`/`last-commit` (controller fills the squash SHA post-merge); counts: fixtures **54** (tail `0052-mongo-fault-delay`), fuzzers **39**, stats **360**, BackendKind tail **30**, DECISIONS tail **ADR-0226** (next-free **ADR-0227**); `next-skill` = the NEXT phase cold-start (the §9 family's remaining `redis`/`kafka_broker`/`thrift`, or the next ROADMAP row — per SKILL_ROUTING the next phase opens with BRAINSTORM or SPEC).
- next-prompt.txt: rewrite for the NEXT-phase cold-start.
- `mongoproxy/doc.go`: flip the 29.3 forward-pointers to LANDED (fault/log/drain/close-direction LANDED; phase-29 CLOSED).

- [ ] **Step 4: The six-gate (run honestly; quote outputs into PROGRESS.md)**

```bash
go build ./...
go vet ./...
golangci-lint run
go test ./... -race -short
ls -d test/fixtures/[0-9]* | wc -l            # 54
go test ./test/differential/ -count=1         # FULL suite byte-exact (54 dirs incl. the 53-dir seam-back-compat gate — R1)
# h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — the accesslog formatter seam keeps the HTTP path byte-identical; 29.3 touches no HTTP filter)
```
Expected: build/vet/lint clean; `-race -short` PASS; the FULL 54-dir suite byte-exact PASS (incl. the 53-dir R1 back-compat / seam non-perturbation proof); h2spec 53/53 + proxy-wasm 10/10. Quote all outputs into PROGRESS.md. Per `reference_differential_break_protocol_count1`, the R4 + R-HALT breaks were run with `-count=1` (Tasks 4 + 11).

- [ ] **Step 5: Commit (the atomic completion bundle)**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md next-prompt.txt internal/filter/network/mongoproxy/doc.go PROGRESS.md
git commit -m "phase 29.3 Task 12: ADR-0226 body + BEHAVIOR_CONTRACT 29.3 bundle + parent-row-29 ROLLUP + six-gate green"
```

---

## Acceptance checklist (SPEC §13.3 — the IMPL is done when ALL hold)

1. The async halt/resume seam lands per §3.1 (active-async `ContinueReading`; cross-goroutine safety behind the `haltable` gate; post-handoff withhold-until-resume); never-halting chains byte-identical (R1 — the full 53-dir suite green); the seam `-race` test passes + the deliberate-break proved the mutex necessary (R-HALT, `-count=1`).
2. Fault-delay injection lands per §3.2 (per-request-message eval; re-entrancy guard; `delays_injected` at arm; StopIteration-while-pending; deterministic 100% arms; timer cancel on destroy).
3. The mongo access log lands per §3.3 (the JSON formatter seam; HCM byte-identical; unit goldens + the coverage boundary; NO fixture — AMEND-B10 / D-P7).
4. `cx_drain_close` lands per §3.4 (reply-completion + drain-active → FlushWrite — R-DRAIN; unit value proof + the best-effort `0052` arm / PRESENCE downgrade per D-S29.3-8); `cx_destroy_*` close-direction VALUE parity lands per §3.5 (D-P4 CLOSED — R-CLOSEDIR; local/remote/all-answered).
5. Fixture `0052` green; counts: fixtures 53→**54**, fuzzers **39** (unchanged), stats **360** (unchanged), BackendKind **30** (unchanged) (R6).
6. ADR-0226 §Decision/§Consequences body in place (DECISIONS.md tail STAYS ADR-0226; no new number); the BEHAVIOR_CONTRACT 29.3 bundle lands (§8 — incl. the framework async-halt/resume subsection).
7. Six gates green (§13.2); STATE.md advanced; ROADMAP sub-row 29.3 `in-progress → done` **AND parent row 29 `in-progress → done` ATOMICALLY** (the ROLLUP); next-prompt.txt rewritten for the NEXT-phase cold-start.

---

## Appendix — risk register (the implementer reads this first)

- **The seam is genuine concurrency (Tasks 2–4) — TDD it with the synthetic `haltingFilter`, NOT mongo.** The seam lands GREEN (incl. `-race`) before Task 5 wires mongo in. If the cond/`resumeReady` coordination resists `-race`, the IMPL MAY switch the primitive to a per-connection release channel (D-S29.3-1 latitude) — the tests (`TestSeamRace*` + the byte-identity regression) are the arbiter; the SEMANTICS (block-the-dispatcher, release exactly once, never-halting byte-identical) are fixed.
- **The post-handoff resume advance in `ContinueReading` (Task 4 Step 4) is the trickiest code.** The PLAN pins `replayHeld bool` as the park-state (do NOT infer from `replayIdx==0`). For the single-read-filter mongo chain the re-dispatch loop is a no-op (reset+drain+release); keep the general N-filter loop only if it stays `-race`-clean, else simplify + note the completeness boundary.
- **The close-direction setter does NOT auto-promote (Task 10) — the silently-dead trap.** `prefixConn`/`writeChainConn` embed `net.Conn` as an INTERFACE, so `readChainConn.SetCloseDirection` is invisible on the outer conn `tcp_proxy` receives. The forwarding methods on BOTH wraps (Task 10 Step 3) are MANDATORY, and `TestCloseDirectionThroughWrapStack` (through the real composition, NOT a fake `cb`) is the guard — without it the fake-`cb` unit test passes while the real chain never records, and `cx_destroy_*` is silently zero.
- **The typed-nil-in-interface trap (Task 9).** Guard `SetDrainState(rt.dm)` with `if rt.dm != nil` — a typed-nil `*drain.Manager` in the `DrainState` interface is non-nil and would panic in `Draining()`.
- **The five-vs-six request-callback reconciliation (Task 5)** — the as-built codec has five request decoders (no request-side `decodeCommandReply`); the parent §11.6 six-callback set maps to upstream's request decoder. Transcribe against `proxy.cc` v1.37.2; `0052` only needs `decodeQuery`. Record the reconciliation.
- **`cx_drain_close` cross-side reproducibility (D-S29.3-8)** — land the UNIT value proof first (Task 9, deterministic); the `0052` differential arm is best-effort with a PRESENCE downgrade. Do NOT block the phase on a flaky differential drain arm.
- **The accesslog `chan *Record → chan any` change (Task 7)** touches every HCM access-log caller's `Submit` site only structurally (`*Record` satisfies `any`); the `Default` byte path is the regression gate — the existing accesslog + HCM tests MUST stay byte-identical.
- **`ReadFilterCallbacks` grows two methods (Task 9)** — every test fake implementing it must gain `Draining()`/`CloseDirection()`. Grep `func.*ReadFilterCallbacks` + the fakes in `chain_test.go`/`mongoproxy` tests and update them in the same task.
