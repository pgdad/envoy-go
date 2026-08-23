# Phase 91 — `h2-framer-partial-frame-desync` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1)
**Base master:** `f71cc7e2`
**Branch:** `phase-91-brainstorm`
**Date:** 2026-08-22
**Row:** 91, registered `in-progress` at `ROADMAP.md` in THIS commit, `want` bumped **122 -> 123**
**ADR:** none this stage. The strict `^> **STATUS: PROPOSED` guard STAYS AT **0**; the SPEC re-arms it 0 -> 1.

---

## 1. The pick, and why it is defensible as "smallest first"

**SELF-PICKED** per the 2026-07-12 standing directive. No banked mid-lifecycle work existed and that was
PROVEN, not assumed: all **122** rows read `done`, check (1) went silent with the denominator asserted at
122, and NC-A returned the healthy one-line `NOT DONE: row 62` (§8).

**Charter: REMOVE THE READ-DEADLINE POLLING FROM THE HTTP/2 FRAME READER, so a frame read is never
abandoned part-way and the frame stream cannot desynchronize.**

⚠️ **THIS ROW IS NOT THE SMALLEST CANDIDATE BY LINE COUNT, AND THE BRAINSTORM DOES NOT CLAIM IT IS.**
It is the smallest **defensible** one, because measurement this stage collapsed most of the pool that
looked cheaper. Of the seven ADR-0312 §Consequences (xviii) follow-ons — the pool `STATE.md` and
`next-prompt.txt` both direct the roller to prefer — **five are now measured as refuted, unpickable, or
materially more expensive than their own registration claims** (§4). What remains is a **live production
correctness defect** that the project has already told itself to own.

**The defect, in one sentence.** `internal/filter/hcm/h2/framer.go` wraps `http2.Framer.ReadFrame()` in a
**read deadline and retries on timeout**; `ReadFrame` reads via `io.ReadFull`, so a deadline that expires
**mid-frame discards the bytes already consumed off the socket**, and the retry resumes at the wrong byte
offset — permanently desynchronizing the frame stream, silently swallowing a `WINDOW_UPDATE`, and
deadlocking the connection with **no error, no `GOAWAY`, and no `RST_STREAM`**.

**Why it is defensible ahead of everything else on the board:**

1. **It is a real production bug, not a coverage gap or a documentary defect.** It sits on the H/2
   downstream read path for **every** connection, and — a fact the standing record does **not** carry —
   also on the **upstream client** read path (`client.go:307`). Three production call sites, not one.
2. **The project already chartered it in principle.** Phase-80 `PLAN.md:636` states in terms that a
   recurrence *"is now a FINDING, not a re-run"*, and ADR-0312 §Consequences (xii) records the second
   post-fix recurrence and says a future row should own it. This row is that row.
3. **It has been misclassified as a flake three times** and the misclassification is itself measurable
   (§2.5). Leaving it costs the project a recurring false flake attribution on every full-suite run.
4. **It is reproduced, characterised, and counterfactually isolated AT THIS TIP** — by the controller's
   own execution, not inherited (§2).

⚠️ **THE ONE HONEST COUNTER-ARGUMENT, STATED RATHER THAN BURIED.** The cheapest *pin* on the board is
still H1-B′'s single table row (§4.3) at roughly a dozen lines. A roller optimising purely for size would
take it. This row rejects that trade because H1-B′'s **fix** was refuted as cheap this stage, and because
a pin that documents a divergence without repairing a live deadlock is the weaker use of a phase.

---

## 2. The defect, REPRODUCED BY EXECUTION at `f71cc7e2` — with a positive control, a discriminating counterfactual, and a negative control on the harness

### 2.1 The mechanism, in the landed code

`internal/filter/hcm/h2/framer.go`, cited BY SYMBOL:

- **`(*framer).readFrameCtx(ctx)`** (`:164`) arms `SetReadDeadline(now + 50ms)` **inside a loop** and, on
  timeout, re-checks `ctx` and **re-calls `f.ReadFrame()`**.
- **`(*framer).tryReadFrame()`** (`:201`) arms `SetReadDeadline(now + 1ms)` and, on timeout, returns
  `(nil, nil)` meaning *"no frame yet"* — leaving whatever bytes it consumed unaccounted for.

Production call sites (`git grep -n 'readFrameCtx\|tryReadFrame' -- 'internal/filter/hcm/h2/*.go'`, test
files excluded) — **THREE, and the third is the one the record omits**:

```
internal/filter/hcm/h2/conn.go:259    s.fr.readFrameCtx(s.ctx)    (*ServerConn).Run          — downstream
internal/filter/hcm/h2/conn.go:304    s.fr.tryReadFrame()         batch-drain loop           — downstream
internal/filter/hcm/h2/client.go:307  cc.fr.readFrameCtx(cc.ctx)  upstream client read loop  — UPSTREAM
```

The comment above `readFrameCtx` explains the deadline as a way to observe context cancellation *"within
bounded latency"*. That intent is legitimate; the **implementation** is what is wrong, because it makes
cancellation latency and frame-stream integrity share one mechanism.

### 2.2 Positive control — uncontended, and it proves nothing

```
go test -c ./internal/filter/hcm/h2/ -race -o <scratch>/h2.test      # BUILD_RC=0
./h2.test -test.run '^TestServerConn_TinyWindowDelivery$' -test.count=1 -test.v   x5
  iter=1..5  rc=0  RUN=1  FAILlines=0
BASELINE: ran=5 pass=5 fail=0 (denominator 5)
```

⚠️ **A GREEN RUN IS NOT EVIDENCE THE DEFECT IS ABSENT.** This is exactly the reading that got the defect
absorbed as a flake at phases 85 and 90 (`cleared scoped x5 + full-package x3`). It is recorded here as a
control, not as a result.

### 2.3 The contended arm — the defect, reproduced

Harness: 8 CPU burners pinned to core 0, the test pinned to core 0 with `GOMAXPROCS=1`. Burners started
and killed **by captured PID** (never `pkill -f`, which matches the tool call's own shell and exits 144).
`nproc` = 32. Denominator asserted per iteration via the `=== RUN` count.

```
  iter=1  rc=0 RUN=1 (0.08s)  PASS
  iter=2  rc=1 RUN=1 (10.04s) read frame after 583/1024 body bytes
  iter=3  rc=0 RUN=1 (0.07s)  PASS
  iter=4  rc=1 RUN=1 (10.02s) read frame after 217/1024 body bytes
  iter=5  rc=0 RUN=1 (0.09s)  PASS
  iter=6  rc=1 RUN=1 (10.02s) read frame after 394/1024 body bytes
  iter=7  rc=0 RUN=1 (0.06s)  PASS
  iter=8  rc=1 RUN=1 (10.05s) read frame after 677/1024 body bytes
  iter=9  rc=0 RUN=1 (0.08s)  PASS
  iter=10 rc=0 RUN=1 (0.08s)  PASS
  iter=11 rc=1 RUN=1 (10.04s) read frame after 785/1024 body bytes
  iter=12 rc=1 RUN=1 (10.03s) read frame after 425/1024 body bytes
CONTENDED: ran=12 pass=6 fail=6 (denominator 12)
```

⚠️ **THE DISTRIBUTION IS STRICTLY BIMODAL — ~0.06-0.09 s or ~10.02-10.05 s, with nothing in between in
12 trials.** 10 s is exactly the test's own `frameReadTimeout`. **That is a hang, not slowness.** A
throughput shortfall would smear durations across the interval; this does not. The stall points are
scattered (217, 394, 425, 583, 677, 785 of 1024), so it is not a fixed-offset parse bug.

### 2.4 The DISCRIMINATING COUNTERFACTUAL — widen only the two deadlines

Nothing else changed. `git diff --numstat` = `2 2` on `framer.go` alone; the edit was asserted **BY
SYMBOL** (`5000 * time.Millisecond` at `:170`, `300 * time.Millisecond` at `:202`), not by a successful
build. Identical harness, identical machine, identical burner count:

```
  iter=1..12  rc=0 RUN=1  (0.36s – 0.40s)
WIDENED: ran=12 pass=12 fail=0 (denominator 12)
```

**6 FAIL / 12 unpatched -> 0 FAIL / 12 patched, and the bimodality vanishes** (every run lands in a tight
0.36-0.40 s band). ⚠️ **THE WIDENING IS NOT THE FIX** — it only narrows the window in which a deadline can
land mid-frame. It is here solely because it **isolates the cause to the deadline-polling read** and rules
out the flow-control accounting, the window arithmetic, and the test's own client.

`framer.go` restored via `git checkout --` and verified byte-identical by `sha256sum -c`:
`ad62d4922a9b380163ce516f3658c131bc19350b69eec5229e49eee853d393d5  internal/filter/hcm/h2/framer.go` — `OK`.

### 2.5 The three-way park at the moment of the hang

Captured with `timeout -s QUIT` against a stalled contended run:

- **test client** — parked in `http2.(*Framer).ReadFrame` (`conn_test.go:969`), waiting for DATA. It has
  **already sent** its `WINDOW_UPDATE(1,1)`.
- **`(*ServerConn).Run`** — parked in `netpollblock` inside `readFrameCtx` (`framer.go`), on a **9-byte**
  `io.ReadFull` — i.e. **at a frame-header boundary**, waiting for a frame that will never arrive.
- **the writer** — parked in `(*window).reserveBlocking` (`flow.go`) via `(*ServerConn).writeData`
  (`conn.go`), after 63 of 1024 bytes, waiting on the per-stream send window `ss.sendW`.

⇒ **A `WINDOW_UPDATE` was consumed off the socket and never applied.** `flow.go` is the victim, not the
cause.

### 2.6 ⚠️ THE HISTORICAL RECORD IS WRONG IN A WAY THAT MATTERS

- `f46ba419` — the commit phase-80 records as the **FIX** — touches `.github/workflows/ci.yml` and
  `internal/filter/hcm/h2/conn_test.go`. **It changes ZERO production bytes.** It moved the *test's* read
  deadline from once-before-the-loop to per-frame and widened the ctx 5 s -> 30 s. ⇒ **calling the defect
  "FIXED" was never warranted; the commit made the test survive the bug, not the bug go away.**
- `f2dd994a` touches only `test/differential/harness_test.go` (the backend port band). It is unrelated to
  this defect and was never capable of fixing it.
- Phase 85 (`STATE_HISTORY.md:486`) recorded the first recurrence as *"a NEW unclassed flake … recorded,
  not fixed"* — ⚠️ **it did NOT recognise it as a post-fix recurrence, so the phase-80 FINDING instruction
  was not honoured there.** ADR-0312 §(xii) is the first record to connect the three events.

---

## 3. The mechanism, stated precisely

`http2.Framer.ReadFrame` reads a 9-byte header and then the payload via `io.ReadFull` on the underlying
`net.Conn`. A `SetReadDeadline` that expires **between those two reads, or part-way through either**,
returns a timeout error — and the bytes already drained from the socket are **gone**: they are neither
returned to the caller nor pushed back onto any buffer the next call can see.

`readFrameCtx` treats that timeout as *"nothing arrived yet"* and calls `ReadFrame` again. The next call
reads the **continuation** of the abandoned frame and interprets its first 9 bytes as a **new frame
header**. From that instant the parse is offset and every subsequent frame boundary is wrong.

The tiny-window workload makes this maximally likely and maximally damaging: it is a long run of
identical 13-byte `WINDOW_UPDATE` frames (9-byte header + 4-byte payload), so a shifted parse consumes one
without ever surfacing it to the flow controller. Nothing errors. No `GOAWAY` is emitted. The writer waits
on a window that will never be credited, and the reader waits at a header boundary for a frame the peer
already sent. **Silent, permanent deadlock.**

Under CPU starvation a **1 ms** deadline (`tryReadFrame`) fires mid-read routinely, which is why
contention is the trigger and why the defect is invisible on an idle machine.

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

⚠️ **`reference_deferred_candidate_cost_restale` was checked and, on the code axis, DOES NOT FIRE this
time:** `git diff --numstat b312fc95..HEAD` over the whole tree reads `15 1 next-prompt.txt` and nothing
else, so every candidate file is byte-identical to the commit its cost was measured at. **The costs went
stale for a different reason — they were wrong when written.**

### 4.1 Arm C, the authority VALIDITY reject + the duplicate-`host` reject (ADR-0312 xviii 1+2) — **REJECTED, three blockers**

- ⚠️ **THE SHAPE DECISION HAS THREE OPTIONS, NOT TWO.** Beyond (a) stream-scoped `RST_STREAM` via
  `buildRequest` and (b) connection-scoped GOAWAY-and-close, there is **(c) connection-scoped BARE CLOSE** —
  suppressing the GOAWAY through `emitGoaway`'s existing `goawaySent` guard (`conn.go:864`) and relying on
  `Run`'s deferred `s.conn.Close()` (`conn.go:207`). **(c) is the only shape matching the reference's
  no-GOAWAY/no-RST teardown, and it is the shape candidate (2) requires.** The ADR enumerates only (a) and
  (b), and prices (b) against `(*serverStream).recvHeaders` — a site that **cannot express (c)** at all,
  because `serverStream` holds only the three-method `streamConn` interface.
- ⚠️ **NO SHAPE CAN REACH BYTE-PARITY.** `Run` **Step 2 writes the server's initial SETTINGS at
  `conn.go:217`, before the frame loop reads any HEADERS.** The reference's teardown emits *"not even the
  server's own SETTINGS."* The subject has already put bytes on the wire before any authority reject can
  fire.
- ⚠️ **NO VALIDATION PREDICATE EXISTS TO REUSE.** `isValidAuthority` appears in **docs only** — zero `.go`
  hits tree-wide; there is no `httpguts`, no `ValidHeaderField`, no token/charset validator anywhere. And
  ADR-0312 §(xix) states the reference's accepted authority **charset was never characterised**. ⇒ the row
  opens with **two** unmeasured blockers, not the one the ADR names.
- Unpriced extras: the D-90-DUP pin (`authority_norm_test.go:106`) must be **rewritten, not extended**
  (a `−N` the production-only figures omit), and minting the reference's stat drags an `internal/stats`
  **dependency edge** into a package that is deliberately stats-agnostic per ADR-0254.

### 4.2 H1-D — duplicate `Host` (ADR-0312 xviii 4) — **REJECTED, unchanged**

envoy-go is the RFC 7230 §5.4-conformant side; parity means **defeating the Go stdlib parser** (~5× the
in-scope fix) and is a request-smuggling-surface change. Re-measured on the reference this stage: duplicate
`Host` yields **200** with the authority coalesced to the literal `a.example,b.example` in wire order —
confirming comma-coalescing directly rather than inferring it from a 200.

### 4.3 H1-B′ — HTTP/1.1 with no `Host` (ADR-0312 xviii 3) — **REJECTED as "cheapest"; its PIN half is BANKED**

The ADR calls this *"the cheapest structurally … zero new machinery."* **Measured against the pinned
reference this stage, that is true of the pin and FALSE of the fix.**

Reference arms (`envoyproxy/envoy:contrib-v1.37.2`, fresh connection per arm, authority made observable
via `response_headers_to_add: x-seen-authority: "[%REQ(:AUTHORITY)%]"` and four virtual hosts):

| arm | bytes | reference | authority / vhost |
|---|---|---|---|
| A0 control | `Host: a.example` | **200** | `[a.example]` -> `vh_a` |
| A1 no `Host` | `GET /x HTTP/1.1\r\n\r\n` | **400**, `connection: close`, empty body, conn closed | never routed |
| A2 empty `Host:` | `Host: ` | **200** | **`[]`** -> `vh_default` (`*`) |
| A3 whitespace `Host:` | `Host:    ` | **200** | **`[]`** -> `vh_default` |
| A4 H/1.0 no `Host` | `GET /x HTTP/1.0` | **426** | never routed |
| A7 H/1.0 **with** `Host` | `GET /x HTTP/1.0`, `Host: a.example` | **426** — byte-identical to A4 | never routed |
| A5 absolute-form | `GET http://a.example/x` | **200** | `[a.example]` -> `vh_a` |
| A6 dup `Host` | two `Host` fields | **200** | `[a.example,b.example]` -> `vh_ab` |

Envoy's own log for A1: `Sending local reply with details missing_host_header`. Stats: `downstream_rq_4xx+1`
and **`downstream_cx_protocol_error` did NOT move** — whereas A4 **does** move it, confirming the 400/426
stat split the ADR predicted.

⚠️ **THE REFUTATION.** The reference's 400 is scoped to **ABSENCE**; empty and whitespace-only `Host` are
**accepted with 200** and yield an empty authority. But `http.ReadRequest` **erases that distinction** —
controller-verified with a discriminating positive control:

```
A1-absent    req.Host="" URL.Host="" Header["Host"]present=false len(Header)=0
A2-empty     req.Host="" URL.Host="" Header["Host"]present=false len(Header)=0
A3-ws        req.Host="" URL.Host="" Header["Host"]present=false len(Header)=0
A5-absform   req.Host="a.example" URL.Host="a.example"      <- control discriminates
ctrl-normal  req.Host="a.example" URL.Host=""               <- control discriminates
```

⇒ **A guard written `req.Host == ""` OVER-FIRES: it would 400 A2 and A3, where the reference returns 200,
converting ONE measured divergence into TWO new ones.** Reference parity requires distinguishing absent
from present-and-empty **before or during** parsing — a raw-head peek ahead of `http.ReadRequest`
(`connection.go:163`) or a replacement header reader. **That is new machinery, in a
request-smuggling-sensitive path.**

Two further corrections earned by the same run: **A4 is not a Host divergence at all** — A7 proves the 426
is caused purely by the HTTP/1.0 version, independently confirming ADR-0312 §Context ¶5's mis-attribution
note by execution. And **A5 is a non-divergence** — absolute-form supplies the authority on both sides.

⚠️ **The ADR ALSO names the wrong fix site.** It says `connection.go::dispatchRequest`; that function is
entered *after* `downstreamRqTotal.Inc()` and its only early exit is the 404 path. The two existing
pre-dispatch rejects (`Expect`->417 at `:228`, `Upgrade`->501 at `:237`) live one frame up in
**`(*Filter).serveOneRequest`**, which already has the exact shape the guard needs.

**BANKED, not discarded:** the **pin** half is genuinely zero-machinery — one 5-field row appended to
`TestH1Robustness_KnownDivergencesFromEnvoy`, whose `rawExchange` helper accepts the input unchanged. It
now also has its reference status **measured** rather than asserted. A future row should take it.

### 4.4 The D-89-HOST residue, `h2ReconcileSkipKey` (ADR-0312 xviii 5) — **REJECTED, UNPICKABLE AS WRITTEN**

⚠️ **"Option (b)" is NOWHERE ENUMERATED IN THE TREE.** Scoped to the phase-89 and phase-90 directories,
`option (b)` occurs **exactly once** (`phases/90-…/PLAN.md:983`) and only as a bare label; option (a) is
stated inline once (`SPEC.md:463`, *"leave the SKIP untouched"*). **No sentence anywhere says what (b) is**,
and the nearest candidate (`phases/90-…/BRAINSTORM.md:255`) is explicitly marked *"UNRESOLVED BEHAVIOR
DECISION"* and describes at least two different behaviours (scalar write-back vs carrier projection).
⇒ the row's opening question is not *"is (b) defensible"* but *"what is (b)?"*.

Two further corrections: **D-89-HOST's grounds are not in ADR-0311** — `D-89-HOST` is not one of ADR-0311's
five labelled decisions; the enumerated grounds live at `phases/89-…/PLAN.md:78-88`. And **the retirement is
over-broad**: ground 2 has three clauses and row 90 retires only the middle one, and only for the
client-sent arity. ⚠️ **New finding:** `BEHAVIOR_CONTRACT.md:2040` still carries that rationale verbatim
(*"projecting it would put a mutated `host` beside an unmutated `:authority`"*), so the contract asserts
what ADR-0312 declares retired — an unpriced contract edit any taker inherits.

### 4.5 The H3 arm-A prediction (ADR-0312 xviii 6) — **REFUTED: THERE IS NO DEFECT**

The ADR predicts arm A *"is READ-PREDICTED to reproduce"* on H/3. **It does not.**

quic-go `v0.54.1` does preserve a regular `host` (its `invalidHeaderFields` set omits it, so `Add`
canonicalizes it to `req.Header["Host"]`). But **there is no H/3 analogue of `buildH2Request` at all** —
`h3dispatch.go` never special-cases `host` or `:authority`, and `runH3` routes through the **H1** action
(`asRouterAction`, not `asRouterActionH2`) to `doH1ClusterAction`, whose upstream write is the stdlib's
`(*http.Request).Write`. That function's `reqWriteExcludeHeader` table drops `Host` **unconditionally** and
re-emits it from `r.Host`. Executed: the arm-A upstream bytes are **byte-identical to the control**.

⇒ **the regular `host` cannot reach the upstream carrier on H/3 under any input, and no subject code
implements that suppression — the stdlib does.** Arm B is separately impossible: quic-go rejects an
empty/absent `:authority` with an H3_MESSAGE_ERROR **stream** reset before `ServeH3` is ever invoked.

Re-scoping the candidate to *"does the **reference** diverge on H/3"* costs a raw QPACK/H3 client that does
not exist in-tree (`quic-go/qpack` has **zero** `.go` references and is an **indirect** dep, so using it
changes `go.mod`), plus a new **routed** H3 fixture — `0104-http3-downstream-get` is the only H3-capable
fixture of 121 and its route is a pure `direct_response` with no upstream leg. Floor ~300-400 new lines
plus a `go.mod` change, and that is a LOWER BOUND.

### 4.6 The access-log / Zipkin-span axis (ADR-0312 xviii 7) — **REJECTED: NOT A LIVE DIVERGENCE**

The sourcing asymmetry is real and confirmed by symbol — H1 `Authority: r.Host` (`accesslog_emit.go:55`),
H2 `Authority: req.Authority` (`:116`), H3 `Authority: r.Host` (`:177`). ⚠️ **But row 90 already closed the
VALUE gap:** `emitAccessLogH2` receives the `h2req` produced by `buildH2Request`, which now promotes the
host, so `%REQ(:AUTHORITY)%` already reads the host rather than `-`. ⇒ **what remains is an UNASSERTED
axis — a regression guard for row 90's own fix — not a live divergence.** The one arm still divergent in
value belongs to arm C (§4.1), not here.

It is also **cheaper than the ADR implies**, and that is recorded so the next taker does not re-derive it:
`AccessLogAsserter` (`fixture.go:671`) and `ReferenceLogMounter` (`fixture.go:663`) already exist and are
already dispatched by the runner (`runner_test.go:1170`, `:1357`), so the surface is a config block plus a
driver method — **no new runner machinery and no new fixture index**. The four H2-capable fixtures
(`0004`, `0079`, `0080`, `0119`) contain not even the *word* `access_log`/`tracing`/`zipkin`, and all 17
surface-carrying fixtures are plaintext HTTP/1.1 with zero `transport_socket` — so the surface must be
**minted into** an H2 fixture, and `0079`/`0080`/`0119` are cheaper hosts than `0004` because their config
is a driver-inline Go string with no reference-yaml twin.

### 4.7 The family-window pool — **REJECTED as a pool, and its inventory CORRECTED**

Per the standing guidance the ADR-0312 pool is preferred because its spade-work is freshest; every
family-window candidate needs its cost re-derived from scratch. The inventory itself was re-derived
mechanically (§9.3) and three inherited claims about it are refuted.

---

## 5. Family attribution

**Core-HCM / HTTP-2-codec MAINTENANCE row claiming NO family ordinal.** A maintenance row repairs a landed
deliverable and does not extend a charter — the row-85/86/87/88/89/90 precedent, unbroken for six rows.
The Observability, HTTP/3, gRPC, xDS, Runtime and Operational-tooling family charters are untouched by
this row, and check (2) must therefore still read **SIX** at close.

---
## 6. Anticipated ADR, counts, and the cost FLOOR

### 6.1 The MEASURED cost floor — a built, run, and reverted prototype

A compiling prototype implementing the reader-goroutine shape was built at this tip, measured, and
reverted (`sha256sum -c` => `OK`, `git diff --stat master` EMPTY).

| axis | measured |
|---|---|
| production `.go` | **+160 / −53** |
| files | **3** — `framer.go` (+151/−53), `conn.go` (+5/−0), `client.go` (+4/−0) |
| packages | **1** (`internal/filter/hcm/h2`) |
| test-side | **+0 / −0** in the prototype |
| `gofmt -l` | output **EMPTY** (gated on OUTPUT — it never exits non-zero) |
| `golangci-lint` | rc=0 **and output EMPTY** (gated on OUTPUT, per the phase-90 lesson) |

⚠️ **THIS IS A LOWER BOUND AND `reference_measured_prototype_is_a_lower_bound` WILL FIRE AGAIN.** It has
fired on four consecutive rows, always through UNDER-ENUMERATION. The named gaps are in §6.4; the
test-side `+0` in particular **cannot survive**, because a fix whose central invariant is untested is the
shape `reference_review_mandated_guard_is_untested` warns about.

### 6.2 Does the prototype work, and is the measurement discriminating?

| arm | contended result (denominator asserted, `RUN=1` each) |
|---|---|
| unpatched baseline | **3 FAIL / 12** |
| **prototype installed** | **0 FAIL / 12** (all ~0.08-0.10 s; not one 10 s hang) |
| **reverted, rebuilt, re-run (NEGATIVE CONTROL)** | **6 FAIL / 12** |

⚠️ **THE NEGATIVE CONTROL IS LOAD-BEARING AND IT ALSO EXPOSES A SAMPLING LIMIT.** The harness still
reddens after revert, so the prototype's 0/12 is a real signal — **but the same unmodified tree read 3/12
and 6/12 on two runs**, so the arm's fail rate is itself noisy at n=12. ⇒ **the row must widen the
denominator; 0/12 is not proof of a fix.** Recorded here so the SPEC does not inherit 0/12 as settled.

Regression gates with the prototype installed, denominators reconciled:
`go test ./internal/filter/hcm/h2/ -count=1 -race -v` => rc=0, **204 RUN / 204 PASS / 0 SKIP / 0 anchored
FAIL / 0 `DATA RACE`** (204 = 204 + 0 + 0). Parent package `./internal/filter/hcm/` => rc=0, **322 RUN /
322 PASS / 0 FAIL / 0 `DATA RACE`**.

### 6.3 ⚠️ TWO DESIGN CONSTRAINTS DISCOVERED BY THE PROTOTYPE — the SPEC must not re-derive them

1. **A BUFFERED CHANNEL IS UNSAFE AND THE UNSAFETY IS UNFIXABLE FROM THIS PACKAGE.** `http2.Framer`
   hands out payload sub-slices of a **shared** `fr.readBuf` — verified in the pinned
   `golang.org/x/net@v0.34.0/http2/frame.go`:
   ```go
   fr.getReadBuf = func(size uint32) []byte {
       if cap(fr.readBuf) >= int(size) {
           return fr.readBuf[:size]     // SHARED backing array
       }
       ...
   ```
   `SettingsFrame.p`, `GoAwayFrame.debugData`, `HeadersFrame.headerFragBuf`, `ContinuationFrame` and
   `PushPromiseFrame` all retain slices into it, and **every one of those fields is unexported**, so a
   deep copy is impossible from outside `x/net`. (`SetReuseFrames` is a red herring — controller-verified
   as **never called** anywhere in `internal/`, so it is not the hazard.) ⇒ the shape must be an
   **unbuffered channel plus a release handshake**, releasing at the *start* of each read call, which
   reproduces today's ownership contract exactly with **zero copies**. `conn.go:294` already documents
   that contract.
2. **`tryReadFrame` MUST NOT BECOME A BARE `default:`.** A non-blocking select returns before the reader
   goroutine can be scheduled, silently gutting the burst-drain that `conn.go`'s own comment says
   h2spec 5.1.2/1 depends on. It must wait briefly **on the channel** — abandoning only the wait, never a
   partial frame.

### 6.4 The named gap list the SPEC must price

1. ⚠️ **The burst-drain / h2spec 5.1.2/1 ordering guarantee is pinned by ZERO tests.** The full 204-row
   package went green with a completely re-timed `tryReadFrame` — **so the package suite cannot tell you
   whether ordering survived.** This is the largest unpriced risk and only h2spec or the differential
   suite can speak to it.
2. **The goroutine lifecycle is load-bearing.** `closeReader` needs wiring at **four** sites — `conn.go`'s
   `defer` (ordered **before** `defer s.conn.Close()`), **before** each of the two GOAWAY drain paths
   (otherwise the reader and the drain race for the same socket), and `client.go`'s `readLoop` defer
   (because ctx-cancel teardown alone never closes the client socket).
3. **Lazy reader start is load-bearing and untested** — `settings.go:121` (`readClientSettings`) calls the
   embedded `fr.ReadFrame()` **directly**, as do many test call sites, so starting the goroutine in
   `newFramer` would race all of them.
4. **The aliasing invariant is handled but untested** — nothing would redden if the channel were later
   given capacity.
5. **`readErr` stickiness is new behaviour** (the old code re-read and got the same error again).
6. **`-race` under the full differential suite is unrun**, and a per-connection goroutine across the whole
   fixture fleet is exactly the shape that has produced `-race`-only findings in this project before.
7. **Docs/ADR axis is `+0` in the prototype and cannot stay `+0`.**
8. ✅ **The test axis is genuinely `−0`** — grepped and confirmed: **no test anywhere pins the 50 ms / 1 ms
   polling shape.** `conn_test.go:958` mentions 50 ms only in a comment about the *test's own* client-side
   deadline. That is a positive finding, and it is also *why* gap 1 bites: nothing was protecting the
   behaviour.

### 6.5 Anticipated ADR and count envelope

**ADR-0313** — next-free, TAIL-derived (`grep -oE '^## ADR-[0-9]+' … | tail -1` => `## ADR-0312`;
`grep -c '^## ADR-0313'` => **0**). ⚠️ **NEVER derive from the heading count**, and the heading figure is
SCOPED: `^## ADR-` reads **311** while bare `^## ` reads **319** (the extra 8 are `## Amendment` headings).
⚠️ **A new ADR takes NO `---` separator** — `^---$` stays **216**.

Counts re-derived mechanically at `f71cc7e2` this stage, each stated with its scope:

- `ROADMAP.md` **240 lines / 122 data rows** -> **241 / 123** at this commit (a pure row INSERTION;
  `git diff --numstat` must read `1 0`), sentinel `want` **122 -> 123**.
- `DECISIONS.md` **18367** · tail **ADR-0312** · next-free **ADR-0313** · strict `PROPOSED` guard **0**
  (⚠️ decoy `^**Status:** PROPOSED` at ADR-0231 reads **1** — verify any arm BY LINE AND ADR) · `^---$`
  **216**.
- `BEHAVIOR_CONTRACT.md` **5962** · `STATE.md` **63** · `STATE_HISTORY.md` **514** (strict evictee-absence
  guard **163**, loose **207**) · `BOOTSTRAP_PROMPT.md` **522**.
- phase dirs **131 -> 132** (this row adds one) · differential fixtures **121** at `test/fixtures/`
  (⚠️ NOT `test/differential/fixtures/`, a plausible wrong path returning a SILENT 0), numeric tail
  `0119-grpc-unary-trailers`, next-free **`0120` STAYS UNCONSUMED**.
- fuzzers **55 / 48 files** (`*.go`-scoped under `internal/`) · BackendKind **tail 38** (⚠️ a TAIL VALUE,
  not a count) · `REVIEW.md` **37** (a FILE count; newest at `phases/25.3-…` — the standing departure).
- `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** ·
  `Operational-tooling-family row` **3** · ARM-A malformed ids **57** (`NF=9`) and **69** (`NF=10`) at
  lines **119**/**131**, ESCAPE-AWARE (⚠️ the naive `awk -F'|'` form reads **17** and is NOT a drift
  signal).

**ANTICIPATED ENVELOPE for the row as a whole:** `+0` stats · `+0` config fields · `+0` fuzzers · `+0`
BackendKind · `+0` `go.mod` modules · `+0` fixtures · `+0` packages · **`+1` ADR (ADR-0313)** · `+1`
ROADMAP row · `+1` phase directory. ⚠️ **The `+0` fixture claim is an ANTICIPATION, not a measurement** —
§7 item 6 makes the differential posture an open SPEC decision, and if the SPEC buys a contended
regression surface the fixture axis may move.

---
## 7. What the SPEC owes

1. **DECIDE THE FIX SHAPE, and price the alternative.** The intended shape is a **dedicated reader
   goroutine + channel**, so `ReadFrame` runs with **no deadline** and is never abandoned part-way;
   `readFrameCtx` becomes a `select` on `ctx.Done()` and the channel, and `tryReadFrame` becomes a
   **non-blocking** `select` with a `default:`. The SPEC must state why this beats the alternatives
   (a resumable buffering wrapper is harder because `http2.Framer` holds internal parse state; simply
   widening the deadlines is **not a fix** — §2.4 uses it only as a counterfactual).
2. ⚠️ **SETTLE THE FRAME-PAYLOAD ALIASING QUESTION BY READING THE x/net SOURCE, NOT BY ASSUMING.**
   `http2.Framer` can reuse its internal read buffer across `ReadFrame` calls. Handing frames to another
   goroutine over a channel may let a later read **overwrite the payload of a frame the consumer still
   holds**. The SPEC must quote what `framer.go`'s construction actually does about this and, if a copy is
   required, put that cost in the figure.
3. ⚠️ **COVER ALL THREE CALL SITES OR SAY WHY NOT.** The upstream client (`client.go:307`) is on the same
   defective path and the standing record omits it. A fix that repairs only `conn.go` ships a half-fix.
4. **OWN THE GOROUTINE LIFECYCLE.** A reader goroutine per connection must terminate on close/drain. The
   SPEC must name the shutdown path and the test that proves no leak — this is the axis most likely to be
   under-enumerated.
5. **STATE THE PROOF SHAPE, INCLUDING ITS LIMIT.** The defect is only reachable under CPU contention, so
   the guard cannot be an ordinary unit test. The SPEC must decide whether the row ships a contended
   regression test (and if so, how it stays deterministic in CI) or pins the fix structurally, and it must
   say plainly which. ⚠️ **A green uncontended run is not evidence** — that reading is precisely what
   absorbed this defect as a flake twice.
6. **DECIDE THE DIFFERENTIAL POSTURE.** This is an internal codec-robustness defect with no reference-side
   counterpart to compare against; the SPEC should state whether any differential arm can discriminate it
   at all, and if not, record that as a NAMED departure rather than leaving it implied.
7. **RE-DERIVE EVERY COST AT THE SPEC's OWN TIP.** `reference_measured_prototype_is_a_lower_bound` has
   fired on four consecutive rows, always through UNDER-ENUMERATION. The §6 figure is a floor.
8. **DRAFT ADR-0313 §Context** and re-arm the strict `^> **STATUS: PROPOSED` guard **0 -> 1**. ⚠️ Verify
   the arm **BY LINE AND ADR**, never by the count alone: a historical `^**Status:** PROPOSED` at
   **ADR-0231** (`DECISIONS.md:14866`) also reads 1 and is a decoy.
9. ⚠️ **INCLUDE THE RETAINED ITALIC FOOTER** (`*§Decision and §Consequences follow at the phase-NN IMPL.*`)
   when drafting the ADR. Phase 90's SPEC omitted it and its IMPL had to add the append point its own
   STATUS line already claimed existed.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT RECORDED

Run at stage start on the real file, at `f71cc7e2`. **ACTUAL output, recorded not predicted:**

- **(1)** `want=122` => **SILENT** (all 122 rows `done`, denominator asserted). ⚠️ **The healthy reading on
  an all-`done` board — NOT a reason to create `stop`.**
- **(2)** => **SIX**, at `:200 :206 :212 :222 :228 :236`.
- **(3)** => **SILENT**.

⇒ **ONE check blocks the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** (verified absent at
the git root and in the stage worktree).

All four NCs fired:

- **NC-A** — row-62 doctoring, `NC LANDED? [ in-progress ]` **inspected before trusting the result** =>
  the **ONE-line** `NOT DONE: row 62`, the correct all-`done`-board signature.
- **NC-B** — `want=121` on the real file => `GATE FAIL: examined 122 data rows, expected 121`.
- **NC-C** — check-(3) doctoring, residual **2 -> 0** confirmed first => `NEVER OPENED: gRPC`, WASM
  correctly silent.
- **NC-D** — check-(2) matcher split: long **5** / short **1** / union **6**.

**After registering row 91 the expected reading changes and is recorded here so it is not mistaken for a
regression:** check (1) prints **`NOT DONE: row 91`** with `want=123`, and NC-A returns to its **TWO-line**
form (`row 62` AND `row 91`). Check (2) must still read **SIX** and check (3) must still be **SILENT** —
this row opens no family.

⚠️ **ROW-91 FIELD-COUNT GATE, RUN BEFORE INSTALLING.** `reference_markdown_row_unescaped_pipe_passes_gate`
was reproduced at this tip: a row carrying an unescaped `|` **passes check (1) silently** while the
escape-aware field count catches it at `NF=9`. Row 91 was field-counted to **NF=8** before install.

---

## 9. Findings this stage produced that the next stage must not re-learn

### 9.1 On the defect

1. **Three production call sites, not one** — `conn.go:259`, `conn.go:304`, and ⚠️ **`client.go:307`, the
   UPSTREAM client**, which the standing record omits entirely.
2. **A green uncontended run proves nothing.** 5/5 green uncontended, 6/12 FAIL contended, same binary.
3. **The signature is bimodal**, ~0.07 s or ~10.03 s with nothing between — that is how to tell this hang
   from ordinary slowness, and it is the check phases 85 and 90 did not make.
4. **`f46ba419` changed ZERO production bytes.** Any future reader of the phase-80 "FIXED" claim must know
   that.
5. **The counterfactual is deadline-width**, and it is *not* the fix — it only narrows the window.

### 9.2 On the candidate pool (five of seven follow-ons materially corrected)

6. Arm C's shape decision has **THREE** options; the third (bare close via the `goawaySent` guard) is the
   only reference-shaped one, and **no** shape reaches byte-parity because `Run` writes SETTINGS at
   `conn.go:217` first.
7. **`isValidAuthority` does not exist in any `.go` file** — docs only.
8. **H1-B′'s reference 400 is scoped to ABSENCE**; empty and whitespace-only `Host` are **200**. A
   `req.Host == ""` guard turns one divergence into two. The fix site is `serveOneRequest`, not
   `dispatchRequest`.
9. **H3 arm A does not reproduce** — the stdlib's `reqWriteExcludeHeader` drops `Host` unconditionally.
   There is no defect to fix.
10. **"Option (b)" is never defined anywhere in the tree**, so candidate (5) is unpickable as written.
11. **Candidate (7) is no longer a live divergence** — row 90 closed the value gap; it is an unasserted
    axis. Its machinery (`AccessLogAsserter`, `ReferenceLogMounter`) already exists.
12. ⚠️ **`BEHAVIOR_CONTRACT.md:2040` still asserts the rationale ADR-0312 declares retired.**

### 9.3 On the self-pick inventory itself — THREE inherited claims refuted

13. ⚠️ **"The five terse windows split cleanly on ` + `" is FALSE.** `:212` naively splits to **10**, but
    items 2-3 are one candidate (`upstream SDS (server-cert + validation_context)`) split by a `+` **inside
    parentheses**. True count **9**.
14. ⚠️ **"`:222` is NOT MECHANICALLY COUNTABLE" is FALSE.** Terminating the live sentence at the first
    `**Phase` marker makes it countable: it reads **3** — the dynamic half of the downstream TLS `ssl`
    stat family, the uncounted non-certificate handshake-failure bucket, and tracing
    `spawn_upstream_span`/`http_service`/force-trace.
15. ⚠️ **The "TWO closed candidates in `:222`" warning is MIS-SCOPED.** The `//`-path bug and the
    `/stats/prometheus` gap appear on line 222 only inside **historical narrative**, never in the live
    deferred sentence. **No live window candidate is stale on that ground.**
16. `:236` carries a **third** item the inherited inventory omitted: *an RTDS/SDS validate companion*.
17. ⇒ **Corrected live totals: `:200` 7 · `:206` 10 · `:212` 9 · `:222` 3 · `:228` 8 · `:236` 1 = 38**,
    not the inherited 35 (which excluded `:222` entirely). ⚠️ **Do not quote 88** — that is the naive
    whole-line split of `:222`.

### 9.4 Method

18. ⚠️ **A symbol assertion whose receiver is written `(s *T)` must use `grep -F`, not `-E`** — ERE reads
    the parentheses as a group and returns a **fail-unsafe ZERO**.
19. ⚠️ **`git grep -c 'H2Request'` reads 78 repo-wide vs 21 pathspec-scoped.** State the scope.
20. At **package** scope the anchored and unanchored fail-greps agree at 0 on the h2 package
    (204 RUN / 204 PASS / rc=0), so **agreement at package scope is not evidence the anchored form is
    unnecessary** — the 11-on-green hazard is TREE-scope.

---

## 10. Probe hygiene

- All probes ran from a **private scratch** directory; nothing was written into the worktree except the
  §2.4 counterfactual edit, which was **`sha256sum`-captured before, `git checkout --`-restored after, and
  `sha256sum -c`-verified `OK`**. `git status --porcelain` and `git diff --stat master` both **EMPTY** at
  stage close.
- CPU burners were started and killed **by captured PID**; `pkill -f`/`pgrep -f` were never used, because
  they match the tool call's own shell and exit 144. Burner residue verified **0**.
- The reference probe ran in a container **named** `p91-agentE-ref` and was torn down **BY NAME**. The four
  pre-existing foreign containers (`infallible_booth`, `crazy_kare`, `golink-ai`, `quizzical_goldstine`)
  were recorded before starting and left byte-identical.
- Published probe ports were banded **21091/21092**, below the kernel ephemeral range
  (`net.ipv4.ip_local_port_range = 32768 60999`), and `-p` publishing was used rather than
  `--network host`.
- Test binaries were built with `-o` into scratch, never into the worktree root.
