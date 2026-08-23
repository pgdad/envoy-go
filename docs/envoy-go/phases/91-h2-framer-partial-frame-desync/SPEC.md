# Phase 91 — `h2-framer-partial-frame-desync` — SPEC

**Stage:** SPEC (lifecycle-state 1 -> 2)
**Base master:** `870fc90b`
**Branch:** `phase-91-spec`
**Date:** 2026-08-23
**Row:** 91, stays `in-progress` at `ROADMAP.md:153`; `want` stays **123** (a SPEC changes neither — verified by EMPTY DIFF)
**ADR:** **ADR-0313** §Context drafted at this stage; the strict `^> **STATUS: PROPOSED` guard re-armed **0 -> 1**, verified BY LINE AND ADR

---

## 1. The charter, and the bound this stage puts on it

**Charter (unchanged): remove the read-deadline polling from the HTTP/2 frame reader so a frame read is
NEVER abandoned part-way.**

The bound this SPEC adds: the row fixes the **read** path of `internal/filter/hcm/h2/framer.go` at **all
three** production call sites — `conn.go:259` (`(*ServerConn).Run`), `conn.go:304`
(`(*ServerConn).processFrameAndMaybeDrain`) and `client.go:307` (`(*ClientConn).readLoop`) — and does not
touch the encode/response direction, the H/3 leg, or the flow-control accounting, which §2.5 of the
BRAINSTORM exonerated by execution.

---

## 2. Method, and what this stage refuted

Every claim below was re-derived at `870fc90b` by execution. **This stage refuted SIX inherited claims**,
three of them load-bearing for the fix shape. The BRAINSTORM's *conclusion* survives in every case; three
of its *reasons* do not, and a reason that survives into an ADR acquires authority it has not earned
(`reference_brainstorm_adjective_acquires_adr_authority`).

| # | Inherited claim | Verdict |
|---|---|---|
| R1 | "A deep copy is impossible from outside `x/net`" | **REFUTED** (§4.1) — every aliasing type has an exported accessor |
| R2 | The retaining-type inventory is 5 types | **CORRECTED to 7** (§4.2) — `DataFrame.data` and `UnknownFrame.p` alias too |
| R3 | `SetReuseFrames` is a "red herring" | **NARROWED** (§4.3) — irrelevant to payload aliasing, but *protective* against a second hazard |
| R4 | `closeReader` needs FOUR wiring sites | **REFUTED — SEVEN**, and the fourth as described does not exist (§5.3) |
| R5 | "No reference-side counterpart" grounds the differential departure | **REFUTED** (§7) — the observable IS reference-comparable; the TRIGGER is not harness-controllable |
| R6 | `ROADMAP.md` cites `esalaine` five times | **REFUTED** — 4 LINES / 7 OCCURRENCES; neither figure is 5 (§11) |

⚠️ **A seventh correction is method, not content:** a symbol assertion's **receiver name is part of the
anchor**. `func (ss *serverStream) recvData` reads **0**; the receiver is `s`. This bit the controller
mid-stage, and independently bit an agent, and is the same shape the BRAINSTORM recorded at its §9.4
item 18.

⚠️ **AN EIGHTH FINDING IS THE CONTROLLER REFUTING ITS OWN SOURCE, AND IT IS RECORDED BECAUSE THE NEAR-MISS
IS THE LESSON.** An agent reported that `git grep -c` **under-matches a `find`-based count by one** on
`.ReadFrame(` (69 vs 70), and the controller had already written that into this SPEC before verifying it.
**It DOES NOT REPRODUCE.** Re-derived here with the pattern escaped (`'\.ReadFrame('`) and both arms
summed: `git grep -c -- 'internal/filter/hcm/h2/*.go'` => **71**, `find | xargs /usr/bin/grep -c` =>
**71**, and a per-file `diff` of the two listings is **EMPTY**. The two tools **AGREE**. The agent's
figures were non-comment-filtered subsets, not raw counts, and the "under-match" was an artifact of
comparing differently-filtered arms rather than a tool defect. ⇒ **`feedback_brief_citations_not_evidence`
applies to a subagent's INCIDENTAL figures, not merely its headline claims** — this is the fourth
consecutive stage at which that shape has fired, and it was caught here only because the number was
re-derived rather than restated.

---

## 3. D-91-SHAPE — the fix shape, and why the alternatives lose

**DECISION: a per-connection reader goroutine calling `ReadFrame` with NO deadline, handing frames to the
consumer over an UNBUFFERED channel, gated by a RELEASE HANDSHAKE performed at the START of each consumer
read call.**

- `readFrameCtx(ctx)` becomes: release the previous frame, then `select` on `ctx.Done()` and the frame
  channel. Context cancellation is observed **immediately** rather than within a 50 ms poll slice, so the
  fix strictly improves the property the deadline existed to provide.
- `tryReadFrame()` becomes: release, then `select` on the frame channel with a **short bounded wait**.
  ⚠️ **It MUST NOT become a bare `default:`** — see §6.

**Why the alternatives lose:**

1. **Widening the deadlines is NOT a fix.** It only narrows the window in which a deadline can land
   mid-frame. It is retained solely as the BRAINSTORM's discriminating counterfactual (`2 2`, 6 FAIL/12 ->
   0 FAIL/12), and this SPEC forbids the PLAN from shipping it.
2. **A resumable buffering wrapper loses** because `http2.Framer` holds internal parse state
   (`fr.lastFrame`, `fr.errDetail`, `fr.headerBuf`, the `frameCache`) that is unexported and not
   restartable from outside the package. There is no seam at which a partially-consumed frame can be
   resumed.
3. **A BUFFERED channel loses** — but **NOT for the reason the record gives**. See §4.

---

## 4. D-91-ALIAS — the aliasing constraint SETTLED at the pinned source

Pinned dep verified by execution: `go.mod:18  golang.org/x/net v0.34.0`; `go list -m` agrees. All quotes
are from `/home/esa/go/pkg/mod/golang.org/x/net@v0.34.0/http2/frame.go`.

### 4.1 ⚠️ R1 REFUTED — a deep copy IS possible from outside `x/net`

The BRAINSTORM states that a buffered channel is *"UNSAFE AND UNFIXABLE FROM THIS PACKAGE"* because every
retaining field is unexported and *"a deep copy is impossible from outside `x/net`"*. **The premise is
true and the conclusion drawn from it is false.** Unexported *fields* do not imply an unreachable *value*:
every aliasing type exposes an **exported accessor** returning the aliased slice —
`(*DataFrame).Data()`, `(*GoAwayFrame).DebugData()`, `(*HeadersFrame).HeaderBlockFragment()`,
`(*ContinuationFrame).HeaderBlockFragment()`, `(*PushPromiseFrame).HeaderBlockFragment()`,
`(*UnknownFrame).Payload()`, and `(*SettingsFrame).ForeachSetting()` — so
`append([]byte(nil), f.DebugData()...)` copies the payload in one line, from outside the package. This was
proven by an executed probe in which the copy survived a clobber that destroyed the aliased slice.

**The claim, correctly narrowed — and this is what the ADR must say:**

> You can deep-copy a frame's **bytes**; you cannot reconstruct a valid `http2.Frame` **value** holding
> those bytes. `frame.go` exports exactly **two** producing functions — `ReadFrameHeader` (`:230`) and
> `NewFramer` (`:430`) — and zero setters.

### 4.2 ⚠️ R2 — the retaining-type inventory is SEVEN, not five

`DataFrame.data` (`frame.go:584`, assigned `:630`) and `UnknownFrame.p` (`:931`, assigned `:945`) alias the
shared read buffer and the record omits both. `DataFrame` matters most: it has **5 production call sites**.
`UnknownFrame` matters structurally — `typeFrameParser` routes **every unrecognised frame type** to
`parseUnknownFrame`, so a peer-chosen type always lands in an aliasing frame.

**Value-only (safe to retain):** `PingFrame` (`Data [8]byte`, an EXPORTED array populated by `copy`),
`WindowUpdateFrame`, `RSTStreamFrame`, `PriorityFrame`. **Aliasing: 7 of 11 parser entries.**

### 4.3 ⚠️ THE REAL HAZARD IS AN UNCATCHABLE PANIC, NOT CORRUPTION

```go
func (h *FrameHeader) checkValid() {
	if !h.valid {
		panic("Frame accessor called on non-owned Frame")
	}
}
```

`checkValid()` is called by **8** accessors (`frame.go:596, 762, 817, 898, 940, 1007, 1255, 1289`). Our
production code calls three of them at **14 live sites** — `.Data()` (client.go:465/473/475,
conn.go:595/597), `.ForeachSetting(` (client.go:393, conn.go:675/687, settings.go:138/148),
`.HeaderBlockFragment()` (client.go:427, conn.go:81/127/405) — and there is **no `recover()` anywhere** in
`internal/filter/hcm/h2` or `internal/filter/hcm`. ⇒ **every one is a PROCESS CRASH.** Two
`SettingsFrame` accessors (`Setting`, `NumSettings`) skip the check and read corrupted bytes **silently**
instead. `SetReuseFrames` is confirmed **never called** across all 1021 `.go` files in the tree; it is
irrelevant to payload aliasing but *protective* against `*DataFrame` struct aliasing, so not calling it is
strictly better — "red herring" overstates it (R3).

### 4.4 ⚠️ THE INVALIDATION IS UNCONDITIONAL, AT ENTRY, AND FIRES EVEN ON A FAILED READ

```go
func (fr *Framer) ReadFrame() (Frame, error) {
	fr.errDetail = nil
	if fr.lastFrame != nil {
		fr.lastFrame.invalidate()
	}
	fh, err := readFrameHeader(fr.headerBuf[:], fr.r)
```

`frame.go:496-501`. The previous frame is dead the instant the next `ReadFrame` is **entered** — before any
I/O, and across all five of its error returns. Byte corruption is a **separate**, size- and
offset-dependent event that may not happen at all, which makes buffering a **latent, traffic-shape-
dependent** bug rather than a deterministic one.

### 4.5 ⇒ THE DECISION, ON CORRECTED GROUNDS

**An unbuffered channel + a release handshake performed at the START of each read call.** The justification
is **parity, not impossibility**:

> Today the consumer calls `ReadFrame()` directly, and `frame.go:498-500` invalidates frame N at the
> **entry** of the call that fetches frame N+1. Under the new design the consumer signals release at the
> **start** of the call that fetches frame N+1 — the same instant. ⇒ release-at-start-of-next-read is
> **exact parity** with invalidate-at-entry, and the ownership window is byte-for-byte the one
> `conn.go:291-295` already documents.

⚠️ **AND THE PARITY IS SAFE BECAUSE EVERY CONSUMER ALREADY COPIES** — established at this stage, and
neither the BRAINSTORM nor any agent had it: `startHeaderBlock` copies the fragment explicitly (its comment
names the reuse hazard verbatim), `(*serverStream).recvData` writes into a `bytes.Buffer` (`stream.go:190`),
and the client writes `fr.Data()` into `cs.respBody bytes.Buffer` (`client.go:475`). **No consumer retains
an aliased slice past dispatch.** ⇒ the handshake is sufficient; a per-frame consumed-ack beyond it is
NOT required, and the PLAN must not add one.

⚠️ **THE CROSS-GOROUTINE ALIASING QUESTION IS CLOSED TOO:** the three `pendingDispatch` closures
(`conn.go:467-470`, `:556-560`, `:649-652`) capture only `ctx`, a `*serverStream`, `s` and `streamID` —
**no frame**. So the `go fn()` at `conn.go:358` never touches framer-owned memory, even though those
goroutines outlive `Run` (§5.4).

⚠️ **THIS NARROWS §4.3, AND THE NARROWING IS RECORDED RATHER THAN LEFT BROAD.** Because every consumer
copies at the boundary, an invalidated frame **can never yield wrong data** in this codebase. The hazard is
**PURELY the panic** — `checkValid()` tests the header's `valid` flag *regardless of whether the result is
then copied*, so `f.Data()` at `conn.go:595` still crashes the process. A *"corruption or crash"* framing
would be too broad; the accurate claim is **crash only**.

**The buffered-channel alternative is priced rather than dismissed:** it is *constructible* via the
exported accessors, at one `[]byte` alloc + copy of ≤16384 B (`settings.go:23`, `framer.go:22`) on
essentially every frame — but the dominant cost is that all 14 accessor call sites and every dispatch
signature must move off `http2.Frame` onto a local owned-frame type, because the copy cannot be re-minted
as an `http2.Frame` (§4.1). **A wide refactor of the whole package for a bug that does not need it.**

---

## 5. D-91-SITES — three call sites, lazy start, and SEVEN wiring sites

### 5.1 The three production call sites — CONFIRMED at this tip

| file:line | call | enclosing function |
|---|---|---|
| `conn.go:259` | `s.fr.readFrameCtx(s.ctx)` | `(*ServerConn).Run` (`conn.go:206`) |
| `conn.go:304` | `s.fr.tryReadFrame()` | `(*ServerConn).processFrameAndMaybeDrain` (`conn.go:296`) |
| `client.go:307` | `cc.fr.readFrameCtx(cc.ctx)` | `(*ClientConn).readLoop` (`client.go:305`) |

⚠️ **`tryReadFrame` has ZERO direct tests**, and the SPEC rewrites it. `readFrameCtx` has exactly one
(`framer_test.go:330`).

### 5.2 D-91-LAZY — the reader MUST start lazily. FOUR blockers, three of them unnamed by the record

1. `settings.go:121` — `readClientSettings` calls the **embedded** `fr.ReadFrame()` directly, on **both**
   sides (server `conn.go:225`, client `client.go:285`). *(named by the record)*
2. ⚠️ `conn.go:212` — `readClientPreface(s.conn)` reads the socket **directly**, before any framer read.
3. ⚠️ `conn.go:279` / `conn.go:315` — `io.Copy(io.Discard, s.conn)` reads the socket directly.
4. ⚠️ `contIdleServerConn` (`continuation_test.go:644`) builds a `ServerConn` its own comment says is
   **NEVER Run**; plus **11** test call sites hold a `*framer` and call `ReadFrame` directly, and
   `framer_writeheaderblock_test.go:235` constructs a `framer` literal with a **nil `conn`** — any new
   method dereferencing `f.conn` nil-panics there.

⇒ starting the goroutine in `newFramer` would race all of them. **An explicit `startReader()` seam is
required**, and the record's cost floor contains no such seam.

### 5.3 ⚠️ R4 REFUTED — `closeReader` needs SEVEN wiring sites, not four

| # | file:line | function | today | needs | vs. the record |
|---|---|---|---|---|---|
| 1 | `conn.go:207` | `(*ServerConn).Run` | `defer func(){ _ = s.conn.Close() }()`, first stmt | `defer s.closeReader()` placed **textually AFTER** 207 | covered — **ordering as stated is WRONG** |
| 2 | `conn.go:276-278` | `(*ServerConn).Run` | GOAWAY -> 500 ms deadline -> drain | synchronous **JOIN** before the drain | covered |
| 3 | `conn.go:311-314` | `processFrameAndMaybeDrain` | same drain | synchronous **JOIN** before the drain | covered |
| 4 | `client.go:305` | `(*ClientConn).readLoop` | **ZERO defers** | a **NEW** defer | **claimed as existing — REFUTED** |
| 5 | `client.go:250,255,260` | `NewClientConn` pre-spawn errors | `cancel()`; conn NOT closed | teardown | **MISSED** |
| 6 | `client.go:271-274` | `NewClientConn` ACK-wait `ctx.Done()` | caller gets nil, can NEVER `Close()` | explicit teardown | **MISSED** |
| 7 | `client.go:1009-1010` | `(*ClientConn).Close` | `cancel()` then `conn.Close()`, no join | join, or a stated ordering guarantee | **MISSED** |

⚠️ **Go defers are LIFO.** The record's *"ordered before `defer s.conn.Close()`"* is wrong as source
order: to RUN before the close, `defer s.closeReader()` must be placed **textually after** `conn.go:207`.

### 5.4 ⚠️ `closeReader` MUST BE A SIGNAL-AND-JOIN, NOT A DEADLINE STAMP

The read deadline is a **single conn-wide, last-writer-wins value with no ownership**. A `closeReader` that
merely stamps `SetReadDeadline(past)` races the reader's own deadline clears (`framer.go:173/185/191`);
clear-after-stamp **loses the stamp** and the reader blocks forever on a still-open conn. Worse, an
un-joined reader clearing the deadline turns the bounded 500 ms drains at `conn.go:278`/`:314` into
**unbounded blocking reads** — a NEW production hang in the very code this row exists to de-hang.

⚠️ `flushPendingDispatch` (`conn.go:356-360`) spawns `go fn()` goroutines that **outlive `Run`** and write
to `s.conn` afterwards. **"`Run` returned" != "the connection is quiescent"** — the leak test must POLL,
never sample once.

### 5.5 ⚠️ THE HANDSHAKE CREATES A THIRD PARKED STATE, AND A NEW ORDERING CONSTRAINT

**A1 — `closeReader` must unblock THREE parked states, and a deadline stamp reaches only the first.** The
reader can be parked (i) blocked in `ReadFrame` on the socket, (ii) blocked **sending** on the frame
channel, or (iii) blocked **waiting for release**. ⚠️ **State (iii) exists ONLY BECAUSE the handshake
exists** — it is created by this row's own fix. This is the decisive argument that `closeReader` is
signal-and-join and not a stamp: a stamp addresses (i) alone.

**A2 — release-then-drain is a NEW ordering constraint with no analogue today.** At `conn.go:311-317` the
error path runs `flushPendingDispatch()` -> `emitGoaway` -> `SetReadDeadline(+500ms)` -> `io.Copy`. Under
the handshake the release was **already sent at the start of that call**, so the reader is re-entering
`ReadFrame` on the socket at the exact moment the drain wants it exclusively. Two concrete failures: the
reader **steals the drain's bytes**, and `readFrameCtx`'s `SetReadDeadline(time.Time{})` **clears the
drain's own 500 ms bound**, converting a bounded drain into an unbounded blocking read. ⇒ **THE JOIN MUST
LAND BETWEEN THE RELEASE AND THE DRAIN.**

### 5.6 ⚠️ TWO PLACEMENT CAVEATS — get these wrong and the §4.5 parity is LOST

1. **The release must sit AFTER `readFrameCtx`'s ctx-err early return, NOT at the literal start of the
   call.** Today `framer.go:167-169` returns on `ctx.Err()` **without reaching** the `ReadFrame` at `:171`,
   so a ctx-cancelled read does **not** invalidate frame N. Releasing at the true start would invalidate
   where today's code does not — a behaviour change smuggled in as a refactor.
2. **ONE release per consumer call, never one per retry.** `readFrameCtx`'s loop re-enters `ReadFrame` on
   every 50 ms timeout (`framer.go:189` `continue` -> `:171`); those re-entries are idempotent with respect
   to invalidation. The PLAN must say this explicitly so nobody mints a release-per-retry.

⚠️ **AND THE BATCH-DRAIN CASE IS THE SAFEST, NOT THE RISKIEST — the reason is stronger than parity.**
**Today's empty `tryReadFrame` ALREADY releases frame N**: `frame.go:497-500` invalidates unconditionally
before `readFrameHeader`, so when `tryReadFrame` times out and maps the timeout to `(nil, nil)`
(`framer.go:214-216`), frame N was invalidated at `:499` and `fr.lastFrame` was never reassigned
(`checkFrameOrder` at `frame.go:545` is unreachable on that path). **A failed/empty `tryReadFrame` is an
EARLY RELEASE today.** ⇒ release-at-start-of-next-read cannot open a window the current code does not
already have; at worst it is **more conservative**.

---

## 6. D-91-GAP1 — the burst-drain / h2spec 5.1.2/1 ordering guarantee

### 6.1 The property, and why `tryReadFrame` is its entire load-bearing surface

`conn.go:163-168`, `:242-251` and `:291-295` state the contract in three places. **All HEADERS in one TCP
burst must be admitted (accepted or refused) before any accepted stream's dispatch goroutine starts
writing.** Without it a fast `direct_response` completes and writes DATA before the overflow HEADERS is
even read, and h2spec 5.1.2/1 — which reads the **FIRST** response frame and requires RST_STREAM, not DATA
— fails. This is documented history, not inference: `phases/05.1-downstream-h2/PROGRESS.md:645` records
`pendingDispatch` replacing an `atomic.Int32` for exactly this reason.

`tryReadFrame` is the sole mechanism deciding *"burst exhausted"*. ⚠️ **THIS IS WHY IT MUST NOT BECOME A
BARE `default:`** — a non-blocking select returns before the reader goroutine can be scheduled, silently
gutting the guarantee. It must wait **briefly on the channel**, abandoning only the wait, never a frame.

### 6.2 ⚠️ "PINNED BY ZERO TESTS" — CONFIRMED BY EXECUTION, NOT BY A GREP

A name-scoped grep would be worthless here (`reference_gate_selector_matched_nothing`). The claim was
tested by **gutting the drain and observing what fails**:

**Arm A** — `tryReadFrame` -> `return nil, nil` (`--numstat 2 16`, edit symbol-asserted by `grep -F`, and
a negative control confirming the 1 ms deadline literal reached **0** hits):

| scope | RC | RUN | PASS | FAILTESTS | SKIP |
|---|---|---|---|---|---|
| `h2`, run 0 | 1 | 204 | 203 | **1** | 0 |
| `h2`, runs 1-4 | 0 | 204 | 204 | 0 | 0 |
| `./internal/filter/hcm/` | 0 | 322 | 322 | 0 | 0 |

⚠️ **THE OPPOSITE CONCLUSION WAS NEARLY FILED AT n=1.** Run 0 reddened on
`TestSettingsWalk_DecreaseDrivesWindowNegative_WindowUpdateUnblocks` (i/o timeout,
`conn_settings_test.go:257`). Chased: that test passes **5/5 isolated under arm A**, the clean tree is
**5/5 green at 204/204**, and arm A then ran **4/4 green**. ⇒ a load-dependent one-off whose signature
(i/o timeout under full-package load) is **the same shape as the standing `TinyWindowDelivery` finding** —
plausibly **this row's own defect surfacing in a sibling test**. **At n=1 this would have been reported as
a refutation.** `reference_a_drift_correction_is_itself_a_claim` applies to a red arm as much as a green one.

⚠️ **The parent 322/322 is CORROBORATING, NOT INDEPENDENT** — that package completes in **0.017 s** and
never drives a real `ServerConn` frame loop.

**Arm B** — `time.Sleep(50ms)` then no frame (`--numstat 4 16`). Reddens deterministically 2/2 at
201 PASS / 3 FAILTESTS, wall time 1.76 s -> **42.4 s**. All three classified, and **NONE is an ordering
failure**: `TinyWindowDelivery` (30.01 s) and `Continuation_FloodExceedsBound` (5.00 s) are **slowdown**
timeouts; `WriteData_RespectsPerStreamSendWindow` is a **test artifact sensitive to drain LATENCY** — the
test drips `WINDOW_UPDATE(1,16)` every 5 ms (`conn_test.go:1201`) and arm B's latency lets grants
**coalesce**, so the server legitimately emits one 32-byte frame. Not an RFC violation; any fix faster
than 50 ms never trips it.

⇒ **Arm A is the load-bearing result. With the drain gutted, both ordering-adjacent tests —
`TestServerConn_MaxConcurrentStreamsEnforcement` and `TestServerConn_ConcurrentStreams` — pass 5/5.**

**Why the one plausible candidate is structurally incapable:**
`TestServerConn_MaxConcurrentStreamsEnforcement` uses `newBlockingAction`, whose `WriteH2` opens with
`<-a.release` (`conn_test.go:71`), and `close(releaseCh)` sits at `:333` — **after** the assertion at
`:328`. Streams 1 and 3 physically cannot emit DATA during the observation window, so the ordering is
imposed **by the fixture**, not observed from the subject. The assertion is `if refusedStreams == 0` — a
**COUNT, never an ORDER** (`reference_counter_cannot_gate_a_value`), and the read loop at `:314-326` breaks
on the first RST_STREAM, silently skipping any DATA before it.

**The differential suite is also blind:** `0079` and `0080` are **upstream** (they exercise `client.go`)
and both deliberately size the caps so REFUSED_STREAM **never** fires (`0079/expectations.yaml:35`,
`0080/driver_test.go:12`); `0004` asserts status/body/RR distribution and no ordering.

### 6.3 ⇒ D-91-GAP1: ADOPT h2spec 5.1.2/1 AS THE GATE, AND REQUIRE THE NEGATIVE CONTROL

**The IMPL MUST run `go test ./test/conformance/h2spec/ -timeout 5m -v`** and record the result against the
`95 tests, 94 passed, 1 skipped, 0 failed` baseline (`CONFORMANCE_PINS.md:142`, commit `83ebf029`,
2026-08-10; the skip is invariantly 6.9.2/2). Verified runnable at this tip: `"http2/5.1.2": 1` is a
**pinned** suite in `expectedSuites` (`h2spec.go:51`) reached by the `"http2/5"` selector (`:26`) under
`-S` strict, so a vanished suite trips guard layer 3 rather than passing silently; and `imageRef()`
(`h2spec.go:15`) resolves to a **local** image whose RepoDigest matches the pin **byte for byte** — no pull.
Cost ~9 s warm, already a per-push CI gate, zero new code.

⚠️ **THE NEGATIVE CONTROL IS THE LOAD-BEARING HALF, AND IT IS MANDATORY.** A green h2spec proves nothing
on its own: §6.2 shows the package suite is completely blind on this axis, so *"it passed"* is
uninformative without knowing the gate **can** fail. **The IMPL MUST demonstrate that 5.1.2/1 actually
REDDENS under a degenerate `tryReadFrame`** — arm A is the ready-made injection (`--numstat 2 16`,
restore verified). Without it, `reference_review_mandated_guard_is_untested` fires.

**REJECTED, with reasons recorded:**
- **A new ordering unit test (~80-120 lines).** It must avoid the blocking-dispatcher trap that makes the
  existing test vacuous — i.e. reproduce a TCP-burst race in-process with a *fast* dispatcher. High odds of
  shipping a **second vacuous green** (`reference_liveness_break_needs_failing_baseline`). h2spec already
  does this over a real socket for free.
- **A differential arm (~250-400 lines).** See §7.

⚠️ **RESIDUAL RISK, NAMED:** h2spec 5.1.2/1 exercises exactly ONE shape (fast `direct_response`, default
max=100). It does **not** cover trailer-driven dispatch (`conn.go:467`) or DATA-END_STREAM dispatch
(`conn.go:649`), which queue into the same `pendingDispatch`. **A single-case smoke gate, not a proof.**

---

## 7. D-91-DIFF — the differential posture: a NAMED DEPARTURE, on CORRECTED grounds

**DECISION: this row ships NO differential fixture. Fixtures stay 121; `0120` stays UNCONSUMED.**

⚠️ **R5 — THE BRAINSTORM'S STATED GROUNDS ARE WRONG, AND THE WRONG REASON WOULD GENERALIZE BADLY.** It
records this as *"an internal codec-robustness defect with **no reference-side counterpart**."* That
conflates two claims, and only one is true:

- **TRUE:** the reference cannot **exhibit** the defect. Envoy's C++ codec is nghttp2 driven by an
  event-loop callback feeding a stateful incremental parser — no deadline-polling blocking read, therefore
  no "retry and lose the drained bytes" mechanism. The departure is real.
- ⚠️ **FALSE:** that there is no reference-side **observable** to compare against. The defect's consequence
  is perfectly reference-comparable — the connection deadlocks, no response, no GOAWAY, no RST_STREAM.
  Envoy returns the response; envoy-go returns nothing. That is exactly what `CompareBytes` exists to catch.

**The real blocker is that the TRIGGER is not harness-controllable.** Recorded on these grounds instead:

1. **No scheduling knob exists.** The subject is a subprocess (`harness.go:252`, `:606`) spawned with **no
   `cmd.Env`** and no cpuset/nice/`GOMAXPROCS` hook of any kind. ⚠️ **The near-miss is named so it is not
   mistaken later: the ONE `cmd.Env` in the differential tree is `runner_test.go:1829`, and it sets
   `BACKEND_IDX` on a BACKEND, not the subject.** The sole measured reproduction lever — 8 burners pinned
   to core 0 with `GOMAXPROCS=1` — is unavailable by construction.
2. **The measured base rate makes any arm a flake, not a gate.** Uncontended is **5/5 GREEN**, so an
   uncontended arm is a **guaranteed-vacuous** green; contended-and-pinned is only 6/12. An arm whose green
   is indistinguishable from *"did not trigger"* violates
   `reference_liveness_break_needs_failing_baseline` by construction — delete the fix and it still passes
   most runs.
3. **A red arm is expensive and non-diagnostic.** 90 s per-fixture ctx (`runner_test.go:238`) × strictly
   sequential execution (**`t.Parallel()` appears ZERO times** in `test/differential/`) inside a 20-minute
   binary cap over 121 fixtures. ⚠️ **And a driver that blocks without its own deadline hangs the WHOLE
   binary and takes the remaining fixtures with it** — the harness does not independently watchdog
   `DriveSubject`.
4. **The comparable half is already covered.** The four H2-capable fixtures (`0004`, `0079`, `0080`,
   `0119` — the set re-verified this stage on three independent axes, `codec_type: AUTO` being the only
   value that reaches the H/2 codec) already exercise all three call sites under normal scheduling and
   already byte-compare. They regression-catch any **deterministic** breakage the fix introduces.

⚠️ **A `ReferenceLessFixture` (`fixture.go:693`, precedent `0007b`) IS constructible** — subject-only,
asserting via `SubjectAsserter` (`:709`) — **and is REJECTED anyway**: it still cannot set `GOMAXPROCS` on
the subject subprocess, so it would be a unit stress test wearing fixture clothing, burning a fixture index
and up to 90 s of a sequential budget for a probabilistic signal.

⚠️ **`HTTPExpectations` is HTTP/1.1-ONLY** (it routes through `helpers.HTTPRoundTrip`), re-confirmed this
stage — any H/2 fixture must assert through its own `Drive` hooks.

---

## 8. D-91-PROOF — the proof shape, and its stated limit

**The gate is at the UNIT level, in `internal/filter/hcm/h2`, where the counterfactual was already
measured.** Three required components:

1. **The contended regression measurement on `TestServerConn_TinyWindowDelivery`
   (`conn_test.go:862`), AT A WIDENED DENOMINATOR.** ⚠️ **0 FAIL / 12 IS NOT PROOF.** The BRAINSTORM's own
   negative control read **3/12 and 6/12 on two runs of the SAME unmodified tree**, so the arm's fail rate
   is itself noisy at n=12. **The IMPL MUST run n >= 30 per arm**, report the unpatched and patched rates
   side by side with the denominator asserted per iteration, and state the bimodality explicitly —
   ⚠️ **~0.07 s vs ~10.03 s with nothing between is how you tell this HANG from ordinary slowness, and it
   is the check phases 85 and 90 did not make.**
2. **h2spec 5.1.2/1 plus its mandatory reddening negative control** (§6.3).
3. **A goroutine-leak guard.** ⚠️ **`goleak` is NOT a dependency** — confirmed zero hits in `go.mod` and
   tree-wide — so the guard MUST reuse the in-repo pattern (`runtime.NumGoroutine()` baseline-delta plus a
   poll-until barrier, `internal/cluster/h2pool_test.go:1450`/`:1493`) rather than mint a `go.mod` change
   (`reference_new_subpackage_pulls_transitive_module`). ⚠️ **The h2 package has NO `pollUntil` of its own
   and the three existing copies have THREE DIFFERENT SIGNATURES** (`connpool_test.go:34`,
   `listener_test.go:130`, `hedge_test.go:353`) — an identifier collision; **mirror one, do not import
   one, and do not mint a fourth shape.** ⚠️ **The guard MUST POLL, never sample once** — §5.4.

⚠️ **A GREEN UNCONTENDED RUN IS NOT EVIDENCE.** 5/5 uncontended and 6/12 contended on the SAME binary.
**This is the reading that absorbed a live production deadlock as a flake three times.**

---

## 9. Cost — the BRAINSTORM's `+160 / −53` is a FLOOR, and this stage names what it cannot contain

⚠️ **NO NEW PROTOTYPE NUMBER IS QUOTED HERE.** The BRAINSTORM's prototype was built, measured and
**reverted**; it was never committed, so there is no patch in the tree to re-measure. Quoting a re-derived
figure against a patch nobody can inspect would be exactly the fabrication this project keeps catching.
**What this stage does instead is enumerate the specific work the floor cannot contain.**

`reference_measured_prototype_is_a_lower_bound` has fired **four consecutive rows**, always through
UNDER-ENUMERATION. This stage found the fifth cause before the IMPL did:

1. `closeReader` as a **signal-and-join** (stop chan + done chan + `sync.Once`), not a deadline stamp,
   unblocking **THREE parked states** — one of which the fix itself creates (§5.4, §5.5 A1).
2. That join **inlined at TWO drain sites**, not one deferred call, and **positioned BETWEEN the release
   and the drain** (§5.3 rows 2-3, §5.5 A2).
3. A defer in `readLoop`, which **has none today** (§5.3 row 4).
4. Up to **FOUR new teardown edges** in `NewClientConn` / `Close` (§5.3 rows 5-7).
5. An explicit **`startReader()` seam**, forced by the four lazy-start blockers (§5.2) — the floor contains
   no such seam.
6. The **release handshake** preserving the `frame.go:498-500` ownership invariant that `conn.go:291-295`
   documents (§4.5).
7. A **new leak-test file**, for which the h2 package has no `pollUntil` helper (§8 item 3).

**Any one of (6) or (7) alone plausibly exceeds the floor's headroom.**

**Baseline for the IMPL to reconcile against** (re-derived at `870fc90b`): `framer.go` **218** lines,
`conn.go` **984**, `client.go` **1013**; the `h2` package is **26** `.go` files = **10 production + 16 test**;
package tests total **8166** lines across 16 files, of which `conn_test.go` is **1770** (host of
`TestServerConn_TinyWindowDelivery` at `:862`) and `framer_test.go` is **337** (the likeliest home for the
new tests).

**Green baselines CONFIRMED at this tip under a before/after sha256 guard** (two independent agents
agreeing, one of them with the guard installed on both sides of the run):
`./internal/filter/hcm/h2/ -count=1 -v` => rc=0, **RUN 204 / PASS 204 / anchored-FAIL 0 / SKIP 0**
(204 = 204+0+0), 1.75 s; `./internal/filter/hcm/ -count=1 -v` => rc=0, **322 / 322 / 0 / 0**, 0.016 s.
⚠️ **THE PARENT FIGURE IS CORROBORATING, NOT INDEPENDENT** — 0.016 s means it never drives a real
`ServerConn` frame loop, so **a goroutine-leak regression would not show there.**

**ANTICIPATED ENVELOPE:** `+0` stats · `+0` config fields · `+0` fuzzers · `+0` BackendKind · `+0` `go.mod`
modules · `+0` fixtures · `+0` packages · **`+1` ADR (ADR-0313)** · `+0` ROADMAP rows · `+0` phase
directories. ⚠️ **The `+0` `go.mod` claim is CONDITIONAL on §8 item 3** — adopting `goleak` would break it.

---

## 10. Sentinel — RUN MECHANICALLY, ALL FOUR NCs, ACTUAL OUTPUT RECORDED

Run at stage start on the real file at `870fc90b`. **ACTUAL output, recorded not predicted:**

- **(1)** `want=123` => **`NOT DONE: row 91`** — ⚠️ **the HEALTHY reading while row 91 is open.**
- **(2)** => **SIX**, at `:201 :207 :213 :223 :229 :237`.
- **(3)** => **SILENT**.

⇒ **TWO checks block the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent
at the git root AND in the stage worktree.

All four NCs fired:

- **NC-A** — row-62 doctoring, `NC LANDED? [ in-progress ]` **inspected BEFORE trusting the result** =>
  the **TWO-line** form (`NOT DONE: row 62` AND `NOT DONE: row 91`), correct for an open board.
- **NC-B** — `want=122` on the real file => `NOT DONE: row 91` AND
  `GATE FAIL: examined 123 data rows, expected 122`.
- **NC-C** — check-(3) doctoring, residual **2 -> 0** confirmed FIRST => `NEVER OPENED: gRPC`, WASM
  correctly silent.
- **NC-D** — check-(2) matcher split: long **5** / short **1** / union **6**.

⚠️ **THE ARM-A MALFORMED-ROW GUARD DOES NOT READ ZERO, AND A SPEC THAT ASSERTS `== 0` WILL FAIL.** The
escape-aware form reads **2** — rows **57** (`NF=9`, line 119) and **69** (`NF=10`, line 131) — both
pre-existing content defects carrying literal unescaped `|` inside backticked code spans
(`` `|#k:v` `` and `` `w==nil || w.GetValue()` ``). The naive form reads **17** and is **NOT a drift
signal**. **Row 91 itself field-counts to NF=8 under BOTH forms.** Assert against the known-2 baseline.

⚠️ **THE `--` GUARD IS LOAD-BEARING, RE-CONFIRMED BY EXECUTION:** `command grep -c '-family row'` dies with
`grep: amily row: No such file or directory` while `command grep -c -- '-family row'` reads **67**.

---

## 11. Counts re-derived MECHANICALLY at `870fc90b` — never copied

- `ROADMAP.md` **241 lines / 123 data rows**; row 91 `in-progress` at `:153`; check-(2) anchors
  `:201 :207 :213 :223 :229 :237`; `-family row` **95 occurrences / 67 LINES**; `gRPC-family row` **2**;
  `Operational-tooling-family row` **3**.
- `DECISIONS.md` **18367 LINES** (⚠️ **LINES, not bytes — `wc -c` reads 4171364**) · tail **ADR-0312** ·
  next-free **ADR-0313** (`^## ADR-0313` => **0**) · strict `PROPOSED` guard **0** · `^---$` **216**.
  ⚠️ **THE HEADING FIGURE IS SCOPED: `^## ADR-` reads 311 while bare `^## ` reads 319** — the extra **8**
  are `## Amendment` headings. ⚠️ **311 < the tail id 0312: the ADR id space is SPARSE. Never derive
  next-free from a heading count.** ⚠️ **The strict guard is NOT self-locating** — a historical
  `^**Status:** PROPOSED` at **ADR-0231** (`:14866`) also reads 1.
- `BEHAVIOR_CONTRACT.md` **5962 LINES** (`wc -c` reads 1438532) · `STATE.md` **63** ·
  `STATE_HISTORY.md` **516** (strict evictee-absence guard **163**) · `BOOTSTRAP_PROMPT.md` **522**
  ⚠️ **at the REPO ROOT, not `docs/envoy-go/`** — and `git ls-files` shows exactly **ONE** tracked path,
  so the standing *"2 copies"* means two copies **WITHIN** the single file.
- phase dirs **132** · differential fixtures **121** at `test/fixtures/` (⚠️ **NOT
  `test/differential/fixtures/`, which DOES NOT EXIST and returns a SILENT 0** — control run at this tip)
  · numeric tail `0119-grpc-unary-trailers`, next-free **`0120` STAYS UNCONSUMED**. ⚠️ **121 DIRS but only
  120 DISTINCT INDICES** — `0007a-cors` and `0007b-iteration-probe` share index `0007`; next-free is
  max+1, **not** the directory count.
- fuzzers **55 / 48 files** (`*.go`-scoped under `internal/`) · **BackendKind TAIL 38**
  (= `H2GoawayResponder`) ⚠️ **a TAIL VALUE; the file declares 39 constants, values 0-38 contiguous** ·
  `REVIEW.md` **37** (a FILE count — the standing departure, NAMED not claimed).
- production `.go` under `internal/` **373** (764 total − 391 test).
- **Module path `github.com/pgdad/envoy-go`.**
- ⚠️ **`ROADMAP.md` cites `esalaine` 4 LINES / 7 OCCURRENCES — the long-carried "FIVE times" is WRONG on
  BOTH axES** (R6). No number is carried forward; re-derive and state the axis.

---

## 12. What the PLAN owes

1. **The concrete `framer` struct and the `startReader` / `closeReader` seam**, with `closeReader` as a
   **signal-and-join** over three parked states and the join inlined at both drain sites, **between the
   release and the drain** (§5.3, §5.4, §5.5).
1b. ⚠️ **THE TWO RELEASE-PLACEMENT CAVEATS, WRITTEN OUT (§5.6)** — release **after** `readFrameCtx`'s
   ctx-err early return, and **one release per consumer call, never per retry**. Getting either wrong
   silently loses the §4.5 parity and smuggles a behaviour change in as a refactor.
2. **All SEVEN wiring sites**, with the LIFO ordering stated correctly (§5.3).
3. **`tryReadFrame`'s bounded wait** — a concrete duration, and the argument for it. ⚠️ **NOT a bare
   `default:`** (§6.1). ⚠️ **And it must be well under 50 ms**, or it re-creates arm B's latency artifact
   (§6.2).
4. **The h2spec gate AND its mandatory reddening negative control** (§6.3).
5. **The contended measurement at n >= 30 per arm**, with the bimodality reported (§8 item 1).
6. **The leak guard**, mirroring an existing `pollUntil` shape, polling not sampling (§8 item 3).
7. **`readErr` stickiness** — new behaviour; the old code re-read and got the same error again. The PLAN
   must state it and decide whether it needs a test.
8. **Whether `-race` runs under the full differential suite.** ⚠️ **UNRUN at this stage** and a
   per-connection goroutine across the fixture fleet is exactly the shape that has produced `-race`-only
   findings here before.
9. ⚠️ **A gofmt + golangci-lint pass gated on OUTPUT**, with British spellings swept from `.go` comments
   BEFORE the gate (`reference_golangci_misspell_locale_us` fired **twice** on phase 90).

---

## 13. ADR-0313 §Context — DRAFTED HERE; the strict `PROPOSED` guard is RE-ARMED 0 -> 1

Drafted into `DECISIONS.md` as `## ADR-0313`, with `> **STATUS: PROPOSED` and **no `---` separator**
(`^---$` stays **216**). §Decision and §Consequences are appended IN PLACE at the IMPL, **after the
RETAINED italic footer**, per the ADR-0294-0312 shared block form.

⚠️ **THE ITALIC FOOTER IS INCLUDED AT THIS STAGE.** Phase 90's SPEC omitted it and its IMPL had to add the
append point its own STATUS line already claimed existed (ADR-0312 §Consequences (ix)). The footer form is
`*§Decision and §Consequences follow at the phase-91 IMPL.*`.

⚠️ **THE 0 -> 1 RE-ARM IS VERIFIED BY LINE AND ADR, NEVER BY THE COUNT ALONE** — the historical
`^**Status:** PROPOSED` at **ADR-0231** (`DECISIONS.md:14866`) also reads 1 and is a decoy.

---

## 14. Explicitly NOT MEASURED at this stage — stated so it is never inferred

- **h2spec was NOT RUN.** Its runnability, image digest, pin and recorded baseline were verified
  **read-only**; the result itself is inherited from `83ebf029` (2026-08-10) and is the IMPL's to reproduce.
- **The full differential suite was NOT RUN**, and **`-race` across the fixture fleet is UNMEASURED**.
- **No prototype was built at this stage.** The `+160 / −53` floor is inherited and explicitly NOT
  re-measured (§9).
- **The reference side of the framer defect** — no reference-side probe was run; §7's structural claim
  about nghttp2 is READ, not measured.
- **The encode/response direction**, the H/3 leg's own framing, and `readErr` stickiness under concurrent
  readers.
- **The contended denominator was NOT widened at this stage** — widening is assigned to the IMPL (§8).

---

## 15. Documentary defects — recorded, deliberately NOT fixed here

⚠️ **`ROADMAP.md` row 91's own cite `conn.go:217` for the SETTINGS write is OFF BY ONE** — `:218` is the
`writeServerInitialSettings` call; `:217` is its COMMENT. ⚠️ **An off-by-one that lands on a comment reads
as a correct cite** · ⚠️ **the "esalaine FIVE times" figure is wrong on both axes** (R6) · ⚠️ **ADR-0312
(xviii) item 1's `isValidAuthority` is DOCS-ONLY** — re-confirmed **0** in `.go` tree-wide, present in 5
doc files · `STATE.md` §Project counts SELF-CONTRADICT §Current (fixtures **119** / next-free **ADR-0299**
vs **121** / **ADR-0313**) and carry NO label saying so · `BEHAVIOR_CONTRACT.md:2040` still carries the
D-89-HOST rationale ADR-0312 declares RETIRED · ADR-0312 (xviii) item 4's *"12-row CWE-444 suite"* is **10**
rows landed · item 3 names the WRONG FIX SITE (`dispatchRequest`, should be `serveOneRequest`) · item 6's
H3 arm-A prediction is REFUTED · `ROADMAP.md` rows **57**/**69** malformed (the ARM-A guard) ·
`0004-h2-routing/README.md:40`'s *"the two new arms are appended"* is STALE · `0004-h2-routing/envoy-go.yaml:3`
says *"documentation only"* while `driver/driver.go` reads it · the phantom gate
`git grep -c 'h2.parseHeadersForRequest'` reads **1** (a COMMENT CITATION in `h2dispatch.go`) while the
definition selector `^func.*parseHeadersForRequest` reads **0** · `internal/filter/http/types.go`'s FALSE
*"per ADR-0071"* comment · `DECISIONS.md` records `INNER_EXIT=0` at phase 87, a value nothing in the tree
emits · the xDS cycle guard NOT AUTOMATED · `wasm/doc.go:219` two errors · `rbac.go:50` token `F2` ·
`ADR-0057`'s *"27 round-trips"* (now **31**).
