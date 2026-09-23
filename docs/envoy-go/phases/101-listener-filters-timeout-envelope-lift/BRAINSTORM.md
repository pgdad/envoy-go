# Phase 101 — `listener-filters-timeout-envelope-lift` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle **DONE -> 1**). **Self-picked** under the 2026-07-12 standing
directive, with no human consulted and no banked mid-lifecycle work to advance first (check (1) was
SILENT at `want=132` before this stage's ADD — §8.1).

**Subject in one sentence.** envoy-go boot-REJECTS every `listener_filters_timeout` outside its own
`[1s, 60s]` envelope — on every listener, with or without listener filters, TCP or QUIC — while the
reference accepts and enforces any non-negative duration up to `9223372035.999999999s`, truncating to
whole milliseconds (so a sub-millisecond value DISABLES the timer, like `0s`) and rejecting only
negatives and larger values; **and the obvious repair (delete the range check, `0 3`) is WRONG**, because
envoy-go carries the value as a `uint32` of milliseconds: a `4294968s` timeout WRAPS to 704 ms and drops
a silent client at **705 ms** where the reference holds it open, and `-1s` BOOTS where the reference
refuses it.

**Both sides were MEASURED at this stage's own tip (`1ae29a1b`)** through the real binaries, by a
Docker-only reference agent (image by digest `7edd5b0fd763…`, verified by `docker image inspect`: Id and
RepoDigest both match `ENVOY_TARGET.md:4`) and a Docker-free subject agent, on disjoint port bands and
disjoint worktrees — the phase-98 split — plus a controller-built prototype of the chosen shape (P1,
§6), driven on the discriminating arms. The adjudication this row inherits (`100/BRAINSTORM.md` §4.1,
`100/SPEC.md` §14.3, ADR-0322 §Consequences (g)) was **re-measured rather than inherited** (method note
69): its core held — the envelope is envoy-go's own and the reference enforces `0.5s`, `61s`, `120s` —
and **its implied repair did not** (§0.2).

---

## 0. What this stage refuted — TWELVE claims, by execution

Every item was produced by running something; the command or arm is named. The reference arm numbers
(R1-R14, RB, RR) refer to §2.1, the subject arms (S*) to §2.2, the prototype arms (P0*, P1*) to §6.

### 0.1 🔴 "The upper bound past `120s` is UNMEASURED" — NOW MEASURED, AND IT IS NOT THE PROTOBUF MAXIMUM

`next-prompt.txt` (the envelope bullet) and `100/BRAINSTORM.md` §4.1 left the reference's upper bound
unmeasured. **Measured (RB, bisection, `--mode validate`):** `9223372035s` and `9223372035.999999999s`
→ rc=0 `configuration OK`; `9223372036s` → rc=1 `Invalid duration: Duration out-of-range: …`. The
protobuf `Duration` maximum `315576000000s` is **REJECTED by the reference** at validate AND at boot
(R11, `Invalid duration: Duration out-of-range: goo.gle/debug… seconds: 315576000000`), and
`315576000001s` fails earlier, in JSON-to-proto parsing (`… duration out of range`). `120s`, `3600s`,
`86400s` and `4294968s` all validate, boot, and hold a silent client open at 3000 ms with
`pre_cx_timeout` 0 (R9, R10). **The bound is ~292 years, the `int64`-nanosecond ceiling, not ~10 000
years.** INFERRED (not read from source): the reference converts to a nanosecond-capable form before
milliseconds; the boundary itself is measured.

### 0.2 🔴 THE IMPLIED REPAIR — "lift the reject" — IS AFFIRMATIVELY WRONG ON THREE AXES (P0)

The adjudicated framing is a range-check lift. The subject agent built exactly that (P0: the
`[1000, 60000]` check deleted, `0 3` in `manager.go`) and drove it:

| arm | P0 | reference | verdict |
|---|---|---|---|
| `4294968s` (its ms value 4294968000 > 2³²) | **closed at 705.3 ms**, `pre_cx_timeout` 1 | open at 3000 ms ×3, `pre_cx_timeout` 0 (R10) | 🔴 **P0 MINTS A DROP** — `uint32(4294968000)` = 704 |
| `-1s`, `-0.5s` | **BOOT**, open at 3002 ms (wraps to ~49.7 days) | rc=1 `Invalid duration: Expected positive duration: …` (R12) | 🔴 fail-OPEN on a config the reference refuses |
| `315576000000s` | **BOOT**, open (Go's `AsDuration` saturates; wraps to ~24 days) | rc=1 `Duration out-of-range` (R11) | 🔴 fail-OPEN |
| `0.5s` / `1.0009s` / `61s` / `120s` / `0.0005s` | agree with the reference | — | ✅ |

**A lift that is GREEN on every arm the adjudication named (`0.5s`, `61s`, `120s`) is WRONG on three
arms nobody had run** — method note 86 (the arm that kills a prototype is the one nobody ran) and
method note 37 (a fix shape encodes a parity answer). The storage width is part of the row.

### 0.3 SUB-MILLISECOND VALUES DISABLE THE REFERENCE'S TIMER — BY TRUNCATION, NOT ROUNDING

`0.0005s` and `0.000000001s` validate, boot, and hold a silent client open at 3000 ms ×3 with
`pre_cx_timeout` 0 (R5, R6). **Arm RR** (20 silent probes per value, each held up to 1000 ms):
`0.0009s` and `0.000999999s` → all 20 open, counter 0; `0.001s`, `0.0015s`, `0.0019s` → all drop at
~2.0 ms mean; `0.002s` → ~2.9 ms mean. A rounding implementation would fire `0.0009s` and treat
`0.0019s` as 2 ms; **the reference truncates to whole milliseconds and a 0 ms result means DISABLED.**
envoy-go's existing `total / time.Millisecond` also truncates, and `Pipeline.Run` arms nothing at 0 —
**so the two sides already agree on the rule, and the envelope is the only thing hiding it** (P0d, P1:
`0.0005s` open at 3002 ms, counter 0).

### 0.4 🔴 THE REJECT IS NOT A LISTENER-FILTER REJECT — IT FIRES ON LISTENERS WITH NO FILTERS AND ON QUIC

`parseListenerFiltersTimeout` has exactly ONE caller, `manager.go:833` inside
`buildListenerRuntimeWithCtx`, which builds BOTH TCP and QUIC listeners and runs the parse
unconditionally (`git grep -n parseListenerFiltersTimeout -- internal/`). Measured (S13, S-QUIC):
a TCP listener with **no** `listener_filters` and `120s` is rejected with the envelope message; a QUIC
listener (the `0104` template, validate) rejects `0.5s` and `120s`. The reference boots a no-filter
listener at `120s` and `0.5s` and serves a plain GET (R13). **The divergence's blast radius is every
listener that spells the field, not only those whose timeout does anything.** ⚠️ The reference was NOT
driven on a QUIC listener carrying the field — owed to the SPEC (§10 item 4).

### 0.5 THE ONLY READER IS THE TCP PATH, AND THE FIELD IS A `uint32` IN THREE PLACES

`rt.lfTimeoutMs` is read only at `manager.go:1356` (`p.Run(…, rt.lfTimeoutMs)`); the QUIC files never
read it (INFERRED from `git grep`, not driven — QUIC runs no pipeline, `100/BRAINSTORM.md` §1.4). The
width lives in `parseListenerFiltersTimeout`'s return type, the `listenerRuntime.lfTimeoutMs` field and
`Pipeline.Run`'s `timeoutMs` parameter — **and ADR-0082 §Consequences (b) states the `uint32` parameter
as part of the contract** (`DECISIONS.md:3068`). Widening it is an ADR-level edit, not a local one.

### 0.6 `61s` IS ENFORCED AT 61 s ON BOTH SIDES — NO 60 s CLAMP — BUT THE SUBJECT IS LATER

Reference (R8, one probe held 70 s): FIN at **61003.8 ms**, `pre_cx_timeout` 0 → 1. P0 (P0f): closed at
**61060.5 ms**, and a 3-concurrent repeat at **61035.2-61035.3 ms**. At `0.5s` and `1s` both sides land
within ~2 ms of the deadline; at 61 s the subject is **35-60 ms late** on two boots. Not a divergence
worth a row (the reference is within 4 ms, the subject within 0.1 %), **but a timing window for a long
arm must be sized for it** (`reference_differential_band_sigma_margin`).

### 0.7 AT `0.001s` AN IMMEDIATE REQUEST RACES THE DEADLINE ON THE REFERENCE — NOT A PINNABLE ARM

Reference (R4): a client that sends the GET immediately on connect is served **9 of 10** and **47 of 50**;
the dropped ones read 0 bytes and a FIN, one `pre_cx_timeout` per drop. P0 (P0c, sequential): **10 of 10**
served. The outcome is a race between the client's first byte and a 1 ms timer on both sides. **A fixture
must NOT pin a served/dropped count at 1 ms** — only a silent-client arm is deterministic there (R4
silent: 1.9-2.3 ms; P0: 1.1-1.5 ms).

### 0.8 `REVIEW_FINDINGS.md`'s `stats_flush_interval` CLAUSE IS CONFIRMED ON BOTH SIDES

`REVIEW_FINDINGS.md` (the `validate/bootstrap` bullet) says `stats_flush_interval` is *"accepted outside
reference PGV bounds."* Measured at this tip: the reference rejects `0s`, `0.0005s`, `300s` and `1000s`
(`Proto constraint validation failed (BootstrapValidationError.StatsFlushInterval: value must be inside
range [1ms, 5m0s))`) and accepts `0.001s` and `299s`; envoy-go's `-mode validate` reads `configuration
OK` rc=0 on **all six** (controller, tip binary, one-listener bootstrap). **Fail-OPEN, the mirror of this
row's fail-CLOSED.** Rejected as THIS row and banked with both sides measured (§4.2).

### 0.9 THE ENVELOPE'S OCCURRENCE SET IS WIDER THAN ADR-0082 AND ADR-0322 — ADR-0081 LEANS ON IT

The router and ADR-0322 name ADR-0082 ¶1 and §Consequences (a), (c) as the envelope's home. The subject
agent's case-insensitive census found **ADR-0081 §Consequences (c)** (`DECISIONS.md:3175`) justifying a
latency claim as *"well below the `listener_filters_timeout` envelope's 1s lower bound"*, and ADR-0082
§Consequences (b) pinning the `uint32` (§0.5). Full set in §3.4.

### 0.10 THE REFERENCE'S REJECT MESSAGES ARE NOT BYTE-STABLE

Every reference reject carries a `goo.gle/debugproto|debugonly|debugstr` suffix that **changes from run
to run for the same input** (reference agent). A pin on a reference message must match the prefix only
(`Invalid duration: Expected positive duration`, `Invalid duration: Duration out-of-range`).

### 0.11 `pre_cx_timeout` UNDER `continue…: true` COUNTS ON THE REFERENCE AT `0.5s` — AS AT `1s`

R2: silent 800 ms then GET → `200 DEFAULT l_a2`, counter 0 → 3 over three runs, while the byte-identical
`1s` matched negative (R2-neg) serves the same body with the counter at 0. P0b agrees (1 vs 0). ⚠️ **The
BODY does not discriminate `0.5s` from `1s` under `true` — only the COUNTER does** (method note 55): a
fixture arm that pins only the body cannot tell "fell through at the deadline" from "the bytes arrived
first". The tip is RED on such an arm only because a `0.5s` listener does not BOOT there; once it boots,
the counter beside its `1s` mirror is the only discriminator.

### 0.12 `manager.go:951-962` HAS DRIFTED — THE FUNCTION IS AT `:950-967`

The router's envelope bullet already warned; confirmed: the doc comment starts at `:950`, the `func` at
`:953`, the check at `:963-965`. **Anchor on `parseListenerFiltersTimeout`** (method note 3).

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

`listener_filters_timeout` accepts every value the reference accepts and rejects every value it rejects:
any non-negative `Duration` whose seconds are at most `9223372035` is accepted and TRUNCATED to whole
milliseconds (a 0 ms result — `0s` or any sub-millisecond value — DISABLES the timeout; absent stays
15 s), negatives and larger values are boot-rejected, and the value is carried end to end in a type that
cannot wrap, so the deadline envoy-go enforces is the deadline the operator wrote.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

| candidate | prod floor (built + run at this tip) | reference measured at this tip? | cross-side surface | severity |
|---|---|---|---|---|
| **envelope lift, width-correct (THIS ROW)** | **`8 9` `manager.go` + `1 1` `pipeline.go`** (P1, §6) | **YES — R1-R14, RB, RR** | boot vs reject; drop timing; `pre_cx_timeout` value | fail-CLOSED boot on reference-valid configs, on EVERY listener carrying the field |
| `stats_flush_interval` PGV bounds (§0.8) | not prototyped; one range check | YES (validate, 6 arms) | reject vs accept | fail-OPEN on configs the reference refuses |
| `downstream_cx_total` post-filter (banked) | not prototyped; moves an `Inc` | phase-100 only | counter value | off by one per dropped cx; moves fixture pins |
| SNI case-insensitivity | `4 1` pattern side + unmeasured input side | phase-99 | serve vs DEFAULT | wrong chain |
| order-dependent pairwise fold | not prototyped | **NO** | serve vs close | wrong chain / close |
| close KIND under `continue…: false` | peeker change | phase-100 | FIN vs RST, **invisible to the harness** | cosmetic on the wire |

The lift was adjudicated NEXT IN LINE by three closed documents; **it is adopted after re-measurement,
not inherited**, and adopted with a DIFFERENT shape than they implied (§0.2). It wins on four counts: it
is the only candidate whose reference side is measured end to end at this tip INCLUDING the edge arms
that decide the repair; its production floor is one function, one field and one parameter type; it
completes the ADR (ADR-0082) and function that row 100 just touched, while that context is current; and
its severity is the higher of the two fail-direction candidates — a bootstrap that runs on the reference
**will not start** on envoy-go (fail-closed on migration), where `stats_flush_interval`'s fail-open
needs an operator to have written a config the reference would refuse.

`stats_flush_interval` is smaller in lines and is **banked as the next row, measured** (§4.2). It loses on
severity and on cohesion, not on size — **a trade-off, stated.**

### 1.3 Why the WIDTH fix is folded in and not banked

- **It is COUPLED.** Lifting the upper bound without widening the type admits `4294968s` and drops at
  705 ms — the lift itself MINTS the wrap divergence (§0.2). Same shape as row 100's `0s` fold
  (`100/BRAINSTORM.md` §1.3): a repair that mints a divergence must carry its own antidote.
- **It is the SAME row's validity rule.** The negative and out-of-range rejects are the lower and upper
  edges of the lifted envelope; without them the lift is fail-open on three measured arms.

### 1.4 What this row does NOT buy — stated plainly

- It does **not** change the reference-vs-subject reject MESSAGES (both fail closed; the reference's are
  not byte-stable anyway, §0.10). Parity is in rc and in which values are refused.
- It does **not** pin the 1 ms immediate-request race (§0.7) or the subject's 35-60 ms lateness at 61 s
  (§0.6).
- It does **not** touch `stats_flush_interval` (§4.2), the close KIND, `downstream_cx_total`, or the
  `downstream_listener_filter_{remote_close,error}` names (all banked by row 100).
- It does **not** decide QUIC behaviour beyond the parse: a QUIC listener carrying the field goes from
  REJECTED to ACCEPTED-AND-IGNORED; **the reference's QUIC behaviour with the field is UNMEASURED** — the
  SPEC measures it before chartering the QUIC arm (§10 item 4).

---

## 2. The divergence, MEASURED — both sides, matched negatives, timed observables

**Client:** a Go probe per side (`refclient`, `probe-bin`; both in the session scratchpad) with modes
`silent` (connect, send nothing, time the close or report "open at H ms") and `get` (silent W ms, then
`GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n`). **Base config** on both sides: fixture `0125`'s
`listenerTmpl` — one listener with `tls_inspector`, `fc_indexed` on `transport_protocol: tls`, a
`default_filter_chain`, each an HCM `direct_response` 200 naming the chain; every arm is a one-line diff
of `listener_filters_timeout` (or of `continue…`, or of the `listener_filters` block).

### 2.1 Reference (`envoyproxy/envoy@sha256:7edd5b0fd763…`, digest verified)

| arm | value / `continue…` | validate | boot | driven observable | `pre_cx_timeout` |
|---|---|---|---|---|---|
| R1 | `0.5s` / false | rc0 | runs | 10 seq: 500.8-501.7 ms (mean 501.2, σ 0.3); **50 concurrent: 501.1-502.2 ms (mean 501.8, σ 0.3)**; all FIN, 0 bytes | +10, +50 by delta |
| R2 | `0.5s` / true | rc0 | runs | GET at 800 ms ×3 → `200 DEFAULT`; silent 3000 ms ×2 → open | 0 → 3 → 5 |
| R2-neg | `1s` / true (byte-identical otherwise) | rc0 | runs | GET at 800 ms ×3 → `200 DEFAULT` | 0 → 0 |
| R3 | `1s` / false | rc0 | runs | ×5: 1000.8-1002.8 ms FIN | 5 |
| R4 | `0.001s` / false | rc0 | runs | silent ×5: 1.9-2.3 ms FIN; immediate GET: 9/10 and 47/50 served (§0.7) | one per drop |
| R5 | `0.0005s` / false | rc0 | runs | 3000 ms ×3: **open** | 0 |
| R6 | `0.000000001s` / false | rc0 | runs | 3000 ms ×3: **open** | 0 |
| R7 | `1.0009s` / false | rc0 | runs | ×5: 1000.8-1002.1 ms (indistinguishable from `1s`; truncation settled by RR) | 5 |
| R8 | `61s` / false | rc0 | runs | one probe held 70 s: **FIN at 61003.8 ms** | 0 → 1 |
| R9 | `120s`, `3600s`, `86400s` | rc0 | runs | 3000 ms: open | 0 |
| R10 | `4294968s` | rc0 | runs | 3000 ms ×3: **open** (no wrap) | 0 |
| R11 | `315576000000s` / `315576000001s` | **rc1** `Invalid duration: Duration out-of-range` / JSON parse `duration out of range` | exit 1 | — | — |
| RB | `9223372035s`, `9223372035.999999999s` / `9223372036s` | rc0, rc0 / **rc1** `Duration out-of-range` | runs (multi-listener container) / — | 3000 ms: open | 0 |
| R12 | `-1s`, `-0.5s` | **rc1** `Invalid duration: Expected positive duration` | exit 1 | — | — |
| R13 | NO listener filters, `120s` and `0.5s` | rc0 | runs | GET → `200 DEFAULT`; silent 3000 ms open | 0 |
| R14 | `0s` / false (control) | rc0 | runs | 3000 ms ×3: open | 0 |
| RR | `0.0009s`, `0.000999999s` / `0.001s`, `0.0015s`, `0.0019s` / `0.002s` | rc0 | runs | 20/20 open / ~2.0 ms mean / ~2.9 ms mean | 0 / per drop |

Every drop was a FIN with 0 bytes, observed both through the published port and from inside the
container's own network namespace (a silent client has no unread peeked byte, so `100/SPEC.md` §0.1's
RST case does not arise). R1's counter started at 4 from a stray two-probe run before the arm; **the
deltas are the clean figures.** ⚠️ Probing the container IP directly hung; every probe went through `-p`.

### 2.2 Subject (envoy-go at `1ae29a1b`, the tip binary)

| arm | validate | boot | message source |
|---|---|---|---|
| `0.5s`, `0.001s`, `0.0005s`, `0.000000001s` | rc1 `listener: "l_x": listener_filters_timeout 500ms` (`1ms`, `500µs`, `1ns`) `is outside the supported [1s, 60s] envelope` | rc1 | envelope check |
| `61s`, `120s`, `3600s`, `4294968s` | rc1 (`1m1s`, `2m0s`, `1h0m0s`, `1193h2m48s`) | rc1 | envelope check |
| `315576000000s` | rc1 `… 2562047h47m16.854775807s is outside …` (Go's `AsDuration` SATURATES) | rc1 | envelope check |
| `315576000001s` | rc1 `bootstrap: protojson: … google.protobuf.Duration value out of range` | rc1 | protojson |
| `-1s`, `-0.5s` | rc1 (`-1s`, `-500ms` `is outside …`) | rc1 | envelope check |
| `1.0009s`, `1s`, `60s`, `0s` | rc0 | runs | — |
| no filters, `120s` | **rc1**, envelope message | rc1 | envelope check (§0.4) |
| QUIC (`0104` template), `0.5s` / `120s` | **rc1**, envelope message | — | envelope check (§0.4) |
| `500ms` (spelling control) | rc1 `invalid google.protobuf.Duration value "500ms"` | — | protojson — **a probe must spell `0.5s`** |

**Divergence set at the tip, by rc:** the reference boots and envoy-go refuses `0.5s`, `0.001s`,
`0.0005s`, `0.000000001s`, `61s`, `120s`, `3600s`, `86400s` (by `3600s`'s class), `4294968s`,
`9223372035s`, and every no-filter or QUIC listener carrying any of them. **Agreement at the tip:**
`315576000000s`, `315576000001s`, `-1s`, `-0.5s` (both reject), and `1s`, `60s`, `1.0009s`, `0s` (both
boot). ⚠️ **The four agreeing rejects agree for the WRONG reason on envoy-go** (the envelope, not a
validity rule) — method note 58: the tip could not have answered anything else, and P0 shows that
removing the envelope removes the agreement (§0.2).

### 2.3 The mechanism, at the tip, by symbol

- `parseListenerFiltersTimeout` (`manager.go`): nil → 15000; `seconds==0 && nanos==0` → 0 (disabled,
  ADR-0322); else `ms := d.AsDuration() / time.Millisecond`, reject `ms < 1000 || ms > 60000`, return
  `uint32(ms)`. **`AsDuration` saturates at the Go `time.Duration` maximum** and **`uint32` wraps** —
  both hidden today by the envelope.
- `listenerRuntime.lfTimeoutMs uint32`; the one reader, `p.Run(…, rt.lfTimeoutMs)` at `manager.go:1356`.
- `Pipeline.Run(…, timeoutMs uint32)` (`listenerfilter/pipeline.go`): arms `context.WithTimeout(ctx,
  time.Duration(timeoutMs)*time.Millisecond)` only when `timeoutMs > 0`.

---

## 3. Hazards for the SPEC

### 3.1 The obvious shape passes every adjudicated arm and is wrong (§0.2)

A gate made only of `0.5s`, `61s`, `120s` is GREEN under P0. **The discriminating arms are `4294968s`
(held open ≥ 1 s past 704 ms), a negative value (boot-reject), and `315576000000s` / `9223372036s`
(boot-reject).** Run the chosen shape AND P0 through the same arms (method note 50: identical greens mean
the suite is blind).

### 3.2 Two existing tests pin the envelope and go RED under ANY lift

`TestParseListenerFiltersTimeoutBelowFloorErrors` (input `500ms` via `durationpb.New`) and
`TestParseListenerFiltersTimeoutAboveCapErrors` (input `90s`) — both RED under P0 and P1 (274 `=== RUN`,
exactly these two FAIL). **Read their inputs before re-pointing** (method note 93): `500ms` built in Go
is a valid `Duration`, so the below-floor test becomes an ACCEPT arm (500 ms, enforced); `90s` becomes an
ACCEPT arm. The new REJECT arms (negative, `seconds > 9223372035`) are NEW tests, not re-points.
`TestParseListenerFiltersTimeoutInRange`, `…Default` and `…ZeroDisables` stay green.

### 3.3 The `Pipeline.Run` signature is a contract (§0.5)

Widening `timeoutMs` touches ADR-0082 §Consequences (b), the `runPeekPipeline` helper in
`pipeline_deadline_test.go` (the one typed caller; the seven untyped-constant callers in
`pipeline_test.go` compile unchanged), and `manager.go:1356`. **Alternatives the SPEC must adjudicate
against a reference MEASUREMENT, not by size** (method note 37): (A) widen to `uint64` ms (P1); (B) carry
a `time.Duration` end to end; (C) keep `uint32` and CLAMP `≥ 2³²` ms to `math.MaxUint32` (~49.7 days) —
observably different from the reference only past 49.7 days, and that difference is unmeasurable in any
gate; **(C) is a fix shape that encodes a parity answer nobody can measure.** P1's `uint64` ms times
`time.Millisecond` stays inside `int64` nanoseconds at the maximum accepted value (9223372035999 ms →
9.223372035999e18 ns < 9.223372036854775807e18) — **measured open at 3000 ms, not only computed** (§6).

### 3.4 Occurrence set of the claims the lift falsifies

Present-tense envelope or `uint32` claims, by the subject agent's census (case-insensitive, pathspec-scoped)
plus the controller's:

- **Code comments:** `manager.go:166-167` (`listenerRuntime` field doc, `[1000, 60000]`), `:831-832`
  (`envelope [1000, 60000]`), `:950-952` (the function doc), and the error string at `:963-965`.
- **Tests:** the two in §3.2 (`manager_test.go:3339-3390`).
- **Docs:** `BEHAVIOR_CONTRACT.md:4359` (the `Per-pipeline timeout` bullet — anchor on the LITERAL) and
  `:4338`; ADR-0082 heading, ¶1 and §Consequences (a), (b), (c) (`DECISIONS.md:3040`, `:3052`,
  `:3066-3070`) — **superseded clause by clause by the new ADR, NEVER edited** (the house form);
  ADR-0081 §Consequences (c) (`:3175`); ADR-0322 ¶7, §Consequences (f), (g) and its status line
  (`:19491`, `:19507`, `:19552`, `:19567-19568`, `:19642-19644`) — historical narration of an ACCEPTED
  ADR, adjudicate past-tense vs present-tense per method note 63.
- **Not hits:** the CLI help, `internal/stats/name.go`, `pipeline.go` and `tls_inspector.go` comments, and
  every fixture README (no fixture states the envelope). `REVIEW_FINDINGS.md:185` names the timeout, not
  the envelope.
- **Historical phase docs** (`07.2/*`, `98/PLAN.md`, `100/*`) — closed-phase evidence, not edited.

### 3.5 Timing arms need stated windows

`0.5s` drop: reference 500.8-502.2 ms (σ 0.3, n=60), P0 500.4-502.0 ms (σ 0.4, n=60), P1 500.6-500.9 ms
(n=3). A `[350, 900]` class window is ~1000σ wide; the SPEC sizes it on the `0125` rule. **A 61 s arm is
~61 s of wall clock per side** and the subject was 35-60 ms late (§0.6): a candidate for a unit arm with
an injected clock rather than a differential arm. **The `4294968s` arm needs a hold that outlasts 704 ms
by a margin** (≥ 1500 ms) to discriminate P0.

### 3.6 The fixture MUST NOT pin

The 1 ms immediate-request outcome (§0.7); any reference reject message byte-for-byte (§0.10); the body
alone on a `true` arm (§0.11 — pin the counter by VALUE beside its `1s` mirror); a close KIND.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The naive lift P0 (`0 3`) — **REJECTED: wrong on three measured arms** (§0.2)

### 4.2 `stats_flush_interval` PGV bounds — **REJECTED as THIS row; banked as the NEXT candidate, BOTH SIDES MEASURED** (§0.8)

The reference refuses `0s`, `0.0005s`, `300s`, `1000s` with `value must be inside range [1ms, 5m0s)`;
envoy-go accepts all six arms (`parseStatsSinks`: `v > 0` else the 5 s default — so `0s` silently
becomes 5 s). Smaller than this row, a different family (Observability / bootstrap), fail-OPEN. **Not
prototyped** — the floor is a FLOOR when someone builds it, and its blast radius (fixtures or tests that
set `stats_flush_interval` outside `[1ms, 5m)`) is uncensused.

### 4.3 `downstream_cx_total` post-filter accounting — **REJECTED; unchanged from `100/BRAINSTORM.md` §4.2**, needs a census of every fixture pin on `cx_total`.

### 4.4 The close KIND under `continue…: false` — **REJECTED: invisible to the harness** (`100/SPEC.md` §0.1). This stage's silent-client drops were FIN on the reference even in-namespace, consistent with the unread-byte condition.

### 4.5 The order-dependent pairwise fold in `SelectChain` — **REJECTED: reference side UNMEASURED** (`100/BRAINSTORM.md` §4.3).

### 4.6 SNI case-insensitivity · nested-descent precedence · duplicate-matcher reject parity · duplicate listener addresses (validate-only) · the `rank > 2` guard · `server_names` partial-wildcard acceptance — **REJECTED; unchanged from `100/BRAINSTORM.md` §4.4-4.9, not re-derived.**

### 4.7 The other `REVIEW_FINDINGS.md` "parsed but never enforced" clauses — `maximum_ring_size`, redis `op_timeout` — **REJECTED: unmeasured at this tip**; the same knob-not-enforced class as row 100 (method note 82), each needing its own both-sides measurement. `maximum_ring_size` IS now range-checked at parse (`internal/cluster/manager.go`, `value must be less than or equal to 8388608`); whether it is APPLIED was not measured.

### 4.8 The HCM `stat_prefix` duplicate-registration panic · the driver-owned receiver port race · the dead-port false-green · `ROADMAP.md`'s stale swallowed-panic claim inside a sentinel window — **REJECTED; unchanged, not re-derived.** The sentinel-window claim is **recorded, not tidied** (margin of one).

---

## 5. Family attribution

**A Listener / listener-filter MAINTENANCE row claiming NO family ordinal**, on the row-85-through-91 and
95-100 precedent. It repairs a landed deliverable (phase 07.2's ADR-0082 envelope) and extends no family
charter.

---

## 6. The cost FLOOR — prototyped, run, reverted

**P1** (controller, throwaway worktree `envoy-go-p101-p1` off `1ae29a1b`, since removed — `git worktree
list` shows only `master` and this stage's worktree):

- `parseListenerFiltersTimeout` returns `uint64`: nil → 15000; `seconds < 0 || nanos < 0` → reject
  (`expected a positive duration`); `seconds > 9223372035` → reject (`duration out of range`); else
  `uint64(seconds)*1000 + uint64(nanos)/1e6` — computed from the FIELDS, never from `AsDuration` (which
  saturates). `listenerRuntime.lfTimeoutMs` and `Pipeline.Run`'s `timeoutMs` widened to `uint64`.
- `git diff --numstat`: **`manager.go 8 9`**, **`pipeline.go 1 1`**, **`pipeline_deadline_test.go 1 1`**.
  `gofmt -l` empty, `go build ./...` and `go vet ./internal/listener/...` clean.
- `go test -count=1 -v ./internal/listener/...`: **274 `=== RUN`**, rc=1, **exactly the two envelope pins
  FAIL** (§3.2) — identical to P0's failure set, **so the existing suite cannot tell P0 from P1**
  (method note 50).
- **Driven** (P1 binary, `-mode validate` then a real boot, silent probes, `/stats`):

| arm | P1 | reference |
|---|---|---|
| `4294968s` | rc0; **open at 3000 ms ×3**, counter 0 | open, 0 ✅ |
| `9223372035s`, `9223372035.999999999s` | rc0; `9223372035s` open at 3000 ms ×3, counter 0 | rc0, open ✅ |
| `9223372036s`, `315576000000s` | rc1 `duration out of range: seconds …` | rc1 ✅ |
| `-1s`, `-0.5s` | rc1 `expected a positive duration` | rc1 ✅ |
| `0.0005s` | rc0; open at 3000 ms ×3, counter 0 | open, 0 ✅ |
| `0.5s` / false | rc0; **500.6-500.9 ms** ×3, counter 3 | 500.8-502.2 ms ✅ |
| `61s` | rc0 (not driven on P1; P0f drove it) | rc0 ✅ |
| no filters, `120s` | rc0 | rc0 ✅ |

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE** (`reference_measured_prototype_is_a_lower_bound`). It omits: the
four comment sites (§3.4), the two test re-points and the new reject/wrap/sub-ms unit arms, the SPEC's
width decision (§3.3), a fixture, the new ADR and the `BEHAVIOR_CONTRACT.md` bullet. **Fixture floor by
SHAPE:** a plaintext, PKI-free, multi-listener, silent-client fixture is exactly `0125`'s shape (**+836**,
`git diff --numstat` at the phase-100 IMPL) — the precedent to re-measure; ⚠️ **whether to EXTEND `0125`
or add `0126` is a SPEC decision** (extending moves a landed fixture's pins; `0126` takes `15126` on the
`15000 + index` convention, which reads ZERO hits at this tip — re-census before use).

**Stat surface: `+0` predicted** — the lift adds no name (`downstream_pre_cx_timeout` exists since row
100). A `+0, UNCHANGED` ledger entry in the phase-96-through-99 form, quoting no absolute.

---

## 7. The differential measurement

### 7.1 There is no existing gate

`0125` configures `1s`, `0s` and the default only; no fixture spells a value outside `[1s, 60s]`, so no
fixture can see the divergence — both sides would have to boot, and envoy-go does not.

### 7.2 What the gate must do

Boot BOTH sides on listeners the tip REFUSES — so the tip is RED at boot on the whole fixture, which is a
coarse RED; the per-arm discrimination between P0 and the chosen shape must come from: a `0.5s` silent
drop (timed, window per §3.5), a `0.5s`/`true` counter pin beside a `1s`/`true` mirror (§0.11), a
`4294968s` silent hold outlasting 704 ms (**the P0 killer**), a `0.0005s` silent hold (disabled), and a
no-filter listener at `120s` serving a GET. The REJECT arms (negative, `9223372036s`) cannot live in a
two-side-booting fixture — **unit arms, plus a validate-mode check on each side if the harness can carry
one** (SPEC to decide).

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN ADD

All commands copied verbatim from `next-prompt.txt`, `/usr/bin/grep` named.

### 8.1 PRE-ADD, at `1ae29a1b` (`ROADMAP.md` 250 lines, tail row 100 `done` at `:162`)

(1) **SILENT** · (2) **SIX** at `:210 :216 :222 :232 :238 :246` · (3) **SILENT**. `stop` verified absent
at the git root.

### 8.2 The four NCs and the check-(2) positive control, PRE-ADD — ALL FIRED

NC-A: substitution inspected first, `NC LANDED? [ in-progress ]`, then **ONE** line, `NOT DONE: row 62`
· NC-B (`want=131`): **ONE**, `GATE FAIL: examined 132 data rows, expected 131` · NC-C: residual **0**,
`NEVER OPENED: gRPC   <- NC FIRED` · NC-D: **96 / 68** under `--` · check-(2) positive control:
residual **0**, **6** substitutions asserted.

### 8.3 Escape-aware malformed set and per-line digests, PRE-ADD

`sed 's/\\|//g' ROADMAP.md | awk -F'|' '/^\| *[0-9]/ && NF!=8'` → exactly **{57, 69}** at file lines
**119** (NF 9) and **131** (NF 10). Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f |
md5sum`, first 12 hex): `210 10d7807bf02d` · `216 4a92f7e62fc6` · `222 2a7eb298b9fd` ·
`232 242e53c6f7a3` · `238 b2680e6f4fbf` · `246 6caa1c3ce0e7` — **byte-identical to the phase-100 close.**

### 8.4 POST-ADD — measured on the other side of this stage's own ADD

Row 101 installed after `:162` as `in-progress`, gated FIRST in a scratch file: **8 fields naive, 8
escape-aware**, and **zero** hits for either sentinel match phrase, for the bare word `deferred`
(case-insensitive) and for `-family row`. `ROADMAP.md` **250 -> 251** (`git diff --numstat`: `1 0`).
Re-run at `want=133`:

| check | pre-ADD (§8.1-8.2) | **post-ADD** |
|---|---|---|
| (1) | SILENT | **ONE** — `NOT DONE: row 101` |
| (2) | SIX at `:210 :216 :222 :232 :238 :246` | **SIX at `:211 :217 :223 :233 :239 :247`** — every window shifted +1, as an insertion ABOVE them must |
| (3) | SILENT | SILENT |
| NC-A (row 62 doctored) | ONE | **TWO** — `NOT DONE: row 62`, `NOT DONE: row 101` (substitution inspected: `NC LANDED? [ in-progress ]`) |
| NC-B | ONE at `want=131` | **TWO at `want=132`** — `NOT DONE: row 101`, `GATE FAIL: examined 133 data rows, expected 132` |
| NC-C | FIRED, residual 0 | FIRED, residual 0 |
| NC-D (`--`) | 96 / 68 | **96 / 68** — unmoved; the new cell spells no `-family row` |
| check-(2) positive control | residual 0, 6 substitutions | residual 0, 6 substitutions |
| escape-aware malformed set | {57, 69} at `:119`, `:131` | **{57, 69} at `:119`, `:131`** — unmoved (both above the insert) |
| row 101 NF | — | **8 naive, 8 escape-aware** (at `:163`) |

Per-line md5 at the SHIFTED lines, trailing newline INCLUDED: `211 10d7807bf02d` · `217 4a92f7e62fc6` ·
`223 2a7eb298b9fd` · `233 242e53c6f7a3` · `239 b2680e6f4fbf` · `247 6caa1c3ce0e7` — **all six
byte-identical to §8.3: the windows MOVED and did not CHANGE.** The stale swallowed-panic claim now sits
at `:233`, **recorded, not tidied.**

Fixture registration, re-run verbatim (the blank-import extractor over `test/differential/runner_test.go`):
**dirs 127 = imports 127**, both `comm` directions EMPTY under the `(imports, dirs)` order, split **103
`driver/` + 24 `inputs/`** — unmoved, as a BRAINSTORM adds no fixture.

⇒ **THE SENTINEL DOES NOT FIRE on either side of this ADD. `stop` was evaluated and NOT created**
(absent at the git root and in the stage worktree).

---

## 9. Findings the next stage must not re-learn

1. **The naive lift is wrong** — `uint32` wraps `4294968s` to 704 ms; negatives boot (§0.2).
2. **The reference's upper bound is `9223372035.999999999s`**, not the protobuf maximum (§0.1).
3. **Sub-millisecond values DISABLE the timer on the reference, by truncation** (§0.3) — envoy-go's
   arithmetic already agrees.
4. **The reject fires on no-filter and QUIC listeners too** (§0.4).
5. **The existing suite cannot tell P0 from P1** — both fail exactly the two envelope pins (§6).
6. **Reference reject messages carry a run-varying suffix** — pin the prefix (§0.10).
7. **At 1 ms an immediate request races the timer** — never pin it (§0.7).
8. **`stats_flush_interval` is fail-OPEN, both sides measured** — the next candidate (§4.2).
9. **Compute milliseconds from the Duration FIELDS** — `AsDuration` saturates (§2.3).

---

## 10. What the SPEC owes

1. **Choose the width shape against §3.3** — (A) `uint64` ms, (B) `time.Duration`, (C) clamp — and gate
   it on the P0-killing arms (§3.1), running the chosen shape AND P0 through the same arms.
2. **Specify the validity rule exactly**: accept `seconds ∈ [0, 9223372035]` with non-negative nanos;
   truncate to ms; 0 ms ⇒ disabled; absent ⇒ 15 s; reject negatives and larger seconds — and decide the
   error wording (the reference's is not byte-stable; envoy-go keeps its `listener: %q:` prefix).
3. **Re-point the two envelope pins after reading their inputs** (§3.2) and add RED-at-tip unit arms for
   the wrap, negative, out-of-range and sub-ms edges.
4. **MEASURE the reference on a QUIC listener carrying the field** (§0.4, §1.4) before deciding the QUIC
   arm; envoy-go goes from reject to accept-and-ignore.
5. **Decide `0125`-extend vs `0126`** (§6) and charter the fixture per §7.2: ports censused, windows
   stated, the `4294968s` P0-killer arm, the `true` counter pin with its `1s` mirror, nothing from §3.6.
6. **Decide whether a 61 s arm is a differential arm or a unit arm** (§3.5).
7. **Draft the new ADR's §Context** (next-free **ADR-0323**, the house `> **STATUS: PROPOSED` block
   form) superseding ADR-0082 ¶1 and §Consequences (a), (b), (c) and noting ADR-0081 (c)'s dependency;
   **predict the ledger delta (+0 NAMES)**.
8. **Re-derive the occurrence set** (§3.4) and state what the row does NOT buy (§1.4).

---

## 11. Probe hygiene

- **Worktrees:** stage worktree `/home/esa/git/envoy-go-wt-phase-101-brainstorm` (branch
  `wt-phase-101-brainstorm`, off `1ae29a1b`); the subject agent's throwaway `envoy-go-p101-subj-probe`
  and the controller's `envoy-go-p101-p1` **created and removed**, verified by `git worktree list`.
- **Docker:** one agent only (the reference agent), containers `p101ref-*`, torn down by name
  (`docker ps -a --filter name=p101ref-` → header only); no network created; no other container touched
  (a foreign `golink-ai` container was present and left alone).
- **Ports:** band `16700-16799`, censused first (`git grep -In '167[0-9][0-9]' -- test/ internal/ cmd/`
  → every hit a substring of `16777215` / `16777216`, H2 frame-size bounds and 16 MiB caps; no port;
  nothing bound under `ss -tan` / `ss -uan`). Reference host ports `16700-16717`; subject agent
  `16750-16799`; controller `16780`, `16781`, `16790`, `16791`. ⚠️ **This paragraph spells band numbers;
  it is a hit in the next census.**
- **Scratch:** everything under the session scratchpad (`ref/`, `subj/`, `ctl/`); nothing in any
  worktree. **No production `.go` written, none of the six gates run — a BRAINSTORM's SCOPE, not an
  omission.** The one standing departure (no `REVIEW.md`, none of 93-100) is not this stage's to fix.
