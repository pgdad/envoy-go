# Phase 91 — `h2-framer-partial-frame-desync` — PROGRESS (IMPL)

Lifecycle-state **3 -> 4**. Base `7dd10924`, branch `phase-91-impl`.

## 0. THE HEADLINE

**The defect is fixed and the fix is PROVEN under the contention that reproduces it.** The HTTP/2 frame
reader no longer wraps `ReadFrame` in a read deadline, so a frame read is never abandoned part-way and
`io.ReadFull` can never discard bytes it has already drained off the socket.

⚠️ **AND THE ROW REFUTED FOUR MORE INHERITED CLAIMS BY EXECUTION — two of which would have produced a
gate that REDS ON A CORRECT FIX or PASSES ON A BROKEN ONE.** The largest is that **h2spec 5.1.2/1 is not
a gate for the ordering guarantee at all**, which cost this row one of its two planned RED anchors and
forced it to BUILD the missing gate rather than inherit one.

## 1. SENTINEL — RUN MECHANICALLY, ACTUAL OUTPUT

**At stage start** (row 91 open): (1) `NOT DONE: row 91` · (2) **SIX** at `:201 :207 :213 :223 :229 :237`
· (3) SILENT. All four NCs fired: NC-A the **TWO-line** form (`row 62` AND `row 91`) with
`NC LANDED? [ in-progress ]` inspected first · NC-B `want=122` => `NOT DONE: row 91` **and**
`GATE FAIL: examined 123 data rows, expected 122` · NC-C residual **2 -> 0** => `NEVER OPENED: gRPC`,
WASM correctly silent · NC-D long **5** / short **1** / union **6**.

**At stage close** (row 91 flipped `done`): (1) **SILENT** · (2) **SIX**, unchanged · (3) SILENT.
NC-A dropped to the **ONE-line** form (`NOT DONE: row 62`) — **the EXPECTED transition, not breakage.**
NC-B now prints `GATE FAIL` only. NC-C and NC-D unchanged.
⇒ **ONE check blocks the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** (verified absent
at the git root and in the stage worktree).

## 2. THE PROOF OF FIX — T2 and T11

The defect is **contention-gated, not code-gated**. Levers, all three, per the BRAINSTORM: **8 CPU burners
pinned to core 0, the test process ALSO pinned to core 0, and `GOMAXPROCS=1`.** Each iteration a separate
process with its `=== RUN` denominator asserted; burners started and killed **by captured PID only**
(`pkill -f`/`pgrep -f` were never used — they match the harness's own shell and exit 144), residue
verified with `kill -0`.

| arm | measurement | FAIL | fast pole `< 1 s` | **third mode `1–9 s`** | hang `> 9 s` |
|---|---|---|---|---|---|
| UNPATCHED | T2, n=36 | **9 / 36** (25.0%) | 27 (0.02–0.05 s) | **0** | 9 (10.00–10.03 s) |
| UNPATCHED | T11, n=32, interleaved | **9 / 32** (28.1%) | 23 (0.01–0.04 s) | **0** | 9 (10.01–10.03 s) |
| **PATCHED** | T11, n=32, interleaved | **0 / 32** | 32 (0.03–0.05 s) | **0** | **0** |

⚠️ **THE ARMS WERE INTERLEAVED (U,P,U,P,…), NOT BLOCKED**, because the base rate is demonstrably
non-stationary (3/12, 6/12, 9/36, 9/32 across this row's history); a block design would confound the fix
with machine drift. Interleaving also proves the machine was **still producing the hang during the very
window the patched arm was sampled** — so this is not an "it stopped reproducing" inconclusive.

⚠️ **The uncontended control is 0.01 s PASS on BOTH arms and proves NOTHING.** It is recorded as a
control, never as a result: that reading is exactly what absorbed this deadlock as a "flake" three times.

Stall points scattered **438–810 of 1024** body bytes across both arms — a race halting at an arbitrary
byte, not a fixed-offset parse bug. The slow pole is the test's own function-local
`frameReadTimeout = 10 * time.Second`, so it is a **HANG, not slowness**. At the unpatched arm's own
measured rate, 32 consecutive clean patched iterations arise by chance with probability ≈ **2.6 × 10⁻⁵**.

## 3. ⚠️ THE LARGEST FINDING — RED ANCHOR 2 IS VOID

**h2spec 5.1.2/1 CANNOT FAIL on the burst-drain ordering guarantee.** Measured, not argued:

- Baseline, unpatched: rc=0, `95 tests, 94 passed, 1 skipped, 0 failed`, `--- SKIP: TestH2Spec` **0**,
  `5.1.2. Stream Concurrency: 1/1 passed`. Reconciles exactly with `CONFORMANCE_PINS.md:142`.
- **Arm A ablation** (`tryReadFrame` -> `return nil, nil`, which removes the burst deferral outright):
  **4 runs out of 4 byte-identical to baseline**, zero failing cases.
- The ablation was **confirmed present in the compiled subject by DISASSEMBLY** — `tryReadFrame` reduced
  to 5 instructions (`XORL/XORL/MOVQ/MOVQ/RET`) versus 63 restored; and on the patched tree the *caller*
  `processFrameAndMaybeDrain` compiled to 32 instructions with **zero** references to `framer.go`, zero
  `NewTimer`, zero `selectgo`/`chanrecv`. **A build was never trusted as evidence the ablation landed.**
- Re-run on the PATCHED tree (T12): same result, per-case result lines **byte-identical**. ⇒ the blindness
  is a property of **the gate**, not of the code shape.

**Mechanism, now understood:** Arm A drops no frames — the outer `readFrameCtx` loop still reads and
dispatches every one. Its only effect is that `flushPendingDispatch()` runs per frame instead of per
burst. Combined with the SPEC's finding that the same ablation leaves the h2 package at 204/204,
**the ordering guarantee was pinned by ZERO tests at ANY layer — while this row rewrites the function that
provides it.**

⇒ **This row BUILT the missing gate** (§5), rather than shipping an unguarded rewrite or recording the
hole as "residual risk". The inherited phrase *"h2spec 5.1.2/1 is the ONLY existing gate"* is **refuted**.

## 4. WHAT LANDED

**Production, 3 files, +300 / −55, one package.** `framer.go` moves the read side onto a dedicated
goroutine (`startReader` / `readerLoop` / `release` / `exitErr` / `closeReader`, `tryReadFrameWait = 2 ms`
as a **timer, never a bare `default:`**). `conn.go` and `client.go` take the **eight** wiring edits — 2
`startReader` + 6 `closeReader` — applied bottom-up under per-site text anchors.

**Asserted BY SYMBOL, never by a successful build**, with `grep -F` for receiver-parenthesised anchors
(ERE reads `(f *framer)` as a group and returns a FAIL-UNSAFE ZERO), plus a fabricated-name negative
control reading 0:

- the six reader-side polling/clearing sites are GONE — `time.Now().Add(`, `time.Time{}`,
  `50 * time.Millisecond`, `os.ErrDeadlineExceeded`, `nerr.Timeout()` all read **0**;
- `defer s.fr.closeReader()` is written **textually AFTER** the `conn.Close` defer (207 -> 211) so **LIFO
  runs it FIRST** — asserted mechanically by comparing line numbers, not by eye;
- `client.go`'s three `NewClientConn` error paths take **NO** edit — asserted by a **zero count** over
  that region.

## 5. THE GATE THIS ROW BUILT — beyond the chartered task list, and why

`TestServerConn_BurstDrainDefersDispatchUntilBurstDrained`: 24 streams at `MaxConcurrentStreams = 4`
delivered in a **single `Write`**, against a **fast** dispatcher, asserting **two independent arms with
`t.Errorf` so neither masks the other** — the refusal COUNT is exactly `streams - max` = 20, and EVERY
refusal precedes EVERY DATA frame. **NC: 8/8 RED, both arms firing.** Under the ablation the full package
shows only the two new guards red while **all 204 pre-existing tests stay green**, independently
reconfirming §3.

It is deliberately unlike `TestServerConn_MaxConcurrentStreamsEnforcement`, which is structurally
incapable here: that test's **blocking** dispatcher makes the ordering an artifact of the fixture, and it
asserts a count rather than an order.

## 6. ⚠️ THE NEGATIVE CONTROLS CAUGHT THREE OF THIS ROW'S OWN NEW TESTS BEING NON-DISCRIMINATING

All three PASSED when written. **Passing was never the question.**

| pin | as drafted | cause | fixed |
|---|---|---|---|
| ordering (first draft) | **0/5 RED** | at `max=2` the first RST is at index 0 regardless — the frame loop out-runs dispatch, so the assertion was structurally unable to fire | re-derived workload; 3 candidate assertions measured over 20 ablated runs (count **20/20**, all-before-all 19/20, first-before-first **11/20 — discarded**) |
| `ReadErrIsSticky` | **0/10 RED** (VACUOUS) | a bare peer close stores `io.EOF`, which is exactly what `exitErr` substitutes when `readErr` is nil, so dropping the stored error still returned `io.EOF` | provoke a real `FRAME_SIZE_ERROR`; assert the error CLASS · **10/10 RED** |
| `CtxErrTakesPrecedence` | **7/20 RED** (COIN FLIP) | with the early return deleted both select arms are simultaneously ready, and Go chooses uniformly at random | 32 independent draws (miss ≈ 2 × 10⁻¹⁰) · **20/20 RED** |

⚠️ **A test that passes is not thereby a guard.** Every one of these would have shipped as coverage it did
not provide.

## 7. THE LEAK GUARD — and a second vacuity caught by measurement

`TestFramer_ReaderGoroutineDoesNotLeak`: baseline captured before any connection, 40 `ServerConn`s each
fully closed via a **local** helper (not `startServerConn`, whose 80 accumulating `t.Cleanup` closures
would hold everything open), asserted with a **poll, never a single sample** — dispatch goroutines outlive
`Run`. `goleak` was **NOT** adopted (`+0 go.mod modules` holds).

⚠️ **THE INHERITED "delta 5 over 40 connections" DOES NOT REPRODUCE — the measured window is 0 versus 40.**
With the fix: delta **0** immediate and 0 after the poll. With `closeReader` gutted: **40**, still 40 after
a 2 s poll. **Slack = 2, set from the NOISE FLOOR and deliberately not from the leak size** — the
precedent's `+8` would have masked the very leak the inherited figure described.

⚠️ **AND THE OBVIOUS GUARD WOULD HAVE BEEN VACUOUS, measured not reasoned.** With a plain
request/response iteration the reader parks in `read(2)`, where `Run`'s own `defer s.conn.Close()`
unblocks it **without** `closeReader` — the ablated build showed delta **0** on every run, i.e. the guard
would have been GREEN WITH THE FIX DELETED. The helper therefore floods PINGs so the server blocks writing
ACKs to a non-reading client, parking the reader in the release-wait state that **only** `close(stopCh)`
reaches. **NC: 5/5 RED.**

## 8. THE INLINE-JOIN DISCLOSURE (T13) — a MEASURED no-op, stated as one

Deleting drain join A (`numstat 0 3`) and, separately, join B (`0 6`) each leaves the package at
**rc=0, 204 RUN / 204 `^ *--- PASS` / 116 `^--- PASS` / 0 anchored FAIL / 0 SKIP** — the baseline exactly.

⇒ **Both `s.fr.closeReader()` drain calls are no-ops at this tip**, because every path reaching a drain has
a reader that already exited. They are **retained deliberately** (they make the socket-exclusivity
invariant local to the drain, and they become live guards if a future change lets a running reader reach
one), and this is recorded as a **measured** fact rather than an unexamined "guard". Nothing here argues
for deleting them.

## 9. D-91-RACE (T14) — and the gate was negative-controlled

`-race` was added at **BOTH** subject-build sites (`numstat 2 2`, `"-race"` count **2**) — one alone
silently instruments only one of the two spawn paths. All four ALPN-`h2` fixtures demonstrably ran with
`--- PASS` (`0004-h2-routing`, `0079-h2-multiplex-pool`, `0080-h2-goaway-rotation`,
`0119-grpc-unary-trailers`), `--- SKIP` **0**, `no tests to run` absent. Anchored
`^panic:|DATA RACE|SIGSEGV` over the COMBINED output (`2>&1`, because the detector reports on the
SUBJECT's stderr) = **0**.

⚠️ **The zero was not trusted on its own:** the `-race` build links **51** `racefuncenter`/`__tsan_*`
symbols (65.1 MB) versus **0** for the plain build (52.7 MB), so the instrumentation is real and the clean
gate is a real clean.

⚠️ **NAMED LIMIT:** four fixtures under normal scheduling is a **SMOKE PROBE, not fleet coverage**, and it
cannot reproduce this row's contended trigger. It claims only that it would catch a *deterministic* race
in the subject process, which the unit `-race` gates cannot see at all.

## 10. ⚠️ AN UNATTRIBUTED ONE-OFF, RECORDED RATHER THAN BURIED

A single top-level failure appeared in one early `-race` full-package run during test development. It was
**not captured**, and it did not reproduce in 35 subsequent full-package `-race` runs, 810 `-race`
executions of the new tests, or 4 further full-package `-race` runs by the controller. It is **not
attributed** to this row and **not dismissed**. Its shape is consistent with the known full-suite startup
flake — **that is a conjecture, and it is labelled as one.**

## 11. DOCUMENTARY DEFECTS — RECORDED, deliberately NOT fixed

**New this stage:** the pre-existing `net.Conn` leak at `client.go`'s three `NewClientConn` error paths —
all three `cancel()` without closing `upstream`, and there is no `upstream.Close()` anywhere in the file.
Out of charter; **not fixed quietly while we were in there.**

**Carried:** `ROADMAP.md` row 91's own `conn.go:217` cite is off by one (`:218` is the
`writeServerInitialSettings` call, `:217` its COMMENT — ⚠️ **an off-by-one landing on a comment reads as a
correct cite**) · `STATE.md` §Project counts SELF-CONTRADICT §Current and carry no label saying so ·
`BEHAVIOR_CONTRACT.md:2040` still carries the D-89-HOST rationale ADR-0312 declares RETIRED · ADR-0312
(xviii) item 4's "12-row CWE-444 suite" is **10** rows landed, item 3 names the wrong fix site, item 6's
H3 arm-A prediction is REFUTED · `ROADMAP.md` rows **57**/**69** malformed (the ARM-A guard, ids
re-verified at this tip) · the phantom gate `git grep -c 'h2.parseHeadersForRequest'` reads **1** (a
COMMENT citation) while `^func.*parseHeadersForRequest` reads **0** · `DECISIONS.md`'s `INNER_EXIT=0` at
phase 87, a value nothing in the tree emits · the xDS cycle guard NOT AUTOMATED.

## 12. THE SIX-GATE POSTURE — measured at `cce00093`; DEPARTURES NAMED, COMPLIANCE NOT CLAIMED

**Gate (a) — full tree `go test ./...`, gated on `PIPESTATUS[0]` and a SET RECONCILIATION** (⚠️ **NOT
`INNER_EXIT`, which does not exist in this repo**). Package count from `go list ./...` = **236**;
reconciled **236 / 236 in BOTH directions** on every launch (`list-not-run` EMPTY, `run-not-list` EMPTY).

⚠️ **THREE LAUNCHES WERE NEEDED, AND BOTH RED RUNS ARE REPORTED — NOT ONLY THE GREEN ONE.**

| run | rc | ok | FAIL | anchored FAIL | panic gate |
|---|---|---|---|---|---|
| 1 | **1** | 125 | 2 | 8 | 1 |
| 2 | **1** | 126 | 1 | 4 | 1 |
| 3 | **0** | 127 | 0 | **0** | **0** |

Both failures are **known-register flakes, off this row's axis**, and each was chased rather than waved at:
- `internal/boot` — `TestSDSEndToEnd_FetchFailure_BootFailsClosed`: boot **did** fail closed; only the
  message wording missed the expected phrase. Green isolated (RUN=3, 3 PASS), green as a full package
  (40/40), green in runs 2 and 3. This is the live SDS dial-budget flake in the register.
- `test/differential` fixture `0082-grpc-access-log-buffering` — `panic: ... bind: address already in use`
  on the driver-owned ALS receiver, at ports **46387** and **32991**, both inside the ephemeral band
  **32768-60999**. ⚠️ **This panic ABORTS THE WHOLE TEST BINARY, so every differential fixture after 0082
  never executed and the tree-scope differential census in those runs is WORTHLESS** — which is exactly
  why gate (b) is run standalone. The driver was last touched by an unrelated module-path rename; it is
  **not** this row's code.

**Gate (b) — the differential suite standalone:** rc=0, **121** `--- PASS: TestDifferential/`, `--- SKIP`
**0**, anchored FAIL 0. ⚠️ **THE FIXTURE SET WAS RECONCILED IN BOTH DIRECTIONS** against
`ls -d test/fixtures/[0-9]*/` — 121 vs 121, both difference sets EMPTY. The silent-`t.Skipf` failure mode
is therefore excluded **by set identity, not by a green rc**.
**Gate (e) — the anchored panic gate `^panic:|DATA RACE|SIGSEGV` over the combined output: 0.**

**D-91-RACE (b) — `-race` at the three in-process packages, every figure exact:**

| package | rc | RUN | `^ *--- PASS` | `^--- PASS` | FAIL | SKIP | DATA RACE |
|---|---|---|---|---|---|---|---|
| `./internal/filter/hcm/h2/` | 0 | **211** | **211** | **123** | 0 | 0 | **0** |
| `./internal/cluster/` | 0 | **458** | **458** | **404** | 0 | 0 | **0** |
| `./internal/filter/hcm/` | 0 | **322** | **322** | **226** | 0 | 0 | **0** |

The known `./internal/cluster/` outlier flake did not fire.

**Gate (c):** h2spec **NOT re-run at this tip — INHERITED on a byte-identity proof.** The tree-wide diff
between `3a0e8c23` (where it ran) and `cce00093` is exactly two ADDED `_test.go` files; **zero production
bytes differ anywhere.** Cited figure: rc=0, `95 tests, 94 passed, 1 skipped, 0 failed`,
`--- SKIP: TestH2Spec` **0**. ⚠️ **And per §3, h2spec must NOT be cited as a gate for burst-drain
ordering.** grpc-conformance: **DEPARTURE, deferred in writing.** proxy-wasm **10/16: INHERITED**, not
measured here.

**Gate (d) — fuzzers: 55 / 48 files**, `*.go`-scoped under `internal/` (the axis is stated; repo-wide
doc-contaminated counts differ and are not quoted).

**Gate (f) — DEPARTURE, NAMED: there is no `REVIEW.md` for phase 91.** `git ls-files | grep -c
'REVIEW\.md$'` => **37**, a FILE count and the pre-existing corpus. **No compliance is claimed.**

**Envelope — +0 on every axis:** fixtures **121** · `0120` still unconsumed · `go.mod`/`go.sum` diff
**EMPTY** · BackendKind tail **`H2GoawayResponder`** ⚠️ (**derived with a DIGIT-AWARE identifier class — a
naive `[A-Za-z]+` returns `GoawayResponder`, silently dropping the `H2`; the control was run and is
recorded**) · phase dirs **132** · ADR tail **0313**, next-free **0314**, `^---$` **216** · `ROADMAP.md`
**241 lines / 123 data rows** · `STATE.md` **63** · `STATE_HISTORY.md` **522**, strict guard **163
DELTA 0**. Docker `ps -a` delta **0**; the four foreign containers untouched throughout.

⚠️ **DEPARTURE WORTH CARRYING: a single `go test ./...` is NOT reliably green on this machine at tree
scope. The run-3 green does not erase runs 1 and 2**, and the `0082` bind race is a live tree-scope hazard
that silently voids a differential census when it fires.
