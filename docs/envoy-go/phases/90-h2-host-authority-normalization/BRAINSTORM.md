# Phase 90 — `h2-host-authority-normalization` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1)
**Base master:** `6b4bc7c0`
**Branch:** `phase-90-brainstorm`
**Date:** 2026-08-19
**Row:** 90, registered `in-progress` at `ROADMAP.md` in THIS commit, `want` bumped **121 -> 122**
**ADR:** none this stage. The strict `^> **STATUS: PROPOSED` guard STAYS AT 0; the SPEC re-arms it 0 -> 1.

---

## 1. The pick, and why it is defensible as "smallest first"

**SELF-PICKED** per the 2026-07-12 standing directive. No banked mid-lifecycle work existed and that was
PROVEN, not assumed: every one of the 121 rows read `done` and check (1) went silent with the denominator
asserted at 121 (§8).

**Charter: NORMALIZE `host` AND `:authority` ON THE HTTP/2 DOWNSTREAM LEG so the upstream request carries
exactly one authority, the way the reference does.**

Four candidates were costed by execution against this tip, each by a *built, run, and reverted* prototype.
The pick is smallest on **both** axes that matter — raw size and blast radius:

| candidate | production cost | files | packages | extra commitments |
|---|---|---|---|---|
| **`host`/`:authority` (THIS ROW)** | **+15 / −1** | **1** | **1** | **none** |
| decode-side trailers | +37 / −4 (net +33) | 3 | 1 | Lua coroutine rework (§4.2 N3) |
| ADR-0310 C1 drain | +161 / −31 (net +130) | 3 | 1 | none, but its own rationale is partly refuted |
| ADR-0310 C2 `max_request_headers_kb` | +76 / −2 (net +74) | 5 | **3** | new `stats` root, ADR-0041 + ADR-0061 amendments |
| ADR-0310 C3 SETTINGS advertisement | +31 / −19 (net +12) | 3 | 1 | **REJECTED — measured ANTI-parity** |

It also carries **zero** on every count axis the project tracks: no new stat, no config field, no fuzzer,
no BackendKind, no `go.mod` module, no ADR amendment.

⚠️ **THE ROW IS NOT "SMALL" IN SUBSTANCE.** It has **five** measured divergent arms across **two** codec
legs, and on one of them the SUBJECT IS THE STRICTER SIDE — the opposite polarity from the rest. The +15
figure is a **FLOOR for the narrowest scope only** (§6).

---

## 2. The defect, REPRODUCED BY EXECUTION at `6b4bc7c0` — positive control on every arm, negative control on the isolating arm

Instrument: a purpose-written raw `x/net/http2` **framer client** (byte-exact ordered pseudo + regular
header list) against a raw framer **backend holding ONE `hpack.Decoder` PER CONNECTION**. All four proxied
arms landed on ONE pooled upstream connection (streams 1/3/5/7) and every one decoded cleanly, so the
connection-scoped-dynamic-table hazard was **exercised, not dodged**. Every listener bind asserted via
`ss -ltnp` before any result was read. Reference is `envoyproxy/envoy:contrib-v1.37.2` per `ENVOY_TARGET.md`.

### 2.1 Positive control — arm P, `:authority` only, no `host`

```
SUBJECT   RECV nfields=5 [":method"="GET" | ":path"="/probe/sub-p" | ":scheme"="http" | ":authority"="auth.example.com" | "x-probe"="sub-p"]
REFERENCE RECV nfields=8 [":method"="GET" | ":path"="/probe/ref-p" | ":scheme"="http" | ":authority"="auth.example.com" | "x-probe"="ref-p" | "x-forwarded-proto"="http" | "x-request-id"=… | "x-envoy-expected-rq-timeout-ms"="15000"]
```

Identical modulo the three headers only Envoy adds. **Known-good behaves as known-good on both sides.**

### 2.2 Arm A — both `:authority` AND `host` inbound: the subject FORWARDS BOTH

```
SUBJECT   RECV nfields=6 [… | ":authority"="auth.example.com" | "host"="hostheader.example.com" | "x-probe"="sub-a"]
REFERENCE RECV nfields=8 [… | ":authority"="auth.example.com" | "x-probe"="ref-a" | …]     # host GONE
```

### 2.3 Arm B — `host` only: the subject emits a PRESENT-AND-EMPTY `:authority`

```
SUBJECT   RECV nfields=6 [… | ":authority"="" | "host"="hostonly.example.com" | "x-probe"="sub-b"]
REFERENCE RECV nfields=5 [… | ":authority"="hostonly.example.com" | "x-probe"="ref-b" | …] # host GONE, PROMOTED
```

### 2.4 ⚠️ Arm C — NEW AT THIS STAGE, NOT IN THE PHASE-89 RECORD: the reference KILLS the request

Explicit `:authority: ""` present alongside `host`:

```
SUBJECT   200, forwards ":authority"=""
REFERENCE CLIENT READ-ERR EOF ; backend received NOTHING ;
          http.ingress_h2.downstream_cx_protocol_error: 1
          downstream_rq_http2_total: 4  vs  downstream_rq_completed: 3
```

The reference does not drop-and-proceed here. **It tears the connection down.**

### 2.5 ⚠️ THE NEGATIVE CONTROL ON THE ISOLATING ARM — and why arm B needs one

The whole of arm B rests on distinguishing **`:authority` absent** from **`:authority` present but empty**.
A decode bug would look identical. So the same client sent the same header sets **DIRECT to the backend,
bypassing both proxies**:

```
DIRECT-B  RECV nfields=5 [":method" | ":path" | ":scheme" | "host"="hostonly.example.com" | "x-probe"]      # :authority ABSENT
DIRECT-C  RECV nfields=6 [… | ":authority"="" | "host"="hostonly.example.com" | …]                          # PRESENT-AND-EMPTY
```

**The instrument discriminates the two states.** Without this control, arm B is unfalsifiable.

### 2.6 The H1 leg diverges too — and on one arm the SUBJECT IS STRICTER

The phase-89 record asserts only the H/2 leg. Measured here, with an H1 upstream cluster on both sides to
remove the H1→H2-cluster confound (§7 hazard 3):

| arm | subject | reference | |
|---|---|---|---|
| H1-P `Host: h1.example.com` | `Host: h1.example.com` | `host: h1.example.com` | AGREE |
| H1-A absolute-form + `Host:` | `Host: abs.example.com` | `host: abs.example.com` | AGREE |
| H1-E `Host:` empty, HTTP/1.1 | 200, empty | 200, empty | AGREE |
| **H1-B** HTTP/1.0, no Host | 200, forwards **empty** `Host:` | **426 Upgrade Required**, zero delivery | **DIVERGE** |
| **H1-D** duplicate `Host:` ×2 | **400**, zero delivery | 200, `host: one.example.com,two.example.com` | **DIVERGE — SUBJECT STRICTER** |

⚠️ **H1-E REFUTES any "the reference guarantees a non-empty authority" generalization.** It forwards an
empty `host` with a 200 when the client sends one on HTTP/1.1.

---

## 3. The mechanism, stated precisely

`internal/filter/hcm/h2/stream.go` builds **two** things from the decoded field list, and both mishandle `host`:

- `buildH2Request` (`:331`) fills `H2Request.Authority` from `:authority` only, and its `default:` arm
  appends every non-pseudo field — **including `host`** — onto `out.Headers`, the ordered carrier the
  upstream HEADERS block is emitted from. Nothing ever promotes, and nothing ever suppresses.
- `buildRequest` (`:409`) independently fills the local `authority` from `:authority` only and does
  `regular.Add(name, …)` for `host`, then sets both `u.Host` and `http.Request.Host` from `authority`.
  So on arm B the **route table sees an empty Host** as well.

That second point is why this is not cosmetic: `http.Request.Host` feeds route matching, so an empty
authority is a routing input, not just a wire artifact.

**The reference's rule, measured across arms P/A/B/C:** exactly one authority reaches the upstream —
`:authority` wins if non-empty, `host` is promoted when `:authority` is absent, the regular `host` is
never forwarded, and an explicitly-empty `:authority` is a connection error.

---

## 4. Rejected alternatives — EVERY COST RE-DERIVED AT THIS TIP

⚠️ Per `reference_deferred_candidate_cost_restale`, no figure below is carried from a prior stage; each is
from a prototype built, run, and reverted at `6b4bc7c0`, with `sha256sum -c` proving the restore.

### 4.1 ADR-0310 C3 — `SETTINGS_MAX_HEADER_LIST_SIZE` advertisement — **REJECTED, MEASURED ANTI-PARITY**

```
envoy-go   SETTINGS len=36 NumSettings=6:  0x3=100  0x4=65535  0x5=16384  0x2=0  0x1=4096  0x9=1   => NO 0x6
reference  SETTINGS len=24 NumSettings=4:  0x1=4096 0x8=0      0x3=1024   0x4=16777216             => NO 0x6
```

**The reference does not advertise `0x6` either**, and not because the limit is defaulted — re-run with
`max_request_headers_kb: 96` explicitly set, the frame is byte-identical and still carries no `0x6`.
A positive reading, not an absence of reading: a SETTINGS frame WAS received, `NumSettings=6`, all six
enumerated, `0x6` absent from a **non-empty** set.

⚠️ **ADR-0310 §Consequences (x), the phase-88 PLAN §10, and `BEHAVIOR_CONTRACT.md`'s `## HTTP/2` bullet
ALL file this as deferred PARITY work. All three are wrong.** Implementing it would CREATE a divergence.

Worse, it is not a pure wire change. With `0x6` advertised, x/net-based clients — **which includes this
repo's own differential drivers** — start refusing to send oversized requests client-side
(`http2: request header list larger than peer's advertised limit`, server received no HEADERS;
`transport.go:3118` stores `cc.peerMaxHeaderListSize`, enforced at `:2239` and `:2296`). And envoy-go
itself ignores a peer's `0x6`: the qualified symbol `SettingMaxHeaderListSize` has **zero** hits repo-wide.
Advertising alone is a wire-level lie — with `0x6=61440` advertised, a 100 KiB block still returned 200.

**Defensible residue, folded forward as a note rather than a row:** a test pinning `0x6` **absent on both
sides**, plus a pin on the SETTINGS parameter set — which is currently **entirely unpinned** (adding a 7th
setting on the wire left all 197 `=== RUN` rows green; `TestSettings_RoundTrip` reads the frame but never
asserts the set).

### 4.2 Decode-side trailers — REJECTED on a MEASURED hidden cost

Reproduced cleanly (subject `TRAILERS: <NONE>` vs reference `TRAILERS(wire): x-p90-trailer=t1`, with
positive and negative controls), and the forwarding fix is only **+37 / −4 over 3 files**, green at
`RUN=519 PASS=340 FAIL=0`.

⚠️ **But wiring `RunDecodeTrailers` does NOT make the trailer filter hook observable, and this was
MEASURED, not reasoned.** With the prototype installed and the hook **provably firing**
(`RunDecodeTrailers ENTER n=1 map=map[x-p90-trailer:[t1]]`), Lua **still** logged `LUA-TRAILERS-NIL` —
and the NIL was emitted **before** the ENTER line. `lua/decode_headers.go` runs `envoy_on_request` as a
coroutine at DecodeHeaders and `bodyChunks()` does not park until end-of-stream, so `DecodeTrailers`'
assignment lands after the script has finished. The *useful* version additionally touches
`lua/decode_headers.go`, `lua/body.go`, `lua/lua.go` — coroutine-suspension semantics, categorically larger.

**A row that ships the +33 forwarding fix alone would deliver a hook no filter can observe.**

### 4.3 ADR-0310 C1 — stream-scoped parity via decode-and-discard drain — DEFERRED, still the best NEXT row

Live and measured (subject `GOAWAY ENHANCE_YOUR_CALM(11)` vs reference `RST_STREAM INTERNAL_ERROR(2)` at
a 17 MB flood), mechanism proven with a working negative control, and the mechanism is a **documented
facility of the already-pinned dependency** (`x/net@v0.34.0` `hpack.Decoder.SetEmitEnabled`). Cost
**+161 / −31 net +130**, one package. Rejected here only on size — it is ~8.7× this row.

⚠️ Two things a future C1 row must not re-learn:
- **The client leg is FORCED, proven by compile error** (`client.go:454:49: a.buf undefined`) — the
  accumulator is shared. That reproduces phase-88's D-88-SEQ atomicity by measurement.
- **The 11-row phase-88 continuation roster stays GREEN across the full rewrite**, so it is
  behaviour-anchored and gives a C1 implementation almost no help.

### 4.4 ADR-0310 C2 — `max_request_headers_kb` + `http2.header_overflow` — DEFERRED, and it must not go first

**+76 / −2 over 5 files across THREE packages.** Smaller in lines than C1, larger in commitments, and
⚠️ **its enforcement point (`onHeaderBlock`, post-decode) is REPLACED by C1's incremental one — doing C2
first means writing code C1 deletes.**

⚠️ **And the stat would be invisible where this project asserts stats.** `http2.` is not a recognized
`ExtractTags` root, so the counter is correct in `/stats` and **silently dropped** from
`/stats/prometheus`:

```
stats: WriteProm skipped 1 registered metric name(s) with no recognized top-level segment: http2.header_overflow
```

A differential asserting there would read 0 subject / 2 reference — **a false divergence produced by the
renderer, not the feature.** Fixing it is a new SN rule, i.e. an ADR-0061 amendment (13 roots -> 14).

### 4.5 The `host`/`:authority` reference-exact form on BOTH legs — DEFERRED as scope, not rejected

Closing arms H1-B and H1-D as well is strictly larger and touches
`internal/filter/hcm/connection.go :: (*Filter).dispatchRequest`. **This row's scope decision belongs to
the SPEC** (§7 Q1). It is named here so it is not silently absorbed.

---

## 5. Family attribution

**Core-HCM / HTTP-2-dispatch MAINTENANCE row claiming NO family ordinal** — a maintenance row repairs a
landed deliverable and does not extend a charter (the row-85/86/87/88/89 precedent).

⚠️ **Provenance is OUTSIDE every sentinel window**, like row 89's. A per-line grep of all six windows at
`:199 :205 :211 :221 :227 :235` for `authority`, `host header` or `h2-host` returns **0**. The provenance
is the phase-89 close (`STATE.md:18` and the phase-89 IMPL's D-89-HOST deferral).

**No window narrows at row-done.** The only sentinel-affecting edit this row makes is the +1 data row,
which shifts the six window ANCHORS +1 without changing their COUNT or CONTENT — **MEASURED on both sides
at this BRAINSTORM (§8), never forecast.**

---

## 6. Anticipated ADR, counts, and the cost FLOOR

- **Anticipated ADR-0312** — TAIL-derived (`grep -oE '^## ADR-[0-9]+' … | tail -1` -> `## ADR-0311`;
  `grep -c '^## ADR-0312'` => **0**). ⚠️ NEVER derive from the heading count — headings are 310 and
  headings+1 COLLIDES at the ADR-0209 gap. ⚠️ A NEW ADR TAKES NO `---` SEPARATOR (`^---$` stays 216).
  The SPEC drafts §Context and re-arms the strict `PROPOSED` guard **0 -> 1**.
- **Anticipated counts, all +0:** stat surface **406** delta 0 · fuzzers **55 / 48** · BackendKind tail
  **38** · `go.mod` +0 · config fields +0 · blank imports **121**.
- **Fixtures 121 is NOT yet anticipated +0** — extend-`0004` vs mint-`0120` is a SPEC decision, and
  `0120` STAYS UNCONSUMED at this stage.
- **Cost FLOOR: +15 / −1 production over ONE file and TWO symbols** — `buildH2Request` and `buildRequest`
  in `internal/filter/hcm/h2/stream.go`. ⚠️ **A MEASURED PROTOTYPE IS A LOWER BOUND**
  (`reference_measured_prototype_is_a_lower_bound`); the floor covers arms A and B ONLY.

**NAMED under-enumeration, each a real cost the SPEC must price:**

1. `h2/client.go :: (*ClientConn).RoundTrip` emits `:authority` unconditionally — a defensive
   empty-authority policy lands here.
2. `h2dispatch.go :: h2ReconcileSkipKey` currently DROPS a filter-written `host` (D-89-HOST: SKIP).
   Under a promotion model it arguably belongs on `Authority`. **UNRESOLVED BEHAVIOR DECISION.**
3. `h2dispatch.go :: (*chainDispatchAction).WriteH2` — the `:authority` injection onto `c.req.Header`.
4. **Arm C is not covered by the floor.** The prototype PROMOTES where the reference REJECTS; closing C
   means a new reject in `buildRequest`.
5. The H1 leg entirely (H1-B 426, H1-D coalesce).
6. **The H3 leg entirely — UNPROBED.** `h3dispatch.go` mirrors the `:authority` injection but request
   construction is delegated to quic-go's http3.
7. Differential fixture cost — see §7 Q2, and note the instrument question below.

⚠️ **NO EXISTING TEST PINS THE DEFECT — CONTROLLER-VERIFIED.** Green baseline first
(`go test ./internal/filter/hcm/... -count=1` => exit 0), then green **with** the prototype applied, and
green across the whole tree (`go test ./internal/... -count=1` => **69 packages ok**). The one failure,
`TestAcquireH2Stream_PromoteSkipsDrainingConn`, went **6/6 green on retry** with the prototype still
applied — a flake, not prototype-induced. **So the fix is cheap but ships with ZERO guard; the row must
bring its own.**

---

## 7. What the SPEC owes

**Q1 — SCOPE.** Which arms does row 90 close? A+B only (the +15 floor)? A+B+C? Both legs? Each addition is
priced in §6. **Decide by measurement, not by symmetry.**

**Q2 — ⚠️ THE DIFFERENTIAL ARM CANNOT USE `helpers.H2RoundTrip`, AND THIS IS PROVEN AT THE PINNED SOURCE.**

```
golang.org/x/net@v0.34.0  http2/transport.go:2162-2166
    if asciiEqualFold(k, "host") || asciiEqualFold(k, "content-length") {
        // Host is :authority, already sent.
        continue                      <-- a client-set `host` header is SILENTLY DROPPED
    }
http2/transport.go:2146   f(":authority", host)   <-- :authority ALWAYS synthesized from req.Host/req.URL.Host
```

`helpers.H2RoundTrip` (`test/helpers/h2.go:33`) builds an `*http.Request`, does `req.Header.Add(...)` per
field and hands it to `cc.RoundTrip`, so it inherits both behaviors. **All three H/2 shapes are
STRUCTURALLY INEXPRESSIBLE through it.** This is the same class as the phase-89 PLAN's `order=` line,
which was only refuted at IMPL — **caught at BRAINSTORM this time.**

✅ **The instrument already exists in-tree.** `test/fixtures/0119-grpc-unary-trailers/driver/driver.go` is a
full raw-framer H/2 client — `io.WriteString(conn, http2.ClientPreface)` at `:474`, `http2.NewFramer` at
`:478`, `hpack.NewDecoder` at `:479`, `fr.WriteHeaders` at `:491`, and an explicit hand-built pseudo-header
list including `{Name: ":authority", …}` at `:355-358`. The row extends an existing sanctioned pattern.

**Q3 — WHICH FIXTURE.** ⚠️ **The H2-capable downstream set is FOUR fixtures, not one** —
`0004-h2-routing`, `0079-h2-multiplex-pool`, `0080-h2-goaway-rotation`, `0119-grpc-unary-trailers`
(§9 refutation 1). Extend one, or mint `0120`?

**Q4 — arm C's reject.** Does envoy-go adopt the reference's connection-level teardown, or is it a NAMED
DEPARTURE? Note the reference reaction is a `downstream_cx_protocol_error`, not a 4xx.

**Q5 — H1-D polarity.** On duplicate `Host`, the subject is STRICTER (400 vs coalesce). Adopting parity
would make envoy-go **stop rejecting** something it rejects today. Is that desirable? A NAMED DEPARTURE
may be the better answer.

**Q6 — routing blast radius.** `buildRequest` sets `http.Request.Host` from `authority`; promotion changes
a ROUTE-MATCHING input for host-only requests. Which existing route tests cover host matching?

**Q7 — `h2ReconcileSkipKey`.** Reconcile the promotion model against D-89-HOST's SKIP decision (§6 note 2).

---

## 8. Sentinel — RUN MECHANICALLY, BOTH SIDES, ACTUAL OUTPUT RECORDED

**BEFORE the row add** (`want=121`): (1) **SILENT** · (2) **SIX** at `:199 :205 :211 :221 :227 :235` ·
(3) **SILENT**. 239 lines / 121 data rows.

**AFTER the row add** (`want=122`): (1) **`NOT DONE: row 90`** — ALONE · (2) **SIX** at
`:200 :206 :212 :222 :228 :236` · (3) **SILENT**. 240 lines / 122 data rows.

⚠️ **A DRAFT OF THIS SECTION PREDICTED "(1) SILENT" AFTER THE ADD AND WAS WRONG — CORRECTED TO THE
MEASURED OUTPUT.** Row 90 is `in-progress`, so check (1) MUST name it; a silent check (1) here would mean
the row never landed. **This is why the rule is record-don't-forecast**, and the error was caught only by
running the gate rather than reasoning about it.

⇒ **THE SENTINEL DOES NOT FIRE.** It fails on check (1) (row 90 in-progress) AND on check (2) (still SIX).
**`stop` NOT created** — verified absent at the git root AND in the stage worktree, on both sides. Window
COUNT and CONTENT unchanged; only the anchors shift +1, exactly as §5 says.

**ALL FOUR NCs FIRED on the post-add file:**

- **NC-A** row-62 doctoring, `NC LANDED? [ in-progress ]` INSPECTED FIRST => `NOT DONE: row 62` **and**
  `NOT DONE: row 90`. ⚠️ **The NC-A signature CHANGES SHAPE while a row is in-progress** — it reads ALONE
  only on an all-`done` board. Both lines must be present: row 62 proves the check is live, row 90 proves
  the row landed. A reading of `row 62` alone here would mean the row add was LOST.
- **NC-B** denominator at `want=121` => `GATE FAIL: examined 122 data rows, expected 121` (alongside the
  expected `NOT DONE: row 90`).
- **NC-C** check-(3) doctoring, residual **2 -> 0** confirmed first => `NEVER OPENED: gRPC`, WASM silent.
- **NC-D** check-(2) one-arm => long **5** / short **1** / union **6** (a one-arm strip is NOT an NC for
  the union).

**Row 90 field count = 8**, escape-aware, verified BEFORE installing.

---

## 9. Findings this stage produced that the next stage must not re-learn

1. ⚠️ **THE PHASE-89 CODEC CENSUS IS PROSE-CONTAMINATED AND STRUCTURALLY BLIND.** It records
   `codec_type HTTP1 270 / AUTO 6 / HTTP3 3 / HTTP2 ZERO`. Config-only over `test/fixtures/*/*.yaml`
   reads **HTTP1 212 / AUTO 2 / HTTP2 0 / HTTP3 0** — the larger figures count README prose and Go
   comments. **And the YAML view is blind to 46 of 121 fixtures**, which carry no `envoy-go.yaml` and
   build config inside their Go driver (driver-side: AUTO 3 / HTTP1 50 / HTTP3 2). **The true H2-capable
   downstream set is FOUR fixtures**, not the one a YAML grep suggests. *"No fixture has ever exercised
   downstream H2"* is false.
2. ⚠️ **THE ARM-A MALFORMED-ROW FIGURE RECONCILES ONLY UNDER AN ESCAPE-AWARE FORM.** A naive
   `awk -F'|'` `NF != 8` reads **SEVENTEEN** rows; stripping `\|` first reads exactly ids **57**
   (`fields=9`) and **69** (`fields=10`) at lines 119/131, as carried. **The naive count is not a drift
   signal — do not "correct" the figure.**
3. ⚠️ **THE `STATE_HISTORY.md` "archive labels 202" FIGURE IS NOT REPRODUCIBLE.** Six plausible matcher
   forms give **203 / 163 / 205 / 166 / 165 / 209**; none is 202. The cause is visible: the positive
   control reads **3**, not 1, because entries are cross-referenced inside other entries' prose.
   **Carry NO number.** Use the ANCHORED form (`^- \*\*prior active-phase:\*\*`) for the absence guard —
   and note that writing an eviction NOTE naming the evictee makes a LOOSE grep read 1 in `STATE.md`.
4. ⚠️ **A DRIFT CORRECTION IS ITSELF A CLAIM — ONE FIRED THIS STAGE AND WAS REFUTED.** A probe reported
   the stat-surface figure 406 as stale, "tip measures 403". The canonical command counts **occurrences
   repo-wide** and reads **406** at this tip exactly as carried (405 in `internal/` + 1 in
   `cmd/envoy-go/main_test.go`); the 403 is a **LINE** count scoped to `internal/`. Different unit AND
   different scope. **The carried figure was right.**
5. ⚠️ **THE "~64 KiB ENCODED BAND" IN ADR-0310 §Consequences (xi) AND `BEHAVIOR_CONTRACT.md` IS NOT
   REPRODUCIBLE.** Against the same pinned image with RFC-legal CONTINUATION framing, encoded
   62170 / 66506 / 70170 / 100182 / 150586 / 400729 / 1000229 **all** returned `RST_STREAM
   INTERNAL_ERROR(2)`, stream-scoped, booking `http2.header_overflow` every time. Three independent
   framings were tried; the band appeared in none, and **`COMPRESSION_ERROR(9)` was never observed from
   the reference in any arm.** `continuation_test.go`'s `AcceptsPastReferenceLimit` justifies its 96 KiB
   by that band — the justification does not survive re-measurement, though the 96 KiB figure is fine.
   **RECORDED, NOT FIXED** (append-only doc; and this row does not touch that surface).
6. ⚠️ **`RunDecodeTrailers` "no seam exists" IS REFUTED.** `FilterChain.RunDecodeTrailers`
   (`chain.go:490`) is a complete working seam with **zero production callers**. Missing are CAPTURE
   (`serverStream` drops the fields), CARRIAGE (`H2Request` has no `Trailers`) and EMISSION (`RoundTrip`
   writes no trailing block) — three concrete absences, not an absent seam.
7. ⚠️ **envoy-go FORWARDS H1 request trailers UNGATED, where the reference DROPS them by default.**
   `enable_trailers` occurs **zero** times repo-wide; the reference needs it at **two** independent config
   sites. An unrecorded, opposite-direction divergence — parity would make envoy-go stop doing something
   it does today.
8. ⚠️ **THE H1→H2-UPSTREAM 502 BLOCKER: THE RECORDED CITE NAMES THE WRONG SITE, AND THE BLOCKER IS
   TWO-LEGGED.** `connection.go:467` has not drifted but is the *injection* site (`rf.SetAction`);
   *selection* is at `connection.go:339`, and `actions.go :: (*clusterRouteAction).asRouterAction`
   memoizes `router.H1ClusterAction` with **no `UseH2()` consultation**. `h3dispatch.go:139` has the
   **same defect**, self-documented at `:138`. Mechanism caught on the wire:
   `CONN 2 BAD-PREFACE "POST /probe/h1 HTTP/1.1\r"` — envoy-go negotiates ALPN `h2` and then writes an
   HTTP/1.1 request line onto it.
9. ⚠️ **THE ADVERTISED SETTINGS PARAMETER SET IS ENTIRELY UNPINNED.** Adding a 7th setting on the wire
   left all 197 `=== RUN` rows green.

---

## 10. Probe hygiene

Three probe agents ran on disjoint detached worktrees with private scratch and disjoint port bands. All
three committed nothing, pushed nothing, restored every patched tracked file with `sha256sum -c` verified,
proved `git status --porcelain` and `git diff --stat 6b4bc7c0` both EMPTY, tore down every container BY
NAME, and removed their worktrees.

Foreign containers present throughout and **deliberately left untouched**: `infallible_booth`,
`crazy_kare`, `golink-ai`, `quizzical_goldstine`.

⚠️ **TWO CROSS-CUTTING HAZARDS FIRED AND BOTH ARE METHOD-LEVEL:**

1. **A probe agent's `pgrep -f 'envoy-go -c'` matched a SIBLING agent's process and killed it**
   (PID 2870478 — which belonged to another probe, confirmed by that probe's own log:
   `signal received; initiating graceful drain`, then `connection refused`). The controller issued an
   advisory to both surviving probes; the affected probe **discarded that arm entirely** and re-ran every
   dependent arm with the server launched and probed inside a single call and `ss -ltnp` asserted before
   and after. **All re-run results held.** `reference_parallel_agents_shared_machine_namespaces` fired for
   real. **Kill only PIDs whose cmdline contains your own scratch path.**
2. ⚠️ **EVERY PORT BAND THIS LOOP HAS ASSIGNED SINCE PHASE 87 SITS INSIDE THE KERNEL EPHEMERAL RANGE.**
   `net.ipv4.ip_local_port_range = 32768 60999`, and the assigned bands were 47410-47441, 47450-47559,
   47560-47999 and 48000+. A probe hit three `bind: address already in use` failures on ports `ss -ltn`
   reported FREE — held as TIME-WAIT or outbound sockets, and docker's `-p` proxy does not set
   `SO_REUSEADDR`. **This is also the mechanism behind the long-carried "driver-owned receiver port race":
   all three recorded ports — 35097, 35323, 42039 — are inside the ephemeral range, while the static
   fixture band (10000-19172) and the reserved bands (20000-31007, 11000-14999) are all BELOW it, which is
   exactly why those never race.** Future bands must be chosen **below 32768**, and port availability
   checked with `ss -tan` (ALL states), never `ss -ltn`.

Other hazards worth carrying: `hpack.NewDecoder(4096, nil)` **panics with SIGSEGV** in `callEmit` — a nil
emit func is not "discard"; x/net's `Transport` does **not** apply a peer's `0x6` to the first request on a
fresh connection (needs a warm-up request, else a confident false "x/net ignores it");
`bash /dev/tcp` + `timeout cat <&3` inside command substitution returned **silently empty** on four arms,
a false "no reply"; `go test` **without `-v` prints zero `=== RUN`**, so a census can read `RUN=0` beside
`RC=0` — a vacuous green; and `gofmt` realignment moved a measured diff from `+35/−2` to `+37/−4`, so a
pre-`gofmt` cost figure **understates**.
