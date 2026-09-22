# Phase 100 — `listener-filters-timeout-enforce` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle **DONE -> 1**). **Self-picked** under the 2026-07-12 standing
directive, with no human consulted and no banked mid-lifecycle work to advance first (check (1) was
SILENT at `want=131` before this stage's ADD — §8.1).

**Subject in one sentence.** On a TCP listener with a listener filter (`tls_inspector`, the only one in
the tree), envoy-go never enforces `listener_filters_timeout`: a client that sends nothing is held with
no deadline — a goroutine and a file descriptor per connection, fifty of them measured — until the
CLIENT acts, while the reference times the pipeline out at the configured deadline, books
`listener.<addr>.downstream_pre_cx_timeout`, and then either CLOSES the connection
(`continue_on_listener_filters_timeout: false`) or falls through to chain selection
(`true`).

**Both sides were MEASURED at this stage's own tip (`d5bc9153`)** through the real binaries, by a
Docker-only reference agent (image by digest `7edd5b0fd763…`, verified) and a Docker-free subject agent,
on disjoint port bands and disjoint worktrees — the phase-98 split. The banked record (`98/PLAN.md` §0.1,
§0.3, §10) was **re-measured rather than inherited** (method note 69): its core held, and **four of its
framings did not** (§0.1, §0.5, §0.6, §0.9).

---

## 0. What this stage refuted — THIRTEEN claims, by execution

Every item was produced by running something; the command or arm is named.

### 0.1 🔴 "`continue_on…: false` is unreachable for a silent client, so the reference's DROP never happens" — FALSE

The router's banked split (b) says the drop *never happens*. **It happens — LAZILY, timed by the client.**
Subject, tip binary, `continue_on…: false`, client silent 2500 ms then sends: **EOF with no response
bytes at 2502 ms**, and the server logs `listener-filter pipeline aborted: listener-filter[0]: pipeline
timeout: context deadline exceeded`. The `context.WithTimeout` in `Pipeline.Run` DID expire; it is
consulted only when `Inspect` returns (`pipeline.go:47-53`), and `Inspect` returns only when a byte or an
EOF arrives. A silent client held 30 s is **still open at 30.029 s**, and the abort is logged only when the
client closes. **The defect is not "the drop is unreachable" but "the drop is scheduled by the peer, not
by the deadline."** The reference drops at **1000-1002 ms** while the client is still silent (§2.1).

### 0.2 🔴 The listener-filter timeout ENVELOPE `[1s, 60s]` IS AN envoy-go RESTRICTION, NOT A REFERENCE ONE

ADR-0082 and `BEHAVIOR_CONTRACT.md:4359` present `[1s, 60s]` as the honoured envelope. **The reference
validates AND enforces `0.5s` (drop at 501 ms), `61s` (61000 ms) and `120s` (120001 ms), rc=0, no
warning.** envoy-go boot-REJECTS all three (`listener_filters_timeout 500ms is outside the supported [1s,
60s] envelope`; `parseListenerFiltersTimeout`, `manager.go:951-962`). A static bootstrap the reference
runs, envoy-go refuses — fail-CLOSED. **A second, independent divergence** (§4.1). ⚠️ The spelling
`500ms` fails envoy-go earlier, at `protojson` (`invalid google.protobuf.Duration value "500ms"`) — only
`0.5s` reaches the envelope check. **A probe must spell a Duration the parser accepts, or it measures the
parser, not the envelope** (method note 51).

### 0.3 🔴 `0s` DISABLES THE TIMEOUT ON THE REFERENCE; envoy-go READS IT AS THE 15 s DEFAULT

ADR-0082: *"The default is `15s` when the field is unset (zero-valued duration)."* Reference, `0s`,
`continue…: false`, silent client: **still open at 20019 ms, `downstream_pre_cx_timeout` 0**, never handed
to a chain. Subject, `0s`, `continue…: false`: a send at 2.5 s is served, a send at **16 s gets EOF** with
the abort logged — `0s` behaves as a 15 s (lazy) timeout. ⚠️⚠️ **THIS COUPLES TO THE REPAIR** (§1.3): today
a SILENT client on a `0s` listener is held forever on BOTH sides — an agreement by accident (method note
58). **A repair that enforces the deadline without fixing `0s` MINTS a new divergence: the subject would
start dropping silent clients at 15 s where the reference never does.**

### 0.4 `BEHAVIOR_CONTRACT.md:4359` AND ADR-0082 §Decision ARE FALSE IN THREE CLAUSES, NOT ONE

*"honored"* (§0.1), *"in [1s, 60s] envelope"* as a parity statement (§0.2), and *"default 15s"* for a
zero duration (§0.3). ADR-0082 (b) additionally says `Pipeline.Run(…, 0)` means *"no-op, disables
enforcement"* — true of the FUNCTION, and unreachable from config, because the parser maps a zero
duration to 15000 before the call.

### 0.5 🔴 THE OBVIOUS REPAIR — "set a read deadline, then check `ctx.Err()`" — IS RACY AND LOSES MOST DROPS

The banked note names the repair as *"socket deadlines or a ctx-aware peeker."* The subject agent built
the socket-deadline form in a throwaway worktree. **With the deadline alone, under 50 concurrent silent
`continue…: false` connections, only 21/50 and 14/50 were closed at 1 s across two runs; 65 of 100 FELL
THROUGH** and stayed open until the client closed at 5 s. Mechanism: the read deadline and the context
deadline are the SAME instant on two clocks; when the socket deadline fires first, `tls_inspector` maps
the zero-byte deadline error to `raw_buffer` and returns `Continue, nil` (`tls_inspector.go:53-94`: it never
returns an error), `ctx.Err()` is still nil, and the pipeline reports SUCCESS. **A wall-clock check after
`Run` ("the deadline has passed ⇒ timeout") was load-bearing**: with it, 53/53 aborts were booked — 14 by
the context path and **39 by the wall-clock path**. ⇒ **The repair's SHAPE is a SPEC decision with a
measured trap in it (§3.1), not a one-liner.**

### 0.6 THE PHASE-98 ABSENCE MATCHER WAS BLIND TO `manager.go` — THE CONCLUSION SURVIVES

`98/PLAN.md` §0.1 counted `SetReadDeadline` with pathspec `'internal/listener/**/*.go'`. **That pathspec
does not match `internal/listener/manager.go` or `quic.go`** (`git ls-files 'internal/listener/**/*.go' |
/usr/bin/grep -c '^internal/listener/manager.go$'` → **0**; `git ls-files internal/listener/manager.go` →
present) — `**/` requires at least one directory level. Re-run on the right pathspec:
`git grep -nE 'Set(Read|Write)?Deadline' -- 'internal/listener/' ':!*_test.go'` → **rc=1, zero hits**;
positive control, same pathspec with tests included → `manager_test.go` **15**, `integration_test.go`
**2**. **Zero was right, for a reason the matcher could not supply** (method note 49).

### 0.7 THE `downstream_cx_total` POSITIVE-CONTROL FIGURE `→ 3` HAS ROTTED — it reads **21 files**

`git grep -l 'downstream_cx_total' -- '*.go' | wc -l` → **21** at this tip (the router quotes "→ 3").
`git grep -l 'pre_cx' -- '*.go' | wc -l` → **0**, unchanged. The absence claim still has its positive
control; only the control's number moved.

### 0.8 THE DEFECT WAS NOT "NEW AT THE phase-98 PLAN" — `REVIEW_FINDINGS.md` RECORDED IT ON 2026-07-07

`REVIEW_FINDINGS.md:185-187` (commit `9f26f380`, 2026-07-07, the repo-wide review pass): *"`listener_filters_timeout`
never enforced (silent client hangs a goroutine+fd forever); `continue_on_listener_filters_timeout`
wrongly gates non-timeout errors; SNI chain match is case-sensitive."* The phase-98 PLAN (2026-09-20)
re-discovered it seventy-five days later. **A findings file no stage reads is not a register** — recorded
here so the SPEC cites it.

### 0.9 🔴 "A RESOURCE-EXHAUSTION CHARACTERISTIC" IS ONLY HALF TRUE — UNDER `continue…: true` THE REFERENCE PARKS THE CONNECTION TOO

Reference, `continue…: true`, silent client: the timeout fires at 1000 ms, `fallback to default listener
filter` is logged, the connection is handed to the HCM — and is **still open at 30000 ms and at 90042 ms**;
the reference closes it only when the client does (`downstream_cx_destroy_remote: 1`). No HCM default
timeout fires in 90 s. **So parity under `true` is "fall through AT the deadline and count it", not "close
it".** The resource bound exists only under `continue…: false` (the proto default). The SPEC must not
charter a close under `true` in the name of exhaustion — that would be a NEW divergence.

### 0.10 THE REFERENCE COUNTS `downstream_cx_total` AFTER THE LISTENER FILTERS; envoy-go COUNTS AT ACCEPT

Reference drop arm (`continue…: false`, silent): **`downstream_cx_total: 0`**; the timeout arms' `new
connection` log line appears only after the fallback, and `downstream_cx_length_ms` is measured from the
hand-off, not from accept. Subject: the accept loop increments `cx_total` and `cx_active` before starting
`serveConnection` (`manager.go` ~`:1280-1283`), so a dropped connection counts. **Any cross-side
`downstream_cx_total` pin on a drop arm is off by exactly one per connection, structurally** — the
phase-99 §0.14 no-match trap in another costume (§3.4, banked §4.2).

### 0.11 THE TIMEOUT APPLIES ONLY WHEN LISTENER FILTERS EXIST — AND THE TWO SIDES AGREE, BY A NAMED MECHANISM

Reference, no `listener_filters`, `1s`, `continue…: false`, silent client: **not dropped** at 3002 ms,
`cx_total` 1 at connect, `pre_cx` 0. Subject: `Pipeline.Run` returns `nil` immediately when
`len(filters) == 0` (`pipeline.go:38-40`). **Agreement with a mechanism on both sides**, not a coincidence
— and a matched-negative arm the fixture gets for free.

### 0.12 THE CLOSE KIND DEPENDS ON UNREAD BYTES — FIN vs RST

Reference, `continue…: false`: a silent client sees **FIN (EOF) at 1000 ms**; a client that sent one byte
`0x16` (peeked, never read) sees **RST at 1001 ms** — the matched pair. **Unmeasured on the subject**; the
prototype reported EOF without classifying the kind. ⚠️ **Do not pin a close kind cross-side until both
sides are measured by a client that distinguishes them** (§10 item 4).

### 0.13 "DUPLICATE listener ADDRESSES — envoy-go ACCEPTS (rc=0)" IS TRUE OF VALIDATE ONLY

Subject: `-mode validate` → `configuration OK` rc=0; **boot → rc=1** (`bind: address already in use`).
Reference: `--mode validate` rc=1, `error adding listener: 'l_dup' has duplicate address '0.0.0.0:10000'
as existing listener …`. **Both FAIL CLOSED at boot**; the divergence is confined to validate mode.
Re-banked with that narrowing (§4.7).

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

On a TCP listener with at least one listener filter, the listener-filter pipeline is bounded by
`listener_filters_timeout` **as a real deadline**: at the deadline the connection is CLOSED under
`continue_on_listener_filters_timeout: false` and handed to chain selection (as `raw_buffer`) under
`true`; each timeout books `listener.<addr>.downstream_pre_cx_timeout` (**+1 NAME**); and a `0s` value
DISABLES the timeout, as on the reference.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

| candidate | prod floor (built + run at this tip) | reference measured at this tip? | cross-side surface | severity |
|---|---|---|---|---|
| **timeout enforcement (THIS ROW)** | **`23 6`, one file** (§6) | **YES — R1-R6** | **drop timing, stat value, served body** | a silent peer holds a goroutine + fd per connection, unbounded |
| timeout envelope lift (§0.2) | not prototyped; a range check + ADR | YES (R4) | boot reject vs boot | fail-closed boot |
| `cx_total` accounting (§0.10) | not prototyped; moves an Inc across the pipeline | YES (R1d) | counter value | a counter off by one per dropped cx |
| order-dependent pairwise fold | not prototyped; `SelectChain` pass 2 | **NO** | serve vs close | wrong chain / close |
| SNI case-insensitivity | `4 1` pattern side + unmeasured input side | phase-99 BRAINSTORM | serve vs DEFAULT | wrong chain |
| nested-descent precedence | a `SelectChain` rewrite | ONE arm (G3) | several | wrong chain / close |

`listener_filters_timeout` was adjudicated NEXT IN LINE by `99/BRAINSTORM.md` §4.4. **That adjudication is
adopted after re-measurement, not inherited:** it is the only candidate with a complete both-sides
measurement at THIS tip, a single-file floor, and a severity no other candidate matches — a remote,
unauthenticated peer can pin server resources by connecting and saying nothing, on the proto's DEFAULT
setting (`continue…` defaults to `false`), against a document (`BEHAVIOR_CONTRACT.md:4359`) that says the
timeout is honoured. The fifty-connection arm is the TIMED observable the router's owed item 2 asks for: at 2.5 s
past a 1 s timeout, `downstream_cx_active` read **50** and the server's open files had risen **7 -> 57**;
released only when the client closed.

### 1.3 Why `0s` IS folded in and the envelope lift is NOT

- **`0s` (§0.3) is COUPLED.** Enforcing the deadline changes the observable for every `0s` listener with a
  silent client, from *held forever* (agreeing with the reference, by accident) to *dropped at 15 s*
  (disagreeing). The repair therefore MINTS a divergence unless `0s` is fixed with it. Same function
  (`parseListenerFiltersTimeout`), one clause.
- **The envelope (§0.2) is INDEPENDENT.** The repair changes nothing about a config that is boot-rejected
  today, and lifting the reject needs its own arms (sub-second deadlines stress timing tolerances; the
  reference's upper bound was measured only to `120s`). It amends the SAME ADR, so it is the natural next
  row — **banked in §4.1, not folded.**

### 1.4 What this row does NOT buy — stated plainly

- It does **not** lift the `[1s, 60s]` envelope (§4.1) or move `downstream_cx_total` to post-pipeline
  (§4.2).
- It does **not** close silent connections under `continue…: true` — the reference does not either (§0.9).
- It does **not** decide the close KIND (FIN/RST) until the subject is measured (§0.12).
- It does **not** address *"`continue…` wrongly gates non-timeout errors"* (`REVIEW_FINDINGS.md:186-187`):
  **NOT CONSTRUCTIBLE today** — `tls_inspector` is the only `ListenerFilter` (method-signature matcher and
  the sole non-test `Register`, `internal/boot/boot.go:75`, agree) and its five returns never carry an
  error, so no non-timeout pipeline error can arise. A future filter makes it live; recorded for the SPEC
  (§10 item 7).
- It touches **no QUIC path**: the only `Pipeline.Run` call is `manager.go:1350`; QUIC runs no pipeline.

---

## 2. The defect, MEASURED — both sides, matched negatives, timed observables

**Base config** (both sides, byte-equivalent apart from addresses): one listener, `tls_inspector`,
`listener_filters_timeout: 1s`, `filter_chains[0]` matching `transport_protocol: raw_buffer`
(`direct_response` body `INDEXED`, `stat_prefix: chain_indexed`) and a `default_filter_chain` (body
`DEFAULT`, `stat_prefix: chain_default`); every other arm is a ONE-LINE diff from it. Probes are Go
programs with millisecond timing that classify EOF / RST / still-open. The reference ran from a client
container on a user bridge network (a real kernel socket — `docker-proxy` can mask an RST); host-side
`-p` runs agreed within 1 ms.

### 2.1 Reference (`envoyproxy/envoy@sha256:7edd5b0fd763…`, digest verified)

| arm | delta | client | observable | client timing | `cx_total` | `pre_cx_timeout` |
|---|---|---|---|---|---|---|
| R1a | `continue: true` | immediate | `INDEXED` | 1 ms | 1 | **0** |
| R1b | same | silent 2500 ms, sends | `INDEXED` | 2502 ms | 1 | **1** |
| R1c | chain matches `tls` | silent 2500 ms, sends | `DEFAULT` | 2503 ms | 1 | 1 |
| R1d | `continue: false` | silent | **dropped (FIN)** | **1002 ms** | **0** | 1 |
| R2f | `continue: false` | silent, hold 30 s | dropped (FIN) | 1000 ms | 0 | 1 |
| R2t | `continue: true` | silent, hold 90 s | **still open** | open at 90042 ms | 1 | 1 |
| R3f | `continue: false` | one byte `0x16` | dropped (**RST**) | 1001 ms | 0 | 1 |
| R3t | `continue: true` | one byte `0x16` | **`400`** (HCM `NO_REQUEST_LINE_IN_REQUEST`) | 1001 ms | 1 | 1 |
| R4 | absent | silent, `false` | dropped | 15001 ms | — | 1 |
| R4 | `0s` | silent, `false` | **never dropped** | open at 20019 ms | 0 | **0** |
| R4 | `0.5s` / `61s` / `120s` | silent, `false` | dropped | 501 / 61000 / 120001 ms | — | 1 |
| R5 | no listener filters | silent, `false` | **not dropped** | open at 3002 ms | 1 | 0 |

Log, timeout path: `active_tcp_socket.cc:56 listener filter times out after 1000 ms`, then — **under
`true` only** — `active_tcp_socket.cc:59 fallback to default listener filter`. The R1b-vs-R1a `/stats` diff
moves **only** `downstream_pre_cx_timeout` among counters; `downstream_listener_filter_error` and
`…_remote_close` stay 0 on every timeout arm. **The name exists at boot, value 0, before any traffic**, on
`/stats` (`listener.0.0.0.0_10000.downstream_pre_cx_timeout: 0`) and `/stats/prometheus`
(`envoy_listener_downstream_pre_cx_timeout{envoy_listener_address="0.0.0.0_10000"} 0`) — ⚠️ **so a
NAME-presence pin is VACUOUS (method note 81); pin the VALUE with its mirror arm (R1a).**

### 2.2 Subject (envoy-go at `d5bc9153`)

| arm | delta | client | observable | timing |
|---|---|---|---|---|
| S1-1/2/3 | `continue: true` | immediate / silent 2500 / 10000 ms | `INDEXED` ×3 | 0 / 2502 / 10009 ms |
| S2 | `continue: false` | silent 2500 ms, sends | EOF, no bytes | **2502 ms** (at the send) |
| S3 | `true` or `false` | silent, hold 30 s | **still open** | 30.03 s |
| S3 | `true` / `false` | one byte `0x16`, hold 10 s | still open; on client close `rq_4xx` +1 / abort logged | — |
| S4 | `false` | **50 concurrent silent**, hold 5 s | `cx_active` **50**, open fds **7 -> 57** at 2.5 s | released at client close |
| S5 | `0s` | `false`, send at 16 s | EOF, abort logged | lazy 15 s |

After S1: `downstream_cx_total: 3`, `chain_indexed` 3, `chain_default` 0 — **phase 98 reproduced exactly.**
⚠️ **S1 AGREES WITH R1b ON THE BODY — `INDEXED` on both — while evaluating nothing** (method note 58): the
subject reached `raw_buffer` because the late byte arrived, not because a timeout fired. **Only
`pre_cx_timeout` separates the two mechanisms, which is why the row owns the NAME (§1.1).**

### 2.3 The mechanism, at the tip, by symbol

- `peekerConn.Peek` (`listenerfilter/callbacks.go:47-49`) is `p.br.Peek(n)` — no deadline.
- `Pipeline.Run` (`listenerfilter/pipeline.go:41-53`) builds `context.WithTimeout` and checks `ctx.Err()`
  only after `Inspect` returns.
- `tls_inspector.Inspect` (`tls_inspector.go:53-94`) is not ctx-aware (its own comment, `:55-57`), has 5
  returns and 5 `TransportProtocol` writes, never returns an error.
- `serveConnection` (`manager.go:1350-1361`): `p.Run`; on error, close iff `!rt.continueOnLfTimeout`; then
  the ADR-0320 `raw_buffer` stamp.
- `parseListenerFiltersTimeout` (`manager.go:951-962`): nil or zero → 15000; outside `[1000, 60000]` ms →
  error.

---

## 3. Hazards for the SPEC

### 3.1 The repair shape has a MEASURED race (§0.5)

Two clocks expiring at one instant: the socket deadline and the context. Whichever form the SPEC
chooses — deadline plus a wall-clock check, a ctx-aware peeker (a `SetReadDeadline` driven by
`ctx.Done()`), or a deadline-error classification returned as a pipeline error — **the 50-connection
`continue…: false` arm is the discriminating gate**, run more than once; a single connection passes the
racy shape most of the time. ⚠️ Measure the reference behaviour the chosen shape encodes for PARTIAL
bytes under `true` (R3t answers `400` AT the deadline; the subject's timing under the prototype was not
classified) — method note 37.

### 3.2 The existing suite is BLIND, and one test is VACUOUS

`TestUnifiedDispatchListenerFilterTimeoutAbortsConnection` (`manager_test.go:4036`) and
`…TimeoutContinue`, `integration_test.go:118`'s `listener_filters_timeout_abort`, and
`pipeline_test.go`'s `TestPipelineRunTimeoutSharedAcrossFilters` / `…ZeroTimeoutDisablesEnforcement` all
install a **ctx-aware stub** (`installSlowListenerFilter`, a `select` on `ctx.Done()`), never the real
`tls_inspector` peek. The abort test's only failure branch is `rerr == nil && n > 0` (read at `:4084-4086`)
— **its own 3 s client read deadline expiring satisfies it**, so it cannot tell *closed by the server* from
*never closed*. Measured: tip and prototype both read **266** `=== RUN`, **0** FAIL, identical rosters over
`./internal/listener/...` — **a green package under a behaviour-changing patch** (method note 50). The four
`TestParseListenerFiltersTimeout*` tests pin the envelope and the 15 s default, including for zero — ⚠️
**the `0s` fold (§1.3) must RE-POINT whichever of them asserts zero → 15000; read their inputs first**
(method note 85).

### 3.3 Pre-fix GREEN arms are structural, not bugs in the arms

Every `continue…: true` silent-then-send arm serves `INDEXED` at the tip, as on the reference (§2.2).
Their only falsifiability is the `pre_cx_timeout` value and an NC roster row, **scored per arm** (method
note 61).

### 3.4 The fixture must not pin `downstream_cx_total` on a drop arm (§0.10)

Pin the drop by CLIENT-OBSERVED TIMING (closed before a send scheduled well past the deadline), the served
body on fall-through arms, and `pre_cx_timeout` by VALUE with R1a as its mirror. `cx_total` is structurally
off by one per dropped connection.

### 3.5 Timing arms need stated tolerances

The reference drops at 1000-1002 ms. A fixture that asserts "closed before the client's send at +1500 ms,
and not before +500 ms" is robust; one that asserts an exact millisecond is not. **A resource arm is a claim
about N and T — name both** (50 connections, 2.5 s).

### 3.6 Occurrence set of the false claims

`BEHAVIOR_CONTRACT.md:4359`; ADR-0082 §Decision ¶1-3 and §Consequences (b); every `manager.go` /
`pipeline.go` comment that says *honoured*, *default for zero*, or *[1s, 60s]*; `REVIEW_FINDINGS.md:185-186`
(which becomes stale-in-the-other-direction once fixed). **Sweep case-insensitively, resolve each hit to
its ADR by backward heading search, and say which are left and why** (method notes 31, 38). ADR-0082 is a
**`**Status:** Accepted`** ADR of the OLD form — the SPEC's new ADR supersedes clauses, it does not edit
ADR-0082's decision text.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The timeout ENVELOPE lift (§0.2) — **REJECTED as a fold-in; banked as the NEXT row, measured**

Same ADR, same function, independent observable (boot vs reject). Reference measured at `0.5s`, `61s`,
`120s`; the upper bound beyond `120s` is unmeasured. ⚠️ **It must follow this row, not precede it**: lifting
the reject before enforcement exists would admit configs whose timeout still does nothing.

### 4.2 `downstream_cx_total` accounting at hand-off (§0.10) — **REJECTED; new, banked**

A counter-semantics divergence across every listener with filters, not only timeouts; moving the `Inc`
also moves `cx_active` semantics and every fixture that pins `cx_total`. Its own row, after a census of
the pins it would move.

### 4.3 The order-dependent pairwise fold in `SelectChain` — **REJECTED: its reference side is UNMEASURED, and one of its two axes may not be a parity case at all**

`SelectChain` (`chainmatch.go:80-112`) returns `ErrAmbiguousChainMatch` on the first nil `breakTie`. Of its
two banked axes, the SNI triple needs two chains whose best MATCHED patterns tie — a class the reference
REFUSES at validate (`multiple filter chains with overlapping matching rules`, `99/BRAINSTORM.md` §0.13) —
so that arm may have no reference behaviour to match. The ALPN axis is entangled with nested descent
(§4.5). **Measure the reference on F1 before chartering** (method note 69).

### 4.4 SNI case-insensitivity — **REJECTED; unchanged from `99/BRAINSTORM.md` §1.3.** Pattern side `4 1`, input side unmeasured, three repair sites.

### 4.5 Nested-descent precedence — **REJECTED for size; unchanged** (`99/BRAINSTORM.md` §0.12, one reference arm).

### 4.6 Duplicate-matcher reject parity — **REJECTED; unchanged** (`99/BRAINSTORM.md` §0.13).

### 4.7 Duplicate listener addresses — **REJECTED; NARROWED to validate mode** (§0.13). Both sides fail closed at boot.

### 4.8 The `rank > 2` guard in `sniMatchedRank` — **REJECTED as a row: no cross-side surface.** A coverage fold-in for the next chain-match row.

### 4.9 The HCM `stat_prefix` duplicate-registration panic · `server_names` partial-wildcard acceptance · the driver-owned receiver port race · the dead-port false-green · `ROADMAP.md`'s stale swallowed-panic claim inside a sentinel window — **REJECTED; unchanged, not re-derived.** The sentinel-window claim is **recorded, not tidied** (margin of one).

---

## 5. Family attribution

**A Listener / listener-filter MAINTENANCE row claiming NO family ordinal**, on the row-85-through-91 and
95-99 precedent. It repairs a landed deliverable (phase 07.2's pipeline, ADR-0082) and extends no family
charter.

---

## 6. The cost FLOOR — prototyped, run, reverted

`23 6` in `internal/listener/manager.go` only (`git diff --numstat` in a throwaway worktree, since
removed; ~`+6 -5` of it is gofmt re-aligning `listenerRuntime` fields around one new field): a read deadline
on the raw connection before `p.Run` when filters exist, a wall-clock "deadline passed" check after it
(§0.5), clearing the deadline, and a `downstream_pre_cx_timeout` counter registered beside
`downstream_cx_total` in `registerListenerMetrics` (`manager.go:416-427`). Re-driven against the arms:
`false` drops at **1001 ms** (silent, 30 s hold, `0x16`, and all 50 concurrent, fds back to 7 by 2.5 s);
`true` serves `INDEXED` with `pre_cx_timeout` +1, the `tls` matched negative serves `DEFAULT`, immediate
arms leave the counter at 0. `-race ./internal/listener/...` and `./internal/{stats,admin,boot}/...`,
`./cmd/envoy-go/...` green. **No stat-name guard reddens** — the listener-metrics tests check membership or
the `ssl.` set only; `internal/stats/name.go` `helpText` + `helptext_test.go` `helpTextRoster` want an
entry by convention and are guarded only against each other.

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE** (`reference_measured_prototype_is_a_lower_bound`, twenty-one rows).
It omits the `0s` fold (§1.3), the SPEC's repair-shape decision (§3.1), unit arms that drive the REAL
`tls_inspector` and redden at the tip, the vacuous abort test's repair (§3.2), help text, the occurrence set
(§3.6), a new fixture, an ADR and the ledger entry. **Fixture floor by SHAPE**: a plaintext, PKI-free,
multi-listener fixture — `0123` (the nearest shape: plaintext chain-match, no PKI) is the precedent to
re-measure, NOT `0124` (**940**, TLS with a `pki/gen` package) or the PKI-inflated `1359`/`1393`.

---

## 7. The differential measurement

### 7.1 There is no existing gate

`git grep -l 'listener_filters_timeout' -- test/` → **`0123/README.md` and `0123/driver/driver.go` only**, both
prose/incidental — **no fixture configures the field**, so no fixture can see the defect on either side.

### 7.2 What the gate must do

A new fixture **`0125`** (reference port **`15125`** on the `15000 + index` convention; its only
occurrences under `test/ internal/ cmd/` are `0123`'s reserving prose — **a literal is not a bind; re-census
before use**). Listeners (each a ONE-line diff from the base): `false` (drop arm, timed), `true` (fall-through
arm + R1a mirror + `tls` matched negative), `0s` (never dropped), and no-filters (not dropped). Pins: drop
by client timing inside a stated window (§3.5), bodies, `pre_cx_timeout` by VALUE; **never** `cx_total` on a
drop arm (§3.4), **never** a close kind (§0.12). ⚠️ **The harness must be able to drive a SILENT client** —
`HTTPExpectations` sends immediately; this needs a Drive hook (`reference_differential_http_expectations_tcp_only`).
Prediction: the stat surface moves **+1 NAME** (`listener.<addr>.downstream_pre_cx_timeout`), the first
non-`+0` ledger entry since phase 95's chain — quote it as a DELTA, never an absolute.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN ADD

All commands copied verbatim from `next-prompt.txt`, `/usr/bin/grep` named.

### 8.1 PRE-ADD, at `d5bc9153` (`ROADMAP.md` 249 lines, tail row 99 `done` at `:161`)

(1) **SILENT** · (2) **SIX** at `:209 :215 :221 :231 :237 :245` · (3) **SILENT**.

### 8.2 The four NCs and the check-(2) positive control, PRE-ADD — ALL FIRED

NC-A: substitution inspected first, `NC LANDED? [ in-progress ]`, then **ONE** line, `NOT DONE: row 62`
· NC-B (`want=130`): **ONE**, `GATE FAIL: examined 131 data rows, expected 130` · NC-C: residual **0**,
`NEVER OPENED: gRPC   <- NC FIRED` · NC-D: **96 / 68** under `--` · check-(2) positive control:
residual **0**, **6** substitutions asserted.

### 8.3 Escape-aware malformed set and per-line digests, PRE-ADD

`sed 's/\\|//g' ROADMAP.md | awk -F'|' '/^\| *[0-9]/ && NF!=8'` → exactly **{57, 69}** at file lines
**119** (NF 9) and **131** (NF 10). Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f |
md5sum`, first 12 hex): `209 10d7807bf02d` · `215 4a92f7e62fc6` · `221 2a7eb298b9fd` ·
`231 242e53c6f7a3` · `237 b2680e6f4fbf` · `245 6caa1c3ce0e7` — **byte-identical to the phase-99 close.**

### 8.4 POST-ADD — measured on the other side of this stage's own ADD

Row 100 installed after `:161` as `in-progress`, gated FIRST in a scratch file: **8 fields naive, 8
escape-aware**, and **zero** hits for either sentinel match phrase, for the bare word `deferred`
(case-insensitive) and for `-family row`. `ROADMAP.md` **249 -> 250** (`git diff --numstat`: `1 0`).
Re-run at `want=132`:

| check | pre-ADD (§8.1-8.2) | **post-ADD** |
|---|---|---|
| (1) | SILENT | **ONE** — `NOT DONE: row 100` |
| (2) | SIX at `:209 :215 :221 :231 :237 :245` | **SIX at `:210 :216 :222 :232 :238 :246`** — every window shifted +1, as an insertion ABOVE them must |
| (3) | SILENT | SILENT |
| NC-A (row 62 doctored) | ONE | **TWO** — `NOT DONE: row 62`, `NOT DONE: row 100` (substitution inspected: `NC LANDED? [ in-progress ]`) |
| NC-B | ONE at `want=130` | **TWO at `want=131`** — `NOT DONE: row 100`, `GATE FAIL: examined 132 data rows, expected 131` |
| NC-C | FIRED, residual 0 | FIRED, residual 0 |
| NC-D (`--`) | 96 / 68 | **96 / 68** — unmoved; the new cell spells no `-family row` |
| check-(2) positive control | residual 0, 6 substitutions | residual 0, 6 substitutions |
| escape-aware malformed set | {57, 69} at `:119`, `:131` | **{57, 69} at `:119`, `:131`** — unmoved (both above the insert) |
| row 100 NF | — | **8 naive, 8 escape-aware** |

Per-line md5 at the SHIFTED lines, trailing newline INCLUDED: `210 10d7807bf02d` · `216 4a92f7e62fc6` ·
`222 2a7eb298b9fd` · `232 242e53c6f7a3` · `238 b2680e6f4fbf` · `246 6caa1c3ce0e7` — **all six
byte-identical to §8.3: the windows MOVED and did not CHANGE.**

Fixture registration, re-run verbatim (the blank-import extractor over `test/differential/runner_test.go`):
**dirs 126 = imports 126**, both `comm` directions EMPTY, split **102 `driver/` + 24 `inputs/`** — unmoved,
as a BRAINSTORM adds no fixture. Rename NC (the `0124` import renamed in a scratch copy) → `comm -23`
**1**, `comm -13` **1**, under the `(imports, dirs)` argument order.

⇒ **THE SENTINEL DOES NOT FIRE on either side of this ADD. `stop` was evaluated and NOT created.**

---

## 9. Findings the next stage must not re-learn

1. **The drop under `false` happens, timed by the CLIENT** (§0.1) — the defect is the missing deadline.
2. **A deadline-only repair loses ~65% of drops under concurrency** (§0.5); run the 50-connection arm twice.
3. **`0s` disables the reference's timeout** and must be folded, or the repair mints a divergence (§0.3).
4. **The `[1s, 60s]` envelope is envoy-go's, not the reference's** (§0.2) — next row.
5. **Under `true` the reference parks a silent connection ≥ 90 s** — do not "fix" that (§0.9).
6. **`cx_total` is post-filter on the reference** — never pin it on a drop arm (§0.10).
7. **`500ms` is a protojson reject, not an envelope reject** — spell `0.5s` (§0.2).
8. **`'internal/listener/**/*.go'` does not match `manager.go`** — use `'internal/listener/'` (§0.6).
9. **`REVIEW_FINDINGS.md` holds un-chartered findings no router cites** (§0.8) — grep it when banking.

---

## 10. What the SPEC owes

1. **Choose the repair shape against §3.1**, and gate it on the 50-connection `continue…: false` arm run
   at least twice on the subject — plus the single-connection arms of §2.1.
2. **Measure the subject's behaviour under the chosen shape for R3t** (one byte, `true`): the reference
   answers `400` AT the deadline.
3. **Specify the `0s` fold** (§1.3): parse `0s` to "disabled", keep "absent" → 15 s, and re-point the
   `TestParseListenerFiltersTimeout*` row(s) that pin zero → 15000 after reading their inputs.
4. **Decide the close kind** (§0.12) — measure the subject with a FIN/RST-classifying client, or state that
   the fixture pins neither.
5. **Replace the vacuous abort assertion** (§3.2) with one that distinguishes a server close from a client
   deadline, and add unit arms driving the REAL `tls_inspector` that are RED at the tip.
6. **Charter fixture `0125`** (§7.2): ports censused, a silent-client Drive hook, timing windows, body
   pins, `pre_cx_timeout` by value with its mirror, no `cx_total` on drop arms; floor re-derived by SHAPE.
7. **Record, and decide nothing about, the non-timeout-error gating** (§1.4): NOT CONSTRUCTIBLE today, with
   its mechanism named.
8. **Draft the new ADR's §Context** (next-free **ADR-0322**, the house `> **STATUS: PROPOSED` block
   form) superseding ADR-0082's *honoured* and *zero → 15 s* clauses and leaving its envelope clause for
   §4.1's row; **predict the ledger delta (+1 NAME)** and name the stat-name guards it touches.
9. **Re-derive the occurrence set** (§3.6) and state what the row does NOT buy (§1.4) in §Consequences.

---

## 11. Probe hygiene

- **Worktrees:** stage worktree `/home/esa/git/envoy-go-wt/phase-100-brainstorm` (branch
  `wt-phase-100-brainstorm`, off `d5bc9153`); the subject agent's throwaway `p100-subj-probe` **created and
  removed**, verified by `git worktree list`.
- **Docker:** one agent only (the reference agent), containers and network `p100ref-*`, torn down by name
  (`docker ps -a --filter name=p100ref-` → header only); no other container touched. A foreign session's
  envoy-go process was observed and left alone.
- **Ports:** band `16400-16499`, censused first (`git grep -In '164[0-9][0-9]' -- test/ internal/ cmd/` →
  rc=1, zero hits; nothing bound under `ss -tan`). Reference host ports `16400-16427`; subject
  `16450-16467` and a placeholder `16499`. ⚠️ **This paragraph spells band numbers; it is a hit in the next
  census.**
- **Scratch:** everything under the session scratchpad (`ref/`, `subj/`); nothing in any worktree. **No
  production `.go` written, none of the six gates run — a BRAINSTORM's SCOPE, not an omission.** The one
  standing departure (no `REVIEW.md`, none of 93-99) is not this stage's to fix.
