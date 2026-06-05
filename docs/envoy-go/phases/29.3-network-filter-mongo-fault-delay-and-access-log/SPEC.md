# Phase 29.3 SPEC — the async halt/resume seam + `mongo_proxy` fault-delay + the mongo access log + `cx_drain_close` + the close-direction seam

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 29.3** (`network-filter-mongo-fault-delay-and-access-log`), the THIRD and FINAL of the phase-29 BRAINSTORM-time 3-way pre-split (29.1 / 29.2 / 29.3). It is authored per the phase-22.2 / phase-25.x / phase-28.2 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/29-network-filter-mongo-proxy/SPEC.md`) is this sub-phase's MASTER — its §3.2 (the 29.3 scope), §4.1 (the async halt/resume seam shape), §7 (the `delays_injected`/`cx_drain_close`/`cx_destroy_*` created-at/incremented-at split), §8.4 (the `0052` fixture envelope), §11.6 (fault-delay semantics), §11.7 (continueReading re-dispatch), §11.8 (access-log format), §11.10 (close direction + drain posture), and §12 (D-P5/D-P7/D-P12) remain authoritative; this SPEC EXECUTES + REFINES them into the per-Task surface. The phase-29 parent BRAINSTORM + parent SPEC already drafted the **ADR-0226 §Context** (the 29.3 charter, `DECISIONS.md:14521`), so this sub-phase is **authored, NOT re-brainstormed** (the 28.2-SPEC-after-28.1-IMPL precedent). The next session, per BOOTSTRAP §5, authors the **29.3 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Land the framework's FIFTH structural extension — the **async halt/resume seam** (ACTIVE asynchronous `ContinueReading` + cross-goroutine safety + post-handoff read-halt honoring) — and its FIRST consumer, mongo **fault-delay injection** (`delay` FaultDelay; `delays_injected` at timer-arm; deterministic 100%-probability differential arms); land the **mongo access log** (`access_log` path; the timing-bearing JSON formatter seam → unit goldens + coverage boundary, NO fixture dir); land **`cx_drain_close`** (reply-completion drain close, FlushWrite); CLOSE the deferred **close-direction seam** (D-P4 → `cx_destroy_local/remote_with_active_rq` VALUE parity via a minimal close-direction accessor); prove it cross-side with fixture **`0052-mongo-fault-delay`**; and land the **parent-row-29 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 29.3). Never-halting chains stay byte-identical (R1).

**Architecture:** Unlike 29.1/29.2 (framework-ZERO-touch), 29.3 is the **framework-SURGERY** sub-phase — the ONE consolidated ripple (ADR-0219) where all the deferred framework work converges, precisely because the async halt/resume seam already opens the `chain.go` / `readconn.go` / `tcp_proxy` pump area. The `internal/filter/network/` chain framework gains: (i) halt-state synchronization on `chainRuntime` (a per-connection mutex + a hold-and-release primitive, behind an atomic fast-path so never-halting chains pay no measurable cost); (ii) an async-active `ContinueReading` that advances past the halting filter, dispatches buffered bytes, and re-evaluates terminal readiness (pre-handoff) or releases held bytes to the terminal's pump (post-handoff); (iii) `replayRead` + `readChainConn.Read` honoring a post-handoff StopIteration (withhold-until-resume — lifting the 28.1b SPEC §3.5 observational boundary for HALT purposes only); (iv) a minimal **close-direction accessor** (which `tcp_proxy` pump EOF'd first → `chainRuntime.closeDirection` → a callbacks accessor); (v) a minimal **drain-state accessor** threaded to the network filter callbacks (the `*drain.Manager` is already at the listener runtime — `rt.dm`; 29.3 exposes it). The `mongoproxy` package consumes all of this: fault delay (timer + `ContinueReading`), the access log (a formatter-seam'd `internal/accesslog` sink), `cx_drain_close` (drain accessor + `Connection().Close(FlushWrite)`), and the close-direction-keyed `cx_destroy_*` increment. `internal/accesslog` gains a pluggable formatter seam (HCM callers byte-identical). `TerminalFilter.Handle` signature UNCHANGED; HCM untouched; zero new exported API on the halt seam itself.

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); the as-built `internal/filter/network/mongoproxy/` package (29.1 request + 29.2 response — extended in place); the as-built `internal/filter/network/` chain framework (`chain.go`/`readconn.go`/`writeconn.go`/`callbacks.go`/`types.go` — EXTENDED per §3.1); `internal/filter/tcpproxy/` (the two pump goroutines — the close-direction recording site, §3.5); `internal/accesslog/` (`AsyncFileSink` + `Default` — a pluggable formatter seam added, §3.3); `internal/drain/` (the phase-08.2 `*drain.Manager` — `IsDraining()`, consumed not modified, §3.4); `internal/filter/http/fault/` (the FaultDelay `rollPercent`/`time.AfterFunc` eval precedent, §3.2); the differential harness + `fixture.StatsAsserter` + the `0051` driver template. ZERO new third-party `go.mod` dependencies.

**Authored:** 2026-06-05. **Empirical-pin probe date (inherited):** 2026-06-03 (parent SPEC §11.6/§11.7/§11.8/§11.10). **Baseline-anchor re-pin date:** 2026-06-05 (this SPEC session — §9.1).

---

## 1. Purpose / Mission

Phase 29.3 delivers the seam + fault-delay + access-log + drain + the close-direction close, and the family ROLLUP (parent §3.2 item "29.3"; ADR-0226):

1. **The async halt/resume seam (ADR-0226 — the framework's FIFTH structural extension).** The as-built per-pass-stop semantics (`chain.go` `runData` parks `resumeIdx` at a filter that returned StopIteration; the next read re-dispatches it; handoff is deferred while parked) ALREADY mirror upstream's per-read repeated-StopIteration model pre-handoff (parent AMEND-B13). What is missing — the THREE real extensions (§3.1): (i) **ACTIVE asynchronous `ContinueReading`** (a resume from a `time.AfterFunc` goroutine, outside any dispatch, must actively advance past the halting filter, dispatch the buffered bytes, and re-check terminal readiness — today it degenerates to a flag nothing consumes); (ii) **cross-goroutine safety** (the timer-goroutine resume races the pre-handoff read loop or pump A's `replayRead` on unlocked `chainRuntime` state); (iii) **post-handoff read-halt honoring** (`replayRead` ignores Status + `readChainConn.Read` returns bytes to the terminal's pump regardless — a post-handoff StopIteration is a silent no-op; 29.3 makes it withhold-until-resume). Upstream `std::next(filter->entry())` parity: the halting filter is NOT re-invoked by the resume.

2. **Fault-delay injection (the seam's first consumer; ADR-0226).** The `delay` FaultDelay (parsed + PGV-validated at 29.1 — AMEND-B9) is CONSUMED: per-decoded-request-message evaluation with a re-entrancy guard (parent §11.6); a `rollPercent`-style percentage gate (the phase-09 precedent; deterministic at 100% — no RNG); a `time.AfterFunc` timer → `ContinueReading()` on fire; **`delays_injected` +1 at timer-ARM time**; `OnData`/replay returns StopIteration while the timer pends; the timer is cancelled on connection destroy. This CONSUMES the ADR-0221 §Consequences anticipated-halt-consumer allowance, REFINED to the READ side (per upstream `onData` semantics — not the anticipated write side).

3. **The mongo access log (ADR-0226).** The `access_log` path (parsed at 29.1) is CONSUMED: one JSON line per decoded message in BOTH directions — `{"time": "<wall-clock>", "message": <message.toString>, "upstream_host": "<addr|->"}` (parent §11.8; request messages `full=true`, replies `full=false`). The format is timing-bearing → differential log comparison is NOT viable (AMEND-B10) → the proof is formatter unit goldens + a BEHAVIOR_CONTRACT coverage boundary; **NO access-log fixture dir** (fixture count stays at the `0052` tail). `internal/accesslog` gains a pluggable formatter seam (D-P7 RESOLVED — §3.3).

4. **`cx_drain_close` (ADR-0226).** On a correlated reply, when the active-query list becomes EMPTY and a drain decision is active → `cx_drain_close` +1 → `Connection().Close(FlushWrite)` (parent §11.10; close BETWEEN operations, never mid-query). The drain decision rides the phase-08.2 `*drain.Manager` via a minimal drain-state accessor on the network filter callbacks (§3.4).

5. **The close-direction seam — D-P4 CLOSED (ADR-0226).** The 29.2 D-P4 coverage boundary (`cx_destroy_local/remote_with_active_rq` exist-at-zero / presence-only) is RESOLVED to VALUE parity: a minimal close-direction accessor (which `tcp_proxy` pump EOF'd first → `chainRuntime.closeDirection` → a callbacks accessor) + the direction-keyed increment at `OnDestroy` when the active-query list is non-empty (§3.5). The framework records close TYPE not DIRECTION (`reference_close_direction_framework_gap`); 29.3's halt seam already opens the pump/terminal/`chain.go` area, so the close-direction ripple lands HERE (ADR-0219 one-ripple).

6. **The integration surface** — (a) fixture **`0052-mongo-fault-delay`** (cross-side `StatsAsserter`; DETERMINISTIC 100%-probability arms; `delays_injected` parity; a `cx_drain_close` arm; the `cx_destroy_*` VALUE-parity arms; timing NEVER compared); (b) the framework **back-compat regression gate** (the full 53-dir suite byte-exact — the seam's R1 equivalence proof); (c) the ADR-0226 §Decision/§Consequences body + the BEHAVIOR_CONTRACT 29.3 bundle + the **parent-row-29 ROLLUP** (parent row 29 + sub-row 29.3 → `done` ATOMICALLY) + the six-gate.

After phase 29.3, the project has: a complete `mongo_proxy` (request + response + fault delay + access log + drain + full `cx_destroy_*` parity); the framework's async halt/resume seam (consumed once, byte-identical for everyone else); the FOURTH §9 Network-filters-family row CLOSED. Three family candidates remain (`redis`/`kafka_broker`/`thrift`).

### 1.1 Parent AMENDs + 29.1/29.2 outputs load-bearing for 29.3

- **AMEND-B8 / §11.6** (fault-delay semantics — per-decoded-request-message eval; re-entrancy guard; `delays_injected` at arm; StopIteration-while-pending; timer cancel on close) — §3.2.
- **AMEND-B13 / §4.1** (the seam REFRAMED: the three real extensions = active-async-resume + cross-goroutine safety + post-handoff honoring; upstream has no persistent filter-manager halt) — §3.1.
- **AMEND-B10 / §11.8** (the access log is timing-bearing JSON → unit-test + coverage-boundary fallback; the formatter seam, D-P7) — §3.3.
- **AMEND-B12 / §11.10** (`cx_destroy_*` need close DIRECTION; an as-built framework gap; the drain = reply-completion + FlushWrite) — §3.4/§3.5.
- **Parent §11.7 continueReading pin** (resume at `std::next(filter->entry())` — the filter AFTER the halting one; fresh socket reads re-dispatch from filter 0; socket reads continue during the halt) — §3.1.
- **29.1/29.2 outputs consumed:** the parsed `delay` config (`config.go` — `delayConfigured`/`fixedDelay`/`delayPercentNum`/`delayPercentDenom`; AMEND-B9 PGV-validated; never consumed at 29.1/29.2); the parsed `access_log` path (`config.go` — `accessLog string`; never consumed); the per-connection `decoder` + its `mu sync.Mutex` + the active-query list with the `start time.Time` per entry (`codec.go` — recorded at 29.1, drained at 29.2 `OnDestroy`); the `delays_injected`/`cx_drain_close`/`cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq` counters (all created eagerly at 29.1; presence-only in `0051`; NEVER incremented through 29.2 — §5.1).

### 1.2 29.3-SPEC-additive contributions (what this document pins beyond the parent + 29.1/29.2)

- **§3.1 The async halt/resume seam design (D-P12 RESOLVED at the SEMANTIC level; the mechanism carried to IMPL).** The synchronization shape (a per-`chainRuntime` halt mutex + a hold-and-release primitive, behind an atomic fast-path so the never-halting hot path is byte-identical), the three-context `ContinueReading` (parked-OnNewConnection / re-entrant-in-OnData / NEW async-from-timer), the `replayRead`+`readChainConn.Read` post-handoff withhold-until-resume, and the terminal-readiness re-evaluation after an async resume. The SPEC pins the SEMANTICS + the equivalence guarantee (R1); the exact primitive (Mutex+Cond vs a release channel; block vs withhold) is an IMPL D-question (D-S29.3-1) the `-race` + back-compat gates settle.
- **§3.4 The drain-state accessor.** The `*drain.Manager` is threaded to the listener runtime (`rt.dm`) but NOT reachable from a network filter today. 29.3 threads a minimal drain-state signal to the network filter callbacks (via `network.FactoryCtx`) so the mongo filter can gate `cx_drain_close`. A small accessor (anticipated: `Draining() bool` on the callbacks, backed by the listener's manager), NOT a leak of the whole `*drain.Manager` — the exact surface is D-S29.3-3.
- **§3.5 The close-direction accessor (D-P4 CLOSED).** Which `tcp_proxy` pump EOF'd first → `chainRuntime.closeDirection` (local = downstream-initiated; remote = upstream-initiated) → a callbacks accessor → the mongo `OnDestroy` direction-keyed `cx_destroy_*` increment. The minimal close-direction surgery the 29.2 D-P4 boundary deferred to here.
- **§3.3 The access-log formatter seam (D-P7 RESOLVED).** A pluggable formatter on `internal/accesslog`'s `AsyncFileSink` (default = the HTTP `Default` → existing HCM callers byte-identical) + a mongo record carrier + the mongo JSON formatter; the exact carrier shape (extend `Record` / a typed-payload formatter / a sibling sink) is D-S29.3-4.
- **§6 fixture `0052-mongo-fault-delay`** (the deterministic 100%-probability fault arms + the `cx_drain_close` arm + the `cx_destroy_*` VALUE-parity arms) + the seam back-compat regression gate.
- **D-P5 RESOLVED** (§10.1): the AMEND-B9 delay-PGV reject arms stay UNIT-TEST-ONLY (`0050` carries the `stat_prefix` arm only — the zookeeper D-P4 precedent); a `header_delay`-configured mongo filter is parse-accept-no-delay (the `FixedDelayProvider` path is never taken) — a unit-test arm.

---

## 2. Non-purposes

Phase 29.3 does NOT extend any subsystem beyond the minimum needed to land the seam + its consumers under ADR-0226.

- **2.1 No new exported halt-seam API.** The seam deepens the SEMANTICS of `ContinueReading()` + `replayRead` + `readChainConn.Read` — none of which are exported-surface changes (parent §4.1). `TerminalFilter.Handle` signature is UNCHANGED; the `ReadFilter`/`WriteFilter`/`Status` interfaces are unchanged. The NEW callbacks accessors (drain-state, close-direction) are minimal interface additions, NOT changes to the iteration protocol.
- **2.2 HCM + the HTTP path are UNTOUCHED.** The `internal/accesslog` formatter seam keeps the HTTP `Default` as the default formatter → every existing HCM access-log caller is byte-identical (the regression gate proves it). No HTTP filter, no `internal/http/`, no h2 path changes. h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected.
- **2.3 No write-side halt.** The fault delay halts the READ path (`OnData`/replay returns StopIteration; the ADR-0226 §Context REFINEMENT of the ADR-0221 anticipation). `OnWrite` ALWAYS returns `Continue` (the response decoder never halts — parent §11.5). The `injectWriteDataToFilterChain`/`disableClose` WriteFilterCallbacks surface stays deferred (mongo needs neither).
- **2.4 No access-log fixture; no differential log comparison.** The access log is timing-bearing JSON (AMEND-B10) → unit goldens + a BEHAVIOR_CONTRACT coverage boundary; NO `0053` access-log dir (fixture count tail stays `0052`).
- **2.5 The dynamic HISTOGRAM families stay DEFERRED** (ADR-0060) — `cmd.<cmd>.reply_*`, `collection.<c>.query.reply_*`, callsite `reply_*` remain unmirrored project-wide (the `start time.Time` per active query stays the unconsumed latency basis). No new fixed stats are created (all 23 created at 29.1 — §5.1).
- **2.6 No runtime-key layer.** `mongo.proxy_enabled` / `mongo.logging_enabled` / `mongo.connection_logging_enabled` / `mongo.drain_close_enabled` / `mongo.fault.fixed_delay.percent` / `.duration_ms` — envoy-go has no runtime layer; the filter behaves at key defaults (sniffing on, logging on when a path is configured, drain-close on, fault from the proto). Recorded as the envoy-go-strict departure (the parent §2.1 / §7.5 boundary; the 29.3 BEHAVIOR_CONTRACT bundle records it).
- **2.7 No new built-in; no new `name.go` arm; no new fuzzer.** mongoproxy is already the 8th built-in (29.1); the `mongo.` four-rule tag-extractor arm already handles `delays_injected`/`cx_drain_close`/`cx_destroy_*`; the 39th `FuzzMongoDecode` already covers both directions (29.2). 29.3 wires increments + the seam + a `-race` halt-concurrency test only.
- **2.8 No retroactive zookeeper changes; no real-MongoDB-server fixtures; no OP_MSG decode; no per-route surface** — all per parent §2. The async halt/resume seam is consumed ONLY by mongo fault-delay; zookeeper (never-halting) is byte-identical through it (R1).

---

## 3. The seam + fault-delay + access-log + drain + close-direction (ADR-0226)

Extends the as-built `internal/filter/network/` chain framework + the `internal/filter/network/mongoproxy/` package (29.1+29.2) IN PLACE, plus a formatter seam on `internal/accesslog/`. This is the framework-SURGERY sub-phase (contrast §4 with the 29.1/29.2 zero-touch).

### 3.1 The async halt/resume seam (the framework extension; LOAD-BEARING)

The as-built anchors this extends (verified this session against master tip — §9.1):

- `chain.go` `runData()` — the dispatch loop; OnData StopIteration parks `resumeIdx` at the filter, `connHalted` NOT set; the next read re-dispatches it (the as-built per-pass-stop ALREADY mirrors upstream pre-handoff per-read repeated-StopIteration — AMEND-B13).
- `chain.go` `callbacks.ContinueReading()` — two as-built paths: a parked OnNewConnection halt (`connHalted` true → synchronous resume: `connHalted=false`, `resumeIdx++`, `runData()`); a re-entrant-in-OnData call (sets `resumeRequested`, consumed by `runData`). **THE GAP:** an ASYNC third context — called from a timer goroutine while no dispatch is running, with `resumeIdx` parked at a StopIteration'd filter — currently degenerates to setting a flag nothing consumes.
- `chain.go` `replayRead()` — the post-handoff observational pass; ignores Status; drains the buffer unconditionally (the 28.1b §3.5 boundary). **THE SECOND GAP.**
- `readconn.go` `readChainConn.Read` — returns socket bytes to the terminal's pump regardless of filter Status. **THE THIRD GAP.**
- `chain.go` `terminalReady()` / `handleTerminal` — handoff is deferred while a filter is parked (`resumeIdx < len(filters)`); the wrap composition `writeChainConn(prefixConn(readChainConn(rawConn)))` exists only for ≥1-write-filter chains (mongo qualifies — it registers a WriteFilter).
- `internal/filter/tcpproxy/filter.go` — the two pump goroutines (`io.Copy` each direction; `wg.Wait()`); the §3.5 close-direction recording site.

**The three extensions (the pinned shape; the mechanism is D-S29.3-1):**

1. **ACTIVE asynchronous `ContinueReading`.** A `ContinueReading()` arriving when the chain is NOT dispatching and `resumeIdx` is parked at a StopIteration'd filter must ACTIVELY resume: advance `resumeIdx` PAST the halting filter (upstream `std::next(filter->entry())` parity — the halting filter is NOT re-invoked with the same bytes), dispatch the buffered bytes to the remaining chain, and re-evaluate terminal readiness (a halt that deferred handoff hands off after resume). Pre-handoff, this runs `runData` from `resumeIdx`; if `terminalReady()` becomes true the deferred handoff fires (the listener loop already performs the handoff when `TerminalReady()` — the resume must make that condition reachable from the timer goroutine, anticipated via the same handoff path the read loop drives, or a resume that schedules it; D-S29.3-2).

2. **Cross-goroutine safety.** The resume originates on a `time.AfterFunc` goroutine; pre-handoff it races the listener read-loop goroutine (`onData`/`runData`), post-handoff it races pump A's `replayRead`. The synchronization is the **ADR-0223 minimal-critical-section discipline**: a per-`chainRuntime` halt mutex guarding EXACTLY the halt/resume state (`connHalted`/`resumeIdx`/the post-handoff halt flag/the held-buffer handle) — nothing else. **The never-halting hot path pays NO measurable cost:** the synchronization is gated behind a cheap atomic fast-path (an `atomic.Bool` "a halt is live / this chain is haltable" check) so a chain that never halts (zookeeper, every existing filter, mongo with no `delay` configured) takes the byte-identical pre-29.3 path — the lock + the hold primitive engage ONLY when a halt is actually in progress. This is the R1 equivalence guarantee made concrete.

3. **Post-handoff read-halt honoring.** While a filter has halted the chain post-handoff (returned StopIteration from a replayed `OnData` and not yet resumed): `replayRead` must STOP dispatching at the halting filter, NOT drain the held bytes; and `readChainConn.Read` must NOT return those bytes to the terminal's pump — it withholds them until `ContinueReading` releases (the pump blocks or the bytes are withheld-and-retried — the block-vs-withhold mechanism is D-S29.3-1). On resume the held bytes flow to the remaining filters and then to the pump (upstream: the accumulated read buffer re-dispatches at the next filter). For never-halting filters the 28.1b §3.5 pure-observation path is unchanged — the boundary is lifted for HALT purposes ONLY.

**Why post-handoff is load-bearing (ADR-0226 §Context).** With the production chain `[mongo_proxy, tcp_proxy]`, terminal handoff happens after the first successful `OnData` pass — so in steady state ALL mongo decode runs post-handoff via `replayRead` on the terminal's downstream-pump goroutine. A fault delay firing on the second-or-later message MUST be honored on the post-handoff path or it is invisible. The pre-handoff path matters for a delay firing on the very first message (the common `0052` fixture case) and for framework-level unit parity. **Both paths are specified; both are tested.**

`TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM untouched except the §3.5 close-direction recording; zero-write-filter chains + never-halting chains byte-identical (R1). Anticipated zero new exported API on the seam itself (the new callbacks accessors in §3.4/§3.5 are minimal additions, not iteration-protocol changes).

### 3.2 Fault-delay injection (the seam's first consumer; parent §11.6)

The `delay` FaultDelay was parsed + PGV-validated at 29.1 (`config.go` — `delayConfigured bool`, `fixedDelay time.Duration` [> 0s], `delayPercentNum uint32`, `delayPercentDenom int32`; AMEND-B9). 29.3 consumes it, mirroring upstream `tryInjectDelay` (`proxy.cc:434-449`):

- **Evaluation point: per decoded request-direction message, with a re-entrancy guard.** Upstream calls `tryInjectDelay` at the entry of each request-direction decode callback. The parent §11.6 source read enumerates the upstream set as **`decodeQuery`/`decodeInsert`/`decodeGetMore`/`decodeKillCursors`/`decodeCommand`/`decodeCommandReply` — NOT `decodeReply`** (note it includes `decodeCommand` + the request-path `decodeCommandReply` beyond the proto doc's four named operations Query/Insert/GetMore/KillCursors); this six-callback pin is inherited verbatim, the IMPL transcribing it against `proxy.cc` v1.37.2 (`reference_wire_format_both_sides_see_same_bytes`) rather than re-deriving — D-S29.3-5 scopes only that `0052` exercises OP_QUERY (the load-bearing arm). An armed timer suppresses re-evaluation (the re-entrancy guard). The decoder (which holds `cfg`) makes the roll+duration decision per request message; the filter (which holds the `ReadFilterCallbacks`) owns the timer + the resume.
- **Percentage gate: `rollPercent`-style; DETERMINISTIC at 100%.** The phase-09 precedent (`internal/filter/http/fault/fault.go` — `percentageToFloat` FractionalPercent → [0,100]; `rollPercent` with `p >= 100 → true` / `p <= 0 → false` deterministic, no RNG; a per-instance `*rand.Rand` for intermediate). The `0052` arms use 100% probability → deterministic → no timing nondeterminism in the differential.
- **Timer + resume: `time.AfterFunc` → `ContinueReading()` on fire.** When a delay results: increment **`delays_injected` at ARM time** (synchronous, before/at the `AfterFunc` creation — upstream `stats_.delays_injected_.inc()` at arm), arm `f.delayTimer = time.AfterFunc(duration, f.onDelayTimer)`, and the filter's `OnData`/replay returns **StopIteration** while the timer pends. `f.onDelayTimer` (on the timer goroutine): clear the re-entrancy guard, nil the timer, call `f.cb.ContinueReading()` (the §3.1 async-active path — the load-bearing cross-goroutine resume).
- **The filter↔decoder split.** The decoder signals a newly-armed delay (anticipated: a `takePendingDelay() (time.Duration, bool)` consumed once by the filter after `decodeOnData`); the filter arms the timer + returns StopIteration. The re-entrancy guard (anticipated: an `atomic.Bool` on the decoder or the filter, set at arm, cleared at fire) is touched cross-goroutine (the read goroutine arms; the timer goroutine clears) → it is synchronized (the phase-09 `markedActive atomic.Bool` precedent). The EXACT split + the guard's home is D-S29.3-6.
- **Cancel on destroy.** `OnDestroy` stops the timer (best-effort `timer.Stop()`; the phase-09 precedent + upstream's dtor-asserts-no-pending-timer). The timer goroutine + `OnDestroy` race on the timer handle → the at-most-once discipline (the phase-09 `CompareAndSwap` precedent) governs; D-S29.3-6.

`OnData` (and the post-handoff replay path which calls `OnData`) returns StopIteration ONLY while a delay timer pends; absent a configured `delay`, mongo never halts (byte-identical — R1). The held bytes are exactly the in-flight request being delayed; on timer fire they flow downstream→upstream (the delayed traffic completes — the passthrough-not-broken proof, §6.2 arm 1).

### 3.3 The mongo access log (parent §11.8; D-P7 RESOLVED)

The `access_log` path was parsed + stored at 29.1 (`config.go` — `accessLog string`; empty = disabled). 29.3 consumes it, mirroring upstream `AccessLog::logMessage` (`proxy.cc:37-57`): when a path is configured, one JSON line per decoded message (BOTH directions) — `{"time": "<wall-clock timestamp>", "message": <message.toString>, "upstream_host": "<addr|->"}` — request-direction messages log with `full=true` (documents dumped), replies with `full=false` (documents as counts). The `message.toString()` shapes per opcode (`codec_impl.cc:55-307`) are transcribed into the formatter unit goldens at PLAN/IMPL.

**The `time` field is per-message wall clock → the format is timing-bearing → cross-side log comparison is NOT viable (AMEND-B10).** The proof is formatter unit goldens (against pinned inputs, with the timestamp the only non-deterministic field — asserted by shape/regex, not value) + a BEHAVIOR_CONTRACT coverage boundary. **NO fixture dir** (parent §8.5's total reaches 54 at 29.3 via `0052`; the access log adds no dir, so the tail stays `0052`).

**D-P7 RESOLVED: a pluggable formatter seam on `internal/accesslog` (the PREFERRED model; the carrier shape is D-S29.3-4).** As-built, `AsyncFileSink.run()` hard-wires `s.f.Write(Default(r))` (`writer.go:88`); the `Default` formatter + the `Record` struct are HTTP-shaped. 29.3 adds a pluggable formatter so the existing HTTP path stays byte-identical AND mongo can emit its JSON:

- **(a) Pluggable formatter on `AsyncFileSink` (PREFERRED).** Add a `Formatter` function type + an optional `formatter` field to `AsyncFileSink` (default = `Default` → every existing `NewAsyncFileSink` HCM caller byte-identical); the `run()` loop calls `s.formatter(r)` instead of the hard-wired `Default(r)`. Mongo constructs a sink with its JSON formatter. The OPEN sub-question (D-S29.3-4): the carrier — the HTTP `Record` has no `message`/`upstream_host`-as-mongo-string field, so the mongo record needs a carrier. Anticipated: a minimal mongo record type + a mongo formatter, with the sink generalized to carry it (the least-invasive of: extend `Record` with an opaque field the mongo formatter reads / a typed-payload formatter / a mongo-owned sibling sink reusing the async-writer machinery). The HTTP path's bytes are UNCHANGED regardless (the regression gate proves it).
- The mongo access-log sink is constructed when `cfg.accessLog != ""` (at filter config-parse / boot — the freeze-after-boot discipline; one sink per configured path, the HCM precedent). The per-connection filter submits records on decode.

**Differential-invisibility (AMEND-B10).** The access log has zero cross-side observability under the differential (the timing-bearing `time` field). The proof surface is unit goldens + the coverage boundary, NOT a fixture. The `internal/accesslog` formatter-seam change is regression-gated by the existing HTTP access-log fixtures + the HCM unit tests staying byte-identical.

### 3.4 `cx_drain_close` (parent §11.10; the drain integration)

Upstream (`proxy.cc:254-271, 290-292`): on a correlated reply, when the active-query list becomes EMPTY and `drain_decision_.drainClose(DrainDirection::All)` is true (and runtime `drain_close_enabled`) → `cx_drain_close` +1 → a ZERO-ms timer → `connection().close(FlushWrite)`. The close is BETWEEN operations (never mid-query).

**envoy-go 29.3 integration:**

- **The drain signal.** The phase-08.2 `*drain.Manager` (`internal/drain/manager.go` — `IsDraining() bool`, lock-free atomic) is the drain decision. It is threaded to the listener runtime (`rt.dm`) but is NOT reachable from a network filter today (the as-built gap — §9.1). 29.3 threads a minimal drain-state signal to the network filter callbacks via `network.FactoryCtx` (the factory already receives the manager-bearing context; the listener manager holds `dm`). The mongo filter gates the drain-close on this signal. The exact accessor surface (a `Draining() bool` on the callbacks, backed by the manager — NOT a leak of the whole `*drain.Manager`) is D-S29.3-3.
- **The eval point.** On the response path (`OnWrite` → `decodeReply` → `takeQuery` hit → the active-query list now empty): if the drain signal is active → `cx_drain_close` +1 → `f.cb.Connection().Close(network.FlushWrite)`. The `Connection.Close(CloseType)` surface exists (`callbacks.go` — `FlushWrite`/`NoFlush`; the concrete impl records `closeReq`+`closeType` on the runtime, honored by the read loop / pump teardown). The mongo filter rides this existing close path — it does NOT need its own socket close.
- **No mid-query close.** The drain-close only fires when the list is EMPTY after a correlated reply (upstream's between-operations semantics). The list-empty check is under the per-connection `mu` (the 29.2 correlation lock); the `cx_drain_close` increment + the `Close` call are outside it (the ADR-0223 discipline).
- **The omitted zero-ms timer.** Upstream arms a zero-ms dispatcher timer before closing (a deferral to the next event-loop iteration). envoy-go's `Connection.Close` is already a deferred-close request (it sets `closeReq`; the read loop / pump teardown performs the actual close after the current pass) → the zero-ms timer is structurally subsumed; recorded as a faithful simplification (D-S29.3-7 confirms at IMPL there is no observable difference for the `0052` drain arm).

### 3.5 The close-direction seam — D-P4 CLOSED (parent §11.10 / AMEND-B12)

Upstream keys `cx_destroy_local_with_active_rq` vs `cx_destroy_remote_with_active_rq` on `Network::ConnectionEvent::LocalClose` vs `RemoteClose` delivered to `onEvent` while the active-query list is non-empty (`proxy.cc:355-376`). The 29.2 D-P4 resolution left this a COVERAGE BOUNDARY (presence-only) because the framework records close TYPE not DIRECTION and threading direction requires a `tcp_proxy` + `chain.go` touch (`reference_close_direction_framework_gap`). 29.3 — the framework-surgery sub-phase, already in the pump/terminal/`chain.go` area for §3.1 — CLOSES it:

- **The recording site.** For a `[mongo_proxy, tcp_proxy]` chain, the post-handoff close is detected inside `tcp_proxy`'s two pump goroutines (`tcpproxy/filter.go` — each `io.Copy` returns on its side's EOF; the downstream→upstream pump returning first = downstream/LOCAL initiated; the upstream→downstream pump returning first = upstream/REMOTE initiated). 29.3 records which pump EOF'd FIRST (anticipated: a small `atomic` on the runtime / a `CompareAndSwap`-guarded first-writer-wins, the digest pattern) → a `chainRuntime.closeDirection` (local / remote / unset).
- **The accessor.** A minimal `CloseDirection()` accessor on the network filter callbacks (or the `Connection` surface) returns the recorded direction; the concrete impl reads `chainRuntime.closeDirection`. The exact surface is D-S29.3-3 (folded with the drain accessor — one callbacks-extension ripple).
- **The increment.** The mongo `OnDestroy` (which already drains the residual active-query list under `mu` + Decs the gauge — 29.2) reads the close direction; if the residual list was NON-EMPTY at close → `cx_destroy_local_with_active_rq` +1 (local) or `cx_destroy_remote_with_active_rq` +1 (remote). When the list was empty (all queries answered before close) → neither increments (upstream parity). The increment is OUTSIDE `mu` (the count is snapshotted under the lock, the 29.2 drain pattern).
- **Value parity.** `0051`'s presence-only `cx_destroy_*` arms become VALUE-parity arms at 29.3 (re-asserted in `0052`, and the `0051` README's D-P4 boundary note is CLOSED — the value is now compared). The pre-handoff pure-read close path (`manager.go` read loop — downstream EOF only) keys LOCAL for non-terminal chains; for the production terminal chain the pump-direction recording governs. **By design the pre-handoff LOCAL-keying path has NO differential coverage** — the mongo production chain is ALWAYS terminal (`[mongo_proxy, tcp_proxy]`), so every `0052` close runs the post-handoff pump-direction path; the pre-handoff keying is proven by unit tests only (§13.1 Layer A local/remote/unset) — a recorded completeness boundary, not a gap.

This is the close of the 29.2 SUB-PHASE boundary (the analogue of the 29.1 OP_REPLY recognized-not-decoded boundary closed at 29.2).

### 3.6 File delta

| File | 29.3 change |
|---|---|
| `internal/filter/network/chain.go` | halt-state synchronization on `chainRuntime` (the halt mutex + the atomic fast-path + the post-handoff halt flag + the held-buffer handle); `ContinueReading` async-active third context (advance-past-halting-filter + dispatch + re-eval terminal readiness / release held bytes); `replayRead` honors StopIteration (stop + hold, no drain); `closeDirection` field + recording; the `CloseDirection()`/`Draining()` callbacks accessors' concrete impl |
| `internal/filter/network/readconn.go` | `readChainConn.Read` post-handoff withhold-until-resume (behind the atomic fast-path; byte-identical when no halt is live) |
| `internal/filter/network/callbacks.go` | the `Draining() bool` + `CloseDirection()` accessors on `ReadFilterCallbacks`/`Connection` (minimal additions) |
| `internal/filter/network/types.go` | `network.FactoryCtx` gains the drain-state handle (threaded from the listener manager's `dm`) |
| `internal/filter/tcpproxy/filter.go` | record which pump EOF'd first → `chainRuntime.closeDirection` (the §3.5 close-direction site) |
| `internal/listener/manager.go` | pass the drain handle into `network.FactoryCtx` at chain construction (the `dm` already on `rt`) |
| `internal/accesslog/writer.go` | the pluggable `Formatter` seam on `AsyncFileSink` (default = `Default`; the `run()` call site) + the mongo record carrier (D-S29.3-4) |
| `internal/accesslog/format.go` (or a new mongo formatter file) | the mongo JSON formatter (`time`/`message`/`upstream_host`) + the per-opcode `message.toString` shapes |
| `mongoproxy/codec.go` | the per-request-message fault-delay roll+duration decision + the re-entrancy guard + `takePendingDelay`; the access-log record emission on decode (both directions); the `cx_drain_close` list-empty check on the correlated-reply path; the `cx_destroy_*` direction read at `onDestroy` |
| `mongoproxy/filter.go` | the fault-delay timer (`time.AfterFunc` + `onDelayTimer` + `ContinueReading`) + StopIteration-while-pending in `OnData`/replay; the access-log sink construction (gated on `cfg.accessLog`); the `cx_drain_close` `Connection().Close(FlushWrite)`; the `OnDestroy` close-direction-keyed increment + timer cancel |
| `mongoproxy/config.go` | (no parse change — `delay`/`access_log` already parsed at 29.1; 29.3 consumes them) |
| `mongoproxy/stats.go` | (no roster change — `delays_injected`/`cx_drain_close`/`cx_destroy_*` already eager; 29.3 calls `ms.inc(...)`) |
| `mongoproxy/doc.go` | the package-doc 29.3 forward-pointers updated (fault/log/drain/close-direction LANDED; phase-29 CLOSED) |
| `*_test.go` (network + mongoproxy + accesslog) | the seam unit + `-race` cross-goroutine tests; the fault-delay + access-log-formatter + drain + close-direction unit tests |
| `test/fixtures/0052-mongo-fault-delay/` | the cross-side driver + the deterministic arms |

---

## 4. Framework touchpoints — the SURGERY sub-phase (contrast 29.1/29.2 zero-touch)

29.1 and 29.2 were framework-ZERO-touch (the 28.2 property). **29.3 is the ONE consolidated framework ripple** (ADR-0219) — every deferred framework concern converges here because the async halt/resume seam (§3.1) already opens the `chain.go` / `readconn.go` / `tcp_proxy`-pump area. The production diff to `internal/filter/network/` (the framework files), `internal/filter/tcpproxy/`, `internal/listener/manager.go`, and `internal/accesslog/` is REAL but BOUNDED + REGRESSION-GATED:

- **The halt seam** (`chain.go`/`readconn.go`/`callbacks.go`) — semantics-deepening behind an atomic fast-path; never-halting chains byte-identical (R1 — the full 53-dir suite is the regression gate).
- **The close-direction recording** (`tcpproxy/filter.go` + `chain.go`) — additive (records which pump EOF'd first); no behavior change for any non-mongo consumer.
- **The drain-state threading** (`types.go`/`manager.go`/`callbacks.go`) — additive accessor; the `dm` is already on the listener runtime.
- **The accesslog formatter seam** (`internal/accesslog/`) — additive (default `Default` preserves HCM byte-identity).

This is a shared-framework extension, NOT a network-iteration-protocol change — the `ReadFilter`/`WriteFilter`/`Status`/`TerminalFilter.Handle` contracts are unchanged (§2.1). **The full 53-dir existing fixture suite + h2spec 53/53 + proxy-wasm 10/10 MUST stay green at the 29.3 six-gate** — the seam's R1 back-compat / non-perturbation gate (parent R1; the `0052` no-delay arm + every never-halting fixture).

---

## 5. Stat surface (cross-reference parent §7; +0 creation)

### 5.1 No creation delta — 360 → **360**

All 23 fixed mongo stats were created EAGERLY at 29.1 (D-P1). 29.3 wires increments only: `delays_injected` goes increment-active (at timer-arm); `cx_drain_close` goes increment-active (reply-completion drain close); `cx_destroy_local_with_active_rq` + `cx_destroy_remote_with_active_rq` go increment-active (the D-P4 close-direction VALUE parity — §3.5). All four were created eagerly at 29.1, presence-only in `0051` at 29.2, NEVER incremented until now. **Project stat surface stays 360** (the 28.2 / 29.2 "+0 creation" precedent). The dynamic `cmd.*`/`collection.*`/callsite counters remain excluded from the static count; the dynamic HISTOGRAM families stay deferred (ADR-0060, §2.5).

### 5.2 Prometheus exposition — NO new `name.go` arm

The 29.1 `mongo.` four-rule tag-extractor arm (`name.go`; AMEND-C1) already handles `mongo.<sp>.delays_injected` / `.cx_drain_close` / `.cx_destroy_local_with_active_rq` / `.cx_destroy_remote_with_active_rq` → `envoy_mongo_<leaf>{envoy_mongo_prefix="<sp>"}`. 29.3 adds NO new arm; the `0052` driver reuses the `0049`/`0051` label-aware scrape helpers (`scrapeMongoStats`).

### 5.3 Departure flags + coverage boundaries (the 29.3 BEHAVIOR_CONTRACT subset)

- **The access-log differential-comparison boundary (AMEND-B10 — §3.3).** Timing-bearing JSON → unit goldens + a coverage boundary; NO fixture dir.
- **The runtime-key-gating boundary (parent §2.1 / §7.5 — §2.6).** `mongo.drain_close_enabled` / `mongo.fault.*` / `mongo.logging_enabled` etc. at defaults; the filter behaves at the proto-configured values. Recorded HERE (the 29.3 bundle is the natural home — fault + drain + logging all consult these keys upstream).
- **The dynamic-HISTOGRAM families DEFERRED (ADR-0060 — §2.5).** Carried forward; the `start time.Time` per active query stays the unconsumed latency basis.
- The 29.1/29.2-landed departures (boot-window eager creation; the `stats.IsValidName` guard; the dynamic-metadata differential-invisibility) carry forward unchanged.

---

## 6. The proof surface — fixture `0052` + the access-log unit goldens

Per `reference_differential_fixture_dispatch_constraint`: `0052` is CROSS-SIDE (one runner branch). Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Numbering continues from `0051` (the 29.2 tail): 29.3 lands **`0052`** → 53 → **54** dirs. The access log gets NO dir (AMEND-B10 — §3.3).

**Fixture-design constraints (carry-over + the fault specifics):** (i) DETERMINISTIC 100%-probability fixed delays — `delay: {fixed_delay: 0.100s, percentage: {numerator: 100, denominator: HUNDRED}}` (parent §8.4) — so `delays_injected` is exactly-once-per-delayed-message both sides; (ii) the delay DURATION is NEVER compared (BOOTSTRAP §7.2 timing discipline; only `delays_injected` value + the traffic-completes verdict); (iii) the responder MUST emit correlated OP_REPLY bytes (the `0051` `TCPMongoResponder` BackendKind 30 — reused, no new backend); (iv) all stat assertions post-first-connection (AMEND-B4); (v) decoding-error arms use FRESH connections (AMEND-B6); (vi) the seam non-perturbation arm uses a no-`delay` listener (byte-identical — R1).

### 6.1 `0052-mongo-fault-delay` (cross-side; the load-bearing fixture)

**Topology.** Chain `[mongo_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 docker + envoy-go subprocess). A delayed listener (`stat_prefix: mongo_d`, `delay: {fixed_delay: 0.100s, percentage: {numerator: 100, denominator: HUNDRED}}`) routes to ONE `TCPMongoResponder` backend (BackendKind 30 — reused from `0051`). A SECOND no-delay listener (`stat_prefix: mongo_nd`) for the seam non-perturbation arm. The `MultiListenerDriver` precedent (`0049`) carries the two configs.

**StatsAsserter mechanics.** The driver implements `fixture.StatsAsserter`; both sides scraped via `GET /stats/prometheus`; the `0049`/`0051` label-aware scrape (`scrapeMongoStats`) reused verbatim. No new harness machinery.

**Arms (the SPEC-anticipated spine; the PLAN/IMPL finalizes):**

1. **fault-delay round-trip (the load-bearing arm; PRE-handoff + POST-handoff).** A query through the delayed listener → `delays_injected` +1 both sides; the delayed traffic STILL completes (the responder's correlated OP_REPLY round-trips AFTER the ~100ms delay; the driver reads the reply back) — the passthrough-not-broken proof. Drive ≥2 messages on one connection so a delay fires BOTH on the first message (pre-handoff, before terminal handoff) AND on a later message (post-handoff via `replayRead` — the §3.1 load-bearing path); `delays_injected` reflects each armed delay both sides. The re-entrancy guard is exercised (a multi-message buffer arms at most one delay per pass).
2. **seam non-perturbation (no-delay listener; R1).** A query through `mongo_nd` (no `delay`) → `delays_injected` stays 0; byte-identical request/reply behavior (the seam does not perturb the non-faulted path — the R1 equivalence proof in a live fixture, complementing the full-suite back-compat gate).
3. **`cx_drain_close` (drain-close arm).** With the drain signal active (the driver triggers a listener drain — the phase-08.2 drain path; D-S29.3-8 confirms the fixture's drain-trigger mechanism), a query→reply round trip that empties the active-query list → `cx_drain_close` +1 both sides + the connection closes FlushWrite (the reply is flushed first). If the drain-trigger is not differentially controllable in the fixture harness, `cx_drain_close` falls back to PRESENCE + a unit-test value proof (D-S29.3-8 — anticipated: the fixture drives it; the phase-08.2 admin `/drain` or a listener-drain signal is the vehicle).
4. **`cx_destroy_*` VALUE parity (D-P4 CLOSED — §3.5).** (i) a connection closed by the driver (downstream/LOCAL close) with a query OUTSTANDING (the responder withholds the reply, the `0051` unanswered-arm pattern) → `cx_destroy_local_with_active_rq` +1 both sides (VALUE compared — the 29.2 presence-only boundary CLOSED); (ii) an upstream/REMOTE close with a query outstanding (the responder closes its side) → `cx_destroy_remote_with_active_rq` +1 both sides; (iii) a connection whose queries were all answered before close → NEITHER increments both sides.
5. **all-quiesced roster.** After the arms: `delays_injected` / `cx_drain_close` / `cx_destroy_*` at their asserted values both sides; `op_query_active` == 0 both sides (the 29.2 gauge re-proven green under the fault load).
6. **deliberate-break liveness proof (R4; the `0030`/`0049`/`0051` lesson):** recorded in driver comments + README + PROGRESS.md at IMPL — e.g. (a) temporarily asserting `delays_injected == 2` (when 1 is armed) MUST fail on both runner paths with `-count=1`; (b) temporarily skipping the `cx_destroy_*` direction-keyed increment MUST fail arm 4 (subject-side); (c) temporarily skipping the `cx_drain_close` increment MUST fail arm 3. Both reverted; recorded. Per `reference_differential_break_protocol_count1`, the deliberate-break + any seam break run with `-count=1` (the go-test-cache stale-PASS trap).

### 6.2 The access-log unit goldens (NO fixture; AMEND-B10 — §3.3)

The mongo JSON formatter is proven by unit goldens at `internal/accesslog` (or `mongoproxy`): pinned `message.toString` inputs per opcode (request `full=true`; reply `full=false`) → the exact `{"time":..., "message":..., "upstream_host":...}` line, with the `time` field asserted by shape (the timestamp format), NOT value. The HTTP `Default` formatter's bytes are asserted UNCHANGED (the seam regression — the existing HCM access-log fixtures + format unit tests stay green). A BEHAVIOR_CONTRACT coverage-boundary entry records the no-differential-comparison.

### 6.3 Counts

53 → **54** at 29.3 phase-done (+1; tail `0052-mongo-fault-delay`). The full 53-dir existing suite is the seam back-compat gate (§4) and re-runs byte-exact green at the six-gate. Fuzzers stay **39** (no new fuzzer — §7); stat surface stays **360** (§5.1). No new conformance harness.

---

## 7. The fuzzer + the seam concurrency test (no count change — stays 39)

mongo's decoder is direction-agnostic; the 39th `FuzzMongoDecode` (extended to both directions at 29.2) already covers the decode surface. 29.3 adds NO fuzzer (count stays **39**). The seam's cross-goroutine safety (§3.1) is proven instead by a dedicated `-race -count=N` concurrency test: a `time.AfterFunc`-style async `ContinueReading` racing the read loop (pre-handoff) and `replayRead` (post-handoff), asserting (a) the held bytes are released exactly once on resume; (b) no race report with the halt mutex, AND an IMMEDIATE race report WITHOUT it (the 28.2/29.2 R9 deliberate-break precedent — run with `-count=1` per `reference_differential_break_protocol_count1`); (c) never-halting chains take the byte-identical fast path (no lock contention observable). The fault-delay timer↔OnDestroy race (the at-most-once timer cancel — §3.2) gets its own `-race` test (the phase-09 `markedActive` precedent).

---

## 8. Behavior-contract delta (the 29.3 bundle; per ADR-0052 atomic landing)

ONE atomic bundle at the 29.3 IMPL final task:

- The `### envoy.filters.network.mongo_proxy` subsection gains: the fault-delay semantics (per-request-message eval; re-entrancy guard; `delays_injected` at arm; StopIteration-while-pending; deterministic differential arms); the access-log semantics (the JSON line format both directions; the timing-bearing differential-invisibility boundary); the `cx_drain_close` reply-completion drain semantics; the `cx_destroy_*` close-direction VALUE parity (D-P4 CLOSED — the 29.2 presence-only boundary resolved).
- **NEW framework subsection:** `### Network filter chain framework — async halt/resume (29.3 amendment)` — the active-async `ContinueReading`, the cross-goroutine safety (the halt mutex + the atomic fast-path), the post-handoff withhold-until-resume (the 28.1b §3.5 boundary lifted for halt purposes only), the never-halting byte-identical equivalence (R1), the close-direction accessor, the drain-state accessor.
- **NEW coverage boundaries:** the access-log timing-bearing differential boundary (AMEND-B10); the runtime-key-gating boundary (§2.6 — fault/drain/logging keys at defaults).
- Stat table: **360 → 360** (+0 creation; `delays_injected` / `cx_drain_close` / `cx_destroy_*` go increment-active; explicitly a no-creation increment-wiring delta).
- The **parent-row-29 family ROLLUP note** (the FOURTH §9 Network-filters-family row CLOSED; 3 candidates remain — `redis`/`kafka_broker`/`thrift`).

---

## 9. SPEC-time empirical pins

The 29.3 SPEC does NOT re-execute the parent §11 D29-1..D29-12 pins (resolved once at the parent SPEC; inherited) NOR the 29.1 §11.2 D-P2 re-probe (the dynamic-stat shapes are pinned). The fault/drain/close-direction counters use the SAME four-rule `mongo.` arm already proven; the access log is differential-invisible (no probe needed). No new live probe is required — the seam + fault semantics are pinned from SOURCE (parent §11.6/§11.7/§11.8/§11.10) + the as-built framework (verified at §9.1).

### 9.1 D-S29.3-0 — master-tip baselines + as-built anchors VERIFIED at this SPEC session

Verified against master tip (`git log --oneline -1` at this session = the docs-only `735798e` trailing the 29.2-IMPL squash `620a3d0`). These are the source of the §12 Task-1 first-action gate; the IMPL Task-1 RE-RUNS them against the live IMPL-session tip.

- **Differential fixture-dir count = 53**; numbering tail = **`0051-mongo-responses`**. 29.3 lands `0052` → **54**. Recipe `ls -d test/fixtures/[0-9]* | wc -l` = 53.
- **Fuzzer count = 39** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`). 29.3 adds NONE → stays **39**.
- **Stat surface = 360** (BEHAVIOR_CONTRACT stat table; +23 mongo at 29.1). 29.3 lands +0 → **360**.
- **BackendKind tail = 30** (`TCPMongoResponder`; reused by `0052` — no new BackendKind).
- **DECISIONS.md tail = ADR-0226** (`DECISIONS.md:14521` ADR-0226 §Context); **next-free ADR-0227**. The 29.3 IMPL fills the ADR-0226 §Decision/§Consequences body IN PLACE (no new ADR number; ADR-0044).
- **As-built framework anchors re-verified** (the §3.1/§3.4/§3.5 surgery anchors): `internal/filter/network/chain.go` — `runData()` (the per-pass-stop dispatch loop), `callbacks.ContinueReading()` (the two as-built paths + the async GAP), `replayRead()` (the post-handoff observational pass — the second GAP), `terminalReady()`/`handleTerminal()` (the deferred handoff + wrap composition), `connection.Close(CloseType)` (the existing close request — `closeReq`/`closeType`); `internal/filter/network/readconn.go` — `readChainConn.Read` (the third GAP); `internal/filter/network/callbacks.go` — the `Connection` interface (`Close(ct CloseType)`; `FlushWrite`/`NoFlush`), `ReadFilterCallbacks`/`WriteFilterCallbacks`; `internal/filter/network/types.go` — `FactoryCtx` (the drain-handle threading point); `internal/filter/tcpproxy/filter.go` — the two pump goroutines + `wg.Wait()` (the §3.5 close-direction recording site); `internal/listener/manager.go` — `rt.dm *drain.Manager` (the drain handle, already threaded to the listener, NOT yet to network filters), the chain-construction site, the read-loop close path; `internal/drain/manager.go` — `IsDraining() bool` (the lock-free drain decision); `internal/accesslog/writer.go` — `AsyncFileSink.run()` (`s.f.Write(Default(r))` at the hard-wired formatter call), `NewAsyncFileSink`, the `Sink`/`Record` types; `internal/accesslog/format.go` — `Default(*Record) []byte`; `internal/filter/http/fault/fault.go` — `rollPercent`/`percentageToFloat`/`time.AfterFunc`/`markedActive` (the fault-delay eval + at-most-once-cancel precedent).
- **mongoproxy package anchors** (the §3.2/§3.3/§3.4/§3.5 consumption sites): `config.go` — `delayConfigured`/`fixedDelay`/`delayPercentNum`/`delayPercentDenom`/`accessLog` (parsed at 29.1, consumed here); `codec.go` — the per-request-message decode loop (the fault-eval point), `takeQuery` (the correlated-reply path — the `cx_drain_close` list-empty check), `onDestroy` (the residual-drain — the `cx_destroy_*` direction read), the `mu`/`queries`/`start`; `filter.go` — `OnData`/`OnWrite`/`OnDestroy` + the stored `cb`/`wcb` (the fault timer + the close + the access-log sink); `stats.go` — the eager roster (`delays_injected`/`cx_drain_close`/`cx_destroy_*` exist-at-zero).

### 9.2 Inherited empirical pins (constrain §3; no re-probe)

- Parent §11.6 fault-delay semantics (per-decoded-request-message eval at the decode-callback entry; re-entrancy guard; `delayDuration` FractionalPercent gate + the proto `fixed_delay`; `delays_injected` at arm; StopIteration-while-pending; timer cancel on close) → §3.2.
- Parent §11.7 continueReading re-dispatch (resume at `std::next(filter->entry())`; fresh socket reads re-dispatch from filter 0; socket reads continue during the halt) → §3.1.
- Parent §11.8 access-log format (`{"time","message","upstream_host"}`; request `full=true`/reply `full=false`; the `message.toString` per-opcode shapes; timing-bearing → unit-test + boundary) → §3.3.
- Parent §11.10 close direction (LocalClose/RemoteClose keying; the as-built gap) + drain (reply-completion + zero-ms timer + FlushWrite; the active-query-list-empty condition) → §3.4/§3.5.
- AMEND-B13 the seam reframing (the three real extensions; upstream has no persistent filter-manager halt) → §3.1.
- AMEND-B9 the FaultDelay PGV (oneof required; `fixed_delay` gt 0s) — already validated at 29.1; D-P5 disposes the fixture-vs-unit posture (§10.1).

---

## 10. SPEC-time D-questions

### 10.1 Parent D-questions RESOLVED at this SPEC

- **D-P5 (delay-PGV fixture arms + header_delay posture) — RESOLVED: UNIT-TEST-ONLY; header_delay = parse-accept-no-delay** (§1.2). The AMEND-B9 reject arms (`delay: {}`; `fixed_delay: 0s`) were unit-tested at 29.1 and stay unit-test-only (the `0050-mongo-boot-reject` fixture carries the `stat_prefix` arm only — the zookeeper D-P4 precedent; no new boot-reject dir). A `header_delay`-configured mongo filter parse-accepts but injects no delay (upstream's `FixedDelayProvider` path is never taken for `header_delay`) — a unit-test arm asserts parse-accept + zero `delays_injected`.
- **D-P7 (access-log formatter seam) — RESOLVED: a pluggable formatter on `AsyncFileSink`** (default `Default` → HCM byte-identical; the mongo JSON formatter + carrier; the exact carrier shape is D-S29.3-4) (§3.3).
- **D-P12 (halt-state synchronization design) — RESOLVED at the SEMANTIC level** (§3.1): a per-`chainRuntime` halt mutex guarding EXACTLY the halt/resume state, behind an atomic fast-path so never-halting chains are byte-identical; the post-handoff withhold-until-resume in `readChainConn.Read`; the async-active `ContinueReading` third context. The exact PRIMITIVE (Mutex+Cond vs a release channel; pump block vs withhold-and-retry) is D-S29.3-1, the `-race` + back-compat gates settle it.

### 10.2 29.3-additive D-questions for PLAN / IMPL resolution

- **D-S29.3-1 (the halt-seam synchronization primitive).** Mutex+`sync.Cond` (block the pump's `Read` until resume signals) vs a per-connection release channel (the pump selects on it) vs withhold-and-retry; the exact lock scope (the ADR-0223 minimal critical section). **Resolution at:** IMPL (the `-race` test + the back-compat byte-identity gate decide). Anticipated: a halt mutex + a `sync.Cond` (block-the-pump), gated behind an atomic fast-path.
- **D-S29.3-2 (async-resume terminal handoff).** How an async resume that advances `resumeIdx` past all filters triggers the deferred terminal handoff from the timer goroutine (the read loop normally performs it on `TerminalReady()`). **Resolution at:** IMPL. Anticipated: the resume re-enters the same handoff path the listener loop drives (or schedules it), under the halt mutex; the common `0052` first-message delay fires pre-handoff so the handoff is the read loop's after resume.
- **D-S29.3-3 (the callbacks accessor surface — drain + close-direction).** A `Draining() bool` + `CloseDirection()` on `ReadFilterCallbacks` vs on `Connection`; threading the `*drain.Manager` handle vs a narrow bool through `network.FactoryCtx`. **Resolution at:** IMPL. Anticipated: minimal accessors (NOT a manager leak), one folded callbacks-extension ripple.
- **D-S29.3-4 (the access-log mongo carrier).** Extend `Record` with an opaque mongo field vs a typed-payload formatter (`Formatter` over `any`/a mongo record) vs a mongo-owned sibling sink reusing the async-writer machinery. **Resolution at:** IMPL (the HCM byte-identity gate constrains it). Anticipated: a minimal mongo record type + a mongo formatter, sink generalized to carry it, HTTP path unchanged.
- **D-S29.3-5 (the fault-eval callback set — transcription check).** The parent §11.6 already pins the upstream six-callback set (`decodeQuery`/`decodeInsert`/`decodeGetMore`/`decodeKillCursors`/`decodeCommand`/`decodeCommandReply` — NOT `decodeReply`; §3.2 inherits it verbatim); the IMPL transcribes it against `proxy.cc` v1.37.2 (`reference_wire_format_both_sides_see_same_bytes`) rather than re-deriving. **Resolution at:** IMPL. Anticipated: the six-callback pin stands; `0052` exercises OP_QUERY (the load-bearing arm).
- **D-S29.3-6 (the fault filter↔decoder split + the re-entrancy/cancel guard).** Where the roll+duration decision lives (decoder) vs the timer+resume (filter); the re-entrancy guard's home + the at-most-once timer-cancel CAS (the phase-09 `markedActive` precedent). **Resolution at:** IMPL. Anticipated: decoder decides + signals `takePendingDelay`; the filter owns the `time.AfterFunc` + `cb`; an `atomic.Bool` guard + a CAS-guarded cancel.
- **D-S29.3-7 (the omitted zero-ms drain timer).** Confirm envoy-go's deferred `Connection.Close` subsumes upstream's zero-ms drain timer with no observable `0052`-arm difference. **Resolution at:** IMPL. Anticipated: yes (the close is already deferred to post-pass).
- **D-S29.3-8 (the `0052` drain-trigger mechanism).** How the fixture drives the drain decision cross-side (the phase-08.2 admin `/drain` signal vs a listener-drain trigger) so `cx_drain_close` is differentially asserted; else PRESENCE + a unit-test value proof. **Resolution at:** IMPL. Anticipated: the fixture drives the drain; the phase-08.2 path is the vehicle.

---

## 11. RATIFIED-PENDING items (cross-reference parent §13 + the 29.1/29.2 SPEC, scoped to 29.3)

- **R1 (seam back-compat / non-perturbation).** Every existing fixture (`0000`..`0051`) + every never-halting filter (zookeeper, mongo-without-delay, every HTTP path via the formatter seam) stays byte-exact green at the 29.3 six-gate — the seam's regression gate. The `0052` no-delay arm (§6.1 arm 2) is the live equivalence proof; the full 53-dir suite + h2spec 53/53 + proxy-wasm 10/10 are the back-compat gate.
- **R3 (passthrough invariant + the drain-close exception).** mongoproxy NEVER mutates/drains the chain buffer (read OR write); it returns StopIteration ONLY while a fault-delay timer pends (§3.2); it closes the connection ONLY on the `cx_drain_close` reply-completion drain (§3.4 — upstream parity, NOT a passthrough violation). Decode errors → sniffing off + passthrough continues. Ratified by `0052` arm 2 (no-delay byte-identity) + the unit tests.
- **R4 (StatsAsserter liveness).** Every `0052` stat assertion proven live via a recorded deliberate-break with `-count=1` (§6.1 arm 6 — incl. the `delays_injected`/`cx_drain_close`/`cx_destroy_*` breaks).
- **R8 (deterministic fault arms).** The `0052` arms are 100%-probability + fixed-delay; `delays_injected` value parity is asserted; the delay DURATION is NEVER compared (BOOTSTRAP §7.2 timing discipline).
- **R-HALT (NEW — the seam).** The async halt/resume seam is correct + minimal: the `-race -count=N` concurrency test (§7) is GREEN with the halt mutex and reports a race WITHOUT it; the held bytes release exactly once on resume; never-halting chains take the byte-identical fast path. Plus the live `0052` pre-handoff + post-handoff delay arms (§6.1 arm 1) exercise both halt paths under real traffic.
- **R-DRAIN (NEW).** `cx_drain_close` fires on reply-completion when the drain signal is active + the active-query list is empty; the connection closes FlushWrite (the reply flushed first). Ratified by `0052` arm 3 (or PRESENCE + unit value per D-S29.3-8).
- **R-CLOSEDIR (NEW — D-P4 CLOSED).** `cx_destroy_local/remote_with_active_rq` hold VALUE parity keyed on close direction (the 29.2 presence-only boundary resolved). Ratified by `0052` arm 4 (local/remote/all-answered) + the close-direction unit tests.
- **R6 (counts).** IMPL Task 1 re-pins fixtures 53→54, fuzzers 39 (unchanged), stats 360 (unchanged), BackendKind tail 30 (unchanged), DECISIONS tail ADR-0226 (next-free ADR-0227) against the live IMPL-session tip (§9.1 recipes).
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` mongo fault/drain/close-direction lines match the reference's tag-extracted shape. Ratified intrinsically by the `0052` label-aware both-sides scrape (§5.2).

---

## 12. Per-task structure (~12–16 tasks; the SPEC-anticipated spine)

The 29.3 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine, ordered for green-compiling dependency (the 28.2/29.2 ordering logic: anchors → framework seam → fault delay → access log → drain → close-direction → fixture → docs → six-gate). 29.3's distinctive ordering pin: **the framework seam (§3.1) lands FIRST + GREEN (with its `-race` test) before any mongo consumer wires into it** — the framework is independently testable (a synthetic halting test filter), the mongo consumer rides a proven seam.

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **53** (tail `0051`) + fuzzers **39** + stat surface **360** + BackendKind tail **30** + DECISIONS tail **ADR-0226** (next-free **ADR-0227**) + the §9.1 as-built anchors, against the live IMPL-session tip | §9.1 / R6 |
| 2 | The halt-state synchronization on `chainRuntime` (the halt mutex + the atomic fast-path + the post-handoff halt flag) + a synthetic halting test filter; the never-halting byte-identity unit proof | §3.1 / R1 |
| 3 | `ContinueReading` async-active third context (advance-past-halting-filter + dispatch + re-eval terminal readiness) + the pre-handoff async-resume unit test | §3.1 |
| 4 | `replayRead` + `readChainConn.Read` post-handoff withhold-until-resume + the post-handoff halt unit test + the `-race -count=N` cross-goroutine seam test (R-HALT; the deliberate-break with `-count=1`) | §3.1 / §7 / R-HALT |
| 5 | The fault-delay roll+duration decision in the decoder (per-request-message; re-entrancy guard; `rollPercent` deterministic at 100%) + `takePendingDelay` + unit tests (100% arms; the guard) | §3.2 |
| 6 | The fault-delay timer in the filter (`time.AfterFunc` + `onDelayTimer` + `ContinueReading`; `delays_injected` at arm; StopIteration-while-pending in `OnData`/replay; timer cancel on `OnDestroy`) + the timer↔destroy `-race` test (D-S29.3-6) | §3.2 |
| 7 | The `internal/accesslog` pluggable formatter seam (default `Default` → HCM byte-identical) + the mongo record carrier + the mongo JSON formatter + the per-opcode `message.toString` goldens (D-P7 / D-S29.3-4) | §3.3 |
| 8 | The mongo access-log sink construction (gated on `cfg.accessLog`) + per-message record emission both directions + unit tests (the timing-field shape; the gated-off no-emit) | §3.3 |
| 9 | The drain-state accessor (`Draining()` through `FactoryCtx`/callbacks; D-S29.3-3) + `cx_drain_close` on the reply-completion list-empty path + `Connection().Close(FlushWrite)` + unit tests | §3.4 / R-DRAIN |
| 10 | The close-direction recording (`tcp_proxy` pump-EOF-first → `chainRuntime.closeDirection`) + the `CloseDirection()` accessor + the `OnDestroy` direction-keyed `cx_destroy_*` increment (D-P4 CLOSED) + unit tests (local/remote/all-answered) | §3.5 / R-CLOSEDIR |
| 11 | `0052-mongo-fault-delay` driver + cross-side GREEN all arms (the pre/post-handoff delay arms; the no-delay non-perturbation arm; `cx_drain_close`; the `cx_destroy_*` VALUE-parity arms; the R4 break) + README | §6.1 |
| 12 | The completion bundle: ADR-0226 §Decision/§Consequences body in place + the BEHAVIOR_CONTRACT 29.3 bundle (§8 — incl. the framework async-halt/resume subsection + the access-log + runtime-key boundaries) + STATE.md + ROADMAP sub-row 29.3 `in-progress → done` + **the parent-row-29 ROLLUP** (parent `in-progress → done` ATOMICALLY) + next-prompt.txt + the six-gate (full 54-dir suite + h2spec + proxy-wasm) | §8 / §13 / R-ROLLUP |

### 12.1 ADR-0045 split-gate — SPEC-level check

Production-LoC estimate against the §3 surface (production code; fixture drivers + unit tests EXCLUDED — the 26.x/28.2/29.2 accounting basis):

| Deliverable | Production LoC |
|---|---|
| The async halt/resume seam (`chain.go`/`readconn.go` — sync + async-active resume + post-handoff hold-and-release + the close-direction recording) | ~150–280 |
| The drain + close-direction callbacks accessors (`callbacks.go`/`types.go`/`manager.go`/`tcpproxy/filter.go`) | ~40–80 |
| Fault-delay injection (`codec.go` roll+duration + `filter.go` timer/resume/cancel) | ~90–150 |
| The access-log formatter seam (`internal/accesslog/`) + the mongo formatter + sink construction | ~110–190 |
| `cx_drain_close` (the list-empty check + the close) + the `cx_destroy_*` direction-keyed increment | ~30–60 |
| **Total (production basis)** | **~420–760** |

**Verdict: fits as ONE sub-phase** (well under the ~1500 gate; ~12–16 tasks under the ~25-task gate) — squarely in the parent §11.11/§15 estimate (~470–780 production LoC). The `0052` driver (~450–600 LoC; the `0051` precedent) is excluded per the accounting precedent. **The 29.3 PLAN remains the FINAL gate-check** (parent §3.0); no pre-authorized split axis is anticipated (the 28.2/29.2 single-sub-phase precedent).

---

## 13. Test surface + 29.3 IMPL acceptance checklist

### 13.1 Test surface (per parent §14, scoped to 29.3)

- **Layer A — framework unit tests** (`internal/filter/network/`): the halt-state synchronization (a synthetic halting test filter; pre-handoff async resume; post-handoff withhold-until-resume; the terminal-handoff-after-resume; never-halting byte-identity); the close-direction recording (local/remote/unset); the drain-state accessor. **Layer A — mongoproxy unit tests**: fault delay (100%-probability deterministic arm; the re-entrancy guard suppresses re-eval; StopIteration-while-pending; `delays_injected` at arm; timer cancel on destroy; the header_delay parse-accept-no-delay arm — D-P5); the access-log JSON formatter (per-opcode `message.toString` goldens; the timing-field shape; the gated-off no-emit); `cx_drain_close` (list-empty + drain-active → increment + FlushWrite; not-draining → no close; mid-query → no close); `cx_destroy_*` direction-keyed (local/remote/all-answered). **Layer A — accesslog unit tests**: the pluggable formatter seam (the HTTP `Default` bytes UNCHANGED; a mongo formatter routed correctly).
- **Layer E — race**: the `-race -count=N` async-`ContinueReading` cross-goroutine seam test (mutex necessary + sufficient — R-HALT) + the fault-timer↔OnDestroy at-most-once race + `go test -race -short` across `internal/filter/network/...`.
- **Layer D — differential**: `0052` (cross-side; the pre/post-handoff delay arms; the no-delay non-perturbation arm; `cx_drain_close`; the `cx_destroy_*` VALUE-parity arms; 6 arms) + the FULL 53-dir back-compat suite (R1) → 54/54 green.
- **Layer B — access-log goldens**: the formatter unit goldens (NO fixture — AMEND-B10).
- Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

### 13.2 Six-gate checklist (per the 28.2/29.2 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (54 dirs incl. the 53-dir seam-back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — the accesslog formatter seam keeps the HTTP path byte-identical; 29.3 touches no HTTP filter). All outputs quoted into PROGRESS.md (run honestly — `reference_differential_break_protocol_count1` for the R4 + R-HALT breaks).

### 13.3 29.3 IMPL acceptance checklist

1. The async halt/resume seam lands per §3.1 (active-async `ContinueReading`; cross-goroutine safety behind the atomic fast-path; post-handoff withhold-until-resume); never-halting chains byte-identical (R1 — the full 53-dir suite green); the seam `-race` test passes (R-HALT).
2. Fault-delay injection lands per §3.2 (per-request-message eval; re-entrancy guard; `delays_injected` at arm; StopIteration-while-pending; deterministic 100% arms; timer cancel on destroy).
3. The mongo access log lands per §3.3 (the JSON formatter seam; HCM byte-identical; unit goldens + the coverage boundary; NO fixture — AMEND-B10 / D-P7).
4. `cx_drain_close` lands per §3.4 (reply-completion + drain-active → FlushWrite — R-DRAIN); `cx_destroy_*` close-direction VALUE parity lands per §3.5 (D-P4 CLOSED — R-CLOSEDIR).
5. Fixture `0052` green (the pre/post-handoff delay arms; the no-delay arm; `cx_drain_close`; the `cx_destroy_*` value arms; the R4 break); counts: fixtures 53→54, fuzzers 39 (unchanged), stats 360 (unchanged), BackendKind 30 (unchanged) (R6).
6. ADR-0226 §Decision/§Consequences body lands in place (DECISIONS.md tail STAYS ADR-0226; no new number); the BEHAVIOR_CONTRACT 29.3 bundle lands (§8 — incl. the framework async-halt/resume subsection).
7. Six gates green (§13.2); STATE.md advanced; ROADMAP sub-row 29.3 `in-progress → done` **AND the parent row 29 `in-progress → done` ATOMICALLY** (the ROLLUP — the 18/19/22/24/25/26/28 precedent); next-prompt.txt rewritten for the NEXT-phase cold-start (the §9 family's remaining candidates, or the next ROADMAP row).

---

## 14. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 29.3 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.x/29.1/29.2 precedent); **parent row 29 STAYS `in-progress`** (the ROLLUP is the 29.3 IMPL's, NOT this SPEC's). STATE.md advances to lifecycle-state 2-for-29.3-PLAN with `next-skill = superpowers:writing-plans` scoped to the **29.3 PLAN** (`docs/envoy-go/phases/29.3-network-filter-mongo-fault-delay-and-access-log/PLAN.md`). The SPEC is squash-merged to master + pushed; next-prompt.txt is rewritten for the 29.3-PLAN cold-start. Per `feedback_execution_style` the 29.3 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin` the established worktree/push discipline applies. No ADR number is consumed at this SPEC (ADR-0226 §Context already exists; its §Decision/§Consequences body lands at the 29.3 IMPL — DECISIONS tail STAYS ADR-0226, next-free ADR-0227).

---

## Appendix A — Cross-references

| 29.3 SPEC § | Master / precedent § | Relationship |
|---|---|---|
| §1 Purpose | parent §3.2 (29.3) + ADR-0226 §Context | executes |
| §1.1 AMENDs + 29.1/29.2 outputs | parent §1.1 (B8/B10/B12/B13) + 29.1/29.2 parse+roster | inherits + consumes |
| §1.2 Additive pins | — | NEW (the seam design; the drain + close-direction accessors; the formatter seam; D-P5) |
| §2 Non-purposes | parent §2 + §3.2 | refines (29.3-scoped) |
| §3.1 The async halt/resume seam | parent §4.1 + AMEND-B13 + §11.7 + ADR-0223 | EXECUTES (the three extensions; D-P12 resolved) |
| §3.2 Fault delay | parent §11.6 + the phase-09 fault filter | executes (per-message eval; deterministic 100%) |
| §3.3 Access log | parent §11.8 / AMEND-B10 + §06.2 accesslog | RESOLVES (D-P7 formatter seam; unit-test + boundary) |
| §3.4 cx_drain_close | parent §11.10 + the phase-08.2 drain | executes (reply-completion + FlushWrite; the drain accessor) |
| §3.5 close-direction / D-P4 | parent §11.10 / AMEND-B12 + ADR-0219 + the 29.2 boundary | CLOSES (value parity; the close-direction accessor) |
| §4 Framework touchpoints | 29.1/29.2 §4 zero-touch + ADR-0219 | the SURGERY sub-phase (the one consolidated ripple) |
| §5 Stat surface | parent §7 | refines (+0 creation; increment-wiring) |
| §6 Fixtures + goldens | parent §8.4 + 29.2 §6 (responder) | refines (0052 + the access-log goldens) |
| §7 Fuzzer + seam race | parent §11.12 + 28.2/29.2 R9 | no count change (+ the seam `-race` test) |
| §8 Behavior contract | parent §9 (29.3 bundle) | refines (+ the framework subsection + the rollup) |
| §9 Empirical pins | parent §11.6/§11.7/§11.8/§11.10 | inherits; re-pins D-S29.3-0 (no re-probe) |
| §10 D-questions | parent §12 (D-P5/P7/P12) | resolves; adds D-S29.3-1..8 |
| §11 RATIFIED-PENDING | parent §13 + 28.2/29.2 (R9) | scoped to 29.3 (+ R-HALT/R-DRAIN/R-CLOSEDIR) |
| §12 Tasks + split-gate | parent §15 (29.3 row) + 28.2/29.2 §12 | NEW (task spine; seam-first ordering); gate re-check |

## Appendix B — Phase 29.3 ADR landing summary

- **ADR-0226** (the async halt/resume seam + fault-delay + the mongo access log + `cx_drain_close`) — §Context drafted at the phase-29 parent SPEC (`DECISIONS.md:14521`); §Decision + §Consequences bodies land at 29.3 IMPL Task 12 per ADR-0044. This SPEC's §3 + §5 + §6 are the body's blueprint: the three-extension async halt/resume seam (§3.1), fault-delay injection (§3.2), the access-log formatter seam (§3.3), `cx_drain_close` (§3.4), the close-direction seam D-P4 CLOSED (§3.5), the `0052` fixture (§6). No §Context AMEND is needed at THIS SPEC commit (the 29.2 D-P4 re-scope already amended ADR-0225; the 29.3 charter stands as drafted).
- DECISIONS.md tail STAYS **ADR-0226** at 29.3 phase-done (no new ADR number consumed); next-free **ADR-0227**. The ADR-0226 body + the parent-row-29 ROLLUP land at the 29.3 IMPL; the FOURTH §9 Network-filters-family row CLOSES (3 candidates remain — `redis`/`kafka_broker`/`thrift`).
