# Phase 88 — `h2-continuation-frames` — SPEC

**Date:** 2026-08-14 · **Base master:** `0158192c` (from `git rev-parse master`, NOT a SHA quoted in a brief) · **Branch:** `phase-88-spec` · **Lifecycle-state:** 1 -> 2

**Method — NAMED INLINE DEPARTURE (a FIFTH consecutive stage; the 87 BRAINSTORM/SPEC/PLAN and the 88 BRAINSTORM precedent).** No investigation agents; every probe run INLINE by the controller. The centrepiece is **TWO COMPILING PROTOTYPES built in DETACHED worktrees** (`wt-88-proto-a`, `wt-88-proto-b`, both deleted at close) so Q1 is decided by measurement rather than argument. Also: `probe88` rebuilt from BRAINSTORM §2 and extended with arms J/K; a `q4probe` package driving the repo's OWN `test/helpers.H2CRoundTrip`; the tip binary; an h2c echo backend; h2spec run at BOTH tip and prototype. Ports **47480/47481** (subject), **47485** (echo backend). ⚠️ **`ROADMAP.md` is BYTE-UNTOUCHED at this stage** (verified by empty diff); row 88 stays `in-progress`, `want` stays **120**.

⚠️ **THIS SPEC REFUTES ITS OWN PREDECESSOR ON THE CENTRAL STRUCTURAL CLAIM.** The BRAINSTORM chartered a **two-leg** row (D-88-SEQ: server read, client read). **There are THREE legs.** §3 establishes the third by execution, and it carries a HARD ORDERING CONSTRAINT that did not exist in the BRAINSTORM's plan.

---

## 1. Q1 — the reassembly primitive: DECIDED BY PROTOTYPE, both options built

### 1.1 D-88-PRIM (DECIDED): the per-connection accumulator. `Framer.ReadMetaHeaders` is REFUTED.

**Option A — per-connection accumulator.** A `headerAccum` (stream id + fragment buffer + the originating HEADERS frame's flags) on `ServerConn` and `ClientConn`; `onHeaders` stashes instead of decoding when `END_HEADERS` is clear; a new `onContinuation` appends and, on `END_HEADERS`, hands the reassembled block to an extracted `onHeaderBlockComplete` / `onResponseHeaderBlock`. **BUILT, COMPILES, `go vet` clean, existing `internal/filter/hcm/...` suites GREEN, and it fixes every BRAINSTORM arm by execution (§2).**

**Option B — `fr.ReadMetaHeaders = hpack.NewDecoder(...)`.** **BUILT. REFUTED on four independent grounds, three of them measured:**

1. ⚠️ **IT COMPILES CLEANLY AND THEN HANGS — A SILENT TYPE-SWITCH FALLTHROUGH.** With `ReadMetaHeaders` set, `ReadFrame` returns `*http2.MetaHeadersFrame`, which matches NO arm of either `dispatchFrame`; both fall through to `default:` — *"Unknown frame types are silently ignored per RFC 9113 §4.1"* — so **every HEADERS frame is silently discarded**. `go build ./...` succeeded with zero diagnostics; `go test ./internal/filter/hcm/h2/` then **hung until killed**, and a scoped `-run TestServerConn` with a 45 s timeout was `Terminated` (rc 143). **A change whose failure mode is "compiles, then hangs" is strictly worse than one that fails to compile.**
2. ⚠️ **IT MUTATES THE DECODER IT IS HANDED.** `readMetaFrame` calls `hdec.SetEmitEnabled(true)`, `hdec.SetMaxStringLength(...)` and — decisively — **`hdec.SetEmitFunc(...)`**, replacing the emit callback. `hpackState`'s decoder is constructed with an emit callback that appends into `st.fields`, which is the entire mechanism of `decodeBlock`. Passing `s.hpack.dec` would silently break `decodeBlock` for every remaining caller; passing a *separate* decoder splits connection-scoped dynamic-table state across two decoders, which is the very desynchronisation this row exists to fix.
3. ⚠️ **IT PRE-EMPTS ENVOY-GO'S OWN VALIDATION WITH DIFFERENT ERROR SHAPES.** `readMetaFrame` performs its own pseudo-header checks — `errPseudoAfterRegular`, duplicate-pseudo, mixed request/response types, `headerFieldNameError`, `headerFieldValueError` — and returns them as x/net `StreamError`s. envoy-go already implements that whole family in `buildRequest` (`stream.go`, cite BY SYMBOL) with its OWN frozen strings (`"pseudo-header after regular header"`, `"duplicate :method"`, …). Option B would make x/net's rejects fire FIRST, changing every error string in a family that **h2spec §8.1.2.1 exercises 4/4** and that **row 87 landed rejects into three weeks ago**.
4. **Blast radius, counted:** **6** non-test `HeadersFrame` sites and **37** test-side references would need revisiting — against Option A's **3** production files.

**And the decisive one: Option B does nothing whatsoever for the write leg (§3).** It is a read-side merge facility; the third leg is an emit-side gap.

### 1.2 Cost — MEASURED, not estimated

Prototype A, all three legs, probes removed, production only:

| File | added | deleted |
|---|---|---|
| `internal/filter/hcm/h2/client.go` | 136 | 91 |
| `internal/filter/hcm/h2/conn.go` | 81 | 14 |
| `internal/filter/hcm/h2/framer.go` | 38 | 0 |
| **total** | **+255** | **−105** ⇒ **net +150** |

⚠️ **The `client.go` figure is inflated on BOTH sides by pure code motion** — the old `case *http2.HeadersFrame` body was lifted verbatim into `onResponseHeaderBlock`. The IMPL band is **~+150-190 net production `.go` over THREE files**. ⚠️ **This is a MEASURED prototype and therefore a FLOOR** (`reference_measured_prototype_is_a_lower_bound`): row 87 broke a ten-firing streak only because it had ONE grep-confirmed call site; **this row has three decode sites, two write sites, and per-connection state on two structs.** Named un-enumerated classes: the bound's error-code decision (§4), reviewer-mandated arms, and the fixture extension (§5).

---

## 2. Q5 + the BRAINSTORM arms: prototype A is GREEN on every one, measured

Run against `probe88` with **the bound binary asserted by `ss -ltnp` before every reading** (§7 records why that assertion is not optional):

| Arm | tip | prototype A | reference (BRAINSTORM §2) |
|---|---|---|---|
| **A** control, single HEADERS | 200 `MARKED` | 200 `MARKED` | 200 `MARKED` |
| **B** `:path` in the CONTINUATION | `RST_STREAM PROTOCOL_ERROR` | **200 `MARKED`** | 200 `MARKED` |
| **C0** control, un-split + `x-probe` | 200, header present | 200, header present | 200 `SAW-X-PROBE` |
| **C** `x-probe` in the CONTINUATION | 200, **header GONE** | **200, header present** | 200 `SAW-X-PROBE` |
| **D** split, then an ordinary request | **`GOAWAY COMPRESSION_ERROR`** | **200 both streams** | 200 both streams |
| **E** §6.10 interleaved PING | `GOAWAY PROTOCOL_ERROR` | `GOAWAY PROTOCOL_ERROR` | `GOAWAY PROTOCOL_ERROR` |
| **G** stock transport, 20 000/40 000 B | header **ABSENT** | **header PRESENT** | preserved |
| **I** padded upstream response | `len(pad)=0` | **`len(pad)=20000/40000`** | n/a (upstream leg) |

**Q5 (reject-shape) is answered:** arm B's tip-side `RST_STREAM PROTOCOL_ERROR` becomes **200, matching the reference exactly** — the divergence closes rather than moving, and **no NEW divergence appeared on any arm**.

⚠️ **ONE NAMED, DELIBERATE BEHAVIOUR CHANGE ON ARM E.** The GOAWAY's `LastStreamID` moves **1 -> 0**. Post-fix the HEADERS is buffered rather than dispatched, so `lastInID` is never advanced before x/net's frame-order error tears the connection down. Direction and code are unchanged (`PROTOCOL_ERROR` both sides); only the last-stream-id differs. **The PLAN must decide whether to pin this** — it is a real wire-visible delta and it is recorded here rather than discovered at the IMPL.

---

## 3. ⚠️ THE THIRD LEG — D-88-SEQ IS REFUTED, AND THE READ FIX ALONE IS A REGRESSION

**`WriteContinuation` appears NOWHERE in `internal/`** (grep, whole tree, non-test and test alike). Both header-write sites — `ServerConn.encodeAndWriteHeaders` and the client's request-write in `client.go` — call `fr.WriteHeaders(... EndHeaders: true)` with the **entire** encoded block. **x/net's `WriteHeaders` does not split** (zero `Continuation` references in its body); it only rejects at the 16 MiB wire ceiling. So any outgoing block larger than the PEER's advertised `SETTINGS_MAX_FRAME_SIZE` is an **illegal oversized frame**.

**This is MASKED at the tip and UNMASKED by the read fix.** Measured with the read-side fix alone (no write fix):

- arm I at 20 000/40 000 B ⇒ **`ERR http2: frame too large`** — where the tip merely dropped the headers and returned 200.
- Re-run with the probe client's `MaxReadFrameSize` raised to 1 MiB ⇒ **200 with the full 20 000/40 000-byte pad**. **That is the proof envoy-go emitted ONE oversized HEADERS frame**, and that the peer's default limit is what rejects it.

**Why the request direction (arm G) passed while the response direction (arm I) failed:** x/net's **Server** defaults to `defaultMaxReadFrameSize = 1 << 20` (1 MiB) while its **Transport** uses the 16 KiB spec default. **The write gap is direction-independent; only the peer's advertised limit differs.** Do not read arm G's green as "the client write path is fine".

**D-88-SEQ (REVISED, three legs):**

| Leg | Scope | Anchor |
|---|---|---|
| **88.1** downstream/server READ | `conn.go` accumulator | arms B, C, D, G |
| **88.2** upstream/client READ | `client.go` accumulator | arms I, K |
| **88.3** WRITE, BOTH directions | `framer.go` `writeHeaderBlock` + the two call sites | arm I at default settings |

⚠️ **HARD ORDERING CONSTRAINT, MEASURED: 88.3 MUST land with or before 88.2.** Landing 88.2 alone converts a silent header drop into a hard `frame too large` failure — **a strictly worse user-visible outcome than the defect**. 88.1 alone remains safe and shippable (the request-direction write path is only exercised at >1 MiB against an x/net Server). **The PLAN should treat 88.2+88.3 as ONE atomic leg.**

⚠️ **NO sub-phase ROWS are minted at this stage either** (the row-84 precedent). If the PLAN mints `88.1`/`88.2`/`88.3`, that is a second `want` move and a sentinel-affecting edit — **measure both sides, never forecast.**

---

## 4. Q2 — bounding the accumulator

`maxHeaderBlockSize = 16 << 20` (16 MiB), **chosen to match the limit x/net's own `ReadMetaHeaders` path applies** via `Framer.maxHeaderListSize()`'s documented default, so the two options are not silently different in their exposure. On exceed: drop the accumulator and return a **connection** error.

**OPEN for the PLAN, deliberately not frozen here:**
- **The error code.** The prototype uses `ErrFrameSizeError`. `ENHANCE_YOUR_CALM` is the more idiomatic answer for a flood and is what several proxies emit. **Neither is measured against the reference yet** — the PLAN owes a reference probe or an explicit ADR ruling.
- **Whether a stat is owed.** ⚠️ **A new stat name would break the anticipated stat-surface +0** (§6). The prototype adds none, and the row's default should stay +0 unless the PLAN argues otherwise on the record.
- **The write side needs NO bound** — it splits whatever it is given; it never accumulates.

---

## 5. Q4 — the differential proof shape: EXTEND, and the seam is the repo's OWN helper

**Confirmed by execution, with the bound binary asserted on each side.** A `q4probe` package driving `test/helpers.H2CRoundTrip` — *the exact seam fixture drivers use* — with a padded request header:

| padding | tip | prototype A |
|---|---|---|
| 1 024 B | `x-probe` present, `x-pad` present | present, present |
| **20 000 B** | **`x-probe` ABSENT, `x-pad` ABSENT** | **present, present** |

**No raw framer is needed in a fixture driver.** `H2RoundTrip`/`H2CRoundTrip` already accept `headers []hpack.HeaderField` and x/net splits the block automatically past 16 KiB.

**D-88-DIFF (PROPOSED): EXTEND an existing H2 fixture rather than mint `0120`.** ⚠️ **ONE BLOCKER THE PLAN MUST RESOLVE, FOUND BY READING RATHER THAN ASSUMED:** `0004-h2-routing`'s backend (`backends/main.go`) returns fixed bodies and **does not echo request headers**, so it cannot report which headers arrived. The options, in cost order:
1. Extend `0004`'s own backend with a header-reflecting path (local to the fixture, no new `BackendKind`, no new port).
2. Host the arms in a fixture whose backend already reflects headers — note `HTTPHeaderMutation` (`BackendKind = 9`) is **HTTP/1.1**, and envoy-go mirrors the downstream protocol upstream (measured at the BRAINSTORM: an H2 downstream against an H1 backend yields the H2 preface and a 502), so it is **not** usable behind an H2 downstream.
3. Mint `0120` (next-free, verified).

⚠️ **The response-direction arm (88.2/88.3) additionally needs a backend that EMITS a large header block.** The prototype used an h2c echo backend with `?many=N` / `?big=N`; a fixture needs the equivalent. **This is the single largest un-enumerated cost in §1.2 and the PLAN owes it a measured line count.**

---

## 6. Q3 — the client-leg desync question, SETTLED, with its mechanism named

The BRAINSTORM left open whether the client leg desyncs its HPACK decoder the way the server leg provably does.

⚠️ **THE FIRST PROBE COULD NOT HAVE ANSWERED IT, AND SAYING SO IS THE POINT.** Arm J (three sequential 20 000-byte-padded responses) showed no failure — but **a single 20 KB header value exceeds the 4096-byte HPACK dynamic table and is therefore never indexed**, so dropping it cannot desync anything. The probe was blind to the axis it was built to test (`topic_probe_discipline`).

**Arm K re-runs it with 200 SMALL (100 B) indexable headers** past the 16 KiB split point:

| | tip | prototype A |
|---|---|---|
| K[1] `?many=200` | **165 of 200** headers, upstream-conn header itself **LOST** | **200 of 200**, conn `…:48240` |
| K[2] `?many=200` | **165 of 200**, still succeeds | **200 of 200**, conn `…:48248` |
| K[3] control | 0 (correct), conn `…:40988` | 0 (correct), conn `…:48260` |

**ANSWER: the client leg loses headers silently but does NOT poison the connection — and the reason is connection lifecycle, not code safety.** The three distinct upstream source ports (48240 / 48248 / 48260) show **envoy-go opens a FRESH upstream connection per request in this configuration**, so a desynced upstream decoder never survives to a second request. ⚠️ **The client-side code path is otherwise identical to the server's. This is a latent hazard masked by connection lifecycle, and any future upstream H2 connection reuse would expose it.** Recorded as a bounded measurement, NOT as "the client leg is safe".

---

## 7. Q8 — h2spec is not a red anchor, and this SPEC found WHY

**Measured at BOTH tip and prototype, each from its own run: `95 tests, 94 passed, 1 skipped, 0 failed`** (skip `6.9.2/2`, invariant). **Identical — so h2spec neither catches the defect nor regresses on the fix.** ADR-0307's measurement is re-confirmed.

⚠️ **THE FINDING THAT MAKES THIS QUOTABLE: h2spec's §6.10 CONTINUATION section passes 6/6 AT THE TIP THAT DISCARDS EVERY CONTINUATION FRAME** — including test **1: *"Sends multiple CONTINUATION frames preceded by a HEADERS frame"*** and test **5: *"…preceded by a CONTINUATION frame with END_HEADERS flag"***. §6.2 HEADERS is 4/4 and §8.1.2.1 Pseudo-Header Fields is 4/4 on both sides too. **A conformance section that NAMES the feature is fully green while the feature is entirely absent**, because those tests assert the connection's *reaction* (no error / correct error), never that the header fields arrived. **A green gate is not evidence of a present feature.** This belongs in the ADR.

---

## 8. Q6 — the contract: there is NO sentence to correct, and that is a finding

⚠️ **The BRAINSTORM's framing implied a false contract sentence would need riding. It does not exist.** `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` section (`:2019`, 63 lines to `## HTTP filter chain` at `:2081`) makes **NO claim about CONTINUATION, header-block framing, `END_HEADERS`, or max frame size** — verified by a scoped grep over exactly that range. Every whole-file `CONTINUATION` hit (`:782`, `:798`, `:820`, `:823`, `:825`, `:5109`) is the **tracing** sense — "continuation requests" that continue an inbound trace — and `:5281` is an async-resume "continuation site". **Do not conflate them; do not edit them.**

**So the IMPL ADDS a new contract bullet rather than riding a correction** (unlike row 87, which rode ADR-0309 on a retained false sentence). ⚠️ **Cite by STRING or SYMBOL** — row 87's `:2036` insertion shifted every by-line citation at or below it by +1.

**The one landed FALSE artefact this row does kill is the code comment**, whose two clauses are both refuted by execution:
```go
// Handled by framer as part of ReadMetaHeaders; reaching here means the
// framer gave us a raw ContinuationFrame (shouldn't happen in normal usage,
// but be safe and ignore).
```
`ReadMetaHeaders` is assigned **only** in `test/differential/runner_test.go`, never in `internal/` — and §1.1 now shows that assigning it would *hang the server*, so the comment describes a configuration that was never safe to adopt.

---

## 9. Anticipated counts — each re-derived MECHANICALLY at this tip

- **ADR-0310** — TAIL-derived (`grep -oE '^## ADR-[0-9]+' | tail -1` -> `## ADR-0309`; `grep -c '^## ADR-0310'` -> **0**). ⚠️ headings+1 **COLLIDES** at the ADR-0209 gap. `DECISIONS.md` **18150** lines · **308** headings · `^---$` **216** · STATUS census **22** · strict `PROPOSED` guard **0 -> 1 AT THIS STAGE** (the live pointer the IMPL disarms).
- **fixtures 121**, tail `0119-grpc-unary-trailers`, next-free **`0120`**. ⚠️ **Delta NOT yet +0** — §5's extend-vs-mint decision is the PLAN's.
- **fuzzers 55 / 48 files** — anticipated **+0**, and now stated EXPLICITLY rather than inherited: the accumulator consumes no new config field, and its inputs are already covered by the existing `internal/filter/hcm/h2` fuzz surface over framer input. ⚠️ **The PLAN may still choose to add one for the reassembly buffer; if so, say so and move the count.**
- **BackendKind tail 38** — anticipated **+0** under §5 option 1 or 3; option 2 is foreclosed anyway. (A TAIL VALUE, not a count: 39 declarations, `TCPEcho = 0`.)
- **`go.mod` +0 — MEASURED on the prototype** (`git diff --stat -- go.mod go.sum` empty). Uses only `bytes` and the already-imported `golang.org/x/net/http2`.
- **stat surface DELTA 0 — MEASURED on the prototype** by the same command both sides: `NewCounter(|NewGauge(` occurrence form reads **406** in BOTH trees. No absolute carried (1205 vs 1207 stays contested).

---

## 10. What the PLAN owes

1. **Task decomposition under the REVISED three-leg D-88-SEQ**, honouring the **88.3-with-or-before-88.2** ordering constraint, with the RED anchor for each leg named and re-proven at the PLAN's tip.
2. **§5's fixture blocker resolved** — extend `0004`'s backend, host elsewhere, or mint `0120` — **with the test-side line count MEASURED, not estimated.** This is the largest un-enumerated cost.
3. **§4's two open decisions frozen**: the over-size error code (probe the reference or rule in the ADR) and the stat question (default: none).
4. **§2's arm-E `LastStreamID` 1 -> 0 delta**: pin it, or record it as deliberately unpinned.
5. **The unit-test roster** for `writeHeaderBlock` (block exactly at, one under, and one over `maxFrameSize`; a zero `MaxFrameSize` defaulting to 16384) and for the accumulator (nil-accumulator, wrong-stream, over-bound — all three currently unreachable behind x/net's `checkFrameOrder` and therefore **defence in depth that must be labelled as such, not counted as coverage**).
6. **A per-task budget on top of the measured `~+150-190` production floor.**
7. The sentinel + all four NCs, ONE side (a PLAN edits no ROADMAP — prove by EMPTY DIFF).

---

## 11. Probe hygiene

Both prototype worktrees (`wt-88-proto-a`, `wt-88-proto-b`) are **DETACHED, measured, and DELETED WHOLE at close**; `probe88/` and `q4probe/` were removed before the final cost measurement, and each prototype tree was verified to carry **only production edits**. The h2c echo backend and both subject binaries were killed by filtered PID and their ports verified released. **No container was created at this stage**, and the pre-existing containers belonging to other work were not touched.
