# Phase 101 — `listener-filters-timeout-envelope-lift` — SPEC

**Stage:** SPEC (lifecycle **1 -> 2**). Worktree `/home/esa/git/envoy-go-wt/phase-101-spec` off master
`762c8411`, branch `wt-phase-101-spec`. **Governs:** `BRAINSTORM.md` (535 lines). It was read as evidence and
re-derived before any of it was trusted. Written under the 2026-07-12 standing directive: **no human
consulted.**

**The decision in one paragraph.** `listener_filters_timeout` is carried as a **`uint64` of milliseconds end
to end (shape A)**. `parseListenerFiltersTimeout` returns `uint64` and computes the value from the
Duration's FIELDS, never from `AsDuration` (which saturates). The rules: nil gives **15000**; negative seconds
or negative nanos are **REJECTED** (`expected a positive duration`); seconds above **`9223372035`** are
**REJECTED** (`duration out of range`); anything else is `seconds*1000 + nanos/1e6`, which **truncates** to
whole milliseconds, so `0s` and every sub-millisecond value give **0 = DISABLED**. `listenerRuntime.lfTimeoutMs`
and `Pipeline.Run`'s `timeoutMs` widen to `uint64`. The `[1000, 60000]` envelope is gone. The production
floor is `manager.go` **`8 9`**, `pipeline.go` **`1 1`** and `pipeline_deadline_test.go` **`1 1`** by `git diff
--numstat`. That reproduces `BRAINSTORM.md` §6 exactly, rebuilt at this tip by a separate agent. The shape was
chosen against FOUR alternatives, all built and run at this tip through one 15-arm unit table and the real
binary (§3). **P0**, the naive lift, fails 7 of those arms and drops `4294968s` at 705 ms. **C**, a `uint32`
clamp, fails 2 value arms and encodes a parity answer no gate can measure. **B**, a `time.Duration` end to
end, is **rejected on MECHANISM**: it compiles, vets and lints clean while silently turning every
untyped-constant `Run(…, 1000)` caller into **1000 ns**. Measured, two pipeline tests then fail 18 and 19 of
20 runs, so the defect shows up as a FLAKE, not a clean red. **The reference on a QUIC listener was measured
at this stage (owed item 4):** it accepts every in-range value, serves HTTP/3, logs nothing about the field,
and keeps `downstream_pre_cx_timeout` at 0. The only rejects are the same two Duration-range errors as on
TCP. So envoy-go's post-lift accept-and-ignore on QUIC is the parity answer, and no QUIC differential arm is
chartered (§6). Fixture **`0126-listener-filters-timeout-envelope`** is chartered as a NEW directory; extending
`0125` is rejected (§7). The stat surface is **+0 NAMES**.

---

## 0. What this stage refuted or sharpened — by execution

Every item comes from something that was run. **S-agent** is this stage's Docker-free subject agent (five
shapes, throwaway detached worktrees off `762c8411`, all removed). **R-agent** is its Docker-only reference
agent (image by digest, containers `p101spec-ref-*`, all torn down by name).

### 0.1 🔴 `BRAINSTORM.md` §3.3 lists (B) `time.Duration` as a peer of (A). It is NOT: B silently changes the UNIT of every untyped caller

B was written the obvious way: `Pipeline.Run(…, timeout time.Duration)`, arming only when `> 0`. It passes
`go build ./...`, `go vet ./internal/listener/...` and `GOTOOLCHAIN=go1.26.2 golangci-lint run
./internal/listener/...` (lint shown live by a planted file that fired `unused` and `misspell`). But the
**seven untyped-constant callers** in `pipeline_test.go` (`:31 :51 :79 :104 :131 :153 :168`) still compile, and
`Run(…, 1000)` now means **1000 ns**. Measured with `go test -count=20 -run TestPipelineRun`:
`TestPipelineRunContinuePath` and `…StopIterationPath` FAIL **18 and 19 of 20**. That is a race between a 1 µs
timer and the filter, so it shows up as a FLAKE and not as a deterministic red. `InRange` and `Default` also
fail on units (`lfTimeoutMs = 5000000000, want 5000`), and the field keeps the now-false name `lfTimeoutMs`.
Once all of that is repaired (B', five files: `pipeline_test.go` `7 7` and `manager_test.go` `2 2` added), B'
and A are **indistinguishable on every arm** (§3.2). **The rejection is on mechanism, not size** (method note
37): a width change whose type system accepts the old literals with a new meaning is the hazard class of
method note 36, a value reused under a changed invariant.

### 0.2 🔴 `BRAINSTORM.md` §1.4 and §10 item 4 — the reference's QUIC behaviour, MEASURED: accept and ignore

R-agent, QUIC listener `l_q` (the `0104` shape, `inline_string:` PEM, UDP published with `-p …/udp`), a real
HTTP/3 GET from a quic-go v0.54.1 client (the repo's version). For absent, `0.5s`, `120s`, `0s`, `0.0005s` and
`4294968s`, every arm gave: `--mode validate` rc0; boot LIVE; **200 `QUIC l_q`**; a handshake plus 2000 ms
with no request, then a GET, **served**; `listener.0.0.0.0_16801.downstream_pre_cx_timeout: 0`; and no log line
naming the field (the only warning is the generic connection-limit one, also present in the control). `-1s`
gives rc1 `Invalid duration: Expected positive duration`; `9223372036s` gives rc1 `Invalid duration: Duration
out-of-range`. Those are **the same generic Duration errors as on TCP** (R-agent's TCP sanity arm reproduced
BRAINSTORM R12 and R10 at this tip). ⚠️ **The silent arm is NOT a discriminator by itself.** A QUIC listener
runs no listener filters, and `0125`'s `l_nofilt` shows that a filterless TCP listener also books 0. So "not
cut, counter 0" means "the field does nothing on a filterless listener", not "QUIC is special" (method notes
55 and 58). An extra probe: the reference **rejects** `tls_inspector` in a QUIC listener's `listener_filters`
(`Didn't find a registered implementation for 'envoy.filters.listener.tls_inspector'`), so there is no
TCP-category filter a QUIC arm could carry.

### 0.3 THE OCCURRENCE SET IS WIDER THAN `BRAINSTORM.md` §3.4 — FOUR MORE `DECISIONS.md` HITS

S-agent's census (§5), resolved by backward `^## ADR-` search, adds four hits §3.4 lacks. **ADR-0078
`:3221`** (a Task-9 field list naming `lfTimeoutMs uint32`). **ADR-0082 `:3044` and `:3074`** (the Doctrine
line and Lands-in-task naming the envelope). **ADR-0320 `:19355`** (narration that the contract bullet was
left standing). ⚠️ **The census's own two matchers are each blind to part of the set**: the envelope-pattern
pathspec as briefed excluded `DECISIONS.md`, and the name pattern misses `:3044 :3070 :3074 :19355 :19491
:19552 :19567-19568 :19642-19644`. The union was needed (method note 49: ask what it SPELLED).

### 0.4 TWO OF THE NEW REJECT ARMS ARE GREEN AT THE TIP — BY THE ENVELOPE, FOR THE WRONG REASON

`U-negnanos` (`{0, -5e8}`) and `U-pmax` (`315576000000s`) PASS at the tip, because the envelope rejects them.
Only P0 turns them red (P0 accepts them as `4294966796` and `2077252342` ms). **Their falsifiability at the
tip is structural (method note 61)**, so the IMPL must score them per arm against an NC (§9), never read them
as TDD failures. The other eleven new arms are RED at the tip (§3.2).

### 0.5 `Pipe-max` DOES NOT DISCRIMINATE P0 — ONLY `Pipe-wrap` DOES

Driving `Pipeline.Run` with the parsed maximum (`9223372035.999999999s`) and holding 1 s: P0 PASSES it,
because the value wraps to 2077251487 ms (~24 days) and still outlasts the hold. Only the `4294968s` value,
whose wrapped result (704 ms) is SHORTER than the hold, reddens P0 (**returned at 704.9 ms**). The
discriminating arm is the one whose wrap lands inside the hold. An arm is a discriminator only if the mutation
has a path to it (method note 7d).

### 0.6 THE CLAMP (C) IS NOT MERELY "UNMEASURABLE" — IT FAILS TWO VALUE ARMS

`BRAINSTORM.md` §3.3 says C differs from the reference "only past 49.7 days". Behaviourally that is true:
`Pipe-wrap` PASSES under C, and every driven arm agrees (§3.3). But its PARSE returns `4294967295` for
`4294968s` and for the maximum, so `U-wrap` and `U-max` FAIL. C therefore needs its own test oracle, and a
PARSE-level oracle that states "the value the operator wrote" is exactly what C cannot satisfy. **Rejected:**
it bakes into the type a parity answer that no behavioural gate can see (method note 37), and it keeps the
narrower contract ADR-0082 (b) would otherwise stop stating.

### 0.7 `BRAINSTORM.md` §7.2's "validate-mode check if the harness can carry one" — DECIDED: UNIT ARMS ONLY

The differential harness boots both sides or fails the fixture. There is no validate-mode arm in it, and a
reject fixture would be RED on BOTH sides at every tip. The one reject site is `parseListenerFiltersTimeout`,
whose only caller (`manager.go:833`, re-confirmed by `git grep -n`) is inside `buildListenerRuntimeWithCtx`,
shared by TCP and QUIC. S-agent drove `-mode validate` on the A binary for `-1s`, `-0.5s`, `9223372036s` and
`315576000000s`. Every one read rc1 with the new message, so the binary's validate path REACHES the unit-tested
site (method note 51: proven by the message, not by the rc). A `cmd/envoy-go` `TestEnvoyGoBinary_ModeValidate`
arm would add no path.

### 0.8 CONFIRMED, NOT REFUTED

A's floor `8 9` / `1 1` / `1 1` reproduced exactly. The existing suite fails **exactly** the two envelope pins
under P0, A, B' and C (method note 50: blind between them). No shape moves `pipeline.go:32` (the `Run`
signature), `:33-37` (md5-identical) or `:43` (the cited `context.WithTimeout` line). B changes `:43`'s TEXT
but not its number. `parseListenerFiltersTimeout`'s single caller. The reference's TCP `-1s` reject and
`4294968s` accept.

---

## 1. Scope, restated as a decision

**In:** the validity rule and width (§2), the re-pointed envelope pins and the new unit arms (§4), the
occurrence-set reconciliation (§5), fixture `0126` (§7), ADR-0323 (§8), `BEHAVIOR_CONTRACT.md`'s `Per-pipeline
timeout` bullet and a `+0` ledger entry (§8.2).

**Out, stated plainly (what the row does NOT buy):**
- **No change to reject MESSAGE parity.** Both sides fail closed; the reference's messages carry a run-varying
  `goo.gle/debug…` suffix and are not byte-stable. Parity is measured in rc and in the set of values refused.
- **No 1 ms immediate-request arm** (`BRAINSTORM.md` §0.7: a race on both sides) and **no 61 s timing arm**
  (§7.5).
- **No QUIC enforcement.** QUIC goes from REJECTED to ACCEPTED-AND-IGNORED, which is what the reference does
  (§0.2). No QUIC listener-filter category is added.
- **Not touched:** `stats_flush_interval` (banked, next candidate), the close KIND, `downstream_cx_total`
  post-filter, the `downstream_listener_filter_{remote_close,error}` names, the `0s` shutdown hold (ADR-0322
  (a)), and the 35-60 ms subject lateness at 61 s (`BRAINSTORM.md` §0.6).
- **No stat name** (§8.2).

---

## 2. The validity rule — EXACT (owed item 2)

Input: `*durationpb.Duration d` (the `Listener.listener_filters_timeout` field), read through
`GetSeconds()` (`int64`) and `GetNanos()` (`int32`). **Never `AsDuration()`**: it saturates at the Go
`time.Duration` maximum, so `315576000000s` reads `2562047h47m16.854775807s` (`BRAINSTORM.md` §2.2).

| # | condition (evaluated in this order) | result | reference arm |
|---|---|---|---|
| V1 | `d == nil` | **15000** | absent ⇒ 15 s (phase 100) |
| V2 | `s < 0` **or** `n < 0` | **reject** `listener: %q: listener_filters_timeout: expected a positive duration: %ds %dns` | R12 `-1s`, `-0.5s` (`Expected positive duration`) |
| V3 | `s > 9223372035` | **reject** `listener: %q: listener_filters_timeout: duration out of range: seconds %d` | RB `9223372036s`; R11 `315576000000s` (`Duration out-of-range`) |
| V4 | otherwise | `uint64(s)*1000 + uint64(n)/1e6` — **truncation** | RR: `0.0009s`/`0.000999999s` never fire; `0.0019s` fires at ~2 ms |
| V5 | V4 yields **0** (`0s`, or any `s == 0 && n < 1e6`) | **0 = DISABLED** (`Pipeline.Run` arms nothing) | R5, R6, R14 |

- **Why V2 tests nanos separately:** protojson renders `-0.5s` as `{seconds: 0, nanos: -500000000}`, so
  `s < 0` alone would ACCEPT it (P0 accepted it as `4294966796` ms). The well-formed-Duration invariant
  (same-sign fields, `|n| < 1e9`) is protojson's, and envoy-go runs no PGV (`reference_pgv_forecloses_go_hazard`),
  so V2 must hold even for a Go-constructed mixed-sign Duration. `s > 0, n < 0` is rejected too: it is not a
  valid protobuf Duration.
- **Why the ceiling is `9223372035`:** it is the reference's measured bound (RB bisection). At the maximum
  accepted input the value is 9223372035999 ms, and × `time.Millisecond` = 9.223372035999e18 ns, which is below
  the `int64` ceiling 9.223372036854775807e18. So `Pipeline.Run`'s `time.Duration(timeoutMs)*time.Millisecond`
  cannot overflow. This was measured, not only computed: `Pipe-max` holds, and the A binary holds
  `9223372035.999999999s` open (§3.3). ⚠️ **V3 is therefore LOAD-BEARING for `Pipeline.Run`, not only for
  parity.** Without it, `9223372036s` would give 9223372036000 ms, which overflows to a NEGATIVE duration, and
  `context.WithTimeout` would fire at once. The NC for V3 (§9 NC3) must be read with that in mind.
- **Error wording:** envoy-go keeps its `listener: %q:` prefix (ADR-0082 (a)'s convention, which survives). The
  phrases `expected a positive duration` and `duration out of range` follow the reference's prefixes in
  lowercase. **Unit arms pin the phrase, not the whole string.**
- **Where the reject fires:** on EVERY listener carrying the field, TCP or QUIC, with or without listener
  filters. That matches the reference: its two Duration errors fire on the QUIC listener too (§0.2).

---

## 3. The subject, MEASURED — five shapes, one table (owed items 1 and 3)

### 3.1 The shapes (S-agent, throwaway worktrees off `762c8411`, patched by script, all removed)

| shape | what | `git diff --numstat` | build / vet / lint / gofmt |
|---|---|---|---|
| TIP | unchanged | — | clean |
| P0 | delete the `[1000, 60000]` check only | `manager.go 0 3` | clean |
| **A** | **`uint64` ms end to end, §2's rule** | **`manager.go 8 9`, `pipeline.go 1 1`, `pipeline_deadline_test.go 1 1`** | **clean** |
| B | `time.Duration` end to end, same rule | `manager.go 10 11`, `pipeline.go 3 3`, `pipeline_deadline_test.go 1 1` | clean — **but units wrong** (§0.1) |
| B' | B with its callers repaired | B + `pipeline_test.go 7 7`, `manager_test.go 2 2` | clean |
| C | `uint32`, same rejects, clamp ≥ 2³² ms to `MaxUint32` | `manager.go 10 6` (adds `math`) | clean after `gofmt -w` (import placement) |

### 3.2 The unit matrix — `go test -count=1 -v ./internal/listener/...`, **289 `=== RUN`** in every shape (274 at the tip + 15 probe arms), rc captured with `PIPESTATUS[0]`

One probe file (**Appendix B**, verbatim) was copied unchanged into every shape. A three-line per-shape helper
`p101ToMs` normalised the return type. The FAIL sets below were re-read from the per-shape logs by the
controller (`grep -E '^ *--- FAIL'`), not taken from the agent's summary.

| arm | input | TIP | P0 | **A** | B' | C |
|---|---|---|---|---|---|---|
| U-wrap | `4294968s` ⇒ 4294968000 | FAIL | **FAIL 704** | PASS | PASS | **FAIL 4294967295** |
| U-neg | `-1s` ⇒ reject, `positive` | FAIL (msg) | **FAIL** accepted | PASS | PASS | PASS |
| U-negnanos | `{0,-5e8}` ⇒ reject | *PASS (envelope)* | **FAIL** accepted | PASS | PASS | PASS |
| U-oor | `9223372036s` ⇒ reject, `out of range` | FAIL (msg) | **FAIL** accepted | PASS | PASS | PASS |
| U-pmax | `315576000000s` ⇒ reject | *PASS (envelope)* | **FAIL** accepted | PASS | PASS | PASS |
| U-max | `9223372035.999999999s` ⇒ 9223372035999 | FAIL | **FAIL** | PASS | PASS | **FAIL 4294967295** |
| U-subms | `0.0005s` ⇒ 0 | FAIL | PASS | PASS | PASS | PASS |
| U-1ms9 | `0.0019s` ⇒ 1 | FAIL | PASS | PASS | PASS | PASS |
| U-500ms / U-90s / U-61s | ⇒ 500 / 90000 / 61000 | FAIL | PASS | PASS | PASS | PASS |
| U-nil / U-zero | ⇒ 15000 / 0 | PASS | PASS | PASS | PASS | PASS |
| Pipe-max | `Run` with the parsed max, still blocked at 1 s | FAIL (parse) | PASS (wraps ~24 d) | PASS | PASS | PASS |
| Pipe-wrap | `Run` with the parsed `4294968s`, still blocked at 1 s | FAIL (parse) | **FAIL returned 704.9 ms** | PASS | PASS | PASS |
| existing `…BelowFloorErrors` / `…AboveCapErrors` | `500ms` / `90s` built in Go | PASS | FAIL | FAIL | FAIL | FAIL |
| existing `…InRange` / `…Default` / `…ZeroDisables` | `5s` / nil / `0` | PASS | PASS | PASS | PASS | PASS |

**Discrimination:** P0 vs {A, B', C}: seven arms (U-wrap, U-neg, U-negnanos, U-oor, U-pmax, U-max, Pipe-wrap).
C vs {A, B'}: two VALUE arms (U-wrap, U-max), no behavioural arm. A vs B': **nothing**, which is why B is
decided on mechanism (§0.1). **The existing suite: nothing** (all four lift shapes fail the same two pins).

### 3.3 The real binary (S-agent's Go probe, `0125` template: `tls_inspector`, placeholder STATIC cluster, admin `16851`, listener `16852`; control: TIP at `1s` closed at 1000.5-1000.6 ms, counter 2)

| arm | TIP | P0 | **A** | B | C |
|---|---|---|---|---|---|
| `4294968s`, hold 1500 ms ×3 | rc1 envelope | **closed 705.2-705.4 ms, counter 3** | **open ×3, 0** | open ×3, 0 | open ×3, 0 |
| `0.5s` / false ×3 | rc1 | 500.4-501.0 ms, 3 | **500.3-501.3 ms, 3** | 500.5-501.0 ms, 3 | 500.7-501.1 ms, 3 |
| `0.0005s` ×2 | rc1 | open, 0 | open, 0 | open, 0 | open, 0 |
| `9223372035s` ×2 / `9223372035.999999999s` ×1 | rc1 | open, 0 | open, 0 | open, 0 | open, 0 |
| `-1s`, `-0.5s` validate | rc1 envelope | **rc0** | rc1 `expected a positive duration` | rc1 | rc1 |
| `9223372036s`, `315576000000s` validate | rc1 envelope | **rc0** | rc1 `duration out of range` | rc1 | rc1 |
| QUIC (`0104` subject template), absent / `1s` / `0.5s` / `120s` validate | rc0 / rc0 / **rc1** / **rc1** | rc0 ×4 | **rc0 ×4** | rc0 ×4 | rc0 ×4 |

Reference, for the same arms (`BRAINSTORM.md` §2.1 R1, R5, R10-R12, RB; this stage's R-agent for QUIC):
`4294968s` open, `0.5s` 500.8-502.2 ms, `0.0005s` open, `9223372035[.999999999]s` open, `-1s`/`-0.5s` rc1,
`9223372036s`/`315576000000s` rc1, and QUIC rc0 on every in-range value. **A agrees on every row.** QUIC was
validated only on the subject, not booted or driven. Subject QUIC behaviour after the lift is
accept-and-ignore by mechanism: the one reader, `p.Run(…, rt.lfTimeoutMs)` at `manager.go:1356`, is on the TCP
path.

---

## 4. The production edit — DECIDED: shape A (owed item 1)

**Appendix A is the exact diff** (S-agent's `shape-a.diff`, reproduced from its scratch file). In summary:

1. `parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, error)` — §2's rule.
2. `listenerRuntime.lfTimeoutMs uint64`.
3. `(*Pipeline).Run(…, timeoutMs uint64)`. The body is unchanged: `time.Duration(timeoutMs)*time.Millisecond`
   stays in range by V3.
4. `pipeline_deadline_test.go`'s `runPeekPipeline(server net.Conn, timeoutMs uint64)` — the one typed test
   caller. The seven untyped callers in `pipeline_test.go` compile unchanged **and keep their meaning**
   (milliseconds). That is the property B lacked.

**Comment reconciliation** (not in Appendix A; the IMPL owes it, see §5): `manager.go`'s `listenerRuntime`
field doc (`[1000, 60000] (ADR-0082)`), the build-site comment (`envelope [1000, 60000]`), and the function
doc (`outside [1000, 60000]ms error`). ⚠️ **Layout gate:** no edit may move `pipeline.go:32`, `:33-37` or
`:43`, all cited (`manager.go:479`, `BEHAVIOR_CONTRACT.md:1969`, ADR-0296, ADR-0322). A reproduces
`:33-37` md5-identical and `:43` byte-identical. Re-use `100/PLAN.md` §6's layout-gate form over `pipeline.go`
(the sub-gates for `:33-37` and `:43`).

**Rejected, with the reason:** P0, seven red arms and a 705 ms drop (§3). B, unit reinterpretation of untyped
callers that shows up as a flake (§0.1). C, two red value arms and an unmeasurable parity answer (§0.6). **Diff
size decided none of them.**

---

## 5. Occurrence set of every falsified claim (owed item 8)

Re-derived at `762c8411` by S-agent (two matchers, unioned) and resolved per hit by the controller. **P** means
present-tense and must be reconciled; **H** means historical narration, left alone (method note 63).

| site | text (abridged) | P/H | action |
|---|---|---|---|
| `manager.go:166-167` | `lfTimeoutMs is 0 … or in [1000, 60000] (ADR-0082)` | P | rewrite: any ms value, 0 disabled |
| `manager.go:169` | `lfTimeoutMs uint32` | P (code) | Appendix A |
| `manager.go:831-832` | `envelope [1000, 60000]` | P | rewrite |
| `manager.go:950-953` | doc `outside [1000, 60000]ms error`; `(uint32, error)` | P | Appendix A + doc rewrite |
| `manager.go:963-965` | the check and the envelope message | P (code) | Appendix A |
| `pipeline.go:32` | `timeoutMs uint32` | P (code) | Appendix A |
| `pipeline_deadline_test.go:59` | helper `timeoutMs uint32` | P (code) | Appendix A |
| `manager_test.go:3341-3367`, `:3368-…` | the two envelope pins | P (test) | **re-point** (§6.1) |
| `BEHAVIOR_CONTRACT.md:4338` | `listener_filters_timeout [1s,60s] envelope` (ADR-0082 gloss) | P | rewrite |
| `BEHAVIOR_CONTRACT.md:4359` | `Per-pipeline timeout` bullet: `[1s, 60s] envelope, which is envoy-go's OWN … lifting the envelope is the next row` | P | rewrite (anchor on the LITERAL `Per-pipeline timeout`) |
| ADR-0082 `:3040` heading, `:3044` Doctrine, `:3048`, `:3052` ¶1, `:3066` (a), `:3068` (b), `:3070` (c), `:3074` | the envelope, its message, `timeoutMs uint32` | P, normative | **SUPERSEDED by ADR-0323, never edited** |
| ADR-0081 `:3175` (c) | `well below the … envelope's 1s lower bound` | P, normative | **NOTED by ADR-0323**. The latency claim survives (microseconds is below ANY accepted non-zero value, since the smallest is 1 ms); only its yardstick dies. Never edited |
| ADR-0078 `:3221` | `lfTimeoutMs uint32` (Task-9 field list) | H (a plan-time list) | leave; ADR-0323 names it |
| ADR-0320 `:19355` | the contract bullet `deliberately LEFT STANDING` | H | leave |
| ADR-0322 `:19491`, `:19507` ¶7, `:19552`, `:19567-19568`, `:19642-19644` (g) | `LEAVES … envelope`, `envoy-go still rejects them` | H (an ACCEPTED ADR's record at its time) | leave; ADR-0323 supersedes (g)'s *"envoy-go still rejects"* |
| `REVIEW_FINDINGS.md:185` | names the timeout, not the envelope | — | not a hit |
| closed phase docs (`07.2/*`, `98/PLAN.md`, `100/*`, `101/BRAINSTORM.md`) | — | H | not edited |

**Not hits (re-confirmed):** the CLI help, `internal/stats/name.go`, `tls_inspector.go`, every fixture README.
⚠️ **The IMPL must re-run BOTH matchers at its own tip** and union them (§0.3), case-insensitively, over
`internal/ cmd/ docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md`:
`git grep -niE '60000|\[1s, ?60s\]|1s lower bound|envelope|uint32'` and
`git grep -niE 'listener_filters_timeout|lfTimeoutMs|timeoutMs'`. ⚠️ **Inherit THIS table; use the matchers only
to look for GROWTH** (method note 77).

---

## 6. Unit-test design (owed item 3)

### 6.1 The two envelope pins — INPUTS READ, then re-pointed

| test | line | input | today | after |
|---|---|---|---|---|
| `TestParseListenerFiltersTimeoutBelowFloorErrors` | `manager_test.go:3341` | `durationpb.New(500*time.Millisecond)`, a VALID Duration | `NewManager` must error with the envelope string | **re-point and rename** to an ACCEPT arm: `NewManager` succeeds, `lfTimeoutMs == 500` |
| `TestParseListenerFiltersTimeoutAboveCapErrors` | `:3368` | `durationpb.New(90*time.Second)` | must error | **re-point and rename**: succeeds, `lfTimeoutMs == 90000` |

Suggested names: `…SubSecondAccepted`, `…AboveOldCapAccepted`. The IMPL may choose others; renaming is
required, because a test named `…Errors` that asserts success is a lying name (method note 35).
`TestParseListenerFiltersTimeoutInRange` (`:3299`, `5s`), `…Default` (`:3325`, nil) and `…ZeroDisables`
(`listener_filters_timeout_test.go:123`, `durationpb.New(0)`) are unchanged and stay green under A (§3.2).

### 6.2 New arms — Appendix B's table is the SPEC's floor, installed as real tests

Appendix B's 15 probe arms become permanent tests (house naming, `t.Errorf` per property, each message naming
its property, single-cause). ⚠️ **Drop the `p101ToMs` shim**: under A the parse returns `uint64` directly.
RED/GREEN at the tip is per §3.2: **11 RED at the tip, U-negnanos and U-pmax GREEN at the tip for the wrong
reason (§0.4), U-nil and U-zero GREEN (regression guards).** **Every one is GREEN under A.** Placement:
`internal/listener/listener_filters_timeout_test.go` (row 100's file, which already holds `…ZeroDisables`). The
pipeline arms need `listenerfilter.NewPeekerConn` / `AsPeeker` from package `listener`, as Appendix B does.

### 6.3 A QUIC unit arm — a manager build, RED at the tip

`quic_test.go` builds QUIC listener runtimes. One arm: a QUIC listener carrying `listener_filters_timeout: 0.5s`
(and one carrying `120s`) **builds without error**. That is RED at the tip (the envelope message) and GREEN
under A. This is the subject-side half of §0.2. The reference half is recorded in ADR-0323 §Context; a
differential QUIC arm is NOT chartered (§7.6).

---

## 7. Differential fixture `0126-listener-filters-timeout-envelope` — CHARTERED (owed items 5 and 6)

### 7.1 NEW directory, not a `0125` extension — decided on masking, not size

Extending `0125` would put values the tip REFUSES into `0125`'s bootstrap. At the un-fixed tip the whole
fixture would then fail at subject BOOT, and **every landed `0125` arm (F1, F2, T1-T3, Z1, N1, S) would go
unmeasured in the IMPL's RED capture**. That is method note 51 (an unrelated boot reject masks the site under
test), applied to a sibling's evidence. It would also rewrite a landed fixture's `wantPreCx` pins and README. A
new `0126` keeps `0125` byte-untouched and green at the tip, and its own coarse RED (subject boot reject) is
confined to itself.

### 7.2 Shape

A `MultiListenerDriver` cloned from `0125` (Go-rendered bootstraps, no PKI, no `inputs/`, `BackendCount() 1`
with an unused placeholder cluster). The base listener is `BRAINSTORM.md` §2's: `tls_inspector`, `fc_indexed`
matching `transport_protocol: tls`, and a `default_filter_chain`, each an HCM `direct_response` 200 whose body
names the chain and the listener. **Every listener is a ONE-LINE diff of `listener_filters_timeout`,
`continue…`, or the `listener_filters` block.** Durations are spelled `0.5s`, never `500ms` (protojson).

| listener | delta | reference port |
|---|---|---|
| `l_half` | `0.5s`, `continue…: false` | **15126** |
| `l_half_true` | `0.5s`, `true` | **15232** |
| `l_one_true` | `1s`, `true` (the byte-identical MIRROR of `l_half_true`) | **15233** |
| `l_wrap` | `4294968s`, `false` | **15234** |
| `l_subms` | `0.0005s`, `false` | **15235** |
| `l_nofilt_120` | NO `listener_filters`, `120s`, `false` | **15236** |

### 7.3 Ports — CENSUSED at `762c8411`

`git grep -c '\b<port>\b' -- test/ internal/ cmd/`, piped to `wc -l`, gave **0** for each of `15126`, `15232`,
`15233`, `15234`, `15235`, `15236` (and `15237`). `15126` follows the `15000 + index` convention (`0125` holds
`15125`). `15232-15236` continue the off-convention band `0123`-`0125` hold (`15223`-`15231`). ⚠️ **This
section spells the ports, so it is a hit in the next census. Re-census at the PLAN.**

### 7.4 Arms, pins, and RED at the tip / under P0

| arm | listener | client | pin | tip | P0 |
|---|---|---|---|---|---|
| H1 | `l_half` | silent, hold 2000 ms | closed in **[350, 900] ms**, **0 bytes** | RED (boot) | green |
| W1 | `l_wrap` | silent, hold **1500 ms** | **still open at 1500 ms** | RED (boot) | **RED (705 ms)** — the P0 killer |
| M1 | `l_subms` | silent, hold 1500 ms | still open at 1500 ms | RED (boot) | green (`0.0005s` truncates to 0 under P0 too) |
| T1 | `l_half_true` | silent 800 ms, then GET | body `DEFAULT l_half_true` | RED (boot) | green |
| T2 | `l_one_true` | silent 800 ms, then GET | body `DEFAULT l_one_true` | RED (boot) | green |
| N1 | `l_nofilt_120` | GET at 0 | 200, body `DEFAULT l_nofilt_120` | RED (boot) | green |
| S | all six | — | `downstream_pre_cx_timeout` **by VALUE** per side's own address label on `/stats/prometheus`: `l_half` **1**, `l_half_true` **1**, `l_one_true` **0**, `l_wrap` **0**, `l_subms` **0**, `l_nofilt_120` **0**. A MISSING series is a hard failure, never read as 0 | RED (boot) | **RED** (`l_wrap` 1) |

- **Why the window is `[350, 900]`:** the reference's `0.5s` drop is 500.8-502.2 ms (σ 0.3, n=60); A's is
  500.3-501.3 ms (n=3) and P1's 500.6-500.9 ms. The window excludes both an immediate close and a mis-parse to
  the 1 s mirror or the 15 s default. It is hundreds of σ wide on the measured spreads
  (`reference_differential_band_sigma_margin`). ⚠️ **The reference's `0.5s` spread through the harness's host
  `-p` was not measured at this stage.** `0125`'s 1 s drop widened to 1030 ms once (σ 13.72). The PLAN measures
  it; if needed it WIDENS the window with the measured spread and never drops the arm.
- **Why T1/T2 pin the COUNTER, not the body:** under `true` the body cannot tell "fell through at 0.5 s" from
  "the GET arrived first" (`BRAINSTORM.md` §0.11, R2 vs R2-neg). The body pin only guards chain selection;
  the discriminator is S's `1` beside the mirror's `0` (method note 55). ⚠️ **The body `DEFAULT` rests on the
  reference's R2 measurement with the `tls`-matching `fc_indexed`.** The subject's `0.5s`/`true` fall-through
  body was NOT driven at this stage (A drove only `false`). The PLAN drives it before writing the expectation
  (method note 7j).
- **Why W1 holds 1500 ms:** P0's wrap fires at 704-705 ms. 1500 ms outlasts it by more than 2x, and stays far
  below any real deadline (49.7 days).
- **Wall time:** the arms run concurrently per side. The longest hold is 2000 ms, so `0126` adds ~2-3 s per
  side.

### 7.5 The 61 s arm — NOT a differential arm (owed item 6)

A 61 s arm costs ~61 s of wall clock per side (~122 s per suite run, about 25 % of the ~480 s suite). The only
defect class it could catch is a clamp or cap near 60 s. **That is caught at the parse by U-61s (61000, not
60000), and the enforcement mechanism is value-independent**: one `context.WithTimeout` from the same
`uint64`, exercised at 0.5 s by H1 and at 1 s by `0125`. The reference's 61 s enforcement (61003.8 ms) and the
subject's 35-60 ms lateness are recorded in `BRAINSTORM.md` §0.6 and carried into ADR-0323 §Context. **Decided:
unit arm only.**

### 7.6 QUIC — NO differential arm

(1) The harness cannot carry a mixed TCP+UDP listener pair (banked, `97/PLAN.md` §3). (2) On both sides the
field does nothing on a filterless QUIC listener, so a served-with-counter-0 arm would agree for a reason the
mechanism cannot vary (method note 58). A pin there is vacuous. The subject half is §6.3's unit arm.

### 7.7 What `0126` must NOT pin

The 1 ms immediate-request outcome. Any reject message. The body alone on a `true` arm. A close KIND.
`downstream_cx_total` on a drop listener (the reference counts post-filter). An exact millisecond. Any
`downstream_listener_filter_*` name.

### 7.8 Registration and cost

All four gates (method note 60): `RegisterFixture` in `init()`; the blank import in
`test/differential/runner_test.go`; byte-identity between the registered string and the directory name; the
`NNNN-` shape. **Floor by SHAPE:** `0125` is this exact shape (plaintext, PKI-free, six vs five listeners,
silent-client and timed-close probes) and landed at **+836** (`driver.go` 727, `README.md` 92,
`expectations.yaml` 30 at this tip, by `wc -l`) plus `+1` in `runner_test.go`. `0126` drops `0125`'s 50-way
concurrency arm and adds one listener ⇒ **~+800-900, a FLOOR** (`reference_measured_prototype_is_a_lower_bound`).
The fixture set goes **127 -> 128** (104 `driver/` + 24 `inputs/`).

---

## 8. `ADR-0323` — §Context drafted HERE (owed item 7)

Drafted at the tail of `DECISIONS.md` in the house `> **STATUS: PROPOSED` block form, the phase-100 SPEC's
ADR-0322 precedent: heading, the status blockquote, `### Context`, and a RETAINED italic footer, with
§Decision + §Consequences APPENDED IN PLACE at the IMPL. It **SUPERSEDES** ADR-0082's heading clause
`in [1s, 60s]`, §Decision ¶1's envelope and its message, and §Consequences (a)'s envelope message (the
`listener: %q:` prefix convention SURVIVES), (b)'s `uint32` (it becomes `uint64`, still milliseconds, `0 =
no-op` SURVIVES), and (c) (the "future hardening phase" is this one). It **NOTES** ADR-0081 (c), whose claim
survives without its yardstick, and ADR-0078 `:3221`'s historical field list. It **SUPERSEDES** ADR-0322 (g)'s
*"envoy-go still rejects them"*. This stage re-arms the house `PROPOSED` guard.

### 8.1 The guard, measured after the draft

The house form reads one hit, at the ADR-0323 status line. It is verified by line and by backward heading
search, never by the count (§11 gives the lines).

### 8.2 `BEHAVIOR_CONTRACT.md` ledger — **+0 NAMES**

The lift adds no name. `listener.<addr>.downstream_pre_cx_timeout` exists since row 100, and the reference
registers it at boot on QUIC listeners too (§0.2: `…_16801.downstream_pre_cx_timeout: 0`), as envoy-go does.
The IMPL appends a delta-only `+0, UNCHANGED` ledger entry in the phase-96-through-99 form and quotes **no
absolute**. The `Per-pipeline timeout` bullet is rewritten per §5.

---

## 9. Negative-control roster the PLAN inherits — neutralise, never revert

Each mutant is applied to shape A in a throwaway worktree and marked with an `NC<n>` comment line. Assert the
marker with `grep -c` ≥ 1 before reading a result, and 0 after reversing it (method note 96). **Every row names
the mechanism that carries the mutation to a failure** (method note 7d). "Must redden" is traced through every
arm, including the ones that must NOT redden (method note 76).

| NC | mutation (on A) | mechanism | must redden | must NOT redden |
|---|---|---|---|---|
| NC1 | narrow the return to `uint32(ms)` at the parse's `return` (the width) | 4294968000 and 9223372035999 truncate mod 2³² | U-wrap, U-max, Pipe-wrap, `0126` W1 and S (`l_wrap` 1) | U-neg, U-oor, U-subms, H1, Pipe-max (wraps to ~24 d, §0.5) |
| NC2 | delete V2 (the negative reject) | `uint64(-1)*1000` wraps to a huge value | U-neg, U-negnanos | U-oor, U-pmax, every accept arm |
| NC3 | delete V3 (the ceiling) | `9223372036s` gives 9223372036000 ms; `315576000000s` gives 315576000000000 ms. Both are accepted, so the reject arms go red. (Both would also overflow `time.Duration` inside `Run` and fire at once. No arm drives that; state it and do not add one) | U-oor, U-pmax | U-max, every other arm |
| NC4 | round instead of truncate (`+ 500000` before `/1e6`) | `0.0005s` gives 1, `0.0019s` gives 2 | U-subms, U-1ms9, `0126` M1 (closes ~1-2 ms) and S (`l_subms` ≥ 1) | U-500ms, U-61s, H1 |
| NC5 | compute from `AsDuration()` instead of the fields | saturation happens only above ~292 years, and V3 already rejects that. **NO path to any arm: VACUOUS by construction.** Recorded so nobody runs it as evidence (method note 7d) | — | all |
| NC6 | re-insert the `[1000, 60000]` check | the old envelope | U-500ms, U-90s, U-61s, U-wrap, U-max, U-subms, U-1ms9, both re-pointed pins, §6.3's QUIC arm, all of `0126` | U-neg, U-negnanos, U-oor, U-pmax (rejected for the wrong reason, §0.4), U-nil, U-zero |

**Scored per arm, never per run** (method note 61). U-negnanos and U-pmax are GREEN at the tip; their only
falsifiers are NC2 and NC3.

---

## 10. Hazards the PLAN must carry

1. **A is the chosen shape; do not "simplify" to B.** B compiles into a unit flake (§0.1).
2. **The layout gate over `pipeline.go`** (§4).
3. **The `0126` `true`-arm body is UNMEASURED on the subject** (§7.4). Drive it before writing the expectation.
4. **The reference's `0.5s` spread through `-p` is UNMEASURED** (§7.4). Measure it before fixing the window.
5. **Re-derive the occurrence set at the IMPL tip with BOTH matchers** (§5).
6. **golangci-lint's misspell runs in locale US.** Sweep British spellings from new `.go` comments before the gate.

---

## 11. Counts, re-derived at THIS stage's own tip (the publishing commit)

Each figure comes from running its command in the tree that ships this file.

- `DECISIONS.md` **19644 -> 19662** (`git diff --numstat`: **`18 0`**, against the phase-100 SPEC's `22 0`).
  `^---$` **216** (UNMOVED). `^## ADR-` **321 -> 322**. Bare `^## ` **329 -> 330**. Tail **`## ADR-0323`**.
  `grep -c '^## ADR-0324'` gives **0**, so next-free is **ADR-0324**.
- **House guard `^> \*\*STATUS: PROPOSED`: RE-ARMED.** Its one hit is at `:19648`, which resolves by backward
  heading search to `:19646` `## ADR-0323`. **Decoy** `^\*\*Status:\*\* PROPOSED`: `:14866` resolves to
  `:14864` `## ADR-0231`, md5 of `:14866` `929719b67c87` (first 12 hex, `sed -n p | md5sum`), **BYTE-UNTOUCHED**.
- `ROADMAP.md` **251** / `want` **133** / row 101 `in-progress` at `:163`. **UNMOVED** (a SPEC edits no row).
- `BEHAVIOR_CONTRACT.md` **6000**, UNMOVED (a SPEC does not touch it).
- Fixture set **127 = 127** (103 `driver/` + 24 `inputs/`), UNMOVED. `0126` lands at the IMPL.
- **Gate figures: NOT RUN (a SPEC's scope, not an omission).** They are carried as the phase-100 IMPL's:
  differential 127/0/0, gate (b) 242 packages, h2spec `95 tests, 94 passed, 1 skipped, 0 failed`, fuzzers 56
  targets / 48 files, anchored panic gate 0, `GOTOOLCHAIN=go1.26.2 golangci-lint run ./...` rc 0. The listener
  suite `go test -count=1 -v ./internal/listener/...` read **274** `=== RUN` at `762c8411` (289 with the 15
  probe arms), which is a different selector from the IMPL's 470.
- **File scope: FIVE paths.** `DECISIONS.md`, `STATE.md`, `STATE_HISTORY.md` `2 0`, this `SPEC.md` and
  `next-prompt.txt`. That matches `b6aa3ab3` (phase 100) and `ebc3fc0c` (phase 99). No `.go`, no
  `BEHAVIOR_CONTRACT.md`, no `ROADMAP.md`.

---

## 12. Sentinel — RUN MECHANICALLY AT THIS STAGE's TIP, `/usr/bin/grep`

All commands copied verbatim from `next-prompt.txt`, run at `762c8411` in the canonical root. The SPEC
edits none of the files they read.

(1) **ONE**: `NOT DONE: row 101` at `want=133`. (2) **SIX** at `:211 :217 :223 :233 :239 :247`. (3) **SILENT**.
NC-A: substitution inspected first (`NC LANDED? [ in-progress ]`), then **TWO** (`NOT DONE: row 62`,
`NOT DONE: row 101`). NC-B at `want=132`: **TWO** (`NOT DONE: row 101`, `GATE FAIL: examined 133 data rows,
expected 132`). NC-C: residual **0**, `NEVER OPENED: gRPC   <- NC FIRED`. NC-D: **96 / 68** under `--`.
Check-(2) positive control: residual **0**, **6** substitutions asserted. Escape-aware malformed set exactly
**{57, 69}** at `:119` (NF 9) and `:131` (NF 10). Row 101 NF **8**. Per-line md5 of the six windows,
**trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex): `211 10d7807bf02d` ·
`217 4a92f7e62fc6` · `223 2a7eb298b9fd` · `233 242e53c6f7a3` · `239 b2680e6f4fbf` · `247 6caa1c3ce0e7`.
These are **byte-identical to the phase-101 BRAINSTORM close.** ⇒ **The sentinel does not fire. `stop` was
evaluated and NOT created** (absent at the git root).

---

## 13. Probe hygiene, and what this stage banked

- **Worktrees:** the stage worktree `/home/esa/git/envoy-go-wt/phase-101-spec` (branch `wt-phase-101-spec`,
  off `762c8411`). S-agent's five throwaway detached worktrees were **created and removed**, verified by
  `git worktree list`.
- **Docker:** R-agent only. Containers `p101spec-ref-*` were torn down by name (`docker ps -a --filter
  name=p101spec-ref-` shows the header only). No network was created. A foreign `golink-ai` container was
  present and left alone.
- **Ports:** band `16800-16899`, censused first (`git grep -In '168[0-9][0-9]' -- test/ internal/ cmd/`: two
  hits, both substrings of a CI run id in a `0017` driver comment, no port; `ss -tan`/`ss -uan` empty in the
  band). R-agent used `16800` (admin), `16801/udp` (QUIC), `16802` (validate only) and `16849` (a placeholder
  endpoint, never dialled). S-agent used `16851-16853`. Only TIME-WAIT was left. ⚠️ **This paragraph spells
  band numbers, so it is a hit in the next census.**
- **Scratch:** everything is under the session scratchpad (`ref/`, `subj/`), which **dies with the session**.
  Appendices A and B carry the two artifacts a later stage needs.
- **No production `.go` was written and none of the six gates was run.** That is a SPEC's scope. The one
  standing departure (no `REVIEW.md`, none of 93-100) is not this stage's to fix.
- **Banked (new at this stage):** (a) **the reference refuses `tls_inspector` on a QUIC listener**
  (`Didn't find a registered implementation …`). What envoy-go does with a TCP listener filter on a QUIC
  listener was NOT measured, so a possible fail-open accept is UNMEASURED. (b) **B's hazard class**: any
  future unit change of a Go numeric parameter must census its untyped-constant callers, which compile
  unchanged and get the new meaning silently.

---

## 14. What the PLAN owes

1. **Build and run every code block it embeds** (method note 89): Appendix A applied to the PLAN's own tip; the
   §6 unit arms as final tests with the shim dropped and the two re-points renamed; §6.3's QUIC arm; the
   `0126` driver.
2. **Measure the two unmeasured `0126` cells before writing expectations** (§10 items 3 and 4): the subject's
   `0.5s`/`true` fall-through body, and the reference's `0.5s` spread through the harness's `-p`.
3. **Re-census `0126`'s ports** and **re-derive the occurrence set** with both matchers (§5).
4. **The NC roster (§9)** as appendix patches with `NC<n>` markers. NC5 is recorded VACUOUS and not run.
5. **The layout gate over `pipeline.go`** (§4), reusing `100/PLAN.md` §6.
6. **The doc edits:** ADR-0323 §Decision + §Consequences, the `Per-pipeline timeout` bullet and `:4338`, and
   the `+0` ledger entry.
7. **Record the un-fixed tip before the first production edit**: the 11 RED unit arms named, U-negnanos and
   U-pmax GREEN (structural), and `0126` RED at subject boot.

### 14.1 Coverage of `BRAINSTORM.md` §10

| owed item | discharged at |
|---|---|
| 1 width shape, gated on the P0-killing arms, chosen AND P0 run through the same arms | §0.1, §0.6, §3, §4 |
| 2 the exact validity rule and error wording | §2 |
| 3 re-point the two pins after reading their inputs; RED-at-tip arms for wrap, negative, out-of-range, sub-ms | §6.1, §6.2, §3.2 |
| 4 measure the reference on a QUIC listener | §0.2, §6.3, §7.6 |
| 5 `0125`-extend vs `0126`, ports, windows, P0-killer, counter mirror, nothing from `BRAINSTORM.md` §3.6 | §7 |
| 6 the 61 s arm | §7.5 |
| 7 ADR-0323 §Context `PROPOSED`, supersessions, `+0` NAMES | §8, `DECISIONS.md` tail |
| 8 the occurrence set and what the row does NOT buy | §5, §1 |

---

## Appendix A — shape A, the exact diff (S-agent `shape-a.diff`, built at `762c8411`; comment reconciliation per §5 NOT included)

```diff
diff --git a/internal/listener/listenerfilter/pipeline.go b/internal/listener/listenerfilter/pipeline.go
index d6aed03e..8beb1958 100644
--- a/internal/listener/listenerfilter/pipeline.go
+++ b/internal/listener/listenerfilter/pipeline.go
@@ -29,7 +29,7 @@ type Pipeline struct{}
 //   - OnDestroy is called on every filter (in declaration order) after the
 //     loop ends, regardless of how the loop exited (Continue/StopIteration/
 //     error/timeout).
-func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint32) (retErr error) {
+func (p *Pipeline) Run(ctx context.Context, filters []ListenerFilter, peeker Peeker, inputs *ChainMatchInputs, timeoutMs uint64) (retErr error) {
 	defer func() {
 		for _, f := range filters {
 			f.OnDestroy()
diff --git a/internal/listener/listenerfilter/pipeline_deadline_test.go b/internal/listener/listenerfilter/pipeline_deadline_test.go
index fdab4564..b234700f 100644
--- a/internal/listener/listenerfilter/pipeline_deadline_test.go
+++ b/internal/listener/listenerfilter/pipeline_deadline_test.go
@@ -56,7 +56,7 @@ type runResult struct {
 
 // runPeekPipeline starts Pipeline.Run over a real peekerConn wrapping server
 // with one peekOnlyFilter{5}; the result arrives on the returned channel.
-func runPeekPipeline(server net.Conn, timeoutMs uint32) (net.Conn, <-chan runResult) {
+func runPeekPipeline(server net.Conn, timeoutMs uint64) (net.Conn, <-chan runResult) {
 	pc := NewPeekerConn(server)
 	ch := make(chan runResult, 1)
 	go func() {
diff --git a/internal/listener/manager.go b/internal/listener/manager.go
index d969313c..4e3bc708 100644
--- a/internal/listener/manager.go
+++ b/internal/listener/manager.go
@@ -166,7 +166,7 @@ type listenerRuntime struct {
 	// per-conn pipeline. lfTimeoutMs is 0 (an explicit 0s: disabled, ADR-0322)
 	// or in [1000, 60000] (ADR-0082); default (nil) 15000.
 	listenerFilterFactories []listenerfilter.FilterInstanceFactory
-	lfTimeoutMs             uint32
+	lfTimeoutMs             uint64
 	continueOnLfTimeout     bool
 	// lfPeekBufSize is the LARGEST listenerfilter.InitialReadBufferSizer hint
 	// across the listener's listener_filters[] (tls_inspector's parsed
@@ -950,20 +950,19 @@ func buildNetworkChainFactory(prefix string, filters []*listenerv3.Filter, netRe
 // parseListenerFiltersTimeout parses Listener.listener_filters_timeout per
 // ADR-0082/ADR-0322: nil defaults to 15000ms; an explicit zero is 0 (disabled,
 // as on the reference); other values outside [1000, 60000]ms error.
-func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint32, error) {
+func parseListenerFiltersTimeout(name string, d *durationpb.Duration) (uint64, error) {
 	const defaultMs = 15000
 	if d == nil {
 		return defaultMs, nil
 	}
-	if d.GetSeconds() == 0 && d.GetNanos() == 0 {
-		return 0, nil
+	s, n := d.GetSeconds(), d.GetNanos()
+	if s < 0 || n < 0 {
+		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: expected a positive duration: %ds %dns", name, s, n)
 	}
-	total := d.AsDuration()
-	ms := total / time.Millisecond
-	if ms < 1000 || ms > 60000 {
-		return 0, fmt.Errorf("listener: %q: listener_filters_timeout %s is outside the supported [1s, 60s] envelope", name, total)
+	if s > 9223372035 {
+		return 0, fmt.Errorf("listener: %q: listener_filters_timeout: duration out of range: seconds %d", name, s)
 	}
-	return uint32(ms), nil
+	return uint64(s)*1000 + uint64(n)/1e6, nil
 }
 
 // parseChainSpec converts a listenerv3.FilterChainMatch into the listener-filter
```

## Appendix B — the 15-arm probe table, verbatim (S-agent `p101_probe_test.go`, package `listener`; per-shape shim for A: `func p101ToMs(v uint64) uint64 { return v }`)

```go
package listener

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
)

func p101Parse(t *testing.T, s int64, n int32) (uint64, error) {
	t.Helper()
	v, err := parseListenerFiltersTimeout("l_p101", &durationpb.Duration{Seconds: s, Nanos: n})
	return p101ToMs(v), err
}

func p101Accept(t *testing.T, label string, s int64, n int32, wantMs uint64) {
	t.Helper()
	ms, err := p101Parse(t, s, n)
	if err != nil {
		t.Errorf("%s: accept: got error %v, want accepted", label, err)
		return
	}
	if ms != wantMs {
		t.Errorf("%s: value: got %d ms, want %d ms", label, ms, wantMs)
	}
}

func p101Reject(t *testing.T, label string, s int64, n int32, sub string) {
	t.Helper()
	ms, err := p101Parse(t, s, n)
	if err == nil {
		t.Errorf("%s: reject: accepted with %d ms, want an error", label, ms)
		return
	}
	if sub != "" && !strings.Contains(err.Error(), sub) {
		t.Errorf("%s: reject message: %q does not contain %q", label, err.Error(), sub)
	}
}

func TestP101UWrap(t *testing.T)     { p101Accept(t, "4294968s", 4294968, 0, 4294968000) }
func TestP101UNeg(t *testing.T)      { p101Reject(t, "-1s", -1, 0, "positive") }
func TestP101UNegNanos(t *testing.T) { p101Reject(t, "-0.5s", 0, -500000000, "") }
func TestP101UOor(t *testing.T)      { p101Reject(t, "9223372036s", 9223372036, 0, "out of range") }
func TestP101UPmax(t *testing.T)     { p101Reject(t, "315576000000s", 315576000000, 0, "") }
func TestP101UMax(t *testing.T) {
	p101Accept(t, "9223372035.999999999s", 9223372035, 999999999, 9223372035999)
}
func TestP101USubMs(t *testing.T) { p101Accept(t, "0.0005s", 0, 500000, 0) }
func TestP101U1ms9(t *testing.T)  { p101Accept(t, "0.0019s", 0, 1900000, 1) }
func TestP101U500ms(t *testing.T) { p101Accept(t, "500ms", 0, 500000000, 500) }
func TestP101U90s(t *testing.T)   { p101Accept(t, "90s", 90, 0, 90000) }
func TestP101U61s(t *testing.T)   { p101Accept(t, "61s", 61, 0, 61000) }
func TestP101UZero(t *testing.T)  { p101Accept(t, "0s", 0, 0, 0) }
func TestP101UNil(t *testing.T) {
	v, err := parseListenerFiltersTimeout("l_p101", nil)
	if err != nil {
		t.Errorf("nil: accept: got error %v", err)
		return
	}
	if got := p101ToMs(v); got != 15000 {
		t.Errorf("nil: value: got %d ms, want 15000", got)
	}
}

type p101PeekFilter struct{}

func (p101PeekFilter) Inspect(_ context.Context, p listenerfilter.Peeker, _ *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	_, _ = p.Peek(5)
	return listenerfilter.Continue, nil
}
func (p101PeekFilter) OnDestroy() {}

func p101Pair(t *testing.T) (peer, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	ch := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		ch <- c
	}()
	peer, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return peer, <-ch
}

// p101PipeHold parses (s, n) with the shape's parser, runs Pipeline.Run with
// the PARSED value against a silent loopback peer, and requires Run to still
// be blocked after hold (i.e. the parsed value did not collapse to a short
// or already-expired deadline).
func p101PipeHold(t *testing.T, label string, s int64, n int32, hold time.Duration) {
	t.Helper()
	v, err := parseListenerFiltersTimeout("l_p101", &durationpb.Duration{Seconds: s, Nanos: n})
	if err != nil {
		t.Errorf("%s: pipeline: parse rejected: %v", label, err)
		return
	}
	peer, server := p101Pair(t)
	pc := listenerfilter.NewPeekerConn(server)
	type res struct {
		err error
		el  time.Duration
	}
	ch := make(chan res, 1)
	start := time.Now()
	go func() {
		var p listenerfilter.Pipeline
		err := p.Run(context.Background(), []listenerfilter.ListenerFilter{p101PeekFilter{}}, listenerfilter.AsPeeker(pc), &listenerfilter.ChainMatchInputs{}, v)
		ch <- res{err, time.Since(start)}
	}()
	returned := false
	select {
	case r := <-ch:
		returned = true
		t.Errorf("%s: pipeline hold: Run returned after %v (err %v), want still blocked at %v", label, r.el, r.err, hold)
	case <-time.After(hold):
	}
	_ = peer.Close()
	_ = pc.Close()
	if !returned {
		<-ch
	}
}

func TestP101PipeMax(t *testing.T) {
	p101PipeHold(t, "9223372035.999999999s", 9223372035, 999999999, 1000*time.Millisecond)
}
func TestP101PipeWrap(t *testing.T) { p101PipeHold(t, "4294968s", 4294968, 0, 1000*time.Millisecond) }
```
