# PLAN 88 — `h2-continuation-frames`

**Date:** 2026-08-14 · **Base master:** `fe023fb5` (from `git rev-parse master`, NOT a SHA quoted in a brief) · **Branch:** `phase-88-plan` · **Lifecycle-state:** 2 -> 3

**Method — SUBAGENT-DRIVEN, a NAMED RETURN from a FIVE-STAGE inline departure** (the 87 BRAINSTORM/SPEC/PLAN and the 88 BRAINSTORM/SPEC all ran inline). Four probe agents on disjoint worktrees, disjoint port bands (47490-47529) and private scratch, each committing NOTHING and each proving its tree clean; the controller ran the sentinel battery, the counts, the reconciliation, and re-derived every load-bearing agent claim by execution. **That re-derivation is not ceremonial — it caught a wrong reference-image cite inside this stage** (§9.4).

⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage** — row 88 stays `in-progress` at `:150`, `want` stays **120**, and **NO sub-phase rows are minted** (§3.4). The binding proof is the EMPTY diff in §8.

---

## 0. THE HEADLINE: THIS PLAN REFUTES ITS OWN SPEC ON Q3, AND THE ROW GOT MORE URGENT

**SPEC §6 concluded the client (upstream) leg is *"silent-loss-only, masked by CONNECTION LIFECYCLE rather than code safety"*, resting on three distinct upstream source ports (48240 / 48248 / 48260) showing a fresh upstream connection per request.**

**MEASURED at this tip: the upstream connection is POOLED AND REUSED — `127.0.0.1:34754` for all three requests — so the masking does not exist, and the damage is cumulative:**

```
[K2-many1]    elapsed=4ms   status=200 nRespHdr=219 nXMany=216   <- 84 of 300 headers LOST
[K2-many2]    ROUNDTRIP-ERR after=8.004s context deadline exceeded  <- HANGS, never answered
[K2-control3] elapsed=2ms   status=502 (bad gateway)
backend log:  read GOAWAY LastStreamID=6 ErrCode=COMPRESSION_ERROR
```

⇒ **On the upstream leg the defect is not a silent drop. It is a HANG, then a 502, then envoy-go tearing down a POOLED upstream connection that unrelated concurrent traffic is riding.** The SPEC recorded its result honestly as a bounded measurement rather than "the client leg is safe" — that caution was right, and the bound has now been re-measured in the opposite direction. **88.2 is not the optional leg; it is the one with the worst user-visible failure.**

---

## 1. RED ANCHORS RE-PROVEN BY EXECUTION AT THIS PLAN TIP — AND FOUR SPEC ARM PARAMETERS REFUTED

### 1.1 ⚠️ THE CONTROL VARIABLE IS THE HPACK-**ENCODED** BLOCK SIZE, NOT THE RAW BYTE COUNT

The SPEC's arm sizes were chosen against raw header bytes. Huffman coding shrinks values 20-40%, so **three prescribed arms never split at all and were therefore vacuous**:

| Prescribed arm | SPEC predicted | MEASURED on the wire | Verdict |
|---|---|---|---|
| **G**, 20000 B pad | 200, headers dropped | `HEADERS len=16042` — **one frame**, all 4 headers arrive | **REFUTED — vacuous** |
| **G**, 40000 B pad | 200, headers dropped | `HEADERS 16384` + `CONTINUATION 15638`, `NHDR 4 -> 1` | CONFIRMED |
| **I**, `?big=20000` | response headers dropped | high-entropy ⇒ one frame, `X-Big=<len=20000>` intact | **REFUTED at that size** |
| **K**, `?many=200` | block splits | backend wrote `HEADERS len=15175` — **no CONTINUATION**; 200/200 arrive twice | **REFUTED — cannot answer its own question** |

⚠️ **AND ENTROPY SILENTLY MOVES THE SPLIT POINT.** A *compressible* `'B'x20000` pad **did** split at 20000 B; a high-entropy pad at the same size did not. **Every arm in this row must pin the split by asserting the WIRE FRAMES (or by using a parameter measured to split at the entropy it actually uses) — never by asserting a byte count.** This is the `topic_probe_discipline` invented-inputs shape and it cost two arms this stage.

### 1.2 The arms that DO discriminate — tip vs read-fix prototype

| Arm | TIP | PROTOTYPE (read fix) |
|---|---|---|
| **A** control, un-split `GET /marked` | 200 `MARKED` | 200 — **green both sides, so nothing below is vacuous** |
| **B** `:path` in the CONTINUATION | `RST_STREAM PROTOCOL_ERROR(1)` | **200, `PATH /marked`** |
| **C0** control, un-split `x-probe` | `NHDR 1` header present | `NHDR 1` |
| **C** `x-probe` in the CONTINUATION | **`NHDR 0` — SILENTLY GONE, status 200** | **`NHDR 1` present** |
| **D** split, then an ordinary request | req2 `GOAWAY LastStreamID=1 COMPRESSION_ERROR(9)` then EOF | **both 200, `NHDR 3` each** |
| **G2** stock x/net Transport, 40000 B | `NHDR 4 -> 1` | **`NHDR 4`, `X-Pad` intact** |
| **I** padded upstream response, 40000 B | `nRespHdr 6 -> 3`, `X-Big` GONE | see §3.2 — **gets WORSE first** |
| **K2** `?many=300` x2 + control | 216/300, then **HANG**, then 502 + pooled-conn GOAWAY | **300/300, 300/300, control 200** |
| **F** CONTINUATION flood, 19.6 MB | **absorbed silently, request still proxied** | `GOAWAY ENHANCE_YOUR_CALM(11)` |

Bypass controls proved the backend really emitted the fields (`Ibypass-big40000: X-Big=<len=40000>`; `K2bypass: nXMany=300`), so every "GONE" above is envoy-go's loss, not the backend's omission.

### 1.3 ⚠️ THE LOSS IS PARTIAL, NOT TOTAL — AND THE SPEC'S FIXTURE ASSERTION WOULD HAVE BEEN A DECOY

Two independent seams agree, against SPEC §5's *"tip drops BOTH headers at 20 000 B"*:

- h2c seam: `NHDR 4 -> 1` — one field **survived**; `?many=300` — **216 of 300 survived**.
- `0004` TLS-h2 seam at 32000 B: sentinel `probe=probe-value` **ARRIVED**, `padlen=0` — in **both** directions.

**Mechanism: fields encoded BEFORE the split point survive; fields at or after it are lost.** ⇒ **An arm that asserts a small sentinel header's PRESENCE reads GREEN on this defect.** The load-bearing assertion is the **LENGTH of the large field**. ⚠️ **And do NOT assert *which* headers survive** — that depends on x/net's encoder field ordering and is not a contract.

### 1.4 Findings this stage adds that no prior stage had

1. **The upstream connection is pooled** (§0) — refutes SPEC §6.
2. **Three SPEC arm parameters are vacuous** (§1.1) and entropy moves the split point.
3. **The loss is partial** (§1.3), so the SPEC's intended assertion was a decoy.
4. **The write leg is broken in BOTH directions, measured on the upstream wire** — with `GODEBUG=http2debug=2` the backend read `HEADERS len=35023` and `len=32022` from envoy-go against a peer that advertised `SETTINGS_MAX_FRAME_SIZE=16384`. It only "works" because Go's server Framer reads up to 1 MiB regardless of what it advertised.
5. **The 16 MiB bound is REACHABLE** (§4.1) — the SPEC ordered it labelled dead code.
6. **Arm E, tip only:** after emitting the GOAWAY the tip **still proxied the truncated request and returned 200** on stream 1. The prototype does not. A behavioural improvement the SPEC did not claim.
7. **No existing test in `internal/filter/hcm/...` reddens on the read fix** — i.e. **no existing test covers CONTINUATION at all**. There is no regression net here; the row must build its own.

---

## 2. D-88-SEQ, RE-REVISED: THE THIRD LEG IS TWO-SIDED AND ONLY ONE SIDE IS INDEPENDENTLY ANCHORABLE

### 2.1 The correction

The SPEC treats 88.3 as one write gap. **It is two-sided:**

| Sub-leg | Independent RED anchor at the unmodified tip? | Evidence |
|---|---|---|
| **88.3s** server / response write | **YES** | §2.2 — no read path involved |
| **88.3c** client / request write | **NO** | §2.3 — needs the read fix to become observable |

### 2.2 88.3s IS independently red-anchored — measured at the tip

Listener `codec_type: HTTP2` (`--allow-h2c`), route = `direct_response` (**no upstream at all**), `envoy.filters.http.header_mutation` with 40 `response_mutations` of 1024 high-entropy chars each. Downstream peer advertises the spec-floor 16384 and parses 9-byte frame headers directly off the socket:

```
ADVERTISED SETTINGS_MAX_FRAME_SIZE=16384
RAW-FRAME: HEADERS len=29408 stream=1 flags=0x04 END_HEADERS
  *** EXCEEDS ADVERTISED MAX_FRAME_SIZE 16384 BY 13024 BYTES — ILLEGAL FRAME ***
```
Conforming-peer arm (x/net Framer at the advertised limit): `http2: frame too large`.
Non-vacuous negative control (8 headers, 5915-byte frame): both arms clean.
⚠️ HPACK compression was handled, not assumed: 40x1035 ≈ 41400 plaintext compressed to **29408 measured on-wire**, so the bound was genuinely exceeded.

### 2.3 88.3c is NOT independently anchorable, and the reason is a pre-existing framework property

- The `header_mutation` mirror does **not** work on the request side: `h2dispatch.go:457` calls `rf.SetH2Request(h2req)` **before** the decode chain runs, so decode-side filter mutations never reach the H2 upstream request. Measured: 40 `request_mutations` configured, backend saw `HEADERS len=22`. **This is a pre-existing framework property, NOT a phase-88 defect — do not "fix" it here; file it (§10).**
- The only other route is a large block from downstream, which the tip rejects on read: `GOAWAY lastStreamID=0 errCode=6 (FRAME_SIZE_ERROR)`.

⇒ **Minimum unmasking prerequisite for 88.3c is the read fix.** 88.3s needs none.

### 2.4 ⚠️ THE ORDERING RULING: ONE ATOMIC IMPL COMMIT

The SPEC's constraint ("88.3 with or before 88.2") is **confirmed and strengthened**. Measured on the prototype:

```
PROTO I-big40000, client MaxReadFrameSize=16384  : ROUNDTRIP-ERR http2: frame too large
PROTO I-big40000, client MaxReadFrameSize=1048576: 200 nRespHdr=6, X-Big=<len=40000>  <- FULL FIDELITY
PROTO K2 many1/many2, MaxReadFrameSize=16384     : both http2: frame too large
```
The read fix shipped alone converts a silent 200 into a hard failure **for exactly the traffic the fix exists to repair**.

**RULING — D-88-SEQ (FROZEN): all three legs land in ONE ATOMIC IMPL COMMIT, with TDD held INSIDE it** (the row-87 precedent: every RED census observed with failure lines read BEFORE the production edit, so no census can become an unfalsifiable green-on-arrival claim). Rationale: no intermediate tree state is ever a regression, and the row never has to defend a half-landed codec. **NO sub-phase rows are minted** (row-84 precedent) ⇒ `want` stays **120** and the IMPL's only `ROADMAP.md` edits are the row-88 status cell and the `:204` narrowing (§8.2).

---

## 3. THE ORDERED IMPL TASKS

**T0 — RED census, unit layer.** Land the `writeHeaderBlock` roster (§5.1) and the accumulator arms (§5.2) against the UNFIXED tree. Observe and RECORD the failure lines. Expected RED: all 11 `writeHeaderBlock` rows (the method does not exist), the accumulator arms, and the arm-E/flood pins.
**T1 — RED census, differential layer.** Land the `0004` extension (§6). Confirm `TestDifferential/0004-h2-routing` RED on the **request** arm with the verbatim line, then re-run with the request arm demoted to isolate the **response** arm RED. ⚠️ **The driver is FAIL-FAST: a multi-arm RED census needs ONE RUN PER ARM** (`reference_failfast_driver_masks_later_red_arms`; ADR-0309 §Consequences (vii)) — the second arm is otherwise an unproven claim riding on the first arm's failure, and it reads as proven because the run did fail.
**T2 — production, read legs (88.1 + 88.2).** `conn.go` + `client.go` accumulator.
**T3 — production, write leg (88.3s + 88.3c).** `framer.go` `writeHeaderBlock` + both call sites.
**T4 — GREEN both layers.** Unit 11/11 + accumulator arms; `0004` green both sides against the pinned reference.
**T5 — docs.** BEHAVIOR_CONTRACT bullet (§7.1), ADR-0310 completion (§7.2), the false-comment kill (§7.3), the `0004` doc counts 29 -> 31.
**T6 — gates LAST, against the FINAL tree** (§9).

T0 and T1 are **both prerequisites of T2/T3**.

---

## 4. THE TWO OPEN DECISIONS — FROZEN

### 4.1 ⚠️ D-88-BOUND: the SPEC's "defence in depth" instruction is REFUTED — the bound is REAL coverage

The SPEC ordered the three accumulator guard arms labelled *"defence in depth, not counted as coverage"*. **Measured, two ways, independently:**

| Guard arm | SPEC claim | MEASURED |
|---|---|---|
| nil accumulator | unreachable | **UNREACHABLE — CONFIRMED** (`unexpected CONTINUATION for stream 1`) |
| wrong stream | unreachable | **UNREACHABLE — CONFIRMED** (`got CONTINUATION for stream 3; expected stream 1`) |
| **over-bound 16 MiB** | unreachable | **🔴 REACHABLE — REFUTED** |

At the framer level: HEADERS(16384, !END_HEADERS) + 1024 x CONTINUATION(16384) ⇒ `frames=1025 accumulated=16793600 bound=16777216 firstErr=<nil>`. `checkFrameOrder` caps the **hop**, not the **total**, and imposes no CONTINUATION count limit. End to end: the tip **absorbed a 19.6 MB flood with no error and still proxied the request**.

⚠️ **Nothing upstream of this bound binds.** x/net's `maxHeaderListSize` lives inside `readMetaFrame`, which envoy-go never enables — so **envoy-go does not inherit x/net's CVE-2023-45288 CONTINUATION-flood mitigation**. `SETTINGS_MAX_HEADER_LIST_SIZE` is never advertised (zero hits in `internal/filter/hcm/`). **After this row lands, `maxHeaderBlockSize` is the ONLY cap on a CONTINUATION flood.**

⚠️ **And note the direction of the exposure: the TIP is not vulnerable, because discarding CONTINUATION frames retains nothing. This row's accumulator is what INTRODUCES retention.** The bound is the mitigation for a hazard the fix itself creates. It gets a real break arm, and arms A and B — and only those two — get the defence-in-depth label.

**The nil-accumulator and wrong-stream arms are unreachable through `ReadFrame` only.** Isolation was proven, not assumed: with `AllowIllegalReads = true` the **byte-identical** inputs are both delivered as `*http2.ContinuationFrame` with no error, so `checkFrameOrder` is the sole gate. envoy-go never sets that field; production frame producers are exactly three, all `ReadFrame` (`settings.go:121`, `framer.go:84`, `framer.go:116`). ⚠️ **The IMPL must re-check this if it adds a frame source.**

### 4.2 D-88-CODE: the error code — BOTH candidates REFUTED by the reference

**MEASURED 5x against `envoyproxy/envoy:contrib-v1.37.2`:**
```
RST_STREAM ErrCode=2 name=INTERNAL_ERROR StreamID=1   — stream-scoped, and NO GOAWAY is ever emitted
server-side detail: http2.too_many_headers ; stat http2.header_overflow ; max_request_headers_kb default 60 KiB
```
Not `FRAME_SIZE_ERROR` (the prototype's choice), not `ENHANCE_YOUR_CALM`, not `COMPRESSION_ERROR`, not `PROTOCOL_ERROR`. On a 2 MiB flood the reference tears down by **delayed close, not GOAWAY**. Negative control (HEADERS + 3 CONTINUATIONs under the limit, one continuous encoder) returned a normal 200, so the flood reading is the size policy and not a malformed-client artifact.

**⇒ The real divergence is SCOPE, which nobody had framed.**

**RULING — connection-scoped `ErrEnhanceYourCalm` (0xb) at 16 MiB.** Reasoning, on the record:
- **Stream-scoped is not free under the decided primitive.** D-88-PRIM accumulates raw bytes and decodes once at `END_HEADERS`. Resetting the stream and dropping the buffer leaves the HPACK dynamic table permanently desynced from the peer's encoder — **the exact defect this row exists to fix**. The reference can reset the stream because it decodes incrementally.
- ⚠️ **It is a cost tradeoff, not an impossibility, and this PLAN will not overstate it.** A "decode-and-discard drain" path (keep feeding fragments to the decoder to stay in sync, discard the fields, then RST_STREAM) would make stream-scoped parity achievable. That path is **NOT** in this row's scope; it is filed in §10 with its cost un-enumerated.
- Given the scope divergence is taken deliberately, matching the reference's *code* alone buys nothing. `ENHANCE_YOUR_CALM` is the RFC-idiomatic connection-level flood code. **Two probe agents independently reached it.**
- **Both divergences — scope AND value (16 MiB vs 60 KiB) — are NAMED in the contract (§7.1), not hidden.**

### 4.3 D-88-STAT: **no new stat.** Stat-surface delta **0**, matching the SPEC's default and the prototype (`NewCounter(|NewGauge(` reads **406** in both trees). The reference's `http2.header_overflow` is noted as a parity gap and filed in §10 rather than minted here — a new stat name would break the +0 and is not needed to prove the row.

### 4.4 D-88-LASTID: **PIN IT.**

| | error code | direction | `LastStreamID` |
|---|---|---|---|
| **TIP** | `PROTOCOL_ERROR(1)` | envoy-go -> downstream | **1** |
| **PROTOTYPE** | `PROTOCOL_ERROR(1)` | envoy-go -> downstream | **0** |

SPEC prediction **CONFIRMED by execution**. Origin confirmed as x/net, not envoy-go: the log reads `framer: connection-error code=1` on **both** sides, and that string is emitted only by `translateFramerErr`'s `http2.ConnectionError` branch (`framer.go:56`) — i.e. `Framer.checkFrameOrder`. envoy-go never sees the frame.

**Pin it**, because it is wire-visible and because the same buffering carries the §1.4(6) improvement: the tip proxies a truncated request and returns 200 after emitting the GOAWAY; the prototype does not. **Pin BOTH properties in one test** — `LastStreamID == 0` and "no response frames follow the GOAWAY" — with `t.Errorf` per property (`reference_fatalf_makes_assertions_unreachable`).

---

## 5. THE UNIT-TEST ROSTER

### 5.1 `writeHeaderBlock` — **11 rows, not 4. MEASURED at 130 lines.**

The SPEC proposed four rows. A break arm that **hardcodes 16384 and ignores the peer's advertised value reddens NONE of them.** Rows, with the four SPEC rows marked `[S]`:

| Row | Why it is not redundant |
|---|---|
| `under_max` `[S]` | baseline |
| `exactly_max` `[S]` | off-by-one at the boundary |
| `one_over_max` `[S]` | must produce HEADERS + CONTINUATION, `END_HEADERS` last only |
| `zero_max_defaults_16384` `[S]` | see §5.3 |
| `empty_block` | one HEADERS with an empty fragment + `END_HEADERS`, not zero frames |
| `exact_multiple_2x` | **no trailing empty CONTINUATION** — nothing else detects one |
| `exact_multiple_3x` | same defect at N>2 |
| `three_continuations` | catches a non-looping "at most one CONTINUATION" implementation |
| **`peer_max_larger`** | **highest value — catches a hardcoded 16384** |
| `peer_max_max_legal` | same discriminator at the RFC ceiling |
| `end_stream_with_continuation` | END_STREAM rides the HEADERS only; RFC 9113 §6.10 gives CONTINUATION only END_HEADERS |

**Two per-row assertions the count-only framing misses:** (i) byte-exact reassembly, `concat(fragments) == input`, with a **position-dependent fill so a mis-slice cannot alias a correct slice** — a frame-*count* assertion is compensating-defect-blind (`reference_compensating_defects_cancel_in_the_gate_metric`); (ii) every emitted frame's StreamID equals the HEADERS StreamID.

**Break protocol, RUN on the roster (each reverted, green restored):**
1. hardcode 16384 ⇒ reddens **`peer_max_larger` + `peer_max_max_legal` ONLY**; every SPEC row stays GREEN.
2. always terminate with an empty CONTINUATION ⇒ reddens 7 rows including both `exact_multiple_*`.
3. leak END_STREAM onto the last CONTINUATION ⇒ **isolating**: `end_stream_with_continuation` alone, `frame 1 CONTINUATION carries END_STREAM (0x1); flags=5`.

### 5.2 Accumulator arms

- **over-bound — REAL coverage** (§4.1). Break arm: raise the bound, assert the flood is no longer rejected. End-to-end pin from arm F: `GOAWAY ENHANCE_YOUR_CALM(11)` + log `h2 continuation accumulator exceeded maxHeaderBlockSize`.
- **nil accumulator / wrong stream — DEFENCE IN DEPTH, explicitly labelled, NOT counted as coverage.** Both unreachable behind `checkFrameOrder`; a test asserting them must say so in a comment naming the x/net error it can never get past.
- **arm-E pin** (§4.4), **arm-B/C/D reassembly pins** at the `ServerConn` level, and the client-side mirror.

### 5.3 The zero-`MaxFrameSize` case is REACHABLE, and the policy is ALREADY LANDED

Symbols: `ServerConn.clientS` (`conn.go:55`), `ClientConn.serverS` (`client.go:75`), both `clientSettings`. Neither is seeded; `readClientSettings` (`settings.go:156`) and the mid-connection paths (`conn.go:578`, `client.go:395`) assign `MaxFrameSize` **only if the peer's SETTINGS frame carries that parameter**, which RFC 9113 §6.5 does not require. Measured `0` after an empty SETTINGS, `32768` after a populated control.

**Not hypothetical:** `driveHandshake` (`settings_validate_test.go:38`) sends `fr.WriteSettings()` with **no arguments**, so that whole suite already runs at `clientS.MaxFrameSize == 0`.

⚠️ **DO NOT MINT A POLICY — REUSE THE SIBLING CONSTANT** (`reference_grep_for_sibling_derived_constant`). The identical guard is already landed **twice**:
```go
// Peer SETTINGS_MAX_FRAME_SIZE cap. RFC 9113 §6.5.2 default is 16384 if
// the peer has not yet sent SETTINGS (s.clientS.MaxFrameSize == 0).
maxFrame := int32(s.clientS.MaxFrameSize)
if maxFrame <= 0 { maxFrame = 16384 }
```
at `conn.go:778-783` (`ServerConn.writeData`) and `client.go:854-858` (`ClientConn.writeData`). The guard is load-bearing: with it deliberately removed, the codec emits **unbounded zero-length HEADERS/CONTINUATION frames**.

### 5.4 House style — reuse these, do not reinvent

Table idiom: anonymous `[]struct{...}` named `tests` + `t.Run(tc.name, ...)`, rows banded by `// --- positive controls (must PASS) ---`; canonical example `TestValidateResponseTrailers_Table` (`trailers_validate_test.go:39`).
**Errors are NEVER asserted by exact-message equality** — assert `got.Code`, `got.Stream` (0 = conn-scoped), `strings.Contains(got.Error(), wantMsgSubstr)`, and `errors.Is`, each with `t.Errorf` (`trailers_validate_test.go:215-244`; `errors_test.go` for the `h2: ` prefix discipline).
Server harness: `startServerConn` (`conn_test.go:86`), `driveHandshake` (`settings_validate_test.go:34`), `writeGetHeaders` (`settings_validate_test.go:65`), `assertMidConnGoaway` (`settings_validate_test.go:88`).
Client harness: `dialClientConn` (`client_test.go:395`), `dialClientConnTCP` (`trailers_validate_test.go:291`), peer `fakeH2ServerPeer` (`client_test.go:216`).
⚠️ **No existing helper can write a split header block. Add ONE new peer method alongside the others** — `writeScriptedTrailers` (`trailers_validate_test.go:257`) is the precedent for exactly that, rather than a parallel harness.

---

## 6. THE DIFFERENTIAL RECIPE — D-88-DIFF FROZEN: **EXTEND `0004`. MEASURED AT 115 LINES.**

### 6.1 The decision

**Option 2 is REFUTED, not merely disfavoured.** Only three fixture backends read request headers — `0012-http-header-mutation`, `0015-http-buffer`, `0005-prometheus-stats` — and `grep -ln http2` over all three returns **nothing**. **No fixture backend both reflects headers and speaks H2.**

**Option 1 chosen.** `0004`'s `envoy.yaml` and `envoy-go.yaml` route tables are identical and `prefix: "/api"` already forwards to `c_h2_backend` (`http2_protocol_options`, upstream ALPN `h2`), so new paths under `/api/v1/` need **zero YAML edits on either side** and exercise **both** codec legs. The fixture keeps its single `HTTPSH2` runner branch (`runner_test.go:286`), so the one-dir-one-runner-branch constraint is untouched.

### 6.2 MEASURED cost — re-derived by the controller from the saved patch, not accepted from the report

```
37   0   test/fixtures/0004-h2-routing/backends/main.go
78   0   test/fixtures/0004-h2-routing/driver/driver.go
=> 115 insertions, 0 deletions, 2 files, 0 new files
```
Backend `+37`: `contPadLen`/`contPad` helper, `/api/v1/reflect` (request-direction reporter), `/api/v1/emit` (response-direction large-block emitter) — exact-match `ServeMux` patterns beat the existing `/api/v1/` subtree handler, so no existing behavior moves. Driver `+78`: the `hpack` import, two arm blocks, mirrored constants.
**Plus ~9 MODIFIED doc-count lines** (29 -> 31): `driver.go:242,431,445`, `README.md:3,26`, `expectations.yaml:10,58`. ⚠️ The drive loop really is **29/side** today (9 `/health` + 9 `/api/v1/<n>` + 9 `/missing/<n>` + 2 phase-87 `//edge`); the "27" in `README.md:34` is **correct** as a description of the pre-phase-87 prefix and must **not** be "fixed".

### 6.3 The discrimination proof — both arms RED, both sides executed

```
runner_test.go:1276: subj drive: cont-request-arm: backend saw
  "reflect:probe=probe-value,padlen=0", want "reflect:probe=probe-value,padlen=32000"
  (CONTINUATION-carried request headers lost)
--- FAIL: TestDifferential/0004-h2-routing (3.68s)
```
Response arm isolated by demoting the request arm (fail-fast driver, §3 T1):
```
runner_test.go:1276: subj drive: cont-response-arm: marker="emitted" padlen=0,
  want "emitted"/32000 (CONTINUATION-carried response headers lost)
```
Named subtest **PRINTED** in every run (`-run` no-match footgun excluded). Anchored panic gate `^panic:|DATA RACE|SIGSEGV` ⇒ **0 hits**. The reference passed both new arms in all four runs (the runner drives the reference first and `t.Fatalf`s with a `ref drive:` prefix; no such failure ever appeared).

**The negative controls are what make the RED meaningful:**

| pad | encoded block | subject | `INNER_EXIT` |
|---|---|---|---|
| 1024 B | one frame | PASS | 0 |
| 16000 B | one frame (Huffman ≈12.6 KB) | PASS | 0 |
| **32000 B** | **HEADERS + CONTINUATION** | **FAIL, both arms** | 1 |

**The flip is at the frame-split boundary, not a header-size threshold** — which rules out the competing hypothesis that envoy-go simply caps header size.

### 6.4 ⚠️ Assertion constraints the IMPL must honour

1. **Assert the LENGTH of the large field** (`padlen=32000`), never the small sentinel's presence — the sentinel survives (§1.3).
2. **Never assert WHICH headers survive** — that is x/net encoder field ordering, not a contract.
3. **32000 B is measured to split at this fixture's entropy.** If the IMPL changes the payload, **re-measure the split**; do not assume the byte count carries (§1.1).

### 6.5 Count deltas under option 1 — **+0 on every axis**

fixtures **121 +0** · `BackendKind` tail **38 +0** (39 declarations, `TCPEcho = 0` — a TAIL VALUE, not a count; do NOT "fix") · next-free fixture **`0120` stays unconsumed** · new port **none** (`0004` keeps `refContainerListenerPort = 15004`) · blank imports **121 +0**.
**All three registration gates are already satisfied by `0004`, so the row adds ZERO registration work:** (1) the directory matching `discoverFixtures` (`runner_test.go:1463`); (2) `fixture.RegisterFixture("<dirname>", &driver{})`; (3) ⚠️ **the blank import in `runner_test.go` — the SILENT one**: missing it hits `t.Skipf` at `runner_test.go:202` and the fixture **SKIPS, reading as green**. `expectations.yaml` is **not** a gate (no Go code loads it).
**Port convention, for the record if `0120` is ever revived:** family-banded `10<index>`, **never** max+1 — a `0120` would take **10120**, not `19173`.

---

## 7. DOCS — TEXT CONSTRAINED AT THIS PLAN

### 7.1 `BEHAVIOR_CONTRACT.md` — an ADDED bullet, not a correction

SPEC §8 verified there is **no** sentence to ride: the `## HTTP/2` section (`:2019` -> `## HTTP filter chain` `:2081` at this tip, **re-confirmed by the controller**) makes no claim about CONTINUATION, header-block framing, `END_HEADERS` or max frame size. ⚠️ Every whole-file `CONTINUATION` hit (`:782`, `:798`, `:820`, `:823`, `:825`, `:5109`) is the **tracing** sense and `:5281` is an async-resume "continuation site" — **do not conflate them; do not edit them.**
⚠️ **CITE BY STRING OR SYMBOL** — row 87's `:2036` insertion shifted every by-line citation at or below it by +1.
The bullet must state: reassembly on read both directions; splitting on write both directions bounded by the **peer's** advertised `SETTINGS_MAX_FRAME_SIZE` (default 16384 when unadvertised); and **NAME BOTH DIVERGENCES from the reference — scope (connection vs the reference's measured stream-scoped `RST_STREAM INTERNAL_ERROR`) and value (16 MiB vs the reference's 60 KiB `max_request_headers_kb`).**

### 7.2 `DECISIONS.md` — ADR-0310 completion

Append §Decision + §Consequences **IN PLACE after the RETAINED italic footer** (`*§Decision and §Consequences follow at the phase-88 IMPL.*`), the ADR-0294-0309 shape: **no renumber, and NO `---` separator** (`^---$` stays **216**; a `---` was added at the SPEC, measured at 217, and removed). §Context ¶1-¶4 already exist. **Strict guard `^> \*\*STATUS: PROPOSED` 1 -> 0** — that is the IMPL's job, not this stage's. ⚠️ This is the ONE non-append edit `DECISIONS.md` takes, so the proof is `numstat N 1` and **the `cmp` prefix will NOT hold**; verify instead that the only deleted line is the STATUS line.
§Consequences must record, at minimum: the Q3 refutation (§0); the reachable bound and the **un-inherited CVE-2023-45288 mitigation** (§4.1); the reference error-code refutation and the scope divergence (§4.2); the encoded-vs-raw arm-parameter lesson (§1.1); and h2spec's blindness (§9.3).

### 7.3 The one landed FALSE artefact this row kills

```go
// Handled by framer as part of ReadMetaHeaders; reaching here means the
// framer gave us a raw ContinuationFrame (shouldn't happen in normal usage,
// but be safe and ignore).
```
`conn.go:264-268`. Both clauses are false — `ReadMetaHeaders` is assigned only in test-side peers (`test/differential/runner_test.go:3350`, `test/fixtures/0119-grpc-unary-trailers/driver/driver.go:479`), never in `internal/`, **and SPEC §1.1 showed that assigning it would HANG the server**. It dies at the IMPL.

---

## 8. SENTINEL — RUN AT THIS TIP, ONE SIDE, RECORDED NOT PREDICTED

### 8.1 This stage

Input **238 lines / 120 data rows**. (1) **`NOT DONE: row 88` ALONE**, denominator silent · (2) **SIX** at `:198 :204 :210 :220 :226 :234` · (3) **SILENT** ⇒ the conjunction fails, **the sentinel does NOT fire, `stop` was NOT created** (verified absent at repo root AND stage worktree).
**All four NCs FIRED:** row-62 doctoring (`NC LANDED? [ in-progress ]` inspected FIRST) ⇒ `NOT DONE: row 62` AND `NOT DONE: row 88`; `want=119` ⇒ `GATE FAIL: examined 120 data rows, expected 119`; check-(3) doctoring (residual **2 -> 0** confirmed on the doctored copy first) ⇒ `NEVER OPENED: gRPC` with **WASM correctly silent**; check-(2) one-arm **5** (long) / **1** (short), union **6**.
Leak axes INVARIANT: `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**.
⚠️ **The `-` flag trap fired live at this stage** on `grep -cF '- **prior active-phase...'` (`ugrep: invalid option`). **Always pass `--` before a pattern starting with `-`.**

### 8.2 What the IMPL owes at ROW-DONE

The `:204` gRPC-family window names **11** candidates, one of which is *"the CONTINUATION-discard defect"*. Removing it leaves **10**, so the phrase `remaining deferred (not-yet-chartered) candidates:` survives and check (2) **should** stay SIX. ⚠️ **THAT IS A PREDICTION AND MUST BE MEASURED ON BOTH SIDES AT THE IMPL, NEVER FORECAST.** Row 88's own line does not carry the matcher (measured **0**), so the status flip alone cannot move check (2).

---

## 9. GATES, COUNTS, AND ONE CORRECTED CITE

### 9.1 The IMPL's gates (T6, run LAST against the FINAL tree)
`go build ./...` · `go vet ./...` · `gofmt -l` on touched packages (**gate on OUTPUT — `gofmt -l` never exits non-zero**) · `golangci-lint` (⚠️ misspell locale **US**) · `go test ./... -count=1` · `-race` · the **full** differential suite with **`INNER_EXIT` captured** (the wrapper exits 0 even when the binary aborts mid-suite) · anchored panic gate `^panic:|DATA RACE|SIGSEGV` · `go mod tidy -diff` EMPTY · stat-surface delta by the same command both sides.

### 9.2 Counts re-derived MECHANICALLY at this tip (`fe023fb5`)
`ROADMAP.md` **238 / 120 rows**, row 88 `in-progress` at `:150` · `DECISIONS.md` **18166**, **309** headings, tail **ADR-0310** (§Context only, `PROPOSED`), next-free **ADR-0311** (`grep -c '^## ADR-0311'` ⇒ **0**; TAIL-derived — headings+1 COLLIDES at the ADR-0209 gap), `^---$` **216**, STATUS census **23**, **strict `PROPOSED` guard 1 — ARMED, and this PLAN leaves it armed** · `BEHAVIOR_CONTRACT.md` **5958** · `STATE.md` **64** · `STATE_HISTORY.md` **494** · `BOOTSTRAP_PROMPT.md` **522** · phase dirs **129** · `REVIEW.md` **37** (standing departure, named not claimed) · fixtures **121**, tail `0119`, `0120` free · fuzzers **55 / 48**, anticipated **+0** (the accumulator consumes no new config field; its inputs ride the existing `internal/filter/hcm/h2` fuzz surface) · BackendKind tail **38** · `go.mod` **+0**.

### 9.3 h2spec is NOT a gate for this row
`95 tests, 94 passed, 1 skipped, 0 failed` **identically at tip and prototype**. §6.10 CONTINUATION passes **6/6 at the tip that discards every CONTINUATION frame** — those tests assert the connection's *reaction*, never that the fields arrived. **Cite only from your own run; a green gate is not evidence of a present feature.**

### 9.4 ⚠️ ONE AGENT CITE CORRECTED BY THE CONTROLLER
A probe report cited the reference as `envoyproxy/envoy:v1.33.0`. **Refuted by the code path:** `parseEnvoyTarget` (`harness.go:37`) reads the `**Tag:**` line from `ENVOY_TARGET.md`, which pins **`envoyproxy/envoy:contrib-v1.37.2`**; the `v1.34.0` in `harness_test.go:21/28` is a **synthetic fixture string inside a parser unit test**, not a pin. The report's *results* stand (they came from the real runner); its *image cite* does not. **Carried as a correction, not as a cite** (`feedback_brief_citations_not_evidence`).

### 9.5 ⚠️ THE BINARY-ASSERTION TRAP, RE-DERIVED FIRST-HAND
`strings <tip-binary> | grep -c 'writeHeaderBlock'` ⇒ **9** on the UNMODIFIED tip — Go's bundled `net/http.(*http2writeResHeaders).writeHeaderBlock` and `(*http2writePushPromise)`. The naive assertion reads as *already present*. **The discriminating form is the qualified name:** `hcm/h2\.(\*framer)\.writeHeaderBlock` ⇒ **0 tip / 2 fixed**.

---

## 10. BUDGET — A FLOOR, WITH THE REMAINING UN-ENUMERATED CLASSES NAMED

| Layer | MEASURED | IMPL band |
|---|---|---|
| production, read legs | `client.go` +140/−88, `conn.go` +82/−12 ⇒ **net +122** | |
| production, write leg | `client.go` +5/−3, `conn.go` +6/−3, `framer.go` +62/−0 ⇒ **net +67** | |
| **production total** | **net ~+189** | **~+190-240** |
| differential | **115** (+~9 modified) | 115-160 |
| `writeHeaderBlock` roster | **130** | 130-170 |
| accumulator / arm-E / flood units | not measured | 150-250 |
| **test total** | | **~+400-580** |

⚠️ **Of the read leg's 222 insertions, 94 (42%) are pure code motion and ~95 are comments** — the old `*http2.HeadersFrame` case body lifted into `onResponseHeaderBlock`, **86 lines byte-identical after de-indent** and 8 mechanical parameter substitutions. Genuinely new executable code across both read legs is **~70-80 lines**. Quoting the raw net without this would overstate the work.

⚠️ **QUOTED AS A FLOOR** (`reference_measured_prototype_is_a_lower_bound`). Row 87 broke a ten-firing streak only because it had ONE grep-confirmed call site; **this row has three decode sites, two write sites, and per-connection state on two structs.** Nothing about row 87's escape transfers.

**Un-enumerated classes NAMED, including the three this stage newly discovered:**
1. **`hasPriority`/`priority` on a split HEADERS** — carried on the accumulator, **entirely unprobed**. A HEADERS with both the PRIORITY flag and a CONTINUATION was never sent.
2. **Trailers arriving split** (`HEADERS`+`CONTINUATION` after DATA) — unprobed.
3. **A split block on a stream being concurrently reset** — unprobed.
4. The contract bullet and ADR-0310 §Decision/§Consequences prose.
5. Reviewer-mandated arms (the 87 IMPL's adversarial review added 29 lines and found three real gaps the controller had not).

**Deferred candidates this row DELIBERATELY does not charter** (each with its reason, none silently dropped):
- **stream-scoped header-overflow parity via a decode-and-discard drain** (§4.2) — cost un-enumerated.
- **`max_request_headers_kb` config parity** and the **`http2.header_overflow` stat** (§4.2, §4.3) — would break the stat-surface +0.
- **`SETTINGS_MAX_HEADER_LIST_SIZE` advertisement** — never advertised today (§4.1).
- **decode-side filter mutations never reaching the H2 upstream request** (`h2dispatch.go:457`, §2.3) — a **pre-existing framework property, not a phase-88 defect.**

---

## 11. NEXT

**The phase-88 IMPL.** Row 88 flips `done` at `ROADMAP.md:150` (status cell ALONE — verify the summary prose byte-identical by sha256 over the status-blanked line, the row-86/87 precedent); the `:204` gRPC-family window loses *"the CONTINUATION-discard defect"* (**measure check (2) on BOTH sides**); `want` stays **120**; the strict `PROPOSED` guard goes **1 -> 0** by completing ADR-0310.
