# Phase 91 — `h2-framer-partial-frame-desync` — PLAN

**Stage:** PLAN (lifecycle-state 2 -> 3)
**Base master:** `c735ffdb`
**Branch:** `phase-91-plan`
**Date:** 2026-08-23
**Row:** 91, stays `in-progress` at `ROADMAP.md:153`; `want` stays **123** (a PLAN changes neither — verified by EMPTY DIFF)
**ADR:** **ADR-0313** — §Context was drafted at the SPEC and is BYTE-UNTOUCHED here; the strict
`^> **STATUS: PROPOSED` guard STAYS ARMED at **1**. A PLAN adds no ADR; next-free STAYS **ADR-0314**.

---

## 0. THE HEADLINE — this PLAN refutes SEVEN of its SPEC's own cites, and DISCHARGES one of its design arguments

The SPEC's six DECISIONS (D-91-SHAPE / ALIAS / LAZY / GAP1 / DIFF / PROOF) are carried forward unchanged
and are NOT re-opened. What this stage adds is the concrete code, and what it found while writing it:

1. ⚠️ **SEVEN SPEC cites are WRONG at this tip** (§2). Two of them would have mis-positioned the very edit
   the SPEC mandates; one names a file **that does not exist**.
2. ⚠️ **SPEC §5.5 A2 — "the reader steals the drain's bytes" — is DISCHARGED, not merely handled** (§3.6,
   §4.4). Under the reader design frozen here the reader **exits on its first read error and closes the
   frame channel before the consumer can observe that error**, so *every* path that reaches a drain site
   has a reader that is already gone. The two inline joins are retained as a structural invariant, and
   this PLAN says plainly that they are **no-ops at this tip** rather than implying they do work they do
   not. `reference_review_mandated_guard_is_untested` is the shape being avoided: an inline call that
   looks load-bearing and is not.
3. ⚠️ **`closeReader` is signal + STAMP + join, not signal-and-join.** The SPEC's §5.4 forbids a deadline
   stamp; §5.5 A1 simultaneously requires unblocking a reader **parked in a blocking `read(2)`**, which no
   channel close can reach. The resolution (§3.6): the stamp is **one of three** unblock mechanisms and is
   safe **only because this fix deletes every reader-side deadline clear**, making `closeReader` and the
   two drain sites the *only* writers of the read deadline. The SPEC's objection was to a stamp **beside**
   the old clears; it does not survive their deletion.
4. ⚠️ **One of the SPEC's SEVEN wiring sites takes NO `closeReader` edit** (§4.3), derived rather than
   assumed, because of where `startReader()` has to go. The site's *other* problem — a leaked `net.Conn` —
   is PRE-EXISTING, out of charter, and RECORDED (§9.3).
5. **The split gate is EVALUATED, not assumed: 16 tasks, ~+520 LoC net code. NOT TRIPPED** (§10.3).
6. **D-91-RACE is DECIDED** (§7), and the decision is neither "yes" nor "no": the full suite runs WITHOUT
   `-race` because the code this row changes runs in a **subprocess built without it**, and a bounded,
   race-instrumented **subject** probe over the four H2-capable fixtures is MANDATED in its place.

---

## 1. SENTINEL — RUN MECHANICALLY AT THIS TIP, ACTUAL OUTPUT RECORDED

Run at stage start on the real `docs/envoy-go/ROADMAP.md` at `c735ffdb`. `ROADMAP.md` is BYTE-UNTOUCHED
this stage, so the close-of-stage run is identical and is not separately transcribed.

### 1.1 The three checks — ACTUAL output, recorded not predicted

- **(1)** `want=123` => **`NOT DONE: row 91`** — ⚠️ **the HEALTHY reading while row 91 is open.**
- **(2)** => **SIX**, at `:201 :207 :213 :223 :229 :237`.
- **(3)** => **SILENT**.

⇒ **TWO checks block the sentinel. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent
at the git root AND in the stage worktree (`ls` returned `No such file or directory` for both).

⚠️ **Do NOT "fix" the six.** History is `0 -> 1 -> 3 -> 4 -> 5 -> 6` across ~40 phases; the candidate
sentences are a WINDOW onto a larger deferred backlog (~59), not an inventory of it. Cite **~42** only as
"sentence-visible", never as the inventory.

### 1.2 All four NCs FIRED — ACTUAL output

- **NC-A** — row-62 doctoring. `NC LANDED? [ in-progress ]` **inspected BEFORE trusting the result** =>
  the **TWO-line** form: `NOT DONE: row 62` AND `NOT DONE: row 91`. Correct for an open board; **on an
  all-`done` board this reads ONE line** — do not carry the two-line expectation onto a closed board.
- **NC-B** — `want=122` on the real file => `NOT DONE: row 91` AND
  `GATE FAIL: examined 123 data rows, expected 122`.
- **NC-C** — check-(3) doctoring. Residual `gRPC-family row` **2 -> 0** confirmed FIRST, then
  => `NEVER OPENED: gRPC   <- NC FIRED`; WASM correctly silent.
- **NC-D** — check-(2) matcher split: long **5** / short **1** / union **6**.

⚠️ **The ARM-A escape-aware malformed-row guard reads 2, NOT 0** — rows **57** (`NF=9`, line 119) and
**69** (`NF=10`, line 131), both pre-existing. The NAIVE form reads **17** and is NOT a drift signal.
Row 91 field-counts to **NF=8 under BOTH forms**. A gate asserting `== 0` FAILS on pre-existing content.

⚠️ **A METHOD WARNING RE-LEARNED AT THIS STAGE** (a measurement agent hit it first): substituting a
"reasonable equivalent" for check (1)'s field-parsed awk — e.g. `NF>=8 && $2 ~ /^ *[0-9]+ *$/` — prints
**92**, not 123, because the naive `-F'|'` split of escaped-pipe rows moves `NF` and the numeric `$2`
anchor then rejects rows the canonical form accepts. **Use the `next-prompt.txt` form verbatim.**

---

## 2. ⚠️ SEVEN SPEC CITES REFUTED BY EXECUTION AT THIS TIP

Zero `.go` bytes changed between the SPEC's base `870fc90b` and this tip (`git diff --name-only
870fc90b..c735ffdb -- '*.go'` => **0 files**), so these are not drift — they were wrong when written.
Every one was re-derived here, not restated (`feedback_brief_citations_not_evidence`,
`reference_verification_table_launders_wrong_cites`).

| # | SPEC cite | Verdict at `c735ffdb` |
|---|---|---|
| C1 | `conn.go:276-278` — the `Run` drain block | **WRONG.** The block is **276-281**: `if` 276, `emitGoaway` **277**, `SetReadDeadline(+500ms)` **278**, `io.Copy` **279**, clear **280**, `}` 281. As cited it **stops before the `io.Copy` drain it names.** |
| C2 | `conn.go:311-314` — the `processFrameAndMaybeDrain` drain block | **WRONG.** The block is **311-317**: `if` 311, `flushPendingDispatch()` **312**, `emitGoaway` **313**, `SetReadDeadline(+500ms)` **314**, `io.Copy` **315**, clear **316**, `}` 317. ⚠️ **The SPEC CONTRADICTS ITSELF** — its §5.3 table says `311-314` while its §5.5 A2 says `311-317`. ⚠️ **And this block has a FIFTH statement the other lacks** — `flushPendingDispatch()` at **312**, *before* `emitGoaway` — which is directly load-bearing for join ordering (§4.4). |
| C3 | `frame.go:545` — `checkFrameOrder` | **WRONG, off by 2.** The declaration is **`frame.go:543`**; `:545` is the `fr.lastFrame = f` assignment inside it. |
| C4 | `framer_test.go:330` — "the one existing `readFrameCtx` test" | **WRONG as a test cite.** `:330` is the *call site*. The test is **`TestFramer_ReadFrameCtxCancel`**, declared at **`framer_test.go:320`**. A task told to "edit the test at `:330`" edits the middle of a body. |
| C5 | SPEC §4.5 — `frame.go:497-500` "invalidates **unconditionally**" | **OVERSTATED.** `frame.go:499` is guarded by `if fr.lastFrame != nil` at **498**. The parity argument survives (the guard is nil-only, and `lastFrame` is assigned by `checkFrameOrder` at `frame.go:545`), but "unconditionally" is not what the code says. **The ADR must not inherit the word** (`reference_brainstorm_adjective_acquires_adr_authority`). |
| C6 | SPEC §8 item 3 — `internal/cluster/hedge_test.go:353`, the third `pollUntil` | **THE FILE DOES NOT EXIST.** `ls` => `No such file or directory`. The true path is **`internal/filter/http/router/hedge_test.go:353`** — right line, wrong directory, and a different *package*. A task pointed at the SPEC's path gets a file-not-found. |
| C7 | SPEC §4.1 — `frame.go` exports "exactly **TWO** producing functions" | **TRUE ONLY FOR PACKAGE-LEVEL `func`s.** `/usr/bin/grep -n '^func [A-Z]' frame.go` => exactly **2** (`:230 ReadFrameHeader`, `:430 NewFramer`). But **`(*Framer).ReadFrame` at `frame.go:496` is an exported METHOD producing a `Frame`.** ⚠️ **The D-91-ALIAS conclusion is UNHARMED** — `ReadFrame` mints frames from the *wire*, never from a caller-supplied `[]byte`, so a copied payload still cannot be re-minted as an `http2.Frame`. **But the sentence must carry the qualifier**, or a future reader who finds `:496` will believe the whole argument was sloppy. |

⚠️ **An eighth item is a measurement axis, not a cite, and it is a LIVE FOOTGUN for every gate below.**
On the h2 package's `-v` output `grep -c '^--- PASS'` reads **116**, not 204; **204 is the
including-subtests axis** (`grep -c '^ *--- PASS'`). 116 top-level tests, 88 subtests. On
`./internal/filter/hcm/` the pair is **226 / 322**. **A gate that writes the anchored `^--- PASS` form and
asserts 204 REDS on an unmodified tree.** Every gate in §11 states its axis.

⚠️ **A ninth: the SPEC's contended fast pole "~0.07 s" does not reproduce here.** Five uncontended
isolated runs at this tip read `--- PASS: TestServerConn_TinyWindowDelivery (0.01s)` **5/5**; in-package
it also reads `0.01s`, and under `-race` `0.02s`. The slow pole is unaffected and unmistakable (the next
slowest test in the whole package is `0.50 s`). §6.5 encodes **0.01 s**, not 0.07 s, and classifies by
threshold rather than by a remembered constant.

---

## 3. THE FROZEN DESIGN — `internal/filter/hcm/h2/framer.go`

This is the design the IMPL implements. It is written out so the IMPL does not re-derive it, and so a
reviewer can diff the landed code against a written intent. Line cites are at `c735ffdb`.

### 3.1 The struct, and the ONE-CONSUMER precondition

```go
type framer struct {
	*http2.Framer
	conn net.Conn

	// Reader-goroutine plumbing. All four channels are allocated by newFramer;
	// only the GOROUTINE is lazy (§3.2). A framer built as a composite literal
	// (framer_writeheaderblock_test.go:235) therefore has nil channels, which
	// the read methods detect and reject loudly rather than blocking forever.
	frameCh   chan http2.Frame // UNBUFFERED — D-91-ALIAS. Capacity is pinned by a test (§6.2).
	releaseCh chan struct{}    // capacity 1; carries a TOKEN, never a frame
	stopCh    chan struct{}    // closed by closeReader
	doneCh    chan struct{}    // closed by the reader on exit

	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool

	// readErr is written by the reader goroutine BEFORE it closes frameCh, and
	// read by the consumer ONLY after a receive on frameCh has reported the
	// channel closed. The close/receive pair is the happens-before edge.
	readErr error

	// held is touched ONLY by the single consumer goroutine and is deliberately
	// unsynchronized. It is true exactly while the consumer owns a frame the
	// reader has not been released from.
	held bool
}
```

⚠️ **PRECONDITION, STATED BECAUSE IT IS LOAD-BEARING AND UNENFORCED BY THE TYPE SYSTEM: a framer's READ
side has exactly ONE consumer goroutine.** Server side that is `(*ServerConn).Run` (`conn.go:206`) —
`processFrameAndMaybeDrain` (`conn.go:296`) runs on the same goroutine. Client side it is
`(*ClientConn).readLoop` (`client.go:305`). `held` must never become shared. **The `-race` gates of §11
are what enforce this**; a reviewer should treat any second read-side caller as a design break.

⚠️ `sync.Once` and `atomic.Bool` make `framer` non-copyable. Every use in the tree is already `*framer`
(`git grep '&framer{'` => exactly **2**: `framer.go:117`, `framer_writeheaderblock_test.go:235`), so
`go vet`'s copylocks has nothing to bite — but the IMPL must confirm `go vet ./internal/filter/hcm/...`
is clean rather than assume it.

### 3.2 `newFramer` allocates; `startReader` spawns — D-91-LAZY

`newFramer` (`framer.go:114`) gains the four channel allocations and nothing else. **The goroutine is
spawned by an explicit `startReader()` seam**, because four things read the socket *directly* before any
framer read (SPEC §5.2, all four re-verified at this tip):

1. `settings.go:121` — `readClientSettings` calls the **embedded** `fr.ReadFrame()`, on BOTH sides
   (server `conn.go:225`, client `client.go:285`).
2. `conn.go:212` — `readClientPreface(s.conn)` reads the socket directly.
3. `conn.go:279` / `conn.go:315` — `io.Copy(io.Discard, s.conn)` reads the socket directly.
4. `continuation_test.go:644` — `contIdleServerConn` builds a `ServerConn` its own comment says is
   **NEVER Run**; **11** test call sites hold a `*framer` and call `ReadFrame` directly
   (`framer_test.go` 28, 59, 78, 111, 150, 172, 208, 230, 252 + `settings_test.go` 40, 65 — count
   re-derived at this tip, **11 exactly**); and `framer_writeheaderblock_test.go:235` constructs
   `&framer{Framer: http2.NewFramer(&buf, nil)}` with the `conn` field **omitted** (nil).

```go
// startReader spawns the frame-reader goroutine. It MUST be called exactly once
// per framer, from the connection's own setup goroutine, AFTER every direct
// socket read (preface, initial SETTINGS) and BEFORE the first readFrameCtx or
// tryReadFrame. Idempotent; a framer whose reader was never started is still
// safe to closeReader.
func (f *framer) startReader() {
	f.startOnce.Do(func() {
		f.started.Store(true)
		go f.readerLoop()
	})
}
```

⚠️ **THE NAME IS `readerLoop`, NOT `readLoop`.** `client.go:305` already declares
`func (cc *ClientConn) readLoop()`. Two different loops with the same name in one package is how a
reviewer misreads a defer.

### 3.3 The reader loop — `ReadFrame` with NO deadline, ever

```go
func (f *framer) readerLoop() {
	defer close(f.doneCh)
	for {
		// NO deadline. This is the whole row: a frame read is never abandoned
		// part-way, so io.ReadFull can never discard bytes it already drained.
		frame, err := f.Framer.ReadFrame()
		if err != nil {
			f.readErr = err
			close(f.frameCh) // consumer sees !ok and reads readErr (§3.7)
			return
		}
		select {
		case f.frameCh <- frame: // parked state (ii)
		case <-f.stopCh:
			return
		}
		// The consumer now OWNS `frame`. Do not re-enter ReadFrame until it is
		// released: frame.go:498-499 invalidates the previous frame at the ENTRY
		// of the next ReadFrame (guarded only by `lastFrame != nil` — C5).
		select {
		case <-f.releaseCh: // parked state (iii)
		case <-f.stopCh:
			return
		}
	}
}
```

### 3.4 `readFrameCtx` — and BOTH release-placement caveats, written out

```go
func (f *framer) readFrameCtx(ctx context.Context) (http2.Frame, error) {
	if f.frameCh == nil {
		return nil, errReaderNotStarted
	}
	// ⚠️ CAVEAT 1 (SPEC §5.6.1): the ctx-error early return comes BEFORE the
	// release. Today framer.go:167-169 returns on ctx.Err() WITHOUT reaching the
	// ReadFrame at :171, so a ctx-cancelled read does NOT invalidate frame N.
	// Releasing first would invalidate where today's code does not — a behaviour
	// change smuggled in as a refactor.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.release()
	select {
	case frame, ok := <-f.frameCh:
		if !ok {
			return nil, translateFramerErr(f.exitErr())
		}
		f.held = true
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
```

⚠️ **CAVEAT 2 (SPEC §5.6.2) — one release per consumer call, NEVER one per retry — is DISCHARGED TWICE
OVER, and the PLAN says so rather than leaving it to discipline:**
- **Structurally:** the retry loop is DELETED. The old `for { ... continue }` (`framer.go:165-193`) that
  re-entered `ReadFrame` on every 50 ms timeout does not exist in the new body. There is no retry to
  attach a second release to.
- **By invariant:** `release()` is a no-op unless `held` is true, and `held` is set only by a successful
  receive and cleared by `release()` itself. **Outstanding release tokens are bounded to exactly one**,
  which is also why `releaseCh` can be capacity-1 and `release()` can never block.

```go
// release signals the reader that the frame it last handed over is finished
// with. Called at the START of each consumer read call — exact parity with
// frame.go:498-499's invalidate-at-entry-of-the-next-ReadFrame (SPEC §4.5).
func (f *framer) release() {
	if !f.held {
		return
	}
	f.held = false
	f.releaseCh <- struct{}{} // cap 1; `held` bounds outstanding tokens to 1
}

// exitErr reports why the reader stopped. It substitutes io.EOF for a nil
// readErr so a consumer can never receive (nil, nil) from a CLOSED channel and
// then dereference a nil Frame. The reader never closes frameCh with a nil
// readErr; this is a fail-CLOSED guard, not a live path.
func (f *framer) exitErr() error {
	if f.readErr == nil {
		return io.EOF
	}
	return f.readErr
}
```

⚠️ **`translateFramerErr` is applied on EVERY consumer read, not once at store time.** `connError` /
`streamError` allocate a fresh `*Error` per call, so each caller gets its own value — byte-for-byte the
allocation shape callers have today. Storing a pre-translated `*Error` would share one value across every
subsequent read.

### 3.5 `tryReadFrame` — D-91-WAIT: the bounded wait is **2 ms**

```go
// tryReadFrameWait bounds how long the burst-drain waits for the reader to
// produce the next frame before declaring the burst exhausted. It replaces the
// old 1 ms SOCKET read deadline at framer.go:202.
//
//   > 1 ms  — the new shape interposes a goroutine handoff the old inline read
//             did not have; 1 ms was sufficient when the drain did the read
//             itself and is not obviously sufficient when a wake-up precedes it.
//   < 5 ms  — conn_test.go:1201 drips WINDOW_UPDATE(1,16) every 5 ms. A wait at
//             or above that interval lets grants COALESCE, which is exactly the
//             artifact the SPEC's arm B measured (a legitimate single 32-byte
//             frame reddening WriteData_RespectsPerStreamSendWindow).
//   << 50 ms — arm B measured a 50 ms wait inflating this package's suite from
//             1.76 s to 42.4 s.
const tryReadFrameWait = 2 * time.Millisecond

func (f *framer) tryReadFrame() (http2.Frame, error) {
	if f.frameCh == nil {
		return nil, errReaderNotStarted
	}
	f.release()
	t := time.NewTimer(tryReadFrameWait)
	defer t.Stop()
	select {
	case frame, ok := <-f.frameCh:
		if !ok {
			return nil, translateFramerErr(f.exitErr())
		}
		f.held = true
		return frame, nil
	case <-t.C:
		return nil, nil // burst exhausted — the ONLY (nil, nil) return
	}
}
```

⚠️ **NOT A BARE `default:` (SPEC §6.1).** A non-blocking select returns before the reader can be
scheduled and silently guts the burst-drain that `conn.go:242-251` says h2spec 5.1.2/1 depends on.
⚠️ **A fast-path `select { case <-f.frameCh: ...; default: }` placed BEFORE the timed wait is PERMITTED**
(it avoids a timer allocation when a frame is already pending) **but the `default:` arm may NEVER be the
only arm.** Take it only if §11's wall-time gate shows a regression; the simple form is the default.

⚠️ **ESCAPE HATCH, NAMED SO IT IS NOT A SILENT RETUNE.** If h2spec 5.1.2/1 (§6.6) or the contended arm
(§6.5) shows 2 ms is short, the IMPL MAY raise it to **at most 4 ms** (strictly below the 5 ms drip),
and MUST re-run BOTH gates and record the change and its reason in `PROGRESS.md`. Any value ≥ 5 ms is a
**decision reversal** and requires an ADR amendment, not a constant edit.

### 3.6 `closeReader` — signal + STAMP + join, over THREE parked states

```go
// aLongTimeAgo is a non-zero time far in the past, used to unblock a reader
// parked in a blocking read(2). Mirrors the x/net/http2 idiom.
var aLongTimeAgo = time.Unix(1, 0)

// closeReader stops the reader goroutine and JOINS it. Idempotent, and safe on
// a framer whose reader was never started or whose channels were never
// allocated (a composite literal).
//
// AFTER closeReader RETURNS, THE READER GOROUTINE IS GONE. That is the property
// the two drain sites and the leak guard both depend on.
func (f *framer) closeReader() {
	if f.stopCh == nil {
		return // composite-literal framer; there is no reader
	}
	f.stopOnce.Do(func() {
		// (ii) blocked SENDING on frameCh and (iii) blocked WAITING FOR RELEASE
		// are both released by this close.
		close(f.stopCh)
		// (i) blocked in ReadFrame on the socket is NOT reachable by any channel
		// close. Stamping a past read deadline is the only thing that unblocks a
		// blocking read(2) without closing a conn the framer does not own.
		if f.conn != nil {
			_ = f.conn.SetReadDeadline(aLongTimeAgo)
		}
	})
	if f.started.Load() {
		<-f.doneCh
	}
}
```

⚠️ **WHY THE STAMP IS NOT THE THING SPEC §5.4 FORBIDS.** §5.4 rejects a `closeReader` that *merely*
stamps, on two grounds, and this PLAN answers both **by construction rather than by argument**:

- *"a stamp races the reader's own deadline clears (`framer.go:173/185/191`)"* — **those clears are
  DELETED by this fix.** `framer.go` carries exactly **six** `SetReadDeadline` sites today (170, 173,
  185, 191, 202, 204 — re-derived at this tip, the set is complete); the new `framer.go` has **ZERO**.
  After the fix the read deadline has exactly **three** writers tree-wide: `closeReader`, and the two
  drain sites `conn.go:278`/`:314` with their paired clears at `:280`/`:316`. **There is nothing left to
  race.** The objection was to a stamp *beside* the clears and does not survive their removal.
- *"an un-joined reader clearing the deadline turns the bounded 500 ms drains into unbounded blocking
  reads"* — **the join is what forecloses this**, and it is why closeReader is placed **before** the
  drain arms its own deadline (§4.4), never after.

⚠️ **The stamp is deliberately NOT cleared by `closeReader`.** Both drain sites overwrite it with their
own `+500 ms` immediately afterwards, and after the join no reader exists to be affected. Clearing it
would re-open exactly the unbounded-read hazard §5.4 names.

### 3.7 D-91-ERR — `readErr` stickiness is NEW behaviour, and it is the CORRECT behaviour

**Today:** a genuine read error is returned by `readFrameCtx`; a subsequent call re-enters `ReadFrame` and
gets the same error again from the socket. Stickiness is incidental and comes from the OS.

**After:** the reader exits on its first error, stores it, and closes `frameCh`. Every subsequent consumer
read returns that same error **immediately, without touching the socket**.

**DECISION: adopt the stickiness, and TEST it** (§6.3). Three grounds:
1. It is strictly more deterministic than today and removes a per-error syscall.
2. It is what makes SPEC §5.5 A2 discharge (§4.4): the reader is provably gone by the time any consumer
   observes an error.
3. ⚠️ It changes one observable: an error is now returned even when `ctx` is *also* cancelled and the
   consumer would previously have raced. `readFrameCtx` checks `ctx.Err()` FIRST (§3.4), so a cancelled
   context still wins — **the precedence is unchanged**, and §6.3 pins it.

---

## 4. THE WIRING — EIGHT EDITS ACROSS TWO FILES

### 4.1 The map, against the SPEC's SEVEN sites

| SPEC site | file | edit | note |
|---|---|---|---|
| 1 | `conn.go:207` | `defer s.fr.closeReader()` placed **textually AFTER** `:207` | Go defers are LIFO — to RUN before `s.conn.Close()`, it must be WRITTEN after it. The record's *"ordered before"* is wrong as source order. |
| — | `conn.go` after `:238` | **`s.fr.startReader()`** | NEW; not in the SPEC's list. See §4.2. |
| 2 | `conn.go:277`↔`:278` | `s.fr.closeReader()` between `emitGoaway` and the 500 ms arm | Block is **276-281** (C1), not 276-278. |
| 3 | `conn.go:313`↔`:314` | `s.fr.closeReader()` between `emitGoaway` and the 500 ms arm | Block is **311-317** (C2), not 311-314; note `flushPendingDispatch()` at **312** precedes `emitGoaway`. |
| 4 | `client.go:306` | `defer cc.fr.closeReader()` as the FIRST statement of `readLoop` | `readLoop` has **ZERO** defers today — an ADDITION, not an amendment. |
| 5 | `client.go:250,255,260` | **NO EDIT** | Derived, not assumed — see §4.3. |
| — | `client.go` before `:266` | **`cc.fr.startReader()`** | NEW; must precede `go cc.readLoop()`. See §4.2. |
| 6 | `client.go:272` | `cc.fr.closeReader()` after `cc.cancel()` | The caller receives `nil` and can NEVER call `Close()`; without this the reader is orphaned. |
| 7 | `client.go:1009`↔`:1010` | `cc.fr.closeReader()` between `cc.cancel()` and `cc.conn.Close()` | Gives `Close` the deterministic contract the leak guard asserts: **Close returned ⇒ reader gone**. |

**Eight edits: six `closeReader`, two `startReader`.**

### 4.2 Where `startReader()` goes, and why it cannot go earlier or later

- **Server (`conn.go`)** — immediately after Step 4's `WriteSettingsAck()` (the `}` at `:238`) and before
  the dispatch-loop comment at `:240`. It cannot go earlier: Step 1 `readClientPreface` (`:212`) and
  Step 3 `readClientSettings` (`:225` -> `settings.go:121`) both read the socket directly. It cannot go
  later: `:259` is the first `readFrameCtx`.
- **Client (`client.go`)** — after `readPeerSettingsAndAck()` (`:259`, which routes to the same bare
  `settings.go:121` read) and immediately before `go cc.readLoop()` at `:266`.

### 4.3 ⚠️ WHY SPEC SITE 5 TAKES NO `closeReader` — derived, and the residue RECORDED not smuggled

`client.go:250 / :255 / :260` are the three `cancel()`-and-return error paths of `NewClientConn`. All
three execute **before** `startReader()` (§4.2). **No reader exists to tear down**, so a `closeReader`
there would be dead code — and `closeReader` is already a safe no-op on an unstarted framer anyway.

⚠️ **THIS IS A PLACEMENT-DEPENDENT CONCLUSION, AND IT IS STATED AS ONE.** If the IMPL moves
`startReader()` earlier than §4.2 prescribes, all three sites acquire a real edit. The IMPL must not move
it; if it must, it re-derives this row.

⚠️ **The SPEC's actual complaint about these three sites survives and is NOT this row's to fix.** All
three `cancel()` without closing `upstream` — `git grep -n 'upstream' client.go` shows **no**
`upstream.Close()` anywhere in the file. That is a **pre-existing `net.Conn` leak**, unrelated to frame
reading, out of this row's charter, and recorded in §9.3 as a documentary defect. **It is not "fixed
quietly while we are in there."**

### 4.4 ⚠️ THE JOIN ORDERING — and the honest statement that both inline joins are NO-OPS at this tip

SPEC §5.5 A2 requires the join to land **between the release and the drain**, on the reasoning that the
reader would otherwise be "re-entering `ReadFrame` on the socket at the exact moment the drain wants it
exclusively", stealing the drain's bytes and clearing its 500 ms bound.

**Under the design frozen in §3 that scenario cannot arise, and the PLAN says so.** Enumerate every way a
consumer observes an error:

- `readFrameCtx` returns exactly four things: `ctx.Err()` early (§3.4), a frame, `exitErr()` on a
  **closed** `frameCh`, or `ctx.Err()` from `<-ctx.Done()`.
- `tryReadFrame` returns exactly three: a frame, `exitErr()` on a **closed** `frameCh`, or `(nil, nil)`.

A closed `frameCh` means the reader has **already stored its error and returned**. And the drain at
`conn.go:276-281` is guarded by `hErr != nil && hErr.Stream == 0` — reachable only from a translated
`*Error`, i.e. only from the closed-channel path; the `ctx.Err()` paths return earlier at `conn.go:261-264`
without draining. Same at `conn.go:311-317`.

⇒ **Every path that reaches a drain site has a reader that is already gone, so `<-f.doneCh` returns
immediately and the two inline joins are NO-OPS at this tip.**

**They are still MANDATORY, for two reasons stated plainly:**
1. They make the invariant *local*. A reviewer at `conn.go:314` can see that the drain owns the socket
   exclusively without having to re-derive the error taxonomy above.
2. They are the only thing that holds if a future change lets a live reader reach a drain — and the
   failure they would then prevent is an **unbounded blocking read in the code this row exists to
   de-hang**.

⚠️ **AND THE PLAN REFUSES TO CLAIM THEY DO WORK THEY DO NOT.** Naming a guard "load-bearing" when
deleting it changes nothing is precisely `reference_review_mandated_guard_is_untested`. **§6.7 therefore
requires the IMPL to record that deleting either inline join leaves the suite GREEN** — a stated,
measured no-op rather than an unexamined one.

### 4.5 ⚠️ ONE EXISTING TEST BREAKS, BY DESIGN

`TestFramer_ReadFrameCtxCancel` (`framer_test.go:320`, the call at `:330` — C4) calls `readFrameCtx` on a
framer whose reader was **never started**. Under §3.4's guard it now returns `errReaderNotStarted`
instead of `ctx.Err()`. **The test must gain a `startReader()` call** (and a `defer f.closeReader()`).
This is a **+2/−0 test edit**, and it is listed as its own task (T6) rather than absorbed into another,
because a silently-adjusted assertion is how a real regression gets laundered.

The other **11** direct-`ReadFrame` test call sites (§3.2 item 4) use the **embedded** `http2.Framer`
method and are unaffected. `conn_test.go` and `continuation_test.go` drive real `ServerConn`s through
`Run`, so `startReader()` happens inside `Run` — also unaffected.

---

## 5. THE ORDERED IMPL TASKS — **SIXTEEN**

⚠️ **A NOTE ON TDD, STATED HONESTLY RATHER THAN PERFORMED.** The house pattern (phases 88-90) opens with a
RED census of tests that **compile against the unmodified tip**. That is impossible here: every unit
assertion this row needs names a symbol the row itself creates (`startReader`, `closeReader`, `frameCh`),
so a "RED census" would be a **build break**, not an assertion failure — and a build break proves nothing
(`reference_config_counterfactual_is_not_implementation_counterfactual`).

⇒ **This row's RED anchors are T2 and T3, and they are both recorded BEFORE any production byte moves:**
the **contended reproduction** (`TestServerConn_TinyWindowDelivery`, measured **6 FAIL / 12** at the
BRAINSTORM) and the **h2spec 5.1.2/1 reddening negative control**. Both discriminate on the unmodified
tree. The unit tests of T9/T10 are written test-first *within* their tasks and are **regression pins, not
the row's proof**. Saying otherwise would be the shape `reference_liveness_break_needs_failing_baseline`
warns about.

- [ ] **T1 — Record the pre-edit baselines. ZERO production bytes.** Every figure below with its
  DENOMINATOR asserted and its AXIS stated (§2 item eight). `-v -count=1` on every run.
  - T1a `go test ./internal/filter/hcm/h2/ -count=1 -v` — expect rc=0, `=== RUN` **204**,
    `^ *--- PASS` **204** (⚠️ `^--- PASS` reads **116** — top-level axis), anchored FAIL **0**, SKIP **0**,
    ~**1.745 s**.
  - T1b the same package with `-race` — expect **204 / 204 / 0 / 0**, `DATA RACE` **0**, ~**7.825 s**.
  - T1c `./internal/filter/hcm/` plain and `-race` — expect **322 / 322 / 0 / 0** (⚠️ `^--- PASS` = 226),
    ~0.016 s plain.
  - T1d `./internal/cluster/ -race -count=1 -v` — **this is the ONLY in-process gate that exercises
    `client.go`'s read path across many real H/2 conns** (§7.2). Record rc, RUN, PASS (both axes),
    anchored FAIL, `DATA RACE`, wall time.
  - T1e `gofmt -l internal/filter/hcm/h2/` — ⚠️ **gate on OUTPUT; `gofmt -l` never exits non-zero.**
    Baseline is byte-EMPTY.
  - T1f `golangci-lint run ./internal/filter/hcm/h2/...` — ⚠️ **gate on OUTPUT, not rc.** Baseline is
    rc=0 **and zero bytes** (`golangci-lint` v1.64.8 is installed at `/home/esa/go/bin/golangci-lint`).
  - T1g `go vet ./internal/filter/hcm/...` — record output (copylocks is what would bite §3.1).

- [ ] **T2 — RED ANCHOR 1: the CONTENDED UNPATCHED arm at n >= 30.** Per §6.5. ZERO production bytes;
  prove the tree clean afterwards. ⚠️ **Report the bimodality explicitly and flag any third mode.**

- [ ] **T3 — RED ANCHOR 2: h2spec baseline AND its MANDATORY reddening negative control.** Per §6.6.
  ⚠️ **COMMIT FIRST** (`reference_break_protocol_commit_first`: `git checkout --` restores from HEAD and
  wipes uncommitted work), sha256 before/after, `sha256sum -c` => `OK`, and `git diff --stat master` EMPTY
  at the end.

- [ ] **T4 — production: the framer seam.** `internal/filter/hcm/h2/framer.go`. Add, per §3.1-§3.3 and
  §3.6: the six struct fields, the four channel allocations in `newFramer` (`:114`), `errReaderNotStarted`,
  `tryReadFrameWait`, `aLongTimeAgo`, `startReader`, `readerLoop`, `release`, `exitErr`, `closeReader`.
  ⚠️ **`readerLoop`, NOT `readLoop`** — `client.go:305` already owns that name.

- [ ] **T5 — production: rewrite the two consumer methods and DELETE every reader-side deadline.**
  `readFrameCtx` (`:164`) and `tryReadFrame` (`:201`) per §3.4/§3.5. ⚠️ **`framer.go` must end with ZERO
  `SetReadDeadline` sites** (it has **six** today: 170, 173, 185, 191, 202, 204 — the set re-derived at
  this tip and complete). ⚠️ **Assert the symbol, not the build** — a build proving nothing is
  `reference_symbol_assertion_needs_qualified_name`; use `grep -F` for receiver-parenthesised anchors
  (`-E` reads `(f *framer)` as a group and returns a FAIL-UNSAFE ZERO).

- [ ] **T6 — test: repair `TestFramer_ReadFrameCtxCancel`.** `framer_test.go:320` (⚠️ **the test is at
  :320; :330 is the call site** — C4). Add `startReader()` + `defer closeReader()`. **+2/−0.** Its own
  task per §4.5 — a silently-adjusted assertion is how a real regression gets laundered.

- [ ] **T7 — production: `conn.go` wiring, FOUR edits.** Per §4.1/§4.2: `defer s.fr.closeReader()`
  textually AFTER `:207`; `s.fr.startReader()` after `:238`; `s.fr.closeReader()` between `:277` and
  `:278`; and between `:313` and `:314`. ⚠️ **The two drain blocks are 276-281 and 311-317** (C1/C2), and
  `flushPendingDispatch()` at `:312` already precedes `emitGoaway` in the second — **do not reorder it.**

- [ ] **T8 — production: `client.go` wiring, FOUR edits.** Per §4.1/§4.2: `cc.fr.startReader()` before
  `:266`; `defer cc.fr.closeReader()` as `readLoop`'s first statement (`:306`); `cc.fr.closeReader()`
  after `cc.cancel()` at `:272`; `cc.fr.closeReader()` between `:1009` and `:1010`.
  ⚠️ **`:250/:255/:260` get NO edit** — §4.3, and the reason is written there.

- [ ] **T9 — test: NEW `internal/filter/hcm/h2/framer_reader_test.go`.** The five unit pins of
  §6.2-§6.4. Test-first within the task.

- [ ] **T10 — test: NEW `internal/filter/hcm/h2/framer_leak_test.go`.** The goroutine-leak guard of
  §6.1, **with its discriminating negative control**. ⚠️ **It must POLL, never sample once** — dispatch
  goroutines OUTLIVE `Run` (`conn.go:358`'s `go fn()`).

- [ ] **T11 — the contended measurement, PATCHED, INTERLEAVED with a re-run of the unpatched arm.**
  Per §6.5. ⚠️ **0 FAIL / 12 IS NOT PROOF** — the same unmodified tree read 3/12 and 6/12.

- [ ] **T12 — h2spec, PATCHED: the green run AND the reddening NC re-run against the patched tree.**
  Per §6.6. A green h2spec alone proves nothing (§6.6).

- [ ] **T13 — the inline-join disclosure measurement.** Per §6.7 — record that deleting either inline
  join at `conn.go:277-278`/`:313-314` leaves the package GREEN, so the no-op is **stated and measured**
  rather than implied.

- [ ] **T14 — the bounded race-instrumented SUBJECT probe.** Per §7.3. ⚠️ COMMIT FIRST; sha256 guard;
  revert; prove the tree clean.

- [ ] **T15 — the six-gate posture.** Per §11. ⚠️ **Name departures; do not claim compliance.**

- [ ] **T16 — docs.** Per §9: ADR-0313 §Decision + §Consequences appended IN PLACE after the RETAINED
  italic footer (**no renumber, NO `---` separator** — `^---$` stays **216**), `PROGRESS.md`,
  `ROADMAP.md` row 91 -> `done`, `STATE.md` rolled IN PLACE, `STATE_HISTORY.md` **ONE INLINE LINE**,
  `next-prompt.txt` (`git add -f`).

---

## 6. THE TEST ROSTER

### 6.1 The goroutine-leak guard — `framer_leak_test.go` (NEW)

⚠️ **`goleak` is NOT a dependency** — re-confirmed at this tip: `command grep -n goleak go.mod go.sum`
returns **nothing** (exit 1), and the only tree-wide hits are three prose files saying so. Adopting it
would break the `+0 go.mod modules` envelope (`reference_new_subpackage_pulls_transitive_module`).

⚠️ **The h2 package has NO `pollUntil` and there is no testify** (`require.`/`assert.` => zero hits).
The three in-tree copies have **THREE DIFFERENT SIGNATURES**, and ⚠️ **the SPEC's path for the third one
DOES NOT EXIST** (C6):

| # | path (RE-DERIVED — the SPEC's third is wrong) | signature | budget / poll |
|---|---|---|---|
| a | `internal/cluster/connpool_test.go:34` | `pollUntil(t *testing.T, cond func() bool, msg string)` | 2 s / 1 ms, `t.Fatalf` |
| b | **`internal/filter/http/router/hedge_test.go:353`** (SPEC said `internal/cluster/hedge_test.go`) | `pollUntil(t *testing.T, msg string, cond func() bool)` — **args SWAPPED** | 5 s / 1 ms, `t.Fatalf` |
| c | `internal/listener/listener_test.go:130` | `pollUntil(budget time.Duration, pred func() bool) bool` — **no `t`** | caller / 5 ms, **re-checks after the deadline** |

**MIRROR shape (a).** Grounds: it is the shape the goroutine-leak precedent itself uses
(`internal/cluster/h2pool_test.go`, whose `:1450` baseline and `:1493` barrier are both in
`TestH2PoolWatcherEvictRaceNoLeak` at `:1443`), so the mirrored helper and the mirrored *use* come from
one place. ⚠️ **MIRROR IT, do NOT import it** (it is an unexported test helper in another package) and
⚠️ **do NOT mint a fourth shape.** Name it `pollUntil` in the h2 package — no collision, the package has
none.

The guard, mirroring `h2pool_test.go:1443-1495`:
- capture `baseGoroutines := runtime.NumGoroutine()` **BEFORE** any connection is made;
- open and fully close **40** `ServerConn`s (the BRAINSTORM's measured denominator) driving real frames;
- close with a **poll**, never a single sample:
  `pollUntil(t, func() bool { return runtime.NumGoroutine() <= baseGoroutines+N }, "...")`.
- ⚠️ **`N` is a SLACK, and it must be justified in a comment, not chosen.** The precedent uses `+8` for
  "transient accept goroutines". Derive N from the fixture actually used and say why —
  ⚠️ **dispatch goroutines OUTLIVE `Run`** (`conn.go:356-361`'s `go fn()`), which is exactly why the
  assertion polls.

⚠️ **THE DISCRIMINATING NEGATIVE CONTROL IS MANDATORY AND IS THE HALF THAT MATTERS.** Gut `closeReader`
to a bare `return` and the guard MUST RED. The BRAINSTORM measured **delta 5 over 40 connections** with
the reader leaked. **Without this NC the guard is `reference_review_mandated_guard_is_untested`** — a
green that would stay green with the fix deleted. Record the NC's actual output, restore under a sha256
guard, and re-run to green.

### 6.2 The channel-capacity tripwire

SPEC gap 4: *"the aliasing invariant is handled but UNTESTED — nothing reddens if the channel is later
given capacity"*, and the SPEC RAISED the stakes: capacity yields a **PROCESS CRASH**
(`panic: Frame accessor called on non-owned Frame`, `frame.go:213`), not wrong data — there is **no
`recover()` anywhere** in `internal/filter/hcm/` (re-derived: zero hits, whole subtree including tests).

**Pin it directly:**
```go
if got := cap(f.frameCh); got != 0 {
	t.Fatalf("frameCh capacity must stay 0: a buffered frame channel lets the reader "+
		"re-enter ReadFrame while the consumer still holds frame N, and frame.go:498-499 "+
		"invalidates it — every one of the 14 live accessor call sites then PANICS with no "+
		"recover() in the tree. got cap=%d", got)
}
```
⚠️ **This is a cheap structural pin, and it is deliberately NOT dressed up as a behavioural test.** It
cannot catch a *reordering* of the handshake, only a capacity change. **Say so in the comment**; a pin
that oversells its reach is `reference_counter_cannot_gate_a_value`.

### 6.3 `readErr` stickiness + ctx precedence (D-91-ERR, §3.7)

Three assertions on one loopback TCP conn pair (⚠️ **not `net.Pipe`** where a real socket is wanted —
`reference_netpipe_deadlocks_client_cert_handshake`; mirror whatever pair `framer_test.go` already builds):
1. after the peer closes, the FIRST `readFrameCtx` returns a non-nil error;
2. the SECOND and THIRD return **the same error class immediately** — assert an upper bound on elapsed
   time so a hang cannot pass as stickiness;
3. **ctx precedence is UNCHANGED**: with `frameCh` already closed AND `ctx` cancelled, `readFrameCtx`
   returns **`ctx.Err()`**, not the stored read error (§3.4 checks `ctx.Err()` first).
   ⚠️ Assertion 3 is the one that would catch someone "simplifying" the early return away.

### 6.4 The not-started guard, and `closeReader` idempotence

4. `readFrameCtx` / `tryReadFrame` on a composite-literal framer (`&framer{Framer: ..., conn: nil}`,
   the `framer_writeheaderblock_test.go:235` shape) return **`errReaderNotStarted`** — **not a hang and
   not a nil-deref panic.** ⚠️ Today that shape nil-panics on `f.conn.SetReadDeadline`; under a nil
   channel a `select` would **block forever**. The guard converts a hang into an error, and this test is
   what keeps it that way.
5. `closeReader` is safe called (i) never-started, (ii) twice, (iii) concurrently with a consumer
   blocked in `readFrameCtx`; and **after it returns the reader goroutine is gone** — assert via the
   `runtime.NumGoroutine()` poll of §6.1, not by sleeping.

### 6.5 The contended measurement — **n >= 30 PER ARM, ARMS INTERLEAVED**

Subject: `TestServerConn_TinyWindowDelivery`, `conn_test.go:862`. Its own `frameReadTimeout` is
**`const frameReadTimeout = 10 * time.Second`** at **`conn_test.go:965`** — function-local, inside that
test — which is what the 10 s pole IS.

**Recipe (each iteration a separate process, denominator asserted per iteration):**
```
go test ./internal/filter/hcm/h2/ -run '^TestServerConn_TinyWindowDelivery$' -count=1 -v
```
under: **8 CPU burners pinned to core 0 with `taskset -c 0`**, `GOMAXPROCS=1`,
⚠️ **killed BY CAPTURED PID ONLY** — `pkill -f` / `pgrep -f` match the harness's own shell and kill the
tool call with exit 144 (`reference_pkill_f_kills_own_tool_call`). Verify zero residue by `kill -0` on
each captured PID.

**Per iteration record:** rc, `=== RUN` count (⚠️ **MUST be >= 1** — a `-run` typo prints
`[no tests to run]` and **EXITS 0**), PASS/FAIL, and the elapsed seconds from the `--- PASS/FAIL` line.

**⚠️ INTERLEAVE THE ARMS — U,P,U,P,… — do not run 30 unpatched then 30 patched.** The base rate is
demonstrably non-stationary (**3/12 and 6/12 on two runs of the SAME unmodified tree**), so a block design
confounds the fix with machine drift. Interleaving shares the drift across both arms.

**Classify by threshold, not by a remembered constant** (§2 item nine):
- **fast pole** — measured **0.01 s** at this tip uncontended (5/5), `0.02 s` under `-race`; the
  BRAINSTORM saw ~0.07 s contended. Any iteration **< 1 s** is fast.
- **slow pole** — **~10.03 s**, i.e. `frameReadTimeout` tripping. Any iteration **> 9 s** is the HANG.
- ⚠️ **ANY iteration landing between 1 s and 9 s is a THIRD MODE and REFUTES the bimodality claim.**
  Report it; do not average it away. For calibration the slowest legitimate test in the whole package is
  `Continuation_ArmE_GoawayPin` at **0.50 s**, so nothing legitimate is near the slow pole.

**Report:** `U: x FAIL / 30+`, `P: y FAIL / 30+`, the full elapsed list per arm, and the mode census.
⚠️ **A green uncontended run is NOT evidence** — 5/5 uncontended and 6/12 contended on the SAME binary.
**This is the reading that absorbed a live production deadlock as a flake three times.**

### 6.6 h2spec 5.1.2/1 — the gate AND its MANDATORY reddening negative control (D-91-GAP1)

**The gate:** `go test ./test/conformance/h2spec/ -timeout 5m -v` (the CI form, `ci.yml:71`). The sole
test is **`TestH2Spec`** (`h2spec_test.go:30`). Baseline to reconcile against:
**`95 tests, 94 passed, 1 skipped, 0 failed`** (`CONFORMANCE_PINS.md:142`, commit `83ebf029`, 2026-08-10;
the skip is invariantly 6.9.2/2). Verified read-only at this tip: `"http2/5.1.2": 1` is a **pinned** entry
in `expectedSuites` (`h2spec.go:51`, **31 entries**), reached by the `"http2/5"` selector (`:26`), `-S`
strict IS passed (`h2spec_test.go:131`), and `imageRef()` (`:15`) resolves to a **local** image whose
`docker images --digests` RepoDigest matches
`sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` **byte for byte** — no pull.
⚠️ **h2spec has still NOT BEEN RUN at any stage of this row.** T3/T12 are its first executions here.

⚠️ **THE REDDENING NEGATIVE CONTROL IS THE LOAD-BEARING HALF AND IS MANDATORY.** §6.2 of the SPEC proved
by GUTTING the drain — not by a grep — that the ordering guarantee is pinned by **zero** tests
(`tryReadFrame` degenerated to `return nil, nil` runs the package **204/204 green 4/4**, and both
ordering-adjacent tests pass 5/5). So *"h2spec passed"* is uninformative until the gate is shown able to
fail. **Arm A is the ready-made injection** (`--numstat 2 16`).

⚠️ **AND THE NC MUST PROVE THE SUITE ACTUALLY RAN, NOT MERELY THAT IT EXITED NON-ZERO — or, in the green
direction, that it exited ZERO.** `h2spec_test.go` **SKIPS** rather than fails when Docker is missing
(`:32-34` on `-short`; `:37` on `exec.LookPath("docker")`; `:40-43` on `docker version`). **A broken
Docker yields a green, vacuous run that launders as a pass.** Every h2spec run in T3 and T12 must
therefore assert **`--- SKIP: TestH2Spec` count == 0** and that the printed case census is present, not
just `rc`.

⚠️ **RESIDUAL RISK, NAMED:** 5.1.2/1 exercises exactly ONE shape (fast `direct_response`, default
max=100). It covers **neither** trailer-driven dispatch (`conn.go:467-470`) **nor** DATA-END_STREAM
dispatch (`conn.go:649-652`), which queue into the same `pendingDispatch`. **A single-case smoke gate,
not a proof.**

### 6.7 The inline-join disclosure (§4.4)

Delete `s.fr.closeReader()` from **one** drain site, run the package, record the result; restore; repeat
for the other. **Expected: GREEN both times** — that is the point. Record it, so the two calls are
documented as a **measured** structural invariant rather than an unexamined "guard".
⚠️ **Do NOT convert this into a reason to drop them** (§4.4 gives the two reasons to keep them); and do
NOT report it as a passing test — it is a disclosure.

---

## 7. D-91-RACE — **DECIDED**, and the decision is neither "yes" nor "no"

**DECISION: (a) the full differential suite runs WITHOUT `-race`; (b) `-race` at the THREE in-process
packages that actually run this code is MANDATORY; (c) a BOUNDED, race-instrumented SUBJECT probe over
the four H2-capable fixtures is MANDATORY in place of (a).**

### 7.1 Why (a) — a `-race` differential run would be a VACUOUS GREEN on this row's axis

1. ⚠️ **The subject is a separate process, built WITHOUT `-race`, and there is NO seam.**
   `harness.go:240` and `harness.go:594` each hard-code
   `exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/envoy-go")`. A selector for a seam
   (`SUBJECT_BUILD|buildArgs|buildFlags|os.Getenv|flag.(String|Bool|Int)`) returns **NO HITS** in
   `harness.go`; the only `os.Getenv` calls anywhere under `test/differential/` are
   `harness_test.go:112` and `:376`, both reading **`HOME`** for a docker socket path. ⇒ `go test -race
   ./test/differential/` instruments the harness, drivers and backends — **not the binary that serves
   every fixture's HTTP/2.**
2. ⚠️ **NAMED NUANCE, because the naive check gives the opposite answer.** The differential *test binary*
   **does** link `internal/filter/hcm/h2`: `go list -deps -test ./test/differential/ | grep -c
   'internal/filter/hcm/h2$'` => **1** (positive control `./internal/cluster/` => 1; negative control
   `./internal/stats/` => **0**; the binary has **661** deps, so the selector is alive). ⚠️ **Note the
   `-test` flag is load-bearing — WITHOUT it the same selector reads 0**, which is how a session
   concludes "not linked" and stops looking. The chain is transitive:
   `test/differential -> internal/filter/http/lua (and /ratelimit) -> internal/cluster -> …/h2`.
   **Linking is not executing.** No file under `test/` imports the package in code — selector
   `git grep -n '"github.com/pgdad/envoy-go/internal/filter/hcm/h2"' -- 'test/*.go' 'test/**/*.go'`
   returns **NONE**; all seven textual mentions under `test/` are README prose or `//` comments (e.g.
   `0004-h2-routing/driver/driver.go:810`, `0080-h2-goaway-rotation/driver/driver.go:256`). ⇒ the link
   does not rescue the observation.
3. **Cost, and the cap — a SECONDARY reason, stated as secondary.** CI runs the suite at `-timeout 20m`
   inside a `timeout-minutes: 30` job (`ci.yml`), and records **~6.5 min** at the current corpus
   (5m23s at 91 fixtures, 2026-06-29). Measured `-race` inflation: **4.5×** on `./internal/filter/hcm/h2/`
   (1.745 s -> 7.825 s) and **2.78×** on `./internal/cluster/` (4.260 s -> 11.837 s). 6.5 min × 2.8-4.5
   is **18-29 min** — at or past the binary timeout. Adding it converts a green gate into a timeout.
4. **CI already agrees, and always has.** The only `-race` in `.github/workflows/` is the unit job:
   `go test -short -race ./...` (`ci.yml:24-30`). ⚠️ **`-short` EXCLUDES the differential suite**
   (`TestDifferential` skips under `-short`), so the full suite has **never** been run under `-race` in
   CI, and this row is not departing from a standing practice — it is declining to invent one.

### 7.2 Why (b) — the THREE packages that DO run this code in-process, with baselines

| package | why it is on the roster | baseline measured at this tip |
|---|---|---|
| `./internal/filter/hcm/h2/` | hosts the change | `-race` rc=0, **204 RUN / 204 PASS / 0 anchored FAIL / 0 SKIP / 0 `DATA RACE`**, **7.825 s** |
| `./internal/cluster/` | ⚠️ **the ONLY package that drives the CLIENT read path across many real H/2 conns** — `h2pool_test.go:87` and `h2pool_overflow_integ_test.go:67` call `h2.NewClientConn` directly, and ~10 `dial_h2_test.go` tests reach it via `dial_h2.go:134` | `-race` rc=0, **458 RUN / 458 PASS (top-level axis 404) / 0 FAIL / 0 SKIP / 0 `DATA RACE`**, **11.837 s** (plain: 4.260 s) |
| `./internal/filter/hcm/` | the parent; see the correction below | `322 / 322 / 0 / 0`, 0.016 s plain |

⚠️ **A TENTH SPEC CORRECTION, AND IT CUTS THE OTHER WAY FROM THE FIRST NINE.** The SPEC says the parent
package *"never drives a real `ServerConn` frame loop"*. **That is wrong as a code claim** — and a grep
for `h2.NewServerConn(` in `internal/filter/hcm/*_test.go` returns **ZERO**, which is exactly how the
wrong conclusion is reached. The chain is real: `filter_test.go:150` sets
`CodecType = HttpConnectionManager_HTTP2`, `filter_test.go:161` calls `f.Handle(...)`, and
`filter.go:113 Handle` -> `:125 runH2` -> **`filter.go:189 h2.NewServerConn`** -> `Run()`.
**The SPEC's CONCLUSION nevertheless survives, on a sharper and now-MEASURED reason:** the sole such test,
`TestFilter_Handle_HTTP2_PlaintextH2C` (`filter_test.go:148`), **closes the client before the preface**
(`:158`), so `Run` returns at `conn.go:212-215` and **never reaches `startReader()`**. The reader
goroutine is never spawned there. ⇒ **corroborating, not independent** — but say why correctly.

⚠️ **AND THAT TEST BECOMES A LIVE REGRESSION PIN FOR §3.6's UNSTARTED-FRAMER GUARD.** It is the one
existing test in the tree that runs `Run`'s new `defer s.fr.closeReader()` on a framer whose reader was
**never started**. If `closeReader` were to join `<-f.doneCh` unconditionally, this test would **hang**
and `filter_test.go:165`'s 3-second `time.After` would report it. **T7 must not "fix" that timeout.**

### 7.3 Why (c) — the bounded race-instrumented SUBJECT probe (T14), MANDATORY

Declining (a) leaves a real gap: nothing race-instruments the reader goroutine under **fixture-fleet**
load, and SPEC gap 6 correctly names that as the shape that has produced `-race`-only findings here
before. **Retire it cheaply instead of waving at it:**

1. ⚠️ **COMMIT FIRST** (`reference_break_protocol_commit_first`), then `sha256sum` `harness.go`.
2. Add `"-race"` to the subject build at **BOTH** `harness.go:240` and `harness.go:594` — ⚠️ **both, or
   the probe silently instruments only one of the two spawn paths.**
3. Run **only** the four H2-capable fixtures:
   ```sh
   go test ./test/differential/ -count=1 -v \
     -run 'TestDifferential/^(0004-h2-routing|0079-h2-multiplex-pool|0080-h2-goaway-rotation|0119-grpc-unary-trailers)$'
   ```
   ⚠️ **The `^…$` anchors are load-bearing, and were validated with a decoy** — a scratch reproduction
   with an added `0119-grpc-unary-trailers-extra` entry ran **4** subtests anchored and **5** unanchored.
   Subtests are named by the fixture **directory name verbatim** (`runner_test.go:193`'s `t.Run(fx, ...)`,
   the only `t.Run` in the package; discovery is `discoverFixtures` at `runner_test.go:1462`).
   ⚠️ **`go test -list` CANNOT enumerate these** — it prints only the 17 top-level tests; subtests are
   discovered at runtime. Do not try to source the names from it.
4. ⚠️ **ASSERT THE FOUR FIXTURES ACTUALLY RAN.** A `-run` selector matching nothing prints
   `[no tests to run]` and **EXITS 0** (`reference_differential_run_selector`), and `runner_test.go:200`
   `t.Skipf`s an unregistered fixture with **no fixture-count gate anywhere**
   (`reference_differential_fixture_three_registration_gates`). Assert all four subtest names appear with
   `--- PASS`, and that `--- SKIP` count is **0**.
5. **Gate on the ANCHORED panic pattern `^panic:|DATA RACE|SIGSEGV` over the COMBINED output.** The
   subject's stderr reaches the log (`harness.go:258` `cmd.Stderr = os.Stderr`; the boot-reject path uses
   `io.MultiWriter`), so redirect `2>&1` — ⚠️ **the race detector reports on the SUBJECT's stderr, not
   the test binary's**, and a run captured without `2>&1` would print a clean green over a real report.
6. **Revert both edits, `sha256sum -c` => `OK`, and `git diff --stat master` EMPTY.**

⚠️ **NAMED LIMIT, so this is not oversold:** four fixtures under normal scheduling is a **smoke probe**,
not fleet coverage, and it cannot reproduce the contended trigger (§8 item 1). It exists to catch a
*deterministic* race the unit `-race` gates would miss, nothing more.

---

## 8. DIFFERENTIAL — D-91-DIFF carried forward unchanged, with one correction to its evidence

**NO differential fixture. Fixtures stay 121 dirs / 120 distinct indices; `0120` stays UNCONSUMED.**
(Re-derived at this tip: `ls -d test/fixtures/[0-9]*/ | wc -l` => **121**; distinct 4-digit indices
=> **120**, `0007` the sole doubled index; `ls -d test/fixtures/0120*` => `No such file or directory`.)
⚠️ **The wrong path `test/differential/fixtures/` returns a SILENT `0` when its stderr is dropped** —
confirmed live at this tip (`exit=2`, but `| wc -l` prints `0`).

The SPEC's four grounds are carried verbatim and are not re-opened. **One of them has a wrong stated
reason, corrected here** (`reference_a_drift_correction_is_itself_a_claim` — the correction is itself a
claim, so it was measured):

⚠️ **SPEC §7 ground 4 identifies the four H2-capable fixtures on the basis that *"`codec_type: AUTO`
[is] the only value that reaches the H/2 codec"*. THE SET IS RIGHT; THE REASON IS WRONG, TWICE.**

- **`codec_type` is a proto enum whose ZERO VALUE IS `AUTO`.** **36** fixture dirs never write
  `codec_type` at all and are therefore *also* `AUTO`. A selector gating on `codec_type: AUTO` alone
  **over-selects ~40 dirs**.
- **`AUTO` is not even the stronger trigger.** `filter.go:120-139` reaches `runH2` from **two** arms:
  `case HttpConnectionManager_HTTP2:` at **:125** unconditionally, and `case _AUTO:` at **:127-136**
  *only* when the downstream is a `*stdtls.Conn` whose `NegotiatedProtocol == "h2"`. No fixture uses
  `HTTP2` (only unit tests do); that is a fact about the corpus, not about the enum.

⇒ **THE REAL DISCRIMINATOR IS THE DOWNSTREAM ALPN OFFER.** Re-derived two ways that agree exactly:
`git grep -ln 'alpn_protocols' -- 'test/fixtures/'` filtered to `"h2"` yields **exactly**
`0004-h2-routing`, `0079-h2-multiplex-pool`, `0080-h2-goaway-rotation`, `0119-grpc-unary-trailers` — the
same four the `codec_type: AUTO` literal selects. **State the ALPN reason; the enum reason generalizes
badly** (`reference_probe_discipline`).

⚠️ **AND THE UPSTREAM LEG DOES NOT WIDEN THE SET.** 36 fixture dirs carry `http2_protocol_options` on a
*cluster*, but those are served by **`google.golang.org/grpc`'s own HTTP/2**
(`grpcclient.go:130`'s `grpc.NewClient(... WithContextDialer ...)`), not by `internal/filter/hcm/h2`. The
in-house client is reachable only via `router_h2.go:120 AcquireH2Stream` -> `h2pool.go:298` ->
`dial_h2.go:134`; `router.go`/`retry.go`/`hedge.go` contain **zero** `UseH2`/`AcquireH2Stream`/`DialH2`
hits, so the H1 path never touches it. Even `0068-health-check-grpc` (HTTP/1 downstream, h2c upstream)
does not run the in-house framer on its data plane. **Both framer legs are confined to the same four
fixtures**, which is why §7.3's probe is scoped to exactly them.

**The departure is RECORDED on the SPEC's CORRECTED grounds** — not *"no reference-side counterpart"*
(the observable IS reference-comparable: deadlock vs response, exactly what `CompareBytes` catches), but
**the TRIGGER is not harness-controllable**: no `cmd.Env`, no cpuset, no `GOMAXPROCS` hook on the subject
subprocess (§7.1 item 1 re-confirms there is no build or env seam at all), and the measured base rate
makes any arm a flake rather than a gate.

---

## 9. DOCS — text CONSTRAINED here, LANDED at the IMPL

### 9.1 ADR-0313 — §Decision + §Consequences

Appended **IN PLACE** after the RETAINED italic footer
(`*§Decision and §Consequences follow at the phase-91 IMPL.*`), per the ADR-0294-0312 shared block form.
⚠️ **No renumber. NO `---` separator** — `^---$` STAYS **216**. The strict `^> **STATUS: PROPOSED` guard
is **DISARMED by the IMPL** (1 -> 0); ⚠️ **verify the disarm BY LINE AND ADR**, because the historical
`^**Status:** PROPOSED` at **ADR-0231** (`DECISIONS.md:14866`) also reads 1 and is a decoy.
Next-free STAYS **ADR-0314** (`grep -c '^## ADR-0314'` => **0**).

**§Decision must state, in these terms:**
- the reader-goroutine + **unbuffered** frame channel + **release-at-start-of-next-read** handshake;
- ⛔ **NOT** *"a deep copy is impossible from outside `x/net`"* — that inference is **FALSE** and was
  disproven by an executed probe. **The correct reason:** `frame.go` exports exactly **two package-level
  producing functions** (`ReadFrameHeader` `:230`, `NewFramer` `:430`) and **zero setters**
  — ⚠️ **with the qualifier of C7: `(*Framer).ReadFrame` at `frame.go:496` is an exported METHOD that
  produces a `Frame`, but only from the WIRE, never from a caller-supplied `[]byte`** — so a copied
  payload cannot be re-minted as an `http2.Frame`. You can copy the *bytes*, never the *value*;
- the hazard is **a PANIC and only a panic** (`frame.go:213`; 8 accessors check, production calls three
  at **14** live sites, **zero `recover()`** in `internal/filter/hcm/`), never wrong data, because every
  consumer already copies at the boundary;
- ⚠️ **the word "unconditionally" must NOT be used of `frame.go:499`** (C5) — it is guarded by
  `if fr.lastFrame != nil` at `:498`.

**§Consequences must record, at minimum:** (i) the three parked states and why `closeReader` is
signal + stamp + join (§3.6); (ii) that the stamp is safe **only because** all six reader-side
`SetReadDeadline` sites are deleted; (iii) `readErr` stickiness as new behaviour with ctx precedence
unchanged (§3.7); (iv) `tryReadFrameWait = 2 ms` with its two bounds and the ≤4 ms escape hatch (§3.5);
(v) D-91-RACE and its three parts (§7); (vi) that the two inline drain joins are **measured no-ops at
this tip**, retained as a structural invariant (§4.4, §6.7); (vii) that SPEC site 5 takes no edit and why
(§4.3); (viii) the pre-existing `upstream` leak at `client.go:250/255/260`, recorded not fixed; (ix) the
h2spec residual risk (§6.6); (x) the ten SPEC corrections of §2/§7.2/§8.

### 9.2 The other doc edits

`PROGRESS.md` (NEW in the phase dir) with every gate's ACTUAL output quoted · `ROADMAP.md` row 91
`in-progress` -> `done` (⚠️ **field-count the edited row: NF must stay 8 under BOTH the escape-aware and
naive forms** — an unescaped `|` passes check (1) SILENTLY) · `STATE.md` rolled **IN PLACE**, oldest
§Recent entry evicted by a **DIRECT DATE READ** (⚠️ **the tie shape does NOT carry** — the phase-91 SPEC
had a unique oldest; the phase-90 IMPL broke a two-way tie by last list position. **Read the dates**) ·
`STATE_HISTORY.md` **ONE INLINE LINE** so the strict guard stays at **163, DELTA 0** ·
`next-prompt.txt` (⚠️ **`git add -f`** — tracked but gitignored).

### 9.3 Documentary defects — RECORDED, deliberately NOT fixed

**NEW this stage:** the seven SPEC cite defects of §2 (C1-C7) · the SPEC's *"never drives a real
`ServerConn` frame loop"* (§7.2) · the SPEC's `codec_type: AUTO` discriminator (§8) · ⚠️ **the
pre-existing `net.Conn` leak at `client.go:250/255/260`** — all three `cancel()` without closing
`upstream`, and `git grep -n 'upstream' client.go` shows **no `upstream.Close()` anywhere in the file**
(§4.3).

**Carried:** `ROADMAP.md` row 91's own `conn.go:217` cite is off by one (`:218` is the
`writeServerInitialSettings` call; `:217` is its COMMENT — ⚠️ **an off-by-one that lands on a comment
reads as a correct cite**) · `ROADMAP.md` cites `esalaine` **4 LINES / 7 OCCURRENCES** (lines 74, 75, 113,
237) — the long-carried *"FIVE times"* is wrong on BOTH axes; **state the axis** · `STATE.md` §Project
counts SELF-CONTRADICT §Current (fixtures **119** / next-free **ADR-0299** vs **121** / **ADR-0313**) and
carry NO label saying so · `BEHAVIOR_CONTRACT.md:2040` still carries the D-89-HOST rationale ADR-0312
declares RETIRED · ADR-0312 (xviii) item 4's *"12-row CWE-444 suite"* is **10** rows landed; item 3 names
the WRONG FIX SITE (`dispatchRequest`, should be `serveOneRequest`); item 6's H3 arm-A prediction is
REFUTED · `ROADMAP.md` rows **57**/**69** malformed (the ARM-A guard) · `0004-h2-routing/README.md:40` ·
`0004-h2-routing/envoy-go.yaml:3` · the phantom gate `git grep -c 'h2.parseHeadersForRequest'` reads
**1** (a COMMENT citation) while `^func.*parseHeadersForRequest` reads **0** ·
`internal/filter/http/types.go`'s FALSE *"per ADR-0071"* comment · `DECISIONS.md`'s `INNER_EXIT=0` at
phase 87, **a value nothing in the tree emits** · the xDS cycle guard NOT AUTOMATED · `wasm/doc.go:219` ·
`rbac.go:50` token `F2` · `ADR-0057`'s *"27 round-trips"* (now **31**).

---

## 10. COST — RE-DERIVED FROM THE FROZEN DESIGN, and the SPLIT GATE EVALUATED

### 10.1 ⚠️ The `+160 / −53` floor is NOT used as the estimate — it is used as the thing to beat

`reference_measured_prototype_is_a_lower_bound` has fired **four consecutive rows**, always through
UNDER-ENUMERATION, and the SPEC §9 predicted a fifth. **It fires.** The floor is inherited from a
prototype that was built, measured and **reverted** — there is no patch in the tree to re-measure, so no
re-derived figure is quoted against it. What follows is priced **off §3's written design**, method by
method.

| axis | estimate | derivation |
|---|---|---|
| `framer.go` | **+216 / −60** | struct+comments ~28 · `newFramer` +6 · consts/vars/`errReaderNotStarted` ~22 · `startReader` ~14 · `readerLoop` ~28 · `release` ~14 · `exitErr` ~10 · `closeReader` ~30 · `readFrameCtx` ~34 · `tryReadFrame` ~30. Deleted: the two old bodies **with** their doc comments (`:155-194`, `:196-218`). |
| `conn.go` | **+14 / −0** | 4 statements + ~10 comment lines (§4.1) |
| `client.go` | **+14 / −0** | 4 statements + ~10 comment lines (§4.1) |
| **production total** | **+244 / −60 (net +184)** | 3 files, **1 package** |
| `framer_reader_test.go` (NEW) | **+190** | 5 pins (§6.2-§6.4) |
| `framer_leak_test.go` (NEW) | **+120** | guard + mirrored `pollUntil` (~12) + its NC (§6.1) |
| `framer_test.go` | **+2 / −0** | T6 (§4.5) |
| **test total** | **+312 / −0** | |
| docs (landed at the IMPL) | **~+270** | ADR-0313 §Decision+§Consequences ~85 · `PROGRESS.md` ~180 · ROADMAP 1-line · STATE in place · STATE_HISTORY +1 |

### 10.2 The SEVEN additions SPEC §9 said the floor cannot contain — all priced, plus THREE it missed

| # | SPEC §9 item | priced at |
|---|---|---|
| 1 | `closeReader` as signal-and-join over three parked states | ~30 (§3.6) |
| 2 | that join inlined at TWO drain sites | +2 in `conn.go` (§4.1) |
| 3 | a defer in `readLoop`, which has none today | +1 in `client.go` |
| 4 | "up to FOUR new `NewClientConn` teardown edges" | ⚠️ **TWO, not four** — §4.3 removes three and §4.1 adds the ACK-wait branch and `Close` |
| 5 | the `startReader()` seam | ~14 + 2 wiring (§3.2) |
| 6 | the release handshake | ~14 (§3.4) |
| 7 | a new leak-test file with no `pollUntil` home | ~120 (§6.1) |

⚠️ **THREE MORE THE SPEC DID NOT NAME, found while writing §3:** `exitErr`'s fail-CLOSED nil-`readErr`
guard (~10, §3.4); `errReaderNotStarted` plus the two entry guards that convert a would-be **hang** into
an error (~12, §6.4); and **T6, the repair of an existing test the change breaks** (§4.5) — a `−0` test
axis was never going to survive. ⇒ **the fifth firing, and its cause is again under-enumeration.**

### 10.3 ⚠️ THE SPLIT GATE — EVALUATED, NOT ASSUMED

BOOTSTRAP §6.1 triggers a split if **>~25 numbered tasks** OR **>~1500 LoC of estimated net change**.

- **Tasks: 16** (§5). Under 25.
- **Net code change: ~+496** (production net +184, test +312). **Gross code churn: ~616.** Even counting
  the ~270 doc lines the total is **~770-890**. Under 1500 on every reading.
- **Packages touched: ONE** (`internal/filter/hcm/h2`). Fixtures: **+0**. `go.mod` modules: **+0**.

⇒ **THE SPLIT GATE IS NOT TRIPPED.** Priced against this re-derived estimate, **not** against the
`+160/−53` floor, exactly as the SPEC required.

⚠️ **THE MID-EXECUTION TRIGGER STAYS LIVE.** BOOTSTRAP §6.1 also splits *mid-execution* if any single
task's sub-steps blow past ~10 items. **T4 is the one to watch** — if the framer seam's sub-steps exceed
that, split 91.1 (the framer seam + its unit pins) from 91.2 (the wiring + the leak guard + the gates)
rather than shipping an oversize task.

### 10.4 ANTICIPATED ENVELOPE

`+0` stats · `+0` config fields · `+0` fuzzers (**55 / 48 files**) · `+0` BackendKind (**tail 38**) ·
`+0` `go.mod` modules (**67 = 18 direct + 49 indirect**) · `+0` fixtures (**121 dirs / 120 indices**) ·
`+0` packages · `+0` ROADMAP rows (**123**, `want` stays 123) · `+0` phase directories (**132**) ·
**`+1` ADR completed, `+0` ADRs added** (ADR-0313; next-free stays **ADR-0314**).
⚠️ **The `+0 go.mod` claim is CONDITIONAL on §6.1** — adopting `goleak` would break it.

---

## 11. GATES AND COUNTS

### 11.1 The six-gate posture — **name departures, do not claim compliance**

- **(a)/(b)** full `go test ./...` **and** the 121-fixture differential suite, gated on
  **`PIPESTATUS[0]`** and a **SET RECONCILIATION** — ⚠️ **NOT `INNER_EXIT`, which DOES NOT EXIST in this
  repo.** ⚠️ **Gate 2's failure mode is a SILENT PASS** (`runner_test.go:200` `t.Skipf`s an unregistered
  fixture; no fixture-count gate exists anywhere) — **assert the fixture set, not merely a green suite.**
- **(c)** h2spec: cite ONLY from your own run (§6.6), **with the SKIP-count assertion**.
  grpc-conformance deferred in writing; proxy-wasm **10/16**.
- **(d)** fuzzers **55 / 48 files**.
- **(e)** the ANCHORED panic gate `^panic:|DATA RACE|SIGSEGV` on every differential launch.
- **(f)** **no `REVIEW.md` — standing departure**, NAMED not claimed (`git ls-files | grep -c
  'REVIEW\.md$'` => **37**, a FILE count).

### 11.2 Per-run method rules — every one has drawn blood in this lineage

- ⚠️ **`-v -count=1` on every run.** `go test` without `-v` prints ZERO `=== RUN`; **`RUN=0` beside
  `RC=0` is a VACUOUS GREEN.** Caching serves a stale PASS without `-count=1`.
- ⚠️ **STATE THE PASS AXIS.** `^ *--- PASS` = **204** (h2) / **322** (hcm) / **458** (cluster);
  `^--- PASS` = **116** / **226** / **404**. **A gate asserting 204 against the anchored form REDS on an
  unmodified tree.**
- ⚠️ **Anchored FAIL only:** `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`. On **tree**-scope `-v` output an
  unanchored `grep -c FAIL` reads **11 on a fully green tree**. At package scope both forms read 0 and
  AGREE — **that agreement is NOT evidence the anchored form is unnecessary.**
- ⚠️ **`out=$(...); rc=$?` or `PIPESTATUS[0]`** — `rc=$?` after a pipe is the LAST command's status.
- ⚠️ **`gofmt -l` NEVER exits non-zero — gate on OUTPUT.** ⚠️ **`golangci-lint` must be gated on OUTPUT
  too**; a lint gate run but not gated on is indistinguishable from one never run. Baseline at this tip:
  both byte-EMPTY, `golangci-lint` v1.64.8.
- ⚠️ **Sweep British spellings from `.go` comments BEFORE the lint gate** —
  `golangci-lint`'s misspell runs in locale **US** and fired **twice** on phase 90 (`behaviour`).
  ⚠️ **This PLAN's own prose uses "behaviour" freely; that is fine in Markdown and FATAL in a `.go`
  comment.** The §3 code blocks are the text most likely to be pasted — check them.
- ⚠️ **A BUILD IS NOT EVIDENCE THE EDIT LANDED — ASSERT THE SYMBOL**, QUALIFIED and PATHSPEC-SCOPED, and
  ⚠️ **use `grep -F` for receiver-parenthesised anchors** (`-E` reads `(f *framer)` as a group and
  returns a FAIL-UNSAFE ZERO).
- ⚠️ **`command grep` / `/usr/bin/grep` / `git grep`** — the harness `grep` is a shell FUNCTION honouring
  `.gitignore` and is BLIND to `next-prompt.txt`. Inside `xargs`, `command grep` does NOT survive.
  ⚠️ Pass `--` before any pattern starting with `-`. ⚠️ `grep -c` counts LINES, not OCCURRENCES.
- ⚠️ **Port-band probes BELOW 32768** (`ip_local_port_range = 32768 60999`); `21000-24999` is clear.
  Check with `ss -tan` (ALL states) — `ss -ltn` reads a fail-unsafe "free".
- ⚠️ **NEVER `pkill -f` / `pgrep -f`** — they match the harness's own shell (exit 144). Kill only
  captured PIDs; verify residue with `kill -0`.
- ⚠️ **Check for sibling sessions before blaming this row for a port flake; never tear down a container
  this session did not create** (BY NAME only). Untouched throughout phases 90-91:
  `infallible_booth`, `crazy_kare`, `golink-ai`, `quizzical_goldstine`.
- ⚠️ **`go build ./cmd/envoy-go/` drops an untracked binary in the worktree root** — build with `-o` into
  scratch.
- ⚠️ **TWO SELECTOR SLIPS SELF-CAUGHT WHILE WRITING THIS PLAN — both are the CHARACTER-CLASS family, and
  both were FAIL-UNSAFE (a plausible wrong number, not an error).** (i) `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'`
  on `go.mod` reads **62**, not **67** — it silently drops the five modules whose path carries an
  UPPERCASE letter or an underscore (`client_model`, `AdaLogics`, `Azure`, and two `Microsoft`). (ii)
  `grep -oE '[A-Za-z]+ +BackendKind = 38'` prints `GoawayResponder`, not **`H2GoawayResponder`** — a
  **digit-blind** class silently truncating an identifier. **Prefer the structural form** (`awk` over the
  `require (` block; `[A-Za-z0-9_]+` for a Go identifier) **and always run a control that shows what the
  selector MISSED**, not just what it matched.

### 11.3 Contention-harness facts measured at this tip (for §6.5)

`nproc` = **32** · `taskset` = `/usr/bin/taskset` (util-linux 2.39.3) · burner-kill-by-captured-PID
verified with **0 residue**. ⚠️ **`GOMAXPROCS=1` and `taskset -c 0` are NOT interchangeable:**
`GOMAXPROCS=1` caps the P count but leaves `runtime.NumCPU()` at **32**, whereas `taskset -c 0,1` sets
**both** `GOMAXPROCS(0)` and `NumCPU` to 2. **The BRAINSTORM's reproduction used BOTH** (8 burners pinned
with `taskset -c 0`, plus `GOMAXPROCS=1` on the test). **Reproduce both, and say which you used.**

### 11.4 Counts at this tip — RE-DERIVED MECHANICALLY, NEVER COPIED

`ROADMAP.md` **241 lines / 123 data rows**, row 91 `in-progress` at `:153`, check-(2) anchors
`:201 :207 :213 :223 :229 :237`, `-family row` **95 occurrences / 67 LINES**, `gRPC-family row` **2**,
`Operational-tooling-family row` **3**, `esalaine` **4 lines / 7 occurrences** ·
`DECISIONS.md` **18387 LINES** (⚠️ **LINES — `wc -c` reads 4183732**), tail **ADR-0313**, next-free
**ADR-0314**, `^---$` **216**, ⚠️ **`^## ADR-` 312 vs bare `^## ` 320** — the extra **8** are
`## Amendment` headings, and **312 headings vs tail id 0313 means the id space is SPARSE; headings+1
COLLIDES** · strict `PROPOSED` guard **1 at `:18371`**, nearest preceding heading `## ADR-0313` at
`:18369`; the ADR-0231 decoy at `:14866` also reads 1 ·
`BEHAVIOR_CONTRACT.md` **5962 LINES** — ⚠️ **CITE BY STRING OR SYMBOL, never by line** ·
`STATE.md` **63** · `STATE_HISTORY.md` **518** (strict guard **163**; loose **209 LINES / 211
OCCURRENCES**) · `BOOTSTRAP_PROMPT.md` **522** at the REPO ROOT, **ONE tracked path** ·
phase dirs **132** · fixtures **121 dirs / 120 indices**, next-free **`0120` UNCONSUMED** ·
fuzzers **55 / 48 files** · BackendKind **TAIL 38** (39 constants, values 0-38) · `REVIEW.md` **37** ·
production `.go` under `internal/` **373** (764 − 391); tree-wide `.go` **1021** ·
`go.mod:18 golang.org/x/net v0.34.0`; **67** modules · module path **`github.com/pgdad/envoy-go`** ·
`internal/filter/hcm/h2` **26 `.go` = 10 production + 16 test**, test lines **8166**;
`framer.go` **218**, `conn.go` **984**, `client.go` **1013**, `conn_test.go` **1770**,
`framer_test.go` **337**.

### 11.5 What this PLAN must NOT have changed — verify by EMPTY DIFF

`ROADMAP.md` (row 91 STAYS `in-progress`, `want` STAYS **123**) · `DECISIONS.md` (**a PLAN adds no ADR**;
next-free STAYS **ADR-0314**, strict guard STAYS ARMED at **1**, `^---$` STAYS **216**) ·
`BEHAVIOR_CONTRACT.md` · every `.go` file (**ZERO production bytes**) · `go.mod` / `go.sum`.
Phase dirs STAY **132**; the PLAN lands **inside the existing phase directory**.

---

## 12. EXPLICITLY NOT MEASURED AT THIS PLAN — stated so it is never inferred

- ⚠️ **h2spec has STILL not been RUN** at any stage of this row. Its runnability, digest pin
  (local RepoDigest matches `sha256:5f4a65c…a7aeb0` byte for byte), `-S` strict flag, `expectedSuites`
  membership and the `95/94/1/0` baseline were all verified **READ-ONLY**. T3/T12 are its first
  executions here — **together with the mandatory reddening NC.**
- ⚠️ **No prototype was built at this stage either.** The §10.1 figures are **estimates derived from the
  written design**, not measurements. `+244/−60` is an ESTIMATE; the IMPL reconciles against it and is
  expected to differ.
- ⚠️ **The contended arm was NOT run at this stage.** The 6 FAIL/12, 3/12 and 0/12 figures are INHERITED
  from the BRAINSTORM. The only contended-adjacent thing measured here is that
  `TestServerConn_TinyWindowDelivery` runs **5/5 GREEN uncontended at 0.01 s** — **which proves nothing**
  and is precisely the reading that absorbed this defect as a flake three times.
- **The full differential suite was NOT run at this stage**, and the §7.3 race-instrumented subject probe
  is **UNRUN** — it is specified, not executed.
- **The reference side of the framer defect** — no reference-side probe was run at any stage; the
  *"nghttp2 has no deadline-polling read"* claim remains **READ, not measured**.
- **The encode/response direction**, the H/3 leg's own framing, and `readErr` stickiness **under
  concurrent readers** (which §3.1's one-consumer precondition forbids, but does not enforce).
- ⚠️ **`tryReadFrameWait = 2 ms` is a REASONED choice, not a measured one.** Its two bounds are measured
  (the 5 ms `conn_test.go:1201` drip; arm B's 50 ms → 42.4 s inflation), but **no run has yet shown 2 ms
  is sufficient**. §6.5 and §6.6 are what test it, and §3.5 carries the escape hatch.

---

## 13. NEXT

**State 3 -> 4: the phase-91 IMPL** (`superpowers:executing-plans`, or
`superpowers:subagent-driven-development` for the independent tasks — T9/T10 are disjoint from T7/T8 by
pathspec and can run in parallel; **T4 and T5 both edit `framer.go` and MUST be serialized**).

It owes, in order: the two RED anchors **before any production byte moves** (T2, T3), the framer seam and
its wiring (T4-T8), the test roster (T6, T9, T10), the re-measurements (T11, T12), the two disclosures
(T13, T14), the six-gate posture (T15), and the docs (T16).

⚠️ **THE THREE THINGS MOST LIKELY TO GO WRONG, NAMED IN ADVANCE:**
1. **Shipping a green h2spec without its reddening NC.** The package suite is *measurably* blind on this
   axis (204/204 green with the drain gutted), and the gate SKIPs rather than fails when Docker is
   unavailable. A green with neither NC nor SKIP-assertion is worth nothing.
2. **Reading `0 FAIL / 30` as proof.** The base rate is non-stationary. **Interleave the arms** and
   report the mode census, including any third mode.
3. **Quietly widening `tryReadFrameWait` past 5 ms** to make a gate pass. That is a decision reversal
   (§3.5), not a constant edit.
