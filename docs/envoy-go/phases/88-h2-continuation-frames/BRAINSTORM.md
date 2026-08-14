# Phase 88 — `h2-continuation-frames` — BRAINSTORM

**Date:** 2026-08-13 · **Base master:** `244e482f` (from `git rev-parse master`, NOT a SHA quoted in a brief) · **Branch:** `phase-88-brainstorm` · **Lifecycle-state:** DONE -> 1

**Method — NAMED INLINE DEPARTURE (the 87 BRAINSTORM/SPEC/PLAN precedent, a FOURTH consecutive stage):** no investigation agents; every probe run INLINE by the controller. `feedback_execution_style` (subagent-driven) remains the standing preference and is expected to return at the IMPL, where the work is parallel and file-disjoint. Probes: the tip binary built with `-o` into session scratch; a **raw HEADERS/CONTINUATION h2c client** (`probe88`, x/net `Framer` + `hpack`, written into the worktree, **DELETED at close with the tree proven clean**); a stock `x/net/http2.Transport`; an h2c echo backend served by the same probe binary; ONE reference container (`b88-main`, `contrib-v1.37.2`, torn down **BY NAME**, 0 remaining). Ports **47450/47451** (subject), **47455** (echo backend), **47470** (reference) — bound transiently by probe processes only; nothing survives the stage. ⚠️ **FOUR pre-existing containers from other work were present and DELIBERATELY UNTOUCHED.**

**`ROADMAP.md` is edited by exactly ONE line at this stage** — row 88 registered `in-progress` — and the sentinel `want` is bumped **119 -> 120 in the SAME commit** (the phase-84/85/86/87 precedent).

---

## 1. The pick, and why it is defensible as "smallest first"

**SELF-PICKED** per the 2026-07-12 standing directive. Phase 87 is CLOSED, all 119 chartered rows are `done`, and no banked mid-lifecycle work exists.

**THE PICK: the CONTINUATION two-sided repair.** It was the standing "strongest named candidate" carried by the last three routers, and it was deferred at rows 85, 86 and 87 as "the natural next LARGE row".

⚠️ **THIS BRAINSTORM PRODUCED THE FACT THAT CHANGES THE PRIORITY, AND IT IS NEW: the defect is reachable by ORDINARY CLIENTS, not just by a raw frame writer.** Every prior stage treated CONTINUATION as an exotic wire shape. Measured at this tip (§2.5): a **stock `x/net/http2.Transport`** — the same library Go's own `net/http` H2 client, this repo's `test/helpers`, and a large share of real Go traffic use — **splits a header block automatically once it exceeds the peer's advertised `SETTINGS_MAX_FRAME_SIZE`**, and envoy-go then **silently drops every header past the boundary while still answering 200**. A single large cookie or JWT is enough. `accept-encoding` and `user-agent` were observed disappearing from an otherwise ordinary GET.

The standing directive says *smallest defensible candidate first*. This row is not the smallest by line count, and that is recorded rather than argued away. It is chartered anyway because **every candidate that is genuinely smaller is not a product defect at all** (§4): the stat-surface recount, the REVIEW.md restoration and the hygiene fold-ins have **nothing to reproduce by execution**, and a BRAINSTORM whose charter cannot be red-anchored is not a defensible row. The two smaller *product* candidates are both LARGER than this one on re-derived cost (§4). **Severity is doing the work here, and it is measured severity, not asserted severity: silent header loss on ordinary traffic, plus a connection-fatal `COMPRESSION_ERROR` (§2.4).**

**D-88-SEQ (PROPOSED, the SPEC decides): charter as a SPLIT phase, two legs** — **88.1 the DOWNSTREAM/server leg** (`internal/filter/hcm/h2/conn.go`) and **88.2 the UPSTREAM/client leg** (`internal/filter/hcm/h2/client.go`). Both legs are independently red-anchored below, and 88.1 alone is a coherent shippable fix. ⚠️ **NO sub-phase ROWS are minted at this stage** — the row-84 precedent is a split recorded in PROSE with a single parent row, and minting `88.1`/`88.2` rows would move `want` a second time. If the SPEC mints them, that is a sentinel-affecting edit and must be MEASURED at that stage, not forecast here.

---

## 2. The defect, REPRODUCED BY EXECUTION at `244e482f` — with positive controls on every arm

`internal/filter/hcm/h2/conn.go`'s `dispatchFrame` (cite **BY SYMBOL**; the arm sits at `:264-268` at this tip, and the gRPC-family window's carried cite of `:255-259` has DRIFTED) reads:

```go
case *http2.ContinuationFrame:
    // Handled by framer as part of ReadMetaHeaders; reaching here means the
    // framer gave us a raw ContinuationFrame (shouldn't happen in normal usage,
    // but be safe and ignore).
    return nil
```

⚠️ **BOTH CLAUSES OF THAT COMMENT ARE FALSE, and this is re-confirmed by execution rather than inherited from the gRPC-family window's prose.** `newFramer` (`framer.go`) never assigns `ReadMetaHeaders`; a repo-wide grep finds it assigned **only in a test helper** (`test/differential/runner_test.go:3348`), never in `internal/`. So the framer *does* hand up raw `*http2.ContinuationFrame`s, this arm *is* reached in normal usage, and "ignore" is **not** safe.

The second half of the defect is in `onHeaders`, which decodes with `f.HeadersEnded()` and then **proceeds regardless**:

```go
headers, err := s.hpack.decodeBlock(f.HeaderBlockFragment(), f.HeadersEnded())
```

`decodeBlock` skips `dec.Close()` when `endBlock` is false, but `onHeaders` never branches on it: the PARTIAL field set flows straight into `recvHeaders` and the stream is dispatched. The continuation's bytes are then thrown away, so **envoy-go's HPACK dynamic table permanently diverges from the peer's.**

### 2.1 Probe configuration and the positive control

Subject: tip binary, h2c listener on `127.0.0.1:47450`, routes `/marked` -> `MARKED`, `/plain` -> `PLAIN`, `/echo` -> an h2c echo backend that reflects received headers, catch-all -> 404.

| Arm | Shape | Subject at tip | Reference `contrib-v1.37.2` |
|---|---|---|---|
| **A (positive control)** | single HEADERS, `END_HEADERS=1`, `:path=/marked` | **200 `MARKED\n`** | **200 `MARKED\n`** |

**The control is green on both sides, so every failure below is a real divergence and not a broken probe.**

### 2.2 Arm B — a legal request is REJECTED (`:path` in the CONTINUATION)

HEADERS(`END_HEADERS=0`) carrying `:method`/`:scheme`/`:authority`; CONTINUATION carrying `:path=/marked`.

- **Subject: `RST_STREAM stream=1 code=PROTOCOL_ERROR`** — the request never runs. `:path` was in the discarded fragment, so `buildRequest` saw a request with no path.
- **Reference: `200 MARKED\n`** — reassembles correctly and routes on the continuation's `:path`.

### 2.3 Arm C — a request SUCCEEDS while a header is SILENTLY LOST

HEADERS(`END_HEADERS=0`) carrying all four pseudo-headers; CONTINUATION carrying `x-probe: yes`.

- **Subject: 200, and the echoed header set is `PATH=/echo PROTO=HTTP/2.0` — `x-probe` is GONE.** Its own un-split control (**arm C0**, same request in one frame) echoes `H x-probe: yes`, so the absence is a **loss**, not a probe artifact.
- **Reference: 200 `SAW-X-PROBE`** on both the split and un-split arms.

⚠️ **This is the dangerous mode: no error, no status change, no log line.** An auth token, an `x-forwarded-*` header, or any header a downstream policy depends on can vanish while the request is still routed and served.

⚠️ **NAMED PROBE ASYMMETRY:** the subject observes `x-probe` through the **echo backend**, the reference through a **header-match route** (`match.headers`), because (a) envoy-go rejects `match.headers` outright — `route 0: match.headers is not supported in phase 04`, measured — and (b) the container did not reach the host echo backend (`UNREACHABLE`, verified with `/dev/tcp` from inside the container). **Each side is read against its OWN control**, and the reference's header-match route was itself negative-controlled: `/hdr` without the header returns `NO-X-PROBE`, with it `SAW-X-PROBE`.

⚠️ **CORRECTION TO (b), MADE AGAINST THIS STAGE'S OWN FIRST READING.** The original wording blamed "this host's firewall", which was an **unverified causal claim** — no firewall rule was ever inspected. The actual cause is a **documented trap this stage walked into**: the probe dialled `172.17.0.1`, the **bridge IPAM gateway**, which is precisely the address `reference_host_gateway_ip_docker_desktop` records as NOT working. The reachable address is whatever docker resolves the magic string **`host-gateway`** to — via `--add-host=host.docker.internal:host-gateway` (dialling the HOSTNAME), or resolved as a literal through a throwaway `getent hosts host.docker.internal` container (the `differential.HostGatewayIP` / `0092`-driver-local pattern). ⚠️ **And any reference cluster dialling `host.docker.internal` needs `dns_lookup_family: V4_ONLY`**, since the embedded DNS also returns an AAAA that does not forward and `AUTO` prefers it — a SILENT connect-fail that reads as "the backend emits nothing". **So a host-bound backend IS reachable from the reference container, and the probe asymmetry above was a CONVENIENCE, not a forced choice.** The asymmetry is retained because each side carries its own control and the measurement stands; **the SPEC must not treat (b) as foreclosing a symmetric echo-backend design.** *(This is the §2 reproduction's one causal over-claim, caught and corrected in the same stage that made it — the `reference_a_drift_correction_is_itself_a_claim` discipline: the corrected mechanism is cited to a memory, and no firewall claim is carried in either direction.)*

### 2.4 Arm D — ONE split request POISONS THE WHOLE CONNECTION

Stream 1 sends a split request whose CONTINUATION inserts a header into the HPACK dynamic table; stream 3 then sends a **perfectly ordinary, un-split** request re-using that table entry.

- **Subject: stream 1 answers 200 `PLAIN\n`, then stream 3 -> `GOAWAY code=COMPRESSION_ERROR`.** Subject log: `hcm: h2: h2: COMPRESSION_ERROR: HPACK decode failed`. Every later stream on that connection is dead.
- **Reference: stream 1 200, stream 3 200 `PLAIN\n`.** No desync.

**The blast radius of one split request is the connection, not the request.**

### 2.5 ⚠️ Arms G / H — THE BLAST RADIUS IS ORDINARY TRAFFIC (no raw framer involved)

A stock `x/net/http2.Transport` against the subject's `/echo`, varying one padding header:

| Padding | Subject | Reference (`/hdr` + header-match route) |
|---|---|---|
| 1 024 B | 200, header **PRESENT** | 200 `SAW-X-PROBE` |
| 8 192 B | 200, header **PRESENT** | — |
| 15 000 B | 200, header **PRESENT** | 200 `SAW-X-PROBE` |
| **20 000 B** | **200, header ABSENT** | **200 `SAW-X-PROBE`** |
| **40 000 B** | **200, header ABSENT** | **200 `SAW-X-PROBE`** |

The threshold sits between 15 000 and 20 000 bytes, consistent with the default 16 384-byte `SETTINGS_MAX_FRAME_SIZE`. ⚠️ **In the 20 000/40 000 subject runs the surviving header set was `PATH=/echo PROTO=HTTP/2.0` plus `x-probe` ALONE — `accept-encoding`, `user-agent` and the padding header were ALL silently dropped**, i.e. the loss is not confined to exotic headers; it takes whatever falls past the frame boundary. **The exact threshold and which headers land where are ordering-dependent and must be re-derived at the SPEC, not carried from this table.**

### 2.6 Arm I — the CLIENT leg, proven by execution with a bypass control

`internal/filter/hcm/h2/client.go` has **no `*http2.ContinuationFrame` case at all** (it falls to `default`, silently ignored) and carries the same `decodeBlock(fr.HeaderBlockFragment(), fr.HeadersEnded())` shape at `:417` and `:428`. Probe: the upstream returns a padded RESPONSE header block, which x/net's `http2.Server` splits.

| Response padding | Client -> backend DIRECTLY (bypass control) | Client -> **through envoy-go** |
|---|---|---|
| 1 024 B | `len(x-resp-pad)=1024` | `len(x-resp-pad)=1024` |
| **20 000 B** | `len(x-resp-pad)=20000` | **`len(x-resp-pad)=0`** |
| **40 000 B** | `len(x-resp-pad)=40000` | **`len(x-resp-pad)=0`** |

Status stayed **200** on every arm. **envoy-go silently drops RESPONSE headers from any upstream that splits its header block.** The bypass control proves the backend really sent them.

⚠️ **OPEN, DELIBERATELY NOT CLAIMED:** whether the client leg ALSO desyncs its HPACK decoder the way arm D proves the server leg does. Two consecutive large-response arms both succeeded, which is *consistent with* per-request upstream connections but does **not** establish it. **The SPEC must settle this by probe; this BRAINSTORM asserts only the header loss it measured.**

---

## 3. What is already free, and what envoy-go actually owes

⚠️ **A COST REDUCTION FOUND BY EXECUTION, AND IT NARROWS THE ROW.** Arm E interleaves a PING between a `END_HEADERS=0` HEADERS and its CONTINUATION — an RFC 9113 §6.10 violation.

- **Subject: `GOAWAY code=PROTOCOL_ERROR`.** **Reference: `GOAWAY code=PROTOCOL_ERROR debug="unexpected non-CONTINUATION frame or stream_id is invalid"`.**

**Direction parity — but the subject's green is NOT envoy-go's doing.** The subject log reads `h2: PROTOCOL_ERROR: framer: connection-error code=1`, i.e. it came from `translateFramerErr` mapping an error raised by **x/net's own `Framer.checkFrameOrder`** (`frame.go:519/543`, which tracks `lastHeaderStream` and rejects any non-CONTINUATION frame, or a CONTINUATION on the wrong stream, while a header block is open). ⚠️ **Attributing this to envoy-go would have been exactly the `reference_code_comment_not_evidence` error the false comment in §2 already committed once.**

**Consequence for scope: the row does NOT owe a §6.10 frame-ordering state machine.** The embedded framer supplies ordering enforcement, CONTINUATION-on-wrong-stream rejection, and the `lastHeaderStream` bookkeeping. What envoy-go owes is narrower: **accumulate header-block fragments until `END_HEADERS`, then decode the assembled block exactly once** — on both the server and the client path. **This is the single most important input to the SPEC's cost prototype, and it is why §6's floor sits well under the carried "2-4x row 85" estimate.**

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

Per `reference_deferred_candidate_cost_restale`, a carried cost is stale by construction. Each was re-measured at `244e482f`:

| Candidate | Re-derived at this tip | Verdict |
|---|---|---|
| **`ssl.connection_error`** | Still **absent as a landed stat name**: 3 comment references in `internal/listener/manager.go` (`:373`, `:411`, `:1292`) + 1 in `quic_test.go:234`, 2 mentions in `BEHAVIOR_CONTRACT.md`. Carried floor **+444 whole-`.go`** — **LARGER than this row's floor (§6)**, and it lands a stat NAME (stat-surface delta ≠ 0, extra gate burden). | REJECTED — bigger, and not a correctness defect |
| **`test/conformance/grpc/`** | `test/conformance/` contains **only** `h2spec/` and `proxy-wasm/`; the interop client declared at `BOOTSTRAP_PROMPT.md:350` still does not exist. Building it is a new subsystem, and the gRPC family's own window records it as **9/26 reachable** — i.e. most of it is foreclosed by the measured buffering and H1->H2 ceilings. | REJECTED — larger, and mostly unreachable |
| **stat-surface recount** | The contested absolute is still **1205 vs 1207**, and this row re-measured the occurrence form at **`NewCounter(` 327 / `NewGauge(` 79 = 406** — a DIFFERENT form from the lineage's carried `145/21`. **No absolute is corrected and none is carried.** | REJECTED as a standalone row — it **rides a +0 row**, and this row is not +0 |
| **REVIEW.md restoration** | **37** of **128** phase dirs carry one; unchanged. | REJECTED — process-not-product; **no defect to reproduce**, so no red anchor |
| **D-86-CONN `client.Close` gate** | `boot.NewValidateSDSProvider` present (`internal/boot/boot.go:240`); the Close is still ungated. ~10 lines. | REJECTED as a row — explicitly **a fold-in**; too small to be a defensible phase. **Carried forward as a fold-in candidate.** |
| **hygiene fold-ins** (`harness_test.go:208` stale port inventory, xDS cycle-guard automation) | Unchanged. | REJECTED — fold-ins, not rows; no red anchor |
| **the ~42 candidates in the six family windows** | Windows re-read at `:197 :203 :209 :219 :225 :233`. | REJECTED for now — see §5 |

---

## 5. Family attribution — and the counter-argument, recorded rather than hidden

**Chartered as a core-HCM / HTTP-2-codec MAINTENANCE row claiming NO family ordinal** (the row-85 / row-86 / row-87 precedent: a maintenance row repairs a landed deliverable and does not extend a charter).

⚠️ **THE COUNTER-ARGUMENT, STATED PLAINLY: unlike row 87, THIS ROW'S PROVENANCE IS *INSIDE* A SENTINEL WINDOW.** The gRPC-family paragraph at `ROADMAP.md:203` names "the CONTINUATION-discard defect" in its `remaining deferred (not-yet-chartered) candidates:` list. Someone could reasonably file this as a gRPC-family row. It is filed as core-HCM maintenance because **the defect is protocol-generic**: it is reached by any H2 client with a large header block (§2.5) and by any H2 upstream with a large response header block (§2.6); nothing about it is gRPC-specific. It appears in that window only because the phase-84 BRAINSTORM happened to be the sweep that found it.

**Sentinel consequences, stated as obligations rather than forecasts:**
- **At THIS stage the window is BYTE-UNTOUCHED** and check (2) is re-measured at SIX after the registration (§8). The row is not `done`, so nothing narrows yet.
- **At ROW-DONE the `:203` sentence narrows** (one candidate leaves a list of ~11). The phrase `remaining deferred (not-yet-chartered) candidates:` survives a one-item removal, so check (2) *should* stay SIX — ⚠️ **but that is a PREDICTION and it MUST BE MEASURED at the IMPL on both sides of the edit, never forecast.** `reference_sentinel_matcher_string_self_clears` forecloses every "retire the paragraph" shape.
- ⚠️ **Do NOT let this row's own adjectives acquire ADR authority** (`reference_brainstorm_adjective_acquires_adr_authority`): the SPEC must grep for the SENTENCE it intends to change, not inherit this section's framing.

---

## 6. Anticipated ADR, counts, and the cost FLOOR

- **Anticipated ADR: `ADR-0310`** — **TAIL-derived** (`grep -oE '^## ADR-[0-9]+' | tail -1` -> `## ADR-0309`; `grep -c '^## ADR-0310'` -> **0**). ⚠️ Headings+1 **COLLIDES** at the ADR-0209 gap; never derive from the count. `DECISIONS.md` **18150** lines · **308** headings · `^---$` **216** · **strict `^> **STATUS: PROPOSED` guard 0 — DISARMED; the phase-88 SPEC RE-ARMS it to 1.**
- **Counts re-derived MECHANICALLY at this tip, each anticipated at +0 unless noted:**
  - **fixtures 121**, numeric tail `0119-grpc-unary-trailers`, **next-free `0120`**. **NC OBSERVED:** `mkdir test/fixtures/0120-nc-fake` -> **122**, removed -> **121**. ⚠️ Anticipated delta is **NOT yet +0** — whether this row extends an existing fixture (the row-87 `0004` precedent) or mints `0120` is a SPEC decision.
  - **fuzzers 55 / 48 files** — anticipated **+0** (no new config field, so no new parse arm). ⚠️ A CONTINUATION reassembly buffer is a plausible fuzz target; the SPEC should say yes or no explicitly rather than inherit +0.
  - **BackendKind tail 38** (`H2GoawayResponder BackendKind = 38`, `fixture.go:614`) — anticipated **+0**. ⚠️ **A TAIL VALUE, not a count** (39 constants, `TCPEcho = 0`); do NOT "fix" it to 39.
  - **`go.mod` +0 anticipated** — the fix uses `bytes` and the ALREADY-imported `golang.org/x/net/http2`; no new sub-package, so `reference_new_subpackage_pulls_transitive_module` does not bite. Re-verify with `go mod tidy -diff`.
  - **stat surface: anticipated DELTA 0.** Assert the DELTA by the SAME command on both sides; carry NO absolute (1205-vs-1207 stays contested and WIDENED).
- **COST FLOOR — quoted as a FLOOR, and the reason it is BELOW the carried estimate is NAMED.** The carried figure was **2-4x row 85 (+1046 realized)**. §3 removes the largest imagined component (a §6.10 ordering state machine — x/net already supplies it), and the remaining work is one accumulate-until-`END_HEADERS` buffer per direction. **Estimated production ~+60-140 net `.go` over TWO files; tests a floor of ~+300-700 net.** ⚠️ **THIS IS AN ESTIMATE, NOT A MEASUREMENT — the SPEC MUST ENUMERATE IT BY COMPILING PROTOTYPE.** `reference_measured_prototype_is_a_lower_bound` fired **ten consecutive times** before row 87 broke the streak, and ADR-0309 §Consequences (iii) records that row 87's narrow escape rested on **ONE grep-confirmed call site**. **This row has at least THREE decode sites across TWO files (`conn.go` `onHeaders`; `client.go` `:417` and `:428`) and touches per-connection state — nothing here licenses treating an estimate as a floor-that-holds.**

---

## 7. What the SPEC owes (the §4-style question set)

1. **Q1 — the reassembly primitive.** Per-connection pending-header-block state (stream id + accumulated fragment buffer + the original HEADERS flags) vs. assigning `Framer.ReadMetaHeaders` and letting x/net reassemble. **Decide by COMPILING PROTOTYPE, and price both.** `ReadMetaHeaders` looks cheap but changes the frame type the whole dispatch switch receives (`*http2.MetaHeadersFrame`) and imposes its own limits — **measure the blast radius before preferring it.**
2. **Q2 — bounding the buffer.** An unbounded accumulator is a memory-exhaustion vector (the CONTINUATION-flood class). What limit, what error code, and does it need a stat? ⚠️ A new stat name would break the anticipated stat-surface +0.
3. **Q3 — the client leg's desync question, left OPEN at §2.6.** Settle by probe.
4. **Q4 — the differential proof shape.** Row 87 measured that x/net delivers exotic wire shapes unmodified and that `0004` is extensible; §2.5 shows a **stock transport with a padded header reproduces this WITHOUT a raw framer**, so the cheap path is an extension, not a new fixture. **Confirm by execution; the raw `probe88` client is the measured fallback and is described here well enough to rebuild.**
5. **Q5 — the reject-shape divergence.** Arm B: subject `RST_STREAM PROTOCOL_ERROR` vs reference 200. Post-fix both should be 200; confirm no NEW divergence is introduced, and decide whether any arm is differentially assertable or unit-only (the D-87-REJECT-SHAPE precedent).
6. **Q6 — the false comment and the contract.** The `dispatchFrame` comment must go. Does `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` section carry a sentence this row refutes? ⚠️ **Grep for the SENTENCE; do not inherit §2's framing.** Note the `:2036` insertion from row 87 shifted every by-line cite at or below it by +1 — **cite by STRING or SYMBOL.**
7. **Q7 — D-88-SEQ.** Confirm or refute the two-leg split; if sub-phase rows are minted, that is a second `want` move and a sentinel-affecting edit.
8. **Q8 — h2spec.** ADR-0307 records h2spec as **MEASURED not to gate either side**, so it is **NOT** available as a red anchor. Re-confirm at the SPEC's own tip and cite `95 tests, 94 passed, 1 skipped, 0 failed` **only from that run**.

---

## 8. Sentinel — RUN MECHANICALLY, BOTH SIDES, ACTUAL OUTPUT RECORDED

`stop` was **NOT** created; verified **ABSENT at the repo root AND the stage worktree**, both sides. Input measured **237 -> 238 lines / 119 -> 120 data rows**.

| Check | BEFORE (`want=119`) | AFTER (`want=120`) |
|---|---|---|
| **(1)** rows not `done` | **SILENT** — the FIFTH silent reading in project history, and CORRECT: every row really was `done` | **`NOT DONE: row 88`** — ALONE, denominator silent |
| **(2)** deferred-candidate windows | **SIX** at `:197 :203 :209 :219 :225 :233` | **SIX** at `:198 :204 :210 :220 :226 :234` — shifted uniformly **+1** by the single row inserted above all six windows |
| **(3)** unopened families | **SILENT** | **SILENT** |

**The condition is a CONJUNCTION and check (2) still prints SIX ⇒ the sentinel does NOT fire, and a next stage EXISTS.**

⚠️ **ALL FOUR NCs FIRED ON BOTH SIDES.** With check (1) SILENT on the BEFORE side, the row-62 doctoring NC is the **only** thing distinguishing a correctly-all-done board from a check that had silently stopped parsing:

- **row-62 doctoring** (`NC LANDED? [ in-progress ]` INSPECTED FIRST, both sides): BEFORE -> **`NOT DONE: row 62` ALONE — the silent side still LOOKS**; AFTER -> **`NOT DONE: row 62` AND `NOT DONE: row 88`**.
- **`want` off-by-one:** BEFORE `want=118` -> `GATE FAIL: examined 119 data rows, expected 118`; AFTER `want=119` -> `GATE FAIL: examined 120 data rows, expected 119`.
- **check-(3) doctoring** (residual confirmed **2 -> 0** on the doctored copy FIRST) -> `NEVER OPENED: gRPC   <- NC FIRED`, while `WASM` correctly stayed **silent**.
- **check-(2) one-arm counts:** long arm alone **5**, short arm alone **1**, union **6** — ⚠️ a one-arm strip is **NOT** an NC for the union (never 6 -> 0).

**Every leak axis INVARIANT across the registration** (⚠️ pass `--` before a `-`-leading pattern; the counts are form-dependent — occurrences vs LINES): `-family row` **95 occurrences / 67 LINES** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**. The new row's slug occurrences sit entirely inside row 88's own line. ARM-A flags **{119, 131}** UNAFFECTED — those rows sit ABOVE the insertion point, and the fragile escape-aware command was deliberately NOT re-run.

---

## 9. Probe hygiene

`probe88/` was written into the stage worktree, built with `-o` into session scratch, and **DELETED at close**; `git status --porcelain` and `git diff --stat master` are both verified EMPTY apart from this stage's four intended doc edits. The reference container `b88-main` was removed **BY NAME** with **0 remaining**, and the four pre-existing containers belonging to other work were left untouched. No process, port, or container from this stage survives it.
