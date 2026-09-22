# Phase 100 — `listener-filters-timeout-enforce` — SPEC

**Stage:** SPEC (lifecycle **1 -> 2**). Worktree `/home/esa/git/envoy-go/.worktrees/wt-phase-100-spec` off master
`674786bd`, branch `wt-phase-100-spec`. **Governs:** `BRAINSTORM.md` (514 lines) — read for evidence,
re-derived before trusted. Written under the 2026-07-12 standing directive: **no human consulted.**

**The decision in one paragraph.** `Pipeline.Run` will enforce its own deadline with **ONE clock**: when
`timeoutMs > 0` and the peeker can take a read deadline, a `context.AfterFunc` on the pipeline's
`context.WithTimeout` sets the socket read deadline into the past, so a blocked `Peek` can be interrupted
**only after** `ctx` is done — which makes the existing post-`Inspect` `ctx.Err()` check non-nil by
construction whenever the read was cut short. Before `Run` returns, the callback is stopped (or, if it
already fired, waited for) and the deadline is cleared, so no deadline survives into chain selection or the
TLS handshake. `serveConnection` books `listener.<addr>.downstream_pre_cx_timeout` (**+1 NAME**) on every
`context.DeadlineExceeded` from `Run`, under both `continue_on_listener_filters_timeout` values, and keeps
its existing close-under-`false` / fall-through-under-`true` branch. `parseListenerFiltersTimeout` splits
**nil -> 15000** from **explicit zero -> 0 (disabled)**. The shape (**PB1**) is two production files,
`pipeline.go` **`15 0`** and `manager.go` **`14 6`** by `git diff --numstat`; it was built, driven through the
real binary against every arm below, **run 3x50 and 1x300 concurrent drops without losing one**, and
reverted. **The BRAINSTORM's deadline-only race is CONFIRMED** (63 of the same 100 connections lost). Its
one-file `23 6` wall-clock prototype (PA) is also race-free as measured, and is **REJECTED on mechanism,
not size** (§4.2). Two residual divergences are **measured, named, and NOT bought**: the close KIND (§0.1)
and the partial-byte `400` under `true` (§0.2).

---

## 0. What this stage refuted — by execution

Every item was produced by running something — by this stage's three agents (a Docker-only reference
agent, a Docker-free subject agent, a read-only census agent) or by the controller, who re-ran the
load-bearing unit arm first-hand (§3.3). **Nine** corrections to the BRAINSTORM, and two confirmations its
router asked to be re-measured rather than inherited.

### 0.1 🔴 The close KIND is not merely "unmeasured on the subject" — the subject sends FIN where the reference sends RST, under EVERY candidate shape, and the harness cannot see the difference anyway

`BRAINSTORM.md` §0.12 left the subject's kind unmeasured. Measured with a FIN/RST-classifying client
(A4, one byte `0x16` then silent, `continue…: false`): **FIN at 1000 ms on P0, PA, PB, PC and PB0 alike**; the
reference sends **RST at 1002 ms** (R3f), and RST at 1002 ms for the 5-byte header too (N1). **Mechanism
(subject):** `peekerConn` is a `bufio.Reader`, so the peeked byte has already been READ out of the kernel
buffer; `close()` finds no unread data and sends FIN. The reference peeks with `MSG_PEEK`, leaves the byte
unread, and the kernel answers the close with RST. No timeout shape can change this — it lives in the
peeker. **And a second mechanism hides it on the reference side:** through a host `-p` mapping
(docker-proxy), R3f, N1 and R1d **all** read FIN at 1001 ms; only an in-network client sees the RST. The
differential harness publishes with `-p`, so **a fixture cannot observe the reference's RST at all.**
⇒ **Decision (owed item 4): the fixture pins NO close kind**; the divergence is recorded in ADR-0322 and
banked (§14.3).

### 0.2 🔴 Owed item 2 answered: the subject does NOT answer `400` at the deadline on the one-byte `true` arm — under ANY shape — and the cause is outside listener-filter scope

Reference R3t: **`400` at 1002 ms** (`connection: close`), then FIN at **2001 ms** from the HCM's 1 s
delayed-close timer; N2 (the 5-byte header) the same. Subject under PB0 (and every other shape): the
pipeline times out at 1 s, books `pre_cx_timeout` **1**, falls through to `chain_indexed` as `raw_buffer` —
and then the connection sits **open**: a mid-hold scrape at 3 s read `chain_indexed` `downstream_rq_total`
**0** and `rq_4xx` **0**; both went to 1 only after the client closed, and **no byte ever reached the
client**. envoy-go's HTTP/1 codec waits for a line terminator where the reference's parser rejects `0x16`
as an invalid method byte immediately. **This is an HCM codec divergence that exists with no listener
filter at all**; the timeout repair only makes it reachable at the deadline instead of at the client's
next byte. **Not bought; banked (§14.3); the fixture pins no partial-byte `true` arm.**

### 0.3 🔴 "Re-point the `TestParseListenerFiltersTimeout*` row that pins zero -> 15000" — THERE IS NO SUCH ROW

`BRAINSTORM.md` §3.2 and §10 item 3 (and the router) prescribe a re-point. Reading the four tests' inputs
(method note 85): `InRange` uses `5s`, `BelowFloor` `500ms`, `AboveCap` `90s`, and **`Default` builds its
listener with `mkListener`, which leaves the field NIL** — no test anywhere constructs `durationpb.New(0)`.
The subject agent confirmed it by execution: PB0 (with the fold) reads the same **266 `=== RUN`, 0 FAIL,
identical roster** as the tip. **The fold falsifies ZERO existing assertions; it needs a NEW arm that is
RED at the tip** (§5, U3). Obeying the instruction literally would have produced a re-point of a nil test
that asserts something the fold does not change.

### 0.4 🔴 The prototype's natural layout breaks three CITATIONS of `pipeline.go:43` — a hazard absent from the BRAINSTORM's occurrence set

The subject agent's PB0 put an `IsTimeout` helper and two imports (`errors`, `os`) ABOVE `Run`, moving the
`context.WithTimeout` line from `:43` to `:50` and the `OnDestroy` defer from `:33-37` to `:40-44`. Those
lines are CITED: `manager.go:472-479` (*"the one production `context.WithTimeout` under `internal/listener/`
(`listenerfilter/pipeline.go:43`) cannot escape"*), `manager.go:1311` (*"pipeline.go lines 33-37"*),
`BEHAVIOR_CONTRACT.md:1969`, and ADR-0296 at `DECISIONS.md:17272`, `:17298`, `:17319`. §3.6 of the BRAINSTORM
lists none of them. **The claim those citations make stays TRUE under the repair** (no second
`WithTimeout`, `ctx` never rebound, the deadline cleared before `Run` returns) — only the LINE would rot.
⇒ **The layout is part of the decision (§4.1 (e)):** every insertion lands after `defer cancel()`; the
classification uses `errors.Is(err, context.DeadlineExceeded)` in `manager.go`, which already imports both
packages. **Built and measured as PB1**: `:33-37` and `:43` read the same bytes as the tip.

### 0.5 ⚠️ §7.1's "only `0123/README.md` and `0123/driver/driver.go`" is FALSE in detail — the conclusion survives

`git grep -l 'listener_filters_timeout' -- test/` → **zero files** (rc=1). The two `0123` files mention port
`15125` and the ABSENT `listener_filters` key, not the timeout field. **No fixture configures the field** —
the conclusion holds, for a different reason than the one stated.

### 0.6 ⚠️ `REVIEW_FINDINGS.md`'s entry is `:185-188`, not `:185-187`

The fourth line carries the SNI case clause, which **stays true** after this row.

### 0.7 ⚠️ The BRAINSTORM's ledger prescription ("a DELTA, never an absolute") DEPARTS from the last non-zero precedent — decided deliberately here, not inherited

The last non-zero entry, phase 94 (`BEHAVIOR_CONTRACT.md:5140`), reads `1208 → 1209 (+1)` — the absolute
`A → B` form, as do phases 74, 75, 77 and 92. Phases 96-99 wrote `+0, UNCHANGED` with **no absolute**. The
BRAINSTORM chose delta-only without naming that it breaks the `+N` precedent. **§9 decides it**, on the
ground that three mutually inconsistent absolutes are live at one tip (the router's standing rule: on a
contested count, no number).

### 0.8 ⚠️ The floor "`23 6` in `manager.go` only" is the WRONG FILE SET for the chosen shape

PB1 is `15 0` in `pipeline.go` + `14 6` in `manager.go`. The one-file shape exists (PA, `24 6` +
`7 0` for the prototype's shared helper) and is rejected in §4.2 on mechanism. A floor that names one file
encodes a shape decision (method note 37) the BRAINSTORM itself left open.

### 0.9 ⚠️ The reference moves a SECOND pre-connection counter the BRAINSTORM saw only at zero

`downstream_listener_filter_remote_close` "stays 0 on every timeout arm" (BRAINSTORM §2.1) — true — but it
goes to **1** when a client closes or half-closes DURING inspection (N4: half-close at 301 ms → FIN at
301-302 ms, `remote_close` 1, `pre_cx` 0; R4 `0s`: 1 once the held client finally closed). envoy-go
registers no such name (nor `downstream_listener_filter_error`, which the reference also registers at
boot). The subject agrees on the half-close TIMING (FIN at 300 ms on the tip and every shape) and differs
on the stat. **Not bought; banked (§14.3).** ⚠️ One N4 run read `remote_close` **2** for one probe
connection; three reruns read 1 — unexplained, plausibly a foreign connection to the published port.
Recorded, not pinned.

### 0.10 ⚠️ `TestPipelineRunZeroTimeoutDisablesEnforcement` is STRUCTURALLY VACUOUS

Its filter sleeps **10 ms** and the test asserts `Run(…, 0)` returns nil. A 10 ms filter passes under ANY
budget ≥ 10 ms — including the 15 s default the fold is about. It cannot distinguish "0 disables" from
"0 means 15 s". §5 U4(iii) replaces its role with an arm a 15 s budget would fail.

### 0.11 ✅ CONFIRMED, NOT REFUTED — the two claims the router said to re-measure

- **The §0.5 race.** Deadline-only (P0), 50 concurrent silent `false` clients: **16, 21, 13** closed at the
  deadline across three runs (the two-run pair the BRAINSTORM measured: **63 of 100 lost**, against its
  65); at n=300, **133 of 300** closed and the server's fds rose **7 -> 174**. The unit arm (§5 U1) fails
  under P0 on every run the controller made (**3/3**, 8 of 20 fell through each time).
- **The `0s` coupling.** Reference `0s`: **open at 21021 ms**, `pre_cx` 0. Subject PB **without** the fold:
  **FIN at 15014 ms, `pre_cx` 1** — the minted divergence, observed; PB0 **with** it: open at 17015 ms,
  `pre_cx` 0. Absent field: reference FIN at **15002 ms**, subject (PB0) FIN at **15014 ms**.

---

## 1. Scope, restated as a decision

**Bought:** (a) the deadline is enforced — a TCP connection whose listener-filter pipeline has not finished
at `listener_filters_timeout` is CLOSED under `continue…: false` and handed to chain selection (stamped
`raw_buffer` by the existing ADR-0320 entry stamp) under `true`, at the deadline, not at the client's next
act; (b) each such timeout books `listener.<addr>.downstream_pre_cx_timeout` (**+1 NAME**, registered for
every listener at Start, value 0 at boot, as on the reference); (c) an explicit `0s` DISABLES the timeout
while an absent field keeps the 15 s default.

**NOT bought** (each measured, each with a named reason): the `[1s, 60s]` envelope lift (the next row, §14.3);
`downstream_cx_total` moving post-pipeline (§14.3); a close under `true` (the reference parks a silent
connection ≥ 90 s — BRAINSTORM §0.9 — and N5 re-measured the fall-through connection as USABLE: a GET at
3003 ms served `INDEXED`); the close KIND (§0.1); the partial-byte `400` under `true` (§0.2); the
`downstream_listener_filter_{remote_close,error}` names (§0.9); any QUIC path (the only `Pipeline.Run` call is
`manager.go:1350`; QUIC runs no pipeline); and the non-timeout-error gating of `REVIEW_FINDINGS.md:186-187`,
**recorded, not decided** (§10).

**Family attribution:** a Listener / listener-filter **MAINTENANCE** row claiming **NO family ordinal**, on
the row-85-through-99 precedent (BRAINSTORM §5, unchanged).

---

## 2. The reference, MEASURED — re-run at this stage, not inherited

Image `envoyproxy/envoy@sha256:7edd5b0fd763…`, verified against `ENVOY_TARGET.md:3-4`. A fresh container per
arm on the user bridge network `p100spec-ref-net`, `-l debug`, each arm gated on admin `/ready` = `LIVE`
before measuring (method note 51). The probe is a static Go binary in `busybox:1.36` **on the same
network** (a real kernel socket; §0.1 for why that matters), with millisecond timing and an EOF / RST /
still-open classifier. Stats scraped before and after each arm. Base config: BRAINSTORM §2's, verbatim in
shape (one listener, `tls_inspector`, `1s`, `fc[0]` = `transport_protocol: raw_buffer` → body `INDEXED`,
`stat_prefix: chain_indexed`; `default_filter_chain` → `DEFAULT`, `chain_default`); every arm a one-line diff.

| arm | cont | client | observable (kind @ ms) | `pre_cx` | `cx_total` | `lf_error` | `lf_remote_close` |
|---|---|---|---|---|---|---|---|
| R1a | true | GET at 0 | `INDEXED` @ 0 | 0 | 1 | 0 | 0 |
| R1b | true | silent, GET at 2502 | `INDEXED` @ 2503 | 1 | 1 | 0 | 0 |
| R1d | false | silent | **FIN @ 1001** | 1 | **0** | 0 | 0 |
| R3f | false | `16`, silent | **RST @ 1002** | 1 | 0 | 0 | 0 |
| R3t | true | `16`, silent | **`400` @ 1002**, FIN @ 2001 | 1 | 1 | 0 | 0 |
| R4 `0s` | false | silent, 21 s | **open @ 21021** | **0** | 0 | 0 | 1 (after client close) |
| R4 absent | false | silent, 17.5 s | FIN @ 15002 | 1 | 0 | 0 | 0 |
| R5 no filters | false | silent, 5 s | open @ 5005 | 0 | 1 | 0 | 0 |
| N1 | false | `16 03 01 02 00`, silent | RST @ 1002 | 1 | 0 | 0 | 0 |
| N2 | true | same 5 bytes | `400` @ 1002, FIN @ 2001 | 1 | 1 | 0 | 0 |
| N3 | false | GET at 501 | `INDEXED` @ 501 | **0** | 1 | 0 | 0 |
| N4 | false | half-close at 301 | FIN @ 301-302, no bytes | 0 | 0 | 0 | 1 |
| N5 | true | silent, GET at 3003 | `INDEXED` @ 3004 | 1 | 1 | 0 | 0 |
| N6 ×2 | false | **50 concurrent silent** | **50 FIN, 999-1003 ms, both runs** | 50 | 0 | 0 | 0 |
| N7 `0.5s` | false | silent | FIN @ 502 | 1 | 0 | 0 | 0 |

`--mode validate` (N8): `0s`, `0.5s`, `1s` and absent all rc=0; negative control `-1s` rc=1 (`Expected
positive duration`). Log on the timeout path: `[conn_handler] active_tcp_socket.cc:56 listener filter times
out after 1000 ms` (the figure tracks the config: 500, 15000), then **under `true` only**
`active_tcp_socket.cc:59 fallback to default listener filter`. **`downstream_pre_cx_timeout`,
`downstream_listener_filter_error` and `downstream_listener_filter_remote_close` all exist at boot with value
0** on `/stats` and `/stats/prometheus` (`envoy_listener_downstream_pre_cx_timeout{envoy_listener_address=
"0.0.0.0_10000"} 0`), on the admin listener too — **a name-presence pin is vacuous on the reference side
(method note 81)**.

**What the reference establishes for the repair:** (1) the deadline covers INSPECTION only — N3's GET at
501 ms is served and books nothing; (2) the drop is exact under concurrency (N6: every one of 100 inside
999-1003 ms), so a shape that loses any concurrent drop is a divergence, not a tolerance; (3) under `true` the
fall-through connection is live (N5); (4) `0s` disables; absent means 15 s.

**Not run on the reference:** IPv6 and QUIC listeners; any listener filter other than `tls_inspector`; a GET
sent inside N2/R3t's 1 s delayed-close window; **N6 through a host `-p` mapping** (the fixture's path — §7.5
makes it a PLAN measurement).

---

## 3. The subject, MEASURED — the tip and five shapes

A Docker-free agent built each shape as a patch on `674786bd` in a throwaway worktree (created and removed),
drove the real binary on ports `16550-16599`, and saved every patch and result table to the session
scratchpad. Every enforcing shape also carried the same counter (field + registration + an `Inc` on timeout
under both `continue` values), so `pre_cx` is observable on each.

### 3.1 The shapes

| shape | mechanism | `git diff --numstat` |
|---|---|---|
| **TIP** | no deadline; `ctx.Err()` consulted after `Inspect` returns | — |
| **P0** | `raw.SetReadDeadline(start+timeout)` in `serveConnection`; nothing else | `pipeline.go 7 0`, `manager.go 21 6` |
| **PA** | P0 + a wall-clock "deadline passed" check after `Run` (the BRAINSTORM's prototype) | `7 0`, `24 6` |
| **PB** | `context.AfterFunc(ctx, SetReadDeadline(past))` inside `Run`; stop-or-wait, then clear | `22 0`, `10 5` |
| **PC** | P0's deadline + `peekerConn` records a Peek deadline error, `Run` treats it as a timeout | `10 0`, `21 6`, `callbacks.go 12 2` |
| **PB0** | PB + the `0s` fold | `22 0`, `14 6` |
| **PB1** | **PB0 re-laid-out per §0.4** — no helper, no new import in `pipeline.go`; `errors.Is(err, context.DeadlineExceeded)` in `manager.go` (built by the controller) | **`15 0`, `14 6`** |

(The `7 0` in P0/PA/PC and part of PB/PB0's `22 0` is the prototype's shared `IsTimeout` helper + imports,
which PB1 drops; `~+6 -5` of every `manager.go` figure is gofmt re-aligning `listenerRuntime` fields around
the one new field.)

### 3.2 Per-arm — one fresh process per arm, 1 s timeout, 5 s hold unless stated (kind @ client ms / `pre_cx`)

| arm | TIP | P0 | PA | PB | PC | PB0 |
|---|---|---|---|---|---|---|
| A1 true, GET at 0 | `INDEXED` @0 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 |
| A2 true, silent, GET at 2500 | `INDEXED` @2501 | `INDEXED`/**0** | `INDEXED`/1 | `INDEXED`/1 | `INDEXED`/1 | `INDEXED`/1 |
| A3 false, silent | **open @5004** | **open/0** | FIN @1000/1 | FIN @1001/1 | FIN @1000/1 | FIN @1002/1 |
| A4 false, `16` | open | FIN @1000/1 | FIN @1001/1 | FIN @1000/1 | FIN @1000/1 | FIN @1000/1 |
| A5 true, `16` | open | open/1 | open/1 | open/1 | open/1 | **open/1** (§0.2) |
| A6f false, 5-byte header | open | FIN @1000/1 | FIN @1000/1 | FIN @1000/1 | FIN @1000/1 | FIN @1000/1 |
| A6t true, 5-byte header | open | open/1 | open/1 | open/1 | open/1 | open/1 |
| A7 false, GET at 500 | `INDEXED` @500 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 | `INDEXED`/0 |
| A8 false, half-close at 300 | FIN @300 | FIN @300/0 | FIN @300/0 | FIN @300/0 | FIN @300/0 | FIN @300/0 |
| A11a true, silent | open | open/**0** | open/1 | open/1 | open/1 | open/1 |
| A11b true, GET at 3000 | `INDEXED` @3002 | `INDEXED`/1 | `INDEXED`/1 | `INDEXED`/1 | `INDEXED`/1 | `INDEXED`/1 |

`downstream_cx_total` read 1 on every arm (at-accept accounting, §14.3). **P0's single-connection cells are
coin flips** — A2, A3 and A11a lost the race on this sample, A4, A6f and A11b won it: one sample per arm
cannot separate P0 from the race-free shapes (method note 91), which is why the gate is A9.

### 3.3 A9 — THE DISCRIMINATING GATE: 50 concurrent silent clients, `continue…: false`, 5 s hold, every run recorded

| shape / run | closed ≤ 1.5 s | close spread (ms) | `pre_cx` | fds before → at 2.5 s | which path booked the abort (ctx / wall / peek) |
|---|---|---|---|---|---|
| TIP r1 | **0** (50 open) | — | n/a | 7 → 57 | aborts logged only at client close |
| P0 r1 / r2 / r3 | **16 / 21 / 13** | 999-1001 | 16 / 21 / 13 | 7 → 41 / 36 / 44 | all ctx |
| PA r1-r6 | 50 each | 1000-1001 | 50 | 7 → 7 (r4-r6) | 18/32, 10/40, 10/40, 23/27, 16/34, 32/18 (ctx/wall) |
| PB r1-r6 | 50 each | 999-1001 | 50 | 7 → 7 (r4-r6) | **50/0 every run** |
| PC r1-r6 | 50 each | 1000-1002 | 50 | 7 → 7 (r4-r6) | 13/37, 37/13, 10/40, 37/13, 11/39, 10/40 (ctx/peek) |
| PB0 r1-r3 | 50 each | 1000-1001 | 50 | 7 → 7 | 50/0 every run |
| n=300, one run | P0 **133**, PA / PB / PC **300** | PA 987-1019, PB 986-1014, PC 988-1018 | = closed | P0 7 → 174, others 7 → 7 | PA 165/135, **PB 300/0**, PC 133/167 |

(The first three runs of PA/PB/PC hit a probe bug that skipped the fd sample when every connection closed
before 2.5 s; the probe was fixed and three more runs taken — both sets are listed, none discarded.)

**The unit form of the gate, re-run FIRST-HAND by the controller** (§5 U1, 20 concurrent silent clients
through a manager-built listener with the real `tls_inspector`): **TIP** RED, `got map[open:20]` in 3.00 s ·
**PB0** GREEN **3/3 under `-race`**, 1.00 s each · **P0** RED **3/3**, `map[closed:12 fellThrough:8]` each run ·
**PB1** GREEN **3/3 under `-race`**. The subject agent's own runs agree (TIP 2/2 RED; P0 5/5 RED with 3-8 of 20
fallen through; PA, PB, PC, PB0 5/5 GREEN).

### 3.4 A10 — the `0s` fold (17 s hold, `continue…: false`, one run each)

| shape | `0s` | absent |
|---|---|---|
| TIP | open @ 17015; abort logged when the client closed (lazy 15 s) | the same |
| PB (no fold) | **FIN @ 15014, `pre_cx` 1** — the minted divergence | FIN @ 15014, `pre_cx` 1 |
| PB0 | **open @ 17015, `pre_cx` 0** — the reference's answer | FIN @ 15014, `pre_cx` 1 |

### 3.5 The existing suite is BLIND — CONFIRMED

`go test -count=1 -v ./internal/listener/...`: **TIP, P0, PA, PC and PB0 each read 266 `=== RUN`, 0 FAIL,
identical sorted rosters** — including P0, a racy patch that changes behaviour (method note 50). PB1 plus the
U1 arm: **267 `=== RUN`, 0 FAIL** (controller). Every existing timeout test installs the ctx-aware
`installSlowListenerFilter` stub (a `select` on `ctx.Done()`), which returns at the deadline on its own, so
none of them can see whether the PIPELINE enforces anything. **Named as the selector with the figure:**
this `266` is `./internal/listener/...` only, not the router's five-selector `421`.

---

## 4. The production edit — DECIDED

### 4.1 The shape (PB1)

**(a) `internal/listener/listenerfilter/pipeline.go` — `15 0`.** Inside the existing `if timeoutMs > 0 {`
block, immediately after `defer cancel()`:

```go
		// ONE clock: the socket read is interrupted only AFTER ctx is done, so
		// an interrupted Peek always observes ctx.Err() != nil below.
		if ds, ok := peeker.(interface{ SetReadDeadline(time.Time) error }); ok {
			fired := make(chan struct{})
			stop := context.AfterFunc(ctx, func() {
				_ = ds.SetReadDeadline(time.Unix(1, 0))
				close(fired)
			})
			defer func() {
				if !stop() {
					<-fired
				}
				_ = ds.SetReadDeadline(time.Time{})
			}()
		}
```

`peekerConn` embeds `net.Conn`, so the production peeker satisfies the assertion; a stub peeker without
`SetReadDeadline` keeps today's behaviour (the ctx-aware stubs in the suite are unaffected). The deferred
function runs BEFORE `cancel()` (LIFO), so `stop()` races only the deadline itself, never the explicit
cancel; if the callback already started, the `<-fired` wait guarantees it cannot set the deadline AFTER the
clear and poison the handed-off connection.

**(b) `internal/listener/manager.go` — `14 6`.** A `downstreamPreCxTimeout *stats.Counter` field on
`listenerRuntime`; `rt.downstreamPreCxTimeout = r.NewCounter(prefix + "downstream_pre_cx_timeout")` in
`registerListenerMetrics`, **unconditionally** beside `downstream_cx_total` (the reference registers it on
every listener, filters or not, admin included); in `serveConnection` step (4), before the `continue`
branch:

```go
		if errors.Is(err, context.DeadlineExceeded) {
			rt.downstreamPreCxTimeout.Inc()
		}
```

and `parseListenerFiltersTimeout` split: `if d == nil { return defaultMs, nil }` then
`if d.GetSeconds() == 0 && d.GetNanos() == 0 { return 0, nil }` — the envelope check below it UNCHANGED.

**(c) Help text, as a PAIR in one commit:** `internal/stats/name.go` `helpText` gains
`"envoy_listener_downstream_pre_cx_timeout"` beside `"envoy_listener_downstream_cx_total"`, and
`helptext_test.go` `helpTextRoster` gains `{internal: "listener.0_0_0_0_10000.downstream_pre_cx_timeout"}` —
`TestHelpText_KeySetExact` enforces exact set equality between the two, so adding either alone REDDENS it
(§9).

**(d) Comments falsified by (a)-(b)** — the §6 set, each edited under the constraints in (e).

**(e) LAYOUT CONSTRAINTS — mechanically gated, not advisory (§0.4):**
- `pipeline.go` lines **1-44** keep their line count: **nothing is inserted above `defer cancel()`**, and
  any edit to `Run`'s doc comment is line-count-neutral. Gate: `sed -n '33,37p;43p'` reads the same bytes
  before and after (the `OnDestroy` defer and the `context.WithTimeout` line), plus `git diff` of the file
  showing a single hunk starting after `:44` or a hunk above it of shape `N N`.
- **No new import in `pipeline.go`** (the classification lives in `manager.go`, which already imports
  `errors` and `context`).
- `tls_inspector.go:55-57` (*"ctx is not plumbed into the socket read"*) is edited **comment-only at shape
  `3 3`** (method note 80).
- No `context.WithTimeout` is added anywhere under `internal/listener/`; `serveConnection` does not rebind
  `ctx`. Gate: `git grep -n 'context.WithTimeout' -- internal/listener/ ':!*_test.go'` reads exactly the one
  `pipeline.go:43` hit before and after.

### 4.2 Rejected shapes — decided on MECHANISM, never on size (method note 37)

- **P0 (deadline only) — REJECTED, measured wrong.** Two clocks set to one instant; when the socket wins,
  `tls_inspector` maps the zero-byte error to `raw_buffer` / `Continue, nil` and the pipeline reports
  success. Lost 13-21 of 50 on every run and 167 of 300.
- **PA (deadline + wall-clock check; the BRAINSTORM's `23 6`) — REJECTED although race-free as measured.**
  It keeps two clocks and repairs the race after the fact: 18-80 % of its aborts are booked by the
  wall-clock branch, not the context. It enforces the timeout OUTSIDE the pipeline, so `Pipeline.Run`'s own
  contract ("a single `context.WithTimeout` … shared across all filters' `Inspect` calls") stays unenforced
  for any caller but `serveConnection`, and ADR-0082 §Decision ¶3 stays false as a statement about the
  pipeline. It duplicates the timeout arithmetic in the manager, and its false-positive window (an
  inspection that completes just after the deadline is dropped even though no read was cut) is a second
  semantic on top of the reference's.
- **PC (deadline + peeker-recorded timeout) — REJECTED.** Also race-free as measured and also two clocks
  (26-80 % booked by the peeker flag); touches three files including `callbacks.go`; its error wraps
  `os.ErrDeadlineExceeded` rather than the context's, so the timeout would be classified by two error
  identities.
- **"Return the deadline error from `tls_inspector`" — REJECTED unbuilt.** It breaks the filter's documented
  never-errors contract (five returns, all `Continue, nil`) and makes every future listener filter
  responsible for classifying the socket error correctly; still two clocks.
- **PA, PB and PC are observably INDISTINGUISHABLE** on every client-visible surface measured (timing, kind,
  bodies, `pre_cx`, fds). The choice is PB's single clock and the fact that it makes the pipeline's own
  documented contract true — **not** its size, though it is also the smallest `manager.go` edit.

### 4.3 The declared behaviour changes

| config / client | tip | after | reference |
|---|---|---|---|
| filters, `false`, silent (or partial bytes) | held until the client acts; abort logged then | **closed at the deadline**, `pre_cx` +1 | closed at the deadline |
| filters, `true`, silent then sends | served at the send (no timeout ever fires) | falls through AT the deadline, served at the send, `pre_cx` +1 | the same |
| filters, `true`, silent forever | held | held (fell through at the deadline) | held ≥ 90 s |
| explicit `0s`, client sends at 16 s | **EOF** (lazy 15 s abort) | **served** | served (never times out) |
| absent field, `false`, silent | held until the client acts | closed at 15 s | closed at 15 s |
| filters, manager `ctx` canceled during inspection | held until the client acts | the AfterFunc fires on the parent cancel too: `Run` returns `context.Canceled` → closed under `false`, falls through under `true`, **no `pre_cx` Inc** | — (not measured; mechanism-derived) |
| no listener filters | unchanged (`Run` returns at `len(filters) == 0`) | unchanged | unchanged |

The `0s` row is a **closed → served** change on a config that validates and boots on both sides, and it is
the parity direction (method note 87). The shutdown row is derived from the mechanism, not measured — §5 U5
pins its stat half.

---

## 5. Unit-test design (owed item 5)

Every arm below drives the **REAL** peek path over **loopback TCP** (never `net.Pipe`), states which way it
reads at the tip, and names its own NC (§11).

| id | where | arm | tip | PB1 |
|---|---|---|---|---|
| **U1** | `internal/listener` | 20 concurrent silent clients, real `tls_inspector`, manager-built listener, `1s`, `false`; each client reads with its own 3 s deadline and is classified `closed` (EOF/RST, 0 bytes, < 2 s) / `fellThrough` (bytes) / `open` (client deadline); assert all 20 `closed` and none before 900 ms | **RED** (`open:20`) — measured | GREEN 3/3 `-race` — measured |
| **U2** | `internal/listener` | `TestUnifiedDispatchListenerFilterTimeoutAbortsConnection` **strengthened in place**: keep the `'A'`-byte fall-through branch AND add "the SERVER closed before 2 s" (EOF/`ECONNRESET` with `n == 0`), so a client-deadline expiry at 3 s FAILS | GREEN (the stub is ctx-aware, so the tip's pipeline aborts it) | GREEN |
| **U3** | `internal/listener` | new `TestParseListenerFiltersTimeoutZeroDisables`: `durationpb.New(0)` → `lfTimeoutMs == 0`; `Default` (nil → 15000) stays as is | **RED** (15000) — by mechanism | GREEN |
| **U4** | `listenerfilter` | `Run` with a real `peekerConn` over a loopback pair and a NON-ctx-aware test filter that only `Peek(5)`s and ignores errors (the inspector's discipline; the package cannot import `tls_inspector`): **(i)** silent peer, `timeoutMs` 200 → error wrapping `context.DeadlineExceeded` within `[150, 1000]` ms; **(ii)** peer sends 5 bytes at 20 ms → `Run` returns nil; after sleeping past 200 ms, a `Read` of further peer bytes succeeds (no stale deadline); **(iii)** `timeoutMs` 0, silent peer → `Run` has NOT returned after 400 ms (then the peer closes) | (i) **RED** (returns only when the test's own timer closes the peer); (ii) GREEN; (iii) GREEN | all GREEN |
| **U5** | `internal/listener` | `pre_cx` by VALUE through the registry **by name** (a test naming the struct field would not compile at the tip): real `tls_inspector`, `1s`, `true`: an immediate send leaves `listener.<addr>.downstream_pre_cx_timeout` at **0**; a client silent 1.3 s then sending moves it to **1** and is served; a manager-`ctx` cancel during inspection moves it by **0**; assert the name EXISTS before traffic (a MISSING name is a hard failure, never read as zero — method note 46) | **RED** (name absent) | GREEN |
| **U6** | `internal/stats` | the `helpText` / `helpTextRoster` pair (§4.1 (c)) | n/a (new entries) | GREEN; either half alone → RED |

**U1's code is MEASURED, not sketched** — built and run at the tip, under P0, PA, PB, PC, PB0 and PB1. It uses
only helpers that exist at the tip (`startTaggedBackend`, `mkClusterMgr`, `mkTcpProxyFilter`,
`mkTLSInspectorFilter`, `mkBoot`, `NewManagerWithBaseDirAndAllowH2C`, `testHTTPRegistry`, `testLFRegistry`,
`testNetRegistryWithTerminals`). Its full text is `§A` below; the PLAN inherits it verbatim. U2-U6 are
**specified, not yet built** — the PLAN builds and runs each at the tip and under PB1 before writing it into
a task (the phase-99 PLAN discipline, method note 89).

**Deliberately NOT added:** a unit close-KIND pin (§0.1 — FIN on every shape, RST on the reference: a pin
would lock in a divergence); a partial-byte `true` arm (§0.2 — HCM scope); a timing-exact assertion (every
window is stated as a range, §7.4).

**Pre-fix GREEN arms, stated per arm (method note 61):** U2 (ctx-aware stub), U4 (ii)/(iii), and U5's
immediate-send half (reads 0 either way — which is why U5 asserts name EXISTENCE first). Their
falsifiability is the NC roster, scored per arm.

---

## 6. Occurrence set of every falsified claim (owed item 9)

Re-derived case-insensitively across code comments (tests included) and `docs/` + repo-root `*.md`, with
four matchers (topical, claims, listener-code, line-cite) plus the router's suggested one for growth; every
docs hit resolved to its ADR or section by backward heading search. Full census in the census agent's
report; the classified set:

### 6.1 FALSIFIED by this row — the IMPL edits these

| site | claim | edit |
|---|---|---|
| `manager.go:166-167` (`listenerRuntime` field doc) | *"lfTimeoutMs is in [1000, 60000] (ADR-0082); default 15000."* | 0 (disabled) becomes legal; keep the envelope half |
| `manager.go:948-950` (`parseListenerFiltersTimeout` doc) | *"nil/zero defaults to 15000ms"* | nil → 15000; explicit zero → 0 = disabled |
| `manager.go:1295-1297` (`serveConnection` step (4) doc) | *"on error, honor `continue_on_listener_filters_timeout`"* | true now for a silent client; name the counter |
| `pipeline.go:27-28` (`Run` doc) | *"On context-deadline-exceeded after a filter's Inspect returns"* | the read is now interrupted at the deadline — **line-count-neutral** (§4.1 (e)) |
| `tls_inspector.go:55-57` | *"ctx is not plumbed into the socket read"* | the pipeline now interrupts it by deadline; a deadline error still classifies `raw_buffer` — **shape `3 3`** |
| `BEHAVIOR_CONTRACT.md:4359` (`### Dispatch protocol`, under `## Listener filters`) | *"default 15s; honored in [1s, 60s] envelope; `continue_on…` honored as proto-documented"* | enforced by one clock; absent → 15 s; explicit `0s` disables; the envelope stated as envoy-go's own; the counter named |
| `REVIEW_FINDINGS.md:185-188` (`## Deferred — needs differential/reference verification before landing`) | *"`listener_filters_timeout` never enforced …"* | annotate the timeout clause as fixed at phase 100; **the non-timeout-gating clause stays (§10) and the SNI-case clause stays true** |
| ADR-0082 §Decision ¶1-3, §Consequences (b) (`DECISIONS.md:3052-3068`) | *honored*; *(zero-valued duration)* → 15 s; *enforced*; "test scaffolding … may pass 0" | **SUPERSEDED BY ADR-0322, not edited** — ADR-0082 is an old-form `**Status:** Accepted` ADR |

### 6.2 Left, and why

| site | reason |
|---|---|
| `manager.go:959` error text, `manager_test.go:3339-3367`, ADR-0082 §Consequences (a) and (c), `BEHAVIOR_CONTRACT.md:4338` | the `[1s, 60s]` envelope — **falsified only by the NEXT row** |
| `manager.go:829-830` | *"default 15000ms; envelope [1000, 60000]"* — still true of nil |
| `manager.go:472-479`, `:1311`, `BEHAVIOR_CONTRACT.md:1969`, ADR-0296 (`DECISIONS.md:17272`, `:17298`, `:17319`) | **STAY TRUE — by the layout gate of §4.1 (e)**; no second `WithTimeout`, no rebind, the deadline cleared before `Run` returns, and `:33-37` / `:43` unmoved |
| `listenerfilter/pipeline.go:12-25` (`timeoutMs == 0` no-op; per-pipeline budget), `doc.go:17-18` | stay true; `0` is now also reachable from config |
| ADR-0320 (`DECISIONS.md:19233-19356`), `BEHAVIOR_CONTRACT.md:5146` (phase-98 ledger), ADR-0078 `:3221` | PAST-TENSE records of their own phases |
| `phases/07.2`, `74`, `98`, `99`, `100` documents | historical stage artifacts, never edited |
| `integration_test.go:50-57`, `manager_test.go:3998-4035` stub docs | accurate descriptions of the ctx-aware stub; U2 strengthens the ASSERTION, not the stub |
| `next-prompt.txt` | the router — rolled at every close, not an occurrence-set edit |

### 6.3 Set-difference against the byte-untouched roster (method note 62)

The IMPL's byte-untouched roster must NOT contain `pipeline.go`, `tls_inspector.go`, `manager.go`, `name.go`,
`helptext_test.go`, `BEHAVIOR_CONTRACT.md` or `REVIEW_FINDINGS.md`; it MUST contain ADR-0082's and ADR-0296's
text (edited by supersession only) and `ROADMAP.md`'s six sentinel windows.

---

## 7. Differential fixture `0125-listener-filters-timeout` — CHARTERED (owed item 6)

### 7.1 Shape

A `MultiListenerDriver` in the `0123` mould (bootstraps rendered in Go, no PKI, no `inputs/`), each listener a
ONE-LINE diff from the BRAINSTORM §2 base (`tls_inspector`, `1s`, `fc[0]` `raw_buffer` → `INDEXED`, default →
`DEFAULT`), `BackendCount() 1` with an unused placeholder cluster.

| listener | delta | reference port |
|---|---|---|
| `l_false` | `continue…: false` | **15125** |
| `l_true` | `continue…: true` | **15228** |
| `l_true_tls` | `true`, and `fc[0]` matches `tls` instead of `raw_buffer` (the R1c matched negative) | **15229** |
| `l_zero` | `listener_filters_timeout: 0s`, `false` | **15230** |
| `l_nofilt` | no `listener_filters`, `false` | **15231** |

### 7.2 Ports — CENSUSED

`git grep -n '\b15125\b' -- test/ internal/ cmd/` → **2** hits, both **reserving PROSE, not binds**
(`0123/README.md:294`, `0123/driver/driver.go:116`). `15228`-`15233` → **0** hits each. `15223`-`15227` are
held by `0123`/`0124` (off-convention), so the four extra ports continue that band. ⚠️ **This section spells
the ports; it is a hit in the next census. Re-census at the PLAN.**

### 7.3 Arms, pins, and which are RED at the tip

| arm | listener | client | pin | tip |
|---|---|---|---|---|
| F1 | `l_false` | silent, hold 3 s | server closed in **[700, 1800] ms**, **0 bytes** | **RED** (open at 3 s) |
| F2 | `l_false` | **50 concurrent silent**, hold 3 s | **all 50** closed in [700, 1800] ms, 0 bytes each | **RED** — and RED under P0 (the race gate) |
| F3 | `l_false` | GET at 300 ms | body `INDEXED` | green (structural) |
| T1 | `l_true` | GET at 0 | body `INDEXED` | green — the `pre_cx` MIRROR |
| T2 | `l_true` | silent 2500 ms, then GET | body `INDEXED` | green (structural — method note 61) |
| T3 | `l_true_tls` | silent 2500 ms, then GET | body `DEFAULT` | green (structural) |
| Z1 | `l_zero` | silent, hold 17 s | **still open at 16.5 s** | green (structural: the tip holds lazily) — falsified only by NC3 |
| N1 | `l_nofilt` | silent, hold 3 s | still open at 2.8 s | green (structural, mechanism on both sides) |
| S | all five | — | `downstream_pre_cx_timeout` **by VALUE**, per side's own address label on `/stats/prometheus`: `l_false` **51**, `l_true` **1**, `l_true_tls` **1**, `l_zero` **0**, `l_nofilt` **0**; a **MISSING** series is a hard failure, never read as 0 | **RED** (name absent at the tip) |

Z1 runs concurrently with the other arms on each side so the fixture's wall time grows by ~17 s once, not per
arm. The `l_false` value **51** is `F1 + F2`, with F3 contributing 0 (N3's measured semantics) — an exact
equality, not a floor (method note 7g).

### 7.4 What the fixture must NOT pin

**`downstream_cx_total` on any listener with a drop arm** (the reference counts post-filter: R1d reads 0,
the subject 1 — structurally off by one per dropped connection, BRAINSTORM §0.10); **a close KIND** (§0.1 —
and docker-proxy converts the reference's RST to FIN anyway); **a partial-byte `true` arm** (§0.2);
**a half-close arm** (§0.9 — agreement on timing, the reference's stat has no subject name); **an exact
millisecond** (windows only — the reference spreads 999-1003 ms concurrently, the subject 986-1019 ms at n=300);
**any `downstream_listener_filter_*` name** (subject emits none — a cross-side pin would be vacuous, method
note 46).

### 7.5 Owed measurements for the PLAN

- **F2 on the REFERENCE through the harness's host `-p` publishing** — N6 was measured in-network only. If
  docker-proxy's accept path smears the 50 closes outside the window, widen the window with the measured
  spread, never drop the arm.
- The window `[700, 1800]` against both sides' measured spreads, stated with the σ-margin method of
  `reference_differential_band_sigma_margin`.
- The silent-client Drive hook: the driver receives a plain `host:port` (`fixture.MultiListenerDriver`,
  `test/differential/fixture/fixture.go:725`), so it dials and times the connection itself. **No existing
  fixture drives a silent client or times a server close** — the nearest precedents are `0008`'s raw dial
  (`driver.go:370-393`) and `0045`'s `closed_no_bytes` classifier (`:320-362`).

### 7.6 Registration and cost

All four gates (method note 60): `RegisterFixture` in `init()`; the blank import in
`test/differential/runner_test.go`; byte-identity of the registered string and the directory name; the
`NNNN-` shape. **Floor by SHAPE:** `0123` (plaintext, PKI-free, multi-listener, Go-rendered bootstraps) landed
at **+1361** in the fixture directory plus **+1** in `runner_test.go` (`git show --numstat --format=
884e4b7c`: README 334, `driver.go` 733, `expectations.yaml` 294). `0125` has more arms, a concurrency arm and a
timing classifier ⇒ **≥ ~1360, a FLOOR** (`reference_measured_prototype_is_a_lower_bound`). `0124` (**+941**,
TLS with `pki/gen`) is not the shape.

---

## 8. `ADR-0322` — §Context drafted HERE (owed item 8)

Appended to `DECISIONS.md` at this stage in the house block form: the `> **STATUS: PROPOSED` blockquote, a
`### Context (drafted at the phase-100 SPEC)` section of seven paragraphs, and the RETAINED italic footer;
no `**Status:**` line, no `---`. **It RE-ARMS the house guard** — verified by line and by ADR in §12. Its
§Context carries the measured reference table's decisive rows (method note 21), the race, the chosen
mechanism and the rejected ones, the `0s` coupling, the residual divergences, and what it does not decide.
**It SUPERSEDES ADR-0082 §Decision ¶1's "honored" and "(zero-valued duration)" clauses, ¶2 as a statement of
behaviour, ¶3's "enforced", and §Consequences (b)'s restriction of `0` to test scaffolding; it LEAVES
ADR-0082's envelope — ¶1's `[1s, 60s]` and §Consequences (a), (c) — to the next row, and KEEPS the
per-pipeline shared budget.** It NOTES ADR-0296 (whose `pipeline.go:43` claim stays true under the layout
gate) and ADR-0320 (whose "NOT ENFORCED" record becomes history).

---

## 9. `BEHAVIOR_CONTRACT.md` ledger — **+1 NAME, CONFIRMED BY MEASUREMENT**

**What increments:** `listener.<normalized-addr>.downstream_pre_cx_timeout`, a counter, `Inc` once per
`context.DeadlineExceeded` from `Pipeline.Run`, under both `continue…` values. **What non-test Go file names
it:** at the tip, **none** (`git grep -n 'pre_cx' -- '*.go'` → rc=1); under PB1, `internal/listener/manager.go`
only (registration). **What registry the repair touches:** the process `stats.Registry`, via
`registerListenerMetrics` at Start (post-bind, pre-Freeze), **unconditionally per listener** — so a
no-filter listener carries the name at value 0, as the reference's does (R5 read `pre_cx` 0 on a no-filter
listener). **Measured by value on the real binary** on every arm of §3.2 (0 on A1/A7/A8, 1 on each timeout
arm, 50 after A9) — ⚠️ **a NAME-presence pin is vacuous on the reference (boot registration, value 0), so
only the value with its mirror arm counts.**

**Guards the new name touches:** `TestHelpText_KeySetExact` (exact set equality between `helpText` and
`helpTextRoster` — **reddens if EITHER half is added alone; green when both are**); nothing else. The
listener-metrics tests check MEMBERSHIP (`listener_test.go:79`, `manager_test.go:2645`, `:2744`, `:2788`) or
the `ssl.`-prefixed exact set (`listenerSSLNames`), the admin tests build their own registry, no golden file
carries a listener name, and the `TestNoNewStat_*` guards cover sinks only (census, confirmed at PB0/PB1 by the
unchanged-green listener suite).

**The entry's FORM — DECIDED:** the IMPL appends after the phase-99 entry (`:5148`) a
`**Phase 100 — +1 (… downstream_pre_cx_timeout) …:**` entry in the **delta-only** form: it states `+1`, names
the one name, and **quotes NO absolute**, departing deliberately from the phase-74/75/77/92/94 `A → B (+N)`
form (§0.7). Reason: three mutually inconsistent stat-surface absolutes are live at one tip, and on a
contested count the answer is no number; the delta is what the row asserts and what a guard can check.
The entry says so in its own text, citing phase 94 as the form it departs from.

---

## 10. The non-timeout-error gating — RECORDED, NOT DECIDED (owed item 7)

`REVIEW_FINDINGS.md:186-187`: *"`continue_on_listener_filters_timeout` wrongly gates non-timeout errors."*
**True of the code** — `serveConnection` branches on ANY `p.Run` error. **NOT CONSTRUCTIBLE for a timeout-free
error today**, by a named mechanism: `tls_inspector` is the only `ListenerFilter` (the sole non-test
registration is `internal/boot/boot.go:75`) and its five returns never carry an error, so `Run` errors only
from `ctx.Err()`. **After PB1 one non-timeout error becomes constructible:** a manager-`ctx` CANCEL during
inspection (the AfterFunc fires on the parent cancel) returns `context.Canceled` — which the `true` branch
would treat as a timeout fall-through. That path runs only at shutdown, books no `pre_cx` (U5 pins that), and
is recorded in ADR-0322 §Context ¶7. **Deciding the gating waits for a second listener filter** that can
error on its own.

---

## 11. Negative-control roster the PLAN inherits — neutralise, never revert

Each row names the mechanism that carries its mutation to a failure (method note 7d); each is scored PER ARM.

| NC | mutation (package still compiles) | must redden | must NOT redden |
|---|---|---|---|
| NC1 | PB → P0: drop the AfterFunc block, set `raw.SetReadDeadline(start+timeout)` in `serveConnection` | U1 (measured RED 3/3 controller, 5/5 agent), fixture F2 | U3, U6 |
| NC2 | delete `rt.downstreamPreCxTimeout.Inc()` | U5's silent arm, fixture S (`l_false`, `l_true`, `l_true_tls`) | U1 (closes still happen), F1, F2 |
| NC3 | revert the `0s` split (zero → 15000) | U3; fixture Z1 **only because Z1 holds past 15 s** | U1, U4, U5 |
| NC4 | delete the deferred `ds.SetReadDeadline(time.Time{})` | U4 (ii) | U1, U3 |
| NC5 | skip the `<-fired` wait | **NONE deterministically** — a narrow race; declared blind, not faked (method note `reference_nc_roster_rows_can_be_structurally_vacuous`) | — |
| NC6 | delete `_ = pkConn.Close()` in the abort branch | U2 (strengthened) and U1; **U2's OLD assertion stays green under it** (its client deadline satisfies it) — measure both | U3, U5 |
| NC7 | `errors.Is(err, context.DeadlineExceeded)` → `err != nil` | U5's cancel arm | U1, F1 |
| NC8 | invert: book `pre_cx` only under `true` | fixture S `l_false` (51 → 0) | T-arms' bodies |

NC6's "old assertion stays green" is the §3.2 vacuity made executable: the PLAN runs the OLD and NEW U2 under
NC6 side by side and records both.

---

## 12. Counts, re-derived at THIS stage's own tip

### 12.1 Moved by this stage

- `DECISIONS.md` **19487 -> 19509** (`22 0`, the ADR-0322 append); `^## ADR-` **320 -> 321**; bare `^## `
  **328 -> 329**; tail **ADR-0322**; next-free **ADR-0323**; `^---$` **216, UNMOVED**.
- The house guard `^> \*\*STATUS: PROPOSED` **RE-ARMED**: it hits the new status line, which resolves by
  backward heading search to `## ADR-0322`; the ADR-0231 decoy (`^\*\*Status:\*\* PROPOSED`) still hits
  `:14866`, resolving to `## ADR-0231`, byte-untouched.
- `STATE.md` rolled IN PLACE; `STATE_HISTORY.md` **588 -> 590** (`2 0`); `SPEC.md` new.

### 12.2 NOT moved

`ROADMAP.md` **250** lines / **132** data rows, row 100 `in-progress` at `:162` (a SPEC neither adds nor flips a
row); `BEHAVIOR_CONTRACT.md` **5998**; `REVIEW_FINDINGS.md`; every `.go` file; fixtures **126 = 126**;
phase dirs **141**. **None of the six gates was run — a SPEC's scope, not an omission**; the one standing
departure is no `REVIEW.md` (none of 93-99).

### 12.3 The split gate (BOOTSTRAP §6.1) — for the PLAN to evaluate

Production **`29 6`** measured (PB1, plus ~12-20 comment lines under §6.1's constraints); unit tests U1 (~110,
measured) + U2-U6 (~200, estimated); the fixture **≥ ~1360** (floor by shape). **A raw added-lines reading
lands near or above ~1500, almost all of it fixture** — the phase-98 precedent carried a `+1362` fixture in
an unsplit 20-task PLAN. **The PLAN must state its accounting and evaluate the gate; this SPEC does not
pre-decide a split.**

---

## 13. Sentinel — RUN MECHANICALLY AT THIS STAGE's TIP, `/usr/bin/grep`

Commands copied verbatim from `next-prompt.txt`, run in the stage worktree at session start (`674786bd`) and
again in the publishing tree after every edit of this stage (a SPEC touches no `ROADMAP.md` byte, so both
runs must agree):

(1) **ONE** — `NOT DONE: row 100` · (2) **SIX** at `:210 :216 :222 :232 :238 :246` · (3) **SILENT** · NC-A
**TWO** (`NOT DONE: row 62`, `NOT DONE: row 100`; substitution inspected first: `NC LANDED? [ in-progress ]`) ·
NC-B **TWO** at `want=131` (`NOT DONE: row 100`, `GATE FAIL: examined 132 data rows, expected 131`) · NC-C
**FIRED** (residual 0) · NC-D **96 / 68** under `--` · check-(2) positive control **6 substitutions ASSERTED,
residual 0** · escape-aware malformed set exactly **{57, 69}** at file lines **119** (NF 9) and **131** (NF
10) · row 100 **8** fields under BOTH forms. Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f |
md5sum`, first 12 hex): `210 10d7807bf02d` · `216 4a92f7e62fc6` · `222 2a7eb298b9fd` · `232 242e53c6f7a3` ·
`238 b2680e6f4fbf` · `246 6caa1c3ce0e7` — **byte-identical to the phase-100 BRAINSTORM close.**
⇒ **THE SENTINEL DOES NOT FIRE. `stop` was evaluated and NOT created.**

---

## 14. Probe hygiene, and what this stage banked

### 14.1 Hygiene

- **Worktrees:** stage `wt-phase-100-spec`; the subject agent's throwaway `p100spec-subj-probe` and the
  controller's `p100spec-verify` (detached) were **created and removed**, verified by `git worktree list`.
- **Docker:** the reference agent only — containers and network `p100spec-ref-*`, torn down BY NAME
  (`docker ps -a --filter name=p100spec-ref-` → header only); no other container touched.
- **Ports:** band `16500-16599`, censused first (`git grep -In '165[0-9][0-9]' -- test/ internal/ cmd/` → two
  hits, both hex constants in `internal/cluster/hash.go`, not ports; `ss -tan` / `ss -uan` → nothing bound).
  Reference `16500` (admin) and `16501` (the host-side `-p` comparison); subject `16550-16599`. ⚠️ **This
  paragraph spells band numbers; it is a hit in the next census.**
- **Scratch:** everything under the session scratchpad (`ref/`, `subj/`, `census/`, and the controller's
  `patch-PB1.diff` / `unit-PB1.log`); nothing in any worktree. **No production `.go` written.**

### 14.2 The standing departure

No `REVIEW.md` (none of 93-99) — not this stage's to fix, named not claimed. `golangci-lint` (under
`GOTOOLCHAIN=go1.26.2`) and gate (b) are **not** departures.

### 14.3 Banked by this stage — NOT chartered

- **Close KIND under `false`** (§0.1): the reference sends RST when a peeked byte is unread; envoy-go's `bufio`
  peeker consumes it and sends FIN. Repair lives in the peeker (a linger-0 close when the buffer holds unread
  bytes, or `MSG_PEEK`). **Invisible to the differential harness** through docker-proxy — needs an in-network
  probe or a unit arm.
- **HTTP/1 codec does not reject an invalid method byte until a line terminator** (§0.2): the reference answers
  `400` at once (then a 1 s delayed close); envoy-go waits. Exists with no listener filter; an HCM row.
- **`downstream_listener_filter_remote_close` and `downstream_listener_filter_error`** (§0.9): registered at boot
  on the reference, absent on envoy-go; `remote_close` moves on a client close or half-close during inspection.
- **The `[1s, 60s]` envelope lift** — unchanged from BRAINSTORM §4.1; **now UNBLOCKED once this row lands**
  (the reference also validates `0.5s` rc=0, N8).
- **`downstream_cx_total` post-filter accounting** — unchanged from BRAINSTORM §4.2.

---

## 15. What the PLAN owes

1. **Build and run every code block before embedding it** (method note 89): U2-U6, the fixture, every NC row of
   §11 — at the tip and under PB1, in throwaway worktrees.
2. **Measure F2 on the reference through the harness's `-p` path** and set the §7.3 windows from BOTH sides'
   measured spreads (§7.5).
3. **Gate the layout** of §4.1 (e) mechanically, with each gate shown to FIRE on an input known to trip it
   (method note 7i).
4. **Order the spine** so the un-fixed tip is recorded (U1, U3, U4 (i), U5, fixture F1/F2/S RED) before PB1
   lands; land the help-text pair in ONE commit.
5. **Write the ledger entry, `BEHAVIOR_CONTRACT.md:4359`, the `REVIEW_FINDINGS.md` annotation and ADR-0322
   §Decision + §Consequences** at the IMPL, appended after the retained footer.
6. **Evaluate the split gate** with a stated accounting (§12.3).
7. **Re-census the fixture ports** (§7.2) and run the fixture set extractor in both `comm` directions, naming
   the argument order.

### Coverage of `BRAINSTORM.md` §10 (the nine owed items)

| # | owed | discharged at |
|---|---|---|
| 1 | choose the repair shape against the race; 50-conn `false` arm ≥ 2 runs + single-conn arms | §3.2, §3.3 (6 runs × 50 per race-free shape, 3 for P0, n=300), §4.1-4.2 — **PB1** |
| 2 | measure R3t under the chosen shape | §0.2 — **no `400`; HCM divergence, banked** |
| 3 | specify the `0s` fold; re-point the zero → 15000 row after reading inputs | §4.1 (b), §3.4; §0.3 — **no such row; new U3** |
| 4 | decide the close kind or pin none | §0.1 — **pin none**, measured both sides, banked |
| 5 | replace the vacuous abort assertion; real-`tls_inspector` arms RED at the tip | §5 U1 (measured RED), U2, U4, U5; §11 NC6 |
| 6 | charter fixture `0125` | §7 |
| 7 | record the non-timeout gating | §10 — NOT CONSTRUCTIBLE today, one shutdown path after PB1 |
| 8 | draft ADR-0322 §Context; predict the ledger delta; name the guards | §8, §9 — **+1 confirmed by value; one guard (`KeySetExact`)** |
| 9 | re-derive the occurrence set; state what the row does not buy | §6, §1, §14.3 |

---

## §A. U1 — the measured unit arm, verbatim

```go
package listener

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients drives the REAL
// tls_inspector (not the ctx-aware installSlowListenerFilter stub) through a
// manager-built listener: listener_filters_timeout 1s, continue_on_... false
// (default), N concurrent SILENT clients over real loopback TCP. Each client
// holds a 3 s read deadline of its own, and the test separates the three
// outcomes the tip's abort test conflates:
//
//   - closed: the SERVER closed (EOF or ECONNRESET, zero bytes) before 2 s;
//   - fellThrough: bytes arrived — the pipeline reported success and the
//     tcp_proxy chain's tagged backend answered (the deadline-only race);
//   - open: the CLIENT's own deadline expired — the server never acted.
//
// Concurrency is load-bearing: a single connection passes a racy
// two-clock repair most of the time.
func TestListenerFilterTimeoutRealTLSInspectorDropsSilentClients(t *testing.T) {
	const n = 20
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	l := &listenerv3.Listener{
		Name: "l_lf_real_inspector",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_a")}}},
		ListenerFilters:        []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()
	addr := mgr.Listeners()[0].Addr

	type outcome struct {
		kind string // closed | fellThrough | open | late | other
		ms   int64
	}
	out := make([]outcome, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, derr := net.DialTimeout("tcp", addr, 2*time.Second)
			if derr != nil {
				out[i] = outcome{kind: "other: " + derr.Error()}
				return
			}
			defer func() { _ = c.Close() }()
			start := time.Now()
			_ = c.SetReadDeadline(start.Add(3 * time.Second))
			buf := make([]byte, 16)
			nr, rerr := c.Read(buf)
			ms := time.Since(start).Milliseconds()
			switch {
			case nr > 0:
				out[i] = outcome{"fellThrough", ms}
			case errors.Is(rerr, os.ErrDeadlineExceeded):
				out[i] = outcome{"open", ms}
			case errors.Is(rerr, io.EOF) || errors.Is(rerr, syscall.ECONNRESET):
				if ms < 2000 {
					out[i] = outcome{"closed", ms}
				} else {
					out[i] = outcome{"late", ms}
				}
			default:
				out[i] = outcome{"other: " + rerr.Error(), ms}
			}
		}(i)
	}
	wg.Wait()
	counts := map[string]int{}
	for _, o := range out {
		counts[o.kind]++
	}
	if counts["closed"] != n {
		t.Errorf("want all %d silent clients closed by the server before 2s (listener_filters_timeout 1s, continue=false); got %v", n, counts)
	}
	for i, o := range out {
		if o.kind == "closed" && o.ms < 900 {
			t.Errorf("client %d closed at %d ms, before the 1s deadline", i, o.ms)
		}
	}
}
```
